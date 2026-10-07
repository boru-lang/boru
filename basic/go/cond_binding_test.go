package basic

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
)

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

// runScrutineeForModel models a `case` scrutinee's run in a compile pass's
// SUSPENDED run (lang's case_scrutinee_model_test.go pins the compile end):
// the scrutinee is a block, so a var it ASSIGNS stands and a def it makes
// ends with it. With no recorder armed — plain check, or a kernel-only
// registry like this one — there is no such model, and the scrutinee must
// not run at all: the condition runner itself assigns (the control), the
// helper leaves the cell untouched.
func TestRunScrutineeForModelNeedsArmedRecorder(t *testing.T) {
	r := newTestRegistry(t)
	if err := Register(r); err != nil {
		t.Fatalf("Register: %v", err)
	}
	core.InstallVar(r, "x", NewInteger(1), nil)
	vals, err := parser.Parse("[var x 5 def y 7]")
	if err != nil || len(vals) != 1 {
		t.Fatalf("parse: %v %v", vals, err)
	}
	body := vals[0]
	runScrutineeForModel(r, body)
	if v, _ := r.Defs.Top("x"); core.Canon([]Value{v}) != "1" {
		t.Fatalf("with no recorder armed the scrutinee must not run: x = %s", core.Canon([]Value{v}))
	}
	RunCarrierCondBodyValues(r, body)
	if v, _ := r.Defs.Top("x"); core.Canon([]Value{v}) != "5" {
		t.Errorf("control: the condition runner assigns the scrutinee's var: x = %s", core.Canon([]Value{v}))
	}
	if r.Defs.Has("y") {
		t.Error("a def the scrutinee makes ends with it: the scrutinee is a block")
	}
}
