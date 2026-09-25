package compiler

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// The compiler piece's DispatchBraid installation (the S3 seam's
// active half — see dispatch_hooks.go). Runs at init while the
// compiler is linked; a compiler-less build keeps the inactive
// defaults. The plan-returning installers wrap the concrete
// *userPolyPlan so a declined bake yields a NIL INTERFACE, never a
// typed nil (the check side tests `plan != nil`).

func init() { installDispatchBraid() }

func installDispatchBraid() {
	check.DispatchBraid.RecordOutcome = recordDispatchOutcome
	check.DispatchBraid.TryFoldScalarConst = tryFoldScalarConst
	check.DispatchBraid.TryRecordPoly = tryRecordPoly
	check.DispatchBraid.TryRecordDynBody = tryRecordDynBody
	check.DispatchBraid.CompileUserPolyArms = func(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, committedReturns []*core.Type) check.UserPolyPlan {
		if p := tryCompileUserPolyArms(r, es, word, args, committedReturns); p != nil {
			return p
		}
		return nil
	}
	check.DispatchBraid.PlanUserPoly = func(r *core.Registry, es core.EmitRecorder, word string, args []core.Value, declaredReturns []*core.Type) (check.UserPolyPlan, bool) {
		p, barred := planUserPolyDispatch(r, es, word, args, declaredReturns)
		if p != nil {
			return p, barred
		}
		return nil, barred
	}
}
