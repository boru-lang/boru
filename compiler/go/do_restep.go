package compiler

import (
	"slices"

	core "github.com/boru-lang/boru/core/go"
)

// do_restep.go — NUR317's open half. The interpreter's `do` hands its body's
// whole residual back to the step loop, which re-steps it where the `do`
// stood: a placed fn value the body left — an `if` body arm's one survivor —
// dispatches there, over the results after it. Where the check pass decides
// the `if`, its own run of the body steps the taken arm and dispatches the fn
// inside the body, so the `do`'s modelled outputs are the re-step's and
// nothing the program records after the call applies the fn the compiled body
// hands back: `do [if c [g/v] [0] 5]` modelled `[Integer 5]` and was `fn g 5`
// compiled for the interpreter's `7 5`. Such a call carries SigRef.ReStep,
// and the VM re-steps its results (eng's vm_do_restep.go).

// noteClosureReStep flags the closure call seq when the interpreter re-steps
// its results where nothing the program records does (NUR317): a `do` — its
// native hands the body's whole residual back to the step loop — whose
// compiled body may leave a fn value (fnUnitRec.mayReturnFn) while none of
// the modelled outputs is one whose re-step the program records wherever it
// lands (applyRecorded). Where an output is a union with a Function
// alternative the call is only a candidate (unionReStep): a residual's
// runtime-conditional apply may take the union (`do [if c [l/v] [0]] 5`),
// which only the lowering knows.
func (es *EmitState) noteClosureReStep(sig *core.Signature, unit int, outs []core.Value, seq int) {
	if sig.Callable == nil || sig.Callable.BodyOut != core.BodyOutResidual || unit < 0 || unit >= len(es.fnRecs) || !es.fnRecs[unit].mayReturnFn {
		return
	}
	if slices.ContainsFunc(outs, applyRecorded) {
		return
	}
	f := es.eventInfo[seq]
	if slices.ContainsFunc(outs, core.UnionMayBeFn) {
		f.unionReStep = true
	} else {
		f.reStepResults = true
	}
	es.eventInfo[seq] = f
}

// applyRecorded reports whether the caller's re-step of a modelled output v
// is one the program records itself wherever the value lands — a fn value, a
// fn-typed carrier or a gradual value that may be one. A union with a
// Function alternative is not: its landing records no apply, and only a
// residual's apply may take it (eventFlags.unionReStep).
func applyRecorded(v core.Value) bool {
	return valueMayBeFn(v) && !core.UnionMayBeFn(v)
}

// valueMayBeFn reports whether a modelled value may be a fn at run time: a
// fn value, a fn-typed carrier, a gradual value whose bound admits one, or a
// union with a Function alternative.
func valueMayBeFn(v core.Value) bool {
	return core.IsFnValueResidual(v) || core.IsFnTypedCarrier(v) || (v.Dynamic && core.SigTypeMatches(v, core.TFunction)) || core.UnionMayBeFn(v)
}

// noteBodyReStep records a LITERAL body whose compiled residual a closure
// probe found may leave a fn value none of the dispatch's modelled outputs
// shows (noteClosureReStep's rule), for the dyn-body backstop that takes the
// body when the closure declines. A closure call that records
// clears it (takeBodyReStep).
func (es *EmitState) noteBodyReStep(body core.Value) {
	if es.bodyReSteps == nil {
		es.bodyReSteps = map[string]bool{}
	}
	es.bodyReSteps[body.ID] = true
}

// takeBodyReStep reports and clears noteBodyReStep's record for body.
func (es *EmitState) takeBodyReStep(body core.Value) bool {
	if !es.bodyReSteps[body.ID] {
		return false
	}
	delete(es.bodyReSteps, body.ID)
	return true
}
