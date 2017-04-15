package rubymarshal

// Symbol represents a Ruby Symbol
// https://ruby-doc.org/core-2.4.0/Symbol.html
// TODO: can't actually type this as a string.
// Since Symbols can have specific encoding (and be encoded in Marshal as an IVAR)
type Symbol string

// Class represents a reference to a Ruby Class
// https://ruby-doc.org/core-2.4.0/Class.html
type Class string

// Module represents a reference to a Ruby Module
// https://ruby-doc.org/core-2.4.0/Module.html
type Module string

// RString represents a Ruby String with a specific encoding.
type RString struct {
	Raw      string
	Encoding string
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
	Expr     string
	Encoding string
	Flags    uint8
}

const (
	REGEXP_IGNORECASE    = 1
	REGEXP_EXTENDED      = 1 << 1
	REGEXP_MULTILINE     = 1 << 2
	REGEXP_FIXEDENCODING = 1 << 4
	REGEXP_NOENCODING    = 1 << 5
)
