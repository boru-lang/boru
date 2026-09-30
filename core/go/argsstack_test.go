package core

import (
	"errors"
	"testing"
)

// TestArgsStackLazy pins PushLazy (NUR350): the entry reads as the list of
// its values, built once on the first Top (every read answers the one
// value), and Pop / Truncate drop lazy entries with the rest; a nil
// receiver errors as Push does.
func TestArgsStackLazy(t *testing.T) {
	var none *ArgsStack
	if err := none.PushLazy(nil); !errors.Is(err, errArgsStackNil) {
		t.Errorf("nil PushLazy err = %v, want errArgsStackNil", err)
	}
	as := NewArgsStack()
	_ = as.Push(NewInteger(1))
	_ = as.PushLazy([]Value{NewInteger(7), NewInteger(8)})
	first, ok, err := as.Top()
	if err != nil || !ok {
		t.Fatalf("Top = (%v, %v)", ok, err)
	}
	lst, lerr := AsList(first)
	if lerr != nil || lst.Len() != 2 {
		t.Fatalf("lazy entry = %v, want [7 8]", first)
	}
	again, _, _ := as.Top()
	if again.ID != first.ID || again.String() != first.String() {
		t.Errorf("a second read = %v (%s), want the first read's value %v (%s)", again, again.ID, first, first.ID)
	}
	// Truncate over a lazy entry, then Pop past a lazy one never read.
	_ = as.PushLazy([]Value{NewInteger(9)})
	as.Truncate(2)
	if as.Depth() != 2 {
		t.Fatalf("Depth after Truncate = %d, want 2", as.Depth())
	}
	_ = as.PushLazy(nil)
	if empty, _, _ := as.Top(); !IsConcrete(empty) || empty.String() != NewList(nil).String() {
		t.Errorf("a lazy entry of no values reads %v, want []", empty)
	}
	for k := 0; k < 2; k++ {
		if popped, err := as.Pop(); err != nil || !popped {
			t.Fatalf("Pop = (%v, %v)", popped, err)
		}
	}
	top, _, _ := as.Top()
	if n, _ := AsInteger(top); n != 1 {
		t.Errorf("the eager entry beneath reads %v, want 1", top)
	}
}

func TestArgsStackPushPopTop(t *testing.T) {
	as := NewArgsStack()

	// Empty state.
	if v, ok, err := as.Top(); err != nil || ok || v.Data != nil {
		t.Errorf("empty Top = (%v, %v, %v), want (zero, false, nil)", v, ok, err)
	}
	if popped, err := as.Pop(); err != nil || popped {
		t.Errorf("empty Pop = (%v, %v), want (false, nil)", popped, err)
	}

	// Push two entries.
	if err := as.Push(NewInteger(1)); err != nil {
		t.Fatalf("Push 1: %v", err)
	}
	if err := as.Push(NewInteger(2)); err != nil {
		t.Fatalf("Push 2: %v", err)
	}

	// Top sees the last push.
	v, ok, err := as.Top()
	if err != nil || !ok {
		t.Fatalf("Top after pushes = (%v, %v, %v)", v, ok, err)
	}
	n, _ := AsInteger(v)
	if n != 2 {
		t.Errorf("Top = %d, want 2", n)
	}

	// Pop returns true once per entry, then false.
	if popped, err := as.Pop(); err != nil || !popped {
		t.Errorf("first Pop = (%v, %v), want (true, nil)", popped, err)
	}
	if popped, err := as.Pop(); err != nil || !popped {
		t.Errorf("second Pop = (%v, %v), want (true, nil)", popped, err)
	}
	if popped, err := as.Pop(); err != nil || popped {
		t.Errorf("third Pop = (%v, %v), want (false, nil)", popped, err)
	}
}

// TestArgsStackNilReceiver verifies that every method surfaces a
// non-nil error when called on a nil *ArgsStack — the methods no
// longer silently no-op. Empty-stack flow control is preserved via
// the bool returns of Pop / Top.
func TestArgsStackNilReceiver(t *testing.T) {
	var as *ArgsStack

	if err := as.Push(NewInteger(1)); !errors.Is(err, errArgsStackNil) {
		t.Errorf("nil Push err = %v, want errArgsStackNil", err)
	}
	popped, err := as.Pop()
	if popped {
		t.Errorf("nil Pop popped = true, want false")
	}
	if !errors.Is(err, errArgsStackNil) {
		t.Errorf("nil Pop err = %v, want errArgsStackNil", err)
	}
	v, ok, err := as.Top()
	if ok || v.Data != nil {
		t.Errorf("nil Top = (%v, %v, ...), want (zero, false, ...)", v, ok)
	}
	if !errors.Is(err, errArgsStackNil) {
		t.Errorf("nil Top err = %v, want errArgsStackNil", err)
	}
}
