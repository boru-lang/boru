package lang

import "testing"

// TestNUR317EnclosingReStepUndoesPlacement pins NUR317's closed forms. An
// `if` body arm's paren parks the one fn value it leaves (NUR313), and an
// enclosing re-step undoes that: a `do`, whose results the step loop re-steps
// at the call, and a paren whose two or more survivors its rewind re-steps.
// `def c true do [if c [g/v] [0]]` answered `fn g` compiled for the
// interpreter's 7, `(if c [g/v] [0] 5)` `fn g 5` for `7 5`: the pass's join
// of a data arm and a fn arm is a union carrier the landing's callable test
// did not read, and the placement rule counted the `do`'s re-step as the
// branch's own. Where nothing re-steps it the value stays placed.
func TestNUR317EnclosingReStepUndoesPlacement(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def c true do [if c [g/v] [0]]`, "[7]"},
		{`do [if true [g/v] [0]]`, "[7]"},
		{`def c true do [(if c [g/v] [0])]`, "[7]"},
		{`def c true do [if c [g/v] [0]] 5`, "[7 5]"},
		{`def c true do [if c [l/v] [0]] 5`, "[6]"},
		{`def c true do [if c [l/v] [0]]`, "[fn l(Integer)]"},
		{`def c false do [if c [g/v] [0]]`, "[0]"},
		{`def c true def r (if c [g/v] [0]) r`, "[7]"},
		{`def c true (if c [g/v] [0] 5)`, "[7 5]"},
		{`def c false (if c [g/v] [0] 5)`, "[0 5]"},
		{`def c true (if c [l/v] [0] 5)`, "[6]"},
		{`def c true (if c [g/v] [0] 5 6)`, "[7 5 6]"},
		{`def c true ((if c [g/v] [0]) 5)`, "[7 5]"},
		// A dyn body's lead it parked is the caller's to re-step: the mark
		// window's island re-steps the body's actual results, as it does a
		// lead the body settled itself (NUR271's `do [m.f 5]`).
		{`def c true do [if c [(mkf)] [0] 5]`, "[7 5]"},
		{`def c true do [if c [(mkl)] [0] 5]`, "[6]"},
		// Placed where nothing re-steps it, the value stays data.
		{`def c true if c [g/v] [0]`, "[fn g]"},
		{`def c true if c [g/v] [0] 5`, "[fn g 5]"},
		{`def c true (if c [(mkf)] [0])`, "[fn g]"},
		{`def c true [if c [g/v] [0]]`, "[[fn g]]"},
	} {
		agreeOnBothLanes(t, nur312Pre+c.src, c.want)
	}
	requireLoudDecline(t, nur312Pre+`def c true (if c [(mkf)] [0] 5)`, "fn-value application bounded by a paren", "[7 5]")
}

// TestNUR317DoReStepsItsResults pins NUR317's second half. The check pass's
// own run of a `do` body steps an `if` whose condition it decides into the
// taken arm and dispatches the arm's placed fn inside the body, so the `do`'s
// modelled outputs are the re-step's (`[Integer 5]`), and nothing the program
// recorded after the call applied the fn the compiled body handed back: `do
// [if c [g/v] [0] 5]` was `fn g 5` for the interpreter's `7 5`, `do [if c
// [l/v] [0] 5]` `fn l 5` for 6. The call carries SigRef.ReStep, and the VM
// re-steps the results through the island where that is exact — the fn takes
// nothing, or nothing lies beneath the results and the unit ends after them.
// A value beneath (the prefix a region seats below the run) no longer hides
// the dyn body's re-step either.
func TestNUR317DoReStepsItsResults(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def c true do [if c [g/v] [0] 5]`, "[7 5]"},
		{`def c true do [if c [l/v] [0] 5]`, "[6]"},
		{`def c true do [if c [g/v] [g/v]]`, "[7]"},
		{`def c false do [if c [g/v] [0] 5]`, "[0 5]"},
		{`def c true do [if c [l/v] [0] 5 6]`, "[6 6]"},
		{`def c true (do [if c [g/v] [0] 5])`, "[7 5]"},
		// A fn that takes nothing fires over nothing, whatever lies around it.
		{`def c true 1 do [if c [g/v] [0] 5]`, "[1 7 5]"},
		{`def c true do [if c [g/v] [0] 5] add 1`, "[7 6]"},
		{`def c true do [if c [g/v] [0] 5] drop`, "[7]"},
		// The dyn body's run beneath a prefix (planRegionPrefix).
		{`def c true 1 do [if c [(mkf)] [0] 5]`, "[1 7 5]"},
		{`def c true 1 do [if c [(mkl)] [0]]`, "[2]"},
		// In a fn body, a decided condition's re-step; an undecided one's
		// lone union the closure's own landing re-steps.
		{`def c true def h fn [[][Integer Integer][do [if c [g/v] [0] 5]]] end h`, "[7 5]"},
		{`def h fn [[c:Boolean][Any][do [if c [g/v] [0]]]] end h true`, "[7]"},
	} {
		agreeOnBothLanes(t, nur312Pre+c.src, c.want)
	}
	// A fn that takes an argument, where a value lies beneath the results
	// or the program goes on after them: its collection could reach what the
	// island does not hold.
	for _, c := range []struct{ src, want string }{
		{`def c true do [if c [l/v] [0] 5] add 1`, "[7]"},
		{`def c true 1 do [if c [l/v] [0] 5]`, "[1 6]"},
	} {
		requireLoudDefer(t, nur312Pre+c.src, "re-steps where the `do` stood", c.want)
	}
}
