package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// val_read_alias_test.go pins the thirty-first increment's recorder pieces:
// a `/v` read of a def-bound PRODUCED fn value is a fresh wrap of the
// binding (ResolveRef mints a new Value over the dispatch aggregate), so
// the read's ID carries no provenance; the binding unit remembers the
// bind's producer by name (noteValBind) and the read is aliased to it
// while the binding is still the bind's (aliasValRead), and the aliased
// read is data at an argument slot (argIsProducedClosure).

func TestNoteValBindArms(t *testing.T) {
	es := NewEmitState()
	cur := es.units[0]
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true})
	fn.ID = "T_fn" // values mint no id outside a check pass
	es.noteValBind(cur, "p", fn)
	if cur.valBinds != nil {
		t.Fatal("no registry: nothing recorded")
	}
	es.reg = newTestRegistry(t)
	es.noteValBind(cur, "p", core.NewInteger(1))
	es.noteValBind(cur, "p", fn)
	if len(cur.valBinds) != 0 {
		t.Fatalf("a non-fn value and a fn value no event produced record nothing: %v", cur.valBinds)
	}
	es.producedBy[fn.ID] = producer{seq: 3}
	es.reg.Defs.Push("p", fn)
	es.valBindEpoch = map[string]int{"p": 2}
	es.noteValBind(cur, "p", fn)
	b, ok := cur.valBinds["p"]
	if !ok || b.pr.seq != 3 || b.top != fn.ID || b.gen != es.reg.Defs.Gen("p") || b.epoch != 2 {
		t.Fatalf("a produced fn value records its producer, the entry on top, the generation and the epoch: %+v", b)
	}
	// A bind inside a branch or loop body drops the entry for good.
	es.reg.Check.CondBodyDepth = 1
	es.noteValBind(cur, "p", fn)
	if _, ok := cur.valBinds["p"]; ok {
		t.Fatal("a conditional bind drops the entry")
	}
	es.reg.Check.CondBodyDepth = 0
	// A later bind of the name to something else drops it too.
	es.noteValBind(cur, "p", fn)
	es.noteValBind(cur, "p", core.NewInteger(1))
	if _, ok := cur.valBinds["p"]; ok {
		t.Fatal("a rebind to a plain value drops the entry")
	}
}

func TestAliasValReadArms(t *testing.T) {
	es := NewEmitState()
	es.aliasValRead("r0", "")
	es.aliasValRead("r0", "p")
	if len(es.producedBy) != 0 {
		t.Fatal("no name, no registry: nothing aliased")
	}
	bare := &EmitState{reg: newTestRegistry(t)}
	bare.aliasValRead("r0", "p")
	if bare.producedBy != nil {
		t.Fatal("no unit: nothing aliased")
	}
	es.reg = newTestRegistry(t)
	es.aliasValRead("r0", "p")
	if _, ok := es.producedBy["r0"]; ok {
		t.Fatal("no bind of the name: nothing aliased")
	}
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true})
	fn.ID = "T_fn"
	es.producedBy[fn.ID] = producer{seq: 3}
	es.reg.Defs.Push("p", fn)
	es.valBindEpoch = map[string]int{"p": 1}
	cur := es.units[0]
	es.noteValBind(cur, "p", fn)
	// A later dyn-bind of the name anywhere (the epoch) stales the entry.
	es.valBindEpoch["p"] = 2
	es.aliasValRead("r1", "p")
	if _, ok := es.producedBy["r1"]; ok {
		t.Fatal("a moved epoch: nothing aliased")
	}
	es.valBindEpoch["p"] = 1
	// The generation unchanged: aliased, and remembered as a value read.
	es.aliasValRead("r1", "p")
	if pr, ok := es.producedBy["r1"]; !ok || pr.seq != 3 || !es.valReadIDs["r1"] {
		t.Fatalf("an unmoved binding aliases the read to the bind's producer: %+v %v", pr, es.valReadIDs)
	}
	// The generation moved but the bind's entry is on top again (a frame
	// binding of the name came and went): aliased.
	es.reg.Defs.Push("p", core.NewInteger(9))
	es.reg.Defs.PopEntry("p")
	es.aliasValRead("r2", "p")
	if pr, ok := es.producedBy["r2"]; !ok || pr.seq != 3 {
		t.Fatal("the same entry on top again aliases")
	}
	// Another entry on top (a rebind the recorder did not see): not aliased;
	// nor one with no identity to compare.
	other := core.NewInteger(9)
	other.ID = "T_other"
	es.reg.Defs.Push("p", other)
	es.aliasValRead("r3", "p")
	es.reg.Defs.PopEntry("p")
	es.reg.Defs.Push("p", core.NewInteger(9))
	es.aliasValRead("r3", "p")
	if _, ok := es.producedBy["r3"]; ok {
		t.Fatal("another entry on top, or one without an id: nothing aliased")
	}
	// The name unbound: not aliased.
	es.reg.Defs.PopEntry("p")
	es.reg.Defs.PopEntry("p")
	es.aliasValRead("r4", "p")
	if _, ok := es.producedBy["r4"]; ok {
		t.Fatal("an unbound name: nothing aliased")
	}
}

func TestNoteValReadAliasesWithoutAnOpenUnit(t *testing.T) {
	es := NewEmitState()
	es.reg = newTestRegistry(t)
	fn := core.NewFunction(core.FnDefInfo{Anonymous: true})
	fn.ID = "T_fn"
	es.producedBy[fn.ID] = producer{seq: 3}
	es.reg.Defs.Push("p", fn)
	es.noteValBind(es.units[0], "p", fn)
	es.NoteValRead("r1", "p")
	if pr, ok := es.producedBy["r1"]; !ok || pr.seq != 3 || len(es.fnRecs) != 0 {
		t.Fatalf("the program unit's read aliases with no fn unit open: %+v", pr)
	}
	es.NoteValRead("", "p")
	if _, ok := es.producedBy[""]; ok {
		t.Fatal("an empty id records nothing")
	}
}

func TestArgIsProducedClosureSkipsValRead(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = append(es.fnRecs, &fnUnitRec{outOps: []EmitOperand{{kind: opClosure, closureUnit: 1}}}, &fnUnitRec{nParams: 1})
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 0, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}})
	v := core.NewFunction(core.FnDefInfo{Anonymous: true})
	v.ID = "T_closure"
	es.producedBy[v.ID] = producer{seq: 0}
	sig := &core.Signature{Args: []*core.Type{core.TAny}}
	if !es.argIsProducedClosure("typeof", sig, []core.Value{v}) || es.Compilable {
		t.Fatal("a produced closure at an Any slot refuses")
	}
	es.Compilable = true
	es.valReadIDs = map[string]bool{v.ID: true}
	if es.argIsProducedClosure("typeof", sig, []core.Value{v}) || !es.Compilable {
		t.Fatal("a `/v` read of a def-bound closure is data at the slot")
	}
}
