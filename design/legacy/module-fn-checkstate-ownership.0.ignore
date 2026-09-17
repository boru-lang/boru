# Module-fn body compilation and the `CheckState`-ownership problem

Status: **`.0` analysis / negative result.** No code change landed from this
investigation beyond the soundness fixes noted below; this note records *why*
the obvious fix does not work and what the real fix must do, so the next attempt
starts from the analysis rather than re-discovering the trap.

Branch of record: `claude/bytecode-compilation-bugs-5xlp0l` at commit `d55daf0`
(2026-06). All gates green there; the threading experiment described in §4 was
implemented, shown to break, and reverted.

---

## 1. Goal

Compile programs that drive the `boru:test` **declarative** runner —
`spec Test.run-spec` — to bytecode that runs on the VM, byte-identical to the
interpreter. This is the last large bucket keeping the `voxgig-boru/decision`
spec/property suites (`decision_unit_spec`, `decision_prop_test`,
`decision_prop_spec`) on the whole-program interpreter fallback. The decision
module's **core logic** (`apply-op`, `eval-cond`, `decide`, …) already compiles;
the imperative `unit_test` and `smoke` suites already compile == interpreter. The
blocker is specifically the module-fn bodies of the test framework.

## 2. How a module fn dispatches (the relevant architecture)

A native module (`lang/go/modules/test.go::BuildTestModule`) builds its own
**sub-registry** `modReg` via `native.DefaultRegistry()` and runs a boru preamble
into it. Fns defined in that preamble (`run-spec`, `run-cases`, `run-case`, and
the Go natives `test-describe`, `test-invoke`, `test-record`) carry
`FnDefInfo.Registry = modReg`.

When such a fn is invoked, `engine.go::execFnDefSig` sees a foreign registry and
runs the body via `capturedReg.CallBoru(sig, args, captures)` (`registry.go:1072`),
which spins a sub-engine over `modReg` so module-private words resolve. The body
tokens of `run-spec` are spliced and stepped there.

Two members behave differently and are the crux of the problem:

- **`test-invoke`** (`test.go:1189 invokeSubject`) dispatches the *subject* fn by
  a runtime name in the **parent** registry (`native.New(parent).Run(...)`). So a
  subject call records into the PARENT's `Emit` and compiles to a `CALL_USER`
  unit — recording threads here.
- **`test-describe` / the `run-cases` `for` loop / `test-record` / `deq`** run via
  `CallBoru` in **`modReg`**, whose `Check` is independent of the parent's.

`AnalyseFnBody` (`eng/go/carrier.go:2230`) is the check-mode body analyser; it
keys its memo (`FnSummaries`) and recursion guard (`FnInflight`) by
`(name, arg-types, body)` on **`r.Check`** — i.e. on whichever registry's
`CheckState` is live.

## 3. The bug: module-fn bodies run CONCRETELY during the parent's check pass

`CheckState.Mode` lives **per registry** (it is a value field on `Registry`, see
`eng/go/registry.go:247`). When the top-level program is in check/compile mode,
`parent.Check.Mode == true` but `modReg.Check.Mode == false`. So when `run-spec`'s
body runs via `CallBoru` in `modReg`, the body executes **concretely, not in check
mode**:

1. **Side effects fire.** `test-record` is not gated by
   `r.Check.SkipsSideEffect()` because `modReg` is not in check mode, so it
   mutates the real `TestRun` *during the compile pass*.
2. **Runtime-dependent values fold to constants.** The body compares a subject
   result against the expected output (`expected actual deq`); with the subject
   result an abstract carrier, the concrete fold bakes a wrong boolean.

The visible failure (reproduced with the `evCallUser` promotion that let
`run-spec` reach lowering): a 2-case spec compiled to bytecode reporting
`{total:2 passed:1 failed:1}` where the interpreter reports
`{passed:2 failed:0}` (`module-test.tsv:L38`). The disassembly was just
`PUSH_CONST 3; CALL_USER double; DROP; PUSH_CONST 0; CALL_USER double; DROP;
CALL_NATIVE test-summary` — the entire test logic had been folded away and the
pass/fail counts came from the side-effect leak during the check pass.

This is **latent today**: at `d55daf0` `run-spec` refuses on residual
reconciliation (the `evCallUser` promotion is reverted), so the wrong bytecode is
never produced and the program falls back, byte-identical. But the concrete-fold
path is the hazard that any future attempt to compile module-fn bodies will trip.

> The "compilation" of `unit_test` / `smoke` that exists today is the SAME
> concrete fold. For *deterministic* test bodies (concrete assertions) the fold
> is value-correct, so it matches the interpreter — but it has run the tests at
> COMPILE time via the interpreter and baked the results, which is not what the
> runtime-independence goal wants. It only looks like compilation.

## 4. Why the obvious fix (thread check mode into `modReg`) does NOT work

The tempting fix: around the `CallBoru` in `execFnDefSig`, when the parent is in
check mode, point `modReg.Check` at the parent's check state for the call so the
body runs in check mode there too (side effects suppressed, carrier-results
intercept active, recording threaded into the parent's `Emit`).

Two variants were implemented and BOTH broke check-mode analysis of the imported
decision module:

| Variant | Result |
|---|---|
| Share the whole `CheckState` (`modReg.Check = parent.Check`, restore after) | `decide` and peers fail to match their signatures — **29 errors** `uncalled_function: call to 'decide' matched no signature (arguments: Word, Map)` |
| Share only `Mode` + `Emit` (keep `modReg`'s own memos/diagnostics) | still broken — **39 errors** |

**Root cause — `CheckState` assumes a single registry.** The analyser's state is
bound to one registry's `Defs` and name space, but a module-fn body resolves in a
DIFFERENT `Defs` (the module's). When the parent's `CheckState` is active while
the body steps in `modReg`:

- **`FnSummaries` / `FnInflight` are name-keyed** (`name#argTypes`). The parent
  and module name spaces collide; a module fn's recursion guard or memo can be
  shadowed by a same-named parent entry (or vice versa), so an inflight bail or a
  stale summary is returned for the wrong fn — sigs then "match no signature."
- **Undefined-word handling** (`stepWord`'s check-mode `Word → Atom{Undefined}`
  leniency) interacts with module-private resolution: a module word that should
  resolve in `modReg.Defs` is analysed under a check state that expects the
  parent's name space, so it degrades to an `Atom` and the downstream call sees a
  `Word`/`Atom` where it wanted a typed value (the `arguments: Word, Map`
  signature failure is exactly this).
- **The carrier-results intercept** (`engine.go` `e.registry.Check.IsActive()`
  branch) and `AnalyseFnBody`'s per-pass counters (`FnAnalysisCounts`,
  `StepCount`) all read `r.Check`; mixing the two registries' notions of "the
  current analysis" corrupts the per-fn accounting.

These did not show up in `make test`, the full-corpus differential
(`TestSpecCompiledOrFallback`), or the property fuzzer (5k iters) — the spec
corpus does not exercise the heavy *generic* module-fn patterns
(`decide gen [...]`, `apply-op gen [...]`) the decision module leans on. The
breakage only surfaced in the decision module's own check, via `diverge.sh`.
**Lesson: `diverge.sh` against the decision project is a required gate for any
module-fn check-path change; the in-repo corpus is not sufficient coverage.**

## 5. What the real fix must do

Compiling module-fn bodies soundly is **not** a wrapper around `CallBoru`. It needs
the check/analysis state to be **split by concern** rather than owned wholesale by
one registry:

- **Resolution scope** (which `Defs` a word resolves against) must stay the
  module's `modReg` for the body.
- **Analysis mode + recording** (`Mode`, `Emit`) must follow the *outermost*
  compile pass.
- **Memo / recursion / counters** (`FnSummaries`, `FnInflight`, `InflightBails`,
  `FnAnalysisCounts`, `StepCount`) must be **keyed so cross-registry names cannot
  collide** — either namespaced by registry identity, or carried in a single
  pass-global object that is registry-agnostic.

Concretely, the candidate designs are:

1. **Pass-global `CheckState`, registry-scoped `Defs`.** Hoist `CheckState`
   (mode, emit, memos, counters) out of `Registry` into a single per-compile
   object threaded through the engine, while word resolution continues to use the
   engine's current `Registry.Defs`. Memo keys gain a registry/scope discriminator
   so `decide#…` in `modReg` ≠ a parent `decide#…`. This is the clean fix but a
   real refactor of `CheckState` ownership (every `r.Check.*` reader moves to the
   threaded object).

2. **Per-call check sub-state.** Give `CallBoru` an explicit "analyse-in-check"
   entry that builds a FRESH `CheckState` for the module body, seeded with
   `Mode=true` and the parent's `Emit`, with its OWN memo maps, and that MERGES
   only diagnostics + the uncompilable mark back. This is closer to the failed §4
   attempt but avoids sharing the memo/recursion maps (the proven breakage). Risk:
   recursion across the module/parent boundary (a parent fn calling a module fn
   that calls back) loses cross-boundary cycle detection — needs analysis.

Either way, side-effect suppression must be verified: every side-effecting native
reachable from a module body (`test-record`, IO, store mutation) must honour
`SkipsSideEffect()` so the compile pass cannot mutate observable state.

## 6. Secondary blocker (independent of §5)

Even with §5 solved, the test framework's **code-body words must compile as
closures** for the suites to actually produce a Program rather than refuse:

- `test-describe [body] name` — already declares a `CallableSpec` (BodyOut 0) but
  its body (`run-cases`, a `for` over `cases size`, recursive `run-spec` over
  `subs`) hits Stage-2 code-body / computed-range limits.
- `test-test name [body]` — the imperative form; same shape.
- `run-cases`' `for (cases size) [...]` — a computed-count loop whose body invokes
  `run-case` (which calls `test-invoke` + `test-record`).

So the full path is: **§5 (sound module-body check) + §6 (compile the framework's
code-body words)**. §5 is the prerequisite — without it, §6 would compile bodies
that fold unsoundly.

## 7. What IS sound and landed (for context)

On `d55daf0`, all gate-clean (`make test`, full-corpus differential, fuzzer 5k,
`diverge.sh`):

- **`var` compiles as an inline let** + an **armed-body-error soundness gate**:
  when an armed closure/fn body's analysis errors, the recorder marks the program
  uncompilable (refuse + fall back) instead of closing an EMPTY unit that diverges
  at runtime (`carrier.go::AnalyseFnBody`).
- **Interp-string carrier fold fixed**: a `${expr}` over a runtime value (a
  carrier) used to bake the carrier's render `"dynamic(Any)"` as a constant;
  `evalInterpString` now returns a String carrier and refuses recording, so the
  program falls back and builds the real string (`engine.go::evalInterpParts`).
- The dynamic-dispatch experiment (`evCallUser` value-def-local promotion +
  `get`-over-carrier leniency) that let `run-spec` reach lowering was **reverted**
  — it produced the §3 divergence by riding the concrete-fold path. The
  `evCallUser` promotion is sound in isolation and can return once §5 makes
  module-body compilation sound.

## 8. Recommended next step

Treat §5 as its own design + implementation task: a `CheckState`-ownership
refactor (option 1 preferred for cleanliness), gated by `diverge.sh` against the
decision project from the first commit, not just the in-repo corpus. Only then
re-attempt §6 and the `evCallUser` promotion.
