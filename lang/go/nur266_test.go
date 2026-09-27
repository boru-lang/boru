package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR266RunLeadStopsAtTheStatementEnd pins NUR266: a computed `do`
// body's run re-steps at the `do`, so a fn value it leaves last collects
// nothing past the statement's `end`. The compiled lane applied it to a
// def-bound read written after the `end` (`do (mk) end x` over a run ending
// in `[n:Integer] => [n add 1]` answered 6 for the interpreter's `[fn 5]`):
// the statement-boundary rule (crossesStatementEnd, NUR187) could not place
// a def read, whose reads share one value. The recorder kept each read's
// position, and a read wholly past the boundary proved the crossing.
//
// At the merge of main's #514 main's region rule supersedes the compile:
// a run that may leave a callable seats only as the residual's LAST entries
// (lower.go dynRegionNotLast), because a NAMED zero-argument fn value in the
// run fires across the `end` (`do (mk) end x` over `[g/v]` is the
// interpreter's [7 5], and the statement-boundary seat answered [fn g 5]).
// Every row below declines loudly now.
func TestNUR266RunLeadStopsAtTheStatementEnd(t *testing.T) {
	const lam = `def mk fn [[][List][quote [([n:Integer] => [n add 1])]]] end def x 5 end `
	for _, prog := range []string{
		`do (mk) end x`,   // the run's fn stays data
		`do (mk) end x x`, // every read past the end
		`do (mk) ; x`,     // `;` ends the statement as `end` does
		`do (mk) x`,       // no boundary: the fn takes x forward, 6
		`do (mk) end 7`,   // a literal after the end
	} {
		requireDeclineReason(t, lam+prog, dynRegionNotLastReason)
	}
	requireDeclineReason(t, `def g fn [[][Integer][7]] end def mk fn [[][List][quote [g/v]]] end def x 5 end do (mk) end x`, dynRegionNotLastReason)
	// A run of plain values and a paren's fn result are unchanged.
	requireEngineParity(t, `def mk fn [[][List][quote [1 2]]] end def x 5 end do (mk) end x`, true)
	requireEngineParity(t, `def mk fn [[][Any][([n:Integer] => [n add 1])]] end def x 5 end (mk) end x`, true)

	// Negative (the open half): x read on both sides of the `do`. The run's
	// fn applies to the x beneath on the interpreter ([6 5]); the compiled
	// lane cannot seat the run between the two reads, and declines loudly
	// (it answered [5 6] silently before the reads after a computed body
	// read live). Never another answer.
	src := lam + `x do (mk) end x`
	gi, ei := mustNew(t).RunInterp(src)
	gc, ec := mustNew(t).Run(src)
	if fmt.Sprint(gi) != "[6 5]" || ei != nil {
		t.Fatalf("%s: the interpreter applies the run's fn to the x beneath: %v / %v", src, gi, ei)
	}
	if !(fmt.Sprint(gc) == fmt.Sprint(gi) && ec == nil) && (ec == nil || !strings.Contains(ec.Error(), "compile_failed")) {
		t.Errorf("%s: the interpreter's answer or a loud decline, never another answer; got %v / %v", src, gc, ec)
	}
}

// dynRegionNotLastReason is the substring of compiler lower.go
// dynRegionNotLast, the region rule's decline for entries above a computed
// run that may leave a callable.
const dynRegionNotLastReason = "seat only as the residual's last entries"

// requireDeclineReason asserts src declines at compile time with a reason
// holding wantReason, the compiled lane fails loudly as compile_failed, and
// the interpreter runs it (to a value or an error of its own).
func requireDeclineReason(t *testing.T, src, wantReason string) {
	t.Helper()
	prog, reason, _, err := mustNew(t).CompileCheck(src)
	if prog != nil || err != nil || !strings.Contains(reason, wantReason) {
		t.Errorf("%q: want a compile decline naming %q, got prog=%v reason=%q err=%v", src, wantReason, prog != nil, reason, err)
		return
	}
	if _, _, errC := mustNew(t).RunCompiled(src); codeOf(errC) != "compile_failed" {
		t.Errorf("%q: the compiled lane must fail loudly as compile_failed, got %v", src, errC)
	}
}
