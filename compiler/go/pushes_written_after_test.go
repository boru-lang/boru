package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestPushesWrittenAfter pins the placing-apply screen's position rule
// (markTailPlacements, NUR336): every push after the apply must carry a
// source position after the apply's own.
func TestPushesWrittenAfter(t *testing.T) {
	at := core.SrcPos{Row: 1, Col: 10}
	prog := func(after ...core.SrcPos) *Program {
		p := &Program{Code: []Instr{{Op: OpCallDynMethod}}, Debug: []core.SrcPos{at}}
		for _, d := range after {
			p.Code = append(p.Code, Instr{Op: OpPushConst})
			p.Debug = append(p.Debug, d)
		}
		return p
	}
	for _, c := range []struct {
		name string
		p    *Program
		want bool
	}{
		{"nothing after", prog(), true},
		{"written after on the row", prog(core.SrcPos{Row: 1, Col: 20}), true},
		{"written on a later row", prog(core.SrcPos{Row: 2, Col: 1}), true},
		{"written before (a re-seated operand)", prog(core.SrcPos{Row: 1, Col: 1}), false},
		{"at the apply itself", prog(at), false},
		{"on an earlier row", prog(core.SrcPos{Row: 0, Col: 5}), false},
		{"no position", prog(core.SrcPos{}), false},
		{"one of two before", prog(core.SrcPos{Row: 1, Col: 20}, core.SrcPos{Row: 1, Col: 2}), false},
	} {
		if got := pushesWrittenAfter(c.p, 0); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	short := prog(core.SrcPos{Row: 1, Col: 20})
	short.Debug = short.Debug[:1]
	if pushesWrittenAfter(short, 0) {
		t.Error("a program whose debug table does not match its code must refuse")
	}
}
