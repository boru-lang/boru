package core

import "testing"

// The def table's scopes and the def census (deftable.go, def_census.go):
// every class the scope rule names, each paired with the case that records
// nothing, and the table's scope bookkeeping on nil and live receivers.

func TestDefTableScopes(t *testing.T) {
	var none *DefTable
	if none.EnterScope(ScopeFrame) != 0 || none.ScopeID() != 0 || none.ScopeKindNow() != ScopeModule || none.ReadInScope("x") {
		t.Error("a nil table has one scope, the module")
	}
	none.LeaveScope()
	none.NoteRead("x")
	none.PushAt("x", NewInteger(1), SrcPos{})
	none.PushLeaked("x", NewInteger(1), SrcPos{})
	none.SetEntries("x", nil)
	none.SetTopSite("x", SrcPos{Row: 1})

	dt := NewDefTable()
	if dt.ScopeID() != 0 || dt.ScopeKindNow() != ScopeModule {
		t.Fatal("a fresh table is at module scope")
	}
	dt.LeaveScope()  // a no-op at the module scope
	dt.NoteRead("m") // nothing recorded at module scope
	if dt.ReadInScope("m") {
		t.Error("the module scope records no reads")
	}
	site := SrcPos{Row: 1, Col: 5}
	dt.PushAt("m", NewInteger(0), site)
	frame := dt.EnterScope(ScopeFrame)
	if frame != 1 || dt.ScopeID() != 1 || dt.ScopeKindNow() != ScopeFrame {
		t.Errorf("the first scope is 1, a frame: %d %v", frame, dt.ScopeKindNow())
	}
	block := dt.EnterScope(ScopeBlock)
	if block != 2 || dt.ScopeKindNow() != ScopeBlock {
		t.Errorf("the nested scope is 2, a block: %d", block)
	}
	dt.NoteRead("m")
	if !dt.ReadInScope("m") || dt.ReadInScope("n") {
		t.Error("the block read m and not n")
	}
	dt.Push("b", NewInteger(2))
	dt.PushType("T", TInteger, NewTypeLiteral(TInteger))
	dt.PushTypeAdopted("U", TString, NewTypeLiteral(TString))
	dt.PushLeaked("w", NewInteger(3), SrcPos{Row: 2, Col: 1})
	for _, name := range []string{"b", "T", "U", "w"} {
		if e, _ := dt.TopEntry(name); e.Scope != block {
			t.Errorf("%s bound in scope %d, want the block %d", name, e.Scope, block)
		}
	}
	if e, _ := dt.TopEntry("m"); e.Scope != 0 || e.Site != site || e.Leaked {
		t.Errorf("the module entry: %+v", e)
	}
	if e, _ := dt.TopEntry("w"); !e.Leaked || e.Site.Row != 2 {
		t.Errorf("the leaked entry: %+v", e)
	}
	dt.LeaveScope()
	if dt.ScopeID() != frame || dt.ReadInScope("m") {
		t.Error("leaving the block returns to the frame, whose reads are its own")
	}
	dt.LeaveScope()
	if dt.ScopeID() != 0 {
		t.Error("leaving the frame returns to the module")
	}
	// SetEntries keeps each survivor's scope and site; empty removes the name.
	dt.Push("m", NewInteger(9))
	entries := dt.Entries("m")
	dt.SetEntries("m", entries[:1])
	if e, _ := dt.TopEntry("m"); e.Site != site || dt.Depth("m") != 1 {
		t.Errorf("SetEntries lost the survivor's site: %+v", e)
	}
	dt.SetEntries("m", nil)
	if dt.Has("m") {
		t.Error("SetEntries with no entries removes the name")
	}
	dt.SetTopSite("m", site) // unbound: nothing to stamp
	if dt.Has("m") {
		t.Error("stamping an unbound name binds nothing")
	}
	// A clone continues the scope counter, so its scopes never collide with
	// the ids its inherited entries carry.
	cl := dt.Clone()
	if cl.EnterScope(ScopeBlock) != 3 {
		t.Error("the clone's first scope must follow the parent's last")
	}
	for k, want := range map[ScopeKind]string{ScopeModule: "module", ScopeFrame: "frame", ScopeBlock: "block"} {
		if k.String() != want {
			t.Errorf("%d renders %q", k, k.String())
		}
	}
}

// A fn frame is a scope on both lanes: the baseline push opens it.
func TestFnBaselineOpensScope(t *testing.T) {
	r := newTestRegistry(t)
	r.PushFnBaseline(nil)
	if r.Defs.ScopeKindNow() != ScopeFrame {
		t.Fatal("a pushed fn baseline opens a frame scope")
	}
	r.PopFnBaseline()
	if r.Defs.ScopeKindNow() != ScopeModule {
		t.Fatal("popping the baseline closes it")
	}
	r.PopFnBaseline() // empty: still a no-op
	if r.Defs.ScopeKindNow() != ScopeModule {
		t.Fatal("a pop on an empty baseline stack changes nothing")
	}
}

// censusReg is a registry in check mode with a def site staged.
func censusReg(t *testing.T) (*Registry, func()) {
	t.Helper()
	r := newTestRegistry(t)
	done := r.Check.Begin()
	return r, done
}

func siteAt(r *Registry, row, col int) {
	r.Check.PendingBindPos = SrcPos{Row: row, Col: col}
}

func classesOf(r *Registry) []string {
	var out []string
	for _, e := range r.Check.SortedDefCensus() {
		out = append(out, string(e.Class)+":"+e.Name)
	}
	return out
}

func TestDefCensusValueClasses(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	siteAt(r, 1, 1)
	InstallDef(r, "a", NewInteger(1))
	if len(r.Check.DefCensus) != 0 {
		t.Fatal("a first binding is no finding")
	}
	siteAt(r, 1, 9)
	InstallDef(r, "a", NewInteger(2)) // the same scope: a rebind
	r.PushFnBaseline(nil)
	siteAt(r, 2, 1)
	InstallDef(r, "a", NewInteger(3)) // an inner scope, unread: a shadow
	r.noteAnalysisUseAt("b", SrcPos{Row: 2, Col: 5})
	siteAt(r, 1, 1)
	InstallDef(r, "b", NewInteger(0)) // b is not bound anywhere: no finding
	r.Defs.Pop("a")                   // the frame's teardown
	r.Defs.Pop("b")
	r.PopFnBaseline()
	siteAt(r, 3, 1)
	InstallDef(r, "c", NewInteger(0))
	r.Defs.EnterScope(ScopeBlock)
	r.noteAnalysisUseAt("c", SrcPos{Row: 3, Col: 12})
	siteAt(r, 3, 20)
	InstallDef(r, "c", NewInteger(1)) // the block read c before binding it
	r.Defs.LeaveScope()
	// The bookkeeping note (a reference, the opaque-body walk) is no read.
	r.Defs.EnterScope(ScopeBlock)
	r.noteAnalysisUse("a")
	siteAt(r, 3, 30)
	InstallDef(r, "a", NewInteger(4)) // a shadow, not a shadow-rebind
	r.Defs.LeaveScope()
	got := classesOf(r)
	want := []string{"rebind:a", "shadow:a", "shadow-rebind:c", "shadow:a"}
	if len(got) != len(want) {
		t.Fatalf("census %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("census[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	rb := r.Check.SortedDefCensus()[0]
	if rb.Standing != (SrcPos{Row: 1, Col: 1}) || rb.Site != (SrcPos{Row: 1, Col: 9}) || rb.Scope != ScopeModule || rb.Note != "value" {
		t.Errorf("the rebind finding: %+v", rb)
	}
	if sh := r.Check.SortedDefCensus()[1]; sh.Scope != ScopeFrame {
		t.Errorf("the shadow is a frame's: %+v", sh)
	}
	if sr := r.Check.SortedDefCensus()[2]; sr.Scope != ScopeBlock || sr.Standing != (SrcPos{Row: 3, Col: 1}) {
		t.Errorf("the shadow-rebind finding: %+v", sr)
	}
}

func TestDefCensusFnClasses(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	sig := func(ty *Type) FnDefInfo {
		return FnDefInfo{Signatures: []Signature{{
			Params: []FnParam{{Name: "x", Type: ty}}, Returns: []*Type{TInteger},
			Impl: Boru([]Value{NewInteger(1)}), BarrierPos: BarrierAllForward,
		}}}
	}
	// Through InstallDef over a fn VALUE, as `def f fn […]` installs.
	siteAt(r, 1, 1)
	InstallDef(r, "f", NewFunction(sig(TInteger)))
	siteAt(r, 1, 30)
	InstallDef(r, "f", NewFunction(sig(TString))) // disjoint, same scope: an overload added, legal
	if len(r.Check.DefCensus) != 0 {
		t.Fatalf("an added overload is no finding: %v", classesOf(r))
	}
	siteAt(r, 1, 60)
	InstallDef(r, "f", NewFunction(sig(TInteger))) // overlaps the first: today a replace
	r.PushFnBaseline(nil)
	siteAt(r, 2, 1)
	InstallDef(r, "f", NewFunction(sig(TFloat))) // disjoint but from an inner scope
	r.Defs.Pop("f")                              // the frame's teardown
	r.PopFnBaseline()
	r.PushFnBaseline(nil)
	siteAt(r, 2, 40)
	InstallDef(r, "f", NewInteger(5)) // a value over the word: a legal shadow
	r.Defs.Pop("f")
	r.PopFnBaseline()
	got := classesOf(r)
	want := []string{"overlap:f", "extend-inner:f", "shadow:f"}
	if len(got) != len(want) {
		t.Fatalf("census %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("census[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	if ov := r.Check.SortedDefCensus()[0]; ov.Note != "fn" || ov.Standing.Col != 1 {
		// The overlapped entry, wherever it sits in f's stack — here below
		// the disjoint String overload.
		t.Errorf("the overlap finding names the overlapped overload: %+v", ov)
	}
	// InstallFnDef itself is the registration path, not a def: no finding.
	before := len(r.Check.DefCensus)
	InstallFnDef(r, "g", sig(TInteger))
	InstallFnDef(r, "g", sig(TInteger))
	if len(r.Check.DefCensus) != before {
		t.Error("a direct registration is not the census's subject")
	}
}

func TestDefCensusLeakedBindings(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	site := SrcPos{Row: 1, Col: 16}
	r.Defs.PushLeaked("w", NewInteger(1), site)
	siteAt(r, 1, 30)
	InstallDef(r, "w", NewInteger(2)) // over a leaked binding: no finding
	if len(r.Check.DefCensus) != 0 {
		t.Fatalf("a def over the model's leaked binding is no finding: %v", classesOf(r))
	}
	r.Defs.Pop("w")
	r.noteAnalysisUseAt("w", SrcPos{Row: 1, Col: 29})
	r.noteAnalysisUseAt("w", SrcPos{Row: 1, Col: 29}) // once per site
	r.noteAnalysisUse("zz")                           // unbound: nothing
	ds := r.Check.SortedDefCensus()
	if len(ds) != 1 || ds[0].Class != CensusLeakRead || ds[0].Standing != site || ds[0].Site.Col != 29 {
		t.Errorf("the leak read: %+v", ds)
	}
}

func TestDefCensusTypesAndModules(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	siteAt(r, 1, 5)
	if err := InstallType(r, "Foo", NewTypeLiteral(TInteger)); err != nil { // an alias
		t.Fatal(err)
	}
	siteAt(r, 1, 21)
	if err := InstallType(r, "Foo", NewTypeLiteral(TString)); err != nil {
		t.Fatal(err)
	}
	shape := NewOrderedMap()
	shape.Set("a", NewTypeLiteral(TInteger))
	siteAt(r, 2, 5)
	if err := InstallType(r, "Rec", NewMap(shape)); err != nil { // minted
		t.Fatal(err)
	}
	siteAt(r, 2, 25)
	if err := InstallType(r, "Rec", NewMap(shape)); err != nil {
		t.Fatal(err)
	}
	ns := WithModuleNS(NewMap(NewOrderedMap()), "M", NewInteger(0))
	siteAt(r, 3, 1)
	InstallDef(r, "M", ns)
	siteAt(r, 3, 20)
	InstallDef(r, "M", ns)
	ds := r.Check.SortedDefCensus()
	if len(ds) != 3 {
		t.Fatalf("census %v", classesOf(r))
	}
	for i, want := range []struct{ name, note string }{{"Foo", "type"}, {"Rec", "type"}, {"M", "module"}} {
		if ds[i].Class != CensusRebind || ds[i].Name != want.name || ds[i].Note != want.note {
			t.Errorf("finding %d: %+v, want rebind %s (%s)", i, ds[i], want.name, want.note)
		}
	}
	if ds[0].Standing != (SrcPos{Row: 1, Col: 5}) {
		t.Errorf("the alias finding names the first alias: %+v", ds[0])
	}
}

func TestDefCensusUsesAndBookkeeping(t *testing.T) {
	NoteDefCensusUse(nil, CensusUndef, "a", SrcPos{})
	NoteDefCensusUse(&Registry{}, CensusUndef, "a", SrcPos{})
	var none *CheckState
	none.NoteDefCensus(DefCensusEntry{Class: CensusUndef})
	if none.SortedDefCensus() != nil {
		t.Error("a nil state has no census")
	}
	r := newTestRegistry(t)
	NoteDefCensusUse(r, CensusUndef, "a", SrcPos{Row: 1}) // not in check mode: nothing
	InstallDef(r, "a", NewInteger(1))
	InstallDef(r, "a", NewInteger(2))
	if len(r.Check.DefCensus) != 0 || r.Check.SortedDefCensus() != nil {
		t.Fatal("the interpreter's run is no census")
	}
	done := r.Check.Begin()
	defer done()
	r.Defs.EnterScope(ScopeBlock)
	NoteDefCensusUse(r, CensusUndef, "e", SrcPos{Row: 2, Col: 3})
	r.Defs.LeaveScope()
	NoteDefCensusUse(r, CensusUndef, "a", SrcPos{Row: 2, Col: 1})
	NoteDefCensusUse(r, CensusUndef, "a", SrcPos{Row: 2, Col: 1}) // deduped
	NoteDefCensusUse(r, CensusUndef, "b", SrcPos{Row: 2, Col: 1}) // another name at one site
	NoteDefCensusUse(r, CensusUndef, "c", SrcPos{Row: 1, Col: 9})
	// Inside a named fn body's analysis the finding names the fn.
	r.Check.FnNameStack = append(r.Check.FnNameStack, "outer", "inner")
	NoteDefCensusUse(r, CensusUndef, "d", SrcPos{Row: 4, Col: 1})
	r.Check.FnNameStack = nil
	if ds := r.Check.SortedDefCensus(); ds[len(ds)-1].Fn != "inner" || ds[0].Fn != "" {
		t.Errorf("the enclosing fn: %+v / %+v", ds[0], ds[len(ds)-1])
	}
	ds := r.Check.SortedDefCensus()
	got := make([]string, len(ds))
	for i, e := range ds {
		got[i] = string(e.Class) + ":" + e.Name
	}
	want := []string{"undef:c", "undef:a", "undef:b", "undef:e", "undef:d"}
	if len(got) != len(want) {
		t.Fatalf("census %v, want %v (sorted by site, then class)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("census[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	if ds[3].Scope != ScopeBlock {
		t.Errorf("the var construct inside a block: %+v", ds[3])
	}
}

// A branch join re-installs a name bound in one or both arms: with a
// pre-branch binding it is an ordinary push, without one a leaked binding.
func TestInstallJoinedDefsMarksLeaked(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("pre", NewInteger(0))
	both := map[string]Value{"pre": NewInteger(1), "fresh": NewInteger(3), "thenOnly": NewInteger(5)}
	other := map[string]Value{"pre": NewInteger(2), "fresh": NewInteger(4), "elseOnly": NewInteger(6)}
	installJoinedDefs(r, both, other, false, armsUndecided)
	for name, leaked := range map[string]bool{"pre": false, "fresh": true, "thenOnly": true, "elseOnly": true} {
		e, ok := r.Defs.TopEntry(name)
		if !ok || e.Leaked != leaked {
			t.Errorf("%s: bound %v, leaked %v, want leaked %v", name, ok, e.Leaked, leaked)
		}
	}
}

// A rolled-back body run is a block scope; the scope is closed afterwards
// whatever the body did.
func TestRunCarrierArmBodyIsABlockScope(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	before := r.Defs.nextScope
	RunCarrierArmBody(r, NewList([]Value{NewInteger(1)}))
	if r.Defs.nextScope != before+1 || r.Defs.ScopeID() != 0 {
		t.Errorf("one block scope opened and closed: next %d (was %d), current %d", r.Defs.nextScope, before, r.Defs.ScopeID())
	}
	// A kept body (do) and a condition fragment open none.
	RunCarrierBodyKeepDefs(r, NewList([]Value{NewInteger(1)}))
	RunCarrierCondBody(r, NewList([]Value{NewInteger(1)}))
	if r.Defs.nextScope != before+1 {
		t.Error("a kept or condition body is not a block")
	}
}

// A caller's bindings are visible to a callee's frame but enclose nothing
// of it: the rule binds lexically, so a callee's def of a name its caller
// bound — a module whose fns each keep a local `loop` and call each other —
// is the callee's own, and no finding. The module's bindings enclose every
// frame; a frame's bindings enclose its own blocks.
func TestDefCensusLexicalEnclosure(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	loop := func() Value {
		return NewFunction(FnDefInfo{Signatures: []Signature{{
			Params: []FnParam{{Name: "i", Type: TInteger}}, Returns: []*Type{TInteger},
			Impl: Boru([]Value{NewInteger(1)}), BarrierPos: BarrierAllForward,
		}}})
	}
	siteAt(r, 1, 1)
	InstallDef(r, "m", NewInteger(1)) // module bindings
	InstallDef(r, "mf", loop())
	r.PushFnBaseline(nil) // the caller's frame
	siteAt(r, 2, 1)
	InstallDef(r, "loop", loop())
	InstallDef(r, "t", NewInteger(2))
	r.Defs.EnterScope(ScopeBlock) // a block of the caller's
	InstallDef(r, "b", NewInteger(3))
	r.PushFnBaseline(nil) // the callee's frame, entered from the block
	siteAt(r, 3, 1)
	InstallDef(r, "loop", loop())     // over the caller's local fn: no finding
	InstallDef(r, "t", NewInteger(4)) // over the caller's local value: no finding
	InstallDef(r, "b", NewInteger(5)) // over the caller's block binding: no finding
	if got := classesOf(r); len(got) != 0 {
		t.Fatalf("a caller's bindings enclose nothing: %v", got)
	}
	siteAt(r, 3, 10)
	InstallDef(r, "m", NewInteger(6)) // over the module's value: a shadow
	siteAt(r, 3, 20)
	InstallDef(r, "mf", loop())   // over the module's fn: extend-inner
	r.Defs.EnterScope(ScopeBlock) // the callee's own block
	siteAt(r, 3, 30)
	InstallDef(r, "t", NewInteger(7)) // over the callee's t: a shadow
	r.Defs.LeaveScope()
	got := classesOf(r)
	want := []string{"shadow:m", "extend-inner:mf", "shadow:t"}
	if len(got) != len(want) {
		t.Fatalf("census %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("census[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	// LexicallyEncloses itself: the module always; the callee's frame; not
	// the caller's frame or block; not a closed scope; a nil table says yes.
	callee := r.Defs.ScopeID()
	if !r.Defs.LexicallyEncloses(0) || !r.Defs.LexicallyEncloses(callee) {
		t.Error("the module and the frame itself enclose the frame")
	}
	if r.Defs.LexicallyEncloses(callee-1) || r.Defs.LexicallyEncloses(callee-2) {
		t.Error("the caller's block and frame do not enclose the callee")
	}
	if r.Defs.LexicallyEncloses(callee + 5) {
		t.Error("a closed or unknown scope encloses nothing")
	}
	var nilTable *DefTable
	if !nilTable.LexicallyEncloses(7) {
		t.Error("a nil table encloses")
	}
	// A closed block's id asked from a block with no frame between it and
	// the module: the walk runs out of scopes.
	r.PopFnBaseline()
	r.Defs.LeaveScope()
	r.PopFnBaseline()
	closed := r.Defs.EnterScope(ScopeBlock)
	r.Defs.LeaveScope()
	r.Defs.EnterScope(ScopeBlock)
	if r.Defs.LexicallyEncloses(closed) {
		t.Error("a closed block encloses nothing")
	}
	r.Defs.LeaveScope()
}

// A multi-run body's kept defs are the leak they model (MarkScopeLeaked): a
// read of one with nothing live under it is a leak-read; with a live
// binding under it the read is the rule's, and the block def's own shadow
// finding has said what changes. A frame binding (a param, a capture) is
// noted as such when a body def rebinds it.
func TestDefCensusKeptBodyDefsAndFrameBindings(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	block := r.Defs.EnterScope(ScopeBlock)
	siteAt(r, 1, 9)
	InstallDef(r, "y", NewInteger(9))
	siteAt(r, 1, 20)
	InstallDef(r, "z", NewInteger(1))
	r.Defs.LeaveScope()
	r.Defs.MarkScopeLeaked(block)
	if e, _ := r.Defs.TopEntry("y"); !e.Leaked {
		t.Fatalf("the kept def is marked leaked: %+v", e)
	}
	r.noteAnalysisUseAt("y", SrcPos{Row: 1, Col: 30}) // nothing live under y: a leak-read
	siteAt(r, 2, 1)
	InstallDef(r, "y", NewInteger(2)) // over the leaked entry only: no finding
	r.Defs.EnterScope(ScopeBlock)
	siteAt(r, 2, 10)
	InstallDef(r, "q", NewInteger(3))
	r.Defs.LeaveScope()
	// q stands under a kept binding of the same name the next run leaves.
	r.Defs.MarkScopeLeaked(r.Defs.ScopeID() + 7) // no such scope: nothing changes
	siteAt(r, 3, 1)
	InstallDef(r, "m", NewInteger(0))
	block = r.Defs.EnterScope(ScopeBlock)
	siteAt(r, 3, 10)
	InstallDef(r, "m", NewInteger(1)) // a shadow of the module's m
	r.Defs.LeaveScope()
	r.Defs.MarkScopeLeaked(block)
	r.noteAnalysisUseAt("m", SrcPos{Row: 3, Col: 20}) // the module's m lives under it: no leak-read
	// A frame binding rebound by a body def is noted "param".
	r.PushFnBaseline(nil)
	InstallFrameBinding(r, "n", NewInteger(5))
	if e, _ := r.Defs.TopEntry("n"); !e.Frame {
		t.Fatalf("a frame binding is marked: %+v", e)
	}
	siteAt(r, 4, 1)
	InstallDef(r, "n", NewInteger(6))
	r.Defs.Pop("n")
	UninstallFrameBinding(r, "n")
	r.PopFnBaseline()
	got := classesOf(r)
	want := []string{"leak-read:y", "shadow:m", "rebind:n"}
	if len(got) != len(want) {
		t.Fatalf("census %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("census[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	if ds := r.Check.SortedDefCensus(); ds[2].Note != "param" || ds[0].Standing.Col != 9 {
		t.Errorf("the notes: %+v", ds)
	}
	var nilTable *DefTable
	nilTable.MarkTopFrame("x") // nil-safe, as every marker
	nilTable.MarkScopeLeaked(1)
	r.Defs.MarkTopFrame("nobody") // no such name: nothing
}

// A var binding (the var word, design/IMMUTABLE-DEF.1.md §2.3): declared
// in the current scope with its mark, assigned in place within its frame
// — a replace the ledger notes as one — and never from another frame.
func TestInstallAndAssignVar(t *testing.T) {
	r := newTestRegistry(t)
	InstallVar(r, "n", NewInteger(0), nil)
	e, ok := r.Defs.TopEntry("n")
	if !ok || !e.Var || e.VarType != nil || e.Scope != 0 {
		t.Fatalf("the declared var: %+v", e)
	}
	InstallVar(r, "s", NewString("a"), TString)
	if e, _ := r.Defs.TopEntry("s"); !e.Var || e.VarType != TString {
		t.Fatalf("the typed var: %+v", e)
	}
	// Assignment keeps the depth and the marks, bumps the generation.
	gen := r.Defs.Gen("n")
	AssignVar(r, "n", NewInteger(5), SrcPos{Row: 1, Col: 9})
	if v, _ := r.Defs.Top("n"); v.String() != "5" || r.Defs.Depth("n") != 1 || r.Defs.Gen("n") == gen {
		t.Errorf("assigned: %v depth %d gen %d (was %d)", v, r.Defs.Depth("n"), r.Defs.Gen("n"), gen)
	}
	if e, _ := r.Defs.TopEntry("n"); !e.Var {
		t.Errorf("the mark survives the assignment: %+v", e)
	}
	// Within the frame: the module scope with no frame open, a block of
	// it; a frame's own scope and its blocks; not the module from a frame.
	if !r.Defs.InCurrentFrame(0) {
		t.Error("the module scope is the current frame at the top level")
	}
	block := r.Defs.EnterScope(ScopeBlock)
	if !r.Defs.InCurrentFrame(block) || !r.Defs.InCurrentFrame(0) {
		t.Error("a module block and the module are within the top-level frame")
	}
	r.PushFnBaseline(nil)
	frame := r.Defs.ScopeID()
	inner := r.Defs.EnterScope(ScopeBlock)
	if !r.Defs.InCurrentFrame(inner) || !r.Defs.InCurrentFrame(frame) {
		t.Error("a frame and its block are the current frame")
	}
	if r.Defs.InCurrentFrame(0) || r.Defs.InCurrentFrame(block) {
		t.Error("the module scope and its block are not the frame's")
	}
	r.Defs.LeaveScope()
	r.PopFnBaseline()
	r.Defs.LeaveScope()
	var nilTable *DefTable
	if !nilTable.InCurrentFrame(0) || nilTable.InCurrentFrame(3) {
		t.Error("a nil table is the module scope")
	}
	nilTable.MarkTopVar("x", nil)
	r.Defs.MarkTopVar("nobody", nil) // no such name: nothing
	if _, ok := r.Defs.TopEntry("nobody"); ok {
		t.Error("marking binds nothing")
	}
}

// The check model of a var cell: an assignment the body under analysis
// makes is recorded (CheckState.VarAssigned) so the body runner counts it
// among the body's bindings — a cell replaced in place moves no depth — and
// a rolled-back body's cell is restored to its pre-body value where a kept
// body's assignment stands; a join over a var replaces the cell.
func TestVarCellCheckModel(t *testing.T) {
	r := newTestRegistry(t)
	// A scratch word that assigns the var `n` to 7 when run.
	r.RegisterNativeFunc(NativeFunc{
		Name: "assign-n",
		Signatures: []Signature{{
			Args: []*Type{},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				AssignVar(reg, "n", NewInteger(7), SrcPos{Row: 1, Col: 9})
				return nil, nil
			}, RunInCheck()),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	InstallVar(r, "n", NewInteger(0), nil)
	// Outside a body run nothing is recorded (VarAssigned is nil).
	AssignVar(r, "n", NewInteger(1), SrcPos{})
	if r.Check.VarAssigned != nil {
		t.Fatalf("no body run: %v", r.Check.VarAssigned)
	}
	done := r.Check.Begin()
	defer done()
	// A rolled-back body (an arm): the assignment is a binding of the body,
	// and the cell is restored after it.
	body := NewList([]Value{NewWord("assign-n")})
	_, adds := RunCarrierBodyWithDefs(r, body)
	if v, ok := adds["n"]; !ok || v.String() != "7" {
		t.Errorf("the arm's assignment is among its bindings: %v", adds)
	}
	if v, _ := r.Defs.Top("n"); v.String() != "1" || r.Defs.Depth("n") != 1 {
		t.Errorf("the rolled-back cell is restored: %v depth %d", v, r.Defs.Depth("n"))
	}
	if r.Check.VarAssigned != nil {
		t.Errorf("the record is the body run's own: %v", r.Check.VarAssigned)
	}
	// A kept body (do): the assignment stands.
	RunCarrierBodyKeepDefs(r, body)
	if v, _ := r.Defs.Top("n"); v.String() != "7" {
		t.Errorf("a kept body's assignment stands: %v", v)
	}
	// A nested body's assignment is the enclosing body's too, with the
	// enclosing body's pre value where it assigned first.
	r.Check.VarAssigned = map[string]Value{}
	AssignVar(r, "n", NewInteger(2), SrcPos{}) // the outer body assigns: pre 7
	AssignVar(r, "n", NewInteger(3), SrcPos{}) // again: pre stays 7
	RunCarrierBodyWithDefs(r, body)            // the inner body assigns 7, restores to 3
	if pre := r.Check.VarAssigned["n"]; pre.String() != "7" {
		t.Errorf("the outer record keeps its own pre: %v", r.Check.VarAssigned)
	}
	if v, _ := r.Defs.Top("n"); v.String() != "3" {
		t.Errorf("the inner body restored to the value it found: %v", v)
	}
	r.Check.VarAssigned = map[string]Value{}
	RunCarrierBodyWithDefs(r, body) // the outer body had not assigned n: the inner's pre is merged
	if pre := r.Check.VarAssigned["n"]; pre.String() != "3" {
		t.Errorf("the inner pre is merged into the outer record: %v", r.Check.VarAssigned)
	}
	r.Check.VarAssigned = nil
	// A join over a var replaces the cell; over a def it pushes.
	InstallDef(r, "d", NewInteger(1))
	pushJoinedBinding(r, "n", NewInteger(9))
	pushJoinedBinding(r, "d", NewInteger(9))
	if r.Defs.Depth("n") != 1 || r.Defs.Depth("d") != 2 {
		t.Errorf("joins: n depth %d (cell), d depth %d (pushed)", r.Defs.Depth("n"), r.Defs.Depth("d"))
	}
	if e, isVar := IsVarBinding(r, "n"); !isVar || e.Body.String() != "9" {
		t.Errorf("the cell holds the join and stays a var: %+v", e)
	}
	if _, isVar := IsVarBinding(r, "d"); isVar {
		t.Error("a def is no var")
	}
	if _, isVar := IsVarBinding(r, "nobody"); isVar {
		t.Error("an unbound name is no var")
	}
	// The resident assign arm: replaces a var's cell; installs when the
	// name is no var (a cell torn down with its frame).
	ApplyResidentAssign(r, "n", NewInteger(11))
	if v, _ := r.Defs.Top("n"); v.String() != "11" || r.Defs.Depth("n") != 1 {
		t.Errorf("resident assign replaces: %v depth %d", v, r.Defs.Depth("n"))
	}
	ApplyResidentAssign(r, "fresh", NewInteger(12))
	if e, isVar := IsVarBinding(r, "fresh"); !isVar || e.Body.String() != "12" {
		t.Errorf("resident assign of an unbound name declares: %+v", e)
	}
	ApplyResidentAssign(nil, "n", NewInteger(0)) // nil-safe
}

// A fn baseline opens a frame scope — or, for the one push the compiler
// marks (a token body's closure compile), a block of the enclosing frame,
// so a var the body assigns is the frame's own. The mark is consumed by
// that push alone.
func TestFnBaselineBlockMode(t *testing.T) {
	r := newTestRegistry(t)
	InstallVar(r, "n", NewInteger(0), nil)
	r.Check.NextBaselineIsBlock = true
	r.PushFnBaseline(nil)
	if r.Defs.ScopeKindNow() != ScopeBlock || r.Check.NextBaselineIsBlock {
		t.Fatalf("the marked push opens a block and consumes the mark: %v %v", r.Defs.ScopeKindNow(), r.Check.NextBaselineIsBlock)
	}
	if !r.Defs.InCurrentFrame(0) {
		t.Error("the module var is the frame's own inside a token body")
	}
	r.PushFnBaseline(nil) // the next push is a frame again
	if r.Defs.ScopeKindNow() != ScopeFrame || r.Defs.InCurrentFrame(0) {
		t.Errorf("an unmarked push is a frame: %v", r.Defs.ScopeKindNow())
	}
	r.PopFnBaseline()
	r.PopFnBaseline()
}

// A replace twin over a var (AssignVar's ledger note) replaces the cell with
// the captured concrete value when it is not written back; a written-back
// or computed one does nothing — the unit's own assign op or the root's
// write-back at depth holds the runtime value — and pops nothing, since a
// var's assignment pushed nothing. A fn's overlap replace keeps its
// drop-then-push.
func TestApplyBindTwinVarReplace(t *testing.T) {
	r := newTestRegistry(t)
	InstallVar(r, "n", NewInteger(0), nil)
	entry, _ := r.Defs.TopEntry("n")
	entry.Body = NewInteger(5)
	tr := BindTransition{Kind: BindDefReplace, Name: "n", Depth: 1}
	if err := ApplyBindTwin(r, tr, entry); err != nil {
		t.Fatal(err)
	}
	if v, _ := r.Defs.Top("n"); v.String() != "5" || r.Defs.Depth("n") != 1 {
		t.Errorf("the twin replaced the cell: %v depth %d", v, r.Defs.Depth("n"))
	}
	tr.WrittenBack = true
	entry.Body = NewInteger(9)
	if err := ApplyBindTwin(r, tr, entry); err != nil {
		t.Fatal(err)
	}
	if v, _ := r.Defs.Top("n"); v.String() != "5" || r.Defs.Depth("n") != 1 {
		t.Errorf("a written-back twin does nothing: %v depth %d", v, r.Defs.Depth("n"))
	}
	tr.WrittenBack = false
	entry.Body = NewCarrier(TInteger)
	if err := ApplyBindTwin(r, tr, entry); err != nil {
		t.Fatal(err)
	}
	if v, _ := r.Defs.Top("n"); v.String() != "5" || r.Defs.Depth("n") != 1 {
		t.Errorf("a computed value's twin does nothing: %v depth %d", v, r.Defs.Depth("n"))
	}
}
