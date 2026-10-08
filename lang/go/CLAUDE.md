# CLAUDE.md

## Project

This is **boru** — a concatenative query language implemented in Go. Each
component sits under a top-level folder; the Go implementation of each
lives in a `<component>/go/` subfolder so that future TS ports can sit
alongside as `<component>/ts/`. The Go modules are:

- `eng/go/` — the engine kernel + jsonic parser + kernel spec runner
  (`github.com/boru-lang/boru/eng/go`).
- `basic/go/` — the base language layer
  (`github.com/boru-lang/boru/basic/go`): the fundamental words
  (stack, definition, control-flow, type-generics) and the predefined
  global content types (the Scalar/Time family, Resource/Entity),
  registered against eng. Depends on eng ONLY — the layering
  `eng ← basic ← lang ← cmd` is a hard rule (ADR-013).
- `lang/go/` — the language layer: `lang.New()` API and the consolidated
  `native` package (the eng-shim + every built-in word) plus the
  production spec suite (this module,
  `github.com/boru-lang/boru/lang/go`). The pre-May-2026 split between
  `lang/go/engine/` (language-layer primitives + alias shim) and
  `lang/go/native/` (data-manipulation words) was merged into a single
  `lang/go/native/` package — see `Package layout` below. The
  fundamental-word slices and predefined content types moved DOWN to
  `basic/go` (ADR-013); `native/basic_shim.go` re-exports them so the
  files here keep their historical spellings, and `Register` still
  applies the moved groups at the historical registration points
  (overload appends are order-sensitive).
- `cmd/go/` — the `boru` CLI / REPL
  (`github.com/boru-lang/boru/cmd/go`).
- `calc/go/` — a small calculator built directly on `eng` (learning example,
  `github.com/boru-lang/boru/calc/go`; not published).
- `wpg/` — the wasm web playground: `wpg/wasm` (browser wasm build) and
  `wpg/serve` (standalone HTTP server with embedded HTML)
  (`github.com/boru-lang/boru/wpg`).
- `test/go/` — shared TSV spec-runner scaffolding
  (`github.com/boru-lang/boru/test/go`).
- `test/solardemo/` — standalone HTTP fixture used by API tests
  (`github.com/boru-lang/boru/test/solardemo`).

Language-agnostic content stays at the top of each component:
`eng/spec/`, `lang/spec/`, `lang/doc/`, `calc/doc/`.

## Package layout

`lang/go/` contains:

- `boru.go`, `parse.go` — the public `lang` package (entry points
  `lang.New()`, `(*Boru).Run`, `(*Boru).Check`, `(*Boru).Register`,
  re-exports of `Type`/`Value`/`Signature` for handler authors).
- `native/` — every built-in word and the kernel-shim aliases.
  Sub-files:
  - `aliases.go` — re-exports the eng kernel's exported types/funcs
    as `native.Foo` so the rest of the lang code can stay
    eng-agnostic.
  - `basic_shim.go` — the same pattern one layer up: re-exports the
    fundamental-word registration groups, handler internals, and
    Scalar/Time surface that moved to `basic/go` (ADR-013), under
    their historical package-local spellings.
  - `register.go`, `setup.go` — `Register`/`DefaultRegistry` entry
    points.
  - `native_*.go` — language-layer primitives (math, boolean,
    string, accessor, …). Each file exposes a
    `xxxNatives []NativeFunc` slice that `Register` iterates. The
    fundamental slices (stack, definition, control, type-generics,
    const) live in `basic/go` and arrive through the shim.
  - `natives.go` and the per-feature files (`clone.go`, `fetch.go`,
    `filter.go`, …) — data-manipulation words historically owned
    by the `native` sub-package and merged into this consolidated
    package in May 2026.
  - `format.go`, `query.go`, `sqlite.go`, `fileio.go` —
    feature-specific helpers (`conditional.go` / `forloop.go` /
    `case_exhaustive.go` moved to `basic/go` with the control
    words).
  - `help/` — dynamic help-system implementation (sub-package).
- `formatter/` — code pretty-printer (no engine deps).
- `capabilities/` — file I/O abstraction (`FileOps` interface
  + OS-backed and in-memory implementations).
- `modules/` — loadable modules. Import binds a CamelCase namespace
  (`import "boru:math-util"` → `MathUtil.sqrt`).
  **Naming rule:** a `-util` id + `*Util` namespace marks a **utility
  library** (a collection of pure/domain helper functions):
  `boru:math-util` (`MathUtil`), `boru:array-util` (`ArrayUtil`),
  `boru:time-util` (`TimeUtil`), `boru:type-util` (`TypeUtil`),
  `boru:matrix-util` (`MatrixUtil`), `boru:string-util` (`StringUtil`),
  `boru:bin-util` (`BinUtil`), `boru:struct-util` (`StructUtil`),
  `boru:logic-util` (`LogicUtil`). Capability / framework / DSL modules
  stay plain: `boru:io` (`IO`), `boru:net` (`Net`),
  `boru:vm`, `boru:report`, `boru:test`, `boru:rand`, `boru:query`.
  (The `-util` suffix also conveniently avoids the type-name clashes for
  `Array`/`Time`/`Type`/`Matrix`/`String`, which are builtin types.)
  Exported names must be capitalised (`export "Foo"`; lowercase rejected).
  `boru:struct-util` (`StructUtil.` namespace) holds the voxgig-struct
  data-manipulation words — `clone`, `getpath`, `setpath`, `inject`,
  `merge`, `walk`, `items`, `transform`, `validate`, `selector`,
  `jsonify`, `nodify` — moved OUT of core (see `native/struct_module.go`).
  `boru:io` (`IO.` namespace) holds the I/O words — `printstr`, `read`,
  `write`, `stdin`, `stdout`, `stderr`, `trace` — also moved out of core
  (see `native/io_module.go`); only `print` stays in core.
  `boru:net` (`Net.` namespace) holds the HTTP / API words — `fetch`,
  `prepare`, `direct` (see `native/net_module.go`).
  Further moves out of core: bitwise `band`/`bor`/`bxor`/`bnot`/`bsl`/`bsr`/
  `busr` → `boru:bin-util` (`BinUtil.`); clock/async `now`/`sleep`/`timeout`/`interval`/
  `await`/`cancel` → `boru:time-util` (`TimeUtil.`); `tpartial` → `boru:type-util`
  (`TypeUtil.`); `folder` → `boru:io`; and the derived boolean connectives
  `nand`/`nor`/`xnor`/`iff`/`implies` → `boru:logic-util` (`LogicUtil.`).
  All moved words are no longer available unqualified. (`slice` is NOT
  a string-family straggler: it is a core *sequence* word polymorphic
  over String/List/Bytes, kin of `size`/`take`/`reverse`, which stay
  core for the same reason — NUR019.)
- `test/` — integration tests and TSV spec runners.

## ADRs — only on explicit instruction

**Never add an entry to `ADR.md` unless the maintainer explicitly
instructs it** ("add an ADR for X"). Design conversations, reviews,
and directions accepted in discussion are *discovery* — they are
captured in `design/*.md` notes, not as ADR entries, however settled
they sound. The same rule is stated in the `ADR.md` header.

## Known sharp edges (app-authoring)

Language- and compiler-level sharp edges found by writing a large app
entirely in boru are collected in **`design/BORU-SHARP-EDGES.0.md`** —
minimal repros, root-cause hypotheses, workarounds, and a triage table.
Re-verified 2026-07-30: G8 (recovered-`raise` binding teardown), G11
(returned-list-literal laziness) and G13a (single-token bare-map body)
**no longer reproduce** — fixed by unrelated work. G9 (`case`-default
collection, NUR048), G13b (type-literal map values refusing to
compile, NUR051) and G12 (an `/v`-parked fn not satisfying a
`Function` param, NUR050) were **resolved 2026-07-31** — an open-call
`case` default runs isolated like a matched arm, nested bare type
nodes intern as type operands (ADR-010), and there is exactly one
function type (`Word/__FN` collapsed into `Type/Function`; `/v`-marked
words feed forward collection as references — ADR-011). G10
(`(dot message)` receiverless in an `error` handler) was **resolved
2026-08-03**, retiring NUR049: `error` handler bodies are checked on
the plain pass (`errorReturnsFn`'s seeded run, un-gated), the paren
suggestions carry the sealed-stack caveat, and both shipped examples
are repaired. Read the note before re-deriving a workaround.

## Build & Test

```bash
make test         # from repo root: fans out across every module
make vet          # vet across every module
make fmt          # gofmt across every module
make fmt-docs     # boru fmt over the gated user-facing docs' ```boru blocks
make lint         # golangci-lint across every module — RUN BEFORE COMMIT

cd cmd/go
make build        # builds bin/boru

cd ../wpg
make wasm         # builds ../docs/index.html (bundled boru.wasm playground)
make serve        # runs the HTTP playground on :8080
```

Run a specific test:
```bash
cd lang/go && go test ./test/ -run "TestFactorialTypeScaling" -v
```

**Pre-commit checklist.** Before every `git commit`, run from the
repo root:

```bash
make fmt && make vet && make lint && make test && make cover-gate
```

`make lint` runs `golangci-lint` (configured via `.golangci.yml`)
and catches ineffassign / unused / shadowed-variable issues that
`go vet` and the test suite both miss. Skipping it lets a clean-
locally commit fail CI; the cost of running it is a few seconds.

`make cover-gate` enforces **ADR-008**: 100% unit-test coverage of every
reachable Go statement, measured across the merged cross-suite profile.
The sole exclusions are provably-unreachable defensive guards, each marked
with a proof-carrying `//covergate:allow <reason>` comment on the guard's
opening line; the gate fails if such a guard becomes covered or the pragma
goes stale. See `design/COVERAGE-ALLOWLIST.10.md` and
`design/TEST-SEAMS.10.md`.

## Test discipline — always pair positive with negative

**Whenever you add or change behaviour, add negative tests alongside
the positive ones, wherever a negative case is expressible.** A test
that only proves the happy path passes leaves the actual contract —
*what must be rejected* — unguarded, and that is where regressions hide.
For every "X is accepted / returns Y" assertion, ask "what must be
refused here?" and pin it too:

- the wrong type / shape / arity raises the expected error
  (`ERROR:<substring>` rows in the `.tsv` specs, or an error-asserting
  Go test),
- sibling / unrelated / supertype values are rejected where a specific
  subtype is required,
- an unknown / undefined name errors rather than silently degrading
  (e.g. to `Any`),
- boundary and empty inputs behave as specified.

This is not optional polish: the user-type return-annotation bug
(`lang/spec/user-types.tsv` §7–8) was invisible for as long as it was
precisely *because* every existing row asserted only acceptance and
returned the builtin `[Integer]` — no row ever required a user type to
be rejected, so the silent `Any` degradation passed every test. The
negative rows are what make the type contract real. The `recover()`-based
no-panic tests (see Panic Prevention) are the same discipline applied to
crashes: assert the bad input is handled, not just the good one.

## Dependencies

The `github.com/voxgig/struct` module is published as a Go submodule at
`github.com/voxgig/struct/go`. The `go.mod` replace directive handles this:

```
replace github.com/voxgig/struct v0.1.0 => github.com/voxgig/struct/go v0.1.0
```

If `go build` or `go test` fails downloading `modernc.org/sqlite` (or
other large modules) with a timeout from `storage.googleapis.com`, run:

```bash
GOPROXY=direct go mod download
```

This bypasses the Go module proxy and downloads directly from the source
repositories. After that, `go build ./...` and `go test ./...` will work
normally using the cached modules.

## Jsonic Token Usage

boru uses `github.com/tabnas/jsonic/go` (v0.6.2, over
`github.com/tabnas/parser/go` v0.8.3) for all tokenization and
structural parsing. (This replaced the legacy `github.com/jsonicjs/jsonic/go`
v0.1.6 — the tabnas family is an API-compatible superset port; the swap
removed the jsonicjs dependency entirely.) Defects in it are fixed
UPSTREAM, never behind a boru shim — **ADR-014**, with the case study in
`design/TABNAS-UPSTREAM-FIRST.0.md`. There is exactly one parser — it
lives in the standalone **eng** module at `eng/go/parser/parse.go`
(`github.com/boru-lang/boru/eng/go/parser`); `lang.Parse` re-exports
it. (The old hand-rolled lexer / token / AST / tree-walking evaluator
were removed — jsonic is the sole parsing path.) Key jsonic integration:

- **Options**: the output-shaping flags moved under `Options.Info`:
  `Info:{Text:true}` (quoted vs unquoted distinction), `Info:{List:true,
  Map:true}` (ListRef/MapRef structural metadata) — these were the top-level
  `TextInfo`/`ListRef`/`MapRef` flags in jsonicjs. Plus `List:{Pair:true,
  Child:true}` and `Map:{Child:true}` (typed list/map syntax like `[:String]`
  and `{:String}`), `Value:{Lex:false}` (raw values for custom processing).
- **RuleSpec API**: tabnas exposes rule alternates/actions through accessor
  METHODS (`AddOpen`/`ClearOpen`/`OpenAlts`, `AddBO`/`AddBC`/`ClearActions`,
  …), not the exported `Open`/`Close`/`BO`/`BC` slice fields jsonicjs had.
  `grammar.go` wraps these in `setOpen`/`prependOpen`/`setBO`/… helpers so the
  rule definitions still read as field assignments. `RuleDefiner` takes a
  second `*Parser` arg: `func(rs *jsonic.RuleSpec, _ *jsonic.Parser)`.
- **Custom lex matchers**: jsonicjs's `j.AddMatcher(name, priority, fn)` is
  gone; the `addMatcher` helper appends to `Config().CustomMatchers`
  (priority-interleaved against the built-in matchers using the SAME bands —
  match=1e6, fixed=2e6, …), preserving exact ordering.
- **val coalescer gotcha (CRITICAL)**: tabnas's val rule coalesces the val's
  value from `r.Child.Node` after a close-pushed child completes (`@val-bc`
  re-run), and `@val-ac` restores the primitive token value when the child
  left its node undefined. So a rule pushed from `val.Close` that folds into
  its PARENT's node (dotchain, arrowfold, angle) MUST also mirror the result
  onto its OWN `r.Node`, or the fold is dropped. Dotchain additionally pushes
  one segment per `val.Close` (not an `R:"dotchain"` self-loop) so `r.Child`
  tracks the latest segment.
- **Custom tokens**: `(`, `)`, `.`, `;`, `` ` ``, `${` are registered via
  `j.Token()` so jsonic lexes them as separate fixed tokens even when
  adjacent to text.
- **Grammar rules**: The `"val"` rule is extended with `j.Rule()` to handle
  parens, semicolons (aliased to "end"), dot operators, and template strings.
  Parens push to a custom "paren"/"pelem" rule pair that collects items into
  a `parenGroup`. At the top level, paren groups expand to engine markers
  `( ... )`. In data context (map values), they become `ParenExpr` values
  for inline evaluation by `autoEvalMap`. The `.` operator is a plain
  `#DT` fixed token and jsonic discards inter-token whitespace, so
  spacing around it is irrelevant: `foo.bar`, `foo . bar`, `foo. bar`,
  and `foo .bar` all lex to the identical `foo` `.` `bar` token
  sequence and parse the same. There is **no** adjacency /
  source-position analysis (that belonged to the removed hand-rolled
  lexer — jsonic is now the sole parser).
- **Template string interpolation**: Backtick is removed from jsonic's
  `StringChars` so it is not consumed by the built-in string matcher.
  Instead, `` ` `` (#BT), `${` (#IS), and template literal text (#TL) are
  handled by custom tokens and grammar rules:
  - A `LexMatcher` (priority 1M, registered via `addMatcher`) checks
    `rule.K["boru_tpl"]` to produce #TL tokens for literal text segments only
    inside template strings.
  - `"interp"` rule: opened by #BT in val, sets `K["boru_tpl"]` in BO,
    collects parts into an `interpGroup`.
  - `"ielem"` rule: matches #TL (literal text) or #IS (interpolation start).
  - `"iexpr"/"ieval"` rules: collect expression values between `${` and `}`.
    `iexpr` clears `K["boru_tpl"]` and increments `dlist`/`dmap` so
    expressions parse normally without template literal interference.
  - Nesting works to any depth since each `iexpr` pushes to `val` which
    can match another backtick and open a fresh `interp` rule.
  - `convertInterpGroup` converts the jsonic output to engine
    `InterpString` values (or plain strings if no interpolations found).
- **Number wrapping**: A `j.Sub()` callback wraps floats containing `.` in a
  `numberVal` struct to distinguish integers from decimals at parse time.

## Parser Customization

The parser converts jsonic output to engine values through two semantic contexts:

- **Word context** (top level, lists): unquoted text → words (callable),
  quoted → strings. Lists created in word context are marked `Eval=true`
  for auto-evaluation at end of execution.
- **Data context** (inside maps): unquoted text → words (executable),
  quoted text → strings, `true`/`false` → booleans,
  paren groups → `ParenExpr` (inline evaluation). Type names are NOT
  resolved in either context — the parser is type-name-opaque
  (ADR-012 rule 4): a capitalised name stays an ordinary Word and the
  engine's canonical cascade (`eng/go/resolve.go` — def table → live
  builtin table → type path) resolves it at consumption, exactly as
  user-defined type names have always resolved. Quotation meaning is
  consumption-time (`quote Integer` → `Integer/q`).

Key conversion functions in `parse.go`:
- `convertTopLevel()` / `convertTopLevelValue()` — word context
- `convertDataValue()` / `convertMapData()` — data context (atoms, not strings)
- `convertWordList()` / `convertDataList()` — lists (word context, Eval=true)
- Dotted access — there is no parse-time "expansion" pass. `.` is lexed
  as a separate token (`#DT` in `eng/go/parser/grammar.go`); the parser
  builds a `Reach` value that `lowerReach` (`eng/go/engine.go`) lowers to a
  `dot` / `dotr` token chain (`!` before `.` selects `dotr`). Chained access
  `m.a.b` becomes the token sequence `m dot a dot b` and composes at runtime
  because each `dot` produces the receiver for the next.
  - **`dot` / `dotr` vs `get` / `getr` (CRITICAL).** The accessor family
    splits by how the key is supplied: `dot` / `dotr` (the `.` / `!.`
    sugar targets) **quote** a bare-word key as a literal field name
    (`m.a` ≡ `m dot a` reads field `"a"`), while `get` / `getr`
    **evaluate** the key (`lst get i` reads the *value* of `i`;
    `m get "a"` uses the string). `get` / `getr` therefore carry NO
    QuoteArgs atom sigs — a bare-word `get` with an unbound name is an
    `undefined_word` error. Both pairs share one signature set
    (`accessorGetSignatures` / `accessorGetrSignatures` in
    `native_storage.go` / `native_accessor.go`); `get` / `getr` use the
    `dropQuoteAtomSigs` subset. Every compiler/checker/VM fold site that
    special-cases the family routes through `isGetWord` / `isGetrWord`
    (`eng/go/get_words.go`) so `dot` folds identically to `get` (no
    compiled-coverage loss for dot-access).
- A `/` modifier on a paren / dotted-path result applies to the WHOLE
  group, not the last key: `a.b/m` parses as `(a dot b)/m`.
  `convertTopLevelItems` strips the modifier off the final key (or a
  standalone `/mod` token after the group) and places result-level
  tokens around the group. `/u /s /f /N` desugar to the higher-order
  **words** `usurp` / `stack-args` / `forward-args` / `force-arity N`
  (each returns a NEW function, like `usurp`); they are emitted BEFORE
  the group so they forward-collect the result before it can
  auto-dispatch (critical for `/s`, whose args sit on the stack). `/v`
  and `/q` emit an `eng.NewDispatchMod` marker (`Word/__DM`) AFTER the
  group; `execFnDefLiteral` peeks+consumes it to leave the function as
  inert data, and `stepLiteral` drops an unconsumed marker (a modifier
  on a non-function result is a no-op). See `lang/spec/path-modifier.tsv`.
  `/t` on a WORD is the type-bound sugar: `X/t` desugars in `parseWord`
  to the paren group `(Type of [X/q])` — the bound rides as an ATOM so
  `of` sees the NAME and can bind a named structural type (disjunct,
  predicate) to its minted node rather than its body. It combines with
  no other modifier and is word-only (no group form; computed bounds
  spell `(Type of [expr])` directly). The bounded-Type body itself
  lives in `eng/go/core_boundedtype.go`.

## Argument Ordering (CRITICAL)

### The rule

Post §1.4 unification, dispatch is governed by **one** rule applied
to every signature, regardless of whether the word was historically
"forward-collecting" or "stack-only":

> Each `Signature` declares a boundary `BarrierPos` (the position of
> the `|` marker). Args at sig positions `[0..BarrierPos-1]` may be
> collected from forward tokens (in source order) or fall back to
> the stack. Args at sig positions `[BarrierPos..N-1]` always come
> from the stack. Stack consumption is always **top-down**: sig[i]
> reads the top of the stack, sig[i+1] reads the next-deeper, etc.

`BarrierPos == 0` means "all stack" (legacy stack-only).
`BarrierPos == N` means "all forward-eligible" (legacy forward-prec).
`0 < B < N` mixes the two: forward fills the leading B positions then
stack fills the rest.

### Surface conventions (STYLE-GUIDE.md §S2)

Where the split falls is the only thing a call form chooses. Two
conventions:

**Infix form for words read as infix operators** — `add`, `sub`,
`mul`, `div`, `mod`, `pow`, `and`, `or`, `lt`, `lte`, `gt`, `gte`,
`eq`, `neq`. Write `1 add 2`, `10 sub 3`, `n lte 1`: the left
operand comes off the stack, the right one is written forward.
Binary handlers compute `args[1] OP args[0]` precisely so this
split reads as written.

**Forward form `f a b c` for everything else** — every other word,
at every arity. Reach for it in new code, examples, REPL
transcripts, and documentation:

- It reads naturally — written argument order matches declared
  param order. `def f fn [[a:Integer b:String] …] f 1 "x"` binds
  `a=1, b="x"`.
- The forward phase fills `sig[0..N-1]` directly with `[a, b, c]`,
  so the handler's `args[i]` mirrors what the user wrote at
  position `i+1` after the word.
- It composes cleanly with paren grouping: `(f a b) other-thing`
  contains the call.

Stack form (`c b a f`) and the intermediate splits (`c b f a`,
`c f a b`) all dispatch to the same `sig=[a,b,c]` — but written out
they read back-to-front, because the stack is consumed top-down.
Use them where an enclosing pipeline has already left the arguments
on the stack.

`a f b` is **not** a special two-arg form and is **not** a "swap
form" — both phrasings are legacy misunderstandings and must not
reappear in code, comments, or docs. It is the ordinary rule with
the split at k=1: `b` → sig[0] (the single forward arg), `a` →
sig[1] (the stack top). It differs from `f a b` for exactly the
reason `c f a b` differs from `f c a b`: moving an operand across
the word moves it to a different sig position.

### One arg-flow convention everywhere

After matchSignature picks sig and positions, args flow through
every downstream handoff in **sig order with no reordering**:

- Kernel native handlers (`func(args, …) ([]Value, error)`) —
  `args[i]` is sig position i.
- boru `def fn […]` body via `InstallFnDef`'s registered handler —
  named params bind by name, `args[i]` matches the i-th declared
  param; unnamed params push to body tokens in i-order so body
  position `i` from the bottom holds `args[i]`.
- `CallBoru` and `execFnDefSig` — same: named params bind by name,
  unnamed params push in i-order.
- `args.N` boru accessor — returns `args[N]` directly.

No reversals, no swaps, no permutations between matchSignature and
the handler. The only "reordering" anywhere in the kernel is
`rearrangeForForward`, which is part of how forward-collected args
get laid out so that matchSignature's top-down walk produces the
right sig — it's matchSignature internals, not a separate hop.

Module FnDef wrappers (`makeXxxFnDef` helpers) get a special
short-circuit in `execFnDefLiteral`: trivial single-word delegation
bodies (`[Word(inner-name)]`) dispatch the inner native directly via
`execMatch`, skipping CallBoru entirely. See
`design/SIG-ORDER-REFACTOR.10.md`.

### The unified algorithm

For each candidate `sig`:

1. **Forward phase** — walk sig[0..forwardLimit-1] in order. At each
   step, take the next future token and check it against the next
   sig type. Stop on a structural boundary (open paren, end, function
   word) or on type mismatch — the remaining sig positions then come
   from the stack.

2. **Stack phase** — walk the remaining sig positions in order, top
   of stack first. sig[fwd] = stack top, sig[fwd+1] = next-deeper, etc.

The handler always sees args in sig order: `args[0]` is whatever
matched sig[0], regardless of whether it came from a future token or
the top of the stack.

### Examples

For `def f fn [[A B C] Any [...]]` (no boundary, all forward-eligible),
all of these forms call `Fimpl(a, b, c)`:

```
f a b c     → forward [a,b,c]                        → sig=[a,b,c]
c f a b     → forward [a,b], stack top=c             → sig=[a,b,c]
c b f a     → forward [a],   stack top=b, deeper=c   → sig=[a,b,c]
c b a f     → forward [],    stack top=a, …, c       → sig=[a,b,c]
```

For `def g fn [[A B | C] Any [...]]` (boundary at position 2), only
forms that put `c` on the stack are valid:

```
c g a b     → forward [a,b], stack top=c   → sig=[a,b,c]
c b g a     → forward [a],   stack top=b…c → sig=[a,b,c]
c b a g     → forward [],    stack top=a…c → sig=[a,b,c]
```

For `def h fn [[| A B C] Any [...]]` (boundary at 0, legacy stack-only),
only the all-prefix form matches:

```
c b a h     → forward [], stack top=a, deeper=b, deepest=c → sig=[a,b,c]
```

### Non-commutative two-arg sanity check

With `sub` declared as a 2-arg forward-eligible word — handler computes
`args[1] - args[0]` (post-§1.4 phase 4: every binary math handler
computes `b op a` so the infix split reads as written):

```
sub 10 3    → forward [10,3]                  → sig=[10,3] → 3-10 = -7
3 sub 10    → forward [10], stack top=3       → sig=[10,3] → 3-10 = -7
3 10 sub    → forward [],   stack top=10…3    → sig=[10,3] → 3-10 = -7
10 sub 3    → forward [3],  stack top=10      → sig=[3,10] → 10-3 =  7  (infix — house style)
10 3 sub    → forward [],   stack top=3…10    → sig=[3,10] → 10-3 =  7
```

The first three are one split-set (`f a b ≡ b f a ≡ b a f`); the last
two are the other (`a f b ≡ a b f`). They differ because they put a
different operand in sig[0] — not because two-arg calls follow a
different rule. The phase-4 handler convention is chosen so that the
infix spelling is the one scanning left-to-right, `10 sub 3 = 7`,
which is why infix form is house style for these words
(STYLE-GUIDE.md §S2).

### Implementation

The matcher in `core/go/engine.go::MatchSignature` runs a single
loop over sig positions. Forward limit comes from `sig.BarrierPos`,
overridable per call site by `/s` (force stack: limit=0) or `/f`
(force forward: limit=N). When forward args have to be collected
across paren / nested-forward boundaries, `rearrangeForForward` lays
the collected values out so the post-collection retry sees them
top-down in sig order.

## Lambda Syntax (`=>` / `afn`)

`=>` is a parser token (#AR) that GROUPS: `A => B` parses as the
paren group `(A afn B)` — the arrow binds tighter than any enclosing
word's forward collection, so `def double x:Integer => [x mul 2]`
binds the LAMBDA, not the sig literal (without the fold, def's Any
slot claimed the sig at plan time and the lambda evaporated as an
orphaned anonymous value). The fold is grammar-level
(`setupValRule`'s arrowfold/arrowfoldelem rules plus the
implicit-pair #AR Close alternates in `setupPairGrammar`); `afn`
itself is a regular registered word — there is no lambda type, no
separate runtime path, and typing the word `afn` directly keeps the
old flat (ungrouped) behaviour.

`afn` has signature `[Any Any |]` (both args forward-eligible, both
typed `Any`, body and sig captured via `NoEvalArgs`). The canonical
call form is the infix split `input afn body` (i.e. `input => body`),
mirroring the boru `args[1] op args[0]` reading convention — afn
collects the body as its forward arg and the input sig from the
stack.

The handler:
- auto-wraps a non-list input into `[input]` (same convention as
  `fn`);
- parses params via the shared `eng.ParseFnParams` so every fn
  abbreviation (`name:Type`, `{name:Type}`, bare type words, value
  patterns, `?`, `|`, `__SB`) works in afn;
- auto-wraps a non-list body into `[body]`;
- builds a single-sig `FnDefInfo` with `Returns=[Any]` and
  `Anonymous: true`, and returns a `Function` value indistinguishable
  from `fn`'s output except for the flag.

The `Anonymous` flag on `FnDefInfo` is read **only in check mode**.
Anonymous lambdas have a deliberately conservative static
`Returns=[Any]`; in check mode the dispatch path
(`execFnDefSigStackMatch` for direct invocation,
`InstallFnDef`'s ReturnsFn for `def name (lambda)` installations)
runs `eng.AnalyseFnBody` against the bound carrier args and lets the
analyser's residual stack carry the inferred return type forward.
Normal execution doesn't read the flag; the body runs the same way
a named fn's body does.

### Syntactic gotchas

- **Bare typed-param shorthand takes ONE param.** `x:Integer => body`
  parses everywhere (parens optional — top level, paren contents,
  def operands): the implicit pair closes on the arrow and becomes
  the input sig, equivalent to `[x:Integer] => body`. Multi-param
  needs the bracket form `([x:Integer y:Integer] => …)`: in the bare
  form each pair closes separately and the first strands as a stray
  map (`syntax.tsv` pins the `undefined_word` failure). An explicit
  map sig `{x:Integer} => body` is NOT the named-param form —
  ParseFnParams keys named params on jsonic's Implicit flag, so an
  explicit map builds a single Map-typed param (`fn (Map)`).
- **Single-value body rule.** afn captures one forward token as the
  body. Multi-token bodies must wrap as `[token1 token2 …]` or
  `(token1 token2 …)` — both are CODE, captured unevaluated (the body
  slot is NoEvalArgs for lists and RawParens for parens) and run per
  call with params bound, so `x:Integer => (x mul 2)` works. A
  bare-word body (e.g. `[x:Any] => x`) fails because the engine
  dispatches the word as it walks past it during forward collection —
  wrap as `[x]` to keep the word as data inside the body list.
- **Currying just chains.** `x:Integer => y:Integer => [x add y]`
  parses right-associatively as `(x ⇒ (y ⇒ body))`, and because the
  body paren is raw the inner lambda constructs INSIDE the outer body
  — at call time, with `x` bound and captured. The older list-wrapped
  spelling `x:Integer => [(y:Integer => [x add y])]` remains
  equivalent.
- **Single sig only.** `=>` produces exactly one `FnSig`. For
  multi-overload fns, use the verbose `fn [[input1] [output1] [body1]
  [input2] …]` form.

## Closures and Capture

boru fns and lambdas use **implicit lexical capture**. At fn-construction
time the engine walks the body's bare-Word references; any name that
resolves to a binding made by an **enclosing fn** (a param or local
def of an outer fn currently executing) is snapshotted into the
`FnDefInfo.Captured` list. At dispatch the captured names are installed
as defs in the per-call scope, alongside named params, so the body
sees its captures regardless of what happened to the outer scope after
construction. The mechanism is the standard "factories return inner
fns that retain factory state" pattern:

```
def make-adder fn [[x:Integer] [Function] [fn [[y:Integer] [Integer] [x add y]]]]
def add5 (make-adder 5)
add5 3                              # → 8   (captured x=5 lives in add5)
add5 10                             # → 15
```

The same works via `=>`:

```
def make-adder ([x:Integer] => [([y:Integer] => [x add y])])
def add5 (make-adder 5)
add5 3                              # → 8
```

### What gets captured

| Reference resolves to… | Captured? |
| --- | --- |
| Enclosing-fn param | **Yes** |
| Local def inside enclosing fn body | **Yes** |
| Capture of a yet-further-outer fn | **Yes** (transitive) |
| Module-level / global def | **No** (stays dynamic) |
| Kernel word / native registration | **No** (registry lookup at call) |
| Name unbound at construction time | **No** (forward ref → call-time lookup) |

Concretely:

- **Module-level dynamic.** `def x 1; def f ([y:Any] => [x add y]); def x 2; f 0` returns 2. `x` lives at module scope, the closure doesn't capture it, and the runtime lookup sees the new binding.
- **Recursion via forward ref.** `def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]` works because `fact` isn't bound when its body is parsed — it's a forward reference resolved via the registry at call time. Standard recursive-fn idiom is unchanged.
- **Inner shadows outer.** A param or capture with the same name as a module-level def shadows the module reference inside the body — innermost binding wins.

### Capture semantics: shallow snapshot

The captured `Value` is a struct copy. Pointer-backed payloads (`Map`,
`Store`, `Array`, `Timer`, `Interval`) **share the underlying state**
with the outer binding, so mutations cross the capture boundary:

```
def pair (fn [[] [List] [
  def m {a:1}
  [([] => [m]) ([k v] => [m set k v])]
])
(pair get 1) 'b' 2                  # mutate via second lambda
(pair get 0)                        # → {a:1, b:2}   (shared OrderedMap pointer)
```

Reassignments (`def n new-value`) **don't** affect captures, because
the capture snapshotted the prior Value.

### `args` stays dynamic

`args` is a registered native word bound by the per-call args-stack
push, not by `def`. Inside a captured fn, `args.N` refers to that fn's
own call args, not the surrounding scope's — same as `this` in JS
lambdas. Don't expect to capture caller args; pass them explicitly.

### Implementation notes

- `FnDefInfo.Captured` is `[]CapturedBinding{Name, Value}`, sorted by
  name for deterministic install order. Nil for top-level
  constructions and inner fns whose body references only params,
  module-globals, or forward refs.
- Captures are computed in `eng.ComputeCaptures` (eng/go/fn_capture.go)
  via the body walker `WalkBodyWords`, which skips quoted lists,
  `/q`-atoms, nested `FnDefInfo` payloads (those are inner closures
  with their own analysis), and engine markers.
- The "enclosing-fn baseline" — the def-depth snapshot at each
  currently-active fn entry — lives on `Registry.FnBaselines`. Pushed
  in lockstep with the per-call args-stack push and the existing
  body-local-def cleanup snapshot; popped by `__pa`'s handler at body
  exit and by `CallBoru`'s inline cleanup.
- Captures and named params are installed via `InstallFrameBinding`
  (not `InstallDef`) — captures BEFORE params so params shadow same-
  named captures. `InstallFrameBinding` SHADOWS (pushes a fresh
  DefStack level) where `InstallDef` (the `def` word) runs the same-
  scope overlap-redefinition filter: a callee param whose name and
  signature collide with a live caller binding (the classic comparator
  threaded through a `/v`-parked Function arg) must stack on top and
  tear back down to the caller's level, not destroy it at install time.
  Cleanup is the existing `DefCleanup` + `undef` tail in the
  synthesized body; capture names are appended to the cleanup list at
  install time so they tear down uniformly. See
  `design/legacy/ACCESSOR-SPLIT-AND-CLEANUP-BUG.ignore`.

### Sharp edge: 0-arg lambdas as values vs as calls

> **ADR-016 made this a DEFECT, not a design — and the OBSERVABLE half is
> fixed (2026-08-21).** Arity and origin never change how a function
> behaves, and this gate keyed on both. Where that was reachable — an
> explicit `apply` — it is settled: `apply` MARKS a fn whose only
> signatures are 0-arg (`FnDefInfo.Applied`) and the ordinary re-step
> dispatches it, so `def f ([] => [42])  f/v apply` answers `42` exactly
> as its named twin does. See `design/FN-VALUE-OPEN-WORK.0.md` §5 Hole 1;
> pinned in `lang/spec/valof.tsv` §5, whose heading ("Anonymous-function
> references behave identically") was the one claim in that file which did
> not hold at arity 0.
>
> **Mark, do not call — the seam is the lesson.** The first fix called
> from `applyHandler` via the since-retired `CallBoruFn`, which made apply a SECOND dispatch
> path, and it diverged from the re-step three ways: a native 0-arg fn lost
> its Go handler to an empty boru body, a body's `context` mutations landed
> in a sub-engine instead of the caller's frame, and the check pass could
> not see the call at all (so the shape had to be declared uncompilable).
> Marking routes every arity down the one path and all three go away —
> the shape now COMPILES. If a future change needs apply to do more, add
> to the mark, not to a second path.
>
> **The gate itself stays, deliberately.** Removing it was measured and is
> wrong: `/v` parking collapses (`f/v` answers `42`), and a 0-arg lambda
> stored in a map or list stops being data. The gate is what holds a 0-arg
> lambda as a VALUE wherever one is HELD rather than called — which is what
> `def f ([] => [body])` and every container slot rely on. So the paragraph
> below still describes the code; read it as the parking rule it is, not as
> an origin-keyed dispatch rule. Do not add a second exception in its shape.

An anonymous Function value sitting on the stack with **no args
available** does NOT auto-invoke — it stays as data, which is what
`def f ([] => [body])` relies on (def receives the Function, binds f
to it). Once `f` is then referenced as a Word, stepWord finds the
registered FnDef and dispatches normally (fires the 0-arg sig,
runs body). The "anonymous value vs named call" distinction is
gated in `execFnDefLiteral`: anonymous + no positions matched →
treat as data; otherwise dispatch.

## Quotation System

Lists are **evaluated by default**: `[1 add 2]` → `[3]`. Auto-evaluation
happens in two contexts for parser-created lists (`Eval=true`):

1. **When consumed as a word argument**: `execMatch` (for registered words)
   and `execFnDefSig` (for FnDef auto-invocation) run `autoEvalList` on
   list arguments with `Eval=true`, resolving word elements via the def
   stack (`r.TopOfDefStack`).
   For example: `def c1 10  def c2 20  [c1 c2] myword` passes `[10, 20]`
   to `myword`, not `[atom(c1), atom(c2)]`.

2. **When unconsumed on the stack at end of Run()**: `autoEvalStack` runs
   `autoEvalList` on remaining lists with `Eval=true && !Quoted`.

The `quote` word (forward arg collection) prevents evaluation:
- `quote [1 add 2]` → `[Integer(1), Word(add), Integer(2)]`
- `quote a` → `Atom(a)` (words become atoms)
- `quote 99` → `99` (scalars unchanged)

Quotation is **implicit** for code-body positions via `NoEvalArgs`:
- `fn` body: function definition bodies
- Control words: `if`, `for` branches/bodies
- Higher-order words: `each`, `fold`, `scan`, `outer`, `inner` code-body args
- `do`, `call`, `module`, `var`: list bodies executed as sub-programs

The `NoEvalArgs` field on `Signature` marks arg positions where list
auto-evaluation is suppressed. Unlike `QuoteArgs`, it does NOT affect
forward collection or word→atom conversion — it only prevents
`autoEvalList` from running in `execMatch`. Map auto-evaluation
(`autoEvalMap`) is NOT affected by `NoEvalArgs`.

Implementation: parser sets `Eval=true` on lists. `execMatch` runs
`autoEvalList` on consumed list args unless `NoEvalArgs[i]` is set.
`quote` sets `Quoted=true` (also suppresses auto-eval). End-of-`Run()`
auto-evaluates only lists with `Eval=true && !Quoted` that were never
consumed.

### `def name <node>` binds a value; `word` splices

`def` does NOT use `NoEvalArgs` on its body: a list body is evaluated
and bound as a value, exactly like a map body. `def xs [1 add 2]` binds
`xs = [3]`; `def m {a: (1 add 2)}` binds `m = {a:3}`. Both are early
snapshots (inner def-words resolve at def time).

The old implicit splice (`def double [dup add]` expanding its body at
every reference, Forth-style) is now the explicit **`word`** form, which
wraps a value in an `__SP` splice marker (`eng.NewSplice`, type
`Word/__SP`). When the marker reaches the engine pointer
(`stepLiteral`) it is replaced, unevaluated, by its payload — a plain
list contributes its top-level elements, any other value contributes
itself — and the result is re-stepped against the live stack:

- `word [1,2,3]` → splices `1 2 3`; `def doub word [dup add]; 5 doub` → 10.
- `def name word value` binds the marker (a `word`'d body is collected
  by value via the `TAny` match, so it does not fire during collection).
- The marker also splices inside containers that get evaluated
  (e.g. a `word` reference inside a list literal).

**Splice is forward-form only — there is no prefix-form splice.** An
`__SP` fires the instant it is *stepped at the pointer*; the only place
a stack value is not stepped is when it is collected as a forward
argument. So `def name word [body]` works (the marker is `def`'s
forward arg, skipped by the pointer), but `[body] def name` (prefix /
stack form) puts the body on the stack *before* `def`, where the
pointer steps and fires it before `def` consumes it — a prefix list
body therefore binds the list **value**. `quote` cannot ferry the
marker past this: `quote (word …)` fires the `__SP` inside the paren
before `quote` marks it, and `quote word …` captures the literal token
`word` as an atom. Don't re-attempt a prefix-splice path; use the
forward form.

To add new syntax: register a token with `j.Token()`, extend the `"val"`
rule with `j.Rule()` (definer `func(rs *jsonic.RuleSpec, _ *jsonic.Parser)`,
mutating via the `setOpen`/`prependClose`/`setBO`/… helpers), and add
conversion logic in the appropriate `convert*` function. For context-sensitive
lexing, use the `addMatcher` helper with the rule-aware `LexMatcher` signature
`func(lex *Lex, rule *Rule) *Token` to read `rule.K`/`rule.N` maps.
See the template string interpolation rules for a complete example.

## Module namespaces + the Module descriptor

`import` binds each `export "Name" {…}` to a **PLAIN Map** of the raw
exports carrying the kernel's **module-namespace facet**
(`eng.ModuleNSInfo`, a nil-by-default pointer on `Value` — NUR038
retired the former `Ideal/ModuleExport` wrapper type; FixedID 5001 is
retired, never recycled). Exported values are exactly what the module
exported — a fn stays a `Function`, a constant stays that constant, a
type stays a plain type — so `typeof MathUtil → Map` and ordinary map
words (`keys`, `size`, `each`) work directly on a namespace. All of a
module's namespaces share one **`Ideal/Module`** descriptor
(`native.TModuleInst`), reachable via the synthetic `$module`.

- `NewModuleNamespace(name, fields, module)` —
  `eng.WithModuleNS(NewMap(fields), name, module)`: `name` (→ `.$name`),
  the module's own export `*OrderedMap` (shared, not copied — runtime
  export growth and the kernel growth ledger key on the pointer), and
  the owning Module. The namespace dispatches the ORDINARY map
  `get`/`getr`/`has`/`dot`/`dotr` sigs; the facet-only behaviours hook
  in via `moduleNSGetSynthetic` / `moduleNSGetReturns` /
  `moduleNSGetrReturns` / `moduleNSGetrMiss`
  (`native_module_types.go`): the `$module`/`$name` synthetics, the
  module-flavored `not_found` ("export … not found in module"), and the
  check-mode raw-export resolution (a fn export keeps its `FnDefInfo`;
  a missing key stays strict Any because the keyspace can grow).
  A namespace renders compactly as `Module(Name){key key …}`
  (`eng/go/coretype_list_map_behaviors.go`).
- `NewModuleInstance(desc)` — the descriptor (`ExtensionPayload`
  boxing a `*ModuleDesc` — the pointer is the descriptor's eq/deq
  IDENTITY, per-import-instance, NUR031); `name`/`kind`/`file`/
  `folder`/`exports` are read via `get`. `ModuleDesc.{Ref,Kind,File,
  Folder}` are populated by `Resolve` (native), `loadFileModule`
  (file), and `RunModuleBody` (inline). FixedID: Module 5000.

`module […]` itself produces an `Ideal/Module` and `import` consumes it —
so `typeof (module […]) → Module`. `AsModuleDesc` unwraps the
`ModuleDesc` for hosts.

See `lang/spec/module-instance.tsv` + `test/module_instance_test.go`.

## Module FnDef Wrappers — inner sig BarrierPos (CRITICAL)

Native modules under `lang/go/modules/` follow a sub-registry
pattern: `BuildXxxModule` creates a fresh `subReg`, registers the
inner natives there, and exports FnDef wrappers (carrying
`Registry: subReg`) into the module's export map. When the
wrapper is invoked via `pkg.word` dot-access, dispatch flows
through `eng/go/engine.go::execFnDefLiteral`, which calls
`reg.Lookup(fnDef.Name)` and uses the **looked-up native's
`Signatures`** for `matchSignature` — NOT the wrapper's own
`Signatures`.

**Consequence:** the inner native's `BarrierPos` controls whether
the wrapper dispatches under the infix split `a pkg.word b`.

- If the inner sig has `BarrierPos: 0` (stack-only),
  matchSignature requires every arg on the stack. `a pkg.word b`
  has 1 stack + 1 forward, so dispatch fails silently — the
  FnDef just sits on the stack with the args around it.
- If the inner sig has `BarrierPos: -1` (all-forward eligible) or
  any positive boundary, matchSignature accepts that forward+stack
  split, runs insertForward, completes collection, and dispatches
  normally.

**Rule:** inner natives registered into a module sub-registry
MUST use `BarrierPos: -1` (or a positive value that includes
position 0 as forward-eligible). At runtime the body context
has no forward tokens anyway — `BarrierPos: -1` and `0` both
fall to stack-only — so the change is dispatch-only and safe.

Regression test:
`lang/go/modules/wrapper_dispatch_test.go::TestModuleWrapperInnerSigBarrierPos`
asserts both directions (the broken case explicitly demonstrates
the silent-dispatch-failure mode).

### Wrapper FnSig.Params order

`FnSig.Params` on a module wrapper MUST be declared in the same
order as the inner native's `Signature.Args` — **top-first, sig
order**: `Params[0]` describes the type of the value at sig
position 0 (= top of stack under matchSignature).

This is uniform with everywhere else in the kernel — there is
ONE convention. The wrapper's `Params` types are only used for
the static analyser and user-facing display; at runtime,
`execFnDefLiteral` detects the trivial-delegation shape
(`Body=[Word(fnDef.Name)]`, all-unnamed Params) and calls
`execMatch` on the inner native directly via the matched sig.
No body execution, no token splicing, no push reordering.

boru fns defined inside a module preamble (named params + real
body) take a different path: their body runs via `CallBoru` in the
captured sub-registry so module-private words resolve correctly. Named params bind via `InstallDef`, so
push ordering doesn't apply.

Test: `lang/go/test/sig_order_guard_test.go` pins this convention.

## Registry Bindings (CRITICAL)

Post the TYPE-UNIFORM Phase 4 collapse the `Registry` holds **one**
per-name binding store, `r.Defs` (a `*DefTable`). It carries both
kinds of binding:

- **value bindings** — `def x 1`, fn bodies, fn-body parameters,
  carrier-merge join points, module imports.
- **type bindings** — `def Foo Integer` (a capitalised name). The
  `DefEntry` additionally carries the declaration's lattice `*Type` in
  `DefEntry.TypeDef` plus `Minted bool` — true when this binding
  minted the node, false when it ADOPTED an existing canonical one
  (the alias `def Foo Integer` adopts the Integer node itself).

The capitalisation convention keeps the two kinds of name disjoint
(`Foo` is a type, `foo` a value), so one map suffices. Each name maps
to a shadowing stack; `def x 1; def x 2` → `x` is 2; `undef x` → `x`
is 1.

`r.Types` (a `*TypeTable`) is **not** a binding store — it is the type
*lattice*: it mints type identities (`MintType`), indexes them by ID
(`LookupByID`), retires them (`Retire`), and holds the static builtin
name index (`LookupBuiltinByName`).

**DefTable methods** (`r.Defs.*`):
- reads — `Top(name) (Value, bool)` (what the binding DENOTES: a
  value binding's Body, a **type** binding's lattice NODE — the Stage
  2 flip of design/legacy/TYPE-REPRESENTATION.1.ignore), `TopEntry(name)
  (DefEntry, bool)` (the raw entry: Body + TypeDef + Minted),
  `Has(name)`, `IsType(name)`, `Depth(name)`, `Stack(name) []Value`,
  `Names()`.
- writes — `Push(name, v)` (value binding),
  `PushType(name, def, body)` (type binding),
  `PushTypeAdopted(name, def, body)` (a type binding that ADOPTS an
  existing canonical node — the alias path; `undef` never retires
  it), `Pop(name) bool`,
  `PopEntry(name) (DefEntry, bool)`, `Replace`, `Truncate`, `Delete`,
  `Set`.
- snapshot/restore — `Snapshot() map[string]int` / `Restore(snap)`,
  depth-based; pair around fn-body sandboxes and carrier-merge joins.
  (The predicate sandbox additionally clones `r.Types` to roll back
  lattice mints — see `snapshotPredicateState`.)

**Registry resolution helpers**:
- `r.ResolveTypedName(name) (Value, bool)` — single-store lookup; the
  canonical way to resolve a type-context name. For a type binding it
  yields the lattice NODE (never the declared body — every kind, the
  Stage 2 flip); read the structure with `core.TypeContentOf` or
  `r.TopTypeBody`.
- `r.TopTypeBody(name) (Value, bool)` — the body when name's active
  binding is a *type* binding (false otherwise).
- `r.LookupTypeName(name) *Type` — the active lattice type for name:
  a dynamic binding's minted def, or an external builtin.

`undef` is the universal unbinder: a capitalised name pops the type
binding and retires its lattice node ONLY if this binding minted it
(`DefEntry.Minted` — an adopted alias node survives); a lowercase
name pops a value binding.

## Helper API discipline

The engine consolidates several distributed implicit contracts behind
helper APIs in `eng/util.go` (re-exported through `native/aliases.go`).
Use the helpers rather than
the underlying state. Adding direct field access regresses the
consolidation and will be flagged in code review.

**Concrete-value guards** (panic-prevention, type-literal vs concrete):
- `IsBareTypeNode(v)` — true if `Data == nil` and not a carrier (v IS
  its own lattice node). INCLUDES None/Any/Never; use for naming,
  ordering, and lattice-identity dispatch.
- `IsTypeLiteral(v)` — `IsBareTypeNode` minus None; use when v may be a
  type **constraint** (None is treated as a value, not a type). Never
  key constraint HANDLING on node-bareness alone: since the Stage 2
  flip every named type evaluates to a bare node, and a kind that
  enforces membership through a Unifier (DepScalar, predicate,
  disjunct, negation, FnUndef, binding-body) must be routed by
  `core.HasConstraintUnify`, or the constraint is never run.
- `TypeContentOf(v) (Value, bool)` — the type's declared structure: a
  bare node's `TypeBody` stamp, or v itself when it already IS type
  content. Call at entry in any handler that operates on a type's
  STRUCTURE (`make`, refine bases, describe/inspect schema views, the
  type-algebra words) so a name and an inline body are one case.
- `IsConcrete(v)` — true if `Data != nil` and not a carrier. NOT the
  negation of `Data == nil`: a list/map carrier has `Data != nil` yet
  is not concrete. Never write the raw `v.Data == nil` probe in
  consumer code — see eng/go/CLAUDE.md "Payload-presence vs value-mode".
- `RequireConcreteList(v, op) (ReadList, error)` — unwraps a list-typed
  Value or returns an error when the value is a type literal/carrier.
- `RequireConcreteMap(v, op) (ReadMap, error)` — same for maps.

Handlers that take `TList`/`TMap`/`TAny` args should guard with
`!IsConcrete(args[i])` (or use the `RequireConcreteX` helpers) before
calling `AsList(v)`/`AsMap(v)` — otherwise carriers and type literals
panic on `.Len()`. (`AsList` / `AsMap` are free functions in eng, not
methods on `Value`; the method surface is `Is`, `String`, the node-
content pair `TypeBody`/`SetTypeBody`, and the `AsConcreteX` handler
accessors below.)

**DepScalar-rejecting accessors**:
- `v.AsConcreteString()`, `v.AsConcreteInteger()`, `v.AsConcreteFloat()`,
  `v.AsConcreteBoolean()`, `v.AsConcreteAtom()` — reject DepScalar
  payloads with a clear error rather than silently returning the zero
  value. Always prefer these over the bare free-function accessors
  (`AsString(v)`, `AsInteger(v)`, …) when the arg comes from a
  sig-matched value (a `TString` slot can secretly hold a `DepString`
  constraint). These DepScalar-rejecting variants remain methods on
  `Value` since they're the canonical handler-side error path; the
  low-level accessors were drained to free functions in `eng/` as part
  of the type-decoupling work — see
  `design/legacy/TYPE-DECOUPLING.10.ignore`.

**Check mode**:
- `r.IsCheckMode()` — read-side helper. Replaces `r.Check.Mode` and
  the `r != nil && r.Check.Mode` nil-guarded variants.
- `r.BeginCheckMode() func()` — entry-side helper; resets per-pass
  state and returns a deferred-cleanup function. Use as
  `defer r.BeginCheckMode()()` in the analyser entry point.

**Error construction**:
- `r.BoruError(code, detail, word)` — handler-side error constructor;
  picks up `r.Source` automatically. Replaces the recurring
  `makeBoruError(code, detail, name, r.Source, "")` pattern.
- `r.BoruErrorHint(code, detail, word, hint)` — same with an explicit
  hint string. The engine-internal helpers (`signatureError`,
  `insufficientArgsError`, etc. in `engine.go`) layer above these
  with engine-specific source resolution.

**Args stack** (per-fn-call args list, used by the `args` word):
- `r.PushArgs(list)` / `r.PopArgs()` / `r.TopArgs() (Value, bool)`.
  The underlying `argsStack` field is unexported.

**Context stack** (scoped Store layers for `ctx-set`/`ctx-get`/etc.):
- `r.PushContext(parent)` — push a new copy-on-write child layer.
- `r.PushExistingContext(ctx)` — push an existing Store without
  wrapping (rare; used by module loading to inherit the parent's ctx).
- `r.PopContext()`, `r.Context()` (returns the data map),
  `r.ContextStore()` (returns the StoreInstanceInfo),
  `r.UpdateCtxStoreChain(orig, new)`.

**Canonical type pointers**:
- `CanonicalType(r, t) *Type` — resolve `t` to its canonical lattice
  node via `r.Types.LookupByID`. Use whenever a `*Type` may have come
  from `&v` of a by-value type-literal Value — `behave` Behavior
  installs and LCA-walk identity must reach the canonical pointer,
  not a stack-local copy. See
  `design/legacy/TYPE-CANONICALIZATION.10.ignore`.

**Typed-def reparent**:
- `ReparentValue(v, def) Value` — return a fresh copy of v with
  `Parent` rebound to def. The single primitive every typed-def
  reparent path uses (predicate / refine-bare / FnUndef branches).
  The by-value copy is explicit so Unify-swap results (which may
  return a type literal in the value position) can never be
  silently mutated and stored as the binding.

**Pos threading**:
- `WithPos(v, src)` — return v with Pos copied from src. Use when a
  handler constructs a new Value from an input — error reporting
  downstream then has the source location.

## Undefined Words (CRITICAL)

An undefined word reaching the pointer is an **error**, not a value.
A word that is not registered, not in the def stack, and not a known
literal (`true`/`false`/a type name) raises `[boru/undefined_word]` at
`stepWord`. There is no implicit `Word → Atom` fallback.

Names that are meant as data must be quoted:

- `foo/q` — word-suffix form: parses `foo` directly as `Atom(foo)`.
  This is the preferred short form and the canonical render output
  (canon.go emits `name/q` rather than `(quote name)`). Like the
  other suffix modifiers, `/q` stacks with `/s`, `/f`, and the
  argCount digit in any order (`foo/sq` ≡ `foo/qs`); when `q` is
  present the result is an Atom and the other modifiers are
  accepted syntactically but ignored.
- `quote foo` — function form: captures the upcoming Word as
  `Atom(foo)`.
- `(quote foo)` — same as `quote foo`, inside a paren so forward
  collection can pick up the resulting Atom.
- A `/q`-marked sig position — captures the upcoming Word as an Atom
  during forward collection (`def name body`, `get key map`,
  `set key val store`, etc.).

CheckMode is the single exception: `stepWord` keeps the lenient
"undefined → `Atom{Undefined:true}`" path so static analysis can
continue past a typo. Each dangling Undefined Atom is converted to a
diagnostic + `Any` carrier in the end-of-`Run()` drain in
`engine.go`.

When adding a sig that should accept a bare-word name as data, add `/q`
to the corresponding Atom position. Without `/q`, callers will see an
`undefined_word` error and must wrap the name in `quote` themselves.
boru-defined fns declare the same capability as `name:Atom/q` (or bare
`Atom/q` for an unnamed param) — ParseFnParams sets `FnParam.Quote`,
which flows into `Signature.QuoteArgs` at construction/normalizeSig,
so the dispatch-side behaviour (binding-agnostic word capture) is
identical to a native QuoteArgs slot.

## Value Comparison & Ordering

`cmp` / `lt` / `gt` / `lte` / `gte` / `sort` route through one total
order — see `design/TYPE-ORDERING.10.md` for the canonical
design. The kernel-side implementation lives in `eng/go/compare.go`
and `eng/go/compare_scalar_behaviors.go`; this section captures
what handler authors and word implementers need to know.

**The cascade** (per `CompareValues`):

1. **LCA Comparer walk** — per-family Comparers on `TNumber`,
   `TString`, `TBoolean`, `TAtom`, `TWord`, `TScalar` own same-
   family pairs. A Comparer can return `ErrNoComparer` to opt out
   (DepScalar values do this so numeric Comparers don't read
   `DepScalarInfo` as a zero float).
2. **Rank fallback** — `compareTypes` (Rank → depth → name → ID)
   settles cross-family pairs.
3. **Structural compare** — when types match, lists go length-then-
   element-wise, maps go length-then-sorted-keys-then-values,
   everything else falls to `CanonValue` lex.

**Type-literal-first rule.** Every family Comparer opens with
`litVsConcreteOrder(a, b)`: a bare type literal sorts strictly
below every concrete inhabitant in the same family. So `Integer
cmp 0 → -1`, `String cmp '' → -1`, `Boolean cmp false → -1`,
`Path cmp <any path> → -1`. Two type literals delegate to
`litVsLitOrder` → `compareTypes` so they order by lattice Rank
(`Number cmp Integer → -1`). The rule lives in per-family
Comparers and `comparePaths`; `scalarCompareBehavior` deliberately
does NOT apply it (cross-family pairs must be Rank-only — otherwise
`true cmp Integer` flips wrong).

**Cross-leaf numeric equivalence.** `1 cmp 1.0 → 0` is preserved:
the Number Comparer projects both Integer and Float to `float64`,
so magnitude equality across leaves stays consistent with
arithmetic (`1 + 0 == 1`, `1.0 == 1`).

**Order property.** Strict total order over distinct lattice nodes,
total preorder over values (one deliberate equivalence: cross-leaf
magnitude). Verified by `lang/spec/compare.tsv` (748 rows incl.
transitivity battery + user-defined-type coverage).

**Adding a Comparer to a user/external type.** Implement the
`Comparer` interface on your `TypeBehavior`:

```go
type fooBehavior struct{ defaultBehavior }
func (fooBehavior) Compare(a, b Value) (int, error) {
    // 1. Opt out for shapes you don't own (DepScalar, carriers).
    // 2. Apply litVsConcreteOrder if your type's literal should
    //    sort below concrete instances.
    // 3. Otherwise, project a and b into your domain and return
    //    -1/0/1 (or ErrNoComparer to bubble up to Rank).
}
```

The Comparer participates automatically in `cmp`/`sort`/etc. via
the LCA walk — no separate registration.

## Panic Prevention (CRITICAL)

**Panics must never occur in this codebase.** All code must be defensive
against unexpected input. Return errors instead of panicking — user
errors must be reported as error return values that are printed to the
user, never as panics. This is a hard rule, and per **ADR-005** it has
**no init-time exception**.

The previous carve-out for init-time type registration is withdrawn.
Those helpers now record their error and surface it at the first registry
construction instead of panicking:

- `eng`: `BuiltinInitError()` is checked by `NewRegistry()` (covers
  `mustType` / `builtinDecls`, now including `Ideal/Module` and
  `Node/Map/KeyVal`).
- `basic`: `TypeInitError()` is checked by `DefaultRegistryWithPolicy`
  (covers `RegisterTemporalType`, `registerBytesType`, and the
  `RegisterPatrunType`/`RegisterPidType`/`RegisterServiceType` handle
  registrations — the accumulator lives in `basic/go/typeinit.go` and
  `native` re-exports it).
- `modules/matrix`: `tensorTypeInitErr` is checked by `BuildMatrixModule`.

The only `panic`/`recover` left in the tree are Go standard-library
`Must*` calls on compile-time-constant inputs (e.g. `regexp.MustCompile`
of a literal) and `recover()` guards in tests that *assert* no panic.
Any new `panic` in non-test, non-`Must*`-constant code is a defect — do
not reintroduce `// lint:allow-panic`.

Key patterns to follow:

- **Always nil-check before dereferencing.** `AsMap(v)` and `AsList(v)`
  return `nil` when `Data` is nil (type literals like bare `Map` or
  `List`) **or when `Data` is a non-concrete subtype** (e.g.
  `RecordTypeInfo`, `OptionsTypeInfo`, `ChildTypeInfo`). Never call
  `.Len()`, `.Keys()`, `.Get()` etc. on a potentially nil result
  without checking first.
- **Type literals have nil Data.** `NewTypeLiteral(TMap)` creates a Value
  with `Parent=TMap, Data=nil`. Any code that receives a Value matching
  `TMap`/`TList`/`TAny` must handle the `Data==nil` case. This includes
  signature-matched arguments — `TAny` matches type literals.
- **Map subtypes share Parent=TMap.** RecordTypeInfo, OptionsTypeInfo,
  ChildTypeInfo, and *OrderedMap all have `Parent=TMap`. Code that checks
  `Parent.Equal(TMap)` matches all of them. Use `IsRecordType(v)`,
  `IsOptionsType(v)`, `IsTypedMap(v)` to discriminate, and guard
  `AsMap(v)` calls — it returns nil for non-OrderedMap data.
- **Guard conversion functions.** `valueToAny()` and `valueToMap()` in
  `native/transform.go` have nil-Data guards. If you add new
  conversion helpers, include the same guard.
- **Engine builtin handlers.** Check `args[N].Data == nil` before calling
  `AsMap(args[N])`/`AsList(args[N])` on arguments matched via
  `TMap`/`TList`/`TAny` signatures. See `native_accessor_dotr.go`
  for the canonical pattern.
- **Prefer `val, ok := v.Data.(Type)` over `v.Data.(Type)`.** The
  two-value form never panics; the single-value form panics on mismatch.
- **Write tests that use `recover()`.** For any new word or native
  function, add a test case in `TestTypeLiteralNoPanic` or
  `TestTypeLiteralNoPanicNative` that passes type literals and asserts
  no panic occurs.
