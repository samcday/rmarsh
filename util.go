package rubymarshal

import (
	"reflect"
)

func reverseBytes(b []byte) {
	for i := len(b)/2 - 1; i >= 0; i-- {
		opp := len(b) - 1 - i
		b[i], b[opp] = b[opp], b[i]
	}
}

// Given an interface{}, if it's not a pointer to something, return an interface{}
// that is. This is useful so that our reflection code can just deal consistently with pointers to stuff.
func ptrto(v interface{}) interface{} {
	sigh := reflect.ValueOf(v)
	if sigh.Kind() != reflect.Ptr {
		r := reflect.New(sigh.Type())
		r.Elem().Set(sigh)
		return r.Interface()
	}

	return v
}
