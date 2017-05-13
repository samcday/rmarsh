package rmarsh

import (
	"fmt"
	"reflect"
	"sync"
)

type Mapper struct {
	encLock  sync.RWMutex
	encCache map[reflect.Type]encoderFunc

	decLock  sync.RWMutex
	decCache map[reflect.Type]decoderFunc
}

func NewMapper() *Mapper {
	return &Mapper{}
}

type encoderFunc func(gen *Generator, v reflect.Value) error
type decoderFunc func(p *Parser, v reflect.Value) error

func (m *Mapper) WriteValue(gen *Generator, val interface{}) error {
	v := reflect.ValueOf(val)
	return m.valueEncoder(v)(gen, v)
}

func (m *Mapper) ReadValue(p *Parser, val interface{}) error {
	v := reflect.ValueOf(val)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("Invalid decode target %T, did you forget to pass a pointer?", val)
	}

	return m.valueDecoder(v)(p, v)
}

func (m *Mapper) valueEncoder(v reflect.Value) encoderFunc {
	return m.typeEncoder(v.Type())
}

func (m *Mapper) valueDecoder(v reflect.Value) decoderFunc {
	return m.typeDecoder(v.Type())
}

func (m *Mapper) typeEncoder(t reflect.Type) encoderFunc {
	m.encLock.RLock()
	enc := m.encCache[t]
	m.encLock.RUnlock()
	if enc != nil {
		return enc
	}

	m.encLock.Lock()
	defer m.encLock.Unlock()
	if m.encCache == nil {
		m.encCache = make(map[reflect.Type]encoderFunc)
	}

	m.encCache[t] = newTypeEncoder(t)
	return m.encCache[t]
}

func (m *Mapper) typeDecoder(t reflect.Type) decoderFunc {
	m.decLock.RLock()
	dec := m.decCache[t]
	m.decLock.RUnlock()
	if dec != nil {
		return dec
	}

	m.decLock.Lock()
	defer m.decLock.Unlock()
	if m.decCache == nil {
		m.decCache = make(map[reflect.Type]decoderFunc)
	}
	m.decCache[t] = newTypeDecoder(t)
	return m.decCache[t]
}

func newTypeEncoder(t reflect.Type) encoderFunc {
	switch t.Kind() {
	case reflect.Bool:
		return boolEncoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intEncoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintEncoder
	case reflect.Ptr:
		return newPtrEncoder(t)
	}
	return unsupportedTypeEncoder
}

func newTypeDecoder(t reflect.Type) decoderFunc {
	switch t.Kind() {
	case reflect.Bool:
		return boolDecoder
	case reflect.Ptr:
		return newPtrDecoder(t)
	}
	return unsupportedTypeDecoder
}

func boolEncoder(gen *Generator, v reflect.Value) error {
	return gen.Bool(v.Bool())
}

func boolDecoder(p *Parser, v reflect.Value) error {
	tok, err := p.Next()
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

func intEncoder(gen *Generator, v reflect.Value) error {
	return gen.Fixnum(v.Int())
}

func uintEncoder(gen *Generator, v reflect.Value) error {
	// TODO: properly detect overflow of signed 64bit int size and use a Bignum in that case
	return gen.Fixnum(int64(v.Uint()))
}

func unsupportedTypeEncoder(gen *Generator, v reflect.Value) error {
	return fmt.Errorf("unsupported type %s", v.Type())
}

func unsupportedTypeDecoder(p *Parser, v reflect.Value) error {
	return fmt.Errorf("unsupported type %s", v.Type())
}

type ptrEncoder struct {
	elemEnc encoderFunc
}

func (e *ptrEncoder) encode(gen *Generator, v reflect.Value) error {
	if v.IsNil() {
		return gen.Nil()
	}
	return e.elemEnc(gen, v.Elem())
}

func newPtrEncoder(t reflect.Type) encoderFunc {
	enc := &ptrEncoder{newTypeEncoder(t.Elem())}
	return enc.encode
}

type ptrDecoder struct {
	elemDec decoderFunc
}

func (d *ptrDecoder) decode(p *Parser, v reflect.Value) error {
	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return d.elemDec(p, v.Elem())
}

func newPtrDecoder(t reflect.Type) decoderFunc {
	dec := &ptrDecoder{newTypeDecoder(t.Elem())}
	return dec.decode
}
