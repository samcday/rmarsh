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
