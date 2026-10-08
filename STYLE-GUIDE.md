# boru Style Guide

House style for boru source. Where a rule is enforced by `boru fmt`, this
guide says so; where it is not (yet), it says that too, and the guide is
the authority until the formatter catches up.

Style is not taste here. boru's surface deliberately admits several
spellings of the same thing — that is what makes forward arguments and
the signature sugar possible — so a house style is what keeps a codebase
from carrying five spellings of one idea.

Related: [AGENTS.md](AGENTS.md) for the doc map and the tooling,
[REFERENCE.md](REFERENCE.md) for what each form *means*,
[NUR.md](NUR.md) for the recorded non-uniformities.


## S1 — Function signatures: use the fewest square brackets

**Rule.** Write a signature in the spelling with the fewest square
brackets that expresses it. `boru fmt` normalises toward this form.

For the common case — **one parameter, one return** — that is the
three-argument form with a bare input and a bare output:

```boru
def double fn x:Integer Integer [mul 2 x]        ;# YES — 1 bracket pair
```

Not any of these, which are the same function written longer:

```boru
def double fn x:Integer [Integer] [mul 2 x]      ;# no — 2
def double fn [x:Integer Integer [mul 2 x]]      ;# no — 2
def double fn [[x:Integer] Integer [mul 2 x]]    ;# no — 3
def double fn [x:Integer [Integer] [mul 2 x]]    ;# no — 3
def double fn [[x:Integer] [Integer] [mul 2 x]]  ;# no — 4
```

All six are the *same value*, not merely the same answer — `canon` of
each is `deq` to `canon` of any other. The body's brackets are not
optional: the body is always a list.

Both halves are pinned, so this section cannot drift from the engine
without a test failing: the six spellings each computing `42` are spec
rows in [`lang/spec/fn-triple.tsv`](lang/spec/fn-triple.tsv) §2b, and
the five `canon` equalities against the target form are
`TestFnSignatureSpellingsAreOneValue` in
`lang/go/test/fn_triple_compiled_test.go` (a `canon` of a function value
does not compile, and the spec corpus admits no compile failures).

### The one spelling that is not valid

A **bracketed input** is not a longer way of writing a bare one. It
selects the other form entirely:

```boru
def double fn [x:Integer] [Integer] [mul 2 x]
;# error: [boru/fn_error]: fn: list length must be a non-zero multiple of 3
```

`fn` has two signatures — `fn <input> <output> <body>` (three arguments)
and `fn [ …triples… ]` (one list). A **list** in the input position
always selects the second, so `[x:Integer]` is read as a spec list of
length 1, and `[Integer]` and `[mul 2 x]` are left stranded as separate
arguments. `boru describe fn` states the rule: *"The 3-arg form requires
a NON-LIST input … a list input always selects the spec-list form."*

So the input is bare or the whole signature is a list. Never one bracket
pair around the input alone.

### When you cannot reduce it

The three-argument form takes exactly one input item, so these keep the
spec-list form — that is correct style, not a violation:

```boru
def add2 fn [[a:Integer b:Integer] [Integer] [add a b]]   ;# 2+ parameters
def zero fn [[] [Integer] [42]]                            ;# 0 parameters — [] is a list
def m fn [[a:Integer] [Integer] [add 1 a]
          [s:String]  [String]  [s]]                       ;# overloads
```

For overloads, the flat triple spelling is available and shorter when
every input is a single parameter:

```boru
def m fn [a:Integer Integer [add 1 a] s:String String [s]]  ;# same function
```

Prefer whichever of those two reads better for the arity you have: flat
for a few one-parameter overloads, bracketed when the inputs are wide
enough that the triple boundaries stop being obvious.

### Why fewest brackets

Three reasons, in order of weight:

1. **One spelling per idea.** Six ways to write one signature is six
   things a reader has to recognise as the same thing.
2. **The brackets carry no information here.** `[x:Integer]` and
   `x:Integer` in the input slot denote the same single parameter; the
   extra pair is noise that looks like structure.
3. **It matches the formatter.** `boru fmt` already rewrites the fully
   canonical form to the short one, so the short form is the fixed point
   the tool is heading toward.

### Formatter status (as of 2026-08-19)

`boru fmt` implements this rule **only from the fully canonical
spelling**:

| input | `boru fmt` output |
|---|---|
| `fn [[x:Integer] [Integer] [mul 2 x]]` | `fn x:Integer Integer [mul 2 x]` ✅ |
| `fn [[Integer] [Integer] [add 1]]` | `fn Integer Integer [add 1]` ✅ |
| `fn x:Integer [Integer] [mul 2 x]` | unchanged ❌ |
| `fn [x:Integer Integer [mul 2 x]]` | unchanged ❌ |
| `fn [[x:Integer] Integer [mul 2 x]]` | unchanged ❌ |
| `fn [x:Integer [Integer] [mul 2 x]]` | unchanged ❌ |

It correctly leaves the irreducible forms alone (2+ parameters, zero
parameters, multi-overload spec lists).

So `boru fmt` does not yet make this rule self-enforcing: four of the six
spellings survive it untouched, and a file can pass `fmt` while carrying
several spellings of one signature. Recorded as **NUR088**. Until that
lands, apply S1 by hand — running `fmt` is necessary but not sufficient.


## S1b — Lambdas: no outer parens, bare input for one parameter

**Rule.** The same fewest-brackets rule applies to `=>` lambdas. The outer
parens around a lambda are **never** required in a `def` operand position,
and a single typed parameter needs no brackets either:

```boru
def i t:Any => [t/v]                        ;# YES — 1 pair (the body)
def s [a:Integer b:Integer] => [add a b]    ;# YES — 2+ params bracket the input
```

Not these, which are the same lambdas written longer:

```boru
def i ([t:Any] => [t/v])                     ;# no — 2 bracket pairs + parens
def s ([a:Integer b:Integer] => [add a b])   ;# no — the parens add nothing
```

**Calls take no parens at statement level.** Write `i 5`, not `(i 5)`.
Parenthesise a call only when its *result* is an argument to something
else — `print (i 5)`, `add 1 (i 5)` — or to bound a group.

**Why the outer parens are redundant.** `def` has no `afn`/`=>` keyword
slot — `boru describe def` lists 35 signatures and the lambda lands on
the catch-all `[Atom Any]`. `=>` is a *grammar-level* fold: `A => B`
parses as the paren group `(A afn B)` (`lang/go/CLAUDE.md`, "Lambda
Syntax"), so the arrow binds tighter than `def`'s forward collection and
the lambda arrives already whole. Writing your own parens around it just
repeats a grouping the parser has already done.

**The bare form takes exactly ONE parameter, and overrunning it fails
silently.** In `x:Integer y:Integer => body` each pair closes separately,
so the first strands (`lang/go/CLAUDE.md`, "Syntactic gotchas") — and
nothing says so:

```
$ boru do 'def s x:Integer y:Integer => [add x y] end s 2 3'
fn (Integer) {x:Integer} 2 3
$ echo $?
0
```

The stranded `{x:Integer}`, the uncalled fn, and both operands are just
left on the stack. Two or more parameters therefore bracket the input
list, as above — that is not a violation of this rule, it is the
shortest spelling that parses.

`boru fmt` does not normalise any of this either — same gap as S1
(**NUR088**), so it is on the author.


## S2 — Argument order: one rule, and the infix idiom

### The rule

A call binds its arguments **in signature order**. Matching walks the
signature from position 0 and fills each position from the **forward
stack** — the tokens written after the word, in written order — until it
reaches that signature's **barrier**; every position still unfilled is
then taken from the **value stack**, in reverse: the next position gets
the top of the stack, the one after it the next-deeper value, and so on.
(The barrier is the `|` in a parameter list — the last position that may
be filled forward. Most words have none, which means every position is
forward-eligible; collection still stops early at `end`, `)`, another
function word, or a type mismatch, and the stack takes over there.)

That is the whole rule. It is the same rule at every arity: **two-argument
words are not a special case, and there is no "swap form"**. A call form
is only a choice of *where the split falls*. Move the split and you move
operands across the word; write them so each one still lands in the same
signature position and you have written the same call:

```boru
def f fn [[a:Integer b:Integer c:Integer] Integer [...]]

f 1 2 3     ;# forward [1,2,3]                     → a=1  b=2  c=3
3 f 1 2     ;# forward [1,2], stack top 3          → a=1  b=2  c=3
3 2 f 1     ;# forward [1],   stack 2 then 3       → a=1  b=2  c=3
3 2 1 f     ;# forward [],    stack 1, 2, then 3   → a=1  b=2  c=3
```

Two consequences fall out of it, and both are worth naming:

- **Put every argument on the value stack and you get Forth order.** The
  stack is read top-down, so `3 2 1 f` lists the operands back-to-front,
  exactly as a Forth programmer expects.
- **Write every argument forward and you get written order — which, for a
  non-commutative word, reads backwards.** `sub` returns
  `args[1] - args[0]`, so two forward operands bind `args[0]=1,
  args[1]=3` and the call computes `3 - 1`:

```
$ boru do 'sub 1 3'    →   2
```

That is not a quirk of `sub` and not a two-argument exception — it is the
one rule binding the operands in the order they were written.

### The idiom: infix form for infix operators

boru has no operators. `add`, `sub`, `mul`, `div`, `mod`, `pow`, `and`,
`or`, `lt`, `lte`, `gt`, `gte`, `eq` and `neq` are ordinary two-argument
functions, matched by the rule above like every other word. But readers
carry infix habits for exactly these words, and the rule makes the infix
spelling mean what it looks like: the left operand comes off the value
stack, the right operand is written forward.

**Rule.** For a two-argument word that common convention reads as an
infix operator, write it **infix**:

```boru
1 add 2          ;# YES — 3
10 sub 3         ;# YES — 7
n lte 1          ;# YES — "n ≤ 1"
a and b          ;# YES
```

```boru
add 1 2          ;# no — right answer, wrong idiom
sub 10 3         ;# no — and it is -7, not 7
lte 1 n          ;# no — the forward spelling of "n ≤ 1", and nobody reads it that way
```

All three spellings below are legal, and they differ only in where the
operands sit. The first is house style:

```
1 sub 3     ;# infix — forward [3], stack top 1 → args=[3,1] → 1-3 = -2
1 3 sub     ;# Forth order — forward [], stack 3 then 1 → args=[3,1] → 1-3 = -2
sub 1 3     ;# all forward — args=[1,3] → 3-1 = 2, the reversed reading
```

The first two agree because the operands land in the same signature
positions — the split falls in a different place, but `3` reaches
`args[0]` either way. The third differs because writing both operands
forward binds them in written order instead.

Everything else — words that are not read as operators, and every word of
one or three-or-more arguments — takes **forward form**: `f a b c`, with
the arguments in declared parameter order. Reach for a stack-side split
only when an enclosing pipeline has already left the operands on the
stack.

### Converting a call is an edit, not a reformatting

Moving an operand across the word changes which signature position it
lands in, so for a non-commutative word it changes the answer:

```
$ boru do 'sub 10 3'    →   -7        $ boru do '10 sub 3'   →    7
$ boru do 'lte 5 1'     →   true      $ boru do '5 lte 1'    →   false
```

Transcribing a guard from infix habit into forward form inverts it
silently: a factorial written `if (lte n 1) [1] [...]` returns `1` for
every input, exit 0, no diagnostic — the infix `if (n lte 1) [1] [...]`
is the one that means what it says. **Re-run every example you
convert.** `boru fmt` does not touch call form, so this one is entirely
on the author.


## S3 — Passing a function: use `/v`

A bare name bound to a function **calls** it (ADR-011). To pass the
function itself, write `/v`:

```boru
each [f] xs        ;# a quotation naming f
hof f/v xs         ;# f as an argument
```

A bare `f` before a `Function`-typed slot happens to resolve as a
reference today, but that exception is struck from ADR-011 and is
scheduled for removal (**NUR078**). Write `/v` now.

S3 and S4 are the same operator seen from two sides: `/v` always means
"the binding's value, not a call". Passing a function is the case where
that matters most, because a bare name would call it instead.

> **Renamed 2026-08-19.** `/v` was `/r`, and the word `ref` became
> `valof` — deliberately not `val`, which is far too common a local name
> to reserve (it broke four shipped sources in a first pass). The old
> spellings are gone, not deprecated, and both fail loudly: `r` left the
> modifier alphabet, so `f/r` is no longer a modifier at all — it parses
> as an ordinary slash-bearing name and raises
> `[boru/undefined_word]: undefined word: f/r`, exactly as `ref` does.
> `def val …` stays legal.


## S4 — Reading a value: `/v`

A bare name bound to a function **calls** it. `/v` takes the binding's
**value** instead, disabling the call — and it is total over every
binding kind, so it is also how you read a parameter whose kind you do
not know statically:

```boru
def ident t:Any => [t/v]   ;# 5 → 5, "s" → "s", a lambda → the lambda
```

For a non-function binding `/v` is simply the identity (`def n 5` then
`n/v` is `5`), so you never have to know which case you are in. The word
form is `valof`: `valof add` ≡ `add/v`.

The only refusal left is an **unbound** name — there is no value to take.

`(args).N` still reads a parameter positionally (and is the only way to
reach an argument with no name), but it is no longer needed just to get
a value out of a name.

## S5 — Computed keys: parenthesise for the quoting accessors

`get` and `has` evaluate a bare bound key; `set` quotes it. So:

```boru
m get k          ;# fine — get evaluates k
m has k          ;# fine — has evaluates it too, since 2026-08-21
m set (k) v      ;# needed — set still quotes
```

A bare key to `set` is a silent wrong answer, not an error (NUR040's
class). `has` was in that class until it was changed to evaluate its key
exactly as `get` does: a bound bare key now computes, and an unbound one
raises `undefined_word` loudly rather than silently looking up the
literal name.

## S6 — Ending a statement: prefer `;` over `end`

`;` and `end` are the same token — the parser aliases `;` to `end`
(`eng/go/parser/grammar.go`'s `setupValRule`). Both spellings terminate a
statement, and neither is deprecated. Prefer `;`:

```boru
def x 1 ; def y 2 ; x add y      ;# preferred
def x 1 end def y 2 end x add y  ;# same program, heavier
```

It works everywhere `end` does — inline, at the end of a line, and to
close a definition:

```boru
def inc fn n:Integer Integer [n add 1] ; inc 5   ;# 6
```

### Why

- **A terminator is punctuation, not vocabulary.** `end` is a WORD, and it
  sits in the same namespace the reader is scanning for operators and
  bindings; `;` cannot be mistaken for one. This matters most where
  statements are dense — a line of three `end`s reads as three tokens to
  interpret, where three `;` read as structure.
- **It keeps short statements on one line.** `def x 1 ; def y 2` is a
  natural single line; the `end` spelling pushes toward one statement per
  line whether or not that helps.
- **It matches the comment mark already in use.** Every example in this
  guide comments with `;#`, so `;` is already the familiar punctuation.

### The formatter enforces it

`boru fmt` normalises the terminator to `;` (`normaliseStatementEnd`), so
this convention is applied rather than remembered. `end` stays valid input
— it is the same token — but it will be rewritten on the next format.

The rewrite is **positional**, because `end` is a perfectly good name in
two places a spelling test cannot tell apart, and both are left alone:

```boru
def m {end: 1} ;      ;# a map KEY — survives
def q (m dot end) ;   ;# the accessor quotes the following word — survives
```

`m.end` is safe for a different reason: the `.` sugar lexes as one dotted
word, never a bare `end`.

Formatting a file therefore converts it wholesale. That is intended, but
it means a conversion shows up as a reformatting diff — run it as its own
change, not folded into unrelated work.
