package core

// Final-sweep coverage tests (stage 5): direct-call pins for the
// remaining uncovered arms in the value/behavior/helper layer —
// comparison behaviors, ideal converters, bounded types, flex/COW
// edges, XML behaviors, list/map format behaviors, depscalar edges,
// micron/moduletype/keyval arms, unify edges, and small helpers.
// Every case names the production block it arms.

import (
	"errors"
	"strings"
	"testing"
)

// --- compare_scalar_behaviors.go -------------------------------------------

func TestSweepScalarComparePrologueArms(t *testing.T) {
	// booleanCompareBehavior: the shared prologue settles a literal-vs-
	// concrete pair (type literal sorts first).
	c, err := booleanCompareBehavior{}.Compare(NewTypeLiteral(TBoolean), NewBoolean(true))
	if err != nil || c != -1 {
		t.Errorf("boolean prologue: got (%d, %v), want (-1, nil)", c, err)
	}
	// atomCompareBehavior: same prologue arm.
	c, err = atomCompareBehavior{}.Compare(NewTypeLiteral(TAtom), NewAtom("a"))
	if err != nil || c != -1 {
		t.Errorf("atom prologue: got (%d, %v), want (-1, nil)", c, err)
	}
}

func TestSweepComparePathonsLitVsLit(t *testing.T) {
	// Two bare Pathon type literals: AsPathon fails on both sides and both
	// Data are nil, so comparePathons falls to litVsLitOrder (a tie).
	if got := comparePathons(NewTypeLiteral(TPathon), NewTypeLiteral(TPathon)); got != 0 {
		t.Errorf("lit-vs-lit pathon order = %d, want 0", got)
	}
}

func TestSweepExportedComparerDoctrineHelpers(t *testing.T) {
	if c, ok := LitVsConcreteOrder(NewTypeLiteral(TInteger), NewInteger(1)); !ok || c != -1 {
		t.Errorf("LitVsConcreteOrder = (%d, %v)", c, ok)
	}
	if c := LitVsLitOrder(NewTypeLiteral(TInteger), NewTypeLiteral(TInteger)); c != 0 {
		t.Errorf("LitVsLitOrder same node = %d, want 0", c)
	}
	a := NewPathon([]string{"a"}, false)
	b := NewPathon([]string{"b"}, false)
	if c := ComparePathons(a, b); c != -1 {
		t.Errorf("ComparePathons(a, b) = %d, want -1", c)
	}
}

// --- compare_types.go ------------------------------------------------------

func TestSweepCompareStructuralFallbacks(t *testing.T) {
	// Neither list nor map: compareStructural falls to the canon render.
	c, err := compareStructural(NewString("a"), NewString("b"))
	if err != nil || c >= 0 {
		t.Errorf("compareStructural scalar fallback = (%d, %v)", c, err)
	}
	// compareMapEntries with non-map operands: canon fallback arm.
	c, err = compareMapEntries(NewInteger(1), NewInteger(2))
	if err != nil || c == 0 {
		t.Errorf("compareMapEntries non-map fallback = (%d, %v)", c, err)
	}
}

// --- convert_ideal.go ------------------------------------------------------

func TestSweepConvertBehaviorDelegates(t *testing.T) {
	if got := (objectConvertBehavior{}).Format(NewInteger(7)); got != "7" {
		t.Errorf("objectConvertBehavior.Format = %q", got)
	}
	if !(storeConvertBehavior{}).Match(NewInteger(1), TInteger) {
		t.Error("storeConvertBehavior.Match should delegate to the default")
	}
	if !(reachConvertBehavior{}).Match(NewInteger(1), TInteger) {
		t.Error("reachConvertBehavior.Match should delegate to the default")
	}
}

// --- core_boolean.go -------------------------------------------------------

func TestSweepUnionTypeSingleton(t *testing.T) {
	// Duplicate alternatives dedupe to a single bare literal.
	u := UnionType(NewTypeLiteral(TInteger), NewTypeLiteral(TInteger))
	if !IsBareTypeNode(u) || (&u).Leaf() != "Integer" {
		t.Errorf("UnionType singleton = %v", u)
	}
}

// --- core_boundedtype.go ---------------------------------------------------

func TestSweepNewBoundedTypeBody(t *testing.T) {
	body := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	bt := NewBoundedTypeBody(TScalar, body)
	if !IsBoundedType(bt) {
		t.Fatal("NewBoundedTypeBody did not produce a bounded Type")
	}
}

func TestSweepBoundedTypeSatisfiedViaBodyUnify(t *testing.T) {
	// A behavioral bound (a reparented disjunct body): the nominal probes
	// fail (the minted node is unrelated to Integer) and satisfaction
	// falls to the Unify-against-body arm, whose disjunct fold admits the
	// Integer literal.
	r := newTestRegistry(t)
	// The minted node parents under Type/Disjunct — the shape `def Foo
	// (Integer tor String)` produces — so the reparented body still
	// classifies as a disjunct for the Unify fold.
	node := r.Types.MintType("SweepBoundNode", TDisjunct)
	body := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	bt := NewBoundedTypeBody(node, body)
	child, ok := boundedChild(bt)
	if !ok {
		t.Fatal("boundedChild failed on a bounded Type")
	}
	if !boundedTypeSatisfied(NewTypeLiteral(TInteger), child, nil) {
		t.Error("Integer literal should satisfy the disjunct body bound via Unify")
	}
}

func TestSweepTypeMembershipBehaviorArms(t *testing.T) {
	if !(typeMembershipBehavior{}).Match(NewTypeLiteral(TInteger), TType) {
		t.Error("a bare type literal must be a member of Type")
	}
	if !(typeMembershipBehavior{}).Equal(NewInteger(1), NewInteger(1)) {
		t.Error("typeMembershipBehavior.Equal should delegate to the default")
	}
}

// --- core_flex.go ----------------------------------------------------------

func TestSweepFlexDeepCopyKeepsElemConstraint(t *testing.T) {
	m := mapOf("a", NewInteger(1))
	(&m).SetElemConstraint(NewTypeLiteral(TInteger))
	out, err := FlexDeepCopy(m)
	if err != nil {
		t.Fatalf("FlexDeepCopy map: %v", err)
	}
	if elem, ok := out.ElemConstraint(); !ok || (&elem).Leaf() != "Integer" {
		t.Errorf("flex map lost {:T}: %v %v", elem, ok)
	}

	l := NewList([]Value{NewInteger(1)})
	(&l).SetElemConstraint(NewTypeLiteral(TInteger))
	out, err = FlexDeepCopy(l)
	if err != nil {
		t.Fatalf("FlexDeepCopy list: %v", err)
	}
	if elem, ok := out.ElemConstraint(); !ok || (&elem).Leaf() != "Integer" {
		t.Errorf("flex list lost [:T]: %v %v", elem, ok)
	}
}

func TestSweepFlexXmlAttrCopy(t *testing.T) {
	attr := NewOrderedMap()
	attr.Set("k", NewString("v"))
	x := NewXmlElement("a", attr, []Value{NewString("txt")})
	out, err := FlexDeepCopy(x)
	if err != nil {
		t.Fatalf("FlexDeepCopy xml: %v", err)
	}
	_, oattr, _, ok := xmlParts(out)
	if !ok || oattr == nil || oattr.Len() != 1 {
		t.Errorf("flex xml attributes not copied: %v", out)
	}
}

func TestSweepNodeDeepCopyKeepsElemConstraint(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	fm := NewFlexMap(om)
	(&fm).SetElemConstraint(NewTypeLiteral(TInteger))
	out, err := NodeDeepCopy(fm)
	if err != nil {
		t.Fatalf("NodeDeepCopy flex map: %v", err)
	}
	if elem, ok := out.ElemConstraint(); !ok || (&elem).Leaf() != "Integer" {
		t.Errorf("node map lost {:T}: %v %v", elem, ok)
	}

	fl := NewFlexList([]Value{NewInteger(2)})
	(&fl).SetElemConstraint(NewTypeLiteral(TInteger))
	out, err = NodeDeepCopy(fl)
	if err != nil {
		t.Fatalf("NodeDeepCopy flex list: %v", err)
	}
	if elem, ok := out.ElemConstraint(); !ok || (&elem).Leaf() != "Integer" {
		t.Errorf("node list lost [:T]: %v %v", elem, ok)
	}
}

func TestSweepNodeDeepCopyXmlAttrs(t *testing.T) {
	attr := NewOrderedMap()
	attr.Set("k", NewString("v"))
	// A flex child makes containsFlex true so the Xml branch runs; the
	// attribute loop then rebuilds the attr map.
	x := NewXmlElement("a", attr, []Value{NewFlexMap(NewOrderedMap())})
	out, err := NodeDeepCopy(x)
	if err != nil {
		t.Fatalf("NodeDeepCopy xml: %v", err)
	}
	_, oattr, ocren, ok := xmlParts(out)
	if !ok || oattr == nil || oattr.Len() != 1 || len(ocren) != 1 {
		t.Errorf("node xml shape wrong: %v", out)
	}
	if IsFlexNode(ocren[0]) {
		t.Error("node conversion left a flex child inside")
	}
}

func TestSweepMakeNodeHandlerRegisteredKindPrecedence(t *testing.T) {
	// A host-registered, ENABLED Ideal claiming a Node-family target owns
	// its instantiation — MakeNodeHandler defers to Ideal.Instantiate
	// before any kernel conversion.
	r := newTestRegistry(t)
	kind := &Ideal{
		Name:    "SweepKind",
		Enabled: true,
		Accepts: func(base Value) bool {
			return base.Data == nil && !base.Carrier && base.ID == TFlexMap.ID
		},
		Instantiate: func(typ, data Value, _ *Registry) ([]Value, error) {
			return []Value{NewString("sweep-made")}, nil
		},
	}
	r.Ideals.Register(kind)
	out, err := MakeNodeHandler([]Value{NewTypeLiteral(TFlexMap), mapOf("a", NewInteger(1))}, nil, nil, r)
	if err != nil {
		t.Fatalf("registered-kind make: %v", err)
	}
	if got, _ := AsString(out[0]); got != "sweep-made" {
		t.Errorf("Instantiate result = %v", out)
	}
	// The same kind disabled is a loud compile failure, not a silent fall-through.
	kind.Enabled = false
	if _, err := MakeNodeHandler([]Value{NewTypeLiteral(TFlexMap), mapOf("a", NewInteger(1))}, nil, nil, r); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Errorf("disabled kind: err = %v", err)
	}
}

// --- core_ref.go -----------------------------------------------------------

func TestSweepFunctionWrapperGuards(t *testing.T) {
	notFn := NewInteger(3)
	fnShapeBadPayload := NewValueRaw(TFunction, StrPayload{S: "nope"})

	if _, ok := UsurpFunction(notFn); ok {
		t.Error("UsurpFunction accepted a non-function")
	}
	if _, ok := UsurpFunction(fnShapeBadPayload); ok {
		t.Error("UsurpFunction accepted a TFunction value without FnDefInfo")
	}
	if _, ok := ForceStackFunction(notFn); ok {
		t.Error("rebarrier accepted a non-function")
	}
	if _, ok := ForceStackFunction(fnShapeBadPayload); ok {
		t.Error("rebarrier accepted a TFunction value without FnDefInfo")
	}
	if _, ok := ForceArityFunction(notFn, 1); ok {
		t.Error("ForceArityFunction accepted a non-function")
	}
	if _, ok := ForceArityFunction(fnShapeBadPayload, 1); ok {
		t.Error("ForceArityFunction accepted a TFunction value without FnDefInfo")
	}
}

func sweepFnDef() FnDefInfo {
	return FnDefInfo{
		Name: "sweeporig",
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "a", Type: TInteger}},
			BarrierPos: 1,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return args, nil
			}),
		}},
	}
}

func TestSweepForceForwardFunction(t *testing.T) {
	fn := NewFunction(sweepFnDef())
	wrapped, ok := ForceForwardFunction(fn)
	if !ok {
		t.Fatal("ForceForwardFunction declined a function value")
	}
	wf, wok := wrapped.Data.(FnDefInfo)
	if !wok || len(wf.Signatures) != 1 {
		t.Fatalf("wrapper shape wrong: %v", wrapped)
	}
	if wf.Signatures[0].BarrierPos != 1 {
		t.Errorf("all-forward wrapper barrier = %d, want 1", wf.Signatures[0].BarrierPos)
	}
}

func TestSweepDispatchHandlersStackParts(t *testing.T) {
	orig := NewInteger(9)
	a, b := NewInteger(1), NewInteger(2)

	// rebarrier: barrier 0 puts every arg on the stack part.
	out, err := rebarrierDispatchHandler(orig, 0)([]Value{a, b}, nil, nil, nil)
	if err != nil {
		t.Fatalf("rebarrier handler: %v", err)
	}
	// ( b a orig ) — args reversed onto the stack, then orig.
	if len(out) != 5 || renderAll(out[1:4]) != "2 | 1 | 9" {
		t.Errorf("rebarrier layout = %s", renderAll(out))
	}

	// usurp: barrier 0 emits the args in order below orig.
	out, err = usurpDispatchHandler(orig, 0)([]Value{a, b}, nil, nil, nil)
	if err != nil {
		t.Fatalf("usurp handler: %v", err)
	}
	if len(out) != 5 || renderAll(out[1:4]) != "1 | 2 | 9" {
		t.Errorf("usurp layout = %s", renderAll(out))
	}
}

// --- core_xml.go -----------------------------------------------------------

func TestSweepXmlBehaviorArms(t *testing.T) {
	el := NewXmlElement("a", nil, nil)
	if !(xmlBehavior{}).Match(el, TXml) {
		t.Error("xmlBehavior.Match should delegate to conformance")
	}
	if tag, _, _, ok := XmlParts(el); !ok || tag != "a" {
		t.Errorf("XmlParts = %q, %v", tag, ok)
	}
	// Self-closing render for an element without children.
	if got := (xmlBehavior{}).Format(el); got != "<a/>" {
		t.Errorf("empty element render = %q", got)
	}
}

func TestSweepXmlElementsEqualAttrWalk(t *testing.T) {
	mk := func(val string) Value {
		attr := NewOrderedMap()
		attr.Set("k", NewString(val))
		return NewXmlElement("a", attr, nil)
	}
	if !xmlElementsEqual(mk("v"), mk("v")) {
		t.Error("equal attributes should compare equal")
	}
	if xmlElementsEqual(mk("v"), mk("w")) {
		t.Error("differing attribute values should compare unequal")
	}
}

func TestSweepIsValidXmlNameRejectsBadChar(t *testing.T) {
	if IsValidXmlName("a b") {
		t.Error("space inside an XML name must be rejected")
	}
}

// --- coretype_list_map_behaviors.go ----------------------------------------

type sweepMaterializer struct{ fail bool }

func (m sweepMaterializer) Materialize() (TableData, error) {
	if m.fail {
		return TableData{}, errors.New("sweep boom")
	}
	return TableData{Record: intStrRecord(), Rows: []Value{mapOf("a", NewInteger(1), "b", NewString("x"))}}, nil
}
func (m sweepMaterializer) SourceRecord() RecordTypeInfo { return intStrRecord() }

func TestSweepListFormatBehaviorPayloadArms(t *testing.T) {
	lfb := listFormatBehavior{}

	// TableData: the row-render loop.
	td := TableData{Record: intStrRecord(), Rows: []Value{mapOf("a", NewInteger(1), "b", NewString("x"))}}
	if got := lfb.Format(NewValueRaw(TList, td)); !strings.Contains(got, "table{") || !strings.Contains(got, "a:1") {
		t.Errorf("TableData render = %q", got)
	}

	// MaterializerPayload: error and success arms.
	if got := lfb.Format(NewValueRaw(TList, MaterializerPayload{M: sweepMaterializer{fail: true}})); !strings.Contains(got, "query(error:sweep boom)") {
		t.Errorf("materializer error render = %q", got)
	}
	if got := lfb.Format(NewValueRaw(TList, MaterializerPayload{M: sweepMaterializer{}})); !strings.Contains(got, "table{") {
		t.Errorf("materializer render = %q", got)
	}

	// Typed list with inline elements.
	tl := NewTypedListWithElements(NewTypeLiteral(TInteger), []Value{NewInteger(1), NewInteger(2)})
	if got := lfb.Format(tl); got != "[:Integer 1 2]" {
		t.Errorf("typed-list-with-elements render = %q", got)
	}
}

func TestSweepMapFormatBehaviorArms(t *testing.T) {
	mfb := mapFormatBehavior{}

	// Typed map with inline entries.
	tm := NewTypedMapWithEntries(NewTypeLiteral(TInteger), []ChildEntry{{Key: "a", Value: NewInteger(1)}})
	if got := mfb.Format(tm); got != "{:Integer a:1}" {
		t.Errorf("typed-map-with-entries render = %q", got)
	}

	// A module namespace renders compactly by its export keys.
	ns := WithModuleNS(mapOf("f", NewInteger(1)), "SweepMod", NewModuleInstance(ModuleDesc{Ref: "sweep:mod"}))
	if got := mfb.Format(ns); !strings.Contains(got, "Module(SweepMod){f}") {
		t.Errorf("module namespace render = %q", got)
	}
}

// --- depscalar.go ----------------------------------------------------------

func TestSweepDepScalarEdges(t *testing.T) {
	if got := canonicalBaseType(TNumber); got != TNumber {
		t.Errorf("canonicalBaseType(TNumber) = %v", got)
	}
	// Strict upper bound: value < bound.
	if !depBoundCheck(&DepBound{Value: NewInteger(5)}, false, NewInteger(3)) {
		t.Error("3 < 5 (strict) should hold")
	}
	// combineDepScalars: a has an upper bound, b none.
	hi := &DepBound{Inclusive: true, Value: NewInteger(9)}
	out, ok := combineDepScalars(DepScalarInfo{Hi: hi}, DepScalarInfo{})
	if !ok || out.Hi != hi || out.Lo != nil {
		t.Errorf("combineDepScalars kept the wrong bounds: %+v ok=%v", out, ok)
	}
	// complementWithinBase: interval → disjunct of the two outer rays.
	interval := DepScalarInfo{
		Lo: &DepBound{Value: NewInteger(1)},
		Hi: &DepBound{Value: NewInteger(9)},
	}
	comp := complementWithinBase(TInteger, interval)
	if !IsDisjunct(comp) {
		t.Errorf("interval complement should be a disjunct: %v", comp)
	}
	// complementWithinBase: upper-only → flipped lower bound.
	hiOnly := complementWithinBase(TInteger, DepScalarInfo{Hi: &DepBound{Value: NewInteger(9)}})
	di, err := hiOnly.AsDepScalar()
	if err != nil || di.Lo == nil || di.Hi != nil || !di.Lo.Inclusive {
		t.Errorf("upper-only complement = %+v, %v", di, err)
	}
}

// --- didyoumean.go ---------------------------------------------------------

func TestSweepSuggestNamesDistanceOrder(t *testing.T) {
	// Two hits at distances 1 and 2 force the comparator's distance leg.
	got := SuggestNames("hello", []string{"hballo", "hallo"})
	if len(got) != 2 || got[0] != "hallo" || got[1] != "hballo" {
		t.Errorf("SuggestNames order = %v", got)
	}
}

// --- keyval.go -------------------------------------------------------------

func TestSweepNewKeyVal(t *testing.T) {
	kv := NewKeyVal("name", NewInteger(7), 1, 3)
	if !kv.Parent.Equal(TKeyVal) {
		t.Fatalf("KeyVal parent = %v", kv.Parent)
	}
	m, err := AsMap(kv)
	if err != nil {
		t.Fatalf("KeyVal is not map-readable: %v", err)
	}
	k, _ := m.Get(KeyValK)
	i, _ := m.Get(KeyValI)
	n, _ := m.Get(KeyValN)
	if ks, _ := AsString(k); ks != "name" {
		t.Errorf("k = %v", k)
	}
	if iv, _ := AsInteger(i); iv != 1 {
		t.Errorf("i = %v", i)
	}
	if nv, _ := AsInteger(n); nv != 3 {
		t.Errorf("n = %v", n)
	}
}

// --- micron_kernel.go ------------------------------------------------------

func TestSweepMicronKernelArms(t *testing.T) {
	if _, err := AsMicronType(NewInteger(1)); err == nil {
		t.Error("AsMicronType accepted a non-Micron value")
	}
	om := NewOrderedMap()
	om.Set("user", NewString("a"))
	mv := NewValueRaw(TScalar, MicronPayload{Fields: om})
	fields, err := AsMicronFields(mv)
	if err != nil || fields != om {
		t.Errorf("AsMicronFields = %v, %v", fields, err)
	}
	if _, err := AsMicronFields(NewInteger(1)); err == nil {
		t.Error("AsMicronFields accepted a non-Micron value")
	}
	if PathonContentEqual(PathonInfo{Parts: []string{"a"}}, PathonInfo{Parts: []string{"b"}}) {
		t.Error("differing segments must not be content-equal")
	}
	// Render-bridge registration: install then restore the previous hook.
	prev := micronRenderBridge
	RegisterMicronRenderBridge(func(Value) string { return "sweep" })
	if micronRenderBridge == nil {
		t.Error("bridge not installed")
	}
	RegisterMicronRenderBridge(prev)
	if !MicronFieldsEqual(om, om) {
		t.Error("a field map must equal itself")
	}
}

// --- moduletype.go ---------------------------------------------------------

func TestSweepModuleTypeArms(t *testing.T) {
	if ModuleKind(ModuleDesc{}) != "inline" {
		t.Error("empty kind should default to inline")
	}
	inst := NewModuleInstance(ModuleDesc{})
	if !(moduleTypeBehavior{}).Match(inst, TModule) {
		t.Error("module instance should match its own type")
	}
	if !(moduleTypeBehavior{}).Equal(inst, inst) {
		t.Error("a module instance should equal itself")
	}
	if got := (moduleTypeBehavior{path: "Ideal/Module"}).Format(inst); got != "Module(inline)" {
		t.Errorf("ref-less module render = %q", got)
	}
	if desc, ok := AsModuleDesc(inst); !ok || desc.Kind != "" {
		t.Errorf("AsModuleDesc = %+v, %v", desc, ok)
	}
}

// --- nodify.go -------------------------------------------------------------

type sweepNodifier struct{ defaultBehavior }

func (sweepNodifier) Nodify(Value) (Value, error) { return NewString("nodified"), nil }

func TestSweepNodifyValue(t *testing.T) {
	// No Nodifier anywhere on an Integer's chain: identity, walking past
	// the non-Nodifier behaviors.
	out, err := NodifyValue(NewInteger(4))
	if err != nil {
		t.Fatalf("NodifyValue integer: %v", err)
	}
	if got, _ := AsInteger(out); got != 4 {
		t.Errorf("integer nodify = %v", out)
	}
	// A registered Nodifier terminates the walk with its projection.
	r := newTestRegistry(t)
	nt := r.Types.MintTypeWithBehavior("SweepNodify", TIdeal, sweepNodifier{})
	out, err = NodifyValue(Value{Parent: nt})
	if err != nil {
		t.Fatalf("NodifyValue custom: %v", err)
	}
	if got, _ := AsString(out); got != "nodified" {
		t.Errorf("custom nodify = %v", out)
	}
}

// --- policy_hook.go --------------------------------------------------------

func TestSweepPolicyHookArms(t *testing.T) {
	if LookupModuleCallChecker(nil) != nil {
		t.Error("nil registry must yield no checker")
	}
	r := newTestRegistry(t)
	if LookupModuleCallChecker(r) != nil {
		t.Error("no installed policy must yield no checker")
	}
	if err := policyGateModuleCallReg(r, &ModuleCallID{Module: "m", Export: "e"}); err != nil {
		t.Errorf("checker-less gate must allow: %v", err)
	}
	inner := errors.New("sweep denied")
	pd := PolicyDenied{Err: inner}
	if pd.Error() != "sweep denied" {
		t.Errorf("PolicyDenied.Error = %q", pd.Error())
	}
	if !errors.Is(pd, inner) {
		t.Error("Unwrap must expose the checker error")
	}
}

// --- shape.go --------------------------------------------------------------

func TestSweepShapeSpecialArms(t *testing.T) {
	if got := Shape(NewTypeLiteral(TNever)); got != ShapeNever {
		t.Errorf("Shape(Never) = %v", got)
	}
	inst := NewClassInstance(TClass, ClassInstanceInfo{Fields: NewOrderedMap()})
	if got := Shape(inst); got != ShapeObjectInstance {
		t.Errorf("Shape(class instance) = %v", got)
	}
	ct := NewClassType(TClass, ClassTypeInfo{Fields: NewOrderedMap()})
	if got := Shape(ct); got != ShapeObjectType {
		t.Errorf("Shape(class type) = %v", got)
	}
}

// --- sigimpl.go ------------------------------------------------------------

func TestSweepSignatureImplAccessors(t *testing.T) {
	// A bare Signature declares nothing (and checkFullStackFn's non-Go
	// arm returns nil on the way).
	var s Signature
	if s.DeclaresCheckReturns() {
		t.Error("an implementation-less signature declares no returns")
	}
	// DispatchSig drives the handler with resolved args.
	sig := &Signature{Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		n, _ := AsInteger(args[0])
		return []Value{NewInteger(n + 1)}, nil
	})}
	out, err := DispatchSig(sig, []Value{NewInteger(41)}, newTestRegistry(t))
	if err != nil {
		t.Fatalf("DispatchSig: %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 42 {
		t.Errorf("DispatchSig result = %v", out)
	}
}

// --- storage_helpers.go ----------------------------------------------------

func TestSweepGetKeyCoercions(t *testing.T) {
	if GetKey(NewString("s")) != "s" {
		t.Error("string key")
	}
	if GetKey(NewAtom("a")) != "a" {
		t.Error("atom key")
	}
	if GetKey(NewInteger(12)) != "12" {
		t.Error("integer key")
	}
	if GetKey(NewList(nil)) == "" {
		t.Error("fallback key render should be non-empty")
	}
}

// --- typebehavior.go -------------------------------------------------------

func TestSweepBehaviorWrapperDelegation(t *testing.T) {
	w := behaviorWrapper{prev: DefaultBehavior}
	if got := w.Format(NewInteger(3)); got != "3" {
		t.Errorf("wrapper Format = %q", got)
	}
	if !w.Equal(NewInteger(3), NewInteger(3)) {
		t.Error("wrapper Equal should delegate")
	}
	if _, err := w.Compare(NewInteger(1), NewInteger(2)); err == nil {
		t.Error("wrapper over the default should opt out of Compare")
	}
}

// --- unify_error.go / unify_fnsig.go / unify_fold.go / unify_map.go --------

func TestSweepUnifyErrorPathRender(t *testing.T) {
	e := &UnifyError{Reason: "nope", Path: []string{"a", "b"}}
	if e.Error() != "a.b: nope" {
		t.Errorf("UnifyError.Error = %q", e.Error())
	}
}

func TestSweepUnifyFnUndefNoSatisfy(t *testing.T) {
	undef := NewFnUndef(FnUndefInfo{Sigs: []FnSigSpec{{
		Params: []FnParam{{Type: TInteger}, {Type: TInteger}},
	}}})
	fn := NewFunction(sweepFnDef()) // one param — cannot cover the 2-arg shape
	if _, uerr := unifyFnUndefShape(undef, ShapeFnUndef, fn, ShapeFunction, nil); uerr == nil {
		t.Error("a non-covering function must fail the FnUndef constraint")
	}
}

func TestSweepUnifyCarrierVsTypedArms(t *testing.T) {
	typed := NewTypedMap(NewTypeLiteral(TInteger))
	carrier := NewCarrier(TMap) // Map carrier — NOT flex-family
	// Reversed argument order arms the sb-carrier case; the non-flex
	// Parent then declines.
	if _, ok := unifyCarrierVsTyped(typed, ShapeTypedMap, carrier, ShapeCarrier, ShapeTypedMap, TFlexMap); ok {
		t.Error("a plain Map carrier must not be flex-tagged")
	}
}

func TestSweepUnifyMapFamilyOptionsDispatch(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	opts := NewOptionsType(fields)
	conc := mapOf("a", NewInteger(1))
	if _, uerr := unifyMapFamily(opts, Shape(opts), conc, Shape(conc), nil); uerr != nil {
		t.Errorf("options-vs-concrete dispatch failed: %v", uerr)
	}
}

func TestSweepUnifyConcreteMapsAbsentOmission(t *testing.T) {
	am := NewOrderedMap()
	am.Set("x", NewTypeLiteral(TAbsent))
	bm := NewOrderedMap()
	bm.Set("y", NewTypeLiteral(TAbsent))
	out, uerr := unifyConcreteMaps(am, bm, nil)
	if uerr != nil {
		t.Fatalf("unifyConcreteMaps: %v", uerr)
	}
	m, err := AsMap(out)
	if err != nil || m.Len() != 0 {
		t.Errorf("Absent-valued keys must be omitted, got %v", out)
	}
}

// --- word_name.go ----------------------------------------------------------

func TestSweepValidateWordNameArms(t *testing.T) {
	if err := ValidateWordName(""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty name: %v", err)
	}
	if err := ValidateWordName("$$"); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("all-dollar name: %v", err)
	}
	if err := ValidateWordName("--"); err == nil || !strings.Contains(err.Error(), "hyphens") {
		t.Errorf("all-hyphen name: %v", err)
	}
}

// --- member_behavior.go ----------------------------------------------------

func TestSweepMemberBehaviorArms(t *testing.T) {
	pos := func(v Value) bool {
		n, err := AsInteger(v)
		return err == nil && n > 0
	}
	b, ok := MemberBehavior(pos).(MemberUnifier)
	if !ok {
		t.Fatal("MemberBehavior did not return a MemberUnifier")
	}
	r := newTestRegistry(t)
	mt := r.Types.MintMemberType("SweepPos", TInteger, pos)
	if !b.Match(NewInteger(2), mt) {
		t.Error("a member value should Match")
	}
	if out, uerr := b.Unify(NewTypeLiteral(TInteger), NewInteger(2), nil); uerr != nil || !IsConcrete(out) {
		t.Errorf("member Unify = %v, %v", out, uerr)
	}
	if _, err := b.Compare(NewInteger(1), NewInteger(2)); err == nil {
		t.Error("member Compare should opt out via the default")
	}
	// The parent gate around the predicate: conforming member passes,
	// a value outside the family is rejected before the predicate runs.
	gated, gok := mt.Behavior().(MemberUnifier)
	if !gok {
		t.Fatal("minted member type lost its MemberUnifier")
	}
	if !gated.member(NewInteger(3)) {
		t.Error("gated member should accept a conforming member")
	}
	if gated.member(NewString("x")) {
		t.Error("gated member must reject a value outside the parent family")
	}
}
