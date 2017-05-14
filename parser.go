package rmarsh

import (
	"io"
	"math/big"
	"reflect"
	"strconv"
	"unsafe"

	"github.com/pkg/errors"
)

const (
	bufSize        = 64   // Initial size of our read buffer
	symTblInitSize = 32   // Initial size of symbol table
	symTblMaxInc   = 1024 // Symbol table is doubled in size whenever space is exceeded, increment is capped at this num.
	stackGrowSize  = 8    // Amount to grow stack by when needed
)

// A Token represents a single distinct value type read from a Parser instance.
type Token uint8

// The valid token types.
const (
	tokenStart = iota
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

const (
	ctxArray = iota
	ctxHash
	ctxIVar
)

type parserContext struct {
	typ uint8
	sz  int
	pos int

	ivSym *string // If current context is an IVar, then this will contain the instance variable name
}

// Parser is a low-level streaming implementation of the Ruby Marshal 4.8 format.
type Parser struct {
	r    io.Reader
	cur  Token
	pos  uint64
	st   []parserContext
	stSz int
	cst  *parserContext

	buf []byte
	ctx []byte

	num int64

	bnumbits []big.Word
	bnumsign byte

	symTbl symTable
}

// NewParser constructs a new parser that streams data from the given io.Reader
// Due to the nature of the Marshal format, data is read in very small increments
// please ensure that the provided Reader is buffered, or wrap it in a bufio.Reader.
func NewParser(r io.Reader) *Parser {
	p := &Parser{r: r, buf: make([]byte, bufSize)}
	p.symTbl.off = make([]int, 8)
	p.symTbl.sz = make([]int, 8)
	return p
}

// Reset reverts the Parser into the identity state, ready to read a new Marshal 4.8 stream from the existing Reader.
// If the provided io.Reader is nil, the existing Reader will continue to be used.
func (p *Parser) Reset(r io.Reader) {
	if r != nil {
		p.r = r
	}
	p.pos = 0
	p.cur = tokenStart
	p.stSz = 0
	p.symTbl.reset()
}

// Next advances the parser to the next token in the stream.
func (p *Parser) Next() (Token, error) {
	// If we're currently parsing an IVar, then we handle the next symbol+value pair.
	if p.cst != nil && p.cst.typ == ctxIVar {
		if p.cst.sz > 0 {
			return p.advIVar()
		} else if p.cst.sz < 0 {
			// Crappy state handling being encoded in magic numbers.
			// This situation means we only just parsed the beginning of the IVar
			// in the previous Next() call. So we need to let the actual value read
			// start. We mark the sz as 0 so that once we're back to this context
			// (after current value is parsed) we'll then read the instance variable
			// length and read all the instance vars.
			p.cst.sz = 0
		} else {
			// If we get here, it's because we finished parsing the actual value for an IVar
			// and now it's time to parse the instance variables.
			n, err := p.long()
			if err != nil {
				return tokenStart, errors.Wrap(err, "ivar")
			}
			p.cst.pos = 0
			p.cst.sz = int(n)
			return p.advIVar()
		}
	} else if p.cst != nil && p.cst.pos == p.cst.sz {
		// If we're in the middle of an array/map, check if we've finished it.
		switch p.cst.typ {
		case ctxArray:
			p.cur = TokenEndArray
		case ctxHash:
			p.cur = TokenEndHash
		}

		p.popStack()
		return p.cur, nil
	}

	if err := p.adv(); err != nil {
		return 0, errors.Wrap(err, "rmarsh.Parser.Next()")
	}

	if p.cst != nil {
		p.cst.pos++
	}

	return p.cur, nil
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

// Len returns the number of elements to be read in the current structure.
// Returns 0 if the current token is not TokenStartArray, TokenStartHash, etc.
func (p *Parser) Len() int {
	if p.cur != TokenStartArray && p.cur != TokenStartHash {
		return 0
	}

	return p.cst.sz
}

// Int returns the value contained in the current Fixnum token.
// Returns an error if called for any other type of token.
func (p *Parser) Int() (int64, error) {
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
	bytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&p.ctx))
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

	wordsz := (len(p.ctx) + _S - 1) / _S
	if len(p.bnumbits) < wordsz {
		p.bnumbits = make([]big.Word, wordsz)
	}

	k := 0
	s := uint(0)
	var d big.Word

	for i := 0; i < len(p.ctx); i++ {
		d |= big.Word(p.ctx[i]) << s
		if s += 8; s == _S*8 {
			p.bnumbits[k] = d
			k++
			s = 0
			d = 0
		}
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
	return p.ctx
}

// IVarName returns the name of the instance variable that is currently being parsed.
// Errors if not presently parsing the variables of an IVar.
func (p *Parser) IVarName() (string, error) {
	if p.cst == nil || p.cst.typ != ctxIVar {
		return "", errors.New("rmarsh.Parser.IVarName() called outside of an IVar")
	}

	return *p.cst.ivSym, nil
}

// Text returns the value contained in the current token interpreted as a string.
// Errors if the token is not one of Float, Bignum, Symbol or String
func (p *Parser) Text() (string, error) {
	switch p.cur {
	case TokenBignum:
		return string(p.bnumsign) + string(p.ctx), nil
	case TokenFloat, TokenSymbol, TokenString:
		return string(p.ctx), nil
	}
	return "", errors.Errorf("rmarsh.Parser.Text() called for wrong token: %s", p.cur)
}

func (p *Parser) adv() (err error) {
	var typ byte

	if p.cur == tokenStart {
		if b, err := p.readbytes(3); err != nil {
			return errors.Wrap(err, "reading magic")
		} else if b[0] != 0x04 || b[1] != 0x08 {
			return errors.Errorf("Expected magic header 0x0408, got 0x%.4X", int16(b[0])<<8|int16(b[1]))
		} else {
			// Silly little optimisation: we fetched 3 bytes on the first
			// read since there is always at least one token to read.
			// Saves a couple dozen nanos on them micro benchmarks. #winning #tigerblood
			typ = b[2]
		}
	} else {
		typ, err = p.readbyte()
		if err == io.EOF {
			p.cur = TokenEOF
			return nil
		} else if err != nil {
			return errors.Wrap(err, "read type id")
		}
	}

	switch typ {
	case typeNil:
		p.cur = TokenNil
	case typeTrue:
		p.cur = TokenTrue
	case typeFalse:
		p.cur = TokenFalse
	case typeFixnum:
		p.cur = TokenFixnum
		p.num, err = p.long()
		if err != nil {
			return errors.Wrap(err, "fixnum")
		}
	case typeFloat:
		p.cur = TokenFloat
		if err := p.sizedBlob(false); err != nil {
			return errors.Wrap(err, "float")
		}
	case typeBignum:
		p.cur = TokenBignum
		p.bnumsign, err = p.readbyte()
		if err != nil {
			return errors.Wrap(err, "bignum")
		}

		if err := p.sizedBlob(true); err != nil {
			return errors.Wrap(err, "bignum")
		}
	case typeSymbol:
		p.cur = TokenSymbol
		if err := p.sizedBlob(false); err != nil {
			return errors.Wrap(err, "symbol")
		}
		p.symTbl.add(p.ctx)
	case typeString:
		p.cur = TokenString
		if err := p.sizedBlob(false); err != nil {
			return errors.Wrap(err, "string")
		}
	case typeSymlink:
		p.cur = TokenSymbol
		n, err := p.long()
		if err != nil {
			return errors.Wrap(err, "symlink id")
		}
		id := int(n)
		if id >= p.symTbl.num {
			return errors.Errorf("Symlink id %d is larger than max known %d", id, p.symTbl.num-1)
		}
		p.ctx = p.symTbl.get(id)
	case typeArray:
		p.cur = TokenStartArray
		n, err := p.long()
		if err != nil {
			return errors.Wrap(err, "array")
		}
		p.pushStack(ctxArray, int(n))
	case typeHash:
		p.cur = TokenStartHash
		n, err := p.long()
		if err != nil {
			return errors.Wrap(err, "hash")
		}
		p.pushStack(ctxHash, int(n*2))
	case typeIvar:
		p.cur = TokenStartIVar
		p.pushStack(ctxIVar, -1)
	}

	return nil
}

func (p *Parser) advIVar() (Token, error) {
	if p.cst.pos == p.cst.sz {
		p.cur = TokenEndIVar
		p.popStack()
		return p.cur, nil
	}
	p.cst.pos++

	// Next thing needs to be a symbol, or things are really FUBAR.
	if err := p.adv(); err != nil {
		return p.cur, err
	} else if p.cur != TokenSymbol {
		return tokenStart, errors.Errorf("Unexpected token type %s while parsing IVar, expected Symbol", p.cur)
	}
	sym := string(p.ctx)
	p.cst.ivSym = &sym

	err := p.adv()
	return p.cur, err
}

func (p *Parser) pushStack(typ uint8, sz int) {
	// Grow stack if needed
	if l := len(p.st); p.stSz == l {
		newStack := make([]parserContext, l+stackGrowSize)
		copy(newStack, p.st)
		p.st = newStack
	}

	p.cst = &p.st[p.stSz]
	p.cst.typ = typ
	p.cst.sz = sz
	p.cst.pos = -1

	p.stSz++
}

func (p *Parser) popStack() {
	p.stSz--
	if p.stSz > 0 {
		p.cst = &p.st[p.stSz-1]
		p.cst.pos++
	} else {
		p.cst = nil
	}
}

// Strings, Symbols, Floats, Bignums and the like all begin with an encoded long
// for the size and then the raw bytes. In most cases, interpreting those bytes
// is relatively expensive - and the caller may not even care (just skips to the
// next token). So, we save off the raw bytes and interpret them only when needed.
func (p *Parser) sizedBlob(bnum bool) error {
	sz, err := p.long()
	if err != nil {
		return err
	}

	// For some stupid reason bignums store the length in shorts, not bytes.
	if bnum {
		sz = sz * 2
	}

	p.ctx, err = p.readbytes(uint64(sz))
	return err
}

func (p *Parser) long() (int64, error) {
	b, err := p.readbyte()
	if err != nil {
		return 0, err
	}

	c := int(int8(b))
	if c == 0 {
		return 0, nil
	}

	if c > 0 {
		if 4 < c && c < 128 {
			return int64(c - 5), nil
		}

		raw, err := p.readbytes(uint64(c))
		if err != nil {
			return 0, err
		}
		var x int64
		for i, v := range raw {
			x |= int64(v) << uint(8*i)
		}
		return x, nil
	}

	if -129 < c && c < -4 {
		return int64(c + 5), nil
	}

	c = -c
	raw, err := p.readbytes(uint64(c))
	if err != nil {
		return 0, err
	}
	x := int64(-1)
	for i, v := range raw {
		x &= ^(int64(0xff) << uint(8*i))
		x |= int64(v) << uint(8*i)
	}

	return x, err
}

func (p *Parser) readbyte() (byte, error) {
	buf, err := p.readbytes(1)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func (p *Parser) readbytes(num uint64) ([]byte, error) {
	if uint64(cap(p.buf)) < num {
		p.buf = make([]byte, num)
	}
	b := p.buf[:num]
	if _, err := io.ReadFull(p.r, b); err == io.EOF {
		return nil, err
	} else if err != nil {
		return nil, errors.Errorf("I/O error %q at position %d", err, p.pos)
	}
	p.pos += num
	return b, nil
}

// Stores symbols in a single dimension byte array with lookups for offset+lengths.
// Reduces amount of on-heap allocations.
type symTable struct {
	data []byte
	off  []int
	sz   []int
	num  int
	pos  int
}

func (t *symTable) get(id int) []byte {
	return t.data[t.off[id] : t.off[id]+t.sz[id]]
}

func (t *symTable) add(sym []byte) {
	if len(t.off) == t.num {
		newOff := make([]int, len(t.off)*2)
		copy(newOff, t.off)
		t.off = newOff
		newSz := make([]int, len(t.off)*2)
		copy(newSz, t.sz)
		t.sz = newSz
	}

	if t.pos+len(sym) < len(t.data) {
		incr := len(t.data)
		if incr > symTblMaxInc {
			incr = symTblMaxInc
		}
		if incr < len(sym) {
			incr = len(sym)
		}
		newData := make([]byte, len(t.data)+incr)
		copy(newData, t.data)
		t.data = newData
	}

	n := copy(t.data[t.pos:], sym)
	t.off[t.num] = t.pos
	t.sz[t.num] = n
	t.pos += n
	t.num++
}

func (t *symTable) reset() {
	t.num = 0
	t.pos = 0
}
