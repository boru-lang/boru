package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// cover_merge511_test.go pins the defensive arms of main's #510 recorder
// helpers that the corpus never reaches (the merged ADR-008 gate on the
// reverse-order NUR run's merge of main's #511), each with the verdict the
// arm exists to give.

// modifierWrappedFnShape: a claim only for a usurp / forward-args poly
// record over an operand whose own shape is known, usurp reversing it; no
// claim past the depth bound, over an event past its first result, or over
// an operand with no shape.
func TestModifierWrappedFnShapeArms(t *testing.T) {
	es := NewEmitState()
	if _, ok := es.modifierWrappedFnShape(0, 9); ok {
		t.Error("past the depth bound there is no claim")
	}
	lam := core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "a", Type: core.TInteger}, {Name: "b", Type: core.TString}},
		Impl:   core.Boru([]core.Value{core.NewInteger(1)}),
	}}})
	es.consts = []core.Value{lam}
	wrap := func(word string, op EmitOperand) int {
		return es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: word, poly: true, nout: 1, ops: []EmitOperand{op}}})
	}
	s, ok := es.modifierWrappedFnShape(wrap("usurp", ConstOperand(0)), 0)
	if !ok || s.Arity != 2 || len(s.Params) != 2 || s.Params[0] != core.TString || s.Params[1] != core.TInteger {
		t.Errorf("usurp over a const lambda claims its shape reversed: %+v %v", s, ok)
	}
	s, ok = es.modifierWrappedFnShape(wrap("forward-args", ConstOperand(0)), 0)
	if !ok || s.Params[0] != core.TInteger {
		t.Errorf("forward-args keeps the order: %+v %v", s, ok)
	}
	if _, ok := es.modifierWrappedFnShape(wrap("forward-args", ConstOperand(5)), 0); ok {
		t.Error("an operand with no known shape makes no claim")
	}
	inner := wrap("usurp", ConstOperand(0))
	if _, ok := es.modifierWrappedFnShape(wrap("usurp", EmitOperand{kind: opEvent, idx: inner, resIdx: 1}), 0); ok {
		t.Error("an event's second result is not the fn it built")
	}
}

// soleNoEvalSlot: a signature with two NoEvalArgs positions has no sole one.
func TestSoleNoEvalSlotTwoSlots(t *testing.T) {
	if _, ok := soleNoEvalSlot(&core.Signature{NoEvalArgs: map[int]bool{0: true, 1: true}}, 2); ok {
		t.Error("two NoEvalArgs slots are not one")
	}
	if i, ok := soleNoEvalSlot(&core.Signature{NoEvalArgs: map[int]bool{1: true}}, 2); !ok || i != 1 {
		t.Errorf("the one NoEvalArgs slot is found: %d %v", i, ok)
	}
}

// registryBodyNamesNothingKnown and valueNamesKnown: no registry admits
// nothing; a map with no payload names nothing; a paren token naming a
// bound name is known.
func TestValueNamesKnownShapes(t *testing.T) {
	es := NewEmitState()
	if es.registryBodyNamesNothingKnown(&core.Signature{NoEvalArgs: map[int]bool{0: true}}, []core.Value{core.NewInteger(1)}) {
		t.Error("with no registry nothing is admitted")
	}
	r := newTestRegistry(t)
	es.reg = r
	r.Defs.Push("zzkn", core.NewInteger(1))
	if es.valueNamesKnown(core.Value{Parent: core.TMap, Data: core.MapPayload{}}) {
		t.Error("a map with no payload names nothing")
	}
	paren := core.Value{Data: core.ParenExprPayload{Toks: []core.Value{core.NewInteger(2), core.NewWord("zzkn")}}}
	if !es.valueNamesKnown(paren) {
		t.Error("a paren token naming a bound name is known")
	}
	quiet := core.Value{Data: core.ParenExprPayload{Toks: []core.Value{core.NewInteger(2)}}}
	if es.valueNamesKnown(quiet) {
		t.Error("a paren of data names nothing")
	}
}

// branchResultRenderKnown and fnOpRenderKnown: every value-producing arm
// must be a fn value the compiler renders as the interpreter does; a body
// arm's out operand counts as an arm, a variadic or empty branch does not.
func TestBranchResultRenderKnownArms(t *testing.T) {
	var nilES *EmitState
	if nilES.branchResultRenderKnown(core.NewInteger(1)) {
		t.Error("a nil recorder knows no render")
	}
	es := NewEmitState()
	if es.branchResultRenderKnown(core.NewInteger(1)) {
		t.Error("an identity-less value has no known render")
	}
	es.fnRecs = []*fnUnitRec{{name: "k$body", render: "fn (Integer)"}}
	closure := EmitOperand{kind: opClosure, closureUnit: 0}
	yes := true
	branch := func(b *emitBranch) core.Value {
		seq := es.appendEvent(EmitEvent{kind: evBranch, br: b})
		v := core.NewCarrier(core.TFunction)
		es.setProducedAt(v, seq, 0)
		return v
	}
	if !es.branchResultRenderKnown(branch(&emitBranch{constCond: &yes, hasThenOut: true, thenOut: closure})) {
		t.Error("a decided branch whose body arm returns a rendered closure is known")
	}
	if !es.branchResultRenderKnown(branch(&emitBranch{thenIsVal: true, thenVal: closure, hasElsOut: true, elsOut: closure})) {
		t.Error("an undecided branch whose arms are rendered closures is known")
	}
	if es.branchResultRenderKnown(branch(&emitBranch{thenIsVal: true, thenVal: closure})) {
		t.Error("an else that nets nothing keeps the decline")
	}
	if es.branchResultRenderKnown(branch(&emitBranch{constCond: &yes})) {
		t.Error("a branch with no value-producing arm is not known")
	}
	if es.branchResultRenderKnown(branch(&emitBranch{constCond: &yes, thenIsVal: true, thenVal: ConstOperand(3)})) {
		t.Error("a const index the state cannot resolve is not known")
	}
	if es.branchResultRenderKnown(branch(&emitBranch{constCond: &yes, thenIsVal: true, thenVal: localOperand(0)})) {
		t.Error("a local arm (a fn it was handed) is not known")
	}
}

// dropStampRef clears exactly the ref of the stamp whose unit declined, and
// forgets it; another stamp keeps its ref, and a unit no stamp backs changes
// nothing.
func TestDropStampRefClearsTheUnitsRef(t *testing.T) {
	es := NewEmitState()
	keep, drop := core.Boru(nil), core.Boru(nil)
	keep.SetCompiled(&CompiledFnRef{Unit: 0})
	drop.SetCompiled(&CompiledFnRef{Unit: 1})
	es.stampImpls = map[*core.BoruImpl]int{keep: 0, drop: 1}
	es.dropStampRef(7)
	if len(es.stampImpls) != 2 || drop.Compiled() == nil {
		t.Fatal("a unit no stamp backs changes nothing")
	}
	es.dropStampRef(1)
	if drop.Compiled() != nil {
		t.Error("the declined unit's stamp must lose its ref")
	}
	if _, still := es.stampImpls[drop]; still || keep.Compiled() == nil || es.stampImpls[keep] != 0 {
		t.Errorf("only the declined stamp is forgotten: %v", es.stampImpls)
	}
}

// siblingCallOutputs: a first residual entry no event produced (a written
// value) is no sibling set.
func TestSiblingCallOutputsNeedsAProducer(t *testing.T) {
	es := NewEmitState()
	seq := es.appendEvent(EmitEvent{kind: evCallUser})
	b := core.NewCarrier(core.TInteger)
	es.setProducedAt(b, seq, 0)
	if es.siblingCallOutputs([]core.Value{core.NewInteger(1), b}) {
		t.Error("a written first entry is not a call's output")
	}
}

// Finalize over a STAMP-ONLY unit whose lowering declines: nothing calls the
// unit, so the program keeps compiling — the unit becomes a trap stub at its
// index, and the fn value it backed loses its compiled ref (dropStampRef),
// so the value runs exactly as it would have if nothing had stamped it. A
// unit the program calls declines the program instead.
func TestFinalizeStubsADecliningStampOnlyUnit(t *testing.T) {
	declining := func(stampOnly bool) (*EmitState, *core.BoruImpl) {
		es := rpState(t)
		impl := core.Boru(nil)
		impl.SetCompiled(&CompiledFnRef{Unit: 0})
		es.stampImpls = map[*core.BoruImpl]int{impl: 0}
		// The body reads an event below its own scope floor: "branch reads
		// enclosing computation", a lowering-time decline.
		ev := EmitEvent{seq: 7, kind: evCall, call: emitCall{word: "add", nout: 1, ops: []EmitOperand{EventOperand(1, 0)}}}
		es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "storedfn$body", finished: true, stampOnly: stampOnly,
			frag: &EmitFragment{events: []EmitEvent{ev}, startSeq: 5}})
		return es, impl
	}
	es, impl := declining(true)
	p, reason, ok := es.Finalize(nil)
	if !ok {
		t.Fatalf("a stamp-only unit's decline must not decline the program: %s", reason)
	}
	if len(p.Fns) != 1 || len(p.Fns[0].Code) != 1 || p.Fns[0].Code[0].Op != OpTrap {
		t.Fatalf("the unit must be a trap stub at its index: %+v", p.Fns)
	}
	if impl.Compiled() != nil {
		t.Error("the stamp's ref must be dropped")
	}
	es, _ = declining(false)
	if _, reason, ok := es.Finalize(nil); ok || !strings.Contains(reason, "enclosing computation") {
		t.Errorf("a called unit's decline declines the program: ok=%v %q", ok, reason)
	}
}

// seatRootConsumedRead's guard (NUR207): a consuming event with no source
// position of its own starts the guarded statement at the read itself.
func TestSeatRootConsumedReadPositionlessConsumer(t *testing.T) {
	es := NewEmitState()
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "typeof", nout: 1}})
	rec := &fnUnitRec{frag: &EmitFragment{events: es.frames[0]}}
	read := core.SrcPos{Row: 1, Col: 30}
	lw := &lowerer{}
	es.seatRootConsumedRead(lw, rec, rootWordRead{name: "j", reads: []core.SrcPos{read}}, 0, 0, true, false, nil)
	if len(lw.deopts) != 1 || !lw.deopts[0].bail || lw.deopts[0].start != read {
		t.Fatalf("a guard before a position-less consumer starts at the read: %+v", lw.deopts)
	}
}

// rootResidualIsland / rootResidualStatic (NUR207): a read under a
// runtime-variable region, or under a rotated / mark-window layout, is no
// island — the residual is not the interpreter's written order there.
func TestRootResidualIslandNeedsTheWrittenOrder(t *testing.T) {
	es := NewEmitState()
	seq := es.appendEvent(EmitEvent{kind: evLoop, loop: &emitLoop{}})
	read, region := core.NewCarrier(core.TAny), core.NewCarrier(core.TList)
	es.setProducedAt(region, seq, 0)
	residual := []core.Value{read, region}
	lw := &lowerer{variadic: map[int]bool{seq: true}}
	if es.rootResidualStatic(lw, residual, 0) {
		t.Error("a variadic region above the read makes its depth dynamic")
	}
	lw.variadic = map[int]bool{}
	if !es.rootResidualStatic(lw, residual, 0) {
		t.Error("one-value entries above the read keep its depth static")
	}
	r := rootWordRead{name: "j", reads: []core.SrcPos{{Row: 1, Col: 5}}}
	if _, ok := es.rootResidualIsland(lw, r, residual, 0, 1, OpCallDynTrailTop); ok {
		t.Error("a rotated layout is not the written order")
	}
}
