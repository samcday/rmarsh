package rmarsh

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strconv"
	"unsafe"

	"github.com/pkg/errors"
)

// Parser is a low-level streaming implementation of the Ruby Marshal 4.8 format.
type Parser struct {
	cur  Token // The token we have most recently parsed
	prev Token // The last token we read

	lnkID  int // id of the linked object this Parser is replaying
	parent *Parser

	ctx rng // Range of the raw data for the current token

	num      int
	bnumbits []big.Word
	bnumsign byte

	symTbl rngTbl // Store ranges marking the symbols we've parsed in the read buffer.
	lnkTbl rngTbl // Store ranges marking the linkable objects we've parsed in the read buffer.
}

// NewParser constructs a new parser that streams data from the given io.Reader
// Due to the nature of the Marshal format, data is read in very small increments. Please ensure that the provided
// Reader is buffered, or wrap it in a bufio.Reader.
func NewParser(r io.Reader) *Parser {
	p := &Parser{
		lnkID: -1,
	}
	return p
}

// Replay is used to construct a new Parser that will replay the events of a previously parsed object.
func (p *Parser) Replay(lnkID int) (*Parser, error) {
	// Walk up the parent chain and ensure we aren't replaying something we're already replaying somewhere in the chain.
	for par := p; par != nil; par = par.parent {
		if par.lnkID == lnkID {
			return nil, errors.Errorf("Object ID %d is already being replayed by this Parser", lnkID)
		}
	}

	if lnkID >= len(p.lnkTbl) {
		return nil, errors.Errorf("Object ID %d not valid", lnkID)
	}

	rng := p.lnkTbl[lnkID]
	if rng.end == 0 {
		return nil, errors.Errorf("Object ID %d is currently being parsed and cannot be replayed", lnkID)
	}

	return &Parser{
		parent: p,
		lnkID:  lnkID,
		state:  parserStateTopLevel,
		r:      nil,
		buf:    p.buf,
		pos:    rng.beg,
		end:    rng.end,
		lnkTbl: p.lnkTbl,
		symTbl: p.symTbl,
	}, nil
}

// Next advances the parser to the next token in the stream.
func (p *Parser) Next() (tok Token, err error) {
	for tok == tokenInvalid {
		switch p.state {

		// state when reading elements of an array
		case parserStateArray:
			// sanity check the top of stack is an array.
			cur := p.stack.cur()
			if cur.typ != ctxArray {
				err = p.parserError("expected top of stack to be array, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			}

			cur.pos++
			if cur.pos == cur.sz {
				p.state = parserStateArrayEnd
			} else {
				p.state = parserStateArray
			}

		// state when we've finished parsing an array
		case parserStateArrayEnd:
			cur := p.stack.cur()
			if cur.typ != ctxArray {
				err = p.parserError("expected top of stack to be array, got %s", cur.typ)
				return
			}

			tok = TokenEndArray
			if cur.r != nil {
				cur.r.end = p.pos
			}
			p.state = p.stack.pop()

		// state when reading a key in a hash
		case parserStateHashKey:
			// sanity check the top of stack is an hash.
			cur := p.stack.cur()
			if cur.typ != ctxHash {
				err = p.parserError("expected top of stack to be hash, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			}
			p.state = parserStateHashValue

		// state when reading a value in a hash
		case parserStateHashValue:
			// sanity check the top of stack is an hash.
			cur := p.stack.cur()
			if cur.typ != ctxHash {
				err = p.parserError("expected top of stack to be hash, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			}

			cur.pos++
			if cur.pos == cur.sz {
				p.state = parserStateHashEnd
			} else {
				p.state = parserStateHashKey
			}

		// state when we've completed reading a hash
		case parserStateHashEnd:
			cur := p.stack.cur()
			if cur.typ != ctxHash {
				err = p.parserError("expected top of stack to be hash, got %s", cur.typ)
				return
			}

			// Hash is done, return an end hash token and pop stack.
			tok = TokenEndHash
			if cur.r != nil {
				cur.r.end = p.pos
			}
			p.state = p.stack.pop()

		// initial state of an ivar context - expects to read a value, then transitions to
		// parserStateIVarLength
		case parserStateIVarInit:
			cur := p.stack.cur()
			if cur.typ != ctxIVar {
				err = p.parserError("expected top of stack to be ivar, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			}

			p.state = parserStateIVarLen

		case parserStateIVarLen:
			cur := p.stack.cur()
			if cur.typ != ctxIVar {
				err = p.parserError("expected top of stack to be ivar, got %s", cur.typ)
				return
			}

			cur.sz, err = p.long()
			if err != nil {
				return
			}

			tok = TokenIVarProps

			if cur.sz == 0 {
				p.state = parserStateIVarEnd
			}
			p.state = parserStateIVarKey

		case parserStateIVarKey:
			cur := p.stack.cur()
			if cur.typ != ctxIVar {
				err = p.parserError("expected top of stack to be ivar, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			} else if tok != TokenSymbol {
				// IVar keys are only permitted to be symbols
				err = p.parserError("expected next token to be Symbol, got %q", tok)
				return
			}

			p.state = parserStateIVarValue

		case parserStateIVarValue:
			cur := p.stack.cur()
			if cur.typ != ctxIVar {
				err = p.parserError("expected top of stack to be ivar, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			}
			cur.pos++

			if cur.pos == cur.sz {
				p.state = parserStateIVarEnd
			} else {
				p.state = parserStateIVarKey
			}

		case parserStateIVarEnd:
			cur := p.stack.cur()
			if cur.typ != ctxIVar {
				err = p.parserError("expected top of stack to be ivar, got %s", cur.typ)
				return
			}

			if cur.r != nil {
				cur.r.end = p.pos
			}
			tok = TokenEndIVar
			p.state = p.stack.pop()

		case parserStateUsrMarshalInit:
			cur := p.stack.cur()
			if cur.typ != ctxUsrMarshal {
				err = p.parserError("expected top of stack to be usrMarshal, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			} else if tok != TokenSymbol {
				err = p.parserError("expected next token for usrmarshal object to be Symbol, got %s", tok)
				return
			}

			p.state = parserStateUsrMarshalVal

		case parserStateUsrMarshalVal:
			cur := p.stack.cur()
			if cur.typ != ctxUsrMarshal {
				err = p.parserError("expected top of stack to be usrMarshal, got %s", cur.typ)
				return
			}

			tok, err = p.readNext()
			if err != nil {
				return
			}

			p.state = parserStateUsrMarshalEnd

		case parserStateUsrMarshalEnd:
			cur := p.stack.cur()
			if cur.typ != ctxUsrMarshal {
				err = p.parserError("expected top of stack to be usrMarshal, got %s", cur.typ)
				return
			}

			if cur.r != nil {
				cur.r.end = p.pos
			}

			p.state = p.stack.pop()

		}
	}

	p.prev = p.cur
	p.cur = tok

	// Our state is woven through potentially many nested levels of context.
	// If we start a new context for an array/hash/ivar/whatever, we point its terminal
	// state at our next one. For example if the top level value was a single depth array,
	// once the array had finished parsing it would know to transition to parserStateEOF.
	// this method handles pushing new stack items when needed
	switch tok {
	case TokenStartArray:
		ctx := p.stack.push(ctxArray, p.num, p.state)
		// Make sure to attach the range we added to the lnkTbl in readNext()
		if p.lnkID == -1 {
			ctx.r = &p.lnkTbl[len(p.lnkTbl)-1]
		}
		if p.num == 0 {
			p.state = parserStateArrayEnd
		} else {
			p.state = parserStateArray
		}
	case TokenStartHash:
		ctx := p.stack.push(ctxHash, p.num, p.state)
		// Make sure to attach the range we added to the lnkTbl in readNext()
		if p.lnkID == -1 {
			ctx.r = &p.lnkTbl[len(p.lnkTbl)-1]
		}
		if p.num == 0 {
			p.state = parserStateHashEnd
		} else {
			p.state = parserStateHashKey
		}
	case TokenStartIVar:
		ctx := p.stack.push(ctxIVar, 0, p.state)
		if p.lnkID == -1 {
			ctx.r = &p.lnkTbl[len(p.lnkTbl)-1]
		}
		p.state = parserStateIVarInit
	case TokenUsrMarshal:
		ctx := p.stack.push(ctxUsrMarshal, 0, p.state)
		if p.lnkID == -1 {
			ctx.r = &p.lnkTbl[len(p.lnkTbl)-1]
		}
		p.state = parserStateUsrMarshalInit
	}

	return
}

// ExpectNext is a convenience method that calls Next() and ensures the next token is the one provided.
func (p *Parser) ExpectNext(exp Token) (err error) {
	var tok Token
	tok, err = p.Next()
	if err != nil {
		return
	}

	if tok != exp {
		err = p.parserError("read token %q, expected %q", tok, exp)
		return
	}

	p.cur = tok
	return
}

// ExpectSymbol is a convenience method to ensure the Next() token is a Symbol, returning the symbol
// that was read, or an error otherwise.
func (p *Parser) ExpectSymbol() (sym string, err error) {
	var tok Token
	tok, err = p.Next()
	if err != nil {
		return
	} else if tok != TokenSymbol {
		err = p.parserError(`read token %q, expected "TokenSymbol"`, tok)
	} else {
		sym = string(p.buf[p.ctx.beg:p.ctx.end])
	}
	return
}

// ExpectUnsafeSymbol behaves like ExpectSymbol(), except it returns an unsafe string that is only valid
// until the next call to Reset() on this parser.
func (p *Parser) ExpectUnsafeSymbol() (sym string, err error) {
	var tok Token
	tok, err = p.Next()
	if err != nil {
		return
	} else if tok != TokenSymbol {
		err = p.parserError(`read token %q, expected "TokenSymbol"`, tok)
	} else {
		sym, err = p.UnsafeText()
	}
	return
}

// ExpectString is a convenience method that consumes a string from the next token. If the next token
// is an IVar, it will be unwrapped and the encoding will be checked to ensure it's UTF-8.
func (p *Parser) ExpectString() (str string, err error) {
	var tok Token
	if tok, err = p.Next(); err != nil {
		return
	}

	if tok == TokenLink {
		var subp *Parser
		if subp, err = p.Replay(p.num); err != nil {
			return
		}
		return subp.ExpectString()
	}

	if tok == TokenString {
		str = string(p.buf[p.ctx.beg:p.ctx.end])
		return
	}

	if tok == TokenStartIVar {
		if tok, err = p.Next(); err != nil {
			return
		} else if tok != TokenString {
			err = p.parserError("invalid token %q - expected string", tok)
			return
		}

		str = string(p.buf[p.ctx.beg:p.ctx.end])

		if tok, err = p.Next(); err != nil {
			return
		} else if tok != TokenIVarProps {
			err = p.parserError("invalid token %q - expected ivar props", tok)
			return
		}

		isUTF8 := false
		for {
			if tok, err = p.Next(); err != nil {
				return
			} else if tok == TokenEndIVar {
				break
			} else if tok != TokenSymbol {
				err = p.parserError("invalid token %q - expected symbol", tok)
				return
			}

			if bytes.Equal(p.buf[p.ctx.beg:p.ctx.end], []byte{'E'}) {
				if tok, err = p.Next(); err != nil {
					return
				} else if tok == TokenTrue {
					isUTF8 = true
				}
			} else {
				if err = p.Skip(); err != nil {
					return
				}
			}
		}

		if !isUTF8 {
			err = p.parserError("read a string that is not UTF-8")
		}

		return
	}

	err = p.parserError("invalid token %q - expected string or ivar string", tok)
	return
}

// ExpectUsrMarshal is a convience method to ensure the Next() token is a TokenUsrMarshal, and that
// its class name matches the provided name.
func (p *Parser) ExpectUsrMarshal(name string) (err error) {
	var tok Token

	if tok, err = p.Next(); err != nil {
		return
	}

	if tok == TokenLink {
		var subp *Parser
		if subp, err = p.Replay(p.num); err != nil {
			return
		}
		return subp.ExpectUsrMarshal(name)
	}

	var sym string
	if sym, err = p.ExpectUnsafeSymbol(); err != nil {
		return
	}
	if sym != name {
		err = p.parserError("expected UsrMarshal of class %q but got %q", name, sym)
	}
	return
}

// Skip examines the current token, and will continuously read tokens until the current
// object is fully consumed. Does nothing for single token types like Fixnum, Bool, Nil, Bignum,
// String, Symbol, etc.
func (p *Parser) Skip() (err error) {
	var depth int
	if p.cur == TokenIVarProps {
		depth++
	}
	for {
		switch p.cur {
		case TokenStartArray, TokenStartHash, TokenStartIVar:
			depth++
		case TokenEndArray, TokenEndHash, TokenEndIVar:
			depth--
		case TokenUsrMarshal:
			_, err = p.Next()
			if err != nil {
				return
			}
			_, err = p.Next()
			if err != nil {
				return
			}
			continue
		}

		if depth == 0 {
			return nil
		}

		_, err = p.Next()
		if err != nil {
			return
		}
	}
}

// Len returns the number of elements to be read in the current structure.
// Returns -1 if the current token is not TokenStartArray, TokenStartHash, etc.
func (p *Parser) Len() int {
	if p.cur != TokenStartArray && p.cur != TokenStartHash && p.cur != TokenIVarProps {
		return -1
	}

	return p.stack.cur().sz
}

// LinkID returns the id number for the current link value, or the expected link id for a linkable value.
// Only valid for the first token of linkable values such as TokenFloat, TokenString, TokenStartHash, TokenStartArray,
// etc. Returns -1 for anything else.
func (p *Parser) LinkID() int {
	switch p.cur {
	case TokenLink:
		return p.num
	case TokenStartIVar:
		// IVar is special - we haven't insert something into lnkTbl yet, but we will be.
		return len(p.lnkTbl)
	case TokenFloat, TokenBignum, TokenString, TokenStartArray, TokenStartHash:
		return len(p.lnkTbl) - 1
	}
	return -1
}

// Int returns the value contained in the current Fixnum token.
// A fixnum will not exceed an int32, so this method returns int.
// Returns an error if called for any other type of token.
func (p *Parser) Int() (int, error) {
	if p.cur != TokenFixnum {
		return 0, errors.Errorf("Int() called on incorrect token %q", p.cur)
	}
	return p.num, nil
}

// Float returns the value contained in the current Float token.
// Converting the current context into a float is expensive, be  sure to only call this once for each distinct value.
// Returns an error if called for any other type of token.
func (p *Parser) Float() (float64, error) {
	if p.cur != TokenFloat {
		return 0, errors.Errorf("Float() called on incorrect token %q", p.cur)
	}

	// Avoid some unnecessary allocations by constructing a raw string view over the bytes. This is safe because the
	// fake string is not leaked outside of this method call - the bytes only need to stay constant for the call to
	// strconv.ParseFloat.
	buf := p.buf[p.ctx.beg:p.ctx.end]
	bytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	strHeader := reflect.StringHeader{Data: bytesHeader.Data, Len: bytesHeader.Len}
	str := *(*string)(unsafe.Pointer(&strHeader))

	flt, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse float")
	}
	return flt, nil
}

// Bignum returns the value contained in the current Bignum token.
// Converting the current context into a big.Int is expensive, be  sure to only call this once for each distinct value.
// Returns an error if called for any other type of token.
func (p *Parser) Bignum() (big.Int, error) {
	if p.cur != TokenBignum {
		return big.Int{}, errors.Errorf("Bignum() called on incorrect token %q", p.cur)
	}

	wordsz := (p.ctx.end - p.ctx.beg + _S - 1) / _S
	if len(p.bnumbits) < wordsz {
		p.bnumbits = make([]big.Word, wordsz)
	}

	k := 0
	s := uint(0)
	var d big.Word

	var i int
	for pos := p.ctx.beg; pos <= p.ctx.end; pos++ {
		d |= big.Word(p.buf[pos]) << s
		if s += 8; s == _S*8 {
			p.bnumbits[k] = d
			k++
			s = 0
			d = 0
		}
		i++
	}
	if k < wordsz {
		p.bnumbits[k] = d
	}

	var bnum big.Int
	bnum.SetBits(p.bnumbits[:wordsz])

	if p.bnumsign == '-' {
		bnum = *bnum.Neg(&bnum)
	}
	return bnum, nil
}

// Bytes copies the raw bytes for the current value into the provided buffer.
// It returns an error if the provided buffer is not large enough to fit the data.
// Returns the number of bytes written into the buffer on success.
func (p *Parser) Bytes(b []byte) (wr int, err error) {
	if len(b) < p.ctx.end-p.ctx.beg {
		err = fmt.Errorf("Buffer is too small")
		return
	}
	wr = copy(b, p.buf[p.ctx.beg:p.ctx.end])
	return
}

// UnsafeBytes returns the raw bytes for the current value.
// NOTE: this method is unsafe because the returned byte slice is a reference to an internal read buffer used by this
// Parser. The data in the slice will be invalid on the next call to Reset(). If the data needs to be kept for longer
// than that it should be copied out into a buffer owned by the caller.
func (p *Parser) UnsafeBytes() []byte {
	return p.buf[p.ctx.beg:p.ctx.end]
}

// Text returns the value contained in the current token interpreted as a string.
// Errors if the token is not one of Float, Bignum, Symbol or String
func (p *Parser) Text() (string, error) {
	switch p.cur {
	case TokenBignum:
		return string(p.bnumsign) + string(p.buf[p.ctx.beg:p.ctx.end]), nil
	case TokenFloat, TokenSymbol, TokenString:
		return string(p.buf[p.ctx.beg:p.ctx.end]), nil
	}
	return "", errors.Errorf("rmarsh.Parser.Text() called for wrong token: %s", p.cur)
}

// UnsafeText returns the value contained in the current token interpreted as a string.
// The returned string is a view over data contained in the internal read buffer used by this Parser. It will become
// invalid on the next call to Reset().
func (p *Parser) UnsafeText() (string, error) {
	switch p.cur {
	case TokenFloat, TokenSymbol, TokenString:
		buf := p.buf[p.ctx.beg:p.ctx.end]
		bytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
		strHeader := reflect.StringHeader{Data: bytesHeader.Data, Len: bytesHeader.Len}
		return *(*string)(unsafe.Pointer(&strHeader)), nil
	}
	return "", errors.Errorf("rmarsh.Parser.Text() called for wrong token: %s", p.cur)
}

// Reads the next value in the stream.
func (p *Parser) readNext() (tok Token, err error) {

	// This can be set in the token parsing below. If it is, we'll push it as a new entry into the lnkTbl at the
	// end of this method.
	var newLnkEntry rng

	switch typ {

	case typeFloat:
		start := p.pos - 1
		tok = TokenFloat

		// Float will be at least 2 more bytes - 1 for len and 1 for a digit
		if p.pos+2 > p.buflen {
			if err = p.fill(p.pos + 2 - p.buflen); err != nil {
				err = errors.Wrap(err, "error reading float")
				return
			}
		}

		if p.ctx, err = p.sizedBlob(false); err != nil {
			err = errors.Wrap(err, "error reading float")
			return
		}

		newLnkEntry = rng{start, p.pos}

	case typeBignum:
		start := p.pos - 1
		tok = TokenBignum

		// Bignum will have at least 3 more bytes - 1 for sign, 1 for len and at least 1 digit.
		if p.pos+3 > p.buflen {
			if err = p.fill(p.pos + 3 - p.buflen); err != nil {
				err = errors.Wrap(err, "error reading bignum")
				return
			}
		}

		p.bnumsign = p.buf[p.pos]
		p.pos++

		if p.ctx, err = p.sizedBlob(true); err != nil {
			err = errors.Wrap(err, "error reading bignum")
		}
		newLnkEntry = rng{start, p.pos}

	case typeSymbol:
		tok = TokenSymbol

		// Symbol will be at least 2 more bytes - 1 for len and 1 for a char.
		if p.pos+2 > p.buflen {
			if err = p.fill(p.pos + 2 - p.buflen); err != nil {
				err = errors.Wrap(err, "error reading bignum")
				return
			}
		}

		p.ctx, err = p.sizedBlob(false)
		if err != nil {
			err = errors.Wrap(err, "error reading symbol")
			return
		}

		// We only insert into the symbol table if we're the top level parser.
		if p.lnkID == -1 {
			if err = p.symTbl.add(p.ctx); err != nil {
				return
			}
		}

	case typeString:
		tok = TokenString
		start := p.pos - 1
		if p.ctx, err = p.sizedBlob(false); err != nil {
			err = errors.Wrap(err, "error reading string")
		}
		newLnkEntry = rng{start, p.pos}

	case typeSymlink:
		tok = TokenSymbol
		var n int
		n, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "error reading symlink id")
			return
		}
		if n >= len(p.symTbl) {
			err = p.parserError("parsed unexpected symlink id %d, expected no higher than %d", n, len(p.symTbl)-1)
			return
		}
		p.ctx = p.symTbl[n]

	case typeArray:
		tok = TokenStartArray
		start := p.pos - 1
		p.num, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "error reading array")
			return
		}
		newLnkEntry.beg = start

	case typeHash:
		tok = TokenStartHash
		start := p.pos - 1
		p.num, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "error reading hash")
			return
		}
		newLnkEntry.beg = start

	case typeIvar:
		tok = TokenStartIVar
		newLnkEntry.beg = p.pos - 1

	case typeLink:
		tok = TokenLink
		p.num, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "error reading link")
			return
		}

	case typeUsrMarshal:
		tok = TokenUsrMarshal
		start := p.pos - 1

		newLnkEntry.beg = start

	default:
		err = p.parserError("Unhandled type %d encountered", typ)
		return
	}

	// If a new link table entry was specified whilst parsing this token, we insert it into the link table, but only if:
	//  * We're in the top level parser (adding entries during replay parsing is nonsensical).
	//  * The last token wasn't TokenStartIVar, in this case we already inserted a link table entry for this value,
	if p.lnkID == -1 && newLnkEntry.beg > 0 && p.state != parserStateIVarInit {
		err = p.lnkTbl.add(newLnkEntry)
	}
	return
}

// Strings, Symbols, Floats, Bignums and the like all begin with an encoded long
// for the size and then the raw bytes. In most cases, interpreting those bytes
// is relatively expensive - and the caller may not even care (just skips to the
// next token). So, we save off the raw bytes and interpret them only when needed.
func (p *Parser) sizedBlob(bnum bool) (r rng, err error) {
	var sz int
	sz, err = p.long()
	if err != nil {
		return
	}

	// For some stupid reason bignums store the length in shorts, not bytes.
	if bnum {
		sz = sz * 2
	}

	r.beg = p.pos
	r.end = p.pos + sz

	if r.end > p.buflen {
		err = p.fill(r.end - p.buflen)
	}
	p.pos += sz
	return
}
