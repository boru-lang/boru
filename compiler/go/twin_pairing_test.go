package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The write-back pairing at the seam (review of #459): a root def takes the
// push-kind twin lowered under its name and marks it when it emits a
// write-back; an undef twin pairs with nothing; a name with no twin, an
// index the table does not hold, and a mark of nothing are each a no-op.
func TestTwinPairingGuards(t *testing.T) {
	lw := &lowerer{p: &Program{BindTwins: []core.BindTransition{
		{Kind: core.BindDef, Name: "a"},
		{Kind: core.BindUndef, Name: "a"},
		{Kind: core.BindDefReplace, Name: "b"},
	}}}
	lw.noteTwin(-1)
	lw.noteTwin(7)
	if lw.twinFor != nil {
		t.Fatal("an index the table does not hold pairs nothing")
	}
	lw.noteTwin(1)
	if lw.twinFor != nil {
		t.Fatal("an undef twin pairs with no def")
	}
	if lw.takeTwin("a") != -1 {
		t.Fatal("a name with no pending twin takes -1")
	}
	lw.noteTwin(0)
	lw.noteTwin(2)
	if got := lw.takeTwin("a"); got != 0 {
		t.Fatalf("takeTwin(a) = %d, want 0", got)
	}
	if got := lw.takeTwin("a"); got != -1 {
		t.Fatalf("a taken twin is not taken twice, got %d", got)
	}
	lw.markTwinWrittenBack(-1)
	lw.markTwinWrittenBack(9)
	lw.markTwinWrittenBack(lw.takeTwin("b"))
	if !lw.p.BindTwins[2].WrittenBack || lw.p.BindTwins[0].WrittenBack {
		t.Fatalf("only the paired, written-back twin is marked: %+v", lw.p.BindTwins)
	}
}
