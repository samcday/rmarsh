package rmarsh

import (
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"

	"github.com/pkg/errors"
)

// ErrGeneratorFinished is the error returned when a value is written to a Marshal stream that has already completed.
var ErrGeneratorFinished = fmt.Errorf("Write on finished Marshal stream")

// ErrGeneratorOverflow is the error returned when a value is written past the end of a bounded structure such as an
// array, hash, ivar, struct, etc.
var ErrGeneratorOverflow = fmt.Errorf("Write past end of bounded array/hash/ivar")

// ErrNonSymbolValue is the error returned when anything other than a Symbol is written when a Symbol was expected to
// be the next value. This expectation is enforced when writing the keys of an ivar, struct and object.
var ErrNonSymbolValue = fmt.Errorf("Non Symbol value written when Symbol expected")

const (
	genStateGrowSize = 8 // Initial size + amount to grow state stack by
	symTblGrowSize   = 8
)

// Generator is a low-level streaming implementation of the Ruby Marshal 4.8 format.
type Generator struct {
	w  io.Writer
	c  int
	st genState

	buf  []byte
	bufn int

	symCount int
	symTbl   []string
}

// NewGenerator returns a new Generator that is ready to start writing out a Ruby Marshal stream. Generators are not
// thread safe, but can be reused for new Marshal streams by calling Reset().
func NewGenerator(w io.Writer) *Generator {
	gen := &Generator{
		w:   w,
		buf: make([]byte, 128),
	}
	gen.st.stack = make([]genStateItem, genStateGrowSize)
	gen.Reset(nil)
	return gen
}

// Reset restores the state of the Generator to an identity state, ready to write a new Marshal stream.
// If provided io.Writer is nil, the existing writer is used.
// Reusing Generators is encouraged, to recycle the internal structures that are allocated during generation.
func (gen *Generator) Reset(w io.Writer) {
	if w != nil {
		gen.w = w
	}

	gen.st.reset()

	gen.c = 0
	gen.symCount = 0

	gen.buf[0] = 0x04
	gen.buf[1] = 0x08
	gen.bufn = 2
}

// Nil writes the nil value to the Marshal stream.
func (gen *Generator) Nil() error {
	if err := gen.checkState(false, 1); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeNil
	gen.bufn++
	return gen.writeAdv()
}

// Bool writes a true/false value to the Marshal stream.
func (gen *Generator) Bool(b bool) error {
	if err := gen.checkState(false, 1); err != nil {
		return err
	}

	if b {
		gen.buf[gen.bufn] = typeTrue
	} else {
		gen.buf[gen.bufn] = typeFalse
	}
	gen.bufn++

	return gen.writeAdv()
}

// Fixnum writes a signed/unsigned number to the Marshal stream.
// Ruby has bounds on what can be encoded as a fixnum, those bounds are less than the range an int64 can cover. If the
// provided number overflows it will be encoded as a Bignum instead.
func (gen *Generator) Fixnum(n int64) error {
	if n < fixnumMin || n > fixnumMax {
		var bign big.Int
		bign.SetInt64(n)
		return gen.Bignum(&bign)
	}

	if err := gen.checkState(false, fixnumMaxBytes+1); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeFixnum
	gen.bufn++
	gen.encodeLong(n)
	return gen.writeAdv()
}

// Bignum writes a big.Int value to the Marshal stream.
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

	if err := gen.checkState(false, 2+fixnumMaxBytes+sz); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeBignum
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

// Writes given symbol (or a symlink if symbol already written before) but does not check state or advance write state.
// Intended to be used where symbols are embedded in other value types (like StartObject)
func (gen *Generator) writeSym(sym string) {
	if l := len(gen.symTbl); l == 0 || l == gen.symCount {
		newTbl := make([]string, l+symTblGrowSize)
		copy(newTbl, gen.symTbl)
		gen.symTbl = newTbl
	}

	for i := 0; i < gen.symCount; i++ {
		if gen.symTbl[i] == sym {
			gen.buf[gen.bufn] = typeSymlink
			gen.bufn++
			gen.encodeLong(int64(i))
			return
		}
	}

	l := len(sym)
	gen.buf[gen.bufn] = typeSymbol
	gen.bufn++
	gen.encodeLong(int64(l))
	copy(gen.buf[gen.bufn:], sym)
	gen.bufn += l

	gen.symTbl[gen.symCount] = sym
	gen.symCount++
}

// Symbol writes a Ruby symbol value to the Marshal stream.
// The generator automatically handles writing "symlink" values to the stream if the symbol name has already been
// written in this Marshal stream.
func (gen *Generator) Symbol(sym string) error {
	if err := gen.checkState(true, 1+fixnumMaxBytes+len(sym)); err != nil {
		return err
	}

	gen.writeSym(sym)

	return gen.writeAdv()
}

// Writes given string to stream but does not check state or advance it.
func (gen *Generator) writeString(str string) {
	l := len(str)
	gen.encodeLong(int64(l))
	copy(gen.buf[gen.bufn:], str)
	gen.bufn += l
}

// String writes the given string to the Marshal stream.
// Be sure to call StartIVar first if you need to include encoding information.
func (gen *Generator) String(str string) error {
	l := len(str)
	if err := gen.checkState(false, 1+fixnumMaxBytes+l); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeString
	gen.bufn++
	gen.writeString(str)

	return gen.writeAdv()
}

// Float writes the given float value to the Marshal stream.
func (gen *Generator) Float(f float64) error {
	// String repr of a float64 will never exceed 30 chars.
	// That also means the len encoded long will never exceed 1 byte.
	if err := gen.checkState(false, 1+1+30); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeFloat
	gen.bufn++

	// We pass a 0 len slice of our scratch buffer to append float.
	// This ensures it makes no allocation since the append() calls it makes
	// will just consume existing capacity.
	b := strconv.AppendFloat(gen.buf[gen.bufn+1:gen.bufn+1:len(gen.buf)], f, 'g', -1, 64)
	l := len(b)

	gen.encodeLong(int64(l))
	gen.bufn += l

	return gen.writeAdv()
}

// StartArray begins writing an array to the Marshal stream.
// When all elements are written, EndArray() must be called.
func (gen *Generator) StartArray(l int) error {
	if err := gen.checkState(false, 1+fixnumMaxBytes); err != nil {
		return err
	}
	gen.buf[gen.bufn] = typeArray
	gen.bufn++
	gen.encodeLong(int64(l))

	gen.st.push(genStArr, l)
	return nil
}

// EndArray completes the array currently being generated.
func (gen *Generator) EndArray() error {
	if gen.st.sz == 0 || gen.st.cur.typ != genStArr {
		return errors.New("EndArray() called outside of context of array")
	}
	if gen.st.cur.pos != gen.st.cur.cnt {
		return errors.Errorf("EndArray() called prematurely, %d of %d elems written", gen.st.cur.pos, gen.st.cur.cnt)
	}
	gen.st.pop()

	return gen.writeAdv()
}

// StartHash behins writing a hash to the Marshal stream.
// When all elements are written, EndHash() must be called.
func (gen *Generator) StartHash(l int) error {
	if err := gen.checkState(false, 1+fixnumMaxBytes); err != nil {
		return err
	}
	gen.buf[gen.bufn] = typeHash
	gen.bufn++
	gen.encodeLong(int64(l))

	gen.st.push(genStHash, l*2)
	return nil
}

// EndHash completes the hash currently being generated.
func (gen *Generator) EndHash() error {
	if gen.st.sz == 0 || gen.st.cur.typ != genStHash {
		return errors.New("EndHash() called outside of context of hash")
	}
	if gen.st.cur.pos != gen.st.cur.cnt {
		return errors.Errorf("EndHash() called prematurely, %d of %d elems written", gen.st.cur.pos, gen.st.cur.cnt)
	}
	gen.st.pop()

	return gen.writeAdv()
}

// Class writes a Ruby class reference to the Marshal stream.
func (gen *Generator) Class(name string) error {
	l := len(name)
	if err := gen.checkState(false, 1+fixnumMaxBytes+l); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeClass
	gen.bufn++
	gen.encodeLong(int64(l))
	copy(gen.buf[gen.bufn:], name)
	gen.bufn += l

	return gen.writeAdv()
}

// Module writes a Ruby module reference to the Marshal stream.
func (gen *Generator) Module(name string) error {
	l := len(name)
	if err := gen.checkState(false, 1+fixnumMaxBytes+l); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeModule
	gen.bufn++
	gen.encodeLong(int64(l))
	copy(gen.buf[gen.bufn:], name)
	gen.bufn += l

	return gen.writeAdv()
}

// StartIVar begins writing an IVar to the Marshal stream.
// The next value can be anything that is permitted to have instance variables. The write after that MUST be a Symbol(),
// and each second write after that must be a symbol until l variables have been written and EndIVar() has been called.
func (gen *Generator) StartIVar(l int) error {
	if err := gen.checkState(false, 1+fixnumMaxBytes); err != nil {
		return err
	}
	gen.buf[gen.bufn] = typeIvar
	gen.bufn++

	gen.st.push(genStIVar, l*2)

	// We move the current pos on the ivar to -1, since the next write does not count toward the number of instance
	// vars to be written.
	gen.st.cur.pos = -1
	return nil
}

// EndIVar completes the ivar currently being generated.
func (gen *Generator) EndIVar() error {
	if gen.st.sz == 0 || gen.st.cur.typ != genStIVar {
		return errors.New("EndIVar() called outside of context of ivar")
	}
	if gen.st.cur.pos != gen.st.cur.cnt {
		return errors.Errorf("EndIVar() called prematurely, %d of %d elems written", gen.st.cur.pos, gen.st.cur.cnt)
	}
	gen.st.pop()

	return gen.writeAdv()
}

// StartObject begins writing an object with provided class name to the Marshal stream.
// The next calls must be l pairs of Symbol+<any> calls.
func (gen *Generator) StartObject(name string, l int) error {
	// Need enough space for the two type bytes (object + symbol), the encoded length of the symbol, and the encoded
	// length of the object variables.
	if err := gen.checkState(false, 1+1+fixnumMaxBytes+len(name)+fixnumMaxBytes); err != nil {
		return err
	}
	gen.buf[gen.bufn] = typeObject
	gen.bufn++

	gen.writeSym(name)

	gen.encodeLong(int64(l))

	gen.st.push(genStObj, l*2)
	return nil
}

// EndObject completes the object currently being generated.
func (gen *Generator) EndObject() error {
	if gen.st.sz == 0 || gen.st.cur.typ != genStObj {
		return errors.New("EndObject() called outside of context of object")
	}
	if gen.st.cur.pos != gen.st.cur.cnt {
		return errors.Errorf("EndObject() called prematurely, %d of %d elems written", gen.st.cur.pos, gen.st.cur.cnt)
	}
	gen.st.pop()

	return gen.writeAdv()
}

// StartUserMarshalled begins writing a user marshalled object with provided class name to the Marshal stream.
// User marshalled objects are Ruby objects that have a marshal_load function.
// The next call can be any value type.
// UserMarshalled object state must be completed with a call to EndUserMarshalled().
func (gen *Generator) StartUserMarshalled(name string) error {
	if err := gen.checkState(false, 1+1+fixnumMaxBytes+len(name)); err != nil {
		return err
	}
	gen.buf[gen.bufn] = typeUsrMarshal
	gen.bufn++

	gen.writeSym(name)

	gen.st.push(genStUsrMarsh, 1)
	return nil
}

// EndUserMarshalled completes the user marshalled object currently being written.
func (gen *Generator) EndUserMarshalled() error {
	if gen.st.sz == 0 || gen.st.cur.typ != genStUsrMarsh {
		return errors.New("EndUserMarshalled() called outside of context of user marshaalled object")
	}
	if gen.st.cur.pos != gen.st.cur.cnt {
		return errors.Errorf("EndUserMarshalled() called prematurely, data value not yet written")
	}
	gen.st.pop()

	return gen.writeAdv()
}

// UserDefinedObject writes a user defined object with the given name and data string to the Marshal stream.
// User defined objects are Ruby objects that have a _load function that accepts a string and construct the object.
// If you need to specify encoding on the data string, open an IVar context with StartIVar before calling this method.
func (gen *Generator) UserDefinedObject(name, data string) error {
	if err := gen.checkState(false, 1+fixnumMaxBytes+len(name)+fixnumMaxBytes+len(data)); err != nil {
		return err
	}
	gen.buf[gen.bufn] = typeUsrDef
	gen.bufn++

	gen.writeSym(name)

	gen.encodeLong(int64(len(data)))
	copy(gen.buf[gen.bufn:], data)
	gen.bufn += len(data)

	return gen.writeAdv()
}

// Regexp writes regular expression with given text + flags to the Marshal stream.
// Look at REGEXP_* flags for valid ones.
// To set encoding on the regexp obj, wrap it in an IVar.
func (gen *Generator) Regexp(expr string, flags byte) error {
	if err := gen.checkState(false, 1+fixnumMaxBytes+len(expr)+1); err != nil {
		return err
	}

	gen.buf[gen.bufn] = typeRegExp
	gen.bufn++
	gen.writeString(expr)
	gen.buf[gen.bufn] = flags
	gen.bufn++

	return gen.writeAdv()
}

// StartStruct begins writing a struct value to the Marshal stream.
// l pairs of Symbol + values must be written after this call, and then punctuated with a call to EndStruct
func (gen *Generator) StartStruct(name string, l int) error {
	if err := gen.checkState(false, 1+1+fixnumMaxBytes+len(name)+fixnumMaxBytes); err != nil {
		return err
	}
	gen.buf[gen.bufn] = typeStruct
	gen.bufn++

	gen.writeSym(name)

	gen.encodeLong(int64(l))

	gen.st.push(genStStruct, l*2)
	return nil
}

// EndStruct completes the struct currently being generated.
func (gen *Generator) EndStruct() error {
	if gen.st.sz == 0 || gen.st.cur.typ != genStStruct {
		return errors.New("EndStruct() called outside of context of struct")
	}
	if gen.st.cur.pos != gen.st.cur.cnt {
		return errors.Errorf("EndStruct() called prematurely, %d of %d elems written", gen.st.cur.pos, gen.st.cur.cnt)
	}
	gen.st.pop()

	return gen.writeAdv()
}

func (gen *Generator) checkState(isSym bool, sz int) error {
	// Make sure we're not writing past bounds.
	if gen.st.cur.pos == gen.st.cur.cnt {
		if gen.st.sz == 1 {
			return ErrGeneratorFinished
		}
		return ErrGeneratorOverflow
	}

	if gen.st.cur.typ == genStIVar && gen.st.cur.pos == -1 {
		// We're gonna be writing the IVar length after this next value during writeAdv.
		// So, make sure the buffer size will be big enough to accommodate that also.
		sz += fixnumMaxBytes
	}

	// If we're presently writing an IVar/object, then make sure the even numbered elements are Symbols.
	if gen.st.cur.typ == genStIVar || gen.st.cur.typ == genStObj || gen.st.cur.typ == genStStruct {
		if gen.st.cur.pos&1 == 0 && !isSym {
			return ErrNonSymbolValue
		}
	}

	if len(gen.buf) < gen.bufn+sz {
		newBuf := make([]byte, gen.bufn+sz)
		if gen.bufn > 0 {
			copy(newBuf, gen.buf)
		}
		gen.buf = newBuf
	}

	return nil
}

// Writes the given bytes if provided, then advances current state of the generator.
func (gen *Generator) writeAdv() error {
	gen.st.cur.pos++

	if gen.st.cur.typ == genStIVar && gen.st.cur.pos == 0 {
		// If we just reached pos 0 for the current ivar, it means we wrote the main value and we're about to start
		// on the instnace vars themselves. We need to write out the instance var count now.
		gen.encodeLong(int64(gen.st.cur.cnt / 2))
	}

	// If we've just finished writing out the last value, then we make sure to flush anything remaining.
	// Otherwise, we let things accumulate in our small buffer between calls to reduce the number of writes.
	if gen.bufn > 0 && gen.st.cur.pos == gen.st.cur.cnt && gen.st.sz == 1 {
		if _, err := gen.w.Write(gen.buf[:gen.bufn]); err != nil {
			return err
		}
		gen.c += gen.bufn
		gen.bufn = 0
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

const (
	genStTop = iota
	genStArr
	genStHash
	genStIVar
	genStObj
	genStUsrMarsh
	genStStruct
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
	cap   int
	sz    int
	cur   *genStateItem
}

// Resets generator state back to initial state (which is ready for a new
// top level value to be written)
func (st *genState) reset() {
	st.sz = 1
	st.cur = &st.stack[0]
	st.cur.cnt = 1
	st.cur.pos = 0
	st.cur.typ = genStTop
}

func (st *genState) push(typ uint8, cnt int) {
	if st.sz == len(st.stack) {
		newSt := make([]genStateItem, len(st.stack)+genStateGrowSize)
		copy(newSt, st.stack)
		st.stack = newSt
	}

	st.cur = &st.stack[st.sz]
	st.cur.reset(cnt, typ)
	st.sz++
}

func (st *genState) pop() {
	st.sz--
	if st.sz > 0 {
		st.cur = &st.stack[st.sz-1]
	} else {
		st.cur = nil
	}
}
