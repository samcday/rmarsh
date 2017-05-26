package rmarsh

import (
	"fmt"
	"io"
	"math/big"
	"reflect"
	"runtime"
	"strconv"
	"unsafe"

	"github.com/pkg/errors"
)

const (
	bufInitSz    = 256 // Initial size of our read buffer. We double it each time we overflow available space.
	rngTblInitSz = 8   // Initial size of range table entries
	stackInitSz  = 8   // Initial size of stack
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
	TokenEOF:        "EOF",
}

func (t Token) String() string {
	if n, ok := tokenNames[t]; ok {
		return n
	}
	return "UNKNOWN"
}

// Parser is a low-level streaming implementation of the Ruby Marshal 4.8 format.
type Parser struct {
	r io.Reader // Where we are pulling the Marshal stream bytes from

	cur Token // The token we have most recently parsed

	st     parserState
	stack  parserStack
	lnkID  int // id of the linked object this Parser is replaying
	parent *Parser

	buf []byte // The read buffer contains every bit of data that we've read for the stream.
	end int    // Where we've read up to the buffer
	pos int    // Our read position in the read buffer
	ctx rng    // Range of the raw data for the current token

	num      int
	bnumbits []big.Word
	bnumsign byte

	symTbl rngTbl // Store ranges marking the symbols we've parsed in the read buffer.
	lnkTbl rngTbl // Store ranges marking the linkable objects we've parsed in the read buffer.
}

// A range encodes a pair of start/end positions, used to mark interesting locations in the read buffer.
type rng struct{ beg, end int }

// Range table
type rngTbl []rng

func (t *rngTbl) add(r rng) (id int) {
	// We track the current parse sym table by slicing the underlying array.
	// That is, if we've seen one symbol in the stream so far, len(p.symTbl) == 1 && cap(p.symTable) == rngTblInitSz
	// Once we exceed cap, we double size of the tbl.
	id = len(*t)
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

// parserCtx tracks the current state we're processing when handling complex values like arrays, hashes, ivars,  etc.
// Multiple contexts can be nested in a stack. For example if we're parsing a hash as the nth element of an array,
// then the top of the stack will be ctxHash and the stack item below that will be ctxArray
type parserCtx struct {
	typ  uint8
	sz   int
	pos  int
	r    *rng        // when this context is finished, r (pointing into lnkTbl) is updated with final location
	next parserState // Next state transition when we're done with this stack item
}

// The valid context types
const (
	ctxArray = iota
	ctxHash
	ctxIVar
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

// NewParser constructs a new parser that streams data from the given io.Reader
// Due to the nature of the Marshal format, data is read in very small increments. Please ensure that the provided
// Reader is buffered, or wrap it in a bufio.Reader.
func NewParser(r io.Reader) *Parser {
	p := &Parser{
		r:     r,
		buf:   make([]byte, bufInitSz),
		st:    parserStateTopLevel,
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
		st:     parserStateTopLevel,
		r:      nil,
		buf:    p.buf,
		pos:    rng.beg,
		end:    rng.end,
		lnkTbl: p.lnkTbl,
		symTbl: p.symTbl,
	}, nil
}

// Reset reverts the Parser into the identity state, ready to read a new Marshal 4.8 stream from the existing Reader.
// If the provided io.Reader is nil, the existing Reader will continue to be used.
func (p *Parser) Reset(r io.Reader) {
	p.stack = p.stack[0:0]
	p.cur = tokenInvalid
	p.st = parserStateTopLevel

	// If this a replay Parser, our reset is a little less ... reset-y.
	if p.lnkID > -1 {
		p.pos = p.lnkTbl[p.lnkID].beg
		p.stack = p.stack[0:0]
		return
	}

	if r != nil {
		p.r = r
	}
	p.pos = 0
	p.end = 0
	p.symTbl = p.symTbl[0:0]
	p.lnkTbl = p.lnkTbl[0:0]
}

// Next advances the parser to the next token in the stream.
func (p *Parser) Next() (tok Token, err error) {
	// A couple of state transitions don't yield a token. We pump the transitions until we get one.
	for tok == tokenInvalid {
		if 1 == 0 {
			fmt.Println(runtime.FuncForPC(reflect.ValueOf(p.st).Pointer()).Name())
		}

		tok, p.st, err = p.st(p)
		if err != nil {
			return
		}
	}

	p.cur = tok
	return
}

// ExpectNext is a convenience method that calls Next() and ensures the next token is the one provided.
func (p *Parser) ExpectNext(exp Token) error {
	tok, err := p.Next()
	if err != nil {
		return err
	}
	if tok != exp {
		return errors.Errorf("rmarsh.Parser.ExpectNext(): read token %s, expected %s", tok, exp)
	}
	return nil
}

// Skip examines the current token, and will continuously read tokens until the current
// object is fully consumed. Does nothing for single token types like Fixnum, Bool, Nil, Bignum,
// String, Symbol, etc.
func (p *Parser) Skip() (err error) {
	var depth int
	switch p.cur {
	case TokenStartArray, TokenStartHash, TokenStartIVar, TokenIVarProps:
		depth++
	}

	var tok Token
	for depth > 0 {
		tok, err = p.Next()
		if err != nil {
			return
		}

		switch tok {
		case TokenStartArray, TokenStartHash, TokenStartIVar:
			depth++
		case TokenEndArray, TokenEndHash, TokenEndIVar:
			depth--
		}
	}
	return nil
}

// Len returns the number of elements to be read in the current structure.
// Returns -1 if the current token is not TokenStartArray, TokenStartHash, etc.
func (p *Parser) Len() int {
	if p.cur != TokenStartArray && p.cur != TokenStartHash && p.cur != TokenIVarProps {
		return -1
	}

	return p.stack.cur().sz
}

// LinkId returns the id number for the current link value, or the expected link id for a linkable value.
// Only valid for the first token of linkable values such as TokenFloat, TokenString, TokenStartHash, TokenStartArray,
// etc. Returns -1 for anything else.
func (p *Parser) LinkId() int {
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
		return 0, errors.Errorf("rmarsh.Parser.Int() called for wrong token: %s", p.cur)
	}
	return p.num, nil
}

// Float returns the value contained in the current Float token.
// Converting the current context into a float is expensive, be  sure to only call this once for each distinct value.
// Returns an error if called for any other type of token.
func (p *Parser) Float() (float64, error) {
	if p.cur != TokenFloat {
		return 0, errors.Errorf("rmarsh.Parser.Float() called for wrong token: %s", p.cur)
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
		return 0, errors.Wrap(err, "rmarsh.Parser.Float()")
	}
	return flt, nil
}

// Bignum returns the value contained in the current Bignum token.
// Converting the current context into a big.Int is expensive, be  sure to only call this once for each distinct value.
// Returns an error if called for any other type of token.
func (p *Parser) Bignum() (big.Int, error) {
	if p.cur != TokenBignum {
		return big.Int{}, errors.Errorf("rmarsh.Parser.Bignum() called for wrong token: %s", p.cur)
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

// Bytes returns the raw bytes for the current token.
// NOTE: The return byte slice is the one that is used internally, it will be modified on the next call to Next().
// If any data needs to be kept, be sure to copy it out of the returned buffer.
func (p *Parser) Bytes() []byte {
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

// Reads the next value in the stream.
func (p *Parser) readNext() (tok Token, err error) {
	if p.pos == p.end {
		err = p.fill(1)
		// TODO: should only transition to EOF if we were actually expecting it yo.
		if err != nil {
			err = errors.Wrap(err, "read type id")
			return
		}
	}

	typ := p.buf[p.pos]
	p.pos++

	fn := typeParsers[typ]
	if fn == nil {
		err = errors.Errorf("Unhandled type %d encountered", typ)
		return
	}

	return fn(p)
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

	if r.end > p.end {
		err = p.fill(r.end - p.end)
	}
	p.pos += sz
	return
}

func (p *Parser) long() (n int, err error) {
	if p.pos == p.end {
		err = p.fill(1)
		if err != nil {
			err = errors.Wrap(err, "long")
			return
		}
	}

	n = int(int8(p.buf[p.pos]))
	p.pos++
	if n == 0 {
		return
	} else if 4 < n && n < 128 {
		n = n - 5
		return
	} else if -129 < n && n < -4 {
		n = n + 5
		return
	}

	// It's a multibyte positive/negative num.
	var sz int
	if n > 0 {
		sz = n
		n = 0
	} else {
		sz = -n
		n = -1
	}

	if p.pos+sz > p.end {
		err = p.fill(p.pos + sz - p.end)
		if err != nil {
			return
		}
	}

	for i := 0; i < sz; i++ {
		if n < 0 {
			n &= ^(0xff << uint(8*i))
		}

		n |= int(p.buf[p.pos]) << uint(8*i)
		p.pos++
	}

	return
}

// pull bytes from the io.Reader into our read buffer
func (p *Parser) fill(num int) (err error) {
	// We don't do actual reads in sub Parser, the data is already in the buffer.
	if p.lnkID > -1 {
		return nil
	}

	// Optimisation: if our current stack gives us confidence there *must* be more data to read
	// (i.e we're in an array/hash/ivar and processing anything but the last item)
	// then we add an extra byte to what we read now. This avoids extra read calls for the
	// subsequent type byte.
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].sz > 0 && p.stack[i].pos < p.stack[i].sz-1 {
			num++
		}
	}

	from, to := p.end, p.end+num
	p.end += num

	if to > len(p.buf) {
		// Overflowed our read buffer, allocate a new one double the size,
		buf := make([]byte, len(p.buf)*2)
		copy(buf, p.buf)
		p.buf = buf
	}

	var rd, n int
	for rd < num && err == nil {
		n, err = p.r.Read(p.buf[from:to])
		rd += n
		from += n
	}
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	} else if err != nil {
		err = errors.Wrap(err, "fill")
	}
	return
}

type typeParserFn func(*Parser) (Token, error)

func staticParser(tok Token) typeParserFn {
	return func(*Parser) (Token, error) {
		return tok, nil
	}
}

var typeParsers = []typeParserFn{
	typeNil:   staticParser(TokenNil),
	typeTrue:  staticParser(TokenTrue),
	typeFalse: staticParser(TokenFalse),
	typeFixnum: func(p *Parser) (tok Token, err error) {
		tok = TokenFixnum
		p.num, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "fixnum")
		}
		return
	},
	typeFloat: func(p *Parser) (tok Token, err error) {
		start := p.pos - 1
		tok = TokenFloat

		// Float will be at least 2 more bytes - 1 for len and 1 for a digit
		if err = p.fill(2); err != nil {
			err = errors.Wrap(err, "float")
			return
		}

		if p.ctx, err = p.sizedBlob(false); err != nil {
			err = errors.Wrap(err, "float")
			return
		}

		// We only insert into the link table if we're the top level parser.
		if p.lnkID == -1 {
			p.lnkTbl.add(rng{start, p.pos})
		}
		return
	},
	typeBignum: func(p *Parser) (tok Token, err error) {
		start := p.pos - 1
		tok = TokenBignum

		// Bignum will have at least 3 more bytes - 1 for sign, 1 for len and at least 1 digit.
		if err = p.fill(3); err != nil {
			err = errors.Wrap(err, "bignum")
			return
		}

		p.bnumsign = p.buf[p.pos]
		p.pos++

		if p.ctx, err = p.sizedBlob(true); err != nil {
			err = errors.Wrap(err, "bignum")
		}

		// We only insert into the link table if we're the top level parser.
		if p.lnkID == -1 {
			p.lnkTbl.add(rng{start, p.pos})
		}

		return
	},
	typeSymbol: func(p *Parser) (tok Token, err error) {
		tok = TokenSymbol

		// Symbol will be at least 2 more bytes - 1 for len and 1 for a char.
		if err = p.fill(2); err != nil {
			err = errors.Wrap(err, "symbol")
			return
		}

		p.ctx, err = p.sizedBlob(false)
		if err != nil {
			err = errors.Wrap(err, "symbol")
			return
		}

		// We only insert into the symbol table if we're the top level parser.
		if p.lnkID == -1 {
			p.symTbl.add(p.ctx)
		}
		return
	},
	typeString: func(p *Parser) (tok Token, err error) {
		tok = TokenString
		start := p.pos - 1
		if p.ctx, err = p.sizedBlob(false); err != nil {
			err = errors.Wrap(err, "string")
		}
		// We only insert into the link table if we're the top level parser.
		if p.lnkID == -1 {
			p.lnkTbl.add(rng{start, p.pos})
		}
		return
	},
	typeSymlink: func(p *Parser) (tok Token, err error) {
		tok = TokenSymbol
		var n int
		n, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "symlink id")
			return
		}
		if n >= len(p.symTbl) {
			err = errors.Errorf("Symlink id %d is larger than max known %d", n, len(p.symTbl)-1)
			return
		}
		p.ctx = p.symTbl[n]
		return
	},
	typeArray: func(p *Parser) (tok Token, err error) {
		tok = TokenStartArray
		start := p.pos - 1
		p.num, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "array")
			return
		}
		// We only insert into the link table if we're the top level parser.
		if p.lnkID == -1 {
			p.lnkTbl.add(rng{start, 0})
		}
		return
	},
	typeHash: func(p *Parser) (tok Token, err error) {
		tok = TokenStartHash
		start := p.pos - 1
		p.num, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "hash")
			return
		}
		// We only insert into the link table if we're the top level parser.
		if p.lnkID == -1 {
			p.lnkTbl.add(rng{start, 0})
		}
		return
	},
	typeIvar: func(p *Parser) (tok Token, err error) {
		tok = TokenStartIVar
		return
	},
	typeLink: func(p *Parser) (tok Token, err error) {
		tok = TokenLink
		p.num, err = p.long()
		if err != nil {
			err = errors.Wrap(err, "link")
		}
		return
	},
}

type parserState func(*Parser) (Token, parserState, error)

// Our state is woven through potentially many nested levels of context.
// If we start a new context for an array/hash/ivar/whatever, we point its terminal
// state at our next one. For example if the top level value was a single depth array,
// once the array had finished parsing it would know to transition to parserStateEOF.
// this method handles pushing new stack items when needed
func nextState(p *Parser, tok Token, next parserState) parserState {
	switch tok {
	case TokenStartArray:
		ctx := p.stack.push(ctxArray, p.num, next)
		// Make sure to attach the range we added to the lnktbl in readNext()
		ctx.r = &p.lnkTbl[len(p.lnkTbl)-1]
		if p.num == 0 {
			return parserStateArrayEnd
		} else {
			return parserStateArray
		}
	case TokenStartHash:
		ctx := p.stack.push(ctxHash, p.num, next)
		// Make sure to attach the range we added to the lnktbl in readNext()
		ctx.r = &p.lnkTbl[len(p.lnkTbl)-1]
		if p.num == 0 {
			return parserStateHashEnd
		} else {
			return parserStateHashKey
		}
	case TokenStartIVar:
		p.stack.push(ctxIVar, 0, next)
		return parserStateIVarInit
	}
	return next
}

// the initial state of a Parser expects to read 2-byte magic and then a top level value
func parserStateTopLevel(p *Parser) (tok Token, next parserState, err error) {
	if p.pos == 0 {
		if err = p.fill(3); err != nil {
			return
		}
		if p.buf[p.pos] != 0x04 || p.buf[p.pos+1] != 0x08 {
			err = errors.Errorf("Expected magic header 0x0408, got 0x%.4X", int16(p.buf[p.pos])<<8|int16(p.buf[p.pos+1]))
			return
		}
		p.pos = 2
	}

	tok, err = p.readNext()
	if err != nil {
		return
	}

	// We never expect to actually read an io.EOF because we should always be transitioning
	// to parserStateEOF when we've finished parsing the top level value.
	next = nextState(p, tok, parserStateEOF)
	return
}

// state when reading elements of an array
func parserStateArray(p *Parser) (tok Token, next parserState, err error) {
	// sanity check the top of stack is an array.
	cur := p.stack.cur()
	if cur.typ != ctxArray {
		err = errors.Errorf("expected top of stack to be array, got %d", cur.typ)
		return
	}

	tok, err = p.readNext()
	if err != nil {
		return
	}

	cur.pos++
	if cur.pos == cur.sz {
		next = nextState(p, tok, parserStateArrayEnd)
	} else {
		next = nextState(p, tok, parserStateArray)
	}
	return
}

func parserStateArrayEnd(p *Parser) (tok Token, next parserState, err error) {
	cur := p.stack.cur()
	if cur.typ != ctxArray {
		err = errors.Errorf("expected top of stack to be array, got %d", cur.typ)
		return
	}

	tok = TokenEndArray
	if cur.r != nil {
		cur.r.end = p.pos
	}
	next = p.stack.pop()
	return
}

// state when reading a key in a hash
func parserStateHashKey(p *Parser) (tok Token, next parserState, err error) {
	// sanity check the top of stack is an hash.
	cur := p.stack.cur()
	if cur.typ != ctxHash {
		err = errors.Errorf("expected top of stack to be hash, got %d", cur.typ)
		return
	}

	tok, err = p.readNext()
	if err != nil {
		return
	}
	next = nextState(p, tok, parserStateHashValue)
	return
}

// state when reading a value in a hash
func parserStateHashValue(p *Parser) (tok Token, next parserState, err error) {
	// sanity check the top of stack is an hash.
	cur := p.stack.cur()
	if cur.typ != ctxHash {
		err = errors.Errorf("expected top of stack to be hash, got %d", cur.typ)
		return
	}

	tok, err = p.readNext()
	if err != nil {
		return
	}

	cur.pos++
	if cur.pos == cur.sz {
		next = nextState(p, tok, parserStateHashEnd)
	} else {
		next = nextState(p, tok, parserStateHashKey)
	}
	return
}

func parserStateHashEnd(p *Parser) (tok Token, next parserState, err error) {
	cur := p.stack.cur()
	if cur.typ != ctxHash {
		err = errors.Errorf("expected top of stack to be hash, got %d", cur.typ)
		return
	}

	// Hash is done, return an end hash token and pop stack.
	tok = TokenEndHash
	if cur.r != nil {
		cur.r.end = p.pos
	}
	next = p.stack.pop()
	return
}

// initial state of an ivar context - expects to read a value, then transitions to
// parserStateIVarLength
func parserStateIVarInit(p *Parser) (tok Token, next parserState, err error) {
	cur := p.stack.cur()
	if cur.typ != ctxIVar {
		err = errors.Errorf("expected top of stack to be ivar, got %d", cur.typ)
		return
	}

	tok, err = p.readNext()
	if err != nil {
		return
	}

	next = nextState(p, tok, parserStateIVarLen)

	// If the next state is a nested object (array, hash, etc) we nuke the saved range it has and put it on the ivar
	// instead. This is so that an IVar'd hash/array/whatever will have a replay range that includes this IVar.
	p.stack.cur().r = nil

	// The lnk table item that just got saved needs to have its end scrubbed and it's beginning moved backwards by 1
	// to ensure the replay includes this whole IVar.
	r := &p.lnkTbl[len(p.lnkTbl)-1]
	r.beg--
	r.end = 0
	cur.r = r

	return
}

func parserStateIVarLen(p *Parser) (tok Token, next parserState, err error) {
	cur := p.stack.cur()
	if cur.typ != ctxIVar {
		err = errors.Errorf("expected top of stack to be ivar, got %d", cur.typ)
		return
	}

	cur.sz, err = p.long()
	if err != nil {
		return
	}

	tok = TokenIVarProps

	if cur.sz == 0 {
		next = parserStateIVarEnd
	}
	next = parserStateIVarKey
	return
}

func parserStateIVarKey(p *Parser) (tok Token, next parserState, err error) {
	cur := p.stack.cur()
	if cur.typ != ctxIVar {
		err = errors.Errorf("expected top of stack to be ivar, got %d", cur.typ)
		return
	}

	tok, err = p.readNext()
	if err != nil {
		return
	} else if tok != TokenSymbol {
		// IVar keys are only permitted to be symbols
		err = errors.Errorf("unexpected token %s - expected Symbol for IVar key", tok)
		return
	}

	next = parserStateIVarValue
	return
}

func parserStateIVarValue(p *Parser) (tok Token, next parserState, err error) {
	cur := p.stack.cur()
	if cur.typ != ctxIVar {
		err = errors.Errorf("expected top of stack to be ivar, got %d", cur.typ)
		return
	}

	tok, err = p.readNext()
	if err != nil {
		return
	}
	cur.pos++

	if cur.pos == cur.sz {
		next = nextState(p, tok, parserStateIVarEnd)
	} else {
		next = nextState(p, tok, parserStateIVarKey)
	}

	return
}

func parserStateIVarEnd(p *Parser) (tok Token, next parserState, err error) {
	cur := p.stack.cur()
	if cur.typ != ctxIVar {
		err = errors.Errorf("expected top of stack to be ivar, got %d", cur.typ)
		return
	}

	if cur.r != nil {
		cur.r.end = p.pos
	}
	tok = TokenEndIVar
	next = p.stack.pop()
	return
}

// our EOF state - once we're here we continue to transition to the same state
// and return the same token until the Parser is reset to initial state.
func parserStateEOF(*Parser) (tok Token, next parserState, err error) {
	tok = TokenEOF
	next = parserStateEOF
	return
}
