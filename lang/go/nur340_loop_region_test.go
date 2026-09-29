package lang

import (
	"fmt"
	"strings"
	"testing"
)

// NUR340: a loop's result is modelled as ONE `[:T]` List where the run leaves
// the loop's values themselves, so a dispatch over it — `each (for 1 [mk])
// [x/u]` — matched in the model where the interpreter raises the outer word's
// no-match (or, over a zero-trip loop, "produced no value"). Its forward-form
// body trap was recorded unguarded and won. The match over a region's seat is
// now optimistic (core regionMatch): the trap is the word's rematch, and a
// rematch whose region was WRITTEN after the word — the interpreter spreads a
// paren's values into the call's arguments, a count the window cannot name —
// declines, as the non-trapping sibling (`each (for 1 [1]) [2]`, "consumes
// loop results") always has. Never the body's error.
func TestNUR340LoopRegionOperandDeclines(t *testing.T) {
	for _, c := range []struct{ src, interp string }{
		{`def x 5 end def mk fn [[][Integer][1]] end each (for 1 [mk]) [x/u]`, "cannot call `each`"},
		{`def x 5 end each (for 1 [1]) [x/u]`, "cannot call `each`"},
		{`def x 5 end each (for 2 [1]) [x/u]`, "cannot call `each`"},
		{`each (for 1 [1]) [unpack [z] {a:1}]`, "cannot call `each`"},
		{`each (for 1 [1]) [dup]`, "cannot call `each`"},
		{`each (for 2 [1]) [1 add]`, "cannot call `each`"},
		{`def x 5 end filter (for 1 [1]) [x/u]`, "cannot call `filter`"},
		{`def x 5 end [(each (for 1 [1]) [x/u])]`, "cannot call `each`"},
		{`def x 5 end def n 1 end each (for n [1]) [x/u]`, "cannot call `each`"},
		{`def x 5 end each (while [false] [1]) [x/u]`, "produced no value for each"},
		// A List-valued loop: the model's match holds on this run, but the
		// region's count is still no window the rematch can name.
		{`def x 5 end def mk fn [[][List][[1]]] end each (for 1 [mk]) [x/u]`, "/u requires a function word"},
		{`each (for 2 [[1 2]]) [dup]`, "cannot call `dup`"},
		// ...and over a computed count of zero it was a wrong answer: the
		// body's illegal_ref compiled for the interpreter's "no value".
		{`def x 5 end def mk fn [[][List][[1]]] end def z fn [[][Integer][0]] end each (for (z) [mk]) [x/u]`, "produced no value for each"},
		// A count-varying BRANCH written after the word: the same spread
		// (the rematch read `2` where the interpreter's arguments were `1, 2`).
		{`def x 5 end def c true end each (if c [1 2] [3]) [x/u]`, "cannot call `each`"},
		{`def x 5 end def c false end each (if c [1 2] [3]) [x/u]`, "cannot call `each`"},
		{`def x 5 end def c true end each (if c [[1] [2]] [[3]]) [x/u]`, "/u requires a function word"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !strings.Contains(fmt.Sprint(errI), c.interp) || len(gotI) != 0 {
			t.Errorf("%q: the interpreter raises %q, got %v %v", c.src, c.interp, gotI, errI)
		}
		if compiled {
			t.Errorf("%q: declines, got compiled %v %v", c.src, gotC, errC)
			continue
		}
		if _, reason, _, _ := mustNew(t).CompileCheck(c.src); !strings.Contains(reason, "variadic loop result") {
			t.Errorf("%q: declines over the region operand, got %q", c.src, reason)
		}
	}
}

// The guard's other witnesses keep compiling and agreeing: a declared-Any
// result (NUR264's own), a region BENEATH the word (the rematch reads its top,
// as the interpreter's dispatch reads the stack), a single-count branch in a
// paren, and a non-trapping body over a paren that left nothing — which the
// check pass's recovery reached with a one-operand window and the closure
// record then indexed past (a recovered panic; now each's no_value_error on
// both lanes).
func TestNUR340NeighboursStillAgree(t *testing.T) {
	for _, src := range []string{
		`def x 5 end def mk fn [[][Any][5]] end each (mk) [x/u]`,
		`def mk fn [[][Any][[1 2]]] end each (mk) [dup]`,
		`def x 5 end def c true end if c [99] [1 2] each [x/u]`,
		`def x 5 end def c false end if c [99] [1 2] each [x/u]`,
		`def x 5 end def c true end (if c [99] [1 2]) each [x/u]`,
		`each (for 0 [1]) [2]`,
		`each (1 drop) [2]`,
		`def x 5 end each (for 0 [[1 2]]) [x/u]`,
		`filter (for 0 [1]) [2]`,
		`def x 5 end def mk fn [[][List][[1]]] end def n 0 end each (for n [mk]) [x/u]`,
		`walk (1 drop) [2]`,
		`fold (1 drop) 0 [add]`,
	} {
		requireCompiledParity(t, src)
	}
}
