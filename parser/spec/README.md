# parser/spec — the parser-level parity corpus

The declarative contract `parser/go` and `parser/ts` are both held to. One set
of files, three independent runner pairs (`parser/go/parserspec_test.go` and
`parser/ts/src/parserspec.test.ts` for parsed values;
`parser/go/lexspec_test.go` and `parser/ts/src/lexspec.test.ts` for raw
tokens; `parser/go/dataspec_test.go` and `parser/ts/src/dataspec.test.ts` for
the safe data-decode seam), no shared reader or renderer code between ports.

## Why it exists

`parse-battery.test.ts` opened by declaring "The Go parser is the REFERENCE:
every construct converts to the same value stream." Nothing checked that.
When it was checked for the first time, **15 of 254 rows encoded a render
`parser/go` does not produce** (design/legacy/TS-PARITY-AUDIT.0.ignore) — the battery
had been pinning TypeScript's own behaviour, and the twins could drift with
no gate noticing.

The other twin pairs already had a shared corpus — `eng/spec` for the
engines, `core/spec` for the cores. The parser was the only pair without one.

A parser-level oracle *was* designed: `parser/go/streamdump_test.go` and
`parser/ts/src/streamdump.ts` both dump the value stream for every `eng/spec`
row. Nothing ever ran the comparison — no target, no CI step — so with
`STREAMDUMP_FILE` unset the Go side dumps to a temp dir and discards it.
`make crossdiff` does compare the two *engines*, but hard-fails only when
both produce a value and the values differ, so a parser-level difference that
still evaluates alike is invisible to it.

## Two runners, no shared code

Each runner reads the files and implements the escape decoding and the render
independently, in about forty lines. That is the same discipline `core/spec`
uses, for the same reason: shared scaffolding can hide one bug from both
engines (design/legacy/CORE-GO-TS-DEFECTS.0.ignore, blind spot 9), and a shared reader
would hide exactly the class of defect this corpus was built to catch.

## Format

Tab-separated. A line is a **comment** only when it starts with `#` *and*
contains no tab — `#` is boru's own comment marker, so sources begin with it,
and treating every `#` line as a comment silently drops those rows. Blank
lines are ignored; a whitespace-only source is a real row, not a blank one.

Every column is escaped (`\n`, `\t`, `\\`) so one row is exactly one line. A
render can itself contain a newline — XML text spanning lines does — and an
unescaped one splits the row and truncates it silently. Both failure modes
above were live bugs during this corpus's construction, which is why both are
called out here.

### `lex.tsv` — the formatter-facing token contract

```
src	OK|ERR	canonical token JSON
```

Exactly three columns are required. `OK` means the lexer reached clean EOF;
`ERR` means it stopped at a lexical error and its returned prefix is
untrustworthy. Each token records its kind, raw source, and `si` as a UTF-8
byte offset. The canonical JSON field order is `name`, `src`, `si`.

This deliberately has separate Go and TypeScript readers and renderers rather
than extending the parsed-value harness: raw tokens retain whitespace,
comments, newlines, and boundaries which `parse.tsv` necessarily discards.
Its 34-row exact ratchet covers empty and bad EOF, an unterminated backtick's
clean lexical EOF, trivia, astral-character byte offsets, decimal and dotted
exponents, number/member and colon/arrow suffix boundaries, malformed
separators, hexadecimal/octal/binary overflow boundaries, and adjacent quote
boundaries after plain words and completed receivers.

The §base-prefixed boundaries rows are the ones to read before touching the
decimal-boundary matcher's base-prefix arm. They pin a class where the two
stock scanners disagree — but only for SOME follow characters, since the TS
scanner classifies the run before a `.` by looking past it. One agreeing
shape (`0xFF.x`) is therefore pinned alongside the diverging ones and
labelled as such: it cannot fail on its own, and that is the point. Probing
an agreeing shape and concluding the arm is retirable is the specific
mistake those rows exist to stop.

### `parse.tsv` — the contract

```
src	expected	optional note
```

Exactly three columns are required; the note column is always present and `-`
means no note. This makes an unescaped tab an extra field that fails loudly
instead of silently truncating `expected`. `expected` is the canon of the parsed value
stream (each value rendered, space-joined), or `ERR ` and the first line of
the error text. **Both engines must produce it.**

### `divergent.tsv` — the parity debt

```
src	go	ts	note
```

Sources the two engines render differently — the **live parity-debt
ledger**. Every row must be a real, currently-measured divergence: the Go
runner re-renders each row's source and must reproduce the `go` column
exactly, the TS runner must reproduce the `ts` column, and the reader
refuses equal `go`/`ts` renders and blank justifications. A divergence
that gets fixed therefore fails one runner loudly and the row must move to
`parse.tsv`; a row can never drift into recording behavior neither port
has.

The row count is machine-pinned in both runners (like every other corpus
size), so debt cannot grow silently: adding a row is a deliberate,
two-sided, justified act, and the ratchet direction is shrink-toward-zero —
a row leaves the ledger by being fixed, never by being deleted. Zero rows
remains the goal state; a non-empty ledger is measured, triaged truth, not
accepted-and-forgotten green debt.

### `data.tsv` — the safe data-decode seam

```
src	expected	note
```

Exactly three columns are required. The seam is `SafeParseData` /
`safeParseData` plus `ConvertParsedNumber` / `convertParsedNumber` — the
parsers behind `StructUtil.parse` — which the language-level corpus can
only reach through Go's lang layer, so a twin regression here is invisible
to every other table. (Exactly that happened once: both ports briefly lost
the lenient jsonic superset together, and no gate noticed.)

`expected` is either a canonical decode render or an error form. The
render: insertion-ordered `{k:v ...}` maps, `[v ...]` lists, `'...'`
strings, `true`/`false`, `none` for null, and every numeric token converted
through the public number converter so int/float kind, precision, and exact
spelling participate. `ERR <code>` pins the converter's loud refusal of a
claimed numeric spelling; bare `ERR` pins only that jsonic itself rejects
the source — those messages are deliberately dependency-native and
unpinned. Both runners implement the reader and renderer independently, as
everywhere else in this corpus.

Two seam asymmetries used to be documented here as inexpressible; the
`tabnas` upgrade of 2026-08-10 (jsonic v0.6.0 / parser v0.8.0, ADR-014)
closed both, and each is now pinned by rows instead of prose:

- **Map insertion order, integer-like keys included.** A plain JS object
  enumerates `{2:9, 1:8}` as keys `1, 2` whatever the source wrote, so the
  TS seam could not express what Go's `OrderedMap` preserves. TS now parses
  data with `map.ordered`, and its runner reads the recorded order
  (`dataKeyOrder`) rather than `Object.keys` — the Go runner still walks
  `OrderedMap.Keys`, so the two remain independent implementations of one
  contract.
- **Sign+separator runs.** `+_1` is lenient text in both ports. Go's stock
  scanner used to read it as the number `1`, silently swallowing the `+_`;
  a separator is legal only between digits, so a digit-led `1_` is still
  claimed and refused loudly.

### `nesting.tsv` — the generated-depth contract

```
kind	depth	expected outcome
```

Exactly three columns are required. Rather than committing enormous source
and render strings, each runner independently expands `kind` (`list`, `map`,
`paren`, `typed-list`, `typed-map`, or a mixed list/map/paren spine) to the
requested source-container `depth`. `expected outcome` is `OK` or
`ERR evaluation_limit`.

Successful deep values are deliberately not canonically rendered: recursive
rendering has its own host-stack limit and is outside this parser contract.
Instead, each runner iteratively walks all 501 or 10,000 containers, checking
the exact list/map/paren/typed-container kind, singleton edge, map key, and
terminal value. A parser that accepts the source but truncates or flattens the
result therefore fails. The rows pin both the depth-501 acceptance that the old
TypeScript-only guard violated and the exact shared 10,000-frame boundary.
Root parentheses accept only 9,999 source groups because their implicit
top-level item conversion is also a frame.

This contract replaces the former nesting row in `divergent.tsv`: the
TypeScript converter now walks nested parser nodes on an explicit work stack,
so it reaches the same boundary as Go without depending on the JavaScript host
call stack.

### `shape.tsv` — the semantic-shape contract

```
case	src	expected semantic shape
```

Exactly three columns are required, case names are unique, and `src` and the
expected shape use the same escape notation as `parse.tsv`. Each runner owns a
separate strict reader and recursive renderer.

This is the deliberately non-canonical half of the parser oracle. Canonical
source is retained as a compact payload label, but every rendered value also
records its source position, `Eval` and `Quoted` flags, and word payload
(`ArgCount`, `/s`, `/f`, `/v`, and `/u`). Lists, maps, parentheses, and reach
nodes are traversed so nested and operator-generated positions cannot hide
inside an equal outer canon. Map rendering also records the implicit-pair bit
and computed-key set; typed containers expose their child constraint plus
concrete elements/entries; sugar exposes every `SugarInfo` field, including
the angle head and deferred head error. The modifier rows cover `/N`, `/f`,
`/s`, `/v`, `/u`, `/uv`, and `/q`; the last is represented by the Atom payload
it is specified to emit. Parser-authored values do not set `Quoted` — quoted
source becomes a String or Atom — so the corpus pins that flag false while
pinning both false and true states of `Eval`. Unicode rows pin columns after a
plain value and after the minilang, template, and XML custom matchers. Columns
are 1-based Unicode code points, never UTF-8 bytes or JavaScript UTF-16 units.

Error shapes record the structured `BoruError` fields rather than its rendered
first line: code, detail, row, column, offending source, full source, hint,
notes, and help suggestions. This catches diagnostic parity drift even when
the two user-facing first lines still compare equal in `parse.tsv`.

### `canon-fixpoint.tsv` — ADR-015's fixpoint ledger

ADR-015 says `canon` renders source that re-parses to the same value, and
design/CANON-ROUNDTRIP.0.md §1 keeps the textual FIXPOINT as its diagnostic:
a renderer that fails it is always wrong. Both runners apply it to every
`parse.tsv` row that parses — the canon of the parsed stream, parsed again,
must canon to the same text — and a row that does not must be listed here
with the record that closes it. A listed row that reaches its fixpoint fails
both runners until it is removed; the row count is pinned in both, so the
ledger only shrinks. It landed with NUR072 (2026-09-26), whose own kinds — a
plain word, the lambda fold, the mini literal, the type bound, the group
modifiers — all reach their fixpoint; the rows it lists are the kinds that
gate found (NUR225–NUR227).

## The current debt

**Corpus parity is exact; open-input parity is not.** Every row of
`parse.tsv`, `lex.tsv`, `nesting.tsv`, and `shape.tsv` renders identically
in both ports. But the corpus is not the language: a 2,587-source
parity-probe sweep (token soup over the surface alphabet plus truncation
mutants of `parse.tsv` rows, `scripts/parity-probe.sh`) measured **55
divergences (~2.1%)** on inputs outside the corpus, and follow-up probing
found one further class the sweep's seed missed. The ledger carries one
representative row per class found so far — the class list is what
probing has MEASURED, not a proof of exhaustion; new probe-found classes
get new rows:

- **trailing `=>` fold loss** (2 rows): Go's arrowfold folds a bodyless
  trailing arrow (the input doubles as the body); TS drops the group from
  the value stream — a DATA-class divergence, also visible inside a
  dotchain segment.
- **accept/reject splits** (2 rows): a trailing bare `:` (Go accepts,
  TS refuses) and `=> ,` (Go refuses, TS accepts and silently drops
  tokens).
- **post-`]` recovery detail** (1 row): Go names the offending token after
  an unmatched `]`; TS reports an empty token.
- **error precedence** (2 rows): receiverless-`.` vs unmatched-`(`, and
  bare-`/s` vs unmatched-`(` — the two walks meet the faults in a
  different order.
- **internal type-name leak** (1 row): `unsupported value type
  parser.unclosedParen` vs `UnclosedParen` — neither render is stable.
- **empty-`${}` fold in an unterminated template** (1 row): both ports
  accept `` `${} `` but Go folds the empty interpolation to `interp('')`
  while TS keeps the `interp(${})` hole — a value-level fold divergence.

Fifty probe-agreed sources from the same sweep were promoted into
`parse.tsv` (the `§probe-2026-08-09` section) so the corpus covers the
divergent classes' immediate neighborhoods where the ports DO agree.

The eleven rows found after the ledger first reached zero were closed
rather than accepted:

- Eight rule-step-limit shapes now retain the exact offending `]`/`}` token
  and its position from the TS rule subscriber; before the guard, TS silently
  accepted several of them.
- Two decimal/underscore lexer-boundary rows (`1.2_`, `1_.2`) now reach one
  whole numeric token in both ports through matching, narrowly scoped
  boundary matchers.
- The depth-501 row became the stronger generated `nesting.tsv` matrix after
  TypeScript conversion was made stack-safe through the shared 10,000 limit.

The independent numeric sweep also exposed a shared language-contract bug:
both ports admitted `_` next to a prefix/exponent marker even though
REFERENCE.md permits it only between digits. Both validators now check actual
digits in the literal's base, and the final 66-case matrix lives in
`parse.tsv`.

## History

The ledger held **15 of 254 rows** when it was first checked, and was driven
to zero on 2026-08-08. Measuring what had never been measured then added
three BigDecimal rows, and those were closed too — by giving `core/ts` a
real arbitrary-precision decimal (`core/ts/src/decimal.ts`, a scaled bigint
on apd's model) in place of the binary64 payload that could not represent
what Go represented. What the original fifteen turned out to be:

| class | rows | resolution |
|---|---:|---|
| big-decimal canon | 8 | `core/ts` had `TBigInteger`/`TBigDecimal` as types with **no constructor and no render arm** — every big number lost its `0d` marker |
| typed-container canon | 3 | **both** engines wrong: Go dropped the element-type tag, TS leaked `word(...)`. `REFERENCE.md:228`/`:1224` settle it as `[:Integer]` |
| marker canon | 1 | **TS was right**: `REFERENCE.md:415` makes `end` the word and `;` its synonym; Go rendered it empty, so `1 ;` reparsed as a bare `1` |
| error text | 1 | the two jsonic ports disagree about whether `1_` lexes as a number; TS's fallback now reproduces Go's classification |
| behavioural | 2 | `1e400` → TS had the `float_overflow` refusal but only on a path `1e400` never took |

| BigDecimal range/scale | 3 | found later by measurement; `core/ts` had a binary64 payload, so `0d1e400` overflowed to Infinity, `0d1e-400` underflowed to **zero**, and `0d0.30` lost its scale |

Worth keeping in view: the `go` column was **not** the reference in two of
the five original classes. The header's warning that it is "the reference by
convention, not by proof" was load-bearing.

Two of the eighteen were invisible to the 1765-row `parser-crossdiff`,
because both engines were erroring-by-rendering rather than disagreeing on a
value. `scripts/parity-probe.sh` is what found them.

The main corpus grew from 370 rows to 648 unique sources while driving
`parser/ts` from 93.97% to 100% line coverage; the final gate also requires
100% branches and functions. The 2026-08-09 probe sweep then grew it to 698.
Both runners machine-pin the current corpus sizes: 711 parse rows, 34
raw-lexer rows, 65 data-decode rows, 18 generated-depth rows, 26
semantic-shape rows, and 9 live divergence rows. An intentional corpus change updates both independent
constants and this prose together — a rule this file has now drifted from
three times in three PRs, because the constants are machine-pinned and the
prose is not. Nothing fails when only the constants move, so re-read this
paragraph against `parser/go`'s and `parser/ts`'s row-count constants
whenever you add rows. The growth found further defects the
crossdiff could not:

- an EMPTY `${}` interpolation in an XML **attribute** folded to `""` in Go
  and stayed a runtime hole in TS — Go's nil/empty conflation, mirrored
  anyway because parity with the shipped language is the contract;
- the eight rule-step-cap shapes above, where TS silently accepted programs
  Go rejects;
- an error raised while CONVERTING `${…}` rendered its two halves in the
  opposite order (`interpolation expression error:` before or after
  `[boru/float_overflow]:`).

Each was found by sweeping the regions the coverage report called
uncovered, which is the general lesson: **an uncovered branch in one port is
where a divergence hides**, because nothing has ever compared the two there.
