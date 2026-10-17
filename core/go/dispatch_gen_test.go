package core

import "testing"

// The dispatch generation (DefTable.DispatchGen) is Lookup's cache key: it
// moves when a fn-definition binding is added, removed or rewritten, and
// stays put for a value binding, which Lookup never reads.

func dgFn() Value {
	return NewFunction(FnDefInfo{Signatures: []Signature{{Impl: Boru([]Value{NewInteger(1)})}}})
}

// dgMoves reports whether op moved name's dispatch generation, and checks
// that it moved the plain generation either way.
func dgMoves(t *testing.T, dt *DefTable, name, what string, op func()) bool {
	t.Helper()
	gen, dg := dt.Gen(name), dt.DispatchGen(name)
	op()
	if dt.Gen(name) == gen {
		t.Errorf("%s: the generation did not move", what)
	}
	return dt.DispatchGen(name) != dg
}

func TestDispatchGenMovesOnlyForFnBindings(t *testing.T) {
	dt := NewDefTable()
	fn, other := dgFn(), dgFn()
	moves := func(what string, want bool, op func()) {
		t.Helper()
		if got := dgMoves(t, dt, "x", what, op); got != want {
			t.Errorf("%s: dispatch generation moved = %v, want %v", what, got, want)
		}
	}
	moves("push a value", false, func() { dt.Push("x", NewInteger(1)) })
	moves("push a fn", true, func() { dt.Push("x", fn) })
	moves("push a value over the fn", false, func() { dt.Push("x", NewInteger(2)) })
	moves("pop the value", false, func() { dt.Pop("x") })
	moves("replace the fn by a fn", true, func() { dt.Replace("x", other) })
	moves("replace the fn by a value", true, func() { dt.Replace("x", NewInteger(3)) })
	moves("replace the value by a value", false, func() { dt.Replace("x", NewInteger(4)) })
	moves("replace the value by a fn", true, func() { dt.Replace("x", fn) })
	moves("swap the fn's payload", true, func() { dt.SetTopData("x", IntPayload{N: 5}) })
	moves("swap a value's payload", false, func() { dt.SetTopData("x", IntPayload{N: 6}) })
	moves("swap a value's payload for a fn", true, func() { dt.SetTopData("x", fn.Data()) })
	moves("pop the fn", true, func() { dt.Pop("x") })
	moves("push a fn type binding", true, func() { dt.PushType("x", TInteger, fn) })
	moves("push an adopted fn type binding", true, func() { dt.PushTypeAdopted("x", TInteger, fn) })
	moves("push a type binding", false, func() { dt.PushType("x", TInteger, NewTypeLiteral(TInteger)) })
	moves("push an adopted type binding", false, func() { dt.PushTypeAdopted("x", TInteger, NewTypeLiteral(TInteger)) })
	depth := dt.Depth("x")
	dt.Push("x", NewInteger(7))
	moves("truncate values off", false, func() { dt.Truncate("x", depth) })
	moves("truncate a fn off", true, func() { dt.Truncate("x", depth-3) })
	moves("delete a stack holding a fn", true, func() { dt.Delete("x") })

	dt.Push("y", NewInteger(1))
	if dgMoves(t, dt, "y", "delete a value-only stack", func() { dt.Delete("y") }) {
		t.Error("deleting a value-only stack moved the dispatch generation")
	}
	// The wholesale writers move it unconditionally.
	if !dgMoves(t, dt, "z", "set", func() { dt.Set("z", []Value{NewInteger(1)}) }) {
		t.Error("Set left the dispatch generation")
	}
	snap := dt.Snapshot()
	dt.Push("w", NewInteger(1))
	if !dgMoves(t, dt, "w", "restore a depth snapshot", func() { dt.Restore(snap) }) {
		t.Error("restoring a depth snapshot past a name left its dispatch generation")
	}
	entries := dt.SnapshotEntries()
	dt.Push("z", NewInteger(2))
	dt.Push("v", NewInteger(1))
	if !dgMoves(t, dt, "z", "restore an entries snapshot", func() { dt.RestoreEntriesSnapshot(entries) }) {
		t.Error("restoring an entries snapshot left a restored name's dispatch generation")
	}
	if dt.Has("v") || dt.DispatchGen("v") == 0 {
		t.Error("restoring an entries snapshot must drop a name bound since, moving its dispatch generation")
	}
	// A clone carries the timeline on.
	if c := dt.Clone(); c.DispatchGen("x") != dt.DispatchGen("x") || c.DispatchGen("x") == 0 {
		t.Errorf("clone dispatch generation %d, want the parent's %d", c.DispatchGen("x"), dt.DispatchGen("x"))
	}
	var none *DefTable
	if none.DispatchGen("x") != 0 || none.entriesRef("x") != nil {
		t.Error("a nil table has no dispatch generation and no entries")
	}
}

// TestLookupNegativeCacheSurvivesValueRedefinition is the loop the
// dispatch generation exists for: a value name redefined every pass stacks
// a level per `def`, and Lookup's answer for it — no fn — must stay cached
// rather than re-walk the growing stack, while a fn bound under it after
// all still shows.
func TestLookupNegativeCacheSurvivesValueRedefinition(t *testing.T) {
	r := w8reg(t)
	r.Defs.Push("acc", NewInteger(0))
	if r.Lookup("acc") != nil {
		t.Fatal("a value name looked up as a fn")
	}
	dg := r.Defs.DispatchGen("acc")
	for i := 1; i <= 50; i++ {
		r.Defs.Push("acc", NewInteger(int64(i)))
		if r.Lookup("acc") != nil {
			t.Fatalf("pass %d: a value name looked up as a fn", i)
		}
	}
	if r.Defs.DispatchGen("acc") != dg || r.Defs.Depth("acc") != 51 {
		t.Errorf("redefining a value moved the dispatch generation (%d → %d) or lost a level (depth %d)",
			dg, r.Defs.DispatchGen("acc"), r.Defs.Depth("acc"))
	}
	// A fn bound under the name invalidates the answer, and one beneath a
	// value binding still aggregates, as it always did.
	r.Defs.Push("acc", dgFn())
	r.Defs.Push("acc", NewInteger(99))
	if r.Lookup("acc") == nil {
		t.Error("a fn binding beneath a value binding dropped out of the aggregate")
	}
}

// TestAnonymousAggregateFallbackIsStamped: a reference to an anonymous fn's
// name wraps the cached aggregate as it stands — every signature, the
// synthetic fallback included, already carries the anonymity NewFunction
// stamps — so two references share one signature list.
func TestAnonymousAggregateFallbackIsStamped(t *testing.T) {
	r := w8reg(t)
	InstallFnDef(r, "anonagg", FnDefInfo{
		Anonymous:  true,
		Signatures: []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru([]Value{NewWord("n")})}},
	})
	agg := r.Lookup("anonagg")
	if agg == nil || len(agg.Signatures) < 2 {
		t.Fatalf("expected a boru fn's aggregate with its fallback, got %+v", agg)
	}
	for i := range agg.Signatures {
		if !agg.Signatures[i].Anonymous {
			t.Errorf("aggregate signature %d (fallback %v) is not stamped anonymous", i, agg.Signatures[i].Fallback)
		}
	}
	a, _ := ResolveRef(r, "anonagg")
	b, _ := ResolveRef(r, "anonagg")
	fa, _ := a.Data().(*FnDefInfo)
	fb, _ := b.Data().(*FnDefInfo)
	if fa == nil || fb == nil || !sameList(fa.Signatures, fb.Signatures) || !sameList(fa.Signatures, agg.Signatures) {
		t.Error("two references to one anonymous fn do not share the aggregate's signature list")
	}
	// A named fn's fallback stays unstamped.
	InstallFnDef(r, "namedagg", FnDefInfo{
		Signatures: []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru([]Value{NewWord("n")})}},
	})
	for _, s := range r.Lookup("namedagg").Signatures {
		if s.Anonymous {
			t.Error("a named fn's aggregate signature is stamped anonymous")
		}
	}
}

// TestInstalledFnSigsReusedPerIdentity pins the install cache: binding one
// fn value under one name in one registry reuses the first install's
// signatures, and anything the form is built from — the name, the barrier
// mode, the registry, the authored or capture list — compiles afresh, as
// does an analysis pass and a payload with no identity.
func TestInstalledFnSigsReusedPerIdentity(t *testing.T) {
	r := w8reg(t)
	v := NewFunction(FnDefInfo{
		Signatures: []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru([]Value{NewWord("n")})}},
	})
	fd, _ := v.Data().(*FnDefInfo)
	first := installedFnSigsFor(r, "p", fd, false)
	if again := installedFnSigsFor(r, "p", fd, false); !sameList(first, again) {
		t.Fatal("a second install of one fn under one name recompiled")
	}
	fresh := func(what string, got []Signature) {
		t.Helper()
		if sameList(got, first) {
			t.Errorf("%s: reused the cached install form", what)
		}
	}
	fresh("another name", installedFnSigsFor(r, "q", fd, false))
	first = installedFnSigsFor(r, "p", fd, false)
	fresh("stack-only", installedFnSigsFor(r, "p", fd, true))
	first = installedFnSigsFor(r, "p", fd, false)
	fresh("another registry", installedFnSigsFor(w8reg(t), "p", fd, false))
	first = installedFnSigsFor(r, "p", fd, false)
	homed := *fd
	homed.Registry = w8reg(t)
	fresh("another home", installedFnSigsFor(r, "p", &homed, false))
	first = installedFnSigsFor(r, "p", fd, false)
	relisted := *fd
	relisted.Signatures = append([]Signature(nil), fd.Signatures...)
	fresh("a rebuilt signature list", installedFnSigsFor(r, "p", &relisted, false))
	first = installedFnSigsFor(r, "p", fd, false)
	captured := *fd
	captured.Captured = []CapturedBinding{{Name: "c", Value: NewInteger(1)}}
	fresh("another capture list", installedFnSigsFor(r, "p", &captured, false))
	first = installedFnSigsFor(r, "p", fd, false)
	r.Check.Mode = true
	fresh("an analysis pass", installedFnSigsFor(r, "p", fd, false))
	r.Check.Mode = false
	bare := FnDefInfo{Signatures: fd.Signatures}
	if a, b := installedFnSigsFor(r, "p", &bare, false), installedFnSigsFor(r, "p", &bare, false); sameList(a, b) {
		t.Error("a payload with no identity reused an install form")
	}
}
