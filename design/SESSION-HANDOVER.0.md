# Session handover — the interpreter project

**Purpose.** The one page a fresh session reads to know where the project
stands *right now*. It is deliberately short and CURRENT-STATE-ONLY: the
per-increment narrative lives in the handover pages it points at. Update
this file at the end of every increment.

Last updated: **2026-10-16**.

## Direction

The interpreter is the one execution path. On 2026-10-16 the compiler lane
was removed — `compiler/go`, `eng/go`, their gates, tooling and design
pages — and the project's effort goes to optimising the interpreter.
[COMPILER-LANE-REMOVAL.0.md](COMPILER-LANE-REMOVAL.0.md) records the removal,
the behaviour-preservation argument and the residue left to prune; the code
as it stood is on the `compiler-legacy` branch. The previous page, written
for the full-compilation programme, is archived as
[legacy/SESSION-HANDOVER-FULL-COMPILATION.0.ignore](legacy/SESSION-HANDOVER-FULL-COMPILATION.0.ignore).

The interpreter line's design is
[INTERPRETER-TIERED-EXECUTION.0.md](INTERPRETER-TIERED-EXECUTION.0.md); its
handover page is
[INTERPRETER-PERF-HANDOVER.0.md](INTERPRETER-PERF-HANDOVER.0.md) (main at
6780a6030 after #452, #532, #533); the Value no-copy policy is
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
2. The interpreter tiers of INTERPRETER-TIERED-EXECUTION.0.md, measured
   against PERF-BASELINE.10.md before and after each step.
3. The callback seam's contract (U16 in INTERPRETER-PERF-HANDOVER.0.md),
   still open.
