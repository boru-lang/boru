# Immutable `def` — what breaks, and how to resolve it

**Status:** investigation, 2026-10-06, measured on `8f8084544`. Asked for by
the maintainer: *make `def` an immutable construct, so that re-`def`ing a
name in the same module is a semantic error — `def a 1 def a 2` fails —
and suggest resolutions for the breaking impacts.* Nothing here is built;
the census instrument that produced the numbers was a temporary hook on
the engine's binding install and is not committed.

## 1 What `def` means today

Bindings stack. `def NAME body` pushes onto the name's stack in the
single binding store (`core/go/deftable.go`), `undef` pops, and the top is
what a read resolves to. The language reference documents this as a
feature twice: *"Definitions stack: a second `def` for the same name
shadows the previous one"* (`design/LANGREF.10.md` §Definition Words) and
*"redefinition reaches existing code, as at a REPL"* — a module-scope name
read inside a fn body is **live**, so `def n 1 def f fn [[] [Integer] [n]]
def n 2 f` is `2` (`REFERENCE.md` §Closures; recorded as NUR097). The
engine's notion of **scope** is the *frame*: a fn body (and a lambda, a
`var` block, a module fn body run through `CallBoru`) snapshots the depths
at entry and pops back to them at exit. Every other code body is part of
its enclosing frame: a `def` inside a `do`, an `if`/`case` arm, a `for`,
`while`, `each` or `fold` body **leaks** into the frame and **stacks** one
binding per execution. The one exception is the loop's own iterator name,
which NUR204 made iteration-scoped. Measured on the interpreter lane:

| program                                                                     | result      | what it shows                                           |
|-----------------------------------------------------------------------------|-------------|---------------------------------------------------------|
| `def a 1 def a 2 a undef a a`                                               | `2 1`       | the stacked shadow the proposal forbids                  |
| `def a 1 for 3 [def a (a add 1)] a undef a a undef a a undef a a`           | `4 3 2 1`   | a loop body's def leaks and stacks once per iteration    |
| `def n 0 while [n lt 3] [def n (n add 1)] n undef n n undef n n`            | `3 2 1`     | the `while` counter is written this way, and only this way |
| `def t 0 each [def t (t add 1) t] [1 2 3] drop t undef t t`                 | `3 2`       | callback bodies leak too                                 |
| `def a 1 if true [def a 5] [0] a undef a a`                                 | `5 1`       | an arm's def leaks                                       |
| `def a 1 do [def a 5] undef a a`                                            | `1`         | `do` leaks (BodyOnceKeepsDefs)                           |
| `def i 0 for 3 [def i 9] i`                                                 | `0`         | the iterator name alone is iteration-scoped (NUR204)     |
| `def t 7 def g fn [[] [Integer] [def t 1 for 2 [def t (t add 1)] t]] (g) t` | `3 7`       | a fn frame scopes its locals; the loop leaks into the frame |
| `def a 1 5 var [[a] a mul 2] a`                                             | `10 1`      | `var` is a scoped binder (a def/undef splice today)      |
| `def f fn [[x:Integer] …] def f fn [[x:String] …] (f 1) (f "s")`            | `2 s`       | a second `def` of a fn with disjoint signatures **adds an overload** |
| `def f fn [[x:Integer] … 1] def f fn [[x:Integer] … 2] f 1`                 | `3`         | overlapping signatures **replace**                       |
| `def x 1 def g fn [[] [Integer] [x]] def x 2 (g)`                           | `2`         | module names are live (NUR097)                           |

So "re-`def` a name in the same module" has two readings, and they differ
by an order of magnitude in impact (§3):

- **the literal rule**: a second `def` of a name whose binding from the
  *same frame* is still live is an error — which, under today's leak
  semantics, also makes every `def` inside a loop body an error on its
  second iteration;
- **the lexical rule**: every code body is a scope, and a name binds once
  per scope.

## 2 How it was measured

Three instruments, each with its blind spot stated:

1. **Dynamic census, spec corpus.** A hook on `installDef` recorded every
   install of a name that already had a binding — its depth, the frame
   baseline's depth for the name (0 at module scope), the old and new
   binding kinds, and whether the fn signatures overlap. All 8,684 rows of
   `lang/spec/*.tsv` were run interpreted on a fresh instance each. This
   sees exactly what the engine sees.
2. **Static census, the tree's boru code.** A tokenizer over the 129
   `.boru` files (3,352 `def`s) classified each `def` by lexical scope —
   frame (module top level, fn body, lambda, `Test.test` body, `var`
   block) and the blocks inside it (loop/callback body, `if`/`case`/`do`
   arm) — and counted names bound more than once per scope. This sees what
   a *lexical* rule would see; its fn-body detection covers the list form
   `fn [[in] [out] [body]]`, the triple form and `=>` lambdas.
3. **Dynamic census, real runs.** The same hook in the CLI, running the
   knowledge-graph build (`make -C kg graph`) and the utils test suite.
   This sees the engine's frame rule applied to real code, which is the
   gap between readings 1 and 2 made visible.

Not measured: the nine downstream voxgig-boru libraries (not in this tree;
`design/VOXGIG-BORU-HANDOVER.0.md` has the probe build that could carry
the same hook), and the REPL.

## 3 Impact

### 3.1 The spec corpus (dynamic, interpreter lane)

117 of 8,684 rows (1.3%) bind a name that is already bound. By the
engine's own scope:

| class                                                   | rows | sites | kinds (sites)                                                        |
|---------------------------------------------------------|-----:|------:|----------------------------------------------------------------------|
| module-scope redefinition (the literal rule)            |   64 |    70 | value→value 49, fn+fn disjoint (overload added) 14, fn+fn overlapping 3, type→type 4 |
| same-frame redefinition inside a fn body                |   34 |    39 | value→value 39                                                       |
| a fn body's def shadowing an enclosing name (lexical)   |   14 |    17 | allowed under both readings                                          |
| the loop iterator shadowing an outer def                |    6 |    11 | allowed under both readings                                          |

Of the 70 module-scope sites, 45 fire once (straight-line `def x … def x`)
and 25 fire on every iteration of a loop body. By idiom, the 64 rows are:

| idiom                                                                                   | rows | where                                                      |
|-----------------------------------------------------------------------------------------|-----:|------------------------------------------------------------|
| a loop counter or accumulator rebound in the body (`def n 0 while [n lt 3] [def n (n add 1)]`, `def t 0 for 3 [def t (t add 1)]`) | ~26 | control, code-bodies, callbacks, bytecode-migrated, module-composition, fn-locals-scope |
| pins of the late-binding semantics (`def c 1 def xs [c add 1] def c 10 xs`)             | ~13 | def-node-binding, edge-quote-4, each-variants, edge-fns-1, code-bodies |
| a second `def NAME fn […]` adding an overload (`def tag fn [[m:FlexMap]…] def tag fn [[m:S]…]`) | 14 | as (8), fn-value (4), open-words (2)                       |
| a second `def NAME fn […]` replacing an overload, or a closure rebound                   |    3 | open-words, compare-restrict, fn-value                     |
| the same namespace imported twice                                                       |    3 | edge-modules-1, edge-modules-2                             |
| a type `def`'d inside an `each` body, once per element                                  |    3 | user-types                                                 |

`while` appears in 21 spec rows and in **no** real program in the tree:
the counter-by-redefinition idiom is a corpus idiom, not a codebase one.

### 3.2 The tree's boru code (static, lexical)

129 files, 3,352 `def`s. Under the lexical reading:

| class                                                                        | sites | where                                                                                   |
|------------------------------------------------------------------------------|------:|-----------------------------------------------------------------------------------------|
| module-level sequential redefinition (`def x … def x` at top level)          |     4 | `lang/go/modules/cli.boru` (`r`, lines 791 and 836), `design/examples/entity/usage.boru` (`u` ×4, `o` ×2 — a demo), `scripts/hash-identity-probe.boru` (`dbl` — deliberately shows rebinding) |
| module-level accumulator rebound in a loop body                              |     2 | `bench/interp/fixtures/loopsum.boru`, `nestloop.boru` (`def total 0  for N [def total (total add i)]`) |
| fn-body sequential redefinition                                              |     4 | `vault_tui.boru` (`fresh` ×3, `cols` ×2), `todo-tui-client.boru` (`st2` ×2), `sift.boru` (`cols` rebound in an arm) |
| a fresh name `def`'d inside an `if`/`case`/`do` arm                          |   721 | everywhere; at most 29 are read after the arm, and the sampled ones (`[ def r (cli-add st nm val) r ]`) read it inside the arm |
| a fresh name `def`'d inside a loop or callback body                          |    25 | utils 33 by raw count, kg 5, examples 2; at most 2 read after the loop                  |
| bound by `var`                                                               |   118 | kg (`fold [var [[e acc] …]]`)                                                            |
| single binding                                                               | ~2,450 | the rest                                                                               |

The literal `def a 1 def a 2` shape is **ten sites in seven files**, three
of them demonstrations. Real code binds once and uses `var`, `fold`,
FlexMap/FlexList state and class fields where it needs accumulation.

### 3.3 Real runs (dynamic, the engine's frame rule)

The same code seen by the engine looks very different: the knowledge-graph
build installed an already-bound name **816,990 times at 256 sites**; the
utils suite 17,939 times at 248 sites. By the engine's classification the
kg build has 165 "module-scope" sites — but they are `var`-bound names and
callback-body bindings in nested `fold`s (depth 1→3), i.e. lexical
shadowing inside bodies the engine does not treat as scopes — 76
same-frame sites, almost all a loop-body local inside a fn re-pushed per
iteration (`def t (strip-comment …)` in `gomod.boru`), and 15 shadows.

**This gap — about 10 sites lexically, about 400 by the engine's scope —
is the decision the rule needs** (§5 D1). A runtime check wired into
today's binding store would trip on `var` bodies and on every loop-body
local; a lexical check would trip on almost nothing in real code and on
about a hundred corpus rows.

## 4 What exists only because names can be rebound

An immutable module scope is not only a language-design question; it is
where a large share of the compiler's and checker's difficulty lives. The
inventory, by `grep` over `core/go`, `compiler/go`, `check/go`, `eng/go`,
`basic/go`, `lang/go` (non-test):

| mechanism                                                                  | sites | note                                                                                     |
|----------------------------------------------------------------------------|------:|------------------------------------------------------------------------------------------|
| per-name binding **stacks** (`[]DefEntry`), `Depth`, `Truncate`, `Snapshot`/`Restore` | 35+ | the data structure itself                                                       |
| per-name generation `Defs.Gen` and the dispatch cache keyed on it          | 42    | invalidation exists for rebinds; with none, the cache needs no key                       |
| `Mutations()` in the TCO eligibility gate                                  | 3     | declines eager teardown when arg evaluation rebound anything                            |
| `rebind_notify.go` and `NotifyNameRebound`                                 | 23 (+155 lines) | the freeze discipline: a module read baked into a unit is poisoned by a later rebind |
| `NoteFrozenRead`                                                           | 18    | the baked reads the discipline watches                                                   |
| `RecordDefRebind`, `BindDefReplace`, `NoteBindTransition` (bind twins)     | 54    | the twins exist so a rolled-back replay reproduces pushes, pops and replaces by depth    |
| `NoteRootDefSite`                                                          | 3     | NUR097's late-binding hint                                                               |
| compile refusals whose reason is a redefinition                            | 7     | `core_helpers.go` 199/201/225 (fn redefined in a conditional body, by a capturing value, of a speculative family); `emit.go` 5248/5290 (module binding rebound after a unit baked it), 7157/7162 (loop-carried def rebound) |
| the loop-carried def lowering (`carriedInit`, `emitStore`)                 | 13 mentions | the accumulator idiom modelled as frame slots                                        |
| InstallDef's overlap removal and the family-L leak (NUR149)                | —     | a fn redefinition inside a frame that the teardown cannot undo                           |
| open records that are rebinds                                              | NUR334, NUR367, NUR379 mention it; NUR097, NUR149, NUR204, NUR212 are cited in code |

With module names bound once, the freeze discipline becomes a theorem (a
baked read cannot go stale), the generation keys and the rebind
notifications become REPL-only paths, the three fn-redefinition refusals
disappear, and the two `emit.go` module-rebind refusals are unreachable.
Same-frame immutability (reading 1 inside fn bodies) also retires the
loop-carried lowering — but only once the loop-state constructs of §5 R1
exist, because today that lowering *is* how `def n 0 while … [def n …]`
compiles.

## 5 Decisions the rule needs

- **D1 — what a scope is.** Recommend the lexical reading with one
  refinement: a name binds **once per frame**, where a frame is the module
  top level, a fn or lambda body, a `Test.test` body and a `var` block;
  blocks inside a frame (`if`/`case`/`do` arms, loop and callback bodies)
  may bind **fresh** names, which are block-local — a loop body's binding
  lives for one iteration, generalising NUR204's iterator rule — and may
  **not** rebind a name their frame already binds. Shadowing an *enclosing
  frame's* name from an inner frame (a fn local over a module name) stays
  legal: it is lexical shadowing, not mutation, and 14 corpus rows plus
  most fn bodies rely on it. This makes `def acc 0 for 5 [def acc …]` a
  loud error, never a silent `0`, and leaves the 721 arm-local defs legal.
- **D2 — fn overloads.** A second `def f fn […]` with disjoint signatures
  is an *extension* today and a *rebinding* under the rule. Recommend an
  explicit word — `extend f fn […]` — and the multi-triple `fn [[sig₁]
  [r] [b₁] [sig₂] [r] [b₂]]` form for a word's own overloads; the
  open-words merge onto a core word (`def add fn […]`, `lang/spec/open-words.tsv`)
  moves to `extend` too, which makes the ownership-anchored admission
  visible at the site.
- **D3 — `undef`.** Keep it for the REPL and for frame-local cleanup (the
  engine's own frame tails use it); a module-level `undef` followed by a
  `def` is a rebind by another name, so under the rule it is one. 33 corpus
  rows use `undef`; no real program does.
- **D4 — the REPL.** Every REPL line runs against one persistent registry
  (`cmd/go/internal/repl`); redefinition across lines is its working mode.
  Recommend an interactive flag that allows redefinition and prints
  `redefined x`.
- **D5 — types.** A capitalised `def` binds a type and stacks the same way
  (`type Foo Integer  type Foo String  untype Foo`, LANGREF §Type
  Shadowing). Recommend the same rule; the shadow/`untype` example goes,
  and 3 corpus rows that `def` a type inside an `each` body hoist it.
- **D6 — import.** Importing the same module twice rebinds its namespace
  name (3 rows). Recommend idempotent import of the same module, an error
  for a different module under the same name.
- **D7 — `var`.** `var` splices `def`/`undef` tokens; under reading 1 its
  bindings inside nested callbacks are rebinds. It must become a real
  frame (a binder with its own scope), which is also what the compiler's
  "promoted value-def locals" already treat it as. Prerequisite.
- **D8 — `while`.** The only loop with no way to carry state except
  rebinding. Needs a state-threading form before the rule can land (R1).

## 6 Resolutions, by what breaks

| breaking idiom                                              | corpus rows | tree sites | resolution                                                                                                                                                                                                 |
|-------------------------------------------------------------|------------:|-----------:|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **R1** loop counter / accumulator by rebinding              |         ~26 |          2 | `fold` and `for`'s result values for accumulation (`fold [add] xs 0`, `iota n each […]`); a tail-recursive accumulator (REFERENCE's own recommendation); FlexMap/FlexList or a class field for genuinely mutable state (`b.sum x add "sum" set`); and a state form for `while`: `while init [cond] [body]`, the body taking the state and leaving the new one. The compiler's loop-carried slot lowering (`carriedInit`) is the same shape, so the construct has its lowering already. Not recommended: a `set` on a `def`'d name — it is rebinding under another spelling. |
| **R2** sequential rebinding of a local (`def r … def r …`)  |          34 |          4 | rename (`r1`, `r2`) or restructure; a checker fix-it names both sites                                                                                                                                       |
| **R3** conditional binding (`if c [def x 1] [def x 2] x`)   |           — |  ≤29 (sampled: arm-local) | legal under D1 only if the arms are blocks; the leak-dependent spelling becomes `def x (if c [1] [2])`, which reads better and compiles as a value                                                     |
| **R4** late-binding pins                                    |         ~13 |          0 | rewrite as negative rows expecting `redefinition`; delete the LANGREF/REFERENCE prose that documents stacking and live module names (§1), replacing it with the rule                                      |
| **R5** overload accumulation by a second `def … fn`         |          14 |          0 | multi-triple `fn`, or `extend` (D2)                                                                                                                                                                        |
| **R6** replacement / closure rebinding                      |           3 |          0 | rename                                                                                                                                                                                                     |
| **R7** double import                                        |           3 |          0 | idempotent import (D6)                                                                                                                                                                                     |
| **R8** type redefinition per element; `type … untype` shadowing | 3 + docs |          0 | hoist; drop the example (D5)                                                                                                                                                                               |
| **R9** `var`'s spliced defs                                 |           — |        118 | `var` as a frame (D7)                                                                                                                                                                                      |
| **R10** fresh names inside loop bodies                      |           — |         25 | block-local under D1; the ≤2 read-after-loop sites and corpus rows like `while … [def last i …] last` take `fold`                                                                                           |
| **R11** REPL                                                |           — |          — | interactive allowance with a notice (D4)                                                                                                                                                                   |

Migration mechanics:

1. **A census tool first.** The hook used here, productised as a check
   diagnostic (`redefinition`, error) that names both binding sites and
   the idiom class with its rewrite, shipped one release as a **warning**
   behind `boru check --strict-def` and `boru run --strict-def`, then on by
   default. The spec runner gains a lane that runs the corpus with the flag
   so the rewrite is gated.
2. **Constructs before rule**: `extend`, the `while` state form, `var` as a
   frame (D2, D7, D8). Each is independently useful and lands dark.
3. **Corpus rewrite**: about 98 rows (64 module-scope, 34 same-frame) in 25
   spec files, by the table above; the late-binding rows become the rule's
   negative tests.
4. **Docs**: LANGREF §Definition Words and §Type Shadowing, REFERENCE §Core
   bindings and §Closures, TUTORIAL, the module-cache and content-addressing
   notes that reason about rebinding (`design/CONTENT-ADDRESSING.0.md`
   §"whether `def` rebinds a name or mints"). The rule simplifies all of
   them.
5. **Downstream**: run the voxgig-boru suites through the census before the
   default flips; their migration PRs are the place to carry the rewrites.
6. **Register**: every record the rule moots (NUR097-class late binding,
   NUR149, NUR204's asymmetry, NUR367) is resolved with the rule as its
   fix, and the seven refusals in §4 are deleted with their tests turned
   into `redefinition` expectations.
7. **Then the compiler**: delete the freeze discipline and the generation
   keys outside the REPL path; retire the loop-carried lowering once the
   `while` state form carries the corpus.

## 7 Recommendation

Adopt the rule under D1's lexical reading — **one binding per name per
frame; blocks bind fresh names block-locally and never rebind their
frame's names; frames shadow enclosing frames lexically** — with D2–D8 as
stated. The breaking impact on code people actually run is ten sites in
seven files; the corpus pays about a hundred rows, most of them `while`
counters that no real program writes; and the compiler sheds the single
largest source of its rebind-related machinery and records. The cost that
is not optional is sequencing: `var` as a frame and a state-carrying
`while` have to exist before the error can be the default, or the rule
removes the only spelling of a loop with state.

## 8 Open for the maintainer

1. D1 — lexical frames with block-local fresh names (recommended), or the
   engine's frame rule as it stands (every loop-body `def` an error).
2. D2 — `extend` as a new word, or `def … fn` kept as an extension when
   signatures are disjoint (weaker: the binding still changes).
3. D8 — the `while` state form's spelling, or dropping `while` for `fold`
   and recursion.
4. Whether the first release ships the warning only, or the warning plus
   the constructs.

*Aside, found while measuring: `make -C kg test` fails on the clean tree at
`tests/codegraph_test.boru` with `undefined word: KgReport` — the test
imports `./util.boru` relative to `kg/` but `run` now resolves relative imports
against the script's own directory (NUR083), so the kg test suite has been
red since that change; unrelated to this note.*
