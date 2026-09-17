# Defect investigation — root causes for `legacy/verse-in-boru-report.0.ignore` §6

The Verse comparison report
([`legacy/verse-in-boru-report.0.ignore`](legacy/verse-in-boru-report.0.ignore)) verified its boru
claims by running them, and seven defects fell out. This note is the
follow-up: for each, the cause **in the source**, the blast radius as
*tested*, and what a fix has to decide. Reproduced against `main` @
`ab0e1e0`.

Three items (A, B, F) are unambiguous engine bugs whose fix shape is
clear; two (C, D) need a design call named below; one (E) is mostly
documentation with one engine defect inside it; one (G) is an
already-recorded issue whose worst face is not recorded.

That was the assessment on arrival. Four of the seven have since been
repaired and one attempt was reverted, so read the status block next —
and note that **three of this note's own claims were corrected during the
repairs** (B's exemption for `case`, C's severity, and C's fix
condition). Each correction is kept in place rather than edited away,
because in every case the mistake is more instructive than the fix.

**Status of the repairs**, as of the latest revision:

- **A is fixed for `await`, including the compiled-path hole.** A branch
  that can reach a stateful container is REFUSED at the boundary, the
  answer `send` gives between processes. Cloning was implemented and
  measured first, then replaced: it made the same programs work silently.
  The other five `ForkConcurrent` call sites are deliberately unchanged.
  The compiled path used to permit a fn-local lambda that WRITES a shared
  container — a real data race, and this note twice described it wrongly
  (first as "narrow", then with the wrong mechanism). Both corrections and
  the fix are in §A; all four shapes now refuse in both engines.
- **C's miscompile is closed — and what stands in its place is a refusal,
  an open defect owed a fix.** `errorReturnsFn` refuses instead of
  widening a known arity, so the leaked `internal_error` is gone; a
  program a developer expects to compile still does not. §C now records
  exactly what its "user-visible by default" severity rests on: the
  effects fence, which blocks the silent interpreter re-run once the
  guarded block has emitted output.
- **F is fixed, return side included.** Fixes (1)-(4) landed earlier with
  regression tests — the two verified patches, the inline-union route via
  `paramBodyCarrier`, and the diagnostic reword. The sixth consequence
  those tests exposed (a declared union RETURN accepted but never
  enforced) is now closed too: `FnSig.ReturnPatterns` carries the
  constraint `ParseFnReturns` used to discard, enforced at both the
  runtime and check-mode return boundaries.
- **D is fixed, split mode included.** The advisory landed first; the real
  repair is now in too. `eng.CheckState.ModelEffects` is the third mode —
  concrete values, substituted effect BACKENDS — so `boru check` no longer
  writes, creates, removes or prints through a module body, while a body's
  reads stay real so nothing that loads today starts failing. §D also
  records the correction that mattered most: gating on "the parent is
  checking" DELETES a module body's effect from a default run, because on
  the compiled path the compile pass is the body's only execution.
  `!Check.Compiling` is what separates a preview from the run.
- **B is partially fixed, and the remainder is pinned rather than
  forgotten.** The first fix was implemented, validated and REVERTED; read
  §B before attempting another, because the fourth adversarial lens landed
  after the revert and widened the defect further. What has since landed:
  a cycle-safe `UpdateChain`, one named VM body-entry funnel with an AST
  gate keeping it from re-fragmenting, and a per-body context frame at that
  funnel — which covers every closure-unit body. Separately, an island
  apply no longer OVER-contains (an island continues an expression; it is
  not a body). Still open: the three INLINED forms (`case` arms,
  `otherwise`'s list argument, list auto-evaluation), which need an emitted
  opcode pair. Their design, and the non-linear-exit hazard that shapes it,
  are written up in §B — as are two rejected alternatives.
- **E is fixed for everything it documents.** The code-less runtime
  errors the table names now carry the code their own check-time mirror
  emits; the table names only codes that exist; and a gate re-derives the
  truth set from the source so it cannot drift again. **Both halves that
  were once deferred are now closed too**: every gated capability's
  refusal carries a dispatchable code (via a `policy.Denied` adapter, not
  an `Unwrap` — the reason is in §E), and the engine owns a registered
  enumeration of every error code, checked four ways. The gate's own blind
  spot — it could only see WELL-FORMED codes, so the one malformed code in
  the tree was invisible to it — is recorded in §E with the defect it hid.
- **G** is still diagnosis only — a pre-existing recorded issue.

Two defects found *by* this work rather than by the report are recorded
in §A: **`send`'s refusal does not cover FlexMap** (a message gap, not a
safety hole — the boundary clone makes it safe regardless), and
**`make test-race` was red on main** — 26 race reports at `ab0e1e0`,
before any change here. That one turned out to be a *product* defect, not
test hygiene: dynamic help **executes** the examples it synthesises, so
registering `interval` started a real ticker nobody owned. **Fixed; the
race lane is clean.**

Two findings grew materially during investigation. **F** was reported as a
`case`-exhaustiveness precision gap and turned out to be a parameter-type
erasure that also changes arity silently. **E** was reported as four
phantom error codes and turned out to include an engine defect (a
code-less runtime error) alongside two claims that were simply wrong.

### How much to trust this

B, C, D and F were each put through two independent adversarial reviews
briefed to *refute* them — one attacking citations first, one attacking by
experiment first. Result: **no root cause was refuted**, every symptom
re-reproduced, one verdict CONFIRMED and seven PARTIAL. The PARTIALs were
citation drift (off-by-one line numbers, one term of art used loosely),
blast-radius omissions, and one overstated rule — all folded in here, with
each correction re-verified before it was written down. The causal stories
in A-G are what survived that; the fix *scopes* moved more than the
diagnoses did.

Corrections worth naming because they change what a fix must touch: the
leak rule is not "lowered to a closure" alone (B), `outer` and `walk` also
leak (B), and one of this note's own earlier fix recommendations for D was
retracted as blocked rather than cheap.

One of those corrections was itself wrong and has since been reversed: an
earlier revision of this note recorded that **`case` does not leak**. It
does. The argument for the exemption (no `CallableSpec`, `runCaseBody`
calls `RunResolved`) is accurate about the interpreter and irrelevant to
the compiled path, which desugars `case` inline. That mistake is worth
keeping visible, because it is the same mistake in miniature that the
whole of B turns on: reasoning about one engine's call graph and reporting
the conclusion as if it were the language's semantics. See the corrected
paragraph in B and "Blocker 2 is worse than the table says".

| | Defect | Kind | Severity |
|---|---|---|---|
| **A** | `await`/`spawn` share mutable payloads → data race | soundness | **highest** — undefined behaviour, docs asserted the opposite; **`await` FIXED by refusal**, other fork sites unchanged by design |
| **B** | Compiled `do`/`each` leak `context set` to the parent | miscompile | high — silent wrong answer, default path |
| **C** | `do […] error […]` + trailing expr → leaked `internal_error` | miscompile | high — user-visible by default once the block emits output; **FIXED** |
| **D** | `boru check` runs module bodies; a default run runs them twice | correctness | medium — effects during a "no-run" command; **now REPORTED** |
| **E** | Error-code table documents codes the engine can't produce | docs + 1 engine bug | medium |
| **F** | Shorthand `fn` discards a `def`-bound union/enum param type | engine | **high** — silently made a 1-arg fn callable with 0 args — **FIXED**, param and return sides both |
| **G** | Un-separated forward calls evaluate right-to-left → stale reads | recorded, deferred | medium — invalidates tests silently |


## A. `await` branches share mutable container payloads — a data race

### Root cause

`ForkConcurrent` (`eng/go/fork.go:38`) isolates **execution scopes** and
nothing else. It clones `Defs`, `Types`, `builtinWords`, `Contexts` (with
a private child layer over the parent's top store) and `Args`. Its header
states the guarantee precisely and correctly:

> it shares the parent's read-only infrastructure but isolates every
> mutable execution scope. The fork observes the parent's bindings and
> context as of the fork call; its later writes stay private.

"Its later writes" means writes to those *scopes* — `def`, `context set`.
Cloning a binding table copies `Value`s, and a `FlexMap` / `FlexList` /
class-instance `Value` holds a pointer to a shared `*OrderedMap`. Nothing
deep-copies the payload, so in-place mutation is visible to every fork and
to the parent.

### Evidence

```
import "boru:time-util"
def m (make FlexMap {})
TimeUtil.await [[m set a 1] [m set b 2]] ;
m
→ [{b:2 a:1} {b:2 a:1}] {b:2 a:1}
```

Both branches and the parent observe both writes; a sibling reads an
earlier branch's write. An immutable `Map` behaves correctly by contrast
(copy-returning `set`, parent unchanged) — so the hazard is exactly the
mutable containers the docs name as safe.

`OrderedMap.Set` (`eng/go/value.go:85-91`) is an unsynchronised map assign
plus a slice append, so this is undefined behaviour rather than merely
surprising. `go build -race`:

```
WARNING: DATA RACE
Read at 0x… by goroutine 17:
  eng/go.(*OrderedMap).Set()      eng/go/value.go:87
  native.setFlexMapHandler()      lang/go/native/native_storage.go:749
  native.awaitAll.func1()         lang/go/native/native_temporal_await.go:104
Previous write at 0x… by goroutine 16:
  eng/go.(*OrderedMap).Set()      eng/go/value.go:90
```

### Wrong claims

- EXPLANATION.md:683 — "Mutable side effects within a branch are local to
  that branch's sub-engine."
- HOWTO.md:353 — "Each branch runs in a sub-engine, so writes to mutable
  objects inside one branch do not bleed into the others."

`native_temporal_await.go:23-25` is careful and correct about what it
*does* guarantee (forking on the dispatching goroutine makes reads of
parent scope state race-free); the user-facing prose overclaims from it.

### Fix decision

The safe rule already exists in the tree — `send` refuses to pass mutable
containers between processes — and `await`/`spawn` do not apply it.

1. **Deep-clone mutable payloads into each branch.** The only option that
   leaves the documented semantics standing. Costs a copy per branch per
   container; `AdoptIntoNode`/`AdoptIntoFlex` already do the traversal.
2. **Refuse at the boundary, as `send` does.** Cheap and loud, breaks any
   program that shares a flex container across branches today.
3. **Document sharing as intended and synchronise `OrderedMap`.** Fast,
   but makes `await` a concurrency primitive whose users must reason about
   interleaving — a large change to boru's story, and it would leave
   `deq`/rendering racing too.

Recommend (1), with (2) as the interim if a copy cost is unacceptable.

### RESOLVED via (2) — REFUSE at the branch boundary

The decision went to **(2), refuse**, not (1). `await` now rejects a
program whose branch can reach a stateful container — FlexMap, FlexList,
Store, Table, a class instance — with `not_sendable`, naming both the
binding and the kind. It is the answer `send` already gives at a process
boundary.

Option (1) (deep-clone into each branch) was implemented first and
measured: it worked, matched the immutable-Map oracle exactly, and cost
+16% allocs / +15% ns per `await` call. It was replaced because cloning
makes the same programs work **silently** — it hides the sharing the
author actually wrote, and pays a per-branch copy for every container in
scope whether or not any branch touches it. Refusing states the rule once,
at the point where it is violated.

**Three things the rework needed that were not visible from the analysis.**
Each is worth recording because each looked settled before it was tried.

1. **`sendableViolation` cannot be reused, despite being the same rule.**
   The obvious move — `await` calls `send`'s predicate, same package, same
   words — is wrong, and quietly so. `send` asks *can this be COPIED
   across?* and deep-copies what it admits (`CloneValue` at the boundary),
   so it refuses only containers a copy cannot preserve. `await` copies
   nothing and must refuse anything mutated **in place** — a strictly
   broader set.

   Concretely, `sendableViolation` misses FlexMap. A FlexMap is
   `MapPayload{*OrderedMap}` with a `TFlexMap` tag — *payload-identical*
   to an immutable Map — so a payload-keyed switch reads it as a plain map
   and recurses into it. That is harmless for `send` (the clone makes it
   safe either way; this is a refusal-message gap, **not** a safety hole)
   and fatal at a sharing boundary. The await predicate therefore
   discriminates on the **type tag**, which is what actually separates
   in-place mutation from copy-on-write.

2. **The check first shipped INERT on the default path.** The first
   version walked the branch element as a token list. Compiled, a branch
   body is a synthetic fn-value carrier holding its tokens in a
   `BoruImpl` body (the `CompileStoresBodyList` shape `runParallelBranch`
   unwraps), so the walk found nothing — the boundary was unguarded on
   the *only path most programs take*. It looked correct because the case
   that did fire had been tested with `--no-compile`. Both shapes are
   walked now, and every refusal case is asserted in **both** engine
   modes, which is the assertion that would have caught it.

3. **A lambda does not always CAPTURE what it reaches.**
   `def w ([] => [m set a 1])` with a module-level `m` captures nothing —
   per the capture rules a module-level def stays dynamic — so the
   reference exists only in the lambda's body. Checking `FnDefInfo.Captured`
   alone missed the shape a user is most likely to write. The walk follows
   into referenced function bodies as well, bounded by a `seen` set so a
   recursive fn terminates instead of looping.

**Scope.** `await` only. The other five `ForkConcurrent` call sites are
untouched: nothing documents timers, `spawn`, `service` or the net
listeners as isolated, an `interval` callback accumulating into a captured
Store is a reasonable idiom, and the process paths already have `send`'s
answer.

**Docs.** EXPLANATION.md and HOWTO.md were *false* under sharing and would
have been *misleading* under cloning — both imply you may share a mutable
container and that writes are isolated. They now state the refusal, show
the build-one-per-branch idiom, and say plainly that a container reached
only through a longer chain of calls is not detected: this is a boundary
rule, not a proof of non-aliasing.

**Still shared, deliberately:** a mutable container stored in the
CONTEXT rather than bound by `def`. The fork gives each branch a private
COW context layer, but a pointer stored in it is reachable. The published
repro binds through `def`, which is what this closes; the context path is
unverified and should not be claimed as covered.

### CORRECTION, then FIXED: the compiled-path gap was a live data race

An earlier revision of this section described the known limit as
"narrow", on the grounds that both engines catch a container named
directly in a branch. **That understated it.** Running the pinned program
under `-race` reported a genuine `WARNING: DATA RACE`: two branch
goroutines inside `OrderedMap.Set` at once, via `runParallelBranch` →
`vmContext.run`. On the DEFAULT execution path.

Measured, with `def m (make FlexMap {a:0})` inside a fn body and both
branches invoking the same fn-local lambda:

| lambda body | compiled (before) | interpreted |
|---|---|---|
| `[m]` | refused | refused |
| `[m get a]` | refused | refused |
| `[m size]` | **ALLOWED** | refused |
| `[m set a 1]` | **ALLOWED** — races | refused |

The compiled path refused the two harmless shapes and permitted the
mutating one. That is the worst possible shape for a partial check: it
fired where it did not matter and was silent where it did.

**How it was found is the point.** Not by reasoning — the limit was
written down as narrow and believed. It was found because `make
test-race`, once the interval leak stopped drowning it, reported the race
in a test written to *document* the limit.

#### The stated cause was wrong too

The comment blamed "`w` is unbound, so the indirection is not followed",
which does not survive contact: if `w` were simply unbound, all four rows
would be ALLOWED. Instrumenting the walk gave the actual mechanism:

- `[m]` / `[m get a]` — the branch element arrives as a **raw token
  list**, and `w` IS in `r.Defs`. The walk works.
- `[m size]` / `[m set a 1]` — the body compiles, so the element arrives
  as a **compiled fn-value carrier** and `w` is NOT in `r.Defs`.

So the split was never about the lambda; it was about whether the branch
body compiled. And the value is genuinely out of reach: `w` is not in the
carrier's `Captured`, not in `CompiledFnRef.Captures`, and not in the
unit's constant pool — the branch calls the lambda's unit by index, and
`m` exists only as a live value in a VM frame that a native handler
cannot address.

#### The fix: ask a different question

The measurement that mattered: **`m` was in `r.Defs` the whole time.**
Only `w` was missing. The walk dead-ended one step short of the container
it was looking for.

So the fix does not resolve `w` — it cannot. It records that a reference
went **opaque** (unresolvable in `r.Defs`, and not a bare literal) and,
when the walk finds nothing directly, asks the only sound question left:
*is anything mutable in scope for that opaque reference to have reached?*
If yes, refuse.

The opaque test began with three more exemptions — registered word, type
name, literal — and coverage showed two of them never firing. They are
not merely untested, they are **unreachable from this call site**: the
test runs only where `r.Defs.Top(name)` already missed, every registered
word also carries a `r.Defs` binding (`undef` of a builtin is refused as
`reserved_word`), and a kernel type name is converted by the parser into
a type literal so it never arrives as a Word at all. Instrumenting the
predicate across the whole Go test corpus, every name that reached it
missed both lookups. They were deleted rather than allowlisted — a guard
that cannot fire reads as protection that isn't there.

The literal case is the one that fires, and it is load-bearing: `true` /
`false` / `none` DO arrive as Words and never resolve, so without the
exemption `await [[true] [false]]` with any FlexMap in scope refuses two
branches that touch nothing. That case is now pinned.

All four rows now refuse in both engines, and `-race` is clean.

**The conditional is what makes it usable, and it was not optional.** An
unconditional "opaque reference → refuse" rejects the two idioms the docs
promote, because unresolvable words are everywhere: a map key walks as a
bare Word (`m set a 1` yields `a`), and a branch-local `def` is
unresolvable by construction. A first attempt at the naive rule would
have broken `[m set a 1]` over an immutable Map and the
build-one-per-branch pattern — both of which are in the accepted-cases
test, which is why they were caught before the rule was written rather
than after.

The residual imprecision is stated rather than hidden: a mutable
container in scope that the opaque reference could not actually have
reached is refused anyway. That is the conservative direction, and it is
the direction this boundary already chose when it picked refusal over
cloning.

### `make test-race` was RED on main — and it was a product defect

Running the race lane for A's verification surfaced a failure that has
nothing to do with A: `lang/go/test`'s `TestListAPIWithQuery` reports a
data race and fails under `-race`.

It is a **leaked `interval` callback**. The two stacks are a test's
`a.Run` → `CompileCheck` → `CheckState.Begin` *writing* check state
(`check.go:98`), against `RunTimerCallback` → `Engine.Run` → `stepWord` →
`CheckState.IsActive` *reading* it (`check.go:77`), on a goroutine created
by `startInterval` (`natives.go:706`). Something is firing an interval
while a later test drives a check pass on the same registry. The obvious
suspect — an earlier test that started one and forgot to cancel it — is
wrong; see the root cause below.

Measured on a git worktree at the **untouched branch base** `ab0e1e0`:

| tree | `WARNING: DATA RACE` reports |
|---|---|
| `ab0e1e0` (base, before any of this work) | **26**, test FAILS |
| this branch before the A fix | 6, test FAILS |
| this branch with the A fix | 1–24, test FAILS |
| this branch with the help fix below | **0**, test `ok` |

So the gate was already red before any change here, and A moved the count
down rather than up. The counts are **nondeterministic** — the A-fix row
is a range because repeated runs of the same tree gave 1 and 24 — so only
two things in that column are load-bearing: no run of any tree before the
help fix was ever clean, and every run after it was.

#### Root cause: dynamic help RUNS the examples it invents

The obvious reading — "a test started an interval and forgot to cancel
it" — is wrong, and following it wasted a pass. Rewriting every leaky
`interval` row in `lang/spec/module-time.tsv` to cancel its timer moved
the count **22 → 22**. Those rows do leak; they were not the source.

The source is in the product. With `EnableDynamicHelp`, registering a
word fires `OnRegisterHook` → `GenerateDynamicExamples`, which
**evaluates** each expression it synthesises so `describe` can show a
real result. For `interval` that expression starts a real ticker, on a
registry the generator does not own and will not outlive, and nothing
ever cancels it. The callback then runs `Engine.Run` on a forked registry
for the rest of the process — racing whatever the next test does with
check state.

That makes it a defect with a user, not just a test: **any embedder that
registers the temporal module after `MarkReady()` leaks a goroutine per
registration.** Finding it needed the full goroutine-creation stack; the
summary line names only the test that lost the race.

**Fix** (`lang/go/native/help/help.go`): `GenerateDynamicExamples`
returns early for a word whose result is a handle to something still
*running* — `interval`, `timeout`, `spawn`, `service`, `watch`. Their
`describe` entries keep the example text and lose only the executed
result, which for a word returning an opaque live handle was never the
informative part.

The rule is keyed on the word **name**, which is unsatisfying: a
return-type rule would generalise to any future timer-like word. It was
written that way first and it silently never fired — `SigInfo.Returns` is
**empty** for exactly these words (verified: `interval` and `timeout`
both report `returns=[]` from `BuildFuncInfo`). A rule that cannot fire
is worse than an explicit list, because it reads as covering the case.

Pinned at the seam by `lang/go/native/help/live_handle_example_test.go`
rather than by counting goroutines, which would be timing-dependent: eval
must never run for a live-handle word, and must still run for ordinary
ones (`add`, `sub`, `size`, `upper`) — a skip list that quietly grew to
cover pure words would disable dynamic examples, which is the whole point
of the feature.

The `module-time.tsv` rows are still worth cancelling and are cancelled,
just not because of the race. That is the lesson worth keeping: the first
plausible cause matched the symptom exactly and was still the wrong one,
and only a measurement — not the reasoning — separated them.

#### The fix exposed a test that never existed

`make cover-gate` then failed with exactly one uncovered statement:
`intervalAtomHandler` (`natives.go:686`) — `interval`'s ATOM overload,
where the callback is a bare word rather than a quoted body list.

Nothing tested it. Its coverage came entirely from the side effect this
fix removed: dynamic help synthesised an `interval` example and *ran* it,
which happened to dispatch that handler. So the feature that caused the
data race was also the only thing exercising the overload — and under
ADR-008's 100% floor, that made the gate green for a reason no one had
chosen. It would have broken the moment anyone changed how examples are
generated, and the failure would have looked like a coverage regression
rather than a missing test.

Closed with `TestIntervalWithWordCallback`, the mirror of the `timeout`
version that already existed, which cancels its ticker before asserting.
Worth stating plainly: a coverage gate proves every statement RAN, not
that anything checked what it did. Here it ran inside a help-text
generator that discarded the result.

#### A third defect fell out of writing those rows

Cancelling the timers needed a spelling that keeps the row's expected
value, and the natural one diverges between the engines:

```
import "boru:time-util"
def h (TimeUtil.timeout 1000 [1 add 2])
def t (typeof h)
TimeUtil.cancel h t
                              compiled → Timeout
                              interpreted → [boru/signature_error]:
                                cannot call `cancel` — no signature matches
```

`cancel` takes exactly one argument and `h` supplies it, so the trailing
`t` should simply remain on the stack — which is what the compiled path
does, and what the identical program does with **any other** trailing
value (`TimeUtil.cancel h 7` → `7`, in both engines). The trigger is
narrow: the trailing token must resolve to a **type literal**, and
`cancel` must be a **zero-return** word. `not true t` returns from both
engines fine, so it is not "a type literal in the token stream" on its
own.

**Sharpened, still not fixed.** The trigger is narrower than described
above: it is a bound WORD whose value is a type literal, not a type
literal in the token stream. Measured, with `def t (typeof h)`:

| form | compiled | interpreted |
|---|---|---|
| `TimeUtil.cancel h t` | `Timeout` | `signature_error` |
| `TimeUtil.cancel h Integer` | `Integer` | `Integer` |
| `TimeUtil.cancel h 7` | `7` | `7` |
| `TimeUtil.cancel h end t` | `Timeout` | `Timeout` |

So a type literal written LITERALLY is fine; only one reached through a
binding diverges, and an explicit `end` suppresses it.

The interpreted error names the cause: *"the argument was Timeout (a
Timeout)"*. The argument the matcher received is `t`'s VALUE, not the
handle `h` — so `cancel` was dispatched a SECOND time, with the trailing
token as its argument, and failed because neither of its two 1-arg sigs
(Timeout, Interval) admits a bare type literal. With `7` the second
dispatch is never attempted and the value simply stays on the stack; a
type literal is evidently viable enough for forward collection to commit
to it and then hard-fail the real match.

That puts the defect in `resolveForwardArgs` / `matchSignature` — the two
coordinated collection phases `eng/go/CLAUDE.md` explicitly says to read
`design/FORWARD-COLLECTION-PHASES.10.md` before touching, because their
stop conditions drift apart. Not attempted here for that reason: it is a
one-line-looking change in the least forgiving code in the kernel, and it
deserves the phase document in front of it rather than a guess appended to
an unrelated batch. Recorded here because it is exactly the
class this note keeps finding: the compiled path and the interpreter
disagreeing on a shape no spec row covered. Worked around in the rows
with an explicit `end` (`TimeUtil.cancel h end t`), which both engines
accept.

**Methodological note.** The first control I ran for this was wrong: I
used `git stash push` on files that were already committed, so it stashed
nothing and I re-tested the *fixed* tree while believing it was the
baseline. The conclusion happened to be right; the evidence was worthless.
The worktree at `ab0e1e0` is what actually establishes it. Two of this
note's corrections now trace to a bad control — see also §C's severity —
which is an argument for building the baseline from a known commit rather
than from the working tree.

**Still shared, deliberately:** the fork's context stack. `ForkConcurrent`
already gives each branch a private COW layer so `context set` is
isolated, but a mutable container *stored in* the context is reachable by
pointer. The published repro binds through `def`, which is what this
closes. That path is untested here and should be checked before anyone
claims context-stored containers are isolated too.

### Test gap

`make verify-bytecode` and the `-race` gate both run the corpus, but no
spec row shares a mutable container across `await` branches, so the race
is unreachable from the suite. One row of the shape above under `-race`
catches it.


## B. Compiled `do`/`each` leak `context set` into the parent scope

### Root cause

The context write boundary is implemented **only in the interpreter**, as
a property of `Engine.Run`: every sub-engine run pushes a fresh child
`StoreInstanceInfo` over the parent and pops it on exit
(`eng/go/engine.go:1172-1175`, `defer`-balanced).

The bytecode VM has no equivalent. `eng/go/vm.go` never calls
`Contexts.Push`/`Pop` — its only three `Contexts` references are read-only
`TopData()` handler arguments (vm.go:518, :1127, :1627) — and there is no
context-frame opcode anywhere in the VM or emitter. So when `InvokeBody`
(`eng/go/invoke.go:21`) routes a body through `r.Invoker`, the compiled
body runs at `eng/go/vm.go:410` (`return vc.run(cl.Unit, locals, nil)`;
`invokeClosureOn` spans :389-411) inside the **caller's** context layer.
`context set` then COWs and replaces that layer through
`ContextStack.UpdateChain`
(`eng/go/core_helpers.go:1999`, implemented at
`eng/go/contextstack.go:88-105`, which scans from the top and *replaces*
the matching entry). With no child entry to replace, it replaces the
parent's — so the write survives.

This is a structural omission, not a wrong branch: the frame was never
modelled in the VM.

### Why it surfaced now, and why the word set is what it is

`do`/`each`/`fold`/`scan`/`filter` grew a `CallableSpec`, so their bodies
now lower to real closure units instead of islanding —
`lang/go/native/native_control.go:14-21` says so explicitly ("no longer a
baked-const list re-run through an interpreter sub-engine at run time").
Words that still island keep the correct semantics **by accident**:
`for-each` has `CompileEffect: CompileFallbackBody` and no `Callable`
(`native_array.go:334-335`), so its body reaches `invokeClosureOn`, misses
the `ClosurePayload` cast (vm.go:390-395) and falls through to
`RunResolved` → `runPooledAt` → `Engine.Run`, which pushes.

**The rule is not simply "lowered to a closure".** Adversarial review
produced a live counterexample: `with-decimal` carries a `CallableSpec`
(`native_math.go:406`), lowers to a closure, runs through
`invokeClosureOn` — and does **not** leak (measured 1/1 compiled and
interpreted, against `each`'s 2/1 on the same shape). Its handler pushes
its own frame:

```go
// native_math.go:435-436
r.Contexts.Push(r.Contexts.Top())
defer r.Contexts.Pop()
```

The accurate rule is: **a word leaks iff its body lowers to a closure and
nothing else on the path pushes a frame.**

That counterexample is also the single best argument for the fix below —
it *is* the fix, already in production for one word, and the spec comment
above it (`native_math.go:404-405`) states the reason outright: "the
handler pushes the context around `InvokeBody` so the VM-run body's
BigDecimal ops read the override."

Two further leaking words were found on review and are confirmed here —
`outer` (`CallableSpec` at `native_array.go:428`) and `walk`
(`natives.go:337`), both measured compiled 2 / interpreted 1. So the
leaking set is at least `do`, `each`, `fold`, `scan`, `filter`, `outer`,
`walk`, plus nested `do`, list- and map-literal elements and
interpolated-string holes.

**`case` leaks too — an earlier correction in this note said otherwise and
was wrong.** The reasoning behind that correction was sound but answered a
different question: `caseHandler` → `caseClauses` (`conditional.go:78`) →
`runCaseBody` (`:355-363`) does call `RunResolved` directly and `case` does
have no `CallableSpec`, so `case` never reaches `InvokeBody` (`invoke.go`'s
doc comment at line 10 listing `case` among its clients really is stale).
But "no closure" means the closure-seam patch *cannot reach it*, not that
it is correct. Compiled, `case` does not run `runCaseBody` at all:
`caseReturnsFn` desugars the whole form to a nested-`if` chain
(`conditional.go:141-148`) that is emitted **inline into the caller's
unit**, so the clause body has no sub-engine and no frame. Measured on the
pristine tree, default mode, no fallback warning:

```
case 1 [ 1 [ context set y 77 5 ] 2 [ 6 ] ]
context has y
→ compiled: 1 5 true      interpreted: 1 5 false
```

So `case` is a context boundary interpreted and is not one compiled —
the same divergence as `do`, arrived at by a different route. It belongs
in the leaking set, and it is the clearest single case of why a
seam-level patch cannot finish this job (see "the VM inlines bodies"
below).

`otherwise` with a list argument is the same shape:
`false otherwise [ context set y 77 5 ]` then `context has y` gives
compiled `false true` against the interpreter's `false false`.

The other two rows of the report's table also have explanations:

- **`await` agrees between paths** because `ForkConcurrent` builds a fresh
  `ContextStack` and pushes its own child layer (`fork.go:55-58`),
  independent of which engine runs the branch.
- **`for` and `if` agree at 2** because neither is a boundary in *either*
  engine: `runForLoop` (`forloop.go:44-53`) and `if3Handler`
  (`native_control.go:534-547`) return mark/move tokens spliced onto the
  same engine's tape, so there is no sub-engine to push. **EXPLANATION.md's
  list of sub-engine creators is therefore wrong about `for`, and omits
  `for-each`** — a doc fix independent of the miscompile.

### It accumulates

The divergence is not a single end-of-loop artefact; the interpreter's
boundary is per body invocation (per element), and the compiled path
accumulates across elements:

```
context set 'z' 0 end each [drop context set 'z' ((context get 'z') add 1) end (context get 'z')] [1 2]
→ -force-compile: [1 2]      --no-compile: [1 1]
```

### Fix

Give the VM the frame the interpreter has: push a child context layer
around `invokeClosureOn`'s closure-unit run (`vm.go:389-411`) and pop it
on every exit path, mirroring `engine.go:1172-1175`. A frame-scoped
push/pop is preferable to an opcode, since it belongs to body invocation
rather than to any instruction.

Confidence here is unusually high because the patch already exists: it is
the two lines `with-decimal` runs today (`native_math.go:435-436`), on the
same seam, producing the interpreter's answer. Hoisting them from one
handler to `invokeClosureOn` fixes every leaking word at once and lets
`with-decimal`'s local copy be dropped. Use the handler's own registry
(`invoke.go:23` calls `r.Invoker(r, …)`), not the sub-registry.

### That fix was implemented, validated, and REVERTED — read this first

The three-line patch above was applied and put through the full gate and
an adversarial sweep (1762 test programs across four lenses — differential
hunting, the context model, cost and unwind lifecycle, and registry
choice; the fourth reported after the revert and is folded in below).
**It must not be landed as written.** It is recorded here so the next attempt starts
from the findings rather than from the same confident paragraph.

**The gate was not the reason.** Two checklist runs failed on the patched
tree — once on `TestTuiServeAllViewersGoneQuits`, once on the
`cover-gate` profile-generation step — and *both were contention
artifacts* on a 4-core box, reproduced clean in isolation afterwards
(`covergate` itself reported 100.0%, 64888/64888 statements). Neither was
caused by the patch. The patch was reverted for the three substantive
blockers below, all found by the adversarial sweep rather than by any
gate. That distinction matters for the next attempt: **`make test` and
`make cover-gate` both pass with this patch applied**, which is precisely
why the sweep was necessary.

**What worked.** Every divergence in the table above closed: `do`, `each`,
`fold`, `filter`, `outer`, `walk`, nested `do`, the per-element
accumulation (`[1 2]` → `[1 1]`), and the new-key repro (compiled `7` →
`unknown_key`, matching the interpreter). Shadowing inside a body,
`for-each`, `with-decimal` and prototype-chain reads were unaffected. No
wall-clock regression at 200k elements. A differential regression test was
written and verified to fail on 10 subtests without the patch.

**Blocker 1 — it turns a latent bug into an uncatchable crash.** This is
ship-blocking on its own.

```
context set 'mode' "test" end
do [ context.__sys.fs set mem true  1 ]
context has nope
```

Pre-patch: `1 false`, exit 0. Post-patch: `fatal error: stack overflow`, a
Go runtime fatal that the engine's `recover()` cannot catch and that no
interpreter fallback can rescue — **on the default path**, from a
documented idiom (the in-memory-FS toggle).

`ContextStack.UpdateChain` (`eng/go/contextstack.go:87-105`) relinks
`p.Prototype` *while walking* the chain. With two stack entries that both
reach `origRoot` only through their prototypes, the second walk re-finds
the already-relinked `newRoot` and sets `newRoot.Prototype = newRoot` — a
self-cycle. Pre-patch a compiled body had only one such entry, so the
cycle was unreachable; the patch adds the second. Any later missing-key
lookup then spins forever, and misses are routine: `native_helpers.go:73`
does a `ContextStoreLookup(r, "$decimal-precision")` on **every** BigDecimal
operation.

So `UpdateChain` must be made cycle-safe *before* any second context frame
exists — stop at `newRoot`, or skip entries already relinked.

**Blocker 2 — it is incomplete, so the invariant it advertises is false.**
The patch brackets only the `InvokeBody` closure seam. Still leaking,
each verified compiled-vs-interpreted:

| Path | Why the bracket misses it |
| --- | --- |
| `case` clause bodies | the compiler desugars `case` to an inline nested-`if` chain (`conditional.go:141-148`); no closure is ever created |
| `otherwise [list]` | the list argument is evaluated in the caller's unit |
| a body bound or passed as a **value** (`def b […]`, a `List` param) | the list is auto-evaluated inline as `OpMakeList` operands — and the write lands at *bind* time, before any `do`/`each` runs |
| module-exported `fn` bodies | compiled to a unit reached by the user-call opcode, not through `InvokeBody` |
| `service` handler callbacks | routed via `InvokeCallback` → `runUnitNested` (`vm.go:365`) / `RunUnit` (`vm.go:244`), neither bracketed |
| `def f [body]` | compiled to a user-fn unit reached through CALL_USER/RET |

`vm.go` itself documents `RunUnit` as "the DURABLE-callback twin of
`vmContext.invokeClosureOn`" — so the twin seams are named in the source
and the patch covered one of them. The service case is the worst: handler
dispatch is serialized but *shared*, so an escaping `context set` becomes
cross-request state (observed `2/3/3` compiled against the interpreter's
`2/2/1`).

**Blocker 3 — an unbudgeted per-invocation allocation.**
`ContextStack.Push` allocates a `StoreInstanceInfo` *and* an empty
`map[string]Value`: +2 allocs and +112 bytes per closure-body invocation,
on every `each`/`fold`/`filter` element, whether or not the body mentions
`context`. Measured with the repo's own `allocStatsPerOp` helper: +29-33%
allocs/op on each/fold, and the `do_body` alloc-guard row went 212 → 412
against its 500 ceiling — consuming two thirds of the headroom silently,
because it still fits. `bytecode_allocguard_test.go` states that
allocations are "the hard regression signal" and that a ceiling must never
be raised "without a documented reason"; the guard table has **no**
each/fold/filter row, so the largest regression class is uncovered.

The cost is avoidable: a frame is only observable if the body can reach a
context-writing word, so it can be gated on a static per-`CompiledFn` flag
set by the emitter, or the map made lazy.

### Blocker 2 is worse than the table says — the VM inlines bodies

The fourth adversarial lens (registry choice) landed after the revert and
sharpened the picture materially. Its own question came back **negative**,
which is good news for the patch's shape, and its incidental findings are
bad news for the patch's scope. Everything below re-reproduced by hand on
the pristine tree in **default** mode with no fallback warning.

**A third body-execution surface: list auto-evaluation.** A code body that
arrives as a *value* rather than a literal leaks — and it leaks at the
moment it is bound, not when it is run:

```
def b [ context set y 77 5 ] end
print (context has y) end          # compiled: true    interpreted: false
print "no-call" end                # b is never invoked
print (context has y) end          # compiled: true    interpreted: false
```

`def name [list]` auto-evaluates the list and binds the result (both
engines bind `[5]` — documented behaviour, `lang/go/CLAUDE.md` "`def name
<node>` binds a value"). The interpreter runs that evaluation in a
sub-engine (`autoEvalList` → `runPooledSub`, `engine.go:4218-4224`) and so
gets the `Engine.Run` frame; the emitter records the same list as an
`OpMakeList` over operands emitted **inline into the caller's unit**
(`RecordMakeList` / `recordMakeListInner`, same function), so there is no
frame. The identical divergence appears when the list is a fn argument,
with a body the callee never touches:

```
def run fn [[b:List] [Any] [ 0 ]] end
print (run [ context set y 77 5 ]) end   # 0 on both
print (context has y) end                # compiled: true   interpreted: false
```

**A correction to the lens's own report.** It filed the named-body `for`
and `if` divergences (`def b [context set y 77]` then `for 3 b`) as a
`for`/`if` seam gap. They are not: the write has already happened at
`def` time, and `b` is by then the inert list `[5]`. Deleting the `for`
line entirely leaves the divergence unchanged. `for` and `if` remain
non-boundaries in **both** engines, exactly as this note recorded. The
distinction matters because a fixer chasing the reported symptom would
instrument the wrong two words.

**The registry choice is right.** The lens instrumented `invokeClosureOn`
to compare the registry the patch pushes on against the body unit's actual
dispatch registry, and ran it over the repo's own corpora (`langspec`
including `TestSpecCompiledDifferential`, `docexamples`, `cliexamples`,
`engspec`) plus 94 hand-written cross-registry programs: **zero
mismatches**. Cross-registry closure travel is refused by the compiler and
falls back; `await`, `spawn` and `service` bodies run through `RunUnit` on
a fork whose `vc.r` *is* the fork. So `reg` is the correct stack, and the
patch's cross-registry surfaces (await branch bodies, module fn `do`/`each`,
timer and interval callbacks, service handlers' nested `do`) all moved to
agree with the interpreter. Nothing about the revert is a verdict on the
push target.

**The formulation that makes the scope obvious.** The interpreter pushes a
context frame at exactly **one** site — `engine.go:1172-1174` — and every
nested body reaches it, because spawning a sub-engine is the interpreter's
only way to run one. The VM runs bodies through at least **four** paths:
closure units (`invokeClosureOn`), user-fn units (CALL_USER/RET), durable
callbacks (`RunUnit` / `runUnitNested`), and inlining into the caller's
unit (`case`, `otherwise`, list auto-evaluation, and `if`/`for` — the last
two correctly). The reverted patch bracketed one of the four, and **the
inline path cannot be bracketed by any seam patch at all**: there is no
call to wrap. Either those forms stop being inlined when their tokens can
touch the context, or the frame becomes an emitted opcode pair rather than
a Go-side push around a call.

### What the sweep could not break

Recording the negatives, because they are what a second attempt gets to
keep rather than re-derive:

- **No behavioural regression, at scale.** The differential lens ran ~1425
  distinct programs — a 720-program combinatorial corpus (every ordered
  pair of twelve body-taking words nested two deep × five context
  payloads), 400 randomized 1-3-level programs, and targeted batteries —
  and found **zero** cases where the patch changed an answer the old
  compiled path had got right. Every behaviour change moved compiled
  *toward* the interpreter.
- **The unwind is balanced.** The deferred `Pop` was exercised through
  error, `raise`, `break`/`continue`, early return and tail calls without
  leaking or double-popping, including through `with-decimal`'s
  now-doubled push.
- **No new race at the closure seam.** A `-race` build under 400
  concurrent service calls and parallel `await` bodies each doing
  `do [context set …]` reported nothing new.
- The repo's compiled-differential gates (`TestSpecCompiledDifferential`,
  `TestSpecCompiledOrFallback`, `TestCompiledCombination`,
  `TestPropertyDifferential`) all pass on the patched tree — which is the
  point made above about the gate, restated from the other side: the
  corpus contains no row that would have caught any of this.

### One residual that belongs with defect A

`reg` can be a **shared module sub-registry**, and module sub-registries
are not forked. The patch's `Push`/`Pop` therefore mutates a
`ContextStack` that concurrent branches also touch. Under `-race`, a
module fn called from several `await` branches reports the patch's own
`Pop` racing the interpreter's `Engine.Run` — but the *unpatched* tree
reports **more** races on the same program (95 against 85, 65 of the old
ones already inside `ContextStack` via `CowSet`/`UpdateChain`), the
interpreter dies on it with `fatal error: concurrent map iteration and map
write`, and the compiled answers are nondeterministic. There is no oracle,
so it is not a defect of the patch — it is defect **A**'s hole seen from
another angle, and the patch adds one more writer to it. Whatever fixes A
has to cover module sub-registries, not just container payloads.

### What a correct fix requires, in order

1. Make `UpdateChain` cycle-safe. Nothing else can land first.
   — **DONE.** The walk now stops at `newRoot` rather than relinking it.
   `newRoot.Prototype == origRoot` by construction, so a walk that reached
   `newRoot` matched the relink condition and produced
   `newRoot.Prototype = newRoot`. Stopping there is sufficient: the chain
   past `newRoot` continues to `origRoot` and then to `origRoot`'s
   ancestors, none of which have `origRoot` as their prototype.

   `TestContextStackUpdateChainNoSelfCycle` pins it, structurally and by a
   bounded chain walk, and was confirmed to fail without the guard. Getting
   the repro right took a correction worth recording: **two sibling entries
   are not enough.** With both pointing straight at `origRoot`, each walk
   relinks in a single step and never advances far enough to meet
   `newRoot`, so the first repro passed on the unguarded code and made the
   guard look unnecessary. It needs two *nested* layers
   (`inner → outer → origRoot`) — which is exactly the shape a second
   context frame creates, and exactly why this is step 1.
2. Bracket **all** body-entry seams, not one: `invokeClosureOn`,
   CALL_USER/RET, `RunUnit`, `runUnitNested` — or establish a narrower,
   honestly-stated invariant. Note that this step cannot be completed by
   bracketing alone: the `case` desugaring, `otherwise`'s list argument
   and list auto-evaluation are **inlined**, so they need either an
   emitted frame opcode pair or a rule that stops inlining a body whose
   tokens can reach a context-writing word.

   — **Enabled, not done.** The three re-entrant seams now funnel through
   one named function, `vmContext.enterBodyUnit`, which is where the
   interpreter's single `Engine.Run` site finally has a counterpart. Its
   doc comment carries the five-way map (the three funnelled paths,
   CALL_USER's intra-loop frame push, and the inlined forms) so the count
   lives in the source rather than only in this note — the absence of that
   count is what let a one-of-four patch read as complete.
   `TestVMBodyEntryIsFunnelled` is a source-shape gate: it AST-scans
   `vm.go` and fails on any `vc.run` call outside the funnel and the
   top-level program entry. A source gate rather than a behavioural one
   because the property has no runtime symptom until something is added to
   the seam — which is precisely how it was lost the first time.

   The funnel deliberately changes **no behaviour**: it adds no frame, so
   it cannot hit blocker 1, blocker 3, or advertise a false invariant. It
   also removed a real duplicate — `invokeClosureOn` had its own copy of
   the param/capture locals split and now shares `bindUnitLocals` with the
   other two seams, so there is one binding rule as well as one entry.

   — **NOW DONE for every seam that can be bracketed, with the residue
   named.** `enterBodyUnit` pushes a child context layer and pops it on
   every exit path. Measured across both engines, the whole closure-unit
   class closed: `do`, nested `do`, `each`, `fold`, `filter`, `outer`,
   `scan`, and those words nested inside a fn body all now agree, as does
   the per-element accumulation repro (`[1 2]` → `[1 1]`).

   **CALL_USER needs no bracketing at all, and the note's Blocker-2 table
   was wrong about it.** Measured: a named fn body and a called lambda leak
   the write in BOTH engines. They are not boundaries interpreted either,
   so bracketing CALL_USER would not have completed the fix — it would have
   introduced a NEW divergence, in the other direction. The differential
   test keeps rows for both so a future "finish the job" pass has to
   confront the measurement instead of the table.

   What genuinely remains is the inlined set, and it is unchanged: the
   `case` desugaring, `otherwise`'s list argument, and list
   auto-evaluation emit the body's tokens into the caller's unit, so there
   is no call to wrap and only an emitted opcode pair or a lowering rule
   can reach them. Each is a `wantDiverge` row in the differential test
   rather than an omission.

   One residual sharpened while pinning it, then **root-caused** on a
   later pass. The map-slot-lambda divergence the note recorded is
   source-shape-dependent — `(m.f 1) drop` diverges (compiled contains,
   interpreted leaks) while the bare `m.f 1 drop` agrees — and the
   disassembly says why:

   ```
   (m.f 1) drop → CALL_DYN_METHOD  ; (paren apply)/1 -> 1
   m.f 1 drop   → uncompilable: member fn value auto-applies mid-expression
                  (fn-value-call boundary, Stage 3)
   ```

   `callDynMethod` **islands** the apply: `islandRun` builds a sub-engine
   and calls `eng.Run`, and `Engine.Run` is exactly where the interpreter
   pushes its per-body context layer. So the compiled path is *more*
   isolated than the interpreter here — it delegates the call to an
   interpreter island, and the island's `Run` pushes a frame the
   interpreter's own inline dispatch of the same tokens never pushes. The
   fix therefore belongs at the ISLAND ENTRY (an island continuing an
   in-progress expression is not a body), not at the VM's body seam.

   Two corrections fall out, both worth more than the row itself:

   - It is **not** `enterBodyUnit`'s frame. A hypothesis that §B's own fix
     had created this row was tested by disabling that frame outright —
     `Push` and `Pop` both removed and counted — and the row still
     diverges. An earlier probe that removed only the `Push` suggested
     otherwise and was **unsound**: it left a `Pop` running against the
     parent's layer. Same failure mode as the `UpdateChain` repro recorded
     above, and the same lesson — verify the probe removed what it claims.
   - The ungrouped twin agrees **trivially**. That form does not compile at
     all — itself an open defect, owed a fix — so the runtime silently
     re-runs the program on the interpreter and both sides of the
     comparison ARE the interpreter. The pair is still the finding, but the
     difference between the two rows is that one compiles and the other is
     refused — and the refusal is what looks like agreement.
3. Gate the frame on a static "this body may touch context" flag so the
   allocation is paid only where it is observable, and add an
   each/fold/filter row to the alloc guard.
   — **DONE via the second half of the disjunction, plus the rows.** The
   note offered "a static per-`CompiledFn` flag set by the emitter, or the
   map made lazy". The static flag was NOT taken: a sound one has to be
   conservative about anything it cannot see through (a `CALL_USER`, a
   dynamic dispatch, a nested closure), which makes it true in almost every
   real body and buys nothing while adding a way to be silently wrong — a
   flag that under-reports does not cost allocation, it loses the write.

   The map was made lazy instead, and it turned out not to be a
   micro-optimisation: **every** context write goes through `CowSet`, which
   builds a whole new layer rather than mutating the pushed one, so a
   pushed layer's `Data` map was never written to at all. It is now nil
   (`StoreInstanceInfo.Set` allocates on demand for the one path that does
   write in place). Measured on `for 100 [do body]`: 212 unpatched → 412
   with an eager map → **312** with the lazy one. Half the frame's cost was
   an allocation nothing could ever read.

   The guard rows blocker 3 asked for by name are in
   (`each_scalar` 79/110, `fold_scalar` 77/110, `filter_scalar` 89/120),
   and `do_body`'s ceiling is LOWERED 500 → 400. That last part is the
   point rather than housekeeping: at 500 the frame consumed two thirds of
   the headroom silently, which was the actual objection. At 400 the next
   increment has to declare itself.
4. Land the differential regression test (written, and confirmed to fail
   on 10 subtests without the fix).
   — **DONE**, as `lang/go/context_boundary_differential_test.go`, and it
   does two things the reverted version did not. It asserts BOTH engines
   per row (the original gap survived because the two tests that assert
   these semantics by name run interpreter-only), and it pins the
   still-broken forms as `wantDiverge` rows, each carrying its reason. A
   suite that covered only the fixed half is how this got here; a row that
   starts agreeing fails loudly and asks to be reclassified.
5. Only then correct EXPLANATION.md's boundary list, which is
   independently wrong: it names `for` (not a boundary in either engine)
   and omits `for-each` (a boundary in both).
   — **STILL BLOCKED, and now measurably so.** The blocker is stated at the
   end of this section: the interpreter is itself inconsistent about which
   call forms are boundaries, so there is no single true list to write
   down. The full measurement is now in the differential test, and it
   confirms the problem rather than dissolving it — `if` and `for` are not
   boundaries in either engine, a named fn body and a called lambda are not
   boundaries in either engine, closure bodies are boundaries in both, and
   a paren-grouped map-slot method call is a boundary compiled but not
   interpreted. Writing the list requires DECIDING which of those is
   correct, which is a language-semantics call, not a documentation edit.

   **The call has since been made, and it settles the list.** The
   INTERPRETER is canonical in every remaining case, with one stated rule
   for the shape that looked like a counter-example: *a paren form is never
   a boundary — the value is resolved and then made available to the
   signature matcher.* So the remaining work is not "decide", it is
   "conform", in two opposite directions:

   - `case` clause bodies, `otherwise`'s list argument and list
     auto-evaluation must BECOME boundaries when compiled. All three are
     inlined into the caller's unit, so this needs an emitted context-frame
     opcode pair (plus a context-depth restore at unit entry/exit, so a
     mis-emitted pair cannot silently leave the stack unbalanced) — there
     is no call to wrap.
   - The paren-grouped map-slot method call must STOP being one, at the
     island entry per the root cause above.

   The second of those is **DONE**: `Engine.Run` no longer pushes the frame
   for an ISLAND (`Engine.isIsland`, reading the existing `flowUnwind`
   marker — the two island entry points are its only setters, so one field
   cannot drift out of step with a second consequence the way two would).
   The paren-grouped row is out of `wantDiverge`.

   Only after the FIRST does EXPLANATION.md's list become writable, and it
   will then also need `for` removed and `for-each` added.

#### The inlined-body frame: design, and the hazard that shapes it

The three inlined forms need an emitted pair — call it `OpCtxPush` /
`OpCtxPop` — because there is no call to wrap. The naive version is a
one-line VM case each, and it is **wrong in a way that is worse than the
defect it fixes**: a raw pair leaks a layer whenever control leaves the
region non-linearly, and a leaked layer sits on the REGISTRY and outlives
the run, silently rescoping everything after it. The escapes that matter
are real shapes: a `break`/`continue` out of a `case` arm inside a loop, a
`RET` from inside an arm, and a trap.

Do NOT hand-roll the balance. The VM already has the exact precedent, for
exactly this problem — dynamic-scope bindings, which a discarded frame must
tear down or an `OpBindDynScope` install stays readable in `Defs`:

- `vc.dynBinds` is a slice on the vmContext, not a local;
- each `vmFrame` records a `dynBase` into it;
- `vc.unwindDynBinds(base)` truncates;
- `flowSignal` calls it per discarded frame, alongside `stack =
  stack[:lp.iterBase]`.

Mirror it: `vc.ctxFrames` (recording the *Registry* each layer was pushed
on, since `curReg` changes for a module unit), a `ctxBase` on both
`vmLoop` and `vmFrame`, an `unwindCtxFrames(base)`, and calls at the same
three points `dynBinds` already unwinds at — plus a truncate-to-entry-depth
net at `vc.run`'s exit and in `runVMEntry`, so any remaining imbalance is
bounded to one request instead of corrupting the registry.

Two options were considered and rejected:

- **Emit each arm as a closure UNIT** so the existing `enterBodyUnit`
  covers it, balanced, with no new opcodes. Rejected: a unit boundary
  breaks `break`/`continue` out to an enclosing loop — the very thing the
  island flow-escape translation exists to paper over — so it trades a
  scoping bug for a flow bug.
- **Refuse to compile the shape when the region touches `context`.**
  Rejected twice over. First on DOCTRINE: valid code is owed a compile,
  so every shape the compiler refuses — the ungrouped method call above
  included — is an open defect owed a fix, and this option proposes
  minting one deliberately instead of teaching the emitter the frame.
  Second on ANALYSIS: a called fn can touch context, so a syntactic scan
  misses cases, and the complete version — refuse on any call in the
  region — fires on nearly every `case` arm. This is the same argument
  that killed blocker 3's static per-`CompiledFn` frame flag: a flag that
  under-reports does not cost allocation, it loses the write.

The emission spans still have to be located: the `case` desugar's arm
fragments, and the inline list auto-evaluation that serves both `def name
[list]` and `otherwise`'s / a fn's list argument (the `OpMakeList` operand
region). Cost control matters there — bracketing every list literal would
pay a frame on a hot path — but the region is narrower than it looks, since
a fully-constant list bakes to `PUSH_CONST` and never emits operands.

### Pre-existing defects found while validating the patch

Not caused by it, reproduced on the unpatched binary, all
`-force-compile`-only unless noted:

- `do [context set a 2 end do [context set a 3 end] (context dot a)]` →
  `STORE_LOCAL stack underflow`; interpreter is clean.
- `do [var b 5 (context dot a)]` → `CALL_DYNAMIC underflow`; interpreter
  gives a clean `signature_error`.
- `({k:1} each [drop (outer [drop 7] [1]) get 0]) dot k` → recovered panic
  "index out of range [2] with length 2"; interpreter clean.
- A module fn cannot read a caller-scope context key on the compiled path
  (`unknown_key` compiled, value interpreted) — `RunModuleBody` snapshots
  the parent's top layer at import time (`native_module_module.go:162-163`),
  and under the VM the top-level run has no such layer. This is the mirror
  image of defect B and belongs in the same audit.
- The compiled path renders the `unknown_key` caret span with 11 carets
  where the interpreter uses 3 — harmless today, but it will break any
  future gate that compares stderr byte-for-byte, and defect B's fix
  routes *more* programs onto exactly this error.

  **FIXED, and it was one typo causing two user-visible defects.** The
  Store `get` handler raised
  `r.BoruError("unknown key_error", "unknown key: …", "unknown key")` — the
  message's leading word glued onto the code, and passed as the WORD too.
  So the code contained a SPACE, which no `case` arm can match (the failure
  was undispatchable), and the compiled path renders the caret span at
  `len(Word)` — 11 for "unknown key" against the interpreter's 3 for the
  real token. Corrected to `key_error` (the code the getr twin's own comment
  names as the historical one) with `get` as the word; both engines now
  render an identical 3-caret span and a dispatchable code.

  **And it exposed a blind spot in §E's gate, now closed.** The
  extraction pattern only matched well-formed codes, so a malformed one was
  invisible to every check — not reported, not counted as mintable, not
  required to be in the enumeration. `TestEveryAttachedCodeLiteralIsWellFormed`
  now scans EVERY string literal handed to an error constructor. It found
  four more codes the enumeration was missing (`expected-byte`,
  `bad-encoding`, `cancel-timeout_error`, `cancel-interval_error`), which
  settled the naming rule on the property that actually broke: a code must
  be SPELLABLE as a `case` arm, so a hyphen is fine (boru word names use
  them) and a space or a capital is not. The enumeration went from 233 to
  241 codes, and `errorcodes.go`'s claim that every existing code was
  snake_case is corrected — that was an artefact of how the gate found
  them.
- `[1 2 3] each [ do [ 9 drop ] ]` → compiled raises `[boru/each_error]:
  element 0: body produced no result`; the interpreter returns `[1 2 3]`.
  **Default mode**, no fallback rescue. A nested body that leaves no
  residual is a value the two engines disagree about, unrelated to
  context; the fn-shaped twin (`def run fn [[b:List] [Any] [ do b ]]` over
  `[ 9 drop ]`) fails the same way as a return-arity error.

  **Root-caused, five fixes rejected, NOT fixed** — pinned as a
  `wantDiverge` row in `lang/go/do_empty_residual_test.go`, whose comment
  carries the detail. The cause is in `doListReturnsFn`: a non-empty body
  with an empty residual is modelled as `[Error]` on the reasoning that
  "that is exactly the shape a raising body leaves", discriminated from
  `do []` by token count. But an empty residual has TWO causes — the body
  RAISED (one caught Error) or it COMPLETED net-zero (nothing) — and token
  count separates neither. For the second the model over-reports by one, so
  the enclosing each-body unit is analysed at `[element, Error]` and emits a
  unit returning neither.

  What was tried, so the next attempt starts past it: modelling it empty in
  the compile pass (every net-zero `do` becomes UNLOWERABLE — a `do`
  dispatch with zero modelled outputs cannot be seated, so
  `def b (quote [break]) for 5 [do b i]` stops compiling); modelling it
  empty everywhere (also breaks `do [raise "x"] dot code`, which then
  compiles and raises `signature_error` where today it is refused and
  silently re-run); `MarkUncompilable` (refuses the whole
  `do […] error […]` family — ten tests pin those paths by name);
  `SetCatchVariadic`, the mechanism built for a runtime-variable count
  (the latch IS consumed — verified by instrumenting `catchVariadicFor` —
  but marking the `do` event variadic does not stop the ENCLOSING closure
  being seated at the over-reported count); and a gradual
  `dynamic(Any)` result (one output so still lowerable, variable so the
  consumer should decline — it does not).

  Discrimination is not the blocker: `doBodyMayRaise` is too conservative
  to serve (any registered word is fallible, so `drop` qualifies), a
  diagnostic delta around the body analysis is zero for BOTH shapes
  (measured), but a `CompileDiverges`-keyed token scan does separate them.
  Every model it then selects still hits one of the five. So the fix is not
  in the model — the mismatch between an analysed residual and what the
  unit actually RETs is a closure-body SEATING question in the emitter, and
  that is where the next attempt belongs.
- A lambda in a **map slot**, called as a method, loses its context write
  compiled but keeps it interpreted (`def m {f: ([a:Integer] => [context
  set zz 9 end a])}`) — the *opposite* direction to defect B, and
  pre-existing. So the interpreter is itself inconsistent about which call
  forms are context boundaries. That inconsistency has to be settled
  before EXPLANATION.md's boundary list can be made correct, since there
  is currently no single true list to write down.

### Test gap — the sharpest one here

The intended semantics are **already asserted by name in Go tests**, but
only against the interpreter:
`lang/go/native/context_test.go:98` `TestContextSubEngineIsolation`
("parent context should be unchanged after sub-engine write") and `:120`
`TestContextSubEngineNewKeyDoesNotLeak`. Neither runs under the compiler.

The spec corpus is more interesting still. `lang/spec/edge-containers-2.tsv:101`
asserts the shadowing half and explains the omission of the other half in
its comment:

> a child write SHADOWS inside the child (it also VANISHES when the child
> ends — that shape is pinned interpreter-side only, **as it currently
> compiles to a fallback island**)

So the gap was not "we know this is broken". It was "we know this is
untested on the compiled path, and here is why that is safe" — and the
reason stopped being true when `do` gained a `CallableSpec` and began
lowering to a closure instead of islanding. The comment's premise was
invalidated by a later change, and nothing re-examined it.

That is the transferable lesson: a spec comment justifying an omission by
appeal to *current* lowering behaviour is a landmine, because the
justification is invisible to the gate that would otherwise catch the
change. Either the row covers both modes, or the claim belongs somewhere a
lowering change has to confront it.

Running the two Go tests in both modes, plus a spec row for the vanish
shape, closes the immediate hole.


## C. `do […] error [handler]` + a trailing expression → leaked `internal_error`

### Root cause

An **arity model error at the `error` ReturnsFn seam**, not a
stack-discipline bug in the lowerer.

`errorReturnsFn` (`lang/go/native/native_control.go:1272-1301`) runs the
handler during the compile pass over a seeded `Error` carrier
(`RunCarrierBody`, line 1282), so it holds the resulting stack and can see
`len(stk) == 0` for a handler like `[drop]`. At lines 1289-1290 it treats
"not exactly one" as *unknown* and returns the wide one-value
`dynamic(Any)` bound instead of refusing. The runtime handler
(`errorHandler`, lines 1319-1337) returns `InvokeBody`'s residual
verbatim, with no padding to the declared `Returns: []*Type{TAny}` — so
`error` genuinely produces zero values.

Everything downstream is faithful to that wrong model:

1. `error` declares `CompileEffect: CompileFallbackBody` (line 1188) and
   its handler nets 0, so the closure path (which needs
   `CallableSpec.BodyOut: 1`) declines and the dispatch is recorded as a
   single-output interpreter island — `RecordFallback(span, ins, outs[0],
   pos)` at `eng/go/carrier.go:1888`.
2. `FallbackSpan` (`eng/go/bytecode.go:811-815`) has **no out-count
   field**, so the island's real arity is unrepresentable.
3. `lowerFallback` unconditionally pushes exactly one simulated slot
   (`eng/go/lower.go:2433, 2453`).
4. `Finalize` reconciles the residual, `resolveDynamicApply` sees a
   leading `dynamic(Any)` over static args and classifies it as a runtime
   fn-value apply (`eng/go/emit.go:7143`), emitting `OpCallDynamic /1`
   (`emit.go:7635`).
5. At run time `runFallback` (`eng/go/vm.go:1147-1171`) appends the
   island's real 0 results with no count check, so the stack is one short
   and `callDynamic`'s guard fires (`eng/go/vm.go:673-674`).

The check pass shows the mis-model without compiling at all: `boru check`
on the repro reports the residual as `dynamic(Any) Integer` where the
interpreter's real residual is the single value `5`.

### What "user-visible by default" rests on — the effects fence

The severity claim turns entirely on one detail, worth stating plainly
because two of the three headline variants do *not* support it on their
own. Measured against a binary built from the commit before the fix:

| Program | `-force-compile` | **default** | interpreted |
|---|---|---|---|
| `do [1 div 0] error [drop]` ⏎ `2 add 3` | `CALL_DYNAMIC underflow` | `5` | `5` |
| `1` ⏎ `do [1 div 0] error [drop]` | `CALL_DYNAMIC underflow` | `1` | `1` |
| `def x (do […] error [drop])` ⏎ `x` | `BIND_GLOBAL underflow` | `undefined_word: x` | same |
| **`do [1 print  1 div 0] error [drop]`** ⏎ `2 add 3` | `CALL_DYNAMIC underflow` | **`internal_error`** | `1` then `5` |

For the first three the whole-program interpreter re-run catches the
underflow and takes over **silently**: the user sees the right number
while a miscompile goes unreported, at the cost of a wasted compile, an
unannounced runtime bail, and a defect hidden rather than closed. The
fourth is the one that bites: once the guarded block has emitted output
the fallback is **deliberately blocked** — re-running would duplicate the
output — and the engine says so in the note it attaches:

> the interpreter fallback was blocked: output was already emitted, so
> re-running would duplicate it; run with --no-compile and report this as
> a compiler bug

So an `internal_error` reaches the user in the default configuration, from
ordinary source, with the engine itself asking to be told about it. The
report's §6(b) had this exactly right; an earlier revision of *this* note
called the claim overstated, on the evidence of the three variants that
are rescued — which is the mistake of generalising from the cases that
happen not to trip the fence.

Post-fix, that fourth program refuses at compile time, prints `1`, and
returns `5`. The leaked `internal_error` is gone; what stands in its place
is a program the compiler will not compile, which is an open defect owed a
fix, not a resting place.

### Three variants, one cause

| Program | compiled | interpreted |
|---|---|---|
| `do [1 div 0] error [drop]` ⏎ `2 add 3` | `CALL_DYNAMIC underflow` | `5` |
| `1` ⏎ `do [1 div 0] error [drop]` | `CALL_DYNAMIC underflow` (pc=4) | `1` |
| `def x (do [1 div 0] error [drop])` ⏎ `x` | `BIND_GLOBAL underflow` | `[boru/undefined_word]: undefined word: x` |

A handler that leaves a value (`error [drop 9]`) is fine, and so is a
program that ends at the `error`.

### Why the adjacent no-error case refuses instead of leaking

`do [1] error [drop]` + a trailing expression refuses with
`force-compile: code-body word error (Stage 2)` for an unrelated reason: a
statically concrete do-result bakes into the island span, and a baked arg
beyond the signature's `BarrierPos` (1 for `error`) is rejected
(`eng/go/carrier.go:1878-1879`), so the island declines and
`recordCallRefusal` marks the program uncompilable. Both shapes are
defects — this one refuses to compile, the error-fired one miscompiles —
and the only difference is that this one announces itself.

### Fix

Two independent repairs, both worth doing:

1. **Make `errorReturnsFn` refuse instead of widening.** It already knows
   the true arity — it ran the handler. When the handler nets **zero**,
   call `MarkUncompilable` rather than returning a one-value bound (a
   returned refusal, not a panic, per ADR-005). That stops the wrong answer
   in all three variants at once — but it is containment, not a repair:
   three shapes a developer expects to compile stop compiling, each one an
   open defect owed a fix, and the interpreter re-run behind them is
   scaffolding absorbing that debt.

   **Zero, not `!= 1`** — this note originally said `!= 1` and that is
   wrong. A residual of two or more is not broken: its bottom is the
   unconsumed seeded error, and the paired CLOSURE path nets one from it
   with a runtime strip. `errorReturnsFn`'s own compile-time strip only
   removes that bottom when its identity probe matches, and after a
   `dup`/`drop` it does not — so `error [dup drop "k"]` measures 2 there
   while compiling and running correctly. Refusing on `!= 1` regressed
   exactly that shape; `TestErrorStripInputClosure` caught it, which is a
   good advertisement for the repo's habit of pinning the shapes that must
   keep working, not only the ones that must fail.
2. **Give `FallbackSpan` an out-count.** The island model being
   structurally single-output is the reason a correct arity could not be
   expressed even if known. Adding `NOut` and honouring it in
   `lowerFallback` + `runFallback` removes the whole class.

   The codebase already makes the symmetric assertion on the *input*
   side, which is the strongest argument for adding the output one.
   `runFallback` refuses an island threading more than one input
   (`eng/go/vm.go:1155-1158`) with the rationale: "Assert it so a future
   lowering bug degrades to a loud internal_error → whole-program
   fallback rather than silently mis-ordering the island's inputs." That
   is exactly the failure mode the output side has — and the output side
   has no assertion at all. `runFallback` ends `return append(stack,
   results...)`, and `screenResults` (`vm.go:217-222`) checks only
   `tapeCoupled`, never a count.

### APPLIED via (1) — and what the ratchets say about (2)

(1) is applied: `errorReturnsFn` now calls `MarkUncompilable` when the
handler it just ran nets **zero** values. All three underflow variants now
refuse loudly instead of leaking an `internal_error`: an open defect that
announces itself in place of a silent wrong answer, and still owed a
compiling implementation. Handlers that net one — or that net more because
the seeded error is still under the result — keep compiling to the same
answer. Regression tests in `lang/go/bytecode_error_arity_test.go` pin
every half, including the `>1` boundary the first attempt at this fix broke.

While implementing it, two ratchets turned up that were not in the
original analysis and that bear directly on whether (2) should follow:

```go
// test/go/langspec/compiled_coverage_test.go
const refusalCeiling = 0   // "Never raise it."
const islandCeiling  = 0
```

Every spec row compiles, and **no compiled program in the corpus embeds an
interpreter island at all**. Both must stay at zero before the fallback
and `OpFallback` machinery can be deleted (plan P7). At first read this
looks like an argument against (1) — it adds a refusal category, and
`refusalCeiling` says never raise it. Three things bound what (1) costs;
none of them makes the refusal acceptable:

- Neither ceiling moved. They count **spec rows**, and no row exercises a
  zero-netting `error` handler — which is precisely why the defect
  survived. `TestCompiledCoverage` passes unchanged.
- `tryRecordFallback` already declines `len(outs) != 1`
  (`carrier.go:1748`), so the single-output constraint the fix respects is
  the one that code already enforces.
- Its sibling guard states the preference outright: letting a shape island
  "would convert a clean refusal into a NEW interpreter island (a
  regression on islandCeiling)". **This code prefers a refusal to a new
  island** — a choice between two open defects, not an endorsement of
  either; `refusalCeiling = 0` and `islandCeiling = 0` are the standard
  both are measured against, and (1) owes `refusalCeiling` a return to zero.

The consequence for the test gap is that defect C's repro **cannot become
a spec row** — with (1) in place it refuses and would breach
`refusalCeiling`; without it, it miscompiles. Neither is a state to leave
it in, and that is why the regression lives in Go tests for now. It is
also the reason the gap existed: a shape that cannot be expressed in the
corpus is invisible to the corpus-driven gates.

So (2) — `FallbackSpan.NOut` — is no longer obviously "worth doing too".
It would let this shape compile as a zero-output island, which *raises*
`islandCeiling` off its floor and moves against P7. If the arity model is
to be fixed properly, the aligned repair is to compile `error` **natively**
(no island), not to make the island model more expressive. That is a
different, larger piece of work, and the note's earlier framing of (2) as
a straightforward companion to (1) should not be read as endorsing it.

### A note for the coverage allowlist

`eng/go/vm.go:1811` carries
`//covergate:allow bindGlobal's only error path is its own allow-listed
defensive underflow guard, unreachable without a bytecode-level fault`.
The premise is satisfied as written — a compiler bug *is* a bytecode-level
fault — but a two-line boru program reaches it. These guards are doing real
work in production rather than being dead defensive arms, which is worth
recording in `design/COVERAGE-ALLOWLIST.10.md` when (1) lands.


## D. `boru check` executes imported module bodies; a default run executes them twice

### Root cause — two deliberate decisions that compose badly

**1. `import` runs during check.** All eight `import` signatures are
registered `Go(handler, RunInCheck())`
(`lang/go/native/native_misc.go:131-175`). `RunInCheck()`
(`eng/go/sigimpl.go:86`) sets `GoImpl.RunInCheckMode`, consulted at
`sigimpl.go:214`, telling the check pass to run the real handler instead
of modelling its result. `import` needs this: the checker cannot type
`Mod.v` without the module's actual exports.

**2. Check mode is deliberately not propagated into the body.**
`lang/go/native/native_module_module.go:139-146`:

> CheckMode is deliberately NOT propagated to the module sub-registry.
> Module bodies need concrete string literals (used as export names / map
> keys) which carrier-stripping under CheckMode destroys.

`runModuleBodyCover` (`native_module_module.go:36`) builds a fresh
`newSubRegistry()` and copies an explicit list — `ModuleScope`, the type
sequence, writers, `Effects`, observe hooks, coverage, runtime stamping,
host fileops/formats/extensions, module config, parent context. `Check` is
not on it.

Together: `check` runs `import` for real, and the body it runs has check
mode off — so every effect in a module body executes with full ambient
authority during a type check.

### The multiplication

The pre-flight check runs by default before execution, and file modules
are **never** cached, so the two factors multiply. Counted with a `print`
in the body:

| Program | `-no-check` | `check` | default |
|---|---|---|---|
| one import of `m.boru` | 1 | 1 | **2** |
| two imports of `m.boru` | 2 | 2 | **4** |
| diamond (`a.boru` and `b.boru` each import `m.boru`) | 2 | 2 | **4** |

So the effect count is *importers × passes*, not a fixed 2 — a module
imported from N places in a program run with the default pre-flight check
executes its body 2N times.

`design/MODULE-CACHE.0.md` confirms the caching asymmetry and its status:
native `boru:` modules load "at most once per registry" via an `IsLoaded`
short-circuit, file modules are "never cached — `loadFileModule` re-reads,
re-parses, and re-runs the file body on every `import`", and the note is
**"analysis only — not implemented."**

CLI.md:63 documents `check` as "type-check without running".

### Fix decision

The stated rationale is sound but conflates two *separable* things check
mode does:

- **carrier stripping** — replacing concrete values with type carriers;
  this is what breaks module bodies, since export names and map keys must
  stay concrete;
- **handler substitution** — running a signature's `ReturnsFn` instead of
  its handler; gated independently at `eng/go/sigimpl.go:214`.

Only the first breaks module bodies. So the real repair is a third mode —
*substitute effectful handlers, keep values concrete* — expressible with
switches that already exist, letting a body still produce real export
names while `IO.write` is modelled rather than performed.

Cheaper measures, in increasing order of honesty:

1. **Cache the check-pass module instance for the run.** ~~Kills the
   doubling; check still performs effects once.~~ **Retracted on
   follow-up** — this is not the cheap option it looked like.
   `design/MODULE-CACHE.0.md` exists precisely because "a cache is *not* a
   transparent optimization — it changes observable semantics", and it
   gates the work on an unresolved singleton question. Sharing an instance
   across the check→run boundary is additionally riskier than a
   same-pass cache, since a module instance built while the parent was
   carrier-stripping could carry check-pass state into the real run. This
   option is blocked on MODULE-CACHE.0, not adjacent to it.
2. **Report it.** Emit an info diagnostic naming each module whose body
   was executed during check. No semantic change, and it makes CLI.md's
   claim true-with-a-caveat rather than false.
3. **Deny-all policy around the check-pass body.** Cheap, but denial
   *raises*, aborting the import and losing the exports check needs — so
   it only works if denial on this path becomes a silent stub, which is
   new policy behaviour.

Recommend **(2) now** — it is the only option with no semantic risk, and
it converts a false doc claim into a true one immediately. The split-mode
change is the actual repair. (1) is blocked on MODULE-CACHE.0.

### (2) is DONE

`runModuleBodyCover` emits `module_body_executed_in_check` (info) once per
body execution when the parent registry is in check mode, naming the
module — the import string for a file module, "an inline module body"
otherwise. No semantic change: `import` still runs, the body still runs
outside check mode, and nothing is cached.

Two decisions in it worth stating:

- **Not deduped.** The count *is* the finding. Three importers produce
  three entries, which is what makes "effects multiply by importers, not
  by a fixed 2" visible. A deduped entry would say "a module body ran" and
  conceal the multiplication this note exists to document.
- **Info, not warning.** Nothing about the program is wrong; the
  behaviour is the composition of two deliberate decisions, each correct
  on its own. The defect was that it was invisible.

`CLI.md` now states the exception under `boru check` rather than only
promising "type-check without running", including the 2N figure and the
`--no-check` escape. Tests in `lang/go/test/module_check_effects_test.go`
pin the count, the severity, and — the negative that keeps the advisory
from becoming noise — that a program importing nothing draws no entry.

### THE SPLIT MODE IS DONE — and it is at the capability layer, not dispatch

`eng.CheckState.ModelEffects` is the third mode. Values stay CONCRETE
(`Mode` is false, so dispatch, matching and every handler run exactly as
at runtime) while the effect BACKENDS are substituted. `runModuleBodyCover`
derives it from the parent, so it propagates transitively and no import
path can forget it.

Two things about the shape of the fix are worth more than the fix itself.

**Substituting a HANDLER does not compose; substituting a BACKEND does.**
The section above proposed running `ReturnsFn` instead of the handler for
effectful signatures. That cannot work: a `ReturnsFn` yields a CARRIER,
and outside check mode the rest of the body is not carrier-aware, so the
first downstream consumer raises "expects a concrete map, got a type
literal" — aborting the import and losing the exports, which is the exact
failure mode that disqualified the deny-all option. A concrete *stub*
return fares no better for reads (`parse (IO.read cfg)` on `""`).

So the substitution happens one layer down, where the seam already exists
and is already universal:

- **filesystem** — `EffectiveFileOps`/`hostOrModelled` returns a
  mem-over-host OVERLAY (`capabilities.NewOverlay`, the substrate the
  shipped `__sys.fs overlay` mode already uses). Every fs mutation in the
  tree routes through `EffectiveFileOps` — `write`, `folder`, `remove`,
  `open`, `lock`, `mmap`, `temp`, positioned binary writes; verified by
  grep, there is no `os.WriteFile`/`os.Create`/`os.MkdirAll` bypass in
  `lang/go/native` or `lang/go/modules` — so ONE substitution covers the
  whole class with no call-site changes.
- **output** — `io.Discard` in place of the writers the sub-registry
  copies from its parent.

**The line is WRITES, and that is what makes it safe.** Reads are
untouched, so no module body that loads today can start failing under
check: a body that reads a config still reads the real one, and a body
that writes a file and reads it back still sees its own bytes (the
overlay is a real filesystem — it just doesn't persist). This is the
property no refusal-based option could offer.

One detail the overlay needed: its path authority is `Upper.ResolvePath`,
and a fresh `MemFileOps` resolves relative paths against `""`, which
collapses `resolveBareModule`'s upward walk (`filepath.Dir(".") == "."`)
and would break a bare `import "foo"` inside a modelled body. The upper's
`Cwd` is seeded from the host's own resolution of `"."`.

### The `!Compiling` gate — the correction that mattered most

Gating on "the parent is in check mode" is WRONG, and not visibly so.
Measured: `import module [ print 'loading' … ]` under a default
`lang.Boru.Run` went from one line of output to **none**.

The cause is that §D's own model of the doubling was incomplete. On the
compiled path the module body runs ONLY in the compile pass — `import` is
`RunInCheck`, the exports are baked, and the VM program never re-imports.
That single execution is therefore the RUN's, not a preview of it, so
modelling it does not deduplicate the effect, it deletes it.

`CheckState.Compiling` separates the two exactly:

| pass | entry | body runs | effects |
|---|---|---|---|
| pure check | `Check.Begin` — `boru check`, the CLI's quiet pre-flight, `lang.Boru.Check` | yes | **modelled** |
| compile | `Check.BeginCompilePass` — `RunCompiled` / `CompileCheck` | yes | real |
| interpreter | no check pass | yes | real |

So under the CLI's default check-then-run the body still executes twice,
but exactly ONE of those executions has real effects — which is what a run
should have had all along. Measured on the built binary: one importer
prints `MODBODY` once (was twice), two importers twice (was four), `boru
check` prints nothing.

`TestDefaultRunEmitsModuleBodyEffectExactlyOnce` pins all three outcomes
(0 = the effect was deleted, 2 = the original doubling, 1 = correct) with
the diagnosis in the failure message; it and two others were verified to
fail when the `!Compiling` gate is removed.

### What is deliberately NOT modelled

- **A network send.** Left real, and named in the advisory text. Note the
  reason, because it CHANGED: this originally read "there is no backend to
  substitute", which was true when written and is no longer — `main` has
  since added `capabilities.HTTPOps`, a `Transport(profile, identity)` seam
  that `fetch` resolves its `http.Client` transport from, so a modelling
  transport is now installable. The decision stands on the OTHER reason,
  which the seam does not touch: no synthetic response is safe to invent. A
  fabricated status takes a wrong branch and a fabricated body fails a real
  parse, so modelling a fetch would break bodies that work today — the same
  test the write classes pass and this one does not. A refusing transport is
  no better: denial raises, and the raise aborts the import.
- **A `stdin` read.** A read, so out of scope by the rule above. Modelling
  it as EOF would make a body that reads input at load report a *check
  error* for a program that runs fine — trading a silent effect for a
  false verdict.
- **The effect ledger** (`eng/go/effects.go`) still counts modelled
  writes. Deliberate and conservative: over-counting only forgoes the
  silent interpreter re-run — scaffolding over a refusal, not a facility
  worth protecting — while under-counting a real network send would
  duplicate it. Discarded OUTPUT does stop counting, which is strictly
  correct — nothing escaped, so nothing should fence the re-run.
- **`boru check --emit`** (the disassembly surface) runs a compile pass, so
  its module-body effects are real. Consistent with the table above, and
  unchanged from before.
- **A nested body draws no advisory entry.** The advisory lands on
  `parent.Check`, and a module sub-registry owns its own `CheckState` by
  design (`design/legacy/module-fn-checkstate-ownership.1.ignore` §3.2), so a nested
  entry would be recorded where nothing reads it. The nested body IS
  modelled — verified by `TestModelledEffectsPropagateThroughNestedImports`
  — only its report is missing. Fixing it means routing diagnostics to a
  sink the top registry reads, which is a larger change than the count's
  honesty warrants.

Option (1), caching the check-pass module instance, stays blocked on
MODULE-CACHE.0 and is now also unnecessary for the effect problem: the
doubling of *effects* is gone. What remains doubled is the *work* — the
body is still parsed and executed twice under a default run — which is a
performance question, not a correctness one.

### Test gap — closed

The gap was that no test imported a module whose body had an observable
effect, so nothing distinguished "imported and typed" from "imported,
typed, and executed twice". `lang/go/test/module_check_effects_test.go`
now covers: the write not landing under check and landing under a run,
the write being readable back inside the same body, a real read still
resolving, output discarded under check and printed by a run, transitive
propagation through a nested import, no leak outside check, and the
exactly-once default run.


## E. REFERENCE.md documents error codes the engine cannot produce

The table is not prose. Readers dispatch on it:

```
do [risky] error [dot code case [not_found/q "…" bad_input/q "…"]]
```

so a documented code nothing mints is a `case` arm that can never fire,
and there is no way to find that out short of provoking the condition and
printing the result.

### Method

Extracted every row of REFERENCE.md's "Common codes" table, searched Go
source, tests and `lang/spec/*.tsv` for each, then probed each documented
condition through `do […] error [dot code]` to see what the engine
actually attaches.

### Result — 7 phantoms and a duplicate, out of 23 rows

| Documented | Reality |
|---|---|
| `type_mismatch` | phantom — a real mismatch raises `signature_error` |
| `out_of_range` | phantom — the name is `index_out_of_range`; and the runtime raised it **code-lessly** (below) |
| `unify_fail` | phantom, and wrong in kind: a failed `unify` does not raise at all, it returns the value `~unify-fail false` |
| `extend_user_type` | phantom — the gate exists and raises `extend_owner` (`eng/go/word_extend.go:250`) |
| `io_error` | phantom — I/O failures were code-less; the word's own codes are `read_error` / `write_error` |
| `cap_denied` | phantom — policy refusals are code-less too (below) |
| `cancelled` | phantom — nothing in the temporal module mints it; found by this pass, not by the report |
| `arity_mismatch` | listed **twice**, with two different descriptions |

Correct as documented: `undefined_word`, `user_error`, `def_error`,
`no_value_error`, `uncalled_function`, `reserved_word`,
`locked_signature`, `extend_conflict`, `constraint_violation`,
`arity_mismatch`, `unbound_param`, `gen_without_constructor`,
`incomparable`, `arith_error`, `not_found`. (An earlier draft of this
section also listed `signature_error` as "correct as documented" — it is
a real code, but it was never a row in the table.)

### The engine defect inside it

`[1,2] dotr 9` produces an error whose `code` is `None` — a **code-less**
error, which cannot be dispatched on, defeating
`do […] error [dup .code eq …]` for that condition entirely. The
check-time diagnostic for the same condition is `index_out_of_range`, a
third spelling.

It is one instance of a systemic gap: `lang/go/native` has 417 non-test
`fmt.Errorf` sites and `lang/go/modules` 119, none attaching an `[boru/…]`
code.

### FIXED — the coded sites, the corrected table, and a gate

**(1) The code-less conditions the table names now carry codes.** Each
takes the code that site's own CHECK-MODE mirror already emits, so the
static diagnostic and the runtime error finally agree:

| Site | was | now |
|---|---|---|
| `native_accessor.go` getr/dotr index OOB | code-less | `index_out_of_range` |
| `native_accessor.go` getr on a non-map | code-less | `getr_error` |
| `fileio.go` read failures (missing path, stdin, decode, sqlite) | code-less | `read_error` |
| `fileio.go` write failures (write, atomic, stdout/stderr) | code-less | `write_error` |
| a file op refused by a policy RULE | code-less | `permission_denied` |
| a file op with `fileops` uninstalled | code inside the message TEXT | `capability_not_installed` |

The last two rows are where this nearly went wrong. Coding the read path
alone made a *policy refusal* report `read_error` — dispatchable, and
misleading: `read_error` says fix the file, when the answer is to change
the policy. `fileOpError` therefore prefers a code the failure already
identifies itself by. That is the fileops half of the adapter
`lang/go/policy/error.go`'s header has claimed all along.

The uninstalled case was worse and only surfaced because a test for it
failed: `notInstalledFileOps` wrote its code **into the message** as an
`[boru/capability_not_installed]:` prefix, where `errors.As` cannot find
it. A code inside prose is a code no `case` arm can match, and it would
have been quietly restamped `read_error` by the very fix meant to help.
It is now a typed error (`notInstalledError`). The two refusals stay
distinct on purpose: a rule said no (widen the rule) is not the same
answer as the capability isn't there (install it).

Two things this deliberately does NOT do. It does not touch the other
gated capabilities (network, process, vault, tui), which still drop their
code — see below. And it does not give `Denied` an `Unwrap`, which would
fix all of them at once and also flip compiled-mode fallback semantics;
choosing the CODE is orthogonal to that and carries none of its risk.

A side effect worth stating: `runtimeShouldFallback` treats a foreign Go
error as "re-run on the interpreter" and a `BoruError` as "surface". So a
missing file in compiled mode used to silently re-run the whole program;
now it fails at once. That is the direction the file's own comment on the
exclusive-write path already argued for — a fallback re-run past an
effect fence surfaces as a spurious `internal_error`.

Pinned by `lang/go/error_codes_test.go`, whose negative half carries the
weight: a missing file must NOT report `permission_denied` (a helper that
stamped it on every I/O failure would pass the positive tests and be just
as wrong), and a permitted write+read must still succeed.

One test elsewhere had to move as a result, and it is a small argument
that the work was worth doing. `test/go/specgen`'s two wave-4 tests pin
the classifier arm for a runtime error carrying **no** code, and they
used an out-of-bounds `getr` as their specimen — precisely because it had
none. Coding it broke them. Their comment already recorded one earlier
migration (top-level `1 0 div`, which stopped qualifying when the
checker's arith mirror began flagging it statically), so the specimen has
now moved twice for the same reason: the population of uncoded runtime
errors keeps shrinking. It is `"x" convert Integer` today, and the
failure messages say so, so the next person to close that one gets a
sentence telling them what to do rather than a puzzle.

**(2) The table now names codes that exist.** `type_mismatch` →
`signature_error`; `out_of_range` → `index_out_of_range` (plus
`range_error`, which is a distinct real code); `extend_user_type` →
`extend_owner`; `io_error` → `read_error` / `write_error`; `unify_fail`,
`cancelled`, `cap_denied` and the duplicate `arity_mismatch` removed;
`type_error` added, since return-annotation violations are common and
were undocumented. `unify`'s non-raising behaviour is stated explicitly,
because a reader who came looking for `unify_fail` needs redirecting
rather than silence.

**(3) A gate, so it cannot drift again**
(`test/go/docexamples/errorcodes_test.go`). It extracts every code any
site ATTACHES to an error or diagnostic — 233 of them — and fails if the
table names one that is not in the set, or names one twice.

Two limits of the gate, stated because they bound what it proves:

- It is **one-directional**. Every documented code must be mintable; the
  ~210 mintable codes the table omits are not its business, because the
  table is "common codes", not a census.
- Mintable ≠ correctly described. `type_mismatch` would have failed here;
  a row naming `signature_error` while describing the wrong condition
  would not.

It deliberately does **not** count constant DECLARATIONS as minting.
`policy` declares four fully-qualified code constants and its header says
"the engine adapter copies these onto the produced BoruError" — counting
declarations would have let `cap_denied`'s successor pass the gate while
still never reaching a user. A code is real when a site attaches it.

### CLOSED: every gated capability's refusal now carries a code

Before this pass, **every** policy refusal reached the user without a
code — `fileops`, `network`, `process`, `vault`, `tui` alike:

```
boru -deny fileops.read    -e '… do [IO.read …]    error [dot code]'  → None
boru -deny network.connect -e '… do [Net.fetch …]  error [dot code]'  → None
```

`policy.Denied` carries `Code` (`permission_denied`,
`capability_not_installed`, `modules_disabled`, `policy_attenuation`) and
`lang/go/policy/error.go:3-6` states an engine adapter copies it onto the
produced `BoruError`. No such adapter existed.

`fileops` now has one (`fileOpError`), and it is the capability the
REFERENCE table is mostly about. The first line above returns
`permission_denied`, an uninstalled `fileops` returns
`capability_not_installed`; the second line still returns `None`.

The `Unwrap` route was NOT taken. Giving `Denied` an `Unwrap() error`
returning a `BoruError` would fix all of them at a stroke — and would also
flip `runtimeShouldFallback` (`lang/go/boru.go`) from "foreign error, re-run
on the interpreter" to "boru error, surface", for every denial, in compiled
mode, including denials from sites nobody has audited. That is probably the
right answer eventually — `eng.PolicyDenied` already gets exactly that
treatment at the VM dispatch gate, with a comment explaining that a re-run
would evaluate the program twice — but it is a semantic call about the
fallback fence and belongs to whoever owns it.

The note's own alternative was "repeat `fileOpError`'s trick at each
remaining wrap site … but it has to be redone for every future gated
capability." What landed is that alternative with its one objection
removed: **one** shared adapter (`PolicyRefusal`,
`lang/go/native/policy_error.go`), called from inside each capability's
GATE function rather than at each handler. Nine call sites reach five
gates today and the count only grows; wrapping at the gate is what makes a
new caller unable to forget, which is exactly how four capabilities stayed
code-less after the fileops half was fixed. `fileOpError` now shares the
adapter's classifier and keeps only its own detail (a file op has already
built a message naming the path, which beats the raw blame trail).

Measured, all ten combinations — five scopes × refused-by-rule and
uninstalled:

| scope | by rule | uninstalled |
|---|---|---|
| fileops | `permission_denied` | `capability_not_installed` |
| network | `permission_denied` | `capability_not_installed` |
| process | `permission_denied` | `capability_not_installed` |
| vault | `permission_denied` | `capability_not_installed` |
| terminal | `permission_denied` | `capability_not_installed` |

(The tui scope is named `terminal` in policy. A test written against the
module's name fails at policy-load time with `unknown scope "tui"`, which
is how that was discovered.)

`TestEveryGatedCapabilityRefusalCarriesACode` pins all ten, and the
negative half carries the weight as it did for fileops:
`TestGatedCapabilityFailuresAreNotReportedAsRefusals` proves an unreachable
host and a vault with no backend do NOT report a refusal when no policy is
installed. An adapter that stamped `permission_denied` on every failure
from a gated word would satisfy the positive table and be strictly worse
than the missing code — it would send authors to edit a policy that is not
the cause.

**A regression the gate caught, worth recording because it proves the gate
earns its keep.** The first draft hoisted the two code literals into a
classifier that merely *returned* them, leaving `r.BoruError(code, …)` with
a variable. `test/go/docexamples/errorcodes_test.go` immediately reported
`permission_denied` as unmintable — correct, from its point of view: it
extracts codes from construction sites and deliberately ignores anything
that only *names* a code, because policy's four constants sat
declared-and-never-attached for as long as they existed. The refactor had
made a live code invisible to the one check that keeps REFERENCE.md
honest. The switch now sits directly on top of the `r.BoruError` calls, and
`policy_error.go` says why in a comment so the next reader does not
re-hoist it.

REFERENCE.md's two claims that only fileops carried a code are corrected,
and the capability section now shows the dispatch idiom.

### DONE: the enumeration itself

The original plan was to generate the table FROM a registry-side
enumeration of codes — the doctrine that makes `boru describe` unable to
drift, and the data source `boru explain <code>` would need (R4 in
`legacy/rust-zig-roc-faber-in-boru-report.0.ignore`). It exists now:
`eng/go/errorcodes.go` owns the mechanism and the kernel's 45 codes;
`lang/go/native/errorcodes.go` registers the language layer's 188.
`eng.ErrorCodes()` / `eng.LookupErrorCode(code)` are the accessors.

Four decisions in it, each of which was the smaller of two options:

- **Names and owners, no descriptions.** Restating 233 descriptions in Go
  — authored in one pass, in a file that reads as authoritative — would
  create a large body of unreviewed prose competing with the reviewed copy
  in REFERENCE.md. A wrong description is worse than a pointer to the
  right one. Completeness of the NAME list is what the enumeration is for;
  the gate ties it to the documentation rather than duplicating it.
- **Layered registration, like type registration.** eng cannot enumerate
  lang's codes without inverting the dependency, so each layer registers
  its own under an owner id (`OwnerKernel` / the new `OwnerLang`), exactly
  as `RegisterType` works. A consequence worth stating: the enumeration is
  per-BUILD. A binary linking only eng reports only the kernel's codes,
  which is the honest answer to "what can this program raise?" rather than
  a superset copied from a larger build.
- **A code minted in both eng and lang is owned by eng** (twelve are:
  `type_error`, `undefined_word`, `index_out_of_range`, …). A lang site
  raising `type_error` is raising the kernel's error, not defining its
  own. Registering it twice is refused as double-owned, deliberately.
- **Registration errors are accumulated, not returned** — the call site is
  a package initialiser. `ErrorCodeInitError()` is surfaced by
  `NewRegistry`, following `BuiltinInitError`'s precedent and ADR-005's
  no-init-time-panic rule.

The naming policy the note said was needed first is now a one-line
enforced rule (`^[a-z][a-z0-9_]*$`, checked at registration; all 233
existing codes already conform), and the stability guarantee is stated
where it can be read: renaming or removing a code is a breaking change to
every handler that names it, and a SILENT one — the old `case` arm simply
stops firing. The enumeration cannot prevent a rename; making it
impossible to do by accident is the guarantee it can actually offer.

**The gate is now bidirectional, which is the part that makes any of this
hold.** `test/go/docexamples/errorcodes_test.go` cross-checks three
artefacts: minted (extracted from construction sites), registered
(`eng.ErrorCodes()`), documented (REFERENCE.md).

| check | catches |
|---|---|
| documented ⊆ minted | the original seven phantoms |
| minted ⊆ registered | a new code reaching users with no deliberate entry |
| registered ⊆ minted | an entry for a code no site attaches — a phantom one level down |
| documented ⊆ registered | a documented code `boru explain` could not resolve |

Both new directions were verified to fail when violated. `registered ⊆
documented` is deliberately NOT checked: the table is "common codes", not
a census.

**One correction to the extraction, found while making it
bidirectional.** The `Code:` regex also matched `Code: "boru/init"` and
`Code: "boru/check"` in the LSP server — fields of an LSP `Diagnostic`, not
of a `BoruError`. They are protocol strings no boru program can dispatch
on. A one-directional gate tolerated them, because extra entries only made
it more permissive; a bidirectional one would have forced two LSP strings
into the language's enumeration to stay green. `cmd/go` is therefore no
longer scanned, and the exclusion is stated at the source roots so it
cannot be quietly widened back.


## F. The shorthand `fn` form silently discards a `def`-bound union/enum type

This started as "`case` exhaustiveness is lost in the shorthand form".
The case symptom is real but minor; the actual defect is that **the
shorthand `fn` form loses the declared type of any parameter whose type
name resolves to a payload-carrying value**, and `case` is only the
loudest of five consequences. Two of the others are silent wrong
behaviour.

### Root cause

`NoEvalArgs` does not gate *map* auto-evaluation — only `NoEvalMapArgs`
does. The engine says so itself, at the exact site
(`eng/go/engine.go:3380-3386`):

> `NoEvalMapArgs` (separate from the list-only `NoEvalArgs`) suppresses
> map auto-evaluation at this slot. Used by `def`'s typed-name sig so a
> **Word at the type position arrives raw** — important when the type is a
> fn that's also a registered callable.

The scenario that comment describes *is* this defect, and the gate that
prevents it is applied to exactly one of the three words that need it:

| Signature | `NoEvalArgs` | `NoEvalMapArgs` |
|---|---|---|
| `def` typed-name (`native_definition.go:29`) | — | **`{0: true}`** |
| `fn` 3-arg triple (`:155-161`) | `{0,1,2}` | **absent** |
| `afn` (`:187-194`) | `{0,1}` | **absent** |

Both forms parse identically — the annotation is the map `{x: word(IS)}`
either way — so the divergence is entirely post-parse:

- **Bracket form.** Slot 0 is a *List*, which `NoEvalArgs{0}` does
  suppress, so the nested map is never visited. `IS` reaches
  `ResolveSigType` as a raw **Word** and takes the authoritative name path
  (`eng/go/fn_params.go:487-495`, `r.LookupTypeName("IS")`), yielding the
  minted lattice node with its `*disjunctUnifier` behaviour.
- **Shorthand form.** Slot 0 *is* the map, so `autoEvalMap` dispatches the
  word `IS`, pushing the type's **body** — a `Disjunct` *value* carrying
  `DisjunctInfo`, not a lattice node. `ResolveSigType` has no branch that
  mints a `*Type` from a Disjunct value, so it falls to the inline-disjunct
  branch (`eng/go/fn_params.go:536-542`) and returns `(TAny, &pattern,
  nil)`.

The declared type is gone before any analysis runs. Downstream,
`paramBodyCarrier` (`eng/go/core_helpers.go:701-720`) reads only `p.Type`
and calls `ParamInputCarrier(TAny)` → `NewDynamicCarrier(TAny)`
(`carrier.go:307-310`), instead of the distributing declared-union carrier
`UnionCarrierForType` builds (`carrier.go:316-323`).
`checkCaseExhaustiveness` then sees `v.Dynamic` and takes its dynamic
branch (`case_exhaustive.go:727`) before ever computing alternatives.

**Nothing in the case checker is wrong.** It is fed a `dynamic(Any)`.

Why `Boolean` and `refine` newtypes are unaffected: their `def`'d bodies
evaluate to *bare type nodes* (`Data == nil`), which `IsBareTypeNode`
catches at `fn_params.go:443` and returns as the node itself. Only bodies
that evaluate to a payload-carrying value lose the name.

### Causation proved by patch

Adding `NoEvalMapArgs: {0: true}` to `fn`'s triple signature makes the
shorthand parameter resolve to the named union and the finding disappear.
(Patch applied, observed, reverted; tree clean.)

### Consequences, all verified on the pristine binary

| Consequence | shorthand | bracket |
|---|---|---|
| `case` over a union | `case_not_exhaustive` | proves exhaustive |
| `case` over an **enum** (`def E enum [a b c]`) | `case_not_exhaustive` | proves exhaustive |
| `=>` / `afn` with a union param | `case_not_exhaustive` | n/a (same gap) |
| `case_redundant_default` advisory | lost (0 emitted) | emitted |
| **arity**: `def IN (Integer tor None)`, `f` called with **no** argument | **runs, returns `0`** | `no_signature` |
| **return type**: `def f fn x:Integer IS [x]` then `1 f` | bogus `type_error: f: expected 1 return value(s), got 2` | `1` |

The arity row is the serious one: a declared one-parameter function
becomes callable with zero arguments, silently, because the evaluated
disjunct hits `ParseFnParams`' `None`-stripping branch
(`fn_params.go:158-176`) that is only meant to fire for inline disjuncts,
synthesising a 0-arg overload.

Dispatch remains *sound* in the shorthand form — the `Pattern` still
enforces the union at call time, and `f true` is rejected in both forms.
Only the analysis and the diagnostics degrade.

### The documentation is wrong, and the test that would catch it opts out

`case`'s own hand-authored examples ship two false claims
(`lang/go/native/help/help_control.go:113-114`):

```
def IS (Integer tor String) def f fn x:IS String [case x [Integer "i" String "s"]]
  ; # exhaustive over IS — no default needed          ← actually errors
def f fn x:IS Integer [case x [Integer 1]]
  ; # check ERROR case_not_exhaustive — uncovered: String
                                                     ← actually reports "the scrutinee is dynamic"
```

`TestHelpExamplesCorrect` skips hand-authored examples **by
construction** (`lang/go/test/help_examples_test.go:145-152`, `if e :=
help.Lookup(word); e != nil && len(e.Examples) > 0 { continue }`), with a
reasoned comment: they are curated prose, not the `;# <exact-stack>` shape
the matcher validates.

This is worth stating plainly because AGENTS.md rests real weight on the
opposite property — that `describe` output is "generated from the *live
engine* … so they cannot drift from the code the way prose can"
(AGENTS.md:30). For auto-generated signature and lattice output that
holds. For hand-authored `Entry.Examples` it does not: they are prose that
merely *lives* in Go, and nothing executes them. `describe case`
currently tells the user something false.

### Fixes

1. **(verified)** Add `NoEvalMapArgs: {0: true}` to `fn`'s triple
   signature (`native_definition.go:155-161`) and `{1: true}` to `afn`'s
   (`:187-194`). This is the same suppression `def`'s typed-name signature
   already carries at `native_definition.go:29`, and
   `registerDefKeywordForms` already propagates it into the synthesized
   `def … fn …` forms via `shiftPosFlags` (`:517`). Fixes the union, enum,
   `=>`/afn, arity and lost-advisory rows at once.
2. **(verified)** Add `|| IsDisjunct(v)` to `IsSigTypeValue`
   (`eng/go/fn_def.go:185-187`) so a Disjunct in the output slot is not
   read as a concrete return-by-value. Still needed after (1), because
   `NoEvalMapArgs` does not gate a bare Word in the output slot.
3. **(DONE — took the local option)** The inline union
   `x:(Integer tor String)` is a *different route into the same defect*,
   not a second bug:
   `ParseFnParams` deliberately evaluates a paren annotation itself
   (`fn_params.go:135-152`) and hands the Disjunct to the same branch.
   Neither fix above touches it. Either mint anonymous disjunct lattice
   nodes (today `installDisjunctUnifier` is reachable only from
   `InstallType`, `core_type.go:496` — so this introduces unnamed disjunct
   types, with knock-on effects on `Type.Equal`, dispatch sorting and error
   rendering), or — smaller and strictly local — teach `paramBodyCarrier`
   to build the distributing declared carrier from a Disjunct `Pattern`
   when `p.Type` is `TAny`. The latter also fixes the shorthand row
   independently of (1).

   **The local option is applied.** `paramBodyCarrier` now builds the same
   distributing `Declared` carrier for an inline Disjunct pattern that
   `ParamInputCarrier` builds for a named union, so the two spellings of
   the same domain analyse identically. Worth noting the route failed in
   the **bracket** form too, which is what marks it a separate defect
   rather than another face of the shorthand bug — and means (1) could
   never have covered it. No lattice nodes are minted, so none of the
   `Type.Equal` / dispatch-sorting / error-rendering knock-ons apply.
4. **(DONE) Diagnostic wording.** "the scrutinee is dynamic — no static
   type can prove the clauses cover it" is reachable for
   genuinely-annotated parameters independent of this bug —
   `paramBodyCarrier` marks `{:T}`/`[:T]` typed-container params dynamic
   (`core_helpers.go:703-716`), which I re-confirmed: `def f fn
   m:{:Integer} String [case m […]]` drew it. It should not deny the
   annotation. Now reads:

   > case: this scrutinee is gradual here — no static type is available to
   > prove the clauses cover it; add a trailing default (or an Any clause)

   Describing the POSITION rather than asserting the parameter is untyped.
   Six existing test fragments moved from matching `"dynamic"` to
   `"gradual"`, and a new test forbids the old phrasing outright for the
   three shapes that reach it.

   The documentation half of (4) needed no edit — see below.

Measured fix blast radius: with (1) + (2) applied, the full
`eng/go/... lang/go/...` suite passes with exactly one golden update —
4 lines of `lang/go/native/testdata/fnmodel_equivalence.golden` (the
sig-table rows for `fn`, `afn` and the two synthesized `def` keyword
forms). Zero behaviour-corpus drift.

### LANDED — and a sixth consequence the negative tests found

(1) and (2) are applied, with the predicted 4-line golden update and
nothing else. Five of the six rows in the consequence table above now
match the bracket form exactly, including the arity row (`f` with no
argument is `no_signature` again) and the `case_redundant_default`
advisory. The `case_not_exhaustive` message on a genuinely-incomplete
shorthand `case` is now the precise `uncovered: String` rather than the
"scrutinee is dynamic" misdiagnosis.

Writing the *negative* half of the regression tests — per this repo's
"always pair positive with negative" discipline — turned up a
consequence the original investigation missed, because every row in that
table asserted the shorthand did something WRONG and none asserted what it
must REJECT:

| | shorthand | bracket |
|---|---|---|
| a body returning `Boolean` under a declared union return `IS` | **accepted silently** | `type_error: return value 1: expected IS, got Boolean` |

The declared union return is not merely mis-analysed, it is **unenforced**.
Cause is the mirror image of (1) and is *not* closed by it: `NoEvalArgs`
and `NoEvalMapArgs` gate container auto-evaluation, but a bare Word in the
OUTPUT slot is resolved by forward collection itself, which no flag gates.
So `ResolveSigType` receives the Disjunct VALUE, takes its inline-disjunct
branch (`fn_params.go:536-542`) and answers `(TAny, &pattern)` — and
`ParseFnReturns` (`fn_params.go:429-436`) reads only the `*Type`, dropping
the pattern on the floor. `TAny` accepts everything.

Fix (2) is still doing its job here: it stopped the Disjunct being read as
a return-BY-VALUE, which is what produced the bogus `expected 1 return
value(s), got 2`. It cannot supply enforcement, because there is nowhere
on `FnSig` to put a return pattern — `Returns` is `[]*Type`
(`eng/go/value.go:249`).

**`QuoteArgs: {1: true}` is not the fix — measured, not assumed.** It is
the obvious move (quote the output Word so the NAME survives, exactly as
`def`'s typed-name sig quotes its name slot) and it breaks the language:
`QuoteArgs` participates in forward collection, so every `def f fn …`
call starts failing with *"def is still waiting for 1 argument(s) when
`fn` begins its own dispatch"*. It takes down the synthesized `def`
keyword forms, the plain-builtin output (`fn x:Integer Integer […]`), the
list output and the paren output — all four measured. Reverted.

#### FIXED — `FnSig.ReturnPatterns`

The missing channel is now there: `ReturnPatterns []*Value` on `FnSig`,
positional against `Returns`, the symmetric twin of `FnParam.Pattern`.
`ParseFnReturns` stops discarding what `ResolveSigType` already computed
and returns the patterns alongside the types; they thread through
`FrameTailSpec` onto `ReturnCheckInfo` and are enforced in both the
runtime check (`validateReturnTypes`) and the check-mode mirror
(`checkBodyReturnConformance`), with `Unify(pattern, got)` as the
predicate.

Two details worth keeping:

- **The pattern check must run BEFORE the `exp == TAny` skip.** Both
  enforcement sites short-circuit on a declared `Any`, and a declared
  union degrades its `*Type` to `Any` *precisely when* the pattern is the
  only contract there is. Ordering it after the skip would have compiled,
  passed the positive tests, and enforced nothing.
- **`Type` is an alias of `Value`**, so the pattern pointer doubles as
  the `*Type` the error builder wants. The shorthand's message renders as
  `expected Integer tor String, got Boolean` rather than the useless
  `expected Any`.

Measured:

| form | before | now |
|---|---|---|
| shorthand, `Boolean` body | accepted silently | `expected Integer tor String, got Boolean` |
| bracket, `Boolean` body | `expected IS, got Boolean` | unchanged |
| shorthand, `Integer` or `String` body | accepted | still accepted |

`TestShorthandFnUnionReturnType` was written to **fail loudly when this
was fixed**, and it did — the way out was the deliberate test edit it
asked for. It now asserts rejection for both forms plus the negative half
(a body returning either alternative is still accepted), because a
pattern check that rejected everything would have satisfied the
rejection assertions and been useless.

#### CORRECTION: that fix shipped enforcing nothing on the default path

The table above was measured in **check mode**, and check mode was the
only place the fix worked. Coverage flagged the real state of it: the
enforcement branch in `validateReturnTypes` was never executed by the
suite, because `TestShorthandFnUnionReturnType` calls `checkDiag` and
nothing else. Re-measured across all three engines:

| form, `Boolean` body | `boru check` | `RunInterp` | **compiled `Run`** |
|---|---|---|---|
| shorthand union return | rejects | rejects | **accepted, returned `true`** |
| inline union return | rejects | rejects | **accepted, returned `true`** |
| bracket (named union) | rejects | rejects | rejects |

This is defect A's shape exactly: a contract wired into every path except
the one that ships by default. `ReturnPatterns` reached `FnSig`,
`FrameTailSpec` and `ReturnCheckInfo` — all interpreter-side — but never
reached `CompiledFn`, so the VM's RET contract (`checkReturnContract`)
still saw only `Returns`, and a declared union's `Returns` entry is `Any`.
`Is(Any)` passes everything.

The bracket form's rejection is what made it look fine: a NAMED union
resolves through `r.LookupTypeName` to a real lattice node, so its
`*Type` is `IS` rather than `Any` and the plain type check catches the
Boolean without any pattern. Only the two forms that degrade to `Any` —
shorthand and inline — depend on the pattern, and both were open.

Closed by carrying the patterns the rest of the way:
`CompiledFn.ReturnPatterns` (the RET-side twin of the existing
`ParamPatterns`), populated via a new `SetUnitReturnPatterns` at the same
two sites that already set `SetUnitParamTypes` / `SetUnitDecl`, and
enforced in `checkReturnContract` with the same `Unify(*pat, got)` the
interpreter runs. All three engines now produce the byte-identical
`f: return value 1: expected Integer tor String, got Boolean`.

Threading the patterns to the VM also closed a second, wider gap that had
nothing to do with unions: a **list** output signature carrying a
structural element (`fn [[…] [[:Integer]] […]]`, `[{:String}]`) reaches
`ParseFnReturns`' per-element loop, and that loop's pattern slot was
equally unenforced — `[[:Integer]]` accepted a `["a"]` body on every
engine, while the identical declaration in a PARAM slot raised
`signature_error`. Params and returns are the same contract read in two
directions, and they now refuse the same values.

`TestShorthandFnUnionReturnType` asserts all three engines, and
`TestListOutputSigStructuralReturn` covers the list-output route with
both halves. The general lesson is the one defect A already paid for:
**a green check-mode test is not evidence about the VM.** Where a
contract has more than one enforcement site, the test has to name them.

### `describe case`'s two false examples fixed themselves

The documentation half of fix (4) needs **no edit**. Both hand-authored
examples this note flagged as shipping false claims
(`help_control.go:113-114`) are now true, verified against the patched
binary:

| Example's claim | Now |
|---|---|
| `def IS (Integer tor String) def f fn x:IS String [case x […]]` — "exhaustive over IS — no default needed" | 0 errors ✓ |
| `def f fn x:IS Integer [case x [Integer 1]]` — "check ERROR case_not_exhaustive — uncovered: String" | exactly that message ✓ |

All four other shorthand `case` examples in the same block (Boolean,
intervals, newtype, dynamic-with-default) were re-run and behave as their
comments claim.

This is the better outcome by some distance — the alternative was editing
the docs to describe the bug. It does not close the *structural* problem
the note raised: `TestHelpExamplesCorrect` still skips hand-authored
examples by construction (`help_examples_test.go:145-152`), so these two
were false for as long as they were precisely because nothing executes
them. Two of them are now pinned directly by
`TestDescribeCaseShorthandExamplesAreTrue`, which is a spot fix, not the
general one.

### Test gap

The suite pins case exhaustiveness **exclusively through the bracket
form**. Every union/enum row in `lang/go/test/case_exhaustive_check_test.go`
is written `fn [[x:IS][…][…]]` — lines 65, 69, 73, 77, 81, 219, 268, 272,
275, 298, 306, 310, 339, 349, 428. There is not one shorthand row and not
one `=>`/afn row in that file. The `Boolean`, `refine` and interval rows
that *are* form-insensitive pass either way, so a form-agnostic reader
would not notice the omission.

The tests that would have caught it, in order of leverage:

1. A **form-equivalence** assertion: the shorthand and bracket spellings of
   the same signature must produce the *same* diagnostic set. This
   generalises past `case` and would also have caught the arity and
   return-type divergences.
2. An `FnParam`-level golden asserting `ParseFnParams` yields
   `Type=IS, Pattern=nil` for both forms.
3. Running hand-authored help `Examples` through `check` — a weak
   "must not error" gate is enough and does not need the exact-stack
   matcher.

## G. Un-separated forward calls evaluate right-to-left → stale reads

### Not a new defect

`design/legacy/ERRORS.8.ignore:188-206` records this as VOXGIG **B2a**:

> `(1 add 1) print (2 add 2) print` prints `4` then `2` — un-separated
> chained forward calls evaluate right-to-left.

The fix is §6.1 of that note, explicitly **not landed** ("deferred to the
structure-first lazy-resolution rework"), with statement separation
(`;` / `end`) as the named mitigation.

### The unrecorded manifestation

The note frames B2a as an output-ordering nuisance. Because a newline is
*not* a separator, the same rule silently yields a wrong **value**:

```
def m (make FlexMap {n:0})
m set n 1
m.n                 → 0      (the pre-write value)
```

`m set n 1 ;` then `m.n` → `1`; `(m set n 1)` then `m.n` → `1`. Any
mutate-then-read pair written without an explicit barrier reads stale.

That is a data-correctness face of B2a and belongs in that note's §6 and
NUR029's neighbourhood. `mixed_form_call` already exists as the advisory
hook, and mutate-then-read is a recognisable shape for one.

### Methodological note, recorded deliberately

The first version of the Verse report used un-separated mutate-then-read
programs to *test* whether `await` branches were isolated, read the stale
values as evidence of isolation, and on that basis rejected a correct
claim that mutations escape a branch (defect **A**). The barrier is
load-bearing in test programs, not only in production ones — which is
itself an argument for landing §6.1, since a sequencing rule that can
invalidate a test silently is more expensive than an ordering surprise.


## Cross-cutting observations

**Three of the seven are compiled-vs-interpreted divergences** (B, C, and
the compiled half of G's neighbourhood), all in the same architectural
seam: body invocation. `design/legacy/MISCOMPILE-HUNT-FINDINGS.0.ignore` records 23
prior `--compile != interpret` divergences and the same seam keeps
producing them, which argues for a differential gate specifically over
*body-invoking words × observable side effects* rather than over the
corpus at large.

**The two engines have different *numbers* of body-entry points, and that
is the structural root of the seam's productivity.** The interpreter has
one (`Engine.Run`), so any invariant installed there is automatically
universal. The VM has four, so an invariant has to be installed four
times and nobody can tell by reading one of them whether the other three
agree. Every fix in this seam that is written as "wrap the call" inherits
that asymmetry, which is why the reverted patch was simultaneously correct
in shape and 25% complete. The durable version of this observation is not
a bug list: it is that the VM needs *one* named body-entry function that
every path routes through, in the way `matchSignature` is the one place
argument positions are decided.

**Two are "the guarantee is real but narrower than the prose"** (A, and
B's doc half). Both times the Go-level comment is accurate and the
user-facing doc generalises it. `eng/go/fork.go`'s header says "execution
scope"; EXPLANATION.md says "side effects".

**Three would have been caught by tests that already exist** — B's
semantics are asserted in `context_test.go` but only against the
interpreter; A's shape is one spec row away from the existing `-race`
gate; and F's correct behaviour is asserted fifteen times in
`case_exhaustive_check_test.go`, every one of them in the bracket form.
None needs new machinery, only coverage of the second spelling or the
second execution path.

**The recurring shape is "one axis is exercised, its sibling is not."**
Interpreter but not compiler (B, C). One call spelling but not the other
(F). One container kind but not the mutable one (A). Where the suite has a
choice of surface, it has consistently pinned one and left the other to
inference — and every defect here lives in the unpinned half. A gate that
asserted *equivalence between sibling surfaces* — same program, both
engines; same signature, both forms — would have caught four of the seven
without anyone predicting the specific bug.

**Two documentation claims are load-bearing and false.** EXPLANATION.md
and HOWTO.md assert branch isolation for exactly the values that race (A),
and `describe case` ships an example asserting exhaustiveness for a
program that errors (F). The second matters beyond its own row: AGENTS.md
rests real weight on `describe` being generated from the live engine and
therefore unable to drift. That holds for signatures and lattice output;
it does not hold for hand-authored `Entry.Examples`, which are prose that
merely lives in Go and which the example test skips by construction.
