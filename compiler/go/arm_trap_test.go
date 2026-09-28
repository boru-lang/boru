package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestArmTrapScope pins the sealed-arm trap's contract (recordArmTrap,
// NUR332): it is recorded only in the innermost fragment when that fragment
// is a SEALED branch arm (ArmSealedBranchCapture) — never at the top level
// (RecordTrapErr's own), never in a plain capture (a condition, a loop body,
// the clause-list form's arms), never under an optimistic outer dispatch;
// the first trap per arm wins, the arm's later events are dropped, and the
// fragment ends in the trap, so it diverges.
func TestArmTrapScope(t *testing.T) {
	ae := &core.BoruError{Code: "signature_error", Detail: "cannot call `if` — no signature matches the arguments"}
	pos := core.SrcPos{Row: 1, Col: 30}

	es := NewEmitState()
	if es.RecordArmTrapErr(nil, pos) {
		t.Fatal("no error: nothing to record")
	}
	if es.RecordArmTrapErr(ae, pos) {
		t.Fatal("no fragment is open: the arm trap declines")
	}

	// A plain capture (a condition, a loop body) is not sealed.
	es.ArmBranchCapture()
	end := es.BodyAnalysisGuard()
	if es.RecordArmTrapErr(ae, pos) {
		t.Error("a trap in an unsealed fragment declines")
	}
	end()
	if f := es.TakeFragment(); fragDiverges(f.(*EmitFragment)) {
		t.Error("a declined trap records nothing")
	}

	// A sealed arm takes the trap; the first one wins and the rest of the
	// arm is unreachable.
	es.ArmSealedBranchCapture()
	end = es.BodyAnalysisGuard()
	if !es.RecordArmTrapErr(ae, pos) {
		t.Fatal("a definite no-match in a sealed arm is its trap")
	}
	if !es.RecordArmTrapErr(ae, core.SrcPos{Row: 1, Col: 40}) {
		t.Error("a second raise is unreachable: the first trap stands")
	}
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "add", nout: 1}})
	end()
	frag := es.TakeFragment().(*EmitFragment)
	if len(frag.events) != 1 || frag.events[0].kind != evTrap || frag.events[0].trap.pos != pos || !fragDiverges(frag) {
		t.Fatalf("the arm ends at its trap and diverges: %+v", frag.events)
	}

	// A plain capture after a sealed one is plain again.
	es.ArmSealedBranchCapture()
	es.ArmBranchCapture()
	end = es.BodyAnalysisGuard()
	if es.RecordArmTrapErr(ae, pos) {
		t.Error("ArmBranchCapture re-arms an unsealed capture")
	}
	end()
	es.TakeFragment()

	// A suspended recorder arms nothing.
	resume := es.Suspend()
	es.ArmSealedBranchCapture()
	resume()
	if es.captureArm || es.captureSealed {
		t.Error("an inactive recorder must not arm a capture")
	}

	// Under an optimistically matched outer dispatch (NUR264) the arm trap
	// declines: that dispatch's rematch owns the raise.
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer es.BindRegistry(r)()
	r.Check.OptimisticOuter = &core.OuterMatch{Word: "each"}
	es.ArmSealedBranchCapture()
	end = es.BodyAnalysisGuard()
	if es.RecordArmTrapErr(ae, pos) {
		t.Error("an optimistic outer dispatch owns the raise")
	}
	end()
	es.TakeFragment()
}

// TestWordWrittenAt pins the gate on the optimistic poly's runtime raise
// (RecordPolyCall's PolyRef.Split): the raise underlines the record's word,
// so the word must be the token the source wrote at that position.
func TestWordWrittenAt(t *testing.T) {
	src := "[1 Integer] each [add 1]\n5 $.name apply\nx add/s\ny add\n[1 Integer] each [add, 1]\n\"é\" add 1"
	for _, c := range []struct {
		word string
		row  int
		col  int
		want bool
	}{
		{"add", 1, 19, true},   // a token, then a space
		{"add", 3, 3, true},    // a modifier suffix
		{"add", 4, 3, true},    // the line's end
		{"dot", 2, 5, false},   // a lens key, not the word
		{"ad", 1, 19, false},   // a prefix of a longer token
		{"add", 0, 1, false},   // no position
		{"add", 9, 1, false},   // past the last line
		{"add", 1, 99, false},  // past the line's end
		{"each", 1, 13, true},  // mid-line, then a space
		{"each", 1, 14, false}, // one column off
		{"add", 5, 19, true},   // a comma separator after the word
		{"add", 6, 5, true},    // a multibyte rune before it (columns count runes)
		{"add", 6, 6, false},   // one rune off
	} {
		if got := wordWrittenAt(src, c.word, core.SrcPos{Row: c.row, Col: c.col}); got != c.want {
			t.Errorf("wordWrittenAt(%q @%d:%d) = %v, want %v", c.word, c.row, c.col, got, c.want)
		}
	}
}
