package rmarsh

import (
	"math/big"
	"reflect"
)

const (
	TYPE_NIL        = '0'
	TYPE_TRUE       = 'T'
	TYPE_FALSE      = 'F'
	TYPE_FIXNUM     = 'i'
	TYPE_BIGNUM     = 'l'
	TYPE_FLOAT      = 'f'
	TYPE_ARRAY      = '['
	TYPE_HASH       = '{'
	TYPE_SYMBOL     = ':'
	TYPE_SYMLINK    = ';'
	TYPE_STRING     = '"'
	TYPE_REGEXP     = '/'
	TYPE_IVAR       = 'I'
	TYPE_CLASS      = 'c'
	TYPE_MODULE     = 'm'
	TYPE_OBJECT     = 'o'
	TYPE_LINK       = '@'
	TYPE_USRMARSHAL = 'U'
)

var (
	magic = []byte{4, 8}

	symbolType   = reflect.TypeOf(Symbol(""))
	classType    = reflect.TypeOf(Class(""))
	moduleType   = reflect.TypeOf(Module(""))
	instanceType = reflect.TypeOf(Instance{})
	regexpType   = reflect.TypeOf(Regexp{})
	ivarType     = reflect.TypeOf(IVar{})

	bigIntType   = reflect.TypeOf(*big.NewInt(0))
	bigFloatType = reflect.TypeOf(*big.NewFloat(0))

	ifaceType  = reflect.TypeOf(new(interface{})).Elem()
	stringType = reflect.TypeOf("")
)
