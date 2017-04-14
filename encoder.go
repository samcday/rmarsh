package rubymarshal

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
)

const TYPE_NIL = '0'
const TYPE_TRUE = 'T'
const TYPE_FALSE = 'F'
const TYPE_FIXNUM = 'i'

var magic = fmt.Sprintf("%c%c", 4, 8)

func Encode(val interface{}) ([]byte, error) {
	b := new(bytes.Buffer)

	if _, err := b.WriteString(magic); err != nil {
		errors.Wrap(err, "Failed to write header")
	}

	if err := encodeVal(b, val); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func encodeVal(b *bytes.Buffer, val interface{}) error {
	if val == nil {
		return encodeNil(b)
	}

	t := reflect.TypeOf(val)
	switch t.Kind() {
	case reflect.Bool:
		if err := encodeBool(b, val.(bool)); err != nil {
			return err
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if err := encodeFixnum(b, val); err != nil {
			return err
		}
	default:
		return fmt.Errorf("Don't know how to encode type %T", val)
	}

	return nil
}

func encodeNil(b *bytes.Buffer) error {
	if _, err := b.WriteRune(TYPE_NIL); err != nil {
		return errors.Wrap(err, "Failed to write nil value")
	}
	return nil
}

func encodeBool(b *bytes.Buffer, val bool) error {
	t := TYPE_FALSE
	if val == true {
		t = TYPE_TRUE
	}

	if _, err := b.WriteRune(t); err != nil {
		return errors.Wrap(err, "Failed to write bool value")
	}
	return nil
}

func encodeFixnum(b *bytes.Buffer, val interface{}) error {
	if _, err := b.WriteRune(TYPE_FIXNUM); err != nil {
		return err
	}

	if _, err := b.Write(encodeNum(val)); err != nil {
		return err
	}

	return nil
}

// Marshal encodes numbers in an interesting way.
// 0 is stored as 0. Easy.
// If -123 < x < 122 is stored as is shifted by 5 (matching sign of num). Negative nums stored in two's complement
// x > 122 is stored as byte count + big endian encoding
// x < 122 is stored as byte count (in two's complement) + big endian encoding in 2's complement
// Examples:
// 0 => 0x00
// 1 => 0x06 (5 + num)
// 10 => 0x0F
// 122 => 0x7F
// 123 => 0x01 0x7B
// 256 => 0x02 0x00 0x01
// -1 => 0xFA
// -123 => 0x80
// -124 => 0xFF 0x84
func encodeNum(val interface{}) []byte {
	switch val := val.(type) {
	case int, int8, int16, int32, int64:
		num := reflect.ValueOf(val).Int()
		if num == 0 {
			return []byte{0}
		}

		if num > 0 {
			return encodeNumPos(uint64(num))
		} else {
			return encodeNumNeg(num)
		}
	case uint, uint8, uint16, uint32, uint64:
		num := reflect.ValueOf(val).Uint()
		if num == 0 {
			return []byte{0}
		}
		return encodeNumPos(num)
	default:
		panic(fmt.Sprintf("encodeNum: called with non int/uint type %T", val))
	}

	return nil
}

func encodeNumPos(num uint64) []byte {
	if num < 123 {
		return []byte{byte(num) + 5}
	}

	if num <= 0xFF {
		return []byte{1, byte(num)}
	}

	if num <= 0xFFFF {
		return []byte{2, byte(num & 0xFF), byte(num >> 8 & 0xFF)}
	}

	if num <= 0xFFFFFF {
		return []byte{3, byte(num & 0xFF), byte(num >> 8 & 0xFF), byte(num >> 16 & 0xFF)}
	}

	if num <= 0x3FFFFFFF {
		print(num >> 24 & 0xFF)
		return []byte{4, byte(num & 0xFF), byte(num >> 8 & 0xFF), byte(num >> 16 & 0xFF), byte(num >> 24 & 0xFF)}
	}

	panic("Handling numbers larger than 0x3FFFFFFF is not supported yet")
}

func encodeNumNeg(num int64) []byte {
	if num > -124 {
		return []byte{byte(num - 5)}
	}

	if num >= -0xFF {
		return []byte{negbyte(-1), byte(num)}
	}

	if num >= -0xFFFF {
		return []byte{negbyte(-2), byte(num & 0xFF), byte(num >> 8 & 0xFF)}
	}

	if num >= -0xFFFFFF {
		return []byte{negbyte(-3), byte(num & 0xFF), byte(num >> 8 & 0xFF), byte(num >> 16 & 0xFF)}
	}

	if num >= -0x3FFFFFFF {
		return []byte{negbyte(-4), byte(num & 0xFF), byte(num >> 8 & 0xFF), byte(num >> 16 & 0xFF), byte(num >> 24 & 0xFF)}
	}

	panic("Dunno how to handle this")
}

func negbyte(num int32) byte {
	return byte(num)
}
