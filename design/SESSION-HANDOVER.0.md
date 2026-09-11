# Session handover — the full-compilation project

**Purpose.** The one page a fresh session reads to know where the project
stands *right now*. It is deliberately short and deliberately
CURRENT-STATE-ONLY: the per-increment narrative, the measurements and the
lessons live in [FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md),
which is an append-only log and the wrong place to look for "what is true
today". Update this file at the end of every increment.

Last updated: **2026-09-11**, after increment 57 merged and increment 58 was
attempted and parked.

---

## Where the project is

| gate | value | direction |
|---|---|---|
| `frontierCompileLedger` rows | **29** | down only |
| interp-entry census rows | **29** | down only, fails in BOTH directions |
| refusal ceiling / island ceiling / type-soundness pin | 0 / 0 / 0 | pinned |
| `minCompiledRows` | 6410 | up only |
| `diagnosticParityCeiling` / `armedOnlyCeiling` | 320 / 4 | down only |
| twin-placement frontier shapes | **2** (1 and 2; 3 and 4 graduated) | — |

Increments 1–57 are merged. The most recent three PRs: #448 (increments
46–55, five miscompiles), #449 (increment 56), #450 (increment 57).

## What is in flight

Branch `claude/full-compilation-project-h5lmnt`, PR **#451**.

It carries **no graduation** — increment 58 was attempted and did not work.
What it does carry is worth keeping:

- the measurement of twin-placement shape 1, including the **negative
  result** and why the planned fix cannot work where it was placed;
- a determinism fix in `installExports` / `ensureExportsBound` (both iterated
  `desc.Exports` with a map range);
- the corrected frontier-ledger entry for that row;
- five review findings taken and one rejected with its reasons.

## Increment 58, if you pick it up

The row is `[10 20] each [drop import "boru:math-util" end MathUtil.cbrt 2]`.

**Measured, not guessed.** The bridge sees **1 BindDef twin, 0 def-site
events**. The twin is an ordinary `BindDef` — the frontier note that said
otherwise is corrected. But the event cannot simply be recorded at
`installExports`, because **that runs with the recorder suspended in both
passes**. Any fix starts from three constraints, all found by measurement or
review rather than by reading:

1. **A cached repeat import installs NOTHING.** `ensureExportsBound` guards on
   `!r.Defs.Has(name)`, and `lang/spec/edge-modules-1.tsv` pins the
   consequence (`StringUtil.$module eq StringUtil.$module` → `true`, "a cache
   no-op"). A resident op that installs unconditionally per element is a
   miscompile. This also explains why a TWO-element loop shows ONE twin.
2. **`transplantWordExtensions` is a second twin SOURCE.**
   `core.TransplantExtension` calls `NoteBindTransition` directly, never
   through `InstallDef`, so modules exporting word extensions
   (`boru:time-util`, `boru:matrix-util`, `boru:net`) make twins a
   namespace-install funnel never sees.
3. **Which suspension is it, and what are the two runs?** Both
   `installExports` calls report `recorderActive false`, and only one reaches
   a concrete `EmitState` — with `armResidentDepth 0`. Identify both before
   writing code. If the import genuinely never re-runs under recording, the
   event must be synthesized where the TWIN is noted, which is a *different*
   design from increment 53's, not the same one.

Rejected, with reasons on the PR: minting a fresh module instance per element
(it breaks the cache-no-op row above).

## Other candidates, ranked

- **Twin-placement shape 2** — a type def in a multi-run body whose expression
  READS the element. Needs an op that REBUILDS the type per element rather
  than re-installing one captured body (`typeInstallElementIndependent` is
  the screen that currently declines it).
- **The remaining 29 interp-entry census rows.** The census header in
  `test/go/langspec/interp_entry_census_test.go` carries its own seam table
  saying where they sit and which are ATTRIBUTED (specified interpretation,
  e.g. `boru:debug`) rather than debt.
- Recorded-not-done: tasks on the fn-analysis memo (NUR128 and the
  pass-scoping), NUR129/130/131's open edge, NUR134, the `codeMintPatterns`
  blind spot, the registry-spawn race.

## Process rules this line has paid for

The first is the one that cost the most, four times in one session:

1. **Re-run the gate that owns the edit — including when the edit looks
   cosmetic.** `make fmt && make vet && make lint && make test`, in order,
   all four. A refactor made *while* fixing something else is itself the
   trigger to re-run everything. Four separate red CIs traced to skipping
   this: the variation lane, `gocyclo`, the ADR-008 statement floor, and a
   stale knowledge graph on a commit that contained nothing but a design
   note.
2. **"CI green" ≠ "gated".** `ci.yml` runs `make test` + `cover-gate-core`.
   The repo-wide merged ADR-008 gate is a SEPARATE workflow
   (`cover-gate.yml`, nightly + `workflow_dispatch`). Dispatch it on the
   branch and wait for it before merging. It has two stages — stage 1 is
   `make test` under a coverage profile (where `TestVariationDifferential`
   lives), stage 2 is the 100% statement floor — and the stage tells you
   which kind of failure you have.
3. **A `what remains is…` narrowing inherits the last reader's vantage
   point.** Two frontier notes in a row pointed one layer off the real gap.
   Re-derive from the refusal, not from the sentence.
4. **A count test may be asked of the probe; a SHAPE test may not.** The
   probe carries no `producedBy`, so a const there can be an event in the
   real compile.
5. **Never run two jobs that write `kg/out` concurrently**, and never
   `git add -A` while one is running — that commits a half-written graph.
6. **A pin that stops failing has not necessarily graduated.** Check which
   claim it was making.
7. Recording a non-uniformity in `NUR.md` is mandatory and not subject to
   maintainer instruction; the **Allowed** verdict is what needs the
   maintainer. Never add to `ADR.md` unless explicitly instructed.

## Instruments

- `TestInterpEntryCensus` — the only thing that sees a compiled program whose
  BODY is interpreted. `-force-compile` reports SUCCESS for those.
- `TestVariationDifferential` — transforms each corpus row; catches what a
  new corpus row's own gates cannot. Runs in `make test` and the merged
  coverage gate, NOT in the fast per-PR checks.
- `TestMultiRunBindParityOracle`, `TestFrontierSpec*`, `TestCorpusThreeModes`.
- A temporary `println` at the decision site beats reading the code. Several
  increments' real causes were found that way and only that way.
