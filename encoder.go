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
	rstringType  = reflect.TypeOf(RString{})

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
	if val == nil {
		return enc.nil()
	}

	v := reflect.ValueOf(val)
	isptr := v.Kind() == reflect.Ptr
	typ := v.Type()
	if isptr {
		typ = v.Elem().Type()
	}

	if typ.AssignableTo(symbolType) {
		if isptr {
			return enc.symbol(*val.(*Symbol))
		} else {
			return enc.symbol(val.(Symbol))
		}
	} else if typ.AssignableTo(classType) {
		if isptr {
			return enc.class(*val.(*Class))
		} else {
			return enc.class(val.(Class))
		}
	} else if typ.AssignableTo(moduleType) {
		if isptr {
			return enc.module(*val.(*Module))
		} else {
			return enc.module(val.(Module))
		}
	} else if typ.AssignableTo(instanceType) {
		if isptr {
			return enc.instance(val.(*Instance))
		} else {
			i := val.(Instance)
			return enc.instance(&i)
		}
	} else if typ.AssignableTo(regexpType) {
		if isptr {
			return enc.regexp(*val.(*Regexp))
		} else {
			return enc.regexp(val.(Regexp))
		}
	} else if typ.AssignableTo(rstringType) {
		if isptr {
			rstr := val.(*RString)
			return enc.string(rstr.Raw, rstr.Encoding)
		} else {
			rstr := val.(RString)
			return enc.string(rstr.Raw, rstr.Encoding)
		}
	} else if typ.AssignableTo(bigIntType) {
		if isptr {
			return enc.bignum(val.(*big.Int))
		} else {
			v := val.(big.Int)
			return enc.bignum(&v)
		}
	}

	kind := v.Kind()
	if isptr {
		kind = v.Elem().Kind()
	}

	switch kind {
	case reflect.Bool:
		if isptr {
			return enc.bool(*val.(*bool))
		} else {
			return enc.bool(val.(bool))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if isptr {
			return enc.fixnum(v.Elem())
		} else {
			return enc.fixnum(v)
		}
	case reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		if isptr {
			return enc.float(v.Elem())
		} else {
			return enc.float(v)
		}
	case reflect.Slice, reflect.Array:
		if isptr {
			return enc.slice(v.Elem())
		} else {
			return enc.slice(v)
		}
	case reflect.Map:
		if isptr {
			return enc.map_(v.Elem())
		} else {
			return enc.map_(v)
		}
	case reflect.String:
		if isptr {
			return enc.string(*val.(*string), "UTF-8")
		} else {
			return enc.string(val.(string), "UTF-8")
		}
	default:
		return fmt.Errorf("Don't know how to encode type %T", val)
	}

	return nil
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

func (enc *Encoder) slice(v reflect.Value) error {
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

	enc.ctx.symbolCache[str] = len(enc.ctx.symbolCache) + len(enc.ctx.instCache)

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

func (enc *Encoder) ivar(data func() error, vars map[Symbol]interface{}) error {
	if err := enc.typ(TYPE_IVAR); err != nil {
		return err
	}

	if err := data(); err != nil {
		return err
	}

	if err := enc.write(encodeNum(len(vars))); err != nil {
		return err
	}

	for k, v := range vars {
		if string(k) == "encoding" && reflect.TypeOf(v).Kind() == reflect.String {
			encoding := v.(string)
			// encoding instance var for UTF-8/ASCII are special cased.
			if err := enc.symbol(Symbol("E")); err != nil {
				return err
			}
			if err := enc.bool(encoding == "UTF-8"); err != nil {
				return err
			}
		} else {
			if err := enc.symbol(k); err != nil {
				return err
			}
			if err := enc.val(v); err != nil {
				return err
			}
		}
	}

	return nil
}

func (enc *Encoder) string(str string, encoding string) error {
	return enc.ivar(func() error {
		if err := enc.typ(TYPE_STRING); err != nil {
			return err
		}
		return enc.rawstr(str)
	}, map[Symbol]interface{}{
		Symbol("encoding"): encoding,
	})
}

func (enc *Encoder) regexp(r Regexp) error {
	encoding := r.Encoding
	if encoding == "" {
		encoding = "UTF-8"
	}

	return enc.ivar(func() error {
		if err := enc.typ(TYPE_REGEXP); err != nil {
			return err
		}
		if err := enc.rawstr(r.Expr); err != nil {
			return err
		}
		return enc.write([]byte{r.Flags})
	}, map[Symbol]interface{}{
		Symbol("encoding"): encoding,
	})
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

func (enc *Encoder) map_(v reflect.Value) error {
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
	if id, found := enc.ctx.instCache[i]; found {
		return enc.link(id)
	}

	// Instances with user marshalling are encoded differently.
	if i.UserMarshalled {
		if err := enc.typ(TYPE_USRMARSHAL); err != nil {
			return err
		}
		if err := enc.symbol(Symbol(i.Name)); err != nil {
			return err
		}

		// Need to insert the correct ID into the cache, after class name symbol but before ivars.
		enc.ctx.instCache[i] = len(enc.ctx.symbolCache) + len(enc.ctx.instCache)

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
