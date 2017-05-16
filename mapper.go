package rmarsh

import (
	"reflect"
	"sync"
)

// Mapper provides a high level interface for marshalling/unmarshalling Golang objects from/to a Ruby Marshal stream.
// Mapper instances are thread safe and should be re-used as much as possible for performance reasons.
type Mapper struct {
	encLock  sync.RWMutex
	encCache map[reflect.Type]encoderFunc
}

// NewMapper constructs a new Mapper instance.
func NewMapper() *Mapper {
	return &Mapper{}
}

// WriteValue writes the given Golang object to the provided Generator instance. It is expected that the given Generator
// is in a state that is ready to accept writes for the given object.
func (m *Mapper) WriteValue(gen *Generator, val interface{}) error {
	v := reflect.ValueOf(val)
	return m.valueEncoder(v)(gen, v)
}
