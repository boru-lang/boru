package lang

import "testing"

// TestNUR329NoMatchNamesTheReach pins NUR329's close: a no-match over a
// reach-led forward operand names the reach's VALUE on both lanes. The
// interpreter evaluates the reach before its match, fails it and reports
// "the argument was 1"; the check pass holds the reach's result as a
// carrier, which its stack-first gatherer left out of the failed window, so
// the static trap reported the stack's 7. The written run now joins the
// window when it holds a carrier, and the failure goes to the runtime
// rematch, which renders the run's value.
func TestNUR329NoMatchNamesTheReach(t *testing.T) {
	const m = `def m {a: 1} end `
	for _, src := range []string{
		m + `def f fn [[x:Type][Any][[x]]] end 7 f m.a`,
		m + `def f fn [[x:None][Any][[x]]] end 7 f m.a`,
		m + `def f fn [[x:[:Maybe]][Any][[x]]] end 7 f m.a`,
		m + `def f fn [[x:Type][Any][[x]]] end [7 f m.a]`,
		m + `def f fn [[x:Type][Any][[x]]] end 7 f (m.a)`,
		m + `def f fn [[x:Type][Any][[x]]] end 7 f m.a 9`,
		// Unchanged: no written run, a concrete one, two operands.
		m + `def f fn [[x:Type][Any][[x]]] end f m.a`,
		m + `def f fn [[x:None][Any][[x]]] end 7 f 1`,
		m + `def f fn [[x:Type y:Integer][Any][[x]]] end 7 f m.a`,
		m + `def f fn [[x:Type][Any][[x]]] end m.a f`,
	} {
		agreeOnBothLanes(t, src, "ERROR:cannot call `f`")
	}
	// A matching reach still runs.
	agreeOnBothLanes(t, `def N Integer def m {a: "s"} end def f fn [[x:N][Any][[x]]] end 7 f m.a`, "[[7] s]")
	agreeOnBothLanes(t, `def m {a: Integer} end def f fn [[x:Type][Any][[x]]] end 7 f m.a`, "[7 [Integer]]")
}

// TestNUR329NoMatchNamesTheUnexpandedReach pins NUR329's remainder: after a
// reach the forward phase evaluated, a reach (or template) it never reached
// is named by its token — the interpreter reports `1 and m.b (a Reach)` plus
// the arity note. The stack-only window used to keep the static trap, which
// named the 7 (`7 f m.a m.b`), and a window already past the word lost the
// token for the call's no-match window (`f m.a m.b` named the 1 alone). The
// rematch now carries the token as a constant (unexpandedToken) and the
// call window as itself. agreeOnBothLanes compares the whole report, notes
// included.
func TestNUR329NoMatchNamesTheUnexpandedReach(t *testing.T) {
	const m = `def m {a: 1 b: 2} end `
	for _, c := range []struct{ src, note string }{
		{m + `def f fn [[x:Type][Any][[x]]] end 7 f m.a m.b`, "the arguments were 1 (an Integer) and m.b (a Reach)"},
		{m + `def f fn [[x:None][Any][[x]]] end 7 f m.a m.b`, "the arguments were 1 (an Integer) and m.b (a Reach)"},
		{m + `def f fn [[x:Type][Any][[x]]] end 7 f m.b m.a`, "the arguments were 2 (an Integer) and m.a (a Reach)"},
		{m + `def f fn [[x:Type][Any][[x]]] end 7 f m.a m.b m.a`, "the arguments were 1 (an Integer), m.b (a Reach) and m.a (a Reach)"},
		{m + `def f fn [[x:Type][Any][[x]]] end 7 f m.a m.b end 5`, "the arguments were 1 (an Integer) and m.b (a Reach)"},
		{m + "def f fn [[x:Type][Any][[x]]] end 7 f m.a `s${m.a}`", "the arguments were 1 (an Integer) and `s${m.a}`"},
		{`def m {a: 1 b: {c: 2}} end def f fn [[x:Type][Any][[x]]] end 7 f m.a m.b.c`, "the arguments were 1 (an Integer) and m.b.c (a Reach)"},
		{m + `def f fn [[x:Type][Any][[x]]] end f m.a m.b`, "the arguments were 1 (an Integer) and m.b (a Reach)"},
		{m + `def f fn [[x:Type][Any][[x]]] end def g fn [[] [Any] [f m.a m.b]] end g`, "the arguments were 1 (an Integer) and m.b (a Reach)"},
		// Negative: a paren after the reach is no written operand, and a
		// concrete run keeps the static report.
		{m + `def f fn [[x:Type][Any][[x]]] end 7 f m.a (m.b)`, "the argument was 1 (an Integer)"},
		{m + `def f fn [[x:Type][Any][[x]]] end 7 f 3 m.b`, "the arguments were 3 (an Integer) and m.b (a Reach)"},
	} {
		agreeOnBothLanes(t, c.src, "ERROR:"+c.note)
	}
	// A reach the forward phase does take still runs.
	agreeOnBothLanes(t, m+`def f fn [[x:Type][Any][[x]] [x:Integer y:Integer][Any][[x y]]] end 7 f m.a m.b`, "[7 [1 2]]")
	agreeOnBothLanes(t, `def m {a: Integer b: 2} end def f fn [[x:Type][Any][[x]]] end 7 f m.a m.b`, "[7 [Integer] 2]")
}
