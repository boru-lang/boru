package lang

import (
	"strings"
	"testing"
)

// TestNUR292ComputedIfConditionOrArmThatIsAList pins NUR292's loud half. `if`
// runs a list condition inline — its words take the values beneath the if —
// and splices a list arm in parens, whatever produced the list; the compiled
// branch held a computed list as a value (`if (mk) ["t"] ["f"]` over
// `[false]` answered "t" for "f"). A condition or arm the pass holds
// abstractly that may be a list runs a guard (__codeguard) that passes any
// other value — which answers as before — and defers on a list. An arm the
// branch does not take is never checked, as the interpreter never splices
// it; an arm the pass types List keeps its `[do <arm>]` path.
func TestNUR292ComputedIfConditionOrArmThatIsAList(t *testing.T) {
	mk := func(v string) string { return `def mk fn [[][Any][` + v + `]] end ` }
	for _, src := range []string{
		mk(`[false]`) + `if (mk) ["t"] ["f"]`,
		mk(`[false]`) + `if (mk) ["t"]`,
		mk(`quote [gt 3]`) + `5 if (mk) ["big"] ["small"]`,
		mk(`[false]`) + `def f fn [[c:Any][Any][if c ["t"] ["f"]]] end f (mk)`,
		mk(`[1 2]`) + `if true (mk) ["f"]`,
		mk(`[1 2]`) + `def c false end if c ["t"] (mk)`,
		mk(`[1 2]`) + `def c true end if c (mk)`,
	} {
		gotC, compiled, errC, _, errI := runBothEngines(t, src)
		if !compiled || codeOf(errC) != "internal_error" || !strings.Contains(errC.Error(), "NUR292") || errI != nil || len(gotC) != 0 {
			t.Errorf("%s: a list the interpreter runs as code defers compiled; got %v %v (interpreter %v)", src, gotC, errC, errI)
		}
	}
	for _, c := range []struct{ src, want string }{
		{mk(`42`) + `if (mk) ["t"] ["f"]`, "[t]"},
		{mk(`0`) + `def c (mk) end if c ["t"] ["f"]`, "[f]"},
		{mk(`"s"`) + `if (mk) ["t"]`, "[t]"},
		{mk(`42`) + `if true (mk) ["f"]`, "[42]"},
		{mk(`42`) + `def f fn [[c:Boolean][Any][if c ["t"] (mk)]] end f false`, "[42]"},
		{mk(`[1 2]`) + `if false (mk) ["f"]`, "[f]"},
		{mk(`[1 2]`) + `def c false end if c (mk)`, "[]"},
		{`def mk fn [[][List][[1 2]]] end def c true end if c (mk) ["f"]`, "[1 2]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
