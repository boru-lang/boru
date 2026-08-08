# core/spec — the core-level parity corpus

The declarative contract `core/go` and `core/ts` are both held to. One set of
files, two runners (`core/go/corespec_test.go`, `core/ts/src/corespec.test.ts`),
no shared code between them.

## Why this is not `eng/spec`

`eng/spec` is source text. Replaying it needs a parser, and neither core has
one — that is the whole point of the core cut. So the rows here are written in
a deliberately tiny, parser-free notation that each runner implements
independently in ~40 lines.

More importantly, `eng/spec` is an **agreement set**: rows are added where the
two engines already agree, so it cannot see a construct one of them never
implemented. design/CORE-GO-TS-DEFECTS.0.md documents 22 confirmed defects it
is green across, and traces every one of them to that property.

These rows are written from the **documented contract** (REFERENCE.md,
design/TYPES.10.md, design/INTEGER-OVERFLOW-STRATEGY.5.md) rather than from
either implementation's behaviour. The `expected` column is the oracle. When
an engine disagrees with a row, the engine is wrong — that is the difference
between a spec and a differential, and it is why a row here can fail on
*both* engines at once.

## Format

Three tab-separated columns, `#` starts a comment line:

```
expr	expected	note
```

`expr` is `<kind> <argument>`:

| kind | argument | builds |
|---|---|---|
| `int` | a decimal integer | an Integer value |
| `str` | the rest of the line, raw | a String value |
| `bool` | `true` / `false` | a Boolean value |
| `none` | — | the None value |
| `typelit` | a builtin type name | that type's literal value |
| `list` | space-separated tokens | a List of them |
| `run` | space-separated tokens | runs them through the step loop |

`run` tokens: a bare decimal is an Integer, `'…'` is a String, a known builtin
type name is that type's literal, and anything else is a Word. The runners
register one fixture word, `addq` (`Integer Integer -> Integer`), so the step
loop, the registry, signature matching and dispatch are all exercised without
pulling in a word library.

### Structure tokens

Three bracket forms assemble a nested VALUE out of the flat token stream, so
a row can hand the step loop the containers a parser would have built. They
nest, and they work in `run` and `list` alike.

| form | builds |
|---|---|
| `[ … ]` | an **eval** list — what the parser emits for a `[…]` literal |
| `[q … ]` | a **quoted** list — the same elements with evaluation off |
| `{ … }` | an **eval** map — what the parser emits for a `{…}` literal |
| `{q … }` | a plain map, as a word handler would return one |
| `p( … )` | a paren-EXPRESSION value — the deferred form a map value takes |

The bare bracket is always the PARSER's form and the `q` variant the
runtime's, because that distinction is load-bearing: the step loop
auto-evaluates a container only when the parser marked it `Eval`, so
`{ a: [ addq 1 2 ] }` resolves to `{a:[3]}` and `{q a: [ addq 1 2 ] }`
stays `{a:[word(addq) 1 2]}`. A row that could not say which it meant
could not pin that rule.

Inside a map the items alternate: a token ending in `:` is a KEY and the
item after it is that key's value, so `{ a: 1 b: [ addq 1 2 ] }` is a
two-entry map whose second value is an eval list. Every bracket is its own
whitespace-separated token — `[1 2]` is not a list, `[ 1 2 ]` is — because
the notation has no lexer and is not meant to grow one.

`(` and `)` stay what they were: the paren MARKER values the step loop
consumes inline, not a container. `p( … )` is the other thing a paren can
be — one VALUE holding a deferred expression, which is what a map value
like `{a: (1 add 2)}` actually is. The two need separate spellings because
the notation has no context to tell them apart, and the distinction is
real: markers are consumed by the step loop, a paren-expr value is
evaluated by whatever consumes the container.

`expected` is the canonical rendering (`core.Canon` / `canon()`), or
`ERROR:<code>` for a row that must raise that BoruError taxonomy code.

## What is deliberately absent

Rows for the 22 defects in design/CORE-GO-TS-DEFECTS.0.md are **not** here
yet. They would fail today — several on both engines — and a corpus that
ships red is a corpus people learn to ignore. That document lists each one
with the row text that catches it; the rows land with the fixes.
