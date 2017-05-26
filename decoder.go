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
	objCache []reflect.Value
	curToken Token
}

// NewDecoder builds a new Decoder that uses given Parser to decode a Ruby Marshal stream.
func NewDecoder(p *Parser) *Decoder {
	return &Decoder{p: p}
}

// ReadValue will consume a full Ruby Marshal stream from the given io.Reader and return a fully decoded Golang object.
func ReadValue(r io.Reader, val interface{}) error {
	// TODO: grab Parser instance from a sync.Pool
	return NewDecoder(NewParser(r)).Decode(val)
}

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

var decCache struct {
	sync.RWMutex
	m map[reflect.Type]decoderFunc
}

func (d *Decoder) valueDecoder(v reflect.Value) decoderFunc {
	return d.typeDecoder(v.Type())
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
	// TODO: support other types and coerce them to something bool-y?
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

func stringDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenStartIVar:
		// We're okay with an IVar as long as the next token is a String.
		if err := d.p.ExpectNext(TokenString); err != nil {
			return err
		}
		// TODO: properly parse IVar. For now, we just skip over encoding and such.
		d.p.Skip()
		return nil
	case TokenString, TokenSymbol:
		str, err := d.p.Text()
		if err != nil {
			return err
		}
		v.SetString(str)

		lnkId := d.p.LinkId()
		if lnkId > -1 {
			d.objCache = append(d.objCache, v.Addr())
		}

		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding string", tok)
	}
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

	lnkId := d.p.LinkId()
	if lnkId > -1 {
		d.objCache = append(d.objCache, v.Addr())
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
		if l > len(idxDec.fields) {
			l = len(idxDec.fields)
		}

		for i := 0; i < l; i++ {
			f := idxDec.fields[i]
			fmt.Println("woot %+v", f)
			if err := f.dec(d, v.Field(f.idx)); err != nil {
				return err
			}
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
			fmt.Println("hmmm", len(idxFields), idx)
			idxFields[idx] = idxStructField{idx: int(idx), dec: fdec}
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
		lnkID := d.p.LinkId()
		cached := d.objCache[lnkID]
		if cached.Type().AssignableTo(v.Type()) {
			v.Set(cached)
		} else {
			// TODO: setup a replay parser and run it against the target.
		}
		return nil
	}

	// Push the token back and decode against resolved ptr.
	d.curToken = tok

	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return ptrDec.elemDec(d, v.Elem())
}

func newPtrDecoder(t reflect.Type) decoderFunc {
	dec := &ptrDecoder{newTypeDecoder(t.Elem())}
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
