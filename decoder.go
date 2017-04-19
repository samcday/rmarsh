package rmarsh

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"reflect"

	"github.com/pkg/errors"
)

type Decoder struct {
	r        *bufio.Reader
	off      int64
	symCache map[int]*Symbol
	objCache map[int]interface{}
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (dec *Decoder) Decode(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("Invalid decode target %T", v)
	}

	dec.symCache = make(map[int]*Symbol)
	dec.objCache = make(map[int]interface{})
	dec.off = 0

	m, err := dec.uint16("magic")
	if err != nil {
		return err
	}
	if m != 0x0408 {
		return dec.error(fmt.Sprintf("Unexpected magic header %d", m))
	}

	return dec.val(indirect(rv.Elem()))
}

func (dec *Decoder) val(v reflect.Value) error {
	typ, err := dec.byte("type class")
	if err != nil {
		return err
	}
	switch typ {
	case TYPE_NIL:
		switch v.Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice:
			v.Set(reflect.Zero(v.Type()))
		}
		return nil
	case TYPE_TRUE:
		return dec.bool(v, true)
	case TYPE_FALSE:
		return dec.bool(v, false)
	case TYPE_FIXNUM:
		return dec.fixnum(v)
	case TYPE_BIGNUM:
		return dec.bignum(v)
	case TYPE_SYMBOL:
		return dec.symbol(v)
	case TYPE_ARRAY:
		return dec.array(v)
	// case TYPE_HASH:
	// 	return dec.hash()
	// case TYPE_SYMLINK:
	// 	return dec.symlink()
	// case TYPE_MODULE:
	// 	return dec.module()
	// case TYPE_CLASS:
	// 	return dec.class()
	// case TYPE_IVAR:
	// 	return dec.ivar()
	// case TYPE_STRING:
	// 	return dec.rawstr()
	// case TYPE_USRMARSHAL, TYPE_OBJECT:
	// 	return dec.instance(typ == TYPE_USRMARSHAL)
	// case TYPE_LINK:
	// 	return dec.link()
	default:
		return dec.error(fmt.Sprintf("Unknown type %X", typ))
	}
}

func (dec *Decoder) bool(v reflect.Value, b bool) error {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(b)
		return nil
	case reflect.Interface:
		if v.NumMethod() > 0 {
			return InvalidTypeError{ExpectedType: "bool", ActualType: v.Type(), Offset: dec.off}
		}
		v.Set(reflect.ValueOf(b))
		return nil
	}
	return InvalidTypeError{ExpectedType: "bool", ActualType: v.Type(), Offset: dec.off}
}

func (dec *Decoder) long() (int64, error) {
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

func (dec *Decoder) fixnum(v reflect.Value) error {
	n, err := dec.long()
	if err != nil {
		return err
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() > 0 {
			return InvalidTypeError{ExpectedType: "(u)int", ActualType: v.Type(), Offset: dec.off}
		}
		v.Set(reflect.ValueOf(n))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.OverflowInt(n) {
			return InvalidTypeError{ExpectedType: fmt.Sprintf("int type large enough for value %v", n), ActualType: v.Type(), Offset: dec.off}
		}
		v.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		un := uint64(n)
		if n < 0 {
			return InvalidTypeError{ExpectedType: "int", ActualType: v.Type(), Offset: dec.off}
		}
		if v.OverflowUint(un) {
			return InvalidTypeError{ExpectedType: fmt.Sprintf("uint type large enough for value %v", n), ActualType: v.Type(), Offset: dec.off}
		}
		v.SetUint(un)
		return nil
	}

	if v.Kind() == reflect.Struct && v.Type() == bigIntType {
		bigint := v.Addr().Interface().(*big.Int)
		bigint.SetInt64(n)

		if n < 0 {
			bigint.Neg(bigint)
		}
		return nil
	}

	return InvalidTypeError{ExpectedType: "(u)int", ActualType: v.Type(), Offset: dec.off}
}

func (dec *Decoder) bignum(v reflect.Value) error {
	sign, err := dec.byte("bignum sign")
	if err != nil {
		return err
	}

	sz, err := dec.long()
	if err != nil {
		return err
	}
	bytes, err := dec.bytes(sz*2, "bignum bytes")
	if err != nil {
		return err
	}
	reverseBytes(bytes)

	var bi *big.Int

	switch v.Kind() {
	case reflect.Struct:
		if v.Type() != bigIntType {
			return InvalidTypeError{ExpectedType: "big.Int", ActualType: v.Type(), Offset: dec.off}
		}
		bi = v.Addr().Interface().(*big.Int)
	case reflect.Interface:
		if v.NumMethod() > 0 {
			return InvalidTypeError{ExpectedType: "big.Int", ActualType: v.Type(), Offset: dec.off}
		}
		v.Set(reflect.New(bigIntType))
		bi = v.Interface().(*big.Int)
	}

	bi.SetBytes(bytes)
	if sign == '-' {
		bi.Neg(bi)
	}

	return nil
}

func (dec *Decoder) symbol(v reflect.Value) error {
	off := dec.off

	str, err := dec.rawstr()
	if err != nil {
		return err
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(str)
		return nil
	case reflect.Interface:
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf(str))
		}
	}

	return InvalidTypeError{ExpectedType: "string|Symbol", ActualType: v.Type(), Offset: off}
}

func (dec *Decoder) array(v reflect.Value) error {
	off := dec.off

	szl, err := dec.long()
	if err != nil {
		return err
	}

	sz := int(szl)

	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() > 0 {
			return InvalidTypeError{ExpectedType: "slice|array", ActualType: v.Type(), Offset: off}
		}
		// Holy shit. This is the hackiest crap I've ever written.
		// If we've encountered an array but the target type is interface{}
		// then we construct a slice of type interface{}.
		ifaceT := reflect.TypeOf(new(interface{})).Elem()
		newSlice := reflect.MakeSlice(reflect.SliceOf(ifaceT), sz, sz)
		v.Set(newSlice)
		v = newSlice
	case reflect.Array:
		// We could do an overflow check rather than an exact one.
		// But zeroing out an array via reflection sounds like a whole lot of hard work.
		// And honestly, who the fuck uses arrays in userland?!
		if v.Cap() != sz {
			return InvalidTypeError{ExpectedType: fmt.Sprintf("array[%d]", sz), ActualType: v.Type(), Offset: off}
		}
	case reflect.Slice:
		// Need to initialize a "proper" slice with the type the original slice is using.
		v.Set(reflect.MakeSlice(reflect.SliceOf(v.Type().Elem()), sz, sz))
	default:
		return InvalidTypeError{ExpectedType: "slice|array", ActualType: v.Type(), Offset: off}
	}

	for i := 0; i < int(sz); i++ {
		if err := dec.val(v.Index(i)); err != nil {
			return err
		}
	}

	// TODO: fix caching
	// dec.cacheObj(arr)

	return nil
}

// func (dec *Decoder) hash() (interface{}, error) {
// 	sz, err := dec.num()
// 	if err != nil {
// 		return nil, err
// 	}

// 	m := make(map[interface{}]interface{}, sz)

// 	for i := 0; i < int(sz); i++ {
// 		k, err := dec.val()
// 		if err != nil {
// 			return nil, err
// 		}
// 		v, err := dec.val()
// 		if err != nil {
// 			return nil, err
// 		}
// 		m[k] = v
// 	}

// 	dec.cacheObj(m)

// 	return m, nil
// }

// func (dec *Decoder) symlink() (Symbol, error) {
// 	id, err := dec.num()
// 	if err != nil {
// 		return "", err
// 	}

// 	sym, found := dec.symCache[int(id)]
// 	if !found {
// 		return "", dec.error(fmt.Sprintf("Invalid symbol symlink id %d encountered.", id))
// 	}
// 	return *sym, nil
// }

// func (dec *Decoder) module() (*Module, error) {
// 	str, err := dec.rawstr()
// 	if err != nil {
// 		return nil, err
// 	}

// 	return NewModule(str), nil
// }

// func (dec *Decoder) class() (*Class, error) {
// 	str, err := dec.rawstr()
// 	if err != nil {
// 		return nil, err
// 	}

// 	return NewClass(str), nil
// }

// func (dec *Decoder) ivar() (interface{}, error) {
// 	val, err := dec.val()
// 	if err != nil {
// 		return nil, err
// 	}

// 	num, err := dec.num()
// 	if err != nil {
// 		return nil, err
// 	}

// 	ivars := make(map[string]interface{}, num)
// 	for i := 0; i < int(num); i++ {
// 		k, err := dec.nextsym()
// 		if err != nil {
// 			return nil, err
// 		}
// 		v, err := dec.val()
// 		if err != nil {
// 			return nil, err
// 		}

// 		ivars[k] = v
// 	}

// 	// If this is an ASCII/UTF-8 string and there's no other ivars associated
// 	// Wee just unwrap and return the string itself.
// 	if reflect.TypeOf(val).Name() == "string" && len(ivars) == 1 {
// 		if _, found := ivars["E"]; found {
// 			// It doesn't matter whether it's US-ASCII or UTF-8 proper. ASCII is a subset
// 			// of UTF-8 so we can just pass it along unmolested.
// 			return val, nil
// 		}
// 	}

// 	return &IVar{
// 		Data:      val,
// 		Variables: ivars,
// 	}, nil
// }

// func (dec *Decoder) instance(usr bool) (*Instance, error) {
// 	inst := new(Instance)
// 	var err error

// 	inst.UserMarshalled = usr
// 	if inst.Name, err = dec.nextsym(); err != nil {
// 		return nil, err
// 	}

// 	dec.cacheObj(inst)

// 	if usr {
// 		val, err := dec.val()
// 		if err != nil {
// 			return nil, err
// 		}
// 		inst.Data = val
// 	}

// 	if !usr {
// 		sz, err := dec.num()
// 		if err != nil {
// 			return nil, err
// 		}
// 		inst.InstanceVars = make(map[string]interface{})

// 		for i := 0; i < int(sz); i++ {
// 			key, err := dec.nextsym()
// 			if err != nil {
// 				return nil, err
// 			}

// 			val, err := dec.val()
// 			if err != nil {
// 				return nil, err
// 			}

// 			inst.InstanceVars[key] = val
// 		}
// 	}

// 	return inst, nil
// }

// func (dec *Decoder) link() (interface{}, error) {
// 	id, err := dec.num()
// 	if err != nil {
// 		return nil, err
// 	}

// 	if inst, found := dec.objCache[int(id)]; found {
// 		return inst, nil
// 	}

// 	return nil, dec.error(fmt.Sprintf("Object link with id %d not found.", id))
// }

// // Expects next value in stream to be a Symbol and returns the string repr of it.
// func (dec *Decoder) nextsym() (string, error) {
// 	v, err := dec.val()
// 	if err != nil {
// 		return "", err
// 	}
// 	if sym, ok := v.(Symbol); ok {
// 		return string(sym), nil
// 	} else {
// 		return "", dec.error(fmt.Sprintf("Unexpected value %v (%T) - expected Symbol", v, v))
// 	}
// }

func (dec *Decoder) byte(op string) (byte, error) {
	b, err := dec.r.ReadByte()
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("Error while reading %s", op))
	}
	dec.off++
	return b, nil
}

func (dec *Decoder) bytes(sz int64, op string) ([]byte, error) {
	b := make([]byte, sz)

	if _, err := io.ReadFull(dec.r, b); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error while reading %s", op))
	}
	dec.off += sz
	return b, nil
}

func (dec *Decoder) uint16(op string) (uint16, error) {
	b, err := dec.bytes(2, op)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (dec *Decoder) rawstr() (string, error) {
	sz, err := dec.long()
	if err != nil {
		return "", err
	}

	b, err := dec.bytes(sz, "symbol")
	if err != nil {
		return "", err
	}

	str := string(b)
	dec.cacheObj(str)

	return str, nil
}

func (dec *Decoder) cacheObj(v interface{}) {
	dec.objCache[len(dec.objCache)] = v
}

func (dec *Decoder) error(msg string) error {
	return errors.New(fmt.Sprintf("%s (offset=%d)", msg, dec.off))
}

func indirect(v reflect.Value) reflect.Value {
	for {
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() {
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Ptr {
			break
		}

		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}

		v = v.Elem()
	}

	return v
}
