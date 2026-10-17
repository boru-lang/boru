# Session handover — the interpreter project

**Purpose.** The one page a fresh session reads to know where the project
stands *right now*. It is deliberately short and CURRENT-STATE-ONLY: the
per-increment narrative lives in the handover pages it points at. Update
this file at the end of every increment.

Last updated: **2026-10-17**.

## Direction

The interpreter is the one execution path. On 2026-10-16 the compiler lane
was removed — `compiler/go`, `eng/go`, their gates, tooling and design
pages — and the project's effort goes to optimising the interpreter.
[COMPILER-LANE-REMOVAL.0.md](COMPILER-LANE-REMOVAL.0.md) records the removal,
the behaviour-preservation argument and the residue left to prune; the code
as it stood is on the `compiler-legacy` branch. The previous page, written
for the full-compilation programme, is archived as
[legacy/SESSION-HANDOVER-FULL-COMPILATION.0.ignore](legacy/SESSION-HANDOVER-FULL-COMPILATION.0.ignore).

The interpreter line's plan and handover page is
[INTERPRETER-PERF-HANDOVER.0.md](INTERPRETER-PERF-HANDOVER.0.md): its
2026-10-17 section records the dispatch-cost work (a pooled parser, a
parse-free benchmark, call-free kind checks, `Lookup` across value
redefinitions, a fn value's install form, the matcher's scratch, dispatch
plans) and the levers left. The earlier tiered-execution design, which
promoted hot code to the removed VM, is retired to
[legacy/INTERPRETER-TIERED-EXECUTION.0.ignore](legacy/INTERPRETER-TIERED-EXECUTION.0.ignore).
The Value no-copy policy is
[VALUE-NO-COPY.0.md](VALUE-NO-COPY.0.md), its phases 1–4 landed through
#535 (a `Value` is a 24-byte handle on a shared `Datum`; the no-copy ratchet
`make vet-copies` is at zero in every module). Benchmarks and their
discipline: [PERF-BASELINE.10.md](PERF-BASELINE.10.md), `make bench`,
`bench/register/`.

## State of the gates

- `make commit-gate` and `make ci-local` are the definitions
  (`scripts/commit-gate.sh`, `scripts/ci-steps.sh`); CI runs the same
  steps in parallel jobs under a three-minute ceiling each.
- The langspec regression lane (`test/go/langspec`, nine shards) is green
  on the interpreter alone; the direction lane holds one gate,
  `TestCheckTypeSoundness` (`make test-direction`).
- `make cover-gate-core`, `make cover-gate-check`, `make cover-gate-parser`
  are at 100%; the merged ADR-008 gate (`make cover-gate`) runs nightly.
- Open design questions and defects are in NUR.md (pending records) and
  the open proposals under `design/` (`MODULE-SECURITY.0.md`,
  `STREAM-WORDS.0.md`, `RESOURCE-SAFETY.0.md`, …).

## Next

1. Prune the compiler lane's residue in core
   (COMPILER-LANE-REMOVAL.0.md §4).
2. The dispatch-cost plan's remaining levers (INTERPRETER-PERF-HANDOVER.0.md,
   "The dispatch-cost plan: what is left"), each measured with
   `BenchmarkInterpExec` before and after it lands (PERF-BASELINE.10.md's
   discipline).
3. The callback seam's contract (U16 in INTERPRETER-PERF-HANDOVER.0.md),
   still open.
