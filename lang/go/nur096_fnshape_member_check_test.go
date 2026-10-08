package lang

import (
	"fmt"
	"testing"
)

// TestPlainCheckModelsFnShapeMemberApply pins NUR096's close. A class
// field typed by a fn SHAPE holds a function at run time — the shape's
// membership admits nothing else — and a dot read of it re-steps that fn
// over its argument window on both lanes (`c.op 10` is [10 10] for a
// two-result shape). The plain check held the member carrier and its
// argument instead ([T Integer]); a single-result shape hid it, because
// only the top slot was ever compared. The check now applies the shape's
// declared signature: its arguments are consumed and its declared returns
// take their place, for a named shape and an anonymous one alike. The
// negatives keep the carrier where the run keeps the fn: a paren PLACES
// its value, `/v` says data, an argument that does not fit the shape
// leaves the fn uncalled, and a read with no argument stays a value.
func TestPlainCheckModelsFnShapeMemberApply(t *testing.T) {
	const two = `def C class {op:T} def c (make C {op:(fn [[x:Integer] [Integer Integer] [x x]])}) `
	const named = `def T fnsig [[Integer] [Integer Integer]] ` + two
	const anon = `def C class {op:(fnsig [[Integer] [Integer Integer]])} def c (make C {op:(fn [[x:Integer] [Integer Integer] [x x]])}) `
	for _, c := range []struct{ src, check, run string }{
		{named + `c.op 10`, "[Integer Integer]", "[10 10]"},
		{anon + `c.op 10`, "[Integer Integer]", "[10 10]"},
		{`def T fnsig Integer Integer def C class {op:T} def c (make C {op:(fn [[x:Integer] [Integer] [x add 1]])}) c.op 5`, "[Integer]", "[6]"},
		{`def T fnsig [[Integer Integer] [Integer]] def C class {op:T} def c (make C {op:(fn [[a:Integer b:Integer] [Integer] [a sub b]])}) c.op 10 3`, "[Integer]", "[7]"},
		{`def T fnsig [[Integer] []] def C class {op:T} def c (make C {op:(fn [[x:Integer] [] []])}) 7 c.op 10`, "[Integer]", "[7]"},
		// Negatives: the run keeps the fn a value, and so does the check.
		{`def T fnsig [[Integer] [Integer Integer]] def C class {op:T} ((make C {op:(fn [[x:Integer] [Integer Integer] [x x]])}) dot op) 10`, "[T Integer]", "[fn (Integer) 10]"},
		{named + `c.op "s"`, "[T ProperString]", "[fn (Integer) s]"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		cr, err := a.Check(c.src)
		if err != nil {
			t.Fatalf("%q: check: %v", c.src, err)
		}
		if got := fmt.Sprint(cr.Stack); got != c.check {
			t.Errorf("%q: checked %s, want %s", c.src, got, c.check)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.run {
			t.Errorf("%q: interpreted %v / %v, want %s", c.src, gotI, errI, c.run)
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != c.run {
			t.Errorf("%q: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.run)
		}
	}
}
