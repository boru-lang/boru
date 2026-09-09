package lang

import (
	"strings"
	"testing"
)

// produced_closure_apply_test.go pins the twenty-eighth increment: `apply`
// over a closure this pass PRODUCED — the result of a factory call whose
// unit pushes a closure — at the main program and inside a unit.
//
// Every §5.8 combinator row the twenty-seventh increment re-diagnosed
// refused at `argIsProducedClosure` when the apply word's [Function]
// overload took the closure (`99 (kk 7) apply`): the guard reads a produced
// closure at a word's argument slot as a paren that failed to collapse, but
// under `apply` the closure at the slot IS what the word applies. The word
// now hands it back as a PENDING application (recordCallElided), the check
// engine re-steps the value over the values beneath exactly as the
// interpreter's applyHandler does, and the re-step's dispatch records
// through the fn-VALUE apply over the closure's producer operand (check's
// recordPendingClosureApply → RecordDynApply → OpCallDynApplyTop) — a unit
// call could not: the closure's construction-scope captures are unreachable
// at the call site, and only the runtime payload carries them. A pending
// entry no dispatch consumed refuses at Finalize.

const pcaK = `def kk x:Any => [y:Any => [x/v]] end `

// runCompiledNative is runBothEngines' compiled half with the interp-entry
// hook armed: the VM island seams (`vm:island`, `vm:island-resolved`) are
// the debt the interp-entry census counts, and this family's first cut
// paid it on every row — the VM's apply op compared the closure unit's
// NParams, which counts the trailing capture slots, with the window, so
// every capturing closure took the interpreter's re-step island.
func runCompiledNative(t *testing.T, src string) (gotC []any, compiled bool, islands []string, errC error) {
	t.Helper()
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
		if ev.CheckMode || ev.Attribution != "" || !strings.HasPrefix(ev.Seam, "vm:island") {
			return
		}
		islands = append(islands, ev.Seam)
	})
	gotC, compiled, errC = b.RunCompiled(src)
	disarm()
	return
}

// TestProducedClosureApplyParity pins the shapes that now COMPILE, agree
// on both lanes, and run VM-NATIVE — no interpreter island.
func TestProducedClosureApplyParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{pcaK + `99 (kk 7) apply`, "7 — the K combinator row (was refused: computed closure at a word's argument slot)"},
		{pcaK + `99 (kk 7) apply 1 (kk 8) apply`, "99 8 — two applies of one lambda source, each its own event (the freshened out)"},
		{pcaK + `99 (kk 7) apply 5`, "99 7 — a token after the apply word binds as the interpreter's re-step binds it"},
		{pcaK + `1 99 (kk 7) apply`, "1 7 — a deeper value survives beneath the apply"},
		{pcaK + `(99 (kk 7) apply)`, "7 — paren-bounded"},
		{pcaK + `def g fn [[n:Integer][Integer][n (kk 7) apply]] end g 99`, "7 — inside a fn unit"},
		{pcaK + `def g n:Integer => [n (kk 7) apply] end g 99`, "7 — inside a lambda unit"},
		{`def kk x:Integer => [y:Integer => [x add y]] end 99 (kk 7) apply`, "106 — typed params"},
		{`def mk2 x:Integer => [(fn [[y:Integer][Integer][x add y]])] end 99 (mk2 7) apply`, "106 — an inline fn literal closure"},
		{`def kk x:Any => [y:Any => [x/v]] end def ss f:Function => [g:Function => [x:Any => [(x g/v apply) (x f/v apply) apply]]] end def ii (kk/v (ss kk/v) apply) end 42 ii/v apply`, "42 — I = S K K: a fn value beneath the produced closure is the closure's argument"},
		{`def kk x:Any => [y:Any => [x/v]] end def ss f:Function => [g:Function => [x:Any => [(x g/v apply) (x f/v apply) apply]]] end def ii (kk/v (ss kk/v) apply) end 'hello' ii/v apply`, "hello — the derived identity over a String"},
		{`def cc f:Function => [x:Any => [y:Any => [x/v (y f/v apply) apply]]] end def add2 a:Integer => [b:Integer => [add a b]] end 10 (1 (cc add2/v) apply) apply`, "11 — C: the staged apply's fn-value result applies again"},
		{`def ww f:Function => [x:Any => [x/v (x f/v apply) apply]] end def add2 a:Integer => [b:Integer => [add a b]] end 4 (ww add2/v) apply`, "8 — W"},
		{`def ctrue t:Any => [f:Any => [t/v]] end def cif p:Function => [t:Any => [e:Any => [e/v (t/v p/v apply) apply]]] end 'F' ('T' (cif ctrue/v) apply) apply`, "T — Church true"},
		{`def cfalse t:Any => [f:Any => [f/v]] end def cif p:Function => [t:Any => [e:Any => [e/v (t/v p/v apply) apply]]] end 'F' ('T' (cif cfalse/v) apply) apply`, "F — Church false: its capture-free inner lambda is a stamped const, entered under the value's own contract"},
		{`def app fn [[nd:Any m:Map] [Any] [nd (m get "inc") apply]] def rules {inc: ([x:Integer] => [x add 1])} app 5 rules`, "6 — the fetched-fn apply (increment 27's positive twin), native now"},
	}
	for _, c := range rows {
		gotC, compiled, islands, errC := runCompiledNative(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		if len(islands) > 0 {
			t.Errorf("%q: re-enters the interpreter (%s) — the closure must run VM-native", c.src, islands[0])
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestProducedClosureApplySoundRefusals pins the neighbours that REFUSE —
// the default lane then answers on the interpreter — never a wrong value.
func TestProducedClosureApplySoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, note string }{
		// nothing beneath the closure: the interpreter leaves it as data
		{pcaK + `(kk 7) apply`, "never dispatched", "fn (Any) on the interpreter"},
		// the values beneath match no signature: data again
		{`def kk x:Integer => [y:Integer => [x add y]] end "s" (kk 7) apply`, "never dispatched", "s fn (Integer)"},
		// a fn-typed CARRIER lead (a declared [Function] return) keeps the
		// argument-slot refusal — lifted, `1 99 (mk 7) apply` seated all
		// three as data (measured)
		{`def mk fn [[x:Integer][Function][(fn [[y:Integer][Integer][x add y]])]] end 1 99 (mk 7) apply`, "argument slot", "1 106"},
		// a produced closure applied over ANOTHER produced closure: the
		// second dispatches over the first before apply runs
		{pcaK + `(kk 7) (kk 8) apply`, "argument slot", "8"},
		// a two-arg closure over two literals: the seating cannot reorder
		{`def k2 x:Integer => [[a:Integer b:Integer] => [a sub b]] end 10 3 (k2 0) apply`, "reordered", "-7"},
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
			t.Errorf("%q: compiled — expected a sound refusal (%s)", c.src, c.note)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refused %q, want %q", c.src, reason, c.reason)
		}
	}
}
