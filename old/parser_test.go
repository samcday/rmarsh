

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
	if _, err := p.Float(); err == nil || err.Error() != `Float() called on incorrect token "TokenTrue"` {
		t.Errorf("p.Float() unexpected err %s", err)
	}
}

func BenchmarkParserBignum(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, "0xDEADCAFEBEEF"))
	p := rmarsh.NewParser(buf)

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenBignum {
			b.Fatalf("Unexpected token %s", tok)
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

		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenSymbol {
			b.Fatalf("Unexpected token %s", tok)
		}
		if !bytes.Equal(p.UnsafeBytes(), exp) {
			b.Fatalf("%s != test", p.UnsafeBytes())
		}
	}
}

func BenchmarkParserSymbolMultiByte(b *testing.B) {
	buf := newCyclicReader(rbEncode(b, ":test"))
	p := rmarsh.NewParser(buf)
	exp := []byte("test")

	for i := 0; i < b.N; i++ {
		p.Reset(nil)

		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenSymbol {
			b.Fatalf("Unexpected token %s", tok)
		}
		if !bytes.Equal(p.UnsafeBytes(), exp) {
			b.Fatalf("%s != test", p.UnsafeBytes())
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

		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenString {
			b.Fatalf("Unexpected token %s", tok)
		}
		if !bytes.Equal(p.UnsafeBytes(), exp) {
			b.Fatalf("%s != test", p.UnsafeBytes())
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

		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenStartArray {
			b.Fatalf("Unexpected token %s", tok)
		}
		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenEndArray {
			b.Fatalf("Unexpected token %s", tok)
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

		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenStartArray {
			b.Fatalf("Unexpected token %s", tok)
		}
		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenSymbol {
			b.Fatalf("Unexpected token %s", tok)
		}
		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenSymbol {
			b.Fatalf("Unexpected token %s", tok)
		}
		if !bytes.Equal(p.UnsafeBytes(), exp) {
			b.Fatalf("%s != test", p.UnsafeBytes())
		}
		if tok, err := p.Next(); err != nil {
			b.Fatal(err)
		} else if tok != rmarsh.TokenEndArray {
			b.Fatalf("Unexpected token %s", tok)
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
		t.Errorf("p.Len() = %d, expected 1", p.Len())
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
		t.Errorf("p.Len() = %d, expected 1", p.Len())
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
		t.Errorf("p.Len() = %d, expected 1", p.Len())
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
	if id := p.LinkID(); id != 0 {
		t.Errorf("p.LinkID() = %d != 0", id)
	}
	expectToken(t, p, rmarsh.TokenFloat)
	if id := p.LinkID(); id != 1 {
		t.Errorf("p.LinkID() = %d != 1", id)
	}
	expectToken(t, p, rmarsh.TokenLink)
	if id := p.LinkID(); id != 1 {
		t.Errorf("p.LinkID() = %d != 1", id)
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

func TestParserUsrMarshaal(t *testing.T) {
	p := parseFromRuby(t, `Gem::Version.new('1.2.3')`)

	expectToken(t, p, rmarsh.TokenUsrMarshal)
	expectToken(t, p, rmarsh.TokenSymbol)
	if str, err := p.Text(); err != nil {
		t.Fatal(err)
	} else if str != "Gem::Version" {
		t.Errorf("p.Text() = %s, expected @test", str)
	}
	expectToken(t, p, rmarsh.TokenStartArray)
	expectToken(t, p, rmarsh.TokenStartIVar)
	expectToken(t, p, rmarsh.TokenString)
	expectToken(t, p, rmarsh.TokenIVarProps)
	expectToken(t, p, rmarsh.TokenSymbol)
	expectToken(t, p, rmarsh.TokenTrue)
	expectToken(t, p, rmarsh.TokenEndIVar)
	expectToken(t, p, rmarsh.TokenEndArray)
	expectToken(t, p, rmarsh.TokenEOF)
}
