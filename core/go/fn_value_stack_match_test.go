package core

import "testing"

// fn_value_stack_match_test.go pins FnValueStackMatches — the legacy
// pure-stack path's selection (ExecFnDefSigStackMatch), exported for the VM's
// no-match park — in both directions: what it dispatches, and what it must
// refuse.

// TestUncalledRaisePos: the value's own position, else the first candidate
// that has one.
func TestUncalledRaisePos(t *testing.T) {
	at := SrcPos{Row: 2, Col: 3}
	if got := UncalledRaisePos(at, nil); got != at {
		t.Errorf("the value's own position wins: %+v", got)
	}
	cand := WithPosAt(NewInteger(1), SrcPos{Row: 4, Col: 5})
	if got := UncalledRaisePos(SrcPos{}, []Value{NewInteger(0), cand}); got.Row != 4 || got.Col != 5 {
		t.Errorf("a position-less value takes the first positioned candidate: %+v", got)
	}
}
