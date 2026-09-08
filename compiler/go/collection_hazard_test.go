package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestCollectionHazardNote pins the side table's edges: an inactive or nil
// state and an empty id record nothing, and a marked id reads back.
func TestCollectionHazardNote(t *testing.T) {
	var nilES *EmitState
	if nilES.CollectionHazard("x") {
		t.Error("a nil state has no hazards")
	}
	es := NewEmitState()
	es.NoteCollectionHazard("")
	if es.collectionHazard != nil {
		t.Error("an empty id must not allocate the table")
	}
	es.Compilable = false
	es.NoteCollectionHazard("x")
	if es.CollectionHazard("x") {
		t.Error("an inactive state records nothing")
	}
	es.Compilable = true
	es.NoteCollectionHazard("x")
	if !es.CollectionHazard("x") || es.CollectionHazard("y") {
		t.Errorf("marked ids read back exactly: %v", es.collectionHazard)
	}
	// applyPending: a nil state and a state with no open unit hold no
	// pending apply; a pending id in the innermost unit reads back.
	if nilES.applyPending("x") || (&EmitState{}).applyPending("x") {
		t.Error("no unit, no pending apply")
	}
	es.units[len(es.units)-1].pendingApply = append(es.units[len(es.units)-1].pendingApply, pendingApply{id: "x"})
	if !es.applyPending("x") || es.applyPending("y") {
		t.Error("a pending apply reads back exactly")
	}
	if es.hazardLead(core.Value{ID: "x"}) {
		t.Error("an apply-pending value is the apply word's, never a hazard lead")
	}
}

// TestHazardLeadDeclinesEveryLowering pins the three compiler-side
// consumers of the note (NUR121): RecordDynApply declines a marked fn (the
// paren lead window records through it), and resolveDynamicApply refuses a
// marked fn-carrier lead and a marked dynamic lead at the residual — while
// the same leads unmarked keep their lowering.
func TestHazardLeadDeclinesEveryLowering(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, p: &Program{}}
	lead := core.NewCarrier(core.TFunction)
	residual := []core.Value{lead, core.NewCarrier(core.TInteger)}
	if _, op, reason := es.resolveDynamicApply(lw, residual); op != OpCallDynamic || reason != "" {
		t.Fatalf("an unmarked carrier lead keeps the lowering: op=%v reason=%q", op, reason)
	}
	es.NoteCollectionHazard(lead.ID)
	if _, op, reason := es.resolveDynamicApply(lw, residual); op != 0 || !strings.Contains(reason, "NUR121") {
		t.Errorf("a marked carrier lead must refuse: op=%v reason=%q", op, reason)
	}
	dyn := core.NewDynamicCarrier(core.TAny)
	es.NoteCollectionHazard(dyn.ID)
	if _, op, reason := es.resolveDynamicApply(lw, []core.Value{dyn, core.NewInteger(5)}); op != 0 || !strings.Contains(reason, "NUR121") {
		t.Errorf("a marked dynamic lead must refuse: op=%v reason=%q", op, reason)
	}
	// RecordDynApply: the marked fn declines without marking the program.
	out := core.NewCarrier(core.TInteger)
	if _, ok := es.RecordDynApply([]core.Value{core.NewInteger(5)}, lead, out, core.SrcPos{}); ok {
		t.Error("a marked lead must decline the apply record")
	}
	if !es.Compilable {
		t.Errorf("a decline leaves the program compilable (the window stays for the downstream machinery): %q", es.Reason)
	}
}
