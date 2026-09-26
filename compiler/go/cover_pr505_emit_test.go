package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestUnitTrapScope pins the unit-scoped trap's contract (recordUnitTrap,
// NUR134): no trap outside a body unit or off its root frame, the first trap
// per unit wins, a mark after it cannot decline the program (the unit raises
// first), and the unit's finish drops the unreachable tail — superseding any
// bind twin the tail placed, as the top-level truncation does.
func TestUnitTrapScope(t *testing.T) {
	ae := &core.BoruError{Code: "uncalled_function", Detail: "call to 'dec' matched no signature"}
	pos := core.SrcPos{Row: 1, Col: 5}

	es := NewEmitState()
	if es.recordUnitTrap(EmitTrap{pos: pos}) {
		t.Fatal("no body unit is open: the unit trap declines")
	}
	unit, finish, ok := es.StartFnCompile("k", "do$body", nil, nil, nil, nil, nil, false, core.SrcPos{})
	if !ok || finish == nil {
		t.Fatal("StartFnCompile should open a unit")
	}
	rec := es.fnRecs[unit]

	// A raise inside one of the unit's branch arms is conditional.
	resume := es.beginFragment()
	if es.RecordUnitTrapErr(ae, pos) {
		t.Error("a raise off the unit's root frame declines")
	}
	resume()
	es.TakeFragment()
	if rec.trapSeq != 0 {
		t.Fatalf("a declined trap records nothing: trapSeq=%d", rec.trapSeq)
	}

	// At the root frame the raise is the unit's trap, and the first one wins.
	if !es.RecordUnitTrapErr(ae, pos) || rec.trapSeq == 0 {
		t.Fatal("a definite raise at the unit's root frame is its trap")
	}
	first := rec.trapSeq
	if !es.RecordUnitTrapErr(ae, core.SrcPos{Row: 1, Col: 9}) || rec.trapSeq != first {
		t.Errorf("a second raise is unreachable, the first trap stands: %d vs %d", rec.trapSeq, first)
	}

	// Everything the unit records after its trap is unreachable.
	es.MarkUncompilable("a construct after the trap")
	if !es.Compilable || es.Reason != "" {
		t.Fatalf("a mark inside a trapped unit must not decline: %v %q", es.Compilable, es.Reason)
	}
	es.appendEvent(EmitEvent{kind: evBindTwin, twin: &emitBindTwin{idx: 7, pos: pos}})
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "add", nout: 1}})
	finish(nil)
	if len(rec.frag.events) != 1 || rec.frag.events[0].kind != evTrap {
		t.Fatalf("the unit ends at its trap: %+v", rec.frag.events)
	}
	if !es.supersededTwins[7] || len(es.supersededTwins) != 1 {
		t.Errorf("the tail's twin describes no runtime transition: %v", es.supersededTwins)
	}

	// With the unit closed, a mark declines again.
	es.MarkUncompilable("after the unit")
	if es.Compilable {
		t.Error("a mark outside every trapped unit declines")
	}
}

// TestNameLocalArms pins NameLocal: a nil or declined recorder, an empty id
// or name, and an id with no local name nothing; a registered local takes
// the name.
func TestNameLocalArms(t *testing.T) {
	var nilES *EmitState
	nilES.NameLocal("id", "i")

	es := NewEmitState()
	es.NameLocal("", "i")
	es.NameLocal("id", "")
	es.NameLocal("unregistered", "i")
	if es.units[0].slotNames != nil {
		t.Fatalf("nothing is named: %v", es.units[0].slotNames)
	}
	slot := es.RegisterLocal("id")
	es.NameLocal("id", "i")
	if es.units[0].slotNames[slot] != "i" {
		t.Errorf("the registered local is named: %v", es.units[0].slotNames)
	}

	declined := NewEmitState()
	declined.RegisterLocal("id")
	declined.MarkUncompilable("x")
	declined.NameLocal("id", "i")
	if declined.units[0].slotNames != nil {
		t.Error("a declined recorder names nothing")
	}
}

// TestUnitSlotNamesArms pins the did-you-mean pool's slot table: nil for no
// unit or a unit naming nothing, the first name per slot kept, and an empty
// name, a negative slot or an arm-bound id with no local skipped.
func TestUnitSlotNamesArms(t *testing.T) {
	if unitSlotNames(nil) != nil {
		t.Error("no unit, no names")
	}
	if unitSlotNames(&emitUnit{localByID: map[string]int{}}) != nil {
		t.Error("a unit naming no local has no table")
	}
	u := &emitUnit{
		localByID:   map[string]int{"a": 2, "b": 4},
		slotNames:   map[int]string{0: "i", 1: ""},
		nameSlots:   map[string]int{"j": 0, "acc": 3, "neg": -1},
		boundLocals: map[string]string{"a": "x", "b": "", "nolocal": "y"},
	}
	got := unitSlotNames(u)
	want := map[int]string{0: "i", 2: "x", 3: "acc"}
	if len(got) != len(want) {
		t.Fatalf("unitSlotNames = %v, want %v", got, want)
	}
	for slot, name := range want {
		if got[slot] != name {
			t.Errorf("slot %d = %q, want %q (table %v)", slot, got[slot], name, got)
		}
	}
}

// TestNoteLoopFreshArms pins NoteLoopFresh's two scope arms (NUR214): a scope
// opened at another unit depth carries nothing, and an enclosing armed loop of
// the same unit that already carries the name lends its cell — no fresh slot.
func TestNoteLoopFreshArms(t *testing.T) {
	joined := core.NewCarrier(core.TInteger)

	es := NewEmitState()
	es.loopCarried = []*loopCarriedScope{{unitDepth: 99, slots: map[string]int{}}}
	es.NoteLoopFresh("n", joined)
	if len(es.loopCarried[0].slots) != 0 || es.carriedNames["n"] {
		t.Fatal("a scope of another unit carries nothing")
	}

	es = NewEmitState()
	es.loopCarried = []*loopCarriedScope{
		{unitDepth: 1, slots: map[string]int{"n": 3}},
		{unitDepth: 1, slots: map[string]int{}},
	}
	es.NoteLoopFresh("n", joined)
	u := es.units[0]
	if es.loopCarried[1].slots["n"] != 3 || u.localByID[joined.ID] != 3 || u.numLocals != 0 {
		t.Fatalf("the enclosing loop's cell is reused: slots=%v local=%d numLocals=%d", es.loopCarried[1].slots, u.localByID[joined.ID], u.numLocals)
	}
	if !es.carriedNames["n"] || u.boundLocals[joined.ID] != "n" || !u.loopFresh[joined.ID] {
		t.Error("the fresh binding is carried and read bound-checked")
	}
}

// TestDynTrailAnchorsAtTheRead pins the trailing apply's anchor: the lead's
// own read token (`g` in `(5 g)`), where the interpreter's return check and
// no-match anchor — the lead value's position is only the fallback.
func TestDynTrailAnchorsAtTheRead(t *testing.T) {
	es := NewEmitState()
	unit, finish, _ := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, core.SrcPos{})
	arg := core.NewInteger(5)
	fn := core.NewDynamicCarrier(core.TFunction)
	es.producedBy[fn.ID] = producer{seq: 1}
	read := core.SrcPos{Row: 2, Col: 7}
	es.fnRecs[unit].noteWordReadName(fn.ID, "g", read)
	es.RegisterTrailingApply(fn.ID, 1)
	finish([]core.Value{arg, fn})
	if rec := es.fnRecs[unit]; rec.dynTrailPos != read {
		t.Errorf("the apply anchors at the read: %v, want %v", rec.dynTrailPos, read)
	}
}

// TestSetRootBodyCopies pins SetRootBody: nil-safe, and the seated tokens are
// a copy the caller's later edits do not reach.
func TestSetRootBodyCopies(t *testing.T) {
	var nilES *EmitState
	nilES.SetRootBody([]core.Value{core.NewInteger(1)})

	es := NewEmitState()
	body := []core.Value{core.NewInteger(1), core.NewInteger(2)}
	es.SetRootBody(body)
	body[0] = core.NewInteger(9)
	if len(es.rootBody) != 2 || es.rootBody[0].String() != "1" {
		t.Errorf("the root body is a copy: %v", es.rootBody)
	}
}

// TestRecordDynApplyNameNeedsAnArgument pins NUR162's guard on the name-routed
// apply: a window with no argument records nothing (a 0-arg fn value at a
// paren's tail is data on the interpreter).
func TestRecordDynApplyNameNeedsAnArgument(t *testing.T) {
	es := NewEmitState()
	fn := core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TFunction)), Parent: core.TFunction,
		Data: core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{Returns: []*core.Type{core.TInteger}}}}}
	if es.RecordDynApplyName("h", nil, fn, core.NewCarrier(core.TInteger), core.SrcPos{}) {
		t.Fatal("an apply over no argument records nothing")
	}
	if len(es.frames[0]) != 0 || !es.Compilable {
		t.Errorf("the decline leaves the stream and the verdict alone: %d events, compilable=%v", len(es.frames[0]), es.Compilable)
	}
}

// TestSiblingCallOutputsNeedsTwo pins siblingCallOutputs: an empty or
// single-entry residual is no sibling set, even when its lone entry is a user
// call's output; two outputs of one call are.
func TestSiblingCallOutputsNeedsTwo(t *testing.T) {
	es := NewEmitState()
	if es.siblingCallOutputs(nil) {
		t.Error("an empty residual has no siblings")
	}
	seq := es.appendEvent(EmitEvent{kind: evCallUser})
	a, b := core.NewCarrier(core.TInteger), core.NewCarrier(core.TInteger)
	es.setProducedAt(a, seq, 0)
	es.setProducedAt(b, seq, 1)
	if es.siblingCallOutputs([]core.Value{a}) {
		t.Error("a lone call output has no siblings")
	}
	if !es.siblingCallOutputs([]core.Value{a, b}) {
		t.Error("two outputs of one user call are siblings")
	}
}

// TestSeatDeoptPointUnpromotedGuard pins a GUARD point (deoptPoint.bail)
// tested at its push over a producer the unit did not promote: it has no slot
// to test at, so it tests before its statement instead, at the value's stack
// home. A deopt point in the same place seats nothing; a promoted source's
// guard tests at its slot.
func TestSeatDeoptPointUnpromotedGuard(t *testing.T) {
	rec := &fnUnitRec{promoted: map[int]int{4: 2}}
	flw := &lowerer{}
	seatDeoptPoint(flw, rec, deoptPoint{seq: 3, slot: -1, name: "j", atPush: true, bail: true})
	if len(flw.deopts) != 1 || flw.deopts[0].atPush || !flw.deopts[0].bail || flw.deopts[0].name != "j" {
		t.Fatalf("the guard tests before its statement: %+v", flw.deopts)
	}
	flw = &lowerer{}
	seatDeoptPoint(flw, rec, deoptPoint{seq: 3, slot: -1, name: "j", atPush: true})
	if len(flw.deopts) != 0 || len(flw.deoptAtSlot) != 0 {
		t.Errorf("an unpromoted deopt point seats nothing: %+v %+v", flw.deopts, flw.deoptAtSlot)
	}
	seatDeoptPoint(flw, rec, deoptPoint{seq: 4, slot: -1, name: "k", atPush: true, bail: true})
	if d, ok := flw.deoptAtSlot[2]; !ok || d.name != "k" {
		t.Errorf("a promoted source's guard tests at its slot: %+v", flw.deoptAtSlot)
	}
}

// TestEmbeddedEnclosingIDsSkipsAnEmptyMapMember pins the fn-unit literal's
// kept-member walk: a nested map member with no entries table embeds
// nothing, and an enclosing binding's container beside it is kept whole.
func TestEmbeddedEnclosingIDsSkipsAnEmptyMapMember(t *testing.T) {
	nilMap := core.Value{Parent: core.TMap, Data: core.MapPayload{M: nil}}
	member := core.NewList([]core.Value{core.NewInteger(9)})
	member.ID = "enc"
	lit := core.NewList([]core.Value{nilMap, core.NewList([]core.Value{nilMap, member})})
	keep := embeddedEnclosingIDs(lit, map[string]bool{"enc": true})
	if len(keep) != 1 || !keep["enc"] {
		t.Errorf("only the enclosing member is kept: %v", keep)
	}
}

// TestBranchArmsRetPinnedDivergingArms pins branchArmsRetPinned's arm rule: a
// diverging arm never reaches the merge, so it does not count against a
// ret-pinned sibling — but a branch whose arms BOTH diverge delivers no value
// at all and is not ret-pinned.
func TestBranchArmsRetPinnedDivergingArms(t *testing.T) {
	es := NewEmitState()
	brk := &EmitFragment{events: []EmitEvent{{kind: evBreak}}}
	cont := &EmitFragment{events: []EmitEvent{{kind: evContinue}}}
	if es.branchArmsRetPinned(core.BranchRecord{HasElse: true, Then: brk, Els: cont}) {
		t.Error("two diverging arms are not ret-pinned")
	}
	pinned := core.NewCarrier(core.TInteger)
	es.producedBy[pinned.ID] = producer{seq: 5}
	es.eventInfo[5] = eventFlags{dynBodyResult: true}
	if !es.branchArmsRetPinned(core.BranchRecord{HasElse: true, Then: &EmitFragment{}, ThenStk: []core.Value{pinned}, Els: brk}) {
		t.Error("a dyn-body result beside a diverging arm is ret-pinned")
	}
}

// TestRecordRuntimeDispatchUnresolvedOperand pins the run-time binder's
// record: with the latch set and every operand resolved, the dispatch is the
// plain 0-result call; an operand with no compiled home hands the dispatch to
// RecordCall, whose compile-time-word arm declines it.
func TestRecordRuntimeDispatchUnresolvedOperand(t *testing.T) {
	sig := &core.Signature{Impl: &core.GoImpl{RunInCheckMode: true}}
	pos := core.SrcPos{Row: 1, Col: 1}

	es := NewEmitState()
	src := core.NewCarrier(core.TMap)
	es.RegisterLocal(src.ID)
	es.pendingRuntimeBindCall = true
	es.RecordRuntimeDispatch("unpack", sig, []core.Value{core.NewInteger(1), src}, nil, pos)
	if evs := es.frames[0]; !es.Compilable || len(evs) != 1 || evs[0].call.word != "unpack" || len(evs[0].call.ops) != 2 || evs[0].call.nout != 0 {
		t.Fatalf("a resolved binder records its call: compilable=%v %+v", es.Compilable, es.frames[0])
	}

	es = NewEmitState()
	es.pendingRuntimeBindCall = true
	es.RecordRuntimeDispatch("unpack", sig, []core.Value{core.NewInteger(1), core.NewCarrier(core.TMap)}, nil, pos)
	if es.Compilable || es.Reason != "compile-time word unpack" || len(es.frames[0]) != 0 {
		t.Fatalf("an operand with no home declines as the compile-time word: %v %q %d", es.Compilable, es.Reason, len(es.frames[0]))
	}
	if es.pendingRuntimeBindCall {
		t.Error("the latch is consumed")
	}
}

// TestNameCarriedByABranchSlot pins nameCarried's unit arm: a name the unit
// carries in a frame slot for a branch join (nameSlots) — never a loop's —
// is carried all the same, so an undef of it declines.
func TestNameCarriedByABranchSlot(t *testing.T) {
	es := NewEmitState()
	es.units[0].setNameSlot("x", 0)
	if !es.nameCarried("x") || es.nameCarried("y") {
		t.Fatal("a branch-carried slot carries its name")
	}
	es.DeclineCarriedUndef("x")
	if es.Compilable || es.Reason != "undef of the loop-carried def `x` (Stage 3)" {
		t.Errorf("an undef of a carried name declines: %v %q", es.Compilable, es.Reason)
	}
}

// TestDynTrailSkipsAFnValuedArgument pins the body-tail trailing apply's
// argument rule: a window whose argument is itself a fn value is not lowered
// as the tail apply (the paren's collapse kept it as a token the interpreter
// stepped); a data argument is.
func TestDynTrailSkipsAFnValuedArgument(t *testing.T) {
	for _, c := range []struct {
		arg  core.Value
		want int
	}{
		{core.NewDynamicCarrier(core.TFunction), 0},
		{core.NewInteger(5), 1},
	} {
		es := NewEmitState()
		unit, finish, _ := es.StartFnCompile("k", "fn", nil, nil, nil, nil, nil, false, core.SrcPos{})
		fn := core.NewDynamicCarrier(core.TFunction)
		es.producedBy[fn.ID] = producer{seq: 2}
		if core.IsFnValueResidual(c.arg) {
			es.producedBy[c.arg.ID] = producer{seq: 1}
		}
		es.RegisterTrailingApply(fn.ID, 1)
		finish([]core.Value{c.arg, fn})
		if got := es.fnRecs[unit].dynTrailArity; got != c.want {
			t.Errorf("arg %v: dynTrailArity = %d, want %d", c.arg, got, c.want)
		}
	}
}

// TestTrailingWindowDeclinesAFnValuedArgument pins the trailing paren
// window's argument rule against the leading one's (RecordDynApplyLead): a
// fn-valued argument of a TRAILING window was a token the interpreter
// stepped, so the apply declines; the LEADING window's argument arrived inert
// and the lead collects it as the value it is.
func TestTrailingWindowDeclinesAFnValuedArgument(t *testing.T) {
	setup := func() (*EmitState, core.Value, core.Value) {
		es := NewEmitState()
		fn, arg := core.NewCarrier(core.TFunction), core.NewCarrier(core.TFunction)
		es.RegisterLocal(fn.ID)
		es.RegisterLocal(arg.ID)
		return es, fn, arg
	}
	es, fn, arg := setup()
	if n, ok := es.RecordDynApply([]core.Value{arg}, fn, core.NewCarrier(core.TAny), core.SrcPos{}); ok || n != 0 || len(es.frames[0]) != 0 {
		t.Fatalf("a trailing window over a fn-valued argument declines: %d %v", n, ok)
	}
	es, fn, arg = setup()
	if n, ok := es.RecordDynApplyLead([]core.Value{arg}, fn, core.NewCarrier(core.TAny), core.SrcPos{}); !ok || n != 1 || len(es.frames[0]) != 1 {
		t.Fatalf("a leading window collects its fn-valued argument: %d %v", n, ok)
	}
}

// TestContainerReadResultNilSafe pins ContainerReadResult on a nil recorder
// and on one that has produced nothing.
func TestContainerReadResultNilSafe(t *testing.T) {
	var nilES *EmitState
	if nilES.ContainerReadResult("id") || (&EmitState{}).ContainerReadResult("id") {
		t.Error("no recorder, no container read")
	}
}

// TestRecordArgsProjectionArms pins the `args` projection's record: nothing on
// a declined recorder or for an identity-less list, nothing when a param has
// no compiled home, and — recorded — its event remembered for the `args.N`
// fold, retracted while it is the frame's last event and kept once another
// event follows it.
func TestRecordArgsProjectionArms(t *testing.T) {
	param := core.NewCarrier(core.TInteger)
	list := func() core.Value {
		v := core.NewList([]core.Value{param})
		v.ID = core.GenerateID(core.IDPrefixForType(core.TList))
		return v
	}

	declined := NewEmitState()
	declined.MarkUncompilable("x")
	if declined.RecordArgsProjection(nil, []core.Value{param}, list(), core.SrcPos{}) {
		t.Error("a declined recorder records nothing")
	}
	es := NewEmitState()
	if es.RecordArgsProjection(nil, []core.Value{param}, core.NewList([]core.Value{param}), core.SrcPos{}) {
		t.Error("an identity-less list records nothing")
	}
	if es.RecordArgsProjection(nil, []core.Value{param}, list(), core.SrcPos{}) || len(es.argsProjSeq) != 0 {
		t.Error("a param with no compiled home records nothing")
	}

	es.RegisterLocal(param.ID)
	out := list()
	if !es.RecordArgsProjection(nil, []core.Value{param}, out, core.SrcPos{}) {
		t.Fatal("a projection over the param locals records")
	}
	if seq, ok := es.argsProjSeq[out.ID]; !ok || seq != es.producedBy[out.ID].seq {
		t.Fatalf("the projection's event is remembered: %v", es.argsProjSeq)
	}
	if !es.retractArgsProjection(out.ID) || len(es.frames[0]) != 0 {
		t.Fatal("the frame's last event is retracted")
	}
	kept := list()
	es.RecordArgsProjection(nil, []core.Value{param}, kept, core.SrcPos{})
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "w"}})
	if es.retractArgsProjection(kept.ID) || len(es.frames[0]) != 2 {
		t.Error("a projection another event follows stays put")
	}
}

// TestTrailingApplyParksACallResult pins the trailing arm's park rule: a user
// call's returned closure over one literal is PARKED where it lands, never
// applied to the value beneath — unless an `apply` word dispatched it.
func TestTrailingApplyParksACallResult(t *testing.T) {
	es := NewEmitState()
	es.frames = [][]EmitEvent{{{kind: evCallUser, uc: emitUserCall{nout: 1}, seq: 1}}}
	fnv := core.NewCarrier(core.TFunction)
	es.producedBy[fnv.ID] = producer{seq: 1}
	lw := &lowerer{vm: []vmSlot{{seq: 1, idx: 0}}}
	residual := []core.Value{core.NewInteger(7), fnv}
	if _, ok := es.trailingApply(lw, residual); ok {
		t.Error("a parked call result is data over the literal beneath")
	}
	es.appliedByWord = map[string]bool{fnv.ID: true}
	if rot, ok := es.trailingApply(lw, residual); !ok || rot[0].ID != fnv.ID {
		t.Errorf("an `apply` word applies the parked result: %v %v", rot, ok)
	}
}

// TestResidualSpliceReRead pins the program residual's splice gate: a
// dynamic splice's payload surfacing twice (the spread and a re-read of the
// payload def) declines; once, it resolves to the spread event.
func TestResidualSpliceReRead(t *testing.T) {
	es := NewEmitState()
	payload := core.NewCarrier(core.TList)
	es.producedBy[payload.ID] = producer{seq: 1}
	es.eventInfo[1] = eventFlags{spliceDyn: true}
	if _, reason := es.resolveResidualOperands(&lowerer{}, []core.Value{payload, payload}); reason != "splice payload re-read after the spread (Stage 2)" {
		t.Errorf("a re-read after the spread declines: %q", reason)
	}
	ops, reason := es.resolveResidualOperands(&lowerer{}, []core.Value{payload})
	if reason != "" || len(ops) != 1 || ops[0].kind != opEvent || ops[0].idx != 1 {
		t.Errorf("the spread alone resolves to its event: %v %q", ops, reason)
	}
}

// TestCallResultRenderKnownOffFrame pins callResultRenderKnown over a value
// whose producing event is not in the current frame: no event to read, no
// known render.
func TestCallResultRenderKnownOffFrame(t *testing.T) {
	es := NewEmitState()
	v := core.NewCarrier(core.TFunction)
	es.producedBy[v.ID] = producer{seq: 9}
	if es.callResultRenderKnown(v) {
		t.Error("a producer outside the current frame has no known render")
	}
}

// TestApplyChainStepsShapes pins the apply chain's admission (fnUnitRec.
// applyChain): every pending fn in the residual in order, the last on top,
// every operand re-pushable, the first step with an argument beneath it, no
// fn-valued argument, and nothing left over — any other shape is no chain.
func TestApplyChainStepsShapes(t *testing.T) {
	val := func(id string, fn bool) core.Value {
		v := core.NewCarrier(core.TInteger)
		if fn {
			v = core.NewCarrier(core.TFunction)
		}
		v.ID = id
		return v
	}
	x, h, f, g := val("x", false), val("h", true), val("f", true), val("g", true)
	pend := []pendingApply{{id: "f"}, {id: "g", pos: core.SrcPos{Row: 1, Col: 9}}}
	locals := func(n int) []EmitOperand {
		ops := make([]EmitOperand, n)
		for i := range ops {
			ops[i] = localOperand(i)
		}
		return ops
	}
	steps := applyChainSteps(pend, []core.Value{x, f, g}, locals(3))
	if len(steps) != 2 || steps[0].n != 1 || len(steps[0].ops) != 2 || steps[1].n != 1 || len(steps[1].ops) != 1 || steps[1].pos.Col != 9 {
		t.Fatalf("x f/v apply g/v apply is a two-step chain: %+v", steps)
	}
	withEvent := locals(3)
	withEvent[0] = EventOperand(4, 0)
	for name, got := range map[string][]applyStep{
		"a single pending apply":         applyChainSteps(pend[:1], []core.Value{x, f}, locals(2)),
		"an operand count off the stack": applyChainSteps(pend, []core.Value{x, f, g}, locals(2)),
		"a last fn not on top":           applyChainSteps(pend, []core.Value{x, g, f}, locals(3)),
		"an event operand":               applyChainSteps(pend, []core.Value{x, f, g}, withEvent),
		"a pending fn not in the stack":  applyChainSteps([]pendingApply{{id: "z"}, {id: "g"}}, []core.Value{x, f, g}, locals(3)),
		"a first step with no argument":  applyChainSteps(pend, []core.Value{f, g}, locals(2)),
		"a fn-valued argument":           applyChainSteps(pend, []core.Value{h, f, g}, locals(3)),
		"a value left above the chain":   applyChainSteps(pend, []core.Value{x, f, g, g}, locals(4)),
	} {
		if got != nil {
			t.Errorf("%s is no chain: %+v", name, got)
		}
	}
}
