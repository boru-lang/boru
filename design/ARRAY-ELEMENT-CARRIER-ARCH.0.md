# `Array<T>` element typing — the pre-pass architecture (companion to §4b)

Where the whole-body admission/element decision LIVES, how it is keyed, and how
`get`'s per-call-site `ReturnsFn` consults it. This is the §4b open question of
`ARRAY-ELEMENT-CARRIER.0.md`, settled into a concrete plan. It is grounded in the
check-mode fn-body analysis architecture (citations inline); the soundness bar is
the same `compile == interpret`, 0-miscompile invariant.

## 0. The constraint that drives everything

Three facts about check mode make the obvious "type the get off its receiver"
impossible, and each rules out a tempting shortcut:

1. **Arrays are carriers in check mode, not pointer-backed values.** A
   `make Array` in the analyser yields a carrier `Value`, not an
   `ArrayInstanceInfo`. There is NO runtime pointer identity to key on. Identity
   must be synthesised.
2. **Strict carriers carry no binding-name provenance.** The only existing
   channel — `Value.DynFrom`, stamped in `stepWord` (`engine.go:2062`) — is set
   ONLY for `Dynamic` carriers. §6/Stage 0 chose STRICT element carriers, which
   are non-dynamic, so a strict array get-result reaching `get`'s ReturnsFn has
   its Type and nothing else: no "which local am I" tag.
3. **The armed compile pass is single-pass — no refinement.** `AnalyseFnBody`'s
   A2 refinement loop (`carrier.go:2967`), which re-runs the body to convergence,
   is SKIPPED when armed (recording into a compiled fn unit): "each extra runOnce
   re-records the body into the SAME open fragment, leaving multiple residuals the
   lowerer cannot reconcile." So during the pass that actually emits bytecode, the
   body is walked left-to-right exactly ONCE.

**Corollary — whole-body knowledge is mandatory and cannot be dodged.** The
element type a `get` returns must be the FINAL upper bound across all writes
(§2 of the main doc). The carrier-borne make-time type alone is unsound under a
later widening `set`; and we cannot recover order-independence by re-running the
armed pass (fact 3). Therefore the final element type per array MUST be computed
BEFORE the armed pass — a real pre-pass, not same-walk accumulation (that is the
latent unsoundness the Store `ContextTypes` path takes, §3 of the main doc).

## 1. The three channels

The design uses three distinct channels, each matched to what it must carry:

- **Element type → rides on the carrier value** (the typed-list model). `make`
  mints `NewCarrierTypedArray(T)` with `Data = ChildTypeInfo{Child}` (§4a). The
  carrier flows make → def-binding → get through `stepWord`'s value push, Data
  intact, exactly as typed-list carriers already do. `get` reads
  `DataArrayElemTypeFromValue(args[1])`. No name lookup for the TYPE.
- **Array identity → a stable make-site token stamped on the carrier.** Add an
  `ArrayID` to `ChildTypeInfo` (`value.go:162`; today: Child/Elements/Entries/Len),
  or — proven sufficient in Spike B — reuse `ChildTypeInfo.Child.Pos`. It must be
  STABLE ACROSS PASSES (the accumulation pass and the armed pass mint carriers
  independently), so it is the make call's SOURCE POSITION (`SrcPos`), not a
  per-pass counter — same source token ⇒ same ID every pass (Spike B confirmed
  this is stable and unique per make-site). The make-call `pos` is NOT visible to
  a `ReturnsFn` by default and the source-ARG `Value.Pos` is zero — so
  `carrierResults` must stash `pos` onto `r.Check` immediately before invoking the
  ReturnsFn (one-line kernel change, validated in Spike B).
  Because the ID rides on the carrier value and value copies preserve `Data`,
  `def b a` gives `b` the SAME ID as `a` — so aliasing is handled CORRECTLY (a
  `set` through `b` and a `get` through `a` key the same array), not merely
  rejected. Distinct IDs merged by control flow (`if c [make…] [make…]`) join to
  "ambiguous" → demote to Any.
- **Admission + final element type → a CheckState side-table keyed by ArrayID.**
  `CheckState.ArrayElems map[ArrayID]arrayFact`, cloned per branch and reset per
  fn-body analysis exactly like `ContextTypes` (`check.go:37,84`; `registry.go:456`).
  `arrayFact = {join *Type, makeElem *Type, escaped bool, poisoned bool}`.

## 2. The pre-pass: walk-time accumulation in an UNARMED analysis pass

The pre-pass is NOT a new syntactic walker. A syntactic scan would have to
re-implement descent into `each`/`var`/`if`/`fold` code-bodies (radix-msd's sets
and gets ALL live inside `iota … each [var [[t] … counts set …]]`,
`sort.boru:985-1011`), and `WalkBodyWords`/`collectBodyLocalDefs` (`fn_capture.go`)
deliberately do NOT descend into quoted code-bodies. Instead, reuse the normal
analysis walk, which already enters those bodies to type them:

- Run a dedicated **UNARMED** `AnalyseFnBody` for the fn body before the armed
  compile arm. Unarmed ⇒ the A2 refinement loop is ACTIVE (`carrier.go:2967`,
  `!armed`), so this pass converges the element join to a fixpoint for free —
  this IS the order-independence mechanism (fact 3 says we can't get it from the
  armed pass, so we take it here).
- During this pass, each `arr set i v` records `RecordArraySet(id, valueType)` —
  joins `valueType` into `ArrayElems[id].join` via `JoinCarriers` (mirrors
  `RecordContextSet`, `check.go:295`), and sets `poisoned` if `valueType ⊄
  makeElem` (the dual rule of §9: a set whose value type is `Any`/gradual cannot
  be proven ≤ T → poison). Because the walk reaches into loop bodies, the
  histogram-increment sets are seen with no extra machinery.
  **This hook is NOT `set`'s `ReturnsFn`** (Spike B/B2, §6). Attaching a
  `ReturnsFn` to the Array `set` sig bypasses set's store-op recording
  (`emit.go:2089` "ReturnsFn owns RecordUserCall"; `carrier.go:1016`
  special-cases `set`) and regresses radix to refusal. The accumulation is instead
  a pure side-effect in `carrierResults` (the existing path every word flows
  through): `if word == "set" && Array receiver → poison id if value bound ⊄
  elem`, leaving set's result and store-op recording untouched. **Spike B2
  confirmed this hook works** (radix compiles; non-conforming arrays decline;
  forward-pass sound). Conformance TRUSTS THE BOUND (`valT.ConformsTo(elem)`) — a
  gradual Integer conforms; concrete String and gradual Any do not.
- `escaped` is set when the array's carrier appears in any non-get/set position:
  passed as a call arg, returned as the body residual, stored into a map/store,
  captured by a nested `fn` definition, or aliased (`def b a` — observable when
  `def`'s value is a tracked-array carrier). Conservative: any doubt ⇒ escaped.
  (`convert List arr` at `sort.boru:1039` counts as an escape of a LOCAL — safe;
  radix-msd's `arr` there is the untyped param, so moot.)
- The **self-referential conformance** of the increment idiom (§5) resolves
  naturally here: the increment's set value `(counts get i) add 1`
  (`sort.boru:988`) types `counts get i` against the join-SO-FAR, but the A2
  convergence re-runs until the join is stable, so the final fact is consistent
  with the assumption. The single-shot optimistic assumption of §5 is realised as
  the refinement fixpoint, bounded to ≤2 extra rounds by the existing quota.

After this pass, `ArrayElems` holds, per array, the FINAL join, escape, and poison
— independent of source order.

## 3. The armed pass: get/set consult the frozen table

The armed compile arm (`core_helpers.go` ~518-575) then runs ONCE:

- `make` mints `Array<makeElem>` as before (identity-free; the source element
  type). Its ReturnsFn ALSO stamps the stable `ArrayID` (from make-site Pos).
- `get`'s ReturnsFn reads `id = ArrayID(args[1])` and `tc =
  DataArrayElemTypeFromValue(args[1])`, then consults `ArrayElems[id]`:
  - **admitted** (`!escaped && !poisoned && join == makeElem`, i.e. monomorphic)
    → return GRADUAL `NewDynamicCarrier(join)`. (Implementation note: gradual, not
    strict `NewCarrier` — an out-of-bounds get is `None` at run time, so strict T
    over-claims; `TestCheckTypeSoundness` caught it. Gradual T still narrows the
    Any baseline enough to collapse the radix cascade, and mirrors the list
    integer-get path. See main doc §6.)
  - else → return gradual `NewDynamicCarrier(TAny)` — today's decline-to-Any
    path. It yields no wrong answer, but "not wrong" is not the bar: every
    decline is an unproven case — an open defect, owed a fix and tracked to
    closure, never a sanctioned design outcome.
- `set`'s ReturnsFn is inert in the armed pass (the join is already frozen); it
  only needs to keep the array's element carrier consistent for the recorder.

`get`/`set` therefore never compute admission — they READ a frozen verdict. All
the whole-body reasoning lives in the §2 pre-pass. This is the per-call-site /
whole-body split the §4b open question demanded.

## 4. Option C and Option B are one predicate apart

The accumulation machinery computes the JOIN (Option B's least-upper-bound)
regardless. The two strategies differ ONLY in the get-time acceptance test:

- **Option C (first landing):** admit iff `!escaped && !poisoned && join ==
  makeElem`. A proven-monomorphic, non-escaping array returns its strict element;
  everything else declines to Any. (radix-msd: counts/cur are monomorphic Integer
  locals → admitted; tmp is poisoned by the `arr get`=Any write → Any, harmless.)
- **Option B (later):** admit iff `!escaped`; return strict `NewCarrier(join)`
  (the widened upper bound), handling build-from-empty (§9) and mixed-but-bounded
  writes.

Graduating C→B is a one-line change to the acceptance predicate, NOT a
re-architecture — the join, identity, escape, and pre-pass are shared. This is the
payoff of computing the join even under C.

## 5. Open implementation questions — resolve by spike BEFORE Stage 1

Each is a concrete unknown the map above could not fully settle; pin each with a
throwaway probe (à la Stage 0) before committing representation:

1. **Pass ordering / where the unarmed accumulation hooks.** `buildFnBodyReturnsFn`
   already issues an armed `AnalyseFnBody` (compile arm, `core_helpers.go:549-575`)
   AND an unarmed summary (`:577`). Confirm the order and whether the existing
   unarmed summary can BE the accumulation pass (it walks the same body unarmed),
   or whether a dedicated pre-pass must be inserted before the armed arm. If the
   armed arm currently precedes the summary, it must be reordered or fed the
   summary's `ArrayElems`.
2. **`SrcPos` availability in `make`'s ReturnsFn.** ReturnsFn receives `(args, r)`
   only — call `pos` is passed to `carrierResults` but NOT into the ReturnsFn
   (`carrier.go:489`). Confirm the make SOURCE arg (`args[0/1]`) carries a usable
   `Value.Pos` (`value.go:996`); if not, thread the call pos into ReturnsFn (small
   kernel change) or stash `r.Check.CurrentCallPos` around the dispatch.
3. **`ArrayID` placement & merge.** Add to `ChildTypeInfo` vs a parallel Value
   field; define `JoinCarriers` behaviour for two typed-array carriers with
   distinct IDs (→ ambiguous-identity ⇒ demote element to Any).
4. **`ArrayElems` lifecycle.** Reset at fn-body-analysis entry; PERSIST across the
   A2 re-runs of the SAME body (it is the convergence target); clone per branch
   like `ContextTypes` (`check.go:37`). Confirm it does not leak across sibling fn
   analyses.
5. **Nested-closure capture visibility.** radix-msd's gets/sets run inside `each`/
   `var` bodies where `counts` is a CAPTURE of msd-go. Confirm the captured
   carrier retains its `ArrayID` (capture install is `r.Defs.Push(cb.Name,
   cb.Value)`, `carrier.go` runOnce — a value copy, Data preserved) and that
   `ArrayElems` (on the shared CheckState) is visible inside the nested code-body
   walk. This is the make-or-break for radix-msd specifically.
6. **Escape detection completeness.** Enumerate every non-get/set use that must
   set `escaped`, and add a NEGATIVE test per category (returned, arg-passed,
   map-stored, captured, aliased, `convert`-consumed). Per `lang/go/CLAUDE.md`
   test discipline, the declines matter more than the admits.

## 6. Spike plan (throwaway, no commit) — de-risk before Stage 1

- **Spike A — identity round-trips through capture. DONE (2026-06-28): GREEN.**
  Instrumented `make Array` to mint a typed-array carrier
  (`Value{Parent:TArray, Carrier:true, Data:ChildTypeInfo{Child:NewCarrier(elem)}}`)
  and `get` to log its receiver, then ran `boru check` on the radix-msd driver.
  RESULT: across **35 gets — including every `counts`/`cur`/`tmp` access inside
  the nested `each [var […]]` closures — the receiver arrived `carrier=true,
  dyn=false` with `ChildTypeInfo` INTACT every time** (0 stripped, 0 nil-data).
  Findings:
  - The element type rides the carrier through make→def→capture→get on a STRICT
    (non-dynamic) carrier — **no `DynFrom` needed; the §7 name-keying fallback is
    unnecessary.** (`toCarrier` preserves it via its `if v.Carrier { return v }`
    guard, `carrier.go`.)
  - The inter-array dependency (§5) reproduced live: with the spike `get`
    returning `Any`, `cur`'s source `(iota 11 each [counts get v])` minted
    `Array<Any>` — i.e. `cur` is only `Array<Integer>` once `counts get` is
    admitted, so `cur` must be typed AFTER `counts`.
  - The make ReturnsFn ran ~3× per site (15 makes / 5 sites) under
    recursion/refinement re-analysis — **confirming Q3: the `ArrayID` MUST be
    pass-stable (`SrcPos`-derived), not a per-pass counter.**
  - **Architectural simplification it unlocks:** since the element type rides the
    carrier, the `ArrayElems` side-table is NOT the primary type source — `get`
    reads the element off its receiver carrier directly. The side-table becomes a
    DEMOTION FILTER only: the pre-pass publishes the set of array IDs that are
    poisoned (non-conforming set) or escaped, and `get` returns strict iff its
    receiver's ID is absent from that set, else Any. Smaller surface than §2/§3
    imply.
- **Spike C — carrier-borne gate. DONE (2026-06-28): mechanism GREEN, naive gate
  integration RED.** Wired `make`→typed-array carrier and `get`→strict-element-
  off-carrier (no side-table). RESULTS:
  - (+) radix-msd force-compiles and sorts correctly via the REAL mechanism
    (`[2,24,…]`) — not the Stage-0 global hack. The carrier-borne element type
    alone clears the cascade for the monomorphic happy path.
  - (−) a non-conforming array (`make Array [0 0 0]; a set 0 "x"; (a get 0) add 1`)
    **SILENTLY MISCOMPILES** without a gate: interpret → `x1` (String concat),
    force-compile → `1` (strict-Integer add). This is the soundness keystone made
    concrete — the gate is mandatory, exactly as §0's corollary argued.
  - Adding a get-side poison check DID correctly decline the non-conforming case
    (interpret `x1` == force-compile `x1`) — the gate LOGIC is sound when poison
    is known.
- **Spike B — accumulation hook + identity. DONE (2026-06-28): identity GREEN,
  set-ReturnsFn accumulation RED (design correction required).** RESULTS:
  - **Identity works.** Stashing the call `pos` into `r.Check` inside
    `carrierResults` (one line before `sig.ReturnsFn(args, r)`) and stamping it
    into `ChildTypeInfo.Child.Pos` gave a STABLE, UNIQUE per-make-site id
    (Row 984=counts, 1000=cur, 1001=tmp, 1033=place, 1037=arr), identical across
    the ~3 re-analysis passes. **Resolves Q2: the source-arg `Value.Pos` is ZERO
    and unusable; the call-pos must be stashed by `carrierResults`** (it has `pos`
    in scope; the ReturnsFn does not).
  - **CRITICAL CORRECTION to §2/§3: `set` MUST NOT carry a `ReturnsFn`.** Attaching
    ANY `ReturnsFn` to the Array `set` sig (even one returning `nil`) regresses
    radix to the baseline `code-body word each` refusal. Cause: a `ReturnsFn`
    OWNS the word's `RecordUserCall` (`emit.go:2089`) and `set` has special
    recorder handling (`carrier.go:1016` special-cases `word != "set"`), so a
    ReturnsFn BYPASSES set's store-op recording. The two configs are mutually
    exclusive: with set-ReturnsFn the non-conforming case declines BUT radix
    refuses; without it radix compiles BUT the non-conforming case miscompiles.
    **Therefore set-write accumulation cannot ride set's `ReturnsFn`** — §2's
    "`set`'s ReturnsFn calls `RecordArraySet`" is wrong. The accumulation must
    hook set's EXISTING check/recorder path (the place `carrier.go:1016` /
    `emit.go` already handle `set`), leaving its store-op recording intact.
  - **Still UNVERIFIED (the real remaining risk): the two-pass / forward-pass
    ordering (Q1+Q4).** Because the only set-observation hook tried (ReturnsFn)
    is blocked, whether an unarmed accumulation pass can freeze poison BEFORE the
    armed pass reads it was NOT demonstrated. This is now the gating unknown for
    Stage 1 — see revised plan below.

- **Spike B2 — set-write accumulation via the existing path + forward-pass
  soundness. DONE (2026-06-29): GREEN.** Hooked the set-write OBSERVATION inside
  `carrierResults` as a pure side-effect (`if word == "set" && Array receiver →
  poison id if value bound ⊄ elem`), giving `set` NO `ReturnsFn` so its store-op
  recording stays intact. Poison kept in a package-global keyed by the make-site
  pos (the spike's stand-in for a `CheckState` field). RESULTS:
  - (+) **radix-msd compiles and sorts correctly** (`[2,24,…]`). Only `tmp` is
    poisoned (`val=Any` from the `arr get` param read) — harmless, as designed.
    The `carrierResults` hook does NOT break set's recording (unlike a ReturnsFn).
  - (−) **set-before-get declines**: interpret `x1` == force-compile `x1`.
  - (−) **forward-pass loop (the real test)**: a fn where `(a get 0)` is typed
    BEFORE the poisoning `a set 0 "x"` in source order, and the runtime values
    DIVERGE per iteration (`1` then `"x1"`). Result: **compile == interpret =
    `[1,"x1"]`** — SOUND. The poison on `a` reached the armed `get` (→ Any →
    gradual add → correct polymorphic runtime dispatch) despite the set being
    later in source. **Cross-pass poison accumulation reaches the armed pass —
    Q1 resolved.**
  - (−) **single-pass top-level forward case**: interpret `[1,"x1"]`,
    force-compile REFUSES (`code-body word each`) — no miscompile, but the
    refusal is a DEFECT, not a result to bank: valid code that a developer
    expects to compile, and it does not. Owed a fix and tracked to closure.
    Across EVERY test: zero miscompiles.
  - **Conformance rule corrected**: the predicate must TRUST THE BOUND
    (`valT.ConformsTo(elem)`), NOT reject all dynamic values. An initial
    `!args[1].Dynamic` guard wrongly poisoned `counts` (its increment value is a
    gradual Integer — `dyn=true`, bound Integer) and regressed radix to refusal.
    Gradual Integer conforms; concrete String and gradual Any do not. This matches
    how `ReturnsAddConcat` already trusts bounds.

**Spike verdict: the architecture is validated end-to-end.** Identity (make-site
pos, stashed in `carrierResults`, stamped in `ChildTypeInfo.Child.Pos`),
element-type-rides-the-carrier (Spike A), the payoff (radix compiles), the
soundness need (no-gate miscompiles), the gate logic, the set-observation hook
(carrierResults, not a ReturnsFn), the trust-the-bound conformance rule, and
forward-pass soundness via cross-pass accumulation are all demonstrated.

**IMPLEMENTED (Stage 3, 2026-06-29).** Production landed with:
- Identity = `ChildTypeInfo.ArrayID` (`value.go`), stamped by `make Array`'s
  ReturnsFn from `CheckState.CurCallPos` (stashed in `carrierResults`).
- Poison = `CheckState.ArrayPoison map[SrcPos]bool`, **monotone and branch-SHARED**
  (clone() does not deep-copy it; never rolled back), reset per check run in
  `Begin()`. This resolves Q4 (lifecycle) WITHOUT the spike's package-global:
  monotone-union is the soundness semantics (a non-conforming set on any path
  taints everywhere), and whole-run persistence gives the cross-pass accumulation
  forward-pass soundness needs.
- Observation = `observeArrayWrite` in `carrierResults` (NOT a `set` ReturnsFn,
  per Spike B); gate = `getArrayReturns`. Whole-corpus differential + targeted
  forward-pass / alias / escape regressions all sound.
- Q2 resolved (pos-stash), Q3 resolved (ArrayID field), Q6 (escape) covered for
  the store-as-value and alias cases; conformance trusts the bound.

**Residual item #2 — RESOLVED (2026-06-29) by the strict→gradual switch; the
explicit pre-pass is unnecessary and was rejected for cause.**

The forward-pass concern was a STRICT-era artifact. With `get` returning a strict
`NewCarrier(T)`, a get that fired BEFORE its array's poisoning set (in the single
armed pass — `CompileCheck` is one armed walk, no separate unarmed phase) would
record a committed strict integer op, and a later non-conforming runtime value
would miscompile — so soundness depended on poison being populated first.

The Stage-3 switch to a GRADUAL element (`NewDynamicCarrier(T)`, forced by the
OOB→None soundness fix, §6 of the main doc) removes that dependency entirely: a
gradual element compiles to a GUARDED / polymorphic op, never an unguarded strict
commit. So a get on an array that is only poisoned LATER is still sound — the
guard discharges to the real runtime dispatch (concat for a String, error for a
None) == the interpreter. Poison ordering now affects only PRECISION (gradual-T vs
gradual-Any), never soundness.

Verified by an adversarial battery targeting the EXACT hole — a get-result feeding
a strict consumer (arithmetic, `:Integer` fn param, native array index, mul/sub),
plus identity-loss escapes (merge via `if`, store-as-value, fn-arg mutation,
closure capture) — all single-pass, all runtime-divergent: ZERO miscompiles
(clean-sound or identical error-parity). The structural cases (forward-pass loop,
arg-mutation, capture-mutation) are pinned as permanent regressions in
`bytecode_array_element_test.go`. Soundness now rests on the gradual-modality
guard invariant, which is independently gated by `verify-bytecode`'s property-
differential fuzz (`TestPropertyDifferential`).

Why NOT the explicit unarmed pre-pass: `CompileCheck` is a single armed pass that
deliberately PERSISTS check-mode side effects (def/import/macro RunInCheckMode).
A second (unarmed) walk to pre-populate poison would re-apply those side effects
(re-mint types, re-run Test specs) unless separately sandboxed — a real soundness
risk introduced to close a hole the gradual result already closes. Not worth it.

**Other residual items (NOT blockers — engineering):**
1. **Lifecycle (Q4).** The spike used a package-GLOBAL poison map; production must
   use a `CheckState` field, branch-cloned/reset like `ContextTypes`
   (`check.go:37,84`), so poison does not leak across sibling fns / branches /
   programs. Ordering (Q1) is proven; scoping is the remaining correctness work.
2. **Guarantee the accumulation precedes the armed read.** Forward-pass soundness
   held in all tests, but it relied on poison being set before the armed `get`
   reads — provided incidentally by the existing multi-pass re-analysis, and in
   the single-pass case only by a refusal, which is itself an open defect owed a
   fix and no guarantee to lean on. The production design should GUARANTEE this
   with an explicit unarmed accumulation pass before the armed compile arm (not
   rely on incidental re-analysis), so no single-pass armed-only fn can ever read
   a stale strict type. No miscompile was observed, but make it structural.
3. **Identity field.** Use an explicit `ChildTypeInfo.ArrayID` rather than
   overloading `Child.Pos`; keep the one-line `carrierResults` pos-stash (or
   thread pos into `ReturnsFunc`).
4. **Escape detection (Q6)** still to be built with its negative-test battery.

Stage 1 (representation) is now a real, low-risk landing: every load-bearing
unknown is resolved; what remains is scoping, an explicit pre-pass ordering
guarantee, and escape detection — all ordinary engineering with the soundness
model proven.

## 7. Fallback if carrier-borne identity proves unworkable

If `ArrayID` cannot ride the carrier through capture (Spike A red): key
`ArrayElems` by BINDING NAME, and extend `stepWord` (`engine.go:2059`) to stamp
`DynFrom` (or a new `BoundName`) on typed-array carriers even when non-dynamic, in
check mode only. Then get/set read `args[i].DynFrom`. This is strictly more
fragile than ID-keying (aliasing `def b a` must be treated as escape-both rather
than handled, and shadowing names need frame discrimination), so it is the
fallback, not the default. Both approaches share §2/§3 unchanged above the
identity channel.
