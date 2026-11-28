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
carry call-site modifiers after a slash, in their one written order — an
optional `N` (exactly N arguments), at most one of `s` (stack only) or `f`
(forward only), `u` (the bound function with its argument order reversed)
and `v` (the bound value, never a call): `name/s`, `name/2f`, `name/u`,
`name/uv`, `name/v` —
which build the modified word a parser would; a slash whose suffix is not
a modifier (`Scalar/String`) is part of the name. The runners
register a small fixture vocabulary — `addq` (`Integer Integer -> Integer`)
and a word per mechanism after it, each the smallest that reaches one
dispatch shape — so the step loop, the registry, signature matching and
dispatch are all exercised without pulling in a word library. (Each is
declared, with its reason, in the runners. One carries a shipped word's
name on purpose: `afn n [q body… ]` builds an anonymous one-parameter
function at run time, which is where closure capture happens, and the
kernel's frame analysis recognises a function-building body by that name.)

### The looping words

Three kinds of loop reach the tape, and each has a fixture word over the
kernel's own mechanism — the words that build a loop's tokens are basic's,
the continuation and its stepping are core's:

| word | the loop |
|---|---|
| `eachq xs [q body ]` | DRIVEN: a sealed region per element, the element its one inert input; everything a region leaves goes into the one list the loop answers |
| `callq x [q body ]` | a CALL REGION: a driven loop of one round, `x` its one inert input, whose residual is the word's own results — stepped again where the word stood |
| `forq n [q body ]` | COUNTED: the body for `i` = 0 … n-1, the index bound as `i` |
| `rangeq start end step [q body ]` | COUNTED over any range: the bound is exclusive; a zero step is `ERROR:fixture_zero_step` |
| `whileq [q cond ] [q body ]` | CONDITIONAL: a condition region and a body region in turn; the condition's last value decides |

A counted or conditional round is `mark body… move` with no paren around
it: the body runs on the stack the loop's word stood on, and the loop's
values are stepped where it stood. A row that needs a condition to CHANGE
binds one name several times — `def c false ; def c true ;` — and has the
body pop it with `undef c`.

A move does two things that are no loops, and each has its word:
`ifq [q cond ] [q then ] [q else ]` is the `if` CONTINUATION — a condition
region whose last value picks the branch, a paren group, that takes the
region's place — and `replayq [q body ]` is the plain mark and move, which
runs its body and then runs it once more in the place of everything the
first run left.

Two more words raise the FLOW SIGNALS, which are not errors: `breakq` and
`contq`. The nearest counted or conditional loop ahead on the tape takes
one — break ends the loop with what its completed rounds left, continue
drops the round — and it passes through a group, a container literal, a
call frame and a driven loop to reach it. With no loop to take it, it is
`ERROR:flow_error`.

### Binding at run time

A row's leading `def NAME item ;` clauses bind before the program starts,
and bind the item as it is written. Anywhere else `def` is a WORD —
`def name value` — over the kernel's own definition INSTALL
(`InstallDef`): a plain value is bound; a function is COMPILED under the
name — its optional parameters expanded into signatures, an older
definition of the same parameter types replaced, its body asked then
whether its frames record their entry state. (In the core/ts runner the
word is the plain binding: core/ts has no install.) It is what lets a
function's body make a definition of its own. It carries basic's name on
purpose, as `afn` and `undef` do: the kernel decides BY NAME whether a body
can bind (`frameStateWords`), and gives a body that cannot a frame with no
record of what to undo. `bindq name value` is the same binding under a
name that list does not hold, and stands for every word that binds from
the host without being on it (lang's `unpack` is the shipped one).

### A function written as a specification

`fn [q [q params ] [q returns ] [q body ] … ]` is the kernel's
FUNCTION-SPEC READER (`ParseFnDef` and the readers beneath it): triples of
a parameter list, the returns and a body, one per overload, read into the
function they describe. A parameter is the single word `name:Type`
(`name?:Type` optional, `name:Type/q` a captured name, `name:kw/q` a
keyword), a bare type, or an Integer or String the slot admits exactly;
`|` is the forward barrier and `?` makes the parameter before it
optional. A return is a type or a pattern — `d( Integer String )`, `7`.
The word answers the function as the reader leaves it; binding it with
`def` installs it, and since a nested call in a forward window is not
planned, a row writes `def f ( fn [q … ] )`. Every refusal of the reader
is `ERROR:fn_invalid_spec`. The Go runner's and the Lean runner's only.

### A call's arguments

Two words reach what a call keeps of its arguments beside the bindings.
`argsq` answers the ARGS LIST — every argument of the call a body is
running in, named or not, in signature order — and is
`ERROR:fixture_no_args` where there is no call (basic's `args` word
answers `[]` there; the fixture keeps the two apart). `__pa` is the pop
of that list, and of the frame's baseline with it, AS A WORD. No frame
runs it: the kernel's frame close has done the pop itself since
2026-10-08, and the word outlived its use (basic still registers it). It
is in the vocabulary for the dispatch rows that want a word of no
results — and, being a word, a row can call it in a body, where the
frame's close then pops a second time (NUR405).

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

A function is equal only to ITSELF, so two `fn( … )` literals alike in
text do not unify (`unify.tsv` pins it). A row that needs one function
twice reaches it through a reference — `name/v` — and one binding read
twice IS one function; `unify.tsv` pins that beside it.

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
| `fn( NAME PTYPE … RTYPE [ body ] )` | a function value: its parameters in signature order — a name and a type each — then one return, and a boru body. All of them may be collected forward. The types are the seven the notation spells as literals, and `Number` and `Scalar`, so a row can declare a chain of types each above the next. The name `_` is an UNNAMED parameter: its argument is not bound, and enters the frame as a value |

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

It holds **500 rows**. 471 are a mechanism `core/ts` does not have at
all: closures (26 rows, the `afn` fixture); driven loops (35 rows, the
`eachq` fixture — eight of them the same loop out of a group and inside one
a waiting word evaluates, where the kernel once stepped the loop's inert
inputs); call regions (26 rows, the `callq` fixture); counted and conditional loops (73 rows, the `forq`, `rangeq` and
`whileq` fixtures — six of them NUR400, a full-stack word ending a counted
loop, and five the index at the edge of the Integer range, two of those
NUR402, all pinned as the kernel has it); break and continue (48 rows, the
`breakq` and `contq` fixtures); the `if` continuation and the plain mark
and move (47 rows, the `ifq` and `replayq` fixtures — three of them NUR400
again); the arity widening of a value call
(4 rows, two of them NUR401, pinned as the kernel has it); and an unnamed
parameter's argument as the frame's own stack data (53 rows — `core/ts`
binds it under the empty name — three of them NUR404, pinned as the kernel
has it), with 11 more that read the args list or call `__pa` from inside a
loop, a region, a branch or a closure; the frame's ENTRY SNAPSHOT, which
ends a body's own definitions with the call (28 rows — `core/ts` leaves
them bound — and 4 more that bind in a round, a region or a branch outside
a function); a function that calls itself last, through a branch (13
rows); the deferred residual of an anonymous function's one bare
container (9 rows); an alias of a binding word in the body of a
function value (6 rows); and the function-spec reader and the definition
install (88 rows, the `fn` fixture and `def` over a function — two of
them NUR409, two NUR410 and three NUR411, pinned as the kernel has it). Two are a difference in a mechanism both have:
`core/ts` puts a list argument's quoted BINDING in the args list, where
the kernel puts the argument. Two are a diagnostic: the kernel reports a
void argument of the word named `def` as `def_error`. **Two are NUR407,
and are the only rows where the Lean specification agrees with the `ts`
column**: core/go's tail-call elision changes the answer of a callee that
unbinds its own parameter, and `core/ts`, which only nests, gives the
nested answer — `TestCoreSpecTailCallsUnobservable` in `core/go` runs every
row of the corpus with elision off and holds exactly those two to it. One
is a
recorded kernel defect pinned as the kernel has
it — NUR392, an unmatched `(` in a function's body — where `core/ts`
refuses what Go lets through. The other 22 are what a Go-against-TS search
of the fixture words, and the review of the port, found once the engine
half had closed, each named in the file: the forward barrier's commit half
(a `/u` word included), a type-arg slot that plans a word, the re-dispatch
of a collection the run's end finds unfilled, a function value that parks,
a frame's residual stepped again, a `/uv` word in a boru-bodied call's
window (two rows, on NUR390's accident), a failed plan retried from the
stack (three rows, on NUR398), and one asymmetry in the `undef`
fixture. 135 were measured on 2026-08-08,
none of them visible to the 1808-row engine crossdiff; the file grew to 219
as the Lean model's differential search, the `d( )` / `[: ]` / `{: }`
forms, the `/v` and `/u` modifiers, `afn` and `eachq` opened new surface,
and both halves of the port closed on 2026-11-27. 66 rows went when
`core/ts` gained a real unifier (the meet, not a flat predicate), Go's
canonical form for a resolved disjunct, the whole builtin type table, and
leaf-name rendering of a function's types. 99 went with the ENGINE: the
call frame and its return check, a function value standing in the program,
the `/v` and `/u` modifiers and the function identity a reference carries,
the `end` word, an unquoted list binding, and an unmatched `(` at the top
level and in a container. Of the original 135, the last two went
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
