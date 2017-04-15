package rubymarshal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/pkg/errors"
)

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (dec *Decoder) Decode() (interface{}, error) {
	m, err := dec.uint16("magic")
	if err != nil {
		return nil, err
	}
	if m != 0x0408 {
		return nil, fmt.Errorf("Unexpected magic header %d", m)
	}

	return dec.val()
}

func (dec *Decoder) val() (interface{}, error) {
	typ, err := dec.byte("type class")
	if err != nil {
		return nil, err
	}
	switch typ {
	case TYPE_NIL:
		return nil, nil
	case TYPE_TRUE:
		return true, nil
	case TYPE_FALSE:
		return false, nil
	case TYPE_FIXNUM:
		return dec.num()
	case TYPE_BIGNUM:
		return dec.bignum()
		// case TYPE_ARRAY:
		// 	sz, err := dec.num()
		// 	if err != nil {
		// 		return nil, err
		// 	}
	}

	return nil, nil
}

func (dec *Decoder) num() (int64, error) {
	b, err := dec.byte("num")
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

		raw, err := dec.bytes(int64(c), "number")
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
	raw, err := dec.bytes(int64(c), "number")
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

func (dec *Decoder) bignum() (*big.Int, error) {
	sign, err := dec.byte("bignum sign")
	if err != nil {
		return nil, err
	}

	sz, err := dec.num()
	if err != nil {
		return nil, err
	}
	bytes, err := dec.bytes(sz*2, "bignum bytes")
	if err != nil {
		return nil, err
	}

	var bigint big.Int
	reverseBytes(bytes)
	bigint.SetBytes(bytes)
	if sign == '-' {
		bigint.Neg(&bigint)
	}

	return &bigint, nil
}

func (dec *Decoder) byte(op string) (byte, error) {
	b, err := dec.r.ReadByte()
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("Error while reading %s", op))
	}
	return b, nil
}

func (dec *Decoder) bytes(sz int64, op string) ([]byte, error) {
	b := make([]byte, sz)

	if _, err := io.ReadFull(dec.r, b); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error while reading %s", op))
	}
	return b, nil
}

func (dec *Decoder) uint16(op string) (uint16, error) {
	b, err := dec.bytes(2, op)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}
