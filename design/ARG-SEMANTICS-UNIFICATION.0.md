# Argument Semantics Unification — named and unnamed params must mean the same thing

**Status:** IMPLEMENTED (July 2026, this branch) — see §7 for the
outcome record, including two discoveries the implementation made that
the investigation below did not predict: the CallBoru residual leak and
the flow-control frame-unwind bug. Originally an investigation +
proposal (not an ADR). Follow-on to
design/SUB-ENGINE-MAIN-TAPE-REVIEW.0.md §7, where the unnamed-Function-param
auto-dispatch surfaced as a compile-refusal tier. This note argues the
runtime behaviour itself is the defect, states the invariant that should
hold, maps every observed divergence, and lays out the mechanism to close
them.

## 1. The invariant

> **An argument never acts on its own.** Binding style — named vs unnamed —
> changes only the *access idiom* (reference by name vs positional stack
> consumption / `args.N`). The argument value's behaviour is identical in
> both styles: it is resolved data until the body explicitly uses it.

Today this holds for named params and fails for unnamed ones, because of a
mechanical difference in how the three body-assembly paths lay a call out:

- **Named** params are bound as frame defs (`InstallFrameBinding`) —
  *outside* the tape's step region. The value does nothing until a body
  word references it.
- **Unnamed** params are spliced as raw values *into* the step region of
  the frame (`( a0 a1 body… )` with the pointer starting at the front).
  The engine steps them like fresh program tokens, so any value with
  active step semantics fires on placement.

## 2. The divergence matrix (all verified empirically)

| Shape | Named binding | Unnamed binding |
|---|---|---|
| `Function` arg, body never uses it | inert; returned as data (`f/r`) | **sibling-dependent**: inert when it is the only frame value (`fn [[Function] …]` → returns the fn), **auto-fires** when any sibling value is adjacent (`fn [[Function Integer] [Integer] []]` applied to `mk, 14` → `42` with an empty body) |
| `Function` arg, body applies it | `f x` → call | `(args.0 args.1)` → call — **plus** the placement auto-fire, so the fn runs **twice** |
| `Function` arg, bare reference, arity unmet | `[f]` → `signature_error: no matching signature for f` | value stays data |
| List arg | `Quoted = true` forced at bind ("treated as data values … not expanded as code bodies" — core_helpers.go, registry.go) | placed raw; protected only incidentally (`execFnDefSig` clears `Eval`; `CallBoru` / `buildFnBodyHandler` do not) |
| `__SP` splice-marker arg | bound inert (`def name word value` binds the marker) | fires when the frame steps it (`stepLiteral`) |
| Arg normalization (`Eval`/`Undefined` cleared) | `execFnDefSig` clears both on every arg; `CallBoru` and `buildFnBodyHandler` clear neither | same inconsistency, path-dependent |

Two of these deserve emphasis:

- **Sibling dependence** is the sharpest violation: whether argument 0
  fires depends on whether argument 1 *exists*. Arguments interact with
  each other through frame adjacency — `fn [[Function]]` and
  `fn [[Function Integer]]` give the fn-value argument two different
  meanings.
- **The kernel already ruled on the principle** — for lists. All three
  assembly paths force `Quoted = true` on named list args specifically so
  a resolved argument cannot re-fire as code. Unnamed args never received
  the equivalent decision; the Function/`__SP` cases are the same hole at
  different value kinds.

The named-side `signature_error` on a bare under-applied reference is NOT
part of the defect: a Word reference *must* dispatch (that is the
language-wide Word-vs-value rule, same as any binding), while a value is
data. That difference is access-idiom, not argument semantics, and stays.

## 3. Root cause

`Run` pins `e.pointer = 0` (engine.go) and `execMatch` re-steps handler-
returned tokens, so a frame's unnamed args sit **ahead of the pointer**
and get stepped as if they were fresh source. Stepping is where active
semantics live: `execFnDefLiteral` auto-dispatches a fn value with
operands adjacent, `stepLiteral` fires `__SP`, `Eval` lists auto-evaluate.
Named args never pass under the pointer at all.

The call-site already resolved these values once (forward collection /
auto-eval / paren evaluation). Re-stepping them in the callee frame is a
**second resolution of the same data** — that is the asymmetry.

## 4. Unification options

### U1 — arguments enter the frame *resolved* (recommended)

The frame's step region starts **after** the unnamed args: they are laid
down as pre-resolved stack prefix (exactly what "below the pointer" means
everywhere else in the engine — `resolvedIndicesBefore` consumes them as
stack operands regardless of whether they were ever stepped). Body words
consume them positionally; `args.N` reads them; a fn value among them is
data until the body applies it. Placement alone does nothing — matching
the named side, where binding alone does nothing.

Mechanism, per assembly path:

| Path | Change |
|---|---|
| `execFnDefSig` splice branch + `buildFnBodyHandler` frames (`( a0 a1 body… tail )`) | carry the unnamed-arg span on `FrameOpenInfo` (it already carries `Meta`; `FrameTailSpec.UnnamedCount` computes the same number for the ReturnCheck) and have the frame-open step advance the pointer past the span. One step-site change covers every spliced frame. **Verify first:** `stepOpenParen` currently *overwrites* the cell with a plain `NewOpenParen()` — the TCO probe scans (`fn_frame_probe.go`) rely on `FrameOpenInfo` surviving on the tape, so either the overwrite is on a different branch or the skip must preserve the payload. |
| `CallBoru` / `InvokeBody` / VM `invokeClosure` (sub-engine token streams `a0 a1 body…`) | an engine-level start offset (`Run` honours a `startAt` instead of the hard `pointer = 0`), set to the input count. The pooled-engine release must reset it. |

Choosing pointer-skip over flag-marking (`Quoted = true` on unnamed fn
values, mirroring the list rule) is deliberate: the flag **leaks** — a
body that returns its argument unchanged would hand the caller a
Quoted fn whose later auto-apply is suppressed, while the named path
returns an unquoted one. That would trade today's asymmetry for a new
one. Pointer-skip mutates nothing; it just tells the truth — these
values were already resolved.

The same pass should also close the small normalization gaps with one
shared boundary (per the No-Zero-Value-Overload discipline of resolving
at a single point): quote/clear `Eval` identically for named and unnamed
args in all three paths.

### U2 — placement behaves like a named reference (rejected)

Make unnamed placement always *attempt* dispatch and error when arity is
unmet, mirroring the named `[f]` error. This makes `fn [[Function]]`
(hold a fn as data) inexpressible and keeps arguments un-storable — it
generalises the defect instead of removing it.

## 5. Consequences of U1

- **Sibling dependence disappears.** `fn [[Function]]` and
  `fn [[Function Integer]]` treat the fn value identically.
- **Unnamed fn params become real data** — storable, forwardable,
  applicable exactly once via `(args.0 …)` or positional consumption.
- **The compiler model becomes true.** Check-mode analysis already treats
  unnamed args as inert carriers; the runtime would now agree. The
  "unnamed fn-value param auto-dispatch" refusal tier added with the R2
  work (refusal gate 11 → 14, design/P7-ENDGAME.10.md) can be retired,
  and the still-open module-fn-unit unsoundness (a module fn of this
  shape with an empty body compiles to a 0-return unit) dissolves rather
  than needing its own guard.
- **`__SP` markers passed as args stop firing on placement**, aligning
  with the named `def name word value` binding rule.
- The zero-body implicit-apply trick (`fn [[Function Integer]] […empty…]`)
  stops working; nothing in the repo uses it, and the explicit
  `(args.0 args.1)` spelling replaces it, firing once instead of twice.
- `lang/spec/module-fnvalue-boundary.tsv` rows and the Go twins get
  updated to pin the new inertness; new rows must pin both directions
  (fn value held as data; fn value applied exactly once; marker args
  inert).

## 6. Verification plan

1. Pin CURRENT behaviour of every matrix row above in a scratch branch
   (some are pinned already by the R2 boundary work).
2. Implement U1 behind the five assembly sites; run the full corpus —
   the compiled differential is the strongest oracle here, since the
   compiled side already implements the "arguments are data" model.
3. Retire the refusal tier (gate 14 → 11) and re-run
   `TestCompiledCoverage` / `TestCheckAccuracyRatchet` for the pin walk.
4. Bench: no cost expected (the change removes steps); re-run
   `engine_pool_bench_test.go` and the macro timings from the review doc
   §7 to confirm.
5. Sweep for out-of-tree reliance on implicit frame apply before landing
   (this is a semantics change and deserves a line in the release notes).

## 7. Outcome record (implemented July 2026)

U1 landed exactly as proposed at the placement layer, and the corpus
surfaced two consequences the proposal missed — both fixed:

**Placement (as designed).**

- Spliced frames: `FrameOpenInfo.ArgSpan` carries the unnamed-arg span;
  the step loop's open-paren case skips it (engine.go). Stamped by
  `buildFnBodyHandler` and `execFnDefSig`'s splice branch. The main-loop
  `stepOpenParen` worry from §4 was unfounded — frames step through the
  plain `IsOpenParen` case (`pointer++`), which never touches the
  payload; `stepOpenParen` is the separate `"("`-word handler.
- Sub-engine runs: `Engine.startAt` (one-shot, consumed by Run) starts
  stepping after the resolved inputs. Set by `CallBoru`
  (unnamed-arg prefix) and by the new `RunResolved(r, inputs, tokens)`
  pooled helper, which `InvokeBody`, the VM's `invokeClosure` raw
  branch, map-iteration quotation bodies, `eachrank`, `case` blocks and
  the fn log sink all use.

**Discovery 1 — the residual leak (CallBoru trim).** Making placement
inert leaves an unconsumed unnamed arg at the BOTTOM of the callee's
residual. The frame path already handles this (the ReturnCheck collapse
validates declared returns from the top and discards bottom extras up
to `UnnamedCount`); `CallBoru` had no such discipline — a leftover
fn-value arg flowed back into the caller's stream, where RESULTS
legitimately re-step, and fired there instead. Fix: `CallBoru` now
mirrors the frame's DISCARD (trim residuals beyond `len(sig.Returns)`
from the bottom, capped at the unnamed count) — trim only, no new
errors. A full validate+count mirror was tried and reverted: guard
predicates signal failure via a `None` residual, lambdas auto-declare
`[Any]` over side-effect bodies, and module fns have never been
return-count-checked (`L.two 5` under declared `[Integer]` returning
two values flows both out — a pre-existing frame-vs-CallBoru asymmetry
this work leaves documented, not changed).

**Discovery 2 — flow control leaked live frames.** With value dispatch
splicing frames on the loop's own tape (the R2 flip), a `break`/
`continue` escaping a spliced body let `handleLoopBreak`/`Continue`
discard the region INCLUDING the frame's un-executed cleanup tail —
leaking the per-call Args list, FnBaseline, and param bindings. A step
trace showed the next iteration's `args.0` reading the DEAD CALLEE's
args list. This bug pre-dated the unification for named dispatch too
(silently — the loop usually ended before the leak was observable).
Fix: `unwindLiveFrames` (fn_frame.go) replays each open frame's
canonical tail (`__DC`, `__pa`, force-forward `undef` pairs,
innermost-first) before the flow rewrite discards the region. Pinned by
`TestFlowUnwindKeepsPerCallStacksBalanced` (args-stack + binding
balance after break/continue) and the boundary rows.

**Resulting parity (all pinned).** Value-dispatched and named calls now
agree on every probed observable: application result, module-scope
resolution, `args.N`, def cleanup, raise, and — new with the unwind —
`break` (terminates the enclosing loop, `1`) and `continue` (skips the
iteration, `0`). A lone unnamed `Function` arg stays data; a sibling
can no longer trigger it; explicit `args.N` application fires exactly
once. Rows: `lang/spec/module-fnvalue-boundary.tsv`; Go pins:
`lang/go/native/args_inert_test.go`,
`lang/go/native/fnvalue_sameregistry_test.go`.

**Compiler status.** The `runFnBodyOnce` refusal guard STAYS (comment
updated): the unit model binds unnamed params to slots without pushing
them, so an unnamed arg *flowing to the residual* still diverges from
the interpreter — the guard's reason changed from "auto-dispatch" to
"frame flow", and it still refuses rather than miscompiles (gate at 17, tier
documented in compiled_coverage_test.go / design/P7-ENDGAME.10.md). That is
the better of two failures, not a resolution: the guard is an OPEN DEFECT,
because the rows behind it do not compile. Closing it means modelling
unnamed-param frame flow in unit lowering — a Stage-3+ work item, out of
scope for this change but owed.
