# calc as a learning example for `eng`

Calc is a 350-line Go module that hosts the boru engine kernel
(`github.com/boru-lang/boru/core/go`) and defines its own word vocabulary —
no dependency on `lang/`. It exists so the eng↔lang boundary is
demonstrably an architectural fact: if calc compiles, runs, and
its tests pass, the kernel is a usable algorithm library.

This documentation reads calc as **source material for learning how
to use eng**, not as documentation of the calculator itself. The
calculator is a vehicle; the destination is "I can build a
concatenative interpreter with my own words on top of the boru
engine."

## Documentation layout

| Doc | Question it answers | Read first if you… |
| --- | --- | --- |
| [tutorial.md](tutorial.md) | *How do I build my own engine host?* | …have never used eng before. Walks you through writing calc from scratch in seven steps. |
| [how-to.md](how-to.md) | *How do I do X with eng?* | …already know the basics and want a recipe (binary op, FullStack handler, REPL meta-command, …). |
| [reference.md](reference.md) | *What does this symbol do?* | …are reading calc's source and want a precise definition of every eng symbol it uses. |
| [explanation.md](explanation.md) | *Why is it built this way?* | …want to understand the design choices — argument ordering, the eng/lang split, the dispatch model. |

## Files calc consists of

```
calc/
├── go.mod              # depends only on eng
├── words.go            # word registrations: arithmetic, stack, constants, IO
├── calc.go             # Calc struct: registry + persistent stack + Eval
├── repl.go             # line-oriented REPL with meta-commands
├── cmd/calc/
│   └── main.go         # CLI entry: -e EXPR, positional args, REPL
└── *_test.go           # 95% statement coverage
```

The whole package is small enough to read end-to-end in one sitting.
The four docs in this directory line up against those files so you
can move between explanation and source without losing context.
