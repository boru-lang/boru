package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur343_349_test.go pins two loud divergences closed together: a runtime
// rematch over a branch the island may run again (NUR343), and a member
// read a later dispatch takes beside the value its re-step collects
// (NUR349). Positive rows answer on both lanes, compiled; negative rows pin
// where the lanes still agree by raising, and where the program declines.

// TestNUR343BranchRematchRunsItsIsland: `each` over a single-count branch
// whose result may be a List or an Integer — the pass cannot match `each`
// over the union, records a runtime rematch, and the run matches. The
// rematch's statement island runs the statement on the interpreter from its
// first token: the branch is re-runnable (its arms hold only reads), so
// nothing the compiled code did before the rematch runs twice.
func TestNUR343BranchRematchRunsItsIsland(t *testing.T) {
	const c = `def c true end `
	for _, tc := range []struct{ src, want string }{
		{c + `each (if c [[1]] [3]) [2]`, "[[1]]"},
		{c + `(if c [[1]] [3]) each [2]`, "[[2]]"},
		{c + `each (if c [[1]] ["s"]) [2]`, "[[1]]"},
		{c + `[each (if c [[1]] [3]) [2]]`, "[[[1]]]"},
		{c + `each (if c [[1]] [3]) [2] end 5`, "[[1] 5]"},
		{c + `each (if c [[1]] [3]) [2 3] end 5`, "[[1 1] 5]"},
		{c + `each (if c [[1]] [3]) [1 add 2]`, "[[1]]"},
		{c + `each (if c [[1]] [3]) [2] size`, "[1]"},
		{c + `size (each (if c [[1]] [3]) [2])`, "[1]"},
		{c + `def r (each (if c [[1]] [3]) [2]) end r`, "[[1]]"},
		{c + `def n 0 end each (if c [[1]] [3]) [2]`, "[[1]]"},
		{c + `print "a" end each (if c [[1]] [3]) [2]`, "[[1]]"},
		{c + `each (if (c) [[1]] [3]) [2]`, "[[1]]"},
		// A nested branch, and a member read, in an arm: reads only.
		{c + `each (if c [if c [[1]] [2]] [3]) [2]`, "[[1]]"},
		{c + `def d false end each (if c [if d [[1]] [[7]]] [3]) [2]`, "[[7]]"},
		{c + `def m {a:[1]} end each (if c [m.a] [3]) [2]`, "[[1]]"},
		// Two rematches: the first island runs the program after it too.
		{c + `each (if c [[1]] [3]) [2] end each (if c [[4]] [3]) [5]`, "[[1] [4]]"},
		// A multi-count merge leaves a region beneath the rematch; the
		// island runs the statement from its first token all the same.
		{c + `(if c [[1]] [3 4]) each [2]`, "[[2]]"},
		{c + `if c [[1]] [3 4] each [2]`, "[[2]]"},
		{c + `if c [[1]] [3 4] each [x/u]`, "ERROR:undefined word: x"},
		{`def c false end if c [[1]] [3 4] each [x/u]`, "ERROR:cannot call `each`"},
		// The body raises inside the island as it does interpreted.
		{c + `each (if c [[1]] [3]) [x add 1]`, "ERROR:undefined word: x"},
		// NEGATIVE: the run does not match either — both lanes raise the
		// no-signature report.
		{`def c false end each (if c [[1]] [3]) [2]`, "ERROR:cannot call `each`"},
		{`def c false end (if c [[1]] [3]) each [2]`, "ERROR:cannot call `each`"},
		{`def c false end each (if c [[1]] [3]) [2] end 5`, "ERROR:cannot call `each`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
	// The rematch carries its island.
	prog, reason, _, err := mustNew(t).CompileCheck(c + `each (if c [[1]] [3]) [2]`)
	if prog == nil || err != nil {
		t.Fatalf("must compile: %q %v", reason, err)
	}
	if len(prog.Dispatches) != 1 || prog.Dispatches[0].Restart == nil {
		t.Errorf("the rematch must carry its statement island: %+v", prog.Dispatches)
	}
}

// TestNUR343ArmEffectKeepsTheDefer: NEGATIVE — an arm holding an effect (a
// print) or a binding (a def) is no read the island may run again: the
// rematch plans no island and keeps its designed defer, loud, where the
// run matches.
func TestNUR343ArmEffectKeepsTheDefer(t *testing.T) {
	for _, src := range []string{
		`def c true end each (if c [[print "z" 1]] [3]) [2]`,
		`def c true end each (if c [[1]] [def q 3 q]) [2]`,
	} {
		prog, reason, _, err := mustNew(t).CompileCheck(src)
		if prog == nil || err != nil {
			t.Fatalf("%s: must compile: %q %v", src, reason, err)
		}
		if len(prog.Dispatches) != 1 || prog.Dispatches[0].Restart != nil {
			t.Errorf("%s: an arm the island cannot run again plans none: %+v", src, prog.Dispatches)
		}
	}
}

// TestNUR349MemberApplyBeforeInfixWord: a member read of an opaque Map
// followed by a value and a word that takes both — `1 m.f 7 add` is `1 (m.f
// 7) add` interpreted, 9: the read dispatches at its own token and its
// forward phase takes the 7 before `add` runs. The dispatch notes the
// read's landing as a collecting one; the guarded landing takes the
// statement's island when the run's value is a fn, and data runs on.
func TestNUR349MemberApplyBeforeInfixWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{nurFnMk + `1 m.f 7 add`, "[9]"},
		{nurFnMk + `3 1 m.f 7 add add`, "[12]"},
		{nurFnMk + `1 m.f 7 add end`, "[9]"},
		{nurFnMk + `1 m.f 7 sub`, "[-7]"},
		{nurFnMk + `10 m.f 7 sub`, "[2]"},
		{nurFnMk + `8 m.f 7 eq`, "[true]"},
		{nurFnMk + `1 m.f 7 eq`, "[false]"},
		{nurFnMk + `1 m.f 7 lt`, "[true]"},
		{nurFnMk + `"s" m.f 7 add`, "[s8]"},
		{nurFnMk + `def k 7 end 1 m.f k add`, "[9]"},
		{nurFnMk + `1 m.f (7) add`, "[9]"},
		{nurFnMk + `print "a" 1 m.f 7 add`, "[9]"},
		{nurFnMk + `1 m.f 7 add end 2 m.f 3 add`, "[9 6]"},
		{nurFnMk + `[1 m.f 7 add]`, "[[9]]"},
		{nurFnMk + `[8 m.f 7 eq]`, "[[true]]"},
		{nurFnMk + `(8 m.f 7 eq)`, "[true]"},
		{nurFnMk + `(1 m.f 7 add) typeof`, "[Integer]"},
		{nurFnMk + `def r (1 m.f 7 add) end r`, "[9]"},
		{nurFnMk + `def h fn [[a:Integer b:Integer] [Integer] [a sub b]] end 1 m.f 7 h`, "[7]"},
		{`def mk fn [[] [Map] [{a: {f: ([x:Integer] => [x add 1])}}]] end def m (mk) end 1 m.a.f 7 add`, "[9]"},
		// In a fn body: the unit's statement island.
		{`def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def g fn [[m:Map] [Any] [1 m.f 7 add]] end g (mk)`, "[9]"},
		{`def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def g fn [[m:Map] [Any] [print "x" 8 m.f 7 eq]] end g (mk)`, "[true]"},
		// A named fn that takes no argument is called where it lands.
		{`def g fn [[] [Integer] [5]] end def mk fn [[] [Map] [{f: g/v}]] end def m (mk) end 1 m.f 7 add`, "[1 12]"},
		// Data members run on as the model has them.
		{nurDataMk + `1 m.f 7 add`, "[1 12]"},
		{nurDataMk + `1 m.f 7 add add`, "[13]"},
		// A `/v` read is data where it lands: no landing, no island.
		{nurFnMk + `m.f/v 7`, "[fn (Integer) 7]"},
		{nurFnMk + `1 m.f/v 7`, "[1 fn (Integer) 7]"},
		// NEGATIVE: the re-step leaves `add` / `eq` one value short on both
		// lanes, or the fn matches nothing it could collect.
		{nurFnMk + `m.f 7 add`, "ERROR:cannot call `add`"},
		{nurFnMk + `m.f 7 eq`, "ERROR:cannot call `eq`"},
		{nurFnMk + `[m.f 7 eq]`, "ERROR:cannot call `eq`"},
		{`def mk fn [[] [Map] [{f: ([x:String] => [x])}]] end def m (mk) end 1 m.f 7 add`, "ERROR:cannot call `add`"},
	} {
		agreeOnBothLanes(t, tc.src, tc.want)
	}
}

// TestNUR349UnguardedLandingDeclines: NEGATIVE — a taken member read inside
// a branch arm or a loop body, or in a fn unit whose residual is an apply
// chain, has no guarded landing (those keep today's arms), so the program
// declines, where it used to answer the
// model's window silently: `if c [8 m.f 7 eq] [0]` was `[8 false]` for the
// interpreter's `[true]`. The interpreter's answer stands.
func TestNUR349UnguardedLandingDeclines(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{nurFnMk + `def c true end if c [8 m.f 7 eq] [0]`, "[true]"},
		{nurFnMk + `def c true end if c [1 m.f 7 add] [0]`, "[9]"},
		{nurFnMk + `for 1 [8 m.f 7 eq]`, "[true]"},
		{`def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def g fn [[m:Map c:Boolean] [Any] [if c [8 m.f 7 eq] [0]]] end g (mk) true`, "[true]"},
		// A unit whose residual is an apply chain guards no landing.
		{`def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def g fn [[m:Map f:Function] [Any] [1 m.f 7 add drop 5 f/v apply]] end g (mk) ([x:Integer] => [x])`, "[5]"},
	} {
		prog, reason, _, _ := mustNew(t).CompileCheck(tc.src)
		if prog != nil || !strings.Contains(reason, "NUR349") {
			t.Errorf("%s: must decline with the NUR349 reason, got %v %q", tc.src, prog != nil, reason)
		}
		got, err := mustNew(t).RunInterp(tc.src)
		if err != nil || fmt.Sprint(got) != tc.want {
			t.Errorf("%s: interpreted %v %v, want %s", tc.src, got, err, tc.want)
		}
	}
}
