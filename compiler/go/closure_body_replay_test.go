package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// closureBodyUnit is a code-body closure unit with ONE unnamed input (an
// each body's element) — the quotation-body container reads' fixture
// (2026-09-22).
func closureBodyUnit(t *testing.T) (*EmitState, *emitUnit, *fnUnitRec, core.Value) {
	t.Helper()
	es := NewEmitState()
	elem := core.NewDynamicCarrier(core.TAny)
	unit, _, ok := es.StartFnCompile("k", "each$body", nil, []core.Value{elem}, nil, []string{""}, nil, false, core.SrcPos{})
	if !ok || unit < 0 {
		t.Fatalf("StartFnCompile declined: %d %v", unit, ok)
	}
	rec := es.fnRecs[unit]
	rec.closure = true
	return es, es.units[len(es.units)-1], rec, elem
}

// testRegistry is a fresh registry whose check state the placement marks
// live on.
func testRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// memberRead is a gradual carrier the check pass tagged as a fn-valued
// member read — `ops.inc`'s value in the body.
func memberRead(es *EmitState, member core.Value) core.Value {
	v := core.NewDynamicCarrier(core.TAny)
	v.ID = "T_member_" + core.GenerateID("m")
	es.NoteMemberFnRead(v.ID, member)
	return v
}

// TestNoteClosureBodyReplayArms pins the closure-body trigger: a tagged
// member read above the element arms the whole-frame replay over the token
// region; every other top keeps the unit as it was.
func TestNoteClosureBodyReplayArms(t *testing.T) {
	inc := core.NewValueRaw(core.TFunction, core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}, Returns: []*core.Type{core.TInteger}}}})
	es, u, rec, elem := closureBodyUnit(t)
	top := memberRead(es, inc)
	es.noteClosureBodyReplay(u, rec, []core.Value{elem, top})
	if rec.dynFrameW != 1 || !rec.retReplay {
		t.Errorf("a tagged member read above the element arms the replay over one token: w=%d replay=%v", rec.dynFrameW, rec.retReplay)
	}
	// A fn-typed tagged read arms too.
	es, u, rec, elem = closureBodyUnit(t)
	fnTop := core.NewCarrier(core.TFunction)
	fnTop.ID = "T_fn_member"
	es.NoteMemberFnRead(fnTop.ID, inc)
	es.noteClosureBodyReplay(u, rec, []core.Value{elem, fnTop})
	if rec.dynFrameW != 1 {
		t.Errorf("a fn-typed tagged read arms: w=%d", rec.dynFrameW)
	}
	// Data beneath that is not the input widens the token region.
	es, u, rec, elem = closureBodyUnit(t)
	top = memberRead(es, inc)
	es.noteClosureBodyReplay(u, rec, []core.Value{core.NewInteger(5), top})
	if rec.dynFrameW != 2 {
		t.Errorf("a literal beneath the read is part of the token region: w=%d", rec.dynFrameW)
	}
	_ = elem
	// Declines: an empty residual, an untagged gradual top, an untagged
	// fn-typed top (a captured word read is the interpreter's WORD
	// dispatch), a placed top, a 0-arg-only member (the landing's).
	for name, tc := range map[string]func(es *EmitState) []core.Value{
		"empty":            func(es *EmitState) []core.Value { return nil },
		"untagged gradual": func(es *EmitState) []core.Value { return []core.Value{core.NewDynamicCarrier(core.TAny)} },
		"untagged fn":      func(es *EmitState) []core.Value { return []core.Value{core.NewCarrier(core.TFunction)} },
		"tagged data": func(es *EmitState) []core.Value {
			v := memberRead(es, inc)
			v.Dynamic = false
			v.Parent = core.TInteger
			return []core.Value{v}
		},
		"zero-arg member": func(es *EmitState) []core.Value {
			z := core.NewValueRaw(core.TFunction, core.FnDefInfo{Name: "z", Signatures: []core.Signature{{Returns: []*core.Type{core.TInteger}}}})
			return []core.Value{memberRead(es, z)}
		},
		"placed": func(es *EmitState) []core.Value {
			v := memberRead(es, inc)
			es.reg = testRegistry(t)
			es.reg.Check.ParenPlacedFnIDs = map[string]bool{v.ID: true}
			return []core.Value{v}
		},
	} {
		es, u, rec, _ := closureBodyUnit(t)
		es.noteClosureBodyReplay(u, rec, tc(es))
		if rec.dynFrameW != 0 || rec.retReplay {
			t.Errorf("%s: must not arm (w=%d)", name, rec.dynFrameW)
		}
	}
}

// TestNoteDynFrameReplayPlaced pins NUR182 on the fn-unit replay: a
// paren-placed value no enclosing paren re-stepped is data — skipped as an
// applicable (the count mismatch then compiles and the RET enforces it) —
// and a window that would carry one beside an applicable declines.
func TestNoteDynFrameReplayPlaced(t *testing.T) {
	es, rec, g, _ := wordReadUnit(t)
	u := es.units[len(es.units)-1]
	es.reg = testRegistry(t)
	placed := core.NewDynamicCarrier(core.TAny)
	placed.ID = "T_placed"
	es.reg.Check.ParenPlacedFnIDs = map[string]bool{placed.ID: true}
	if !es.noteDynFrameReplay(u, rec, []core.Value{core.NewInteger(5), placed}, 1) || rec.dynFrameW != 0 {
		t.Errorf("a placed value alone is data: no replay, no decline (w=%d)", rec.dynFrameW)
	}
	if es.noteDynFrameReplay(u, rec, []core.Value{placed, g}, 1) || rec.dynFrameW != 0 {
		t.Errorf("an applicable beside a placed value cannot arm — the island would re-step both (w=%d)", rec.dynFrameW)
	}
	if !es.windowHasPlaced([]core.Value{core.NewInteger(1), placed}) || es.windowHasPlaced([]core.Value{g}) {
		t.Error("windowHasPlaced reads the placed mark")
	}
	// The same value RE-STEPPED by an enclosing paren is an applicable again.
	es.reg.Check.ParenReSteppedFnIDs = map[string]bool{placed.ID: true}
	if es.windowHasPlaced([]core.Value{placed}) {
		t.Error("a re-stepped value is not placed")
	}
}

// TestResidualForceOrderPlaced pins the promotion's data predicate: a
// placed value no longer bails the out-of-order promotion — except inside
// a body whose driver returns the residual to the caller's tape
// (residualToCaller, `do`) when it has siblings.
func TestResidualForceOrderPlaced(t *testing.T) {
	es, rec, _, _ := wordReadUnit(t)
	es.reg = testRegistry(t)
	placed := core.NewDynamicCarrier(core.TAny)
	placed.ID = "T_placed2"
	es.reg.Check.ParenPlacedFnIDs = map[string]bool{placed.ID: true}
	// [literal, placed]: the placed event result above the inert literal
	// is the out-of-order shape the promotion re-pushes in order.
	ops := []EmitOperand{{kind: opLocal}, EventOperand(3, 0)}
	vals := []core.Value{core.NewInteger(5), placed}
	if residualForceOrder(ops, vals, nil) != nil {
		t.Error("with no data predicate a dynamic value bails the promotion")
	}
	if got := es.residualForceOrderFor(0, rec, ops, vals); got == nil || !got[3] {
		t.Errorf("a placed value is data for a fn unit: the promotion runs, got %v", got)
	}
	rec.closure = true
	rec.residualToCaller = true
	if got := es.residualForceOrderFor(0, rec, ops, vals); got != nil {
		t.Errorf("a do body's placed value with a sibling keeps the bail (the caller re-steps it): %v", got)
	}
	if got := es.residualForceOrderFor(0, rec, ops[:1], vals[:1]); got != nil {
		t.Errorf("one operand is never out of order: %v", got)
	}
	rec.residualToCaller = false
	if got := es.residualForceOrderFor(0, rec, ops, vals); got == nil {
		t.Error("an each body's placed value is data")
	}
	if es.residualForceOrderFor(1, rec, ops, vals) != nil || es.residualForceOrderFor(0, rec, ops[:1], vals) != nil {
		t.Error("a trailing apply or a count mismatch takes no promotion")
	}
}

// TestProducedFnCarrierInFnUnit pins the `apply` word's pending
// registration for a produced fn-typed carrier: it fires inside a fn unit
// or a plain lambda (below the program's unit) for a carrier a call of this
// pass produced, and stands aside at the program level, for a value no
// event produced, for data, and inside a code-body closure unit (whose own
// gate declines the probe so the body runs as a raw token list).
func TestProducedFnCarrierInFnUnit(t *testing.T) {
	es := NewEmitState()
	produced := core.NewCarrier(core.TFunction)
	produced.ID = "T_factory_result"
	es.producedBy = map[string]producer{produced.ID: {seq: 7}}
	if es.producedFnCarrierInFnUnit(produced) {
		t.Error("the program unit keeps its residual arms")
	}
	es, rec, _, _ := wordReadUnit(t)
	es.producedBy = map[string]producer{produced.ID: {seq: 7}}
	if !es.producedFnCarrierInFnUnit(produced) {
		t.Error("a produced fn carrier inside a fn unit registers")
	}
	unproduced := core.NewCarrier(core.TFunction)
	unproduced.ID = "T_param"
	if es.producedFnCarrierInFnUnit(unproduced) || es.producedFnCarrierInFnUnit(core.NewInteger(5)) {
		t.Error("an unproduced carrier or data never registers here")
	}
	rec.closure = true
	if es.producedFnCarrierInFnUnit(produced) {
		t.Error("a code-body closure unit is left to its own gate")
	}
	rec.lambdaUnit = true
	if !es.producedFnCarrierInFnUnit(produced) {
		t.Error("a plain lambda is a fn unit")
	}
}

// TestUndoProbeStamps pins the probe fork's stamp undo: the refs a probe
// state stamped onto shared sig impls are cleared when the probe is
// discarded, so the real pass stamps them itself and Finalize seats their
// Program.
func TestUndoProbeStamps(t *testing.T) {
	es := NewEmitState()
	impl := &core.BoruImpl{Body: []core.Value{core.NewInteger(1)}}
	impl.SetCompiled(&CompiledFnRef{Unit: 3})
	es.stampImpls = map[*core.BoruImpl]int{impl: 3}
	es.undoProbeStamps()
	if impl.Compiled() != nil || es.stampImpls != nil {
		t.Errorf("the probe's stamp is cleared: compiled=%v table=%v", impl.Compiled(), es.stampImpls)
	}
	es.undoProbeStamps() // idempotent on an empty table
}

// TestNoteWordReadDefReadName pins NUR156's recorder arm: a bare read of an
// ENCLOSING-scope binding names itself on the unit only when it is a
// def-table read (NoteDefRead), and never counts strictly — the value is not
// this unit's to seat. A non-def enclosing read still records nothing.
func TestNoteWordReadDefReadName(t *testing.T) {
	es, _, rec, _ := closureBodyUnit(t)
	outer := core.NewCarrier(core.TFunction)
	outer.ID = "T_outer_fn"
	pos := core.SrcPos{Row: 1, Col: 7}
	es.NoteWordRead(outer, "f", pos)
	if rec.wordReadNames[outer.ID] != "" || rec.wordReads[outer.ID] != 0 {
		t.Errorf("an enclosing-scope read that is no def read records nothing: %q %d", rec.wordReadNames[outer.ID], rec.wordReads[outer.ID])
	}
	es.NoteDefRead(outer.ID, "f")
	es.NoteWordRead(outer, "f", pos)
	if rec.wordReadNames[outer.ID] != "f" || rec.wordReadPos[outer.ID] != pos || rec.wordReadFirst[outer.ID] != pos {
		t.Errorf("a def read names itself at the read: %q %v %v", rec.wordReadNames[outer.ID], rec.wordReadPos[outer.ID], rec.wordReadFirst[outer.ID])
	}
	if rec.wordReads[outer.ID] != 0 {
		t.Errorf("a def read is never accounted strictly: %d", rec.wordReads[outer.ID])
	}
	// A second read keeps the FIRST position.
	es.NoteWordRead(outer, "f", core.SrcPos{Row: 2, Col: 1})
	if rec.wordReadFirst[outer.ID] != pos {
		t.Errorf("wordReadFirst keeps the first read: %v", rec.wordReadFirst[outer.ID])
	}
}

// TestNoteClosureBodyReplayDefRead pins the trigger's def-read arm: a
// tagged member read that arrived through a def-table read arms only under
// its binding NAME, and the window then carries the word table (the VM
// re-steps the entry as the word: `cannot call `f“ on a no-match).
func TestNoteClosureBodyReplayDefRead(t *testing.T) {
	inc := core.NewValueRaw(core.TFunction, core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{Params: []core.FnParam{{Type: core.TInteger}}, Returns: []*core.Type{core.TInteger}}}})
	es, u, rec, elem := closureBodyUnit(t)
	top := memberRead(es, inc)
	es.NoteDefRead(top.ID, "f")
	es.noteClosureBodyReplay(u, rec, []core.Value{elem, top})
	if rec.dynFrameW != 0 || rec.retReplay {
		t.Errorf("a def read with no word name is the read model's: must not arm (w=%d)", rec.dynFrameW)
	}
	es.NoteWordRead(top, "f", core.SrcPos{Row: 1, Col: 3})
	es.noteClosureBodyReplay(u, rec, []core.Value{elem, top})
	if rec.dynFrameW != 1 || !rec.retReplay {
		t.Fatalf("a NAMED def read of a tagged member arms the replay: w=%d replay=%v", rec.dynFrameW, rec.retReplay)
	}
	if len(rec.dynFrameWords) != 1 || rec.dynFrameWords[0].Name != "f" {
		t.Errorf("the window carries the word table under the binding name: %+v", rec.dynFrameWords)
	}
}
