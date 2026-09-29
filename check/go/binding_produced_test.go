package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// producedRecorder is an armed-recorder double whose only override is
// AlreadyProduced: the IDs a recorded dispatch event produced.
type producedRecorder struct {
	core.EmitRecorder
	produced map[string]bool
}

func (p *producedRecorder) AlreadyProduced(id string) bool { return p.produced[id] }

// TestExprRefsCarrierProducedBinding pins the const-fold veto's produced arm
// (NUR331): a container fold that reads a def binding whose value a recorded
// event produced (`def s (Rand.with-seed 3)`) declines, because the binding's
// check-time value is the MODEL of the event's output — a shape-only instance
// for Rand.with-seed — not the run's value. A concrete binding no event
// produced (a literal's) still folds, and an ID-less binding is never
// mistaken for a produced one.
func TestExprRefsCarrierProducedBinding(t *testing.T) {
	r := newTestRegistry(t)
	defer r.Check.Begin()()
	e := core.NewTop(r)

	model := mapOf("k", core.NewInteger(1))
	literal := mapOf("k", core.NewInteger(2))
	idless := core.NewInteger(3)
	idless.ID = ""
	r.Defs.Push("s", model)
	r.Defs.Push("lit", literal)
	r.Defs.Push("n", idless)
	r.Check.Emit = &producedRecorder{EmitRecorder: core.TheInactiveEmit, produced: map[string]bool{model.ID: true, "": true}}

	if !exprRefsCarrier(e, []core.Value{core.NewWord("s")}) {
		t.Error("a read of an event-produced binding must veto the fold")
	}
	if exprRefsCarrier(e, []core.Value{core.NewWord("lit")}) {
		t.Error("a literal's binding still folds")
	}
	if exprRefsCarrier(e, []core.Value{core.NewWord("n")}) {
		t.Error("an ID-less binding is not a produced one")
	}
}
