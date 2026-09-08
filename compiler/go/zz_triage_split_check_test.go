package compiler

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

func TestS6aTryFoldScalarConstNondeterministicDeclines(t *testing.T) {
	r := newTestRegistry(t)
	sig := statefulSig(core.CompileScalarFold, func(n int) []core.Value {
		return []core.Value{core.NewInteger(int64(n))}
	})
	if _, ok := tryFoldScalarConst(r, sig, []core.Value{core.NewInteger(1)}); ok {
		t.Error("a nondeterministic handler must not const-fold")
	}
}

func TestS6aTryFoldScalarConstNonInertResultDeclines(t *testing.T) {
	r := newTestRegistry(t)
	sig := statefulSig(core.CompileScalarFold, func(int) []core.Value {
		return []core.Value{core.NewTypeLiteral(core.TInteger)} // bare type node: not inert
	})
	if _, ok := tryFoldScalarConst(r, sig, []core.Value{core.NewInteger(1)}); ok {
		t.Error("a non-inert-const result must not fold")
	}
	// Positive twin: a deterministic inert scalar folds.
	sig2 := statefulSig(core.CompileScalarFold, func(int) []core.Value {
		return []core.Value{core.NewInteger(7)}
	})
	folded, ok := tryFoldScalarConst(r, sig2, []core.Value{core.NewInteger(1)})
	if !ok {
		t.Fatal("deterministic inert const should fold")
	}
	if n, err := core.AsInteger(folded); err != nil || n != 7 {
		t.Errorf("folded = %v, want 7", folded)
	}
}

func TestS6aConcreteHandlerEvalNonConcreteResult(t *testing.T) {
	r := newTestRegistry(t)
	sig := &core.Signature{
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return []core.Value{core.NewCarrier(core.TInteger)}, nil
		}),
	}
	if _, ok := concreteHandlerEval(r, sig, nil); ok {
		t.Error("a carrier result is neither concrete nor a bare type node")
	}
}

func TestS6aIsModuleInnerSigGuards(t *testing.T) {
	if isModuleInnerSig(nil, "w", nil) {
		t.Error("nil registry must decline")
	}

	r := newTestRegistry(t)
	sub := newTestRegistry(t)
	em := core.NewOrderedMap()
	em.Set("s6aw", core.NewValueRaw(core.TFunction, core.FnDefInfo{Registry: sub, Name: "s6aw"}))
	r.Modules.MarkLoaded("s6amod", core.ModuleDesc{
		ID: "s6amod",
		Exports: map[string]*core.OrderedMap{
			"nil-entry": nil, // the em == nil continue arm
			"real":      em,  // inner Lookup(name) == nil continue arm
		},
	})
	if isModuleInnerSig(r, "s6aw", &core.Signature{}) {
		t.Error("an export whose sub-registry lacks the word must decline")
	}
}

// --- tryRecordPoly ------------------------------------------------------------

// TestEmitStateNilReceiver calls the nil-safe *EmitState methods on a nil
// pointer: every one has an `es == nil` guard and must not panic.
func TestEmitStateNilReceiver(t *testing.T) {
	var es *EmitState

	if es.Armed() {
		t.Fatal("nil EmitState is not armed")
	}
	if es.Active() || es.SuspendedNow() {
		t.Fatal("nil EmitState is not active")
	}
	if !es.TopFrameOnly() {
		t.Fatal("nil EmitState is top-frame")
	}
	es.BindRegistry(nil)
	if es.Sites() != nil {
		t.Fatal("nil Sites should be nil")
	}
	if es.ZeroOutProduced("x") || es.AlreadyProduced("x") {
		t.Fatal("nil produced probes should be false")
	}
	if es.UnitVariadic(0) {
		t.Fatal("nil unitVariadic should be false")
	}
	es.RegisterTrailingApply("id", 1)
	if es.TrailingApplyArity("id") != 0 {
		t.Fatal("nil TrailingApplyArity should be 0")
	}
	es.MarkUncompilable("x")
	es.Suspend()()
	if es.TakeFragment() != nil {
		t.Fatal("nil TakeFragment should be nil")
	}
	es.MarkValueDef(core.NewInteger(1))
	if es.OperandRepushable(core.NewInteger(1)) {
		t.Fatal("nil OperandRepushable should be false")
	}
	if es.CanSeatAcrossFragment(core.NewInteger(1)) {
		t.Fatal("nil CanSeatAcrossFragment should be false")
	}
	if _, ok := es.tryReturnedClosure(core.NewInteger(1), core.SrcPos{}); ok {
		t.Fatal("nil tryReturnedClosure should refuse")
	}
}

// seedProduced makes v resolve to an event operand: resolveOperand finds a
// producedBy entry (and v is not a type body, so the events-first arm wins).
func seedProduced(es *EmitState, v core.Value, seq int) {
	es.producedBy[v.ID] = producer{seq: seq, idx: 0}
}

// residualReadStable's no-name arm: a value never noted as a def read (or a
// reg-less state) is never a stable residual re-push candidate.
func TestResidualReadStableNoName(t *testing.T) {
	if (&EmitState{}).residualReadStable(core.Value{}) {
		t.Error("a value with no def-read note must not be stable")
	}
}

// TestWindowReadsID pins the replay tail-proof's window-read probe (the
// §9.8 dyn-bind skip): only a value the window actually holds reads.
func TestWindowReadsID(t *testing.T) {
	w := core.NewCarrier(core.TFunction)
	w.ID = "g1"
	if !windowReadsID([]core.Value{w}, "g1") {
		t.Error("a window value's own ID must read")
	}
	if windowReadsID([]core.Value{w}, "zz") {
		t.Error("an ID absent from the window must not read")
	}
}

func TestStripZeroOutPhantomsKeepsNonPhantom(t *testing.T) {
	// Mint inside a pass so the values carry the compile identities the
	// producedBy bookkeeping keys on (runtime mints elide IDs).
	c := &core.CheckState{}
	defer c.Begin()()
	es := NewEmitState()
	phantom := core.NewInteger(1)
	kept := core.NewInteger(2)
	seedProduced(es, phantom, 1)
	f := es.eventInfo[1]
	f.zeroOut = true
	es.eventInfo[1] = f
	seedProduced(es, kept, 2) // eventInfo[2].zeroOut defaults false
	got := es.stripZeroOutPhantoms([]core.Value{phantom, kept})
	if len(got) != 1 || got[0].ID != kept.ID {
		t.Fatalf("strip = %v, want just the kept value", got)
	}
}

func TestSetLoopBodyApplyDeclines(t *testing.T) {
	es := NewEmitState()
	// bodyStk[0] fn-typed carrier but not an event operand → false.
	if es.setLoopBodyApply(&EmitFragment{}, []core.Value{core.NewCarrier(core.TFunction), core.NewInteger(1)}) {
		t.Fatal("non-event fn head should decline")
	}
	// bodyStk[0] resolves to an event (dynamic fn carrier); a fn-valued
	// trailing arg → false.
	fn := core.NewDynamicCarrier(core.TFunction)
	seedProduced(es, fn, 1)
	if es.setLoopBodyApply(&EmitFragment{}, []core.Value{fn, core.NewCarrier(core.TFunction)}) {
		t.Fatal("fn-valued loop-apply arg should decline")
	}
}

func TestInstanceFnFieldRiskUnresolvableKey(t *testing.T) {
	es := NewEmitState()
	inst := instanceVal(mapOfFields())
	es.fnRiskFields = map[string]map[string]bool{inst.ID: {"f": true}}
	// No concrete key among args → any tracked field is risky → true.
	if !es.instanceFnFieldRisk([]core.Value{inst}) {
		t.Fatal("unresolvable key over a tracked instance should be risky")
	}
	// Concrete key that is NOT risky → false.
	if es.instanceFnFieldRisk([]core.Value{inst, core.NewAtom("other")}) {
		t.Fatal("concrete non-risky key should not be risky")
	}
	// Empty risk table → false.
	if (NewEmitState()).instanceFnFieldRisk([]core.Value{inst}) {
		t.Fatal("no risk table should be false")
	}
}

func TestProducerWordMiss(t *testing.T) {
	es := NewEmitState()
	v := core.NewInteger(1)
	seedProduced(es, v, 99) // producedBy hit, but no frame event with seq 99
	if _, ok := es.producerWord(v.ID); ok {
		t.Fatal("no matching frame event should miss")
	}
}

func TestProducerReturnedClosureArity(t *testing.T) {
	// id not produced → false.
	es := NewEmitState()
	if _, ok := es.producerReturnedClosureArity("nope"); ok {
		t.Fatal("unproduced id should decline")
	}
	// produced, but the frame has only a non-matching seq → loop then trailing miss.
	es = NewEmitState()
	es.producedBy["x"] = producer{seq: 99}
	es.frames[0] = []EmitEvent{{seq: 1, kind: evCall}}
	if _, ok := es.producerReturnedClosureArity("x"); ok {
		t.Fatal("no matching seq should decline")
	}
	// matching evCallUser with an out-of-range unit → false.
	es = NewEmitState()
	es.producedBy["u"] = producer{seq: 5}
	es.frames[0] = []EmitEvent{{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 99}}}
	if _, ok := es.producerReturnedClosureArity("u"); ok {
		t.Fatal("out-of-range unit should decline")
	}
	// matching unit whose rec is not a single closure out → false.
	es = NewEmitState()
	es.producedBy["u"] = producer{seq: 5}
	es.fnRecs = []*fnUnitRec{{}}
	es.frames[0] = []EmitEvent{{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 0}}}
	if _, ok := es.producerReturnedClosureArity("u"); ok {
		t.Fatal("non-closure out should decline")
	}
	// closure out whose closureUnit is out of range → false.
	es = NewEmitState()
	es.producedBy["u"] = producer{seq: 5}
	es.fnRecs = []*fnUnitRec{{outOps: []EmitOperand{{kind: opClosure, closureUnit: 99}}}}
	es.frames[0] = []EmitEvent{{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 0}}}
	if _, ok := es.producerReturnedClosureArity("u"); ok {
		t.Fatal("out-of-range closureUnit should decline")
	}
}

// TestConstLambdaArity pins the CONST arm of the arity recovery (the
// eleventh increment): a capture-free lambda the fold baked as one value
// carries its arity on its own signature, and the exclusions are the
// fn-value apply's soundness argument — only an ANONYMOUS boru lambda with
// one own signature answers.
func TestConstLambdaArity(t *testing.T) {
	boru := func() core.SigImpl { return &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}} }
	lam := func(name, module string, sigs ...core.Signature) core.Value {
		v := fnVal(sigs...)
		fd := v.Data.(core.FnDefInfo)
		fd.Name, fd.Module = name, module
		v.Data = fd
		return v
	}
	twoParam := core.Signature{Params: []core.FnParam{{Name: "a"}, {Name: "z"}}, Impl: boru()}
	rows := []struct {
		note string
		v    core.Value
		want int
		ok   bool
	}{
		{"an anonymous two-param lambda", lam("", "", twoParam), 2, true},
		{"a zero-param lambda", lam("", "", core.Signature{Impl: boru()}), 0, true},
		{"a NAMED fn value (word dispatch, not value semantics)", lam("addone", "", twoParam), 0, false},
		{"a MODULE export", lam("", "boru:fn-util", twoParam), 0, false},
		{"a Go-impl fn (no boru body)", lam("", "", core.Signature{Params: []core.FnParam{{Name: "a"}}}), 0, false},
		{"two own signatures — no one arity", lam("", "", twoParam, core.Signature{Params: []core.FnParam{{Name: "a"}}, Impl: boru()}), 0, false},
	}
	for _, c := range rows {
		es := NewEmitState()
		idx := es.internUnpooled(c.v)
		got, ok := constLambdaArity(es.consts, idx)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: got (%d,%v), want (%d,%v)", c.note, got, ok, c.want, c.ok)
		}
	}
	// A non-fn const, and an index outside the pool.
	es := NewEmitState()
	if _, ok := constLambdaArity(es.consts, es.internUnpooled(core.NewInteger(5))); ok {
		t.Error("a plain value carries no arity")
	}
	if _, ok := constLambdaArity(es.consts, -1); ok {
		t.Error("an out-of-range const index declines")
	}
	if _, ok := constLambdaArity(es.consts, len(es.consts)); ok {
		t.Error("an index past the pool declines")
	}
}

// TestProducerReturnedClosureConstArm pins the two questions apart: the
// arity recovery answers for a baked lambda const, and the CLOSURE
// predicate (which guards a ClosurePayload only the VM can invoke) does
// not.
func TestProducerReturnedClosureConstArm(t *testing.T) {
	es := NewEmitState()
	es.producedBy["u"] = producer{seq: 5}
	lam := fnVal(core.Signature{Params: []core.FnParam{{Name: "x"}}, Impl: &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}})
	es.fnRecs = []*fnUnitRec{{outOps: []EmitOperand{ConstOperand(es.internUnpooled(lam))}}}
	es.frames[0] = []EmitEvent{{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 0}}}
	if arity, ok := es.producerReturnedClosureArity("u"); !ok || arity != 1 {
		t.Errorf("a baked lambda's arity = (%d,%v), want (1,true)", arity, ok)
	}
	if es.producerReturnedClosure("u") {
		t.Error("a baked lambda const is not a compiled closure payload")
	}
	es.fnRecs[0].outOps = []EmitOperand{{kind: opClosure, closureUnit: 1}}
	es.fnRecs = append(es.fnRecs, &fnUnitRec{nParams: 3})
	if !es.producerReturnedClosure("u") {
		t.Error("a closure out op IS a compiled closure payload")
	}
	if _, ok := es.producerReturnedOutOp("nope"); ok {
		t.Error("an unproduced id has no out op")
	}
}

func TestRecordCallOperandsInertFnBake(t *testing.T) {
	es := NewEmitState()
	sig := &core.Signature{Args: []*core.Type{core.TFunction}, FnInertArgs: map[int]bool{0: true}}
	fn := fnVal(core.Signature{Args: []*core.Type{core.TInteger}})
	ops, ok := es.RecordCallOperands("stash", sig, []core.Value{fn})
	if !ok {
		t.Fatal("an inert fn operand should bake as a const")
	}
	if len(ops) != 1 || ops[0].kind != opConst {
		t.Fatalf("baked operand = %+v, want a const", ops)
	}
}

func TestNoEvalBodiesInertSentinel(t *testing.T) {
	// A body that is inert data but carries a break sentinel is refused.
	sig := &core.Signature{NoEvalArgs: map[int]bool{0: true}}
	body := core.NewList([]core.Value{core.NewWord("break")})
	if noEvalBodiesInert(sig, []core.Value{body}) {
		t.Fatal("a break-bearing body should not be inert")
	}
	es := NewEmitState()
	if es.noEvalBodiesInertScoped(sig, []core.Value{body}) {
		t.Fatal("a break-bearing body should not be scoped-inert")
	}
}

func TestInternCompoundEmptyID(t *testing.T) {
	es := NewEmitState()
	lst := core.NewList([]core.Value{core.NewInteger(1)})
	lst.ID = "" // a compound with no ID takes the un-pooled append path
	if idx := es.intern(lst); idx != 0 || len(es.consts) != 1 {
		t.Fatalf("empty-id compound intern = %d (consts=%d)", idx, len(es.consts))
	}
}

func TestRecordCallOperandsInertFnBakeCaptured(t *testing.T) {
	es := NewEmitState()
	sig := &core.Signature{Args: []*core.Type{core.TFunction}, FnInertArgs: map[int]bool{0: true}}
	// A CAPTURED fn value must NOT bake as a const: a frozen const body leaves
	// the captured names as unbound Words (`word(c)`), so a later invocation
	// reads them unbound (the capturing-sink miscompile). It routes to the
	// closure path instead — which, for a NON-anonymous fn like this synthetic
	// one, declines (tryReturnedClosure requires an anonymous single-sig
	// lambda), so the operand does not resolve and RecordCallOperands refuses.
	// The program then falls back faithfully. (Anonymous capturing lambdas whose
	// body compiles DO resolve to an opClosure — see
	// lang/go's TestPatrunFnValueStoreCompiles / TestReturnedCapturingClosureApply.)
	fn := core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TFunction)), Parent: core.TFunction,
		Data: core.FnDefInfo{Signatures: []core.Signature{{Args: []*core.Type{core.TInteger}}},
			Captured: []core.CapturedBinding{{Name: "c", Value: core.NewInteger(1)}}}}
	ops, ok := es.RecordCallOperands("stash", sig, []core.Value{fn})
	if ok {
		t.Fatalf("captured non-anonymous fn must NOT resolve as a const bake: ops=%+v", ops)
	}
}

func TestRecordDefRebindRefusals(t *testing.T) {
	// fn-valued rebind → refuse.
	es := NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{"n": 0}}}
	es.RecordDefRebind("n", core.NewCarrier(core.TFunction), core.SrcPos{})
	if es.Compilable {
		t.Fatal("fn-valued loop-carried rebind should refuse")
	}
	// unresolvable rebind → refuse.
	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{"n": 0}}}
	es.RecordDefRebind("n", core.NewCarrier(core.TInteger), core.SrcPos{})
	if es.Compilable {
		t.Fatal("unresolvable loop-carried rebind should refuse")
	}
}

func TestRefuseCarriedUndefFound(t *testing.T) {
	es := NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 1, slots: map[string]int{"n": 0}}}
	es.RefuseCarriedUndef("n")
	if es.Compilable {
		t.Fatal("undef of a loop-carried name should refuse")
	}
}

func TestMixedDynamicApplyShape(t *testing.T) {
	// Interior dynamic value that is NOT event-produced → declines.
	es := NewEmitState()
	dyn := core.NewDynamicCarrier(core.TAny)
	residual := []core.Value{core.NewInteger(1), dyn, core.NewInteger(2)}
	if _, ok := es.mixedDynamicApplyShape(residual); ok {
		t.Fatal("non-event interior dynamic should decline")
	}
	// Same shape, but the carrier carries a method-shape annotation → declines
	// before the event-produced
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	es2 := NewEmitState()
	es2.reg = r
	r.Check.MethodShapes = map[string]core.Value{dyn.ID: core.NewInteger(0)}
	if _, ok := es2.mixedDynamicApplyShape(residual); ok {
		t.Fatal("annotated interior carrier should decline")
	}
}

func TestDynamicStackShuffleOK(t *testing.T) {
	// Shuffle word but no registry bound → false.
	if NewEmitState().dynamicStackShuffleOK("dup", &core.Signature{}) {
		t.Fatal("no registry should decline")
	}
	// Shuffle word whose sig has a non-Any arg → false.
	r, sig := miniShuffleReg(t, "dup", []*core.Type{core.TInteger})
	es := NewEmitState()
	es.reg = r
	if es.dynamicStackShuffleOK("dup", sig) {
		t.Fatal("a non-Any shuffle sig should decline")
	}
}

func TestRecordShuffleElidedMismatch(t *testing.T) {
	r, sig := miniShuffleReg(t, "swap", []*core.Type{core.TAny, core.TAny})
	es := NewEmitState()
	es.reg = r
	d1, d2 := core.NewDynamicCarrier(core.TAny), core.NewDynamicCarrier(core.TAny)
	// outs are not an ID-preserving permutation of args (one fresh id) → false.
	if es.recordShuffleElided("swap", sig, []core.Value{d1, d2}, []core.Value{d1, core.NewInteger(0)}) {
		t.Fatal("non-permutation outputs should not elide")
	}
}

func TestTryReturnedClosureRefusals(t *testing.T) {
	r := covRegistry(t, nil)
	// Anonymous lambda with an unresolvable capture → declines.
	es := NewEmitState()
	es.reg = r
	withCap := core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TFunction)), Parent: core.TFunction,
		Data: core.FnDefInfo{Anonymous: true,
			Captured:   []core.CapturedBinding{{Name: "c", Value: core.NewCarrier(core.TInteger)}},
			Signatures: []core.Signature{{}}}}
	if _, ok := es.tryReturnedClosure(withCap, core.SrcPos{}); ok {
		t.Fatal("unresolvable capture should decline")
	}
	// Anonymous lambda with more than one own signature → declines.
	es = NewEmitState()
	es.reg = r
	multiSig := core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TFunction)), Parent: core.TFunction,
		Data: core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{}, {}}}}
	if _, ok := es.tryReturnedClosure(multiSig, core.SrcPos{}); ok {
		t.Fatal("multi-sig lambda should decline")
	}
}

func TestTryReturnedClosureProbeFails(t *testing.T) {
	r := covRegistry(t, nil)
	es := NewEmitState()
	es.reg = r
	// One own sig, a nil-typed param (exercises the t==nil → Any default), and
	// a body that references an unknown word so the probe compile refuses.
	lam := core.Signature{
		Params:     []core.FnParam{{Name: "y", Type: nil}},
		Impl:       core.Boru([]core.Value{core.NewWord("no_such_word_zzz9")}),
		Returns:    []*core.Type{core.TInteger},
		BarrierPos: -1,
	}
	v := core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TFunction)), Parent: core.TFunction,
		Data: core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{lam}}}
	if _, ok := es.tryReturnedClosure(v, core.SrcPos{}); ok {
		t.Fatal("an uncompilable lambda body should decline")
	}
}

func TestStartFnCompileFinishZeroOut(t *testing.T) {
	es := NewEmitState()
	_, finish, ok := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, core.SrcPos{})
	if !ok {
		t.Fatal("StartFnCompile should open a unit")
	}
	// A body residual entry that is a 0-output statement guard's phantom is
	// skipped in the finish resolution.
	phantom := core.NewInteger(1)
	es.producedBy[phantom.ID] = producer{seq: 1}
	f := es.eventInfo[1]
	f.zeroOut = true
	es.eventInfo[1] = f
	finish([]core.Value{phantom})
}

func TestStartFnCompileFinishDynTrail(t *testing.T) {
	es := NewEmitState()
	_, finish, _ := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, core.SrcPos{})
	arg := core.NewInteger(5)
	fn := core.NewDynamicCarrier(core.TFunction)
	es.producedBy[fn.ID] = producer{seq: 1} // resolves to an event
	es.RegisterTrailingApply(fn.ID, 1)      // arity 1 == len(bodyStk)-1
	finish([]core.Value{arg, fn})
}

func TestStartFnCompileFinishPendingApply(t *testing.T) {
	es := NewEmitState()
	unit, finish, _ := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, core.SrcPos{})
	u := es.units[len(es.units)-1]
	fnHead := core.NewDynamicCarrier(core.TFunction)
	fnLast := core.NewDynamicCarrier(core.TFunction)
	es.producedBy[fnHead.ID] = producer{seq: 1}
	es.producedBy[fnLast.ID] = producer{seq: 2}
	u.pendingApply = []pendingApply{{id: fnLast.ID, pos: core.SrcPos{Row: 1, Col: 9}}}
	// bodyStk[:last] carries a fn VALUE: an argument of the pending apply
	// (the thirty-first increment — `(a:Any => [b:Any => [a/v]]) p/v apply`
	// hands the lambda to the pair), so the whole residual is the window.
	finish([]core.Value{fnHead, fnLast})
	if rec := es.fnRecs[unit]; !es.Compilable || rec.dynTrailArity != 1 || !rec.dynTrailApply || rec.dynTrailPos.Col != 9 {
		t.Fatalf("a fn value beneath the pending apply is the window's argument: compilable=%v arity=%d apply=%v pos=%v", es.Compilable, rec.dynTrailArity, rec.dynTrailApply, rec.dynTrailPos)
	}
	// A MID-BODY pending apply (its fn not the residual's top) still refuses.
	es = NewEmitState()
	_, finish, _ = es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, core.SrcPos{})
	u = es.units[len(es.units)-1]
	es.producedBy[fnHead.ID] = producer{seq: 1}
	es.producedBy[fnLast.ID] = producer{seq: 2}
	u.pendingApply = []pendingApply{{id: fnHead.ID}}
	finish([]core.Value{fnHead, fnLast})
	if es.Compilable {
		t.Fatal("a mid-body pending apply should refuse")
	}
}

func TestRecordCallRefusalQuotedOperand(t *testing.T) {
	es := NewEmitState()
	sig := &core.Signature{QuoteArgs: map[int]bool{0: true}}
	if !es.recordCallRefusal("usurp", sig, nil, nil, core.SrcPos{}, false, false) || es.Compilable {
		t.Fatal("an uncovered quoted-operand word should refuse")
	}
}

func TestFinalizeResidualArms(t *testing.T) {
	// A zeroOut phantom in the residual is skipped, yielding an empty program.
	es := NewEmitState()
	phantom := core.NewInteger(1)
	es.producedBy[phantom.ID] = producer{seq: 1}
	f := es.eventInfo[1]
	f.zeroOut = true
	es.eventInfo[1] = f
	if p, _, ok := es.Finalize([]core.Value{phantom}); !ok || p == nil {
		t.Fatal("a zeroOut-only residual should finalize")
	}

	// A bare Word residual materialises but is not an inert const → refuses.
	es = NewEmitState()
	if _, why, ok := es.Finalize([]core.Value{core.NewWord("x")}); ok || why == "" {
		t.Fatal("a non-materialisable residual should refuse")
	}

	// An unfinished fn unit with no terminal trap → refuses.
	es = NewEmitState()
	es.fnRecs = []*fnUnitRec{{name: "ghost"}}
	if _, why, ok := es.Finalize(nil); ok || why == "" {
		t.Fatal("an unfinished fn unit should refuse")
	}
}

func TestEventDivergesDeepArms(t *testing.T) {
	// else arm is a plain value (armValue) → never diverges.
	evVal := EmitEvent{kind: evBranch, br: &emitBranch{hasElse: true, elsIsVal: true}}
	if eventDivergesDeep(&evVal) {
		t.Fatal("value-else branch should not diverge")
	}
	// else arm is computed (armComputed) → never diverges.
	evComp := EmitEvent{kind: evBranch, br: &emitBranch{hasElse: true, elsIsVal: true, elsComputed: true}}
	if eventDivergesDeep(&evComp) {
		t.Fatal("computed-else branch should not diverge")
	}
	// A non-control event kind falls through to the trailing return false.
	evStoreEv := EmitEvent{kind: evStore, store: &emitStore{}}
	if eventDivergesDeep(&evStoreEv) {
		t.Fatal("store event should not diverge")
	}
}

func TestEventPosStore(t *testing.T) {
	p := core.SrcPos{}
	ev := EmitEvent{kind: evStore, store: &emitStore{pos: p}}
	if eventPos(ev) != p {
		t.Fatal("eventPos(store) mismatch")
	}
}

func TestEventsThroughSeqMiss(t *testing.T) {
	evs := []EmitEvent{{seq: 1}, {seq: 2}}
	// A seq not present returns the whole slice.
	if got := eventsThroughSeq(evs, 99); len(got) != 2 {
		t.Fatalf("miss should return all events, got %d", len(got))
	}
}

func TestComputedArmCondOKDirect(t *testing.T) {
	tru := true
	// ConstCond set → false (the disjoint const-cond path owns it).
	if computedArmCondOK(core.BranchRecord{ConstCond: &tru}, EmitOperand{}) {
		t.Fatal("const-cond should not be a computed-arm cond")
	}
	// opNone / default cond → false.
	if computedArmCondOK(core.BranchRecord{}, EmitOperand{kind: opNone}) {
		t.Fatal("opNone cond should be rejected")
	}
	// A stack event cond → true.
	if !computedArmCondOK(core.BranchRecord{}, EmitOperand{kind: opEvent}) {
		t.Fatal("event cond should be accepted")
	}
}

func TestEmbedsEnclosingCompound(t *testing.T) {
	// MapPayload with nil backing map → false (the nil-map guard).
	nilMap := core.Value{Parent: core.TMap, Data: core.MapPayload{M: nil}}
	if embedsEnclosingCompound(nilMap, map[string]bool{}) {
		t.Fatal("nil-map should not embed")
	}
	// A map whose (compound) member is an enclosing binding's value → true.
	member := core.NewList([]core.Value{core.NewInteger(9)})
	om := core.NewOrderedMap()
	om.Set("k", member)
	mp := core.NewMap(om)
	if !embedsEnclosingCompound(mp, map[string]bool{member.ID: true}) {
		t.Fatal("map embedding an enclosing compound should report true")
	}
}

func TestReturnsAllScalarEmpty(t *testing.T) {
	if returnsAllScalar(nil) {
		t.Fatal("no declared returns should not be all-scalar")
	}
}

func TestZeroArgFnOut(t *testing.T) {
	if !zeroArgFnOut([]core.Value{core.NewInteger(1), zeroArgFn()}) {
		t.Fatal("a concrete 0-param fn out should be flagged")
	}
	if zeroArgFnOut([]core.Value{core.NewInteger(1)}) {
		t.Fatal("no fn out should not be flagged")
	}
}

func TestReadsFnMember(t *testing.T) {
	fn := fnVal(core.Signature{Args: []*core.Type{core.TInteger}})
	// list, concrete int key → fn at that index.
	if !readsFnMember([]core.Value{core.NewList([]core.Value{fn}), core.NewInteger(0)}) {
		t.Fatal("list int-key fn should be reported")
	}
	// list, no key → loop finds the fn.
	if !readsFnMember([]core.Value{core.NewList([]core.Value{fn})}) {
		t.Fatal("list loop fn should be reported")
	}
	// nil-backed map → skipped.
	if readsFnMember([]core.Value{{Parent: core.TMap, Data: core.MapPayload{M: nil}}}) {
		t.Fatal("nil map should be skipped")
	}
	// map, no key → loop finds the fn.
	if !readsFnMember([]core.Value{s7MapOf("f", fn)}) {
		t.Fatal("map loop fn should be reported")
	}
	// flat instance, concrete key → fn field.
	if !readsFnMember([]core.Value{instanceVal(mapOf2("f", fn)), core.NewAtom("f")}) {
		t.Fatal("instance key fn should be reported")
	}
	// flat instance, no key → loop finds the fn.
	if !readsFnMember([]core.Value{instanceVal(mapOf2("f", fn))}) {
		t.Fatal("instance loop fn should be reported")
	}
}

func TestFlatInstanceNonFnKey(t *testing.T) {
	// readsFnMember: concrete key present but the field is not a fn → continue.
	inst := instanceVal(mapOf2("k", core.NewInteger(1)))
	if readsFnMember([]core.Value{inst, core.NewAtom("k")}) {
		t.Fatal("non-fn field should not be reported")
	}
	// containerFnAutoDispatchRisk: same, with a non-0-arg field.
	if containerFnAutoDispatchRisk([]core.Value{inst, core.NewAtom("k")}) {
		t.Fatal("non-fn field should not be risky")
	}
}

func TestContainerFnAutoDispatchRisk(t *testing.T) {
	fn := zeroArgFn()
	if !containerFnAutoDispatchRisk([]core.Value{core.NewList([]core.Value{fn}), core.NewInteger(0)}) {
		t.Fatal("list int-key 0-arg fn should be risky")
	}
	if !containerFnAutoDispatchRisk([]core.Value{core.NewList([]core.Value{fn})}) {
		t.Fatal("list loop 0-arg fn should be risky")
	}
	if containerFnAutoDispatchRisk([]core.Value{{Parent: core.TMap, Data: core.MapPayload{M: nil}}}) {
		t.Fatal("nil map should be skipped")
	}
	if !containerFnAutoDispatchRisk([]core.Value{s7MapOf("f", fn)}) {
		t.Fatal("map loop 0-arg fn should be risky")
	}
	if !containerFnAutoDispatchRisk([]core.Value{instanceVal(mapOf2("f", fn)), core.NewAtom("f")}) {
		t.Fatal("instance key 0-arg fn should be risky")
	}
	if !containerFnAutoDispatchRisk([]core.Value{instanceVal(mapOf2("f", fn))}) {
		t.Fatal("instance loop 0-arg fn should be risky")
	}
}

func TestInterpMemberInert(t *testing.T) {
	// list member not inert → false.
	if InterpMemberInert(core.NewList([]core.Value{core.NewCarrier(core.TInteger)})) {
		t.Fatal("carrier list member should not be inert")
	}
	// nil-backed map → false.
	if InterpMemberInert(core.Value{Parent: core.TMap, Data: core.MapPayload{M: nil}}) {
		t.Fatal("nil map should not be inert")
	}
	// map of inert members → true.
	if !InterpMemberInert(s7MapOf("k", core.NewInteger(1))) {
		t.Fatal("map of inert members should be inert")
	}
	// paren-expr with a non-inert token → false.
	if InterpMemberInert(core.NewParenExpr([]core.Value{core.NewCarrier(core.TInteger)})) {
		t.Fatal("carrier paren token should not be inert")
	}
}

func TestInterpMemberInertBadInterp(t *testing.T) {
	// A value whose Parent is TInterpString but whose payload is not an
	// InterpStringPayload → AsInterpString errors → not inert.
	bad := core.Value{Parent: core.TInterpString, Data: core.IntPayload{N: 1}}
	if InterpMemberInert(bad) {
		t.Fatal("a malformed interp-string should not be inert")
	}
}

func TestValueRefsNameMap(t *testing.T) {
	// nil-backed map → false.
	if valueRefsName(core.Value{Parent: core.TMap, Data: core.MapPayload{M: nil}}) {
		t.Fatal("nil map references no name")
	}
	// map whose member is a Word → true.
	if !valueRefsName(s7MapOf("k", core.NewWord("x"))) {
		t.Fatal("a Word map member is a name reference")
	}
}

func TestIsTypeBodyPayload(t *testing.T) {
	if !isTypeBodyPayload(core.Value{Data: core.ChildTypeInfo{}}) {
		t.Fatal("ChildTypeInfo is a type-body payload")
	}
	if isTypeBodyPayload(core.NewInteger(1)) {
		t.Fatal("an integer is not a type-body payload")
	}
}

func TestThenArmComputed(t *testing.T) {
	br := &emitBranch{thenComputed: true}
	if br.thenArm() != armComputed {
		t.Fatal("thenComputed should classify as armComputed")
	}
}

// TestResolveOperandIdentitylessCompoundRescued pins the fn-unit rescue:
// a compound with no compile identity (a runtime mint) must route to
// dynScopeRescue — never into the ID-keyed freshen/share machinery —
// while the same compound WITH an identity resolves normally.
func TestResolveOperandIdentitylessCompoundRescued(t *testing.T) {
	r := seam7Reg(t)
	defer r.Check.Begin()()
	es := NewEmitState()
	es.BindRegistry(r)
	// One extra unit puts resolution inside a compiled fn frame.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}})

	blank := core.Value{Parent: core.TList, Data: core.ListPayload{Elems: []core.Value{core.NewInteger(1)}}} // no ID
	if _, ok := es.resolveOperand(blank); ok {
		t.Error("identity-less compound inside a fn unit must be rescued, not placed")
	}
	// Positive twin: the same compound minted with an identity resolves.
	minted := core.NewList([]core.Value{core.NewInteger(1)})
	if minted.ID == "" {
		t.Fatal("test premise: pass-minted list should carry an ID")
	}
	if _, ok := es.resolveOperand(minted); !ok {
		t.Error("pass-minted compound must still resolve inside a fn unit")
	}
}

// compileStoredBody declines (fails safe) without a live EmitState / registry,
// and for a non-list body operand — the defensive guards on the spawn code-body
// compile path.
func TestCompileStoredBodyGuards(t *testing.T) {
	var nilES *EmitState
	if _, ok := nilES.compileStoredBody(core.NewInteger(1)); ok {
		t.Fatal("nil EmitState must decline")
	}
	if _, ok := (&EmitState{}).compileStoredBody(core.NewInteger(1)); ok {
		t.Fatal("reg-less EmitState must decline")
	}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.InitRootContext()
	if _, ok := (&EmitState{reg: r}).compileStoredBody(core.NewInteger(1)); ok {
		t.Fatal("a non-list body must decline")
	}
}

// compileStoredFnUnit declines (fails safe) when it has no live EmitState /
// registry to compile against — the defensive guards on the recorder-internal
// path. A nil receiver and a reg-less state both return (0, false).
func TestCompileStoredFnUnitGuards(t *testing.T) {
	var nilES *EmitState
	if _, ok := nilES.compileStoredFnUnit(core.FnDefInfo{}, 0, core.SrcPos{}); ok {
		t.Fatal("nil EmitState must decline")
	}
	if _, ok := (&EmitState{}).compileStoredFnUnit(core.FnDefInfo{}, 0, core.SrcPos{}); ok {
		t.Fatal("reg-less EmitState must decline")
	}
	// A reg-ful state with an out-of-range / ineligible sig index declines
	// at the per-sig gate (REFUSAL-CLOSURE §7b).
	es := NewEmitState()
	es.reg = runUnitReg(t)
	if _, ok := es.compileStoredFnUnit(core.FnDefInfo{}, 0, core.SrcPos{}); ok {
		t.Fatal("an out-of-range sig index must decline")
	}
}

func TestW9UserPolyArmShapeOK(t *testing.T) {
	body := core.Boru([]core.Value{core.NewWord("p")})

	// Empty body → false.
	if userPolyArmShapeOK(&core.Signature{}, nil) {
		t.Error("empty body should refuse")
	}
	// Quote/type/form arg slots → false.
	if userPolyArmShapeOK(&core.Signature{Impl: body, QuoteArgs: map[int]bool{0: true}}, nil) {
		t.Error("a QuoteArgs slot should refuse")
	}
	// A quoted param → false.
	if userPolyArmShapeOK(&core.Signature{
		Impl:   body,
		Params: []core.FnParam{{Name: "p", Quote: true}},
	}, []*core.Type{core.TInteger}) {
		t.Error("a quoted param should refuse")
	}
	// Returns length mismatch → false.
	if userPolyArmShapeOK(&core.Signature{
		Impl:    body,
		Params:  []core.FnParam{{Name: "p", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger, core.TInteger},
	}, []*core.Type{core.TInteger}) {
		t.Error("a Returns arity mismatch should refuse")
	}
	// nil/non-nil Returns mismatch → false.
	if userPolyArmShapeOK(&core.Signature{
		Impl:    body,
		Params:  []core.FnParam{{Name: "p", Type: core.TInteger}},
		Returns: []*core.Type{nil},
	}, []*core.Type{core.TInteger}) {
		t.Error("a nil vs concrete Returns slot should refuse")
	}
	// Matching shape → true.
	if !userPolyArmShapeOK(&core.Signature{
		Impl:    body,
		Params:  []core.FnParam{{Name: "p", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger},
	}, []*core.Type{core.TInteger}) {
		t.Error("a matching arm shape should pass")
	}
}

func TestW9FindOwningFnDef(t *testing.T) {
	r := newTestRegistry(t)
	// nil impl → not found.
	if _, ok := findOwningFnDef(r, "w", nil); ok {
		t.Error("nil impl must not resolve an owner")
	}
	// A non-FnDefInfo binding above the word: the scan skips it (continue)
	// and, with no fn entry present, returns not-found.
	r.Defs.Push("w9poly", core.NewInteger(7))
	impl := core.Boru([]core.Value{core.NewWord("x")})
	if _, ok := findOwningFnDef(r, "w9poly", impl); ok {
		t.Error("a non-fn binding must be skipped and yield not-found")
	}
}

func TestW9TryCompileUserPolyOwnerRefusal(t *testing.T) {
	r := newTestRegistry(t)
	// Two same-arity overloads whose owning def is a MACRO: the owner gate
	// (owner.Macro) refuses, keeping the interpreter in charge.
	core.InstallFnDef(r, "w9macro", core.FnDefInfo{
		Macro: true,
		Signatures: []core.Signature{
			{Params: []core.FnParam{{Name: "a", Type: core.TInteger}}, Returns: []*core.Type{core.TInteger}, Impl: core.Boru([]core.Value{core.NewWord("a")})},
			{Params: []core.FnParam{{Name: "b", Type: core.TInteger}}, Returns: []*core.Type{core.TInteger}, Impl: core.Boru([]core.Value{core.NewWord("b")})},
		},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9macro", []core.Value{core.NewInteger(1)}, []*core.Type{core.TInteger}) != nil {
		t.Error("a macro-owned poly set should refuse")
	}
}

func TestW9TryCompileUserPolyArmCompileFails(t *testing.T) {
	r := newTestRegistry(t)
	// Two same-arity overloads that pass the shape/owner gates but whose
	// bodies are deferred-param-list residuals: compileUserPolyArm refuses,
	// so the whole poly set is kept in the interpreter.
	def := func(p string) core.Signature {
		inner := core.NewList([]core.Value{core.NewWord(p)})
		inner.Eval = true
		return core.Signature{
			Params:  []core.FnParam{{Name: p, Type: core.TInteger}},
			Returns: []*core.Type{core.TInteger},
			Impl:    core.Boru([]core.Value{inner}),
		}
	}
	core.InstallFnDef(r, "w9defer", core.FnDefInfo{
		Signatures: []core.Signature{def("a"), def("b")},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9defer", []core.Value{core.NewInteger(1)}, []*core.Type{core.TInteger}) != nil {
		t.Error("a poly set with an uncompilable arm should refuse")
	}
}

func TestW9CompileUserPolyArmRefusals(t *testing.T) {
	r := newTestRegistry(t)

	// Empty body → -1,false.
	if _, ok := compileUserPolyArm(r, NewEmitState(), "w", &core.Signature{}, core.FnDefInfo{}); ok {
		t.Error("empty body arm should refuse")
	}

	// A deferred-param-list body (eval list referencing a param) → -1,false.
	inner := core.NewList([]core.Value{core.NewWord("p")})
	inner.Eval = true
	deferredSig := &core.Signature{
		Params: []core.FnParam{{Name: "p", Type: core.TInteger}},
		Impl:   core.Boru([]core.Value{inner}),
	}
	if _, ok := compileUserPolyArm(r, NewEmitState(), "w", deferredSig, core.FnDefInfo{}); ok {
		t.Error("a deferred-param-list body should refuse")
	}

	// A valid body but an INACTIVE recorder → StartFnCompile fails → -1,false.
	plainSig := &core.Signature{
		Params:  []core.FnParam{{Name: "p", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewWord("p")}),
	}
	if _, ok := compileUserPolyArm(r, inactiveEmitState(), "w", plainSig, core.FnDefInfo{}); ok {
		t.Error("an inactive recorder should fail StartFnCompile")
	}
}

func TestRecordMakeListRefusals(t *testing.T) {
	// Not the top frame → declines.
	es := NewEmitState()
	es.frames = append(es.frames, nil)
	if es.RecordMakeList(nil, []core.Value{core.NewInteger(1)}, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("non-top-frame make-list should decline")
	}
	// A bare type node element with a canonical ID INTERNS as a type
	// operand and records (ADR-010 / NUR051 — `[a None]` is data)…
	es = NewEmitState()
	if !es.RecordMakeListInner(nil, []core.Value{core.NewTypeLiteral(core.TInteger)}, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("type-node element with a canonical ID should intern and record")
	}
	// …while one with NO canonical ID has no runtime home → declines.
	es = NewEmitState()
	noID := core.NewTypeLiteral(core.TInteger)
	noID.ID = ""
	if es.RecordMakeListInner(nil, []core.Value{noID}, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("type node with no canonical ID should decline")
	}
}

func TestRecordMakeMapRefusals(t *testing.T) {
	es := NewEmitState()
	// length mismatch → declines.
	if es.RecordMakeMap(nil, []string{"a"}, nil, false, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("key/val mismatch should decline")
	}
	// A bare type node value with a canonical ID INTERNS as a type
	// operand and records (ADR-010 / NUR051 — `{r: None}` is data)…
	es = NewEmitState()
	if !es.RecordMakeMap(nil, []string{"a"}, []core.Value{core.NewTypeLiteral(core.TInteger)}, false, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("type-node value with a canonical ID should intern and record")
	}
	// …while one with NO canonical ID has no runtime home → declines.
	es = NewEmitState()
	noID := core.NewTypeLiteral(core.TInteger)
	noID.ID = ""
	if es.RecordMakeMap(nil, []string{"a"}, []core.Value{noID}, false, core.NewInteger(0), core.SrcPos{}) {
		t.Fatal("type node with no canonical ID should decline")
	}
}

func TestRecordCallRefusalArms(t *testing.T) {
	pos := core.SrcPos{}
	// sig == nil.
	es := NewEmitState()
	if !es.recordCallRefusal("w", nil, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("nil sig should refuse")
	}
	// anonymous dispatch (word == "").
	es = NewEmitState()
	if !es.recordCallRefusal("", &core.Signature{}, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("empty word should refuse")
	}
	// full-stack word.
	es = NewEmitState()
	fs := &core.Signature{Impl: &core.GoImpl{FullStack: true}}
	if !es.recordCallRefusal("depth", fs, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("full-stack word should refuse")
	}
	// get-family read that may auto-dispatch a fn member.
	es = NewEmitState()
	getArgs := []core.Value{core.NewList([]core.Value{zeroArgFn()})}
	if !es.recordCallRefusal("get", &core.Signature{}, getArgs, nil, pos, false, false) || es.Compilable {
		t.Fatal("fn-member read should refuse")
	}
	// context-dependent word `args`.
	es = NewEmitState()
	if !es.recordCallRefusal("args", &core.Signature{}, nil, nil, pos, false, false) || es.Compilable {
		t.Fatal("args should refuse")
	}
}

// fnValueInputs: a param carrying only a value PATTERN has no bare type
// (FnParam.Type nil) and binds a gradual Any input carrier; a typed param
// binds its own type. Pinned directly — the shape is otherwise reachable
// only through a capturing pattern-param returned fn, whose probe declines
// before the carrier is consumed (PR #295 merge coverage).
func TestFnValueInputsPatternParamDefaultsAny(t *testing.T) {
	inputs, names := fnValueInputs([]core.FnParam{{Name: "p"}, {Name: "q", Type: core.TInteger}})
	if len(inputs) != 2 || len(names) != 2 || names[0] != "p" || names[1] != "q" {
		t.Fatalf("inputs/names = %v %v", inputs, names)
	}
	if inputs[0].Parent == nil || !inputs[0].Parent.Equal(core.TAny) {
		t.Errorf("a pattern-only param must bind an Any carrier, got %v", inputs[0].Parent)
	}
	if !inputs[1].Parent.Equal(core.TInteger) {
		t.Errorf("a typed param must keep its type, got %v", inputs[1].Parent)
	}
}

func TestEmitEmptyIDGuards(t *testing.T) {
	es := NewEmitState()

	// setProducedAt with an identity-less value: no entry — a later ""
	// lookup is a guaranteed miss, and no other value can false-hit it.
	blank := core.NewInteger(1) // runtime mint → "" ID
	if blank.ID != "" {
		t.Fatal("test premise: runtime mint should be identity-less")
	}
	es.setProducedAt(blank, 3, 0)
	if _, ok := es.producedBy[""]; ok {
		t.Error("setProducedAt keyed the provenance map on \"\"")
	}
	// Positive twin: a minted value registers normally.
	end := core.BeginIDMintScope()
	minted := core.NewInteger(2)
	end()
	es.setProducedAt(minted, 4, 0)
	if pr, ok := es.producedBy[minted.ID]; !ok || pr.seq != 4 {
		t.Error("minted value did not register in producedBy")
	}

	// RegisterLocal: "" refuses with -1 and never inserts; two distinct
	// identity-less values must NOT collapse onto one slot.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}})
	if slot := es.RegisterLocal(""); slot != -1 {
		t.Errorf("RegisterLocal(\"\") = %d, want -1", slot)
	}
	if slot := es.RegisterLocal(""); slot != -1 {
		t.Errorf("second RegisterLocal(\"\") = %d, want -1", slot)
	}
	if _, ok := es.units[len(es.units)-1].localByID[""]; ok {
		t.Error("RegisterLocal inserted a \"\" slot")
	}
	// Positive twin: real IDs get distinct slots.
	a := es.RegisterLocal("S_aaaaaaaaaaaa")
	b := es.RegisterLocal("S_bbbbbbbbbbbb")
	if a == -1 || b == -1 || a == b {
		t.Errorf("real IDs got slots %d/%d, want distinct non-negative", a, b)
	}
}

// readFnMemberValue / memberFnReadValue arm coverage (REFUSAL-CLOSURE.0 §3):
// the pinpointing walk's decline arms, each unreachable-by-shape from the
// lang fixtures alone — a carrier arg is skipped, a nil-backed map arg is
// skipped, a non-fn member misses, and a bool-only tag (zero member) reports
// no pinpointed value.
func TestReadFnMemberValueArms(t *testing.T) {
	fn := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "d", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}}}
	om := core.NewOrderedMap()
	om.Set("f", fn)
	fnMap := core.NewMap(om)
	key := core.NewAtom("f")

	// Positive: concrete map + concrete key pinpoints the member.
	if got, ok := readFnMemberValue([]core.Value{fnMap, key}); !ok {
		t.Fatalf("concrete map+key must pinpoint the member, got ok=false (%v)", got)
	}
	// A carrier arg is skipped (the walk continues past it to the map).
	if _, ok := readFnMemberValue([]core.Value{carrierVal(core.TMap), fnMap, key}); !ok {
		t.Errorf("a leading carrier arg must be skipped, not decline the walk")
	}
	// A nil-backed map payload is skipped.
	nilMap := core.Value{Parent: core.TMap, Data: core.MapPayload{M: nil}}
	if _, ok := readFnMemberValue([]core.Value{nilMap, key}); ok {
		t.Errorf("a nil-backed map cannot pinpoint a member")
	}
	// A non-fn member misses.
	om2 := core.NewOrderedMap()
	om2.Set("f", core.NewInteger(5))
	if _, ok := readFnMemberValue([]core.Value{core.NewMap(om2), key}); ok {
		t.Errorf("a non-fn member must not pinpoint")
	}

	// memberFnReadValue: a bool-only tag (zero member — the computed-key
	// scan) reports no pinpointed value; an untagged id likewise.
	es := NewEmitState()
	es.NoteMemberFnRead("tagged-only", core.Value{})
	if _, ok := es.MemberFnReadValue("tagged-only"); ok {
		t.Errorf("a zero-member tag must not report a pinpointed value")
	}
	if _, ok := es.MemberFnReadValue("never-tagged"); ok {
		t.Errorf("an untagged id must not report a pinpointed value")
	}
	es.NoteMemberFnRead("rich", fn)
	if got, ok := es.MemberFnReadValue("rich"); !ok || !core.IsConcrete(got) {
		t.Errorf("a rich tag must report the member")
	}
}

func TestW9TryCompileUserPolyEarlyReturns(t *testing.T) {
	r := newTestRegistry(t)
	args := []core.Value{core.NewInteger(1)}

	// The early-return guard: an inactive recorder OR empty args refuses
	// before any lookup.
	if tryCompileUserPolyArms(r, inactiveEmitState(), "w", args, []*core.Type{core.TInteger}) != nil {
		t.Error("inactive recorder should refuse")
	}
	if tryCompileUserPolyArms(r, NewEmitState(), "w", nil, []*core.Type{core.TInteger}) != nil {
		t.Error("empty args should refuse")
	}
	// Empty committedReturns is ADMITTED (REFUSAL-CLOSURE.0 §6a) — an
	// unknown word still refuses through the agg==nil gate.
	if tryCompileUserPolyArms(r, NewEmitState(), "w", args, nil) != nil {
		t.Error("unknown word should refuse (zero-return contract is admitted)")
	}
	// Unknown word → agg==nil → nil.
	if tryCompileUserPolyArms(r, NewEmitState(), "w9unknown", args, []*core.Type{core.TInteger}) != nil {
		t.Error("unknown word should refuse")
	}
	// A single same-arity overload → len(sigIdx) < 2 → nil.
	core.InstallFnDef(r, "w9one", core.FnDefInfo{
		Signatures: []core.Signature{{
			Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
			Returns: []*core.Type{core.TInteger},
			Impl:    core.Boru([]core.Value{core.NewWord("n")}),
		}},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9one", args, []*core.Type{core.TInteger}) != nil {
		t.Error("a single overload should refuse (single-overload path handles it)")
	}
}

func TestDynApplyLeadEligible(t *testing.T) {
	fnCarrier := func(id string) core.Value {
		v := core.NewCarrier(core.TFunction)
		v.ID = id
		return v
	}

	// The inactive recorder and a nil/inactive state decline.
	if core.TheInactiveEmit.DynApplyLeadEligible(core.Value{}) {
		t.Error("the inactive recorder must decline")
	}
	var nilES *EmitState
	if nilES.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("a nil recorder must decline")
	}

	// Outside any open fn unit (the top level) declines: the Stage-G shape
	// is a fn-body param apply, and top-level parens keep their machinery.
	es := NewEmitState()
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("the top level must decline")
	}

	// openUnit appends an open unit with the given rec — the StartFnCompile
	// ritual reduced to the fields the gate consults.
	openUnit := func(es *EmitState, rec *fnUnitRec) *emitUnit {
		u := &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}}
		es.fnRecs = append(es.fnRecs, rec)
		es.openUnitRecs = append(es.openUnitRecs, len(es.fnRecs)-1)
		es.units = append(es.units, u)
		return u
	}

	// A native code-body CLOSURE unit declines (its analysis frame is the
	// CallableSpec inputs, not a per-call named frame).
	es = NewEmitState()
	u := openUnit(es, &fnUnitRec{closure: true})
	u.localByID["g1"] = 0
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("a native code-body closure unit must decline")
	}

	// The Stage 2 closure-flag split: a LAMBDA unit ("fnval" — a returned
	// lambda's own body, a real named-param frame with capture slots) is
	// ADMITTED; the end-to-end witness is the factory's apply-the-capture
	// body (`(g v)` inside the returned lambda —
	// frontier-hof-audit.tsv §9).
	es = NewEmitState()
	u = openUnit(es, &fnUnitRec{closure: true, lambdaUnit: true})
	u.localByID["g1"] = 0
	if !es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("a lambda unit's own slot must be admitted")
	}

	// An UNNAMED-param unit declines: the frame re-pushes its args beneath
	// the region, so the interpreter's leading collection can reach past the
	// sealed window the trailing model records (`(args.0 args.1)` over a
	// two-arg fn nets 28 interpreted vs the model's no-match).
	es = NewEmitState()
	u = openUnit(es, &fnUnitRec{nParams: 2, nUnnamed: 2})
	u.localByID["g1"] = 0
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("an unnamed-param unit must decline")
	}

	// A lead that is NOT one of the unit's slots declines.
	es = NewEmitState()
	openUnit(es, &fnUnitRec{nParams: 1})
	if es.DynApplyLeadEligible(fnCarrier("gX")) {
		t.Error("a non-local lead must decline")
	}

	// An EVENT-provenance local (a computed def promoted to a slot) declines
	// — RecordDynApply hard-refuses an event fn (runtime quote state
	// unknown), so admitting it would turn a compiling shape into a refusal.
	es = NewEmitState()
	u = openUnit(es, &fnUnitRec{nParams: 1})
	u.localByID["g1"] = 0
	es.producedBy["g1"] = producer{seq: 3}
	if es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("an event-provenance lead must decline")
	}

	// A plain param slot is eligible; a CAPTURE stays eligible even with a
	// parent-unit produced entry (capID precedence — it resolves to its own
	// slot, not the unreachable parent event).
	es = NewEmitState()
	u = openUnit(es, &fnUnitRec{nParams: 1})
	u.localByID["g1"] = 0
	if !es.DynApplyLeadEligible(fnCarrier("g1")) {
		t.Error("a named param slot must be eligible")
	}
	u.localByID["c1"] = 1
	u.capID["c1"] = true
	es.producedBy["c1"] = producer{seq: 5}
	if !es.DynApplyLeadEligible(fnCarrier("c1")) {
		t.Error("a captured slot with a parent-unit event entry must stay eligible")
	}
}

func TestW9UnitNetsZero(t *testing.T) {
	// Bounds guards: nil recorder / out-of-range unit → false.
	var nilES *EmitState
	if nilES.UnitNetsZero(0) {
		t.Error("nil EmitState must report false")
	}
	es := NewEmitState()
	if es.UnitNetsZero(-1) || es.UnitNetsZero(0) {
		t.Error("out-of-range unit must report false")
	}
	// A 0-residual unit qualifies; a residual-carrying or variadic one does not.
	es.fnRecs = append(es.fnRecs,
		&fnUnitRec{},
		&fnUnitRec{outOps: []EmitOperand{ConstOperand(0)}},
		&fnUnitRec{variadic: true},
	)
	if !es.UnitNetsZero(0) {
		t.Error("an empty-residual unit must net zero")
	}
	if es.UnitNetsZero(1) {
		t.Error("a residual-carrying unit must not net zero")
	}
	if es.UnitNetsZero(2) {
		t.Error("a variadic unit must not net zero")
	}
	// The inactive recorder's stub answer.
	if core.TheInactiveEmit.UnitNetsZero(0) {
		t.Error("inactive recorder must report false")
	}
}

// carrierVal is an unresolvable operand: a stripped carrier with no recorded
// original, so materialise (and thus resolveOperand) refuses it.
func carrierVal(t *core.Type) core.Value { return core.NewCarrier(t) }

// --- materialise list/map rebuild ---------------------------------------

// --- eventPos / eventsThroughSeq ----------------------------------------

// --- record-method guards: inactive / unresolvable ----------------------

// --- RecordBranch refusal arms ------------------------------------------

// --- fn-value / container-member classifiers ----------------------------

func ptrVal(v core.Value) *core.Value { return &v }

// --- materialise list/map rebuild ---------------------------------------

// --- eventPos / eventsThroughSeq ----------------------------------------

// --- record-method guards: inactive / unresolvable ----------------------

// --- RecordBranch refusal arms ------------------------------------------

// --- fn-value / container-member classifiers ----------------------------

// TestEmitCheckpointHandle pins the S2 opaque-handle contract: the
// inactive recorder hands out a nil EmitCheckpoint, and the concrete
// Rollback ignores any handle that is not its own snapshot type (the
// no-op decline the dropped interface no-ops used to provide).
func TestEmitCheckpointHandle(t *testing.T) {
	e := core.TheInactiveEmit
	if e.Checkpoint() != nil {
		t.Fatal("inactive Checkpoint should be nil")
	}
	e.Rollback(nil)
	es := NewEmitState()
	es.Rollback(nil) // not an emitCheckpoint: ignored
	if cp := es.Checkpoint(); cp == nil {
		t.Fatal("concrete Checkpoint should hand out a (possibly zero) snapshot")
	}
}

// TestPlainCheckRunsAgainstRecorderInterface swaps a counting NON-EmitState
// recorder into a plain check pass and asserts (a) the pass completes with
// the same diagnostics as the stock pass, and (b) the recorder interface was
// actually consulted — the checker's only coupling to the emit machinery is
// the interface.
func TestPlainCheckRunsAgainstRecorderInterface(t *testing.T) {
	runPass := func(rec core.EmitRecorder) []core.CheckDiagnostic {
		r := recorderTestRegistry(t)
		defer r.Check.Begin()()
		if rec != nil {
			r.Check.Emit = rec
		}
		eng := core.NewTop(r)
		// A dispatch plus a genuine diagnostic (undefined word).
		toks := []core.Value{
			core.NewInteger(1), core.NewWord("padd"), core.NewInteger(2),
			core.NewWord("nosuchword"),
		}
		_, _ = eng.Run(toks)
		return append([]core.CheckDiagnostic(nil), r.Check.Diagnostics...)
	}

	base := runPass(nil)
	fake := &countingRecorder{EmitRecorder: core.TheInactiveEmit}
	got := runPass(fake)

	if len(base) != len(got) {
		t.Fatalf("diagnostics diverge under the fake recorder: stock %d, fake %d", len(base), len(got))
	}
	for i := range base {
		if base[i].Code != got[i].Code || base[i].Word != got[i].Word {
			t.Errorf("diagnostic %d diverges: stock %s/%s, fake %s/%s",
				i, base[i].Code, base[i].Word, got[i].Code, got[i].Word)
		}
	}
	if fake.activeCalls == 0 && fake.armedCalls == 0 && fake.recordCalls == 0 {
		t.Fatalf("the check pass never consulted the recorder interface (active=%d armed=%d record=%d)",
			fake.activeCalls, fake.armedCalls, fake.recordCalls)
	}
}

func TestIsolateEmitAndBudget(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	saved := r.Check.Emit
	restore := r.Check.IsolateEmit()
	if r.Check.Emit == saved {
		t.Error("IsolateEmit did not swap the emit state")
	}
	restore()
	if r.Check.Emit != saved {
		t.Error("IsolateEmit restore lost the original")
	}

	r.Check.StepCount = 7
	r.Check.BudgetTripped = false
	restoreB := r.Check.IsolateBudget()
	r.Check.StepCount = 999
	r.Check.BudgetTripped = true
	restoreB()
	if r.Check.StepCount != 7 || r.Check.BudgetTripped {
		t.Error("IsolateBudget did not roll the counters back")
	}

	var nilC *core.CheckState
	nilC.IsolateEmit()()
	nilC.IsolateBudget()()
}

// TestAppendResidualSeqs pins the promotion planner's residual input
// (NUR126): a residual closure's lexical CAPTURES are enclosing-scope
// operands and must be counted, or their producers are never promoted to
// frame locals and the closure push materialises an event operand through
// pushOperand's const arm — baking the event's SEQ as a const index.
func TestAppendResidualSeqs(t *testing.T) {
	cap0 := EventOperand(4, 0)
	cap1 := EventOperand(5, 0)
	nested := EmitOperand{kind: opClosure, closureUnit: 1, closureCaps: []EmitOperand{cap1}}
	ops := []EmitOperand{
		EventOperand(2, 0), // a plain residual event
		EventOperand(3, 1), // a non-zero result index is not a promotion candidate
		ConstOperand(9),    // a const references no producer
		localOperand(0),    // nor a local
		{kind: opClosure, closureUnit: 0, closureCaps: []EmitOperand{cap0, nested}},
	}
	got := appendResidualSeqs(nil, ops)
	want := []int{2, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("seqs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seqs = %v, want %v", got, want)
		}
	}
	if n := appendResidualSeqs(nil, nil); len(n) != 0 {
		t.Errorf("an empty residual references nothing: %v", n)
	}
}
