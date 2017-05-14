package rmarsh

import (
	"fmt"
	"reflect"
)

type encoderFunc func(gen *Generator, v reflect.Value) error

func (m *Mapper) valueEncoder(v reflect.Value) encoderFunc {
	return m.typeEncoder(v.Type())
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

func newTypeEncoder(t reflect.Type) encoderFunc {
	switch t.Kind() {
	case reflect.Bool:
		return boolEncoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intEncoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintEncoder
	case reflect.Float32, reflect.Float64:
		return floatEncoder
	case reflect.String:
		return stringEncoder
	case reflect.Ptr:
		return newPtrEncoder(t)
	}
	return unsupportedTypeEncoder
}

func boolEncoder(gen *Generator, v reflect.Value) error {
	return gen.Bool(v.Bool())
}

func intEncoder(gen *Generator, v reflect.Value) error {
	return gen.Fixnum(v.Int())
}

func uintEncoder(gen *Generator, v reflect.Value) error {
	// TODO: properly detect overflow of signed 64bit int size and use a Bignum in that case
	return gen.Fixnum(int64(v.Uint()))
}

func floatEncoder(gen *Generator, v reflect.Value) error {
	return gen.Float(v.Float())
}

func stringEncoder(gen *Generator, v reflect.Value) (err error) {
	if err = gen.StartIVar(1); err != nil {
		return
	}
	if err = gen.String(v.String()); err != nil {
		return
	}
	if err = gen.Symbol("E"); err != nil {
		return
	}
	if err = gen.Bool(true); err != nil {
		return
	}
	return gen.EndIVar()
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

func unsupportedTypeEncoder(gen *Generator, v reflect.Value) error {
	return fmt.Errorf("unsupported type %s", v.Type())
}
