package check

// Seam-6a wave B: direct in-package unit tests for the previously
// unreached guard / decline arms in carrier.go (the check-mode compile
// helpers). Per design/TEST-SEAMS.10.md these are driven by direct calls
// with synthetic Values and, where a recorder gate applies, an armed
// EmitState (r.Check.Emit = NewEmitState()). No package globals are
// swapped; every test uses a fresh registry.

import (
	"math/big"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestS6aJoinedElementCarrierNilMapPayload(t *testing.T) {
	m := core.NewMap(core.NewOrderedMap())
	m.Data = core.MapPayload{M: nil}
	if _, ok := joinedElementCarrier(m); ok {
		t.Error("nil-backed MapPayload must decline")
	}
}

func TestS6aJoinedElementCarrierNilParentElem(t *testing.T) {
	lst := core.NewList([]core.Value{core.NewInteger(1), {}})
	if _, ok := joinedElementCarrier(lst); ok {
		t.Error("a nil-Parent element must decline the join")
	}
}

// --- UnionCarrierForType ----------------------------------------------------

func TestS6aUnionCarrierForTypeNil(t *testing.T) {
	if _, ok := core.UnionCarrierForType(nil); ok {
		t.Error("core.UnionCarrierForType(nil) must decline")
	}
}

// --- DataListElemTypeFromValue ----------------------------------------------

func TestS6aDataListElemTypeMapMixedBranches(t *testing.T) {
	m := core.NewOrderedMap()
	m.Set("a", core.NewInteger(1))
	m.Set("b", core.NewList([]core.Value{core.NewInteger(2)}))
	m.Set("c", core.NewString("x")) // never reached: the walk breaks at Any
	if got := DataListElemTypeFromValue(core.NewMap(m)); !got.Equal(core.TAny) {
		t.Errorf("cross-branch map values: got %v, want Any", got)
	}
}

func TestS6aScalarFoldOperandBigNumPayloads(t *testing.T) {
	if !ScalarFoldOperand(core.NewBigInteger(big.NewInt(5))) {
		t.Error("a BigInt payload is a scalar fold operand")
	}
	if ScalarFoldOperand(core.NewList([]core.Value{core.NewCarrier(core.TInteger)})) {
		t.Error("a carrier-bearing list is not a scalar fold operand")
	}
}

// A CONTAINER never folds, however inert its interior — `eq` over one is
// container identity, which check mode's copies cannot carry. Pinned after
// a live false positive: `(m get 'a') eq (m get 'a')` folded to false over
// two clones of one stored list and pruned the live branch.
func TestS6aScalarFoldOperandDeclinesContainers(t *testing.T) {
	dataList := core.NewList([]core.Value{core.NewInteger(1)})
	if !core.IsInertConst(dataList) {
		t.Fatal("fixture: a data-only list must be an inert const — otherwise this test proves nothing")
	}
	if ScalarFoldOperand(dataList) {
		t.Error("a data-only list is an inert const but must NOT fold (container identity)")
	}
	m := core.NewOrderedMap()
	m.Set("a", core.NewInteger(1))
	dataMap := core.NewMap(m)
	if !core.IsInertConst(dataMap) {
		t.Fatal("fixture: a data-only map must be an inert const")
	}
	if ScalarFoldOperand(dataMap) {
		t.Error("a data-only map is an inert const but must NOT fold")
	}
	// The tail decline: a payload-bearing value that is neither a
	// container nor a foldable scalar (a Word token). It used to be
	// reached by the carrier-bearing list above, which now exits at the
	// container guard.
	if ScalarFoldOperand(core.NewWord("x")) {
		t.Error("a Word token is not a scalar fold operand")
	}
	// The positive half of the contract: scalars — including the
	// immutable structured ones, which compare structurally — keep
	// folding, so `(n eq 0)` with a const n still reads as a known bool.
	for _, v := range []core.Value{
		core.NewInteger(0), core.NewString("s"), core.NewBoolean(false), core.NewAtom("a"),
	} {
		if !ScalarFoldOperand(v) {
			t.Errorf("%s must remain a scalar fold operand", v.Parent)
		}
	}
}

// --- isConcreteContainerReturn ----------------------------------------------

func TestS6aIsConcreteContainerReturn(t *testing.T) {
	if isConcreteContainerReturn(core.NewDynamicCarrier(core.TList)) {
		t.Error("dynamic values are not concrete container returns")
	}
	if isConcreteContainerReturn(core.Value{}) {
		t.Error("nil-Parent values are not concrete container returns")
	}
	if !isConcreteContainerReturn(core.NewCarrier(core.TList)) {
		t.Error("a strict List carrier is a concrete container return")
	}
}

// --- applyGradualContagion ----------------------------------------------------

func TestS6aGradualContagionStrictDedup(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Check.Strict = true
	pos := core.SrcPos{Row: 3, Col: 7}
	r.Check.Diagnostics = []core.CheckDiagnostic{{
		Code: "dynamic_dispatch", Word: "s6aword", Row: 3, Col: 7,
	}}
	out := applyGradualContagion(r, "s6aword",
		[]core.Value{core.NewDynamicCarrier(core.TAny)},
		[]core.Value{core.NewCarrier(core.TInteger)}, pos, false)
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("duplicate dynamic_dispatch diagnostic added: %d", len(r.Check.Diagnostics))
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Errorf("contagion should mark the result dynamic: %+v", out)
	}
}

// registerIslandWord registers a word with the given compile effect and
// arg count and returns the REGISTERED sig pointer (identity matters).
func registerIslandWord(t *testing.T, r *core.Registry, name string, effect core.CompileEffect, argc int, barrier int) *core.Signature {
	t.Helper()
	args := make([]*core.Type, argc)
	for i := range args {
		args[i] = core.TAny
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: name,
		Signatures: []core.Signature{{
			Args:          args,
			CompileEffect: effect,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(0)}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: barrier,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration of %s: %v", name, err)
	}
	return &r.Lookup(name).Signatures[0]
}

func TestS6aBodyFreeForFallbackLiteralAndTypeWords(t *testing.T) {
	r := newTestRegistry(t)
	if !BodyFreeForFallback(r, core.NewList([]core.Value{core.NewWord("true"), core.NewWord("false")})) {
		t.Error("bare literal words are VM-resolvable")
	}
	if !BodyFreeForFallback(r, core.NewList([]core.Value{core.NewWord("Integer")})) {
		t.Error("type-name words are VM-resolvable")
	}
	if BodyFreeForFallback(r, core.NewList([]core.Value{core.NewWord("s6a_no_such")})) {
		t.Error("an unresolvable word must fail the body scan")
	}
}

// --- disjunctPartitionReturns ------------------------------------------------------

func strictDisjunct(alts ...core.Value) core.Value {
	d := core.NewDisjunct(alts)
	d.Carrier = true
	return d
}

func TestS6aDisjunctPartitionUnknownWordDeclines(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	d := strictDisjunct(core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString))
	if _, ok := disjunctPartitionReturns(r, "s6a_unknown_word", []core.Value{d}, core.SrcPos{}); ok {
		t.Error("an unknown word must decline the partition")
	}
}

func TestS6aDisjunctPartitionComboCapDeclines(t *testing.T) {
	r := newTestRegistry(t)
	registerIslandWord(t, r, "s6apart", 0, 1, -1)
	done := r.Check.Begin()
	defer done()
	alts := make([]core.Value, disjunctPartitionCap+1)
	for i := range alts {
		alts[i] = core.NewInteger(int64(i))
	}
	if _, ok := disjunctPartitionReturns(r, "s6apart", []core.Value{strictDisjunct(alts...)}, core.SrcPos{}); ok {
		t.Error("a cross product over the cap must decline")
	}
}

func TestS6aDisjunctPartitionUnannotatedOverloadDeclines(t *testing.T) {
	r := newTestRegistry(t)
	// A word with NO Returns and NO ReturnsFn: the default arm declines.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s6apartu",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
			BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	done := r.Check.Begin()
	defer done()
	d := strictDisjunct(core.NewTypeLiteral(core.TInteger), core.NewTypeLiteral(core.TString))
	if _, ok := disjunctPartitionReturns(r, "s6apartu", []core.Value{d}, core.SrcPos{}); ok {
		t.Error("an unannotated overload must decline the whole partition")
	}
}

// --- disjunctCombos / joinReturnRows / dynamicReachableValueReturns ----------------

func TestS6aDisjunctCombosOverLimit(t *testing.T) {
	d := strictDisjunct(core.NewInteger(1), core.NewInteger(2), core.NewInteger(3), core.NewInteger(4))
	if _, ok := disjunctCombos([]core.Value{d}, 3); ok {
		t.Error("a product beyond the limit must decline")
	}
}

func TestS6aJoinReturnRowsArityMismatch(t *testing.T) {
	rows := [][]core.Value{
		{core.NewCarrier(core.TInteger)},
		{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)},
	}
	if _, ok := joinReturnRows(rows); ok {
		t.Error("mismatched return arity must decline")
	}
}

func TestS6aDynamicReachableValueReturnsUnknownWord(t *testing.T) {
	r := newTestRegistry(t)
	if got := dynamicReachableValueReturns(r, "s6a_missing", nil); got != nil {
		t.Errorf("unknown word: got %v, want nil", got)
	}
}

// --- narrowDynamicUses / slotIsPolymorphic -----------------------------------------

func TestS6aNarrowDynamicUsesGuards(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	// Unbound origin name: skipped.
	a := core.NewDynamicCarrier(core.TAny)
	a.SetDynFrom("s6a_unbound")
	sig := &core.Signature{Args: []*core.Type{core.TInteger}}
	narrowDynamicUses(r, "w", sig, []core.Value{a})
	if _, ok := r.Defs.Top("s6a_unbound"); ok {
		t.Error("an unbound origin must not gain a binding")
	}

	// Bound but no longer dynamic: skipped.
	r.Defs.Push("s6a_rebound", core.NewInteger(3))
	b := core.NewDynamicCarrier(core.TAny)
	b.SetDynFrom("s6a_rebound")
	narrowDynamicUses(r, "w", sig, []core.Value{b})
	if got, _ := r.Defs.Top("s6a_rebound"); got.Dynamic {
		t.Error("a since-rebound name must stay untouched")
	}

	// Nil slot type: skipped.
	r.Defs.Push("s6a_dyn", core.NewDynamicCarrier(core.TAny))
	c := core.NewDynamicCarrier(core.TAny)
	c.SetDynFrom("s6a_dyn")
	nilSlotSig := &core.Signature{Args: []*core.Type{nil}}
	depth := r.Defs.Depth("s6a_dyn")
	narrowDynamicUses(r, "w", nilSlotSig, []core.Value{c})
	if r.Defs.Depth("s6a_dyn") != depth {
		t.Error("a nil slot type must not push a narrowing")
	}
}

func TestS6aSlotIsPolymorphicEmptyWord(t *testing.T) {
	r := newTestRegistry(t)
	if slotIsPolymorphic(r, "", nil, 0, core.TInteger) {
		t.Error("an empty word is never polymorphic")
	}
}

// --- ReturnsFreshInstance -----------------------------------------------------------

func TestS6aReturnsFreshInstanceParameterlessSchemaNonConcreteData(t *testing.T) {
	r := newTestRegistry(t)
	fields := core.NewOrderedMap()
	fields.Set("x", core.NewTypeLiteral(core.TInteger))
	body := core.NewRecordType(fields)
	schemaT := r.Types.MintType("S6aRec", core.TIdeal)
	schema := core.NewTypeSchema(schemaT, &core.TypeSchemaInfo{Kind: core.SchemaRecord, Body: body})

	fn := ReturnsFreshInstance(0)
	out := fn([]core.Value{schema, core.NewCarrier(core.TMap)}, r)
	if len(out) != 1 {
		t.Fatalf("out len = %d, want 1", len(out))
	}
	if !out[0].Parent.Equal(core.TMap) || !out[0].Dynamic {
		t.Errorf("want a dynamic Map carrier riding the field schema, got %+v", out[0])
	}
	if _, ok := out[0].Data.(core.RecordTypeInfo); !ok {
		t.Errorf("want RecordTypeInfo payload, got %T", out[0].Data)
	}
}

// --- core.RecordTypedDefMake / core.objectMakeSig ---------------------------------------------

// --- core.CommonAncestorType / core.isNoneArm -------------------------------------------------

func TestS6aCommonAncestorTypeNil(t *testing.T) {
	if got := core.CommonAncestorType(nil, core.TInteger); !got.Equal(core.TAny) {
		t.Errorf("nil operand: got %v, want Any", got)
	}
}

// --- core.JoinCarriersInner width cap -----------------------------------------------------

func TestS6aJoinCarriersWidthCapCollapses(t *testing.T) {
	mk := func(lo, hi int64) core.Value {
		var alts []core.Value
		for i := lo; i <= hi; i++ {
			alts = append(alts, core.NewInteger(i))
		}
		return strictDisjunct(alts...)
	}
	a := mk(1, 5)
	b := mk(6, 10)
	got := core.JoinCarriersInner(a, b)
	if !got.Carrier || !got.Parent.Equal(core.TInteger) || core.IsDisjunct(got) {
		t.Errorf("10 alternatives must collapse to a plain Integer carrier, got %+v", got)
	}
}

// --- core.JoinCarriersInner None arms -----------------------------------------------------

func TestS6aRunCarrierBodyErrorPath(t *testing.T) {
	r := newTestRegistry(t)
	body := core.NewList([]core.Value{core.NewWord("s6a_undefined_word")})
	stk, adds := core.RunCarrierBodyWithDefs(r, body)
	if stk != nil {
		t.Errorf("an erroring body yields a nil stack, got %v", stk)
	}
	if len(adds) != 0 {
		t.Errorf("an erroring body yields no def additions, got %v", adds)
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "branch_error" {
			found = true
		}
	}
	if !found {
		t.Error("expected a branch_error diagnostic")
	}
}

// --- core.extractGuardClauses ---------------------------------------------------------------

// --- core.ApplyGuardNarrowing / core.ApplyComplementNarrowing -------------------------------------

func TestS6aApplyGuardNarrowingInactive(t *testing.T) {
	r := newTestRegistry(t)
	restore := core.ApplyGuardNarrowing(r, core.NewList([]core.Value{core.NewWord("x")}))
	restore() // noop
}

func TestS6aApplyGuardNarrowingRedundantGuardDedup(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Defs.Push("s6ax", core.NewCarrier(core.TInteger))
	cond := core.NewList([]core.Value{core.NewWord("s6ax"), core.NewWord("is"), core.NewTypeLiteral(core.TInteger)})

	restore := core.ApplyGuardNarrowing(r, cond)
	restore()
	n1 := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "redundant_guard" {
			n1++
		}
	}
	if n1 != 1 {
		t.Fatalf("first narrowing: %d redundant_guard diags, want 1", n1)
	}

	restore = core.ApplyGuardNarrowing(r, cond) // second: dedup arm
	restore()
	n2 := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "redundant_guard" {
			n2++
		}
	}
	if n2 != 1 {
		t.Errorf("second narrowing must dedup, got %d diags", n2)
	}
}

func TestS6aApplyComplementNarrowingGuards(t *testing.T) {
	// Inactive check state: noop.
	r := newTestRegistry(t)
	restore := core.ApplyComplementNarrowing(r, core.NewList([]core.Value{core.NewWord("x")}))
	restore()

	// Active, but the clause's name is unbound: skipped.
	done := r.Check.Begin()
	defer done()
	cond := core.NewList([]core.Value{core.NewWord("s6a_nobind"), core.NewWord("is"), core.NewTypeLiteral(core.TInteger)})
	restore = core.ApplyComplementNarrowing(r, cond)
	restore()
	if _, ok := r.Defs.Top("s6a_nobind"); ok {
		t.Error("an unbound clause name must stay unbound")
	}
}

// --- refineRecursiveSummary ---------------------------------------------------------------

func TestS6aRefineRecursiveSummaryJoinsRounds(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Check.FnSummaries = map[string][]core.Value{}
	result := []core.Value{core.NewCarrier(core.TInteger)}
	got := refineRecursiveSummary(r, "s6akey", 0, result, func() []core.Value {
		return []core.Value{core.NewCarrier(core.TString)}
	})
	if len(got) != 1 {
		t.Fatalf("summary len = %d, want 1", len(got))
	}
	// The refined hypothesis must cover BOTH rounds' types.
	if got[0].Parent.Equal(core.TInteger) && !core.IsDisjunct(got[0]) {
		t.Errorf("summary did not widen past round 1: %+v", got[0])
	}
}

// --- RunFnBodyOnce / AnalyseFnBody -----------------------------------------------------------

func TestS6aIsDeferredWordListShapes(t *testing.T) {
	// Parent=TList with a non-list payload: AsList fails.
	bad := core.NewValueRaw(core.TList, core.IntPayload{N: 1})
	bad.Eval = true
	if IsDeferredWordList(bad) {
		t.Error("a non-list payload must decline")
	}

	// A nested list containing a word: deferred.
	inner := core.NewList([]core.Value{core.NewWord("w")})
	inner.Eval = true
	outer := core.NewList([]core.Value{inner})
	outer.Eval = true
	if !IsDeferredWordList(outer) {
		t.Error("a nested word-bearing list is deferred")
	}

	// No words anywhere: not deferred.
	plain := core.NewList([]core.Value{core.NewInteger(1), core.NewList([]core.Value{core.NewInteger(2)})})
	plain.Eval = true
	if IsDeferredWordList(plain) {
		t.Error("a word-free list is not deferred")
	}
}
