package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestSetUnitParamTypesGuard(t *testing.T) {
	NewEmitState().SetUnitParamTypes(-1, nil, nil) // out-of-range → no-op
}

func TestStartFnCompileEmptyParamNames(t *testing.T) {
	es := NewEmitState()
	a1, a2 := core.NewCarrier(core.TInteger), core.NewCarrier(core.TInteger)
	unit, finish, ok := es.StartFnCompile("k", "fn", nil, []core.Value{a1, a2}, nil, nil, nil, false, core.SrcPos{})
	if !ok || finish == nil {
		t.Fatalf("StartFnCompile should open a fresh unit: ok=%v", ok)
	}
	finish(nil) // close cleanly with an empty body residual
	_ = unit
}

func TestRecordBranchRefusals(t *testing.T) {
	// condFrag present but empty stack → refuse.
	es := NewEmitState()
	es.RecordBranch(core.BranchRecord{CondFrag: &EmitFragment{}, CondStk: nil})
	if es.Compilable {
		t.Fatal("empty condition body should refuse")
	}
	// condFrag result unresolvable.
	es = NewEmitState()
	es.RecordBranch(core.BranchRecord{CondFrag: &EmitFragment{}, CondStk: []core.Value{carrierVal(core.TInteger)}})
	if es.Compilable {
		t.Fatal("unresolvable condition result should refuse")
	}
	// default cond unresolvable.
	es = NewEmitState()
	es.RecordBranch(core.BranchRecord{Cond: carrierVal(core.TInteger)})
	if es.Compilable {
		t.Fatal("unresolvable condition should refuse")
	}
	// const-cond with an uncaptured then arm.
	es = NewEmitState()
	tru := true
	es.RecordBranch(core.BranchRecord{ConstCond: &tru, Then: nil})
	if es.Compilable {
		t.Fatal("uncaptured taken arm should refuse")
	}
	// then VALUE unresolvable (else-form).
	es = NewEmitState()
	es.RecordBranch(core.BranchRecord{Cond: core.NewInteger(1), HasElse: true, ThenValue: ptrVal(carrierVal(core.TInteger))})
	if es.Compilable {
		t.Fatal("unresolvable then value should refuse")
	}
	// else VALUE unresolvable.
	es = NewEmitState()
	es.RecordBranch(core.BranchRecord{Cond: core.NewInteger(1), HasElse: true, ThenValue: ptrVal(core.NewInteger(2)), ElsValue: ptrVal(carrierVal(core.TInteger))})
	if es.Compilable {
		t.Fatal("unresolvable else value should refuse")
	}
}

func TestNoteMemberFnReadGuards(t *testing.T) {
	NewEmitState().NoteMemberFnRead("", core.Value{}) // empty id → no-op
	inactiveEmitState().NoteMemberFnRead("id", core.Value{})
	es := NewEmitState()
	es.NoteMemberFnRead("id", core.Value{})
	if !es.MemberFnRead("id") {
		t.Fatal("a noted member-fn read should be readable")
	}
}

func TestRecordInterpRefusals(t *testing.T) {
	es := NewEmitState()
	if es.RecordInterp(nil, nil, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("no holes should decline")
	}
}

func TestRecordClosureCallRefusals(t *testing.T) {
	es := NewEmitState()
	// nil sig → declines.
	if es.RecordClosureCall("w", nil, nil, 0, 0, nil, nil, nil, nil, false, core.SrcPos{}) {
		t.Fatal("nil sig should decline")
	}
	// an unresolvable non-body operand → declines.
	sig := &core.Signature{}
	if es.RecordClosureCall("w", sig, []core.Value{carrierVal(core.TInteger)}, -1, 0, nil, nil, []core.Value{core.NewInteger(0)}, nil, false, core.SrcPos{}) {
		t.Fatal("unresolvable closure-call operand should decline")
	}
}

func TestRecordDynMethodGuards(t *testing.T) {
	if inactiveEmitState().RecordDynMethod(core.NewDynamicCarrier(core.TFunction), nil, nil, "m", core.SrcPos{}) {
		t.Fatal("inactive RecordDynMethod should decline")
	}
	es := NewEmitState()
	// fn unresolvable → declines.
	if es.RecordDynMethod(carrierVal(core.TFunction), nil, nil, "m", core.SrcPos{}) {
		t.Fatal("unresolvable method value should decline")
	}
	// fn resolves, an arg is unresolvable → declines.
	fn := core.NewDynamicCarrier(core.TFunction)
	seedProduced(es, fn, 1)
	if es.RecordDynMethod(fn, []core.Value{carrierVal(core.TInteger)}, nil, "m", core.SrcPos{}) {
		t.Fatal("unresolvable method arg should decline")
	}
}

func TestDynInputsProven(t *testing.T) {
	// arity mismatch → false.
	es := NewEmitState()
	sig := &core.Signature{CompileEffect: core.CompileRunsBodyIsolated, Args: []*core.Type{core.TInteger}}
	if es.DynInputsProven(sig, nil) {
		t.Fatal("arity mismatch should not be proven")
	}
	// dynamic operand with a concrete parent but a nil sig position → false.
	sigNil := &core.Signature{CompileEffect: core.CompileRunsBodyIsolated, Args: []*core.Type{nil}}
	if es.DynInputsProven(sigNil, []core.Value{core.NewDynamicCarrier(core.TInteger)}) {
		t.Fatal("nil sig position should not be proven")
	}
	// dynamic operand whose parent does not conform to the sig position → false.
	sigStr := &core.Signature{CompileEffect: core.CompileRunsBodyIsolated, Args: []*core.Type{core.TString}}
	if es.DynInputsProven(sigStr, []core.Value{core.NewDynamicCarrier(core.TInteger)}) {
		t.Fatal("non-conforming dynamic operand should not be proven")
	}
	// a genuinely-WIDENED gradual operand (dynamic Any) is exactly the
	// unproven case the guard defends — refused even on a conforming sig.
	sigAny := &core.Signature{CompileEffect: core.CompileRunsBodyIsolated, Args: []*core.Type{core.TAny}}
	if es.DynInputsProven(sigAny, []core.Value{core.NewDynamicCarrier(core.TAny)}) {
		t.Fatal("a widened dynamic(Any) operand must refuse the proof")
	}
}

func TestOperandRepushableBareNode(t *testing.T) {
	es := NewEmitState()
	// A bare type node with an ID is freely re-pushable (type operand).
	if !es.OperandRepushable(core.NewTypeLiteral(core.TInteger)) {
		t.Fatal("a bare type node should be re-pushable")
	}
}

// stampCompiledRef writes onto the first own boru body sig and reports true; a fn
// value with no own boru body sig (all Go / all fallback) reports false.
func TestStampCompiledRef(t *testing.T) {
	ref := &CompiledFnRef{Prog: &Program{}, Unit: 0}
	boruFd := core.FnDefInfo{Signatures: []core.Signature{{Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}}}}
	if !StampCompiledRef(boruFd, ref) {
		t.Fatal("a boru body sig must accept the stamp")
	}
	if CompiledRef(&boruFd.Signatures[0]) != ref {
		t.Fatal("stamp did not land on the sig")
	}
	// A fallback-only sig is skipped; a Go sig has no *BoruImpl → no stamp.
	goFd := core.FnDefInfo{Signatures: []core.Signature{
		{Fallback: true, Impl: &core.BoruImpl{}},
		{Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
			return nil, nil
		})},
	}}
	if StampCompiledRef(goFd, ref) {
		t.Fatal("a fn value with no own boru body sig must not be stamped")
	}
}

// CompiledRef reads the durable unit reference off a boru body sig, and returns
// nil for a Go sig (the callback-invocation seam's "no compiled unit" signal).
func TestSignatureCompiledRef(t *testing.T) {
	ref := &CompiledFnRef{Prog: &Program{}, Unit: 3}
	boruSig := &core.Signature{Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}, Compiled: ref}}
	if got := CompiledRef(boruSig); got != ref {
		t.Fatalf("boru-body CompiledRef = %v, want the stamped ref", got)
	}
	// An un-armed boru body reports no unit.
	bareSig := &core.Signature{Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}}
	if got := CompiledRef(bareSig); got != nil {
		t.Fatalf("un-armed CompiledRef = %v, want nil", got)
	}
	// A Go sig has no boru body at all.
	goSig := &core.Signature{Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, nil
	})}
	if got := CompiledRef(goSig); got != nil {
		t.Fatalf("Go-sig CompiledRef = %v, want nil", got)
	}
}

func TestDisassembleTailArms(t *testing.T) {
	inner := &Program{}
	p := &Program{
		Fns:          []CompiledFn{{Name: "g", NParams: 0, NLocals: 0}},
		storedFnRefs: []*CompiledFnRef{{}, {Prog: inner}},
	}
	out := p.Disassemble()
	if !strings.Contains(out, "fn f0 g/0") {
		t.Errorf("empty-locals fn header missing:\n%s", out)
	}
	if p.StoredRefCount() != 2 || p.StoredRefStampedCount() != 1 {
		t.Errorf("stored-ref counts = %d/%d, want 2/1",
			p.StoredRefCount(), p.StoredRefStampedCount())
	}
}
