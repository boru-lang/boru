package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A None arm is a lattice root: NewTypeLiteral(TNone) has Parent==nil. The
// parent-math collapse blocks must not run against it — ConformsTo(nil) is a
// vacuous true, so `Integer.ConformsTo(None)` would collapse the merge to
// NewCarrier(None.Parent==nil), a Parent-less carrier the engine halts on when
// it steps an `if b [99] [None]` result (undefined stack entry). The join must
// instead yield a valid, non-nil-Parent carrier.
func TestS6aJoinCarriersNoneArm(t *testing.T) {
	none := core.NewTypeLiteral(core.TNone)
	if core.ParentOf(&none) != nil {
		t.Fatalf("precondition: None literal has a nil Parent, got %v", core.ParentOf(&none))
	}

	// value-vs-None: Integer|None union, a valid disjunct carrier (never a
	// Parent-less carrier).
	got := core.JoinCarriersInner(core.NewInteger(99), none)
	if core.ParentOf(&got) == nil {
		t.Fatalf("value-vs-None join must not produce a nil-Parent carrier: %+v", got)
	}
	if !core.IsDisjunct(&got) {
		t.Errorf("Integer joined with None should be a disjunct carrier, got %+v", got)
	}

	// None-vs-value is symmetric.
	if g2 := core.JoinCarriersInner(none, core.NewInteger(99)); core.ParentOf(&g2) == nil {
		t.Fatalf("None-vs-value join must not produce a nil-Parent carrier: %+v", g2)
	}

	// both-None collapses to a proper (non-nil-Parent) None carrier.
	both := core.JoinCarriersInner(none, core.NewTypeLiteral(core.TNone))
	if core.ParentOf(&both) == nil || !core.ParentOf(&both).Equal(core.TNone) || !core.IsCarrier(&both) {
		t.Errorf("None joined with None should be a None carrier, got %+v", both)
	}
}

// --- core.RunCarrierBodyWithDefs -----------------------------------------------------------
