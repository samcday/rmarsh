package rmarsh

import (
	"fmt"
	"reflect"
)

type decodeContext struct {
	curToken Token
}

func (ctx *decodeContext) nextToken(p *Parser) (Token, error) {
	if ctx.curToken != tokenStart {
		tok := ctx.curToken
		ctx.curToken = tokenStart
		return tok, nil
	}
	return p.Next()
}

type decoderFunc func(*Parser, reflect.Value, *decodeContext) error

func (m *Mapper) valueDecoder(v reflect.Value) decoderFunc {
	return m.typeDecoder(v.Type())
}

func (m *Mapper) typeDecoder(t reflect.Type) decoderFunc {
	m.decLock.RLock()
	dec := m.decCache[t]
	m.decLock.RUnlock()
	if dec != nil {
		return dec
	}

	m.decLock.Lock()
	defer m.decLock.Unlock()
	if m.decCache == nil {
		m.decCache = make(map[reflect.Type]decoderFunc)
	}
	m.decCache[t] = newTypeDecoder(t)
	return m.decCache[t]
}

func newTypeDecoder(t reflect.Type) decoderFunc {
	switch t.Kind() {
	case reflect.Bool:
		return boolDecoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intDecoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintDecoder
	case reflect.Float32, reflect.Float64:
		return floatDecoder
	case reflect.String:
		return stringDecoder
	case reflect.Ptr:
		return newPtrDecoder(t)
	}
	return unsupportedTypeDecoder
}

func boolDecoder(p *Parser, v reflect.Value, ctx *decodeContext) error {
	tok, err := ctx.nextToken(p)
	if err != nil {
		return err
	}

	switch tok {
	case TokenTrue, TokenFalse:
		v.SetBool(tok == TokenTrue)
		return nil
	// TODO: support other types and coerce them to something bool-y?
	default:
		// TODO: build a path
		return fmt.Errorf("Unexpected token %v encountered while decoding bool", tok)
	}
}
func intDecoder(p *Parser, v reflect.Value, ctx *decodeContext) error {
	tok, err := ctx.nextToken(p)
	if err != nil {
		return err
	}

	switch tok {
	case TokenFixnum:
		n, err := p.Int()
		if err != nil {
			return err
		}
		if v.OverflowInt(n) {
			return fmt.Errorf("Decoded int %d exceeds maximum width of %s", n, v.Type())
		}
		v.SetInt(n)
		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding int", tok)
	}
}

func uintDecoder(p *Parser, v reflect.Value, ctx *decodeContext) error {
	tok, err := ctx.nextToken(p)
	if err != nil {
		return err
	}

	switch tok {
	case TokenFixnum:
		n, err := p.Int()
		if err != nil {
			return err
		}
		un := uint64(n)
		if v.OverflowUint(un) {
			return fmt.Errorf("Decoded uint %d exceeds maximum width of %s", n, v.Type())
		}
		v.SetUint(un)
		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding uint", tok)
	}
}

func floatDecoder(p *Parser, v reflect.Value, ctx *decodeContext) error {
	tok, err := ctx.nextToken(p)
	if err != nil {
		return err
	}

	switch tok {
	case TokenFloat:
		f, err := p.Float()
		if err != nil {
			return err
		}
		if v.OverflowFloat(f) {
			return fmt.Errorf("Decoded float %f exceeds maximum width of %s", f, v.Type())
		}
		v.SetFloat(f)
		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding float", tok)
	}
}

func stringDecoder(p *Parser, v reflect.Value, ctx *decodeContext) error {
	tok, err := ctx.nextToken(p)
	if err != nil {
		return err
	}

	switch tok {
	case TokenString, TokenSymbol:
		str, err := p.Text()
		if err != nil {
			return err
		}
		v.SetString(str)
		return nil
	default:
		return fmt.Errorf("Unexpected token %v encountered while decoding string", tok)
	}
}

type ptrDecoder struct {
	elemDec decoderFunc
}

func (d *ptrDecoder) decode(p *Parser, v reflect.Value, ctx *decodeContext) error {
	tok, err := ctx.nextToken(p)
	if err != nil {
		return err
	}

	// If the token is nil, then we nil the ptr and move on.
	if tok == TokenNil {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	// TODO: if the token is a link, we dig up the cached reference and use that.

	// Push the token back and decode against resolved ptr.
	ctx.curToken = tok

	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return d.elemDec(p, v.Elem(), ctx)
}

func newPtrDecoder(t reflect.Type) decoderFunc {
	dec := &ptrDecoder{newTypeDecoder(t.Elem())}
	return dec.decode
}

func unsupportedTypeDecoder(p *Parser, v reflect.Value, ctx *decodeContext) error {
	return fmt.Errorf("unsupported type %s", v.Type())
}
