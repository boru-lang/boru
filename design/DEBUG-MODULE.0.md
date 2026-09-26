# boru:debug — a debugging & introspection module

> **2026-09-26 (NUR063).** Seven of these words are self-knowledge, not
> debugging — `words`, `defs`, `modules`, `sig`, `body`, `deps`, `shape` —
> and their canonical home is now **`boru:scry`** (`design/BORU-SCRY.0.md`).
> Both modules build them from one constructor (`selfKnowledge`,
> `lang/go/modules/scry.go`), so the answers cannot fork. The `Debug.*`
> copies are frozen at these seven, documented deprecated in `describe`,
> and removed in the first minor release after the one that ships
> `boru:scry`. The rest of this note stands.

Status: **implemented through Phase 3 (every in-process surface).** Built
and shipping in `lang/go/modules/debug.go` + `debug_step.go`: all of
printing, structural/system introspection, value sizing, performance
measurement, **and interactive stepping** — 27 words: `tap`, `label`,
`dump`, `assert`, `todo`, `parse`, `deps`, `explain`, `sig`, `body`,
`watch`, `stack`, `disasm`, `words`, `defs`, `modules`, `sizeof`, `shape`,
`heap`, `gc`, `steps`, `time`, `bench`, `trace`, `profile`, `step`,
`break`, `break-when`, `run-stepped`. Backed by three small engine seams
(`Engine.SetTrace` for the step/profile hook, §6.1; `Registry.CurrentStack`
via a current-engine stack for `Debug.stack`, §6.2; the `DebugOps` /
`StepController` host capability for stepping, §6 / §7.4-style), with docs
(`docs_debug.go`), Go tests (`debug_test.go` — incl. a headless scripted
step controller), and an executable spec (`lang/spec/module-debug.tsv`).

The **§7 cross-process features** now have a **working host-level
implementation** that does not wait for the boru `Service` model: attaching
to a running runtime and the serverless debug channel are realized on Go's
`net/http` (`lang/go/debugserve`) with the `boru debug serve` / `boru debug
attach` CLI — a debug server wraps a live `*native.Registry` behind
authenticated HTTP introspection (`words`/`defs`/`heap`/`eval`) and an
invocation-keyed event relay, with a Bearer token (static or a vault
capability id) and a loopback-default bind. What remains is the *elegant
unification* the design targets — routing these through a first-class boru
`Service`/`DebugTarget`, a boru-level `Debug.attach` word over a `connect`
transport, remote stepping, and a **live** (refreshing) TUI dashboard
host. Those still want the `Service`/`Process` actor layer
(`SERVICES.0.md`/`PROCESSES.0.md`, RFC-only) and a language-level socket
primitive; the host-level core proves the capability and the auth model in
the meantime (§7.2/§7.3/§7.6).

This captures the design of the native module `boru:debug` (namespace
`Debug`) that collects boru's debugging affordances behind one import:
simple printing, interactive stepping, and structural / memory /
performance analysis of both a program and the running system.

Per `lang/go/CLAUDE.md` this is a *framework / capability* module, so the
id stays plain (`boru:debug`, not `-util`) and the namespace is `Debug`.


## 1. Why a module (and why not core)

The pieces of "debugging" already exist in scattered, partial forms:

- `print` (core) writes a value and **consumes** it — no passthrough, so
  you cannot drop it into the middle of a pipeline without restructuring
  the code.
- `IO.trace` (`boru:io`) runs a quoted body in a sub-engine and prints the
  full step-by-step stack evolution. This is the single richest debug
  affordance today, and it is buried in the I/O module.
- `Vm.parse` (`boru:vm`) returns the parsed token/value list without
  running it — a structural view, but framed as a VM concern.
- `inspect`, `typeof`, `is`, `describe`, `help` (core) answer
  per-value / per-word structural questions.
- `boru:report` pretty-prints records / tables / lists.

These were each added where they were first needed, not where a user
*looks* for them. A developer who wants to debug should reach for one
import — `import "boru:debug"` — and find printing, tracing, stepping, and
introspection together, composed and consistent. The module **reuses**
the underlying machinery (it does not fork it): `Debug.trace` shares
`eng.RunTrace`, `Debug.parse` shares the parser path `Vm.parse` uses,
the pretty-printers delegate to `boru:report`, and the structural words
delegate to the existing `inspect` / `typeof` / `describe` algorithms.

These words stay where they are for back-compat; `Debug.*` is the curated
front door, and a couple of genuinely new capabilities (interactive
stepping, profiling, memory stats) live only here.

Keeping it a module also means **zero cost when unused** — debugging
touches the host (stdout, the clock, the Go runtime, interactive input)
and pokes at the registry, so it is naturally a *capability* gated by
policy. A program that never imports `boru:debug` pays nothing and is
granted nothing.


## 2. Design principles

1. **Value-returning where possible; effects are explicit.** A debug
   word that can return data does (so it composes), and the few words
   that print do so as a deliberate, labelled side effect. The signature
   "print *and pass the value through*" (`Debug.tap`) is the workhorse —
   it is the gap `print` leaves.
2. **Pure analysis is pure.** Parsing source, sizing an in-memory value,
   inspecting a word's signature, or disassembling a body needs no host
   capability and is allowed even under restrictive policies. Only the
   *effectful* surfaces (writing output, reading the clock, sampling Go
   memstats, blocking for interactive input) require the `debug` scope.
3. **Forward-form canonical.** Every example is written `Debug.word a b`
   (see `eng/go/CLAUDE.md` "Surface form recommendation"). Inner natives
   registered into the sub-registry use `BarrierPos: -1` so the dotted
   infix form dispatches too (the module-wrapper rule in
   `lang/go/CLAUDE.md`).
4. **Build on existing seams, add none gratuitously.** The engine already
   exposes a per-step `TraceCallback` (`Engine.trace`), a `Recorder`
   (StackForm), `EffectiveClock`, the `inspect`/`typeof` algorithms, and
   the `vm` sub-engine. The module is mostly a *curation and packaging*
   layer over these. The one new engine-side concept is a `DebugSession`
   (§6) that owns the step hook for stepping and profiling.
5. **Sub-engine isolation, parent attenuation.** Words that *run* user
   code (`Debug.trace`, `Debug.time`, `Debug.bench`, `Debug.step`) run it
   in a sub-engine composed under the parent policy, exactly as `boru:vm`
   does — a debug word can never grant the traced code more than the
   parent already had.


## 3. The five surfaces

Below, each surface lists its proposed words with signatures in
forward form. `~>` denotes "returns". Types in `Capitalised` are boru
lattice types; new module-owned types are defined in §5.

### (A) Printing & tracing — "what is flowing through here?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.tap` | `Any ~> Any` | Print the value (formatted), then **return it unchanged**. The pipeline tap `print` can't be. `(compute) Debug.tap further-process`. |
| `Debug.label` | `Any String ~> Any` | Like `tap` but prefixes a label: `x "after-map" Debug.label`. Prints `after-map: <value>`, returns `x`. |
| `Debug.dump` | `Any ~> Any` | Like `tap` but prints the *full* `inspect` view (type, structure, provenance) rather than the short form. Returns the value. |
| `Debug.watch` | `Atom/q ~> None` | Print the current binding of a def each time control passes the word: `x/q Debug.watch`. Resolves the name in the live def stack. |
| `Debug.trace` | `List ~> Any` | Run a quoted body in a sub-engine, printing the step-by-step stack evolution. Delegates to `eng.RunTrace` (the engine behind `IO.trace`); re-exported here as the discoverable home. |
| `Debug.assert` | `Boolean String ~> None` | If the boolean is false, raise `[boru/assertion_failure]` with the message; else no-op. (Distinct from `boru:test`'s `assert.*`, which are suite-scoped.) |
| `Debug.todo` | `String ~> Never` | Always raises `[boru/not_implemented]` with the message. A typed hole that the checker treats as `Never` so it unifies anywhere. |

`Debug.tap` / `Debug.label` are the headline ergonomics win: printf-style
debugging that doesn't force you to break a concatenative pipeline apart.

### (B) Interactive stepping — "walk me through execution"

The engine's `Engine.trace` hook already fires once per step with
`(step, pointer, stack, note)`. `Debug.trace` consumes that
non-interactively. Stepping makes it **interactive**: install a hook that,
at each step (or each breakpoint), pauses and consults a controller for
the next action (step / continue / step-over / inspect / quit).

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.step` | `List ~> Any` | Run the body in a sub-engine under interactive control. At each step, prints the current stack + pointer and prompts (via the host `DebugOps` controller, §6) for `n`(ext) / `c`(ontinue) / `s`(tack) / `q`(uit). Returns the body's residual top-of-stack. |
| `Debug.break` | `None ~> None` | A breakpoint marker. When `Debug.step` (or a stepping session) reaches it, control pauses regardless of step/continue state. A bare no-op when no debug session is active, so leaving `Debug.break` in code is harmless in production. |
| `Debug.break-when` | `Boolean ~> None` | Conditional breakpoint: pauses only if the boolean is true. |
| `Debug.run-stepped` | `String ~> Any` | Parse + step a source string (the REPL / CLI entry point), rather than a pre-quoted body. |

Non-interactive hosts (CI, wasm playground, tests) supply a *scripted*
controller — a pre-recorded list of actions — so stepping is testable
without a TTY. The REPL supplies a line-reading controller; a future
`boru debug <file>` CLI subcommand (out of scope here, noted in §9) would
supply a richer one.

### (C) Program structural analysis — "what is this code/word made of?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.parse` | `String ~> List` | Parse source to the quoted token/value list without running it. Shares the path behind `Vm.parse`; re-exported as the natural home for "show me the AST". |
| `Debug.disasm` | `List ~> List` | Compile a quoted body to its StackForm op list (the `Recorder` output) and return it as inspectable data — the "bytecode view". |
| `Debug.sig` | `Atom/q ~> List` | The signatures of a word, as structured data (params, returns, barrier position) — the machine-readable form of what `describe` renders. |
| `Debug.body` | `Atom/q ~> Any` | For a boru-defined fn, the quoted body tokens; for a native, a descriptor noting it is host-implemented. |
| `Debug.deps` | `List ~> List` | The set of word names a quoted body references (via the body-walker `WalkBodyWords` already used for closure capture). Useful for "what does this touch?". |
| `Debug.explain` | `Atom/q ~> String` | The full `describe` text for a word, returned as a String (so it can be embedded, not just printed). |

These are pure — no policy needed — because they read source, the
registry's static word table, or an in-memory value.

### (D) System structural analysis — "what is the running system?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.words` | `None ~> List` | Every registered word name (the registry's native + def word table). |
| `Debug.defs` | `None ~> Map` | Current value/type bindings in scope (`r.Defs.Names()` → top binding each), as a Map. |
| `Debug.modules` | `None ~> List` | Imported modules with their id / kind / exports (from the Module descriptors). |
| `Debug.types` | `None ~> List` | The type lattice: every minted/builtin type with name, rank, and parent — a dump of `r.Types`. |
| `Debug.stack` | `None ~> List` | A **snapshot of the current data stack** at the call site, as a list, without consuming it. The REPL `/stack` meta-command as a word. |
| `Debug.scope` | `None ~> Map` | The active scope chain: params, captures, and local defs of the enclosing fn frames. |

`Debug.stack` is subtle — a word cannot normally see "the whole stack"
because dispatch only hands it its matched args, and a native handler
receives `(args, named, body, r *Registry)` but **not** the live
`*Engine` (the registry doesn't hold one — `enterInterpRun` only bumps a
depth counter). So this is the **one** word that needs a new engine seam;
§6.2 specifies it (the recommended *gated-snapshot* mechanism) and weighs
the alternatives. Every other word in the module reuses an existing
seam.

### (E) Memory analysis — "what does this cost to hold?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.sizeof` | `Any ~> Integer` | Estimated retained byte size of a boru value (deep walk of lists/maps/strings/payloads). Pure — walks the value graph in-process. |
| `Debug.shape` | `Any ~> Map` | Structural census of a value: counts by kind (ints, strings, lists, maps), max depth, total node count. Pure. |
| `Debug.heap` | `None ~> Map` | Go-runtime heap stats (`runtime.MemStats`: alloc, total-alloc, heap-objects, num-GC) as a Map. **Host capability** — only meaningful with a real runtime; gated under `debug`. |
| `Debug.gc` | `None ~> Map` | Force a GC and return before/after heap deltas. Capability-gated; useful for "is this leaking?". |

`Debug.sizeof` / `Debug.shape` are pure value analysis (no host).
`Debug.heap` / `Debug.gc` read the Go runtime and so are effectful
capabilities behind a `DebugOps` seam (so wasm / sandboxed hosts can stub
or refuse them).

### (F) Performance analysis — "where does the time/work go?"

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Debug.time` | `List ~> Map` | Run the body once, return `{result, elapsed-ms, steps}`. Wall-clock via `EffectiveClock` (so a `FixedClock` makes it deterministic in tests); step count from the engine loop. |
| `Debug.bench` | `List Integer ~> Map` | Run the body N times, return `{n, total-ms, mean-ms, min-ms, max-ms, steps-per-run}`. |
| `Debug.steps` | `List ~> Integer` | Run the body, return only the number of engine steps executed (cost in "instructions", clock-independent — the deterministic perf metric). |
| `Debug.profile` | `List ~> Table` | Run the body under the step hook, accumulating per-word step counts (and, if the clock is real, per-word time), returned as a Table sorted by cost. The concatenative analogue of a sampling profiler. |

`Debug.steps` is the *deterministic* performance signal — it does not
depend on the wall clock, so it is reproducible in CI and is the right
thing to assert on in regression tests ("this refactor must not increase
step count"). `Debug.time` / `Debug.bench` add the wall-clock dimension
for real tuning.


## 4. Composition examples

```
import "boru:debug"

# (A) printf-debugging without breaking the pipeline
[1 2 3 4] (each [dup mul]) Debug.tap (fold add 0) Debug.label "sum"
#   prints:  [1 4 9 16]
#   prints:  sum: 30
#   result:  30

# (C/D) what is this word, and what is in scope?
add/q Debug.sig            # ~> structured signatures of `add`
Debug.words                # ~> every registered word
Debug.stack                # ~> snapshot of the current data stack

# (F) is my refactor cheaper?
[expensive-thing] Debug.steps      # ~> 1842   (deterministic)
[expensive-thing] 1000 Debug.bench # ~> {n:1000 mean-ms:0.7 ...}

# (E) how big is this in memory?
big-table Debug.sizeof     # ~> 48211
big-table Debug.shape      # ~> {maps:300 strings:1200 max-depth:4 ...}

# (B) walk it interactively (REPL / debug CLI)
[tricky-pipeline] Debug.step
```


## 5. Module-owned types

Registered into the sub-registry per import (module-scoped, *not* global
builtins — they have no FixedID and reach the user via exports, exactly
like `boru:io`'s `StreamKind`). Reach them as `Debug.TypeName`.

| Type | Shape | Used by |
|------|-------|---------|
| `Timing` | `refine Record [result:Any elapsed-ms:Float steps:Integer]` | `Debug.time` |
| `BenchResult` | `refine Record [n:Integer total-ms:Float mean-ms:Float min-ms:Float max-ms:Float steps-per-run:Integer]` | `Debug.bench` |
| `ProfileRow` | `refine Record [word:String calls:Integer steps:Integer ms:Float]` | `Debug.profile` (Table of these) |
| `MemStat` | `refine Record [alloc:Integer total-alloc:Integer heap-objects:Integer num-gc:Integer]` | `Debug.heap` / `Debug.gc` |
| `StepRecord` | `refine Record [step:Integer pointer:Integer word:String stack-depth:Integer]` | stepping / profiling hook payloads |

Returning structured records (rather than ad-hoc maps) lets callers pipe
straight into `report.table`, assert with `boru:test`, or `getpath` into
fields with type safety.


## 6. Architecture & engine seams

### What already exists (reuse, don't rebuild)

- **`Engine.trace TraceCallback`** — `func(step, pointer int, stack []Value, note string)`, set on a sub-engine and driven by the Run loop. `eng.RunTrace` already builds on it. This is the foundation for `Debug.trace` (non-interactive), `Debug.step` (interactive), `Debug.steps`/`Debug.profile` (counting).
- **`Engine.recorder Recorder`** (StackForm) — the compiled op stream behind `Debug.disasm`.
- **`EffectiveClock(r)`** — the time source behind `Debug.time`/`Debug.bench`; honours an installed `FixedClock` for deterministic tests.
- **`inspect` / `typeof` / `describe` / `WalkBodyWords`** — the algorithms behind surface (C).
- **The `boru:vm` sub-engine pattern** (`runInSubEngine`, policy composition) — the isolation model every code-running debug word follows.

### What is new

1. **`DebugSession`** (lives in `eng/go`, alongside `trace.go`) — a small
   struct that owns an interactive/scripted step controller and the
   per-word accumulators for profiling. `Debug.step` / `Debug.profile`
   install a `DebugSession`-backed `TraceCallback` on the sub-engine.
   The controller is an interface:

   ```go
   type StepController interface {
       // OnStep is called at each engine step (or each breakpoint hit);
       // it returns the next action (Step, Continue, StepOver, Quit).
       OnStep(rec StepRecord, stack []Value) StepAction
   }
   ```

   Implementations: `lineController` (REPL/TTY), `scriptedController`
   (a fixed action list — used by tests and non-interactive hosts),
   `noopController` (production: `Debug.break` becomes a no-op).

2. **`DebugOps` host capability** (lang `capabilities/`, alongside
   `FileOps` / `Clock`) — the seam for the genuinely host-bound surfaces:

   ```go
   type DebugOps interface {
       MemStats() MemStat        // Debug.heap / Debug.gc
       ForceGC()                 // Debug.gc
       Controller() StepController // interactive stepping source
   }
   ```

   Default OS-backed impl reads `runtime.MemStats` and wires a TTY
   controller; the wasm / sandbox impl stubs MemStats to zeroes and
   supplies a no-op controller, so `boru:debug` *loads* everywhere but its
   effectful words degrade gracefully (and are policy-refusable).

3. **`Debug.stack` seam** — the single new engine-internal read. It is
   not a body-wrapping word, so it can't use the `TraceCallback` route
   the others do; §6.2 specifies the mechanism and the recommendation.

### 6.1 Implementation mechanism, word by word

The headline fact: **almost every interesting word is a new *consumer* of
the per-step `TraceCallback` the engine already fires** — no new engine
code. The Run loop, today, runs:

```go
// eng/go/engine.go (Run loop, per step)
if e.trace != nil {
    snapshot := e.tape.Snapshot()          // the WHOLE stack: resolved + pending
    note := e.traceNote
    e.traceNote = ""
    e.trace(step, e.pointer, snapshot, note)
}
```

`eng.RunTrace` (behind `IO.trace`) already proves the pattern: build a
sub-engine, set `sub.trace`, run the body. Each debug word that *wraps a
quoted body* is the same shape with different bookkeeping in the
callback.

**(F) Performance + (A/B) body-runners — one sub-engine, five callbacks.**

```go
// shared scaffold for Debug.trace / steps / time / bench / profile / step
func runUnderHook(parent *native.Registry, body []native.Value,
        hook native.TraceCallback) ([]native.Value, error) {
    sub := native.New(parent)        // child engine; boru:vm policy-composition model
    sub.SetTrace(hook)               // thin exported setter over Engine.trace
    return sub.Run(append([]native.Value(nil), body...))
}
```

| Word | Callback does | Result built from |
|------|---------------|-------------------|
| `Debug.steps` | `n++` | `n` (deterministic — no clock) |
| `Debug.time` | `n++` | `EffectiveClock(r).Now()` deltas around `Run` + `n` → `Timing` |
| `Debug.bench` | `n++` | loop the scaffold N times, reduce clock deltas → `BenchResult` |
| `Debug.profile` | read `snapshot[pointer]`; `prof[word].{calls,steps}++` | reduce `prof` → Table of `ProfileRow` |
| `Debug.trace` | (delegates to `eng.RunTrace` verbatim) | printed trace |
| `Debug.step` | consult the `StepController` (below); may block | residual top-of-stack |

So profiling and step-counting are *zero new engine code* — they read the
word sitting at `snapshot[pointer]` each fire. `Debug.steps` is the
clock-free, reproducible perf signal; `Debug.time`/`bench` add wall-clock
via the existing `EffectiveClock` capability (a `FixedClock` freezes it
for tests).

**(B) Interactive stepping — the callback blocks.** `Debug.step` installs
a `DebugSession`-backed hook that, at a single-step or a breakpoint,
consults the controller and can block for input:

```go
sub.SetTrace(func(step, pointer int, stack []native.Value, _ string) {
    if !session.shouldPause(pointer, stack) { return }   // continue: cheap path
    rec := native.StepRecord{Step: step, Pointer: pointer,
        Word: wordNameAt(stack, pointer), StackDepth: pointer}
    switch session.ctl.OnStep(rec, stack) {
    case native.StepQuit:     panic(stepQuit{})            // recovered by the wrapper → clean stop
    case native.StepContinue: session.single = false
    case native.StepNext:     session.single = true
    }
})
```

`Debug.break` is a registered **no-op native** (`debug-break`); the hook
recognises `IsWord(stack[pointer]) && name == "debug-break"` and forces a
pause. With no session installed the word does nothing — so it is inert
and safe to leave in committed code. `shouldPause` returns false on the
fast path, so an active session that is "continuing" pays only a word-name
compare per step.

**(C) Program structure — direct delegation, no host, no sub-engine.**
These take the `r *Registry` the handler already gets and call existing
algorithms: `Debug.parse` → `r.ParseFunc(src)` (the `Vm.parse` path);
`Debug.disasm` → `runUnderHook` variant with `sub.SetRecorder(rec)`,
return the StackForm ops as a list; `Debug.sig`/`body`/`explain` →
`native.FnDefFromValue` + the `describe` formatter; `Debug.deps` →
`native.WalkBodyWords` (the closure-capture walker).

**(D) System structure — read the registry off the handler arg.**
`Debug.words` → `r.Defs.Names()` ∪ native table; `Debug.defs` →
`r.Defs.Names()` then `r.Defs.Top(name)` each; `Debug.types` → walk
`r.Types`; `Debug.modules` → the Module descriptors. All pure reads of
state the handler already holds.

**(E) Memory.** `Debug.sizeof`/`shape` are a pure recursive walk of the
`Value` graph (list elems, map entries, string/payload bytes) — in
process, no capability. `Debug.heap`/`gc` go through the new `DebugOps`
capability, retrieved exactly like the clock via the generic accessor:

```go
const CapDebugOps = "engine.debugops"
func EffectiveDebugOps(r *native.Registry) (capabilities.DebugOps, bool) {
    return eng.Cap[capabilities.DebugOps](r, CapDebugOps)   // host installs; policy wraps
}
```

The OS impl reads `runtime.MemStats`; the wasm/sandbox impl returns
zeroes. Policy-gated under `debug.runtime` via the existing
`HostPolicy`/`pol.Check` plumbing — no new policy machinery, just a scope
name (§8).

### 6.2 The `Debug.stack` seam (the one new engine read)

`Debug.stack` wants the live data stack *at its own call site*, but a
native handler never receives the `*Engine`, and the registry holds no
pointer to it. Three ways to close that, in recommended order:

1. **Gated snapshot (recommended).** When — and only when — a debug
   session is active, the Run loop publishes `e.tape.Snapshot()` to a
   registry field each step; `Debug.stack`'s handler reads it off `r`.
   The publish is guarded by the same `e.trace != nil`-style flag the
   trace path uses, so a program that never debugs pays **nothing** (no
   per-step snapshot, no allocation). This reuses the snapshot the trace
   path already computes when a session is live, so the marginal cost is a
   pointer store.
2. **Body-wrapping reframe.** Drop the bare word for
   `Debug.stack-of [body]`, which falls out of the `TraceCallback`
   scaffold for free (no new seam) — at the cost of worse ergonomics
   (you must wrap the surrounding code).
3. **REPL-only.** Keep the existing `/stack` meta-command and ship no
   word. Zero engine change, but the capability isn't programmatic.

Recommendation: **option 1**. It is the only one that gives the natural
`Debug.stack` ergonomics while keeping the non-debug path cost-free, and
the exposure is strictly read-only (the handler copies the published
snapshot; the live tape is never handed out or mutated). It is a small,
well-bounded addition: one guarded field write in the Run loop plus a
setter, mirroring how `PushArgs`/`TopArgs` already mirror per-call engine
state onto the registry for the `args` word.

### File layout (mirrors existing modules)

```
lang/go/modules/debug.go          # BuildDebugModule + sub-registry wiring
lang/go/modules/debug_test.go     # word-level tests (incl. negative rows)
lang/go/modules/docs_debug.go     # registerDocs("boru:debug", {...})
lang/go/capabilities/debug.go     # DebugOps interface + OS / stub impls
eng/go/debug_session.go           # DebugSession, StepController, profiling accumulators
lang/spec/debug.tsv               # executable spec rows (positive + negative)
design/DEBUG-MODULE.0.md          # this file
```

`BuildDebugModule` follows the `BuildIOModule` shape: a sub-registry,
inner natives with `BarrierPos: -1`, FnDef wrappers via the
`makeModuleFnDef` helper, module-scoped types via a `Mint…` call,
exported under the `"Debug"` namespace; registered in the `modules` map
in `modules.go` as `"debug": BuildDebugModule`; docs in `docs_debug.go`.

### FixedID range

Only the *global* externally-registered types need FixedIDs. The
`Debug.*` types above are **module-scoped** (minted per import, no
FixedID — like `IO.StreamKind`), so no FixedID allocation is required and
the `fixedid_stability_test.go` snapshot is untouched. Should any debug
type ever need to be wire-stable, draw from the reserved
`5000-9999` band per `eng/go/CLAUDE.md`.


## 7. Live dashboard, remote attach & the serverless debug channel

Surfaces (A)–(F) debug a program you are *running yourself*. This section
adds the three surfaces for debugging a system that is **already running**
— a live TUI dashboard of runtime state, attaching to a long-lived
process, and interrogating ephemeral serverless invocations — and the
authentication that makes them safe. All three reuse infrastructure boru
already has rather than inventing transports: the `api` service + its
discovery file, the bubbletea `tui` client, the `Service`/`call` model
(`SERVICES.0.md`), and the vault capability-token broker.

### 7.0 The unifying idea: a debug source is a `Service`

Everything here rests on one decision. **A unit of debuggable state is a
`Service`** (`SERVICES.0.md` §1) that answers two requests:

- `call {op:"meta"}` → a `WidgetMeta` (title, kind, refresh hint, layout).
- `call {op:"sample"}` → a `DebugCell` (the current data to render).

That single decision buys all three features at once, because `call`
obeys the **uniform "assume-remote" contract** (`SERVICES.0.md` §8): the
*same* `call {op:"sample"} src` works whether `src` is an in-process value,
a process in a running `boru serve` reached over a socket, or a serverless
invocation reached through a relay. The dashboard is just a client that
lays out a set of these services and polls each on a tick — exactly what
`cmd/go/internal/tui` already does against the `api` service today. Attach
and serverless differ only in **how the `Service` handle is obtained and
authenticated**, not in the protocol.

So the data model is wire-uniform and render-agnostic:

| Type | Shape | Notes |
|------|-------|-------|
| `DebugCell` | `refine Record [kind:String data:Any]` | `kind ∈ {"text","table","record","gauge","sparkline","log"}`; `data` is plain immutable values so it crosses a transport unchanged (`PROCESSES.0.md` `not_sendable` is satisfied — no `Store`/`Array`/`Object` in a cell). |
| `WidgetMeta` | `refine Record [id:String title:String kind:String refresh-ms:Integer span:Integer]` | layout + cadence hints; `span` is grid columns. |
| `DebugWidget` | a `Service` (or a constructor returning one) answering `{op:"meta"}` / `{op:"sample"}` | the contributable unit (§7.1). |

The Go TUI host renders a `DebugCell` by `kind`; the boru side only ever
produces **data**, never terminal escapes — which is precisely why the
same cell renders locally and streams back from a remote/serverless
target.

### 7.1 Live runtime dashboard (`Debug.dashboard`) — extensible via module widgets

```
Debug.dashboard [widgets] {refresh-ms:500 …} -> None     # boru entry: run a TUI
boru debug top [--attach <target>]                        # CLI entry (Phase 3)
```

`Debug.dashboard` takes a list of `DebugWidget`s, hands them to the
bubbletea host (the existing `cmd/go/internal/tui` model, generalised
from "a table of services" to "a grid of widget cells"), and polls each
widget's `{op:"sample"}` on its `refresh-ms`. The host owns layout,
input, and rendering; boru owns the widget set and the sampled data.

**Extensibility — widgets are contributed by other modules.** A widget is
just a `Service`-shaped value a module **exports**, so any module adds
dashboard content with no change to `boru:debug`. Discovery is two-tier:

1. **Explicit (always works):** the caller passes the widget list —
   `Debug.dashboard [ (Debug.heap-widget) (Serve.supervision-widget app)
   (Net.traffic-widget) my-widget ]`.
2. **Registry (zero-config):** a module advertises widgets by exporting a
   conventionally-named provider, `debug-widgets -> [DebugWidget]`.
   `Debug.discover-widgets` scans the imported modules' exports for that
   provider (the same export-walk `stampExportProvenance` already does in
   `modules.go`) and returns every contributed widget, so
   `Debug.dashboard Debug.discover-widgets` shows everything the loaded
   modules offer.

Built-in widgets shipped by `boru:debug` are thin wrappers over §3's
words: `stack`, `defs`, `heap`/`gc` (gauge), `profile` (table),
`steps`/`time` (sparkline of a sampled body), `processes` (§7.2),
`log` (a ring buffer the `Debug.tap`/`label` words can feed). Other
core modules are the obvious first contributors:

| Module | Widget | Cell |
|--------|--------|------|
| `boru:serve` | supervision tree — services, states, restart counts, mailbox depth/high-water (`SERVICES.0.md` §8.1) | `table` |
| `boru:net` | in-flight HTTP requests, status histogram, latency | `table` + `sparkline` |
| `boru:rand` | active seeded instances (for reproducibility audits) | `record` |
| user module | anything: queue depths, cache hit-rate, domain metrics | any `kind` |

A user module exports a widget the same way it exports a service
(`SERVICES.0.md` §2):

```boru
# metrics.boru — contribute a dashboard widget
export "debug-widgets" fn [[] [List] [
  [ ( Debug.widget {
        id: "cache" title: "Cache hit-rate" kind: "gauge" refresh-ms: 1000
        sample: [ [req state] => [ Debug.cell "gauge" (cache-hit-ratio) ] ]
      } ) ]
]]
```

`Debug.widget {meta… sample:[handler]}` is sugar that builds the
`Service` (an `add {op:"meta"}` returning the meta, an `add {op:"sample"}`
running the handler), so widget authors never touch the service words
directly unless they want `wrap`/`prior` layering (auth, caching) on a
widget — which they get for free because a widget *is* a service.

### 7.2 Attaching to a running process (`Debug.attach`)

A long-lived `boru serve` (or any program that opted into a debug
endpoint) exposes its processes/services as debug sources. Attaching is
**obtaining authenticated `Service` handles to them from outside**.

```
Debug.attach {target} {token:…} -> DebugTarget      # connect to a running runtime
boru debug attach <target>                            # CLI: opens the dashboard attached
```

- **Target resolution** mirrors the existing `tui`/`api` story: a **local**
  attach reads the `$TMPDIR/boru-api.json` discovery file (`{url, token,
  pid}`, mode 0600) the `api` service already writes (`cmd/go/internal/api`);
  a **remote** attach takes an explicit `{http: "https://host:port"}` and a
  token. No new discovery mechanism.
- **The debug endpoint is a service, not a new server.** Per
  `SERVICES.0.md` §5 the `api` service "introspects its own server"; the
  debug endpoint is the same idea extended with read-only introspection
  ops: `{op:"processes"}` (list `Pid`s, names, mailbox depth, state —
  `PROCESSES.0.md` §3), `{op:"inspect" pid:…}` (a process's bindings /
  current word / step count via §3's `Debug.*`), `{op:"sample" widget:…}`
  (drive a remote widget), and the **guarded** `{op:"step" pid:…}` that
  installs the §6.1 `DebugSession` hook *on that process's engine* and
  streams `StepRecord`s back. Because a served process is a single
  goroutine consuming its mailbox, a step request is just another message;
  it pauses that one process without touching its siblings.
- **`DebugTarget`** is a handle whose `processes`/`inspect`/`widgets`
  resolve to remote `Service` calls under the uniform contract — so
  `Debug.dashboard (Debug.attach {…}).widgets` renders a **remote**
  runtime with the identical code path as a local one. Attaching the
  stepper (`boru debug attach --step <pid>`) is the remote form of §3(B).

Attach is strictly **read-only by default**; stepping/pausing a remote
process requires an elevated scope on the presented token (§7.4), because
pausing a production process is a privileged, observable act.

### 7.3 The serverless debug channel

Serverless invocations break the attach model: they are **ephemeral**
(gone before you can connect), **cold/scaled-to-zero**, and usually
**cannot accept an inbound socket**. You cannot attach *to* them — so the
channel inverts direction: the invocation **publishes** to, and **polls**
from, an out-of-band **debug relay**, keyed by an invocation id.

```
# inside the function (one line, gated + no-op when no channel configured)
Debug.channel {invocation: id  relay: "…"  token: …}    -> DebugChannel
# from the interrogator
Debug.interrogate {relay:"…" invocation: id  token: …}  -> DebugTarget
```

Two cooperating flows over the same relay (which is itself a
`boru:serve` service — a `proxy`-shaped broker, `SERVICES.0.md` §6):

1. **Emit (always cheap).** When a channel is configured, `Debug.tap` /
   `label` / `assert` / `profile` and any widget `sample` **also** publish
   their `DebugCell`/event to the relay tagged with the invocation id, as
   fire-and-forget `send` with `overflow:"drop"` (`SERVICES.0.md` §8.1) so
   debugging **never** adds latency or backpressure to the hot path. With
   no channel configured the emit compiles to nothing — zero serverless
   overhead by default.
2. **Interrogate (on-demand).** `Debug.interrogate` subscribes to the
   relay for that invocation id and receives the event stream
   (post-hoc, like distributed tracing) **and** — for a still-running
   invocation that opted into a control channel — can issue
   `{op:"inspect"}` / `{op:"step"}` requests that the function picks up by
   **polling** the relay between steps (the function pulls commands; the
   relay never pushes into a sandbox that can't be reached). This is the
   `gen_server`-deferred-reply shape (`SERVICES.0.md` §1 `@from`/`defer`):
   the relay holds the interrogator's request until the function's next
   poll answers it.

The relay decouples the two lifetimes: a function can emit and vanish; an
interrogator can connect before, during, or after, addressing by
invocation id. For live stepping, a thin synchronous wrapper
(`boru debug invoke …`) holds the invocation open against the relay so a
human can step a single cold start — opt-in, since it defeats the
scale-to-zero economics. Aggregating many invocations (one relay, many
ids) is the natural extension and is where the dashboard's `log`/`table`
widgets point in serverless mode.

### 7.4 Authentication via `boru:vault` (the trust boundary)

Attach and the serverless channel cross trust boundaries — a debug
endpoint that exposes process bindings, lets you pause a production
process, or streams a function's internal state is a **high-value
target**. Authentication is therefore not a static `--token` but the
**vault capability-token model** already proven by the credential proxy
(`cmd/go/internal/vault/proxy.go`, `proxy_security_test.go`).

- **Capability tokens, not passwords.** `boru vault` mints a scoped,
  revocable capability for debug access:
  `boru vault grant debug --target <id> --scope debug.attach --expires 1h
  --budget 500`. The holder presents `Authorization: Bearer
  <capability-id>`; the endpoint validates exactly as the proxy does today
  — hashed token → alias binding → **revoked / expired / method / budget /
  host-policy** checks. The real session/keys never leave the broker; the
  token is a least-privilege handle, not a secret.
- **Scopes map to debug power (least privilege).** `debug.observe`
  (read-only widgets/metrics) < `debug.inspect` (process bindings, step
  records) < `debug.control` (pause/resume, install a remote stepper). A
  dashboard-only operator gets `debug.observe`; pausing a production
  process needs `debug.control`. These compose with the engine policy
  scopes of §8 — the token scopes gate *remote* access, the policy scopes
  gate what the *endpoint* is even willing to do.
- **The endpoint is a vault-style `proxy` (`SERVICES.0.md` §6).** Its
  `before` interceptors do capability auth + scope check; the `target` is
  the local introspection service; `after` records audit + decrements the
  budget. Denials carry the proxy's blame chain and **never leak state**
  in the error. Emergency revocation is the unified `call {op:"pause"}`
  control — a leaked debug token is killed centrally at the broker.
- **Protocol versioning + transport hygiene, reused.** The debug wire
  envelope carries `X-Boru-Debug-Protocol` (mirroring
  `X-Boru-Vault-Protocol`); a stale agent fails loud. Local endpoints bind
  loopback by default and require an explicit `--allow-public` to expose,
  exactly like the proxy. For the serverless relay, **both** legs
  authenticate: the function presents a *producer* capability (emit-only,
  scoped to its own invocation id) and the interrogator a *consumer*
  capability — so a compromised function cannot read another's channel and
  an interrogator cannot inject into the function beyond its granted scope.

The net: one auth model (vault capabilities) secures the dashboard, the
attach endpoint, and the serverless relay, with scopes giving
read-only-by-default and audited, revocable, time/budget-bounded access to
the privileged operations.

### 7.5 New module-owned types (this section)

| Type | Shape |
|------|-------|
| `DebugCell` | `refine Record [kind:String data:Any]` |
| `WidgetMeta` | `refine Record [id:String title:String kind:String refresh-ms:Integer span:Integer]` |
| `DebugWidget` | a `Service` answering `{op:"meta"}` / `{op:"sample"}` |
| `DebugTarget` | handle to an attached runtime; `.processes` / `.inspect` / `.widgets` → remote `Service` calls |
| `DebugChannel` | a configured serverless emit/poll channel bound to an invocation id |

These depend on the `Service` type (`SERVICES.0.md`) and the process layer
(`PROCESSES.0.md`), so §7 as a whole is **gated on those RFCs landing** —
called out in phasing (§9) and the gap below.

### 7.6 Dependency honesty

§7 is the most forward-looking part of this design and rests on
not-yet-built substrate. Concretely it needs: the `Service`/`call` model
and the served-process layer (both RFC-only today —
`SERVICES.0.md`/`PROCESSES.0.md`); a debug-introspection service on
`boru serve`; the bubbletea host generalised from the services table to a
widget grid; a `connect`/transport for remote `call` (the **TCP server
boru still lacks** — `SERVICES.0.md` §10); and a relay service for the
serverless channel. What it does **not** need to invent: the auth model
(vault capabilities exist), the discovery/transport bones (the `api`
service + discovery file exist), the renderer (the `tui` model exists),
or the data contract (it is plain immutable `DebugCell`s). The phasing in
§9 sequences §7 strictly after the in-process surfaces and the
process/service layer it builds on.


## 8. Policy & safety

A new policy scope **`debug`** governs the effectful surfaces:

| Scope op | Gates |
|----------|-------|
| `debug.print` | `Debug.tap` / `label` / `dump` / `watch` / `trace` (host Output writer) |
| `debug.step` | `Debug.step` / `break*` / `run-stepped` (blocks for input) |
| `debug.introspect` | `Debug.words` / `defs` / `modules` / `types` / `stack` / `scope` (reads registry) |
| `debug.runtime` | `Debug.heap` / `gc` (reads Go runtime) |
| `debug.serve` | exposing a debug endpoint on `boru serve` (§7.2): the local introspection service is *installable* only with this scope |
| `debug.remote` | `Debug.attach` / `dashboard --attach` / `interrogate` (outbound — opening a debug connection to another runtime/relay) |

The **remote** scopes (`debug.serve`, `debug.remote`) gate the
*engine/host* side — whether a runtime will host or originate a debug
connection at all. They are distinct from, and composed with, the
**vault capability scopes** of §7.4 (`debug.observe`/`inspect`/`control`),
which gate what a *presented token* is allowed to do once connected. Both
must pass: the policy scope says "this process may participate in remote
debugging"; the token scope says "this caller may perform this operation."

The **pure** words (`Debug.parse`, `disasm`, `sig`, `body`, `deps`,
`explain`, `sizeof`, `shape`, `steps`, `time`, `bench`, `assert`, `todo`)
need no scope — they read source / in-memory values / the static word
table and a clock that is itself already a capability. Code-running words
(`trace`, `step`, `time`, `bench`, `profile`) compose the child engine
under the parent policy (the `boru:vm` model), so traced code can never
exceed the parent's grants.

Default profiles: `sandbox` denies `debug.runtime` and `debug.step`
(no host runtime, no interactive blocking) but may allow
`debug.introspect` and `debug.print`. The repo's existing policy plumbing
(`HostPolicy`, `pol.Check`, per-scope `Installed()`) is reused verbatim —
no new policy machinery, just a new scope name.


## 9. Phasing

The surfaces have very different build costs; ship in order of
value-per-effort:

- **Phase 1 — printing + pure structural + deterministic perf** (small,
  no new engine seams beyond `Debug.stack`):
  `tap`, `label`, `dump`, `watch`, `assert`, `todo`, `parse`, `sig`,
  `body`, `deps`, `explain`, `words`, `defs`, `modules`, `types`,
  `stack`, `sizeof`, `shape`, `steps`, `time`, `bench`. Almost all of
  this is curation over existing algorithms.
- **Phase 2 — tracing + profiling** (drives the existing `TraceCallback`
  harder): `trace` (re-export), `profile`, `disasm` (needs the
  `Recorder`/StackForm wired through a sub-engine).

- **Phase 3 — interactive stepping** (`StepController` + `DebugOps`):
  `step`, `break`, `break-when`, `run-stepped`, plus a scripted controller
  for tests. The TTY/REPL controller and a possible `boru debug <file>` CLI
  subcommand are the host-side follow-on.
- **Phase 4 — runtime memory** (`DebugOps.MemStats`): `heap`, `gc`.

> **Shipped — all of Phases 1–4 (every in-process word).**
> `lang/go/modules/debug.go` + `debug_step.go` implement the full 27-word
> surface: `tap`, `label`, `dump`, `watch`, `assert`, `todo`, `parse`,
> `deps`, `explain`, `sig`, `body`, `disasm`, `words`, `defs`, `modules`,
> `stack`, `sizeof`, `shape`, `heap`, `gc`, `steps`, `time`, `bench`,
> `trace`, `profile`, `step`, `break`, `break-when`, `run-stepped`. Three
> engine seams back them: `Engine.SetTrace` (step/profile hook, §6.1),
> `Registry.CurrentStack` via a defer-balanced current-engine stack
> (`Debug.stack`, §6.2 — the "current engine" variant, chosen for
> zero hot-loop cost), and the `DebugOps`/`StepController` host capability
> (stepping; a `scriptedController` exercises it headlessly in tests).
> Deltas from the design as written: `stack` filters tape markers to a
> clean data stack; `watch` takes a `String` name (not `Atom/q`) to avoid
> the dotted-quote complexity; `disasm` returns the StackForm pretty-print
> as a String; `types`/`scope` are folded into `words`/`defs`/`sig` and
> not shipped as separate words; interactive `step` uses the `DebugOps`
> controller (a scripted controller in tests, a TTY one in the REPL host),
> and `StepQuit` *detaches* rather than preempting (no mid-Run interruption
> seam, and control-flow panics are forbidden).

Phases 1–4 are the in-process module and are all shipped — nothing in a
later phase blocks an earlier one. Phases 5–7 are the §7
remote surfaces; each **depends on prior RFCs landing** and so is
sequenced after them, not after Phase 4 alone:

- **Phase 5 — local dashboard + widgets.** **In-process core shipped:**
  `Debug.widget TITLE SAMPLE-SOURCE` builds a lightweight `{title, sample}`
  widget map (the open-Q6 "lighter contract that ships before services"
  answer — a `String` sample source sidesteps map-literal auto-eval), and
  `Debug.dashboard [widgets]` renders a one-shot SNAPSHOT of every widget
  to output, surviving a bad panel. Widgets are plain data, so any module
  contributes them by exporting a function returning a list of widget maps
  (`Debug.dashboard SomeModule.widgets`) — the extensibility goal, met
  without the `Service` layer. **Remaining:** the live-refreshing bubbletea
  TUI host (a cmd/go render loop polling widgets on a tick) and
  `Debug.discover-widgets` auto-discovery; the richer `DebugCell`/
  `WidgetMeta` typing and the service-shaped widget form arrive with the
  `Service` layer.
- **Phase 6 — attach.** **Host-level core shipped** (`lang/go/debugserve` +
  `boru debug` CLI): rather than wait for the boru `Service` model and a
  language-level socket, the attach surface is realized directly on Go's
  `net/http` — the substrate that already underpins the `api` service.
  `debugserve.Server` wraps a `*native.Registry` behind authenticated HTTP
  introspection (`/debug/words|defs|heap|eval`), `debugserve.Client`
  attaches over HTTP, and `boru debug serve [file.boru]` / `boru debug attach
  <words|defs|heap|eval|events>` are the user-facing front door (with a
  `$TMPDIR/boru-debug.json` discovery file, mirroring `api`). Auth is an
  optional Bearer token — a static token *or* a vault capability id,
  validated as the vault proxy does (§7.4); loopback-by-default with an
  explicit `--allow-public`. `lang.Boru.NativeRegistry()` is the one new
  accessor it needed. **Remaining (the Service-model unification):**
  routing through a first-class `Service`/`DebugTarget`, a boru-level
  `Debug.attach` word over a `connect` transport, and remote
  widgets/stepping — these still want `SERVICES.0.md`/`PROCESSES.0.md`.
- **Phase 7 — serverless channel.** **Host-level core shipped:** the
  `debugserve` relay carries the out-of-band channel — a function `Emit`s
  events keyed by invocation id (`POST /debug/emit?id=`), an interrogator
  reads them back (`GET /debug/events?id=`, `boru debug attach events
  <id>`), per-invocation isolated and ring-bounded. **Remaining:** the
  drop-policy boru-side emit wiring into `Debug.tap`/`label`, the
  poll-for-commands live-control leg, and the producer/consumer capability
  split — these layer on once the boru-level transport/`Service` form lands.

The auth model (vault capabilities) is reused, not built — wired in at
Phases 6–7 as the Bearer token the `debugserve` server validates.


## 10. Test discipline

Per `lang/go/CLAUDE.md`, every behaviour gets a paired negative test:

- `Debug.tap`/`label`/`dump` must **return the input unchanged** (assert
  the residual value, not just that something printed) and must print to
  the *registry Output writer* (capture it), not real stdout.
- `Debug.steps` must be **deterministic** across runs; pair with a row
  asserting that a *more expensive* body yields a *strictly greater*
  count (the contract is "counts real work").
- `Debug.time`/`bench` must use a `FixedClock` in tests so `elapsed-ms`
  is reproducible; assert structure/types, and assert that a real clock
  path doesn't panic.
- Negative rows: `Debug.sig undefined-word/q` errors (not silent `Any`);
  `Debug.sizeof` / `shape` on a **type literal / carrier** must not panic
  (the `TestTypeLiteralNoPanic` discipline); effectful words refuse under
  a denying policy with the documented error; `Debug.assert false "msg"`
  raises `[boru/assertion_failure]`, `Debug.assert true "msg"` does not.
- `Debug.step` is exercised with the `scriptedController` so the
  interactive path is covered headlessly; assert that `Debug.break`
  with **no active session** is a pure no-op (production safety).
- §7 (remote) discipline: a `DebugCell` must contain **only immutable**
  values — pair a positive render row with a negative one asserting a cell
  carrying a `Store`/`Array`/`Object` is refused at the transport boundary
  (`PROCESSES.0.md` `not_sendable`). Auth: assert an **expired / revoked /
  out-of-scope** capability token is rejected and that the error **leaks no
  state** (mirror `proxy_security_test.go`); assert `debug.observe` cannot
  reach a `{op:"step"}`/`{op:"pause"}` op. The serverless emit path must be
  a **no-op when no channel is configured** (zero overhead) and **never
  block** the hot path under `overflow:"drop"`.

Spec rows live in `lang/spec/debug.tsv`; Go-level tests (host capture,
policy refusal, no-panic, scripted stepping, capability rejection) in
`debug_test.go`.


## 11. Open questions for the maintainer

1. **Re-export vs relocate.** `Debug.trace` and `Debug.parse` duplicate
   `IO.trace` / `Vm.parse`. Re-export (keep both, `Debug.*` is an alias)
   is the back-compat-safe default proposed here. Do you instead want the
   canonical home moved to `boru:debug` with `IO.trace`/`Vm.parse`
   deprecated? (Affects whether this is purely additive.)
2. **`Debug.stack` engine seam.** §6.2 recommends the *gated-snapshot*
   read (one guarded field write in the Run loop, cost-free when no debug
   session is active, strictly read-only). Is that acceptable, or would
   you prefer the body-wrapping `Debug.stack-of [body]` reframe (no engine
   change at all) — keeping native handlers strictly arg-scoped?
3. **CLI surface.** Should interactive stepping get a first-class
   `boru debug <file>` subcommand (a Phase-3 host follow-on), or stay
   REPL-only for now?
4. **Scope granularity.** Is the `debug` scope with six ops
   (§8, incl. the two remote ops) the right shape, or should `debug.runtime`
   fold into the existing host/runtime gating and the remote ops into the
   existing `network`/`process` scopes instead of living under `debug`?
5. **Widget discovery convention.** Is a conventional
   `export "debug-widgets"` provider (§7.1) the right zero-config
   mechanism, or should widgets register through an explicit
   `Debug.register-widget` call (no magic export name) — trading
   discoverability for explicitness?
6. **Widget = full `Service`, or a lighter record?** Modelling a widget as
   a `Service` (§7.0) buys `wrap`/`prior` + location-transparency for free
   but couples `boru:debug`'s dashboard to the (unbuilt) service layer. Is a
   lighter `{meta sample}` record contract worth defining for a Phase-5
   in-process dashboard that ships *before* services, with an upgrade path
   to the service form?
7. **Serverless control channel — poll vs hold-open.** §7.3 makes live
   stepping of an invocation opt-in (a synchronous wrapper holds it open,
   defeating scale-to-zero). Is post-hoc event streaming (always cheap)
   sufficient for the serverless story, with live stepping deferred — or is
   interactive cold-start stepping a day-one requirement?
8. **Where do debug capability tokens live in vault?** Should
   `boru vault grant debug` be a first-class vault verb with its own
   audit/scope surface, or a thin convention over the existing
   capability-token machinery (`proxy.go`) with `debug.*` aliases? (Leaning
   thin convention — reuse, don't fork.)

No ADR entry is proposed — per repo policy this stays a `design/` note
until a maintainer says otherwise.
