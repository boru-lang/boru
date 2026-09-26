package lang

import "testing"

// TestNUR243ValidProgramsCompile pins NUR243's close: three valid programs
// the lowering refused compile and agree. A constant branch whose taken arm
// leaves no value is a 0-value statement (its guard carried a covergate
// pragma whose proof was false); a loop's rebind of a name a branch may
// leave unbound is carried like any pre-loop binding, bound-checked after
// the loop; and a parser dispatch declines only over its parser operand
// (TestFnDispatchBranchBoundSourceAgrees).
func TestNUR243ValidProgramsCompile(t *testing.T) {
	const f = `def f fn [[c:Boolean][Any][if c [def x 1] [] for 3 [def x 5] x]] end `
	const g = `def f fn [[c:Boolean][Any][if c [def x 1] [] for 0 [def x 5] x]] end `
	for _, r := range []struct{ src, want string }{
		{`def x 0 if [true] [def x 1] [2] end x`, "[1]"},
		{`def x 0 if [false] [2] [def x 1] end x`, "[1]"},
		{`if [true] [def y 5] [2] end y`, "[5]"},
		{`def f fn [[] [Any] [if [true] [def x 1] [2] 7]] end f`, "[7]"},
		{f + `f true`, "[5]"},
		{f + `f false`, "[5]"},
		{g + `f true`, "[1]"},
		{g + `f false`, "ERROR:undefined_word"},
		{`def i 0 for 3 [def i 9] i`, "[0]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
