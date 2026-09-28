package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestUserCallSourceOfCarriedRootDefCompiles pins the loop-carried ROOT def
// whose value a USER fn call computes — `def i 0  while [i lt 3] [def i
// (inc i)]  i` for any user fn `inc`, and the `apply` spellings callbacks.tsv
// L89 (`def i (i inc/v apply)`) and module-composition.tsv L104 (`def i (i
// M.inc/v apply)`) — which declined "dynamic-scope def `i` of unpromoted
// computed value": the carried store popped the call's result off the sim
// top, and the root write-back (OpBindGlobal) that follows found nothing —
// a native producer promotes to a frame slot on valueDef, but a user call's
// plain store source was left on the stack. The planner promotes a user
// call whose result a carried root def writes back (writeBackSrc), so the
// store loads the slot and the write-back re-pushes it.
func TestUserCallSourceOfCarriedRootDefCompiles(t *testing.T) {
	for _, src := range []string{
		`def inc fn [[n:Integer][Integer][n add 1]] end def i 0 end while [i lt 3] [def i (inc i)] end i`,
		`def inc fn [[n:Integer][Integer][n add 1]] end def i 0 end while [i lt 3] [def i (i inc/v apply)] end i`,
		`import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end def i 0 end while [i lt 3] [def i (i M.inc/v apply)] end i`,
		`def dbl fn [[n:Integer][Integer][n mul 2]] end def x 1 end while [x lt 10] [def x (dbl x)] end x`,
		`def inc fn [[n:Integer][Integer][n add 1]] end def n 0 end for 3 [def n (inc n)] end n`,
		`def inc fn [[n:Integer][Integer][n add 1]] end def n 0 end for 3 [def n (inc n) n] end n`,
		// The same bind inside a fn frame (a frame slot, no write-back) and
		// with a native producer compiled before.
		`def f fn [[][Integer][def inc fn [[n:Integer][Integer][n add 1]] def i 0 while [i lt 3] [def i (inc i)] i]] end f`,
		`def i 0 end while [i lt 3] [def i (i add 1)] end i`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def inc fn [[n:Integer][Integer][n add 1]] end def i 0 end while [i lt 3] [def i (inc i)] end i`)
	if !strings.Contains(dis, "CALL_USER") || !strings.Contains(dis, "BIND_GLOBAL") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the user call's result must feed the carried store and the write-back natively:\n%s", dis)
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
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interpreted %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
	// A carried def of ANOTHER name beside the index compiles as before.
	requireEngineParity(t, `def j 0 end for 3 [def j (j add i)] end j`, true)
}
