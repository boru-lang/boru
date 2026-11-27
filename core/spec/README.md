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
implemented. design/legacy/CORE-GO-TS-DEFECTS.0.ignore documents 22 confirmed defects it
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
| `float` | a decimal number | a Float value |
| `str` | the rest of the line, raw | a String value |
| `bool` | `true` / `false` | a Boolean value |
| `none` | — | the None value |
| `typelit` | a builtin type name | that type's literal value |
| `list` | space-separated tokens | a List of them |
| `run` | space-separated tokens | runs them through the step loop |

`run` tokens: a bare decimal is an Integer, `'…'` is a String, a known builtin
type name is that type's literal, and anything else is a Word. A word may
carry call-site modifiers after a slash — `name/s` (stack only), `name/f`
(forward only), `name/N` (exactly N arguments), `name/Ns` / `name/Nf`, and
`name/v` (the bound value, never a call) —
which build the modified word a parser would; a slash whose suffix is not
a modifier (`Scalar/String`) is part of the name. The runners
register a small fixture vocabulary — `addq` (`Integer Integer -> Integer`)
and a handful like it, each the smallest word that reaches one dispatch
shape — so the step loop, the registry, signature matching and dispatch are
all exercised without pulling in a word library.

### The unifier's two words

Unification has no client in a bare core registry (`is`, `case` and typed
defs are basic-layer words), so two fixture words are the door onto it.
Both take two arguments, forward, and sit directly over the kernel's own
unifier — Go `UnifyR`, TS `unify`, Lean `unify`:

| word | returns |
|---|---|
| `unifyq a b` | the PREDICATE: `true` when the two values unify, `false` when they do not |
| `meetq a b` | the VALUE: the unified value itself — the meet of the two — or `ERROR:fixture_no_meet` when they do not unify |

`meetq` is the one that states a rule. `d( 5 6 )` against `Integer`
unifies whether the answer is `5` or `5 tor 6`, so the predicate cannot
tell the right meet from the wrong one; `core/spec/unify.tsv` closes with
the value rows for that reason. (It arrived late: `core/ts` had a flat
predicate and no meet until the unifier was ported, and a question one
engine cannot answer pins a port gap as a language contract.)

One caution for new rows: a function VALUE unifies by identity in
`core/go` — two structurally identical `fn( … )` literals do not unify —
so keep functions out of `unifyq` / `meetq` rows until that is settled.

### Structure tokens

A few bracket forms assemble a nested VALUE out of the flat token stream, so
a row can hand the step loop the containers and types a parser would have
built. They nest, and they work in `run` and `list` alike.

| form | builds |
|---|---|
| `[ … ]` | an **eval** list — what the parser emits for a `[…]` literal |
| `[q … ]` | a **quoted** list — the same elements with evaluation off |
| `{ … }` | an **eval** map — what the parser emits for a `{…}` literal |
| `{q … }` | a plain map, as a word handler would return one |
| `p( … )` | a paren-EXPRESSION value — the deferred form a map value takes |
| `d( … )` | a DISJUNCT of the listed items (`d( )` is the empty one) |
| `[: T … ]` | a typed list: the child constraint, then any retained elements |
| `{: T k: v … }` | a typed map: the child constraint, then any retained entries |
| `fn( NAME PTYPE RTYPE [ body ] )` | a function value: one parameter, one return, a boru body |

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

### `divergent.tsv` — the parity debt

```
expr	go	ts	note
```

Expressions the two engines answer DIFFERENTLY. Each runner asserts its
**own** column, so both suites stay green while the difference stays pinned:
if either engine moves, its column fails and the row is re-examined rather
than silently re-baselined. Each runner also **fails a row whose `go` and
`ts` columns are equal** — a fixed divergence must move to the spec file it
belongs in, or this stops being an honest debt list. Shrink-only, exactly as
`parser/spec/divergent.tsv` is.

It holds **30 rows** — every one of them in the ENGINE: the call frame's
return check, a function value standing in the program, the `/v` modifier,
the `end` word, an unquoted list binding, and an unmatched `(`. 135 were
measured on 2026-08-08, none of them visible to the 1808-row engine
crossdiff; the file grew again to 95 as the Lean model's differential
search and the `d( )` / `[: ]` / `{: }` forms opened the unifier, and 65 of
those closed on 2026-11-27 when `core/ts` gained a real unifier (the meet,
not a flat predicate), Go's canonical form for a resolved disjunct, the
whole builtin type table, and leaf-name rendering of a function's types.
Of the original 135, the last two went
together: both `boomq { a: ) }` rows were filed under the strict forward
barrier and were really a different defect — core/ts resolved a map
argument's values through `resolveWordsDeep` where Go runs each one as a
PROGRAM, so a value that could not run was carried into the consuming word
instead of faulting from inside the map. `design/CORE-TS-DIVERGENCES.1.md`
has the root causes; the headline is that one class produces a WRONG ANSWER
rather than a wrong error (`7 8 addq ( ) ( 5 )` is `15 5` in Go and `7 13`
here), and the largest class is the strict forward barrier that `core/ts`
does not implement at all.

How they were found is the reusable part. `core/ts` was at 88.2% coverage
against `core/go`'s 100%, and the parser half of this programme had already
established that **an uncovered branch in one port is where a divergence
hides** — nothing has ever compared the two engines there. Sweeping each
uncovered region produced 139 candidate expressions, of which **135 diverged**.
Pinning them took `core/ts` from 88.20% to 90.71% without a line of new
engine code, and the same sweep run to its end took it to **99.58%** —
finding, among others, that `/q` word capture is conditional on the slot
type, that an unfilled forward marker was escaping as a program result,
and that check mode's undefined-word placeholder was being stepped past
before anything could consume it.

## What is deliberately absent

Rows for the 22 defects in design/legacy/CORE-GO-TS-DEFECTS.0.ignore are **not** here
yet. They would fail today — several on both engines — and a corpus that
ships red is a corpus people learn to ignore. That document lists each one
with the row text that catches it; the rows land with the fixes.
