package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR301HandlerRunsNotSeatedAsOne pins NUR301. An error handler's run on
// the caught path is the handler's own count, where the pass modelled one
// value, and the compiled lane seated it as one: a literal handler the
// closure path refused ran as an interpreter island whose values were not
// a region (`[do [raise oops 'x'] error [drop 5 6]]` was [5 [6]] for
// [[5 6]]), and a `do` whose literal body ends in a computed handler's run
// took one seat (`[do [do [raise oops 'x'] error (mk)] error (mk)]`, the
// same). Silent on main and on this branch. The island's run is a region
// and a run now (TryRecordFallback), a computed handler's run is a run
// (dynBodyRun), a literal body holding a run the closure path declines
// takes a computed body's marks (noteBodyRun), and a strip over a growing
// region stays one.
func TestNUR301HandlerRunsNotSeatedAsOne(t *testing.T) {
	const two = `def mk fn [[][List][quote [drop 5 6]]] end `
	const g = `def g fn [[x:Integer][Integer][x add 100]] end def mk2 fn [[][List][quote [drop g/v]]] end `
	for _, c := range []struct{ src, want string }{
		{two + `[do [do [raise oops 'x'] error (mk)] error (mk)]`, "[[5 6]]"},
		{two + `[do [do [raise oops 'x'] error (mk)]]`, "[[5 6]]"},
		{two + `(do [do [raise oops 'x'] error (mk)]) add 1`, "[5 7]"},
		{two + `do [do [raise oops 'x'] error (mk)] error [drop 9]`, "[5 6]"},
		{`[do [do [raise oops 'x'] error [drop 5 6]]]`, "[[5 6]]"},
		{`do [raise oops 'x'] error [drop 5 6]`, "[5 6]"},
		// A value beneath a computed handler's run: the prefix island.
		{two + `[3 do [raise oops 'x'] error (mk)]`, "[[3 5 6]]"},
		{two + `3 4 do [raise oops 'x'] error (mk)`, "[3 4 5 6]"},
		{g + `3 do [raise oops 'x'] error (mk2)`, "[103]"},
		{g + `[3 do [raise oops 'x'] error (mk2)]`, "[[103]]"},
		// The handlers that net one value keep compiling.
		{`(do [raise bad_input 'boom'] error [dot code]) eq bad_input/q`, "[true]"},
		{`[do [raise "boom"] error ['fallback']]`, "[['fallback']]"},
		{`[do [9223372036854775807 add 1] error [drop 42]]`, "[[42]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, reason, want string }{
		{`def r (do [raise oops 'x'] error [drop 5 6]) r`, "consumes loop results", "[6 5]"},
		{`3 do [raise oops 'x'] error [drop 5 6]`, "residual shape beyond Stage 1", "[3 5 6]"},
	} {
		requireLoudDecline(t, c.src, c.reason, c.want)
	}
	// A single-value seat takes the island's run under the runtime count
	// check (FallbackSpan.CheckOne): a path whose handler leaves one value
	// runs compiled, and one that leaves two is the loud designed defer —
	// the island's own, where the interpreter answers want.
	for _, c := range []struct{ src, what, want string }{
		{`[do [raise oops 'x'] error [drop 9 8]]`, "error's island left 2", "[[9 8]]"},
		{`(do [raise oops 'x'] error [drop 5 6]) add 1`, "error's island left 2", "[5 7]"},
		{`[do [do [raise oops 'x'] error [drop 5 6]] error [drop 9]]`, "do over a computed body left 2", "[[5 6]]"},
		{`def f fn [[b:Boolean][Any][[do [if b [raise oops 'x'] [7]] error [drop 5 6]]]] end [f true f false]`, "error's island left 2", "[[[5 6] [7]]]"},
	} {
		prog, reason, _, err := mustNew(t).CompileCheck(c.src)
		if prog == nil || err != nil {
			t.Errorf("%q: want a compiled program, got decline %q / %v", c.src, reason, err)
			continue
		}
		gotC, _, errC := mustNew(t).RunCompiled(c.src)
		if !isBailDefect(errC) || !strings.Contains(errC.Error(), c.what) || len(gotC) != 0 {
			t.Errorf("%q: want the loud dyn-body-one defer (%s), got %v / %v", c.src, c.what, gotC, errC)
		}
		if gotI, errI := mustNew(t).RunInterp(c.src); errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interpreter answered %v / %v, want %s", c.src, gotI, errI, c.want)
		}
	}
	for _, c := range []struct{ src, want string }{
		// The nested computed form compiles as the region it is.
		{two + `[do [do [raise oops 'x'] error (mk)] error [drop 9]]`, "[[5 6]]"},
		// A path whose handler is not taken seats the body's one value.
		{`def f fn [[b:Boolean][Any][[do [if b [raise oops 'x'] [7]] error [drop 5 6]]]] end [f false]`, "[[[7]]]"},
		// The mini-s3 shape: a checked computed body under a one-value
		// literal handler binds as it did before the island was a region.
		{`def risky fn [[b:Any][Any][def ok (do b error [ drop false ]) if ok [ 1 ] [ 0 ]]] end risky [false]`, "[0]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
