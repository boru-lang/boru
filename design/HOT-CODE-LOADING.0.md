# HOT CODE LOADING

A report on boru's **hot code loading** ability — what the runtime can
already do, what BEAM does that it cannot, and the design for closing the
gap — written to ground the first of three requirements adopted into the
service model (`SERVICES.0.md` §7.4):

> Hot-code swapping allows you to build an extensible **plugin system**
> (in the style of the Pi coding agent's extensions) which **reloads live
> without dropping state**.

`SERVICES.0.md` §7.3 previously scoped hot code loading out ("not a goal").
That stance is **revised**: live reload is now a requirement, because the
plugin-system payoff is judged worth the (modest, as this report shows)
mechanism cost. The suffix on this file is an implementation-completeness
indicator (`legacy/IMPLEMENTATION-STATUS.10.ignore`): `.0` — design/report only, no
`reload` word exists yet.

## 1. Summary

boru is **structurally closer to hot code loading than the service RFCs
assumed**. The interpreter is late-bound end to end: word references
resolve through the def table **at call time**, module-level names are
deliberately *not* captured by closures, re-`def` shadows and `undef`
unshadows, and the bytecode layer already **self-invalidates** compiled
units whose dependencies are rebound (`NotifyNameRebound` → interpreter
fallback). File modules are **never cached** — a re-`import` re-reads,
re-parses, re-runs, and rebinds the namespace — and old function values
keep their defining module sub-registry alive by ordinary GC, which is
BEAM's old-code/current-code generation scheme without the purge.

What is missing is not a binding mechanism but a **protocol**: a blessed
`reload` operation with a defined propagation rule for *running*
processes/services (each fork holds an independent def-table snapshot), a
**state-migration hook** (the `code_change` analog — precedented by
aless's re-anchor), handler-stack hygiene on services, and a
**persistent, policy-attenuated sub-engine** so untrusted plugins can be
isolated and rebuilt wholesale. None of these requires new kernel
machinery; they compose from `import`, services, processes, and `boru:vm`.

## 2. What the runtime already provides

Each mechanism below is load-bearing for reload, with its evidence.

### 2.1 Late binding: names resolve at call time

- A bare word steps through the def table / native registry **when
  executed** (`stepWord`; resolution cascade in `core/go/resolve.go`).
  Closure capture deliberately excludes module-level and global defs —
  "Module-level / global def → **No** (stays dynamic)"; "Kernel word /
  native registration → **No** (registry lookup at call)"
  (lang/go/CLAUDE.md, "Closures and Capture"). The canonical example in
  that guide: `def x 1  def f ([y] => [x add y])  def x 2  f 0` → `2`.
- Namespace access `Ns.word` is the same, twice over: `Ns` resolves
  through the def table at call time, then `dot` reads the export map's
  current entry. Swap the binding (or grow the map) and the next call
  sees it.
- Recursion is a forward reference resolved via the registry per call —
  a redefined recursive fn re-enters its **new** self, unlike a
  BEAM-local call which stays in the old version until a qualified call.
- `def` shadows (`def x 1; def x 2` → 2), `undef` unshadows — the def
  table is a per-name stack (`core/go/deftable.go`), i.e. the runtime
  already has a reversible swap primitive.

### 2.2 Compiled code self-invalidates on rebinding

Hot swap usually dies at the compiler; here it already survived it:

- A `def` (or `undef`) of a name that an **already-compiled stored
  handler or spawn body** reads *poisons* that frozen unit —
  `r.Check.Recorder().NotifyNameRebound(name)` — so `Finalize` leaves it
  unstamped and `InvokeCallback` **falls back to `CallBoru`**, where the
  interpreter resolves the new binding at call time
  (`basic/go/native_definition.go`, the def- and undef-site comments).
- Runtime-stamped (**detached**) fn units — the module-load sweep, service
  handlers stamped at `add`, codec fns — carry **dep snapshots keyed on
  def-table generations** (per dep: shadow depth + mutation generation,
  generations carried across `ForkConcurrent` clones); `InvokeCallback`
  validates freshness on every invoke and falls back to `CallBoru` on any
  mismatch (`compiler/go/bytecode.go::DepsFresh`;
  `design/legacy/RUNTIME-STAMPING.0.ignore`). Wrong-code execution is structurally
  excluded; the cost of a swap is de-optimization, not incorrectness.
- Better: for detached refs the de-optimization is **temporary**. Each
  carries a JIT re-stamp box (`RestampBox`,
  `compiler/go/stamp_runtime.go::JitRestamp`; `legacy/REFUSAL-CLOSURE.0.ignore` §7c):
  a stale ref re-compiles against the **live** bindings at invoke time —
  a stable rebind pays one compile and runs on the VM again — bounded at
  `RestampMaxTries = 3` total re-compiles per ref so a hot rebinding loop
  degrades to the interpreter instead of paying a compile per invoke.
  Compile-time refs (created inside a whole-program pass) have no box: a
  poisoned unit stays on the interpreter for good. And since a re-imported
  module re-runs the stamp sweep at load (§2.3), the **new** generation
  enters compiled execution immediately either way.
- The one hard boundary: a **force-compiled program bakes its imports at
  the compile pass** ("the VM program does not re-import",
  `lang/go/native/native_module_module.go`, the `!Compiling` comment).
  Hot reload is an interpreter-path feature; a compiled unit re-enters
  the new world only through the fallback above, or by re-stamping at the
  next module load.

### 2.3 `import` is already a code loader you can run twice

- **File modules are never cached** ("file modules are never cached, so
  the count IS the finding", `native_module_module.go`,
  `runModuleBodyCover`). Every `import "./mod.boru"` re-reads the file,
  re-parses, runs the body in a **fresh sub-registry**
  (`RunModuleBody`), collects exports, and installs each as a namespace
  binding via `InstallDef` (`installExports`) — which *shadows* any
  previous binding of the same name. Re-import **is** reload, today, for
  the importing scope.
- Native (`boru:`) modules are cached per registry
  (`resolveNativeMod` → `LoadedDesc`), and a repeated import re-binds
  only absent names (`ensureExportsBound`) and idempotently re-runs the
  word-extension transplant — so builtin modules stay stable while file
  modules stay fluid. A plugin is a file module; the split is exactly
  right.
- Module loading re-runs **runtime stamping** over the fresh
  sub-registry's defs (`StampFnValueInPlace` at the end of
  `runModuleBodyCover`), so a reloaded module's fns re-enter the
  compiled fast path with new dep snapshots. No stale-code tax lingers.

### 2.4 Generational old code, purged by GC

An exported fn carries its defining sub-registry
(`resolveModuleExport` sets `fnDef.Registry = modReg`). After a
re-import:

- callers that resolve **by name** (`Ns.word`, service handler lookup)
  get the new generation;
- fn **values** extracted earlier keep executing the old generation,
  pinning the old sub-registry until the last reference dies.

That is BEAM's current/old code split with unbounded generations and no
`purge` — the Go GC is the purge. Consequences the design must own: no
"kill processes on old code" lever (arguably a feature), and a leak shape
(a long-lived holder of an old closure retains a whole module
sub-registry — observable, if needed, through the debug surface).

### 2.5 Fork semantics: what a swap does and does not reach

`ForkConcurrent` (`core/go/fork.go`) gives every process/connection actor
its own **clone of the def table** (independent stacks) while sharing the
read-mostly infrastructure by pointer. `DefTable.Clone` copies entries
shallowly: bound *values* with pointer payloads (Map's `*OrderedMap`,
flex containers, `Store`) remain shared; the binding *stacks* do not.

So after a parent-scope re-import:

- **new forks** (new connections, restarted services) see the new code;
- **running forks** keep their def-table snapshot — a rebind in the
  parent does not reach them (correct: it would race);
- **in-place mutation** of a shared pointer payload *is* visible through
  every fork — but concurrent mutation is exactly what the concurrency
  model forbids (`await` refuses reachable mutable containers at branch
  boundaries; `send` refuses mutable messages, `not_sendable`).

This settles the propagation design (§4): reload must reach running
processes **as a message**, not as shared mutation. Conveniently, an
actor that re-imports *inside its own loop* mutates only its own def
table — single-threaded, race-free, no locks — and its loop state
(bindings threaded through the tail call) survives untouched.

### 2.6 Services: swap the handlers, keep the state

The phase-1 service implementation (`lang/go/native/native_service.go`)
separates exactly what a live reload wants separated:

- **state** is a flex map that mutates in place through every alias (the
  documented divergence from the RFC's `Store`), owned by the service
  value and untouched by handler registration;
- **behaviour** is a patrun of per-pattern **handler stacks** — `add` on
  an existing pattern *pushes* (newest outermost, `prior` continuation)
  rather than overwriting.

So "reload a service's code without dropping its state" is nearly a
one-liner today: re-import the module, `add` the fresh handlers (they
shadow the old), keep the same service value. Two gaps: repeated
re-`add` grows the per-pattern stacks with no removal word (hygiene —
§5.3), and there is no blessed way to hand an existing state map to a
fresh `service` construction (adoption — §5.2).

### 2.7 Live injection surfaces already exist

- The CLI REPL keeps **one persistent registry per session**
  (`cmd/go/internal/repl/repl.go`: "One *Boru per session over the
  persistent registry") — incremental `def`s and re-imports against a
  live engine are the REPL's normal operation.
- `boru:repl` is that surface as a **network service** (a line-protocol
  REPL server evaluating via `boru:vm`;
  `legacy/NETWORK-IMPLEMENTATION-PLAN.0.ignore` §1.5) — i.e. remote live code
  injection into a running process already ships, policy-gated.
- `Debug.watch` reports every change to a binding
  (`lang/go/modules/docs_debug.go`) — binding mutation is an observable
  event, which a reload UI can hook.

### 2.8 The trigger and the precedent: watch + re-anchor (aless)

The `aless` viewer (repo `voxgig-boru/aless`) is the in-family precedent
for *live reload without dropping state*, at the data level:

- **Trigger.** `IO.watch` delivers change events headless, but its
  callback fork **starves under `Tui.run`** (aless dx-report §2 — the
  callback's concurrent fork needs the runtime the TUI driver owns while
  parked on its mailbox). The shipped workaround is the shape to bless:
  a **spawned metronome process** (`spawn` + sleep + `send {tag:"tick"}`)
  driving mtime+size polling from inside the owning loop. Reload
  triggers must arrive **as messages into the owner's loop**, not as
  out-of-band callbacks — the same conclusion §2.5 reaches for
  propagation.
- **State preservation.** On change, aless rebuilds the document and
  **re-anchors** the old view state onto it (`AlessTabs.reanchor`:
  cursor, expansion set, scroll — retained across the rebuild; a failed
  parse keeps the old view and surfaces the error). That is the
  `code_change` analog: *state survives; the artifact is rebuilt; a
  migration function maps old state onto the new artifact; failure keeps
  the old generation*.

### 2.9 Isolation for untrusted plugins: `boru:vm`

`Vm.run`/`run-with`/`run-sandbox`/`run-compute`
(`lang/go/modules/vm.go`) construct a **fresh registry under an
attenuated policy** (child ⊆ parent, enforced) per call. As shipped it is
one-shot — run to completion, return the last value — which is the right
*trust* boundary for a plugin but the wrong *lifetime*: a plugin engine
must persist between calls and be **droppable and rebuildable** for
reload. §5.4 proposes the persistent handle.

## 3. BEAM comparison

| BEAM mechanism | boru today | gap |
| --- | --- | --- |
| Code server; module table swapped atomically | re-`import` rebinds the namespace in the importing scope | no engine-wide swap; propagation is per-scope (§4 makes that the design, not a bug) |
| Two generations per module (current/old) | new namespace binding vs old fn values pinning the old sub-registry | **unbounded** generations; GC is the only purge |
| Qualified call `Mod:f()` picks up new code | name-resolved `Ns.word` / def-table lookup at call time | running forks hold def-table snapshots — reached only via reload messages (§4) |
| Local calls stay in old code until qualified call | captured fn values keep old code | same, by design |
| `code_change/2` state migration | aless `reanchor` precedent; service state survives handler swap (§2.6) | no blessed hook (§5.2) |
| `code:purge/1` kills lingering old-code processes | last reference dies → GC reclaims sub-registry | no forced purge; leak shape is "old closure retains module" |
| `.beam` load; JIT re-warms | `import` re-reads source; stamping re-runs at load; stale detached refs **JIT re-stamp** at invoke (`RestampBox`, ≤3 tries); `NotifyNameRebound` de-optimizes compile-time units permanently | force-compiled whole programs bake imports (§2.2) |
| `sys:suspend/resume` around upgrade | service dispatch mutex; `Pausable` / `{op:"pause"}` control requests (SERVICES §5) | wiring reload into the pause window is convention, not code |

The structural difference worth stating plainly: BEAM needs the module
table because its processes share **no** heap; boru's forks share
immutable values and the read-only infrastructure, so code distribution
can ride the **existing message plane** instead of a global table. That
keeps reload inside the concurrency model rather than beside it.

## 4. Design: reload as a protocol, not a mutation

The recommended shape — all pieces exist except the words:

1. **`reload <path>`** (sugar over `import`): re-import the file module,
   rebinding its namespace in the *caller's* scope. Because file modules
   are uncached this is today's behaviour given a name; the word exists
   so intent is visible, so the checker can treat it as an effect, and so
   it can return `{ok, generation, errors}` instead of raising — a failed
   parse/run **keeps the old binding** (the aless rule: a broken reload
   never takes down the running generation).
2. **Propagation is a control message.** A supervisor (`server`, once it
   lands) broadcasts `{op:"reload" module:…}` to its services; each
   service re-imports **inside its own dispatch turn** (mutex-held,
   race-free), swaps its handlers (§5.3), keeps its state map, and
   answers with its new generation. Processes handle the same message in
   their `receive` loop and tail-recurse into the new code. Nothing is
   mutated across a goroutine boundary; unreachable actors simply stay on
   the old generation until they drain — exactly the "assume remote"
   discipline of SERVICES §8 applied to code.
3. **The plugin unit is a module exporting a service constructor**
   (SERVICES §2 verbatim — no new plugin format). Host contract:
   `Plugin.New {opts} -> Service`, plus an optional
   **`Plugin.migrate old-state old-generation -> state`** the host calls
   between generations (the `code_change`/re-anchor hook). Absent
   `migrate`, state passes through unchanged.
4. **The trigger is a watcher actor**: `IO.watch` where it works, the
   metronome+poll pattern under a TUI driver (§2.8), either way
   delivering `{op:"reload"}` into the owner's loop. Auto-reload is
   therefore an *app-level* choice composed from shipped words, not an
   engine mode.

## 5. Gap list (what to build)

1. **`reload` word** — named re-import with keep-old-on-failure and a
   generation counter (§4.1). Small; entirely on existing machinery.
2. **State adoption + migration** — let `service` adopt an existing
   state map (`service (state-of old)` keeping identity, not copying),
   and bless the `migrate` hook (§4.3). Without this, "without dropping
   state" works only within one service value.
3. **Handler-stack hygiene** — a way to *replace* a pattern's handler
   stack rather than only push (`add {pattern} [h] svc {replace: true}`,
   or `clear-handlers {pattern} svc`). Otherwise every reload deepens
   the `prior` chain (`serviceState.stacks`, `native_service.go`).
4. **Persistent sub-engine (`Vm.open`)** — `Vm.open {policy} -> Engine`,
   `Vm.load code eng`, `Vm.call req eng`, `Vm.close eng`; reload of an
   untrusted plugin = `close` + `open` + `load`, with state held on the
   host side of the boundary and re-injected through `migrate`. This is
   the Pi-shaped tier: same-trust plugins load as modules into the host
   engine (cheap, shared), untrusted plugins get an attenuated engine
   each (isolated, rebuildable).
5. **Generation observability** — surface per-module generation and
   pinned-old-generation counts through the debug/status surface
   (`call {op:"status"}` already reports mailbox stats; add code
   generations) so the "GC is the purge" stance is inspectable.
6. **Reconciling reload with the compiled tier** — superseded as a
   "stated rule": `RELOAD-INVALIDATION.0.md` designs the mechanism that
   lets reload and transparent compilation coexist with *zero* hot-path
   cost (per-ref valid flags flipped push-style through a reverse
   dependency index, replacing the per-invoke `DepsFresh` walk;
   compile-time refs unified with detached refs at Finalize; world-pinned
   whole-program units with the `InvokeCallback` seam as the reload
   boundary; a per-world restamp budget replacing the lifetime cap). It
   also records a confirmed pre-existing compiled-mode divergence
   (mid-program rebind of a stored-handler dep) that must be pinned and
   fixed regardless of reload.

Explicit non-goals, matching BEAM experience: no in-place mutation of
shared export maps (racy; rejected in favour of §4.2's message
propagation), no forced purge/kill of old-generation holders, no
cross-node code distribution until distribution itself lands
(SERVICES §7.4.3).

## 6. Open questions

1. **Reload granularity** — per-module only, or also per-service
   ("rebuild this service from its module, migrate state")? (Leaning:
   per-module word + per-service convention via the supervisor message.)
2. **Who owns `migrate` failure** — keep old generation running (aless
   rule) or crash the service into its restart policy? (Leaning: keep
   old + report, letting supervision stay about faults, not upgrades.)
3. **Should `reload` bump a *namespace* identity** so `eq` can
   distinguish generations of `Plugin`, or stay value-transparent?
   (Leaning: transparent; expose generation via `$module`.)
4. **`Vm.open` capability scope** — gate persistent sub-engines on the
   existing `process` scope or a new `engine` scope? (Leaning: reuse
   `process`; an engine is a heavier process.)
