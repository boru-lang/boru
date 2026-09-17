package check

import core "github.com/boru-lang/boru/core/go"

// The dispatch-record hook layer — the four-piece split's S3 seam
// (design/legacy/ENG-FOUR-PIECE.0.ignore). Every call the CHECK piece makes into
// the compiler's recording / folding / poly-bake machinery routes
// through the DispatchBraid slot table below, so check files carry no
// named compiler symbols: the compiler piece installs its
// implementations at init (installDispatchBraid,
// dispatch_hooks_install.go), and a compiler-less check build runs the
// named inactive defaults — the exact declines an unarmed recording
// pass produces. Behavior is byte-identical to the direct calls the
// table replaces.

// UserPolyPlan is the check piece's opaque view of one poly user
// call's compiled arm table (the compiler's plan handed back by
// CompileUserPolyArms/PlanUserPoly): the record site substitutes the
// return-join carriers and unpacks the parallel slices into
// EmitRecorder.RecordUserPolyCall without naming the compiler's
// concrete plan type. Installers must return a NIL INTERFACE (not a
// typed nil) when the plan declines.
type UserPolyPlan interface {
	SubstituteJoinedOuts(out []core.Value)
	SigIdx() []int
	Units() []int
	Impls() []core.SigImpl
	Sigs() []core.Signature
}

// DispatchBraid is the check->compiler slot table. The slots are
// inactive only on a build linked without the compiler piece — a
// configuration where nothing records, so every slot declines.
var DispatchBraid = struct {
	// RecordOutcome forwards a matched dispatch's concrete outcome to
	// the recording pipeline (const folds, poly arms, deferred lists).
	RecordOutcome func(r *core.Registry, word string, sig *core.Signature, args, out []core.Value, pos core.SrcPos, ownerReg *core.Registry)
	// TryFoldScalarConst asks the compiler for a scalar const-fold of
	// a pure dispatch over inert operands.
	TryFoldScalarConst func(r *core.Registry, sig *core.Signature, args []core.Value) (core.Value, bool)
	// TryRecordPoly offers a hazardous dispatch to the poly recorder
	// (the VM re-match lowering) and reports whether it was taken.
	TryRecordPoly func(r *core.Registry, word string, sig *core.Signature, args, outs []core.Value, pos core.SrcPos, disjunctStraddle bool, ownerReg *core.Registry, dynamicRecovery bool, noMatch *core.PolyNoMatchSpec) bool
	// CompileUserPolyArms bakes every same-arity overload's body unit
	// for a user-poly re-match plan (nil = the bake declined an arm).
	CompileUserPolyArms func(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, committedReturns []*core.Type) UserPolyPlan
	// PlanUserPoly decides whether a multi-overload user-fn call site
	// must ride the user-poly runtime re-match instead of a static arm
	// commit.
	PlanUserPoly func(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, declaredReturns []*core.Type) (UserPolyPlan, bool)
}{
	RecordOutcome:       inactiveDispatchRecordOutcome,
	TryFoldScalarConst:  inactiveDispatchTryFoldScalarConst,
	TryRecordPoly:       inactiveDispatchTryRecordPoly,
	CompileUserPolyArms: inactiveDispatchCompileUserPolyArms,
	PlanUserPoly:        inactiveDispatchPlanUserPoly,
}

// The inactive defaults are NAMED so the seam test pins them
// (TestInactiveDispatchBraid): each is the exact decline an unarmed
// recording pass produces.
func inactiveDispatchRecordOutcome(*core.Registry, string, *core.Signature, []core.Value, []core.Value, core.SrcPos, *core.Registry) {
}

func inactiveDispatchTryFoldScalarConst(*core.Registry, *core.Signature, []core.Value) (core.Value, bool) {
	return core.Value{}, false
}

func inactiveDispatchTryRecordPoly(*core.Registry, string, *core.Signature, []core.Value, []core.Value, core.SrcPos, bool, *core.Registry, bool, *core.PolyNoMatchSpec) bool {
	return false
}

func inactiveDispatchCompileUserPolyArms(*core.Registry, core.EmitRecorder, string, []core.Value, []*core.Type) UserPolyPlan {
	return nil
}

func inactiveDispatchPlanUserPoly(*core.Registry, core.EmitRecorder, string, []core.Value, []*core.Type) (UserPolyPlan, bool) {
	return nil, false
}

// The check-side accessors keep every call site spelled as before; the
// braid is the seam.

func dispatchRecordOutcome(r *core.Registry, word string, sig *core.Signature, args, out []core.Value, pos core.SrcPos, ownerReg *core.Registry) {
	DispatchBraid.RecordOutcome(r, word, sig, args, out, pos, ownerReg)
}

func dispatchTryFoldScalarConst(r *core.Registry, sig *core.Signature, args []core.Value) (core.Value, bool) {
	return DispatchBraid.TryFoldScalarConst(r, sig, args)
}

func dispatchTryRecordPoly(r *core.Registry, word string, sig *core.Signature, args, outs []core.Value, pos core.SrcPos, disjunctStraddle bool, ownerReg *core.Registry, dynamicRecovery bool, noMatch *core.PolyNoMatchSpec) bool {
	return DispatchBraid.TryRecordPoly(r, word, sig, args, outs, pos, disjunctStraddle, ownerReg, dynamicRecovery, noMatch)
}

func dispatchCompileUserPolyArms(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, committedReturns []*core.Type) UserPolyPlan {
	return DispatchBraid.CompileUserPolyArms(r, es, word, args, committedReturns)
}

func dispatchPlanUserPoly(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, declaredReturns []*core.Type) (UserPolyPlan, bool) {
	return DispatchBraid.PlanUserPoly(r, es, word, args, declaredReturns)
}
