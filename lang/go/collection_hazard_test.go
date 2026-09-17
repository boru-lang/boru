package lang

// NUR121 (measured 2026-09-05). A fn-typed carrier the check model leaves
// UNAPPLIED lets a later dispatch in the same scope collect its argument,
// and every lowering that applies a lead to the values after it then
// applies the lead to that dispatch's RESULT: `g x add 1` compiled to 18
// (g over add's 6) where the interpreter answers 16 ((g x) then add 1) —
// on the DEFAULT lane, exit 0, in the paren lead window, the whole-frame
// replay and the unnamed-param replay alike. The engine now notes the
// collection hazard where the scope is known (Engine.noteCollectionHazards)
// and the lowerings decline a marked lead. These rows pin the four witnesses
// as refusals the interpreter absorbs and their admitted twins as parity.

import (
	"strings"
	"testing"
)

const hzF = `def f fn [[g:Function x:Integer][Integer]`

func TestCollectionHazardRefuses(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	rows := []struct{ src, reason string }{
		{hzF + `[g x add 1]]  f (z:Integer => [mul 3 z]) 5`, "NUR121"},
		{hzF + `[(g x add 1)]]  f (z:Integer => [mul 3 z]) 5`, "NUR121"},
		{`def f fn [[Function Integer][Integer][args.0 args.1 add 1]]  f (z:Integer => [mul 3 z]) 5`, "NUR121"},
		{`def f fn [[Function Integer][Integer][(args.0 args.1 add 1)]]  f (z:Integer => [mul 3 z]) 5`, "NUR121"},
		{`def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x add 1)]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)`, "body result of unknown provenance"},
		// the lead left ALONE after the collection: `drop` took the argument,
		// and the model's g stands where the interpreter's g already ran
		{hzF + `[g x drop 9]]  f (z:Integer => [mul 3 z]) 5`, "NUR121"},
		{`def f fn [[g:Function x:Integer][Any][g x drop]]  f (z:Integer => [mul 3 z]) 5`, "NUR121"},
		// a code-body frame REWINDS: the do applies its placed lead at its
		// close, so a later word collects the applied value, not the model's
		// second survivor (`do [mk 7] add 1` was 80 for the interpreter's 71)
		{`def mk fn [[] [Function] [([y:Integer] => [y mul 10])]]  do [mk 7] add 1`, "NUR121"},
		{`def f fn [[x:Any] [Any] [([y:Integer] => [y mul 10])]]  do [(f 5) 2] add 1`, "NUR121"},
		{`def f fn [[x:Any] [Any] [([y:Integer] => [y mul 10])]]  do [(f 5) 2] drop`, "NUR121"},
		// CONSERVATIVE: the do's out carries no arity, so a collection above
		// it is a hazard by the coarse rule even though a 1-arg lead never
		// reaches the collected value (`70 9` on both lanes, via the fallback).
		{`def mk fn [[] [Function] [([y:Integer] => [y mul 10])]]  do [mk 7 8] add 1`, "NUR121"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — the hazard lead must refuse (it answered 18 for the interpreter's 16)", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal drifted: want %q in %q", c.src, c.reason, reason)
		}
		// The fallback answers the interpreter's 16 on both lanes.
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// The admitted twins: the lead's argument is what the interpreter's lead
// collects — a plain param, a nested paren's result, a def-local bound
// BEFORE the lead is read, the lead applied and THEN added to — so the
// lowering stands and answers with parity.
func TestCollectionHazardAdmittedTwins(t *testing.T) {
	rows := []struct{ src, want string }{
		{hzF + `[g x]]  f (z:Integer => [mul 3 z]) 5`, "15"},
		{hzF + `[(g x)]]  f (z:Integer => [mul 3 z]) 5`, "15"},
		{hzF + `[(g x) add 1]]  f (z:Integer => [mul 3 z]) 5`, "16"},
		{hzF + `[g (add x 1)]]  f (z:Integer => [mul 3 z]) 5`, "18"},
		{hzF + `[(g (add x 1))]]  f (z:Integer => [mul 3 z]) 5`, "18"},
		{hzF + `[def y (add x 1)  g y]]  f (z:Integer => [mul 3 z]) 5`, "18"},
		{`def f fn [[Function Integer][Integer][(args.0 args.1)]]  f (z:Integer => [mul 3 z]) 5`, "15"},
		{`def f fn [[Function Integer][Integer][args.1 add 1]]  f (z:Integer => [mul 3 z]) 5`, "6 — the frame-prefix Function is inert"},
		// LAZY leads: a paren-placed single survivor is re-stepped only at the
		// enclosing close, over what survives there, and a user call's
		// returned closure is parked — the apply over add's result IS the
		// interpreter's, so the mark does not apply (hazardLead).
		{`def f fn [[Function Integer][Integer][((args.0) args.1 add 1)]]  f (z:Integer => [mul 3 z]) 5`, "18 — parked, then re-stepped over 6"},
		{`def mk fn [[] [Function] [([y:Integer] => [y add 1])]]  ((mk) 7 add 1)`, "9"},
		{`def mk fn [[] [Function] [([y:Integer] => [y add 1])]]  mk 7 add 1`, "fn (Integer) 8"},
		// the do's rewind with nothing collected after it, and a two-value
		// rewind whose first survivor is what a later word collects
		{`def f fn [[x:Any] [Any] [([y:Integer] => [y mul 10])]]  do [(f 5) 2]`, "20"},
		{`def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)`, "15"},
		{`def app fn [[g:Function][Function][( fn [[x:Integer][Integer][(g (add x 1))]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)`, "18"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}
