# boru CLI Reference

The `boru` binary bundles the language runtime, the REPL, a static
type-checker, a code formatter, a module-packaging toolchain, a
registry client, a local key vault, a Language Server, and a
service supervisor. This document describes every subcommand it
supports.

## Contents

* [Quick start](#quick-start)
* [General usage](#general-usage)
  * [`--options`](#--options--engine-options-as-jsonic)
  * [Bytecode compilation](#bytecode-compilation)
* [Language execution](#language-execution)
  * [`boru` / `boru run`](#boru--boru-run)
  * [`boru do`](#boru-do)
  * [`boru check`](#boru-check)
  * [`boru test`](#boru-test)
  * [`boru help`](#boru-help)
  * [`boru describe`](#boru-describe)
  * [`boru fmt`](#boru-fmt)
  * [`boru model`](#boru-model)
  * [`boru build`](#boru-build)
* [Debugging](#debugging)
  * [`boru debug`](#boru-debug)
* [Project lifecycle](#project-lifecycle)
  * [`boru prep`](#boru-prep)
  * [`boru pack`](#boru-pack)
  * [`boru clean`](#boru-clean)
* [Registry client](#registry-client)
  * [`boru install`](#boru-install)
  * [`boru register`](#boru-register)
  * [`boru login`](#boru-login)
  * [`boru publish`](#boru-publish)
* [Secrets](#secrets)
  * [`boru vault`](#boru-vault)
* [Permissions](#permissions)
  * [`boru policy`](#boru-policy)
  * [Per-command policy flags](#per-command-policy-flags)
* [Supervisor control](#supervisor-control)
  * [`boru ctl`](#boru-ctl)
* [Long-running services](#long-running-services)
  * [`boru repl`](#boru-repl)
  * [`boru registry`](#boru-registry)
  * [`boru lsp`](#boru-lsp)
  * [`boru exec`](#boru-exec)
  * [`boru serve`](#boru-serve)
  * [`boru tui`](#boru-tui)
* [REPL meta-commands](#repl-meta-commands)
* [Exit codes](#exit-codes)


## Quick start

```bash
# Until v0.1.0 is tagged, build from a clone:
git clone https://github.com/boru-lang/boru
cd boru/cmd/go && go install ./boru

boru -version
boru                                 # start the REPL
boru do 'add 1 2'                    # one-shot expression
boru script.boru                      # run a file
boru check script.boru                # type-check without running (but see below re: imports)
boru check src/*.boru                 # …every named file, exit 1 if any fails
boru fmt script.boru                  # format in place (always rewrites)
```


## General usage

```
boru [options] [script.boru]
boru <subcommand> [args...]
```

When the first argument is a registered subcommand, the binary
dispatches to it. Otherwise the legacy "execute or REPL" path runs.

Global flags accepted by `boru` (and equivalently by `boru run`):

| Flag | Meaning |
|------|---------|
| `-e EXPR` | Evaluate `EXPR` and exit. |
| `-r PATH` | Path to a local registry (used by import and install). |
| `-s INT` | Random seed for ID generation. Default: current time. |
| `-check` | Run static type-check before execution; abort on error. |
| `-compile` | **Experimental.** Execute via the bytecode compiler when the program is compilable; when the emitter refuses, the run is **silently** re-run on the interpreter — containment for an open defect, not a fallback (see [Bytecode compilation](#bytecode-compilation)). |
| `-force-compile` | **Experimental.** Require the bytecode compiler; abort with the refusal reason if the program is not compilable (see [Bytecode compilation](#bytecode-compilation)). |
| `-options OPTS` | Engine options as a jsonic blob (see below). |
| `-version` | Print the version and exit. |


### `--options` — engine options as jsonic

`--options` takes a single **jsonic** value — relaxed JSON where a bare
`key:value,key:value` is an implicit map and a **colon chain nests**, so
options read like dotted paths:

```bash
boru --options tape:initial:65536 script.boru          # {tape:{initial:65536}}
boru --options 'tape:initial:65536,tape:grows:9'      # two tape knobs
boru --options 'tape:{initial:65536,grows:9,factor:3}' # explicit nested map
```

The blob is a **nested map**; the same jsonic that parses boru data parses
it (`a:1,b:c:2` → `{a:1, b:{c:2}}`). Option handling is **strict** — an
unknown key or a wrong-typed value is an error, not a silent no-op, so a
typo fails loudly:

```
$ boru --options tape:boguskey:10 -e 'add 1 2'
error: unknown tape option "boguskey" (known: initial, grows, factor)
```

Recognised options:

| Path | Type | Meaning | Default |
|------|------|---------|---------|
| `tape:initial` | int | Initial execution-tape capacity, in entries. | program size, floored at 1024 |
| `tape:grows`   | int | Max number of tape reallocations (N). | 7 |
| `tape:factor`  | number | Per-grow size multiplier (M). | 2.7 |

The tape is the engine's execution buffer. It grows at most `grows`
times by `factor` from `initial`, a hard ceiling of `initial · factorᴺ`
entries; crossing 90/95/99% of it warns on stderr and exceeding it fails
loudly with `[boru/tape_exhausted]` — the engine never consumes unbounded
space. Raise `tape:initial` (or `tape:grows`) for a legitimately large
program (deep recursion, huge generated programs); lower it to trip a
runaway sooner. See `design/TAPE-DATA-STRUCTURE.10.md`.


### Bytecode compilation

> **Experimental.** boru ships a bytecode compiler that lowers the
> statically-typed subset of a program to a compact instruction stream and runs
> it on a small VM. **Execution defaults to the compiler** — the standard is
> that every valid program compiles, with no exceptions. Anything the emitter
> refuses is a **defect** owed a fix, not a supported outcome; today such a
> program is **silently** re-run on the interpreter, which is scaffolding
> absorbing that defect — the answer comes back and nothing in the run says
> the compile failed, which is what makes the silence bad rather than
> harmless. The interpreter is not a fallback the design leans on, so the
> mode is not merely a performance knob: use `--force-compile` to make a
> refusal visible, and `--no-compile` to pin the interpreter.

There are three modes, selected per run by a flag or an environment variable:

| Flag | Env | Behaviour |
|------|-----|-----------|
| `--compile` | `BORU_COMPILE` | **Default.** Compile and run on the VM when the whole program is compilable; when any part is not, the program is **silently** re-run on the interpreter. The result is the same either way, which is precisely why the failure hides: the refusal is an open defect and nothing in the run reports it. |
| `--force-compile` | `BORU_FORCE_COMPILE` | **Strict.** *Require* the bytecode path. If the program is not compilable, abort with the emitter's refusal reason instead of re-running it on the interpreter. Use this to *guarantee* a run went through the compiler, and in CI to keep refusals visible (verifying the compilable subset, benchmarking the VM, or catching a compiler defect the silent re-run would hide). |
| `--no-compile` | `BORU_NO_COMPILE` | **Interpreter.** The kill switch: it **overrides** both of the above *and* the default, so a deployment (or a differential run) can pin the interpreter. |

Note that `--compile` / `BORU_COMPILE` select the same mode the default
already uses; they remain accepted, and are worth writing when a script
wants the intent on the record.

Precedence: `--no-compile` / `BORU_NO_COMPILE` wins over everything; otherwise
`--force-compile` / `BORU_FORCE_COMPILE` wins over `--compile` /
`BORU_COMPILE`; with none of them set, the mode is the default compile-first
one, refusals included (`cmd/go/internal/run/run.go`, `ResolveCompileMode`).

```bash
boru --compile script.boru              # compile; a refusal is silently interpreted
boru --force-compile script.boru        # demand the compiler; fail loudly if it can't
BORU_COMPILE=1 boru script.boru          # same as --compile, via the environment
boru --no-compile script.boru           # pin the interpreter
BORU_NO_COMPILE=1 boru --compile s.boru  # kill switch: runs the interpreter anyway
boru do --force-compile 1 add 2        # the flags work on `boru do` too
```

A `--force-compile` refusal names the construct the emitter could not lower —
which is to say it names the defect, and that defect is owed a fix:

```
$ boru --force-compile -e '(size (for 5 [i]))'
error: force-compile: consumes loop results (Stage 2 loops only feed the program residual)
```

Both flags are accepted by `boru` / `boru run` and by `boru do`. Genuine runtime
errors (e.g. division by zero, a type error) surface identically in every mode;
only the *uncompilable* outcome differs — silently absorbed by `--compile`, so
the defect goes unreported, and named by `--force-compile`.

## Language execution

### `boru` / `boru run`

Execute a script, an `-e` expression, or drop into the REPL when
nothing is supplied.

```bash
boru                         # REPL
boru -e 'add 1 2'            # prints "3"
boru script.boru              # runs the file
boru -check script.boru       # type-check first, then run
boru -e '...' -r ./registry  # with a custom registry
```

Output: the final stack contents, space-separated, on stdout.
Errors go to stderr; exit code 1 on failure.

**Program arguments (`IO.args`).** Anything after the script path — or,
in `-e` mode, after the expression — is the program's own argument
vector, readable with `IO.args` (`import "boru:io"`). `-e` **ends option
processing** (the `node -e` / `python -c` convention), so a dash-prefixed
first argument reaches the program instead of being read as a `boru`
flag:

```bash
boru script.boru --fast a       # IO.args → ['--fast' 'a']
boru -e '…' --fast a           # IO.args → ['--fast' 'a']  (no separator needed)
boru -e '…' -- --fast          # a leading `--` is accepted and stripped
```

`boru` flags for the run itself go **before** the script/`-e` expression
(`boru -s 5 -e '…' --fast` seeds the run and passes `--fast` to the
program).

**Environment (`IO.env`).** `boru run` and a built binary both install the
real process environment, readable with `IO.env <name>` — the value, or
`none` when the name is unset. `""` is a real value, distinct from unset,
so a program can tell `FOO=` from no `FOO` at all. `IO.env-all` returns
the whole visible environment as a Map, sorted by name; it is a separate
word rather than a no-argument form of `IO.env`.

```bash
NO_COLOR=1 boru script.boru      # IO.env "NO_COLOR" → '1'
boru script.boru                 # IO.env "NO_COLOR" → none
```

The environment is a **capability**, not ambient state: an embedded host
that installs none (the default for `lang.New`, and what the spec runner
does) sees every name unset and an empty `IO.env-all` — the runtime never
reaches for the real environment behind the host's back. A policy can
narrow it further: the `env` scope's `read` op takes a `name` argument, so
`read-only` exposes an allowlist (`LANG`, `TZ`, `BORU_*`) and everything
else reads as unset, while `compute` uninstalls the capability outright.
A denied name reads as unset rather than raising — a program probing for
an optional variable takes its default path, and an error would leak which
names exist.

**Exit codes (`IO.exit`).** `IO.exit <code>` ends the program with a
status of the caller's choosing (`0..125`). It is what makes a boru program
usable in a shell pipeline: `if mytool …; then` reads the status, and
without it every program could only ever say 0 (clean) or 1 (any failure).
Exiting is not failing — nothing is printed, for any code, and the residual
stack is not flushed.

```bash
boru run check.boru; echo $?      # whatever the program asked for
```

126, 127 and 128+n are refused at the call: they are the shell's own
(`not executable`, `not found`, `killed by signal n`), and a program that
returns one misreports how it died. A refused code is an ordinary failure —
status 1 with the range explained.

The convention `boru:cli` will follow, and worth following by hand until
then: `0` success, `1` runtime failure, `2` usage error.

**Filters and terminals (`IO.read-line`, `IO.is-tty`).** `IO.read-line` yields
the next line of a stream or `File` handle without its terminator, and
`none` at end of input — which is what lets a filter loop terminate, since
`""` is a legitimate blank line. LF and CRLF both strip; a final line with
no trailing newline is still a line. Whole-input slurping stays
`IO.read (IO.stdin)`, and the two share one reader, so a `read-line`
followed by a `read` yields the rest of the stream rather than losing
whatever the line read had buffered.

`IO.is-tty` answers per stream, because the asymmetric case is the common one —
stdout piped while stderr is still a terminal. With `IO.env "NO_COLOR"` it
is the whole colour decision, without the runtime hard-coding it:

```bash
mytool | cat            # IO.is-tty (IO.stdout) → false
mytool                  # IO.is-tty (IO.stdout) → true on a terminal
```

The word is `tty`, not the `tty?` of `design/CLI-PROGRAMS.0.md` §5: `?` is a
fixed lexer token (the optional-param marker), so `tty?` cannot be
dispatched at all — and in a dot chain the mis-parse is silent, so the RFC
spelling would have shipped a word that quietly did nothing.

Both are host capabilities, so an embedded host decides what they see. With
no probe installed every stream answers `false`, and a permissions profile
that uninstalls the `terminal` scope clears it too — five of the seven shipped
profiles do. That is a deliberate design choice over gating the word per
call: `Check` on an uninstalled scope *raises*, which would abort the colour
idiom above under any sandbox instead of answering it. `false` is a usable
answer; an error is not.

Under the hood `IO.exit` raises a **reserved control error** (`boru/exit`,
carrying `{code}`); nothing in the runtime calls `os.Exit`. That is what
lets each driver decide what a program's exit request means to it, and the
decisions differ where they must:

| Driver | What `IO.exit N` does |
|---|---|
| `boru run`, `boru do`, a built binary | exits with `N`, printing nothing |
| the REPL | ends the session, reporting the code |
| `boru test` | ends **that file** — its remaining cases do not run, and it is reported as errored, because a suite that exits half-way has not passed the cases it never reached |
| `Vm.run-sandbox` and the other sub-engines | converted to an ordinary error: a sandboxed program must not be able to terminate its host |
| a served `/v1/exec` request | reported as the request's outcome; the server keeps serving |
| an embedded host | whatever it decides — read the code with `lang.ExitCode(err)` |

A handler-less `do [...]` does **not** catch it. `do`'s escape hatch turns a
body error into an Error value, which is right for a failure, but an exit is
a control transfer, and demoting it to data would silently turn `IO.exit 4`
into exit 0 for any program whose exit happens to sit inside a `do`. (This
deviates from `design/CLI-PROGRAMS.0.md` §4, which sketched `do … error …`
handlers observing the exit; `error` receives `do`'s *result*, so letting a
handler see it means letting a plain `do` swallow it, and the trap is worse
than the loss.)

### `boru do`

Evaluate the remaining args as a boru expression. Slightly more
shell-friendly than `boru -e` because positional words don't need
extra quoting.

```bash
boru do add 1 2                  # prints 3
boru do 'import "boru:string-util" "hello" StringUtil.upper'  # prints HELLO
boru do 'iota 5 each [dup mul]'  # prints [0 1 4 9 16]
```

If the expression **begins** with a negative number, the leading
`-N` is parsed as an unknown command-line flag
(`flag provided but not defined: -7`). Separate the flags from the
expression with `--`:

```bash
boru do -- '-7 0 add'           # leading negative literal needs --   (prints -7)
```

`boru do` accepts the same `--compile` / `--force-compile` flags as `boru run`
(see [Bytecode compilation](#bytecode-compilation)); place them before the
expression: `boru do --force-compile 1 add 2`.

### `boru check`

Run the static type-checker without executing. It drives the same
engine in carrier mode — so checking stays in lockstep with runtime
dispatch — reports diagnostics to stderr, and exits non-zero when any
Error-severity diagnostic is found (exit 0 with `--soft`; add
`--pedantic` to gate on the advisory tiers too).

**Every target is checked.** `check` takes any number of file and
directory targets: a file is checked as named, a directory contributes
every `.boru` file beneath it (skipping `.boru/`, which holds fetched and
generated package artifacts), and a path named twice is checked once.
Diagnostics **accumulate** — a file that fails does not stop the files
after it — and the command exits `1` when **any** target reported an
error, so `boru check src/*.boru` is green only when every one of those
files is clean. A target that expands to no `.boru` file is an error,
not a quiet success: checking nothing must never report green. A
mistyped or unreadable target fails the invocation before any file is
checked, so a bad path can never be mistaken for a clean run.

With several targets the report gains attribution — a `check: file
<path>` banner on stderr ahead of each file's diagnostics, `check:
<path>: <carriers>` on stdout, and a closing `check: total …
across N file(s)` line. The one-target report is unchanged, as is the
`check: row:col: [sev] code: detail` diagnostic one-liner that editors
and CI scripts key on.

**Relative imports anchor per file.** Each target's `import
"./lib.boru"` resolves against **that file's own directory** — a
multi-file run has no single working directory that could be right for
every target — which is the same anchor a binary built by `boru build`
uses. `boru run` and `boru debug` still resolve a script's relative
imports against the process working directory, so a program checked from
elsewhere can still be refused when run from there.

**One exception: imported module bodies do run** — with their effects
modelled. The checker cannot type `Mod.value` without the module's real
exports, so `import` executes during the check pass, and module bodies
deliberately do not run in check mode (carrier-stripping would destroy
the string literals a body uses as export names). What the body does NOT
get is the ambient authority to change anything:

* **filesystem writes are modelled.** The body runs against a
  mem-over-real overlay, so a `write` / `folder` / `remove` / `open` in a
  module body leaves the real filesystem untouched. Reads still resolve —
  a body that loads a config file at import time works exactly as it
  does at runtime, and one that writes a file and reads it back still
  sees its own bytes.
* **output is discarded.** A module that prints at load draws nothing
  during a check.
* **a network send and a `stdin` read are not modelled** and still
  happen. No invented response is safe for a body that reads one — a
  fabricated status takes a wrong branch, a fabricated body fails a real
  parse — and stdin is a read, where the rule above is that reads stay real
  so nothing that loads today starts failing.

Each execution is reported as an info diagnostic
(`module_body_executed_in_check`) naming the module.

File modules are not cached, so the body runs once per importer per pass.
A plain `boru script.boru` pre-flight checks and then runs, so an imported
body executes twice — but only the second execution's effects are real,
because the compile pass is the one the program's own module load happens
in. Use `--no-check` to skip the pre-flight pass entirely.

```bash
boru check script.boru
boru check src/*.boru                # every named file is checked
boru check src                       # every .boru file under src/
boru check -e '1 add "x"'
boru check --json script.boru        # machine-readable output
boru check script.boru --json        # flags may follow the targets
boru check --soft script.boru        # exit 0 even on errors
boru check --strict script.boru      # surface every dynamic dispatch
boru check --pedantic script.boru    # exit non-zero on warnings and infos too
boru check -- -odd-name.boru         # -- ends flag parsing for its segment
boru check -h                        # the documented usage, exit 0
```

Flags:

* `-e EXPR` — type-check an inline expression. Mutually exclusive with
  file targets.
* `--json` — emit JSON diagnostics. One target emits the bare
  `CheckResult` object; several emit an array of
  `{"file": …, "result": …, "error": …}` entries, one per file, in the
  order they were checked.
* `--soft` — return exit code 0 even when diagnostics are reported.
* `--strict` — additionally report (as non-gating info) every dispatch
  over a dynamic operand: the points where the checker matched
  optimistically and the runtime re-verifies. The gradual-typing
  migration surface — tighten these and the diagnostics disappear.
* `--pedantic` — promote the advisory tiers: exit non-zero when any
  warning- or info-severity diagnostic is reported, not only on errors.
  Without it every non-error run exits 0, so a `warning` cannot fail a
  build; with it, CI can gate on advisories while a local
  `boru check` and `boru run` stay unchanged. It composes **under**
  `--soft`: `--soft` still means "never gate", so `--soft --pedantic`
  exits 0. Applies identically to `--json`. Both flags govern *reported
  diagnostics* only — unparseable source or an unreadable file still
  exits 1, since there is no diagnostic summary to downgrade.
* `-r PATH`, `-s SEED` — same as `boru run`.
* `--color auto|always|never` — as for `run` and `do` (see **Color**
  below).

**What it catches** (full list in the language reference's diagnostics
table):

* `no_signature` / `uncalled_function` — a call that matches no
  signature. `uncalled_function` is the function-VALUE form: a named
  function reached as a call (e.g. an imported `Pkg.fn`) with arguments
  that match none of its signatures. It errors at runtime too — the check
  reports the same finding at the same place, without running the
  program. Write `Pkg.fn/v` when you mean to pass the function itself.
* `unreachable_signature` — an `fn` overload that an earlier, more
  general overload already subsumes, so first-match dispatch can never
  reach it.
* `stranded_type_call` — a capitalised name bound to a **function** body
  written in call position. `def` on a capitalised name binds a TYPE, so
  the name never calls: it places its lattice node and leaves the
  arguments after it unconsumed, and the program still exits 0 with a
  wrong stack. The combinator names are all capitals, so transcribing
  `S`, `K`, `I` lands here first; lowercase the name to bind a callable
  function. The deliberate predicate-type use —
  `def Even fn n:Integer Boolean [eq 0 (mod 2 n)] end 4 is Even` — stays
  silent, as does any type carried as data.
* `undefined_word`, `unused_def`, `unreachable_branch`,
  `record_shape_mismatch`, … — typos, dead bindings, constant `if`
  branches, record-field mismatches.

**Every run pre-flights by default.** `boru run` (and `boru -e`) runs
the checker first and aborts before executing if any error is found,
so a type bug can't slip into a run. The default gate is quiet: a
clean program produces no checker output at all (stdout and stderr
stay clean for the program's own output); diagnostics print only when
the run is about to abort. `--check` upgrades the pre-flight to
verbose (all diagnostics, including the advisory tiers);
`--no-check` (or `BORU_NO_CHECK=1`) skips the pre-flight entirely:

```bash
boru script.boru               # checked by default; aborts on any error
boru --check script.boru       # verbose pre-flight: advisories too
boru --no-check script.boru    # skip the pre-flight (runtime errors only)
```

**Rich diagnostics.** Errors and check findings render as full
diagnostic reports: the source excerpt with a caret under the failing
word, secondary labeled locations (the declaration a violated contract
came from, where an offending value was produced), `= note:` lines
explaining *why* — for a failed call, per-candidate verdicts like
``candidate `upper (String)` — argument 1: expected String, got 99 (an
Integer)`` — and `= help:` lines with actionable fixes (``did you mean
`upper`?``, "did you swap the arguments?", `see boru describe <word>`).
Each `check:` one-liner keeps its stable grep-able format; the rich
block below it is additive.

**Color.** `run`, `do`, and `check` take `--color auto|always|never`
(default `auto`: color only when the output is a real terminal, and
never when `NO_COLOR` is set). The REPL auto-detects the same way.
The CLI's own `auto` decision reads `NO_COLOR` through the *same*
environment capability `IO.env` reads, and tests the destination with the
same character-device probe `IO.is-tty` uses — so a program and the
diagnostics printed around it can never disagree about either. A host
that installs a filtered environment view filters what the CLI sees too;
before any program instance exists, `auto` falls back to the process.
Machine-read surfaces — `check --json`, the LSP, the wasm playground —
are always plain.

```bash
boru do --color always '99 upper'   # force the ANSI palette (e.g. under a pager)
boru check --color never script.boru # plain text even on a terminal
```

**In your editor.** `boru lsp` publishes these same diagnostics as you
type — see [`boru lsp`](#boru-lsp).

### `boru test`

Discover and run test suites written with the `boru:test` framework. A
suite is any `*_test.boru` file that imports `boru:test` and registers
cases with `Test.test` / `Test.describe` (parking cases with
`Test.skip`). `boru test` runs each file, prints its per-case report,
and exits non-zero if any case failed or any suite errored.

```bash
boru test                            # run every *_test.boru under the cwd
boru test math_test.boru              # run one suite file
boru test src/ lib/                  # walk several directories
boru test --coverage sift_test.boru   # add coverage + an HTML report
```

Suites run on the **bytecode compiler by default** — the normal execution
mode. A file the compiler refuses is **silently** re-run on the interpreter,
so the suite still reports green while the compile failed; that refusal is an
open defect, and `--force-compile` is how you surface it. Discovery: no
arguments walks the current directory recursively for `*_test.boru`; a
directory argument is walked the same way; an explicit file argument is run
verbatim (even without the `_test.boru` suffix).

Flags:

* `--coverage` — measure line coverage of every user module a suite
  imports (`import "./mod.boru"`), aggregated across all suites. It prints
  a summary line per module —
  `cover <id>  <pct>% (<covered>/<total>)  uncovered: <lines>`, where the
  `uncovered:` list names the exact source lines that never ran — and
  writes a browsable **HTML report** to a `coverage/` folder: an
  `index.html` summary plus one page per module with each source line
  coloured covered (green) or uncovered (red). Because the bytecode VM
  folds some source positions, compiled coverage is a *subset* of the
  interpreter's (some folded lines show as falsely uncovered); pair with
  `--no-compile` for the exact, line-granular set when driving a module
  to full coverage.
* `--coverage-dir PATH` — where the HTML report is written (default
  `coverage`). Ignored without `--coverage`.
* `--coverage-min PCT` — fail the run (exit 1) when **aggregate** line
  coverage — total covered lines over total executable lines across every
  imported module — is below `PCT`, even if every test passed. Implies
  coverage measurement, so `boru test --coverage-min 80` gates without
  needing `--coverage` (add `--coverage` too if you also want the HTML
  report). Pair with `--no-compile` so folded compiled positions don't
  drag the number below the real figure.
* `--no-compile` / `--force-compile` / `--compile` — select the engine
  exactly as for [`boru run`](#bytecode-compilation).
* `-r PATH` — registry path, same as `boru run`. The permission flags
  (`--perms`, `--allow`, `--deny`, …) are accepted too.

### `boru help`

`help` documents the **boru tool**: its subcommands and their flags.
(For the **language** — words, categories, and modules — use
[`boru describe`](#boru-describe).)

```bash
boru help                    # introduction + the subcommand list (this overview)
boru help vault              # one subcommand: summary, and where its flags live
boru help check
```

With no argument it prints an orientation to the tool — the two kinds
of help (`help` for the CLI, `describe` for the language), the usage
forms, and every subcommand. With a subcommand name it prints that
command's one-line summary and points at `boru <subcommand> -h` for the
full flag set. An unknown name exits non-zero and suggests
`boru describe <name>` in case a language word was meant.

### `boru describe`

`describe` documents the **boru language**: its built-in words, the
categories they fall into, and the loadable modules.

```bash
boru describe                       # a categorised guide to every word and module
boru describe add                   # full docs for one word: signatures, examples, notes
boru describe math                  # the words in one category
boru describe boru:type-util         # a module and the words it exports
boru describe boru:type-util:tpartial   # one exported word of a module
```

The forms:

* **no argument** — every built-in word grouped by category (math,
  compare, boolean, string, type, …), then the loadable modules, then
  the drill-in forms below.
* **`<word>`** — the word's signatures, precedence, description, and
  examples (the same data the REPL `describe` word and the LSP hover
  show). A bare name is matched as a category first, so `describe type`
  opens the *type* category; use `describe typeof` for the word.
* **`<category>`** — the words in that category, each with its one-line
  summary.
* **`boru:<module>`** (or a bare built-in module id like `math-util`) —
  the module's summary and exported words.
* **`boru:<module>:<word>`** — a single exported word, with its module
  provenance.

A name that matches none of these is given one more chance: `describe`
tries to **load it as a module** — a native `boru:` module, an installed
module, or a file path (`./lib.boru`) — and describes it if the load
succeeds. Only when that fails does it report that nothing is known by
that name.

Inside the REPL the `describe` *word* and `/describe` meta-command look
up a single word the same way.

### `boru fmt`

Format `.boru` source **in place**. `fmt` takes file paths only — it has
no flags and no stdout/diff mode; every named file that changes is
rewritten and its path is printed. With **no** arguments it walks the
current directory tree and reformats every `.boru` file it finds
(skipping anything under `.boru/`).

```bash
boru fmt script.boru              # rewrite this file in place
boru fmt lib/*.boru               # rewrite several files in place
boru fmt                         # rewrite every .boru file under the cwd
```

### `boru model`

Build a [voxgig](https://github.com/voxgig/model) model: unify a
`.jsonic` model (CUE-style unification, via
[aontu](https://github.com/rjrodger/aontu)) into a single JSON model and
write it to `<base>/<name>.json`. It is the command-line twin of the
`boru:model` language module and mirrors the `@voxgig/model` CLI. Generator
*actions* are Go callbacks that cannot be loaded from the command line, so
this command performs the core job — resolving the model and writing the
JSON — with an optional watch loop.

```bash
boru model model/model.jsonic            # build -> model/model.json
boru model --print model.jsonic          # print the unified JSON to stdout (no file written)
boru model --dryrun model.jsonic         # resolve only; write nothing
boru model --base build model.jsonic     # output + @"..." imports resolve from ./build
boru model --watch model.jsonic          # rebuild on change until Ctrl-C
```

Flags: `--base` (import / output directory; default the model file's
directory), `--watch`, `--dryrun`, `--print`, and `--config` (resolve a
`.model-config` build — off by default). A unification conflict, an
unresolvable reference, or a missing source file exits non-zero with the
error on stderr.


### `boru build`

Compile a boru program into a **standalone native executable** — a single
file that runs the program without needing the source or a separate `boru`
install. The produced binary runs through the full interpreter, so it
executes *any* program, and prints the residual stack exactly as `boru run`
would.

```bash
boru build prog.boru                  # -> ./prog  (default name = basename)
boru build prog.boru -o tool          # -> ./tool
./prog                              # runs the baked-in program
```

There are two build mechanisms:

| Mechanism | How | Tradeoffs |
|-----------|-----|-----------|
| **Self-embedding launcher** (default) | Copies the running `boru` binary and appends the program as a payload; at startup the copy detects the payload and runs it. | No Go toolchain or network needed; works from a stock `boru`. Binary is ≈ the size of `boru` (it bundles the whole runtime) and targets the host OS/arch only. |
| **Native** (`--native`) | Generates a tiny Go `main` that embeds the program and runs `go build`. | Smaller, dead-code-eliminated binary; cross-compiles via `GOOS`/`GOARCH`. Requires the Go toolchain **and** the boru source/module graph — set `BORU_SRC` to a repo checkout, or run from within the `cmd/go` module. |

**File imports are bundled.** A program that imports other `.boru` files
(`import "./lib.boru"`) is fully self-contained: every transitively-imported
file is embedded and resolved from an in-memory file system at run time, so
the binary works even when the source files are gone. Built-in `boru:`
modules (`import "boru:math-util"`, …) are already part of the runtime and
need no bundling. An import that is neither a `boru:` module nor an explicit
`.boru`/`.lang` file path is rejected at build time.

**Every build pre-flights by default.** `boru build` runs the same static
pre-flight as `boru run` (see [`boru check`](#boru-check)) and refuses to
produce a binary if any error is found — a type bug that would abort the
built tool's very first execution fails the *build* instead, with the
diagnostics on stderr. The gate is quiet on a clean program, resolves
relative file imports against the entry file's directory (exactly how the
built binary will resolve them, wherever the build is invoked from), and —
like `boru check` — executes imported file-module bodies to learn their
exports. `--no-check` (or `BORU_NO_CHECK=1`) skips the pre-flight and
builds anyway.

Flags:

| Flag | Meaning |
|------|---------|
| `-o <path>` | Output binary path. Default: source basename without `.boru` (`prog.boru` → `prog`). |
| `--native` | Use the Go-toolchain path instead of the self-embedding launcher. |
| `--keep` | (native) Retain the generated temp build directory and print its path. |
| `--compile` / `--force-compile` | Bake the experimental bytecode compile-mode into the binary (see [Bytecode compilation](#bytecode-compilation)). `--force-compile` makes the produced binary abort on uncompilable input. |
| `-r <registry>` | Registry path baked into the binary. |
| `-s <seed>` | Random seed baked into the binary. |
| `--options <jsonic>` | Engine [`--options`](#--options--engine-options-as-jsonic) baked in (validated at build time). |
| `--no-check` | Skip the static pre-flight check and build anyway (env: `BORU_NO_CHECK=1`). |

A missing source file, an unbundlable import, a failed pre-flight check, or
(for `--native`) a failed `go build` exits non-zero with the error on stderr.


## Debugging

### `boru debug`

The interactive debugger (design/BORU-DEBUGGER.0.md), plus the
cross-process introspection server and client it grew out of.

**Interactive launch** — run a program under the debugger:

```bash
boru debug prog.boru                      # pause at the first source line
boru debug --script cmds.dbg prog.boru    # drive the session from a command file (CI)
boru debug --post-mortem prog.boru        # on an uncaught error, inspect the fault state
boru debug --no-check prog.boru args...   # skip the pre-flight; args reach IO.args
```

The program runs on the interpreter (the per-step trace does not fire
on the compiled VM path) and pauses **between source lines** — one stop
per line, however many engine steps the line expands to. At the
`(adbg)` prompt:

| Command | Does |
|---------|------|
| `step`, `s` | run to the next source line, **descending into body evaluations** (`each`/`fold`/`do` bodies, paren groups — labelled `(in body)`) |
| `next`, `n` | like `step`, but run through deeper frames and bodies — stay at statement level |
| `out`, `o` | run until the current fn frame returns |
| `continue`, `c` | run to the next breakpoint (or completion) |
| `break <spec>` | set a breakpoint: a source line (`break 12`) or a word (`break add`); bare `break` lists them |
| `delete [spec]` | delete one breakpoint; bare `delete` clears all |
| `watch <name>` | pause when the binding's value changes (old → new); bare `watch` lists |
| `unwatch [name]` | delete one watch; bare `unwatch` clears all |
| `quit`, `q` | **detach** — the program continues to completion |
| `stack` | every data-stack entry, top (`#0`) first |
| `bt` | backtrace of live fn frames — including module fns' |
| `back` | browse one recorded step earlier (time-travel, read-only) |
| `forward`, `fwd` | browse one step later; at the newest entry, return to the live pause |
| `history` | list the recorded trail, newest first |
| `replay` | re-run from the start to the browsed entry (side effects re-run) |
| `defs`, `locals` | the program's bindings (`defs all` for every binding) |
| `print <expr>`, `p`, `eval` | evaluate in the paused program's scope |
| `list`, `l` | source around the current line |
| `help`, `h`, `?` | the command list |

Breakpoints come in four kinds. From the prompt (or `--break` at
launch): a **source line** (`break 12` — pauses on entering the line,
once per loop iteration, including inside body evaluations) and a
**word** (`break add` — pauses whenever that word is about to
dispatch). In source: the `boru:debug` module's inline markers —
`Debug.break` pauses whenever the debugger is attached (and is a no-op
otherwise, so it is safe to commit), `Debug.break-when <cond>` pauses
conditionally. And **data watchpoints**: `watch n` pauses when the
binding `n` changes value — including its first definition — showing
old → new (the compare runs per step, only while watches are set).
`quit` cannot kill the program mid-run — the engine has no preemption
seam (ADR-005) — so it detaches and the program drains at full speed.
End-of-input on the command stream (Ctrl-D, or a drained `--script`
file) also detaches. In `--script` mode each command is echoed after
the prompt, so the transcript is a self-contained, reproducible record
— the debugger's CI story.

Two error-debugging modes. With `--break-on-error`, every raise
pauses **before it unwinds** — including errors a `do` handler will
catch — with the stack, scope, and backtrace live at the fault;
resuming lets the error proceed to its handler (or out of the
program). With `--post-mortem`, an **uncaught** error opens one final
inspection prompt over the fault state before the error reaches
stderr and the run exits 1. Errors a `do` handler catches are not
post-mortems; a session you already `quit` stays closed.

**Time travel** — every line-level stop is recorded in a bounded ring
(the last 64, in every run mode), and `back`/`forward` browse it
read-only: `stack`, `bt`, and `list` follow the browsed snapshot,
while `print` and `defs` stay live (bindings are not rewound — the
view says so). `history` lists the trail. Resuming execution clears
the browse cursor. From a browsed entry, `replay` re-runs the program
FROM THE START on a fresh registry and pauses at that entry's line
stop — deterministic re-execution reproduces the state, and from
there you can step **forward** through a moment the live run already
passed. The price is stated up front: the program's side effects
re-run.

**Editor integration** — `--dap` speaks the Debug Adapter Protocol
over stdio (Content-Length-framed JSON), so any DAP client can drive
the same session: launch, line breakpoints, function breakpoints
(`setFunctionBreakpoints` maps onto word breakpoints — a
concatenative program's functions ARE its words), stepping
(`next`/`stepIn`/`stepOut`), stack/scopes/variables, evaluate, and
stopped events (`breakpoint`, `step`, `exception` under
`--break-on-error`). Program output arrives as `output` events; the
program's exit code is reported in the `terminated`/`exited` events.
`--dap` and `--script` are mutually exclusive; `pause` is refused
honestly (no preemption seam — same as `quit` above).

`bt` chains frames across every engine running on the program's
registry — a pause inside an `each` body still shows the enclosing
fn's frame — and breakpoints reach **module fn bodies** too (their
captured registries resolve the session's hook through the import
chain). A pause inside a file-imported module names the MODULE file
and its own source text — the banner, `list`, and the DAP
`stackTrace` all follow the firing engine's file — and the module
fn's own frame appears in `bt` (the engine chain spans the import
registries, and each named call labels its body engine).

Flags: `--script F`, `--break SPEC` (repeatable), `--break-on-error`,
`--post-mortem`, `--dap`, `--no-check` (also `BORU_NO_CHECK`),
`--color auto|always|never`.

Current limits (see the design note's roadmap): a concurrent fork's
pauses are delivered best-effort (dropped whenever the session is
busy) and are not labelled with the fork's identity; relative
`import` paths resolve against the working directory (matching
`boru run`); a marker pause inside an imported module names the module
file but `list` has no module source to page there; and `step` may
surface engine-internal evaluations of multi-line fn literals as
extra `(in body)` stops — scripted sessions should drive to a
location with breakpoints or markers rather than counted steps.

**Cross-process introspection & remote stepping** — serve a runtime's
state over HTTP, and interrogate or DRIVE it:

```bash
boru debug serve [--bind 127.0.0.1:7777] [--token T] [--step] [file.boru]
boru debug attach <words|defs|heap|eval SRC|events ID> [--url U] [--token T]
boru debug attach <pause|step|next|out|continue|quit|break SPEC|delete SPEC>
```

`serve` loads `file.boru` (if given), then serves the registry's
introspection endpoints until interrupted, writing a discovery file
(`$TMPDIR/boru-debug.json`) that `attach` reads by default. The optional
Bearer token is a static string or a vault capability id; binds are
loopback-only unless `--allow-public`.

With `--step`, `serve` instead runs the program **under the remote
stepping debugger**: it pauses at the first source line and waits, and
a separate process drives it with the `attach` stepping verbs —
`pause` shows the current stop (location, source line, stack summary),
`step`/`next`/`out`/`continue` deliver the same actions the `(adbg)`
prompt has (each waits briefly for the next stop and prints it; a
long-running program answers `running` — check again with `pause`),
`break`/`delete` manage line and word breakpoints, and `quit` detaches
(the program drains to completion, exactly like the TTY `quit`).
`attach eval` at a pause evaluates in the paused program's scope —
the stepping server routes it through the session, so it cannot
deadlock against the parked engine. Interrupting the server while the
program is parked also detaches and drains before exit. The stepping
surface shares the introspection transport's auth and its trust
posture: `attach eval` is already remote code execution, so gate both
with `--token`.


## Project lifecycle

A boru "project" is a directory with a `boru.jsonic` manifest plus
one or more `.boru` source files. The lifecycle commands operate on
that directory layout.

### `boru prep`

Parse `boru.jsonic` and write `.boru/boru.json` (the resolved manifest
used by downstream tools).

```bash
boru prep                    # current directory
boru prep ./mymodule         # specific directory
```

### `boru pack`

Build a publishable `.zip` of the current module from the resolved
manifest. Output goes under `.boru/`.

```bash
boru pack                    # uses ./boru.jsonic
boru pack ./mymodule
```

### `boru clean`

Delete everything under `.boru/` except dotfiles. A no-op if the
directory doesn't exist.

```bash
boru clean
boru clean ./mymodule
```


## Registry client

Registries are simple HTTP services that host module zips. The
default registry URL is baked into the binary; override with `-r`.

### `boru install`

Download and install a module by versioned name.

```bash
boru install acme/widgets-1.2.3
boru install acme/widgets-1.2.3 -r https://registry.example.com
```

Installed modules become importable as `acme/widgets`.

### `boru register`

Create an account on a registry. Interactive.

```bash
boru register
boru register -r https://registry.example.com
```

### `boru login`

Log in to a registry; stores a token in the local config. With
`--vault`, the token is stored in the (encrypted) vault under an alias
instead of plaintext `~/.boru/user.jsonic` (requires an initialized vault;
set `BORU_VAULT_PASSPHRASE` or be prompted).

```bash
boru login
boru login -r https://registry.example.com
boru login --vault                          # token → vault (alias: boru-registry-token)
boru login --vault --vault-alias=my-reg     # custom alias
```

### `boru publish`

Upload the current module (or a specified directory) to a registry.
Requires a prior `boru login`. When the token was stored with `boru login
--vault`, publish reads it back from the vault automatically; `--vault`
(optionally `--vault-alias=NAME`) forces a vault read.

```bash
boru publish                                # current dir
boru publish ./mymodule
boru publish -r https://registry.example.com
boru publish --vault                        # read the token from the vault
```


## Secrets

### `boru vault`

A local credentials vault, backed by the OS keyring where possible
(macOS Keychain, Linux Secret Service, Windows Credential Manager,
1Password) or a file fallback.

```bash
boru vault -i                            # interactive TUI (menu-driven; keys shown on screen)
boru vault init                          # initialise, pick backend
boru vault status                        # backend, secret count, lock state, generation
boru vault add --from-clipboard github_token   # read from clipboard, then wipe it
boru vault add --from-stdin github_token       # read one line from stdin
boru vault add github_token                     # prompt (input not echoed)
boru vault add --expiry=90d --from-stdin github_token  # optional expiry reminder
boru vault add --ip-whitelist=10.0.0.0/8,203.0.113.7 --from-stdin ci_key  # restrict proxy use to these client IPs/CIDRs
boru vault list                          # aliases and metadata (incl. EXPIRES, IP-WHITELIST)
boru vault get github_token              # redacted by default
boru vault get github_token --reveal     # show the value
boru vault expiry                        # list pending key expiries, soonest first
boru vault expiry --namespace=proj       # filter expiries by namespace
boru vault expiry --within=30d           # only keys due within 30 days (or overdue)
boru vault expiry set github_token 2026-12-31  # set/replace an expiry
boru vault expiry clear github_token     # remove an expiry
boru vault rm github_token               # remove (also: remove, delete)
boru vault mv github_token proj:gh       # rename / move between namespaces (also: rename)
boru vault mv proj: team:                # rename a whole namespace
boru vault rotate --from-stdin github_token         # replace the value (keeps the alias/metadata)
boru vault rotate --revoke-caps --from-stdin github_token  # …and revoke every capability on it (incident response)
boru vault lock                          # block get/grant until unlocked
boru vault unlock                        # re-enable access
boru vault verify                        # reconcile store + keyring (--prune repairs)
boru vault export --out=vault.borux       # portable, passphrase-encrypted bundle
boru vault import vault.borux             # restore a bundle (or a .env file)
boru vault grant --agent=ci --ttl=2h github_token   # issue scoped capability token
boru vault grant --agent=svc 'proj:*'    # namespace-wildcard capability: read all of proj via the wire protocol
boru vault revoke <token-id>             # revoke a token
boru vault providers                     # list provider presets (built-in + custom)
boru vault provider add --url=https://api.corp.example --auth-style=header:X-Corp-Key corp   # define a custom upstream preset
boru vault provider rm corp              # remove a custom preset (refused while aliases still use it)
boru vault scan .                        # scan files for leaked secret-like strings
boru vault scan --home                   # scan credential dotfiles (~/.npmrc, ~/.netrc, ~/.aws/credentials, …) for plaintext secrets
boru vault scan --home --match-vault     # …and flag which on-disk creds are already vaulted
boru vault audit                         # show the structured audit log
boru vault audit --action proxy.request --last 20
boru vault audit --json                  # raw JSONL
boru vault policy apply policy.boru       # declaratively apply policy
boru vault config                        # show vault config
boru vault config --set namespace.default=proj   # set a config key (also --unset)
boru vault password add --scope=read --namespaces=ci ci-bot  # scoped password (keyslot)
boru vault password add --scope=read --ttl=30m --generate agent  # TEMPORARY password: random, printed once, expires in 30m (hand to an agent)
boru vault password add --rotate --scope=read --ttl=12h --generate agent  # RE-ISSUE under the same name: fresh password + reset TTL, old one invalidated
boru vault password rm --temp            # revoke ALL temporary passwords at once
boru vault password list                 # list keyslots (with scope/namespaces + EXPIRES)
boru vault history                       # content-revision history (newest first)
boru vault restore 7                     # restore metadata to generation 7 (admin)
boru vault proxy                         # run local credential broker (loopback only)
boru vault serve                         # HTTP wire protocol for secret provision (HashiCorp-Vault-style, read-only)
boru vault mcp                           # stdio MCP server over aliases
boru vault exec gh,openai -- mycmd       # run mycmd with secrets in env
boru vault exec --ask GITHUB_TOKEN -- make tag-push   # prompt (echo off) for a value not in the vault, inject as $GITHUB_TOKEN
boru vault exec --ask-passphrase -- make deploy-ts deploy-py  # prompt once, validate, inject BORU_VAULT_PASSPHRASE for nested boru calls
boru vault folder                        # list known vault folders (discovered + recorded by init)
boru vault folder add ~/.othervault      # register an already-existing vault into the index

# Custom location: a folder and/or an inner file-name suffix, by flag or env.
boru vault --folder=/secure/vault --suffix=work init   # vault at /secure/vault/vault.work.*
BORU_VAULT_FOLDER=/secure/vault BORU_VAULT_SUFFIX=work boru vault list
```

#### Mode notes

A few modes that need more than one line:

- **`rotate`** replaces a secret's *value* while keeping the alias and
  its metadata. Read the new value the same ways as `add`
  (`--from-stdin`/`--from-env`/`--from-clipboard`, or an interactive
  prompt). `--revoke-caps` additionally revokes every capability
  scoped to the alias — the incident-response move when a token leaks.
  `--expiry` updates the reminder; omit it to keep the current one.
  `--ip-whitelist` updates the IP allowlist (below); a *present but
  empty* `--ip-whitelist=` clears it, omitting it keeps the current one.

- **`--ip-whitelist`** (on `add` / `rotate`) is an optional per-key
  allowlist of client source IPs or CIDR blocks (comma-separated, e.g.
  `10.0.0.0/8,203.0.113.7`; IPv4 and IPv6). It is **enforced by the
  credential broker** (`vault proxy`): a proxy request for a
  whitelisted key from an IP outside the list is denied `403`. It does
  **not** affect host-side `vault get` / `vault exec` (which have no
  client IP), and only bites once the proxy is bound off-loopback
  (`proxy --allow-public`). An empty whitelist means no restriction.
  Shown in `vault list` (the `IP-WHITELIST` column) and the TUI detail.

- **`serve`** runs the **wire protocol**: a read-only HTTP API for
  secret *provision*, shaped after HashiCorp Vault's KV v2 surface so a
  client needs only an address and a token — stock Vault client
  libraries (and the sekreto secret-provider library) work unchanged.
  `GET /v1/secret/data/<name>` (or `<ns>/<name>`) returns the secret in
  the KV v2 response shape (the value under `data.data.value`); `LIST
  /v1/secret/metadata[/<ns>]` (or `GET …?list=true`) enumerates the
  keys the token may read; `GET /v1/sys/health` reports liveness
  (`503` when locked, `501` when uninitialized); `GET
  /v1/auth/token/lookup-self` describes the presented token. Clients
  authenticate with a capability bearer token from `vault grant`,
  presented as the `X-Vault-Token` header (HashiCorp convention) or an
  `Authorization: Bearer` credential. A capability names one alias, or
  a **namespace wildcard** — `vault grant 'proj:*'` — letting one
  token read every secret in exactly one namespace. Wildcards resolve
  like names: a bare `'*'` means the active default namespace (root
  when none is set), `':*'` forces root. They work only on `serve`,
  never on the proxy or MCP broker (those resolve capabilities by
  exact alias and fail closed).
  Capability TTL, `--methods`, `--max-calls`, `--require-approval`,
  and the per-alias `--ip-whitelist` are all enforced, and each secret
  read debits the call quota. Binds loopback `127.0.0.1:8200` by
  default (`--listen`, `--allow-public`); the surface is deliberately
  read-only — provisioning hands secrets out, mutation stays on the
  authenticated CLI.

- **`lock` / `unlock`** flip a flag in the store that blocks `get` and
  `grant` (and most mutations) without destroying anything — useful
  for an admin handoff or while investigating an incident. The values
  stay encrypted in the backend; `unlock` re-enables access.

- **`config`** reads and writes vault settings. `boru vault config`
  prints them; `--set key=value` / `--unset key` change one. Keys the
  vault reads: `namespace.default` (the namespace bare aliases use),
  `journal.keep` (how many content-history generations to retain), and
  `audit.enabled` (set to `false` to stop writing the audit log).

- **`password`** manages **keyslots** — the scoped passwords that can
  open the vault. `password add --scope=<read|write|move|admin>
  --namespaces=<list> <name>` mints one (a `*` namespace = all, `:` =
  root); `assign` binds it to a user, `set` changes its value, `rm`
  removes it (`--rekey` re-encrypts the namespaces it could reach),
  and `list` shows them. This is how a CI password reads `ci:` secrets
  without being able to touch `prod:`.

  **Temporary passwords** (`--ttl`) are the way to authorize an
  already-running agent (e.g. a Claude session) without handing over the
  master passphrase: `password add --scope=read --ttl=30m --generate
  <name>` mints a **time-boxed** slot with a **randomly generated**
  password printed once. It stops authenticating the moment `--ttl`
  elapses (enforced at every `openSession`), and is never admin-scoped.
  **Re-issue** an expiring token under the **same name** with `password
  add --rotate <name>` — it mints a fresh password and resets the TTL,
  invalidating **that name's** old password, so you don't have to keep
  inventing new names (a plain `add` over an existing name is refused and
  points you at `--rotate`; `--rotate` over a name that doesn't exist yet just
  creates it). The new password must **differ** from the current one —
  re-issuing a slot with the same secret is refused (it would leave the old
  password still working); `--generate` always mints a fresh random one.
  Rotation is scoped to the one named slot: if a **different** slot happens to
  share the same password (reuse is allowed), it is a separate credential and
  is untouched — revoke it on its own name. Rotation re-issues
  *access* — it does not re-encrypt already-reachable namespaces, and a
  long-running agent that already opened a `proxy`/`mcp` session keeps its
  in-memory data keys until it restarts; rotation stops the old credential
  from opening **new** sessions, but to cut a live agent off immediately use
  `password rm --rekey <name>` (incident response).
  Revoke one early with `password rm <name>`, or pull **every** temporary
  password at once with `password rm --temp`. `list`'s `EXPIRES` column
  shows each slot's expiry (marked `(expired)` once past). The interactive
  TUI (`vault -i` → Passwords) can create a temporary password (set a TTL
  in the Add form), and revoke one (`D`) or all of them (`T`); the
  randomly-`--generate`d form is CLI-only. **Brokers**: `vault proxy` /
  `vault mcp` started under a temporary password refuse to cache the
  session and re-authenticate per request, so they stop serving the moment
  it expires (and warn at startup). See
  [Explanation → the vault](EXPLANATION.md#the-vault-why-a-local-credential-store)
  for the model.

- **`history` / `restore`** expose the append-only content journal.
  `history` lists past generations (`--limit N` for the newest few);
  `restore <generation>` rolls the *metadata* back to one of them
  (admin-scoped; `--list` enumerates restorable points, `--dry-run`
  previews). Secret values in the backend are not rewound.

- **`scan`** matches provider-token *shapes* in file contents
  (defaults to `.`). `--home` instead sweeps the well-known credential
  dotfiles (`~/.npmrc`, `~/.netrc`, `~/.aws/credentials`, `~/.pypirc`,
  `~/.gem/credentials`, …) for plaintext secrets in those tools' own
  formats; `--match-vault` flags which findings are already vaulted;
  `--quiet` prints nothing and relies on the exit code (`2` = found);
  `--max-bytes` skips large files. Env-var references like
  `${NPM_TOKEN}` and obvious placeholders are not flagged.

- **`audit`** prints the structured log. Each event records an
  `action` (e.g. `vault.add`, `vault.exec`, `proxy.request`), the
  `alias`, an `outcome` (`ok`/`error`/`denied`), and a `reason` —
  never a secret value. Filter with `--action`/`--last`, or `--json`
  for raw JSONL.

- **`mcp`** runs a stdio [Model Context Protocol](https://modelcontextprotocol.io)
  server that exposes the vault's aliases to an MCP-aware agent as
  request tools, brokering the real secret server-side exactly like
  `proxy`. `--agent=<name>` sets the identity attributed in the audit
  log and matched against capabilities (default `mcp`). It speaks
  JSON-RPC on stdin/stdout, so it is launched by the agent host, not
  run interactively.

#### Interactive TUI (`boru vault -i`)

`boru vault -i` (or `--interactive`) opens a menu-driven terminal UI for
managing the vault — no need to remember verbs or flags; the valid keys are
always shown in a footer, with `?` for full help and `:` for a command
palette. It covers vault *management* (secrets, capabilities, scoped
passwords, maintenance, config); the runtime commands `proxy`, `mcp`, and
`exec` stay on the command line.

Switching between vaults is built in: the active vault shows in the header,
and `ctrl+o` (or `:vaults`) opens a picker to switch, create, or set a
default — including vaults in custom `--folder` locations, which `init`
records in a small index (`~/.boru/vaults.jsonic`, locations only, never
secrets). `boru vault folder` prints the same list from the command line, and
`boru vault [--suffix=NAME] folder add <dir>` registers a pre-existing vault
(the suffix is auto-detected when the folder holds exactly one).

`boru vault -i --boru` (experimental) runs the TUI's **boru implementation** —
the same vault driven by a boru program (the `boru:vault-tui` module) over
the `boru:vault` bridge words, per `design/VAULT-TUI-PORT.0.md`. The
bubbletea TUI stays the default until the boru port reaches full parity.

The secret value is never taken as a command-line argument — that
would leak it into your shell history and the process listing.
`vault add` (and `vault rotate`) read it from `--from-clipboard`,
`--from-stdin`, `--from-env=VAR`, or, with none of those, an
interactive no-echo prompt. `--from-clipboard` reads the value
straight from the OS clipboard and wipes the clipboard afterwards;
it works on macOS (pbpaste/pbcopy), Linux (wl-clipboard on Wayland,
or xclip / xsel on X11), and Windows (PowerShell).

**Namespaces.** Alias names may carry one namespace qualifier,
`ns:name`, so two projects can each have an `openai_key`
(`proj1:openai_key`, `proj2:openai_key`). The namespace is part of the
alias identity — it flows through capabilities, the proxy URL
(`/proj1:openai_key/v1/...`), export bundles, and the audit log. Set a
default namespace with `vault config --set namespace.default=proj` (or
per-invocation with `BORU_VAULT_NAMESPACE`); bare names then resolve
into it for every command — `vault add key` stores `proj:key`, `vault
get key` reads it back. `:name` (leading colon) forces the root
namespace, and `:` means root anywhere a namespace is named, including
filter values and the env var. There is no silent fallback between
namespaces: a bare name that misses errors (with a hint when a
root-level twin exists). Reporting filters by namespace: `list
--namespace=proj`, `audit --namespace=proj`, `export --namespace=proj`
(`:` = root only), and `status` breaks alias counts down per
namespace. `vault exec proj:key -- cmd` injects the secret as `$key` —
the env name derives from the base name. Policy files take names
literally (no default applied), so a committed policy means the same
thing on every machine.

**Expiries.** A key can carry an optional expiry — a reminder of when
the upstream credential lapses so you can rotate it in time. Set one at
`add` time with `--expiry`, on rotation with `rotate --expiry`, or any
time with `vault expiry set <alias> <when>`; clear it with `vault
expiry clear`. `<when>` is a calendar date (`2026-12-31`), an RFC3339
timestamp, or a duration from now with day support (`90d`, `720h`,
`30d12h`). Expiry is purely informational and **never enforced** — an
expired alias still resolves, since the real key may outlive the
estimate. `vault list` shows an `EXPIRES` column, and `vault expiry`
reports the keys that have one, soonest (and most overdue) first, with
a human status (`in 90d`, `expired 3d ago`). Narrow it with
`--namespace=NS` (`:` = root) or `--within=DURATION` (keys due inside a
window, plus anything already overdue).

**Custom location.** By default the vault lives in `~/.boru` with files
named `vault.<part>` (`vault.jsonic`, `vault.keyring`, `vault.lock`,
`vault.audit.jsonl`). Two knobs override this: `--folder`/`BORU_VAULT_FOLDER`
puts the vault in a folder you choose, and `--suffix`/`BORU_VAULT_SUFFIX`
names the files `vault.<suffix>.<part>` so several vaults can share one
folder without colliding (a flag wins over the matching env var). The
flags are global — place them before the mode (`boru vault --folder=PATH
add …`) — and apply to one invocation, so pass the same folder and
suffix (or export the env vars) to every command that touches that
vault.

The store (`vault.jsonic`) and the secret keyring are separate files,
written one after the other; each write is atomic and fsync-durable
(temp file → fsync → rename → directory fsync), and concurrent writers
— the broker persisting quota counters while you run a command, two
commands at once — are serialized by an advisory lock
(`~/.boru/vault.lock`) held only around each load-modify-save, so no
update is lost across processes. A crash between the two file writes —
or a partial import — can still desync them, so `vault verify`
reconciles them: it reports dangling metadata (an alias with no
secret), orphaned keyring entries (a secret with no metadata, file
backend only), capabilities bound to a vanished alias, and stale
`.tmp` files, exiting non-zero when anything is found; `--prune`
repairs them. Verify runs under the same vault lock as the writers,
so its snapshot is consistent — a half-committed `add` can never show
up as a false orphan — and a repair can never clobber a concurrent
command. The mutating commands are ordered to fail safe (a crash
leaves a harmless orphan, never a dangling reference), so in practice
`verify` should find nothing — it's the audit that proves it.

Rename and move with `vault mv <src> <dst>`: both sides are alias
references (`vault mv key proj:key`), or both denote whole namespaces
with a trailing colon (`vault mv proj: team:`; `:` alone is root, so
`vault mv proj: :` moves everything in `proj` to root). Destinations
are pre-flighted — a bulk rename never half-commits — and the keyring
copy is verified before the old entry is deleted, so a failure can
leave a duplicate but never lose a secret. Capabilities follow the
key by default (the same bearer token then works against the new
proxy path); pass `--revoke-caps` to revoke them instead, and
`--dry-run` to preview. Legacy aliases whose namespace is only a
metadata tag are selected by namespace moves too, gaining properly
qualified names (or just losing the tag when moved to root).

Passphrases follow the same rule: every command that needs one (the
vault passphrase for the file backend, the bundle passphrase for
`export`/`import`) prompts for it interactively with echo suppressed —
no flags or environment setup needed. `BORU_VAULT_PASSPHRASE` and
`BORU_VAULT_EXPORT_PASSPHRASE` are the non-interactive overrides for
services, CI, and stdin pipelines; prefer setting them per-invocation
over `export`-ing them into an interactive shell, where they would
land in shell history and every child process's environment.

`boru vault grant` issues a scoped capability for an alias (or a
namespace wildcard like `'proj:*'`) and prints a one-time bearer
**token**; only the token's hash is stored, so save it when shown. The
credential broker (`boru vault proxy`) authenticates that token and
never accepts a prefix of it, and binds to loopback only unless you
pass `--allow-public`. The wire protocol (`boru vault serve`) accepts
the same tokens for direct secret reads under the same policy checks.
The MCP server (`boru vault mcp --agent=NAME`) is gated the same way:
it exposes and forwards only aliases the named agent has been granted
a capability for, enforcing the same TTL, host/method allowlists, and
call/cost quotas. The file backend requires a non-empty passphrase.

Two quota caveats to know when relying on `grant`'s quantitative
limits. **`--max-calls` is a soft cap under concurrency**: the check
runs at request start and the counter persists after the response, so
N simultaneous in-flight requests can overshoot the cap by up to N−1
— use it for budget hygiene, not as a hard rate limiter.
**`--max-cost-cents` is debited only from an `X-Boru-Vault-Cost-Cents`
response header**, which the built-in providers' real APIs do not
send; unless your upstream (or a middlebox you control) sets that
header, the cost meter stays at zero and the budget never trips.

**Upstream providers.** The broker forwards an alias's requests to the
base URL of its provider preset — the compiled-in ones (`openai`,
`anthropic`, `github`) or a **custom preset** minted with
`boru vault provider add --url=<base> [--auth-style=<style>] <name>`
(styles: `bearer`, `x-api-key`, `header:<name>`, `query:<name>`,
`none`). Custom presets live in the vault store, are listed by
`vault providers` with a `SOURCE` column, and are validated at mint
time — URL shape (http/https, a real hostname, a valid port, no embedded
`user:pass@`, no query or fragment), auth style (a `header:<name>` must
be a valid HTTP header name and not one net/http controls itself, such
as `Host`), and preset name; a plain-`http` base URL warns, since the
secret would travel unencrypted. Custom presets referenced by an alias
travel in `vault export` bundles, so a custom-backed alias still brokers
after `vault import`. The broker never follows an upstream redirect —
the injected secret only ever reaches the preset's own host. Built-in names can never be redefined or
removed — a store entry can never redirect a compiled-in provider — and
`provider rm` refuses while any alias still references the preset,
naming the blockers. (Flags precede the name, as with `password add`.)

**Moving a vault between machines or OSes.** A `file`-backend vault is
already portable: copy `~/.boru/vault.jsonic` and `~/.boru/vault.keyring`
and bring the passphrase. Secrets stored in an OS keychain or 1Password
do *not* live under `~/.boru`, so to move those — or to move to a
different OS — use `boru vault export`, which writes a self-describing,
passphrase-encrypted bundle (you are prompted for the bundle
passphrase; `BORU_VAULT_EXPORT_PASSPHRASE` overrides for scripts) of
the aliases and their values, independent of the source backend. `boru vault import` restores it into any backend on the target,
skipping aliases that already exist unless you pass `--overwrite`.
`import` reads a `.env` file or an export bundle, auto-detected. Both
formats are versioned: an older `boru` refuses a newer bundle rather than
mishandling it.

`boru vault exec` resolves the listed aliases against the keyring
and spawns the given command with each value injected as an
environment variable. The child inherits the caller's stdio and
exit code is propagated. The secret value only ever appears in the
child's environment block — never on the command line, never in
the audit log.

```bash
# alias `github_token` becomes $github_token in the child
boru vault exec github_token -- gh repo list

# Remap or uppercase the env names
boru vault exec github_token=GITHUB_TOKEN -- gh repo list
boru vault exec --upper github_token,openai -- ./my-script.sh

# Add a fixed prefix to every derived name
boru vault exec --prefix=APP_ --upper api_key -- ./run.sh   # → $APP_API_KEY

# Sanitize ambient env (keeps PATH/HOME/USER/SHELL/TERM/LANG/LC_ALL/TMPDIR)
boru vault exec --clear-env api_key -- ./hermetic-tool
```

### Publishing a package with a vault-held token

Most package publishers don't read an arbitrary env var — each reads its
credential from a specific variable (or, for npm, a `~/.npmrc` line). The
`--for=<tool>` recipe presents one vault secret in the exact form a
publisher expects, so the token is read from the (encrypted) vault and
exists only in the child's environment — no `~/.npmrc` edit, nothing on
the command line:

```bash
boru vault add --from-stdin npm_token            # store the token once
boru vault exec --for=npm     npm_token   -- npm publish
boru vault exec --for=pnpm    npm_token   -- pnpm publish
boru vault exec --for=cargo   crates_tok  -- cargo publish
boru vault exec --for=pypi    pypi_token  -- twine upload dist/*
boru vault exec --for=poetry  pypi_token  -- poetry publish
boru vault exec --for=github  gh_pat      -- gh release upload v1 dist/*
boru vault exec --for=hackage hackage_key -- stack upload .
boru vault exec --for=terraform tfc_token -- terraform apply

# Scoped / GitHub Packages npm registry (works for npm/pnpm/composer/terraform):
boru vault exec --for=npm --registry=npm.pkg.github.com gh_npm -- npm publish
```

`--for` is **repeatable**, and each entry may name its own secret as
`--for=<tool>=<alias>`, so one child process can carry several tools'
credentials at once — e.g. publish to npm *and* push a GitHub release tag
from a single `make publish`, each token from its own vault secret:

```bash
boru vault exec --for=npm=npm_token --for=github=gh_pat -- make publish
```

Each `--for=<tool>=<alias>` is independent: distinct tokens use distinct
aliases; one token serving two tools just repeats the alias
(`--for=github=gh_pat --for=npm=gh_pat`). The bare `--for=<tool> <alias>`
form (secret from the positional arg) still works for a single tool. If two
recipes would set the same variable to different secrets, exec stops with an
error rather than silently picking one.

Recipes are pure env injection — a secret into the exact variable(s) each
tool reads:

| `--for=` | Tool(s) | Env var(s) |
|---|---|---|
| `npm` · `yarn` · `pnpm` · `bun` | Node | per-registry `_authToken` / `YARN_NPM_AUTH_TOKEN` / `NPM_CONFIG_TOKEN` |
| `pypi`/`twine` · `uv` · `poetry` · `hatch` · `flit` | Python | `TWINE_*` / `UV_PUBLISH_TOKEN` / `POETRY_PYPI_TOKEN_PYPI` / `HATCH_INDEX_*` / `FLIT_*` |
| `cargo` · `gem` · `hex` | Rust/Ruby/Elixir | `CARGO_REGISTRY_TOKEN` / `GEM_HOST_API_KEY` / `HEX_API_KEY` |
| `hackage` | Haskell | `HACKAGE_KEY` (`stack upload` reads it directly; `cabal upload` needs `--token "$HACKAGE_KEY"`) |
| `swift` · `cocoapods`/`pod` | Swift/iOS | `SWIFTPM_REGISTRY_TOKEN` / `COCOAPODS_TRUNK_TOKEN` |
| `composer`/`packagist` | PHP | `COMPOSER_AUTH` (http-basic JSON) |
| `github`/`gh` · `gitlab`/`glab` · `terraform`/`tf` | CLI tokens | `GH_TOKEN`+`GITHUB_TOKEN` / `GITLAB_TOKEN` / `TF_TOKEN_<host>` |

The recipe sets the env-var names, so an alias under `--for` takes no
`=ENV`/`--prefix`/`--upper` remap. A tool that takes its credential
another way — a flag, stdin, or a config file — has no `--for` recipe,
but plain `vault exec` still keeps the secret in the vault; see
[Publishers without a recipe](#publishers-without-a-recipe) below.

To rehearse the plumbing without unlocking the vault, add `--dry-run`: no
passphrase is read and an obviously-fake filler value is injected in place
of the real secret. The aliases are still resolved (a typo is still an
error), so you can confirm the command and env wiring against a publisher's
own dry run:

```bash
boru vault exec --dry-run --for=npm npm_token -- npm publish --dry-run
```

#### Publishers without a recipe

Some tools don't read their credential from an environment variable, so
there's no `--for` recipe for them. The secret still belongs in the
vault — store it once and feed the injected `$tok` to whatever the tool
expects (the alias is `tok` below; `vault exec` puts its value in
`$tok`). Group by how the tool takes the credential:

**As a `--flag`** — wrap in `sh -c` so the shell expands `$tok` onto the
flag (the value can show in `ps`, usually fine in CI):

```bash
# NuGet — dotnet nuget push --api-key
boru vault exec tok -- sh -c \
  'dotnet nuget push pkg.nupkg --api-key "$tok" --source https://api.nuget.org/v3/index.json --skip-duplicate'

# JSR — deno / npx jsr publish --token
boru vault exec tok -- sh -c 'npx jsr publish --token "$tok"'
```

**On stdin** — pipe it in, so it never reaches the argument list:

```bash
# Docker — docker login --password-stdin, then push
boru vault exec tok -- sh -c \
  'printf %s "$tok" | docker login REGISTRY -u USER --password-stdin && docker push REGISTRY/IMAGE:TAG'

# Helm — helm registry login --password-stdin, then push (OCI)
boru vault exec tok -- sh -c \
  'printf %s "$tok" | helm registry login REGISTRY -u USER --password-stdin && helm push chart-0.1.0.tgz oci://REGISTRY/NS'
```

**In an environment variable the tool reads** (just not a standard name)
— remap the alias to that exact variable with `tok=ENV`:

```bash
# Gradle — ORG_GRADLE_PROJECT_<repo>Password (<repo> = the maven { name = } block, case-sensitive)
boru vault exec tok=ORG_GRADLE_PROJECT_mavenPassword -- gradle publish

# Conan 2 — CONAN_PASSWORD, with the (non-secret) username set alongside
CONAN_LOGIN_USERNAME=USER boru vault exec tok=CONAN_PASSWORD -- conan upload pkg/1.0 -r REMOTE -c
```

**In a config file that interpolates an environment variable** — point
the config at a variable, then inject it:

```bash
# Maven — settings.xml <server> with <password>${env.tok}</password>
#   (the <server> id must match the distributionManagement repository id)
boru vault exec tok -- mvn -s settings.xml deploy

# pub.dev — record that the token lives in $tok, then publish
boru vault exec tok -- sh -c 'dart pub token add https://pub.dev --env-var tok && dart pub publish --force'
```

In every case the token is read from the encrypted vault into one child
process and is gone when it exits — never written to `~/.npmrc`,
`~/.docker/config.json`, `settings.xml`, or your shell history. (Flag
and env-var spellings verified against each tool's official docs,
2026-06.)

For boru's **own** registry, `boru login --vault` stores the registry token
in the vault instead of plaintext `~/.boru/user.jsonic`, and `boru publish`
reads it back automatically — see **[publish](#boru-publish)**.

Inside boru programs the vault is accessed through the `vault`
capability — see **[Reference §Capabilities](REFERENCE.md#capabilities)**.


## Permissions

### `boru policy`

Inspect and test permission profiles. Most commands accept either a
built-in profile name (`full`, `trusted`, `client`, `read-only`,
`sandbox`, `compute`) or a path to a `.jsonic`/`.json` file.

```bash
boru policy list                              # built-in profile names
boru policy show sandbox                      # pretty-printed JSON
boru policy validate ./my-policy.jsonic       # schema + semantic check
boru policy test sandbox engine.add           # exit 0 = allowed
boru policy explain sandbox fileops.write path=/etc/passwd
# profile:  sandbox
# scope:    fileops
# op:       write
# decision: DENY
# blame:    global.disk.write (rule #1)
```

### Per-command policy flags

Every command that builds a `lang.Boru` accepts these flags:

| Flag | Effect |
|---|---|
| `--perms NAME\|PATH\|JSONIC` | Auto-detected: name, file, or inline. |
| `--perms-file PATH` | Explicit file path. |
| `--perms-inline JSONIC` | Inline jsonic (`@-` = stdin, `@PATH` = file). |
| `--allow scope.op` | Add an allow rule (repeatable). |
| `--deny scope.op` | Add a deny rule (repeatable). |
| `--allow-global OP` | Raise a global hard cap. |
| `--deny-global OP` | Lower a global hard cap. |
| `--no-install scope` | Remove a capability slot entirely. |
| `--install scope` | Force-install (overrides inherited install=false). |

Environment fallbacks (consulted when no `--perms*` flag is set):

```bash
BORU_POLICY=sandbox boru do 'add 1 2'
BORU_POLICY_FILE=./prod.jsonic boru script.boru
```

Examples:

```bash
boru do --perms=sandbox add 1 2
boru -e 'add 1 2' --perms=read-only
boru exec -p 8091 --perms=sandbox          # bound at startup; immutable per request
boru do --perms=sandbox --allow=engine.shell true
boru exec --perms=trusted --no-install=network --no-install=sqlite
```

See **[HOWTO §Sandbox untrusted code](HOWTO.md#sandbox-untrusted-code)**
for a walkthrough, and
**[design/PERMISSIONS.10.md](design/PERMISSIONS.10.md)**
for the schema.


## Supervisor control

### `boru ctl`

Drive a running `boru serve` process via its `api` service.

```bash
boru ctl status                          # list services
boru ctl info <service>                  # detail on one
boru ctl pause <service>                 # pause an instance
boru ctl resume <service>                # resume it
boru ctl stop <service>                  # stop and remove
```

Flags:

* `--api URL` — base URL of the api service. Defaults to the
  discovery file written by `boru serve`.
* `--token TOK` — bearer token. Defaults to the discovery file.


## Long-running services

These subcommands run until interrupted. They can all be composed
under one process via `boru serve`.

### `boru repl`

Start the read-eval-print loop. Same surface as plain `boru` with no
arguments — kept as an explicit subcommand for composition.

```bash
boru repl
boru repl -r ./registry
```

### `boru registry`

Serve a directory of module zips over HTTP — the simplest possible
registry.

```bash
boru registry -r ./modules -p 8080
```

* `-r PATH` — registry folder (required).
* `-p PORT` — listen port (default 8080).

### `boru lsp`

Run a Language Server Protocol server.

```bash
boru lsp                     # stdio mode (for IDE integration)
boru lsp -p 9001             # TCP mode
```

* `-p PORT` — TCP port (0 = stdio, the default).

### `boru exec`

Serve boru code execution over HTTP. POST source to `/v1/exec` and
get back the residual stack; the last value on the stack is exposed
as the top-level `result`. Each request runs in a fresh boru
instance, so requests are stateless and safe for concurrent use.

```bash
boru exec                                    # bind 127.0.0.1:8091
boru exec -p 8091                            # listen on :8091
boru exec -bind 0.0.0.0:8091 -r ./modules    # custom bind + registry
```

* `-bind HOST:PORT` — interface and port (default `127.0.0.1:8091`).
* `-p PORT` — short form; if non-zero, overrides `-bind`.
* `-r PATH` — registry folder passed to every boru instance.

Routes:

* `POST /v1/exec` — body `{"code": "..."}`; returns
  `{"result": ..., "stack": [...], "output": "...", "error": "..."}`.
  boru errors (parse / type / runtime) come back at HTTP 200 with
  `error` set, so clients can distinguish them from transport errors.
* `GET /healthz` — liveness probe.

Example:

```bash
curl -s -X POST http://127.0.0.1:8091/v1/exec \
  -H 'Content-Type: application/json' \
  -d '{"code": "add 1 2"}'
# {"result":3,"stack":[3]}
```

### `boru serve`

Run one or more services in a single process. Services are stacked
with `+` separators. Each service accepts its own flags.

```bash
boru serve repl
boru serve registry -r ./modules -p 8080
boru serve lsp + registry -r ./modules
boru serve api --bind 127.0.0.1:8090 + repl + lsp
```

The `api` service is the control plane; `boru ctl` talks to it.

### `boru tui`

Interactive terminal UI driven by an `api` service.

```bash
boru tui                            # connect via discovery file
boru tui --api http://localhost:8090 --token abc
```

Keys: ↑/↓ move, `p` pause, `r` resume, `x` stop, `q` quit.


## REPL meta-commands

Inside the REPL, lines that begin with `/` are *meta-commands*
(handled by the REPL, not the language):

| Meta-command | Effect |
|--------------|--------|
| `/help` | Print the language overview and the meta-command list |
| `/describe [name]` | Same as the `describe` word — the categorised guide, or one word / category / module |
| `/stack [n]` | Print the current stack (optionally just the top `n` entries) |

Help in the REPL mirrors the CLI, with one substitution: where
[`boru help`](#boru-help) lists the tool's subcommands, the REPL's `/help`
lists the REPL's meta-commands. Everything under
[`boru describe`](#boru-describe) — the categorised index, categories,
`boru:<module>` and `boru:<module>:<word>` — works the same at the prompt,
both as the `describe` *word* and as `/describe`:

```
>> describe                       # categorised guide to words and modules
>> describe add                   # full docs for one word
>> describe math                  # the words in one category
>> /describe boru:type-util:tpartial   # a module word (no quoting needed via /describe)
```

The `describe` and `help` *words* are ordinary boru, so an argument that
contains punctuation must be quoted: a module reference carries `:`
(`describe "boru:type-util"`), and a dotted namespace export carries `.`
— which is otherwise the `get` operator — so it too is quoted
(`describe "ArrayUtil.indices"`, after `import "boru:array-util"`). The
`/describe` meta-command takes its argument raw, so no quoting is needed
there.

Plain boru expressions work as usual; exit with Ctrl-D (EOF):

```
>> add 1 2
3
>> /stack
  [0] 3
```


## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | A user-facing error (parse, type-check, runtime, I/O) |
| `2` | Usage error (bad flag or missing argument) |

Long-running services (`repl`, `serve`, etc.) exit `0` on a clean
shutdown (`SIGINT`/`SIGTERM`) and `1` on a fatal internal error.
