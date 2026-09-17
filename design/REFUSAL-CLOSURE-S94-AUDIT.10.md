# REFUSAL-CLOSURE §9.4 raise-site audit (2026-07-17)
Full classification of all 71 MarkUncompilable / refusal raise-sites.
5 subsumed | 23 designed-keep | 16 defensive-only | 27 open.

Framing corrected 2026-09-16. Every site below that can fire on a valid
program is a DEFECT: an unimplemented or unproven case in the compiler,
owed a fix and tracked here to closure. Done is a language that
compiles, as a developer expects — ALL valid code compiles, no
exceptions. The interpreter is NOT a fallback for the compiler and is
not allowed to become one. What keeps the user's answer correct
meanwhile is machinery, not design: `MarkUncompilable` latches the
refusal, `RunCompiled` re-runs the whole program on the interpreter,
and the fallback-island opcode absorbs a refused region — and it does
this SILENTLY, which hides the failure rather than excusing it.
Scaffolding over a known defect, described here so it is not mistaken
for absent. The bucket labels below — *designed-keep* above all —
record WHY a site is still unbuilt and what mechanism it waits on.
Nothing is permanently exempt from compiling, and no label discharges a
site.

## Completeness-review landings update (2026-08-03, PR #327)

The checker-compiler-completeness-review implementation
(design/checker-compiler-completeness-review.0.md §9) landed the named
mechanism — fully or for a bounded first shape — at SIX of the open
sites. Evidence is the flipped/held pins in the suites; every graduation
went through the full gate chain (census 7253 rows, 6872/6872 compilable
rows native, 0 refusals, 0 islands, differential green).

- **`core_helpers.go:1032`-family (gradual-Any multi-overload) — the §6
  join mechanism LANDED** (review §9.11): "type the call residual as the
  dynamic join of the arms' returns" is exactly `userPolyPlan.outs` +
  `recordCallElided`'s poly-alias arm. Differing-declared-returns arm
  sets now compile via OpCallUserPoly (frontier-poly-join rows graduated
  to fn-value.tsv §9; the strict-disjunct twin flipped in
  TestProbeWideningUnionReturnPoly). The site remains reachable for the
  residual-carrying-arm declines (unitNetsZero and siblings) — the
  narrowed §6 residual-typing tail, ledgered with the count-mismatch
  negative in bytecode_poly_join_test.go.
- **`emit.go:3124` (mid-body dynamic apply) — the Stage-G paren-collapse
  LANDED the one-arg leading shape** (review §9.6b): `(g x) add 100`
  compiles natively (TestChainedForwardApplyCompiles). The site remains
  for the `apply`-word mid-body spelling (fnValueM2Refusal pins hold).
- **`emit.go:3157` (unapplied fn-value in fn-body residual) — the
  fn-body dynamic-apply lowering LANDED for the convergent shapes**:
  compose / twice / depth-3 chains / both def-split spellings compile
  (§9.6b/§9.8 — the paren-collapse + the replayIsBodyTail windowReadsID
  arm). The site remains for the CHAINED MULTI-ARG apply (`f (g x y)`,
  TestMultiArgChainedApplyRefuses) — the spellings' collection orders
  diverge beyond one argument, so the remaining shape needs the
  token-order OpCallDynamic chain, not the convergence argument.
- **`engine.go:6959` (leading paren-bounded fn-value apply) —
  RecordDynApply extended to the leading case LANDED for the eligible
  leads** (DynApplyLeadEligible: a Function-typed param/capture slot of
  an open named-param fn unit). The site remains for event-provenance
  and non-member dynamic leads, where the probe evidence (§9.6b) shows
  the convergence argument does NOT hold — those still refuse, open
  cases needing a different argument.
- **The full-stack refusal family — FoldFullStack LANDED** (review
  §9.6a): depth/pick/roll over provably-exact stacks elide statically;
  the family's remaining reachable shapes are ledgered
  (frontier-full-stack.tsv: the event-permuting roll, the closure-unit
  occurrence).
- **The zero-netting `error` handler (post-audit site,
  native_control.go) — the §8.2(6) proven-raise increment LANDED**
  (review §9.13): a strict-Error do-result fixes the arity at zero and
  the dispatch records a 0-output call (stripResidualShapeOK want-0).
  The maybe-raising variable-arity twin still refuses — open
  (frontier-do-error-arity.tsv).

Adjudication note (review §9.14): the open set's residual entries all
retain a NAMED future mechanism and a pinned reachable fixture, so the
audit is CURRENT as a record — every site is tracked, none is untracked.
Tracked is not closed. The review's §8.3 finish line is a language that
compiles as a developer expects: zero divergences; every
runtime-reachable construct COMPILES NATIVELY; and until a defect is
closed, its refusal loud and self-explanatory, never silent and never
reported as success. The audit is complete against that line on the
2026-08-03 tree; the tree is not. Sites below still refuse, and the
runtime's re-run of a refused program on the interpreter is silent.

## Probe-sweep + landings update (2026-07-17, feat/refusal-closure-tail)

A 6-agent worktree-isolated probe sweep re-tested every open site (plus the
three review declines) EMPIRICALLY on HEAD — 4 already-compile (stale
premises), 8 quick wins, 18 confirmed deep. Landed on the tail branch:

- **S9.2a loop-carried variadic store — LANDED** (was designed follow-on):
  CheckState.LoopBodyDepth + the NestedBodyDepth==LoopBodyDepth split gate +
  the analysis-round depth correction (the [5 0 5 0] silent-SetAt root cause)
  + the splitBound multiOut residual.
- **S9.1 rows 1-2 def-over-catch-region — LANDED** (was designed follow-on):
  SplitEventRegionBind, the STATIC-region twin of the S5 split (splice at
  nout-1, sim-entry removal; the compiled catch path defers wholesale).
- **Closure-body trailing apply (emit.go:3176 family) — FIXED as a real
  compile**: the probe surfaced a live off-corpus miscompile ([[1 1]] vs
  [[3 3]]); root cause was a QUOTE-DISCIPLINE mismatch (a compiled local
  carries the stored /r quote verbatim; the interpreter's read strips one
  level). callDynTrailTop now strips the applied copy's quote and
  RecordDynApply fences inline-quoted fns. The shape compiles with parity.
- **emit.go:4215-family computed range START/STEP — LANDED**:
  computedRangeBounds passes bounds as-is; RecordLoop admits const+local,
  refuses events.
- **method_shape.go:213 0-arg landing — LANDED**: an all-0-arg member skips
  the statement-window scan (it never forward-collects).
- **emit.go:4326 (behave case) — LANDED**: behave carries CompileStoresFn.

Stale premises confirmed already-compiling (no change needed):
engine.go:8286-family (Any-operand poly — compiles via OpCallUserPoly; the
site only fires on discarded isolated-analysis passes), emit.go:2848-family
(computed rebind sources all resolve), emit.go:4114 (the documented fixture
compiles; the site is narrower), the quoted-returned-fn review fence
(declining as its author intended — the shape it holds back is still open
and still owed a compile).

Quick-win landings (all committed on the tail branch):
engine.go:8219-family union-RETURN poly LANDED (tryCompileUserPolyArms +
RecordUserPolyCall wired into the strict-disjunct branch); emit.go:2539
both-computed arms over a non-event cond LANDED (lowerBothComputedMatCond +
the widened RecordBranch gate). The one reclassified item is
engine.go:3161 multi-overload member arrival — ATTEMPTED and
RECLASSIFIED DEEP: the probe's mechanism (widen tryMemberFnArrivalDispatch's
single-sig gate) is misattributed — the dot fixture routes through the M2c
tryShapedMethodDispatch whose statement-window scan declines on the trailing
op, and the get-form fixture declines upstream of the §3 arrival gate too;
landing it needs the window model relaxed to consume exactly the shared
arity (the 0-arg precedent) PLUS the multi-sig claim, threaded through BOTH
models. The interp-hole FUNCTION formatting fix LANDED (CompiledFn.Render →
ClosurePayload.Render at OpPushClosure; interpHoleStringifiesUnstably
deleted; the fn-hole pins flipped to compile rows). The other 18 open sites
are confirmed deep.

## open (27)
- `core_helpers.go:1032` — gradual-Any arg to multi-overload user fn `<w>`: ambiguous dispatch, no poly re-match
  The §6 poly-decline funnel for the clusterC (gradual-Any ambiguous) hazard: §6a/§6b subsumed the bakeable arms (those now compile via OpCallUserPoly and never reach here), but this fires when tryCompileUserPolyArms still declines an arm (residual-carrying / differing-declared-returns) — §6 sketches the missing mechanism (type the call residual as the dynamic join of the arms' returns, the §1 machinery).
- `core_helpers.go:1034` — fn-predicate-typed overload dispatch at `<w>` is runtime-evaluated (no poly re-match)
  The §6 poly-decline funnel for the fn-predicate hazard; pinned reachable via `zpick -3` in bytecode_flip_divergences_test.go where the declared-[] arm's body leaves a residual (fails unitNetsZero) so the poly bake declines — the residual-carrying arm is a genuinely compilable shape awaiting the §6 residual-typing / §1-join mechanism, not yet landed.
- `eng/go/emit.go:2419` — if: <name>-branch result of unknown provenance
  A branch arm whose merge value is a computed residual that resolveOperand cannot place (no event, no local, not an inert const) — the §9.4 'branch result of unknown provenance' provenance tail; a future generalised §1-style residual-provenance island seating the arm value would compile it.
- `eng/go/emit.go:2434` — if: condition result of unknown provenance
  A computed condition-body result that resolveOperand cannot place — the same §9.4 unknown-provenance tail as the arm sites; a residual-provenance island seating the condition operand would compile it.
- `eng/go/emit.go:2441` — if: condition of unknown provenance
  A default (non-frag) condition value with no compiled home — the §9.4 unknown-provenance tail; seating the computed condition operand via a future provenance mechanism would compile it.
- `eng/go/emit.go:2483` — if: then value of unknown provenance
  A value-then arm (`if cond X Y`) whose already-evaluated X is a computed residual resolveOperand cannot place — §9.4 unknown-provenance tail; a provenance-seating mechanism would compile it (the plain-const/local/type value-then already compiles).
- `eng/go/emit.go:2521` — if: else value of unknown provenance
  A value-else arm (`if cond [t] Y`) whose already-evaluated Y is a computed residual resolveOperand cannot place — §9.4 unknown-provenance tail; a provenance-seating mechanism would compile it (plain-value else already compiles).
- `eng/go/emit.go:2539` — if: both computed arms need an event condition (Stage 2)
  `if cond (a) (b)` with BOTH arms computed but a const/condFrag/const-cond condition: lowerBothComputed needs the cond as a stack event for its OpReverse+JMP_IF_FALSE select; a widened both-computed lowering that materialises a non-event cond onto the stack (the computedArmCondOK cases already handled for the single-arm case) would compile it.
- `eng/go/emit.go:2843` — loop-carried def `<name>` rebound to a function value (Stage 3)
  A conditional in-loop rebind of a carried name to an fn VALUE: the carried-slot store machinery exists but declines fn-shaped values (isFnValueResidual); a future mechanism storing/dispatching an fn value through the carried frame slot (the store path plus a dynamic-apply over the slot) would compile it — a genuine compilable shape with no mechanism yet.
- `eng/go/emit.go:2848` — loop-carried def `<name>` rebind of unknown provenance
  An in-loop rebind whose value is a computed residual resolveOperand cannot place into the carried slot — the §9.4 unknown-provenance tail applied to the loop-carried store; a provenance-seating mechanism for the store source would compile it (concrete/local rebinds already store via evStore).
- `eng/go/emit.go:3068` — fn <name>: body result of unknown provenance
  After tryReturnedClosure (which S9.2d widened to nameless verbose fns) declines, a curried-factory returning a NAMED / multi-sig / nested-capturing closure still refuses — §9.2 names the mechanism (extend tryReturnedClosure to capturing/nested closures via §7a unpooled-const capture); it also backstops the deferred-residual computed-map divergence — containment for that gap, not a resolution of it.
- `eng/go/emit.go:3124` — fn <name>: apply of a dynamic fn value not at the body tail (Stage 3)
  Pinned reachable (bytecode_fnvalue_m2_test.go: mid-body apply, double apply); §9.4 lists this as a candidate for subsumption under a generalised §1 body-window re-step island — the mechanism is designed but not yet landed, so it is a genuinely-compilable shape awaiting that landing.
- `eng/go/emit.go:3157` — fn <name>: unapplied fn-value in body residual (dynamic apply not compiled in a fn body)
  Reachable from source ((fnv 100) applying a Function-param inside a fn body, pinned in bytecode_clustere_test.go) — the interpreter applies fnv→one value while the compiler leaves [fnv,100] unapplied; a fn-body dynamic-apply lowering (resolveDynamicApply extended to bodies / generalised §1 body window) would compile it.
- `eng/go/emit.go:3176` — closure <name>: unapplied fn-value in body residual (dynamic apply not lowered)
  The closure-body twin ([1 2] each [(x x comp)] leaves [a,b,comp] unapplied) — the comment explicitly names lowering the trailing apply via OpCallDynamicTrailing in a closure body as the follow-on feature, so it is a compilable shape with no landed mechanism yet.
- `eng/go/emit.go:3946` — polymorphic dispatch at <word>
  Fires when tryRecordPoly declines a genuinely-dynamic native dispatch it cannot yet lower — notably multi-result poly (len(outs)>1, needs per-result seating) and non-core/usurp-wrapper poly (pinned reachable in specgen fail-compile tsvs); a multi-result OpCallNativePoly with per-result residual seating is the un-landed mechanism (§9.4 tail).
- `eng/go/emit.go:4114` — anonymous function dispatch (Stage 3)
  word=="" is a runtime fn-value / usurp-wrapper dispatch (the callee is a runtime value) — genuine Stage-3 higher-order that no landed §-mechanism covers (§3 handles member-fn arrival, RecordDynApply the trailing apply); closing it needs a runtime callee-dispatch op (island the value-call like callDynMethod) generalised to bare anonymous callees.
- `eng/go/emit.go:4215` — for: computed range list (Stage 2 follow-on)
  a for-loop over an OpMakeList-assembled runtime range diverges under the CALL_NATIVE for-handler; no §-mechanism covers it (literal/local ranges lower via OpForSetup) — closing it needs an OpForSetup-style lowering that consumes a runtime-assembled range list.
- `eng/go/emit.go:4326` — function-valued operand at <word> (Stage 3)
  a fn-typed operand reaching a fn-INVOKING native (not introspect/store-fn/fn-inert) is Stage-3 higher-order that declined the closure/dynbody/dynapply paths upstream; closing it needs the closure/OpPushClosure path extended to the invoking-native operand slot (the §7a/§9.2 higher-order tail).
- `eng/go/emit.go:4336` — function value reaches <word> (Stage 3)
  the FnDefInfo-value mirror of :4326 — a concrete fn value reaches a non-inert invoking slot; same Stage-3 higher-order gap, subsumable by generalising tryReturnedClosure / the store-fn stamp to the invoke slot rather than a permanent divergence.
- `eng/go/engine.go:3110` — forward operand accounting across a dynamic/island residual (Stage 3)
  §1's tryRecordDriftWindow (hooked immediately before this refuseForwardStackDrift raise) now compiles the terminal, bystander-free drift window, but this raise stays reachable for the deliberately-excluded narrower shapes (non-terminal `add 1 drop`, leading-bystander `1 2 3 do...add 1`, both pinned at bytecode_edge_findings_test.go:112-117); the future landing is the statement-end variadic-absorbing window §1 sketches.
- `eng/go/engine.go:3161` — member fn value auto-applies mid-expression (fn-value-call boundary, Stage 3)
  §3's tryMemberFnArrivalDispatch subsumes the single-plain-signature arrival-apply (pinned graduated in TestEdgeFindingMemberFnApplyMidExpression), but this raise stays reachable for a MULTI-overload member read whose runtime first-match the arrival model cannot claim (pinned refusing, same test) plus computed-key/anonymous members — needs a §6b-style runtime first-match over the member's frozen sigs.
- `eng/go/engine.go:6959` — fn-value application bounded by a paren (dynamic value precedes args)
  §9.2e landed the leading paren-bounded apply for a memberFnRead carrier (guarded OpCallDynMethod), but this raise fires precisely for a leading dynamic that is NOT a member read — a def-bound anon fn read `(mk 7)` or an opaque computed value — which needs RecordDynApply extended to the leading case (or a §1-style mark-bounded paren-window island).
- `eng/go/engine.go:7956` — surface-shape typed dispatch at <w>
  The S2 generic surface call (`g (make Circle {})` over `gen [(T extends Shape)]`) has no landed mechanism; §9.2 lists it as needing runtime re-match over the exposer's registered op (the §2/§6b precedent) — that re-match is what the shape is owed; an opt-out is not on offer, nothing is exempt from compiling.
- `eng/go/engine.go:8219` — unmatched dispatch recovered at <w>
  The strict-disjunct-partition branch: reached when tryRecordPoly (not native-poly-safe) and tryRecordRecoveredUserFn (multi-overload) both decline for a multi-overload user fn over a disjunct operand — the §2 scope-note names this as awaiting §6b's stored-sig-table re-match at the no-match recovery site (no OpDispatchRematch trap is attempted in this partition branch).
- `eng/go/engine.go:8286` — unmatched dispatch recovered at <w>
  The Any/disjunct-carrier branch after tryRecordPoly, tryRecordRecoveredUserFn AND the §2 tryRecordUnmatchedDispatchTrap all decline — a multi-overload user fn over an Any-typed operand whose guarded CALL_USER would bake one overload where the interpreter runtime-dispatches a sibling; the §2 scope-note flags multi-overload-over-Any (Cluster C) as the plausible §6b stored-sig-table follow-on.
- `eng/go/engine.go:8351` — unmatched dispatch recovered at <w>
  The imprecise-carrier fall-through after the trap and native-poly recovery decline — the pinned `dynamic under a stack residual` shape (bytecode_findings_test.go:3965-3967) where OpDispatchRematch's written-tuple contiguity bound cannot prove a contiguous window slice; the §2 island (a mark-bounded residual window) is the ready future mechanism for the cell-behind-a-residual shape.
- `method_shape.go:213` — shaped 0-arg method landing not modelable at <name>
  A genuine-0-arg member fn whose auto-apply landing sits before a NON-INERT window (a raw Word, not the inert-token arrival window §3 models); §3's arrival-apply landed only for the single-plain-sig inert-token window and lists non-inert windows as a decline fence — pinned reachable in method_shape_zeroarg_test.go; the mechanism (model the landing then re-step the following word over the dynamic result) is a §3 widening not yet landed.

## subsumed (5)
- `eng/go/emit.go:3026` — closure captures a runtime-minted value (no compile identity)
  §7a (StampDetachedFn capture-clone mint) now compiles the computed-capture handler shape — pinned gone in TestComputedCaptureStampsAndRunsWithCaptures; the site survives only as the whole-program-compile freshen/share belt §7 still carries (positional capture-slot numbering can't skip an ID-less slot there) — an unbuilt case, not an exemption.
- `eng/go/emit.go:4193` — dynamic input at <word>
  §2 dynamic-operand rematch (OpDispatchRematch) and §1 drift-window now compile the dynamic-operand shapes at their own seams (tryRecordDriftWindow, the rematch screen) before RecordCall; this arm survives only for a dynamic carrier with no proven inputs, no shuffle exemption, and no compiled home — the narrowed unresolvable residual.
- `eng/go/engine.go:3869` — splice over a computed payload (runtime spread unknown at compile time)
  §9.2b landed RecordSpliceDyn/OpSpliceDyn (commit b0689cb8, pinned TestSpliceDynComputedPayloadCompiles); this raise is now only the mechanism's own decline backstop, reached solely when the payload operand has no compiled home (unresolvable dynamic carrier).
- `eng/go/engine.go:4158` — interpolated string with a runtime-computed part
  RecordInterp/OpInterp compiles the dynamic-hole interpolation; this raise is reachable only for the narrower holesOK-false residue (a hole yielding 0 or >1 values, so no single operand-stack slot per hole), which the mechanism's precondition does not yet cover.
- `eng/go/engine.go:4258` — interpolated XML with a runtime-computed part
  §9.2c landed RecordInterpXml/OpInterpXml (pinned mustCompileWithParity in bytecode_xmlinterp_test.go); this raise is reachable only for the same narrow residue the string sibling has — a 0-or-many-valued hole (holesOK false), pinned refusing at bytecode_xmlinterp_test.go:43-50.

## designed-keep (23)
The label is this audit's name for a site where the obvious compiled
form would diverge from the interpreter and no runtime op fixes it yet.
Each entry records why the site is still unbuilt and what it waits on.
They are open defects with no known mechanism — never sanctioned
outcomes, and never permission to leave a shape refused. (The single
entry that is not a defect is emit.go:4117: a compile-time word runs
during the check pass and has no runtime shape to compile at all.)

- `carrier.go:4174` — fn body analysis error in <name>: <err>
  runFnBodyOnce refuses when an ARMED recording's body analysis errors (a mere check-mode imprecision like `get` on an element carrier); the unit would close EMPTY and the VM's empty closure raises `body produced no result` where the interpreter succeeds, and no runtime op reconstructs a body the compile front-end could not analyze — so the shape stays refused and the runtime absorbs it on the interpreter; the defect is the front end's inability to analyze the body.
- `core_helpers.go:154` — fn '<name>' redefined inside a conditional body (branch/loop) shadows an outer overload
  The CondBodyDepth conditional-fn-shadow refusal (PR #275 divergence 2): a drop-then-push inside an if/case/loop arm leaves def depth UNCHANGED so the depth-growth rollback structurally cannot revert it, baking a shadow the interpreter drops when the branch isn't taken — pinned with mustRefuseWithParity in bytecode_edge_findings_test.go / bytecode_stage2_loopcarried_test.go.
- `eng/go/emit.go:1524` — fn body literal embeds an enclosing binding's container (per-call spine identity over a shared member)
  Reachable and pinned (TestPR225P1Refusals, `def c [9] def mk fn [[] [List] [[c]]]`): the interpreter builds a per-call-fresh outer spine wrapping a SHARED member instance, which neither OpPushConstFresh (deep clone) nor a pooled shared const can model; §9.2 lists a selective spine-only freshen only as a hypothetical, and no runtime op reconstructs this mixed identity yet, so it stays refused — an open defect whose mechanism is unbuilt, not a resting place.
- `eng/go/emit.go:2094` — module binding <name> rebound after a fn unit baked its value
  Reachable and pinned (frozen_module_read_test.go): a fn/closure UNIT froze a concrete module binding as a const or spliced tokens while the interpreter re-resolves the live name per call; a later module-scope rebind makes the frozen unit diverge, and re-resolving per call would just be the interpreter — the §8-class shape, which REFUSAL-CLOSURE §8 now records as the hardest open case in the envelope rather than an opt-out from it.
- `eng/go/emit.go:2865` — undef of the loop-carried def `<name>` (Stage 3)
  An `undef` of a name an active armed loop carries: the interpreter's undef exposes the PREVIOUS binding while the compiled carried slot still holds the rebound value, so compiled reads would diverge; no runtime op yet reconciles the popped-binding-vs-live-slot split without re-running the interpreter's scope semantics — so the shape stays refused: an open defect, not a permanent exemption.
- `eng/go/emit.go:3142` — closure <name>: body value count differs from declared returns
  A closure (each/scan/…) body count-mismatch must raise the higher-order word's OWN taxonomy (each_error "body produced no result"), not RET's type_error — a compiled RET as lowered today would diverge on error taxonomy, so the shape stays refused and the interpreter absorbs it (the §1/§4 closure-refusal doctrine); what it is owed is a compiled RET that raises the higher-order word's own taxonomy.
- `eng/go/emit.go:3344` — fn call operand of unknown provenance
  An arg to a compiled user-fn call with no producing event / frame local / const home (pinned reachable across tier_probe/ljoin/modinstance tests) — the value has no compiled home under today's provenance model and no runtime op supplies one, so the shape stays refused and the interpreter absorbs it; it waits on the same residual-provenance mechanism as the unknown-provenance tail.
- `eng/go/emit.go:3352` — capture <name> of <fn> unreachable at a call site
  A closure capture unreachable from the call site has no compiled home under today's model (the interpreter absorbs that shape at run time, per the RecordUserCall doc comment); currently exercised only white-box, but it sits in this bucket identically in kind to the 3344 operand-provenance guard — the same open provenance defect.
- `eng/go/emit.go:3393` — fn call operand of unknown provenance
  The RecordUserPolyCall twin of 3344 — an operand with no compiled home in the committed multi-overload poly dispatch; the poly path pre-gates operands so it is only hit via the emit_seam7 white-box synthetic carrier, but the property it guards (no static provenance) is the same unbuilt provenance case as 3344 — absorbed by the interpreter, not settled.
- `eng/go/emit.go:3566` — for: body nets multiple values per iteration
  Narrowed by the net-drivers landing to ONLY Function-bearing multi-value loop regions (pinned in vary_differential_test.go): a parked Function auto-applies across iterations when a later value lands above it, so verbatim accumulation would diverge — the shape stays refused while no runtime op models that auto-apply; an open defect, not a settled one.
- `eng/go/emit.go:3572` — for: body result of unknown provenance
  The multi-value-branch operand-provenance guard, pinned reachable in bytecode_loop_provenance_test.go (a module-scope-def'd Module instance in a loop body has no producing event / frame local / const home) — no compiled home under today's provenance model; the interpreter absorbs it at run time, which contains the defect without closing it.
- `eng/go/emit.go:3596` — for: body result of unknown provenance
  The single-value-branch twin of 3572, pinned reachable in the same bytecode_loop_provenance_test.go single-value case — the loop body's sole result value has no static provenance today; the same open provenance defect, absorbed by the interpreter.
- `eng/go/emit.go:4117` — compile-time word <word>
  sig.runInCheckMode() words execute during the check pass itself (the compile front-end); baking them would re-run compile-time-only handlers at VM time, so there is no runtime shape here to compile — a front-end fact, and per the review's §8.3 not a refusal that bears on the compile-everything line.
- `eng/go/emit.go:4123` — full-stack word <word>
  a GoImpl.FullStack handler receives the entire resolved stack (depth/pick/roll); its behaviour is a function of the whole live stack the VM does not present as operands, so no CALL_NATIVE operand layout reproduces it today — and FoldFullStack (review §9.6a) already compiles the provably-exact-stack cases, so what is left here is an open tail, not a by-design refusal.
- `eng/go/emit.go:4137` — fn value read from a container auto-dispatches (Stage 3)
  a get-family read surfacing a 0-arg-satisfiable fn member auto-dispatches in the interpreter while the VM would push it as inert data (miscompile mechanism E); the annotated shaped-method read is exempted to tryShapedMethodDispatch, so what remains is the receiver-signal belt that still refuses — containment for miscompile mechanism E until the auto-dispatch is modelled, not a shape exempt from compiling.
- `eng/go/emit.go:4145` — context-dependent word <word>
  args/__pa read the interpreter's per-call args stack, which the VM's CALL_USER frame deliberately does not maintain (it binds params to frame locals); a compiled body reading args would fault, and no runtime op restores the abandoned args stack today, so the shape stays refused — closing it means a frame model that serves `args`, not an exemption.
- `eng/go/emit.go:4171` — code-body word <word> (Stage 2)
  a code-body word that splices onto the tape (CompileExecutesBody), or re-runs a name-referencing body in a sub-engine (execBodyRefsNames) diverges because the sub-engine resolves against the registry while the compiled context holds the name as a VM frame local; the isolated-frame / pure-inert-data cases are already exempted, so the remainder still refuses — an open defect awaiting a sub-engine whose name resolution reaches the compiled frame, not a permanent exemption.
- `eng/go/emit.go:4190` — quoted-operand word <word>
  an uncovered implicit-quote operand (usurp / force-arity / ref-family) has its quoted result re-stepped by the engine as dispatch-manipulating meta; get/getr/set and inert-atom module natives are already exempted, so the residual is a meta-word case no CALL_NATIVE honours today — still open, still owed a mechanism for dispatch-manipulating meta.
- `eng/go/emit.go:4202` — unannotated or opaque word <word>
  a dynamic (untypeable) OUTPUT means the checker could not type the word and the recorded signature is a guess, not a proof — baking it would commit a best-guess overload; forceDynOut already admits the proven declared-Any case, so the un-proven remainder still refuses — a case the checker cannot type YET, owed a proof rather than a guess.
- `eng/go/emit.go:4363` — capturing handler stored at <word> (validated as a function value)
  a CAPTURING handler at a STRICT store slot (CompileFnHandlerStrict — service/add validates+dispatches the value as an FnDefInfo) cannot stamp and would fall to a bare OpPushClosure the native rejects (the §9.2e paren-apply factory-body miscompile); the shape stays refused and the interpreter absorbs it per callback — a stamping gap for capturing handlers at strict slots, still open.
- `eng/go/emit.go:4472` — operand of unknown provenance or not statically materialisable at <word>
  resolveOperand (plus the inert-fn/closure fallbacks) exhausted every way available TODAY to give the operand a compiled home; with no static provenance the value cannot be lowered, so the whole program refuses here and the runtime silently re-runs it on the interpreter — the terminal catch for the unknown-provenance defect, never its resolution.
- `eng/go/emit.go:4498` — fn value read from a container auto-dispatches (Stage 3)
  the RecordPolyCall (poly-path) mirror of :4137 — same container-read fn-value auto-dispatch divergence (interpreter invokes on landing, VM pushes as data), annotated shaped reads exempted; the receiver-signal belt still refuses on the poly seam, guarding miscompile mechanism E until that auto-dispatch is modelled.
- `lower.go:1126` — for: side-effect loop result is consumed (Stage 3)
  A zeroOut side-effect loop whose empty result is consumed by `def x (for …)` or fed as an operand: the interpreter's forward-collection over the empty producer GRABS THE NEXT TOKEN, which a compiled 0-value loop cannot replicate today — §9.4 files this consumed-side-effect-loop in the designed-keep bucket per §5, i.e. unbuilt with no named mechanism yet (distinct from §5's top-level variadic-collect def, which landed).

## defensive-only (16)
These sixteen cannot fire on a valid program — they are internal
consistency belts, reached only white-box or argued unreachable — so
they are not defects against the compile-everything contract. One that
ever did fire on real source would be.

- `callable_words.go:250` — higher-order `<w>` over a gradual-Any collection: ambiguous overload (List vs Map), no static commit and no poly re-match
  §9.3 names this exact site defensive-only: a CompileDynBody sig declines one line above at :248 (routes to the dyn-body poly re-match) and a CrossCollectionTokenShape word falls through at :242, so every shipping Callable word bypasses this raise — pinned white-box in dynbody_unit_test.go.
- `eng/go/emit.go:1169` — (the MarkUncompilable function definition; the internal trapAt early-return, not a shape refusal)
  This is the MarkUncompilable latch itself, not a raise site; its trapAt guard silently drops any mark at-or-after a terminal top-level trap (the interpreter raises at the trap and never reaches the later construct), so it never records a real refusal shape.
- `eng/go/emit.go:2403` — if: <name>-branch not captured
  frag==nil means the recorder never captured the arm fragment for a const-cond branch; every real captured `if` arm supplies a fragment, so this fires only on an internal recording gap (exercised white-box via BranchRecord{Then:nil} in TestRecordBranchRefusals), not on a compilable boru shape.
- `eng/go/emit.go:2429` — if: condition body produces no value
  A condFrag whose body diverges or leaves an empty stack has no truth value to branch on — an `if` with a genuinely void condition body; pinned white-box (TestRecordBranchRefusals empty-condition case) but represents a degenerate no-value condition rather than a compilable computation, effectively an internal guard on a non-shape.
- `eng/go/emit.go:2468` — if: then-branch not captured
  The 2-arg no-else zeroOut statement-guard path where b.Then is nil: the arm body still runs on the true path but was not captured for lowering — an internal recording gap for a shape that otherwise always supplies b.Then, not a compilable source shape.
- `eng/go/emit.go:2496` — if: computed then value with non-stack condition (Stage 2)
  Carries //covergate:allow: by the time a COMPUTED then value (op.kind==opEvent) is reached, ev.br.cond was already resolved on the disjoint default/frag/const paths, so computedArmCondOK cannot return false without a bytecode-level fault — an unreachable compiler defensive belt.
- `eng/go/emit.go:2547` — if: computed else value with non-stack condition (Stage 2)
  Carries //covergate:allow: mirror of :2496 for the single-computed-else arm — ev.br.cond was already resolved on a disjoint path before an opEvent else value is reached, so computedArmCondOK cannot fail without a bytecode-level fault.
- `eng/go/emit.go:2806` — loop-carried def `<name>`: pre-loop value no longer resolves
  The re-resolve happens on a repeat AnalyseLoopBody round for an already-seen carried name; the pre-loop value is round-invariant (registered on the first round), so a resolution that now fails is a recording inconsistency, not a source shape — exercised only by a synthetic unresolvable carrier white-box (emit_seam7_test.go:1240).
- `eng/go/emit.go:3522` — for: body not captured
  The native forCarrierAnalyse only calls RecordLoop under the `lowerable` gate, which first runs ArmLoopCapture+TakeFragment, so body is never nil from real source — this fires only via the emit_seam7 white-box RecordLoop(nil,...) construction, an internal-consistency belt.
- `eng/go/emit.go:3529` — for: range of unknown provenance
  The caller decomposes bounds to NewInteger consts (start/step) with the end an already-validated operand before setting lowerable; resolveOperand of these always succeeds, so the guard fires only via emit_seam7's synthetic unresolvable-carrier RecordLoop call — a bytecode-fault belt.
- `eng/go/emit.go:3533` — for: computed range start/step (Stage 2 follow-on)
  native_control's lowerable gate only sets lowerable=true when start/step are already concrete integers (parseRange/computedRangeBounds require const start/step), so a computed start/step never reaches RecordLoop from source — the guard fires only white-box; the computed-start SHAPE is a future widening but this emit.go site cannot fire from real boru.
- `eng/go/emit.go:3604` — for: iterator slot not registered
  AnalyseLoopBody registers the iterator local before RecordLoop runs, so the localByID lookup always succeeds from real source; the guard fires only via emit_seam7's synthetic all-const-range/empty-body RecordLoop with an unregistered iter — an internal-consistency belt.
- `eng/go/emit.go:4109` — dispatch without a signature at <word>
  recordDispatchOutcome/carrierResults only runs on a MATCHED dispatch, so sig is non-nil on every real program path; the nil arm is reachable only via the white-box seam pin emit_seam7_test.go:41 and cannot fire without an unmatched dispatch being recorded, which the pipeline never does.
- `eng/go/emit.go:4120` — user fn call <word> (Stage 3)
  a resolvable user-fn call is recorded as CALL_USER by tryRecordClosure/core_helpers BEFORE RecordCall, so an fnFrame sig reaching recordCallRefusal is the residual no-home arm; the ordinary user-fn dispatch never lands here (it is the Stage-3 belt behind the closure path).
- `eng/go/engine.go:6980` — fn-value application bounded by a paren (dynamic value precedes args)
  Carries //covergate:allow: reached only if RecordDynMethod declines after fnVal was gated to a member-read EVENT and every argVal is an isRecordableLiteral resolveOperand can seat — an invariant that cannot break without a future window-shape fault, so the belt is unreachable and its refusal never fires on real source.
- `method_shape.go:482` — shaped method apply: operand of unknown provenance at <w>
  RecordDynMethod's resolveOperand-fail arm: a real shaped-method Origin is always an event-backed member-read carrier and its args are isRecordableLiterals (the engine.go:6979 covergate states this seam `cannot decline`), so this only fires under a white-box fabricated PendingMethodApply on a bare non-event carrier — covered by fault-injection in method_shape_seam9_test.go (TestW9TryRecordMethodApplyRecordFails), not by any boru source shape.
