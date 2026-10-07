package lang

import (
	"fmt"
	"testing"
)

// NUR149's second half (the seventy-third increment): a designed defer — an
// internal_error the compiled VM raises when it cannot continue —
// raised inside a `do` (or `eval`) body was TRAPPED as an Error value by the
// escape hatch (DoListHandler / DoEvalList), instead of propagating to the
// top-level run that re-runs interpreted. Stranded, the internal message
// surfaced as data at top level, or an enclosing fn's return contract
// rejected the Error (`type_error … got Error`) where the interpreter
// answered cleanly. bodyErrorPropagates now re-raises a defer (as it already
// re-raised an IO.exit request), so the fallback completes. A genuine boru
// error stays trapped — the escape hatch is unchanged.
func TestDoDeferFallsBackNotTrapped(t *testing.T) {
	// A poly native seat used to commit an arity over the first call's
	// gradual residual and bail at run time on the second call; wrapped in a
	// `do`, the bail had to PROPAGATE out rather than surface as a trapped
	// Error — the shapes this pinned. The seat now retries the other
	// arities the word declares (callPoly, NUR147), so the three compile and
	// answer exactly as the interpreter does: the value, or h's own
	// return-count error. Parity stays the contract.
	const svc = `def svc (service {}) end  add {} ([r:Map state:Any] => [1]) svc  `
	for _, src := range []string{
		svc + `do [call {} svc call {} svc]`,
		svc + `def h fn [[][List][do [call {} svc call {} svc]]] end h`,
		svc + `do [do [call {} svc call {} svc]]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
		if !compiled {
			t.Errorf("%q: the poly seat's retry compiles this shape (NUR147); it did not", src)
		}
	}
	// The escape hatch is unchanged: a GENUINE boru error inside `do` is
	// trapped as an Error value and the program stays compiled.
	trapped := []struct{ src, want string }{
		{`do [raise bad_input "nope"] error [dot code]`, "[bad_input]"},
		{`do [raise bad_input "nope"]`, "[error(nope)]"},
		{`do [1 add 2]`, "[3]"},
		// A user `raise internal_error` shares the public code with a VM
		// defer but carries no VMDefer marker, so it stays CAUGHT (Codex P2
		// on #469): the marker, not the code, distinguishes a defer.
		{`do [raise internal_error "boom"] error [dot code]`, "[internal_error]"},
		{`do [raise internal_error "boom"]`, "[error(boom)]"},
	}
	for _, c := range trapped {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if !compiled {
			t.Errorf("%q: a genuine error (or a plain body) must stay COMPILED (the escape hatch is unchanged)", c.src)
		}
		if got := fmt.Sprint(gotC); got != c.want {
			t.Errorf("%q = %s, want %s", c.src, got, c.want)
		}
	}
}

// NUR149's first half: a capture-free fn body redefinition of a MODULE fn
// with an overlapping signature. The drop-then-push leaves the frame's def
// depth unchanged, so the interpreter keeps the shadow past the call (`f 1`
// is 2 after g) — the leak phase 4 of design/IMMUTABLE-DEF.1.md resolves
// with NUR149; the lanes agree on it meanwhile. Before phase 2 the module
// fn here was bound in an arm the model could not decide (a SPECULATIVE
// family, whose fn-body redefinition declined as family L); an arm's def
// is the arm's own block local now, so the family shape is gone with the
// leak that made it, and the module fn is defined outright.
func TestFnBodyRedefOfModuleFnAgrees(t *testing.T) {
	const modF = `def f fn [[x:Integer][Integer][x add 100]] end  `
	const g = `def g fn [[][Integer][def f fn [[x:Integer][Integer][x add 1]] end  do [f 5]]] end  `
	for _, c := range []struct{ src, want string }{
		// g's body redefines the module f (x add 1): the interpreter keeps
		// g's f past the call, so `f 1` = 2.
		{modF + g + "g f 1", "[6 2]"},
		// No module f: g's body's f is the only binding.
		{g + "g", "[6]"},
	} {
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q interpreted = %v / %v, want %s", c.src, got, err, c.want)
		}
		requireEngineParity(t, c.src, false)
	}
	// A DISJOINT signature is a fresh push above the module fn (placed for
	// the frame, the seventy-second increment), and a NON-family overlap
	// takes the compiled replace twin: both must keep compiling.
	compiled := []struct{ src, want string }{
		{modF + `def g fn [[][String][def f fn [[s:String][String][s]] end  do [f "a"]]] end  g f 1`, "[a 101]"},
		{`def g fn [[][Integer][def f fn [[x:Integer][Integer][x add 1]] end  do [def f fn [[x:Integer][Integer][x add 50]] end  f 5]]] end  def f fn [[x:Integer][Integer][x add 100]] end  f 1 g f 1`, "[101 55 51]"},
	}
	for _, c := range compiled {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: must compile, got reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		gotC, compiledFlag, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if !compiledFlag {
			t.Errorf("%q: must run compiled, got %v", c.src, errC)
		}
		if got := fmt.Sprint(gotC); got != c.want {
			t.Errorf("%q = %s, want %s", c.src, got, c.want)
		}
	}
}
