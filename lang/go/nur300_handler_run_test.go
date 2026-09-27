package lang

import "testing"

// TestNUR300ComputedHandlerRun pins NUR300. A computed error handler's run
// leaves 0-or-more values, which the interpreter splices onto its tape and
// steps, where the check pass models one. The compiled lane seated it as
// one plain value, so its consumers answered silently wrong on main and on
// this branch: `[do [raise oops 'x'] error (mk)]` over a two-value handler
// was `[5 [6]]` for the interpreter's `[[5 6]]`. The run is a variadic
// region now, as a computed `do` body's is (recordDynBodyCall): a residual
// seats it whole, a single-value seat takes it under the runtime count
// check, and a layout that cannot place it declines. Where the check
// refuses a run, the error's count island re-runs the statement with the
// do, its body, the word and the handler written as the run (handlerRun).
func TestNUR300ComputedHandlerRun(t *testing.T) {
	const g = `def g fn [[][Integer][7]] end `
	const two = `def mk fn [[][List][quote [drop 5 6]]] end `
	const one = `def mk fn [[][List][quote [drop 5]]] end `
	const fnv = `def mk fn [[][List][quote [drop g/v]]] end `
	for _, c := range []struct{ src, want string }{
		{two + `do [raise oops 'x'] error (mk)`, "[5 6]"},
		{two + `do [raise oops 'x'] error (mk) 9`, "[5 6 9]"},
		{two + `for 2 [do [raise oops 'x'] error (mk)]`, "[5 6 5 6]"},
		{two + `if true [do [raise oops 'x'] error (mk)] [0]`, "[5 6]"},
		{two + `def f fn [[][Any][do [raise oops 'x'] error (mk)]] end f`, "ERROR:expected 1 return value(s), got 2"},
		{two + `(do [7] error (mk)) add 1`, "[8]"},
		{one + `[do [raise oops 'x'] error (mk)]`, "[[5]]"},
		{one + `def r (do [raise oops 'x'] error (mk)) r`, "[5]"},
		{one + `(do [raise oops 'x'] error (mk)) add 1`, "[6]"},
		{g + one + `do [g/v] error (mk)`, "[7]"},
		// The silent forms, through the count island: the run is written in
		// the do's place, and the interpreter steps it there.
		{two + `[do [raise oops 'x'] error (mk)]`, "[[5 6]]"},
		{two + `def r (do [raise oops 'x'] error (mk)) r`, "[6 5]"},
		{two + `(do [raise oops 'x'] error (mk)) add 1`, "[5 7]"},
		{two + `do [raise oops 'x'] error (mk) add 1`, "[5 7]"},
		{two + `3 [do [raise oops 'x'] error (mk)]`, "[3 [5 6]]"},
		{two + `(mk) drop [do [raise oops 'x'] error (mk)]`, "[[5 6]]"},
		{g + fnv + `[do [raise oops 'x'] error (mk)]`, "[[7]]"},
		{g + fnv + `(do [raise oops 'x'] error (mk)) typeof`, "[Integer]"},
		{`def mk fn [[][List][quote [drop]]] end [do [raise oops 'x'] error (mk)]`, "[[]]"},
		{`def f fn [[h:List][Any][[do [raise oops 'x'] error h]]] end f (quote [drop 5 6])`, "[[5 6]]"},
		{`def f fn [[h:List][Any][(do [raise oops 'x'] error h) add 1]] end f (quote [drop 5 6])`, "ERROR:expected 1 return value(s), got 2"},
		{`def f fn [[h:List][Any][def r (do [raise oops 'x'] error h) r]] end f (quote [drop 5 6])`, "ERROR:expected 1 return value(s), got 2"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A seat no island reaches (a map literal's value) takes the runtime
	// check's loud defer.
	requireCheckedOneDefer(t, two+`{a: (do [raise oops 'x'] error (mk))}`, "[{a:[5 6]}]")
	// A value beneath the run: main answered [5 3 6].
	requireLoudDecline(t, two+`3 do [raise oops 'x'] error (mk)`, "residual shape beyond Stage 1", "[3 5 6]")
}
