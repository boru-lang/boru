package lang

import (
	"errors"
	"strings"
	"testing"
)

// TestNUR229To231OnTheMergedTree pins main's NUR229, NUR230 and NUR231
// (recorded on main by #515's merged cover-gate pass) as they stand on the
// merged tree, where the reverse-order NUR run's case and dispatch work
// answers them on both lanes: a def inside a case clause body is joined
// past the case (NUR229, silent on main), a value-less case scrutinee
// inside a do body is the interpreter's trapped error value (NUR230), and
// `mini` over a member holding the Function TYPE is the interpreter's
// signature_error (NUR231).
func TestNUR229To231OnTheMergedTree(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def x 1 end do [case [1] [1 [def x 5 "one"] "other"]] end x`, "[1 one 5]"},
		{`def x 1 end do [case 2 [1 [def x 5 "one"] "other"]] end x`, "[other 1]"},
		{`def x 1 end case 1 [1 [def x 5 "one"] "other"] end x`, "[1 one 5]"},
		{`do [case [1 drop] [5 "five" "other"] 9]`, "[error(case: value expression produced no value to dispatch on)]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// NUR231's bail is gone: both lanes raise the interpreter's
	// signature_error at mini. The no-match NOTES still differ (NUR311): the
	// compiled report names the window the pass matched, the interpreter's
	// forward collection stopped at the reach and supplied none.
	const mini = `import "boru:minilang" end def m {e: Function} end mini m.e 'ab'`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, mini)
	var ec, ei *BoruError
	if !compiled || len(gotC) != 0 || len(gotI) != 0 || firstErrLine(errC) != firstErrLine(errI) ||
		!errors.As(errC, &ec) || !errors.As(errI, &ei) || ec.Row != ei.Row || ec.Col != ei.Col ||
		!strings.Contains(firstErrLine(errI), "cannot call `mini`") {
		t.Errorf("%s: compiled %v / %v, interpreter %v / %v", mini, gotC, errC, gotI, errI)
	}
}

// TestNUR233PromotedSplitBind pins NUR233's close: a def of a static
// multi-value region's first value (SplitEventRegionBind) whose results the
// planner promoted to frame locals — the spilled value re-pushed under a
// later result — binds from its local, where the splice bind found the
// stack empty (`def x (5 dup)  x add 1` bailed BIND_GLOBAL splice
// underflow where the interpreter answers [5 6]).
func TestNUR233PromotedSplitBind(t *testing.T) {
	const g = `def g fn [[n:Integer] [Boolean] [n gt 5]]  `
	for _, c := range []struct{ src, want string }{
		{`def x (5 dup) x add 1`, "[5 6]"},
		{`def x (5 dup) x`, "[5 5]"},
		{`def x (1 2) x add 10`, "[2 11]"},
		{`def x (1 2 3 dup) x add 10`, "[2 3 3 11]"},
		{`def x (5 dup) 7 x`, "[5 7 5]"},
		{`def x (5 dup) drop x`, "[5]"},
		{g + `def x (5 dup)  if (g 9) [def x 9] [] end x add 1`, "[5 10]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
		interp, compiled, ok := bcsLanes(t, c.src)
		if !ok {
			continue
		}
		if want, got := bcsInstalls(t, interp.RunInterp, "x"), bcsInstalls(t, compiled.RunCompiledStrict, "x"); len(want) != len(got) || (len(want) > 0 && want[0] != got[0]) {
			t.Errorf("%s: next request's install stack of x: compiled %v, interp %v", c.src, got, want)
		}
	}
}
