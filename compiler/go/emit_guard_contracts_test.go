package compiler

import (
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Direct pins of emit.go helpers on shapes their callers rarely or never
// present today (a nil store, an identity-less value, a producer outside the
// frame, a signature the registered words do not declare): each test states
// the answer the helper owes for the shape, beside the ordinary answer it is
// contrasted with. The whole-program paths are pinned in lang/go
// (recorder_edge_paths_test.go).

// TestEmbeddedEnclosingIDsNilStoreMember — the embedded-member walk of a
// fn-unit body literal (embeddedEnclosingIDs, the per-call fresh push's keep
// set) treats a member Map whose entries store is nil (core.NewMap(nil)) as
// empty rather than dereferencing it, and still keeps the enclosing member
// beside it.
func TestEmbeddedEnclosingIDsNilStoreMember(t *testing.T) {
	enclosing := core.NewList([]core.Value{core.NewInteger(9)})
	enclosing.ID = "enc-1"
	nilStore := core.NewMap(nil)
	nilStore.ID = "own-map"
	inner := core.NewList([]core.Value{nilStore, enclosing})
	inner.ID = "own-list"
	keep := embeddedEnclosingIDs(core.NewList([]core.Value{inner}), map[string]bool{"enc-1": true})
	if len(keep) != 1 || !keep["enc-1"] {
		t.Fatalf("keep = %v, want exactly the enclosing member", keep)
	}
	if got := embeddedEnclosingIDs(core.NewList([]core.Value{inner, nilStore}), map[string]bool{}); got != nil {
		t.Errorf("a literal embedding no enclosing container keeps nothing, got %v", got)
	}
}

// TestUnreachableUnitStubDropsStampRef — the stamp-only recovery replaces a
// unit whose lowering declined with its trap stub, trims the region
// descriptors lowered for the discarded body, and DROPS the compiled ref of
// the fn value that stamped that unit (dropStampRef), so the value runs as
// though nothing had stamped it — a native callback seam would otherwise
// enter the trap. Another stamp's ref is untouched.
func TestUnreachableUnitStubDropsStampRef(t *testing.T) {
	es := NewEmitState()
	kept, stubbed := &core.BoruImpl{}, &core.BoruImpl{}
	kept.SetCompiled(&CompiledFnRef{Unit: 0})
	stubbed.SetCompiled(&CompiledFnRef{Unit: 1})
	es.stampImpls = map[*core.BoruImpl]int{kept: 0, stubbed: 1}
	p := &Program{Fns: []CompiledFn{{Name: "kept"}}, Regions: []RegionDesc{{}, {}}}
	es.unreachableUnitStub(p, &fnUnitRec{name: "storedfn$body"}, 1)
	if len(p.Fns) != 2 || len(p.Fns[1].Code) != 1 || p.Fns[1].Code[0].Op != OpTrap {
		t.Fatalf("the unit must be replaced by a one-instruction trap, got %+v", p.Fns)
	}
	if len(p.Traps) != 1 || !strings.Contains(p.Traps[0].Detail, "stamp-only fn unit storedfn$body") || len(p.Regions) != 1 {
		t.Errorf("trap %+v, regions %d: want the stub's internal_error and the descriptors past the floor dropped", p.Traps, len(p.Regions))
	}
	if stubbed.Compiled() != nil {
		t.Error("the stubbed unit's fn value must lose its compiled ref")
	}
	if kept.Compiled() == nil || len(es.stampImpls) != 1 || es.stampImpls[kept] != 0 {
		t.Errorf("another stamp's ref and entry are untouched (stampImpls=%v)", es.stampImpls)
	}
}

// TestBranchArmsRetPinnedBothArmsDiverge — a branch inherits the RET-pinned
// marking only through an arm that REACHES the merge with a ret-pinned
// value. Both arms diverging (break / continue) reach no merge, so nothing is
// pinned even though "diverges or pinned" holds arm by arm; one diverging arm
// beside a ret-pinned one does inherit it.
func TestBranchArmsRetPinnedBothArmsDiverge(t *testing.T) {
	es := NewEmitState()
	brk := &EmitFragment{events: []EmitEvent{{kind: evBreak}}}
	cont := &EmitFragment{events: []EmitEvent{{kind: evContinue}}}
	if es.branchArmsRetPinned(core.BranchRecord{HasElse: true, Then: brk, Els: cont}) {
		t.Error("a branch whose arms both diverge reaches no merge, so nothing is ret-pinned")
	}
	pinned := core.Value{ID: "pinned-1"}
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "do", nout: 1}})
	es.producedBy[pinned.ID] = producer{seq: seq}
	es.eventInfo[seq] = eventFlags{dynBodyResult: true}
	els := &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{word: "do", nout: 1}}}}
	if !es.branchArmsRetPinned(core.BranchRecord{HasElse: true, Then: brk, Els: els, ElsStk: []core.Value{pinned}}) {
		t.Error("a diverging arm beside a ret-pinned arm inherits the marking")
	}
}

// TestRecordRuntimeBindDispatchUnresolvedOperand — a run-time binder's
// dispatch (NoteRuntimeBind's latch — unpack over a source the pass cannot
// read) is emitted as a plain 0-result CALL_NATIVE only when every operand
// has a compiled home. An operand with none hands the dispatch to
// RecordCall, whose compile-time-word arm declines it loudly: no event, and
// the latch is consumed.
func TestRecordRuntimeBindDispatchUnresolvedOperand(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	sig := &core.Signature{Args: []*core.Type{core.TList, core.TMap}, Impl: core.Go(nil, core.RunInCheck())}
	names := core.NewList([]core.Value{core.NewAtom("a")})
	src := core.NewCarrier(core.TMap)
	src.ID = "src-without-home"

	es := NewEmitState()
	es.reg = r
	es.NoteRuntimeBind("a")
	before := len(es.frames[0])
	es.RecordRuntimeBindDispatch("unpack", sig, []core.Value{names, src}, core.SrcPos{})
	if es.Compilable || !strings.Contains(es.Reason, "compile-time word unpack") {
		t.Errorf("an operand with no compiled home must decline the dispatch, got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
	if len(es.frames[0]) != before || es.pendingRuntimeBindCall {
		t.Errorf("the decline appends no event and consumes the latch (events %d -> %d, pending=%v)", before, len(es.frames[0]), es.pendingRuntimeBindCall)
	}

	// Every operand placed: the dispatch is the call itself.
	es = NewEmitState()
	es.reg = r
	es.NoteRuntimeBind("a")
	es.RecordRuntimeBindDispatch("unpack", sig, []core.Value{names, constMapWith("a", core.NewInteger(1))}, core.SrcPos{})
	fr := es.frames[0]
	if !es.Compilable || len(fr) != 1 || fr[0].call.word != "unpack" || fr[0].call.nout != 0 || len(fr[0].call.ops) != 2 {
		t.Errorf("placed operands emit one 0-result call, got compilable=%v reason=%q events=%d", es.Compilable, es.Reason, len(fr))
	}
}

// TestContainerReadResultNilSafe — the paren-lead admission asks
// ContainerReadResult of every lead; a nil recorder and one with no
// provenance table answer "no" rather than dereferencing, and a recorded
// get dispatch's result answers "yes".
func TestContainerReadResultNilSafe(t *testing.T) {
	var nilES *EmitState
	if nilES.ContainerReadResult("r") {
		t.Error("a nil EmitState must decline")
	}
	if (&EmitState{}).ContainerReadResult("r") {
		t.Error("an EmitState with no provenance table must decline")
	}
	es := NewEmitState()
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "get", nout: 1}})
	es.producedBy["read-1"] = producer{seq: seq}
	if !es.ContainerReadResult("read-1") {
		t.Error("a recorded get dispatch's result is a container read")
	}
	if es.ContainerReadResult("unrecorded") {
		t.Error("a value with no producer is no container read")
	}
}

// TestModifierWrappedFnShapeOperands — the shape a usurp wrapper claims is
// its operand's own with the params reversed, whether the operand is an
// event or a compiled closure operand; a NON-FIRST result of a multi-result
// producer is not a fn value the recorder can shape, so no claim is made.
func TestModifierWrappedFnShapeOperands(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{{nParams: 2, paramTypes: []*core.Type{core.TInteger, core.TString}}}
	closure := EmitOperand{kind: opClosure, closureUnit: 0}
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "usurp", poly: true, nout: 1, ops: []EmitOperand{closure}}})
	s, ok := es.modifierWrappedFnShape(seq, 0)
	if !ok || s.Arity != 2 || len(s.Params) != 2 || !s.Params[0].Equal(core.TString) || !s.Params[1].Equal(core.TInteger) {
		t.Errorf("usurp over a closure operand claims its params reversed, got ok=%v shape=%+v", ok, s)
	}
	two := es.appendEvent(EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0, nout: 2}})
	seq = es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "usurp", poly: true, nout: 1, ops: []EmitOperand{EventOperand(two, 1)}}})
	if s, ok := es.modifierWrappedFnShape(seq, 0); ok {
		t.Errorf("a multi-result producer's second result makes no claim, got %+v", s)
	}
}

// TestRecordArgsProjectionDeclines — the `args` projection records only on
// an active recorder, for a list with an identity, over params that each
// have a compiled home; a projection that records is remembered by the
// list's ID at the event it produced, for the args.N fold's retraction.
func TestRecordArgsProjectionDeclines(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := func(id string) core.Value {
		v := core.NewList(nil)
		v.ID = id
		return v
	}
	params := []core.Value{core.NewInteger(1), core.NewInteger(2)}

	es := NewEmitState()
	es.Compilable = false
	if es.RecordArgsProjection(r, params, list("proj-0"), core.SrcPos{}) {
		t.Error("an inactive recorder records nothing")
	}
	es = NewEmitState()
	if es.RecordArgsProjection(r, params, list(""), core.SrcPos{}) {
		t.Error("an identity-less projection cannot be remembered, so it is not recorded")
	}
	noHome := core.NewCarrier(core.TInteger)
	noHome.ID = "param-without-home"
	if es.RecordArgsProjection(r, []core.Value{noHome}, list("proj-1"), core.SrcPos{}) {
		t.Error("a param with no compiled home leaves the projection unrecorded")
	}
	if len(es.argsProjSeq) != 0 || len(es.frames[0]) != 0 {
		t.Errorf("a declined projection leaves the recorder untouched (%d projections, %d events)", len(es.argsProjSeq), len(es.frames[0]))
	}
	if !es.RecordArgsProjection(r, params, list("proj-2"), core.SrcPos{}) {
		t.Fatal("placed params record the projection")
	}
	if seq, ok := es.argsProjSeq["proj-2"]; !ok || es.producedBy["proj-2"].seq != seq || !es.frames[0][len(es.frames[0])-1].call.makeList {
		t.Errorf("the projection is remembered at its OpMakeList event (argsProjSeq=%v producedBy=%v)", es.argsProjSeq, es.producedBy["proj-2"])
	}
}

// TestRegistryBodyScreens pins the registry-run body screens (receive's and
// Test.cover's CompileRunsBodyOnRegistry) on the shapes the registered
// words' own signatures never present: a flagless signature, an evaluated
// operand beside the body, a body with no registry to ask, a sentinel body,
// a nil-store map member, paren tokens, and the binder forms a rebind scan
// cannot resolve.
func TestRegistryBodyScreens(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Defs.Push("bound-x", core.NewInteger(1))
	noop := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{Name: "zzq-native", Signatures: []core.Signature{{
		Args: []*core.Type{core.TAny}, Impl: core.Go(noop), Returns: []*core.Type{core.TAny},
	}}})
	es := NewEmitState()
	es.reg = r
	list := func(vs ...core.Value) core.Value { return core.NewList(vs) }
	w := core.NewWord

	// runsBodyOnRegistryAtModuleScope: the flag is the word's own promise.
	body := list(core.NewInteger(1))
	flagless := &core.Signature{Args: []*core.Type{core.TList}, NoEvalArgs: map[int]bool{0: true}}
	if es.runsBodyOnRegistryAtModuleScope(flagless, []core.Value{body}) {
		t.Error("a word that does not run its body over the registry never takes the registry rule")
	}
	mixed := &core.Signature{Args: []*core.Type{core.TInteger, core.TList}, NoEvalArgs: map[int]bool{1: true}, CompileEffect: core.CompileRunsBodyOnRegistry}
	gradual := core.NewCarrier(core.TInteger)
	if !es.runsBodyOnRegistryAtModuleScope(mixed, []core.Value{gradual, body}) {
		t.Error("only the NoEval body is screened; an evaluated operand beside it is the dispatch's own")
	}
	allBodies := &core.Signature{Args: []*core.Type{core.TInteger, core.TList}, NoEvalArgs: map[int]bool{0: true, 1: true}, CompileEffect: core.CompileRunsBodyOnRegistry}
	if es.runsBodyOnRegistryAtModuleScope(allBodies, []core.Value{gradual, body}) {
		t.Error("a non-concrete BODY operand declines")
	}

	// registryBodyNamesNothingKnown: the nested-position admission.
	sig := &core.Signature{Args: []*core.Type{core.TList}, NoEvalArgs: map[int]bool{0: true}, CompileEffect: core.CompileRunsBodyOnRegistry}
	if !es.registryBodyNamesNothingKnown(sig, []core.Value{list(w("zzq-unknown"), core.NewInteger(1))}) {
		t.Error("a body naming nothing the registry knows is admitted")
	}
	if es.registryBodyNamesNothingKnown(sig, []core.Value{list(w("zzq-native"))}) {
		t.Error("a body naming a registered word declines")
	}
	if es.registryBodyNamesNothingKnown(sig, []core.Value{list(w("bound-x"))}) {
		t.Error("a body naming a bound name declines")
	}
	if es.registryBodyNamesNothingKnown(sig, []core.Value{list(core.NewInteger(1), w("break"))}) {
		t.Error("a body carrying a flow sentinel is not inert-scoped, so it declines")
	}
	if (&EmitState{}).registryBodyNamesNothingKnown(sig, []core.Value{body}) {
		t.Error("with no registry to resolve names against, nothing is admitted")
	}

	// valueNamesKnown: every container shape is walked.
	if es.valueNamesKnown(core.NewMap(nil)) {
		t.Error("a map with no entries store names nothing")
	}
	if !es.valueNamesKnown(constMapWith("k", w("bound-x"))) {
		t.Error("a map member naming a bound name is known")
	}
	if !es.valueNamesKnown(core.NewParenExpr([]core.Value{core.NewInteger(1), w("zzq-native"), core.NewInteger(2)})) {
		t.Error("a paren token naming a registered word is known")
	}
	if es.valueNamesKnown(core.NewParenExpr([]core.Value{core.NewInteger(1), w("zzq-unknown")})) {
		t.Error("a paren naming nothing known is not")
	}

	// bodyRebindsBoundName: a binder whose name the scan cannot read is a rebind.
	for _, c := range []struct {
		name string
		body core.Value
		want bool
	}{
		{"a trailing binder with no name", list(core.NewInteger(1), w("def")), true},
		{"a computed binder name", list(w("def"), core.NewParenExpr([]core.Value{w("n")}), core.NewInteger(1)), true},
		{"a binder of a bound name", list(w("def"), w("bound-x"), core.NewInteger(2)), true},
		{"a binder of a fresh name", list(w("def"), w("zzq-fresh"), core.NewInteger(2)), false},
		{"a nested binder of a bound name", list(list(w("undef"), w("bound-x"))), true},
	} {
		if got := es.bodyRebindsBoundName(c.body); got != c.want {
			t.Errorf("%s: bodyRebindsBoundName = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestCallResultRenderKnownProducerOffFrame — a parked call result renders
// known only when its producing user call is an event of the CURRENT frame
// whose unit returns a render-known fn; a provenance entry whose event is
// not in the frame (recorded in a closed fragment) answers no.
func TestCallResultRenderKnownProducerOffFrame(t *testing.T) {
	es := NewEmitState()
	fi := es.internUnpooled(storedFnBody(core.NewString("T")))
	es.fnRecs = []*fnUnitRec{{outOps: []EmitOperand{ConstOperand(fi)}}}
	seq := es.appendEvent(EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}})
	es.producedBy["res-1"] = producer{seq: seq}
	if !es.callResultRenderKnown(core.Value{ID: "res-1"}) {
		t.Error("a user call returning a baked fn const renders known")
	}
	es.producedBy["res-2"] = producer{seq: seq + 100}
	if es.callResultRenderKnown(core.Value{ID: "res-2"}) {
		t.Error("a producer outside the current frame answers no")
	}
}

// TestBranchResultRenderKnownArms — an `if` result renders known only when
// EVERY value-producing arm is a render-known fn operand: a compiled closure
// carrying its render string or an anonymous capture-free lambda const. A
// closure without a render, a const outside the pool, an event arm, a 2-arg
// branch (no else value), a decided branch whose taken arm nets nothing, and
// a nil / identity-less query all answer no.
func TestBranchResultRenderKnownArms(t *testing.T) {
	var nilES *EmitState
	if nilES.branchResultRenderKnown(core.Value{ID: "b"}) {
		t.Error("a nil EmitState must decline")
	}
	es := NewEmitState()
	if es.branchResultRenderKnown(core.Value{}) {
		t.Error("an identity-less value must decline")
	}
	lam := es.internUnpooled(core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Anonymous: true}})
	es.fnRecs = []*fnUnitRec{{render: "fn (Integer)"}, {}}
	rendered := EmitOperand{kind: opClosure, closureUnit: 0}
	unrendered := EmitOperand{kind: opClosure, closureUnit: 1}
	taken := true
	for _, c := range []struct {
		name string
		br   *emitBranch
		want bool
	}{
		{"two rendered closures", &emitBranch{hasElse: true, hasThenOut: true, thenOut: rendered, hasElsOut: true, elsOut: rendered}, true},
		{"a lambda const value beside a rendered closure", &emitBranch{hasElse: true, thenIsVal: true, thenVal: ConstOperand(lam), hasElsOut: true, elsOut: rendered}, true},
		{"a closure with no render string", &emitBranch{hasElse: true, hasThenOut: true, thenOut: rendered, hasElsOut: true, elsOut: unrendered}, false},
		{"a closure unit out of range", &emitBranch{hasElse: true, hasThenOut: true, thenOut: EmitOperand{kind: opClosure, closureUnit: 7}, hasElsOut: true, elsOut: rendered}, false},
		{"a const index outside the pool", &emitBranch{hasElse: true, hasThenOut: true, thenOut: ConstOperand(99), hasElsOut: true, elsOut: rendered}, false},
		{"an event arm", &emitBranch{hasElse: true, hasThenOut: true, thenOut: rendered, hasElsOut: true, elsOut: EventOperand(0, 0)}, false},
		{"a 2-arg branch: no else value", &emitBranch{hasThenOut: true, thenOut: rendered}, false},
		{"a decided branch whose taken arm nets nothing", &emitBranch{constCond: &taken, hasElse: true}, false},
	} {
		seq := es.appendEvent(EmitEvent{kind: evBranch, br: c.br})
		id := fmt.Sprintf("branch-%d", seq)
		es.producedBy[id] = producer{seq: seq}
		if got := es.branchResultRenderKnown(core.Value{ID: id}); got != c.want {
			t.Errorf("%s: branchResultRenderKnown = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestApplyChainStepsShapes — applyChainSteps partitions a body residual
// into a chain of `apply` steps only when every pending fn sits in the
// residual in order with the last on top, every operand is re-pushable, no
// operand is written between two applies, no step's argument is a fn value,
// and nothing is left above the last step. Every other shape is nil.
func TestApplyChainStepsShapes(t *testing.T) {
	val := func(id string) core.Value {
		v := core.NewInteger(1)
		v.ID = id
		return v
	}
	x, y := val("x"), val("y")
	f, g := core.Value{ID: "f"}, core.Value{ID: "g"}
	fnArg := fnVal()
	pend := []pendingApply{{id: "f"}, {id: "g"}}
	locals := func(n int) []EmitOperand {
		ops := make([]EmitOperand, n)
		for i := range ops {
			ops[i] = localOperand(i)
		}
		return ops
	}
	steps := applyChainSteps(pend, []core.Value{x, f, g}, locals(3))
	if len(steps) != 2 || steps[0].n != 1 || len(steps[0].ops) != 2 || steps[1].n != 1 || len(steps[1].ops) != 1 {
		t.Fatalf("`x f/v apply g/v apply` is a two-step chain, got %+v", steps)
	}
	for _, c := range []struct {
		name string
		pend []pendingApply
		stk  []core.Value
		ops  []EmitOperand
	}{
		{"one pending apply", pend[:1], []core.Value{x, f}, locals(2)},
		{"an operand count that is not the residual's", pend, []core.Value{x, f, g}, locals(2)},
		{"the last pending fn below the top", pend, []core.Value{x, g, f}, locals(3)},
		{"an event operand", pend, []core.Value{x, f, g}, []EmitOperand{EventOperand(4, 0), localOperand(1), localOperand(2)}},
		{"a pending fn missing from the residual", []pendingApply{{id: "h"}, {id: "g"}}, []core.Value{x, f, g}, locals(3)},
		{"the first fn with no argument beneath it", pend, []core.Value{f, g}, locals(2)},
		{"an operand written between two applies", pend, []core.Value{x, f, y, g}, locals(4)},
		{"a fn value among a step's arguments", pend, []core.Value{fnArg, f, g}, locals(3)},
		{"a value left above the last step", pend, []core.Value{x, f, g, g}, locals(4)},
	} {
		if got := applyChainSteps(c.pend, c.stk, c.ops); got != nil {
			t.Errorf("%s: want nil, got %+v", c.name, got)
		}
	}
}
