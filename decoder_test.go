package rmarsh_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/samcday/rmarsh"
)

var (
	rubyDec    *exec.Cmd
	rubyDecOut *bufio.Scanner
	rubyDecIn  io.Writer
)

var streamDelim = []byte("$$END$$")

func scanStream(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) >= 7 {
		for i := 0; i <= len(data)-7; i++ {
			if bytes.Compare(data[i:i+7], streamDelim) == 0 {
				return i + 7, data[0:i], nil
			}
		}
	}
	return 0, nil, nil
}

func testRubyEncode(t *testing.T, payload string, v interface{}) {
	if rubyDec == nil {
		rubyDec = exec.Command("ruby", "decoder_test.rb")
		// Send stderr to top level so it's obvious if the Ruby script blew up somehow.
		rubyDec.Stderr = os.Stdout

		stdout, err := rubyDec.StdoutPipe()
		if err != nil {
			panic(err)
		}
		stdin, err := rubyDec.StdinPipe()
		if err != nil {
			panic(err)
		}
		if err := rubyDec.Start(); err != nil {
			panic(err)
		}

		rubyDecOut = bufio.NewScanner(stdout)
		rubyDecOut.Split(scanStream)
		rubyDecIn = stdin
	}

	_, err := io.WriteString(rubyDecIn, fmt.Sprintf("%s\n", payload))
	if err != nil {
		panic(err)
	}

	rubyDecOut.Scan()
	raw := rubyDecOut.Bytes()
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

// func TestDecodeSymlink(t *testing.T) {
// 	testRubyEncode(t, "[:test,:test]", []interface{}{Symbol("test"), Symbol("test")})
// }

// func TestDecodeModule(t *testing.T) {
// 	testRubyEncode(t, "Process", NewModule("Process"))
// }

// func TestDecodeClass(t *testing.T) {
// 	testRubyEncode(t, "Gem::Version", NewClass("Gem::Version"))
// }

// func TestDecodeInstance(t *testing.T) {
// 	testRubyEncode(t, `Gem::Version.new("1.2.3")`, &Instance{
// 		Name:           "Gem::Version",
// 		UserMarshalled: true,
// 		Data:           []interface{}{"1.2.3"},
// 	})
// }

// func TestDecodeLink(t *testing.T) {
// 	testRubyEncode(t, `u = Gem::Version.new("1.2.3"); [u,u]`, func(v interface{}) error {
// 		arr, ok := v.([]interface{})
// 		if !ok {
// 			return fmt.Errorf("Unexpected type %T", v)
// 		}
// 		if arr[0] != arr[1] {
// 			return fmt.Errorf("%v (%T) != %v (%T)", arr[0], arr[0], arr[1], arr[1])
// 		}
// 		return nil
// 	})
// }
