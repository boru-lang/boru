package core

import "testing"

// TestInactiveCheckBraid pins the check-less defaults behind the S9
// dispatch-hook table: each is the exact decline an unarmed analysis
// pass produces (identity for the paren collapse, empty restores, zero
// results). The live table is check-installed at init; these bodies are
// the post-cut core-only configuration.
func TestInactiveCheckBraid(t *testing.T) {
	inactiveCheckMixedFormAdvisories(nil, WordInfo{}, nil, nil, SrcPos{}, 0, 0)
	if err := inactiveCheckModeAssumeSig(nil, WordInfo{}, nil, nil, SrcPos{}); err != nil {
		t.Fatal("inactive assumeSig must be nil error")
	}
	if got := inactiveCheckModeFallbackPositions(nil, 3); got != nil {
		t.Fatal("inactive fallbackPositions must be nil")
	}
	if got := inactiveCheckModeParenFnCollapse(nil, 2, 7); got != 7 {
		t.Fatalf("inactive parenFnCollapse must be identity on closeIdx, got %d", got)
	}
	if ok, err := inactiveCheckModeSurfaceShape(nil, WordInfo{}, SrcPos{}); ok || err != nil {
		t.Fatal("inactive surfaceShape must decline")
	}
	if v, ok := inactiveConcreteEvalOnce(nil, nil); ok || IsConcrete(v) {
		t.Fatal("inactive concreteEvalOnce must decline")
	}
	inactiveDrainUndefinedAtoms(nil)
	inactiveNoteStrandedTypeCall(nil, []Value{NewCarrier(TInteger)})
	if inactiveExprRefsCarrier(nil, []Value{NewCarrier(TInteger)}) {
		t.Fatal("inactive exprRefsCarrier must be false")
	}
	inactiveNoteSpeculativeBarrierCommit(nil, ForwardInfo{})
	inactiveDeclineForwardStackDrift(nil, nil, nil)
	inactiveDeclineStrandedMemberFn(nil, nil)
	inactiveShareCheckState(nil, nil)() // the restore closure is a no-op
	if err := inactiveSpliceAnonCheckResult(nil, 0, 0, nil, nil, nil); err != nil {
		t.Fatal("inactive spliceAnon must be nil error")
	}
	inactiveSpliceCheckResults(nil, nil, nil)
	if err := inactiveSpliceFnValueCheckResult(nil, 0, 0, FnDefInfo{}, nil, nil); err != nil {
		t.Fatal("inactive spliceFnValue must be nil error")
	}
	inactiveTagCheckModeDefRead(nil, nil, "x", SrcPos{})
	if inactiveTryDynamicFnValueDispatch(nil, 0) {
		t.Fatal("inactive dynamicFnValue must decline")
	}
	if inactiveTryMemberFnArrivalDispatch(nil, 0) {
		t.Fatal("inactive memberFnArrival must decline")
	}
	if inactiveParenPlacedFnCarrier(nil, 0) {
		t.Fatal("inactive parenPlacedFnCarrier must decline")
	}
	if inactiveTryShapedMethodDispatch(nil, 0) {
		t.Fatal("inactive shapedMethod must decline")
	}
	if d := inactiveUndefinedWordCheckDiag(nil, "w", SrcPos{}); d.Code != "" {
		t.Fatal("inactive undefinedWordDiag must be zero")
	}
}

// TestInactiveDriftWindowRecorder pins the compiler-less default behind
// the drift-island hook: with no compiler linked there is nothing to
// record, so the offer declines and the caller keeps its compile failure.
func TestInactiveDriftWindowRecorder(t *testing.T) {
	if inactiveDriftWindowRecorder(nil, WordInfo{}, nil, nil) {
		t.Fatal("inactive drift-window recorder must decline")
	}
}

// TestInactiveRegionRecorder pins the compiler-less default behind the
// region-descriptor hook. Unlike the drift recorder there is no verdict to
// check — the slot returns nothing — so what this proves is that the default
// is REACHABLE and inert: it must not panic on the arguments a real dispatch
// hands it, including the degenerate ones a compiler-less build never
// filters (a nil window, a nil registry, an index past the end).
//
// The named default exists for exactly this. An anonymous func(...){} assigned
// at init would be unreachable in core's own profile and fail the merged
// ADR-008 gate, which is the rule every seam slot here follows.
func TestInactiveRegionRecorder(t *testing.T) {
	inactiveRegionRecorder(nil, nil, WordInfo{}, 0)
	inactiveRegionRecorder(NewTape(nil, 0), nil, WordInfo{Name: "add"}, -1)

	// And the live slot defaults to it, so a build with no compiler linked
	// collects exactly as it did before the seam existed.
	tape := NewTape([]Value{NewWord("add"), NewInteger(1)}, 0)
	RegionRecorder(tape, nil, WordInfo{Name: "add", ArgCount: -1}, 0)
	if tape.Len() != 2 {
		t.Fatalf("the inactive recorder must not touch the window: len = %d, want 2", tape.Len())
	}
}
