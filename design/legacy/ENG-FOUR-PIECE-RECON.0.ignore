# Refactor Recon Synthesis: splitting `eng/go` into eng(VM) → compiler → check → core

Target dependency chain: **eng (VM) → compiler → check → core**, with core a standalone pure interpreter. All paths below are relative to `/home/user/boru/eng/go/`.

## 0. Verification notes (disagreements resolved by spot-reading)

| Dispute | Verdict after reading |
|---|---|
| `emit_recorder.go` "primary: check" vs "compiler contract" | **Split/re-cut.** Read in full: the interface is deliberately package-sealed (17 unexported methods, doc lines 22–24), and its signatures name compiler types (`*Program`, `*EmitFragment`, `emitOperand`, `emitCheckpoint`, `TypedBindSpec`, `PolyNoMatchSpec`, `FallbackSpan`, `BranchRecord`). It cannot move to check as-is. See seam S2. |
| `effects.go` "primary: eng" | **Splits.** Verified: `EffectLedger`/`Note`/`Count`/`Registry.NoteEffect` are dependency-free and called by lang runtime words during plain interpretation → **core**. `ArmEffectFence` + `noteEffectWriter` → **eng**. |
| `typed_bind.go` "eng" | **Core.** `RunTypedBind` duplicates `defTypedHandler`'s validate/reparent using only core helpers; making it the shared core routine erases the duplicated error-string contract. `TypedBindSpec` moves to check (spec types, S2). |
| `coverage.go` `noteVMCoverage` "eng" | **Stays core.** It's a method on `*Registry` (core receiver — Go forbids cross-package methods) reading only core fields + `[]SrcPos`. eng→core call, legal direction. |
| `engine.go:9462-9466` check→compiler call | **Confirmed verbatim**: `if es := e.registry.Check.Recorder(); es.active() { … tryCompileUserPolyArms(…) … es.RecordUserPolyCall(…) }` — check-recovery code driving compiler arm-baking. Family F6. |
| `Registry` struct field mix | Confirmed at registry.go:140–260: `Check *CheckState`, `Invoker`, `nestedRunner func(ref *CompiledFnRef, …)`, `vmRunning int32`, `runtimeStamping`, `stampLog` all on the core Registry. |
| `type Type = Value` | Confirmed, `typetable.go:55`. |
| Parser | `eng/go/parser/` is already a separate package importing only core surface (no Check/Emit/Carrier usage found). |
| specfix | `eng/go/specfix/` uses check **and** compiler surface (`CheckDiagnostic`, `AnalyseLoopBody`, `ApplyGuardNarrowing`, `BranchRecord`, `CompileEffect` flags, `CallableSpec`) — must sit above all four pieces. |
| Files no reader covered | `eng_rest_seam9.go` (test seams → core, split per-package later), `coretype_format_behaviors.go` (empty stub, delete), `coretype_list_map_behaviors.go` (List/Map format behaviors → core). |

Measured counts: 97 `Check.` references in engine.go alone; 48 `Check.IsActive()` sites package-wide; 88 `Recorder()` call sites; **13 concrete `.(*EmitState)` asserts outside the emit cluster** (carrier.go×4, core_helpers.go×2, engine.go×2, method_shape.go×2, drift_window.go, stamp_runtime.go, emit_recorder.go); ~80 `.Carrier` flag reads outside carrier.go.

---

## 1. File manifest

### core (pure interpreter — standalone)
Whole files: `argsstack.go`, `boru_error.go`, `canon.go`, `capability.go`, `clone.go`, `compare_deqkey.go`, `compare_scalar_behaviors.go`, `compare_types.go`, `contextstack.go`, `convert_ideal.go`, `core_flex.go`, `core_ref.go` (minus one hook, F7), `core_xml.go`, `coretype_list_map_behaviors.go`, `deftable.go`, `define_type.go`, `depscalar.go` (minus the `RememberOriginal` block → hook), `didyoumean.go`, `dispatch_explain.go`, `errorcodes.go` (code strings stay; ownership note only), `exit_error.go`, `flowctrl.go`, `fn_capture.go`, `fn_def.go`, `fn_frame.go`, `fn_frame_elide.go`, `fn_frame_probe.go`, `fn_params.go`, `fnsig.go`, `fork.go`, `generics.go`, `generics_instantiate.go`, `get_words.go`, `gobridge.go`, `ideal.go`, `keyval.go`, `macro_expand.go`, `match.go`, `member_behavior.go`, `membership.go`, `micron_kernel.go`, `module_gate.go`, `modules.go`, `moduletype.go`, `nativefunc.go`, `nodify.go`, `payload.go`, `print.go`, `process.go`, `resolve.go`, `return_check_msg.go`, `shape.go`, `sigimpl.go` (Compiled slot → opaque handle), `size.go`, `storage_helpers.go`, `sugar.go`, `surface.go`, `table.go`, `tape.go`, `trace.go`, `truthy.go`, `typebehavior.go`, `typed_bind.go` (moved in), `types.go`, `typetable.go`, `unify*.go` (all 13), `util.go` (minus predicateSandbox check-snapshot → hook), `weak_flex.go`, `word_name.go`, `xml_interp.go`, `diag_msg.go`, `word_extend.go` (minus 2 hooks), `compare.go` (minus `OrderingReturnsFn`), `equal.go` (StoreShapeInfo arms via opaque payload identity), `value.go` (bulk; see splits).

### check
Whole files: `carrier.go` (bulk), `check.go` (minus `BeginCompilePass`/`IsolateEmit`), `drypass.go`, `indexcheck.go`, `deadsig.go`, `guard_predicate.go`, `store_shape.go` (payload shell issue, S6), `module_export_growth.go`, `method_shape.go` (minus EmitState recording halves), plus extracted regions listed under Splits, plus `RenderCheckDiagnostic`/`severityStyle`, `CheckState`+`CheckDiagnostic`+severity table (out of registry.go), `CheckMakeConstruction`, `Tor/Tand/TnotReturnsFn`, `OrderingReturnsFn`, `GenBindingCarrier`, the re-cut `EmitRecorder` interface + `inactiveEmit`, and the spec types (`TypedBindSpec`, `FallbackSpan`, `PolyNoMatchSpec`, `BranchRecord` — "check proves" artifacts).

### compiler
`emit.go` (minus `ModuleScopeBinding` → core), `lower.go` (cleanest file), `bytecode.go` (minus `ClosurePayload`/`CompiledFnRef` runtime-value machinery → core-opaque, S5), `callable_words.go`, `compile_sandbox.go` (via exported core snapshot API, S10), `stamp_report.go`, `stamp_runtime.go`, `drift_window.go` (off the `*Engine` receiver, via hook), `user_poly.go` (invoked via check hook), the `recordDispatchOutcome` + `tryRecord*/tryFold*` family extracted from carrier.go, `BeginCompilePass`/`IsolateEmit` as compiler-side entry points wrapping check's `Begin`.

### eng (VM)
`vm.go` (minus core-predicate extractions: `tapeCoupled`, `isAppliableFn`, `isDelegationFnDef`, and the contract checkers `checkParamContract`/`checkReturnContract`/`guardArgs` → shared core routines), `vm_args_debug.go`, `vm_args_release.go`, `vm_markwindow.go`, `vm_poly_nomatch.go`, `vm_rematch.go`, `runIslandResolved` (out of engine_pool.go), `vmDefer`/`vmDeferAlt` (out of interp_entry.go), `ArmEffectFence`+`noteEffectWriter` (out of effects.go), `invokeCompiledUnit`/the VM half of `InvokeCallback` (out of invoke.go, behind seam S4).

### Above all pieces (unchanged homes)
`parser/` (depends only on core), `specfix/` (test scaffolding over the full facade), lang/basic layers.

### Files that SPLIT — the function boundaries

| File | core keeps | check gets | compiler gets | eng gets |
|---|---|---|---|---|
| **engine.go** (9972 ln) | Run/step loop, stepWord, forward collection (1712–2321), execMatch splicing, marks/moves/flow (7307–7644), stepEnd/DefCleanup, curry probes, scan sugar, policy gates, barrier commit, error/hint helpers | the entire 9067–9972 recovery region (`checkModeFallbackPositions/SurfaceShape/AssumeSig`, `spliceCheckResults`…), `tagCheckModeDefRead`, `checkMixedFormAdvisories`, `checkForwardStrandsOperand`, `refuseForwardStackDrift`, `refuseStrandedMemberFn`, `spliceAnonCheckResult`, `spliceFnValueCheckResult`, `spliceFnCheckTail`, `shareCheckState(From)`, `noteSpeculativeBarrierCommit`, `checkModeParenFnCollapse`, `drainUndefinedAtoms`, `undefinedWordCheckDiag`, `valueCarriesCarrier`/`exprRefsCarrier`, `concreteEvalOnce` | paren-window emit classification (`recordParenLeadingApply`, `parenLeadFnApplyIdx`, `recordParenLeadFnApply`, the 8071–8156 block), `tryRecordRecoveredUserFn`, `tryRecordUnmatchedDispatchTrap`, inline Record* blocks (3661, 4150, 4545, 4593, 4724, 5159) | — (stackform `Recorder`/`RecorderSkipper` observer interface stays core) |
| **carrier.go** | `CommonAncestorType`, `DataListElemTypeFromValue`, sentinel scans, `calleeValueLeaksFlow`, `resolveTypeNameArgs` | carriers/joins/narrowing, `StripToCarriers`, `carrierResults` (type half), `AnalyseLoopBody`/`AnalyseFnBody`, guard narrowing | `recordDispatchOutcome` + all `tryRecord*/tryFold*`, `specialWordResults` emit arm, `RecordTypedDefMake`, `stripZeroOutResiduals` | — |
| **registry.go** | Registry struct (fields re-cut per S1/S4/S8), Lookup, RunPredicate (hooks), engine-lifecycle plumbing | `CheckState`, `CheckSeverity`+table, `SeverityFor`, `CheckDiagnostic`, `DefaultCheckStepBudget` (~600 ln) | `runtimeStamping`/`stampLog` (→ compiler-owned side table) | `Invoker`/`nestedRunner`/`vmRunning` (→ core-declared opaque seam, S4) |
| **value.go** | Value, Signature, ID minting | `ReturnsFn` slot semantics; `Carrier`/`Dynamic`/`dynFrom`/`FailedDispatch` stay as core-declared analysis flags (S5) | `CompileEffect`+16 flags, `CallableSpec`, `StoredBodySpec` stay core-declared opaque metadata (S5); `checkPassDepth`/`BeginIDMintScope` stay core, armed via hook | — |
| **emit_recorder.go** | — | re-cut exported `EmitRecorder` + `inactiveEmit` + `Recorder()` | concrete `*EmitState` remains sole implementor | — |
| **check.go** | — | bulk | `BeginCompilePass`, `IsolateEmit` | — |
| **emit.go** | `ModuleScopeBinding` | — | bulk incl. `stampCompiledRef` | — |
| **bytecode.go** | `ClosurePayload`, `CompiledFnRef`/`restampBox`/`depsFresh` (or opaque handles, S5) | — | Program/Opcode/Instr model | — |
| **code_effect.go** | `CodeEffectInfo` struct (sealed payload, load-bearing for `positionalMatch`) | `AnalyseCodeEffectCarrier` | — | — |
| **core_helpers.go** | installDef/InstallFnDef/compileFnSigs skeletons (hooks at F7 sites) | `buildFnBodyReturnsFn`, `checkFnBodyAtConstruction`, `checkBodyReturnConformance`, carrier builders (`narrowArgsToParams`, `paramBodyCarrier`, …) | `planUserPolyDispatch`, the armed-compile block inside `buildFnBodyReturnsFn` | — |
| **effects.go** | `EffectLedger`, `Note`, `Count`, `NoteEffect` | — | — | `ArmEffectFence`, `noteEffectWriter` |
| **coverage.go** | all incl. `noteVMCoverage` (core receiver) | — | — | — |
| **diag_render.go** | `renderSite`/`writeMarker` | `RenderCheckDiagnostic`, `severityStyle` | — | — |
| **interp_entry.go** | noteInterp/bail holder | — | — | `vmDefer`, `vmDeferAlt` |
| **engine_pool.go** | pool | — | — | `runIslandResolved` |
| **invoke.go** | `InvokeBody` + the seam declaration | — | — | compiled-ref fast path |
| **signature.go** | bulk + `CheckFullStackFunc`/`ReturnsFunc` type decls (pure func-over-core-types — stay as core extension slots) | the four carrier arms of `sigTypeMatches` via S3b hooks | — | — |
| **core_type.go** / **word_extend.go** / **core_ref.go** / **core_make.go** / **core_boolean.go** / **compare.go** / **util.go** / **generics_unify.go** | bulk | `CheckMakeConstruction`, `Tor/Tand/TnotReturnsFn`, `OrderingReturnsFn`, `GenBindingCarrier`, predicate-sandbox check snapshot (hook) | predicate-type stamping (hook S8) | — |
| **method_shape.go** | window-scan helpers may stay core (they use only Engine matcher state) | dispatch models (off the `*Engine` receiver → hook slots, S9) | `RecordDynMethod`/`noteShapedRead` halves via interface | — |

---

## 2. Top entanglement families blocking the split

**F1 — Core consults CheckState directly** (~48 `IsActive()` sites; 97 `Check.` refs in engine.go). Hardest: **`matchSignature` itself behaves differently under check** (`AmbiguousGradualSplit` latch, `gradualAny` gated on `!Check.Compiling`, `carrierMixedConform`); `Run()` strips input to carriers + meters the step budget on the hot loop; `Registry.Lookup` gates its dispatch cache on `IsActive()`; `RunPredicate` short-circuits on `Check.Mode`; `validateReturnTypes`, `policyGateWord`, `policyGateModuleCallReg`, `noteCoverage`.

**F2 — Core/check reach the emit recorder** (88 `Recorder()` sites + 13 concrete `.(*EmitState)` asserts outside the emit cluster). Hardest: `tryRecordDynBody` pokes raw `EmitState` internals (`appendEvent`, `producedBy`, `eventInfo`, `SiteCounts`, `dynEnv`); `execMatch`→`FoldFullStack` (engine.go:3667) and `stepLiteral`→`RecordSpliceDyn` (engine.go:4150) put the *concrete compiler type in the step loop*; `stepCloseParen`'s three-way core/check/compiler braid (8071–8156).

**F3 — Carrier-awareness inside core dispatch/unification** (~80 `.Carrier` flag reads outside carrier.go, mostly benign; ~6 hard code-calls). Hardest: `sigTypeMatches` **calls `NewCarrier` and `carrierOfLiteral`** (real check-code calls, not flag reads); `unifyCarrierVsTyped` + the flex/list/map gradual-tag arms; `depScalarUnifier.Match` carrier acceptance.

**F4 — Core structs declare downstream state** (declaration-level, ~10 structs). `Registry{Check, Invoker, nestedRunner(*CompiledFnRef), vmRunning, runtimeStamping, stampLog}`; `Value{Carrier, Dynamic, dynFrom, FailedDispatch}`; `Signature{ReturnsFn, CompileEffect, Callable, StoredBodies}`; `BoruImpl.Compiled *CompiledFnRef`; `CheckState.Emit EmitRecorder`; the sealed-Payload catalogue naming `GuardFactInfo`/`*StoreShapeInfo`/`ClosurePayload`/`CodeEffectInfo`; `Engine{reuseTape, flowUnwind, elemEvalRecordable, recorder}`.

**F5 — Core calls UP into the VM** (~6 sites, each load-bearing). `InvokeCallback`/`invokeCompiledUnit` → `RunUnit`/`jitRestamp`; `execFnDefSig`'s runtime branch; `RunPredicate`'s compiled path; `InstallType` → `StampDetachedFn`+`stampCompiledRef`; the island contract (`reuseTape`/`flowUnwind`/`exitWithFlowCtrl`/`runIslandResolved`).

**F6 — Check calls the compiler** (wrong direction for compiler→check; ~30 sites). Hardest: engine.go:9462 `tryCompileUserPolyArms` + `RecordUserPolyCall` (checker recovery drives arm compilation); `recordDispatchOutcome`'s whole chain; `MarkUncompilable` from recovery paths; `AnalyseLoopBody`'s `Checkpoint`/`Rollback`/`TakeFragment`; `planUserPolyDispatch`.

**F7 — Install-time check hooks** (~8 sites). `installDef`/`InstallWordExtension` → `RecordFnBinder`; `InstallFnDef`/`InstallWordExtension` → `checkFnBodyAtConstruction`; `compileFnSigs`/`compileFnDef` bake `buildFnBodyReturnsFn` onto **every** signature/anon fn; `ResolveRef` → `recordUse`; installDef's `CondBodyDepth`+`MarkUncompilable`.

**F8 — Legal-direction but package-private access** (dozens of symbols). VM uses unexported core (`dispatchHandler()`, `makeBoruError(At)`, `BoruError.fullSource`, `TapeConfig.resolve`, `growthCeiling`, `noMatchDiag`, `reorderHintFor`); `compile_sandbox.go` reads 5 unexported Registry fields; VM replays core invariants by hand (Args discipline, context bracket).

**F9 — Value-ID minting/provenance contract.** `checkPassDepth` gates `NewValueRaw` ID minting purely to feed `producedBy`; join/narrow code preserves IDs solely for the compiler's provenance model; Program embeds live `*Registry` pointers (`PolyRef.Reg`, `CompiledFn.Reg`, `UserPolyRef.Reg`).

**F10 — Shared diagnostic-text contract.** `noMatchDiag`/`undefinedWordDetail`/`buildReturnTypeError`/`buildReturnCountError` are consumed byte-identically by all four pieces — must land in core.

---

## 3. Seam designs

**S1 — `CheckHooks` interface in core** (breaks F1, F7, and the check half of F2).
```go
// core (registry.go)
type AnalysisHooks interface {
    Active() bool                       // replaces Check.IsActive()
    ModeActive() bool                   // replaces Check.Mode probes
    NoteStep(pos SrcPos) error          // step-budget metering
    NoteDefRead(id, name string); NoteUse(name string); NoteFnBinder(name string)
    StripInput(toks []Value) []Value    // Run()'s carrier strip (identity by default)
    OnUnmatchedDispatch(e *Engine, w WordInfo, args []Value, pos SrcPos) (handled bool, err error) // routes to checkModeSurfaceShape/AssumeSig
    OnLiteralArrival(e *Engine, v Value) (handled bool, err error) // tryShapedMethodDispatch etc.
    OnParenClose(e *Engine, win []Value, pos SrcPos) (handled bool)
    OnInstallFn(r *Registry, name string, fd *FnDefInfo)          // checkFnBodyAtConstruction + ReturnsFn baking
    PredicateSnapshot() func()          // predicateSandbox restore closure
    OnDefInstall(r *Registry, name string) // CondBodyDepth/MarkUncompilable
}
type noopHooks struct{} // every method no-op/false
```
Installed on `Registry.hooks` by check's `Begin`; `Registry.Check` becomes an opaque `Analysis any` slot with a typed accessor exported from check. Hot-loop cost mitigated by a cached plain `analysisActive bool` on Registry that check flips (core reads a bool, not an interface call, in `Lookup` and the step loop).

**S2 — Re-cut `EmitRecorder`, home it in check** (breaks F2/F6's interface half). Check owns an exported interface; compiler's `*EmitState` implements it (legal: compiler imports check). Required surgery: (a) export the 17 unexported methods or drop them from the check-facing surface (most — `bindRegistry`, `topFrameOnly`, `producedBy`-adjacent — are only called by code moving *into* compiler anyway); (b) opaque handles for compiler types: `TakeFragment() EmitFragmentHandle` / `Checkpoint() EmitCheckpoint` where these are check-declared empty-ish interfaces the compiler's concrete types satisfy; (c) **drop `Finalize`/`resolveOperand` from the check surface** — their callers (lang `CompileCheck`, `stamp_runtime.go`, `tryFold*`) live at/above compiler and hold `*EmitState` concretely; (d) spec types `TypedBindSpec`, `FallbackSpan`, `PolyNoMatchSpec`, `BranchRecord` move to **check** (check proves → compiler stores → eng replays; both consumers import check legally). (e) The 13 concrete asserts: 8 disappear because the asserting function moves to compiler (carrier.go×4, drift_window, stamp_runtime, core_helpers `storedGradualDepth` block); the rest (engine.go×2, method_shape×2, `inClosureUnit`) get promoted onto the interface or routed through S1.

**S3 — `DispatchOutcome` hook: compiler registers on check** (breaks F6's core). Check publishes:
```go
// check
type DispatchRecorder interface {
    OnDispatchOutcome(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos)
    OnUserPolyUnmatched(r *Registry, word string, args []Value, declared *Value) (plan *PolyPlan, ok bool) // replaces tryCompileUserPolyArms call
    OnBodyAnalysis(...)  // AnalyseLoopBody/AnalyseFnBody capture points
}
```
`recordDispatchOutcome`, the `tryRecord*/tryFold*` family, `user_poly.go`, and `drift_window.go` move to compiler and become this hook's implementation; check's `carrierResults` calls the hook instead of the recorder.

**S3b — Carrier constructor seam for F3's hard calls.** The `Carrier`/`Dynamic` flags **stay core Value fields** (documented as analysis flags — this makes ~74 of the 80 reads legal flag probes). The 6 code-calls (`sigTypeMatches`→`NewCarrier`/`carrierOfLiteral`, `unifyCarrierVsTyped`'s narrowing) go behind two nullable func vars in core (`carrierProbe func(t *Type) Value`, set by check's init) — or, simpler, `NewCarrier`/`carrierOfLiteral` move to core (they are ~20-line constructors over core Value); the unify gradual arms stay core since a pure interpreter never produces `ShapeCarrier` operands (dead-but-compiled arms, coverage note in §5).

**S4 — Compiled-execution seam in core** (breaks F5). Generalize the existing `Registry.Invoker` inversion:
```go
// core
type CompiledRuntime interface {
    InvokeCompiled(r *Registry, ref any /*opaque CompiledFnRef*/, args []Value) ([]Value, bool, error)
    StampDetached(r *Registry, fd *FnDefInfo, pos SrcPos) (any, bool)   // InstallType predicate stamping
    RunIsland(...)                                                      // island contract
}
```
`BoruImpl.Compiled` becomes `Compiled any` (or a tiny core `CompiledRef interface{ Fresh(r *Registry) bool }`); eng installs the runtime at `RunProgram` entry; `InvokeCallback`'s fast path, `jitRestamp`, and `core_type.go:443` route through it, no-op default = interpreter path. `nestedRunner`/`Invoker`/`vmRunning` stay core-declared but typed opaquely.

**S5 — Declarative metadata stays core-declared.** `CompileEffect` (bitfield), `CallableSpec`, `StoredBodySpec`, `ReturnsFunc`/`CheckFullStackFunc`, `RunInCheck` — pure data/func-type declarations over core types with zero behavior. Keep them in core as documented extension vocabulary (pragmatic; avoids inverting hundreds of signature-registration sites in lang/basic). Same for `ClosurePayload` and `CompiledFnRef`'s *shells* if S4's opaque-handle route proves too invasive for rendering/equality (value.go:3683, payload.go:395).

**S6 — Payload seal extension.** Export an embeddable marker: `type PayloadBase struct{}` with the unexported `payloadMarker()` in core; check/compiler payload types (`GuardFactInfo`, `*StoreShapeInfo` — and `ClosurePayload` if it moves) embed it. Preserves must-opt-in discipline while letting payloads live out-of-package. (`CodeEffectInfo` simply stays core — it's load-bearing for `positionalMatch`.)

**S7 — Effect ledger**: type + `NoteEffect` to core (verified necessary); `ArmEffectFence` to eng; the `Unwrap` writer protocol documented as a core contract.

**S8 — Stamping side-table**: `runtimeStamping`/`stampLog` move off Registry into a compiler-owned arming object reached via S4's `StampDetached`; `ForkConcurrent` propagation handled by a core `forkHooks` callback the compiler registers.

**S9 — Engine hook slots for check dispatch models.** `tryShapedMethodDispatch`/`tryMemberFnArrivalDispatch`/`tryDynamicFnValueDispatch` cannot remain `*Engine` methods post-split. Engine gains `dispatchHooks AnalysisHooks` (same S1 interface, `OnLiteralArrival`); the window-scan helpers they need (`matchSignature` access) are exposed via a narrow exported `Engine` API (`MatchSignatureAt`, `TapeWindow`).

**S10 — Exported Registry snapshot API** for compile_sandbox: `Registry.SnapshotForAnalysis() RegistrySnapshot` / `RestoreFromSnapshot`, covering `pendingGen`, `Modules.seq/loaded`, `builtinWords`, `Capabilities.store`, `dispatchCache.reset` (precedents exist: `DefTable.Clone`, `TypeTable.Clone`).

---

## 4. Staged migration (each stage green)

**Stage 0 — In-package hygiene (no behavior change).** (a) Eliminate the 13 concrete `.(*EmitState)` asserts: promote `FoldFullStack`/`RecordSpliceDyn`/`inClosureUnit`/`memberFnReadValue`/`dynamicStackShuffleOK`/`noteShapedRead` onto the interface or move the asserting functions into emit-cluster files. (b) Physically regroup misfiled functions into piece-named files *within* package eng (`check_recovery.go` from engine.go's 9067–9972 region, `compiler_dispatch_record.go` from carrier.go's tryRecord family, etc.). (c) Add a CI script that maps files→pieces and fails on new wrong-direction identifier references (a "virtual package" lint). This stage is pure `git mv`-within-package + mechanical edits; full test suite + cover-gate unchanged.

**Stage 1 — Invert core→eng (F5).** Introduce S4's `CompiledRuntime` + opaque `Compiled any`; move `vmDefer`/`vmDeferAlt`, `runIslandResolved` wiring; split effects.go (S7); route `InstallType` stamping through S8. Pure-interpreter behavior already exists as every seam's nil-default, so tests stay green; add unit tests for the no-op arms (ADR-008).

**Stage 2 — Invert core→check (F1, F7, F3-hard).** Land S1 `AnalysisHooks` + the cached `analysisActive` bool; convert engine.go/registry.go/policy_hook.go/core_ref.go/core_helpers.go touchpoints one region at a time (Run strip → step budget → stepWord notes → dispatch recovery → install hooks); move `NewCarrier`/`carrierOfLiteral` to core (S3b). `CheckState` decl moves out of registry.go into check-side files (still same package). Differential gates (`test/go/engspec/crossdiff_test.go`, langspec) pin behavior.

**Stage 3 — Invert check→compiler (F2, F6).** Re-cut `EmitRecorder` (S2): re-home spec types, opaque fragment/checkpoint handles, drop Finalize/resolveOperand from the check surface; land S3 `DispatchRecorder` and move `recordDispatchOutcome`+family, `user_poly.go`, `drift_window.go`, `planUserPolyDispatch`, `BeginCompilePass`/`IsolateEmit` to compiler-side files. This is the highest-risk stage — gate on `test/go/engspec` compile/interp crossdiff.

**Stage 4 — The actual package split, bottom-up.** Create `eng/go/core`, then `check`, then `compiler`, then keep the VM in the existing `eng/go` (or `eng/go/vm`) — and turn the current `package eng` into (or retain it as) a **facade of type aliases** (`type Value = core.Value`, `type Type = core.Value`, `type Registry = core.Registry`, const/func re-exports), so `lang/`, `basic/`, `parser/`, `specfix/`, and all external imports of `github.com/boru-lang/boru/eng/go` compile unchanged. Export-or-relocate the F8 symbols as each boundary is cut (core exports `DispatchHandler()`, `MakeBoruError`, error internals via setters; S10 snapshot API). Internal white-box tests move with their code.

**Stage 5 — Gate re-baselining.** Split covergate profiles per new package; per-piece standalone corpus lanes (core-only interpret lane from `corpus_standalone_test.go`, skipping check/compile rows via `specfix.ErrSkipRow`); raise `ENG_GATE_FLOOR` equivalents per piece; migrate `//covergate:allow` comments with their functions.

---

## 5. Risks

- **Circular-dependency traps.** (1) The recorder seam: if any check-surface method keeps a compiler type (miss one of `emitOperand`/`emitCheckpoint`/`*EmitFragment`), the check package cannot compile — the re-cut must be total. (2) `Program` embeds **live `*Registry` pointers** captured at check time (`PolyRef.Reg`, `CompiledFn.Reg`, `UserPolyRef.Reg`) — compiler→core value-wise, but a *lifetime* contract: the artifact dies with the check-pass registry; re-keying it mid-refactor is a silent-corruption hazard, so preserve pointer identity and document it. (3) `CheckState.Emit` + `Clone`'s pointer-aliasing ("shared, not snapshotted") must survive the interface re-cut. (4) `checkPassDepth` ID-minting: core-owned counter armed by check via hook — do not let provenance (`producedBy`) IDs drift or the compiler's operand resolution silently degrades to MarkUncompilable.
- **`type Type = Value` alias.** Alias must become `type Type = core.Value` in the facade — legal Go, preserves identity. But canonical-`*Type` discipline (`CanonicalType`, orphan-pointer hazard) now spans packages: every package holds pointers into `core.TypeTable`; the "no methods on Value" rule means the whole free-function surface (`AsInteger`, predicates…) must be exported from core — large but mechanical.
- **`Registry.Check` field / hot-loop cost.** `Lookup` consults check state per word lookup and `Run` per step. An interface call there is measurable (`perf_baseline_bench_test.go` exists — use it). The cached-bool mitigation (S1) is mandatory, and the bool must be flipped at *every* pass entry/exit (Begin, sandbox restore, Clone, ForkConcurrent) or check mode silently disables.
- **Engine pool.** Pooled sub-engines carry `reuseTape`/`flowUnwind` (VM island flags) and `elemEvalRecordable` (compiler flag) on the core Engine struct; pool reset must clear piece-owned state it can no longer see — give Engine an exported reset hook list, or the pool leaks check/compile state into interpreter runs (a bug class the current single package masks).
- **Parser placement.** Already clean: `eng/go/parser` uses only core surface (verified — no Check/Emit/Carrier). It keeps importing the facade; zero change needed. Do **not** move it below core — it emits sugar markers whose consumers are the core step loop.
- **specfix placement.** Uses check + compiler surface (`CheckDiagnostic`, `AnalyseLoopBody`, `BranchRecord`, `CompileEffect`, `CallableSpec`) — it must stay above all four pieces, importing the facade. The "standalone core" story changes: today `corpus_standalone_test.go` proves *eng* standalone; post-split you want a *core*-standalone lane, which needs a specfix runner variant that never touches check/compiler symbols (split specfix's `check.go`/`words.go` from `runner.go` if core-lane purity is enforced by imports).
- **Coverage gates (ADR-008 + standalone).** The merged 100% gate applies per reachable statement: every no-op hook default (S1's `noopHooks`, S4's nil arms) adds statements needing tests — the `inactiveEmit` file shows the precedent and the cost. Check-mode-only arms left in core (unify carrier arms, S3b) are unreachable from core's own suite — they'll need either `//covergate:allow` proofs or the check package's suite in the merged profile (fine for the merged gate, but the **standalone** `cover-gate-eng` floor (currently 89) must be re-scoped per piece, and core-standalone will not reach check-only arms: plan to move those arms behind S3b hooks rather than allowlist them). Also: many tests are *internal* (white-box `package eng` tests, e.g. `interp_bake_probe_internal_test.go`); every symbol relocation drags tests — budget test migration as the largest LOC item of Stage 4, and keep the facade aliasing so *external* tests (`test/go/*`) never move.
- **Unexported cross-piece symbols (F8).** Hundreds of identifiers legal today only because of the single package (VM→`sigError` machinery, `makeBoruError`, `compile_sandbox`→Registry privates). If the export churn stalls Stage 4, the fallback that still captures ~80% of the value is stopping after Stage 3: one Go package, four enforced "virtual packages" via the Stage-0 lint — all wrong-direction *calls* eliminated, physical split deferred.