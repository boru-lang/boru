package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in generics_unify.go — the ParenExpr child-type
// resolver arms, the genBinder gate, inference edge arms, and the
// default-vs-inferred-parameter mixing error. Per design/TEST-SEAMS.10.md.

import "testing"

func TestW8ResolveChildTypeExprRunError(t *testing.T) {
	r := w8reg(t)
	child := NewParenExpr([]Value{NewWord("__w8_undefined__")})
	v := NewTypedList(child)
	if _, err := ResolveChildTypeExpr(r, v); err == nil {
		t.Fatal("a child type expression that fails to run must error")
	}
}

func TestW8ResolveChildTypeExprTypedMap(t *testing.T) {
	r := w8reg(t)
	child := NewParenExpr([]Value{NewInteger(5)})
	v := NewTypedMap(child)
	got, err := ResolveChildTypeExpr(r, v)
	if err != nil {
		t.Fatalf("typed-map child expr should resolve: %v", err)
	}
	if !IsTypedMap(&got) {
		t.Fatalf("expected a typed map, got %v", got)
	}
}

func TestW8ResolveChildTypeExprTypedListElements(t *testing.T) {
	r := w8reg(t)
	child := NewParenExpr([]Value{NewInteger(5)})
	v := NewTypedListWithElements(child, []Value{NewInteger(1)})
	got, err := ResolveChildTypeExpr(r, v)
	if err != nil {
		t.Fatalf("typed-list-with-elements child expr should resolve: %v", err)
	}
	if !IsTypedList(&got) {
		t.Fatalf("expected a typed list, got %v", got)
	}
}

func TestW8GenBinderMergeNonParam(t *testing.T) {
	b := newGenBinder([]GenParam{{Name: "T"}})
	b.merge("Z", NewTypeLiteral(TInteger)) // Z is not a declared param → no-op
	if _, ok := b.bindings["Z"]; ok {
		t.Fatal("merging a non-parameter name must not bind")
	}
}

func TestW8InferFromChildPatternNotChildType(t *testing.T) {
	b := newGenBinder([]GenParam{{Name: "T"}})
	// A non-child-type pattern is ignored (AsChildType fails).
	inferFromChildPattern(b, NewInteger(1), NewInteger(2))
	if len(b.bindings) != 0 {
		t.Fatal("non-child-type pattern must contribute no bindings")
	}
}

func TestW8InferFromChildPatternTypedListConcreteChild(t *testing.T) {
	r := w8reg(t)
	node := MintTypeParam(r, GenParam{Name: "T"})
	b := newGenBinder([]GenParam{{Name: "T"}})
	pattern := NewTypedList(NewTypeLiteral(node))
	// arg is a typed list whose child is a concrete value (non-bare, but
	// with a Parent) — the aci.Child.Parent arm.
	arg := NewTypedList(NewInteger(5))
	inferFromChildPattern(b, pattern, arg)
	if _, ok := b.bindings["T"]; !ok {
		t.Fatal("typed-list arg with a concrete child must contribute a binding")
	}
}

func TestW8InferSchemaBindingsBodyNotMap(t *testing.T) {
	rf := NewOrderedMap()
	rf.Set("x", NewTypeLiteral(TInteger))
	info := &TypeSchemaInfo{Params: []GenParam{{Name: "T"}}, Body: NewRecordType(rf)}
	// body claims TMap and is concrete, but its payload is not a map, so
	// AsMap fails and inference yields no bindings.
	body := NewValueRaw(TMap, IntPayload{N: 1})
	got := InferSchemaBindings(info, body)
	if len(got) != 0 {
		t.Fatal("non-map body payload must contribute no bindings")
	}
}

func TestW8InferAndInstantiateDefaultBeforeInferred(t *testing.T) {
	r := w8reg(t)
	schemaNode := r.Types.MintType("W8Mix", TClass)
	bNode := MintTypeParam(r, GenParam{Name: "B"})
	rf := NewOrderedMap()
	rf.Set("b", NewTypeLiteral(bNode))
	info := &TypeSchemaInfo{
		Name: "W8Mix",
		Type: schemaNode,
		Params: []GenParam{
			{Name: "A", HasDefault: true, Default: NewTypeLiteral(TInteger)},
			{Name: "B"},
		},
		Body: NewRecordType(rf),
		Kind: SchemaRecord,
	}
	schemaVal := NewTypeSchema(schemaNode, info)
	bm := NewOrderedMap()
	bm.Set("b", NewInteger(5)) // supplies evidence for B, not A
	body := NewMap(bm)
	if _, err := InferAndInstantiateSchema(r, schemaVal, body); err == nil {
		t.Fatal("a defaulted parameter before an inferred one must error loudly")
	}
}
