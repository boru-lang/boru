# `boru:fmt` — a formatter as a core module, and the XSLT question

Status: Phases 1, 3-rules, 3-vocabulary, 4-embedded, the Phase-2 seam, **and
the trivia-preserving tabnas CST front end** have **landed**, and the source
formatter is now **verified corruption-free** (seven `fmt`-corruption bugs
found and fixed — see "Formatter correctness baseline"). The tabnas front end
(`formatter.TabnasParse`, driven by `eng/go/parser.LexTokens`) is now the
formatter's **`DefaultParse`** — `boru fmt`, the `boru:fmt` module, the LSP, and
the Markdown/HTML embedders all parse through the boru-configured tabnas lexer.
It re-coalesces tabnas's fine tokens back into the hand-lexer's coarse words
and is pinned **byte-identical** to the retained hand-lexer (`HandParse`)
across the whole 73-file `.boru` corpus plus an edge-case battery
(`formatter/format_tabnas_test.go`). The earlier "impedance mismatch → keep the
hand-lexer" recommendation is thereby **superseded** — the conversion was
worked all the way through (see the Phase-2 entry). **Phase 3-full is now also
DONE**: the formatter's layout decisions live in a declarative RULE TABLE
**expressed in boru** — `formatter/fmt-rules.boru`, the stylesheet, embedded
and parsed at build time exactly as an XSLT stylesheet is expressed in XML
— and the Go emitter is the generic PROCESSOR that interprets it (the
`formatter.Rules` struct is only the stylesheet's compiled form; `Fmt.rules`
returns the table as data and `Fmt.format-with` overrides it per call). Whitespace is owned by the rules (attach classes, indent, width,
fill strategies), which resolves the earlier "Doc algebra can't reproduce the
output" fork: the rules emit their exact whitespace, so the output stays
byte-identical while every layout decision became data (see the Phase-3
entry). Phase 5's `describe`/help example canonicalisation
is **DONE** (156 of 161 examples rewritten to fmt's canonical form, the other 5
one-line and verified non-corrupted, pinned by `help/fmt_examples_test.go`); the
only inline "prose examples" left are the Notation paragraph's snippets that
*explain* the `# returns` convention, correctly left verbatim — so Phase 5 is
complete. See "Phased plan" for the per-phase state. This is a discovery/design
note, not an ADR (see `lang/go/CLAUDE.md`, "ADRs — only on explicit
instruction").

## Context

`boru fmt` today is a CLI subcommand (`cmd/go/internal/fmt`) that calls a
self-contained, hand-rolled pretty-printer, `formatter.Format(src) string`
(`lang/go/formatter/format.go`). It has three limitations we want to remove:

1. **No runtime API.** Formatting is reachable only from the shell, not from
   boru code. We want `import "boru:fmt"` / `Fmt.format`.
2. **Rules are baked into Go control flow.** The layout policy (spacing,
   wrapping at 70 cols, type-name capitalisation, map-shorthand expansion,
   `fn` bracketing) is imperative Go. We want the rules expressed
   **declaratively, as boru**, so the formatter's policy is data the language
   can read and extend — the same "the tool documents/defines itself" stance
   as `describe`.
3. **Its own tokenizer.** It re-lexes source with a bespoke scanner rather
   than the real parser. We want it to **parse with a tabnas parser**, passed
   in as a **parameter**, so there is one lexing story and the parser is
   swappable/testable.

Plus new behaviour: **72-column** lines (was 70) except **backtick/template
content is left verbatim**; **single fn params/returns drop their brackets**;
**all code examples everywhere** (docs, `describe`, …) run through fmt; and a
way for user code to format **embedded** boru inside Markdown/HTML via special
comment markers.

## Findings that constrain the design

These come from reading the parser, the module system, and the formatter
(see the four investigation threads summarised below).

- **F1 — The semantic parser is lossy.** `eng/go/parser.Parse` returns a flat
  `[]eng.Value` stream (not a CST). It **drops comments** (jsonic consumes
  `#`/`//` at the lex layer — `parse_test.go:556-572`), **alphabetises map
  keys** (jsonic hands back a Go `map[string]any`; the converter sorts —
  `parse.go:876,1106,1251`), **collapses backtick strings** to plain strings
  or lossy `InterpString`s (`grammar.go:322-381`, no `InterpString` arm in
  `canon.go`), and **discards whitespace**. Formatting off this stream would
  reorder users' maps and delete their comments. Unusable for a faithful
  formatter as-is.
- **F2 — tabnas' *lexer* is not lossy.** `jsonic.NewLex(src)` is drivable
  directly and exposes raw token text (`Token.Src`), positions (row/col/byte
  offset), and quote spelling (`Text.Quote` distinguishes `` ` `` / `'` /
  `"`). Custom `LexMatcher`s (priority bands match=1e6 … space=3e6) can
  intercept spans **before** the built-in space/comment consumer — the exact
  mechanism boru already uses for template literals and minilang. So a
  **lossless CST is buildable on the lexer layer**: comments (medium effort —
  capture is easy, re-attachment to the right node is the work), backtick
  verbatim (low — read `Token.Src`), source-ordered maps (parse maps at the
  token layer instead of via `MapRef`).
- **F3 — Import direction pins placement.** `modules → native → eng`. The
  `formatter` package is a leaf both can import (it has no engine deps today).
  Therefore: the shared driver must stay a **plain package importable by both
  `native` and `modules`**; describe/help (in `native`) cannot call the
  `boru:fmt` *module* directly (that would be `native → modules → native`).
  Doc-example formatting must run at the **CLI / `genhelp`** layer, which
  already builds a full registry and runs boru.
- **F4 — The XSLT substrate already exists.** boru has, natively:
  type/shape **multiple dispatch** (most-specific-wins over the lattice) — a
  direct analogue of `<xsl:template match="…">`; **`patrun`** (property-bag
  router, specificity-ordered, stores fns as values) — a priority/mode table;
  **`walk`** (depth-first pre/post-order tree rewrite with node replacement) —
  `apply-templates`; and **`fold`/`each`/`scan`** — the serialisation
  catamorphism. No new engine machinery is required to express template rules.
- **F5 — The bracketless `fn` form already *parses*.** `fn x:Integer
  [Integer] [body]` and `fn x:Integer Integer [body]` are valid triple-form
  spellings (`lang/spec/fn-triple.tsv:28,30,39`); only a **sole `List`-typed
  param** must keep the bracketed spec-list form (`tnot List` guard,
  `fn-triple.tsv:49`). No renderer emits the bracketless form yet — this is a
  **render-side** change, not a grammar change.

## Target architecture (layered, cycle-safe)

```
        ┌─────────────────────────────────────────────┐
  cmd   │ boru fmt (files, .md/.html)   boru describe  │
        └───────────────┬───────────────────┬─────────┘
                        │ shared Go driver   │ genhelp: examples → fmt
        ┌───────────────▼───────────────┐    │
 modules│ boru:fmt  →  Fmt.format,       │    │
        │            Fmt.* rule vocab    │    │
        └───────────────┬───────────────┘    │
        ┌───────────────▼────────────────────▼─────────┐
 leaf   │ package fmtcore (importable by native+modules)│
 pkg    │  ┌─────────────┐  ┌───────────────┐  ┌──────┐ │
        │  │ lossless    │→ │ document      │→ │ emit │ │
        │  │ tabnas CST  │  │ algebra + rule│  │(72col│ │
        │  │ (param:     │  │ engine (boru  │  │ back-│ │
        │  │  parser fn) │  │ decl. rules)  │  │ tick)│ │
        │  └─────────────┘  └───────────────┘  └──────┘ │
        └───────────────────────────────────────────────┘
```

**Shared driver.** One Go entry point, `formatter.Format` (evolving into
`fmtcore`), is called by *both* the CLI and the `Fmt.format` word. We
deliberately do **not** route file bytes through `a.Run("… Fmt.format …")`
(escaping hell, one registry per file). "The CLI calls into the module" is
satisfied in spirit by a single shared implementation, not by the CLI
literally dispatching a boru word per file.

**Parser as a parameter.** `fmtcore.Format` takes a `Parse` function value
(the lossless tabnas CST builder in production; a stub in tests). This is the
"parse using a tabnas parser (which is a parameter to the formatting)"
requirement, made concrete.

## The XSLT investigation

The request: *algorithmically investigate whether an XSLT-like approach works,
and whether XSLT itself could be expressed by `boru:fmt`.*

### XSLT in one paragraph

XSLT is a declarative tree-transformation language: a stylesheet is a set of
**template rules**, each with a **match pattern** (an XPath) and a body; the
processor walks the source tree and, at each node, selects the
**most-specific / highest-priority** matching template, instantiates its body
(literal result nodes + `<xsl:value-of>`), and recurses via
`<xsl:apply-templates select="…">`. **Modes** partition rule sets;
**priorities** break ties. XSLT (with its template layer) is Turing-complete.

### Does an XSLT-shaped approach work for formatting?

Formatting *is* a tree transformation: `parse-tree → layout-document → text`.
The XSLT control model maps onto boru primitives essentially 1:1 (F4):

| XSLT construct | boru realisation |
|---|---|
| `<xsl:template match="Integer">` | a rule fn dispatched by node **type/shape** (multiple dispatch, most-specific-wins over the lattice); or a `patrun` pattern keyed on node attributes |
| template **priority** / import precedence | `patrun` specificity + alphabetical tie-break, or lattice `Rank` |
| **modes** (`mode="toc"`) | a `mode` key in the `patrun` subject, or a separate rule table |
| `<xsl:apply-templates select="children">` | `walk` (pre/post-order) or `fold`/`each` over child nodes, re-entering rule dispatch |
| `<xsl:value-of>` / literal result | boru producing **document** fragments (the `Fmt.*` vocabulary below) |
| `<xsl:param>` / `<xsl:with-param>` | ordinary fn params / captured closure state |

So an XSLT-style declarative formatter is not only feasible, it is the
*natural* boru expression of the problem. The rule set becomes a collection of
`Fmt.rule`-registered fns (or a `patrun` table) keyed by node kind; a driver
`apply`s them depth-first and folds the resulting document to text.

### Could XSLT *itself* be expressed by `boru:fmt`?

Partly, and instructively:

- **The template-dispatch + apply-templates core: yes.** If `boru:fmt`'s rule
  engine is generalised from "match boru fmt-CST nodes" to "match **arbitrary**
  nodes by pattern, emit a sequence, recurse on selected children", it is a
  general match-driven tree-transform engine — the XSLT execution model. Both
  are declarative, both Turing-complete, so any XSLT transformation has a
  `boru:fmt` rule-set equivalent in principle.
- **The gap is XPath.** XSLT's selection power lives in XPath — axes
  (ancestor/descendant/following-sibling), positional predicates
  (`[position() mod 2]`), and value predicates. boru's analogues are partial:
  `Reach`/dotted paths, `StructUtil.getpath`/`selector`, and `patrun`'s
  attribute matching cover child/attribute selection and equality predicates,
  but not the full axis algebra. Expressing arbitrary XSLT would require a
  small **path-query vocabulary** (an XPath-subset) layered on `walk` — a
  well-scoped addition, not a redesign.

**Conclusion.** Adopt the XSLT model for the formatter (template rules keyed by
node kind, `apply` recursion, a document-algebra output). Design the rule
engine to be **node-generic** so it is, modulo an XPath-subset query layer, a
superset capable of expressing XSLT — but do **not** build the XPath layer now;
it is out of scope for formatting and belongs behind its own `parse`/`query`
module if ever wanted.

### The output side: a document algebra

XSLT emits a result tree; a *formatter* needs width-aware layout, so the rule
bodies produce a **Wadler/Prettier-style document** rather than raw text. The
proposed `Fmt.*` vocabulary (the "formatting words"):

- `Fmt.text s` — literal text (never broken).
- `Fmt.verbatim s` — literal text exempt from width (backtick/template bodies).
- `Fmt.line` / `Fmt.softline` / `Fmt.hardline` — a space-or-break / nothing-or-break / forced break.
- `Fmt.concat [d …]`, `Fmt.group d` (flatten if it fits ≤ width, else break),
  `Fmt.indent n d`, `Fmt.nest d`.
- `Fmt.kind node` — the node's dispatch key (an XSLT match pattern): a
  `$kind`-tagged Map's tag, else `map` / `list` / `scalar`. **LANDED.**
- `Fmt.children node` — the child sequence (apply-templates' node list): a
  Map's `{$kind:'entry' key value}` entry nodes, a List's elements, else
  `[]`. **LANDED.**

A Go (or boru) `Fmt.render width doc` prints the document, and `width` defaults
to **72**, with `Fmt.verbatim` spans passed through untouched (the
backtick-verbatim rule falls out of the algebra, not a special case in every
wrapper).

`Fmt.kind` and `Fmt.children` are deliberately **pure value→value
transforms** — classification and child-exposure only. Dispatch and recursion
stay in ordinary boru: fetch the rule fn from a table keyed by kind, apply it,
recurse on children. So the rule engine itself is boru (no fn-invocation or
registry-threading in Go), and a whole formatter reads like an XSLT
stylesheet — a rule table plus a one-line `apply` driver:

```
import "boru:fmt"
def apply fn [nd:Any Any [nd (rules get (Fmt.kind nd))]]
def rules {
  scalar: (nd:Any => (canon nd))
  entry:  (nd:Any => ({fmt:'concat' parts:[nd.key ':' (apply nd.value)]}))
  map:    (nd:Any => ({fmt:'group' body:{fmt:'concat' parts:((Fmt.children nd) each [apply])}}))
}
Fmt.render 72 (apply {a:1 b:{c:2}})    # → "a:1b:c:2"  (nested map recursed)
```

The `apply` driver applies a fn fetched from the table (a dynamic dispatch the
bytecode compiler leaves to the interpreter). See
`modules/fmtrule.go`, the `module-fmt.tsv` rows, and the end-to-end
`TestFmtDeclarativeFormatter`.

## Existing rules → declarative form

The current `format.go` rules restated as template rules (Phase 3 encodes
these as `Fmt.*` boru):

| Current behaviour (`format.go`) | Template rule |
|---|---|
| 2-space indent; no tabs | `Fmt.indent 2` in container rules |
| collapse blank runs to one; trailing newline | root rule |
| comma → no space before, one after; `:`/`.`/`?` attach | token-adjacency rules |
| no space inside `[] {} ()` | container rules emit `open · body · close` |
| type-name capitalisation in type position (not after `def`/`refine`, not before `:`, not `record`/`object`/`function`) | a rule on type-annotation nodes |
| map shorthand `{foo}`→`{foo:foo}`, `{foo?}`, `{foo/r}` | map-entry rule |
| wrap at width; `fn`/trailing-container/statement strategies | `Fmt.group` + width |
| **NEW: width 72** | `Fmt.render` default |
| **NEW: backtick verbatim** | `Fmt.verbatim` |
| **NEW: single param/return unbracketed** (except sole `List` param) | fn rule; keep spec-list form when the sole param conforms to `List` (F5) |

## Phased plan

- **Phase 1 — module + runtime API (DONE).** `lang/go/modules/fmt.go`
  (`BuildFmtModule`, `FmtNatives` with `format` : String→String wrapping
  `formatter.Format`, `BarrierPos:-1`), `docs_fmt.go`, catalog registration in
  `modules.go` + `help_render.go`, tests in `fmt_test.go` (both handler arms +
  end-to-end dispatch). `import "boru:fmt"` / `Fmt.format` works; `boru describe
  "boru:fmt"` lists it; the CLI keeps calling the shared driver.
- **Phase 2 — parser as a parameter, AND the tabnas CST front end (DONE).**
  Seam: `Format(src)` → `FormatWith(src, Parse)`, with `Parse` a
  `func(src) *Node` front end. The layout rules and emitter run on the tree
  regardless of front end. The tabnas front end that plugs into `Parse` is now
  **built and is the default** (`formatter.TabnasParse`, wired as
  `DefaultParse`; the hand-lexer is retained as `HandParse`, the independent
  reference). It is proven byte-identical to the hand-lexer across the whole
  `.boru` corpus + an edge battery (`TestTabnasParse*`). The build worked the
  earlier impedance-mismatch objection all the way through — here is how each
  concern resolved:
  - **Trivia lexing (mechanism).** `eng/go/parser.LexTokens` drives the
    boru-configured tabnas lexer with `cfg.IgnoreSet = {}`, so `Next` returns
    `#SP`/`#LN`/`#CM` verbatim, each carrying `Src` + byte offset `SI`. It
    returns `(tokens, ok)`; `ok` is false when `lex.Err` is set (an
    unterminated string/backtick, a bad token), the signal the front end uses
    to fall back.
  - **Layering (F3) — resolved as option (a).** `formatter` now imports
    `eng/go/parser` for `LexTokens`. No import cycle
    (`formatter → eng/parser → eng`; eng/parser imports neither back). The
    package's engine-dep-free leaf status was the cost, paid deliberately —
    reusing the ONE configured lexer beats re-declaring boru's lex
    customisations in a second place (option (b)'s duplication).
  - **Coarse-vs-fine impedance — re-coalesced, not re-implemented.**
    `tabnasTokenize` accumulates a run of adjacent word-part tokens
    (`#TX #DT #LA #RA #AR #ML #NR #BD #XML`, plus a mid-word `#ST`) into one
    coarse word, exactly reproducing the hand-lexer's `isDelimiter` word rule
    — `foo.bar`, `a<b>`, `x="1"/>` each collapse back to one word. A handful
    of narrow rules close the byte-for-byte gap the fine tokens open:
    - a lone `#NR` whose literal ends in a bare `.` (`1.`) splits into a
      number + a dot operator — the hand-lexer only keeps a `.` in a number
      when a digit follows (`1.5`);
    - a run of blank lines arrives as ONE `#LN` (`"\n\n\n"`) and is
      re-split into one `TokNewline` per `\n`;
    - a standalone `.` (empty word buffer) is a `TokDot`, not glued;
    - a **backtick literal** is scanned verbatim from source via the shared
      `scanBacktick` (the flat `Next` loop lacks the grammar-rule context that
      drives `#TL` lexing, so it mis-lexes the interior — e.g. a `//` inside a
      URL swallows the line tail into a comment); the swallowed tail is
      recovered by re-tokenising the byte gap.
    This is bounded adapter code (≈120 lines), not a second hand-lexer, and it
    is regression-locked to the hand-lexer by the corpus differential.
  One supporting finding from the original probe, retained for the record:
  - **The configured lexer's token stream is PROVEN (probe run).** Driving
    the boru-configured jsonic (`setupBaseTokens` + the template / bignum /
    minilang / xml matchers) via `jsonic.NewLex(src, j.Config())` with
    `lex.Config.IgnoreSet = {}` yields a clean, workable stream: `#TX` word,
    `#NR`/`#BD` number, `#ST` string, `#CM` whole line comment, `#LN`/`#SP`
    trivia, `#OS`/`#CS`/`#OB`/`#CB`/`#OP`/`#CP` brackets, `#DT` dot — and
    crucially **`#ML` captures a whole `+re/[a-z]+/` minilang literal** and
    `#CM` a whole comment, so those need NO special handling. This is the
    stream `LexTokens` now returns, and `tabnasTokenize` adapts.
  - **SUPERSEDED — the impedance mismatch was worked through, not avoided.**
    The earlier conclusion recommended keeping the hand-lexer as the default
    because reproducing its COARSE word tokens from tabnas's FINE tokens
    (`foo.bar` vs `foo`·`.`·`bar`; `x="1"/>` vs `x=`·`"1"`·`/`·`>`) meant
    re-coalescing — "more code for byte-identical output". That objection was
    correct about the mechanism (re-coalescing IS required) but the code turned
    out bounded (≈120 lines of adapter, all covered) and the byte-identity is
    exactly the property that makes the swap SAFE, verified by the corpus
    differential. The probe also paid for itself in corruption fixes (#7 `##`
    is a line comment; and, on the second pass, the `//`-in-backtick line-tail
    swallow and the `1.`-trailing-dot rule). The default is now the tabnas
    front end; the hand-lexer stays as `HandParse`, the differential oracle.
- **Phase 3 — rules.** Rule behaviour DONE in the Go formatter: 72-col,
  backtick-verbatim (`scanBacktick`), and bracketless single param/return
  (`elideFnBrackets`). The declarative **substrate** is DONE: a
  Wadler/Prettier document algebra (`formatter.Doc`/`RenderDoc`) exposed as
  the runtime word **`Fmt.render`**, driven by a boru doc tree
  (`{fmt:'group' body:…}` etc.) — "formatting words in the fmt module,
  expressed as boru data". The XSLT-style **rule vocabulary** is now also DONE:
  the two pure words **`Fmt.kind`** (node → dispatch key) and
  **`Fmt.children`** (node → child sequence) make a declarative rule table +
  a one-line `apply` driver the natural boru expression of a formatter
  (`modules/fmtrule.go`, demonstrated end-to-end by
  `TestFmtDeclarativeFormatter` — see "The output side" above). **Phase 3-full
  is DONE** — the built-in layout rules are now DATA and the Go emitter is the
  processor that interprets them:
  - **The rule table is boru source.** The canonical table is
    `formatter/fmt-rules.boru` — a commented boru stylesheet, embedded and
    parsed at first use (`rules_boru.go::DefaultRules`); the file is itself
    fmt-clean (laid out in the style it defines — pinned by
    `TestRulesStylesheetFmtClean`), and a broken stylesheet surfaces at
    module construction via `DefaultRulesInitError` (the ADR-005 pattern),
    never as a panic or a silent zero table. The `formatter.Rules` struct
    is the stylesheet's COMPILED form. It carries every layout decision
    the formatter makes: the 72-column width, the 2-space indent step, the
    statement-start grouping words, the `fn` trigger, the attach classes
    (which token kinds glue to which — the whitespace policy), the
    dot-suffix glue rule, the per-kind TEMPLATES, and the ordered STRATEGY
    list naming the statement-level templates (`comment-only` → `inline` →
    `trailing-comment` → `fn` → `trailing-container` → `wrap`). None of
    these lives in the walk any more; `emitStatement` is a template dispatch
    over the strategy list, `attach` reads the kind sets, every fit check
    reads the width, every glyph and indent reads the table.
  - **The templates section IS the XSLT-template layer.** Every node kind
    has a template BODY in the stylesheet — literals around one recursion
    op, e.g. `list:['[' apply/q ']']`, `map:['{' entries/q '}']`,
    `word:[text/q]`, `comma:[',']`, `root:[statements/q]` — the direct
    analogue of `<xsl:template match="list">[<xsl:apply-templates/>]</…>`.
    The ops are the closed recursion vocabulary (`text` the node's source
    text, `apply` the rendered children, `entries` the map's canonicalised
    key:value entries, `statements` the statement sequence); a punctuation
    body is just the literal it emits. Container brackets and punctuation
    glyphs are no longer parameters — they are the templates' literals
    (the former `brackets` key is retired). How an overflowing body BREAKS
    (fit checks, grouping, greedy fill) belongs to the width-driven
    strategies — the output method, as xsl:output/indent is processor
    machinery in XSLT — selected and ordered by the stylesheet's
    `strategies` list.
  - **The boru surface**: **`Fmt.rules`** returns the canonical table as boru
    data; **`Fmt.format-with <rules> <src>`** formats under a (partial)
    override table — width, indent, attach, brackets, strategies,
    statement-starts, fn-word are all overridable from boru, each validated
    (unknown keys/kinds/classes/strategies error). The AUTHORITY pin:
    `Fmt.format-with (Fmt.rules) src == Fmt.format src` (spec row + Go
    round-trip test) — the boru table is the formatter's configuration, not
    documentation of it.
  - **How the whitespace question resolved.** The earlier "impedance" fork
    asked how declarative rules could reproduce the emitter's greedy fill
    when the Wadler Doc algebra breaks all-or-nothing. Answer: templates own
    their whitespace — the XSLT model outputs TEXT, not groups. The rule
    table's attach classes, indent, width, and the `wrap` fill template ARE
    the whitespace policy, so the declarative formatter reproduces the
    canonical output byte-for-byte (every golden test and corpus gate passes
    unchanged) while a different table produces a genuinely different style.
    No reflow happened and none was needed.
  - **What stays in Go and why.** The processor (recursion, measurement,
    template interpretation) is Go, exactly as an XSLT processor is not
    written in XSLT — and it keeps `boru fmt`/LSP fast. The fully-boru path
    remains available and demonstrated for user-defined formatters:
    `Fmt.tree` (the CST as a `$kind`-tagged value tree) + `Fmt.kind` /
    `Fmt.children` dispatch + rule fns (`TestFmtTreeDeclarativeFormatter`
    reproduces `Fmt.format` on the attachment-free core), and `sp` — the
    structure-path minilang — selects into the same tree
    (`(Fmt.tree src) mini sp '//text'`), giving XSLT-style match patterns
    over the format CST.
  - **R4 RESOLVED (concrete finding) — the substrate does NOT reproduce the
    current output; the two use different layout models.** The Go emitter
    (`emitList`/`emitStatement`/`wrapStatement`) does **greedy fill-wrapping**:
    an overflowing list keeps words flowing across continuation lines
    (`[alpha … kilo` / `      lima mike november oscar` / `  ]`). The
    `formatter.Doc`/`RenderDoc` document algebra behind `Fmt.render` does
    **all-or-nothing Wadler/Prettier group breaking**: an overflowing group
    breaks *every* line point (one item per line). They are not the same
    formatter, and no rule set emitting into today's Doc algebra can reproduce
    the emitter's output. Consequence: routing `Fmt.format` through the
    declarative Doc-algebra rules — Phase 3-full as written — is **NOT
    output-preserving**. It re-flows the ENTIRE `.boru` corpus and every
    fmt-clean-pinned example/doc block from the emitter's fill style to the
    document algebra's per-line style, colliding with the byte-identical /
    corruption-free / 100%-gate discipline every other phase held to (Phases
    3-rules and 5 pinned that exact output). So Phase 3-full cannot be
    "completed" the output-preserving way the others were; it forks on a
    maintainer decision:
    - **(a) Accept the reflow.** Adopt the document-algebra layout as the
      canonical style, port the rules onto `Fmt.render`, regenerate every
      golden test and re-pin every example/doc block. Genuinely declarative
      and the design's intent — but a repo-wide, user-visible formatting
      change requiring explicit sign-off.
    - **(b) Preserve the output.** Reimplement the emitter's exact greedy-fill
      + fn/trailing-container/statement strategies as boru rules that emit
      strings directly (bypassing the generic Doc algebra), plus a
      source-`*Node` → boru-value bridge and engine-driven `Fmt.format`. Keeps
      byte-identity but is a large multi-session port; the layout rules being
      boru (not Go) sit outside the Go coverage gate, only the bridge/driver
      need Go coverage.
    - **(c) Extend the algebra with a `fill` primitive** matching the
      emitter's greedy wrap, then port onto it (still large; the fn/container
      strategies remain to encode).
    **RESOLUTION (this fork is closed).** The maintainer's direction — the
    XSLT-style algorithm should handle output whitespace — exposed the wrong
    premise in the fork: the choice was never "Doc algebra or bust". XSLT
    templates emit TEXT, whitespace included, so the rules can own the exact
    current whitespace. Implemented as the rules-as-data / processor split
    (see the Phase 3-full DONE entry above): every layout decision moved into
    `formatter.Rules` (boru-visible and overridable via `Fmt.rules` /
    `Fmt.format-with`), the emitter became the table's interpreter, and the
    output stayed byte-identical — no reflow, no imperative boru
    reimplementation of the walk, no algebra extension needed for the
    default style. (A `fill` Doc primitive, option (c), remains a possible
    future addition for Doc-DATA rule sets that want the greedy style.)
  - **Source bridge BUILT + declarative source formatter DEMONSTRATED.** The
    piece common to every fork — getting the boru-source layout tree into boru
    values so the rule vocabulary can reach it — is now done: **`Fmt.tree`**
    (`modules/fmttree.go`) parses source and returns its layout CST as a
    `$kind`-tagged value tree (`{$kind:'word'|'list'|… text:… children:[…]}`),
    the shape `Fmt.kind` dispatches on. `formatter.ParseTree` exposes the same
    transformed tree the emitter walks (capitalisation + fn-bracket elision
    applied), so a rule set only decides layout. `TestFmtTreeDeclarativeFormatter`
    is the payoff: a formatter for **boru source** written as a declarative boru
    rule table over `Fmt.tree` (rules keyed by `Fmt.kind`, an `attaches` rule
    reproducing the emitter's token-adjacency logic, and a `fold` over
    `node.children` threading the previous node) reproduces `Fmt.format`
    **byte-for-byte for EVERY statement that fits on one line** — the full
    inline layer, attachment included (`x:Integer`, `m.k`, `x?`, `foo.bar`,
    `input.(baz)`, nested containers, multi-param sigs all verified). This
    proves the vocabulary reaches and formats real source, not just data —
    the ONLY remaining port is the **wrapping strategies** (blocked on the
    (a)/(b)/(c) fork above), NOT the inline layer or the tree-access plumbing,
    both of which are done and covered.
  - **Finding — the imperative reproduction (fork (b)) fights the engine's
    scoping; the boru-natural path is Doc-DATA (fork (a)).** Writing the inline
    layer surfaced three distinct engine-scoping frictions that a recursive
    string-building formatter hits but a Doc-DATA one does not: (1) a `get`
    with an integer index in a mutually-recursive fn fails dispatch; (2) a
    map-value paren-expr (`{s:(join …)}`) cannot see body-local `def`s; (3) a
    body-local `def` holding a USER-FN result is lost after a subsequent `if`
    in a fold body. Each was worked around (index-free `fold`, bare-word map
    values, compute-the-`if`-first + inline the fn call), but the pattern is
    clear: reproducing the *imperative* emitter in boru is not "just more boru",
    it fights the interpreter — whereas the DATA formatter
    (`TestFmtDeclarativeFormatter`) builds `{fmt:…}` Doc data via `each` and
    hits none of this. So fork (a) (emit Doc data, accept the reflow) is the
    boru-*idiomatic* completion and fork (b) (byte-identical imperative port) is
    the one that fights the tool.
    This finding stands as the reason the completion did NOT take fork (b)'s
    shape (a byte-identical imperative port in boru): that path fights the
    interpreter. The landed resolution sidesteps it — the rules moved to
    DATA and the fast Go processor interprets them, so production
    `Fmt.format` keeps its performance, the canonical style is expressed as
    a boru-readable/overridable table, and the engine-scoping frictions
    catalogued above never arise. The fully-boru vocabulary stays available
    (and source-capable) for user-defined formatters.
- **Phase 4 — embedded formatting + fenced doc blocks DONE; describe/inline
  examples REMAINING.** `boru fmt` reformats ```` ```boru ```` fences and
  `<!-- borufmt --> … <!-- /borufmt -->` marker regions in Markdown/HTML (CLI
  dispatch by extension; runtime words `Fmt.format-markdown` /
  `Fmt.format-html`). The prose docs' ```` ```boru ```` blocks are now run
  through fmt (README) and pinned fmt-clean by a gate
  (`test/go/docexamples`). A formatter fix keeps a trailing `# comment`
  attached to its statement (never wrapped off), so annotated example lines
  survive fmt and the `# returns` extractor still pairs them. REMAINING: the
  inline `expr # returns X` prose examples (not in fenced blocks) and the
  `describe`/help worked examples. Both use deliberate column alignment that
  fmt collapses — a visible-output change to the docs and the `describe`
  command warranting maintainer sign-off, not a silent migration. Two
  the `describe`/help worked examples are now **DONE**: every example is run
  through fmt and the 156 that fit the width are rewritten to fmt's canonical
  form (the dominant change is the `;#` result-marker spaced to `; #`, plus
  map-colon tightening, comma removal, double-space collapse, and the
  bracketless single-fn form). The 5 that fmt would WRAP (long multi-statement
  examples) stay one line — `describe` renders one example per line — and are
  verified non-corrupted instead. `help/fmt_examples_test.go` pins every
  non-wrapping example as fmt-clean (a no-op under fmt) and the wrapping few as
  non-corrupted, so they cannot drift back out. The remaining "inline prose
  examples" turn out to be a NON-item: the only inline `# returns` snippets in
  the Diátaxis docs (2 per file) live in the **Notation** paragraph that
  *explains the `# returns` convention itself* (`square 4  # returns 16`) — the
  `  #` double-space IS the notation being documented, so running fmt over it
  would collapse the very convention the prose defines. They are correctly left
  verbatim. Phase 5 is therefore complete. (A no-wrap width on the source
  formatter — so the 5 long examples could be one-line-canonical too — is a
  possible nicety, low value since they are already one line and verified
  clean.)

## Formatter correctness baseline (verified 2026-07)

Before any "run all examples through fmt" work, the formatter was audited by
running `Format` over every describe/help example and every `lang/spec/*.tsv`
input and flagging any change that survived whitespace/comma/bracket/case
normalisation (a candidate semantic change). Three real corruption bugs were
found and fixed; after them the corpus is **corruption-free** — every change
fmt makes is a documented, semantically-preserving canonicalisation (comma
removal, bracket elision, safe type capitalisation, map-shorthand expansion,
width wrapping):

1. **Minilang literals** (`+re/[a-z]+/` → `+re/ [a-z] +/`, `+gex|a*b|` →
   `+gex | a*b |`). The hand-lexer split them at their inner `[`/`]`/`|`.
   Fixed with `scanMinilang` (verbatim atomic scan mirroring the parser's
   `setupMiniLitMatcher`).
2. **`none` / `list` / `any` / `node`** rewritten to their type-literal
   spelling (`size none` → `size None`, `list 1 2 3` → `List 1 2 3`) — but
   these are valid VALUE / function words distinct from the types
   (`canon none` → `none`; `none tcmp None → 1`). Removed from `knownTypes`;
   the other ten names are undefined as values so they still capitalise.
3. **Silent truncation** — `tryFnFormat` emitted only `header [args][ret][body]`
   from the first triple, so any tokens after the `fn` wrapper, and every
   overload past the first, were DELETED (`def f fn […] end f 1 2 3` lost
   `end f 1 2 3`). It now declines to the general (node-preserving) path
   unless the wrapper is the last node and has exactly one triple.
4. **Dot-key capitalization** — the literal `dot` / `dotr` accessor words
   quote their following bare word as a field NAME, so `opts dot table`
   became `opts dot Table` (a different field). Added an `afterDot` guard.
5. **Comment absorption** — a line comment runs to end of line, so a closing
   delimiter placed after it on the same line was commented out
   (`{a:1 # note b:2}` → the `}` is inside the comment → unbalanced). Any
   container with a line comment now renders multi-line, closing on its own
   line.
6. **Content after a leading block comment dropped** — `## bc ## foo` →
   `## bc ##` (the `foo` deleted), because emitStatement returned only a
   leading comment's text; now only fires for a comment-ONLY statement.
7. **`##` mis-modelled as a bounded block comment** — boru has none (`#` and
   `##` both run to end of line; `## c ## print 42` runs nothing, verified),
   but the hand-lexer treated `## … ##` as bounded and REFORMATTED what
   followed as code (`## t ## x is integer` → `## t ## x is Integer`,
   corrupting comment text). `#`/`##` are now one line-comment kind; the
   `TokBlockComment` / `NdBlockComment` types (a construct that never
   existed) are gone. **Found by the Phase-4 tabnas-lexer probe** — the
   configured lexer returns the whole `## … ##` span as one `#CM`, which
   exposed the mismodel; the CST investigation paid off before any CST was
   built.

Bugs 4–7 were found by sweeping all repo `.boru` programs (and, for 7, the
Phase-4 lexer probe); the final verification is that fmt turns **0 of the
72 parseable files** into unparseable source. The risk was never the
layout, it was fmt changing what code MEANS.

**Known non-corruption limitation (pre-existing, cosmetic):** the
bracketless single-param fn form is not idempotent when it must wrap — the
wrap introduces a root-level newline that the formatter's physical-newline
statement splitting re-reads as a boundary, so a second pass drops the
continuation indent. Both passes parse to the identical program (no data or
semantic change); only the indentation of the continuation line differs. A
real fix needs continuation-aware statement splitting (or wrapping the
bracketless body inside a delimiter), out of scope for the correctness pass.

## Risks

- **R1 — Coverage gate (ADR-008).** Every new Go statement needs a test or a
  proof-carrying `//covergate:allow`. Phases 2–3 add real logic; budget test
  effort accordingly. Phase 1 is fully covered.
- **R2 — Blast radius of the render changes.** Bracketless params and 72-col
  change the output of *every* formatted fn/long line; expect wide updates to
  golden tests, `lang/spec/*.tsv`, and docs. Doing them in Phase 3 (behind the
  declarative engine) keeps the churn to one reviewable change.
- **R3 — Comment re-attachment** is the genuinely hard part of Phase 2
  (placing a captured comment against the right CST node). This is why the
  current formatter is lexical; the lossless CST must solve it, not inherit it.
- **R4 — Two formatting paths during transition.** Until Phase 3 subsumes it,
  `formatter.Format` (lexical) and any `fmtcore` path must be kept reconciled
  or one clearly authoritative.
