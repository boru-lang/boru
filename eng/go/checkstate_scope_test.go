package eng

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// The §4 collision guard, validated at the mechanism level
// (design/legacy/module-fn-checkstate-ownership.1.ignore §5a, the prerequisite that makes
// §5b's shared CheckState safe).
//
// Under §5b a module sub-registry transiently shares the parent compile pass's
// *CheckState — including its FnSummaries / FnInflight / FnAnalysisCounts maps.
// Those maps are keyed by FnAnalysisKey. If the key did not include a
// per-registry scope, a module preamble fn and a parent fn of the SAME name +
// arg-types + (overlapping) source position would alias the same memo entry —
// the `.0` §4 `uncalled_function` cascade. AnalysisScopeID() makes the keys
// disjoint by construction. This test pins that:
//
//   - two distinct registries (parent + module sub-registry) produce DIFFERENT
//     keys for identical (name, args) — so sharing one CheckState cannot alias;
//   - with a CONSTANT scope (the pre-§5a behaviour / a stubbed scopeID) the same
//     inputs DO collide — proving the scope prefix is precisely what prevents it.
//
// This is the in-repo stand-in for "confirm the gate goes red without the
// scopeID": the decision module that exercised the full §4 surface at the corpus
// level is external, so the mechanism is validated directly here.
func TestAnalysisScopeIDDisambiguatesMemoKeys(t *testing.T) {
	parent, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	sub, err := core.NewRegistry() // a module sub-registry mints its own regID
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	if parent.AnalysisScopeID() == sub.AnalysisScopeID() {
		t.Fatalf("registries must have distinct scope ids (got %d for both)", parent.AnalysisScopeID())
	}

	// Identical fn shape on both sides of the boundary: same name, same arg
	// types — the exact §4 collision input.
	name := "decide"
	args := []core.Value{core.NewCarrier(core.TWord), core.NewCarrier(core.TMap)}

	keyParent := check.FnAnalysisKey(parent.AnalysisScopeID(), name, args, nil, nil)
	keySub := check.FnAnalysisKey(sub.AnalysisScopeID(), name, args, nil, nil)
	if keyParent == keySub {
		t.Fatalf("§4 collision: a module fn and a parent fn share a memo key across registries\n  key: %q", keyParent)
	}

	// Disable the disambiguator (constant scope, as a stubbed scopeID would) —
	// the two sides now build the SAME key, which is what re-creates the §4
	// breakage under §5b's shared CheckState. Proving the scope prefix is
	// precisely what keeps them apart.
	constKeyParent := check.FnAnalysisKey(0, name, args, nil, nil)
	constKeySub := check.FnAnalysisKey(0, name, args, nil, nil)
	if constKeyParent != constKeySub {
		t.Fatal("control: identical inputs under a constant scope should produce identical keys")
	}
	if keyParent == constKeyParent && keySub == constKeySub {
		t.Fatal("scope prefix had no effect — keys equal the constant-scope key on both sides")
	}
}

// CheckState.Clone must DEEP-copy the maps + slice so a sandboxed run's in-place
// mutation cannot bleed into the snapshot (and a restore cannot bleed back) —
// the §3.2 invariant that makes the predicate / compile sandboxes sound now that
// Check is a shared *CheckState mutated in place during module-fn analysis. The
// pre-conversion by-value snapshot is gone; this is what replaces it.
func TestCheckStateCloneIsolatesMaps(t *testing.T) {
	c := &core.CheckState{
		Mode:             true,
		StepCount:        5,
		FnSummaries:      map[string][]core.Value{"a": {core.NewCarrier(core.TInteger)}},
		FnInflight:       map[string]bool{"a": true},
		FnAnalysisCounts: map[string]int{"a": 1},
		DefsInstalled:    map[string]core.SrcPos{"a": {}},
		DefsUsed:         map[string]bool{"a": true},
		ContextTypes:     map[string]core.Value{"a": core.NewCarrier(core.TInteger)},
		Diagnostics:      []core.CheckDiagnostic{{Code: "x"}},
	}
	cp := c.Clone()

	// Mutate every map + the slice + a scalar on the clone.
	cp.FnSummaries["b"] = nil
	cp.FnInflight["b"] = true
	cp.FnAnalysisCounts["b"] = 2
	cp.DefsInstalled["b"] = core.SrcPos{}
	cp.DefsUsed["b"] = true
	cp.ContextTypes["b"] = core.NewCarrier(core.TWord)
	cp.Diagnostics = append(cp.Diagnostics, core.CheckDiagnostic{Code: "y"})
	cp.StepCount = 99

	// The original must observe none of it.
	if _, ok := c.FnSummaries["b"]; ok {
		t.Error("FnSummaries leaked from clone to original")
	}
	if c.FnInflight["b"] || c.FnAnalysisCounts["b"] != 0 || c.DefsUsed["b"] {
		t.Error("a bool/int map leaked from clone to original")
	}
	if _, ok := c.DefsInstalled["b"]; ok {
		t.Error("DefsInstalled leaked from clone to original")
	}
	if _, ok := c.ContextTypes["b"]; ok {
		t.Error("ContextTypes leaked from clone to original")
	}
	if len(c.Diagnostics) != 1 {
		t.Errorf("Diagnostics leaked: original has %d entries, want 1", len(c.Diagnostics))
	}
	if c.StepCount != 5 {
		t.Errorf("scalar leaked: StepCount=%d, want 5", c.StepCount)
	}
}
