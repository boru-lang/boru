package core

import "testing"

// isTypeBodyLegacy is the 18-arm shape enumeration IsTypeBody carried
// before the payload seam (design/legacy/TYPE-REPRESENTATION.1.ignore §N4),
// preserved verbatim as the equivalence ORACLE: the seam must answer
// exactly as the enumeration did, for every payload kind.
func isTypeBodyLegacy(v Value) bool {
	if IsBareTypeNode(v) {
		return true
	}
	if IsImplicitMap(v) {
		return true
	}
	if IsRecordType(v) {
		return true
	}
	if IsOptionsType(v) {
		return true
	}
	if IsTableType(v) {
		return true
	}
	if IsDisjunct(v) {
		return true
	}
	if IsNegation(v) {
		return true
	}
	if IsTypedList(v) {
		return true
	}
	if IsTypedMap(v) {
		return true
	}
	if IsBoundedType(v) {
		return true
	}
	if IsClassType(v) {
		return true
	}
	if IsSurfaceType(v) {
		return true
	}
	if IsTypeSchema(v) {
		return true
	}
	if v.IsDepScalar() {
		return true
	}
	if v.Parent.Equal(TFnUndef) {
		return true
	}
	if v.Parent.Equal(TFunction) {
		return true
	}
	if IsMicronType(v) {
		return true
	}
	if IsHostTypeBody(v) {
		return true
	}
	return false
}

type ptcHostBody struct{}

func (ptcHostBody) hostTypeBody() {}

// TestIsTypeContentMirrorsLegacy runs a value of every payload kind —
// type content and ordinary values alike, plus the owner-guard edge
// cases (shared variants at the wrong identity, carriers at the
// fn-shape identities) — through both the seam and the oracle.
func TestIsTypeContentMirrorsLegacy(t *testing.T) {
	implicit := NewOrderedMap()
	implicit.Set("x", NewTypeLiteral(TInteger))
	implicit.Implicit = true
	plainMap := NewOrderedMap()
	plainMap.Set("x", NewInteger(1))

	disjunct := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewNone()})
	misparentedDisjunct := disjunct
	misparentedDisjunct.Parent = TList // shared variant at the wrong identity

	typedList := NewValueRaw(TList, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	typedMap := NewValueRaw(TMap, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	bounded := NewValueRaw(TType, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	strayChild := NewValueRaw(TInteger, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})

	fnShapeCarrier := NewCarrier(TFnUndef)
	fnCarrier := NewCarrier(TFunction)

	cases := []struct {
		name string
		v    Value
	}{
		{"bare node", NewTypeLiteral(TInteger)},
		{"integer", NewInteger(5)},
		{"string", NewString("s")},
		{"boolean", NewBoolean(true)},
		{"none", NewNone()},
		{"list", NewList([]Value{NewInteger(1)})},
		{"plain map", NewMap(plainMap)},
		{"implicit map (record shape)", NewImplicitMap(implicit)},
		{"disjunct", disjunct},
		{"misparented disjunct payload", misparentedDisjunct},
		{"negation", NewValueRaw(TNegation, NegationInfo{Inner: NewTypeLiteral(TString)})},
		{"typed list", typedList},
		{"typed map", typedMap},
		{"bounded type", bounded},
		{"stray ChildTypeInfo", strayChild},
		{"dep scalar", NewDepScalar(DepGT, NewInteger(10))},
		{"word", NewWord("w")},
		{"atom", NewAtom("a")},
		{"fn-shape carrier", fnShapeCarrier},
		{"function carrier", fnCarrier},
		{"host type body", NewExtension(TScalar, ptcHostBody{})},
		{"host instance", NewExtension(TScalar, struct{}{})},
	}
	for _, c := range cases {
		if got, want := IsTypeBody(c.v), isTypeBodyLegacy(c.v); got != want {
			t.Errorf("%s: IsTypeBody=%v, legacy oracle=%v", c.name, got, want)
		}
	}
}

// The constant-false payload kinds answer false regardless of owner —
// exercised directly so the seam's inventory stays fully covered.
func TestIsTypeContentOrdinaryPayloads(t *testing.T) {
	ordinary := []Payload{
		ReachInfo{}, IntPayload{}, FloatPayload{}, BigIntPayload{},
		DecimalPayload{}, StrPayload{}, BoolPayload{}, AtomPayload{},
		PathonPayload{}, MicronPayload{}, ListPayload{}, ParenExprPayload{},
		InterpStringPayload{}, TimePayload{}, DurationPayload{},
		TimezonePayload{}, NonePayload{}, WordInfo{}, ForwardInfo{},
		MarkInfo{}, MoveInfo{}, SpliceInfo{}, SugarInfo{},
		ReturnCheckInfo{}, DefCleanupInfo{}, FrameOpenInfo{},
		ModuleDesc{}, ClassInstanceInfo{}, DispatchModInfo{},
		ErrorInfo{}, GenInstRef{}, GenParam{}, PathonInfo{},
		PayloadBase{}, ResourceInstanceInfo{}, ResourceTypeInfo{},
		XmlElementPayload{}, XmlInterpPayload{}, CalDurationData{},
		noneSentinel{}, &FlexListData{}, &FlexXmlData{}, &GenSpecInfo{},
		&IntervalInfo{}, &StoreInstanceInfo{}, &TimeoutInfo{},
		&WeakFlexListData{}, &WeakFlexMapData{}, &WeakFlexXmlData{},
		MaterializerPayload{}, TableData{},
	}
	owner := NewInteger(1)
	for i, p := range ordinary {
		if p.IsTypeContent(&owner) {
			t.Errorf("ordinary payload %d (%T) must not be type content off its identity", i, p)
		}
	}
	// The always-true kinds, for the same inventory reason.
	content := []Payload{
		ClassTypeInfo{}, &SurfaceInfo{}, &TypeSchemaInfo{},
		DepScalarInfo{}, MicronTypeInfo{},
	}
	for i, p := range content {
		if !p.IsTypeContent(nil) {
			t.Errorf("content payload %d (%T) must answer true", i, p)
		}
	}
	// Owner-guard edges the battery above does not construct: nil
	// owners on the guarded kinds.
	guarded := []Payload{
		DisjunctInfo{}, NegationInfo{}, ChildTypeInfo{}, RecordTypeInfo{},
		OptionsTypeInfo{}, TableTypeInfo{}, TableData{},
		MaterializerPayload{}, FnUndefInfo{}, FnDefInfo{}, MapPayload{},
	}
	for i, p := range guarded {
		if p.IsTypeContent(nil) {
			t.Errorf("guarded payload %d (%T) must answer false with no owner", i, p)
		}
	}
}

// TypeBody: the node-side structure recovery stamped by InstallType.
func TestTypeBodyStamp(t *testing.T) {
	r := newTestRegistry(t)
	// A structural binding records its content on the node.
	if err := InstallType(r, "PtcOne", NewInteger(1)); err != nil {
		t.Fatalf("install: %v", err)
	}
	node := r.LookupTypeName("PtcOne")
	body, ok := node.TypeBody()
	if !ok {
		t.Fatal("a structural binding must stamp its content")
	}
	if got, _ := AsInteger(body); got != 1 {
		t.Errorf("recovered content must be the declared body, got %v", body)
	}
	// A bare refine newtype has no structure — nothing recorded.
	prefab := r.Types.MintRefinePrefab(TInteger)
	if err := InstallType(r, "PtcNom", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("refine install: %v", err)
	}
	if _, ok := r.LookupTypeName("PtcNom").TypeBody(); ok {
		t.Error("a bare refine records no content")
	}
	// An adopted alias never stamps the adopted node.
	if err := InstallType(r, "PtcAlias", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("alias install: %v", err)
	}
	if _, ok := TInteger.TypeBody(); ok {
		t.Error("an alias must not stamp the canonical aliased node")
	}
	// An ordinary value answers false.
	five := NewInteger(5)
	if _, ok := five.TypeBody(); ok {
		t.Error("an ordinary value has no type body")
	}
	// A disjunct binding's alternatives are recoverable from the node.
	if err := InstallType(r, "PtcMaybe",
		NewDisjunct([]Value{NewTypeLiteral(TInteger), NewNone()})); err != nil {
		t.Fatalf("disjunct install: %v", err)
	}
	mb, ok := r.LookupTypeName("PtcMaybe").TypeBody()
	if !ok || !IsDisjunct(mb) {
		t.Errorf("the disjunct body must be recoverable from the node, got %v ok=%v", mb, ok)
	}
}
