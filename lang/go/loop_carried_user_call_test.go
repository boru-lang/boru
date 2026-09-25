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

// TestForIndexDefInBodyPending pins NUR204 as it stands: a body def of the
// enclosing counted loop's OWN index — `def i 0  for 3 [def i 9]  i` — is
// the interpreter's 2 (the loop leaves its index level bound past the loop:
// the last index; inside a nested loop the outer loop's restore, 0) where
// the compiled loop carried the def and wrote the body's value back (9), a
// silent miscompile on main (3768c46) for a native or a user-call value
// alike. The shape declines loudly now, on both paths and inside a fn,
// until the index level is modelled; closing it must update this pin.
func TestForIndexDefInBodyPending(t *testing.T) {
	for _, c := range []struct {
		src  string
		want string
	}{
		{`def i 0 end for 3 [def i 9] end i`, "[2]"},
		{`def inc fn [[n:Integer][Integer][n add 1]] end def i 0 end for 3 [def i (inc i)] end i`, "[2]"},
		{`def i 0 end for 3 [def i (i add 1) i] end i`, "[1 2 3 2]"},
		{`def f fn [[][Integer][def i 0 for 3 [def i 9] i]] end f`, "[2]"},
		{`def i 0 end for 3 [for 2 [def i 9]] end i`, "[0]"},
		{`def i 0 end for 3 [if true [def i 9] []] end i`, "[2]"},
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: the interpreter's index level: want %s, got %v / %v", c.src, c.want, gotI, errI)
		}
		requireCompileDefect(t, c.src, gotC, errC)
		if errC == nil || !strings.Contains(errC.Error(), "own index `i`") {
			t.Errorf("%q: NUR204's shape must decline as the loop's own index: %v", c.src, errC)
		}
	}
	// A carried def of ANOTHER name beside the index compiles as before.
	requireEngineParity(t, `def j 0 end for 3 [def j (j add i)] end j`, true)
}
