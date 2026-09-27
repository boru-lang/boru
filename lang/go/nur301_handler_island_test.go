package lang

import "testing"

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
		{`[do [raise oops 'x'] error [drop 9 8]]`, "consumes loop results", "[[9 8]]"},
		{`def r (do [raise oops 'x'] error [drop 5 6]) r`, "consumes loop results", "[6 5]"},
		{`3 do [raise oops 'x'] error [drop 5 6]`, "residual shape beyond Stage 1", "[3 5 6]"},
		{`(do [raise oops 'x'] error [drop 5 6]) add 1`, "consumes loop results", "[5 7]"},
		{`[do [do [raise oops 'x'] error [drop 5 6]] error [drop 9]]`, "consumes loop results", "[[5 6]]"},
		{two + `[do [do [raise oops 'x'] error (mk)] error [drop 9]]`, "consumes loop results", "[[5 6]]"},
		{`def f fn [[b:Boolean][Any][[do [if b [raise oops 'x'] [7]] error [drop 5 6]]]] end [f true f false]`, "consumes loop results", "[[[5 6] [7]]]"},
	} {
		requireLoudDecline(t, c.src, c.reason, c.want)
	}
}
