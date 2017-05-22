package rmarsh_test

import (
	"bytes"
	"reflect"
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
	testDecoder(t, `"test".force_encoding("ASCII-8BIT")`, &s)
	if s != "test" {
		t.Errorf(`%v != "test"`, s)
	}
}

func TestDecoderFixnumArray(t *testing.T) {
	var arr []int
	testDecoder(t, `[123,321]`, &arr)
	if !reflect.DeepEqual(arr, []int{123, 321}) {
		t.Fatalf("%+v != [123,321]", arr)
	}
}

func TestDecoderStringArray(t *testing.T) {
	var s []string
	testDecoder(t, `["test".force_encoding("ASCII-8BIT"),"test".force_encoding("ASCII-8BIT")]`, &s)
	t.Log(s)
	if !reflect.DeepEqual(s, []string{"test", "test"}) {
		t.Errorf(`%+v != ["test", "test"]`, s)
	}
}

func TestDecoderStringLink(t *testing.T) {
	var s []*string
	testDecoder(t, `s = "test".force_encoding("ASCII-8BIT"); [s, s]`, &s)

	if *s[0] != "test" {
		t.Errorf(`%+v != "test"`, s[0])
	}

	if s[0] != s[1] {
		t.Error("ptrs do not match")
	}
}

func TestDecoderArrayLink(t *testing.T) {
	var arr []*[]int
	testDecoder(t, `a = [123]; [a,a]`, &arr)

	if arr[0] != arr[1] {
		t.Error("ptrs do not match")
	}
}
