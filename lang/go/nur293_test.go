package lang

import "testing"

// TestNUR293ComputedListArmRaisesItsError pins NUR293's close. The compiled
// `if` runs a COMPUTED List arm as `[do <arm>]` (computedArmDoBody), which is
// the interpreter's arm splice on every axis but one: `do` traps a body
// error as an Error value, and the splice raises it. `if true (mk) ["f"]`
// over mk's `quote [1 div 0]` answered `error(division by zero)` compiled
// where the interpreter raises arith_error. The arm runs through __arm now,
// `do` without the trap: its values land as before, and its error raises on
// both lanes. A `do` written in the source keeps its
// trap on both lanes.
func TestNUR293ComputedListArmRaisesItsError(t *testing.T) {
	mk := func(body string) string { return `def mk fn [[][List][quote ` + body + `]] end ` }
	for _, c := range []struct{ src, want string }{
		{mk(`[1 div 0]`) + `if true (mk) ["f"]`, "ERROR:division by zero"},
		{mk(`[1 div 0]`) + `def c true end if c (mk) ["f"]`, "ERROR:division by zero"},
		{mk(`[raise oops "x"]`) + `if true (mk) ["f"]`, "ERROR:x"},
		{mk(`[add 1]`) + `5 if true (mk) ["f"]`, "ERROR:cannot call `add`"},
		{mk(`[1 div 0]`) + `def f fn [[c:Boolean][Any][if c (mk) ["f"]]] end f true`, "ERROR:division by zero"},
		{mk(`[1 2]`) + `def c true end if c (mk) ["f"]`, "[1 2]"},
		{mk(`[1 div 0]`) + `def c false end if c (mk) ["f"]`, "[f]"},
		{`do [1 div 0]`, "[error(division by zero)]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
