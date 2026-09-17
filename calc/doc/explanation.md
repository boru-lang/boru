# Explanation — why calc is shaped the way it is

This document is about understanding, not getting things done. If
you want a recipe, see [how-to.md](how-to.md); if you want a
walkthrough, see [tutorial.md](tutorial.md). Here we step back and
look at *why* calc's code reads the way it does — what the design
decisions are and what they imply for any host that wants to use
the eng kernel.

## The eng/lang split

The boru repository contains three Go modules that matter for
this story:

```
eng/go/     algorithms — registry, dispatch, parser, type lattice
lang/       words — every word boru programs use lives here
cmd/go/     the boru CLI and friends, built on lang
```

The architectural rule is **no words live in `eng/`**. The kernel
exposes:

- value constructors (`NewInteger`, `NewDecimal`, …),
- type lookups (`TInteger`, `TNumber`, …),
- the dispatch step loop,
- the parser,
- *exported handler primitives* that any host can wire into a
  word of its choosing (`LtHandler`, `MakeHandler`, `EqHandler`, …).

A host module — lang for the production language, calc for the
calculator — picks the names, signatures, and dispatch behaviour
of its own vocabulary by registering NativeFunc values.

Calc is the proof that this split is real. If lang ever
accidentally introduced a dependency on its own word
registrations into eng (think of a `RegisterCoreWords` that did
the lang-internal work), calc would either fail to compile or
fail at runtime because lang's words would have intruded into
"its" registry.

Two practical consequences for any host you write:

- **`go.mod` is small.** Calc depends on `github.com/boru-lang/boru/eng/go`
  and transitively on the tabnas parser family (`tabnas/jsonic`,
  `tabnas/parser`). That's it. No sqlite, no csv, no struct
  manipulation toolkit.
- **You own the names.** `add` in calc has nothing to do with
  `add` in lang. They share no signature, no handler, no surface
  guarantees. Calc decided what `add` means for itself.

## Why eng exports `LtHandler` but not `lt`

Several handlers in eng are exported — `LtHandler`, `GtHandler`,
`TandHandler`, `MakeHandler`, etc. — but the corresponding word
names (`lt`, `gt`, `tand`, `make`) are not registered anywhere
in eng. Why bother exporting handlers if the words don't exist?

Because the **handler is the algorithm** and the **registration
is the language design**. Lang and calc both want a `lt` word
that compares two values, but they might want different
signatures or different dispatch boundaries. Lang's `lt` in
`lang/go/native/native_compare.go` registers two overloads — one
that builds a DepScalar refinement (`Integer lt 10`), another
that does ordinary comparison. A host that didn't want the
DepScalar overload would register only the second.

In both cases the *comparison algorithm itself* — convert both
values to comparable form, compute the ordering — should not be
duplicated. eng exports `LtHandler` so every host points dispatch
at the same algorithm rather than rewriting it.

Calc doesn't currently register `lt`, but the pattern is the
same as the binary arithmetic words: pick the signature, point
the handler at the algorithm. See [how-to.md](how-to.md) for the
binary-op recipe.

## Forward vs stack dispatch — what `BarrierPos` actually does

Every NativeSig declares an arg-list. The dispatcher fills those
args by walking the **forward limit** for that sig:

- positions `[0 .. BarrierPos-1]` are *forward-collected* — the
  dispatcher scans the tokens after the word and gathers values
  in source order;
- positions `[BarrierPos .. N-1]` come from the **stack**, top
  down — sig[BarrierPos] = top, sig[BarrierPos+1] = next-deeper, etc.

`BarrierPos` has three canonical values:

- `BarrierAllForward` (-1) — the sentinel that says "no `|`
  boundary specified; default to all-forward." Resolved at
  registration to `len(Args)`. Use this for normal words like
  `add 2 3`.
- `0` — explicit all-stack dispatch. Forth-style stack ops like
  `dup`, `drop`, `swap` set this so the dispatcher doesn't even
  look at what's after the word.
- `N` (`1 ≤ N < len(Args)`) — explicit barrier at position N.
  Used by words like `get key map` where the leading slot
  forward-collects but the trailing slot must come from the
  stack.

There is no per-word "ForwardArgs" flag any more — every sig
carries its dispatch policy in its own `BarrierPos` field.

The unified rule means **any layout works**, and it is the same
rule at every arity — two-argument words are not a special case,
and there is no "swap form". For a 2-arg forward-eligible word:

```
sub 10 3     forward [10,3]                  → sig=[10,3]
3 sub 10     forward [10], stack top=3       → sig=[10,3]
3 10 sub     forward [],   stack top=10…3    → sig=[10,3]
10 sub 3     forward [3],  stack top=10      → sig=[3,10]  ← infix
10 3 sub     forward [],   stack top=3…10    → sig=[3,10]
```

The first three put `10` in sig[0]; the last two put `3` there.
Nothing distinguishes them but where the split between the
forward phase and the stack phase falls. The infix spelling
`10 sub 3` is the one that reads as `10 minus 3 = 7`.

## Why `args[1] op args[0]`, not `args[0] op args[1]`

You will see this pattern in every binary handler in calc:

```go
res := op(b, a)              // b op a, not a op b
// where b is args[1] and a is args[0]
```

`words.go` even names the closure parameters as `a` and `b`
explicitly: `func(a, b float64) (float64, error) { return b - a, nil }`.

This is the infix convention from the previous section. The
dispatcher binds `args` in signature order; with a 2-arg
forward-eligible word, args[0] is the value seen first after the
word and args[1] is whatever fills the remaining slot. Reading
`10 sub 3` as "10 minus 3":

- The reader sees `10` first, then `sub`, then `3`.
- `sub` forward-collects `3` (args[0]). For the second arg,
  forward is exhausted, so it falls back to the stack — there
  it finds `10` (args[1]).
- For `10 sub 3 = 7` to read naturally the handler must compute
  `args[1] - args[0]`.

The convention costs the all-forward spelling (`sub 10 3` becomes
`3-10 = -7`, and `sub 1 3` is `2`), but infix is the most common
surface form a user types, so calc — like every other boru host —
picks it as canonical. Test cases in `calc_test.go` document this
with both the infix `10 sub 3 = 7` and the RPN `10 3 sub = 7`
rows.

## Why the stack persists across `Eval` and `Run` does not

`eng.Engine.Run` is one-shot: it takes a token slice and runs
until exhaustion. There's no "engine memory" between two
separate `Run` calls. That's good — engines are cheap to spawn
and stateless engines are easier to reason about.

A REPL needs the stack to carry from line to line. Calc solves
this by **wrapping the engine** in a `Calc` struct that owns the
stack as plain Go state, and reseeding the engine with that state
at the start of every `Eval`:

```go
seed = append(seed, c.stack...)   // previous stack first
seed = append(seed, values...)    // then this line's tokens
result, err := eng.NewTop(c.Registry).Run(seed)
c.stack = result                  // persist for next Eval
```

Literal values are no-ops at the start of a program — they just
land on the stack — so prepending the previous stack and parsing
fresh input on top gives the user a continuous-stack illusion
without any engine-level support for it.

This pattern transfers directly: any time you want a "session"
of REPL-like calls on top of stateless engine runs, hold the
stack in the host and reseed.

## Why `Calc` holds the registry, not the engine

`Calc.Registry` is `*eng.Registry`; `Calc` does *not* hold an
`*eng.Engine`. Each `Eval` builds a fresh engine via
`eng.NewTop(c.Registry).Run(seed)`.

The reason: the engine carries per-run state — a step counter,
the carrier stack for type-check mode, a pending FlowCtrl
slot. None of that should persist between user inputs in a
REPL. The registry, on the other hand, holds the def stack,
type stack, and word table — exactly the state that *should*
persist (so `def x 1` on one line is visible on the next).

This split tracks the eng API itself: `Registry` is a value
holder, `Engine` is a runner. Calc embraces the same idea.

## Why calc owns its output writer

calc.New(out) takes an `io.Writer`. The `print` and `show`
handlers close over that writer:

```go
fmt.Fprintln(out, args[0].String())
```

Two reasons:

1. **Tests can capture output**. `bytes.Buffer` plugs in where
   `os.Stdout` would normally go. The whole test suite uses
   this — see `calc_test.go::TestPrintWritesToConfiguredWriter`.
2. **Hosts can compose**. A web-based calc would pass an
   `http.ResponseWriter`; a daemon might pass a `*log.Logger`'s
   writer; the CLI passes `os.Stdout`. None of those changes
   touch the word registration code.

This trick generalises: anything in eng that "would print" should
actually take an `io.Writer` from the registry or close over one
at registration time. The registry exposes `r.Output` for words
to find a default; calc bypasses that by closing over `out`
directly, which is slightly simpler when you control all the
words.

## Why the REPL uses `:` for meta-commands

boru word names match `[a-z][a-z0-9-]*` — a leading colon can
never appear in a valid program. That gives calc a reserved
prefix it can grab without ambiguity. `:stack` is unambiguously
a meta-command, not a parse-failure with a confusing error
message.

The shape that emerges — "REPL is a Go program that occasionally
delegates a line to the engine" — keeps the host firmly in
control. Meta-commands run as Go code with full access to the
registry: `:words` walks `r.Defs.Names()`, `:clear` mutates the
struct's `stack` field, `:help` is a literal string. The engine
never sees those lines.

## What "calc is the proof" actually proves

Three concrete claims:

1. **eng compiles and runs without lang.** If lang's
   registrations had hidden imports into eng, calc would fail
   to build. Calc's `go.mod` is the contractual statement of
   "eng's public API is enough".
2. **eng's dispatch is generic over words.** Calc registers
   words eng has never heard of (`pi`, `depth`, `show`). They
   dispatch correctly through the same code path that lang's
   word use. There is no "lang lane" inside eng.
3. **The kernel/word boundary is at NativeFunc registration.**
   Above that boundary is the host; below it is the algorithm.
   You can read the boundary right in calc/words.go — the
   `r.RegisterNativeFunc` calls are exactly the seam.

If any of those claims broke, calc's tests would fail. They
currently pass at 95% statement coverage, so the architecture is
factually intact — not just designed-to-be intact.

## Where to read next

- The kernel's design choices around payload sealing, type
  lattice management, FixedID allocation, and registration
  policy are documented in [`eng/go/CLAUDE.md`](../../eng/go/CLAUDE.md).
- The production word set's design — `def` / `fn` / typed
  parameter binding, quotation, the `args` frame, check-mode
  carriers — is in [`lang/CLAUDE.md`](../../lang/CLAUDE.md).
- The dispatch algorithm itself, including the unified
  `args[1] op args[0]` reasoning, has its mathematical
  justification in
  [`design/legacy/SIGNATURE-MATCHING-PSEUDOCODE.10.ignore`](../../design/legacy/SIGNATURE-MATCHING-PSEUDOCODE.10.ignore).
