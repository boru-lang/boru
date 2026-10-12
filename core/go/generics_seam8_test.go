package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached branches in generics.go (genParamUnifier Match/Format) and
// generics_instantiate.go (typeArgNode surface arm, containsTypeParam
// object arm, and the substitution / instantiation error-propagation
// arms). Driven by crafting placeholder nodes (MintTypeParam) and hand-
// built schema/type values. Per design/TEST-SEAMS.10.md.

import "testing"

func w8ParamNode(r *Registry) *Type { return MintTypeParam(r, GenParam{Name: "T"}) }
func w8ParamLit(r *Registry) Value  { return NewTypeLiteral(w8ParamNode(r)) }

// --- generics.go: genParamUnifier -----------------------------------------

func TestW8GenParamUnifierMatchConforms(t *testing.T) {
	r := w8reg(t)
	node := w8ParamNode(r)
	g, ok := node.Behavior().(*genParamUnifier)
	if !ok {
		t.Fatal("minted placeholder must carry a genParamUnifier")
	}
	// A value whose Parent conforms to the placeholder node matches.
	v := NewValueRaw(node, IntPayload{N: 1})
	if !g.Match(v, node) {
		t.Fatal("value tagged with the placeholder node must match it")
	}
}

func TestW8GenParamUnifierFormat(t *testing.T) {
	r := w8reg(t)
	node := w8ParamNode(r)
	g := node.Behavior().(*genParamUnifier)
	// A bare literal renders as the parameter name.
	if s := g.Format(NewTypeLiteral(node)); s != "T" {
		t.Fatalf("bare placeholder literal should render as \"T\", got %q", s)
	}
	// A concrete value falls through to the wrapped Behavior's Format.
	if s := g.Format(NewInteger(5)); s == "" {
		t.Fatal("concrete value should render via the base behavior")
	}
}

// --- generics_instantiate.go: typeArgNode / containsTypeParam --------------

func TestW8TypeArgNodeSurface(t *testing.T) {
	r := w8reg(t)
	node := r.Types.MintType("W8Surf", TAny)
	sv := NewSurfaceType(node, &SurfaceInfo{Type: node})
	if got := typeArgNode(sv); got == nil || IDOf(got) != IDOf(node) {
		t.Fatalf("typeArgNode of a surface must return its minted node, got %v", got)
	}
}

func TestW8ContainsTypeParamObjectField(t *testing.T) {
	r := w8reg(t)
	node := r.Types.MintType("W8Obj", TClass)
	of := NewOrderedMap()
	of.Set("f", w8ParamLit(r))
	ot := NewClassType(node, ClassTypeInfo{Fields: of})
	if !containsTypeParam(ot) {
		t.Fatal("object type with a placeholder field must contain a type param")
	}
}

// --- generics_instantiate.go: SubstituteTypeParams error arms --------------

func TestW8SubstituteGenInstRefHeadError(t *testing.T) {
	r := w8reg(t)
	ref := NewGenInstRef(GenInstRef{Head: w8ParamLit(r), Args: nil})
	if _, err := SubstituteTypeParams(r, ref, map[string]Value{}, nil); err == nil {
		t.Fatal("unbound placeholder head must error")
	}
}

func TestW8SubstituteGenInstRefArgError(t *testing.T) {
	r := w8reg(t)
	ref := NewGenInstRef(GenInstRef{Head: NewTypeLiteral(TInteger), Args: []Value{w8ParamLit(r)}})
	if _, err := SubstituteTypeParams(r, ref, map[string]Value{}, nil); err == nil {
		t.Fatal("unbound placeholder arg must error")
	}
}

func TestW8SubstituteSelfInFlightNode(t *testing.T) {
	r := w8reg(t)
	selfNode := r.Types.MintType("W8Self", TInteger)
	ref := NewGenInstRef(GenInstRef{Head: NewTypeLiteral(selfNode), Args: nil})
	got, err := SubstituteTypeParams(r, ref, map[string]Value{}, selfNode)
	if err != nil {
		t.Fatalf("Self head resolving to the in-flight node must not error: %v", err)
	}
	if !IsBareTypeNode(&got) || IDOf(&got) != IDOf(selfNode) {
		t.Fatalf("expected the self node literal, got %v", got)
	}
}

func TestW8SubstituteGenInstRefSchemaHead(t *testing.T) {
	r := w8reg(t)
	schemaNode := r.Types.MintType("W8Box", TType)
	info := &TypeSchemaInfo{Name: "W8Box", Body: NewTypeLiteral(TInteger), Kind: SchemaRecord}
	schemaVal := NewTypeSchema(schemaNode, info) // sets info.Type = schemaNode
	ref := NewGenInstRef(GenInstRef{Head: schemaVal, Args: nil})
	got, err := SubstituteTypeParams(r, ref, map[string]Value{}, nil)
	if err != nil {
		t.Fatalf("schema head should instantiate: %v", err)
	}
	if !ValueType(got).Equal(TInteger) {
		t.Fatalf("degenerate schema body is Integer, got %v", got)
	}
}

func TestW8SubstituteStructuralErrorArms(t *testing.T) {
	r := w8reg(t)
	empty := map[string]Value{}

	// Record field.
	rf := NewOrderedMap()
	rf.Set("x", w8ParamLit(r))
	if _, err := SubstituteTypeParams(r, NewRecordType(rf), empty, nil); err == nil {
		t.Fatal("record field placeholder must error")
	}

	// Object field.
	node := r.Types.MintType("W8O2", TClass)
	of := NewOrderedMap()
	of.Set("x", w8ParamLit(r))
	if _, err := SubstituteTypeParams(r, NewClassType(node, ClassTypeInfo{Fields: of}), empty, nil); err == nil {
		t.Fatal("object field placeholder must error")
	}

	// Typed list child.
	if _, err := SubstituteTypeParams(r, NewTypedList(w8ParamLit(r)), empty, nil); err == nil {
		t.Fatal("typed-list child placeholder must error")
	}

	// Typed map child.
	if _, err := SubstituteTypeParams(r, NewTypedMap(w8ParamLit(r)), empty, nil); err == nil {
		t.Fatal("typed-map child placeholder must error")
	}

	// Disjunct alternative (first alt OK, second errors).
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), w8ParamLit(r)})
	if _, err := SubstituteTypeParams(r, d, empty, nil); err == nil {
		t.Fatal("disjunct alt placeholder must error")
	}

	// Negation inner.
	if _, err := SubstituteTypeParams(r, NewNegation(w8ParamLit(r)), empty, nil); err == nil {
		t.Fatal("negation inner placeholder must error")
	}
}

func TestW8SubstituteFnUndefArms(t *testing.T) {
	r := w8reg(t)
	empty := map[string]Value{}

	// Parent is TFnUndef but the payload is not a FnUndefInfo → pass through.
	odd := NewValueRaw(TFnUndef, IntPayload{N: 1})
	got, err := SubstituteTypeParams(r, odd, empty, nil)
	if err != nil {
		t.Fatalf("non-FnUndefInfo TFnUndef value must pass through: %v", err)
	}
	if _, ok := got.Data.(IntPayload); !ok {
		t.Fatal("value should be returned unchanged")
	}

	// A param Pattern that references an unbound placeholder errors.
	pat := w8ParamLit(r)
	fuPat := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{Params: []FnParam{{Pattern: &pat}}}}})
	if _, err := SubstituteTypeParams(r, fuPat, empty, nil); err == nil {
		t.Fatal("fnundef param pattern placeholder must error")
	}

	// A return slot that is an unbound placeholder node errors.
	fuRet := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{Returns: []*Type{w8ParamNode(r)}}}})
	if _, err := SubstituteTypeParams(r, fuRet, empty, nil); err == nil {
		t.Fatal("fnundef return placeholder must error")
	}
}

// --- generics_instantiate.go: InstantiateSchema error arms -----------------

func TestW8InstantiateSchemaDefaultError(t *testing.T) {
	r := w8reg(t)
	uNode := MintTypeParam(r, GenParam{Name: "U"})
	schemaNode := r.Types.MintType("W8D", TType)
	info := &TypeSchemaInfo{
		Name:   "W8D",
		Type:   schemaNode,
		Params: []GenParam{{Name: "T", HasDefault: true, Default: NewTypeLiteral(uNode)}},
	}
	if _, err := InstantiateSchema(r, info, nil); err == nil {
		t.Fatal("default referencing an unbound placeholder must error")
	}
}

func TestW8InstantiateSchemaBodyError(t *testing.T) {
	r := w8reg(t)
	schemaNode := r.Types.MintType("W8B2", TType)
	info := &TypeSchemaInfo{
		Name: "W8B2",
		Type: schemaNode,
		Body: NewTypeLiteral(MintTypeParam(r, GenParam{Name: "Z"})),
	}
	if _, err := InstantiateSchema(r, info, nil); err == nil {
		t.Fatal("body referencing an unbound placeholder must error")
	}
}

func TestW8InstantiateSchemaClassBodyNotObject(t *testing.T) {
	r := w8reg(t)
	schemaNode := r.Types.MintType("W8C2", TType)
	info := &TypeSchemaInfo{
		Name: "W8C2",
		Type: schemaNode,
		Body: NewTypeLiteral(TInteger),
		Kind: SchemaClass,
	}
	if _, err := InstantiateSchema(r, info, nil); err == nil {
		t.Fatal("SchemaClass with a non-object body must error")
	}
}
