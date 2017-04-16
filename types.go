package rubymarshal

// Symbol represents a Ruby Symbol
// https://ruby-doc.org/core-2.4.0/Symbol.html
// TODO: can't actually type this as a string.
// Since Symbols can have specific encoding (and be encoded in Marshal as an IVAR)
type Symbol string

func NewSymbol(s string) *Symbol {
	sym := Symbol(s)
	return &sym
}

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
	Variables map[Symbol]interface{}
}

func NewEncodingIVar(data interface{}, encoding string) *IVar {
	iv := IVar{Variables: make(map[Symbol]interface{})}
	iv.Data = data
	iv.Variables[Symbol("encoding")] = rawString(encoding)
	return &iv
}

// A bit of a hack to ensure we break a recursive loop when handling encoding instance var
type rawString string
