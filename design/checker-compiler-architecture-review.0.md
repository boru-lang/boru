# Checker ↔ Bytecode-Compiler Architecture Review and Plan

Status: **largely implemented** (2026-06, branch
`claude/pensive-thompson-vv4387`). This was a `.0` proposal; the bulk of it has
now landed across 11 gate-clean commits. **Read the new
[Implementation report](#implementation-report-2026-06) and
[Completion guide](#completion-guide-remaining-refusals--the-path-to-p7) first** —
they record what landed, what was deliberately *not* done (with reasons), and
what the next session should do. The original analysis (§0–§10) is preserved
below, with inline **LANDED** / **NOT DONE** annotations.

**Follow-on review session (2026-06, branch `claude/lucid-davinci-ajtxt5`).** A
critical re-read verified every claim below against the live tree (numbers exact,
gates green, opcodes sound) and acted on its own recommendations across more
gate-clean commits:

- **§4.5 finished** — the last name-keyed table, `callableWords`, was the only
  remaining instance of the eng↔module coupling §4.5 set out to remove (it
  hard-coded the `boru:test` words `test-test` / `test-describe`). It is now a
  word-level `Signature.Callable` (`*CallableSpec`) declaration; eng names no
  callable word. See the corrected §4.5 note.
- **Trailing fn-value boundary** (completion-guide #1, the "biggest lever") —
  `5 m.f`, `[..] r.one-of` now compile via the new `OpCallDynamicTrailing`.
  Ratchet **73 → 71**.
- **Quoted-operand inert words** (completion-guide coverage cluster) —
  `quote` / `codequote` / `raise` / `timeout` / `interval` declare a new
  `CompileQuoteInert` effect, so the recorder bakes their inert quoted operand
  (a symbol, or a code body held as data) + `CALL_NATIVE` instead of refusing —
  the declarable analogue of the get/getr/set exemption. Cleared 7 of the 10
  quoted-operand rows. Ratchet **71 → 64**. The 3 `timeout`/`interval` rows whose
  code BODY reaches the check as a carrier (no recoverable inert provenance) stay
  refused — a §4.3 / `materialise` gap, not the flag.
- **`rand-list-of` generator body** (completion-guide code-body cluster) —
  `Rand.list-of [body] n` runs a 0-input generator body n times; it now declares
  a `CallableSpec` (the same closure shape as `do`) and its handler runs the
  compiled closure via `InvokeBody`. Ratchet **64 → 63**. The other 5 code-body
  rows need distinct, more-involved mechanisms (see the completion-guide note).
- **Value-arm `if`** (completion-guide if-branch cluster) — `if cond v1 v2` with
  VALUE arms (not `[body]` code) refused "then-branch not captured" because the
  then arm was run as a body (nil fragment) while only the else handled a value.
  Made the then arm symmetric with the else (a value-then pushes its value, like
  value-else). Cleared the direct form AND the usurp-if shape (`usurp if`
  dispatches `if` with value arms) — 3 rows. Ratchet **63 → 60**. The 3 remaining
  if-branch rows are NOT this: 1 computed-else/variadic-statement-if (the
  variadic-merge refactor) + 2 cross-fn `break`/`continue` (a soundness boundary
  — see below).
- **`returnsof` static return type** (completion-guide dynamic cluster) —
  `TypeUtil.returnsof` reported its output as the opaque `TAny` (dynamic), so the
  dispatch refused as a dynamic output — unlike `arityof`/`typeof`, whose concrete
  output bakes. A `ReturnsFn` now computes the precise return type for a concrete
  fn (the same value the handler produces), so the output is a concrete type node
  and bakes a `CALL_NATIVE`. Ratchet **60 → 59**. This was the *one* cleanly
  tractable row in the dynamic cluster; the other 16 are the hard soundness
  frontier (see the completion-guide note).
- **Doc precision** — two overstatements in the original report are corrected
  inline (the §4.1a "`layoutOperands` deleted / unreachable by construction"
  claim, and §4.5's "all five tables migrated"), and the P7 distance is stated
  honestly. The `refusalCeiling` history was moved out of the test into
  [§11](#11-refusal-ceiling-decrement-history) below.

**Continuation session (2026-06, branch `claude/bytecode-compiler-review-ai3l9u`).**
Five more gate-clean landings took the ratchet **16 → 11** (every landing 0
divergences across the differential + whole-corpus + combination + property-fuzz
gates, `-race`, and `-tags borudebug`; islands stayed **0**). Newest first:

- **patrun fn-value store** (`function value reaches word`) — `add {cmd:"sum"}
  (=>…) pm` stashes a fn in a Patrun dispatch table and never invokes it on the
  VM tape. The `add` overload `(Map, Any, Patrun)` now declares `CompileStoresFn`
  (the existing minilang/parselang flag), so a PURE fn literal rides as an inert
  const; a capturing closure still declines at `isInertConst` and falls back.
  **12 → 11**.
- **mixed fn-value-call boundary** (`OpCallDynamicMixed`) — the 2-arg mixed apply
  `3 m.f 2` (a dynamic fn INTERIOR to the residual `[3, m.f, 2]`: forward `2` →
  sig[0], stack `3` → sig[1]). The leading/trailing layouts can't express args
  straddling the fn; the new op islands the whole `[before…, fn, after…]` window
  verbatim — the same token sequence the interpreter ran, so the fn auto-applies
  (or, if non-callable, stays put) for any arity/split. The fn's producing event
  is promoted to a frame local so the window re-pushes in source order. This
  **completes** the fn-value-call boundary that completion-guide #2 flagged.
  **13 → 12**.
- **sub-registry poly dispatch** — a module word with a dynamic input
  (`StructUtil.getpath $.a.b (StructUtil.setpath …)`) refused because
  `tryRecordPoly`'s CORE-dispatch guard required a MAIN-registry builtin.
  `tryRecordPoly` now threads an `ownerReg` (the sub-registry, carried on
  `MatchResult.Reg` from `execFnDefLiteral`'s trivial-delegation dispatch) and
  records a `PolyRef` carrying it; `callPoly` re-matches over that registry. The
  sub-registry pointer is the same object the check pass shares with `RunProgram`.
  **14 → 13**.
- **conditional dynamic apply in a branch** — `if (n eq 0) [99] MathUtil.sqrt 16`
  where a branch arm is a fn value applied to a trailing arg. A trivial-delegation
  module fn value now bakes as an inert const, and a branch result that MAY be a
  fn carries a `mayBeFn` event flag so a leading-branch-result over static args
  lowers to a runtime-conditional `OpCallDynamic`. **15 → 14**.
- **dispatch-recovery operand order** — `(3 and "x") add 1`: a strict-disjunct
  straddle that `matchSignature` fails and `checkModeAssumeSig` recovers now
  records `OpCallNativePoly` (operands rebuilt into signature order) instead of
  latching uncompilable. **16 → 15**.

**Investigation (not landed) — `module-rand.tsv:38`**, `def r (Rand.with-seed 2)
r.list-of [body] 3`: looked like the "lowest-risk" residual-lowering row but is
actually a **method-fn-value-apply boundary**. A seeded `Rand` instance is a Map
of trivial-delegation wrappers each carrying its OWN sub-registry closing over a
private `randState`; `r.list-of` is a runtime fn value bound to that generator.
Because it is *resolvable*, the checker applies it (producing the `[Integer]`
return type) but **never reaches `carrierResults`/`RecordCall`** — so the result
is an orphan residual ("residual value of unknown provenance"). The top-level
`Rand.list-of` works only because it is a STATIC module fold (→ `CALL_NATIVE`
rand-list-of + `PUSH_CLOSURE` body). Clearing it requires the check-mode apply of
a resolvable delegation method-fn-value to *record* a compilable dispatch
(closure body + a sub-registry `CALL_NATIVE` run against the instance's seeded
generator) — fn-value-apply + sub-registry + code-body intersection, medium risk
(RNG-draw faithfulness), NOT a scheduling one-liner.

**Recommendation: bank at 11.** The easy seam is mined out — each remaining row
needs a sizeable refactor (branch-result-modeling, the method-fn-value-apply
recording above, parselang/Test.run-spec residual ordering) or touches a flagged
hazard (the IO 0-output context-mutation cluster, which produced a divergence
when tried). See the refreshed [Completion guide](#completion-guide-remaining-refusals--the-path-to-p7).


This note is a router + analysis for the next person (or session) working on the
boru bytecode compiler. It captures (a) how the checker and the bytecode compiler
actually interact today, (b) the current state of the runtime-independence
ratchet, (c) concrete generalizations / simplifications / data-structure
improvements, and (d) the state-of-the-art algorithms we are reinventing ad-hoc
and could adopt instead. It is meant to be read alongside
`design/boru-bytecode-runtime-independence.0.md` (the P5–P7 work plan, now stale
on numbers — see "Current state" below) and the two module guides
(`eng/go/CLAUDE.md`, `lang/go/CLAUDE.md`).

---

## Implementation report (2026-06)

The doc's core thesis **held**: the architecture is sound, and incremental
cleanup + a destination model — *not* a rewrite — was the right call. Eleven
gate-clean commits on `claude/pensive-thompson-vv4387` took the ratchet
**82 → 73 refused, islanded 5, differential 0 mismatches throughout** (full gate
per commit: `fmt/vet/lint/test` × 35 packages + coverage + differential +
`TestSpecCompiledOrFallback` + `TestOnlyMetaFallsBack` + alloc ceilings).

| Commit | §ref | What landed | Ratchet |
|---|---|---|---|
| `a6e30e2` | §4.7 | Delete dead `SchemaArg`/`materialiseSchema` (confirmed no callers) | — |
| `c8e46d5` | §4.4 | 4 per-event side-maps → one `eventInfo` map | — |
| `d8d5b94` | §4.3 | Merge `RecordStrip`+`RememberOriginal` (light — see below) | — |
| `eb6f3b1` | §5 | User-fn return-count error path via exact `OpRet` | 82 → 79 |
| `9ee0a4c` | §5 | `OpTrap` for orphan-`gen` / `unpack`-missing-key | 79 → 74 |
| `b4d985d` | §6 | Root-cause axis on the ratchet (`correct-error` asserted 0) | — |
| `f415ef1` | §4.5 | `Signature.CompileEffect` bitfield; migrate `fnStoreWords` | — |
| `c9e72a3` | §4.5 | Migrate `fnIntrospectionWords`; split `ReadsFn`/`StoresFn` | — |
| `0b41ddd` | §4.5 | Migrate the `carrier.go` word tables (ModuleFold/IslandPure/FallbackBody) | — |
| `380e80b` | §4.1b | `OpReverse` stack-scheduling op (N-event reverse layout) | 74 → 73 |
| `57dcbb8` | §4.1a | **Spill-to-local DDCG seating — `layoutOperands` special-casing deleted** | — |

**Per-recommendation outcome** (also annotated inline in §4–§6):

- **§4.7** — **LANDED.** Dead code; deleted.
- **§4.4** — **LANDED (consolidated, not moved).** A true per-`emitEvent`-struct
  move is *unsafe* (events are value-copied into frames/fragments — a flag set
  after append never reaches the copies), so the four maps became one seq-keyed
  `eventInfo`. **Integer value handles deferred:** `Value.ID` is a kernel-wide
  string, so handles need a field on `Value`.
- **§4.3** — **PARTIAL (light).** Merged the two strip recorders into one. The
  full carrier back-pointer was **declined as net-negative**: `origByID` is
  already recording-scoped (zero cost in normal check mode) and clears *none* of
  the live provenance refusals, so a `*Value` field on the hot `Value` type was
  not justified.
- **§5** — **LANDED.** Both error paths compile now; the `correct-error`
  root-cause bucket is **0**. The count case needed no new opcode (extended
  `OpRet` from underflow-only to exact-count via a frame stack-base, scoped to
  user-fn frames so closures still raise `each_error`); the suppression case
  added `OpTrap`.
- **§6 meta** — **LANDED.** `rootCause` second axis on the histogram.
- **§4.5** — **LANDED (completed in the follow-on session).** A capability
  *bitfield* (`CompileReadsFn`/`StoresFn`/`ModuleFold`/`IslandPure`/`FallbackBody`);
  a word-level `NativeFunc.CompileEffect` OR'd into each sig keeps per-word
  classification to one declaration. Five capability tables migrated to the
  bitfield; `checkModeLiteralWords` **kept** (consulted pre-dispatch on raw
  tokens, where no matched sig exists). **Correction to the original note ("all
  five migrated"):** the §4.5 problem statement listed *seven* tables, and the
  seventh — `callableWords` — is NOT a capability flag (it carries structural
  per-word data: the body operand position, output count, and an `inputs`
  closure), so it could not fold into the bitfield, and it still hard-coded the
  `boru:test` module words `test-test` / `test-describe` — the exact eng↔module
  coupling §4.5 targeted. The follow-on session lifted it to a word-level
  `Signature.Callable` (`*CallableSpec{BodyPos, BodyOut, Inputs}`, copied from
  `NativeFunc.Callable` at registration, read back via the resolved
  `sig.Callable`). eng now names **no** callable word — the core transforms
  declare in `lang/native`, the `boru:test` bodies in their own module. The
  package-global `callableWords` map is deleted.
- **§4.6** — **NOT DONE.** The `carrierResults` try-chain collapse is *not
  cleanly feasible*: the fallthrough is load-bearing — the same word (`get`)
  takes static-index-fold / module-fold / island / plain-call paths by operand
  *shape*, not by its declared effect.
- **§4.1/§4.2 big bet** — **LANDED (headline).** `OpReverse` + `spillSeat` turn
  the `layoutOperands` ladder's *hard* cases (the `reorder`/`shapeBeyond`/
  `notAdjacent` shapes that used to refuse outright) into a spill: a hard operand
  shape spills its event operands to **frame-local destinations** (`OpStoreLocal`)
  and re-pushes in sig order — DDCG's "data destination = a frame slot",
  generalized from the existing `planValueDefLocals` promotion. **Precision (a
  correction to the original wording "ladder deleted / unreachable by
  construction"):** the `layoutOperands` 0/1/2/N ladder is NOT deleted — it still
  exists, and its hard cases now *delegate to* `spillSeat` rather than refusing.
  `spillSeat` itself still *declines* in two cases (fewer stack slots than event
  operands, or a genuinely non-operand value interleaved on top), so the refusal
  class is not literally "unreachable by construction" — it is empirically **0 on
  the current corpus** (verified: no operand-shape bucket in the histogram), and
  the spill's success path is parity-tested directly (`bytecode_findings_test.go`,
  the `[1 (2 add 3) 4]` interleave) since the corpus does not drive it. **Not
  needed:** the §4.2 `valueNumber` reframe (`producedBy` already *is* the
  value-graph DAG); recording-side destination-threading and `DUP`/`TUCK`/`ROT`
  (the spill subsumes them — no current-corpus payoff).

**Honest verdict on the big bet.** Its payoff was **structural, not numeric**:
the operand-layout refusals were already at 0 (cleared by `OpReverse`), so
deleting the special-casing moved the ceiling by ~0 — but it made the whole class
*unreachable as the corpus grows*. The large ceiling drop the doc predicted for
DDCG lives in the *remaining* refusals, which are **feature-entangled, not
operand-layout** — see the completion guide.

---

## Completion guide (remaining refusals + the path to P7)

Live histogram (verify with `go test -run TestCompiledCoverage -v`):
**2869 rows — 2541 compiled (0 islanded), 317 check-errors, 11 refused.**
Root-cause axis: **soundness 7 · scheduling 3 · coverage 1 · opcode 0 ·
correct-error 0.** (Was 46 at the start of the continuation session, before the
dispatch-recovery operand order (→ 15), conditional dynamic apply in a branch
(→ 14), sub-registry poly (→ 13), the mixed fn-value-call boundary (→ 12), and
the patrun fn-value store (→ 11) landed. **Islands are 0** — every compiled row
runs entirely in the VM. The remaining 11 are the hard residue; see below.)

| n | bucket | root cause | what it actually needs |
|---|---|---|---|
| 4 | operand provenance | soundness | a fn BODY (or method-apply) result the residual cannot materialise. **`bytecode-combinations.tsv:74`** — a returned CAPTURING closure (`mk2`) applied in a `for`. **`def-node-binding.tsv:54`** — a fn body `[c1]` returning a list built from a param under def-snapshot binding semantics (the param does NOT shadow a module-level `c1`). **`recursion.tsv:72`** — a fn body `[n]` that is a dynamic-scope name lookup. **`module-rand.tsv:38`** — a method-fn-value apply (`r.list-of [body] 3` on a seeded instance), investigated below. Each is bespoke; the §4.3 back-pointer was measured and is NOT justified |
| 3 | dynamic input | soundness | **`flex.tsv:138`** flex-list mutation (`drop l`). **`module-io.tsv:29/30`** the IO context-store `set` receiver — a 0-output CONTEXT-MUTATION poly PLUS an `IO.read` fn-value-apply boundary; tried and **produced a divergence**, deferred (the doc's flagged side-effect hazard) |
| 3 | residual lowering | scheduling | **`module-parselang.tsv:23`** ("call result above a literal") and **`module-test.tsv:38`** ("call results reordered") — large multi-import `ParseLang.register` / `Test.run-spec` programs needing the residual-promotion machinery; plus one further residual-ordering row (per the histogram) |
| 1 | branch leaves extra values | coverage | **`recursion.tsv:53`** — a fn returning MULTIPLE values via a recursive branch (`[n mul 2 m (n sub 1)]`). Needs branch-result-modeling (single-result branch lowering today) |

**The fn-value-call boundary, the value-arm/usurp `if`, the sub-registry poly,
the dispatch-recovery straddle, and the conditional-branch dynamic apply are all
CLEARED** (continuation session) — they are no longer in this table.

**Priority order for the next session** (highest leverage first) — note the easy
seam is mined out; every item below is a refactor or touches a flagged hazard:

1. **Branch-result-modeling** (the variadic-merge refactor) — still the top
   structural lever. It clears the multi-result-branch row (`recursion.tsv:53`,
   `fn m` returns a recursive multi-value) by representing a 0-or-1 (or N) branch
   result as a first-class value a downstream consumer can absorb (today only the
   program residual can). A refactor of `lowerArms` / `lw.variadic`, not a patch.
2. **Method-fn-value-apply recording** (`module-rand.tsv:38`). A seeded `Rand`
   instance is a Map of trivial-delegation wrappers each carrying its OWN
   sub-registry closing over a private `randState`; `r.list-of [body] 3` is a
   runtime fn value applied over a code body. Because the method is *resolvable*,
   the checker applies it (yielding the `[Integer]` return type) but **never
   reaches `carrierResults`/`RecordCall`**, so the result is an orphan residual.
   Clearing it needs the check-mode apply of a resolvable delegation
   method-fn-value to record a compilable dispatch — a closure body + a
   sub-registry `CALL_NATIVE` run against the instance's seeded generator (the
   sub-registry poly + `OpCallDynamicMixed` machinery is the toolkit; the gap is
   the RECORDING of the method apply). Medium risk (RNG-draw faithfulness).
3. **Residual ordering for the multi-import programs** (`module-parselang.tsv:23`,
   `module-test.tsv:38`). Tagged scheduling — the fallback is faithful and a fix
   is differential-verified — but these are large programs whose residual has a
   call result above/around a literal that the in-order reconciliation rejects;
   the forceOrder/local-promotion machinery (used for the simpler residual rows)
   may extend to them, but it is not a one-liner.
4. **The bespoke operand-provenance bodies** (`bytecode-combinations.tsv:74`
   capturing-closure-returned-and-applied, `def-node-binding.tsv:54`
   list-of-param + def-snapshot, `recursion.tsv:72` dynamic-scope name). Each is a
   distinct binding-semantics subtlety, easy to diverge on — investigate per-row
   only if one blocks a wider cluster.
5. **Dynamic-input mutation cluster** (`flex.tsv:138`, `module-io.tsv:29/30`) —
   the **highest-risk** remaining work. The IO rows need a 0-output
   context-mutation poly (the flagged side-effect hazard) AND an `IO.read`
   fn-value-apply fix; a first attempt produced a divergence. Treat as a deliberate
   soundness effort, not a quick win.

**P7 (delete `OpFallback`)** is gated on `refusalCeiling` reaching 0;
`islandCeiling` is **already 0** (no row islands — the earlier `case` /
`each`-over-map islands all cleared). So the remaining distance to P7 is exactly
the 11 refusals above.

**Honest distance to P7 (do not read "11" as "almost there").** Of the 11,
**7 are soundness-gated** (4 operand-provenance bespoke bodies + 3 dynamic-input
mutation), **3 are residual-ordering scheduling** (large multi-import programs),
and **1 is the multi-result branch** (branch-result-modeling). None is a
cheap/bounded win like the ones banked this session — each needs a refactor
(branch-result-modeling, method-apply recording, residual-promotion extension) or
touches a flagged hazard (context-mutation poly). The incremental gate-clean
ratchet remains the right vehicle, but **the recommendation is to bank at 11**:
the next decrements are substantial, per-cluster efforts that each warrant their
own session budget. Re-run `go test -run TestCompiledCoverage -v` for the live
histogram before starting.

### Dead ends / proven not worth it (save the dig)

- **§4.6 try-chain collapse** — fallthrough is load-bearing (operand-shape-dependent path).
- **§4.4 integer value handles** — blocked by the kernel-wide string `Value.ID`.
- **§4.3 full back-pointer** — net-negative as a pure refactor; only revisit if it clears provenance rows.
- **DDCG recording-side threading / `DUP`/`TUCK`/`ROT`** — no payoff (the spill subsumes them; operand-layout refusals already 0).
- **fn-body container assembly** (the pre-existing reverted dead end, §2) — the scope-resolution divergence is unchanged; do not retry without solving it.

### A correction to §7's verification list

`TestSpecCompiledOrFallback` and `TestOnlyMetaFallsBack` **exist** and are
load-bearing (error-taxonomy parity + the P7 partition ceilings). The
`TestCompiledConcurrent` named in §7 **does not exist**; use `go test -race` on
the existing suites for the race check.

---

## 0. TL;DR (the verdict)

The core design is **sound and modern — do not rewrite it.** boru's "the compiler
*is* the carrier checker with a recording side-effect" is exactly the
**single-pass abstract-interpretation baseline-compiler** architecture that every
production WebAssembly engine converged on (Titzer's V8 / Wizard work). The
checker already runs an abstract interpreter over an operand stack; emission
piggybacks on the analysis it already does. That foundation is right.

What has accumulated on top of it is:

1. **Ad-hoc reinventions of well-studied algorithms** — the lowerer hand-codes
   the *stack-scheduling* problem (DAG → stack machine), and the provenance
   layer hand-rolls *value numbering*. There is mature literature for both
   (destination-driven code generation; Koopman stack scheduling; SSA/GVN).
2. **Scattered state and classification tables** that make the system harder to
   extend than it should be — ~7 name-keyed "how does this word compile?"
   tables, ~10 parallel side-maps on `EmitState`, and a strip→remember→
   materialise dance that exists only to undo information the checker threw away.

The recommendation is **incremental cleanup first** (low-risk, gate-clean), then
a **one bigger bet** (reframe the trace as a value graph and lower it with
destination-driven codegen) that is what would actually drive the remaining
refusals toward zero *and* delete the `layoutOperands` special-casing.

---

## 1. How the checker and compiler interact today

### The single channel: `CheckState.Emit`

The entire coupling is one field (`eng/go/registry.go`, `CheckState.Emit
*EmitState`). When it is `nil` you get a plain type-check; when it is set to a
`NewEmitState()`, the *same* dispatch machinery additionally records what it
does. The dispatch funnel is `carrierResults` (`eng/go/carrier.go`), which ends
in a chain:

```
tryFoldStaticIndex → tryFoldModuleConst → tryRecordClosure →
tryRecordPoly → tryRecordFallback → RecordCall
```

and then `EmitState.Finalize(residual)` (`eng/go/emit.go`) linearises the
recorded event trace into a `Program` (`eng/go/bytecode.go`). The host entry
point is `(*Boru).CompileCheck` (`lang/go/boru.go`).

### What the checker hands the compiler

- **The resolved signature** (monomorphic dispatch). The checker selects the
  overload via `matchSignature`; the compiler bakes that as `CALL_NATIVE`
  instead of re-dispatching.
- **Operand provenance**, carried on `Value.ID`. `resolveOperand` (`emit.go`)
  maps a value to (a) an earlier event's output via `producedBy map[string]
  producer`, (b) a frame local, (c) a canonical type node, or (d) an inert
  const. Unknown provenance ⇒ `MarkUncompilable`.
- **Inferred return carriers** — `Signature.Returns` / `ReturnsFn`. The
  `Dynamic` flag means "statically unknown" (opaque ⇒ refuse).
- **Whole-program fn analysis** — `FnSummaries` + `AnalyseFnBody` fixed points.
- **Out-of-band facts** — `SuppressedRuntimeError`, `RecordContextSet`/
  `LookupContextType`, and the `Compilable`/`Reason` latch.

### Key data structures (the "IR")

- The trace: `EmitState.frames [][]emitEvent`; an `emitEvent` is one of
  `evCall / evBranch / evLoop / evCallUser / evFallback` (+ `makeList` /
  `makeMap` flags on `emitCall`).
- The operand: `emitOperand{kind, idx, resIdx, closureUnit, closureCaps}` with
  kinds `opConst / opEvent / opLocal / opType / opClosure`.
- Provenance: `producedBy map[string]producer{seq,idx}` — value-ID → producing
  (event, result-index).
- The lowerer: `lowerer.vm []vmSlot{seq,idx}` — a *simulated operand stack* that
  re-derives stack positions, with `layoutOperands` arranging operands and
  inserting at most one `SWAP`.

**Read these first in a new session:** `eng/go/emit.go` (the recorder + lowerer
data structures, ~2400 lines), `eng/go/lower.go` (`layoutOperands`,
`lowerCall`, `planValueDefLocals`), `eng/go/carrier.go` (`carrierResults` and
the try-chain), `eng/go/callable_words.go` (closure-body compilation).

---

## 2. Current state (numbers, June 2026)

**Numbers below are the pre-implementation (82-refused) snapshot — kept for the
breakdown prose. The CURRENT live state is 64 refused; see the
[Completion guide](#completion-guide-remaining-refusals--the-path-to-p7) for the
up-to-date histogram and root-cause split.**

The ratchet is **far** ahead of the prose in
`design/boru-bytecode-runtime-independence.0.md` (which still says 459 / 15). The
review-session `test/go/langspec/compiled_coverage_test.go` snapshot:

```
2769 rows — 2375 compiled (5 islanded), 312 check-errors, 82 refused
```

Refusal histogram at 82 (the buckets are normalised by `normaliseReason`):

| n  | bucket | nature |
|----|--------|--------|
| 20 | operand provenance | heterogeneous long tail (see below) |
| 12 | dynamic/opaque output | soundness-gated |
| 10 | quoted-operand word | meta words (usurp / ref family) |
| 8  | residual lowering (Stage 1 limit) | stack scheduling |
| 6  | code-body word (NoEvalArgs) | mostly boru:test property words (2-body) |
| 6  | if-branch lowering | variadic-statement-if, computed-else, usurp-if |
| 5  | dynamic input | soundness-gated |
| 5  | suppressed runtime error | *correct refusals* — must emit error path for P7 |
| 3  | multi-return fn | *correct refusals* — count-mismatch error rows |
| 2  | user fn call (Stage 3) | meta |
| 1  | dispatch recovery (best guess) | meta |
| 1  | fn-value-call boundary | meta |
| 1  | function value reaches word | one non-bakeable fn value |
| 1  | operand shape (Stage 1 limit) | stack scheduling |
| 1  | other: "branch leaves extra values" | branch lowering |

The "operand provenance" bucket (20) breaks down into: generic-`make` in fn
bodies (~3), fn-body provenance (capturing lambdas, list-of-param) (~3), dynamic
module outputs (minilang/parselang/rand) (~4), module-type get (~3), macros
(~1), and assorted (`def m {a:g}`, `x/r`, `x/u`) (~3).

**The two downward ratchets** (gates on P7 = deleting the interpreter fallback):
`refusalCeiling` and `islandCeiling`, both in `compiled_coverage_test.go`. Both
must reach 0 before the `OpFallback` machinery can be deleted. This §2 block is
the *82-baseline* snapshot; the histogram above is frozen at that point for
continuity. **Current live values: `refusalCeiling` = 11, `islandCeiling` = 0**
(see the [Completion guide](#completion-guide-remaining-refusals--the-path-to-p7)
for the up-to-date 11-row breakdown and §11 for the decrement trail). The
`suppressed runtime error` (5) and `multi-return fn` (3) buckets above are
**gone** — they now compile error programs (§5, LANDED), and the `operand shape
(Stage 1 limit)` row is gone too (§4.1, LANDED).

### What landed in the June-2026 review session (for continuity)

Seven gate-clean increments took the ceiling 115 → 82 (each a separate commit on
branch `claude/checker-bytecode-compiler-info-pqd2w0`, each verified against the
full gate — differential 0 mismatches, `-race`, alloc, full `make test`):

1. **N-event already-in-layout lowering** (`lower.go::layoutOperands`) — accept
   any N event operands already in sig order on top (was 0/1/2 only).
2. **Residual ordering via local promotion** (`emit.go::Finalize` +
   `planValueDefLocals`) — an out-of-order data residual force-promotes its
   events to frame locals.
3. **`OpMakeMap`** (`bytecode.go`/`vm.go`/`emit.go`/`lower.go`/`engine.go`) —
   computed `make` construction bodies (`make Outer {i:(make Inner …)}`) assemble
   the map at runtime from value operands; keys ride in `Program.MakeMaps`. The
   shared `autoEvalMap` const-fold is split schema-vs-data by `match.Name ==
   "make"` so class defaults still bake while make data values become events.
4. **Make-instance lists via `OpMakeList`** (`emit.go::RecordMakeList`) — dropped
   the `make`-element exclusion (an OpMakeList of make events rebuilds fresh
   instances per run, sound).
5. **0-output closure bodies** (`callable_words.go` + `emit.go::
   RecordClosureCall` + `lang/go/modules/test.go`) — generalised the closure
   path from exactly-1 to 0-or-1 outputs (`callableWord.bodyOut`); registered
   `test-test` so `Test.test "n" [body]` compiles, run via `InvokeBody`.
6. **boru:test describe bodies** — reused the 0-output infra for `test-describe`.
7. **`fnStoreWords`** (`emit.go`) — minilang/parselang `register` *store* a fn
   (never invoke on the VM tape), so the fn bakes as an inert const, exempt from
   the function-valued-operand refusal.

**One attempt was correctly reverted (a real dead end):** extending
`OpMakeList`/`OpMakeMap` into fn-body frames. It is **unsound** — a body word
that shadows a module def resolves to the param local in check mode but to the
module def in the interpreter (`def c1 1 … fn [[c1:Integer]…[[c1]]]` returns
`[9]` compiled vs `[1]` interpreted). The top-frame restriction on
`RecordMakeList`/`RecordMakeMap` is therefore **load-bearing** — do not retry
this without solving the scope-resolution divergence first.

---

## 3. Strengths (validated by the state of the art — do not change)

- **Compiler = abstract interpreter + emission.** This is the WASM
  baseline-compiler design (Titzer). The validator/checker infers operand-stack
  types in one pass and emits as a side-effect. Keep it.
- **Probe-then-commit** for speculative compiles (`tryReturnedClosure`,
  `recordClosureDispatch` run a throwaway `EmitState` first). Sound and reusable.
- **The refusal ratchet** as a regression gate. Excellent — keep it, but see
  §6 for refining it.
- **Soundness-first refusals.** Several refusals stop a real divergence
  (mutable-instance aliasing, scope-resolution divergence). The discipline of
  "refuse rather than diverge" is right *as between those two failures* — a
  refusal is the lesser one — but neither is an outcome the design accepts:
  each of these rows still fails to compile, so each is an open defect owed a
  faithful lowering. The differential gate (0 mismatches) is the backstop that
  keeps the worse failure out; it is not a licence for the lesser one.

---

## 4. Findings: generalizations & simplifications

Each item: the problem, the evidence, the proposed change, the risk.

### 4.1 The lowerer reinvents stack scheduling

**Problem.** `layoutOperands` (`lower.go`) hand-codes the 0/1/2/N-operand cases,
inserts at most one `SWAP`, and *refuses* anything needing a 3-deep rotate
("operand shape beyond Stage 1", "stack discipline underflow", "call result
above a literal"). This is the classic **stack-scheduling** problem (scheduling
a DAG onto a stack machine, NP-complete in general).

**Evidence.** The `case 0/1/2/default` ladder in `layoutOperands`; the
`reorder`/`shapeBeyond`/`notAdjacent` refusal strings; the residual-ordering
patch (increment 2); the value-def-locals promotion (`OpStoreLocal`, an ad-hoc
"spill to a frame slot").

**Proposed change.** Two options (4.1a is the bigger bet of §5):

- **(4.1a) Destination-driven code generation (DDCG).** Thread a *data
  destination* (stack / frame-local / effect-none / specific slot) and a
  *control destination* **downward** during recording, instead of materialising
  every result onto the simulated stack and shuffling afterward. Your
  `OpStoreLocal` value-def-locals and the new 0-output ("effect/none")
  destination are already *special cases* of destinations; DDCG unifies them.
- **(4.1b) If keeping post-hoc reconciliation:** adopt Koopman's distance-based
  stack scheduling and add **DUP / TUCK / ROT** opcodes (the VM has only `SWAP`
  — that is *why* the 3-deep cases refuse). Limit direct access to the top 3
  stack slots (matches the VM's reach) and rank use/reuse pairs by distance.

**Risk.** 4.1a is a recording-model change (significant); 4.1b is additive
(new opcodes + a heuristic) and lower-risk.

### 4.2 `producedBy` is value numbering — make it first-class

**Problem.** `producedBy map[string]producer` (value-ID → producing event) is
**local value numbering** done by hand, and the carrier-identity hacks (fresh
IDs for impure `make`, the de-collision in `RecordCall`, dup-body distinct-IDs)
are manual value-number management.

**Proposed change.** Frame the trace as an **SSA / GVN value graph** rather than
a linear event list. Multi-result calls, dup bodies, and CSE then fall out
naturally, and lowering becomes a *scheduling* pass over a DAG (where DDCG /
stack-scheduling live) cleanly separated from "what the checker decided."

**Risk.** Large; this is part of the big bet (§5). But even the *framing* (a
`valueNumber` type instead of string IDs) is a useful intermediate step.

### 4.3 Kill the strip → remember → materialise dance

**Problem.** `RecordStrip` / `RememberOriginal` / `origByID` / `materialise`
exist **only** because the checker strips concrete values to carriers (losing
info) and the compiler reconstructs it via ID-preservation + a side table.

**Proposed change.** A carrier should **carry a back-pointer to its concrete
origin** (e.g. a `*Value` on the carrier payload) so the compiler reads it
directly. No ID-keyed recovery, no preservation fragility. This is the single
biggest *simplification* available and removes a class of "operand of unknown
provenance" refusals caused by lost originals.

**Risk.** Medium. Carrier construction is load-bearing for the checker (CSE,
memoization, typing); change `toCarrier` carefully and lean on the differential
gate.

### 4.4 Consolidate `EmitState`'s parallel side-maps

**Problem.** Four per-event-seq boolean maps — `zeroOutSeq`, `typeOut`,
`valueDefs`, `genericSeq` — are all "property of event N" and belong as **flags
on `emitEvent` / `emitCall`**. The string-ID-keyed hot maps (`producedBy`,
`origByID`, `constIdx`) want **integer value handles**, not minted string IDs
(you hash strings on every operand resolution).

**Proposed change.** Move the per-seq booleans onto the event struct; introduce
an integer value-handle table. Mechanical, low-risk, and it speeds up the hot
path.

**Risk.** Low.

### 4.5 Move word compile-classification onto the `Signature`

**Problem.** The checker/compiler consult **seven+ scattered name-keyed tables** —
`callableWords`, `fnStoreWords`, `fnIntrospectionWords`, `moduleConstFoldWords`,
`checkModeLiteralWords`, `fallbackWords`, `islandPureWords` — plus predicates
`dynOutNativeOK`, `quoteOperandInertOK`, `isModuleInnerSig`. They all answer one
question: *how does this word behave for compilation?* Adding a word's compile
behaviour means editing eng-side tables and **coupling eng to module word
names** (the June session added `test-test` and `fnStoreWords` entries this way).

**Proposed change.** Replace them with a declared **`Signature.CompileEffect`**
(an enum: `Pure / SideEffect / Invokes / Stores / Reads / CodeBody / …` plus a
couple of capability bits). The word *declares* its compile-relevant semantics
once; the eng compiler stops hard-coding module names. The `test-test` /
`fnStoreWords` edits would have been one-line module-side declarations. This is
the best **information-sharing** fix and de-couples eng from modules.

**Risk.** Medium (touches sig construction across natives + modules), but
purely additive if defaults preserve current behaviour.

### 4.6 Unify the `Record*` / `tryRecord*` surface

**Problem.** ~13 recorders (`RecordCall`, `RecordClosureCall`, `RecordMakeList`,
`RecordMakeMap`, `RecordUserCall`, `RecordBranch`, `RecordLoop`,
`RecordFallback`, …) share a repeated spine: resolve operands, run the
double-record guard (`producedBy[outs[0].ID]`), append an event, register
outputs, latch refusal.

**Proposed change.** Factor that spine into one core; each recorder supplies only
its event-shape. Once `CompileEffect` (4.5) exists, most of the `carrierResults`
try-chain becomes a single dispatch on the sig's declared effect.

**Risk.** Medium; do it *after* 4.5.

### 4.7 Remove dead / half-wired mechanisms

`SchemaArg` and the `materialiseSchema` counter (`CheckState`) are defined,
propagated (`registry.go:1054`), and **never read** (confirmed during the
`OpMakeMap` work — the schema-vs-data split is keyed on `match.Name` instead).
Either wire them or delete them. Low-risk cleanup.

---

## 5. State of the art we are missing

| Technique | Maps to | Payoff |
|---|---|---|
| **Destination-driven code generation** (Dybvig/Hieb/Butler) | `layoutOperands` shuffles, value-def-locals, residual-ordering refusals | Dissolves the largest refusal class in one model; generalises local/effect destinations |
| **Stack scheduling** (Koopman) | The SWAP-only lowerer, "3-deep rotate" refusals | Add DUP/TUCK/ROT + a distance heuristic; near-total elimination of shuffle refusals |
| **SSA / GVN** (Cocke) | `producedBy` + carrier-identity hacks | Principled multi-result / dup / CSE handling; DAG lowering |
| **Single-pass abstract-interp baseline compilers** (Titzer / WASM) | The whole `CheckState.Emit` model | *Validates the architecture*; their "validator emits control-transfer info for the interpreter in one pass" is a template for emitting **more** from one pass |
| **"Abstract compilation → abstract bytecodes"** | the const-fold path | Framing for *which* deterministic sub-evaluations to bake |
| **Emit a Trap/Raise opcode** | "suppressed runtime error" (5) + "multi-return count-mismatch" (3) | These are not uncompilable — they are *known to error*. Compile the error path instead of refusing. A small, principled addition that clears 8 "correct refusals" for P7 |

**Why DDCG is the headline.** A recursive code generator traditionally
materialises every intermediate value through one result location and then
shuffles. DDCG inverts this: the *caller* of a codegen step knows where it wants
the result, so it passes a destination down. Data destinations include
"accumulator/stack", "a specific frame slot", and "effect/none" (discard — the
0-output side-effect case the June session added by hand). Control destinations
thread where execution resumes, giving one-pass conditionals/short-circuits.
Caveat (Bernstein): stack bytecode loses the tree-lifetime structure DDCG
exploits, so this is a **recording-side** change (push destinations down as the
trace is built), not a lowering-side patch.

---

## 6. Recommended sequencing

> **Status: this sequence was executed.** See the
> [Implementation report](#implementation-report-202606). Annotations below.

**Low-risk, do first (pure simplification, gate-cleanly):**
1. §4.4 — consolidate `EmitState` side-maps onto `emitEvent`; integer value
   handles. — **LANDED** (consolidated to one `eventInfo` map; struct-move unsafe,
   integer handles deferred).
2. §4.7 — delete (or wire) `SchemaArg` / `materialiseSchema`. — **LANDED** (deleted).
3. §4.3 — carrier carries its concrete origin; delete the
   strip/remember/materialise side tables. — **PARTIAL** (recorders merged; full
   back-pointer declined as net-negative).

**Medium, high-payoff (the information-sharing win):**
4. §4.5 — `Signature.CompileEffect`. — **LANDED** (bitfield; 5 capability tables
   migrated, plus the 7th — `callableWords` — lifted to `Signature.Callable` in
   the follow-on session, completing the eng↔module decoupling). Then §4.6 —
   unify the recorder spine and collapse the `carrierResults` try-chain. —
   **NOT DONE** (fallthrough load-bearing; not cleanly feasible).
5. **Emit a Trap/Raise opcode** (§5 last row). — **LANDED** (`OpTrap` + exact
   `OpRet`; cleared all 8 correct-refusal rows).

**The big bet (prototype on a branch, behind the differential gate):**
6. §4.2 + §4.1a — reframe the trace as a value graph and lower it with
   destination-driven codegen + a few stack ops (DUP/ROT/TUCK). — **LANDED
   (headline)**: `OpReverse` + spill-to-local destinations deleted the
   `layoutOperands` special-casing. The value-graph reframe and DUP/ROT/TUCK were
   *not needed* (`producedBy` already the DAG; spill subsumes the stack ops).
   Payoff was structural, not numeric — the remaining shuffle/provenance refusals
   are feature-entangled (fn-value boundary, branch-result-modeling), not operand
   layout. See the completion guide.

**A meta-improvement to the ratchet itself.** — **LANDED** (`rootCause` axis).
The `refusalCeiling` conflates
"genuinely can't compile (soundness)" with "lowering is too weak (scheduling)"
with "missing opcode". Add a second axis to `normaliseReason` — bucket by *root
cause* (soundness / scheduling / opcode / correct-error) — so a future session
knows which investment actually moves the number. The "correct-error" bucket
(8 rows) is a good first proof: it should become *compiled error programs*, not
refusals.

---

## 7. Verification discipline (unchanged contract)

Every increment must stay gate-clean. From the repo root and `test/go/langspec`:

```bash
make fmt && make vet && make lint && make test          # full pre-commit
cd test/go/langspec
go test -run TestCompiledCoverage -v                    # ratchet (refusal + island ceilings)
go test -run TestSpecCompiledDifferential -v            # 0 mismatches REQUIRED
go test -run TestSpecCompiledOrFallback                 # error-taxonomy parity
go test -race -run 'TestCompiledConcurrent'             # race
cd ../../../lang/go && go test ./ -run TestCompiledAllocCeilings   # alloc ceilings
```

Lower `refusalCeiling` (and `islandCeiling`) monotonically; never raise.
Gate-clean-or-revert per increment; if an item cannot stay gate-clean, revert it
and document why (the fn-body-assembly dead end in §2 is the template).

A quick ad-hoc refusal-bucket dump (delete before commit) — a throwaway test in
`test/go/langspec` that iterates the `.tsv` rows, calls `a.CompileCheck`, and
logs `normaliseReason(reason)` filtered by a `BUCKET` env var — is the fastest
way to see what a change cleared. The June session used exactly this.

---

## 8. Key file / symbol index (for navigation)

| Concern | Location |
|---|---|
| The recording side-effect channel | `eng/go/registry.go` — `CheckState.Emit` |
| Dispatch funnel + try-chain | `eng/go/carrier.go` — `carrierResults` (~line 467) |
| Recorder + lowerer data structures | `eng/go/emit.go` — `EmitState`, `emitEvent`, `emitOperand`, `Record*`, `Finalize` |
| Stack-scheduling / operand layout | `eng/go/lower.go` — `layoutOperands`, `lowerCall`, `planValueDefLocals` |
| Closure-body compilation | `eng/go/callable_words.go` — `callableWords`, `compileClosureBody`, `tryRecordClosure` |
| Opcodes + Program | `eng/go/bytecode.go`; VM in `eng/go/vm.go` |
| Word compile-classification tables | `eng/go/emit.go` (`fnIntrospectionWords`, `fnStoreWords`), `eng/go/carrier.go` (`moduleConstFoldWords`, `checkModeLiteralWords`, `fallbackWords`, `islandPureWords`, `dynOutNativeOK`, …) |
| const-fold / schema-vs-data | `eng/go/engine.go` — `autoEvalList`, `autoEvalMap`, `constFoldContainerVal` |
| Host entry point | `lang/go/boru.go` — `(*Boru).CompileCheck` |
| The ratchet | `test/go/langspec/compiled_coverage_test.go` |
| Differential gate | `test/go/langspec/compiled_differential_test.go` |
| The (stale-on-numbers) P5–P7 plan | `design/boru-bytecode-runtime-independence.0.md` |

---

## 9. Sources (state of the art)

- Dybvig, Hieb, Butler — *Destination-Driven Code Generation*:
  https://bernsteinbear.com/assets/img/ddcg.pdf
- Bernstein — *A quick look at destination-driven code generation*:
  https://bernsteinbear.com/blog/ddcg/
- Koopman — *A Preliminary Exploration of Optimized Stack Code Generation*:
  https://users.ece.cmu.edu/~koopman/stack_compiler/stack_co.html
- Titzer — *Whose Baseline Compiler Is It Anyway?* (arXiv 2305.13241):
  https://arxiv.org/abs/2305.13241
- Titzer et al. — *A Fast In-Place Interpreter for WebAssembly* (single-pass
  abstract-interp validator): https://www.cs.tufts.edu/comp/150FP/archive/ben-titzer/wasm-interp.pdf
- *Value numbering*: https://en.wikipedia.org/wiki/Value_numbering
- HN discussion — *type checking is a special case of abstract interpretation*:
  https://news.ycombinator.com/item?id=34088644

---

## 10. How to start a new session from this doc

1. Re-run the ratchet (`§7`) to confirm the current `refusalCeiling` and the
   live histogram — the numbers in `§2` drift as work lands.
2. Pick from `§6` by appetite: a low-risk cleanup (4.4 / 4.7 / 4.3), the
   information-sharing refactor (4.5 → 4.6), the 8-row "compile the error path"
   win (Trap/Raise), or the big DDCG bet.
3. Keep each change a separate gate-clean commit; revert-and-document on any
   divergence. Update the ratchet ceiling (downward only) with a one-line
   rationale in `compiled_coverage_test.go`, as the existing entries do.
4. Do **not** retry fn-body container assembly without first solving the
   scope-resolution divergence (`§2`, the reverted dead end).

---

## 11. Refusal-ceiling decrement history

The full per-decrement rationale used to live in one ~6 KB string literal on
`refusalCeiling` in `compiled_coverage_test.go`. That comment now keeps only the
most recent few decrements inline (plus a pointer here); the older trail is
recorded below, newest-first, so the test stays readable. Each entry is a
`from -> to` transition with the change that earned it. The deeper narrative for
the recent ones is in the [Implementation report](#implementation-report-2026-06)
and §2; this is the index.

**Continuation session (branch `claude/bytecode-compiler-review-ai3l9u`):**
(The 46 → 16 trail from intervening sessions lives inline in the
`refusalCeiling` comment in `compiled_coverage_test.go`; the entries below are
this session's.)

- **12 → 11** — patrun fn-value store: the `add` overload `(Map, Any, Patrun)`
  stashes its value arg in the patrun side table and never invokes it on the VM
  tape, so it now declares `CompileStoresFn` — a PURE fn literal rides as an inert
  const (`add {cmd:"sum"} (=>…) pm`); a capturing closure still declines at
  `isInertConst` and falls back. Cleared the `function value reaches add` row.
- **13 → 12** — mixed fn-value-call boundary (`OpCallDynamicMixed`): a dynamic fn
  INTERIOR to the residual `[3, m.f, 2]` (forward `2` → sig[0], stack `3` →
  sig[1]) fits neither the leading nor trailing-1 layout. The new op islands the
  whole `[before…, fn, after…]` window verbatim — the same token sequence the
  interpreter ran, so the fn auto-applies (or stays put if non-callable) for any
  arity/split; the fn's producing event is promoted to a frame local so the
  window re-pushes in source order. Completes the fn-value-call boundary.
- **14 → 13** — sub-registry poly dispatch: a module word with a dynamic input
  (`StructUtil.getpath … (StructUtil.setpath …)`) refused because
  `tryRecordPoly`'s CORE-dispatch guard required a MAIN-registry builtin.
  `tryRecordPoly` now threads an `ownerReg` (the sub-registry, carried on
  `MatchResult.Reg` from the trivial-delegation dispatch) and records a `PolyRef`
  carrying it; `callPoly` re-matches over that registry (the same object the check
  pass shares with `RunProgram`).
- **15 → 14** — conditional dynamic apply in a branch
  (`if (n eq 0) [99] MathUtil.sqrt 16`): a trivial-delegation module fn value now
  bakes as an inert const, and a branch result that MAY be a fn carries a
  `mayBeFn` event flag so a leading-branch-result over static args lowers to a
  runtime-conditional `OpCallDynamic` (applies on the fn arm, leaves `[value,
  arg]` on the data arm).
- **16 → 15** — dispatch-recovery operand order (`(3 and "x") add 1`): a
  strict-disjunct straddle that `matchSignature` fails and `checkModeAssumeSig`
  recovers now records `OpCallNativePoly` (operands rebuilt into signature order)
  instead of latching the program uncompilable.

**Follow-on session (branch `claude/lucid-davinci-ajtxt5`):**

- **47 → 46** — dynamic input to a closure-dispatch word: `RecordClosureCall`
  pre-declined ANY dynamic non-body operand, but the resolveOperand loop below
  already declines one that genuinely can't resolve. A caught `do` error reaching
  `error [handler]` is a resolvable event whose closure input is a FIXED `TError`
  carrier — independent of the dynamic value, so the handler runs faithfully over
  whatever it is. Dropping the pre-decline compiles `do [{x:1} !. y] error [get
  code]` island-free, differential-clean. The 4 remaining dynamic-input rows keep
  their own blockers (flex mutation, IO-context `set` receiver, struct-module
  `getpath`).
- **51 → 47** — value-def-locals in fn units: `planValueDefLocals` (the
  computed-result → frame-slot promotion) now runs for EVERY compiled unit, not
  just the top-level program — called in `StartFnCompile`'s `finish` while the
  unit is live, with the unit's OUTPUT operands as the residual-equivalent extra
  references, and `CanSeatAcrossFragment` dropped its `len(units) != 1` guard. So
  a computed scrutinee read across a desugared `case` if-chain (`get code case
  […]`) seats as a frame local inside a fn unit. Cleared the 4
  `error [get code case [value-clauses]]` rows — completing the error cluster (all
  6 `error [handler]` rows now compile; the residual dynamic-output rows are
  `await` + path-modifier — refusals that are correct in preferring no answer
  to a wrong one, and still open defects). Sound because a non-repushable
  computed event reaching `CanSeatAcrossFragment` is always produced in the
  current unit (an enclosing value is a capture → local → repushable).
- **53 → 51** — catch frame (`do` / `error` closure bodies that raise): `raise` is
  a CALL_NATIVE whose handler returns an error the VM already propagates and
  `do`/`error` already catch via `InvokeBody` — the only block was the recorder
  refusing a closure body that nets fewer values than declared. Marking `raise`
  **CompileDiverges** (it always raises → control never returns past it) makes
  such a body DIVERGENT: it compiles with NO RET (vacuously satisfying any
  BodyOut), and the runtime error propagates to the catcher (sound in a
  branch/loop arm too, like break/continue). `error` was then collapsed to ONE
  `(List, Any)` sig with a runtime `IsError` branch — a static TError-vs-TAny pick
  was unsound (the checker types `do [raise …]` as Any and baked the pass-through
  even when the runtime value is an Error) — plus a `CallableSpec` so the handler
  body compiles as a closure. Cleared `error [get message]` + the predicate-clause
  `error [case …]`. The other 4 `error [get code case [value-clauses]]` rows wait
  on value-def-locals in fn units (a computed scrutinee `CanSeatAcrossFragment`
  can't seat when `len(units) != 1`).
- **57 → 53** — dot-access reach in an inert code body: the property-test words
  `Test.prop` / `Test.check-prop` / `Test.skip` take TWO code bodies
  (`[r.int 0 100]` + `[0 gte]`) that are inert at the call (prop stores them in a
  PropertySpec map, skip discards them, check-prop CallBorus them in its native
  handler), so `noEvalBodiesInert` already bakes them as const operands — but a
  dot-access reach (`r.int`, an Eval=true receiver Reach) inside a body was not
  an inert const MEMBER, so the body list failed `isInertConst`. Unlike
  `isInertReach` (the STANDALONE detached lens — receiverless, Eval=false, never
  expanded at the pointer), a reach as a MEMBER of a never-evaluated compound is
  pure DATA (the VM pushes the baked compound verbatim and never expands a
  reach), so `inertReachMember` admits a receiver reach whose receiver/literal-key
  tokens are themselves inert (a computed paren segment still refuses). This was
  NOT the "2-body closure" problem it looked like — all three bake their bodies
  as consts. Cleared the 3 code-body rows + `Rand.map-from` (its schema map holds
  the same reach), all island-free.
- **59 → 57** — module-exported Table-type get fold: a Table type literal
  (`Test.TestSet`) carries a `TableTypeInfo` payload, so `tryFoldModuleConst`'s
  ride switch (`isInertConst` | bare-type-node) rejected it and the get over the
  immutable module export refused "unknown provenance … at get" — unlike a BARE
  type node (`Test.TestCase`, `Data==nil`) which already folds. `TableTypeInfo`
  wraps its row `RecordTypeInfo`, so it joins the structural-type-body const
  family in `isInertConst` + `typeBodyConstOK` (same `fieldsOK` interior check;
  a carrier interior still refuses). The get bakes the immutable type and the
  downstream make/is/istype compile natively (both `Test.TestSet` rows). This was
  the one clean bounded win in the operand-provenance bucket; the §4.3
  back-pointer was measured against the rest and is NOT justified (see the
  completion-guide priority 3 — ~10 of the remaining rows are correct dynamic/
  error refusals, the rest want bespoke per-row machinery).
- **60 → 59** — `returnsof` static return type: a `ReturnsFn` computes the fn's
  precise declared return at check time, so `TypeUtil.returnsof` reports a
  concrete type node instead of the opaque `TAny` and bakes a `CALL_NATIVE` like
  `arityof`/`typeof`. The one cleanly-tractable row of the dynamic cluster.
- **63 → 60** — value-arm `if`: `if cond v1 v2` with VALUE arms (not `[body]`
  code) refused "then-branch not captured" (the then was run as a body → nil
  fragment, while only the else handled a value). The then arm is now symmetric
  with the else (a value-then pushes its value). Cleared the direct value-arm
  `if` and the usurp-if shape (3 rows). The 3 remaining if-branch rows are 1
  variadic-merge + 2 cross-fn break/continue (soundness).
- **64 → 63** — `rand-list-of` generator body: `Rand.list-of [body] n` declares a
  `CallableSpec` (the `do` closure shape, 0-input/1-output), so the body compiles
  to a closure the handler runs n times via `InvokeBody` instead of refusing the
  NoEvalArgs code body. Differential-clean (the RNG draws advance the same module
  generator, so compiled == interpreted).
- **71 → 64** — quoted-operand inert words: `quote` / `codequote` / `raise` /
  `timeout` / `interval` declare `CompileQuoteInert`, so the recorder bakes their
  inert quoted operand + `CALL_NATIVE` (the get/getr/set exemption, made
  declarable). Cleared 7 of 10; the 3 `timeout`/`interval` rows whose code body is
  a carrier stay refused (a §4.3 / `materialise` gap).
- **73 → 71** — trailing fn-value auto-apply: `5 m.f` / `[..] r.one-of` (the fn
  trails its arg) compile via the new `OpCallDynamicTrailing` (rotate the fn to
  the residual front; identical to `OpCallDynamic` when callable, fn-on-top when
  not). Bounded to one arg.

**Review session (branch `claude/checker-bytecode-compiler-info-pqd2w0`):**

- **74 → 73** — `OpReverse` N-event reverse operand layout (`is-between (a)(b)(c)`).
- **79 → 74** — suppressed-runtime-error rows compile a terminal `OpTrap`
  (orphan `gen`, `unpack` of a missing key); nested traps still fall back.
- **82 → 79** — user-fn return-count mismatch compiles via exact-count `OpRet`
  (frame stack-base, scoped to user-fn frames; closures keep `each_error`).
- **94 → 82** — fn-storing words (minilang/parselang `register`) bake the fn as
  an inert const (stored, never invoked on the VM tape).
- **95 → 94** — `boru:test` describe-body closures (`Test.describe "g" [body]`).
- **107 → 95** — `boru:test` case-body closures (`Test.test "n" [body]`, 0-output
  side-effect bodies; closure path generalized to 0-or-1 outputs).
- **109 → 107** — `OpMakeList` of make-instance lists (fresh instances per run).
- **112 → 109** — `OpMakeMap` for computed `make` construction bodies.
- **113 → 112** — residual ordering via local promotion (event-above-literal,
  for non-fn/dynamic residuals).
- **115 → 113** — N-event already-in-layout lowering (3+ adjacent event operands
  in sig order).

**Earlier (pre-115, branches `bytecode-compiler-review-*` and the P0–P5 plan),
condensed:** fn-body return-count phantom (119→115); fn-value field calls
`{b:f/r}` via poly-get + `OpCallDynamic` (129→119); branch-fragment value-def
locals (135→129); value-def-locals + class mutable-default bake (140→135);
factory-apply, a returned closure applied via leading `OpCallDynamic` (140→139);
and the long P0→P5 ramp behind that — `StructUtil` result types (222→216),
filter result typing (224→222), then P0 651 → P3 642 (wide poly) → P4 616
(`OpCallDynamic`) → P5 598 (multi/0-result calls) → make carrier-identity 580 →
value-def locals (`OpStoreLocal`) 568 → dup carrier-identity 565 → predicate
provenance 555 → if value-else arm 545 → case desugar 542 → multi/0-return fns
538 → fn-value apply 527 → method fields 521 → if-guard 519 → dynamic-output
core builtins 514 → module inner natives 459 → 3-arg push+swap 453 →
strict-disjunct poly 445 → atom-keyed set 425 → computed-else if 421 →
variadic-else if 417 → fn-value introspection 407 → filter lambda 402 → map
lambda 399 → `with-decimal` 394 → `args.N` 390 → `word`/splice 363 →
`macroexpand` 359 → make container defaults 349 → `usurp` 312 → case no-default
310 → query DSL 283 → reach lenses 268 → scalar carrier-keep 258 →
module-synthetic const-fold 227 → `OpMakeList` 213 → (+11 minilang corpus 224,
+1 patrun 217) → type-pattern operands 196 → generic schema 185 → typed-list
generics 181 → `fnsig` 178 → surface types 159.
