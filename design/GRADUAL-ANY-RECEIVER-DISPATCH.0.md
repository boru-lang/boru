# Gradual `Any`-Receiver Dispatch — Full Compilation

**Status:** Proposed. Design only; no compiler code changed by this document.

Companion reading: `FORWARD-COLLECTION-PHASES.10.md`, `FORWARD-COLLECTION-TRAPS.0.md`,
`legacy/PARAM-GUARD-SKIP-MISCOMPILE.0.ignore`, `legacy/dynamic-modality-report.10.ignore`, `legacy/P7-ENDGAME.10.ignore`.

---

## Context

The boru compiler lowers bytecode from a check-mode analysis pass. Under `--compile`
(the default mode, in which a program the compiler refuses is **silently** re-run on
the interpreter) the compiled program is contractually **byte-identical** to the
interpreter. That contract has one outcome, not two: all valid code compiles. A
refusal is not its other branch — it is an open defect, and the silent re-run only
hides that defect from the user. A dispatch over a strict `Any`-carrier **receiver**
breaks the contract outright:

```
def f fn [[xs:Any] [List] [(xs Sort.quick Sort.by-number end)]]
```

with `Sort.quick [comp:Function lst:List]` (comparator FIRST) and
`Sort.by-number [b:Any a:Any]`. Over a concrete `List` receiver it compiles
correctly (`PUSH xs; PUSH by-number(const); CALL quick`). Over an `Any`-typed
argument (so `xs` stays a strict `Any` carrier) it **inverts** to
`PUSH xs; PUSH quick(const); CALL by-number` → runtime
`cmp: cannot order List and Function`, while the interpreter always dispatches
`quick`. A silent-ish miscompile (the compiled program raises an error the
interpreter does not).

**Already shipped (`f85f0f72`):** check-prop gen/property bodies (stored-param
units) now use `ParamInputCarrier` (a dynamic/gradual carrier) instead of
`NewCarrier`, so their inputs match gradually. Safe because scoped to stored bodies
(genuinely runtime-typed inputs); passes every differential. All six voxgig-boru
libraries fully compile. This document addresses the *remaining* general case: an
ordinary user fn with an `Any`-typed param used as a dispatch receiver.

**Why the obvious fix is rejected.** Ungating the general `genArgs`
(`eng/go/core_helpers.go` ~1147‑1225) so ALL `Any`-into-`Any` params become gradual
**breaks the langspec census** (`test/go/langspec/compiled_coverage_test.go`
`TestCompiledCoverage`, a strict zero-divergence differential) with real
`Assert.equal: expected 1, got 2` miscompiles across many spec files. The comment at
`core_helpers.go` ~1198 already warned: "applied globally it flipped whole-program
main-pass rows (module-repl)." Confirmed empirically — the naive change was tried
and reverted.

## Root cause (precise)

- The receiver `xs` steps as a **literal** (`engine.go` ~2672) — a strict
  `NewCarrier(TAny)` lands on the stack.
- `Sort.quick`'s dispatch runs `matchSignature`. The forward scan collects
  `Sort.by-number` into the `comp:Function` slot; the **stack scan** tests
  `lst:List` against `xs` via `sigTypeMatches` (`eng/go/signature.go:269`).
- `sigTypeMatches` routes a **non-dynamic** carrier to strict `v.Is(t)` (line 299);
  `Any` does not conform to `List` → the stack slot rejects → `quick` matches **no**
  signature (`sig == nil`).
- With `quick` abandoned, its parked forward is stranded; the trailing function word
  `by-number` begins its **own** dispatch (`curryOrStack`/`implicitEnd`, `engine.go`
  ~7117 / ~5907) and greedily consumes `xs` + the `quick` const → inversion.

A **dynamic** carrier (`signature.go:279`, the not-disjoint rule) would match `List`
optimistically and commit `quick` — which is what the fix must achieve, soundly.

## The key distinction (what makes a scoped fix possible)

Three things look alike but are mechanically separate code sites:

1. **The forward/stack SPLIT decision** of the currently-dispatching word —
   `matchSignature`'s stack-scan admission (`engine.go` ~7770), forward-scan
   admission (~7533), and `resolveForwardArgs` `pruneViable` (~1850). This is where
   `quick` lives or dies. It operates on the **external operands** of the word.
2. **Interior body compilation** of a committed unit — `genArgs`
   (`core_helpers.go` ~1147‑1225) builds the callee body's carrier inputs. **This is
   the census-breaking site**: a gradual body input flips every interior
   `sigTypeMatches` optimistic (the `expected 1 got 2` miscompiles).
3. **Overload ranking** — choosing among a word's own same-arity sigs.

The interpreter's *dispatch target* (which word) is positional and
receiver-type-**in**dependent. The bug is (1) using the receiver's static type to
wrongly abandon the target. The census breakage is (2). Because (1) and (2) are
physically different functions with no shared state, we can widen the receiver match
at (1) without touching (2). This is the entire basis for a safe fix.

## Soundness model (machinery to reuse)

- **`checkParamContract`** (`eng/go/vm.go:2047`): the CALL_USER **entry guard**.
  Re-runs `sigTypeMatches` over the CONCRETE runtime value against the committed
  unit's DECLARED param types; raises `signature_error` on mismatch. Uses
  `sigTypeMatches` (not `v.Is`) so it matches the interpreter's runtime dispatch
  exactly (`vm.go` ~2060).
- **`OpCallUserPoly`** (`vm.go:543`, `matchUserPoly`): when ≥2 same-arity overloads
  of ONE word are statically reachable, compile all arms and re-run `MatchSignature`
  over concrete values at runtime — the interpreter's first-match. Gated by
  `tryCompileUserPolyArms` (`eng/go/user_poly.go`).
- **The dynamic not-disjoint rule** (`signature.go:279‑297`) — the exact optimistic
  match logic to reuse for the split decision.

## THE LOAD-BEARING INVARIANT (do not weaken)

`checkParamContract` makes committing the positional target sound **only when the
committed word's receiver slot is DISCRIMINATING** — a concrete, non-predicate,
non-refinement type.

- For a concrete slot `T`, "passes the guard" ≡ `sigTypeMatches(v, T)` ≡ the
  interpreter's stack-slot admission test. They coincide → commit is sound.
- For a **loose/`Any`** slot, the guard (`vm.go:2057`) short-circuits to a guaranteed
  pass. A value the interpreter's split would reject-and-unwind on passes the guard →
  the target can invert at runtime undetected → a **new** miscompile class (a second
  door to the census regression).
- For a **predicate/refinement** slot (`def Big (Integer gt 10)` threaded as a param
  *type*), the user-fn guard enforces the NOMINAL type only (see
  `legacy/PARAM-GUARD-SKIP-MISCOMPILE.0.ignore` and the carve at `engine.go` ~8349‑8357). A
  nominally-typed-but-predicate-failing value passes the guard yet the interpreter
  raises → unsound.

The reported bug's slot is `List` (concrete) → soundly fixable. The fully-general
"any `Any`-into-`Any` param" case is the loose-slot case where commit is **unsound**,
so this design must refuse it rather than miscompile it — and that refusal leaves the
shape uncompiled: an open defect owed a fix by some later mechanism, not a resting
place. **This precondition is an invariant, not an optimization.**

## Goal & non-goals

**Goal:** full compilation (no interpreter island) of a forward-collecting dispatch
over a gradual/`Any` receiver, committing to the interpreter's positional target,
**for the soundly-committable subset**: a concrete non-predicate receiver slot with
exactly one statically-reachable overload. Everything else still refuses — each such
refusal an open defect this design does not close.

**Non-goals — the limits of *this* mechanism, not the project's bar (which is that
all valid code compiles, no exceptions):** universal full compilation of every
`Any`-receiver dispatch, because committing on a loose or predicate slot is provably
unsound and so needs a different mechanism than the one designed here; any change to
`genArgs`/interior body carriers; any change to overload ranking; a generalized
non-terminal interpreter re-dispatch (which would reintroduce the interpreter
dependency `legacy/P7-ENDGAME.10.ignore` is deleting).

**Honest framing.** "The redesign" = full compilation for the concrete-slot,
single-overload subset (the reported bug and most real code) + **refusal** for the
genuinely-ambiguous loose/predicate/multi-overload subset. It lowers the census
frontier by the count of gradual-receiver-inversion rows; it does not reach zero for
the unsound subset, and that remainder is not something to sign off — those rows stay
open defects, each owed a fix by a later mechanism.

## Design — a cascade

**Phase 0 — stop the wrong answer (refuse; ship first, independently valuable).**
Add `refuseGradualReceiverInversion` beside the existing `refuseForwardStackDrift`
(`engine.go` ~3069), fired from the `sig == nil` branch (~2828, before
`checkModeAssumeSig`). Detect: a function word matched no signature where the sole
blocking slot was a **stack** position holding a strict param-origin `Any` carrier
(`v.Parent.Equal(TAny) && v.Carrier && !v.Dynamic && !IsDisjunct(v)`) AND a trailing
function word follows that would dispatch. `MarkUncompilable(...)`, after which the
runtime **silently** re-runs the program on the interpreter. That trades one defect
for another: the miscompile stops, and in its place the shape fails to compile and
says nothing about it. The refusal produces no wrong answer, but "not wrong" is not
the bar, and its **zero census risk** is only the census being unable to see a row
that never compiled. Does not lower the ceiling; it is scaffolding holding the
correctness hole shut while Phase 1 is built, not a fix for it. Landable on its own
as an urgent stop-gap.

**Phase 1 — the real fix (scoped split-widening).**
Introduce `sigTypeMatchesSplit(v, t)` — identical to `sigTypeMatches` except it
admits a **strict param-origin `Any` carrier** via the not-disjoint rule
(`signature.go:279‑297`) *without* mutating the value's `Dynamic` flag. Call it
**only** at the three split sites — `engine.go` ~7770 (stack admission, primary),
~7533 (forward admission), ~1850 (`pruneViable`) — and **only** during a compile
pass over a param-origin `Any` receiver. `genArgs` stays strict, so interior body
dispatch is untouched → no census regression.

Gate the commit on the soundness invariant:

- **Single reachable overload** of the widened word
  (`dynamicReachableOverloadCount(r, word, args) < 2`, `carrier.go` ~2436) — else
  Phase 2.
- **Concrete, non-predicate, non-refinement** receiver slot (`!Equal(TAny)`,
  `!IsDepScalar`, not a bare-refine nominal) — else Phase 0 refuses.
- **Diagnostic parity**: the committed word's `checkParamContract` failure must
  produce the interpreter-identical error diagnostic (holds for a concrete slot;
  assert in a differential).

Runtime soundness is then `checkParamContract` at the committed unit's entry —
already present, no VM change.

**Phase 2 — widen coverage (poly for the ≥2-overload sub-case).**
When the widened receiver makes ≥2 same-arity overloads of the same word reachable,
route through `tryCompileUserPolyArms` (`user_poly.go`) → `OpCallUserPoly` re-selects
the interpreter's first-match at runtime. Cross-word inversion with a multi-overload
target stays refused (Phase 0) — still an open defect after this cascade, not a case
closed by it. **Do not** build a generalized non-terminal re-dispatch.

## Edge cases (must be handled/tested)

- **Multi-overload comparator-first fn** (the primary trap): widened receiver makes
  ≥2 overloads reachable → static rank ≠ interpreter's runtime first-match → route to
  Phase 2 or refuse. Gate on `dynamicReachableOverloadCount`.
- **Disjunct receiver** (already refused — do not disturb here): `sigTypeMatches`'
  strict-disjunct branch (`signature.go:307‑325`) + `AmbiguousGradualSplit`
  (`engine.go` ~7362) already refuse the mixed case, leaving that shape uncompiled
  and owed a fix of its own. Scope the widening with `!IsDisjunct(v)`.
- **Nested `(a (b Fn1 Fn2) Fn3)`**: inner commit's return carrier feeds the outer
  split; compounding optimism. Bound the widening to **param-origin** `Any`
  (traceable via `SetDynFrom` provenance, `engine.go` ~2670), not arbitrary computed
  `Any`.
- **Genuinely-ambiguous receiver where even the interpreter errors**: the committed
  word may raise at a *different word* than the interpreter's strand error → a
  **divergent error row** (census is byte-identical). Holds for the concrete-`List`
  slot (both blame the List mismatch); verify per shape.
- **`strictForwardBarrier`** interaction: widening changes whether the "function word
  barrier" (`engine.go` ~6071, see `FORWARD-COLLECTION-TRAPS.0.md`) fires. Audit
  barrier test rows with `Any` receivers.

## Validation surface (every phase must pass)

- `test/go/langspec`: `TestCompiledCoverage` (zero-divergence census — must not
  regress; ceiling should DROP in Phase 1), `TestSpecCompiledDifferential`
  (`≥ minCompiledRows`), `TestVariationDifferential` (`BORU_VARY_SEEDS=100 …`),
  `TestOnlyMetaFallsBack` (tier ratchets). Run: `cd test/go/langspec && go test ./...`.
- `lang/go`: `TestRunCompiledStrict` (force-compile), the `Disassemble()` opcode pins
  in `lang/go/bytecode_*_test.go`.
- `make cover-gate` (100% floor; new statements need covering tests in
  `lang/go/bytecode_*_test.go`).
- `make fmt && make vet && make lint && make test`.
- **Voxgig differential**: rebuild `boru`, re-run the six libraries' four-surface
  harness (interpret / check / `--compile` parity / `--force-compile`); expect no
  regression and the general shape now compiling.
- **New pins** (`lang/go/bytecode_gradual_receiver_test.go`): the general shape
  `def f fn [[xs:Any] [List] [(xs S.quick S.by-number)]]` over an `Any`-typed arg
  fully compiles (no `FALLBACK` in disassembly) + parity; a negative pin recording
  that a loose-`Any`-slot / multi-overload / predicate-slot shape **refuses** — the
  program then being **silently** re-run on the interpreter — rather than
  miscompiling, the pin tracking that refusal as an open defect rather than blessing
  it; the concrete-vs-`Any` receiver select the identical target.

## Risks

- **New miscompile class via the loose-slot door** — mitigated by the
  concrete-non-predicate-slot invariant + Phase 0 refusal; the census is the oracle.
- **Overload-selection re-entry inside the widened word** — mitigated by the
  single-reachable-overload gate.
- **Error-diagnostic divergence** on the error path — mitigated by the
  diagnostic-parity gate + differential.
- **Provenance leakage** (widening a computed `Any`, not a param-origin one) —
  mitigated by the `SetDynFrom` param-origin restriction.

## Critical files (for the implementation that follows this design)

- `eng/go/signature.go` — new `sigTypeMatchesSplit` beside `sigTypeMatches` (269).
- `eng/go/engine.go` — split-scan call sites ~7770 / ~7533; `resolveForwardArgs`
  prune ~1850; Phase-0 refusal guard at the `sig == nil` branch ~2828 (new fn near
  ~3069).
- `eng/go/vm.go` — `checkParamContract` (~2047): no change; it is the reused guard.
- `eng/go/user_poly.go` — Phase-2 routing via `tryCompileUserPolyArms`.
- `eng/go/carrier.go` — reuse `dynamicReachableOverloadCount` (~2436).
- **DO NOT touch** `eng/go/core_helpers.go` `genArgs` (~1147‑1225) — the
  census-breaking site; interior body carriers stay strict.

## Verification (end-to-end, for the eventual implementation)

1. Repro before/after with a built `boru`: `boru run --force-compile` (exit 0 = full)
   and `--no-compile` vs `--compile` parity on
   `def g fn [[x:Any][Any][x]] def f fn [[xs:Any][List][(xs Sort.quick
   Sort.by-number end)]] (([7 9 11 5 8] g) f)`.
2. `cd test/go/langspec && go test ./...` — census/differentials green; note the
   `TestCompiledCoverage` ceiling delta.
3. `make fmt vet lint test cover-gate` from repo root.
4. Voxgig four-surface harness across the six libraries.

## Line-number caveat

The `~NNNN` anchors above are approximate and drift as `eng/go` evolves. Resolve
each by symbol name (`sigTypeMatches`, `resolveForwardArgs`, `matchSignature`,
`curryOrStack`, `checkParamContract`, `tryCompileUserPolyArms`,
`dynamicReachableOverloadCount`, `refuseForwardStackDrift`) before editing.
