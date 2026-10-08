# REFINE-NEWTYPE-VS-SUBSET.10 — Symmetric refine matching at every type boundary

This document records a semantics decision and refactor for how
`refine` types match values at function **parameter** and **return**
boundaries. It resolves a long-standing param/return asymmetry by
splitting `refine` into the two concepts it was conflating and giving
each a single, symmetric matching rule.

Status: **design + implementation (this branch).**

Sibling docs: `TYPE-UNIFORM.0` (the `def`/`make`/`refine` surface),
`TYPE-CANONICALIZATION.0`, `TYPE-DECOUPLING.0`.

## 1. The asymmetry

A user type used as a fn **parameter** annotation behaved differently
from the same type used as a **return** annotation:

```
def Pos (refine Integer)
def f fn [[x:Pos] [Any] [x]]   f 7            # accepted    → 7
def mk fn [[]     [Pos] [7]]   mk             # REJECTED    → expected Pos, got Integer
```

The param side admitted a bare `Integer 7` for a `Pos` slot; the return
side rejected the identical value. The two boundaries ran *different
predicates*:

- **Param** — `signature.go::sigTypeMatches` → `v.Is(t)` → the type's
  `Behavior.Match`. For a bare refine this was deliberately **lenient**
  (`bareRefineUnifier.Match` returned `v.Parent.Matches(baseType)` —
  any base-family value passed); for a predicate refine it was
  **value-sensitive** (`depScalarUnifier.Match` ran the predicate).
- **Return** — `engine.go` return-check → `value.Parent.Matches(exp)`,
  a raw **nominal** lattice-descent test (`t.Matches(p)` ≡
  `t.IsAncestor(p)` ≡ "t <: p") that ignores the type's Behavior.

So boru ran *value-sensitive/lenient* semantics going in and *strict
nominal* semantics coming out. No language adopts that split on purpose
(see §3). It was masked until recently because user-type return
annotations resolved to `Any` (the prior bug fixed in
`user-types.tsv` §7); once returns resolved correctly, the divergence
surfaced.

## 2. Two concepts wearing one keyword

`refine` builds two genuinely different things, which is why one
matching rule never fit:

| Surface | Concept | Example |
|---|---|---|
| `def Pos (refine Integer)` | **nominal newtype** — a distinct name, no added invariant | `Pos`, `UserId`, `Celsius` |
| `def Big (Integer gt 10)` | **refinement / subset type** — a predicate carves a subset | `Big`, `NonEmpty`, `Percent` |

Their representations already differ in the kernel:

- bare refine → a minted lattice node parented at the base; **no
  payload** (`IsBareTypeNode(body)`), Behavior = `bareRefineUnifier`.
- predicate refine → body carries `DepScalarInfo{Lo,Hi}`
  (`body.IsDepScalar()`), Behavior = `depScalarUnifier`. The predicate
  is self-contained (bounds + comparators), so it evaluates with no
  registry.

*(Update, 2026-08-20: since the type-node fusion
([legacy/TYPE-REPRESENTATION.1.ignore](legacy/TYPE-REPRESENTATION.1.ignore) §6) a NAMED
predicate refine also evaluates to its minted node; the
`DepScalarInfo`-carrying body is the node's recorded content, recovered
via `core.TypeContentOf`. Membership is unchanged — the §5 net table
below still holds verbatim.)*

## 3. What other languages do

No mainstream language uses the param/return asymmetry deliberately;
they pick one discipline and apply it **symmetrically**, splitting by
which concept the type is.

**Nominal newtypes** — distinct name, **explicit construction at every
boundary**, symmetric-strict:

| Language | Construct |
|---|---|
| Haskell | `newtype Pos = Pos Int` (explicit `Pos 42`) |
| Rust | `struct Pos(i32)` (explicit `Pos(42)`) |
| Go | `type Pos int` (explicit `Pos(i)`; untyped constants convert) |
| Scala 3 | `opaque type Pos = Int` (transparent in scope, opaque outside) |

Under the nominal reading boru's *param* was the outlier — Haskell/Rust
reject a bare `42` at a `Pos` parameter too.

**Refinement / subset types** — **value-sensitive**, symmetric: a base
value is admitted wherever the refined type is expected *iff the
predicate holds*, checked statically (solver) or at the boundary:

| Language | Construct | Admission |
|---|---|---|
| F* | `x:int{x > 10}` | SMT proves predicate; symmetric subtyping |
| Liquid Haskell | `{v:Int \| v > 0}` | SMT at each use site |
| Dafny | `type Pos = x: int \| x > 0` | predicate provable; symmetric |
| Ada/SPARK | `subtype Pos is Integer range 1..Max` | same type + constraint; **runtime check**, both directions |

Ada is the sharpest mirror: it has *both* a `subtype` (constraint,
implicit + runtime-checked, symmetric) and a derived `type … is new …`
(distinct, explicit conversion) — exactly the two boru concepts.

## 4. Decision

Split `refine` matching by kind, each **symmetric** across param and
return:

1. **Predicate refine → subset type.** Value-sensitive at *both*
   boundaries: a base-family value is admitted iff it satisfies the
   predicate. Run the predicate at the return boundary too. The value
   keeps its base tag (a subset type is a constraint, not a distinct
   nominal identity) — no reparent on return.

2. **Bare refine → nominal newtype.** Symmetric-strict: only a value
   whose tag *is* the refine type (or a subtype) is admitted, at *both*
   boundaries. A plain base value is not a member — construct one
   explicitly (`def x:Pos 42`, which reparents via Unify, or a
   constructor fn). This **tightens the parameter** to match the
   already-strict return, rather than loosening the return.

This is what the surveyed languages do: subset types value-sensitive
and symmetric (F*/Liquid Haskell/Dafny/Ada-subtype); newtypes
nominal and symmetric (Haskell/Rust/Go/Scala).

## 5. Implementation

The boundaries collapse onto **one predicate**: `value.Is(exp)`. Both
the param matcher (`sigTypeMatches` → `v.Is(t)`) and the return check
ask the same membership question; the per-type `Behavior.Match`
supplies the policy.

1. **`engine.go` return check** — `value.Parent.Matches(exp)` →
   `value.Is(exp)`. Routes the return through the declared type's
   Behavior, exactly like the param. For builtins and objects `.Is`
   coincides with `.Parent.Matches` on concrete values, so they are
   unchanged; predicate refines become value-sensitive; bare refines
   stay nominal (per change 2).

2. **`unify_refine.go` `bareRefineUnifier.Match`** — concrete-value
   branch `v.Parent.Matches(b.baseType)` → `v.Parent.Matches(t)`. The
   bare refine is now nominal: `42.Is(Pos)` is `false`, matching the
   `is` word (which was already nominal) and the return boundary.
   `def x:Pos 42` still mints a `Pos` (Unify/reparent is a separate
   path, unchanged).

3. **`depScalarUnifier.Match`** — unchanged; already value-sensitive
   (base family + `depScalarCheck`). The return check now reuses it via
   `.Is`, so predicate refines are value-sensitive on the way out too.

Net effect per kind:

| | param (before) | param (after) | return (before) | return (after) |
|---|---|---|---|---|
| bare refine `Pos` | lenient (any Integer) | **nominal** | nominal | nominal |
| predicate refine `Big` | value-sensitive | value-sensitive | nominal (rejected valid) | **value-sensitive** |
| builtin / object | nominal | nominal | nominal | nominal |

## 6. Behaviour change & migration

This **changes documented behaviour**: a bare refine no longer admits a
bare base value at a parameter.

```
# before: 99    after: ERROR (signature) — a plain Integer is not a Pos
def Pos (refine Integer) def g fn [[n:Pos] [Integer] [99]] 42 g

# the newtype way to obtain a Pos:
def Pos (refine Integer) def g fn [[n:Pos] [Integer] [99]] def x:Pos 42 x g   # → 99
```

Predicate refines gain symmetric returns:

```
def Big (Integer gt 10) def mk fn [[] [Big] [50]] mk   # → 50    (predicate holds)
def Big (Integer gt 10) def mk fn [[] [Big] [5]]  mk   # → ERROR (predicate fails)
```

Spec coverage: `lang/spec/user-types.tsv` — bare-refine param rows
flip to rejection with newtype-construction positives added; predicate
return rows added (value-sensitive, both polarities); the §7/§8 user
return rows stand.

## 7. Alternatives rejected

- **Coerce/reparent base→refine on return** (Ada `subtype` assignment
  style). Rejected for bare refines: silently stamping `42` as `Pos`
  erases the newtype's whole point (explicit construction). For
  predicate refines it is unnecessary — subset membership doesn't
  require a tag change.
- **Loosen the return to match the lenient param** (the inverse).
  Rejected: it abandons nominal identity for newtypes and keeps the
  unsound "any base value is a Pos" reading the newtype decision
  explicitly overturns.
