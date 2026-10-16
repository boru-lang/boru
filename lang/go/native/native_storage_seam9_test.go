package native

import "testing"

// Coverage for the error/edge arms of the storage words (`set` / `get` /
// `context`) and their check-mode ReturnsFn narrowers in
// native_storage.go. The container handlers are driven directly with
// crafted args (wrong-type containers, type-literals) so each reject arm
// fires; the ReturnsFn narrowers are pure functions called directly with
// crafted carriers.

// w9Top runs src and returns the top-of-stack value.
func w9Top(t *testing.T, src string) Value {
	t.Helper()
	out, err := w9Run(t, w9Reg(t), src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if len(out) == 0 {
		t.Fatalf("run %q: empty stack", src)
	}
	return out[len(out)-1]
}

// w9XmlTaggedInt is a malformed value: an Integer payload tagged as Xml.
// AdoptIntoFlex routes it to the Xml branch, where FlexDeepCopy fails
// (xmlParts can't read an IntPayload) — exercising the adopt error arm.
func w9XmlTaggedInt() Value {
	v := NewInteger(7)
	v.SetParent(TXml)
	return v
}

func TestW9SetContainerRejects(t *testing.T) {
	r := w9Reg(t)

	// setMapHandler: non-concrete map receiver.
	_, err := setMapHandler([]Value{NewString("k"), NewInteger(1), NewTypeLiteral(TMap)}, nil, nil, r)
	w9wantErr(t, err, "")

	// setClassInstanceHandler: type literal receiver.
	_, err = setClassInstanceHandler([]Value{NewString("f"), NewInteger(1), NewTypeLiteral(TClass)}, nil, nil, r)
	w9wantErr(t, err, "type literal")

	// setClassInstanceHandler: concrete non-class receiver.
	_, err = setClassInstanceHandler([]Value{NewString("f"), NewInteger(1), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "class instance")

	// setClassInstanceHandler: undeclared field on a nameless class → the
	// container-name fallback fires, then sealed_field.
	ct := &ClassTypeInfo{Name: "", Fields: NewOrderedMap()}
	inst := ClassInstanceInfo{TypeRef: ct, Fields: NewOrderedMap()}
	_, err = setClassInstanceHandler([]Value{NewString("undeclared"), NewInteger(1), NewClassInstance(TClass, inst)}, nil, nil, r)
	w9wantErr(t, err, "is not a field")

	// setFlexMapHandler: non-FlexMap receiver, then adopt failure.
	_, err = setFlexMapHandler([]Value{NewString("k"), NewInteger(1), NewTypeLiteral(TFlexMap)}, nil, nil, r)
	w9wantErr(t, err, "expected a FlexMap")
	fm := w9Top(t, "flex {a:1}")
	_, err = setFlexMapHandler([]Value{NewString("k"), w9XmlTaggedInt(), fm}, nil, nil, r)
	w9wantErr(t, err, "flex")

	// setFlexListHandler: non-FlexList receiver, then adopt failure.
	_, err = setFlexListHandler([]Value{NewInteger(0), NewInteger(1), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a FlexList")
	fl := w9Top(t, "flex [1 2 3]")
	_, err = setFlexListHandler([]Value{NewInteger(0), w9XmlTaggedInt(), fl}, nil, nil, r)
	w9wantErr(t, err, "flex")

	// setFlexXmlHandler: non-FlexXml receiver, then a non-String value is
	// coerced to String (the val.Is(TString) arm).
	_, err = setFlexXmlHandler([]Value{NewString("a"), NewInteger(1), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a FlexXml")
	fx := w9Top(t, "flex <a>hi</a>")
	out, xerr := setFlexXmlHandler([]Value{NewString("count"), NewInteger(99), fx}, nil, nil, r)
	if xerr != nil {
		t.Fatalf("setFlexXml coerce: %v", xerr)
	}
	if len(out) != 1 {
		t.Fatalf("setFlexXml coerce: expected receiver back, got %v", out)
	}

	// setListHandler: non-integer index, then non-concrete list.
	_, err = setListHandler([]Value{NewTypeLiteral(TInteger), NewInteger(1), w9List(NewInteger(0))}, nil, nil, r)
	w9wantErr(t, err, "concrete Integer index")
	_, err = setListHandler([]Value{NewInteger(0), NewInteger(1), NewTypeLiteral(TList)}, nil, nil, r)
	w9wantErr(t, err, "")

	// setStoreHandler: non-Store receiver.
	_, err = setStoreHandler([]Value{NewString("k"), NewInteger(1), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a Store")
}

func TestW9GetContainerRejects(t *testing.T) {
	r := w9Reg(t)

	// getXmlHandler: non-Xml receiver, then an unknown key reads None.
	_, err := getXmlHandler([]Value{NewString("tag"), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "non-Xml")
	xml := w9Top(t, "<a>hi</a>")
	out, xerr := getXmlHandler([]Value{NewString("bogus"), xml}, nil, nil, r)
	if xerr != nil || len(out) != 1 || !out[0].Is(TNone) {
		t.Fatalf("getXml unknown key: out=%v err=%v", out, xerr)
	}

	// getNodeHandler: type-literal receiver.
	_, err = getNodeHandler([]Value{NewString("a"), NewTypeLiteral(TMap)}, nil, nil, r)
	w9wantErr(t, err, "type literal")

	// getObjectHandler: type-literal receiver.
	_, err = getObjectHandler([]Value{NewString("a"), NewTypeLiteral(TClass)}, nil, nil, r)
	w9wantErr(t, err, "type literal")

	// getObjectHandler via the mutable-map branch: present key returns the
	// value, absent key reads None.
	m := w9Map("a", NewInteger(1))
	out, oerr := getObjectHandler([]Value{NewString("a"), m}, nil, nil, r)
	if oerr != nil || len(out) != 1 {
		t.Fatalf("getObject present: out=%v err=%v", out, oerr)
	}
	if n, _ := AsInteger(out[0]); n != 1 {
		t.Fatalf("getObject present: a = %d, want 1", n)
	}
	out, oerr = getObjectHandler([]Value{NewString("zzz"), m}, nil, nil, r)
	if oerr != nil || len(out) != 1 || !out[0].Is(TNone) {
		t.Fatalf("getObject absent: out=%v err=%v", out, oerr)
	}

	// getObjectHandler via the Resource branch: missing field reads None.
	ri := NewValueRaw(TResource, ResourceInstanceInfo{Fields: NewOrderedMap()})
	out, oerr = getObjectHandler([]Value{NewString("nope"), ri}, nil, nil, r)
	if oerr != nil || len(out) != 1 || !out[0].Is(TNone) {
		t.Fatalf("getObject resource absent: out=%v err=%v", out, oerr)
	}

	// getStoreHandler: non-Store receiver.
	_, err = getStoreHandler([]Value{NewString("k"), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "expected a Store")
}

func TestW9ContextArms(t *testing.T) {
	r := w9Reg(t)
	for r.Contexts.Top() != nil {
		r.Contexts.Pop()
	}

	// contextHandler with no active context errors.
	_, err := contextHandler(nil, nil, nil, r)
	w9wantErr(t, err, "no active context")

	// contextReturns with no context (and not compiling) yields a bare
	// Store carrier.
	out := contextReturns(nil, r)
	if len(out) != 1 || !out[0].Is(TStore) {
		t.Fatalf("contextReturns nil-context: %v", out)
	}

	// getStoreReturnsFn with too few args yields dynamic(Any).
	out = getStoreReturnsFn([]Value{NewString("k")}, r)
	if len(out) != 1 {
		t.Fatalf("getStoreReturnsFn short args: %v", out)
	}
}

func TestW9RecordSchemaFieldReturns(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("a", NewInteger(1))
	fields.Set("f", NewCarrier(TFunction))
	rt := RecordTypeInfo{Fields: fields}

	// Absent key → dynamic(Any).
	if out := recordSchemaFieldReturns(rt, NewString("z")); len(out) != 1 {
		t.Fatalf("absent key: %v", out)
	}
	// Function-typed field → dynamic(Any).
	if out := recordSchemaFieldReturns(rt, NewString("f")); len(out) != 1 {
		t.Fatalf("function field: %v", out)
	}
}

func TestW9GetNodeReturnsArms(t *testing.T) {
	r := w9Reg(t)

	// Wrong arity → dynamic.
	if out := getNodeReturns([]Value{NewString("k")}, r); len(out) != 1 {
		t.Fatalf("arity: %v", out)
	}

	// Carrier of a minted record type → schema field lookup.
	recType := NewRecordType(func() *OrderedMap {
		om := NewOrderedMap()
		om.Set("a", NewInteger(1))
		return om
	}())
	carrier := NewCarrier(&recType)
	if out := getNodeReturns([]Value{NewString("a"), carrier}, r); len(out) != 1 {
		t.Fatalf("record carrier: %v", out)
	}

	// Concrete Options receiver conforms to Map but AsMap fails → dynamic.
	if out := getNodeReturns([]Value{NewString("k"), NewOptionsType(NewOrderedMap())}, r); len(out) != 1 {
		t.Fatalf("options: %v", out)
	}

	// Function-typed field with r==nil → dynamic (the non-noted arm).
	// A carrier of TFunction has Parent conforming to TFunction (a bare
	// type literal does not — its Parent is the meta-type).
	fnMap := w9Map("f", NewCarrier(TFunction))
	if out := getNodeReturns([]Value{NewString("f"), fnMap}, nil); len(out) != 1 {
		t.Fatalf("function field r==nil: %v", out)
	}
}

func TestW9GetIntKeyReturnsArms(t *testing.T) {
	r := w9Reg(t)

	// Non-integer key over a concrete list → dynamic (AsInteger fails).
	if out := getIntKeyReturns([]Value{NewString("x"), w9List(NewInteger(1))}, r); len(out) != 1 {
		t.Fatalf("non-int key: %v", out)
	}

	// List-conforming but non-list payload (a TableType) → dynamic.
	tbl := NewTableType(RecordTypeInfo{Fields: NewOrderedMap()})
	if out := getIntKeyReturns([]Value{NewInteger(0), tbl}, r); len(out) != 1 {
		t.Fatalf("table-type: %v", out)
	}

	// Dispatch-bearing element (Function carrier) → dynamic.
	if out := getIntKeyReturns([]Value{NewInteger(0), w9List(NewCarrier(TFunction))}, r); len(out) != 1 {
		t.Fatalf("function element: %v", out)
	}
}

func TestW9GetObjectResourceReturnsArms(t *testing.T) {
	r := w9Reg(t)

	// getObjectReturns guard: wrong arity.
	if out := getObjectReturns([]Value{NewString("k")}, r); len(out) != 1 {
		t.Fatalf("getObjectReturns arity: %v", out)
	}

	// getObjectReturns over a non-class type binding → dynamic (AsClassType
	// fails on the refine body).
	if _, err := w9Run(t, r, "def Foo refine Integer"); err != nil {
		t.Fatalf("def Foo: %v", err)
	}
	fooCarrier := NewCarrier(r.LookupTypeName("Foo"))
	if out := getObjectReturns([]Value{NewString("x"), fooCarrier}, r); len(out) != 1 {
		t.Fatalf("non-class body: %v", out)
	}

	// getResourceReturns guard: wrong arity.
	if out := getResourceReturns([]Value{NewString("k")}, r); len(out) != 1 {
		t.Fatalf("getResourceReturns arity: %v", out)
	}

	// getResourceReturns: no resource type bound for the carrier's name.
	if out := getResourceReturns([]Value{NewString("x"), NewCarrier(TFloat)}, r); len(out) != 1 {
		t.Fatalf("no resource binding: %v", out)
	}

	// getResourceReturns: a resource type whose field is Function-typed →
	// dynamic. Bind the resource type under the carrier type's leaf name.
	rr := w9Reg(t)
	fom := NewOrderedMap()
	fom.Set("f", NewCarrier(TFunction))
	rr.Defs.Push("Integer", NewValueRaw(TResource, ResourceTypeInfo{Fields: fom}))
	if out := getResourceReturns([]Value{NewString("f"), NewCarrier(TInteger)}, rr); len(out) != 1 {
		t.Fatalf("resource function field: %v", out)
	}
	// A key not in the resource schema reads None.
	if out := getResourceReturns([]Value{NewString("missing"), NewCarrier(TInteger)}, rr); len(out) != 1 {
		t.Fatalf("resource absent field: %v", out)
	}
}

func TestW9IsClosureBearingWrapper(t *testing.T) {
	r := w9Reg(t)
	// FnDefInfo whose registry does not know the name → not closure-bearing.
	fd := FnDefInfo{Registry: r, Name: "no_such_native_xyz"}
	if isClosureBearingWrapper(NewValueRaw(TFunction, &fd)) {
		t.Fatalf("expected false for an unresolvable wrapper")
	}
	// Non-FnDef value → false.
	if isClosureBearingWrapper(NewInteger(1)) {
		t.Fatalf("expected false for a non-FnDef value")
	}
}
