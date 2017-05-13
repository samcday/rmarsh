package rmarsh_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
)

var (
	rbEnc, rbDec       *exec.Cmd
	rbEncOut, rbDecOut *bufio.Scanner
	rbEncIn, rbDecIn   io.Writer

	streamDelim = []byte("$$END$$")
)

func scanStream(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) >= 7 {
		for i := 0; i <= len(data)-7; i++ {
			if bytes.Compare(data[i:i+7], streamDelim) == 0 {
				return i + 7, data[0:i], nil
			}
		}
	}
	return 0, nil, nil
}

func rbEncode(t testing.TB, payload string) []byte {
	if rbEnc == nil {
		rbEnc = exec.Command("ruby", "rb_encoder.rb")
		// Send stderr to top level so it's obvious if the Ruby script blew up somehow.
		rbEnc.Stderr = os.Stdout

		stdout, err := rbEnc.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := rbEnc.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := rbEnc.Start(); err != nil {
			t.Fatal(err)
		}

		rbEncOut = bufio.NewScanner(stdout)
		rbEncOut.Split(scanStream)
		rbEncIn = stdin
	}

	_, err := io.WriteString(rbEncIn, fmt.Sprintf("%s\n", payload))
	if err != nil {
		t.Fatal(err)
	}

	if !rbEncOut.Scan() {
		t.Fatal(rbEncOut.Err())
	}

	return rbEncOut.Bytes()
}

func rbDecode(t testing.TB, b []byte) string {
	if rbDec == nil {
		rbDec = exec.Command("ruby", "rb_decoder.rb")
		// Send stderr to top level so it's obvious if the Ruby script blew up somehow.
		rbDec.Stderr = os.Stdout

		stdout, err := rbDec.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := rbDec.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := rbDec.Start(); err != nil {
			t.Fatal(err)
		}

		rbDecIn = stdin
		rbDecOut = bufio.NewScanner(stdout)
	}

	if _, err := rbDecIn.Write(b); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(rbDecIn, "$$END$$"); err != nil {
		t.Fatal(err)
	}

	if !rbDecOut.Scan() {
		t.Fatalf("Error scanning output")
	}
	return rbDecOut.Text()
}

type cyclicReader struct {
	b   []byte
	off int
	sz  int
}

func (r *cyclicReader) Read(b []byte) (int, error) {
	n := copy(b, r.b[r.off:])
	r.off += n
	if r.off >= r.sz {
		r.off = 0
	}
	return n, nil
}

func newCyclicReader(b []byte) *cyclicReader {
	return &cyclicReader{
		b:   b,
		off: 0,
		sz:  len(b),
	}
}
