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
