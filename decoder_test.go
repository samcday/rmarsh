package rubymarshal

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func testRubyEncode(t *testing.T, payload string, expected interface{}) {
	cmd := exec.Command("ruby", "decoder_test.rb")
	cmd.Stdin = strings.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Ruby encode failed: %s\n%s", err, stderr.String())
	}

	raw := stdout.Bytes()
	dec := NewDecoder(bytes.NewReader(raw))
	v, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode() failed: %s\nRaw ruby encoded:\n%s", err, hex.Dump(raw))
	}

	if !reflect.DeepEqual(v, expected) {
		t.Errorf("Decode() gave %#v (%T), expected %#v\nRaw ruby encoded:\n%s", v, v, expected, hex.Dump(raw))
	}
}

func TestDecodeNil(t *testing.T) {
	testRubyEncode(t, "nil", nil)
}

func TestDecodeTrue(t *testing.T) {
	testRubyEncode(t, "true", true)
}

func TestDecodeFalse(t *testing.T) {
	testRubyEncode(t, "false", false)
}

func TestDecodeFixnums(t *testing.T) {
	testRubyEncode(t, "0", int64(0))
	testRubyEncode(t, "1", int64(1))
	testRubyEncode(t, "122", int64(122))
	testRubyEncode(t, "0xDEAD", int64(0xDEAD))
	testRubyEncode(t, "0xDEADBE", int64(0xDEADBE))

	testRubyEncode(t, "-1", int64(-1))
	testRubyEncode(t, "-123", int64(-123))
	testRubyEncode(t, "-0xDEAD", int64(-0xDEAD))
}

func TestDecodeBignums(t *testing.T) {
	var exp big.Int
	exp.SetString("DEADCAFEBEEF", 16)
	testRubyEncode(t, "0xDEADCAFEBEEF", &exp)

	exp.SetString("-DEADCAFEBEEF", 16)
	testRubyEncode(t, "-0xDEADCAFEBEEF", &exp)
}

func TestDecodeArray(t *testing.T) {
	testRubyEncode(t, "[]", []interface{}{})
	testRubyEncode(t, "[nil, true, false]", []interface{}{nil, true, false})
	testRubyEncode(t, "[[]]", []interface{}{[]interface{}{}})
}

func TestDecodeSymbol(t *testing.T) {
	testRubyEncode(t, ":test", NewSymbol("test"))
}

func TestDecodeSymlink(t *testing.T) {
	testRubyEncode(t, "[:test,:test]", []interface{}{NewSymbol("test"), NewSymbol("test")})
}

func TestDecodeModule(t *testing.T) {
	testRubyEncode(t, "Process", NewModule("Process"))
}

func TestDecodeClass(t *testing.T) {
	testRubyEncode(t, "Gem::Version", NewClass("Gem::Version"))
}

func TestDecodeString(t *testing.T) {
	testRubyEncode(t, `"test".force_encoding("SHIFT_JIS")`, "test")
}
