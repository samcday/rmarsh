package rubymarshal

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"unicode/utf8"

	"github.com/pkg/errors"
)

const (
	encodeNumMin = -0x3FFFFFFF
	encodeNumMax = +0x3FFFFFFF
)

var (
	magic = []byte{4, 8}

	symbolType   = reflect.TypeOf(Symbol(""))
	classType    = reflect.TypeOf(Class(""))
	moduleType   = reflect.TypeOf(Module(""))
	instanceType = reflect.TypeOf(Instance{})
	regexpType   = reflect.TypeOf(Regexp{})
	ivarType     = reflect.TypeOf(IVar{})

	bigIntType = reflect.TypeOf(*big.NewInt(0))
)

// Encoder takes arbitrary Golang objects and writes them to a io.Writer in Ruby Marshal format.
type Encoder struct {
	w   io.Writer
	ctx encodingCtx
}

type encodingCtx struct {
	symbolCache map[string]int
	instCache   map[*Instance]int
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode will create an Encoder, write the given value, and return the encoded byte array.
func Encode(val interface{}) ([]byte, error) {
	b := new(bytes.Buffer)
	enc := NewEncoder(b)

	if err := enc.Encode(val); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (enc *Encoder) Encode(val interface{}) error {
	// Setup a new encoding context for this encode run.
	enc.ctx = encodingCtx{
		symbolCache: make(map[string]int),
		instCache:   make(map[*Instance]int),
	}

	if _, err := enc.w.Write([]byte(magic)); err != nil {
		errors.Wrap(err, "Failed to write Marshal 4.8 header")
	}

	if err := enc.val(val); err != nil {
		return errors.Wrap(err, "Error while encoding to Ruby Marshal 4.8")
	}

	return nil
}

func (enc *Encoder) val(val interface{}) error {
	return enc.val2(val, true)
}

func (enc *Encoder) val2(val interface{}, strwrap bool) error {
	if val == nil {
		return enc.nil()
	}

	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	kind := v.Kind()
	switch kind {
	case reflect.Slice, reflect.Array:
		return enc.array(v)
	case reflect.Map:
		return enc.hash(v)
	}

	switch data := ptrto(val).(type) {
	// Ruby wrapper types
	case *Symbol:
		return enc.symbol(*data)
	case *Class:
		return enc.class(*data)
	case *Module:
		return enc.module(*data)
	case *Instance:
		return enc.instance(data)
	case *Regexp:
		return enc.regexp(*data)
	case *IVar:
		return enc.ivar(data)
	case *rawString:
		return enc.string(string(*data))

	// Core types
	case *big.Int:
		return enc.bignum(data)
	case *bool:
		return enc.bool(*data)
	case *uint, *uint8, *uint16, *uint32, *uint64, *int, *int8, *int16, *int32, *int64:
		return enc.fixnum(v)
	case *float32, *float64, *complex64, *complex128:
		return enc.float(v)
	case *string:
		if strwrap {
			raw := rawString(*data)
			ivar := NewEncodingIVar(&raw, "UTF-8")
			return enc.ivar(ivar)
		}
		return enc.string(*data)
	}

	return fmt.Errorf("Don't know how to encode type %T", val)
}

func (enc *Encoder) write(b []byte) error {
	_, err := enc.w.Write(b)
	return err
}

func (enc *Encoder) typ(typ byte) error {
	return enc.write([]byte{typ})
}

func (enc *Encoder) nil() error {
	return enc.typ(TYPE_NIL)
}

func (enc *Encoder) bool(val bool) error {
	if val == true {
		return enc.typ(TYPE_TRUE)
	}

	return enc.typ(TYPE_FALSE)
}

func (enc *Encoder) fixnum(v reflect.Value) error {
	ival := v.Int()
	if ival < encodeNumMin || ival > encodeNumMax {
		return enc.bignum(big.NewInt(ival))
	}

	if err := enc.typ(TYPE_FIXNUM); err != nil {
		return err
	}

	return enc.write(encodeNum(v.Interface()))
}

func (enc *Encoder) bignum(num *big.Int) error {
	if err := enc.typ(TYPE_BIGNUM); err != nil {
		return err
	}

	if num.Sign() > 0 {
		if err := enc.write([]byte{'+'}); err != nil {
			return err
		}
	} else {
		if err := enc.write([]byte{'-'}); err != nil {
			return err
		}
	}

	b := num.Bytes()
	sz := int64(math.Ceil(float64(len(b)) / 2))
	if sz > encodeNumMax {
		return fmt.Errorf("Received a number so large that I can't even fit it into a Ruby bignum. Congrats, I think you just unlocked some kind of achievement.")
	}
	reverseBytes(b)
	if err := enc.write(encodeNum(sz)); err != nil {
		return err
	}
	return enc.write(b)
}

func (enc *Encoder) float(v reflect.Value) error {
	if err := enc.typ(TYPE_FLOAT); err != nil {
		return err
	}
	return enc.rawstr(fmt.Sprintf("%v", v.Interface()))
}

func (enc *Encoder) array(v reflect.Value) error {
	if err := enc.typ(TYPE_ARRAY); err != nil {
		return err
	}

	len := v.Len()
	if err := enc.write(encodeNum(len)); err != nil {
		return nil
	}

	for i := 0; i < len; i++ {
		if err := enc.val(v.Index(i).Interface()); err != nil {
			return err
		}
	}

	return nil
}

func (enc *Encoder) symbol(val Symbol) error {
	str := string(val)
	if !utf8.ValidString(str) {
		return fmt.Errorf("Symbol %s is not valid UTF-8", str)
	}

	if id, found := enc.ctx.symbolCache[str]; found {
		return enc.symlink(id)
	}

	if err := enc.typ(TYPE_SYMBOL); err != nil {
		return err
	}

	if err := enc.write(encodeNum(len(str))); err != nil {
		return err
	}

	enc.ctx.symbolCache[str] = len(enc.ctx.symbolCache)

	return enc.write([]byte(str))
}

func (enc *Encoder) symlink(id int) error {
	if err := enc.typ(TYPE_SYMLINK); err != nil {
		return err
	}
	return enc.write(encodeNum(id))
}

func (enc *Encoder) link(id int) error {
	if err := enc.typ(TYPE_LINK); err != nil {
		return err
	}
	return enc.write(encodeNum(id))
}

func (enc *Encoder) ivar(ivar *IVar) error {
	if err := enc.typ(TYPE_IVAR); err != nil {
		return err
	}

	if err := enc.val2(ivar.Data, false); err != nil {
		return err
	}

	if err := enc.write(encodeNum(len(ivar.Variables))); err != nil {
		return err
	}

	for k, v := range ivar.Variables {
		if string(k) == "encoding" {
			refl := reflect.ValueOf(v)
			if refl.Kind() == reflect.String {
				encoding := refl.String()
				// encoding instance var for UTF-8/ASCII are special cased.
				if encoding == "UTF-8" || encoding == "US-ASCII" {
					if err := enc.symbol(Symbol("E")); err != nil {
						return err
					}
					if err := enc.bool(encoding == "UTF-8"); err != nil {
						return err
					}
					continue
				}
			}
		}
		if err := enc.symbol(Symbol(k)); err != nil {
			return err
		}
		if err := enc.val(v); err != nil {
			return err
		}
	}

	return nil
}

func (enc *Encoder) string(str string) error {
	if err := enc.typ(TYPE_STRING); err != nil {
		return err
	}
	return enc.rawstr(str)
}

func (enc *Encoder) regexp(r Regexp) error {
	if err := enc.typ(TYPE_REGEXP); err != nil {
		return err
	}
	if err := enc.rawstr(r.Expr); err != nil {
		return err
	}
	return enc.write([]byte{r.Flags})
}

func (enc *Encoder) rawstr(str string) error {
	if err := enc.write(encodeNum(len(str))); err != nil {
		return err
	}
	return enc.write([]byte(str))
}

func (enc *Encoder) class(c Class) error {
	if err := enc.typ(TYPE_CLASS); err != nil {
		return err
	}
	return enc.rawstr(string(c))
}

func (enc *Encoder) module(m Module) error {
	if err := enc.typ(TYPE_MODULE); err != nil {
		return err
	}
	return enc.rawstr(string(m))
}

func (enc *Encoder) hash(v reflect.Value) error {
	if err := enc.typ(TYPE_HASH); err != nil {
		return err
	}

	keys := v.MapKeys()
	if err := enc.write(encodeNum(len(keys))); err != nil {
		return err
	}
	for _, k := range keys {
		if err := enc.val(k.Interface()); err != nil {
			return err
		}
		if err := enc.val(v.MapIndex(k).Interface()); err != nil {
			return err
		}
	}
	return nil
}

func (enc *Encoder) instance(i *Instance) error {
	// if id, found := enc.ctx.instCache[i]; found {
	// 	return enc.link(id)
	// }

	// Instances with user marshalling are encoded differently.
	if i.UserMarshalled {
		if err := enc.typ(TYPE_USRMARSHAL); err != nil {
			return err
		}
		if err := enc.symbol(Symbol(i.Name)); err != nil {
			return err
		}

		// Need to insert the correct ID into the cache, after class name symbol but before ivars.
		enc.ctx.instCache[i] = len(enc.ctx.instCache)

		if err := enc.val(i.Data); err != nil {
			return err
		}
	} else {
		if err := enc.typ(TYPE_OBJECT); err != nil {
			return err
		}
		if err := enc.symbol(Symbol(i.Name)); err != nil {
			return err
		}

		// Need to insert the correct ID into the cache, after class name symbol but before ivars.
		enc.ctx.instCache[i] = len(enc.ctx.symbolCache) + len(enc.ctx.instCache)

		if err := enc.write(encodeNum(len(i.InstanceVars))); err != nil {
			return err
		}
		for k, v := range i.InstanceVars {
			if err := enc.symbol(Symbol(k)); err != nil {
				return err
			}
			if err := enc.val(v); err != nil {
				return err
			}
		}
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

	if num <= encodeNumMax {
		return []byte{4, byte(num & 0xFF), byte(num >> 8 & 0xFF), byte(num >> 16 & 0xFF), byte(num >> 24 & 0xFF)}
	}

	panic(fmt.Sprintf("Cannot encode num %v - value too large", num))
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

	if num >= encodeNumMin {
		return []byte{negbyte(-4), byte(num & 0xFF), byte(num >> 8 & 0xFF), byte(num >> 16 & 0xFF), byte(num >> 24 & 0xFF)}
	}

	panic(fmt.Sprintf("Cannot encode num %v - value too small", num))
}

func negbyte(num int32) byte {
	return byte(num)
}
