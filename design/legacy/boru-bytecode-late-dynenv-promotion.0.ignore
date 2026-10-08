# Bytecode: late-armed DynEnv value-def promotion

Closes the last `--force-compile` refusal on the `voxgig-boru/bloom-filter`
unit/smoke suites: `fn make-bloom: dynamic-scope def \`k-val\` of unpromoted
computed value`. After this the library and **all five** of its test suites
compile fully (`boru --force-compile`), output byte-identical to the interpreter.

## The refusal

`DynEnv` (`Program.DynEnv`, `EmitState.dynEnv`) is a **program-wide** mode
armed by `tryRecordDynBody` (`carrier.go`) the first time a `do {…}` /
dynamically-dispatched body can only compile via the dynamic backstop. Under
it, `lowerDynBind` re-pushes every `def`-bound value for its registry-visible
`OpBindDynScope` twin, so every dyn-bound **computed** source must live in a
frame local (`planValueDefLocals`' `(es.dynEnv && valueDef)` trigger, merged
through `collectDynBindSources`).

The trigger fires at each fn unit's **finish** (`StartFnCompile`'s closure,
`planValueDefLocals`). But `dynEnv` is program-wide and can arm **after** a
unit finishes: in bloom, `def bf ({…} Bloom.make)` compiles `make-bloom`
(dynEnv still false — `def k-val (derive-k m-val n-val)` stays on the
single-consume sim stack), and only a later `bf Bloom.params` — whose body is
`do {n:[bf.n], p:[bf.p], …}`, opaque-typed so it takes the dynamic backstop —
arms `dynEnv`. `Finalize` then lowers the already-finished `make-bloom` widened
and `lowerDynBind` refuses the unpromoted `k-val`.

This is the plan-vs-lower drift the `tryReturnedClosure` comment
(`emit.go`, "DynEnv arming mid-pass otherwise drifts plan against lowering")
already fixes for **returned closures** by arming `dynEnv` off a probe before
the real compile. A top-level `def` fn has no such probe.

## The fix

`promoteLateDynBind` (`lower.go`), called from `Finalize`'s per-unit loop
*before* the unit is lowered, when `dynEnv` is finally known:

- No-op unless `es.dynEnv` — a program that never becomes DynEnv is
  **byte-for-byte unchanged** (the common path pays nothing).
- Walks `rec.frag`'s recorded events (recursing arms/bodies via
  `collectPromotableEvents`), and for each dyn-bind event whose computed
  source is not already promoted, seats it into a fresh frame local and
  rewrites its references (`rewritePromotedRefs` / `promoteOperand`). This is
  **exactly** the `(es.dynEnv && valueDef)` promotion `planValueDefLocals`
  would have applied had `dynEnv` been armed at plan time — deferred, not
  different.
- Idempotent: a unit that finished *after* the arming already promoted these
  (they are in `rec.promoted`), so they are skipped.
- A source that is not a single-output call (`singleOutputCall` — a fragment
  RESULT that must stay on its sim, a makeMap/branch/loop value, a
  multi-output producer) is **left untouched** for `lowerDynBind` to refuse:
  a refusal rather than a wrong store — the better of the two failures, and
  still a defect owed a lowering.

## Soundness

The promotion is the store-once/re-push-per-use discipline already used for
every multiply-referenced or dyn-bound value-def; it changes the compiled
**shape** (an extra `OpStoreLocal` + `OpPushLocal`) but not the value, so the
residual stays byte-identical. The differential is structurally blind to the
arming *order*, so the fix ships a hand-pinned off-corpus regression:

- `lang/go/bytecode_late_dynenv_test.go::TestLateDynEnvArmingPromotesComputedDefs`
  — `RunCompiledStrict == Run` for the make-field, chained-def, and
  in-if-arm shapes, each asserted to compile with **no FALLBACK island**;
  plus a negative case pinning that a branch-valued dyn-bind source refuses
  rather than miscompiling — the pin fences the miscompile, and the refusal
  it fences is itself an open defect.
- `eng/go/lower_latedyn_test.go::TestSingleOutputCall` — the recursion-blind
  arms.

Gates: `make fmt/vet/lint/test`, `make verify-bytecode` (differential +
or-fallback + combination + property fuzz, plus the `-race` and `-tags
borudebug` lanes), and `make cover-gate` (100%) all green.
