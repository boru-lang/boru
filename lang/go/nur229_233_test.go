package lang

import "testing"

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
		{`var x 1 end do [case [1] [1 [var x 5 "one"] "other"]] end x`, "[1 one 5]"},
		{`var x 1 end do [case 2 [1 [var x 5 "one"] "other"]] end x`, "[other 1]"},
		{`var x 1 end case 1 [1 [var x 5 "one"] "other"] end x`, "[1 one 5]"},
		{`do [case [1 drop] [5 "five" "other"] 9]`, "[error(case: value expression produced no value to dispatch on)]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// NUR231's bail is gone: both lanes raise the interpreter's
	// signature_error at mini, notes and all — the report's tuple stops at
	// the reach's type value as the interpreter's does (NUR311).
	agreeOnBothLanes(t, `import "boru:minilang" end def m {e: Function} end mini m.e 'ab'`, "ERROR:cannot call `mini`")
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
		{g + `var x (5 dup)  if (g 9) [var x 9] [] end x add 1`, "[5 10]"},
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
