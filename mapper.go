package rmarsh

import (
	"fmt"
	"reflect"
)

type Mapper struct {
}

type encoderFunc func(gen *Generator, v reflect.Value) error

func (m *Mapper) WriteValue(gen *Generator, val interface{}) error {
	v := reflect.ValueOf(val)
	return valueEncoder(v)(gen, v)
}

func valueEncoder(v reflect.Value) encoderFunc {
	return typeEncoder(v.Type())
}

func typeEncoder(t reflect.Type) encoderFunc {
	switch t.Kind() {
	case reflect.Bool:
		return boolEncoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intEncoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintEncoder
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

func unsupportedTypeEncoder(gen *Generator, v reflect.Value) error {
	return fmt.Errorf("unsupported type %s", v.Type())
}
