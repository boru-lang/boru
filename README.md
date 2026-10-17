# boru

**boru** is a typed, word-based query language. A program is a sequence
of *words*, and a word takes its arguments where you write them —
`add 1 2` reads left to right, like a conventional call — while binary
operations also read naturally in infix position (`10 sub 3`). Every
value carries a hierarchical type, and every word declares typed
signatures that drive dispatch. Underneath, boru is concatenative: words
can equally take their arguments from a value stack, which is what
makes point-free pipelines compose — but you can write a lot of boru
before thinking about the stack, so start with the forward forms below
and meet the stack later. The reference implementation is in Go and
ships as a single `boru` binary that includes a REPL, a type checker, a
formatter, an LSP server, a registry client, a secrets vault, and a
multi-service supervisor.

> **The repo as data.** A machine-readable, evidence-backed knowledge
> graph of this repository — its modules, docs, tools, and concepts —
> lives at [`kg/out/graph.json`](kg/out/graph.json), built in boru by the
> pipeline in [`kg/`](kg/README.md). Refresh it whenever a PR changes
> the repository's structure or documentation (`make -C kg graph`).

> **Notation.** In code, a trailing `# returns …` comment shows what an
> expression evaluates to (`square 4  # returns 16`); in prose we say
> "`square 4` returns `16`". The comment is ordinary documentation — `#`
> just begins a line comment — not special syntax. (We deliberately
> avoid an `=>` arrow for results: `=>` is real syntax in boru, the
> anonymous-function arrow, sugar for the word `afn`.)

```boru
# words take their arguments where you write them
add 1 2 # returns 3
10 sub 3 # returns 7

# typed functions, lists, maps, records, concurrency
def square fn x:Number Number [mul x x]
square 4 # returns 16

[1, 2, 3] each [square] # returns [1 4 9]
{name:"Ada"}.name # returns 'Ada'

def Point refine Record [x:Number y:Number]
make Point [3 4] # returns {x:3 y:4}

import "boru:time-util" TimeUtil.await [[add 1 2] [add 3 4]] # returns [3 7]

# macros: add new syntax in boru itself (hygienic; this one expands to an `if`)
def unless (macro [[c body] [quote [if unquote c [] unquote body]]])
unless false [42] # returns 42
```

The stack is still there when you want it: `10 3 sub` is the all-stack
spelling of `10 sub 3`, and pipeline-style code leaves intermediate
values on the stack between words (stack-shuffling words like `dup` and
`swap` work only that way). The **[Tutorial](TUTORIAL.md)** introduces
the stack model when you need it, and the
**[Explanation](EXPLANATION.md)** covers how forward collection changes
the feel of stack code.


## Forward arguments — the primary way to write boru

boru is concatenative, but you rarely have to think in stack terms,
because the defining feature of the surface syntax is **forward
arguments**: a word takes its operands from the tokens written *after*
it, in declared order, exactly like a conventional function call.

```boru
add 1 2 # returns 3 — the word comes first
def square fn x:Number Number [x mul x]
square 4 # returns 16 — your own words read the same way
import "boru:math-util" # imports read forward, too
3 7 MathUtil.min # returns 3 — module words are then in scope
```

This is the canonical, recommended form for **all new code, examples,
and documentation** — including imports: write `import "boru:math-util"`,
not the stack spelling. Forward form reads top-to-bottom and
left-to-right, the written argument order matches the declared parameter
order, and you never have to track what is sitting on the stack:

- `f a b c` binds `a`, `b`, `c` to the first, second, third parameters.
- Calls compose by parenthesising: `(f a b) g c`.
- The same rule covers built-ins, your own `def fn`s, and `import`.
- Dotted module words follow it too: `MathUtil.min 3 7` and the args-first
  `3 7 MathUtil.min` both dispatch.

**Except for the operators.** The two-argument words common convention
reads as infix operators — `add`, `sub`, `mul`, `div`, `mod`, `pow`,
`and`, `or`, `lt`, `lte`, `gt`, `gte`, `eq`, `neq` — are written
**infix**: `1 add 2`, `10 sub 3`, `n lte 1`. They are ordinary
two-argument functions, not operators the grammar knows about; infix is
simply the spelling that reads the way it computes
([STYLE-GUIDE.md §S2](STYLE-GUIDE.md)).

### One rule behind both

Arguments bind in signature order. Matching takes them from the tokens
after the word, in written order, up to that signature's barrier, and
fills whatever positions remain from the value stack in reverse — top of
stack first. That is the whole of it, at every arity: two-argument words
are **not** a special case and there is no "swap form".

Two consequences fall out. Put every operand on the stack and you get
Forth order, so `10 3 sub` is `7`. Write every operand after the word
and you get the order you wrote them, which for an asymmetric word reads
backwards: `sub 1 3` is `2`, and `sub 10 3` is `-7`. The stack forms are
there for point-free pipelines; see the **[Tutorial](TUTORIAL.md)** to
start writing boru and the **[Explanation](EXPLANATION.md)** for how
forward collection works underneath.


## Install

Until v0.1.0 is tagged, build from a clone (the `cmd/go` module
carries local `replace` directives, so `go install …@latest` is
not yet supported):

```bash
git clone https://github.com/boru-lang/boru
cd boru/cmd/go
go install ./boru
boru -version
```

Then:

```bash
boru                                  # start the REPL
boru do 'add 1 2'                     # one-shot expression
boru script.boru                       # run a file
boru check script.boru                 # type-check, don't run
boru fmt script.boru                   # format in place (always rewrites)
boru build script.boru -o tool         # compile to a standalone executable
boru help                             # introduction + the subcommand list
boru describe                         # a categorised guide to every built-in word
```

A wasm-powered browser playground is bundled in
[`docs/index.html`](docs/index.html); build it locally with
`make -C wpg wasm`. On every push to `main`, the
[`Deploy playground to Pages`](.github/workflows/pages.yml) workflow
rebuilds it and publishes the result to GitHub Pages, so the live
playground always tracks the latest engine.


## Documentation

The manual is organised into four documents, one for each kind of
learning need, plus a CLI reference and the architecture record:

| Document | When to read it |
|----------|-----------------|
| **[Tutorial](TUTORIAL.md)** | You are new to boru and want to learn it step by step. |
| **[How-To Guides](HOWTO.md)** | You have a specific task and want a recipe. |
| **[Reference](REFERENCE.md)** | You need the precise behaviour of a syntax form, type, or word. |
| **[Explanation](EXPLANATION.md)** | You want to understand *why* boru is the way it is. |
| **[CLI Reference](CLI.md)** | You want to drive the `boru` binary from the shell. |
| **[Style Guide](STYLE-GUIDE.md)** | You are writing boru and want the house style — which of several equivalent spellings to use, and which of those `boru fmt` enforces for you. |
| **[Architecture Design Record](ADR.md)** | You want the key architectural decisions and the reasoning behind them. |
| **[Non-Uniformity Register](NUR.md)** | You want the recorded deviations from the language's uniform rules, each pending, resolved, or explicitly allowed. |
| **[Agent Guide](AGENTS.md)** | You are an AI agent (or new contributor) and want a map of the docs, the tooling, and how to discover the language with `boru describe`/`boru help`. |
| **[Project knowledge graph](kg/README.md)** | You want the repository as data: modules, docs, tools, and concepts with evidence-backed relations, in [`kg/out/graph.json`](kg/out/graph.json) — built (and dog-fooded) in boru. Keep it updated with each PR. |

Suggested reading orders:

- **Brand new to boru?** Start with the **[Tutorial](TUTORIAL.md)**.
  After you've written a few programs, read the
  **[Explanation](EXPLANATION.md)** to understand the model.
- **Coming from Forth / Factor / APL?** Skim the
  **[Explanation](EXPLANATION.md)** first — it explains how forward
  collection and type-directed dispatch change the feel of stack
  code — then dive into the **[Reference](REFERENCE.md)**.
- **Building a script or a tool?** Use the
  **[How-To Guides](HOWTO.md)** and **[CLI Reference](CLI.md)** as
  your starting points; consult the **[Reference](REFERENCE.md)**
  for word-by-word detail.


## Repository layout

| Path | What it is |
|------|------------|
| `cmd/go/` | The `boru` CLI / REPL (`github.com/boru-lang/boru/cmd/go`). |
| `lang/go/` | The language layer: public `lang` API and the consolidated `native` word library. |
| `basic/go/` | The base language layer: fundamental words and predefined content types, on core+parser only (ADR-013). |
| `check/go/` | The type-checker module: the analysis pass, carriers, and check-mode diagnostics. Builds on core alone. |
| `core/go/` | The interpreter kernel: values, types, matching, the step loop — the one execution path. Standalone (apd only); every other module builds on it. |
| `parser/go/` | The parser module: boru source text to kernel values (tabnas/jsonic lexer plus the embedded declarative grammar). Depends on core alone. |
| `calc/go/` | A small calculator built directly on `core` + `parser` (learning example). |
| `wpg/` | The wasm web playground (`wpg/wasm` + `wpg/serve`). |
| `test/` | Shared TSV spec-runner scaffolding and HTTP test fixtures. |
| `kg/` | The project knowledge graph: a boru-built, evidence-backed map of the repo (pipeline + committed bundle). |
| `utils/` | A coreutils subset written in boru (`cat`, `wc`, `head`, `grep`, …) — real programs, built with `boru build`. |
| `docs/` | The bundled wasm playground (`index.html`). |
| `lang/spec/` | Engine spec TSV files (the language's executable spec). |
| `design/` | Internal design notes and proposals. |


## Building from source

```bash
make test                            # all modules
make vet                             # all modules
make fmt                             # all modules
make lint                            # all modules (golangci-lint)

cd cmd/go && make build              # builds bin/boru
cd wpg     && make wasm              # builds docs/index.html
cd wpg     && make serve             # runs the playground on :8080
```

Before committing, run from the repo root:

```bash
make fmt && make vet && make lint && make test
```


## Upgrade notes (pre-1.0 breaking changes)

boru is pre-1.0 and the surface still moves. Libraries written against
mid-2026 snapshots most commonly need these renames:

- **Namespaces are capital-initial.** `test.test` → `Test.test`,
  `assert.equal` → `Assert.equal`, `math.sqrt` → `MathUtil.sqrt`.
  Lowercase export names are rejected.
- **Utility modules took a `-util` suffix** (avoiding builtin type-name
  clashes): `boru:array` → `boru:array-util` (binds `ArrayUtil`), and
  likewise `boru:math-util`, `boru:time-util`, `boru:type-util`,
  `boru:matrix-util`, `boru:bin-util`, `boru:struct-util`,
  `boru:logic-util`, `boru:string-util`.
- **Words moved out of core into modules:** string words →
  `boru:string-util` (with `indexof` now **haystack-last**, and the list
  form split out as `ArrayUtil.indices`); voxgig-struct words
  (`merge`, `setpath`, `jsonify`, …) → `boru:struct-util`; bitwise →
  `boru:bin-util`; I/O except `print` → `boru:io`; HTTP → `boru:net`;
  clock/async → `boru:time-util`; derived boolean connectives →
  `boru:logic-util`. Moved words are no longer available unqualified.
- **Module fn exports use the referent form:**
  `export "Mod" {double: double/v}` (a bare fn name in an export map
  would be invoked, not referenced).
- **`refine Object {…}` is removed — classes are defined with `class`:**
  `def Foo class {x:1}` (paren-free), subclass via `def Bar refine Foo
  {…}`. Class instances are flat (no prototype chain), **sealed**
  (undeclared field writes raise `sealed_field`), strictly typed at
  `make` and `set` (no silent conversion; predicate field types run),
  root under `Ideal/Class` (so `p is Object` is now false), and
  serialize as pure JSON with a `$class` key (`StructUtil.jsonify` /
  `StructUtil.reify`).
- **`set` mutates a Store or class instance in place and returns
  nothing** — `def r (b set k v)` binds nothing; read the container
  back instead. A **FlexMap**/**FlexList** `set` also mutates in place
  but *returns the node* (so writes chain). On an immutable **Map**,
  `set` is copy-returning: `{a:1} set b 2` yields a new map and the
  receiver is unchanged.


## Contributing

Bug reports, proposals, and pull requests are welcome on
[GitHub](https://github.com/boru-lang/boru). For non-trivial
language changes, open an issue first — the design notes under
`design/` are the historical record of how previous
proposals played out.


## License

boru is released under the terms of the [MIT License](LICENSE.md).
