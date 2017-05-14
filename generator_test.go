package rmarsh_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
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

func BenchmarkGenReset(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)
	}
}

func BenchmarkGenNil(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

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
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

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
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

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
	gen := rmarsh.NewGenerator(ioutil.Discard)
	var bnum big.Int
	bnum.SetString("DEADCAFEBEEFEEBAE", 16)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

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
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

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
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

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
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.Float(123.123123123); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenArray(t *testing.T) {
	testGenerator(t, `[false, true, nil]`, func(gen *rmarsh.Generator) error {
		if err := gen.StartArray(3); err != nil {
			return err
		}
		if err := gen.Bool(false); err != nil {
			return err
		}
		if err := gen.Bool(true); err != nil {
			return err
		}
		if err := gen.Nil(); err != nil {
			return err
		}
		return gen.EndArray()
	})
}

func TestGenArrayOverflow(t *testing.T) {
	gen := rmarsh.NewGenerator(ioutil.Discard)
	if err := gen.StartArray(1); err != nil {
		t.Fatal(err)
	}
	if err := gen.Nil(); err != nil {
		t.Fatal(err)
	}
	if err := gen.Nil(); err != rmarsh.ErrGeneratorOverflow {
		t.Fatalf("Unexpected error %+v", err)
	}
}

func BenchmarkGenArray(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.StartArray(1); err != nil {
			b.Fatal(err)
		}
		if err := gen.Nil(); err != nil {
			b.Fatal(err)
		}
		if err := gen.EndArray(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenLargeArray(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.StartArray(10); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < 10; i++ {
			if err := gen.Nil(); err != nil {
				b.Fatal(err)
			}
		}
		if err := gen.EndArray(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenHash(t *testing.T) {
	testGenerator(t, `{:foo=>123}`, func(gen *rmarsh.Generator) error {
		if err := gen.StartHash(1); err != nil {
			return err
		}
		if err := gen.Symbol("foo"); err != nil {
			return err
		}
		if err := gen.Fixnum(123); err != nil {
			return err
		}
		return gen.EndHash()
	})
}

func BenchmarkGenHash(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.StartHash(1); err != nil {
			b.Fatal(err)
		}
		if err := gen.Symbol("test"); err != nil {
			b.Fatal(err)
		}
		if err := gen.Fixnum(123); err != nil {
			b.Fatal(err)
		}
		if err := gen.EndHash(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenClass(t *testing.T) {
	testGenerator(t, `Class<File>`, func(gen *rmarsh.Generator) error {
		return gen.Class("File")
	})
}

func BenchmarkGenClass(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.Class("File"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenModule(t *testing.T) {
	testGenerator(t, `Module<Process>`, func(gen *rmarsh.Generator) error {
		return gen.Module("Process")
	})
}

func BenchmarkGenModule(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.Module("Process"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenIVar(t *testing.T) {
	testGenerator(t, `IVarTest<"bacon">`, func(gen *rmarsh.Generator) error {
		if err := gen.StartIVar(1); err != nil {
			return err
		}
		if err := gen.StartHash(0); err != nil {
			return err
		}
		if err := gen.EndHash(); err != nil {
			return err
		}
		if err := gen.Symbol("@ivartest"); err != nil {
			return err
		}
		if err := gen.String("bacon"); err != nil {
			return err
		}
		return gen.EndIVar()
	})
}

func BenchmarkGenIVar(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.StartIVar(1); err != nil {
			b.Fatal(err)
		}
		if err := gen.String("test"); err != nil {
			b.Fatal(err)
		}
		if err := gen.Symbol("E"); err != nil {
			b.Fatal(err)
		}
		if err := gen.Bool(true); err != nil {
			b.Fatal(err)
		}
		if err := gen.EndIVar(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenIVarInvalidKey(t *testing.T) {
	gen := rmarsh.NewGenerator(ioutil.Discard)
	if err := gen.StartIVar(1); err != nil {
		t.Fatal(err)
	}
	if err := gen.Nil(); err != nil {
		t.Fatal(err)
	}

	if err := gen.Nil(); err != rmarsh.ErrNonSymbolValue {
		t.Fatalf("Unexpected error %+v", err)
	}
}

func TestGenObject(t *testing.T) {
	testGenerator(t, `#Object<:@bar=123 :@foo="test">`, func(gen *rmarsh.Generator) error {
		if err := gen.StartObject("Object", 2); err != nil {
			return err
		}
		if err := gen.Symbol("@foo"); err != nil {
			return err
		}
		if err := gen.String("test"); err != nil {
			return err
		}
		if err := gen.Symbol("@bar"); err != nil {
			return err
		}
		if err := gen.Fixnum(123); err != nil {
			return err
		}
		return gen.EndObject()
	})
}

func TestGenObjectInvalidKey(t *testing.T) {
	gen := rmarsh.NewGenerator(ioutil.Discard)
	if err := gen.StartObject("Foo", 1); err != nil {
		t.Fatal(err)
	}

	if err := gen.Nil(); err != rmarsh.ErrNonSymbolValue {
		t.Fatalf("Unexpected error %+v", err)
	}
}

func TestGenUserMarshalled(t *testing.T) {
	testGenerator(t, `UsrMarsh<[{:foo=>"bar"}]>`, func(gen *rmarsh.Generator) error {
		if err := gen.StartUserMarshalled("UsrMarsh"); err != nil {
			return err
		}
		if err := gen.StartArray(1); err != nil {
			return err
		}
		if err := gen.StartHash(1); err != nil {
			return err
		}
		if err := gen.Symbol("foo"); err != nil {
			return err
		}
		if err := gen.String("bar"); err != nil {
			return err
		}
		if err := gen.EndHash(); err != nil {
			return err
		}
		if err := gen.EndArray(); err != nil {
			return err
		}
		return gen.EndUserMarshalled()
	})
}

func TestGenUserDefined(t *testing.T) {
	testGenerator(t, `UsrDef<"test">`, func(gen *rmarsh.Generator) error {
		return gen.UserDefinedObject("UsrDef", "test")
	})
}

func BenchmarkGenUserDefined(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.UserDefinedObject("UsrDef", "test"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGenRegexp(t *testing.T) {
	testGenerator(t, `/test/i`, func(gen *rmarsh.Generator) error {
		return gen.Regexp("test", rmarsh.RegexpIgnoreCase)
	})
}

func TestGenStruct(t *testing.T) {
	testGenerator(t, `TestStruct<"test">`, func(gen *rmarsh.Generator) error {
		if err := gen.StartStruct("TestStruct", 1); err != nil {
			return err
		}
		if err := gen.Symbol("test"); err != nil {
			return err
		}
		if err := gen.String("test"); err != nil {
			return err
		}
		return gen.EndStruct()
	})
}

func BenchmarkGenStruct(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := gen.StartStruct("TestStruct", 1); err != nil {
			b.Fatal(err)
		}
		if err := gen.Symbol("Test"); err != nil {
			b.Fatal(err)
		}
		if err := gen.Bool(true); err != nil {
			b.Fatal(err)
		}
		if err := gen.EndStruct(); err != nil {
			b.Fatal(err)
		}
	}
}
