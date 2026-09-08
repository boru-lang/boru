package lang

import (
	"strings"
	"testing"
)

// function_slot_test.go pins the thirtieth increment: a closure this pass
// PRODUCED handed to a slot DECLARED `Function` — a user fn's `p:Function`
// param, which the interpreter's frame binding installs as data — is the
// call's argument, not a stranded apply, so argIsProducedClosure's
// word-agnostic hold no longer refuses it (`cif (cnot ctrue/v)`); an `Any`
// slot keeps the refusal (`typeof (h 5)` is the paren that failed to
// collapse), and so does apply's own Function slot over a fn-typed carrier.
//
// Lifting the hold exposed a latent collision in the fn-analysis memo key:
// a lambda body that is one `=>` group carries no position, and the key
// identified a body by its first token's position, so cnot's inner
// `t:Any => [f:Any => …]` and cif's inner `t:Any => [e:Any => …]` — both a
// `p:Function` capture over `t:Any` — shared one key and one compiled UNIT,
// and cif's returned closure ran cnot's body (T for the interpreter's F).
// Such a body now keys on its canonical text. And a compiled closure bound
// for a NAMED param takes the param's name at frame entry as a fn value
// does (nameFrameFns → nameClosureValue), so `(w (kk 7)) 4` renders
// `fn g(Any) 4` on both lanes.

const fsChurch = `def ctrue t:Any => [f:Any => [t/v]] end def cfalse t:Any => [f:Any => [f/v]] end def cif p:Function => [t:Any => [e:Any => [e/v (t/v p/v apply) apply]]] end def cnot p:Function => [t:Any => [f:Any => [t/v (f/v p/v apply) apply]]] end `

// TestFunctionSlotParity pins the shapes that now COMPILE, agree on both
// lanes and run VM-native.
func TestFunctionSlotParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{fsChurch + `'F' ('T' (cif (cnot ctrue/v)) apply) apply`, "F — Church not true (the ledger row; answered T before the memo-key fix)"},
		{fsChurch + `'F' ('T' (cif (cnot cfalse/v)) apply) apply`, "T — Church not false"},
		{fsChurch + `def cif7 p:Function => [t:Any => [e:Any => [(t/v p/v apply)]]] end 'F' ('T' (cif7 (cnot ctrue/v)) apply) apply`, "fn (Any) — the inner apply's fn result, unapplied (was T: cnot's body under cif7's closure)"},
		{`def kk x:Any => [y:Any => [x/v]] end def w fn [[g:Function v:Integer][Any][v g/v apply]] end w (kk 7) 4`, "7 — a produced closure at a plain fn's Function param, applied in the body"},
		{`def kk x:Any => [y:Any => [x/v]] end def w fn [[g:Function v:Integer][Any][(v g/v apply)]] end w (kk 7) 4`, "7 — paren-bounded"},
		{`def kk x:Any => [y:Any => [x/v]] end def w fn [[g:Function][Any][typeof g/v]] end w (kk 7)`, "Function — the closure as data"},
		{`def kk x:Any => [y:Any => [x/v]] end def w fn [[g:Function][Function][g/v]] end (w (kk 7)) 4`, "fn g(Any) 4 — the frame binding names the closure"},
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

// TestFunctionSlotSoundRefusals pins the neighbours that still REFUSE.
func TestFunctionSlotSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason string }{
		// an Any slot: the paren that failed to collapse
		{`def mk fn [[g:Function][Function][( fn [[v:Integer][Integer][(g v)]] )]] end def h (mk (z:Integer => [add 7 z])) end typeof (h 5)`, "argument slot"},
		// apply's own Function slot over a fn-typed CARRIER lead
		{`def mk fn [[x:Integer][Function][(fn [[y:Integer][Integer][x add y]])]] end 1 99 (mk 7) apply`, "argument slot"},
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
