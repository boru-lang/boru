package lang

import (
	"fmt"
	"strings"
	"testing"
)

// arm_tail_apply_test.go pins the thirty-fourth increment: a pending
// `apply`-word application at a branch ARM's tail — a fn-typed param
// applied inside the arm with its window inside the arm (`[ 1 k/v apply ]`,
// the CPS rows' then arm) — collapses to the one gradual value the apply
// nets (`EmitRecorder.ArmTailApply`, the arm twin of the unit finish's
// pending-apply arm), recorded as the apply event in the arm's fragment so
// the arm's lowering applies the fn where the interpreter's applyHandler
// does. The window is the arm's own: the arm frame seals the enclosing
// stack off on both lanes, so a pending fn with nothing beneath it inside
// the arm keeps the refusal (the interpreter applies it over an empty
// stack and parks it).

const atkCPS = `def factk fn [[n:Integer k:Function][Any][ if (lte 1 n) [ 1 k/v apply ] [ def kk ( fn r:Integer Any [ def m (mul n r) m k/v apply ] ) (factk (sub 1 n) kk/v) ] ]] end `

// TestArmTailApplyParity pins the shapes that now COMPILE, agree on both
// lanes and run VM-native.
func TestArmTailApplyParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{atkCPS + `(factk 5 (v:Integer => [v]))`, "120 — CPS factorial (the ledger row)"},
		{atkCPS + `(factk 10 (v:Integer => [add 0 v]))`, "3628800 — deeper recursion (the ledger row)"},
		{`def w fn [[n:Integer k:Function][Any][if (n gt 0) [n k/v apply] [0]]] end (w 5 (v:Integer => [add v 1])) (w 0 (v:Integer => [add v 1]))`, "6 0 — the then arm applies the param, the else arm is plain"},
		{`def w fn [[n:Integer k:Function][Any][if (n gt 0) [n k/v apply] [0 k/v apply]]] end (w 5 (v:Integer => [add v 1])) (w 0 (v:Integer => [add v 1]))`, "6 1 — both arms apply"},
		{`def w fn [[n:Integer k:Function][Any][if (n gt 0) [n 2 k/v apply] [0]]] end (w 5 (fn [[a:Integer b:Integer][Integer][a sub b]]))`, "-3 — a two-value window inside the arm"},
	}
	for _, c := range rows {
		gotC, compiled, islands, errC := runCompiledNative(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		if len(islands) > 0 {
			t.Errorf("%q: re-enters the interpreter (%s)", c.src, islands[0])
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestArmTailApplySoundRefusals pins the neighbour that still REFUSES,
// with the interpreter's own answer.
func TestArmTailApplySoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// nothing beneath the pending fn inside the arm: the interpreter
		// applies it over the arm's empty stack and parks it
		{`def w fn [[n:Integer k:Function][Any][if (n gt 0) [k/v apply] [0]]] end (w 5 (v:Integer => [add v 1]))`, "not at the body tail", "[fn k(Integer)]"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a sound refusal", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refused %q, want %q", c.src, reason, c.reason)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter answers %v (%v), want %s", c.src, gotI, errI, c.interp)
		}
	}
}
