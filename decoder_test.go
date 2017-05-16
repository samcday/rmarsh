package rmarsh_test

import (
	"bytes"
	"testing"

	"github.com/samcday/rmarsh"
)

func testDecoder(t *testing.T, expr string, v interface{}) {
	b := rbEncode(t, expr)
	if err := rmarsh.ReadValue(bytes.NewReader(b), v); err != nil {
		t.Fatal(err)
	}
}

func TestDecoderNilPtr(t *testing.T) {
	b := true
	ptr := &b
	testDecoder(t, "nil", &ptr)

	if ptr != nil {
		t.Fatalf("ptr %+v is not nil", ptr)
	}
}

func TestDecoderBool(t *testing.T) {
	var v bool
	testDecoder(t, "true", &v)

	if v != true {
		t.Errorf("%v != true", v)
	}

	var ptr *bool
	testDecoder(t, "true", &ptr)

	if *ptr != true {
		t.Errorf("%v != true", ptr)
	}

	var silly *****bool
	testDecoder(t, "true", &silly)

	if *****silly != true {
		t.Errorf("%v != true", silly)
	}
}

func BenchmarkMapperReadTrue(b *testing.B) {
	r := newCyclicReader(rbEncode(b, "true"))
	p := rmarsh.NewParser(r)
	dec := rmarsh.NewDecoder(p)

	var v bool

	for i := 0; i < b.N; i++ {
		v = false
		p.Reset(nil)

		if err := dec.Decode(&v); err != nil {
			b.Fatal(err)
		} else if v != true {
			b.Fatalf("%v != true", v)
		}
	}
}

func TestDecoderInt(t *testing.T) {
	var n uint8
	testDecoder(t, "254", &n)
	if n != 254 {
		t.Errorf("%v != 254", n)
	}

	var un uint16
	testDecoder(t, "666", &un)
	if un != 666 {
		t.Errorf("%v != 666", un)
	}
}

func BenchmarkMapperReadUint(b *testing.B) {
	r := newCyclicReader(rbEncode(b, "0xDEAD"))
	p := rmarsh.NewParser(r)
	dec := rmarsh.NewDecoder(p)

	var n int32

	for i := 0; i < b.N; i++ {
		n = 0
		p.Reset(nil)

		if err := dec.Decode(&n); err != nil {
			b.Fatal(err)
		} else if n != 0xDEAD {
			b.Fatalf("%X != 0xDEAD", n)
		}
	}
}

func TestDecoderFloat(t *testing.T) {
	var n float32
	testDecoder(t, "123.321", &n)
	if n != 123.321 {
		t.Errorf("%v != 123.321", n)
	}
}

func TestDecoderString(t *testing.T) {
	var s string
	testDecoder(t, `[116,101,115,116].pack('c*')`, &s)
	if s != "test" {
		t.Errorf(`%v != "test"`, s)
	}
}
