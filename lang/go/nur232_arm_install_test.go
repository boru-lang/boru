package lang

import (
	"errors"
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
// run and the name's whole install stack in a following request.
func TestNUR232RootArmInstall(t *testing.T) {
	const g = `def g fn [[n:Integer] [Boolean] [n gt 5]]  `
	for _, c := range []struct{ src, name string }{
		{`if true [def y 9] [] end 0`, "y"},
		{g + `if (g 9) [def y 9] [] end 0`, "y"},
		{g + `if (g 1) [def y 9] [] end 0`, "y"},
		{`def c true if c [def y 9] [] end 0`, "y"},
		{g + `def x 5 if (g 9) [def x 9] [] end x`, "x"},
		{g + `def x 5 if (g 1) [def x 9] [] end x`, "x"},
		{g + `def x (5 dup drop)  if (g 9) [def x 9] [] end x`, "x"},
		{g + `def x (5 add 0)  if (g 9) [def x 9] [] end x`, "x"},
		{g + `if (g 9) [if (g 9) [def x 1] []] [] end x`, "x"},
		{g + `if (g 9) [def x 1] [def x 2] end x`, "x"},
		{g + `def x 5 if (g 9) [def x 9] [] end def x 7 x`, "x"},
		{g + `def x 5 if (g 9) [def x (g 3)] [] end x`, "x"},
		{g + `def x 5 if (g 9) [def x [1 2]] [] end x`, "x"},
		{`for 3 [if (i gt 0) [def x i] []] end x`, "x"},
		{g + `def x 5 for 2 [if (g 9) [def x i] []] end x`, "x"},
		// A fn frame's arm def is torn down with the call on both lanes.
		{g + `def f fn [[] [Any] [def x 5 if (g 9) [def x 9] [] end x]] f`, "x"},
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
	for _, c := range []struct{ src, want string }{
		{g + `def x (for 2 [5]) if (g 9) [def x 1] [] end undef x x`, "[5 5]"},
		{`def x (for 2 [5]) def c false if c [def x 1] [] end undef x 7`, "[5 7]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// The miss raises at the read, with the interpreter's code and caret.
	// The did-you-mean pool differs by design: a compiled root offers its
	// loop variables (Program.LocalNames, NUR146), here `(for 2 [5])`'s i.
	for _, src := range []string{
		`def x (for 2 [5]) def c false if c [def x 1] [] end undef x x`,
		g + `def x (for 2 [5]) if (g 1) [def x 1] [] end undef x x`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		var ec, ei *BoruError
		if !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || firstErrLine(errC) != firstErrLine(errI) ||
			!errors.As(errC, &ec) || !errors.As(errI, &ei) || ec.Row != ei.Row || ec.Col != ei.Col ||
			!strings.Contains(firstErrLine(errI), "undefined word: x") {
			t.Errorf("%s: compiled %v / %v, interpreter %v / %v", src, gotC, errC, gotI, errI)
		}
	}
	const twice = `def x (for 2 [5]) def c true if c [def x 1] [] end if c [def x 2] [] end undef x x`
	prog, reason, _, err := mustNew(t).CompileCheck(twice)
	if prog != nil || err != nil || !strings.Contains(reason, "undef of the loop-carried def `x`") {
		t.Errorf("an undef exposing an earlier join's slot keeps its decline: prog=%v reason=%q err=%v", prog != nil, reason, err)
	}
}
