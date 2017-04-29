package rmarsh_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/samcday/rmarsh"
)

func testGenerator(t *testing.T, exp string, f func(gen *rmarsh.Generator) error) {
	b := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(b)
	if err := f(gen); err != nil {
		t.Fatal(err)
	}

	str := rbDecode(t, b.Bytes())
	if str != exp {
		t.Fatalf("Generated stream %s != %s\nRaw marshal:\n%s\n", str, exp, hex.Dump(b.Bytes()))
	}
}

func TestGenNil(t *testing.T) {
	testGenerator(t, "nil", func(gen *rmarsh.Generator) error {
		return gen.Nil()
	})
}

func TestGenOverflow(t *testing.T) {
	b := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(b)
	if err := gen.Nil(); err != nil {
		t.Fatal(err)
	}
	if err := gen.Nil(); err != rmarsh.ErrGeneratorFinished {
		t.Fatalf("Err %s != rmarsh.ErrGeneratorFinished", err)
	}
}

func TestGenBool(t *testing.T) {
	testGenerator(t, "true", func(gen *rmarsh.Generator) error {
		return gen.Bool(true)
	})
	testGenerator(t, "false", func(gen *rmarsh.Generator) error {
		return gen.Bool(false)
	})
}
