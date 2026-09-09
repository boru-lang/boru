package lang

import (
	"fmt"
	"strings"
	"testing"
)

// apply_data_receiver_test.go pins the thirty-second increment: a fn VALUE
// beneath the `apply` word is DATA. applyHandler re-steps the applied fn
// over the RESOLVED stack, where a fn value on the value stack never
// dispatches — only a tape token does — so the recorder's two windows under
// the apply word admit a fn-valued entry: the paren-bounded pending apply
// over a fn-typed carrier (`(q/v p/v apply)` hands q to p; RecordDynApply)
// and the gradual-lead event (`cfalse/v (…) apply` hands cfalse to the
// applied closure; recordGradualApplyEvent). A paren window with no apply
// word keeps declining a fn-valued entry: inside the paren that value was a
// token the interpreter stepped.
//
// Admitting the window exposed a hole in the bare-read accounting (NUR123):
// a unit ending in a body-tail apply skipped fnResidualReplayReason's every
// arm, the READ accounting included, so a bare read of a fn-typed capture
// consumed as a window ARGUMENT (`(x (n f/v apply) apply)`, the numeral's
// `n`, which the interpreter DISPATCHES over `f/v`) lowered as data and the
// csucc rows compiled to `f` applied to `n`. The accounting now runs under
// a tail apply too, and those rows refuse soundly.

const adrChurch = `def ctrue t:Any => [f:Any => [t/v]] end def cfalse t:Any => [f:Any => [f/v]] end def cif p:Function => [t:Any => [e:Any => [e/v (t/v p/v apply) apply]]] end `

const adrNum = `def czero f:Function => [x:Any => [x/v]] end `

// TestApplyDataReceiverParity pins the shapes that now COMPILE, agree on
// both lanes and run VM-native.
func TestApplyDataReceiverParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{adrChurch + `def cand p:Function => [q:Function => [cfalse/v (q/v p/v apply) apply]] end 'F' ('T' (cif (ctrue/v (cand ctrue/v) apply)) apply) apply`, "T — Church and, T and T (the ledger row)"},
		{adrChurch + `def cor p:Function => [q:Function => [q/v (ctrue/v p/v apply) apply]] end 'F' ('T' (cif (cfalse/v (cor cfalse/v) apply)) apply) apply`, "F — Church or, F or F (the ledger row)"},
		{adrNum + `def s n:Function => [f:Function => [x:Any => [(x (n/v f/v apply) apply)]]] end (s czero/v)`, "fn (Function) — a fn-typed capture beneath a paren-bounded apply over a fn-typed param (the numeral's value spelling)"},
		{`def fgen fn s:Function Function [ ( fn n:Integer Integer [ if (lte 1 n) [1] [ mul n ((sub 1 n) (s/v s/v apply) apply) ] ] ) ] end def fact (fgen fgen/v) end (fact 5)`, "120 — the U combinator's self-application `(s/v s/v apply)` (the ledger row, found graduated by the frontier gate)"},
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

// TestApplyDataReceiverIslanded pins the two Church rows that COMPILE and
// agree but still re-enter the interpreter: a def'd lambda's VALUE (the
// main program's `cfalse/v`, and in the `cor` row `ctrue/v` too) applied
// inside a unit through a carrier lead takes the interpreter island, and
// the island's interpreter-minted result islands again — measured as two
// islands (`cfalse` over 'T', its result over 'F') where the twin rows run
// native. Not the inner lambda's captures (a capturing `cfalse` islands
// the same way); the value that arrives carries no unit the op can enter,
// and which arrival is stamped is the next diagnosis. The pin is the
// island: when it stops islanding, graduate the rows.
func TestApplyDataReceiverIslanded(t *testing.T) {
	rows := []string{
		adrChurch + `def cand p:Function => [q:Function => [cfalse/v (q/v p/v apply) apply]] end 'F' ('T' (cif (cfalse/v (cand ctrue/v) apply)) apply) apply`,
		adrChurch + `def cor p:Function => [q:Function => [q/v (ctrue/v p/v apply) apply]] end 'F' ('T' (cif (cfalse/v (cor ctrue/v) apply)) apply) apply`,
	}
	for _, src := range rows {
		gotC, compiled, islands, errC := runCompiledNative(t, src)
		if !compiled {
			t.Errorf("%q: not compiled", src)
			continue
		}
		if len(islands) == 0 {
			t.Errorf("%q: runs native now — graduate the row (delete its ledger entry, move it to bytecode-migrated.tsv)", src)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(src)
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

// TestApplyDataReceiverSoundRefusals pins the neighbours that still REFUSE,
// with the interpreter's own answer.
func TestApplyDataReceiverSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// the numeral's BARE read `n` beneath the paren-bounded apply: a word
		// dispatch (n over f/v) the window would lower as data — the
		// accounting under the tail apply refuses (compiled to `[fn n(Function) 2]`
		// before the accounting ran there)
		{adrNum + `def csucc n:Function => [f:Function => [x:Any => [(x (n f/v apply) apply) f/v apply]]] end def toint n:Function => [0 ((k:Integer => [add k 1]) n/v apply) apply] end def c1 (csucc czero/v) end def c2 (csucc c1/v) end def c3 (csucc c2/v) end (toint c3/v)`, "unknown provenance", "[3]"},
		{adrNum + `def s n:Function => [f:Function => [x:Any => [(n f/v apply)]]] end (s czero/v)`, "unknown provenance", "[fn (Function)]"},
		// a fn value beneath a paren window with NO apply word keeps declining
		{`def ap p:Function => [q:Function => [(q/v p/v)]] end def fst a:Any => [b:Any => [a/v]] end def k (x:Any => [x/v]) end ((ap fst/v) k/v apply)`, "unknown provenance", "[fn x(Function)]"},
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
