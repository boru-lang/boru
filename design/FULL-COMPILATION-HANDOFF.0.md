# Full compilation — handoff for the bind-twin line

**Point in time: 2026-09-02; first written 2026-08-30 at `007ac5c`.** This is
a state-of-play note for whoever picks up Stage 4's remaining piece. The
design is [FULL-COMPILATION.0.md](FULL-COMPILATION.0.md); §6.5 is the section
that matters here. This document does not restate the design — it records
where the work stands, what has been measured, and the three or four things
that will waste a day if they are re-derived from scratch.

**Read it as a running log, not a snapshot.** Sections are appended as work
lands and the earlier ones are left standing with their dates, because what
a measurement said BEFORE a fix is half of why the fix is what it is. Where
an early section states a plan the later ones overtook, the later one wins;
each says so explicitly.

## Where the work is

Stages 0, 1 and 2 are landed. Stage 4's recorder and apply kernel are
landed. **The bind twins are landed too, and FLIPPED** (2026-09-01/02):
rollback-and-replay is the only regime, the keep-installs path is deleted,
and the rollback base rides with the Program. What that did and did not buy
is the whole subject of the blocks below — in particular it did NOT license
the keep-regime latch deletions the payoff list names, which are measured
one gate at a time in "The payoff gates, measured".

The rest of this section is the state BEFORE the twins existed, kept because
the population measurement is what sized every increment that followed.
What existed then was `CheckState.BindLedger`, an **inert** record of every
runtime-visible binding transition the check pass performs, in source
order. Nothing read it to decide anything, so it could not regress a program;
it could only size the next increment.

### The population, measured over `lang/spec` (7644 rows)

As of the 2026-08-31 oracle closure and the BindSigUndef split (ten phantom
truncated-region entries removed; two of the four old def-replace entries
were sig-undef NO-OPS — locked matches that remove nothing — and no longer
note at all, taking one row's only transition with them; the corpus
contributes no top-level loop-join entries and no REAL sig-undef removal —
the synthetic rows supply both shapes):

	rows with transitions   4290
	transitions total       7441
	  def                    6361   85%
	  type-install           1048
	  undef                    30
	  def-replace               2
	  sig-undef                 0   (corpus; the synthetic row supplies it)
	deepest single row         36   —  unpack 'boru:math-util' sqrt 16.0

`TestBindLedgerCensus` produces this. Three consequences, and the third
reverses an earlier plan:

- `def` is 85%, so its twin is the one whose cost matters; the other three
  can be the simple, obviously-correct ones.
- `undef` and `def-replace` together are **34 transitions in the entire
  corpus**.
- The deepest single row is **36, not 1672**. The earlier figure counted
  frame-locals. With those excluded no corpus program performs more than a
  few dozen replayable transitions, so **per-transition twin cost is not a
  budget concern** — an earlier note in §6.5 said it was, and that was
  wrong.

## The next increment, in order

1. ~~**Close the live-depth oracle's 9 mismatches**~~ **DONE 2026-08-31** —
   the oracle is landed green with no allowance
   (`test/go/langspec/bind_ledger_live_oracle_test.go`); see the section
   below for what the 9 actually were, because both prior readings of them
   were wrong in instructive ways.
2. **Emit the `def` twin while the check-pass installs are still KEPT**, so
   it is inert, and assert the replay matches. **DONE (2026-08-31), in two
   halves.** The TABLE half: every compiled Program carries
   `Program.BindTwins` — the pass's ledger mirrored entry for entry through
   `NoteBindTransition`'s own funnel (`EmitRecorder.RecordBindTwin`, fired
   after every ledger suppression, table-appended unconditionally — no
   Active/Suspend gate, because a second filter would be a second source of
   truth), and `TestBindTwinsEqualLedger` asserts table == ledger
   elementwise over every compiled corpus row, no allowance (7270 compiled
   rows, 4100 with twins, 0 mismatched). The POSITIONED half: an
   `evBindTwin` event is appended at note time whenever the recorder is
   LIVE, and lowers to an `OpBindTwin` instruction (arg = table index; a VM
   no-op under the keep regime) at the transition's production-order
   position; `TestBindTwinOpsArePlacedOrderedSubset` gates it — every op
   indexes a real entry, indices strictly increase within each unit, and
   placement is a SUBSET of the table by design (7124 of 7154 compiled-row
   entries placed; the ~30 unplaced are the suspended-recorder class —
   each/fold body defs, which have no stream home until the twin is
   arm-resident — plus ops discarded with an island). The FLIP's refusal
   logic is what will tighten subset to equality: a program with an
   unplaced twin must refuse rather than replay incompletely.

   Two lessons from the positioned half, paid for in one corpus row each:
   - `emptyFlexHookOperand`'s "no event recorded since the construction"
     proof enumerates BOOKKEEPING event kinds to skip (evDynBind, and now
     evBindTwin). Any future bookkeeping event kind must join that skip or
     it silently refuses `walk`'s empty-flex accumulator row
     (`corpus-core.tsv:50` went 7270 → 7269 compiled; found by diffing the
     compiled-row LIST against the previous commit, which is faster and
     sharper than re-running the census — keep that probe in the toolbox).
   - Golden churn was 4 tests, all hand-written disasm strings in
     `lang/go/bytecode_emit_test.go` — small because BIND_TWIN emits only
     at def/undef/type/join sites, and worth re-pinning by hand: each new
     BIND_TWIN line in a golden documents where a twin lands.
3. **Only then** flip to rollback-and-replay onto
   `core/go/binding_sandbox.go`, which is already built with the exact
   mints-retained / retirements-rolled-back partition §6.5 requires.
   **FLIP v1 LANDED (2026-08-31), staged behind `BORU_TWIN_REGIME=1` —
   default OFF stays byte-identical.** The machinery, end to end: the
   recorder reads the flag once per pass (`compiler.TwinRegimeEnabled` →
   `EmitState.twinRegime`), stamps `Program.TwinRegime` at Finalize, and
   under the regime refuses any program whose twin table is not FULLY
   stream-placed (the subset-to-equality tightening this step always
   promised: an unplaced twin is a transition the rollback would lose).
   lang's compiled entries (`RunAutoValues`, `RunCompiledStrict`) take a
   `SnapshotBindings` before the pass under the same switch and, for a
   stamped Program, roll back via the new
   `core.RestoreBindingsForReplay` at the one safe between-phases point —
   RestoreBindings minus the module-ledger rollback, because imports ran
   once on the pass and must NOT re-run on the next request; the
   namespace BINDING is what the def twin replays. At run time each
   placed `OpBindTwin` calls `core.ApplyBindTwin` (bind_twin_apply.go):
   push kinds re-install the captured entry (Push / PushType /
   PushTypeAdopted by Minted), an undef pops-then-retires-if-minted, a
   sig-undef removes the captured entry by identity (ID, else value
   equality — never by position), a def-replace pops then pushes. The
   COMPUTED-def pairing landed exactly as the needGlobal-coincidence
   fact below predicted: `ApplyBindTwin` skips a captured entry matching
   `TypeDef == nil && !IsConcrete && !IsBareTypeNode`, and that def's
   `OpBindGlobal` (its `GlobalBindSpec.Push` stamped by lowerDynBind
   under the regime) PUSHES the runtime value instead of SetAt — twin
   before bind in stream order, so replace-twins net zero with the push.

   **Measured on landing, whole corpus
   (`test/go/langspec/bind_twin_regime_test.go`, the regime lane —
   regime-compiled vs fresh interpreter): 6411 rows compiled, 0
   divergences, no allowance.** The default recorder compiled 6416 the
   same day: the full-placement gate costs exactly 5 rows (classified in
   the lane by the `twin regime:` refusal reason). The flip-era analysis
   called four of them "island-discarded" — **that mechanism label was
   WRONG, and the 2026-08-31 recovery instrumented rather than assumed**:
   all five rows record through the CLOSURE path (`tryRecordClosure` —
   `RecordFallback` never fires for any of them), so the do bodies
   compile to closure units whose defs are frame-local, and the
   interpreter's leak is what the rollback loses. Island-discard
   accounting in Finalize would have recovered NOTHING — and worse, an
   island-re-execution account is unsound for a capitalised def (the
   regime retains minted type IDs, so a VM-time re-run of
   `def Big Integer` re-mints against the surviving name-part — the
   bodyHasReplayHazard class). The landed recovery is §6.5-faithful
   instead (replay, never re-execution): **do-body twin ADOPTION** —
   `CallableSpec.BodyOnceKeepsDefs` (set on `do` alone: handler runs
   the body exactly once, defs leak; check-mode twin is
   `RunCarrierBodyKeepDefs`) licenses `EmitState.AdoptBodyTwins` after
   the closure record to PLACE each suspended twin whose noted position
   is a token site in that body's tree as a real `evBindTwin` after the
   call event, so the lowered `OpBindTwin` replays the captured
   identical entry once the unit returns. Adoption is FENCED (the
   Codex P1 round on #421, each fence verified by repro): only twins
   noted inside the dispatch's own outermost keep-defs body run
   (KeepDefsBodyGuard's bracket — an aliased quotation's earlier
   multi-run twins stay out), excluding sub-ranges noted under a
   nested non-keep body run (a nested each's per-element transitions
   — analyseHigherOrderBodyVals now suspends through
   BodyAnalysisGuard so the taint sees it), and only at the root
   stream (a do nested in a callback's compiled unit keeps its sound
   refusal). Each fenced-out shape refuses to the interpreter; the
   fences cost zero corpus rows. That recovered the four
   do-body rows (`do [def Big Integer …]`, the predicate variant,
   `do [def x 5 raise …]`, the quoted `[def zz 5 …] do`). The ONE
   remaining regime-only refusal is the suspended-recorder each-body
   leaking def (`bytecode-migrated.tsv:41`) — deliberately unflagged:
   a multi-run body's runtime re-runs the body per element where the
   ledger noted one generalized transition with carrier-valued
   captures, so a single replay would be wrong in count and value; it
   waits for arm-residency.
   Cross-request persistence — keep-on-compile's contract, now delivered
   by replay — is pinned by `TestTwinRegimeSmoke` (a replayed binding is
   readable by the next request's check pass on the same instance), and
   holds for adopted twins too (`do [def x 5] x add 1` then `x add 10`
   → 15 on the same instance).
   `Program.TwinRegime` also drives the disasm mode tags
   (`(inert)`/`(replay)`, `(push)` on global binds).

   What remains for the DEFAULT flip, in order: (a) run the regime lane
   long enough to trust it (it is committed and green — every push of
   this PR now exercises the flip corpus-wide); (b) arm-resident twins
   for the each-body row (whose design review re-scoped the NUR110
   claim: per-arm twins are NUR110's mechanism half only — the read
   side still const-folds from the joined check model, so NUR110's
   close is a follow-on with its own read-side lowering, and the
   if-arm half has zero corpus rows); (c) flip the default, delete the
   keep-regime latches the payoff list names (frozen-read/
   NotifyNameRebound gates, emit.go's rebind latches, family L's
   CondBodyDepth refusal, NUR037), and collapse `GlobalBindSpec.Push`/
   the twin-regime branches into the only path.

   **(c) THE FLIP LANDED (2026-09-01): rollback-and-replay is the ONLY
   regime.** `BORU_TWIN_REGIME`, `compiler.TwinRegimeEnabled`,
   `EmitState.twinRegime` and `Program.TwinRegime` are gone; lang's
   compiled entry points snapshot before every check pass and roll back
   through `RestoreBindingsForReplay` unconditionally; every placed
   `OpBindTwin` replays (the inert arm is deleted); `GlobalBindSpec.Push`
   is deleted because a global bind can only PUSH now — the SetAt arm
   went with it, and with it `DefTable.SetAt` itself, which had no other
   production caller. The disassembler tags every twin `(replay)` and
   tags global binds with nothing, because there is no second mode for
   either tag to contrast with. The flag-armed corpus lane
   (`TestSpecCompiledDifferentialTwinRegime`) is retired as a duplicate
   of the default differential, which inherits its floor (6410); the
   hand-checkable `TestTwinRegimeSmoke` stays. The three golden
   failures the rehearsal predicted resolved exactly as predicted —
   four `(inert)`→`(replay)` annotation lines, instruction streams
   untouched — and the NUR116 pin rows became unconditional
   compile-with-parity assertions, which DISCHARGES NUR116 (the
   default lane no longer exists to carry it; the record is deleted per
   the register's rule and this commit names it).

   What the flip did NOT do, deliberately: the keep-regime latches the
   payoff list names are NOT deleted here. The rehearsal measured
   (above) that the stored-handler dep-rebind refusal is load-bearing
   with the regime on — disabling it sends those programs into a VM
   internal_error and a fallback, not into correct compiled code — so
   its deletion needs §6.9's runtime-lookup half, and the frozen-read /
   NotifyNameRebound / family-L / NUR037 gates each want the same
   `-force-compile` measurement before they go. That is the next
   increment's list, gate by gate, each with its own measurement.
   Two follow-ons also stay separate: NUR115 (foldaxis's analysing
   ReturnsFn) and nested multi-run bodies (the latch-to-stack design
   the second rehearsal wrote up).

   **(d) NUR115 DISCHARGED (2026-09-02): foldaxis analyses its body.**
   The regime's one known silent divergence is closed the way the
   rehearsal prescribed — not by refusing the shape but by giving
   `foldaxis` an analysing ReturnsFn. `foldaxisReturnsFn` is fold's
   accumulator fixed point one rank down (both carriers from
   `rank2ElemType`, the same carriers the Callable's Inputs hand the
   compiled body; the result is scan's list-of-accumulators shape, where
   the structural `ReturnsPreserveListAt(2)` had answered the data's ROW
   type — wrong for a reduction that returns one value per lane).
   Analysing the body is what records its bind twins, and with the twins
   visible the word earns `BodyMultiRunKeepsDefs` on the handler-verified
   ground its siblings have (foldaxisHandler reduces every lane through
   doFold: InvokeBody once per lane element past the seed, on the shared
   registry, no def cleanup). Six parity-oracle rows measure it: both
   axes; the elem-valued row's install ORDER (top-down `9 4 3 1` — the
   var pair's `b` half is the accumulator); one-element lanes, where the
   body runs ZERO times and the install count must be zero on both sides
   (a replayed check-pass twin would install once — arm-residency is
   what makes zero right); the empty rank-2 list (the gradual-Any
   element arm); and the two sibling refusals (root read of an arm-bound
   name; nested multi-run body). The NEGATIVE CONTROL ran before the
   record was deleted: with the structural ReturnsFn restored and the
   flag kept, the parity rows lose every install (`[5 5]` against `[]`),
   the read-after row fails check with `undefined_word` instead of
   refusing at the fence, and the nested row COMPILES silently wrong —
   the divergence exactly as NUR115 described it, now pinned by rows
   that fail without the fix. `eachrank` alone still carries the
   structural ReturnsFn; it is safe only because it refuses early, and
   its comment now says an analysing ReturnsFn comes before its flag.
   The general test stands: a new body word whose ReturnsFn does not
   analyse the body is invisible to the twins, and no gate will say so.

   **What the review of that change found (2026-09-02), and it was
   bigger than the change.** An adversarial review of the analysing
   ReturnsFn measured, on both engines, that `rank2ElemType` — the
   first-row element type foldaxis had used since its Callable landed —
   was WRONG for any data whose rows differ in type: a lane draws its
   elements from every row, and "rectangular" means equal length, not
   equal type. Two consequences. The PRE-EXISTING one: the compiled body
   was baked for the first row's type, so `ArrayUtil.foldaxis 1 [add]
   [[1 2] ['a' 'b']]` answered `[3 0]` compiled against the
   interpreter's `[3 'ab']` — a second silent foldaxis divergence that
   NUR115 never named, reproducing identically before the change. The
   NEW one: the analysing ReturnsFn typed the RESULT from the first row
   too and the compiler baked that into the result's consumers, so a
   check-refused program became a wrong-answering one. Both close at the
   one root: `rank2ElemCarrier` joins EVERY row's elements through
   scan's own `ElementCarrierFromValue` (a mixed population is a strict
   Disjunct the body dispatch distributes per alternative; a shape the
   pass cannot open is the gradual Any), and both the Callable's Inputs
   and the ReturnsFn take it. Corpus rows in module-array.tsv pin the
   mixed-row and heterogeneous-row answers on both engines; a direct
   unit test pins each carrier arm and the result join. Two more from
   the same review: a statically-empty rank-2 list is no longer analysed
   (scan's `StaticListLen` guard — an operand-starved body over `[]`
   runs on both engines and must not refuse at check), and
   `foldaxis 1 [add] [[]]` — one row, zero columns, one EMPTY lane —
   PANICKED in the handler on `lane[0]`; it now raises `foldaxis_error`
   as fold's no-init rule does, mirrored at check time exactly as fold's
   statically-empty case is (`staticEmptyLaneDetail`, a RuntimeMirror at
   the call site the checker exposes — `foldaxis_error` had to be
   registered as an error-severity code first, or the check-accuracy
   ratchet counted the row as a checker blind spot), with a corpus row
   and wave3 pins. None
   of these is a NUR record: each surfaced and closed inside this PR,
   the register deletes a Resolved record, and the rows are the trace.
   The method note is the one worth keeping: the divergence the twin
   regime exposed (NUR115) sat NEXT TO a divergence it did not, and only
   a reviewer who measured mixed-type data found the second — the
   oracle rows test bindings, the corpus rows test values, and a new
   body-word typing needs both.

   **(e) The VARIATION LANE's exposure (2026-09-02, CI on the flip PR).**
   The rehearsal ran every module suite with the flag on and the corpus
   lane at full parity, and still missed one population: the variation
   differential (`TestVariationDifferential`, test/go/langspec) re-embeds
   sampled corpus rows in every wrapping context (each-body, do-body,
   do-catch, for-body …) and classifies each variant — a lane the
   selective local belt never reached and the 10-minute default `go test`
   timeout hides (the langspec package needs `make test`'s 35m). It found
   two things, both the flip's and both settled honestly rather than
   widened around:
   - **Fifteen variants now REFUSE where the old default compiled them on
     the check pass's kept install** — one new bucket, `twin regime
     (unplaced bind transition)`, four shapes: an `import` inside a
     multi-run body (a module bind is not a BindDef the arm-residency
     bridge installs), a TYPE def inside a multi-run body (the bridge
     pairs BindDef twins only), a `do` body whose closure compile
     declines to a Stage-3 residual shape so its twins are never adopted
     (`do [def b true 1 2 (if b [] [9 9])]` — its `1 2 b` and
     `(if b …)`-only siblings compile), and a call into a
     boru-IMPLEMENTED module (`Sift.parse`, sift.boru) inside the do body
     that imported it — measured, not yet root-caused: the same call
     with the import outside the do compiles, and the import without the
     call compiles; it is the import-and-call pair inside one once-run
     body that leaves a twin the adoption declines.
     Every one is the sound direction: a replay the rollback would lose is
     exactly what the placement gate refuses. Pinned in
     `varyRefusalLedger` with one representative row per shape in
     lang/spec/frontier/frontier-twin-placement.tsv (each ledgered with
     its failure mode, so a silent graduation or a drift fails). Each
     shape names its graduation: resident module binds and resident type
     twins inside compiled units, a closure lowering that admits the
     declined do body, and the root cause of the import-and-call pair.
   - **One KNOWN MISCOMPILE graduated.** The mount-handler loop variant
     pinned in `varyKnownMiscompiles` since 2026-07-30 (a flex map
     captured by a mount handler lost its identity across loop
     iterations: `expected a FlexMap, got FlexMap`) no longer diverges —
     measured with `-force-compile`, `hello mounted hello mounted` on
     both engines. The account that fits the error text: the keep-installs
     default left the check pass's own `files` instance behind for the
     handlers' dep to see while the loop re-bound the name; the regime
     rolls that install back, so one runtime instance is all there is.
     The pin is deleted (its stale arm fired, as designed) and the map is
     empty for the first time since it was created.
   The lesson for the next flip-sized change: the belt is `make test`,
   not the suites one remembers to run.

   **(f) The rollback rides WITH the Program (2026-09-02, Codex P1 on the
   flip PR).** The flip put the snapshot/rollback pair in lang's two
   compiled entry points — which left every OTHER compile-then-run caller
   without one: the documented low-level `CompileCheck` then
   `eng.RunProgram` flow, and eng's own compile-then-run tests
   (`compileTokens` + `RunProgram`). With the replay now unconditional,
   that flow stacked a second install on the check pass's kept one —
   `def X (refine Integer)` at depth 2, a later `undef X` leaving a
   binding and its type live. Fixed by making the base the PROGRAM's:
   `EmitState.BindRegistry` snapshots the program registry's bindings at
   its first bind (the same moment `progReg` is captured, before the
   check pass performs a transition), `Finalize` stamps
   `Program.ReplayBase` / `ReplayReg`, and `eng.runProgram` restores it
   before executing — only when run ON that registry (a foreign DefTable
   must never be installed; a base-less hand-built Program restores
   nothing). `RestoreBindingsForReplay` now restores from a CLONE so a
   Program can run more than once and every run rolls back to the same
   base. lang's explicit pair is deleted: one mechanism, and every
   caller inherits it. Pinned at every layer — core (the clone keeps the
   snapshot pristine), compiler (first bind captures, a sub-registry
   re-bind neither re-captures nor re-targets), eng (hand-built program:
   depth 1 not 2, a second run still 1, foreign base and no base both
   restore nothing), lang (Codex's exact low-level flow, def then
   undef). Not a NUR record: the divergence surfaced and closed inside
   one PR, and the register deletes a Resolved record; this note and the
   commit are its trace.

## The payoff gates, measured (2026-09-02)

§6.5 names four gates the bind twins were supposed to delete: the
frozen-read refusals (`NoteFrozenRead` / `NotifyNameRebound`), the interim
stored-handler latches, family L's conditional fn shadow, and NUR037's
fn-local-fn refusal. **All four are load-bearing. The claim is wrong for
every one of them, and wrong differently each time** — which is why the
measurement had to be gate by gate rather than a single sweep.

The common root, stated once because each gate rediscovers it: **the twins
fix WHERE THE REGISTRY IS; every one of these gates defends against WHAT IS
IN THE BYTECODE.** Rollback-and-replay makes VM-time binding state equal
the interpreter's tape state at the corresponding token. It does not
un-bake a const, un-inline a splice, or re-resolve a call target the
lowering already chose. Any gate whose hazard lives in an emitted
instruction is untouched by it, and three of these four do.

- **FROZEN-READ — load-bearing, and it would fail SILENTLY.** Measured:
  `def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  def x 2  f 0`
  answers `1 2` interpreted. `boru check --emit` on the no-rebind twin
  shows the unit as `PUSH_CONST k0 ; 1 / PUSH_LOCAL l0 / CALL_NATIVE add /
  RET` with `BIND_TWIN … (replay)` already present in the root and
  `fallbacks=0`. The twin replays; the const does not move. Delete the
  hammer and both calls run that same unit — `1 1`, with no VM error to
  notice. The `undef` variant is worse: the interpreter RAISES
  `undefined_word`, the baked const answers `1`.
  The design's own diagnosis at §6.5 line 903 — "Only the FROZEN-READ shape
  is a region's business … That is exactly what OpCollect answers" — is
  right about ONE of the three shapes the gate covers and wrong as a
  description of the gate. Shape (2), the `region_desc.go` `k` pair where a
  rebind flips a forward token between value-slot and barrier, is a region
  problem. Shape (1) is not: in `x add y` the token PRECEDES the word, so it
  is a value-stack operand that no forward-collecting region window ever
  sees. Shape (3), a splice payload, is not either: a rebind changes the
  INSTRUCTION SEQUENCE, and no descriptor re-derives that. **And OpCollect
  does not exist** — `Program.Regions` (`compiler/go/bytecode.go`) has zero
  writers and zero readers in the tree (`grep -rn "\.Regions"`), and
  `OpCollect`/`OpDispatchGeneric` appear only in comments.
- **STORED-HANDLER DEP-REBIND — load-bearing, and already measured so in
  the rehearsal.** Re-verified against the code: the gate is
  `NotifyNameRebound`'s `depHit` arm, and the counterexample is
  `design/RELOAD-INVALIDATION.0.md` §3's F1 (interpreter `6 105 12`,
  compiled-with-poisoning-only `12 12 12`). Poisoning alone falls back to
  `CallBoru`, which resolves the LIVE def table — and module-scope def
  sites all execute in the check pass, so by VM time the table holds the
  pass-final binding for calls sequenced BEFORE the rebind too. The twins
  make VM-time def order real, which is the necessary half; the sufficient
  half is a runtime LOOKUP at the call, i.e. §6.9's `OpDispatchGeneric`.
  Until then, disabling it buys a VM `CALL_NATIVE_POLY no match`
  internal_error and a fallback, not compiled code.
- **FAMILY L — load-bearing, and the flip made the shape WORSE.** For an
  `if`/`case`/`for` arm the check pass records NO transition at all:
  `runCarrierBodyDefsAdds` raises `RolledBackBodyDepth` and
  `NoteBindTransitionEntry` returns early on it. No ledger row, no twin, no
  op — there is nothing for the replay to replay, so deleting the gate
  produces a silent miscompile rather than a graduation. And the half the
  rollback DOES fix now cuts the other way: the registry ends up holding the
  outer overload while the already-lowered `OpCallUser` still names the
  shadow unit, so bytecode and registry disagree with each other. Under the
  old keep-installs default they at least agreed. Measured on the
  gate-exempt cond-fragment sibling: one `BIND_TWIN` naming the OUTER
  `def g`, and `CALL_USER f0` where `f0` is the SHADOW body.
- **NUR037 — load-bearing, and excluded from the twins BY CONSTRUCTION.**
  `NoteBindTransitionEntry` returns early whenever `FnBodyDepth > 0`, and
  that is exactly this gate's population: a `def … fn` inside a fn body.
  The exclusion is deliberate and measured (it took the ledger from 69254
  entries to 6110 — fn-body defs are frame-locals, and the compiled lane
  gives them slots, not registry bindings). No twin exists to place; the
  arm-residency bridge is root-fenced out of fn units anyway; and
  `RunFnBodyOnce` already restores its snapshot, so the old default and the
  new rollback leave the VM registry identical for this shape. The design
  credits the deletion to "the name is looked up live" — that is
  `OpDispatchGeneric` again, and a live lookup without a BINDER half finds
  either nothing (the original NUR037 bug) or an outer same-named binding
  the twins did replay, which is worse than the refusal.

**RE-MEASURED 2026-09-05, on the Stage 4b tree (binding-sensitive memo,
body re-run environment).** The three gates that stayed on the §6.9 list
are exactly as load-bearing as above, and Stage 4b was never going to
move them — each is a shape the memo cannot reach:

- Stored-handler dep-rebind (F1, `6 105 12`): the default lane still
  refuses `module binding bonus rebound after a stored handler captured it
  as a dep` and the interpreter answers; `-force-compile` refuses. A stored
  handler is a fn VALUE, i.e. an escaping unit by construction, so the memo
  has no call site to re-record — the apply is through the value.
- Family L: both `if` arms still refuse (`fn 'g' redefined inside a
  conditional body (branch/loop) shadows an outer overload`), taken or not,
  and so does a loop body that RUNS (`for 1`, `for n` with `n` 1, `[1]
  each`). A loop body that provably never runs (`for 0`, `for n` with `n`
  0, `[] each`) compiles and answers the outer overload (`101`), agreeing
  with the interpreter — the redefinition is never analysed, so
  `CondBodyDepth` never sees it. Sound in both directions.
- NUR037: both pinned shapes (`for-each [step] xs`, `each [step] xs` over
  a fn-local `step`) still refuse `code-body names fn-local fn`; the
  interpreter answers `{x:true y:true}` and `[2 3]`.

Nothing here is a memo problem. All three want the runtime LOOKUP half —
and family L and NUR037 the BINDER half — that the paragraph below files
them under; the memo's contribution is only that the frozen-read gate no
longer keeps them company.

**What this changes about the plan.** The four gates leave the twins'
payoff list and re-file under §6.9 (`OpDispatchGeneric` — the lookup half)
plus, for family L and NUR037, a BINDER half that makes a conditionally- or
frame-locally-bound name registry-visible at VM time. §6.5's payoff
sentence is corrected in the design accordingly. The honest summary of the
flip's dividend is the one the corpus already showed: full parity with the
old default, NUR116 discharged, NUR115 discharged, one known miscompile
graduated, and the rebind gates untouched.

**Method note, because it generalises.** Three of the four verdicts came
from `boru check --emit` rather than from running anything: if the hazard is
a baked instruction, the disassembly settles the question before any gate is
disabled. Reach for it first — and never for `-compile`, which falls back
silently and reports a gate deletable when it is not.

## Nested multi-run bodies: the latch-to-stack design

The (c) block above promised "the latch-to-stack design the second
rehearsal wrote up". **No such write-up existed** — it lived in session
notes and nowhere in `design/`, `NUR.md` or the history. This section is
that design, reconstructed from the code on 2026-09-02 and measured. Three
of the sketch's four remembered parts turned out to be wrong or incomplete;
those corrections are the reason it is worth writing down rather than
re-deriving.

**`fold-nested-multirun` is FOUR declines, not one**, and the first fires
on a nested body that binds nothing at all:

- **The compile-phase clobber (primary).** `MultiRunBodyGuard` is invoked
  from `analyseHigherOrderBodyVals`, i.e. from EVERY higher-order ReturnsFn
  — `each`, `for-each`, `fold`/`scan` (through the accumulator fixed
  point), `outer`, `inner` (twice per dispatch), `eachrank`, `foldaxis` —
  not only the words carrying `BodyMultiRunKeepsDefs`. The latch is written
  by all of them and read for the flagged few. The analysis phase leaves it
  CORRECT; then `tryRecordClosure`'s body compile RE-RUNS the outer body,
  the nested word's ReturnsFn fires again, and its guard close overwrites
  the single slot. `AdoptResidentTwins` then fails the identity fence and
  adopts nothing. Measured three ways: `fold [ var [[a b] ([1] each [add
  1]) (a add b)] ] [1 2] 0` refuses although the nested body binds NOTHING;
  the same shape with `for-each` (which can never adopt) refuses too; and
  the same shape with `filter` — which does not route through
  `analyseHigherOrderBodyVals` — COMPILES. The clobber is precisely "any
  `analyseHigherOrderBodyVals` caller inside the body".
- **Count.** The outer bracket's `[from,to)` CONTAINS the inner body's
  twins, but the outer unit's fragment holds no def event for them — they
  live in the nested unit's own fragment — so the strict
  `len(events) != len(twins)` fence declines. This is what owned-set
  subtraction is for.
- **Supersede.** The fixed-point exemption compares against the single
  slot, which by the time an outer round-2 guard closes holds the CHILD's
  latch. A fold accumulator around a nested body therefore never exempts
  its dead rounds.
- **The inner dispatch can never adopt.** Its `AdoptResidentTwins` runs
  mid-way through the outer body compile, where the root fence sees an open
  unit recording and returns immediately.

**Part 1 — the publication gate. LANDED 2026-09-02, and it is the
highest-value piece.** At guard OPEN compute `publish := r != nil &&
r.Check != nil && r.Check.FnBodyDepth == 0`; when false, suspend and taint
exactly as today but publish nothing. This is exact rather than heuristic:
`FnBodyDepth == 0` is precisely the condition under which a twin can be
noted at all, so a gated run has an empty range by construction and has
nothing to say. Nil-safety is not optional — existing tests call the guard
with a nil registry.

What it bought, measured: **two shapes graduated from refusal to parity**
and are now oracle rows —
`fold [ var [[a b] ([1] each [add 1]) (a add b)] ] [1 2] 0` (`3`) and
`[1 2] each [ var [[r] def x r (for-each [add 1] [1 2]) x] ]` (`[1 2]`),
the second being a nested word that carries no `BodyMultiRunKeepsDefs` and
so could never have adopted anyway; it declined purely by writing the latch
on its way past. Parity here means the install stacks were measured equal
on both engines, not merely that the programs compile. The `filter` control
that never refused stays as the attribution.

What it did NOT buy, also measured: `fold-nested-multirun` and
`each-nested-multirun` still refuse — those are the count, supersede and
inner-adopt declines, which need Parts 2 to 4. So does
`aliased-body-memo-hit`, which is the point of that row.

**Part 2 — the stack, and the tree it leaves behind.** "A stack of open
brackets" is necessary but NOT sufficient, and this is the sketch's first
real error: by the time any adoption runs, every bracket has closed (the
analysis phase precedes the record phase), so a stack of open brackets is
empty exactly when it is needed. The stack's job is to build a persistent
TREE — nodes carrying `bodyID`, `from`/`to`, `reg`, `evSeq`, their
surviving `children`, the ranges of EVERY closed child (superseded ones
included), and a descent cursor — which a second, compile-time stack then
walks.

**Per-level supersede — the sketch omits it, and it is required.** The
fixed-point test must compare a closing node against the previous SIBLING
at its own nesting level, not against the last bracket closed anywhere.
With any nested body word the global comparison target is the child, so the
outer fixed point never supersedes its dead rounds. The `evSeq` witness
still holds at sibling level: analysis rounds run with recording suspended,
so the event counter cannot move between them.

**Part 3 — owned-set subtraction, and it must subtract TWO things.** The
sketch names only child ranges. `owned(n)` is `[n.from, n.to)` minus every
child range AND minus the superseded indices: an inner fixed point nested
inside a SURVIVING outer round leaves dead twins inside the outer's range
but outside the surviving child's range, and subtracting only the survivors
drags them into the outer's owned set and breaks the strict count.

**Part 4 — the root fence, and the sketch is literally wrong here.** There
is no `armResidentDepth > 0` condition in `AdoptResidentTwins` to relax;
the fence is `len(openUnitRecs) != 0 || len(frames) != 1`. A bare
"`> 1`" permission would never check WHAT the open unit is. The counting
form is the shape to aim at — the open-unit count matching the bracket
depth, so an adoption is permitted exactly when the unit being adopted into
is the one this bracket's dispatch compiled.

**What must NOT be relaxed, and the row that enforces it.** The
`closureLatch.fresh` memo fence looks like a single-latch relic once a
descent stack exists. It is not. `aliased-body-memo-hit` in the parity
oracle (landed 2026-09-02, ahead of any stack work) pins the shape: one
quoted body dispatched twice shares a body ID AND an analysis key, so the
second dispatch memo-hits the first's unit; the body is a var pair whose
notes carry Pos 0:0, which makes the position cross-check inert while names
and order are identical. Neither position nor name+order can separate the
two brackets — `fresh` and the per-event `residentTwin >= 0` staleness belt
are the only fences left. A stack that resolves a node by `bodyID` and lets
a bracket claim a unit it did not compile pairs bracket 2's twins against
bracket 1's stamped events; turn `residentTwin` into a list (the natural
move, since that field is accounting and not a value source) and the last
belt goes with it — and the program COMPILES, silently, two twins satisfied
by one op inside a unit neither bracket owns. The row's single-dispatch
control compiles today, which is what attributes the refusal to the memo
hit rather than to the aliasing.

**THE GATE HAD A SIBLING, and finding it root-caused an open question
(2026-09-02).** `frontier-twin-placement.tsv` shape 4 — a `do` that imports
a module and then CALLS into it — was recorded here and in its ledger as
MEASURED BUT NOT ROOT-CAUSED, with a guess buried in the wording: "a twin
the do-body adoption declines", i.e. a twin with no body site. It had a
body site. Instrumenting the placement gate showed the do's published keep
bracket coming back EMPTY (`[1 1]`, its floor already past the import's own
`Sift` twin), so `AdoptBodyTwins` had nothing to walk.

The cause is `MultiRunBodyGuard`'s clobber exactly, one guard over:
`KeepDefsBodyGuard` published from a single latch
(`lastKeepRange`/`lastKeepTaints`), and `tryRecordClosure`'s body compile
RE-RUNS the do body — so a `do` inside a fn that the body CALLS
(`sift-parse-do`'s) opened its own keep bracket during that re-run and
overwrote the outer one. The same publication gate fixes it: publish only at
`FnBodyDepth == 0`. The guard takes a registry now, as its multi-run sibling
already did.

Measured: both sift shapes compile with parity, the frontier row graduated
into `lang/spec/module-sift.tsv`, and the variation lane went pass 388 → 390
with the twin-regime bucket 15 → 13. The other three placement shapes still
refuse, correctly — they are genuinely different (multi-run bodies, and a do
whose closure compile declines).

**The class, stated once so the next one is cheap to find.** A single-slot
latch published at a guard close is unsound whenever a LATER phase re-runs
the same body, because the re-run's nested guards write the same slot. Three
such latches exist; two had the bug and are now gated. `lastClosure` does
not, and the reason is worth knowing: it is assigned AFTER the compile it
describes, so a nested dispatch's write during that compile is itself
overwritten. `lastUserPoly` is cleared rather than published and is not in
this class. When a fourth appears, the question to ask is not "is it keyed
correctly" but "who else writes it between the phase that sets it and the
phase that reads it".

**Four more hazards the implementation must answer.** The probe fork copies
`armResidentDepth` but deliberately not the latch — share the tree or the
cursors by reference and the probe, which recompiles the same body, consumes
the real state's cursors. The tree is built by the analysis descent and
walked by the compile descent, so any dispatch that opens a bracket in one
pass but not the other drifts them. `armBoundNames` would start gaining
names mid-unit-compile, so the read fence begins poisoning from an inner
dispatch's stream position. And the emit checkpoint snapshots
`twinAdoptions` but nothing of the tree's cursors — latent today because
rollback bails whole once an adoption has appended a unit.

   **THE FLIP REHEARSAL, and what it found (2026-09-01).** The corpus
   lane is not the flip's whole exposure, and the cheapest way to see
   the rest is to RUN THE SUITES WITH THE FLAG ON —
   `BORU_TWIN_REGIME=1 go test ./...` per module. The corpus rows are
   `lang/spec` TSVs; the Go suites carry shapes the corpus has none
   of, and they are where the flip actually bites. Three classes were
   closed by that rehearsal, each with a principled account rather
   than a widened allowance:

   - **Post-trap twins are UNREACHABLE, not lost.** Finalize truncates
     the stream at a terminal trap, dropping any `evBindTwin` after it;
     the placement gate then refused (NUR058's typed-def trap row). But
     the check pass walks past a trap where EXECUTION stops, so those
     twins describe defs the interpreter never performs — with the
     installs rolled back and no op to replay them, the bindings
     correctly do not exist. `truncateAtTrap` collects them and the
     gate exempts them. This is STRICTER than the keep-installs
     default, which strands the pass's post-trap install in the
     registry.
   - **Superseded analysis rounds record speculative twins.** Some
     higher-order analyses re-run a body to a fixed point (fold's
     accumulator widens between rounds — `analyseHigherOrderBodyVals`'
     own contract). Every round recorded the body's transitions afresh
     while only the last was latched, so `fold [ var [[k acc] (push k
     acc) ]] [1 2] []` refused. `MultiRunBodyGuard` now marks a
     superseded range exempt: only the surviving round's ops execute,
     so the dead rows install nothing. Same category as the ledger's
     rolled-back-body exclusion.
   - **The sibling multi-run words graduate on the measured
     mechanism.** `fold`, `scan` and `outer` carry
     `BodyMultiRunKeepsDefs` — each handler-verified (all drive
     `InvokeBody` per element/pair on the shared registry with no def
     cleanup, exactly as `eachHandler` does) and each pinned by its own
     oracle row, including outer's PAIR-GRID population and the
     fold-accumulator fixed point.

   **Two words were deliberately NOT flagged, and one is a new
   BLOCKER.** `eachrank` refuses earlier as a Stage-2 code-body word,
   so its flag could never fire and nothing could measure it — an
   unmeasurable graduation is not one. `foldaxis` is worse and more
   important: measured through the parity oracle, a def in a foldaxis
   body installs TWICE on the interpreter and NOT AT ALL under the
   regime, **with or without the flag** — no twin is recorded for that
   body, so the placement gate is blind and the program compiles
   silently wrong. Today's keep-installs default hides it (the pass's
   own install answers the read), which is exactly why it surfaced
   only under the rehearsal. Repro:
   `import "boru:array-util"  ArrayUtil.foldaxis 0 [var [[a b] def x 5 (a add b)]] [[1 2] [3 4]]`,
   then read `x`. The flip must either record that body's twins or
   refuse the shape; until then it is the regime's one known SILENT
   divergence.

   **The sweep is done, and the class is BOUNDED AT TWO WORDS.** The
   root cause is not in the handler — foldaxis reduces through the very
   `doFold` that `fold` carries the flag for — it is the RETURNS
   FUNCTION. `foldaxis` and `eachrank` alone use the structural
   `ReturnsPreserveListAt`, which never calls
   `analyseHigherOrderBodyVals`; every other Callable body word
   (`each`, `fold`, `scan`, `outer`) has a ReturnsFn that analyses its
   body, and all four are now flagged and oracle-pinned. A body the
   check pass never RUNS records no bind twins, and a gate that can
   only check twins that exist is blind BY CONSTRUCTION — which is the
   general statement of the hazard, and the thing to test for when any
   new body word is added: if its ReturnsFn does not analyse the body,
   the twin machinery cannot see it. Of the two, only `foldaxis`
   diverges silently; `eachrank` refuses earlier as a Stage-2
   code-body word, which sends the whole program to the interpreter —
   the sound direction. So the flip's remaining work here is one word,
   with a known mechanism: give foldaxis an analysing ReturnsFn (which
   also earns it the flag, measured through a new oracle row), or
   refuse the shape. Registered as **NUR115** — the register's job is
   that a divergence is never silently baselined, and this one is
   invisible to every existing gate.

   **The payoff list's frozen-read deletion is NOT licensed by the
   flip alone — measured, not argued.** Disabling the stored-handler
   dep-rebind refusal (`NotifyNameRebound`'s `depHit` arm) fails the
   same tests IDENTICALLY with the regime on and off, so replay is not
   what those tests turn on. The design's F1 counterexample
   (interpreter `6 105 12` vs compiled `12 12 12`) no longer
   reproduces at the VALUE level — but the control that matters is
   `-force-compile`: with the gate disabled those programs do not run
   compiled at all, they hit a VM `CALL_NATIVE_POLY no match`
   internal_error and fall back, so the correct-looking numbers come
   from the INTERPRETER. Deleting the refusal therefore trades a cheap
   early refusal for a wasted VM run plus a fallback, and the deletion
   needs the runtime-lookup half (§6.9's `OpDispatchGeneric` work),
   not the twins'. Re-measure with `-force-compile` before believing
   any "this gate is obsolete now" claim — `-compile` falls back
   silently and will tell you the gate is deletable when it is not.

   **SECOND REHEARSAL (2026-09-01, on merged main): 7 failures, all in
   `lang/go`, and the first one root-caused was a REAL BUG in the
   bridge.** `AdoptResidentTwins`' registry fence compared the noting
   and unit registries against `es.reg` — which is LAST-BIND-WINS,
   re-bound at the top of every `Engine.Run` including module-body and
   island sub-engines, and never restored — where its own doc says
   "the program registry". So a body that merely CALLED a
   module-defined boru fn (the whole `Test.prop` surface: the each
   body calls `Test.run-property`, whose body analysis re-binds
   `es.reg` to the `boru:test` sub-registry) left the fence comparing
   against a foreign pointer, declined an adoption whose registries
   actually agreed, and the program refused for want of the placement.
   Fixed by comparing against `progReg`, captured once at the first
   bind and never re-bound — the same pointer Finalize's `CompiledFn.Reg`
   stamp already uses for this exact hazard. Three `Test.prop` tests
   go green; rehearsal failures 7 → 4.

   Also settled: the `expected 3, got 2` line is NOT a divergence. It
   is `boru:test`'s own stderr report from a DELIBERATELY failing
   fixture (`TestTestBodyFailingAssertParity`), printed twice because
   the harness runs the source on both surfaces and byte-identical
   failure text is what that test asserts. It passes with the flag on
   and off; it merely sat next to the real failures in combined output.

   **And the refusal-REASON item was hiding a live DEFAULT-lane
   miscompile — NUR116.** `def _ (for [1 4] [i]) _` answers `1 2 3`
   compiled and `2 3 1` interpreted, under `-force-compile`, today,
   with no flag involved. `SplitLoopRegionBind` still declines the
   first-value split for `_`/`$` on the premise that such a name
   "records no dyn-bind event" — which the regime's `RecordDynBind`
   falsified. The regime is the SOUND side here, so the fix rides with
   the flip: admit `_`/`$` at the root under the regime, keep
   capitalised names declining (they still record nothing). The
   corpus cannot see it — zero `def _ (for` rows in 124 TSVs.

   The three GOLDEN failures are annotation-only and must be edited
   WITH the flip, not before it: the goldens are inline raw strings in
   `lang/go/bytecode_emit_test.go`, the instruction streams are
   byte-identical between modes (same opcodes, offsets, const indices,
   jump targets), and the only differences are the disassembler's own
   mode tags — `(inert)` → `(replay)` on bind twins and a `(push)`
   suffix on global binds. Editing them before the flip would break the
   default lane, which still emits `(inert)`.

   Still open at the rehearsal's end, for whoever takes the flip:
   golden bytecode diffs (`TestEmitGoldens`,
   `TestEmitMacroExpansionGolden`, `TestEmitModuleCallLowering`,
   `TestZZScratchDisB1` — the regime adds twin ops and push-mode
   binds, so the goldens must be REGENERATED, not patched), the
   `Test.prop` surface (`TestPropSpec*`, three tests, an
   `expected 3, got 2` assertion divergence worth its own
   root-cause), the nested multi-run bodyID fence, and one
   refusal-REASON precedence change (`TestEdgeFindingLoopCollectDefCompiles`
   sees the loop-consume reason where it expects the residual-shape
   one — cosmetic, but the test pins the string).

   **Arm-residency step 0 is LANDED: the cross-request parity oracle**
   (`test/go/langspec/bind_multirun_parity_test.go`). The increment
   switches part of the twins' contract from pass-left fidelity (the
   sandbox-proven invariant every existing gate pins) to INTERPRETER
   fidelity with runtime-dependent count/value/order — and no lane
   measured that: the regime differential compares same-request
   results only, and the full-placement gate is syntactic. The oracle
   enumerates every probed name's full install stack on a fresh
   interpreter instance vs a fresh regime instance (read/undef
   alternation — undef of a missing name is a silent no-op, so reads
   are the drain detector) and pins each shape as measured-parity or
   refused-by-substring; graduation is a reviewed classification edit,
   never drift. Its founding catch was a LIVE regime hole, fixed in
   the same commit: RecordDynBind's historical `_`/`$` name skip left
   a root computed `def _` with a placed carrier-entry twin
   (carrier-class-skipped) and no Push-mode OpBindGlobal partner — the
   binding silently lost cross-request (`def _ ([1 2] each [1]) 9`
   then `_` → undefined_word vs the interpreter's [[1 1]]). The gate
   now admits root `_`/`$` defs under es.twinRegime only; default
   bytecode is byte-identical. The measured each-body semantics the
   graduation must match, from the same instrumentation: one install
   per element per site, stacked in element order with per-element
   runtime values (the ledger's one generalized carrier-valued entry
   cannot replay it — the arm op must carry the runtime value); var
   params net zero per iteration via a balanced Pos-0:0 def/undef
   pair; mid-iteration raise leaves earlier elements' installs.

   **Arm-residency's MECHANISM is LANDED for `each`'s non-var-pair
   population.** `OpBindResident` (Program.ResidentBinds) executes
   inside the compiled per-invocation unit, once per element, with the
   RUNTIME value — install through `core.InstallDef` (the
   interpreter's own installer: per-element repeats stack), peek/pop
   value modes mirroring GlobalBindSpec, no unwind trail (a
   mid-iteration raise leaves earlier installs), regime-only emission.
   The bridge: `CallableSpec.BodyMultiRunKeepsDefs` (each alone,
   handler-verified) + `MultiRunBodyGuard` (a body-identity-keyed
   twin-range latch delegating to BodyAnalysisGuard, so #421's taint
   holds) + `AdoptResidentTwins` (strict total NAME+ORDER pairing of
   the bracket's BindDef twins against the fresh unit's def events —
   any mismatch, leftover, stale memo unit, or foreign registry
   declines everything, sound) + `lowerResidentBind` (sim-top peek /
   pushed-copy pop / inert-literal bake; anything else refuses) +
   `twinsFullyPlaced` counting real resident ops via
   ResidentBinds[arg].Twin. Two extra fences: root reads of arm-bound
   names REFUSE (NoteDefRead poisons `armReadRefusal`, surfaced at
   Finalize's placement seam under the `twin regime:` prefix — the
   fence is regime-only machinery, so it lives in the placement-gate
   layer, NOT as a recorder MarkUncompilable site: the refusal-site
   census counts that layer and its count only falls; definedness is
   body-run-dependent at zero iterations; a later live root install
   lifts it), and the
   `_`/`$` name gate opens inside arm-resident body compiles so
   `def _2 …` gets its event seat. Graduated with measured parity in
   the oracle: each-literal-def, each-zero-iterations,
   each-underscore-def; read-after re-pinned to its own refusal.

   **ROW 41 IS GRADUATED — the regime reaches FULL CORPUS PARITY:
   6416 rows compiled (equal to the default recorder), ZERO
   regime-only refusals, zero divergences.** The final two pieces:
   the var-param pair places BOTH halves in-arm (`__varundef`'s
   handler notes the teardown through `RecordDynUndef` — an
   undef-flagged dyn-bind event recorded only inside arm brackets;
   the bridge pairs kinds strictly, BindDef↔install and
   BindUndef↔teardown; the op's undef arm pops per element — net
   zero per completed iteration, def-half leaked on a mid-body
   raise, interpreter-identical), and element-dependent def CHAINS
   (`def res (if …) def _2 res` — two installs off one producer)
   lower via regime-gated force-promotion: arm-resident body
   compiles force-promote every def's computed source
   (collectDynBindSources' bracket arm), and planBranchPromotion
   treats a forceOrder branch source as a promotion trigger rather
   than dead. STILL REFUSED, deliberately: nested multi-run bodies
   (the bodyID fence), root reads of arm-bound names (their own
   refusal), and fold/scan/filter/outer/inner (unflagged until
   their handlers' leak semantics are verified with oracle rows per
   word — each such graduation is one flag + oracle rows, the
   mechanism is done).

   The sandbox's first client and the data-level proof that preceded the
   flip: `TestBindingSandboxRollbackAndReplay`
   (`test/go/langspec/bind_replay_sandbox_test.go`) runs snapshot →
   compile pass → `RestoreBindings` → replay **from the Program's own
   recorded captures**: each note captures the `DefEntry` it installed
   (`RecordBindTwin(tr, entry)`; `Program.BindTwinEntries`, 1:1 with the
   table), and the harness re-installs those captures in table order —
   the exact contract a twin op executes. Whole corpus: 7644 rows cycled,
   0 failures — after every replayed transition the live depth equals the
   ledger's recorded depth, and the final stacks match the pass-left
   stacks in entry identity (TypeDef pointer, Minted; compared via the
   new `DefTable.Entries`). The skipped rows (undef / def-replace) are by
   design: an undef twin captures NOTHING — at VM time it pops whatever
   is then live and retires a minted type from the popped entry itself —
   and a def-replace twin must reproduce the overlap-drop; both are
   VM-op work, not data-replay work. One operand decision is thereby
   already made and proven: PUSH-kind twins carry their entry, captured
   at the note (the only moment the identical object is knowably on
   top). The remaining operand decision is the COMPUTED value-def class,
   whose captured body is a check-pass carrier, not the runtime value —
   those twins must ride the evDynBind/OpBindGlobal seat machinery in
   push mode instead of replaying the capture.

   Facts established for the flip, each paid for:
   - `OpBindGlobal` today covers only ROOT defs of NON-concrete values
     (lowerDynBind's needGlobal — a concrete `def x 1` emits NO op at
     all), so under rollback every value-def twin needs an operand story,
     not just the write-back class. The def twin's value resolution wants
     to ride the evDynBind machinery (src/srcSeq/val + the promotion
     seats), which already exists per value def — not a second resolver.
   - A JOINED binding's twin replayed unconditionally at the join re-creates
     NUR110's leak at the VM level, but it also exactly REPRODUCES today's
     keep-regime behavior — so flip v1 may replay joins unconditionally
     with no regression, and arm-residency then closes NUR110 as its own
     increment.
   - The ledger's absolute depths are exactly reproducible over the
     rolled-back registry (the harness's per-transition assertion), so a
     twin op can trust its recorded depth at VM time.
   - A SetAt-based interim (widening OpBindGlobal to concrete defs under
     the keep regime) was considered and rejected: for a loop-body def the
     runtime value changes per iteration while the interpreter PUSHES per
     iteration, so the write-back is not behavior-neutral there — the flip
     changes semantics coherently or not at all.
   - The COMPUTED-value twin class coincides exactly with lowerDynBind's
     `needGlobal` predicate (root && !IsConcrete && !IsBareTypeNode): a
     const-folded `def x (1 add 2)` records a concrete 3 (verbatim class),
     and a bare-node body is self-representing — so at the flip, "skip the
     BIND_TWIN whose captured entry matches needGlobal's predicate, and
     flip that def's OpBindGlobal from SetAt to Push" partitions the def
     population with no third case.
   - **`BindDefReplace` HAD two producers with different replay semantics
     — split DONE (2026-08-31, `BindSigUndef`).** The conflation was not
     theoretical: probed live, a plain two-overload fn's signature undef
     removed an entry (depth 2 → 1) while recording delta 0, and the
     composition gate read clean because the corpus lacks the shape
     entirely (the blind-spot class again; the synthetic rows now supply
     both it and the locked no-op counterpart). `UninstallFnSigs` now
     commits and notes each removal individually as `BindSigUndef`
     (delta -1, the note carrying the REMOVED entry via
     `NoteBindTransitionEntry` so a twin can remove that identical entry
     mid-stack), and notes NOTHING when every match is locked — a no-op
     is not a transition. `BindDefReplace` keeps exactly one producer:
     installDef's drop-then-push redefinition.
   - **Islands re-execute; twins must not double-apply there.** A do-body
     compiled as a FALLBACK island re-runs its tokens in a sub-engine at
     VM time, executing any `def` inside it for real — so a twin event
     discarded with the island's events is CORRECT, not a placement gap.
     The unplaced-twin refusal the flip needs must therefore distinguish
     "discarded into an island that re-executes the transition" (sound —
     do not refuse, do not re-apply) from "noted under a suspended
     recorder with nothing to apply it" (refuse). Conflating them either
     double-installs do-body defs or refuses every islanded program.

**Do not invert that staging.** Rollback-first breaks every program until
the last transition has a twin, and offers no intermediate state where the
differential means anything. This is the same reasoning that made Stage 2
land its three re-seats separately.

## The strong oracle: landed, GREEN, no allowance (2026-08-31)

`Boru.NativeRegistry()` exposes the live registry, so the ledger's final
depth per name is compared against the depth the check pass **actually
left**. That is a strictly stronger check than `TestBindLedgerDepthsCompose`'s
self-composition, and it is the assertion step 2 above needs. It is landed as
`TestBindLedgerLiveDepths` (`test/go/langspec/bind_ledger_live_oracle_test.go`),
over the corpus AND the synthetic rows, at **0 mismatches with no allowance**.

The 9 mismatches closed as THREE fixes, and both prior readings of them were
wrong in ways worth keeping:

- **Gensym temps were not "macro construction"** — they were two
  snapshot/restore-truncated eval regions whose installs the ledger recorded
  while their own `Restore` tore them down: `expandMacroWith`'s template run,
  and the DYNAMIC-HELP example eval (`makeDynamicEval`), which fires mid-pass
  from the fn-registration hook and was the phantom "construction-time
  expansion" (found by stack-tracing the note, not by reading). Both now
  bracket with `CheckState.SuppressBindLedger` — the truncation rule the body
  runner already carried, applied at two more truncation sites.
- **The module-io multi-pass hypothesis was tested first, and it was wrong**
  — the doubling reproduces in a single pass and at plain runtime. The real
  cause was a genuine leak: `narrowDynamicUses`' analysis push had no
  top-level popper, so the leaked carrier SHADOWED the real binding for later
  Runs on the same instance (`typeof l` answered `dynamic(Ideal)`, not
  `Lock`). Fixed with a guarded pass-end pop (`CheckState.PassEndCleanups`);
  pinned by `lang/go/narrow_leak_test.go` and `check/go/narrow_passend_test.go`.
  So the warning above ("the oracle may be the fault") had the right posture
  and the wrong mechanism: the oracle was RIGHT, and what it found was a
  user-visible bug, not a bookkeeping gap.
- **The synthetic while row was itself malformed** — `while (n gt 0) […]`
  hands `while` a Boolean its `(List, List)` signature refuses, so the row
  never exercised a loop and "passed" composition while measuring nothing
  (the same lesson as the corpus blind spot, one level up: a gate's OWN
  synthetic input can be the thing that cannot contain the case). Corrected
  to `[n gt 0]`, the oracle immediately showed `AnalyseLoopBody`'s final
  joined post-loop pushes bypassing the ledger (raw `r.Defs.Push`, no note).
  They are now ledgered — the loop join is `InstallJoinedDefs`' one-branch
  rule and records like it.

Census after: 7453 → 7443 (ten phantom truncated-region entries out,
loop-join entries in); → 7441 after the BindSigUndef split (two locked-match
no-op notes gone).

**STRENGTHENED 2026-08-31 (Codex review round on PR #418), and the stronger
form found four more real defects.** The oracle now checks BOTH directions:
ledgered names against live depth (as before), and — for rows that COMPILED
— every name whose live depth changed across the pass with NO ledger entry
(the direction the first cut could not see; a wholly-missed transition
reported as success). Two documented scopings keep it honest rather than
tolerant: the unledgered direction runs only where a Program exists (a
refused or erroring row legitimately abandons partial state no twin will
replay — `gen [T] gen [U] …` raises with the outer binder still pushed, in
both engines), and `__`-prefixed interner bindings (`__const:`, `__gen:`)
are §6.5's own compile-time products. What the stronger form caught, each
fixed rather than excluded:

- **The both-arms branch join never noted.** `InstallJoinedDefs`' both-arms
  push (`if c [def op 1] [def op 2]`) had no `NoteBindTransition` at all —
  the function's own doc claimed every push noted — so a live binding with
  no ledger entry, invisible to the ledgered-names-only walk.
- **Narrowing-only "adds" recorded phantom defs at BOTH joins.** An arm or
  loop body that merely CONSUMED a dynamic name through a typed slot grew
  the def stack via `narrowDynamicUses`; the join re-pushed it and noted a
  `def` for a name the source never defines — and the joined carrier leaked
  past the pass (the staleness bug's join-level sibling). The discriminator
  is exact because narrowing preserves the value's ID: an add whose ID
  equals the pre-binding's ID is the same runtime value under a tighter
  bound — skipped at both `InstallJoinedDefs` and `AnalyseLoopBody`, no
  push, no note (`narrowedSameBinding`).
- **The truncated-region restores could not undo pops.** `Defs.Snapshot` /
  `Restore` is depth-based — restore can only TRUNCATE — so a macro
  template or help-eval that ran `undef` on a pre-existing name (or an
  overlapping redefinition) escaped the region while `SuppressBindLedger`
  suppressed its note. Fixed with the new CONTENT-preserving, IN-PLACE
  `DefTable.SnapshotEntries` / `RestoreEntriesSnapshot` (gen-guarded, so
  untouched names see zero cache churn and restored gens move forward).
  **A hazard paid for on the way: `RestoreBindings` — the table SWAP — is
  only safe at the between-phases point.** Used inside these nested
  regions it broke every reference the enclosing pass holds across the
  region, measured as ~200 corpus rows' ledgered defs vanishing from the
  post-pass registry. The flip's runtime client uses it exactly once,
  between CompileCheck and RunProgram, never nested.
- **The narrowing cleanup's pop guard needed the push-time DEPTH in its
  identity token** (Codex's P1): a later check-mode rebind can bind a
  carrier `ValuesEqual` cannot tell from the narrowing's own value, and
  value equality alone would pop that real binding; the depth rules the
  impostor out.

The loop-join POSITION limitation stands as recorded (every joined name
gets the note-time CurWordPos floor — a real site inside the body, not each
def's own): §6.5 already names the join position as the one to revisit, and
the flip's arm-resident twins replace join twins wholesale, at which point
each arm def carries its own position.

## Stage 4a: the region table, inert (2026-09-03)

The twin line is finished; this is the first increment of Stage 4's
dispatch half, and it deliberately repeats the twins' own method — **emit
the table while nothing reads it, and assert it against the whole corpus**
— so the first descriptor `OpCollect` executes is one the corpus has
already exercised.

**What the target was NOT, and why that matters more than what it was.**
The obvious first slice is "collect the simplest dispatches" — every slot a
const, hand the window to the existing opcodes. That slice was already
built once and reverted, and §6.2 records why: `TestEmitSplitFormsIdentical`
requires `1 add 2`, `1 2 add` and `add 2 1` to lower identically, because
the forward/stack split is SURFACE SYNTAX the compiler normalises away. A
descriptor carries that split, so routing the forward spelling and leaving
the stack spelling on `CALL_NATIVE` lets syntax survive into the bytecode.
The design's conclusion is the one to build against: **the target is not
dispatches that can be described, it is dispatches the ordinary lowering
CANNOT HANDLE** — and of §6.5's latches, exactly one is a region's
business, the frozen-read `k` pair. Verified at the CLI before any code was
written:

	def w fn [[a:Any b:Any][Any][a]]  def k 5  def go fn [[][Any][w k 1]]  go
	→ 5 on both lanes
	… then `def k fn [[][Integer][9]]  go`
	→ compiled: REFUSED, "module binding k rebound after a fn unit baked its value"
	→ interpreted: raises, `w` is still waiting for 2 arguments when `k` begins its own dispatch

That pair is `OpCollect`'s acceptance test and it is still open. This
increment is the table it will read.

**What landed.** `Program.Regions` has its first writer. Phase B
(`compiler/go/region_complete.go`) claims the Phase-A capture at
`RecordCall` and fills the sources of the slots the dispatch actually took
forward; `RegionDesc.NFwd` records where the claim stopped; `Validate`
permits the invalid zero beyond it, with a boundary (an unsourced slot
carrying an INDEX is still a defect, and `NFwd` is range-checked). Nothing
reads any of it. `make verify-bytecode` stays byte-identical because a side
table is not code.

**Four things the corpus said that the design did not.**

1. **Phase B is an INDEX, not a search, and that is why it works.** Three
   models were reverted for searching a written-order slot for its
   already-resolved operand. The rule removes the search: matching fills sig
   positions from the forward tokens IN WRITTEN ORDER, so over the leading
   positions the two orders are the same order and slot *i* is sig position
   *i*. The source is `ops[i]`. What the reverted models got wrong was
   assuming the correspondence instead of checking it — completion compares
   each slot to its operand by VALUE IDENTITY (not structural equality: `add
   1 1` would coincide) and stops at the first that does not.

2. **A word slot's answer is decided by where its BINDING lives, not by
   where the dispatch sits — and getting that wrong is a miscompile.** Inside
   a fn body the analysis binds params and body-local defs into the def stack
   so the body can be analysed, so `a` in `[add a b]` resolves during the
   pass — but the emitted body reads it from the FRAME, and at run time the
   def stack holds no such binding.

   The first cut keyed the exception on the OPERAND (take the source when it
   is a frame local), which was right for params and wrong twice over, both
   found in review. A body-local `def x 1` emits `PUSH_CONST` while the name
   exists nowhere at run time, and a computed one is promoted to a frame slot
   only AFTER completion — either way the slot kept a confident `SlotWordRef`
   describing a lookup that will miss or find an unrelated outer binding. And
   keying it on "inside a fn unit" instead would have been worse in the other
   direction: it deletes the `k` pair's own descriptor, which is the shape
   OpCollect exists for.

   The discriminator is the CLOSURE-CAPTURE rule, verbatim:
   `Defs.Depth(name) > FnBaselines[top][name]` means the name lives inside an
   enclosing fn; equal means module scope. Captures and descriptors ask the
   same question — will a live lookup still find this name where the body
   runs — so they must not answer it two ways. A frame-bound name takes
   `SlotLocal` when the operand names its slot and STOPS the claim otherwise;
   a module-scope name stays live. The pair that pins it is one name at one
   position with two scopes:

	def k 5  def f fn [[][Integer][add k 2]]              → NFwd 2, slot 0 wordRef
	def k 5  def f fn [[][Integer][def k 9  add k 2]]     → NFwd 0

   Measured: of 6361 claimed word-token slots only **848 are live wordRefs**,
   5513 are frame slots, and roughly 2960 more stopped the claim rather than
   describe a binding the runtime will not have.

2b. **A word slot resolves against the DISPATCH's registry**, not the
   recorder's `es.reg` — which is the last registry `BindRegistry` saw, and
   after a call into a boru-implemented module that is the module's
   sub-registry. Measured: `M.m 5 end add x 2` recorded `add` with NFwd 0
   because `x` was sought in M's table. The registry now rides the Phase-A
   offer. It must never reach `RegionDesc`: a Program is shared and a run may
   be handed a different registry fork, which is why `Finalize` declines to
   stamp one onto ordinary fn units.

3. **A recorder-side table is not rollback-safe.** The first cut appended
   descriptors to a slice on the `EmitState`. `Rollback` does not truncate
   it — a discarded loop-analysis round leaves its descriptors behind, and
   once the table is indexed that shifts every later index. Moving the
   append to `lowerCall`, where `DispatchSpec` already lives, removed **55
   descriptors** from the corpus table, every one of them a round that was
   never lowered. The descriptor now rides its call EVENT, which is what
   puts it under the existing rollback and gives the lowering site the link
   from a call in unit K to its own descriptor.

4. **The claim is a small prefix of the region.** 22958 of 65959 span slots
   claimed; 17650 descriptors claim nothing forward at all, 5292 a prefix,
   15792 the whole span. `NFwd` is not a micro-optimisation — §6.2 measured
   the alternative at +9.9% on `arith_chain64`, because a region runs to the
   next hard delimiter and classifying all of it once per dispatch is
   quadratic.

**The table's stated bound.** Phase B is seated on `RecordCall` alone — the
mono native dispatch. `RecordUserCall`, `RecordPolyCall`,
`RecordUserPolyCall`, `RecordDynApply`, `RecordDynMethod` and the drift
window each record an event with no descriptor. Corpus-wide that is 38734 descriptors
over 5629 programs — a fraction of the dispatches Phase A offers a capture
to. The bound is pinned, not merely noted: `lang/go/region_capture_e2e_test.go`
fails if a user-fn call starts producing a descriptor without the census's
claim being widened with it.

**What is next, in order.** (a) Lift the seat to the other record families,
or decide per family that it stays out. (b) The eng-side `CollectHost`
adapter over the descriptor — `eng/go/collect_seam_host_test.go` is already
a foreign host driving all three kernel entry points, and it is the
template; the adapter goes in `eng`, above `cover-gate-core`, because a
descriptor-typed function in core is a module cycle, not merely a coverage
problem. (c) `OpCollect` executing, routed ONLY at the frozen-read refusal,
with the `k` pair as its acceptance test. Two traps waiting there:
`core/go/collect_kernel.go:714` and `:757` are `//covergate:allow` guards
that a descriptor-backed window can make reachable — the merged gate fails
them as `nowCovered`, and they must be graduated in the same change; and
`RegionState`, the per-execution raise-selection state, still has no
producer, so a first `OpCollect` that produces a WINDOW must not claim the
raise path.

## Stage 4a-2: what lifting the frozen-read guard actually shows (2026-09-03)

Item (b) above is landed — `eng/go/region_host.go`, the VM's `CollectHost`
over a `RegionDesc`, classifications delegated to core and every evaluation
declining. Item (c) is where this section corrects the plan, because the
measurement contradicts the premise (c) was written on.

**The experiment.** Suppress the `MarkUncompilable` at
`compiler/go/emit.go:3158-3160`, rebuild the CLI, run under
`-force-compile`, revert. Not a thought experiment — the guard is one `if`,
and what it is holding up is directly observable.

| program | interpreter | compiled, guard lifted |
|---|---|---|
| `def k 5  def f fn [[] [Integer] [k add 2]]  f  def k 9  f` | `7 11` | **`7 7`** |
| the same with `undef k` | raises `undefined_word` | **`7 7`** |
| the same with `def k "x"` | raises `type_error` on f's return | **`7 7`** |

**The guard is holding up THREE miscompiles, not one.** The plan above
named only the first.

**And the answer is not one an operand can supply.** The disassembly of the
first row, guard lifted:

	0003 BIND_TWIN   w2   ; bind twin def k @depth 2 (replay)
	fn f0 f/0 (locals=0):
	0000 PUSH_CONST  k1   ; 5 (Integer)
	0001 PUSH_CONST  k0   ; 2 (Integer)
	0002 CALL_NATIVE s0   ; add (Number, Number)
	; sites: mono=3 poly=0

Two facts fall out, and the second is the expensive one.

- **The bind twin already replays the rebind correctly.** `k` IS live at run
  time — pc 0003 is the twin doing its job. It is the READ that was baked.
  So the missing piece is narrower than "make the rebound value reachable";
  that half is finished and was finished before this line started.
- **The call site is MONO,** and this is the half the plan had not priced.
  `add (Number, Number)` was selected at compile time FROM the frozen `5`,
  and `OpCallNative` invokes THAT signature's handler directly (`eng/go/vm.go`
  — `s := p.Sigs[in.Arg]`, then `s.Sig.DispatchHandler()`); it never
  rematches. So an `OpCollect` that supplies a live operand and leaves the
  recorded signature alone does not reproduce the interpreter on the third
  row by any route: it either trips `checkNativeParamContract` into a
  `signature_error` where the sig carries `Guard`, or hands a String to the
  numeric handler where it does not. The INTERPRETER rematches, finds
  `add`'s String arm, concatenates to `'x2'`, and is caught only by f's
  declared `[Integer]` return. Selecting that arm is precisely the extra
  decision `OpCollect` owes — and it may not re-derive it from the bare
  window: `dispatch_agreement_census_test.go` states the generic lane must
  re-create the planner's selection from the descriptor, never from the
  window.

**The corpus cannot see any of this.** Running
`TestSpecCompiledDifferential` with the guard suppressed **passes** — no
corpus row carries a frozen read followed by a rebind, exactly as the
population measurement predicted. This is the "green gate over a corpus
that cannot contain the case" lesson in its purest form: the only thing in
the tree that catches the guard's removal is
`lang/go/frozen_module_read_test.go`, a hand-written pin.

That pin covered two shapes (the scalar read and the splice twin). It now
covers four — the `undef` arm of `NotifyNameRebound`, which nothing
exercised, and the cross-family rebind, which is the row that will refuse
to let a future routing step supply an operand while keeping a stale mono
selection. Each was verified non-vacuous the only way that means anything:
suppress the guard and watch the new row fail.

**Revised order for (c).** Not `OpCollect` first. `Finalize` returns at
`emit.go:8685` on a latched refusal, BEFORE the lowerer runs, so while the
latch stands it blinds every descriptor from anything downstream. The
order is: pin the guard (done, above); then give `frozenReads` — today a
`map[string]bool` that throws away WHICH site the name fed — enough
structure for a router to ask "which operand froze `k`?"; then convert the
latch into an obligation discharged at `Finalize` against the finished
artifact, which is behaviour-identical and is the only state in which a
corpus differential over this shape means anything; then `OpCollect`.


## Stage 4a-3: seven claims sent for verification, seven came back PARTIAL (2026-09-03)

The revised order above names the next increment as "give `frozenReads` its
site structure". Before any of it was written, the load-bearing claims behind
it were sent out to be checked against the tree rather than transcribed.
**Every one came back PARTIAL** — none simply wrong, none simply right — and
one of the corrections was not a design note at all but a live miscompile.
The corrected statements are below; the original readings are recorded with
them, because in four cases the original is the reading a later increment
would naturally re-derive.

### The one that was not a design finding: a const-pool type collision

`core.CanonValue` renders a value through its nearest BASE-type arm
(`core/go/canon.go` — the `v.Parent.ConformsTo(TInteger)` case returns the
digits and nothing else). That is right for canon's own jobs, and wrong as a
const-pool key: two values whose only difference is their nominal type render
identically, so `intern`'s canon pool merged them into one `Consts` slot and
whichever was interned FIRST donated its `Parent` to the other site.

	def Pos (refine Integer)  def x:Pos 42  typeof 42  typeof x
	  interpreted -> Integer Pos        compiled -> Integer Integer

Not a `-force-compile` curiosity: `-compile` falls back only on a REFUSAL and
this program never refused, so the wrong answer was what `boru run` printed.
Both orderings diverge, and so do two distinct refinements of one base
(`Pos`/`Neg` both rendering as `7`). Fixed here by keying the canon pool on
the type ID as well as the rendering (`constPoolKey`, compiler/go/emit.go),
pinned by `lang/go/const_pool_type_test.go` in both orderings plus a negative
that fails if the fix ever degenerates into "stop pooling".

Worth stating because it is the second time this has bitten in the same
place: **`CanonValue` is a VALUE identity, never a type identity, and never a
site identity.** The comment at compiler/go/emit.go:2207 still asserts
"Compounds are never pooled (intern), so idx belongs to exactly this
materialise context" — false since `constIDIdx` landed, and any reasoning
that cites it as a per-site guarantee is unsound.

### The six corrections that stay design findings

**1. A const index cannot be a baked-site identity.** Confirmed, and the
classification is wider than "scalars pool by canon": `intern` has FOUR
classes — `FnDefInfo` unpooled unconditionally; the identity class (extension
/ xml payloads, List, Map, type bodies, ParenExpr) pooled by `Value.ID` ONLY
when the ID is non-empty and unpooled when it is; everything else pooled by
the canon key; and `internUnpooled` as a separate entry point for the
`OpBindDynScope` name. So a const index is a value identity under two
different rules and a site identity under none.

**2. The open-unit index is the sound anchor — on the state that will be
Finalized, and nowhere else.** `openUnitRecs[len-1]` is an index into
`fnRecs`, which is append-only, which `Rollback` refuses to unwind once a
round has grown, and which `Finalize` walks one-for-one into `Program.Fns`.
That all holds. What the original reading missed is that the RECORDER IS
SWAPPED at five probe boundaries, and two of them install a fresh
`NewEmitState()` whose `fnRecs` is empty — so an index taken during such a
probe is `0`, numerically colliding with a real and unrelated Program unit.
Any per-unit structure must therefore be written only on the state that will
be Finalized, or explicitly merged back on probe success. There is no such
merge today.

**3. `frozenReads` outliving a `Rollback` is DELIBERATE, not a gap — and the
earlier note in this file had the sign backwards.** It is absent from both
the checkpoint and `Rollback`, and the discard shape is reachable. But it has
exactly one consumer, whose only effect is `MarkUncompilable`; `Rollback`
never deletes from it; so a retained entry can only ever cause OVER-REFUSAL,
never a miscompile. The miscompile direction is a MISSING entry, which
`Rollback` structurally cannot produce. Adding `frozenReads` to the
checkpoint as a "gap fix" is therefore FORBIDDEN: it would convert a
conservative refusal into a silent wrong answer. The same holds for every
other monotone refusal-direction field.

**4. The latch is guarded by nothing that names it.** No test in the tree
pins its reason string — the text appears only at the `MarkUncompilable`
call, one comment in `compiler/go/region_desc.go`, and three design docs.
`MarkUncompilable` is FIRST-REASON-WINS, so the latch could be deleted
outright and `TestModuleReadRebindRefusesAndMatches` would still pass,
provided any other refusal fired first on those four rows — including a
refusal introduced by the very change under review. The pin added earlier
today closes the shapes; it does not close this. (Also: the "96 refusal
sites" figure is the CENSUS's line metric. Live `MarkUncompilable` call
sites are 95; production-compiler ones 93.)

**5. Corpus population is ZERO, not three.** No corpus row exercises the
latch — measured directly, not inferred from the differential passing. Three
rows carry the read-inside-a-unit-then-rebind SHAPE and none can trip it,
because the shape has FOUR ingredients and every one of the three is missing
the third: `def` -> read inside a unit -> **the unit is CALLED** -> rebind. A
fn unit is analysed at a CALL site, so `frozenReads` stays empty until the
call happens. Write the four-ingredient shape into any future note; "no row
carries a frozen read followed by a rebind" is right about the outcome and
silent about the reason.

**6. There is no READ-SITE position on the recorder side, but the recorder is
not position-free.** `NoteFrozenRead` takes only a name and the refusal path
is positionless end to end. `CheckState.CurWordPos` must NOT be used to fill
the gap — it names the enclosing `def`/`fn`, which is a confidently wrong
caret of exactly the kind `check_state.go` documents. Two things ARE
available: the enclosing unit's own position, already in scope where the note
is taken; and the read's true position at the core call site, as `stepWord`'s
`val` parameter, one line below where `top.ID` is already handed to
`NoteDefRead`.

**7. The latch predicate is not what is wrong — its INPUT is.** The two
conjuncts faithfully report what they are given. `frozenReads` is populated
in core on `IsConcrete(top) && ModuleScopeBinding(...)`, i.e. on whether the
CHECK-MODE value happened to be concrete at the read, used as a proxy for
"the unit BAKED it". The bake/live decision is made later and independently
in `resolveOperand`. The proxy is neither necessary nor sufficient, which is
why the shape errs in both directions at once — and why a fix that edits the
two-conjunct expression fixes neither. The next increment's real subject is
that proxy.

### Citation rot, fixed here and measured beyond here

Three references this line owns pointed at line numbers that had moved: the
fn-unit latch and the stored-handler twin were both cited at positions now
occupied by unrelated code. Corrected, and now written as `file:line
(symbol)` so the symbol survives the next drift.

The wider rot is NOT swept here, only measured: `eng/go/emit.go` — a path
that has not existed since the four-module split — is cited **52 times** in
`design/REFUSAL-CLOSURE-S94-AUDIT.10.md` and appears in ten further design
files. That is a mechanical sweep of its own, and bundling it into a findings
commit would bury both.


## Stage 4a-4: the freeze discipline was guarding one binding kind of three (2026-09-04)

The revised order above names "give `frozenReads` its site structure" as the
next increment, and Stage 4a-3's finding #7 sharpens it to "the real subject is
the proxy". Both were sent out to be measured against the tree before anything
was written, the same method Stage 4a-3 used. **The proxy is not what was
wrong. What was wrong is that two of the three things a unit can freeze were
not being noted at all**, and each was a live, silent miscompile on the DEFAULT
lane — `boru run`, no flags, exit 0, wrong number.

### The three bakes, and the two that had no gate

A compiled unit can freeze a module-scope binding three ways. `resolveOperand`
has exactly one value-bake chokepoint and one type return; the third is not an
operand at all, it is the call target the lowering chose.

| the unit bakes | as | had a gate before this |
|---|---|---|
| the read's VALUE | `PUSH_CONST` | yes — the original freeze discipline |
| the read's TYPE identity | `PUSH_TYPE` | **no** |
| the CALL TARGET | `CALL_USER` / `TAIL_CALL_USER` | **no** |

```
def k 5                    def f fn [[] [Integer] [k add 2]]   f  def k 9       f
def T Integer              def f fn [[] [Boolean] [5 is T]]    f  def T String  f
def g fn [[][Integer][1]]  def f fn [[][Integer][g]]           f  def g fn [[][Integer][2]]  f
```

The interpreter answers `7 11`, `true false`, `1 2`. The first refused and fell
back. The second and third compiled and answered `7 7`… no — `true true` and
`1 1`. Each `undef` twin is worse in the same way every time: the compiled
program ANSWERS where the interpreter raises `undefined_word`, because the
baked artifact outlives the binding that produced it.

**The call-target row is the one to take seriously.** It is not an exotic
shape — define a helper, define a caller, call it, redefine the helper:

	def helper fn [[x:Integer] [Integer] [x add 1]]
	def use    fn [[x:Integer] [Integer] [helper x]]
	use 1  def helper fn [[x:Integer] [Integer] [x add 100]]  use 1
	  interpreted -> 2 101      compiled -> 2 2

That is a REPL session and it is what `reload` does. The disassembly is the
§6.5 lesson in four lines — the twin replays the rebind correctly at pc 0003
and the call target does not move:

	0003 BIND_TWIN   w2   ; bind twin def-replace g @depth 1 (replay)
	0004 CALL_USER   f0   ; f/0
	fn f0 f/0: 0000 TAIL_CALL_USER f1   ; g/0     <- the OLD unit
	fn f1 g/0: 0000 PUSH_CONST k0 ; 1

### The root cause is structural, and it will recur

`NotifyNameRebound` is called from HANDLERS, not from the binding store. Before
this increment there were two call sites, both in `basic/go/native_definition.go`,
both on the lowercase arms of `def` and `undef`. Every other binder returned
before reaching one:

- `DefHandler`'s capitalised arm returned `core.InstallType(...)` directly.
- `UndefHandler`'s capitalised arm returned after `NoteBindTransition`.
- `UndefFnHandler` returned after `UninstallFnSigs` — no notification of any kind.

And on the READ side the same shape: a type name never travels `stepWord`'s
simple-value substitution branch (the type-literal arm returns first), so
`NoteFrozenRead` was never even attempted for it; a fn name falls through to
`Lookup` and dispatches, so it was never attempted there either.

**So the discipline was never wrong; it was never asked.** A rebind
notification attached to handlers is a notification each new handler can
silently forget, and each forgetting is a miscompile generator.

**The durable fix LANDED — see "The funnels" below.** Both halves are seated
now: the notification on core's binding OPERATIONS, the read note on one
classifier, and `core/go/rebind_funnel_test.go` fails if either is bypassed.

### What landed

Both missing arms of the discipline, on both halves — the note and the
notification — plus the table's first structure. `frozenReads` is now
`map[string]core.FrozenBake` rather than `map[string]bool`: value, type, or
call target, first bake winning, and the refusal NAMES it ("… baked its call
target"). That is the site structure the revised order asked for, in the form
the defects actually demanded rather than the form the plan guessed: a router
asking "can OpCollect answer this one?" needs the KIND before it needs the
position, because the three kinds are repaired by three different mechanisms
and only one of them is a region's business.

**Cost, measured: zero.** `test/go/langspec/compiled_coverage_test.go`'s
refusal ceiling stays at 0 across all three arms — the corpus contains none of
these shapes, exactly as the population measurement predicts. The over-refusal
this buys is real but conservative and bounded: adding a NON-colliding overload
(`def g fn [[x:Integer] …]` beside a 0-arg `g`) now refuses a program the two
lanes agreed on. Self-recursion does NOT refuse — `fact` reading its own name
inside its own unit was the shape to check first, and it keeps compiling.

### A THIRD instance of the same fault, found by review

`T/v` and `k/v` resolve through `stepWordVal`, which reaches NEITHER of
`stepWord`'s substitution branches — so both bakes escaped the latch while
`resolveOperand` baked them exactly as it bakes the plain spelling. Codex
raised the TYPE half on this line's own PR; measuring it showed the VALUE half
beside it, and that one is OLDER than the type arm — it had been open since the
freeze discipline first landed.

	def T Integer  def f fn [[] [Boolean] [5 is T/v]]  f  def T String  f
	  interpreted -> true false      compiled -> true true
	def k 5  def f fn [[] [Integer] [k/v add 2]]  f  def k 9  f
	  interpreted -> 7 11            compiled -> 7 7

Three instances now, and they say the same thing in three registers: the note
is attached to READ PATHS the way the notification is attached to HANDLERS, so
every path that resolves a binding its own way escapes it silently. **The sweep
above did not find this one** — it varied the binding kind and the rebind site,
and never varied the SPELLING of the read. When the next author extends the
matrix, spelling is the third axis.

### The spelling axis, swept after the fact

Once `/v` showed that the SPELLING of a read is a real axis, seventeen further
spellings were run through both lanes. All clean, which is what lets the
call-target and type arms be stated as closed rather than merely un-falsified:

- `k/q` and `T/q` (quote — an Atom, no bake), `k/s` (an arg-order modifier),
  `(k)` (paren), and `word k` (the splice, already pinned);
- a module TYPE in a PARAM annotation (`fn [[x:T] …]`) and in a RETURN
  annotation (`fn [[] [T] …]`) — the two positions where a type name is read
  by the signature rather than by the body;
- a type reached through a `refine`, a class FIELD type, `make T`, an
  `fnsig` over it, a `tor` union of two module types, `Type of [T/q]`, and a
  type stored in a list and read back out;
- a type read transitively, through a second fn the caller invokes;
- `typeof 5 eq T`, and a type embedded in a map read through dot access.

Zero divergences. Three axes are now swept — binding kind, rebind site,
read spelling — and every cell is now closed.

### How far the sweep went, so the next author does not repeat it

The matrix above (three bakes x two rebind sites x def/undef) was not the whole
sweep. Fourteen further shapes were run through both lanes and are CLEAN, which
is what licensed treating the `do`-body arm (closed below) as the sole
remainder rather than the first of many:

- rebind inside an `if` arm, an `each` body, and a `for` body — all three bakes
  where applicable (family L's refusal and the arm-resident machinery own these);
- word EXTENSIONS (`def add fn [[a:Flag b:Flag] …]`, a dispatch binding pushed
  without passing through `installDef`), both the re-extend and the
  signature-undef spelling;
- a module namespace read inside a unit (`MathUtil.sqrt`);
- compound value rebinds — map, list and string — which route through the value
  arm as expected;
- a class-type rebind;
- a two-level call chain where the rebind is below both units.

Zero divergences. The sweep harness is a dozen lines of shell around
`boru run` vs `boru run -no-compile`; it is worth rebuilding rather than
reasoning, because three of this session's four findings came out of it and
none came out of reading.

### The same mistake, one layer down, caught by the same method

Worth recording because it happened WHILE writing the fix for it. The
call-target note went in inline at `recordUserCallOrApply` — one of the
`RecordUserCall` call sites — which is structurally the arrangement the root
cause above condemns: a note attached to CALL SITES rather than to a funnel.
It missed the second site, `BuildFnBodyReturnsFn`'s ZERO-OUTPUT
`RecordUserCall`, and a 0-output fn kept baking a stale target:

	def g fn [[] [] [print 1]]  def f fn [[] [] [g]]  f  def g fn [[] [] [print 99]]  f
	  interpreted -> 1 99      compiled -> 1 1

**And the differential is structurally blind to it**, which is the part to
carry forward: a 0-output fn leaves no value, so both lanes return `[]` and
the divergence is only on stdout. No value-comparing gate can see this family.
Found by re-reading the diff against the enumeration of `RecordUserCall`
callers rather than by any test. The note is now a function every site calls,
and the pin asserts the REFUSAL rather than a value, since there is no value
to assert.

### The REBIND-SITE axis, and why the record of it was wrong twice

This arm was recorded as NUR117 and deferred, on the reading that the latch's
`len(openUnitRecs) == 0` guard exempted it. Measuring it before writing the fix
falsified that reading, and then a second one — so the record is Resolved and
deleted, and what it got wrong is kept here, because both mistakes are the kind
a reader would re-derive.

	def k 5  def f fn [[] [Integer] [k add 2]]  f  do [def k 9]  f
	  interpreted 7 11        compiled 7 7

**WRONG THE FIRST TIME: the guard was not the exemption.** Instrumented, the
deciding call arrives with `openUnitRecs` EMPTY and `suspended` at 1 — the
guard would have passed. `KeepDefsBodyGuard` SUSPENDS for the body run, so
`NotifyNameRebound` returned at its `!Active()` line and never consulted the
table at all. The fix is therefore a PLACEMENT, not a predicate: the
frozen-read latch now runs ABOVE that early return, and everything below it
still needs `Active()` (the stored-ref poisoning walks refs this suspension is
not part of).

**WRONG THE SECOND TIME: the multi-run arm was not covered.** The record said
an each/fold body's install "is per-element and already has its own machinery
(`armBoundNames`, the arm-resident twins)". It is not per-element — the body
leaks its LAST iteration's def to module scope — and `armBoundNames` only
refuses a later TOP-LEVEL READ of the name. Reach the frozen unit through a
CALL instead and nothing sees it:

	def k 5  def f fn [[] [Integer] [k add 2]]  f  [1] each [def k 9  k]  f
	  interpreted 7 [9] 11      compiled 7 [9] 7

Found only because the `do` fix's own negative controls were being written; it
is not a shape the earlier matrix contained.

**What the fix actually asks.** "Is this rebind one the runtime performs
against the module-scope binding set?" — and the answer is a THREE-way split
over WHY the recorder is suspended, not over what is open:

	suspended == keepModuleDepth + multiRunModuleDepth   (with no unit open)

Both counters are raised only by body guards whose defs LEAK, and only at
module scope — each reuses its guard's existing `FnBodyDepth == 0` publication
gate, so a `do` or an `each` inside a fn body raises neither and stays
correctly exempt. Any other suspension in flight — a fn body, a branch arm, an
each nested inside a do — raises `suspended` alone and breaks the equality.

**That equality is what makes the change ADDITIVE**, which is the property to
preserve if this is ever touched again: it can only ever ADD the refusals it is
for, never remove one the latch already made. The `if`-arm rebind, which fires
today with `suspended == 0`, is unaffected.

Measured cost: the corpus refusal ceiling stays at **0** with both arms.

### The funnels: the structural fix (2026-09-04)

Six miscompiles in this stage, and every one of them the same shape — a binder
or a read path that never reached the discipline. Fixing the six instances left
the CLASS open, so both halves are now seated on funnels
(`core/go/rebind_notify.go`).

**The notification** moved off basic's `def`/`undef` word handlers onto core's
binding OPERATIONS: `installDef` (under `!shadow`), `UninstallDef`,
`InstallType`, `UninstallFnSigs`, and a new `UninstallType` — the capitalised
undef, which had been written inline in the handler and was thereby the one
unbinder notifying nothing at all. A word library cannot bind a name without
calling one of these, so it cannot skip the notification by construction.

**Two floors were rejected, and the reasons are the design:**

- **`DefTable.Push` and friends** are too low. They carry frame bindings, guard
  narrowings, generic parameter installs and carrier joins as well as user
  rebinds, so a narrowing push at module scope (`if (k is Integer) […]`) would
  refuse every program with a frozen `k`. The SEMANTIC binding operation is the
  level at which "the user rebound this name" is actually true.
- **`NoteBindTransition`** looks like a free ride — it is already seated at
  exactly these sites — but its population is NARROWER in two directions that
  each lose a refusal. It suppresses on `RolledBackBodyDepth > 0`, which drops
  the `if`-arm rebind the latch refuses today; and `UninstallFnSigs` notes only
  when a removal COMMITS, so a sig-undef whose every match is locked would stop
  notifying. The two answer different questions — "what must the VM replay"
  versus "what might have gone stale in the bytecode" — and the second is
  deliberately the wider set.

The funnel makes no scope decision: every call is unconditional and
`rebindReachesModuleScope` decides. Keeping it dumb is what lets a new binding
operation join without re-deriving that reasoning.

**The read note** moved off the three resolution branches onto one classifier,
`Engine.noteBindingRead`. The branches had each carried their own copy of the
decision and the copies disagreed — the type arm noted unconditionally, the
value arm required `IsConcrete`, and the `/v` arm noted nothing — which is
precisely how `T/v` and `k/v` escaped. Now every path hands the classifier the
RESOLVED value and gets the same answer by construction.

**`core/go/rebind_funnel_test.go` is the part that closes the class.** It
drives every binding operation and every read path and fails, with a message
naming the funnel, if one stops going through it. Verified non-vacuous both
ways: removing the `InstallType` seat fails the operation row, removing the
`/v` routing fails both read rows. The frame-binding negative is there too —
it is the row that fails if anyone "simplifies" the funnel down onto
`DefTable.Push`.

Cost: the corpus refusal ceiling is unmoved at **0**, and every end-to-end pin
keeps its exact refusal text, so the firing set is unchanged.

### Two design claims this corrects

`design/RELOAD-INVALIDATION.0.md` is wrong in two places, both load-bearing,
and both in the same direction — they credit the whole-program hammer with
coverage it does not have:

- §2.1's audit table gives the whole-program `CALL_USER` unit's freshness as
  "`frozenReads` + `NotifyNameRebound` → whole-program refusal". For a CALL
  target that was false: the read never populated `frozenReads`, so nothing
  refused. True as of this increment, and worth re-reading as a claim that
  only became true by being checked.
- §3 F1 says "The whole-program hammer would have refused this program had the
  read been in an ordinary unit." The `helper`/`use` program above IS the read
  in an ordinary unit, and it was not refused.

### Corrections to this file's own earlier findings

Measured this session, with instrumentation since reverted:

- **Stage 4a-3 finding #2 undercounts.** "Two of them install a fresh
  `NewEmitState()`" — it is FOUR (`tryReturnedClosure`, `compileStoredFnUnit`,
  `compileStoredBody`, `compileStoredParamBody`); only `recordClosureDispatch`
  forks. And the five named boundaries are not all of them: `CheckState.IsolateEmit`
  installs a fresh state and fires DURING the compile pass on the state later
  Finalized, and `StampDetachedSig` arms another through
  `fork.Check = &core.CheckState{…}` + `BeginCompilePass`, which escapes a
  `.Emit =` grep entirely.
- **The anchor question finding #2 leaves open has an answer: the
  `*fnUnitRec` POINTER, never the int index.** `fnRecs` is `[]*fnUnitRec` and
  the fork copies POINTERS, so a shared-prefix unit is the same object in both
  states while a fresh probe's units are pointers no other state holds — the
  collision finding #2 predicts (measured: a probe wrote its frozen read at
  open-unit index 0 while the real state already owned an unrelated index 0)
  cannot happen to a pointer. It survives `Rollback` (fnRecs is append-only;
  Rollback bails when the count changed) and resolves to a Program unit
  (Finalize walks fnRecs one-for-one into `Program.Fns`). Pointer identity
  removes ALIASING but not the DISCARD: a probe's entries are still dropped,
  and a stored-probe case with a nested `each` unit was measured accumulating
  and discarding one. Today's name-keyed map is insensitive to that because
  the real pass re-runs the same body and re-notes; a per-site table would not
  be, and would need an explicit merge-back keyed on
  `check.FnAnalysisKey` (which contains no EmitState-derived data).
- **Finding #5's "corpus population is ZERO" is right about the LATCH and
  wrong as a statement about the table.** 68 of 2447 def-bearing corpus rows
  populate `frozenReads`; none trips the latch, because none is followed by a
  module-scope rebind. A per-site redesign has 68 live rows to keep working.
- **Finding #6's "one line below" is five.** `NoteDefRead` and the frozen-read
  gate are five lines apart in `stepWord`, and the file has since moved again.
  The substantive half holds: the read's true position is `stepWord`'s `val`,
  and it is the ONLY place it exists — measured, the compiler side cannot
  recover it, because at the bake `v.Pos()` is the DEF-SITE literal (or 0:0),
  not the read.
- **Finding #7's two error directions both land somewhere other than where it
  implies.** The over-refusal is NOT flex/Store — those read as CARRIERS, so
  `IsConcrete` is false, the note never fires, and they already route live to
  `OpLookupDynScope`. It is an INLINE top-level body: `def k 5  do [k add 2]
  def k 9  do [k add 2]` opens a fnRec, so the "no-op at top level" exemption
  misses it, yet the emitter records a SEPARATE closure per site (`f0` bakes
  5, `f1` bakes 9) and the program is correct with the latch suppressed. Cost
  today is performance, not correctness. And for every read that DOES reach
  the predicate the two conjuncts are locally faithful — 17 concrete
  module-scope reads, all baking; 11 non-concrete, all live; zero
  counterexamples either way. The proxy's fault is not in the expression. It
  is in what the expression is never asked about, which is what this increment
  found.


## Stage 4b: analysis order is program order — for units (2026-09-04)

The handoff's revised order (Stage 4a-2) named the next increment as "give
`frozenReads` its site structure, convert the latch into an obligation
discharged at `Finalize`, then `OpCollect`". Measuring the tree before
writing any of it changed the plan, the same way Stage 4a-3's measurement
did: the frozen-read class is not a routing problem and never was. **It is
the unit MEMO.** `FnAnalysisKey` keys a fn/closure unit on scope, name,
argument types, captures and body position, and omits the BINDINGS of the
enclosing-scope names the body reads — so a unit analysed at one program
point is reused at every later call site whatever the bindings are there.
Every frozen-read miscompile, and every refusal the discipline made to
prevent one, is that omission seen from a different side.

### Twelve more silent miscompiles, two mechanisms

Sweeping the rebind-site axis with reads placed BEFORE the rebind — a
placement no earlier sweep had varied — found the class was wider than the
memo. Default lane, exit 0, wrong answer, all on the tree at `9bf9662`:

	def k 5  do [ k  def k 9  k ]                                          5 9      -> 9 9
	def m {a:1}  do [ m  def m {a:2}  m ]                                  {a:1} {a:2} -> {a:2} {a:2}
	def k 5  do [ def t k  def k 9  t ]                                    5        -> 9
	def k 5  [1 2] each [ k  def k 9 ]                                     [5 9]    -> [9 9]
	def k 5  [1 2] each [ k  def k (k add 1) ]                             [5 6]    -> [6 7]
	def k 5  [1 2] fold [ k  add  def k 9 ] 0                              7 0      -> 11 0
	def k 5  [1 2] scan [ k  add  def k 9 ] 0                              [1 7] 0  -> [1 11] 0
	def k 5  for 2 [ k  def k 9 ]                                          5 9      -> 9 9
	def k 5  def f fn [[] [Integer] [k add 2]]  do [ f  def k 9  f ]      7 11     -> 11 11
	def k 5  def f fn [[] [Integer] [k add 2]]  [1 2] each [ f  def k 9 ] [7 11]   -> [11 11]
	def k 5  def f fn [[] [Integer] [k add 2]]  def g fn [[] [Integer] [do [ k  def k 9  k ] add]]  g   14 -> 18
	def k 5  def f fn [[] [Integer] [k add 2]]  def g fn [[] [Integer] [def k 9  f]]  g  f            11 7 -> 11 11

Two mechanisms, and neither is the latch's:

- **The leaked-state re-run.** A leaking body — a `do` body, or an
  each/fold/scan body the runtime re-runs per element — is analysed once
  with recording SUSPENDED (`KeepDefsBodyGuard` / `MultiRunBodyGuard`) and
  then RE-RUN to compile (`tryRecordClosure` → `recordClosureDispatch`'s
  probe and real compiles). The re-run began from the state the first run
  LEAKED, so every read before the body's own rebind baked the rebound
  value. `do [ k  def k 9  k ]` compiled to `PUSH_CONST 9; PUSH_CONST 9`,
  and the fn-calling variants baked 9 into `f`'s unit because `f` was first
  analysed during the re-run, after the leak. This is at every scope — the
  `do` inside `g` shows it in a fn frame — and it is what NUR117's arm
  could not see: that arm refused a rebind AFTER a unit had recorded, and
  here the unit records after the rebind.
- **The caller-frame shadow.** `g`'s frame-local `def k 9` shadows the
  module `k`; `f` called from `g` reads 9 by dynamic scope, and the unit
  the memo kept for `f` — baked under `g`'s frame — served the top-level
  call too. The freeze discipline's `ModuleScopeBinding` gate never
  looked: from `f`'s baseline the caller's frame-local IS an enclosing
  binding.

### What landed (`compiler/go/unit_memo.go`)

**A. The memo is binding-sensitive.** `NoteFrozenRead` now carries the
binding's `DefTable.Gen` at the read (core passes it from the registry the
read resolved in; the type arm registers its read's value ID too, as the
value arm always did) and records it on the OPEN unit — `fnUnitRec.bakes`,
alongside `frozen` (the kind, for the refusal's noun). `StartFnCompile`
treats a FINISHED memo hit as stale when any bake — the unit's own, or one
reachable through the units it calls, polys or pushes as closures
(`forEachUnitRef`, the one reachability walk) — has a generation that has
moved, and compiles a fresh unit for that site; the stale unit keeps
serving the sites that already reference it. An unfinished unit is never
stale (in-flight recursion must reuse it). The program-wide `frozenReads`
map is gone.

**B. The body re-run environment.** Both guards clone `r.Defs` when a body
run opens and publish the clone at close, keyed by body ID
(`KeepDefsBodyGuard` grew a `bodyID` parameter for it; a later fixed-point
round keeps the FIRST round's start). `tryRecordClosure` claims it and
`recordClosureDispatch` swaps it in around every compile — probe, real,
each extra hook — through the new `core.Registry.SwapDefs`, restoring the
leaked table after. A `do` body re-runs from its start (it runs once, so
the re-run IS the run). A multi-run body re-runs from its start with every
name it rebound replaced by `JoinCarriers` of the start and end carriers:
iteration-varying, hence non-concrete, hence a live read with a runtime
re-match downstream — which is exactly the lookup half §6.9 asks for, in
the one place the memo cannot supply it. Types the body installed are
carried over from the leaked table rather than restored to absent: a
minted node's lattice part survives either way, and a re-run that
re-defines the name over a surviving part without its binding trips the
parts conflict `RunCarrierBodyKeepDefs` already records.

**C. The latch narrows to escaping units.** `NotifyNameRebound`'s
frozen-read refusal now fires only for a bake held by — or reachable from —
a unit whose reference ESCAPES into a value (a returned closure, a stamped
fn value, a fn-value closure body): its later "call" is an apply of the
value, invisible to the memo. Same text. Everything else is the memo's.
NUR117's counters and `rebindReachesModuleScope` stay, serving that arm.

**D. The residual-order hazard is refused, not fixed.** A RE-PUSHABLE
residual read — a live `OpLookupDynScope`, or a loop-carried slot — is
re-pushed at the END of its fragment, after a bind of the same name the
fragment recorded later than the read. Pre-existing and independent of
the memo: `def k (1 add 4)  def f fn [[] [Integer Integer] [k  def k 9
k]]  f` answered `9 9` for `5 9`, and the `for` row above is the
loop-carried form. `residualReadHazard` refuses it by name at the unit
finish, a branch arm and a loop body — through `residualStands`, the ONE
`MarkUncompilable` site those four settlements now share with their
"result of unknown provenance" arm: the refusal-site census
(`refusalSiteCeiling`) caught the first cut's four new sites at once
(100 against 96), and the fold took the count to 93, where the ceiling
now sits. The hazard is keyed by FRAGMENT
IDENTITY (`EmitFragment.id`, monotone from `beginFragment`): a read in a
nested fragment reaches the enclosing residual only through the nested
construct's result, and two sibling arms can share a start seq, so neither
depth nor seq is an identity. The first cut keyed on name and seq and
refused every `def acc 0  for n [def acc (acc add 1)]  acc` in the tree —
the post-loop read is AFTER the store, but the body's read was before it.
The precise fix — a per-read snapshot event — needs per-read identity the
recorder does not have (every read of a binding carries the binding's own
value ID); recorded as the follow-on.

**E. A probe/real asymmetry the hazard made visible.** `forkForProbe`
gives the probe fresh emission tables — no `producedBy` — so an enclosing
binding read whose value an EVENT produced (`k` after a leaking `do`
rebound it: `def k 5  do [ k  def k 9  k ]  do [ k  def k 12  k ]`) bakes
as a const in the probe and routes LIVE (`OpLookupDynScope`) in the real
compile. Before this increment the two verdicts agreed by accident (a live
read never refused); the hazard refuses the live read, so the real compile
can now decline after a clean probe. `recordClosureDispatch`'s post-real
guard is therefore a reachable arm, not the defensive one its
`//covergate:allow` claimed — the pragma is gone and the row pins it. The
principled fix is a probe that carries the routing tables the real compile
consults (`producedBy`, `eventInfo`), so the two compile the SAME unit; it
was not attempted here because it changes every closure probe's verdict
across the corpus and wants its own measurement.

### Measured

Over the 70-program sweep (every shape above plus the Stage 4a-4 matrix):
**0 divergences**, and 18 refusals, every one sound — 6 under the residual
hazard, 4 multi-run bodies whose closure declines because the joined read
is in the residual (`code-body word each (Stage 2)`), and 8 that refused
the same way before. Every row of `TestModuleReadRebindRefusesAndMatches`
(27, all refusals) now splits three ways: value / type / call-target /
transitive / 0-output / `/v` / do-body rebinds COMPILE with parity
(`TestModuleReadRebindCompilesWithParity`); the `undef` rows refuse on
check diagnostics (the re-recorded unit reads a name that is gone — the
interpreter's undefined_word, one pass earlier); the multi-run rows refuse
at the arm-residency read gate. The cross-family row (`def k "x"`) compiles
and raises f's return type_error — and exposed NUR118, a PRE-EXISTING
blame-position divergence of every compiled return-contract error
(measured on the tree before this stage: `1:43` interpreted, `1:30`
compiled for `def f fn [[m:Map] [Integer] [m get "a"]]  f {a:"s"}`).

Corpus refusal ceiling: unmoved at 0. `cover-gate-core` 100%. The
`for`-loop body word `for` is untouched: `AnalyseLoopBody` already joins
between rounds, and its recorded round reads the joined carrier through
the loop-carried slot — the `for` row above was the residual-order
hazard, not the leak.

### What this corrects in the plan

- **Stage 4a-2's revised order is superseded.** "Convert the latch into a
  Finalize obligation, then OpCollect" was written on the premise that the
  latch guards a lowering decision. It guards a MEMO decision, which is
  made at `StartFnCompile`, and that is where it is now made correctly.
  `OpCollect`'s acceptance pair (`w k 1` → 5, then the raise) answers both
  spellings today: the second `go` re-records against `k`'s fn binding and
  the unit compiles the interpreter's strict-barrier raise.
- **Stage 4a-3 finding #7** ("the next increment's real subject is the
  proxy") was right about the proxy being the wrong input and wrong about
  where the fix goes: the `IsConcrete` proxy still feeds `frozen` — for the
  escaping latch, where over-noting only over-refuses — and the memo asks a
  different question (has the binding moved), for which the proxy's
  imprecision costs at most a spare unit.
- **§6.5's payoff paragraph and "The payoff gates, measured".** The
  frozen-read gate leaves the `OpDispatchGeneric` list: it is re-filed
  under the memo, closed for every unit the memo can re-record, and kept
  only for escaping units. The stored-handler, family-L and NUR037 gates
  are untouched by this stage and stay filed where that section put them.
- **`design/RELOAD-INVALIDATION.0.md` §2.1's freshness cell** for the
  whole-program `CALL_USER` unit is corrected in place a second time: the
  answer is per-site re-recording, not whole-program refusal.

### What the next author should not re-derive

- The memo's staleness key is `DefTable.Gen`, per name, read from the
  registry the unit's body resolves in (`fnUnitRec.reg`). It moves on
  every push / pop / replace / truncate / set of that name — including a
  frame-local push in a caller — which is exactly why the caller-frame
  shadow is caught for free and why a fn with a body-local literal `def`
  is NOT recompiled per call: its own def is fn-scoped, and fn-scoped reads
  are never bakes.
- `forEachUnitRef` is the one reachability primitive. A new way for a unit
  to reference another unit must join it or both the staleness walk and the
  escaping latch go blind — `TestUnitStaleWalksEveryReference` enumerates
  the edges it knows.
- The environment must be entered afresh per compile from its own clone;
  the probe mutates the table it runs on.
- `takeBodyEnv` claims the start exactly once; a guard close that finds the
  recorder suspended publishes nothing (its body is re-run, guard and all,
  during the outer compile), which is what bounds the live clones to one
  per open nesting level.
- No end-to-end row pins the escaping latch's text: every returned-closure
  shape that would reach it at top level refuses earlier on its residual
  shape. The compiler unit tests pin it; the first such shape to compile
  owes `lang/go/frozen_module_read_test.go` its row.

## Module-family values read live (2026-09-05)

The frontier ledger's largest contained family after Stage 4b, measured
before choosing it: of 102 ledgered refusals, 34 are "unknown provenance",
and 12 of those are one mechanism — NUR031's Module-descriptor and
namespace identity rows (`M.$module eq M.$module`, `IO deq IO`, the
per-import-instance and shared-descriptor rows), every one green on the
interpreter and refused `operand of unknown provenance or not statically
materialisable`. A namespace RESIDUAL (`import "boru:io" IO`) refused the
sibling `residual value not statically materialisable`. Inside a fn unit
the namespace read already compiled: the enclosing-binding arm of
`dynScopeRescue` routes it to `OpLookupDynScope`. Top level had no arm.

Two facts decided the shape of the fix:

- **The const gate refuses these values ON PURPOSE.** A namespace is a
  pointer-shared map of fn exports; a descriptor is an extension payload,
  and `ConstBakeable`'s contract names module instances among the types
  that must not implement it (`core/go/typebehavior.go`). Baking was
  therefore never the answer; the live read is — the same read the unit
  path already makes, and the one that honours a re-import after `undef`
  (`edge-modules-1.tsv`'s GOTCHA row: identity is per-import-instance).
- **A `$module` read was ELIDED as a compile-time name resolution**
  (`recordCallElided`'s ExtensionPayload arm, meant for the export-fn and
  namespace resolutions the real dispatch records elsewhere), so its result
  — the shared descriptor, `ns.Module`, a stored Value that is NEVER minted
  — reached `eq` with no event and no ID. `setProducedAt` refuses an empty
  ID by design (a `""` key would alias every identity-less value onto one
  producer), so even an un-elided dispatch registered nothing.

What landed, five small parts in the seams that already existed:

1. `tagCheckModeDefRead` (check) tags a module-scope MODULE-FAMILY read
   with `DynFrom`, exactly as it tags a module-scope flex — the name channel
   that does not depend on the value having an ID.
2. `dynScopeRescue`'s top-level arm admits `IsModuleFamilyValue(v)` beside
   the S5 loop bind; `resolveResidualOperands` tries the same rescue for a
   module-family residual, gated by `moduleResidualStable` — the binding
   must still hold the very instance (namespace facet pointer, or boxed
   `*ModuleDesc` pointer), because the residual re-push runs at the END of
   the program (`def x IO  x  def x 5` refuses; `def x IO  x  def x IO`
   compiles).
3. `recordCallElided` no longer elides a `dot`/`get` whose result is a
   Module instance; `RecordCall` mints the event's identity for a
   module-family out with no ID (targeted — the general empty-ID case keeps
   `setProducedAt`'s refusal and its reasons).
4. `tryFoldModuleConst` declines when a module-family operand is
   event-produced: `typeof MathUtil.$module` and `MathUtil.$module.name`
   used to fold over the elided descriptor; folding over an event would
   orphan it, so they record a real dispatch now. Same answers; two
   instructions instead of a const.
5. `lowerDynBind` emits no bind op for a ROOT def of a module-family value
   with no producing event (`def m (module […])`): the check pass installed
   the binding and it survives to run time, kept or replayed by its twin
   (a concrete captured entry), so the promised live read resolves it. A
   frame-local def of one keeps the refusal.

Measured: all 12 rows graduate (the ledger's `stale entry` rule fired for
each; the rows moved into `compare-restrict.tsv`'s opaque-Ideal section
and `edge-modules-1.tsv` §6, and both frontier files retired with them),
whole-corpus differential clean, refusal-site census unchanged at 93, and
`lang/go/bytecode_loop_provenance_test.go`'s two shapes — a Module
instance as a for-body result, which had been the minimal in-repo
UNSEATABLE value — compile with parity and are re-pinned as such. The
frontier ledger stands at 90 rows. The end-to-end pins are
`lang/go/module_value_read_test.go`; `TestModuleResidualStable` drives the
stability gate's nameless and nil-registry arms.

What this does not do: a frame-local `def m (module […])` inside a fn body
still refuses (`fn f: body result of unknown provenance`) — the binding is
popped with the frame, and giving it an `OpBindDynScope` needs an operand
for a value no event produced. And the general question the elision hid —
an extension result with no ID reaching a consumer — is answered here only
for the module family; `setProducedAt`'s comment says why the general mint
is not free.

## A returned closure is parked (2026-09-05)

Found while sizing family C (the NUR038-seal twin-result rows, "dynamic
value precedes residual args"). Measuring what the interpreter does with a
callable RESULT followed by a later value — the question family C turns on
— exposed a silent default-lane miscompile older than every stage on this
branch: `def mk fn [[] [Function] [([y:Integer] => [y add 1])]]  mk 7`
compiled to `8` against the interpreter's `fn (Integer) 7`. A user fn's
returned closure is PARKED where it lands; only a paren rewind over two or
more survivors, or a read that dispatches, applies it
(design/PAREN-RESTEP-RULE.0.md, whose new §2.1 carries the measured
table). `resolveDynamicApply`'s fn-carrier and dynamic-lead arms applied
every lead a paren had NOT placed — the `leadPlacedNotRead` conjunction
answered "placed by a paren?" and nothing asked "produced by a call?".

What landed (`compiler/go/emit.go`):

- `callResultPlaced` / `callResultPlacedIn`: a single-output user call's
  result, or a user member's arrival-apply result (the dyn-method event
  `tryMemberFnArrivalDispatch` records — `userMemberFn` checks the word
  names a boru-bodied fn), that no enclosing paren re-stepped and no read
  delivered, is placed data. Wired into the dynamic-lead and fn-carrier
  arms, the precedes-args loop and the unconsumed-carrier loop of
  `resolveDynamicApply`, and into `noteDynFrameReplay`, where a parked
  result is not a possible unapplied call — so `def g fn [[] [Any] [mk
  7]]  g` compiles to the RET count check and raises the interpreter's
  `type_error … got 2 — [fn (Integer) 7]` instead of replaying the frame
  to `8`.
- Three things deliberately do NOT park, each measured: a NATIVE word's
  returned fn (`do [mk] 7` is `8` — its delivery re-steps it), a
  MULTI-output user call's leading fn (`def g fn [[] [Any Any] [mk 7]] g`
  is `8` — its own frame rewound before returning; the shape stays a
  refusal because the check model does not perform that rewind), and a
  def-read or member-read lead (dispatches).
- Family C's twin-result residual (`m.p 5 m.p 7`) is two placed values
  now: six of the nine frontier rows graduate to `fn-value.tsv` §6. The
  three that stay carry the dynamic result UNDER a later window (the stack
  form, a computed first argument, a lambda member). Stage 4b's splice
  fallback row (`… do [n drop word op] …`, two Any-typed call results)
  was the same shape and graduates with them — `3 40` on both lanes.

The probe's unit-side lookup searches the unit's captured fragment first
(`callResultPlacedIn(v, rec.frag)`): `TakeFragment` has moved the unit's
events off the frame stack by the time its residual is settled, which is
why the first cut found nothing inside units.

The park's first cut widened one hazard the old refusal had been holding:
a user fn that returns a fn it was HANDED (`def app fn
[[g:Function][Function][g/v]]  app (z:Integer => [mul 3 z])`) renders
that value under the PARAM's name on the interpreter (`fn g(Integer)`) and
under its own on the compiled lane (`fn (Integer)`) — same value, one
render. `callResultRenderKnown` restores the residual's render gate for
every parked call result except one whose callee returns a compiled
anonymous closure carrying its render string (the `mk 7` shape). The
paren-placed spelling of that render difference pre-dates this branch,
and the `Any`-typed twin slips the gate; both are NUR119.

Not done here: the `[Any Any]` rewind-after-count-check shape, and the
0-arg-overload twin (`m.e 5 m.e 7`, NUR035's guard) — both refuse soundly.

## Where the frontier stands after these four increments (2026-09-05)

The frontier ledger (`test/go/langspec/frontier_spec_test.go`) went 102 →
84 across the third and fourth increments. What is left, by family and by
MEASURED blocker — each row below was run on both lanes on this tree, and
the shapes that already compile are named so the next author starts from
the boundary, not from the family's label:

- **22 × "body result of unknown provenance" — the closure-capture family**
  (Church encodings, combinators, parser combinators). Returned closures
  that CAPTURE the factory's params already compile when the inner body is
  the Stage-G paren-apply shape over typed params: `def app fn
  [[g:Function][Function][( fn [[x:Integer][Integer][(g x)]] )]]  def h
  (app (z:Integer => [mul 3 z]))  (h 5)` is `15` on both lanes
  (`PUSH_CLOSURE` + `CALL_DYN_TRAIL_TOP`). Three separate blockers sit
  beside it, each measured: (a) a BARE-NAME apply of a captured Function
  with a forward arg (`[g x]`, no paren) refuses; (b) a `/v` READ of a
  captured fn returned as data (`[g/v]`) refuses; (c) an `Any`-typed inner
  param (`[[x:Any][Any][(g x)]]`) refuses. *(Corrected 2026-09-05, the
  attempted twelfth increment, by surfacing the closure PROBE's own
  reason: (a) and (c) are ONE refusal — "closure fnval$body: body value
  count differs from declared returns", the count check that fires before
  the replay classifier a fn unit reaches — and (c)'s named gate,
  "cannot prove its argument non-function", does not appear at all. The
  outer "fn app: body result of unknown provenance" every one reports is
  just the factory seeing an unresolvable closure operand.)* The Church rows use all
  three at once (`x:Any => [x/v]`, `f/v apply` chains), so they graduate
  only when all three land; the parser-combinator rows are (a) plus the
  `if` arms. Sizing: three increments, (c) first — it is the gate the
  fourth increment's probe already reasons about (a parked result is not a
  fn that dispatches).
- **8 × "def-bound computed fn apply"** (`def h (FnUtil.compose …)  (h 5)`,
  the FnUtil combinators). A def-read of a computed Go-impl closure is a
  WORD dispatch in the interpreter (MatchSignature over the value's own
  signatures); `OpCallDynamic`'s island runs value semantics, which leave
  a named Go-impl fn as data. `OpCallDynApplyTop` — `apply`-word
  semantics over a paren-bounded window — is the candidate lowering for
  the PAREN spelling (`(h 5)`: the window is the whole arg set, so arity
  need not be known); the bare spelling (`h 5`) needs the arity and stays.
  Note the same shape over a COMPILED factory's closure already compiles
  (`def h (mk 1)  (h 2)` is 3) because `producerReturnedClosureArity`
  knows the arity. *(The eleventh increment took that one look and the
  guess here was wrong: `def keep fn [[g:Function][Function][( fn
  [[x:Integer][Integer][x add 1]] )]]  def h (keep …)  (h 5)` refused not
  for its unused capture but because the returned lambda captures
  NOTHING, so the fold bakes it as a const rather than a closure. The
  recovery now reads the arity off the const, and every shape in this
  note's family compiles; what remains under this reason is the Go-impl
  FnUtil family, whose word dispatch OpCallDynamic's value semantics
  cannot reproduce.)*
- **8 × "check diagnostics"** — not one mechanism (L-DO variadic regions,
  the staged Church spellings, the `/v` hold on a def-bound computed fn,
  the strict-lane FnUtil rows). Leave until the families above move.
- **5 × "computed closure at a word's argument slot"** and **3 × "fn-value
  -call boundary"** (family C's leftovers: the stack form `5 m.p m.p 7`,
  a computed first argument, a lambda member `m.l 5 m.l 7`). All three
  leftovers are the ARRIVAL model declining to claim the apply
  (`tryMemberFnArrivalDispatch`: no forward literal window, an anonymous
  member), after which the member read stays a dynamic value under a
  later window. The lambda-member row is the smallest: admit an anonymous
  single-sig member with a boru body.
- **The `[Any Any]` rewind-after-count-check shape** (`def g fn [[] [Any
  Any] [mk 7]]  g` is `8` interpreted): the check model does not perform
  the frame rewind that a passing return-count check enables, so the
  unit's out count is wrong before any lowering could be. A check-model
  change, not a compiler one; refuses soundly today.
- **The probe/real asymmetry** (Stage 4b, E): a closure probe that carries
  `producedBy`/`eventInfo` so the probe and the real compile judge the
  same unit. Changes every closure probe's verdict; wants the whole-corpus
  differential as its gate.

A test-suite note, recorded because it cost a belt run: `test/go/langspec`
died once today with `fatal error: concurrent map read and map write` in
`DefTable.TopEntry` under `TestDiagnosticSurfaceParity`'s check pass (a
`tand` unify resolving a predicate ref), with NO goroutine in the dump still
writing a def table. The sweep runs two workers, each with its own
`lang.New()` per row, so the writer was a goroutine that had already
finished — an async body a corpus row spawns (`module-time.tsv`'s
`TimeUtil.timeout` / `interval` rows, `module-io`, `module-net`) running
inside the same row's registry while its check pass was still reading.
Pre-existing and rare: the same package passed four belts and three
coverage runs the same day on trees that differ only by a residual-gate
predicate. If it recurs, the fix is in the check pass of the timer words
(run the body synchronously under check, or fence the registry), not in
the compiler.

IT RECURRED, at a second site, 2026-09-10 (the increment 42-45 batch gate):
`lang/go/modules` died with the same `fatal error: concurrent map read and
map write`, this time in `DefTable.Depth` under
`modules.serveRawHandler`'s connection goroutine — `InvokeCallbackFn` →
`CallBoru` on the registry the test's main goroutine is still using. Same
class, same verdict, a different async owner: a raw-socket connection
handler rather than a timer body. Re-ran three times immediately after,
all green, on the identical tree. The generalised statement is worth
having: **any async body boru spawns runs in the registry that spawned it,
and nothing fences it against a concurrent reader.** The two witnesses
(timer words, serve-raw) are the two places the suite spawns one. The fix
is a registry fence at the spawn seam, not per-word — and it is
`InvokeCallback`'s, not the compiler's.

Two rules this line paid for today, worth keeping in front of every
increment: measure the INTERPRETER first, with the shape's siblings (the
paren, the body frame, the fn body, the multi-output twin), because the
compiled lane's existing model may be wrong in the same place the new
family is (the returned-closure park hid inside family C); and check the
lane answers on the DEFAULT lane, since `-force-compile` refuses where the
default lane silently compiles.

## Two silent miscompiles found measuring the closure-capture family (2026-09-05)

The fifth increment set out to land the closure-capture family's blocker
(c) and landed none of it: measuring the gate turned up two DEFAULT-lane
miscompiles older than this branch, each with several witnesses, and the
rule is that a wrong answer with exit 0 outranks a frontier row. Both are
fixed and pinned; (c) is re-sized below, and (a) has a plan and a measured
obstacle.

**NUR120 (resolved, number retired) — a lambda's return count.** A `=>` /
`afn` lambda carries a `Returns=[Any]` PLACEHOLDER that the analyser infers
past for the result TYPE; the interpreter enforces its COUNT at every
dispatch boundary — the named call, the paren apply, the `apply` word, a
member read, a Function param, and the callback seam. The compiled lane
dropped it three ways: `BuildFnBodyReturnsFn` nilled the declared returns
for the unit's RET as well as for the analyser; `fnValueRetSpec` declined
to carry a contract on the closure value; and under a `BodyResultTop`
word the closure body was TRIMMED to its top value at compile time
(`trimToTopResult`), which hid the count from `checkClosureReturn` for
NAMED fns too — and, for a 2-return declaration, raised where the
interpreter passes. Measured, all exit 0: `def f ([x:Integer] => [x 1])
f 5` answered `5 1`; `5 (…) apply` likewise; `each ([x:Integer] => [x 1])
[1 2]` answered `[1 1]`; `def f fn [[n:Integer][Integer][n 1]]  each f/v
[1 2]` answered `[1 1]`; the verbose `fn [[x][Any][x 1]]` twin was
enforced on both lanes all along. Fix: `check.LambdaCountContract` (n
`Any` slots — a count, never a type) is the compiled unit's RET contract
and the closure value's `RetTypes` for an anonymous lambda; the trim is
retired (the out-of-order residual it once sidestepped is
`residualForceOrderFor`'s promotion now); the VM's `checkClosureReturn`
enforces the count over the WHOLE residual with the unit's own unnamed
allowance, exactly as `__RC` does — the frameless `checkReturnContract`
never raised on an over-count. Pinned on both lanes by
`lang/go/lambda_return_count_test.go` (every path, plus the 1-value
bodies and the allowances). What remains is pre-existing: the NUR118
position, and on two value-applied paths a callee name and a missing
position (the NUR118 addendum).

The count contract then met the interpreter's OTHER seam, and the corpus
said so (`walk {…} {…} (m:Any => [m.path print])` raised `expected 1
return value(s), got 0` compiled and ran clean interpreted). A handler
that hands a FnDefInfo to `InvokeCallbackFn` runs it through CallBoru,
whose return discipline is `enforceCallBoruReturns` (NUR069): the
declared TYPES are checked head-to-head over the aligned residual (the
surplus sits at the bottom) and the COUNT is never raised — `walk`'s
hooks, `each`/`fold` over a MAP, `filter`'s Function form. The TOKEN seam
(`InvokeBody`: `each` over a list, `apply`, a paren call) steps the fn
value and `__RC` enforces the count. The compiled lane had ONE discipline
for both — and it was wrong for NAMED fns on the fn-value seam before
this increment (`walk … cb/v` over a 0-value body raised compiled; an
`[Integer Integer]` declaration over one value raised compiled). So the
seam is now explicit: `core.InvokeCallbackBody` is the compiled-closure
twin of `InvokeCallbackFn` (it stamps `ClosurePayload.RetTrim` on the
VALUE at the seam, never on the stored closure — the same closure can
cross both), the three handler branches that route a compiled closure
where their FnDefInfo path routes to `InvokeCallbackFn` call it
(`invokeBodyTop`, `runFilterCallback`, `callWalkHook`), and the VM applies
`checkCallBoruContract` — the mirror of `enforceCallBoruReturns`, predicate
skip included — from `checkClosureReturn` under `RetTrim` and from the root
RET of a unit entered through `RunUnit` / `runUnitNested` /
`runForeignUnit` (`enterCallbackUnit`, `vmContext.rootRetTrim`; a closure
the body invokes through the token seam clears it). Pinned by
`TestLambdaReturnCountOnTheFnValueSeam`.

**NUR121 (resolved, number retired) — the collection hazard.** A
fn-typed CARRIER the check model leaves unapplied — a `g:Function` param
or capture read as a word, an `args.0` read — lets a LATER dispatch in the
same scope stack-collect the argument the interpreter's `g` would have
taken first, and every lowering that applies a lead to the values after
it then applies `g` to that dispatch's RESULT: `def f fn [[g:Function
x:Integer][Integer][g x add 1]]  f (z:Integer => [mul 3 z]) 5` is 16
interpreted and was 18 compiled. Five witnesses on the current tree, all
default lane, exit 0: the bare body `[g x add 1]` (the whole-frame
replay), the paren lead window `(g x add 1)`, the unnamed-param replay
`args.0 args.1 add 1` bare and in a paren, and the same window inside a
returned closure. Nothing in the recorded trace tells these apart from
`(g (add x 1))`, where the nested paren seals `g` off and 18 is right on
both lanes: the operands and events are identical, only the paren SCOPE
differs — and the scope is the engine's. So the engine notes it where a
stack collection and its scope are both in hand
(`Engine.noteCollectionHazards`, at execMatch's check-mode splice and
the full-stack fold): every unapplied fn-typed value below the lowest
stack-collected index in the same paren scope is marked
(`EmitRecorder.NoteCollectionHazard`), never reaching into a frame's
resolved-argument prefix (`FrameOpenInfo.ArgSpan`, the run's `StartAt`
prefix — arguments are inert). The three lowerings decline a marked
EAGER lead (`EmitState.hazardLead`): `RecordDynApply` (the paren lead
window records through it), `noteDynFrameReplay`, and
`resolveDynamicApply`'s two lead arms. A PLACED lead is lazy and keeps
its lowering — a paren-placed single survivor re-steps only at the
enclosing close over what survives there, and a user call's returned
closure is parked (design/PAREN-RESTEP-RULE.0.md) — so `((mk) 7 add 1)`
is still 9 and `((args.0) args.1 add 1)` still 18 on both lanes; the
first cut refused those and the parity pins caught it. A marked eager
lead left ANYWHERE in a residual refuses too (`residualStands`, the
program residual in Finalize): `[g x drop]` and `do [(f 5) 2] drop`
answered `fn (Integer)` where the interpreter answers the count error
and nothing — the lead had no args after it for the arms to see.

The corpus then named the class's other face: a REWINDING code-body frame.
`do [mk 7] add 1` — a returned closure parked inside a `do` — is 71
interpreted (the do's close re-steps the parked value over 7, then add)
and was 80 compiled (the model's `add` took the 7, then the outer residual
applied the closure to 8); `do [(f 5) 2] add 1` likewise 21 against 30.
The do's OUT is a native word's result, eager by the park rule, and the
hazard mark catches it. Four `bytecode-migrated.tsv` rows of the shape
`do [(f 5) 2] error [dot code]` refused under the first cut, and the
mark-window pins (`TestMarkWindowDoCatchCompiles`) said why that was
wrong: the L-DO do-catch lowering re-steps the whole region at the
program's end, and `error`'s catch clause is a STRIP-INPUT hop that
passes a non-error region through untouched — the one later collection
that is transparent by construction, which is what the mark window's
claim ("every residual entry re-marked through strip-input hops") encodes.
So the scan marks nothing for a `StripsUnconsumedInput` word, the four
rows compile through the window as before, and only a collection that
CONSUMES (`add 1`, `drop`) marks the lead. A `/v`-read value with a
PENDING `apply` is excluded on the same principle (the apply word owns it
at its own position), which keeps the mid-body-apply pins on their own
diagnosis. Compiling the consuming shapes means the do body applying its
placed lead at its OWN close (the frame-rewind model — Stage 5's regions),
not widening the coarse rule.
The rule is coarse in one measured direction: `do [mk 7 8] add 1` is
`70 9` on both lanes (a 1-arg lead never reaches the 8 that `add`
collected) and refuses anyway, because a `do` out carries no arity; an
arity-aware mark (`producerReturnedClosureArity` at the consumer, the
scan recording the gap) is the refinement, when a row pays for it. Pinned
by `core/go/engine_collection_hazard_test.go` (the scan's scope),
`compiler/go/collection_hazard_test.go` (each consumer), and
`lang/go/collection_hazard_test.go` (ten witnesses as sound fallbacks,
twelve admitted twins as parity).

**NUR122 (pending) — the nameless compiled apply.** Measuring the
fallbacks also measured the error lane of the fn-value apply that
ALREADY compiles: `def f fn [[g:Function x:Integer][Integer][g x]]  f
(z:String => [z]) 5` raises `signature_error: cannot call `g``
interpreted and `type_error: f: expected 1 return value(s), got 2 — [fn
(String) 5]` compiled (the whole-frame replay's island parks an
anonymous no-match as data); the paren spelling raises the no-match
with an EMPTY name at the body's position; a 0-arg `g` fires interpreted
and parks compiled. Every non-erroring program in the family agrees;
this is the error contract Stage 3's Apply kernel owes (§6.4), and
NUR119's re-label is its value half. Recorded, not fixed.

**Blocker (c), re-sized.** The gradual-argument gate
(`parenLeadFnApplyIdx`, TestS5BParenLeadFnApplyIdxGradualArgDeclines)
is not a compiler increment. `(g x)` with `x:Any` and a fn-valued x at
run time is a WORD dispatch of `g` under its binding name, three-way on
`g`'s own slot type: a `Function` slot resolves `x` to its value and
APPLIES (`hasPendingForwardExpectingFunction`); an `Any` slot steps `x`
and raises the strict-barrier text (`g is still waiting for 1
argument(s) when `x` begins its own dispatch`); any other slot rejects
`x`'s type before stepping it and raises the lead's own no-match. A
faithful lowering needs the binding NAMES at run time (NUR119), a
word-semantics island over the frame's bindings (the value island parks
where the word raises — NUR122), and, for a compiled-closure `g`, a
FnDefInfo to dispatch by word (a ClosurePayload has none). That is Stage
3's Apply error contract, not a gate to widen; the pin stands.

**Blocker (a), measured, not landed.** A lambda unit (a returned fn
VALUE's own named-param frame, `fnval`) count-refuses `[g x]` before
`noteDynFrameReplay` runs (the closure count check is user-fn-only). The
arm is one line — a lambdaUnit with `nUnnamed == 0` takes the user-fn
discipline, and `closureResidualHasUnappliedFn` is skipped when
`dynFrameW > 0` — and with it `(h 5)` answers 15, `[g x y]` -3,
`[def y (add x 1)  g y]` 18. Two things stopped it landing this
increment: the whole-frame replay's island PARKS a no-match where the
interpreter's word dispatch raises (NUR122 — the same divergence the
paren spelling carries, but the bare spelling would add the 0-arg-fires
case to it), and a returned closure's `ClosurePayload` carries no
`RetTypes`, so a 2-value body that the arm lets through returns two
values where the interpreter raises the count error — `tryReturnedClosure`
must attach `fnValueRetSpec(fd, lam, pos)` to its `opClosure` operand
first (`EmitOperand.closureRet` exists for exactly this). Land the
RetSpec, then the arm, then decide whether NUR122's bare-spelling
widening is acceptable under a record or needs the island to re-label
first.

## The bare fn-binding read is a word dispatch (NUR123), and two more findings (2026-09-05, the sixth increment)

Measuring blocker (a)'s parity rows on the tree after NUR120/NUR121 landed
turned up a sixth default-lane class, older than this branch and broader
than (a): **a bare read of a frame binding that holds a fn is a WORD
dispatch on the interpreter and was a slot push on the compiled lane.**
`stepWord` does not substitute a binding whose value is a FnDefInfo — it
"goes through normal Lookup", the registered-word path under the binding
name (`installDef` re-labels the value `Name = name` at bind time): a 0-arg
fn fires, an n-arg fn collects forward from the tokens after it and from
the frame's stack below it, a no-match raises `cannot call `g``; the one
exception is a pending forward whose next slot expects a Function, which
takes the value as data. The check model binds a CARRIER for the param
(the fn-carrier side table for `g:Function`, `Defs` for `x:Any`), nothing
can dispatch a carrier, so the read landed in the residual as a value and
lowered as `PUSH_LOCAL`. Exit 0 on the default lane: `def f fn
[[g:Function][Any][g]]  f ([] => [42])` answered `fn` for 42; `f
(z:Integer => [z])` answered `fn (Integer)` where the interpreter raises;
`def id fn [[x:Any][Any][x]]  id ([] => [42])` answered `fn`; `(g)`, `def
y 1  g`, a quoted arg, a returned closure and a named 0-arg lambda the
same (NUR123's table has thirteen rows). The count-mismatch replay
already existed for `[g x]`; what it lacked was the NAME — its island
re-stepped the VALUE (NUR122's park) — and the count-MATCHING residual
never armed it at all.

**What landed.** The classification is the engine's, made where the read
happens: `Engine.noteWordRead` (both bare-read paths — the `Defs` value
branch and the fn-carrier side-table branch) notes a fn-typed or gradual
carrier read with no Function-expecting forward pending through the new
`EmitRecorder.NoteWordRead(v, name, pos)`, and `stepWordVal` notes a `/v`
read through `NoteValRead`. The emitter counts them on the innermost open
unit (`fnUnitRec.wordReads` — fn-typed reads strictly; gradual ones name
themselves only) and at the unit's finish ARMS the whole-frame replay
over a residual carrying such a read whatever the count
(`noteWordReadReplay`: `dynFrameWindow`, the body-tail test anchored at
the READ's position rather than the carrier's — a param carrier has none,
and `[def y 1  g]` never read as a tail — and at most one value-semantics
applicable beside the word reads), recording the names and positions per
token (`fnUnitRec.dynFrameWords` → `CompiledFn.DynFrameWords`, keyed by
the op's unit-local pc; `noteDynFrameReplay`'s count-mismatch arming
records them too). The VM's `callDynFrame` reads the table
(`callDynFrameWords`): a region whose word-read entries hold plain data
and nothing appliable is the residual already and skips the island (the
identity fn over 5 costs no interpreter run); otherwise each fn-valued
word-read entry is installed as a frame binding under its name
(`InstallFrameBinding` — the interpreter's own param install, unquoted as
its arrival path delivers it), a compiled closure first bridged to a
FnDefInfo carrying one handler-bearing signature over the unit's declared
param types whose handler runs the closure on the VM (`closureAsWord`),
and the region re-steps with the WORD at the read's position in the
value's place — the interpreter's own dispatch, errors and positions
included — with the bindings popped in reverse afterwards. Then the
accounting: every fn-typed read the engine noted must be seated in the
window or consumed by a fn-value apply lowering (`creditWordRead` from
`RecordDynApply` and `RegisterTrailingApply` — accepted lowerings whose
0-arg and no-match faces are NUR122's), else the unit refuses
(`wordReadAccounting`): a list or map literal's member, an if-arm's
residual, a stack-collected argument, and a binding read both bare and by
`/v` (one value ID, two dispatch semantics — `[g drop g/v]` would have
FIRED the `/v` value) refuse soundly. A gradual read is best effort: it
arms when seatable and otherwise keeps the slot push it always had —
refusing it would refuse every `m k get` over an Any param, measured on
the corpus's own fn bodies (`TestDynScopeCaptureDef`,
`TestDoMapValueEvalNoDynEnv` fell over on the first cut).

**Measured.** Every fn-typed witness agrees on both lanes now, value and
error text alike (`lang/go/word_read_dispatch_test.go`, 27 parity rows and
7 refusal rows); the corpus stays at 0 refusals; `compiler/go/
word_read_test.go`, `eng/go/vm_dyn_words_test.go` and
`core/go/engine_word_read_test.go` pin the arms. The two rows of
`lang/spec/frontier/frontier-fnparam-deref.tsv` — the maintainer's
2026-08-15 ruling that a bare name is a CALL, held open because the
compiler read the param as a value — graduated into `fn-value.tsv` §7
(`typeof (grab nought)` is Integer on both lanes, `typeof (hold dbl)`
raises `cannot call `c`` on both) and the file is retired; frontier
ledger 84 → 82, corpus 7670 rows.

Three gates shaped the landing, each a measurement the first cut failed.
The interp-entry census (ceiling 33, a downward ratchet) rose to 44 when
the words island ran BEFORE the Apply kernel's frame push — `[act s]`
over a compiled lambda had entered the callee as a frame and now
islanded — so the kernel runs first and the words island takes only what
it declines; a word-read lead over plain data that matches NO prefix of
the tokens after it (`wordLeadNoMatch`: the interpreter's forward
collection takes what a signature needs and leaves the rest, so `[g x]`
over a 0-arg g FIRES) raises the no-match natively through `NoMatchDiag`,
the builder the interpreter's own dispatch uses, with no island; and the
`hold dbl` row still counted one `Engine.Run` — `InstallFrameBinding` →
`InstallFnDef` → the registry's `OnRegisterHook`, dynamic help's example
generator, which RUNS an engine for every fn-valued param bind on the
interpreter too. A shadowing install fires no register hook now
(`installFnDef`'s `hook` arm): a per-call frame binding is not a
registration. The refusal-site census (93, never rises) took the two new
refusals only once the three replay verdicts shared one site
(`fnResidualReplayReason`), and `Finalize` went over the gocyclo cap
until the words-table seat became `seatDynFrameWords`. One more, found
measuring the bridge: a closure unit recorded no declared param types
(`SetUnitParamTypes` was the CALL_USER site's), so the first bridge
declared every param Any and `g "s"` over a `z:Integer` lambda RAN the
closure (`[0]`) where the interpreter no-matches. A lambda's unit now
carries its contract (`lamParamContract`, seated by
`recordClosureDispatch` and `tryReturnedClosure`), the bridge declares
it, and a unit without one declines the bridge rather than guess. One thing stays open on
NUR123's record: the GRADUAL read the pass never types as a fn (the next
section re-measures it: a body-local bound to a container element, `def j
(m get "f")  j` is 42 interpreted and `fn` compiled, while a gradual
PARAM's read refuses — the faithful fix is the same word dispatch at the
READ site under a runtime "is it a fn" test, a per-read op rather than the
residual replay). A NOTES-only difference in the no-match error turned out to be
an interpreter diagnostic wart: `sigError`'s written-tuple walk did not
stop at engine markers, so a no-match at a body's end listed the frame's
DefCleanup marker as the argument the caller supplied (`… and __dc (a
__DC)`). Both walks stop at a Mark, a Move or an internal marker now
(`isEngineMarker`), the full-corpus gate — which compares notes — holds,
and the `hold dbl` row graduates with byte-identical diagnostics. NUR122 is narrowed: the whole-frame replay's
two witnesses agree now; the paren window `(g x)` keeps value semantics
(credited as an accepted read) and its empty-name no-match stays.

**The in-unit face of NUR120, found on the way.** `Program.ClosureRet` was
keyed by `len(p.Code)` from INSIDE a fn unit's lowerer — the main code's
length, a pc no unit ever reaches — so every closure pushed inside a fn
body lost its callback contract: `def f fn [[] [Any] [each (x:Integer =>
[x 1]) [1 2]]]  f` answered `[1 1]` compiled where the interpreter raises
the count error, and the named `cb/v` twin inside a fn the same (exit 0,
on the tree that closed NUR120). The table is per code now
(`CompiledFn.ClosureRet`, the lowerer keys its own emission target at its
own pc, the VM reads the current unit's — `closureRetAt`); five in-unit
rows joined `lang/go/lambda_return_count_test.go`. NUR120's number stays
retired: same defect, same fix line, pinned in the same file.

**Two more findings, recorded.** NUR124 (not fixed): a stack-shuffle word
that moves a produced closure to the top RETURNS it, and the interpreter
re-steps a native's returned fn where it lands — `[(mk 3)] each [5 swap]`
is `[15]` interpreted and `[fn (Integer)]` compiled, `[5 over]` 45 against
`fn`, `[5 swap drop]` each_error against `[5]` (the top-level spellings
refuse; the code-body one over a produced closure compiles through the
shuffle fold). NUR125: the check pass PANICKED (recovered as
internal_error) on `def h fn [[k:Any][Any][{a: k}]]  h ([] => [42])` — a
map literal over a gradual param holding a fn; both lanes, pre-existing at
89822d8; resolved in the next section.

**Blocker (a), re-sized again.** The lambda-unit arm still waits (task:
the returned closure's RetSpec on `tryReturnedClosure`'s operand, then
`DynApplyLeadEligible`'s `lambdaUnit` discipline). The word-read machinery
is what that arm should ride: a `[g x]` inside a returned closure is a
word read of a CAPTURE, and `NoteWordRead` already counts it on the
innermost unit when the capture is one of its locals — the lambda unit
needs only the replay arming the user-fn path now has (`!rec.closure` is
the gate to widen, with the count discipline of a user fn), and NUR122's
bare-spelling widening is moot because the replay dispatches by name.

## The word path's nil-handler guard (NUR125), and NUR123's leftover re-measured (2026-09-05)

**NUR125, resolved; number retired.** The panic was a nil function call in
`execMatch`. The check pass's `RunFnBodyOnce` (check/go/carrier.go) binds a
named parameter by pushing the ARGUMENT onto `Defs` — a concrete lambda
value included, so the binding holds a FnDefInfo that no `installDef` ever
gave a handler-bearing signature — and a map literal's const-fold sub-run
(`concreteEvalOnce`, check mode off) then dispatched the bare read of `k`
by name, the interpreter's word path over a bound FnDefInfo, into a
signature whose `DispatchHandler()` is nil. ADR-005 allows an error and
never a panic: `execMatch` now raises `internal_error` ("no runnable
implementation for `k` on the word path") on a nil runner, the fold
declines on it, and the literal records normally. The program's check pass
is clean, the interpreter answers `{a:42}`, and the compiled lane REFUSES
it (NUR123's strict accounting, below) and answers the same. Pinned in
`core/go/engine_word_read_test.go` (the raw push, the error — no panic)
and as refusal rows in `lang/go/word_read_dispatch_test.go`; the raw push
itself is left as it is, the guard being the ADR-005 floor under every
caller of the word path.

**NUR123's leftover, re-measured on this tree.** The record's first draft
called the gradual PARAM read consumed outside the residual divergent
(`[[k:Any][Any][k typeof]]  h ([] => [42])` "Integer / Function"). It is
not: the pass re-runs a gradual param under the argument's RUNTIME type — a
`Function` carrier when the call passed a fn (an instrumented build shows
`carrier=true parent=Function fnTyped=true` on the second run, after the
`dynamic(Any)` first) — so `NoteWordRead` counts the read strictly and
every consumption outside the residual refuses ("bare read of `k` is
consumed where the interpreter dispatches it"): `k typeof`, `[k]`,
`{a: k}`, `{a: (k)}` all refuse and answer the interpreter's value. What
stays open is the gradual read the pass never types as a fn: a body-local
bound to a CONTAINER ELEMENT, which the Map carrier types as
`dynamic(Any)` on every run —

    def h fn [[m:Map][Any][def j (m get "f")  j]]  h {f: ([] => [42])}
        42 interpreted, fn compiled;   j typeof: Integer / Function;
        {a: j}: {a:42} / {a:fn}   — default lane, exit 0, compiled

— noted best-effort (`Dynamic`, admits a fn), and the residual spelling
did not seat the replay either. The record carries the new witnesses; the
fix is the per-read op (the same word dispatch at the read site under a
runtime "is it a fn" test), the next increment.

## The gradual body-local's residual read seats (2026-09-05, the eighth increment)

**Root cause, located.** The engine noted the read; the emitter's
`NoteWordRead` dropped it at its first gate — `u.localByID[v.ID]` — because
a body-local `def` bound to a computed value is read through its PRODUCING
EVENT (resolveOperand's events-first rule), never through a slot: the
locals table held only the param. Three things fix the residual spelling:
a body-local producer of the unit (`producedBy` hit, not an
`enclosingBindIDs` snapshot — resolveOperand's own test) counts as the
unit's word read, named and gradual; the tail test anchors a word-read
entry at its READ (`replayIsBodyTailAnchored`, `SetPos` — `WithPosAt` keeps
an existing position, which a produced value and a param carrier both have,
so the sixth increment's anchoring had never applied), admitting every
event between the producer and the read as run-before-dispatch on both
lanes and declining only an event after the read; and the VM's word-read
no-match reads the INSTALLED binding (`Registry.Lookup`) with the closure
bridge carrying `BarrierPos = len(params)` as `compileFnDef` resolves it,
so the "group the call in parens" help line appears on both lanes. A
binding read both bare and by `/v` is kept out of the seat
(`wordReadName`) so the accounting's refusal stays the one that names the
mix. Measured on the family (`lang/go/word_read_dispatch_test.go`,
TestBodyLocalWordReadParity, full-text parity):

    def j (m get "f")  j           42 / 42   (was fn)     {f: 5} → 5, no island
    … j 3                          9         … def y 1  j     42      … j drop  j   42
    … x j (stack-collect)          15        … m.f  j         42      … (j)         42
    … {f: (z:Integer => [z])}      cannot call `j`, help line and all, both lanes

**Still open, best-effort slot pushes.** The read consumed outside the
residual (`j typeof` Integer / Function, `{a: j}`, `if true [j] [0]`, `[1]
each [j]`, `do [j]`) or followed by an event (`j  def y 1`, `j  5 drop`):
42 interpreted, `fn` compiled, exit 0. `j j add` reaches the VM's
CALL_NATIVE_POLY no-match deferral and answers 84 through the interpreter
(slow, not wrong — and not a native compile). `j/v` renders `fn` for the
interpreter's `fn j` (NUR119). Refusing a gradual read consumed elsewhere
would refuse every `def n (m get "k")  n add 1` in the corpus, so the fix
is a per-read DEOPT: after the read's push, an op that tests the value at
run time and, on a fn, hands the rest of the body (the source tokens from
the read on) to the interpreter with every frame name registry-visible
(OpBindDynScope for the params and the defs so far), then jumps to the
unit's RET with the island's residual — the residual replay generalised to
a mid-body start. Closure units decline it at first.

## A gradual read consumed anywhere deopts to the interpreter at its statement (2026-09-05, the ninth increment)

**What was left.** After the eighth increment a body-local bound to a
container element (`def j (m get "f")`, typed `dynamic(Any)` on every run)
read bare in its unit's RESIDUAL seated the whole-frame replay; the same
read consumed anywhere else — `j typeof`, `{a: j}`, `if true [j] [0]`, a
read an event follows (`j  def y 1`), a def sourcing it (`def k j`) —
kept the slot push and answered a fn value (or, for the def, a value)
where the interpreter dispatches the binding as a WORD. Refusing a gradual
read consumed elsewhere would refuse every `def n (m get "k")  n add 1`
in the corpus; the check pass cannot type the element; so the read is
guarded at RUN TIME.

**The mechanism (OpDeoptIfFn, `CompiledFn.Deopts`, `CompiledFn.Body`).**
At the unit's finish `planDeopts` turns each such read — a producer of
the unit (resolveOperand's events-first rule), not fn-typed (those are
accounted strictly), not credited to an apply lowering — into a deopt
point placed where the statement holding the read BEGINS
(`deoptPointFor`): at the read's own push when its consumer follows it on
the stack (`j typeof`, `a j add`, `x j` — the compiled stack then holds
exactly what the interpreter's held at the read: the lowerer emits the
test in `pushOperand`, keyed by the promoted slot, `deoptAtSlot`); at the
read itself, off the push, when an event runs between the read and that
consumer (`j (m get "g") add`: the get runs before the push, so the test
goes before the get, where the stack is the read's); before the
consumer's first op when a forward word or a def collects the read (`def
k j`, at the `def`; `def k (j)` likewise — the def's source HOLDS the
read), a branch holds it in an arm (`if c [j] [0]`, at the `if`, found by
scanning back from the arm) or a loop body holds it (`for 1 [j typeof]`,
at the `for` — the loop event stands at its count); at the literal's or
paren's own token when the read sits inside one (`{a: j}`, `[j 1]`, `(j
typeof)`, `((m get "g") j add)`, `[(j typeof)]`) or a word after the paren
consumes it (`(j) typeof`); at the read when nothing consumes it and an
event follows (`j  def y 1`, `j  (m get "g") drop`). The check pass
seats the fn's body tokens on the unit (`SetUnitBody`, from
check_fnbody's `bodyCopy`) and the point names the body token the
statement starts at. At run time the VM tests the binding's value; plain
data costs the test alone; a fn hands the rest of the body to the
interpreter (`deoptIfFn`: `runIslandResolved` over the frame region as
the resolved prefix — the read's own stack entry dropped when it lives
there — with the body tokens from the statement's token), tears down the
defs the island made (`core.TruncateFrameDefs` over a snapshot taken at
the deopt: the interpreter's __dc duty, done by hand since the marker is a
frame-tape token), replaces the frame region with the island's residual
and continues at the unit's RET, which applies the RetReplay discipline.
A deopt unit runs under the interpreter's frame environment: every named
param and every body-local def is registry-visible (`emitDynParamBinds`,
`lowerDynBind`'s `unitDynEnv`; the bind's own source re-push is not a read
— `lowerer.binding`), every def's computed source is promoted
(`collectDynBindSources`, `promoteLateDynBind`), and the unit never
tail-calls (`unitBindsDynScope`). The whole-frame replay's word island
still seats a lone residual read; a deopt is planned only for the rest.

**Soundness, and what declines.** The island is faithful only when the
interpreter's stack at the statement's start is the compiled stack at
the test. An operand of the consumer (for a point tested before it) or of
a LATER root event, or an inert operand of the residual, written before
the start — a literal (a pooled scalar counted by canon against its
consumptions, a def of a literal consuming its own: `def y 1  for 1 [j
typeof]`; a compound by its own position, or by canon when the fold
re-minted it without one — `[1 2]  (j typeof)  drop` pushes the list
FRESH after the drop, past the island's jump to RET, and answered no
value at all before the accounting saw it), a local or def read there
more often than consumed (`NoteLocalRead` records every bare read's
position per unit — both of stepWord's substitution paths), or the
result of an event before the start that no def consumed (a def-bound
result is promoted to its slot here and bound in the interpreter's frame,
and the island reads it by name: `def n (m size)  def j (m get "f")  j  n
drop` is 42) — is a value the compiled stack still lacks, and such a
point DECLINES (best effort, the slot push it always had): `5  j typeof`
and `def y 5  y  j typeof` (both lanes raise the count error; the texts
differ, `[5 Function]` for `[5 Integer]` — on NUR123's record), `x {a: j}
size add`, `5 (j) add` (which the paren apply's runtime deferral then
answers, 47 on both lanes). A unit whose island would spell a body-local
FN def declines too (`deoptDefsBindable` mirrors `lowerDynBind`'s literal
admission: inert data, or a value resolving to a const or a local — a fn
value is neither), found by the corpus: `sift-check-detect`'s `(paths is
None)` planned a point whose tail spells `def check-paths fn […]`, and
eight `module-sift.tsv` rows refused at that bind. A read nested in a
closure body is not this unit's to place: closure units (`[1] each [j]`, `do [j]`) keep their slot
push — the read lives in the closure unit, whose frame is the driving
handler's — the next increment. `j/v` still renders `fn` for the
interpreter's `fn j` (NUR119).

**Measured** (`lang/go/word_read_dispatch_test.go`,
TestBodyLocalDeoptParity — 61 full-text parity rows): `j typeof` Integer,
`{a: j}` {a:42}, `{a: j, b: (print "x")}` prints once, `if true [j] [0]`
42 (and the else arm, a computed condition over a param, the if's result
under `typeof`), `j  def y 1` 42, `j  5 drop` 42, `j j add` 84 natively,
`5 j add` 47, `a j add` 43, `1 2 j drop add` 3, `def k j  k` the
interpreter's "def is still waiting" error, a fn with effects fires once
and effects before the statement never repeat (`print "before"  j
typeof`), and every plain-data twin pays the test alone; the paren-nested
reads (`(j typeof)` Integer — was Function, `(j) typeof`, `((m get "g") j
add)` 43, `[(j typeof)]`, `{a: (j typeof)}`, `(j typeof) typeof` Number,
`((print "in") j typeof)` prints `in fired`, `def y 5  (j typeof)  y add
1  drop`), the loop body (`for 1 [j typeof]`, `for 1 [(j typeof)]`, `def
y 1  for 1 [j typeof]`), `def k (j)  k` 42, `j (m get "g") add` 43, the
def-bound result read by name (`def n (m size)  … j  n add` 43), an event
after a residual read (`j  (m get "g") drop`, `j  print "x"`), a def the
island never reads keeping its plain lowering (`def y (m get "g")  def j
(m get "f")  j typeof`). Compiler pins: TestPlanDeoptsShapes,
TestPlanDeoptsDeclines, TestPlanDeoptsNestedReads,
TestPlanDeoptsAccounting, TestPlanDeoptsStartDeclines,
TestEmitDeoptsBeforeStackHome, TestPromoteLateDynBindDeoptNames,
TestBodyTokenHelpers; VM: TestDeoptIfFnArms (its three defensive arms are
pinned and lost their coverage pragmas). The corpus differential is clean
and no corpus row refuses (the deopt adds no refusal site;
`runIslandResolved` is the interp-entry census's existing site).

## A code body's read of a captured fn-holding local deopts too (2026-09-05, the tenth increment)

**What was left.** The ninth increment skipped closure units, so a `[…]`
body a native runs inside the caller's frame (each, do, fold, for) still
pushed the slot for a bare read of the enclosing fn's body-local: `def j
(m get "f")  [1] each [j]` was `[fn]` for `[42]`, `[1 2] each [j]` over a
1-arg lambda `[fn (Integer) fn (Integer)]` for `[3 6]`, `do [j]` `fn` for
42, `do [j typeof]` Function for Integer (default lane, exit 0).

**The mechanism.** `compileClosureBody` seats a non-escaping code body's
tokens on its unit (`SetUnitBody`; a lambda value — `fnval` — a stored fn
and a spawned body escape the frame their bindings live in and seat
none). `planDeopts` then plans a closure unit's points as a fn unit's,
with one difference: a CAPTURED value's home is its capture slot
(`deoptPoint.slot`; `deoptConsumer` matches the consumer by that slot),
so the atPush test keys on the slot and an at-event test names it
directly. The island reads the captured names through the registry, so
the ENCLOSING unit must bind them: at the closure's finish
`seedParentDeopt` reads the parent's root frame — still open, at
`fnUnitRec.rootFrame` — and admits each captured name the tail spells
when the parent binds it from a re-pushable source (a root-level def of a
literal or of a single-output call, or a param); a def of the name inside
one of the parent's nested open frames (the arm the closure sits in)
declines. The parent then keeps that environment even when it plans no
points of its own (`planDeoptsEnv`) and, should a later event make a
seeded name unbindable, drops its children's points with it
(`dropDeoptChildren`). The code body runs inside the parent's frame
(each, do call it there), so the parent's `BIND_DYN_SCOPE` entries are
live when the closure's island runs; the closure's own RET follows the
island (the nested run's top RET).

**Measured** (TestBodyLocalDeoptParity, eleven more rows): `[1] each [j]`
[42], `[1 2] each [j]` [3 6], plain data `[5]` and `[6 7]`, `do [j]` 42,
`do [j typeof]` Integer, `do [(j typeof)]` Integer, `do [def y 1  j]` 42,
`if true [do [j]] [0]` 42, `[1] each [j] size` 1, `5  do [j]  add` 47.
`for 2 [j]` and `[1 2] fold [j] 0` agree on the value and the message and
differ only in the return-contract error's position (1:24 against the
call site) — NUR118, pre-existing, the same on their plain-data twins.
`[1] each [(j typeof)]` and `[1 2] fold [j add] 0` refuse before the
point is planned ("result above a literal (Stage 3)", pre-existing) and a
lambda body over the local (`[1] each [x:Integer => [j]]`) refuses at the
code-body word — sound fallbacks both. Compiler pins:
TestPlanDeoptsCaptureSeedsParent (the capture point, the seeded parent,
the dropped children, a rebind of the name in one of the parent's OPEN
nested frames — the arm the closure sits in, whose bind is popped with
it — and the top-level body with no enclosing unit),
TestPlanDeoptsChildSeededNames (a unit that plans points of its own and
carries a child's seeded names binds the union) and
TestSeatUnitDeoptsUnpromoted (an at-push point with neither a capture
slot nor a promoted source seats nothing, and a diverging body seats no
points at all); the lambda and stored-fn units keep their slot push
(TestPlanDeoptsDeclines).
The corpus differential is clean and no corpus row refuses.

**Still open on NUR123.** `j/v` renders `fn` for the interpreter's `fn j`
(NUR119); a point whose statement the compiled stack cannot match
declines (`5  j typeof`, `x {a: j} size add`); a body-local read in a
lambda value's body (`([] => [j])`) escapes the frame and keeps its slot
push.

## A capture-free factory's lambda carries its own arity (2026-09-05, the eleventh increment)

**What was refused, and why the reason was half right.** A def-bound
factory result applied by its binding's read — `def mk fn [[n:Integer]
[Function] [( fn [[x:Integer][Integer][x add 1]] )]]  def h (mk 1)  (h
2)` — refused as "def-bound computed fn apply (closure shape unknown —
Stage 1)". The shape was not unknown; the recovery was looking in one
place. `producerReturnedClosureArity` answered only when the factory
unit's single out op was an `opClosure`, which happens exactly when the
returned lambda CAPTURES something (`[x add n]` over the factory's `n`
lowers to PUSH_CLOSURE). A capture-free lambda is a constant, so the fold
bakes the whole thing as ONE const value (`PUSH_CONST … (Function)`,
with its body compiled beside it as a `storedfn$body` unit) and the
recovery declined — though the arity is written on that const's own
signature.

**The measurement that separated them.** Nine factory rows on the default
lane: every one whose returned lambda reads an enclosing binding compiled
already; every capture-free twin refused, whatever the factory's own
params, whatever the spelling (`fn [[…]]` or `=>`), and whatever the
inner param's type. The discriminator is the CAPTURE, not the arity, not
the gradual param, and not (as the ledger's note guessed) an unused one.

**The mechanism.** `producerReturnedOutOp` is the walk both questions
share; two callers now ask different things of it.
`producerReturnedClosureArity` gains a CONST arm — `constLambdaArity`
reads the arity off a baked ANONYMOUS lambda: one own signature, a boru
body, no name, no module origin. Those exclusions ARE the fn-value
apply's soundness argument (`resolveDynamicApply`): OpCallDynamic runs
anonymous-VALUE semantics, which leave a NAMED Go-impl fn — `add/v`, a
module export, a `FnUtil.const` result — as data where the interpreter's
word dispatch would apply it, so those keep the refusal.
`producerReturnedClosure` is the OTHER question, and it had been sharing
the same function by accident: `argIsProducedClosure` and the
dyn-bound-closure note guard a ClosurePayload — a value only the VM's
re-entrant runner can invoke — which a const-baked lambda is NOT. Left
merged, the const arm made `def mk fn [[] [Function] [([] => [42])]]  def
f fn [[g:Function][Any][g]]  f (mk)` refuse where it had compiled; split,
it compiles again. The split is the increment's real lesson: one helper
was answering "what arity" and "is this a payload" at once, and only the
first was ever true of a const.

**What graduated, measured on both lanes.** The def-bound apply family:
`(h 2)`, the bare `h 2`, the zero-param factory, the `=>` spelling, the
gradual inner param (`[[x:Any][Any][x typeof]]` — Integer on both lanes),
the multi-arg callee, and the factory that takes a Function it never uses
(the ledger's "unused capture" note, now explained). And the trailing
KEEPQ family, whose refusal had a different stated cause — "runtime quote
state unknown" — that turns out to have been an ARITY problem all along:
`CALL_DYN_TRAIL_KEEPQ` already decides the quote at RUN time (inert when
quoted, applied when not), and what it lacked was the callee's arity to
trim the window with. `def choose fn [[][Function][quote (fn [[a:Integer
z:Integer][Integer][a add z]])]] (1 2 choose)` is `1 2 fn (Integer,
Integer)` on both lanes, its unquoted twin is `3`, and a 1-arg callee
under a 2-wide window trims to `1 3`.

**What still refuses, and each was measured.** A callee with more params
than the window has values (the interpreter leaves it unapplied and
nothing here models that); a factory whose ARMS return different lambdas
(no one provable shape — the row that keeps the graduation honest); a
curried chain `((h 2) 3)`; two applies of one binding; a named Go-impl or
module fn value. The park's own rows are untouched: `(mk 1) 2` is still
`fn (Integer) 2`.

**Pins.** `lang/go/returned_closure_park_test.go` (the def-bound apply and
its paren twin as parity rows, moved off the fallback list),
`lang/go/bytecode_dynapply_body_test.go` (both quote polarities, the
window trim, and the two standing refusals),
`compiler/go/zz_triage_split_check_test.go` (TestConstLambdaArity over
the anonymous / named / module / Go-impl / two-signature axis, and
TestProducerReturnedClosureConstArm holding the two questions apart).

## The twelfth increment was attempted and reverted, and what it measured (2026-09-05)

**The blocker chain, corrected.** Surfacing the closure PROBE's own
refusal (a temporary print at `tryReturnedClosure`'s `!probeOK`) shows
the 22-row closure-capture family's blockers (a) and (c) are ONE refusal,
not two gates: `closure fnval$body: body value count differs from
declared returns`. A lambda VALUE's body whose residual is a
word-dispatch window (`[g x]` over a captured g leaves [g, x] in the
model) is count-refused before it can reach the replay classifier a fn
unit reaches. The `Any`-typed paren twin refuses identically; the gate
the ledger named ("cannot prove its argument non-function") never fires.
`[if … [(g x)] [0]]` refuses earlier still ("if: then-branch result of
unknown provenance") and `[g/v]` alone as "unapplied fn-value in body
residual".

**The attempt.** Four edits, each measured: the replay classifier admits
a lambda unit (`rec.closure && !rec.lambdaUnit`); the closure count
refusal defers to it; `closureResidualHasUnappliedFn` skips an armed
replay (`rec.dynFrameW > 0`); and — found by the sweep —
`tryReturnedClosure` stamps the count contract its operand had been
missing (`fnValueRetSpec`, the same spec the callback path uses; without
it `[g x 1]` answered `15 1` where the interpreter raises). With all
four, `[g x]` and `[x g]` compile and answer 15.

**Why it was reverted.** The whole-frame replay islands VALUES; the
interpreter steps TOKENS. Succeeding dispatches agree; FAILING ones do
not. `def app fn [[g:Function][Function][( fn [[x:Integer][Integer][g
x]] )]]  def h (app (z:String => [z]))  (h 5)` raises the same headline
at the same position on both lanes, but the interpreter's notes read
`candidate `g (String)` takes 1 argument, but none were supplied` (its
forward token is the WORD `x`) where the island's read `the argument was
5 (an Integer)` (its region already holds the 5). Making more programs
compile at the price of more live error-text divergences is the trade
this line has refused before.

**The mechanism that can.** The ninth increment's DEOPT hands the
interpreter the BODY TOKENS (`CompiledFn.Body` from the statement's
token), which reproduces `g x` exactly — notes and all. The tenth
increment excluded `fnval` units from `planDeopts` because a lambda
value escapes the frame its bindings live in; but a lambda value applied
through the VM runs in its OWN frame, whose params and captures are its
locals, so a SELF-CONTAINED deopt (params and captures bound at the
closure's entry, no parent seeding) is the shape to try next. The
missing `closureRet` stamp above is real either way — today unobservable
only because the count check refuses first.

**Recorded beside it**: NUR122 gains a third witness, live on main — the
already-compiling paren form over a captured NAMED fn names that fn
where the interpreter names the param (`cannot call `add`` at 1:80
against `cannot call `g`` at 1:64).

## A returned lambda's computed capture was baked as a constant (2026-09-05, the twelfth increment, NUR126)

**Found by walking away from a refusal.** The attempted blocker-(a) work
above ended in a revert; measuring what a lambda VALUE's body does with a
GRADUAL capture — the shape NUR123's record listed as open — turned up a
wrong ANSWER instead: `def h fn [[m:Map][Function][def j (m get "f")  (
fn [[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)` answered
`7`, the caller's own argument, for the interpreter's `42`. The factory's
bytecode computed the capture, DROPped it, and pushed `PUSH_CONST` over
the producing event's SEQ read as a const index. The same emit panics the
disassembler outright when that index is out of range — which is how the
defect was confirmed rather than guessed.

**The invariant that broke.** `pushOperand` materialises const, local and
type operands only ("event operands are already on the stack", its own
comment), and its switch ends in a `default` that treats everything else
as a const. Nothing enforced that; the enforcement was supposed to come
from upstream, where `planValueDefLocals` promotes a producer that
something still references and `forEachOperand` deliberately surfaces a
closure's captures so a def used ONLY as a capture is counted. A RETURNED
lambda never passes through that walk — its closure operand is the unit's
RESIDUAL — and the planner's residual input listed the out-ops' own event
seqs with captures invisible.

**The fix is one walk.** `appendResidualSeqs` collects the seqs a residual
references INCLUDING each closure's captures (recursing through nested
closures), and the fn-unit planner takes it; the capture then lowers as
`STORE_LOCAL` + `PUSH_LOCAL`.

**A latch was tried beside it, and withdrawn — the reason is worth
keeping.** `EmitState.capFail` made pushOperand record a capture it could
not materialise so Finalize would refuse rather than ship the const push.
It cost two corpus rows: `def cols (vt-rtable-cols scr.name)` captured by
a lambda inside the SAME `case` arm (`vault_tui.boru:1106`, :1109) is a
producer the planner promotes only at a unit's ROOT, so it still reaches
the push as an event. Those rows pass today because the closures they
build are never invoked — the wrong capture is unobserved — so the latch
would have traded two compiling rows, against a refusal ceiling of 0, for
no observable correctness. Promotion INSIDE an arm is the increment that
closes it; until then the shape is recorded on NUR126 with its two corpus
sites rather than guarded.

Instrumenting the planner narrowed that increment's first move: this is a
COUNTING gap, not a rewrite one. `RewritePromotedRefs` already recurses
into fragments and rewrites `closureCaps`, but the producer never enters
`planValueDefLocals`'s `captured` set, though an event capture does reach
the push (`EVENTCAP unit=23 kind=2 idx=165` at `vault_tui.boru:1106`).
Three hypotheses were then tested and eliminated (the detail is on
NUR126): the body operand is NOT kept outside `ev.call.ops`
(`RecordClosureCall` appends an ordinary `evCall` whose `ops[bodyPos]` is
the closure); the planner does not see-and-decline it (its walk never
reports that capture); and it is not the unit's residual closure needing
the capture mark (seeding `captured` from `rec.outOps` changes nothing,
and no residual carries it). The mechanism is now traced (the caller path at the
push): the closure is an ordinary call operand inside a MULTI-VALUE arm,
and `collectPromotableEvents` recurses only into `residualN <= 1`
fragments — so the planner sees neither the capture nor the producer,
while `lowerFragment` lowers the arm regardless. Collecting captures over
the whole fragment tree is measured NOT sufficient: the promotion loop
iterates the same restricted set, so the producer is still not a
candidate. The remaining work is to make an intermediate producer inside
a multi-value arm promotable without disturbing that arm's residual
values — the distinction the walk's own comment protects. The planner
cannot draw it today: `fragmentResultSeqs` marks only the single out
operands and a multi-value arm's residual values appear in no recorded
list (`EmitFragment.residualOps` covers only the inert multi-out loop
body), so the increment begins in the RECORDER by capturing that list and
only then extends the walk.

**Measured.** Every value kind a capture can hold (Integer, String, List,
a computed param expression, two computed captures at once, the param
itself), the capture used together with the lambda's own param, and a
second instance of the same factory keeping its own captured value — all
agree on both lanes and compile natively
(`lang/go/closure_capture_promotion_test.go`). The rows that refuse still
refuse for their pre-existing reasons ("fn h: body result of unknown
provenance"), and the corpus differential is clean with no new refusal.

**One thing this corrects.** NUR123's record said a lambda VALUE's body
"refuses soundly". It does not: with the bogus constant gone, the
fn-valued capture read bare as the body answers `fn` where the
interpreter dispatches the binding as a word and answers 42 — a live
divergence the constant had been masking. The record now says so, and the
fix is the deopt inside a lambda unit named above.

## A multi-value branch arm's residual is now RECORDED, not counted (2026-09-05, the thirteenth increment, NUR126)

**What was measured first.** The twelfth increment left NUR126 half open: a
def made inside a branch or `case` ARM and captured by a lambda built in the
same arm still reached `pushOperand` as an `opEvent`, and that switch's
`default` emitted `PUSH_CONST <event seq>`. The corpus's only witnesses were
two vault-tui sites whose closures are never invoked, so the trace was run
back to a standalone one:

```
if true [ def c (7777 add 1)  99 (each [ (r:Integer => [ r add c ]) apply ] [1 2]) ] [ 7 8 ]
    interpreted  99 [7779 7780]        compiled  7778 [7778 7779]     ← default lane, exit 0
def m {a: 3}  if true [ def c (m get "a")  99 (each [ … ]) ] [ 7 8 ]
    interpreted  99 [4 5]              compiled  3 [1 2]
```

The disassembly named both faults at once:

```
0005 CALL_NATIVE s0   ; add          ← 7778, and the global bind does not pop it
0006 BIND_GLOBAL g0
0008 PUSH_CONST  k1   ; 7777         ← the capture, lowered as a const over the event seq
0010 CALL_NATIVE s1   ; each
```

The literal `99` was never pushed at all. `lowerFragment`'s multi-value arm
path only COUNTED the sim slots (`len(lw.vm) == frag.residualN`), so the
`add`'s leftover filled the slot the interpreter holds `99` in and the count
matched anyway. The capture was the same defect one layer up:
`collectPromotableEvents` refuses to walk a `residualN>1` fragment, so the
arm's interior was never a promotion candidate.

**Why the planner could not fix it alone.** Both faults are the same missing
fact — the planner has no record of WHICH values a multi-value arm leaves.
`fragmentResultSeqs` marks only the single out operands (`thenOut`, `elsOut`,
`condOut`, `bodyOut`); the several residual values of a multi-value arm lived
on the lowering sim and in no recorded list. So the increment begins in the
RECORDER.

**The change.** `captureArmResidual` (compiler/go/emit.go) replaces
`captureInertArmResidual`: it records a multi-value ARM's residual as a
resolved operand list with EVENT entries admitted, where the loop side keeps
the all-inert restriction — a `for` body re-pushes its residual on every
iteration, and a frame slot would hold only the last value; an arm runs once
per taken path. Then, in the lowerer:

- `armRepushableResidual` opens `collectPromotableEvents`' walk (both passes,
  so the fragment ids stay in lockstep) into an arm captured whole;
- `armResidualForceSeqs` names that residual's event entries and
  `planValueDefLocals` merges them into `forceOrder`, exempting them from
  `fragResultStaysOnSim` — the top entry IS the arm's out operand, which the
  stay-on-the-sim rule would otherwise pin;
- `RewritePromotedRefs` rewrites `frag.residualOps` so the re-push reads the
  allocated slots rather than the producing seqs.

The arm then lowers exactly as the PROGRAM residual always has — store each
computed value, push the whole residual in the interpreter's order:

```
0006 STORE_LOCAL l0     ; the add
0007 PUSH_LOCAL  l0     ; … for the bind
0010 PUSH_LOCAL  l0     ; … and for the capture
0013 STORE_LOCAL l1     ; the each result
0014 PUSH_CONST  k5     ; 99
0015 PUSH_LOCAL  l1
```

**Measured payoff.** Over the twelve rows now pinned in
`lang/go/multivalue_arm_residual_test.go`: four exit-0 wrong answers become
correct, five arms that refused ("branch leaves extra values") compile, and
three unchanged shapes (a single-value arm, both directions of the all-inert
`if c [99] [1 2]` merge) lower as before. The langspec differential, the
whole-corpus fallback suite, the combination matrix and the property
differential are clean, with no new refusal.

**What it does NOT reach, named exactly.** The screen `captureArmResidual`
inherits from the loop side — decline a residual containing a parked
Function, whose auto-apply the interpreter performs when a later value lands
above it — is what still excludes the corpus's own two sites.
`lang/go/modules/vault_tui.boru:1105` and :1108 sit in a `case` arm that
leaves FOUR values led by the deferred `Tui.table` member, so the capture
declines at entry 0 (`ARMCAP decline fn n=4 at 0`) and the arm keeps the
count-only path with its bogus capture. Those rows pass only because the
closures are never invoked. Admitting a parked Function to the re-push is the
auto-apply family (NUR121, NUR124), not this increment, and a refusing latch
was already tried and withdrawn here: it costs those two rows against a
corpus refusal ceiling of 0 while fixing nothing observable.

## A lambda VALUE's body plans its deopt from its OWN frame (2026-09-06, the fourteenth increment, NUR123)

**The shape the twelfth increment uncovered.** With NUR126's bogus constant
gone, a fn-valued capture read bare as a returned lambda's whole body stood
exposed as a live divergence — the interpreter dispatches the binding as a
WORD, the compiled lane pushed the slot:

```
def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any][j]] )]]  def q (h {f: ([] => [42])})  (q 7)
    interpreted  42            compiled  fn          ← default lane, exit 0
… [[x:Integer][Any][j typeof]] …
    interpreted  Integer       compiled  Function
```

The `{f: 5}` twins agreed on both lanes throughout, which is what places the
fault on the word-dispatch rule and not on the capture.

**Why the tenth increment could not reach it, and where its reasoning was
half right.** `callable_words.go` withheld the body tokens from every
escaping unit:

```go
// an escaping body — a lambda value, a stored fn, a spawned body — has
// no frame to resume in and seats none.
if word != "fnval" && word != "storedfn" && word != "spawnbody" {
	es.SetUnitBody(unit, bodyToks)
}
```

and `planDeopts` bailed to `planDeoptsEnv` for the same units. The reason
given is true of ONE frame and false of another. `seedParentDeopt` — the
code-body route — asks the ENCLOSING unit to bind the captured names
registry-visibly, which is sound precisely because `each` / `do` / `fold` /
`for` run the body IN that frame. A returned lambda is applied later, when
the factory's frame is gone, so that route is genuinely unavailable. But the
lambda's OWN frame is live for the whole apply, and its captures ride in
slots `nParams…nParams+nCaps-1` — so the unit can bind them itself.

**The change, in three seams already built.** `callable_words.go` seats body
tokens for `fnval` (a `storedfn` / `spawnbody` is invoked by the host outside
any such call and still seats none). `planDeopts` routes such a unit through
a new `lambdaNamesSelfBound` instead of `seedParentDeopt`: every name the
island spells must be one of this unit's own locals (a param or a capture), a
def the island's own tokens make, or a word the registry resolves — anything
else would have resolved from the factory's frame and the points decline, as
they did before. `emitDynParamBinds` then extends its EXISTING
`OpPushLocal` + `OpBindDynScope` loop over the capture slots for such a unit,
so `RET` truncates them exactly as it already does for params.

**Measured**: nine rows agree that did not — the bare read, `j typeof`, a
read inside a list literal, the capture used with the lambda's own param
(`x j add` → 49), two reads that both dispatch (`j j add` → 84), the plain-data
twins that pay the test alone, and a fn whose effects must run exactly once.
Two shapes keep the whole-program refusal, a sound interpreter fallback:
`{a: j}` as the lambda's body ("body result of unknown provenance") and two
factory instances live at once ("fn value precedes residual args").

**Still open, measured against the pre-change binary so it is not read as a
regression**: a read inside a BRANCH ARM of a lambda body —
`( fn [[x:Integer][Any][if true [j] [0]]] )` — still answers `fn` for the
interpreter's 42. The arm plans no point and keeps its slot push, exactly as
every lambda body did before this increment. That and the declined points
(`5  j typeof`) are what remains of NUR123 besides NUR119's render.

## The lambda-body deopt's own gate was too strict (2026-09-06, the fifteenth increment, NUR123)

**What the fourteenth increment left, and what it actually was.** That
increment recorded one shape still diverging — a read inside a BRANCH ARM of a
lambda body. Measuring the family first, before touching the arm machinery,
split it in a way that named a different cause:

```
( fn [[x:Integer][Any][if (x gt 3) [j] [0]]] )     agrees      ← a DYNAMIC condition
( fn [[x:Integer][Any][if true    [j] [0]]] )      42 vs fn    ← a CONSTANT one
```

Both put the read in an arm, so the arm was not the discriminator. The
disassembly confirmed it: the dynamic row emits `BIND_DYN_SCOPE` for both the
param and the capture and then `DEOPT_IF_FN`; the constant row emits neither,
so the unit had planned no points at all. An instrumented run placed the
decline precisely — `deoptPointFor` returned ok for both, and the post-loop
gate rejected the constant row over the name `true`:

```
SELFBOUND reject "true" own=map[j:true x:true] made=map[] all=map[if:true j:true true:true]
```

`lambdaNamesSelfBound` had been written as a REGISTRY-membership test: a name
the islands spell must be an own local, a def the island makes, or a word
`Registry.Lookup` resolves. `true` is spelled as a word and is not a registry
entry, so a token that needs no binding at all declined the whole point.

**The rule that is actually load-bearing.** A lambda serves its islands from
its own frame, so the ONE name it cannot serve is one an ENCLOSING unit
supplies — that unit's own def or its param — and that this lambda did not
capture: the island would read it out of a frame that no longer exists.
Everything else resolves exactly as the interpreter resolves it there: this
unit's own locals, a def the island's own tokens make, a native or module
word, a top-level (global) def that outlives every frame, and a bare
literal-word. The test now scans the still-open enclosing units — their
`locals` and their root frame's `evDynBind`s — instead of the registry.

**A second gap the same rows exposed.** A nested code body inside a lambda
(`( fn [[x:Integer][Any][[1] each [j]]] )`) routes through `seedParentDeopt`,
which asks the PARENT to bind — and the parent is the lambda. It reached
`planDeoptsEnv`, which set `deoptEnv` but not `lambdaDeopt`, so
`emitDynParamBinds` bound the params only and the child's island could not
resolve the capture. Marking it there (behind the same
`lambdaNamesSelfBound` check) closes it.

**Measured**: six of the seven rows in the family agree that did not —
`if true [j] [0]`, its else-arm twin, the arm under a trailing `typeof`, the
already-working dynamic-condition row, the plain-data twin, and
`[1] each [j]`, which had been raising ``did you mean `h` or `q`?`` where the
interpreter answers `[42]` (a wrong ERROR, not just a wrong value).

**Still open, and NOT a deopt gap** — worth stating precisely, because the
obvious guess is wrong. `( fn [[x:Integer][Any][do [j]]] )` answers `fn` for
the interpreter's 42, but the deopt machinery is working there: the lambda
binds `j` and the `do$body` unit carries its `DEOPT_IF_FN`, exactly as the
`each` row does. The fault is in the lambda body's RESIDUAL model:

```
0002 PUSH_LOCAL  l1     ; j
0003 PUSH_CLOSURE f2    ; do$body
0004 CALL_NATIVE s0     ; do (List)
0005 DROP               ; ← the do's result, discarded
0006 PUSH_LOCAL  l1     ; ← and the raw slot returned in its place
0007 RET
```

The `do` body's value IS `j`'s value, so its result carrier is `j`'s carrier;
`resolveOperand` maps the body's residual back to the LOCAL, the lowering
re-pushes the slot and drops the call — losing the dispatch the island
performed. That is a residual-identity defect in the same family, not a
missing deopt point, and it needs its own increment.

**One contract this changes on purpose.** `compiler/go/deopt_test.go` asserted
"a lambda unit keeps its slot push". That was the pre-fourteenth-increment
contract; the test now asserts the current one (a lambda plans from its own
frame and carries `lambdaDeopt`), and `TestLambdaNamesSelfBound` covers both
directions of the new gate — own param and capture, an island-made def, a
native, the bare literal-word that caused this, and the two rejects (an
enclosing unit's param and its frame def).

## A capture's slot overrode an event of the unit's OWN body (2026-09-06, the sixteenth increment, NUR123)

**The remainder the fifteenth increment named, confirmed and located.** That
increment recorded `( fn [[x:Integer][Any][do [j]]] )` as still answering `fn`
for the interpreter's 42, and said it was not a deopt gap. Measuring the
family fixed the discriminator exactly, and it is not `do` either:

```
def h fn [[m:Map][Any][def j (m get "f")  do [j]]]                    agrees   ← a PLAIN fn body
( fn [[x:Integer][Any][do [j typeof]]] )                              agrees   ← the point fires inside
( fn [[x:Integer][Any][do [j]]] )                                     42 vs fn ← a LAMBDA body
```

The same `do [j]` agrees in a plain fn body and diverges in a lambda, so the
ENCLOSING UNIT KIND is the variable. The two disassemblies say why:

```
plain fn body            lambda body
CALL_NATIVE do           CALL_NATIVE do
RET                      DROP            ← the call's result, discarded
                         PUSH_LOCAL l1   ← the raw capture slot returned instead
                         RET
```

**Cause.** `do` returns its body's value unchanged, so the result carrier IS
`j`'s carrier. `resolveOperand` opens with an override:

```go
if cur := es.units[len(es.units)-1]; cur != nil && cur.capID[v.ID] {
	if slot, ok := cur.localByID[v.ID]; ok {
		return localOperand(slot), true
	}
}
```

whose own comment gives its reason — the captured value "may carry a
producedBy entry from the ENCLOSING unit … but that event lives in the parent
frame and is unreachable from inside the body". Here the producing event is in
THIS unit's own frame, so the reason does not hold; the override sent the
residual back to the slot, the lowering dropped the call, and with it went the
word dispatch the body's deopt island had just performed.

**The change** is that guard, one predicate wide: `producedInCurrentUnit`
takes the capture slot only when the producing event is NOT this unit's own.

**Which floor is live depends on when resolution runs**, and the first attempt
missed it — worth recording, because it looked like the fix simply did not
work. While the body records, the unit's floor is the one `beginFragment`
pushed for its root frame (`es.fragFloors[rec.rootFrame]`); by the time the
`finish` callback resolves the body RESIDUAL that frame is closed and the
floor has moved onto `rec.frag.startSeq` — the same value the provenance
sweep a few lines below already compares against. The predicate reads
whichever is live.

**Measured**: `do [j]` and its paren twin `(do [j])` agree where they answered
`fn`; the plain-fn-body row, the `do [j typeof]` row and the plain-data twin
are unchanged. Every module suite is clean, which matters here because
`resolveOperand` is a hot, central function.

**A correction this increment owes itself.** It first shipped with
`( fn [[x:Integer][Any][do [j] typeof]] )` recorded as still open, explained
as "the `typeof` consumes the do's result as an OPERAND mid-recording — a
different resolution path the guard does not reach". That reading was wrong.
It IS the same guard; the guard's own index was off by one. `beginFragment`
appends to `es.frames` and `es.fragFloors` together, but `es.frames` opens
with a ROOT frame it never pushed a floor for, so frame n's floor is
`fragFloors[n-1]`. Indexing them as parallel put every recording-time
resolution out of range, so that arm silently declined and only the
finish-time `rec.frag` arm ever fired — which is exactly why the body-residual
forms were fixed and the operand form was not. With the index corrected
`do [j] typeof` answers `Integer` on both lanes.

The pin was complicit and is worth naming: `TestProducedInCurrentUnit` set
`fragFloors: []int{0, 5}` against `rootFrame: 1` and asserted index 1 — it was
written to match the implementation rather than the invariant, so it passed
over the bug and only failed once the code was right. It now states the
frames/floors relationship explicitly and covers `rootFrame == 0`, the root
frame that has no floor of its own.

**Still open**: NUR124 (a stack-shuffle word re-stepping a produced closure)
is untouched — `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]
[(mk 3)] each [5 swap]` is `[15]` interpreted and `[fn (Integer)]` compiled.

## The trailing apply's head carries its binding name (2026-09-06, the nineteenth increment, NUR122)

**The divergence.** The interpreter reads the head of `(g 5)` as a WORD —
`stepWord` routes a bound `FnDefInfo` through `Registry.Lookup` — so a no-match
names the binding, anchors the caret at the read, and explains the failing
overloads from the fn it found. The compiled apply holds a frame SLOT, which
carries none of that:

```
def app3 fn [[g:Function][Integer][(g 5)]]  app3 (z:String => [z])
    interpreted  cannot call `g` … --> 1:37 … candidate `g (String)` … help: group the call in parens …
    compiled     cannot call ``   … --> source position unknown
```

NUR122 had already located this and named the model — `callDynFrameWords`
carries a `DynFrameWord` name+position table for the whole-frame replay — so
this increment started from a boundary, not from scratch.

**What the measurement changed about the plan.** The record's disassembly
showed a lambda-body witness and inferred the fault was the lambda's. Five rows
said otherwise. The two-argument paren apply `(g 5 6)` AGREES, and so does the
no-paren `g x`: both lower to `CALL_DYN_FRAME`, which already has the table. So
the fault is not lambda-specific and not `CALL_DYN_TRAIL_TOP`-generic — it is
exactly the ops that reach `callDynTrailTop`. And the op has TWO lowering
routes, not one: the body-tail apply (`[(g 5)]`, seated in emit.go where the
whole residual collapses) and the event apply (`[1 add (g 5)]`,
`[def t (g 5) t]`, seated in lower.go where the result seats as an operand).
Fixing only the route the witness used would have left the other two shapes
diverging.

**The fix.** `CompiledFn.DynApplyName` maps the pc of an `OpCallDynTrailTop` /
`OpCallDynTrailKeepQ` to the binding name its head was read bare under and that
read's position, seated by both routes from the same `wordReadNames` /
`wordReadPos` tables NUR123's replay reads. `RecordDynApply` captures the pair
BEFORE it consumes the `pendingApply` entry — that consume is what tells a bare
read apart from an `apply`-word arrival, whose flavour
(`OpCallDynApplyTop`, a different dispatch) is deliberately not seated. The VM
builds the no-match through `NoMatchDiag` with the APPLIED fn rather than
`RuntimeNoMatch`'s registry lookup, since a frame binding is a slot on this
lane and `Registry.Lookup` finds nothing.

**One reconciliation the name alone did not buy.** A boru fn authored without a
`|` boundary carries `BarrierPos == BarrierAllForward` (-1). Registration
resolves that sentinel to the sig's arg count (`upsertFnDef`), and the
diagnostic reads the RESOLVED value through `HasForwardSigs` to decide the
"group the call in parens" help — so the interpreter printed that line and the
raw const, still carrying -1, did not. The tempting fix was to widen
`HasForwardSigs` to accept the sentinel, which is a no-op for every
registry-held caller. It was not taken: engine.go dispatches a raw fn VALUE at
the pointer through the same predicate, and that value can carry -1, so
widening it would move dispatch mode, not just a help line. `installedSigView`
resolves the sentinel for the diagnostic only. Same reconciliation
`closureAsWord` already makes for the replay's bridge, and for the same reason.

**Measured**: `(g 5)` is byte-identical on both lanes — message, code, caret,
both notes and the help — for a plain fn body, a lambda VALUE's body, an event
apply consumed by a native word and one consumed by a value-def. NUR122's two
lambda witnesses agree on name and position (`g` at 1:64; the capture row named
`add` at 1:80 before). The succeeding twins are unchanged, as they always
agreed.

**The boundary, and why it was recorded rather than guessed.** Parity is
byte-for-byte when the argument is a LITERAL or paren-computed (`(g 5)`,
`(g (1 add 1))`) and stops at the NOTES when it is a WORD (`(g y)`, `(g j)`):
the pointer substitutes a word onto the value stack before the head dispatches,
so the interpreter's forward window consumed no token and reports `takes 1
argument, but none were supplied` where the compiled lane names the value it
applied. This is the same remainder the attempted twelfth increment measured —
the replay islands values, the interpreter steps tokens. Four rows suggested a
discriminator (operand kind: const and event are written, local is not), which
is exactly the kind of rule this line has been wrong about before, so it is
recorded with its witnesses and left to its own increment. Both lanes raise the
same signature_error under the same name at the same place, and the succeeding
twin agrees exactly, so the residue is note-only.

**Two corrections cover-gate made to this increment.** Both were mine, and
neither showed up in `make test`.

The seat was written for BOTH lowering routes on the reasoning that either
could carry a bare-read head. It cannot: every body-tail `dynTrail` the corpus
reaches carries the `apply` flavour, so the emit.go arm was dead code from the
moment it was written — the paren body-tail form it was written for either
refuses or lowers through the EVENT route, which is what actually seats the
name for `(g 5)`. The parity rows had been passing through lower.go all along.
The seat, its `rec.dynTrailName` field and its capture are gone, with the
measurement recorded at the site. Any per-op discrimination at that site has
the same problem, so there is no narrower version of it to keep: the honest
form is not to write it.

The other correction is the one worth generalising. Adding a NAMED arm to
`noMatchIfSigged` made its existing NAMELESS arm unreachable — every no-match
the corpus reaches now carries a name. A new branch can strand an old one, and
a 100% floor is what catches that; a suite that only asks "did anything break"
never will. The fix was not to delete the correct fallback but to find the
shape that genuinely reaches it — a `/v` delivery head, a value delivery rather
than the word dispatch the interpreter names — and pin it. That witness turned
out to carry a pre-existing divergence of its own, byte-identical on `120fb37`,
now recorded as NUR124's fifth witness: the compiled lane APPLIES a /v-parked
fn where the interpreter leaves it as data, because `callDynTrailTop` strips
the applied copy's quote to mirror a read-substituted arrival. The pin asserts
only the nameless diagnostic and that the interpreter still parks, so it fails
loudly if either half moves.

**Still open**: NUR122's position-only row (`f ([] => [42]) 5`, interp 2:1 vs
compiled 1:43) and the written tuple above; NUR124's discriminator and its new
fifth witness; NUR119.

## The written tuple is a PREFIX, and the nineteenth increment's rule was wrong (2026-09-07, the twentieth increment, NUR122)

A measurement increment. Nothing is fixed here; what changes is the recorded
rule, which the next attempt would otherwise have built on. Everything below
is pre-existing, byte-identical on `120fb37`.

**What the nineteenth increment recorded.** Parity stops at the notes when the
argument is a WORD, and three rows suggested the discriminator was operand
kind: `opConst` and `opEvent` written, `opLocal` not. That was recorded rather
than encoded precisely because three rows are not a rule — and the instinct
was right, because it is wrong three separate ways.

**(1) It is a PREFIX, not a filter.** Forward collection walks the tokens after
the head left to right and stops at the first WORD read, which the pointer had
already substituted onto the value stack. Over a 3-param callee:

```
(g 5 6 y)   written [5 6]
(g 5 y 6)   written [5]      <- decisive: a filter would say [5 6]
(g y 5 6)   written []
```

`(g 5 y 6)` is the row that separates the two readings, and it cannot exist in
the one-argument family: over one argument a prefix and a filter are the same
function.

**(2) It spans TWO ops.** Those multi-argument rows lower to `OpCallDynFrame`,
the whole-frame replay — not `OpCallDynTrailTop`, which multi-argument applies
never reach. The replay's own no-match hands its entire arg window to
`NoMatchDiag`, exactly the fault the trailing apply has. So a fix scoped to the
op the nineteenth increment touched would have closed half the family.

The reason the first sweep missed all of this is worth recording, because it
looked like evidence: every two-parameter row it tried was spelled
`(z:String w:String => …)`, which parses as a **Map**, not a lambda. All seven
died at check time with `got (Map)`, and the sweep read that as "multi-argument
applies do not reach this op" — a conclusion drawn from a syntax error. The
correct spelling is `([z:String w:String] => …)`. A row that dies at check time
is not a row about runtime behaviour, and this line has now been fooled by that
twice in two days (the other was `b5`, a check-time row read as a
counterexample to the rule it was silent about).

**(3) The discriminator is SYNTACTIC, not an operand kind.** A body-local bound
to a literal folds its read to `PUSH_CONST` — byte-identical to the operand a
written literal produces — and is still not written:

```
def f fn [[g:Function][Integer][def y 7  (g y)]]   PUSH_CONST 7, written []
def f fn [[g:Function][Integer][(g 5)]]            PUSH_CONST 5, written [5]
```

So the proposed operand-kind fix would have answered that row wrong while
looking right on every literal row it was derived from.

**What closing it actually needs — and a fourth correction, to this section's
own first draft.** It first read that the fix needed a signal neither lane has,
because `noteWordRead` gates on `IsFnTypedCarrier(v)` or a gradual carrier
admitting Function, and filed the next increment as a core/go kernel change.
That was wrong, and reading one line further would have shown it: both engine
bare-read sites (engine.go ~2636 and ~2782) call `NoteLocalRead(id, pos)`
UNCONDITIONALLY, right beside the gated `noteWordRead`, recording every bare
read's value ID on the innermost open unit. Probed over this family it
discriminates exactly, and it gets the row operand kind could not:

```
(g 5)                          localReads 0   written
(g (1 add 1))                  localReads 0   written
(g y)          y a param       localReads 1   not written
def y 7  (g y) folded to const localReads 1   not written
def y (1 add 6)  (g y)         localReads 1   not written
```

`localReads` gets the folded row right precisely because it records the
syntactic fact — a bare read happened — rather than anything about the
lowering. So `noteWordRead`'s gate stays exactly as it is, NUR123's
fn-dispatch signal, and is NOT relaxed; the next increment is a compiler and
VM change (count the leading run at RecordDynApply, seat one integer, truncate
the args handed to NoMatchDiag at both diagnostic sites), not a kernel one.

The pattern is the same one this section already records twice: the first
reading of a gate's absence was drawn from where the search stopped, not from
what the code does.

**Pins**: `TestWrittenTuplePrefixRule` and
`TestWrittenTupleConstFoldedLocalDeclines` in
lang/go/dyn_apply_head_name_test.go, which fail if any of the three findings
moves — including the interpreter side, so a change in its collection
behaviour surfaces as a failing record rather than a silent drift.

## The written tuple is the leading run (2026-09-07, the twenty-first increment, NUR122)

The implementation half of the previous section's measurement, and it lands
where that section said it would rather than where the section before it did.

**The change.** `writtenRun` counts the leading arguments with no recorded bare
read; the recorder seats that count on the trailing apply's site
(`DynApplyHead.NWritten`) and marks each replay region entry
(`DynFrameWord.Read`); the two VM diagnostics — `noMatchIfSigged` and
`callDynFrameWords`' word-lead no-match — slice their arg window to it before
handing it to `NoMatchDiag`. All eight witnesses reach parity: three
`OpCallDynTrailTop` rows and five `OpCallDynFrame` rows, the multi-argument
prefixes included.

The signal is `fnUnitRec.localReads`, already populated ungated by
`NoteLocalRead` for every bare read. `noteWordRead`'s fn-admitting gate is
untouched: it answers NUR123's "does this read DISPATCH", which is a different
question from "was there a read at all", and relaxing it would have moved the
deopt machinery that reads it.

**What the first attempt broke, and how it surfaced.** Marking read-ness by
allocating the replay's word table whenever ANY entry was a bare read looked
harmless and was not: a nil table from `dynFrameWordsFor` MEANS "this window
carries no word read", and `noteWordReadReplay` / `replayValueApplicables` arm
the replay on that. Widening it armed replays that had no business arming and
turned three previously-compiling module rows into refusals
(`TestFnUnitLoopApplyFlowCrossesIsland`, `TestFnUnitLoopApplyValueCalleeDefers`,
`TestModuleReadNoRebindStillCompiles`). The marks now ride only on a table that
already exists for a NAME, in a second pass, and the nil contract is documented
at the site as load-bearing.

That is the third time in this line a datum added "for diagnostics only" turned
out to be read as a SIGNAL somewhere else. The pattern worth carrying: before
widening when a structure is allocated, find who tests it for emptiness.

**A `/v` read is a substitution too** (found by a review bot on #441). The
first cut consulted only `localReads`, so a value reference counted as
WRITTEN: `(g y/v)` named the argument where the interpreter reports `takes 1
argument, but none were supplied`. Pre-existing — identical on `f0d208c` — but
this increment would have carried it forward unclosed, which is the point: a
fix that closes eight witnesses can still be incomplete in a direction none of
them probes. `readSubstituted` now consults `valReads` as well.

Two things worth keeping from how it surfaced. The finding named the right
incompleteness with the DIRECTION REVERSED — it argued the interpreter counts
`/v` as written and that ID-aliasing makes the compiled lane under-count,
where measurement shows the opposite: `/v` is not written, and the lane
over-counted. Verifying rather than implementing from the description is what
produced the correct fix. And the two mixed shapes `(g y/v y)` and
`(g y y/v)` are parity under EITHER reading, because the bare read of the same
id stops the run whatever the `/v` occurrence does; only the lone `(g y/v)`
separates them. A family built from the mixed spellings alone would have
confirmed a wrong rule.

The ID-keyed lookup the finding worried about is sound here, and the record
says why: a WRITTEN occurrence is a literal or a paren-computed event, each
minted with its own fresh ID, so it never shares an ID with a binding read.

**Pins**: the three decline tests flip to parity —
`TestDynApplyHeadNameWrittenTuple`, `TestWrittenTuplePrefixRule`,
`TestWrittenTupleConstFoldedLocal`, plus `TestWrittenTupleValRefIsSubstituted`
for the `/v` half. Each asserts the TUPLE as well as parity,
because parity alone passes if both lanes drift together, and the prefix is the
whole point; the prefix-vs-filter row additionally asserts that the trailing
literal past the word does NOT appear.

## NUR124's discriminator: the apply word records as an identity (2026-09-07, the twenty-second increment)

The eighteenth increment found the site, wrote the fix, and withdrew it. This
one supplies the missing signal, and the useful part is where it was hiding.

**The bind.** `5 (mk 3)` answered 15 compiled where the park rule leaves
`[5 fn (Integer)]` — a silent wrong value on the default lane. Adding
`callResultPlaced` to `trailingApply` fixes it, but refuses
`10 (mk2 5) apply`, a corpus row that parks identically and then APPLIES
because the trailing word dispatches the parked value on purpose. Both lower
to the same `OpCallDynamicTrailing`, verified again here, so the residual
shape cannot separate them.

**Why the obvious signal was unavailable, and the record said so.**
`pendingApply` is UNIT-scoped and returns false outright when no fn unit is
open — exactly the program residual's case — so the previous increment
recorded that the discriminator "has to be program-level provenance for the
`apply` word", and stopped there rather than guessing.

**Where it actually lives.** `apply` returns the fn concrete in check mode so
the engine RE-STEPS it, and that re-step records through `RecordCall` as an
IDENTITY: `word == "apply"`, `args[0].ID == outs[0].ID`. That ID is the one
`trailingApply` later meets as `fnv`. So the provenance was already flowing
through the recorder; nothing had to be threaded, only noticed and kept
(`EmitState.appliedByWord`).

Two probes got there, and the first was wrong in a way worth recording. The
obvious hook is `recordCallElided`'s `apply` arm, which explicitly handles a
concrete `FnDefInfo` — and it never fires for this row, because the value
arrives as a carrier rather than an `FnDefInfo`. A probe at every `RecordCall`
found the real one immediately. Reading the arm that mentions your case is not
the same as measuring which arm runs.

**Measured**: `10 (mk2 5) apply` still compiles to 11; `5 (mk 3)` refuses
soundly through an EXISTING site (so the refusal-site census does not rise) and
the default lane answers correctly on the interpreter; `(mk 3) 5`,
`5 (mk 3) 7`, `1 2 (mk 3)` unchanged; corpus differential and refusal ceiling
pass — the gate that defeated the previous attempt.

**Still open in NUR124**: the SHUFFLE family, a different arm running the
opposite way — `[(mk 3)] each [5 swap]` is `[15]` interpreted and
`[fn (Integer)]` compiled, the lane parking where the interpreter applies.

## NUR124's shuffle family is two defects, and two rows agree for the wrong reason (2026-09-07, the twenty-third increment)

A measurement increment. Nothing is fixed; what changes is the recorded shape
of the remaining half, which the next attempt would have built on. Every row
is pre-existing.

**What the record said.** A produced CLOSURE put on top by a stack-shuffle
word is re-stepped by the interpreter and parked by the compiled lane —
one defect, about closures.

**Axis 1 is not about closures.** A plain `FnDefInfo` diverges too, the moment
anything follows the shuffle:

```
def g fn [[x:Integer][Integer][x mul 3]]  [g/v] each [5 swap drop]
    interpreted  each_error: body produced no result
    compiled     [5]
```

`[g] 5 → [g,5] swap → [5,g]`: the interpreter applies g THERE, giving [15],
and `drop` empties the stack. The compiled body leaves g, `drop` removes it,
body ends [5]. The interpreter re-steps AT THE SHUFFLE; the compiled body at
BODY END, if at all.

**Two rows agree for the wrong reason.** `[g/v] each [5 swap]` and
`[g/v] each [5 over]` pass on both lanes — only because nothing follows the
shuffle, so body-end and shuffle-time coincide. They are trap rows. Write the
`/v` out: the CLOSURE spellings of those same two bodies,
`[(mk 3)] each [5 swap]` and `[(mk 3)] each [5 over]`, are Axis 2
DIVERGENCES, so the body alone does not identify the row. A family built from
them would report this fixed, and the divergence would survive under a green
suite. This is the third time in four increments that the row which separates
two readings turned out to be the one the obvious family omits: `(g 5 y 6)`
for the written-tuple prefix, the lone `(g y/v)` for substitution, and now
`[5 swap drop]` for re-step timing.

**Axis 2 is.** `[(mk 3)] each [5 swap]` answers `[fn (Integer)]` where the
FnDefInfo twin answers [15] — same body, only the payload differs
(`ClosurePayload` against `FnDefInfo`). So even at body end a compiled closure
is not re-stepped.

**Where both come from, probed rather than reasoned.** `eachHandler` is the
SAME handler on both lanes: the island runs the same `each` word token through
the sub-engine and reaches the same `InvokeBody`. The handler is not the
divergence. What differs is what it is handed — the compiled body arrives as a
LIST VALUE whose elements happen to include Words (`[5, word(swap)]`), where
the interpreter's body is a CODE block off the tape.

An earlier note in this session put that distinction at the ISLAND and
concluded the record was wrong about the mechanism. Probing `runFallback`
showed the island preloads the recorded span TOKENS and steps them, so the
island was the wrong level; the body is the right one. The inference had been
carried over from NUR122's written-tuple work by analogy rather than measured
here — the same failure mode this document already records twice.

**For the implementing increment**: fix Axis 1 first and RE-MEASURE Axis 2 —
a shuffle-time re-step may subsume the closure case or may not, and assuming
either way is how the trap rows win. `[dup drop 5]` is the control that must
not move.

## The re-step deopt: a native's fn result dispatches where it lands (2026-09-07, the twenty-fourth increment, NUR124)

The twenty-third increment left Axis 1 as "fix first, then re-measure Axis
2". This one fixes it, and the useful part is that the family turned out to
be neither about shuffles nor about closures.

**What the rule actually is.** `spliceMatchResults` puts a native word's
results back onto the tape at the call's position and the main loop steps
them; an unquoted fn among them dispatches WHERE IT LANDS, collecting forward
over the tokens written after the call and then from the stack beneath it.
Measured on the interpreter with the shape's siblings before touching the
compiler (the rule this line pays for every time):

```
[g/v] each [5 swap 7]        [21]   g collects the written 7
[g/v] each [5 tuck]          [15]   results [5 g 5]: g collects the 5 after it
[g/v] each [5 6 rot]         [18]   nothing written: g takes the 6 beneath
[g/v] each [5 swap drop]     each_error — g(5) = 15, then drop
[5] each [{f: g/v} get "f" drop]     each_error — the same, from a map read
def m {f: g/v}  m get "f" drop 7     uncalled_function — g finds no argument
```

So "the interpreter re-steps AT the shuffle" (the record's Axis 1) is one
instance of "a native's fn-typed result is re-stepped into a dispatch". The
shuffle words are only where NUR124 first met it; a poly `get`, a typed
container read, any native returning a fn does the same.

**Why the compiled lane disagreed only sometimes.** The agreeing rows
(`[5 swap]`, `[5 swap 7]`, `[5 tuck]`) all compile the `each` as an ISLAND:
the closure probe declines them because the fn survives to the body's
residual (`closureResidualHasUnappliedFn`), and the island's interpreter
answers. `[5 swap drop]` compiles to a real unit — the fn is consumed inside
the body, so the residual guard sees nothing — and that unit's `CALL_NATIVE
swap; CALL_NATIVE drop` is the miscompile. The check pass never modelled the
dispatch because the element it stepped is a payload-less `Function`
carrier: `execFnDefLiteral` has no signatures to match and steps it past.

**What landed** (`core/go/engine.go`, `compiler/go/emit.go`, `lower.go`,
`bytecode.go`, `eng/go/vm.go`):

1. The pass NOTES the result at the call — `Engine.noteFnResultReSteps`,
   `EmitRecorder.NoteFnResultReStep(v, resume)` — for a fn-typed or
   fn-admitting gradual carrier, unquoted, when a PLAIN body token follows.
   Every exclusion was measured against a row: a concrete `FnDefInfo` (the
   pass dispatches it itself — `[[g/v] get 0]` compiles to `TAIL_CALL_USER
   g`); a user fn's result (parked, PAREN-RESTEP-RULE); a CODE-BODY word's
   results (`sig.Callable != nil` — `do [mk 7] add 1` would otherwise trade
   NUR121's refusal for this one, and `do [mk 7] 8` would stop compiling: the
   pass's carrier view of a body's residual is not the closure lowering's,
   which applies the lead at the body's finish); a pending forward's
   collection; and the non-plain followers — the group's close paren (the
   park rule's), a `/v` modifier, a marker, an already-placed value, the end
   of the tape (the residual arms').
2. The recorder keys the note on the PRODUCING event (`es.producedBy`) and,
   at the unit's finish, plans a RE-STEP point right after it
   (`planReStepDeopts`, a `deoptPoint` with `restep`), reusing NUR123's
   name-binding planning (`deoptDefsBindable`, `seedParentDeopt`,
   `lambdaNamesSelfBound`) and its deferred-operand accounting — with two
   corrections for a point that sits AFTER its event rather than before: the
   accounting runs from the RESUME token (`d.start = note.resume`), the
   event's own consumptions count as done, and its own results are exactly
   what the island re-steps (not deferred). Both were found by the debug
   print, not by reading: the first cut declined every point because the
   consumer-operand check written for pre-event points asked for `swap`'s
   already-consumed `5`.
3. The lowerer emits `OpDeoptIfFn` right after the event's op
   (`emitReStepAfter`) over the results on top of the sim, with
   `DeoptSpec.Results` = their count and `DeoptSpec.Prefix` = the unit's
   UNNAMED params it has not pushed yet. That prefix is the increment's
   second finding: the interpreter's frame holds unnamed inputs on its stack
   BOTTOM (named ones are bindings), the compiled unit holds them in slots
   and pushes them at their consumer, so an island resuming mid-body ran `g
   drop` over an empty region for `[5] each [{f: g/v} get "f" drop]` and
   raised `uncalled_function` for the interpreter's `each_error`. The prefix
   rides on NUR123's points too (`emitDeoptsBefore`, the atPush arm); a
   param pushed inside a nested fragment declines the point. A fn-TYPED note
   no point serves REFUSES through a lowerer decline — the refusal-site
   census counts `MarkUncompilable` sites and stays at 93 — while a gradual
   one keeps the model it always had.
4. The VM (`reStepIfFn`) tests the results with the main loop's own
   predicate (`core.FnValueDispatchesAtPointer`, exported for it) and, on a
   fn, runs `runIslandResolved` over `prefix ++ region` with tokens
   `[results…] ++ Body[Token:]`: a fn VALUE stepped at the pointer dispatches
   as it does on the interpreter, forward collection included. The residual
   replaces the frame region and the run loop continues at RET — NUR123's
   discipline unchanged.

**Measured.** Eleven witnesses agree and compile (`lang/go/restep_deopt_test.go`);
the trap rows and the concrete-element row are unchanged; the corpus
differential, the refusal ceiling and the frontier ledger pass. One golden
moved by design: `TestShuffleRestepTimingDeclines`'s Axis-1 arm now asserts
parity. `TestCancelTimeout` failed once under the parallel belt and passed
three times alone — the timer class the process rules already name.

**Three things this increment measured and did NOT fix**, each pinned as
measured in `TestReStepDeoptOpenShapes` so the next author starts from the
boundary:

- **Axis 2 stands, and its site is now exact.** `[(mk 3)] each [5 swap
  drop]` islands (the produced closure makes the list dynamic); the island's
  sub-engine re-steps the element after `swap` exactly as the interpreter
  would, and a `ClosurePayload` is not an `FnDefInfo`, so `execFnDefLiteral`
  steps it past. `closureAsWord` (vm_dyn_words.go) bridges a closure to a
  dispatchable `FnDefInfo` for the WORD path; the value path needs the same
  bridge at `execFnDefLiteral` — or the island's registry to install it.
- **A static-index FOLD.** `[5 h/v] get 1 drop 7` in a fn body:
  `tryFoldStaticIndex` returns the param's own carrier as the result, so
  there is no event and nothing is noted; the value re-stepped is a LOCAL's,
  which is NUR123's atPush shape with a value re-step instead of a word
  read. The map twin (`{a: h/v} get "a" drop 7`, a poly `get`) now deopts
  and raises — naming `g` where the interpreter's frame binding renamed the
  value `h` (NUR122's class).
- **The MAIN program.** No unit body to resume into: a gradual note there
  is dropped, as NUR123's declined points are (`def m {f: g/v}  m get "f"
  drop 7` answers 7), and a strict one refuses. `[g/v] fold [5 swap drop]
  0` is the same defect one level up — the fold ISLAND's strict `Any` result
  is re-stepped by the interpreter into `g 0`, and the note's condition
  (fn-typed or gradual) does not admit a strict `Any`; widening it to
  "admits Function" would plan a test after every `Any`-returning native in
  every unit, which wants the whole-corpus golden churn measured first.
  A main-level re-step is the rest-of-program island Stage 5's regions
  describe.

Two rules this increment adds to the ones above: **a green sibling is not
evidence the mechanism is right** — `[5 swap 7]` and `[5 tuck]` agreed
through an island, and the increment that "fixed" them by compiling the
unit would have broken them; and **a deopt planned before an event is not a
deopt planned after it** — every accounting NUR123 wrote assumes the island
starts before its consumer runs, and each of those assumptions had to be
found and flipped for a point that starts after.

## The closure value bridge: Axis 2, and a parked window the bridge exposed (2026-09-07, the twenty-fifth increment, NUR124)

Re-measured after the re-step deopt, Axis 2 was exactly where the
twenty-third increment left it: `[(mk 3)] each [5 swap]` answered `[fn
(Integer)]` for `[15]`, and the site was now exact — a produced closure is a
`ClosurePayload`, and `execFnDefLiteral` dispatches only an `FnDefInfo`.

**What landed.** The interpreter asks the VM piece for the fn a closure
stands in for: `CompiledRuntime.ClosureAsFnDef` (core's S4 seam, a third
method beside `InvokeCompiled` / `StampDetached`; the core default declines
with the value) builds — through the same `closureFnDef` builder NUR123's
`closureAsWord` now shares — one signature over the unit's declared param
contract whose handler applies the closure through `Registry.Invoker`, the
body-closure seam the running VM installs; a closure met outside any VM run,
or one whose program or unit the payload cannot name, stays data.
`execFnDefLiteral` (`fnDefAtPointer`) puts the bridged value in the
closure's place on the tape, so the dispatch rules below it — forward
collection, the stack match, the ADR-016 park of a 0-arg anonymous value —
decide exactly as for the interpreter's own fn. `CompiledFn.Lambda` carries
the source lambda's anonymity to the bridge for that last rule; without it
`[(mk0)] each [dup drop]` would fire the 0-arg closure where the interpreter
parks `[fn]`.

**Two things the first cut got wrong, both found by measuring, not
reading.** `compileFnDef` attached the boru body-runner to EVERY anonymous
sig, Go handler or not, so the bridge's empty body ran an empty frame that
returned its argument: `[5 swap]` came back `[5]`, `[5 swap 7]` `[7]` — a
plausible wrong value, not an error. It now keeps a `*GoImpl`. And the
re-step deopt's runtime test was the interpreter's own predicate, which does
not admit a closure — so `[(mk 3)] each [5 swap drop]` compiled to the unit
with its DEOPT_IF_FN and never fired; `core.FnValueDispatchesAtPointer`
admits an unquoted closure now that the island it hands the results to
dispatches one.

**The pre-existing miscompile the bridge exposed.**
`TestApplyWordClaimsParkedResult` pinned `5 (mk 3) 7` and `1 2 (mk 3)` as
agreeing; with the bridge they answered `5 21` and `1 6`. Their NAMED twins
— `def mkf fn [[k:Integer][Function][g/v]]  5 (mkf 3) 7` — answered `5 21`
on the ORIGINAL tree, measured on a build of the stashed main: the residual
window islands (`OpCallDynamicMixed`, the `mixedDynamicApplyShape` and
`trailingWindowApplyShape` arms) re-step their window verbatim, and a
PARKED user-fn result — placed data the interpreter never re-steps
(PAREN-RESTEP-RULE) — is applied live there. The closure twins agreed only
because the sub-engine could not dispatch a closure. Both arms now decline a
`callResultPlaced` lead; the residual then lays the parked pair out as plain
data, and all four rows agree — natively, no island. The member-read shapes
those arms exist for (`3 m.f 2`, `1 2 m.f`, family C's `m.p 5 m.p 7`) are
unaffected: a member read is no call result.

**Measured.** Eleven closure rows agree and compile
(`TestClosureValueReStepParity`), the 0-arg controls among them;
`TestShuffleRestepTimingDeclines`' Axis-2 arm asserts parity; the open
shapes pinned by `TestReStepDeoptOpenShapes` are two now (the static-index
fold, the main program). The seam's arms are pinned in core
(`engine_closure_bridge_test.go`, a stub runtime) and eng
(`closure_bridge_test.go`).

**What remains of NUR124** is the two shapes above and the fold-island
`Any` result at the main program — all main-level or fold-path, none in a
unit with a body. The next family in line is the handoff's frontier list.

**Two review findings on the bridge, both real, both fixed the same day
(Codex on PR #444).** The bridge minted a fresh `FnDefInfo` identity on
every call, so two tape copies of ONE closure bridged into two functions:
`[(mk 3)] each [dup eq]` answered `[false]` compiled for the interpreter's
`[true]`. A closure now carries the identity its push minted
(`ClosurePayload.Ident`, `core.NewFnIdentity`, one per construction as the
interpreter mints one per `fn`), `eq` compares it (closure to closure, and
closure to the bridged copy either way round), and the bridge's token
carries it (`core.NewFunctionIdentified`); two constructions stay two
functions (`[(mk 3) (mk 3)] fold [eq] 0` is false on both lanes). The
identity is a process-wide SEQUENCE, not a heap token: the first cut
allocated one per push and the compiled lane's alloc guard caught it the
same hour (`do_body` 312 → 412 allocations for a hundred `do body`
pushes, `each`/`fold`/`filter` one each) — a counter costs an atomic add,
and the token a bridge needs is minted at the bridge, where an allocation
is already the price of the dispatch. And the bridged
value used to REPLACE the closure on the tape, so a copy that parked (a
no-match, a 0-arg lambda) could escape into a binding as a Go-handled fn
closing over the finished run's `Registry.Invoker` — a stale `vmContext`
whose step counter every later application would keep consuming. The bridge
now stands in for the one dispatch only: `fnDefAtPointer` hands
`execFnDefLiteral` the bridged fn to decide and run that dispatch, and the
tape keeps the payload, so a park leaves the closure itself (pinned at the
seam: `engine_closure_bridge_test.go`'s park cases assert the payload, the
identity in `fn_identity_test.go`, `closure_bridge_test.go` and
`lang/go/closure_identity_test.go`).

## The closure-capture family's three blockers were one gate, and the frame binding's name (2026-09-07, the twenty-sixth increment)

With NUR124's two axes closed, the next family on the frontier list was the
closure-capture one — the three MEASURED blockers the 2026-09-05 section
names: (a) a bare-name apply of a captured Function with a forward arg
(`[g x]`), (b) a `/v` read of a captured fn returned as data (`[g/v]`), and
(c) an `Any`-typed inner param beside the capture (`[[x:Any][Any][(g x)]]`).
The attempted twelfth increment had already found that (a) and (c) are ONE
refusal — the closure COUNT check ("body value count differs from declared
returns"), which fires at the unit's finish before the whole-frame replay a
fn unit reaches. This increment takes the consequence.

**What landed.** A lambda VALUE unit (`tryReturnedClosure`'s `fnval` body)
takes the fn path's residual replay — `fnResidualReplayReason` — instead of
the closure count refusal, gated by `fnUnitRec.plainLambda`: the unit is a
lambda and no param carries a value PATTERN. Two facts make it a fn in
every way that matters at its finish: its count contract is enforced at
invoke (`checkClosureReturn` raises the interpreter's own `expected N return
value(s)`), and a bare read of a captured fn is the word dispatch NUR123's
replay seats (`OpCallDynFrame` re-steps the window under the binding's
name). The replay's one-applicable rule on the count-MISMATCH path is now
`replayLeadApplicables == 1`: beside a FN-TYPED lead, a GRADUAL WORD-READ
entry — Dynamic, not fn-typed, read bare under a frame name — does not
compete, because the replay re-steps it as the interpreter's own word
dispatch, faithful whether it holds a fn or data; so a gradual `x` beside
`g` no longer blocks the apply, and that is what closed (c). With no
fn-typed value in the window the count is the original one-applicable rule,
so a gradual body-local's read alone (`def j (m get "f")  j 3`, NUR123) is
still the lead. The first two cuts of that rule were both wrong and the
full lang suite caught each within the hour: the first discounted EVERY
word-read entry, so `f (g x y)` (two fn-typed reads, a paren that collapses
to no event) armed the replay, whose flat value re-step raised ``cannot
call `f` `` for the interpreter's 14, and `g x def q 9 g q` re-armed across
its bind; the second discounted a gradual read even when it was the ONLY
applicable, and the NUR123 body-local rows refused. A second fn-typed
value, word read or not, and a gradual EVENT result beside the lead
(`(g (x get "k"))`, whose runtime fn the value re-step would apply) decline
as before. For (b), a plain lambda whose whole residual is one `/v` read of
a captured fn is exempt from the unapplied-fn refusal: the read delivers
the value quoted, the return strips the quote, and the caller decides —
`((h 5) 2)` is 6, `(h 5) 2` the parked pair.

**The rename (b) needed.** `[g/v]` returned the captured fn as data and the
lanes then RENDERED it differently: `fn g(Integer)` interpreted, `fn
(Integer)` compiled. The interpreter's frame binding renames a fn value it
binds (`installDef`: `fnDef.Name = name` for a Function-family body); the
VM bound the caller's value verbatim. A plain fn body carried the same
divergence on the default lane before this increment — `def f fn
[[g:Function][Function][g/v]]  (f (z:Integer => [z])) 3` rendered `fn
(Integer) 3` for `fn g(Integer) 3`, a COMPILING row (NUR122's class). The
VM now names a fn bound for a NAMED param at every frame entry
(`nameFrameFns`: `bindUnitLocals`, CALL_USER, the tail call, OpCallUserPoly,
the dyn-apply entry) — only an `FnDefInfo` of THIS registry, the payload the
interpreter's rule names. A module wrapper (a foreign Registry) takes
installDef's REBINDING path, which installs the inner native's OVERLOADS
under the param's name; the VM does not mirror that, so the wrapper keeps
its own name and signatures on the compiled lane — `(f MathUtil.sqrt/v)
16.0` renders `fn sqrt(Number) 16.0` for the interpreter's `fn
g(BigDecimal) or (BigInteger) or (Float) or (Integer) 16.0`, pinned open as
measured (`TestClosureCaptureOpenShapes`); the wrapper still DISPATCHES
(`(g 16.0)` is 4.0 on both lanes). A compiled closure keeps its render. One
re-pin followed: `dyn_apply_head_name_test.go`'s nameless arm (`(g/v 5)`
over a String lambda) now reads ``cannot call `g` `` — the nameless builder
prints the applied fn's OWN name, which is the frame's now — where it read
``cannot call `` `` before.

**The gate the first cut needed.** Reordering the finish so a lambda unit
reaches the replay made the PATTERN-param lambda `def mk fn
[[x:Integer][Function][(fn [[0][Integer][x]])]]  ((mk 5) 1)` miscompile: the
replay applied the closure where the interpreter, whose frame binding
matches the pattern at the apply, parks the pair. The closure apply ops do
not enforce a value pattern, so `plainLambda` declines one and the count
refusal that was guarding it stays (`TestClosureCaptureSoundRefusals`); the
patterns are seated on the record at the unit's OPEN (`compileClosureBody`
takes `paramPatterns`), not only from the contract seated after the compile,
because the finish reads them.

**Measured.** Twelve closure-capture rows agree and compile
(`TestClosureCaptureParity`): the bare-name apply and its no-match twin, a
0-arg capture firing as the word, the gradual-param spelling with its two
no-match twins (a String for `x`, a String lambda for `g`), the `/v` read
rendering `fn g(Integer)` on both lanes, the plain-fn rename, a named fn
taking the param's name, and a module wrapper dispatching through `(g
16.0)`. Five neighbours are pinned as SOUND refusals: the returned `/v` fn
applied downstream (`((h 5) 2)`, `(h 5) 2`, `def q (h 5)  q 2`), a gradual
arg that turns out to be a fn (the interpreter's strict-barrier error), a
gradual event argument, and the pattern lambda. One off-frontier negative
graduated with the family: a CAPTURING sink fn handed to `Log.register`
(`bytecode_fnvalue_m2_test.go`) compiles as a closure unit now and rides as
a closure operand the non-strict store word invokes through the compiled
runtime — the sink fires with its captured `p` on both lanes, in this run and
a later one. The frontier ledger went 82 → 78: the §1.6 compose row,
the §9 `mk0` 0-arg row (the fnval unit models the raise), and both §9d
gradual-param spellings graduated to `bytecode-migrated.tsv`. NUR122's open
list re-measured with the rename in place: `f ([] => [42]) 5` is still the
POSITION-only row (interp 1:50, compiled 1:43); the fold twin NUR124
recorded (`{a: h/v} get "a" drop 7`) now raises `call to 'h'` on both lanes
with only its position differing (interp 1:75 at the read, compiled 1:100 at
the call); and a DEF-bound rename (`def k g/v  k 2` → ``cannot call `k` ``)
agrees because the compiled lane falls back there.

**What the next author should not re-derive.** The count check and the
replay are not two gates on one unit but two paths, and a lambda VALUE unit
belongs on the fn path unless its params carry something the closure ops do
not enforce; a pattern is the one such thing today. And a rename at frame
entry is cheap but partial by design: the interpreter's rebinding path for a
module wrapper installs OVERLOADS, and mirroring that is a registry-visible
install, not a payload edit.

## `apply` over a gradual lead (2026-09-07, the twenty-seventh increment)

With the closure-capture family moved, the §5.8 combinator rows were
re-measured on both lanes, and the ten still refusing all blocked on ONE
mechanism inside their returned lambdas, not on the lambdas themselves:
`apply` over a value the check cannot type as a fn. Three spellings, one
cause: `x (x f/v apply) apply` (the W combinator's body — a fn-typed
carrier's own paren apply feeds a second `apply`), `nd (m get "inc") apply`
(the ledger's "apply over a dynamic lead" pair, a rule fetched from a Map
param), and the same inside a paren. Plain fn bodies with these shapes were
the smallest witnesses, because a returned lambda's probe refusal surfaces
only as the factory's "body result of unknown provenance".

**Where it blocked, measured.** A fn-typed CARRIER's paren apply netted a
STRICT `Any` carrier (`NewCarrier(TAny)` at the paren collapse), so a later
`apply` over it matched no overload in check mode and took the checker's
best-fit recovery — "unmatched dispatch recovered at apply" — a refusal.
And a GRADUAL lead that does match matches `apply`'s [Reach Any] overload,
not [Function]: CompareSignatures tries the longer overload first, and a
Dynamic value is admitted to the Reach slot because the matcher cannot rule
a lens out. The recorder's apply arm expected the [Function] match (the
fn-typed carrier case) and refused the rest as "apply over a dynamic lead
(overload unprovable)".

**What landed.**

- The paren collapse mints a GRADUAL result for a lead the apply WORD owns
  — a gradual value, or a fn-typed carrier under the word, `(x f/v apply)`:
  `last.Dynamic || es.ApplyPending(last.ID)` → `out.Dynamic`. The honest
  type — a later dispatch over it matches gradually and records a runtime
  re-match, as a map get's result already does. A plain paren apply of a
  fn-typed carrier (`(1 2 c)`) keeps its strict `Any`: the first cut made
  every carrier lead's result gradual and an each body's residual refused
  ("result above a literal", `TestTrailingApplyQuoteDiscipline`) — the
  comparator convention's consumers were built on the strict result.
- `apply`'s [Reach Any] overload needed NO check-mode model: its declared
  `Any` return already rides gradual (`declaredReturnCarriers`, "declared
  Any riding dynamic"). A ReturnsFn returning a strict `Any` for a concrete
  lens was tried first and refused `p (reach 0 [x (k)]) apply` — a
  compiling row — because the strict result took the dispatch off the poly
  path onto the generic record and its dynamic-lead refusal; the lang suite
  caught it (`TestReachComputedSegmentLowers`) and the model came out.
- The recorder records the Reach-matched dispatch over a gradual lead as an
  apply EVENT (`recordGradualApplyEvent`): [receiver, lead] with the lead on
  top, one result, the apply word's position, lowered to a new op
  `OpCallDynApplyOne`. Inside a unit only — the program residual has no
  single-consumer window, so the main program keeps its refusal — and never
  for a concrete or unidentified lead, a fn-value receiver, or an operand the
  recorder cannot place. A gradual lead ALONE on the stack matches
  [Function] instead and keeps the standing refusal (nothing beneath it to
  apply to, and no single-consumer window at the unit's finish) — a first
  cut registered it as the fn-typed carrier's pending apply, dead code no
  program reaches, which the merged coverage gate caught. The pending entry
  now carries the apply WORD's position (`pendingApply{id, pos}`), which
  both the tail lowering and the paren event stamp on the op, so a runtime
  no-match raises where the interpreter's `apply` does.
- `RecordDynApply` admits a lead that is not a fn-value residual when the
  apply word holds it pending, and the paren classification asks the
  recorder (`EmitRecorder.ApplyPending`, a new seam method) so a Dynamic last
  value the apply word owns is the trailing apply, not the leading-dynamic
  reorder hazard `recordParenLeadingApply` refuses; the apply word's unquote
  lets a `/v`-read lead apply there too.
- The VM's apply-word op applies a fn as the interpreter's `applyHandler`
  re-step does (`applyReStep`): the args are the resolved STACK and the fn
  is stepped over them, through `core.MarkApplied` (moved to core so the
  runtime handler, the check model and the VM share the one decision), so a
  0-arg fn fires and leaves the receiver beneath its result — the earlier
  island stepped the fn first with the args as forward tokens, which put a
  0-arg fn's result BENEATH the receiver ([42 4] for the interpreter's
  [4 42]). The event form commits exactly ONE result: any other count (a
  0-arg or 2-arg fn), a lens on top (the [Reach Any] overload's own get) and
  a compiled closure of another arity DEFER the run to the interpreter; a
  value that is no fn raises the interpreter's own `apply` no-match over the
  same two stack values, byte for byte and at the same position (`w15 {f:
  42} 4` → ``cannot call `apply` `` at 1:114 on both lanes, with the two
  candidate notes).

**Measured.** The W combinator's body as a fn (`x/v (x f/v apply) apply`)
and the same in a paren answer 8 on both lanes, natively; the fetched-fn
apply is 6 and its negative twin the identical no-match; the W combinator's
RETURNED lambda compiles and, def-bound, applies (`def w4 (ww add2/v)  (w4
4)` → 8 natively). The lens, the 0-arg fn and the 2-arg fn defer and answer
through the fallback with the interpreter's exact value or error
(`TestGradualApplyDefers`). The frontier ledger went 78 → 77: the "apply
over a dynamic lead" pair's NEGATIVE twin graduated to
`bytecode-migrated.tsv` (it raises the no-match natively; the plain check
flags that no-match at the call over the concrete rule map where the
compiled unit's carrier analysis does not, so the diagnostic-parity ceiling
takes it, 320 → 321, the "lost under compilation" class), while the positive
twin compiles but ISLANDS: the map-member lambda `rules.inc` is baked as a
const with no compiled unit, so the apply word's re-step runs its body on
the interpreter (`vm:island-resolved`) — the interp-entry census's debt,
which graduating the row would have moved into the main corpus (the census
caught it, 34 over its ceiling of 33). It stays ledgered as "islanded", and
the frontier gate now detects a VM island through the interp-entry hook
(the `vm:island` seams — a native handler's own interpretation stays the
handler's, as for the census), not only an OpFallback span, so such a row
is never reported as a stale entry to graduate. Graduation = stamping a fn VALUE on
its first apply (`StampDetachedFn` from the op) so the unit enters
VM-native. (Graduated in the twenty-eighth increment — and the diagnosis
was wrong: the value WAS stamped; the apply op's event form declined the
entry on the unit's own empty return contract instead of the applied
value's.) And TEN §5.8 rows moved their refusal: the combinator's returned
lambda compiles now, so each refuses at the MAIN program's `apply` over the
produced closure — the Stage-2 argument-slot refusal the staged B/K rows
already hold, re-diagnosed in the ledger as such (the two `cand` rows did
NOT move: their inner lambda applies a def-bound lambda read by `/v`,
`cfalse/v (q/v p/v apply) apply`, the NUR101 def-read hold). That refusal —
the apply WORD over a produced closure at the main program (`99 (kk 7)
apply`, `4 (ww add2/v) apply`) — is now most of the §5.8 family's last
blocker: twelve rows behind one gate (`argIsProducedClosure` at the
top-level dispatch, plus the apply word's `len(units) > 1` hold), with the
closure's arity known at record time (`producerReturnedClosureArity`).

**What the next author should not re-derive.** Which `apply` overload the
check picks for a gradual lead is decided by CompareSignatures' order, not
by the value: the [Reach Any] match is the common case and the model the
recorder must lower; the [Function] match is the lone-value case, which
has nothing to lower. And the
apply word's re-step is STACK-shaped: an island that steps the fn first over
the args as forward tokens is a different dispatch, and a 0-arg fn is where
it shows.

## `apply` over a produced closure at the main program (2026-09-08, the twenty-eighth increment)

The twenty-seventh increment left twelve §5.8 rows behind one gate: the
main program's `apply` over a closure the pass PRODUCED — a factory call
whose unit pushes a closure (`99 (kk 7) apply`, `4 (ww add2/v) apply`).
The refusal was `argIsProducedClosure`, the word-agnostic guard that reads
a produced closure at a word's argument slot as a paren that failed to
collapse (`typeof (h 5)` taking the FUNCTION). Under `apply` that reading
is wrong: the closure at the slot is exactly what the word applies.

**Where it blocked, measured.** Two things had to hold at once. (1) apply's
own dispatch: the guard fired before anything modelled the word. (2) The
re-step: applyHandler hands the fn back and the engine steps it over the
values beneath; in check mode that dispatch runs the value's ReturnsFn
(`BuildFnBodyReturnsFn`, built at the lambda's construction with no name)
and lands in `RecordUserCall`, which resolves the unit's captures at the
call site — and a produced closure's captures are the factory body's own
carriers, unreachable there ("capture x of  unreachable at a call site").
The §4.3 fallback (`recordFnValueApplyFallback`) rescues a DEF-BOUND value
through its name's def-site operand; a nameless value had no route. The
same runtime value, though, is the closure PAYLOAD the producer event
leaves on the VM stack, captures and all — the operand the fn-value apply
(`RecordDynApply`) already lowers for the paren-bounded twin `(99 (kk 7))`.

**What landed.**

- `argIsProducedClosure` exempts the apply word's one-arg overload over a
  CONCRETE closure value. A fn-typed CARRIER — a declared `[Function]`
  return — keeps the refusal: the check engine cannot re-step a carrier, so
  nothing models the apply at the program level, and lifted for carriers
  too `1 99 (mk 7) apply` seated all three as data (measured). The two-arg
  Reach overload keeps it as well: a closure at its receiver slot is data.
- `recordCallElided` registers the apply as a PENDING application on the
  open unit — the program unit included — under the value's id, carrying
  the value (`pendingApply.fn`), resolved BEFORE the registered-output arm
  that would otherwise elide apply's identity result silently.
  `producedFnValue` admits a closure a unit returns
  (`producerReturnedClosure`) and the result of a compiled fn-value apply
  (the staged combinators' inner apply nets the next closure).
- The re-step's record site (check's `recordUserCallOrApply`) asks the
  recorder for the pending value by the sig BODY's backing array
  (`EmitRecorder.PendingClosureApply`, a new seam method; a position would
  not do — a body whose one token is a `=>` group carries none) and records
  through `RecordDynApply` over it: the fn resolves to the closure's
  producer operand, the window is the sig-ordered args reversed (sig[0] on
  top), the out is FRESHENED as the §4.3 fallback freshens — the memoised
  residual is shared across calls of one shape, and `99 (kk 7) apply 1 (kk
  8) apply` is [99 8], not [8 8] — and a fn VALUE result keeps its payload
  under a fresh id so the next `apply` over it records the same way. The
  lowering is the apply word's unquoting op at the apply word's position
  (`OpCallDynApplyTop`).
- `RecordDynApply` admits a fn-valued ARG under a produced-closure pending
  apply: the re-step matched those args against the closure's own
  signature (`kk/v (ss kk/v) apply` binds kk to `g:Function`), so they are
  data to the op exactly as to the interpreter; every other window keeps
  declining a fn-valued arg.
- `Finalize` refuses a pending application on the program unit that no
  dispatch consumed — nothing beneath the closure, or values that match no
  signature, where the interpreter leaves the fn as data (`(kk 7) apply`,
  `"s" (kk 7) apply`); a fn unit's finish already owned its own entries.
  No new `MarkUncompilable` site (the census stays at 93): Finalize returns
  the reason.
- A tape-side arm in `spliceAnonCheckResult` was written first and
  measured unreachable — every lambda dispatch here runs its ReturnsFn, and
  that path is the legacy stack-match fallback — and came out again.
- The VM's apply-word op (`callDynApply`) ran a compiled closure VM-native
  only when the unit's `NParams` equalled the window — and `NParams` counts
  the trailing CAPTURE slots too, so every capturing closure took the
  interpreter's re-step island instead (`vm:island-resolved`). The seven
  graduated rows all islanded, and the interp-entry census caught it (40
  over its ceiling of 33) where the value parity could not. The arity is
  `NParams - NCaptures`; the increment-27 W-combinator rows, which had
  islanded the same way since they landed, run native now too.
- The same op's EVENT form entered a stamped fn-value unit only when the
  UNIT declared the one return the model committed — and a stamped lambda
  unit declares no returns of its own (`compileStoredFnUnit` passes none),
  so every const lambda the apply word met, stamped and ready, islanded.
  The reading was the wrong one: the contract the frame's RET enforces is
  the APPLIED VALUE's (`applyRetContract`), which `dynMethodClaimOK`
  already reads for the method op. Church false's inner lambda `f:Any =>
  [f/v]` and the fetched-fn positive twin's `rules.inc` were STAMPED all
  along (the disassembly shows their `storedfn$body` units); the
  increment-27 diagnosis "never stamped" was wrong, and the entry gate
  was the island.

**Measured.** Eight rows graduated (ledger 77 → 69), every one VM-native
with no interpreter entry: K (`99 (kk 7) apply` → 7), W, C, I = S K K (42
and 'hello'), Church true and false, and the fetched-fn apply's positive
twin (`app 5 rules` → 6), the twenty-seventh increment's "islanded" row.
The interp-entry census holds at its ceiling of 33 rows with the two fixes
in — its `vm:island-resolved` entries fell 24 → 8 across the corpus. `1 99 (kk 7)
apply` is [1 7]; a token after the word binds as the re-step binds it (`99
(kk 7) apply 5` → [99 7]); the shape compiles inside a fn unit and a lambda
unit, and over an inline `fn` literal closure. Re-diagnosed: the two B rows
— the x-level closure's call-site residual is TWO values (the gradual `(x
g/v apply)` result and the fn-typed capture its tail `apply` consumes at
the unit's finish), so the single-out record site declines and the unit
call refuses the capture; the Church pair rows — the projection's body puts
a lambda VALUE beneath a fn-typed param's pending apply, which the unit
finish's whole-residual lowering declines; the `cnot` rows — `(cif (cnot
ctrue/v))` hands a produced closure to a user fn's Function slot, the
argument-slot family proper. Sound refusals pinned: no arguments beneath, a
no-match beneath, the carrier lead, a produced closure over another (the
second dispatches over the first before apply runs), a two-arg closure over
two literals (the seating cannot reorder).

**What the next author should not re-derive.** The check-mode dispatch of
a nameless lambda value runs its sig's ReturnsFn and records at
`RecordUserCall`, not at `spliceAnonCheckResult`. And a pending entry keyed
by the value's id serves apply's own dispatch, but the re-step's record
site sees only the body: match constructions by the sig body's array,
never by position.

## A unit's tail apply collapses into its call-site residual, and a returned lambda's count contract (2026-09-08, the twenty-ninth increment)

The twenty-eighth increment left the two B-combinator rows one step short:
the main program's `apply` over the produced closure recorded, but the
x-level closure's dispatch refused "capture f of  unreachable at a call
site". Its body `[(x g/v apply) f/v apply]` ends in the apply word over a
fn-typed CAPTURE, which the check engine elides — the identity result flows
to the residual — so the call site's analysed residual held [result, f],
TWO values, where the unit's finish had lowered the window as the
whole-residual OpCallDynApplyTop and nets ONE. The single-out record site
declined and the unit call refused the construction-scope capture.

**What landed.**

- `EmitRecorder.UnitTailApply` (a new seam method) reports the window width
  a unit's finish lowered as the fn-value apply at its body tail
  (`fnUnitRec.dynTrailArity`), and the check pass's user-fn ReturnsFn
  collapses the call-site residual the same way (`collapseTailApply`): the
  WHOLE residual must be the window — a lambda leaving a value beneath the
  applied result is the interpreter's count error at the RET, which a
  collapsed two-value residual would hide — and it becomes one GRADUAL
  result.
- The PLAIN check pass — no recorder, no unit to ask — models the same
  elided tail apply as leaving the carrier, and the two graduated rows
  tripped the type-soundness gate the moment they entered the main corpus:
  the program's checked types were [Integer Function] for an actual
  [Integer]. `collapseElidedTailApply` is the recorder-free twin: a body
  whose last token is the `apply` word and whose residual ends in a
  fn-typed carrier nets one value at run time (every other count is the
  interpreter's own return error), so the residual collapses to one
  gradual result — only where one value is the contract (a lambda, a
  declared single return); a declared tuple keeps its residual, since the
  applied fn may under-apply into exactly that count.
- The collapse exposed a latent gap: a RETURNED lambda's closure carried no
  return contract (`tryReturnedClosure`'s operand had no `closureRet`), so
  `checkClosureReturn` never enforced the lambda's count contract, and a
  body that under-applies at its tail (`[7 x f/v apply]` over a 1-arg f)
  answered [7 8] where the interpreter raises `expected 1 return value(s),
  got 2`. Latent because every such call site refused. The operand now
  carries `fnValueRetSpec`'s contract, and the unbound form raises byte for
  byte (`4 (bb2 …) apply` → `: expected 1 return value(s), got 2 — [7 8]`
  at the apply word on both lanes).
- The DEF-BOUND form (`def h (bb2 …)  (h 4)`) named the error `h:` on the
  interpreter — installDef renames a fn value bound by `def` — and `:`
  compiled: the stored payload had no name. The compiled lane now renames
  where the interpreter does: `RecordDynBind` notes the def name by the
  produced fn value's producing slot (`EmitState.defNameAt`), the lowerer
  seats it on the value's promoted STORE_LOCAL (`Program.StoreNames` /
  `CompiledFn.StoreNames`, keyed by pc — `seatStoreName`), and the VM's
  store op renames the ClosurePayload it stores (`nameStoredClosure`:
  `RetName` for its diagnostics, `Render` through the closure bridge for
  its `fn h(Integer)` render; the first name wins, as the frame rename's
  does). The message agrees; the POSITION does not (interp 1:100 at `h`,
  compiled 1:102 at the argument — the call event's position is its first
  operand's), NUR122's open position-only class, pinned as measured.
- The compiled error's CARET was one character wide where the
  interpreter underlines the whole `apply` token: `stampAt` copied the
  debug entry's row and column but not its token text (`SrcPos.Src`). It
  copies the text now when the error carries none.

**Measured.** The two B rows graduated (ledger 69 → 67): 14 on both lanes,
VM-native; `(h 4) add 1` over a closure whose tail applies its capture is 9;
two calls give two results; the under-applying tail raises the count error
on both lanes, byte for byte unbound and message-identical def-bound. Sound
refusals pinned: a gradual param beneath the tail apply (the factory's
"body result of unknown provenance").

## A produced closure at a declared Function slot, and the memo key of a position-less lambda body (2026-09-08, the thirtieth increment)

The two `cnot` rows refused at cif's `p:Function` ARGUMENT slot
(`cif (cnot ctrue/v)`): `argIsProducedClosure`, the word-agnostic guard
that reads a produced closure at a word's slot as a paren that failed to
collapse, held it there. That reading protects `typeof (h 5)` — an `Any`
slot would swallow the FUNCTION — but a slot DECLARED `Function` asked for a
fn value: the interpreter's frame binding installs the closure as data.

**What landed.**

- `argIsProducedClosure` takes the matched signature and exempts a
  produced closure at a slot declared exactly `Function` — a named param
  (`Params[i].Type`) or a positional one (`Args[i]`) — for every word but
  `apply`, whose own Function slot APPLIES its value and whose account the
  twenty-eighth increment already gave (a fn-typed carrier there keeps the
  refusal). The call's operand carries the payload; inside the callee the
  param's apply ops take it as any fn value.
- Lifting the hold exposed a LATENT miscompile in the fn-analysis memo:
  `FnAnalysisKey` identified a body by its first token's position, and a
  lambda body that is one `=>` group carries none — so cnot's inner
  `t:Any => [f:Any => …]` and cif's inner `t:Any => [e:Any => …]`, both a
  `p:Function` capture over `t:Any` under the synthetic `fnval$body` name,
  shared one key, one FnSummaries entry and one compiled UNIT. cif's
  returned closure ran cnot's body: `'F' ('T' (cif (cnot ctrue/v)) apply)
  apply` answered T for the interpreter's F, and the shape with cif's body
  changed to `[(t/v p/v apply)]` answered T for the interpreter's `fn
  (Any)`. Such a body now keys on its canonical text (`core.CanonValue`
  per token) where the position is zero. The main corpus never held two
  such lambdas of one shape, which is why no gate had caught it; the
  fn-quota key keys the same way and only groups budgets, so it stays.
- A compiled closure bound for a NAMED param takes the param's name at
  frame entry as a fn value does: `nameFrameFns` names a `ClosurePayload`
  slot through `nameClosureValue`, the rename the twenty-ninth increment
  built for def-bound closures (program-free now: the payload names its
  own program), so `(w (kk 7)) 4` renders `fn g(Any) 4` on both lanes where
  the compiled lane rendered `fn (Any) 4`. A capture slot is left alone,
  as before.

**Measured.** The two `cnot` rows graduated (ledger 67 → 65), agreeing
byte for byte and VM-native; a produced closure at a plain fn's Function
param applied in the body (7), paren-bounded, held as data (`typeof g/v`
→ Function) and rendered agree. Sound refusals pinned: the `typeof (h 5)`
Any slot and apply's Function slot over a fn-typed carrier.

**What the next author should not re-derive.** A body's first token
position is not an identity: the parser gives a `=>` group none, and every
lambda whose body is one such group looks alike to a position key. Any
memo that must tell two bodies apart needs the text (or the backing array,
as `PendingClosureApply` uses) when the position is zero.

## A lambda value beneath a unit's tail apply, and the `/v` read of a def-bound produced closure (2026-09-08, the thirty-first increment)

The two Church pair rows stood behind two gates, one in the projection's
unit and one at the main program.

Inside `cfst` (`(a:Any => [b:Any => [a/v]]) p/v apply`) a lambda VALUE
sat beneath the fn-typed param's pending tail apply, and the unit finish's
whole-residual lowering declined a fn-valued window entry (`argsOK`). The
reading was wrong for the `apply` word: applyHandler re-steps the fn over
the RESOLVED stack, where a parked lambda is data the callee's Function
param binds — the projection is the pair's argument, not a second apply —
and `OpCallDynApplyTop`'s window binds it the same way.

At the main program `(cfst p/v)` refused "fn call operand of unknown
provenance": a `/v` read of a fn binding is a FRESH WRAP of the binding
(`ResolveRef` mints a new Value over the dispatch aggregate on every read),
so the read's ID had no producer — and the def's own value (`def p (2
(cpair 1) apply)`) had been promoted to a store the read could not name.

**What landed.**

- The unit finish's pending-apply arm takes every value beneath the
  pending fn as the window, fn-valued or not (the paren arm's fn-value
  exclusion stays; unreachable until the thirty-second increment).
- `NoteValRead` carries the binding NAME (a seam signature change). The
  binding unit remembers a dyn-bind of a PRODUCED fn value by name
  (`noteValBind`: the bind-time PRODUCER, the binding's generation, the
  installed entry's identity and a per-name bind epoch), and a `/v` read
  of the name is aliased to that producer (`aliasValRead`) while the
  binding is still the bind's: no dyn-bind of the name recorded since, in
  any unit, and the generation unchanged — or the bind's entry on top
  again after a frame binding of the name came and went (`(cfst p/v)`
  binds cfst's own `p` param). Any bind of the name in the unit drops the
  entry; one inside a branch or loop body leaves it dropped. The aliased
  read is data at an argument slot (`argIsProducedClosure` skips it:
  `typeof p/v` is Function on both lanes).
- Three faults the alias exposed, each measured before its fix. The
  producer is taken at the BIND, not by the value's ID at the read: a
  memoised body returns ONE residual value for every call of one shape,
  so `def p (kk 7) end def q (kk 8) end 3 p/v apply 4 q/v apply` read q's
  closure for p (`3 20` for `3 19`). The re-step of such a read takes the
  pending closure apply BEFORE the name fallback (`recordUserCallOrApply`):
  with the fallback first the pending entry was never consumed and
  `3 p/v apply` refused at Finalize; the pending route also runs the
  closure VM-native where the fallback's name lookup islands. And a
  binding renames a NAMED closure too (`nameClosureValue`): `(w p/v)`
  rendered `fn p(Integer)` for the interpreter's `fn f(Integer)`, since
  installDef renames whatever it binds; the copy in the slot is renamed,
  the stored value keeps its own name.
- Found on the way, PRE-EXISTING on the default lane and now a sound
  refusal at installDef's existing site: a fn body's def of a CAPTURING fn
  value over an outer overloading def outlives the call on the
  interpreter — the drop-then-push leaves the frame's def depth unchanged,
  so DefCleanup pops nothing — where the analysis restores its snapshot
  and the compiled program keeps the outer closure's bake: `def p (kk 7)
  end def g fn [[][Integer][def p (kk 9) 1]] end g (p 3)` answered `1 10`
  for the interpreter's `1 12`. A capture-free literal takes the compiled
  bind twin and agrees, so only the capturing class refuses (a capturing
  literal already refused at its reads). The bind epoch above is the same
  hazard seen from the read side: the entry on top again after the call is
  no proof the run holds it.

**Measured.** The two Church pair rows graduated and the nested replacing def is pinned in the ledger with the interpreter's answer (ledger 65 → 64),
agreeing byte for byte and VM-native, with two pairs read twice each and
the plain-fn twin; the def-read family agrees on the apply word, a rebind,
two closures of one source, the word and value spellings side by side, the
data slot, the callee-side apply and the frame render. Sound refusals
pinned: nothing beneath the read, a no-match beneath, a code body's read,
a read in another unit, a conditional rebind, and the nested replacing def
with the interpreter's answer.

**What the next author should not re-derive.** A `/v` read of a fn
binding never carries the binding's ID: trace it by NAME, and take the
producer at the bind. A binding's generation is a rebind detector that
frame bindings also trip; the installed entry's identity is not one that
survives a call whose body rebinds the name — hence both keys and the
epoch. The interpreter's frame cleanup pops by depth growth, so a
drop-then-push inside a frame is a permanent rebind.

## A fn value beneath the apply word is data, and the read accounting under a tail apply (2026-09-08, the thirty-second increment)

The four Church and/or rows (`cand`, `cor`) refused inside their inner
lambda: `cfalse/v (q/v p/v apply) apply`. The paren-bounded pending apply
over the fn-typed param `p` declined its window because the entry beneath
(`q/v`, a fn-typed param) was a fn value ("nothing has established it is
not an applicable of its own"), and the gradual-lead event of the outer
apply declined its receiver (`cfalse/v`) for the same reason ("the
interpreter's apply would meet two applicables"). Neither holds under the
`apply` WORD: applyHandler re-steps the applied fn over the RESOLVED stack,
where a fn value on the value stack never dispatches — only a tape token
does — so every value beneath the lead is the callee's data, exactly as
the op binds it. A paren window with no apply word keeps the decline: inside
the paren that value was a token the interpreter stepped.

**What landed.**

- `RecordDynApply` admits a fn-valued window entry when a pending apply
  (the apply word's) owns the event; `recordGradualApplyEvent` admits a
  fn-valued receiver. The paren-without-apply window still declines.
- The unit finish's PAREN arm (a paren-bounded trailing apply as the whole
  residual, `TrailingApplyArity`) still excludes a fn-valued window entry
  — the tokens inside a paren are stepped, so such a value may dispatch —
  and the admitted windows made that exclusion reachable for the first
  time: its `//covergate:allow` pragma is gone (the coverage gate reported
  the guard covered).
- Admitting the windows exposed a hole in the bare-read accounting
  (NUR123): `fnResidualReplayReason` returned early for a unit ending in a
  body-tail apply, skipping the READ accounting with the count and replay
  arms, so a bare read of a fn-typed capture consumed as a window ARGUMENT
  — the numeral's `n` in `(x (n f/v apply) apply)`, which the interpreter
  DISPATCHES over `f/v` — lowered as data: the three csucc rows compiled to
  `f` applied to `n` (`expected 1 return value(s), got 2 — [fn n(Function)
  2]` for the interpreter's 3). The accounting now runs under a tail apply
  too (the count and replay arms stay skipped there), and those rows refuse
  soundly; their `n/v` spelling compiles.

**Measured.** All four Church and/or rows compile and agree byte for
byte; two run VM-native and graduated (T and T, F or F), and the frontier
gate found the §4 U-combinator row graduated with them — its
self-application `(s/v s/v apply)` is the same shape — so the ledger is
64 → 61;
and two still ISLAND — a def'd lambda's VALUE (the main program's
`cfalse/v`, `ctrue/v` too in the or row) applied inside a unit through a
carrier lead re-enters the interpreter, and the island's interpreter-minted
result islands again; they stay ledgered as islanded. Not the inner
lambda's captures (a capturing `cfalse` islands the same way); which
arrival carries a unit the op can enter is the next diagnosis. The numeral
rows are re-diagnosed to the bare-read class. Sound refusals pinned: the
csucc row with the interpreter's 3, its minimal shape, and a fn value
beneath a paren window with no apply word.

**What the next author should not re-derive.** A value beneath the apply
word is never an applicable: the word's re-step resolves the stack first.
The bare-read accounting must run for every plain-lambda or user-fn unit,
whatever owns its residual — an early return for one residual owner is a
hole for every read that owner's window consumes. The numeral rows' next
gate is a bare fn-typed read with a FORWARD argument (`n f/v`), which the
check models as data and the interpreter as a dispatch.

## The `/v` read of a def-bound capturing fn literal (2026-09-08, the thirty-third increment)

The two CPS rows (`factk`) refused "fn call operand of unknown provenance"
at `(factk (sub 1 n) kk/v)`: `kk` is a fn-body-local `def` of a CAPTURING
fn LITERAL (`def kk ( fn r:Integer Any [ … n … k/v apply ] )`), and its
`/v` read is a fresh wrap of the binding (ResolveRef) with no producer —
the thirty-first increment's alias traces a read to the bind-time
PRODUCER, and a literal has no event. Called as a word (`(kk 3)`) the
literal compiles as a unit with its captures passed at the call; as a
VALUE it had no operand.

**What landed.**

- `noteValBind` records a def-bound capturing literal (anonymous or
  nameless, unquoted) with its captures' bind epochs; `aliasValRead`
  builds the literal's closure operand at the first read
  (`tryReturnedClosure`: its unit pushed with the captures resolved in the
  binding unit), caches it on the bind, and hands it to the read
  (`readOps`), which `resolveOperand` consults before any other
  resolution. The alias holds only while every captured name is unbound
  since the def — the literal snapshotted its captures (NUR097), and a
  read-site construction after `def n 7` would see the new value where the
  interpreter's `kk` keeps 5.
- The push carries the def name (`ClosureRetSpec.DefName`) and the VM names
  the closure under it (`nameClosureValue`, as a store does), so a returned
  read renders `fn kk(Integer)` on both lanes.
- A bind inside a branch or loop body now records like any other: the
  arm's rollback moves the binding's generation and the entry on top, which
  the read checks, so the conditional-bind drop of the thirty-first
  increment was redundant.

**Measured.** The read handed to a Function param, the arrow spelling, an
unrelated bind between, the closure handed through and applied at the
main program, the value return and its render, the apply-word spelling and
the top-level read agree byte for byte and VM-native. Sound refusals
pinned with the interpreter's answers: a captured fn-local rebound between
the def and the read, the literal redefined by another, the read inside a
branch arm (the arm's gradual result — `residualLeadReStepped` — a
separate hold), and the CPS row, whose refusal MOVED to the then-arm's
pending apply over the `k:Function` param (`[ 1 k/v apply ]` is not the
body tail): re-diagnosed in the ledger, no row graduated.

**What the next author should not re-derive.** A capturing literal is a
closure the interpreter constructs at the `def`; the compiled read
constructs it at the read, which is the same closure exactly when no
capture was rebound between — the per-name bind epoch is the guard, not
the generation (a frame binding of a captured name bumps the generation
too). The CPS rows' next gate is the branch ARM: a pending apply whose
window is the whole arm residual, and the arm's gradual result at its
lead, which the re-step hold refuses because a native word's fn result
would dispatch where it lands.

## A pending apply at a branch arm's tail (2026-09-08, the thirty-fourth increment)

With the fn-local literal's read resolved (the thirty-third increment),
the two CPS rows refused "fn factk: apply of a dynamic fn value not at the
body tail" at the then arm `[ 1 k/v apply ]`: the `k:Function` param's
pending `apply`-word application sits at an ARM's tail, and the unit
finish's pending-apply arm lowers a pending window only as the whole BODY
residual.

**What landed.**

- `EmitRecorder.ArmTailApply` (a new seam method): the `if` word calls it
  on each arm's analysed residual, after the body run and before the arm's
  fragment is taken. An arm whose residual ends in a pending fn with at
  least one value beneath it INSIDE the arm collapses to the one gradual
  value the apply nets — `RecordDynApply` over that window, recorded into
  the arm's still-open fragment, so the arm's lowering applies the fn
  where the interpreter's applyHandler does, and the pending entry is
  consumed. The window is the arm's own: the arm frame seals the
  enclosing stack off on both lanes (the interpreter's arm is a frame
  that closes like a paren), so a pending fn with nothing beneath it in
  the arm passes through and keeps the unit finish's refusal — the
  interpreter applies it over the arm's empty stack and parks it.
- After the collapse the arm has ONE survivor, so `residualLeadReStepped`
  (the NUR101 rewind hold over a fn-typed lead among two or more) does not
  fire; the branch join types the arm gradual.

**Measured.** The two CPS rows graduated (ledger 61 → 59): the
continuation-passing factorial answers 120 and 3628800 on both lanes,
VM-native, with the fn-local continuation `kk` handed through the
recursive call as a closure value; a then-arm apply, both arms applying,
and a two-value window inside the arm agree. Sound refusal pinned: a
pending fn with nothing beneath it in the arm, with the interpreter's
parked value.

**What the next author should not re-derive.** A branch arm is a frame
on both lanes: its residual is the window an apply inside it sees, and
nothing outside it. The unit finish, the paren collapse and now the arm
are the three places a pending apply is consumed; a fourth residual owner
(a loop body, a `do` body) would need the same call at its own tail.

## The def-bound computed fn: a claimed shape, the read as a dispatch, and a wrapper applied on its own signatures (2026-09-08, the thirty-fifth increment)

The fn-util behaviour rows — `def k (FnUtil.const 7)  (k 99)`, `def h
(FnUtil.compose addone/v double/v)  (h 5)`, flip, partial, `on`, memoize —
refused "def-bound computed fn apply (closure shape unknown — Stage 1)":
the check pass binds a computed fn as a Function CARRIER of no shape, the
read substitutes that carrier, and the residual classifier had nothing to
lower it by. The refusal's own note blamed the island ("a lowered apply
here returned 99 for 7"); measured, the island applies the value exactly
as the interpreter does, and the 99 was `tryNativeFnApply` resolving the
wrapper's LABEL `const` through the live registry to the singleton-type
maker — a pre-existing miscompile of the EVENT spelling
`((FnUtil.const 7) 99)`, which compiled and answered 99.

**What landed.**

- `core.IsSelfContainedGoFnDef` and the VM's own-signature apply: an
  anonymous fn value wrapping nothing whose own signatures are all Go
  handlers (fn-util's `goFnValue` shape) is VM-native applicable on ITS OWN
  signatures — the interpreter's rule for an anonymous value — admitted
  ahead of the parked-native arm that resolves by name. `callDynMethod`
  gained `callDynamic`'s modifier-chain retry, so a `flip` wrapper over a
  compiled user fn enters that fn's unit with the args reversed instead of
  islanding. Error anchoring mirrors `stampErrPos`: the dispatching token's
  text replaces the word a handler named (`h`, one caret, not
  `FnUtil.compose`).
- `CheckState.FnShapes` (`NoteFnShape` / `FnShapeArity`): the arity a
  producing word CLAIMS for its computed-fn carrier. fn-util's words gained
  check-mode ReturnsFns that mint the Function carrier and claim the
  wrapper's arity — a constant for const, compose, pipe, curry and `on`;
  read off a concrete operand's single signature for flip, partial and
  memoize; no claim for a computed or overloaded operand.
  `producerReturnedClosureArity` consults the claim as its third source.
- `check.tryShapedFnReadArrival`, chained into the member-fn arrival hook:
  a def-read fn carrier with a claimed arity is the interpreter's WORD
  dispatch, modelled at the read over the wrapper's arity of
  evaluation-fixed tokens inside the statement — one guarded
  `OpCallDynMethod` (`RecordDynMethod` under the def name; the extra
  tokens of a longer window stay on the tape, `(k 1 2)` is `7 2`). Every
  other window REFUSES rather than declines: the residual classifier's
  flattened window loses the statement — `bigger 3 ; 5` lowered as
  `bigger 3 5` (compiled false for the interpreter's signature_error), and
  `(bigger 3 ; 5)` the same inside a paren, since the word dispatched at
  the `;` before the collapse. Measured before the model landed, on the
  claim alone.
- `EmitRecorder.DefReadName`: the read model's key from a carrier id to the
  word the interpreter dispatches.
- The fn-carrier read substitution (stepWord) runs on EVERY analysis pass,
  not only a compile pass. It was compile-scoped on the premise that a
  plain check never holds a table entry (a factory's `fn` handler runs in
  check and constructs the concrete fn); a native that RETURNS a fn
  carrier breaks it — `boru check` reported undefined_word and unused_def
  on `def k (FnUtil.const 7)  (k 99)`, and the check-accuracy and
  diagnostic-parity gates counted every such row the moment it left the
  frontier ledger (18 false positives, parity 339 over the 321 ceiling —
  found by the langspec gate, not the probes). The plain check then held
  the carrier and its arguments where the interpreter leaves one result —
  the type-soundness ratchet saw `(bigger 3 5)` as [Function Integer
  Integer] for the runtime's [Boolean] — so the read model has a
  plain-check half (`tryShapedFnReadWindow`, chained into the dynamic
  fn-value window): the same window collapses to one dynamic(Any), the
  def-bound test being the fn-carrier side table itself, since a plain
  check has no live recorder to remember the read.

**Measured.** The eight fn-util behaviour rows graduated (ledger 59 → 51),
all VM-native; the event spelling answers 7 on both lanes; `k 1 ; 3`,
`3 (k 99) add`, a 0-param wrapper's bare read `(p)` and two reads of one
wrapper agree; a wrapper whose applied fn returns two values raises the
same type_error at the same caret. Sound refusals pinned with the
interpreter's answers: the stack form `5 k`, a param read `(k x)` inside a
fn body, a paren inside the window, `(k 1 2)` (the survivor inside the
paren), an overloaded operand to flip (no claim), and the two statement-
window rows. The curried-chain row stays ledgered under a new mode: the
read model nets one dynamic value for `(c 10)`, so `(c10 3)` is a dynamic
lead with a paren-bounded argument.

**What the next author should not re-derive.** A def-bound computed fn's
read is a dispatch, and the dispatch happens AT THE READ with the word's
statement window — not at the residual, whose flattening cannot see a
`;`. The residual classifier keeps the event-lead spellings (a paren
re-step over two survivors), where the paren is the window. A claim is a
statement of the producing word's construction, never an inference from
the carrier; a word that cannot see its operand concretely claims
nothing, and the classifier's refusal stands.

## The read model over a def-bound produced closure, with the result shape (2026-09-08, the thirty-sixth increment)

The thirty-fifth increment's read model keyed on a claim only fn-util's
words wrote, so a def-bound PRODUCED closure — the factory pattern, `def h
(mk (z:Integer => [add 7 z]))` — still reached the residual classifier,
which had the closure's arity from its unit but not the statement: `h 2 ;
3` over a two-param closure lowered as `h 2 3` (compiled 12 for the
interpreter's signature_error), the same flattening the thirty-fifth
increment closed for the wrappers. And the typeof, repeated-read and
curried-chain rows stood behind three refusals that were one gap: the
apply of a def-bound closure did not collapse at the read.

**What landed.**

- `CheckState.FnShape` carries a RESULT shape beside the arity: the shape of
  the fn the wrapper returns when that is a claimed fn too (a curried
  chain's next level, a factory's factory). fn-util's `curry` claims the
  chain (one unary level per param, the last returning the value); every
  other word's claim keeps a nil result.
- The recorder claims a def-bound produced closure's shape at the def
  (`noteClosureShapeBind`, from `RecordDynBind`): the closure unit's param
  count, its own single closure out-op recursing for a factory of
  factories (`closureOpShape`), or a const lambda's. A producing word's
  own claim stands. `producerReturnedClosureArity` reads the same shape.
- The read models' one result follows the claim (`shapedReadOut`): a
  dynamic value, or — for a result shape — a Function carrier claimed with
  it, so `def f2 (f1 2)` binds a shaped carrier through the fn-carrier side
  table and `(f2 3)` models its own dispatch. The VM side is the
  thirty-fifth increment's: `OpCallDynMethod` over a closure enters
  `invokeClosure`, the chain's intermediate closure the one result.

**Measured.** Four rows graduated (ledger 51 → 47): the typeof operand
(`typeof (h 5)` is Integer), the repeated reads (`(h2 5) (h2 10)` is 15 30),
the three-level `mk2` chain (6) and the fn-util curry chain (7), all
VM-native; a three-level curry, a capturing closure read twice, a
two-param closure's full window and a read whose window is its statement
agree. The two flattened-window spellings refuse with the interpreter's
signature_error; the survivor inside a paren refuses with its answer. The
filter-body and each-body twins compile and agree but island, and stay
ledgered as islanded (re-diagnosed with the thirty-eighth increment: in
their forward form the read sits in the DATA list and the literal BODY
nets several values — a multi-value HOF body, not a read). Three neighbours pinned as refusals by
earlier increments compile and agree now and are re-pinned as parity: the
returned fn placed beside a value (`(h 5) 2`), its def-bound read applied
(`def q (h 5)  q 2`), and the two-factory row (`(q 7) (r 1)`). The bare
0-arity read of a body-local closure (`def r (mk)  r`) needed the read
model to CREDIT the read it dispatches (`RecordDynMethod` →
`creditWordRead`), or the NUR123 accounting counted it lost.

**What the next author should not re-derive.** A claim is written where
the shape is KNOWN — the producing word's ReturnsFn, or the def of a
produced closure — never inferred at the read; the read model then treats
a wrapper and a closure alike. A chain compiles because the claim carries
its result's shape and the read mints a carrier that carries the rest: no
level is special.

## The condition loop on the counted loop's frame (2026-09-09, the thirty-seventh increment)

`while` was family H's one member: the recorder's code-body-word gate
refused it, and where the recorder admitted the pure-literal regions the
VM bailed mid-run on the spliced mark/cond/move tokens and RunCompiled
re-ran the whole program on the interpreter. Six ledger rows and no
lowering. The plan (FULL-COMPILATION.0.md §6.8) named `WHILE_SETUP` /
`WHILE_NEXT` opcodes; none was needed.

**The interpreter's semantics, measured before a line was lowered.** The
condition region's LAST value decides and the extras are dropped; an
empty region raises `runtime_error: while: condition produced no value`;
`break` and `continue` discard the partial round's values; body values
accumulate (a variadic result, as `for`'s); a def the body rebinds
persists after the loop, and a zero-iteration loop leaves the pre-loop
value.

**What landed.**

- A `while` records on the counted loop's OWN frame (`RecordWhile`, over
  the `recordLoopEvent` record it now shares with `RecordLoop`):
  `FOR_SETUP`/`FOR_NEXT` over the consts start 0, step 1, end MaxInt64
  and a scratch iterator local no name reaches. The condition is a second
  fragment on the loop event (`emitLoop.cond`/`condOut`), lowered at the
  head of every iteration; a falsy value jumps (`JMP_IF_FALSE`) to a
  `FLOW_BREAK` placed past the back-edge, which pops the loop frame and
  trims the round exactly as a `break` from a callee does. Body
  classification, the iterator slot, the event and its result marks are
  `for`'s. The lowering admits a condition netting exactly one value and
  refuses every other count — sound, since the interpreter's fallback
  raises (empty) or drops (extra) where the lowered shape cannot.
- The check-mode model (`whileReturnsFn`) analyses the BODY first and the
  condition second. A def the body rebinds is registered loop-carried by
  the body's analysis, and a condition analysed before it resolved its
  read to the pre-loop value — the compiled loop never terminated. Both
  analyses run to their fixed points over the joined bindings, so the
  order changes no verdict. `EndLoopCarried` appends to the pending
  carried inits rather than replacing them (each analysis closes a carried
  scope).
- Every loop-event traversal in the lowerer visits the condition beside the
  body — `childFragments`/`fragmentOuts` are `[cond, body]`,
  `forEachOperand`, `forEachFragmentOperand`, `eachClosureCap`,
  `fragmentResultSeqs`, the dyn-bind scan and the memo walk. Without that
  a condition reading an enclosing computation (`(c get 'n') lt 3`)
  refused as "branch reads enclosing computation" and, once admitted, as
  "result operand of get is not on top"; `for`'s body with the same read
  compiled all along.

**Measured.** Four of the six ledger rows graduate (ledger 47 → 43): the
falsy condition, `break`, the truthiness read and the flex counter, all
VM-native and moved to `lang/spec/control.tsv` §7 with nine neighbours
(a carried rebind read by the condition and after the loop, zero
iterations, `break` and `continue` discarding the round, a `continue`
and a `break` inside a branch arm, a fn-body while over a param, a nested
while). The two remaining rows are re-diagnosed, each under a gate that is
not the loop's: the `continue` row's body holds `if ((c get 'n') eq 2)
[continue]`, a computed-condition no-else if whose one arm diverges,
which the branch recorder refuses under `for` too (`if (n eq 2)
[continue]` over a plain read compiles); the empty condition refuses as
"condition nets 0 values, not one" where a terminal trap (RecordTrap, top
level only) would graduate it. A two-value condition and a multi-value
body with a rebind (the pre-existing "dynamic-scope def of unpromoted
computed value", `for`'s too) refuse soundly.

**A pre-existing `for` divergence, recorded here, not fixed.** `def i 0
for 3 [def i (i add 1)] i` answers 2 on the interpreter and 3 compiled;
with `break` in the body, 0 against 1. The body's `def` of the ITERATOR's
own name rebinds the iterator's binding in the interpreter and the loop's
teardown leaves the outer `i` at the last iterator value, where the
compiled loop treats the def as a loop-carried rebind of the outer def
(`def n 0 for 3 [def n (n add 1)] n` is 3 on both). Not the while's shape
— its iterator is a scratch slot. The next author should decide which
answer is the language's before making the lanes agree: the
interpreter's reads as a teardown artefact.

**What the next author should not re-derive.** A condition loop is a
counted loop whose count is never reached, plus one fragment and one
conditional exit; the exit is the `break` machinery, not a new frame.
The order the two regions are analysed in is load-bearing for the
carried slots and for nothing else.

## The fn-carrier read inside a nested body, and the gate that was two gates (2026-09-09, the thirty-eighth increment)

The thirty-fifth increment's substitution — a read of a name def-bound to
a Function CARRIER resolves through the per-pass side table — declined
inside ANY nested body (`NestedBodyDepth > 0`: a branch arm, a loop body,
a `do` body) on the premise that such a body is re-run from its tokens,
where the compiled body carries no binding for the name. The premise had
a real failure behind it (`do [(f 2)]` once compiled to an island that
raised `undefined word: f`), but the decline was the wrong instrument:
it left `if c [(f 2)] [0]` and `for 2 [(f 2)]` reporting a FALSE
undefined_word — an ERROR-severity diagnostic on a correct program, on
the plain check — and `do [(f 2)]` behind the check-diagnostics sentinel.

**What landed.**

- The substitution fires inside a nested body too (`stepWord`, core).
  The read then models as the dispatch it is where the body lowers
  inline — a branch arm, a loop body, a while body — and a `do` body
  reaches the dyn-body backstop (its closure probe still declines the
  carrier's read: the probe carries no producer tables, so the operand
  has no compiled home), where the sub-engine resolves the name through
  the program's DynEnv twin to the runtime closure. All compile and
  agree with no VM island.
- The code-body gate (`recordCodeBodyClosureRead`) was two gates. It
  keyed on `producerReturnedClosure` at the def, which is true for a
  typed factory's CARRIER result (`def f (mk 1)`, a declared `Function`
  return) and for a lambda factory's CONCRETE closure (`def h (mkg …)`)
  alike. Measured with the gate removed: the carrier case compiles and
  agrees (its read is the side-table substitution and the read model),
  while the concrete case MISCOMPILES — `do [(h 1)]` compiled and raised
  `undefined word: g` (the captured param) for the interpreter's 8, and
  `[1 2] each [p/v apply]` islanded to `[[1 2]]` for `[[8 9]]`: inside
  the body's unit the read resolves to the FnDefInfo itself, whose home
  is an event outside the unit. `RecordDynBind` now notes only a CONCRETE
  produced closure for the gate; a carrier-bound name passes to the
  closure path.

**Measured.** The `do [(f 2)]` row leaves the ledger (43 → 42) and three
INLINE neighbours graduate to `lang/spec/bytecode-migrated.tsv` — a
computed-condition branch arm, a loop body with a body-local def, a while
body — each lowering with no interpreter entry. The `do` spellings
themselves stay in `frontier-hof-audit.tsv`, un-ledgered: they compile and
agree, but `do` reaches the dyn-body backstop, whose handler re-runs the
body in a sub-engine, and the batch gate's interp-entry census counts that
where `frontierRowCompiles` cannot see it (the section below records what
that cost). The plain check is clean on every one. The concrete-closure
rows keep the gate's refusal, the stack-form `each` and `filter` bodies
keep "code-body word … (Stage 2)", a two-value arm keeps "then-branch
result of unknown provenance".

**Measured and NOT landed: the `/v` read of a carrier-bound name.**
`stepWordVal` deliberately skips the side table (`TestStepWordValCarrier
KeepsUndefinedDiag`), so `2 h/v apply` over a typed factory's carrier
reports a false undefined_word AND a false unused_def on the plain check.
Consulting the table there was tried: `typeof h/v` compiles and agrees,
but the value read then hits the thirty-fifth increment's read model,
which treats the carrier as a BARE read and refuses "the statement ends
short of the wrapper's arity" — a misleading reason for a value read —
and the remaining spellings re-diagnose without graduating (`2 h/v apply`
is the apply-over-a-fn-typed-carrier refusal). The pmany/pseq rows were
unaffected either way. Graduation needs the `/v` read to mint a fresh ID
the alias (`aliasValRead`) seats, so the read model does not claim it;
the strict-lane false positive is the reason to do it.

**Found on the way, not fixed.** A produced closure whose body returns the
wrong declared type raises type_error on both lanes, but the compiled
report anchors on the fn literal with an empty name (`: return value 1:
expected Integer, got ProperString @1:36 src="fn"`) where the interpreter
anchors on the read (`f: … @1:93 src="f"`) — NUR122's position-and-name
class, for `checkClosureReturn`.

**What the next author should not re-derive.** The nested-body question
is not "is the body re-run" but "what does the read resolve to": a
carrier resolves through the side table on every pass and every depth,
and only a concrete produced closure has a home the unit cannot reach.
The next step for the code-body words is the closure PROBE: a fn unit
lowers the same read as `LOOKUP_DYN_SCOPE` + `CALL_DYN_METHOD` (the
dyn-scope rescue), and the probe declines it only because
`forkForProbe` seeds no `producedBy` — `[1 2] each [(f 1)]` refuses
"code-body word each (Stage 2)" on exactly that.

## The runtime-variadic REGION already existed: it is the loop's (2026-09-10, the thirty-ninth increment, NUR067)

`await {mode:'first'}` / `{mode:'any'}` hand back the winning branch's
WHOLE residual — 0-or-more values, a count that can EXCEED any static seat.
`awaitVariadicResult` refused the whole program on the compile pass, on the
stated grounds that "the emitter's variadic machinery covers only the
SHRINKING direction (the L-DO catch's N seats, 1 delivered)" and that the
growing direction needs "a runtime-variadic region representation" —
`design/FULL-COMPILATION.0.md` §6.6 named that as the *generalized mark
region*, an `OpStackMark` with no static count.

**The premise was half wrong, and the half that was wrong is the expensive
half.** Two measurements, both from reading code that was already there:

1. **The VM's mark ops are already count-agnostic** (`eng/go/vm.go`,
   `vmMark`). Their comments say "0-or-1", but `OpDropToMark` does
   `stack[:m]` — which truncates ANY count — and `OpPopMark` keeps whatever
   is above the mark. Nothing at the VM was 0-or-1-specific.
2. **A value-producing loop's region IS the representation.** `RecordLoop`
   registers ONE carrier for a loop whose trip count is a runtime value and
   marks the event `variadicResult`; `lowerLoop` pushes ONE simulated slot
   and marks `lw.variadic[seq]`. Every downstream rule then reads that one
   mark: `layoutOperands` refuses the slot as a call/list operand,
   `seatResults` admits it only in a variadic-absorbing LAST position, the
   store and dyn-bind hooks refuse it.

So the growing direction needed no new opcode and no new machinery. It
needed the await result to be RECORDED as the region it already is.

**What landed.**

- `awaitVariadicResult` returns the SAME `core.NewVariadicCarrier` on both
  passes. There is no compile-pass branch left at all — one model, and the
  wholesale `MarkUncompilable` is deleted (the refusal-site census drops one).
- `callVariadicRegion` (compiler) reads that carrier straight out of the
  dispatch's `outs` in `RecordCall`: exactly one out, and that out a
  variadic spread. Self-identifying, so there is no latch to leak onto a
  later dispatch the way `SetCatchVariadic` has to guard against.
- `eventFlags.variadicRegion` joins `variadicResult`. The pair is deliberate:
  `variadicResult` alone means "runtime-variable count" and every existing
  rule keys on it; `variadicRegion` adds "and the recorded slot stands for
  the WHOLE run", which is what `lowerCall` needs to mark `lw.variadic`.
- `lowerCall` gives the region the loop's lowering — one slot, `lw.variadic`
  — and refuses the two dispositions that need a static count: a frame
  PROMOTION (stores exactly `nout` values) and the DEAD-result drop (pops
  exactly one). Both refusals are reachable and pinned.

**The miscompile the change EXPOSED, and the guard for it.** With the region
recorded but nothing else changed, `99 TimeUtil.await {mode:'first'} [[1 2 3]]`
compiled to `CALL_DYNAMIC_TRAILING` and answered `[1 2 99 3]` where the
interpreter answers `[99 1 2 3]`. The cause is the spread carrier's own
contract: `NewVariadicCarrier` is Dynamic *by construction* (it must match
optimistically so the soundness oracle can intercept it), and
`resolveDynamicApply`'s trailing arm reads "a Dynamic value last over one
arg" as a fn value to apply. `residualHasVariadicRegion` now declines the
whole fn-value-call boundary when any residual entry is a region: a region
is a COUNT, not a value, so no apply arm can classify it. The ordinary
residual seating then rules — a lone region seats, a region above an inert
tail refuses "call result above a literal".

**Measured.** The ledger drops from 38 rows to 37: the plain-residual row
(`await {mode:'first'} [[1 2 3]]`) compiles and moves to
`lang/spec/module-time.tsv` with two siblings that pin the other two counts
(one value via `{mode:'any'}` over a rejecting branch, and zero via `[[]]`).
The two rows left in `frontier-await-winner.tsv` refuse at their CONSUMER,
each with the reason its LOOP twin already gives: `size [(await …)]` gets
"consumes loop results", the same as `size [(for 3 [i])]`; `99 await … [[]]`
gets "residual shape beyond Stage 1 (call result above a literal)", the same
as `99 for 3 [i]`. Their `failsWith` pins were rewritten accordingly.

**What the next author should not re-derive.** These two rows are no longer
an *await* frontier and should not be worked as one. They are the general
consuming half — an op that collects a region into a list, and a residual
seating that can put fixed values BENEATH a region — and graduating either
one graduates the `for` spelling in the same stroke. Start from the loop
rows, which are simpler and already in the corpus.

**Also fixed on the way.** `awaitResidual`'s doc referred to
`awaitClearVariadic`, a function that does not exist anywhere in the tree —
a stale reference to a latch design that was never built.

## Seating an inert prefix beneath a region (2026-09-10, the fortieth increment, NUR067)

The thirty-ninth increment gave the growing direction a REPRESENTATION and
left the two CONSUMING shapes refusing. This is the first of them, and it is
shared with the loop rows verbatim: a residual of
`[inert…, REGION]` — `99 for 3 [i]`, `99 await {mode:'first'} [[]]` — where
values have to end up BENEATH a run whose length is a runtime value.

**Why the ordinary seating cannot do it.** `seatResults` pushes inert
operands as a TRAILING tail, above the last event result, and refuses an
event above a queued tail ("call result above a literal"). For a region there
is no other static option: pushing the prefix after the region lands it on
TOP of the run, and no fixed offset reaches past a runtime count. The
existing repair for the non-region case — promote the producing event to a
frame local and re-push in residual order (`forceOrder`) — is exactly what a
region cannot do: a promotion stores `nout` values.

**What landed.** `OpSeatBelowMark`, one new op, and a plan that arms it:

- `planRegionPrefix` / `regionPrefixShape` (Finalize, before lowering)
  recognise `[inert…, REGION-last]` and put an `OpStackMark` in `markBefore`
  — the same map `planVariadicClaims` and `planMarkWindow` use, and it
  declines to both, so a program has one mark client.
- `seatRegionPrefix` (after lowering) pushes the prefix ABOVE the finished
  run and emits `OpSeatBelowMark n`. It returns a BOOL rather than a reason:
  a shape it cannot close falls through to the ordinary seating, whose
  refusal is the honest one, and the unused mark is emitted into a program
  that never runs.
- `vmSeatBelowMark` (eng) pops the mark m, lifts the top n values, and re-lays
  the stack as `[…, prefix, region…]`. The region's length is never named —
  that is the whole point, and it is why the ZERO case needs no special arm.

**Two facts the plan has to get right, neither visible from a passing row.**

1. **`singleSlotRegion` is not `variadicResult`.** A `do`-catch is
   `variadicResult` too, and it seats `nout` STATIC slots that shrink at run
   time — there is no one slot to seat a prefix under. Only a value-producing
   loop (`RecordLoop`'s `hasBodyOut` arm) and a `variadicRegion` call qualify.
2. **A loop's `condOut` / `bodyOut` are its FRAGMENTS' results.** The mark
   opens before the region's event, so an operand already live on the
   ENCLOSING stack sits BELOW the mark and the region would pop from beneath
   it (`def m {n:3}  99 for (m get "n") [i]` — the bound is a live `get`
   result). `regionReadsTheStack` therefore declines an event or closure
   operand — but it walks `start`/`end`/`step`/`carried` only, NOT
   `forEachOperand`: the fragment outs never touch this scope, and counting
   them would have declined every computed-body loop (`99 for 2 [(1 add 2)]`)
   silently and for a wrong reason.

**Measured.** Seven rows enter `lang/spec/bytecode-migrated.tsv` — the loop
spellings at three counts (three values, zero, a two-deep prefix), a computed
body, a multi-value body, and both await counts — and the ledger drops from
37 to 36. The stack-reading region keeps its refusal with the interpreter's
answer pinned beside it.

**What the next author should not re-derive.** The remaining consuming shape
is the collecting paren (`size [(for 3 [i])]`, `size [(await …)]`), and it
needs the OTHER half of §6.6's generalized region: an op that builds a List
of `stack[mark:]`. The mark plumbing this increment added is already the
right half of it — `markBefore`, one client per program, the plan/seat split
— so that increment is a second closing op, not new machinery.

## Collecting a region into a List, and NUR067 closed (2026-09-10, the forty-first increment)

The last consuming shape: a list literal whose one element is a paren over a
runtime-variadic region — `[(for 3 [i])]`, `size [(await {mode:'any'}
[[7 8]])]`. `OpMakeList` cannot build it, and for one reason: its Arg is a
STATIC element count.

**What landed.** `OpMakeListToMark` — pop the innermost mark m, replace
`stack[m:]` with one List of those values in order (ascriptions stripped, as
`OpMakeList` strips them: list elements are stored data). The plan is the
prefix plan's twin: `planRegionCollect` / `regionCollectShape` recognise the
pair and put an `OpStackMark` in `markBefore`; `collectRegionTop` (a bool, so
a shape it cannot close falls through to the ordinary refusal) emits the
close. The collected List is a static single value, so a PROMOTED result
stores and re-pushes like any other — `def xs [(for 3 [i])]  xs` compiles,
and only the region needed the mark.

**The safety argument is ADJACENCY, and it is the thing to keep.** The mark
opens before the region's event, so everything above it at run time must be
the region and nothing else. An event between the region and the list literal
could leave a value there, or take one from beneath it. So the shape requires
the two to be NEIGHBOURS in the top-level frame, and the seam test pins that
they must be — no whole-program row can show it, because the row that would
break is the row that refuses.

**What stays refused, and why it is not arbitrary.** A list literal with any
element BESIDE the region (`[9 (for 3 [i])]`, `size [(for 3 [i]) 9]`) keeps
"consumes loop results": its other elements would have to seat either side of
a run whose length is a runtime value, which is the prefix problem — and
OpSeatBelowMark solves that only for the PROGRAM residual, where there is
exactly one place to put things. A DEAD binding (`def _ [(for 3 [i])] 5`)
keeps it too: dropping the result wants a static count to pop.

**NUR067 is closed.** All three of its rows compile, `frontier-await-winner.tsv`
is deleted, and its ledger entry is gone (36 → 35). Eight rows enter
`lang/spec/bytecode-migrated.tsv`, `size [(await {mode:'any'} [[7 8]])]`
among them — the row the whole NUR was opened on, whose one-seat layout
compiled a stranded 7 and a 1-element list where the interpreter answers 2.

**What the next author should not re-derive.** The three increments took a
family that `design/FULL-COMPILATION.0.md` §6.6 had scheduled for the G-lane
("the generalized mark region") and closed it in the T-lane, with two closing
ops over machinery that already existed. The reusable lesson is the one the
thirty-ninth increment found: **a value-producing loop's region was already
"one recorded slot, a runtime count", and the VM's mark ops were already
count-agnostic.** Before building a representation, check whether the one you
need is already carrying a different client.

## A CALLABLE in the region: the review finding, and the older defect behind it (2026-09-10, NUR067 / NUR129)

A P1 review finding on the region PR, and it is correct. The row:

```
def g fn [[x:Integer] [Integer] [x add 1]]  await {mode:'first'} [[5 g/v]]
  interpreted  [6]      — the winner's residual [5, g/v] is spliced back onto
                          the tape and RE-STEPPED, so g/v dispatches over 5
  compiled     [5 fn]   — OpCallNative appends the handler's results as data
```

The rule it violates is NUR124's: a Function that ARRIVES on the stack
dispatches over what is beneath it. A region carries no per-value seat, so
nothing in the compiled lane re-steps one.

**Measured before deciding, and the measurement changed the answer.** The
same probe run on `0e0ad83` — before any of the three increments — says:

| row | on 0e0ad83 | on the branch |
|---|---|---|
| `for 2 [g/v]` | compiles, `[fn fn]` vs the interpreter's `uncalled_function` | unchanged |
| `5 for 1 [g/v]` | REFUSED ("above a literal") | compiled, `[5 fn]` vs `[6]` |
| `[(for 2 [g/v])]` | REFUSED ("consumes loop results") | compiled, `[[fn fn]]` vs an error |
| `await {mode:'first'} [[5 g/v]]` | REFUSED (NUR067) | compiled, `[5 fn]` vs `[6]` |

So the divergence is OLDER than the region work and lives in the bare loop
residual; the increments extended it into three shapes that used to refuse.
That split decided the fix: close the three the increments opened, record the
one they did not.

**The guard.** `regionValsMayBeCallable` answers, of the values a region's run
is modelled to leave, whether any could be callable — a fn value, a
Function-typed carrier, or a DYNAMIC one, whose runtime type the model does
not bound. It seats in two places:

- `RecordLoop` marks `eventFlags.regionMayBeFn` from the body residual, and
  `singleSlotRegion` refuses such a region — so BOTH consumers decline it
  through one test rather than repeating it.
- The await side cannot use the modelled element (`Any`, deliberately — the
  branches are unevaluated code bodies). It uses the COMPILED BRANCH UNITS
  instead: `compileStoredBody` inspects each unit's `outOpsVals`, and a
  non-empty element that did not compile counts as unknown-and-therefore-
  unsafe. `RecordCall` then refuses the dispatch rather than recording a
  region. Every graduated row still compiles, because their branch residuals
  are concrete.

**What it costs, stated rather than discovered.** The dynamic arm is wide on
purpose, so the two consumers decline regions the model merely cannot vouch
for — `99 for 3 [(f i)]` and its family — not only ones that carry a fn.
Those never compiled before this batch, so nothing regresses; the narrowing
(a dynamic residual that provably excludes Function, the not-disjoint rule the
residual lowering already uses for the same question) is the follow-up.

**The bare loop residual is NUR129, not fixed.** Applying the same predicate
at `RecordLoop` would refuse every loop whose body residual is dynamic, which
is a large class of programs that compile CORRECTLY today. Trading a rare
wrong answer for a common lost compile is not a call to make quietly, so it
is recorded with the measurement and what a real fix needs.

**And it costs back the refusal site the thirty-ninth increment saved.** That
increment deleted `awaitVariadicResult`'s wholesale `MarkUncompilable` and
lowered the census ceiling 93 → 92; this guard adds one refusal at the same
arity, so the ceiling goes back to 93 and the BATCH is net zero. Worth stating
plainly rather than leaving the earlier number to stand: a representation that
removes a refusal and then turns out to owe a smaller one has not reduced the
refusal machinery, and the census is right to say so. The ceiling comment
carries the round trip.

**What the next author should not re-derive.** The finding named await; the
defect is the region's, and both producers have it. Measure a review finding
against the MERGE BASE before scoping the fix — here that is the whole
difference between "the increments introduced a divergence" (three shapes,
closed) and "the increments extended one" (a fourth, older, recorded).

## A statically-empty `while` condition is a certainty, not an approximation (2026-09-10, the forty-second increment)

`while [] [1]` refused with "while: condition nets 0 values, not one" — the
lowering admits a condition netting exactly one value, and an empty region
nets none. That reads like the honest refusal it was: the recorder standing
aside where its model does not reach.

It is not. The refusal was keyed on the CHECK PASS's residual count, and the
row's real property is a fact about the SOURCE: the condition list holds no
tokens, so no value can be produced, so the interpreter's very first
condition round nets nothing and raises before the body has run once. That is
provable without any analysis at all.

**What landed.** `whileReturnsFn` (basic/go/whileloop.go) tests the condition
operand for the literally-empty list (`emptyWhileCond`) and records a
TERMINAL trap carrying the interpreter's own error — code `runtime_error`,
detail `while: condition produced no value`, word `while` — instead of
letting `RecordWhile` refuse. Everything recorded before the loop is kept and
emitted; `OpTrap` aborts there, exactly where the interpreter does.

**Why the guard is the source and not the count.** A condition WITH tokens
that the analysis happens to net zero values for is a different claim: the
count at run time is the condition's to decide, and the recorder has no proof.
Only the empty list is certain. `TestWhileNonEmptyConditionDoesNotTrap` pins
that direction, which is the half a positive row cannot show.

**The trap is top-level only, and that is `RecordTrap`'s pre-existing rule,
not a new one.** A trap inside a fn body, a branch arm or a loop fragment is
CONDITIONAL — the program has one terminal point and a fragment is not it —
so `RecordTrap` declines there and the arity refusal stands, with the
interpreter's answer intact. Both shapes are pinned in
`lang/go/while_compile_test.go`.

## The program residual can be rebuilt, and a frame count was written back too early (2026-09-10, the forty-third increment)

`(1 add 2) (3 add 4) 1 roll` refused with "residual shape beyond Stage 1
(call results reordered)". The full-stack fold models the permutation exactly
— it holds both values and knows the answer is `7 3` — and then the LOWERING
declines, because each call left its result where it ran and no static offset
reaches past a value already on the stack.

**What landed.** `seatResidualRebuild` (compiler/go/lower.go), tried only
after the in-place `seatResults` declines: spill every simulated-stack entry
to a fresh frame local, then push the residual back exactly as recorded — an
event result from its spill temp, an inert operand from its own push. It is
the program-residual twin of `spillSeat`, the call-site DDCG fallback that
has done this at operand sites all along; the two differ only in operand
direction (a residual is bottom-first, a call's operands sig-first).

One mechanism closes four shapes at once, and only the first was ledgered:
a PERMUTED residual, a DUPLICATED call result (`0 pick` — one temp read
twice), a DROPPED one, and an inert value seated BENEATH a call result (the
shape whose refusal read "call result above a literal").

**What it declines, and why each is a precondition rather than a policy.** A
VARIADIC region operand: its run is not one spillable stack entry, so
`OpStoreLocal` would take the run's last value and leave the rest. An event
operand not on the simulated stack: there is no value to spill for it. An
armed mark plan (the mark window, the region prefix, the region collect): its
`OpStackMark` is already emitted and indexes the very stack a spill would
empty. None of these is reachable from a source row that also needs the
rebuild, so all four are dialled directly in
`compiler/go/residual_rebuild_test.go`.

**The defect this exposed, which is the part worth carrying forward.**
`Finalize` seeded the lowerer's frame-local counter from the unit's planned
count, lowered the events, and wrote the count BACK — and only then ran the
residual reconciliation. Every spill temp the residual seating allocated
therefore landed outside the frame: `Program.NumLocals` said 0 while the code
held `STORE_LOCAL l0`. The failure mode is the quiet one — the VM fails its
first store, the program falls back to the interpreter, and the answer is
still RIGHT, with `compiled=false` and an EMPTY refusal reason to explain it.
The write-back moved after the reconciliation. It had been latent because no
existing residual path allocated a local; the general lesson is that
"compiled=false with no reason" is a bug report, not a refusal, and worth a
test of its own (`TestResidualRebuildFrameCountsTheSpills`).

## `for-each` never compiled its body, and that is why its Function form could not (2026-09-10, the forty-fourth increment)

`def dbl x:Integer => [mul 2 x]  for-each dbl/v [1 2 3]` refused with
"function-valued operand at for-each (Stage 3)". The gate is real: a
fn-valued operand reaching a fn-INVOKING word is refused because that
handler re-steps the fn on the tape, and the VM has no tape.

But `each dbl/v [1 2 3]` compiles, through the same handler family and the
same `InvokeBody` seam. The difference was not the operand at all —
**`for-each` declared no `CallableSpec`**. Its body therefore never compiled
to a closure: the dispatch baked the token list as a plain `List` const and
the handler interpreted it once per element. And for a word whose body does
not compile, the gate's premise is TRUE, so the refusal was correct for the
wrong reason.

**What landed.** A `CallableSpec` for `for-each`, and its case in
`lambdaCallbackInputs` (which had none, the same missing-case shape family
G's `each` rows had in 2026-08-27). Both graduate together, because the
closure body is what makes the fn-valued operand admissible.

**The spec is each's minus three flags, and each omission is the word's
own.** `BodyOut` 0, not 1: `forEachHandler` discards every invocation's
result, so the unit declares no returns and RETs whatever the body nets —
which is also why `EmptyBodyErrors` is absent, since a 0-net body is
for-each's ordinary case rather than an error the handler raises.
`BodyResultTop` is set for the stronger reason: the handler reads NOTHING of
the residual, so a fortiori never below its top. `CrossCollectionTokenShape`
is NOT set, and that one matters: it licenses committing to the List overload
for a statically-ambiguous (gradual-Any) collection, on the grounds that
`eachHandler` delegates to the map iteration when the runtime value turns out
to be a map. `forEachHandler` does not — it reads `args[1]` as a list — so
committing would raise where the interpreter iterates. The ambiguous-overload
refusal stays, pinned in `TestForEachKeepsTheAmbiguousOverloadRefusal`.

**The lambda convention was MEASURED, not inherited.** Sharing a handler
family is not evidence about the callback shape.
`for-each ([e:Any] => [typeof e print]) [1 2 3]` prints `Integer` and the
same lambda over `{a:1 b:2}` prints `KeyVal` — a list hands the bare element,
a map hands the KeyVal, which is each's convention, confirmed rather than
assumed. `TestForEachLambdaConventionMatchesTheInterpreter` keeps that
measurement as a test.

**What compiling the body COSTS, and why it is right anyway.** Two shapes
that previously "compiled" now refuse: `def acc (flex []) end [1 2 3]
for-each [acc swap append drop] end acc` draws "residual value of unknown
provenance", and the same body under a trailing literal draws "body leaves
extra values". They compiled before only because the body was never compiled
— the const list rode through and the interpreter ran it. Both refusals are
byte-identical to what `each` draws on the identical body today, so the
change makes the two words uniform rather than making for-each worse; the
corpus's one for-each row is unaffected.

## What the batch gate caught: two graduations and a render (2026-09-10, repairs to the forty-second-to-forty-fourth increments)

Three reds, and the shape of two of them is the one this line keeps meeting:
**a "sound refusal" test is a claim that can expire.**

**1. Two refusal rows became parity rows** — and both had to be MEASURED
before being moved, because a shape that starts compiling is a graduation
only if it compiles to the interpreter's answer.

- `def k2 x:Integer => [[a:Integer b:Integer] => [a sub b]] end 10 3 (k2 0)
  apply` was pinned as "the seating cannot reorder". The forty-third
  increment IS the reordering, so it compiles — to `-7`, which is the
  interpreter's answer. It moved into
  `TestProducedClosureApplyParity`.
- `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  5 (mk 3)` was
  pinned as "call result above a literal", with a test message asserting that
  an unclaimed parked result "must not compile to an apply (it answered 15)".
  The rebuild seats that residual now — and the compiled program leaves
  `[5 fn (Integer)]`, the PARKED PAIR, exactly as the interpreter does. The
  15 in that message was a note about an older attempt, not a live
  measurement; the assertion that matters (never applies) is unchanged and
  now checked on a compiling program.

**2. A spec row's expected value was written from the wrong renderer.** The
new for-each row `def acc (flex []) end … for-each dbl/v [1 2 3] end acc`
was written as `[]` because that is what `RunCompiled`'s host-value
projection prints; the TSV runner renders the ENGINE value, which is
`(flex [])`. Two lanes, two renderers — the corpus is the engine's.

**3. The check-accuracy ratchet asked the increment to finish its job.**
`control.tsv=2 (pin 1)` — the new `while [] [1]` ERROR row was an error row
the CHECKER did not flag. Bumping the pin would have been the wrong repair:
the pin exists to track checker coverage the compiler has, and the
forty-second increment's whole premise is that an empty condition is
statically DECIDABLE. So the check pass now mirrors it —
`runtime_error: while: condition produced no value`, error severity,
`RuntimeMirror` set, which is what keeps the compile pipeline compiling the
program to its terminal trap rather than refusing on an error diagnostic.

Two details worth carrying forward. The diagnostic is shaped inline rather
than through `CheckAddUniqueDiagnostic`, because the code is the RUNTIME's
own (`runtime_error`, so the report and the raise read alike) and that code
has no entry in `checkCodeSeverity` — an unclassified code defaults to
`info`, which does not gate `boru check`. And it is gated on
`FnBodyDepth == 0 && NestedBodyDepth == 0`: a mirror claims "the program
errors", so a fn body (runs only if called), a branch arm (only if taken)
and a catching `do` (swallows it) must all stay silent — the same
reachability rule the trap's top-level-only guard enforces one layer down.

**4. The interp-entry census tightened.** 33 → 32: for-each's body coming off
the interpreter takes fn-value.tsv's module-scope-fn-as-body-word row with
it. The census fails in BOTH directions by design, so the fall is a required
edit, not an optional one.

## The review was right about the class and wrong about the culprit (2026-09-10, the forty-fifth increment, NUR131)

A P1 review finding on the residual rebuild: *"When a full-stack word
duplicates or moves an event-produced closure, this unconditional rebuild
admits a residual whose callable must be re-stepped by the interpreter …
These shapes previously refused at residual seating."*

The witnesses are real, and they are worse than the finding says — they are
silent wrong answers on the default lane:

```
def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]] end
  5 (mk 3) 0 pick      compiled [5 fn (Integer) fn (Integer)]   interpreted [45]
  5 (mk 3) 1 roll      compiled [fn (Integer) 5]                interpreted [15]
```

**The premise is wrong, and checking it was the whole job.** Measured on the
merge base `d65f25a`: both compile to the same wrong answers THERE. They
never went through residual seating at all — the disassembly shows
`FoldFullStack`'s own promotion (`STORE_LOCAL` right after the call, then two
`PUSH_LOCAL`), which leaves the residual already in production order, so
`seatResults` accepts it and the rebuild is never reached. Two more witnesses
turned up the same way (`9 (mk 3) 9 2 roll`, `7 (mk 3) 1 pick`).

**Both were worth doing anyway.** The rebuild does not cause these, but it is
a mechanism whose entire job is re-pushing values, so it gets the wider
possibly-callable screen (`regionValsMayBeCallable`) before it may seat
anything — free, since it only forgoes graduations never realised. And the
FOLD gets a narrow one: decline `pick`/`roll` when a preserved entry is both
event-produced and provably a Function.

**The asymmetry between the two screens is the part to keep.** A wide screen
costs nothing on new machinery and costs live compiles on old: `def g …
(1 add 2) g/v 0 pick` is 5 on both lanes today, because a def-bound `/v` read
is not event-produced and the deopt machinery covers it. Widening the fold's
screen to match the rebuild's would have taken that row with it. Choose the
screen's width by what it costs where it sits, not by symmetry.

**The lesson for reading review findings.** A bot finding is a bug report,
and the report here was accurate about the CLASS and wrong about the
mechanism and the blame. Verifying against the merge base — one worktree,
three minutes — is what separated "my increment introduced this" from "my
increment is next to this", and the fix is different in each case: the first
would have been a revert, the second is two guards and a record.

## A deferred expression is an argument against the TRAP, not against the REMATCH (2026-09-10, the forty-sixth increment)

`p apply $.name` and `[10 20 30] apply $.1` refused the whole program with
"unmatched dispatch recovered at apply". Both raise `signature_error` on the
interpreter — `apply` is stack-only in both overloads (NUR098), so a forward
lens never matches — and a spec ERROR row should still yield a Program that
raises the same taxonomy at the same point.

`TryRecordUnmatchedDispatchTrap` declined them, on a stated and sound reason:
their window holds a REACH, and a deferred expression EXPANDS at dispatch
time, so a static trap baked over the unexpanded token could describe a value
the runtime never sees (`flex.tsv` L88/L95 — a reach over a mutated flex cell
resolves at run time where the static match saw the raw Reach).

**Read the reason against the instrument.** That argument is entirely about
the TRAP, which bakes a static error. The RUNTIME REMATCH reads no static tag
at all: it re-runs the match at run time over the values the interpreter's own
dispatch examines, which are the EXPANDED ones. So the same fact that
disqualifies the trap is what qualifies the rematch. The decline was one
`return false` too early — it short-circuited both.

The change is one line of classification: a deferred token sets
`needsRematch` instead of returning false. The flex witnesses take the
DEFERRING arm and keep their answers; the two ledger rows take the RAISING
one and are byte-identical to the interpreter, caret and candidate notes
included.

**What it does not cross, and why that line is structural.** A window whose
operands are in the wrong ORDER carries sigError's reorder hint ("did you
swap the arguments?"), which is derived from TAPE STATE the runtime rebuild
has no access to — the rematch already declines on exactly that. So
`$.1 [10 20 30] apply` keeps its refusal, and it is now the fallback-isolation
suite's refusing tail. That suite's tail has been re-chosen four times as the
subset widened; this is the first time it was picked for a reason that
widening cannot reach, rather than for being merely uncompiled today.

## The single-slot gate was inherited, not needed (2026-09-10, the forty-seventh increment)

`7 def b true do [1 2 (if b [] [9 9])]` refused with "variadic result
promoted to frame slots". The do-region ALONE compiles — `def b true do
[...]` answers `[1 2]` and its false twin `[1 2 9 9]` — so only the inert
prefix beneath it was the problem.

**The promotion was the problem, not the shape.** An out-of-order residual
(an event result above an inert bottom) force-promotes every residual event
to a frame local so the reconciliation can re-push in order. For a variadic
region that is unsound — the stores pop exactly nout values while the run's
runtime length is a different number — which `lowerCall` then said, one stage
later. Excluded from `forceOrder`, the run stays on the sim as a REGION and
`OpSeatBelowMark` lifts the prefix beneath it: the fortieth increment's
machinery, one producer over.

**Three gates were in the way and all three were inherited from the two
producers that happened to exist** (a value-producing loop, await's winner),
not from anything the mechanism needs:

- the plan's SINGLE-SLOT test. `variadicRegionEvent` asks the looser question
  the prefix actually needs — is the run's length not the recorded count? —
  while `singleSlotRegion` stays for the COLLECT, whose shape genuinely is a
  list literal over one recorded operand;
- `regionPrefixShape`'s "the region is the LAST entry", now the trailing
  contiguous RUN of the producer's entries;
- `seatRegionPrefix`'s "exactly one region operand", likewise.

`OpSeatBelowMark` never named the run's length, so the 2-value and 4-value
arms lower identically — which is the whole point, and what a fixed-count
lowering that happened to agree on one arm would fail on the other.

**Only the seq the plan will seat is excused, and only from `forceOrder`.** A
variadic region bound to a NAME (`def x (do …) x`) is promoted by its
dyn-bind source instead, and keeps the earlier, more informative refusal —
which is what the six `TestS9FrontierDefOverCatchRegion` rows check, and what
a blanket "never force-promote a variadic" would have quietly downgraded.

## The island's one slot was already the region (2026-09-10, the forty-eighth increment)

`def xs [0] do [1 div (xs 0 getr)] error [drop] end 2 add 3` refused with
"error: handler nets no value — the single-output island model would leave
the stack one short". The diagnosis was right — the caught path nets zero
where the pass-through nets one, and a fixed seat cannot carry both — and the
conclusion was wrong. The refusal's own comment named the fix it could not
see: "FallbackSpan has no out-count field to say otherwise".

It needs none. A run whose length is a runtime value is a REGION, and the
island's ONE simulated slot IS that representation already: `runFallback`
appends whatever the re-run produced, 0 values or 1. **Nothing changed in
what is emitted.** `errorReturnsFn` returns the honest 0-or-more spread
instead of marking the program uncompilable, `RecordFallback` marks the event
a region, and the residual absorbs the run.

**One ordering fix came with it, and it is the kind worth watching for.** The
region guard in `resolveDynamicApply` ran BELOW the NUR121 hazard scan. That
scan asks whether a fn-value LEAD had its argument collected by a later
dispatch — and a region is not a lead, it is a count. Asked of one, it
answered yes for this handler's 0-or-1 run and refused a program with no fn
value in it at all. A guard that says "this entry is not a value" has to run
before every guard that asks a question about the value.

**The through-line of all three increments**, and the reason they landed
together: each refusal was a true statement about a mechanism that was not
the only mechanism available. A deferred token defeats the trap and not the
rematch; a variable count defeats a promotion and not a mark; a variable
count defeats a fixed seat and not an island's own slot. The ledger falls
36 → 32.

## What a mirror needs is a RAISE, not a call (2026-09-10, the forty-ninth increment)

`import "boru:fn-util"  FnUtil.flip 5` refused the whole program with the
generic "check diagnostics". Behind it, `execFnDefLiteral`'s
`uncalled_function` arm — a NAMED fn value reached as a call whose match
failed — carried this note:

> NOT a RuntimeMirror: a mirror promises the program still compiles and
> raises the identical error, and there is no call here to compile —
> dispatch did not resolve, exactly like `no_signature`.

Every clause is true. The conclusion does not follow, because what a mirror
needs is not a CALL but something that RAISES IDENTICALLY, and a terminal
`OpTrap` is exactly that — the same answer `while []`'s empty condition got
seven increments earlier. Both rows raise the interpreter's own error today,
caret, span and `/v` hint included, and `frontier-fn-util.tsv` carries no
compile-ledger rows.

**The screen is narrower than the word-dispatch trap's, deliberately.**
`TryRecordUnmatchedDispatchTrap` resolves word bindings and routes the
inexact cases to the runtime rematch; there is no rematch op for THIS site
(the failure is a fn VALUE reached as a call, not a word dispatch), so the
screen only has to be SOUND, and `uncalledDispatchDefinite` takes the
obviously-sound version: every candidate a plain concrete const. A carrier, a
dynamic, an undefined placeholder, a raw word token, an open paren, a reach,
an unexpanded paren expr and a template string each decline, and the
whole-program refusal stands.

**The diagnostic's content does not move; only its stamp does, and only on a
compile pass.** `boru check` never compiles, so it still reports
`uncalled_function` at error severity — the mirror flag is what lets the
pipeline compile past a finding whose recording model is exact, and a plain
check has no trap to be exact about.

## A jump reaches the right pc and skips the discipline (2026-09-10, the fiftieth increment, NUR132)

Found shrinking the last `while` frontier row, not looking for it. `for 3 [
(7 add 2) if (i eq 2) [continue] [5] end ]` answered `9 5 9 5 9` where the
interpreter answers `9 5 9 5`, silently, exit 0, on the default lane.

A `break` / `continue` whose loop lives in the SAME unit lowered to a bare
`OpJmp`: to the loop's end for a break, back to `FOR_NEXT` for a continue.
Both destinations are right. Neither jump does the two things the VM's own
`flowSignal` does — and those two things are what the interpreter does:

- **it trims the round.** The interpreter's break/continue splices the
  round's tape back to the mark it opened, so a value the round already
  produced goes with it. `stack[:lp.iterBase]`, in the VM.
- **it pops the loop.** A break's jump lands PAST the `FOR_NEXT` that is the
  only op popping the loop, so the entry leaked. That one is not a wrong
  answer but a wrong PROGRAM: after `for 2 [ (i add 0) end for 3 [ if (i eq
  1) [break] [0] end ] ]` the outer loop's `FOR_NEXT` read the inner loop's
  stale counter and never terminated, dying at the growth ceiling where the
  interpreter answers `0 0 1 0`.

**Why the corpus never had a witness.** The round's value has to be COMPUTED.
`control.tsv` §7 already pinned the rule over a CONST one (`while [true] [1
break] end 'x'`), and those rows are right for the reason that hid this: a
const the round never seats is dropped at lowering, so there is nothing left
to trim. The computed twins mostly refuse before reaching the terminator
("branch leaves extra values"). Every witness lives in the narrow gap between
those two facts.

**The fix is a deletion.** Both terminators emit the FLOW signal ops
unconditionally; the signal resolves the nearest open loop at run time, and
`vmLoop` already carries the destinations the jumps named (`exitPC` is the
FOR_NEXT's own exit target, `nextPC` the FOR_NEXT). So `loopCtx.endHoles` —
a second source of truth for a pc the FOR_NEXT already holds — is gone. The
`while` lowering's condition-false exit had emitted `OpFlowBreak` for exactly
this reason since the thirty-seventh increment; the body's own terminators
now agree with it.

## "Nets no value" is not "diverges", and an arm can sit under its own condition (2026-09-10, the fifty-first increment)

The last `while` frontier row, and `frontier-while.tsv` with it. Family H is
CLOSED.

`def c (flex {n:0}) end while [(c get 'n') lt 3] [ set 'n' ((c get 'n') add
1) c end if ((c get 'n') eq 2) [continue] end (c get 'n') ]` refused with
"if: computed-branch non-eager arm diverges". The ledger's own note had the
important half right — the refusal is not the loop's, it is the body's `if` —
and then took the message at its word. The gate said DIVERGES and tested
`hasOut`, which two different things fail:

- a **0-netting** arm reaches the merge having produced nothing, so the join
  is 1-or-0 where the single slot models 1. That is a real problem, and it is
  the only one: `for 3 [ (7 add 2) if (i eq 2) [] end i ]` still refuses.
- a **diverging** arm (break, continue, raise, a tail call) leaves the
  construct and never reaches the merge at all. Every path that ARRIVES
  carries the eager value, so the slot is exact. `fragDivergesDeep` already
  existed to answer this, for `lowerArms`.

**A second gate was behind it, and it is the more interesting one.** The
computed-arm lowering assumed the eager arm is on TOP of the stack, because
in the spelling it was written for — `if c [t] (expr)` — the arm is the last
thing written and so the last thing evaluated. An arm filled from the VALUE
STACK by the argument-order rule was there BEFORE the `if` was reached, and
the condition, a forward token, is evaluated at the dispatch: the two sit the
other way round. No swap is owed in that layout, and the lowering refused
rather than not swapping. Both layouts lower now, which also compiles the
value-netting twin `for 3 [ (7 add 2) if (i eq 2) [3] end i ]` that neither
fact alone would have reached.

## What the review caught: a predicate that was complete for the producers that existed (2026-09-10, NUR133)

Three Codex P1 findings on PR #448, all three real, all three reproduced
before being touched. They are one question asked of four producers: what
does this region's own event CONSUME, and where does its run live?

`regionReadsTheStack` walked `ev.call.ops` and a loop's operands. That is the
complete list for a value-producing loop and await's winner — both `evCall` —
and it was written when those were the only two region producers there were.
The forty-seventh increment admitted a variadic USER CALL and the
forty-eighth an island's own run, and neither is examined by either arm, so
the mark opened above a value the region's op then popped:

- `def f fn [[n:Integer] [] [for n [i]]] 9 f (1 add 2)` → `0 9 1 2` for the
  interpreter's `9 0 1 2`;
- `def xs [1] [do [1 div (xs 0 getr)] error [drop]]` → `1 []` for `[1]`.

The predicate now switches on the event kind and names each producer's
operands, and its comment says the thing the old one implied wrongly: **a
kind that is not listed here is UNSCREENED, not operand-less.**

The third finding is separate and simpler. `RecordFallback` marked the island
a region without `regionMayBeFn`, and an island's run is the INTERPRETER
executing arbitrary code — what it appends is not bounded by the modelled out
at all. A Function passed through a handler was seated as data where the
interpreter re-steps it against the value beneath (`uncalled_function` for
`[6]`). Marked unconditionally now: a narrower claim would have to be a claim
about interpreted code the recorder never saw.

**Two of the three were the forty-eighth increment's, and one was not, and
measuring said which.** On the merge base `6bc55db` the two `error`-region
shapes REFUSED — so that increment turned two sound refusals into wrong
answers, which is the worst direction. The `evCallUser` witness diverged
there identically (`0 1 9 2`, a different wrong answer from the same
program), so the review's diagnosis of it was wrong about the cause and right
about the divergence: underneath the mark plan, `lowerUserCall`
force-promoted a VARIADIC-returning callee's result to ONE frame slot. One
`STORE_LOCAL` pops one value; the run had three, and the other two were
stranded beneath the prefix. `lowerCall` has carried the equivalent guard
since PR #280 and needs `nout >= 2` there — the user-call twin is needed at
`nout` 1, because a variadic slot IS one slot.

The const-argument twin `9 f 3` still compiles natively and is still seated
by `OpSeatBelowMark`. That pin is the point: the fix is three declines where
the model is not exact, not a retreat from the mark machinery.

## The fold's "top unit" was inherited from where it was first needed (2026-09-11, the fifty-second increment)

`[10 20] each [drop 1 2 3 1 pick]` compiled with an interpreter island, and
`frontier-full-stack.tsv`'s last ledger row said why: "the fold declines
outside the top unit; the island seam owns it". True, and the gate it
describes has two halves that were never separately argued.

`FoldFullStack` bakes the recorder's simulated stack as the runtime stack,
and the exactness condition it needs is that those two coincide HERE. The old
gate read that as `len(es.frames) != 1 || len(es.units) != 1`:

- the FRAME half is load-bearing. An open fragment — a branch arm, a loop
  body mid-capture — holds events not yet reconciled into any scope's
  residual, so the model is not a stack anybody will hold.
- the UNIT half is not. A compiled body unit has its own stack discipline
  exactly as the top unit does, and the decisive measurement is `depth`:
  `[10 20] each [1 2 3 depth]` is **4** on both lanes, not 4-plus-the-
  collection. The body's stack is the body's; the element is in it and the
  collection is not, and the recorder models exactly that.

So the gate is now "the current UNIT's root frame" — `frames[0]` for the top
unit, which is the old test verbatim, and `fnUnitRec.rootFrame` for a body
unit — plus one new screen the wider admission needs: an entry produced
OUTSIDE this unit is a capture or the parent's residual, not something on
this unit's stack, and declines.

**The widening had to be narrowed again, and the variation differential is
what said so.** The first cut admitted any entry whose producer was in this
unit. That turned FOUR working islands into REFUSALS — `(1 add 2) (3 add 4)
1 pick` wrapped in a fn, `do`, `each` and module body — and island-to-refusal
is backwards. The cause is not exactness at all: a body unit has no residual
REBUILD. The top unit can take a permuted residual (`seatResidualRebuild`,
the forty-third increment, spills every entry to a frame local and re-pushes
it in the recorded order); a body unit's residual seating refuses a result
that lands above a literal. So a body unit admits the fold only over entries
that need no rebuild — consts and locals, re-pushable from the same operand
home in any order — which is exactly what `varyRefusalLedger`'s own bucket
text had already predicted the graduation would be ("widened to the current
unit's local/const model").

So the class SPLIT rather than graduated: the const/local occurrence folds,
the event-produced one stays a working island, and the frontier keeps one
row — a different one. Graduation of the remainder = the residual rebuild
inside a body unit.

**What the widening must not take with it, and does not.** NUR131's callable
screen is per-ENTRY, so a produced closure shuffled inside a body is still
data to the fold and still refuses. The top-level rows that already folded
still fold. Both are pinned.

**The measurement that nearly stopped it, and why it did not.** Two fn-body
probe rows flagged as divergent — `def zf fn [[a:Integer][Any][1 2 3 depth]]
end (zf 9)` among them. Values, code, detail and the declaration note all
agree; the only difference is the CARET, `1:29` (the body's first token)
compiled against `1:48` (the call site) interpreted. That is NUR118, recorded
since 2026-09-04 and excluded by this line's own convention — one unit serves
every call site, so the compiled RET check is stamped with the unit's
position. Worth writing down that the flag fired and what it turned out to
be, because the next author running the same probe will see it too.

## The twin-placement cluster is THREE mechanisms, not one (2026-09-11, measured, not fixed)

Three ledger rows share one refusal string — "twin regime: a bind transition
has no stream placement (a multi-run-body or post-trap twin), so the rollback
would lose it" — and it is tempting to read that as one cause worth one
increment. It is not, and each row's own `why` in the ledger already said so.
Measured minimal witnesses, so the next author starts from facts:

```
[10 20] each [drop def zq 5 zq]                        COMPILES
[10 20] each [drop def zq 5 def zr 6 zr]               COMPILES
[10 20] each [drop undef zq 7]                         COMPILES
[10 20] each [drop def ZA (Integer gt 5) 7]            refuses   (type def)
[10 20] each [drop import "boru:math-util" end 7]      refuses   (module bind)
do [def b true  do [1 2 (if b [3] [9 9])]]             refuses   (nested do)
```

So the arm-residency bridge already carries the VALUE-def and UNDEF halves,
one or many, per element. What is left is three unrelated things:

1. **A TYPE def.** `AdoptResidentTwins` excludes it explicitly — `tr.Kind !=
   BindDef && != BindUndef`, or `bindTwinEntries[i].TypeDef != nil` — under a
   comment that says "shapes this increment does not carry".
2. **A module bind.** An `import` performs its transition without an
   `evDynBind` def-site event to pair with, so the bridge's
   `len(events) != len(twins)` check declines before the kind is even read.
3. **The nested `do`.** Nothing to do with residency: the inner body's
   closure compile declines on its residual shape, so the once-run body's
   def twin is never adopted at all. Confirmed element-independent of the
   variadic arm — `do [def b true do [1 2 (if b [3] [9 9])]]` refuses the
   same way as the `[]` twin the ledger carries.

**Which decline fires, measured rather than read off the code** (an ZZDBG
print in `AdoptResidentTwins`, reverted):

```
[10 20] each [drop def zq 5 zq]                    twin kind=def  typeDef=false   ADOPTED
[10 20] each [drop def ZA (Integer gt 5) 7]        twin kind=type-install         declines on KIND/TYPEDEF
[10 20] each [drop import "boru:math-util" end 7]  twin kind=def  typeDef=false   declines on COUNT: events=0 twins=1
```

So the two are not the same decline. A type def's twin is a distinct KIND
(`type-install`) the gate names explicitly. An import's twin is an ordinary
`def` — the `BindKind` census was right that a module namespace binding IS a
def — and it fails one line later because `import` records no `evDynBind`
def-site event for the bridge to pair an op with.

**The soundness question that makes either more than plumbing.** The obvious
move is to let `OpBindResident` replay the twin ENTRY per element, which is
what `ApplyBindTwin` already does at the top level. That is correct exactly
when the entry is the SAME every element, and both ledgered witnesses are
(`(Integer gt 5)` and `(A tand B)` do not read the element) — but nothing in
the bridge proves it.

**A tempting shortcut, closed by measurement.** It looks as though the import
half might be exempt: if a module path had to be a literal, its namespace
would be element-independent by construction and no screen would be needed.
It does not have to be. `def zp "boru:math-util" end import zp end
MathUtil.cbrt 8` runs and checks clean, so a computed — and therefore
possibly element-dependent — import path is legal boru. Both halves need the
same treatment.

That treatment is: the def site has to record provenance the bridge can
screen (an import records no event at all today), and the screen has to
establish element-independence — an inert-const path operand, a type
expression reading no unit-local or param — or else the op must REBUILD per
element instead of replaying. Choosing between those is the increment; the
wiring is the easy half.

## What CI caught: three spec rows in the wrong corpus (2026-09-10, repairs to the forty-sixth and forty-eighth increments)

PR #448's CI went red on `test/go/langspec` and the local batch gate agreed.
Nothing about the three compiler changes was wrong; the SPEC PLACEMENT of
three rows was, in two ways, and both are worth stating as rules.

**A sound refusal does not belong in the main corpus, whatever its answers
are.** The forty-sixth increment put `$.1 [10 20 30] apply` into
`lang/spec/apply.tsv` §5 beside the two rows it graduated. Its answers agree
on both lanes — it falls back and raises the interpreter's error — but it
REFUSES, and this corpus's refusal ceiling is 0 (`TestRefusalsAreFailures`,
`TestCompiledCoverage`). The row's refusal is real and stated (sigError's
reorder hint reads tape state the runtime rebuild cannot reproduce), so it
belongs where a refusal is pinned: the Go test that already asserts it
(`TestReorderHintWindowKeepsItsRefusal`), and not in the corpus at all.

**"It compiles" is not "it graduated" while an OpFallback span is still in
it.** The forty-eighth increment's own text says the region representation IS
the island's one simulated slot, and that is literal: the two maybe-raising
`do … error` rows compile — a real result, they refused before — and the
island stays. `bytecode-migrated.tsv`'s island ceiling is 0 too, so they go
back to `frontier-do-error-arity.tsv`, ledgered "islanded" alongside family
G's other working islands. Graduation is now a smaller, nameable step:
seating that region without re-entering the interpreter.

The lesson for the batch rhythm is narrower than "run the gate": the module
suites and `TestFrontier` were green before the push, and neither can see
these. The corpus-wide ceilings live in `test/go/langspec` and only the full
package run reaches them.

## The coverage gate found a functional hole, not a missing test (2026-09-10)

`make cover-gate` came back with ONE uncovered statement in the whole tree —
`compiler/go/emit.go`, the line inside the callable guard that sets
`storedBodyFnResidual` from a COMPILED branch unit's residual. Every other
module was 100%.

The uncovered line was not a test gap. It was the guard's compiled-unit half
never running at all:

```go
if unit < len(es.fnRecs) && es.fnRecs[unit] != nil &&
    regionValsMayBeCallable(es.fnRecs[unit].outOpsVals) { … }
```

`outOpsVals` is assigned in `fnResidualReplayReason` — AFTER its first line,
which returns early for a closure unit that is not a plain lambda. A
`spawnbody` unit (`compileStoredBody` → `compileClosureBody(…,
ClosureInValue, …)`) is exactly such a closure, so its `outOpsVals` was never
set. The callable test therefore read an EMPTY slice for every branch body
that compiled, and answered false. The guard only ever worked through its
other arm — "this element did not compile, so its residual is unknown".

**Measured, and it was a live divergence in the fix that was supposed to
close them:**

```
def g fn [[x:Integer] [Integer] [x add 1]]  9 await {mode:'first'} [[g/v]]
  interpreted  [10]      g/v arrives above 9 and dispatches
  compiled     [9 fn]
```

`[[5 g/v]]` declines to compile as a unit and so was caught; `[[g/v]]`
compiles and was not. The two rows differ by one token.

**The fix** is to record the residual values for EVERY unit, before the
closure early-return. It only ever populates a field that was empty for those
units, and the replay accounting the early return guards is untouched.

**What the next author should not re-derive.** A 100% coverage gate is not
only a test-completeness instrument. A statement that cannot be reached is a
statement whose CONDITION is never true, and when that condition is a guard,
"never true" is the bug. This one was found by the gate and by nothing else:
every functional test of the guard passed, because they exercised the arm that
worked.

## What the batch gate caught: three gates the region rows moved (2026-09-10, repairs to the thirty-ninth-to-forty-first increments)

The three region increments each passed their own tests and their own corpus
files. The batch gate then answered with three reds, and all three were the
gates doing their job on rows that had never been inside them before —
`frontier-await-winner.tsv` sat OUTSIDE every live census, so moving its rows
into the main corpus is the first time they meet these.

**1. Type soundness (the one that mattered).**

```
TYPE UNSOUND bytecode-migrated.tsv:L85: 99 TimeUtil.await {mode:'first'} [[1 2 3]]
  checked=[Integer dynamic(Any)] actual=[Integer Integer Integer Integer]
```

`stackTypeCovered` recognised a variadic spread ONLY at `checked[0]`, which is
where the `[]`-declared recursive fn's residual puts it (recursion.tsv:53). The
region rows put it on TOP of a fixed entry instead, and the fixed-length path
below then rejected a residual that says exactly what happens: "an Integer at
the bottom, then 0-or-more values". The oracle now reads the spread WHEREVER it
sits — fixed entries below align with the bottom of the runtime stack, fixed
entries above with the top, everything between absorbed — and every absorbed
entry still passes `typeCovered(elem, ·)`, so this is count flexibility only,
exactly as before. `variadic_spread_test.go` gained the new positions with
their negatives, including a leak in the absorbed middle while both fixed ends
match. 0 violations across 6460 rows.

The reason this is a fix and not a weakening: the pin was 0 and the count was
0 before the batch, so there was no other violation for the generalisation to
mask, and it admits nothing the spread carrier does not already claim.

**2. The interp-entry census, 35 against a ceiling of 33.** `BORU_LOG_CENSUS_ROWS=1`
named the two rows in one line each, and they were the same shape:
`await {mode:'first'} [[]]` — an EMPTY branch body. `compileStoredBody`
declines an empty token list (its first guard), so that branch fell to
`interpretBranchBody`, which spawned a sub-engine over zero tokens: one
interpreter entry inside an otherwise compiled program, for a result that is
empty by construction. The short-circuit is one guard in
`interpretBranchBody`, and it is behaviour-identical by construction — zero
tokens, zero steps, an empty stack either way. Back to 33 with two rows more
in the corpus than before.

The pin is `TestAwaitEmptyBranchEntersNoInterpreter`, and it applies the
census's OWN filter (`CheckMode || Attribution != ""`) rather than counting
every entry — the first version counted check-mode entries and failed on all
three rows for the wrong reason. Verified against a control with the guard
deleted: 1 unattributed entry per row.

**3. `TestNoStrandedOracleReads`.** The refusal rows in
`bytecode_await_test.go` compared `a.Run(src)` against `b.RunInterp(src)` as a
"fallback parity" check. The gate is right and the check was vacuous: after a
compile refusal `Run` IS the interpreter, so it was comparing that lane to
itself (NUR106). The rows already assert the expected value written out, which
is the assertion that carries weight; the second run is gone.

**What the next author should not re-derive.** A frontier TSV lives outside
the live censuses BY DESIGN, so "the row passes its own file" says nothing
about what the batch gate will find when the row graduates. Budget for it: of
the three reds here, one was a real gap in a gate (the oracle), one was real
debt the rows newly exposed (the empty branch), and one was a genuinely
vacuous assertion of mine. None was a bookkeeping bump.

## What the batch gate caught: two graduations that moved a ratchet (2026-09-09, repairs to the thirty-seventh and thirty-eighth increments)

The thirty-seventh and thirty-eighth increments each graduated rows into the
main corpus, and the batch gate answered with three red ratchets. Both
causes were real, and neither was in the compiled code — the compiled and
interpreted lanes agreed on every row throughout. What moved was what the
corpus MEASURES.

**A while's check-mode residual was a List where the runtime leaves N
values.** `whileReturnsFn` modelled the loop's residual as one typed-List
carrier, mirroring `for`'s non-static arm. For `for` that arm is
unreachable in the corpus (a computed count refuses), so the claim was
never measured; the moment the while rows entered `lang/spec/control.tsv`
the type-soundness oracle read it and flagged 5 violations against a pin of
0 — checked `[List, String]` where the runtime leaves `1 2 3 'z'`.

The claim was false, not merely imprecise: a while's trip count is never
static, so its residual is 0-or-more values of the body's type. That shape
has a device already — the VARIADIC SPREAD carrier (`SpreadPayload`), which
a `[]`-declared recursive fn's leak and `await`'s winner-takes-all use, and
which the oracle absorbs count-wise while still checking every element's
type. The plain check now returns it. The RECORDING pass keeps the
typed-List carrier: there it is the loop EVENT's stand-in, never read as a
type, because the compile lane refuses every consumption of a loop result
("consumes loop results") — measured, both spellings, before the change.

**A row that compiles is not a row that compiles NATIVELY.** The
thirty-eighth increment graduated `do [(f 2)]` into the main corpus on the
strength of `frontierRowCompiles`, which sees an `OpFallback` span and the
VM's own island seams. It does not see a HANDLER re-running a body in a
sub-engine, which is exactly what `do` does when its body reaches the
dyn-body backstop: the interp-entry census caught it (35 against a ceiling
of 33, both new rows named), and the diagnostic-parity count rose with it
(322 against 321). The two `do` spellings went back to
`frontier-hof-audit.tsv` — un-ledgered, since they DO compile and agree and
the frontier gate asserts that — and the three INLINE nested-body
spellings, which lower with no interpreter entry at all, stayed in the
corpus. Both ratchets returned to their pins with no ceiling raised.

**A third ratchet, and a pre-existing defect underneath it.** The
diagnostic-parity gate then read 322 against a ceiling of 321 — but only in
a FULL-PACKAGE run: alone it read exactly 321, twice, with identical row
sets. The switch is one corpus row (`bytecode-migrated.tsv`'s
`nd (m get "inc") apply` over a non-fn member) whose PLAIN check reports
nothing the first time the process ever checks that fn definition and two
errors every time after — measured directly, in a tenth of a second, with
fresh `lang.New()` instances either side. It is keyed on the definition
(name plus body: the same body under different names does not warm it), it
survives a fresh registry, and merely RUNNING the program warms it too. It
is not the ID sequence (reseeding does not change it) and not the
per-registry analysis memo or quota. **It reproduces unchanged at the
thirty-fifth increment**, so it is older than this batch: `boru check`'s
verdict on one program depends on what the process did before it, which is
the NUR103 class taken one step further — the verdict depends not only on
who is asking but on when. **LOCATED 2026-09-09** — see the section below.

What this batch actually added was ONE diverging row, and it was
accidental: the branch-arm row graduated with the thirty-eighth increment
wrote its condition as `def c (1 lt 2)`, which folds, so the else arm is
statically unreachable and the plain check says so where the compiling pass
does not. The row exists to pin a carrier-bound read inside a branch ARM,
not to exercise a decidable condition, so its condition is now a flex-map
read the checker cannot fold. Isolated parity returns to 320 — one below
the ceiling, where the thirty-fifth increment left it.

**The lesson, and the tool it earned.** A graduation is a claim about a row
that three gates measure independently, and passing the one you happen to
run is not passing them. The parity gate now names its diverged rows under
`BORU_LOG_PARITY_ROWS=1`, as the census and check-accuracy gates already
did — its ceiling has only ever moved with the exact row named, and
re-deriving that row by hand across 7,700 rows is the step that was
missing.

## The help hook's seventh leak channel: why the FIRST check in a process is the wrong one (2026-09-09)

The cold/warm defect above is a contaminated FIRST check, not a stricter
later one, and the contamination comes from the documentation system.

`lang.New` installs `EnableDynamicHelp`, which sets `Registry.OnRegisterHook`.
`installFnDef` fires that hook on EVERY fn installation — including the
user program's own `def app fn […]`, mid-check. The hook synthesises an
example expression from the word's NAME and one SAMPLE VALUE per declared
param type (`app 2 {a:1,b:2}` for `[[nd:Any m:Map]]`) and, the first time
that expression string is seen IN THE PROCESS, EVALUATES it for real —
a full `Engine.Run` in the very registry that is mid-`Check`. The gate is a
package-level `dynamicExampleResults` map that is never cleared, which is
exactly why the behaviour is once-per-process.

`makeDynamicEval` knows this run must be hermetic and closes six leak
channels, each documented in its header: the EmitState, the diagnostics,
the def stack, the step budget, the filesystem and stdin. The fn-analysis
memo is an unclosed SEVENTH. The synthetic run drives `AnalyseFnBody` over
the user's real body and memoises the residual in `Check.FnSummaries`; and
`FnAnalysisKey` renders arg TYPE NAMES only (`carrierTypeName`), so the
example's literal `2` and the program's literal `5` build the same key. The
program's own call then takes the memo HIT and is handed the EXAMPLE's
residual instead of analysing its own concrete arguments — so `apply` is
never dispatched against the concrete member and neither the `no_signature`
nor the two-value `type_error` is produced. The synthetic run's own
diagnostics are truncated on the way out, so the contamination is silent:
it shows up only as a MISSING diagnostic on the real call.

**The direction matters.** With the hook nil'd, the first check in a fresh
process already reports both errors. WARM is the uncontaminated answer; the
cold check is the wrong one, and every gate baselined on a first-check
answer is baselined on pollution.

**Measured, not argued.** The poisoned entry is directly observable: for
`app 'x' rules` — whose only call site is `(ProperString, Map)` — a cold
process holds an extra `#app#Integer,Map` summary that no part of the
program could have written. Breaking the key collision that way removes the
cold state entirely, with no warm process needed. Priming with the same
name and signature but a COMPLETELY DIFFERENT body warms the row; priming
with the same name and a different param type (a different rendered example
string) does not — so the key is the rendered expression, not the name and
not the body. Simulating the fix from the test side, by snapshotting and
restoring the memo family around the hook, makes cold equal warm equal the
no-hook control while `describe` still prints its generated example.
`CompileCheck` is identical either way because `BeginCompilePass` nils
`FnSummaries` before the compiling pass could consume the poison.

**The cut is TWO TABLES, and the wider one is wrong.** The instinct is to
restore the whole fn-analysis family — `FnInflight`, `FnNameInflight`,
`FnBodyChecked` and `PendingFnBodies` alongside `FnSummaries` and
`FnAnalysisCounts`. That was tried first and it REGRESSED four shapes that
compile today to "body result of unknown provenance": the recursive closure
of `bytecode_emit_test.go`'s capture-slot pin, the stored-sig poly of §6b,
the CPS arm-tail apply, and an in-unit named callback. The reason is the
same fact that makes the leak possible — the hook fires MID-INSTALL of the
user's own fn, so the enclosing pass has analyses IN FLIGHT and QUEUED
across it, and restoring those tables throws that queued work away; nothing
then analyses the pending body. What LEAKS is the memo of FINISHED
analyses; what must SURVIVE is the record of analyses still owed. Restore
the first, never the second. The measurement is in the helper's own comment
so a future widening has to argue with it.

**Pinning a once-per-process defect has its own trap, and it was walked
into.** The synthetic evaluation happens once per process per rendered
example STRING, and that string is built from the fn's NAME and its PARAM
TYPES. So a pin whose fixture reuses a name+signature another test in the
same package also defines is VACUOUS in the suite that CI runs: whichever
file sorts first spends the one evaluation, and the pin then measures the
already-warm path and passes with the fix deleted. The first draft of
`TestCheckVerdictIsHermeticAcrossProcessHistory` used `def app fn [[nd:Any
m:Map] …]`, which `bytecode_edge_findings_test.go` (sorting earlier) also
defines — it failed when run alone and passed in `go test .`. The rule is
the same one the leak pin already stated for `dhleak`: give the fixture a
name defined nowhere else. And the FEATURE guard has the mirror trap — the
example EXPRESSION is rendered from the signature alone and prints whether
or not anything was evaluated, so asserting it does not forbid the one wrong
fix ("skip the evaluation instead of isolating it"). Only the result half,
`;# 2` rather than the placeholder `;# ...`, comes from the synthetic run.
Assert the line.

**The frontier ledger does not move.** The wider cut also re-diagnosed
`frontier-capture-namespace.tsv:15` — under it the row refused at its own
residual rather than at the capture slot. The narrow cut does not: that row
still refuses with `capture ParseLang of calc unreachable at a call site`,
and the ledger entry stands unedited. A re-diagnosis measured against an
abandoned fix is not a measurement.

**What the next author should not re-derive.** The memo is deliberately
keyed on types, not values; making it concreteness-aware would collapse its
hit rate and change checking repo-wide. The local cut is isolation — the
seventh channel closed like the other six — not a smarter key. And a
per-registry `dynamicExampleResults` is strictly worse: it makes EVERY
check behave like today's cold one, so the leak would mask the real
diagnostic every time instead of once.

## The memo outlives the pass too: a SECOND, hook-independent instance of the same class (2026-09-09, measured, NOT fixed)

Closing the seventh channel makes one check hermetic against the help
example. It does not make the memo pass-scoped, and there is a second way a
check's verdict depends on history — reachable with the help hook disabled
entirely, so it is not the leak above wearing a different hat.

`CheckState.Begin()` resets every other member of the fn-analysis family —
`FnAnalysisCounts`, `FnNameInflight`, `FnBodyChecked`, `PendingFnBodies`,
`InflightBails` — and does NOT reset `FnSummaries`. Only `BeginCompilePass`
nils it, and `SetStrictCheck` clears it by hand, its comment already
conceding the point ("on a reused instance a bare Begin keeps them"). So the
memo survives from one `Check` to the next on one registry. That matters
because `FnAnalysisKey` identifies a body by its FIRST TOKEN'S row:col, not
by its content: two different programs whose fn shares a name, arg type
names, captures and body start position build the identical key, and the
second check is handed the first's residual.

Measured:

    a := lang.New()
    a.Check(`def zzff fn [[x:Integer] [Any] [x mul 2]]     zzff 2`)  // clean
    a.Check(`def zzff fn [[x:Integer] [Any] [x mul {a:1}]] zzff 2`)  // → []

A fresh instance checking the second program alone reports its
`no_signature` (`mul` got `(Integer, Map)`). Both bodies start at 1:33 and
both calls are `(Integer)`, so the keys collide and the failing dispatch is
never attempted. The reverse order is safe — the memo caches the residual,
not the diagnostics — so the failure is ONE-DIRECTIONAL, exactly as the help
leak is: an error silently disappears, never a phantom appears.

**Why it is recorded rather than fixed here.** Nothing shipped reaches it:
every in-tree `Check` caller builds a fresh instance
(`cmd/go/internal/lsp/diagnostics.go`, and `cmd/go/internal/check/check.go`
inside its per-target loop). And the two candidate fixes are both changes to
the memo's LIFETIME or its KEY, repo-wide: have `Begin()` nil `FnSummaries`
as it nils `FnAnalysisCounts` (giving up cross-check memoisation, with its
own gate to run), or make the key carry body CONTENT rather than body
position (which is the same "make the key smarter" move the seventh-channel
section argues against for the type-name half, and would want its own
hit-rate measurement). Neither belongs bolted onto a leak fix. The shape an
embedder would hit is an instance-caching LSP re-checking an edited buffer:
it would stop reporting a type error the user had just introduced.

## What closing the seventh channel cost, and what it exposed (2026-09-09, all three measured)

Adversarial review of the fix found three things the fix itself does not
say. All three are reproduced against built binaries — `boru-with` (the
shipped cut) against `boru-without` (6007c2a) — not argued from the code.

**1. The same leak is still open one caller over.** The defect's SHAPE is
"an analysis whose diagnostics are truncated while its `FnSummaries` writes
survive". `compiler/go/code_effect.go`'s `AnalyseCodeEffectCarrier` is a
self-described DRY pass over a stored quoted code list: it snapshots
`diagBase`, runs `core.RunCarrierBody`, and calls `TruncateDiagnostics` —
with no `IsolateFnAnalysis`. So the dry run's summaries survive under the
type-name key and the program's own call site takes the hit, exactly as it
did through the help hook. Reachable from ordinary source:

    def zk1 fn [[nd:Any m:Map] [Any] [nd (m get "inc") apply]]
    def rules {inc: 42}
    def ops [(quote [zk1 2 rules])]     # <- add these two lines and
    def b (ops get 0)                   #    the no_signature disappears
    zk1 5 rules

Without the two middle lines the check reports both errors; with them the
`no_signature: apply` is silently lost and the surviving `type_error`
renders the DRY pass's operand `2` instead of the user's `5`. `boru-without`
reports ZERO errors for the same program — both channels were leaking — so
the shipped cut is a partial improvement, not a complete one.
`IsolateFnAnalysis` is already the right primitive; it is simply not
deferred at that site. Left for its own increment because it is a change to
the compiler's check path and wants the full ratchet run, not a bolt-on.

**2. It exposes a pre-existing `unreachable_branch` attribution defect, and
that is a REGRESSION on a shipped file.** `boru check utils/wc.boru` goes
from `0 warnings` to one FALSE `unreachable_branch` at 166:3. The warning is
wrong: `wc-row` has two call sites, one passing the literal `"total"` (for
which the then-branch is indeed dead) and one passing `(it.label)`, which is
`""` whenever `acc.named` is false. The then-branch is reachable.

The mechanism is not the leak fix. `EmitUnreachableBranch`
(`basic/go/native_control.go`) reports at `CurCallPos` — a position INSIDE
the shared fn body — while the constancy comes from THIS call's bound
argument. It is a fact about one call site reported at a position both call
sites share. Before the fix, the memo hit meant only one call shape was ever
analysed, so only one such warning could ever be emitted, and the defect was
masked. An honest analysis analyses both shapes, and both warnings survive
`CheckAddUniqueDiagnostic` because it dedupes on code+detail+position and the
details differ. The result is one `if` drawing two CONTRADICTORY warnings at
one position — measured on a three-line program:

    def zwr fn [[label:String] [String] [ if (label eq "") ["empty"] [label] ]]
    def a (zwr "")
    def b (zwr "total")

    without: 1:39 constant true; else-branch is unreachable
    with:    1:39 constant true; else-branch is unreachable
             1:39 constant false; then-branch is unreachable

No suite catches it: `utils/` is outside `make test`'s module fan-out, and
`utils_e2e_test.go` runs the binaries rather than asserting check output.
The principled repair is at the emitter — a constancy that comes from a
bound param is a property of the CALL, not of the code, and must not be
reported at the body's position — but distinguishing it from a genuine
source literal (`if true [...] [...]` written in a body) is a diagnostics
design question with corpus-wide reach and the check-accuracy gate to
answer to. It is not a bolt-on to a leak fix. Also worth recording: the same
honesty turned `kg/tests/resolution_test.boru` from silent to two TRUE
errors (an undefined `KgSchema` the memo hit had been hiding), so the
exposure cuts both ways.

**3. `boru check` is materially slower, and the cost is NOT the clone.**
Best-of-3 wall clock, same machine, byte-identical output:

    kg/storage.boru        1.96s -> 3.82s  (+95%)
    kg/validate.boru       0.78s -> 1.25s  (+61%)
    lang/go/modules/sift.boru  2.35s -> 3.30s  (+41%)
    aggregate over 31 corpus programs         +9.4%

Several programs got FASTER (-16% to -33%) because an honest analysis
short-circuits earlier, so it is a redistribution with a bad tail rather
than a uniform tax. **Do not optimise `cloneMap`**: a control build that
keeps both clones and makes the restore a no-op runs at the without-fix
speed. The cost is the DISCARDED memo forcing re-analysis — `AnalyseFnBody`
misses go 852 -> 1396 on kg/storage.boru. It is linear in (hook evaluations
x per-example cascade), not quadratic: a synthetic deep call chain at
N=25/50/100 shows identical miss counts on both builds. The honest way to
buy it back is to stop doing the synthetic evaluation inside the user's pass
at all, not to make the isolation cheaper.

For the record, two things that are NOT problems, both measured. The
`FnAnalysisCounts` refund cannot let a program exceed `FnAnalysisQuota`: the
counter increments on the memo-MISS path only, so refunding the example's
increments and re-consuming them on the program's own now-missing calls nets
the same or lower. And nothing changes when help is disabled — the call sits
inside `makeDynamicEval`'s returned closure, which a registry with no
`OnRegisterHook` never reaches.

## The root fix: help examples are generated ON DEMAND (2026-09-09)

The isolation above closes the seventh channel. It does not answer the
question the three costs were really asking, which is why a DOCUMENTATION
feature was running inside a user's analysis pass at all.

`EnableDynamicHelp` set `OnRegisterHook` to a function that built the word's
`FuncInfo`, synthesised an example, and EVALUATED it — a full `Engine.Run` —
at every fn registration. The hook fires from `installFnDef`, so the user's
own `def f fn […]` triggered it mid-check. Nothing about that timing was
needed: the only consumer of the result is `describe` (and LSP hover), and
those run later, with a registry in hand.

So the hook now RECORDS and nothing else — `Registry.NoteHelpWord`, one map
insert — and every render site goes through one new function,
`native.FormatWordHelp(r, info)`, which generates the example if the word was
registered after startup and then renders. Seven `help.FormatDynamic` call
sites across `lang/go/native/describe.go`, `cmd/go/internal/describe` and
`cmd/go/internal/lsp/hover.go` route through it; there is now exactly one
place where a synthetic evaluation can happen, and it is a place the user
asked for output.

**What it fixes, measured.**

- The leak is gone at the source: nothing synthetic runs during a check, so
  there is no `FnSummaries` write to isolate. `IsolateFnAnalysis` stays as
  belt-and-braces — and it is now load-bearing for a DIFFERENT timing, since
  the evaluation happens inside the user's RUN of `describe` and must not
  disturb that run's def stack, budget, recording or diagnostics.
- `boru check` gets dramatically faster, because the eager hook was paying
  for every word's example on every run and the memo hit only ever hid part
  of that cost. Best-of-2 over 30 corpus programs: **21.3s -> 5.7s, -73%**.
  Per program: `lang/go/modules/cli.boru` 2.53s -> 0.13s (-95%),
  `kg/storage.boru` 1.89s -> 0.24s (-87%), `kg/validate.boru` 0.78s -> 0.16s.
  For comparison, the ISOLATION alone made these SLOWER (kg/storage 2.47s ->
  4.09s), because it discarded the memo and forced re-analysis. Laziness
  removes the work instead of redoing it.
- `describe` is unchanged: `describe kk9` still prints
  `kk9 2 {a:1,b:2}   ;# 2`, generated on demand.
- Diagnostics are unchanged from the isolated build: 53 corpus programs
  compared line for line, zero differences. Laziness and isolation reach the
  same honest answer; only the cost differs.

**What it does NOT fix, and this is the useful negative.** The false
`unreachable_branch` on `utils/wc.boru` survives laziness exactly as it
survived isolation — measured on both builds. That settles a question worth
not re-deriving: the warning is not an artifact of HOW the leak was closed.
It is a latent attribution defect (`EmitUnreachableBranch` reports at a
position inside the shared fn body while the constancy comes from THIS call's
bound argument) that the memo hit was masking by only ever analysing one call
shape. ANY correct fix reveals it, so it is not avoidable by choosing a
different one — it has to be fixed at the emitter, and that is its own piece
of work with the check-accuracy gate to answer to.

**What the next author should not re-derive.** The register-time hook was not
load-bearing for anything. Its one implementation was help, its one consumer
is `describe`, and moving the work to the consumer is strictly better on all
three axes at once. The one thing recording still buys is the "post-ready
words only" restriction — a word registered BEFORE `MarkReady` has a
build-time snapshot from `genhelp`, and generating for it on demand would be
new behaviour, not a fix. `IsHelpWord` is what keeps that line.

## The unreachable_branch attribution fix: a dead branch is a claim about the CODE (2026-09-09)

The lazy-help change exposed a latent defect rather than causing one:
`boru check utils/wc.boru` reported a FALSE `unreachable_branch`, and one `if`
could draw two CONTRADICTORY warnings at a single position. The memo had been
masking it by only ever analysing one call shape per (name, arg-type-names)
key; it survives BOTH ways of removing that mask (isolation and laziness),
which is what proved it latent.

**The mechanism.** The check pass analyses a fn body at least twice: a
DECLARATION-shaped run with carrier args (`checkFnBodyAtConstruction` →
`ParamBodyCarrier`), and one run per concrete CALL SHAPE. When a call passes a
concrete value the body's `if` folds for THAT call, and
`EmitUnreachableBranch` reports it at `CurCallPos` — a position inside the
SHARED body. A property of one call, reported where every caller sees it.

**The fix is one counter.** `CheckState.CallShapeDepth` counts enclosing
`AnalyseFnBody` frames entered with at least one `IsConcrete` argument or
capture; `EmitUnreachableBranch` returns early while it is positive. The
declaration-shaped run keeps it at 0 even at `FnBodyDepth > 0`, so it stays
loud — and it is the run whose verdict holds for every caller. Three
production files, +38/-15.

`FnBodyDepth > 0` alone would NOT work, and this was measured, not assumed: a
genuinely-written `if true` inside a body emits at `FnBodyDepth == 1`,
byte-identical to the false positive. The narrower counter is the whole point.

**Measured.** `utils/wc.boru` 1 -> 0 warnings. The three-line repro's
contradictory pair 2 -> 0. Every true positive survives at exactly 1: a
written `if true` at top level, in a body, in a body called under two shapes,
in a fn never called; a body-local fold `("a" eq "b")`; the folded-paren
golden `if (1 gt 0) [10] [20]`; lambdas, nested inner fns, generics, and
self-/mutually-recursive fns. Four corpus rows stop being flagged, each proven
a false positive BY EXECUTION — `repeat "a" 3` returns 3 through the `[0]`
base case the checker called dead, `MR.fac 10 1` returns 3628800 through
`[acc]`, and `MR.aev 8` returns true through `[true]`. Nothing anywhere gains
a warning.

**It also fixes a second, independent defect.** All three emit constructions
now funnel through the one guarded helper, and none of them ever used
`CheckAddUniqueDiagnostic` — so a written `if true` in a body called under two
shapes used to emit the SAME warning three times (generalized run + two
specialisations). It now emits once.

**The accepted cost, and why it is acceptable.** A genuinely dead branch in a
CALLED module fn no longer warns, because a module fn has no
declaration-shaped run to fall back on — recorded as NUR128, with the reason
(module registries run their body with check INACTIVE so exports get concrete
names) and the trap in the obvious recovery. The class is already incoherent
on HEAD: the same module fn, never called, reports nothing. The fix makes it
consistently silent rather than call-dependent.

**What the next author should not re-derive.** Three other designs were built
and measured before this one was chosen, and each is a dead end worth not
re-walking:

- **A provenance bit on the value** (mark a constant whose concreteness came
  from a param binding) is smaller still and keeps the module class — but it
  provably silences a condition constant for EVERY call shape, including a
  semantic tautology like `if (s eq s)`, whose else arm is unreachable for
  every possible input. The design cannot express "constant regardless of the
  param's value"; that is inherent, not a bug.
- **Evidence intersection across shapes** (emit only where every analysis
  agrees) loses no true positive in the plain pass, but silently loses a
  literally-written `if true` in the ARMED pass, invisible to every gate
  because no corpus row has that shape. It also installs a second diagnostics
  channel that must mirror every lifecycle operation of the first — eight
  `TruncateDiagnostics` pairings, none compiler-enforceable.
- **Re-attributing the warning to the call site** does not fix the class at
  all: the contradictory pair reproduces one hop of nesting up, and
  `utils/wc.boru` still warns.

Do not look for provenance ON the folded Boolean: it is `Pos=0:0` with no
`dynFrom` at every emit site, byte-identical for a source-written `if true`
and a fold over a bound param. The analysis CONTEXT is the only signal there
is.

## …and the region-prefix seat, which is the shape the rebuild cannot take (2026-09-11, the fifty-fifth increment)

The rebuild spills one stack entry per residual operand. A residual shaped
`[inert…, REGION]` has nothing to spill: the region's run is a length the
compiler does not know, and the prefix must land BENEATH it. `OpSeatBelowMark`
is the op for exactly that, and the forty-first increment gave the PROGRAM
residual the plan and the seating. A body unit had neither — its lowerer is
constructed with `markBefore` nil and `regionPrefixSeq` zero — so
`reconcileResults` met `do [1 (if b [] [9 9])]` and refused "result above a
literal".

It has both now: `planRegionPrefixUnit` arms the plan before the unit's
`lowerEvents` walk (the walk reads `markBefore` as it goes, so the
`OpStackMark` lands ahead of the region-starting event), and
`reconcileResults` tries `seatRegionPrefix` first, exactly as
`seatProgramResidual` orders them.

**Why it needed a new shape function, which is the part worth keeping.** The
obvious move — call `regionPrefixShape` with the unit's residual values — is
what I wrote first, and it declines every time. Measured: the inner do-body's
residual is `[1, "None tor Integer"]` and the SECOND value's ID is not in
`producedBy` at all, because a branch merge's carrier is minted at the JOIN
rather than by the event. The unit's resolved OPERANDS say the same thing
plainly — `[CONST, EVENT(the branch)]` — so `regionPrefixShapeOps` reads the
shape off those instead. Same three conditions, no inference.

**What it graduates, A/B-measured against the parent commit:** `do [1 (if
true [] [9 9])]`, the same under a body-local condition, the same one unit
further down (a nested `do`), and the `each`-body twin. Rows in
`lang/spec/control.tsv` §2.

**The asymmetry it did NOT explain, stated rather than smoothed over.** The
2-value-run twin `do [1 (if false [] [9 9])]` also answers identically on both
lanes — but it never reaches this seat: its `do` records no closure and falls
to the dyn-body BACKSTOP (a sub-engine over baked const tokens), which is an
interp entry, and the census caught it the moment I put the row in the corpus
(33 against a ceiling of 32). So the row is pinned in a Go test instead, with
that fact written down. WHY the 0-value arm records a closure and the 2-value
arm does not is a RECORD-stage question this increment neither changed nor
answered — `closureResidualExact` is where to start.

**What did not move.** The variation census is unchanged (pass 381, refused
57) and no ledger row graduated: the sweep's seeds and the frontier's rows do
not happen to contain this shape. A real widening with no ratchet to show for
it is worth saying out loud, because the temptation is to keep pulling until
some number moves — and the number that would have moved here was the
interp-entry census, in the wrong direction.

## A body unit gets the residual rebuild, and the gate that was waiting on it (2026-09-11, the fifty-fourth increment)

Two increments earlier, the per-unit fold was admitted inside a body only
over consts and locals, and the ledger said exactly why: *"a body unit has no
residual REBUILD … folding a permutation there turns a sound island into a
REFUSAL"*. That sentence was the whole specification for this increment.

**The change is two lines of policy and one of ordering.** `reconcileResults`
— the fn-unit caller of `seatResults` — takes the same `seatResidualRebuild`
fallback `seatProgramResidual` has had since the forty-third increment, and
`FoldFullStack` drops its `unit > 0` screen. Neither is useful without the
other: the rebuild alone is dead code, because nothing in a body unit
produced a permuted residual to seat.

**Two screens a fn RET needs that the program residual does not**, and they
are the reason this is not simply "call the same function":

- a RUNTIME-VARIABLE-count event (`eventFlags.variadicResult` — a loop, or a
  branch whose arms leave different counts). The rebuild spills one stack
  entry per operand, so one slot cannot stand for a run of a length the
  compiler does not know. The program residual ABSORBS such an event; a RET
  does not. This is what still refuses `do [def b true do [1 2 (if b []
  [9 9])]]`, and the twin-placement ledger's shape 3 is re-worded to say so:
  its graduation is a variadic-capable body-unit seat, not this.
- the residual shapes whose own post-processing reads the SEATED LAYOUT — a
  body-tail dynamic apply (`OpCallDynTrailTop` consumes the top N), a
  whole-frame replay (`OpCallDynFrame`), a RET-replay discipline. None may
  have the layout rearranged underneath it.

The CALLABLE screen carries over unchanged (`regionValsMayBeCallable` over
`rec.outOpsVals`, which is recorded for every unit including closures).

**The ordering bug this surfaced, which is the part worth reading.** The
first time the rebuild fired, the VM crashed: `index out of range [2] with
length 2`. A fn unit grew `cf.NLocals` from `flw.numLocals` BEFORE
`reconcileResults`, so the rebuild's spill temps were outside the frame. The
program's own write-back sits AFTER its reconciliation and carries a comment
naming the identical bug it was moved for — the correct ordering had been
derived once already, for the other unit, and the fn unit's copy of the step
did not get it. Recorded as NUR136, and fenced by walking EVERY unit's
`STORE_LOCAL`/`PUSH_LOCAL` against that unit's own `NLocals` rather than
pinning the one witness.

**What graduated.** `frontier-full-stack.tsv` is deleted — its last row and
two siblings are in `lang/spec/corpus-core.tsv`. The variation ledger's
`islanded` bucket is gone (the sweep reports **islanded=0** with no ledgered
bucket for it at all), four `result above a literal (Stage 3)` buckets
vanished with it, and the census moved pass 376 → 381, refused 62 → 57, twin
regime 10 → 9.

## The TYPE half of the arm-residency bridge: three problems, and only the first was plumbing (2026-09-11, the fifty-third increment)

The twin-placement cluster's shape 2 — `[10 20] each [drop def A (Integer gt
10) def B (Integer lt 20) def x:(A tand B) 15 x]` — is in the main corpus
now (`lang/spec/user-types.tsv`). Its ledger entry blamed one thing ("the
bridge pairs BindDef twins only"), which was true and was the least of it.
Three things had to be solved, and the measurements that produced each are
worth keeping because two of them reverse the obvious move.

**1. There is no def-site event to pair against.** Measured with a print in
`AdoptResidentTwins`: a type def inside an each body records NOTHING in the
unit's fragment. `[10 20] each [drop def ZA (Integer gt 5) 7]` records one
event, and it is the `drop`. A type install is a purely check-time product —
the mint happens once, the top-level twin replays it at its own stream
position, and the compiled stream carries no instruction for it — so there
was no site for the bridge to stamp.

`RecordTypeInstall` now writes one, and ONLY inside the arm-resident bracket
(`armResidentDepth`), which is the same discipline `RecordDynUndef` took for
the var-param teardown: outside the bracket it records nothing, so no other
lane's event stream changes. Every `BindTypeInstall` note goes through one
funnel (`Registry.NoteTypeInstall`) so a twin can never exist without its
event or vice versa — either way the bridge's total pairing declines and the
program refuses.

**2. Replaying the captured entry per element is WRONG, and only a
cross-request probe can see it.** This is the one that would have shipped. A
type binding has no runtime value, so the obvious op is the one
`OpBindTwin` already performs: push the captured entry. It compiles, the
driving request answers correctly on both lanes, and the corpus is green.
Then the parity oracle's install probe reads the name on a LATER request:

```
[10 20] each [drop def Big (Integer gt 5) 7]   both lanes [7 7]
Big                                             both lanes Big
undef Big                                       both lanes 1
Big                       interpreted Big   compiled  bytecode: internal:
                                                      unresolvable type operand Big
```

One `*Type` in two def-stack levels, both flagged `Minted`, so the first
`undef` retires the node out from under the level below it — `Retire` is
`delete(byID, ID)` with no reference count. The interpreter never meets this
because every element MINTS: N executions leave N distinct nodes. Recorded as
NUR135; the op re-installs the captured BODY instead
(`core.ApplyResidentTypeBind`), so each element mints its own node, exactly
as the install arm re-runs `InstallDef` rather than replaying a value.

Note what caught it: not the corpus, not the differential, not the variation
sweep — `bind_multirun_parity_test.go`, the cross-request oracle the design
review demanded before any placement work. It is the only lane that reads a
binding on a later request.

**2b. The replay cannot go through the installer's front door.** The first
cut of the fix called `InstallType` and failed on ELEMENT 0: `type: name part
"Big" in "Big" conflicts with an existing type name`. The rollback before a
compiled run restores BINDINGS and not registered name PARTS, and
`validateTypeName` skips the part check only when the name is currently a
type binding — so the replay is rejected on the check pass's own leftovers.
`InstallTypeBody` is `InstallType` past its two entry checks, and exists for
this caller alone. (Same NUR135: retirement and part registration are not one
reversible operation.)

**3. The screen is the actual increment.** An element-DEPENDENT type
expression is ordinary boru, measured:

```
[10 20] each [var [[e] def ZB (Integer gt e) 7]]
  def q:ZB 15 q   → type_error (15 fails against the TOP, `Integer gt 20`)
  undef ZB then def q:ZB 15 q → [15]  (it passes against the one below)
```

Two different nodes, observably. So the bridge must PROVE element-
independence before it stamps, and `typeInstallElementIndependent` does it
with two halves, each closing one route the element can take into the
expression:

- THROUGH A NAME — every binding the body installs is a twin in the bracket
  (that is the bridge's own premise, since a leftover twin declines), so the
  bracket's names ARE the body-bound set. A word from outside the body — a
  module-scope type, an enclosing fn's capture — is constant for the whole
  loop and passes: `def zh 5  [10 20] each [drop def ZH (Integer gt zh) 7]`
  compiles.
- THROUGH A DISPATCH that did not const-fold — such a word records an EVENT
  at its own position inside the unit, so an expression with no event in its
  token span folded from its own tokens alone. The converse, a word the check
  pass DID fold, is licensed everywhere else in this compiler: a folded
  call's value is baked.

The third route, reading the element off the VALUE STACK, is closed by the
LANGUAGE and not by the screen — a paren group binds forward-only, so
`[10 20] each [def ZG (Integer gt) 7]` and `(mk)` for a one-arg `mk` both
raise "no signature matches" on the interpreter. Measured; do not re-derive.

**A trap in the token walk.** `collectTokenSites` documented itself as
descending "a paren group and a nested body [which] are both list-shaped".
A paren group is NOT list-shaped — `AsList` declines a `ParenExprPayload` —
so the screen's first cut treated `(Integer gt 5)` as one opaque token and
declined the very row it exists to admit. The doc is corrected, and the walk
is NOT widened: `collectTokenSites` decides which twins `AdoptBodyTwins` may
adopt, and widening it would adopt twins noted inside a paren group, which
nothing has argued for. `collectExprSites` is the separate walk that
descends.

**What stays on the frontier.** The element-dependent row, now shape 2 of
`frontier-twin-placement.tsv`. Its graduation is an op that REBUILDS the type
per element — re-evaluating the expression against that element — rather
than re-installing one captured body. That is a genuinely different
mechanism, not a wider screen.

## The screen that was incomplete twice (2026-09-11, NUR137)

The fifty-fifth increment's own adversarial re-read found a silent miscompile
that had been in the branch for four increments, and the finding is worth
keeping for its SHAPE rather than its fix, which is six lines.

**What happened.** `regionReadsTheStack` decides whether a region's own event
consumes something already on the stack — if it does, a mark opened before
that event would sit above a value the event then pops. It switched on the
event kind and ended:

```go
default:
    ops = ev.call.ops
```

An `evBranch`'s operands live in `ev.br`. So for a branch the default read the
ZERO-VALUE `emitCall`, found nil ops, and answered "does not read the stack"
— for every branch region there is.

The forty-seventh increment had widened `variadicRegionEvent` to admit branch
regions, and its own doc says why, correctly: *"the plan's single-slot gate
was inherited from the two producers that happened to exist, not from
anything the mechanism needs."* True of the gate. Also true of the SCREEN,
and that half was not noticed.

```
def zs [0] def zt (zs 0 getr)  1 (if (zt gt 0) [] [9 9])
  interpreted  1 9 9
  compiled     9 1 9        silent, exit 0, the DEFAULT lane
```

Bisected: `6bc55db` (merge base) refused, `25b1af3` (46) refused, `d663fe1`
(47) answers `9 1 9`.

**Why it surfaced now.** The fifty-fifth increment gave a BODY unit the same
prefix plan, and inside a fn unit the same unscreened shape is not a wrong
answer but an internal one — `SEAT_BELOW_MARK prefix reaches past the mark`,
on the default lane, where increment 54 had refused cleanly. Probing my own
new seat with a condition the check pass cannot fold is what turned it up;
the corpus, the differential, the variation sweep and CI were all green
across it, because no row spells a branch region with an event condition.

**The lesson, and what the fix actually changes.** This is NUR133's mistake a
second time — a new region producer meeting a screen written for the
producers that happened to exist — and NUR133's own fix had left a comment
predicting it, in this exact function:

> A kind whose operands are not listed here does not "have none": it is
> unscreened.

The comment was right and did not prevent the recurrence, because the code
still silently produced an answer for an unnamed kind. A comment cannot hold
a line that the default case undercuts. So the fix is not a fifth `case`: the
DEFAULT now returns `true`. An event kind the screen does not name is assumed
to read the stack, which declines the plan. Adding a region producer now
costs a refusal until someone lists its operands — loud, and in the sound
direction.

**Generalise this when you meet the shape.** Two widenings in this line have
now admitted producers to a consumer without widening the consumer's screen.
If you widen a gate that admits event kinds, the question to ask is not "does
my new kind work" but "what does every OTHER path keyed on kind do with it" —
and where such a path has a default, make the default the safe answer before
you add the case.

## The kind-keyed audit, and a guard that had to be tested twice (2026-09-11, the fifty-sixth increment)

NUR137 was the second time a new EVENT KIND reached a predicate keyed on kind
and inherited whatever its default happened to do. So the increment after it
audited every `switch ev.kind` in `compiler/go` rather than fixing another
instance. The result is better than expected, and the numbers are worth
recording so nobody re-runs it:

| site | default | verdict |
|---|---|---|
| `lower.go lowerEvent` | `reason = "unknown event kind"` | **already right** — an unnamed kind REFUSES |
| `lower.go forEachOperand` | `default: // evBranch` | **the one finding** |
| `lower.go singleOutputCall`, `:3297`, `emit.go:12996` | `return false` | conservative |
| `lower.go:1765` (promotion walk) | `continue` | conservative |
| `emit.go regionReadsTheStack` | `return true` | fixed by NUR137 |
| `eventPos`, `eventDivergesDeep`, `eventsBindDynScope`, + 8 statement switches | no default | falls through to a safe nothing |

**The finding.** `forEachOperand` ended `default: // evBranch` and
dereferenced `ev.br`. It was correct only because every other kind is cased
above it. A new kind would either nil-deref — which ADR-005 forbids outright
— or contribute no operands, and an unvisited operand is an unreferenced one:
`planValueDefLocals` marks a live producer dead and the lowerer drops the
value. Same class as NUR137, wider blast radius. `evBranch` is an explicit
case now.

**The guard is the deliverable, not the case.** A comment did not hold this
line the first time: NUR133's fix left one *in the function that then broke*,
saying exactly what would go wrong. What holds a line is a test that FAILS
when the premise changes. `evKindEnd` is a sentinel one past the last kind,
and `TestEventKindCensus` asserts the census matches it — so adding a kind
fails a test whose message names every kind-keyed site that must learn about
it. `TestForEachOperandHandlesEveryKind` walks a well-formed event of every
kind through the operand walker.

**Four things that only came out by testing the guard itself, and the last
two came from review rather than from me.**

1. The census's FIRST form asserted `evBindTwin == len(evKinds)`. Simulating
   a new kind did not fail it — a kind added after `evBindTwin` does not
   change `evBindTwin`. A guard that does not guard is worse than none,
   because it is believed. Hence the sentinel.
2. `TestForEachOperandHandlesEveryKind` failed on `evTrap` at first, and the
   code was right: a trap's operands are its REMATCH window, and the fixture
   built a trap without one. The fixture was fixed, NOT the assertion — the
   assertion is what would catch a kind whose case visits nothing.
3. The site list was WRONG, and the way it was wrong is the same mistake one
   level up. The first cut named eight sites; review named three more
   (`eachClosureCap`, `childFragments`, `RewritePromotedRefs`); parsing the
   package found **nineteen**. All three of review's were in the audit above
   — they are "no default, falls through to a safe nothing" rows — but the
   audit is not what a future author reads. The FAILURE MESSAGE is, and it
   was pointing at less than half the surface. So the list is no longer
   maintained by hand: `TestKindKeyedSiteCensus` parses the package and
   fails if any `switch X.kind` sits in a function neither
   `eventKindSites` nor `operandKindSites` classifies (19 and 8
   respectively, and the split matters — the second keys on OPERAND kind,
   which a new event kind cannot reach).
4. `TestEventPosHandlesEveryKind` shipped with `default: continue` — a
   default that quietly does nothing while looking like coverage, which is
   verbatim the defect this increment exists to remove. Review caught it. It
   now fails on an unclassified kind, and the comment says what it used to
   do rather than quietly correcting it.

**The pattern under 3 and 4 is worth naming.** Both are the increment's own
subject applied to the increment's own guard, and both were found by someone
re-reading it rather than by the author who had just written the rule down.
Knowing the rule is not the same as satisfying it; that is what review is
for, and it is the second time on this line that a Codex pass has been the
thing that closed a class rather than an instance.

**The rule this leaves.** When you widen a gate that admits event kinds, the
question is not "does my new kind work here" but "what does every OTHER path
keyed on kind do with it". And when you write a guard against a class, spend
the extra minutes making it fail on purpose before trusting it.

## What the ledger excludes, and why each exclusion was measured

Each of these was arrived at by instrumenting and counting, not by reading.

| exclusion | reason | effect |
|---|---|---|
| `FnBodyDepth > 0` | a fn body's `def` is a FRAME-LOCAL, pushed per call and popped by the frame teardown, which never reaches `UninstallDef`; the compiled lane gives it a slot, not a registry binding | 69254 → 6110 |
| `!shadow` in `installDef` | `InstallFrameBinding` shadows rather than removes, so a macro or fn PARAMETER does not outlive the pass | 6110 → 5705, incoherence 274 → 2 |
| `BindDefReplace` as its own kind | `installDef`'s overlap filter DROPS the colliding entry before pushing, so net depth is unchanged; a twin must replace, not push | incoherence 2 → 0 |
| `RolledBackBodyDepth > 0` | an install inside any `keep=false` body is undone by that body's own truncation | 7455 → 7453 |

`RolledBackBodyDepth` is deliberately **wider** than `CondBodyDepth`: a
condition fragment is truncated too, even though it is not conditional.
What makes an install unrecordable is the truncation, not the
conditionality.

## Two lessons this line has already paid for

### A green gate over a corpus that cannot contain the case is not evidence

`TestBindLedgerDepthsCompose` reported a clean **0 incoherent over 7644
rows** while every top-level branch-arm `def` was being recorded TWICE at
the same depth — the arm's own speculative `installDef` plus the joined
binding `InstallJoinedDefs` leaves behind, straddling a truncation in
`runCarrierBodyDefsAdds` that is not itself a transition. A twin built
against that ledger would have pushed the binding twice, and the
differential would have reported it much later as a mysterious divergence.

The gate was silent because `lang/spec` contains exactly three
`if …[def …]` rows and **all three are inside fn bodies**, where
`FnBodyDepth` suppresses them. The one shape the census exists to size —
NUR110's own — was the one shape the corpus could not report on.

`TestBindLedgerBranchArmDepthsCompose` now supplies synthetic top-level
rows (both arms, one arm, a pre-existing outer binding, two `if`s in
sequence, a `while` body) and was verified to FAIL without the exclusion:
8 incoherent transitions across 6 rows. **When the population being
measured is a shape the corpus lacks, the gate has to supply it.**

### A count of call sites is not a count of transitions

This section of §6.5 already carried that lesson about `modules.go`
(eleven export-install sites, ZERO recorded transitions — they are
test-setup helpers). It then had to be applied to its own author: a review
found **five instrumentation sites missing, 1750 transitions, 23% of the
population** (5705 → 7455), and one row of the §6.5 site table was not
merely incomplete but false — it asserted `word_extend.go` has "no direct
`Defs` mutation, it reaches the table through `installDef`". Each missed
site bypassed the function the reading had assumed was the funnel:
`InstallJoinedDefs`, `word_extend.go`'s two direct pushes, capitalised
`undef` → `Defs.PopEntry`, signature undef → `UninstallFnSigs`, and
`installDef`'s module-wrapper rebind early return.

## Positions: three attempts, all three wrong for different reasons

A twin is emitted at its source position, so every ledger entry needs one.
Both obvious sources are wrong, and the next author will reach for them:

- The **value's own `Pos`** is the VALUE token. `def x 1` gives 1:7, the `1`.
- **`CurWordPos`** has already MOVED by install time whenever the body was
  analysed first: `def f fn [[x:Integer] [Integer] [def y x y]]` gives 1:34,
  the INNER `def`.
- Staging the site in `CheckState.PendingBindPos` and **clearing it on each
  note** lets a suppressed body-local note steal the position its enclosing
  `def` had already staged.

What works is **save/restore** around each install, in
`InstallAndRecordDef` — the only frame that knows the def site. Precedence
is staged-site → value position → `CurWordPos` as the floor for values that
carry none (an `undef`, a word extension, a fn body).

One limit, stated rather than papered over: a JOINED branch binding lands
on the `if`, because the join runs after that dispatch and the arm's own
`def` token is already gone. Revisit when the twin op needs a finer
position than the construct that produced the binding.

## Process rules this line earned the hard way

- **For anything touching `installDef` / `carrier_join` / `carrier_body` /
  `resolveOperand`, wait for the FULL `make test` before committing.** A
  revert (`4c06f5a`) came from pushing on a single passing test while the
  `lang/go` suite was still running.
- **Run `make -C kg graph` after editing any tracked design doc.** Two CI
  failures came from forgetting.
- **Never run a second coverage job alongside `make cover-gate`.** Doing so
  starved `TestServeStepShutdownDrains` (a 10s wall-clock bound on a
  signal-driven shutdown) and cost a full re-run to disprove. The clean
  re-run passed; the failure was self-inflicted load, and it is recorded
  here so the next person does not spend the same hour on it.
- CI runs `cover-gate-core` per PR, not the merged `cover-gate`. Both need
  to be green locally: a statement covered in the merged profile can be
  uncovered in the core-standalone one. `cover-gate-core` was red for two
  commits because core/go's own suite reaches `NoteBindTransition` only
  with check mode off, so every statement past its first guard was
  unreachable from that suite alone.
- **The pre-commit checklist is NOT what CI runs, and the gap is where two
  red builds in one batch came from (2026-09-10).** `make fmt && make vet &&
  make lint && make test && make cover-gate` misses six CI steps, each of
  which has failed at least once: `make -C kg verify`, `make -C lang/go
  lint-assertions`, `make parser-parity`, `make cover-gate-core` (the rule
  above — it was already written here and skipped anyway), the bytecode RACE
  gates, and the `borudebug` args-aliasing gates. Run them, or read
  `.github/workflows/ci.yml` before a push and run whatever it lists.
- **`make test` and `TestFrontier` cannot see a corpus-wide ceiling.** The
  module suites and the frontier ledger were green before increments 46-48
  were pushed, and `test/go/langspec` was red on four counts: a refusal in a
  corpus whose refusal ceiling is 0, two rows carrying an OpFallback island
  in one whose island ceiling is 0, three more carrying a `CallBoru` inside
  a handler that only the interp-entry census counts, and a diagnostic-parity
  ratchet. Every one of those lives in that one package, and only the FULL
  package run reaches them — `-run TestFrontier` does not. A batch that adds
  or moves a spec row is not gated until `test/go/langspec` has run whole.

## Constraints still in force

- **No third NUR110 refusal attempt.** Three are recorded in `NUR.md` as
  measured and rejected. The rule all three miss is that a branch-arm `def`
  LEAKS and SHADOWS and exists iff the body ran — which needs a third
  binding state, which is what the twins are for. NUR110 stays open until
  they land.
- Do not rebuild the broad region routing (breaks
  `TestEmitSplitFormsIdentical`).
- Do not relax `!fd.ArgsReversed` in `vmNativeApplicable` (a measured -7).
- The interpreter stays the reference oracle. Islanding interpretation
  inside compiled code is not acceptable, and the interpreter is not an
  escape hatch. The word island (the sixth increment) and the per-read
  deopt (the ninth and tenth) are the measured, bounded exception: they
  enter the interpreter only through the interp-entry census's existing
  site, only for a binding the check pass cannot type as a fn or as data
  (a gradual read the interpreter dispatches as a WORD when the value is
  a fn), and never to avoid a lowering the compiler could make — a
  refusal there would refuse the corpus's own `def n (m get "k")  n add
  1`, and a slot push there is a wrong answer.

## Where the tests live

| gate | what it pins |
|---|---|
| `test/go/langspec/bind_ledger_census_test.go` | the census, and that every entry names a real source site |
| `test/go/langspec/bind_replay_equivalence_test.go` | depth composition over the corpus, and over the synthetic branch-arm rows the corpus lacks |
| `test/go/langspec/bind_ledger_live_oracle_test.go` | the STRONG oracle: final ledger depth per name == the depth the pass left in the live registry, corpus + synthetics, no allowance |
| `test/go/langspec/bind_twin_emission_test.go` | the emission gate: every compiled Program's `BindTwins` table == the pass's ledger elementwise — what a recorder-lifecycle hole would break |
| `compiler/go/bind_twin_test.go` | `RecordBindTwin`'s contract directly: unconditional append (suspension must not filter), nil-receiver safe, the finalized Program carries a copy |
| `test/go/langspec/bind_twin_emission_test.go` (`TestBindTwinOpsArePlacedOrderedSubset`) | the placement gate: every OpBindTwin indexes a real entry, strictly increasing per unit; placement ⊆ table until the flip's refusal tightens it |
| `test/go/langspec/bind_replay_sandbox_test.go` | the sandbox's first client: rollback + ledger replay reproduces the pass-left registry over every push-only corpus row (depths per transition, entry identity per name) |
| `test/go/langspec/bind_twin_regime_test.go` | THE FLIP's lane (`BORU_TWIN_REGIME=1` via t.Setenv): regime-compiled vs fresh interpreter over the corpus, divergences gated at 0, refusals classified, compiled floor 6400; plus the hand-checkable smoke (concrete def, computed def, cross-request persistence, type install) |
| `core/go/bind_twin_apply_test.go` | `ApplyBindTwin`'s arms directly — each kind, the carrier-class skip, sig-undef identity match, `RestoreBindingsForReplay`'s pass-final module ledger |
| `compiler/go/bind_twin_test.go` (`TestFinalizeTwinRegimeStampAndPlacementGate`) | the regime stamp and the full-placement refusal (a table-only twin refuses; flag off tolerates) |
| `core/go/bind_ledger_note_test.go` | `NoteBindTransition`'s arms directly — suppressions, position precedence, kind/name/depth |
| `core/go/check_state_passend_test.go` | the pass-end cleanup seam (LIFO, exactly once, reset by `Begin`, Clone-isolated) and the `SuppressBindLedger` bracket |
| `check/go/narrow_passend_test.go` | the narrowing pop at pass end (popped on top, left alone when buried) and that `AnalyseLoopBody` ledgers its joined installs |
| `lang/go/narrow_leak_test.go` | the user-visible half of the narrowing leak: a later `Run` reads the bound Lock, not the leaked carrier |
| `eng/go/checkstate_lifecycle_test.go` | that every new `CheckState` field is classified reset-by-`Begin` or persistent |
| `core/go/engine_restep_note_test.go` | the RE-STEP note's arms (NUR124): which native results the pass notes, at which token, and every exclusion — quoted, concrete, a user fn's, a code-body word's, a pending forward's, a non-plain follower |
| `compiler/go/restep_deopt_test.go` | the recorder's note on the producing event, the planner's placement and declines (a resume outside the body, a deferred residual literal), the lowering's op and prefix, the strict refusal, and the prefix on NUR123's statement points |
| `eng/go/vm_deopt_test.go` (`TestReStepIfFnArms`) | the VM's re-step arm: plain results a no-op, a fn re-stepped as a token over the region and the prefix, the island's own error, the defensive arms |
| `lang/go/restep_deopt_test.go` | the timing family's parity and lowering (the op follows the swap), the siblings the note leaves alone, the closure family's parity (`TestClosureValueReStepParity`), and the open shapes pinned as measured |
| `core/go/engine_closure_bridge_test.go` / `eng/go/closure_bridge_test.go` | the closure VALUE bridge from both sides: a bridged closure dispatches over the stack and collects forward, a declined bridge / a quoted closure / a parked 0-arg lambda / a no-match stay data — as the PAYLOAD, never the bridge; the seam's declines, the Anonymous flag, and the closure's identity riding on the bridge |
| `core/go/fn_identity_test.go` / `lang/go/closure_identity_test.go` | the closure identity token: copies of one closure are one function (`dup eq` true on both lanes), constructions are distinct, a token-less payload is nothing, a bridged copy is eq to the closure either way round |
| `lang/go/gradual_apply_test.go` | `apply` over a gradual lead inside a unit: the parity rows (the W combinator's body and returned lambda, the fetched-fn apply, the no-match twins byte for byte), the runtime states the op defers (a lens, a 0-arg fn, a 2-arg fn) and the sound refusals (the main-program apply over a produced closure, a two-return body) |
| `compiler/go/word_read_test.go` (`TestRecordGradualApplyEventDeclines`) | `recordGradualApplyEvent`'s declines and its one positive arm (the event's flavour flags and position) |
| `eng/go/vm_seam7_test.go` (`TestSeam7CallDynApplyOneArms`, `TestSeam7CallDynApplyTopArms`) | the apply-word op arm by arm: one result commits, a 0-arg fn fires above the receiver (the tail form) and defers (the event form), a lens defers, data raises the interpreter's own `apply` no-match, `closureUnit`'s declines, the re-step's island error |
| `core/go/fn_identity_test.go` (`TestParenTrailingFnApply`) | the paren classification's trailing arm — a Dynamic last value is the lead exactly when the apply word holds it pending — and `MarkApplied`'s arms |
| `core/go/engine_stage5b_test.go` (`TestS5BCloseParenPendingGradualLead`) | the paren collapse over a pending gradual lead records the trailing apply with a GRADUAL out and collapses the window; with nothing pending the same window is kept |
| `compiler/go/plain_lambda_test.go` | `plainLambda`'s arms (a code body, a typed lambda, no contract, a pattern param) and that a code-body closure keeps its own count discipline |
| `eng/go/frame_name_test.go` | `nameFrameFns`: a lambda bound for a named param takes the name; an unnamed slot, a value already so named, a module wrapper and a compiled closure are left alone; `bindUnitLocals` names through it |
| `lang/go/closure_capture_test.go` | the family's parity (the bare-name apply, the gradual param, the `/v` read, the rename, a module wrapper dispatching), the sound refusals (the downstream apply, the gradual fn arg, the pattern lambda), and the module-wrapper render pinned open |
| `lang/go/produced_closure_apply_test.go` | the produced-closure apply family's parity AND that every row runs VM-native (the interp-entry hook sees no `vm:island` seam — the closure-arity fix) (K, W, C, I = S K K, Church true and false, the fetched-fn apply, two applies of one source, a token after the word, a deeper value, a paren, fn and lambda units, an inline fn literal) and its sound refusals (nothing beneath, a no-match beneath, the carrier lead, a produced closure over another, a two-arg closure over literals, the B row's two-value residual) |
| `compiler/go/produced_closure_apply_test.go` | `PendingClosureApply` (match by the sig body's array, a carrier entry skipped, an equal body in another array, no unit, a nil recorder), `producedFnValue` (a unit's closure, a fn-value apply's result, a native result), Finalize's refusal of a leftover pending apply |
| `compiler/go/emit_codebody_guard_test.go` (`TestArgIsProducedClosureArms`) | apply's one-arg overload over a concrete closure is exempt from the argument-slot refusal; the two-arg overload and a fn-typed carrier keep it |
| `lang/go/val_read_alias_test.go` | the thirty-first increment's parity (both Church pair rows, two pairs read twice each, the plain-fn and two-value twins, the def-read family on the apply word, a rebind, two closures of one source, both spellings, the data slot, the callee-side apply, the frame render) and its sound refusals (nothing beneath, a no-match beneath, a code body's read, a read in another unit, a conditional rebind, the nested replacing def with the interpreter's answer) |
| `compiler/go/val_read_alias_test.go` | `noteValBind` (no registry, a non-fn or unproduced value, the recorded producer/entry/generation/epoch, the conditional and rebind drops), `aliasValRead` (no name, registry or unit; no entry; a moved epoch; the unmoved binding; the same entry on top again; another entry; the name unbound), `NoteValRead` with no fn unit open, and `argIsProducedClosure`'s value-read skip |
| `compiler/go/zz_triage_split_check_test.go` (`TestStartFnCompileFinishPendingApply`) | a fn value beneath the pending apply is the window's argument; a mid-body pending apply still refuses |
| `check/go/pending_closure_apply_test.go` (`TestRecordUserCallOrApplyPendingFirst`) | the record site's order: the pending route before the name fallback, the fallback when nothing is pending |
| `core/go/engine_word_read_test.go` (`TestStepWordValNotesTheName`) | a `/v` read hands the recorder the binding's name beside the read's id |
| `core/go/check_fncarrier_test.go` (`TestInstallDefRefusesCapturingRedefinitionInFnBody`) | installDef's fn-body arm: a capturing redefinition inside a fn body refuses; a capture-free literal there and a capturing value at the top level do not |
| `lang/go/apply_data_receiver_test.go` | the thirty-second increment's parity (the native Church and/or rows, the U-combinator factorial, the numeral's `n/v` spelling), the two islanding Church rows pinned as islanded with parity, and its sound refusals with the interpreter's answers (the csucc row, its minimal shape, a fn value beneath a paren window with no apply word) |
| `compiler/go/zz_triage_from_check_test.go` (`TestRecordDynApplyDeclines`), `compiler/go/word_read_test.go` (`TestRecordGradualApplyEventDeclines`, `TestFnResidualReplayReasonArms`) | a fn-valued window entry records under the apply word and declines without it; a fn-valued receiver records; the read accounting runs under a tail apply (an uncredited read refuses, a credited one passes) |
| `lang/go/literal_read_test.go` | the thirty-third increment's parity (the read handed to a Function param, the arrow spelling, an unrelated bind between, the closure handed through, the value return and its render, the apply-word spelling, the top-level read) and its sound refusals with the interpreter's answers (a rebound capture, the literal redefined, the read in a branch arm, the CPS row) |
| `compiler/go/val_read_alias_test.go` (`TestNoteValBindLiteralArms`, `TestAliasValReadLiteralArms`) | the bind arm (a capturing anonymous or nameless literal records with its captures' epochs; capture-free, named or quoted records nothing) and the read arm (a moved capture epoch declines, an unbuildable closure declines and caches nothing, `readOps` resolves first) |
| `lang/go/arm_tail_apply_test.go` | the thirty-fourth increment's parity (both CPS rows, a then-arm apply, both arms applying, a two-value window inside the arm) and its sound refusal with the interpreter's answer (a pending fn with nothing beneath it in the arm) |
| `compiler/go/arm_tail_apply_test.go` | `ArmTailApply`: an inactive state, a residual of one, a top that is no pending apply and a window the recorder declines pass through; a pending fn over the arm's window records the apply event at the apply's position, consumes the entry and nets one gradual value |
| `core/go/recorder_stage5_test.go` | the inactive `ArmTailApply` passes the residual through |
| `eng/go/frame_name_test.go`, `eng/go/store_name_test.go` | a named closure is renamed by a frame binding and by a store; the same name is a no-op |
| `check/go/pending_closure_apply_test.go` | the record site's arms: the out and arg gates, the pending lookup, the recorder's decline, the freshened carrier (parent, Dynamic, a nil parent as Any), a fn-value out under a fresh id, the window reversal |
| `core/go/recorder_stage5_test.go` | the inactive `PendingClosureApply` default misses |
| `eng/go/vm_apply_closure_arity_test.go` | the apply op's closure arm: a closure of another arity (param slots, not `NParams`) takes the re-step and parks; the event form defers the parked pair — no compiling program reaches the arm, so it is pinned at the seam |
| `lang/go/tail_apply_collapse_test.go` | the tail-apply collapse's parity (both B rows, a def-bound closure whose tail applies its capture, two calls), its sound refusal (a gradual param beneath the tail apply), and the returned lambda's count contract: the unbound under-applying tail byte for byte, the def-bound form message-identical with the position pinned open (NUR122) |
| `check/go/tail_apply_collapse_test.go` | `collapseTailApply`'s arms (no tail apply, a residual shorter or wider than the window, a data top, the window to one gradual result, a fn value on top) and `collapseElidedTailApply`'s (a tuple contract, an empty body, a one-value residual, another last word, a data top, the collapse) |
| `compiler/go/unit_tail_apply_test.go` | `UnitTailApply`: a nil recorder, a unit out of range, no tail apply, the window width |
| `compiler/go/store_name_test.go` | `seatStoreName`: no recorder, no table, a slot no def named, the name at the store's pc |
| `eng/go/store_name_test.go` | `storeNameAt` (the main code's and a unit's table, an empty name, an unseated pc, a unit beyond the program) and `nameStoredClosure` (a plain value, a named closure, a known unit's RetName and render, a unit beyond the program, a unit the bridge cannot describe) |
| `lang/go/function_slot_test.go` | the Function-slot family's parity (both `cnot` rows, the cif7 twin that ran cnot's body, a produced closure at a plain fn's Function param applied, paren-bounded, held as data, and the frame render) and its sound refusals (an Any slot, apply's Function slot over a carrier) |
| `compiler/go/emit_codebody_guard_test.go` (`TestArgIsProducedClosureArms`) | a declared Function param and a positional Function slot are exempt; an Any slot, a nil signature and apply's own Function slot over a carrier keep the refusal |
| `eng/go/frame_name_test.go` | a compiled closure bound for a named param is named (its render kept when the payload names no unit); a capture slot is left alone |
| `lang/go/def_computed_fn_test.go` | the thirty-fifth increment's parity (the eight fn-util rows, the event spelling, the bare read, a read with a following statement, the apply feeding a dispatch, two reads, a 0-param wrapper, the wrapper's own error at the read's caret) and its sound refusals with the interpreter's answers (the stack form, a param read in a fn body, a paren in the window, the survivor inside a paren, an overloaded flip operand, and the two statement-window rows whose interpreter answer is the signature_error) |
| `check/go/fn_read_arrival_test.go` | `tryShapedFnReadArrival`: the consumed window (the def name, the read carrier, one dynamic result, the extra token left), the arity-0 read, every refusal with its reason (a window past the tape end, a statement end inside it, a word inside it, an operand with no compiled home), and the silent declines (quoted, no id, unread, unclaimed, not a fn carrier); `tryShapedFnReadWindow`, the plain-check half: the collapsed window and its declines (quoted, not def-bound, no claim, short, non-fixed, past the end) |
| `core/go/fn_shape_test.go` | `IsSelfContainedGoFnDef`'s arms and `NoteFnShape` / `FnShapeArity` (inactive, no id, a negative count, the claim, a 0 claim, the clone, the reset) |
| `eng/go/vm_self_contained_fn_test.go` | the own-signature apply under a registered word of the same label (leading, the method op, the declined arg count), `callDynMethod`'s modifier-chain retry (a flipped delegation and its error), and a handler error anchored on the value's token text |
| `compiler/go/fn_shape_claim_test.go` | the claim as `producerReturnedClosureArity`'s third source (claimed, unclaimed, no registry) and `DefReadName` |
| `core/go/check_fncarrier_test.go` (`TestStepWordPlainCheckSubstitutesCarrier`), `lang/go/def_computed_fn_test.go` (`TestDefComputedFnPlainCheckClean`) | the plain check reads a bound fn carrier with no undefined_word and no unused_def, and the silent-fallback mark stays a compile pass's |
| `lang/go/closure_read_model_test.go` | the thirty-sixth increment's parity (the typeof operand, the repeated reads, the `mk2` chain, the fn-util curry chain, three curry levels, the plain read, a read whose window is its statement, a two-param closure's window, a capturing closure read twice) and its sound refusals with the interpreter's answers (the survivor inside a paren, the two flattened-window spellings whose interpreter answer is the signature_error), and the filter-body twin pinned as islanded with parity |
| `compiler/go/fn_shape_claim_test.go` (`TestClosureOpShapeArms`, `TestNoteClosureShapeBindArms`) | the closure shape (a factory of factories claims the chain, a two-value body has no result, a unit beyond the program, an event, the bounded recursion, a const lambda, a const beyond the table) and the claim at the def (no registry, a plain value, the unit's arity, a standing claim kept, an unproduced carrier) |
| `check/go/fn_read_arrival_test.go` (`TestShapedFnReadResultShape`) | a claim with a result shape makes the read's result a Function carrier carrying the next level, on both halves |
| `lang/go/modules/fn_test.go` (`TestFnShapeFromOperandArms`) | curry's chain claim (three unary levels over three params, no claim over a unary fn or no operand) |
| `lang/go/modules/fn_test.go` (`TestFnShapeReturnsClaims`, `TestFnShapeFromOperandArms`) | the ReturnsFn mints the carrier and claims a constant, claims nothing for an unknown arity, mints alone with no registry; the operand arms (partial's slot, memoize's count, no operand, a non-fn, a carrier, an overload) |
| `lang/go/while_compile_test.go` | the thirty-seventh increment's parity (the falsy condition, `break`, the truthiness read, the flex counter, a two-arm if over the enclosing computation, a carried rebind read by the condition and after the loop, zero iterations, a computed condition over the carried slot, `break` and `continue` discarding the round, a `continue` and a `break` inside a branch arm, a fn-body while over a param, a fn body rebinding a module def, a nested while, two carried rebinds) and its sound refusals (the empty condition with the interpreter's runtime_error, a two-value condition, a multi-value body with a rebind) |
| `compiler/go/while_record_test.go` | `RecordWhile`'s arms (inactive, a missing fragment, a condition netting zero or two values, a condition of unknown provenance, the recorded loop: the consts 0/1/MaxInt64, the condition fragment and its out, the scratch iterator) and the loop traversals visiting the condition (`childFragments`, `fragmentOuts`, `forEachFragmentOperand`, `fragmentResultSeqs`) |
| `lang/go/nested_body_fn_carrier_test.go` | the thirty-eighth increment's parity (a `do` body's read, consumed downstream, two reads, a multi-value body, both branch arms, an arm-local def, a loop body, a body-local def, a while body, an args-bearing `do` body in a fn, a data-list read inside a `do`), the plain check clean of undefined_word / unused_def on the nested reads (an unbound name still flagged), and the sound refusals (a lambda factory's concrete closure in a `do` body and a branch arm, the stack-form `each`, a two-value arm) |
| `core/go/check_fncarrier_test.go` (`TestStepWordNestedBodySubstitutesCarrier`) | the substitution fires at NestedBodyDepth > 0 with no diagnostic and no compile-only mark |
| `compiler/go/emit_codebody_guard_test.go` (`TestRecordDynBindNotesOnlyConcreteClosures`) | `RecordDynBind` notes a concrete produced closure for the code-body gate and not a carrier-bound one |
| `lang/go/while_compile_test.go` (`TestWhileEmptyConditionTraps`, `TestWhileNonEmptyConditionDoesNotTrap`) | the forty-second increment: the empty condition compiles to a terminal trap with the interpreter's own error (with a prefix before it, and whatever the body is), a condition WITH tokens never traps, and the empty condition below the top level keeps the arity refusal |
| `lang/go/residual_rebuild_test.go` | the forty-third increment's parity (the permuting roll, `swap`, three results rotated both ways, a duplicated result, a dropped one, an inert value beneath a result, non-Integer results), that an in-order residual still spills nothing, and the frame-count pin (`TestResidualRebuildFrameCountsTheSpills`) |
| `compiler/go/residual_rebuild_test.go` | `seatResidualRebuild`'s seam: the emitted spill/re-push stream for a permutation, one temp read twice for a duplicate, a dropped entry, and the four declines (an empty sim, an operand absent from it, a result index absent from it, a variadic region, each of the three armed mark plans — each emitting nothing) |
| `lang/go/foreach_closure_test.go` | the forty-fourth increment's parity (the Function form, a side-effecting fn value, the quotation twin, a lambda over a list, the empty body, a value-netting body, an empty collection, both map forms), that the body lowers to its own closure unit, the measured lambda convention on both lanes, and the ambiguous-overload refusal that `CrossCollectionTokenShape` would have (wrongly) lifted |
| `lang/go/residual_rebuild_test.go` (`TestShuffledClosureRefusesAndTheInterpreterApplies`, `TestShuffledFnReadStillCompiles`) | NUR131: the four fold witnesses and the two rebuild-screen shapes refusing with the interpreter's answers, and the def-bound `/v` shuffles plus a non-callable event pair still compiling |
| `compiler/go/residual_rebuild_test.go` (`TestSeatProgramResidualScreensACallable`) | the caller's screen at the seam: a non-callable residual rebuilds, a Function-typed one and a Dynamic one keep the seating's refusal and emit nothing |
| `lang/go/deferred_rematch_test.go` | the forty-sixth increment: both ledger rows compiling to a byte-identical raise (and with a prefix before them), the deferring arm (the flex witnesses, the lens spellings that always matched), and the reorder-hint window keeping its refusal with the interpreter's own hint |
| `lang/go/region_prefix_test.go` (`TestRegionPrefixSeatsAMultiSeatRegion`, `TestMultiSeatRegionEmitsTheMarkAndSeat`) | the forty-seventh increment: both arms of the branch-variant do region under a prefix, a two-value prefix, a non-Integer prefix, and the emitted stream — mark before the region, seat after it, no STORE_LOCAL |
| `compiler/go/region_prefix_test.go` (`TestVariadicRegionEventAdmitsTheDoCatch`) | the two predicates side by side: a do-catch is a region for the PREFIX and not for the COLLECT, a loop is both, a fixed-arity call is neither, and a possibly-callable region is out of both (NUR129) |
| `lang/go/bytecode_do_error_arity_test.go` (`TestMaybeRaisingZeroNettingHandlerIsARegion`, `TestRegionHandlerRefusesAFixedSeatConsumer`) | the forty-eighth increment: both runtime arms of the maybe-raising handler through one lowering, the proven-raise twin keeping its fixed arity, a one-netting handler unaffected, and the fixed-seat consumer's byte-identical def_error |
| `lang/go/uncalled_dispatch_trap_test.go` | the forty-ninth increment: both fn-util ledger rows and two module-native twins raising byte-identically through the trap (with a prefix before one), the emitted terminal TRAP with no island, the two inexact-operand declines keeping their refusal, and the plain check still reporting uncalled_function at error severity |
| `core/go/uncalled_dispatch_definite_test.go` | `uncalledDispatchDefinite` arm by arm: concrete consts and an empty candidate list admit; a carrier, a dynamic, an undefined placeholder, a raw word token, an open paren, a paren expr, a reach and a template string decline |
| `lang/go/loop_flow_trim_test.go` | the fiftieth increment (NUR132): every witness of the round that survived its own break/continue, the rows that always agreed, and the nested-loop program that never terminated because a break did not pop its loop |
| `compiler/go/loop_flow_signal_test.go` | the seam: `lowerBreak` / `lowerContinue` emit exactly one FLOW signal op carrying no target, in a loop and from a fn unit alike, and a break with neither refuses and emits nothing |
| `lang/go/diverging_arm_lowering_test.go` | the fifty-first increment: the last frontier-while row and its `for` twins compiling with parity, the stack-supplied layout's value-netting twin with them, both other divergence kinds (raise, and a fn body's return-count contract — position excluded per NUR118), and the 0-netting arm keeping its refusal under the message that now says what it means |
| `lang/go/region_stack_read_test.go` | NUR133: the three review witnesses refusing with the interpreter's answers — a callable passed through an island's run, a fallback input beneath its own mark, a variadic callee's result promoted to one slot — and the const-argument twin still seated natively by OpSeatBelowMark |
| `lang/go/codebody_fold_test.go` | the fifty-second increment: the ledger row and its family running NATIVE (no island) with parity, `depth` counting the body's own stack including the element, the top-level rows still folding, and NUR131's callable screen still refusing |
