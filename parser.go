package rmarsh

import (
	"bufio"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

type Parser struct {
	r   *bufio.Reader
	cur Token
	pos uint64

	num int64
}

type Token uint8

const (
	tokenStart = iota
	TokenNil
	TokenTrue
	TokenFalse
	TokenFixnum
	TokenEOF
)

var tokenNames = map[Token]string{
	TokenNil:    "TokenNil",
	TokenTrue:   "TokenTrue",
	TokenFalse:  "TokenFalse",
	TokenFixnum: "TokenFixnum",
}

func (t Token) String() string {
	if n, ok := tokenNames[t]; ok {
		return n
	}
	return "UNKNOWN"
}

func NewParser(r io.Reader) *Parser {
	return &Parser{r: bufio.NewReader(r)}
}

func (p *Parser) Reset() {
	p.pos = 0
	p.cur = tokenStart
}

func (p *Parser) Next() (Token, error) {
	if err := p.adv(); err != nil {
		return 0, errors.Wrap(err, "rmarsh.Parser.Next()")
	}
	return p.cur, nil
}

// Int returns the value contained in the current Fixnum token.
// Returns an error if called for any other token.
func (p *Parser) Int() (int64, error) {
	if p.cur != TokenFixnum {
		return 0, errors.Errorf("rmarsh.Parser.Int() called for wrong token: %s", p.cur)
	}
	return p.num, nil
}

func (p *Parser) adv() error {
	if p.cur == tokenStart {
		if b, err := p.readbyte(); err != nil {
			return errors.Wrap(err, "magic header")
		} else if b != 0x04 {
			return errors.Errorf("Unexpected magic header 1st byte %X", b)
		}
		if b, err := p.readbyte(); err != nil {
			return errors.Wrap(err, "magic header")
		} else if b != 0x08 {
			return errors.Errorf("Unexpected magic header 2nd byte %X", b)
		}
	} else {
		if _, err := p.r.Peek(1); err == io.EOF {
			p.cur = TokenEOF
			return nil
		}
	}

	typ, err := p.readbyte()
	if err != nil {
		return errors.Wrap(err, "read type id")
	}

	switch typ {
	case TYPE_NIL:
		p.cur = TokenNil
	case TYPE_TRUE:
		p.cur = TokenTrue
	case TYPE_FALSE:
		p.cur = TokenFalse
	case TYPE_FIXNUM:
		p.cur = TokenFixnum
		p.num, err = p.long()
		if err != nil {
			return errors.Wrap(err, "fixnum")
		}
	}

	return nil
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
		bytes := make([]byte, 8)
		for i, v := range raw {
			bytes[i] = v
		}
		return int64(binary.LittleEndian.Uint64(bytes)), nil
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
	b, err := p.r.ReadByte()
	if err != nil {
		return 0, errors.Errorf("I/O error %q at position %d", err, p.pos)
	}
	p.pos++
	return b, nil
}

func (p *Parser) readbytes(num uint64) ([]byte, error) {
	b := make([]byte, num)
	if _, err := io.ReadFull(p.r, b); err != nil {
		return nil, errors.Errorf("I/O error %q at position %d", err, p.pos)
	}
	p.pos += num
	return b, nil
}
