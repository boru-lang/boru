package core

// DefEntry is one binding on a name's stack. Body is the bound value.
// TypeDef is the minted lattice type when the binding is a *type*
// binding (a capitalised `def`), or nil for an ordinary value binding.
// Minted records that the binding MINTED its TypeDef (the normal
// InstallType path) rather than ADOPTING an existing canonical node
// (a type alias — `def Foo Integer`): `undef` retires the lattice node
// only for minted bindings, since retiring an adopted node would
// delete another binding's (or a builtin's) identity from the ID index.
type DefEntry struct {
	Body    Value
	TypeDef *Type
	Minted  bool
}

// DefTable holds the stacked bindings for every name. Post the
// TYPE-UNIFORM Phase 4 collapse it is the *single* binding store —
// both `def`-bound values and the type bindings a capitalised `def`
// installs live here, keyed by name (the capitalisation convention
// keeps the two kinds of name disjoint, so one map suffices). Each
// name maps to a stack; the top is the active binding. `def NAME body`
// pushes, `undef NAME` (and the def-cleanup machinery) pops.
//
// A *type* binding additionally carries the minted lattice `*Type`
// (DefEntry.TypeDef) so `undef` can retire it from the type lattice.
//
// Extracted from Registry to keep that struct from accumulating stack-
// bookkeeping methods. Pair it with Snapshot / Restore for sandboxing
// patterns (fn-body, predicate, carrier merges) that need to roll back
// a region of pushes wholesale.
type DefTable struct {
	stacks map[string][]DefEntry
	// mutations counts binding pushes and pops since construction.
	// Monotone; never reset. Consumers compare two readings to detect
	// "did any binding change in between" — the TCO gate uses it to
	// decline eager frame teardown when arg auto-evaluation installed
	// or removed a binding the parked teardown would have sequenced
	// differently (design/legacy/TCO-STAGED.10.ignore Stage 3).
	mutations int64
	// gen is a per-name monotone generation counter, bumped by `touch`
	// whenever a name's binding stack changes (push / pop / replace /
	// truncate / delete / set). It is the invalidation key for
	// Registry.dispatchCache: a cached aggregate for a name is valid
	// exactly while Gen(name) is unchanged. Unlike `mutations` (a single
	// global count read by the TCO gate) this is per-name, so a param
	// push for `n` does not invalidate the cached dispatch table for
	// `add` — the property that makes the cache pay off in hot loops.
	gen map[string]int64
}

// NewDefTable returns an empty def table ready for use.
func NewDefTable() *DefTable {
	return &DefTable{stacks: make(map[string][]DefEntry), gen: make(map[string]int64)}
}

// touch bumps name's generation counter. Called from every mutator that
// changes name's binding stack; the bump is what invalidates a stale
// Registry.dispatchCache entry for name.
func (dt *DefTable) touch(name string) {
	dt.gen[name]++
}

// Gen returns name's current generation counter (0 if never mutated).
// Registry.Lookup compares this against its cached reading to decide
// whether the cached dispatch aggregate is still valid.
func (dt *DefTable) Gen(name string) int64 {
	if dt == nil {
		return 0
	}
	return dt.gen[name]
}

// Top returns what the most recent binding for name DENOTES, or
// (zero Value, false) if name is unbound. Canonical read for "what
// does this name resolve to right now". A value binding denotes its
// Body; a TYPE binding denotes its minted (or adopted) lattice node —
// the Stage 2 flip of design/legacy/TYPE-REPRESENTATION.1.ignore §5: a type name
// evaluates to its type, for every declaration kind, so `canon M`
// prints M and `fn M Any […]` sees a type where it used to see the
// declaration's structural body (NUR090). Consumers that need the
// declared STRUCTURE read it from the node (Value.TypeBody) or from
// the stored entry (TopTypeBody), never from evaluation.
func (dt *DefTable) Top(name string) (Value, bool) {
	if dt == nil {
		return Value{}, false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return Value{}, false
	}
	e := ds[len(ds)-1]
	if e.TypeDef != nil {
		return NewTypeLiteral(e.TypeDef), true
	}
	return e.Body, true
}

// TopEntry returns the most recent binding (body plus the type def, if
// any) for name, or (zero DefEntry, false) if name is unbound.
func (dt *DefTable) TopEntry(name string) (DefEntry, bool) {
	if dt == nil {
		return DefEntry{}, false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return DefEntry{}, false
	}
	return ds[len(ds)-1], true
}

// Push pushes a new value binding for name.
func (dt *DefTable) Push(name string, v Value) {
	if dt == nil {
		return
	}
	dt.mutations++
	dt.touch(name)
	dt.stacks[name] = append(dt.stacks[name], DefEntry{Body: v})
}

// PushType pushes a new type binding for name: the body plus the
// minted lattice type that carries this declaration's identity.
func (dt *DefTable) PushType(name string, def *Type, body Value) {
	if dt == nil {
		return
	}
	dt.mutations++
	dt.touch(name)
	dt.stacks[name] = append(dt.stacks[name], DefEntry{Body: body, TypeDef: def, Minted: true})
}

// PushTypeAdopted pushes a type binding whose TypeDef is an EXISTING
// canonical node the binding adopts rather than mints — the alias path
// (`def Foo Integer` binds the canonical Integer node itself). The
// entry's Minted flag stays false so `undef` never retires the adopted
// node.
func (dt *DefTable) PushTypeAdopted(name string, def *Type, body Value) {
	if dt == nil {
		return
	}
	dt.mutations++
	dt.touch(name)
	dt.stacks[name] = append(dt.stacks[name], DefEntry{Body: body, TypeDef: def})
}

// Pop pops the top binding for name. Returns true if there was a
// binding to pop. When the stack becomes empty the entry is removed
// from the map so Has returns false.
func (dt *DefTable) Pop(name string) bool {
	_, ok := dt.PopEntry(name)
	return ok
}

// PopEntry pops the top binding for name and returns it. The returned
// DefEntry's TypeDef is non-nil when a type binding was popped — the
// caller (undef) uses it to retire the type from the lattice.
func (dt *DefTable) PopEntry(name string) (DefEntry, bool) {
	if dt == nil {
		return DefEntry{}, false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return DefEntry{}, false
	}
	dt.mutations++
	dt.touch(name)
	top := ds[len(ds)-1]
	if len(ds) == 1 {
		delete(dt.stacks, name)
	} else {
		dt.stacks[name] = ds[:len(ds)-1]
	}
	return top, true
}

// Mutations returns the monotone count of binding pushes and pops.
// Compare two readings to detect intervening binding changes.
func (dt *DefTable) Mutations() int64 {
	if dt == nil {
		return 0
	}
	return dt.mutations
}

// TruncationCoveredBy reports whether truncating every name's stack to
// the depth recorded in snapshot (the DefCleanup operation) would
// remove only bindings whose names satisfy allowed. Allocation-free —
// the TCO eligibility gate runs this per detected tail call to prove
// the eager teardown removes nothing a dynamic read could miss.
func (dt *DefTable) TruncationCoveredBy(snapshot map[string]int, allowed func(string) bool) bool {
	if dt == nil {
		return true
	}
	for name, ds := range dt.stacks {
		if len(ds) > snapshot[name] && !allowed(name) {
			return false
		}
	}
	return true
}

// Has reports whether name has any active binding.
func (dt *DefTable) Has(name string) bool {
	if dt == nil {
		return false
	}
	return len(dt.stacks[name]) > 0
}

// IsType reports whether name's active binding is a type binding.
func (dt *DefTable) IsType(name string) bool {
	if dt == nil {
		return false
	}
	ds := dt.stacks[name]
	return len(ds) > 0 && ds[len(ds)-1].TypeDef != nil
}

// Depth returns the number of bindings currently stacked for name
// (0 if unbound).
func (dt *DefTable) Depth(name string) int {
	if dt == nil {
		return 0
	}
	return len(dt.stacks[name])
}

// Replace overwrites the body of the top binding for name with v,
// preserving the binding's type def. Returns true if there was a
// binding to replace; false (and no-op) if the stack was empty. Used
// by carrier-narrowing in `is` to re-bind the active iteration
// variable to a narrowed type.
func (dt *DefTable) Replace(name string, v Value) bool {
	if dt == nil {
		return false
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return false
	}
	dt.touch(name)
	ds[len(ds)-1].Body = v
	return true
}

// Truncate pops bindings from the top of name's stack until its depth
// equals want. If want >= current depth, no-op. If the stack becomes
// empty the entry is removed from the map.
func (dt *DefTable) Truncate(name string, want int) {
	if dt == nil {
		return
	}
	ds := dt.stacks[name]
	if want < 0 {
		want = 0
	}
	if want >= len(ds) {
		return
	}
	dt.touch(name)
	if want == 0 {
		delete(dt.stacks, name)
		return
	}
	dt.stacks[name] = ds[:want]
}

// Delete removes name's stack entirely. No-op if name is unbound.
func (dt *DefTable) Delete(name string) {
	if dt == nil {
		return
	}
	if len(dt.stacks[name]) > 0 {
		dt.touch(name)
	}
	delete(dt.stacks, name)
}

// Set replaces name's entire stack with value bindings carrying the
// given bodies. If bodies is empty the entry is removed from the map.
// Used by UninstallFnSigs (removes a specific middle entry then writes
// back) and by the def-handler's compile-then-replace path that
// filters out fallback entries before re-installing — both operate on
// value-binding (fn-def) stacks.
func (dt *DefTable) Set(name string, bodies []Value) {
	if dt == nil {
		return
	}
	dt.touch(name)
	if len(bodies) == 0 {
		delete(dt.stacks, name)
		return
	}
	entries := make([]DefEntry, len(bodies))
	for i, b := range bodies {
		entries[i] = DefEntry{Body: b}
	}
	dt.stacks[name] = entries
}

// Entries returns a snapshot of the full DefEntry stack for name,
// oldest-first — Body plus the type-binding half (TypeDef, Minted) that
// Stack's bodies-only view drops. Returns nil if name is unbound; the
// returned slice is owned by the caller. This is the read the bind-twin
// replay needs (§6.5 "replay, never re-execution"): re-installing a
// transition means reconstructing the ENTRY the pass left — a value push,
// a minted type binding, or an adopted alias — not just its body.
func (dt *DefTable) Entries(name string) []DefEntry {
	if dt == nil {
		return nil
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return nil
	}
	out := make([]DefEntry, len(ds))
	copy(out, ds)
	return out
}

// EntriesSnapshot captures every name's FULL entry stack plus its generation
// counter, for RestoreEntriesSnapshot. Unlike the depth-based Snapshot /
// Restore pair — whose restore can only TRUNCATE, so a pop or a same-depth
// replacement inside the region escapes it — this pair restores CONTENT
// exactly. Unlike Clone + a table swap (RestoreBindings), the restore below
// mutates IN PLACE, so code holding the *DefTable across the region keeps a
// valid reference, which is what a region nested INSIDE a pass (a macro
// template run, the dynamic-help eval firing mid-install) requires.
type EntriesSnapshot struct {
	stacks map[string][]DefEntry
	gens   map[string]int64
	valid  bool
}

// SnapshotEntries captures the table's current entry stacks for a later
// RestoreEntriesSnapshot.
func (dt *DefTable) SnapshotEntries() EntriesSnapshot {
	if dt == nil {
		return EntriesSnapshot{}
	}
	s := EntriesSnapshot{
		stacks: make(map[string][]DefEntry, len(dt.stacks)),
		gens:   make(map[string]int64, len(dt.stacks)),
		valid:  true,
	}
	for name, ds := range dt.stacks {
		cp := make([]DefEntry, len(ds))
		copy(cp, ds)
		s.stacks[name] = cp
		s.gens[name] = dt.gen[name]
	}
	return s
}

// RestoreEntriesSnapshot restores the table to a SnapshotEntries capture, in
// place. Only names whose GENERATION moved since the snapshot are touched —
// the per-name gen counter records every mutation, so an untouched name needs
// (and gets) no churn, keeping dispatch-cache invalidation proportional to
// what the region actually changed. Restored and deleted names are touched,
// so their gens move FORWARD — never rewound — which is what keeps a stale
// cache entry from colliding with a re-advanced timeline (the hazard
// RestoreBindings' wholesale cache reset exists for).
func (dt *DefTable) RestoreEntriesSnapshot(s EntriesSnapshot) {
	if dt == nil || !s.valid {
		return
	}
	for name := range dt.stacks {
		if _, ok := s.stacks[name]; !ok {
			dt.mutations++
			dt.touch(name)
			delete(dt.stacks, name)
		}
	}
	for name, want := range s.stacks {
		if dt.gen[name] == s.gens[name] {
			continue
		}
		cp := make([]DefEntry, len(want))
		copy(cp, want)
		dt.mutations++
		dt.touch(name)
		dt.stacks[name] = cp
	}
}

// Stack returns a snapshot of the bodies currently stacked for name,
// oldest-first. Returns nil if name is unbound. The returned slice is
// owned by the caller.
func (dt *DefTable) Stack(name string) []Value {
	if dt == nil {
		return nil
	}
	ds := dt.stacks[name]
	if len(ds) == 0 {
		return nil
	}
	bodies := make([]Value, len(ds))
	for i, e := range ds {
		bodies[i] = e.Body
	}
	return bodies
}

// Names returns a snapshot of all names currently bound. The slice is
// owned by the caller — mutating it has no effect on the table.
// Iteration order is map-iteration order.
func (dt *DefTable) Names() []string {
	if dt == nil {
		return nil
	}
	names := make([]string, 0, len(dt.stacks))
	for k := range dt.stacks {
		names = append(names, k)
	}
	return names
}

// Snapshot returns a per-name depth map covering every currently-bound
// name. Pair with Restore to roll a region of code back to the
// snapshotted state — additions and pushes during the region are
// unwound in one call. Used by the fn-body sandbox, predicate
// sandboxing, and the carrier-merge join points that need to compare
// branch states against a common pre-state.
func (dt *DefTable) Snapshot() map[string]int {
	if dt == nil {
		return nil
	}
	snap := make(map[string]int, len(dt.stacks))
	for k, v := range dt.stacks {
		snap[k] = len(v)
	}
	return snap
}

// Restore rolls every stack back to the depths recorded in snap
// (typically obtained from Snapshot). Names that are present in the
// table but absent from snap are deleted entirely. Names whose
// recorded depth is zero are also deleted.
func (dt *DefTable) Restore(snap map[string]int) {
	if dt == nil {
		return
	}
	for name := range dt.stacks {
		want, ok := snap[name]
		if !ok {
			dt.touch(name)
			delete(dt.stacks, name)
			continue
		}
		dt.Truncate(name, want)
	}
}

// Clone returns a deep copy of the def table: every name's binding
// stack is copied into a fresh slice so the clone and the original can
// be pushed to and popped from independently.
//
// DefEntry values are copied SHALLOWLY. That is correct for the *Type
// identity (a lattice node is a shared immutable), and correct for a
// bound Body only insofar as the binding itself is a snapshot: a `def`
// on the clone cannot disturb the original. It does NOT make the bound
// VALUE independent — a FlexMap / FlexList / Store / class-instance
// Value holds a pointer to shared state, so an in-place mutation
// through the clone's binding is visible through the original's.
//
// For sequential shadowing scopes that is exactly right (the whole
// point of a mutable container is that everyone holding it sees the
// writes). For CONCURRENT forks it is not — `await` therefore DECLINES a
// reachable mutable container at the branch boundary rather than relying
// on this copy (lang/go/native/native_temporal_await.go).
func (dt *DefTable) Clone() *DefTable {
	if dt == nil {
		return NewDefTable()
	}
	stacks := make(map[string][]DefEntry, len(dt.stacks))
	for name, st := range dt.stacks {
		cp := make([]DefEntry, len(st))
		copy(cp, st)
		stacks[name] = cp
	}
	// The gen map is COPIED so the clone continues the parent's generation
	// timeline rather than restarting it. dispatchCache is reset on the
	// forked registry (fork.go), so cache validity never depended on either
	// choice — but a runtime-stamped CompiledFnRef's dep snapshot
	// (depSnapEntry.Gen) IS compared across the stamp registry and its
	// forks: a per-connection fork must see the stamp-time generation for
	// an untouched module binding, or every stamped callback would read as
	// stale on arrival and permanently fall back to the interpreter.
	gen := make(map[string]int64, len(dt.gen))
	for name, g := range dt.gen {
		gen[name] = g
	}
	return &DefTable{stacks: stacks, gen: gen}
}

// HoldsType reports whether any LIVE entry, under any name, binds def — the
// question a type binding's pop asks before retiring the node it minted:
// a node pushed under one name more than once (a bind twin replaying one
// captured entry per element) is retired by the LAST pop, not the first,
// so the surviving levels keep resolving through LookupByID (NUR135).
func (dt *DefTable) HoldsType(def *Type) bool {
	if dt == nil || def == nil {
		return false
	}
	for _, stack := range dt.stacks {
		for i := range stack {
			if stack[i].TypeDef == def {
				return true
			}
		}
	}
	return false
}
