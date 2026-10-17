package check

// Standalone-gate tests for AnalyseLoopBody's join decisions (carrier.go)
// and the helpers behind them: a body def of the loop's OWN bind name, a
// fresh binding of a loop that may run zero times, a speculative undef that
// moves the pass's generation, and a module bind the compiled lane's one
// replay cannot stand for. The loop-capture protocol is observed through
// an EmitRecorder stub (zcaEmit, zz_cover_carrier_analysis_test.go) that
// additionally records NoteLoopFresh and Suspend.

import (
	"testing"
)

// isBindName: a name is one of the loop's own bind variables exactly when
// it is listed.
func TestGateStandaloneIsBindName(t *testing.T) {
	if !isBindName([]string{"i", "j"}, "j") {
		t.Error("a listed bind name is a bind name")
	}
	if isBindName([]string{"i"}, "k") || isBindName(nil, "i") {
		t.Error("an unlisted name is not a bind name")
	}
}
