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
