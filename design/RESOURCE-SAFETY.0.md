# RESOURCE-SAFETY

Design for **guaranteed-cleanup resource safety** in boru — the `ensure` and
`bracket` core words.

## Context

This is the implementation-ready expansion of idea #2 from
`design/legacy/effect-oriented-programming-in-boru-report.0.ignore` ("Resource safety:
`ensure` / `bracket` / `scoped`"), which ranked it the clearest concrete gap an
effect-oriented lens exposes in boru — and the one place where both EOP lineages
converge on the same primitive (ZIO's `acquireRelease`/`scoped`/`ensuring`, and the
algebraic-effects `finally` clause).

boru today reifies a failure as a value (`do […] error […]`, `raise`,
`design/legacy/ERRORS.8.ignore`) but has **no construct that guarantees a finalizer runs**. A
cleanup step written after a body simply does not execute when the body raises:

```boru
def h (open-something)
risky-thing-that-might-raise h     # raises → the next line never runs
h close                            # LEAKED
```

`do […] error […]` does not solve this: it *catches* the error (turning it into a
value) but a) it is the wrong tool when you want the error to keep propagating, and
b) the cleanup would have to be duplicated into both the success path and the
handler. What is missing is the inverse of `error`: run cleanup **unconditionally**
and then let the original outcome (value, error, or loop-control signal) continue.

This is a **design RFC only — no implementation code yet**, matching how other
subsystems were designed first (`PROCESSES.0.md`, `SERVICES.0.md`,
`STREAM-WORDS.0.md`).

### Honest framing: what actually needs cleanup today

A resource-inventory pass found that boru currently exposes **very few leakable
native handles**:

| Resource | boru-visible handle? | Released today by |
| --- | --- | --- |
| `timeout` / `interval` timers | **Yes** — `Ideal/Timeout`, `Ideal/Interval` | explicit `cancel` (`boru:time-util`) |
| File I/O (`read`/`write`) | No | implicit — opened-read-closed inside the handler |
| HTTP (`fetch`) | No | implicit — `defer resp.Body.Close()` inside the handler |
| SQLite | No | implicit — host-side `*SQLiteStore.Close()`, `defer stmt.Close()` |
| `Store` / `Array` / `Object` | Yes (stateful) | garbage collection |

So the *immediate* beneficiaries are narrow: timer `cancel`, and user-orchestrated
**undo-on-failure** logic (restore a `Store` snapshot, roll back a partial write,
release a hand-rolled lock). The primitive's value is mostly **forward-looking
infrastructure**: it becomes load-bearing exactly as the roadmap adds real handles —
file/socket handles, the actor-per-connection sockets and process lifecycles of
`PROCESSES.0.md` phase 3, and any future `open`/`close` or transaction API. Building
the control structure now, before those handles exist, means they can ship *with*
their safety story rather than retrofitting one. This RFC states that scope honestly
rather than implying boru is leaking handles today.

### Relationship to `legacy/ERRORS.8.ignore`

`ensure`/`bracket` are the *cleanup* complement to that document's *catch* surface.
They share the same engine machinery — bodies run through the `InvokeBody` seam, and
a failure is the same in-flight Go `error` that `do`/`error`/`raise` already produce
and catch. No new control-flow primitive is introduced; the only new behaviour is
"run this body whatever happens, then resume the original outcome."

### Relationship to the effect report (idea #1)

If static effect inference (`legacy/effect-oriented-programming-in-boru-report.0.ignore` #1)
lands, `bracket`/`ensure` are effect-transparent wrappers: the effect set of
`bracket [acq] [use] [rel]` is the union of the effect sets of its three bodies. They
add control structure, not effects, so they need no capability of their own (§7).

## 1. Scope decisions

1. **Two core words: `ensure` and `bracket`.** `ensure [body] [cleanup]` is the
   finalizer; `bracket [acquire] [use] [release]` is the acquire-use-release triple
   that threads a resource value through. A `scoped` convenience is deferred (§9,
   open question 4).
2. **Packaging = core `control` words.** They sit alongside `do`/`error` in
   `lang/go/native/native_control.go` (`controlNatives`), unqualified, not behind an
   `import`. Resource safety is a control structure, not a library.
3. **`ensure` does not catch.** A failing body re-raises *after* cleanup. To both
   clean up and catch, compose: `do [ensure [body] [cleanup]] error [handler]`. This
   keeps the two concerns (`error` = catch, `ensure` = finalize) orthogonal and each
   word single-purpose.
4. **Cleanup runs on every exit path** — normal return, `raise`/error, *and*
   `break`/`continue` flow-control escape — and is implemented with a Go `defer` so
   it also fires on an internal panic recovered at the engine boundary (§3, §4).
5. **The body's outcome is primary.** If the body fails *and* the cleanup also fails,
   the body's error propagates; the cleanup error is attached as suppressed data, not
   lost and not promoted over the original (§5).
6. **Names verified collision-free.** `ensure`, `bracket`, `scoped`, `finally` are
   unregistered. `guard` (`native_type.go:193`, the conditional-value word) and
   `using` (`native_query.go:154`, a query word) are taken and **not** reused.

## 2. Motivation & the gap

The target is the universal acquire-use-release shape, with cleanup guaranteed:

```boru
# ensure: cleanup always runs, body's outcome is preserved
ensure [
  start-transaction
  do-risky-writes              # may raise
] [
  finalize-or-rollback         # ALWAYS runs, even if the writes raised
]

# bracket: resource threaded through, released no matter what
bracket
  [ acquire-resource ]         # produce exactly one resource value
  [ use-it ]                   # runs with the resource on its stack
  [ release-it ]               # runs with the resource on its stack, ALWAYS
```

Without this, every effectful boru program that holds transient state has to
hand-duplicate its cleanup into a success path and one or more failure handlers — and
the `break`/`continue` escape path (which is *not* an error and so is invisible to
`error`) cannot be covered at all from boru today.

## 3. The enabling machinery (already present)

Three existing facts make this a small, low-risk addition:

- **`InvokeBody(r, body, inputs) ([]Value, error)`** (`eng/go/invoke.go`) is the
  universal seam every body-running word uses (`do`, `error`, `each`, `fold`, …). It
  runs a quoted body either VM-native (`r.Invoker`) or on a fresh sub-engine sharing
  the registry, pushing `inputs` first. `ensure`/`bracket` route through it unchanged,
  so they inherit compiled-closure execution for free.
- **boru failures are Go `error` values returned up the stack** (`eng/go/engine.go`
  `Run` returns `(nil, err)` and short-circuits on the first failing step). Because
  the failure unwinds as an ordinary Go return, a Go **`defer` in the handler is a
  reliable cleanup hook** — it fires on the error return and on a recovered panic.
- **`break`/`continue` are a separate signal** (`r.FlowCtrl`, `eng/go/flowctrl.go`),
  *not* an error: a sub-engine that hits `break` with no enclosing loop returns
  `(residual, nil)` and leaves `r.FlowCtrl` set for an outer frame to resolve. The
  finalizer must therefore run on the `err == nil && r.FlowCtrl != FlowNone` path too,
  and must **preserve** the pending signal across its own cleanup body (§4).

The `cancel`-on-`timeout`/`interval` pair is the existing precedent for
"handle + explicit cleanup word"; `ensure`/`bracket` generalise it from one fixed
release operation to an arbitrary cleanup body.

## 4. Runtime design (Go)

Both words live in `controlNatives` (`lang/go/native/native_control.go`), each a
single signature with `NoEvalArgs` on every body position and `BarrierPos: -1` (all
forward-eligible, canonical form). Forward calling convention means the handler sees
`args[i]` = the i-th body as written (`ensure [body] [cleanup]` → `args[0]=body`,
`args[1]=cleanup`).

### `ensure`

```
ensure [body] [cleanup] -> <body result>
```

Sketch (illustrative; exact helpers settle during implementation):

```go
func ensureHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) (out []Value, retErr error) {
    if !IsConcrete(args[0]) || !IsConcrete(args[1]) {
        return nil, r.BoruError("ensure_error", "ensure: body and cleanup must be concrete lists", "ensure")
    }
    body, cleanup := args[0], args[1]

    ran := false
    runCleanup := func(primary error) error {
        if ran { return primary }
        ran = true
        // A pending break/continue from the body must survive the cleanup,
        // and the cleanup itself must not be allowed to redirect control.
        savedFlow := r.FlowCtrl
        r.FlowCtrl = FlowNone
        _, cerr := InvokeBody(r, cleanup, nil)
        if r.FlowCtrl != FlowNone {            // cleanup tried to break/continue
            r.FlowCtrl = FlowNone
            cerr = r.BoruError("ensure_error",
                "ensure: cleanup must not break/continue", "ensure")
        }
        r.FlowCtrl = savedFlow                 // restore the body's signal
        return mergeCleanupError(primary, cerr) // §5 policy
    }
    // defer is the panic/early-return safety net; the explicit call below
    // is the normal path (so a cleanup error can surface in retErr).
    defer func() { retErr = runCleanup(retErr) }()

    res, err := InvokeBody(r, body, nil)
    if err != nil {
        return nil, err                        // defer runs cleanup, then re-raises
    }
    return res, nil                            // success: defer still runs cleanup
}
```

The single `defer` covers all four exit paths:

| Body exit | `InvokeBody` returns | `r.FlowCtrl` | cleanup runs? | then |
| --- | --- | --- | --- | --- |
| normal value | `(res, nil)` | `FlowNone` | ✅ | return `res` |
| `raise`/error | `(nil, err)` | `FlowNone` | ✅ | re-raise `err` (or merged, §5) |
| `break`/`continue` | `(residual, nil)` | `FlowBreak/Continue` | ✅ (signal saved/restored) | return `residual`, signal still pending for outer loop |
| internal panic | (stack unwinds) | — | ✅ (via `defer`) | panic continues to the engine boundary recover |

### `bracket`

```
bracket [acquire] [use] [release] -> <use result>
```

- Run `acquire` via `InvokeBody(r, acquire, nil)`. It must produce **exactly one**
  value — the resource `R`. Zero values is an `ensure_error`-style "acquire produced
  no resource" (the same void-group reasoning as `legacy/ERRORS.8.ignore` §3); more than one is
  an error (ambiguous resource).
- **If `acquire` fails, do not run `release`** (there is nothing to release) — the
  error propagates directly. This matches ZIO `acquireRelease` (the release is bound
  to a *successful* acquire).
- Run `use` via `InvokeBody(r, use, []Value{R})` — the resource is **pushed onto the
  use body's stack**, the same convention by which `error` pushes the caught error
  and `each`/`fold` push the element/accumulator. A body binds it idiomatically with a
  lambda param or `var [[h] …]`.
- **Always run `release` via `InvokeBody(r, release, []Value{R})`**, through the same
  `defer`-guarded path as `ensure`'s cleanup (so break/continue and panic are covered
  identically). `release` receives the *original* `R` captured at acquire time.
- Return `use`'s result on success; re-raise `use`'s error (merged with any release
  error per §5) on failure.

`bracket` is definable as `ensure` plus the resource threading; it is a distinct word
rather than sugar because the "exactly one resource, released-only-if-acquired"
contract is worth enforcing structurally.

## 5. Cleanup-error policy (decided)

Two failures can coincide (body fails *and* cleanup fails). The rule, leaning on
`ErrorInfo.Data` which already exists for `raise` payloads:

- **Body fails, cleanup succeeds** → body's error propagates unchanged.
- **Body succeeds, cleanup fails** → cleanup's error propagates (it is now the only
  failure).
- **Body fails, cleanup also fails** → the **body's error is primary** and
  propagates; the cleanup error is attached under a `suppressed` key on the primary
  Error's `Data` (`{code:…, message:…}`), so a programmatic `error [handler]` can read
  `e.suppressed` but the formatted report still shows the original cause. This mirrors
  ZIO's suppressed-cause handling using machinery boru already has, and never silently
  discards a failure.

`mergeCleanupError(primary, cerr)` in the sketch implements exactly this table.

## 6. Static analysis (check mode)

In `boru check`, both bodies are walked so their defs and diagnostics propagate
(`RunCarrierBody` / `RunCarrierBodyWithDefs`, `eng/go/carrier.go`):

- `ensure`'s static return carrier = the **body's** carrier result (cleanup's result
  is discarded at runtime, so it does not contribute to the type).
- `bracket`'s static return carrier = the **use** body's carrier result; `acquire`'s
  single-value carrier is pushed as the resource type into `use` and `release`.
- A `ReturnsFn` provides these so the checker infers a precise return type rather than
  `Any`, consistent with how `if`/`do` already model branch results.
- Tie-in to effect inference (report idea #1): the inferred effect set of an
  `ensure`/`bracket` node is the union of its bodies' effect sets — the wrapper itself
  contributes none.

## 7. Capability integration

`ensure` and `bracket` perform **no effects of their own** — they only sequence other
bodies — so unlike `spawn` (`PROCESSES.0.md` §7) they require **no capability** and
are available under every policy profile, including `sandbox`/`compute`/`read-only`
(`design/PERMISSIONS.10.md`). Whatever the bodies do remains gated by their own words'
capabilities; wrapping a `network` call in `bracket` neither adds nor removes the
`network` requirement. This is the correct stance: a control structure that could be
denied by policy would make cleanup *less* reliable, defeating the purpose.

## 8. Interaction with concurrency

- **`await` branches** each run on an isolated `ForkConcurrent` sub-engine
  (`native_temporal_await.go`). An `ensure`/`bracket` inside a branch runs its cleanup
  within that fork, on that fork's registry — correct and self-contained. Resources
  acquired in a branch should be released in the same branch (cross-branch sharing is
  already discouraged: messages are immutable, mutable state does not propagate back).
- **Future processes** (`PROCESSES.0.md`): when the actor runtime adds
  `context.Context` cancellation, a cancelled process unwinds its body as a Go return;
  `ensure`'s `defer`-based cleanup is exactly the hook that makes "release the
  connection when the process is shut down" work without extra machinery. `ensure` is
  thus the substrate for the eventual interruption story — noted, not built here.

## 9. Worked examples

```boru
# 1. Timer cleanup (works TODAY — uses the existing timeout/cancel handles)
import "boru:time-util"
bracket
  [ TimeUtil.interval 1000 [poll] ]      # acquire: returns an Ideal/Interval
  [ afn [tick] [ run-for-a-while ] ]     # use: resource pushed onto the body stack
  [ afn [tick] [ tick TimeUtil.cancel ] ] # release: always stops the ticker

# 2. Undo-on-failure with a Store snapshot (works TODAY)
def st (make Store {count: 0})
ensure [
  st set "count" 10
  risky-step                             # may raise
] [
  # always runs; restore is a no-op on success, a rollback on failure
  reconcile st
]

# 3. Catch AND clean up — compose ensure inside do…error
do [
  ensure [ open-and-use ] [ always-close ]
] error [
  dup .code eq 'io_error if [ drop {} ] [ reraise ]
]

# 4. File handle (FORWARD-LOOKING — open/close do not exist yet, see Context)
bracket
  [ open "data.json" 'read ]             # future handle-returning word
  [ afn [h] [ h read-all reify ] ]
  [ afn [h] [ h close ] ]                # guaranteed close once handles can leak
```

Note the conventions: the resource is **pushed onto the use/release body stacks**
(bound here with an `afn` param, exactly as an `error` handler binds the caught
error); `release` always receives the resource captured at acquire time; and `ensure`
re-raises so example 3 must wrap it in `do…error` to actually catch.

## 10. Testing

Per the repo's positive-with-negative discipline (`lang/go/CLAUDE.md`), a new
`lang/spec/resource-safety.tsv` battery plus Go tests:

- **Cleanup fires on every path:** body returns a value; body `raise`s; body
  `break`/`continue`s inside an enclosing `for`; nested `ensure` runs cleanups in LIFO
  order.
- **bracket contract:** resource is threaded to `use` and `release`; `release` runs on
  both success and failure; **`acquire` failure skips `release`** and propagates;
  `acquire` producing zero / multiple values errors.
- **Cleanup-error policy (§5):** body-ok + cleanup-fails propagates the cleanup error;
  body-fails + cleanup-fails propagates the body error with `e.suppressed` populated.
- **Flow-control preservation:** a `break` in a `bracket` body still breaks the outer
  loop *after* `release` runs; a `break`/`continue` *in the cleanup body* is an error.
- **No-effect / capability:** `ensure`/`bracket` run under the `sandbox` profile.
- **Panic safety:** a Go test (recover-based, like `TestTypeLiteralNoPanic`) asserts
  cleanup still runs when the body panics, and that type-literal args
  (`Data == nil`) error cleanly rather than panicking.

## 11. Phased roadmap

- **Phase 1 (this RFC):** `ensure` and `bracket` as core control words, with the
  all-paths cleanup guarantee, the §5 error policy, check-mode return inference, and
  the spec battery. No engine changes beyond two handlers — everything rides
  `InvokeBody` + a Go `defer`.
- **Phase 2:** a `scoped` convenience (open question 4) and, once real handles exist,
  `open`/`close`-style words that are *meant* to be used inside `bracket`.
- **Phase 3:** interruption-aware cleanup, when `PROCESSES.0.md` adds
  `context.Context` cancellation — `ensure`'s `defer` is already the right hook (§8).

## Open questions

1. **`release` return value** — discard it (current design: `release`'s result is
   dropped, like a cleanup), or thread it out? *Leaning discard* — a finalizer that
   produced a value would muddy `bracket`'s "return the `use` result" contract.
2. **Multi-resource `bracket`** — nest `bracket`s (current answer), or add a
   list-of-resources form `bracket [[acq1][acq2]] …`? *Leaning nest* for phase 1;
   nesting already gives LIFO release order for free.
3. **`ensure` arg order** — `ensure [body] [cleanup]` (body first, reads
   chronologically) vs `ensure [cleanup] [body]`. *Leaning body-first* (matches the
   `do [body] error [handler]` reading order: the thing that runs is written first).
4. **`scoped` shape** — boru has no block scope below the fn body, so a `scoped`
   resource that auto-releases "at end of scope" has no obvious scope boundary to bind
   to. Options: (a) make `scoped` pure sugar for a `bracket` whose `use` is the rest
   of the enclosing body (hard without a block construct), or (b) drop `scoped` and
   keep `bracket` as the one acquire-use-release form. *Leaning (b)* unless a concrete
   need appears.
5. **Cleanup step limit / re-entrancy** — should a cleanup body get a fresh step
   budget so a near-exhausted run can still clean up, or share the parent's remaining
   budget? *Leaning share* for phase 1 (simpler, and a runaway cleanup is itself a
   bug), revisit if it bites.
