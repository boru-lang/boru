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
// At the merge of main's #514 main's rules superseded that seat: a NAMED
// zero-argument fn value in the run fires across the `end` (`do (mk) end x`
// over `[g/v]` is the interpreter's [7 5], which the statement-boundary seat
// answered [fn g 5], silent). A run whose tokens are not proven to leave only
// parking values compiled under the plain-run check (NUR213), which deferred
// loudly when it left a fn value, as the lambda here does. The do's count
// island now takes such a run (NUR348, compiler planCountRestarts): the
// statement runs again on the interpreter with the run written in the do's
// place, where its step re-steps the fn exactly as the interpreter's does.
func TestNUR266RunLeadStopsAtTheStatementEnd(t *testing.T) {
	const lam = `def mk fn [[][List][quote [([n:Integer] => [n add 1])]]] end def x 5 end `
	for _, c := range []struct{ prog, want string }{
		{`do (mk) end x`, "[fn (Integer) 5]"},     // the run's fn stays data
		{`do (mk) end x x`, "[fn (Integer) 5 5]"}, // every read past the end
		{`do (mk) ; x`, "[fn (Integer) 5]"},       // `;` ends the statement as `end` does
		{`do (mk) x`, "[6]"},                      // no boundary: the fn takes x forward
		{`do (mk) end 7`, "[fn (Integer) 7]"},     // a literal after the end
	} {
		agreeOnBothLanes(t, lam+c.prog, c.want)
	}
	agreeOnBothLanes(t, `def g fn [[][Integer][7]] end def mk fn [[][List][quote [g/v]]] end def x 5 end do (mk) end x`, "[7 5]")
	// A run of zero-argument anonymous lambdas parks wherever it lands, and
	// seats as data (NUR282).
	requireEngineParity(t, `def mk fn [[][List][quote [([] => [42])]]] end def x 5 end do (mk) end x`, true)
	requireEngineParity(t, `def mk fn [[][List][quote [([] => [42])]]] end def x 5 end do (mk) x`, true)
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
