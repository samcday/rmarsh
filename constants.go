package rmarsh

import (
	"math/big"
)

var (
	magic = []byte{4, 8}
)

const (
	typeNil        = '0'
	typeTrue       = 'T'
	typeFalse      = 'F'
	typeFixnum     = 'i'
	typeBignum     = 'l'
	typeFloat      = 'f'
	typeArray      = '['
	typeHash       = '{'
	typeSymbol     = ':'
	typeSymlink    = ';'
	typeString     = '"'
	typeRegExp     = '/'
	typeIvar       = 'I'
	typeClass      = 'c'
	typeModule     = 'm'
	typeObject     = 'o'
	typeLink       = '@'
	typeUsrMarshal = 'U'
	typeUsrDef     = 'u'
	typeStruct     = 'S'
)

// Modifier flags for Ruby regular expressions
const (
	RegexpIgnoreCase    = 1
	RegexpExtended      = 1 << 1
	RegexpMultiline     = 1 << 2
	RegexpFixedEncoding = 1 << 4
	RegexpNoEncoding    = 1 << 5
)

const (
	// The highest+lowest values that can be encoded in a fixnum
	fixnumMin = -0x40000000
	fixnumMax = +0x3FFFFFFF

	// Max number of a bytes a fixnum can occupy
	fixnumMaxBytes = 5
)

// Ripped from math/big since we deal with the raw Words ourselves
const (
	_m    = ^big.Word(0)
	_logS = _m>>8&1 + _m>>16&1 + _m>>32&1
	_S    = 1 << _logS
)
