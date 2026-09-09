package check

// Direct in-package unit test for callShapeSpecialised (carrier.go), the
// predicate behind CheckState.CallShapeDepth. Per design/TEST-SEAMS.10.md it
// is driven by direct calls with synthetic Values.
//
// The CAPTURES arm needs its own test rather than riding a program: only the
// compiler suite reaches it through real source, so under the STANDALONE
// check gate (`make cover-gate-check`, check/go by its own suite alone) that
// arm is otherwise uncovered even though the merged gate is satisfied.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestCallShapeSpecialised pins what makes a body analysis a per-call-shape
// specialisation rather than the declaration-shaped run that is entitled to
// report a dead branch.
func TestCallShapeSpecialised(t *testing.T) {
	carrier := core.NewDynamicCarrier(core.TInteger)
	concrete := core.NewInteger(1)
	cap := func(v core.Value) []core.CapturedBinding {
		return []core.CapturedBinding{{Name: "c", Value: v}}
	}

	for _, c := range []struct {
		name     string
		args     []core.Value
		captures []core.CapturedBinding
		want     bool
	}{
		// The generalized run: every param is a carrier, nothing in the body
		// can fold on an argument, so a constancy found here is the code's.
		{"no args at all", nil, nil, false},
		{"carrier args only", []core.Value{carrier, carrier}, nil, false},
		{"carrier arg and carrier capture", []core.Value{carrier}, cap(carrier), false},
		// One actual value anywhere is enough: FnAnalysisKey keys on captures
		// too, so a closure specialised on a concrete capture is re-analysed
		// per capture shape exactly as one specialised on a concrete arg is.
		{"one concrete arg", []core.Value{carrier, concrete}, nil, true},
		{"concrete capture, carrier args", []core.Value{carrier}, cap(concrete), true},
		{"concrete capture, no args", nil, cap(concrete), true},
	} {
		if got := callShapeSpecialised(c.args, c.captures); got != c.want {
			t.Errorf("%s: callShapeSpecialised = %v, want %v", c.name, got, c.want)
		}
	}
}
