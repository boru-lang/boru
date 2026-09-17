# Interpreter-speed measurement fixtures

Cross-language fixtures that quantify how slow the **boru interpreter**
(`BORU_NO_COMPILE=1`) is relative to the **bytecode VM** (default) and to
other dynamic-language interpreters (CPython, Ruby, Node). They exist to
turn the "interpreted boru is orders of magnitude too slow" claim into a
reproducible number and to anchor before/after comparisons for any
interpreter optimization.

## Workloads

Three equivalent programs, one per dispatch character, each producing an
identical result in every language (checked by `run.sh`):

| Fixture | Shape | Stresses |
|---|---|---|
| `fib` | naive recursive `fib(24)` | fn-call frames + paren operands |
| `loopsum` | sum `0..99999` in a `for` loop | dispatch + arithmetic + loop machinery |
| `nestloop` | `300 × 300` nested `for` counting | nested loop / body re-evaluation |

Files: `fixtures/<name>.{boru,py,rb,js}`.

## Running

```bash
BORU=/path/to/boru bench/interp/run.sh [reps]     # default reps=3, best-of-N
```

`run.sh` reports best-of-N wall-clock (ms) per language. Wall-clock
includes process start + parse; startup is ~20 ms for `boru`, ~17 ms for
Python, so subtract it before quoting execution-only ratios (fixtures
are sized so execution dominates on the boru-interp column).

### Watch out for Ruby: its wall-clock is almost all startup

Ruby is the one column where subtracting startup is *not* optional — it
changes the conclusion. Empty-program startup here is ~14 ms for `boru`
and ~63 ms for `ruby` (RubyGems loading; `ruby --disable-gems` drops it
back to ~14 ms). These fixtures run in only ~4 ms of actual Ruby (timed
with an internal `Process.clock_gettime` best-of-5: fib 4.5 ms, loopsum
3.9 ms, nestloop 3.8 ms), so a "62 ms" Ruby wall-clock cell is ~95 %
overhead. Taken at face value the wall-clock table makes the boru
compiled VM look as fast as Ruby (55 ms vs 62 ms) — but that is a
startup artifact. On **execution only** (startup subtracted, this
container): the boru interpreter is ~320–980× slower than Ruby (worst on
recursive `fib`) and the compiled VM is ~11–46× slower. Do not read the
raw Ruby column as an execution ratio.

## Reference snapshot (2026-07, 4-core Xeon 2.80GHz)

Original baseline (before the interpreter-perf work):

| workload | boru-interp | boru-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib      | ~22,000 ms | 418 ms | 23 ms | 83 ms | 51 ms |
| loopsum  |  ~3,950 ms | 115 ms | 25 ms | 80 ms | 47 ms |
| nestloop |  ~3,330 ms | 110 ms | 22 ms | 84 ms | 48 ms |

After the six-cause fix series (`design/legacy/INTERPRETER-SPEED-PLAN.10.ignore`):

| workload | boru-interp | boru-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib      | ~9,100 ms | 380 ms | 20 ms | 68 ms | 38 ms |
| loopsum  | ~2,430 ms |  93 ms | 21 ms | 66 ms | 36 ms |
| nestloop | ~2,160 ms |  86 ms | 18 ms | 65 ms | 37 ms |

After the second-pass series (`design/legacy/INTERPRETER-PYTHON-PARITY.10.ignore` —
frame-skeleton memoization, mode-gated ID elision, scratch/buffer reuse,
trace-gating completion, Equal fast path):

| workload | boru-interp | boru-compiled | python | ruby | node |
|---|---:|---:|---:|---:|---:|
| fib      | ~5,080 ms | 232 ms | 22 ms | 79 ms | 46 ms |
| loopsum  | ~1,510 ms |  65 ms | 23 ms | 77 ms | 44 ms |
| nestloop | ~1,310 ms |  59 ms | 21 ms | 78 ms | 42 ms |

Cumulative: the interpreter is ~4.3× faster than the original baseline on
recursion and ~2.5× on loops; the interp↔compiled execution gap is
~14–23× on most shapes, and allocations per op are down 50–75 % from the
second-pass start. The compiled VM also gained (~15–20 %) from the
ID-elision and type-equality work. Analysis, per-fix notes, and the
closing assessment of what remains (and why the VM is the performance
story): `design/legacy/INTERPRETER-PYTHON-PARITY.10.ignore`.

After the `Type.Equal` interval-label fast path (2026-07, same box;
best-of-5, wall-clock incl. ~14 ms startup):

| workload | boru-interp (before → after) | Δ |
|---|---:|---:|
| fib      | 4,387 ms → 4,028 ms | −8.2 % |
| loopsum  | 1,454 ms → 1,350 ms | −7.1 % |
| nestloop | 1,219 ms → 1,147 ms | −5.9 % |

A fresh CPU profile put `Type.Equal` at the top of the interpreter flat
profile (~13 %), driven almost entirely by the per-token marker-predicate
cascade (`IsWord` / `IsOpenParen` / `IsForward` / …): each predicate did
a `v.Parent.Equal(TMarker)` that, on the overwhelmingly common *mismatch*,
fell through to a 14-char ID **string** compare. `Equal` now settles
labelled-builtin pairs with an int compare on the DFS interval label
(`In`) it already carries, so the string compare is skipped on every such
step. Pure-CPU, allocation-neutral, semantics-identical (the `In` path is
only taken when both nodes are labelled builtins, where `In`-equality is
exactly `ID`-equality). This is why the Ruby gap on these shapes is
mostly *fixed per-token dispatch overhead* a tree-walker re-pays every
step — see the top-of-file Ruby note for the startup-adjusted comparison.
