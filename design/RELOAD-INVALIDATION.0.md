# RELOAD INVALIDATION

How **hot code reloading** and **transparent compilation** coexist without
making compiled code slower — the mechanism design that closes the gap
between `HOT-CODE-LOADING.0.md` (reload as a protocol) and the shipped
compiled-by-default execution story (`P7-ENDGAME.10.md`,
`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md`). Grounded in a survey of how
production VMs solve the same problem (§4) and an audit of boru's current
freshness machinery (§2–3), including one **new, empirically confirmed
divergence bug** (§3 F1). Suffix `.0`: design only, nothing here is
implemented.

## 1. The requirement

Three constraints, jointly:

1. **Reload works** — a module re-imported at runtime takes effect at the
   next dispatch, per the `HOT-CODE-LOADING.0.md` protocol.
2. **Compilation stays transparent** — no user-visible modes, flags, or
   "don't compile your server" rules. The user never knows or cares which
   tier runs their code. (This is already the shipped posture:
   `CompileTry` has been the default since the P7 flip of 2026-07-08, and
   Stage J plans to delete the unbounded interpreter route entirely.)
3. **Compiled code gets no slower** — the freshness machinery must cost
   ~nothing on the steady-state hot path. Reload support may cost
   something *at the reload event*; it may not tax every call.

Constraint 3 is violated **today**, before reload even exists: every
invoke of a runtime-stamped callback pays a pull-based freshness
validation (§2.2). And constraint 1 is unsound today for one unit kind
(§3 F2). So this design is not "add reload support to a finished
compiler" — it is a freshness-architecture revision that the compiled
tier needs anyway.

## 2. Where boru stands (audit)

### 2.1 The three compiled-unit kinds and their freshness

(Per `REFUSAL-CLOSURE.0.md` §7, `RUNTIME-STAMPING.0.md`, and the emitter.)

| | whole-program `CALL_USER` unit | compile-time stored ref | runtime-stamped detached ref |
|---|---|---|---|
| reachable via | `CALL_USER` in the program stream — **no per-call seam** | `InvokeCallback` seam | `InvokeCallback` seam |
| freshness | per-site RE-RECORDING, **compile-time only**: each unit records the `DefTable.Gen` of every enclosing binding it baked, and a later call site whose generation has moved compiles a fresh unit (Stage 4b, `compiler/go/unit_memo.go`); a unit whose reference escapes into a value keeps the whole-program refusal (this cell read "`frozenReads` + `NotifyNameRebound` → whole-program refusal" until 2026-09-04, and was FALSE for a baked CALL TARGET until the same day — see the corrections under §3 F1) | `depNames` + `poisoned`, **compile-time only** (`DepSnap == nil` ⇒ vacuously fresh at runtime) | `DepSnap {Depth, Gen}` validated **per invoke** (`DepsFresh`) |
| on staleness | program interprets wholesale | that handler interprets forever | JIT re-stamp (`RestampBox`, ≤ 3 lifetime tries), then interpreter |

### 2.2 The hot-path cost today (constraint-3 audit)

From the invoke-seam audit (`eng/go/compiled_runtime_vm.go:18-24`,
`compiler/go/bytecode.go:704-717`):

- `DepsFresh` runs on **every** invoke of a detached ref — a non-nil
  `DepSnap` is guaranteed, even when empty. Cost: map-range setup plus,
  per dep, **two Go map lookups** (`Defs.Gen(name)`, `Defs.Depth(name)`).
  Real handler dep sets run ~1–5 names (the emitter's own examples:
  todo-api's `live-todos`, mini-redis's `arg-at`/`kv-read`), so the seam
  pays ~2–10 map lookups per call before the VM runs a single opcode.
- A ref that went permanently stale (restamp budget exhausted) is worse:
  every invoke pays the failed walk **plus** the `RestampBox` mutex and
  twin check before the interpreter runs the call — the de-optimized path
  is the most expensive path.
- No Go benchmark measures the seam (`InvokeCallback`/`DepsFresh` appear
  in no `*bench*` file); the only recorded VM-vs-interpreter callback
  number is prose ("~19x", `net_socket.go:596-598`). The work in §6 adds
  the benchmark first.

This is **pull-based validation**: every call asks "is my world still
current?" — the cost lands on the hot path, proportional to dep count.
The survey (§4) shows every production VM that solved this problem moved
the cost to the **rebind event** instead.

### 2.3 What transparency already means here

`boru run`/`do`/`test`/REPL/exec are compiled-by-default (`CompileTry`)
with check-by-default preflight. A whole-program refusal is a failure —
an unimplemented or unproven case in the compiler, owed a fix — and the
runtime today absorbs it instead of reporting it: the program is re-run
on the interpreter, with a warning at the `run` surface and **silently**
at `do`/REPL/built binaries. The silence makes it worse, not better — a
failure that hides itself is one nobody is tracking. Runtime stamping is
armed on every compiled-mode request and kept armed across that
interpreter re-run, so callbacks still earn the VM under a refused top
level (`NET-COMPILE-FRONTIER.0.md` Addendum 6: mini-s3's driver refuses
whole-program — an open defect — while all 23 callback units run
stamped); that bounds how much of the program the defect costs, it does
not settle the defect. Stage J's end state makes refusal a compile error
with enumerated interpreter carve-outs — per-invoke declines among them,
each of them a case still owed a compiler rather than a sanctioned
outcome. **This design must live inside that trajectory**: reload rides
the seam, not a new mode.

## 3. Findings (what breaks which constraint)

**F1 — CONFIRMED DIVERGENCE (shipped, no reload involved): mid-program
rebinds of a stored-handler dep produce wrong values under the default
compiled mode.** Repro (2026-08-15, tree at `54e7c30` + this branch):

```boru
def bonus 1
def svc (service {})
add {op:"go"} ([req:Map state:Any] => [ bonus add 5 ]) svc
print (call {op:"go"} svc)     # interpreter: 6      compiled: 12
def bonus 100
print (call {op:"go"} svc)     # interpreter: 105    compiled: 12
def bonus 7
print (call {op:"go"} svc)     # interpreter: 12     compiled: 12
```

`boru --no-compile` prints `6 105 12` (the documented call-time-binding
semantics); default `boru run` and `--force-compile` both print
`12 12 12` — **every call sees the pass-final binding**. Mechanism: the
rebinds correctly poison the stored ref (`NotifyNameRebound`), so every
`call` is routed to `CallBoru` — but module-scope `def` sites execute
**only during the compile pass** (`def` is RunInCheck; "that single
execution is the RUN's"), so by VM time the def table already holds the
final value and the "live" lookup reads hoisted state. The PR #243
per-ref carve-out (`NoteFrozenRead` skips stored-ref-attributed reads,
`emit.go:5240-5247`) assumed the CallBoru route restores interpreter
semantics; it restores *late binding* but not *point-in-program* binding.
The whole-program hammer would have refused this program had the read
been in an ordinary unit. A regression spec must pin `6 105 12` across
all three surfaces.

> **CORRECTION, measured 2026-09-04.** That last sentence was false when it
> was written, and so was the §2.1 table cell it rests on. The hammer refuses
> a read whose VALUE a unit baked; it had no arm at all for a read whose CALL
> TARGET the lowering baked, because a fn name falls through `stepWord` to
> `Lookup` and dispatches — it never travels the simple-value substitution
> branch, so `NoteFrozenRead` was never even attempted and `frozenReads` stayed
> empty. The read in an ordinary unit therefore did NOT refuse:
>
> ```boru
> def helper fn [[x:Integer] [Integer] [x add 1]]
> def use    fn [[x:Integer] [Integer] [helper x]]
> use 1  def helper fn [[x:Integer] [Integer] [x add 100]]  use 1
> #   interpreted -> 2 101      compiled -> 2 2
> ```
>
> Closed by the call-target arm of the freeze discipline
> (`design/FULL-COMPILATION-HANDOFF.0.md`, Stage 4a-4), which also closed the
> same hole for a baked TYPE identity, and — in the same stage's second half —
> for a rebind written inside a `do` or multi-run body, where the recorder is
> suspended and the latch was never consulted at all (the record that carried
> that arm is Resolved and deleted).
>
> **And then the hammer itself went (Stage 4b, later the same day).** The
> read in an ordinary unit no longer refuses at all: the unit memo is
> binding-sensitive, so the second `use 1` re-records `use` against the new
> `helper` and the program compiles `2 101`. Whole-program refusal remains
> only for a unit whose reference escapes into a value — the stored-ref
> column's per-ref poisoning is the other survivor, and F1's point-in-program
> finding stands for it.

**F2 — reload-soundness hole: runtime rebinds cannot invalidate
compile-time refs.** `NotifyNameRebound` is an `EmitState` method gated
on `es.Active()` (`emit.go:2369-2371`) — it exists only during a compile
pass. A rebind performed *at runtime* (precisely what `reload` does)
poisons nothing, and a compile-time ref's `DepSnap == nil` makes
`DepsFresh` vacuously true — the VM would run stale baked code with no
check failing. Today this is unreachable mostly by accident: the REPL
behaves correctly (verified: `6` then `105` across lines — the store-site
detached stamp governs there), and replay-hazard screens refuse `import`
inside re-run bodies. The moment `reload` ships, F2 is a miscompile
generator.

**F3 — constraint-3 violation:** the per-invoke `DepsFresh` walk (§2.2).

**F4 — the restamp budget fights reload.** `RestampMaxTries = 3` is a
*lifetime* cap: the fourth reload of a plugin permanently de-optimizes
every dependent ref — and the permanently-stale original keeps paying the
walk + mutex per invoke because a successful twin is never republished to
the sig (`box.Cur` is re-checked per call; `Impl.Compiled` never swaps).
A live-reload session legitimately re-justifies recompilation that a
flapping production dep does not; the budget must tell them apart (§5.4,
and the survey's R5: HotSpot's cutoff is 400, per method, with
queue-feedback pacing — 3-per-lifetime is the wrong shape).

**F5 — false staleness from shadow/unshadow.** `Gen` bumps on pop too, so
a dep shadowed once and unshadowed reads stale-forever at the same depth
— burning restamp budget on a non-event.

**F6 — `Tui.run` update/view never stamp** (no trigger site in
`tui*.go`): on a program the runtime routed to the interpreter they
interpret every frame. Not a reload issue; noted because §5's index makes
the fix free to carry.

## 4. Prior art — how everyone else did it

(Condensed from the two survey passes; sources in the appendix of each:
HotSpot dependency contexts / deopt, Truffle Assumptions, Julia world
ages + backedges, BEAM export tables + BeamAsm, SBCL fdefns + block
compilation, .NET ReJIT/Hot Reload, Self/Smalltalk deopt + inline caches,
Revise.jl, Flutter hot reload, Vite HMR, Emacs native-comp.)

Two stable equilibria exist:

- **Cheap-swap indirection** (BEAM export table, SBCL fdefn, .NET
  precode): every cross-boundary call pays one predictable load/jump
  forever; redefinition is a pointer store. Honest, simple — and a
  *standing per-call cost*, which constraint 3 forbids (it is also what
  boru's emitter spent the ARRAYIFICATION/P7 era removing: baked consts
  and dispatch commitment are exactly the anti-indirection wins;
  reverting them "degrades to the OpLookupDynScope machinery that already
  exists", per the emitter audit).
- **Zero-cost speculation + push invalidation** (HotSpot, Truffle,
  Julia): compiled code carries **no guard at all**; the compile
  *registers a dependency edge* (nmethod dependency, Assumption object,
  backedge); the **redefinition event** walks the edges and invalidates
  precisely the dependents (patch entry / cap world / flip assumption);
  execution falls to a baseline tier and re-optimizes. Steady-state
  per-call cost: zero. The cost moved to the rare event.

The speculation family needs three capabilities, and **boru already has
all three**: (1) a safe transition boundary — the `InvokeCallback` seam
(unit granularity; next-invocation semantics, same as .NET Hot Reload and
Erlang's "new code at the next qualified call" — no OSR needed); (2) a
transition tier for code an invalidation has just unstamped — `CallBoru`
(exists, effect-fenced): somewhere to stand until the restamp lands,
never a destination for code the compiler failed on; (3) dependency edges
— `depNames`/`DepSnap` are already computed per ref at stamp time. What
is missing is only the **reverse index and the push event**. boru's
current scheme is neither equilibrium: it pays per call (pull) *and*
still needs restamp machinery.

Also load-bearing from the survey:

- **Julia's world pinning**: running code keeps its world; validity is
  decided at entry, so per-call cost is zero and in-flight execution gets
  snapshot consistency as a *feature*. This is the right semantics for
  boru's whole-program units (§5.5).
- **Erlang/BeamAsm**: a non-speculative per-module JIT changed *nothing*
  user-visible about reload — transparency comes from making the
  compiled tier target the same swap boundary the interpreter uses.
- **Revise.jl's ordering lesson**: shrink the invalidation surface
  *before* caching aggressively, and invalidate by dependency edges,
  recompile lazily.
- **Budget shapes that work** (HotSpot): per-site counts before
  per-method; each recompile should *learn* (drop the failed assumption)
  so convergence is monotone; a finite cap whose terminal state is the
  baseline tier, never an error; and counter reset on events that
  legitimately re-justify compilation — an explicit `reload` is exactly
  such an event.

## 5. The design: assumption edges, pushed invalidation, pinned worlds

One sentence: **replace the per-invoke DepSnap walk with a per-ref valid
flag flipped by the rebind event through a reverse dependency index, give
every ref (compile-time included) that flag, republish restamped twins,
and pin whole-program units to their stamp world with the seam as the
reload boundary.**

### 5.1 The valid flag (hot path → one atomic load)

`CompiledFnRef` gains an atomic `assumeValid` word. The invoke seam
becomes:

```
ref := CompiledRef(sig)
if ref != nil && ref.Prog != nil {
    if ref.assumeValid.Load() { run the VM }        // steady state: 1 load + branch
    else                     { slow path (§5.3) }   // walk → restamp → republish
}
```

The flag is a **hint, not a verdict** — this is the one place boru's
model differs from HotSpot/Truffle and it must be stated precisely.
Registries fork (`ForkConcurrent`), def tables diverge by design (an
old-generation service keeps its bindings until it drains — that is the
`HOT-CODE-LOADING.0.md` propagation model), and one ref is shared by
every fork. So:

- `assumeValid == true` must guarantee freshness **on every registry that
  can invoke the ref**. It is set only when the snapshot matches the
  stamp registry, and *cleared* by any mutation of a dep name **in any
  registry** (§5.2). A clear is global and conservative.
- `assumeValid == false` means "somebody, somewhere, rebound a dep": the
  slow path runs today's `DepsFresh(r)` against the *invoking* registry
  and decides per registry — fresh here (the rebind was elsewhere) → run
  the VM (do **not** re-set the flag; the rebinder's world is still
  divergent); stale here → `JitRestamp` exactly as today, with the
  per-registry `box.Cur.DepsFresh(r)` twin check unchanged.

Steady state after a reload settles: the restamped twin is published
(§5.3) with a fresh snapshot and `assumeValid = true` → back to the
one-load path. Old-generation forks pay the walk until they drain, which
is the draining generation's cost, not the current one's.

### 5.2 The reverse index (the push event)

A process-wide `CodeIndex`: `dep name → set of *CompiledFnRef*` (weak
refs so dead refs don't leak), populated at stamp time from the same
`depNames`/`DepSnap` the stamp already computes, plus a Bloom filter over
all indexed names.

The write path hooks the **existing** per-name generation bumps
(`DefTable` push/pop/replace/delete — the same sites that today make
`DepsFresh` fail): after bumping `gen[name]`, consult the Bloom filter;
on a miss (the overwhelmingly common case — frame params, body locals,
loop vars) the cost is one hash-and-test; on a hit, walk
`CodeIndex[name]` and `assumeValid.Store(false)` on each entry. This is
Truffle's `Assumption.invalidate()` with the def table as the mutation
source. Frame-binding churn is filtered structurally: only names that are
*some ref's dep* ever hit the index, and a shadow of such a name (a
body-local `def helper` over a module `helper`) **must** clear the flag —
that replaces the depth check the per-invoke walk performed.

Cost accounting: the hot *call* path drops from ~2×deps map lookups to
one atomic load. The *def* path gains a Bloom test (~ns) and, for watched
names only, an index walk — i.e., the cost moved to the rebind event,
which is the entire point. An explicit `reload` is the expensive case and
is allowed to be: re-import, rebind, index walk, restamps — all off the
steady-state path.

### 5.3 Republication (fixing the permanently-slow stale path)

`BoruImpl.Compiled` becomes an atomic pointer. When `JitRestamp` produces
a fresh twin, the winner **publishes it onto the sig**
(`Impl.Compiled.Store(newRef)`), so subsequent invokes take the one-load
path on the twin instead of re-walking + mutex + twin-check per call
forever. Old-generation registries that still match the *old* snapshot
keep working: the old ref remains reachable as the twin's `box` ancestor
(the restamp box carries the chain), or simply re-restamps for that
registry — either way bounded, never per-call.

### 5.4 Budget rework (reload is not flapping)

Replace the lifetime `RestampMaxTries = 3` with:

- **Per-world tries**: the counter resets whenever the *cause* is new —
  i.e. the dep generations the failed snapshot was built against differ
  from the current ones (each distinct rebind event re-justifies one
  compile). An explicit `reload <mod>` therefore always earns its
  restamp, however many reloads have happened (survey rule R5's missing
  decay, added).
- **A small consecutive-failure cap** (2–3) against genuine flapping —
  a dep rebinding *between* invokes repeatedly. Terminal mechanism
  unchanged: `CallBoru`, now reached cheaply (one atomic load fails, no
  walk, no mutex — the poisoned ref just declines). A ref parked there is
  a unit that stopped compiling: an open defect against the flapping
  case, owed a fix and tracked to closure, not a resting state.
- F5's false-staleness (shadow/unshadow gen bump) stops mattering: the
  restamp that follows re-snapshots at current gens once, and per-world
  counting doesn't tax it.

### 5.5 The three unit kinds under the new scheme

- **Detached refs** — as above; strictly faster than today.
- **Compile-time stored refs** — **unified with detached refs at
  Finalize**: every surviving (unpoisoned) stored ref gets a `DepSnap`
  computed against the pass-final def table, a `RestampBox`, and an
  `assumeValid` flag, and registers in the `CodeIndex`. There is then
  exactly **one** ref kind at runtime, and F2 closes: a runtime `reload`
  that rebinds a dep flips the flag like any other rebind, the seam
  routes that handler to the interpreter, and the restamp brings it
  back. (The §7a capture-identity concern is moot at Finalize — the pass
  is over; restamps go through `StampDetachedSig`'s clone discipline.)
- **Whole-program `CALL_USER` units** — **world-pinned, Julia-style**: a
  compiled program executes in the world its compile pass stamped; no
  per-call or per-statement checks are added (constraint 3), and no
  entry-patching machinery is built. Reload reaches a running program at
  its **seam crossings**: service dispatch, `receive`, `call`/`send`,
  `InvokeCallback` — which is where server code actually lives (mini-s3:
  the driver either pins or — as it does today — refuses, an open defect;
  all 23 callbacks ride the seam). The replay-hazard screens (`import` in
  re-run bodies) stay. This is the documented contract line (survey rule
  R3), and it is Erlang's own line translated: *new code takes effect at
  the next dispatch through a seam; a compiled straight-line loop that
  never crosses one keeps its generation until it returns.* `boru check`
  can warn when a loop body both reads reloadable module state and never
  crosses a seam — lint, not a mode.

### 5.6 Fixing F1 (point-in-program bindings under the VM)

F1 is orthogonal to reload but sits on the same fault line and the fix
belongs to this design: a module-scope `def` site whose name is **any
stored ref's dep** must not be hoisted-and-forgotten — the lowering
installs the registry-visible runtime bind twin at that site (the
`RecordDynBind`/`OpBindDynScope` machinery already exists for
dyn-scope reads; extend its criterion to "name ∈ CodeIndex ∪ any
stored-ref depNames"), so VM-time def order is real, the `CallBoru` route
reads point-in-program state, and — once §5.2 lands — the same runtime
bind is the push event that flips dependent flags. Until the lowering
lands, the interim stopgap is to widen the `NoteFrozenRead` hammer to
stored-ref reads of names the program text rebinds (refuse → whole
program interprets → no wrong answer). That stops the miscompile, but it
is not a fix: it trades a wrong answer for a compiler that fails on
programs it is supposed to compile, and every refusal it manufactures is
a defect owed closure by the bind twins above. The spec row pinning
`6 105 12` across all three surfaces lands first, either way.

### 5.7 Transparency (what the user sees)

Nothing new. No flags, no modes, no reload-vs-compiled decision. The
observable surface is diagnostics only: `--compile-report` grows
invalidation/restamp counters per ref; `call {op:"status"}` reports code
generation + stale/restamped counts per service (ties into
`HOT-CODE-LOADING.0.md` §5.5's generation observability); the `boru
check` lint of §5.5 names the one semantic line users must know, which is
the same line Erlang users already learn.

## 6. What to build, in order

1. **Pin the contracts**: spec rows for F1 (`6 105 12`, three surfaces)
   and a seam micro-benchmark (`InvokeCallback` with 0/2/5-dep refs,
   fresh and stale) — the baseline every later phase must not regress.
2. **F1 interim stopgap**: widen the frozen-read hammer to stored-ref
   deps rebound in program text — it stops the wrong answer now and makes
   the compiler fail on programs it is supposed to compile, so every
   refusal it opens is tracked as a defect until §5.6's bind twins retire
   it (step 6).
3. **Valid flag + reverse index + Bloom gate** (§5.1–5.2): DepsFresh
   drops off the hot path. Ship with the benchmark from (1) proving the
   steady-state win and the def-path cost bound.
4. **Republication + budget rework** (§5.3–5.4): atomic
   `Impl.Compiled`, per-world tries.
5. **Unify stored refs at Finalize** (§5.5): one runtime ref kind;
   closes F2 ahead of the `reload` word shipping.
6. **Bind twins for indexed names** (§5.6): F1 fixed structurally;
   remove the interim hammer widening.
7. **Stamp triggers for `Tui.run` update/view** (F6) — rides the same
   machinery.
8. **Wire `reload`** (`HOT-CODE-LOADING.0.md` §5.1) to the index: the
   reload event = rebind + flag flips + budget reset + module-load
   re-stamp — all mechanisms above, no new ones.

## 7. Open questions

1. **Index scope** — one process-global `CodeIndex` (simplest; matches
   ref sharing) or per-root-registry (tighter invalidation for `Vm.open`
   sub-engines, at the cost of registration plumbing)? (Leaning:
   process-global first; sub-engine partitioning when `Vm.open` lands.)
2. **Weak refs** — Go has no canonical weak reference; the index likely
   needs epoch-based sweeping (drop entries whose ref's Prog was
   collected) or registration tied to ref lifetime. (Leaning: sweep on
   index-walk, amortized into the rare event.)
3. **Flag-clear granularity** — clear on *any* watched-name mutation
   (simplest, conservative, some false invalidations on shadowing) vs
   distinguishing shadow-push from module-scope rebind at the def site
   (cheaper walks, more def-path logic)? (Leaning: conservative first;
   the slow path is a walk, not a recompile, so false clears are cheap.)
4. **World-pin lint shape** (§5.5) — checker diagnostic
   (`loop_never_crosses_seam`) or a documented-only contract? (Leaning:
   documented first, lint when a real program hits it.)
5. **Does Stage J change anything here?** When refusal stops being
   absorbed and surfaces as a compile error, the F1-interim hammer
   widening (step 2) turns the defects it opened into visible errors on
   shipped programs — the honest reading of what was always there, and
   the reason steps 5–6 must land before Stage J's flip, or the hammer
   widening must be scoped to the reload-bearing paths only.
