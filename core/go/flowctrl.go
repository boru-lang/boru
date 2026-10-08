package core

import "fmt"

// FlowCtrl is a non-error signalling channel for control-flow primitives
// (break, continue, ...). It travels separately from `error` so the two
// concerns stay distinct: errors mean execution FAILED, FlowCtrl values
// mean execution should be REDIRECTED.
//
// Transport: handlers set Registry.FlowCtrl rather than returning an
// error sentinel. The Run loop reads it after every step. Because
// sub-engines share a Registry, the signal naturally propagates across
// nested Run frames without abusing the error return.
//
// The `break` and `continue` words themselves live in the lang layer
// (lang/go/engine/native_control.go) — eng only owns the FlowCtrl type
// plus the Run-loop dispatch. To extend the channel with a new
// signal, add a constant here and a matching case in the Run loop's
// post-step handler.
type FlowCtrl uint8

const (
	FlowNone FlowCtrl = iota
	FlowBreak
	FlowContinue
)

func (f FlowCtrl) String() string {
	switch f {
	case FlowNone:
		return "none"
	case FlowBreak:
		return "break"
	case FlowContinue:
		return "continue"
	default:
		return fmt.Sprintf("FlowCtrl(%d)", uint8(f))
	}
}

// HoldFlowAt records where the run that let the pending signal out stood —
// pos, and whether it stood on a token at all (set) — unless an inner run
// already holds it (Registry.FlowAtHeld): the innermost position is the one
// the `outside loop` report points at (NUR355, NUR358).
func (r *Registry) HoldFlowAt(pos SrcPos, set bool) {
	if r.FlowAtHeld {
		return
	}
	r.FlowAt, r.FlowAtSet, r.FlowAtHeld = pos, set, true
}

// TakeFlow clears the pending signal and the position held with it.
func (r *Registry) TakeFlow() {
	r.FlowCtrl = FlowNone
	r.FlowAt, r.FlowAtSet, r.FlowAtHeld = SrcPos{}, false, false
}

// RaiseFlowOutsideLoop takes the pending signal and returns the `<ctrl>
// outside loop` flow_error over src, at the position an inner run holds
// (FlowAtHeld, NUR358) or else at pos — the report a run with no loop to
// take the signal raises.
func (r *Registry) RaiseFlowOutsideLoop(pos SrcPos, src string) error {
	ctrl := r.FlowCtrl
	if r.FlowAtHeld {
		pos = r.FlowAt
	}
	r.TakeFlow()
	return makeBoruErrorAt("flow_error", fmt.Sprintf("%s outside loop", ctrl), ctrl.String(), src, "", pos)
}
