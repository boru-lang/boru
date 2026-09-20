package lang

import (
	"sync"
	"testing"
)

// TestPredicateBodyRunsOnTheVM is a MEASUREMENT pinned as a test — the same
// measurement that used to assert the opposite, kept pointing the other way.
//
// The claim it used to pin: a predicate body never runs as a compiled unit, so
// the `is`-against-a-predicate-type compile failure was honest and had to stand. That
// claim was true, and its stated CAUSE — "predicate bodies do not compile to
// units" — was wrong. They compiled. `def Positive fn […]` stamped a detached
// unit at construction and recorded Stamped:true in the stamp ledger; the
// runtime then ran the interpreter anyway, and the ledger and the runtime
// disagreed with nobody watching.
//
// The decline was one line in runUnitNested: `ref.Prog != vc.p`. A DETACHED ref
// is compiled on an isolated fork into its own standalone one-unit Program, so
// it never equals the running program — the mid-run nested path rejected every
// one of them by construction while InvokeCallback's doc promised "nested in a
// live run, or fresh on an idle registry". Only the idle half was real.
// vm_foreign_unit.go hosts the foreign program's unit instead of declining it,
// and these rows moved onto the VM with no other change.
//
// The control row stays load-bearing, and now says the useful thing: a
// predicate reached through ORDINARY DISPATCH — no `is`, no gate — runs on the
// VM too. That was pre-existing debt in compiled programs; it is gone rather
// than merely accounted for.
func TestPredicateBodyRunsOnTheVM(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"dispatch through a predicate param (no `is`, no gate)",
			`def Pos fnpred n:Integer [n gt 0]  def f fn [[x:Pos] [Integer] [x]]  f 5`},
		{"a refine predicate at a typed def",
			`def Pos fnpred n:Integer [n gt 0]  def x:Pos 5  x`},
	} {
		a := mustNew(t)
		var mu sync.Mutex
		seams := map[string]int{}
		disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
			mu.Lock()
			defer mu.Unlock()
			if !e.CheckMode {
				seams[e.Seam]++
			}
		})
		_, ran, err := a.RunCompiled(tc.src)
		if noteCompileDefect(t, tc.src, nil, err) {
			continue
		}
		disarm()
		if !ran || err != nil {
			t.Fatalf("%s: ran=%v err=%v", tc.name, ran, err)
		}
		mu.Lock()
		got := seams["InvokeCallback:callboru"]
		mu.Unlock()
		if got != 0 {
			t.Errorf("%s: the predicate body took InvokeCallback's CallBoru arm %d time(s) — "+
				"a compiled program grew an interpreter island back inside a native handler, "+
				"which no whole-program FALLBACK check can see", tc.name, got)
		}
	}
}
