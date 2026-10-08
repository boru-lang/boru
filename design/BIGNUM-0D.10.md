# BigInteger & BigDecimal — exact numbers via the `0d` literal

Status: **implemented**. This is the as-built record for the two
arbitrary-precision numeric leaves added under `Scalar/Number`. It
supersedes the single-`Decimal`/`d`-suffix sketch in
[legacy/NUMERIC-TOWER.0.ignore](legacy/NUMERIC-TOWER.0.ignore). User-facing documentation lives
in [REFERENCE.md](../REFERENCE.md) (§ "Arbitrary-precision numbers");
the executable spec is [`lang/spec/bignum.tsv`](../lang/spec/bignum.tsv).

## Motivation

`Integer` (int64, overflows → `[boru/integer_overflow]`) and `Float`
(IEEE-754 binary64) leave two gaps a query language — where money and
large aggregates live — cannot ignore:

- integers beyond ±2⁶³;
- decimals that binary `Float` cannot represent (`0.1 add 0.2` →
  `0.30000000000000004`).

We close both with two **opt-in, exact** leaves rather than changing
`Integer`/`Float` (the Java "explicit BigInteger" model, not Python's
single unbounded int):

- **`BigInteger`** — `math/big.Int`, unbounded exact integers.
- **`BigDecimal`** — `github.com/cockroachdb/apd/v3`, exact base-10
  (pure-Go, wasm-friendly).

`Integer`/`Float` semantics are **byte-identical** to before; you opt in
with the `0d` prefix.

## Confirmed decisions (load-bearing)

1. **Literals.** `0d`/`0D` prefix. Digits only → `BigInteger`; a `.`
   **or** an exponent (`e`/`E`) → `BigDecimal` (so `0d12.5`, `0d1.5e3`,
   **and** `0d1e3` are all BigDecimal). A leading sign and single `_`
   digit-separators are allowed, reusing the fixed-width literal rules.
   Scale is preserved on round-trip (`0d0.10` → `0d0.10`).

2. **Type infection — widest exact leaf wins.** Among the exact leaves
   the ladder is `Integer < BigInteger < BigDecimal`; a mixed-leaf op
   promotes to the widest operand (`1 add 0d2` → BigInteger,
   `0d2 add 0d0.5` → BigDecimal, `1 add 0d0.5` → BigDecimal). The legacy
   `Integer ⊕ Float → Float` rule is **unchanged**. `BigInteger ⊕ Float`
   and `BigDecimal ⊕ Float` are an **`[boru/type_error]`** — the exact
   types never silently degrade to a binary Float (matches Python
   `Decimal`+`float`, C#).

3. **Transcendentals.** apd-backed (`sqrt`, `cbrt`, `exp`, `log` (Ln),
   `log10`, fractional `pow`) compute to the active decimal context and
   return `BigDecimal`; the trig family and the rest apd lacks accept a
   Big argument via the lossy `AsFloatApprox` and return `Float`.

4. **Division.** `BigInteger div BigInteger` truncates toward zero →
   BigInteger (with `mod`), like `Integer div`. `BigDecimal div
   BigDecimal` → BigDecimal rounded to the active context, default IEEE
   **decimal128** (34 significant digits, round-half-even).

## Cross-leaf comparison is honest

`0d5 eq 5`, `1 cmp 0d1.0` → `0`, `0d0.5 eq 0.5` are all true (magnitudes
exact in every leaf). But `0.1 eq 0d0.1` → **false**: the Float's true
value is `0.1000000000000000055…`, not one tenth — the same result
Python's `Decimal('0.1') == 0.1` gives. Implemented by comparing in a
common `big.Rat` domain (`big.Rat.SetFloat64` captures the float's exact
binary value, never re-rounds), so equality never re-introduces binary
error.

## Implementation map

| Concern | Location |
|---|---|
| Lattice leaves (FixedID 100/101, ranks above Float) | `eng/go/typetable.go`, `eng/go/types.go` |
| Payloads (`BigIntPayload`, `DecimalPayload`) | `eng/go/payload.go` |
| Constructors / accessors / rendering / `AsFloatApprox` | `eng/go/value.go` |
| Canonical `0d…` round-trip | `eng/go/canon.go` |
| Lexer matcher (claims the whole `0d…` run incl. the `.`) | `eng/go/parser/grammar.go` |
| Literal → value conversion | `eng/go/parser/parse.go` |
| Exact cross-leaf compare / equality (`big.Rat`) | `eng/go/compare_scalar_behaviors.go`, `eng/go/compare.go` |
| Widest-leaf return inference | `eng/go/carrier.go` |
| Arithmetic tower dispatch (`towerOps`, Big⊕Float error) | `lang/go/native/native_helpers.go`, `native_math.go` |
| `convert` to/from the Big leaves | `lang/go/native/native_type.go` |
| Transcendental builders + `with-decimal` | `lang/go/native/native_helpers.go`, `native_math.go`, `lang/go/modules/math.go` |

### The lexer `.`-split problem

jsonic lexes `.` as a fixed `#DT` token, so `0d12.5` would split into
`0d12` · `.` · `5`. A high-priority `LexMatcher` (`setupBigNumberMatcher`,
gated off inside template literals) hand-scans the whole `0d…` run and
emits one text token routed through `parseWord`. It consumes a `.` only
when the next char is a digit, so `0d5.foo` keeps the dot as member
access (`0d5 get foo`).

### Scoped rounding — `with-decimal`

`with-decimal {precision: N, rounding: "…"} [body]` pushes a
precision/rounding override onto the existing copy-on-write context
stack; `decimalContext(r)` reads it (default decimal128 when unset). It
threads through both arithmetic and the apd-backed transcendentals,
unwinds at end of block, and nests. Rounding names: `half-even`
(default), `half-up`, `half-down`, `down`, `up`, `ceiling`, `floor`.

## Test surface

- `lang/spec/bignum.tsv` — literals, `typeof`, arithmetic (both leaves),
  promotion, comparison/sort, transcendentals, `with-decimal`, `convert`,
  and negative rows (Big⊕Float `type_error`, `0.1 eq 0d0.1 → false`,
  malformed `0d`, divide-by-zero, Float→Big convert refusal).
- `eng/go/parser/bignum_lit_test.go` — the `.`-split regression and
  malformed-literal syntax errors.
- `lang/go/test/paren_native_type_literal_test.go` — no-panic coverage
  for the Big words and `with-decimal` with bare type-literal args.
- `lang/go/test/fixedid_stability_test.go` — FixedID snapshot.
