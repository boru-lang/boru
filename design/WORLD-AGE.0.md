# Julia's world age, and what it means for boru's compiled-code validity

A discovery note (2026-10-16), not an ADR: the maintainer asked for the
thread "World age for beginners: one way to compile a dynamic language"
to be investigated as useful to boru. The code was read at `fff7d8bd2`;
every cited line was re-checked at `f2f63a271`.

**About the sources.** I could not read the thread
(<https://discourse.julialang.org/t/world-age-for-beginners-one-way-to-compile-a-dynamic-language/134402>).
This session's egress proxy refused `discourse.julialang.org` (403 on CONNECT),
and WebFetch failed on DNS. The Hacker News discussion of it
(news.ycombinator.com/item?id=46584729) was refused the same way.

- **What the post says** comes only from search-engine snippets of it, marked
  **[S]**.
- **The replies could not be read at all.** Nothing below attributes a position
  to a reply.
- **The mechanism** comes from the primary sources the post explains, read from
  the JuliaLang GitHub sources:
  - the manual chapter *The World Age mechanism* (`doc/src/manual/worldage.md`,
    cited as **M**);
  - *Redefining Methods* (`doc/src/manual/methods.md`);
  - the devdocs section *Updates to the world counter*
    (`doc/src/devdocs/locks.md`, cited as **D**) and the CodeInstance fields
    (`doc/src/devdocs/ast.md`);
  - the `invokelatest` and `invoke_in_world` docstrings
    (`base/Base_compiler.jl`);
  - `HISTORY.md`;
  - the Juliette artifact README and its litmus tests
    (github.com/julbinb/juliette-wa, the companion to Belyakova et al.,
    OOPSLA 2020);
  - Holy, Bezanson and Nash, *Analyzing sources of compiler latency in Julia:
    method invalidations* (julialang.org blog, 2020, cited as **2020 blog**).

**boru probes** compare `lang.RunInterp` with `lang.RunCompiled` on one
program at a time (a scratch two-lane harness, outside the repository).

## 1. Julia's world age in brief

**The counter.** There is one global, monotonically increasing world counter.
It is bumped by every change to the global method table and, since 1.12, by
every change to the global binding table: method, type and const definitions,
typed-global declarations, and `using`/`import` (M).

A definition is published in four steps (D):
1. take `world_counter_lock`;
2. compute `new_world = counter + 1`;
3. publish the method-table entry or binding partition with range
   `[new_world, ∞)`;
4. release-store the counter.

**Which world runs.** Each Task has a local world. It is never ahead of the
global counter but "may run arbitrarily behind it" (M). A new task inherits its
creator's world at creation and keeps it (M, "World age capture"; the 1.8 note
in `HISTORY.md`).

**When the world advances.** The world advances at:
- the start of every top-level statement and every REPL prompt;
- a top-level definition;
- `Core.@latestworld`;
- some macros, such as `@eval` (M).

It can only be raised permanently at top level. Inside a function, the same
constructs are a syntax error ("World age increment not at top level"), or
their world side effect is ignored. Hence "Julia may assume that the world age
does not change within the execution of an ordinary function … the key
invariant that allows Julia to optimize" (M).

**Consequence: `eval` cannot reach its own caller.** In Juliette's example, `g`
calls `f`, `eval`s a new `f(x)=0`, then calls `f` again.
- `g(5)` returns 1, not 0.
- The next top-level call, `g(666)`, returns 0 (README).
- The manual's `tryeval` raises `MethodError … method too new to be called from
  this world context` (methods.md).
- Indirection through another function does not help. Only returning to top
  level does (litmus tests).

**Escape hatches.**
- `invokelatest(f, …)` runs `f`, and everything `f` calls ("age propagates
  through calls", litmus), in the latest world at entry, then returns to the
  old world. It "is somewhat slower … and the type of the result cannot be
  inferred".
- `invoke_in_world(w, f, …)` freezes a world. REPL and editor tooling use it
  "to prevent unwanted method invalidation and recompilation latency"
  (docstrings).

**Compiled code is keyed to worlds.**
- Every `CodeInstance` carries `[min_world, max_world]`. `max_world = ∞`
  means "valid until a backedge says otherwise" (ast.md).
- Callees and bindings record **backedges** to the code that depends on them.
- A new definition walks those backedges and caps `max_world`. This is push
  invalidation.
- The next call finds no valid code instance and recompiles (D; 2020 blog).
- Inference runs in a `validation_world`. It promotes its results to
  `[w, ∞)` only if the counter did not move meanwhile (D).

**Cost model.** Compiled code carries no guard. Changes "don't take effect
immediately. This is key to Julia's ability to statically infer and compile
code to run fast, without the usual JIT tricks and overhead" (methods.md).
The cost lands at definition time (the backedge walk) and at the next call
(the recompile). When a function keeps acquiring methods, Julia stops
specialising on its method count, and the code widens instead of being
invalidated again (2020 blog, the `applyf` example).

**1.12: binding partitions.** Const and struct redefinitions now follow world
age. A binding is a list of `(world range → meaning)` partitions, and old
meanings stay reachable via `@world` (M; `HISTORY.md`). Ordinary **global
assignment is not world-aged**: it "becomes visible immediately" (M).

**What the post adds [S].**
- It frames the topic as "compilers need information ahead of time to make fast
  code".
- It argues that because `eval`'s effects land only at top level, compiled code
  need not account for mid-execution redefinition.
- It shows a `baz(newvalue)` that `eval`s `const pi = …` and then reads `pi`
  both directly and through `@invokelatest @__MODULE__().pi`. The reported
  result is `(4, 3, 4)`: the running method stays in its world, while
  `invokelatest` sees the new const.
- It notes that the const examples need 1.12, since the 2020 paper covered
  methods only.
- It presents world age as what makes caching compiled code feasible
  ("just-ahead-of-time").
- The snippets do not identify the author.

**Trade-offs.**
- The user-facing cost is the "method too new" error and having to reach for
  `invokelatest`.
- Invalidation causes latency: loading one package invalidated "hundreds or
  thousands of MethodInstances" (2020 blog).
- World numbers are not stable across precompilation (M).
- [S] 1.12 produced new binding warnings for some users.
- [S] The 2020 study found 4–9% of more than 4,000 packages bypass world age.

## 2. What boru does today

### The binding model is not Julia's

There is one table of per-name shadow stacks, `DefTable`
(core/go/deftable.go:32-50).

- **`def` at module scope is both definition and assignment.**
  `def n 0 while [n lt 3] [def n (n add 1) n]` → `1 2 3 z`
  (lang/spec/control.tsv:162-163).
- **Module names are not captured; they resolve at call time.**
  `def x 1 def f ([y:Integer] => [x add y]) def x 2 f 0` → `2`
  (lang/spec/edge-fns-1.tsv:13).
- **Body-local defs push onto the same stacks, so a callee sees its caller's
  local.**
  `def k 5  def f fn [[] [Integer] [k add 2]]  def g fn [[] [Integer] [def k 9  f]]  g  f`
  → `11 7` on both lanes (measured; also lang/go/analysis_order_test.go:45).
  That is dynamic scope, and no time-based world captures it.
- **A rebind inside a running body is visible to the next call.**
  `… do [ f  def k 9  f ]` → `7 11` (analysis_order_test.go:35; measured).

### Versioning primitives

- **Per-name generation.** `DefTable.gen` is bumped by every push, pop,
  replace and truncate (`touch`; deftable.go:49, 60, 67).
- **Table-wide count.** `mutations` counts all pushes and pops, frame bindings
  included (deftable.go:40).
- **Fork copies.** `Clone` copies the gen map into forks so that stamps compare
  across them (deftable.go:568-590).
- **Dispatch cache.** The interpreter's dispatch cache is keyed by
  `(name, gen)` (core/go/registry.go:411-423, 1186-1215).

### The three kinds of compiled unit

The taxonomy is RELOAD-INVALIDATION.0.md §2.1's.

**1. Whole-program units.**
- *How the compile pass works.* It executes the program once, in program order,
  so a top-level read bakes exactly what the interpreter reads at that point
  (compiler/go/unit_memo.go:5-17).
- *What gets baked.* A fn unit bakes module bindings as `PUSH_CONST`,
  `PUSH_TYPE` or `CALL_USER` (core/go/rebind_notify.go:5-10).
- *What runs.* The VM enters the baked target `p.Fns[in.Arg]` with arity,
  policy and parameter-contract gates but **no binding check**
  (eng/go/vm.go:5613-5614).
- *Routed dispatch.* A routed `DISPATCH_GENERIC` looks the word up live
  (eng/go/vm_generic.go:108) but still enters the committed unit (:73-87).
- *Validity is decided at compile time.*
  - `NoteFrozenRead` records the `Gen` of every baked binding
    (unit_memo.go:72-102; rebind_notify.go:140; check/go/check_fnbody.go:978).
  - `StartFnCompile` treats a memo hit as stale when any baked binding's
    generation has moved, and compiles a fresh unit for that call site
    (compiler/go/emit.go:8059; `unitStale`, unit_memo.go:142-172).
  - A unit whose reference escapes into a value cannot be re-recorded, so a
    later rebind of anything it baked fails the whole compile
    (`NotifyNameRebound`, emit.go:5258, 5286).
  - Bind twins replay each module-scope bind at its stream position, so
    VM-time binding order is real (core/go/bind_twin_apply.go;
    FULL-COMPILATION.0.md §6.5).

In Julia's terms: **each call site is compiled for the world it will run in,
and pays nothing per call**. The sequence of worlds is simulated at compile
time.

**2. Compile-time stored refs.** Their `depNames` and `poisoned` fields exist
only during compilation. At run time, `DepSnap == nil` reads as "vacuously
fresh" (compiler/go/bytecode.go:1145-1174, 1235-1239). This is
RELOAD-INVALIDATION's F2.

**3. Runtime-stamped refs.** These are fn values first applied through a seam
(`LazyStampFnSigAs`, compiler/go/stamp_runtime.go:412) and run-time token
bodies (`StampTokenBody`, :472).
- *Stamping.* They compile on a `ForkConcurrent` copy against live bindings,
  and snapshot `{Depth, Gen}` for every module dependency, following callees
  (stamp_runtime.go:205-217; `storedHandlerDepsDeep`, emit.go:5220).
- *Every invoke checks freshness.* Each invoke runs `DepsFresh`
  (bytecode.go:1235-1247; eng/go/compiled_runtime_vm.go:28-33;
  eng/go/vm_token_body.go:146-157).
- *Stale refs re-stamp, up to a budget.* `JitRestamp` recompiles against live
  bindings, at most `RestampMaxTries = 3` times over the ref's life
  (stamp_runtime.go:226-262).
- *Past the budget, the interpreter runs.* The seam falls back to `CallBoru`.
  lang/go/runtime_token_body_test.go:59-62 pins this, the past-budget row as
  "open, by design".

This is **pull** validation: two map lookups per dependency per invoke
(RELOAD-INVALIDATION §2.2), keyed by per-table generations plus shadow depth.

**Where boru still defers to the interpreter:**
- `CallBoru`, for an exhausted or declined stamp;
- token bodies with a replay hazard, a flow sentinel or `undef`;
- the census rows. The committed `GATE_STATUS.md`, which may be stale, lists
  `InvokeCallback:callboru×5` among 157 engine entries.

### What the project already decided

- **Julia's semantics were rejected.** "it *changed the language's*
  redefinition visibility … boru compiles the live def-stack read instead"
  (FULL-COMPILATION.0.md:344-349, 2503-2512; INTERPRETER-PERF-HANDOVER.0.md:229).
- **Julia's implementation was adopted, on paper.** RELOAD-INVALIDATION §4-5
  takes Julia's push invalidation and "world-pinned, Julia-style" whole-program
  units for reload. None of it is built: there is no `assumeValid` and no
  `CodeIndex` in the code.

### New findings from the probes

Neither divergence is in NUR.md. Both reproduce on main at `86bda1e7c`
(the merge base of the no-copy work), so neither is that work's.

- **D1: a redefinition takes effect before it runs.**

  | program | interpreter | compiled |
  |---|---|---|
  | `def h fn [[] [Integer] [1]]  def g fn [[] [Integer] [def h fn [[] [Integer] [2]]  0]]  h` | `[1]` | `[2]` |
  | the same, ending `h g h` | `[1 0 2]` | `[2 0 2]` |
  | `g`'s body as `[h def h … h]`, then calling `g` | `[1 2]` | `[2 2]` |

  In the first program `g` is never called, and the compiled run reports
  `compiled=true`.

  Likely cause, consistent with the code but not traced line by line:
  1. `runFnBodyOnce` analyses `g`'s body under a depth-only
     `Snapshot`/`Restore` (check/go/carrier.go:2954, 3098;
     deftable.go:536-549).
  2. A same-signature redefinition is a drop-then-push at the *same depth*
     (design/COMPILABLE-SUBSET.md:307-318).
  3. So the analysis's `h₂` survives into the module scope that the rest of the
     compile pass bakes.
- **D2: sibling `await` branches.**
  `import "boru:time-util"  def k 0  TimeUtil.await [ [def k 1  k] [def k 2  k] ]`
  → `[[1 2]]` interpreted, `[[fn 2]]` compiled. A single branch agrees, and so
  do branches that rebind different names. I did not investigate the cause.
- **H1 (unverified).** Generations are per table and are copied into forks.
  - Two forks that each rebind a dependency once reach the same
    `(Depth, Gen)` with different values.
  - A ref, or a `RestampBox.Cur` twin, is stamped in one fork and shared via
    the fn value's `BoruImpl`.
  - It would therefore pass `DepsFresh` in the other fork.

  D2 kept me from isolating a reproduction.

## 3. Mapping

| Julia | boru counterpart | difference |
|---|---|---|
| Global world counter | per-name `DefTable.gen`; table-wide `mutations` | No total order across forks or registries. `mutations` also counts frame pushes. |
| Task-local world, captured at task creation | each fork's cloned `DefTable` (core/go/fork.go:8-41); timers fork at schedule time | A fork's own writes are private too. There is no `invokelatest` to the parent. |
| The world advances only at top level | every `def` is visible at the next dispatch, mid-body included | No pinning. `def` doubles as assignment. Scope is dynamic. |
| `invokelatest` / `invoke_in_world` | not needed (always the latest) / fork snapshot, module sub-registry | The frozen world is implicit, not callable. |
| `[min_world, max_world]` on a CodeInstance; forward edges | whole program: a unit per call site, keyed by its baked generations. Runtime: `DepSnap` plus the restamp twin. | Static for whole programs; pulled on every invoke at run time. |
| Backedges and push invalidation | compile time only: `NotifyNameRebound` walks `storedFnRefs` | None at run time. `CodeIndex` is proposed, not built. |
| Lazy recompile; widening after repeated invalidation | `JitRestamp` (≤3 per ref), then the interpreter | Bounded, and it ends in the interpreter, not in wider code. |
| Binding partitions (1.12); global assignment not world-aged | shadow stacks without birth stamps, depth-only `Snapshot`/`Restore`; every `def` is assignment-like | A same-depth replace cannot be undone (D1). |
| "Method too new" error | none | The dynamic semantics never refuse. |

## 4. What boru could take from it

The proposals are ranked by value over cost.
- **P1–P4 are pure implementation.** Every current spec row and interpreter
  answer stands.
- **P5 and P6 change observable semantics.**

### P1. Backedges and push invalidation for runtime refs

*Implementation; medium size.*

**What to build** (RELOAD-INVALIDATION §5.1–5.3):
- a valid flag on each ref;
- a reverse index from dependency name to refs. It is filled from the existing
  `DepSnap`, behind a Bloom-filter test in `DefTable.touch`;
- republication of the restamped twin.

This is Julia's capping of `max_world`, with the def table as the event source.

**What it buys:**
- One atomic load per invoke, instead of two map lookups per dependency.
- The precondition for `reload` (F2, F3).

**A requirement Julia does not have.** Under dynamic scope, shadowing is a
validity event (today's `Depth` check). So a frame-local push of a *watched*
name must also clear the flag. The event source is "any touch of a watched
name", not a module-only counter.

**Forks.** Refs are shared across forks. The flag can only be a hint, and the
slow path stays per registry (§5.1).

### P2. Version stamps unique across forks

*Implementation; small.*

Have every `touch` draw from one process-wide atomic counter
(`gen[name] = world.Add(1)`).

- Generations stay monotone per name.
- They become unique across forks and registries, and totally ordered.
- That removes H1 by construction.
- It also makes "valid for worlds ≥ w" meaningful across module sub-registries,
  which `storedHandlerDepsDeep` skips today.

After VALUE-NO-COPY phase 4, pointer identity of the binding entry is the
alternative.

### P3. Versioned bindings for the check pass's sandboxes

*Implementation; small to medium. Fixes D1.*

Take 1.12's binding partitions as the model:
1. stamp each `DefEntry` with its birth stamp (from P2);
2. log every replaced or removed entry;
3. `Snapshot` becomes an O(1) stamp read;
4. `Restore` undoes every change newer than that stamp.

Same-depth replaces then become reversible.

**Stopgap, to price first:** run `runFnBodyOnce` under the existing
`SnapshotEntries`/`RestoreEntriesSnapshot` (deftable.go:383-460, already used
by macro expansion).

**Depth-only sites to audit:**
- check/go/carrier.go:738 and :2954;
- core/go/carrier_body.go:145;
- compiler/go/compiler_dispatch_record.go:493;
- basic/go/conditional.go:495.

### P4. Fall back to a live read, not to the interpreter

*Implementation; medium size.*

When the same dependency keeps invalidating a ref, re-stamp with that
dependency read **live** (`liveNames`; the routed-slot machinery exists).
Today the budget runs out and the call lands in `CallBoru`.

**Why:**
- It is Julia's widening ("stop making assumptions"), and the HotSpot rule
  RELOAD-INVALIDATION §4 already cites: "each recompile should learn".
- It converges monotonically.
- It retires the "open, by design" row (runtime_token_body_test.go:62).
- It serves FULL-COMPILATION.0.md §6.7's totality goal. That section's
  "unbounded but memoised" re-stamp still pays one compile per world for a
  flapping dependency.

### P5. World pinning for `reload`, at seam crossings

*Changes semantics, but only for `reload`, which has not shipped.*

RELOAD-INVALIDATION §5.5 pins compiled whole-program units to the world they
were compiled in. A reload reaches them only at seam crossings: service
dispatch, `receive`, `call`/`send`, and `InvokeCallback`.

**Under two-lane parity this is not an implementation choice.**
- The interpreter would see a reload at the very next dispatch.
- So the interpreter must adopt the same rule.
- Otherwise every program that reloads inside a seamless loop diverges between
  the lanes.

**Recommendation:** specify it as language semantics, with spec rows, before
`reload` ships. No current row conflicts.

**The natural boundaries.** In boru the world boundaries would be seam
crossings and fork creation. They play the part of Julia's top-level statements
and task creation.

### P6. Julia's rule for running code

*Changes semantics. Do not adopt.*

The rule would make definitions executed inside running code invisible until
control returns to top level.

**It would break pinned behaviour:**
- the analysis-order parity rows (analysis_order_test.go:32-57), for example
  `do [ f def k 9 f ]` → `7 11` and `[1 2] each [f def k 9]` → `[7 11]`;
- loops that use `def` as assignment (control.tsv:162-163;
  fn-locals-scope.tsv:141-144, 209-212, 256).

**It would also not address dynamic scope.**

**The narrowest variant** would pin only fn redefinitions made inside fn bodies,
which is the D1 family.
- It would flip the `g` probe from `[1 2]` to `[1 1]`.
- No spec row pins that case (control.tsv:207 covers only the untaken arm).
- It buys nothing over P3.
- It brings in Julia's main usability cost.

## 5. Risks and open questions

**Dynamic scope.** A world fast path must also count shadowing pushes of
watched names, or it is unsound. Julia never faces this, because its locals
are not in the global table.

**Concurrency.**
- Forks already snapshot bindings per task, which is boru's version of world
  capture.
- But fn values, their `BoruImpl` slots and their `RestampBox` are shared
  across forks, so validity is per registry (H1).
- D2 shows that the compile pass already models sibling branches wrongly.
- Pending timers keep their schedule-time snapshot (fork.go:27-30), so
  `reload` will not reach them. That should be stated.

**Modules.**
- Each sub-registry has its own table and generations.
- Dependencies of callees homed in another module are skipped.
- So the index and the stamps must span registries (RELOAD-INVALIDATION §7,
  Q1).
- Go has no weak references, so the index's lifecycle is an open question
  (§7, Q2).

**The check pass.** Analysis runs the program and mutates bindings; D1 is one
such mutation leaking. With P1 or P2, touches made only by analysis on forks
must not trigger invalidation storms. Those would be correct but slow.

**The no-copy plan.**
- "Immutable once published" (VALUE-NO-COPY.0.md:88-101) makes a baked
  constant safe while its binding is unchanged, so freshness reduces to binding
  identity.
- `SetTopData`'s in-place payload swap (deftable.go:134) would then have to
  mint instead.
- No version scheme here sees flex containers mutating in place, just as
  Julia's world age does not version its heap.

**Latency.** Julia's history includes invalidation storms (2020 blog).
- Measure re-stamp counts on module loads before stamping more widely.
- Build RELOAD-INVALIDATION §6(1)'s seam benchmark first.

## 6. Recommendation

**Do not import Julia's world-age semantics.**
- In boru, `def` is also assignment, and module names are dynamically scoped.
- Pinning a world inside running code would break pinned spec rows and the
  documented late binding. The project reached the same conclusion before
  (FULL-COMPILATION.0.md:344-349).
- The whole-program compiler already has Julia's key property: compiled code
  with no per-call guard. It gets it by simulating the program's sequence of
  worlds at compile time, through Stage 4b's binding-sensitive memo.

**Do import Julia's implementation:**
- backedge-driven push invalidation for runtime refs (P1, the unbuilt
  RELOAD-INVALIDATION §5);
- process-unique monotone version stamps (P2);
- versioned bindings, so the check pass rolls back exactly (P3);
- widening to live reads instead of falling back to the interpreter (P4).

**Treat world pinning as a language decision for `reload` alone (P5).** Specify
it for both lanes before `reload` ships.

**File D1 and D2 as NUR records now.**
