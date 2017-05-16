package rmarsh

import (
	"fmt"
	"io"
	"reflect"
	"sync"
)

// A Decoder decodes a Ruby Marshal stream into concrete Golang structures.
type Decoder struct {
	p *Parser

	curToken Token
}

// NewDecoder builds a new Decoder that uses given Parser to decode a Ruby Marshal stream.
func NewDecoder(p *Parser) *Decoder {
	return &Decoder{p: p}
}

// ReadValue will consume a full Ruby Marshal stream from the given io.Reader and return a fully decoded Golang object.
func ReadValue(r io.Reader, val interface{}) error {
	return NewDecoder(NewParser(r)).Decode(val)
}

func (d *Decoder) Decode(val interface{}) error {
	v := reflect.ValueOf(val)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("Invalid decode target %T, did you forget to pass a pointer?", val)
	}

	return d.valueDecoder(v.Elem())(d, v.Elem())
}

func (d *Decoder) nextToken() (Token, error) {
	if d.curToken != tokenStart {
		tok := d.curToken
		d.curToken = tokenStart
		return tok, nil
	}
	return d.p.Next()
}

type decoderFunc func(*Decoder, reflect.Value) error

var decCache struct {
	sync.RWMutex
	m map[reflect.Type]decoderFunc
}

func (d *Decoder) valueDecoder(v reflect.Value) decoderFunc {
	return d.typeDecoder(v.Type())
}

func (d *Decoder) typeDecoder(t reflect.Type) decoderFunc {
	decCache.RLock()
	dec := decCache.m[t]
	decCache.RUnlock()
	if dec != nil {
		return dec
	}

	decCache.Lock()
	defer decCache.Unlock()
	if decCache.m == nil {
		decCache.m = make(map[reflect.Type]decoderFunc)
	}
	decCache.m[t] = newTypeDecoder(t)
	return decCache.m[t]
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

func boolDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
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
func intDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenFixnum:
		n, err := d.p.Int()
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

func uintDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenFixnum:
		n, err := d.p.Int()
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

func floatDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenFloat:
		f, err := d.p.Float()
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

func stringDecoder(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
	if err != nil {
		return err
	}

	switch tok {
	case TokenString, TokenSymbol:
		str, err := d.p.Text()
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

func (ptrDec *ptrDecoder) decode(d *Decoder, v reflect.Value) error {
	tok, err := d.nextToken()
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
	d.curToken = tok

	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return ptrDec.elemDec(d, v.Elem())
}

func newPtrDecoder(t reflect.Type) decoderFunc {
	dec := &ptrDecoder{newTypeDecoder(t.Elem())}
	return dec.decode
}

func unsupportedTypeDecoder(d *Decoder, v reflect.Value) error {
	return fmt.Errorf("unsupported type %s", v.Type())
}
