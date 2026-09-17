# Tiered execution: Python-class performance for interpreter mode

**Status:** Design / roadmap. The concrete architecture for closing the
remaining interpreter↔Python gap, grounded in the machinery verified
during the 2026-07 second-pass optimization series
(`legacy/INTERPRETER-PYTHON-PARITY.10.ignore`). Not yet implemented.

## Why this is the path

The 2026-07 series took the tree-walker to its practical floor: nine
gated commits cut allocations 50–80% per shape, dropped GC from ~35% to
~11% of CPU, and made recursion 4.6× faster than the original baseline —
and the profile is now DIFFUSE (no cost above ~6% flat). The remaining
gap (fib ~215×, loops ~60× Python) is the execution model itself:
per-token dispatch over 72-byte values with per-call overload matching.

CPython is not a tree-walker. Its "interpreter" parses to bytecode and
runs a tight dispatch loop — exactly boru's compiled mode, which already
runs fib at ~235ms wall (≈10× Python wall, most of it fixed startup) on
the same fixtures. **"Interpreter performance like Python" means doing
what Python does: compile transparently.** The tier keeps interpreter
mode's semantics and observability (the tree-walker remains the
reference) while hot code runs on the existing, differential-tested VM.

> **Scope note (2026-09-16).** This is a tiering design *inside
> interpreter mode* — which execution engine runs a hot fn body — and it
> is a different axis from compile mode's refusal contract. Nothing here
> softens that contract: a program `--compile` refuses is a **failure**,
> the interpreter is not a fallback for the compiler and is not allowed
> to be one, and an earlier version of this section set the target at
> "fall back rarely", which is not a target this project accepts. Within
> the tier, a body the VM cannot take is a body the tier failed to
> promote — a performance defect to close, not a resting place.

## Architecture: fn-level tiering inside interpreter mode

Tier boundary: the NAMED FN BODY — the same unit the emitter already
compiles (`EmitState.StartFnCompile` fn units) and the unit whose
dispatch the interpreter already owns end-to-end (`buildFnBodyHandler`,
core_helpers.go). Loops inherit the speedup by living inside fns; the
top-level statement stream stays tree-walked (cheap, once-per-statement).

1. **Hotness counter.** `FnDefInfo` gains a `*fnHeat` pointer (shared
   across copies, like `FnFrameMeta`): `{calls atomic.Int32, state
   atomic.Int32 /* cold | compiling | compiled | refused */, prog
   *Program}`. `buildFnBodyHandler`'s fast path increments; at
   `tierThreshold` (default 16; `BORU_TIER=0` disables) it attempts
   promotion once.
2. **Promotion = the EXISTING compile pipeline over a synthetic call.**
   Run `BeginCompilePass` + the recorder over the token stream
   `[fnname ⟨carrier args⟩]` in a sandboxed fork of the registry (the
   predicate-sandbox pattern, `snapshotPredicateState`) — the emitter
   already compiles the fn and its callees into fn units and an OpCall
   spine; `Finalize` yields a `*Program` whose entry takes the N args as
   locals. Any refusal (`MarkUncompilable`, dynScopeRescue residue, the
   identity-less-capture guard) → `state = refused`, tree-walk forever,
   zero behavior change. This reuses the whole correctness story: the
   compile pipeline refuses what it cannot prove, and the interpreter
   remains the semantics oracle.
3. **Dispatch.** In the leaf fast path, `state == compiled` short-
   circuits: run `prog` on the per-registry reusable VM (the pooled
   island-engine pattern in reverse — `vm.go` already runs Programs with
   an interpreter fallback island INSIDE it, so a VM-refused shape
   degrades composably). Args in, results out — the same contract
   CallBoru has. Flow-control (`break`/`continue` escaping a compiled
   body) already has a defined VM translation (`flowUnwind`,
   engine.go).
4. **Invalidation.** Promotion pins the per-name `Defs.Gen` of every
   word the unit resolved (the dispatch-cache counter, registry.go). A
   gen bump on any pinned name → `state = cold`, counter reset — same
   invalidation discipline the F3 analysis established.
5. **Observability.** `boru run --no-tier` and trace hooks force the
   tree-walker (tracing disables promotion — the trace IS the
   tree-walk); `Debug.profile` reports tier states per fn.

## Verification plan

- Differential battery: every `lang/spec/*.tsv` row executed in
  interpreter mode with `tierThreshold=1` (promote immediately) must
  equal tierless output — the spec suite becomes the tier's oracle,
  exactly as it is the VM's today.
- The recursion/closure/break/args frame tests from the F5 series run
  tiered; ForkConcurrent under `-race` (fnHeat is shared state).
- Alloc/latency gates: `TestInterpAllocCeilings` gains tiered twins;
  `bench/interp` gains a `boru-interp-tiered` column.

## Expected outcome (measured basis)

A promoted fn runs at compiled-mode speed: Stage6 measured
`recursion_nontail` at 8.18ms interp vs 0.36ms compiled (23×) and
`for_tight` 3.07ms vs 0.17ms (18×) on identical semantics. With
promotion covering the fixtures' hot fns, interpreter-mode wall-clock
converges on compiled mode's: **fib ~235ms, loops ~60–65ms — the same
~3–10× band vs CPython that CPython-class VMs occupy relative to each
other, with startup dominating.** Closing the residual band is then VM
work (dispatch loop, superinstructions, startup), shared with the
default mode — one performance story instead of two.

## Implementation findings (2026-07 survey)

Two load-bearing facts from reading the execution surface:

1. **Argument passing needs no new emit machinery.** Pre-install the
   reserved names `__tier0…__tierN-1` as dynamic carriers in the sandbox
   registry, then compile the constructed token stream
   `[fnname __tier0 … __tierN-1]`: the emitter resolves the reads through
   `dynScopeRescue` into runtime name-lookup operands, and compiles the
   fn itself as a normal fn unit + OpCall spine. At dispatch:
   `InstallDef` the real args under those names, `RunProgram`, `undef` —
   correctness rides entirely on existing, differential-tested paths.
   Per-call overhead is 2 map ops per arg (measured elsewhere as ~ns —
   irrelevant next to the 18–23× unit speedup).
2. **The T1 critical path is `RunProgram`'s entry guards** (vm.go:34-49):
   `interpRunActive()` rejects starting a compiled run while an
   interpreter run is in flight — precisely the tier's shape. The
   islands machinery proves the reverse nesting (VM→interp on one
   registry, single goroutine, `r.Invoker` save/restore) is sound; the
   tier needs the symmetric argument audited and a deliberate,
   tested nested-entry path (a tier-scoped variant of the guard that
   still rejects genuinely concurrent foreign runs). Flow-control
   crossing the boundary reuses the island translation (`flowUnwind`,
   `OpFlowBreak`/`OpFlowContinue` unwinding).

## Sequencing

- **T1** `fnHeat` + promotion skeleton behind `BORU_TIER` (default off),
  promotion via the sandboxed compile pass, dispatch short-circuit,
  spec-suite differential at threshold 1. Land dark.
- **T2** invalidation (gen pinning), trace/debug interplay, fork safety;
  flip default on for `run`/`do` in interpreter mode.
- **T3** measure; re-pin gates; update the parity note and README with
  the tiered column.
- **T4** residual VM work if the band still matters: startup (lazy
  module init dominates the ~20ms), dispatch-loop tightening.
