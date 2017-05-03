package rmarsh

import (
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
)

var ErrGeneratorFinished = fmt.Errorf("Attempting to write value to a finished Marshal stream")

const (
	genStateGrowSize = 8 // Initial size + amount to grow state stack by
)

type Generator struct {
	w  io.Writer
	c  int
	st genState

	buf  []byte
	bufn int

	symCount int
	symTbl   []string
}

func NewGenerator(w io.Writer) *Generator {
	gen := &Generator{
		w:   w,
		buf: make([]byte, 128),
	}
	gen.st.stack = make([]genStateItem, genStateGrowSize)
	gen.st.reset()
	return gen
}

func (gen *Generator) Reset() {
	gen.c = 0
	gen.st.reset()
	gen.symCount = 0
}

// Nil writes the nil value to the stream
func (gen *Generator) Nil() error {
	if err := gen.checkState(1); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_NIL
	gen.bufn++
	return gen.writeAdv()
}

// Bool writes a true/false value to the stream
func (gen *Generator) Bool(b bool) error {
	if err := gen.checkState(1); err != nil {
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

	if err := gen.checkState(fixnumMaxBytes + 1); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_FIXNUM
	gen.bufn++
	gen.encodeLong(n)
	return gen.writeAdv()
}

func (gen *Generator) Bignum(b *big.Int) error {
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

	if err := gen.checkState(2 + fixnumMaxBytes + sz); err != nil {
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

func (gen *Generator) Symbol(sym string) error {
	if l := len(gen.symTbl); l == 0 || l == gen.symCount {
		newTbl := make([]string, l+symTblGrowSize)
		copy(newTbl, gen.symTbl)
		gen.symTbl = newTbl
	}

	for i := 0; i < gen.symCount; i++ {
		if gen.symTbl[i] == sym {
			if err := gen.checkState(1 + fixnumMaxBytes); err != nil {
				return err
			}
			gen.buf[gen.bufn] = TYPE_SYMLINK
			gen.bufn++
			gen.encodeLong(int64(i))
			return gen.writeAdv()
		}
	}

	l := len(sym)

	if err := gen.checkState(1 + fixnumMaxBytes + l); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_SYMBOL
	gen.bufn++

	gen.encodeLong(int64(l))
	copy(gen.buf[gen.bufn:], sym)
	gen.bufn += l

	gen.symTbl[gen.symCount] = sym
	gen.symCount++

	return gen.writeAdv()
}

func (gen *Generator) String(str string) error {
	l := len(str)
	if err := gen.checkState(1 + fixnumMaxBytes + l); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_STRING
	gen.bufn++
	gen.encodeLong(int64(l))
	copy(gen.buf[gen.bufn:], str)
	gen.bufn += l

	return gen.writeAdv()
}

func (gen *Generator) Float(f float64) error {
	str := strconv.FormatFloat(f, 'g', -1, 64)
	l := len(str)

	if err := gen.checkState(1 + fixnumMaxBytes + l); err != nil {
		return err
	}

	gen.buf[gen.bufn] = TYPE_FLOAT
	gen.bufn++
	gen.encodeLong(int64(l))
	copy(gen.buf[gen.bufn:], str)
	gen.bufn += l
	return gen.writeAdv()
}

func (gen *Generator) checkState(sz int) error {
	if gen.st.sz == 0 {
		return ErrGeneratorFinished
	}

	if len(gen.buf) < sz {
		gen.buf = make([]byte, sz)
	}

	// If we're in top level ctx and haven't written anything yet, then we
	// gotta write the magic.
	if gen.st.cur.pos == 0 && gen.st.sz == 1 {
		gen.buf[0] = 0x04
		gen.buf[1] = 0x08
		gen.bufn += 2
	}

	return nil
}

// Writes the given bytes if provided, then advances current state of the generator.
func (gen *Generator) writeAdv() error {
	if gen.bufn > 0 {
		if _, err := gen.w.Write(gen.buf[:gen.bufn]); err != nil {
			return err
		}
		gen.c += gen.bufn
		gen.bufn = 0
	}

	gen.st.adv()
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

const (
	genStTop = iota
	genStArr
)

type genStateItem struct {
	cnt int
	pos int
	typ uint8
}

func (st *genStateItem) reset(sz int, typ uint8) {
	st.cnt = sz
	st.pos = 0
	st.typ = typ
}

type genState struct {
	stack []genStateItem
	sz    int
	cur   *genStateItem
}

// Resets generator state back to initial state (which is ready for a new
// top level value to be written)
func (st *genState) reset() {
	st.sz = 1
	st.cur = &st.stack[0]
	st.stack[0].reset(1, genStTop)
}

func (st *genState) adv() {
	st.cur.pos++
	// If we've finished with the current ctx, we pop it
	if st.cur.pos == st.cur.cnt {
		st.sz--
	}
}
