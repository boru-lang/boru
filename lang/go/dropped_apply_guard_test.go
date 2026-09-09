package lang

import (
	"fmt"
	"strings"
	"testing"
)

// dropped_apply_guard_test.go pins the def site's DROPPED-APPLY refusal
// (the fn-carrier dup check in basic/go/native_definition.go's def handler):
// a `def` whose value is the VERY Function carrier another name already
// denotes this pass means the body's apply was never modelled — the analysis
// handed the callee straight back — so a compiled program would bind both
// names to one slot and leak the unconsumed arguments into the residual.
// Refuse, and the interpreter fallback owns the shape: slow, not wrong.
//
// The guard's own comment cites `def f2 (f1 2)` over a curried factory, and
// that shape no longer reaches it: the read model carries a RESULT SHAPE, so
// every level of a curry chain models its own dispatch and mints a FRESH
// carrier (the negative row below compiles and agrees on both lanes). What
// still drops the apply is a wrapper whose arity the PRODUCING word cannot
// claim: `FnUtil.flip` over the OVERLOADED `sub` has no one arity
// (fnOperandArity declines for a multi-signature operand), so no FnShape is
// noted, the compile pass's read model declines SILENTLY (its `!claimed`
// arm — no refusal of its own), and the paren places the callee unchanged.
// The second def then binds `fs`'s own carrier while the interpreter binds
// the Integer the apply produced: `def fs (FnUtil.flip sub/v) end
// def g (fs 3 10) end g` is `-7` interpreted, and the analysis's `g` is the
// flip wrapper. That divergence is what the guard catches, and it is the
// FIRST refusal on the pass — no earlier model has anything to say here.

const dagFlip = `import "boru:fn-util"  def fs (FnUtil.flip sub/v) end `
const dagCurry = `import "boru:fn-util"  ` +
	`def add3 fn [[a:Integer b:Integer c:Integer][Integer][a add (b add c)]] end ` +
	`def c (FnUtil.curry add3/v) end `

// TestDroppedApplyDefRefusal pins the refusal itself: the second def binds a
// carrier the pass already bound under another name, the guard names the
// dropped apply, and the interpreter answers with the value the apply
// really produced.
func TestDroppedApplyDefRefusal(t *testing.T) {
	rows := []struct{ src, interp, note string }{
		{dagFlip + `def g (fs 3 10) end g`, "[-7]",
			"the analysis binds g to fs's own carrier; the interpreter binds the applied Integer"},
		{dagFlip + `def g (fs 3 10) end g add 0`, "[-7]",
			"`g add 0` only type-checks in the interpreter BECAUSE g is an Integer there — the apply the analysis dropped"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, res, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v (%s)", c.src, cerr, c.note)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected the dropped-apply refusal (%s)", c.src, c.note)
			continue
		}
		if !strings.Contains(reason,
			"def of a computed fn whose apply the analysis dropped") {
			t.Errorf("%q: refused %q, want the dropped-apply reason (%s)", c.src, reason, c.note)
		}
		for _, d := range res.Diagnostics {
			if d.Code == "undefined_word" {
				t.Errorf("%q: the check reports %s (%s) on a correct program", c.src, d.Code, d.Detail)
			}
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

// TestDroppedApplyDefRefusalNeighbours pins the two negatives: the SAME
// unclaimed wrapper applied without a second def refuses somewhere else
// entirely (the residual classifier — the guard is about the duplicate
// BIND, not about the unclaimed wrapper), and a def chain whose applies the
// read model DOES model — the curried factory the guard's comment names —
// binds a fresh carrier per level, compiles, and agrees on both lanes.
func TestDroppedApplyDefRefusalNeighbours(t *testing.T) {
	// No second def: nothing rebinds fs's carrier, so the dropped-apply
	// guard stays silent and the flattened residual owns the refusal.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	const bare = dagFlip + `(fs 3 10)`
	prog, reason, _, cerr := a.CompileCheck(bare)
	if cerr != nil {
		t.Fatalf("%q: check: %v", bare, cerr)
	}
	if prog != nil {
		t.Errorf("%q: compiled — the unclaimed wrapper's apply has no model", bare)
	}
	if !strings.Contains(reason, "closure shape unknown") {
		t.Errorf("%q: refused %q, want the residual classifier's reason", bare, reason)
	}
	if strings.Contains(reason, "the analysis dropped") {
		t.Errorf("%q: refused with the dropped-apply reason, but no def rebinds the carrier", bare)
	}

	// The curried chain: `def c1 (c 1)` and `def c2 (c1 2)` each model their
	// own dispatch and bind a carrier of their own, so the guard never sees
	// a duplicate and the whole chain compiles.
	const chain = dagCurry + `def c1 (c 1) end def c2 (c1 2) end (c2 3)`
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotC, compiled, errC := b.RunCompiled(chain)
	if !compiled {
		t.Errorf("%q: not compiled — the modelled curry chain must not trip the dropped-apply guard", chain)
	}
	d, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, errI := d.RunInterp(chain)
	requireParity(t, chain, gotC, errC, gotI, errI)
	if fmt.Sprint(gotI) != "[6]" {
		t.Errorf("%q: interpreter answers %v (%v), want [6]", chain, gotI, errI)
	}
}
