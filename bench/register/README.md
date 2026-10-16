# The performance register

A permanent, host-keyed record of how fast this system is, and how that
changes as the implementation does. Specified by
[`design/FULL-COMPILATION.0.md`](../../design/FULL-COMPILATION.0.md) §14.

The measurement discipline this repository already had was good at
*relative* answers (`benchstat` before/after, the alloc-ceiling gates in
`make test`) and bad at *longitudinal* ones: snapshots lived as prose in
design notes, and `bench/networking/README.md` carries both warnings this
register exists to fix — "absolute req/s track the box", and a superseded
row that had silently measured the wrong lane. A staged compiler rebuild
needs a record that outlives the note that produced it.

## The two files

Both are committed and **append-only**.

- **`hosts.jsonl`** — one row per distinct machine. `host` is the id:
  `h:` plus the decimal FNV-1a-64 of a canonical tuple, using the same
  hash the knowledge graph uses for identity, so there is one hash in
  this repository rather than two. The tuple is recorded in each row as
  `id_fields`, so changing it later is detectable rather than silently
  re-keying every host. Memory is rounded to whole GiB *inside the hash*
  because `MemTotal` drifts by a few KB across kernels and containers on
  one physical box; the exact `mem_kb` still rides in the row, alongside
  CPU model, core and thread counts, arch, OS name and version, kernel,
  virtualization class and CPU governor.
- **`measurements.jsonl`** — one row per measurement:
  `{ts, commit, host, surface, workload, metric, value, unit, n,
  benchtime, go, os_version}`. `go` and `os_version` ride on *every* row
  because a host drifts under a stable id — toolchains and patch levels
  move while the hardware does not.

`surface` is one of **`check`** (the static pass), **`compile`** (emit +
lower), **`interp`** (interpreted execution), **`exec`** (compiled
execution), **`parse`**, and **`e2e`** (the server workloads: req/s,
µs/round-trip). All the implementation surfaces are first-class, so a
change that buys execution speed by spending check time is *visible*
rather than invisible.

## Rules

**It records; it never gates.** Execution time is too noisy to fail CI
on. The deterministic alloc ceilings in `make test` remain the only
performance *gates*; this is the memory.

**Rows are never edited or deleted.** A measurement discovered to be
wrong is *superseded by a new row naming it* — the same discipline the
frontier ledger uses. `test/go/register` gates the schema and checks that
every measurement resolves to a host on file; it deliberately asserts
nothing about values.

**Absolutes compare only within one host id.** Across hosts, only ratios
travel — compiled/interpreted, boru/Go, check-cost vs exec-cost. Rows
record the absolutes; reports derive the ratios.

## Capturing

```bash
cd cmd/go && make build          # the harness needs the boru binary
make bench-register              # or: BORU=cmd/go/bin/boru bench/register/run.sh 1s
```

`BENCH_TIME` tunes `-benchtime` (default `1s`).
`bench/register/hostid.sh` derives the fingerprint and prints the
`hosts.jsonl` row on its own if you want to inspect it. Set
`REGISTER_LABEL` to name the machine.

### CPU-profile share

```bash
bench/register/cpushare.sh [benchtime] [repeats]   # defaults: 3s, 3
```

A second instrument, for the question wall clock cannot answer on a
shared box: *what fraction of interpreter CPU do the collection loops
cost?* Profile share is comparative **within a single run**, so the
machine's noise largely divides out of it — which is why
`design/FULL-COMPILATION.0.md` §11 re-specified falsifier F1b's gate onto
it after two runs of byte-identical code disagreed by 5.4% geomean on
wall clock.

It appends `cum_pct_total` (share of all samples) and `cum_pct_interp`
(share of `Engine.Run`) rows under `surface: interp`, workload
`cpushare/<anchor>`, each carrying `spread` across the repeats. **Always
read a share against its `spread`** — the whole point of the instrument
is that a 0.2-point move under a 2-point spread is not a move.

Two things to know before trusting a number out of it:

- It profiles the **dispatch-dense** interpreter benchmarks
  (`BenchmarkParens`, `BenchmarkInterpBaseline`), not the
  collection-*word* suite. In `BenchmarkPerfWords` the 500-element inner
  work swamps dispatch and the same loops sit near 3–5% of samples, where
  a real regression hides inside the spread. Override with
  `CPUSHARE_BENCH` if you know why you want to.
- The anchors are the three loops' **callers**, which is what lets one
  profile be compared to another across a refactor that renamed or
  extracted the callees. Anything the callees cost shows up in them.
  `collectArrival` has no samples in any benchmark here at all, so
  nothing in this repository measures the arrival path — treat that as a
  known blind spot, not as a pass.

Every stage of the full-compilation plan lands with a before/after pair
on at least one host: the pair is part of the stage's deliverable.

## Reading it

The seed rows (2026-08-25, commit `8f13e1b`) were taken at
`benchtime=200ms` on a shared 4-core container while other work was
running — deliberately recorded that way rather than discarded, since
`benchtime`, `host` and `commit` are on every row and a reader can filter
on them. Treat them as a format shakedown, not a performance claim; the
first quiet full-length run supersedes them by addition.
