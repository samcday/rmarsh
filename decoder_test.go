package rmarsh_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/samcday/rmarsh"
)

func testRubyEncode(t *testing.T, payload string, v interface{}) {
	raw := rbEncode(t, payload)

	if err := rmarsh.NewDecoder(bytes.NewReader(raw)).Decode(v); err != nil {
		t.Fatalf("Decode() failed: %s\nRaw ruby encoded:\n%s", err, hex.Dump(raw))
	}
}

func TestDecodeNil(t *testing.T) {
	var m map[string]int
	testRubyEncode(t, "nil", &m)
	if m != nil {
		t.Errorf("Expected m to be nil, got %v %T", m, m)
	}

	var s []string
	testRubyEncode(t, "nil", &s)
	if s != nil {
		t.Errorf("Expected s to be nil, got %v %T", s, s)
	}
}

func TestDecodeBool(t *testing.T) {
	var b bool
	testRubyEncode(t, "true", &b)
	if !b {
		t.Errorf("Expected b to be true, got false")
	}

	testRubyEncode(t, "false", &b)
	if b {
		t.Errorf("Expected b to be false, got true")
	}

	var i interface{}
	testRubyEncode(t, "true", &i)
	if !reflect.DeepEqual(i, true) {
		t.Errorf("Expected i to be true, got %v %T", i, i)
	}
}

func TestDecodeFixnums(t *testing.T) {
	var n *int8
	testRubyEncode(t, "1", &n)
	if *n != 1 {
		t.Errorf("Expected n to be 1, got %v", n)
	}
	testRubyEncode(t, "-122", &n)
	if *n != -122 {
		t.Errorf("Expected n to be -122, got %v", n)
	}

	var un uint16
	testRubyEncode(t, "0xDEAD", &un)
	if un != 0xDEAD {
		t.Errorf("Expected un to be 0xDEAD, got %v", un)
	}
}

func TestDecodeFloats(t *testing.T) {
	var f *float32
	testRubyEncode(t, "1.123", &f)
	if *f != 1.123 {
		t.Errorf("Expected f to be 1.123, got %v", *f)
	}
}

func TestDecodeBignums(t *testing.T) {
	var b big.Int
	testRubyEncode(t, "0xDEADCAFEBEEF", &b)
	if b.Text(16) != "deadcafebeef" {
		t.Errorf("Expected b to be 0xDEADCAFEBEEF, got %s", b.Text(16))
	}
	testRubyEncode(t, "-0xDEADCAFEBEEF", &b)
	if b.Text(16) != "-deadcafebeef" {
		t.Errorf("Expected b to be -0xDEADCAFEBEEF, got %s", b.Text(16))
	}

	var pb *big.Int
	testRubyEncode(t, "0xDEADCAFEBEEF", &pb)
	if pb.Text(16) != "deadcafebeef" {
		t.Errorf("Expected b to be 0xDEADCAFEBEEF, got %s", pb.Text(16))
	}

	var anon interface{}
	testRubyEncode(t, "0xDEADCAFEBEEF", &anon)
	if v := anon.(*big.Int).Text(16); v != "deadcafebeef" {
		t.Errorf("Expected b to be 0xDEADCAFEBEEF, got %s", v)
	}
}

func TestDecodeSymbol(t *testing.T) {
	var str string
	testRubyEncode(t, ":test", &str)
	if str != "test" {
		t.Errorf("Expected str to be 'test', got %s", str)
	}

	var sym *rmarsh.Symbol
	testRubyEncode(t, ":testsym", &sym)
	if *sym != "testsym" {
		t.Errorf("Expected sym to be 'testsym', got %s", *sym)
	}
}

func TestDecodeArray(t *testing.T) {
	var i interface{}
	testRubyEncode(t, `[123, true, nil]`, &i)
	if !reflect.DeepEqual(i, []interface{}{int64(123), true, nil}) {
		t.Errorf(`Expected i to be [123, true, nil], got %v %T`, i, i)
	}

	var iarr []int
	testRubyEncode(t, `[123,321]`, &iarr)
	if !reflect.DeepEqual(iarr, []int{123, 321}) {
		t.Errorf(`Expected iarr to be [123, 321], got %s`, iarr)
	}

	var iparr []*int
	testRubyEncode(t, `[123,321]`, &iparr)
	v1 := 123
	v2 := 321
	if !reflect.DeepEqual(iparr, []*int{&v1, &v2}) {
		t.Errorf(`Expected iparr to be [123, 321], got %s`, iparr)
	}
}

type decodeFromArray struct {
	_   struct{} `rmarsh_indexed`
	Foo string
	Bar int
}

func TestDecodeArrayToStruct(t *testing.T) {
	var d decodeFromArray
	testRubyEncode(t, `["test", 123]`, &d)
	if !reflect.DeepEqual(d, decodeFromArray{struct{}{}, "test", 123}) {
		t.Errorf(`Expected d to be {"test", 123}, got %v`, d)
	}
}

func TestDecodeHashToMap(t *testing.T) {
	m := make(map[string][]*int)

	testRubyEncode(t, "{:foo => [123]}", &m)
	v := 123
	if !reflect.DeepEqual(m, map[string][]*int{"foo": []*int{&v}}) {
		t.Errorf(`Expected m to be {"foo": 123}, got %v`, m)
	}
}

type nested struct {
	Test int
}
type aStruct struct {
	Foo ***int     `rmarsh:"foo"`
	Bar [][][]bool `rmarsh:"bar"`
	// Quux nested
}

func TestDecodeHashToStruct(t *testing.T) {
	var st aStruct
	testRubyEncode(t, `{:foo => 123, :bar => [[[true]]]}`, &st)

	if ***st.Foo != 123 {
		t.Errorf("Expected st.Foo to equal 123, got %v", st.Foo)
	}
	if !reflect.DeepEqual(st.Bar, [][][]bool{{{true}}}) {
		t.Errorf("Expected st.Bar to equal [[[true]]], got %v", st.Bar)
	}
}

func TestDecodeString(t *testing.T) {
	var str string
	testRubyEncode(t, `"test"`, &str)

	if str != "test" {
		t.Errorf("Expected s to be test, got %s", str)
	}
}

func TestDecodeLink(t *testing.T) {
	var i []interface{}
	testRubyEncode(t, `[a = "test", a]`, &i)

	if i[0] != i[1] {
		t.Errorf("Expected i[0] to equal i[1]")
	}
}

func TestDecodeSymlink(t *testing.T) {
	var syms []rmarsh.Symbol
	testRubyEncode(t, "[:test,:test]", &syms)
	if syms[0] != syms[1] {
		t.Errorf("Expected syms[0] to equal syms[1]")
	}
}

func TestDecodeModule(t *testing.T) {
	var mod rmarsh.Module
	testRubyEncode(t, "Process", &mod)
	if mod != "Process" {
		t.Errorf("Expected mod to equal Process")
	}
}

func TestDecodeClass(t *testing.T) {
	var cl rmarsh.Class
	testRubyEncode(t, "Gem::Version", &cl)
	if cl != "Gem::Version" {
		t.Errorf("Expected cl to equal Gem::Version")
	}
}

// func TestDecodeInstance(t *testing.T) {
// 	testRubyEncode(t, `Gem::Version.new("1.2.3")`, &Instance{
// 		Name:           "Gem::Version",
// 		UserMarshalled: true,
// 		Data:           []interface{}{"1.2.3"},
// 	})
// }
