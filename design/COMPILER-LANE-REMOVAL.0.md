# COMPILER-LANE-REMOVAL.0 — the interpreter is the one execution path

**Status:** Landed 2026-10-16. The direction the maintainer set on
2026-10-16: *merge what you have; branch `compiler-legacy`; then remove the
compiler lane — we will focus on optimising the interpreter.* This note
records what was removed, what was kept and why, how the removal was shown
to preserve behaviour, and what is left to prune.

## 1. What the compiler lane was

From 2026-07 to 2026-10 boru carried two execution lanes. A program was
parsed, run through the check pass with a **recorder** armed
(`compiler/go`: `EmitState`, the lowering, the bytecode emitter), and the
recorded program was executed on a bytecode VM (`eng/go`: `vm.go`, the
run / fork entry points, `Vm.compile`); the tree-walking interpreter in
`core/go` was the oracle the compiled lane was measured against (the
langspec differential gates, the frontier ledger, the interpreter-entry
census). The full-compilation programme (design/legacy/FULL-COMPILATION.0.ignore
and its handoff, review, re-plan and assessment) aimed at making the compiled
lane total.

On 2026-10-16 the lane was removed outright. The interpreter had meanwhile
become the faster path to optimise — the fn-call paths inline on the
caller's tape (#532), collection words as loops on the tape and container
literals as sealed regions (#533), the Value no-copy policy through phase
4 (#535) — and the compiled lane's debt (its refusals, its own
miscompiles, a second implementation of every control word) was effort the
interpreter line could not spend twice. The design the interpreter line
follows is design/INTERPRETER-TIERED-EXECUTION.0.md; its handover page is
design/INTERPRETER-PERF-HANDOVER.0.md; its baseline discipline is
design/PERF-BASELINE.10.md.

## 2. What was removed

- **Modules**: `compiler/go`, `eng/go`, `tools/piecetool` (the module-cut
  tool, whose remaining use was the facades), `test/go/vary` (the compiled
  lane's variation harness). `go.work` and every `go.mod` no longer name
  them.
- **core/go**: `CallableSpec`, `BodyOutResidual`, `StoredBodySpec`,
  `Signature.Callable` / `.StoredBodies`, `NativeFunc.Callable` /
  `.StoredBodies` — the per-slot compile declarations — and the
  `sole.Callable != nil` clause of `SingleOverloadRecoverable`.
- **check/go**: the S3 seam (`DispatchBraid`, `UserPolyPlan`, the
  `inactiveDispatch*` defaults and their forwarders). Two things the
  recorder used to supply through it are now plain analysis here:
  `tryFoldScalarConst` (`scalar_fold.go`) and `AnalyseCodeEffectCarrier`
  (`code_effect.go`).
- **basic/go, lang/go**: every `Callable:` block, every `Record*` /
  `Note*` reach into the recorder, the compiled closure arms
  (`IsCompiledClosure`, `ClosureAsFnDef`, `InvokeCallbackBody`,
  `wrapCompiledClosure`), `Program`, `CompileCheck`, `RunCompiled*`,
  `SetCallSiteSpecialisation`, `SetInterpAttribution`, the `-compile-report`
  machinery, the `boru:vm` module's `compile` word, the `--emit` flag of
  `boru check`, the compile pre-flight of `boru build`, and the compile
  report of `boru run`. `Run` is the interpreter; `RunInterp` and
  `RunInterpValues` remain as spellings of it.
- **Gates and tooling**: the differential, compile-or-fallback, frontier,
  census, declaration-census, sweep and bytecode-combination tests of
  `test/go/langspec` and their ledgers (`compile_failures.tsv`,
  `COMPILED_STATUS.md`, `SWEEP_STATUS.md`); the `fail-compile` and
  `compiler-mismatch` buckets of the generated syntax matrix (specgen's
  `-compile-out` / `-mismatch-out` / `-vary*`); `make verify-bytecode`,
  `fuzz-bytecode`, `status`, `sweep-status`, `handler-worklist`,
  `cover-gate-eng`, `cover-gate-compiler`, `publish-eng`; the `borudebug`
  build tag (its CI job in `.github/workflows/ci.yml` runs a no-op step
  until the workflow file is edited by a credential with the workflow
  scope — the removal PR could not push one; the intended workflow files
  are staged in `ci/`, whose README says how to apply them); the compiled
  rows of the race gate. The direction lane now holds one gate,
  `TestCheckTypeSoundness`.
- **Documents**: the full-compilation programme's pages and the handovers
  written for it were moved to `design/legacy/` as `.ignore` files
  (FULL-COMPILATION*, COMPILABLE-SUBSET, DO-STRUCTURE-COMPILATION,
  HANDLER-MIGRATION-LINE, AOT-COMPILE, NUR-RUN-HANDOFF, NUR-ROUND6-HANDOVER,
  ENG-COVERAGE-PARITY, VOXGIG-BORU-HANDOVER, and the previous
  SESSION-HANDOVER as SESSION-HANDOVER-FULL-COMPILATION). The old kernel
  guide, eng/go's CLAUDE.md, was folded into `core/go/CLAUDE.md` without
  its compiler passages. NUR.md's records that cite the compiled lane are
  history and were not rewritten.

## 3. Why plain `boru run` and `boru check` did not change

On the plain pass the recorder was core's **inactive** no-op — `Active()`
and `Armed()` false, every `Record*` / `Note*` a no-op — and
`Check.Compiling` was false, so every reach into the recorder
constant-folded to the plain pass's value. Deleting a reach therefore
deletes a branch the plain pass never took. The two exceptions — the
scalar constant fold and the code-effect analysis, which the check pass
consumed through the seam even with the recorder inactive — were ported
into `check/go` as plain functions before the seam went. The corpus
agrees: every langspec gate, every module suite, the real-program suite
(`TestRealProgramsRun`) and the generated syntax matrix pass on the
interpreter alone, and `make -C kg graph` produces the committed bundle
byte for byte.

ADR-008 (100% coverage of every reachable statement) is what shaped the
cut: a dead arm cannot be stubbed, it has to go, which is why the removal
reached into every handler that had a compiled twin rather than
switching the lane off.

## 4. Follow-ups — residue to prune, not to build on

What remains of the recording pass in core, each a candidate for deletion
in its own change:

- `BeginCompilePass` and the compile-pass fields `CheckState` still
  carries; `Registry.Effects` / `NoteEffect` / `EffectLedger`.
- The `CompileEffect` flags on signatures (~100 registrations); the check
  pass still reads `CompileScalarFold`, `CompileDynBody` and
  `CompileFallbackBody`, so these go when check stops reading them.
- `RunCarrierArmBody`, `PendingResidue`, `armResidue`, `Engine.ArmBody`,
  `pendingResidueOf` (`pending_literal.go`); `VMDefer` / `IsVMDefer`;
  `TypedBindSpec`, `RunTypedBind`, `RunTypedBindCons`, `RunTypeInstall`;
  the Go-side loop drive (`Invoker`, `StartLoop`, `InvokeBody`), reached
  now only by the fixtures; `TryRecordRecoveredUserFn` (a rename).
- `RuntimeMirror` and `InheritObserveHooks` stay: the check pass and the
  langspec false-positive gate read the first, and the second has coverage.
- `lang/go/native/aliases.go` still re-exports a few names only the lane
  needed; the `unused` lint is the guide.

The first sweep with that guide (`golangci-lint run --tests=false`, which
reports a helper only tests still reach) ran in the removal's own PR,
once the standalone gates were back at 100%: it deleted the thirteen core
helpers the lane's callers had orphaned — the written-window rematch
(`rematchWritten`, `rematchWrittenSplit`, `withGradualWrittenOperands`,
`forwardReach`), the poly reach bound (`polyReachBound`,
`polyReachUnboundable`, `mapTupleToWindow`), the paren-trailing-fn
probes (`parenFeedsPendingForward`, `trailingFnCollectsPastClose`),
`undecidedPatternWindow`, `plainReStepToken`, `concreteFnSingleReturn`
and `containsFlexOrStore` — and nine in check, all callers of which sat
inside the armed-recorder branches: `narrowToDeclaredParam`,
`typedContainerCarrier`, `quoteParamCarrierBind`, `disjunctCombosTakeSig`,
`anyDisjunctCarrier`, `anyImpreciseCarrier`, `widestSatisfiableOverload`,
`soleSigParamsNominal` and `fnDefName`, with the tests that drove them.
The two core helpers main already carried dead (`boruErrSite`,
`returnTypeError`) and the `basic_shim.go` aliases, which exist for the
word files and the suites above, were left alone.

Beyond core: the TypeScript twin `eng/ts` still carries its own compiled
lane (`compile.ts`, `emit.ts`, `lower.ts`, `bytecode.ts`, `vm.ts`) over the
`eng/spec` corpus — `make test-ts` is unchanged and that removal is a
separate decision; `design/RELOAD-INVALIDATION.0.md` and
`design/BIDIRECTIONAL-CHECKER.0.md` keep proposals whose compiled halves
are now moot; `lang/spec/module-composition.tsv` §6 keeps the rows the
compiled lane could not answer, as the interpreter's to keep answering.

## 5. Getting the code back

The last commit before the removal is on the `compiler-legacy` branch
(`git checkout compiler-legacy`): the four removed modules, the lane's
gates and the design pages as they stood. Nothing on `main` depends on it.
