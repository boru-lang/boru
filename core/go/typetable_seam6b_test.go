package core

// Seam-6b cluster B1: unit tests for previously-unreached blocks in
// typetable.go, types.go, and typebehavior.go. All calls are in-package
// and direct (guard / error / prefix arms) per design/TEST-SEAMS.10.md.
// Tests that touch package-level state (builtinInitErrs, the Builtin
// table indexes) save and restore it via t.Cleanup and never run in
// t.Parallel.

import (
	"strings"
	"sync/atomic"
	"testing"
)

// --- typetable.go: nil receivers ------------------------------------------

func TestS6b1TypeNilReceivers(t *testing.T) {
	var tp *Type
	if got := tp.Path(); got != "" {
		t.Errorf("nil Path() = %q, want empty", got)
	}
	if got := tp.Root(); got != nil {
		t.Errorf("nil Root() = %v, want nil", got)
	}
}

func TestS6b1TypeTableNilReceivers(t *testing.T) {
	var tt *TypeTable
	if got := tt.LookupBuiltinByName("Integer"); got != nil {
		t.Errorf("nil LookupBuiltinByName = %v, want nil", got)
	}
	if _, ok := tt.ExpandShortName("Integer"); ok {
		t.Error("nil ExpandShortName must return ok=false")
	}
	if got := tt.Clone(); got != nil {
		t.Errorf("nil Clone = %v, want nil", got)
	}
	cp := tt.CloneDynamic()
	if cp == nil {
		t.Fatal("nil CloneDynamic must return a fresh dynamic table")
	}
	if cp.byID == nil {
		t.Error("nil CloneDynamic result must have initialised maps")
	}
}

// --- typetable.go: depthOf fallback ---------------------------------------

func TestS6b1DepthOfAdHocParent(t *testing.T) {
	// A parent with an unset Depth (ad-hoc *Type) takes the typeDepth
	// walk: chain length 1, so a child of it sits at depth 2.
	adhoc := &Type{tmeta: &typeMeta{Name: "S6b1AdHoc"}}
	if got := depthOf(adhoc); got != 2 {
		t.Errorf("depthOf(ad-hoc parent) = %d, want 2", got)
	}
}

// --- typetable.go: mintID / externalBandFor -------------------------------

func TestS6b1MintIDNilSeqAndPrefixes(t *testing.T) {
	// Bare literal table: the first mint must create the counter.
	bare := &TypeTable{byID: map[string]*Type{}}
	def := bare.MintType("s6b1bare", nil)
	if bare.seq == nil {
		t.Fatal("first mint on a bare table must initialise seq")
	}
	if !strings.HasPrefix(def.ID, "T_") {
		t.Errorf("nil-parent mint ID = %q, want T_ prefix", def.ID)
	}

	// mintID's category prefix reads parent.Root().Name. Every builtin
	// now roots at Any, so the Scalar/Node/Word arms fire only for
	// ad-hoc chains whose TOP node carries the branch name — build
	// those directly.
	dt := NewDynamicTypeTable()
	s := dt.MintType("s6b1s", &Type{tmeta: &typeMeta{Name: "Scalar"}})
	if !strings.HasPrefix(s.ID, "S_") {
		t.Errorf("Scalar-rooted mint ID = %q, want S_ prefix", s.ID)
	}
	if s.Rank() != 21_000_000_000 {
		t.Errorf("Scalar-rooted mint Rank = %d, want 21e9", s.Rank())
	}
	n := dt.MintType("s6b1n", &Type{tmeta: &typeMeta{Name: "Node"}})
	if !strings.HasPrefix(n.ID, "N_") {
		t.Errorf("Node-rooted mint ID = %q, want N_ prefix", n.ID)
	}
	w := dt.MintType("s6b1w", &Type{tmeta: &typeMeta{Name: "Word"}})
	if !strings.HasPrefix(w.ID, "W_") {
		t.Errorf("Word-rooted mint ID = %q, want W_ prefix", w.ID)
	}

	// A mint under the REAL Word branch takes the Word external band
	// (the branch walk stops one step below Any).
	rw := dt.MintType("s6b1rw", TWord)
	if rw.Rank() != 51_000_000_000 {
		t.Errorf("Word-branch mint Rank = %d, want 51e9", rw.Rank())
	}
}

func TestS6b1ExternalBandForNil(t *testing.T) {
	if got := externalBandFor(nil); got != 0 {
		t.Errorf("externalBandFor(nil) = %d, want 0", got)
	}
}

// --- typetable.go: AdoptSeqFrom -------------------------------------------

func TestS6b1AdoptSeqFrom(t *testing.T) {
	dt := NewDynamicTypeTable()
	before := dt.seq
	dt.AdoptSeqFrom(nil) // must be a no-op
	if dt.seq != before {
		t.Error("AdoptSeqFrom(nil) must not touch the counter")
	}
	// A source with no counter gets one minted, then shared.
	src := &TypeTable{}
	dt.AdoptSeqFrom(src)
	if src.seq == nil {
		t.Fatal("AdoptSeqFrom must initialise a nil source counter")
	}
	if dt.seq != src.seq {
		t.Error("AdoptSeqFrom must share the source counter pointer")
	}
}

// --- typetable.go: RegisterType leaf ambiguity ------------------

func TestS6b1RegisterTypeLeafDup(t *testing.T) {
	dt := NewDynamicTypeTable()
	if _, err := dt.RegisterType("S6b1Ldup", 91001, "plugin:test", nil); err != nil {
		t.Fatalf("root registration: %v", err)
	}
	// Same leaf name under the root: the leaf index entry must be
	// blanked so short-name expansion declines to guess.
	if _, err := dt.RegisterType("S6b1Ldup/S6b1Ldup", 91002, "plugin:test", nil); err != nil {
		t.Fatalf("child registration: %v", err)
	}
	if _, ok := dt.ExpandShortName("S6b1Ldup"); ok {
		t.Error("ambiguous leaf name must not expand")
	}
}

// --- typetable.go: Clone / CloneDynamic map copies -------------------------

func TestS6b1CloneCopiesByName(t *testing.T) {
	tt := &TypeTable{byName: map[string]*Type{"S6b1CN": TInteger}}
	nt := tt.Clone()
	if nt == nil {
		t.Fatal("Clone returned nil")
	}
	if got := nt.byName["S6b1CN"]; got != TInteger {
		t.Errorf("Clone byName copy = %v, want TInteger", got)
	}
}

func TestS6b1CloneDynamicCopiesAllIndexes(t *testing.T) {
	seq := new(atomic.Int64)
	tt := &TypeTable{
		byID:      map[string]*Type{"S6b1id": TInteger},
		byName:    map[string]*Type{"S6b1nm": TInteger},
		parts:     map[string]bool{"S6b1pt": true},
		bypath:    map[string]*Type{"S6b1pp": TInteger},
		rootSet:   map[string]bool{"S6b1rt": true},
		leafIndex: map[string]string{"S6b1lf": "S6b1pp"},
		seq:       seq,
	}
	cp := tt.CloneDynamic()
	if cp.byID["S6b1id"] != TInteger || cp.byName["S6b1nm"] != TInteger {
		t.Error("CloneDynamic must copy byID and byName")
	}
	if !cp.parts["S6b1pt"] {
		t.Error("CloneDynamic must copy parts")
	}
	if cp.bypath["S6b1pp"] != TInteger {
		t.Error("CloneDynamic must copy bypath")
	}
	if !cp.rootSet["S6b1rt"] {
		t.Error("CloneDynamic must copy rootSet")
	}
	if cp.leafIndex["S6b1lf"] != "S6b1pp" {
		t.Error("CloneDynamic must copy leafIndex")
	}
	if cp.seq != seq {
		t.Error("CloneDynamic must SHARE the mint counter")
	}
	// Copies are independent maps.
	cp.byID["S6b1new"] = TFloat
	if _, leaked := tt.byID["S6b1new"]; leaked {
		t.Error("CloneDynamic byID must be a fresh map")
	}
}

// --- typetable.go: registerBuiltin error arms -------------------------------

// s6b1FreshTable builds an empty fully-initialised table so
// registerBuiltin negative tests never touch the package Builtin table.
func s6b1FreshTable() *TypeTable {
	return &TypeTable{
		byID:      map[string]*Type{},
		byName:    map[string]*Type{},
		parts:     map[string]bool{},
		bypath:    map[string]*Type{},
		rootSet:   map[string]bool{},
		leafIndex: map[string]string{},
	}
}

// s6b1SaveInitErrs snapshots and restores the package-level
// builtinInitErrs accumulator around a test that provokes init errors,
// so NewRegistry in later tests stays clean.
func s6b1SaveInitErrs(t *testing.T) {
	t.Helper()
	saved := builtinInitErrs
	t.Cleanup(func() { builtinInitErrs = saved })
}

func TestS6b1RegisterBuiltinMissingParents(t *testing.T) {
	s6b1SaveInitErrs(t)
	base := len(builtinInitErrs)
	tt := s6b1FreshTable()

	// Explicit ParentPath that was never declared.
	tt.registerBuiltin(builtinDecl{Path: "S6b1RB1", ParentPath: "S6b1Missing", FixedID: 91101})
	if tt.bypath["S6b1RB1"] != nil {
		t.Error("a decl with a missing ParentPath must not register")
	}
	if len(builtinInitErrs) != base+1 {
		t.Fatalf("expected 1 recorded init error, got %d", len(builtinInitErrs)-base)
	}

	// Implicit parent (path prefix) that was never declared.
	tt.registerBuiltin(builtinDecl{Path: "S6b1RB2/S6b1RB2c", FixedID: 91102})
	if tt.bypath["S6b1RB2/S6b1RB2c"] != nil {
		t.Error("a decl with a missing path parent must not register")
	}
	if len(builtinInitErrs) != base+2 {
		t.Fatalf("expected 2 recorded init errors, got %d", len(builtinInitErrs)-base)
	}
}

func TestS6b1RegisterBuiltinDuplicateFixedID(t *testing.T) {
	s6b1SaveInitErrs(t)
	base := len(builtinInitErrs)
	tt := s6b1FreshTable()
	tt.registerBuiltin(builtinDecl{Path: "S6b1RB3", FixedID: 91103})
	if tt.bypath["S6b1RB3"] == nil {
		t.Fatal("first registration should succeed")
	}
	tt.registerBuiltin(builtinDecl{Path: "S6b1RB4", FixedID: 91103})
	if tt.bypath["S6b1RB4"] != nil {
		t.Error("duplicate FixedID must not register")
	}
	if len(builtinInitErrs) != base+1 {
		t.Errorf("expected 1 recorded init error, got %d", len(builtinInitErrs)-base)
	}
}

func TestS6b1RegisterBuiltinLeafDupBlanks(t *testing.T) {
	s6b1SaveInitErrs(t)
	tt := s6b1FreshTable()
	tt.registerBuiltin(builtinDecl{Path: "S6b1RB5", FixedID: 91104})
	tt.registerBuiltin(builtinDecl{Path: "S6b1RB5/S6b1RB5", FixedID: 91105})
	if got := tt.leafIndex["S6b1RB5"]; got != "" {
		t.Errorf("ambiguous leaf must blank the index entry, got %q", got)
	}
	if _, ok := tt.ExpandShortName("S6b1RB5"); ok {
		t.Error("ambiguous leaf name must not expand")
	}
}

// --- typetable.go: MintTestType category prefixes ---------------------------

func TestS6b1MintTestTypePrefixes(t *testing.T) {
	// MintTestType's category prefix also reads parent.Root().Name, and
	// every builtin roots at Any — so the branch arms fire only when the
	// parent chain's TOP node carries the branch name. Pre-seed the
	// test-only pool with ad-hoc roots (legitimate use of the test
	// scaffolding's cache) and restore afterwards.
	seed := map[string]string{
		"S6b1PoolSc": "Scalar",
		"S6b1PoolNd": "Node",
		"S6b1PoolWd": "Word",
	}
	for k, name := range seed {
		testTypePool[k] = &Type{tmeta: &typeMeta{Name: name}}
	}
	t.Cleanup(func() {
		for k := range seed {
			delete(testTypePool, k)
			delete(testTypePool, k+"/S6b1Kid")
		}
	})

	s := MintTestType("S6b1PoolSc/S6b1Kid")
	if !strings.HasPrefix(s.ID, "S_") {
		t.Errorf("Scalar test mint ID = %q, want S_ prefix", s.ID)
	}
	n := MintTestType("S6b1PoolNd/S6b1Kid")
	if !strings.HasPrefix(n.ID, "N_") {
		t.Errorf("Node test mint ID = %q, want N_ prefix", n.ID)
	}
	w := MintTestType("S6b1PoolWd/S6b1Kid")
	if !strings.HasPrefix(w.ID, "W_") {
		t.Errorf("Word test mint ID = %q, want W_ prefix", w.ID)
	}
}

// --- typetable.go: mustBuiltinType missing path -----------------------------

func TestS6b1MustBuiltinTypeMissing(t *testing.T) {
	s6b1SaveInitErrs(t)
	base := len(builtinInitErrs)
	def := mustBuiltinType("S6b1/NoSuchBuiltin")
	if def == nil {
		t.Fatal("mustBuiltinType must return a placeholder, not nil")
	}
	if def.Name() != "S6b1/NoSuchBuiltin" || def.Origin != OriginBuiltin {
		t.Errorf("placeholder = %+v, want Name=path Origin=builtin", def)
	}
	if len(builtinInitErrs) != base+1 {
		t.Errorf("expected a recorded init error, got %d new", len(builtinInitErrs)-base)
	}
	if BuiltinInitError() == nil {
		t.Error("BuiltinInitError must surface the recorded error")
	}
}

// --- types.go ---------------------------------------------------------------

func TestS6b1BuildTypeNamesSkipsNilEntry(t *testing.T) {
	Builtin.byName["S6b1NilName"] = nil
	t.Cleanup(func() { delete(Builtin.byName, "S6b1NilName") })
	m := buildTypeNames()
	if _, ok := m["S6b1NilName"]; ok {
		t.Error("buildTypeNames must skip nil byName entries")
	}
	if m["Integer"] != TInteger {
		t.Error("buildTypeNames must keep real entries")
	}
}

func TestS6b1TypeNameByIDNotUserFacing(t *testing.T) {
	fake := &Type{ID: "S6b1FakeByID", tmeta: &typeMeta{Name: "S6b1NotInByName"}}
	Builtin.byID["S6b1FakeByID"] = fake
	t.Cleanup(func() { delete(Builtin.byID, "S6b1FakeByID") })
	if got := TypeNameByID("S6b1FakeByID"); got != "" {
		t.Errorf("a type absent from byName must yield \"\", got %q", got)
	}
}

func TestS6b1ResolveTypePathNonCanonicalEntry(t *testing.T) {
	// A bypath entry whose *Type resolves Path() to a DIFFERENT key is
	// not canonical — ResolveTypePath must reject it (the final guard).
	decoy := &Type{tmeta: &typeMeta{Name: "S6b1Decoy"}}
	Builtin.bypath["S6b1Rtp/S6b1Sub"] = decoy
	t.Cleanup(func() { delete(Builtin.bypath, "S6b1Rtp/S6b1Sub") })
	if got, ok := ResolveTypePath("S6b1Rtp/S6b1Sub"); ok || got != nil {
		t.Errorf("non-canonical entry resolved: %v %v", got, ok)
	}
}

func TestS6b1PathSubtypeNil(t *testing.T) {
	var tp *Type
	if tp.PathSubtype(TInteger) {
		t.Error("nil receiver must not be a path subtype")
	}
	if TInteger.PathSubtype(nil) {
		t.Error("nil pattern must not match")
	}
}

func TestS6b1IsCapitalisedNameEmpty(t *testing.T) {
	if IsCapitalisedName("") {
		t.Error("empty name is not capitalised")
	}
}

// --- typebehavior.go ---------------------------------------------------------

// s6b1PrevWrapper completes behaviorWrapper into a full TypeBehavior
// (the embed deliberately omits Match) so it can flow through
// PrevBehavior's interface parameter.
type s6b1PrevWrapper struct{ behaviorWrapper }

func (s6b1PrevWrapper) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }

func TestS6b1PrevBehaviorChain(t *testing.T) {
	w := behaviorWrapper{prev: DefaultBehavior}
	if got := w.Prev(); got != DefaultBehavior {
		t.Errorf("Prev() = %v, want DefaultBehavior", got)
	}
	prev, ok := PrevBehavior(s6b1PrevWrapper{behaviorWrapper{prev: DefaultBehavior}})
	if !ok || prev != DefaultBehavior {
		t.Errorf("PrevBehavior(wrapper) = %v %v, want DefaultBehavior true", prev, ok)
	}
	prev, ok = PrevBehavior(DefaultBehavior)
	if ok || prev != nil {
		t.Errorf("PrevBehavior(plain) = %v %v, want nil false", prev, ok)
	}
}

// TestEqualIntervalLabelInvariant pins the soundness contract of the
// interval-label fast path in (*Type).Equal: among nodes carrying a DFS
// interval label (In > 0), In is a UNIQUE per-node identity and In-equality
// is EXACTLY ID-equality. If either invariant ever breaks — a future change
// to labelIntervals, or a labelled node minted outside the builtin table —
// the fast path would silently mis-answer identity, so this test guards it
// directly rather than relying on downstream dispatch to notice.
func TestEqualIntervalLabelInvariant(t *testing.T) {
	var nodes []*Type
	for _, n := range Builtin.byID {
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		t.Fatal("no builtin nodes")
	}

	// Every labelled node has a unique In (the fast path treats In-equality
	// as node identity, which is only sound if In never collides).
	seen := map[int]string{}
	labelled := 0
	for _, n := range nodes {
		in := n.In()
		if in <= 0 {
			continue
		}
		labelled++
		if prev, dup := seen[in]; dup {
			t.Errorf("In=%d shared by two nodes: %q and %q", in, prev, n.ID)
		}
		seen[in] = n.ID
	}
	if labelled == 0 {
		t.Fatal("no labelled builtin nodes — labelIntervals did not run")
	}

	// Equal (which uses the In fast path) must agree with pure ID/tmeta
	// identity for every pair — both the equal case and, crucially, the
	// mismatch case (the negative half of the contract).
	idIdentity := func(a, b *Type) bool {
		if a == b {
			return true
		}
		if a.tmeta != nil && a.tmeta == b.tmeta {
			return true
		}
		return a.ID != "" && a.ID == b.ID
	}
	sawMismatchPair := false
	for _, a := range nodes {
		for _, b := range nodes {
			want := idIdentity(a, b)
			if got := a.Equal(b); got != want {
				t.Errorf("Equal(%q,%q)=%v, ID-identity=%v (In %d vs %d)",
					a.ID, b.ID, got, want, a.In(), b.In())
			}
			if !want {
				sawMismatchPair = true
			}
		}
	}
	if !sawMismatchPair {
		t.Fatal("no distinct-node pair exercised — negative case unverified")
	}

	// Spot-check the two labelled builtins the marker predicates hit hottest.
	if !TInteger.Equal(TInteger) {
		t.Error("TInteger.Equal(TInteger) = false, want true")
	}
	if TInteger.Equal(TOpenParen) {
		t.Error("TInteger.Equal(TOpenParen) = true, want false (distinct builtins)")
	}
}
