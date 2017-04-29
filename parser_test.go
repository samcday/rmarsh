package rmarsh_test

import (
	"bytes"
	"encoding/hex"
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
	tok, err := p.Next()
	if err != nil {
		t.Fatalf("Error reading token: %+v\nRaw:\n%s\n", err, hex.Dump(curRaw))
	}
	if tok != exp {
		t.Fatalf("Expected to read token %s, got %s\nRaw:\n%s\n", exp, tok, hex.Dump(curRaw))
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
	raw := rbEncode(b, "nil")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
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
	raw := rbEncode(b, "true")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
	}
}

func TestParserFixnum(t *testing.T) {
	p := parseFromRuby(t, "123")
	expectToken(t, p, rmarsh.TokenFixnum)
	if n, err := p.Int(); err != nil {
		t.Errorf("p.Int() err %s", err)
	} else if n != 123 {
		t.Errorf("p.Int() = %d, expected 123", n)
	}
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Int(); err == nil || err.Error() != "rmarsh.Parser.Int() called for wrong token: TokenTrue" {
		t.Errorf("p.Int() unexpected err %s", err)
	}
}

func BenchmarkParserFixnum(b *testing.B) {
	raw := rbEncode(b, "0xBEEF")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
		p.Int()
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

func BenchmarkParserFloat(b *testing.B) {
	raw := rbEncode(b, "123.321")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
		p.Float()
	}
}

func TestParserBigNum(t *testing.T) {
	p := parseFromRuby(t, "-0xDEADCAFEBEEF")
	expectToken(t, p, rmarsh.TokenBigNum)
	if n, err := p.BigNum(); err != nil {
		t.Errorf("p.BigNum() err %s", err)
	} else if str := n.Text(16); str != "-deadcafebeef" {
		t.Errorf("p.BigNum() = %s, expected -deadcafebeef", str)
	}
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Float(); err == nil || err.Error() != "rmarsh.Parser.Float() called for wrong token: TokenTrue" {
		t.Errorf("p.Float() unexpected err %s", err)
	}
}

func BenchmarkParserBigNum(b *testing.B) {
	raw := rbEncode(b, "0xDEADCAFEBEEF")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
		p.BigNum()
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

func BenchmarkParserSymbol(b *testing.B) {
	raw := rbEncode(b, ":test")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
		p.Text()
	}
}

func TestParserString(t *testing.T) {
	// We generate this string in a convoluted way so it has no encoding (and thus no IVar)
	p := parseFromRuby(t, "[116,101,115,116].pack('c*')")
	expectToken(t, p, rmarsh.TokenString)
	if str, err := p.Text(); err != nil {
		t.Errorf("p.Text() err %s", err)
	} else if str != "test" {
		t.Errorf("p.Text() = %s, expected test", str)
	}
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserString(b *testing.B) {
	raw := rbEncode(b, "[116,101,115,116].pack('c*')")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
		p.Text()
	}
}

func TestParserEmptyArray(t *testing.T) {
	p := parseFromRuby(t, "[]")
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkEmptyArray(b *testing.B) {
	raw := rbEncode(b, "[]")
	buf := bytes.NewReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		buf.Reset(raw)
		p.Reset()

		p.Next()
		p.Next()
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

	// Next token should be the :@test value, a fixnum
	expectToken(t, p, rmarsh.TokenFixnum)
	// And the current instance variable should be @test
	if str, err := p.IVarName(); err != nil {
		t.Fatal(err)
	} else if str != "@test" {
		t.Errorf("p.IVarName() = %s, expected @test", str)
	}

	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserIVarHash(t *testing.T) {
	p := parseFromRuby(t, `{}.tap{|v|v.instance_variable_set(:@test, 123)}`)
	expectToken(t, p, rmarsh.TokenStartIVar)
	expectToken(t, p, rmarsh.TokenStartHash)
	expectToken(t, p, rmarsh.TokenEndHash)
	expectToken(t, p, rmarsh.TokenFixnum)
	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserIVarString(t *testing.T) {
	p := parseFromRuby(t, `"test"`)
	expectToken(t, p, rmarsh.TokenStartIVar)
	expectToken(t, p, rmarsh.TokenString)
	expectToken(t, p, rmarsh.TokenTrue)
	if str, err := p.IVarName(); err != nil {
		t.Fatal(err)
	} else if str != "E" {
		t.Errorf("p.IVarName() = %s, expected E", str)
	}
	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEOF)
}
