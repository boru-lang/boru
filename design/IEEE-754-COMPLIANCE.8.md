# IEEE-754 Compliance — what `Float` would require

Status: **Tier 0 done; Tier 1 done; Tier 2 proposed (impractical on wasm)**. This audits boru's
`Float` (binary64) against IEEE 754-2019 and states what full and
partial conformance would require. **Tier 0 (the NaN-comparison fix) is
done**, and the `inf`/`-inf`/`nan` literals + matching rendering from
Tier 1 landed alongside it — see [Tier-0 status](#tier-0-status). The
pre-fix transcripts in §"The three deviations" are retained as the record
of what was fixed. It is a companion to
[NUMERIC-TOWER](legacy/NUMERIC-TOWER.0.ignore) and
[INTEGER-OVERFLOW-STRATEGY](INTEGER-OVERFLOW-STRATEGY.5.md); the
WAT-AUDIT Exhibits H (eq-but-not-substitutable) and L (Decimal-is-float)
are the surface symptoms this document's gaps explain.

Every transcript was reproduced against `cmd/go/bin/boru`. A clean NaN is
made with `def nan (-8.0 0.5 pow)` (√−8).

## TL;DR

The **arithmetic core is already mostly conformant** — it rides on Go's
`float64`, which is hardware IEEE binary64: `+ − × ÷` and `sqrt` are
correctly rounded (round-ties-to-even), and `±0`, `±∞`, subnormals, and
NaN all propagate. What is *not* conformant splits into three buckets:

1. **Deviations boru deliberately/accidentally introduced** — div/mod by
   zero raise an error instead of producing `±∞`/NaN, and the comparison
   words are mutually inconsistent on NaN (this one is a genuine bug that
   also breaks `sort`).
2. **Missing surface** — no `Inf`/`NaN`/`-0` literals; the float-to-string
   formatter is not conformant (no exponential form; special values can't
   round-trip).
3. **Missing IEEE machinery** — no sticky exception flags, no
   directed-rounding modes, no `remainder`/`fma`/`totalOrder`/
   `roundToIntegral`/signaling-NaN. Some of this is blocked by Go/wasm
   not exposing FPU state (see [Hard blockers](#hard-blockers)).

So "IEEE-754 compliance" for boru is really two different targets:
**(A) IEEE arithmetic *semantics*** (the values and basic-op rounding) —
largely present, a few fixable deviations away; and **(B) full
clause-by-clause *conformance*** (flags + rounding modes + the operation
catalogue) — a much larger effort, partly impractical on the wasm target.

## Conformance audit

| IEEE 754-2019 requirement | boru today | Status |
| --- | --- | --- |
| **binary64 format** (§3.4) | Go `float64` | ✅ conformant |
| **Correctly-rounded `+ − × ÷`** (§5.4) | hardware, via the Float handler path | ✅ |
| **Correctly-rounded `sqrt`** (§5.4) | `MathUtil.sqrt` → `math.Sqrt` | ✅ |
| **`±0`** distinct, `−0 == +0` (§5.11) | `0.0 -1.0 mul → -0.0`; `-0.0 eq 0.0 → true`; `-0.0 lt 0.0 → false` (relationals keep ±0 equal despite the totalOrder slot below) | ✅ |
| **±∞ propagation** (§6.1) | overflow → `+Inf.0` / `-Inf.0` | ✅ (produced by overflow) |
| **Subnormals** (§3.4) | stored/propagated (`5e-324` survives) | ✅ |
| **NaN propagation** (§6.2) | `√−8 → NaN`, propagates through ops | ✅ (quiet only) |
| **`x ÷ 0 → ±∞`, `0÷0 → NaN`** (§7.3) | Float: ✅ (±inf/nan); Integer: hard error by design; no sticky flag | ✅ value / ❌ flag |
| **NaN comparison = unordered** (§5.11) | `eq/neq/lt/lte/gt/gte` all correct (false for NaN) | ✅ Tier 0 |
| **`totalOrder` predicate** (§5.10) | `cmp`/`tcmp`/`sort`: `-0.0` strictly before `+0` (NUR013) and NaN greatest — conforming for signed zeros and for the single observable qNaN; NaN sign/payload ordering has no boru surface (one `nan` literal, rendering collapses the sign) | ✅ Tier 0 (NaN sign/payload: recorded acceptance — unobservable) |
| **Directed rounding modes** (§4.3) | only the hardware default (ties-to-even) | ❌ |
| **Sticky exception flags** (§7) | none | ❌ |
| **`remainder` (round-to-nearest)** (§5.3.1) | `MathUtil.remainder` (`mod` stays truncated `fmod`) | ✅ |
| **`fma`** (§5.4.1) | `MathUtil.fma` (single-rounding `a·b+c`) | ✅ |
| **`roundToIntegral*`** (§5.9) | all 5 directions: trunc/floor/ceil/round/round-even | ✅ (returns Integer, not Float) |
| **min/max with NaN** (§9.6) | `min/max` ignore NaN, order-independent (2008 `minNum`) | ✅ 2008-style |
| **Decimal↔binary string conversion** (§5.12) | parse correctly-rounded (`ParseFloat`); format shortest + scientific for extremes; round-trips | ✅ |
| **`Inf`/`NaN` literals & canonical strings** (§5.12) | `inf`/`-inf`/`nan` literals; render to same tokens (round-trip) | ✅ Tier 0 |
| **Signaling NaN** (§6.2) | not exposed (Go doesn't) | ❌ |

## The three deviations boru introduced

### 1. Division / modulo by zero errors instead of ±∞ / NaN — ✅ RESOLVED

Originally both leaves errored. The chosen resolution is option (1) below
(*Float is IEEE, Integer is strict*): **Float** div/mod by zero now
follows IEEE-754, while **Integer** stays a hard error (there is no
integer infinity).

```
1.0 0.0 div    # inf                  (was: error)
-1.0 0.0 div   # -inf
0.0 0.0 div    # nan
7.0 0.0 mod    # nan
1 0.0 div      # inf                  (a Float operand routes via the Float path)
1 0 div        # error: division by zero   (Integer — unchanged, by design)
```

What is still *not* IEEE here: there is no sticky `divByZero`/`invalid`
status flag (Go does not expose FPU flags — Tier 2). The result *values*
are conformant; the *signalling* is not.

### 2. NaN comparison is internally incoherent (genuine bug) — ✅ RESOLVED (Tier 0)

*(Transcript below shows the pre-fix behaviour; see [Tier-0 status](#tier-0-status) for the resolution — `lte`/`gte` are now unordered-false and `cmp`/`sort` use a NaN-last total order.)*

```
nan nan eq    # false   ✅ IEEE
nan 5.0 neq   # true    ✅ IEEE
nan 5.0 lt    # false   ✅ IEEE (unordered)
nan 5.0 gt    # false   ✅ IEEE (unordered)
nan 5.0 lte   # true    ❌ IEEE says false
nan 5.0 gte   # true    ❌ IEEE says false
nan 5.0 cmp   # 0       ❌ NaN is not "equal" to 5.0
[3.0 nan 1.0 2.0] sort   # [3.0 NaN.0 1.0 2.0]  ❌ unsorted — total order broken
```

`eq/neq/lt/gt` already implement the IEEE *unordered* rule (all false,
except `neq` true). But `lte`/`gte`/`cmp` are derived from the total-order
projection where NaN collapses to "equal to everything" (`cmp → 0`).
This is self-contradictory — `lt` and `eq` are both false yet `lte` is
true — and it silently corrupts `sort`/`min`/`max`/`dedup` for any list
containing NaN (the comparator violates transitivity and antisymmetry,
so the result order is undefined). This bug exists **independently of any
IEEE ambition** and should be fixed regardless. See
`design/TYPE-ORDERING.10.md` — the fix is to give NaN a defined slot via
IEEE `totalOrder` for `cmp`/`sort`, while keeping the relational
predicates unordered.

### 3. The float-to-string formatter is not conformant — ✅ RESOLVED

*(Special values now render as the parseable `inf`/`-inf`/`nan` literals (Tier 0), and extreme magnitudes use scientific notation (`|x| >= 1e21` or `< 1e-10`) while the everyday range stays plain. All forms re-parse to the same Float.)*

`FormatFloat` (`eng/go/value.go:903`) uses `strconv.FormatFloat(f,'f',-1,64)`
— shortest mantissa, but **never exponential** — then appends `.0`:

```
1.7976931348623157e308   # renders as a 309-digit integer + ".0"
5e-324                   # renders as 0.000…(323 zeros)…5
+Inf.0  /  -Inf.0  /  NaN.0   # special values — and none can be re-read
```

Two conformance problems: (a) extreme magnitudes are unreadable and
non-canonical (IEEE §5.12 expects a shortest form that round-trips —
`'g'` format, with exponent); (b) special values render in a form that
**cannot be parsed back** (there are no `Inf`/`NaN` literals), so
round-trip is broken for them. Finite values *do* round-trip (the long
decimal re-parses), just unreadably.

## Missing surface: literals & I/O

- **`Inf` / `NaN` / `-0.0` literals.** `Inf`, `NaN`, `Infinity` are all
  `undefined_word`. IEEE programs need to *write* these. Add lexer
  literals (or words `inf`/`nan`) and a signed-zero literal that the
  parser preserves.
- **Correctly-rounded literal parse.** `Float` literals currently take
  the `float64` jsonic produced. The Phase-0a integer fix parses integers
  from exact digits; the analogous guarantee for `Float` is that the
  decimal→binary64 conversion be correctly rounded (round-ties-to-even).
  `strconv.ParseFloat` is; **verify jsonic's lexer is, or parse the Float
  literal from its source digits** the same way Phase 0a does for ints.
- **Conformant rendering.** Switch `FormatFloat` to a shortest-round-trip
  form with exponent for out-of-range magnitudes, and render special
  values as parseable tokens (`inf`/`-inf`/`nan`) — co-designed with the
  literals so print∘parse is identity.

## Missing machinery

These are the IEEE operations/attributes with no boru surface at all.
Most of the *operations* are thin wrappers over Go's `math` package and
are cheap; the *attributes* (flags, rounding modes) are the hard part.

**Operations (cheap — wrap `math`):**
- `remainder` (IEEE §5.3.1, round-to-nearest) — distinct from the current
  `mod`/`fmod`. `math.Remainder`.
- `fma a b c` (§5.4.1, correctly-rounded `a·b+c`) — `math.FMA`.
- `totalOrder` (§5.10) — the NaN-defined total order; reuse for `cmp`/`sort`.
- the full `roundToIntegral{TiesToEven,TiesToAway,TowardZero,
  TowardPositive,TowardNegative}` set (§5.9) — today only `round/trunc/
  floor/ceil` exist (and only in the math module).
- classification + sign ops (§5.7): `isNaN`, `isInfinite`, `isFinite`,
  `isNormal`, `isSubnormal`, `classify`, `copySign`, `nextAfter`,
  `scalb`, `logb` — `math.IsNaN`/`IsInf`/`Signbit`/`Nextafter`/`Ldexp`/
  `Logb`/`Copysign`.
- 2019 `minimum`/`maximum` (NaN-propagating) alongside the current
  NaN-ignoring `min`/`max`.

**Attributes (hard — see blockers):**
- The five **rounding-direction modes** (§4.3) with dynamic selection
  (e.g. a `with-rounding 'toward_zero [ … ]` block, mirroring the
  proposed `with-decimal` context in NUMERIC-TOWER).
- The five **sticky exception flags** (§7: invalid, divByZero, overflow,
  underflow, inexact) — raisable, testable, clearable — plus the §8
  alternate-exception-handling blocks.
- **Signaling NaN** (§6.2) and NaN payloads.

## Hard blockers

Full clause-by-clause conformance is **impractical on boru's targets**,
not just laborious:

- **Go does not expose the FPU status/control word.** There is no
  portable way to read the five sticky flags or to change the rounding
  direction at runtime. Achieving §4 (rounding modes) and §7 (flags)
  would require either (a) a software binary64 implementation (a
  soft-float library — large, slow), or (b) per-architecture assembly to
  drive `MXCSR`/`FPCR`, which is non-portable and **not available under
  WebAssembly** — and boru ships a wasm playground (`wpg/`). So directed
  rounding and exception flags cannot be delivered on wasm without
  soft-float.
- This is why most "IEEE-754" languages (Python, JS, Java, Go itself)
  implement IEEE *arithmetic and values* but do **not** expose the full
  flags/rounding-mode machinery either. boru would be in good company
  targeting semantic conformance rather than the full standard.

## The philosophical tension

boru's design treats `1 div 0` as a loud error (and Phase 0 made integer
overflow an error too). IEEE-754 treats `1.0 / 0.0` as a *normal*
operation returning `+∞` with a flag. These are fundamentally opposed for
the float path. Three coherent resolutions:

1. **Float is IEEE, Integer is strict.** `Float` div/mod by zero returns
   `±∞`/NaN (IEEE); `Integer` div/mod by zero stays an error (no integer
   ∞ exists, so this is forced anyway). This is the cleanest split and
   matches how most languages behave — and it is consistent with the
   leaves being genuinely different types.
2. **Keep errors, drop the IEEE claim for those ops.** Document that boru
   Float arithmetic is IEEE binary64 *except* that division/modulo by
   zero are errors rather than ∞/NaN. Honest, zero behavioural change.
3. **Mode switch.** A `with-float 'ieee [ … ]` context that opts into
   ∞/NaN locally; strict-error elsewhere.

Recommendation: **(1)**. It is the least-surprising for a Float
(everyone else returns ∞/NaN), it removes a special case rather than
adding one, and it leaves Integer's fail-loud behaviour intact.

## What "required" means, by tier

**Tier 0 — fix the bug (do regardless of any IEEE goal).** ✅ **Done.**
The NaN comparison incoherence (§Deviation 2): `lt`/`lte`/`gt`/`gte` now
honour the unordered rule (false for NaN) via `numericUnordered`
(`eng/go/compare.go`), and `cmp`/`tcmp`/`sort` use a total order where
NaN sorts greatest (`numberCompareBehavior.Compare`,
`eng/go/compare_scalar_behaviors.go`), so a list with a NaN sorts
deterministically instead of being left unsorted. `eq`/`neq` were already
correct. As part of the same change the `inf`/`-inf`/`nan` reserved
literals were added (parser, mirroring `true`/`false`/`none`) and
`FormatFloat` now renders specials as those parseable tokens (was the
unparseable `+Inf.0`/`NaN.0`), so print∘parse is identity. Verified by
`lang/spec/float-special.tsv`, `eng/go/compare_nan_test.go`, and
`eng/go/parser/parse_test.go::TestParseSpecialFloatLiterals`.

> **`min`/`max` with NaN — resolved (Tier 1).** `MathUtil.min`/`max` now
> ignore NaN (IEEE-2008 `minNum`/`maxNum`): a NaN operand is dropped, so
> the result is order-independent (`min nan 5` and `min 5 nan` are both
> 5); only when every operand is NaN is the result NaN. This treats NaN
> as "missing", which suits a data/query language.

> **Signed zeros in the total order — resolved (NUR013).** The
> `totalOrder` (§5.10) comparison, recorded: IEEE orders `-qNaN < -inf <
> neg finite < -0 < +0 < pos finite < +inf < +qNaN` (with NaN
> sign/payload ordering); boru's total order is now `-inf < neg finite <
> -0.0 < +0 < pos finite < inf < nan`. Conforming on **signed zeros** —
> `numberCompareBehavior.Compare` breaks a zero-magnitude tie by
> `math.Signbit`, in both the float-projection path and the exact
> big-rat path, so `-0.0 cmp 0.0 → -1` and `sort [0.0 -0.0] → [-0.0
> 0.0]`. Only the Float `-0.0` slots as `-0`; Integer `0` and the Big
> zeros (`0d0`, `0n0` — including a negative-zero decimal spelling,
> which `big.Rat` normalises away) slot as `+0`, keeping the order
> transitive across leaves. Conforming on **NaN placement** for the
> single observable quiet NaN; NaN *sign/payload* ordering is a recorded
> acceptance — boru has one `nan` literal and rendering collapses the
> sign (`nan -1.0 mul → nan`), so the divergence is unobservable. The
> relational words are carved out (`signedZeroPair` beside
> `numericUnordered` in `eng/go/compare.go`): `lt`/`lte`/`gt`/`gte`
> treat every ±0 pair as equal per §5.11 (`-0.0 lt 0.0 → false`,
> `-0.0 lte 0.0 → true`), and `eq`/`deq` stay true (Go float `==`) —
> so the total order now distinguishes a pair `eq` equates, the exact
> inverse of the NaN asymmetry (`nan cmp nan → 0`, `nan eq nan →
> false`). Both asymmetries are the same recorded two-regime split.
> Verified by `lang/spec/float-special.tsv` (signed-zero section),
> `lang/spec/edge-scalars-2.tsv` §1, and `eng/go/compare_zero_test.go`.

**Tier 1 — IEEE arithmetic *semantics* (the realistic compliance target).**
- ✅ Decision (1) above: `Float` div/mod by zero → `±∞`/NaN, not error;
  Integer stays strict.
- ✅ `inf`/`-inf`/`nan` literals + `FormatFloat` rendering them back, so
  print∘parse round-trips for specials (landed with Tier 0).
- ✅ `remainder`, `copysign`, `nextafter`, and the `is-nan`/`is-inf`/
  `is-finite`/`signbit` classifiers as words in `boru:math-util`.
- ✅ Correctly-rounded `Float` literal parse — the dotted-literal path
  now parses the exact source digits with `strconv.ParseFloat`
  (round-ties-to-even) rather than trusting jsonic's float64.
- ✅ `fma`; `round-even` (the IEEE roundTiesToEven mode, completing
  trunc/floor/ceil/round/round-even); `scalb`/`logb`.
- ✅ A defined `min`/`max` NaN policy — IEEE-2008 `minNum`/`maxNum`
  (ignore NaN, order-independent: `min nan 5 → 5`, both NaN → NaN).
- ✅ Exponential `FormatFloat` for extreme magnitudes — `|x| >= 1e21` or
  `< 1e-10` render in `'e'` form; the everyday range (incl. small
  decimals like `0.00001`) is untouched. Both forms re-parse to the same
  Float. The conservative magnitude threshold (rather than Go's `'g'`,
  whose `exp < -4` rule would have rewritten `0.00001` → `1e-05`) keeps
  the common range stable, so no existing spec changed.

Tier 1 is complete; boru's Float now behaves like every other mainstream
language's double — which is what "IEEE-754" colloquially means.

**Tier 2 — full clause conformance (largely impractical on wasm).**
- The five dynamic rounding-direction modes (§4) via a scoped context.
- The five sticky exception flags + alternate handling (§7–8).
- Signaling NaN (§6.2).
These need soft-float or non-portable FPU control and are **out of reach
under WebAssembly**; pursue only with an explicit soft-float decision and
a non-wasm scope.

## Non-goals

- Decimal (base-10) IEEE formats — that is the `Decimal` leaf in
  NUMERIC-TOWER, a separate axis from binary64 conformance.
- binary16/binary32/binary128 — boru has one float width (binary64).
