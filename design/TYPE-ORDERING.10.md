# TYPE-ORDERING.10 — The Value Lattice & Comparison Total Order

This document records the design of boru's value ordering: the lattice
that places every Value in a total preorder, the cascade
`CompareValues` uses to settle a pair, and the deliberate anomalies
we accepted. It is the canonical reference for `tcmp` / `sort` and the
`Comparer` capability seam.

> **Surface split (post-`tcmp`).** `CompareValues` — everything below —
> is the **unrestricted total order**, unchanged. It is surfaced
> directly by `tcmp` and used by `sort` and the collection words. The
> everyday ordering words `cmp` / `lt` / `lte` / `gt` / `gte` add a
> shallow guard *on top*: they accept a pair only when it is the same
> type or resolves via a same-family Comparer (stage 1 below), and
> raise `[boru/incomparable]` for pairs that would settle only by the
> Rank fallback (stage 2). So this document describes what `tcmp`
> computes; the restricted words compute the same number but refuse the
> cross-family cases. Equality (`eq`/`neq`/`deq`) is independent of this
> cascade and stays total. See WAT-AUDIT Exhibit P and
> `lang/spec/compare-restrict.tsv`.

## TL;DR

* Every Value has a `Parent` pointing at its lattice node.
  `typeof v == v.Parent`. The lattice has `Any` at the root of the
  main hierarchy; `None` and `Never` are sui-generis roots kept apart
  so the `Parent.Equal(TNone)` shortcut in the dispatch path doesn't
  silently match every value.
* Every type carries a unified `Rank` integer. Kernel types get a
  positional slot in their branch's `n·10¹⁰` band; user-defined
  (`MintType`) and external (`RegisterExternalBuiltin`) types share a
  per-branch external band one increment up (Scalar
  `2·10¹⁰`→`2.1·10¹⁰`, Node `3·10¹⁰`→`3.1·10¹⁰`, Ideal
  `4·10¹⁰`→`4.1·10¹⁰`, Word `5·10¹⁰`→`5.1·10¹⁰`, Type
  `6·10¹⁰`→`6.1·10¹⁰`).
* `CompareValues` first walks the LCA chain looking for a `Comparer`
  capability. If one is found (Number, String, Boolean, Atom, Word,
  Scalar) it owns the result — same-family pairs order by content.
  Only when no Comparer applies does the Rank cascade run.
* `compareTypes` (the Rank fallback) cascades Rank → depth → name → ID.
* Same-Parent concrete values are settled by `compareStructural`:
  Lists by length-then-element-wise, Maps by length-then-keyset-lex-
  then-value-wise.
* **Order is a total preorder, not a strict total order.** Three
  documented anomalies are accepted as deliberate trade-offs (see
  *Anomalies*); they all reduce to "type literals share an equivalence
  class with the family's zero-valued inhabitant."

## The Lattice — full table

`Rank` is the discriminator the comparator keys on for cross-family
pairs. The kernel positional bands step `n·10¹⁰` apart per branch;
external/user bands sit one increment up so they always rank after
the kernel positional subtree in the same branch.

```
RANK             TYPE PATH                  REPRESENTATIVE LITERAL          NOTES
─────────────────────────────────────────────────────────────────────────────────────────────
─── root band (1·10¹⁰) ──────────────────────────────────────────────────────────────────────
11_000_000_000   Any                        Any                              top — matches anything
12_000_000_000   None                       none      None                   the unit; type literal
13_000_000_000   Never                      Never                            empty / bottom

─── kernel Scalar band (2·10¹⁰) ─────────────────────────────────────────────────────────────
20_000_000_000   Scalar                     Scalar
20_100_000_000   Scalar/Atom                Atom      red/q                  atom literal via /q
20_200_000_000   Scalar/Boolean             Boolean   false   true           false < true
20_300_000_000   Scalar/Number              Number
20_310_000_000   Scalar/Number/Integer      Integer   -1  0  1  2  42        positional + per-value
20_320_000_000   Scalar/Number/Decimal      Decimal   0.0  3.14              per-value
20_400_000_000   Scalar/String              String
20_410_000_000   Scalar/String/EmptyString  EmptyString   ''                 sole inhabitant
20_420_000_000   Scalar/String/ProperString ProperString  'apple'  'banana'  lex
20_500_000_000   Scalar/Path                Path      a/b/c   /abs/path      volume → abs-first → lex → prefix-first (NUR012)

─── external/user Scalar band (2.1·10¹⁰) ────────────────────────────────────────────────────
21_000_000_000   Scalar/Time                (make Date '2026-05-23')         external — Time family
21_000_000_000   Scalar/Time/Date           (make Date …)
21_000_000_000   Scalar/Time/DateTime       (make DateTime …)
21_000_000_000   Scalar/Time/Instant        (make Instant …)
21_000_000_000   Scalar/Time/TimeOfDay      (make TimeOfDay …)
21_000_000_000   Scalar/Time/Duration       (make Duration 'PT1H')
21_000_000_000   Scalar/Time/Duration/CalDuration  (make CalDuration 'P1Y')
21_000_000_000   Scalar/Time/Duration/ClkDuration  (make ClkDuration 'PT1H')
21_000_000_000   Scalar/Time/Timezone       (make Timezone 'UTC')
21_000_000_000   Scalar/<user>              def Positive (refine Integer …)  ties: depth → lex name

─── kernel Node band (3·10¹⁰) ───────────────────────────────────────────────────────────────
30_000_000_000   Node                       Node
30_100_000_000   Node/List                  List      []   [1 2 3]           length → element-wise
30_110_000_000   Node/List/Args             (args stack frame)
30_200_000_000   Node/Map                   Map       {}   {a:1, b:2}        length → keys → values
30_210_000_000   Node/Map/Inspect           inspect add                       inspection-result map

─── external/user Node band (3.1·10¹⁰) ──────────────────────────────────────────────────────
31_000_000_000   Node/<user>                def Pair (refine List …)

─── kernel Ideal band (4·10¹⁰) ──────────────────────────────────────────────────────────────
40_000_000_000   Ideal                      Ideal
40_100_000_000   Ideal/Class                def P (class {x:1})
40_110_000_000   Ideal/Resource             Resource
40_111_000_000   Ideal/Resource/Entity      make Entity {kind:'api' …}
40_300_000_000   Ideal/Record               refine Record [x:Integer]
40_400_000_000   Ideal/Options              make Options {x:1, y?:String}
40_500_000_000   Ideal/Error                (raised error value)
40_600_000_000   Ideal/Store                (sys-store layer)
40_610_000_000   Ideal/Store/System         __sys
40_700_000_000   Ideal/Table                refine Table Foo

─── external/user Ideal band (4.1·10¹⁰) ─────────────────────────────────────────────────────
41_000_000_000   Ideal/Fetch                make FetchRequest {…}
41_000_000_000   Ideal/Fetch/Request
41_000_000_000   Ideal/Fetch/Response
41_000_000_000   Ideal/Interval             make Interval PT1S
41_000_000_000   Ideal/Tensor               make Tensor [[1 2][3 4]]
41_000_000_000   Ideal/Tensor/Matrix        refine Matrix {rows:2 cols:2}
41_000_000_000   Ideal/Tensor/Vector        refine Vector {n:3}
41_000_000_000   Ideal/Timeout              make Timeout PT5S
41_000_000_000   Ideal/<user>               def Person refine Record […]

─── kernel Word band (5·10¹⁰) ───────────────────────────────────────────────────────────────
50_000_000_000   Word                       add   foo                         bare-word values
50_100_000_000+  Word/__FW … Word/__IN/__DC  (internal runtime markers — forward, paren, end,
                                              fn, mark, move, module, def-cleanup)

─── external/user Word band (5.1·10¹⁰) ──────────────────────────────────────────────────────
51_000_000_000   Word/<user>                (no user path today; reserved)

─── kernel Type band (6·10¹⁰) ───────────────────────────────────────────────────────────────
60_000_000_000   Type                       Type
60_100_000_000   Type/Function              fn [Integer Integer [add 1]]
60_200_000_000   Type/FunctionSignature     fnsig [Integer Integer]
60_300_000_000   Type/Disjunct              Integer tor String tor None
60_310_000_000   Type/Disjunct/Enum         enum [red green blue]

─── external/user Type band (6.1·10¹⁰) ──────────────────────────────────────────────────────
61_000_000_000   Type/<user>                def M (Integer tor String)       user types parented in the Type band
```

*(Update, 2026-08-20: the type-node fusion
([legacy/TYPE-REPRESENTATION.1.ignore](legacy/TYPE-REPRESENTATION.1.ignore) §6, the `tcmp`
flip) changed the user-band exemplar. `def Positive (Integer gt 0)` now
mints a node parented at `Integer`, so it orders in the SCALAR band —
just after `Integer`, strictly below the concrete integers:
`sort [Positive Integer 0 5 -3]` → `[Integer Positive -3 0 5]`. Only
user-named types whose parent is itself in the Type band — named
disjuncts, named function signatures — rank here at 6.1·10¹⁰.)*

## In boru, literals are types

A concrete value's `Parent` IS its type — there is no separate "value
inhabits type" indirection. `42`, `'hello'`, `true` each have their
own lattice identity (the `Scalar/Number/Integer/42` /
`Scalar/String/ProperString/hello` / `Scalar/Boolean/true` paths).
The Rank is inherited from the leaf type, so values within a leaf
share a Rank and tie-break via the family `Comparer` on numeric /
lex content. `typeof 5 == Integer` is one parent hop up the chain;
`typeof Integer == Number` is one more.

## The `CompareValues` cascade

```
CompareValues(a, b):
  1. aType, bType = ValueType(a), ValueType(b)
       - Data == nil && !Carrier → &v (the value IS the lattice node)
       - otherwise              → v.Parent
  2. lca = lowestCommonAncestor(aType, bType)
  3. Walk lca up the parent chain looking for a Comparer:
        ┌─────────────────────┬───────────────────────────────────┐
        │ numberComparer      │ on TNumber → float64 magnitude    │
        │ stringComparer      │ on TString → UTF-8 lex            │
        │ booleanComparer     │ on TBoolean → false < true        │
        │ atomComparer        │ on TAtom → name lex               │
        │ wordComparer        │ on TWord → rendered-form lex      │
        │ scalarComparer      │ on TScalar → comparePaths fallback│
        └─────────────────────┴───────────────────────────────────┘
       Each returns ErrNoComparer when its payload-extraction
       doesn't apply (DepScalar in a numeric Comparer, etc.) so the
       walk can resume.
  4. No Comparer found → compareTypes(aType, bType)
       Rank → typeDepth → lex Name → lattice ID
  5. Types identical and concrete → compareStructural:
        List → compareListElems (length, then element-wise CompareValues)
        Map  → compareMapEntries (length, then sorted-keys lex,
                                  then per-key CompareValues)
        else → strings.Compare(CanonValue(a), CanonValue(b))
```

The Comparer cascade firing *before* the Rank cascade is what
preserves traditional numeric semantics across the Integer / Decimal
positional split: even though `Decimal` (Rank `20.32·10⁹`) ranks
above `Integer` (Rank `20.31·10⁹`), `1.1 lt 2` is `true` because the
LCA `Number` has the numeric `Comparer` and it owns the comparison —
`AsNumber(1.1) = 1.1`, `AsNumber(2) = 2.0`, `1.1 < 2.0` → `-1`.

## Node ordering

Two concrete nodes of the same Parent are settled by
`compareStructural`:

### Lists

```
[1 2 3] cmp [1 2 4]     → -1   (lengths 3 = 3; (3 cmp 4) = -1)
[1 'a'] cmp [1 'b']     → -1   (per-position Comparer)
[1.1 2] cmp [1 2.1]     →  1   ((1.1 cmp 1) =  1, stops the walk)
[1 2 3] cmp [1 2 3]     →  0   (all elements equal)
[1 2 3] cmp [1 2 4 5]   → -1   (length 3 < length 4 — element walk skipped)
[]      cmp [1]         → -1
[9]     cmp [1 2 99]    → -1   (length wins)
```

### Maps

```
{a:1 b:2} cmp {a:1 b:3}  → -1   (same length, sorted keys [a,b] match;
                                  (2 cmp 3) = -1 stops the walk)
{a:1 b:2} cmp {b:2 a:1}  →  0   (declaration order doesn't matter)
{a:1 b:2} cmp {a:1 c:2}  → -1   (same length, sorted keys differ:
                                  'b' < 'c')
{a:1 b:2} cmp {a:1}      →  1   (length 2 > length 1)
{}        cmp {a:1}      → -1
```

### Nested

`CompareValues` recurses, so `[[1] [2]] cmp [[1] [3]] = -1` resolves
to `CompareValues([1], [2])` at position 1 → `-1` via the list rule
→ `1 vs 2` via the Number Comparer.

## Cross-family ordering — Rank decides

When no Comparer applies (the LCA walk reaches Any without finding
one), `compareTypes` settles by Rank, then depth, then name, then
ID. This produces the macro ordering:

```
Booleans < Numbers < Strings < Paths < Times          (Scalar band)
                <
Lists < Maps                                          (Node band)
                <
Objects < Records < Options < Errors < Stores < Tables (Ideal band)
                <
Words                                                 (Word band)
                <
Functions < FunctionSignatures < Disjuncts < Enums    (Type band)
```

Example: `true tcmp 5 = -1` (Boolean Rank `20.2·10⁹` < Integer Rank
`20.31·10⁹`); `5 tcmp 'a' = -1` (Integer < String); `'a' tcmp [1] =
-1` (String band < List band). (These are cross-family, so `tcmp` —
the restricted `cmp`/`lt` raise `[boru/incomparable]` here.)

## Type-literal-first rule (the family-zero anomaly, now fixed)

Earlier, a scalar type literal compared *within its own family*
fell through to the family Comparer, which read its `Data == nil`
payload as a zero value and tied it with the family's zero-valued
inhabitant (`Integer cmp 0 → 0`, `String cmp '' → 0`, `Boolean cmp
false → 0`). That violated antisymmetry — distinct lattice nodes
collapsed into the same equivalence class — and reduced the order
to a preorder.

The fix is a two-line preamble at the top of every family Comparer:

```go
// litVsConcreteOrder — type literal sorts FIRST.
if c, ok := litVsConcreteOrder(a, b); ok { return c, nil }
// Both literals — order by lattice Rank.
if a.Data == nil && b.Data == nil {
    return litVsLitOrder(a, b), nil
}
// Both concrete — proceed with the family's value compare.
```

Result:

```
Integer cmp 0       → -1     # Integer type literal < the value 0
Number  cmp Integer → -1     # by Rank (Number 20.3·10⁹ < Integer 20.31·10⁹)
Integer cmp Decimal → -1     # by Rank
String  cmp ''      → -1     # String type literal < the empty string
Boolean cmp false   → -1     # Boolean type literal < the value false
Atom    cmp foo/q   → -1     # Atom type literal < every concrete atom
Path    cmp (make Path ['a']) → -1   # Path literal < every concrete path
```

The rule applies **only within a family** — `scalarCompareBehavior`
(the catch-all on `Scalar` that handles cross-family pairs) does
NOT apply it, so cross-family ordering remains Rank-only:

```
true tcmp Integer → -1   # Boolean Rank 20.2·10⁹ < Integer Rank 20.31·10⁹
5    tcmp String  → -1   # Integer Rank < String Rank
```
(Cross-family → shown with `tcmp`; `cmp`/`lt` would raise
`[boru/incomparable]`.)

The Path family runs through `comparePaths` (no Comparer on `TPath`
itself; the LCA walk reaches `Scalar`); `comparePaths` carries its
own `litVsConcreteOrder` check at the top so the rule applies
inside the Path family without leaking into other scalar
cross-family pairs.

### What's still equivalent

After the fix the order is a **strict total order over distinct
lattice nodes**, with one deliberate value-level equivalence:

```
1 cmp 1.0       → 0        # cross-leaf numeric magnitude
1.0 + 0 == 1.0           # consistent with arithmetic
```

Cross-leaf magnitude equality (`Integer 1 ≡ Decimal 1.0`) is
preserved because the alternative — making `1` and `1.0` order
strictly — breaks numeric semantics that every user expects
(`1 + 0 == 1`, both render as `1`, both unify against `Integer`).
This equivalence is confined to the cross-leaf magnitude case and
does not produce the per-family-zero collapse that the type-literal
issue did.

### NaN in the total order

IEEE-754 leaves `NaN` *unordered* (every relational comparison with it
is false), but the total order this document defines must place every
value, or `sort` over a list containing a NaN would violate transitivity
and silently leave it unsorted. So `numberCompareBehavior.Compare` gives
NaN a defined slot: it sorts **after every non-NaN value**
(`-inf < finite < inf < nan`), and two NaNs tie. This is the IEEE
`totalOrder` placement (with a single, unsigned quiet NaN) and is what
`cmp` / `tcmp` / `sort` and the collection words consume.

This is deliberately **separate** from the relational words `lt` / `lte`
/ `gt` / `gte`, which keep the IEEE *unordered* semantics (false whenever
a NaN is involved) via `numericUnordered` in `eng/go/compare.go` — they
do **not** read the total-order slot. `eq` stays false for NaN (so `neq`
is true). See `design/IEEE-754-COMPLIANCE.8.md` Tier 0.

### Signed zeros in the total order

The same two-regime split covers the signed zeros (NUR013). The
recorded `totalOrder` (§5.10) comparison: IEEE orders `-qNaN < -inf <
neg finite < -0 < +0 < pos finite < +inf < +qNaN`; boru's total order
is `-inf < neg finite < -0.0 < +0 < pos finite < inf < nan` —
conforming on signed zeros and on the placement of the single
observable quiet NaN, with NaN sign/payload ordering a recorded
acceptance (unobservable: one `nan` literal, rendering collapses the
sign).

Concretely, `numberCompareBehavior.Compare` breaks a zero-magnitude
tie by `math.Signbit`: the Float `-0.0` sorts **strictly before every
positive zero** (`-0.0 cmp 0.0 → -1`, `sort [0.0 -0.0] → [-0.0
0.0]`). Only the Float `-0.0` slots as `-0` — Integer `0` and the Big
zeros (`0d0` / `0n0`, including a negative-zero decimal spelling,
which the exact `big.Rat` domain normalises away) slot as `+0`, so
the tiebreak applies identically in the float-projection path and the
big-rat path and the `0` / `0d0` / `-0.0` triangle stays transitive
(`0 tcmp -0.0 → 1`, `0d0 tcmp -0.0 → 1`, `0 tcmp 0d0 → 0`).

The relational words are carved out via `signedZeroPair` (beside
`numericUnordered` in `eng/go/compare.go`): `lt` / `lte` / `gt` /
`gte` treat every ±0 pair as EQUAL per IEEE §5.11 (`-0.0 lt 0.0 →
false`, `-0.0 lte 0.0 → true`), and `eq` / `deq` stay true (Go float
`==`). So the total order distinguishes a pair `eq` equates — the
exact inverse of the NaN asymmetry (`nan cmp nan → 0`, `nan eq nan →
false`); both are the one recorded two-regime split. The deq-based
collection words (`unique`, `group`) are unaffected — they never read
the total order for equality.

### `none` and `Never`

`none` and `Never` are degenerate roots with no Comparer-bearing
ancestor; comparisons against them go straight through the Rank
cascade:

```
none tcmp 5     → -1     (None Rank 12·10⁹ < Integer band)
none tcmp Any   →  1     (None Rank 12·10⁹ > Any Rank 11·10⁹)
Never tcmp Any  →  1     (Never Rank 13·10⁹ > Any Rank 11·10⁹)
```

`none tcmp none = 0` (same type — `cmp` accepts it too); `none` and any
other value compare strictly by Rank, so a cross-type pair needs
`tcmp` (`cmp`/`lt` raise `[boru/incomparable]`).

### Summary of order properties

| Property | Holds? |
|---|---|
| Reflexive — `a cmp a = 0` | ✓ |
| Transitive — `a < b ∧ b < c ⇒ a < c` | ✓ |
| Total — every pair compares | ✓ |
| Antisymmetric over distinct lattice nodes | ✓ |
| Antisymmetric over distinct values | ✗ for cross-leaf numeric magnitude (`1 ≡ 1.0`) — deliberate |

## Implementation pointers

| Concern | File | Key symbol |
|---|---|---|
| Lattice declaration | `eng/go/typetable.go` | `builtinDecls`, `Rank` |
| External/user Rank band | `eng/go/typetable.go` | `externalBandFor` |
| Total-order tiebreak | `eng/go/compare_types.go` | `compareTypes`, `rankOf` |
| LCA + Comparer walk | `eng/go/compare.go` | `CompareValues`, `lowestCommonAncestor` |
| Per-family Comparers | `eng/go/compare_scalar_behaviors.go` | `numberCompareBehavior`, `stringCompareBehavior`, … |
| Structural compare | `eng/go/compare_types.go` | `compareStructural`, `compareListElems`, `compareMapEntries` |
| Spec coverage | `lang/spec/compare.tsv` | tests for every cell above |

## Verification

`lang/spec/compare.tsv` carries the test matrix: every scalar cross
product, the node ordering rules, the cross-numeric chain, the
anomalies, and an explicit transitivity battery. The spec runner
asserts each row; any drift fails CI.
