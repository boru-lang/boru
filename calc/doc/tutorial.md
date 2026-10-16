# Tutorial — Build calc from scratch

> **Audience.** You know Go and have never used the boru engine. By
> the end you will have rebuilt every line of the calc module
> yourself and understood what it does.
>
> **Outcome.** A working `calc -e "10 sub 3"` that prints `7`, plus
> a REPL that carries a stack across lines.

You don't need to read this whole tutorial in one go — each step
leaves you with a runnable program. Stop after step 3 if all you
want is to dispatch one word; finish step 7 to get a full REPL.

## Prerequisites

A clone of the [boru repo](https://github.com/boru-lang/boru).
`go 1.24+`. No network access during the build — `core/go/` is a
local module imported via a replace directive.

```
cd boru
ls core/go       # types, registry, dispatch live here (the parser is parser/go)
```

## Step 1 — A new module that depends only on eng

Create the module directory:

```bash
mkdir -p mycalc/cmd/mycalc
cd mycalc
```

`mycalc/go.mod`:

```
module example.com/mycalc

go 1.24

require github.com/boru-lang/boru/core/go v0.0.0

require (
	github.com/tabnas/jsonic/go v0.6.0 // indirect
	github.com/tabnas/parser/go v0.8.0 // indirect
)

replace github.com/boru-lang/boru/core/go => ../core/go
```

The `replace` line points at the local copy of `core/go`. In a
production setup you would depend on a published version of eng;
inside this repo every module uses a replace directive so changes
propagate without a release.

The whole point of calc is that **this is the only dependency**:
no lang, no native_*, no transitive sqlite or csv. Read this as
"the kernel really is enough to host a host."

## Step 2 — A registry with no words

`mycalc/calc.go`:

```go
package mycalc

import "github.com/boru-lang/boru/core/go"

func New() (*eng.Engine, error) {
    r, err := eng.NewRegistry()
    if err != nil {
        return nil, err
    }
    r.InitRootContext()
    return eng.NewTop(r), nil
}
```

`eng.NewRegistry()` returns a fresh `*Registry` — a `Defs` table
(definitions like `def foo …`), a `Types` table (type names),
plus internal stacks for the dispatch loop. **No words are
registered.** That is the eng→lang refactor's invariant: the
kernel ships zero words.

`r.InitRootContext()` creates the root scope for `set` / `get` /
context-like operations. It's idempotent and harmless to call
even if you never use context-aware words.

`eng.NewTop(r)` returns an `*Engine` configured as the *top-level*
engine — any unhandled `FlowCtrl` (break / continue / return) at
end-of-Run becomes an error rather than propagating to a parent.
Use `eng.New(r)` for sub-engines spawned inside word handlers.

Run a literal-only program to prove the wiring:

```go
v := []eng.Value{eng.NewInteger(7), eng.NewInteger(42)}
out, err := New().Run(v)        // returns [7, 42], nil
```

Literals just pass through. There are no words registered yet,
so any `eng.NewWord("…")` you slip into the input would raise
`[boru/undefined_word]`.

## Step 3 — Define your first word

Add an `add` word that consumes two integers and pushes their sum.

```go
func RegisterWords(r *eng.Registry) {
    r.RegisterNativeFunc(eng.NativeFunc{
        Name: "add",
        Signatures: []eng.NativeSig{{
            Args:       []*eng.Type{eng.TInteger, eng.TInteger},
            BarrierPos: eng.BarrierAllForward, // default all-forward dispatch
            Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
                a, _ := eng.AsInteger(args[0])
                b, _ := eng.AsInteger(args[1])
                return []eng.Value{eng.NewInteger(b + a)}, nil
            },
            Returns: []*eng.Type{eng.TInteger},
        }},
    })
}
```

Three things to notice:

1. **`BarrierPos: eng.BarrierAllForward`** (i.e. -1) means `add`
   is forward-collecting — when the dispatcher sees `add`, it
   scans the *following* tokens for the two args before falling
   back to the stack. The sentinel resolves to `len(Args)` at
   registration time.
2. **`Args: []*eng.Type{eng.TInteger, eng.TInteger}`** declares
   the signature. The dispatcher uses this both to pick the
   right overload at runtime and as carrier types in check mode.
3. **The handler computes `b + a`, not `a + b`.** Under eng's
   unified dispatch rule the handler receives args in signature
   order: `args[0]` is whatever bound to sig[0], regardless of
   whether it came from forward or stack. To make `10 sub 3 =
   7` read naturally, every binary handler is written as
   `args[1] op args[0]`. See `explanation.md` for the full
   reasoning.

Wire it up:

```go
func New() (*eng.Engine, error) {
    r, _ := eng.NewRegistry()
    RegisterWords(r)
    r.InitRootContext()
    return eng.NewTop(r), nil
}
```

Now `add 2 3` evaluates to `[5]`:

```go
e, _ := New()
input := []eng.Value{eng.NewWord("add"), eng.NewInteger(2), eng.NewInteger(3)}
out, _ := e.Run(input)
// out[0] is NewInteger(5)
```

Stop here if all you wanted was to learn how to register one
native word. Calc adds a parser, more words, a stack, and a REPL.

## Step 4 — Take source instead of token slices

Hand-constructing token slices gets old. The parser lives next to
the kernel at `github.com/boru-lang/boru/parser/go`:

```go
import "github.com/boru-lang/boru/parser/go"

values, err := parser.Parse("add 2 3")     // returns []eng.Value
e, _ := New()
out, _ := e.Run(values)
```

The parser uses jsonic under the hood and emits the same
`[]eng.Value` shape the engine consumes. Lists `[a b c]`, maps
`{k:v}`, parens `(…)`, dotted access `foo.bar` — all of it is
already understood by the kernel and parser without any words
registered.

Optional: tell the registry about the parser so any word that
needs to parse strings later on (think `import "x.boru"`) has a
reference:

```go
r.SetParseFunc(parser.Parse)
```

Calc does this in [calc.go](../calc.go).

## Step 5 — More words with multi-type dispatch

`add` only handles integers. Real calculators promote on demand:
`add 1 2 = 3` (Integer), `add 1.5 2.5 = 4.0` (Decimal). The eng
way is to write one handler that uses `eng.AsNumber` (which
accepts either) and registers a `[TNumber, TNumber]` signature.

See [calc/words.go](../words.go) for the production version. The
shape is:

```go
func numHandler(op func(a, b float64) (float64, error), preferInt bool) eng.Handler {
    return func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
        a, _ := eng.AsNumber(args[0])
        b, _ := eng.AsNumber(args[1])
        res, err := op(a, b)
        if err != nil {
            return nil, err
        }
        if preferInt && args[0].Parent.Matches(eng.TInteger) && args[1].Parent.Matches(eng.TInteger) && res == math.Trunc(res) {
            return []eng.Value{eng.NewInteger(int64(res))}, nil
        }
        return []eng.Value{eng.NewDecimal(res)}, nil
    }
}
```

`preferInt` is a calc convention — `add`/`sub`/`mul`/`mod`/`pow`
collapse back to Integer when both inputs are Integer and the
result is whole. `div` always returns Decimal so `1 div 2 = 0.5`
instead of `0`.

Pick a few more words to hand-register: `sub`, `mul`, `div`,
`neg`, `abs`. The pattern doesn't change, just the operator.

## Step 6 — Persist the stack across `Run` calls

A one-shot `calc -e EXPR` mode is fine but a REPL needs the
stack to carry between lines. Wrap registry + engine + output
writer in a struct:

```go
type Calc struct {
    Registry *eng.Registry
    Out      io.Writer
    stack    []eng.Value
}

func (c *Calc) Eval(src string) ([]eng.Value, error) {
    values, err := parser.Parse(src)
    if err != nil {
        return nil, fmt.Errorf("parse: %w", err)
    }
    seed := make([]eng.Value, 0, len(c.stack)+len(values))
    seed = append(seed, c.stack...)
    seed = append(seed, values...)

    result, err := eng.NewTop(c.Registry).Run(seed)
    if err != nil {
        return nil, err
    }
    c.stack = result
    return result, nil
}
```

The trick is **prepend the previous stack as literal seed
values** before each parse. Because literals at the start of a
program just sit on the stack, the next program sees them as if
they had been pushed by hand. `1 2 add` and `1 2` then `add` both
collapse to `[3]`.

A new `NewTop(c.Registry)` per call gives a fresh dispatch
counter (so MaxSteps doesn't accumulate) while sharing the
registry — defs you create with `def foo 1` on one line persist
to the next.

If `Run` returns an error the stack stays at its previous value —
[calc.go](../calc.go) does this by reassigning `c.stack = result`
only on success.

## Step 7 — A REPL with meta-commands

Read lines from stdin, run them through `Eval`, print the result.
Reserve a prefix (calc uses `:`) for non-boru commands:

```go
func REPL(c *Calc, in io.Reader, out io.Writer) {
    scanner := bufio.NewScanner(in)
    for {
        fmt.Fprint(out, "calc> ")
        if !scanner.Scan() {
            return
        }
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        if strings.HasPrefix(line, ":") {
            // handle :quit, :stack, :clear, …
            continue
        }
        stk, err := c.Eval(line)
        if err != nil {
            fmt.Fprintf(out, "  error: %s\n", err)
            continue
        }
        if len(stk) > 0 {
            fmt.Fprintln(out, FormatStack(stk))
        }
    }
}
```

[repl.go](../repl.go) wires this to readline-style history and
meta-commands. The pattern is identical to the [reference REPL
in cmd/go/internal/repl/repl.go](../../cmd/go/internal/repl/repl.go) —
that one drives `lang` instead of pure `eng`, but the shape of
"parse, run, print" is the same.

## You now have a host

Build and run:

```bash
cd mycalc
go run ./cmd/mycalc -e "10 sub 3"
# 7
go run ./cmd/mycalc
# calc> 1 2 add
# 3
# calc> :quit
```

You wrote a working concatenative interpreter on top of the boru
kernel without touching the kernel itself. Every word in your
language is yours — including the names, the signatures, the
return types, and the dispatch behaviour. The kernel handed you
parsing, type dispatch, and the step loop, and stayed out of the
way for everything else.

From here, dig into:

- **[how-to.md](how-to.md)** for recipes the tutorial skipped over
  (FullStack handlers, 0-arg constants, stack underflow
  reporting, output redirection).
- **[reference.md](reference.md)** for the exhaustive list of
  every eng symbol calc uses, with a one-line definition each.
- **[explanation.md](explanation.md)** for the reasoning behind
  the `args[1] op args[0]` convention, why eng owns no words,
  and how forward / stack dispatch actually works.
