package lang

import (
	"testing"
)

// def_computed_fn_test.go pins the thirty-fifth increment: the def-bound
// COMPUTED fn family (fn-util's produced wrappers). Three pieces meet here —
// the VM applies a self-contained Go-impl fn value on its OWN signatures
// (the event-lead spelling `((FnUtil.const 7) 99)` compiled to 99 for the
// interpreter's 7 while the wrapper's label resolved to the registered
// `const` word); fn-util's check-mode ReturnsFns CLAIM the wrapper's arity
// (CheckState.FnShapes); and the check pass models the def-bound wrapper's
// WORD DISPATCH at the read (check's tryShapedFnReadArrival), over the
// wrapper's arity of evaluation-fixed tokens inside the statement, as one
// guarded OpCallDynMethod. The residual classifier's flattened window is
// declined for this class: `bigger 3 ; 5` is the interpreter's
// signature_error, not `bigger 3 5`.

const dcfImport = `import "boru:fn-util"  `
const dcfOn = dcfImport + `def sq x:Integer => [mul x x] end def gt2 fn [[a:Integer b:Integer][Boolean][a gt b]] end def bigger (FnUtil.on gt2/v sq/v) end `
const dcfK = dcfImport + `def k (FnUtil.const 7) end `

// TestDefComputedFnPlainCheckClean pins the fn-carrier read substitution on
// the PLAIN check: `boru check` used to report undefined_word (and an
// unused_def warning) on a correct program whose def is bound to a fn
// CARRIER a native returned, because the substitution ran only on a compile
// pass; the plain check now resolves the read the same way.
func TestDefComputedFnPlainCheckClean(t *testing.T) {
	// The plain check's stack carries the dispatch's ONE result for the
	// read's window (the type-soundness ratchet saw `(bigger 3 5)` as
	// [Function Integer Integer] for the runtime's [Boolean]).
	rows := []struct {
		src   string
		depth int
	}{
		{dcfK + `(k 99)`, 1},
		{dcfOn + `(bigger 3 5)`, 1},
		{dcfK + `k 1 ; 3`, 2},
		{dcfImport + `def f x:Integer => [add 1 x] end def p (FnUtil.partial f/v 10) end (p)`, 1},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		res, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%q: check: %v", c.src, err)
		}
		for _, d := range res.Diagnostics {
			if d.Code == "undefined_word" || d.Code == "unused_def" {
				t.Errorf("%q: the plain check reports %s (%s) on a correct program", c.src, d.Code, d.Detail)
			}
		}
		if len(res.Stack) != c.depth {
			t.Errorf("%q: the plain check leaves %v, want %d value(s)", c.src, res.Stack, c.depth)
		}
	}
}
