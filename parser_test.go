package rmarsh_test

import (
	"bytes"
	"testing"

	"github.com/samcday/rmarsh"
)

func parseFromRuby(t *testing.T, expr string) *rmarsh.Parser {
	b := rbEncode(t, expr)
	return rmarsh.NewParser(bytes.NewReader(b))
}

func expectToken(t testing.TB, p *rmarsh.Parser, exp rmarsh.Token) {
	tok, err := p.Next()
	if err != nil {
		t.Fatalf("Error reading token: %s", err)
	}
	if tok != exp {
		t.Errorf("Expected to read token %s, got %s", exp, tok)
	}
}

func TestParserNil(t *testing.T) {
	p := parseFromRuby(t, "nil")
	expectToken(t, p, rmarsh.TokenNil)
	expectToken(t, p, rmarsh.TokenEOF)
	// Hitting EOF should just continue to return EOF tokens.
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserBool(t *testing.T) {
	p := parseFromRuby(t, "true")
	expectToken(t, p, rmarsh.TokenTrue)
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	expectToken(t, p, rmarsh.TokenTrue)
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserFixnum(t *testing.T) {
	p := parseFromRuby(t, "123")
	expectToken(t, p, rmarsh.TokenFixnum)
	if n, err := p.Int(); err != nil {
		t.Errorf("p.Int() err %s", err)
	} else if n != 123 {
		t.Errorf("Expected p.Int() = %d, expected 123", n)
	}
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Int(); err == nil || err.Error() != "rmarsh.Parser.Int() called for wrong token: TokenTrue" {
		t.Errorf("p.Int() unexpected err %s", err)
	}
}

func BenchmarkDecodeFixnum(b *testing.B) {
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
		t.Errorf("Expected p.Float() = %f, expected 123.321", n)
	}
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Float(); err == nil || err.Error() != "rmarsh.Parser.Float() called for wrong token: TokenTrue" {
		t.Errorf("p.Float() unexpected err %s", err)
	}
}

func BenchmarkDecodeFloat(b *testing.B) {
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
		t.Errorf("Expected p.BigNum() = %s, expected -deadcafebeef", str)
	}
	expectToken(t, p, rmarsh.TokenEOF)

	p = parseFromRuby(t, "true")
	p.Next()
	if _, err := p.Float(); err == nil || err.Error() != "rmarsh.Parser.Float() called for wrong token: TokenTrue" {
		t.Errorf("p.Float() unexpected err %s", err)
	}
}

func BenchmarkDecodeBigNum(b *testing.B) {
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
