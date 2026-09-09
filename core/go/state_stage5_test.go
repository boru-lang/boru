package core

import "testing"

// TestCheckStateCloneDeep pins Clone's deep-copy contract: the nil
// receiver clones to nil, and a populated state's diagnostics slice,
// maps, nested binder/call-graph sets, and fn-name stack are all
// duplicated so sandbox mutation cannot bleed into the snapshot.
func TestCheckStateCloneDeep(t *testing.T) {
	if (*CheckState)(nil).Clone() != nil {
		t.Fatal("nil CheckState must clone to nil")
	}
	c := NewCheckState()
	c.Diagnostics = []CheckDiagnostic{{Code: "unused_def", Word: "x"}}
	c.FnSummaries = map[string][]Value{"f#Integer": {NewCarrier(TInteger)}}
	c.FnInflight = map[string]bool{"f#Integer": true}
	c.FnNameInflight = map[string]int{"f": 1}
	c.FnAnalysisCounts = map[string]int{"f": 2}
	c.DefsInstalled = map[string]SrcPos{"x": {Row: 1, Col: 2}}
	c.DefsUsed = map[string]bool{"x": true}
	c.ContextTypes = map[string]Value{"k": NewCarrier(TString)}
	c.CtxShapes = map[*StoreInstanceInfo]Value{{}: NewCarrier(TStore)}
	c.MethodShapes = map[string]Value{"id": NewInteger(1)}
	c.FnBinders = map[string]map[string]bool{"x": {"f": true}}
	c.FnCallGraph = map[string]map[string]bool{"f": {"g": true}}
	c.FnNameStack = []string{"f"}

	cp := c.Clone()
	if cp == c {
		t.Fatal("Clone must return a distinct pointer")
	}
	cp.Diagnostics[0].Code = "mutated"
	if c.Diagnostics[0].Code != "unused_def" {
		t.Fatal("Diagnostics must be cloned, not aliased")
	}
	cp.FnBinders["x"]["h"] = true
	if c.FnBinders["x"]["h"] {
		t.Fatal("FnBinders inner sets must be cloned, not aliased")
	}
	cp.FnCallGraph["f"]["h"] = true
	if c.FnCallGraph["f"]["h"] {
		t.Fatal("FnCallGraph inner sets must be cloned, not aliased")
	}
	cp.FnNameStack[0] = "mutated"
	if c.FnNameStack[0] != "f" {
		t.Fatal("FnNameStack must be cloned, not aliased")
	}
	cp.FnSummaries["other"] = nil
	if _, ok := c.FnSummaries["other"]; ok {
		t.Fatal("FnSummaries header must be cloned, not aliased")
	}
	// cloneNestedSet / cloneMap nil arms.
	if cloneNestedSet(nil) != nil {
		t.Fatal("cloneNestedSet(nil) must be nil")
	}
	if cloneMap[string, int](nil) != nil {
		t.Fatal("cloneMap(nil) must be nil")
	}
}

// TestCheckStateSmallAccessors covers the passive helpers: ModelsEffects,
// the speculative-baseline bracket, the budget isolator, and the
// empty-call-graph reachability decline.
func TestCheckStateSmallAccessors(t *testing.T) {
	if (*CheckState)(nil).ModelsEffects() {
		t.Fatal("nil CheckState must not model effects")
	}
	c := NewCheckState()
	if c.ModelsEffects() {
		t.Fatal("fresh CheckState must not model effects")
	}
	c.ModelEffects = true
	if !c.ModelsEffects() {
		t.Fatal("ModelEffects=true must report modelling")
	}

	// Push/PopSpecBaseline bracket one speculative region.
	snap := map[string]int{"x": 1}
	c.PushSpecBaseline(snap)
	if len(c.SpecBaselines) != 1 {
		t.Fatal("PushSpecBaseline must push")
	}
	c.PopSpecBaseline()
	if len(c.SpecBaselines) != 0 {
		t.Fatal("PopSpecBaseline must pop")
	}

	// IsolateBudget: nil receiver no-op; real receiver snapshots and
	// restores StepCount/BudgetTripped.
	(*CheckState)(nil).IsolateBudget()()
	c.StepCount = 7
	c.BudgetTripped = false
	restore := c.IsolateBudget()
	c.StepCount = 999
	c.BudgetTripped = true
	restore()
	if c.StepCount != 7 || c.BudgetTripped {
		t.Fatalf("IsolateBudget restore: got StepCount=%d tripped=%v, want 7/false", c.StepCount, c.BudgetTripped)
	}

	// callReaches declines outright on an empty call graph.
	if c.callReaches("f", "g") {
		t.Fatal("empty call graph must not reach")
	}
}

// TestAddDiagnosticCaughtBody pins the central caught-region downgrade:
// inside an error-trapping region (CaughtBodyDepth > 0) an error-severity
// finding is re-attributed to info with CaughtAtRuntime stamped.
func TestAddDiagnosticCaughtBody(t *testing.T) {
	c := NewCheckState()
	c.CaughtBodyDepth = 1
	c.AddDiagnostic(CheckDiagnostic{Code: "type_error", Detail: "d"})
	if len(c.Diagnostics) != 1 {
		t.Fatalf("want 1 diagnostic, got %d", len(c.Diagnostics))
	}
	d := c.Diagnostics[0]
	if d.Severity != SeverityInfo || !d.CaughtAtRuntime {
		t.Fatalf("caught-region error must downgrade to info+CaughtAtRuntime, got %q caught=%v", d.Severity, d.CaughtAtRuntime)
	}
}

// TestNoteMethodShapeArms drives NoteMethodShape's vetting arms and the
// annotation store, plus the MethodShapeMember reader.
func TestNoteMethodShapeArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	c := NewCheckState()
	c.Mode = true

	out := NewCarrier(TFunction)
	out.ID = "ms$1"

	// Non-FnDefInfo member: vetted out.
	c.NoteMethodShape(out, NewInteger(1))
	if len(c.MethodShapes) != 0 {
		t.Fatal("non-fn member must not annotate")
	}
	// FnDefInfo but no registry / no name: vetted out.
	c.NoteMethodShape(out, NewFunction(FnDefInfo{Name: "m"}))
	if len(c.MethodShapes) != 0 {
		t.Fatal("registry-less member must not annotate")
	}
	// Named + registry-carrying but NOT a trivial delegation (real body).
	real := FnDefInfo{Name: "m", Registry: r, Signatures: []Signature{{
		Params: []FnParam{{Type: TAny}},
		Impl:   Boru([]Value{NewInteger(1)}),
	}}}
	c.NoteMethodShape(out, NewFunction(real))
	if len(c.MethodShapes) != 0 {
		t.Fatal("non-delegation member must not annotate")
	}
	// The shaped-instance-method class: a named trivial-delegation
	// wrapper (body `[Word(inner)]`, all-unnamed params) carrying a
	// sub-registry.
	member := NewFunction(FnDefInfo{Name: "m", Registry: r, Signatures: []Signature{{
		Params: []FnParam{{Type: TAny}},
		Impl:   Boru([]Value{NewWord("inner")}),
	}}})
	c.NoteMethodShape(out, member)
	if len(c.MethodShapes) != 1 {
		t.Fatal("delegation member must annotate")
	}
	got, ok := c.MethodShapeMember("ms$1")
	if !ok || !ValuesEqual(got, member) {
		t.Fatal("MethodShapeMember must return the annotated member")
	}
	// Reader declines: nil receiver, empty id, unknown id.
	if _, ok := (*CheckState)(nil).MethodShapeMember("ms$1"); ok {
		t.Fatal("nil receiver must decline")
	}
	if _, ok := c.MethodShapeMember(""); ok {
		t.Fatal("empty id must decline")
	}
	if _, ok := c.MethodShapeMember("nope"); ok {
		t.Fatal("unknown id must decline")
	}
}

// TestContextShape pins the per-layer shape minting: nil receiver/store
// degrade to a plain Store carrier; a live layer mints once and re-uses
// the same shape pointee with a fresh ID per call.
func TestContextShape(t *testing.T) {
	if v := (*CheckState)(nil).ContextShape(&StoreInstanceInfo{}, 0); !v.Carrier || !v.Parent.Equal(TStore) {
		t.Fatal("nil receiver must return a plain Store carrier")
	}
	c := NewCheckState()
	if v := c.ContextShape(nil, 0); !v.Carrier || !v.Parent.Equal(TStore) {
		t.Fatal("nil store must return a plain Store carrier")
	}
	store := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{}}
	v1 := c.ContextShape(store, 3)
	v2 := c.ContextShape(store, 3)
	if v1.ID == "" || v2.ID == "" || v1.ID == v2.ID {
		t.Fatalf("each call must mint a fresh ID, got %q / %q", v1.ID, v2.ID)
	}
	s1, ok1 := v1.Data.(*StoreShapeInfo)
	s2, ok2 := v2.Data.(*StoreShapeInfo)
	if !ok1 || !ok2 || s1 != s2 {
		t.Fatal("one live layer must share one shape pointee")
	}
	if len(c.CtxShapes) != 1 {
		t.Fatalf("want one minted shape, got %d", len(c.CtxShapes))
	}
}

// TestBeginCompilePassAndIsolateEmit pins the compile-pass arming ritual
// and the hermetic emit swap against the core-only inactive hooks.
func TestBeginCompilePassAndIsolateEmit(t *testing.T) {
	// Nil receiver: Begin's no-op closure comes back.
	(*CheckState)(nil).BeginCompilePass()()

	c := NewCheckState()
	c.FnSummaries = map[string][]Value{"stale": nil}
	c.FnInflight = map[string]bool{"stale": true}
	done := c.BeginCompilePass()
	defer done()
	if !c.Compiling {
		t.Fatal("BeginCompilePass must mark the pass Compiling")
	}
	if c.FnSummaries != nil || c.FnInflight != nil {
		t.Fatal("BeginCompilePass must drop the fn-body memos")
	}
	// The core-only build's constructor slot hands back the inactive
	// recorder (the compiler piece installs the real one at init).
	if c.Emit != TheInactiveEmit {
		t.Fatalf("core-only BeginCompilePass recorder = %T, want the inactive no-op", c.Emit)
	}

	// IsolateEmit: nil receiver no-op; real receiver swaps and restores.
	(*CheckState)(nil).IsolateEmit()()
	saved := c.Emit
	restore := c.IsolateEmit()
	if c.Emit != TheInactiveEmit {
		t.Fatalf("core-only IsolateEmit recorder = %T, want the inactive no-op", c.Emit)
	}
	restore()
	if c.Emit != saved {
		t.Fatal("IsolateEmit restore must reinstall the saved recorder")
	}
}

// TestSpecUndefBlocked pins the speculative-region undef gate on a
// registry: inactive outside check mode / without a region, blocking a
// pre-region binding inside one, and letting an in-region push pop.
func TestSpecUndefBlocked(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if r.SpecUndefBlocked("x") {
		t.Fatal("outside check mode must not block")
	}
	r.Check.Mode = true
	defer func() { r.Check.Mode = false }()
	if r.SpecUndefBlocked("x") {
		t.Fatal("no speculative region must not block")
	}
	r.Check.PushSpecBaseline(map[string]int{"x": 0})
	defer r.Check.PopSpecBaseline()
	// Depth("x") == 0 <= baseline 0: popping would delete an enclosing
	// binding from the model — blocked.
	if !r.SpecUndefBlocked("x") {
		t.Fatal("pre-region binding must block")
	}
	// A binding pushed INSIDE the region (depth above the snapshot) pops
	// normally.
	r.Defs.Push("x", NewInteger(1))
	defer r.Defs.Pop("x")
	if r.SpecUndefBlocked("x") {
		t.Fatal("in-region binding must not block")
	}
}

// TestRescueForwardRefDiagnostics pins the end-of-pass forward-reference
// rescue: nil receiver and diagnostic-less registries return early, and
// a fn-body undefined_word whose name is bound by end of pass is dropped
// while an unbound one is kept.
func TestRescueForwardRefDiagnostics(t *testing.T) {
	(*Registry)(nil).RescueForwardRefDiagnostics()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.RescueForwardRefDiagnostics() // nil Diagnostics: early return

	r.Defs.Push("later", NewInteger(1))
	defer r.Defs.Pop("later")
	r.Check.Diagnostics = []CheckDiagnostic{
		{Code: "undefined_word", FnBody: true, Word: "later"},
		{Code: "undefined_word", FnBody: true, Word: "genuinely-missing"},
	}
	r.RescueForwardRefDiagnostics()
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Word != "genuinely-missing" {
		t.Fatalf("rescue must drop only the bound forward ref, got %+v", r.Check.Diagnostics)
	}
}

// TestIsolateFnAnalysisArms pins the seventh leak channel's isolation (the
// synthetic help-example evaluation runs a real AnalyseFnBody in the registry
// that is mid-pass — see IsolateFnAnalysis): a nil receiver is a no-op, the
// memo of FINISHED analyses is restored, and the record of analyses still
// OWED is deliberately left alone, because the hook fires mid-install and the
// enclosing pass has work in flight across it.
func TestIsolateFnAnalysisArms(t *testing.T) {
	(*CheckState)(nil).IsolateFnAnalysis()() // nil receiver: no-op

	c := &CheckState{}
	c.FnSummaries = map[string][]Value{"own": {NewInteger(1)}}
	c.FnAnalysisCounts = map[string]int{"own": 1}
	c.FnInflight = map[string]bool{"own": true}
	c.PendingFnBodies = []PendingFnBody{{Fn: FnDefInfo{Name: "own"}}}

	restore := c.IsolateFnAnalysis()
	// The synthetic run's memo writes, which must be undone...
	c.FnSummaries["leaked"] = []Value{NewInteger(2)}
	c.FnAnalysisCounts["leaked"] = 9
	// ...and the enclosing pass's own in-flight work, which must NOT be.
	c.FnInflight["queued"] = true
	c.PendingFnBodies = append(c.PendingFnBodies, PendingFnBody{Fn: FnDefInfo{Name: "queued"}})
	restore()

	if _, leaked := c.FnSummaries["leaked"]; leaked {
		t.Error("FnSummaries kept the isolated region's write")
	}
	if _, leaked := c.FnAnalysisCounts["leaked"]; leaked {
		t.Error("FnAnalysisCounts kept the isolated region's write")
	}
	// The negative that matters: restoring must not CLEAR the pass's own
	// entries, nor drop work queued while the hook ran — dropping the pending
	// body is what regressed four compiling shapes to "unknown provenance".
	if len(c.FnSummaries["own"]) != 1 || c.FnAnalysisCounts["own"] != 1 {
		t.Error("the restore dropped the pass's own memo entries")
	}
	if !c.FnInflight["queued"] || len(c.PendingFnBodies) != 2 {
		t.Errorf("the restore discarded work the enclosing pass queued across the hook: inflight=%v pending=%d",
			c.FnInflight, len(c.PendingFnBodies))
	}
}
