package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// producedRecorder is an armed-recorder double whose only override is
// AlreadyProduced: the recorder keys a recorded dispatch event stamped.
type producedRecorder struct {
	core.EmitRecorder
	produced map[core.RecKey]bool
}

func (p *producedRecorder) AlreadyProduced(k core.RecKey) bool { return p.produced[k] }

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

	// The event stamps its output's holder (no identity change follows, so
	// the key stands); a literal's binding carries no key. The ID-less
	// binding carries a key the recorder vouches for, so only its missing
	// identity keeps it from reading as produced.
	model := mapOf("k", core.NewInteger(1))
	core.SetRecKey(&model, core.RecKey{Pass: 1, Seq: 1})
	literal := mapOf("k", core.NewInteger(2))
	idless := core.NewInteger(3)
	core.SetID(&idless, "")
	core.SetRecKey(&idless, core.RecKey{Pass: 1, Seq: 2})
	r.Defs.Push("s", model)
	r.Defs.Push("lit", literal)
	r.Defs.Push("n", idless)
	r.Check.Emit = &producedRecorder{EmitRecorder: core.TheInactiveEmit, produced: map[core.RecKey]bool{core.RecKeyOf(&model): true, core.RecKeyOf(&idless): true}}

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
