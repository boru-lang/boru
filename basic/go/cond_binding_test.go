package basic

import "testing"

// cond_binding_test.go covers the binding-shape probe the kept `if`
// condition declines by (NUR212's follow-up, analyseCondFragment): a
// condition that binds a name compiles as a kept fragment unless it nets
// more than its one decision value or holds a value-less `do` — then the
// probe decides whether it bound anything the program can observe. The
// recording pass itself is not linked in this module; lang's
// if_clause_list_test.go pins the lowering end to end.

func TestBindingShapeChanged(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("x", NewInteger(1))
	r.Defs.Push("__gen:Box[Integer]", NewInteger(0))

	before := bindingShape(r)
	if bindingShapeChanged(r, before) {
		t.Fatal("an untouched table has not changed")
	}

	// A push of an existing name (a redefinition), a new name, a pop that
	// empties a name, and a same-depth rebind are all observable.
	for name, mutate := range map[string]func(){
		"a redefinition": func() { r.Defs.Push("x", NewInteger(5)) },
		"a new name":     func() { r.Defs.Push("y", NewInteger(5)) },
		"an unbinding":   func() { r.Defs.Pop("x") },
		"a same-depth rebind": func() {
			r.Defs.Pop("x")
			r.Defs.Push("x", NewInteger(9))
		},
	} {
		r.Defs.Truncate("y", 0)
		r.Defs.Truncate("x", 0)
		r.Defs.Push("x", NewInteger(1))
		before = bindingShape(r)
		mutate()
		if !bindingShapeChanged(r, before) {
			t.Errorf("%s: the table changed", name)
		}
	}

	// A generic instantiation's hidden memo is not a name the program
	// observes — neither its install nor its removal counts.
	r.Defs.Truncate("y", 0)
	before = bindingShape(r)
	r.Defs.Push("__gen:Box[String]", NewInteger(0))
	r.Defs.Pop("__gen:Box[Integer]")
	if bindingShapeChanged(r, before) {
		t.Error("a generic memo's install or removal is not an observable binding")
	}
}
