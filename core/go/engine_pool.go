package core

// Pooled sub-engine entry points layered over the registry's reusable
// sub-engine pool (takeSubEngine / putSubEngine, registry.go — the same
// pool runPooledSub drives for the auto-evaluation seams).
//
// These wrappers add the two per-run configurations the plain pool does
// not carry:
//
//   - a RESOLVED-ARGUMENT prefix (Engine.startAt): the leading inputs
//     are call-site-resolved arguments and enter as stack data below
//     the pointer, never re-stepped — the sub-engine twin of
//     FrameOpenInfo.ArgSpan (arguments are inert;
//     design/legacy/ARG-SEMANTICS-UNIFICATION.0.ignore);
//   - top-engine semantics (Engine.isTop) for callback sites that
//     historically spawned NewTop (behave bodies): an unhandled
//     break/continue at end-of-Run errors instead of propagating, and
//     a handler panic recovers into a clean internal_error. New and
//     NewTop share one step limit, so top-ness is the only delta.
//
// Results are COPIED before the engine is pooled: Run's return value
// aliases the tape's backing array (Tape.TakeAll ownership transfer),
// and the pool's next Reload would clobber a retained slice.
//
// Nesting is naturally safe: take POPS the engine, so a body that
// re-enters the pool (an `each` inside an `each`) runs on a different
// engine; the outer one returns to the pool only after the inner run
// finished.

// RunPooled executes tokens exactly as `New(r).Run(tokens)` would, on a
// pooled engine.
func RunPooled(r *Registry, tokens []Value) ([]Value, error) {
	return runPooledAt(r, tokens, 0, false)
}

// RunPooledTop is RunPooled with NewTop semantics for the one run.
func RunPooledTop(r *Registry, tokens []Value) ([]Value, error) {
	return runPooledAt(r, tokens, 0, true)
}

// RunResolved executes `inputs… tokens…` with the inputs entering as
// RESOLVED stack data: stepping starts at the first token, so an input
// with active step semantics (a Function value, an __SP marker) cannot
// fire on placement. This is the seam every callback-style body run
// uses — only the body acts on its arguments.
func RunResolved(r *Registry, inputs, tokens []Value) ([]Value, error) {
	// Observability seam (interp_entry.go): the callback-style body path.
	r.noteInterp("RunResolved")
	input := make([]Value, len(inputs)+len(tokens))
	copy(input, inputs)
	copy(input[len(inputs):], tokens)
	return runPooledAt(r, input, len(inputs), false)
}

func runPooledAt(r *Registry, tokens []Value, startAt int, top bool) ([]Value, error) {
	e := r.TakeSubEngine()
	e.IsTop = top
	e.StartAt = startAt
	res, err := e.Run(tokens)
	if len(res) > 0 {
		res = append([]Value(nil), res...)
	}
	// Clear the per-run configuration before pooling — putSubEngine
	// resets only elemEvalRecordable, and top-ness must not leak into
	// the auto-eval seams' next use of the same engine. startAt is
	// already consumed by Run (consumeStartAt), cleared here only for
	// the defensive symmetry.
	e.IsTop = false
	e.StartAt = 0
	r.PutSubEngine(e)
	return res, err
}
