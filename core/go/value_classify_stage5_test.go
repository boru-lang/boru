package core

// Stage-5 coverage tests for value_classify.go: the active-token walker,
// module-scope binding rule, the const-bake classifier arms (extension
// capability walk, type bodies, fn values, surfaces, schemas, quoted
// parens), the fn-shape predicates, and the trivial-delegation check.
// White-box (package core); values are built through the public
// constructors so every payload is a legitimate sealed shape.

import "testing"

// c5BakeBehavior is a minimal TypeBehavior that also implements the
// ConstBakeable capability, so the ExtensionPayload arm of IsInertConst
// can find (or be refused) a bake verdict on the parent chain.
type c5BakeBehavior struct{ allow bool }

func (c5BakeBehavior) Match(v Value, t *Type) bool { return false }
func (c5BakeBehavior) Format(v Value) string       { return "c5" }
func (c5BakeBehavior) Equal(a, b Value) bool       { return false }
func (b c5BakeBehavior) BakeableConst(Value) bool  { return b.allow }

// newC5BakeType mints an ad-hoc lattice node carrying c5BakeBehavior.
// A fresh zero Value gets its own tmeta via SetName/SetBehavior, so no
// shared kernel type is ever mutated.
func newC5BakeType(allow bool) *Type {
	t := &Type{}
	t.SetName("C5Bake")
	t.SetBehavior(c5BakeBehavior{allow: allow})
	return t
}

// --- BearsActiveTokens ----------------------------------------------------

func TestS5CBearsActiveTokens(t *testing.T) {
	if !BearsActiveTokens(NewWord("w")) {
		t.Error("a word is an active token")
	}
	if !BearsActiveTokens(NewList([]Value{NewInteger(1), NewWord("w")})) {
		t.Error("a list containing a word bears active tokens")
	}
	if BearsActiveTokens(NewList([]Value{NewInteger(1)})) {
		t.Error("a plain scalar list bears no active tokens")
	}
	om := NewOrderedMap()
	om.Set("k", NewWord("w"))
	if !BearsActiveTokens(NewMap(om)) {
		t.Error("a map containing a word bears active tokens")
	}
	pm := NewOrderedMap()
	pm.Set("k", NewInteger(1))
	if BearsActiveTokens(NewMap(pm)) {
		t.Error("a plain scalar map bears no active tokens")
	}
	if BearsActiveTokens(Value{Parent: TMap, Data: MapPayload{}}) {
		t.Error("a nil-backed map payload bears no active tokens")
	}
	if BearsActiveTokens(NewInteger(1)) {
		t.Error("a scalar bears no active tokens")
	}
}

// --- ModuleScopeBinding ---------------------------------------------------

func TestS5CModuleScopeBinding(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !ModuleScopeBinding(r, "c5anything") {
		t.Error("with no fn baseline every binding is module scope")
	}
	r.PushFnBaseline(r.Defs.Snapshot())
	defer r.PopFnBaseline()
	r.Defs.Push("c5local", NewInteger(1))
	defer r.Defs.Pop("c5local")
	if ModuleScopeBinding(r, "c5local") {
		t.Error("a binding pushed after the baseline is fn-local")
	}
	if !ModuleScopeBinding(r, "c5unbound") {
		t.Error("an unbound name stays module scope under a baseline")
	}
}

// --- IsInertConst: ExtensionPayload capability walk ------------------------

func TestS5CInertConstExtensionCapability(t *testing.T) {
	yes := Value{Parent: newC5BakeType(true), Data: ExtensionPayload{Body: 1}}
	if !IsInertConst(yes) {
		t.Error("ConstBakeable(true) extension must bake")
	}
	no := Value{Parent: newC5BakeType(false), Data: ExtensionPayload{Body: 1}}
	if IsInertConst(no) {
		t.Error("ConstBakeable(false) extension must refuse")
	}
	nocap := Value{Parent: TAny, Data: ExtensionPayload{Body: 1}}
	if IsInertConst(nocap) {
		t.Error("an extension with no ConstBakeable in its chain must refuse")
	}
}

// --- IsInertConst: type-body / descriptor arms ------------------------------

func TestS5CInertConstDescriptorArms(t *testing.T) {
	mf := NewOrderedMap()
	mf.Set("a", NewTypeLiteral(TInteger))
	micron := Value{Parent: TMicron, Data: MicronTypeInfo{Fields: mf}}
	if !IsInertConst(micron) {
		t.Error("a carrier-free Micron type body bakes")
	}
	dep := Value{Parent: TInteger, Data: DepScalarInfo{}}
	if !IsInertConst(dep) {
		t.Error("a dependent-scalar predicate bakes by value")
	}
	fnsig := Value{Parent: TFnUndef, Data: FnUndefInfo{
		Sigs: []FnSigSpec{{Params: []FnParam{{Type: TInteger}}}},
	}}
	if !IsInertConst(fnsig) {
		t.Error("a pattern-free fn signature value bakes")
	}
	req := NewOrderedMap()
	req.Set("area", NewInteger(1))
	surf := Value{Parent: TSurface, Data: &SurfaceInfo{Type: TSurface, Required: req}}
	if !IsInertConst(surf) {
		t.Error("a surface with inert required shapes bakes")
	}
	schema := Value{Parent: TClass, Data: &TypeSchemaInfo{
		Type: TClass,
		Body: NewTypeLiteral(TInteger),
		Params: []GenParam{
			{Name: "T", HasBound: true, Bound: NewTypeLiteral(TInteger)},
			{Name: "U", HasDefault: true, Default: NewTypeLiteral(TString)},
		},
	}}
	if !IsInertConst(schema) {
		t.Error("a data-only generic schema bakes")
	}
}

func TestS5CInertConstFnDefArms(t *testing.T) {
	captured := NewFunction(FnDefInfo{Captured: []CapturedBinding{{Name: "x", Value: NewInteger(1)}}})
	if IsInertConst(captured) {
		t.Error("a capturing fn value must refuse")
	}
	pure := NewFunction(FnDefInfo{})
	if !IsInertConst(pure) {
		t.Error("a registry-free fn value bakes")
	}
	moduleFn := NewFunction(FnDefInfo{Registry: &Registry{}})
	if !IsInertConst(moduleFn) {
		t.Error("a non-macro module fn value bakes as data")
	}
	macro := NewFunction(FnDefInfo{Registry: &Registry{}, Macro: true})
	if IsInertConst(macro) {
		t.Error("a macro must refuse")
	}
}

func TestS5CInertConstMapAndQuotedParen(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	if !IsInertConst(NewMap(om)) {
		t.Error("an all-inert map bakes")
	}
	bad := NewOrderedMap()
	bad.Set("c", NewCarrier(TInteger))
	if IsInertConst(NewMap(bad)) {
		t.Error("a carrier-bearing map must refuse")
	}
	quoted := NewParenExpr([]Value{NewWord("add"), NewInteger(1)})
	quoted.Quoted = true
	if !IsInertConst(quoted) {
		t.Error("a quoted inert paren bakes as code-as-data")
	}
	unquoted := NewParenExpr([]Value{NewInteger(1)})
	if IsInertConst(unquoted) {
		t.Error("an unquoted paren is re-stepped and must refuse")
	}
}

// --- fn shape predicates ---------------------------------------------------

func TestS5CFnShapePredicates(t *testing.T) {
	if !IsFnTypedCarrier(NewCarrier(TFunction)) {
		t.Error("a Function-typed carrier is an fn-typed carrier")
	}
	if IsFnTypedCarrier(NewFunction(FnDefInfo{})) {
		t.Error("a concrete fn value is not a carrier")
	}
	if !IsFnValueResidual(NewFunction(FnDefInfo{})) {
		t.Error("a concrete fn value is an fn residual")
	}
	if !IsAppliableFn(NewFunction(FnDefInfo{})) {
		t.Error("an FnDefInfo payload is appliable")
	}
	if !IsAppliableFn(Value{Parent: TFunction}) {
		t.Error("a Function-typed value is appliable")
	}
	if IsAppliableFn(NewInteger(1)) {
		t.Error("an integer is not appliable")
	}
	if IsModuleFamilyValue(NewTypeLiteral(TInteger)) {
		t.Error("a bare type literal is not a module family value")
	}
}

// --- typeBodyConstOK arms --------------------------------------------------

func TestS5CTypeBodyConstOKArms(t *testing.T) {
	fresh := NewOrderedMap()
	fresh.Set("xs", NewFlexList(nil))
	rec := Value{Parent: TMap, Data: RecordTypeInfo{Fields: fresh}}
	if !typeBodyConstOK(rec) {
		t.Error("a record whose default is a freshened instance bakes")
	}
	of := NewOrderedMap()
	of.Set("o", NewTypeLiteral(TInteger))
	opts := Value{Parent: TMap, Data: OptionsTypeInfo{Fields: of}}
	if !typeBodyConstOK(opts) {
		t.Error("an inert options body bakes")
	}
	child := Value{Parent: TList, Data: ChildTypeInfo{
		Child:    NewTypeLiteral(TInteger),
		Elements: []Value{NewInteger(1)},
		Entries:  []ChildEntry{{Key: "k", Value: NewInteger(2)}},
	}}
	if !typeBodyConstOK(child) {
		t.Error("an inert typed-container body bakes")
	}
	dis := Value{Parent: TDisjunct, Data: DisjunctInfo{Alternatives: []Value{NewTypeLiteral(TInteger)}}}
	if !typeBodyConstOK(dis) {
		t.Error("an inert disjunct body bakes")
	}
	cf := NewOrderedMap()
	cf.Set("a", NewTypeLiteral(TInteger))
	class := Value{Parent: TClass, Data: ClassTypeInfo{Fields: cf}}
	if !typeBodyConstOK(class) {
		t.Error("a data-only class body bakes")
	}
	mf := NewOrderedMap()
	mf.Set("m", NewTypeLiteral(TString))
	micron := Value{Parent: TMicron, Data: MicronTypeInfo{Fields: mf}}
	if !typeBodyConstOK(micron) {
		t.Error("an inert Micron type body bakes")
	}
}

// --- inert reach / const members -------------------------------------------

func TestS5CInertReachAndConstMembers(t *testing.T) {
	lens := NewReach(ReachInfo{Segments: []ReachSeg{{KeyLit: NewAtom("name")}}})
	if !isInertReach(lens) {
		t.Error("a receiverless literal-key lens is inert")
	}
	lit := NewTypeLiteral(TInteger)
	if lit.ID == "" {
		t.Fatal("kernel type literals carry an ID")
	}
	if !IsInertConstMember(lit) {
		t.Error("an ID-bearing bare type node rides as a const member")
	}
	if !IsInertConstMember(NewFunction(FnDefInfo{})) {
		t.Error("a pure fn value rides as a const member")
	}
	// A fn value carries the registry that minted it — a main-program fn as
	// much as a module export — and the home says nothing about mutability:
	// the value is immutable code either way and is applied against that home
	// at run time (FnHome). Only a lexical capture is live state.
	if !IsInertConstMember(NewFunction(FnDefInfo{Registry: &Registry{}})) {
		t.Error("a fn value with a home rides as a const member too")
	}
	if IsInertConstMember(NewFunction(FnDefInfo{Captured: []CapturedBinding{{Name: "x", Value: NewInteger(1)}}})) {
		t.Error("a capturing fn value must refuse as a member")
	}
	if !IsInertConstMember(NewParenExpr([]Value{NewWord("w"), NewInteger(1)})) {
		t.Error("an inert deferred paren rides as a const member")
	}
	if IsInertConstMember(NewParenExpr([]Value{NewCarrier(TInteger)})) {
		t.Error("a carrier-bearing paren must refuse as a member")
	}
}

// --- IsDelegationFnDef ------------------------------------------------------

func TestS5CIsDelegationFnDef(t *testing.T) {
	if IsDelegationFnDef(FnDefInfo{}) {
		t.Error("a sig-less fn is not a delegation")
	}
	delegation := FnDefInfo{Signatures: []Signature{
		{Impl: Boru([]Value{NewWord("inner")})},
	}}
	if !IsDelegationFnDef(delegation) {
		t.Error("an all-[Word] wrapper is a delegation")
	}
	real := FnDefInfo{Signatures: []Signature{
		{Impl: Boru([]Value{NewInteger(1)})},
	}}
	if IsDelegationFnDef(real) {
		t.Error("a real body is not a delegation")
	}
}

// --- TypeIsFnShape / fn-shape-typed carriers --------------------------------

func TestTypeIsFnShapeAndFnShapeCarrier(t *testing.T) {
	if TypeIsFnShape(nil) {
		t.Error("nil type is not a fn shape")
	}
	if !TypeIsFnShape(TFnUndef) {
		t.Error("the anonymous fnsig type is a fn shape")
	}
	if TypeIsFnShape(TInteger) || TypeIsFnShape(TFunction) {
		t.Error("Integer and Function itself are not fn SHAPES")
	}
	// A carrier typed by a fn shape is a maybe-callable carrier — its
	// runtime inhabitant is a function (the NUR095 compiled half).
	shapeCarrier := NewCarrier(TFnUndef)
	if !IsFnTypedCarrier(shapeCarrier) {
		t.Error("a fn-shape-typed carrier must read as an fn-typed carrier")
	}
	fnCarrier := NewCarrier(TFunction)
	if !IsFnTypedCarrier(fnCarrier) {
		t.Error("a Function-typed carrier still reads as an fn-typed carrier")
	}
	if IsFnTypedCarrier(NewCarrier(TInteger)) {
		t.Error("an Integer carrier is not fn-typed")
	}
	if IsFnTypedCarrier(NewTypeLiteral(TFnUndef)) {
		t.Error("a CONCRETE fnsig type literal is not a carrier")
	}
}

// IsSteplessValue is an ALLOWLIST, and its test has to be one too: each admitted
// payload proved individually, and the refusals proved for the three ways a
// value stops being placed — a Word (a kind the switch does not name), an
// Eval-marked list the loop evaluates, and a bare type node with no payload at
// all. A denylist reading of this predicate is what would make a later token
// kind silently become data, so the negative rows are the contract.
func TestIsSteplessValue(t *testing.T) {
	stepless := []struct {
		name string
		v    Value
	}{
		{"integer", NewInteger(1)},
		{"float", NewFloat(1.5)},
		{"string", NewString("x")},
		{"boolean", NewBoolean(true)},
		{"atom", NewAtom("a")},
	}
	for _, tc := range stepless {
		if !IsSteplessValue(tc.v) {
			t.Errorf("%s: IsSteplessValue = false, want true", tc.name)
		}
	}

	if IsSteplessValue(NewWord("add")) {
		t.Error("a Word dispatches — IsSteplessValue must not admit it")
	}
	if IsSteplessValue(NewTypeLiteral(TInteger)) {
		t.Error("a bare type node carries no payload — IsSteplessValue must not admit it")
	}
	evalList := NewList([]Value{NewInteger(1)})
	evalList.Eval = true
	if IsSteplessValue(evalList) {
		t.Error("a list is not a scalar leaf — IsSteplessValue must not admit it")
	}
	evalInt := NewInteger(1)
	evalInt.Eval = true
	if IsSteplessValue(evalInt) {
		t.Error("Eval marks a value the loop EVALUATES — an admitted payload with it set is still active")
	}

	// A CARRIER is the case that needs the IsConcrete half specifically, and it
	// needs both shapes. A scalar carrier has nil Data, so it is refused by the
	// switch like a bare node; a carrier that DOES hold an admitted payload
	// reaches the matched arm and must still be refused there — it stands for a
	// value the analysis has not seen, which is the opposite of one the loop can
	// place.
	if IsSteplessValue(NewCarrier(TInteger)) {
		t.Error("a scalar carrier carries no payload — IsSteplessValue must not admit it")
	}
	payloadCarrier := NewInteger(1)
	payloadCarrier.Carrier = true
	if IsSteplessValue(payloadCarrier) {
		t.Error("a carrier over an admitted payload is still a carrier — IsConcrete is what refuses it")
	}
}

// The window predicate is the conjunction, and the empty window is the case
// worth pinning: running nothing returns what it was given, which is exactly
// why the callers may skip the engine.
func TestIsSteplessWindow(t *testing.T) {
	if !IsSteplessWindow(nil) {
		t.Error("the empty window is stepless — running nothing is the identity")
	}
	if !IsSteplessWindow([]Value{NewInteger(1), NewString("x"), NewBoolean(false)}) {
		t.Error("an all-scalar window is stepless")
	}
	if IsSteplessWindow([]Value{NewInteger(1), NewWord("add"), NewInteger(2)}) {
		t.Error("one active token makes the whole window active")
	}
}

// --- IsNativeWordFnDef / RegisteredWordIsNative ------------------------------

// TestParkedNativeApplyGate pins the two halves of the VM's parked-native apply
// gate. They ask the SAME question — "is there a boru body anywhere in the way?"
// — of two different things, the parked VALUE and the live NAME, because the
// language lets those drift apart.
func TestParkedNativeApplyGate(t *testing.T) {
	goSig := func() Signature {
		return Signature{Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, nil
		})}
	}
	if IsNativeWordFnDef(FnDefInfo{}) {
		t.Error("a sig-less value names no native")
	}
	if !IsNativeWordFnDef(FnDefInfo{Name: "add", Signatures: []Signature{goSig(), goSig()}}) {
		t.Error("every own sig a Go handler IS a parked native word")
	}
	mixed := FnDefInfo{Name: "add", Signatures: []Signature{
		goSig(),
		{Impl: Boru([]Value{NewInteger(1)})},
	}}
	if IsNativeWordFnDef(mixed) {
		t.Error("one boru-bodied own sig disqualifies the value — that body needs its dispatch frame")
	}

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if RegisteredWordIsNative(nil, "add") {
		t.Error("no registry, no answer")
	}
	if RegisteredWordIsNative(r, "no-such-word-here") {
		t.Error("an unbound name is not a native word")
	}
	r.Register("gate-native", goSig())
	if !RegisteredWordIsNative(r, "gate-native") {
		t.Error("a purely Go-handled registration is native through and through")
	}
	// The drift the gate exists for: a boru overload EXTENDS the name (the one
	// rebinding `extend_owner` permits), so a value parked while the word was
	// purely native would otherwise reach that body by name.
	r.Register("gate-extended", goSig(), Signature{Impl: Boru([]Value{NewInteger(1)})})
	if RegisteredWordIsNative(r, "gate-extended") {
		t.Error("a name carrying any boru overload must keep the interpreter")
	}
}
