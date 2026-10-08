package core

import "testing"

// TestArmResidualSweepBrackets: an arm body's run (Engine.ArmBody) sweeps its
// residual containers under CheckState.ArmResidualSweep, which the sweep
// leaves balanced; any other run leaves it untouched (NUR352).
func TestArmResidualSweepBrackets(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	lit := NewList([]Value{NewInteger(1), NewInteger(2)})
	lit.Eval = true
	for _, arm := range []bool{true, false} {
		e := New(r)
		e.ArmBody = arm
		out, err := e.Run([]Value{lit})
		if err != nil || len(out) != 1 {
			t.Fatalf("arm=%v: %v %v", arm, out, err)
		}
		if r.Check.ArmResidualSweep != 0 {
			t.Errorf("arm=%v: the sweep's bracket is left open: %d", arm, r.Check.ArmResidualSweep)
		}
	}
}
