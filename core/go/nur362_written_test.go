package core

import "testing"

// TestBarrierBars pins NUR362's predicate: an overload bars a window of
// nFwd written operands when its barrier stops its forward collection short
// of them and it then reads the stack — never an all-forward overload, a
// fallback, or one whose barrier reaches every written operand.
func TestBarrierBars(t *testing.T) {
	key := Signature{Args: []*Type{TString, TMap}, BarrierPos: 1}
	all := Signature{Args: []*Type{TString, TMap}, BarrierPos: 2}
	open := Signature{Args: []*Type{TString, TMap}, BarrierPos: -1}
	one := Signature{Args: []*Type{TString}, BarrierPos: 1}
	fb := Signature{Args: []*Type{TString, TMap}, BarrierPos: 0, Fallback: true}
	for _, tc := range []struct {
		sigs []Signature
		nFwd int
		want bool
	}{
		{[]Signature{key}, 2, true},
		{[]Signature{all, key}, 2, true},
		{[]Signature{key}, 1, false},
		{[]Signature{all}, 2, false},
		{[]Signature{open}, 2, false},
		{[]Signature{one}, 2, false},
		{[]Signature{fb}, 1, false},
		{nil, 2, false},
	} {
		if got := BarrierBars(tc.sigs, tc.nFwd); got != tc.want {
			t.Errorf("BarrierBars(%v, %d) = %v, want %v", tc.sigs, tc.nFwd, got, tc.want)
		}
	}
}

// TestPublishWritten pins the written-count channel (CheckState.CurWritten):
// published only on a recording pass, for a split holding a written operand
// within the operands, answered only for the dispatch's own slice, and
// unpublished by the restore.
func TestPublishWritten(t *testing.T) {
	args := []Value{withID(NewInteger(1)), withID(NewInteger(2))}
	e := layoutEngine(t, append(append([]Value{}, args...), NewWord("w")), 2)
	for _, n := range []int{0, -1, 3} {
		e.PublishWritten(args, n)()
		if e.Registry.Check.CurWritten != nil {
			t.Errorf("a split of %d publishes nothing", n)
		}
	}
	restore := e.PublishWritten(args, 2)
	if n, ok := e.Registry.Check.WrittenFor(args); !ok || n != 2 {
		t.Errorf("published for its own operands: got %d, %v", n, ok)
	}
	if _, ok := e.Registry.Check.WrittenFor(args[:1]); ok {
		t.Error("another operand slice reads nothing")
	}
	if _, ok := e.Registry.Check.WrittenFor(nil); ok {
		t.Error("no operands read nothing")
	}
	restore()
	if e.Registry.Check.CurWritten != nil {
		t.Error("the restore unpublishes the split")
	}
	// Not a recording pass: nothing publishes.
	e.Registry.Check.Emit = nil
	e.PublishWritten(args, 2)()
	if _, ok := e.Registry.Check.WrittenFor(args); ok {
		t.Error("an inactive recorder publishes nothing")
	}
}

// TestForwardSplitExported pins ForwardSplit as forwardSplit's export: the
// last rearrangement's count when it laid out the word at the pointer, and
// none otherwise.
func TestForwardSplitExported(t *testing.T) {
	e := layoutEngine(t, []Value{withID(NewInteger(1)), NewWord("w")}, 1)
	if e.ForwardSplit() != 0 {
		t.Error("no rearrangement: no split")
	}
	e.fwdSplitAt, e.fwdSplitN, e.fwdSplitPos = e.Pointer, 1, e.Tape.At(e.Pointer).Pos()
	if e.ForwardSplit() != 1 {
		t.Errorf("the word's own rearrangement: got %d", e.ForwardSplit())
	}
}
