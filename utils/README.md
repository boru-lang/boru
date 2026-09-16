# utils/ — a coreutils subset, written in boru

These are real programs, not examples. Each one is a boru source file that
`boru build` turns into a standalone executable, parses its own arguments with
[`boru:cli`](../lang/go/modules/cli.boru), reads stdin or files, writes stdout,
and chooses an exit code. They exist to answer a question the language could
not answer before: *can you actually write a command-line tool in boru?*

Every program here is therefore also a test of the runtime's CLI contract —
argv, environment, exit codes, stream detection, baked permissions — and each
was chosen because it exercises something specific:

| Program | What it proves |
|---|---|
| `cat` | streaming a line at a time; a program that never slurps |
| `tee` | the only **writer** here, and therefore the program the baked-permissions pair is built from |
| `wc` | counting; matches GNU `wc` byte for byte, column layout included |
| `seq` | numeric formatting — zero padding, separators, float steps |
| `head` | early exit **without reading the rest** — the sharp test of incremental input (`tail` proves nothing: it is correctly implementable by slurping) |
| `grep` | regex, the 0/1/2 exit-code convention, and `--color=auto` — the suite's witness for `IO.is-tty` and `NO_COLOR` |
| `cut` | string words over unicode: everything counted in runes, never bytes |
| `sort` | collections, and the opposite end of `head`'s axis — a program that *must* read everything before it can emit anything |
| `uniq` | adjacency: it collapses **runs**, not files, which is why `sort \| uniq` is the idiom and why both are here |
| `printenv` | the enumerating half of the environment capability — nothing else witnesses it |
| `true`, `false` | `IO.exit`'s tri-state with nothing else in the way, so a wrong exit code has exactly one possible cause |

Every program is driven by a suite under `tests/`: **995 cases**, all passing.

## Running them

```bash
make -C ../cmd/go build     # the boru binary
make check                  # type-check every program and test
make test                   # run the suites
make binaries               # build every program into bin/
printf 'b\na\n' | ./bin/sort
```

`boru build` performs no type check of its own, so `make check` is not
optional — it is the only gate between a typo and a shipped binary.

## Why a Makefile here and a Go test over there

This directory is not in the repository's Go module list, so nothing in it is
reached by `make test` at the root. A top-level boru directory outside the Go
fan-out rots — `kg/` demonstrated that — so `cmd/go` carries an end-to-end Go
test — `cmd/go/internal/build/utils_e2e_test.go` — that builds real programs
from here with `boru build` and runs the produced executables: it pipes into
them, passes argv, hands them an environment, and checks `$?`. That test is
what keeps this directory honest in CI; the Makefile is what makes it pleasant
to work in.

It also owns the claims that are properties of a **built binary** rather than
of a running program, and so cannot be checked from inside boru at all:

- argv and the process environment reach a built binary,
- the exit code a program chooses is the process's exit status,
- and the **baked-permissions pair** — the same source built twice, once with
  `-perms read-only` and once with the write allowed. One build failing proves
  nothing on its own; only the pair shows the baked profile is what decided.
  (`design/CLI-PROGRAMS.1.md` §1.)

## House rules for a program here

Learned by writing them, and each one prevents a silent failure:

- **Terminating a module-export statement with `end` is no longer required**
  (kept as style here): two consecutive unterminated statements used to invert
  and drop one of the calls with no diagnostic (NUR038, resolved — a completed
  value-called collection now seals, and a dot-read callee is a collection
  barrier like its bare-word twin). `end` remains good hygiene for readers.
- **Swallow every residual.** The driver prints whatever the program leaves on
  the stack, so `IO.write` and `flex set` results must be dropped
  (`(IO.write …) drop`), and a program that has finished should end with
  `IO.exit 0;`.
- **Accumulate into `flex`**, not into an immutable Map or List: the immutable
  form is quadratic, and these programs read files.
- **Prefer declaring callbacks at top level** rather than inside another fn.
  This is now style advice, not a defect workaround: a fn-local callback used
  as a higher-order body word once resolved under the interpreter and died
  with `undefined_word` under the compiler (NUR037); a capture-free local
  callback now compiles (its def is placed for the frame), and a local
  callback that CAPTURES an enclosing binding is refused, so the interpreter
  runs the whole program instead — correct, just slower. A module-scope
  callback keeps the program on the compiled path either way.
- **Take argv as a parameter** in anything you want to test — `boru test`
  cannot inject an argument vector. Take the *environment* and the *terminal*
  as parameters too, for the same reason: a fn that reads them can only ever
  be tested against the one it is running in.
- **Guard the standalone entry point.** A suite reaches a program with
  `import "./prog.boru"`, and an import RUNS the file's top level — so the
  `IO.exit` at the bottom would end the suite before a case had run, and a
  program that reads stdin would block on the runner's own. The guard is
  `IO.env-all` being non-empty, which is a host capability rather than a
  heuristic: `boru run` installs the environment and `boru test` does not.
- **Do not name a local after a built-in.** `append`, `emit`, `dup`, `word`,
  `all` and friends pass `boru check`, run correctly under the bytecode
  compiler, and then die at runtime under `-no-compile` when the interpreter
  tears the binding down with `undef`.
- **`boru fmt` is not idempotent here** (NUR046), which is why `make fmt` is a
  target but not part of `make all`, and why these sources are hand-formatted.
