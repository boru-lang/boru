package check

// Standalone-gate tests for carrier.go's dispatch-modelling helpers whose
// remaining arms only the merged gate reached: the element-type read over a
// collection holding a type VALUE, gradual contagion's widen-to-Any arm and
// the reachability probe behind it, the fn-local body scan's value-read
// report, the fn-predicate hazard over a refinement whose bound only the run
// knows, and the fresh-instance model of a NAMED constructor target.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// gsGoImpl is a trivial native implementation for fixture signatures —
// check-mode analysis never invokes it.
func gsGoImpl() *core.GoImpl {
	return core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return args, nil
	})
}

// A type literal among a collection's values is a type VALUE, no value of
// its Parent (NUR323): the element type a body sees is unknown (Any), for a
// map's values and a list's elements alike. The same collections holding
// plain values keep their common type.
func TestGateStandaloneElemTypeOverTypeValues(t *testing.T) {
	plainMap := mapOf("a", core.NewInteger(1), "b", core.NewInteger(2))
	if got := DataListElemTypeFromValue(plainMap); !got.Equal(core.TInteger) {
		t.Errorf("a map of Integers reads Integer, got %v", got)
	}
	typeMap := mapOf("t", core.NewTypeLiteral(core.TString), "a", core.NewInteger(1))
	if got := DataListElemTypeFromValue(typeMap); !got.Equal(core.TAny) {
		t.Errorf("a map holding a type value reads Any, got %v", got)
	}
	plainList := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	if got := DataListElemTypeFromValue(plainList); !got.Equal(core.TInteger) {
		t.Errorf("a list of Integers reads Integer, got %v", got)
	}
	typeList := core.NewList([]core.Value{core.NewInteger(1), core.NewTypeLiteral(core.TInteger)})
	if got := DataListElemTypeFromValue(typeList); !got.Equal(core.TAny) {
		t.Errorf("a list holding a type value reads Any, got %v", got)
	}
}

// gsRegisterWiden registers `zgswidenq`, three same-arity overloads: a List
// return and a no-single-return sibling (a ReturnsFn) that both take
// (Any, Integer)-shaped operands, and a (String, String) overload a
// concrete Integer second operand never reaches.
func gsRegisterWiden(t *testing.T, r *core.Registry) {
	t.Helper()
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zgswidenq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TAny, core.TInteger}, Returns: []*core.Type{core.TList}, BarrierPos: 2, Impl: gsGoImpl()},
			{Args: []*core.Type{core.TNode, core.TInteger}, BarrierPos: 2, Impl: gsGoImpl(),
				ReturnsFn: func([]core.Value, *core.Registry) []core.Value {
					return []core.Value{core.NewDynamicCarrier(core.TAny)}
				}},
			{Args: []*core.Type{core.TString, core.TString}, Returns: []*core.Type{core.TInteger}, BarrierPos: 2, Impl: gsGoImpl()},
		},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register zgswidenq: %v", err)
	}
}

// reachableUnknownReturnSibling: two or more reached same-arity overloads,
// one declaring no single return, is the case dynamicReachableReturns
// abandons for a partially-known call. An overload a concrete operand
// misses is skipped; one reached overload alone, an unknown word and a
// single-signature word are never that case.
func TestGateStandaloneReachableUnknownReturnSibling(t *testing.T) {
	r := newTestRegistry(t)
	gsRegisterWiden(t, r)
	dyn := core.NewDynamicCarrier(core.TAny)

	if !reachableUnknownReturnSibling(r, "zgswidenq", []core.Value{dyn, core.NewInteger(1)}) {
		t.Error("the List and the ReturnsFn overloads both reach (dyn, Integer): an unknown-return sibling")
	}
	// A String second operand reaches the (String, String) overload alone.
	if reachableUnknownReturnSibling(r, "zgswidenq", []core.Value{dyn, core.NewString("s")}) {
		t.Error("a single reached overload is not the unknown-sibling case")
	}
	if reachableUnknownReturnSibling(r, "zgsnosuchq", []core.Value{dyn}) {
		t.Error("an unknown word has no siblings")
	}
	r.RegisterNativeFunc(core.NativeFunc{Name: "zgsmonoq", Signatures: []core.Signature{
		{Args: []*core.Type{core.TAny}, BarrierPos: 1, Impl: gsGoImpl()},
	}})
	if reachableUnknownReturnSibling(r, "zgsmonoq", []core.Value{dyn}) {
		t.Error("a single-signature word has no siblings")
	}
}

// applyGradualContagion's widen-to-Any arm: a dynamic operand reaches the
// List overload AND a sibling with no single declared return, so the
// committed List is only one of the run's possible result types — the
// strict container keep does not apply and the result widens to a FRESH
// dynamic(Any). Where the operands reach one overload only, the concrete
// List return stays strict (the pairing form).
func TestGateStandaloneContagionWidensUnknownSibling(t *testing.T) {
	r := newTestRegistry(t)
	gsRegisterWiden(t, r)
	pos := core.SrcPos{Row: 3, Col: 1}

	committed := core.NewCarrier(core.TList)
	out := applyGradualContagion(r, "zgswidenq",
		[]core.Value{core.NewDynamicCarrier(core.TAny), core.NewInteger(1)}, []core.Value{committed}, pos, false)
	if len(out) != 1 || !core.IsDynamic(&out[0]) || !core.IsCarrier(&out[0]) || !core.ParentOf(&out[0]).Equal(core.TAny) {
		t.Fatalf("widened result = %#v, want one dynamic(Any) carrier", out)
	}
	if core.IDOf(&out[0]) == core.IDOf(&committed) {
		t.Error("the widening mints a fresh identity")
	}

	single := core.NewCarrier(core.TList)
	out = applyGradualContagion(r, "zgswidenq",
		[]core.Value{core.NewDynamicCarrier(core.TAny), core.NewString("s")}, []core.Value{single}, pos, false)
	if len(out) != 1 || core.IsDynamic(&out[0]) || !core.ParentOf(&out[0]).Equal(core.TList) {
		t.Errorf("one reachable overload keeps the concrete List strict, got %#v", out)
	}
}

// BodyRefsFnLocalFns reports a fn-local fn named in a code body, and whether
// any reference is a VALUE read (`f/v`, `f/u`): such a read resolves to the
// fn value the closure would return as data where the interpreter
// dispatches it, so the caller keeps the compile failure. A plain
// dispatching reference names the fn without a value read.
func TestGateStandaloneBodyRefsFnLocalValueRead(t *testing.T) {
	r := bsRegistry(t)
	r.PushFnBaseline(r.Defs.Snapshot()) // zgsstep unbound: baseline depth 0
	defer r.PopFnBaseline()
	r.Defs.Push("zgsstep", bsFn(core.NewInteger(1)))
	defer r.Defs.Pop("zgsstep")
	sig := bsBodySig(core.CompileFallbackBody, nil)

	names, valueRead := BodyRefsFnLocalFns(r, sig, []core.Value{core.NewList([]core.Value{core.NewWord("zgsstep")})})
	if len(names) != 1 || names[0] != "zgsstep" || valueRead {
		t.Errorf("a dispatching reference: names %v, valueRead %v — want [zgsstep], false", names, valueRead)
	}
	for name, ref := range map[string]core.Value{
		"/v": core.NewWordRef("zgsstep"),
		"/u": core.NewWordUsurp("zgsstep", false),
	} {
		names, valueRead = BodyRefsFnLocalFns(r, sig, []core.Value{core.NewList([]core.Value{ref})})
		if len(names) != 1 || names[0] != "zgsstep" || !valueRead {
			t.Errorf("a %s reference: names %v, valueRead %v — want [zgsstep], true", name, names, valueRead)
		}
	}
}

// FnPredicateOverloadHazard over a refinement whose bound only the run
// knows (`def T (Integer gt (size s))`, NUR308): its check-mode match
// admits every value, the same leniency as a predicate type, so with two
// reachable arms the overload set is a hazard — whether the refinement is
// the slot's type node or the param's pattern. A refinement over a
// compile-time-known bound matches self-contained: no hazard.
func TestGateStandaloneHazardUnknownRefinement(t *testing.T) {
	r := newTestRegistry(t)
	unknownBound := func() core.Value { return core.NewDepScalar(core.DepGT, core.NewCarrier(core.TInteger)) }
	sibling := core.Signature{Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: gsGoImpl()}

	unknownNode := r.Types.MintType("ZgsRunBound", core.TInteger)
	unknownNode.SetTypeBody(unknownBound())
	knownNode := r.Types.MintType("ZgsKnownBound", core.TInteger)
	knownNode.SetTypeBody(core.NewDepScalar(core.DepGT, core.NewInteger(3)))
	pattern := unknownBound()

	r.RegisterNativeFunc(core.NativeFunc{Name: "zgsrunnodeq", Signatures: []core.Signature{
		{Args: []*core.Type{unknownNode}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: gsGoImpl()},
		sibling,
	}})
	r.RegisterNativeFunc(core.NativeFunc{Name: "zgsknownnodeq", Signatures: []core.Signature{
		{Args: []*core.Type{knownNode}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: gsGoImpl()},
		sibling,
	}})
	r.RegisterNativeFunc(core.NativeFunc{Name: "zgsrunpatq", Signatures: []core.Signature{
		{Params: []core.FnParam{{Type: core.TInteger, Pattern: &pattern}}, Returns: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: gsGoImpl()},
		{Args: []*core.Type{core.TNumber}, Returns: []*core.Type{core.TNumber}, BarrierPos: 1, Impl: gsGoImpl()},
	}})
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}

	dynInt := []core.Value{core.NewDynamicCarrier(core.TInteger)}
	if !FnPredicateOverloadHazard(r, "zgsrunnodeq", dynInt) {
		t.Error("a run-bound refinement slot type beside a reachable sibling is a hazard")
	}
	if !FnPredicateOverloadHazard(r, "zgsrunpatq", dynInt) {
		t.Error("a run-bound refinement PATTERN beside a reachable sibling is a hazard")
	}
	if FnPredicateOverloadHazard(r, "zgsknownnodeq", dynInt) {
		t.Error("a refinement over a known bound matches self-contained: no hazard")
	}
}

// ReturnsFreshInstance over a NAMED constructor target — the minted node a
// name evaluates to (the Stage 2 flip): the structure the model inspects is
// the node's recorded declaration, so a record node's field schema rides
// the instance carrier exactly as an inline record body's does. A bare node
// with no recorded declaration is made as itself (a strict carrier of the
// node).
func TestGateStandaloneFreshInstanceNamedTarget(t *testing.T) {
	r := zcaRegistry(t)
	fields := core.NewOrderedMap()
	fields.Set("bits", core.NewTypeLiteral(core.TInteger))
	named := r.Types.MintType("ZgsRec", core.TMap)
	named.SetTypeBody(core.NewRecordType(fields))

	out := ReturnsFreshInstance(0)([]core.Value{core.NewTypeLiteral(named), core.NewMap(core.NewOrderedMap())}, r)
	if len(out) != 1 || !core.IsCarrier(&out[0]) || !core.IsDynamic(&out[0]) {
		t.Fatalf("named record target = %#v, want one dynamic instance carrier", out)
	}
	rt, ok := core.DataOf(&out[0]).(core.RecordTypeInfo)
	if !ok || rt.Fields == nil {
		t.Fatalf("instance carrier Data = %T, want the node's declared RecordTypeInfo", core.DataOf(&out[0]))
	}
	if _, has := rt.Fields.Get("bits"); !has {
		t.Error("the declared field schema must ride the instance carrier")
	}

	bare := r.Types.MintType("ZgsBare", core.TMap)
	out = ReturnsFreshInstance(0)([]core.Value{core.NewTypeLiteral(bare)}, r)
	if len(out) != 1 || core.IsDynamic(&out[0]) || core.DataOf(&out[0]) != nil || !core.ParentOf(&out[0]).Equal(bare) {
		t.Errorf("a declaration-less node is made as itself, got %#v", out)
	}
}
