package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR232RootArmInstall pins NUR232's close: a ROOT arm's def lowers a
// registry install beside its branch-carried slot store (compiler
// rootArmInstall), so the binding the arm made is left for the next request
// on the same instance, as the interpreter's arm leaves it. The run itself
// always agreed — its reads use the slot — but `def x 5  if (g 9) [def x 9]
// [] end x` left the next request reading 5 for the interpreter's 9, and
// `if (g 9) [def y 9] [] end 0` left no y at all. Each row compares the
// run and the name's whole install stack in a following request. Since
// phase 2 (design/IMMUTABLE-DEF.1.md §2.1) an arm's def is the arm's own,
// so the rows carry the value out as the rule spells it: a `var` the arm
// assigns, or the branch value the def binds.
func TestNUR232RootArmInstall(t *testing.T) {
	const g = `def g fn [[n:Integer] [Boolean] [n gt 5]]  `
	for _, c := range []struct{ src, name string }{
		{`var y 0 if true [var y 9] [] end 0`, "y"},
		{g + `var y 0 if (g 9) [var y 9] [] end 0`, "y"},
		{g + `var y 0 if (g 1) [var y 9] [] end 0`, "y"},
		{`var y 0 def c true if c [var y 9] [] end 0`, "y"},
		{g + `var x 5 if (g 9) [var x 9] [] end x`, "x"},
		{g + `var x 5 if (g 1) [var x 9] [] end x`, "x"},
		{g + `var x (5 dup drop)  if (g 9) [var x 9] [] end x`, "x"},
		{g + `var x (5 add 0)  if (g 9) [var x 9] [] end x`, "x"},
		{g + `var x 0 if (g 9) [if (g 9) [var x 1] []] [] end x`, "x"},
		{g + `def x (if (g 9) [1] [2]) end x`, "x"},
		{g + `var x 5 if (g 9) [var x 9] [] end var x 7 x`, "x"},
		{g + `var x 5 if (g 9) [var x (g 3)] [] end x`, "x"},
		{g + `var x 5 if (g 9) [var x [1 2]] [] end x`, "x"},
		{`var x 0 for 3 [if (i gt 0) [var x i] []] end x`, "x"},
		{g + `var x 5 for 2 [if (g 9) [var x i] []] end x`, "x"},
		// A fn frame's arm def is torn down with the call on both lanes.
		{g + `def f fn [[] [Any] [var x 5 if (g 9) [var x 9] [] end x]] f`, "x"},
	} {
		interp, compiled, ok := bcsLanes(t, c.src)
		if !ok {
			continue
		}
		want := bcsInstalls(t, interp.RunInterp, c.name)
		got := bcsInstalls(t, compiled.RunCompiledStrict, c.name)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: next request's install stack of %s: compiled %v, interp %v", c.src, c.name, got, want)
		}
	}
}

// TestNUR237SplitUndefAfterAJoin pins the undef of a root split-bound name
// a branch join seated (main's NUR226 seat): the undef exposes the split's
// own binding, which a root read resolves live through the registry, so it
// compiles and reads what the interpreter reads — the split's value when
// the arm ran, undefined_word when it did not (the path whose undef popped
// the name's only level). A binding under the join that IS the slot's (an
// earlier join's) keeps the carried-undef decline.
func TestNUR237SplitUndefAfterAJoin(t *testing.T) {
	const g = `def g fn [[n:Integer] [Boolean] [n gt 5]]  `
	// Since phase 2 the arm's def is the arm's own: the undef after it pops
	// the split's binding, the only one standing, so the read raises on the
	// taken path too (the compiled lane declines the shadow).
	for _, c := range []struct{ src, want string }{
		{g + `def x (for 2 [5]) if (g 9) [def x 1] [] end undef x x`, "ERROR:undefined word: x"},
		{`def x (for 2 [5]) def c false if c [def x 1] [] end undef x 7`, "[5 7]"},
	} {
		ruleOrDecline(t, c.src, c.want)
	}
	// The miss raises at the read, with the interpreter's code and caret.
	// The did-you-mean pool differs by design: a compiled root offers its
	// loop variables (Program.LocalNames, NUR146), here `(for 2 [5])`'s i.
	for _, src := range []string{
		`def x (for 2 [5]) def c false if c [def x 1] [] end undef x x`,
		g + `def x (for 2 [5]) if (g 1) [def x 1] [] end undef x x`,
	} {
		ruleOrDecline(t, src, "ERROR:undefined word: x")
	}
	const twice = `def x (for 2 [5]) def c true if c [def x 1] [] end if c [def x 2] [] end undef x x`
	prog, reason, _, err := mustNew(t).CompileCheck(twice)
	if prog != nil || err != nil || !strings.Contains(reason, "check diagnostics") {
		t.Errorf("an undef exposing an earlier join's slot keeps its decline: prog=%v reason=%q err=%v", prog != nil, reason, err)
	}
}
