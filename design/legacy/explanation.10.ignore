# boru Explanation

This document explains the ideas and design decisions behind boru. It
complements the [Tutorial](tutorial.md) (learning-oriented),
[How-To Guides](how-to.md) (task-oriented), and
[Reference](reference.md) (information-oriented).


## What Is a Concatenative Language?

In most languages, you compose functions by nesting calls:
`f(g(x))`. In a concatenative language, you compose by placing words
next to each other: `x g f`. Each word transforms the stack, and
the output of one word becomes the input of the next.

This has a deep consequence: **programs are compositions**. The
program `g f` is itself a valid program that can be named and
reused. There is no syntactic overhead for combining operations —
juxtaposition *is* composition.

```
def double [dup add]
def quadruple [double double]
5 quadruple         => 20
```

`quadruple` is not calling `double` twice in the traditional sense.
It is the composition of `double` with itself. The stack flows
through both transformations.


## The Stack Model

boru uses a single data stack. Every literal pushes a value. Every
word pops its arguments and pushes its results. The stack is the
implicit data flow — you rarely need to name intermediate values.

```
3 4 add 2 mul       => 14
```

Step by step: push 3, push 4, `add` pops both and pushes 7, push 2,
`mul` pops both and pushes 14.

The stack model eliminates the need for variable binding in simple
cases. When naming is helpful, `var` and `def` provide it — but the
default is point-free composition.


## Forward Collection: Beyond Reverse Polish

Traditional concatenative languages (Forth, Factor) use strict
reverse Polish notation: arguments always precede the word. boru
extends this with **forward collection**: a word can gather
arguments that appear *after* it.

```
1 2 add         => 3    # classic prefix (stack)
add 1 2         => 3    # fully forward
1 add 2         => 3    # infix: one from stack, one forward
```

All three are equivalent. The word `add` needs two arguments. If
fewer are on the stack when it executes, it waits and collects
subsequent values as forward arguments.

This makes boru readable in ways that pure stack languages are not.
`10 sub 3` reads as "10 subtract 3", not the Forth-style `10 3 -`.

### How Collection Works

When a word executes:

1. Check the stack for available arguments that match the signature.
2. If not enough, enter forward-collection mode: each subsequent
   token is evaluated and checked against the next expected type.
3. If a token does not match, collection stops.
4. Once all arguments are gathered (from either source), execute.

Forward arguments fill signature positions starting from `sig[0]`.
Stack arguments fill remaining positions, top-of-stack first. After
rearrangement, the word always sees arguments in signature order.

### The `end` Keyword

Sometimes you need to stop forward collection explicitly. The `end`
keyword forces a word to execute with whatever arguments it has,
taking any remaining from the stack:

```
def x 10
def y add x end 5
y                   => 15
```

Without `end`, `add` would try to collect `5` as an argument instead
of letting it be the value bound to `y`.


## Type-Directed Dispatch

Every value in boru carries a hierarchical type. The type
`Scalar/Number/Integer` is a child of `Scalar/Number`, which is a
child of `Scalar`. A child matches its parent, but not vice versa.

Words declare their signatures as type patterns. When a word has
multiple signatures, the engine tries each in order and uses the
first match. This gives natural overloading:

```
add 1 2             => 3       # Integer + Integer -> Integer
add 1.0 2           => 3.0     # Decimal + Number -> Decimal
add "a" "b"         => 'ab'    # Scalar + Scalar -> String (concat)
```

The type system is not just for validation — it drives dispatch.
The same word name can behave differently depending on the types of
its arguments, without explicit conditionals.


## Immutability and Mutability

boru draws a clear line between immutable values and mutable objects:

- **Scalars** (strings, numbers, booleans, atoms) are immutable.
- **Nodes** (lists, maps) are immutable values. Operations return
  new copies.
- **Objects** (stores, arrays, records, tables) are mutable
  instances. Operations modify in place.

This distinction matters for concurrent code. When `await` runs
parallel branches, each branch works with its own sub-engine. 
Immutable values are safe to share. Mutable objects require care —
changes inside a parallel branch do not automatically propagate to
the parent.


## Quotation and Evaluation

Lists serve double duty in boru: they are both data structures and
code bodies. By default, lists created in word context are
**evaluated** — their contents are executed as a sub-program when
consumed:

```
[1 add 2]           => [3]       # evaluated
quote [1 add 2]     => [1,add,2] # quoted: evaluation suppressed
```

Code-body positions in control words (`def`, `if`, `for`, `each`,
`fold`, etc.) implicitly suppress evaluation. This is why you can
write:

```
def double [dup add]
```

The list `[dup add]` is not evaluated when bound to `double`. It is
stored as a code body and executed each time `double` is called.

The `do` word explicitly evaluates a list:

```
do [1 add 2]        => 3
```

This duality — lists as both data and code — is characteristic of
homoiconic languages and gives boru its metaprogramming capabilities.


## The Options Pattern

Many words accept an optional options map as an additional argument.
This provides named configuration without polluting the main
signature:

```
"hello world" split " "                     # basic
"hello world" split " " {trim: true}        # with options
```

Options maps are regular maps that match the `Map` type in
signatures. Words that accept options always list the options
variant as a separate signature, so the options argument is always
optional.


## Parallel Execution Model

The `await` word bridges boru's sequential stack model with Go's
concurrent primitives. Each element in the parallel list runs in its
own goroutine with an independent sub-engine:

```
await [[sleep 100 1] [sleep 100 2]]     => [1,2]
```

The four modes (`all`, `full`, `first`, `any`) mirror JavaScript's
Promise combinators, providing familiar semantics for concurrent
workflows:

- **all**: All branches must succeed. One error fails the whole
  operation.
- **full**: All branches complete. Results include status
  (`'ok` or `'error`) regardless of success.
- **first**: The first branch to complete wins. Others are
  abandoned.
- **any**: The first non-error result wins. All must fail for the
  operation to fail.

Each branch runs under `do` semantics: the list is evaluated as a
sub-program, and the final stack value becomes the result.


## Error Handling as Values

boru treats errors as values, not exceptions. When an operation fails
(e.g., division by zero), it produces an error value that sits on
the stack like any other value:

```
do [1 div 0]        => Error(...)
```

The `error` word pattern-matches on error values:

```
do [1 div 0] error [drop 42]       => 42
```

If the value on the stack is not an error, the `error` word does
nothing. This makes error handling compositional — you can chain
operations and handle errors at the boundary.


## Store and Context

boru's execution context is a `Store` — a mutable key-value store
with prototype chain lookup. When you `set` a key, it is stored in
the current context. When you `get` a key, the lookup walks the
prototype chain, similar to JavaScript's prototype inheritance.

```
context set x 42
context get x           => 42
```

Sub-engines (created by `do`, `for`, `each`, etc.) get their own
context store with the parent's store as prototype. This means they
can read parent values but writes are local — a form of lexical
scoping via copy-on-write semantics.


## Module System

Modules provide namespace isolation. A module is defined by
evaluating a list in a fresh context, then exporting the resulting
definitions:

```
import utils [
  def helper [dup add]
  def greet fn [String String [`hello ${args.0}`]]
]
```

After import, `utils.helper` and `utils.greet` are available. The
dot notation is syntactic sugar for module member access.

File imports load external boru source:

```
import "lib/utils.boru"
```

Renaming on import prevents name collisions:

```
import [helper as h] "lib/utils.boru"
```


## Design Influences

boru draws from several traditions:

- **Forth / Factor**: Stack-based execution, word definitions, quotations
- **APL / J / K**: Array operations (`iota`, `reshape`, `grade`,
  `outer`, `inner`), rank polymorphism aspirations
- **JavaScript**: Promise-like parallel execution modes, familiar
  string operations
- **SQL**: Planned dataframe/table operations
- **Prolog**: Unification-based type matching
- **Haskell**: Type-directed dispatch, immutable-by-default values


## Further Reading

- [Tutorial](tutorial.md) — learn boru step by step
- [How-To Guides](how-to.md) — practical recipes
- [Reference](reference.md) — complete word and type listing
- [Design Documents](design/) — internal design notes and plans
