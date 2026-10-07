package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestUserCallSourceOfCarriedRootDefCompiles pins the loop-carried ROOT var
// whose value a USER fn call computes — `var i 0  while [i lt 3] [var i
// (inc i)]  i` for any user fn `inc`, and the `apply` spellings callbacks.tsv
// L89 (`var i (i inc/v apply)`) and module-composition.tsv L104 (`var i (i
// M.inc/v apply)`) — which declined "dynamic-scope def `i` of unpromoted
// computed value" as defs: the carried store popped the call's result off
// the sim top, and the root write-back that follows found nothing — a
// native producer promotes to a frame slot, but a user call's plain store
// source was left on the stack. The planner promotes a user call whose
// result a carried root cell writes back (writeBackSrc). Since phase 2 of
// design/IMMUTABLE-DEF.1.md the cell is a `var`; a `def` inside the body is
// the iteration's block local.
func TestUserCallSourceOfCarriedRootDefCompiles(t *testing.T) {
	for _, src := range []string{
		`def inc fn [[n:Integer][Integer][n add 1]] end var i 0 end while [i lt 3] [var i (inc i)] end i`,
		`def inc fn [[n:Integer][Integer][n add 1]] end var i 0 end while [i lt 3] [var i (i inc/v apply)] end i`,
		`import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end var i 0 end while [i lt 3] [var i (i M.inc/v apply)] end i`,
		`def dbl fn [[n:Integer][Integer][n mul 2]] end var x 1 end while [x lt 10] [var x (dbl x)] end x`,
		`def inc fn [[n:Integer][Integer][n add 1]] end var n 0 end for 3 [var n (inc n)] end n`,
		`def inc fn [[n:Integer][Integer][n add 1]] end var n 0 end for 3 [var n (inc n) n] end n`,
		// The same assignment inside a fn frame (a frame slot, no
		// write-back) and with a native producer compiled before.
		`def f fn [[][Integer][def inc fn [[n:Integer][Integer][n add 1]] var i 0 while [i lt 3] [var i (inc i)] i]] end f`,
		`var i 0 end while [i lt 3] [var i (i add 1)] end i`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def inc fn [[n:Integer][Integer][n add 1]] end var i 0 end while [i lt 3] [var i (inc i)] end i`)
	if !strings.Contains(dis, "CALL_USER") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the user call's result must feed the carried assignment natively:\n%s", dis)
	}
}

// TestForIndexDefInBodyIsTheIterations pins NUR204's close — the LEXICAL
// index scope, on both lanes: a body def of the enclosing counted loop's
// OWN index (`def i 0  for 3 [def i 9]  i`) rebinds the ITERATION's binding
// and nothing else. The interpreter pops the body's level with the
// iteration and the index level with the loop (popIterLevels), so the
// pre-loop binding shows after it; the compiled loop stores the def into
// its index slot (the loop's own bind name is never loop-carried), so no
// write-back reaches the root. Both lanes used to answer differently, and
// neither the pre-loop value (2 interpreted — the index level survived the
// loop; 9 compiled — the write-back).
func TestForIndexDefInBodyIsTheIterations(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def i 0 end for 3 [def i 9] end i`, "[0]"},
		{`def inc fn [[n:Integer][Integer][n add 1]] end def i 0 end for 3 [def i (inc i)] end i`, "[0]"},
		{`def i 0 end for 3 [def i (i add 1) i] end i`, "[1 2 3 0]"},
		{`def i 0 end for 3 [def i 9 i] end i`, "[9 9 9 0]"},
		{`def f fn [[][Integer][def i 0 for 3 [def i 9] i]] end f`, "[0]"},
		{`def i 0 end for 3 [for 2 [def i 9]] end i`, "[0]"},
		{`def i 0 end for 3 [if true [def i 9] []] end i`, "[0]"},
		{`def i 7 end for 3 [i] end i`, "[0 1 2 7]"},
	} {
		if gotI, errI := mustNew(t).RunInterp(c.src); errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interpreted %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		// The body's def of the index is its block-local shadow (phase 2):
		// the compiled lane agrees where it compiles and declines the
		// shadow where it does not, until its own block scopes land.
		requireEngineParity(t, c.src, false)
	}
	// A carried var of ANOTHER name beside the index compiles as before.
	requireEngineParity(t, `var j 0 end for 3 [var j (j add i)] end j`, true)
}
