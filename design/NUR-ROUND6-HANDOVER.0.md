# Handover: the NUR rounds, stopped mid round 6 (2026-09-30)

This page is for whoever picks up the Non-Uniformity Register (NUR.md) work
next. It covers the state at stop, the six open records, the round-6 work in
progress (saved as patches beside this file), and how each round was run. The
project-wide state page is [SESSION-HANDOVER.0.md](SESSION-HANDOVER.0.md).
**Corrected 2026-10-05**, when this handover was integrated into the baseline
branch with the voxgig-boru handover: the patches were re-saved in a form that
builds and measured on the baseline (the table under "Round 6" says what each
one does), and the claims below that a review found wrong were fixed.
The design rules are in [COMPILABLE-SUBSET.md](COMPILABLE-SUBSET.md), and
root [CLAUDE.md](../CLAUDE.md) has the argument rule and the gates.

## State at stop

- `main` is at **64c5ab2f3**, round 5 (#526). Everything up to there is
  merged and green, including CI's 21 checks and the merged ADR-008 coverage
  gate. The development branch `claude/zealous-thompson-aigko7` is reset to
  that commit.
- Rounds so far: #520, #521, #522 (round 1), #523 (round 2), #524 (round 3),
  #525 (round 4) and #526 (round 5). Each deleted the records it fixed and
  narrowed the rest.
- Round 6 was started on 64c5ab2f3 and **stopped on request**. Nothing from it
  is merged. The partial work is in `handover/round6-wip/` (see below).
- **2026-10-05, the baseline.** This page, the voxgig-boru handover
  ([VOXGIG-BORU-HANDOVER.0.md](VOXGIG-BORU-HANDOVER.0.md), NUR366–NUR383
  and its §5 refusals) and the re-saved patches were merged onto 64c5ab2f3 on
  one branch; [SESSION-HANDOVER.0.md](SESSION-HANDOVER.0.md) opens with where
  that leaves the project.

### Gates at stop (round 5's tree, 64c5ab2f3)

The generated table `test/go/langspec/GATE_STATUS.md` (`make gate-status`)
is the authority and has twenty rows; CI renders the seventeen the langspec
shards measure into every run's summary. The values on round 5's tree:

| Gate | Live | End state |
|---|---:|---:|
| compile failures (corpus rows that fail to compile) | 1 | 0 |
| interp-entry census rows | 18 | 0 |
| engine entries (unattributed interpreter runs on the compiled path) | 159 | 0 |
| diagnostic parity divergences (plain vs compile-armed check) | 46 | 0 |
| type-soundness violations | 6 | 0 |
| armed-only diagnostics | 2 | 0 |
| reducible (tier-2) rows | 1 | 0 |
| locally-resolved defers | 1 | 0 |
| sweep call-form failures | 286 | 0 |
| sweep compile failures | 1 | 0 |
| sweep islands | 2 | 0 |
| compute gaps | 0 | 0 |
| correct-error compile failures | 0 | 0 |
| interpreter islands | 0 | 0 |
| interpreter-only rows | 0 | 0 |
| runtime defers | 0 | 0 |
| sweep call-form crashes, sweep crashes, sweep empty cells, sweep invalid seeds | 0 | 0 |

Eleven of the twenty gates are open, nine are at their end state, and there
are no regressions. Core coverage (`make cover-gate-core`) is 100%, and the
merged ADR-008 gate passes on that commit (its nightly run of 2026-10-03
failed on a cmd/go timing test, `TestServeStepShutdownDrains`, and passed
the nights before and after).

## The open records

Five of the six have a binding maintainer verdict (2026-09-29): **resolve by
fix**, meaning every remaining shape compiles and agrees with the interpreter.
NUR364 is notes only and has no verdict yet, so work on it is not
maintainer-directed. None of the six is a silent wrong answer. What remains of
NUR334, NUR336, NUR359 and NUR361 is a loud compile refusal or a loud runtime
defer; NUR356 and NUR364 are diagnostic divergences (a wrong runtime error
raised before the interpreter's no-match, and a wider operand window). NUR.md
has the full text.

| Record | What remains | Pinned by |
|---|---|---|
| NUR334 | A read after a computed keep-defs body. Remaining: the call at a fn body's tail or a tail replacement; a word before the call on its level (`def k 3 end k f …`); a lazy list read after the call; a spliced word read twice or nested (`[w]`, `(w)`), redefined, or preceded by a word that may collect it. | lang `TestNUR334CallResultEdges`, `TestNUR334SplicedWordEdges` |
| NUR336 | A paren apply over a data member. Remaining: a loop continuation with body or carried defs, a `while` loop, nested loops, a stop in a later body statement, or an effect before it; a word collecting the paren forward (`drop print (q.f 7)`, `size [(q.f 7)]`, root `drop [(m.f 7)]`); a type value before the stop (`Integer end (q.f 7) drop drop`). | lang `TestNUR336LoopContinuationEdges`, `TestNUR336LateStartEdges` |
| NUR356 | A value-raising native inside a literal handed to a call matched at run time raises before the interpreter's no-match (`each (h) [convert Integer (s)]`, `get (l) 5`). The fix owed: evaluate the literal after the match, as the interpreter does. Also a computed arm's pending literal (`if c (mk) [0]`), a designed defer. | lang `nur356_remainder_test.go` |
| NUR359 | A computed run re-stepping a fn value that **takes an argument** still defers loudly (`vm:dyn-body-plain`). Zero-argument fns are done. | lang `TestNUR359ArgTakingFnStaysLoud` |
| NUR361 | A gradual read nested in an inline body (a `var` body in an `each` callback, an arm, a loop body) that holds a **fn** defers loudly. Data values compile. It needs an island that resumes mid nested body. | lang `TestNUR361NestedGradualReadGuarded` |
| NUR364 | Notes only. A failed call over a pending literal in a fn body renders a wider operand window compiled (`the arguments were 3 and {f:(1 add 1)}` against the interpreter's `the argument was {f:(1 add 1)}`). | none |

## Round 6: the work in progress

Four agents ran in parallel, each in its own worktree on 64c5ab2f3. They were
stopped mid-task. Each one's diff against 64c5ab2f3 (committed WIP plus
uncommitted edits) is saved here as a patch. The patches first saved on
2026-09-30 applied cleanly but three of them did NOT build: they called
debug helpers (`dbg336`, `zzRaise`/`zzLog`, `dumpXProgram`) that lived in
the agents' scratch files and were stripped with them. On 2026-10-05 the
patches were re-saved with those calls removed and gofmt applied, and each
was measured on 64c5ab2f3: `go build` and `go vet` of the touched modules,
the unit suites of core/go, check/go, compiler/go and eng/go, and lang/go's
record tests (`cd lang/go && go test -run 'NUR33[46]|NUR35[69]|NUR36[14]'
.`). Still not run on any of them: lang/go's full suite, the langspec gates,
the census and coverage (A excepted, below). The brief they worked to is
`handover/round6-wip/BRIEF.md`.

| Patch | Records | State at stop (2026-09-30) | Measured on 64c5ab2f3 (2026-10-05) |
|---|---|---|---|
| `A-NUR334.patch` | NUR334 | Two shapes closed with tests: a word before the call on its level, and a read after a lazy list holding the call. Both are written into the call-result island as the values the compiled code read (`compiler/go/call_result_island.go`). New test file `lang/go/nur334_round6_test.go` (`TestNUR334CallResultReadsWritten`, with negatives). Was starting on the fn-body tail call. Still loud: the tail call and tail replacement, and the spliced-word shapes. | Builds, vet clean, the four unit suites pass once `TestSubstIsland` is updated for the new `RestartConst` arm (A had changed the contract without the test; the arm now also refuses the zero Value as the compiler's fault); the record tests and lang/go's full root suite (118 s) pass. The langspec quick subset was still running when this was written; the baseline's next commit records its result and whether A landed. |
| `B-NUR336.patch` | NUR336 | Three WIP commits: seat type values and literal parens in a unit island; late start past forward words (a parked lead in a paren collected forward); a root late start at a list or a waiting word. The "word collecting the paren forward" rows in `nur336_348_remainders_test.go` changed from loud to agreeing. The comment cites `TestNUR336ForwardWordLateStart`, which does not exist yet. Uncommitted edits in `emit.go` and `landing_restart.go` were mid-debug. Loop-continuation shapes not started. | Builds once its two `dbg336` calls are removed (`landing_restart.go`) and `core/go/emit_recorder.go` is gofmt'd. B narrowed `typeDeferred`'s signature without updating the existing `TestTypeDeferredUnknownIndex`; with that call narrowed too, vet is clean, the four unit suites pass and the record tests pass — the rows the agent reported as agreeing do. |
| `C-NUR356-364.patch` | NUR356, NUR364 | All uncommitted, no tests yet. Was wiring the interpreter's operand window into the user-poly record, the lowering and the VM (`check_recovery.go`, `check_specs.go`, `vm_poly_nomatch.go`, `eager_literal.go`). Treat it as a sketch: re-derive and test before trusting it. | Builds once the `zzRaise`/`zzLog` block is removed from `eager_literal.go` and `compiler/go/emit.go` is gofmt'd. Vet clean, the four unit suites pass, the record tests pass — which says only that it breaks nothing it has no tests of its own. |
| `D-NUR359-361.patch` | NUR359, NUR361 | One WIP commit: re-step an argument-taking fn in a computed run at a collection stop (`compiler/go/restep_tail.go`, `eng/go/vm_do_restep.go`). Uncommitted edits were mid-way through the `vm.go` integration. `TestNUR359ArgTakingFnStaysLoud` is not yet converted. NUR361 not started. | Builds once the `dumpXProgram(p)` call is removed from `eng/go/vm.go`. Vet clean, the four unit suites pass, the record tests pass — `TestNUR359ArgTakingFnStaysLoud` among them, so the shape still defers loudly: D's re-step is not yet reached. |

To resume one: `git apply design/handover/round6-wip/<patch>` on a fresh
branch off main (or off 64c5ab2f3 if main has moved), build it, run the
measurements above to confirm the starting state, read the matching rows,
and finish against the brief. The worktrees themselves
(`.claude/worktrees/agent-*`) lived in a temporary container. Do not count on
them surviving; the patches are the durable copy. Delete
`design/handover/round6-wip/` once its work is merged or dropped.

## How a round was run

1. **Split** the open records into four tracks by shared mechanism. Give each
   to an agent in its own worktree (`isolation: worktree`), based on the
   current main head, with the shared brief. The brief contains the doctrine
   (the interpreter is the oracle, and soundness is absolute), the validation
   list, the trailers, and "do not edit NUR.md or the docs: report ready-to-
   paste FIXED or narrowed text instead".
2. **Integrate**: cherry-pick each agent's commits onto the branch. Conflicts
   were mostly in `compiler/go/emit.go`; keep both sides.
3. **Docs**: delete resolved NUR records (the rationale goes in the commit
   message), narrow the rest, keep the open-list table in sync, and add any
   new compile refusals to COMPILABLE-SUBSET.md §5 as a dated "Open refusals
   recorded" block. NUR.md records answer divergences only.
4. **Gates, locally**: `make commit-gate`; `make gate-status` (no REGRESSION
   rows); `make cover-gate-core` (100%); `cd test/go && go test ./aritygate/
   ./sentinelgate/`; `make -C kg verify` (after `make -C cmd/go build`).
5. **Push and open a PR**, dispatch `cover-gate.yml` on the branch (about 30
   minutes), and wait for Codex's review. Codex reviews every PR and has found
   real P1s. Fix, reply on the thread, resolve it, and re-dispatch the
   coverage gate on the new head.
6. **Squash-merge** when all checks, the coverage gate and the review are
   clean. Then reset the branch: `git checkout -B <branch> origin/main &&
   git push --force-with-lease`.

## Things that will cost a day if re-learned

- **Disk.** The session's disk is a fixed allowance. The Go build cache grew
  to 24 GB and filled it. Four worktree creations then failed with "unable to
  write file". `go clean -cache` frees it, and builds rebuild on demand.
- **Binaries.** Build the CLI with `make -C cmd/go build`. A bare `go build
  -o` of the `cmd/go` package writes a non-executable archive (the package is
  not `main`), which breaks `make -C kg verify` with "Permission denied".
- **aritygate noise.** `.claude/worktrees/agent-ac3628638e7bad6d2` belongs to
  someone else and must not be touched. aritygate reports its files as
  unpinned, so filter `.claude/worktrees` out of the output. Genuine new
  arity sites in your own files get pinned in the table, with a note that
  they implement the argument rule.
- **The event-kind census.** Any new `switch ev.kind` in `compiler/go` must be
  registered in `event_kind_census_test.go`. Its default for an unnamed kind
  must be the sound one.
- **Round 5's lesson.** A fix at one layer can expose a latent defect in
  another. Making the interpreter answer `def x (for 3 [… break …]) end x`
  exposed a compiled `BIND_GLOBAL` underflow, because the S5 first-value bind
  was sized by the loop's static count; `EmitState.bodyEscapes` now declines
  it. After an interpreter change, re-run the same shapes on the compiled
  lane.
- **Known flake.** `TestVariationDifferential` can report "HUNG" (30 s) on
  module-sift variants when the machine is loaded. It passes alone.
- **A saved patch is not a build.** Three of the four patches first saved here
  called debug helpers that lived in the stripped scratch files, so "applies
  cleanly" was true and "builds" was not. Re-apply a saved patch in a clean
  worktree and build it before describing its state.
- **A changed contract needs its existing tests re-run.** A's new
  `RestartConst` arm and B's narrowed `typeDeferred` signature each broke an
  existing unit test in a module the agent had not re-run (eng/go,
  compiler/go). The four module suites take under a minute together.
- **Long runs.** Container restarts kill detached (`nohup`) processes, so use
  tracked background commands. A langspec coverage profile takes about 25
  minutes.
- **Workflows.** This line's token cannot push `.github/workflows/*`.
