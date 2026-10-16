package core

import "testing"

// TestInactiveAnalysisImpl pins the check-less defaults behind the S1
// implementation table: each is the exact no-op an inactive analysis
// pass produces. The live table is check-installed at init; these
// bodies are the post-cut core-only configuration.
func TestInactiveAnalysisImpl(t *testing.T) {
	inactiveFnConstructionPass(nil, "", FnDefInfo{})
	if inactiveReturnsFn(nil, "", FnSig{}, FnDefInfo{}) != nil {
		t.Fatal("inactive returnsFn must be nil")
	}
	in := []Value{NewInteger(1)}
	if got := inactiveStripToCarriers(in); len(got) != 1 || !ValuesEqual(got[0], in[0]) {
		t.Fatal("inactive stripToCarriers must pass through")
	}
	if inactiveCarrierResults(nil, "", nil, nil, SrcPos{}, nil, false) != nil {
		t.Fatal("inactive carrierResults must be nil")
	}
	if inactiveMixedConform(Value{}, TInteger) {
		t.Fatal("inactive mixedConform must be false")
	}
	if inactiveValueCarriesCarrier(NewCarrier(TInteger)) {
		t.Fatal("inactive valueCarriesCarrier must be false")
	}
	if inactiveAtUncaughtTopLevel(nil) {
		t.Fatal("inactive atUncaughtTopLevel must be false")
	}
	// The two body-model slots (ADR-013, 2026-08-08). nil is in-band —
	// AnalyseFnBody already documents an empty result as "the analyser
	// aborted, treat as an Any carrier" — so the check-less default is
	// the same no-analysis regime the slots above define, not a
	// different one.
	if inactiveAnalyseFnBody(nil, "", nil, nil, nil, nil, nil, false) != nil {
		t.Fatal("inactive analyseFnBody must be nil")
	}
	if inactiveAnalyseLoopBody(nil, Value{}, nil, nil, false) != nil {
		t.Fatal("inactive analyseLoopBody must be nil")
	}
	// The exported accessors are the surface a word library outside core
	// calls (basic's `fn` / `for`); pin that they route to the table.
	saveFn, saveLoop := AnalysisImpl.AnalyseFnBody, AnalysisImpl.AnalyseLoopBody
	defer func() {
		AnalysisImpl.AnalyseFnBody, AnalysisImpl.AnalyseLoopBody = saveFn, saveLoop
	}()
	marker := []Value{NewInteger(7)}
	AnalysisImpl.AnalyseFnBody = func(*Registry, string, []string, []Value, []Value,
		[]CapturedBinding, []*Type, bool) []Value {
		return marker
	}
	AnalysisImpl.AnalyseLoopBody = func(*Registry, Value, []string, []Value, bool) []Value {
		return marker
	}
	if got := RunFnBodyAnalysis(nil, "", nil, nil, nil, nil, nil, false); len(got) != 1 ||
		!ValuesEqual(got[0], marker[0]) {
		t.Fatal("RunFnBodyAnalysis must dispatch through AnalysisImpl")
	}
	if got := RunLoopBodyAnalysis(nil, Value{}, nil, nil, false); len(got) != 1 ||
		!ValuesEqual(got[0], marker[0]) {
		t.Fatal("RunLoopBodyAnalysis must dispatch through AnalysisImpl")
	}
}
