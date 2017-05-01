package rmarsh

import (
	"fmt"
	"io"
	"math"
	"math/big"
)

var ErrGeneratorFinished = fmt.Errorf("Attempting to write value to a finished Marshal stream")

type genState struct {
	cnt int
	pos int
}

type Generator struct {
	w    io.Writer
	c    int
	st   []genState
	stSz int

	buf  []byte
	bufn int
}

func NewGenerator(w io.Writer) *Generator {
	gen := &Generator{w: w, stSz: 1, st: make([]genState, 8), buf: make([]byte, 128)}
	gen.st[0].cnt = 1
	return gen
}

func (gen *Generator) Reset() {
	gen.c = 0
	gen.stSz = 1
	gen.st[0].cnt = 1
	gen.st[0].pos = 0
}

// Nil writes the nil value to the stream
func (gen *Generator) Nil() error {
	if err := gen.checkState(); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_NIL
	gen.bufn++
	return gen.writeAdv()
}

// Bool writes a true/false value to the stream
func (gen *Generator) Bool(b bool) error {
	if err := gen.checkState(); err != nil {
		return err
	}

	if b {
		gen.buf[gen.bufn] = TYPE_TRUE
	} else {
		gen.buf[gen.bufn] = TYPE_FALSE
	}
	gen.bufn++

	return gen.writeAdv()
}

// Fixnum writes a signed/unsigned number to the stream.
// Ruby has bounds on what can be encoded as a fixnum, those bounds are
// less than the range an int64 can cover. If the provided number overflows
// it will be encoded as a Bignum instead.
func (gen *Generator) Fixnum(n int64) error {
	if n < fixnumMin || n > fixnumMax {
		var bign big.Int
		bign.SetInt64(n)
		return gen.Bignum(&bign)
	}

	if err := gen.checkState(); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_FIXNUM
	gen.bufn++
	gen.encodeLong(n)
	return gen.writeAdv()
}

func (gen *Generator) Bignum(b *big.Int) error {
	if err := gen.checkState(); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_BIGNUM
	gen.bufn++
	if b.Sign() < 0 {
		gen.buf[gen.bufn] = '-'
	} else {
		gen.buf[gen.bufn] = '+'
	}
	gen.bufn++

	// We don't use big.Int.Bytes() for two reasons:
	// 1) it's an unnecessary buffer allocation which can't be avoided
	//    (can't provide an existing buffer for big.Int to write into)
	// 2) the returned buffer is big-endian but Ruby expects le.
	bits := b.Bits()
	l := len(bits)

	// Calculate the number of bytes we'll be writing.
	sz := 0
	for i, d := range bits {
		for j := 0; j < _S; j++ {
			sz++
			d >>= 8
			if d == 0 && i == l-1 {
				break
			}
		}
	}

	// bignum is encoded as a series of shorts. If we have an uneven number of
	// bytes we gotta pad it out.
	if sz&1 == 1 {
		sz++
	}

	gen.encodeLong(int64(math.Ceil(float64(sz) / 2)))

	w := 0
	for i, d := range bits {
		for j := 0; j < _S; j++ {
			gen.buf[gen.bufn] = byte(d)
			gen.bufn++
			w++
			d >>= 8
			if d == 0 && i == l-1 {
				break
			}
		}
	}

	for w < sz {
		gen.buf[gen.bufn] = 0
		gen.bufn++
		w++
	}

	return gen.writeAdv()
}

func (gen *Generator) checkState() error {
	if gen.stSz == 0 {
		return ErrGeneratorFinished
	}

	// If we're in top level ctx and haven't written anything yet, then we
	// gotta write the magic.
	cst := gen.st[gen.stSz-1]
	if cst.pos == 0 && gen.stSz == 1 {
		gen.buf[0] = 0x04
		gen.buf[1] = 0x08
		gen.bufn += 2
	}

	return nil
}

// Writes the given bytes if provided, then advances current state of the generator.
func (gen *Generator) writeAdv() error {
	if gen.bufn > 0 {
		if err := gen.write(gen.buf[:gen.bufn]); err != nil {
			return err
		}
		gen.bufn = 0
	}

	cst := gen.st[gen.stSz-1]
	cst.pos++
	// If we've finished with the current ctx, we pop it
	if cst.pos == cst.cnt {
		gen.stSz--
	}
	return nil
}

func (gen *Generator) encodeLong(n int64) {
	if n == 0 {
		gen.buf[gen.bufn] = 0
		gen.bufn++
		return
	} else if 0 < n && n < 0x7B {
		gen.buf[gen.bufn] = byte(n + 5)
		gen.bufn++
		return
	} else if -0x7C < n && n < 0 {
		gen.buf[gen.bufn] = byte((n - 5) & 0xFF)
		gen.bufn++
		return
	}

	for i := 1; i < 5; i++ {
		gen.buf[gen.bufn+i] = byte(n & 0xFF)
		n = n >> 8
		if n == 0 {
			gen.buf[gen.bufn] = byte(i)
			gen.bufn += i + 1
			return
		}
		if n == -1 {
			gen.buf[gen.bufn] = byte(-i)
			gen.bufn += i + 1
			return
		}
	}
	panic("Shouldn't *ever* reach here")
}

func (gen *Generator) write(b []byte) error {
	l := len(b)
	if n, err := gen.w.Write(b); err != nil {
		return err
	} else if n != l {
		return fmt.Errorf("I/O underflow %d != %d", n, l)
	}
	gen.c += l
	return nil
}
