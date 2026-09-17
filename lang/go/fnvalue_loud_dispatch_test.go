package lang

import (
	"fmt"
	"strings"
	"testing"
)

// fnvalue_loud_dispatch_test.go — the RUNTIME half of
// design/legacy/FN-VALUE-DISPATCH.0.ignore that a spec row cannot express: a failed
// fn-value dispatch raises at the CALL, so an enclosing `do […] error […]`
// traps it like any other failure. The old end-of-run residue drain raised
// outside every handler, which aborted a program that had explicitly asked
// to handle the case.
//
// Not a `lang/spec` row because check mode evaluates a code body's parens on
// its own straight line and therefore reports the call as an error while the
// run traps it — the spec harness would read that as a checker false
// positive. The divergence predates this contract (verified against the
// previous binary) and is recorded in the design note's §6.

const loudMod = `import module [ def w fn [[x:Integer] [Integer] [x]] export "M" {w: w/v} ] end  `

// A do-catch around the failing call yields the error's code as a value, and
// the program completes.
func TestFailedFnValueDispatchIsCatchable(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.RunInterp(loudMod + `do [(M.w "x")] error [dot code]`)
	if err != nil {
		t.Fatalf("the do-catch did not trap the dispatch failure: %v", err)
	}
	if fmt.Sprint(got) != "[uncalled_function]" {
		t.Errorf("got %v, want the caught code [uncalled_function]", got)
	}
}

// Without a handler the same call aborts, at the call's own position.
func TestFailedFnValueDispatchRaisesUncaught(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.RunInterp(loudMod + `M.w "x"`)
	if err == nil {
		t.Fatal("an unhandled failed dispatch must abort the run")
	}
	if !strings.Contains(err.Error(), "uncalled_function") {
		t.Errorf("err = %v, want uncalled_function", err)
	}
	// The hint names the escape hatch, because the other reading of a failed
	// dispatch is "I meant to pass the function itself".
	if !strings.Contains(err.Error(), "w/v") {
		t.Errorf("err = %v, want the /v hint", err)
	}
}
