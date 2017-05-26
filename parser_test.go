package rmarsh_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/samcday/rmarsh"
)

var curRaw []byte

func parseFromRuby(t *testing.T, expr string) *rmarsh.Parser {
	b := rbEncode(t, expr)
	curRaw = b
	return rmarsh.NewParser(bytes.NewReader(b))
}

func expectToken(t testing.TB, p *rmarsh.Parser, exp rmarsh.Token) {
	err := p.ExpectNext(exp)
	if err != nil {
		t.Fatalf("Error reading token: %+v\nRaw:\n%s\n", err, hex.Dump(curRaw))
	}
}

func BenchmarkParserReset(b *testing.B) {
	raw := rbEncode(b, "nil")
	buf := newCyclicReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)
	}
}

func TestParserInvalidMagic(t *testing.T) {
	raw := []byte{0x04, 0x07, '0'}
	p := rmarsh.NewParser(bytes.NewReader(raw))
	_, err := p.Next()
	if err == nil || err.Error() != "Expected magic header 0x0408, got 0x0407" {
		t.Fatalf("Unexpected err %s", err)
	}
}

func TestParserNil(t *testing.T) {
	p := parseFromRuby(t, "nil")
	expectToken(t, p, rmarsh.TokenNil)
	expectToken(t, p, rmarsh.TokenEOF)
	// Hitting EOF should just continue to return EOF tokens.
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserNil(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "nil"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenNil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParserBool(t *testing.T) {
	p := parseFromRuby(t, "true")
	expectToken(t, p, rmarsh.TokenTrue)
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	expectToken(t, p, rmarsh.TokenTrue)
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserBool(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "true"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenTrue); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParserFixnum(t *testing.T) {
	tests := []int{
		0x00,
		0x01,
		0x06,
		0xF0,
		0xDEAD,
		0x3FFFFFFF,
		-0x00,
		-0x01,
		-0x06,
		-0xF0,
		-0xDEAD,
		-0x3FFFFFFF,
	}

	for _, num := range tests {
		p := parseFromRuby(t, fmt.Sprintf("%#.2X", num))
		expectToken(t, p, rmarsh.TokenFixnum)
		if n, err := p.Int(); err != nil {
			t.Fatalf("p.Int() err %s", err)
		} else if n != num {
			t.Fatalf("p.Int() = %#.2X, expected %#.2X", n, num)
		}
		expectToken(t, p, rmarsh.TokenEOF)
	}

	p := parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Int(); err == nil || err.Error() != "rmarsh.Parser.Int() called for wrong token: TokenTrue" {
		t.Errorf("p.Int() unexpected err %s", err)
	}
}

func BenchmarkParserFixnumSingleByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "100"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenFixnum); err != nil {
			b.Fatal(err)
		}
		if n, err := p.Int(); err != nil || n != 100 {
			b.Fatalf("%v %v", n, err)
		}
	}
}
func BenchmarkParserFixnumMultiByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "0xBEEF"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenFixnum); err != nil {
			b.Fatal(err)
		}
		if n, err := p.Int(); err != nil || n != 0xBEEF {
			b.Fatalf("%v %v", n, err)
		}
	}
}

func TestParserFloat(t *testing.T) {
	p := parseFromRuby(t, "123.321")
	expectToken(t, p, rmarsh.TokenFloat)
	if n, err := p.Float(); err != nil {
		t.Errorf("p.Float() err %s", err)
	} else if n != 123.321 {
		t.Errorf("p.Float() = %f, expected 123.321", n)
	}
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Float(); err == nil || err.Error() != "rmarsh.Parser.Float() called for wrong token: TokenTrue" {
		t.Errorf("p.Float() unexpected err %s", err)
	}
}

func BenchmarkParserFloatSingleByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "1.to_f"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenFloat); err != nil {
			b.Fatal(err)
		}
		if f, err := p.Float(); err != nil || f != 1 {
			b.Fatalf("%v %v", f, err)
		}
	}
}

func BenchmarkParserFloatMultiByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "123.321"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenFloat); err != nil {
			b.Fatal(err)
		}
		if f, err := p.Float(); err != nil || f != 123.321 {
			b.Fatalf("%v %v", f, err)
		}
	}
}

func TestParserBignum(t *testing.T) {
	p := parseFromRuby(t, "-0xDEADCAFEBEEF")
	expectToken(t, p, rmarsh.TokenBignum)
	if n, err := p.Bignum(); err != nil {
		t.Errorf("p.Bignum() err %s", err)
	} else if str := n.Text(16); str != "-deadcafebeef" {
		t.Errorf("p.Bignum() = %s, expected -deadcafebeef", str)
	}
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Float(); err == nil || err.Error() != "rmarsh.Parser.Float() called for wrong token: TokenTrue" {
		t.Errorf("p.Float() unexpected err %s", err)
	}
}

func BenchmarkParserBignum(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "0xDEADCAFEBEEF"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenBignum); err != nil {
			b.Fatal(err)
		}
		if _, err := p.Bignum(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParserSymbol(t *testing.T) {
	p := parseFromRuby(t, ":test")
	expectToken(t, p, rmarsh.TokenSymbol)
	if str, err := p.Text(); err != nil {
		t.Errorf("p.Text() err %s", err)
	} else if str != "test" {
		t.Errorf("p.Text() = %s, expected test", str)
	}
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserSymbolSingleByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, ":E"))
	p := rmarsh.NewParser(buf)
	exp := []byte("E")

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenSymbol); err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(p.Bytes(), exp) {
			b.Fatalf("%s != test", p.Bytes())
		}
	}
}

func BenchmarkParserSymbolMultiByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, ":test"))
	p := rmarsh.NewParser(buf)
	exp := []byte("test")

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenSymbol); err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(p.Bytes(), exp) {
			b.Fatalf("%s != test", p.Bytes())
		}
	}
}

func TestParserString(t *testing.T) {
	// We generate this string in a convoluted way so it has no encoding (and thus no IVar)
	p := parseFromRuby(t, `"test".force_encoding("ASCII-8BIT")`)
	expectToken(t, p, rmarsh.TokenString)
	if str, err := p.Text(); err != nil {
		t.Errorf("p.Text() err %s", err)
	} else if str != "test" {
		t.Errorf("p.Text() = %s, expected test", str)
	}
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserString(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, `"test".force_encoding("ASCII-8BIT")`))
	p := rmarsh.NewParser(buf)
	exp := []byte("test")

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenString); err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(p.Bytes(), exp) {
			b.Fatalf("%s != test", p.Bytes())
		}
	}
}

func TestParserEmptyArray(t *testing.T) {
	p := parseFromRuby(t, "[]")
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserEmptyArray(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "[]"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenStartArray); err != nil {
			b.Fatal(err)
		}
		if err := p.ExpectNext(rmarsh.TokenEndArray); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParserNestedArray(t *testing.T) {
	p := parseFromRuby(t, "[[]]")
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "[[], []]")
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserSymlink(t *testing.T) {
	p := parseFromRuby(t, "[:test, :test]")
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenSymbol)
	expectToken(t, p, rmarsh.TokenSymbol)
	if str, err := p.Text(); err != nil {
		t.Errorf("p.Text() err %s", err)
	} else if str != "test" {
		t.Errorf("p.Text() = %s, expected test", str)
	}
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserSymlink(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "[:test, :test]"))
	p := rmarsh.NewParser(buf)
	exp := []byte("test")

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if err := p.ExpectNext(rmarsh.TokenStartArray); err != nil {
			b.Fatal(err)
		}
		if err := p.ExpectNext(rmarsh.TokenSymbol); err != nil {
			b.Fatal(err)
		}
		if err := p.ExpectNext(rmarsh.TokenSymbol); err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(p.Bytes(), exp) {
			b.Fatalf("%s != test", p.Bytes())
		}
		if err := p.ExpectNext(rmarsh.TokenEndArray); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParserHash(t *testing.T) {
	p := parseFromRuby(t, "{}")
	expectToken(t, p, rmarsh.TokenStartHash)
	expectToken(t, p, rmarsh.TokenEndHash)
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "{:foo => 123}")
	expectToken(t, p, rmarsh.TokenStartHash)
	expectToken(t, p, rmarsh.TokenSymbol)
	expectToken(t, p, rmarsh.TokenFixnum)
	expectToken(t, p, rmarsh.TokenEndHash)
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "{{} => 123}")
	expectToken(t, p, rmarsh.TokenStartHash)
	expectToken(t, p, rmarsh.TokenStartHash)
	expectToken(t, p, rmarsh.TokenEndHash)
	expectToken(t, p, rmarsh.TokenFixnum)
	expectToken(t, p, rmarsh.TokenEndHash)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserIVarArray(t *testing.T) {
	p := parseFromRuby(t, `[].tap{|v|v.instance_variable_set(:@test, 123)}`)
	expectToken(t, p, rmarsh.TokenStartIVar)
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenEndArray)

	expectToken(t, p, rmarsh.TokenIVarProps)
	if p.Len() != 1 {
		t.Errorf("p.Text() = %d, expected 1", p.Len())
	}

	expectToken(t, p, rmarsh.TokenSymbol)
	if str, err := p.Text(); err != nil {
		t.Fatal(err)
	} else if str != "@test" {
		t.Errorf("p.Text() = %s, expected @test", str)
	}
	expectToken(t, p, rmarsh.TokenFixnum)

	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserIVarHash(t *testing.T) {
	p := parseFromRuby(t, `{}.tap{|v|v.instance_variable_set(:@test, 123)}`)
	expectToken(t, p, rmarsh.TokenStartIVar)
	expectToken(t, p, rmarsh.TokenStartHash)
	expectToken(t, p, rmarsh.TokenEndHash)

	expectToken(t, p, rmarsh.TokenIVarProps)
	if p.Len() != 1 {
		t.Errorf("p.Text() = %d, expected 1", p.Len())
	}

	expectToken(t, p, rmarsh.TokenSymbol)
	expectToken(t, p, rmarsh.TokenFixnum)
	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserIVarString(t *testing.T) {
	p := parseFromRuby(t, `"test"`)
	expectToken(t, p, rmarsh.TokenStartIVar)
	expectToken(t, p, rmarsh.TokenString)

	expectToken(t, p, rmarsh.TokenIVarProps)
	if p.Len() != 1 {
		t.Errorf("p.Text() = %d, expected 1", p.Len())
	}

	expectToken(t, p, rmarsh.TokenSymbol)
	if str, err := p.Text(); err != nil {
		t.Fatal(err)
	} else if str != "E" {
		t.Errorf("p.Text() = %s, expected E", str)
	}
	expectToken(t, p, rmarsh.TokenTrue)
	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserLink(t *testing.T) {
	p := parseFromRuby(t, `a = 1.2; [a, a]`)
	expectToken(t, p, rmarsh.TokenStartArray)
	if id := p.LinkId(); id != 0 {
		t.Errorf("p.LinkId() = %d != 0", id)
	}
	expectToken(t, p, rmarsh.TokenFloat)
	if id := p.LinkId(); id != 1 {
		t.Errorf("p.LinkId() = %d != 1", id)
	}
	expectToken(t, p, rmarsh.TokenLink)
	if id := p.LinkId(); id != 1 {
		t.Errorf("p.LinkId() = %d != 1", id)
	}
	expectToken(t, p, rmarsh.TokenEndArray)
}

func TestParserReplayArray(t *testing.T) {
	p := parseFromRuby(t, `[]`)
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)

	sub, err := p.Replay(0)
	if err != nil {
		t.Fatal(err)
	}
	expectToken(t, sub, rmarsh.TokenStartArray)
	expectToken(t, sub, rmarsh.TokenEndArray)
	expectToken(t, sub, rmarsh.TokenEOF)
}

func TestParserReplayHash(t *testing.T) {
	p := parseFromRuby(t, `{}`)
	expectToken(t, p, rmarsh.TokenStartHash)
	expectToken(t, p, rmarsh.TokenEndHash)
	expectToken(t, p, rmarsh.TokenEOF)

	sub, err := p.Replay(0)
	if err != nil {
		t.Fatal(err)
	}
	expectToken(t, sub, rmarsh.TokenStartHash)
	expectToken(t, sub, rmarsh.TokenEndHash)
	expectToken(t, sub, rmarsh.TokenEOF)
}

func TestParserReplayFloat(t *testing.T) {
	p := parseFromRuby(t, `1.2`)
	expectToken(t, p, rmarsh.TokenFloat)
	expectToken(t, p, rmarsh.TokenEOF)

	sub, err := p.Replay(0)
	if err != nil {
		t.Fatal(err)
	}
	expectToken(t, sub, rmarsh.TokenFloat)
	expectToken(t, sub, rmarsh.TokenEOF)
}

func TestParserReplayRawString(t *testing.T) {
	p := parseFromRuby(t, `"test".force_encoding("ASCII-8BIT")`)
	expectToken(t, p, rmarsh.TokenString)
	expectToken(t, p, rmarsh.TokenEOF)

	sub, err := p.Replay(0)
	if err != nil {
		t.Fatal(err)
	}
	expectToken(t, sub, rmarsh.TokenString)
	expectToken(t, sub, rmarsh.TokenEOF)
}

func TestParserReplayIVarString(t *testing.T) {
	p := parseFromRuby(t, `"test"`)
	expectToken(t, p, rmarsh.TokenStartIVar)
	expectToken(t, p, rmarsh.TokenString)
	expectToken(t, p, rmarsh.TokenIVarProps)
	expectToken(t, p, rmarsh.TokenSymbol)
	expectToken(t, p, rmarsh.TokenTrue)
	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEOF)

	sub, err := p.Replay(0)
	if err != nil {
		t.Fatal(err)
	}
	expectToken(t, sub, rmarsh.TokenStartIVar)
	expectToken(t, sub, rmarsh.TokenString)
	expectToken(t, sub, rmarsh.TokenIVarProps)
	expectToken(t, sub, rmarsh.TokenSymbol)
	expectToken(t, sub, rmarsh.TokenTrue)
	expectToken(t, sub, rmarsh.TokenEndIVar)
	expectToken(t, sub, rmarsh.TokenEOF)
}

func TestParserReplayContrived(t *testing.T) {
	p := parseFromRuby(t, `a = 1.2; [a, a]`)

	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenFloat)
	expectToken(t, p, rmarsh.TokenLink)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)

	sub, err := p.Replay(0)
	if err != nil {
		t.Fatal(err)
	}

	expectToken(t, sub, rmarsh.TokenStartArray)
	expectToken(t, sub, rmarsh.TokenFloat)
	expectToken(t, sub, rmarsh.TokenLink)
	expectToken(t, sub, rmarsh.TokenEndArray)
	expectToken(t, sub, rmarsh.TokenEOF)

	sub2, err := p.Replay(1)
	if err != nil {
		t.Fatal(err)
	}
	expectToken(t, sub2, rmarsh.TokenFloat)
	expectToken(t, sub2, rmarsh.TokenEOF)
}

// func TestParserReplayIVar(t *testing.T) {
// 	p := parseFromRuby(t, `["test", 321]`)

// 	expectToken(t, p, rmarsh.TokenStartArray)
// 	expectToken(t, p, rmarsh.TokenStartIVar)
// 	expectToken(t, p, rmarsh.TokenString)
// 	expectToken(t, p, rmarsh.TokenSymbol)
// 	expectToken(t, p, rmarsh.TokenTrue)
// 	expectToken(t, p, rmarsh.TokenEndIVar)
// 	expectToken(t, p, rmarsh.TokenFixnum)
// 	expectToken(t, p, rmarsh.TokenEndArray)
// 	expectToken(t, p, rmarsh.TokenEOF)

// 	sub, err := p.Replay(1)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	expectToken(t, sub, rmarsh.TokenStartIVar)
// 	expectToken(t, sub, rmarsh.TokenString)
// 	expectToken(t, sub, rmarsh.TokenSymbol)
// 	expectToken(t, sub, rmarsh.TokenTrue)
// 	expectToken(t, sub, rmarsh.TokenEndIVar)
// 	expectToken(t, sub, rmarsh.TokenEOF)
// }
