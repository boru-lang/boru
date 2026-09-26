# boru:scry — a boru system's knowledge of itself, as plain data

**Status: design proposal — first slice shipped (2026-09-26, NUR063).**
`lang/go/modules/scry.go` builds `boru:scry` with the seven words §6
adopts from `boru:debug` (`words`, `defs`, `modules`, `sig`, `body`,
`deps`, `shape`), from the one constructor both modules use; §6's verdict
is implemented. Everything else in §4 — the graphs, `types`, `info`,
`schema`, `trace` — is unbuilt. Companion note:
[BORU-VIZ.0.md](BORU-VIZ.0.md) specifies `boru:viz`, the diagram-source
generator that is this module's first consumer and defines the shared
data contract (its §3). The split is a maintainer decision (2026-08-12):
**the mechanism whereby a boru system gains knowledge of itself lives in
a separate module, `boru:scry`** — scry never draws, viz never
introspects, and the seam between them is ordinary boru values.

Per `lang/go/CLAUDE.md`, an introspection framework is not a `-util`
word library, so the id stays plain: **`boru:scry`, namespace `Scry`**.

*Scry*: to see distant or hidden things. The name is the job: the
running system, made visible — as data on the stack, never as prose.

## 1. Purpose

boru already tells users about itself in *prose*: `boru describe`, the
`describe`/`help` words, `Debug.explain`. Prose is for reading; it
cannot be filtered, folded, asserted on, or drawn. `boru:scry` is the
structured twin: every fact the system knows about itself — its
modules, words, signatures, bodies, reference edges, types, value
shapes, and execution steps — available as Lists and Maps from inside
the language.

Consumers, in intended order of arrival:

- **`boru:viz`** — draws scry output as diagrams
  ([BORU-VIZ.0.md](BORU-VIZ.0.md) §6).
- **Tests** — architectural assertions ("no cycles in the word graph",
  "module X never references module Y") become one-line boru:test
  cases over `Scry.word-graph` + `Viz.cycles`.
- **Documentation tooling** — generated indexes and inventories that
  cannot drift, because they read the live registry (the same property
  `boru describe` already has, now composable).
- **Agents and external tools** — today they parse `describe` text;
  scry gives them data.

## 2. What exists scattered, and why a module

The raw material is already in the tree; scry is a *curation and
packaging* layer over existing seams, adding as few new ones as
possible (the `boru:debug` design principle, `DEBUG-MODULE.0.md` §2.4).

| Self-knowledge | Where it lives today | Gap scry closes |
|---|---|---|
| Dispatchable words, defs, importable modules | `Debug.words` / `Debug.defs` / `Debug.modules` (`lang/go/modules/debug.go`) | scattered in a debugging module; no graph form |
| Word signatures as data | `Debug.sig` (`debug.go:485-510`) | — (adopted) |
| Word bodies; per-body reference edges | `Debug.body`; `Debug.deps` over `native.WalkBodyWords` (`debug.go:198-222`) | per-body only — no whole-system word graph |
| Word/type schema data | `inspect` (`lang/go/native/native_inspect.go`) | shapes are ad-hoc; scry pins stable ones |
| Type lattice navigation | `boru:type-util` (`parent`, `root`, `lca`, `alts`, `paramsof`, `returnsof`) | navigation, not enumeration; no catalog of what exists |
| Word docs / categories / module catalog | `lang/go/native/help` (categories table, `moduleCatalog`, `FuncInfo`) | **text-only** — the taxonomy is not exposed as data to boru at all |
| Execution steps | the cross-registry hook `Registry.SetDebugTraceFrom` + `RunningEngineChain` (`core/go/debug_trace_hook_test.go`); `Debug.trace`/`IO.trace` **print** | no data-returning trace |
| Value structure census | `Debug.shape`, `StructUtil.walk` | — (adopted) |
| Static diagnostics / compilability | `Vm.check`, `Vm.compile` (already data) | none — stays in `boru:vm` (§3) |
| Repo-level structure graph | kg bundle + `KgQuery.*` | none — offline/repo scope, deliberately separate (§3) |

The gaps in column three are the module's actual work: **a whole-system
word graph, a module graph, a data-returning trace, stable schema
shapes, and the help taxonomy as data.** Everything else is
re-packaging.

## 3. Boundaries

- **vs `boru:debug`** — debug is *interactive and quantitative*: taps,
  stepping, breakpoints, sizing, perf. Scry is *declarative*: what the
  system is, as data. Today debug also carries proto-scry words
  (`words`, `defs`, `modules`, `sig`, `body`, `deps`, `shape`); §6
  proposes how the overlap resolves without breaking anything.
- **vs `boru:type-util`** — type-util navigates from a *given* type
  (parent, lca, alts). Scry enumerates and catalogs what the *system*
  has (`Scry.types`, `Scry.schema`, `Scry.type-graph`). Rule of
  thumb: type-util answers "what is this type?", scry answers "what
  types are there and how do they relate?".
- **vs `boru:vm`** — vm analyses *other* source under an explicit
  policy (`Vm.check`, `Vm.compile`, sub-engine runs). Scry reads *this
  live registry*. Self-analysis of source stays vm's job; scry does
  not grow a checker surface.
- **vs the kg pipeline** — kg is the *repository's* evidence-backed
  graph, built offline from `go.work`/`go.mod` and prose. Scry is the
  *process's* live view. They deliberately share the viz graph
  contract so both draw with the same words, but neither subsumes the
  other.
- **vs `boru:origin`** (`design/VALUE-TRACING.0.md`, unshipped) —
  origin is per-value *provenance* (where did this value come from),
  infectious marks and ancestry. Scry is system structure and step
  traces. If origin ships, its `Origin.graph` `{nodes edges}` output
  should adopt the same contract (§9 Q5) so viz draws it too; origin
  remains its own module.

## 4. The word surface

Notation: `~>` denotes "returns". Every word except `Scry.trace` is
read-only and pure: no registry mutation, no I/O. `Scry.trace` is the
carve-out — the *observation* it adds is pure, but it **runs the body
it is given**, so the body's own effects (I/O, `def`s, mutation)
happen exactly as they would under `Debug.steps` (§8); callers and
policy work must not treat a traced run as effect-free.
Graph-returning words emit the contract shape of
[BORU-VIZ.0.md](BORU-VIZ.0.md) §3.1 verbatim; `Scry.trace` emits §3.3
rows; `Scry.schema` emits §3.4.

### Census — what is here

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Scry.modules` | `~> List` | Every native module and its import status: `[{id:'boru:io' namespace:'IO' imported:true} …]`. Richer than `Debug.modules` (which lists ids only). |
| `Scry.words` | `opts:Map ~> List` | Every dispatchable word: `[{name kind:'native'|'defined' module:?} …]`. Opts filter: `{module:'boru:io'}`, `{kind:'defined'}`. |
| `Scry.defs` | `~> Map` | Current def-bound names → active top binding (adopts `Debug.defs`). |
| `Scry.types` | `~> List` | Registered named types: `[{name parent kind:'class'|'record'|'disjunct'|…} …]`. |

### Per-word — what is this word

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Scry.sig` | `Any ~> List` | Signatures as `[{args returns} …]` (adopts `Debug.sig`). |
| `Scry.body` | `Any ~> List` | Quoted body of a boru-defined word; atom `native` for host words (adopts `Debug.body`). |
| `Scry.deps` | `List ~> List` | Distinct word names a quoted body references (adopts `Debug.deps` / `native.WalkBodyWords`). Reach *receivers* are included; literal field keys are not — see `word-graph` for the module-export consequence. |
| `Scry.info` | `String ~> Map` | Describe-as-data: `{name module category doc examples signatures}`. **Needs the one genuinely new Go export**: the `help` taxonomy (categories table, catalog, `FuncInfo`) surfaced as values (phase 3). |

### Graphs — how it hangs together

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Scry.module-graph` | `~> Map` | Contract graph: nodes = modules touched by this program (kind-tagged), edges = import relations (program → module always; module → module where a boru-written module's imports are known, e.g. `boru:repl` → `boru:net`). |
| `Scry.word-graph` | `opts:Map ~> Map` | Contract graph of word references. **Not** a plain fold of `deps`: `WalkBodyWords` deliberately does not walk literal Reach field keys (`core/go/fn_capture.go` — `.code` is a field name, not a reference), so a body's `MathUtil.sqrt` surfaces only `MathUtil`. The builder therefore additionally resolves Reach segments whose receiver is an imported module namespace into `Module.export` edges, or module calls would be absent from the graph. Opts: `{roots:[names]}` (default: all defs), `{depth:n}`, `{include-native:false}` (default — native leaves appear only with `true`, or the graph drowns in `add`/`get`). Nodes carry `kind:'native'|'defined'` and `group:<module>`, so `Viz.collapse` lifts it to a module view for free. |
| `Scry.type-graph` | `opts:Map ~> Map` | Contract graph of a lattice fragment: parent edges from `{root:'Any' depth:n}` or `{types:[names]}`. |

### Values and runtime — what is this thing, what happened

| Word | Signature | Behaviour |
|------|-----------|-----------|
| `Scry.shape` | `Any ~> Map` | Structural census of a value: counts by kind, node count, max depth (adopts `Debug.shape`). |
| `Scry.schema` | `Type ~> Map` | Contract schema of a record/class/surface type: `{name kind fields parents relations}` — the stable curation of what `inspect` returns ad-hoc. |
| `Scry.trace` | `List opts:Map ~> List` | Run a quoted body, returning contract trace rows `[{seq word depth module} …]` instead of printing. Built on the **cross-registry debug trace hook** — `Registry.SetDebugTraceFrom` + `RunningEngineChain` — not `Engine.SetTrace` alone: `CallBoruNamed` runs each boru-defined or module-fn body in a fresh sub-engine and a module fn's frame leaves no tape marks (`core/go/registry.go`), so only the from-registry chain walk sees nested calls; `depth` is the engine-chain length. Effects of the body happen (see the §4 preamble carve-out). Opts: `{max-steps:10000}` (hard bound — the row List is memory), `{include-stack:false}` (`true` snapshots stack heads per row, expensive). Deterministic under `FixedClock` for spec rows. |

Error codes: `scry_unknown_word`, `scry_unknown_type`,
`scry_bad_roots`, `scry_trace_overflow` (body exceeded `max-steps`;
partial rows are *not* returned silently — overflow is an error unless
`{truncate:true}`).

## 5. Composition examples

```
import "boru:scry"
import "boru:viz"
import "boru:string-util"

# what is loaded, and what does the import surface look like?
Scry.modules                      # ~> [{id:'boru:io' namespace:'IO' imported:true} …]
Viz.graph (Scry.module-graph) {}  # ~> paste into the PR

# architectural assertion: my pipeline never touches the vault words
# (filter's Function form: callback first, and over a list the
#  callback receives a {key value} pair — read the node via .value)
def pg (Scry.word-graph {roots:['main-pipeline']})
filter ([n:Any] => [eq 0 (StringUtil.indexof "Vault." n.value.id)]) pg.nodes
# ~> []   — assert empty in a boru:test case

# which defined words are unreferenced? (census + graph, no new words)
def wg (Scry.word-graph {})
def all-defs (keys (Scry.defs))
def referenced (each $.to wg.edges)   # Reach-lens each: one field per edge
# … set-difference in ordinary boru

# what ran, drawn as a sequence diagram
Viz.seq (Scry.trace [fetch parse summarize] {max-steps:5000}) {}

# a record's schema, asserted and drawn from the same value
def s (Scry.schema Order)
s.fields                          # ~> [{name:'id' type:'String'} …]
Viz.classes [s] {}
```

## 6. Relationship to boru:debug — resolving the overlap

`boru:debug` shipped (through Phase 3) with seven words that are
self-knowledge, not debugging: `words`, `defs`, `modules`, `sig`,
`body`, `deps`, `shape`. Under the scry split those are scry's to own.
Proposal, least-breaking first:

1. **Shared natives, two surfaces.** The Go handlers behind those
   words are refactored into shared helpers; `BuildScryModule` and
   `BuildDebugModule` both register them. No behaviour change, no
   removal; `boru describe` marks the debug variants "canonical home:
   `boru:scry`".
2. **Scry is where the surface grows.** New capability (`word-graph`,
   `module-graph`, `schema`, data `trace`, `info`) lands only in scry;
   the debug copies are frozen at today's seven.
3. **The dual surface is a non-uniformity, recorded as
   [NUR063](../NUR.md#nur063)** in the same commit as this note. The
   maintainer's verdict (2026-08-15): scry canonical, the debug copies
   frozen behind the same handlers and deprecated on a stated timeline.
   **Implemented 2026-09-26:** one constructor (`selfKnowledge`) builds
   the seven for both surfaces — the handlers differ only in the name an
   error gives the word (`Scry.sig` / `Debug.sig`) and the code an
   unknown word raises (`scry_unknown_word` / the historical
   `debug_error`); `describe` marks each `Debug.*` copy deprecated,
   naming its `Scry.*` twin; the copies are removed in the first minor
   release after the one that ships `boru:scry`.

   The richer census shapes §4 sketches (`Scry.modules` with
   `{id namespace imported}`, `Scry.words` with filter opts) are scry's
   growth: they land as new forms of scry's words, never by changing
   what the shared seven answer while the debug copies exist.

`Debug.trace` (prints) and `Scry.trace` (returns rows) are different
words for different jobs and both stay.

## 7. Architecture

Unlike viz (pure boru), scry is necessarily **Go natives**: it packages
engine seams. Following `lang/go/CLAUDE.md` module conventions
(`BarrierPos: -1` inner natives, sig-order params, `makeTypedFnDef`
wrappers, docs/catalog/spec lattice — as itemised in
[BORU-VIZ.0.md](BORU-VIZ.0.md) §7's checklist).

**What already exists (reuse, don't rebuild):**

- The seven debug handlers (§6) and their engine seams:
  `native.WalkBodyWords`, registry enumeration, `fn.Signatures`.
- `inspect`'s type/word analysis (`lang/go/native/native_inspect.go`)
  under `Scry.schema`.
- The cross-registry debug trace hook — `Registry.SetDebugTraceFrom`
  + `RunningEngineChain` — under `Scry.trace`. (`Engine.SetTrace`
  alone cannot see into the sub-engines `CallBoruNamed` spawns for
  boru-defined and module-fn bodies.)
- The type registry under `Scry.types` / `Scry.type-graph`.

**What is new (small, and only two touch anything outside the module):**

- Graph assembly (`word-graph`, `module-graph`, `type-graph`) — folds
  over existing enumerations, emitting the viz contract shape.
- A row-collecting trace callback (a `TraceCallback` that appends
  rows, bounded by `max-steps`) — new code over an existing seam.
- The `help` taxonomy data export for `Scry.info` — the one new
  export surface in `lang/go/native/help` (phase 3, and the part most
  worth maintainer eyes before building).

File layout: `lang/go/modules/scry.go`, `docs_scry.go`,
`scry_test.go`, `lang/spec/module-scry.tsv`.

## 8. Policy, safety, phases

Scry is read-only introspection of the process's own program — the
same exposure class as `boru:debug` (bodies, defs, and step traces
reveal loaded code). No new policy scope is needed: per-module import
allowlists already gate it, and **shipped profiles that deny
`boru:debug` should deny `boru:scry` identically**
(`lang/go/policy/profiles/*.jsonic` get one added line each).
`Scry.trace` runs the given body in-engine exactly as `Debug.steps`
does today — it inherits, not extends, that execution surface.

Delivery phases (aligned with viz's — its §9):

1. **Census + per-word + graphs + shape** — pure curation of existing
   seams; unblocks viz's module/word/type graph views.
2. **`Scry.schema` + `Scry.trace`** — unblocks `Viz.classes` and
   `Viz.seq`.
3. **`Scry.info`** (help-taxonomy export) and profile updates.

Test discipline: scry outputs are single-line `canon`-able data, so —
unlike viz — the full surface is TSV-expressible: positive/negative
row pairs in `lang/spec/module-scry.tsv` for every word (ADR-003),
`FixedClock` determinism for trace rows, Go tests for the taxonomy
export, ADR-008 coverage throughout.

## 9. Open questions for the maintainer

1. **The debug overlap verdict** (§6) — **decided** (maintainer,
   2026-08-15; implemented 2026-09-26, NUR063): deprecate. The debug
   copies are kept, frozen and marked, through the release that ships
   scry, and removed in the next minor release.
2. **`Scry.info` shape**: is exporting the `help` taxonomy as data
   acceptable, and should `examples` ship in it (they are generated
   into `examples_gen.go` and sizeable)? (Leaning: yes, with
   `examples` behind `{examples:true}`.)
3. **Trace bounds**: is `max-steps:10000` the right default, and
   should `include-stack:true` cap snapshot width (stack heads only)?
   (Leaning: yes and yes — heads-only, depth 3.)
4. **`Scry.words` and user-defined types in namespaces**: should
   census words see module sub-registries (e.g. words inside an
   imported namespace) or only the top registry? (Leaning: top
   registry + imported namespaces, which is what dispatch can
   actually reach.)
5. **Origin alignment**: when `boru:origin`
   (`design/VALUE-TRACING.0.md`) ships, `Origin.graph` should emit
   the shared graph contract so viz draws provenance unchanged.
   Record that expectation there, or here? (Leaning: an amendment
   note on VALUE-TRACING.0.md when origin work starts.)

No ADR entry is proposed — per repo policy this stays a `design/` note
until a maintainer says otherwise.
