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
