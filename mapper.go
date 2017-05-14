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
