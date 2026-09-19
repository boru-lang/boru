package core

import "testing"

// TestBoruImplCompiledSlot pins the atomic compiled-ref slot's surface: a nil
// impl reads as no ref; a stored ref reads back; a nil store drops it (the
// compile-time undo, dropStampRef); Clone carries body, frame meta, dispatch
// handler and the ref, and a clone of an unstamped impl carries none;
// NewBoruImplCompiled builds a body impl already carrying its ref.
func TestBoruImplCompiledSlot(t *testing.T) {
	var none *BoruImpl
	if none.Compiled() != nil {
		t.Fatal("a nil impl reads as no ref")
	}
	a := &BoruImpl{Body: []Value{NewInteger(1)}, FnFrame: &FnFrameMeta{Name: "f"}, dispatch: func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) { return nil, nil }}
	if a.Compiled() != nil {
		t.Fatal("a fresh impl carries no ref")
	}
	a.SetCompiled("ref")
	if a.Compiled() != "ref" {
		t.Fatalf("SetCompiled/Compiled round trip: %v", a.Compiled())
	}
	c := a.Clone()
	if c == a || c.Compiled() != "ref" || c.FnFrame != a.FnFrame || c.dispatch == nil || len(c.Body) != 1 {
		t.Fatalf("Clone must copy body, frame meta, dispatch and the ref into a fresh impl: %+v", c)
	}
	a.SetCompiled(nil)
	if a.Compiled() != nil {
		t.Fatal("a nil store drops the ref")
	}
	if c.Compiled() != "ref" {
		t.Fatal("the clone's slot is its own")
	}
	if bare := a.Clone(); bare.Compiled() != nil {
		t.Fatal("a clone of an unstamped impl carries no ref")
	}
	pre := NewBoruImplCompiled([]Value{NewInteger(2)}, "pre")
	if pre.Compiled() != "pre" || len(pre.Body) != 1 {
		t.Fatalf("NewBoruImplCompiled: %+v", pre)
	}
}
