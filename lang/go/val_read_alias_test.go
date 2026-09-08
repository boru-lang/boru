package lang

import (
	"fmt"
	"strings"
	"testing"
)

// val_read_alias_test.go pins the thirty-first increment: the Church pair
// rows, which stood behind two gates. Inside the projection's unit a lambda
// VALUE sits beneath the fn-typed param's pending tail apply
// (`(a:Any => [b:Any => [a/v]]) p/v apply`): applyHandler re-steps the fn
// over the RESOLVED stack, where a parked lambda is data the callee's
// Function param binds, so the unit finish's whole-residual window takes
// every value beneath, fn-valued or not. At the main program the `/v` read
// of a def-bound produced closure (`(cfst p/v)`) is a fresh wrap of the
// binding — ResolveRef mints a new Value over the dispatch aggregate — so
// the read's ID carried no provenance: the binding unit now remembers the
// bind's producer by name and aliases the read to it while the binding is
// still the bind's (the recorder's noteValBind / aliasValRead), the re-step
// of such a read takes the pending route before the name fallback (VM-native
// where the fallback islands), and a binding renames a NAMED closure too
// (`(w p/v)` renders under w's param, as installDef renames whatever it
// binds).
//
// Found on the way, pre-existing and now a sound refusal: a fn body's def of
// a capturing fn value over an outer overloading def outlives the call on
// the interpreter (the drop-then-push leaves the frame's def depth
// unchanged, so DefCleanup pops nothing) where the compiled program kept the
// outer closure's bake — `g (p 3)` answered `1 10` for the interpreter's
// `1 12`.

const vraK = `def kk k:Integer => [z:Integer => [add k z]] end `

const vraPair = `def cpair a:Any => [b:Any => [s:Function => [b/v (a/v s/v apply) apply]]] end def cfst p:Function => [(a:Any => [b:Any => [a/v]]) p/v apply] end def csnd p:Function => [(a:Any => [b:Any => [b/v]]) p/v apply] end `

// TestValReadAliasParity pins the shapes that now COMPILE, agree on both
// lanes and run VM-native.
func TestValReadAliasParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{vraPair + `def p (2 (cpair 1) apply) end (cfst p/v)`, "1 — Church pair, first projection (the ledger row)"},
		{vraPair + `def p (2 (cpair 1) apply) end (csnd p/v)`, "2 — second projection (the ledger row)"},
		{vraPair + `def p (2 (cpair 1) apply) end def q (4 (cpair 3) apply) end (cfst p/v) (csnd q/v) (cfst q/v) (csnd p/v)`, "1 4 3 2 — two pairs read twice each; cfst's own `p` param comes and goes between the reads"},
		{`def cfst p:Function => [(a:Any => [b:Any => [a/v]]) p/v apply] end def q s:Function => [5 (7 s/v apply) apply] end (cfst q/v)`, "7 — the lambda beneath the tail apply handed to a plain fn"},
		{`def two p:Function => [(a:Any => [a/v]) 5 p/v apply] end def q fn [[x:Any y:Function][Any][(9 y/v apply) add x]] end (two q/v)`, "14 — two values beneath the tail apply, the lambda bound top-down to the second param"},
		{vraK + `def p (kk 7) end def w f:Function => [f/v] end (w p/v) 4 p/v`, "fn f(Integer) 4 fn p(Integer) — handed to a Function param the read renders under the param's name; the stored value keeps the def's"},
		{vraK + `def p (kk 7) end 3 p/v apply`, "10 — the read applied by the apply word: the pending route, VM-native"},
		{vraK + `def p (kk 7) end def p (kk 8) end 3 p/v apply`, "11 — a rebind moves the read"},
		{vraK + `def p (kk 7) end def q (kk 8) end 3 p/v apply 4 q/v apply`, "3 19 — two closures of one lambda source, each read its own producer (the shared residual id read q's closure for p until the bind-time producer)"},
		{vraK + `def p (kk 7) end (p 3) 4 p/v apply`, "10 11 — the word spelling and the value spelling side by side"},
		{vraK + `def p (kk 7) end typeof p/v`, "Function — the read as data at an Any slot"},
		{vraK + `def p (kk 7) end def w f:Function => [3 f/v apply] end (w p/v)`, "10 — applied inside the callee"},
		{vraK + `def p (kk 7) end def g fn [[][Integer][def q (kk 9) 1]] end g (p 3)`, "1 10 — a fn body's def of ANOTHER name leaves the binding alone"},
		{vraK + `def p (kk 7) end def g fn [[][Integer][1]] end g 3 p/v apply`, "1 10 — a call between the bind and the read"},
		{vraK + `def g fn [[][Integer][def q (kk 9) 1 q/v apply]] end g`, "10 — a fn body's own def, read in the same unit"},
		{vraK + `def p (kk 7) end p/v 4`, "fn p(Integer) 4 — the value spelling followed by a token stays data on both lanes"},
		{vraK + `def p (kk 7) end (p/v 4)`, "11 — paren-bounded, the value applies"},
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

// TestValReadAliasSoundRefusals pins the neighbours that still REFUSE, the
// nested replacing def among them with the interpreter's own answer.
func TestValReadAliasSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// nothing beneath the read: the interpreter parks the value
		{vraK + `def p (kk 7) end p/v apply`, "never dispatched", "[fn p(Integer)]"},
		// a no-match beneath
		{vraK + `def p (kk 7) end 'x' p/v apply`, "never dispatched", "[x fn p(Integer)]"},
		// a code body's read of the def-bound closure (Stage 2)
		{vraK + `def p (kk 7) end [1 2] each [p/v apply]`, "code body reads a def-bound compiled closure", "[[8 9]]"},
		// a read in another unit: the alias is the binding unit's
		{vraK + `def p (kk 7) end def g fn [[][Integer][3 p/v apply]] end g`, "unreachable at a call site", "[10]"},
		// a conditional rebind
		{vraK + `def p (kk 7) end if true [def p (kk 8)] 3 p/v apply`, "redefined inside a conditional body", "[fn p(Integer)]"},
		// the pre-existing miscompile, now refused: a fn body's def of a
		// capturing fn value over an outer overloading def outlives the call
		{vraK + `def p (kk 7) end def g fn [[][Integer][def p (kk 9) 1]] end g (p 3)`, "redefined inside a fn body", "[1 12]"},
		{vraK + `def p (kk 7) end def g fn [[][Integer][def p (kk 9) 1 p/v apply]] end g 3 p/v apply`, "redefined inside a fn body", "[10 12]"},
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
