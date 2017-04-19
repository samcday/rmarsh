package rmarsh

import (
	"fmt"
	"reflect"
)

// Symbol represents a Ruby Symbol
// https://ruby-doc.org/core-2.4.0/Symbol.html
type Symbol string

// Class represents a reference to a Ruby Class
// https://ruby-doc.org/core-2.4.0/Class.html
type Class string

func NewClass(s string) *Class {
	class := Class(s)
	return &class
}

// Module represents a reference to a Ruby Module
// https://ruby-doc.org/core-2.4.0/Module.html
type Module string

func NewModule(s string) *Module {
	mod := Module(s)
	return &mod
}

// Instance represents an instance of a Ruby class.
// If the Instance is not user marshalled (runtime class has marshal_dump/marshal_load method)
// then the InstanceVars are serialized as part of the instance.
// Otherwise the Data value is serialized (and is what is passed in to marshal_load on Ruby side)
type Instance struct {
	Name           string
	UserMarshalled bool
	InstanceVars   map[string]interface{}
	Data           interface{}
}

// Regexp represents a Ruby Regexp expression.
type Regexp struct {
	Expr  string
	Flags uint8
}

const (
	REGEXP_IGNORECASE    = 1
	REGEXP_EXTENDED      = 1 << 1
	REGEXP_MULTILINE     = 1 << 2
	REGEXP_FIXEDENCODING = 1 << 4
	REGEXP_NOENCODING    = 1 << 5
)

type IVar struct {
	Data      interface{}
	Variables map[string]interface{}
}

func NewEncodingIVar(data interface{}, encoding string) *IVar {
	iv := IVar{Variables: make(map[string]interface{})}
	iv.Data = data
	iv.Variables["encoding"] = rawString(encoding)
	return &iv
}

// A bit of a hack to ensure we break a recursive loop when handling encoding instance var
type rawString string

type InvalidTypeError struct {
	ExpectedType string
	ActualType   reflect.Type
	Offset       int64
}

func (e InvalidTypeError) Error() string {
	return fmt.Sprintf("Invalid type %s encountered at offset %d - expected %s", e.ActualType, e.Offset, e.ExpectedType)
}
