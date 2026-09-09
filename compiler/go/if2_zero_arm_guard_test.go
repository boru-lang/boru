package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// if2_zero_arm_guard_test.go pins RecordBranch's ZERO-VALUE THEN arm — the
// 2-arg `if` path (`!HasElse`, a then arm that nets nothing) — and the
// fragment guard that opens it.
//
// That arm is the ONE branch shape recorded WITHOUT resolving a merge value:
// there is no `resolveArm` call to run the usual nil-fragment check, and the
// event it appends is marked zeroOut, so a missing arm fragment sails past
// every later consumer and reaches the lowerer as a branch with nothing to
// lower on its true path. What happens then is not a soft refusal —
// lowerBranch -> lowerArms -> lowerArm hands the nil fragment to
// lowerFragment, which dereferences it (lower.go): deleting this guard turns
// `if 5 6` into a nil-pointer PANIC inside CompileCheck. That is what the
// guard buys, and it is bought for a DOCUMENTED program rather than a
// defensive impossibility: `boru describe if` lists the 2-arg VALUE form
// (`if 5 6 ;# 6`), and a non-list then arm never reaches fragment capture at
// all — runCarrierBodyDefsAdds (core/go/carrier_body.go) returns on its
// `AsList` failure BEFORE installing BodyAnalysisGuard, so If2ReturnsFn's
// TakeFragment hands RecordBranch a nil fragment over an empty then-stack.
//
// lang/go/bytecode_if2_valuearm_test.go is the whole-program half; this is the
// seam half, which can additionally show WHERE the refusal lands — before the
// event is appended, so no half-recorded branch is left behind.
//
// Both directions are pinned: the nil fragment refuses and records nothing,
// and the same record carrying a real (empty-bodied) fragment records the
// zeroOut branch and stays compilable.

// i2gState builds a recorder ready to take a branch record, with the registry
// bound so operand resolution behaves as it does in a real compile pass.
func i2gState(t *testing.T) *EmitState {
	t.Helper()
	es := NewEmitState()
	es.BindRegistry(seam7Reg(t))
	return es
}

// i2gRecord is the shape If2ReturnsFn hands the recorder for a then arm that
// nets nothing: no else, an empty then-stack, and the phantom None the caller
// registers as the branch result. `frag` is the arm fragment under test.
func i2gRecord(frag core.EmitFragmentRef, out core.Value) core.BranchRecord {
	return core.BranchRecord{
		Cond:    core.NewBoolean(true),
		HasElse: false,
		Then:    frag,
		ThenStk: nil,
		Out:     out,
	}
}

func TestIf2ZeroValueArmRequiresItsFragment(t *testing.T) {
	// The REFUSAL. TakeFragment returned nothing (the non-list then arm of
	// `if 5 6`), so the recorder has no events to lower on the true path.
	es := i2gState(t)
	out := core.NewCarrier(core.TNone)
	es.RecordBranch(i2gRecord(i2gNilFragmentRef(), out))

	if es.Compilable {
		t.Fatal("a 2-arg if whose then arm was never captured must refuse the program")
	}
	if es.Reason != "if: then-branch not captured" {
		t.Errorf("refusal reason = %q, want the uncaptured-then-branch refusal", es.Reason)
	}
	// The refusal must land BEFORE the branch is appended: a recorded event
	// with no fragment is exactly what the guard exists to keep out of the
	// trace, and Finalize would otherwise reach it.
	if es.SiteCounts[SiteMono] != 0 {
		t.Errorf("a refused branch must append no event, got %d mono sites", es.SiteCounts[SiteMono])
	}
	if _, ok := es.producedBy[out.ID]; ok {
		t.Error("a refused branch must not register its result as produced")
	}

	// THE TWIN: the same record, with the fragment an `if true []` really
	// does capture (a body that runs and leaves nothing). It records — a
	// zeroOut branch, no merge slot — and the program stays compilable.
	es2 := i2gState(t)
	out2 := core.NewCarrier(core.TNone)
	es2.RecordBranch(i2gRecord(&EmitFragment{}, out2))

	if !es2.Compilable {
		t.Fatalf("a captured 0-value then arm must record, got refusal %q", es2.Reason)
	}
	if es2.SiteCounts[SiteMono] != 1 {
		t.Fatalf("a recorded branch = %d mono sites, want 1", es2.SiteCounts[SiteMono])
	}
	pr, ok := es2.producedBy[out2.ID]
	if !ok {
		t.Fatal("the recorded branch must register its (phantom None) result, so RecordCall's double-record guard elides the dispatch")
	}
	if !es2.eventInfo[pr.seq].zeroOut {
		t.Error("a 2-arg if with a 0-value then arm must be marked zeroOut (no merge slot for Finalize to reconcile)")
	}
}

// i2gNilFragmentRef reproduces what TakeFragment returns when nothing was
// captured: a core.EmitFragmentRef holding a NIL *EmitFragment, not an
// untyped nil. asFragment must yield a nil fragment either way — writing the
// literal nil here would test a shape the recorder never actually receives.
func i2gNilFragmentRef() core.EmitFragmentRef {
	es := NewEmitState()
	return es.TakeFragment()
}
