package rmarsh_test

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"testing"

	"github.com/samcday/rmarsh"
)

func testMapperWriteValue(t *testing.T, exp string, v interface{}) {
	b := new(bytes.Buffer)
	gen := rmarsh.NewGenerator(b)

	if err := rmarsh.NewMapper().WriteValue(gen, v); err != nil {
		t.Fatal(err)
	}

	str := rbDecode(t, b.Bytes())
	if str != exp {
		t.Fatalf("Generated stream %s != %s\nRaw marshal:\n%s\n", str, exp, hex.Dump(b.Bytes()))
	}
}

func TestMapperWriteValueNilPtrs(t *testing.T) {
	ptrs := []interface{}{
		(*bool)(nil),
		(*int32)(nil),
		(*float64)(nil),
		(*string)(nil),
	}

	for _, ptr := range ptrs {
		testMapperWriteValue(t, `nil`, ptr)
	}
}

func TestMapperWriteValueBool(t *testing.T) {
	testMapperWriteValue(t, `true`, true)
	v := true
	testMapperWriteValue(t, `true`, &v)
}

func BenchmarkMapperWriteTrue(b *testing.B) {
	gen := rmarsh.NewGenerator(ioutil.Discard)
	mapper := rmarsh.NewMapper()
	v := true

	for i := 0; i < b.N; i++ {
		gen.Reset(nil)

		if err := mapper.WriteValue(gen, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func TestMapperWriteValueInt(t *testing.T) {
	testMapperWriteValue(t, `123456`, 123456)
}

func TestMapperWriteValueFloat(t *testing.T) {
	testMapperWriteValue(t, `123.321`, 123.321)
}

func TestMapperWriteValueString(t *testing.T) {
	testMapperWriteValue(t, `"test"`, "test")
}
