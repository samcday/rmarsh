package rmarsh_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"

	"github.com/samcday/rmarsh"
)

var curRaw []byte

func parseFromRuby(t *testing.T, expr string) *rmarsh.Parser {
	b := rbEncode(t, expr)
	curRaw = b
	return rmarsh.NewParser(bytes.NewReader(b))
}

func expectToken(t testing.TB, p *rmarsh.Parser, exp rmarsh.Token) ([]byte, int) {
	tok, buf, lnkID, err := p.Read()
	if err != nil {
		t.Fatal(err)
	} else if tok != exp {
		t.Fatalf("Token %q is not expected %q: %+v\nRaw:\n%s\n", tok, exp, hex.Dump(curRaw))
	}

	return buf, lnkID
}

func BenchmarkParserReset(b *testing.B) {
	raw := rbEncode(b, "nil")
	buf := newCyclicReader(raw)
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)
	}
}

func TestParserNil(t *testing.T) {
	p := parseFromRuby(t, "nil")

	expectToken(t, p, rmarsh.TokenNil)

	expectToken(t, p, rmarsh.TokenEOF)
	// Hitting EOF should just continue to return EOF tokens.
	expectToken(t, p, rmarsh.TokenEOF)
}

func TestParserInvalidMagic(t *testing.T) {
	raw := []byte{0x04, 0x07, '0'}
	p := rmarsh.NewParser(bytes.NewReader(raw))
	_, _, _, err := p.Read()
	if err == nil || err.Error() != "Expected magic header 0x0408, got 0x0407" {
		t.Fatalf("Unexpected err %s", err)
	}
}

func BenchmarkParserNil(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "nil"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, _, _, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenNil {
			b.Fatalf("Wrong token %s", tok)
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

		if tok, _, _, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenTrue {
			b.Fatalf("Unexpected token %s", tok)
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
		_, n := expectToken(t, p, rmarsh.TokenFixnum)
		if n != num {
			t.Fatalf("p.Int() = %#.2X, expected %#.2X", n, num)
		}
		expectToken(t, p, rmarsh.TokenEOF)
	}
}

func BenchmarkParserFixnumSingleByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "100"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, _, n, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenFixnum {
			b.Fatalf("Unexpected token %s", tok)
		} else if n != 100 {
			b.Fatalf("%v %v", n, err)
		}
	}
}

func BenchmarkParserFixnumMultiByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "0xBEEF"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, _, n, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenFixnum {
			b.Fatalf("Unexpected token %s", tok)
		} else if n != 0xBEEF {
			b.Fatalf("%v %v", n, err)
		}
	}
}

func TestParserFloat(t *testing.T) {
	p := parseFromRuby(t, "123.321")
	b, _ := expectToken(t, p, rmarsh.TokenFloat)
	if n, err := strconv.ParseFloat(string(b), 64); err != nil {
		t.Errorf("p.Float() err %s", err)
	} else if n != 123.321 {
		t.Errorf("p.Float() = %f, expected 123.321", n)
	}
	expectToken(t, p, rmarsh.TokenEOF)
}

func BenchmarkParserFloatSingleByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "1.to_f"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, _, _, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenFloat {
			b.Fatalf("Unexpected token %s", tok)
		}
	}
}

func BenchmarkParserFloatMultiByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "123.321"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, _, _, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenFloat {
			b.Fatalf("Unexpected token %s", tok)
		}
	}
}

func TestParserSymbol(t *testing.T) {
	p := parseFromRuby(t, ":test")
	b, _ := expectToken(t, p, rmarsh.TokenSymbol)
	str := string(b)
	if str != "test" {
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

		if tok, data, _, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenSymbol {
			b.Fatalf("Unexpected token %s", tok)
		} else if !bytes.Equal(data, exp) {
			b.Fatalf("%s != test", data)
		}
	}
}

func BenchmarkParserSymbolMultiByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, ":test"))
	p := rmarsh.NewParser(buf)
	exp := []byte("test")

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, data, _, err := p.Read(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenSymbol {
			b.Fatalf("Unexpected token %s", tok)
		} else if !bytes.Equal(data, exp) {
			b.Fatalf("%s != test", data)
		}
	}
}
