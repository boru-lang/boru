package lang

import (
	"fmt"
	"strings"
	"testing"
)

// gradual_apply_test.go pins the twenty-seventh increment: `apply` over a
// GRADUAL lead inside a fn unit. The check matches a Dynamic lead against
// apply's [Reach Any] overload (the longer overload is tried first) and
// models one gradual result; the recorder records that dispatch as an apply
// EVENT over the one value beneath the lead (recordGradualApplyEvent), the
// tail form registers the same pending apply the fn-typed carrier form
// does, and a fn-typed carrier's own paren apply now nets a GRADUAL result
// so a further `apply` over it matches instead of taking the checker's
// best-fit recovery. The VM's op (OpCallDynApplyOne) applies a fn as the
// interpreter's applyHandler re-step does — the args as the stack, the fn
// stepped over them — commits exactly one result, raises the interpreter's
// own `apply` no-match for data, and DEFERS a lens, a closure of another
// arity or any other result count to the interpreter.

const gaAdd2 = `def add2 a:Integer => [b:Integer => [add a b]]  `

// TestGradualApplyParity pins the shapes that COMPILE and agree: the
// apply-word chains of the §5.8 combinator bodies as plain fn bodies, the
// fetched-fn apply the ledger held, and the W combinator's returned lambda
// applied through a def binding.
func TestGradualApplyParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{gaAdd2 + `def w fn [[f:Function x:Integer][Any][x (x f/v apply) apply]]  w add2/v 4`, "8", "apply over a fn-typed carrier's own apply result"},
		{gaAdd2 + `def w fn [[f:Function x:Any][Any][x/v (x f/v apply) apply]]  w add2/v 4`, "8", "the W combinator's body over a gradual x"},
		{gaAdd2 + `def w fn [[f:Function x:Any][Any][(x (x f/v apply) apply)]]  w add2/v 4`, "8", "the same in a paren"},
		{`def app fn [[nd:Any m:Map] [Any] [nd (m get "inc") apply]] def rules {inc: ([x:Integer] => [x add 1])} app 5 rules`, "6", "the fetched-fn apply (the ledger's row)"},
		{`def ww f:Function => [x:Any => [x/v (x f/v apply) apply]] end ` + gaAdd2 + `def w4 (ww add2/v) end (w4 4)`, "8", "the W combinator's returned lambda, def-bound and applied"},
		{gaAdd2 + `def w fn [[m:Map x:Integer][Any][x (m get "f") apply]]  w {f: 42} 4`, "", "data on top: the interpreter's apply no-match, byte for byte"},
		{gaAdd2 + `def w fn [[m:Map x:Integer][Any][(x (m get "f") apply)]]  w {f: "s"} 4`, "", "the paren form's no-match"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if c.want != "" && !strings.Contains(fmt.Sprint(gotC), c.want) {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
		if c.want == "" && (errC == nil || !strings.Contains(errC.Error(), "cannot call `apply`")) {
			t.Errorf("%q: want the interpreter's apply no-match, got %v (%s)", c.src, errC, c.note)
		}
	}
}

// TestGradualApplyDefers pins the runtime states the op hands to the
// interpreter rather than model: a lens on top (the [Reach Any] overload's
// own dispatch), a 0-arg fn (its result lands beside the untouched receiver)
// and a 2-arg fn (the interpreter under-applies and parks) — each answers
// through the fallback with the interpreter's exact value or error.
func TestGradualApplyDefers(t *testing.T) {
	rows := []string{
		gaAdd2 + `def w fn [[m:Map x:Integer][Any][x (m get "f") apply]]  w {f: $.a} 4`,
		gaAdd2 + `def w fn [[m:Map x:Integer][Any][x (m get "f") apply]]  w {f: ([] => [42])} 4`,
		gaAdd2 + `def w fn [[m:Map x:Integer][Any][x (m get "f") apply]]  w {f: (fn [[a:Integer b:Integer][Integer][a sub b]])} 4`,
	}
	for _, src := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled {
			t.Errorf("%q: ran compiled — the op must defer this runtime state", src)
		}
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

// TestGradualApplySoundRefusals pins the neighbour that still REFUSES: a
// body declaring two returns over the one-result model. (The apply word
// over a produced closure at the MAIN program was pinned here as a refusal
// until the twenty-eighth increment compiled it — produced_closure_apply_test.go.)
func TestGradualApplySoundRefusals(t *testing.T) {
	rows := []string{
		gaAdd2 + `def w fn [[m:Map x:Integer][Any Any][x (m get "f") apply]]  w {f: ([] => [42])} 4`,
	}
	for _, src := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", src, cerr)
		}
		if prog != nil || reason == "" {
			t.Errorf("%q: compiled (reason %q) — expected a sound refusal", src, reason)
		}
	}
}
