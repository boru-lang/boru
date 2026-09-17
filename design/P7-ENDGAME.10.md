# P7 Endgame — record (.10)

Status: **landed** (2026-07-04, branch
`claude/boru-local-reasoning-design-rb7elj`). The closing record of the
checker/bytecode completion program executed against
[`CHECKER-BYTECODE-COMPLETION-PLAN.0.md`](CHECKER-BYTECODE-COMPLETION-PLAN.0.md)
(whose execution log carries the per-landing detail) and the Phase-6
staged sweep specified by
[`STAGE3-INLINING-DESIGN-ROUND.0.md`](STAGE3-INLINING-DESIGN-ROUND.0.md).

## What the program achieved (start → finish, one session-week)

| Metric | Start (main @ 060157b) | Finish |
|---|---:|---:|
| Whole-program refusals | 151 | **11** |
| Native rows / islands | 3,475 / 1 | **3,613 / 1** (99.7 % of compilable) |
| Error-row fallbacks | 83 | **3** (each an open defect; see Addendum 5) |
| Tier-1 / tier-2 partitions | 0 / 1 | **0 / 0** |
| Known miscompiles | mechanisms A + E open | **0** |
| Checker Any-frontier | 381 rows / 11.52 % | **195 / 5.90 %** (ceiling 12 → 7) |
| `missing_returns` | 87 | **0** (registry-wide gate, empty allowlist) |
| Soundness pins | 8 | **7** (each with a rationale) |
| Check adoption | opt-in `--check` | **check-by-default** (quiet gate) |
| Compiled-mode adoption | opt-in `--compile` | **compiled-by-default** (this record) |

Throughout: false positives 0/3313, differential and
compile-or-fallback **0 divergences** at every landing, `VERIFY
PASSED` (including `-race`, combinations, property-fuzz, and
`borudebug` lanes) and full `make test` before every commit.

> **Addendum (2026-07-04, maintainer direction):** the default flip in
> action 1 was landed and then **reverted to opt-in** — compiled mode
> is OFF by default, controlled by the checker-style flag family
> `--compile` / `--force-compile` / `--no-compile` (+ `BORU_COMPILE` /
> `BORU_FORCE_COMPILE` / `BORU_NO_COMPILE`, the `--no` twin winning over
> everything). The safety case below still holds and the flip remains
> a one-line change whenever the maintainer chooses to take it; the
> other two endgame actions (the gated frontier, the standing
> perf/alloc baseline) are unaffected.

> **Addendum 2 (2026-07-08, maintainer direction — the I-wave
> close-out):** the flip is **taken**: compiled mode is ON by default.
> The I1–I5 improvement wave first shrank the residue table below —
> I1 (unnamed-param frame flow: `args` stack in the VM, unnamed args
> pushed through the frame) retired the unnamed fn-value param guard
> for the residual-flow rows; I2 (runtime re-match for recovered
> dispatch: PolyRef.NOut arity claims, flex StoreShapeInfo narrowing
> in the compile pass, generic-slot reachability) closed the G5 flex
> tier; I3 (guarded 0-arg method landings recorded by the check pass,
> guard-owned declines) closed the container auto-dispatch tier; I4
> (OpLookupDynScope / OpBindDynScope over the runtime def stack,
> reachability-gated by FnBinders + FnCallGraph) closed the M6
> dynamic-scope tier; I5 (CompiledFn.Reg — every compiled unit
> dispatches against its OWNING registry, Registry.Invoker threads the
> calling registry, escaped lambdas keep module scope) closed three of
> the four repl served-handler rows and retired bodyConstructsFn.
> Refusals: **21 → 9** on the 5,941-row corpus (5,617 compiled, **0
> islands**, differential + property fuzz clean at every step); the
> remaining 9 are the three non-definite error rows (still refused,
> each an open defect), five
> module fn-value-boundary rows (dynamic apply inside fn units), and
> one fn-value-reaches-`drop` row — see the tier ledger in
> `compiled_coverage_test.go` (`refusalGate = 9`). With that,
> `ResolveCompileMode` returns `CompileTry` by default (`boru run` /
> `boru do`), `EvalOptions` drives TRY, and `boru build` bakes TRY with
> a new `--no-compile` opt-out (a frozen binary has no env knobs).
> The `--no` twin and the env kill switch win over everything,
> unchanged. **The whole-program fallback path stays for now**
> (maintainer decision, same rationale as below): it is scaffolding
> that absorbs the 9 documented-tier refusals so the user still gets
> an answer. Those 9 refusals are defects against the compile
> contract — each an unimplemented or unproven case, owed a fix and
> tracked to closure — and the path that absorbs them is containment,
> never a reason a refusal is acceptable.

> **Addendum 3 (2026-07-08, I7 — dynamic apply inside fn units):** the
> module fn-value-boundary tier (module-fnvalue-boundary 24/25/31/32/33)
> is closed; refusals **9 → 4** (5,622 of 5,941 rows compiled, 0
> islands, differential + property fuzz clean). Two mechanisms:
> **OpCallDynFrame** replays a fn body's whole end-of-body residual
> through a nested interpreter run (`RunResolved` with the unnamed-param
> re-pushes as the resolved prefix — the sub-engine twin of the frame's
> ArgSpan), so an unapplied fn value dispatches by execFnDefLiteral's
> own runtime rule, including below-window stack collection (gated to
> bodies whose apply is the LAST statement — replayIsBodyTail — since
> the replay fires at the RET and a later effectful statement would
> otherwise reorder ahead of the callee's effects); the RET
> then takes the **RetReplay** discipline (the CallBoru trim-only
> return path for foreign-registry fns, count-exact-or-defer so a
> runtime-variable residual is deferred to the containment path rather
> than returned wrong — a deferral that is an unimplemented case, not
> a resolution). And the per-iteration loop apply extends to Function
> params (`applyFn` re-push, apply-first / apply-last seated by source
> order), with an escaping break/continue
> crossing the island boundary via the registry FlowCtrl contract
> (`Engine.flowUnwind` tears down the island's live frames;
> `escapedFlow` translates the signal to the compiled cross-frame
> unwind). The 4 remaining refusals are the three non-definite
> error rows and the fn-value-reaches-`drop` row (`refusalGate = 4`) —
> four open defects, not four sanctioned outcomes.

> **Addendum 4 (2026-07-13, staleness notice):** the corpus has grown again
> (5,941 → 6,267 rows) and the residue table below is one generation stale:
> the carrier-parity, module-fnvalue-boundary, G5 flex, M6 dynamic-scope, and
> compute-frontier-island rows have all since compiled, and the live refusal
> set is **9 rows, all one tier** — the "unmatched dispatch recovered"
> soundness rows pinned row-exact in
> `test/go/langspec/compiled_refusals_test.go` (`knownRefusals`), which is the
> authoritative per-row ledger. Gates ratcheted to match (`refusalGate = 9`,
> `computeRefusalCeiling = 0`). The completion program for the remaining
> distance — these 9 rows, the off-corpus leaves (L-DO/L-EACH/L-JOIN/L-NP),
> the L-DUP fallback-soundness bug, the never-compiled entry points, and the
> Stage-J fallback deletion — is
> [`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md`](RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md).

> **Addendum 5 (2026-09-16, maintainer direction — doctrine
> correction):** this record was written in places as though a refusal
> were a sanctioned second outcome and the interpreter a fallback the
> design leans on. That framing is wrong and has been corrected below.
> The interpreter is **not** a fallback for the compiler and is not
> allowed to be one. Failure to compile is a **failure**: done is a
> language that compiles as a developer expects — all valid code
> compiles, no exceptions. Every refusal recorded here is therefore a
> **defect** — an unimplemented or unproven case, owed a fix and
> tracked to closure — not an acceptable worst case. The measurements,
> tier ledgers, and landing history below are untouched; only the
> policy statements they were wrapped in are restated. The runtime
> path that **silently** re-runs a refused program on the interpreter is
> described as what it is: scaffolding that absorbs a known defect so
> the user still gets an answer. The silence is the worst of it —
> nothing in the run says the compile was refused, so the failure hides
> itself — and it is named here to indict that path, pending the
> deletion tracked in
> [`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md`](RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md).

## The three endgame actions

1. **Compiled mode is the default.** `ResolveCompileMode` returns
   `CompileTry` with no flags — the Stage-7 flip the rollout contract
   reserved (`BORU_NO_COMPILE` is the kill switch; `--force-compile`
   surfaces a refusal as the loud error it is, instead of silently
   routing the program to the interpreter; the legacy `--compile` /
   `BORU_COMPILE` opt-ins are accepted no-ops). Safety case: the
   differential gates hold byte-identical values *and* error taxonomy
   across the 3,875-row corpus, the combination matrix, and the fuzz
   lanes. The flip does not rest on the fallback path, and that path
   is no part of the safety case: it absorbs the refusals still open
   against the compile contract, each of them a defect owed a fix and
   each absorbed without a word to the user.
2. **The coverage frontier is gated again.** The growth-phase
   informational policy on refusals/islands is over:
   `TestCompiledCoverage` now gates at the documented-tier floor
   (`refusalGate = 11`, `islandGate = 1`). A new refusal is a defect:
   it must be fixed, or — until the fix lands — recorded row-exact in
   a named tier and tracked to closure. Never drift in silently, and
   never read a tier entry as a resolution. The gate only moves down
   as tiers close; up only with a new named tier recorded here.
3. **Perf/alloc baseline stands as gated.** The alloc ceilings inside
   `make verify-bytecode` ran green at every landing; the compiled
   default makes them the de-facto runtime baseline. No re-baseline
   was needed — no landing regressed them.

## The remaining 11 + 1, by tier (the honest residue)

The table is the snapshot as this record closed, kept as it was
written. Read it under the corrected doctrine (Addendum 5): every row
is a refusal, and every refusal is a defect — an unimplemented or
unproven case, owed a fix and tracked to closure. A tier name says
where the fix is tracked; it never exempts a row, and a feasibility
proof explains a defect rather than sanctioning it. What has closed
since is recorded in Addenda 2–4 above, and the live per-row ledger is
`knownRefusals` in `test/go/langspec/compiled_refusals_test.go` — not
this snapshot.

| Tier | Rows | Disposition |
|---|---|---|
| Container auto-dispatch guard (miscompile-E) | module-log 72/73, module-rand 14/15 | **Open defect.** The guard refuses rather than miscompiles, but the refusal is owed a fix: compiling them requires modeling the interpreter's landing-auto-dispatch semantics. The guard must never be weakened for coverage — the semantics get modelled, or the rows stay open on the books. |
| G5 flex path-shape typing | flex 88/95 | Named non-Phase-6 track (StoreShapeInfo stage 3+). |
| M6 dynamic-scope tier | recursion 71/72 | Maintainer tiering decision — dynamic-scope frames are the one semantics the unit model cannot honestly claim. |
| Non-definite error rows (still refused) | convert-ideal 30, forward-barrier 80, word-splice 115 | The entire error allowlist: each has a written feasibility proof that the dispatch may succeed at runtime. The proof says why the row is not yet compilable — it is an unproven case, not a sanctioned outcome, and the row stays open until it compiles. |
| Compute-frontier island | error.tsv 25 | The single `OpFallback` span — one unproven region inside an otherwise compiled unit, contained by a fallback island and owed a real lowering. |
| Unnamed fn-value param guard (miscompile-E) | module-fnvalue-boundary rows (6) | **Open defect**, guarded rather than miscompiled (guard added July 2026 with the execFnDefSig same-registry splice flip; re-justified under the arguments-are-inert unification — design/ARG-SEMANTICS-UNIFICATION.0.md §7). Originally the VALUE twin of the container auto-dispatch tier (the interpreter auto-fired the value where the unit stranded it). The unification removed the auto-fire, but the unit model still binds unnamed params to slots without pushing them onto the operand stack, so an unnamed arg FLOWING to the residual (`args.N` reads, the return-discipline trim) still diverges — the guard stays until unnamed-param frame flow is modelled in unit lowering. Guard in `carrier.go::runFnBodyOnce`; refusal gate 11 → 17; compute-gap ceiling 9 → 13. The module-fn EMPTY-body shape remains outside the guard's reach and outside the corpus (documented in both design notes). |

**The unbounded whole-program fallback in `RunCompiled` stays for
now** — as scaffolding, not as design. `RunCompiled` silently re-runs
a refused program on the interpreter (the refusal itself recorded
through `MarkUncompilable`, sub-program regions through the
fallback-island opcode), so that the 11 open documented-tier defects
still produce an answer for the user. The silence is the worst of it:
a compile failure that hides itself is still a failure, and one
nobody is told about. That is containment for known defects, not a
second branch of the contract — the contract is that the program
compiles. Its deletion (`Compile` + `RunProgram`, refusal = compile
error) is gated on the tiers above reaching zero, and is the one
remaining P7 line item, owned by the tier owners — not a hidden
default.

## Out of scope, recorded

The voxgig force-compile sweep (M5) remains blocked-external — the
client corpus is not in this environment; the 35/48 figure from PR
#224 is the last measured value and the sweep re-run is owed at the
next environment that carries it. Bytecode serialization for
`boru build` remains explicitly unproposed.
