package core

// Core-suite pins for the NUR run's check-state, replay-screen, bind-twin,
// pending-drain and predicate-admission additions. Each helper here is
// production-called only from check/, compiler/ or basic/, so core's OWN
// suite (cover-gate-core) reaches them only through tests like these,
// which drive the seam directly with positive and negative cases paired.

import (
	"errors"
	"strings"
	"testing"
)

// --- CheckState: Clone's deep copies (RaiseWatches / RootDefSites / FnReads) ---

// Pins that Clone deep-copies the raise watches (snap maps included), the
// root def sites and the fn-read sets: a clone mutated after the fact must
// not write through to the original. A state with none of them clones them
// as nil.
func TestNurRunCloneDeepCopiesLateBindingAndRaiseState(t *testing.T) {
	c := &CheckState{
		RaiseWatches: []RaiseWatch{
			{Nested: 1, Fn: 0, Hit: true, Snap: map[string]int{"x": 2}},
			{Nested: 2, Fn: 1}, // nil snap stays nil through cloneIntMap
		},
		RootDefSites: map[string][]SrcPos{"x": {{Row: 1, Col: 1}}},
		FnReads:      map[string]map[string]bool{"f": {"x": true}},
	}
	cp := c.Clone()
	if len(cp.RaiseWatches) != 2 || !cp.RaiseWatches[0].Hit || cp.RaiseWatches[0].Snap["x"] != 2 {
		t.Fatalf("clone lost the raise watches: %+v", cp.RaiseWatches)
	}
	if cp.RaiseWatches[1].Snap != nil {
		t.Fatalf("a nil watch snap must clone as nil, got %v", cp.RaiseWatches[1].Snap)
	}
	cp.RaiseWatches[0].Snap["x"] = 9
	cp.RaiseWatches[0].Hit = false
	cp.RootDefSites["x"][0].Row = 7
	cp.FnReads["f"]["y"] = true
	if c.RaiseWatches[0].Snap["x"] != 2 || !c.RaiseWatches[0].Hit {
		t.Fatal("clone shares a raise watch with the original")
	}
	if c.RootDefSites["x"][0].Row != 1 {
		t.Fatal("clone shares a root def-site slice with the original")
	}
	if c.FnReads["f"]["y"] {
		t.Fatal("clone shares an fn-read set with the original")
	}

	// Negative: none of the three set — the clone carries none.
	empty := (&CheckState{}).Clone()
	if empty.RaiseWatches != nil || empty.RootDefSites != nil || empty.FnReads != nil {
		t.Fatalf("an empty state must clone empty: %+v %+v %+v", empty.RaiseWatches, empty.RootDefSites, empty.FnReads)
	}
}

// --- CheckState: recordUse's FnReads arm ---------------------------------------

// Pins that a read recorded while a named fn is under analysis lands in that
// fn's read set (the innermost name on FnNameStack), and that a read with no
// fn on the stack — or with check mode off — records no fn read.
func TestNurRunRecordUseTracksFnReads(t *testing.T) {
	c := &CheckState{Mode: true, FnNameStack: []string{"outer", "f"}}
	c.recordUse("x")
	c.recordUse("y")
	if !c.FnReads["f"]["x"] || !c.FnReads["f"]["y"] || len(c.FnReads) != 1 {
		t.Fatalf("reads must attribute to the innermost fn: %v", c.FnReads)
	}
	if !c.DefsUsed["x"] || !c.DefsUsed["y"] {
		t.Fatalf("the use itself must still be recorded: %v", c.DefsUsed)
	}

	// Negative: at the root (no fn on the stack) the use is recorded but no
	// fn read is.
	c.FnNameStack = nil
	c.recordUse("z")
	if !c.DefsUsed["z"] {
		t.Fatal("a root read is still a use")
	}
	for fn, reads := range c.FnReads {
		if reads["z"] {
			t.Fatalf("a root read was attributed to %q", fn)
		}
	}

	// Negative: check mode off — nothing at all.
	off := &CheckState{FnNameStack: []string{"f"}}
	off.recordUse("x")
	if off.FnReads != nil || off.DefsUsed != nil {
		t.Fatal("an inactive check must record nothing")
	}
}

// --- CheckState: NoteRootDefSite ----------------------------------------------

// Pins that NoteRootDefSite appends each positioned root def in program
// order, and ignores a nil receiver, an empty name and an unpositioned def.
func TestNurRunNoteRootDefSite(t *testing.T) {
	(*CheckState)(nil).NoteRootDefSite("x", SrcPos{Row: 1, Col: 1}) // must not panic

	c := &CheckState{}
	c.NoteRootDefSite("", SrcPos{Row: 1, Col: 1})
	c.NoteRootDefSite("x", SrcPos{})
	if c.RootDefSites != nil {
		t.Fatalf("an unnamed or unpositioned def must record nothing: %v", c.RootDefSites)
	}

	c.NoteRootDefSite("x", SrcPos{Row: 1, Col: 1})
	c.NoteRootDefSite("x", SrcPos{Row: 4, Col: 2})
	if got := c.RootDefSites["x"]; len(got) != 2 || got[0].Row != 1 || got[1].Row != 4 {
		t.Fatalf("sites must accumulate in program order: %v", got)
	}
}

// --- cloneIntMap ------------------------------------------------------------------

// Pins cloneIntMap: nil stays nil; a non-nil map is copied, not shared.
func TestNurRunCloneIntMap(t *testing.T) {
	if cloneIntMap(nil) != nil {
		t.Fatal("nil must clone as nil")
	}
	src := map[string]int{"a": 1, "b": 2}
	cp := cloneIntMap(src)
	if len(cp) != 2 || cp["a"] != 1 || cp["b"] != 2 {
		t.Fatalf("clone lost entries: %v", cp)
	}
	cp["a"] = 5
	if src["a"] != 1 {
		t.Fatal("clone shares storage with the source")
	}
}

// --- CheckState: the do-body raise watches (NUR134) ---------------------------

// Pins the raise-watch stack: nil receivers and an empty stack are inert; a
// raise at another nesting or fn-body depth misses; a raise at the watch's
// own level hits and takes the snapshot, and only the FIRST hit does.
func TestNurRunRaiseWatches(t *testing.T) {
	var nilState *CheckState
	nilState.PushRaiseWatch()
	nilState.NoteDefiniteRaise(func() map[string]int { t.Fatal("nil state must not snapshot"); return nil })
	if hit, snap := nilState.PopRaiseWatch(); hit || snap != nil {
		t.Fatal("a nil state has no watch to pop")
	}

	c := &CheckState{}
	c.NoteDefiniteRaise(func() map[string]int { t.Fatal("no watch open: must not snapshot"); return nil })
	if hit, snap := c.PopRaiseWatch(); hit || snap != nil {
		t.Fatal("an empty stack pops nothing")
	}

	// The watch opens one nesting level below the current one.
	c.NestedBodyDepth, c.FnBodyDepth = 2, 1
	c.PushRaiseWatch()
	if w := c.RaiseWatches[0]; w.Nested != 3 || w.Fn != 1 || w.Hit {
		t.Fatalf("watch opened at the wrong level: %+v", w)
	}

	snaps := 0
	snap := func() map[string]int { snaps++; return map[string]int{"k": snaps} }

	// Negative: a raise nested deeper (a branch arm) is conditional.
	c.NestedBodyDepth = 4
	c.NoteDefiniteRaise(snap)
	// Negative: at the right nesting but inside a called fn's body.
	c.NestedBodyDepth, c.FnBodyDepth = 3, 2
	c.NoteDefiniteRaise(snap)
	if c.RaiseWatches[0].Hit || snaps != 0 {
		t.Fatal("a raise at another level must not hit the watch")
	}

	// Positive: at the watch's own level — hit, snapshot taken once.
	c.FnBodyDepth = 1
	c.NoteDefiniteRaise(snap)
	c.NoteDefiniteRaise(snap)
	if snaps != 1 {
		t.Fatalf("only the first hit snapshots, got %d snapshots", snaps)
	}
	hit, got := c.PopRaiseWatch()
	if !hit || got["k"] != 1 {
		t.Fatalf("pop must report the first hit's snapshot: %v %v", hit, got)
	}
	if len(c.RaiseWatches) != 0 {
		t.Fatal("pop must close the watch")
	}
}

// --- CheckState: EmitLateBindingHints (NUR097) --------------------------------

// Pins the late-binding hint: one info diagnostic, at the later def, for a fn
// that reads a name bound BEFORE the fn and re-bound after it — and nothing
// for a nil state, empty evidence, a fn with no root site, the fn's own name,
// or a name first defined after the fn (an ordinary forward reference).
func TestNurRunEmitLateBindingHints(t *testing.T) {
	(*CheckState)(nil).EmitLateBindingHints() // must not panic

	empty := &CheckState{}
	empty.EmitLateBindingHints()
	onlyReads := &CheckState{FnReads: map[string]map[string]bool{"f": {"x": true}}}
	onlyReads.EmitLateBindingHints()
	if len(empty.Diagnostics)+len(onlyReads.Diagnostics) != 0 {
		t.Fatal("missing evidence must hint nothing")
	}

	c := &CheckState{
		FnReads: map[string]map[string]bool{
			"f":    {"f": true, "fwd": true, "x": true, "same": true},
			"anon": {"x": true}, // no root def site: skipped
		},
		RootDefSites: map[string][]SrcPos{
			"x":    {{Row: 1, Col: 1}, {Row: 3, Col: 5}}, // before AND after f
			"f":    {{Row: 2, Col: 1}},                   // f's own def
			"fwd":  {{Row: 4, Col: 1}},                   // first defined after f
			"same": {{Row: 2, Col: 0}, {Row: 2, Col: 1}}, // before f on its row, then at f's own site
		},
	}
	c.EmitLateBindingHints()
	if len(c.Diagnostics) != 1 {
		t.Fatalf("want exactly one hint, got %+v", c.Diagnostics)
	}
	d := c.Diagnostics[0]
	if d.Code != "late_binding" || d.Word != "f" || d.Row != 3 || d.Col != 5 {
		t.Fatalf("hint mis-attributed: %+v", d)
	}
	if !strings.Contains(d.Detail, "`f` reads `x`, re-def'ed at line 3") {
		t.Fatalf("hint detail: %q", d.Detail)
	}
	if d.Severity != SeverityInfo {
		t.Fatalf("late_binding is info, got %q", d.Severity)
	}
}

// --- replay_hazard.go ---------------------------------------------------------------

// Pins BindNameToken: a bare word or a quoted atom yields its name; anything
// else (a computed name) yields "".
func TestNurRunBindNameToken(t *testing.T) {
	if got := BindNameToken(NewWord("Foo")); got != "Foo" {
		t.Fatalf("word: %q", got)
	}
	if got := BindNameToken(NewAtom("bar")); got != "bar" {
		t.Fatalf("atom: %q", got)
	}
	if got := BindNameToken(NewInteger(3)); got != "" {
		t.Fatalf("a computed operand has no static name: %q", got)
	}
}

// Pins BodyHasReplayHazard: an import, and a capitalised def/var/undef, flag
// the body (also nested in a list or a paren); a lowercase binder, a binder
// with no operand, a non-container and a hazard-free nested body do not.
func TestNurRunBodyHasReplayHazard(t *testing.T) {
	list := func(vs ...Value) Value { return NewList(vs) }
	for _, tc := range []struct {
		name string
		body Value
		want bool
	}{
		{"import", list(NewWord("import"), NewString("./m.boru")), true},
		{"capitalised def", list(NewWord("def"), NewWord("T"), NewInteger(1)), true},
		{"capitalised var atom", list(NewWord("var"), NewAtom("Tv"), NewInteger(1)), true},
		{"capitalised undef", list(NewWord("undef"), NewWord("T")), true},
		{"nested in a paren", NewParenExpr([]Value{NewInteger(1), list(NewWord("def"), NewWord("T"), NewInteger(2))}), true},
		{"lowercase def", list(NewWord("def"), NewWord("t"), NewInteger(1)), false},
		{"binder without operand", list(NewInteger(1), NewWord("def")), false},
		{"hazard-free nesting", list(list(NewWord("def"), NewWord("t"), NewInteger(1)), NewWord("other")), false},
		{"not a container", NewInteger(7), false},
	} {
		if got := BodyHasReplayHazard(tc.body); got != tc.want {
			t.Errorf("%s: BodyHasReplayHazard = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// --- bind_twin_apply.go: the type-name re-check ---------------------------------

// Pins TypeNameFree: a name whose part the run already reserved conflicts
// (type_error, the front door's detail), a live same-named type binding is a
// redefinition and passes, and a fresh name passes.
func TestNurRunTypeNameFree(t *testing.T) {
	r := newTestRegistry(t)
	if err := TypeNameFree(r, "NrcFreshName"); err != nil {
		t.Fatalf("a fresh name must be free: %v", err)
	}
	r.RegisterPart("NrcClash")
	err := TypeNameFree(r, "NrcClash")
	var ae *BoruError
	if !errors.As(err, &ae) || ae.Code != "type_error" || !strings.Contains(ae.Detail, "conflicts with an existing type name") {
		t.Fatalf("a reserved part must conflict, got %v", err)
	}
	// Positive: the same reserved part under a LIVE type binding of that
	// name is a redefinition, which the front door lets through.
	r.Defs.PushType("NrcClash", r.Types.MintType("NrcClash", TInteger), NewTypeLiteral(TInteger))
	if err := TypeNameFree(r, "NrcClash"); err != nil {
		t.Fatalf("a live same-named type binding must pass: %v", err)
	}
}

// Pins that a type twin (minted or adopted) whose name a run-time mint
// already holds raises instead of pushing, and that a free name still
// replays and reserves its part.
func TestNurRunApplyBindTwinTypeNameConflict(t *testing.T) {
	r := newTestRegistry(t)
	minted := r.Types.MintType("NrcTwinTaken", TInteger)

	r.RegisterPart("NrcTwinTaken")
	err := ApplyBindTwin(r, BindTransition{Kind: BindTypeInstall, Name: "NrcTwinTaken"},
		DefEntry{Body: NewTypeLiteral(TInteger), TypeDef: minted, Minted: true})
	if err == nil || !strings.Contains(err.Error(), "NrcTwinTaken") {
		t.Fatalf("minted twin over a held part must raise, got %v", err)
	}
	if r.Defs.IsType("NrcTwinTaken") {
		t.Fatal("a refused minted twin must not push")
	}

	r.RegisterPart("NrcTwinAliasTaken")
	err = ApplyBindTwin(r, BindTransition{Kind: BindTypeInstall, Name: "NrcTwinAliasTaken"},
		DefEntry{Body: NewTypeLiteral(TInteger), TypeDef: TInteger})
	if err == nil {
		t.Fatal("adopted twin over a held part must raise")
	}
	if r.Defs.IsType("NrcTwinAliasTaken") {
		t.Fatal("a refused adopted twin must not push")
	}

	// Positive: a free name replays and reserves the part.
	if err := ApplyBindTwin(r, BindTransition{Kind: BindTypeInstall, Name: "NrcTwinFree"},
		DefEntry{Body: NewTypeLiteral(TInteger), TypeDef: TInteger}); err != nil {
		t.Fatalf("a free name must replay: %v", err)
	}
	if !r.Defs.IsType("NrcTwinFree") || !r.IsKnownPart("NrcTwinFree") {
		t.Fatal("a replayed type twin must bind and reserve its name")
	}
}

// --- analysis_hooks.go: the foreign-registry drain arm (NUR128) ----------------

// Pins RunPendingFnBodyChecks over a body queued for ANOTHER registry (an
// exported module fn): the drain shares the pass's state into that registry,
// forces a re-analysis, and keeps only the structural unreachable_branch
// findings it produced — while a body of the pass's own registry keeps every
// finding, and the findings made before the drain are untouched.
func TestNurRunPendingDrainForeignRegistryKeepsStructuralOnly(t *testing.T) {
	r := newTestRegistry(t)
	module := newTestRegistry(t)
	done := r.Check.Begin()
	t.Cleanup(done)

	savedPass := AnalysisImpl.FnConstructionPass
	savedShare := CheckBraid.ShareCheckStateFrom
	t.Cleanup(func() {
		AnalysisImpl.FnConstructionPass = savedPass
		CheckBraid.ShareCheckStateFrom = savedShare
	})
	// The check side's sharing, modelled: owner analyses on caller's state.
	shared := 0
	CheckBraid.ShareCheckStateFrom = func(owner, caller *Registry) func() {
		shared++
		prev := owner.Check
		owner.Check = caller.Check
		return func() { owner.Check = prev }
	}
	forced := map[string]bool{}
	AnalysisImpl.FnConstructionPass = func(reg *Registry, name string, fn FnDefInfo) {
		forced[name] = reg.Check.ForceFnReanalysis
		reg.Check.AddDiagnostic(CheckDiagnostic{Code: "unreachable_branch", Detail: name + " dead arm"})
		reg.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Detail: name + " unresolved"})
	}

	r.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Detail: "before the drain"})
	NoteFnBodyPendingIn(r, module, FnDefInfo{Name: "exported"})
	NoteFnBodyPending(r, FnDefInfo{Name: "own"})
	// Negative guards: no owner / no registry queues nothing.
	NoteFnBodyPendingIn(nil, module, FnDefInfo{Name: "orphan"})
	NoteFnBodyPendingIn(r, nil, FnDefInfo{Name: "orphan"})
	if got := len(r.Check.PendingFnBodies); got != 2 {
		t.Fatalf("want 2 queued bodies, got %d", got)
	}

	RunPendingFnBodyChecks(r)

	if shared != 2 {
		t.Fatalf("each drained body shares the pass state, got %d shares", shared)
	}
	if !forced["exported"] || forced["own"] {
		t.Fatalf("only the foreign body forces re-analysis: %v", forced)
	}
	if r.Check.ForceFnReanalysis {
		t.Fatal("the force flag must be reset after the drain")
	}
	if module.Check == r.Check {
		t.Fatal("the shared state must be restored on the module registry")
	}
	var got []string
	for _, d := range r.Check.Diagnostics {
		got = append(got, d.Code+":"+d.Detail)
	}
	want := []string{
		"undefined_word:before the drain",
		"unreachable_branch:exported dead arm",
		"unreachable_branch:own dead arm",
		"undefined_word:own unresolved",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("diagnostics after the drain:\n got %v\nwant %v", got, want)
	}
}

// Pins the named inactive default of CheckBraid.ShareCheckStateFrom: a
// check-less core shares nothing, and its restore is a callable no-op.
func TestNurRunInactiveShareCheckStateFrom(t *testing.T) {
	owner := newTestRegistry(t)
	caller := newTestRegistry(t)
	before := owner.Check
	restore := inactiveShareCheckStateFrom(owner, caller)
	if restore == nil {
		t.Fatal("the inactive default must return a restore func")
	}
	if owner.Check != before || owner.Check == caller.Check {
		t.Fatal("the inactive default must not share the caller's state")
	}
	restore()
	if owner.Check != before {
		t.Fatal("the inactive restore must leave the owner's state alone")
	}
}

// --- unify_predicate.go: the analysis-time carrier admission (NUR102) ---------

// nrcPredicateUnifier installs `name` as a predicate type over input whose
// body is body, and returns the minted node with its PredicateUnifier.
func nrcPredicateUnifier(t *testing.T, r *Registry, name string, input *Type, body []Value) (*Type, *PredicateUnifier) {
	t.Helper()
	fn := NewValueRaw(TFunction, FnDefInfo{Signatures: []Signature{{
		Params:  []FnParam{{Name: "n", Type: input}},
		Returns: []*Type{TBoolean},
		Impl:    Boru(body),
	}}})
	if err := InstallType(r, name, fn); err != nil {
		t.Fatalf("InstallType(%s): %v", name, err)
	}
	def := r.LookupTypeName(name)
	if def == nil {
		t.Fatalf("LookupTypeName(%s) = nil", name)
	}
	p, ok := def.Behavior().(*PredicateUnifier)
	if !ok {
		t.Fatalf("%s carries no PredicateUnifier: %T", name, def.Behavior())
	}
	return def, p
}

// Pins that under an active analysis a carrier of the predicate's input type
// is admitted without running the body (the arm stays reachable), while a
// carrier of an unrelated type, and any carrier outside analysis, fall to
// the lattice walk and are refused.
func TestNurRunPredicateUnifierAdmitsInputTypedCarrier(t *testing.T) {
	r := newTestRegistry(t)
	// A body that would REFUSE every candidate — the admission must not run it.
	def, p := nrcPredicateUnifier(t, r, "NrcNever", TInteger, []Value{NewBoolean(false)})

	if p.Match(NewCarrier(TInteger), def) {
		t.Fatal("outside analysis a carrier is refused by the lattice walk")
	}

	done := r.Check.Begin()
	defer done()
	if !p.Match(NewCarrier(TInteger), def) {
		t.Fatal("an Integer carrier must be admitted under analysis")
	}
	if p.Match(NewCarrier(TString), def) {
		t.Fatal("a carrier of an unrelated type must not be admitted")
	}
}
