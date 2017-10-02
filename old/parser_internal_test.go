package rmarsh

import (
	"bytes"
	"testing"
)

func TestParserRngTblGrow(t *testing.T) {
	var tbl rngTbl

	tbl.add(rng{})
	if len(tbl) != 1 {
		t.Fatalf("len(tbl) != 1 == %d", len(tbl))
	}
	if cap(tbl) != rngTblInitSz {
		t.Fatalf("cap(tbl) != rngTblInitSz == %d", cap(tbl))
	}

	for i := 0; i < rngTblInitSz; i++ {
		tbl.add(rng{})
	}

	if cap(tbl) != rngTblInitSz*2 {
		t.Fatalf("cap(tbl) != rngTblInitSz*2 == %d", cap(tbl))
	}
}

func TestParserReset(t *testing.T) {
	var b1, b2 bytes.Buffer
	p := NewParser(&b1)

	p.Reset(&b2)
	if p.r != &b2 {
		t.Fatalf("p.r == %v, not %v", p.r, b2)
	}

	p.Reset(nil)
	if p.r != &b2 {
		t.Fatalf("p.r == %v, not %v", p.r, b2)
	}
}

func TestParserReplayRecursive(t *testing.T) {
	p := Parser{lnkID: 1}
	if _, err := p.Replay(1); err.Error() != "Object ID 1 is already being replayed by this Parser" {
		t.Fatalf("Unexpected err %s", err)
	}

	p2 := Parser{parent: &p, lnkID: 123}
	if _, err := p2.Replay(1); err.Error() != "Object ID 1 is already being replayed by this Parser" {
		t.Fatalf("Unexpected err %s", err)
	}
}
