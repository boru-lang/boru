package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR208BranchOfFnValues pins NUR208's close: a BRANCH whose arms are
// fn values is placed where the interpreter places it, and applied where
// the interpreter's `apply` word applies it. Two record-side defects:
//
//   - The residual's may-be-fn lead arm (NUR159's) applied a branch result
//     over the entry after it on the event's flag alone, asking no
//     placement question; a user paren PLACES its value (the paren re-step
//     rule), so `(if c f g) 5` is `[fn 5]` on the interpreter and was `[5]`
//     compiled. The arm asks leadPlacedNotRead now, as its siblings do.
//   - `apply` over the branch result was elided: its identity result
//     carries the branch's own id, which the registered-output arm took for
//     the branch's, and the application was lost (`[fn]` for 42). The lead
//     registers the word's pending application now, as a produced closure's
//     does, and lowers as the apply word's own op.
func TestNUR208BranchOfFnValues(t *testing.T) {
	const (
		cT  = `def c true end `
		mOn = `def m {e: true} end `
		mOf = `def m {e: false} end `
		one = `([x:Integer] => [x]) ([x:Integer] => [0])`
		nul = `([] => [42]) ([] => [2])`
	)
	for _, r := range []struct{ src, want string }{
		// Placed by the paren: data beside the value after it.
		{cT + `(if c ` + one + `) 5`, "[fn (Integer) 5]"},
		{mOn + `(if (m 'e' get) ` + one + `) 5`, "[fn (Integer) 5]"},
		// Re-stepped by an enclosing paren: applied.
		{cT + `((if c ` + one + `) 5)`, "[5]"},
		// The apply word applies whichever arm ran.
		{cT + `(if c ` + nul + `) apply`, "[42]"},
		{mOn + `(if (m 'e' get) ` + nul + `) apply`, "[42]"},
		{mOf + `(if (m 'e' get) ` + nul + `) apply`, "[2]"},
		{mOn + `5 (if (m 'e' get) ([x:Integer] => [x add 1]) ([x:Integer] => [0])) apply`, "[6]"},
		// Data arms: apply raises its no-match on both lanes.
		{cT + `(if c [1] [2]) apply`, "ERROR:cannot call `apply`"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}

	// Arms that disagree on being a fn under `apply` have no one model for
	// the application: the compile declines loudly, as before.
	src := mOn + `(if (m 'e' get) ([] => [42]) [7]) apply`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	if !noteCompileDefect(t, src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), "unmatched dispatch") {
		t.Errorf("%s: want the standing decline, got %v compiled=%v err=%v", src, gotC, compiled, errC)
	}
	if gotI, errI := mustNew(t).RunInterp(src); errI != nil || fmt.Sprint(gotI) != "[42]" {
		t.Errorf("%s: interp got %v / %v, want [42]", src, gotI, errI)
	}
}
