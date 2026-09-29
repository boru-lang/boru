package core

import (
	"slices"
	"testing"
)

// TestNUR329GradualWrittenOperands pins the widening of a failed dispatch's
// window by its written run (withGradualWrittenOperands): a carrier or a
// dynamic written after the word joins the window, with every other written
// operand, so the failure goes to the runtime rematch that renders the run's
// value; a written run of concrete values, one holding a deferred
// expression, and no written run at all leave the window as it was.
func TestNUR329GradualWrittenOperands(t *testing.T) {
	w := NewWord("f")
	reach := NewReachFromKeys(NewWord("m"), []Value{NewAtom("b")})
	for _, c := range []struct {
		what   string
		tape   []Value
		window []int
		want   []int
	}{
		{"a carrier written after the word", []Value{NewInteger(7), w, NewCarrier(TInteger)}, []int{0}, []int{0, 2}},
		{"a dynamic, after a concrete one", []Value{NewInteger(7), w, NewInteger(1), NewDynamicCarrier(TAny)}, []int{0}, []int{0, 2, 3}},
		{"a window already past the word", []Value{w, NewCarrier(TInteger)}, []int{1}, []int{1}},
		{"concrete written values", []Value{NewInteger(7), w, NewInteger(1)}, []int{0}, []int{0}},
		{"a deferred reach in the run", []Value{NewInteger(7), w, NewCarrier(TInteger), reach}, []int{0}, []int{0}},
		{"nothing written", []Value{NewInteger(7), w}, []int{0}, []int{0}},
	} {
		ptr := slices.IndexFunc(c.tape, func(v Value) bool { return IsWord(v) })
		e := engWithTape(t, c.tape, ptr)
		if got := e.withGradualWrittenOperands(c.window, nil); !slices.Equal(got, c.want) {
			t.Errorf("%s: window %v, want %v", c.what, got, c.want)
		}
	}
}
