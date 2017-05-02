package rmarsh_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
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

func BenchmarkGenNil(b *testing.B) {
	buf := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		gen.Reset()

		if err := gen.Nil(); err != nil {
			b.Fatal(err)
		}
	}
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

func BenchmarkGenBool(b *testing.B) {
	buf := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		gen.Reset()

		if err := gen.Bool(true); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenFixnums(t *testing.T) {
	testGenerator(t, "123", func(gen *rmarsh.Generator) error {
		return gen.Fixnum(123)
	})
	testGenerator(t, "666", func(gen *rmarsh.Generator) error {
		return gen.Fixnum(666)
	})
	testGenerator(t, fmt.Sprintf("%d", 0xDEADCAFEBEEF), func(gen *rmarsh.Generator) error {
		return gen.Fixnum(0xDEADCAFEBEEF)
	})
}

func BenchmarkGenFixnum(b *testing.B) {
	buf := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		gen.Reset()

		if err := gen.Fixnum(123); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenBignum(t *testing.T) {
	var bigpos, bigneg big.Int
	bigpos.SetString("+DEADCAFEBEEFEEBAE", 16)
	bigneg.SetString("-DEADCAFEBEEFEEBAE", 16)
	testGenerator(t, bigpos.String(), func(gen *rmarsh.Generator) error {
		return gen.Bignum(&bigpos)
	})
	testGenerator(t, bigneg.String(), func(gen *rmarsh.Generator) error {
		return gen.Bignum(&bigneg)
	})
}

func BenchmarkGenBignum(b *testing.B) {
	buf := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(buf)
	var bnum big.Int
	bnum.SetString("DEADCAFEBEEFEEBAE", 16)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		gen.Reset()

		if err := gen.Bignum(&bnum); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenSymbol(t *testing.T) {
	testGenerator(t, ":test", func(gen *rmarsh.Generator) error {
		return gen.Symbol("test")
	})
}

func BenchmarkGenSymbol(b *testing.B) {
	buf := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		gen.Reset()

		if err := gen.Symbol("test"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenString(t *testing.T) {
	testGenerator(t, `"foobar"`, func(gen *rmarsh.Generator) error {
		return gen.String("foobar")
	})
}

func BenchmarkGenString(b *testing.B) {
	buf := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		gen.Reset()

		if err := gen.String("test"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenFloat(t *testing.T) {
	testGenerator(t, `123.123123123`, func(gen *rmarsh.Generator) error {
		return gen.Float(123.123123123)
	})
}

func BenchmarkGenFloat(b *testing.B) {
	buf := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset()
		gen.Reset()

		if err := gen.Float(123.123123123); err != nil {
			b.Fatal(err)
		}
	}
}
