# Performance baseline — suite, gates, and 2026-07 snapshot

This note records the repo's performance-baseline system: what the
benchmark suites cover, how to run and compare them, the deterministic
regression *gates* that run in `make test`, and the measured snapshot
taken when the system landed (2026-07, together with the sub-engine
pool optimization described at the end).

## The suites

Run everything from the repo root:

```bash
make bench                 # all suites, 1s benchtime each
make bench BENCH_TIME=5s   # longer runs for lower variance
```

Compare before/after an engine change with benchstat:

```bash
git stash && make bench > before.txt && git stash pop
make bench > after.txt
benchstat before.txt after.txt
```

| Suite | Where | Pins |
|---|---|---|
| `BenchmarkKernel*` | `core/go/kernel_bench_test.go` | kernel primitives: CompareValues (scalar/list/map), Unify, CanonValue, OrderedMap |
| `BenchmarkTape*` | `core/go/tape_bench_test.go` | gap-buffer tape vs slice splicing on deep recursion |
| `BenchmarkFlatProgramRecording` | `core/go/recorder_frame_test.go` | the StackForm recorder's cost over a flat program |
| `BenchmarkParse` | `parser/go/perf_baseline_bench_test.go` | parse cost per source shape (scalars, lists, maps, fns, parens, dotchains, interp strings, lambdas) |
| `BenchmarkInterpBaseline` | `lang/go/interp_baseline_bench_test.go` | end-to-end Run per dispatch shape (the Stage-0 corpus) |
| `BenchmarkParens` | `lang/go/paren_bench_test.go` | per-paren representation cost |
| `BenchmarkPerfWords` | `lang/go/perf_baseline_bench_test.go` | collection words (sort/filter/each/fold over 500 elems), map build/get, string words |
| `BenchmarkPerfCheck` | same file | static-check cost per dispatch shape |

## The gates (run in `make test`, no benchmarks needed)

Execution time is GC-noisy; allocations per op are deterministic, so
the hard regression signals are allocation ceilings:

- `TestInterpAllocCeilings` (`lang/go/interp_allocguard_test.go`) —
  interpreter-mode allocs/op AND **bytes/op** per shape. The byte
  ceiling catches the class alloc counts miss: ONE big allocation per
  op (the pre-pool interpreter allocated a fresh ~1024-entry tape
  per body invocation — a single alloc, ~164KB).

Lower a ceiling when an optimization reduces allocations; never raise
one without a documented reason.

## Snapshot (2026-07, 4-core Intel Xeon 2.10GHz, go1.24)

Selected rows; the full run is reproducible via `make bench`.

Kernel primitives:

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| KernelCompareScalars (5 pairs) | 791 | 0 | 0 |
| KernelCompareList64 | 11,407 | 0 | 0 |
| KernelCompareMap64 | 24,546 | 4,608 | 4 |
| KernelUnifyScalarType | 632 | 0 | 0 |
| KernelCanonNested | 5,870 | 1,632 | 29 |

Interpreter vs compiled (Stage6, execution only) — HISTORICAL: measured
before the compiler lane was removed on 2026-10-16
(COMPILER-LANE-REMOVAL.0.md); the compiled column no longer exists, and the
interpreter column is the baseline the interpreter line moves:

| Shape | interp ns/op | compiled ns/op | speedup |
|---|---|---|---|
| arith_chain64 | 779,802 | 35,291 | 22× |
| compare_loop | 4,987,989 | 236,003 | 21× |
| map_get | 8,233,648 | 235,948 | 35× |
| recursion_nontail | 21,834,523 | 458,010 | 48× |
| recursion_tail | 97,545,389 | 2,288,290 | 43× |
| do_body | 2,443,561 | 82,377 | 30× |

Word families (interpreted `Run`, includes parse):

| Shape | ns/op | B/op |
|---|---|---|
| sort_int500 | 1,730,709 | 507,221 |
| each_int500 | 11,538,654 | 3,236,111 |
| fold_int500 | 8,228,252 | 2,049,784 |
| string_split_join | 517,866 | 397,253 |

## The sub-engine pool (what moved the numbers)

Before 2026-07, every `InvokeBody` / `autoEvalList` / paren-value /
interp-hole evaluation built a fresh sub-engine whose tape starts at
`DefaultTapeInitialFloor` (1024 entries ≈ 164KB), so each element of a
higher-order word paid ~164KB and retained results pinned whole tape
buffers. `Registry.enginePool` (registry.go) now pools idle reusable
sub-engines with `reuseTape` — the same in-place `Tape.Reload` the VM's
island engine proved sound — and `runPooledSub` (invoke.go) copies
results out of the tape alias. Reentrancy is safe by construction
(nested sub-runs pop different engines); `ForkConcurrent` resets the
pool because pooled engines pin their creating registry.

Measured effect: `each [add 1]` over 800 elements dropped from 64.6ms
/ 155.8MB per run to 16.2ms / 5.0MB (4× time, 31× bytes); `fold` from
59.7ms / 153.9MB to 11.1ms / 3.1MB. Compiled `do_body` fell from
≈4,800 to ≈411 allocs/op and the branch shapes from ≈1,900 to ≈1,012.
The alloc-guard ceilings were lowered to pin the new levels.
