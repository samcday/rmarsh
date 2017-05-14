package rmarsh

import (
	"fmt"
	"reflect"
)

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
