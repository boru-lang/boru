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
	r.Check.NoteFnShape(out, 2)
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
