# Integer Overflow — Strategy

Status: **Phase 0 implemented; Phase 1 proposed**. This document expands
[WAT-AUDIT](legacy/WAT-AUDIT.5.ignore) Exhibit K ("Integer overflow has two
contradictory silent behaviours"), establishes what boru did before the
fix (which was worse than the audit recorded), surveys what other
languages do, and recommends a phased strategy. **Phase 0 (the real
integer lexer + checked, uniformly-erroring arithmetic) has been
implemented** — see [Phase 0 status](#phase-0-status) below. The §1
transcripts describe the *pre-fix* behaviour they motivated; they no
longer reproduce against the current tree. Phase 1 (arbitrary-precision
`Integer`) remains a proposal.

Notation: examples use the `# returns …` convention (a trailing comment,
the documentation result form — see REFERENCE). `cmd/go/bin/boru` at the
audit commit was used for every transcript.

---

## 1. The problem, fully

Exhibit K names two silent behaviours. There are in fact **three**
distinct silent failures, spanning the lexer and the runtime, and they
disagree with each other on *type*, on *magnitude*, and even on *sign*.

### 1.1 Three behaviours, observed

```
9223372036854775807 1 add        # returns 9223372036854776000.0   (Float, ~maxint)
2 63 pow                         # returns -9223372036854775808     (Integer, wrapped, negative)
4000000000 4000000000 mul        # returns -2446744073709551616     (Integer, wrapped, negative)
```

- **(a) Literal → Float.** `9223372036854775807 1 add` does *not*
  overflow in `add`. The literal `9223372036854775807` is already a
  `Float` before `add` runs (`typeof` confirms), so the float answer
  comes from the literal, not the operator. This is a **lexer**
  defect.
- **(b) `pow` wraps.** The integer `pow` loop
  (`lang/go/native/native_math.go:120`) does raw `int64` multiplication
  and silently wraps two's-complement.
- **(c) `mul` wraps.** `numericBinaryHandler`'s `intFn`
  (`lang/go/native/native_helpers.go:33`) does `b * a` in `int64` and
  wraps. So do `add` and `sub`.

A user cannot predict which of "approximately right but a Float",
"sign-flipped Integer", or "garbage Integer" they get — the behaviour
depends on whether the overflow happens in the *lexer* or the *runtime*,
and on *which* runtime word. That unpredictability is the bug;
the individual wrong answers are symptoms.

### 1.2 The lexer defect is worse than recorded

The audit frames the lexer issue as "large literals fall back to
`Decimal`/`Float`". The real mechanism is more corrosive. boru has no
integer lexer: jsonic parses every numeric token to `float64`, and
`numberValToValue` (`eng/go/parser/parse.go:1215`) decides Integer vs
Float *after the fact* from that float:

```go
func floatToValue(f float64) eng.Value {
    if f == float64(int64(f)) && !math.IsInf(f, 0) && !math.IsNaN(f) {
        return eng.NewInteger(int64(f))     // round-trips through float64
    }
    return eng.NewFloat(f)
}
```

Because `float64` has a 53-bit mantissa, **every integer literal above
2⁵³ has already lost precision before this code runs.** Two failure
modes follow, and the first is silent value corruption with the *correct
type*:

```
typeof 9007199254740993          # returns Integer      — 2⁵³+1, looks fine
9007199254740993                 # returns 9007199254740992   — WRONG: off by one, no signal
9007199254740995                 # returns 9007199254740996   — WRONG: off by three
```

`9007199254740993` is a perfectly valid `int64`, well below `maxint`,
yet boru hands back `…992` typed as `Integer` with no error and no Float
to hint that anything happened. For a data/query language this is the
most dangerous case in the whole audit: a row count or an ID silently
changes value and stays an `Integer`.

The second mode is the audit's "fall back to Float", which only kicks in
right at the boundary where the float→int round-trip itself fails:

```
typeof 9223372036854775806       # returns Float        — maxint−1, can't round-trip
typeof 9223372036854775807       # returns Float        — maxint exactly
```

So `9223372036854775807` — which *is* `int64` max and should be the
canonical largest `Integer` — cannot be written as one at all today.

### 1.3 Even the documentation that tried to describe this is wrong

`REFERENCE.md:418` currently states:

> Integer overflow is silent and inconsistent. `add`/`mul` past `maxint`
> promote to `Float` (losing integer precision) … `pow` instead wraps.

But `mul` does **not** promote to Float — it wraps
(`4000000000 4000000000 mul` returns a negative Integer, §1.1c). The
prose conflates the lexer's Float fallback (§1.1a) with runtime
behaviour and miscategorises `mul`. This is itself evidence for the
central recommendation below: **the behaviour is confusing enough that
the documentation pass got it wrong.** Documentation cannot be the fix
for a silent wrong answer (§5).

### 1.4 Why it is inconsistent — root cause

| Site | Path | On overflow | Result |
| --- | --- | --- | --- |
| Literal `> 2⁵³` | jsonic → `float64` → `floatToValue` | precision already lost | wrong `Integer`, then `Float` near maxint |
| `add`/`sub`/`mul` | `numericBinaryHandler.intFn`, raw `int64` | two's-complement wrap | wrong `Integer` (sign flips) |
| `pow` | hand-rolled `int64` loop | two's-complement wrap | wrong `Integer` |

Three code paths, three policies, none of them checked, none of them
flagged. There is no single place where "what does `Integer` mean at its
boundary" is decided — so it is decided three different ways by
accident.

---

## 2. What other languages do

Five strategies exist in the wild. Only three are defensible defaults;
two (the two boru currently uses) are widely regarded as anti-patterns.

| Strategy | On overflow | Languages |
| --- | --- | --- |
| **Promote to bignum** | result is exact, integer widens | Python 3, Ruby, JavaScript `BigInt`, Lisp/Scheme, Haskell `Integer`, Erlang/Elixir, Smalltalk |
| **Error / trap** | clean failure | SQL (PostgreSQL `integer out of range`), Swift (traps by default), Rust (panics in debug), Ada (`Constraint_Error`), C# `checked` |
| **Wrap, documented** | two's-complement, *by contract* | Go, Java, C# `unchecked`, Rust `--release`, C unsigned |
| **Silent float promotion** | precision lost above 2⁵³ | JavaScript pre-`BigInt` numbers, Lua ≤ 5.2 — *boru's literal path* |
| **Silent signed wrap** | UB or quiet garbage | C signed overflow (UB), *boru's runtime path* |

Three observations matter for boru:

1. **The "naturally expected" answer is bignum.** Python/Ruby/Lisp users
   never see overflow; `2 ** 63` is just `9223372036854775808`. For a
   language whose audience writes data transforms and aggregates, "the
   number is simply correct" is the least-surprising behaviour by a wide
   margin. This is the dominant choice among dynamic, data-oriented
   languages — exactly boru's category.

2. **Query languages specifically error.** PostgreSQL does **not** wrap
   and does **not** promote to float — it raises
   `ERROR: integer out of range`. SQL Server, DB2, Oracle behave the
   same on fixed-width integer columns. A query language treating
   overflow as a hard error has strong precedent; silent wrap or silent
   float would be a data-integrity bug in any of them.

3. **Wrapping is only acceptable when it is a documented contract.** Go
   and Java wrap, but their specs *say so*, the width is fixed and
   visible (`int64`), and the audience expects machine arithmetic. Even
   then, the modern safety-first languages moved away from it: Swift and
   Rust-debug trap by default and make wrapping an *explicit* operator
   (`&+`, `wrapping_add`). C's silent **signed** wrap (undefined
   behaviour) is the cautionary tale the whole industry cites — and it
   is precisely boru's runtime behaviour today.

**Best-practice synthesis.** For a high-level, dynamically-typed
data/query language the recommended defaults, in order, are:
(1) transparent bignum promotion (Python/Ruby model — correctness with
no user burden), or (2) a hard overflow error (SQL model — correctness
with a clear failure). Silent wrap is acceptable *only* as an explicit,
documented, opt-in operator. Silent float promotion is recommended by no
one — it is the JavaScript `Number` trap that motivated `BigInt`.

---

## 3. The split: errors vs documentation vs expected behaviour

The task asks how to divide the fix across these three levers. They are
not alternatives — each is the right tool for a *different* part of the
problem, and the failure mode is using the wrong lever.

### 3.1 Naturally expected behaviour (do the right thing silently)

This lever applies where there is an unambiguous correct answer that the
user expects without thinking. `9223372036854775807 1 add` has one
defensible result: `9223372036854775808`. Bignum promotion delivers
exactly that. When a correct, unsurprising answer is *cheaply
available*, producing it silently is strictly better than either an
error (annoying) or documentation (unread). This is the lever for the
literal path unconditionally — `9007199254740993` must mean
`9007199254740993` — and for the arithmetic path **if** boru adopts
bignum Integers.

### 3.2 Errors (fail loudly where you cannot be both correct and silent)

This lever applies where the engine *cannot* return the right answer
under its chosen representation. If `Integer` stays fixed-width `int64`,
then `maxint add 1` has no correct `int64` result — and the only honest
options are "promote the type" (changes `typeof`, surprising) or
"error". A typed `[boru/integer_overflow]` error is the SQL-correct
choice: it never lies. boru already uses this lever for the sibling case
— `1 div 0` raises `division by zero` rather than returning a bogus
number — so an overflow error is *consistent with existing design*, not
a new concept. The rule: **a silent wrong answer is never acceptable; if
you cannot be silently right, be loudly wrong.**

### 3.3 Documentation (define the boundary of a *consistent* policy)

Documentation is necessary but **never sufficient**, and it is the wrong
primary tool here. §1.3 is the proof: the behaviour was so inconsistent
that the documentation describing it was itself incorrect. You cannot
document your way out of "three code paths disagree" — you can only
document a policy *after* it has been made uniform. Once `Integer` has a
single, consistent contract (whether "arbitrary precision" or "int64,
overflow is an error"), documentation's job is to state that contract
plainly — which today's REFERENCE/help do not do at all (neither
mentions the width or the overflow rule before the audit added the
flawed note). Documentation pins the boundary of a coherent policy; it
cannot create coherence.

### 3.4 The decision tree

```
Is there a correct, unsurprising answer cheaply available?
├── yes → produce it silently            (§3.1 — literals always; arithmetic iff bignum)
└── no  → can the representation hold it?
         ├── no  → raise [boru/integer_overflow]   (§3.2 — never wrap, never float-coerce)
         └── n/a → document the contract once it is uniform   (§3.3)
```

Silent wrap and silent float promotion appear nowhere in this tree.
That is the point: both of boru's current behaviours are off the tree
entirely.

---

## 4. Recommended strategy

A two-phase plan. Phase 0 is unambiguous correctness at near-zero
compatibility cost and is compatible with *either* long-term
destination, so it can ship immediately without waiting on the larger
decision. Phase 1 is the language-defining choice and shares machinery
with the numeric-tower work.

### Phase 0 — stop the silent corruption (do now)

Two changes, both of which only ever replace a *wrong* result with a
correct one or a clear error. Neither needs new type machinery.

**0a. Lexer: parse integers as integers.** Add a real integer path so
literals never round-trip through `float64`. A token with no `.` / `e`
is parsed with `strconv.ParseInt(src, 10, 64)`:

- success → exact `Integer` (so `9223372036854775807` is finally a
  correct `Integer`, and `9007199254740993` keeps its true value);
- `ErrRange` → raise `[boru/integer_overflow]: integer literal out of
  range` (or, post-Phase 1, build a `big.Int`).

This kills §1.2 entirely — both the silent value corruption above 2⁵³
*and* the Float fallback near maxint. It lands at
`eng/go/parser/parse.go` (`numberValToValue` / the `numberVal` sub), and
needs jsonic to hand over the source digits (already captured in
`numberVal.Src`).

**0b. Runtime: checked arithmetic, one uniform policy.** Replace the raw
`int64` ops in `numericBinaryHandler.intFn`
(`native_helpers.go:33`) and the `pow` loop (`native_math.go:120`) with
checked arithmetic — `math/bits.Add64`/`Mul64`, or an explicit
post-condition overflow test — and apply **one** policy across
`add`/`sub`/`mul`/`pow`. **Interim policy: error**
(`[boru/integer_overflow]`), mirroring the existing `division by zero`.

After Phase 0, boru is *consistent and honest*: every overflow, in the
lexer or the runtime, in any word, produces the same clean error. No
silent wrong answer survives. This is shippable on its own and is the
correctness floor under either Phase-1 outcome.

#### Phase 0 status

**Done.** As implemented:

- **0a (lexer).** `setupNumberSub` (`eng/go/parser/grammar.go`) now
  carries the source digits for *every* number token, and
  `numberValToValue` (`eng/go/parser/parse.go`) parses a *plain decimal
  integer* (optional sign `-`/`+`, digits, `_` separators) from its exact
  text with `strconv.ParseInt`. `9007199254740993` is now its true value
  (was silently `…992`); `9223372036854775807` is finally a usable
  `Integer` (was a `Float`); an out-of-range literal raises
  `[boru/integer_overflow]`. **Base-prefixed (`0x`/`0o`/`0b`) literals are
  also parsed exactly** (`strconv.ParseInt` base 0), so hex/oct/bin above
  2^53 keep full precision and are range-checked too — a later follow-up
  to the original Phase 0, which had left them on the float path. Only
  *scientific* whole literals (`1e3`) remain on the float-derived path
  (the `1e3 → Integer` / `1.5 → Float` rules are unchanged); writing an
  exact large integer in scientific notation is discouraged in REFERENCE.
- **0b (runtime).** `add`/`sub`/`mul`/`pow`
  (`lang/go/native/native_math.go`) use checked int64 helpers
  (`checkedAddInt`/`checkedSubInt`/`checkedMulInt`/`checkedPowInt`) and
  raise `[boru/integer_overflow]` uniformly on overflow instead of
  wrapping. Float handlers are untouched (they saturate to ±Inf).
- **Source positions.** Both overflow errors are located in the source:
  the runtime error is stamped at the operator token by the engine's
  `stampErrPos`, and the literal error carries the offending literal's
  row/col (captured into `numberVal` in `setupNumberSub` and threaded
  into the parse error).
- **Tests.** Parser exactness/overflow/position + non-decimal
  preservation (`eng/go/parser/parse_test.go`), checked-helper boundary
  tests (`lang/go/native/checked_arith_test.go`), end-to-end error
  position (`lang/go/test/overflow_position_test.go`), and spec rows
  (`lang/spec/arithmetic.tsv` §6, positive + negative).
- **Docs.** `REFERENCE.md` and the `add`/`sub`/`mul`/`pow` help entries
  now state the int64 range and the overflow-is-an-error contract.

### Phase 1 — arbitrary-precision Integer (decide, then do)

The recommended long-term destination is **bignum `Integer`**:
`big.Int`-backed with an `int64` fast path, so the common case stays on
the hardware integer and only promotes when it must.

- Overflow becomes **promotion**, not error: `2 63 pow` returns
  `9223372036854775808`, `maxint add 1` is simply correct. Phase 0's
  error path becomes a promotion path; the literal `ErrRange` branch
  builds a `big.Int` instead of erroring.
- This is the Python/Ruby/Lisp model — the "naturally expected
  behaviour" (§3.1) for a data language, and the choice that makes
  Exhibit K *unrepresentable* rather than merely diagnosed.

Why bignum over "stay int64 + permanent error": a query language sums
columns and computes products where crossing 2⁶³ is plausible and not a
user mistake; erroring there is a sharp edge bignum removes for free.
The error policy is the right *interim* and the right *floor*, but exact
unbounded integers are the right *destination*.

**This is not separate work.** [NUMERIC-TOWER](legacy/NUMERIC-TOWER.0.ignore) §
"Adjacent decision (Exhibit K)" already calls for exactly this and notes
it shares the parser-literal-exactness, payload, and promotion machinery
with the exact-`Decimal` proposal. Open decision #4 there
("`Integer`→`big.Int` in the same pass, or defer") is *this* decision.
Sequencing:

- **`Integer` (`big.Int`)** removes the overflow wrap and the
  large-literal trap — the whole of Exhibit K.
- **`Decimal` (`apd`)** adds exact base-10 (Exhibit L's reserved name).

Both add a `Scalar/Number` leaf, both need the lexer to read exact digit
strings (Phase 0a is the down-payment), both extend the same
`numericBinaryHandler` promotion lattice. Plan them as one
"numeric representation" pass; Phase 0 is the prerequisite either way.

### Implementation sketch (Phase 1)

Grounded in the kernel conventions (`eng/go/CLAUDE.md`):

- **Payload** (`eng/go/payload.go`): add a `BigIntPayload{N *big.Int}`
  wrapper variant; keep `IntPayload{N int64}` as the fast path.
  `NewInteger` stays `int64`; add `NewBigInteger(*big.Int)` that
  normalises down to `IntPayload` when the value fits.
- **Arithmetic** (`native_helpers.go::numericBinaryHandler`,
  `native_math.go`): the `intFn` does checked `int64`; on overflow it
  promotes both operands to `big.Int` and retries, returning a
  normalised `Integer`. Same shape as the proposed tower dispatch.
- **Comparer** (`compare_types.go`): the `Number` LCA Comparer must
  compare a `big.Int`-backed Integer against `int64`, `Float`, and
  (future) `Decimal` without re-introducing precision loss — compare via
  `big.Int`/`big.Rat`, not by projecting to `float64`.
- **Lexer** (`eng/go/parser`): Phase 0a, extended so an out-of-`int64`
  literal builds a `big.Int` rather than erroring.
- **Rendering**: `big.Int.String()` — already exact.

### Compatibility

- **Phase 0a** *recovers* correctness: `9223372036854775807` becomes a
  usable `Integer` (was an unusable Float); sub-2⁵³ literals stop being
  silently corrupted. The only behavioural loss is genuinely
  out-of-range literals, which error instead of silently float-ing —
  they were wrong before.
- **Phase 0b** turns previously-silent wraps into errors. Any program
  relying on a wrapped/float result was already getting a wrong number;
  the error surfaces a latent bug. Pin the new boundary behaviour with
  `ERROR:` rows in the math spec TSVs (per the test discipline in
  `lang/go/CLAUDE.md`).
- **Phase 1** turns those Phase-0 errors into correct large results —
  strictly more accepting, no new rejections. Medium-risk only in that
  `typeof` of a huge result stays `Integer` (good) and equality/compare
  must hold across the int64/bignum boundary (covered by the Comparer
  change + `compare.tsv` rows).

### Open decisions

1. **Phase 1 destination: bignum (recommended) vs permanent int64+error.**
   Bignum is the data-language-correct choice and matches the numeric
   tower; permanent-error is cheaper but leaves a sharp edge on
   legitimate large aggregates.
2. **Bundle with `Decimal`?** Recommended yes — shared lexer/payload/
   promotion machinery (NUMERIC-TOWER open decision #4).
3. **Explicit wrapping ops?** If any audience wants machine arithmetic,
   expose it as named opt-in words (`wrap-add`, …) à la Swift `&+` /
   Rust `wrapping_add` — never as the default. Likely unnecessary for a
   query language; list as a non-goal unless asked.
4. **Error code.** Recommend a dedicated `[boru/integer_overflow]` for
   both the literal and runtime paths so the diagnostic is greppable and
   uniform.

### Non-goals (this pass)

- Unsigned integer types.
- `Rational` (`big.Rat`) — reserved by the numeric tower, not built here.
- Changing `div`'s truncating semantics (Exhibit H) — independent.
