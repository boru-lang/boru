# VALUE-PATTERN-DISPATCH — signature exhaustiveness & the variable-reference precision gap

**Status: investigation report (point-in-time).** Records why *signature*
exhaustiveness — the overload-level analogue of the shipped `case`
exhaustiveness (`design/case-exhaustiveness.0.md`) — is not a self-contained
addition, so a future scoped effort can pick it up without re-deriving it.

## Context

`case` exhaustiveness landed in July 2026 (`design/case-exhaustiveness.0.md`,
`lang/go/native/case_exhaustive.go`): `boru check` requires a `case`'s clauses
to cover the scrutinee's static type. That feature is deliberately scoped to
the `case` construct. This note covers the *other* dispatch site the same idea
motivates — **function value-pattern overloads** — and the precision gap that
blocks it.

## Goal

Given value-pattern overloads over a closed-set (enum) parameter:

```
def render fn [ [red/q] [String] ["R"]  [green/q] [String] ["G"] ]   # 'blue' omitted
```

an uncovered call (`blue`) raises `no_signature` at runtime. Signature
exhaustiveness would catch that at check time — a **model-level** error like
`case_not_exhaustive`, NOT a `RuntimeMirror`. (A finding emitted when `render`
is merely *defined* has no guaranteed-executed call to mirror, and
`RuntimeMirror` diagnostics deliberately do not refuse compilation
— `eng/go/registry.go` — which is the opposite of the gate we want. `main`
classifies its sibling `case_not_exhaustive` the same way, for the same
reason.)

## What already exists

The checker has a call-site partition, `disjunctPartitionReturns`
(`eng/go/carrier.go`), reached as a dispatch-failure rescue in `engine.go`
(the "strict disjunct rescue"). When a disjunct-typed argument reaches a word,
it enumerates the alternatives (`disjunctCombos` → `alternativeCarriers`),
first-matches each against the overloads (`firstMatchingSig`), and emits
**`partial_dispatch`** for any alternative that reaches no overload. It works
today for **type** unions:

```
def IorS (Integer tor String)
def only-int fn [[n:Integer] [Integer] [n add 1]]
def use fn [[x:IorS] [Integer] [only-int x]]
use 5
# check: partial_dispatch: only-int has no overload for alternative (String)
#        of a declared union parameter …
```

## The partition fix (ready, correct, confined — but not sufficient alone)

The partition does NOT fire for enums for two reasons, both traced to exact
sites, both fixable with a confined change that leaves the type-union path
byte-identical:

1. **`alternativeCarriers` widens concrete members.** `flattenAlternatives`
   maps a concrete alternative (`red`) to its type node (`Atom`), so every enum
   member collapses to an `Atom` carrier and the value identity a value-pattern
   overload needs is gone. Fix: expand a strict disjunct preserving CONCRETE
   alternatives as their value, widening only bare-type / nested alternatives.
   Type-union alternatives are type literals, so that path is unchanged.

2. **`firstMatchingSig` is pattern-blind.** It type-matches only, so it cannot
   tell `[red/q]` from `[blue/q]` — every atom member matches the first
   `Atom`-typed sig. Fix: for a CONCRETE arg, additionally require the sig's
   concrete `Pattern` to `Unify` with it, mirroring `patternsOk`'s
   concrete-scalar branch (`eng/go/match.go`). Non-concrete args (type
   carriers, the type-union path) keep the prior type-only behaviour.

With both, the partition correctly computes enum coverage — an incomplete set
yields `partial_dispatch: render has no overload for alternative (blue) …`
naming the exact uncovered member.

These changes are correct and low-risk (confined to the disjunct-partition
subsystem; type unions unaffected). They are NOT sufficient on their own — see
the blocker.

## The real blocker: value-pattern dispatch through a variable reference

The value never reaches the partition AS a disjunct in real programs, and even
a concrete member fails to dispatch through a variable. The decisive
comparison (against `cmd/go/bin/boru check`):

| Program | Result |
|---|---|
| `render red/q` — direct literal | clean |
| `def c red/q ; render c` — **untyped** variable | false `no_signature` "got (Atom)" |
| `def c:Color red/q ; render c` — typed variable | identical false `no_signature` |

The binding `c` stores **concrete `red`** (value retained — verified). But at
the **call site**, resolving the reference `c` yields an abstract `Atom`
carrier, so the value-pattern sig `[red/q]` cannot match. This is a **general
value-pattern-dispatch precision gap through any variable reference** — it is
NOT enum-specific and reproduces with a plain `def`.

Two distinct paths, two distinct problems:

- **Abstract path: reached, but the partition can't compute.** Uncalled fn
  bodies ARE analysed at construction — `checkFnBodyAtConstruction`
  (`eng/go/core_helpers.go`) runs `AnalyseFnBody` for every freshly installed
  non-generic body with GENERALIZED parameter carriers — so
  `def paint fn [[c:Color] [String] [render c]]` is checked even uncalled, and
  the generalized `Color` disjunct does reach the partition at `render c`
  (a strict-disjunct dispatch is observable there). What blocks a finding here
  is the partition's inability to compute enum coverage — the two bugs the fix
  above addresses (concrete-member widening + pattern-blind `firstMatchingSig`)
  — NOT reachability. (An earlier draft wrongly attributed this to fn bodies
  going unanalysed; corrected per PR #291 review.)
- **Concrete path: a separate precision gap.** A top-level `render c`, and the
  per-call-shape re-analysis when a fn is called with a concrete argument
  (`paint red/q` re-runs the body with `c` = `red`), resolve the variable
  reference to an abstract `Atom` carrier rather than the concrete value — so a
  value-pattern sig can't match and a false `no_signature` is raised even when
  the overload set is COMPLETE (reproduced above). This is the general
  variable-reference precision gap, independent of enums.

(The `case` feature sidesteps both: it works over the clause list's own static
analysis of the scrutinee type, not over overload dispatch — which is why it
could ship self-contained.)

## What a scoped effort would take

1. **Partition fix (confined, the first step).** Preserve concrete alternatives
   in `alternativeCarriers`; add value-pattern awareness to `firstMatchingSig`.
   This should surface the finding on the ABSTRACT construction-time path — the
   first thing to confirm, since the disjunct already reaches the partition
   there.
2. **Variable-reference precision (the harder half).** Resolve a reference to a
   known-concrete binding as that value at the call site, so the CONCRETE path
   stops raising false `no_signature`. This touches how every `def` reference is
   typed in analysis and shifts inferred types across the corpus — a core
   checker change, and the broader win (it improves ALL value-pattern dispatch
   through variables, not just enums).

Both are error-severity paths (`no_signature` / `partial_dispatch` gate
`boru check`), deserving their own design + review.

## Recommendation

- Open **value-pattern-dispatch precision through variable references** as a
  scoped effort with its own design pass. Pair the partition fix (recorded
  above) with the precision fix; signature exhaustiveness falls out.
- Until then, `case` (shipped) is the blessed enum-dispatch construct and needs
  none of the above.

## Reproductions

Runnable against `cmd/go/bin/boru check`:

```
# false no_signature via untyped variable (the core gap)
def render fn [[red/q] [String] ["R"] [green/q] [String] ["G"] [blue/q] [String] ["B"]]
def c red/q
render c

# works — direct literal
render red/q
```
