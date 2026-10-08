package core

import (
	"strings"
	"testing"
)

// Wave 4b coverage: generic schemas end to end — the spec / param /
// schema payload wrappers and the placeholder machinery (generics.go),
// substitution and instantiation (generics_instantiate.go), and the
// binding-inference layer (generics_unify.go).

// w4bSchema builds `def NAME gen [params...] refine Record body(...)`
// by hand: mints the placeholders, builds the record body via mkBody
// (keyed by placeholder node), pops the bindings, installs via
// InstallType, and returns the shared schema payload.
func w4bSchema(t *testing.T, r *Registry, name string, params []GenParam,
	mkBody func(spec *GenSpecInfo) Value) *TypeSchemaInfo {
	t.Helper()
	spec := &GenSpecInfo{Params: params}
	PushGenBindings(r, spec)
	body := mkBody(spec)
	PopGenBindings(r, spec)
	info := &TypeSchemaInfo{Params: spec.Params, Body: body, Kind: SchemaRecord}
	if err := InstallType(r, name, NewTypeSchema(TType, info)); err != nil {
		t.Fatalf("InstallType %s: %v", name, err)
	}
	return info
}

func w4bRecordBody(field string) func(*GenSpecInfo) Value {
	return func(spec *GenSpecInfo) Value {
		fields := NewOrderedMap()
		fields.Set(field, NewTypeLiteral(spec.Params[0].Node))
		return NewRecordType(fields)
	}
}

// --- payload wrappers (generics.go) ---------------------------------------------

func TestGenSpecAndParamWrappers(t *testing.T) {
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	sv := NewGenSpec(spec)
	if !IsGenSpec(sv) {
		t.Fatal("NewGenSpec not a spec")
	}
	if IsGenSpec(NewInteger(1)) {
		t.Error("integer claims spec")
	}
	got, err := AsGenSpec(sv)
	if err != nil || got != spec {
		t.Errorf("AsGenSpec = %v, %v", got, err)
	}
	if _, err := AsGenSpec(NewInteger(1)); err == nil {
		t.Error("AsGenSpec accepted an integer")
	}

	pv := NewGenParamValue(GenParam{Name: "U"})
	if !IsGenParamValue(pv) || IsGenParamValue(sv) {
		t.Error("param-value predicate broken")
	}
	p, err := AsGenParamValue(pv)
	if err != nil || p.Name != "U" {
		t.Errorf("AsGenParamValue = %+v, %v", p, err)
	}
	if _, err := AsGenParamValue(NewInteger(1)); err == nil {
		t.Error("AsGenParamValue accepted an integer")
	}

	if _, err := AsTypeSchema(NewInteger(1)); err == nil {
		t.Error("AsTypeSchema accepted an integer")
	}
	if _, err := AsGenInstRef(NewInteger(1)); err == nil {
		t.Error("AsGenInstRef accepted an integer")
	}
}

func TestPendingGenLifecycle(t *testing.T) {
	r := newTestRegistry(t)
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	if err := r.SetPendingGen(spec); err != nil {
		t.Fatalf("SetPendingGen: %v", err)
	}
	if r.PendingGen() != spec {
		t.Error("PendingGen did not report the parked spec")
	}
	// A second gen with no constructor between them errors.
	if err := r.SetPendingGen(&GenSpecInfo{}); err == nil {
		t.Error("double SetPendingGen accepted")
	}
	// Suspend removes; restore-once brings it back exactly once.
	restore := r.SuspendPendingGen()
	if r.PendingGen() != nil {
		t.Error("suspend left the spec pending")
	}
	restore()
	if r.PendingGen() != spec {
		t.Error("restore did not reinstate the spec")
	}
	r.TakePendingGen() // consume
	restore()          // second call is a no-op — must not resurrect
	if r.PendingGen() != nil {
		t.Error("restore resurrected a consumed spec")
	}
	if r.TakePendingGen() != nil {
		t.Error("TakePendingGen on empty returned a spec")
	}
}

func TestTypeParamNodesAndBounds(t *testing.T) {
	r := newTestRegistry(t)
	free := MintTypeParam(r, GenParam{Name: "T"})
	if !IsTypeParamNode(free) || TypeParamName(free) != "T" {
		t.Error("free placeholder mis-minted")
	}
	if !IsUnconstrainedTypeParam(free) {
		t.Error("free placeholder claims a bound")
	}
	if IsTypeParamNode(nil) || TypeParamName(nil) != "" || IsUnconstrainedTypeParam(nil) {
		t.Error("nil type-param probes not nil-safe")
	}
	if IsTypeParamNode(TInteger) || IsUnconstrainedTypeParam(TInteger) {
		t.Error("Integer claims placeholderhood")
	}
	// An unconstrained placeholder admits anything.
	if !NewInteger(1).Is(free) || !NewString("s").Is(free) {
		t.Error("unconstrained placeholder rejected a value")
	}
	// A bounded placeholder admits by Is-membership in the bound.
	bounded := MintTypeParam(r, GenParam{Name: "N", HasBound: true, Bound: NewTypeLiteral(TNumber)})
	if IsUnconstrainedTypeParam(bounded) {
		t.Error("bounded placeholder claims unconstrained")
	}
	if !NewInteger(1).Is(bounded) {
		t.Error("Integer rejected by extends Number")
	}
	if NewString("s").Is(bounded) {
		t.Error("String admitted by extends Number")
	}
	if bounded.Parent == nil || !bounded.Parent.Equal(TNumber) {
		t.Error("bounded placeholder not reparented at its bound")
	}
	// A compound bound (disjunct) routes through Unify.
	disjB := MintTypeParam(r, GenParam{Name: "D", HasBound: true,
		Bound: NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})})
	if !NewInteger(1).Is(disjB) || NewBoolean(true).Is(disjB) {
		t.Error("disjunct bound membership broken")
	}
	// Format renders the parameter name on a bare literal.
	if got := NewTypeLiteral(free).String(); got != "T" {
		t.Errorf("placeholder literal renders %q, want T", got)
	}
	// AttachGenBound retrofits a bound onto a provisional node.
	late := MintTypeParam(r, GenParam{Name: "L"})
	AttachGenBound(r, late, GenParam{Name: "L", HasBound: true, Bound: NewTypeLiteral(TString)})
	if IsUnconstrainedTypeParam(late) {
		t.Error("AttachGenBound did not land the bound")
	}
	if late.Parent == nil || !late.Parent.Equal(TString) {
		t.Error("AttachGenBound did not reparent")
	}
	if !NewString("s").Is(late) || NewInteger(1).Is(late) {
		t.Error("retrofitted bound not consulted")
	}
}

func TestSchemaUnifierAndInfoOf(t *testing.T) {
	r := newTestRegistry(t)
	info := w4bSchema(t, r, "W4bBox", []GenParam{{Name: "T"}}, w4bRecordBody("value"))
	node := r.LookupTypeName("W4bBox")
	if node == nil {
		t.Fatal("schema node not bound")
	}
	got, ok := SchemaInfoOf(node)
	if !ok || got != info {
		t.Error("SchemaInfoOf missed the installed schema")
	}
	if _, ok := SchemaInfoOf(TInteger); ok {
		t.Error("Integer claims a schema")
	}
	if _, ok := SchemaInfoOf(nil); ok {
		t.Error("nil claims a schema")
	}
	// The schema value renders via schemaUnifier.Format.
	sv, _ := r.TopTypeBody("W4bBox")
	if got := sv.String(); !strings.Contains(got, "schema<") || !strings.Contains(got, "[T]") {
		t.Errorf("schema render = %q", got)
	}
	// A child node conforms to the bare schema by plain ancestry (the
	// path an instantiation node takes — it mints as a child of the
	// schema node).
	child := r.Types.MintType("W4bBoxChild", node)
	if !NewTypeLiteral(child).Is(node) {
		t.Error("child literal does not conform to the schema node")
	}
	if _, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TInteger)}); err != nil {
		t.Fatalf("InstantiateSchema: %v", err)
	}
	// A plain value does not.
	if NewInteger(1).Is(node) {
		t.Error("integer conforms to the schema node")
	}
}

// --- substitution (generics_instantiate.go) --------------------------------------

func TestContainsTypeParamShapes(t *testing.T) {
	r := newTestRegistry(t)
	pn := MintTypeParam(r, GenParam{Name: "T"})
	lit := NewTypeLiteral(pn)

	if !containsTypeParam(lit) {
		t.Error("bare placeholder literal missed")
	}
	if !containsTypeParam(NewTypeLiteral(TSelf)) {
		t.Error("Self literal missed")
	}
	if containsTypeParam(NewTypeLiteral(TInteger)) {
		t.Error("Integer literal flagged")
	}
	fields := NewOrderedMap()
	fields.Set("v", lit)
	if !containsTypeParam(NewRecordType(fields)) {
		t.Error("record field placeholder missed")
	}
	clean := NewOrderedMap()
	clean.Set("v", NewTypeLiteral(TInteger))
	if containsTypeParam(NewRecordType(clean)) {
		t.Error("clean record flagged")
	}
	if !containsTypeParam(NewTypedList(lit)) || containsTypeParam(NewTypedList(NewTypeLiteral(TInteger))) {
		t.Error("typed-list child detection broken")
	}
	if !containsTypeParam(NewTypedMap(lit)) {
		t.Error("typed-map child placeholder missed")
	}
	if !containsTypeParam(NewDisjunct([]Value{NewTypeLiteral(TInteger), lit})) {
		t.Error("disjunct alternative placeholder missed")
	}
	if containsTypeParam(NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})) {
		t.Error("clean disjunct flagged")
	}
	if !containsTypeParam(NewNegation(lit)) || containsTypeParam(NewNegation(NewTypeLiteral(TInteger))) {
		t.Error("negation inner detection broken")
	}
	// GenInstRef head and args.
	ref := NewGenInstRef(GenInstRef{Head: NewTypeLiteral(TSelf), Args: []Value{NewTypeLiteral(TInteger)}})
	if !containsTypeParam(ref) {
		t.Error("Self-headed instref missed")
	}
	ref2 := NewGenInstRef(GenInstRef{Head: NewTypeLiteral(TInteger), Args: []Value{lit}})
	if !containsTypeParam(ref2) {
		t.Error("placeholder arg in instref missed")
	}
	ref3 := NewGenInstRef(GenInstRef{Head: NewTypeLiteral(TInteger), Args: []Value{NewTypeLiteral(TString)}})
	if containsTypeParam(ref3) {
		t.Error("clean instref flagged")
	}
	// Fn-shape params / patterns / returns.
	fnP := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{Params: []FnParam{{Name: "x", Type: pn}}}}})
	if !containsTypeParam(fnP) {
		t.Error("fn param placeholder missed")
	}
	pat := NewTypedList(lit)
	fnPat := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{Params: []FnParam{{Name: "x", Type: TList, Pattern: &pat}}}}})
	if !containsTypeParam(fnPat) {
		t.Error("fn pattern placeholder missed")
	}
	fnR := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{Returns: []*Type{pn}}}})
	if !containsTypeParam(fnR) {
		t.Error("fn return placeholder missed")
	}
	fnClean := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{
		Params:  []FnParam{{Name: "x", Type: TInteger}},
		Returns: []*Type{TString},
	}}})
	if containsTypeParam(fnClean) {
		t.Error("clean fn shape flagged")
	}
}

func TestSubstituteTypeParamsShapes(t *testing.T) {
	r := newTestRegistry(t)
	pn := MintTypeParam(r, GenParam{Name: "T"})
	lit := NewTypeLiteral(pn)
	bindings := map[string]Value{"T": NewTypeLiteral(TInteger)}
	self := r.Types.MintType("W4bSelfNode", TRecord)

	// Raw word naming the parameter substitutes by name.
	got, err := SubstituteTypeParams(r, NewWord("T"), bindings, nil)
	if err != nil || !(&got).Equal(TInteger) {
		t.Errorf("word T → %v (%v)", got, err)
	}
	// A word naming nothing passes through.
	got, _ = SubstituteTypeParams(r, NewWord("unrelated"), bindings, nil)
	if !IsWord(got) {
		t.Errorf("unrelated word rewritten: %v", got)
	}
	// The Self word resolves to the in-flight node.
	got, _ = SubstituteTypeParams(r, NewWord("Self"), bindings, self)
	if !(&got).Equal(self) {
		t.Errorf("Self word → %v", got)
	}
	// Bare placeholder literal.
	got, err = SubstituteTypeParams(r, lit, bindings, nil)
	if err != nil || !(&got).Equal(TInteger) {
		t.Errorf("bare T → %v (%v)", got, err)
	}
	// Unbound placeholder errors.
	if _, err := SubstituteTypeParams(r, lit, map[string]Value{}, nil); err == nil {
		t.Error("unbound placeholder substituted")
	}
	// Self literal resolves to the in-flight node; passes through without one.
	got, _ = SubstituteTypeParams(r, NewTypeLiteral(TSelf), bindings, self)
	if !(&got).Equal(self) {
		t.Errorf("Self literal → %v", got)
	}
	got, _ = SubstituteTypeParams(r, NewTypeLiteral(TString), bindings, nil)
	if !(&got).Equal(TString) {
		t.Errorf("plain literal rewritten: %v", got)
	}
	// Record fields.
	fields := NewOrderedMap()
	fields.Set("v", lit)
	got, err = SubstituteTypeParams(r, NewRecordType(fields), bindings, nil)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	ri, _ := AsRecordType(got)
	fv, _ := ri.Fields.Get("v")
	if !(&fv).Equal(TInteger) {
		t.Errorf("record field → %v", fv)
	}
	// Typed list / typed map children.
	got, err = SubstituteTypeParams(r, NewTypedList(lit), bindings, nil)
	if err != nil {
		t.Fatalf("typed list: %v", err)
	}
	ci, _ := AsChildType(got)
	if !(&ci.Child).Equal(TInteger) {
		t.Errorf("typed-list child → %v", ci.Child)
	}
	got, err = SubstituteTypeParams(r, NewTypedMap(lit), bindings, nil)
	if err != nil {
		t.Fatalf("typed map: %v", err)
	}
	ci, _ = AsChildType(got)
	if !(&ci.Child).Equal(TInteger) {
		t.Errorf("typed-map child → %v", ci.Child)
	}
	// Disjunct and negation recurse.
	got, err = SubstituteTypeParams(r, NewDisjunct([]Value{lit, NewTypeLiteral(TString)}), bindings, nil)
	if err != nil {
		t.Fatalf("disjunct: %v", err)
	}
	di, _ := AsDisjunct(got)
	if !(&di.Alternatives[0]).Equal(TInteger) {
		t.Errorf("disjunct alt → %v", di.Alternatives[0])
	}
	got, err = SubstituteTypeParams(r, NewNegation(lit), bindings, nil)
	if err != nil {
		t.Fatalf("negation: %v", err)
	}
	ni, _ := AsNegation(got)
	if !(&ni.Inner).Equal(TInteger) {
		t.Errorf("negation inner → %v", ni.Inner)
	}
	// Fn-shape params, patterns, and returns.
	pat := NewTypedList(lit)
	fn := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{
		Params: []FnParam{
			{Name: "x", Type: pn},
			{Name: "xs", Type: TList, Pattern: &pat},
		},
		Returns: []*Type{pn},
	}}})
	got, err = SubstituteTypeParams(r, fn, bindings, nil)
	if err != nil {
		t.Fatalf("fn shape: %v", err)
	}
	fi, ok := got.Data.(FnUndefInfo)
	if !ok {
		t.Fatalf("fn shape result = %v", got)
	}
	if !fi.Sigs[0].Params[0].Type.Equal(TInteger) || !fi.Sigs[0].Returns[0].Equal(TInteger) {
		t.Error("fn param/return slot not substituted")
	}
	pci, _ := AsChildType(*fi.Sigs[0].Params[1].Pattern)
	if !(&pci.Child).Equal(TInteger) {
		t.Errorf("fn pattern child → %v", pci.Child)
	}
	// A structural binding cannot occupy a *Type slot.
	structural := map[string]Value{"T": NewRecordType(NewOrderedMap())}
	if _, err := SubstituteTypeParams(r, fn, structural, nil); err == nil {
		t.Error("structural binding accepted in an fn-shape slot")
	}
	// Concrete values pass through untouched.
	got, _ = SubstituteTypeParams(r, NewInteger(42), bindings, nil)
	if n, _ := AsInteger(got); n != 42 {
		t.Errorf("concrete rewritten: %v", got)
	}
	// substituteParamNode: nil stays nil; non-placeholder passes through.
	if nt, err := substituteParamNode(r, nil, bindings, nil); err != nil || nt != nil {
		t.Error("nil param node not preserved")
	}
	if nt, _ := substituteParamNode(r, TInteger, bindings, nil); nt != TInteger {
		t.Error("plain param node rewritten")
	}
	if nt, _ := substituteParamNode(r, TSelf, bindings, self); nt != self {
		t.Error("Self param node not tied to the in-flight node")
	}
	if _, err := substituteParamNode(r, pn, map[string]Value{}, nil); err == nil {
		t.Error("unbound param node substituted")
	}
}

func TestInstantiateSchemaDeferredAndNested(t *testing.T) {
	r := newTestRegistry(t)
	info := w4bSchema(t, r, "W4bDefBox", []GenParam{{Name: "T"}}, w4bRecordBody("value"))

	// Args still carrying a placeholder defer to a GenInstRef.
	other := MintTypeParam(r, GenParam{Name: "U"})
	deferred, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(other)})
	if err != nil {
		t.Fatalf("deferred instantiation: %v", err)
	}
	if !IsGenInstRef(deferred) {
		t.Fatalf("deferred = %v, want GenInstRef", deferred)
	}
	ref, _ := AsGenInstRef(deferred)
	if len(ref.Args) != 1 {
		t.Errorf("ref args = %v", ref.Args)
	}
	// Substituting the deferred ref with a concrete binding resolves it
	// through the real instantiation.
	resolved, err := SubstituteTypeParams(r, deferred, map[string]Value{"U": NewTypeLiteral(TString)}, nil)
	if err != nil {
		t.Fatalf("resolve deferred: %v", err)
	}
	ri, aerr := AsRecordType(resolved)
	if aerr != nil {
		t.Fatalf("resolved = %v (%v)", resolved, aerr)
	}
	fv, _ := ri.Fields.Get("value")
	if !(&fv).Equal(TString) {
		t.Errorf("resolved field = %v, want String", fv)
	}
	// A head that resolves to no schema errors.
	bad := NewGenInstRef(GenInstRef{Head: NewTypeLiteral(TInteger), Args: []Value{NewTypeLiteral(TString)}})
	if _, err := SubstituteTypeParams(r, bad, map[string]Value{}, nil); err == nil {
		t.Error("non-schema head resolved")
	}
}

func TestInstantiateSchemaSelfRecursion(t *testing.T) {
	r := newTestRegistry(t)
	// def W4bLink gen [T] refine Record {v:T next:(Self of [T])}
	info := w4bSchema(t, r, "W4bLink", []GenParam{{Name: "T"}}, func(spec *GenSpecInfo) Value {
		fields := NewOrderedMap()
		fields.Set("v", NewTypeLiteral(spec.Params[0].Node))
		fields.Set("next", NewGenInstRef(GenInstRef{
			Head: NewTypeLiteral(TSelf),
			Args: []Value{NewTypeLiteral(spec.Params[0].Node)},
		}))
		return NewRecordType(fields)
	})
	inst, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TInteger)})
	if err != nil {
		t.Fatalf("recursive instantiation: %v", err)
	}
	ri, aerr := AsRecordType(inst)
	if aerr != nil {
		t.Fatalf("instantiation = %v (%v)", inst, aerr)
	}
	next, ok := ri.Fields.Get("next")
	if !ok {
		t.Fatal("next field lost")
	}
	// The knot ties back to the in-flight instantiation node.
	if !IsBareTypeNode(next) || next.Parent == nil || !next.Parent.Equal(info.Type) {
		t.Errorf("next = %v, want the instantiation node literal", next)
	}
}

func TestSchemaShortNameAndCanonArg(t *testing.T) {
	if schemaShortName(&TypeSchemaInfo{Name: "Class/Box"}) != "Box" {
		t.Error("path short name broken")
	}
	if schemaShortName(&TypeSchemaInfo{Name: "Plain"}) != "Plain" {
		t.Error("plain short name broken")
	}
	if schemaShortName(&TypeSchemaInfo{Type: &Type{tmeta: &typeMeta{Name: "NodeName"}}}) != "NodeName" {
		t.Error("node-name fallback broken")
	}
	if schemaShortName(&TypeSchemaInfo{}) != "<schema>" {
		t.Error("empty short name fallback broken")
	}
	if canonTypeArg(NewTypeLiteral(TInteger)) != "Integer" {
		t.Error("named node canon broken")
	}
	if got := canonTypeArg(NewInteger(3)); got != "3" {
		t.Errorf("value canon = %q", got)
	}
	// typeArgNode: class types denote their minted node.
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	if err := InstallType(r, "W4bCArg", NewValueRaw(TClass, ClassTypeInfo{Fields: fields})); err != nil {
		t.Fatalf("class InstallType: %v", err)
	}
	body, _ := r.TopTypeBody("W4bCArg")
	if node := typeArgNode(body); node == nil || node.Name() != "W4bCArg" {
		t.Errorf("class typeArgNode = %v", node)
	}
	if got := canonTypeArg(body); got != "W4bCArg" {
		t.Errorf("class canon arg = %q", got)
	}
	if typeArgNode(NewInteger(1)) != nil {
		t.Error("concrete value denotes a node")
	}
}

// --- inference (generics_unify.go) -----------------------------------------------

func TestResolveSigChildParam(t *testing.T) {
	r := newTestRegistry(t)
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	PushGenBindings(r, spec)
	defer PopGenBindings(r, spec)

	// A raw-Word child naming the live placeholder resolves to its literal.
	tl := NewTypedList(NewWord("T"))
	got := ResolveSigChildParam(r, tl)
	ci, _ := AsChildType(got)
	if !IsBareTypeNode(ci.Child) || TypeParamName(&ci.Child) != "T" {
		t.Errorf("resolved child = %v", ci.Child)
	}
	// Typed map form.
	got = ResolveSigChildParam(r, NewTypedMap(NewWord("T")))
	ci, _ = AsChildType(got)
	if TypeParamName(&ci.Child) != "T" {
		t.Errorf("typed-map child = %v", ci.Child)
	}
	// Elements / entries survive the rebuild.
	withElems := NewTypedListWithElements(NewWord("T"), []Value{NewInteger(1)})
	got = ResolveSigChildParam(r, withElems)
	ci, _ = AsChildType(got)
	if len(ci.Elements) != 1 {
		t.Error("typed-list elements lost")
	}
	withEntries := NewTypedMapWithEntries(NewWord("T"), []ChildEntry{{Key: "k", Value: NewInteger(1)}})
	got = ResolveSigChildParam(r, withEntries)
	ci, _ = AsChildType(got)
	if len(ci.Entries) != 1 {
		t.Error("typed-map entries lost")
	}
	// A word naming a non-placeholder passes through untouched.
	passthru := NewTypedList(NewWord("NoSuchParam"))
	got = ResolveSigChildParam(r, passthru)
	ci, _ = AsChildType(got)
	if !IsWord(ci.Child) {
		t.Error("unrelated word child rewritten")
	}
	// Non-childtype values and nil registries pass through.
	if v := ResolveSigChildParam(r, NewInteger(1)); !IsConcrete(v) {
		t.Error("integer rewritten")
	}
	if v := ResolveSigChildParam(nil, tl); !IsTypedList(v) {
		t.Error("nil registry rewrote the value")
	}
	// A literal child (already resolved) passes through.
	lit := NewTypedList(NewTypeLiteral(TInteger))
	got = ResolveSigChildParam(r, lit)
	ci, _ = AsChildType(got)
	if !(&ci.Child).Equal(TInteger) {
		t.Error("literal child rewritten")
	}
}

// TestResolveSigRecordFields covers the structural-record twin of
// ResolveSigChildParam: an INLINE record parameter's field type words resolve
// at sig install, where the named `type R record {…}` spelling had them
// resolved by `record`'s own dispatch. An unresolved field word used to reach
// the schema-bearing param carrier, so a body field read narrowed to
// dynamic(Word) and the step loop then dispatched that carrier as a nameless
// token (NUR103).
func TestResolveSigRecordFields(t *testing.T) {
	r := newTestRegistry(t)
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	PushGenBindings(r, spec)
	defer PopGenBindings(r, spec)
	if err := InstallType(r, "RsrfRec", NewRecordType(intStrRecord().Fields)); err != nil {
		t.Fatalf("InstallType: %v", err)
	}

	fields := NewOrderedMap()
	fields.Set("pretty", NewWord("Boolean")) // builtin name
	fields.Set("param", NewWord("T"))        // live type-param placeholder
	fields.Set("rec", NewWord("RsrfRec"))    // user type body
	fields.Set("nope", NewWord("notAType"))  // names no type
	fields.Set("status", NewString("ok"))    // a value is a type (ADR-010)
	got := ResolveSigRecordFields(r, NewMap(fields))
	gm, err := AsMap(got)
	if err != nil {
		t.Fatalf("AsMap: %v", err)
	}
	pretty, _ := gm.Get("pretty")
	if !IsBareTypeNode(pretty) || !ValueType(pretty).Equal(TBoolean) {
		t.Errorf("builtin field = %v (want the Boolean type)", pretty)
	}
	param, _ := gm.Get("param")
	if !IsBareTypeNode(param) || TypeParamName(&param) != "T" {
		t.Errorf("type-param field = %v", param)
	}
	rec, _ := gm.Get("rec")
	if !IsRecordType(rec) {
		t.Errorf("user-type field = %v (want the record body)", rec)
	}
	nope, _ := gm.Get("nope")
	if !IsWord(nope) {
		t.Errorf("word naming no type rewritten: %v", nope)
	}
	status, _ := gm.Get("status")
	if !IsConcrete(status) {
		t.Errorf("literal value pattern rewritten: %v", status)
	}

	// A field type EXPRESSION evaluates, and a paren that does not name one
	// type is left alone — silently, since a sig install is no place to raise.
	exprFields := NewOrderedMap()
	exprFields.Set("one", NewParenExpr([]Value{NewWord("Integer")}))
	exprFields.Set("two", NewParenExpr([]Value{NewWord("Integer"), NewWord("String")}))
	exprFields.Set("val", NewParenExpr([]Value{NewInteger(1)}))
	exprFields.Set("bad", NewParenExpr([]Value{NewWord("noSuchWordAtAll")}))
	em, _ := AsMap(ResolveSigRecordFields(r, NewMap(exprFields)))
	one, _ := em.Get("one")
	if !IsBareTypeNode(one) || !ValueType(one).Equal(TInteger) {
		t.Errorf("field type expression = %v (want the Integer type)", one)
	}
	for _, k := range []string{"two", "val", "bad"} {
		if fv, _ := em.Get(k); !IsParenExpr(fv) {
			t.Errorf("field %q rewritten: %v", k, fv)
		}
	}

	// A nested inline record resolves recursively.
	inner := NewOrderedMap()
	inner.Set("b", NewWord("Integer"))
	outer := NewOrderedMap()
	outer.Set("a", NewMap(inner))
	nested := ResolveSigRecordFields(r, NewMap(outer))
	nm, _ := AsMap(nested)
	av, _ := nm.Get("a")
	im, _ := AsMap(av)
	bv, _ := im.Get("b")
	if !IsBareTypeNode(bv) || !ValueType(bv).Equal(TInteger) {
		t.Errorf("nested field = %v (want the Integer type)", bv)
	}

	// Nothing to resolve: the SAME value comes back, not an equal copy — the
	// pattern is stored on the signature and compared by the dispatcher.
	inertFields := NewOrderedMap()
	inertFields.Set("a", NewInteger(1))
	inertFields.Set("b", NewMap(NewOrderedMap()))
	inert := NewMap(inertFields)
	if out := ResolveSigRecordFields(r, inert); out.ID != inert.ID {
		t.Errorf("all-concrete field map rebuilt: %v", out)
	}

	// Non-map values, an empty payload, and a nil registry pass through.
	if v := ResolveSigRecordFields(r, NewInteger(1)); !IsConcrete(v) {
		t.Error("integer rewritten")
	}
	if v := ResolveSigRecordFields(r, NewMap(nil)); v.Data == nil {
		t.Error("nil-backed map rewritten")
	}
	if v := ResolveSigRecordFields(nil, NewMap(fields)); !v.Parent.Equal(TMap) {
		t.Error("nil registry rewrote the value")
	}
}

func TestResolveChildTypeExpr(t *testing.T) {
	r := newTestRegistry(t)
	// A ParenExpr child evaluates in a sub-engine.
	pe := NewParenExpr([]Value{NewTypeLiteral(TInteger)})
	got, err := ResolveChildTypeExpr(r, NewTypedList(pe))
	if err != nil {
		t.Fatalf("ResolveChildTypeExpr: %v", err)
	}
	ci, _ := AsChildType(got)
	if !(&ci.Child).Equal(TInteger) {
		t.Errorf("evaluated child = %v", ci.Child)
	}
	// Typed-map variant with entries preserved.
	tm := NewTypedMapWithEntries(pe, []ChildEntry{{Key: "k", Value: NewInteger(1)}})
	got, err = ResolveChildTypeExpr(r, tm)
	if err != nil {
		t.Fatalf("typed-map expr: %v", err)
	}
	ci, _ = AsChildType(got)
	if !(&ci.Child).Equal(TInteger) || len(ci.Entries) != 1 {
		t.Errorf("typed-map evaluated child = %v", ci.Child)
	}
	// An expression producing two values errors.
	multi := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})
	if _, err := ResolveChildTypeExpr(r, NewTypedList(multi)); err == nil {
		t.Error("multi-value child expression accepted")
	}
	// Non-paren children and non-typed values pass through.
	plain := NewTypedList(NewTypeLiteral(TString))
	if got, err := ResolveChildTypeExpr(r, plain); err != nil || !IsTypedList(got) {
		t.Error("plain typed list rewritten")
	}
	if got, err := ResolveChildTypeExpr(r, NewInteger(1)); err != nil || !IsConcrete(got) {
		t.Error("integer rewritten")
	}
	if got, err := ResolveChildTypeExpr(nil, plain); err != nil || !IsTypedList(got) {
		t.Error("nil registry rewrote the value")
	}
}

func TestUnifyTypeParamAndLitNode(t *testing.T) {
	r := newTestRegistry(t)
	pn := MintTypeParam(r, GenParam{Name: "T", HasBound: true, Bound: NewTypeLiteral(TNumber)})
	lit := NewTypeLiteral(pn)

	if typeParamLitNode(lit) == nil {
		t.Error("placeholder literal not detected")
	}
	if typeParamLitNode(NewTypeLiteral(TInteger)) != nil || typeParamLitNode(NewInteger(1)) != nil {
		t.Error("typeParamLitNode overclaims")
	}
	// Same placeholder on both sides.
	if got, err := unifyTypeParam(lit, pn, NewTypeLiteral(pn), nil); err != nil || !IsBareTypeNode(got) {
		t.Errorf("T vs T = %v, %v", got, err)
	}
	// A value satisfying the bound is admitted.
	if got, err := unifyTypeParam(lit, pn, NewInteger(5), nil); err != nil {
		t.Errorf("5 vs T extends Number: %v", err)
	} else if n, _ := AsInteger(got); n != 5 {
		t.Errorf("admitted = %v", got)
	}
	// A value outside the bound fails.
	if _, err := unifyTypeParam(lit, pn, NewString("s"), nil); err == nil {
		t.Error("string admitted by extends Number")
	}
	// The Unify fold reaches the placeholder embedded in a typed list.
	if _, ok := Unify(NewTypedList(lit), NewList([]Value{NewInteger(1), NewInteger(2)})); !ok {
		t.Error("typed-list-over-placeholder rejected a conforming list")
	}
	if _, ok := Unify(NewTypedList(lit), NewList([]Value{NewString("x")})); ok {
		t.Error("typed-list-over-placeholder admitted a violating list")
	}
}

func TestInferGenBindings(t *testing.T) {
	r := newTestRegistry(t)
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	PushGenBindings(r, spec)
	defer PopGenBindings(r, spec)
	pn := spec.Params[0].Node

	if got := InferGenBindings(nil, nil, nil); len(got) != 0 {
		t.Error("nil spec produced bindings")
	}
	// Placeholder param slot binds typeof(arg).
	params := []FnParam{{Name: "x", Type: pn}}
	b := InferGenBindings(spec, params, []Value{NewInteger(1)})
	if bv, ok := b["T"]; !ok || !(&bv).Equal(TInteger) {
		t.Errorf("T binding = %v", b["T"])
	}
	// Fewer args than params stops cleanly.
	if got := InferGenBindings(spec, params, nil); len(got) != 0 {
		t.Error("missing arg produced a binding")
	}
	// Typed-list pattern over the placeholder: union of element types.
	pat := NewTypedList(NewTypeLiteral(pn))
	lparams := []FnParam{{Name: "xs", Type: TList, Pattern: &pat}}
	b = InferGenBindings(spec, lparams, []Value{NewList([]Value{NewInteger(1), NewString("s")})})
	if bv, ok := b["T"]; !ok || !IsDisjunct(bv) {
		t.Errorf("mixed-list binding = %v", b["T"])
	}
	// A typed-list ARG contributes its child constraint directly.
	b = InferGenBindings(spec, lparams, []Value{NewTypedList(NewTypeLiteral(TString))})
	if bv, ok := b["T"]; !ok || !(&bv).Equal(TString) {
		t.Errorf("typed-list arg binding = %v", b["T"])
	}
	// Typed-map pattern with a concrete map arg.
	mpat := NewTypedMap(NewTypeLiteral(pn))
	mparams := []FnParam{{Name: "m", Type: TMap, Pattern: &mpat}}
	b = InferGenBindings(spec, mparams, []Value{mapOf("a", NewBoolean(true))})
	if bv, ok := b["T"]; !ok || !(&bv).Equal(TBoolean) {
		t.Errorf("map binding = %v", b["T"])
	}
	// A raw-Word pattern child also names the parameter.
	wpat := NewTypedList(NewWord("T"))
	wparams := []FnParam{{Name: "xs", Type: TList, Pattern: &wpat}}
	b = InferGenBindings(spec, wparams, []Value{NewList([]Value{NewFloat(1.5)})})
	if bv, ok := b["T"]; !ok || !(&bv).Equal(TFloat) {
		t.Errorf("word-pattern binding = %v", b["T"])
	}
	// A pattern over a non-parameter name binds nothing.
	opat := NewTypedList(NewWord("Zzz"))
	oparams := []FnParam{{Name: "xs", Type: TList, Pattern: &opat}}
	if got := InferGenBindings(spec, oparams, []Value{NewList([]Value{NewInteger(1)})}); len(got) != 0 {
		t.Error("non-param pattern bound")
	}
}

func TestInferSchemaBindingsAndInstantiate(t *testing.T) {
	r := newTestRegistry(t)
	info := w4bSchema(t, r, "W4bInfBox", []GenParam{{Name: "T"}}, w4bRecordBody("value"))

	if got := InferSchemaBindings(nil, mapOf("value", NewInteger(1))); len(got) != 0 {
		t.Error("nil schema produced bindings")
	}
	// Field evidence binds the parameter.
	b := InferSchemaBindings(info, mapOf("value", NewInteger(42)))
	if bv, ok := b["T"]; !ok || !(&bv).Equal(TInteger) {
		t.Errorf("schema binding = %v", b["T"])
	}
	// A non-map body binds nothing.
	if got := InferSchemaBindings(info, NewInteger(1)); len(got) != 0 {
		t.Error("non-map body bound")
	}
	// schemaFields on an fn-shape schema is nil.
	if schemaFields(&TypeSchemaInfo{Body: NewFnUndef(FnUndefInfo{})}) != nil {
		t.Error("fn-shape schema has fields")
	}
	// Full inference + instantiation from a construction body.
	inst, err := InferAndInstantiateSchema(r, NewTypeSchema(info.Type, info), mapOf("value", NewString("s")))
	if err != nil {
		t.Fatalf("InferAndInstantiateSchema: %v", err)
	}
	ri, _ := AsRecordType(inst)
	fv, _ := ri.Fields.Get("value")
	// typeof "s" is the exact leaf (a String subtype), so check by family.
	if !IsBareTypeNode(fv) || !(&fv).ConformsTo(TString) {
		t.Errorf("inferred field = %v", fv)
	}
	// An uninferable, undefaulted parameter is a loud unbound_param.
	if _, err := InferAndInstantiateSchema(r, NewTypeSchema(info.Type, info), mapOf("other", NewInteger(1))); err == nil {
		t.Error("uninferable parameter silently instantiated")
	}
	// A non-schema value errors.
	if _, err := InferAndInstantiateSchema(r, NewInteger(1), mapOf("v", NewInteger(1))); err == nil {
		t.Error("non-schema accepted")
	}
	// Typed-list field constraints contribute element evidence.
	tinfo := w4bSchema(t, r, "W4bInfLst", []GenParam{{Name: "T"}}, func(spec *GenSpecInfo) Value {
		fields := NewOrderedMap()
		fields.Set("xs", NewTypedList(NewTypeLiteral(spec.Params[0].Node)))
		return NewRecordType(fields)
	})
	b = InferSchemaBindings(tinfo, mapOf("xs", NewList([]Value{NewBoolean(true)})))
	if bv, ok := b["T"]; !ok || !(&bv).Equal(TBoolean) {
		t.Errorf("typed-list field binding = %v", b["T"])
	}
}

func TestGenBindingCarrierAndInstall(t *testing.T) {
	r := newTestRegistry(t)
	// A node binding becomes a carrier of that node.
	c := GenBindingCarrier(r, NewTypeLiteral(TInteger))
	if !c.Carrier || !c.Parent.Equal(TInteger) {
		t.Errorf("node carrier = %v", c)
	}
	// A tor-merged disjunct stays a disjunct carrier.
	c = GenBindingCarrier(r, NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)}))
	if !c.Carrier || !IsDisjunct(c) {
		t.Errorf("disjunct carrier = %v", c)
	}
	// A concrete value falls back to its Parent.
	c = GenBindingCarrier(r, NewInteger(5))
	if !c.Carrier || !c.Parent.Equal(TInteger) {
		t.Errorf("value carrier = %v", c)
	}
	// A payload-bearing value with no Parent degrades to Any.
	c = GenBindingCarrier(r, Value{Data: IntPayload{N: 1}})
	if !c.Carrier || !c.Parent.Equal(TAny) {
		t.Errorf("parentless carrier = %v", c)
	}

	// InstallGenCallBindings infers and installs body-scoped bindings.
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	PushGenBindings(r, spec)
	pn := spec.Params[0].Node
	PopGenBindings(r, spec)
	installed := InstallGenCallBindings(r, spec, []FnParam{{Name: "x", Type: pn}}, []Value{NewString("s")})
	if len(installed) != 1 || installed[0] != "T" {
		t.Fatalf("installed = %v", installed)
	}
	def := r.LookupTypeName("T")
	if def == nil || !def.ConformsTo(TString) {
		t.Errorf("T resolves to %v, want a String type", def)
	}
	for _, n := range installed {
		r.Defs.Pop(n)
	}
	// A parameter with no inferred binding is skipped.
	if got := InstallGenBindingMap(r, spec, map[string]Value{}); len(got) != 0 {
		t.Error("empty binding map installed names")
	}
}
