# `math/cmplx` → `boru:cmplx` *(NICHE)*

> **Status: design proposal — not implemented. NICHE.** A curated,
> hand-written native module wrapping Go's `math/cmplx`. Read
> [`README.10.md`](README.10.md) first for the shared conventions this
> note assumes. Flagged **niche** in the README roster alongside `Mail`
> and `Bytes` — see the candid §10 on whether it is worth shipping at
> all before a native Complex type exists.

## 1. Package & status

Go package: [`math/cmplx`](https://pkg.go.dev/math/cmplx) — elementary
functions over `complex128`: magnitude, phase, polar/rectangular
conversion, conjugate, and the transcendentals (`Exp`, `Log`, `Sqrt`,
`Pow`). This note specifies `boru:cmplx` (namespace `Cmplx`). Design
proposal; no Go code exists yet, and shipping is genuinely in question
(§10).

## 2. Why curated

There is no "raw bridge vs curated" framing here, because **boru has no
complex type at all** — `complex128` has no `core.FromNative` /
`core.ToNative` mapping, so a mechanical `go:math/cmplx` bridge could not
even pass an argument. Any boru surface must *invent* a representation. The
curated choice is to model a complex number as a **`Map {re, im}`** of two
Floats, so every word reads and writes ordinary inspectable data and the
results compose with `get`, `set`, comparison, and serialization.

## 3. Import & namespace

```
import "boru:cmplx"        # binds the Cmplx namespace
```

The bare capitalized package name `Cmplx` is free (not a builtin type,
not an existing module namespace), so **no `-util` suffix** is needed
(per the naming rule in `lang/go/CLAUDE.md` "Package layout"). Words are
dot-accessed: `Cmplx.rect`, `Cmplx.abs`, …

## 4. API

**The complex-number representation:** a complex number is a **`Map` with
two Float keys, `re` (real part) and `im` (imaginary part)**. Polar
results are a `Map {r, theta}` (`r` = magnitude, `theta` = phase in
radians). This Map modeling is the central — and contentious (§10) —
design decision; state it up front.

Signatures are **top-first, sig order** (position 0 = top of stack), per
the README "Argument order & dispatch" rule. All inner natives use
`BarrierPos: -1` so the infix form `a Cmplx.word b` dispatches.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `complex(re, im)` | `rect` | `[Float im, Float re] -> Map` | Build a complex number from real and imaginary parts. | Constructor: `re Cmplx.rect im` (infix) → `{re, im}` Map. There is no Go `cmplx` function for this (it is the `complex` builtin); curated as the canonical way to mint the Map. |
| `cmplx.Polar` | `polar` | `[Map] -> Map` | Decompose a complex number into magnitude and phase. | `cmplx.Polar` returns `(r, theta)`; collapsed to a `Map {r, theta}` (`Map → Map`). |
| `cmplx.Rect` | `from-polar` | `[Float theta, Float r] -> Map` | Build a complex number from magnitude and phase. | Inverse of `polar`: `r Cmplx.from-polar theta` → `{re, im}` Map. Renamed from `Rect` to avoid confusion with the `rect` constructor (Go's `Rect` *is* the polar→rect builder). |
| `cmplx.Abs` | `abs` | `[Map] -> Float` | Magnitude (modulus) of a complex number. | `Map → Float`; `√(re²+im²)`. |
| `cmplx.Phase` | `phase` | `[Map] -> Float` | Argument (phase angle, radians) of a complex number. | `Map → Float`; `atan2(im, re)`. |
| `cmplx.Conj` | `conj` | `[Map] -> Map` | Complex conjugate (negate the imaginary part). | `Map → Map` `{re, -im}`. |
| `cmplx.Exp` | `exp` | `[Map] -> Map` | Complex exponential `e^z`. | `Map → Map`. |
| `cmplx.Log` | `log` | `[Map] -> Map` | Principal complex natural logarithm. | `Map → Map`. |
| `cmplx.Sqrt` | `sqrt` | `[Map] -> Map` | Principal complex square root. | `Map → Map`. |
| `cmplx.Pow` | `pow` | `[Map exp, Map base] -> Map` | Complex power `base^exp`. | `base Cmplx.pow exp` (infix), both operands `{re, im}` Maps → `Map`. |

Every `Map`-typed argument is reconstructed into a Go `complex128` as
`complex(re, im)` after reading the two Float keys via the value bridge;
every complex result is decomposed back into a `{re, im}` (or
`{r, theta}` for `polar`) Map.

## 5. Types

Scalars / Map only. **No opaque external handle** — the whole modeling
choice is to represent a complex number as a plain inspectable Map of
Floats rather than introduce a `complex128` payload type, so there is no
`RegisterExternalBuiltin` / FixedID allocation. Convert at the boundary
with `core.FromNative` / `core.ToNative` (`core/go/gobridge.go`):
Float↔float kinds, Map↔`map[string]any`. (This is exactly the friction
§10 weighs: a native Complex type, if it existed, would replace the Map.)

## 6. Errors

No panics (`core/go/CLAUDE.md` "Panic Prevention"; guard with
`RequireConcreteMap` and `AsConcreteFloat` before use). Failure is
signalled via `r.BoruError(code, detail, word)`:

| code | raised when |
|---|---|
| `bad-arg` | a complex arg is not a Map, or lacks numeric `re`/`im` keys (or `r`/`theta` for `from-polar`); a scalar arg is not a Float/Number. |

The `cmplx` functions themselves are total over `complex128` (they return
`Inf`/`NaN` components rather than erroring), mirroring how
`boru:math-util` treats IEEE specials — so there is no Go `error` to
unwrap; a `NaN`/`Inf` component simply rides in the result Map's Float.

## 7. Policy / capabilities

**None — pure.** Purely in-memory arithmetic. Runs under any policy.

## 8. Overlap

Touches `boru:math-util` (`MathUtil`) — which owns `math` and `math/big`
and the real-valued transcendentals (`sqrt`, `exp`, `log`, `atan2`, …) —
but does **not** move or change any of those words. The dividing line:
`MathUtil` operates on **real** Numbers and returns Numbers; `Cmplx`
operates on the `{re, im}` Map and returns Maps. The names deliberately
echo (`MathUtil.sqrt` vs `Cmplx.sqrt`) because they are the real and
complex versions of the same function; the namespace keeps them disjoint.

## 9. Examples (args-before form)

All args-before form (`re Cmplx.rect im` / `z Cmplx.abs`); never pure
forward.

```
import "boru:cmplx"

3.0 Cmplx.rect 4.0                 # → {re:3.0 im:4.0}   (re word im)
{re:3.0 im:4.0} Cmplx.abs          # → 5.0
{re:0.0 im:1.0} Cmplx.phase        # → 1.5707963267948966   (pi/2)
{re:3.0 im:4.0} Cmplx.conj         # → {re:3.0 im:-4.0}
{re:1.0 im:0.0} Cmplx.exp          # → {re:2.718281828459045 im:0.0}
{re:1.0 im:0.0} Cmplx.polar        # → {r:1.0 theta:0.0}
{re:0.0 im:1.0} Cmplx.pow {re:2.0 im:0.0}   # → i^2 ≈ {re:-1.0 im:~0.0}
"x" Cmplx.abs                       # ERROR:bad-arg   (not a {re,im} Map)
```

## 10. Open questions / out of scope — is this worth shipping?

**The Map-modeling friction is real, and this section is candid about it.**

- A complex number as `{re:3.0 im:4.0}` is verbose to write, easy to
  malform (a typo'd or missing key is a runtime `bad-arg`, not a type
  error caught early), and gives up the natural arithmetic boru has for
  numbers: `z1 add z2` does **not** add two complex Maps (Map `add` is
  not complex addition), so users would need `Cmplx.add`/`Cmplx.mul`
  words too — which this note has *not* included, widening the surface
  further. Without them the module can transform a complex number but not
  do complex *arithmetic*, which is an odd half-tool.
- The honest alternative is to **defer the whole module until boru has a
  native Complex type** (a leaf in the Number lattice with its own
  literal syntax and Comparer, the way Float/Integer/Big are). With a
  real type, `cmplx` words would take and return `Complex` values,
  arithmetic operators would just work, and the Map gymnastics vanish.
  That is a much larger language change, but it is the only thing that
  makes a complex-math module genuinely idiomatic.
- **Recommendation to weigh:** ship `boru:cmplx` only if a concrete user
  need for complex math appears *before* a native Complex type is on the
  roadmap; otherwise mark it deferred. It is in the roster flagged niche
  precisely so this decision is taken deliberately rather than by
  default.
- **Out of scope regardless:** the inverse-trig / hyperbolic complex
  functions (`Asin`, `Acos`, `Atan`, `Sinh`, `Cosh`, `Tanh`, `IsInf`,
  `IsNaN`) — the nine words above cover the common cases; the long tail
  waits for demand (and ideally a native type).

## 11. Implementation sketch

Wiring checklist — no Go code here. Reference: `lang/go/modules/math.go`
(`BuildMathModule`, the pure-module canonical builder).

- `lang/go/modules/cmplx.go` — `BuildCmplxModule(parent *native.Registry)
  (native.ModuleDesc, error)`: isolated `native.DefaultRegistry()`
  sub-registry, register a `CmplxNatives []native.NativeFunc` slice (each
  inner sig `BarrierPos: -1`), wrap each word as an `FnDef` export into an
  `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Cmplx": …}}`. Each handler reads `re`/`im` (or `r`/`theta`)
  Floats from the Map, builds a `complex128`, applies the `cmplx`
  function, and decomposes the result back into a `{re, im}` Map.
- Register `"cmplx": BuildCmplxModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_cmplx.go` — `registerDocs("boru:cmplx",
  map[string]string{…})` with a one-liner per export (else
  `TestModuleExportDocs` fails).
- `lang/spec/module-cmplx.tsv` — `input⇥expected⇥description` rows leading
  with `import "boru:cmplx"`; pin the round-trip (`rect` → `abs`/`phase`,
  `polar` → `from-polar`) and pair every positive row with an
  `ERROR:<substring>` sibling (a non-Map / missing-key `bad-arg`) per Test
  discipline (`lang/go/CLAUDE.md`).
- Boundary conversion via `core.FromNative` / `core.ToNative`
  (Float↔float kinds, Map↔`map[string]any`). No FixedID entry (no
  external type — the Map *is* the representation), no policy wiring.
