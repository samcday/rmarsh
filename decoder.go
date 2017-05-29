package rmarsh

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// A Decoder decodes a Ruby Marshal stream into concrete Golang structures.
type Decoder struct {
	p        *Parser
	objCache map[int]reflect.Value
	curToken Token
}

// NewDecoder builds a new Decoder that uses given Parser to decode a Ruby Marshal stream.
func NewDecoder(p *Parser) *Decoder {
	return &Decoder{p: p, objCache: make(map[int]reflect.Value)}
}

// ReadValue will consume a full Ruby Marshal stream from the given io.Reader and return a fully decoded Golang object.
func ReadValue(r io.Reader, val interface{}) error {
	// TODO: grab Parser instance from a sync.Pool
	return NewDecoder(NewParser(r)).Decode(val)
}

// Decode will consume a value from the underlying parser and marshal it into the provided Golang type.
func (d *Decoder) Decode(val interface{}) error {
	v := reflect.ValueOf(val)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("Invalid decode target %T, did you forget to pass a pointer?", val)
	}

	return d.valueDecoder(v.Elem())(d, v.Elem())
}

func (d *Decoder) nextToken() (Token, error) {
	if d.curToken != tokenInvalid {
		tok := d.curToken
		d.curToken = tokenInvalid
		return tok, nil
	}
	return d.p.Next()
}

type decoderFunc func(*Decoder, reflect.Value) error

func (d *Decoder) valueDecoder(v reflect.Value) decoderFunc {
	return d.typeDecoder(v.Type())
}

var decCache struct {
	sync.RWMutex
	m map[reflect.Type]decoderFunc
}

func (d *Decoder) typeDecoder(t reflect.Type) decoderFunc {
	decCache.RLock()
	dec := decCache.m[t]
	decCache.RUnlock()
	if dec != nil {
		return dec
	}

	decCache.Lock()
	defer decCache.Unlock()
	if decCache.m == nil {
		decCache.m = make(map[reflect.Type]decoderFunc)
	}
	decCache.m[t] = newTypeDecoder(t)
	return decCache.m[t]
}

func newTypeDecoder(t reflect.Type) decoderFunc {
	switch t.Kind() {
	case reflect.Invalid:
		return skipDecoder
	case reflect.Bool:
		return boolDecoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intDecoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintDecoder
	case reflect.Float32, reflect.Float64:
		return floatDecoder
	case reflect.String:
		return stringDecoder
	case reflect.Slice:
		return newSliceDecoder(t)
	case reflect.Struct:
		return newStructDecoder(t)
	case reflect.Ptr:
		// Since Ruby doesn't offer pointer types
		// We can simplify our type handling by normalising all Go pointers down to
		// single depth. e.g if we've been passed a ***string, we'll normalise it
		// down to *string before doing anything with it.
		if t.Elem().Kind() == reflect.Ptr {
			return newPtrIndirector(t)
		}
		return newPtrDecoder(t)
	}
	return unsupportedTypeDecoder
}

func skipDecoder(d *Decoder, v reflect.Value) error {
	_, err := d.p.readNext()
	if err != nil {
		return err
	}
	return d.p.Skip()
}

func boolDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenTrue, TokenFalse:
		v.SetBool(tok == TokenTrue)
		return nil
	default:
		// TODO: build a path
		return fmt.Errorf("Unexpected token %v encountered while decoding bool", tok)
	}
}
func intDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenFixnum:
		n, err := d.p.Int()
		if err != nil {
			return err
		}
		nn := int64(n)
		if v.OverflowInt(nn) {
			return fmt.Errorf("Decoded int %d exceeds maximum width of %s", n, v.Type())
		}
		v.SetInt(nn)
		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding int", tok)
	}
}

func uintDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenFixnum:
		n, err := d.p.Int()
		if err != nil {
			return err
		}
		un := uint64(n)
		if v.OverflowUint(un) {
			return fmt.Errorf("Decoded uint %d exceeds maximum width of %s", n, v.Type())
		}
		v.SetUint(un)
		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding uint", tok)
	}
}

func floatDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenFloat:
		f, err := d.p.Float()
		if err != nil {
			return err
		}
		if v.OverflowFloat(f) {
			return fmt.Errorf("Decoded float %f exceeds maximum width of %s", f, v.Type())
		}
		v.SetFloat(f)
		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding float", tok)
	}
}

func stringDecoder(d *Decoder, v reflect.Value) (err error) {
	var tok Token

	tok, err = d.nextToken()
	if err != nil {
		return
	}

	if tok == TokenLink {
		lnkID := d.p.LinkID()
		cached, ok := d.objCache[lnkID]
		if ok {
			cached = cached.Elem()
			if cached.Kind() == reflect.String {
				v.SetString(cached.String())
				return
			}
		}

		err = fmt.Errorf("Unknown link id %d", lnkID)
		return
	}

	isIVar := tok == TokenStartIVar
	lnkID := d.p.LinkID()

	if isIVar {
		tok, err = d.p.Next()
		if err != nil {
			return
		}
	}

	if tok != TokenString && tok != TokenSymbol {
		return fmt.Errorf("Unexpected token %v encountered while decoding string", tok)
	}

	var str string
	str, err = d.p.Text()
	if err != nil {
		return
	}
	v.SetString(str)

	_, ok := d.objCache[lnkID]
	if !ok {
		cacheV := reflect.New(v.Type())
		cacheV.Elem().SetString(str)
		d.objCache[lnkID] = cacheV
	}

	if isIVar {
		// TODO: properly parse IVar. For now, we just skip over encoding and such.
		if err = d.p.ExpectNext(TokenIVarProps); err != nil {
			return
		}
		err = d.p.Skip()
	}

	return
}

type sliceDecoder struct {
	elemDec decoderFunc
}

func (sliceDec *sliceDecoder) decode(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	if tok != TokenStartArray {
		return fmt.Errorf("Unexpected token %v encountered while decoding slice", tok)
	}

	l := d.p.Len()

	// If the underlying slice already has enough capacity for this array, then we just resize
	// and use it.
	cap := v.Cap()
	if cap >= l {
		v.SetLen(l)
		v.SetCap(l)
	} else {
		v.Set(reflect.MakeSlice(v.Type(), l, l))
	}

	lnkID := d.p.LinkID()
	if lnkID > -1 {
		d.objCache[lnkID] = v.Addr()
	}

	for i := 0; i < l; i++ {
		if err := sliceDec.elemDec(d, v.Index(i)); err != nil {
			return err
		}
	}

	if err := d.p.ExpectNext(TokenEndArray); err != nil {
		return err
	}

	return nil
}

func newSliceDecoder(t reflect.Type) decoderFunc {
	dec := &sliceDecoder{newTypeDecoder(t.Elem())}
	return dec.decode
}

type idxStructField struct {
	idx int // index in the struct
	dec decoderFunc
}
type idxStructDecoder struct {
	fields []idxStructField
}

func (idxDec *idxStructDecoder) decode(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenStartArray:
		l := d.p.Len()

		for i := 0; i < l; i++ {
			if i < len(idxDec.fields) {
				f := idxDec.fields[i]
				if f.dec != nil {
					if err := f.dec(d, v.Field(f.idx)); err != nil {
						return err
					}
					continue
				}
			}

			if _, err := d.p.Next(); err != nil {
				return err
			}

			if err := d.p.Skip(); err != nil {
				return err
			}
		}
		if err := d.p.ExpectNext(TokenEndArray); err != nil {
			return err
		}

		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding indexed struct", tok)
	}

}

func newStructDecoder(t reflect.Type) decoderFunc {
	// A struct decoder can either be indexed or named.
	// Indexed decoders expect to decode a Ruby Array into a Go struct.
	// Named decoders expecet to decode a Ruby Hash/Struct into a Go struct.
	var idxFields []idxStructField
	named := make(map[string]decoderFunc)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		if f.PkgPath != "" {
			continue
		}

		fdec := newTypeDecoder(f.Type)

		meta := strings.Split(f.Tag.Get("rmarsh"), ",")
		if meta[0] == "" {
			continue
		}
		if meta[0] == "_indexed" {
			if len(named) > 0 {
				return newErrorDecoder(fmt.Errorf("Cannot mix named and _indexed fields in struct %s", t))
			}
			idx, err := strconv.ParseInt(meta[1], 10, 32)
			if err != nil {
				return newErrorDecoder(fmt.Errorf("Struct %s field %q has invalid _indexed value %q", t, f.Name, meta[1]))
			}
			if len(idxFields) <= int(idx) {
				idxFields = append(idxFields, make([]idxStructField, int(idx)-len(idxFields)+1)...)
			}
			idxFields[idx] = idxStructField{idx: i, dec: fdec}
		} else {
			if len(idxFields) > 0 {
				return newErrorDecoder(fmt.Errorf("Cannot mix named and _indexed fields in struct %s", t))
			}
			named[f.Name] = fdec
		}
	}

	if len(idxFields) > 0 {
		dec := &idxStructDecoder{idxFields}
		return dec.decode
	}
	if len(named) > 0 {
		dec := &idxStructDecoder{}
		return dec.decode
	}
	return skipDecoder
}

type ptrDecoder struct {
	elemDec decoderFunc
}

func (ptrDec *ptrDecoder) decode(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	// If the token is nil, then we nil the ptr and move on.
	if tok == TokenNil {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	// If we've just parsed a link, then we see if we've cached the object already.
	// If we we have, and it's directly assignable to the pointer type we have, then
	// we can just immediately assign it and continue.
	// Otherwise we start a replay parser and run it on the target.
	if tok == TokenLink {
		lnkID := d.p.LinkID()
		cached, ok := d.objCache[lnkID]

		if ok && cached.Type().AssignableTo(v.Type()) {
			v.Set(cached)
			return nil
		}

		// TODO: setup a replay parser and run it against the target.
		return fmt.Errorf("Unhandled link encountered. %d", lnkID)
	}

	v.Set(reflect.New(v.Type().Elem()))

	// Push the token back and decode against resolved ptr.
	d.curToken = tok

	lnkID := d.p.LinkID()
	if _, ok := d.objCache[lnkID]; !ok {
		cacheV := reflect.New(v.Type()).Elem()
		cacheV.Set(v)
		d.objCache[lnkID] = cacheV
	}

	err = ptrDec.elemDec(d, v.Elem())

	return err
}

func newPtrDecoder(t reflect.Type) decoderFunc {
	dec := &ptrDecoder{newTypeDecoder(t.Elem())}
	return dec.decode
}

type ptrIndirector struct {
	types   []reflect.Type
	elemDec decoderFunc
}

func (ptrIndir *ptrIndirector) decode(d *Decoder, v reflect.Value) error {
	for _, typ := range ptrIndir.types {
		if v.IsNil() {
			v.Set(reflect.New(typ))
		}
		v = v.Elem()
	}
	return ptrIndir.elemDec(d, v)
}

func newPtrIndirector(t reflect.Type) decoderFunc {
	var types []reflect.Type
	for t.Elem().Kind() == reflect.Ptr {
		types = append(types, t.Elem())
		t = t.Elem()
	}

	dec := &ptrIndirector{types: types, elemDec: newPtrDecoder(t)}
	return dec.decode
}

type errorDecoder struct {
	err error
}

func (errDec errorDecoder) decode(d *Decoder, v reflect.Value) error {
	return errDec.err
}

func newErrorDecoder(err error) decoderFunc {
	return errorDecoder{err}.decode
}

func unsupportedTypeDecoder(d *Decoder, v reflect.Value) error {
	return fmt.Errorf("unsupported type %s", v.Type())
}
