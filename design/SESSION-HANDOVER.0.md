# Session handover — the full-compilation project

**Purpose.** The one page a fresh session reads to know where the project
stands *right now*. It is deliberately short and deliberately
CURRENT-STATE-ONLY: the per-increment narrative, the measurements and the
lessons live in [FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md),
which is an append-only log and the wrong place to look for "what is true
today". Update this file at the end of every increment.

Last updated: **2026-09-18**.

**Read in this order:** the definition of done below; then
[FULL-COMPILATION-REVIEW.0.md](FULL-COMPILATION-REVIEW.0.md) (2026-09-17,
the plan re-examined and re-staged — its §5 is the work order) and
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md) §10.1 (the same staging as
the design's own amendment); then `make gate-status` for the live numbers.
The per-increment narrative, every measurement and every lesson live in
[FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md), the
append-only log, which is the wrong place to look for what is true today.
This page is kept under 200 lines on purpose.

## Definition of done (ruled by the maintainer, 2026-09-14)

> done is a language that compiles, as a developer expects. all valid
> code compiles, no exceptions

Read as a checkable contract, this is T1 exactly as
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md) §1 states it, with **no
carve-out list**:

- Every program the interpreter accepts produces a `Program`. That is
  the whole contract, not one branch of two: `compile_failed` is not a
  result, it is a DEFECT against the contract — an unimplemented or
  unproven case, owed a fix and tracked to closure. **The
  `BORU_COMPILE_FALLBACK` hatch retired on 2026-09-19, with every other
  fallback:** the two `RunCompiled` carve-outs, the runtime-bail re-run,
  `Run`'s own fallback, the CLI's warn-and-re-run and its three compile
  modes, the fn-value seam's degrade, the detached-callback retry, and
  the `await` branch re-run. There is one outcome now, and the debt that
  was hiding behind them is counted in three ledgers
  (`test/go/langspec/compile_failures.tsv`,
  `lang/go/compile_defect_test.go`,
  `lang/go/test/compile_defect_test.go`).
- A refusal site is such a defect, and its disposition is the fix it is
  owed: exactly three are legal — a generic lowering, a trap that raises
  the interpreter's own error at the same moment, or deletion.
  "Carve-out" is not one of them, and neither is "leave it to the
  interpreter".

**Vocabulary — say what it is.** A program that does not compile has hit a
BUG. Do not call it a "refusal", in a gate name, a message, a commit, a
report or a conversation: "refuse" reads as a decision the compiler made
and was entitled to make, and it is not one. Say **compile failure**, or
bug, or defect. The counters are RATCHETS ON A BUG COUNT, never budgets
— a ceiling exists so the number cannot grow while it is being driven to
zero, and lowering one by deleting a corpus row rather than by compiling
it is the one move that is never allowed. The maintainer has had to
correct this twice; the 2026-09-18 sweep (PR #471 §11) fixed the gate
names, the user-facing message and the `compile_failed` error code, and
**248 distinct "refus" identifier forms remain in the Go tree** as a
separate mechanical sweep. Do not add more.
- "Valid" is decided by the interpreter. The checker may never be the
  reason a program fails to compile, so the whole-program "check
  diagnostics" sentinel goes (Stage 8's T1 half is inside done).
- Computed code is code: runtime-supplied bodies compile too (Stage 7 is
  inside done, and "runtime compilation must itself never refuse" is part
  of the contract).

**Framing correction (maintainer, 2026-09-16).** The interpreter is NOT a
fallback for the compiler, and it is not allowed to be one. Failure to
compile is a FAILURE — never "slow, not wrong", never an acceptable worst
case, never a co-equal branch of a two-outcome contract. The run-time path
that **silently** re-runs a refused program on the interpreter is
SCAFFOLDING that absorbs a known defect so the user still gets an answer —
and nothing in the run says the compile was refused, which makes that
failure worse, not milder: it hides itself. Where the sections
below describe that path, they describe machinery, not a sanctioned
outcome, and it never makes a refusal acceptable: every refusal recorded on
this page is a defect owed a fix and tracked to closure.

In [FULL-COMPILATION-ASSESSMENT.0.md](FULL-COMPILATION-ASSESSMENT.0.md)
§5 this settles the **T1 half** of outcome B: no compilation carve-outs,
so O4 is answered NO for compilation and "carve-out" is no longer a
disposition a refusal site can take. It does NOT choose between B and
B′, because that choice turns on T2's attributed set (question 1 below),
which the ruling does not address. Plan against B's figures as the upper
bound: Tier A plus the whole of Tier B (§6), with the generic lane (B1)
as the critical path, because it is the only mechanism that can give a
refusal site a "generic lowering" disposition in bulk.

Two questions the ruling does not settle, put to the maintainer the same
day and open until answered:

1. Whether a word whose ANSWER is the engine's own behaviour
   (`boru:debug`'s stepper and profiler, `RunTrace`) may still interpret
   at run time inside a compiled program. This is T2's attributed set,
   a runtime question, not a compilation one; the census currently treats
   those entries as specified and not debt.
2. Whether `boru check` agreeing with `boru run` (T4, the 279 rows where
   the checker reports an error the compiler compiles past) is inside
   "as a developer expects".

## Where the project is

The gates carry two numbers each (`test/go/langspec/lanes_test.go`): an
END STATE — the design's number, asserted by the direction lane
(`BORU_DIRECTION_GATES=1`, `make test-direction`, red by design until the
work is done; CI renders the lane's table from the regression shards into
every run's summary) — and a REGRESSION ceiling, the last merged value,
which only falls and which the default lane asserts.
`make gate-status` prints both for every gate, refreshes
[../test/go/langspec/GATE_STATUS.md](../test/go/langspec/GATE_STATUS.md)
and appends the instant censuses. The values on 2026-09-19, head of the S1b-2
change (2026-09-17's values, head of PR #471, in the history column;
increments 1–73, P0 and S0 are on `main`; S1a and S1b-1 on PR #474):

| gate | live | end state | what moved it |
|---|---:|---:|---|
| compile failures | 53 | 0 | 113 at the corpus expansion (+710 rows of ordinary idioms); every one a BUG in COMPILABLE-SUBSET.md §5, not a policy. Since 2026-09-18 (P0) the ceiling is the sum of `test/go/langspec/compile_failures.tsv`, one line per spec file, asserted per file under `BORU_SPEC_FILES` too. 113 → 60 on 2026-09-19 (S1a: each/fold/scan/filter declare CompileDynBody); 60 → 53 the same day (S1b-2: a computed fn value def-bound at the top level resolves at a forward slot and at a `/v` read — the collection seat and stepWordVal consult the fn-carrier side table, so the dispatch matches instead of failing at "unmatched dispatch recovered") |
| the generated sweep (S0): cells failing to compile / islanded / diverged | 36 / 2 / 3 | 0 / 0 / 0 | the sweep's first run, 2026-09-18: 53 words × the operand kinds, 305 cells, 138 passing, 115 n/a; 1932 call-form variants, 200 failing and 2 panicking. `test/go/langspec/SWEEP_STATUS.md` is the list; the divergences are NUR154, NUR156, NUR159–161, pinned. 44 / 5 → 36 / 2 on 2026-09-19 (S1a); 149 cells pass, 2086 variants with 206 failing — the six new failures are variants of cells S1a released |
| compute gaps | 49 | 0 | 107 at the expansion; three fell when NUR153 closed; 104 → 56 at S1a; 56 → 49 at S1b-2, the seven newly-compiling rows |
| interpreter islands | 0 | 0 | 12 at the expansion, all fn-VALUE callbacks; two fell when NUR153 closed; the last ten at S1a (the fn-value callbacks now lower to a poly re-match) |
| interp-entry census rows | 78 | 0 | 54 at the expansion (fn-value islands 23, raw-token code bodies 14, `boru:test` quotation bodies 8, round trips 6, repl 3); two fell when NUR153 closed; 52 → 102 at S1a — the fifty-one rows the ambiguity gate released run compiled and enter the interpreter once through the RunResolved seam (the G-lane-first landing), measured row by row against `main`; 102 → 77 at S1b-1 — the fn-value seam made native, twenty-five fn-value callback rows leave, the token-body rows stay for S3; 77 → 78 at S1b-2, ONE row entering and none leaving (callbacks.tsv:L154, `FnUtil.compose`'s wrapper — a row that FAILED TO COMPILE before, so the walk reaches it for the first time; the other six rows the increment compiles enter nothing) |
| engine entries / runtime defers | 422 / 8 | 0 / 0 | 379 at the expansion; thirteen fell when NUR153 closed; 366 → 489 at S1a, the same rows as the census (Engine.Run×489, RunResolved×179); 489 → 419 at S1b-1 (RunResolved×109); 419 → 422 at S1b-2, the three elements of that one newly-compiling wrapper row |
| known divergences (`knownDivergences`) | 5 | 0 | NUR154, NUR155, NUR156 ×3 — the ledger is pinned both ways |
| type-soundness violations | 5 | 0 | checker debt the expansion exposed |
| diagnostic parity / armed-only | 351 / 11 | 0 / 0 | checker debt the expansion exposed; both fell at S1b-2 on the seven rows it compiles — parity by seven (both passes type them alike now), armed-only by the five `boru check` called clean while compiling refused |
| `MarkUncompilable` sites / undeclared handlers | 92 / 94 | 0 / 0 | sites unchanged since 2026-08-25; handlers 114 → 94 on the migration line |
| routed dispatches / oracle reproduced | 676 / 446,999 of 473,151 | — | increments 62–65 |

The forecast and the probabilities are the review's §2.3; refresh them at
the end of each step of §5, not each increment.

## What is next

> **Read [FULL-COMPILATION-REPLAN.0.md](FULL-COMPILATION-REPLAN.0.md)
> first (2026-09-18).** It re-estimates the remainder at **75–130
> session-days** (the review said 105–175, measured two hours before the
> velocity work landed), and changes the order in four ways: **P0** per-file
> compile-failure ratchets go FIRST — `BORU_SPEC_FILES` reported counts
> instead of asserting them, which is how two regressions reached a working
> tree on 2026-09-18 while six-second filtered runs stayed green — **P0
> LANDED the same day**: `test/go/langspec/compile_failures.tsv`, one
> compile-failure count per spec file, asserted for every file a run walks,
> filtered or not (`compile_failure_ledger_test.go`). **S0 STARTED the same
> day**: the generated sweep — `test/go/sweep` (the seed table
> `seeds.tsv`, one hand-written program per declaration-relevant word ×
> operand kind), `TestGeneratedSweep` (its gates, asserted under a corpus
> filter too) and `make sweep-status` (the matrix,
> `test/go/langspec/SWEEP_STATUS.md`) — whose first run found NUR159–163:
> three miscompiles, a compiler panic, an interpreter non-uniformity. S0
> still owes the module exports as rows, signature-level cells, and the
> corpus ratchets re-based on the sweep; **S1a**,
> the gradual-Any collection overload commitment, is carved out ahead of S1
> as 19 rows on one mechanism — **S1a LANDED 2026-09-19**, in one
> session-day: each/fold/scan/filter declare `CompileDynBody`, the nineteen
> rows and thirty-one more compile (113 → 60), islands 10 → 0, at the cost
> of the interp-entry census (52 → 102) and engine entries (366 → 489),
> which is the G-lane-first landing S1b retires — **S1b STARTED the same
> day**: its first increment makes the fn-value seam native (a callback
> value runs its unit on the VM, stamped at first application and memoised
> on the value — the review's "unit half"), census 102 → 77, engine entries
> 489 → 419, no compile-failure change; its SECOND increment the same day
> resolves a computed fn value at a forward slot (the collection seat and
> the `/v` read consult the fn-carrier side table, so `each f/v xs` over a
> factory's result dispatches instead of refusing) and makes a fn-VALUE
> CLOSURE a fn value at every callback seam — matched against its own
> signature before its unit runs, which is what S1a's release had left
> unsound — compile failures 60 → 53, census 77 → 78 and engine entries
> 419 → 422 on the single wrapper row that newly compiles; what S1b still
> owes is in the handoff log's S1b entries; a THIRD increment was built,
> measured and REVERTED the same day — `set`, `push`, `unshift` and
> `append` declaring CompileStoresFn is worth six rows (53 → 47) and is
> sound in itself, but it retires a gate that was MASKING NUR169, a paren
> netting one fn value that the interpreter applies and the compiled lane
> silently does not; re-land it once that apply lowers; **S2 splits**, because only 35 of its 94
> signatures are a sweep and the other 59 need a mechanism that depends on
> S1b; and S2 is judged by `undeclaredHandlerCeiling`, never by the compile-
> failure count. The binding gate is the full unfiltered corpus at about
> twelve minutes, and it only halved — the project got about 1.4× faster,
> not fifty.

The review's §5, in order: **S0** the generated word-inventory sweep
(the corpus is a sample and under-measures by construction); **S1** fn
values as one convention (the 12 islands, 59 refusals and 23 census rows
are one family; NUR153 was ruled on 2026-09-18 — the tape rule
everywhere — and its implementation opens S1); **S2** in parallel, the
handler-migration line —
[HANDLER-MIGRATION-LINE.0.md](HANDLER-MIGRATION-LINE.0.md) is a second
session's brief and `make handler-worklist` its list; then S3–S7. The
rulings each step waits on (O2, O4, O5, the attributed set, NUR110,
NUR078) carry a recommendation each in the review's §6; NUR153 is ruled
(2026-09-18, the recommendation). What was
in flight before the review (increments 69–73's detail, the parked
increment 58, the earlier candidate list) is in the log under "Moved from
SESSION-HANDOVER.0.md (2026-09-17)".

## Process rules this line has paid for

The first is the one that cost the most, four times in one session:

1. **Re-run the gate that owns the edit — including when the edit looks
   cosmetic.** `make commit-gate` (three minutes, on what the change
   touched) before every commit. A refactor made *while* fixing something
   else is itself the trigger to re-run everything. Four separate red CIs
   traced to skipping this: the variation lane, `gocyclo`, the ADR-008
   statement floor, and a stale knowledge graph on a commit that contained
   nothing but a design note.
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
7. **A refusal message is a claim about the code, not a measurement of it.**
   Two different shapes shared one sentence for twelve increments, and a test
   pinned the generalisation as a contract with its reasoning written out —
   which is the form in which a wrong claim is hardest to see. Increment 59
   was four lines once the print was read.
8. Recording a non-uniformity in `NUR.md` is mandatory and not subject to
   maintainer instruction; the **Allowed** verdict is what needs the
   maintainer. Never add to `ADR.md` unless explicitly instructed.
9. **Work by mechanism, not by row.** A family-level increment retires ten
   to twenty rows; a row-level one retires one to three and one in seven
   is reverted. A new shape lands on the generic lane FIRST and takes a
   typed lowering later, by proof — every miscompile of 2026-09-17 was a
   bespoke typed lowering, none in shared-kernel code — and the count of
   `vm:generic-*` defer arms may only fall (FULL-COMPILATION.0.md §10.1).
10. **`make commit-gate` before a commit, `make ci-local` before a push.**
    Both are three-minute contracts: the commit gate on what the change
    touched, CI as parallel jobs each under the ceiling (`scripts/ci-steps.sh`
    is the one definition `ci.yml` and `ci-local` share). The regression
    lane is what blocks; the direction lane is the gate table CI renders
    from the shards. A regression ceiling is raised only with the row that
    moved it named in the constant's comment; an end state is never raised.
11. **Iterate on one family with `BORU_SPEC_FILES`**, and run the whole
    package (`make test-langspec SHARD=n` for every shard) before the push:
    the corpus-wide ceilings, floors and both-ways ledgers are reported,
    not asserted, under a filter. The per-file compile-failure ledger
    (`compile_failures.tsv`) is the exception: it asserts on every file the
    filtered run walks, both ways, so a change that compiles a file's rows
    lowers its line in the same change, and one that stops a row compiling
    fails the six-second run with the rows named.

## Instruments

- `TestInterpEntryCensus` — the only thing that sees a compiled program whose
  BODY is interpreted. `-force-compile` reports SUCCESS for those.
- `TestVariationDifferential` — transforms each corpus row; catches what a
  new corpus row's own gates cannot. Runs in `make test` and the merged
  coverage gate, NOT in the fast per-PR checks.
- `TestMultiRunBindParityOracle`, `TestFrontierSpec*`, `TestCorpusThreeModes`.
- A temporary `println` at the decision site beats reading the code. Several
  increments' real causes were found that way and only that way.
- `BORU_SPEC_FILES=<names or globs>` — every corpus walk in langspec and
  the interpreter oracle (`TestSpecProd`) over the named files only; the
  per-file compile-failure ledger still asserts for those files. Every
  walk runs on all cores through `specWalk` (`walk_test.go`);
  `BORU_SPEC_WORKERS=1` is the sequential, directory-ordered form for a
  temporary println.
- `make gate-status` / `BORU_DIRECTION_GATES=1` — every gate's live value
  against its end state and its regression ceiling; the ledgers
  (`knownDivergences`, `regionOracleFindings`, `diagSurfaceLedger`) are
  pinned both ways whichever lane runs.
- `make handler-worklist` — the Stage-6 list, one undeclared signature per
  line; `make cover-gate` is cached per module (`COVER_FRESH=1` to redo)
  and profiles every module before failing.
- `TestGeneratedSweep` / `make sweep-status` — the generated sweep (S0):
  every declaration-relevant word × operand kind (`test/go/sweep/seeds.tsv`)
  through both engines and every call form of `vary`'s transform table,
  in about 15 s; its word × kind matrix is `SWEEP_STATUS.md`, its counts
  are ratchets that assert under `BORU_SPEC_FILES` too, and a divergence
  is a miscompile pinned to its NUR (`sweepKnownMiscompiles`). A program
  that panics an engine or blocks past `vary.Deadline` names itself
  instead of taking the run down.
- The spawn seams (`timeout`, `interval`, the model watcher, the net
  acceptor and each connection) all run their bodies on a fork; a
  parent-minted callback stays on the fork (NUR152's `FnHome`), pinned
  under the race detector by `TestTimeoutBodyAppliesParentFnOnItsFork` and
  `TestModelWatchForkNoRace` (40/40 under load on 2026-09-17). The
  registry a unify is armed with is threaded through the kernel, not
  kept on a package-global stack (2026-09-18; the parallel corpus walks
  found the race); `TestUnifyRegistryArmedConcurrentNoRace` pins it and
  CI's race gates run it. NUR157 is the one oddity that threading kept.
