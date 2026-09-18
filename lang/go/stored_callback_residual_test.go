package lang

import (
	"fmt"
	"strings"
	"testing"
)

// NUR153, RULED (maintainer, 2026-09-18) and pinned as THE RULE. A fn body's
// residual pending container evaluates in the live frame — against the bound
// params and captures — UNLESS the fn is an anonymous `=>` lambda whose body
// is a single bare container literal, which DEFERS: the container leaves the
// frame unevaluated and resolves where its consumer evaluates it. One value
// means one thing wherever it goes (design/FUNCTION-VALUE-SCOPE.0.md §11), so
// every seam asks the one predicate (core.ResidualEvalsInFrame) — the tape
// apply, the CallBoru sub-run a native seam invokes, and the compiler's
// residual-recording admission alike.
//
// Before the ruling there were two regimes: the tape deferred while a native
// seam (a service `call`) evaluated in the live frame, and the compiled stamp
// — one unit — took the seam's, so a stamped stored `=>` value applied on the
// tape answered `[[{z:1}]]` where the interpreter answered `[[99]]`. Closing
// the regime split closed that divergence with it.
func TestStoredCallbackResidualRegimes(t *testing.T) {
	// 1. The fn-word twin: a NAMED fn evaluates its body in-frame everywhere,
	// through every seam and on both engines. The rule does not touch it.
	fnWord := `def a 99 def api (patrun Function) add {cmd:"x"} (fn [[a:Map][List][[a]]]) api def h (find {cmd:"x"} api) h {z:1}`
	gotC, _, errC, gotI, errI := runBothEngines(t, fnWord)
	requireParity(t, fnWord, gotC, errC, gotI, errI)
	if got := fmt.Sprint(gotI); got != "[[{z:1}]]" {
		t.Errorf("fn-word twin = %s, want [[{z:1}]] (a named fn reads its param in-frame)", got)
	}

	// 2. THE RULE on the tape: an anonymous `=>` whose body is a single bare
	// container defers, so the bare `a` resolves in MODULE scope — the pinned
	// no-closures transparency (def-node-binding.tsv §3).
	tape := `def a 99 def api (patrun Function) add {cmd:"x"} ([a:Map] => [[a]]) api def h (find {cmd:"x"} api) h {z:1}`
	gotC, _, errC, gotI, errI = runBothEngines(t, tape)
	requireParity(t, tape, gotC, errC, gotI, errI)
	if got := fmt.Sprint(gotI); got != "[[99]]" {
		t.Errorf("tape apply = %s, want [[99]] (the deferred container resolves in module scope)", got)
	}
	// …and the COMPILED lane agrees: the divergence NUR153 recorded is closed.
	// It answered [[{z:1}]] until 2026-09-18.
	c, _ := New()
	cv, cerr := c.RunCompiledStrict(tape)
	if cerr != nil || fmt.Sprint(cv) != "[[99]]" {
		t.Errorf("tape apply, compiled = %v/%v, want [[99]] — the stamp must take the one rule; if this diverges again, NUR153 is the record to reopen", cv, cerr)
	}

	// 3. THE SAME RULE through a native seam. The seam used to evaluate the
	// container in the live frame (regime 2) and answer `unknown 'BOGUS'`;
	// under the rule it defers, so the bare `req` is unbound exactly as the
	// tape leaves it — and the seam's answer EQUALS the tape twin's, value and
	// error taxonomy. That equality is the ruling, stated as a test.
	seam := `def svc (service {})
add {} ([req:Map state:Any] => [ {message: (join "" ["unknown '" req.cmd "'"])} ]) svc
call {cmd:"BOGUS"} svc`
	tapeTwin := `def api (patrun Function) add {cmd:"x"} ([req:Map state:Any] => [ {message: (join "" ["unknown '" req.cmd "'"])} ]) api
def h (find {cmd:"x"} api) h {cmd:"BOGUS"} 0`
	s, _ := New()
	sv, serr := s.RunInterp(seam)
	tw, _ := New()
	tv, terr := tw.RunInterp(tapeTwin)
	if serr == nil || terr == nil {
		t.Fatalf("both the seam and its tape twin must raise for a deferred container reading its param: seam=%v/%v twin=%v/%v", sv, serr, tv, terr)
	}
	// Compare the TAXONOMY line, not the rendered text: each error quotes its
	// own source, and the two programs differ by construction.
	head := func(err error) string { return strings.SplitN(err.Error(), "\n", 2)[0] }
	if !strings.Contains(head(serr), "undefined_word") || head(serr) != head(terr) {
		t.Errorf("seam error %q != tape twin error %q — the seam must answer what the tape answers", head(serr), head(terr))
	}
	if got := fmt.Sprint(sv); got != fmt.Sprint(tv) {
		t.Errorf("seam value %s != tape twin value %s — the seam must answer what the tape answers", got, fmt.Sprint(tv))
	}

	// 4. The migration the rule asks for: a COMPUTING body (multi-token, or a
	// single paren expression) evaluates in-frame, so a handler that must read
	// its params wraps the container. Both engines, through the seam.
	computing := `def svc (service {})
add {} ([req:Map state:Any] => [ ({message: (join "" ["unknown '" req.cmd "'"])}) ]) svc
((call {cmd:"BOGUS"} svc) get "message")`
	gotC, _, errC, gotI, errI = runBothEngines(t, computing)
	requireParity(t, computing, gotC, errC, gotI, errI)
	if got := fmt.Sprint(gotI); got != "[unknown 'BOGUS']" {
		t.Errorf("computing-body seam = %s, want [unknown 'BOGUS'] (the paren makes the body compute, so it reads its param in-frame)", got)
	}
}
