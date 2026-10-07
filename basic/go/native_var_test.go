package basic

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The var word's typed-name form (VarTypedHandler), def's sibling, pinned
// at the handler: the annotation must name a
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
	// The three forms, def's siblings in def's order — the typed-name map
	// form, the string name, the quoted name — each check-mode and
	// all-forward.
	sigs := varWordSignatures
	if len(sigs) != 3 || !sigs[0].Args[0].Equal(TMap) || !sigs[0].NoEvalMapArgs[0] ||
		!sigs[1].Args[0].Equal(TString) || !sigs[2].Args[0].Equal(TAtom) || !sigs[2].QuoteArgs[0] ||
		sigs[0].BarrierPos != -1 || sigs[1].BarrierPos != -1 || sigs[2].BarrierPos != -1 ||
		!sigs[0].RunInCheckMode() || !sigs[1].RunInCheckMode() || !sigs[2].RunInCheckMode() {
		t.Errorf("the var word's forms: %+v", sigs)
	}
	// A unify that answers with the type's own node keeps the value.
	if err := call(VarTypedHandler, typed("f", NewTypeLiteral(TNumber)), NewInteger(3)); err != nil {
		t.Fatalf("an Integer into a Number var: %v", err)
	}
	if v, _ := r.Defs.Top("f"); core.IsBareTypeNode(v) || v.String() != "3" {
		t.Errorf("the value, not the type node, is held: %v", v)
	}
}

// TestVarWordRefusesCapturedVar pins the frame rule on a CAPTURED var
// (core.InstallCapturedBinding: a frame binding marked Var): the closure
// reads the var's value, and `var NAME v` over it is a var_error — a var of
// another frame, the module's and an enclosing fn's alike. A captured DEF
// of the same name is no var: the statement declares the closure's own.
func TestVarWordRefusesCapturedVar(t *testing.T) {
	r := newTestRegistry(t)
	core.InstallCapturedBinding(r, core.CapturedBinding{Name: "s", Value: NewInteger(5), Var: true})
	_, err := VarWordHandler([]Value{NewAtom("s"), NewInteger(0)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "cannot assign a var of an enclosing frame") {
		t.Fatalf("assigning a captured var: %v, want the frame rule's var_error", err)
	}
	if v, _ := r.Defs.Top("s"); v.String() != "5" || r.Defs.Depth("s") != 1 {
		t.Errorf("the captured value must stand: %v depth %d", v, r.Defs.Depth("s"))
	}
	core.InstallCapturedBinding(r, core.CapturedBinding{Name: "d", Value: NewInteger(5)})
	if _, err := VarWordHandler([]Value{NewAtom("d"), NewInteger(0)}, nil, nil, r); err != nil {
		t.Fatalf("a var over a captured def declares: %v", err)
	}
	if e, ok := r.Defs.TopEntry("d"); !ok || !e.Var || r.Defs.Depth("d") != 2 {
		t.Errorf("the closure's own var shadows the captured def: %+v depth %d", e, r.Defs.Depth("d"))
	}
}
