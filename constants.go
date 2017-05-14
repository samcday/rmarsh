package rmarsh

import (
	"math/big"
)

var (
	magic = []byte{4, 8}
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
	TYPE_USRDEF     = 'u'
	TYPE_STRUCT     = 'S'
)

const (
	REGEXP_IGNORECASE    = 1
	REGEXP_EXTENDED      = 1 << 1
	REGEXP_MULTILINE     = 1 << 2
	REGEXP_FIXEDENCODING = 1 << 4
	REGEXP_NOENCODING    = 1 << 5
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
