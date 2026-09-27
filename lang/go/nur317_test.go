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
