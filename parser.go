package rmarsh

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// A Token represents a single distinct value type read from a Parser instance.
type Token uint8

// The valid token types.
const (
	tokenInvalid = iota
	tokenStart
	TokenNil
	TokenTrue
	TokenFalse
	TokenFixnum
	TokenFloat
	TokenBignum
	TokenSymbol
	TokenString
	TokenStartArray
	TokenEndArray
	TokenStartHash
	TokenEndHash
	TokenStartIVar
	TokenIVarProps
	TokenEndIVar
	TokenLink
	TokenUsrMarshal
	TokenEOF
)

var tokenNames = map[Token]string{
	TokenNil:        "TokenNil",
	TokenTrue:       "TokenTrue",
	TokenFalse:      "TokenFalse",
	TokenFixnum:     "TokenFixnum",
	TokenFloat:      "TokenFloat",
	TokenBignum:     "TokenBignum",
	TokenSymbol:     "TokenSymbol",
	TokenString:     "TokenString",
	TokenStartArray: "TokenStartArray",
	TokenEndArray:   "TokenEndArray",
	TokenStartHash:  "TokenStartHash",
	TokenEndHash:    "TokenEndHash",
	TokenStartIVar:  "TokenStartIVar",
	TokenIVarProps:  "TokenIVarProps",
	TokenEndIVar:    "TokenEndIVar",
	TokenLink:       "TokenLink",
	TokenUsrMarshal: "TokenUsrMarshal",
	TokenEOF:        "EOF",
}

func (t Token) String() string {
	if n, ok := tokenNames[t]; ok {
		return n
	}
	return "UNKNOWN"
}

// A ParserError is a description of an error encountered while parsing a Ruby Marshal stream.
type ParserError struct {
	msg    string
	Offset int
}

func (e ParserError) Error() string {
	return e.msg
}

// Parser is a low-level pull-based parser of the Ruby Marshal format.
// A Parser will pull bytes from an underlying io.Reader as needed, but will never buffer past the
// end of the current Marshal stream. Even though effort is made to be as efficient in pulling bytes
// as possible, if the Marshal data is already fully available then it should be wrapped in a bufio.Reader
// before being handed to a Parser.
// Parser is very low level and is mostly intended as a building block for the Decoder. You probably
// want to be using that.
type Parser struct {
	r io.Reader // our byte source.

	buf    []byte // The read buffer contains every byte of data that we've read from the stream.
	bufcap int    // Current capacity of the read buffer.
	buflen int    // The number of bytes we've read into the read buffer.
	pos    int    // Our byte position in the read buffer.

	state parserState
	stack parserStack
}

func NewParser(r io.Reader) *Parser {
	return &Parser{
		r:      r,
		buf:    make([]byte, bufInitSz),
		bufcap: bufInitSz,
		state:  parserStateTopLevel,
	}
}

// Reset reverts the Parser into the identity state, ready to read a new Marshal 4.8 stream from the existing Reader.
// If the provided io.Reader is nil, the existing Reader will continue to be used.
func (p *Parser) Reset(r io.Reader) {
	p.stack = p.stack[0:0]
	// p.cur = tokenInvalid
	p.state = parserStateTopLevel

	// If this a replay Parser, our reset is a little less ... reset-y.
	// if p.lnkID > -1 {
	// 	p.pos = p.lnkTbl[p.lnkID].beg
	// 	p.stack = p.stack[0:0]
	// 	return
	// }

	if r != nil {
		p.r = r
	}
	p.pos = 0
	p.buflen = 0
	// p.symTbl = p.symTbl[0:0]
	// p.lnkTbl = p.lnkTbl[0:0]
}

func (p *Parser) Read() (tok Token, b []byte, num int, err error) {
	// Quick early bailout check here. If parser state is "parserStateEOF" then we can just
	// return an EOF token and exit.
	if p.state == parserStateEOF {
		tok = TokenEOF
		return
	}

	// Gets set to false after we run SM.
	runSM := true

	// READ BYTES IF NECESSARY
	// Running the state machine can bail out back here if there's not enough data in the read buffer
	// to transition to the next state.
	// This code would be WAY less complicated if we just filled the buffer with method calls when needed...
	// But that costs too many precious nanos.
	needed := 0
pullbytes:
	if needed > 0 {
		// TODO: port over the stack-based prefetch here.

		from, to := p.buflen, p.buflen+needed

		if to > p.bufcap {
			// Overflowed our read buffer, allocate a new one double the current size, or the required size if it's larger.
			p.bufcap = p.bufcap * 2
			if p.bufcap < to {
				p.bufcap = to
			}
			buf := make([]byte, p.bufcap)
			copy(buf, p.buf[0:p.buflen])
			p.buf = buf
		}

		p.buflen += needed

		var n int
		for from < to && err == nil {
			n, err = p.r.Read(p.buf[from:to])
			from += n
		}
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
			return
		} else if err != nil {
			err = errors.Wrap(err, "fill")
			return
		}

		needed = 0
	}

	// RUN THE STATE MACHINE
	if runSM {
		switch p.state {
		// the initial state of a Parser expects to read 2-byte magic and then a top level value
		case parserStateTopLevel:
			if p.pos == 0 {
				// We can safely pull up to 3 bytes immediately. 2 bytes for the magic and all top level values
				// will be at least 1 byte large.
				if p.buflen < 3 {
					needed = 3 - p.buflen
					goto pullbytes
				}

				if p.buf[p.pos] != 0x04 || p.buf[p.pos+1] != 0x08 {
					err = p.parserError("Expected magic header 0x0408, got 0x%.4X", int16(p.buf[p.pos])<<8|int16(p.buf[p.pos+1]))
					return
				}
				p.pos = 2
			}

			// Our next state is EOF.
			// Unless we read something interesting below which pushes something onto the stack.
			p.state = parserStateEOF
		}

		// Now that we've run the SM, we don't want to run it again if the stream reads
		// need to go back to pullbytes
		runSM = false
	}

	// READ SOMETHING FROM THE STREAM
	if p.pos == p.buflen {
		// This is the worst possible situation to be in - we have to go to the io.Reader to pull a single byte.
		// This situation shouldn't occur very often on real world streams - as we should usually have enough to context to
		// be doing safe read aheads.
		needed = 1
		goto pullbytes
	}

	typ := p.buf[p.pos]
	rd := 1

	switch typ {
	case typeNil:
		tok = TokenNil

	case typeTrue:
		tok = TokenTrue

	case typeFalse:
		tok = TokenFalse

	case typeFixnum:
		tok = TokenFixnum
		// num, err = p.long()

		if p.pos+rd+1 > p.buflen {
			needed = 1
			goto pullbytes
		}
		rd++

		var sz int
		num, sz = p.readLongByte(p.buf[p.pos+1])
		if sz > 0 && p.pos+rd+sz > p.buflen {
			needed = p.pos + rd + sz - p.buflen
			goto pullbytes
		} else if sz > 0 {
			for i := 0; i < sz; i++ {
				if num < 0 {
					num &= ^(0xff << uint(8*i))
				}

				num |= int(p.buf[p.pos+rd]) << uint(8*i)
				rd++
			}
		}
	}

	p.pos += rd

	return
}

// Given an encoded byte, determines if it represents a concrete long or the number of bytes
// needed to decode the real long.
func (p *Parser) readLongByte(b byte) (n int, sz int) {
	n = int(int8(b))
	if 4 < n && n < 128 {
		n = n - 5
	} else if -129 < n && n < -4 {
		n = n + 5
	} else if n > 0 {
		sz = n
		n = 0
	} else if n != 0 {
		sz = -n
		n = -1
	}

	return
}

// func (p *Parser) long() (n int, err error) {
// 	if p.pos == p.buflen {
// 		err = p.fill(1)
// 		if err != nil {
// 			err = errors.Wrap(err, "error parsing long")
// 			return
// 		}
// 	}

// 	n = int(int8(p.buf[p.pos]))
// 	p.pos++
// 	if n == 0 {
// 		return
// 	} else if 4 < n && n < 128 {
// 		n = n - 5
// 		return
// 	} else if -129 < n && n < -4 {
// 		n = n + 5
// 		return
// 	}

// 	// It's a multibyte positive/negative num.
// 	var sz int
// 	if n > 0 {
// 		sz = n
// 		n = 0
// 	} else {
// 		sz = -n
// 		n = -1
// 	}

// 	if p.pos+sz > p.buflen {
// 		err = p.fill(p.pos + sz - p.buflen)
// 		if err != nil {
// 			err = errors.Wrap(err, "error parsing long")
// 			return
// 		}
// 	}

// 	for i := 0; i < sz; i++ {
// 		if n < 0 {
// 			n &= ^(0xff << uint(8*i))
// 		}

// 		n |= int(p.buf[p.pos]) << uint(8*i)
// 		p.pos++
// 	}

// 	return
// }

// pull bytes from the io.Reader into our read buffer
// func (p *Parser) fill(num int) (err error) {
// 	// We don't do actual reads in sub Parser, the data is already in the buffer.
// 	// if p.lnkID > -1 {
// 	// 	return errors.Errorf("fill() called on replay Parser")
// 	// }

// 	// Whenever we go to pull bytes from the source, we prefetch as much as possible. We do this by examining the current
// 	// stack. For example if we're processing an array with 500 elements and we've currently parsing element 232, then we
// 	// know there's at *least* 267 bytes to come (even if every following element was just a single nil byte).
// 	for i := len(p.stack) - 1; i >= 0; i-- {
// 		if n := p.stack[i].sz - p.stack[i].pos - 1; n > 0 {
// 			num += n
// 		}
// 	}

// 	from, to := p.buflen, p.buflen+num

// 	if to > p.bufcap {
// 		// Overflowed our read buffer, allocate a new one double the current size, or the required size if it's larger.
// 		p.bufcap = p.bufcap * 2
// 		if p.bufcap < to {
// 			p.bufcap = to
// 		}
// 		buf := make([]byte, p.bufcap)
// 		copy(buf, p.buf[0:p.buflen])
// 		p.buf = buf
// 	}

// 	p.buflen += num

// 	var n int
// 	for from < to && err == nil {
// 		n, err = p.r.Read(p.buf[from:to])
// 		from += n
// 	}
// 	if err == io.EOF {
// 		err = io.ErrUnexpectedEOF
// 	} else if err != nil {
// 		err = errors.Wrap(err, "fill")
// 	}
// 	return
// }

// Constructs a ParserError using the current pos of the Parser.
func (p *Parser) parserError(format string, a ...interface{}) ParserError {
	return ParserError{fmt.Sprintf(format, a...), p.pos}
}

const (
	bufInitSz    = 256 // Initial size of our read buffer. We double it each time we overflow available space.
	rngTblInitSz = 8   // Initial size of range table entries
	stackInitSz  = 8   // Initial size of stack
)

type parserState uint8

const (
	parserStateTopLevel = iota
	parserStateArray
	parserStateArrayEnd
	parserStateHashKey
	parserStateHashValue
	parserStateHashEnd
	parserStateIVarInit
	parserStateIVarLen
	parserStateIVarKey
	parserStateIVarValue
	parserStateIVarEnd
	parserStateUsrMarshalInit
	parserStateUsrMarshalVal
	parserStateUsrMarshalEnd
	parserStateEOF
)

// parserCtx tracks the current state we're processing when handling complex values like arrays, hashes, ivars,  etc.
// Multiple contexts can be nested in a stack. For example if we're parsing a hash as the nth element of an array,
// then the top of the stack will be ctxTypeHash and the stack item below that will be ctxTypeArray
type parserCtx struct {
	typ  uint8
	sz   int
	pos  int
	r    *rng        // when this context is finished, r (pointing into lnkTbl) is updated with final location
	next parserState // Next state transition when we're done with this stack item
}

// The valid context types
const (
	ctxTypeArray = iota
	ctxTypeHash
	ctxTypeIVar
	ctxTypeUsrMarshal
	ctxTypeReplay
)

type parserStack []parserCtx

func (stk parserStack) cur() *parserCtx {
	if len(stk) == 0 {
		return nil
	}
	return &stk[len(stk)-1]
}

func (stk *parserStack) push(typ uint8, sz int, next parserState) *parserCtx {
	// We track the current parse sym table by slicing the underlying array.
	// That is, if we've seen one symbol in the stream so far, len(p.symTbl) == 1 && cap(p.symTable) == rngTblInitSz
	// Once we exceed cap, we double size of the tbl.
	l := len(*stk)
	if c := cap(*stk); l == c {
		if c == 0 {
			c = stackInitSz
		} else {
			c = c * 2
		}
		newStk := make([]parserCtx, c)
		copy(newStk, *stk)
		*stk = newStk[0:l]
	}

	*stk = append(*stk, parserCtx{typ: typ, sz: sz, r: nil, next: next})
	return &(*stk)[l]
}

func (stk *parserStack) pop() (next parserState) {
	next = (*stk)[len(*stk)-1].next
	*stk = (*stk)[0 : len(*stk)-1]
	return
}

// A rng encodes a pair of start/end positions, used to mark interesting locations in the read buffer.
type rng struct{ beg, end int }

// Range table
type rngTbl []rng

func (t *rngTbl) add(r rng) (err error) {
	// We track the current parse sym table by slicing the underlying array.
	// That is, if we've seen one symbol in the stream so far, len(p.symTbl) == 1 && cap(p.symTable) == rngTblInitSz
	// Once we exceed cap, we double size of the tbl.
	id := len(*t)
	if c := cap(*t); id == c {
		if c == 0 {
			c = rngTblInitSz
		} else {
			c = c * 2
		}
		newT := make([]rng, c)
		copy(newT, *t)
		*t = newT[0:id]
	}
	*t = append(*t, r)
	return
}
