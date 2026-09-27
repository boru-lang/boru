package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR222ValuelessDoSeatedDeclines pins NUR222: a value-less `do` body
// (non-empty, empty residual, no certain raise) is modelled as the one
// Error a caught raise would leave, latched runtime-variable — and a clean
// run leaves nothing. Seated at a fixed count — promoted to a local in a
// branch fragment, dropped under a condition's decision value — the phantom
// compiled and bailed at run time (STORE_LOCAL / DROP underflow). Each such
// seat declines now, loudly, and the program answers through the
// interpreter. A word that CONSUMES the phantom as its operand (`1 do
// [(1 add 1) drop] drop`) compiles through the do's count island
// (TestNUR222ConsumedPhantomCountIsland). A body of literals and plain stack
// shuffles has no phantom at all (2026-09-27): run isolated, it cannot
// raise, so `do` nets what the pass saw — nothing.
func TestNUR222ValuelessDoSeatedDeclines(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`if [do [(1 add 1) drop] true] [2] [3]`, "[2]"},
		{`if [true] [do [(1 add 1) drop] 2] [3]`, "[2]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if compiled || codeOf(errC) != "compile_failed" {
			t.Errorf("%q: the phantom's fixed seat declines, got compiled=%v %v %v", c.src, compiled, gotC, errC)
		}
		requireCompileDefect(t, c.src, gotC, errC)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: the interpreter answers %s, got %v %v", c.src, c.want, gotI, errI)
		}
	}
	// Negative: where the latch's count is absorbed — the program's residual,
	// an arm whose out IS the do — and where the raise is certain (a static
	// zero division is `raise`'s twin: one Error, never the phantom), the
	// program compiles with parity.
	for _, src := range []string{
		`do [1 drop]`,
		`do [1 drop] 2`,
		`2 do [1 drop]`,
		`if true [do [1 drop]] [3]`,
		// A shuffle-only body nets nothing on both lanes, so its every seat
		// holds: the consumer, the condition, the arm and the literal.
		`1 do [1 drop] drop`,
		`1 do [1 2 swap drop drop] add 1`,
		`if [do [1 drop] true] [2] [3]`,
		`if [true] [do [1 drop] 2] [3]`,
		`[do [1 drop] 7]`,
		`for 1 [do [for 2 [raise 'x']] drop i]`,
		`(do [1 0 div]).code`,
		`do [1 0 div] error [dot code]`,
		`if [true] [do [raise 'x'] 2] [3]`,
	} {
		requireEngineParity(t, src, true)
	}
	// A certain raise ends the body: a def after it never happens.
	src := `do [(0 div 0) def q 5] end q`
	_, errI := mustNew(t).RunInterp(src)
	_, _, _, errC := mustNew(t).CompileCheck(src)
	if codeOf(errI) != "undefined_word" || (errC != nil && !strings.Contains(errC.Error(), "q")) {
		t.Errorf("%s: the def after a certain raise is never made, got interp %v, check %v", src, errI, errC)
	}
}

// TestNUR222ConsumedPhantomCountIsland pins NUR222's consumer half. A call
// that takes a caught body's phantom as an operand checks the run's count at
// run time (SigRef.CountCheck). A miss re-runs the do's statement on the
// interpreter from its first token, with the do word and its body written as
// the run the call left (RestartResults), so the body never runs twice. A
// paren before the do is written as its value (`(g) 1 do [...] drop`, which
// answered `[1]` for `[5]` silently until the island took it). A seat no
// island can take — a trap before the statement, a call before the do that
// took an operand off the stack, a loop — defers loudly (vm:do-count).
func TestNUR222ConsumedPhantomCountIsland(t *testing.T) {
	const g = `def g fn [[][Any][5]] end `
	for _, c := range []struct{ src, want string }{
		{`1 do [(1 add 1) drop] drop`, "[]"},
		{g + `1 do [(g) drop] drop`, "[]"},
		{g + `1 do [g drop] drop`, "[]"},
		{`1 do [def x 5] drop x`, "[5]"},
		{`1 do [(raise 'x') drop] drop`, "[1]"},
		{`1 do [print "p" 2 drop] drop`, "[]"},
		{`1 do [(1 add 1) drop] drop 9`, "[9]"},
		{`def n (flex []) end 1 do [push 1 n] drop n`, "[1 [1]]"},
		{`1 do [(1 add 1) drop] drop ; 2 do [(1 add 1) drop] drop`, "[]"},
		{`def f fn [[][Any][1 do [(1 add 1) drop] drop 7]] end f`, "[7]"},
		{`1 do [(1 add 1) drop] typeof`, "[Integer]"},
		{`1 do [(0 div 0) drop] typeof`, "[1 Error]"},
		// The island's own run raises the interpreter's error.
		{`do [(1 add 1) drop] drop`, "ERROR:cannot call `drop`"},
		// A paren before the do, and an effect, are written as their
		// runs (NUR296's call run): neither runs twice.
		{g + `(g) 1 do [(1 add 1) drop] drop`, "[5]"},
		{g + `[(g) 1 do [(1 add 1) drop] drop]`, "[[5]]"},
		{`print "p" 1 do [(1 add 1) drop] drop`, "[]"},
		// A call before the do that took a stack operand written right
		// before it is written as its run too (callRun).
		{`1 add 2 [1 do [(1 add 1) drop] drop]`, "[3 []]"},
		{`"x" print/s 1 do [(1 add 1) drop] drop`, "[]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, c := range []struct{ src, want, loud string }{
		{`1 2 do [(1 add 1) drop] add`, "[3]", "DISPATCH_REMATCH underflow"},
		// A top-level list literal's token carries no position, so no
		// statement island can find the do inside it; nor can one re-run a
		// call before the do whose operand lies beneath the statement.
		{`[1 do [(1 add 1) drop] drop]`, "[[]]", "a caught body's run left 0 value(s)"},
		{`1 end add 2 [1 do [(1 add 1) drop] drop]`, "[3 []]", "a caught body's run left 0 value(s)"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreted %v %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || codeOf(errC) != "internal_error" || !strings.Contains(errC.Error(), c.loud) {
			t.Errorf("%s: loud compiled (%s), got compiled=%v %v %v", c.src, c.loud, compiled, gotC, errC)
		}
	}
}
