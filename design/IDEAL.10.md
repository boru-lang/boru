# Ideals — Type-Kinds as First-Class Registered Descriptors

> **Removal note (2026-07):** the `Array` kind used as an example Ideal
> throughout was removed; disregard `Array` in the Ideal enumerations
> below (FlexList — a Node, not an Ideal — covers the mutable-list role).
> The `Object` kind remains as the internal Ideal, but it now
> instantiates **class** types (there is no user-facing `Object`
> type/word; `class` is the surface form).

Status: implemented — Phases 1-3 (the `type` / `make` pipeline,
`Enabled` enforcement, the conformance contract) plus a host kind
family, `Tensor` / `Matrix` / `Vector`, in `lang/go/modules/matrix`.
Sections §1-§12 are the original design; **§13 records what building
it changed.**
Branch: `claude/review-architecture-go-practices-SoLNa`.

This document proposes the **Ideal**: a struct that describes a
type-*kind* (Object, Record, Table, Array, …) and lives in a
per-`Registry` registry with the same first-class, dynamically
controllable status as a capability. It builds directly on the
`def` / `make` / `type` surface established by `legacy/TYPE-UNIFORM.10.ignore`.
It does **not** change behaviour by itself.

## 1. Motivation

The `type` word (post `TYPE-UNIFORM`) is boru's single type
constructor. Its handler, `typeHandler` in
`lang/go/native/native_type.go`, dispatches with a hard-coded
if-else chain:

```go
if base.Data == nil && base.Parent.Equal(TObject) { … objectHandler }
if IsObjectType(base)                            { … objectWithParentHandler }
if base.Data == nil && base.Parent.Equal(TRecord) { … recordHandler }
if base.Data == nil && base.Parent.Equal(TTable)  { … tableHandler }
return … "type: base must be Object, Record, Table, or an object type"
```

The same closed set of kinds is re-enumerated, by hand, in at least
four kernel sites:

- **construction** — `typeHandler` (above);
- **instantiation** — `MakeHandler` / `MakeWithOpts` in `core_make.go`;
- **unification** — `unifyMaps`'s record / options / typed-map forks
  in `unify.go`;
- **rendering** — the per-kind branches of `listFormatBehavior` /
  `mapFormatBehavior` in `coretype_list_map_behaviors.go`.

Three consequences:

1. **Adding a kind means forking the kernel.** A host that wants a
   `Graph`, `Matrix`, or `Tensor` type-kind cannot — the kinds are Go
   branches, not data.
2. **The per-kind logic is duplicated.** The Object/Record/Table
   review (commit `512b584`) consolidated the safe overlaps
   (`formatFieldBag`, `unifyFieldBags`) but the structural pattern —
   "every kind reimplements construct / instantiate / unify / format"
   — remains.
3. **There is no principled home** for relationships between kinds
   (Options ≈ Record-without-order; Table ≈ collection-of-Record).

The goal: give type-kinds the status capabilities already have —
**registered, named, dynamically enabled/disabled, host-extendable** —
so the kernel dispatches over data instead of branching over a closed
set.

## 2. The concept

An **Ideal is a type-kind made first-class** — the type *constructor*
turned into registered data. The name is deliberate: an Ideal is the
archetype of a family of types, the Platonic "Object-ness" that every
`Object/Foo` is an instance-of-an-instance of.

Three layers, cleanly separated:

| Layer | What it is | Example | Lifetime |
|---|---|---|---|
| **Ideal** | a type-kind | `Object`, `Record`, `Table` | registry entry |
| **`*Type`** | one type identity | `Object/Foo` | minted, lattice node |
| **`Value`** | one instance | `make Foo {…}` | runtime value |

An Ideal is one level above `TypeBehavior`. `TypeBehavior`
(`Match`/`Format`/`Equal`, plus the optional `Comparer`/`Hasher`/
`Walker`) is a *per-`*Type`* vtable — it answers "how does *this type*
behave". An Ideal is a *per-kind* vtable — it additionally answers
"how do you *construct* a type of this kind, and *instantiate* one".
§6 describes how the two relate.

## 3. The `Ideal` struct

> **Implemented differently — see §13.** The struct shipped smaller
> than this, and `Refines` became a kind-lattice edge, not the
> func-inheritance pointer described below.

```go
// An Ideal is the archetype of a type-kind — the first-class,
// registered descriptor for a family of types. It is the type
// constructor turned into data.
type Ideal struct {
    Name    string  // "Object", "Record", "Table", "Array", "Resource"
    Root    *Type   // lattice anchor for this kind (TObject, TRecord, TList…)
    Refines *Ideal  // optional: a specialisation of another Ideal (§8)
    Enabled bool    // dynamic on/off, exactly like a capability slot

    // --- type-level ops: drive `refine ‹base› arg` ---
    Construct func(base, arg Value, r *Registry) (Value, error)
    Unify     func(a, b Value) (Value, bool)

    // --- value-level ops: drive `make`, `is`, rendering, `.field` ---
    Instantiate func(typ, data Value, r *Registry) (Value, error)
    Match       func(v, typ Value) bool
    Format      func(v Value) string
    Field       func(v, key Value) (Value, bool)
    Equal       func(a, b Value) bool

    // --- declarative metadata: lets shared helpers stay generic ---
    Inherits    bool  // supports subtyping?      Object yes, Record no
    OrderStrict bool  // member order is identity? Record yes, Options no
}
```

The operations are **func fields, not a Go interface**. This is
intentional and is what makes an Ideal a *struct* rather than a type:

- a `nil` func means "not overridden" — fall through to `Refines`'s
  func, then to a kernel default. An Ideal can be partial.
- it keeps Ideals first-class data: an Ideal is a value you can
  construct, copy, store, and (§4) register at runtime.

The `Inherits` / `OrderStrict` flags are *declarative* metadata: the
kernel's shared helpers (`unifyFieldBags(a, b, orderStrict)` already
exists) read them, so two field-bag Ideals can share one helper and
differ only by a boolean.

## 4. The `IdealRegistry` and dynamic control

```go
type IdealRegistry struct {
    byName  map[string]*Ideal
    ordered []*Ideal   // deterministic iteration / root-resolution order
}
```

Held on the `Registry`, beside the existing slots:

```go
type Registry struct {
    Defs         *DefTable          // bindings (TYPE-UNIFORM Phase 4)
    Types        *TypeTable         // the type lattice
    Capabilities *CapabilityRegistry
    Ideals       *IdealRegistry     // ← new
    …
}
```

It is a **separate registry from `Capabilities`**, not a shared one.
Capabilities and Ideals have the same *status* — registered, named,
per-`Registry`, dynamically controllable — but a different *nature*:
a capability is a host *effect* slot (file I/O, clocks); an Ideal is a
pure type-system *descriptor*. Conflating their registries would
couple the type system to the host-effects surface. "Same status, not
same registry."

Dynamic control mirrors capabilities exactly:

- **add** — `r.Ideals.Register(&Ideal{…})`; a host package installs a
  `Graph` kind at module-init time.
- **disable / enable** — `Ideal.Enabled = false`; a sandboxed
  sub-engine ships without `Table` (no SQL surface). `refine Table …`
  on a disabled Ideal errors cleanly: *"the Table kind is not
  available in this registry"*.
- **replace** — swap the `Object` Ideal for a stricter variant.
- **per-`Registry` isolation** — each `Registry` owns its
  `IdealRegistry`; a module sub-registry inherits a copy, so kind
  availability is scoped just like word availability.

## 5. Data-driven dispatch — the `*Type.Ideal` back-pointer

> **Implemented differently — see §13.** The `*Type.Ideal`
> back-pointer was not adopted: structural kinds are
> payload-discriminated, so dispatch is by `Accepts` predicate.

The one structural move that removes the hard-coded chain: add a
back-pointer from a type identity to its Ideal.

```go
type Type struct {
    …
    Ideal *Ideal  // the Ideal that governs this kind (nil for plain scalars)
}
```

Every type an Ideal constructs carries that Ideal. The bare root
literals (`Object`, `Record`, `Table`) carry the kernel Ideals from
registration. Dispatch then becomes a **pointer-follow**, not a
search:

```
refine X arg      →  X.Parent.Ideal.Construct(X, arg, r)
make T data       →  T.Parent.Ideal.Instantiate(T, data, r)
unify(a, b)       →  a.Parent.Ideal == b.Parent.Ideal → ideal.Unify(a, b)
v.String()        →  v.Parent.Ideal.Format(v)
v is T            →  T.Parent.Ideal.Match(v, T)
```

Worked example — object inheritance, today four hard-coded branches,
becomes one rule:

```
def Animal (refine Object {legs:Integer}) ; Object literal → objectIdeal.Construct
def Dog    (refine Animal {breed:String}) ; Animal.Parent.Ideal == objectIdeal
                                          ; → objectIdeal.Construct(Animal, …) → subtype
make Dog {legs:4 breed:"x"}               ; Dog.Parent.Ideal.Instantiate
```

`typeHandler` shrinks to: resolve the base, follow `.Ideal`, call
`Construct` (or error if the base names no Ideal). The
construction/instantiation/unification/format **logic itself does not
move** into the Ideal at first — the Ideal's funcs initially just
*point at* the existing `objectHandler` / `recordHandler` /
`tableHandler` / `unifyFieldBags` / `formatFieldBag`. The refactor is
"make dispatch data-driven", not "rewrite the kinds".

## 6. Relationship to existing machinery

> **Extended in practice — see §13.** A host Ideal whose constructed
> types round-trip through `def` also needs the `eng.HostTypeBody`
> marker; `ExtensionPayload` alone is opaque to the kernel's type
> machinery.

**`TypeBehavior`.** An Ideal *produces* the `Behavior` for the types
it mints: `Construct` calls `r.Types.MintType` and sets
`def.Behavior` to a thin shim forwarding `Match`/`Format`/`Equal` to
the Ideal. `TypeBehavior` stays as the per-`*Type` interface the
kernel's dispatch points already consult — but for Ideal-governed
types it has a single source of truth (the Ideal) instead of being
wired up separately from the constructor. Non-Ideal types (plain
scalars, `Date`, plugin leaf types) keep supplying their own
`Behavior` directly, unchanged.

**The payload seal.** `Value.Data` is the sealed `eng.Payload`
interface; only `eng`-defined structs carry `payloadMarker()`. This
forces a deliberate asymmetry:

- **kernel Ideals** (Object/Record/Table/Array) keep their typed
  payloads (`ObjectTypeInfo`, `RecordTypeInfo`, …) — they are defined
  in `eng`.
- **host-registered Ideals** must carry their type-info and
  instance-info inside `ExtensionPayload{Body any}` — the existing
  escape hatch — and unwrap it inside their own funcs.

This is acceptable because it is exactly the plugin-type story
already in use (`ExtensionPayload` is *designed* as the
kernel-doesn't-inspect-this hatch), and because the kernel never
inspects an Ideal-governed payload directly — it always goes through
the Ideal's vtable. It is, however, a stated invariant: builtin and
host Ideals are equivalent in *capability* but not in *payload
representation*.

**The lattice.** Ideals sit *on top of* `TypeTable`, not instead of
it. `Construct` still calls `MintType` to obtain a lattice identity
(ID, parent chain). The TYPE-UNIFORM Phase 4 collapse already made
`TypeTable` purely the lattice; Ideals are the constructor layer
above it.

## 7. Accepted constraint — semantics are pluggable, syntax is uniform

Ideals make a type-kind's **construction and semantics** pluggable.
They do **not** make its **surface literal syntax** pluggable, and
**this is a deliberate, accepted limitation, not a deficiency to fix
later.**

The parser hard-wires the structural literals: `{…}` → map,
`[…]` → list, `[:T]` / `{:T}` → typed list/map. A host Ideal does
**not** get a bespoke literal. It is reached only through the uniform
words:

```
refine Graph {nodes:List edges:List}   ; construct  — the constructor word
make GraphType {nodes:[…] edges:[…]}   ; instantiate — the `make` word
g .nodes                               ; access — the dotted accessor
```

We accept this because:

- **One grammar.** boru has exactly one parser — a single jsonic
  grammar (`eng/go/parser`). Per-plugin grammar extension means
  runtime grammar mutation: a large complexity, ambiguity, and
  audit-safety cost for a small ergonomic gain.
- **The uniform surface is sufficient.** `refine Name arg` + `make` +
  `.field` is a complete, readable interface for any structural kind.
  A `Graph` reached via `refine Graph …` is not meaningfully worse than
  one reached via a hypothetical `‹…›` literal.
- **Closed syntax keeps the language learnable and auditable.** The
  set of literal forms a reader must know stays fixed regardless of
  which Ideals a registry has loaded.

Consequence, stated plainly: a dynamic Ideal is **semantically
first-class and syntactically uniform**. It participates fully in
`type` / `make` / `is` / `typeof` / rendering / unification; it never
gets its own punctuation. An Ideal is a *kind*, not a *grammar
extension*.

## 8. Ideal vs. plain Type — the boundary principle

> **See §13** for how `Refines` actually behaves once built — a
> kind-lattice edge with an availability rule, not func-inheritance.

`Refines` makes it tempting to model every domain archetype as an
Ideal (`Resource`, `Entity`, `Vector`, `Matrix`, `Tensor`). Resist
that until it pays for itself. The principle:

> An Ideal is warranted only when a kind needs **its own
> construction semantics or its own instance representation**.
> Otherwise it is a plain named type.

- `Object` / `Record` / `Table` / `Array` — clearly Ideals: each has
  a distinct constructor *and* a distinct instance shape.
- `Resource` / `Entity` — today they are object *types*
  (`installResourceTypes`). They stay types. `Resource` becomes an
  Ideal **only** if it needs construction rules `Object` cannot
  express (identity minting, a mandatory `kind` discriminator,
  lifecycle hooks). `Refines: objectIdeal` exists for exactly that
  case — an Ideal that *is* Object plus extra invariants — but it is
  a sharp tool: every `Refines` edge is a kind the language's readers
  must now know.

This keeps the Ideal registry small and meaningful rather than a
dumping ground for every named shape.

## 9. Testing

Ideals are unusually testable, and the design should lean on that.

**Per-Ideal unit tests.** Each Ideal is a vtable of near-pure
functions. `Construct`, `Instantiate`, `Unify`, `Format`, `Match`
each test in isolation with hand-built `Value`s — no engine run
needed.

**The Ideal conformance contract.** The highest-value test artefact:
a single parameterised suite — `TestIdealConformance(ideal *Ideal)`
— run against *every* registered Ideal, asserting the invariants a
kind must satisfy regardless of who wrote it:

- `Construct` then `Instantiate` round-trips a representative value;
- an instance `is` its own constructed type;
- `Unify(t, t)` is `t` (reflexivity); `Unify` is order-insensitive
  up to the Ideal's `OrderStrict` flag;
- `Format` is total and non-empty — it never panics, including on
  type literals and carriers (the kernel's panic-prevention rule);
- a disabled Ideal makes `type`/`make` fail with a clear error, not
  a panic or a silent wrong answer.

Because the suite is parameterised over the registry, a host's new
Ideal is held to the *same* contract as the kernel's — the contract
*is* the definition of "a valid Ideal".

**The existing spec suites become the regression gate.** Ideals do
not change the `type` / `make` surface, so every row in
`lang/spec/*.tsv`, `eng/spec/*.tsv`, and the `lang/go/test` TSV
suites must pass unchanged. Phase 1 (§10) is *defined* as "zero spec
diff" — the entire existing suite is the proof that data-driven
dispatch is behaviour-preserving.

**Registry tests.** Enable/disable, add, replace, per-`Registry`
isolation, deterministic iteration order — mirroring the existing
capability-registry tests.

**The end-to-end plugin proof.** One test that registers a *toy*
Ideal entirely from the test package (a `Pair` kind, say — two
ordered fields, no inheritance), then drives `type` / `make` / `is` /
`typeof` / rendering against it and asserts it behaves
indistinguishably from a kernel kind. This is the test that proves
the abstraction actually holds at its boundary — that "host Ideal"
is not a second-class citizen in practice.

**Stability gates.** If an Ideal's `Construct` mints lattice types,
the `fixedid_stability_test.go` discipline applies to any
externally-registered Ideal with stable IDs.

## 10. How Ideals help HKT

Higher-kinded types — abstracting over type *constructors* rather
than types — were discussed earlier as a long-horizon question. Full
HKT is a large language-design commitment boru has not made. Ideals do
not make it; they **remove the structural blocker** and supply the
substrate. Concretely:

**The precondition.** You cannot abstract over type constructors
until type constructors are *things*. Today they are Go if-else
branches inside `typeHandler`. After Ideals they are **named entries
in `r.Ideals`** — reified, enumerable, queryable. That is the
necessary first step, and it is the whole of what this proposal
commits to.

**What Ideals then give a future HKT layer, for free:**

- **A runtime-queryable kind on every value.** `v.Parent.Ideal` lets
  boru code branch on "what kind of structure is this" without
  knowing the concrete type. That is *ad-hoc kind-polymorphism*
  already: a generic `empty`, `size`, or structural `map` word can
  consult the Ideal and act uniformly across `Table` / `Array` /
  `List`.
- **A kind lattice.** The `Refines` chain *is* a hierarchy of kinds.
  "Generic over any `Collection`" becomes "any Ideal whose `Refines`
  chain reaches `collectionIdeal`" — bounded kind-polymorphism,
  expressed with machinery this proposal already includes.
- **The substrate for parameterised constructors.** `Table` is,
  conceptually, `Collection(Record)` — a constructor applied to a
  constructor. Ideals do not deliver that application, but because a
  constructor is now data (`Ideal`) and `Construct` already takes a
  base argument, *composing* Ideals becomes a library problem, not a
  kernel-grammar problem.

**The boru-idiomatic shape of HKT.** boru checks types against a
runtime lattice and treats static analysis as a best-effort
check-mode pass. The Ideal-enabled form of HKT matches that
philosophy: kind-polymorphism is **runtime dispatch on
`v.Parent.Ideal`**, and kind-checking is a **check-mode consultation
of the Ideal registry** — not a System-F-ω elaboration. Ideals make
*pragmatic, runtime-flavoured* HKT reachable; they do not drag in
academic HKT.

**Honest scope — what Ideals do *not* provide.** Signature-level
("static") HKT would still need: kind variables in `Signature`
(a slot that says "any `F[Integer]` where `F` is a `Collection`
Ideal"); kind-aware matching in `eng/go/match.go`; and a surface
syntax to *write* a kind-abstracted word. Ideals are
necessary-not-sufficient for that. The correct claim is precise:
**Ideals keep the HKT door open and lay its foundation; they are the
first step, not the destination.**

## 11. Migration plan

Phased, green at every step — the same discipline as TYPE-UNIFORM.

- **Phase 1 — reify, no behaviour change.** Add `Ideal`,
  `IdealRegistry`, `r.Ideals`, and `*Type.Ideal`. Register
  Object/Record/Table/Array as Ideals whose funcs *point at* the
  existing handlers. Convert `typeHandler` (and the `type`-level
  dispatch in `unify`/`Format`) to follow `.Ideal`. **Acceptance:
  zero diff in any spec suite.**
- **Phase 2 — value-level ops.** Move `Instantiate` and the
  value-level vtable (`Match`/`Field`/`Equal`) onto Ideals; convert
  `MakeHandler` and the accessor/`is` paths. Larger blast radius.
- **Phase 3 — open the registry.** Expose `r.Ideals.Register` to host
  modules; add `Enabled` enforcement and the conformance suite as the
  gate for host Ideals.
- **Later, on concrete need only.** `Refines`, `Resource`-as-Ideal,
  parameterised Ideals. Not before a real kind demands them.

## 12. Resolved decisions and open questions

**Resolved**

- Syntax is uniform, not pluggable — Ideals are kinds, not grammar
  extensions (§7).
- Ideals live in their own registry, separate from capabilities;
  same status, different nature (§4).
- The vtable is a struct of func fields (`nil` → inherit/default),
  not a Go interface (§3).
- Ideal vs. Type is governed by the "own construction or own
  representation" principle; `Resource`/`Entity` stay types for now
  (§8).
- Ideals are the *foundation* for HKT, explicitly not HKT itself
  (§10).

**Open**

- Cross-kind unification: today "records only unify with records" is
  implicit. With Ideals it must be explicit — likely "fail unless one
  Ideal declares a coercion to the other".
- Whether the Ideal fully *subsumes* `TypeBehavior` or merely
  *produces* it (§6 proposes "produces" — the lighter change).
- Whether host Ideals minting stable-ID lattice types need a reserved
  FixedID range, as external builtin types do.
- Partial application of Ideals (`Collection(Record)`) — deferred to
  the HKT horizon (§10), noted here so the struct shape does not
  preclude it.

## 13. Implementation findings

Phases 1-3 (the `type` / `make` pipeline, `Enabled` enforcement, the
conformance contract) are implemented, as is a host kind family —
`Tensor`, `Matrix` and `Vector` in `lang/go/modules/matrix`. Sections
§1-§12 are the original design; this section records where building it
changed that design.

**The `Ideal` struct, as built.** Smaller than §3's proposal:

```go
type Ideal struct {
    Name        string
    Enabled     bool
    Refines     *Ideal
    Accepts     func(base Value) bool
    Construct   func(base, arg Value, r *Registry) ([]Value, error)
    Instantiate func(typ, data Value, r *Registry) ([]Value, error)
}
```

The value-level vtable (`Unify`, `Match`, `Format`, `Field`, `Equal`)
and the declarative metadata (`Root`, `Inherits`, `OrderStrict`) are
not built yet — they join when `unify` and rendering become
data-driven. `Construct` and `Instantiate` return `[]Value` to match
the kernel handler convention.

**Dispatch is by `Accepts` predicate, not the §5 back-pointer.**
Records, tables, options and the shaped tensor types are
*payload-discriminated*: they share a base `*Type` (`Map`, `List`,
`Matrix`) and are told apart by their payload. A `*Type.Ideal`
back-pointer cannot distinguish a record from a plain map, so it was
not adopted. Dispatch is a scan of `Accepts` predicates
(`IdealRegistry.Match` / `For`). With a handful of kinds the scan is
free; the predicate, not the pointer, is the contract. A shaped type
(`refine Matrix {rows:3 cols:3}`) is likewise payload-discriminated — it
is not minted, so it carries no per-type FixedID.

**`Refines` is a kind-lattice edge, not vtable inheritance.** §3 said
a nil func "falls through to `Refines`'s func"; that never happened.
`Matrix` and `Vector` each carry their own `Construct` / `Instantiate`
because their data forms genuinely differ (a flat list, nested lists,
a `{shape data}` map). What a kind family shares is *helper functions*
and a *factory* — `tensorConstruct(kind, vtype)` builds three Ideals
whose funcs are closures over the kind. `Refines` instead carries:

- the **availability chain** — `Ideal.available()` walks `Refines`,
  reporting a kind usable only when it and every kind it refines are
  enabled, so disabling `Tensor` disables `Matrix` and `Vector`;
- an explicit, queryable **kind hierarchy** — the §10 HKT substrate,
  now real.

§3 and §8 should describe `Refines` this way and drop the
func-inheritance framing.

**A refinement ripples into every dispatch site.** The Phase-3
disabled-kind error tested `!Ideal.Enabled`. With `Refines` that is
wrong: a kind can be unavailable because its *base* is disabled while
its own `Enabled` is true. `typeHandler`, `MakeHandler` and
`MakeObjHandler` now gate on `available()`.

**Host Ideals need `HostTypeBody`, not just `ExtensionPayload`.** §6
said a host carries its payload in `ExtensionPayload` and "the kernel
never inspects an Ideal-governed payload directly." The second half is
false for *constructed types*: `def Foo (refine Matrix {rows:3 cols:3})`
routes a host type through `InstallType`, and `typeof` plus `make`'s
target resolution must recognise it *as a type* — but a bare
`ExtensionPayload` is opaque, so `IsTypeBody` returns false. The fix
is a kernel-provided embeddable marker, `eng.HostTypeBody`: a host
embeds it in the struct it uses for a constructed type, and
`IsTypeBody` / `TypeOf` / `isTypeLike` detect it generically — still
without inspecting the concrete shape. The seal holds; it needed one
bit of structure exposed: "this payload is a type, not an instance".

**A kind's lattice placement interacts with `make`'s overloads.**
`make` is a set of typed overloads, two of them keyed on metatypes
(`TScalarType`, `TObjectType`). The tensor family first sat under
`Scalar/Number` — inherited from the legacy `Matrix` type — which made
a bare `Matrix` literal match the scalar-cast overload `[TScalarType
Any]` and never reach the Ideal dispatch. Moving the family to
`Node/Tensor` fixed it and is correct anyway: a tensor is a structured
container, not a scalar. For any future kind: place its root `*Type`
so its metatype does not collide with `make`'s typed overloads —
containers belong under `Node`.

**A host kind is import-gated.** A type *identity* is global —
`Matrix` registers into `eng.Builtin` at package init, so the literal
always parses. A *kind* is per-`Registry`: `r.Ideals` is populated
when the module is imported (`BuildMatrixModule` calls
`registerTensorIdeals`). There is no global Ideal table. Before
`import "boru:matrix"`, `Matrix` names a type but `refine Matrix` /
`make Matrix` raise "the Matrix type-kind is not available" — the
disabled-kind path. This is §4's per-`Registry` isolation made
concrete: identity is global, *capability* is registry-scoped.

**`def` / `make` resolution is generic.** A host kind installed
through `InstallType` reuses the resolution records already use —
`stepWord` resolves a type-bound name (since the type-node fusion,
[legacy/TYPE-REPRESENTATION.1.ignore](legacy/TYPE-REPRESENTATION.1.ignore) §6, to the minted
lattice NODE; `make` recovers the stored body via
`core.TypeContentOf`), `MakeHandler` dispatches on it. Beyond the
`HostTypeBody` marker, no per-kind kernel plumbing was needed.
