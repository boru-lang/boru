package lang

import (
	"strings"
	"testing"
)

// TestNUR292ComputedIfConditionOrArmThatIsAList pins NUR292's loud half. `if`
// runs a list condition inline — its words take the values beneath the if —
// and splices a list arm in parens, whatever produced the list; the compiled
// branch held a computed list as a value (`if (mk) ["t"] ["f"]` over
// `[false]` answered "t" for "f"). A value condition or value arm the pass
// holds abstractly that may be a list is guarded by the LOWERING
// (__codeguard, BranchRecord.Guard): the guard passes any other value —
// which answers as before, a fn landing where it did — and defers on a
// list. An arm the branch does not take is never checked, as the
// interpreter never splices it; an arm the pass types List keeps its
// `[__arm <arm>]` path. A member fn condition is coerced and a member fn arm
// lands at the merge, as NUR280 pins.
func TestNUR292ComputedIfConditionOrArmThatIsAList(t *testing.T) {
	mk := func(v string) string { return `def mk fn [[][Any][` + v + `]] end ` }
	for _, src := range []string{
		mk(`quote [gt 3]`) + `5 if (mk) ["big"] ["small"]`,
		mk(`quote [gt 3]`) + `def f fn [[c:Any][Any][5 if c ["t"] ["f"]]] end f (mk)`,
		mk(`[1 2]`) + `if true (mk) ["f"]`,
		mk(`[1 2]`) + `def c false end if c ["t"] (mk)`,
		mk(`[Integer]`) + `if (mk) ["t"] ["f"]`,
		mk(`[{a:1}]`) + `if (mk) ["t"] ["f"]`,
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
		{mk(`[1 2]`) + `if false (mk) ["f"]`, "[f]"},
		{`def mk fn [[][List][[1 2]]] end def c true end if c (mk) ["f"]`, "[1 2]"},
		{`def inc fn [[n:Integer][Integer][n add 1]] end def m (flex {h: inc/v}) end 5 if m.h ["t"] ["f"]`, "[5 t]"},
		{mk(`([y:Integer] => [y])`) + `5 if (mk) ["t"] ["f"]`, "[5 t]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR292PlainListConditionAndArmCompile pins the list forms the guards
// compile. A condition's inline run over a list of plain values only places
// them, and the branch reads the last (stepMoveIf), so __condguard answers
// the last element and raises the interpreter's "no value" over an empty
// list; an arm's paren splice places a one-value plain list's value, which
// __codeguard answers. boru:sift's column dedupe is the corpus's witness: its
// condition is ArrayUtil.member's Boolean mask, `[true]` or `[false]`, and
// the compiled branch read the mask's truthiness — always true — before the
// guard made it loud (module-sift.tsv L112, whose `size` hid the answer).
func TestNUR292PlainListConditionAndArmCompile(t *testing.T) {
	mk := func(v string) string { return `def mk fn [[][Any][` + v + `]] end ` }
	for _, c := range []struct{ src, want string }{
		{mk(`[false]`) + `if (mk) ["t"] ["f"]`, "[f]"},
		{mk(`[false]`) + `if (mk) ["t"]`, "[]"},
		{mk(`[1 2 true]`) + `if (mk) ["t"] ["f"]`, "[t]"},
		{mk(`[[]]`) + `if (mk) ["t"] ["f"]`, "[f]"},
		{mk(`[false]`) + `def f fn [[c:Any][Any][if c ["t"] ["f"]]] end f (mk)`, "[f]"},
		{`def f fn [[c:Any][Any][if c ["t"] ["f"]]] end f [true]`, "[t]"},
		{`def mk fn [[][List][[false]]] end if (mk) ["t"] ["f"]`, "[f]"},
		{`import "boru:array-util" def acc ["a"] end if (ArrayUtil.member ["a"] acc) ["dup"] ["new"]`, "[dup]"},
		{`import "boru:array-util" def acc ["a"] end if (ArrayUtil.member ["b"] acc) ["dup"] ["new"]`, "[new]"},
		{mk(`[7]`) + `if true (mk) ["f"]`, "[7]"},
		{mk(`[[7 8]]`) + `if true (mk) ["f"]`, "[[7 8]]"},
		{mk(`["s"]`) + `def c false end if c ["t"] (mk)`, "[s]"},
		{`import "boru:sift"  Sift.parse blocks/q {table:true} "a: 1\nb: 2\n\na: 3\nb: 4"`, "[[{a:'1' b:'2'} {a:'3' b:'4'}]]"},
		{mk(`[]`) + `if (mk) ["t"] ["f"]`, "ERROR:condition produced no value"},
		{`def f fn [[c:Any][Any][if c ["t"] ["f"]]] end f []`, "ERROR:condition produced no value"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
