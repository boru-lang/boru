package core

import "testing"

// ApplyResidentBind's two arms against the interpreter's own semantics
// (the parity contract the arm-resident twins carry): the install arm
// goes through InstallDef so per-element repeats STACK — a plain value
// pushes a fresh level each call, never a replace — and the undef arm
// pops the live entry, retiring a minted node only when the popped
// binding minted it. A nil registry is a no-op.
func TestApplyResidentBind(t *testing.T) {
	ApplyResidentBind(nil, "x", false, NewInteger(1)) // must not panic

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Install arm: two calls stack two levels with their own values —
	// the measured interpreter leak shape (last element's install on top).
	ApplyResidentBind(r, "x", false, NewInteger(10))
	ApplyResidentBind(r, "x", false, NewInteger(20))
	if d := r.Defs.Depth("x"); d != 2 {
		t.Fatalf("two resident installs must stack: depth = %d, want 2", d)
	}
	if v, ok := r.Defs.Top("x"); !ok || v.String() != "20" {
		t.Fatalf("top after two installs = %v, want the second value 20", v)
	}

	// Undef arm: pops exactly one level.
	ApplyResidentBind(r, "x", true, Value{})
	if v, ok := r.Defs.Top("x"); !ok || v.String() != "10" {
		t.Fatalf("top after one resident undef = %v, want the first value 10", v)
	}
	ApplyResidentBind(r, "x", true, Value{})
	if _, ok := r.Defs.Top("x"); ok {
		t.Fatal("the second resident undef must drain the stack")
	}
	ApplyResidentBind(r, "x", true, Value{}) // empty pop: silent no-op, like undef

	// Undef arm retires a minted node — the arms must not drift from
	// ApplyBindTwin's BindUndef even though a var param never mints.
	minted := r.Types.MintType("Rz", TInteger)
	r.Defs.PushType("Rz", minted, NewTypeLiteral(TInteger))
	ApplyResidentBind(r, "Rz", true, Value{})
	if r.Types.LookupByID(minted.ID) != nil {
		t.Fatal("popping a minted type binding must retire its node")
	}
}

// ApplyResidentTypeBind's contract: each call MINTS ITS OWN node from the
// captured body, so N elements leave N independently retirable levels — the
// interpreter's shape, and the one a replay of the captured entry gets
// wrong (one shared node, so the first undef retires the level below it).
// A nil registry is a no-op; an installer compile failure comes back rather than
// being swallowed.
func TestApplyResidentTypeBind(t *testing.T) {
	if err := ApplyResidentTypeBind(nil, "Tz", DefEntry{}); err != nil {
		t.Fatalf("nil registry: %v", err)
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// The captured entry as the check pass leaves it: a node it minted and
	// the BODY that produced it. Only the body is re-installed.
	captured := r.Types.MintType("Tz", TInteger)
	entry := DefEntry{TypeDef: captured, Body: NewInteger(5), Minted: true}

	for i := 0; i < 2; i++ {
		if err := ApplyResidentTypeBind(r, "Tz", entry); err != nil {
			t.Fatalf("element %d: %v", i, err)
		}
	}
	if d := r.Defs.Depth("Tz"); d != 2 {
		t.Fatalf("two resident type installs must stack: depth = %d, want 2", d)
	}
	top, ok := r.Defs.TopEntry("Tz")
	if !ok || top.TypeDef == nil || top.TypeDef == captured {
		t.Fatalf("top entry = %+v, want a FRESHLY minted node, not the captured one", top)
	}
	// Retire the top the way `undef` does; the level below must survive.
	ApplyResidentBind(r, "Tz", true, Value{})
	rest, ok := r.Defs.TopEntry("Tz")
	if !ok || rest.TypeDef == nil || r.Types.LookupByID(rest.TypeDef.ID) == nil {
		t.Fatalf("after one undef the remaining node is unresolvable (%+v) — the arm shared one node", rest)
	}

	// An installer compile failure propagates: the arms BELOW validateTypeName still
	// police the body's own rules, and a refine prefab that is not in the
	// lattice cannot be renamed and bound.
	prefab := r.Types.MintRefinePrefab(TInteger)
	lost := DefEntry{Body: NewTypeLiteral(prefab)}
	r.Types.Retire(prefab)
	if err := ApplyResidentTypeBind(r, "Tlost", lost); err == nil {
		t.Fatal("a body the installer declines must return its error, not install nothing silently")
	}
}

// ApplyResidentVar is the install arm of a var DECLARATION inside a resident
// body: the per-element cell is MARKED a var (its declared type kept), so
// the body's later assignment replaces it in place — one level — where the
// plain install arm had the assignment declare a second cell beside it. A
// nil registry is a no-op.
func TestApplyResidentVar(t *testing.T) {
	ApplyResidentVar(nil, "x", NewInteger(1), nil) // must not panic

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ApplyResidentVar(r, "x", NewInteger(10), TInteger)
	e, ok := IsVarBinding(r, "x")
	if !ok || e.VarType != TInteger {
		t.Fatalf("the resident var install is not a typed var binding: %+v / %v", e, ok)
	}
	ApplyResidentAssign(r, "x", NewInteger(11))
	if d := r.Defs.Depth("x"); d != 1 {
		t.Errorf("x is %d deep after assigning the resident var, want 1 (replaced in place)", d)
	}
	if v, _ := r.Defs.Top("x"); v.String() != "11" {
		t.Errorf("x reads %v after the assignment, want 11", v)
	}
	// The negative half: the plain install arm's cell is not a var, so the
	// same assignment declares beside it.
	ApplyResidentBind(r, "y", false, NewInteger(10))
	ApplyResidentAssign(r, "y", NewInteger(11))
	if d := r.Defs.Depth("y"); d != 2 {
		t.Errorf("y is %d deep after assigning over a plain resident install, want 2", d)
	}
}
