package core

import "testing"

// --- inactiveEmit: the no-op recorder -----------------------------------

// TestRecorderAccessorFallback covers CheckState.Recorder()'s inactive
// fallback: a nil CheckState and a CheckState with no installed Emit both
// return the shared no-op recorder.
func TestRecorderAccessorFallback(t *testing.T) {
	var c *CheckState
	if c.Recorder() != TheInactiveEmit {
		t.Fatal("nil CheckState should yield the inactive recorder")
	}
	c2 := &CheckState{}
	if c2.Recorder() != TheInactiveEmit {
		t.Fatal("empty CheckState should yield the inactive recorder")
	}
}

// --- direct-call helpers -------------------------------------------------

func fnVal(sigs ...Signature) Value {
	return Value{ID: GenerateID(IDPrefixForType(TFunction)), Parent: TFunction, Data: FnDefInfo{Signatures: sigs}}
}

// zeroArgFn is a genuine 0-param fn value (two 0-arg sigs → real + phantom),
// the shape the interpreter auto-dispatches when it lands with no operands.
func zeroArgFn() Value { return fnVal(Signature{}, Signature{}) }

func TestFnValueZeroArg(t *testing.T) {
	if !FnValueZeroArg(zeroArgFn()) {
		t.Fatal("two 0-arg sigs should be a genuine 0-param fn")
	}
	if !FnValueZeroArg(fnVal(Signature{})) {
		t.Fatal("single 0-arg sig should be a genuine 0-param fn")
	}
	// A DIRECT-literal mixed overload set (a real 0-arg among arg-taking
	// sigs, no synthetic Fallback): the recorded events model the fire,
	// so the read-guard does not decline it.
	if FnValueZeroArg(fnVal(Signature{Args: []*Type{TInteger}}, Signature{})) {
		t.Fatal("a direct-literal mixed overload set compiles — no compile failure")
	}
	// The PARKED spelling of the same mixed set (the aggregate view,
	// recognisable by its synthetic Fallback sig): the landing is
	// unmodelled, so the guard must decline — the representation gap the
	// predicate's doc records.
	if !FnValueZeroArg(fnVal(Signature{Args: []*Type{TInteger}}, Signature{}, Signature{Fallback: true})) {
		t.Fatal("a parked mixed overload set must decline until the landing model covers it")
	}
	// A parked ARG-taking fn (no real 0-arg overload, just the synthetic
	// Fallback): lands as data in both engines — no compile failure.
	if FnValueZeroArg(fnVal(Signature{Args: []*Type{TInteger}}, Signature{Fallback: true})) {
		t.Fatal("a parked args-only fn lands as data — no compile failure")
	}
	if FnValueZeroArg(NewInteger(1)) {
		t.Fatal("non-fn value is not a 0-param fn")
	}
}

// --- producerWord / producerReturnedClosureArity ------------------------

// --- make-list / make-map / interp / closure compile failures -------------------

// --- recordCallCompileFailure classifier arms ----------------------------------

// --- recordCallOperands fn-inert const bake -----------------------------

// --- RecordPolyCall / RecordDynMethod guards ----------------------------

// --- flat-instance non-fn key (the continue arms) -----------------------

// --- isInertConst / type-body classifier family -------------------------

func TestAllInertAndFields(t *testing.T) {
	if allInert([]Value{NewInteger(1)}, func(Value) bool { return false }) {
		t.Fatal("a failing predicate should not be all-inert")
	}
	if allFieldsInert(nil, func(Value) bool { return true }) {
		t.Fatal("a nil field map is not inert")
	}
}

func TestTypeBodyConstOKChildType(t *testing.T) {
	// Child not const-safe → false.
	bad := Value{Data: ChildTypeInfo{Child: NewCarrier(TInteger)}}
	if typeBodyConstOK(bad) {
		t.Fatal("carrier child should decline")
	}
	// Child ok, but an entry value is not const-safe → false.
	badEntry := Value{Data: ChildTypeInfo{
		Child:   NewInteger(1),
		Entries: []ChildEntry{{Key: "k", Value: NewCarrier(TInteger)}},
	}}
	if typeBodyConstOK(badEntry) {
		t.Fatal("carrier entry should decline")
	}
}

func TestIsInertConstPayloadArms(t *testing.T) {
	// Micron instance → inert.
	micron := Value{Parent: TAny, Data: MicronPayload{Fields: NewOrderedMap()}}
	if !IsInertConst(micron) {
		t.Fatal("a Micron instance should be inert")
	}
	// nil-backed map → not inert.
	if IsInertConst(Value{Parent: TMap, Data: MapPayload{M: nil}}) {
		t.Fatal("nil map should not be inert")
	}
	// XML element whose attribute value is a carrier → not inert.
	attr := NewOrderedMap()
	attr.Set("x", NewCarrier(TInteger))
	xml := Value{Parent: TAny, Data: XmlElementPayload{Tag: "a", Attr: attr}}
	if IsInertConst(xml) {
		t.Fatal("carrier attribute should decline")
	}
}

func TestFnSigConstOK(t *testing.T) {
	pat := NewCarrier(TInteger)
	info := FnUndefInfo{Sigs: []FnSigSpec{{Params: []FnParam{{Pattern: &pat}}}}}
	if fnSigConstOK(info) {
		t.Fatal("a carrier param pattern should decline")
	}
}

func TestSurfaceConstOK(t *testing.T) {
	if surfaceConstOK(nil) {
		t.Fatal("nil surface should decline")
	}
	if surfaceConstOK(&SurfaceInfo{Type: nil}) {
		t.Fatal("no canonical node should decline")
	}
	if !surfaceConstOK(&SurfaceInfo{Type: TInteger}) {
		t.Fatal("a surface with no required ops should be const-ok")
	}
}

func TestSchemaConstOK(t *testing.T) {
	if schemaConstOK(nil) {
		t.Fatal("nil schema should decline")
	}
	if schemaConstOK(&TypeSchemaInfo{Type: nil}) {
		t.Fatal("no canonical node should decline")
	}
	// A body Word that names no parameter → isParam false, declines.
	if schemaConstOK(&TypeSchemaInfo{Type: TInteger, Body: NewWord("nope")}) {
		t.Fatal("a non-param body word should decline")
	}
	// param bound not const-safe → false.
	if schemaConstOK(&TypeSchemaInfo{Type: TInteger, Body: NewInteger(1),
		Params: []GenParam{{HasBound: true, Bound: NewCarrier(TInteger)}}}) {
		t.Fatal("carrier bound should decline")
	}
	// param default not const-safe → false.
	if schemaConstOK(&TypeSchemaInfo{Type: TInteger, Body: NewInteger(1),
		Params: []GenParam{{HasDefault: true, Default: NewCarrier(TInteger)}}}) {
		t.Fatal("carrier default should decline")
	}
}

func TestIsInertQuotedParen(t *testing.T) {
	// Quoted but not actually a paren-expr payload → AsParenExpr errors.
	bad := Value{Parent: TParenExpr, Data: IntPayload{N: 1}, Quoted: true}
	if isInertQuotedParen(bad) {
		t.Fatal("a non-paren payload should decline")
	}
	// Quoted paren with a carrier token → declines.
	p := NewParenExpr([]Value{NewCarrier(TInteger)})
	p.Quoted = true
	if isInertQuotedParen(p) {
		t.Fatal("a carrier token should decline")
	}
}

func TestIsInertReach(t *testing.T) {
	// Not a reach → false.
	if isInertReach(NewInteger(1)) {
		t.Fatal("an integer is not an inert reach")
	}
	// A reach with a computed segment → false.
	r := NewReach(ReachInfo{Segments: []ReachSeg{{Computed: true}}})
	if isInertReach(r) {
		t.Fatal("a computed segment should decline")
	}
}

func TestInertReachMemberSeam7(t *testing.T) {
	// Not a reach → false.
	if inertReachMember(NewInteger(1)) {
		t.Fatal("an integer is not a reach member")
	}
	// IsReach by parent but the payload is not ReachInfo → AsReach errors.
	if inertReachMember(Value{Parent: TReach, Data: IntPayload{N: 1}}) {
		t.Fatal("a non-reach payload should decline")
	}
	// A reach whose receiver token is a carrier → declines.
	r := NewReach(ReachInfo{Receiver: []Value{NewCarrier(TInteger)}})
	if inertReachMember(r) {
		t.Fatal("a carrier receiver should decline")
	}
}

func TestIsInertConstMemberParenErr(t *testing.T) {
	// A value that IsParenExpr by parent but whose payload is not a
	// ParenExprPayload → AsParenExpr errors → not an inert member.
	bad := Value{Parent: TParenExpr, Data: IntPayload{N: 1}}
	if IsInertConstMember(bad) {
		t.Fatal("a non-paren paren-typed value should not be an inert member")
	}
}

// --- small direct helpers: thenArm / OperandRepushable / RegisterLocal ---

func TestTypeBodyConstOKElements(t *testing.T) {
	// Child ok, but an Elements entry is a carrier → allInert fails.
	v := Value{Data: ChildTypeInfo{Child: NewInteger(1), Elements: []Value{NewCarrier(TInteger)}}}
	if typeBodyConstOK(v) {
		t.Fatal("a carrier element should decline")
	}
}

// --- loop-carried def machinery -----------------------------------------

// --- Finalize / dynamic-apply shape guards ------------------------------

// --- shuffle-word gates -------------------------------------------------

// --- interpMemberInter interp-string error arm --------------------------

// --- tryReturnedClosure early compile failures ----------------------------------

// --- tryReturnedClosure body-compile path -------------------------------

// --- StartFnCompile finish residual paths -------------------------------

// --- quoted-operand compile failure ---------------------------------------------

// --- Finalize residual arms ---------------------------------------------

// TestInactiveConstructorSlots pins the compiler-less fallbacks behind
// the pass-arming constructor slots: both hand out the inactive no-op
// recorder. The live slots are compiler-installed at init; the fallback
// bodies are the post-cut core-only configuration.
func TestInactiveConstructorSlots(t *testing.T) {
	if inactiveEmitStateHook() != TheInactiveEmit {
		t.Fatal("inactiveEmitStateHook must hand out the inactive recorder")
	}
	if inactiveIsolatedEmitHook(TheInactiveEmit) != TheInactiveEmit {
		t.Fatal("inactiveIsolatedEmitHook must hand out the inactive recorder")
	}
}
