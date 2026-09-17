# Checker + Bytecode Compiler — Completeness Review and Completion Plan

Status: **executed** — see [`P7-ENDGAME.10.md`](P7-ENDGAME.10.md) for
the closing record and the execution log below for every landing.

**Doctrine correction (supersedes the original framing everywhere in
this doc).** The interpreter is **not** a fallback for the compiler, and
is not allowed to be one. Failure to compile is a *failure*. Done is a
language that compiles as a developer expects — all valid code compiles,
no exceptions — so every refusal counted below is a **defect**: an
unimplemented or unproven case, owed a fix and tracked to closure, never
a sanctioned design outcome. The runtime path that re-runs a refused
program on the interpreter is **scaffolding** that absorbs a known defect
so the user still gets an answer; where this doc describes it, it is
described as machinery, never as a safety net the design leans on and
never as a reason a refusal is acceptable.

Originally: Snapshot date **2026-07-03**, tree = `main` @ `060157b`
(post-PR #224 cross-module element typing). All gate numbers below were
measured by running the ratchet/differential suites on this tree, not
copied from older docs — refusal counts in earlier docs (0, 6, 19, 23,
24, 112…) are snapshots against a growing corpus and are superseded by
the live census here.

Companions: `CARRIER-STATIC-TYPECHECK-REPORT.10.md` (checker
architecture), `checker-precision-fronts.0.md` (the two designed
precision unlocks), `boru-bytecode-finish-line.0.md` +
`boru-bytecode-next-stages.0.md` (compiler stage sequencing),
`boru-bytecode-completion.0.md` (re-scoped P7),
`local-reasoning-for-global-properties-in-boru-report.0.md` (proposals 3,
4, 10 referenced below), `module-fn-checkstate-ownership.7.md` (the
module-body wall).

---

## Part 1 — Where the two subsystems actually stand

They are one system: the compiler is the carrier checker run with a
recording side effect (`CheckState.Emit`), so checker precision is
compiler coverage and checker soundness is compiler correctness. The
review is split only for presentation.

### 1.1 Type checker — live envelope (all four ratchet suites PASS)

| Metric | Pin / ceiling | Live measured | Note |
|---|---|---|---|
| False positives (spec corpus) | `pinnedFalsePositives = 0` | **0 / 3313** value rows | holding |
| Check-silent ERROR rows | `pinnedUnflaggedErrorRows = 172` | **172 / 485** | by policy never reaches 0 (value/state-dependent errors are the runtime's job) |
| Type-soundness violations | `pinnedTypeSoundnessViolations = 8` | **7 / 3306** | improved below pin — the test itself says lower the pin |
| Any-frontier count | 345 (informational) | **381 / 3306** | corpus growth |
| Any-frontier **ratio** | `anyFrontierRatioCeilingPct = 12` | **11.52 %** | **only ~0.48 pt headroom** |
| Check-rejected-but-runs (fuzz) | `pinnedCheckRunDivergent = 104` | **104** | all hand-verified dead-branch `case`/`if` |

What is genuinely landed and solid: single-intercept parity dispatch
(checker runs the real engine loop over carriers); the gradual
`dynamic(T)` modality with narrowing-through-use; strict-disjunct
per-alternative dispatch; guard narrowing for lattice/class types
(incl. paren forms and else-branch complements); branch join + bounded
Kleene loop/recursion fixpoints with assume-guarantee on declared
returns; the loud-diagnostics program (uncalled_function,
unreachable_signature, module-export propagation, `run --check`
preflight, LSP wiring); `--strict` with `dynamic_dispatch` infos;
`forward_strands_operand` at ~100 % measured precision; the severity
table with a completeness gate; and (PR #224) cross-module element
typing.

**Known gaps, in rough severity order** (each verified by live probe on
this tree):

- **G1 — predicate-refinement returns unverified statically.**
  `def Big (Integer gt 10)  def mkbad fn [[] [Big] [5]]  mkbad` checks
  clean (exit 0) and fails only at the runtime RET boundary.
  `checkBodyReturnConformance` flags only provable disjointness. The
  smart-constructor pattern — the reason subset types exist — is
  unchecked exactly where it matters. Also the source of soundness pin
  `user-types.tsv:124`.
- **G2 — guard-blind narrowing for refinement types; validate-then-call
  is a gating false positive.** `extractGuardClauses` skips any guard
  type carrying a predicate body, so `if (x is Big) [g x] [0]` with
  `x:Integer` is a gating `no_signature` (exit 1) while the program
  runs correctly. The FP = 0 pin is corpus-relative; this is a live
  off-corpus FP class, and the named blocker for check-by-default.
- **G3 — no redundant-guard / entailed-dead-branch diagnostics.**
  A provably dead `if (n is Big) …` with `n:Big` produces zero
  diagnostics; `unreachable_branch` fires only on literal `true/false`
  conditions. The checker computes the facts and says nothing.
- **G4 — typed code values (the `do` escape hatch).** `ops get 0 do`
  is `dynamic(Any)`; rated "Severe"; the dominant Any-frontier feeder
  (minilang 47, struct 32, parselang 31 rows…). Design complete
  (`checker-precision-fronts.0.md` §1, `CodeEffectInfo` /
  `Code[in→out]`), **no code**.
- **G5 — store-identity context typing.** `CheckState.ContextTypes` is
  one flat flow-insensitive map; store layering is flattened; same-name
  keys of different stores join. Design complete (§2,
  `StoreShapeInfo`), **no code**.
- **G6 — options maps through `Any` slots.** Words without a declared
  Options schema silently ignore atom-key typos at check *and* runtime
  (`emit json {prety:true}` — clean under `--strict`, wrong output).
- **G7 — `missing_returns` noise / incomplete native `Returns`
  coverage.** Unannotated native sigs warn per call site and feed the
  Any frontier; annotation errors have historically been a soundness
  source (six time-util fixes). No gate asserts full coverage.
- **G8 — residual soundness pins (7).** `forward-barrier.tsv:83`
  (needs concrete-condition folding), `recursion.tsv:53` (for-spread
  modeling), `class.tsv:85`, `corpus-core.tsv:61`, `module-rand.tsv:37`
  and `patrun.tsv:40` (dynamic-dispatch class, possibly irreducible),
  `user-types.tsv:124` (falls to G1).
- **G9 — emit/check entanglement.** `CheckState.Emit` remains a field;
  `emit.go` is 4.3 k lines riding the checker pass; the
  recorder-interface decoupling (comprehensive review Tier-2 item 6) is
  only partially done.
- **G10 — dead-branch attribution.** The 104 fuzz divergences are
  static-checking-by-design (an error in a never-taken arm gates the
  program) but are not labelled as such for users.

### 1.2 Bytecode compiler + VM — live gates (all PASS)

| Gate | Live result |
|---|---|
| Census (`TestCompiledCoverage`) | **3,875** spec value rows → **3,476 compile** (1 island), 248 check-error, **151 refused** → 95.8 % of the 3,627 valid rows produce a Program |
| Accepted-row differential | **3,296 rows, 0 mismatches** (floor 2,224) |
| Whole-corpus compile-or-fallback | 3,875 rows, 3,451 via compiled path, **0 divergences** (values *and* error taxonomy) |
| Re-scoped-P7 partition (`TestOnlyMetaFallsBack`, **enforced**) | tier-1 interpreter-only **0** (cap 3), tier-2 reducible **1** (`flex`; ceiling 1), error-row 83, compute gap **68** (ceiling 86) |
| Property fuzz | 2×1,500 programs, 1,955 compiled path, **0 divergences** |
| Corpus three-mode + combinations parity | green |
| `correct-error == 0` (the one hard coverage gate) | **0** — every known-error row compiles a trap, never silently refuses |
| `COMPILED_STATUS.md` | **stale** (says 3,524 rows / 112 refused; live is 3,875 / 151) |
| External voxgig force-compile corpus | **35 / 48 files** (per PR #224 message; sweep **not re-run** post-merge — "re-gate" outstanding) |

Refusal histogram (151 rows): dispatch recovery (best-guess) **81**,
operand provenance 22, function-value-reaches-word 12, typed-def
dynamic refinement reparent 12, fn-value-call boundary 9,
function-valued operand 5, user-fn call (Stage 3) 4, unconsumed
fn-value carrier 3, code-body word 1, dynamic input 1, paren-bounded
fn-value application 1. Root cause: **125 soundness, 26 coverage,
0 correct-error**. Refusal/island ceilings were deliberately demoted to
informational (`compiled_coverage_test.go:174-180`) because the corpus
grows; the only hard coverage assertion is correct-error = 0.

Refusal containment, as machinery: refusal and compiled-mode internal
errors roll back the registry snapshot and **silently** re-run the
interpreter. The silence is the worst of it — a compile defect fires,
the user's valid program answers anyway, and nothing on the default
path says the compiler failed. That is scaffolding holding an open
defect off the user's output, not a fallback the design leans on, and
it makes no refusal acceptable. `--force-compile` is the only mode that
surfaces the failure at all: there refusal is the loud error it should
always have been. Compiled mode is **opt-in** (`--compile` /
`BORU_COMPILE`), off by default.

**Correctness debt (the part that is *wrong*, not just missing):**

- **M1 — const-pool identity aliasing (Mechanism A, open, deferred by
  design).** Fn-returned container literals can be const-baked such
  that `(mk) eq (mk)` is `true` compiled vs `false` interpreted.
  Narrowed to identity only (no mutation divergence), but it is a live
  compile≠interpret divergence — the one class that violates the hard
  invariant.
- **M2 — Mechanism E remainder.** `/r`-deferred map-field auto-invoke
  (`{f:make42/r}.f`) and nested-factory apply in the main residual
  (`(((mk 1) 2) 3)` leaks a Function value).
- **M3 — latent asymmetries flagged, unconfirmed.**
  `checkParamContract` vs `MatchSignature` guard drift; tail-call
  return checks using static `ConformsTo` instead of the runtime
  membership rule.
- (Closed: PARAM-GUARD-SKIP miscompile fixed with runtime param guards
  at `OpCallUser`; miscompile mechanisms B, C, D contained by refusing
  to compile — the divergence is gone, but each refusal is an open
  compile defect still owed a fix.)

**The hard wall, named honestly:** sound module-body / cross-registry
compilation (`module-test.tsv:38` and everything shaped like it) is
really **Stage-3 user-fn-call inlining with concrete-argument
propagation** — very high risk, needs a corpus re-baseline, two
attempts already reverted (`module-fn-checkstate-ownership.7.md`
Step 3; `boru-bytecode-stage3-inlining-plan.0.md`). Every completion
path (voxgig 48/48, refusals → 0) runs through it eventually.

---

## Part 2 — Definition of "finished"

Drawn from the projects' own asymptotes; nothing here invents a new
goal.

**Checker done =**
1. FP = 0 held *and made corpus-independent*: the known off-corpus FP
   class (G2 validate-then-call) legalized.
2. Every literal-decidable error shape statically flagged — the named
   remainder is predicate returns (G1). The 172 unflagged rows shrink
   only by class, never chase individual value-dependent rows.
3. Soundness pins reduced to the irreducible dynamic-dispatch core
   (target: pin ≤ 4 of the current 7, each with a written
   irreducibility rationale).
4. Any-frontier **ratio ceiling lowered stepwise** (12 → 10 → 8 %) as
   the two designed precision fronts (G4, G5) land; `do` over genuinely
   runtime-constructed code stays dynamic forever, by design.
5. **Check-by-default**: `boru run` preflights with `--no-check` opt-out
   (gated on 1).

**Compiler done = re-scoped P7.** The standard is a language that
compiles as a developer expects: **all valid code compiles, no
exceptions.** The corpus is the evidence for that, not its boundary — a
valid program that refuses off-corpus is the same defect as one that
refuses on it.
1. `compile == interpret` (values + error taxonomy) with **zero known
   divergences** — i.e. M1/M2 fixed; this is currently violated only
   by M1/M2.
2. Tier-2 reducible → 0 (one row: `flex`); compute frontier 68 → 0;
   refusals 151 → 0 **on the live corpus**, and every refusal raised
   afterwards — on the corpus or off it — tracked to closure as a
   defect, never accepted; islands tier-1-only (currently 1 non-tier-1
   island).
3. Delete the unbounded whole-program fallback: `RunCompiled` becomes
   `Compile` + `RunProgram`; refusal surfaces as a compile error; a new
   gate asserts every `OpFallback` span classifies tier-1.
4. External acceptance: **48/48** voxgig files under `--force-compile`.
5. Ceilings re-armed: once refusals/islands hit 0, restore them from
   informational to **gating** ratchets (the demotion was a
   growth-phase policy, not the end state).

Out of scope of "finished" (recorded so nobody scope-creeps it in):
bytecode serialization / interpreter-free `boru build` executables (no
doc proposes it; `build` today embeds source), multi-shot islands
(reverted twice, wrong direction), and chasing the 172 unflagged rows
to zero (explicitly never-zero by policy).

---

## Part 3 — The plan

Ordering principle: **correctness before coverage, precision before
frontier burn-down** (the compiler rides the checker, so checker
precision fronts convert refusals for free — the 81-row
dispatch-recovery bucket is mostly checker imprecision seen from the
compiler side), and the one very-high-risk item (Stage 3 inlining) goes
last with everything easier landed first to shrink its blast radius.
Every item lands gate-clean-or-revert under the standing discipline:
`make fmt && make vet && make lint && make test`,
`make verify-bytecode`, ratchets move down only, `make status` refresh,
per-row landing tests, one commit per item with before/after deltas.

### Phase 0 — housekeeping (immediate; hours)

| # | Item | Acceptance |
|---|---|---|
| 0.1 | Lower `pinnedTypeSoundnessViolations` 8 → 7 (test output already instructs it) | ratchet locked at 7 |
| 0.2 | `make status` — regenerate stale `COMPILED_STATUS.md` to the 3,875-row census | `TestCompiledStatus` fresh |
| 0.3 | Re-run the voxgig force-compile sweep + full `make test` (the PR #224 "re-gate before merging" debt) | 35/48 confirmed (or corrected) and recorded |
| 0.4 | Any-frontier headroom decision: with 0.48 pt to the 12 % ceiling, the next corpus-heavy merge trips the gate. Policy: **land Phase 2.1/2.2 before accepting large new corpus batches**; do not raise the ceiling. | written into this doc; enforced by review |

### Phase 1 — correctness debt (compiler miscompiles + latent drift; ~days)

| # | Item | Files | Acceptance |
|---|---|---|---|
| 1.1 | **M1** const-pool identity aliasing: stop const-baking fn-returned container literals whose identity can be observed (bake-refuse under `eq`-reachability, or allocate per call). Prefer the refusal-containment shape used for B/C/D — where it refuses, the divergence is contained and the compile defect stays open. | `eng/go/emit.go`, `bytecode_constbake_test.go` | the `(mk) eq (mk)` family differentially pinned; 0 known divergences |
| 1.2 | **M2** `/r`-deferred map-field auto-invoke + nested-factory apply residual | `eng/go/emit.go`, `carrier.go` | MISCOMPILE-HUNT rows compile correctly; where the change can only contain the divergence by refusing, the divergence is gone but the compile defect stays open and tracked |
| 1.3 | **M3** parity audits: `checkParamContract` ≡ `MatchSignature`; tail-call return check uses the runtime membership rule, not static `ConformsTo` | `eng/go/vm.go` | targeted differential tests for both seams |
| 1.4 | Soundness-pin burn-down: `forward-barrier.tsv:83` (concrete-condition folding), `recursion.tsv:53` (for-spread modeling); write irreducibility rationales for `patrun.tsv:40` / `module-rand.tsv:37` if they are the dynamic-dispatch core; triage `class.tsv:85`, `corpus-core.tsv:61` | `eng/go/carrier.go` | pin 7 → ≤ 4, each residual documented |

### Phase 2 — checker precision, tier 1: refinement types (~days; highest leverage per line)

| # | Item | Acceptance |
|---|---|---|
| 2.1 | **G1** — evaluate predicates on *concrete* carriers at return boundaries (machinery exists for parameters); `unverified_return` info under `--strict` for abstract carriers. Closes the `mkbad` hole; clears pin `user-types.tsv:124`. | probe: `mkbad` check exit 1; symmetric with parameter behaviour |
| 2.2 | **G2** — refinement-aware guard narrowing: `extractGuardClauses` accepts DepScalar-bodied types; then-branch narrows to the refined type, else-branch takes the complement where representable. Kills the validate-then-call FP class. | probe: `if (x is Big) [g x] [0]` checks clean; FP = 0 becomes corpus-independent; **unblocks Phase 5.2** |
| 2.3 | **G3** — `redundant_guard` + entailed `unreachable_branch` advisories (non-gating info): syntactic/interval entailment over DepScalarInfo bounds and carrier-vs-guard subsumption; no SMT. Also label the 104 fuzz dead-branch rejections with the same attribution (G10). | new severity-table codes + completeness gate; advisory precision measured like FORWARD-STRAND (≥ ~99 % on corpus sweep) |

### Phase 3 — compiler frontier, tranche 1 (independent of Phase 2; ~1–2 weeks)

Sequenced from `boru-bytecode-next-stages.0.md` (H, B, I already
landed):

| # | Item (stage) | Rows it moves | Risk |
|---|---|---|---|
| 3.1 | **Stage A** — variadic branch-result modeling (`stageA` doc is design-complete): arms may leave 0-or-N values and merge; unlocks conditional `RecordTrap` (nested/branch error rows), the computed-scrutinee `case` island, `fn m:` extra-values | the 1 island → 0; several error-rows; if-branch lowering | medium (the one structural lowering refactor) |
| 3.2 | **Stage D remainder** — dynamic-receiver `set`/`push`/`raise` (the stated voxgig bottleneck) | operand-provenance + voxgig files | medium |
| 3.3 | **Stage E** — VM reference cell for `flex` | the last tier-2 reducible row → 0 | medium |
| 3.4 | Error-row traps completion: `MathUtil!.nope`, minilang/parselang dynamic-source rows (needs 3.1's conditional traps) | error-row bucket shrinks; `correct-error` stays 0 | low |
| 3.5 | **Stage G remainder + fn-value frontier** — `fn-value.tsv:19`, method-through-map (`method-fnvalue-codebody` design), function-valued operands, unconsumed fn-value carriers | ~20 rows across 5 buckets | medium-high |

### Phase 4 — checker precision, tier 2: the two designed fronts (~2–4 weeks, staged)

| # | Item | Staging | Acceptance |
|---|---|---|---|
| 4.1 | **G4 — typed code values** (`CodeEffectInfo`, `Code[in→out]`): (a) literal producers + `do` consumer; (b) op-table `ChildTypeInfo` element-effect propagation; (c) higher-order consumers (`each`/`fold`). Compile pass **refuses** (never islands) `do` over an effect carrier until the VM grows an invocation path. | per `checker-precision-fronts.0.md` §1 | Any-ratio drops several points → **lower `anyFrontierRatioCeilingPct` 12 → 10**; dispatch-recovery refusal bucket shrinks measurably |
| 4.2 | **G5 — store-identity context typing** (`StoreShapeInfo`): abstract store carriers minted per creation site; ctx/set/get read/write the shape; PushContext copy-on-write layering; retire flat `ContextTypes` last | per §2, precision-only (unknown store → today's behaviour) | module-heavy frontier rows (test/log spans) drop; ceiling 10 → 8 where measured |
| 4.3 | **G7** — native `Returns`/`ReturnsFn` coverage gate + module-fn framework return types (recovers the +4 frontier rows from checkstate-ownership §6) | one sweep + a completeness test | `missing_returns` on the spec corpus → 0 |
| 4.4 | **G6** — options-schema lint: atom-keyed map literal flowing into an `Any` slot of a schema-less word → advisory; schema coverage sweep for the worst offenders (`emit`, …) | non-gating info first | probe `{prety:true}` diagnosed |
| 4.5 | **G9** — finish the emit/check decoupling (recorder interface; drop `CheckState.Emit`). Do this **before** Phase 6 — the module-body work is materially safer against a decoupled recorder. | comprehensive-review Tier-2 item 6 end state | `emit.go` consumes a narrow interface; no behaviour change (differential green) |

### Phase 5 — re-census + adoption flip

| # | Item | Acceptance |
|---|---|---|
| 5.1 | Re-census after Phases 2–4: the 81-row dispatch-recovery bucket and the 12-row typed-def-reparent bucket are expected to collapse substantially from checker precision alone; refresh `COMPILED_STATUS.md`; lower every informational count that moved | census recorded; remaining refusals re-bucketed for Phase 6 |
| 5.2 | **Check-by-default** (`boru run` preflights; `--no-check` opt-out) — now legal because 2.2 removed the known FP class | flip lands with release note; fuzz FP gate extended to the run path |
| 5.3 | Voxgig sweep re-run; ratchet the floor upward from 35/48 (must-not-drop becomes must-reach as Phase 3/6 items land) | recorded per-file with owning stage |

### Phase 6 — the hard wall: Stage 3 / Stage C (module bodies + user-fn inlining; the last big rock)

Sound cross-registry module-body compilation = user-fn-call inlining
with concrete-argument propagation. Two attempts reverted; treat as a
project, not a task:

1. **Design round first** — reconcile `boru-bytecode-stage3-inlining-plan.0.md`
   with `module-fn-checkstate-ownership.7.md`'s closing correction
   (closure-capture provenance is the root cause); decide the
   inlining boundary (per-call summaries vs body splice), the
   provenance model for captured carriers, and the corpus re-baseline
   protocol **before** code.
2. Land behind a flag with the full differential + `-race` +
   combinations matrix as the gate, on the smallest reproducer
   (`module-test.tsv:38`) first, then the fn-value/module buckets.
3. Only then sweep the remaining refusal buckets (operand-provenance
   cascades, dynamic-scope frames — Stage F) that cascade off it.

Exit: refusals → 0 and islands tier-1-only **on the live corpus**.

### Phase 7 — endgame (re-scoped P7 deletion)

1. Delete the unbounded whole-program fallback: `RunCompiled` →
   `Compile` + `RunProgram`; refusal is a compile error; new gate
   asserts every `OpFallback` span classifies tier-1.
2. Re-arm ceilings as **gating** at 0 (refusals, non-tier-1 islands);
   `correct-error == 0` stays.
3. Perf/alloc re-baseline; flip `--compile` default on (interpreter
   remains behind `BORU_NO_COMPILE` until a full release cycle passes).
4. Record the end state in a `.10` doc; open a *separate* decision doc
   if bytecode serialization for `boru build` is ever wanted — it is not
   part of this plan.

### Dependency sketch

```
0.1–0.4 ─┬─ 1.1–1.4 ──────────────┐
         ├─ 2.1 ─ 2.2 ─ 2.3       ├─ 5.1 ─ 5.2/5.3 ─ 6 ─ 7
         └─ 3.1 ─ 3.2/3.3/3.4/3.5 ┘
              4.1 ─ 4.2 ─ 4.3/4.4 ─ 4.5 (before 6)
```

Phases 1, 2, 3 are parallelizable across sessions; 4 follows 2; 5 needs
2+4 (ratio drops) and 3 (island/tier-2 zeros); 6 needs 4.5 and benefits
from everything; 7 is gated on 6's exit criteria.

### Risks

- **Corpus growth vs finish line.** Every new spec row is a compiler
  test; the frontier drifts up while the plan burns it down. Policy:
  finish lines are defined against the live corpus at each phase's
  re-census; the informational-ceiling policy stays until Phase 7
  re-arms them. Phase 0.4 protects the one gate that can trip
  spuriously (Any ratio).
- **Stage 3 is the schedule.** Phases 0–5 are well-understood,
  gate-protected work. Phase 6 has already eaten two reverts; the
  design-round-first structure and the 4.5 decoupling are the
  mitigation. If 6 stalls, everything else still lands — but a stall is
  not a resting place: the rows Stage 3 owns stay uncompiled defects,
  and the interpreter re-run that keeps them answering is scaffolding
  over that hole, not a safety net this plan may settle on.
- **Checker/compiler coupling cuts both ways.** Precision fronts
  (Phase 4) move refusal buckets for free, but any carrier change can
  move compiled behaviour; every Phase 2/4 item must run
  `make verify-bytecode`, not just the checker ratchets.

---

## Execution log

- **2026-07-04 — Phase 6 Stage M2c landed (shaped-instance-method dispatch):
  refusals 19 → 11, native 3605 → 3613; census deltas ONLY in the 8 targeted
  rows.** The M2 execution log's sketch, implemented with these recorded
  corrections (eng/go/method_shape.go is the feature's home):
  - **Sketch correction 1 — the stall was mis-attributed.** Live probes show
    the statement-position method apply ALREADY compiled when it sat at the
    program residual's tail (`l.info "req" ; 42` compiled natively via the
    leading OpCallDynamic window). The 8 rows refused because the call sits
    MID-PROGRAM: the unmodelled auto-dispatch stranded [dyn, args] on the
    check stack under the NEXT statements' events. So the feature is exactly
    the sketch's remaining two limbs: check-mode modelling at the point the
    interpreter dispatches, and the mid-stream guarded event.
  - **Sketch correction 2 — the "method-shape annotation" carries the MEMBER,
    not sigs+counts.** getNodeReturns' fn-member branch NOTES the resolved
    trivial-delegation wrapper against the read's fresh dynamic carrier
    (CheckState.MethodShapes, ID-keyed side table — the fnRiskFields/CtxShapes
    precedent; Begin-reset, Clone-copied). The carrier itself stays
    dynamic(Any), byte-identical to before — every checker ratchet
    (FP 0/3313, unflagged 170, soundness 7, frontier 195, fuzz 104) is
    unchanged. NoteMethodShape vets centrally: named delegation wrapper with
    a foreign sub-registry only, never a macro, and NEVER a member with a
    genuine 0-arg overload — the miscompile-E auto-dispatch class
    (module-log:72,73 span.finish; module-rand:14,15 bool/float) cannot be
    annotated, and their reads still refuse at the get-family guards first;
    all four rows verified refusing with the identical reason.
  - **The model reuses the interpreter's own machinery, not a mirror.** When
    the compile pass steps the annotated carrier at the pointer (stepLiteral —
    where execFnDefLiteral would dispatch the concrete member),
    tryShapedMethodDispatch runs the ENGINE'S OWN matchSignature over the
    same tape and commits only when the match is PURE-FORWARD over a
    contiguous, inert, evaluation-fixed statement window (concrete scalars /
    atoms / literal containers with no words, parens, interps, reaches,
    computed keys) — so the compiled window is the exact token sequence the
    interpreter's End-bounded forward collection consumes; any stack-reaching
    or partial match declines to today's paths. The dispatch then models
    through the SAME carrierResults (declared returns, folds, contagion),
    with the outcome seam routed by CheckState.PendingMethodApply to the new
    RecordDynMethod — first in recordDispatchOutcome's chain, so the member's
    inner native can never record as a check-time CALL_NATIVE (which would
    bake the shape instance's sub-registry: the freeze-gate).
  - **The guarded op.** OpCallDynMethod (spec table DynMethods: word, NArgs,
    NOut) lays out [args…, fn-on-top] (sig order — callDynTrailTop's proven
    reversed-window forward bind); the fn operand is the dot-read EVENT (the
    runtime value; nothing of the shape instance bakes), args are interned
    consts. The VM applies via the delegation fast path or the island and
    enforces BOTH claim halves: a non-callable/quoted runtime value or a
    result-count mismatch raises internal_error → RunCompiled re-runs the
    interpreter (runtimeShouldFallback — the containment path: the defect
    is absorbed SILENTLY and the run still answers, loud only under
    --force-compile). Pinned both ways by
    TestShapedMethodClaimViolationDefers (a registered lying shape:
    Returns [] declared, 1 value returned → fallback with the correct
    [7 42], internal_error under force) and
    TestShapedMethodRegisteredShapeCompiles (the honest twin runs compiled).
  - **Row 55's second half needed one shape addition:** logger-child now
    declares ReturnsFn loggerShapeReturns (state-independent, like the
    parent constructors), so `def c (l.child {y:2})` binds a shape instance
    and `c.info` resolves — the paren-bounded row rides the same model (the
    model runs inside paren frames unchanged).
  - **Two adjacent pre-existing defects found by the landing battery, both
    fixed:** (a) tryNativeFnApply (the CALL_DYNAMIC-family fast path) handed
    the HANDLER the fn's sub-registry where the interpreter's execMatch hands
    the dispatching engine's registry — host state installed on the instance
    (a frozen clock, policy, output) was silently dropped on the compiled
    fast path only (a frozen-clock logger stamped WALL time compiled; name
    resolution keeps using fnDef.Registry). Fixed to vc.r; pinned by
    TestShapedMethodEffectOrdering (byte-identical print/sink interleaving
    under a frozen clock). (b) The residual LEADING/MIXED dynamic-apply
    windows absorb values across statement boundaries — probe-confirmed
    live miscompile on the committed tree: `c.add (1 add 2) ;
    Log.measurements size` compiled to [3] where the interpreter gives [1]
    (the window fed the NEXT statement's results to the method as args).
    Fixed for the class this feature owns: an ANNOTATED method-read carrier
    now declines the leading and mixed residual windows outright
    (methodShapeAnnotated — the statement-window model is the only owner of
    its apply; trailing shapes draw from the STACK, the interpreter's own
    cross-statement stack-form dispatch, and stay). The narrowing cost ZERO
    corpus rows; the divergent shape is pinned refusing with fallback parity
    (TestShapedMethodComputedArgStaysRefused). The UNANNOTATED instances of
    the same hazard (plain fn-value maps mid-stream) remain a latent
    pre-existing corner — flagged for the M-stage review beside the M2 log's
    OpCallDynTrailTop quoted-fn note.
  - Census: refusals 19 → 11 (fn-value-call boundary 7 → 0, paren-bounded
    1 → 0; dispatch recovery 5, container auto-dispatch 4, operand provenance
    2 all byte-identical); native 3605 → 3613; islands still 1 (none added);
    tier-1/2 0; error rows 3 unchanged; computeRefusalCeiling 17 → 9 with
    rationale (remainder: auto-dispatch guard 4, flex G5 2, dynamic-scope
    M6 2, island 1). Differential 3356/0, or-fallback 3589/0. Landing tests
    lang/go/bytecode_methodshape_test.go (8 positives incl. the paren-bounded
    child row, effect-order pin, 4 guard-row refusal pins, capturing-member
    and computed-arg negatives with fallback parity, claim-violation
    defer + honest-shape positive). Phase 7's entry now reads: 11 refusals =
    4 auto-dispatch guard (open until a runtime model is built) +
    2 flex G5 + 2 recursion M6 tier + 3 non-definite error rows, plus
    the 1 compute-frontier island.

- **2026-07-04 — Phase 6 Stages M3 + M4 landed (DSL registration + definite
  traps): refusals 40 → 19, native 3584 → 3605, error allowlist 12 → 3;
  census deltas ONLY in the targeted rows.** Per
  design/STAGE3-INLINING-DESIGN-ROUND.0.md §6, with these recorded design
  corrections:
  - **M3 module-parse (11/11 targeted +1 bonus: module-parse.tsv:14,18,37-44
    landed AND :15, the "dynamic input" row the design said might need M2c).**
    The design offered "a check-only registration table consulted by
    moduleExportGet, or Pos-idempotent runtime registration"; the landed
    mechanism is NEITHER install — the `parse` macro's deferred-kind branch
    (native_macro.go) now RECORDS one CALL_NATIVE of a new
    parselang-internal word, `parselang-deferred-dispatch` (kind, source,
    opts → dynamic(Any)), whose runtime handler resolves `parse_<kind>`
    against the LIVE export map (by then populated by the compiled
    Parse.register call) and replays the parse macro's own expansion tail
    (wrapper value + source + opts + end in a sub-engine — the token
    sequence the interpreter re-steps after `ParseLang dot parse_<kind>`
    resolves); a kind still missing at run time raises the macro's
    byte-identical parse_unknown_lang. The runtime lookup IS the
    registration proof, so a runtime-conditional Parse.register needs no
    static refusal: the miss raises exactly as the interpreter does
    (TestDeferredParseDispatchMissParity; the interpreter stamps the `parse`
    word, the compiled op the kind atom — code+detail identical, both
    positions present, the full-corpus error-lane contract). The check pass
    installs NOTHING into the export map — the reverted register-ReturnsFn
    LEAK stays closed (TestDeferredParseCheckObservationFree re-runs the
    row interpretively on the same instance after a compile pass; a leak
    would raise "already registered"). module-parse:15 cleared as a side
    effect: with provenance on the parse result, the dot's EXISTING poly
    recovery records where "dynamic input at dot" used to refuse.
  - **M3 minilang:320 (1/1).** A second mechanism the design only implied:
    tryFoldModuleConst's missing-key None fold declines because "a module's
    keyspace can GROW at runtime" — made provable via a module-export GROWTH
    LEDGER (eng/go/module_export_growth.go, pointer-keyed by the exports
    OrderedMap through a new eng ExportFieldsCarrier interface on the lang
    payload — the §4 pointer-payload discipline). MiniLang registers its map
    at build time (its program-reachable growers are audited: register +
    register-compiled; host registration is between-runs); each grower's
    check twin notes the keys its runtime execution may install (lang_<kind>
    always; the capitalised member-type key unless the fn is provably
    non-filter-shaped; compile_<name> for register-compiled) or POISONS on a
    non-concrete name; notes are per-pass (reset beside
    ResetParseDeferredKinds). The fold then admits exactly the
    ledger-proven-stable absence: `MiniLang.Gen` after registering a
    non-filter `gen` folds to None (the row's whole point — no member type
    is ever minted), while `MiniLang.Gen2` after a FILTER-shaped register
    and `MiniLang.lang_gen` stay declined (TestMiniLangAbsenceFoldCompiles
    pins both directions — folding those would bake a stale absence).
  - **M4 definite traps (9 landed: apply:37,38 · open-words:32,83,84,90,100 ·
    generics-sugar:37 · generics:60 — within the 6-10 forecast).** The
    design named "provably disjoint carrier args"; the landed proof is
    stronger and pairing-independent: per overload, assignment FEASIBILITY —
    bipartite matching of slots against the complete candidate window
    (checkModeFallbackPositions(maxN), which bounds every forward/stack
    split), with an edge wherever the pair is not PROVABLY failing (concrete
    values replay the matcher's own per-value test via definiteSlotFail's
    mirror of the scan's word-resolution chain; strict carriers use the
    residualProvablyDisjoint core extended with sigTypeMatches' structural
    carves — Options slots take concrete maps, Map/Node slots take
    Options/Record tags — plus Word-disjointness against the scan's
    word-coercion branches). Needed beyond plain disjointness because add's
    3-arg [Map Any Patrun] overload has an Any slot (defeats universal
    disjointness; its Map slot is the zero-edge kill) and open-words:100
    needs the COUNTING argument (two Point slots, one Point-compatible
    candidate — the Integer must occupy one). Two probe/gate-found
    soundness corrections: (a) a CONTAINER-family carrier declines — a
    check-mode `for` records its value-body residue as ONE typed-list
    carrier where the runtime leaves loose per-iteration values, so the
    tag-conformance premise fails (`add 100 (for 4 [add i 1])`, caught by
    TestCompiledCombinationParity); (b) the trap window must not cross an
    UNEVALUATED paren group (in-paren raw tokens are not what the runtime
    match examines). Both were latent pre-M4: such programs died at
    Finalize's residual seating, which now legitimately relaxes — a
    trap-truncated program with unconsumed prior call results compiles (the
    interpreter aborts at the same point with those values live;
    TestTrapKeepsPriorCallEffects). The stale
    "carrier operand declines" negative (`5 inc apply`) became the positive
    battery (TestUnmatchedDispatchTrapCarrierDisjoint, with exact
    code+detail+position parity); the new negatives pin the shapes that must
    keep declining: the REFINEMENT ESCAPE (a Boolean-declared return
    carrying a Flag-reparented tag matches the merged [Flag Flag] overload
    at run time — Flag ⊑ Boolean blocks the proof), a predicate param that
    PASSES at run time, the matching disjunct alternative
    (forward-barrier:80's shape), and the splice/flex-Reach pins.
  - Census: refusals 40 → 19 (operand provenance 13 → 2 = recursion:71,72;
    dispatch recovery 14 → 5 = convert-ideal:30 (its Foo carrier CONFORMS to
    the [Node Ideal] overload's Ideal slot — feasible, so the trap proof
    declines and the row stays an open defect), word-splice:115 (splice
    screen), forward-barrier:80 (List alternative), flex:88,95 (Reach pin);
    dynamic input 1 → 0; fn-value-call boundary 7, paren-bounded 1,
    container auto-dispatch 4 unchanged = the M2c stall + the miscompile-E
    guard); islands still 1 (none added); tier-1/2 0; error rows 12 → 3
    (the only permitted direction — the 9 M4 rows now COMPILE to
    byte-identical taxonomy); computeRefusalCeiling 29 → 17 with
    rationale. Full battery green before and after (differential +
    or-fallback + combinations + property fuzz + -race + borudebug: VERIFY
    PASSED; make test exit 0). Remaining 19 + 1 island against the design's
    §7 forecast: 8 fn-value shapes need the M2c shaped-instance-method
    feature, 4 sit on the auto-dispatch guard (open until the runtime
    model is built), 2 are the M6 dynamic-scope tier (recursion:71, 72),
    2 are G5 flex path-shape typing (flex:88,95), 3 are non-definite
    error rows (convert-ideal:30, forward-barrier:80, word-splice:115),
    plus the 1 compute-frontier island — Phase 7's entry now reads
    "0 outside documented tiers" over exactly these families.

- **2026-07-04 — Phase 6 Stage M2 landed (fn-value frontier, partial-M2c):
  refusals 68 → 40, native 3556 → 3584; census deltas ONLY in
  the targeted buckets.** Per design/STAGE3-INLINING-DESIGN-ROUND.0.md §6
  Stage M2, with these recorded design corrections:
  - **M2a (3/3: recursion.tsv:90-92).** The design named "route the fn-unit
    residual through resolveDynamicApply + extend trailingApply to a local
    fn"; the landed mechanism differs: the `apply` dispatch over a
    Function-typed CARRIER is ELIDED with a pending-apply ledger on the
    enclosing unit (recordCallElided → emitUnit.pendingApply), and
    StartFnCompile's finish lowers the ONE pending as the whole-residual
    window — the fn's arity is unknowable at the apply site, so only the
    whole window is faithful to applyHandler's re-step — or refuses ("apply
    of a dynamic fn value not at the body tail"), so a pending apply can
    never compile as unapplied data. Correction found by probe: applyHandler
    UNQUOTES the fn, so a /r-parked (Quoted) callee needs a NEW opcode
    OpCallDynApplyTop (unquote-then-apply; applyHandler's byte-identical
    non-fn error) — OpCallDynTrailTop keeps the paren semantics (a quoted fn
    stays data). RecordDynApply consumes a matching pending and flags the
    event (dynApplyUnquote) for the same opcode split.
  - **M2b (12/12: path-modifier.tsv:17-20,23-25,28,52-55).** The real blocker
    was NOT get-side folding: the modifier words (usurp / stack-args /
    forward-args / force-arity) are RunInCheck, whose dispatches bypass the
    check intercept and recorded NOTHING — the wrapper value reached the
    residual with no provenance. Landed: the gradual branch
    (checkModeGradualFn) records an OpCallNativePoly (recordGradualWrap →
    RecordPolyCall; the VM's callPoly re-matches the word's own sigs with
    the kernel MatchSignature, so a runtime non-fn raises the identical
    illegal_ref), and the residual applies the runtime-built wrapper via the
    EXISTING leading OpCallDynamic — plus a new TRAILING-window shape
    (trailingWindowApplyShape → OpCallDynamicMixed over the whole residual,
    events promoted to locals): `10 3 m.s/s` islands the window VERBATIM,
    because a BarrierPos-0 stack-args wrapper collects nothing forward and
    cannot ride the trailing-1 rotation contract. The by-name Atom forms
    stay unrecorded when the resolved value is dynamic (no runtime def
    stack); recording keys on the RESOLVED value, never the name.
  - **M2c (2/14: module-log.tsv:62,83 — the rest reported below).**
    Log.register declares CompileStoresFn (the minilang/parselang register
    precedent): the sink fn is stored and invoked by the Go-side sink
    machinery through a fresh sub-engine, never on the tape; a pure fn
    literal bakes as a const, a capturing one refuses. STALLED — the
    instance-method rows (module-log 53,54,56,77-80 statement-position
    `l.info "req"` / `c.add 1`, + 55 paren-bounded `(l.child {y:2})`): the
    method read is deliberately dynamic (loggerShapeReturns' check-mode
    shape instance must never bake — freeze-gate: the runtime instance
    carries per-call state), so compiling these needs the shaped
    instance-method dispatch model the design sketched: a method-shape
    annotation on the read result (member sigs + return count from the
    ReturnsFn shape instance), check-mode modelling of the auto-dispatch at
    the point the interpreter performs it, and a mid-stream CALL_DYNAMIC
    event with shape-known nout (0 for the log methods) — a guarded opcode
    can defer to the interpreter via internal_error if the shape claim ever
    fails at run time. That is a coherent one-session feature but engine
    surgery beyond this session's gate budget; nothing landed toward it, so
    the 8 rows are byte-identical refusals. module-log 72,73 +
    module-rand 14,15 stay on the miscompile-E auto-dispatch guard
    (containerFnAutoDispatchRisk / zeroArgFnOut) — the guard was not
    weakened, and the four rows stay open defects behind it.
  - **M2d (11/11: module-minilang.tsv:306-315 + corpus-core.tsv:134).** The
    design's "bake the fn as a const where the consumer treats it as DATA"
    landed, with a correction: `is` CANNOT take whole-sig CompileReadsFn — a
    Function in its TYPE slot is a predicate the handler INVOKES
    (RunPredicate; `5 is Positive`, pinned by TestFnValueIntrospectionLowers
    and TestRunCompiledFallbackIsolation, both of which caught the blanket
    attempt). Landed the positional Signature.FnInertArgs{1:true} (value
    slot only) + the positional gate in recordCallOperands. Second
    correction: minilang:314's real blocker was StripToCarriers destroying
    the parser-emitted `/r` dispatch-mod marker (Word/__DM) in check mode —
    the marker leaked into the check stack as a phantom carrier the runtime
    never has; toCarrier now exempts it as the control token it is, so an
    inline `(lambda)/r` parks/drops exactly as the interpreter (pinned:
    `(1 add 2)/r` → 3 native). corpus-core:134's two-lambda walk landed by
    compiling the ASCEND-slot lambda to its OWN closure unit
    (extraNoEvalHookSlots → recordClosureDispatch extras →
    RecordClosureCall extraOps; walkClassifyHook already classifies a
    compiled closure per slot); the Phase 3.2/3.3 "lambda ascend stays
    refused" negative pin was stale against this design and is now the
    positive two-lambda parity case (a CAPTURING ascend lambda is the new
    negative).
  - Stale-doc note: `(3 and "x") add 1` (next-stages Stage H) already
    compiles natively with the interpreter's operand order on this branch;
    the M2 revert-criteria pin is now a POSITIVE order pin ('x1', never
    '1x'). Latent corner observed, unchanged: OpCallDynTrailTop islands an
    appliable-but-QUOTED fn as [fn, args] where the interpreter's paren
    residual would be [args, fn] — unreached by the corpus; flagged for the
    M-stage review.
  - Census: refusals 68 → 40 (function-valued operand 5 → 0;
    fn-value-reaches-word 11 → 0; unconsumed carrier 3 → 0; operand
    provenance 22 → 13; fn-value-call boundary 7, paren-bounded 1 and
    container auto-dispatch 4 unchanged = the reported M2c stall; dispatch
    recovery 14 and dynamic input 1 untouched); islands still 1 (none
    added); tier-2 0; error rows 13 → 12 (module-log:83 left the allowlist
    the only permitted direction — it now COMPILES to the byte-identical
    runtime sink-exists error); computeRefusalCeiling 56 → 29 with
    rationale. Landing tests: bytecode_fnvalue_m2_test.go (4 batteries,
    positives + order/unquote pins + refusal negatives with fallback
    parity), TestWalkHookClosureCompiles two-lambda positive + capturing
    negative. M3 (DSL registration) and M4 (definite traps) are unblocked —
    neither depends on the M2c stall.

- **2026-07-03 — Phase 0 executed.**
  - 0.1 `pinnedTypeSoundnessViolations` lowered 8 → 7 (generics.tsv:75
    left the violation set; the remaining 7 verified live:
    class.tsv:85, corpus-core.tsv:61, forward-barrier.tsv:83,
    module-rand.tsv:37, patrun.tsv:40, recursion.tsv:53,
    user-types.tsv:124).
  - 0.2 `make status` — `COMPILED_STATUS.md` regenerated to the live
    3,875-row census (3,475 native + 1 island, 151 refused, 248
    check-error).
  - 0.3 **voxgig sweep: blocked-external in this environment** — the
    client corpus is a separate repository not present here; the
    35/48 figure from PR #224's message remains unverified. The
    `make test` half of the re-gate debt is covered (full suite green
    on this tree). The sweep re-run stays owed at the next
    environment that carries the corpus (also gates Phase 5.3).
  - 0.4 headroom policy in force as written (no ceiling raise; Phase
    2.1/2.2 before large corpus batches).
- **2026-07-03 — Phase 1.1 (M1) landed** (`fix(compile): per-call
  identity for fn-body container literals`): OpPushConstFresh +
  ID-pooled compound interning + escaping-multi-read refusal; probe
  matrix 5 divergent shapes → parity, sharing/deq preserved; census
  unchanged (zero coverage cost); VERIFY PASSED + full suite green.
- **2026-07-03 — Phase 1.2 (M2) landed**: both Mechanism-E remainders
  now refuse instead of miscompiling — the divergence is contained,
  two open compile defects left behind. Deferred-field auto-invoke:
  get-family reads from containers holding a GENUINELY 0-param fn
  member refuse at both record sites (receiver-keyed; concrete keys
  inspect only the read member; the parked phantom 0-arg sig is
  discounted, so applied member calls `m.b 2` and multi-param member
  reads keep compiling — pinned by TestEmitFnValueFieldCallCompiles).
  Nested-factory curried chain: statically-recovered closure arity vs
  tail-arg count refuses the chain, single-apply factories keep
  compiling. Census cost: 2 rows (3474/153 vs 3476/151), both
  previously divergence-exposed.
  Landing test TestFnValueAutoApplyRefusals (4 refusals with
  fallback-parity + 4 preserved).
- **2026-07-03 — Phase 1.4a landed: comparison concrete folding
  (CompileScalarFold).** eq/neq/deq/cmp/tcmp/lt/lte/gt/gte over
  compile-time-known scalars now fold to their CONCRETE result in
  check mode (tryFoldScalarConst — double-eval agreement guard like
  the module fold; check-mode literals ride as concrete-payload
  carriers, admitted by scalarFoldOperand). A/B-verified: `5 eq 0`
  was a bare Boolean carrier before, concrete `false` after. This is
  the named PREREQUISITE for the forward-barrier.tsv:83 pin
  ("needs comparison ops to fold concretes"); the pin itself stays
  until the `if` analysis COMMITS statically-determined branches —
  probe shows even `if false [1] [2]` joins both arms today — which
  belongs with Phase 3.1 (Stage A branch-result modeling). All
  ratchets hold (FP 0, unflagged 172, soundness 7, frontier 381,
  fuzz FP 104); census cost 4 rows, all one new bucket
  ("if-branch lowering [scheduling]" — branch lowering meeting const
  conds where it expected events), reclaimed by Phase 3.1. Landing
  test TestScalarConstFold (5 folds + param-carrier and
  cross-family negatives). Soundness pin stays 7: recursion.tsv:53
  needs recursion unrolling (Phase 3/6 territory);
  patrun.tsv:40 + module-rand.tsv:37 are the irreducible
  dynamic-dispatch core (a Patrun/seeded-instance member's static
  type is unknowable by design); class.tsv:85 (const-singleton field
  widened by set) and corpus-core.tsv:61 (enum identity) remain
  triage candidates for Phase 2/4.
- **2026-07-03 — Phase 2.1 landed: static return-value membership.**
  `checkBodyReturnConformance` now asks `got.Is(exp)` for a
  compile-time-known SCALAR body residual — the mkbad predicate-return
  hole is closed statically (probe: check exit 1 with the
  byte-identical runtime type_error) and the nominal newtype-return
  hole with it; reparented (`def x:UserId 42  x`) and good-predicate
  bodies stay clean; abstract/value-dependent residuals keep the
  runtime RET check by design. pinnedUnflaggedErrorRows lowered
  172 → 170. ALSO fix-forward for 31913df: the scalar fold now KEEPS
  recording the dispatch (`folded.ID = out[0].ID`) instead of eliding
  the event — event elision stranded every lowering that anchors on a
  condition event (emit goldens, computed-arm if, variadic-else
  claims; the 31913df battery was red). With recording preserved all
  pinned lowering tests pass, the 4-row census dip reversed, and the
  2 newly-flagged rows reclassified compiled → check-error
  (3472 compiled / 153 refused / 250 check-errors).
- **2026-07-03 — Phase 2.2 + 2.3 landed: predicate guard narrowing +
  redundant_guard.** extractGuardClauses now admits DepScalar-bodied
  guard types, narrowing to the MINTED lattice node
  (DefEntry.TypeDef; the constraint value's Parent is only the base
  family), whose depScalarUnifier admits tag-conforming abstract
  carriers nominally. Validate-then-call (`if (x is Big) [g x] [0]`,
  paren AND list guard forms) checks clean — the off-corpus FP class
  and named check-by-default blocker is legalized; unguarded and
  else-branch misuse still gate; runtime parity verified both
  directions. ApplyGuardNarrowing additionally emits the non-gating
  `redundant_guard` info when the guarded binding is a NON-CONCRETE
  strict carrier whose tag already entails the guard (`if (n is Big)`
  with n:Big) — concrete per-shape bindings and dynamic bindings stay
  quiet (a dynamic guard discharges the modality; value-level interval
  entailment is future work). New severity-table entry (Info tier).
  All pins hold (FP 0, unflagged 170, soundness 7, frontier 381, fuzz
  104); census/differential/partition unchanged-green; full lang and
  eng suites ok. Landing test TestPredicateGuardNarrowing (2 clean
  forms, 2 gating negatives, runtime parity, advisory positive + 2
  advisory negatives).
- **2026-07-03 — Phase 3.1 (Stage A) verified ALREADY LANDED; gate
  re-run green; one soundness negative pinned.** The stageA design doc
  and the completeness audit were stale: commit 31f02f3 (2026-06-28,
  ancestor of this branch) implemented variadic branch-result modeling
  — resolveArm + fragMulti multi-value arm accounting, allowVariadic
  fragment propagation, the variadic CALL_USER sentinel with a seeded
  FnSummaries hypothesis for recursive convergence, and
  seatResults/reconcileResults tail-position acceptance (VM unchanged,
  per the doc's own Correction 1). recursion.tsv:53 compiles natively
  and RunCompiled == Run == [6 4 2]; the m 0/1/3/5 convergence and the
  soundness gate are pinned by TestStageAVariadicBranchResult /
  TestStageAVariadicSoundnessGate, to which the missing paren-form
  negative `add (m 3) 1` was added (the only diff). Full battery
  green: VERIFY PASSED, make test exit 0, differential 0 divergences,
  census 3472/153/250 with the "branch leaves extra values" and
  "if-branch lowering [scheduling]" buckets both at 0. Residual noted:
  `def p fn [[n:Integer] [Integer Integer] [n n add 1]]` refuses as
  "result above a literal (Stage 3)" — a Stage-3 seating gap, tracked
  under Phase 3.5/6. recursion.tsv:53 stays among the 7 soundness pins
  as a checker-PRECISION straggler (static spread modeling), distinct
  from the compile-coverage gap Stage A closed.
- **2026-07-03 — Phase 3.2/3.3 landed: tier-2 → 0 (the P7 tier-2
  floor).** Live-state verification showed the docs stale AGAIN: the
  doc-named Stage E flex rows and ALL three Stage D rows already
  compile on this branch. The REAL remaining tier-2 row was
  corpus-core.tsv:50 — core `walk` (visit-only traversal) had no
  closure model ("code-body word walk (Stage 2)"). Landed: a
  Callable spec for walk with LambdaSharesTokenShape, a
  provenance-proven empty-flex ascend-hook admission (main-registry
  `flex` sig identity + empty const containers + top-level scope + no
  intervening event → the classify-time token snapshot stays empty,
  byte-identical), and moduleScopeMutableCaptures threading for
  lambda bodies so the flex accumulator mutates the same cell in both
  engines. reducibleCeiling ratcheted 1 → 0 with rationale;
  refusals 153 → 151; compute frontier 70 → 69. Landing test
  TestWalkHookClosureCompiles (6 positives incl. loop-iteration
  mutation + 5 refusal negatives). Full battery green on the final
  tree (VERIFY PASSED, make test exit 0, differential 0 divergences);
  orchestrator spot-checked the corpus row parity via CLI. Residuals:
  the two-lambda 4-arg walk stays an open Stage-3 refusal; the last
  "dynamic input" row is module-parse.tsv:15, not a Stage-D shape;
  the stale next-stages/finish-line doc sections are noted, unedited.
- **2026-07-04 — Phase 3.4 landed: unmatched-dispatch trap programs —
  error rows 83 → 13, refusals 151 → 81.** The dominant class (70
  rows) was the check-mode dispatch-recovery fall-through DISCARDING a
  failure it had proven: tryRecordUnmatchedDispatchTrap (engine.go)
  now records a terminal OpTrap with the byte-identical taxonomy
  (plain signature_error and the void-arg def_error/no_value_error
  variants via the interpreter's own voidArgErrorFor), gated on
  definiteness — declines on any carrier / dynamic / deferred-token
  operand (Reach/ParenExpr/InterpString/Splice; the flex-Reach
  soundness counterexample found mid-development is pinned as a
  negative) and stays top-level-only via RecordTrap's existing guard.
  Finalize lowers a trap-truncated unfinished fn unit as an
  unreachable defensive stub. Landing tests: 10 trap positives with
  code+Detail+position parity, prior-effect ordering
  (`print 'a' raise 42`), and carrier/splice/flex-Reach negatives.
  Census: native 3473 → 3543. Partition now 0 tier-1 / 0 tier-2 /
  13 error-row / 69 compute. Residual 13: 7 need carrier-disjointness
  proofs, 1 branch-arm trap modeling, 3 unblock via typed-def
  compilation (cluster 3), 2 not statically definite yet. Full battery
  green (VERIFY PASSED, make test exit 0, differential + fallback 0
  divergences); orchestrator spot-checked taxonomy + effect-order
  parity via CLI.
- **2026-07-04 — Phase 3.5a landed: compiled typed-def
  store-with-reparent (OpBindTyped) — both typed-def refusal buckets
  → 0, refusals 81 → 78.** RunTypedBind (typed_bind.go) mirrors
  defTypedHandler exactly (predicate → RunPredicate + conditional
  ReparentValue; refine → Unify + ReparentValue over the CANONICAL
  node; DepScalar → self-contained Unify, no reparent), with the spec
  carrying the *Type pointer (module sub-registry mints resolve via
  CanonicalType at run time, never a main-table ID). RecordTypedBind
  remints the bound value's provenance ID — the exact §B aliasing
  mechanism, closed. The op rides the generic emitCall machinery
  (promotion/dead-drop free) rather than a fused store. Probe-parity:
  typeof renders the newtype compiled (was Integer — §B divergence),
  and the plain-fmt.Errorf validate-fail text is byte-identical
  (pinned via RunCompiledStrict; the interpreter's own errors carry
  no position, so none is added). Census: native 3543 → 3546; of the
  12 rows, 3 compiled outright and 9 progressed to the open-words
  merged-word dispatch frontier (user-fn-call 4 → 10, dispatch
  recovery 11 → 14); error rows stay 13 (open-words 83/84/90 now
  refuse at best-guess recovery, not the definite trap seam).
  computeRefusalCeiling ratcheted 86 → 66. Landing test
  TestTypedDefBindCompiles (6 positives, 3 byte-identical FAILs,
  2 negatives). Full battery green (VERIFY PASSED, make test exit 0,
  0 divergences).
- **2026-07-04 — Phase 4.1 (stage 1) landed: CodeEffectInfo typed code
  values — and a leverage recalibration.** The producer
  (AnalyseCodeEffectCarrier: dry-pass RunCarrierBody over concrete
  word-bearing list elements at the get-index read site, diagnostics
  truncated, compile-pass-gated) and the `do` consumer (effect Out
  surfaced as BOUNDED dynamic(T) — deliberately not strict, since the
  body's free words re-resolve at do time; a bound, not a proof) are
  in, with payload-discipline registration, a recursion guard, and 5
  landing tests incl. compile-refusal pins (checker precision must
  not imply compile coverage — verified nothing mis-lowers). All
  ratchets byte-identical (FP 0, unflagged 170, soundness 7, frontier
  381 = 11.52%, census 3547/78, VERIFY PASSED, make test 0).
  RECALIBRATION (live-verified): the spec's "do-hatch dominates the
  frontier" estimate is stale — the 381 frontier rows contain ZERO
  get-index→do rows; the real feeders are G7-class declared-Any DSL
  parser returns (minilang 47, struct 32, parselang 31) and G5
  stores. The 12→10 ratio-ceiling step therefore hangs on Phase 4.3
  (Returns coverage) and 4.2 (StoreShapeInfo), which move ahead of
  CodeEffectInfo stages 2–3 in leverage order. Stage 2/3 scope
  recorded by the agent (op-table ChildTypeInfo join with loud decay;
  higher-order consumers; fn-param boundary currently strips the
  effect).
- **2026-07-04 — Phase 4.3 landed: declared-Any burn-down + the
  Returns-coverage gate — Any-frontier 381 → 220 (11.52% → 6.65%),
  ratio ceiling 12 → 8, missing_returns 87 → 0.** Truthful
  Returns/ReturnsFn annotations across the real feeder families
  (minilang 47→10 via record-shape returns; struct 32→6 with
  handler-mirrored container kinds; parselang 31→9 via PURE
  concrete-source folds through the real handlers; corpus-core 23→4;
  module-test 21→9; Vm 19→8; + make's record FIELD SCHEMA riding
  instance carriers with nested propagation enabling r.fst.m). The
  new TestNativeReturnsCoverage gate enumerates the default registry
  AND every boru: module sub-registry: every sig must declare check
  returns — allowlist EMPTY (512 raw shapes all annotated).
  pinnedAnyFrontierRows 345 → 220. Two incidents resolved per
  discipline (annotation fixed, never the pin): a tany Disjunct fold
  read as a union carrier (soundness 7→8 transient — Disjunct/Enum
  fold results now ride dynamic); a Test.prop shape claim starving
  run-property dispatch (withdrawn, documented). All other ratchets
  byte-identical; census unchanged; VERIFY PASSED; make test 0.
  Residuals: patrun/flex/log stores (G5 → Phase 4.2), error .code
  (sound Any), user-registered DSL fns (G4-adjacent), Vm.run*
  (honestly Any), the module-fn return-seam schema loss (follow-up).
- **2026-07-04 — Phase 4.2 (stage 1 + natural stage-2 slice) landed:
  StoreShapeInfo store-identity context typing — Any-frontier
  220 → 195 (6.65% → 5.90%), ratio ceiling 8 → 7.** The abstract
  payload (`*StoreShapeInfo{Scope, KeyTypes, Vals, ValsPoisoned}`,
  eng/go/store_shape.go; pointer payload so every carrier copy aliases
  ONE shape, mirroring runtime container aliasing; KeyTypes carries
  VALUES not bare *Type — the JoinCarriers domain, the ChildTypeInfo/
  flat-ContextTypes precedent) is minted per creation site / live
  layer in PLAIN check mode by the live store-creating words: `context`
  (one shape per live ctx LAYER, keyed by the *StoreInstanceInfo
  pointer — stable in check mode because the COW Impl never runs; each
  engine Run's pushed layer gets its own shape, so the stage-2
  scope-correctness headline falls out: a `do` body's same-named ctx
  write no longer contaminates the outer read, agreeing with the
  runtime pop), `flex` over a concrete map (nested maps recursively,
  mirroring FlexDeepCopy; flex-of-flex CLONES; handles stored in
  another flex SHARE), and `patrun` (unkeyed value join). set/get over
  a shaped Store, set/get/dot over a shaped FlexMap (reads surface
  GRADUAL dynamic(T), the record-schema rule — flex trees have
  untracked writers), and add/find over a shaped patrun (find =
  dynamic(join ∪ None); a dispatch-bearing stored value POISONS the
  join, so the pinned patrun.tsv:40 residual is untouched — the
  "irreducible" pin verdict is refined: the CONTAINER is shapeable,
  the stored-fn dispatch is not) read/write the shape; every miss and
  every unshaped store keeps the flat-ContextTypes path byte-for-byte
  (writes go to BOTH maps), so precision only increases. Live-state
  findings: the spec's `ctx-set`/`ctx-get`/`make Store` words do NOT
  exist — the live surface is set/get/context + the G5 residual list's
  flex/patrun; and store/flex/patrun rows COMPILE natively today (the
  "store ops refuse" note was stale), so every shape path is
  !Compiling-gated and TestStoreShapeCompileDiscipline pins native
  compile + parity + the one pre-existing drop-refusal unchanged.
  CtxShapes rides CheckState (Begin-reset, Clone-copied; the lifecycle
  gate's mutateContainers extended for pointer-keyed maps). One engine
  fix surfaced by the work: checkModeAssumeSig's no-compatible-sig
  fallback pass preferred ANY ReturnsFn-bearing sig — with the patrun
  `add` overload ranked first by specificity, a failing 2-arg `add`
  recovery assumed the 3-arg sig, corrupting the disjunct-rescue
  window and feeding ReturnsFns short args slices (a latent
  index-out-of-range class); the pass now requires an
  arity-SATISFIABLE window (engine.go), and the new ReturnsFns are
  len-guarded. fnmodel_equivalence.golden regenerated for the 7
  intentional +returnsfn annotations. Landing
  tests (lang/go/store_shape_test.go): the two-stores-no-join headline
  (flex + patrun), ctx layer isolation with runtime agreement, nested/
  aliased/cloned flex flows, 6 negatives (unshaped store keeps flat
  set-then-get, unseen keys stay dynamic(Any) never None, lambda
  poison, empty table), and TestStoreShapeObservationFree (a check
  pass full of ctx/flex/patrun writes leaves the REAL context store's
  Data, pointer, and stack depth untouched). Ratchets: FP 0, unflagged
  170, soundness 7 (identical rows), fuzz 104, frontier pin 220 → 195
  (patrun 17→1, flex 15→9, minilang 10→9). Residuals → stage 3/4:
  FlexList/patrun element ordering stays unkeyed (pop/shift/get-index
  rows), `node` does not thread shapes onto Parent=TMap carriers (the
  positionalMatch Data!=nil pattern rule), fn-param stores stay
  unshaped (the call-boundary carrier is fresh), ContextTypes retire
  blocked on those readers, and replace-on-write flow-sensitivity
  stays deferred (join-only keeps sandbox leaks monotone).
- **2026-07-04 — Phase 4.4 landed: G6 options-typo hardening via
  emit-family Options schemas; the GENERAL options-map lint is
  documented blocked-on-precision.** Live probe reproduced the symptom
  (`emit json {prety:true} {a:1}` checked clean under --strict and
  emitted compact JSON). The deciding precision finding: atom-spelled
  and string-spelled map keys are INDISTINGUISHABLE post-parse
  (`{a:1} cmp {'a':1}` → 0 — OrderedMap keys are plain strings), so
  the plan's "atom-keyed map literal" signal does not exist at the
  dispatch seam; every concrete map argument would qualify and the
  advisory would fire on legitimate data maps (merge/inject/make
  inputs) — far below the ~100% advisory bar. Landed the sanctioned
  fallback instead: per-kind Options schemas on the built-in
  emitters' opts slot (native.EmitOptsSchema — json/jsonic/yaml
  {pretty,indent}, csv {separation}, xml {pretty}, tsv/toml/ini
  EMPTY; indent typed Number, mirroring optIndent's int64/float64
  tolerance; emit_auto carries the natural-kind union), riding
  Signature.Patterns exactly like `convert`'s convertOptsPattern —
  a typo'd or wrong-typed key is now a HARD dispatch rejection at
  check AND run time, while a dynamic opts map still matches
  (Options vs non-concrete Map keeps the schema). Host-
  (RegisterHostEmitter) and boru-registered (EmitLang.register)
  emitters keep a plain-Map opts slot — their key sets are their
  own. The data slot is untouched (`emit json {prety:true}` still
  emits `{"prety":true}`). The reserved `options_key_unchecked`
  severity-table entry (Info, registry.go) documents the
  blocked-on-precision state. Four corpus smoke rows
  (corpus-modules.tsv:47-50) had encoded the hole itself — raw
  exported-emitter calls passing junk `{a:1,b:2}` opts and relying
  on silent ignoring; updated to the valid `{}` opts (intent
  preserved), restoring every ratchet number byte-identical. Landing
  tests: TestEmitOptsSchemaTypoRejected (8 rejections incl. auto /
  tsv-any-key / wrong value types; 4 accepteds incl. data-slot and
  registered-emitter arbitrary keys), TestEmitHostEmitterOptsUnchecked.
- **2026-07-04 — Phase 4.5 landed: emit/check decoupling — CheckState
  holds the EmitRecorder interface; the checker runs with no
  *EmitState knowledge.** The check side's real surface (grepping
  Check.Emit usages outside the emit cluster) is ~50 recorder
  operations — beyond the plan's ~25 bar, but a record*-only subset
  would have forced KEEPING the concrete field, so the full coherent
  interface landed in one pass: EmitRecorder
  (eng/go/emit_recorder.go) with the exported record / branch / loop
  / fn-unit / lifecycle families plus an unexported eng-only tail
  (unexported members mean no out-of-package implementations, by
  design). CheckState.Emit is now EmitRecorder; product reads go
  through the never-nil CheckState.Recorder() accessor; Begin resets
  to the inactive no-op recorder (inactiveEmit — every method
  mirrors the historical nil-receiver answer), so a PLAIN check pass
  touches no EmitState code at all. Recorder internals that had
  leaked into check-side files became narrow methods: Armed() (the
  historical `Emit != nil` probe — bare nil-guards map 1:1, no
  site-by-site behavior audit needed), bindRegistry, topFrameOnly,
  suspendedNow, Sites, zeroOutProduced, alreadyProduced,
  unitVariadic. The emit cluster (emit.go / lower.go /
  callable_words.go) owns the concrete type: compileClosureBody,
  tryRecordClosure and recordClosureDispatch type-assert (declining
  exactly as the nil field did), and IsolateEmit's fresh-state
  construction moved behind emit.go's newIsolatedEmit. user_poly.go
  now takes EmitRecorder (the fnRecs variadic read → unitVariadic).
  The CheckState lifecycle gate grew a third bucket
  (reset-to-canonical-non-zero) for Emit. Landing tests:
  TestPlainCheckUsesInactiveRecorder (a compile-free pass never arms
  an EmitState) and TestPlainCheckRunsAgainstRecorderInterface (a
  counting NON-EmitState fake runs the pass with identical
  diagnostics — interface-only coupling proven). ZERO behavior
  change: every ratchet / census / frontier / differential number
  byte-identical (FP 0/3313, unflagged 170/485, soundness 7,
  frontier 195 pin 195, census 3547 compiled / 250 check-errors /
  78 refused / 1 islanded, differential 3299/0, fallback 3522/0,
  ceilings 3/0/66).
- **2026-07-04 — Phase 4 CLOSED (4.4 + 4.5) and Phase 5 executed
  (5.1 re-census, 5.2 check-by-default; 5.3 voxgig stays
  blocked-external).** 4.4: boru:emitlang gains a declared Options
  schema — the {prety:true} typo is now a hard dispatch rejection
  (the G6 probe shape). 4.5: the checker talks to the compiler
  through the narrow recorder interface (eng/go/emit_recorder.go);
  EmitState implements it; behavior-identical by gates. 5.2: boru run
  pre-flights by DEFAULT — quiet gate (diagnostics only when
  aborting), --check upgrades to verbose, --no-check / BORU_NO_CHECK
  opt out; TestCheckByDefault pins all five behaviors. Also landed
  from PR #225 review (P1s, both probe-confirmed real): the
  embedded-enclosing-binding literal refusal (fresh spine over
  shared member is unmodelable — embedsEnclosingCompound) and the
  class-instance fn-field auto-dispatch refusal (make-time risk
  tracking by instance ID + zeroArgFnOut backstop), pinned by
  TestPR225P1Refusals with fallback-parity. All landed through the
  full battery (VERIFY PASSED, make test 0, differential 0
  divergences) and pushed as 581f143 / 8651c5f / 40a92b7; both
  review threads answered.
- **2026-07-04 — maintainer direction: compiled-mode default reverted
  to OPT-IN.** ResolveCompileMode returns CompileOff bare again; the
  flag family now mirrors the checker exactly (--compile /
  --force-compile / --no-compile + env twins, --no-compile winning
  over all). P7-ENDGAME.10.md carries the addendum; the frontier gate
  and perf baseline stand.
- **2026-07-04 — Phase 7 executed; program closed.** Compiled mode
  flipped to default (ResolveCompileMode → CompileTry bare;
  BORU_NO_COMPILE the kill switch, per the rollout contract's reserved
  Stage-7 language); the coverage frontier re-armed as a GATE at the
  documented-tier floor (refusalGate 11 / islandGate 1 in
  TestCompiledCoverage, tier decomposition in-comment — a ratchet over
  11 open defects, not a licence for them); perf/alloc
  baseline confirmed standing (verify-bytecode alloc ceilings green
  at every landing). The unbounded fallback deletion stays gated on
  the tiers reaching 0, recorded in P7-ENDGAME.10.md with the full
  achievement table (refusals 151 → 11, error rows 83 → 3,
  Any-frontier 381 → 195, miscompiles 0, check- and
  compiled-by-default).
- **2026-07-03 — Phase 1.3 (M3) verified sound, no change needed.**
  (a) `checkParamContract` already routes through `sigTypeMatches` —
  the interpreter's own runtime param match (deliberately NOT `v.Is`,
  which over-raises on Options/structural params) — plus
  `OpenUnifyMap`/`Unify` for ParamPatterns; the flagged asymmetry was
  closed by the param-guard fix. (b) RET enforcement uses `v.Is` (the
  membership rule); tail marking's static `ConformsTo` in
  `tailCompatibleReturns` is sound because callee→caller conformance
  is checked in the subset direction and nested predicate refines are
  CONJUNCTIVE — probe: `3 is (Big lt 5)` → false (Big's bound still
  applies) — so a lattice child's membership is always a subset of
  its ancestor's, and the eliminated caller check is implied by the
  callee's own RET check.
