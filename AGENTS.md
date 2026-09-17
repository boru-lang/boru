# AGENTS.md — agent guide to the boru repository

**boru** is a concatenative, strongly-typed query language implemented in
Go: programs are sequences of *words* that transform a *stack*. The
reference implementation ships as a single `boru` binary (REPL, type
checker, formatter, LSP, registry client, vault, supervisor). New to the
repo? Skim [README.md](README.md) first.

This file is a **router**. It points you at the right documentation and
tools for the task in front of you; it does not duplicate them. Read the
linked source — it is authoritative, this page is just the index.

> **The repo as data — the project knowledge graph.** The repository
> ships a map of itself: Go modules and packages, documents, tools, and
> core concepts, every relation backed by evidence. Structure comes from
> **code** — `go.work` and every `go.mod`, quoting the actual `require`
> lines, so `depends_on` is directed and cannot drift from the build;
> the rest comes from quoted passages in the docs.
>
> **To orient, read [`kg/out/graph.md`](kg/out/graph.md)** — a few
> hundred lines, module dependency view first ("what does `eng/go`
> depend on, and what depends on it?"). The full bundle is
> [`kg/out/graph.json`](kg/out/graph.json): the machine contract, ~115 KB,
> not something to read whole. Query either with `kg/queries.boru`
> (`modules`, `code-unit-by-path`, `dependencies-of`, `dependents-of`, …).
>
> The code half refreshes itself. **If your PR changes the repository's
> documentation set, update
> [`kg/project/boru-project.jsonic`](kg/project/boru-project.jsonic);
> either way rebuild with `make -C kg graph`** so the committed graph
> stays current (the build is deterministic — unchanged input, unchanged
> bytes). `make -C kg verify` tells you whether it is current without
> rebuilding, and names the files that moved: the `generated_at` stamp
> is pinned and is NOT a freshness signal, the input digest is.


## First: let the tool document itself (`boru describe` / `boru help`)

Before grepping source or guessing a word's signature, **ask the binary**.
The `boru` CLI documents both the language and itself, and that output is
generated from the *live engine* — signatures, precedence, type lattice,
and worked examples are the real ones the runtime uses, so they cannot
drift from the code the way prose can. Each worked example carries the
engine's own answer: a stack render, `(no value)` when the example leaves
the stack empty, or `error [boru/<code>]` when the engine refuses it.
`lang/go/test/help_examples_test.go` re-runs every **generated** example
against a fresh engine and fails on any mismatch, so those results are
gated, not asserted. Two scopes to know:

- **Hand-authored examples** (a word's `help.Entry.Examples`) are curated
  prose and are NOT re-run — `TestHandAuthoredExamplesWin` only checks
  that they are the ones rendered. Treat them as reviewed documentation,
  not as gated output.
- **Placeholders** (`;# ...`) survive only where the generator cannot
  evaluate an example — chiefly words implemented in a loadable module,
  which it evaluates on a registry that has not imported it. Every such
  site is tracked in a ratchet keyed by `word | expression`, and both a
  NEW placeholder and a STALE entry fail the build.

For "what does this word do / what are its signatures / which module is it
in", `boru describe` is the source of truth; reach for it first.

Run it without building anything:

```bash
cd cmd/go && go run ./boru <args>      # e.g. go run ./boru describe add
```

or build the binary once and call it directly:

```bash
cd cmd/go && make build               # → cmd/go/bin/boru
./bin/boru describe add
```

There are **two** discovery systems.

### `boru describe` — the *language* (words, categories, modules)

| Command | Shows |
|---------|-------|
| `boru describe` | A categorised guide: every built-in word grouped by category, then the loadable modules. Start here. |
| `boru describe <word>` | One word in full: summary, precedence, all signatures, worked examples, notes. e.g. `boru describe add` |
| `boru describe <category>` | The words in one category. e.g. `boru describe math` (categories: math, compare, boolean, binary, string, stack, storage, control, type, query, io, help) |
| `boru describe boru:<module>` | A module's summary and the words it exports. e.g. `boru describe boru:type-util` |
| `boru describe boru:<module>:<word>` | One exported word of a module, with provenance. e.g. `boru describe boru:type-util:tpartial` |

If the name isn't a known word/category/module, `describe` tries to **load
it as a module** (installed package or `./file.boru`) and document that. So
`boru describe ./mylib.boru` works too.

### `boru help` — the *CLI* (subcommands and their flags)

| Command | Shows |
|---------|-------|
| `boru help` | An introduction plus every subcommand (run, do, check, fmt, vault, lsp, serve, …). |
| `boru help <subcommand>` | One subcommand's summary; then run `boru <subcommand> -h` for its full flag set. e.g. `boru help vault` |

Rule of thumb: **`help` = the tool, `describe` = the language.**

### In the REPL

`boru repl` (or just `boru` with no args). The same two systems are at the
prompt, plus REPL meta-commands:

- Words: `describe` and `help` are ordinary boru words. An argument that
  contains `:` or `.` (a module ref or a dotted export) is source syntax,
  so quote it: `describe "boru:type-util"`, `describe "boru:type-util:tpartial"`.
- Meta-commands (lines starting with `/`): `/describe [name]` (takes its
  argument raw — no quoting), `/help` (overview + the meta-command list),
  `/stack [n]`.

Full REPL reference: [CLI.md → REPL meta-commands](CLI.md#repl-meta-commands).


## Task router — where to read

| If you want to … | Read |
|------------------|------|
| Learn boru step by step | [TUTORIAL.md](TUTORIAL.md) |
| A recipe for a specific task | [HOWTO.md](HOWTO.md) |
| The precise behaviour of a syntax form, type, or word | [REFERENCE.md](REFERENCE.md) — or `boru describe <word>` for one word |
| Know which of several equivalent spellings to write | [STYLE-GUIDE.md](STYLE-GUIDE.md) — house style, and what `boru fmt` does and does not enforce |
| Understand *why* boru is designed the way it is | [EXPLANATION.md](EXPLANATION.md) |
| Drive the `boru` binary (every subcommand, REPL) | [CLI.md](CLI.md) |
| The key architectural decisions and their rationale | [ADR.md](ADR.md) |
| The recorded non-uniformities of the language and their verdicts | [NUR.md](NUR.md) |
| The formal semantics | [FORMAL-SPEC.md](FORMAL-SPEC.md) |
| The executable language spec (the rows tests run against) | [`lang/spec/*.tsv`](lang/spec/) |
| See the repository itself as a graph (modules, packages, docs, concepts, evidence) | [`kg/out/graph.md`](kg/out/graph.md) to read, [`kg/out/graph.json`](kg/out/graph.json) to query — guide: [kg/README.md](kg/README.md) |
| Know what a Go module depends on, or what depends on it | [`kg/out/graph.md`](kg/out/graph.md) §Code units — read from `go.work` + `go.mod`, not from prose |


## Build, test, verify

From the repo root, the **pre-commit checklist** (run all five before every
commit — `make lint` catches what `vet` and `test` miss):

```bash
make fmt && make vet && make lint && make test && make cover-gate
```

`make ci-local` runs exactly the steps CI runs, in CI's order
(`scripts/ci-steps.sh` is the one definition both share) — use it before a
push; the five-target line above is the subset a change usually needs.
Two switches make iteration fast: `BORU_SPEC_FILES=callbacks.tsv,fold-*.tsv`
restricts every corpus walk in `test/go/langspec` to the named spec files
(the ten gates over one family run in seconds), and `BORU_DIRECTION_GATES=1`
arms the direction lane — the gates against their END STATE, red by design
until full compilation is done (`make test-direction`; the default lane
asserts only the regression ceilings and is what blocks). `make gate-status`
prints every gate's live value against both numbers.

`make cover-gate` enforces **ADR-008**: 100% unit-test coverage of every
reachable Go statement, the sole exclusions being provably-unreachable
guards carrying a proof-carrying `//covergate:allow <reason>` comment on the
guard's opening line (`design/COVERAGE-ALLOWLIST.10.md`). It is the
slowest of the five and the one most often skipped; skipping it is how a
merged PR turns CI red.

Faster, scoped iteration:

```bash
cd lang/go && go test ./native/ -run TestSomething -v
cd cmd/go  && make build        # builds cmd/go/bin/boru
cd wpg     && make wasm          # builds the docs/ wasm playground
```


## Working in the code — module deep guides

The detailed, **must-read** conventions live next to the code they govern.
Read the relevant one before changing that module:

| Area | Guide |
|------|-------|
| Engine kernel — types, values, signatures, matching, the step loop, the parser bridge | [eng/go/CLAUDE.md](eng/go/CLAUDE.md) |
| The parser — source text to kernel values, and the never-invent-names rule | [parser/go/CLAUDE.md](parser/go/CLAUDE.md) |
| Language layer — native words, modules, registry, help/describe, capabilities | [lang/go/CLAUDE.md](lang/go/CLAUDE.md) |

A few rules from those guides that bite hardest when missed:

- **Never** add an entry to `ADR.md` unless a maintainer explicitly says so;
  design discussion is captured in `design/*.md`, not the ADR.
- **Record every non-uniformity in [NUR.md](NUR.md)** the moment one
  surfaces — in code review, in a design, or while coding and debugging.
  Recording is mandatory and needs no maintainer instruction; the
  *allowed* verdict does. A **Pending** record does NOT block the PR —
  it may stay open across many merges. The register's job is that a
  divergence is never lost or silently baselined.
- **Pair every positive test with a negative one** — assert what must be
  *rejected*, not just what passes.
- **Panics are forbidden** outside annotated init-time type registration;
  return errors instead.
- **Argument order is ONE rule, at every arity.** A call binds its
  arguments in signature order: matching fills positions from the
  **forward stack** (the tokens after the word, in written order) up to
  the signature's barrier, then fills the rest from the **value stack**
  in reverse (top of stack first). Two-argument words are **not** a
  special case and there is **no "swap form"** — that phrasing is a
  legacy misunderstanding; if you find it in a doc, fix the doc. Two
  consequences: all operands on the value stack gives Forth order, and
  all operands written forward gives written order — which for a
  non-commutative word reads backwards (`sub 1 3` is `2`, because `sub`
  computes `args[1] - args[0]`).
- **Infix form is the idiom for infix operators**: write `1 add 2`,
  `10 sub 3`, `n lte 1` — one operand on the stack, one forward — for
  every two-argument word common convention reads as an operator (`add`,
  `sub`, `mul`, `div`, `mod`, `pow`, `and`, `or`, `lt`, `lte`, `gt`,
  `gte`, `eq`, `neq`). Everything else takes forward form `f a b c`.
  Mind the operand order when converting: `sub 10 3` is `-7` while
  `10 sub 3` is `7` ([STYLE-GUIDE.md](STYLE-GUIDE.md) §S2).
- **Fewest square brackets in a signature**: `fn x:Integer Integer [body]`,
  not the five longer spellings of the same thing
  ([STYLE-GUIDE.md](STYLE-GUIDE.md) §S1). `boru fmt` only collapses the
  fully canonical form, so this one is on you (NUR088).


## Repository layout

| Path | What it is |
|------|------------|
| `cmd/go/` | The `boru` CLI / REPL (and the `help`/`describe` plumbing). |
| `lang/go/` | The language layer: public `lang` API + the `native` word library + loadable `modules`. |
| `basic/go/` | The base language layer: fundamental words (stack, definition, control, type-generics) + predefined content types. Depends on core+parser only — not check, not compiler, both of which it reaches through core-owned seams (ADR-013). |
| `eng/go/` | The VM and run/fork entry points over core+check+compiler, plus the kernel spec runner. The parser moved out to `parser/go`; `basic`, `calc` and `cmd` no longer depend on eng at all. |
| `compiler/go/` | The compiler module: the emit recorder, lowering, and the bytecode emitter. Builds on check and core; eng runs its bytecode. |
| `check/go/` | The type-checker module: the analysis pass, carriers, and check-mode diagnostics. Builds on core alone. |
| `core/go/` | The interpreter core module: values, types, matching, the step loop. Standalone (apd only); eng builds on it. |
| `parser/go/` | The parser module: boru source text → `[]core.Value` (tabnas/jsonic lexer + the embedded declarative grammar). Depends on core alone. |
| `lang/spec/` | The executable language spec (TSV files). |
| `calc/go/` | A small calculator built on `eng` (learning example). |
| `wpg/` | The wasm web playground. |
| `test/` | Shared TSV spec-runner scaffolding and HTTP fixtures. |
| `kg/` | The project knowledge graph: an evidence-backed boru pipeline and its generated bundle. |
| `utils/` | A coreutils subset written in boru — real programs that prove the CLI story (argv, exit codes, streams, baked permissions). |
| `design/` | Internal design notes and proposals (historical record). |
