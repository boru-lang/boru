package core

import "testing"

// layoutRecorder is an ACTIVE recorder with nothing else to do: the layout
// is published only on a recording pass.
type layoutRecorder struct{ inactiveEmit }

func (layoutRecorder) Active() bool { return true }

// withID gives a test value the identity a tape token carries.
func withID(v Value) Value {
	v.ID = GenerateID("layout")
	return v
}

// layoutEngine is a recording engine over tape with its pointer at at.
func layoutEngine(t *testing.T, tape []Value, at int) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	r.Check.Emit = layoutRecorder{}
	e := NewTop(r)
	e.Tape = NewTape(tape, StackHeadroom)
	e.Pointer = at
	return e
}

// TestDispatchLayoutIsExactOrNothing pins NUR242's layout. `0 fold [add]
// s` is two written operands and one stack operand, published in
// signature order with their count; `s 0 fold [add]` is one written and
// two beneath. A group or statement boundary, or the tape's end, closes
// each side. Every other shape publishes nothing: a value the dispatch
// did not take beside the window (the interpreter's report would list it),
// an operand that is not the tape's own value (a resolved word, an
// evaluated list), a gap, a modified word, a tape-only report layer, or no
// recording pass.
func TestDispatchLayoutIsExactOrNothing(t *testing.T) {
	zero, body, s := withID(NewInteger(0)), withID(NewList([]Value{NewWord("add")})), withID(NewString("s"))
	fold := NewWord("fold")
	// `0 fold [add] s`, the operands in signature order: [add], s, 0.
	e := layoutEngine(t, []Value{zero, fold, body, s}, 1)
	args := []Value{body, s, zero}
	restore := e.PublishLayout(args, []int{2, 3, 0}, SrcPos{})
	if l := e.Registry.Check.LayoutFor(args); l == nil || l.NFwd != 2 {
		t.Fatalf("0 fold [add] s: two written, one beneath; got %+v", l)
	}
	// Only the slice it was published for reads it.
	if e.Registry.Check.LayoutFor(append([]Value(nil), args...)) != nil {
		t.Error("a copy of the operands is another record's")
	}
	if e.Registry.Check.LayoutFor(args[:2]) != nil || e.Registry.Check.LayoutFor(nil) != nil {
		t.Error("a shorter or empty operand list is another record's")
	}
	restore()
	if e.Registry.Check.CurLayout != nil || e.Registry.Check.LayoutFor(args) != nil {
		t.Error("the restore unpublishes the layout")
	}
	// `s 0 fold [add]` in a group: one written, two beneath, the open paren
	// below and the close paren after.
	e = layoutEngine(t, []Value{NewOpenParen(), s, zero, fold, body, NewCloseParen()}, 3)
	args = []Value{body, zero, s}
	restore = e.PublishLayout(args, []int{4, 2, 1}, SrcPos{})
	if l := e.Registry.Check.LayoutFor(args); l == nil || l.NFwd != 1 {
		t.Errorf("(s 0 fold [add]): one written, two beneath; got %+v", l)
	}
	restore()
	// Statement boundaries close a side too.
	e = layoutEngine(t, []Value{NewEnd(), zero, fold, body, NewEnd()}, 2)
	args = []Value{body, zero}
	restore = e.PublishLayout(args, []int{3, 1}, SrcPos{})
	if e.Registry.Check.LayoutFor(args) == nil {
		t.Error("end-bounded: exact")
	}
	restore()

	declines := []struct {
		name string
		tape []Value
		at   int
		args []Value
		pos  []int
		prep func(e *Engine)
	}{
		{"no operands", []Value{fold}, 0, nil, nil, nil},
		{"positions short", []Value{fold, body}, 0, []Value{body}, nil, nil},
		{"no recording pass", []Value{fold, body}, 0, []Value{body}, []int{1}, func(e *Engine) { e.Registry.Check.Emit = nil }},
		{"not a word", []Value{zero, body}, 0, []Value{body}, []int{1}, nil},
		{"a modified word", []Value{NewWordModified("fold", -1, true, false), body}, 0, []Value{body}, []int{1}, nil},
		{"a void group", []Value{fold, body}, 0, []Value{body}, []int{1}, func(e *Engine) { e.voidGroups = []string{"fold"} }},
		{"a gap", []Value{fold, zero, body}, 0, []Value{body}, []int{2}, nil},
		{"not the tape's value", []Value{fold, body}, 0, []Value{withID(NewList([]Value{NewInteger(3)}))}, []int{1}, nil},
		{"an operand with no identity", []Value{fold, body}, 0, []Value{{Parent: TList}}, []int{1}, nil},
		{"a value beneath", []Value{withID(NewInteger(7)), zero, fold, body}, 2, []Value{body, zero}, []int{3, 1}, nil},
		{"a value after", []Value{zero, fold, body, withID(NewInteger(5))}, 1, []Value{body, zero}, []int{2, 0}, nil},
	}
	for _, c := range declines {
		e := layoutEngine(t, c.tape, c.at)
		if c.prep != nil {
			c.prep(e)
		}
		restore := e.PublishLayout(c.args, c.pos, SrcPos{})
		if e.Registry.Check.CurLayout != nil {
			t.Errorf("%s: no layout, got %+v", c.name, e.Registry.Check.CurLayout)
		}
		restore()
	}
}

// TestSigOrderPositions pins the index twin of SigOrderArgs: the forward
// run first in source order, then the stack run top first; an nStack out
// of range reads the whole list as the stack run.
func TestSigOrderPositions(t *testing.T) {
	if got := SigOrderPositions([]int{4, 5, 7, 8}, 2); len(got) != 4 || got[0] != 7 || got[1] != 8 || got[2] != 5 || got[3] != 4 {
		t.Errorf("stack 4 5, forward 7 8: got %v", got)
	}
	if got := SigOrderPositions([]int{4, 5}, 9); len(got) != 2 || got[0] != 5 || got[1] != 4 {
		t.Errorf("an nStack past the list reads it all as the stack: got %v", got)
	}
}
