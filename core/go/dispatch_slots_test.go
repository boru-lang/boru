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
	if v, ok := inactiveConcreteEvalOnce(nil, nil); ok || IsConcrete(&v) {
		t.Fatal("inactive concreteEvalOnce must decline")
	}
	inactiveDrainUndefinedAtoms(nil)
	inactiveNoteStrandedTypeCall(nil, []Value{NewCarrier(TInteger)})
	if inactiveExprRefsCarrier(nil, []Value{NewCarrier(TInteger)}) {
		t.Fatal("inactive exprRefsCarrier must be false")
	}
	inactiveNoteSpeculativeBarrierCommit(nil, ForwardInfo{})
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
	if inactiveTryShapedMethodDispatch(nil, 0) {
		t.Fatal("inactive shapedMethod must decline")
	}
	if d := inactiveUndefinedWordCheckDiag(nil, "w", SrcPos{}); d.Code != "" {
		t.Fatal("inactive undefinedWordDiag must be zero")
	}
}
