package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fn_shape_claim_test.go pins the thirty-fifth increment's compiler pieces:
// producerReturnedClosureArity's third source — a computed-fn carrier's
// arity CLAIMED by its producing word's check-mode ReturnsFn
// (CheckState.FnShapes) — and the def-read name accessor the read model
// keys on (DefReadName).

func TestProducerReturnedClosureArityClaim(t *testing.T) {
	es := NewEmitState()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.reg = r
	defer r.Check.Begin()()
	out := core.NewCarrier(core.TFunction)
	r.Check.NoteFnShape(out, core.FnShape{Arity: 2})
	if n, ok := es.producerReturnedClosureArity(out.ID); !ok || n != 2 {
		t.Errorf("a claimed carrier's arity = %d/%v, want 2", n, ok)
	}
	if _, ok := es.producerReturnedClosureArity("T_unclaimed"); ok {
		t.Error("an unclaimed, unproduced ID has no arity")
	}
	es.reg = nil
	if _, ok := es.producerReturnedClosureArity(out.ID); ok {
		t.Error("with no registry there is no claim table to consult")
	}
}

func TestDefReadName(t *testing.T) {
	es := NewEmitState()
	if _, ok := es.DefReadName("T_x"); ok {
		t.Error("no reads recorded")
	}
	es.NoteDefRead("T_x", "k")
	if name, ok := es.DefReadName("T_x"); !ok || name != "k" {
		t.Errorf("DefReadName = %q/%v, want k", name, ok)
	}
}

// closureOpShape: a closure unit's param count, the chain through its single
// closure out-op, a const lambda, and the shapes that are none.
func TestClosureOpShapeArms(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{
		{nParams: 1, outOps: []EmitOperand{{kind: opClosure, closureUnit: 1}}}, // mk2's first level
		{nParams: 2, outOps: []EmitOperand{{kind: opEvent, idx: 0}}},           // its second, returning a value
		{nParams: 3, outOps: []EmitOperand{{kind: opEvent, idx: 0}, {kind: opEvent, idx: 1}}},
	}
	s, ok := es.closureOpShape(EmitOperand{kind: opClosure, closureUnit: 0}, 0)
	if !ok || s.Arity != 1 || s.Result == nil || s.Result.Arity != 2 || s.Result.Result != nil {
		t.Errorf("a factory of factories claims the chain, got %+v/%v", s, ok)
	}
	if s, ok := es.closureOpShape(EmitOperand{kind: opClosure, closureUnit: 2}, 0); !ok || s.Arity != 3 || s.Result != nil {
		t.Errorf("a two-value body has no result shape, got %+v/%v", s, ok)
	}
	if _, ok := es.closureOpShape(EmitOperand{kind: opClosure, closureUnit: 9}, 0); ok {
		t.Error("a unit beyond the program is no shape")
	}
	if _, ok := es.closureOpShape(EmitOperand{kind: opEvent, idx: 0}, 0); ok {
		t.Error("an event result is no shape")
	}
	if _, ok := es.closureOpShape(EmitOperand{kind: opClosure, closureUnit: 0}, 9); ok {
		t.Error("the recursion is bounded")
	}
	lam := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Params: []core.FnParam{{Type: core.TInteger}, {Type: core.TInteger}}, Impl: core.Boru([]core.Value{core.NewWord("add")}),
	}}})
	es.consts = []core.Value{lam}
	if s, ok := es.closureOpShape(EmitOperand{kind: opConst, idx: 0}, 0); !ok || s.Arity != 2 {
		t.Errorf("a const lambda's param count, got %+v/%v", s, ok)
	}
	if _, ok := es.closureOpShape(EmitOperand{kind: opConst, idx: 5}, 0); ok {
		t.Error("a const beyond the table is no shape")
	}
}

// noteClosureShapeBind: the claim at a def of a produced closure, and every
// case that writes none.
func TestNoteClosureShapeBindArms(t *testing.T) {
	es := NewEmitState()
	// the factory unit (mk) returns the closure unit; the closure takes one param
	es.fnRecs = []*fnUnitRec{
		{nParams: 1, outOps: []EmitOperand{{kind: opClosure, closureUnit: 1}}},
		{nParams: 1, outOps: []EmitOperand{{kind: opEvent, idx: 0}}},
	}
	es.producedBy = map[string]producer{"T_h": {seq: 41}}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 41, kind: evCallUser, uc: emitUserCall{nout: 1, unit: 0}})
	carrier := core.NewCarrier(core.TFunction)
	carrier.ID = "T_h"
	es.noteClosureShapeBind(carrier) // no registry: nothing to write to
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.reg = r
	defer r.Check.Begin()()
	plain := core.NewInteger(1)
	plain.ID = "T_h"
	es.noteClosureShapeBind(plain) // not a fn carrier
	if _, ok := r.Check.FnShapeOf("T_h"); ok {
		t.Fatal("a plain value claims no shape")
	}
	es.noteClosureShapeBind(carrier)
	if n, ok := r.Check.FnShapeArity("T_h"); !ok || n != 1 {
		t.Errorf("a def of the produced closure claims its unit's arity, got %d/%v", n, ok)
	}
	// A standing claim (a producing word's own) is kept.
	r.Check.NoteFnShape(carrier, core.FnShape{Arity: 5})
	es.noteClosureShapeBind(carrier)
	if n, _ := r.Check.FnShapeArity("T_h"); n != 5 {
		t.Error("a producing word's claim stands")
	}
	unknown := core.NewCarrier(core.TFunction)
	unknown.ID = "T_native"
	es.noteClosureShapeBind(unknown) // no producer
	if _, ok := r.Check.FnShapeOf("T_native"); ok {
		t.Error("an unproduced carrier claims nothing")
	}
}
