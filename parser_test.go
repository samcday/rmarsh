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

func expectToken(t *testing.T, p *rmarsh.Parser, exp rmarsh.Token) {
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
