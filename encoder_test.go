package rubymarshal

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os/exec"
	"testing"
)

func checkAgainstRuby(t *testing.T, val interface{}, expected string) {
	b, err := Encode(val)
	if err != nil {
		t.Fatalf("Encode() failed: %s", err)
	}

	cmd := exec.Command("ruby", "test.rb")
	cmd.Stdin = bytes.NewReader(b)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Ruby decode failed: %s\n%s\nRaw encoded:\n%s", err, stderr.String(), hex.Dump(b))
	}

	result := stdout.String()
	if result != expected {
		t.Errorf("Encoded %v (%T), Ruby saw %s, expected %q\nRaw encoded:\n%s", val, val, result, expected, hex.Dump(b))
	}
}

func TestEncodeNil(t *testing.T) {
	checkAgainstRuby(t, nil, "nil")
}

func TestEncodeBools(t *testing.T) {
	checkAgainstRuby(t, true, "true")
	checkAgainstRuby(t, false, "false")

	// Ptrs
	v := true
	checkAgainstRuby(t, &v, "true")
}

func TestEncodeSymbols(t *testing.T) {
	// Basic symbol test
	checkAgainstRuby(t, Symbol("test"), ":test")

	// Basic symlink test
	checkAgainstRuby(t, []Symbol{Symbol("test"), Symbol("test")}, "[:test, :test]")

	// Slightly more contrived symlink test
	checkAgainstRuby(t, []Symbol{
		Symbol("foo"),
		Symbol("bar"),
		Symbol("bar"),
		Symbol("foo"),
	}, "[:foo, :bar, :bar, :foo]")

	// Ptr test
	sym := Symbol("foo")
	checkAgainstRuby(t, &sym, ":foo")
}

func TestEncodeInts(t *testing.T) {
	checkAgainstRuby(t, 0, "0")
	checkAgainstRuby(t, 0xDE, "222")
	checkAgainstRuby(t, 0xDEAD, "57005")
	checkAgainstRuby(t, 0xDEADBE, "14593470")
	checkAgainstRuby(t, 0x3DEADBEE, "1038801902")

	checkAgainstRuby(t, -0xDE, "-222")
	checkAgainstRuby(t, -0xDEAD, "-57005")
	checkAgainstRuby(t, -0xDEADBE, "-14593470")
	checkAgainstRuby(t, -0x3DEADBEE, "-1038801902")

	// Ptrs
	v := 123
	checkAgainstRuby(t, &v, "123")
}

func TestEncodeBigNums(t *testing.T) {
	// Check that regular int64s larger than the fixnum encodable range become bignums.
	checkAgainstRuby(t, int64(0xDEADCAFEBEEF), "244838016401135")

	// Check that actual math.Big numbers encode properly too.
	var huge1, huge2 big.Int
	huge1.SetString("DEADCAFEBABEBEEFDEADCAFEBABEBEEF", 16)
	huge2.SetString("-DEADCAFEBABEBEEFDEADCAFEBABEBEEF", 16)

	checkAgainstRuby(t, huge1, "295990999649265874631894574770086133487")
	checkAgainstRuby(t, huge2, "-295990999649265874631894574770086133487")

	// And ptrs.
	v := int64(0xDEADCAFEBEEF)
	checkAgainstRuby(t, &v, "244838016401135")
	checkAgainstRuby(t, &huge1, "295990999649265874631894574770086133487")
}

func TestEncodeFloats(t *testing.T) {
	checkAgainstRuby(t, 123.33, "123.33")

	// Ptrs
	v := 123.33
	checkAgainstRuby(t, &v, "123.33")
}

func TestEncodeStrings(t *testing.T) {
	checkAgainstRuby(t, "hi", `"hi"`)

	// Ptrs
	v := "test"
	checkAgainstRuby(t, &v, `"test"`)
}

func TestEncodeClass(t *testing.T) {
	checkAgainstRuby(t, Class("Gem::Version"), "Gem::Version")

	// Ptrs
	v := Class("Gem::Version")
	checkAgainstRuby(t, &v, "Gem::Version")
}

func TestEncodeModule(t *testing.T) {
	checkAgainstRuby(t, Module("Gem"), "Gem")

	// Ptrs
	v := Module("Gem")
	checkAgainstRuby(t, &v, "Gem")
}

func TestEncodeSlices(t *testing.T) {
	checkAgainstRuby(t, []int{}, "[]")
	checkAgainstRuby(t, []int{123}, "[123]")
	checkAgainstRuby(t, []interface{}{123, true, nil, Symbol("test"), "test"}, `[123, true, nil, :test, "test"]`)

	// Ptrs
	v := []int{123}
	checkAgainstRuby(t, &v, "[123]")
}

func TestEncodeMap(t *testing.T) {
	checkAgainstRuby(t, map[string]int{"foo": 123, "bar": 321}, `{"bar"=>321, "foo"=>123}`)

	// Ptrs
	v := map[int]int{123: 321}
	checkAgainstRuby(t, &v, `{123=>321}`)
}

func TestEncodeInstance(t *testing.T) {
	inst := Instance{
		Name: "Object",
		InstanceVars: map[string]interface{}{
			"@test": 123,
		},
	}
	checkAgainstRuby(t, inst, "#Object<:@test=123>")

	// Checking object links
	checkAgainstRuby(t, []interface{}{&inst, &inst}, "[#Object<:@test=123>, #Object<:@test=123>]")

	checkAgainstRuby(t, Instance{
		Name:           "Gem::Version",
		UserMarshalled: true,
		Data:           []string{"1.2.3"},
	}, `#<Gem::Version "1.2.3">`)
}

func TestEncodeRegexp(t *testing.T) {
	checkAgainstRuby(t, Regexp{
		Expr:  "test",
		Flags: REGEXP_MULTILINE | REGEXP_IGNORECASE,
	}, `/test/mi`)

	// Ptrs
	v := Regexp{Expr: "test"}
	checkAgainstRuby(t, &v, "/test/")
}
