# TYPE-CANONICALIZATION.10 — Canonical Pointers, Unified Resolve, Unified Reparent

This document captures a refactor that closes the third generation of
type-system fractures the prior two refactors didn't reach.

- **TYPE-DECOUPLING.0** sealed `Value.Data` and externalised domain
  types — the *payload* hole.
- **TYPE-UNIFORM.0** collapsed `type`/`object`/`record`/`table`/`untype`
  onto `def`/`make`/`refine` — the *surface* hole.

This pass closes the discipline holes that surfaced once
`refine`-bare scalar subtypes started carrying user-installed
Comparers via `behave`. The visible API is uniform after TYPE-UNIFORM,
but the underlying handlers still carry pre-collapse assumptions:
several sites grab non-canonical `*Type` pointers, three independent
`resolve-name-to-type` helpers exist, and four bespoke
typed-def reparent branches each invent their own pattern.

Status: design + implementation in flight (this branch).

## 1. Why the bugs clustered

Working a single user-level scenario — `def Foo refine Integer; def
Bar refine Integer; def x:Foo 1; def y:Bar 2; behave compare/q (fn
[[a:Item b:Item] [Integer] [body]])` — uncovered seven distinct bugs.
Each was latent because no one test exercised the full chain. They
sort cleanly into two clumps.

### Fracture A — `Type = Value` alias has no canonicalization discipline

The kernel aliases `type Type = Value` (`eng/typetable.go:53`). A
"type literal" is dual: it is a `Value` with `Data == nil` *and* it is
the canonical `*Type` registered in `TypeTable.byID`. Most code paths
produce or consume "a type literal" as a Value, then take `&v` (a
pointer to the stack copy) instead of looking up the canonical
node. `Equal` papers over the divergence by comparing IDs, but
mutations to fields like `Behavior` (which `behave` writes through
the canonical pointer) don't propagate to the orphan copies. Four
of the seven bugs reduce to this:

- `eng.LookupDefType` returned `&v` of a local dereference.
- `eng.ResolveDefType` returned `v.Parent` for a bare type literal
  (wrong axis entirely — a bare literal IS its lattice node, not
  the supertype) and `ValueType(v)` then produced `&v`.
- `lang.refineBareHandler` minted user subtypes with `parent =
  &baseType` (a stack-local copy).
- `eng.InstallType` else branch used the same `&bodyType` pattern.

### Fracture B — typed-def reparent is bespoke per kind

`defTypedHandler` (lang/go/native/native_definition.go) carries four
independent branches that all do the same conceptual thing — "the
constraint says T; rewrap body's Parent as T" — each invented in
isolation:

- Predicate-type branch (`fn [n:Integer …]`) — `out.Parent = def`.
- ObjectType branch (`def x:Person {…}`) — builds an instance via
  `eng.MakeObject`.
- FnUndef branch (`def f:Mapper fn […]`) — `unified.Parent = def`.
- Refine-bare branch (new in this work) — `reparented.Parent = def`
  after walking past intervening user refines to the kernel root.

Three of these silently differ on questions like "what if the body
came from Unify's swap path?" (Unify returns the literal, not the
value, when one side is a strict-subtype literal). The refine-bare
case originally got this wrong and stored the type literal as the
binding instead of the body's payload.

### Fracture C — three resolve helpers, one job

Resolving "name X to a `*Type`" has three implementations:

- `eng.LookupDefType(r, name) *Value` — returns the body of a type
  binding, with my recent canonicalization patch.
- `eng.Registry.ResolveTypedName(name) (Value, bool)` — single-store
  lookup, by Value.
- `eng.ResolveTypeName(name) (*Type, error)` — kernel-name table for
  the static builtin set.

Each consumer (fn-sig parsing, `is`, `inspect`, `make`, `behave`'s
validator) picks one. They disagree subtly on canonical-pointer
identity and on whether user bindings shadow builtins.

## 2. The unified design

Three primitives, every consumer routes through them.

### 2.1 `r.CanonicalType(name string) *Type`

The single name → canonical `*Type` resolver. Consults the dynamic
DefTable first (so a user `def Foo …` shadows the builtin lookup
that lives in `TypeTable.byName`), then the kernel name table, then
returns nil. Always returns the canonical lattice pointer — the one
registered in `TypeTable.byID` — so `Behavior` mutations through it
reach every consumer that compares by ID.

`ResolveTypedName`, `LookupDefType`, and `ResolveTypeName` all
become thin shims over this single helper or are removed outright.

### 2.2 `eng.CanonicalType(t *Type, r *Registry) *Type`

The `*Type` canonicalizer. Given a `*Type` (possibly a stack copy
from `NewTypeLiteral` or `&body`), return the canonical pointer
from the lattice via `r.Types.LookupByID(t.ID)`. Falls back to the
input when there is no canonical (degenerate roots, test fixtures
with empty IDs). Every site that historically grabbed `&v` of a
type-literal Value routes through here.

### 2.3 `eng.ReparentValue(body Value, def *Type) Value`

The single typed-def reparent primitive. Copies the body, sets
`Parent = def`, preserves `Data`/`Eval`/`Pos`/`Quoted`/`Carrier`,
and returns. Used by every `defTypedHandler` branch (predicate,
object instance, FnUndef, refine-bare). Codifies the invariant that
"reparent preserves payload" so the Unify-swaps-to-literal trap I
hit on refine-bare can never re-emerge.

### 2.4 The `refine` constructor protocol

Today `refineBareHandler` and `def Foo X` (plain alias) are
indistinguishable at the binding site. The fix in this branch added
a side-channel (mint an anonymous subtype with empty Name; let
InstallType detect "Origin=UserDef && Name=''"). That works but it
sneaks intent through field heuristics.

The cleaner protocol: `refineBareHandler` returns a wrapped
`Value` whose `Data` carries a dedicated marker payload
(`RefinePrefab{Anon *Type}`). `InstallType` matches on the marker
type via the sealed `Payload` interface (no string-name probing) and
either renames-and-binds (for refine output) or takes the alias path
(for any other bare type literal). Migration adds one variant to
`payload.go`'s direct-variant list.

## 3. Migration map

Files touched, in implementation order. Each step ships its own
tests; the suite stays green at every step.

| Step | What | Files |
|---|---|---|
| 1 | `CanonicalType` helpers + replace canonicalization hot spots | `eng/util.go`, `eng/fn_params.go`, `eng/core_type.go`, `lang/native/native_type.go` |
| 2 | `ReparentValue` helper + reparent-callsite consolidation | `eng/util.go`, `lang/native/native_definition.go` |
| 3 | Resolve unification | `eng/registry.go`, `eng/fn_params.go`, `eng/types.go`, every consumer |
| 4 | `RefinePrefab` payload + protocol cleanup | `eng/payload.go`, `eng/core_type.go`, `lang/native/native_type.go` |
| 5 | CLAUDE.md updates capturing the new discipline | `eng/go/CLAUDE.md`, `lang/go/CLAUDE.md` |

## 4. Tests

The refine-bare scalar suite added in this branch
(`lang/spec/behave.tsv` Section 9 + `lang/test/typed_def_test.go`'s
`TestRefineBareSubtype*` and `TestUserComparatorOnRefineParent`)
already exercises every path that surfaced a bug. The refactor must
keep all of these green. Where the unified design produces a
behavior that differs from a current test expectation, the
deliberate change is documented in the commit and the test
expectation updated.

## 5. Non-goals

- The Comparison & Ordering subsystem (TYPE-ORDERING.0) is already
  unified — one cascade, one Rank scheme. This refactor does not
  touch it.
- The Payload sealing (TYPE-DECOUPLING.0) is complete; no payload
  variant inventory changes except the new `RefinePrefab`.
- No new user-visible word, no surface-syntax change.
