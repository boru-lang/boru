package core

import "testing"

// The def table is the sandbox's whole point: a binding the check pass pushed
// after the capture is gone on restore, and one it POPPED comes back. Both
// directions matter — the twins replay a transition, so the rollback has to
// undo it whichever way it went.
func TestRestoreBindingsRollsBackDefs(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("keep", NewInteger(1))
	r.Defs.Push("popped", NewInteger(2))

	snap := r.SnapshotBindings()

	r.Defs.Push("added", NewInteger(3))
	r.Defs.Push("keep", NewInteger(99)) // shadow the captured binding
	r.Defs.Pop("popped")

	r.RestoreBindings(snap)

	if _, ok := r.Defs.Top("added"); ok {
		t.Error("a binding pushed after the capture survived the rollback")
	}
	if v, ok := r.Defs.Top("keep"); !ok || v.String() != "1" {
		t.Errorf("shadowed binding restored as %v/%v, want 1", v, ok)
	}
	if v, ok := r.Defs.Top("popped"); !ok || v.String() != "2" {
		t.Errorf("popped binding restored as %v/%v, want 2", v, ok)
	}
}

// THE PARTITION, half one: a type MINTED after the capture is a compile-time
// product and must SURVIVE. This is the half SnapshotForCompile gets wrong for
// this purpose — it restores r.Types wholesale, so the mint disappears and
// OpPushType is left resolving an ID that no longer exists.
func TestRestoreBindingsKeepsMintedTypes(t *testing.T) {
	r := newTestRegistry(t)
	snap := r.SnapshotBindings()

	minted := r.Types.MintType("Minted", TInteger)
	if minted == nil || IDOf(minted) == "" {
		t.Fatal("MintType produced no identity")
	}

	r.RestoreBindings(snap)

	if got := r.Types.LookupByID(IDOf(minted)); got != minted {
		t.Errorf("minted type %s did not survive the rollback (got %v) — a compile-time product was replayed away", IDOf(minted), got)
	}
}

// THE PARTITION, half two, and the hazard §6.5 names outright: restore r.Defs
// alone and a capitalised `undef`'s RETIREMENT stays applied, so the restored
// type BINDING points at a node the lattice no longer holds — a live binding
// on a dead ID, in a registry the VM has not reached the twin for.
func TestRestoreBindingsUndoesTypeRetirement(t *testing.T) {
	r := newTestRegistry(t)
	def := r.Types.MintType("Retired", TInteger)
	r.Defs.PushType("Retired", def, NewTypeLiteral(TInteger))

	snap := r.SnapshotBindings()

	// What a capitalised `undef` does: pop the binding, then retire the node
	// this binding minted.
	r.Defs.Pop("Retired")
	r.Types.Retire(def)
	if r.Types.LookupByID(IDOf(def)) != nil {
		t.Fatal("Retire did not remove the node — the test is measuring the wrong thing")
	}

	r.RestoreBindings(snap)

	if !r.Defs.IsType("Retired") {
		t.Fatal("the type binding did not come back")
	}
	if got := r.Types.LookupByID(IDOf(def)); got != def {
		t.Errorf("binding restored but its lattice node is still retired (got %v) — a live binding on a dead ID", got)
	}
}

// The module ledger rolls back so a twin can re-bind the already-produced
// instance: a module executes ONCE, in the front end, and the twin re-installs
// that same instance at its source position.
func TestRestoreBindingsRollsBackModuleLedger(t *testing.T) {
	r := newTestRegistry(t)
	if r.Modules == nil {
		t.Skip("registry has no module ledger")
	}
	r.Modules.Loaded["before"] = ModuleDesc{Ref: "before"}
	r.Modules.seq = 7

	snap := r.SnapshotBindings()

	r.Modules.Loaded["after"] = ModuleDesc{Ref: "after"}
	r.Modules.seq = 9

	r.RestoreBindings(snap)

	if _, ok := r.Modules.Loaded["after"]; ok {
		t.Error("a module loaded after the capture survived the rollback")
	}
	if _, ok := r.Modules.Loaded["before"]; !ok {
		t.Error("a module loaded before the capture was dropped")
	}
	if r.Modules.seq != 7 {
		t.Errorf("module seq = %d, want 7", r.Modules.seq)
	}
}

// The zero value must never read as an empty-but-usable snapshot: restoring one
// would install a nil DefTable and take the registry's bindings with it. Nil
// receivers are inert for the same reason every other registry entry point is.
func TestBindingSandboxZeroValueAndNilSafety(t *testing.T) {
	r := newTestRegistry(t)
	r.Defs.Push("x", NewInteger(1))
	r.RestoreBindings(BindingSandbox{})
	if _, ok := r.Defs.Top("x"); !ok {
		t.Error("restoring the zero sandbox wiped the def table")
	}

	var nilReg *Registry
	if s := nilReg.SnapshotBindings(); s.valid {
		t.Error("a nil registry produced a valid snapshot")
	}
	nilReg.RestoreBindings(BindingSandbox{valid: true}) // must not panic

	// A registry with no module ledger snapshots and restores the rest.
	bare := newTestRegistry(t)
	bare.Modules = nil
	bare.Defs.Push("y", NewInteger(2))
	s := bare.SnapshotBindings()
	bare.Defs.Push("z", NewInteger(3))
	bare.RestoreBindings(s)
	if _, ok := bare.Defs.Top("z"); ok {
		t.Error("ledger-less registry did not roll its defs back")
	}
}

// The id-map helpers are the partition itself, so they are pinned directly as
// well as through the sandbox: a nil table answers nothing rather than panicking,
// and re-admission reports how many it undid so a caller can assert the
// partition rather than trust it.
func TestTypeIDSnapshotAndReadmit(t *testing.T) {
	var nilTT *TypeTable
	if got := nilTT.idSnapshot(); got != nil {
		t.Errorf("nil idSnapshot = %v, want nil", got)
	}
	if got := nilTT.readmitRetired(map[string]*Type{"a": TInteger}); got != 0 {
		t.Errorf("nil readmitRetired = %d, want 0", got)
	}

	r := newTestRegistry(t)
	a := r.Types.MintType("A", TInteger)
	snap := r.Types.idSnapshot()
	b := r.Types.MintType("B", TInteger)
	r.Types.Retire(a)

	if n := r.Types.readmitRetired(snap); n != 1 {
		t.Errorf("re-admitted %d ids, want exactly 1 (the retired one)", n)
	}
	if r.Types.LookupByID(IDOf(a)) != a {
		t.Error("the retired id was not re-admitted")
	}
	if r.Types.LookupByID(IDOf(b)) != b {
		t.Error("re-admission dropped an id minted after the capture")
	}
	if n := r.Types.readmitRetired(snap); n != 0 {
		t.Errorf("a second re-admission moved %d ids — it must be idempotent", n)
	}
}
