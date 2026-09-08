package lang

import (
	"strings"
	"testing"
)

// tail_apply_collapse_test.go pins the twenty-ninth increment: a unit whose
// finish lowered the fn-value apply at its body tail (`x f/v apply` over a
// fn-typed capture — OpCallDynApplyTop over the whole residual) nets ONE
// value at run time, but the check pass's call-site residual still held the
// window: the apply word over a fn-typed carrier is elided and its identity
// result flows to the residual, so a call of such a closure saw [result, f]
// — two outs — and the produced-closure record site (single-out) declined
// into the unit call's capture refusal ("capture f of  unreachable at a
// call site"). The call site now collapses the window as the unit does
// (EmitRecorder.UnitTailApply → check's collapseTailApply): one gradual
// result, the values beneath surviving.

// TestTailApplyCollapseParity pins the shapes that now COMPILE, agree on
// both lanes and run VM-native.
func TestTailApplyCollapseParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{`def bb f:Function => [g:Function => [x:Any => [(x g/v apply) f/v apply]]] end 4 ((n:Integer => [add n 3]) (bb (n:Integer => [mul n 2])) apply) apply`, "14 — B with inline-lambda args (ledger row, was refused)"},
		{`def bb f:Function => [g:Function => [x:Any => [(x g/v apply) f/v apply]]] end def d fn n:Integer Integer [mul n 2] end def e fn n:Integer Integer [add n 3] end 4 (e/v (bb d/v) apply) apply`, "14 — the named-/v twin"},
		{`def bb2 f:Function => [x:Integer => [x f/v apply]] end def h (bb2 (n:Integer => [mul n 2])) end (h 4) add 1`, "9 — a def-bound closure whose body tail applies its capture"},
		{`def bb2 f:Function => [x:Integer => [x f/v apply]] end def h (bb2 (n:Integer => [mul n 2])) end (h 4) (h 5)`, "8 10 — two calls, two results"},
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

// TestTailApplyCollapseSoundRefusals pins the neighbours that still REFUSE.
func TestTailApplyCollapseSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason string }{
		// a gradual param beneath the tail apply: the factory's body result
		// has no provenance the recorder can place
		{`def bb2 f:Function => [x:Any => [x f/v apply]] end 4 (bb2 (n:Integer => [mul n 2])) apply`, "unknown provenance"},
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
	}
}

// TestReturnedLambdaCountContract pins the count contract a RETURNED lambda's
// closure now carries (tryReturnedClosure's closureRet): a body that
// under-applies at its tail — `[7 x f/v apply]` over a 1-arg f nets [7 8] —
// raises the interpreter's `expected 1 return value(s), got 2` on the
// compiled lane too, where an uncontracted closure answered [7 8] (latent
// before the collapse landed: every such call site refused). The unbound
// form agrees byte for byte; the DEF-BOUND form names the binding on both
// lanes (the def-site rename, StoreNames) and differs only in POSITION —
// the interpreter reports the head `h`, the compiled op its argument — the
// NUR122 position-only class, pinned as measured.
func TestReturnedLambdaCountContract(t *testing.T) {
	const unbound = `def bb2 f:Function => [x:Integer => [7 x f/v apply]] end 4 (bb2 (n:Integer => [mul n 2])) apply`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, unbound)
	if !compiled {
		t.Fatalf("%q: not compiled", unbound)
	}
	requireParity(t, unbound, gotC, errC, gotI, errI)
	if errC == nil || !strings.Contains(errC.Error(), "expected 1 return value(s), got 2") {
		t.Errorf("the count contract must raise: %v", errC)
	}
	const bound = `def bb2 f:Function => [x:Integer => [7 x f/v apply]] end def h (bb2 (n:Integer => [mul n 2])) end (h 4)`
	gotC, compiled, errC, gotI, errI = runBothEngines(t, bound)
	if !compiled || errC == nil || errI == nil || len(gotC) != 0 || len(gotI) != 0 {
		t.Fatalf("both lanes raise: compiled=%v %v/%v interp %v/%v", compiled, gotC, errC, gotI, errI)
	}
	first := func(err error) string { return strings.SplitN(err.Error(), "\n", 2)[0] }
	if first(errC) != first(errI) || !strings.Contains(first(errC), "h: expected 1 return value(s), got 2") {
		t.Errorf("the def-bound closure names its binding on both lanes:\n  compiled %s\n  interp   %s", first(errC), first(errI))
	}
	if !strings.Contains(errI.Error(), "1:100") || !strings.Contains(errC.Error(), "1:102") {
		t.Errorf("measured, open (NUR122 position-only): interp at the head 1:100, compiled at the argument 1:102:\n  compiled %v\n  interp   %v", errC, errI)
	}
}
