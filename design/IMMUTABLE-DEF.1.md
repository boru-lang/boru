# Immutable `def`, block scopes and `var` — the plan

**Status:** plan, 2026-10-06, decided with the maintainer question by
question (§1) on top of the measured investigation in
[IMMUTABLE-DEF.0.md](IMMUTABLE-DEF.0.md), whose §5–§8 this note closes. The
series lands on one branch with the new semantics as the default (no
transition flag in the language); `boru check --def-census` is the
migration aid. Nothing here is built yet; §5 is the order it is built in.

## 1 Decisions

| # | question | ruling |
|---|---|---|
| 1 | shadowing | **Legal everywhere**: an inner scope (a fn body, a block) may bind a *value* name an enclosing scope binds; lexical shadowing, dynamic visibility as today |
| 2 | overloads from an inner scope | **Error**: a block or fn body cannot add overloads to a word an enclosing scope bound; overloads are added only in the scope that bound the name |
| 2a | a value def over an enclosing word | **Legal**: `def f 5` inside a fn where `f` is a module fn shadows the word in that scope (dispatching `f` there pushes 5); only a *fn* def over an enclosing fn is the error of #2 |
| 3 | assignment spelling | **`var` again**: `var n 0` declares when no visible var `n` exists, `var n (n add 1)` assigns when one does |
| 4 | assignment reach | **Own scope and the blocks nested in it** — never across a fn boundary in either direction; a fn body cannot assign a module var |
| 4a | reading a module var in a fn body | **Legal, live**: read at call time, as module names are read today; only assignment is scoped |
| 5 | the untyped binder (`var [[e acc] …]`'s 63 kg sites) | **Untyped lambda params**: `[e acc] => […]`, a bare param meaning `Any` |
| 6 | a closure capturing a fn-local var | **Snapshot at creation**, as today's captures |
| 7 | exporting a var | **Refused** (`export` error); a module exposes state through fns |
| 8 | "values only" | **Syntactic**: `var NAME fn …` and `var NAME <Type>` are refused; a fn value that arrives at run time is stored and behaves as a fn value does today |
| 9 | rollout | **One series, default on**, with the census as the aid |
| 10 | the REPL | **Allowed with a notice** (`redefined x`) — its registry persists across lines by design |
| 11 | `do` | **Transparent**: `do`'s body is not a block; its defs reach the enclosing scope (one `do [def x …]` at module level is a module def) |
| 12 | `undef` | **Removed** from the language (§2.5) |
| 13 | the loop-counter shape under block shadowing (`def n 0 for 3 [def n (n add 1)] n`, now a legal block-local shadow answering `0` where today it answers `3`) | **Legal, with a check warning** `shadow_rebind` when a block-local def's value reads the very name it binds; the interpreter stays silent |

Everything the maintainer asked for in one sentence: *a name binds once
per scope; blocks are scopes; `def f fn […]` adds overloads and never
replaces one; `var` is the only mutable binding and holds values;
`undef` and the `var [[…]]` construct go.*

## 2 The rule

### 2.1 Scopes

- **Module scope**: the top level of a program, of a file module and of
  `import module […]`'s body (each is its own registry today).
- **Frame scope**: a fn or `=>` lambda body, per call.
- **Block scope**: a code body a word runs — the arms of `if` and `case`,
  the bodies of `for`, `while`, `each`, `fold`, `filter`, `scan`, `map`,
  `Test.test`, `Assert.throws` and every other word that runs a code list
  (the interpreter's `InvokeBody` seam and the inline splices of
  `stepMoveIf`/the loop regions). A block scope lives for one execution of
  the body: one iteration of a loop, one call of a callback.
- **Not scopes**: `do`'s body (#11), paren groups `( … )`, list and map
  literals.

A scope's bindings are visible to the code that runs while it is live,
including fns called from it (boru's dynamic scoping is unchanged; the
checker's call-graph rescue for dynamically visible names keeps working).

### 2.2 `def`

- `def NAME body` (and the typed `def NAME:Type body`) binds NAME in the
  **current** scope. If the current scope already binds NAME — by `def`,
  `var`, a type def, a param, an `unpack`, an `import` — the program fails
  with **`redefinition`**, naming both sites.
- An inner scope may bind a value name an enclosing scope binds (#1). A
  name bound to a **fn** in an enclosing scope may be shadowed by a value
  def (#2a) but not by a fn def (#2): `def f fn […]` where `f` is an
  enclosing scope's fn is `redefinition` ("cannot extend an enclosing
  scope's word from an inner scope").
- **Overloads are additive.** `def f fn […]` where the current scope binds
  `f` to a fn adds the new signatures. If any new signature *overlaps* an
  existing one (`FnDefsOverlap`, the test the engine runs today before it
  silently replaces) the program fails with `redefinition` ("overlapping
  overload"). The multi-triple `fn [[sig₁] [r] [b₁] [sig₂] [r] [b₂]]` form
  stays the one-site spelling.
- Core words keep today's rules: `def add 5` is `reserved_word`; `def add
  fn […]` is an extension admitted only with a nominal type the scope
  owns (`extend_owner`), and it is additive already.
- **Types.** A capitalised `def` binds a type and follows the same rule;
  `def Foo Integer def Foo String` is `redefinition`. `untype` no longer
  exists; LANGREF §Type Shadowing is deleted.
- **Fn params** are per-call frame bindings, not defs: they shadow as they
  do today (`InstallFrameBinding`), including a word's name.

### 2.3 `var`

- `var NAME value` and `var NAME:Type value`. If a var NAME is **visible
  within the current frame** — declared in the current scope or in an
  enclosing block scope of the same frame, or in the module scope when the
  current scope is the module scope or a block of it — the statement
  **assigns** it (a typed var checks the value against its type:
  `type_error`). Otherwise it **declares** a var in the current scope,
  subject to the same `redefinition` rule as `def` (a `def` and a `var` of
  one name in one scope collide).
- A var declared outside the current frame is readable (#4a) and never
  assignable (#4): `var n …` inside a fn body where the only visible `n`
  is a module var is **`var_error`** ("cannot assign a var of an enclosing
  frame"); so is a block of a fn assigning the module's var.
- NAME is a lowercase name (capitalised names are types). The constructor
  keyword forms — `var NAME fn …`, `var NAME class …`, every form `def`
  mirrors — and a type literal (`var NAME Integer`) are refused
  (`var_error`, "a var holds a value: use def for a fn / a type"); a
  function value that reaches the statement as a VALUE — `var f (mk 10)`,
  and so `var f (x:Integer => […])`, since `=>` folds to a paren group — is
  stored and dispatches when read, as any fn value does (#8; the refusal is
  syntactic, #9, and a paren is a value).
- Reading a var pushes its current value. A fn value created while a
  fn-local var is live captures the var's **value at creation** (#6), as
  `ComputeCaptures` snapshots every enclosing-fn local today; the closure
  cannot assign it (#4).
- `export "M" {n: n}` where `n` is a var fails with `export_error` ("a
  module exports no var") (#7).
- The old `var [[names] body]` construct is removed; its one role — binding
  stack values to untyped names in a callback — is taken by **untyped
  lambda parameters**: `[e acc] => […]` binds `e` and `acc` as `Any` (#5).
  A bare word in a lambda's parameter list is a name, not a type.

### 2.4 Loop state

The loop counter and accumulator idioms spell as

```
var n 0  while [n lt 3] [var n (n add 1)]  n        => 3
var t 0  for 5 [var t (t add i)]  t                 => 10
```

`fold`, `for`'s result values and tail recursion stay the idiomatic forms
for accumulation; `var` is for state that is genuinely mutable. The
compiler's loop-carried `def` lowering (`carriedInit`/`emitStore`) is the
same shape and is retargeted to `var`.

### 2.5 `undef`

Removed from the language: `undef` becomes an undefined word. Block locals
end with their block, frame locals with their frame, module names are
immutable, and the REPL allows redefinition (#10), so nothing a program
can write needs it. The engine's own uses move to internal words users
cannot spell: the frame tail's param pops (`AppendFrameTail` emits
`undef/f <param>` pairs) become `__ud <param>`; `__varundef` goes with the
construct; `UndefFnHandler` (remove an overload by signature) goes.

### 2.6 `import`, modules

Importing the **same** module twice under one name is idempotent;
importing a **different** module under a name already bound is
`redefinition`. A module body is a module scope: its names bind once;
its vars are its own (#7). `unpack` binds like `def`.

### 2.7 The REPL

Every line runs against one persistent registry. A line that rebinds a
name is accepted and the REPL prints `redefined x` (`Registry.
AllowRedefinition`, set only by the REPL); overlapping overloads replace
there, as today. Scripts, modules and `boru -e` keep the error.

### 2.8 Errors

| code | when | the message carries |
|---|---|---|
| `redefinition` | a second binding of a name in one scope; a fn def over an enclosing scope's fn; an overlapping overload; a different module under a bound name | both sites (the standing binding's and the new one's), the idiom class and its rewrite: *loop counter → `var`*, *sequential rebinding → `var` or a second name*, *conditional binding → `def x (if c [a] [b])`*, *overlapping overload → one `fn` with both triples*, *extension from an inner scope → move the overload to the owning scope* |
| `var_error` | assignment across a fn boundary; a capitalised name; the fn/type spellings; `var` of a name bound by `def` in the same scope | the var's declaration site |
| `export_error` | exporting a var | the var's declaration site |
| `type_error` | a typed var assigned a value of another type | the declared type, the value |
| `shadow_rebind` (warning) | a block-local `def NAME v` whose `v` reads NAME — the counter/accumulator shape, now a shadow that answers differently | the shadowed binding's site; the rewrite (`var`, or a new name) |

The check pass reports every statically visible case (the static census
is the default `boru check`); `installDef` and the var handler raise the
same codes at run time for the shapes only a run can see (a computed body
`do (quote [def x …])`, a macro expansion, `unpack` of a run-time map).
`shadow_rebind` is a check warning only — the run is legal.

### 2.9 Examples

| program | today | under the rule |
|---|---|---|
| `def a 1 def a 2 a` | `2` | `redefinition` at `def a 2`, pointing at `def a 1` |
| `def n 0 for 3 [def n (n add 1)] n` | `3` | `0` — the body's `def n` is a block-local shadow (#1), and `boru check` warns `shadow_rebind` (#13); the intended program is `var n 0 for 3 [var n (n add 1)] n` → `3` |
| `def t 0 each [def t (t add 1) t] [1 2 3]` | `[1 2 3]`, `t` is `3` after | `[1 1 1]`, `t` is `0`, with the same warning; `var t 0 each [var t (t add 1) t] [1 2 3]` → `[1 2 3]`, `t` is `3` |
| `if true [def w 1] [def w 2] w` | `1` | `undefined word: w` (the arm's `w` ended with the arm); write `def w (if true [1] [2])` |
| `do [def q 5] q` | `5` | `5` (#11) |
| `for 2 [def x 9] x` | `9` | `undefined word: x` |
| `def f fn [[x:Integer] [Integer] [1]] def f fn [[x:String] [Integer] [2]] (f 1) (f "s")` | `1 2` | `1 2` — disjoint overloads add |
| `def f fn [[x:Integer] [Integer] [1]] def f fn [[x:Integer] [Integer] [2]] f 1` | `2` | `redefinition` (overlapping overload) |
| `def g fn [[] [Integer] [def f fn [[x:String] [Integer] [2]] 0]]` with a module `f` | adds `f`'s overload past the call (NUR149's family) | `redefinition` (an inner scope cannot extend `f`) |
| `def r 1 def h fn [[] [Integer] [def r 2 r]] (h) r` | `2 1` | `2 1` — a value shadows |
| `def x 1 def g fn [[] [Integer] [x]] def x 2 (g)` | `2` (NUR097) | `redefinition` |
| `var n 0 def inc fn [[] [Integer] [var n (n add 1)]] (inc)` | — | `var_error`: a fn body cannot assign a module var |
| `var n 0 def peek fn [[] [Integer] [n]] var n 5 (peek)` | — | `5` — a module var is read live |
| `def Foo Integer def Foo String 1 is Foo` | `false` | `redefinition` |
| `each ([e acc] => [acc add e]) …` | `unknown type "e"` | `e`, `acc` are `Any` params |
| `fold [var [[e acc] acc set (e.id) e]] xs {}` | works | the construct is gone: `fold ([e acc] => [acc set (e.id) e]) xs {}` |
| `def a 1 undef a` | binding popped | `undefined word: undef` |

## 3 Mechanism

### 3.1 Interpreter (`core/go`)

- **Scope records.** The registry gets a scope stack: `{id, kind
  (module/frame/block), frame id, snapshot}`; `DefEntry` records the scope
  id that bound it. `installDef` compares the standing top entry's scope
  with the current one: same scope → `redefinition`; enclosing scope →
  push (a shadow), except a fn def over a fn-bound name (#2). The fn
  frame's `FnBaselines`/`TopFnBaseline` and `DefCleanupInfo.Snapshot` are
  this mechanism for frames already; blocks reuse them. The runtime check
  costs one map read per `def`.
- **Block entry and exit.** `InvokeBody` pushes a block scope before
  `RunResolved` and truncates every name to its snapshot after — one seam
  for every callback body. `stepMoveIf` splices `branch… + BlockEnd` where
  `BlockEnd` is the existing `DefCleanup` marker with a block snapshot,
  spliced **only when the arm's tokens contain a `def` or `var`** (a
  shallow scan, memoised per body list as `bodyNeedsFrameState` is for
  frames) — an arm without bindings costs nothing. Loop regions truncate
  at collection (`collectLoopRegion`), generalising `ForCont.IterDepth`
  (NUR204) from the iterator to every name the body bound. `do` keeps
  `BodyOnceKeepsDefs` (#11).
- **TCO.** The tail probe's pattern `)* __DC __pa (__ud name)* [__RC] )`
  learns to skip an arm's `BlockEnd` marker; with no marker on arms that
  bind nothing, `if (n lte 0) [acc] [s2 (n sub 1) …]` is unchanged, and an
  arm that binds a local before its tail call still elides (the frame
  snapshot already covers the arm's bindings). Checked by
  `Registry.TCO.{Detected, Elided, Replaced}` on the recursion rows.
- **`var`.** A `DefEntry` of kind `Var` whose value is mutable in place;
  assignment resolves the nearest visible var entry, refuses one whose
  frame is not the current frame (#4), checks the declared type, writes
  the value and bumps `Defs.Gen(name)` (the dispatch cache and the
  in-place prototype's `ForwardInfo.Gen` guard read it). Reads are the
  binding reads of today. `ComputeCaptures` already snapshots by value.
- **`undef`.** `AppendFrameTail` emits `__ud`; `undef`, `__varundef` and
  the var construct's `VarHandler` are deleted.
- **Import.** `import` compares the module ref under a bound namespace
  name: same ref → no-op, different → `redefinition`.

### 3.2 Check pass (`check/go`, `core/go` carriers)

- The pass already tracks arm, loop and fn-body depth (`CondBodyDepth`,
  `LoopBodyDepth`, `FnBodyDepth`, `FnBinders`) and models arm defs by
  snapshot-restore (`runCarrierBodyDefsAdds`); it gains the per-scope
  bound-name set that makes `redefinition` a static finding with both
  sites, and the var rules (§2.3) as static findings.
- `var` typing: a typed var's reads carry its declared type; an untyped
  var's reads are gradual (`Any`) unless every assignment in scope agrees
  on a type — a precision improvement for a later slice, not this one.
- Untyped lambda params: `ParseFnParams` accepts a bare word as a name
  with type `Any` (today: `unknown type "e"`).
- The speculative-fn-family machinery (`SpecFnNames`, `NoteSpecFnDef`,
  `SpecArmDepth`'s fn-def path, the three fn-redefinition refusals in
  `core_helpers.go`) models fn defs in arms and fn bodies extending or
  replacing enclosing words — shapes the rule makes errors. Deleted in
  Phase 4 once the corpus pins the errors.

### 3.3 Compiler (`compiler/go`, `eng/go`)

- A block-local `def` is a unit local scoped to its arm or body — the
  branch-promotion path (`planBranchPromotion`, the promoted value-def
  locals the var splice used) generalised; the twins that reproduce a
  leaking arm def (`BindDefReplace`, the arm-depth rollback) are no longer
  needed for arms and loop bodies. `do` keeps the kept-defs model
  (`kept_defs.go`, NUR210/NUR334) because it still leaks (#11).
- `var`: a fn-local var is a mutable slot; a module var is a live registry
  cell — reads and writes compile to the live-lookup ops a run-time bind's
  read already uses (NUR200's "live lookup of the registry's cell"), never
  baked into a unit. The loop-carried lowering (`carriedInit`/`emitStore`)
  becomes the var slot's.
- With module defs immutable, the freeze discipline (`rebind_notify.go`,
  `NoteFrozenRead`, `NotifyNameRebound`) has one remaining trigger, the
  REPL's redefinitions; Phase 4 either keeps it REPL-only or has the REPL
  drop its unit cache on a `redefined` line.
- The compiled↔interpreted differential (`TestSpecCompiledDifferential`)
  stays green at every phase: both lanes move together.

### 3.4 Language layer, CLI, REPL, docs

- `basic/go`: the `var` word (replacing `VarHandler`), `afn`/`fn` untyped
  params, `export`'s refusal, `undef` and `__varundef` removed, `__ud`
  added; `def`'s handler routes through the scope check.
- `boru check --def-census`: the static findings over a file set as a
  table — site, name, class, standing site, rewrite — exit 0; the plain
  `boru check` reports them as errors once Phase 3 lands.
- The REPL sets `AllowRedefinition` and prints the notice.
- Docs: REFERENCE §Definition and scoping and §Closures (the "redefinition
  reaches existing code, as at a REPL" paragraph becomes the var rule),
  LANGREF §Definition Words ("Definitions stack…" and the
  `def add5 add 5 end` examples, already broken by the strict barrier) and
  §Type Shadowing, TUTORIAL, CONTENT-ADDRESSING.0's rebinding discussion,
  FORWARD-COLLECTION-PHASES' `undef` mentions, CLI.md for the census.

## 4 Migration

### 4.1 The census (phase 0, measured 2026-10-06)

`boru check --def-census` (§3.4) classifies every binding and every read of
a block's leaked def by what the rule makes of it; `test/go/langspec`'s
`TestDefCensusCorpus` runs it over the whole spec corpus and ratchets the
count of rows with a finding the rule forbids (`defCensusRowCeiling`,
**151** of 8,684 rows; `BORU_LOG_DEF_CENSUS=1` lists them). The classes
are the rule's own cases (core/go/def_census.go):

| class | becomes | corpus rows | where |
|---|---|---:|---|
| `rebind` — a second def of a name in one scope (a module `def x … def x`, a fn-body local re-bound, a body def over a param, a `do` body's def over the enclosing scope's) | `redefinition` | 30 (33 sites, 11 files) | fn-locals-scope 12, edge-quote-4 6, def-node-binding 3, edge-modules-2 2, one each in generics, edge-quote-3, edge-modules-1, edge-fns-1, each-variants, code-bodies, bytecode-combinations |
| `overlap` — a `def f fn […]` overlapping an overload the scope already has | `redefinition` | 3 | compare-restrict L163, fn-value L333, open-words L77 |
| `extend-inner` — a fn body adding an overload to an enclosing scope's word | `redefinition` | 0 | — (NUR149's family is pinned in Go tests, not corpus rows) |
| `leak-read` — a read of a block's def after the block, nothing live under it | `undefined_word` | 11 | fn-locals-scope only (L167–168, L211, L245–251, L254) |
| `shadow-rebind` — a block binding a name an enclosing scope binds, having read it: the loop counter | legal; `shadow_rebind` warning; answers differently | 39 (43 sites) | code-bodies 14, fn-locals-scope 10, control 9, module-fnvalue-boundary 2, module-composition 2, callbacks 1, bytecode-migrated 1 |
| `undef` | removed | 59 (67 sites, 19 files) | open-words 8, edge-types-1 8, fn-value 7, class 6, module-fnvalue-boundary 5, fn-locals-scope 4, callbacks 4, fn-triple 3, module-time 2, module-parselang 2, def-node-binding 2, one each in 8 files |
| `var-construct` | `[e acc] => […]` | 9 (10 sites) | code-bodies 6, three others |
| `shadow` — any other inner-scope shadow | legal, unchanged | 17 rows (not counted) | fn-locals-scope |

Fourteen rows add disjoint overloads (`def f fn … def f fn …`); they are no
finding and stay legal. A repeat `import` of one native module is a cache
no-op today (edge-modules-1 L64, L98) and stays one; a second module bound
to one namespace is a `rebind` of kind `module`.

The tree, by the same census over every `.boru` file (a finding inside an
imported module's fn is reported under the importer with the module's
positions and `in <fn>`; counted once per site):

| where | findings |
|---|---|
| `kg/*.boru`, `kg/tests` | **119** `var-construct` sites the pass runs (187 constructs by text — the pass does not run `filter`'s body, so a `var` inside one is not seen); nothing else |
| `utils/*.boru` | 1 `shadow` (`seq.boru` 448:9, `c` in `seq-main`) |
| `lang/go/modules/*.boru`, `design/examples/**` | nothing (the sift module's per-fn local `loop` helpers are each fn's own: a caller's binding is dynamically visible but encloses nothing, §2.1) |
| `bench/interp/fixtures` | 2 `shadow-rebind` (`loopsum.boru`, `nestloop.boru`: `def total 0 for … [def total …]`) |
| `editors/linguist/samples`, `lang/go/test/check_fixtures`, `testdata` | 2 `shadow` |

So the tree's migration is the kg `var` rewrite (phase 1) and two bench
fixtures; the corpus's is 151 rows across the classes above, 59 of them
`undef` demonstrations.

### 4.2 The earlier estimate

Counted by hand in IMMUTABLE-DEF.0 §3 and re-counted on `d56a59922`,
kept for the record; the census above supersedes it where they differ
(the hand count missed fn-body `undef`s and the `each`-callback
accumulators, and counted the `var` construct by its one-line spelling):

| what breaks | corpus rows | tree sites | rewrite |
|---|---:|---:|---|
| loop counter / accumulator by rebinding (`def n 0 while … [def n …]`, `def t 0 for … [def t …]`) — now a legal shadow that answers differently | ~26 (census: 39) | 2 (`bench/interp/fixtures/loopsum.boru`, `nestloop.boru`) | `var`; the `shadow_rebind` warning (#13) finds every site |
| sequential rebinding at module level (`def x … def x …`) | ~45 sites in 64 rows (census: 30 rows, every scope) | 10 in 7 files (3 are demos; census: 0 — those were `var` constructs and callers' locals) | `var`, or a second name |
| sequential rebinding inside a fn body | 34 | 4 | `var`, or a second name |
| a def inside an `if`/`case` arm or loop body read after it | 2 (census: 11) | ≤2 (census: 0) | `def x (if c [a] [b])`; `fold` |
| late-binding pins (`def c 1 def xs [c add 1] def c 10 xs`) | ~13 | 0 | become `redefinition` rows |
| a second `def f fn […]` adding disjoint overloads | 14 | 0 | **unchanged** — additive overloads stay legal |
| a second `def f fn […]` replacing an overload; a closure rebound | 3 | 0 | become `redefinition` rows |
| `undef` | 34 rows in 11 files (census: 59 in 19) | 0 (two comments) | deleted, or rewritten as the scope they demonstrate |
| the `var [[…]]` construct | 9 | 69 (63 in `kg/*.boru`, 6 in `kg/tests`; by text 187) | `[e acc] => […]` |
| double `import` | 3 | 0 | idempotent import (already a cache no-op for a native module) |
| a type def'd per element in an `each` body | 3 | 0 | legal (block-local) or hoisted |

The nine downstream voxgig-boru libraries (not in this tree) get the same
census before they take the release.

## 5 Phases and gates

Each phase is a commit series that keeps `make commit-gate`, the full
langspec gates and `TestSpecCompiledDifferential` green; a phase whose
semantics change what the corpus pins carries the corpus rewrite in the
same series.

| phase | deliverable | gate | size |
|---|---|---|---|
| **0 — census and the code** (done 2026-10-06) | scope ids on the def table (module / frame / block, lexical enclosure); the census classes in the check pass (report-only, §4.1); `boru check --def-census` and `CheckResult.DefCensus`; `TestDefCensusCorpus`, a downward ratchet on the corpus rows the rule will fail. `var_error`/`export_error` exist; `redefinition` and `shadow_rebind` are minted with their first raising sites in phase 3 — the code gate (`TestEveryRegisteredCodeIsMinted`) refuses a registered code no site raises | the census over lang/spec, kg, utils, examples agrees with §4 within the counted shapes (it does: §4.1); coverage 100 % | 2–3 days |
| **1 — constructs** | untyped lambda params; the `var` word on both lanes (interpreter cell + compiler slot/live cell, typed and untyped, captures, export refusal); kg's 69 sites and the 9 corpus rows rewritten; the `var [[…]]` construct, `__varundef` and `VarHandler` deleted | kg's suites and `make -C kg graph` byte-identical before and after; the var rows of §2.9 pinned both lanes | 4–6 days |
| **1, shipped 2026-10-06** | untyped lambda params (`[e acc] => […]`, a bare lowercase word is a param of type `Any`; a capitalised one still names a type); the `var` word in its quoted-name form (`var n 0`) on both lanes — declaration, in-place assignment, the frame rule, closures, typed checks on concrete values, `export` refusal, the keyword-form refusal, loop-carried and branch-joined cells (the check model treats a var as one cell: `CheckState.VarAssigned`, the joins replace), the compiler's `ASSIGN_DYN_SCOPE` op and the replace-kind resident bind; `lang/spec/var.tsv`. **Deferred to the construct's removal**: the typed-name and string-name forms (`var n:Integer 0`, `var "n" 0`) — registered beside the construct's one-list signature they make `var [[a b] …]`'s forward/stack split depend on gradual operands (ten kg programs declined), so they wait with it; **the callback protocol** (ruled 2026-10-06, shipped the same day): a Function form hands the container's natural unit — the ELEMENT of a list, the VALUE of a map entry — and the whole entry as a `KeyVal {k v i n}` only when the callback types its entry param `KeyVal` (`core.CallbackWantsKeyVal`: the last param, so `fold`/`scan`'s `[acc kv]` follow the same rule); `filter`'s `{key value}` pair over a list is gone, and a non-Boolean predicate result or a scalar receiver is its loud `filter_error`. Both runtime arms (`native/filter.go`, `native_map_iter.go`) bridge every contract-bearing compiled closure to its signature (`ClosureAsFnDef`) and match it themselves, so the unit's `KeyVal` input shape, `ClosureWantsKeyVal` and the VM's map-arm no-match went with the pair; the compiler's `entryCarrier` follows the same rule. §4.1's `filter` sites in kg are now a mechanical rewrite. | the var rows of var.tsv on both lanes; core at 100 % | — |
| **1, kg rewritten 2026-10-07** | kg's 187 `var [[…]]` constructs rewritten as lambdas (156 in the pipeline, 31 in its suites; the five quotation-only slots — check-prop bodies, `error` handlers — as `[([names] => […]) apply]`); `make -C kg graph` byte-identical, `make -C kg check test` green. The kg suites run through `boru test` (the run lane anchors a script's imports at its own directory since NUR083, which the `boru tests/x_test.boru` recipe no longer satisfied — every suite failed its imports). The rewrite surfaced four lane defects, each fixed and pinned on both lanes: a CAPTURING lambda literal in a dyn-body callback slot declined ("dynamic input at each"; now resolved as the fn value it is, count-agnostic at the unit so the handler's seam owns the count — `recordDynBodyCall`, and the compile-defect ledger's two walk declines went with it, 336 → 334); a count-ambiguous poly over a gradual accumulator bailed at run time (`vm:poly-nout-drift`; `check.PolyNOutAmbiguous` marks the event and the op commits no count); the fold/scan fixed point had no answer for a Function body and typed the result a strict Any that failed every later read (`analyseCallbackFn`: positional over a map, stack order over a list; a fn-valued input or an unanalysable body answers a gradual Any); and a run-time-stamped token body was hosted on the RUNNING registry where a module fn's unit had installed the names it reads on the MODULE's (`undefined word: ev` at kg's check-code-units; `hostForeignOn`: the body's home is the calling registry, its live `args` read bridged from the running one). `KeyValCarrier` moved to core. **Still open in this phase**: the construct's deletion (`VarHandler`, `__varundef`, the `[TList]` signature, `CensusVarConstruct`) and the typed- and string-name `var` forms that wait on it. | kg suites and graph; the lang/eng/check pins named above | — |
| **1, construct removed 2026-10-07** | the `var [[…]]` construct is gone: `VarHandler`, `__varundef`, the `[TList]` signature, `CensusVarConstruct`, `RecordDynUndef` and the compiler's construct arms deleted; the `var` word's typed-name and string-name forms (`var n:Integer 0`, `var "n" 0`) registered in its place (`varWordSignatures`; var.tsv §4 and lang's `native_var_test.go` pin them on both lanes); `help var` rewritten and the examples regenerated. The corpus followed: every construct row in lang/spec (code-bodies, corpus-structures, edge-errors-1, bytecode-migrated, the frontier files), the sweep seeds, the lang/eng/check/compiler/basic Go tests (146 sites by script, the rest by hand — a stack-form site becomes `vals ([names] => […]) apply`, an inline-value site `(([names] => […]) v…)`, a quotation-only slot `[([names] => […]) apply]`, an empty declaration list the inlined body, a body that must leave one value gains its `drop`), REFERENCE, TUTORIAL §14, HOWTO, LANGREF and COMPILABLE-SUBSET; NUR.md's dated repros keep their historical spelling. Two stale filter pair-protocol rows (lang/go/test syntax.tsv, `stamp_filterlambda_test.go`) fixed. The rewrite surfaced one more lane defect, fixed and pinned: the no-match recovery scored its candidate overloads in TAPE order against SIGNATURE-order slots (check `checkModeAssumeSig`), so `doc get (k)` over a lambda's strict-Any param assumed get's None-receiver overload and a user poly committed its Any arm on the None carrier ('leaf' for 'map'; `TestUserPolyAnyWrapperDispatch`) — it scores in signature order now (`core.SigOrderPositions`), and the one row that compiled only through the mis-scoring (`do (mk) end x/v apply`, NUR384) is counted ledger debt (`compileDefectCeiling` 334 → 335; 337 with NUR386's two rows). The frame rule is one rule (ruled 2026-10-07): a closure's `var s …` over an ENCLOSING FN's var is the var_error a fn body meets over a module var — the capture carries the cell's kind (`core.CapturedBinding.Var`, installed Frame-and-Var by `InstallCapturedBinding`, which `bindVar` refuses); a captured var is read as the value it held at the closure's construction, and a captured def is shadowed by the closure's own var (var.tsv §3; a plain capture had let the statement declare or assign a copy silently). A named fn's refusal is the check pass's stop, not yet the run's trap (NUR386). Phase 1 is complete. | the var rows of var.tsv; the rewritten corpus rows on both lanes; kg suites and graph byte-identical | — |
| **2 — block scopes** | scope records; `InvokeBody` and arm/loop block entry/exit on the interpreter; arm/body locals as scoped unit locals on the compiler; the TCO probe through `BlockEnd`; NUR204 generalised | the 2 leak rows and the arm/loop examples of §2.9 pinned; TCO counters equal on the recursion rows; differential green | 5–8 days (the compiler half is most of it) |
| **3 — immutability** | `installDef` refuses same-scope rebinding and inner-scope extension; additive overloads with overlap → error; types; idempotent import; `undef` removed, `__ud`; the REPL notice; the corpus rewrite of §4 with the late-binding and replacement rows as negative tests; the bench fixtures | `boru check` reports the errors; every census class has a pinned negative row; all gates green | 3–5 days |
| **4 — retire and document** | the speculative-fn-family machinery, the arm-leak twins and the fn-redefinition refusals deleted; the freeze discipline REPL-only; NUR097, NUR149, NUR204, NUR367 resolved and the records deleted; REFERENCE/LANGREF/TUTORIAL/CLI rewritten; the kg bundle rebuilt | coverage 100 % after the deletions; `make ci-local` | 2–4 days |

Phases 1 and 2 are independent of each other and could run in either
order; 3 needs both (it removes the only spelling of loop state otherwise).

## 6 Risks and the check for each

- **The compiler's binding model is a rewrite, not a deletion.** The bind
  ledger, twins and kept-defs exist to reproduce leak and rebind semantics
  compiled; arm and loop locals become slots, `do` keeps its latch. Check:
  the differential at every phase, and the compile-failure ledger
  (`compile_failures.tsv`) must not grow.
- **`do` stays transparent (#11) while every other body is a block**, so
  `for 3 [do [def t …]]` binds `t` in the loop body's scope per iteration
  (fine) but `do [def x 1] do [def x 2]` at module level is `redefinition`
  — the kept-defs latch for a computed `do` body (`do (mk)`) stays a
  compile decline (NUR210/NUR334). Check: the `do` rows of
  `code-bodies.tsv` on both lanes.
- **TCO through `BlockEnd`.** Covered by the recursion rows' counters and
  a new row with a local bound before the tail call.
- **Shadowing a word with a value (#2a)** means dispatch inside that scope
  resolves to a value: `def f 5 … f` pushes 5 where the module `f` would
  have dispatched. Legal by ruling; the check pass's dispatch model
  already resolves names by the def stack, so no new machinery, but the
  census should report it as a *shadow* so authors see it.
- **The silent shape (#13).** `def n 0 for 3 [def n (n add 1)] n` runs and
  answers `0`; only `boru check` says why. `boru run` pre-flights the check
  pass by default, so the warning reaches the terminal, but a run under
  `--no-check` sees nothing. Check: the census lists every site in the
  corpus and the tree before Phase 3 lands; the warning has a pinned row.
- **Captured vars are snapshots (#6)** — a closure made in a loop body
  sees the var as it was; authors expecting shared state will be
  surprised once. The REFERENCE paragraph states it with an example.
- **Export refusal (#7)** needs the export walk to see a var's binding
  kind, not its value (the map literal has already evaluated it): `export`
  consults `Defs` for each exported name.
- **Per-`def` cost.** One scope comparison per install on the interpreter;
  `BenchmarkStage6`'s interpreter rows and the in-place prototype's
  numbers are the regression check.
- **The in-place prototype** (`design/IN-PLACE-COMPILATION.0.md`) guards a
  completion on `Defs.Gen`; var assignment bumps it, so the guard stays
  honest. Its verify lane runs over the rewritten corpus once.
- **Downstream.** The voxgig-boru suites run the census before the
  release; their migration is theirs.

## 7 Register

Resolved by the rule (deleted with the resolving commits in Phase 4):
NUR097 (live module names — `def n 2` after a closure read it is now
`redefinition`), NUR149 (the family-L leak — an inner scope cannot extend
a word), NUR204 (the iterator's asymmetry — every loop-body binding is
iteration-scoped), NUR367 (a loop-rebound local — `redefinition`). Kept:
NUR210 and NUR334 (`do`'s kept defs, #11). New non-uniformities the rule
introduces, recorded Pending as they land: `do` as the one transparent
body word; a value may shadow a word where a fn may not extend it.
