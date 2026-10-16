package core

import (
	"strings"
	"testing"
)

func flexSampleMap() Value {
	inner := NewOrderedMap()
	inner.Set("x", NewInteger(1))
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewList([]Value{NewInteger(2), NewMap(inner)}))
	return NewMap(m)
}

// --- AdoptIntoFlex ---------------------------------------------------------

func TestAdoptIntoFlexDeepCopiesPlainContainers(t *testing.T) {
	src := flexSampleMap()
	got, err := AdoptIntoFlex(src)
	if err != nil {
		t.Fatalf("AdoptIntoFlex(map): %v", err)
	}
	if !IsFlexNode(got) {
		t.Fatalf("adopted map is not flex: %v", got)
	}
	// Deep: the nested list became flex too, and mutating the copy does
	// not leak into the immutable source.
	fm, err := AsMutableMap(got)
	if err != nil {
		t.Fatalf("flex map payload: %v", err)
	}
	fm.Set("a", NewInteger(99))
	sm, _ := AsMap(src)
	orig, _ := sm.Get("a")
	if n, _ := AsInteger(orig); n != 1 {
		t.Errorf("mutating the flex copy leaked into the source: %v", orig)
	}
	b, _ := fm.Get("b")
	if !IsFlexNode(b) {
		t.Errorf("nested list was not flexed: %v", b)
	}

	// Lists too.
	lgot, err := AdoptIntoFlex(NewList([]Value{NewInteger(1)}))
	if err != nil {
		t.Fatalf("AdoptIntoFlex(list): %v", err)
	}
	if !IsFlexNode(lgot) {
		t.Errorf("adopted list is not flex: %v", lgot)
	}

	// Xml elements too.
	xgot, err := AdoptIntoFlex(NewXmlElement("p", nil, []Value{NewString("hi")}))
	if err != nil {
		t.Fatalf("AdoptIntoFlex(xml): %v", err)
	}
	if !IsFlexNode(xgot) {
		t.Errorf("adopted xml is not flex: %v", xgot)
	}
}

func TestAdoptIntoFlexPassThroughs(t *testing.T) {
	// Already-flex values share the handle.
	om := NewOrderedMap()
	om.Set("k", NewInteger(1))
	fx := NewFlexMap(om)
	got, err := AdoptIntoFlex(fx)
	if err != nil {
		t.Fatalf("AdoptIntoFlex(flex): %v", err)
	}
	gm, _ := AsMutableMap(got)
	gm.Set("k2", NewInteger(2))
	if om.Len() != 2 {
		t.Error("flex pass-through did not share the handle")
	}

	// Non-concrete (type literal) passes through.
	if v, err := AdoptIntoFlex(NewTypeLiteral(TMap)); err != nil || !IsBareTypeNode(&v) {
		t.Errorf("type literal: v=%v err=%v", v, err)
	}
	// Scalars pass through.
	if v, err := AdoptIntoFlex(NewInteger(7)); err != nil {
		t.Errorf("scalar: %v", err)
	} else if n, _ := AsInteger(v); n != 7 {
		t.Errorf("scalar changed: %v", v)
	}
	// A map-family TYPE BODY (RecordTypeInfo) is not an OrderedMap
	// payload — it passes through unchanged rather than erroring.
	rec := NewRecordType(intStrRecord().Fields)
	if v, err := AdoptIntoFlex(rec); err != nil {
		t.Errorf("record body: %v", err)
	} else if !IsRecordType(v) {
		t.Errorf("record body changed shape: %v", v)
	}
}

// --- AdoptIntoNode ---------------------------------------------------------

func TestAdoptIntoNodeNormalisesFlex(t *testing.T) {
	fom := NewOrderedMap()
	fom.Set("k", NewFlexList([]Value{NewInteger(1)}))
	fx := NewFlexMap(fom)
	got, err := AdoptIntoNode(fx)
	if err != nil {
		t.Fatalf("AdoptIntoNode(flex): %v", err)
	}
	if IsFlexNode(got) {
		t.Fatalf("node copy is still flex: %v", got)
	}
	gm, _ := AsMap(got)
	kv, _ := gm.Get("k")
	if IsFlexNode(kv) {
		t.Errorf("nested flex list survived: %v", kv)
	}

	// Identity fast path: a plain container with no flex inside is
	// returned unchanged.
	src := flexSampleMap()
	same, err := AdoptIntoNode(src)
	if err != nil {
		t.Fatalf("AdoptIntoNode(plain): %v", err)
	}
	if !ExactEqual(src, same) {
		t.Errorf("plain map was rebuilt: %v", same)
	}

	// A plain list carrying a flex map inside gets rebuilt.
	lst := NewList([]Value{NewFlexMap(NewOrderedMap())})
	nl, err := AdoptIntoNode(lst)
	if err != nil {
		t.Fatalf("AdoptIntoNode(list-with-flex): %v", err)
	}
	le, _ := AsList(nl)
	if IsFlexNode(le.Get(0)) {
		t.Errorf("flex element survived node conversion: %v", le.Get(0))
	}

	// Flex XML normalises too.
	fxm, err := FlexDeepCopy(NewXmlElement("p", nil, []Value{NewString("t")}))
	if err != nil {
		t.Fatalf("FlexDeepCopy(xml): %v", err)
	}
	nx, err := AdoptIntoNode(fxm)
	if err != nil {
		t.Fatalf("AdoptIntoNode(flexxml): %v", err)
	}
	if IsFlexNode(nx) {
		t.Errorf("flex xml survived: %v", nx)
	}
}

// --- MakeNodeHandler ---------------------------------------------------------

func TestMakeNodeHandlerConversions(t *testing.T) {
	r := newTestRegistry(t)

	// make FlexMap m
	out, err := MakeNodeHandler([]Value{NewTypeLiteral(TFlexMap), flexSampleMap()}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsFlexNode(out[0]) {
		t.Errorf("make FlexMap: out=%v err=%v", out, err)
	}
	// make FlexList l
	out, err = MakeNodeHandler([]Value{NewTypeLiteral(TFlexList), NewList([]Value{NewInteger(1)})}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsFlexNode(out[0]) {
		t.Errorf("make FlexList: out=%v err=%v", out, err)
	}
	// make Map flexmap — the inverse.
	fom := NewOrderedMap()
	fom.Set("a", NewInteger(1))
	out, err = MakeNodeHandler([]Value{NewTypeLiteral(TMap), NewFlexMap(fom)}, nil, nil, r)
	if err != nil || len(out) != 1 || IsFlexNode(out[0]) {
		t.Errorf("make Map: out=%v err=%v", out, err)
	}
	// make List flexlist
	out, err = MakeNodeHandler([]Value{NewTypeLiteral(TList), NewFlexList([]Value{NewInteger(1)})}, nil, nil, r)
	if err != nil || len(out) != 1 || IsFlexNode(out[0]) {
		t.Errorf("make List: out=%v err=%v", out, err)
	}
	// make FlexXml xml / make Xml flexxml
	out, err = MakeNodeHandler([]Value{NewTypeLiteral(TFlexXml), NewXmlElement("p", nil, nil)}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsFlexNode(out[0]) {
		t.Errorf("make FlexXml: out=%v err=%v", out, err)
	}
	out, err = MakeNodeHandler([]Value{NewTypeLiteral(TXml), out[0]}, nil, nil, r)
	if err != nil || len(out) != 1 || IsFlexNode(out[0]) {
		t.Errorf("make Xml: out=%v err=%v", out, err)
	}
}

func TestMakeNodeHandlerRejectsWrongSources(t *testing.T) {
	r := newTestRegistry(t)
	cases := []struct {
		target *Type
		src    Value
		want   string
	}{
		{TFlexMap, NewInteger(1), "FlexMap source"},
		{TFlexList, NewInteger(1), "FlexList source"},
		{TMap, NewList(nil), "Map source"},
		{TList, flexSampleMap(), "List source"},
		{TFlexXml, NewInteger(1), "FlexXml source"},
		{TXml, NewInteger(1), "Xml source"},
	}
	for _, c := range cases {
		_, err := MakeNodeHandler([]Value{NewTypeLiteral(c.target), c.src}, nil, nil, r)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("make %s of %s: err = %v, want %q", c.target, c.src, err, c.want)
		}
	}

	// A non-bare target defers to the generic MakeHandler rather than
	// hijacking; whatever it does, it must not panic.
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("non-bare target panicked: %v", rec)
			}
		}()
		_, _ = MakeNodeHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r)
	}()

	// A Node target this handler doesn't own falls to the default branch
	// (make String 5 → conversion via the generic dispatcher).
	out, err := MakeNodeHandler([]Value{NewTypeLiteral(TString), NewInteger(5)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("default-branch make: out=%v err=%v", out, err)
	}
	if s, _ := AsString(out[0]); s != "5" {
		t.Errorf("make String 5 = %v", out[0])
	}
}

// --- MakeScalarHandler / builtinBaseOf ---------------------------------------

func TestMakeScalarHandlerConversions(t *testing.T) {
	r := newTestRegistry(t)
	out, err := MakeScalarHandler([]Value{NewTypeLiteral(TString), NewInteger(5)}, nil, nil, r)
	if err != nil {
		t.Fatalf("make String 5: %v", err)
	}
	if s, _ := AsString(out[0]); s != "5" {
		t.Errorf("make String 5 = %v", out[0])
	}
	// Source already conforms: identity.
	out, err = MakeScalarHandler([]Value{NewTypeLiteral(TInteger), NewInteger(7)}, nil, nil, r)
	if err != nil {
		t.Fatalf("make Integer 7: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 7 {
		t.Errorf("make Integer 7 = %v", out[0])
	}
	// Target must be a type literal.
	if _, err := MakeScalarHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected a type literal") {
		t.Errorf("concrete target: err = %v", err)
	}
}

func TestMakeScalarHandlerRefinementNewtype(t *testing.T) {
	r := newTestRegistry(t)
	foo := r.Types.MintType("Foo", TInteger)
	// `make Foo 1` reparents to the refinement.
	out, err := MakeScalarHandler([]Value{NewTypeLiteral(foo), NewInteger(1)}, nil, nil, r)
	if err != nil {
		t.Fatalf("make Foo 1: %v", err)
	}
	if !ParentOf(&out[0]).Equal(foo) {
		t.Errorf("make Foo 1 parent = %v, want Foo", ParentOf(&out[0]))
	}
	// A non-conforming source converts through the builtin base first.
	out, err = MakeScalarHandler([]Value{NewTypeLiteral(foo), NewString("42")}, nil, nil, r)
	if err != nil {
		t.Fatalf("make Foo '42': %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 42 || !ParentOf(&out[0]).Equal(foo) {
		t.Errorf("make Foo '42' = %v (parent %v)", out[0], ParentOf(&out[0]))
	}

	// builtinBaseOf: walks to the first builtin ancestor, nil when none.
	if base := builtinBaseOf(foo); base == nil || !base.Equal(TInteger) {
		t.Errorf("builtinBaseOf(Foo) = %v", base)
	}
	orphan := r.Types.MintType("Orphan", nil)
	if base := builtinBaseOf(orphan); base != nil {
		t.Errorf("builtinBaseOf(Orphan) = %v, want nil", base)
	}
	// A rootless user type cannot convert: unsupported-target error.
	if _, err := MakeScalarHandler([]Value{NewTypeLiteral(orphan), NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("make Orphan 1 succeeded")
	}
}

// --- MakeWithOpts / parseMakeOptions ------------------------------------------

func TestMakeWithOptsRecordBase(t *testing.T) {
	r := newTestRegistry(t)
	rec := NewRecordType(intStrRecord().Fields)
	optsT := NewOrderedMap()
	optsT.Set("base", NewBoolean(true))
	// base:true fills missing fields with type zero values.
	out, err := MakeWithOpts([]Value{rec, mapOf("a", NewInteger(1)), NewMap(optsT)}, nil, nil, r)
	if err != nil {
		t.Fatalf("MakeWithOpts base: %v", err)
	}
	m, err := AsMap(out[0])
	if err != nil {
		t.Fatalf("result not a map: %v", err)
	}
	b, ok := m.Get("b")
	if !ok {
		t.Fatal("field b not defaulted")
	}
	if s, _ := AsString(b); s != "" {
		t.Errorf("defaulted b = %v, want empty string", b)
	}

	// parseMakeOptions negative: a non-map options value.
	if _, err := MakeWithOpts([]Value{rec, mapOf("a", NewInteger(1)), NewInteger(3)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "options must be a map") {
		t.Errorf("non-map opts: err = %v", err)
	}
}

func TestMakeWithOptsClassAndPathon(t *testing.T) {
	r := newTestRegistry(t)

	// Class target routes through makeObject.
	fields := NewOrderedMap()
	fields.Set("n", NewTypeLiteral(TInteger))
	ct := r.Types.MintType("Class/Pt", TClass)
	cls := NewClassType(ct, ClassTypeInfo{Fields: fields, Name: "Pt", Type: ct})
	out, err := MakeWithOpts([]Value{cls, mapOf("n", NewInteger(3)), NewMap(NewOrderedMap())}, nil, nil, r)
	if err != nil {
		t.Fatalf("MakeWithOpts class: %v", err)
	}
	if len(out) != 1 || !IsFlatInstance(out[0]) {
		t.Errorf("class make result = %v", out)
	}

	// Pathon target honours the abs option.
	po := NewOrderedMap()
	po.Set("abs", NewBoolean(true))
	out, err = MakeWithOpts([]Value{NewTypeLiteral(TPathon), NewString("a/b"), NewMap(po)}, nil, nil, r)
	if err != nil {
		t.Fatalf("MakeWithOpts pathon: %v", err)
	}
	pi, err := AsPathon(out[0])
	if err != nil {
		t.Fatalf("pathon result: %v", err)
	}
	if !pi.Abs {
		t.Errorf("abs option ignored: %+v", pi)
	}
}

// --- MakeObjHandler ------------------------------------------------------------

func TestMakeObjHandler(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("n", NewTypeLiteral(TInteger))
	ct := r.Types.MintType("Class/Obj", TClass)
	cls := NewClassType(ct, ClassTypeInfo{Fields: fields, Name: "Obj", Type: ct})

	out, err := MakeObjHandler([]Value{cls, mapOf("n", NewInteger(9))}, nil, nil, r)
	if err != nil {
		t.Fatalf("MakeObjHandler: %v", err)
	}
	flds, ok := FlatInstanceFields(out[0])
	if !ok {
		t.Fatalf("result not a flat instance: %v", out[0])
	}
	nv, _ := flds.Get("n")
	if n, _ := AsInteger(nv); n != 9 {
		t.Errorf("field n = %v", nv)
	}

	// Unknown field / missing field are loud.
	if _, err := MakeObjHandler([]Value{cls, mapOf("bogus", NewInteger(1))}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Errorf("unknown field: err = %v", err)
	}
	if _, err := MakeObjHandler([]Value{cls, mapOf()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "missing field") {
		t.Errorf("missing field: err = %v", err)
	}
	// A non-map source is rejected.
	if _, err := MakeObjHandler([]Value{cls, NewInteger(1)}, nil, nil, r); err == nil {
		t.Error("non-map class source succeeded")
	}
}

// --- MakeScalarOptsHandler -----------------------------------------------------

func TestMakeScalarOptsHandler(t *testing.T) {
	po := NewOrderedMap()
	po.Set("abs", NewBoolean(true))
	out, err := MakeScalarOptsHandler([]Value{NewTypeLiteral(TPathon), NewMap(po), NewString("x/y")}, nil, nil, nil)
	if err != nil {
		t.Fatalf("MakeScalarOptsHandler pathon: %v", err)
	}
	pi, err := AsPathon(out[0])
	if err != nil || !pi.Abs {
		t.Errorf("pathon opts: %v %v", pi, err)
	}
	// Non-Pathon target ignores the opts and delegates to the scalar path.
	out, err = MakeScalarOptsHandler([]Value{NewTypeLiteral(TString), NewMap(NewOrderedMap()), NewInteger(3)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("MakeScalarOptsHandler string: %v", err)
	}
	if s, _ := AsString(out[0]); s != "3" {
		t.Errorf("scalar-opts make = %v", out[0])
	}
}

// --- ResolveFieldType -----------------------------------------------------------

func TestResolveFieldType(t *testing.T) {
	r := covRegistry(t, nil)

	// A capitalised name bound to a type body resolves to the body.
	if err := InstallType(r, "FooT", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	got := ResolveFieldType(r, NewString("FooT"))
	if !IsBareTypeNode(&got) {
		t.Errorf("FooT did not resolve to a type body: %v", got)
	}

	// A lowercase string that happens to spell a word name stays a string.
	got = ResolveFieldType(r, NewString("cadd"))
	if s, _ := AsString(got); s != "cadd" {
		t.Errorf("lowercase name resolved: %v", got)
	}

	// An unbound capitalised name passes through.
	got = ResolveFieldType(r, NewString("NoSuchType"))
	if s, _ := AsString(got); s != "NoSuchType" {
		t.Errorf("unbound name changed: %v", got)
	}

	// A concrete list is evaluated as code (word-name strings become words).
	got = ResolveFieldType(r, NewList([]Value{NewString("cadd"), NewInteger(1), NewInteger(2)}))
	if n, _ := AsInteger(got); n != 3 {
		t.Errorf("list field type did not evaluate: %v", got)
	}

	// A list that fails to evaluate passes through unchanged.
	bad := NewList([]Value{NewWord("zzz_not_defined")})
	got = ResolveFieldType(r, bad)
	if !ExactEqual(got, bad) {
		t.Errorf("failing list changed: %v", got)
	}

	// Everything else passes through.
	got = ResolveFieldType(r, NewInteger(7))
	if n, _ := AsInteger(got); n != 7 {
		t.Errorf("scalar changed: %v", got)
	}
}
