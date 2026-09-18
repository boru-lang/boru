package core

import (
	"testing"
)

// itemUnifier is a minimal Unifier for testing. Returns a fixed
// value so tests can assert "this Unifier fired".
type itemUnifier struct {
	prev   TypeBehavior
	fixed  Value
	failOn bool
}

func (i *itemUnifier) Match(v Value, t *Type) bool {
	if i.prev != nil {
		return i.prev.Match(v, t)
	}
	return DefaultBehavior.Match(v, t)
}
func (i *itemUnifier) Format(v Value) string {
	if i.prev != nil {
		return i.prev.Format(v)
	}
	return DefaultBehavior.Format(v)
}
func (i *itemUnifier) Equal(a, b Value) bool {
	if i.prev != nil {
		return i.prev.Equal(a, b)
	}
	return DefaultBehavior.Equal(a, b)
}
func (i *itemUnifier) Unify(a, b Value, _ *Registry) (Value, *UnifyError) {
	if i.failOn {
		return Value{}, &UnifyError{Reason: "item unifier rejected"}
	}
	return i.fixed, nil
}

// LCA walk finds a Unifier on a refine subtype when both operands
// share that type. Pins the basic "user installs Unifier; it fires"
// case at the kernel level — no lang machinery involved.
func TestDispatchUnifier_SameTypeBothSides(t *testing.T) {
	r, err0 := NewRegistry()
	if err0 != nil {
		t.Fatal(err0)
	}
	item := r.Types.MintType("Item", TInteger)
	want := NewInteger(99)
	item.ensureTMeta().Behavior = &itemUnifier{prev: item.Behavior(), fixed: want}

	x := NewInteger(3)
	x.Parent = item
	y := NewInteger(4)
	y.Parent = item

	v, err := UnifyExplain(x, y)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got, _ := AsInteger(v)
	if got != 99 {
		t.Fatalf("Unifier did not fire — got %d, want 99", got)
	}
}

// LCA walk finds a Unifier on a parent type when one operand is the
// parent itself. The "more specific" side rule: start from the
// subtype's chain.
func TestDispatchUnifier_MoreSpecificStarts(t *testing.T) {
	r, err0 := NewRegistry()
	if err0 != nil {
		t.Fatal(err0)
	}
	item := r.Types.MintType("Item", TInteger)
	want := NewInteger(42)
	item.ensureTMeta().Behavior = &itemUnifier{prev: item.Behavior(), fixed: want}

	x := NewInteger(3)
	x.Parent = item // Item-typed
	y := NewInteger(5)
	// y stays Integer-typed (no reparent)

	v, err := UnifyExplain(x, y)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got, _ := AsInteger(v)
	if got != 42 {
		t.Fatalf("Unifier on more-specific side did not fire — got %d, want 42", got)
	}
}

// Failure from a Unifier propagates up as a UnifyError.
func TestDispatchUnifier_FailurePropagates(t *testing.T) {
	r, err0 := NewRegistry()
	if err0 != nil {
		t.Fatal(err0)
	}
	item := r.Types.MintType("Item", TInteger)
	item.ensureTMeta().Behavior = &itemUnifier{prev: item.Behavior(), failOn: true}

	x := NewInteger(3)
	x.Parent = item
	y := NewInteger(4)
	y.Parent = item

	_, err := UnifyExplain(x, y)
	if err == nil {
		t.Fatal("expected Unifier failure to propagate")
	}
	if err.Reason != "item unifier rejected" {
		t.Errorf("got reason %q, want 'item unifier rejected'", err.Reason)
	}
}

// ErrNoUnifier opt-out causes the walk to continue past the
// placeholder Behavior.
func TestDispatchUnifier_OptOutContinuesWalk(t *testing.T) {
	r, err0 := NewRegistry()
	if err0 != nil {
		t.Fatal(err0)
	}
	item := r.Types.MintType("Item", TInteger)

	// Install a Unifier that always opts out.
	item.ensureTMeta().Behavior = &optOutUnifier{prev: item.Behavior()}

	x := NewInteger(3)
	x.Parent = item
	y := NewInteger(4)
	y.Parent = item

	// Should fall through to the kernel's structural rule, which
	// rejects 3 vs 4 as "same type, different literal".
	_, err := UnifyExplain(x, y)
	if err == nil {
		t.Fatal("expected structural rule to reject different literals")
	}
}

type optOutUnifier struct {
	prev TypeBehavior
}

func (o *optOutUnifier) Match(v Value, t *Type) bool {
	if o.prev != nil {
		return o.prev.Match(v, t)
	}
	return DefaultBehavior.Match(v, t)
}
func (o *optOutUnifier) Format(v Value) string {
	if o.prev != nil {
		return o.prev.Format(v)
	}
	return DefaultBehavior.Format(v)
}
func (o *optOutUnifier) Equal(a, b Value) bool {
	if o.prev != nil {
		return o.prev.Equal(a, b)
	}
	return DefaultBehavior.Equal(a, b)
}
func (o *optOutUnifier) Unify(a, b Value, _ *Registry) (Value, *UnifyError) {
	return Value{}, ErrNoUnifier
}
