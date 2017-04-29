package rmarsh

import (
	"fmt"
	"io"
)

const (
	genStateStart = iota
	genStateTop
	genStateDone
)

var ErrGeneratorFinished = fmt.Errorf("Attempting to write value to a finished Marshal stream")

type genState struct {
	cnt  int
	pos  int
	prev *genState
}

type Generator struct {
	w  io.Writer
	c  int
	st *genState
}

func NewGenerator(w io.Writer) *Generator {
	return &Generator{w: w, st: &genState{cnt: 1}}
}

func (g *Generator) Nil() error {
	if err := g.checkState(); err != nil {
		return err
	}

	g.write([]byte{TYPE_NIL})

	g.advState()
	return nil
}

func (g *Generator) Bool(b bool) error {
	if err := g.checkState(); err != nil {
		return err
	}

	if b {
		g.write([]byte{TYPE_TRUE})
	} else {
		g.write([]byte{TYPE_FALSE})
	}

	g.advState()
	return nil
}

func (g *Generator) checkState() error {
	if g.st == nil {
		return ErrGeneratorFinished
	}

	// If we're in top level ctx and haven't written anything yet, then we
	// gotta write the magic.
	if g.st.pos == 0 && g.st.prev == nil {
		if err := g.write(magic); err != nil {
			return err
		}
	}

	return nil
}

func (g *Generator) advState() {
	g.st.pos++
	if g.st.pos == g.st.cnt {
		g.st = g.st.prev
	}
}

func (g *Generator) write(b []byte) error {
	l := len(b)
	if n, err := g.w.Write(b); err != nil {
		return err
	} else if n != l {
		return fmt.Errorf("I/O underflow %d != %d", n, l)
	}
	g.c += l
	return nil
}
