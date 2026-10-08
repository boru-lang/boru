package core

// Stage-5 coverage tests for value.go: ReadList/OrderedMap accessors,
// FnDefInfo signature views, scalar formatters, constructor variants,
// the interp/xml source renderers, accessor error arms, and the
// kernelFormatDefault switch arms previously unreached. White-box
// (package core) with directly crafted, seal-legitimate Values built
// through the public constructors. Deterministic; the one global
// touched (micronRenderBridge) is restored via defer.

import (
	"errors"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// --- ReadList / OrderedMap ------------------------------------------------

func TestS5VReadListAndSortedKeys(t *testing.T) {
	rl := NewReadList([]Value{NewInteger(10), NewInteger(20)})
	if rl.Len() != 2 {
		t.Fatalf("NewReadList Len = %d, want 2", rl.Len())
	}
	if v, ok := rl.GetOk(1); !ok {
		t.Error("GetOk(1) must succeed")
	} else if n, _ := AsInteger(v); n != 20 {
		t.Errorf("GetOk(1) = %v, want 20", v)
	}
	if _, ok := rl.GetOk(-1); ok {
		t.Error("GetOk(-1) must fail")
	}
	if _, ok := rl.GetOk(2); ok {
		t.Error("GetOk(2) must fail")
	}
	om := NewOrderedMap()
	om.Set("b", NewInteger(1))
	om.Set("a", NewInteger(2))
	sorted := om.SortedKeys()
	if len(sorted) != 2 || sorted[0] != "a" || sorted[1] != "b" {
		t.Errorf("SortedKeys = %v, want [a b]", sorted)
	}
	// Keys stays insertion-ordered (contrast with SortedKeys).
	keys := om.Keys()
	if keys[0] != "b" || keys[1] != "a" {
		t.Errorf("Keys = %v, want [b a]", keys)
	}
}

// --- CompileEffect.Has ----------------------------------------------------

func TestS5VCompileEffectHas(t *testing.T) {
	if !CompileReadsFn.Has(CompileReadsFn) {
		t.Error("effect must include itself")
	}
	if CompileReadsFn.Has(CompileStoresFn) {
		t.Error("effect must not include an unset flag")
	}
}

// --- FnDefInfo signature views --------------------------------------------

func TestS5VOwnSigsFallbackFilter(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{
		{Fallback: true},
		{Params: []FnParam{{Type: TInteger}}},
	}}
	own := fn.OwnSigs()
	if len(own) != 1 || own[0].Fallback {
		t.Errorf("OwnSigs must filter the fallback sig, got %d sigs", len(own))
	}
	onlyFallback := &FnDefInfo{Signatures: []Signature{{Fallback: true}}}
	if got := onlyFallback.OwnSigs(); len(got) != 0 {
		t.Errorf("OwnSigs(only fallback) = %d sigs, want 0", len(got))
	}
}

func TestS5VReturnPatternInRange(t *testing.T) {
	pat := NewTypeLiteral(TInteger)
	rc := ReturnCheckInfo{ReturnPatterns: []*Value{&pat}}
	if got := rc.ReturnPattern(0); got == nil || !IsBareTypeNode(*got) {
		t.Errorf("ReturnPattern(0) = %v, want the stored pattern", got)
	}
	if rc.ReturnPattern(1) != nil {
		t.Error("ReturnPattern(1) must be nil for a short slice")
	}
}

// --- AllFields inheritance / Store tombstone ------------------------------

func TestS5VClassAllFieldsParentChain(t *testing.T) {
	pf := NewOrderedMap()
	pf.Set("a", NewTypeLiteral(TInteger))
	parent := &ClassTypeInfo{Fields: pf}
	cf := NewOrderedMap()
	cf.Set("b", NewTypeLiteral(TString))
	child := &ClassTypeInfo{Fields: cf, Parent: parent}
	all := child.AllFields()
	if got := all.Keys(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("class AllFields keys = %v, want [a b]", got)
	}
}

func TestS5VResourceAllFieldsParentAndCache(t *testing.T) {
	pf := NewOrderedMap()
	pf.Set("id", NewTypeLiteral(TString))
	parent := &ResourceTypeInfo{Fields: pf}
	cf := NewOrderedMap()
	cf.Set("n", NewTypeLiteral(TInteger))
	child := &ResourceTypeInfo{Fields: cf, Parent: parent}
	all := child.AllFields()
	if got := all.Keys(); len(got) != 2 || got[0] != "id" || got[1] != "n" {
		t.Errorf("resource AllFields keys = %v, want [id n]", got)
	}
	if again := child.AllFields(); again != all {
		t.Error("second AllFields call must return the cached map")
	}
}

func TestS5VStoreGetTombstone(t *testing.T) {
	proto := &StoreInstanceInfo{Data: map[string]Value{"k": NewInteger(1)}}
	si := &StoreInstanceInfo{
		Deleted:   map[string]bool{"k": true},
		Prototype: proto,
	}
	if _, ok := si.Get("k"); ok {
		t.Error("tombstoned key must report absent despite the prototype binding")
	}
}

func TestS5VGenerateObjectTypeID(t *testing.T) {
	id := GenerateObjectTypeID()
	if !strings.HasPrefix(id, "T_") || len(id) != 14 {
		t.Errorf("GenerateObjectTypeID = %q, want T_ + 12 hex chars", id)
	}
}

// --- elem constraint / ascription facets ----------------------------------

func TestS5VElemConstraintClear(t *testing.T) {
	v := NewList([]Value{NewInteger(1)})
	v.SetElemConstraint(NewTypeLiteral(TInteger))
	if _, ok := v.ElemConstraint(); !ok {
		t.Fatal("element constraint must be attached")
	}
	v.SetElemConstraint(Value{}) // zero Value clears
	if _, ok := v.ElemConstraint(); ok {
		t.Error("zero-Value constraint must clear the tag")
	}
	v.SetElemConstraint(NewTypeLiteral(TInteger))
	v.SetElemConstraint(NewTypeLiteral(TAny)) // bare Any clears too
	if _, ok := v.ElemConstraint(); ok {
		t.Error("bare-Any constraint must clear the tag")
	}
}

func TestS5VAscribedSetAndStrip(t *testing.T) {
	v := NewInteger(1)
	v.SetAscribed(TNumber)
	if v.AscribedType() != TNumber {
		t.Fatal("SetAscribed must attach the ascription")
	}
	stripped := StripAscribed(v)
	if stripped.AscribedType() != nil {
		t.Error("StripAscribed must clear the ascription")
	}
	plain := StripAscribed(NewInteger(2))
	if plain.AscribedType() != nil {
		t.Error("StripAscribed on a plain value stays nil")
	}
}

// --- scalar formatters ----------------------------------------------------

func TestS5VFormatFloatSpecialsAndExtremes(t *testing.T) {
	if got := FormatFloat(math.NaN()); got != "nan" {
		t.Errorf("NaN = %q", got)
	}
	if got := FormatFloat(math.Inf(1)); got != "inf" {
		t.Errorf("+Inf = %q", got)
	}
	if got := FormatFloat(math.Inf(-1)); got != "-inf" {
		t.Errorf("-Inf = %q", got)
	}
	if got := FormatFloat(1e21); got != "1e+21" {
		t.Errorf("1e21 = %q, want 1e+21", got)
	}
	if got := FormatFloat(1e-11); got != "1e-11" {
		t.Errorf("1e-11 = %q, want 1e-11", got)
	}
}

func TestS5VFormatBigNonNil(t *testing.T) {
	if got := FormatBigInteger(big.NewInt(-5)); got != "-0d5" {
		t.Errorf("FormatBigInteger(-5) = %q", got)
	}
	if got := FormatBigInteger(big.NewInt(7)); got != "0d7" {
		t.Errorf("FormatBigInteger(7) = %q", got)
	}
	if got := FormatBigDecimal(apd.New(25, -1)); got != "0d2.5" {
		t.Errorf("FormatBigDecimal(2.5) = %q", got)
	}
}

// --- constructor variants -------------------------------------------------

func TestS5VEvalAndImplicitConstructors(t *testing.T) {
	l := NewEvalList([]Value{NewInteger(1)})
	if !l.Eval || !l.Parent.Equal(TList) {
		t.Error("NewEvalList must mark Eval on a TList value")
	}
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	m := NewEvalMap(om)
	if !m.Eval || !m.Parent.Equal(TMap) {
		t.Error("NewEvalMap must mark Eval on a TMap value")
	}
	im := NewImplicitMap(om)
	if !IsImplicitMap(im) {
		t.Error("NewImplicitMap must produce an implicit map")
	}
	e := NewEnum([]Value{NewAtom("a"), NewAtom("b")})
	if !e.Parent.Equal(TEnum) || !IsDisjunct(e) {
		t.Error("NewEnum must build a TEnum-tagged disjunct")
	}
}

func TestS5VAtomReferent(t *testing.T) {
	if _, ok := AtomReferent(NewInteger(1)); ok {
		t.Error("non-atom has no referent")
	}
	a := NewAtom("x")
	if _, ok := AtomReferent(a); ok {
		t.Error("fresh atom has no referent")
	}
	a2 := SetAtomReferent(a, NewInteger(9))
	ref, ok := AtomReferent(a2)
	if !ok {
		t.Fatal("referent must be readable after SetAtomReferent")
	}
	if n, _ := AsInteger(ref); n != 9 {
		t.Errorf("referent = %v, want 9", ref)
	}
}

// --- type naming ----------------------------------------------------------

func TestS5VTypeNameAndPathOf(t *testing.T) {
	if got := TypeNameOf(NewTypeLiteral(TInteger)); got != "Integer" {
		t.Errorf("TypeNameOf(literal) = %q", got)
	}
	if got := TypeNameOf(NewInteger(1)); got != "Integer" {
		t.Errorf("TypeNameOf(concrete) = %q", got)
	}
	if got := TypePathOf(NewTypeLiteral(TInteger)); got != "Scalar/Number/Integer" {
		t.Errorf("TypePathOf(literal) = %q", got)
	}
	if got := TypePathOf(NewString("s")); got != "Scalar/String/ProperString" {
		t.Errorf("TypePathOf(concrete) = %q", got)
	}
}

func TestS5VIsNilBehaviorFallback(t *testing.T) {
	bare := &Type{} // no tmeta => Behavior() is nil
	if NewInteger(1).Is(bare) {
		t.Error("Integer must not conform to an unrelated bare node")
	}
	if NewInteger(1).Is(nil) {
		t.Error("Is(nil) must be false")
	}
}

func TestS5VIsNoneShapeCarrierForm(t *testing.T) {
	if !IsNoneShape(Value{Parent: TNone}) {
		t.Error("hand-built Value{Parent: TNone} is a None shape")
	}
}

// --- interp / xml source renderers ----------------------------------------

func TestS5VRenderInterpParts(t *testing.T) {
	parts := []InterpPart{
		{Lit: "a b"},
		{Expr: []Value{NewWord("x"), NewInteger(2)}},
	}
	want := "interp('a b' ${word(x) 2})"
	if got := renderInterpParts(parts); got != want {
		t.Errorf("renderInterpParts = %q, want %q", got, want)
	}
	// The InterpString value renders identically through the kernel arm.
	if got := kernelFormatDefault(NewInterpString(parts)); got != want {
		t.Errorf("kernelFormatDefault(interp) = %q, want %q", got, want)
	}
}

func TestS5VRenderXmlTmplSrc(t *testing.T) {
	leaf := XmlTmpl{
		Tag: "br",
		Attr: []XmlAttrTmpl{{
			Name: "class",
			Parts: []InterpPart{
				{Lit: "x"},
				{Expr: []Value{NewWord("e"), NewWord("f")}},
			},
		}},
	}
	wantLeaf := `<br class="x${word(e) word(f)}"/>`
	if got := renderXmlTmplSrc(leaf); got != wantLeaf {
		t.Errorf("renderXmlTmplSrc(leaf) = %q, want %q", got, wantLeaf)
	}
	full := XmlTmpl{
		Tag: "div",
		Cren: []XmlCren{
			{Kind: XmlCrenLit, Lit: "hi"},
			{Kind: XmlCrenExpr, Expr: []Value{NewWord("g"), NewInteger(1)}},
			{Kind: XmlCrenChild, Child: &leaf},
		},
	}
	wantFull := "<div>hi${word(g) 1}" + wantLeaf + "</div>"
	if got := renderXmlTmplSrc(full); got != wantFull {
		t.Errorf("renderXmlTmplSrc(full) = %q, want %q", got, wantFull)
	}
	// The XmlInterp skeleton renders through the kernel arm.
	if got := kernelFormatDefault(NewXmlInterp(full)); got != "interp-xml("+wantFull+")" {
		t.Errorf("kernelFormatDefault(xml interp) = %q", got)
	}
}

// --- disjunct ordering tiebreak -------------------------------------------

func TestS5VDisjunctAltLessCanonTiebreak(t *testing.T) {
	a, b := NewInteger(1), NewFloat(1)
	// Cross-leaf magnitude equality: the primary CompareValues order
	// cannot separate 1 and 1.0, so the CanonValue tiebreak must give a
	// strict deterministic order (exactly one direction is "less").
	if disjunctAltLess(a, b) == disjunctAltLess(b, a) {
		t.Error("canon tiebreak must order 1 and 1.0 strictly")
	}
}

// --- accessor error arms and marker method --------------------------------

func TestS5VAccessorErrorArms(t *testing.T) {
	notIt := NewInteger(1)
	if _, err := AsSplice(notIt); err == nil {
		t.Error("AsSplice(int) must error")
	}
	if _, err := AsMove(notIt); err == nil {
		t.Error("AsMove(int) must error")
	}
	if _, err := AsDisjunct(notIt); err == nil {
		t.Error("AsDisjunct(int) must error")
	}
	if _, err := AsOptionsType(notIt); err == nil {
		t.Error("AsOptionsType(int) must error")
	}
	DispatchModInfo{}.payloadMarker() // seal marker is callable in-package
}

// --- flat instance schema keys --------------------------------------------

func TestS5VFlatInstancePartsResourceSchema(t *testing.T) {
	sf := NewOrderedMap()
	sf.Set("id", NewTypeLiteral(TString))
	rt := &ResourceTypeInfo{Fields: sf, Name: "Ideal/Resource/Entity/S5"}
	fields := NewOrderedMap()
	fields.Set("id", NewString("a"))
	ri := NewResourceInstance(TResourceEntity, ResourceInstanceInfo{TypeRef: rt, Fields: fields})
	got, schemaKeys, ok := FlatInstanceParts(ri)
	if !ok || got == nil {
		t.Fatal("FlatInstanceParts must succeed on a resource instance")
	}
	if len(schemaKeys) != 1 || schemaKeys[0] != "id" {
		t.Errorf("schemaKeys = %v, want [id]", schemaKeys)
	}
}

// --- table / list payload accessors ---------------------------------------

func TestS5VTableDataAccessors(t *testing.T) {
	rf := NewOrderedMap()
	rf.Set("c", NewTypeLiteral(TInteger))
	td := TableData{Record: RecordTypeInfo{Fields: rf}, Rows: []Value{NewInteger(1)}}
	v := NewValueRaw(TList, td)
	info, err := AsTableType(v)
	if err != nil {
		t.Fatalf("AsTableType(TableData): %v", err)
	}
	if info.Record.Fields.Len() != 1 {
		t.Error("AsTableType must surface the row record")
	}
	rows, err := AsList(v)
	if err != nil || rows.Len() != 1 {
		t.Errorf("AsList(TableData) = (%v, %v), want 1 row", rows.Len(), err)
	}
}

func TestS5VAsListEdgeArms(t *testing.T) {
	if _, err := AsList(NewTypeLiteral(TList)); err == nil {
		t.Error("AsList(type literal) must error on nil data")
	}
	okv := NewValueRaw(TList, MaterializerPayload{M: s7Mz{}})
	if rl, err := AsList(okv); err != nil || rl.Len() != 0 {
		t.Errorf("AsList(materializer payload) = (%d, %v), want empty", rl.Len(), err)
	}
	boom := errors.New("s5 boom")
	bad := NewValueRaw(TList, MaterializerPayload{M: s7Mz{err: boom}})
	if _, err := AsList(bad); err == nil {
		t.Error("AsList(failing materializer payload) must error")
	}
}

func TestS5VAsMutableListFlexAndFlexXml(t *testing.T) {
	fl := NewFlexList([]Value{NewInteger(1)})
	elems, err := AsMutableList(fl)
	if err != nil || len(elems) != 1 {
		t.Errorf("AsMutableList(flex) = (%d, %v), want 1 elem", len(elems), err)
	}
	fx := NewFlexXml("a", nil, nil)
	fd, err := AsFlexXml(fx)
	if err != nil || fd.Tag != "a" {
		t.Errorf("AsFlexXml = (%v, %v), want tag a", fd, err)
	}
}

func TestS5VAsFloatApproxLeaves(t *testing.T) {
	got, err := AsFloatApprox(NewBigDecimal(apd.New(25, -1)))
	if err != nil || got != 2.5 {
		t.Errorf("AsFloatApprox(BigDecimal 2.5) = (%v, %v)", got, err)
	}
	got, err = AsFloatApprox(NewInteger(3))
	if err != nil || got != 3 {
		t.Errorf("AsFloatApprox(3) = (%v, %v)", got, err)
	}
}

// --- Value.String dynamic wrapper -----------------------------------------

func TestS5VStringDynamicCarrier(t *testing.T) {
	if got := NewDynamicCarrier(TInteger).String(); got != "dynamic(Integer)" {
		t.Errorf("dynamic carrier String = %q", got)
	}
}

// --- kernelFormatDefault arms ---------------------------------------------

func TestS5VKernelFormatMarkerArms(t *testing.T) {
	bounded := NewValueRaw(TType, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	if got := kernelFormatDefault(bounded); got != "Integer/t" {
		t.Errorf("bounded type = %q, want Integer/t", got)
	}
	angle := NewSugar(SugarInfo{Kind: SugarAngle, Name: "Box", Items: []Value{NewTypeLiteral(TInteger)}})
	if got := kernelFormatDefault(angle); !strings.Contains(got, "angle Box") {
		t.Errorf("sugar angle = %q", got)
	}
	tb := NewSugar(SugarInfo{Kind: SugarTypeBound, Items: []Value{NewTypeLiteral(TInteger)}})
	if got := kernelFormatDefault(tb); !strings.Contains(got, "type-bound") {
		t.Errorf("sugar type-bound = %q", got)
	}
	fw := NewForward(ForwardInfo{FuncName: "f", CollectedArgs: 1, ExpectedArgs: 2})
	if got := kernelFormatDefault(fw); got != "forward(f,1/2)" {
		t.Errorf("forward = %q", got)
	}
	if got := kernelFormatDefault(NewOpenParen()); got != "(" {
		t.Errorf("open paren = %q", got)
	}
	if got := kernelFormatDefault(NewCloseParen()); got != ")" {
		t.Errorf("close paren = %q", got)
	}
	if got := kernelFormatDefault(NewEnd()); got != "end" {
		t.Errorf("end = %q", got)
	}
	if got := kernelFormatDefault(NewParenExpr([]Value{NewInteger(1)})); !strings.HasPrefix(got, "paren(") {
		t.Errorf("paren expr = %q", got)
	}
	if got := kernelFormatDefault(NewMark("m1")); got != "mark(m1)" {
		t.Errorf("mark = %q", got)
	}
	if got := kernelFormatDefault(NewMove("a", "b")); got != "move(a,b)" {
		t.Errorf("move = %q", got)
	}
	if got := kernelFormatDefault(NewNone()); got != "none" {
		t.Errorf("none = %q", got)
	}
}

func TestS5VKernelFormatScalarArms(t *testing.T) {
	if got := kernelFormatDefault(NewAtom("kite")); got != "kite" {
		t.Errorf("atom = %q", got)
	}
	if got := kernelFormatDefault(NewBigInteger(big.NewInt(5))); got != "0d5" {
		t.Errorf("big integer = %q", got)
	}
	if got := kernelFormatDefault(NewBigDecimal(apd.New(25, -1))); got != "0d2.5" {
		t.Errorf("big decimal = %q", got)
	}
	if got := kernelFormatDefault(NewBoolean(false)); got != "false" {
		t.Errorf("false = %q", got)
	}
	if got := kernelFormatDefault(NewPathon([]string{"a", "b"}, true)); got != "/a/b" {
		t.Errorf("pathon = %q", got)
	}
}

func TestS5VKernelFormatMicronBridge(t *testing.T) {
	orig := micronRenderBridge
	defer func() { micronRenderBridge = orig }()
	micronRenderBridge = func(Value) string { return "MICRON" }
	mv := NewValueRaw(TMicron, MicronPayload{Fields: NewOrderedMap()})
	if got := kernelFormatDefault(mv); got != "MICRON" {
		t.Errorf("micron bridge = %q", got)
	}
}

func TestS5VKernelFormatInstanceAndTypeArms(t *testing.T) {
	// Class instance whose TypeRef has no Name: renders under the ID.
	ci := NewClassInstance(TClass, ClassInstanceInfo{
		TypeRef: &ClassTypeInfo{ID: "T_s5cid", Fields: NewOrderedMap()},
		Fields:  NewOrderedMap(),
	})
	if got := kernelFormatDefault(ci); !strings.HasPrefix(got, "Ideal/Class/T_s5cid{") {
		t.Errorf("class instance = %q", got)
	}
	// Class instance with a NAMED TypeRef: renders under the type name.
	named := NewClassInstance(TClass, ClassInstanceInfo{
		TypeRef: &ClassTypeInfo{Name: "Ideal/Class/S5N", ID: "T_s5n", Fields: NewOrderedMap()},
		Fields:  NewOrderedMap(),
	})
	if got := kernelFormatDefault(named); !strings.HasPrefix(got, "Ideal/Class/S5N{") {
		t.Errorf("named class instance = %q", got)
	}
	// Resource instance with a named TypeRef.
	ri := NewResourceInstance(TResourceEntity, ResourceInstanceInfo{
		TypeRef: &ResourceTypeInfo{Name: "Ideal/Resource/Entity/S5R", Fields: NewOrderedMap()},
		Fields:  NewOrderedMap(),
	})
	if got := kernelFormatDefault(ri); !strings.HasPrefix(got, "Ideal/Resource/Entity/S5R{") {
		t.Errorf("resource instance = %q", got)
	}
	// Anonymous class TYPE body: renders under object<Ideal/Class/ID>.
	ct := NewClassType(TClass, ClassTypeInfo{ID: "T_s5ct", Fields: NewOrderedMap()})
	if got := kernelFormatDefault(ct); !strings.HasPrefix(got, "object<Ideal/Class/T_s5ct>") {
		t.Errorf("class type = %q", got)
	}
	// Disjunct and negation arms.
	dis := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if got := kernelFormatDefault(dis); got != "Integer tor String" {
		t.Errorf("disjunct = %q", got)
	}
	if got := kernelFormatDefault(NewNegation(NewTypeLiteral(TInteger))); got != "tnot Integer" {
		t.Errorf("negation simple = %q", got)
	}
	if got := kernelFormatDefault(NewNegation(dis)); got != "tnot (Integer tor String)" {
		t.Errorf("negation compound = %q", got)
	}
	// A compiled closure carrying its source rendering.
	cl := NewValueRaw(TFunction, ClosurePayload{Render: "fn zed"})
	if got := kernelFormatDefault(cl); got != "fn zed" {
		t.Errorf("closure render = %q", got)
	}
}

// --- FormatFnDef ----------------------------------------------------------

func TestS5VFormatFnDefArms(t *testing.T) {
	if got := FormatFnDef(FnDefInfo{}); got != "fn" {
		t.Errorf("empty = %q, want fn", got)
	}
	if got := FormatFnDef(FnDefInfo{Name: "f"}); got != "fn f" {
		t.Errorf("named sigless = %q, want fn f", got)
	}
	anon := FnDefInfo{Signatures: []Signature{{Params: []FnParam{{Type: TInteger}}}}}
	if got := FormatFnDef(anon); !strings.HasPrefix(got, "fn ") || got == "fn " {
		t.Errorf("anonymous with sig = %q", got)
	}
}

// --- IsTypeValue ----------------------------------------------------------

func TestS5VIsTypeValue(t *testing.T) {
	if !IsTypeValue(NewTypeLiteral(TInteger)) {
		t.Error("bare type literal is a type value")
	}
	if IsTypeValue(NewTypeLiteral(TNone)) {
		t.Error("the None literal is not a type value")
	}
	if !IsTypeValue(NewOptionsType(NewOrderedMap())) {
		t.Error("options type is a type value")
	}
	if !IsTypeValue(NewList([]Value{NewInteger(1), NewTypeLiteral(TInteger)})) {
		t.Error("list containing a type leaf is a type value")
	}
	if IsTypeValue(NewList([]Value{NewInteger(1)})) {
		t.Error("plain concrete list is not a type value")
	}
	tm := NewOrderedMap()
	tm.Set("t", NewTypeLiteral(TString))
	if !IsTypeValue(NewMap(tm)) {
		t.Error("map containing a type leaf is a type value")
	}
	pm := NewOrderedMap()
	pm.Set("k", NewInteger(1))
	if IsTypeValue(NewMap(pm)) {
		t.Error("plain concrete map is not a type value")
	}
}

// TestS5VIsUnboundSlot pins the VM's bound-checked local read: the ZERO
// Value — what a compiled frame slot holds before any store — and nothing
// the engine mints: a runtime value carries a Parent, a type node its
// metadata, a carrier its flag, an identity-only or dynamic value its bit.
func TestS5VIsUnboundSlot(t *testing.T) {
	if !(Value{}).IsUnboundSlot() {
		t.Error("the zero Value is the unbound slot")
	}
	for name, v := range map[string]Value{
		"an integer":     NewInteger(1),
		"a carrier":      NewCarrier(TInteger),
		"a type node":    NewTypeLiteral(TInteger),
		"an identity":    {ID: "slot-1"},
		"a dynamic bit":  {Dynamic: true},
		"a carrier bit":  {Carrier: true},
		"a bare payload": {Data: IntPayload{N: 1}},
	} {
		if v.IsUnboundSlot() {
			t.Errorf("%s is not the unbound slot", name)
		}
	}
}
