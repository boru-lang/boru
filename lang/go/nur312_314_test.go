package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nur312Pre defines the factories the three records share: mkf returns a
// named fn that takes nothing (g), mkl one that takes an Integer (l), and mka
// an anonymous lambda over an Integer, each through an Any return, so the
// check pass holds only a dynamic carrier for the value.
const nur312Pre = `def g fn [[][Integer][7]] end def l fn [[x:Integer][Integer][x add 1]] end ` +
	`def mkf fn [[][Any][g/v]] end def mkl fn [[][Any][l/v]] end def mka fn [[][Any][[x:Integer] => [x add 1]]] end `

// requireLoudDefer asserts src compiles and that its run is the designed
// defer naming why (a compiler-defect bail, never an answer), where the
// interpreter answers wantI — a value, or an error containing "ERROR:<text>".
func requireLoudDefer(t *testing.T, src, why, wantI string) {
	t.Helper()
	if prog, reason, _, err := mustNew(t).CompileCheck(src); prog == nil || err != nil {
		t.Errorf("%q: want a compiled program, got decline %q / %v", src, reason, err)
		return
	}
	gotC, _, errC := mustNew(t).RunCompiled(src)
	if !isBailDefect(errC) || !strings.Contains(errC.Error(), why) || len(gotC) != 0 {
		t.Errorf("%q: want the loud defer %q, got %v / %v", src, why, gotC, errC)
	}
	gotI, errI := mustNew(t).RunInterp(src)
	if sub, isErr := strings.CutPrefix(wantI, "ERROR:"); isErr {
		if errI == nil || !strings.Contains(errI.Error(), sub) {
			t.Errorf("%q: interpreter %v / %v, want an error containing %q", src, gotI, errI, sub)
		}
	} else if errI != nil || fmt.Sprint(gotI) != wantI {
		t.Errorf("%q: interpreter %v / %v, want %s", src, gotI, errI, wantI)
	}
}

// TestNUR312PlacedFnInADriftWindow pins NUR312. A forward-drift window (`2
// (mkf) add 3`: add over a dynamic top, a literal after it) islands the
// window verbatim, and the recorder writes a value the interpreter parks — a
// user call's result — inside its own paren, so the one-survivor rule parks
// it again. A fn that takes nothing fired inside that paren first: [2 10]
// compiled, where the interpreter's add meets the parked fn and raises. The
// island now starts after the placed value, over the window beneath it as
// resolved inputs (placedWindowSplit).
func TestNUR312PlacedFnInADriftWindow(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`2 (mkf) add 3`, "ERROR:cannot call `add`"},
		{`2 mkf add 3`, "ERROR:cannot call `add`"},
		{`def h fn [[][Any][2 (mkf) add 3]] end h`, "ERROR:cannot call `add`"},
		// An argument-taking fn parked as before (NUR287's paren), and data.
		{`2 (mkl) add 3`, "ERROR:cannot call `add`"},
		{`5 mka add 1`, "ERROR:cannot call `add`"},
		{`def mk fn [[][Any][42]] end mk mk add 1`, "[42 43]"},
		{`2 (mkf) 3`, "[2 fn g 3]"},
	} {
		agreeOnBothLanes(t, nur312Pre+c.src, c.want)
	}
}

// TestNUR313BodyArmPlacesItsValue pins NUR313. `if` splices a body arm
// inside its own paren (spliceArg), and a one-survivor paren parks a fn
// value; the check pass handed the joined value back to the step loop, whose
// re-step the lowering took as a guarded landing or a lead apply: `def c true
// if c [(mkf)] [0]` answered 7 for `fn g`, and `if c [(mkl)] [0] 5` 6 for
// `fn l 5`. A branch whose body arms each net one value is placed now
// (branchPlaces); one with a value arm lands on that arm's path alone
// (splitLanding), and a split branch leading a residual apply declines.
func TestNUR313BodyArmPlacesItsValue(t *testing.T) {
	const one = `def one fn [[][Integer][1]] end `
	for _, c := range []struct{ src, want string }{
		{`if true [(mkf)] [0]`, "[fn g]"},
		{`if false [0] [(mkf)]`, "[fn g]"},
		{`def c true if c [(mkf)] [0]`, "[fn g]"},
		{`def c true if c [(mkf)] [(mkf)]`, "[fn g]"},
		{`def c true if c [(mkl)] [0] 5`, "[fn l(Integer) 5]"},
		{`if true [(mkl)] [0] 5`, "[fn l(Integer) 5]"},
		{`if true [(mkf)] [0] 5`, "[fn g 5]"},
		{`def c true [if c [(mkf)] [0]]`, "[[fn g]]"},
		// A bare read of a def bound to it dispatches, on both lanes.
		{`def c true def r (if c [(mkf)] [0]) r`, "[7]"},
		// The split branches: the value arm's path re-steps, the body's places.
		{`def c true if c [(mkf)] 0`, "[fn g]"},
		{`def c false if c [(mkf)] 0`, "[0]"},
		{`def c false if c 0 [(mkf)]`, "[fn g]"},
		{one + `def c true if c [(mkf)] one/v`, "[fn g]"},
		{one + `def c false if c [(mkf)] one/v`, "[1]"},
		{one + `def c true if c one/v [(mkf)]`, "[1]"},
		{`def c true if c (mkf) [(mkf)]`, "[7]"},
		{`def c false if c [(mkf)] (mkf)`, "[7]"},
		{`def c true if c [(mkf)] (mkf)`, "[fn g]"},
		// A value arm alone keeps NUR159's landing.
		{one + `if true one/v [2]`, "[1]"},
		{one + `def c true if c [0] one/v 5`, "[0 5]"},
	} {
		agreeOnBothLanes(t, nur312Pre+c.src, c.want)
	}
	requireLoudDecline(t, nur312Pre+one+`def c true if c [(mkf)] one/v 5`,
		"a branch whose body arm places a fn value and whose value arm is re-stepped leads the residual (NUR313)", "[fn g 5]")
}

// TestNUR314LoopResultsReStep pins NUR314. A loop's end splices its results
// back where the loop stood and steps them (stepMoveCont's done arm,
// handleLoopBreak), so a fn value among them dispatches; the compiled loop
// left it as data: `for 2 [(mkf)]` answered `[fn g fn g]` for [7 7]. An
// isolated loop — nothing beneath its results in the frame, its exit the
// end of its unit — steps them on the island now (loopExitReStep); anywhere
// else the re-step may reach a value the island does not hold, and it is a
// designed defer.
func TestNUR314LoopResultsReStep(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`for 1 [(mkf)]`, "[7]"},
		{`for 2 [(mkf)]`, "[7 7]"},
		{`for 2 [(mkl)]`, "ERROR:call to 'l' matched no signature"},
		{`def i 0 while [i lt 2] [def i (i add 1) (mkf)]`, "[7 7]"},
		{`for 3 [if [i eq 1] [break] [g/v]]`, "[7]"},
		{`def h fn [[][Any][for 1 [(mkf)]]] end h`, "[7]"},
		{`for 2 [do [raise oops 'x'] error [drop l/v]]`, "ERROR:call to 'l' matched no signature"},
		// Anonymous lambdas the re-step parks, and each other collected.
		{`for 2 [(mka)]`, "[fn (Integer) fn (Integer)]"},
		{`def mk0 fn [[][Any][[] => [9]]] end for 2 [(mk0)]`, "[fn fn]"},
		{`def mkh fn [[][Any][[f:Any] => [5]]] end for 2 [(mkh)]`, "[5]"},
		{`def mk fn [[n:Integer][Function][(fn [[x:Integer][Integer][x add n]])]] for 2 [(mk 1)]`, "[fn (Integer) fn (Integer)]"},
		{`for 3 [i]`, "[0 1 2]"},
	} {
		agreeOnBothLanes(t, nur312Pre+c.src, c.want)
	}
	const why = "a loop's result is a fn value the interpreter re-steps where the loop stood"
	requireLoudDefer(t, nur312Pre+`for 1 [(mkf)] 5`, why, "[7 5]")
	requireLoudDefer(t, nur312Pre+`for 2 [(mkl)] 5`, why, "ERROR:call to 'l' matched no signature")
	requireLoudDefer(t, nur312Pre+`for 1 [(mka)] 5`, why, "[6]")
}
