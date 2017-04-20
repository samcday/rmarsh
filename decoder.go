package rmarsh

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strconv"

	"github.com/pkg/errors"
)

type Decoder struct {
	r        *bufio.Reader
	off      int64
	symCache map[int]*Symbol
	objCache map[int]reflect.Value
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
	dec.objCache = make(map[int]reflect.Value)
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
	case TYPE_FLOAT:
		return dec.float(v)
	case TYPE_BIGNUM:
		return dec.bignum(v)
	case TYPE_SYMBOL:
		return dec.symbol(v)
	case TYPE_ARRAY:
		return dec.array(v)
	case TYPE_HASH:
		return dec.hash(v)
	case TYPE_SYMLINK:
		return dec.symlink(v)
	case TYPE_LINK:
		return dec.link(v)
	case TYPE_IVAR:
		return dec.ivar(v)
	case TYPE_STRING:
		return dec.string(v)
	// case TYPE_MODULE:
	// 	return dec.module()
	// case TYPE_CLASS:
	// 	return dec.class()
	// case TYPE_USRMARSHAL, TYPE_OBJECT:
	// 	return dec.instance(typ == TYPE_USRMARSHAL)
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
	off := dec.off

	n, err := dec.long()
	if err != nil {
		return err
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() > 0 {
			return InvalidTypeError{ExpectedType: "(u)int", ActualType: v.Type(), Offset: off}
		}
		v.Set(reflect.ValueOf(n))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.OverflowInt(n) {
			return InvalidTypeError{ExpectedType: fmt.Sprintf("int type large enough for value %v", n), ActualType: v.Type(), Offset: off}
		}
		v.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		un := uint64(n)
		if n < 0 {
			return InvalidTypeError{ExpectedType: "int", ActualType: v.Type(), Offset: off}
		}
		if v.OverflowUint(un) {
			return InvalidTypeError{ExpectedType: fmt.Sprintf("uint type large enough for value %v", n), ActualType: v.Type(), Offset: off}
		}
		v.SetUint(un)
		return nil
	}

	if v.Kind() == reflect.Struct && v.Type() == bigIntType {
		bigint := v.Addr().Interface().(*big.Int)
		bigint.SetInt64(n)
		return nil
	}

	return InvalidTypeError{ExpectedType: "(u)int", ActualType: v.Type(), Offset: off}
}

func (dec *Decoder) float(v reflect.Value) error {
	off := dec.off

	str, err := dec.rawstr()
	if err != nil {
		return err
	}

	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return err
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() > 0 {
			return InvalidTypeError{ExpectedType: "float", ActualType: v.Type(), Offset: off}
		}
		v.Set(reflect.ValueOf(f))
		return nil
	case reflect.Float32, reflect.Float64:
		if v.OverflowFloat(f) {
			return InvalidTypeError{ExpectedType: fmt.Sprintf("float type large enough for value %v", f), ActualType: v.Type(), Offset: off}
		}
		v.SetFloat(f)
		return nil
	}

	if v.Kind() == reflect.Struct && v.Type() == bigFloatType {
		bigf := v.Addr().Interface().(*big.Float)
		bigf.SetFloat64(f)
		return nil
	}

	return InvalidTypeError{ExpectedType: "float", ActualType: v.Type(), Offset: off}
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

	return setString(v, str, off)
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
		newSlice := reflect.MakeSlice(reflect.SliceOf(ifaceType), sz, sz)
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
		if err := dec.val(indirect(v.Index(i))); err != nil {
			return err
		}
	}

	dec.cacheObj(v)

	return nil
}

func (dec *Decoder) hash(v reflect.Value) error {
	off := dec.off
	sz, err := dec.long()
	if err != nil {
		return err
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() > 0 {
			return InvalidTypeError{ExpectedType: "map", ActualType: v.Type(), Offset: off}
		}
		newMap := reflect.MakeMap(reflect.MapOf(ifaceType, ifaceType))
		v.Set(newMap)
		v = newMap
		fallthrough
	case reflect.Map:
		if err := dec.hashMap(v, int(sz)); err != nil {
			return err
		}
	case reflect.Struct:
		if err := dec.hashStruct(v, int(sz)); err != nil {
			return err
		}
	default:
		return InvalidTypeError{ExpectedType: "map", ActualType: v.Type(), Offset: off}
	}

	// 	dec.cacheObj(m)

	return nil
}

func (dec *Decoder) hashMap(v reflect.Value, sz int) error {
	for i := 0; i < sz; i++ {
		rk := reflect.New(v.Type().Key())
		rv := reflect.New(v.Type().Elem())

		if err := dec.val(rk.Elem()); err != nil {
			return err
		}
		if err := dec.val(rv.Elem()); err != nil {
			return err
		}

		v.SetMapIndex(rk.Elem(), rv.Elem())
	}
	return nil
}

func (dec *Decoder) hashStruct(v reflect.Value, sz int) error {
	for i := 0; i < sz; i++ {
		rk := reflect.New(stringType)
		if err := dec.val(rk.Elem()); err != nil {
			return err
		}

		f := findStructField(v, rk.Elem().String())
		if !f.IsValid() {
			// Create an interface variable to hold whatever we deserialize as the value
			// then throw it away.
			if err := dec.val(indirect(reflect.New(ifaceType))); err != nil {
				return err
			}
			continue
		}

		if err := dec.val(indirect(f)); err != nil {
			return err
		}
	}
	return nil
}

func (dec *Decoder) symlink(v reflect.Value) error {
	off := dec.off

	id, err := dec.long()
	if err != nil {
		return err
	}

	sym, found := dec.symCache[int(id)]
	if !found {
		return UnresolvedLinkError{Id: id, Offset: off}
	}

	return setString(v, string(*sym), off)
}

func (dec *Decoder) link(v reflect.Value) error {
	off := dec.off

	id, err := dec.long()
	if err != nil {
		return err
	}

	_, found := dec.objCache[int(id)]
	if !found {
		return UnresolvedLinkError{Id: id, Offset: off}
	}

	return fmt.Errorf("Object links currently unimplemented")
}

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

func (dec *Decoder) ivar(v reflect.Value) error {
	if err := dec.val(v); err != nil {
		return err
	}

	sz, err := dec.long()
	if err != nil {
		return err
	}

	// TODO: come up with an idiomatic way to handle IVars...
	// For now we're just throwing them away.
	for i := 0; i < int(sz); i++ {
		if err := dec.val(indirect(reflect.New(ifaceType))); err != nil {
			return err
		}
		if err := dec.val(indirect(reflect.New(ifaceType))); err != nil {
			return err
		}
	}

	return nil
}

func (dec *Decoder) string(v reflect.Value) error {
	off := dec.off

	str, err := dec.rawstr()
	if err != nil {
		return err
	}

	fmt.Printf("eh %v\n", v.Type())
	return setString(v, str, off)
}

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
	// dec.cacheObj(str)

	return str, nil
}

func (dec *Decoder) cacheObj(v reflect.Value) {
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

func setString(v reflect.Value, str string, off int64) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(str)
		return nil
	case reflect.Interface:
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf(str))
			return nil
		}
	}

	return InvalidTypeError{ExpectedType: "string|Symbol", ActualType: v.Type(), Offset: off}
}

func findStructField(v reflect.Value, name string) reflect.Value {
	f := v.FieldByName(name)
	if f.IsValid() {
		return f
	}

	for i := 0; i < v.NumField(); i++ {
		if v.Type().Field(i).Tag.Get("rmarsh") == name {
			return v.Field(i)
		}
	}

	return reflect.Value{}
}
