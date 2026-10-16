package core

import (
	"errors"
	"testing"
)

// The kernel lets a type say how it ORDERS (Comparer), SIZES (Sizer),
// HASHES (Hasher), UNIFIES (Unifier), RENDERS (Format) and PROJECTS
// (Nodifier, IdealConverter). Until Truther and DeepEqualer it could not
// say whether it is EMPTY or when two of its values are the SAME — those
// two answers were hardcoded kernel cascades with no opt-in.
//
// These tests pin both new walks in both directions: a type that opts in
// owns its answer, and one that does not is completely unaffected.

// ── truthiness ───────────────────────────────────────────────────────

type capTruthyBehavior struct {
	defaultBehavior
	truthy  bool
	decline bool
	fail    bool
}

func (b capTruthyBehavior) Truthy(Value) (bool, error) {
	switch {
	case b.decline:
		return false, ErrNoTruther
	case b.fail:
		return true, errors.New("body blew up")
	}
	return b.truthy, nil
}

// capType mints an ad-hoc lattice node under parent carrying behavior.
func capType(t *testing.T, name string, parent *Type, behavior TypeBehavior) *Type {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	ty := r.Types.MintType(name, parent)
	if ty == nil {
		t.Fatalf("MintType(%s) returned nil", name)
	}
	ty.ensureTMeta().Behavior = behavior
	return ty
}

func TestTruthinessCapabilityOwnsItsAnswer(t *testing.T) {
	// An Integer subtype whose Truther says FALSE. Without the walk the
	// number cascade would read the magnitude and call 7 truthy.
	falsy := capType(t, "Falsy", TInteger, capTruthyBehavior{truthy: false})
	v := NewInteger(7)
	SetParent(&v, falsy)
	if CoerceBoolean(v) {
		t.Error("a Truther returning false must win over the number cascade")
	}

	// …and the other direction: 0 is falsy by magnitude, truthy by body.
	truthy := capType(t, "Truthy", TInteger, capTruthyBehavior{truthy: true})
	z := NewInteger(0)
	SetParent(&z, truthy)
	if !CoerceBoolean(z) {
		t.Error("a Truther returning true must win over the zero-is-falsy rule")
	}

	// The plain type is untouched — the override is scoped to the node
	// that installed it, which is the whole point of the parent walk.
	if !CoerceBoolean(NewInteger(7)) || CoerceBoolean(NewInteger(0)) {
		t.Error("a plain Integer must keep the magnitude rule")
	}
}

func TestTruthinessCapabilityDeclineAndFailure(t *testing.T) {
	// ErrNoTruther means "not mine" — the cascade answers instead.
	decl := capType(t, "Declines", TInteger, capTruthyBehavior{decline: true})
	v := NewInteger(0)
	SetParent(&v, decl)
	if CoerceBoolean(v) {
		t.Error("a declining Truther must fall through to the zero-is-falsy rule")
	}
	v = NewInteger(7)
	SetParent(&v, decl)
	if !CoerceBoolean(v) {
		t.Error("a declining Truther must fall through to the magnitude rule")
	}

	// A body that ERRORS also falls through, rather than forcing a
	// verdict: CoerceBoolean has no error channel, so the built-in
	// reading is the safer of the two available answers.
	bad := capType(t, "Fails", TInteger, capTruthyBehavior{fail: true, truthy: true})
	z := NewInteger(0)
	SetParent(&z, bad)
	if CoerceBoolean(z) {
		t.Error("an erroring Truther must fall through, not impose its would-be answer")
	}
}

func TestTruthinessCapabilityInheritedByDescendants(t *testing.T) {
	// The walk is nearest-first up the parent chain, so a subtype with no
	// Truther of its own inherits its branch's — matching how Comparer
	// and Sizer already behave.
	base := capType(t, "Base", TInteger, capTruthyBehavior{truthy: false})
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	sub := r.Types.MintType("Sub", base)
	v := NewInteger(7)
	SetParent(&v, sub)
	if CoerceBoolean(v) {
		t.Error("a descendant must inherit its branch's Truther")
	}
}

// TestTruthinessUnchangedForEveryKernelType is the negative half: no
// kernel Behavior implements Truther, so the walk must be inert and the
// documented cascade must answer exactly as before.
func TestTruthinessUnchangedForEveryKernelType(t *testing.T) {
	om := NewOrderedMap()
	cases := []struct {
		name string
		v    Value
		want bool
	}{
		{"true", NewBoolean(true), true},
		{"false", NewBoolean(false), false},
		{"nonzero", NewInteger(7), true},
		{"zero", NewInteger(0), false},
		{"nonzero float", NewFloat(0.5), true},
		{"zero float", NewFloat(0), false},
		{"none", NewTypeLiteral(TNone), false},
		{"empty string", NewString(""), false},
		{"nonempty string", NewString("x"), true},
		{"the string false", NewString("false"), true},
		{"empty list", NewList(nil), false},
		{"nonempty list", NewList([]Value{NewInteger(1)}), true},
		{"empty map", NewMap(om), false},
		{"atom", NewAtom("a"), true},
	}
	for _, c := range cases {
		if got := CoerceBoolean(c.v); got != c.want {
			t.Errorf("%s: CoerceBoolean = %v, want %v", c.name, got, c.want)
		}
	}
	full := NewOrderedMap()
	full.Set("a", NewInteger(1))
	if !CoerceBoolean(NewMap(full)) {
		t.Error("a non-empty map must stay truthy")
	}
}

// ── deep equality ────────────────────────────────────────────────────

type capDeqBehavior struct {
	defaultBehavior
	equal   bool
	decline bool
	fail    bool
}

func (b capDeqBehavior) DeepEqualValues(Value, Value) (bool, error) {
	switch {
	case b.decline:
		return false, ErrNoDeepEqualer
	case b.fail:
		return true, errors.New("body blew up")
	}
	return b.equal, nil
}

// capDeqPair builds two values of an ad-hoc Ideal subtype carrying
// behavior. Ideal-branch values with no payload reach DeepEqual's
// terminal `false` today, which is exactly where the capability hooks.
func capDeqPair(t *testing.T, name string, behavior TypeBehavior) (Value, Value) {
	t.Helper()
	ty := capType(t, name, TIdeal, behavior)
	a := NewExtension(ty, "one")
	b := NewExtension(ty, "two")
	return a, b
}

func TestDeepEqualCapabilityAnswersAtTheTerminalPoint(t *testing.T) {
	// Baseline: without a DeepEqualer these two reach the terminal false.
	plainA, plainB := capDeqPair(t, "PlainIdeal", nil)
	if DeepEqual(plainA, plainB) {
		t.Fatal("fixture assumption broken: the pair must be deq-false without a capability")
	}

	// With one installed, the type's own answer is used.
	a, b := capDeqPair(t, "EqualIdeal", capDeqBehavior{equal: true})
	if !DeepEqual(a, b) {
		t.Error("an installed DeepEqualer must answer for its own values")
	}

	// A capability saying "not equal" is indistinguishable in RESULT from
	// the fall-through, but it must be reached — pin it by asserting the
	// declining variant and the false-returning one agree, which they can
	// only do if both paths run.
	no, no2 := capDeqPair(t, "UnequalIdeal", capDeqBehavior{equal: false})
	if DeepEqual(no, no2) {
		t.Error("a DeepEqualer returning false must report not-equal")
	}
}

func TestDeepEqualCapabilityDeclineAndFailure(t *testing.T) {
	a, b := capDeqPair(t, "DeclinesIdeal", capDeqBehavior{decline: true})
	if DeepEqual(a, b) {
		t.Error("a declining DeepEqualer must fall through to the terminal false")
	}
	c, d := capDeqPair(t, "FailsIdeal", capDeqBehavior{fail: true, equal: true})
	if DeepEqual(c, d) {
		t.Error("an erroring DeepEqualer must fall through, not impose its would-be answer")
	}
}

// TestDeepEqualCapabilityIsAdditive is the negative half. The walk sits
// at the terminal point, so it can only turn a false into a real answer
// — every arm above it must be unreachable from the capability, and a
// reflexive pair must stay reflexive.
func TestDeepEqualCapabilityIsAdditive(t *testing.T) {
	// A DeepEqualer installed on a SCALAR type must not be able to
	// override the scalar arm: the pair is resolved long before the walk.
	lying := capType(t, "LyingInteger", TInteger, capDeqBehavior{equal: true})
	a, b := NewInteger(1), NewInteger(2)
	SetParent(&a, lying)
	SetParent(&b, lying)
	if DeepEqual(a, b) {
		t.Error("the capability must not reach past the scalar arm — 1 deq 2 is false")
	}
	// …and the same values that were equal stay equal.
	c, d := NewInteger(3), NewInteger(3)
	SetParent(&c, lying)
	SetParent(&d, lying)
	if !DeepEqual(c, d) {
		t.Error("3 deq 3 must stay true")
	}

	// Kernel pairs are unchanged.
	if !DeepEqual(NewList([]Value{NewInteger(1)}), NewList([]Value{NewInteger(1)})) {
		t.Error("list deep equality regressed")
	}
	if DeepEqual(NewList([]Value{NewInteger(1)}), NewList([]Value{NewInteger(2)})) {
		t.Error("list deep inequality regressed")
	}
	if !DeepEqual(NewString("x"), NewString("x")) || DeepEqual(NewString("x"), NewString("y")) {
		t.Error("string deep equality regressed")
	}
}

// ── deq-index consistency (PR #325 review, Codex P1) ─────────────────
//
// DeepEqual and the DeqIndex behind `unique` / `member` / indices must
// agree. Before the classifier learned about the capability, a
// DeepEqualer-carrying value was classed DeqNeverEqual: Add filed it
// nowhere, FirstMatch answered -1, and `unique` kept what `deq` called
// a duplicate.

func TestDeqIndexConsultsDeepEqualCapability(t *testing.T) {
	// The capability says every pair of this type is equal.
	a, b := capDeqPair(t, "IndexedEqualIdeal", capDeqBehavior{equal: true})
	if !DeepEqual(a, b) {
		t.Fatal("fixture: the pair must be DeepEqual")
	}

	var idx DeqIndex
	if got := idx.Add(a); got != 0 {
		t.Fatalf("Add(a) = %d, want 0", got)
	}
	if got := idx.FirstMatch(b); got != 0 {
		t.Errorf("FirstMatch(b) = %d, want 0 — the index must agree with DeepEqual", got)
	}

	// The classification behind that: scannable, in a real family.
	if _, class := DeqKey(a); class != DeqUnkeyed {
		t.Errorf("DeqKey class = %v, want DeqUnkeyed", class)
	}
	if fam := deqFam(a); fam == "" {
		t.Error("deqFam must give capability values a scan family — an empty family makes DeqUnkeyed inert")
	}

	// A capability that answers false: scannable but never matching.
	c, d := capDeqPair(t, "IndexedUnequalIdeal", capDeqBehavior{equal: false})
	var idx2 DeqIndex
	idx2.Add(c)
	if got := idx2.FirstMatch(d); got != -1 {
		t.Errorf("FirstMatch = %d, want -1 — the capability said not equal", got)
	}
}

func TestDeqIndexCapabilityFamiliesDoNotCross(t *testing.T) {
	// Two DIFFERENT capability-carrying types: their LCA sits above both
	// owners, where no DeepEqualer exists, so they can never match — and
	// must not share a scan family (the tightness half of the fix).
	// Minted in ONE registry: minted IDs are deterministic per registry,
	// so two fresh registries would mint ID-colliding nodes that
	// Type.Equal conflates.
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	tyA := r.Types.MintType("FamAIdeal", TIdeal)
	tyA.ensureTMeta().Behavior = capDeqBehavior{equal: true}
	tyB := r.Types.MintType("FamBIdeal", TIdeal)
	tyB.ensureTMeta().Behavior = capDeqBehavior{equal: true}
	a := NewExtension(tyA, "one")
	b := NewExtension(tyB, "two")
	if DeepEqual(a, b) {
		t.Fatal("fixture: cross-owner pairs must not be DeepEqual")
	}
	if deqFam(a) == deqFam(b) {
		t.Errorf("different owners must scan in different families, both %q", deqFam(a))
	}
	var idx DeqIndex
	idx.Add(a)
	if got := idx.FirstMatch(b); got != -1 {
		t.Errorf("FirstMatch across owners = %d, want -1", got)
	}
}

func TestDeqIndexNeverEqualPreservedWithoutCapability(t *testing.T) {
	// With NO DeepEqualer in the chain the terminal classification stands.
	// NUR031's sealed-payload arm does NOT rescue this fixture: its Body
	// is a STRING, and reference identity needs a reference — a payload
	// whose body is not a pointer has only contents, and comparing those
	// would make two independently built payloads eq. So it declines, and
	// the pre-capability answer is what remains.
	a, b := capDeqPair(t, "PlainIndexIdeal", nil)
	if _, class := DeqKey(a); class != DeqNeverEqual {
		t.Errorf("DeqKey class = %v, want DeqNeverEqual for a non-pointer host payload", class)
	}
	var idx DeqIndex
	idx.Add(a)
	if got := idx.FirstMatch(b); got != -1 {
		t.Errorf("FirstMatch = %d, want -1", got)
	}
	// Reflexive probe included: never-equal means equal to NOTHING.
	if got := idx.FirstMatch(a); got != -1 {
		t.Errorf("FirstMatch(self) = %d, want -1", got)
	}
}

// ── size helpers (the fold guard's primitives) ───────────────────────

func TestSizeOwnerAndAbove(t *testing.T) {
	// A List's chain is owned by the kernel List node's Sizer.
	if owner := SizeOwner(TList); owner == nil || !owner.Equal(TList) {
		t.Errorf("SizeOwner(TList) = %v, want the List node itself", owner)
	}
	// A branch with NO Sizer anywhere — None's chain tops out at Any —
	// has no owner, matching SizeOf's 0.
	if owner := SizeOwner(TNone); owner != nil {
		t.Errorf("SizeOwner(TNone) = %v, want nil", owner)
	}
	if got := SizeOf(NewTypeLiteral(TNone)); got != 0 {
		t.Errorf("SizeOf(none) = %d, want 0", got)
	}

	// SizeOfAbove: resuming the walk above the List node skips the
	// element-count rule and finds nothing (List's parent chain has no
	// Sizer for a list payload)…
	lst := NewList([]Value{NewInteger(1), NewInteger(2)})
	if got := SizeOfAbove(TList, lst); got == 2 {
		t.Error("SizeOfAbove must skip the node it is called on")
	}
	// …while resuming above an Integer refinement still reaches the
	// Number magnitude rule — the arm userBehavior.Size leans on.
	ref := capType(t, "SizeRef", TInteger, nil)
	seven := NewInteger(7)
	SetParent(&seven, ref)
	if got := SizeOfAbove(ref, seven); got != 7 {
		t.Errorf("SizeOfAbove(refine Integer, 7) = %d, want the magnitude 7", got)
	}
	// A nil start is the zero answer, not a walk.
	if got := SizeOfAbove(nil, seven); got != 0 {
		t.Errorf("SizeOfAbove(nil) = %d, want 0", got)
	}
}
