# F# Units of Measure in boru: Short Feasibility Report

## Scope
Evaluate whether F#-style units of measure (dimensional tagging of numeric values, checked at signature match time) can be applied to boru.

## F# Units Recap
- Numeric types are annotated with units: `1.5<m>`, `float<m/s^2>`.
- Units form an algebra under `*`, `/`, integer powers; dimensionless is `1`.
- Checked statically, erased at runtime, supports generic unit variables.
- Catches dimensional mismatch bugs (`m + s` is a type error).
- Does not do unit conversion or affine offsets — those stay explicit.

## boru Baseline
- `Scalar/Number` with `Integer` and `Decimal` subtypes.
- Hierarchical type paths with subtype matching (`Matches()`).
- Signature-based dispatch on argument types.
- Typed records and typed lists/maps already model structured values.

## Mapping Sketch
- Represent a unit as a structured tag: product of base units with integer exponents, e.g. `{m:1, s:-2}`.
- Attach tag to a `Number` either as a **subtype extension** (`Number/Quantity/<tag-id>`) or as a **wrapper record** `{value:Number, unit:UnitTag}`.
- Extend arithmetic words (`add`, `sub`, `mul`, `div`, `pow`) to check/propagate tags.
- Provide dimensionless fallback: unadorned numbers keep current behaviour.

## Feasibility Verdict

| Area | Verdict |
|---|---|
| Wrapper-record units (library only) | **Feasible now**, no engine changes |
| Subtype-based units via hierarchical type path | **Feasible**, needs dynamic type-path generation |
| Signature dispatch on unit tags | **Feasible** via `Matches()` extension for tagged number types |
| Generic unit variables in signatures | **Moderate** — signature matcher has no unit-variables today |
| Fully static checking (F#-equivalent) | **Hard** — boru dispatch is runtime; best boru can do is fail fast at call site |
| Unit algebra for `mul`/`div`/`pow` | **Feasible**, mechanical |
| Conversion between compatible units (m ↔ cm) | **Feasible** as explicit `convert` word |
| Affine units (°C/K, timestamps) | **Out of scope**, same as F# |

## Syntax Recommendations

### Design constraints
- boru is concatenative: every token is a word or a literal. New syntax must either be a literal form or a word.
- `<` and `>` are used as comparison words, so F#'s `1.5<m>` form would clash.
- Atoms are bare unquoted words, so `9.81 m` parses today as *number then atom* — giving us a ready composition point.
- Existing map syntax `{m:1, s:-2}` already expresses an exponent-keyed unit tag cleanly.

### Tier 1 — library-only, no parser change (preferred first step)
Use a word that pairs a number with a unit map.

```boru
def g qty 9.81 {m:1, s:-2}      # 9.81 m/s^2
def d qty 100 {m:1}              # 100 m
def t qty 10 {s:1}               # 10 s
```

Convenience constructors per base unit avoid map noise:

```boru
def g 9.81 m/s^2                 # m/s^2 is a unit word that rebuilds the tag
def d 100 m
def t 10 s
mul d (div 1 t)                  # -> qty 10 {m:1, s:-1}
```

Here `m`, `s`, `m/s^2`, `kg`, etc. are **unit words** registered in a `boru:units` module. Each takes a `Number` from the stack and returns a tagged `Quantity`. Composite unit words (`m/s`, `N`, `J`) are just predefined compositions.

### Tier 2 — unit-literal sugar (opt-in, one parser hook)
If Tier 1 proves noisy, introduce a single postfix-marker token. Two candidates, both avoid the `<…>` clash:

**Option A — `#` suffix** (recommended):
```boru
9.81#m/s^2
100#m
10#s
```
Parse rule: after a number literal, `#` followed by a unit expression (slash, caret, digits, atoms) produces a `Quantity` literal.

**Option B — `` ` `` suffix**: conflicts with template strings — rejected.

**Option C — bracketed**: `9.81[m/s^2]` — conflicts with list-index intuition — rejected.

### Unit expressions inside the tag
Regardless of literal form, the unit side uses a tiny grammar reused for both literals and the `qty` word:

```
unit   := factor ( ('*' | '/') factor )*
factor := base ( '^' integer )?
base   := atom | '(' unit ')'
```

Examples:
- `m`
- `m/s`
- `m/s^2`
- `kg*m/s^2`
- `1/s` (dimensionless numerator allowed)

This grammar produces the same `{base:exponent}` map everywhere, so `qty 9.81 m/s^2` and `9.81#m/s^2` are interchangeable.

### Type-annotation syntax
For record fields and function signatures, annotate a numeric type with a unit tag using the typed-map convention already in boru:

```boru
def Velocity (refine Record [speed:Number#m/s])
def Force    (refine Record [magnitude:Number#kg*m/s^2])

def accelerate fn [
  [[v:Number#m/s, a:Number#m/s^2, t:Number#s] [Number#m/s]]
  [a t mul v add]
]
```

The `Number#<unit>` form is a compound type literal parsed as `Number` refined by a unit tag. Signature matching treats unit mismatch the same as type mismatch.

### Generic unit variables
Use a leading `'` on an atom inside the unit expression to mark a unit variable, echoing F#'s `'u`:

```boru
def sqr fn [[x:Number#'u] [Number#'u^2]] [x x mul]
def per-second fn [[x:Number#'u] [Number#'u/s]] [x 1#s div]
```

Unit variables are matched structurally at call time: the matcher records a binding on first occurrence and checks consistency on the rest.

### Pretty-printing
- Default string form of a `Quantity` is `<value><space><unit>`, e.g. `9.81 m/s^2`.
- Inspection/REPL form shows the tag map when `verbose` is set: `qty(9.81, {m:1, s:-2})`.

### Syntax recommendation
Ship **Tier 1 only** first: `qty`, unit words per base, and `Number#<unit>` in type positions kept as documentation convention (no parser change yet). Promote to **Tier 2** (the `#` literal suffix and real compound type parsing) once usage in 2-3 realistic modules confirms the noise cost of Tier 1.

## Recommended First Cut
1. Library word `qty` builds a tagged number: `qty 9.81 {m:1, s:-2}`.
2. Arithmetic words gain `[Quantity, Quantity]` signatures that check tag compatibility and propagate via unit algebra.
3. `convert` word applies a scale factor and retags.
4. Errors use existing error-value convention: `unit-mismatch`, `non-integer-exponent`.
5. Defer generic unit variables and engine-native tagging until real usage justifies them.

## Implementation

Concrete plan for building Tier 1 inside the current engine, with file references.

### Value representation
Add a `Quantity` payload carried as a typed-Object, slotting into the existing `Object` branch of the type tree:

```
Object
  Quantity                    -- tagged numeric value
```

- New type constant `TQuantity` registered next to `TResource`/`TTable` in `internal/engine/types.go`.
- Payload struct:
  ```go
  type QuantityData struct {
      Value Value          // Parent must match TNumber (Integer or Decimal)
      Unit  UnitTag        // canonical unit, integer exponents keyed by base atom
  }
  type UnitTag map[string]int
  ```
- `UnitTag` is canonical: keys sorted, zero exponents dropped, empty map = dimensionless. Store the canonical form so equality is a plain `reflect.DeepEqual` / map compare.
- Add `NewQuantity(v Value, u UnitTag) Value` constructor mirroring `NewDecimal` / `NewDate` in `value.go`.
- Add `(v Value) AsQuantity() (*QuantityData, bool)` accessor following the `AsDate` / `AsCalDuration` pattern. Return `nil, false` for type literals with `Data==nil` (panic-prevention rule).

### Type matching
- `TQuantity` is a leaf under `TObject`, so existing `Matches()` in `internal/engine/types.go` handles parent/child matching without change.
- For per-unit dispatch, extend `Matches()` with an optional tag check: a signature can carry a `UnitConstraint` (nil = any unit). This is implemented as a wrapper `TaggedType{Base: TQuantity, Unit: UnitTag, Vars: []string}` passed in `NativeSig.Args`. The matcher in `match.go` already walks `Type` values; add a narrow `matchTaggedType` helper called when `Base.Equal(TQuantity)`.
- Generic unit variables (`'u`) become entries in `Vars`. A `UnitBindings` map is threaded through `matchSignature` alongside the existing argument collection state, reset per dispatch attempt.

### Unit algebra
A single pure helper covers the arithmetic cases:

```go
// internal/engine/units.go
func UnitMul(a, b UnitTag) UnitTag
func UnitDiv(a, b UnitTag) UnitTag
func UnitPow(a UnitTag, n int) UnitTag
func UnitEqual(a, b UnitTag) bool
func UnitCanonical(a UnitTag) UnitTag
func UnitFormat(a UnitTag) string    // "m/s^2"
func UnitParse(s string) (UnitTag, error)
```

Each is mechanical: add/subtract exponents, drop zeros, sort keys. No external deps.

### Arithmetic word extensions
Touch the existing `registerAdd`, `registerSub`, `registerMul`, `registerDiv`, `registerPow`, `registerMod` in `internal/engine/native_math_*.go`. Pattern, using `add` as the canonical example (currently at `native_math_add.go:40`):

```go
addQtyHandler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
    a, _ := args[1].AsQuantity()
    b, _ := args[0].AsQuantity()
    if !UnitEqual(a.Unit, b.Unit) {
        return nil, NewBoruError("unit-mismatch",
            fmt.Sprintf("add: %s vs %s", UnitFormat(a.Unit), UnitFormat(b.Unit)))
    }
    sum, err := addNumbers(a.Value, b.Value)   // reuse existing numeric helper
    if err != nil { return nil, err }
    return []Value{NewQuantity(sum, a.Unit)}, nil
}

registerBinaryMathWord(r, "add",
    /* existing numeric closures... */,
    NativeSig{Args: []Type{TQuantity, TQuantity}, Handler: addQtyHandler},
    /* existing temporal sigs... */,
)
```

- `add` / `sub` / `mod`: require `UnitEqual`, propagate the shared tag.
- `mul`: unit is `UnitMul(a, b)`.
- `div`: unit is `UnitDiv(a, b)`; dimensionless result collapses to plain `Number`.
- `pow`: exponent must be an `Integer` literal at match time; unit becomes `UnitPow(base, n)`. Non-integer exponent → `non-integer-exponent` error.
- `negate`, `abs`, `min`, `max`, `sign`: accept `Quantity`, preserve tag.
- Comparisons (`lt`, `gt`, `eq`, …) in `compare.go`: require `UnitEqual`, then compare underlying numbers.

### Mixed Quantity/Number interop
One choice, documented once: **promote raw numbers only inside `mul`, `div`, `pow`; reject them in `add`/`sub`/comparisons**. This matches F# (you can scale a `float<m>` by a raw `float`, but you can't add a `float` to a `float<m>`). Implementation:

- Add extra sigs `[TQuantity, TNumber]` and `[TNumber, TQuantity]` to `mul`, `div`, `pow` only. Treat the raw number as dimensionless.
- `add`/`sub` keep only the `[TQuantity, TQuantity]` and existing `[TNumber, TNumber]` sigs — cross-dispatch produces the normal no-match error.

### New words
Register in a new `internal/engine/native_math_qty.go`:

- `qty [Number, Map] -> Quantity` — build from a value and a unit-map literal.
- `unit [Quantity] -> Map` — extract the unit tag as a map.
- `value [Quantity] -> Number` — extract the raw numeric.
- `dimensionless? [Quantity] -> Boolean`
- `convert [Quantity, Map, Decimal] -> Quantity` — retag and scale (`convert q {m:1, cm:-1} 100` says 1 m = 100 cm).
- `compatible? [Quantity, Quantity] -> Boolean` — same-unit check used by tooling/debugging.

Per-base unit words (`m`, `kg`, `s`, `A`, `K`, `mol`, `cd` for SI; plus `m/s`, `m/s^2`, `N`, `J`, `W`, `Hz`, `Pa` as composites) live in a new native module `internal/native/units.go` loaded via `import "boru:units"`. Each is a 1-arg word `[Number] -> Quantity` whose handler just calls `NewQuantity(n, precomputedTag)`.

### Parser hook (Tier 2 only — defer)
If/when the `#` literal suffix is adopted, the changes are localised:

1. Register `#` via `j.Token()` in `internal/parser/parse.go` (alongside the existing custom tokens documented in `lang/CLAUDE.md`).
2. Extend the `"val"` rule so a number followed by `#` opens a `"unitexpr"` sub-rule that lexes the unit grammar (`atom`, `*`, `/`, `^`, integer, `(`, `)`).
3. In `convertTopLevelValue` / `convertDataValue`, fold the resulting tokens into a `Quantity` literal via `UnitParse`.
4. For signature positions, teach `convertTypeLiteral` to recognise `Number#<unit>` and produce a `TaggedType` entry.

Nothing in the engine changes for Tier 2 — it only adds a shorter path to the same `NewQuantity` constructor.

### Serialization
- `print` (`internal/engine/print.go`): render as `"<value> <unit>"`, with `UnitFormat` producing `m/s^2`.
- `inspect`: use the existing map-emit path to show `qty(<value>, {m:1, s:-2})`.
- JSON (`internal/native/jsonify.go`): emit `{"$qty": <number>, "$unit": {"m": 1, "s": -2}}`. Round-trip via a `$qty` reviver in `jsonify.go` parse path.
- SQL storage (`internal/engine/sqlite.go`): store as TEXT in the above JSON form; decode on column read.

### Testing
Mirror the existing pattern (`native_temporal_*_test.go`, `math_bool_coverage_test.go`):

1. `units_test.go` — pure unit-algebra coverage (`UnitMul`/`UnitDiv`/`UnitPow`/canonicalisation).
2. `native_math_qty_test.go` — dispatch cases: `add` same-unit, `add` mismatch, `mul` tag propagation, `div` collapse to dimensionless, `pow` integer check, raw-number interop rules.
3. Panic-prevention: extend `TestTypeLiteralNoPanic` in `internal/engine/type_scaling_test.go` so the new `qty`, `unit`, `value`, `convert`, and per-base unit words receive a `Quantity` type literal (`Data==nil`) and return an error cleanly.
4. End-to-end: add a boru-level script under `lang/go/test/` mirroring `lang/go/test/*.boru` layout — a small physics example and a finance (currency tags) example.

### Effort estimate
- Engine type + accessors: ~0.5 day.
- Unit algebra + tests: ~0.5 day.
- Arithmetic/comparison sig extensions: ~1 day.
- New words + per-base unit module: ~1 day.
- Serialization paths: ~0.5 day.
- Panic-prevention and integration tests: ~1 day.
- Docs (`LANGREF.md`, `SIGNATURES.md`, `TYPES.md`): ~0.5 day.

**Total Tier 1: ~5 dev days.** Tier 2 parser work is an additional ~2-3 days and can follow later.

## Risk
- Dispatch cost: tagged-number signatures widen the match set for hot arithmetic words.
- Interop: tagged numbers flowing into untagged words must degrade gracefully (strip tag or error — pick one and document).
- Serialization: JSON/record round-trips need a canonical encoding of unit tags.

## Verdict
**Feasible as a library-first subsystem.** boru cannot replicate F#'s zero-cost static guarantee, but a runtime-checked equivalent at signature-match time captures the same class of dimensional bugs with minimal engine change.
