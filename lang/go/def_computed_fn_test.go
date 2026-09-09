package lang

import (
	"fmt"
	"strings"
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
// refused for this class: `bigger 3 ; 5` is the interpreter's
// signature_error, not `bigger 3 5`.

const dcfImport = `import "boru:fn-util"  `
const dcfPair = dcfImport + `def addone x:Integer => [add 1 x] end def double x:Integer => [mul 2 x] end `
const dcfSub2 = dcfImport + `def sub2 fn [[a:Integer b:Integer][Integer][a sub b]] end `
const dcfOn = dcfImport + `def sq x:Integer => [mul x x] end def gt2 fn [[a:Integer b:Integer][Boolean][a gt b]] end def bigger (FnUtil.on gt2/v sq/v) end `
const dcfK = dcfImport + `def k (FnUtil.const 7) end `

// TestDefComputedFnParity pins the shapes that now COMPILE, agree on both
// lanes and run VM-native.
func TestDefComputedFnParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{dcfPair + `def h (FnUtil.compose addone/v double/v) end (h 5)`, "11 — compose (the ledger row)"},
		{dcfPair + `def h (FnUtil.pipe addone/v double/v) end (h 5)`, "12 — pipe (the ledger row)"},
		{dcfK + `(k 99)`, "7 — const (the ledger row)"},
		{dcfSub2 + `def fs (FnUtil.flip sub2/v) end (fs 3 10)`, "7 — flip: the usurp wrapper enters sub2's unit reversed (the ledger row)"},
		{dcfSub2 + `def p (FnUtil.partial sub2/v 10) end (p 3)`, "7 — partial (the ledger row)"},
		{dcfOn + `(bigger 3 5)`, "false — on (the ledger row)"},
		{dcfOn + `(bigger 5 3)`, "true — on (the ledger row)"},
		{dcfImport + `def f fn x:Integer Integer [add 1 x] end def m (FnUtil.memoize f/v) end (m 4)`, "5 — memoize (the ledger row)"},
		{dcfImport + `((FnUtil.const 7) 99)`, "7 — the EVENT lead: the wrapper applied on its own signature, not the registered `const` word (compiled 99 before)"},
		{dcfK + `k 99`, "7 — the bare word read at the program's end"},
		{dcfK + `k 1 ; 3`, "7 3 — the read consumes its arity inside the statement; the next statement is its own"},
		{dcfK + `3 (k 99) add`, "10 — the apply's result feeds a later dispatch"},
		{dcfPair + `def h (FnUtil.compose addone/v double/v) end (h 5) (h 6)`, "11 13 — two reads, two applies"},
		{dcfImport + `def f x:Integer => [add 1 x] end def p (FnUtil.partial f/v 10) end (p)`, "11 — a 0-param wrapper applies on its bare read"},
		{dcfImport + `def two x:Integer => [x x] end def h (FnUtil.compose two/v two/v) end (h 5)`, "the wrapper's own type_error, anchored on the read token (`h`, one caret) on both lanes"},
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

// TestDefComputedFnSoundRefusals pins the neighbours that REFUSE, with the
// interpreter's own answer.
func TestDefComputedFnSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// the stack form: the window is short, the interpreter fills from the stack
		{dcfK + `5 k`, "the statement ends short of the wrapper's arity", "[7]"},
		// a param read inside a fn body is not evaluation-fixed
		{dcfK + `def g x:Integer => [(k x)] end (g 1)`, "an argument is not an evaluation-fixed value", "[7]"},
		// a paren group inside the window
		{dcfK + `(k (add 1 2))`, "an argument is not an evaluation-fixed value", "[7]"},
		// the token past the arity stays inside the paren with the result
		{dcfK + `(k 1 2)`, "bounded by a paren", "[7 2]"},
		// an overloaded operand: flip's wrapper has no one arity — no claim
		{dcfImport + `def fs (FnUtil.flip sub/v) end (fs 3 10)`, "closure shape unknown", "[-7]"},
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

// TestDefComputedFnStatementWindowRefusals pins the flattened-window
// miscompile the read model closes: the interpreter dispatches `bigger` at
// the `;` with one argument and raises; the residual classifier saw
// [bigger, 3, 5] and would have applied both.
func TestDefComputedFnStatementWindowRefusals(t *testing.T) {
	for _, src := range []string{dcfOn + `bigger 3 ; 5`, dcfOn + `(bigger 3 ; 5)`} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — the window crosses the statement", src)
			continue
		}
		if !strings.Contains(reason, "the statement ends short of the wrapper's arity") {
			t.Errorf("%q: refused %q", src, reason)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if _, errI := d.RunInterp(src); errI == nil || !strings.Contains(errI.Error(), "cannot call `bigger`") {
			t.Errorf("%q: interpreter answers %v, want the signature_error", src, errI)
		}
	}
}

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
