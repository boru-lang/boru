package lang

import (
	"fmt"
	"strings"
	"testing"
)

// NUR149's second half (the seventy-third increment): a designed defer — an
// internal_error the compiled VM raises to request whole-program fallback —
// raised inside a `do` (or `eval`) body was TRAPPED as an Error value by the
// escape hatch (DoListHandler / DoEvalList), instead of propagating to the
// top-level run that re-runs interpreted. Stranded, the internal message
// surfaced as data at top level, or an enclosing fn's return contract
// rejected the Error (`type_error … got Error`) where the interpreter
// answered cleanly. bodyErrorPropagates now re-raises a defer (as it already
// re-raised an IO.exit request), so the fallback completes. A genuine boru
// error stays trapped — the escape hatch is unchanged.
func TestDoDeferFallsBackNotTrapped(t *testing.T) {
	// A poly native seat commits an arity over the first call's gradual
	// residual and the second call bails at run time (NUR147); wrapped in a
	// `do`, the bail must still fall back, not surface as a trapped Error.
	// Parity is the contract: the fallback answers exactly as the interpreter
	// does, error or value — where the trap gave a stranded internal message.
	const svc = `def svc (service {}) end  add {} ([r:Map state:Any] => [1]) svc  `
	fellBack := []string{
		// Top level: the internal message used to print as data; now [1 1].
		svc + `do [call {} svc call {} svc]`,
		// Inside a fn body: h's return contract used to reject the Error;
		// now both lanes raise h's own return-count error (parity).
		svc + `def h fn [[][List][do [call {} svc call {} svc]]] end h`,
		// A nested do around the bail.
		svc + `do [do [call {} svc call {} svc]]`,
	}
	for _, src := range fellBack {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
		if compiled {
			t.Errorf("%q: the run bails inside `do` and must FALL BACK, not stay compiled", src)
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

// NUR149's first half (the seventy-third increment): a capture-free fn body
// redefinition of a SPECULATIVE-FAMILY name (a fn defined in a branch arm
// the model could not decide — the seventieth increment) is the family-L
// leak inside a fn body. The drop-then-push leaves the frame's def depth
// unchanged, so the interpreter keeps the shadow past the call while the
// compiled def lowers to nothing and the family's live-lead dispatch resolves
// the arm's binding (whose unit no call site compiled) or an unbound name.
// No compiled twin reproduces a frame-local shadow the interpreter does not
// tear down, so InstallDef refuses and the whole program falls back — slow,
// not wrong.
func TestFnBodySpecFamilyRedefRefuses(t *testing.T) {
	const pre = `def m {e: %s} end  if (m "e" get) [def f fn [[x:Integer][Integer][x add 100]] end] [] end  `
	const g = `def g fn [[][Integer][def f fn [[x:Integer][Integer][x add 1]] end  do [f 5]]] end  `
	refused := []struct{ src, want string }{
		// The arm ran (module f = x add 100), g's body redefines f (x add 1):
		// the interpreter keeps g's f past the call, so `f 1` = 2.
		{fmt.Sprintf(pre, "true") + g + "g f 1", "[6 2]"},
		// The arm did not run (f unbound): g's body's f is the only binding.
		{fmt.Sprintf(pre, "false") + g + "g", "[6]"},
	}
	for _, c := range refused {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: %v", c.src, cerr)
		}
		if prog != nil || !strings.Contains(reason, "redefined inside a fn body replaces a module-scope speculative-family overload") {
			t.Errorf("%q: want the family-L-in-fn-body refusal, got compiled=%v reason=%q", c.src, prog != nil, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if got := fmt.Sprint(gotC); got != c.want {
			t.Errorf("%q = %s, want %s", c.src, got, c.want)
		}
	}
	// A DISJOINT signature is a fresh push above the family (placed for the
	// frame, the seventy-second increment), and a NON-family overlap takes
	// the compiled replace twin: both must keep compiling.
	compiled := []struct{ src, want string }{
		{fmt.Sprintf(pre, "true") + `def g fn [[][String][def f fn [[s:String][String][s]] end  do [f "a"]]] end  g f 1`, "[a 101]"},
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
			t.Errorf("%q: must stay compiled", c.src)
		}
		if got := fmt.Sprint(gotC); got != c.want {
			t.Errorf("%q = %s, want %s", c.src, got, c.want)
		}
	}
	// An IN-FUNCTION speculative family (f absent at fn entry, created in an
	// undecidable in-fn branch) redefined later in the SAME fn is NOT the
	// module-family leak: the baseline gate keeps it off the refusal (Codex
	// P2 on #469). It COMPILES (no refusal); its routed live-lead dispatch
	// may still defer to the interpreter at run time, which falls back with
	// the same answer — contained, not fixed.
	{
		src := `def m {e: true} end  def g fn [[][Integer][if (m "e" get) [def f fn [[x:Integer][Integer][x add 1]] end] [] def f fn [[x:Integer][Integer][x add 2]] end  f 5]] end  g`
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Errorf("the in-function family must NOT be refused: reason=%q err=%v", reason, cerr)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
		if got := fmt.Sprint(gotC); got != "[7]" {
			t.Errorf("in-function family = %s, want [7]", got)
		}
	}
}
