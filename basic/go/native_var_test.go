package basic

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The var word's typed-name form (VarTypedHandler), reachable through its
// deferred signature once the `var [[…]]` construct is gone (native_var.go's
// registration note) and pinned here meanwhile: the annotation must name a
// type; the declaration and every assignment check the value against it; a
// typed re-declaration of a live var is refused.
func TestVarTypedHandler(t *testing.T) {
	r := newTestRegistry(t)
	typed := func(name string, ann Value) Value {
		m := core.NewOrderedMap()
		m.Set(name, ann)
		return NewMap(m)
	}
	call := func(h func([]Value, map[string]Value, []Value, *Registry) ([]Value, error), args ...Value) error {
		_, err := h(args, nil, nil, r)
		return err
	}
	if err := call(VarTypedHandler, typed("n", NewTypeLiteral(TInteger)), NewInteger(1)); err != nil {
		t.Fatalf("a typed declaration: %v", err)
	}
	if e, ok := r.Defs.TopEntry("n"); !ok || !e.Var || e.VarType == nil || !e.VarType.Equal(TInteger) {
		t.Fatalf("the typed var: %+v", e)
	}
	if err := call(VarWordHandler, NewAtom("n"), NewInteger(2)); err != nil {
		t.Fatalf("a conforming assignment: %v", err)
	}
	if v, _ := r.Defs.Top("n"); v.String() != "2" || r.Defs.Depth("n") != 1 {
		t.Errorf("assigned in place: %v depth %d", v, r.Defs.Depth("n"))
	}
	err := call(VarWordHandler, NewAtom("n"), NewString("x"))
	if err == nil || !strings.Contains(err.Error(), "does not unify with declared type Integer") {
		t.Errorf("a mismatching assignment is a type_error: %v", err)
	}
	err = call(VarTypedHandler, typed("n", NewTypeLiteral(TString)), NewString("x"))
	if err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Errorf("a typed re-declaration is refused: %v", err)
	}
	err = call(VarTypedHandler, typed("s", NewTypeLiteral(TString)), NewInteger(1))
	if err == nil || !strings.Contains(err.Error(), "does not unify with declared type String") {
		t.Errorf("a mismatching declaration is a type_error: %v", err)
	}
	// The annotation must be exactly one type name.
	two := core.NewOrderedMap()
	two.Set("a", NewTypeLiteral(TInteger))
	two.Set("b", NewTypeLiteral(TInteger))
	if err := call(VarTypedHandler, NewMap(two), NewInteger(1)); err == nil || !strings.Contains(err.Error(), "exactly one key") {
		t.Errorf("two keys: %v", err)
	}
	if err := call(VarTypedHandler, typed("q", NewInteger(5)), NewInteger(1)); err == nil || !strings.Contains(err.Error(), "must be a type value") {
		t.Errorf("a non-type annotation: %v", err)
	}
	shape := core.NewOrderedMap()
	shape.Set("a", NewTypeLiteral(TInteger))
	if err := call(VarTypedHandler, typed("rec", NewMap(shape)), NewMap(core.NewOrderedMap())); err == nil || !strings.Contains(err.Error(), "must be a type value") {
		t.Errorf("a structural annotation is no type value here: %v", err)
	}
	// A named structural type is a type body but not a bare node: refused
	// as a var's type for now.
	if err := core.InstallType(r, "Rec", NewMap(shape)); err != nil {
		t.Fatal(err)
	}
	recNode, _ := r.Defs.Top("Rec")
	if err := call(VarTypedHandler, typed("rec", recNode), NewMap(core.NewOrderedMap())); err != nil && !strings.Contains(err.Error(), "a var's type is a type name") && !strings.Contains(err.Error(), "does not unify") {
		t.Errorf("a named structural type: %v", err)
	}
	// A non-concrete value conforms by its static type and keeps its identity.
	c := core.NewCarrier(TInteger)
	if err := call(VarWordHandler, NewAtom("n"), c); err != nil {
		t.Fatalf("a carrier of the right type: %v", err)
	}
	if v, _ := r.Defs.Top("n"); v.ID != c.ID {
		t.Error("the carrier is stored as is")
	}
	if err := call(VarWordHandler, NewAtom("n"), core.NewCarrier(TString)); err == nil {
		t.Error("a carrier of another type is refused")
	}
	// The deferred forms: the typed-name map form and the string-name form,
	// def's siblings, each check-mode and all-forward.
	deferred := VarWordDeferredSignatures()
	if len(deferred) != 2 || !deferred[0].Args[0].Equal(TMap) || !deferred[0].NoEvalMapArgs[0] ||
		!deferred[1].Args[0].Equal(TString) || deferred[0].BarrierPos != -1 || deferred[1].BarrierPos != -1 ||
		!deferred[0].RunInCheckMode() || !deferred[1].RunInCheckMode() {
		t.Errorf("the deferred forms: %+v", deferred)
	}
	// A unify that answers with the type's own node keeps the value.
	if err := call(VarTypedHandler, typed("f", NewTypeLiteral(TNumber)), NewInteger(3)); err != nil {
		t.Fatalf("an Integer into a Number var: %v", err)
	}
	if v, _ := r.Defs.Top("f"); core.IsBareTypeNode(v) || v.String() != "3" {
		t.Errorf("the value, not the type node, is held: %v", v)
	}
}
