# REFUSAL-CLOSURE.0 — compiling the remaining refusal shapes

Status: DESIGN (2026-07-15; **revised 2026-07-16** after an adversarial
review — reason strings, op names, and mechanism preconditions corrected
against HEAD; a §9 inventory added; the closure claim scoped honestly.
§6a **landed** 2026-07-16 with the review's required net-zero gate; §5
**blocked**, reclassified to a check-mode change — see the sections).
The runtime-independence program's ratchets sit at their finish lines
(census 6000/6000 native, refusals 0, bails 0, spec frontier 0
expected-red, public `Run` compiled; the *langspec* frontier compile
ledger separately carries 3 expected-red rows — §9.1, a different
ledger, not a contradiction). What remains is the OFF-CORPUS refusal
envelope — shapes that still return `compile_refused`. Every one of
them is a DEFECT: an unimplemented or unproven case in the compiler,
owed a fix and tracked here to closure. Done is a language that
compiles, as a developer expects — ALL valid code compiles, no
exceptions. The interpreter is NOT a fallback for the compiler and is
not allowed to become one; the runtime path that SILENTLY re-runs a
refused program on it is scaffolding that absorbs a known defect so
the user still gets an answer — and the silence hides the failure
instead of excusing it, so it is never a reason a refusal is
acceptable. This note designs the compile strategy for **each of the
eight families below** (§9 inventories what the eight do NOT cover),
so that each one can be landed. Every mechanism reuses machinery that
already exists and is proven; none requires a new architectural idea.

(Framing corrected 2026-09-16: earlier revisions of this note described
refused shapes as running on the interpreter "by design" and their cost
as "slow, not wrong". That was wrong. They run there because the
compiler has a gap, and the gap is owed a fix.)

The shared soundness rule, unchanged: a shape compiles only when its
compiled execution is BYTE-IDENTICAL to the interpreter (values, error
taxonomy, output, binding state) — until that is proven the case stays
unproven, and its refusal stands as an OPEN DEFECT rather than a
resting place. Each landing must flip the shape's pinned-refusal test
to a compile-parity pin: for §1/§3/§4 those are the
`mustRefuseWithParity` calls in bytecode_edge_findings_test.go (whose
§1 header documents the graduation pattern); §2/§5/§6a are pinned
elsewhere — `zzRefusingRow` (bytecode_effectfence_test.go), the
variadic pin in `TestGlobalBindEnvelope` (bytecode_globalbind_test.go),
and the declining-poly pin (bytecode_flip_divergences_test.go,
re-pointed to the `zpick` fixture at the §6a landing) — which assert
`compile_refused` (and should also pin the reason substring; see §5)
directly. And every landing passes the full battery.

## 1. Wide error-join forward drift — `5 do [7] error ["x"] add 1`

**LANDED 2026-07-16** (`tryRecordDriftWindow`, eng/go/drift_window.go).

Refusal was: "forward operand accounting across a dynamic/island residual
(Stage 3)". The catch result joins to dynamic(Integer|String), so `add`'s
dispatch is value-dependent: the String overload matches all-stack
(consuming the leading `5`), the Integer overload forward-collects `1`.
No static record picks one arm, and the two arms consume DIFFERENT stack
depths — that accounting was the refusal.

**As landed — a STATIC window, no marks.** The doc's from-mark sketch
rested on a stale premise (the §2 lesson again): the window is
fixed-width at record time — [leading residual(s), the ONE catch result,
the word, the forward literal] — because the drift gate itself requires
the matched operands to be a contiguous span directly under the word, and
a fixed window needs no `OpStackMark`. `tryRecordDriftWindow` (hooked
right before `refuseForwardStackDrift`) records one generic evCall event
whose operands are the window laid out top-first — the WORD ITSELF rides
as an inert const (`word(add)`) — lowering to the existing
`OpCallDynamicMixed` with Arg = window width. The VM islands the window
verbatim through `islandRun`: the word token DISPATCHES in the island
with the interpreter's own registry-resolved forward collection over the
LIVE value, so the arm choice AND its residual count are byte-identical
by construction (the Integer pass-through forward-collects → `[5 8]`; the
String handler binds all-stack). The event is variadic-flagged; the
in-order layout machinery promotes the catch result to a frame local and
re-pushes the window in source order.

Gates (each declining shape still refuses — an open defect, pinned):
- TERMINAL only — nothing after the forward literal but End/DefCleanup
  (a downstream consumer would need a static count the island can't
  promise): `… add 1 drop` still refuses.
- BYSTANDER-FREE — no data values below the window (`1 2 3 do … add 1`):
  the in-order reconciliation cannot interleave the window's const
  re-pushes with untouched values.
- No variadic-event operands (fixed width), contiguous matched span,
  every operand with a compiled home.

Landing tests: `TestEdgeFindingForwardAcrossErrorResidual` (both raise
paths compile with parity; both decline fences pinned). The negatives
already pinned (mul/sub/String tokens, no-leading-residual) stay native —
the window fires only where refuseForwardStackDrift fired.

**Scope (per the adversarial review's qualification):** this is the
review's sanctioned narrow-window variant — the island result is
variadic, so mid-statement drift shapes (`… add 1 mul 2`) and
fragment-context drift sites still refuse — unproven cases, still owed
a compile (pinned: `… add 1 drop`). The statement-end window the review
sketched is the widening those shapes are owed.

## 2. Deferred-token dispatch windows — `def f fn [[x:List][List][x]] def m (flex {a:1}) f m.a`

(The reproducer is the full `zzRefusingRow` fixture,
bytecode_effectfence_test.go:94 — `f` must be a defined fn for the
dispatch-recovery window to exist; without the preamble the source fails
as check diagnostics, not this refusal.)

Refusal: "unmatched dispatch recovered at f". **LANDED 2026-07-16** — and
the mechanism turned out SMALLER than the island this section originally
designed. Investigation showed the premise was stale: by dispatch-recovery
time the check pass has already EVALUATED the reach in place (the recorded
poly-dot event, a product of the Phase-4.2 store-shape typing), so the
failed window holds an event-produced DYNAMIC carrier — not a raw Reach
token. The definiteness screen was declining it at its `v.Dynamic` arm,
one line above the Reach screen the refusal was attributed to.

**Mechanism as landed: route dynamics to the runtime rematch.** The
screen now classifies a Dynamic operand alongside carriers
(`v.Carrier || v.Dynamic → hasCarrier`, engine.go): OpDispatchRematch
re-runs the match over the operand's LIVE runtime value — exactly what
the interpreter's dispatch examines, so the static tag (which is all
"dynamic" means) never enters. A runtime no-match raises the
byte-identical rich signature_error (verified: code, detail, position,
notes and help all match); a match defers to the interpreter. The
existing rematch gates still hold the line: a dynamic with no compiled
home fails operand resolution, and a window under a leading stack
residual fails the written-tuple contiguity bound (that shape — the
cell mutated through an opaque fn param behind a residual — still
refuses whole-program, an open defect owed a mechanism, pinned in
TestUnmatchedDispatchTrapNegatives). The zzRefusingRow fence fixture,
the RunCompiledReason pin and the trap-negatives pin were re-pointed to
the §5 variadic-loop-def shape (blocked indefinitely, so stable).

The raw-Reach/ParenExpr/InterpString token screen REMAINS for windows
that genuinely hold unevaluated tokens; if such a shape resurfaces, the
island election originally designed here (a fully-baked OpFallback span
re-step, arming dynEnv so the island resolves names registry-visibly)
is the ready mechanism.

**Scope note (from the adversarial review):** "unmatched dispatch
recovered at `w`" has several decline causes; the landing above owns
the deferred-token one. The others are still-open defects until designed:
multi-overload user fns over an Any/disjunct operand at the no-match
recovery sites (plausibly §6b's stored-sig-table re-match);
predicate/refinement-typed user-fn params, where the guarded CALL_USER
enforces only nominal types (a candidate for re-running the predicate
via the existing RunTypedBind/OpBindTyped machinery); and
tryRecordPoly's own safety gates (meta / fn-value / mutating /
code-body / multi-result words).

## 3. Member-fn auto-apply mid-expression — `m.double 21 eq 42`

Refusal: "member fn value auto-applies mid-expression". The interpreter
applies the parked fn the moment `21` arrives (a stepLiteral
ARRIVAL-loop event, not a word dispatch); the recorder only sees word
dispatches, so the downstream `eq` stole the operand in the record.

**Mechanism: record the arrival-apply as a dispatch event. LANDED
2026-07-16** — via the M2c shaped-method chassis rather than the
OpCallDynamicTrailing seam this bullet sketched. The memberFnRead tag
now carries the pinpointed member VALUE (readFnMemberValue — a concrete
container + concrete key), and tryMemberFnArrivalDispatch
(method_shape.go), hooked beside tryShapedMethodDispatch where the
check pass steps the member-read carrier, claims the member's SINGLE
plain signature's arity of inert tokens (the ARRIVAL window — the
parked fn fires the moment its args arrive, so the next word never
enters the collection) and records a guarded mid-stream OpCallDynMethod
whose runtime value — the real container read's product — drives the
apply (the island path, byte-identical to the interpreter's forward
auto-dispatch). Two implementation notes: the model runs SUSPENDED (a
plain user fn's ReturnsFn records its own CALL_USER through
core_helpers, not the outcome seam — a live probe leaks a phantom event
the lowering cannot seat), and no body unit is compiled (callDynMethod
islands any plain callable; the declared return contract is
engine-enforced at run time, so the modelled out carrier is sound). The
paren-bounded variant (`(m.double 21) eq 42`, previously "fn-value
application bounded by a paren") graduated with it. Decline fences mark
today's still-open cases: computed-key reads (no pinpointed member),
multi-overload members (runtime first-match not modelled), captures,
anonymous members, quote/no-eval params, non-inert windows.

## 4. Computed branch bodies — `if (n eq 0) [99] (range 2 4)`

Refusal: "computed branch arm is a spliced list body" (raised from
lang/go/native/native_control.go:694, prefixed "if: …"). The
interpreter's spliceArg EXECUTES a paren-arrived list as a code body;
the compiled value path would push the list as data.

**Mechanism: the dyn-body island, per arm. LANDED 2026-07-16** — as a
BODY SYNTHESIS rather than a bespoke lowering: computedArmDoBody
(native_control.go) rewrites a COMPUTED List-conforming arm to the
equivalent `[do <arm>]` body and routes it through the ORDINARY arm-body
path (RunCarrierBodyWithDefs + the branch fragment capture), where the
dyn-body machinery already owns the computed `do`. The equivalence was
probe-proven on every axis before landing: multi-values, def leaking
(do's keep-defs ≡ the splice's inline leak), and break/continue (the
FlowCtrl escape — the divergence-1 OpFallback translation makes this
hold in compiled loops). Recording pass only, so plain checks keep the
value-arm surface; dead arms, scalar/paren-scalar arms, def-bound
concrete list arms and quoted arms all keep their prior behavior
(pinned). The 2-arg `if`'s computed arm still refuses ("if:
then-branch not captured") — an open defect owed the follow-on widening;
`case` arms are value-semantics in both engines and stay native.

(The review's filed quoted-break divergence in the adjacent do-body
seam landed independently as the divergence-1 fix + the PR #275
sentinel-scan widenings: valueHasSentinel, the interp/XML/map
recursion, and the transitive callee scan.)

## 5. Variadic loop-collect defs — `def xs (for 3 [1])`

**LANDED 2026-07-17** (SplitLoopRegionBind + the splice-at-depth
OpBindGlobal).

Refusal was: "def `xs` consumes loop results (Stage 2 loops only feed the
program residual)" — the VARIADIC-producer arm of lowerDynBind. The
interpreter binds `xs` to the region's FIRST value (the pending forward
collects the first-ARRIVED value — probe-pinned: `def xs (for 2 [7 8]) xs`
binds 7) and spills the remaining N−1 as residual.

**As landed — no forward-collection change at all.** The doc's "deep,
broad engine change" premise dissolved once the read path was understood
(the stale-premise lesson, a fifth time): reads of the binding do not
need the check pass's carrier model to be per-value faithful, because
they can re-resolve the LIVE registry binding at run time
(OpLookupDynScope) — the splice bind installs the real first value
before any read executes. So the landing is three small seams:

- **Check-mode half** (`SplitLoopRegionBind`, emit.go): a TOP-LEVEL def
  whose value is a STATICALLY-COUNTED variadic loop region (RecordLoop
  now stamps `eventInfo.regionN` = trips × per-iteration net when all
  three bounds are concrete and trips ≥ 1) swaps the binding for a fresh
  ELEMENT carrier; the region carrier itself returns to the check stack
  as the N−1 REST residual — still the loop event's variadic out, so the
  existing disposition owns it unchanged. The element typing is what
  kills the silent-miscompile risk the blocker named: downstream
  dispatch types against the element, not the region
  (`def xs (for 3 [1]) xs add 1` → [1 1 2], pinned).
- **Lowering half** (lowerDynBind + GlobalBindSpec.Splice): the bind
  emits an OpBindGlobal in splice mode — bind `stack[top−(regionN−1)]`
  and splice it out — the region's deepest value at its statically-known
  depth. No marks needed (the from-mark prototype was for the
  runtime-variable case the static gate excludes). Stacked regions
  compose: each bind fires immediately after its loop, so the depth
  spans only its own region (`def a (for 2 [1]) def b (for 2 [5])
  a add b` → [1 5 6], pinned).
- **Read path** (dynScopeRescue): a top-level arm admits reads of
  split-bound names (they have no event/local home by construction),
  lowering OpLookupDynScope; a dispatching binding (a region of fn
  values) is not compiled — the existing dyn-scope-read screens route it
  to the interpreter at run time, an absorbed gap and a case still owed
  a compile.

Gates (each declining shape still refuses — an open defect, pinned):
a DYNAMIC count (the split needs the static region size — the
zzRefusingRow fixture family re-pointed here), fn-body defs (the root
gate), and zero-trip loops (pruned; the def forward-collects the next
token, which already compiled).

Landing tests: `TestEdgeFindingLoopCollectDefCompiles` — the canonical
[1 1 1], no-read [1 1], distinct-values [2 3 1], multi-value-body
[8 7 8 7], typed downstream dispatch, stacked regions, double reads,
const-resolved and range-form counts, the dynamic-count refusal, and
the branch-arm read (error parity). The former refusing pins
(zzRefusingRow, RunCompiledReason, TestGlobalBindEnvelope,
TestEmitRefusals) flipped or re-pointed to the dynamic-count sibling.

## 6. Poly-decline arms (fn-predicate / gradual-Any overloads)

tryCompileUserPolyArms declines for MORE than two reasons; the full
reachable set (user_poly.go): zero committed returns, the body-local
fn-baseline gate, non-identical declared returns across arms
(userPolyArmShapeOK — probe-verified reachable:
`def id fn [[x:Any][Any][x]] def g fn [[a:Integer][Integer][a]
[a:String][String][a]] g (id 5)` refuses "gradual-Any arg to
multi-overload user fn `g`"), quote/type-literal/no-eval/raw/form param
slots, anonymous/macro/captured owning defs, deferred-param-list arms,
and variadic-returning arm units — plus defensive gates (<2 same-arity
arms, nil aggregate) that are non-shapes. This section designs the first
two; the others need mechanisms (the differing-returns case could type
the call's residual as the dynamic join of the arms' declared returns —
the §1 machinery) before the envelope closes. An entry in §9 records
such a gap; it does not discharge it. The gradual-Any probe above
should be pinned in bytecode_edge_findings_test.go.

- **§6a: zero committed returns** (`len(committedReturns) == 0` — the
  zero-return overload set). **LANDED 2026-07-16.** The poly gate's
  `len(committedReturns) == 0` bar is dropped: an empty committed contract
  is admitted, `userPolyArmShapeOK` already matches Returns position-wise
  (0 == 0 keeps the arms consistent), and a new per-arm `unitNetsZero`
  gate requires every arm's body to net exactly zero residual values — so
  the recorded 0-output `OpCallUserPoly` is byte-identical to whichever arm
  the VM's runtime re-match selects. (The gate is load-bearing: a
  declared-`[]` arm pushes no ReturnCheck, so a body that nets values
  flows them verbatim to downstream consumers — `def f fn [[x:Integer]
  [] [x add 1]] f 1 add 1` → 3 — which a fixed 0-out call site cannot
  carry.) `buildFnBodyReturnsFn`'s 0-residual path records the poly call
  and returns nothing; anonymity stays refused by `findOwningFnDef`'s
  `owner.Anonymous` gate. A declared-`[]` arm whose body leaves a
  RESIDUAL (the "residual IS the result" shape) fails `unitNetsZero`, so
  that set keeps its refusal (the `pick`/`zpick` fixture). The `shout`
  fixture (`TestPredicateOverloadDispatchCompiledParity`) now compiles
  with output parity (`"o\n"`, two `CALL_USER_POLY`); the declining
  fixture was re-pointed to `zpick` to keep `planUserPolyDispatch`'s
  refusal arm covered.
- **Body-local multi-overload fns** (the fn-baseline gate: the runtime
  Lookup cannot resolve a name popped before the VM runs). **LANDED
  2026-07-16.** UserPolyRef gains a stored dispatch table
  (UserPolyRef.Sigs — the arm signatures frozen at record time);
  matchUserPoly's stored mode re-matches over the frozen subset with no
  live Lookup and no index/Impl drift guard (there is no live table to
  drift from). The doc's "binding cannot change" premise needed one
  repair, found by probing: DYNAMIC-SCOPE mutation — a callee whose own
  body rebinds the same name overlap-replaces the local in place, and
  the replacement SURVIVES the callee's teardown (interpreter
  semantics, pinned at [141] in TestBodyLocalMultiOverloadPolyStored's
  mutator negative) — so the freeze gates on Check.FnBinders: when any
  OTHER fn binds the word as a body-local, the freeze is unsafe, so
  that shape still refuses — an open defect the interpreter absorbs at
  run time, not a home for it. Pins: both arms dispatch by runtime value
  through one compiled program, a no-match defers to the interpreter's
  canonical signature_error, and the module-scope live-Lookup mode is
  untouched.

  (The review's filed single-overload analogue — body-local conditional
  shadowing baking the shadow — landed as the CondBodyDepth
  conditional-fn-shadow refusal, PR #275 divergence 2.)

## 7. Per-callback stamp declines (not whole-program refusals)

- **Runtime-minted capture values** ("closure captures a runtime-minted
  value (no compile identity)"): the identity gate exists so the
  freshen/share machinery can't misclassify. **LANDED 2026-07-16** — and
  simpler than the const-bake this bullet designed: for a DETACHED unit
  the capture is per-ref and frozen, so StampDetachedFn mints a fresh
  identity on a CLONE of the captured slice (confined to the fork's
  compile; the published value stays untouched) and the existing capture
  slot path keys it like any ID-bearing capture — no emit change at all.
  The whole-program compile keeps the identity gate (there the
  freshen/share concern is live). Pinned in TestStampDetachedFnShapeGates
  (eng) and TestComputedCaptureStampsAndRunsWithCaptures (lang,
  end-to-end with per-value refs + interpreter parity).
- **Multi-overload fn values**: **LANDED 2026-07-16** — and no
  sig-table dispatch mode was needed (a stale-premise reduction again):
  the invoke seam already dispatches through MatchFnSig BEFORE
  InvokeCallback, so the matched sig's own `Impl.Compiled` ref IS the
  sig table. As landed: `storedSigEligible` replaces the
  single-own-sig gate (per-sig: own, boru body, non-empty,
  sentinel-free); the compile-time store-fn bake loops every stampable
  sig (per-sig unit + ref, `compileStoredFnUnit(fd, sigIdx, pos)`);
  the runtime path gains `StampDetachedSig(r, fd, sigIdx, pos)` with
  per-sig deps and a per-sig §7c restamp box, and
  StampFnValue/StampFnValueInPlace loop all stampable sigs (partial
  success is per-sig: a declining sibling stays plain and is left to
  the interpreter at its own matches — that sibling is an open defect
  the stamp pass has not closed). Pins:
  TestStoredFnMultiOverloadStampsPerSig (compile-time, incl. the
  sentinel-declined sibling), TestMultiOverloadHandlerStampsPerSig +
  ...PartialStamp (end-to-end service dispatch: the two-arity handler
  shape that never stamped, both units on the VM, parity), and the eng
  stamp-gate/report pins flipped to per-sig.
- **Stale-dep refs degrade permanently to CallBoru**: **LANDED
  2026-07-16.** InvokeCallback, on depsFresh failure, re-runs
  StampDetachedFn against the live bindings via a per-ref restampBox
  (CompiledFnRef.restamp — allocated by StampDetachedFn only, so
  compile-time refs keep the interpreter path): the box carries the
  stamp inputs (the §7a-cloned fd + pos) and the current re-stamped twin
  under a mutex serialising concurrent invokers of one shared sig; each
  re-stamp snapshots the new generations, a stable rebind pays ONE
  compile then runs the VM again, and restampMaxTries (3) bounds a hot
  rebinding loop — after the budget the seam stays on CallBoru, the
  interpreter path: a tracked degradation the runtime absorbs, not an
  acceptable resting state. Pinned in TestInvokeCallbackJITRestamp
  (freshen-to-live-value parity, twin reuse without recompile, budget
  exhaustion, disarmed decline). This is also the mechanism for the
  plan's Phase-6 "JIT detached-unit cache" item.

## 8. boru-written mini compile hooks — the hardest open case

A boru compile hook is a macro whose check-time expansion is
CONTRACTUALLY not the runtime expansion (MINILANG.5.md §13): the hook
may read state that exists only at runtime. Both compile strategies
known today are unsound or self-defeating: baking the check-time
expansion violates the contract, and a runtime JIT of the hook + re-step
of its expansion is exactly the interpreter with extra steps. That makes
this the HARDEST OPEN CASE in the envelope — not an exemption from it.
The refusal is still a defect against the contract that all valid code
compiles, and what it is owed is a mechanism (a hook contract the
recorder can see, or a runtime expansion the VM can execute), not a
permanent exemption. (Framing corrected: this section previously
recorded the shape as a DESIGNED opt-out, "recorded, not scheduled".)
(Go hooks compile since fa9e844; the non-concrete src/opts cases still
refuse: the record cannot see the values the runtime expansion would
consume.)

## 9. Inventory — what §1–§8 do NOT cover

The eight families above were derived from the raise-site inventory
(grep MarkUncompilable / refusal-reason strings across eng/go/emit.go,
lower.go, engine.go, carrier.go, core_helpers.go and the lang natives).
The following LIVE shapes are outside them; each needs a mechanism or
an unreachability argument before the envelope can be called closed.
Where an audit files a site as a "keep", read it as NO KNOWN MECHANISM
YET: the entry records a gap nobody has solved, and never licenses
one.

### 9.1 The langspec frontier's three expected-red rows (L-DO part 2)

The langspec frontier compile ledger (frontierCompileLedger,
test/go/langspec/frontier_spec_test.go) pins three live refusing rows:
two def-msg do-catch rows refusing **"residual shape beyond Stage 1"**
(the seatResults raise family, emit.go:6808 — call-result-above-a-
literal / results-reordered / unconsumed-call-results) and one
module-export-in-variadic-region row refusing **"residual value not
statically materialisable"** (emit.go:6569). Their design belongs to the
completion plan's Phase-5 "L-DO part 2b" (see
RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md); those landings flip the
frontierCompileLedger rows. Until then the closure claim excludes them.

### 9.2 Probe-verified expressible shapes — RESOLVED 2026-07-17

- **9.2b Splice over a computed payload** — **LANDED** (b0689cb8):
  RecordSpliceDyn / OpSpliceDyn spreads a DATA payload verbatim and
  DEFERS a code-bearing one to the interpreter; top-frame only.
- **9.2c Interpolated XML with a runtime-computed part** — **LANDED**
  (00de15df): OpInterpXml, the tree twin of OpInterp — buildXmlFromTmpl
  collects holes in traversal order, rebuildXmlFromTmpl re-assembles.
- **9.2d Curried-factory body provenance** — **LANDED** (944e0c65):
  tryReturnedClosure now admits any NAMELESS fn value (the `=>` form
  already compiled; the verbose `fn` form just lacked the Anonymous
  flag). Three-level currying remains open (§9.4 emit.go:3068).
- **9.2e Paren-bounded LEADING fn-value application** — **LANDED**
  (ba3cca94): a guarded OpCallDynMethod at the paren boundary (the §3
  chassis) gated on member-read provenance; the window collapses to one
  carrier before any trailing op. Unmasked + fixed a latent service
  capturing-handler miscompile (CompileFnHandlerStrict).
- **9.2f Surface-shape typed dispatch** — CONFIRMED already compiles:
  the documented fixture (`def g gen [(T extends Comparable)] …`)
  compiles with parity; the engine.go:7956 refusal is the S2 GENERIC
  SURFACE CALL (`g (make Circle {})` over an exposer op) — a DIFFERENT,
  still-open shape (§9.4 open list). Pinned in TestS9SurfaceTypedDispatch.
- **9.2a Loop-carried store of a variadic result** — **DESIGNED
  FOLLOW-ON**: the §5 first-value split one frame down, but the
  splice-at-depth bind is stack-top-relative while a loop-body fragment
  needs FRAGMENT-base-relative depth (the naive splice miscompiled
  [5 0 5 0]). Needs a fragment-relative splice + the spilled rest
  flowing as the enclosing loop's variadic body — its own landing.
  Pinned refusing with parity (TestS9LoopCarriedVariadicStore).
- **9.2g fn/afn construction over a computed operand** — **OPEN,
  DEFERRED**: the param PATTERN is the runtime value, so a compiled
  unit bakes the check-time carrier; the known mechanism is a
  per-evaluation FnDefInfo re-construction op, unbuilt because the
  shape is so exotic (§8-class). The refusal is a defect until it
  lands. Pinned refusing (TestS9FnComputedOperand).

### 9.3 Residual guard-owned declines

The typed-def RecordTypedBind decline arms (native_definition.go:646
dynamic-refinement reparent, :717 fn-predicate bind, :998 DepScalar
validation — the first two pinned) and the shaped-method guards
(method_shape.go:213 zero-arg landing, :482 operand of unknown
provenance — both pinned) are still absorbed, silently, by the
interpreter at run time. AUDITED 2026-07-17: the audit filed the
RecordTypedBind arms and method_shape.go:482 as DESIGNED-KEEP —
permanent refusals. The classification is retired here; nothing is
designed to stay refused. What those entries actually record is the
difficulty, and the difficulty is real (no compile home for the
operand's provenance / a validate-reparent the runtime OpBindTyped
already mirrors where compilable), so read them as NO KNOWN MECHANISM
YET — open defects owed one, exactly like method_shape.go:213, which
the audit already files as OPEN (a 0-arg member auto-apply before a
non-inert window — a §3 extension). callable_words.go:250's
gradual-Any collection ambiguity is DEFENSIVE-ONLY (every shipping
Callable word carries CompileDynBody or CrossCollectionTokenShape;
pinned white-box in dynbody_unit_test.go). See
design/REFUSAL-CLOSURE-S94-AUDIT.10.md for the full per-site table.

### 9.4 The Stage-2/3 raise-site tail — AUDITED 2026-07-17

A parallel audit classified all **71** MarkUncompilable / refusal
raise-sites across emit.go / engine.go / lower.go / carrier.go /
callable_words.go / core_helpers.go / method_shape.go / user_poly.go.
Verdict split: **5 subsumed, 23 no-known-mechanism (the bucket the
audit labelled designed-keep), 16 defensive-only, 27 open** (a
genuinely-compilable shape with no landed mechanism yet). No hidden
miscompiles surfaced — every open site is a known Stage-3 higher-order
gap or an unknown-provenance residual tail, each refusing today, the
refusal SILENTLY absorbed by a re-run on the interpreter: parity, but
no compiled answer, and a defect still owed one. The groups:

- **subsumed** (5) — a landed mechanism now compiles the shape; the site
  survives only as the mechanism's own decline backstop: emit.go:3026
  (§7a capture mint), emit.go:4193 (§2 rematch / §1 drift), engine.go:3869
  (§9.2b splice), engine.go:4158 (OpInterp), engine.go:4258 (§9.2c
  OpInterpXml). The section-level graduations engine.go:3110 (§1), :3161
  (§3), :6959 (§9.2e) narrow to their non-terminal / non-member residue.

- **no-known-mechanism** (23; filed by the audit as *designed-keep*,
  i.e. permanent — a verdict the corrected doctrine does not allow) —
  the obvious compiled form would diverge and no runtime op yet fixes
  it, which makes each one an open defect waiting on a mechanism
  nobody has designed: context-dependent words (`args`/`__pa`),
  full-stack words (depth/pick/roll), compile-time (check-pass) words,
  container-fn auto-dispatch (miscompile-E belt), frozen-module-rebind
  (§8-class), undef of a loop-carried def, the
  per-call-spine-over-shared-member identity, closure body-count
  taxonomy mismatch, and the operand/capture no-compile-home guards.

- **defensive-only** (16) — unreachable belts (cannot fire without a
  bytecode-level fault); most already carry `//covergate:allow`.

- **open** (27) — the honest remaining frontier, in clusters: the
  higher-order / fn-value-in-body family (emit.go:3124/3157/3176 apply /
  unapplied-fn residual, :4114 anonymous dispatch, :4326/:4336 a fn value
  reaching an invoking native), the if-arm / condition unknown-provenance
  tail (emit.go:2419-2539), the loop-carried fn-rebind (:2843/:2848), the
  unmatched-dispatch recovery sites (engine.go:8219/8286/8351), the
  poly-decline funnel (core_helpers.go:1032/1034 — §6's fn-predicate /
  gradual-Any hazards), the computed-range-list loop (emit.go:4215), and
  the S2 generic surface call (engine.go:7956). Each needs its own
  mechanism + battery; none is a quick win.


**Subsumption (2026-07-21, Stage-3 fn-value dispatch):** the "result
above a literal" arm is PARTIALLY subsumed for the fn-body BODY-TAIL
dynamic-apply shape — a count-mismatched residual carrying a `Dynamic`
value (the boru:fmt stylesheet driver `[nd (rules get (Fmt.kind nd))]`)
now arms the whole-frame replay (`noteDynFrameReplay` widened to
`Dynamic`; `replayForceOrder` re-pushes the out-of-order residual in
token order; `replayIsBodyTail` proves the tail by recorded-trace seq)
instead of refusing at seating. The arm remains reachable — and pinned —
for non-armed shapes (variadic events, multi-result windows, plain
out-of-order residuals the forceOrder bail keeps). The MID-BODY dynamic
apply now refuses earlier, via "unapplied fn-value in body residual"
(the replay's decline), pinned with the graduated shapes by
`TestEdgeFindingDynamicFnValueApplyBodyTail`
(lang/go/bytecode_edge_findings_test.go §6). No pre-existing pin
covered the graduated shape, so no pin flips — the graduation adds
pins (edge-findings §6, fmt_compiled_parity_test.go, module-fmt.tsv
corpus rows).

## Sequencing and gates

Cheapest-first, each with the standard battery + fullcorpus
0-divergence + census ratchets, one landing per commit:

1. §5 loop-collect defs — **LANDED 2026-07-17** (the S5 first-value
   split: SplitLoopRegionBind + the splice-at-depth OpBindGlobal; the
   feared check-mode forward-collection change was never needed — reads
   re-resolve the live binding via OpLookupDynScope; see §5). The
   TestGlobalBindEnvelope variadic pin flipped to compile parity.
2. §6a zero-return poly arms — **LANDED 2026-07-16** (unitNetsZero gate);
   the declining-poly pin flipped, re-pointed to the `zpick` fixture.
3. §2 deferred-token windows — **LANDED 2026-07-16** (the dynamic-operand
   rematch; see §2 for why the island design was not needed). The
   effect-fence pins, RunCompiledReason and the trap-negatives refusal
   were re-pointed to the §5 variadic-loop-def fixture (stable — §5 is
   blocked indefinitely).
4. §7a capture identities — **LANDED 2026-07-16** (the detached-stamp
   capture-clone mint; see §7). §7c JIT re-stamp — **LANDED 2026-07-16**
   (the per-ref restampBox; see §7).
5. §6b sig-table poly — **LANDED 2026-07-16** (UserPolyRef.Sigs stored
   mode + the FnBinders dynamic-scope gate; see §6). §3 arrival-apply —
   **LANDED 2026-07-16** (tryMemberFnArrivalDispatch on the M2c chassis;
   see §3). §4 computed branch arms — **LANDED 2026-07-16**
   (computedArmDoBody synthesis; see §4). §1 drift window — **LANDED
   2026-07-16** (tryRecordDriftWindow — a STATIC OpCallDynamicMixed
   window, no marks needed; see §1). §7b multi-sig stamps — **LANDED
   2026-07-16** (per-sig refs; the matched sig's Impl IS the sig
   table; see §7). Every item of §1–§7 is now landed — §5 included
   (2026-07-17) — leaving the §8 hooks and the §9 inventory open.

After all of §1–§7, **the enumerated refusal families are closed** —
the remaining interpreter execution on any default path would be:
check-mode (the compile front-end itself), module loads (attributed),
const-folds (attributed), explicit RunInterp (a user's own request, not
a compiler outcome), the still-open §8 hooks, and the **§9 inventory**
(the L-DO part-2 residues, the seven probe-verified shapes, the
guard-owned declines, and whatever the §9.4 tail audit does not
retire) — each of the last two a defect the runtime is still absorbing.
The envelope is empty for every expressible shape only once §9 is also
worked off; at that point the `BORU_COMPILE_FALLBACK` hatch plus the
hatched legacy pins (49 at this writing — the authoritative count is
`git grep -c 'Setenv("BORU_COMPILE_FALLBACK"' -- '*_test.go'`; each
landing shifts it) can be retired on schedule, after re-pointing the
stamp-suite pins per §9.2.

The external validation for all of this remains the voxgig-boru sweep
(steps 7–9 re-baseline) in a session sourced from that org.
