# Handover: interpreter performance and the Value no-copy policy (2026-10-09)

This page is for whoever picks up the interpreter's performance work or
the Value no-copy policy. The session it hands over ran from 2026-10-08 to
2026-10-09: it merged three PRs (#452, #532, #533), filed issues #530 and
#531, and ended with a review of a new policy that changed no code. The
project-wide current-state page is SESSION-HANDOVER.0.md; this page holds
only what this session knows that the code, the PRs and the issues do not.

Everything the session left in scratch space is saved under
[handover/interpreter-perf/](handover/interpreter-perf/): the four copy
inventories, two experiment patches, eight probe corpora with their
outputs on main, and the sources of every scratch benchmark and harness.
Its README says how to use each.

## State at stop

- **Main is at `6780a6030`** (#533), and CI is green on it (run 2065).
  The PR's last run needed one re-run of a platform job that died
  downloading a module from `proxy.golang.org` before any test ran.
- **No open PR, no unpushed work.** Every local branch's commits are on
  a remote branch, there are no stashes, and the working tree was clean.
- **Open on GitHub:** issues #530 and #531, and the Codex P1 review thread
  on the merged PR #532 (decision 6 below).
- **The no-copy policy is reviewed, not started.** No code changed for it.

## What landed

| PR | squash on main | what it did | measured |
|---|---|---|---|
| #452 | `23e123516` | Preserve wire compatibility and fix portability failures (opened before this session; reviewed, updated and merged here): merged 250 commits of main into it, fixed three of four Codex findings (keychain round-trip check, Windows mmap flush, binary goldens), restored the shipped scrypt cost for the golden tests; adds CI's `lint-current` job and the native platform matrix | — |
| #532 | `571a8a77d` | Interpreter fn-call paths: `CallBoru` on a pooled sub-engine; the leaf fast path for fn values and module fns (bodies naming no binding word skip the def-table snapshots; the rule sees a binding word through an alias); a fn frame closes with its `DefCleanup` marker alone instead of a `__pa` word and an `undef` per param (NUR385 resolved); a pooled tape's reload clears only the previous program's cells and keeps the fresh-tape ceiling; a fn value's dispatch form compiles once per identity. Added `BenchmarkFnKinds`. Recorded NUR384 and U15 | twelve fn kinds −41% geomean, interpreter lane |
| #533 | `6780a6030` | Loops, container literals and tail body runs on the tape: `ForCont` became `core.Loop` with debugging annotations; natives drive loops through a `LoopDriver` whose iterations are sealed regions of the caller's tape (`each`, `for-each`, `fold`, `scan`, `outer`, `inner`, `eachrank`, `foldaxis`, `filter`'s quotation form, the map forms); pending list and map literals, members, computed keys and interpolation holes run as one-iteration regions; `core.CallRegion` returns a native's last body run as a region (`case`, the compiled lane's `__arm`); pooled call loops; a literal of scalars is its own value. Also bumped CI's `lint-current` to golangci-lint v2.14.0 and answered six Codex findings (recorder, stack view, breakpoints, ascription, void groups, pool retention) | scalar literals 20–26% faster; `each` over 100 elements 1936 → 1242 allocs/op, `fold` 1415 → 721 |

The design of the tape work is in the kernel guide, `eng/go/CLAUDE.md`,
sections "Loops on the tape (one protocol)" (drivers, call regions, the
pool, what observers see) and "Container literals are sealed regions of
the tape" (the hold rule for per-dispatch scratch state, the stepless
rule). Read the hold rule before adding any per-dispatch field to the
engine: a region stepping inside a dispatch must set it aside.

### What still runs on a sub-engine, and why

Sub-engines are now the exception on the interpreter lane. Measured after
#533, `each [mul 2] xs`, `fold`, `for-each`, `filter [gt 2]`, a map body,
a same-module fn value, a lambda literal, a closure and nested bodies all
run with zero sub-engine entries. These still use one:

- **The callback seam's contract**: a `=>` lambda over a map (`each`,
  `fold`, `scan`, `for-each`) and `filter`'s Function form run one
  `CallBoru` per element, because a `break` inside such a lambda stops at
  the lambda boundary and the return count is trimmed, and tests pin both
  (decision 7).
- **Registry boundaries**: a fn value applied across a module boundary
  runs on its defining registry through `CallBoru`.
- **Bodies that are not a native's last act**: `do` (traps the error),
  `case` predicates and the scrutinee body (the last value decides),
  `with-precision` (context teardown after the body), FnUtil and the
  macro dispatch.
- **Go modules that loop or read a body's result**: `Test.check-prop`,
  `Rand.list-of`, `StructUtil.walk`, `Tui.run`, the parse matchers,
  service, codec, sockets, logging.
- **Analysis mode** (the check pass's recorder brackets read the sub-run's
  flags), tapeless engines, and the VM's islands.

Each needs its own tape construct to move: a trap region for `do`, a
registry-switch frame for cross-module calls, and a decision on the
callback contract.

## The open thread: the Value no-copy policy

On 2026-10-09 the maintainer set this policy: *"Value structures should
never need to be copied. They can be modified in situ as needed by any
stage, but copying is forbidden."* The session was asked to review its
impact, plan the refactor and estimate the performance effect, without
changing code. The review is four documents:

- [VALUE-NO-COPY.0.md](VALUE-NO-COPY.0.md) — the review: verdict, where
  copies happen, the seven hazard classes in-situ writes would hit, the
  design the plan converges on, the six-phase plan, the decisions, the
  estimate.
- [VALUE-NO-COPY-EXCEPTIONS.0.md](VALUE-NO-COPY-EXCEPTIONS.0.md) — every
  copy allowed to remain, with its invariant and expiry phase.
- [VALUE-NO-COPY-MEASUREMENTS.0.md](VALUE-NO-COPY-MEASUREMENTS.0.md) —
  sizes, profiles, vet copy counts, the synthetic representation
  benchmark and the receiver experiment.
- The four inventories in
  [handover/interpreter-perf/inventory/](handover/interpreter-perf/inventory/).

The findings in one paragraph: struct copy and zero routines are 23–25% of
CPU on both lanes, so the copies are worth attacking; `go vet` with a
zero-size marker counts 14,887 copies in non-test code. But "modify in
situ" is only sound once per-occurrence state (position, quoted, eval,
ascription, check-mode tags, names) moves out of the shared value into
the slot that holds it, and lists and maps have value semantics today, so
in-situ writes there would change the language. A pointer representation
with a heap allocation per value measured about 2.7× slower than today on
a copy-dominated loop; only arena allocation or a smaller by-value header
wins. The plan therefore does the semantic work first and decides the
representation on phase 3's numbers.

**Where to start.** Phases 0 to 2 depend on no decision and can start now:

1. Phase 0: apply `patches/vet-nocopy-marker.patch` and add a
   `make vet-copies` ratchet with per-module ceilings from the
   measurements note §3.
2. Phase 1: apply `patches/pointer-receivers.patch` (11% geomean on
   Stage6), fix the 105 test call sites listed beside it, then move the
   step loop, the matcher and the VM run loop to pointer reads as the
   review's §6 lists.
3. Phase 2: delete the redundant copies (class E of the exceptions list).

Use the probe corpora and the langspec gates as the oracle for every
phase, and `compare.py` from the handover README for every timing. Phase
3 onward waits on decisions 1 to 5.

**Relation to the issues.** Issue #531 recorded the `Value` copy cost on
2026-10-08 and proposed "borrow for reads, shrink the header, split Value
from Type", on the premise that the header is a per-occurrence datum. The
plan keeps the first two steps (borrowing for reads is phase 1, and the
header shrinks in phase 3), leaves the third as decision 5, and replaces
the premise: the occurrence's facets move to the slot, and the value
itself becomes shared and immutable once published. Issue #530 (the native `add` call protocol)
is independent of the policy and still open.

## Decisions waiting on the maintainer

From the no-copy review (its §7 has the detail and a recommendation for
each):

1. Is constructing a new container that shares its elements a "copy"?
   The plan assumes not.
2. Lists and maps: keep value semantics (by construction, sharing
   elements) or mutate in place like flex containers?
3. Is phase 4, the pointer representation, required, or is a ~40-byte
   immutable header passed by value an acceptable end state?
4. Approve or amend the sanctioned exceptions list.
5. Keep `type Type = Value` with a node-reference literal, or split
   `Type` from `Value`.

Carried from the merged PRs:

6. **The leaf rule's call-time hole** (Codex P1 on #532, thread still
   open). A binding word handed to a leaf body through a `Function`
   parameter leaks its binding past the call on every interpreter path,
   and has since the speed plan. Measured on main:
   `def h fn [[b:Function n:Integer][Integer][b y 2 n]]  h def/v 1  y`
   answers `[1 2]` interpreted, while the compiled lane's check pass
   refuses it with "undefined word: y". Recorded as U16 in
   COMPILABLE-SUBSET.md §5, the ledger the register's scope ruling assigns
   a refusal to. Options: a run-time rule (a snapshot taken
   lazily at the first install inside a leaf frame, touching the fn
   baseline, the bind twin ledger and the eager tail-call teardown); accept
   it as a documented property of the leaf fast path; or treat
   `Function`-typed params as frame state, which slows higher-order user
   fns on the named path. The probe shapes are in the handover README.
7. **The callback seam's contract.** Moving the `=>` lambda forms over
   maps and `filter`'s Function form onto the tape means deciding whether
   a `break` inside the lambda still stops at the lambda boundary and the
   return count is still trimmed.

## Follow-ups found and not done

- **Issue #530's fixes**: a pointer-identity fast path in `numLeaf`, the
  `screenResults` label built only on the error path, `stampFnResultPos`
  skipped when the declared return cannot be a Function, a single-value
  return for natives; and, longer term, intrinsic lowering of strict
  scalar `add`, `lt`, `eq` (about 7× on the add itself).
- **Re-installing a `Function` parameter on every call.** Binding a fn
  value under a parameter name goes through `installFnDef`, which
  recompiles the callback's signatures each time; it was 13% of a
  user-defined fold loop. Cacheable on the function's identity the way
  `compiledFnDefFor` caches the dispatch form.
- **User-defined higher-order words are ~5× slower than the natives**
  (`mysum inc/v xs 0 0`, 45–51 ms, against 6–9 ms for the native words
  over 500 elements). About half is forward paren-group evaluation and
  dispatch matching (#530, #531 territory); a boru prelude for the
  collection words waits on that.
- **Inlining user fns into compiled units** (assessed, not built): sound
  under the existing memo if recursive, `args`-reading, dyn-scope,
  fit-island and module-gated bodies are excluded; ceiling about 45% on
  call-heavy code and 5–10% on the real suites.
- **Found by the no-copy inventories, each to reproduce before acting**:
  - `check/go/check_fnbody.go:646` calls `CloneValue` "for a fresh ID",
    but `CloneValue` returns a Function unchanged, so the carrier shares
    the residual's ID.
  - Pooled engines do not clear `resolvedScratch`, `loopTokens`,
    `peScratch` or `excludeScratch` on release
    (`core/go/engine_pool.go:642-670`), so they pin values across reuses.
  - `resolveAtomReferents` (`core/go/engine.go:1616-1633`) recurses into
    nested lists of the program it was given and stamps their atoms in
    place through the shared element arrays, so a program run twice may
    see the first run's referents.
  - `ResolveWordsDeep` rebuilds both `Unify` operands on every call, even
    when nothing resolves (`core/go/resolve.go:75-155`).
- **The committed gate table is stale.** `test/go/langspec/GATE_STATUS.md`
  was last refreshed by #529; #532 and #533 moved the interpreter-entry
  census (a literal is no longer an entry). `make gate-status`
  regenerates it.

## Answers given during the session

Kept here because they exist nowhere else.

- **Are defs immutable?** No. A def is a per-name shadow stack (`def x 1
  def x 2 x` is 2, `undef x` gives 1 back); a second `def f fn` with the
  same signature replaces, a different one stacks an overload; closures
  capture params and fn-locals but read module scope live (NUR097); the
  compiler honours live rebinding through per-call-site memo keys and
  dependency snapshots, and the design rejected world ages.
- **What does a native `add` cost, compiled?** About 550 ns and 1.5
  allocations: a quarter is `numLeaf`'s three `ConformsTo` chains per
  operand, a fifth the result slice, the rest the VM's protocol and
  `Value` copies. Filed as #530 with the per-piece table.
- **Why classify operands the checker already proved?** One overload,
  `[Number Number]`, covers the numeric tower and the handler picks the
  leaf; the handler is lane-agnostic, so the checker's proof never reaches
  it; and refinements keep their own tag, so pointer identity alone would
  misclassify `def Pos (refine Integer)`.
- **Why not pass `Value` by reference?** The answer on 2026-10-08 was
  that the header carries per-occurrence facets set on copies; filed as
  #531. The policy of 2026-10-09 reverses the direction, and the review
  reconciles the two by moving the facets to the slot.
- **Can a user-written `myeach` inline its callback?** Yes, it always
  could: a user fn returns tokens that splice onto the tape. A Go word
  that runs its body inside its handler could not, which is what the
  `LoopDriver` protocol (#533) fixed for the collection words.
- **Sub-engines versus handlers on the tape.** Three options were
  weighed: the `for` model generalised (chosen and built), handlers that
  edit the tape (which converge on the same re-entry marker), and a boru
  prelude (right eventually, a 5× regression today).

## Things that will cost a day if re-learned

- **Run gates alone.** On the 4-core box the commit gate ran 380 s
  against its 180 s ceiling, and the `lang` root suite 253 s against
  150–180 s, when suites ran beside it. `TestVariationDifferential`'s
  30-second per-variant budget timed out on `for 2 [import "boru:sift" …]`
  under five concurrent gate runs; alone it finishes in under 2 s
  (`probes/sift-differential.txt`). A timeout under load is not a hang.
- **The commit gate's lint stage** sometimes fails with "parallel
  golangci-lint is running"; the gate retries each module alone, and a
  standalone `make lint` is the check.
- **The commit gate does not run every CI lane.** CI's arity-keyed-site
  gate, which the commit gate does not run, flagged two `len(...)`
  comparisons on a cache key, and CI's `cover-gate-core` found a
  statement the local runs had not covered. Run `make ci-local` and
  `make cover-gate-core` before pushing a change to the kernel.
- **CI's `lint-current` job tracks `go-version: stable`.** Go 1.27.2
  (2026-10-09) writes export data v5, which golangci-lint v2.13.2's
  x/tools could not read; v2.14.0 fixed it. The next Go release can break
  it the same way; the fix is a newer linter, not the diff.
- **A platform job dying in "Build and start the CLI"** on a
  `proxy.golang.org` stream error is infrastructure; one re-run is the
  remedy.
- **A merge conflict stops CI entirely.** After main moved, PR #532's
  branch conflicted in `kg/out/graph.*` and GitHub created no CI runs for
  its pushes until main was merged in and `make -C kg graph` regenerated
  the bundle.
- **GitHub access can drop mid-session** (`git push` fails with "could
  not read Username for https://github.com"; the GitHub tools say the
  account is not connected). Reconnecting at claude.ai/connect-github
  restores both; reload the GitHub tool schemas afterwards. The session's
  git proxy also refuses branch deletion.
- **`pkill -f 'go test'` kills the calling shell**, whose own command line
  matches the pattern.
- **Tape-ceiling warnings in test output are not a regression**; main
  prints them too. Compare against a stash that includes untracked files
  (`git stash -u`), or a new file is silently left in the "before" tree.
- **A pooled object's release must follow the splice of its results.**
  Clearing a pooled Loop's results inside `Finish` zeroed the very slice
  being spliced ("undefined stack entry at position N").

## Leftovers

Remote branches whose content is on main, so deletable at will:
`perf/interp-fn-call-paths` (#532), `perf/sealed-literals` (#533) and
`fix/preserve-wire-format-compatibility` (#452), each with a tip whose tree
is identical to its squash commit's, and `perf/tape-loops`, an ancestor of
`perf/sealed-literals`. `wf-push-probe` holds
one probe commit adding two lines to one file, from testing push
permission; it was never meant to merge. The session's git proxy refused
the deletion when it was tried on `wf-push-probe`.
