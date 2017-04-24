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

// Module represents a reference to a Ruby Module
// https://ruby-doc.org/core-2.4.0/Module.html
type Module string

// Instance represents an instance of a Ruby class.
// If the Instance is not user marshalled/defined (runtime class has marshal_load or _load method)
// then the InstanceVars are serialized as part of the instance.
// Otherwise the Data value is serialized (and is what is passed in to marshal_load or _load on Ruby side)
type Instance struct {
	Name           string
	UserMarshalled bool
	UserDefined    bool
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
	return fmt.Sprintf("Invalid type %s encountered - expected %s (at offset %d)", e.ActualType, e.ExpectedType, e.Offset)
}

type UnresolvedLinkError struct {
	Type   string
	Id     int64
	Offset int64
}

func (e UnresolvedLinkError) Error() string {
	return fmt.Sprintf("Invalid %s symlink id %d found (at offset %d)", e.Type, e.Id, e.Offset)
}

type IndexedStructOverflowError struct {
	Num      int
	Expected int
	Offset   int64
}

func (e IndexedStructOverflowError) Error() string {
	return fmt.Sprintf("Indexed struct ran out of exported fields at %d, need %d (at offset %d)", e.Num, e.Expected, e.Offset)
}
