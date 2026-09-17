# boru Explanation

This document explains the ideas behind boru — the *why* behind the
syntax, the type system, and the runtime. It complements the
**[Tutorial](TUTORIAL.md)** (learning), **[How-To Guides](HOWTO.md)**
(tasks), and **[Reference](REFERENCE.md)** (precise behaviour).

> **Notation.** In code, a `# returns …` comment shows what an
> expression evaluates to (`square 4  # returns 16`); in prose we say
> "`square 4` returns `16`". The comment is ordinary documentation, not
> special syntax. (boru has no result arrow — `=>` is the
> anonymous-function word `afn` — so results are written as comments.)

## Contents

* [What is a concatenative language?](#what-is-a-concatenative-language)
* [The stack model](#the-stack-model)
* [Forward collection: beyond reverse Polish](#forward-collection-beyond-reverse-polish)
* [The `end` keyword](#the-end-keyword)
* [Type-directed dispatch](#type-directed-dispatch)
* [Function signatures and refinement types](#function-signatures-and-refinement-types)
* [Tail calls and the tape](#tail-calls-and-the-tape)
* [Type ordering](#type-ordering)
* [Immutability and mutability](#immutability-and-mutability)
* [Quotation and evaluation](#quotation-and-evaluation)
* [Macros](#macros)
* [The Options pattern](#the-options-pattern)
* [Parallel execution model](#parallel-execution-model)
* [Errors as values](#errors-as-values)
* [Store and context](#store-and-context)
* [Module system](#module-system)
* [Ideals and type-kinds](#ideals-and-type-kinds)
* [Generics as memoised type construction](#generics-as-memoised-type-construction)
* [Capabilities](#capabilities)
* [The CLI: two surfaces](#the-cli-two-surfaces)
* [The vault: why a local credential store](#the-vault-why-a-local-credential-store)
* [Design influences](#design-influences)


## What is a concatenative language?

In most languages you compose functions by *nesting* them — `f(g(x))`.
In a concatenative language you compose by *placing them next to each
other* — `x g f`. Every word is a function on the stack, and the
output of one is the input of the next. Composition is concatenation.

```
def double word [dup add]
def quadruple word [double double]
5 quadruple                       # returns 20
```

`quadruple` is not "calling double twice" in the usual sense. It is
the textual juxtaposition of `double` with itself: a new program
that *is* the composition. There is no syntactic overhead for
combining operations — no parentheses, no `compose(...)`, no `.then`.


## The stack model

boru has a single data stack. Every literal pushes; every word pops
its arguments and pushes its results. The stack is the implicit
data flow.

```
3 4 add                           # returns 7
7 2 mul                           # returns 14
```

Step by step: push 3, push 4, `add` consumes both and pushes 7; then
push 2, `mul` consumes both and pushes 14. There is no `tmp = a + b`
intermediate — the stack *is* the intermediate.

Written as one line the two steps compose by parenthesising the first
— `(3 4 add) 2 mul` returns `14`. The bare `3 4 add 2 mul` does **not** give
14: because `add` can also *collect forward* (the next section), it
grabs the following `2` as a second argument — computing `4 add 2 =
6` — which leaves `3` and `6` on the stack for `mul`, giving `18`.
When a trailing word would otherwise reach forward past the value you
mean to leave on the stack, insert a barrier: parenthesise the first
step — `(3 4 add) 2 mul` — or stop the collection with `end` or `;`
(`3 4 add ; 2 mul`). All three give `14`. A newline is **not** a
barrier: in a file `3 4 add` followed by `2 mul` on the next line still
evaluates to `18`, because the stack and forward collection both carry
across line breaks. (The REPL is the exception — it evaluates and
clears the stack line by line, so `3 4 add` then `2 mul` there prints
`7` and then errors for want of a second operand.)

This eliminates the need for variable binding in simple cases. When
naming actually helps readability, `def`, `var`, and named-parameter
`fn` are there. The default is *point-free composition*.


## Forward collection: beyond reverse Polish

Traditional concatenative languages (Forth, Factor) use strict
reverse Polish notation: arguments always *precede* the word. boru
extends this with **forward collection**: a word can gather
arguments that appear *after* it.

```
1 2 add                           # returns 3 — classic prefix
add 1 2                           # returns 3 — fully forward
1 add 2                           # returns 3 — infix: one stack, one forward
```

All three are equivalent. The word `add` needs two arguments. If
fewer are on the stack when it runs, it enters a forward-collecting
mode and consumes following tokens until its signature is filled.

This lets boru read naturally in infix position. `10 sub 3` reads
"ten minus three"; `not true` reads "not true"; and with the string
module imported, `StringUtil.upper "hello"` reads "uppercase hello".
You never have to mentally reverse-engineer `10 3 -`. (String words
like `upper`/`lower`/`split` are not built in — they live in
`boru:string-util`; see the [Reference](REFERENCE.md). Only words such
as `add`, `sub`, `mul`, `not`, `dup` are available without an import.)

### Forward by default — a cultural rule

Forward collection is not just available, it is the **language
default**, pinned as [ADR-004](ADR.md#adr-004): every word — core,
module, and your own `fn` definitions — collects forward unless its
semantics are intrinsically about the stack. The only standing
exception is the traditional Forth stack vocabulary (`dup`, `swap`,
`drop`, `over`, `rot`, …), which is stack-only because manipulating
the stack *is* what those words mean.

Two practical consequences:

- **Write the forward form first — except for the operators.**
  `word arg1 arg2` is the canonical call shape in code, docs, and
  examples; the pipeline form (`arg2 arg1 word`) is an equivalent you
  reach for when a value is already flowing on the stack. The one
  standing exception is the set of two-argument words common
  convention reads as infix operators (`add`, `sub`, `mul`, `div`,
  `mod`, `pow`, `and`, `or`, `lt`, `lte`, `gt`, `gte`, `eq`, `neq`),
  which house style writes infix: `1 add 2`, `10 sub 3`
  ([STYLE-GUIDE.md §S2](STYLE-GUIDE.md)).
- **Don't ask for a word to be flipped.** When forward collection is
  awkward at one call site, the per-call levers are grouping
  (`(…)`, `end`, `;`) and the modifiers `/s` (stack-only here),
  `/f` (forward-only here), and `/N` (exactly N args) — not a change
  to the word's default. `(add 1 1) print/s`, for instance, prints a
  value that is already on the stack.

### How collection works

When a word executes, boru fills its argument slots in this order:

1. **Forward first.** Walk the tokens after the word in source
   order, left to right. Each token is evaluated and its type
   checked against the next-to-fill slot. If the type matches, the
   value goes into `args[0]`, then `args[1]`, …, and the walk
   continues. If the type doesn't match, or the walk hits a barrier
   (`end`, `)`, another function word), forward collection stops.
2. **Stack second.** Any slots still empty are filled from the
   stack, top of stack into the next-to-fill slot first, then the
   next-deeper value, and so on.

That is the entire rule, and it is the same rule for every word at
every arity. Two-argument words are **not** a special case, and there
is no "swap form" — both are legacy misreadings that made a single
rule look like a family of them. A call form chooses only where the
split between the two phases falls.

The two phases explain the two orders a newcomer notices. Put every
argument on the stack and step 2 does all the work, reading top-down,
so you get Forth order (`10 3 sub`). Write every argument after the
word and step 1 does all the work, in the order you wrote them — so
for an asymmetric word the reading inverts: `sub 1 3` binds
`args[0]=1, args[1]=3` and returns `2`.

So `args[0]` is whichever argument is closest to the word in source
position (or the deepest forward arg if all came from the right);
`args[N-1]` is the furthest. Handlers can rely on a single
positional contract regardless of how the user wrote the call.

For an asymmetric operation like `sub` (handler returns
`args[1] - args[0]` — deeper minus top), this single rule produces
the same answer for every call form:

```
10 3 sub        # all-stack:   args[0]=top=3, args[1]=10  →  7
10 sub 3        # mixed:       args[0]=3, args[1]=10       →  7
sub 3 10        # all-forward: args[0]=3, args[1]=10       →  7
```

After rearrangement, the word always sees arguments in signature
order. This is why the body doesn't have to care which side they
came from.

### Type-directed collection

Forward collection respects types. Consider:

```
not true 42
```

1. `not` needs one `Boolean`. Stack is empty. Enter forward mode.
2. `true` matches `Boolean`. Collected. `not` runs → `false`.
3. `42` is not consumed (no waiting word). It is pushed.

Result: `false 42`. Type matching governs *how far* a word reaches,
but it does **not** make a word reject an argument of a type one of
its signatures accepts. `add`, for instance, has both a numeric
signature and `String`-concatenation signatures, so a string is a
*valid* second argument: `add 1 "x"` does not stop and fail — it
collects `"x"` and concatenates, giving `'x1'`. Forward collection
narrows where the boundary falls; it is not a guarantee that a
"wrong-looking" value will be refused. (Concatenation does require a
`String`, though: that constraint lives in `add`'s signatures — see
below — not in forward collection.)

### Forward greediness and stranded operands

Forward collection reaches *past* values already on the stack, which can
surprise you when a binary word sits between its operands:

```
1 2 add 3 mul                     # returns 5 — not the 9 you might expect
```

Reading left to right you might expect `(1 + 2) * 3 = 9`. Instead `add`
reaches forward for `3` and takes `2` from the stack — computing `3 + 2 = 5`
and leaving the `1` stranded underneath. `mul` then multiplies `5 * 1 = 5`.
Group the operands you actually mean to combine:

```
(1 2 add) 3 mul                   # returns 9
```

`boru check` surfaces the suspicious case as a non-gating advisory:

```text
$ boru check -e '1 2 add 3 mul'
check: [info] forward_strands_operand: add collected a forward argument
  while a Number operand was left unconsumed on the stack — it may be
  stranded; group the intended operands, e.g. (… add …)
```

It fires only when a word both reaches forward *and* takes a stack argument
while leaving a **sibling** operand (a value of the same type it just
consumed) behind — so the idiomatic infix form `10 sub 3` and a deliberately
deeper stack such as `1 2 3 add` stay quiet.


## The `end` keyword

`end` is the escape hatch for the rare case where forward collection
would go too far. It stops the nearest waiting word: collection
ceases, any unfilled argument slots fall back to the stack, and the
word runs with what it has:

```
import "boru:math-util" MathUtil.sqrt 16        # returns 4.0
```

`import` accepts optional arguments after the module id — a rename
list, another path — so without `end` it reaches forward and tries
to take `MathUtil` as one of them. That fails with `undefined word:
MathUtil`, because the module that would define the name hasn't
loaded yet. With `end`, `import` stops at the module id, runs,
installs the `MathUtil` namespace, and `MathUtil.sqrt 16` evaluates
next.

It's needed less often than you might think — type-directed
collection cuts most of the cases — but when the type system can't
disambiguate (e.g., two adjacent words that both happen to accept
the same type), `end` is the simple, explicit fix.

A **function word** beginning the next statement acts as an implicit
`end` for a waiting word that can already fire. The practical case is
the else-less guard: in

```
def f fn [[n:Integer] [Integer] [
  if (n eq 0) [raise "n must not be zero"]
  def q (10 div n)
  q
]]
```

`if` has an optional third (else) argument, but `def` starting its
own statement commits the two-argument form first — the guard raises
*before* the `def` body can divide by zero, and a value-producing
statement after the guard keeps its result instead of being
swallowed as a phantom else. Only a **value** (a literal, or a
parenthesised expression like `if (c) [t] (e)`) following the
then-branch is taken as the else.

### A word only reaches for what it can use

Collection is *structure-first*: a word forward-collects a following token
only when one of its signatures could actually take it. Anything else — a
parenthesised expression, or a value whose type fits none of the word's
remaining argument slots — is left alone to run as its own expression. This
is why an unterminated `import` no longer swallows the code that uses it:

```
import "boru:string-util"          # no `end` needed…
(StringUtil.upper "hi")           # …this paren runs on its own → 'HI'
```

`import` takes its module path and stops, because the following
parenthesised expression matches none of its argument shapes. (Earlier
builds eagerly evaluated that paren *before* `import` ran, so it failed with
`undefined word: StringUtil` — see `design/LAZY-ARG-RESOLUTION.10.md`.) You
still reach for `end` when the next token *could* legitimately be the word's
argument — most commonly a second string path right after `import`:

```
import "boru:math-util" "foo" print    # without `end`, import would load "foo"
```

An empty paren `()` is the empty expression: it produces no value, so it
contributes nothing where it appears (`5 () add 3` → `8`; the `()` is
simply skipped). Empty parens nest freely: `(())`, `( () )`, `(add 1 ())`
all parse.


## Type-directed dispatch

Every value in boru carries a hierarchical type. The type
`Scalar/Number/Integer` is a child of `Scalar/Number`, which is a
child of `Scalar`, which is a child of `Any`. A child matches its
parent; the reverse is false.

Words declare *signatures* — patterns over types. When a word has
multiple signatures, the engine tries each in order and uses the
first that matches. This produces natural overloading without a
separate dispatch construct:

```
add 1 2                           # returns 3 — Integer + Integer
add 1.0 2                         # returns 3.0 — Float + Number, promotes
"a" add "b"                       # returns 'ab' — String + Scalar, concatenates
add 1 "x"                         # returns 'x1' — Scalar + String, the Integer coerces
add true 1                        # signature_error — no overload takes Boolean+Integer
```

The same `add` covers numeric addition and string concatenation —
not because it has an `if isString` inside, but because its signatures
match different argument shapes. Concatenation lives in two overloads,
`[String, Scalar]` and `[Scalar, String]`, so it fires only when *at
least one* operand is a `String`; a `Boolean`+`Number` pair matches
neither and is a `[boru/signature_error]` dispatch miss rather than a
silent stringification.

This makes the type system *active*: it isn't just for verification,
it drives behaviour.


## Function signatures and refinement types

A `fn` declares types for both its inputs and its outputs:

```
def avg fn [[a:Number b:Number] [Float] [(add a b) div 2.0]]
```

Inputs are checked when the function is called; outputs are checked
when its body finishes. The important principle is that these are not
two different checks — **a value is accepted at a parameter slot, a
return slot, or by the `is` word using one and the same membership
rule**: *is this value a member of that type?* Asking the same
question at every boundary is what keeps the language honest — a
function can't accept a value its own signature would reject, and a
value that flows out of one function flows into the next without
surprise.

What "member of that type" *means* depends on the kind of type, and
user types built with `refine` come in two kinds that answer it
differently. boru keeps both, because they correspond to two genuinely
different intentions — and the rest of the world splits the same way.

**A bare refinement is a newtype.** `def UserId (refine Integer)`
gives you a distinct name with no new constraint. The point is
*identity*, not validation: a `UserId` and a `ProductId` are both
integers underneath, but mixing them is a bug. So a bare `Integer` is
**not** a `UserId` — you have to say so explicitly (`def id:UserId
42`). This is exactly how `newtype` works in Haskell, tuple structs in
Rust, defined types in Go, and opaque types in Scala: the wrapper is
deliberate, required at every boundary, symmetric.

**A predicate refinement is a subset type.** `def Big (Integer gt
10)` carves a subset out of the integers by a *predicate*. Here the
point is *validation*, not identity: any integer over ten already
qualifies, with nothing to construct. Membership is the predicate, and
it is checked the same way going in (a parameter) and coming out (a
return). This is how subset types work in F\*, Liquid Haskell, Dafny,
and Ada's range subtypes: value-sensitive and symmetric.

```
def UserId (refine Integer)               # newtype — identity
42 is UserId                  # returns false — a raw Integer is not a UserId
def id:UserId 42   id is UserId  # returns true — constructed explicitly

def Big (Integer gt 10)                   # subset — validation
50 is Big                     # returns true — 50 qualifies, no construction
5  is Big                     # returns false — 5 does not
```

The trap boru avoids is treating these asymmetrically — lenient on the
way in, strict on the way out (or vice-versa). No mainstream language
does that on purpose; each picks one discipline per kind and applies
it at every boundary. boru does the same: newtypes are nominal and
symmetric, subset types are value-sensitive and symmetric. The full
rationale is in `design/REFINE-NEWTYPE-VS-SUBSET.10.md`.


## Tail calls and the tape

boru guarantees tail-call elimination — the precise conditions are in
**[Reference: Recursion and tail calls](REFERENCE.md#recursion-and-tail-calls)**.
The mechanism falls out of the execution model rather than being
bolted on.

The engine runs a program on a single *tape* of tokens and walks a
pointer across it. A function call does not push a frame onto a call
stack — it **splices the body into the tape** at the call site,
bracketed as a frame: an open paren, the body, a synthesized cleanup
tail (un-define the parameters, pop the per-call state), and the
close paren. The tape *is* the continuation: everything beyond the
pointer is exactly the work that remains.

That makes tail position visible rather than inferred. At the moment
a call dispatches, it is a tail call precisely when the only tokens
between it and the enclosing frame's close are that frame's own
cleanup tail — "nothing left to do afterwards" is literally readable
off the tape. And because the callee's arguments are concrete values
by dispatch time, and closures snapshotted their captures at
construction, the caller's cleanup can simply run *early*: the engine
executes the parked tail now and the callee's frame takes over the
caller's region. Frames replace instead of stacking, so depth never
accumulates — elimination is a reordering of work the tape had
already scheduled, not a new semantics.

One boru-specific boundary shapes the conditions. Name resolution is
dynamic — an enclosing frame's bindings stay visible to everything it
calls, until the frame exits. The locally-defined-recursive-fn idiom
depends on that:

```
def out fn [[] [Integer] [def go fn [[n:Integer] [Integer] [if (n lte 0) [99] [go (n sub 1)]]] go 3]]
out                               # returns 99
```

`go`'s body finds `go` through the *enclosing frame's* binding (the
name was unbound when the inner fn was constructed — the standard
forward-reference idiom). A frame holding a binding its callee does
not re-bind must therefore stay alive: the initial `go 3` call nests,
while `go`'s own self-recursion — which re-binds everything its
frames hold — eliminates. This is why the guarantee asks the callee
to re-bind what the caller's frame binds: elimination must be
unobservable, and under dynamic resolution "unobservable" includes
every name a deeper callee might still read.

The failure taxonomy at the limits is a corollary. An infinite tail
loop re-uses one frame forever — pure CPU, caught by the step budget
as `[boru/evaluation_limit]`. Unbounded non-tail recursion grows the
tape — caught by its growth ceiling as `[boru/tape_exhausted]`. Each
guard names the resource the program actually consumed.

## Type ordering

boru has a single total order over every value, computed in two stages:

1. **LCA-Comparer.** Find the least common ancestor of the two
   types. If the ancestor declares a comparer, use it (so
   `Integer`↔`Float` runs the numeric comparer at the `Number`
   level, and the instant-bearing `Time` leaves —
   `Date`/`DateTime`/`Instant` — compare chronologically at the
   `Time` level).
2. **Rank fallback.** Otherwise compare the integer `Rank` each
   type carries, giving cross-family pairs a defined position
   (roughly `Boolean < Number < String < … < List < Map < …`).

That total order is what **`sort`** and the collection words use, and
it is surfaced directly by **`tcmp`** (`1 tcmp "a"` returns `-1`).

But ordering values of *unrelated* families is almost always a
mistake, so the everyday ordering words — `cmp`, `lt`, `lte`, `gt`,
`gte` — are **family-restricted**. They compare only same-type values
or values stage 1 can place with a real family comparer; a pair that
would only order by the stage-2 Rank fallback (`1 lt "a"`,
`List lt Map`) raises `[boru/incomparable]` and points you at `tcmp`:

```
1 lt 2.0                          # returns true        — Integer and Float share Number
1 lt "a"                          # returns error       — different families; use tcmp
1 tcmp "a"                        # returns -1          — the total order, on demand
```

**Equality is *not* restricted.** `eq`/`neq`/`deq` compare across types
freely — values of different types are simply not equal (`1 eq "1"`
returns `false`), which is safe and needs no escape hatch.

**Two equalities, one rule.** For **Scalars**, `eq` and `deq` are the
same thing: equality by *value* (`1 eq 1.0` and `1 deq 1.0` are both
true — cross-leaf magnitude). For the **Nodes and Ideals** (lists,
maps, XML, class instances, `Store`), `eq` is *reference* equality —
are these the same container? — while `deq` is *value* equality,
comparing contents deeply: `["a"] eq ["a"]` is false, `["a"] deq
["a"]` is true; two distinct stores holding the same entries are `deq`
but not `eq`. `Error` is a value-like Ideal — an immutable value with
no handle — so `eq` and `deq` both compare its fields, coinciding like
a scalar. The `Module` descriptor (`M.$module`) is an identity-equal
opaque handle: `eq` and `deq` are both its instance identity (every
namespace one import binds shares the instance; distinct imports are
distinct instances). Words that operate on values use `deq`: the
collection words (`ArrayUtil.unique`/`member`/`indices`/`group`)
dedup, test, and group by the `deq` class. A few values have no
settled equality — they are not `deq` even to themselves: host
payloads, and any container holding one. (`nan` is separately
non-reflexive, by the IEEE rule.) Functions and words compare by
content, and a declared TYPE is `deq` (and `eq`) to itself but never
to a same-bodied sibling — identity is the declaration, so two
`class {a:Integer}` definitions under different names are not `deq`.

A KERNEL type literal sorts strictly below every concrete inhabitant
of its family; a user-named type denotes its minted node and orders
in the type band (same-family, so the restricted words allow it — but
write the literal on the *right*: a type literal on the **left** of
`lt`/`gt`/`lte`/`gte` constructs a predicate refinement instead, so
`Integer lt 0` returns the subset type `(Integer lt 0)`, not a
boolean — see [Function signatures and refinement
types](#function-signatures-and-refinement-types)):

```
0 gt Integer                      # returns true
Integer tcmp 0                    # returns -1
```

Lists are length-first then element-wise; maps are key-set then
value-wise. The end effect: everything is sortable through `tcmp` /
`sort`, while a stray cross-type `lt`/`cmp` is caught rather than
silently answered.


## Immutability and mutability

boru draws a deliberate line between immutable values and mutable
objects:

* **Scalars** (numbers, strings, booleans, atoms, times) are
  immutable. Every operation returns a new value.
* **Nodes** (lists, maps) are immutable values. List/map operations
  return new copies. (The `flex` family — FlexMap/FlexList — is the
  deliberately mutable Node exception; see the Reference.)
* **Ideals** (Store, Record-instance, Table-instance,
  Class-instance, Tensor) are mutable. Their methods modify in
  place.

This distinction matters for concurrency. When `await` runs
parallel branches in separate sub-engines, immutable values are
safe to share, mutable Ideals are not — changes inside a branch
don't propagate to the parent.

Mutable instances are deliberately rare in idiomatic boru: prefer
returning a new value to mutating, until a benchmark says otherwise.


## Quotation and evaluation

Lists are *dual-purpose*: data structures and code bodies. By
default a list literal is **evaluated** — its elements run and the
list holds the results:

```
[add 1 2]                         # returns [3] — evaluated by default
[1 2 3]                           # returns [1 2 3] — plain data, nothing to run
```

`quote` is the opt-out. It keeps the list (or the next token)
unevaluated, as data — so the elements stay as written (words become
atoms) and can be run later with `do`:

```
[add 1 2] size                    # returns 1 — already evaluated: one element, 3
quote [add 1 2] do                # returns 3 — held as data, then run
```

Some positions are *implicitly* quoted — they take a list as code to
run later, not a value to evaluate now: all branches of `if`, the
body of `for`, the function passed to `each` / `fold`, and the body
list of a `fn` definition. That is why `fn [[n:Integer] [Integer]
[add n 1]]` stores `[add n 1]` as the function body instead of trying
to run it at definition time. (A plain `def name [body]` is *not* one of
these positions — it evaluates the list and binds the result, so a
Forth-style splice uses the explicit `word` form: `def double word
[dup add]`.)

To evaluate a held list at the point of use, use `do`:

* `do [add 1 2]` — runs it as a sub-program, leaving results on the
  stack.

The duality — lists as both data and code — is the homoiconic core
that lets boru do metaprogramming with no separate AST type.


## Macros

Quotation lets you *hold* code as data; **macros** let you *transform*
it. A macro is a function the engine runs at **expansion time**, on its
operands **as unevaluated code**, whose returned tokens are spliced into
the call site in place of the call. Where a normal word receives
*values*, a macro receives *forms* — so it can build new control
structures and syntax in boru itself, not in Go.

```
def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])
twice 5                           # returns 10
```

`twice 5` does not pass `5` to a function; it rewrites the call to the
code `5 add 5`, which then runs. The template is an ordinary `quote
[ … ]` region — default-data, the polarity flip from boru's default-eval
— and `unquote` / `splice` are the holes where operands flow back in:
`unquote x` inserts one node, `splice xs` spreads a list's elements.

This is the classic LISP dividend, and boru builds it from parts it
already had — `quote`, the splice marker behind `word`, raw-form
argument capture, and closure capture — rather than a new engine. Two
LISP problems come along for free:

* **Hygiene.** A name a macro introduces (a literal `def tmp` in a
  template) is auto-renamed to a fresh `gensym`, so it can never clash
  with a same-named variable at the call site. The `unquote`/`splice`
  boundary doubles as the provenance marker — names that came *from* the
  caller (through an escape) are left alone, which is exactly the
  distinction hygienic macro systems must otherwise synthesize.
* **Staging.** Macros are define-before-use and expand left-to-right, so
  generated code is just more source that re-enters the same evaluator.

Reference details and the full operator set are in
**[Reference: Macros](REFERENCE.md#macros)**; the design and its LISP
lineage are in `design/MACROS.8.md`.


## The Options pattern

Most non-trivial words accept an optional trailing `Map` to carry
named flags. This keeps the main signature small while leaving
room for growth:

<!-- boru-test: skip -->
```
# split/replace live in boru:string-util; shown unqualified here for
# brevity (in real code: import "boru:string-util" StringUtil.split …).
"hello world" split " "                              # basic
"hello world" split " " {trim: true}                 # with options
"aaa" "a" "b" {scope:'all, count:2} replace          # returns 'bba'
await {mode: 'first} [[sleep 100 1] [sleep 10 2]]    # returns 2
```

By convention, every word that takes options declares the option
form as a separate, last-resort signature — so the options map is
always optional and the option-less call still works. Options keys
are atoms (`'all`, `'insensitive`, `'last`), not strings, so
typos surface at type-check time.

An options schema is an `Options` type (`make Options {…}`), and a
field may declare a **concrete default**. When the caller omits such a
field, dispatch **materializes** the default into the map the handler
or `fn` param receives — so the receiver always sees a complete map
and never re-derives defaults:

<!-- boru-test: skip -->
```
def opts (make Options {x:1 y:2})
def f fn [[m:opts] [Map] [m]]
f {x:10}                          # → {x:10 y:2}   (y's default filled in)
f {}                              # → {x:1 y:2}
```

Only genuine concrete defaults are materialized. A field declared
`T tor None` (optional, no real default) stays absent when omitted —
its "default" is `None`, i.e. *unset* — and a bare type-literal field
(`{x:Integer}`) is required, so omitting it is a signature error, not a
default.


## Parallel execution model

The `await` word bridges boru's sequential stack model with Go's
goroutines. Each element of the parallel list runs in its own
goroutine with an independent sub-engine:

<!-- boru-test: skip -->
```
await [[sleep 100 1] [sleep 100 2]]   # returns [1 2]
```

The four modes mirror JavaScript's Promise combinators, providing
familiar semantics:

* **`'all`** (default) — every branch must succeed; the first
  error fails the whole `await`.
* **`'full`** — every branch completes; results carry
  `{status:'ok|'error, value:...}` (like `Promise.allSettled`).
* **`'first`** — the first branch to complete wins (race).
* **`'any`** — the first non-error result wins.

Each branch runs under `do` semantics: the list is evaluated as a
sub-program, and the final stack value becomes the result. A branch's
`def` and `context set` writes are local to that branch's sub-engine.

Sharing a **stateful container** — `FlexMap`, `FlexList`, `Store`,
`Table`, a class instance — across branches is **refused**, not
isolated. Branches run on separate goroutines, so an in-place write to
one is a data race; `await` rejects the program at the boundary rather
than letting it corrupt state, the same answer `send` gives at a
process boundary:

<!-- boru-test: skip -->
```boru
def m (make FlexMap {})
await [[m set a 1] [m set b 2]]
# error: not_sendable — branch reaches `m`, a mutable FlexMap
```

Immutable values are unaffected: a plain `Map` or `List` returns a copy
from `set`, so branches never share one. Build the container inside
each branch and combine the results:

<!-- boru-test: skip -->
```boru
await [
  [def a (make FlexMap {}) a set k 1]
    [def b (make FlexMap {}) b set k 2]
]
# [{k:1} {k:2}]
```

The refusal covers what a branch body references, including one level
into a function it names — and when a reference cannot be resolved at
all, it falls back to refusing if any mutable container is in scope. It
is a boundary rule, not a proof: a container reached only through a
longer chain of calls, with nothing mutable visible at the boundary, is
not detected. Building the container inside each branch is always safe.


## Errors as values

boru lets you treat errors as values rather than as exceptions — but
this happens at a `do [...]` boundary, not automatically. When a word
fails *in the open*, it unwinds: `1 div 0` on its own aborts the
program (and `1 div 0 dup` never reaches `dup`). Wrap the failing code
in a `do [...]` block and the failure is instead *reified* — the block
produces an `Error` value that sits on the stack like any other:

```
do [1 div 0]                      # returns error(division by zero)
```

So `do [...]` is the construct that converts an unwinding failure into
a first-class value; outside it, errors propagate.

`error` is a pattern-match: if the top of the stack is an `Error`,
run the handler; otherwise no-op. Handlers see the error value on
the stack and choose what to do with it:

```
do [1 div 0] error [drop 42]      # returns 42

do [IO.read (make Pathon "missing.json")] error [
  dup .code eq 'io_error if [
    drop {}
  ] [
    "fatal: " printstr print
  ]
]
```

This makes error handling *compositional* — errors flow through
pipelines exactly like ordinary values, and you handle them at the
boundary where it makes sense.


## Store and context

The execution context is a `Store` — a mutable key-value map with
prototype-chain lookup. `set` writes to the current store, `get`
walks the prototype chain (parent first), and a nested body that runs
in its own sub-engine inherits from the parent's store.

```
context set x 42
context dot x                     # returns 42
```

This is functionally JavaScript-style prototype inheritance: child
contexts can read parent bindings, but writes are local. It gives
you lexical scoping with copy-on-write semantics, without any
explicit closure construct.

**Which bodies are write boundaries.** The rule is "runs in its own
sub-engine", and that is narrower than it reads. Boundaries: `do`, `each`,
`fold`, `filter`, `outer`, `scan`, `for-each`, `await`, a `case` clause
body, and an auto-evaluated list. **Not** boundaries: a `for` body, an
`if` branch, a named `fn` body, a called lambda, and a paren group — a
paren resolves its value and hands it to the signature matcher, so it
never opens a scope.

`for` and `if` are the two worth stating explicitly, because a reader
reasonably expects them to scope and they do not:

```
for 1 [ context set y 5 ]
context has y/q                   # true — the write escaped the body
```

Two caveats. `set` on a context layer is copy-on-write, so "writes are
local" means the write does not reach the parent LAYER — a nested body
still sees, and can shadow, everything above it. And the bytecode compiler
cannot yet bracket the boundaries it INLINES into the caller's code — a
`case` clause body, an auto-evaluated list, an interp-string or xml hole —
so a `context` reference inside one refuses compilation. That refusal is a
DEFECT, not a design choice: all valid code is meant to compile, and this
case is unimplemented and owed a fix. Today the runtime silently routes the
whole program to the interpreter instead — scaffolding that contains the
open defect and hides it, which makes the failure worse, not tolerable. The
boundary above does hold on both engines, so the program's meaning is
unchanged, but that does not make the refusal acceptable; it is tracked to
closure. See NUR054 and
`design/verse-report-defects-investigation.0.md` §B.


## Module system

A module is a fresh evaluation context. You build one by
evaluating a list in a new store, then expose the resulting
bindings under a namespace:

```
import utils [
  def helper [dup add]
  def greet fn [[String] [String] [`hello ${args.0}`]]
]
```

After import, `utils.helper` and `utils.greet` are available — the
dot is just field access on the module's exported map.

File imports load source from disk; renaming on import (`import
[helper as h] "..."`) prevents collisions; built-in modules
(`boru:math-util`, `boru:time-util`, `boru:matrix-util`) are
host-provided and follow the same shape.

There is no global namespace flattening: every imported binding
lives under the module's prefix until you alias it explicitly.


## Ideals and type-kinds

boru has a system for *type-kinds* called **Ideals**. An Ideal is
the type-constructor turned into data — `Class`, `Record`, `Table`,
`Store` are all instances. Each Ideal carries:

* a name and a lattice anchor (so the kind has a place in the
  hierarchy),
* func fields for `Construct`, `Instantiate`, `Match`, `Format`,
  `Field`, `Equal`, `Unify`,
* metadata flags (`Inherits`, `OrderStrict`) that let shared
  helpers stay generic.

The practical consequence: a host program can register a *new*
type-kind (e.g. `Graph`, `Tensor`, `Stream`) at runtime, and
the kernel routes `refine`, `make`, `is`, and unification through
it the same way it does for the built-ins. The `boru:matrix-util`
module does exactly this for `Matrix` and `Vector`.

You usually don't write Ideals — you use them via `class` / `refine`
(Class / Record / Table) and `make`. The framework matters only if you're
extending the language with a new kind of typed container.


## Generics as memoised type construction

Generic types follow the same "types are values" doctrine as the
rest of the system. A schema (`def Box<T> class {value:T}`) is a
value holding a type body with placeholder nodes embedded;
instantiation (`Box of [Integer]`, or the sugar `Box<Integer>`) is
ordinary execution that substitutes the placeholders and **interns
the result** — one lattice node per (schema, arguments), minted as
a child of the schema. That one decision buys most of the
semantics for free: `typeof` names the instantiation because it IS
a type; `is Box` works by plain ancestry; two mentions of
`Box<Integer>` are `teq` because they're the same node; and the
checker gets monomorphization without new machinery, because its
per-call memo keys on argument types that now include the
instantiation's identity. Bounds reuse the membership question the
whole system already answers — `extends C` admits exactly what
`is C` admits, so predicates, disjunctions, and surfaces are all
valid bounds with zero special cases. The trade-off chosen for v1
is **invariance**: `Box<Integer>` is not a `Box<Number>`, the same
conservative default the nominal class system uses.


## Capabilities

Side-effecting words (`read`, `write`, `fetch`, `sqlite-*`,
`timeout`, `interval`, `sleep`, vault lookups) are gated by
*capabilities* — runtime feature flags on the Registry. The CLI
turns them all on by default; embeddings (Wasm playground, an
LLM tool host) can disable any of them.

When a disabled word runs, it fails with a permission-denied
error. This is the same shape as any other error: the calling code can
catch it with `do ... error [...]` and react appropriately. A refused
FILE operation carries a code a handler can dispatch on —
`permission_denied` when a rule said no, `capability_not_installed`
when the capability was never installed — while refusals from the
other gated scopes do not yet carry a stable code, so match on the
message for those.

Capabilities are deliberately coarse — one flag per system —
because the per-call enforcement happens *inside* the words. Finer
sandboxing (e.g., a path whitelist for `read`) is layered on top
via the `capabilities.FileOps` interface, which an embedder
provides directly.

A second, finer layer sits above capabilities: **policy profiles**.
Where a capability is a single on/off flag per system, a policy is a
set of allow/deny rules over `scope.op` pairs (`disk.read`,
`network`, `vault.get`, …) with optional quantitative caps. The CLI
exposes them with `--perms <profile|file|inline>` plus ad-hoc
`--allow`/`--deny` rules, and `boru policy {list,show,test,explain}`
inspects them — `explain` prints the blame chain for a decision, so
"why was this denied?" is always answerable. Capabilities decide
*whether a kind of effect is possible at all*; policy decides
*which specific calls are permitted*. See the [Reference → CLI](REFERENCE.md#cli-reference)
and [CLI.md → Permissions](CLI.md#permissions) for the surface.


## The CLI: two surfaces

The `boru` binary has two kinds of subcommand, and the split is
deliberate.

**One-shot commands** run, transform, or inspect and then exit:
`run`/`do` evaluate code, `check` type-checks, `fmt` formats, the
project verbs (`prep`, `pack`, `install`, `publish`, …) move modules
in and out of registries, and `vault` manages secrets. They are
ordinary Unix tools — read input, write output, set an exit code —
so they compose in shell pipelines and CI without ceremony.

**Long-running services** stay up and serve: `repl`, `registry` (an
HTTP module host), `lsp` (a Language Server for editors), `exec` (an
HTTP code-execution endpoint), and the vault `proxy` (a credential
broker). The umbrella `serve` command composes several of them into
one process under a single graceful-shutdown lifecycle —
`boru serve registry … + exec …` — and `ctl`/`tui` drive a running
supervisor through its `api` service. The reason services are a
separate surface is that they each own a scarce resource (a port, or
stdio) and a lifetime; making that explicit keeps the one-shot tools
free of server concerns and lets the supervisor reject conflicts
(two services both wanting stdin) up front.

The same permission model spans both surfaces: every execution path
that runs user code (`run`, `do`, `exec`, the REPL) accepts the
policy flags above, so an HTTP `exec` endpoint is sandboxed the same
way a local `boru do` is.


## The vault: why a local credential store

Agents and build tools need credentials, and the usual answers are
bad: a token pasted into `~/.npmrc` or `.env` is plaintext on disk;
a token in a cloud secret manager needs the network and an identity
just to start. The vault is a **local-first, encrypted** store that
sits between those — secrets live on your machine, encrypted at
rest, reachable without a network round-trip.

Three design choices shape it:

* **Metadata and values are split.** The store file
  (`vault.jsonic`) holds only *locations and metadata* — alias
  names, providers, namespaces, expiry reminders — never secret
  values. The values live in the chosen **backend** (the OS keychain
  by default; an encrypted file, Secret Service, wincred, or
  1Password otherwise). So the file you might accidentally commit or
  back up carries no secrets, and the same metadata can front
  different backends on different machines.

* **Delegation is by capability, not by sharing the secret.** You
  rarely want to hand an agent the raw token. `vault grant` mints a
  **scoped capability token** — a bearer string shown once, stored
  only as a hash — bound to an alias, a host/method allowlist, a TTL,
  and optional call/cost caps. The agent presents it to the
  `proxy` (or the `mcp` server), which injects the real secret
  server-side and enforces the scope. Revocation (`vault revoke`) is
  immediate because the broker checks the live store on every call.
  This mirrors the capability-token pattern: authority is
  attenuable, expiring, and auditable, and the secret itself never
  leaves the broker.

* **Multi-party access is by keyslot.** A vault can be opened by
  several **scoped passwords** (`vault password`), each a keyslot
  granting a scope (read/write/move/admin) over a set of namespaces.
  This is how a CI password can read `ci:` secrets without being able
  to touch `prod:`, and how access is rotated (`--rekey`) or revoked
  without re-encrypting the whole vault.

Around that core: every mutation is **integrity-sealed** and written
to an append-only **content history** (`vault history` /
`restore`), and every read/grant/exec is recorded in a structured
**audit log** that never contains secret values. The interactive
**TUI** (`boru vault -i`) exists because the surface is large (~30
modes, hidden-input prompts, typed confirmations) and hard to
discover by flag alone; it always shows the available keys on
screen.

Finally, the vault's *consumption* surface — `exec`, `proxy`, `mcp` —
is kept CLI-only and never run from the TUI. `exec` injects secrets
into a child process's environment (with `--for` recipes that place
a token in the exact variable a publishing tool reads, so the secret
never touches `~/.npmrc` or the command line); `proxy` and `mcp` are
long-lived brokers. Each hands the terminal, a port, or stdio to
something else, which is the services surface, not an interactive
menu. The full operational reference is [CLI.md → Secrets](CLI.md#secrets);
a worked walkthrough is [Tutorial → the vault](TUTORIAL.md#21-manage-secrets-with-the-vault).


## Design influences

boru draws from several traditions:

* **Forth, Factor** — stack-based execution, word definitions,
  quotations, the basic "code is a sequence of words" feel.
* **APL, J, K** — array operations (`iota`, `reshape`, `grade`,
  `outer`, `inner`), the array-everywhere intuition.
* **JavaScript** — Promise combinators (`all`, `allSettled`, `race`,
  `any`), prototype-chain stores, template literals.
* **SQL** — relational thinking behind tables and records.
* **Prolog** — unification-based type matching (`unify`).
* **Haskell** — type-directed dispatch, immutable-by-default values,
  total comparison.
* **Lisp** — homoiconicity, lists as data and code, REPL-driven
  development.

The result is intended to feel like a stack language that doesn't
fight readability, a query language that doesn't fight composition,
and an array language that doesn't require you to leave the rest
of programming behind.
