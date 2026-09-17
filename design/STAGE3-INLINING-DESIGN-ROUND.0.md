# Stage 3 / Phase 6 design round — user-fn-call compilation with concrete-argument propagation

Status: **design round, implementation-ready; no code.** This is the Phase 6
"design round first" deliverable required by
`design/CHECKER-BYTECODE-COMPLETION-PLAN.0.md` §Phase 6. Written against the
live tree at `f4c56a1` (branch `claude/boru-local-reasoning-design-rb7elj`,
2026-07-04), with every load-bearing claim re-verified against committed code,
committed tests, and a fresh 3,875-row `--force-compile` sweep — not against
the June design docs, several of which are now materially stale (§1).

**Doctrine correction — supersedes the original framing everywhere in this
doc.** The interpreter is **not** a fallback for the compiler, and is not
allowed to be one. Failure to compile is a *failure*; done is a language
that compiles as a developer expects — all valid code compiles, no
exceptions. Every one of the 78 refusals counted below is therefore a
**defect**: an unimplemented or unproven case, owed a fix and tracked to
closure, never a sanctioned design outcome. Under `--compile` the runtime
**silently** re-runs a refused program on the interpreter (`RunCompiled`),
so the row still answers — and the silence is the worst of it: a failure
to compile that hides itself. That re-run is **scaffolding** absorbing
known defects; where this doc describes it, or a guard that refuses, it
describes machinery around an open hole, never a fallback the design leans
on and never a reason a refusal is acceptable. Wherever the text below
reads otherwise — "sound" refusals, rows that "legitimately" refuse,
permanent refusal tiers — it is corrected in place: a tier is a docket for
defects not yet scheduled, not an approved terminus.

> **Point-in-time snapshot — the refusal accounting below is superseded.**
> The "fn value read from a container auto-dispatches" bucket (4 rows) and
> the "container auto-dispatch — miscompile-E guard" line in the
> stage table have since CLOSED: the arity-0 landing model shipped
> (shapedMethodApplyWindow's all-0-arg path for shaped members;
> tryMemberFnArrivalDispatch's empty-window claim for pinpointed plain
> members — the break-2 closure, `design/FN-VALUE-OPEN-WORK.0.md` §4),
> the read guards skip what the landing models own, and all four rows
> compile. Read the tables as the 2026-07-04 baseline, not live state.

Read first: `boru-bytecode-stage3-inlining-plan.0.md` (the June build record),
`module-fn-checkstate-ownership.7.md` (the ownership project + its closing
correction), `MODULE-FN-PARAM-SLOT-COMPILATION.0.md` (**the doc that actually
describes the live architecture** — §9's fn-dispatch unification), and
`boru-bytecode-next-stages.0.md` §C/§F/§G.

---

## 0. Live state (measured this session)

Method: the committed census at `f4c56a1` (`test/go/langspec/COMPILED_STATUS.md`)
plus a full per-row `boru run -force-compile` sweep of all 3,875 spec value rows
extracted from `f4c56a1:lang/spec/*.tsv`. The sweep binary was built from the
working tree (dirty with the concurrent Phase 4.4/4.5 work — options-schema +
recorder-interface decoupling, both coverage-neutral by design); its refusal
total and every bucket count **byte-match the committed census**, so the
per-row inventory below is treated as the `f4c56a1` ground truth. Re-confirm on
a clean tree at Stage 0 (§6) before moving any ceiling.

```
3,875 value rows — 3,546 native + 1 island, 78 refused, 250 check-error
computeRefusalCeiling 66; tier-1 0; tier-2 0; error-row allowlist 13
```

The 78 refusals, by row (sweep output, grouped by normalised reason):

| bucket (census) | n | rows |
| --- | ---: | --- |
| operand provenance | 22 | module-parse:14,18,37–44 · path-modifier:17,18,19,20,23,28,52,53,55 · module-minilang:320 · recursion:71 · recursion:72 |
| dispatch recovery (best guess) | 14 | open-words:32,83,84,90,100 (`add`) · apply:37,38 · flex:88,95 (`drop`) · convert-ideal:30 · forward-barrier:80 (`each`) · generics-sugar:37 · generics:60 · word-splice:115 (`f`) |
| function value reaches word (Stage 3) | 11 | module-minilang:306–315 (`is`) · corpus-core:134 (`walk`) |
| user fn call (Stage 3) | 10 | open-words:72,77,78,82,85,86,87,98,99 · micron:200 — **all ten are a module-defined `add`** |
| fn-value-call boundary | 7 | module-log:53,54,56,77,78,79,80 |
| function-valued operand (Stage 3) | 5 | recursion:90,91,92 (`apply`) · module-log:62,83 (`log-register`) |
| fn value read from a container auto-dispatches (Stage 3) | 4 | module-log:72,73 · module-rand:14,15 |
| unconsumed fn-value carrier (closure render) | 3 | path-modifier:24,25,54 |
| dynamic input | 1 | module-parse:15 |
| paren-bounded fn-value application | 1 | module-log:55 |

The one island is an `OpFallback` span in the compute-frontier partition (row
not named by the census; identify at Stage 0).

**The headline measurement: none of the 78 is a "module-fn body does not
compile" row.** `module-test.tsv:38` — the reproducer Phase 6 names — compiles
and runs byte-identical **natively** today (probe: `-force-compile` clean,
`{total:2 passed:2 failed:0}`; pinned by
`lang/go/bytecode_borutest_cascade_test.go::TestBoruTestCascade_FlatPassing`,
which asserts **no FALLBACK island**, and by the recursive sub-spec parity
tests in the same file). `module-rand.tsv:38`, `module-parselang.tsv:23`,
`macro.tsv:45` (as an expansion-depth error), `def-node-binding.tsv:54`,
`patrun.tsv:40` and `fn-value.tsv:19` all pass `-force-compile` as well
(probed individually this session).

## 1. Corrections to stale docs (read before believing anything older)

1. **`CHECKER-BYTECODE-COMPLETION-PLAN.0.md` Part 1's "hard wall" framing is
   stale in its example.** "Sound module-body / cross-registry compilation
   (`module-test.tsv:38` and everything shaped like it) … two attempts already
   reverted" was written from the June ownership docs. The wall was in fact
   **broken between those docs and now**, on this branch's own ancestry, by a
   third architecture neither reverted attempt used (§2.3). The named
   reproducer compiles natively. What Phase 6 inherits is the 78-row frontier
   above, which is a **different decomposition** (§6, §7).
2. **`module-fn-checkstate-ownership.7.md`'s closing correction ("the only way
   … is to INLINE run-spec at the call site where `s` IS the concrete
   literal") is falsified by the landed system.** run-spec compiles as a
   *generalised, shared* param-slot unit; recursion terminates **at run time,
   data-driven inside the unit** (`for (subs size)` iterates zero times over
   the real `subs`), so check-time concrete-argument propagation was never
   required for this row. Check-time zero-count `for` pruning ALSO landed
   (`TestRunSpecHarnessCompiles` positives) but is an optimisation, not the
   load-bearing mechanism.
3. **`boru-bytecode-stage3-inlining-plan.0.md`'s status header is real, not
   aspirational** — its "refusals 5 → 0" series (module-parselang:23,
   module-rand:38, module-test:38, macro:45, def-node-binding:54) landed as
   commits `6874aa9 → e753714` (June 2026), all ancestors of HEAD. Its
   *analysis* sections describing why inlining is needed are superseded by
   the unification (§2.3); its warnings about value-keyed specialisation
   (§3.2 below) remain valid.
4. **`MODULE-FN-PARAM-SLOT-COMPILATION.0.md` §9's regression note ("run-spec
   no longer compiles NATIVELY") is itself stale**: the four cascade kernel
   fixes recorded in `bytecode_borutest_cascade_test.go` (FnSummaries
   delete-key, poly registry threading, **genArgs-keyed fn units**, 0-return
   no-provenance residual drop) restored native compilation of the recursive
   harness after the unification. The comment inside
   `TestRunSpecHarnessCompiles` ("FALLBACK ALLOWED") predates that and the
   sibling cascade test's no-island assertion is the current contract.
5. **`boru-bytecode-next-stages.0.md` Stage C's design ("cross-registry
   EmitState sharing" as future work) is DONE** — `shareCheckState`
   (`f4c56a1:eng/go/engine.go:4705`) plus the unification. Stage C's row list
   is cleared. Stage D (sub-registry poly) partially landed (PolyRef.Reg,
   `f4c56a1:eng/go/bytecode.go:337-345`). Stage F (dynamic scope,
   recursion:72) and parts of Stage G (fn-value application) are the live
   remainder and reappear in §6.

## 2. Root-cause reconciliation — what was tried, what failed, what landed

Phase 6's mandate: explain the two reverted attempts and how this design avoids
their failure modes.

### 2.1 Reverted attempt 1 — capture-threaded islands (built twice, reverted twice)

`design/boru-bytecode-capture-threaded-islands.0.md`. Thread a code body's free
outer-locals into an `OpFallback` island as named `def` bindings
(`FallbackSpan.Captures` + install/teardown in `vm.go::runFallback`). Both
prototypes were **correct and gate-clean** and were still reverted, for a
strategic reason that this design adopts as a rule:

- it **moved zero corpus rows** (the only candidate row additionally needed
  sound consumer-quoted-key analysis), and
- even in success it converts refused → **islanded**, which *raises* the
  island count. The P7 finish line is islands → 0; an island is a step away
  from native. A relaxation probed along the way (admit any unbound free word)
  was proven **unsound** by the fuzzer (seed 1 iter 748 — a forward self-ref
  resolves differently in the island sub-engine).

**Rule carried forward: no stage below may convert a refusal into an island.**
Every stage's exit is refused → *native* (or refused → compiled trap with
byte-identical taxonomy), and the island count is pinned ≤ 1 throughout.

### 2.2 Reverted attempt 2 — the checkstate-ownership Step-3 reroute

`module-fn-checkstate-ownership.7.md` Step 3 (`tryModuleFnUnit`): route the
check-mode module-preamble-fn dispatch through its registered `ReturnsFn` (unit
compile) **while the inline `CallBoru` path still existed for everything else**.
Measured results:

- did **not** clear module-test:38 (the binding constraint was one level
  deeper — the closure/capture analysis, then deeper again: check-time
  termination of the recursion), and
- **regressed** coverage (refusals 9 → 10, dispatch-recovery 3 → 4): other
  module rows' `CallBoru`-recorded carriers differed from the `ReturnsFn`
  residuals, so a partial reroute perturbed rows it never targeted.

The `.7` closing corrections then mispinned the root cause twice
(closure-capture provenance; then get-on-concrete-Map folding — the get-fold
landed as an independent precision win but was **verified inert** for the
row). The durable lesson is not either root cause; it is:

**A dispatch seam may not have two recording paths.** Any change that makes
*some* dispatches of a family record differently from the rest of that family
shifts carrier residuals corpus-wide and fails the coverage gate even when the
differential stays green. Fixes must either convert a whole seam or be
strictly additive behind a probe (forked emit state, commit-on-clean).

Also permanently on the books from `.0`–`.6`: sharing a whole `CheckState`
across registries without key discrimination cascades `no_signature` (29/39
errors, `.0` §4 — fixed by `FnAnalysisKey` scopeIDs, `registry.go:144,703`);
running module bodies **concretely** during the compile pass leaks side
effects and bakes folded test results (`.0` §3 — the `{passed:1 failed:1}`
divergence); and the dynamic-help example eval's diagnostics were load-bearing
until the hermetic-eval + construction-check decouple landed (`.7` steps 1–4).

### 2.3 What actually landed — the third architecture

The wall fell to the **fn-dispatch unification** (`c5ff7bd`, 2026-06-28,
`MODULE-FN-PARAM-SLOT-COMPILATION.0.md` §9) plus the cascade kernel fixes:

1. **One dispatch path.** Every function — named, bare-word, trivial module
   wrapper, real module-fn body — dispatches through `execMatch`
   (`engine.go:4211-4245`: "ONE dispatch path, no exceptions"), so every body
   is compiled the same way: a **param-slot unit** via the matched sig's
   `ReturnsFn` = `buildFnBodyReturnsFn` (`core_helpers.go:727`).
2. **Cross-registry recording.** `shareCheckState` (`engine.go:4705-4723`)
   transiently points the module sub-registry's `Check` (a `*CheckState`,
   `registry.go:80-94`) at the main pass for the dispatch, so the body's unit
   records into the MAIN program while **names still resolve in the module's
   own `Defs`/`Types`** — the exact split-by-concern `.0` §5 demanded.
   `FnAnalysisKey(r.AnalysisScopeID(), …)` keeps memo/inflight keys disjoint.
3. **Generalised, genArgs-keyed units.** The unit compiles against
   `NewCarrier(argType)` generalisations (`core_helpers.go:824-871`), keyed on
   the generalised args (`:871`) — so a **recursive call with different
   concrete args reuses the in-flight unit** (the fnUnits hit *is* the
   recursion guard), and record-schema params ride `recordSchemaCarrier`
   (`core_helpers.go:384-…`) so field reads keep their declared types.
   Recursion terminates in the **VM**, over real data, not in the checker.
4. **The cascade kernel fixes** (pinned by
   `lang/go/bytecode_borutest_cascade_test.go`): stale-summary deletion before
   an armed re-record (`core_helpers.go:915`), `PolyRef.Reg` sub-registry
   threading, genArgs-keyed units (a recursive call previously allocated a
   second, empty unit → silent 0-case miscompile), and the 0-return
   no-provenance residual drop.

This is why "inlining" as the June docs conceived it (analyse the body under
the caller's concrete values so the checker can terminate the recursion) never
landed and is **not** the design here: the generalised-unit model solved the
same rows with strictly less blast radius, and its memo key already gives
per-instantiation units for generics ("monomorphization for free",
`core_helpers.go:744-753`).

### 2.4 What exactly still breaks (the residual mechanics)

Three mechanical gaps produce all 78 refusals plus the known off-corpus walls:

- **(a) The merged-word seam lacks `shareCheckState`.** An *open word* — a
  module-defined `add` merged into the importer's dispatch table
  (`aggregateDispatch`) — dispatches as a bare word through `execMatch` on the
  main engine, and its `ReturnsFn` closure runs `es := r.Check.Emit` against
  the **module registry captured at `InstallFnDef` time**. Nothing shares the
  check state on this path (only the dot-access `execFnDefLiteral` sites do,
  `engine.go:4240,4791`), so `es` is inactive, `StartFnCompile` declines,
  `fnUnit = -1`, and `recordCallRefusal` fires "user fn call add (Stage 3)"
  (`emit.go:2656`). This is precisely the mechanism the comment at
  `engine.go:4229` names — applied to a seam the unification did not reach.
  It accounts for the entire user-fn-call bucket (10) and the 5 open-words
  dispatch-recovery rows (the ERROR variants additionally need §6/M4).
- **(b) Fn VALUES still have no general application/read model.** ~43 rows: a
  fn value read from a container and applied through path modifiers
  (path-modifier 12), instance-method dispatch over a module-made receiver
  (module-log 12, module-rand 2), a fn value as a word operand
  (module-minilang `is` 10, corpus-core:134 `walk`), `apply` over a param fn
  (recursion:90-92), fn-valued registration operands (module-log:62,83),
  DSL results whose producers record nothing (module-parse 10 + 
  module-minilang:320). These are Stage G/D shapes riding `OpCallDynamic` /
  `OpCallNativePoly` machinery, **not** body-compilation gaps.
- **(c) Carrier-poisoned bodies under recursion (off-corpus).** The radix-msd
  4-layer cascade (`MODULE-FN-PARAM-SLOT-COMPILATION.0.md` §16): an Array
  carrier's element reads Any → `add` widens to Scalar → the genArgs
  generalisation does not narrow a widened arg back to the declared param →
  and the attempted narrowing exposes a **recursion-bail provenance gap** (the
  in-flight bail returns declared-return carriers that were never
  `setProduced`, so the enclosing branch-merge residual has no producedBy and
  the unit refuses "body result of unknown provenance"). This is the one place
  where **concrete/narrowed-argument propagation is still a live design
  question** — and it is a *unit-refinement* question, not a splice question.

## 3. The inlining-boundary decision

**Decision: per-call-shape UNITS — the live model — extended, never literal
token splice.** Three tiers, strictly ordered; each lower tier is reached only
when the tier above refuses, via probe (`forkForProbe`, `emit.go:594-620`) so a
declined tier leaves the real state untouched:

1. **Tier U (live, default): generalised shared unit**, genArgs-keyed
   (`core_helpers.go:871`), `CALL_USER` at every call site, VM-side param
   guards (`SetUnitParamTypes`) and RET checks. Generics already get
   per-instantiation units through the same key. Recursion = fnUnits hit.
2. **Tier N (new, Phase 6 M5a): declared-param narrowing of genArgs** — mirror
   `narrowArgsToParams` (`core_helpers.go:471-…`) inside the genArgs loop,
   gated `genSpec == nil` (narrowing a generic type VARIABLE breaks
   instantiation — pinned by TestEmitGenericInstantiation). Verified sound in
   isolation (verify-bytecode + crossdiff green when probed in §16) but lands
   only **together with** the recursion-bail provenance fix (M5b), because
   alone it trades refusal reasons without compiling anything.
3. **Tier S (new, escape hatch, default OFF): call-site value-specialised
   unit.** Only when U and N both refuse AND every arg is a compile-time
   concrete inert value: compile a **second** unit against the concrete args,
   keyed `spec:<callsitePos>#<FnAnalysisKey(scopeID,name,concreteArgs,caps,body)>`
   — the key includes the concrete **values** (FnAnalysisKey already
   fingerprints arg values for concrete args via their type+payload identity;
   extend the key with a canonical value render if not), so a same-type
   different-value call NEVER reuses baked constants (the exact hazard
   `boru-bytecode-stage3-inlining-plan.0.md` feature (1) flagged). The unit is
   still a unit — param slots, param guards, RET checks — so soundness rides
   the same machinery; only const-folding inside the body sees real values.
   Bounded: probe-first, one specialisation per call site, recursion inside a
   specialised body immediately falls back to Tier U for the self-call (the
   generalised key), so no specialisation chains.

Why not token splice (record the body's events into the caller's stream):
- The tape interpreter splices; the **recorder must not** — a spliced body's
  events land in the caller's frame with the caller's residual discipline, and
  every one of `StartFnCompile.finish`'s contracts (declared-return counts,
  closure-vs-fn taxonomy, trailing-apply residuals, `emit.go:1908-2000`) would
  need a parallel spliced variant: two recording paths for one dispatch — the
  §2.2 anti-pattern verbatim.
- Splice destroys memoisation (every call site re-records the body) and
  step-budget behaviour (`StepCount` grows with call count, not body count),
  and turns recursion back into a check-time termination problem — the
  precise problem the unification dissolved.
- The measured demand is nil: zero of the 78 rows needs caller-context
  folding; the off-corpus demand (radix-msd) is answered by N (+ the layer-1
  Array-element-carrier work, which is Phase 4-family checker precision).

Per-call **summaries** (the other alternative named in the plan) already
exist — `FnSummaries` keyed identically to units — and are not the gap; the
gap is the merged-word seam (§2.4a) and fn-value machinery (§2.4b).

## 4. The provenance model for captured carriers

The live model, kept, with its invariants made explicit (this section is
normative for M-stage reviews):

- **A capture is a construction-time value snapshot with a stable ID.**
  `CapturedBinding{Name, Value}`; `AnalyseFnBody` binds the SAME Value
  (`carrier.go:3338`, `r.Defs.Push(cb.Name, cb.Value)`), and `StartFnCompile`
  registers the capture slot **by `cb.Value.ID`** after the param slots
  (`emit.go:1901-1904`, `u.capID`), so body references resolve to slots purely
  by ID — no name lookup at run time.
- **Capture operands re-resolve at every call site.** `RecordUserCall`
  resolves `rec.caps` through `resolveOperand` *in the caller's scope*
  (`emit.go:2143-2150`); an unreachable capture refuses ("capture … 
  unreachable at a call site") — never a guess. `enclosingIDs`
  (`snapshotCompoundBindingIDs`, `emit.go:939`) whitelists enclosing compound
  consts into the unit.
- **Module-scope state is captured by identity, with a written exactness
  argument.** `moduleScopeMutableCaptures` / `moduleScopeInstanceCarrier`
  (`callable_words.go:107,157`): a compiled body cannot rebind a module-scope
  name, so the value threaded at `OpPushClosure` equals every per-run lookup
  the interpreter makes. This is the model for any new cross-registry capture
  class: admit only what is provably rebind-free, and let `resolveOperand`
  refuse the rest.
- **Result identity ≠ operand identity.** When a recorded operation *changes*
  the value it binds (reparent, validate), the result must get a **fresh
  provenance ID registered against the new event** — `RecordTypedBind`'s
  remint (`emit.go:2478-2495`) is the precedent (miscompile B's mechanism,
  closed). Rule for M5b: any path that hands back a carrier standing for "the
  result of this call/branch" must either `setProduced` it or remint-and-seat
  it; a declared-return bail carrier that skips both is the radix-msd layer-4
  bug.
- **Aliasing is pointer-payload, not ID-graph.** `StoreShapeInfo`
  (`store_shape.go`) is the precedent: one heap payload shared by every
  carrier copy, mutation join-only/monotone, pointer-keyed side tables
  (`CtxShapes` keyed by `*StoreInstanceInfo`). Any Stage-3 need for "two
  carriers are the same runtime cell" uses this shape — never ID equality
  across registries. (IDs are process-global via `GenerateID`, so
  cross-registry ID *collision* is not a hazard; cross-registry ID
  *aliasing-as-identity* is, and pointer payloads avoid it.)

## 5. The cross-registry protocol

What a compiled Program may carry across the module seam, each with a landed
precedent; M-stages extend these, they do not invent new channels:

| channel | precedent | rule |
| --- | --- | --- |
| `*Type` pointers from sub-registry mints | `TypedBindSpec` + `RunTypedBind` (`typed_bind.go`); `CanonicalType` (`util.go:260`) | resolve by ID against the runtime TypeTable; on miss **the carried pointer IS the canonical node** (module mints never enter the main byID table; sub-registries adopt only the importing tree's ID *counter* — `typetable.go:258-269,325-333` — so pointers stay live and IDs never collide) |
| native handlers in sub-registries | `NativeRef.Reg` (`bytecode.go:361-365`), `PolyRef.Reg` (`:337-345`) | the op carries the owning `*Registry`; the VM re-matches/executes against it. A Program therefore pins its sub-registries by reference — fine for compile-and-run-in-session; a hard blocker for bytecode serialisation, which stays out of scope (plan Part 2) |
| user-fn bodies | fn units into the MAIN program via `shareCheckState` + `StartFnCompile` | the unit is registry-free at run time: params/captures are slots, natives are NativeRef.Reg, private helper fns are further units |
| module-private defs | **never cross by name.** | visible to the body only during check-mode analysis (resolution stays in `capturedReg.Defs` — `.0` §5's split); by lowering time each reference has become a const, a slot, a unit, or a NativeRef. The caller's namespace is untouched; there is no runtime def-stack for compiled code (the `args`/`__pa` refusal, `emit.go:2673`, stays) |
| module-scope mutable instances | capture-by-identity (§4) | |
| store/context state | `StoreShapeInfo` (check-side only) | compile-side behaviour unchanged; shapes are gated `!Compiling` |

**M1's protocol extension (the only new wiring in Phase 6's first stage):**
generalise `shareCheckState` from the `execFnDefLiteral` sites to **every
check-mode invocation of a foreign-registry sig's `ReturnsFn`** — concretely,
at the `carrierResults` seam (`engine.go:2625` → `carrier.go:522` →
`declaredReturnCarriers`'s ReturnsFn call), keyed off the sig's owning
registry (`match.Reg` / `FnDefInfo.Registry`), restore-on-exit, no-op when
owner == main or check inactive. This converts the WHOLE seam (every
merged-word user-fn dispatch), not a subset — the §2.2 rule. The memo-key
scopeID discipline (`.1` §5a) already guarantees no cross-registry collisions;
the `CheckState.Clone` snapshot sites (`util.go:281-…`,
`compile_sandbox.go`) already restore in place so transient sharers observe
rollbacks.

## 6. Staged landing plan

Discipline for every stage (unchanged from the repo standard): probe-first;
`make fmt && vet && lint && test`; `make verify-bytecode` (differential +
whole-corpus fallback parity + combinations + property fuzz + `-race` +
`borudebug`, **0 divergences**); crossdiff (0 interpret divergences);
`make status`; ratchets move down only, with rationale; per-stage landing test
with positive + negative; **gate-clean-or-revert**. No stage may raise the
island count (§2.1) or migrate rows *into* untargeted buckets.

### Stage 0 — clean-tree re-census (hours; no engine code)

The §0 inventory came from a dirty-tree binary (bucket totals byte-match the
committed census, but per-row identity deserves a clean pin once the
concurrent Phase 4.4/4.5 work lands). Re-run the census + `make status`,
commit the refreshed `COMPILED_STATUS.md`, name the 1 island row, and record
the 78-row list in the landing-test fixture for the M-stages.
**Gate:** census reproduces §0 (or the doc's forecast table is re-based, in
this file, before M1). **Revert criterion:** n/a (no product code).

### Stage M1 — the merged-word seam: shareCheckState at the ReturnsFn boundary

Target mechanics: §2.4(a); protocol: §5. When the check pass dispatches a
merged/open user fn whose owning registry ≠ the active one, share the check
state around the `ReturnsFn` call so `buildFnBodyReturnsFn` unit-compiles into
the main program and `RecordUserCall` references it. The module-minted param
types (`Flag`, `Point`, `Baron`) ride `SetUnitParamTypes` as `*Type` pointers
under the CanonicalType rule (§5) — the VM param guard then enforces them at
`CALL_USER` entry exactly as the interpreter's `matchSignature` did.

- **Reproducers:** `open-words.tsv:72` (value:
  `import module [def Flag (refine Boolean) def add fn [[a:Integer b:Flag]…]…]
  add 1 (M.mk true)` → `true`) and `micron.tsv:200` (refine-with-body class).
- **Forecast:** user-fn-call 10 → 0. The 5 open-words dispatch-recovery rows
  (`:32,83,84,90,100`) are the same family's ERROR/ambiguous variants: expect
  the value paths to progress and the ERROR rows to need M4's definite traps
  or the existing `OpCallUserPoly` re-match (`tryCompileUserPolyArms`,
  `core_helpers.go:806-…`) extended to merged cross-registry arm sets — count
  them for M1+M4 jointly, not M1 alone.
- **Gate additions:** the `.0` §4 regression class — a diagnostic-count
  assertion over a decision-shaped fixture (`lang/go/module_fn_checkpath_gate_test.go`
  already exists; extend with a merged-`add` case), because the historical
  failure mode of check-state sharing was +29/+39 spurious `no_signature`, not
  a differential miss.
- **Revert criteria:** any refusal-bucket total rises; any new
  dispatch-recovery row appears outside open-words/micron (the §2.2 poison
  signature); any check-diagnostic count change on the corpus; any
  verify-bytecode/crossdiff divergence.

### Stage M2 — fn-value application & read frontier (the big block, sub-staged)

§2.4(b), ~43 rows. Not body compilation — `OpCallDynamic`-family machinery.
Sub-stages, each independently gated, ordered by mechanism reuse:

- **M2a `apply`-over-param-fn** (recursion:90-92; function-valued-operand 3):
  the two coordinated lowering changes already pinned in
  `boru-bytecode-next-stages.0.md` §"Update": route a fn unit's body residual
  through `resolveDynamicApply`, and extend `trailingApply` to accept a
  `resolveOperand`-able (local) fn. `RecordDynApply` (`emit.go:2214`) is the
  landed intermediate-apply precedent (def-bound applies, `baaf45b7`).
- **M2b path-modifier map-stored fns** (path-modifier 12: operand-provenance
  9 + unconsumed-carrier 3): a concrete map's fn member with `/u /f /2 /s`
  modifiers. The get-side fold is the documented hazard
  (`native_storage.go:489` family — folding tried and reverted); the sound
  route is the M2a apply machinery over a `CALL_NATIVE get` result plus
  modifier-aware arity, never a check-time fold of the member.
- **M2c instance-method dispatch** (module-log 12 incl. the boundary-7,
  paren-1, log-register-2; module-rand:14,15): the `OpCallDynamicMixed`-class
  pattern (capture-threaded doc §"family" table) — a shaped receiver carrier
  whose method resolves at run time. Needs with-seed/span/logger returns to
  declare record shapes typing their methods as Function (the Phase 4.3
  precedent: record-shape Returns) + CALL_DYNAMIC carrying the callee's
  NoEvalArgs/closure shape for code-body methods (`r.list-of` analysis in the
  stage3 plan — the freeze-gate analysis there stands: **never const-fold a
  stateful draw**). module-rand:14/15 sit behind the miscompile-E guard
  (a refusal that stops a known miscompile, `emit.go:2661-2671`); they clear
  only when the auto-dispatch has a real runtime model, and until that model
  is built they stay refused — an open defect owed that model, not an
  approved outcome. Do not weaken the guard to move 2 rows: the fix is the
  runtime model, never a looser guard.
- **M2d fn-value-as-operand** (module-minilang:306-315 `is` + corpus-core:134
  `walk` two-lambda form): bake an immutable module-export fn value as a const
  operand (`e3f925ce` precedent: module-fn-value-as-arg) where the consumer
  treats it as DATA (`is` against a predicate value); the walk 4-arg
  two-lambda row rides the existing walk closure model
  (LambdaSharesTokenShape) extended to the second hook.
- **Forecast:** M2a 3; M2b 12; M2c 12–14 of which module-rand:14/15 and
  module-log:72/73 may still refuse until the auto-dispatch model lands —
  the guard holds, the defect stays open and tracked; M2d 11.
- **Revert criteria:** operand-ORDER divergence anywhere (the
  `(3 and "x") add 1` and `[1x]`-vs-`[x1]` history — every M2 sub-stage
  re-runs the reverted shapes as pinned negatives); any weakening of the
  miscompile-E refusals without a runtime model; fuzz divergence.

### Stage M3 — DSL registration/provenance rows (module-parse 11 + minilang:320)

> **Historical (2026-07):** the registration surfaces this stage compiled
> (`Parse.register` / `MiniLang.register` / the deferred-kind dispatch)
> have since been REMOVED — the kind namespaces are frozen and custom
> languages are fn VALUES. The compile machinery described here was
> replaced by `Parse.parser` + the recorded `parselang-fn-dispatch` call
> (fn-value dispatch), and the minilang growth ledger now has an empty
> growth set.

`Parse.grammar/spec/action` and `MiniLang.register` produce values through
handler-only natives whose results record nothing (the parselang:23 mechanism,
solved for parselang by giving registration a sound check-mode twin — see the
stage3 plan's "register-ReturnsFn attempt": the LEAK it hit is the design
constraint: **check-mode registration must not mutate the shared runtime
export map**; use a check-only registration table consulted by
`moduleExportGet` in check mode, or Pos-idempotent runtime registration).
Forecast: 8–11 of the 11; `module-parse:15` (dynamic input at dot) may need
M2c's receiver model too. Revert criteria: any `parse_kind_exists`-class
differential mismatch (the exact recorded failure of the reverted attempt);
export-map state observable after a check pass
(`TestStoreShapeObservationFree` is the model for the pin).

### Stage M4 — definite-trap extensions (the ERROR rows)

apply:37,38 · convert-ideal:30 · forward-barrier:80 · generics-sugar:37 ·
generics:60 · word-splice:115 (+ open-words:83,84,90,100 with M1). These are
the "7 need carrier-disjointness proofs" residual of the Phase 3.4 trap work:
`tryRecordUnmatchedDispatchTrap` declines on carrier operands; extend
definiteness to *provably disjoint* carrier args (a `Box<String>` instance vs
a declared `Box<Integer>` param is statically Never — the same disjointness
proof `checkBodyReturnConformance` uses). Forecast: 6–10. Revert criteria:
any trap whose taxonomy/Detail/position differs from the interpreter by one
byte; the flex-Reach negative stays pinned.

### Stage M5 — unit refinement under recursion (off-corpus; the voxgig lever)

M5a genArgs declared-param narrowing (Tier N, §3) + M5b the recursion-bail
provenance fix (§4 rule: bail carriers must be seated/reminted against the
recursive `CALL_USER` event so branch merges keep producedBy). Land together;
neither alone compiles anything (§16's measurements). Then re-probe radix-msd:
if layer 1 (Array element typing) is still binding, that work belongs to the
checker-precision track (parameterised mutable-Array carriers with
set-widening — genuinely multi-session), NOT to Phase 6. Tier S (value
specialisation) is implemented only if a live row/file demands it after M5;
it ships behind `BORU_BC_SPEC=1` (internal env, default off, the `BORU_COMPILE`
precedent) with the census run both ways in CI until it has a non-zero win,
else it is deleted, not kept dormant (the capture-threaded lesson: dead
machinery in the miscompile-sensitive tree is a cost).

### Stage M6 — deferral decisions (with the maintainer, not code)

recursion:72 (true dynamic scope — Stage F: recommend option 2, deferral on
the maintainer's docket, per next-stages §F); recursion:71 (branch-scope
forward ref — same family); corpus-core:134 if M2d declines;
module-rand:14/15 + module-log:72/73 if the auto-dispatch model is not built.
Deferral is a schedule, not a sanction: each row stays an open defect with a
named unlock (a VM def-stack mirror, the two-hook walk extension, the
auto-dispatch runtime model) and an owner. Phase 7's "re-arm ceilings at 0"
means 0 — any row still refusing then is compile debt carried into Phase 7,
which the tier accounting tracks rather than excuses.

### Corpus re-baseline protocol (every M stage)

- `computeRefusalCeiling` (66) ratchets down by exactly the stage's cleared
  rows; the census diff must show ONLY the targeted rows moving
  refused → native (or refused → error-trap for M4), with the differential,
  fallback-parity, combinations and fuzz gates green before and after.
- Any *untargeted* row that moves (either direction) blocks the merge until
  explained in the commit message — bucket migration was the signature of
  both historical regressions (§2.2, `.6` §3).
- `COMPILED_STATUS.md` regenerates per stage; ceilings never rise; the
  error-row allowlist (13) may only shrink.

## 7. Bucket impact forecast (honest counts against §0)

| bucket | n | M1 | M2 | M3 | M4 | M5 | still refused = open defect |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| user fn call (Stage 3) | 10 | 10 | | | | | 0 |
| dispatch recovery | 14 | (5→M4) | | | 5 + 5–6 | | 0–4 (flex:88,95 are G5-stage-3/4 shapes; forward-barrier:80 needs branch-typed each) |
| fn value reaches word | 11 | | 11 | | | | 0–1 (corpus-core:134) |
| operand provenance | 22 | | 9 | 11 | | | 2 (recursion:71,72 → M6 docket) |
| fn-value-call boundary | 7 | | 7 | | | | 0 |
| function-valued operand | 5 | | 5 | | | | 0 |
| container auto-dispatch | 4 | | 0–4 | | | | 0–4 (miscompile-E guard holds; defect open) |
| unconsumed carrier | 3 | | 3 | | | | 0 |
| dynamic input | 1 | | 0–1 | 0–1 | | | 0–1 |
| paren-bounded apply | 1 | | 1 | | | | 0 |
| **total** | **78** | **10** | **~36–40** | **~9–12** | **~10** | **0 on-corpus** | **~2–10** |

What Phase 6 does **not** touch (for Phase 7's entry assessment): the 13
allowlisted error rows (their own Phase 3.4 residual), the 1 island, the 250
check-error rows, the tier-1 (`Vm.run`) rows (deferred here, not resolved),
M1/M2 miscompile classes (closed in Phase 1), and bytecode serialisation. M5
moves zero corpus rows by design — its wins are voxgig-file and
fuzz-robustness wins.

## 8. Risk register

1. **Check-state sharing regressions (M1).** Historical failure = spurious
   diagnostics corpus-wide, invisible to the differential. Mitigation: the
   diagnostic-count gate + the module_fn_checkpath fixture + whole-seam (not
   partial) conversion + scopeID keys already landed. Residual risk: the
   `StepCount`/budget merge (one shared budget was a latent under-count —
   watch for new `step_budget_exceeded` on module-heavy rows; if any appear
   they are *correct* but must be triaged, `.1` §8).
2. **Two-paths drift (all stages).** Any additive path must be probe-first
   and commit-on-clean (`forkForProbe` seeding — the §14 recursive-closure
   lesson: a probe that loses the enclosing fnUnits tables re-compiles the
   enclosing fn and manufactures phantom refusals).
3. **Operand order on dynamic apply (M2).** Two reverts in history. Every M2
   sub-stage pins the exact reverted shapes as negatives; `OpCallDynTrailTop`'s
   reversal contract is normative.
4. **Freeze-gate violations (M2c).** Any path that lets a stateful module
   word's draw const-fold into a shared structure re-opens the module-rand
   divergence. `RecordMakeList`'s builtin-word screen stays; specialised units
   (Tier S) must inherit it — a Tier-S unit may fold *arguments*, never
   *effectful call results*.
5. **Step-budget / tape interactions (Tier S).** A value-specialised unit
   re-analyses a body per call site; bound = one per site, probe-first, and
   the FnAnalysisQuota counter already caps per-name analysis rounds
   (`carrier.go:3444-3462`). If quota diagnostics appear, S is over-firing —
   revert criterion, not a tuning knob.
6. **Dynamic-scope frames (Stage F overlap).** recursion:72's `g` reads the
   *caller's* `n`. No unit model can compile it without a VM def-stack mirror
   (`OpDynLookup`) — expensive, semantically corner-case. This design assumes
   M6 defers it, which leaves the defect open on the docket rather than
   settled; if the maintainer schedules it instead, it is a NEW opcode + VM
   feature outside this round.
7. **What stays refused if assumptions fail — defects carried, not closed:**
   if the auto-dispatch model is never built — module-rand:14/15,
   module-log:72/73; if flex path-shape typing (G5 stages 3/4) stalls —
   flex:88/95; if the two-hook walk extension declines — corpus-core:134;
   plus the M6 rows. Then Phase 6 lands ~64–68 of 78 and the remainder is
   **unfinished**: each survivor keeps its defect record, its named unlock
   and its owner, and Phase 7 enters with that debt visible rather than
   retired. A refusal left standing is a hole in "all valid code compiles",
   never a documented allowance.
8. **Concurrent-work skew.** This round was designed against `f4c56a1` while
   Phase 4.4/4.5 edits were in flight. Stage 0 exists precisely to re-pin the
   inventory; if the recorder decoupling (4.5) changes any `es`-nil-vs-armed
   semantics at the seams named here (`Armed()`/`suspendedNow()` in the
   in-flight diff), M1's implementation should target the post-4.5 recorder
   interface, not `Check.Emit` directly.

## 9. Exit criteria (Phase 6, restated against live state)

Refusals 78 → ≤ 10 — a staging target, not done. Done is 0. Every survivor
is an open defect and carries either (a) a written account of what is
unimplemented or unproven plus the maintainer-agreed schedule for fixing it,
or (b) a named non-Phase-6 owner (G5 store shapes, checker Array-element
precision) — and it stays on the books until it compiles; islands still ≤ 1
and never increased by any stage; `correct-error == 0` held;
`computeRefusalCeiling` ratcheted monotonically with per-stage rationales;
the voxgig sweep re-run (Phase 5.3 debt) after M5. Phase 7 then deletes the
whole-program interpreter re-run (`RunCompiled` → `Compile` + `RunProgram`,
completion plan P7 item 3): scaffolding removed, not a fallback taken away,
so every row still refusing surfaces as the compile failure it always was.
