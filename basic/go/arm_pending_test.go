package basic

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// arm_pending_test.go pins basic's half of NUR356: which `if` arms are
// spliced (the interpreter leaves what they end with pending past the
// `if`), and the computed arm's designed defer.

func TestArmsSplicedAndResidue(t *testing.T) {
	r := newTestRegistry(t)
	r.Check.CurCallPos = SrcPos{Row: 1, Col: 3}
	if !armsSpliced(r) {
		t.Error("an `if` the program wrote splices its arms")
	}
	r.Check.CurCallPos = SrcPos{}
	if armsSpliced(r) {
		t.Error("the case desugar's synthesized `if` does not")
	}
	p := PendingResidue{Left: true, Reads: []string{"x"}}
	if got := armResidue(true, p); !got.Left {
		t.Error("a spliced arm carries its residue")
	}
	if got := armResidue(false, p); got.Left {
		t.Error("a swept arm carries none")
	}
}

// pendingWords is a parser-shaped (Eval) list literal of words.
func pendingWords(names ...string) Value {
	elems := make([]Value, len(names))
	for i, n := range names {
		elems[i] = NewWord(n)
	}
	return core.NewEvalList(elems)
}

func TestArmHoldsSteppingLiteral(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  Value
		want bool
	}{
		{"a literal read", NewList([]Value{pendingWords("x")}), true},
		{"in a paren", NewList([]Value{NewParenExpr([]Value{pendingWords("x")})}), true},
		{"in a nested body", NewList([]Value{NewWord("do"), NewList([]Value{pendingWords("x")})}), true},
		{"scalars", NewList([]Value{core.NewEvalList([]Value{NewInteger(1)}), NewWord("x")}), false},
		{"a paren of scalars", NewList([]Value{NewParenExpr([]Value{NewInteger(1)})}), false},
		{"no list", NewInteger(1), false},
	} {
		if got := armHoldsSteppingLiteral(tc.arm); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

// TestArmSpliceHandlerDefersAPendingLiteral: a computed arm holding a
// literal the interpreter would leave pending past the `if` is a designed
// defer — loud — where the run would evaluate it at the arm's end.
func TestArmSpliceHandlerDefersAPendingLiteral(t *testing.T) {
	r := newTestRegistry(t)
	_, err := ArmSpliceHandler([]Value{NewList([]Value{pendingWords("x")})}, nil, nil, r)
	if err == nil || !IsVMDefer(err) || !strings.Contains(err.Error(), "NUR356") {
		t.Fatalf("want the NUR356 designed defer, got %v", err)
	}
	out, err := ArmSpliceHandler([]Value{NewList([]Value{NewInteger(1)})}, nil, nil, r)
	out, err = armRun(r, out, err)
	if err != nil || len(out) != 1 {
		t.Errorf("a scalar arm runs: %v %v", out, err)
	}
}

// armRun steps the region a computed arm's handler returns (CallRegion)
// on an engine, as the run holding the `if` would, and hands back its
// values; anything else passes through.
func armRun(r *Registry, out []Value, err error) ([]Value, error) {
	if err != nil || !IsLoopRegion(out) {
		return out, err
	}
	return NewTop(r).Run(out)
}
