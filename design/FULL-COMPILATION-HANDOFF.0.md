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
