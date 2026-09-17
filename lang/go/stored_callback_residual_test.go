package lang

import (
	"fmt"
	"testing"
)

// NUR153, pinned as MEASURED. A stored `=>` callback's single container
// residual evaluates under TWO regimes on the interpreter: deferred past the
// frame when the value is applied on the tape (the lambda rule — the bare `a`
// resolves in module scope), and in the live frame when a native seam invokes
// it through InvokeCallback / CallBoru (a service `call`). The compiled stamp
// is one unit and takes the seam regime, so the tape apply of a stamped stored
// `=>` value diverges. The `fn`-word twin evaluates in-frame everywhere. The
// day any of the three answers below moves, NUR153 is the record to update.
func TestStoredCallbackResidualRegimes(t *testing.T) {
	// The fn-word twin: in-frame on both engines, through every seam.
	fnWord := `def a 99 def api (patrun Function) add {cmd:"x"} (fn [[a:Map][List][[a]]]) api def h (find {cmd:"x"} api) h {z:1}`
	gotC, _, errC, gotI, errI := runBothEngines(t, fnWord)
	requireParity(t, fnWord, gotC, errC, gotI, errI)
	if got := fmt.Sprint(gotI); got != "[[{z:1}]]" {
		t.Errorf("fn-word twin = %s, want [[{z:1}]]", got)
	}

	// Regime 1 — the tape apply of the stored `=>` value defers.
	tape := `def a 99 def api (patrun Function) add {cmd:"x"} ([a:Map] => [[a]]) api def h (find {cmd:"x"} api) h {z:1}`
	b, _ := New()
	iv, ierr := b.RunInterp(tape)
	if ierr != nil || fmt.Sprint(iv) != "[[99]]" {
		t.Errorf("tape apply, interpreted = %v/%v, want [[99]] (regime 1: deferred to module scope)", iv, ierr)
	}
	// …and the stamped unit takes regime 2: the KNOWN divergence. When this
	// starts answering [[99]], the interpreter has one rule — close NUR153.
	c, _ := New()
	cv, cerr := c.RunCompiledStrict(tape)
	if cerr != nil || fmt.Sprint(cv) != "[[{z:1}]]" {
		t.Errorf("tape apply, compiled = %v/%v — NUR153 pins [[{z:1}]] as the open divergence; if it now agrees with the interpreter, close the record", cv, cerr)
	}

	// Regime 2 — through a native seam, the same kind of value reads its
	// param in the live frame, on both engines.
	seam := `def svc (service {})
add {} ([req:Map state:Any] => [ {message: (join "" ["unknown '" req.cmd "'"])} ]) svc
((call {cmd:"BOGUS"} svc) get "message")`
	gotC, _, errC, gotI, errI = runBothEngines(t, seam)
	requireParity(t, seam, gotC, errC, gotI, errI)
	if got := fmt.Sprint(gotI); got != "[unknown 'BOGUS']" {
		t.Errorf("seam invocation = %s, want [unknown 'BOGUS'] (regime 2: the live frame)", got)
	}
}
