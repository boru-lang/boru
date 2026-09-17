# boru Bytecode — Next Stages (detailed work to refusalCeiling 0 + P7)

Status: design / work-breakdown. Written after the session that drove
`refusalCeiling` 20 → 16 and landed the variadic-stack VM mechanism. This is the
execution spec for the remaining 16 refusals and the final P7 deletion. Companion
to `boru-bytecode-cluster5-residual-lowering.0.md` (Gap A/B detail),
`boru-bytecode-finish-line.0.md` (roadmap), and `module-fn-checkstate-ownership.{0..6}.md`
(the module-body-compilation project).

Read those first for context; this doc is the *forward* plan, organised by the
**foundation** each remaining row needs rather than row-by-row, because the rows
cluster onto a small number of foundational changes.

> **Doctrine (overrides any older framing in this doc).** Done is a language
> that compiles, as a developer expects — ALL valid code compiles, no
> exceptions. The interpreter is **not** a fallback for the compiler and is
> not allowed to become one. Every refusal counted below is a **defect** — an
> unimplemented or unproven case, owed a fix and tracked to closure — never a
> sanctioned design outcome, and never a tier a row can be retired into to
> make a ceiling fall. The containment machinery is real and is described
> here as machinery: `RunCompiled` **silently** re-runs a refused program on
> the interpreter (the refusal itself recorded through `MarkUncompilable`,
> sub-program regions through the fallback-island opcode) so the user still
> gets an answer while the defect is open. The silence makes it worse, not
> better — the failure hides itself. Stage J deletes that path.

---

## Update (2026-07-21) — Stage-3 fn-value dispatch: body-tail dynamic apply landed

The fn-unit fn-value-application frontier moved by widening the EXISTING
whole-frame replay recorder (`noteDynFrameReplay`, per REFUSAL-CLOSURE §2's
route-dynamics-to-runtime doctrine) rather than the two coordinated
`resolveDynamicApply`/`trailingApply` changes sketched below — the
`OpCallDynFrame` mechanism subsumes them for the body-tail shape:

- **Trigger widened**: a count-mismatched fn-body residual carrying a
  **`Dynamic`** value (a bounded gradual carrier — a map get over `Any`,
  the boru:fmt stylesheet driver `def apply fn [nd:Any Any [nd (rules get
  (Fmt.kind nd))]]`) now arms `OpCallDynFrame` + `RetReplay` exactly like a
  statically Function-typed one. Faithful for BOTH runtime outcomes: a
  callable value re-steps under `execFnDefLiteral`'s own rule (forward
  collection included); a non-callable value stays data and the RET raises
  the interpreter's own count error. This also CLOSED the latent
  divergence where such a body compiled under the symmetric-RET-error
  assumption but the interpreter applied the runtime-callable value.
- **Tail proof re-anchored on the trace**: `replayIsBodyTail` orders by
  recorded event seq when the window holds an event result (source columns
  cannot order a nested-paren argument against its consumer); the all-inert
  window keeps the source-position proof.
- **Out-of-order residual seated**: `replayForceOrder` promotes the
  residual events so `[inert-local, dyn-event]` re-pushes in exact token
  order — the layout the replay re-steps.
- Pins: `TestEdgeFindingDynamicFnValueApplyBodyTail` (graduation + the
  still-refusing mid-body shape), `fmt_compiled_parity_test.go` (both
  boru:fmt declarative-formatter demos compiled, `wasCompiled` asserted —
  including the dispatch-by-`Fmt.kind` end-to-end demo whose dynamic ATOM
  key trips a pre-existing `get (Map, dynamic(Atom))` checker
  false-positive, so it is pinned here rather than in the check-accuracy-
  gated corpus), module-fmt.tsv corpus rows (a check-clean concrete-key
  driver + its not-callable count-error twin).

## Update (2026-06-25) — islands back to 0; quoted-operand cluster cleared

A status-completion pass (branch `claude/frame-cleanup-over-pop-dzhf04`)
re-measured against the grown live corpus and landed the safe, sound
wins, all `verify-bytecode`-clean with per-row landing tests:

- **`has` / `inspect` quoted-operand** (corpus-core.tsv ×4) →
  `CompileQuoteInert`, baked as plain CALL_NATIVE. **refusals 23 → 19.**
- **`do {map}`** (corpus-core.tsv:60) — moved `CompileFallbackBody` from
  the word to the List sig; the Map sig is a pure value-eval that bakes.
  **islands 2 → 1.**
- **`outer`** (corpus-core.tsv:101) — routed through the `InvokeBody`
  seam + a `CallableSpec`, compiling its body to a closure unit.
  **islands 1 → 0** (the `islandCeiling` target, restored after corpus
  growth re-introduced 2 islands).

**Live state now: islands 0, refusals 19** (the corpus grew from the 2830
rows below to 3435, so the absolute refusal count is higher than the
historical 16; the ratchets are informational per `compiled_coverage_test.go`).

**Confirmed about the remaining 19 (this pass):** they are the
fn-value-application + module-body frontier, not coverage gaps. The
largest cluster (9, path-modifier `/u /f /2` over a map-stored fn) is the
documented hazard at `native_storage.go:489` (folding a fn-valued field
to concrete was tried and reverted). The `apply`-over-DYNAMIC-fn rows
(3, `comp/r apply` where `comp` is a fn param) were probed: eliding
`apply`'s recording is **unsound** — it leaves the fn+arg uncollapsed (a
return-count mismatch that "compiles" a malformed unit instead of
refusing; and refusing is not a resting place either — it leaves the
rows open). The real fix is to EMIT an `OpCallDynamic` inside the fn
unit with the apply operand order (`[args, fn]`) reversed to
`CALL_DYNAMIC`'s `[fn, args]` — Stage G proper, gated by the same
operand-order discipline as the `(3 and "x") add 1` revert. **Concrete
blocker found this pass:** `resolveDynamicApply` (the fn-value-call
boundary) is wired ONLY into the program-residual `Finalize`
(`emit.go:3279`), NOT into the fn-unit finish (`StartFnCompile`'s
`finish`). And `trailingApply` requires the fn to be an event-produced
value that is the last VM push — a fn PARAM is a frame local, not an
event. So clearing `apply`-over-param-fn needs BOTH: route a fn unit's
body residual through `resolveDynamicApply`, AND extend `trailingApply`
to accept a `resolveOperand`-able (event OR local) fn. Two coordinated
lowering changes, not a flag flip. The rest are
Stage C (module-body compilation, a corpus-re-baseline project),
Stage E (flex reference cells), and Stage F (dynamic scope). None is a
single safe commit; each needs the deliberate per-stage work below.

## 0. Current state (historical — measured when this doc was written)

| ratchet | value | target |
| --- | ---: | --- |
| `refusalCeiling` (compiled_coverage_test.go) | **16** | 0 |
| `islandCeiling` (compiled_coverage_test.go) | **0** ✓ | 0 (done) |
| `reducibleCeiling` (compiled_metafallback_test.go) | **2** | 0 |
| `interpreterOnlyCeiling` | 0 / cap 3 | 0 — a capped row is an open defect, not a tier |
| `computeRefusalCeiling` | 18 / 86 | 0 |

Corpus: 2830 value rows; 2502 compile (0 islanded), 16 refuse.

### Infrastructure already in place (reuse these)

- **Variadic stack region** — `OpStackMark` / `OpDropToMark` / `OpPopMark`
  (bytecode.go), a per-run `marks []int` in the VM (`vm.go::run`), the
  `planVariadicClaims` pre-pass + `lw.markBefore`/`lw.variadicElse` lowering
  (lower.go). Built for the chained variadic-if; the primitive (truncate the stack
  to a saved depth) is the building block for **all** runtime-variable-count work.
- **Terminal-trap infrastructure** — `EmitState.RecordTrap` (top-frame only) +
  `MarkUncompilable` is a no-op once `trapAt` is set (emit.go), so an operation
  that *is* the trap no longer self-refuses. Reuse for any remaining error-row.
- **Container const-fold** — `constFoldContainerVal` + the `autoEvalMap` bare-value
  fold (engine.go), gated by `exprRefsCarrier` + `containsSharedMutable`.
- **Residual reconciliation** — `forceOrder` + `planValueDefLocals` +
  `seatResults` (emit.go/lower.go); promotes out-of-order producers to frame
  locals.
- **fn-value trailing apply** — `resolveDynamicApply` + `OpCallDynamic` /
  `OpCallDynamicTrailing` (emit.go:2710 / vm.go:329) for a residual fn value
  applied to its neighbours.

---

## 1. The 16 remaining rows by foundation

| # | foundation (stage) | rows | risk |
| --- | --- | --- | --- |
| A | variadic branch/return modeling | `recursion.tsv:53` | high |
| B | conditional dynamic apply in a branch | `forward-barrier.tsv:83` | high |
| C | sound module-body compilation (cross-registry EmitState) | `module-test.tsv:38`, `module-parselang.tsv:23`, `module-rand.tsv:38` | very high (corpus re-baseline) |
| D | dynamic dispatch to module/sub-registry words | `reach.tsv:38`, `module-io.tsv:29`, `module-io.tsv:30` | high |
| E | VM reference cells (by-reference containers) | `flex.tsv:138` | high |
| F | dynamic-scope frames | `recursion.tsv:72` | high (soundness) |
| G | closure-return / fn-value-call boundary | `bytecode-combinations.tsv:74`, `def-node-binding.tsv:54`, `fn-value.tsv:19`, `patrun.tsv:40` | high (soundness) |
| H | dispatch-recovery operand order | `bytecode-combinations.tsv:113` | medium |
| I | divergent macro (open defect / spec decision: is the row valid boru?) | `macro.tsv:45` | n/a |

Stages C–G also clear the 2 reducible rows (Test/Assert, quote-macro) and chip at
`computeRefusalCeiling`.

---

## Stage A — variadic branch/return modeling (Gap A remainder)

> **Superseded VM analysis — see
> [`boru-bytecode-stageA-branch-result-modeling.0.md`](boru-bytecode-stageA-branch-result-modeling.0.md).**
> The deep-dive validated this stage against the live tree (June 2026) and found
> the VM side is **already done**: `OpRet` never truncates the stack
> (`vm.go:812-849`) and `checkReturnContract` no-ops when `len(Returns)==0`
> (`vm.go:1030-1032`), so a `[]`-declared variadic return needs **no opcode
> change** — points 2 below ("OpRet leaves a fixed count") and the "OpRet must
> leave all values" design bullet are stale. The load-bearing change is the
> recorder's **single-operand arm model** (`emitBranch.thenOut/elsOut`,
> `resolveArm` at `emit.go:922` takes only `stk[len-1]`), not the VM. Read the
> deep-dive for the corrected, staged plan; the rest of this section is the
> original framing.

**Row.** `def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]] m 3`
→ `6 4 2`. Reason: `fn m: branch leaves extra values (Stage 2 lowers single-result
branches)` (`lower.go:864`, `lowerFragment`).

**Root cause.** Three coupled gaps:
1. **Multi-value branch arm.** The else body `[n mul 2 m (n sub 1)]` leaves
   `n*2` (1 value) **plus** the recursive `m (n sub 1)` result. `lowerFragment`
   (lower.go:840) requires an arm to net exactly `[out]` or 0 — it rejects >1.
2. **Variadic fn return.** `m` declares `[]` returns but produces N values at
   run time (N grows per frame). `reconcileResults` (lower.go:760) rejects a
   variadic fn-body residual (`rejectVariadic=true`), and `OpRet` leaves a fixed
   count.
3. **Variadic `CALL_USER`.** The recursive `m (n sub 1)` returns a runtime-
   variable count; `evCallUser` carries a fixed `nout` (`emit.go:1411`,
   `RecordUserCall`), and `AnalyseFnBody`'s residual is a fixed stack.

**Design.**
- Model a branch/fn result as a **count range** `[lo,hi]` (extend `lw.variadic`
  from a bool to carry a range, or add `lw.variadicN map[int]struct{lo,hi int}`).
  The then=0/else=(1+variadic) arm merge yields a variadic the residual absorbs —
  `lowerArms` (lower.go:1116) already marks a merge variadic on count mismatch;
  extend it to allow an arm that is *itself* multi/variadic.
- `lowerFragment`: when `allowVariadic` and the arm's residual is `[fixed…,
  variadic-tail]`, lower all of it and propagate a variadic merge slot instead of
  refusing.
- Fn return: let a `[]`-declared fn return its full residual. `reconcileResults`
  accepts a variadic tail; `OpRet` must leave **all** values above the frame base
  (it already pops the frame — change the result-count handling to "everything
  above `stackBase`" for a variadic-return unit). `checkReturnContract`
  (vm.go:943) must skip the count check for a variadic-return unit.
- Variadic `CALL_USER`: `AnalyseFnBody` returns a residual whose tail is marked
  variadic; `RecordUserCall` records `nout = -1` (sentinel: "all results");
  `lowerUserCall` (lower.go) pushes a variadic sim slot. The VM's `OpCallUser`
  already appends whatever the callee left, so no opcode change — only the
  sim-side count model.

**Hardest part.** The recursion must converge: `AnalyseFnBody`'s memo
(`FnSummaries`) keys on arg types; `m`'s summary is "variadic", and the recursive
call inside reads that same summary — a fixed point reached in one pass because
the variadic marker is count-agnostic. Verify the memo does not loop.

**Soundness hazard.** A variadic fn result must only ever flow to the **program
residual** or another variadic-absorbing position — never to a fixed-arity call
operand (the count is unknown at the call site). Gate: refuse a variadic
`CALL_USER` result consumed as a sig-position operand — the gate stops a
miscompile, and the refusal it leaves is the next defect on the list (model the
count at the call site), not the finish line.

**Files.** `eng/go/lower.go` (`lowerFragment`, `lowerArms`, `reconcileResults`,
`lowerUserCall`, the `variadic` model), `eng/go/emit.go` (`RecordUserCall` nout
sentinel, `AnalyseFnBody` residual marking), `eng/go/vm.go` (`OpRet` /
`checkReturnContract` variadic-return), `eng/go/carrier.go` (`AnalyseFnBody`).

**Verification.** `recursion.tsv` rows; a fn returning a *fixed* multi-count must
still count-check (no regression). New `bytecode_findings_test.go` row:
`m 3 → [6 4 2]`, plus a negative (a variadic result fed to `add` must refuse
until that shape is modelled — the test pins the containment, it does not bless
the refusal). Full `verify-bytecode`; lower `refusalCeiling` 16 → 15.

---

## Stage B — conditional dynamic apply in a branch

**Row.** `import "boru:math-util" def n 5 if (n eq 0) [99] MathUtil.sqrt 16`
→ `4.0`. Reason: `if: else value of unknown provenance` (`emit.go:1004`).

**Root cause.** The 3-arg if's **else** is the module-export fn value
`MathUtil.sqrt` (a `get sqrt MathUtil` result), and the trailing `16` auto-applies
to the if **result** — but *only* on the false branch (where the result is the
fn). On the true branch the result is `99` and `16` is a separate residual value.
So the auto-apply is **conditional on the runtime branch**:
- `n=5` (false) → else → `sqrt` → apply `16` → `4.0`.
- `n=0` (true) → then → `99`; `16` stays → `[99, 16]`.

Two sub-problems: (1) resolve a module-export fn value as a branch-else operand
(today `resolveOperand` declines it → "unknown provenance"); (2) lower a trailing
apply that fires only when the branch produced a callable.

**Design.**
- Resolve `MathUtil.sqrt` (an immutable module-export fn) as a baked **const fn
  value** operand — the "no-capture fn value bakes as a const" path already
  exists; extend it to a module-export `get` of a fn field (the get folds to the
  concrete FnDef, which bakes).
- The if result is then a value that is *sometimes* a fn. The trailing `16` must
  lower to a **runtime-conditional apply**: push 16, then `OpCallDynamic`/
  `OpCallDynamicTrailing` over the if result — `callDynamic` (vm.go:329) **already**
  checks callability at run time and leaves the value + arg as data when not
  callable. So the existing dynamic-apply opcode handles "apply if callable, else
  leave both" — exactly the `[99,16]` vs `[4.0]` contract.
- The work is connecting the branch result (variadic-or-value) to the trailing
  `resolveDynamicApply` path, which today only triggers for a residual whose
  *lead/tail* is a fn value, not for a branch result that *may* be a fn.

**Soundness hazard.** Verify `callDynamic`'s non-callable fall-through produces
exactly `[result, 16]` in the same order the interpreter leaves them (the prior
`(3 and "x") add 1` revert was an operand-order divergence — re-test order).

**Files.** `eng/go/carrier.go` (`resolveOperand` for a module-export fn get →
const fn), `eng/go/emit.go` (`resolveDynamicApply` extension to a branch result),
`eng/go/vm.go` (`callDynamic` — already handles the fall-through; verify order).

**Verification.** `forward-barrier.tsv:83` (n=5 → 4.0) AND a both-polarity battery
(n=0 → [99 16]); 0 divergences. `refusalCeiling` -1.

---

## Stage C — sound module-body compilation (cross-registry EmitState) — LARGEST

### Update (2026-07) — cross-scope binding reads now compile via dynamic scope

The cross-registry EmitState *sharing* mechanism (`shareCheckStateFrom`,
`execFnDefSig`) was already in place, so module-preamble fn bodies are
*attempted* as units. The residual blocker was a specific read shape inside
those units: **an enclosing-scope binding read whose value carries a producing
event from the parent/module frame** — a module-scope `flex` accumulator
(`mustache-acc` in the Template lexers), a computed module `def` (a seeded
`Rand.with-seed` carrier). Such a read resolved to the enclosing event operand,
which is unreachable across the fn unit's scope floor → refused ("branch reads
enclosing computation") or islanded into a non-inert refusal.

The fix (this session) routes it through the dynamic-scope path that Stages
E/F already built:

- **`resolveOperand`** (emit.go): when a value's producing event is an
  enclosing-scope binding (its ID is in the reading unit's `enclosingBindIDs`
  snapshot, taken at unit open) and we are inside a fn unit, route to
  `dynScopeRescue` instead of the in-frame event operand. Immutable list/scalar
  literals are consts with no producing event, so they still const-fold — only
  computed / mutable enclosing bindings take this path.
- **`dynScopeRescue`** (emit.go): admit an enclosing binding unconditionally
  (it is genuinely in dynamic scope for the fn), bypassing the fn-binder
  reachability model, which only covers names bound *inside* a fn frame. A
  typo is still refused (absent from the snapshot AND has no fn binder).
- **`collectDynBindSources`** (lower.go): promote a `def`'s computed source for
  its `OpBindDynScope` install whenever the name is dynamically read
  (`dynScopeNames`), not only under full `dynEnv` mode — so a module-scope
  `def (flex …)` becomes registry-visible for the unit's `OpLookupDynScope`.

At run time `OpLookupDynScope` re-resolves the live binding per call against the
module registry — for a mutable flex/rand carrier that is the SAME cell the
interpreter reads/mutates (by-reference, Stage E), so the compiled read advances
in lockstep. Verified byte-identical `compile == interpret` across the Template
corpus (mustache/handlebars/liquid/jinja render, the mutating `mustache-acc`
lexers) and the rand receiver-closure case across many seeds; `verify-bytecode`
and the full corpus differential stay clean. Landed refusals 11 → 9 in the
langspec census (the `Rand.with-seed` receiver closure + the compute-frontier
gap); `module-rand.tsv:38` compiles. `TestRandCarrierReceiverClosureCompiles`'
frozen-draw guard was updated to recognise a `LOOKUP_DYN_SCOPE` receiver as
dynamically resolved (not frozen); `tryRecordFallback`'s non-inert-arg refusal
gained a direct unit test (`TestS6aTryRecordFallbackBakedNonInertDeclines`)
since the compiler front-end no longer reaches it for this shape.

The Test-harness / parselang rows below remain (they need the hermetic
dynamic-help eval + corpus re-baseline of the original plan, steps 2–4).

**Rows.** `module-test.tsv:38` (Test harness), `module-parselang.tsv:23`
(sublanguage parse), `module-rand.tsv:38` (seeded generator) — plus the 2
**reducible** rows (Test/Assert, quote-macro) and several `computeRefusalCeiling`
rows. This stage has the widest payoff and the widest blast radius.

**Root cause (verified this session).** A module-preamble boru fn (`run-spec`,
`run-cases`, `parse_calc`, generator bodies) dispatched as a value runs its body
via `CallBoru` (`registry.go:1103`) → `sub.Run`, which records into whatever frame
is active. Attempting to unit-compile it via `buildFnBodyReturnsFn` returned a
bare `[Any]` because **the module sub-registry has a separate, inactive
EmitState**: `e.registry.Check.Emit != capturedReg.Check.Emit` and
`capturedReg.Check.Emit.Active()` is false (confirmed with a debug probe). So
`StartFnCompile` declines and no unit records — the body's internal calls (e.g.
the test subject `double`) leak onto the program residual.

This is the **shelved §5b/§6** "sound module-body compilation" project
(`module-fn-checkstate-ownership.6.md`). §6 documents why it is not a localized
fix: the dynamic-help example eval's diagnostics are load-bearing for both
construction-time checking AND the compilation corpus, so the change requires a
**corpus re-baseline** plus investigating a masked "did not throw" soundness gap.

**Design (the project, in order).**
1. **Cross-registry EmitState sharing.** When the compiling program dispatches a
   module-preamble boru fn, the fn body must unit-compile **against the main
   program's EmitState** while resolving names in its **own sub-registry scope**.
   Concretely: at the `execFnDefSig` capturedReg branch (engine.go ~4411), when
   `e.registry.Check.IsActive()`, temporarily install the main EmitState +
   check-mode onto `capturedReg.Check` (Emit is a `*EmitState`, registry.go:291)
   for the duration of `StartFnCompile` + `AnalyseFnBody` + `RecordUserCall`, then
   restore. Watch the coupled state: `FnSummaries` memo (must be scoped by
   `AnalysisScopeID`/`regID` so a module fn's summary doesn't collide — already
   prefixed, registry.go:134), `Diagnostics` (must not double-collect), and
   `suspended`/fragment depth.
2. **Hermetic dynamic-help eval** (§6 step 1). Make `GenerateDynamicExamples`
   (native_help.go) run in a fully isolated check state (snapshot+restore
   diagnostics, not just `Suspend()`), so synthetic-example diagnostics never gate
   compilation or pollute the corpus.
3. **First-class construction-time body check** (§6 step 2). Replace the help
   eval's accidental side-channel with a real post-binding, carrier-arg body check
   (the §6a-A principle: an abstract `Map`/`List` param reads `dynamic(Any)`), so
   `TestCheckUncalledFnBodyTypoStillFlagged` / `TestForwardStrandAdvisory` stay
   sound.
4. **Corpus re-baseline** (§6 step 3). With synthetic errors gone, re-baseline the
   `test/go/langspec` ceilings and per-row tiers, and investigate the masked
   `Assert.throws "did not throw"` row (a likely latent compile-soundness gap the
   eval was hiding).

**Soundness hazard.** This stage can change *what compiles and how it runs*
(§6 found a behavioral change behind partial suppression). Do steps 1–4 as ONE
reviewed change, never a partial diagnostic filter.

**Files.** `eng/go/engine.go` (`execFnDefSig`), `eng/go/registry.go`
(`CallBoru`, `CheckState` sharing helper — add a `BorrowCheck(parent)`/restore that
shares `Emit`+`Mode` and snapshots `Diagnostics`), `lang/go/native/native_help.go`
(hermetic eval), `eng/go/carrier.go` (construction-time body check), and
`test/go/langspec/*` (re-baseline).

**Verification.** `module-test.tsv:38`, `module-parselang.tsv:23`,
`module-rand.tsv:38` compile with parity; `reducibleCeiling` 2 → 0; the two
construction-check tests stay green; the corpus re-baseline is a deliberate,
reviewed ceiling change with the "did not throw" row explained. This is the one
stage that is a *project*, not a commit.

---

## Stage D — dynamic dispatch to module / sub-registry words

**Rows.** `reach.tsv:38` (`StructUtil.getpath $.a.b (StructUtil.setpath …)`),
`module-io.tsv:29/30` (`IO.write`/`read` over a dynamic context receiver). Reason:
`dynamic input at getpath` / `at set`.

**Root cause.** `tryRecordPoly` (carrier.go:847) re-matches a word's signatures at
run time via `OpCallNativePoly`, but it requires a **main-registry builtin**
(`r.IsBuiltinWord(word)`, carrier.go:860) and the matched sig to be the word's
main-registry binding (the CORE-dispatch guard, carrier.go:898–915). `getpath`,
`set`, etc. live in **module sub-registries**, so poly declines and the row
refuses — the VM's `callPoly` (vm.go:257) re-matches over `r.Lookup(word)` (main
registry), which would run the wrong word for a module-qualified name.

**Design.** Teach `OpCallNativePoly` (and `PolyRef`) to carry the **owning
sub-registry** so the VM re-matches over the correct registry. `PolyRef` gains a
registry/module handle; `callPoly` resolves the word in that registry; the
recorder admits a module word when its sub-registry sigs are safe (no meta /
fn-value / code-body / side-effect-on-dynamic-shape). This is the cluster-4
"poly-extension to non-core safe builtins" generalised to sub-registries.

**Soundness hazard.** A module word's overloads must be **value-faithful** under
runtime re-match (same first-match the interpreter takes). Exclude any word whose
dispatch mutates registry/context state in a shape-dependent way until proven.

**Files.** `eng/go/bytecode.go` (`PolyRef` + a registry handle in the Program
pools), `eng/go/vm.go` (`callPoly` registry-aware re-match), `eng/go/carrier.go`
(`tryRecordPoly` sub-registry admission).

**Verification.** `reach.tsv:38`, `module-io.tsv:29/30` compile with parity; a
negative (a genuinely runtime-shape-dependent module word) still refuses —
containment while that word's dispatch is unproven, and a row still owed.

---

## Stage E — VM reference cells (by-reference containers)

**Row.** `flex.tsv:138`: `def l [1 2] push 3 l drop l` → `[1 2]`. Reason:
`dynamic input at drop`. Also the **reducible** `flex` entry.

**Root cause.** `flex` is a reference-semantics container; mutations (`push`,
`drop`) must be visible through aliases. The VM value model is **by-value**, so a
mutation through one binding is not seen through another — the compiled program
would diverge. The carrier refuses rather than bake an unsound by-value copy;
that refusal contains the divergence but does not close the row — the missing
reference-cell model is the defect, and the design below is the fix it is owed.

**Design.** Add a **reference cell** to the VM value model — a heap-boxed
`*RefCell{ Value }` payload (eng/go/payload.go marker) that `flex` instances carry,
so `push`/`drop`/index through any alias mutate the shared cell. This mirrors the
interpreter's pointer-backed `Map`/`Store`/`Array` (which already share state
across capture, per lang CLAUDE.md "Capture semantics"). The compiled `push`/`drop`
operate on the cell, not a copy.

**Soundness hazard.** Reference cells interact with const-baking
(`isInertConst` must NOT bake a mutable ref cell as a shared const — same
`containsSharedMutable` screen used by the map-field fold) and with the args-
aliasing invariant (the `-tags borudebug` lane). Couple with the tier-2 `flex`
entry.

**Files.** `eng/go/payload.go` (RefCell payload), `eng/go/value.go` (flex value
construction), `eng/go/vm.go` (push/drop on a cell), `eng/go/emit.go`
(`isInertConst` excludes ref cells), `lang/go/native` flex handlers.

**Verification.** `flex.tsv` rows incl. alias-mutation; `reducibleCeiling` -1 (the
flex entry); the `-race` and `borudebug` lanes stay green.

---

## Stage F — dynamic-scope frames

**Row.** `def g fn [[] [Integer] [n]] def f2 fn [[n:Integer] [Integer] [g]] f2 42`
→ `42`. Reason: `fn g: body result of unknown provenance`. `g` reads `n`, which is
**not** its param or capture — it resolves dynamically to the *caller* `f2`'s `n`
(dynamic scoping).

**Root cause.** The compiler models lexical scope (params + lexical captures,
fn_capture.go). `g`'s `n` is neither — it is the dynamic call-stack's nearest `n`.
Compiling it requires modeling the **dynamic def-stack** at run time, which the
static unit model deliberately does not (a unit is reused across call sites with
different dynamic environments).

**Design (per-case soundness — this is the riskiest semantically).** One live
option, and one that is closed:
1. **Compile a dynamic read** — `g`'s body emits a runtime def-stack lookup for
   `n` (a new `OpDynLookup name` reading the VM's def-stack), faithful to the
   interpreter. Bounded but adds a dynamic-scope opcode and a VM def-stack mirror.
2. **Classify as permanent fallback** — REJECTED. Dynamic-scope reads are rare,
   but `recursion.tsv:72` is valid boru, so it must compile; documenting
   `g`-style reads as a permanent tier (like tier-1 Vm.run) and excluding them
   from `refusalCeiling` would drop the ceiling while the defect stayed open.

Do (1). An earlier pass recommended (2) on the old doctrine, and the plan it
cited calls this row "correctly refused" — that phrase is quoted as written and
is wrong: a refusal of valid code is never correct. Dynamic scope is a semantic
corner, which makes the `OpDynLookup` work delicate, not optional.

**Files (option 1).** `eng/go/bytecode.go` (`OpDynLookup`), `eng/go/vm.go`
(def-stack mirror), `eng/go/carrier.go` (emit a dynamic read for an unresolved
body word that the interpreter resolves dynamically).

**Verification.** `recursion.tsv:72` parity; a typo (genuinely undefined) still
errors `undefined_word`, not a silent dynamic read.

---

## Stage G — closure-return / fn-value-call boundary

**Update (2026-08-03): the SINGLE-ARG leading param apply LANDED — compose
compiles.** `stepCloseParen`'s paren-collapse records `(g x)` (a leading
Function-typed param/capture of an open NAMED-PARAM fn unit, exactly one
argument) through the same `RecordDynApply` event as the trailing spelling
`(x g)` — inside a sealed named frame the two spellings converge for every
runtime arity (probe-pinned: mismatched callees no-match identically). The
admission is `EmitState.DynApplyLeadEligible` (unnamed-param frames keep
the whole-frame replay — their leading collection reaches beneath the
window; event leads keep the curried machinery; closure units decline;
multi-arg leads never collapse). `compose` / `twice` / depth-3 chains /
mid-body `(g x) add 100` are native; rows in `lang/spec/fn-value.tsv` §8.
See checker-compiler-completeness-review.0.md §9.6b. The REST of this
stage (below) is unchanged.

**Update (2026-06): `bytecode-combinations.tsv:74` LANDED (refusalCeiling 7 → 6).**
The factory `def mk2 fn [[x:Integer] [Function] [([y:Integer] => [x add y])]] for
3 [(mk2 i) 10]` → `10 11 12` now compiles FULLY NATIVE (no island). Two
coordinated pieces:
- **Returned capturing closure.** `tryReturnedClosure` (emit.go) previously
  declined a returned lambda with captures (`len(fd.Captured) > 0`); it now
  resolves each capture in the FACTORY body's scope (the captured `x` is the
  factory's frame local) to a `closureCap`, threaded before `OpPushClosure`
  exactly as the in-place closure-dispatch path (`recordClosureDispatch`). So a
  top-level immediate apply `(mk 5) 10` → `15` already compiled native once the
  returned closure carried its capture (the program-residual `resolveDynamicApply`
  leading-fn-carrier case applies it; `OpCallDynamic` invokes the
  `ClosurePayload` VM-natively).
- **Per-iteration dynamic apply in a loop body.** `RecordLoop.setLoopBodyApply`
  (emit.go) detects a loop body residual that is a LEADING fn carrier (the `mk2`
  call result) with trailing STATIC args (the const `10`) and seats the args on
  `EmitFragment.applyArgs`; `lowerFragment` (lower.go) then pushes them and emits
  one `OpCallDynamic` per iteration, netting the applied value — the leading-fn
  case `resolveDynamicApply` lowers for the PROGRAM residual, now reachable inside
  a loop body fragment. Soundness gate: the leading value must be a Function/FnDef
  CARRIER (always callable) produced by an event, and every trailing arg must be a
  non-fn, non-dynamic RE-PUSHABLE operand (const/local/type) — a computed arg is
  already on the sim, so it fails the sole-fn residual check and refuses rather
  than double-pushing — containment for a shape not yet modelled, and a row still
  owed. Verified 0 divergences (corpus + combinations + property fuzz + `-race` +
  `borudebug`). Landing test:
  `lang/go/bytecode_findings_test.go::TestReturnedCapturingClosureApply` (positive
  + the computed-apply-arg negative).

**Remaining rows.** `def-node-binding.tsv:54` (`[[c1]]` fn-body list
literal — VERIFIED HAZARD, reverted), `fn-value.tsv:19` (`m.f` a map-stored fn
applied), `patrun.tsv:40` (a dispatch-table fn reaches `add`). NOTE: `fn-value`
and `patrun` may already be compiled by later work — re-measure against the live
corpus (`make status`) before picking them up; the live refusal set is the source
of truth, not this list.

**Root cause.** Each is a fn VALUE whose identity/provenance the residual model
can't track:
- `mk2` returns a **closure capturing a param** (`x`); the returned-closure unit
  exists (`tryReturnedClosure`, emit.go:1296) but the *factory return through a
  fn body residual* refuses ("body result of unknown provenance").
- `def-node-binding:54` `[[c1]]` evaluates the inner list in a scope where the
  param is bound differently than a naive `OpMakeList` over locals — verified to
  diverge (`[[9]]` vs `[[1]]`), reverted. Needs the interpreter's per-call list
  assembly scope.
- `fn-value:19` / `patrun:40` — a fn value retrieved at run time (map get /
  patrun find) applied to args: `dynamic value precedes residual args` /
  `function value reaches add`. The fn value's arity/identity is dynamic.

**Design.** Build on `tryReturnedClosure` + `resolveDynamicApply`:
- Closure return: let a fn-body residual that is a returned closure value flow as
  an `OpPushClosure` operand (the unit exists); the blocker is the residual
  reconciliation declining it — admit a closure operand in `StartFnCompile.finish`
  (emit.go:1296 already tries it; extend to the factory-in-a-loop shape).
- fn-value application: route `m.f`/`find …` results through `OpCallDynamic` (the
  trailing-apply path) — the value is dynamic but the apply opcode handles it; the
  blocker is the "dynamic value precedes residual args" gate refusing a *leading*
  dynamic fn with following args. Extend `resolveDynamicApply` to the leading-fn-
  then-args shape (it currently favours trailing).
- `def-node-binding:54`: requires fn-body container assembly matching the
  interpreter's binding scope — defer; it is the documented hazard.

**Soundness hazard.** Operand ORDER on dynamic apply (the `(3 and "x") add 1`
revert); the `[[c1]]` scope divergence. Every change here re-tests the differential
on the exact reverted shapes.

**Files.** `eng/go/emit.go` (`tryReturnedClosure`, `resolveDynamicApply`,
`StartFnCompile.finish`), `eng/go/carrier.go`, `eng/go/vm.go` (`callDynamic`).

---

## Stage H — dispatch-recovery operand order

**Row.** `bytecode-combinations.tsv:113`: `(3 and "x") add 1` → `'x1'`. Reason:
`unmatched dispatch recovered at add`. `3 and "x"` doesn't match a numeric `and`;
the interpreter *recovers* by treating the mismatch as a value, then `add` does
string concat → `'x1'`.

**Root cause.** The recovery path (`engine.go` ~6199) produces a result the poly
lowering mis-orders: a prior attempt poly-lowered to `[1x]` vs the interpreter's
`[x1]` (operand order) — verified & reverted.

**Design.** Record the recovered dispatch with the **interpreter's operand order**
preserved. The recovery yields a concrete value; lower it as a normal
`CALL_NATIVE` (not poly) with the operands in the order the recovered handler
consumed them. The fix is purely operand-order bookkeeping at the recovery seam.

**Files.** `eng/go/engine.go` (recovery seam), `eng/go/emit.go` (record the
recovered call with correct operand order).

**Verification.** `bytecode-combinations.tsv:113` → `'x1'` (exact order); the
prior `[1x]` divergence is the regression guard.

---

## Stage I — divergent macro (`macro.tsv:45`)

**Row.** `def loopy (macro [[a] [quote [loopy unquote a]]]) macroexpand (loopy 1)`
→ `ERROR:expansion too deep`. **Not statically expandable.** At check time `loopy`
resolves to a carrier, so `expandAllMacros` (macro_expand.go:410, bound 256) never
re-expands to the depth limit it raises at *run* time; the compiler cannot know
the expansion diverges without running it (and running it to the limit at compile
time is itself the divergence).

**Decision (spec/scope — yours).** The open question is what the row *is*, not
where to file its refusal. Either:
- **Spec reclassification** — decide a divergent `macroexpand` is not valid
  boru: an error row by definition, whose compiled twin raises the same
  `expansion too deep`. That closes the row rather than shelving it.
- **Compile the divergence** — if the row IS valid boru, it must compile: emit
  the expansion as runtime work so the VM raises the depth limit exactly where
  the interpreter raises it, and the row stops being an exception.

What is not available is a permanent interpreter-only tier that excludes the row
from `refusalCeiling` — that drops the ceiling while the defect stays open.
Static expansion alone cannot decide it (the compiler cannot know the expansion
diverges without running it), so until the spec decision lands `macro.tsv:45` is
an open defect like any other, and the scaffolding that re-runs it on the
interpreter is containment, not a resting place.

---

## Stage J — re-scoped P7 deletion (the endpoint)

Gated on `refusalCeiling == 0` and `reducibleCeiling == 0` — every row compiled,
none retired into a tier. Stages F/I close their rows; they do not file them away.

1. `lang/go/boru.go::RunCompiled` — replace the `a.Run(src)` whole-program fallback
   with `Compile` + `RunProgram`; a refusal becomes a surfaced compile error.
   This is the deletion of the scaffolding: today that path re-runs a refused
   program on the interpreter **silently**, so the defect never reaches the
   developer at all — a failure that hides itself.
2. Keep the `OpFallback` island (`vm.go::runFallback`, `bytecode.go::OpFallback`,
   `emit.go::RecordFallback`) but add a gate asserting **every** `OpFallback` span
   classifies tier-1 (`interpreterOnlyWords`). With `islandCeiling == 0` today,
   the only such span is a `Vm.run` of genuinely runtime-constructed source — an
   open defect held where the gate can see it, not a licensed tier.
3. Re-baseline perf/alloc (`bytecode_*_bench_test.go`,
   `bytecode_allocguard_test.go`).

---

## Recommended sequencing (leverage ÷ risk)

1. **H** (dispatch-recovery order) — smallest, well-understood, 1 row.
2. **B** (conditional dynamic apply) — reuses `callDynamic`; 1 row.
3. **A** (variadic branch/return) — reuses the variadic-stack primitive; 1 row +
   unblocks future variadic shapes.
4. **D** (sub-registry poly) — 3 rows; medium.
5. **G** (closure/fn-value) — 3–4 rows; soundness-careful.
6. **E** (reference cells) — 1 row + reducible; VM value-model.
7. **F** / **I** — the two semantic corners: compile the dynamic read (F option
   1); settle what `macro.tsv:45` IS (I). Neither is retired into a tier.
8. **C** (module-body compilation) — the project; do as ONE reviewed change with
   the corpus re-baseline. Highest payoff (3 rows + 2 reducible + computeGap), but
   schedule deliberately.
9. **J** — P7 once the ceilings are 0.

Each non-C stage lands as its own gate-clean commit with a per-row landing test
and a one-line ratchet-rationale, exactly like the four wins this session. C is a
reviewed project with an explicit corpus re-baseline.

## Discipline (unchanged)

`make fmt && make vet && make lint && make test`, then `make verify-bytecode`
(differential + whole-corpus parity + combination + property fuzz + `-race` +
`borudebug`, **0** divergences) and `make status`. Ratchets move down only, with a
rationale. Gate-clean-or-revert: every unsound attempt this session and prior was
caught by the differential and reverted — keep that the backstop. A revert
restores parity; it does not close the row, which goes straight back on the list
as an open defect.
