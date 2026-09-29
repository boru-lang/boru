package compiler

import core "github.com/boru-lang/boru/core/go"

// takenLandingUnguarded is the decline of a TAKEN landing (NUR349) no
// landing op can guard: the dispatch that took the value would run over a
// fn the interpreter re-steps first.
const takenLandingUnguarded = "a dispatch took a member read the run may find callable beside a value written after it, which the interpreter's re-step collects first, and no landing can guard it (NUR349)"

// NoteTakenLanding is the recorder side of the check pass's taken-landing
// note (core's noteCollectedLandings, NUR349). The dispatch at the pass's
// pointer took v off the stack together with a value written after it, where
// the step loop re-stepped v with that value to collect and noted no landing
// (CheckState.StoodAsideLandingIDs): `1 m.f 7 add` over an opaque Map's
// member. The interpreter re-steps a callable v where it lands — a member
// read dispatches at its own token (ADR-011) — its forward phase collecting
// the 7 before `add` runs (9), where the model hands `add` the fn and the 7.
// So the read's event owes a COLLECTING landing (LandingCollects), guarded
// where nothing compiled re-steps the value (guardRootLandings /
// guardUnitLandings): at run time a callable value takes its statement's
// island, and data runs on as the model has it.
//
// Only a single-result MEMBER READ (the get family) is taken here. Its
// landing op hangs on the read's own event (seatCallResults); a value any
// other event produced keeps today's arms — a native's fn result its re-step
// deopts (NUR124), a user call's result the interpreter parks, a branch's
// join its placement rule (NUR313) — and so does a def-bound read, its
// name's word dispatch, which the gradual read's own guard seats (NUR123).
func (es *EmitState) NoteTakenLanding(v core.Value) {
	if !es.Active() {
		return
	}
	pr, ok := es.producedBy[v.ID]
	if !ok || es.isDefRead(v) {
		return
	}
	if ev := es.eventBySeq(pr.seq); ev == nil || ev.kind != evCall || !isGetFamilyWord(ev.call.word) || ev.call.nout != 1 {
		return
	}
	es.NoteReStepLanding(v, v.Pos())
	es.NoteLandingNext(v, core.LandingNextCollect, false, core.Value{})
	if es.landingTaken == nil {
		es.landingTaken = map[int]bool{}
	}
	es.landingTaken[pr.seq] = true
}

// takenLandingFail is emitLandingAfter's verdict on a TAKEN landing
// (NUR349): the program declines where the landing op cannot be guarded — a
// landing inside a fragment (a branch arm, a loop body), which keeps today's
// arms (noteRootBeneathLanding), and one in a fn unit whose residual is a
// tail apply chain or a trailing apply, which guardUnitLandings leaves to
// those arms. Either would run the dispatch over the fn.
func (lw *lowerer) takenLandingFail(seq int) string {
	if !lw.es.landingTaken[seq] {
		return ""
	}
	if !(lw.landingRoot || lw.isFnUnit) || lw.depth != 0 {
		return takenLandingUnguarded
	}
	if u := lw.rec; u != nil && (len(u.applyChain) > 0 || u.dynTrailArity > 0) {
		return takenLandingUnguarded
	}
	return ""
}
