package rubymarshal

import (
	"bytes"
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
		t.Fatalf("Error checking Ruby: %s\n%s", err, stderr.String())
	}

	result := stdout.String()
	if result != expected {
		t.Errorf("Encoded %v (%T), Ruby saw %s, expected %q", val, val, result, expected)
	}
}

func TestEncodeNil(t *testing.T) {
	checkAgainstRuby(t, nil, "nil")
}

func TestEncodeBools(t *testing.T) {
	checkAgainstRuby(t, true, "true")
	checkAgainstRuby(t, false, "false")
}

func TestEncodeSymbols(t *testing.T) {
	checkAgainstRuby(t, Symbol("test"), ":test")

	checkAgainstRuby(t, []Symbol{Symbol("test"), Symbol("test")}, "[:test, :test]")
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
}

func TestEncodeStrings(t *testing.T) {
	checkAgainstRuby(t, "hi", `"hi"`)
}

func TestEncodeClass(t *testing.T) {
	checkAgainstRuby(t, Class("Gem::Version"), "Gem::Version")
}

func TestEncodeModule(t *testing.T) {
	checkAgainstRuby(t, Module("Gem"), "Gem")
}

func TestEncodeSlices(t *testing.T) {
	checkAgainstRuby(t, []int{}, "[]")
	checkAgainstRuby(t, []int{123}, "[123]")
	checkAgainstRuby(t, []interface{}{123, true, nil, Symbol("test"), "test"}, `[123, true, nil, :test, "test"]`)
}

func TestEncodeMap(t *testing.T) {
	checkAgainstRuby(t, map[string]int{"foo": 123, "bar": 321}, `{"bar"=>321, "foo"=>123}`)
}

func TestEncodeInstance(t *testing.T) {
	checkAgainstRuby(t, Instance{
		Name: "Object",
		InstanceVars: map[string]interface{}{
			"@test": 123,
		},
	}, "#Object<:@test=123>")

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
}
