package core

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
)

// OriginKind classifies where a *Type was registered. Every *Type
// is either seeded at init from the package-level builtinDecls list
// (OriginBuiltin) or minted at runtime via TypeTable.MintType — the
// path the `type` word and the anonymous `object {…}` handler take
// when introducing new identities (OriginUserDef).
type OriginKind uint8

const (
	// OriginBuiltin is set on every *Type seeded from builtinDecls
	// into the package-level Builtin table. Builtins have a stable
	// FixedID and never go away. Value-tagged subtypes minted by
	// NewType for paths like Scalar/Number/Integer/42 are also
	// OriginBuiltin — they're parametric instances of a builtin
	// parent, not user declarations.
	OriginBuiltin OriginKind = iota
	// OriginUserDef is set on every *Type minted by TypeTable.MintType
	// — the named `type Foo …` flow and the anonymous `object {…}`
	// constructor. Each mint produces a fresh *Type with a unique ID;
	// named types are then registered via Bind, anonymous ones are
	// not.
	OriginUserDef
)

// String returns a short human-readable label for the origin.
func (o OriginKind) String() string {
	switch o {
	case OriginBuiltin:
		return "builtin"
	case OriginUserDef:
		return "userdef"
	}
	return "unknown"
}

// Type is an alias for Value. The type lattice and the value space
// are one structure: a "type" is a Value used as a lattice node —
// its Parent is its supertype, its Behavior drives is/format/equal,
// its Name/FixedID/Rank carry lattice metadata. The full field set
// and the kernel/value duality are documented on the Value struct
// in value.go.
//
// The alias keeps the *Type spelling working at the call sites that
// traffic in lattice nodes; *Type and *Value are the same type.
// Type identity is pointer equality; builtins are seeded at init
// from builtinDecls, and MintType mints fresh identities at runtime.
type Type = Value

// IsNative reports whether t is a built-in type seeded at init from
// the package-level builtinDecls list. Returns false for user-defined
// types installed via the `type` word and for transient pool entries
// minted by NewType for unknown paths. Safe to call on a nil receiver.
func (t *Type) IsNative() bool {
	return t != nil && t.Origin == OriginBuiltin
}

// Owner sentinels for typeMeta.Owner (design/OPEN-WORDS.1.md §4).
// Every real registration/mint path stamps one of these or a module
// id; empty string means UNOWNED (ad-hoc / test fixtures) and anchors
// nothing.
const (
	// OwnerKernel marks kernel builtins (builtinDecls) and
	// kernel-shipped globally-registered types.
	OwnerKernel = "boru:kernel"
	// OwnerLang marks the language layer — the built-in words, the
	// bundled modules, and the gated capabilities. Used as the owner id
	// for lang's error-code registration (errorcodes.go); tooling reads it
	// to tell a kernel failure from a built-in-word failure, which is not
	// derivable from the code name.
	OwnerLang = "boru:lang"
	// OwnerProgram marks types minted by the top-level program
	// (refine / class outside any module body).
	OwnerProgram = "program"
)

// anyFixedID is Any's stable FixedID. Hardcoded here so Path() can
// short-circuit the "skip Any as parent" check without referencing
// the TAny var — that would create an initializer cycle (Builtin →
// registerBuiltin → Path → TAny → Builtin).
const anyFixedID = 1

// Path returns the slash-separated path by walking up the parent chain.
//
// Any acts as the universal lattice root — every other top-level type
// (Scalar, Node, Ideal, Type, Word, None, Never) chains to it via its
// Parent pointer — but Path() stops at Any so the textual paths stay
// the historical short form (Scalar.Path() == "Scalar", not
// "Any/Scalar"). This keeps FixedIDs / serialised Value IDs / spec
// tests / external registrations stable while still letting
// `typeof Scalar` saturate at `Any`.
func (t *Type) Path() string {
	if t == nil {
		return ""
	}
	if t.Parent == nil || t.Parent.FixedID() == anyFixedID {
		return t.Name()
	}
	return t.Parent.Path() + "/" + t.Name()
}

// Root returns the top of the ancestry chain.
func (t *Type) Root() *Type {
	if t == nil {
		return nil
	}
	for t.Parent != nil {
		t = t.Parent
	}
	return t
}

// IsAncestor reports whether ancestor lies on t's parent chain (or is
// t). Comparison is by lattice identity (Equal) so a by-value
// type-literal copy still recognises its own ancestors.
func (t *Type) IsAncestor(ancestor *Type) bool {
	// O(1) interval fast-path for the static builtin lattice: t descends
	// from ancestor iff t's DFS entry falls within ancestor's [In, Out]
	// range. Valid only when BOTH nodes are labelled (In > 0) — which is
	// exactly the builtins labelled at table construction, whose ancestry
	// is fixed. Minted / external / ad-hoc types (In == 0) fall through to
	// the structural walk, which stays the single source of truth.
	if t.In() > 0 && ancestor.In() > 0 {
		return ancestor.In() <= t.In() && t.In() <= ancestor.Out()
	}
	for x := t; x != nil; x = x.Parent {
		if x.Equal(ancestor) {
			return true
		}
	}
	return false
}

// depthOf returns the lattice depth a child of parent should carry: the
// root sits at depth 1, every step down adds one. parent.Depth is set at
// parent's own construction (builtins register parent-before-child), so
// this is O(1); a parent with an unset Depth (ad-hoc *Type) yields the
// walk-based length so the field still ends up consistent.
func depthOf(parent *Type) int {
	if parent == nil {
		return 1
	}
	if parent.Depth() > 0 {
		return parent.Depth() + 1
	}
	return typeDepth(parent) + 1
}

// labelIntervals assigns every node in the table a DFS nested-set interval
// [In, Out] so IsAncestor can answer descendant queries in O(1) (see the
// In/Out fields on Value and the fast-path in IsAncestor). Called once,
// after the builtin lattice is fully wired, on the package-level Builtin
// table only — dynamic per-registry tables never re-label, so their minted
// types keep In == 0 and route through the walk. Children are visited in ID
// order so the numbering is deterministic across runs.
func (tt *TypeTable) labelIntervals() {
	children := make(map[string][]*Type, len(tt.byID))
	var roots []*Type
	for _, t := range tt.byID {
		if t.Parent == nil {
			roots = append(roots, t)
			continue
		}
		children[t.Parent.ID] = append(children[t.Parent.ID], t)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })

	counter := 0
	var visit func(t *Type)
	visit = func(t *Type) {
		counter++
		t.ensureTMeta().In = counter
		kids := children[t.ID]
		sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
		for _, c := range kids {
			visit(c)
		}
		t.ensureTMeta().Out = counter
	}
	for _, r := range roots {
		visit(r)
	}
}

// TypeTable is the canonical catalogue of types. Builtin is the
// package-level table; per-Registry dynamic tables extend it via
// MintType.
//
// Post the TYPE-UNIFORM Phase 4 collapse the TypeTable is purely the
// type *lattice* — it no longer owns a dynamic binding stack. Type
// bindings installed by a capitalised `def` live in the Registry's
// single DefTable; this table only mints lattice identities
// (MintType), indexes them by ID (byID), and keeps the static builtin
// name index (byName).
type TypeTable struct {
	byID      map[string]*Type
	byName    map[string]*Type  // builtin name → Type (static; dynamic bindings live in DefTable)
	parts     map[string]bool   // every Part name used by a registered type
	bypath    map[string]*Type  // builtin-only path index (dynamic types can collide on path)
	rootSet   map[string]bool   // roots, for fast IsRoot checks
	leafIndex map[string]string // builtin leaf-name → full path; "" if ambiguous
	seq       *atomic.Int64     // minted-ID counter, one per registry TREE (shared by forks/modules, copied by rollback clones) — see mintID
	// MintOwner is the provenance stamped onto every type this table
	// mints (design/OPEN-WORDS.1.md §4): OwnerProgram for a program
	// registry, the module's owner token for a module sub-registry.
	// Set once at registry construction; copied by Clone/CloneDynamic.
	MintOwner string
}

// dynamicIDBase is the starting point for minted IDs, chosen well above
// any builtin FixedID so dynamic IDs never collide with builtins.
const dynamicIDBase = 0x10000000

// Lookup returns the Type for a builtin path, or nil if none.
// Dynamic types are NOT in this index — use Registry.LookupTypeName
// for shadow-aware resolution and LookupByID for direct identity lookup.
func (tt *TypeTable) Lookup(path string) *Type {
	if tt == nil {
		return nil
	}
	return tt.bypath[path]
}

// LookupByID returns the Type for a canonical ID, or nil if none.
func (tt *TypeTable) LookupByID(id string) *Type {
	if tt == nil || id == "" {
		return nil
	}
	return tt.byID[id]
}

// LookupBuiltinByName returns the builtin Type registered under a
// user-facing short name, or nil. Dynamic type bindings are NOT here —
// they live in the Registry's DefTable; use Registry.LookupTypeName
// for shadow-aware resolution across both.
func (tt *TypeTable) LookupBuiltinByName(name string) *Type {
	if tt == nil {
		return nil
	}
	return tt.byName[name]
}

// IsRoot reports whether part is a top-level root name (Scalar, Node, …).
func (tt *TypeTable) IsRoot(part string) bool {
	if tt == nil {
		return false
	}
	return tt.rootSet[part]
}

// KnownPart reports whether part appears in any registered type's path.
func (tt *TypeTable) KnownPart(part string) bool {
	if tt == nil {
		return false
	}
	return tt.parts[part]
}

// NewDynamicTypeTable returns an empty TypeTable for per-Registry use.
// Builtins are NOT pre-seeded; lookups for builtins go through the
// package-level Builtin table at call sites that need them.
func NewDynamicTypeTable() *TypeTable {
	return &TypeTable{
		byID:      make(map[string]*Type),
		byName:    make(map[string]*Type),
		parts:     make(map[string]bool),
		seq:       new(atomic.Int64),
		MintOwner: OwnerProgram,
	}
}

// mintID generates a fresh ID for a dynamically registered Type. The
// prefix mirrors the builtin convention (S_/N_/W_/T_) so dynamic IDs
// carry the same root-category signal as builtins.
//
// The counter (TypeTable.seq) is a SHARED POINTER across every table
// whose minted types can reach the same program — one counter per
// registry TREE, not per table. Type identity (`teq`, the nominal
// `is` walk, dispatch) is ID-based, and a per-table counter gave the
// Nth mint in any two sibling tables the same ID: a `refine Integer`
// minted in one inline module was teq-identical to a `refine String`
// minted in another, and to the first top-level mint after the
// import (design/OPEN-WORDS.0.md §4.2; pinned in mintid_test.go and
// lang/spec/module-instance.tsv §7). Who shares:
//
//   - CloneDynamic (concurrent forks — await results escape): SHARES.
//   - Module sub-registries (exports escape): adopt the importing
//     tree's counter via AdoptSeqFrom (RunModuleBody, BuildIOModule).
//   - Clone (rollback sandboxes): COPIES. Sandbox mints are discarded
//     on restore, so the parent's later mints must reproduce the same
//     IDs as if the sandbox never minted — that copy is what keeps a
//     check-mode engine and a plain run of the same program minting
//     identical IDs (the type-soundness ratchet compares the two by
//     identity).
//
// One counter per tree — rather than per process — keeps dynamic IDs
// a deterministic function of the program: two engines running the
// same source mint the same IDs. Two UNRELATED engines in one process
// can still mint colliding IDs; hosts exchanging Values across
// engines is out of scope (and was never sound). Atomic because
// await forks mint concurrently.
func (tt *TypeTable) mintID(parent *Type) string {
	if tt.seq == nil {
		// Tables constructed as bare literals (the package Builtin
		// table, test fixtures) get a counter on first mint.
		tt.seq = new(atomic.Int64)
	}
	n := tt.seq.Add(1)
	prefix := "T_"
	if parent != nil {
		switch parent.Root().Name() {
		case "Scalar":
			prefix = "S_"
		case "Node":
			prefix = "N_"
		case "Word":
			prefix = "W_"
		}
	}
	return fmt.Sprintf("%s%012x", prefix, dynamicIDBase+n)
}

// Adopt registers an EXISTING minted type in tt's ID index without
// re-minting — the canonical pointer travels as-is. Escaped module
// types (a module's exported type literals, the Parent tag on values
// its words produce) need this so the compiled OpPushType path
// (Registry.Types.LookupByID) resolves them in the IMPORTING registry,
// not only in the module sub-registry that minted them. Idempotent;
// a same-ID re-adoption keeps the first pointer (canonical identity).
func (tt *TypeTable) Adopt(t *Type) {
	if tt == nil || t == nil || t.ID == "" {
		return
	}
	if _, exists := tt.byID[t.ID]; !exists {
		tt.byID[t.ID] = t
	}
}

// AdoptSeqFrom makes tt draw minted-type IDs from src's counter, so
// types minted in tt can never collide with types minted anywhere in
// src's registry tree. Every sub-registry whose minted types can
// escape to the parent program (module bodies — their exports carry
// minted types) must adopt the importing tree's counter; see mintID.
func (tt *TypeTable) AdoptSeqFrom(src *TypeTable) {
	if src == nil {
		return
	}
	if src.seq == nil {
		src.seq = new(atomic.Int64)
	}
	tt.seq = src.seq
}

// MintType creates a fresh *Type with Origin=OriginUserDef and
// registers it in byID. The returned *Type is unbound — call Bind to
// associate it with a user-facing name. Callers typically mint, then
// construct a body Value using the returned *Type as its Parent, then
// Bind. Anonymous types (e.g. `object {…}` not installed by name)
// skip the Bind step and just keep the *Type as the Value's identity.
//
// Behavior defaults to DefaultBehavior. Callers needing custom
// dispatch construct the *Type via this path then set def.Behavior
// before exposing it, or use MintTypeWithBehavior.
func (tt *TypeTable) MintType(name string, parent *Type) *Type {
	def := &Type{
		Parent: parent,
		tmeta:  &typeMeta{Name: name, Depth: depthOf(parent), Behavior: DefaultBehavior, Owner: tt.MintOwner},
		Origin: OriginUserDef,
	}
	// User and external types share a single Rank band per kernel
	// branch — they sit one band above the kernel positional ranks
	// so they sort AFTER every kernel builtin in the same branch
	// (e.g. `def Foo refine Integer` ranks above Integer, Float,
	// String, etc.). Same-band types tiebreak via compareTypes →
	// depth → lex name. See externalBandFor for the per-branch
	// constants.
	if parent != nil {
		def.ensureTMeta().Rank = externalBandFor(parent)
	}
	def.ID = tt.mintID(parent)
	tt.byID[def.ID] = def
	return def
}

// MintRefinePrefab mints an anonymous user subtype as the output of
// `refine BaseType` (the bare-refine constructor surface). The
// returned *Type carries no Name; the paired `def Foo` recognises
// the prefab via IsRefinePrefab and renames-and-binds. This is the
// protocol channel between `refine` and `def` for distinguishing
// the subtype path (`def Foo refine Integer`) from the alias path
// (`def Foo Integer`, where the body remains the input type literal
// verbatim) — see TYPE-CANONICALIZATION.0.
func (tt *TypeTable) MintRefinePrefab(parent *Type) *Type {
	return tt.MintType("", parent)
}

// IsRefinePrefab reports whether v is an anonymous refine-bare
// prefab awaiting rename. True iff v is a bare type literal whose
// lattice node is user-minted with no Name — the unique shape
// `MintRefinePrefab` produces.
func IsRefinePrefab(v Value) bool {
	return IsBareTypeNode(v) && v.Origin == OriginUserDef && v.Name() == ""
}

// externalBandFor returns the Rank band for user/external types
// rooted at parent's branch. Each band sits one increment above the
// corresponding kernel band (Scalar 20e9 → external 21e9, Node 30e9
// → 31e9, Ideal 40e9 → 41e9, Word 50e9 → 51e9, Type 60e9 → 61e9),
// so external/user types always sort after every positional kernel
// type in the same branch. Types with no recognised root (or rooted
// directly at Any/None/Never) fall back to the parent's Rank.
func externalBandFor(parent *Type) int {
	if parent == nil {
		return 0
	}
	// Walk to the branch root (the immediate child of Any, or a
	// degenerate root). Any itself has FixedID=anyFixedID; stop one
	// step below.
	branch := parent
	for branch.Parent != nil && branch.Parent.FixedID() != anyFixedID {
		branch = branch.Parent
	}
	switch branch.Name() {
	case "Scalar":
		return 21_000_000_000
	case "Node":
		return 31_000_000_000
	case "Ideal":
		return 41_000_000_000
	case "Word":
		return 51_000_000_000
	case "Type":
		return 61_000_000_000
	}
	return parent.Rank()
}

// RegisterType installs a globally-registered, wire-stable type from
// outside the kernel decl table — host modules (temporal, matrix,
// fetch, plugin packages, …) that own a type the kernel doesn't need
// to know about by name. Conceptually equivalent to a builtinDecls
// row, but supplied at runtime by the owning module, which stamps its
// OWNER id (design/OPEN-WORDS.1.md §4) — the provenance the
// ownership-anchored signature rules check. This is the unified
// successor of the retired RegisterExternalBuiltin: the "external
// builtin" category is gone; what remains is one owner-carrying
// registration used for every global type.
//
// FixedID allocation policy: each module reserves a stable per-module
// range so cross-version ID stability survives reorderings and
// plugin loadings. Reserved ranges:
//
//	  100-999    eng-internal future-builtins
//	 1000-1999   lang/go/modules/time   (Date, DateTime, Instant, …)
//	 2000-2999   lang/go/modules/matrix (Matrix)
//	 3000-3999   lang/go/native/fetch              (Fetch, Request, Response)
//	 4000-4999   lang/go/engine (Timeout, Interval)
//	 5000-9999   reserved for future kernel use
//	10000+       host / third-party plugin types
//
// Callers register at module init (e.g. modules.RegisterTypes(r))
// and capture the returned *Type into a package-level variable. The
// kernel's dispatch path consults the type's Behavior — no special
// case for "external vs builtin" exists at runtime.
//
// Validates the path is well-formed (every part starts with [A-Z]),
// the parent path is registered, and the FixedID is unused. Returns
// the minted *Type on success.
func (tt *TypeTable) RegisterType(path string, fixedID int, owner string, behavior TypeBehavior) (*Type, error) {
	if owner == "" {
		return nil, fmt.Errorf("RegisterType: %q needs a non-empty owner id (OwnerKernel, a module id, …)", path)
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 || path == "" {
		return nil, fmt.Errorf("RegisterType: empty path")
	}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("RegisterType: invalid path %q (empty part)", path)
		}
		c := p[0]
		if c < 'A' || c > 'Z' {
			if !strings.HasPrefix(p, "__") {
				return nil, fmt.Errorf("RegisterType: invalid path %q (part %q must start with [A-Z])", path, p)
			}
		}
	}

	var parent *Type
	if len(parts) > 1 {
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		parent = tt.bypath[parentPath]
		if parent == nil {
			return nil, fmt.Errorf("RegisterType: parent %q not registered for %q", parentPath, path)
		}
	}

	if existing := tt.bypath[path]; existing != nil {
		return nil, fmt.Errorf("RegisterType: path %q already registered", path)
	}

	id := formatFixedID(path, fixedID)
	if existing, dup := tt.byID[id]; dup {
		return nil, fmt.Errorf("RegisterType: FixedID %d for %q collides with %q", fixedID, path, existing.Path())
	}

	if behavior == nil {
		behavior = DefaultBehavior
	}

	// A dynamic table (NewDynamicTypeTable) leaves the builtin-only
	// indexes nil; initialise on demand so registering into one writes a
	// real map instead of panicking on a nil-map assignment below.
	if tt.bypath == nil {
		tt.bypath = make(map[string]*Type)
	}
	if tt.rootSet == nil {
		tt.rootSet = make(map[string]bool)
	}
	if tt.leafIndex == nil {
		tt.leafIndex = make(map[string]string)
	}

	def := &Type{
		ID:     id,
		Parent: parent,
		tmeta:  &typeMeta{Name: parts[len(parts)-1], Depth: depthOf(parent), FixedID: fixedID, Behavior: behavior, Owner: owner},
		Origin: OriginBuiltin,
	}
	// External builtins share the user-/external-type band for
	// their branch (one increment above the kernel positional band)
	// so they sort after every kernel builtin in the same branch
	// and tiebreak among themselves by depth then name.
	if parent != nil {
		def.ensureTMeta().Rank = externalBandFor(parent)
	}
	tt.byID[id] = def
	tt.bypath[path] = def
	if parent == nil {
		tt.rootSet[path] = true
	}
	tt.byName[def.Name()] = def
	for _, p := range parts {
		tt.parts[p] = true
	}
	if existing, dup := tt.leafIndex[def.Name()]; dup {
		if existing != "" {
			tt.leafIndex[def.Name()] = ""
		}
	} else {
		tt.leafIndex[def.Name()] = path
	}
	// Refresh the parser's bare-name lookup snapshot so the newly-
	// registered type is resolvable by source-text references like
	// `Foo`. Only the package-level Builtin table feeds typeNames;
	// per-Registry dynamic tables do not.
	if tt == Builtin {
		refreshTypeNames()
	}
	return def, nil
}

// MintTypeWithBehavior is MintType plus a custom TypeBehavior. Used
// by registration paths that want to install a domain-specific
// Behavior at mint time (predicate types, dependent scalars, plugin
// types). A nil behavior falls back to DefaultBehavior.
func (tt *TypeTable) MintTypeWithBehavior(name string, parent *Type, behavior TypeBehavior) *Type {
	def := tt.MintType(name, parent)
	if behavior != nil {
		def.ensureTMeta().Behavior = behavior
	}
	return def
}

// Retire removes a dynamically-minted type from the ID index. Called
// by `undef` when a type binding is popped from the Registry's single
// DefTable, so the retired identity no longer resolves via LookupByID.
func (tt *TypeTable) Retire(def *Type) {
	if tt == nil || def == nil {
		return
	}
	delete(tt.byID, def.ID)
}

// idSnapshot returns a copy of the canonical-ID index. It is the TYPE half of
// the narrow binding sandbox (binding_sandbox.go): enough to tell which IDs
// existed at capture time, and nothing else — the Type pointers are shared
// because a def is immutable once minted.
func (tt *TypeTable) idSnapshot() map[string]*Type {
	if tt == nil {
		return nil
	}
	out := make(map[string]*Type, len(tt.byID))
	for id, def := range tt.byID {
		out[id] = def
	}
	return out
}

// readmitRetired re-admits every ID in snap that tt has since lost — undoing
// RETIREMENTS while leaving MINTS in place, which is the partition the bind
// twins need (binding_sandbox.go states why each half goes the way it does).
// The result is the union of the two id sets. Returns how many were re-admitted
// so a caller can assert the partition rather than trust it.
func (tt *TypeTable) readmitRetired(snap map[string]*Type) int {
	if tt == nil {
		return 0
	}
	n := 0
	for id, def := range snap {
		if _, live := tt.byID[id]; !live {
			tt.byID[id] = def
			n++
		}
	}
	return n
}

// Clone returns a deep copy of tt — used for snapshot/restore around
// predicate sandbox boundaries. Type pointers are shared (defs are
// immutable once minted); only the stacks themselves are duplicated.
func (tt *TypeTable) Clone() *TypeTable {
	if tt == nil {
		return nil
	}
	nt := &TypeTable{
		byID:      make(map[string]*Type, len(tt.byID)),
		byName:    make(map[string]*Type, len(tt.byName)),
		parts:     make(map[string]bool, len(tt.parts)),
		MintOwner: tt.MintOwner,
		// COPY the mint counter, don't share it: Clone is the rollback
		// snapshot (predicate / compile sandboxes), whose mints are
		// discarded on restore. The parent's later mints must reproduce
		// the same IDs as if the sandbox never minted — that is what
		// keeps a check-mode pass and a plain run of the same program
		// minting identical IDs. See mintID.
		seq: new(atomic.Int64),
	}
	if tt.seq != nil {
		nt.seq.Store(tt.seq.Load())
	}
	for k, v := range tt.byID {
		nt.byID[k] = v
	}
	for k, v := range tt.byName {
		nt.byName[k] = v
	}
	for k, v := range tt.parts {
		nt.parts[k] = v
	}
	return nt
}

// builtinDecl describes one builtin type. The declarative list below
// is the SINGLE SOURCE OF TRUTH for all builtin types — IDs, parents,
// user-facing visibility, everything.
type builtinDecl struct {
	Path       string
	ParentPath string // optional: explicit lattice parent override. Use for nominal roots that should chain to Any without nesting the path (e.g. Scalar/Node/Ideal sit under Any in the lattice but their Path() stays "Scalar"/"Node"/"Ideal").
	FixedID    int
	IsInternal bool   // true for Word/__XX runtime markers
	Alias      string // optional friendly short name for ExpandShortName (e.g. "Paren" → Word/__OP)
	Rank       int    // unified lattice rank — see builtinDecls
}

// builtinDecls lists every builtin type. Parent-first ordering is
// required so registerBuiltin can wire Parent pointers as it walks.
//
// FixedID values are stable across runs and must not change once
// assigned — they appear in serialized IDs. New types must use a fresh
// number, never recycle an old one.
//
// Rank is the unified lattice rank: a single integer giving the total
// order CompareValues / compareTypes use for cross-type ordering. It is
// positional — a type's Rank is its parent's Rank plus a depth-scaled
// offset, so a parent always ranks below its whole subtree and sibling
// order is least-to-most complex:
//
//	depth 0  roots          1e10 bands (Any/None/Never share band 1)
//	depth 1  branch kinds   +1e8 per sibling
//	depth 2  refinements    +1e7 per sibling   (Word markers: +1e3)
//
// User types (MintType) and external builtins (RegisterType)
// do not get a positional slot — they inherit the parent's Rank, and
// compareTypes breaks the resulting ties by name/id. Max rank ≈ 6e10,
// far under the int64 ceiling.
var builtinDecls = []builtinDecl{
	// Roots. Any/None/Never are childless degenerate roots packed into
	// the first 1e10 Rank band; the five structural roots take a 1e10
	// band each. See the unified-Rank scheme on builtinDecl.Rank.
	// Any is THE structural root for the main type hierarchy. The
	// structural roots (Scalar, Node, Ideal, Type, Word) chain to it
	// via ParentPath="Any"; Path() and PathOf skip Any so the declared
	// short paths stay stable (Scalar.Path() == "Scalar") while typeof
	// saturates uniformly (`typeof Scalar` → `Any`).
	//
	// None (unit) and Never (bottom) are deliberately kept as their
	// own roots — they're degenerate types with special unification
	// semantics (None unifies only with None; Never is uninhabited),
	// and chaining them to Any would make every `Parent.Equal(TAny)`
	// shortcut in the dispatch path silently match them too.
	{Path: "Any", FixedID: 1, Rank: 11_000_000_000},
	{Path: "None", FixedID: 2, Rank: 12_000_000_000},
	{Path: "Never", FixedID: 61, Rank: 13_000_000_000},
	// Absent is the kernel-internal type denoting "key/value not
	// present". It is the third degenerate root (alongside None and
	// Never) and shares their unification rule — only unifies with
	// itself. Used by map unification to synthesize the fill value for
	// a missing key, so `?:T` desugars to `disjunct(T, None, Absent)`
	// and the "may be absent" semantic is carried entirely in the
	// type rather than via map metadata.
	{Path: "Absent", FixedID: 74, Rank: 14_000_000_000},
	{Path: "Scalar", FixedID: 3, ParentPath: "Any", Rank: 20_000_000_000},
	{Path: "Node", FixedID: 11, ParentPath: "Any", Rank: 30_000_000_000},
	{Path: "Ideal", FixedID: 48, ParentPath: "Any", Rank: 40_000_000_000},
	{Path: "Word", FixedID: 17, ParentPath: "Any", Rank: 50_000_000_000},
	{Path: "Type", FixedID: 39, ParentPath: "Any", Rank: 60_000_000_000},

	// Scalar branch — children ordered least-to-most complex.
	{Path: "Scalar/Atom", FixedID: 18, Rank: 20_100_000_000},
	{Path: "Scalar/Boolean", FixedID: 10, Rank: 20_200_000_000},
	{Path: "Scalar/Number", FixedID: 7, Rank: 20_300_000_000},
	{Path: "Scalar/Number/Integer", FixedID: 8, Rank: 20_310_000_000},
	{Path: "Scalar/Number/Float", FixedID: 9, Rank: 20_320_000_000},
	{Path: "Scalar/Number/BigInteger", FixedID: 100, Rank: 20_330_000_000},
	{Path: "Scalar/Number/BigDecimal", FixedID: 101, Rank: 20_340_000_000},
	{Path: "Scalar/String", FixedID: 4, Rank: 20_400_000_000},
	{Path: "Scalar/String/EmptyString", FixedID: 6, Rank: 20_410_000_000},
	{Path: "Scalar/String/ProperString", FixedID: 5, Rank: 20_420_000_000},
	// Scalar/Micron — the structured-scalar family root (content
	// equality, immutable, object-like named properties read via
	// dot/get). Micron takes the positional slot Scalar/Path held
	// before it moved into the family as Pathon; the family sits
	// between String and the external Scalar band, so cross-family
	// ordering is unchanged by the move. The identities stay
	// kernel-declared here; the family CONTENT — validators,
	// grammars, Behaviors, the -on naming rule (a SubtypeNamer
	// capability the bind sites consult generically) — lives in
	// basic/go/micron.go. Pathon keeps Path's historical FixedID 47:
	// formatFixedID derives serialised IDs from the path ROOT only,
	// so the rename is byte-identical on the wire.
	{Path: "Scalar/Micron", FixedID: 111, Rank: 20_500_000_000},
	{Path: "Scalar/Micron/Pathon", FixedID: 47, Rank: 20_510_000_000},
	{Path: "Scalar/Micron/Emailon", FixedID: 112, Rank: 20_520_000_000},
	{Path: "Scalar/Micron/Urlon", FixedID: 113, Rank: 20_530_000_000},
	{Path: "Scalar/Micron/Ipon", FixedID: 114, Rank: 20_540_000_000},
	{Path: "Scalar/Micron/Hoston", FixedID: 115, Rank: 20_550_000_000},
	{Path: "Scalar/Micron/Semveron", FixedID: 116, Rank: 20_560_000_000},
	{Path: "Scalar/Micron/Cidron", FixedID: 117, Rank: 20_570_000_000},
	{Path: "Scalar/Micron/Macon", FixedID: 118, Rank: 20_580_000_000},
	{Path: "Scalar/Micron/Coloron", FixedID: 119, Rank: 20_590_000_000},
	{Path: "Scalar/Micron/Mimon", FixedID: 120, Rank: 20_600_000_000},
	{Path: "Scalar/Micron/Qion", FixedID: 121, Rank: 20_610_000_000},
	{Path: "Scalar/Micron/Phonon", FixedID: 122, Rank: 20_620_000_000},
	// Scalar/Time and descendants live in lang/go/native/native_temporal.go.

	// Node branch.
	{Path: "Node/List", FixedID: 12, Rank: 30_100_000_000},
	{Path: "Node/List/Args", FixedID: 13, Rank: 30_110_000_000},
	{Path: "Node/List/FlexList", FixedID: 79, Rank: 30_120_000_000},
	// Node/List/FlexList/WeakFlexList — the weak-valued FlexList child:
	// mutable-handle elements are held weakly w.r.t. GC and splice-
	// compact at observation points. See design/FLEX-ATTRS.1.md §4.
	{Path: "Node/List/FlexList/WeakFlexList", FixedID: 124, Rank: 30_121_000_000},
	{Path: "Node/Map", FixedID: 14, Rank: 30_200_000_000},
	{Path: "Node/Map/Inspect", FixedID: 31, Rank: 30_210_000_000},
	{Path: "Node/Map/FlexMap", FixedID: 78, Rank: 30_220_000_000},
	// Node/Map/FlexMap/WeakFlexMap — the weak-valued FlexMap child.
	// Scalars store strongly, mutable handles weakly, immutable Nodes
	// are declined (Python's weakref rule). design/FLEX-ATTRS.1.md §4.
	{Path: "Node/Map/FlexMap/WeakFlexMap", FixedID: 123, Rank: 30_221_000_000},
	// Node/Map/KeyVal — the map-iteration entry value (keyval.go).
	// Kernel-declared since the ADR-012 stage-2 move from lang. The
	// Rank is EXPLICITLY the Node external band (externalBandFor):
	// the type registered externally before the move, so the band
	// value — not a positional kernel rank — preserves every
	// pre-move ordering result bit-for-bit.
	{Path: "Node/Map/KeyVal", FixedID: 5002, Rank: 31_000_000_000},
	// Node/Xml — the immutable element value embedded XML literals
	// (`<tag>…</tag>`) and `parse xml` produce. A dedicated Node-branch
	// type (not a Map subtype) with a custom Behavior that serialises
	// back to XML; see design/XML-LITERAL.0.md and core_xml.go. The
	// parser emits it directly, so it is kernel-declared.
	{Path: "Node/Xml", FixedID: 108, Rank: 30_300_000_000},
	// Node/Xml/FlexXml — the mutable build-in-place variant of Node/Xml,
	// reached via `flex <xml>`, mirroring FlexMap/FlexList. Inherits the
	// Xml Behavior through the parent chain. See design/XML-LITERAL.0.md §5.
	{Path: "Node/Xml/FlexXml", FixedID: 110, Rank: 30_310_000_000},
	// Node/Xml/FlexXml/WeakFlexXml — the weak-valued FlexXml child:
	// mutable-handle children held weakly, spliced on sweep; text and
	// attributes stay strong. See design/FLEX-ATTRS.1.md §4.
	{Path: "Node/Xml/FlexXml/WeakFlexXml", FixedID: 125, Rank: 30_311_000_000},

	// Ideal branch — the structural type-kinds (Class, Record, Options,
	// Error, Store, Table) are direct children of Ideal: peer kinds. The
	// bare Object container type was removed (FixedID 30 retired, not
	// recycled); class instances live under Ideal/Class. Resource/Entity
	// are the SDK object-type hierarchy (Resource ← Entity), their own
	// peer kind under Ideal (they no longer descend from Object). FixedIDs
	// 36/37 are kept across the re-root so serialised Resource/Entity IDs
	// stay wire-stable. External modules graft Tensor / Timeout / Fetch /
	// … on as further Ideal/* kinds via RegisterType.
	{Path: "Ideal/Resource", FixedID: 36, Rank: 40_120_000_000},
	{Path: "Ideal/Resource/Entity", FixedID: 37, Rank: 40_121_000_000},
	// FixedID 44 retired with Ideal/Array (the mutable indexed container)
	// when it was removed; FlexList covers the mutable-list role. Not recycled.
	{Path: "Ideal/Record", FixedID: 16, Rank: 40_300_000_000},
	{Path: "Ideal/Options", FixedID: 38, Rank: 40_400_000_000},
	{Path: "Ideal/Error", FixedID: 45, Rank: 40_500_000_000},
	{Path: "Ideal/Store", FixedID: 42, Rank: 40_600_000_000},
	{Path: "Ideal/Store/System", FixedID: 43, Rank: 40_610_000_000},
	{Path: "Ideal/Table", FixedID: 15, Rank: 40_700_000_000},
	// Ideal/Reach — a first-class dot-access node (m.a.b). The parser emits
	// it, so it is kernel-declared. See design/REACH.10.md.
	{Path: "Ideal/Reach", FixedID: 29, Rank: 40_800_000_000},
	// Ideal/Class — the root of user-defined class types (the nominal
	// record kinds minted by the `class` word). Class types are minted
	// as children of this node; instances are children of their class.
	// Classes are NOT Object subtypes — Object is the open mutable
	// keyed container, classes are sealed nominal records. See
	// design/legacy/CLASS-OBJECT.10.ignore.
	{Path: "Ideal/Class", FixedID: 102, Rank: 40_900_000_000},
	// Ideal/Surface — the root of user-defined surface types (the
	// pure-contract operation sets minted by the `surface` word).
	// Conformance is explicit (`<Type> exposes <Surface>`) and checked
	// loudly at declaration; membership is a conformance-set probe via
	// surfaceUnifier. See design/legacy/SURFACES.10.ignore.
	{Path: "Ideal/Surface", FixedID: 103, Rank: 40_910_000_000},
	// Ideal/Module — the module descriptor (moduletype.go). Kernel-
	// declared since the ADR-012 stage-2 move from lang; the Rank is
	// EXPLICITLY the Ideal external band, same reasoning as
	// Node/Map/KeyVal above.
	{Path: "Ideal/Module", FixedID: 5000, Rank: 41_000_000_000},

	// Word branch — Word/__XX entries are internal runtime markers,
	// packed at 1e3 Rank spacing. They expose friendly short-name
	// aliases (e.g. "Paren" → Word/__OP) so ResolveTypeName / NewType
	// can resolve them by their lang-level label.
	{Path: "Word/__FW", FixedID: 21, IsInternal: true, Alias: "Forward", Rank: 50_100_000_000},
	{Path: "Word/__OP", FixedID: 22, IsInternal: true, Alias: "Paren", Rank: 50_100_001_000},
	{Path: "Word/__CP", FixedID: 72, IsInternal: true, Alias: "CloseParen", Rank: 50_100_002_000},
	{Path: "Word/__ED", FixedID: 73, IsInternal: true, Alias: "End", Rank: 50_100_003_000},
	{Path: "Word/__PE", FixedID: 63, IsInternal: true, Rank: 50_100_004_000},
	{Path: "Word/__IS", FixedID: 51, IsInternal: true, Rank: 50_100_005_000},
	// Word/__XI — interpolated XML literal skeleton (`<p>${x}</p>`). A
	// runtime marker like __IS: the engine evaluates it in place to a
	// Node/Xml. See design/XML-LITERAL.0.md §4 and core_xml.go.
	{Path: "Word/__XI", FixedID: 109, IsInternal: true, Rank: 50_100_005_500},
	// Word/__FN (FixedID 23, "Fndef") — RETIRED 2026-07-31 (ADR-011):
	// collapsed into Type/Function; there is exactly one function type.
	// The FixedID is retired, never recycled.
	{Path: "Word/__RC", FixedID: 25, IsInternal: true, Alias: "Returncheck", Rank: 50_100_007_000},
	{Path: "Word/__MK", FixedID: 27, IsInternal: true, Alias: "Mark", Rank: 50_100_008_000},
	{Path: "Word/__MV", FixedID: 28, IsInternal: true, Alias: "Move", Rank: 50_100_009_000},
	{Path: "Word/__IN", FixedID: 20, IsInternal: true, Rank: 50_100_011_000},
	{Path: "Word/__IN/__DC", FixedID: 64, IsInternal: true, Rank: 50_100_011_001},
	{Path: "Word/__SP", FixedID: 75, IsInternal: true, Alias: "Splice", Rank: 50_100_012_000},
	{Path: "Word/__DM", FixedID: 76, IsInternal: true, Alias: "DispatchMod", Rank: 50_100_013_000},
	{Path: "Word/__SG", FixedID: 80, IsInternal: true, Alias: "Sugar", Rank: 50_100_014_000},

	// Type (metatype) branch.
	{Path: "Type/Function", FixedID: 19, Rank: 60_100_000_000},
	{Path: "Type/FunctionSignature", FixedID: 24, Rank: 60_200_000_000},
	{Path: "Type/Disjunct", FixedID: 26, Rank: 60_300_000_000},
	{Path: "Type/Disjunct/Enum", FixedID: 62, Rank: 60_310_000_000},
	{Path: "Type/Negation", FixedID: 77, Rank: 60_400_000_000},
	// Type/Self — the placeholder the receiver/argument positions of a
	// surface schema use (`{area: fnsig [[Self] [Float]]}`). `exposes`
	// substitutes the candidate type for Self when checking
	// conformance. As a constraint outside a surface schema, no value
	// matches Self — it is a placeholder, not a category. Generic
	// schemas reuse it for self-reference (`Self of [T]` — D5 in
	// design/legacy/GENERICS.10.ignore).
	{Path: "Type/Self", FixedID: 104, Rank: 60_500_000_000},
	// Generics metatypes (design/legacy/GENERICS.10.ignore): TypeParam is the root
	// unconstrained type-parameter placeholders mint under (bounded
	// ones mint under their bound's node); GenSpec is the value `gen
	// […]` produces and the GenSpec-aware constructor overloads
	// consume; GenParam is the per-parameter value `extends`/`default`
	// build inside a gen list. Schema and instantiation nodes are
	// MintType-dynamic — only these three roots are kernel-declared.
	{Path: "Type/TypeParam", FixedID: 105, Rank: 60_600_000_000},
	{Path: "Type/GenSpec", FixedID: 106, Rank: 60_700_000_000},
	{Path: "Type/GenParam", FixedID: 107, Rank: 60_800_000_000},
}

// Builtin is the package-level TypeTable holding every builtin type.
// It is populated once at init from builtinDecls and is read-only
// thereafter — per-Registry dynamic tables extend it via PushType.
var Builtin = newBuiltinTypeTable()

// builtinInitErrs collects any error hit while building the Builtin
// table or resolving a well-known T* constant. These can only arise
// from a programmer error in builtinDecls — a missing parent, a
// duplicate FixedID, an unknown well-known path — never from runtime
// input. Per ADR-005 (no deliberate panics; infrastructure code always
// returns errors) they are accumulated here at init and surfaced as an
// error from NewRegistry rather than crashing the process.
var builtinInitErrs []error

func recordBuiltinInitErr(err error) {
	builtinInitErrs = append(builtinInitErrs, err)
}

// BuiltinInitError returns the first error accumulated while building the
// builtin type table, or nil. NewRegistry consults it so a malformed
// builtinDecls or a bad well-known path is reported as an error at
// construction time instead of panicking during package init.
func BuiltinInitError() error {
	if len(builtinInitErrs) == 0 {
		return nil
	}
	return builtinInitErrs[0]
}

func newBuiltinTypeTable() *TypeTable {
	tt := &TypeTable{
		byID:      make(map[string]*Type, len(builtinDecls)),
		byName:    make(map[string]*Type),
		parts:     make(map[string]bool),
		bypath:    make(map[string]*Type, len(builtinDecls)),
		rootSet:   make(map[string]bool),
		leafIndex: make(map[string]string),
	}
	for _, d := range builtinDecls {
		tt.registerBuiltin(d)
	}
	// Number the fully-wired lattice so IsAncestor / ConformsTo can answer
	// builtin-vs-builtin subtype queries in O(1) via the interval test.
	tt.labelIntervals()
	// Type answers membership (`v is Type`, `t:Type` params, `[Type]`
	// returns) via typeMembershipBehavior — one question at every
	// boundary, per the refine doctrine (the membership set strictly
	// contains the default Parent-conformance set, so this only widens
	// acceptance to the literals and bodies `is Type` already admits).
	// Installed on the canonical node here so every lookup sees it.
	if t := tt.bypath["Type"]; t != nil {
		t.ensureTMeta().Behavior = typeMembershipBehavior{}
	}
	return tt
}

func (tt *TypeTable) registerBuiltin(d builtinDecl) {
	parts := strings.Split(d.Path, "/")
	var parent *Type
	switch {
	case d.ParentPath != "":
		parent = tt.bypath[d.ParentPath]
		if parent == nil {
			recordBuiltinInitErr(fmt.Errorf("typetable: ParentPath %q not registered before %q (declare parents first in builtinDecls)", d.ParentPath, d.Path))
			return
		}
	case len(parts) > 1:
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		parent = tt.bypath[parentPath]
		if parent == nil {
			recordBuiltinInitErr(fmt.Errorf("typetable: parent %q not registered before %q (declare parents first in builtinDecls)", parentPath, d.Path))
			return
		}
	}
	id := formatFixedID(d.Path, d.FixedID)
	if existing, dup := tt.byID[id]; dup {
		recordBuiltinInitErr(fmt.Errorf("typetable: duplicate FixedID %d for %q (already used by %q)", d.FixedID, d.Path, existing.Path()))
		return
	}
	def := &Type{
		ID:         id,
		Parent:     parent,
		tmeta:      &typeMeta{Name: parts[len(parts)-1], Depth: depthOf(parent), FixedID: d.FixedID, Rank: d.Rank, Behavior: DefaultBehavior, Owner: OwnerKernel},
		IsInternal: d.IsInternal,
		Origin:     OriginBuiltin,
	}
	tt.byID[id] = def
	tt.bypath[d.Path] = def
	if parent == nil {
		tt.rootSet[d.Path] = true
	}
	if !d.IsInternal {
		tt.byName[def.Name()] = def
	}
	for _, p := range parts {
		tt.parts[p] = true
	}
	if existing, dup := tt.leafIndex[def.Name()]; dup {
		// Ambiguous leaf name — mark with "" so ExpandShortName won't expand.
		if existing != "" {
			tt.leafIndex[def.Name()] = ""
		}
	} else {
		tt.leafIndex[def.Name()] = d.Path
	}
	if d.Alias != "" {
		tt.leafIndex[d.Alias] = d.Path
	}
}

// ExpandShortName returns the full builtin path for a short leaf name
// (e.g. "Integer" → "Scalar/Number/Integer"). Returns ok=false if the
// name is unknown or maps to multiple builtin paths.
func (tt *TypeTable) ExpandShortName(short string) (string, bool) {
	if tt == nil {
		return "", false
	}
	p, ok := tt.leafIndex[short]
	if !ok || p == "" {
		return "", false
	}
	return p, true
}

// formatFixedID formats a fixed numeric ID with the prefix derived
// from the path's root part. Output is 14 chars: "<prefix>_<12 hex>".
func formatFixedID(path string, num int) string {
	root := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		root = path[:i]
	}
	prefix := "T_"
	switch root {
	case "Scalar":
		prefix = "S_"
	case "Node":
		prefix = "N_"
	case "Word":
		prefix = "W_"
	}
	return fmt.Sprintf("%s%012x", prefix, num)
}

// MintTestType is a test-only helper that mints a *Type from a
// slash-separated path, walking from the builtin root where possible
// and minting intermediate *Types as needed. Used by lattice /
// ConformsTo / Specificity tests that need synthetic type hierarchies;
// production
// code goes through NewType (strict — unknown paths error) or
// TypeTable.MintType (explicit name + parent).
//
// Short-name first parts are auto-expanded the same way NewType does
// it, so MintTestType("Number/Float") attaches under the builtin
// Scalar/Number rather than under a fresh top-level "Number".
//
// Minted entries are cached per path string so repeated calls return
// the same *Type. Origin is OriginUserDef. NOT for use outside tests.
func MintTestType(path string) *Type {
	if def := testTypePool[path]; def != nil {
		return def
	}
	parts := strings.Split(path, "/")
	// Auto-expand short-name first parts via the Builtin leaf index
	// (mirrors NewType so test paths under "Number" land beneath
	// Scalar/Number).
	if _, isRoot := Builtin.bypath[parts[0]]; !isRoot {
		if fullPrefix, ok := Builtin.ExpandShortName(parts[0]); ok {
			expanded := fullPrefix
			if len(parts) > 1 {
				expanded += "/" + strings.Join(parts[1:], "/")
			}
			parts = strings.Split(expanded, "/")
		}
	}
	fullPath := strings.Join(parts, "/")
	// If the fully-expanded path is itself a builtin, return that — no
	// need to mint a separate test type for known types.
	if def := Builtin.bypath[fullPath]; def != nil {
		testTypePool[path] = def
		return def
	}
	var parent *Type
	if len(parts) > 1 {
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		if p := Builtin.bypath[parentPath]; p != nil {
			parent = p
		} else {
			parent = MintTestType(parentPath)
		}
	}
	testTypeSeq++
	prefix := "T_"
	if parent != nil {
		if root := parent.Root(); root != nil {
			switch root.Name() {
			case "Scalar":
				prefix = "S_"
			case "Node":
				prefix = "N_"
			case "Word":
				prefix = "W_"
			}
		}
	}
	def := &Type{
		ID:     fmt.Sprintf("%st%011x", prefix, testTypeSeq),
		tmeta:  &typeMeta{Name: parts[len(parts)-1], Behavior: DefaultBehavior, Owner: OwnerProgram},
		Parent: parent,
		Origin: OriginUserDef,
	}
	if parent != nil {
		def.ensureTMeta().Rank = externalBandFor(parent)
	}
	testTypePool[path] = def
	return def
}

var testTypePool = map[string]*Type{}
var testTypeSeq int

// BuiltinIDForPath returns the canonical Builtin ID for path, or ""
// if the path is not a registered builtin.
func BuiltinIDForPath(path string) string {
	if def := Builtin.bypath[path]; def != nil {
		return def.ID
	}
	return ""
}

// mustBuiltinType returns the Type for a builtin path. Panics if the
// path is not registered — used by the well-known T* constants in
// types.go, where any missing entry is a programmer error.
func mustBuiltinType(path string) *Type {
	def := Builtin.bypath[path]
	if def == nil {
		recordBuiltinInitErr(fmt.Errorf("typetable: builtin %q not in Builtin table", path))
		// Return a degenerate placeholder so package-level var-init
		// signature slices that reference this T* constant stay non-nil.
		// The recorded error is surfaced at NewRegistry, which returns it
		// before the registry is ever used, so the placeholder is never
		// actually dispatched against.
		return &Type{tmeta: &typeMeta{Name: path, Owner: OwnerKernel}, Origin: OriginBuiltin}
	}
	return def
}

// CloneDynamic returns a copy of a per-Registry dynamic type table for a
// concurrently-running fork. The lookup maps are copied so a mint in the
// fork (which writes byID/byName/parts) cannot race the parent's maps;
// the *Type pointers are shared, since a minted type's identity is
// stable and concurrent forks only read pre-existing types. Builtins are
// unaffected — they live in the package-level Builtin table, which is
// read-only after init and safe to share. Used by
// Registry.ForkConcurrent.
func (tt *TypeTable) CloneDynamic() *TypeTable {
	if tt == nil {
		return NewDynamicTypeTable()
	}
	cp := &TypeTable{
		byID:      make(map[string]*Type, len(tt.byID)),
		byName:    make(map[string]*Type, len(tt.byName)),
		parts:     make(map[string]bool, len(tt.parts)),
		MintOwner: tt.MintOwner,
		// SHARE the mint counter: a concurrent fork's values (await
		// branch results) escape back to the parent, so a mint in the
		// fork must never collide with a mint anywhere else in the
		// tree. See mintID.
		seq: tt.seq,
	}
	for k, v := range tt.byID {
		cp.byID[k] = v
	}
	for k, v := range tt.byName {
		cp.byName[k] = v
	}
	for k, v := range tt.parts {
		cp.parts[k] = v
	}
	if tt.bypath != nil {
		cp.bypath = make(map[string]*Type, len(tt.bypath))
		for k, v := range tt.bypath {
			cp.bypath[k] = v
		}
	}
	if tt.rootSet != nil {
		cp.rootSet = make(map[string]bool, len(tt.rootSet))
		for k, v := range tt.rootSet {
			cp.rootSet[k] = v
		}
	}
	if tt.leafIndex != nil {
		cp.leafIndex = make(map[string]string, len(tt.leafIndex))
		for k, v := range tt.leafIndex {
			cp.leafIndex[k] = v
		}
	}
	return cp
}
