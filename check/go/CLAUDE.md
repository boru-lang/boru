# check/go — Type checker CLAUDE.md

The `check` module is the static-analysis pass cut out of the kernel
(design/legacy/ENG-FOUR-PIECE.0.ignore): carrier typing and check mode, the
fn-body / construction / ordering mirrors, the dispatch-recovery
braid, and the standalone diagnostic passes — dead overloads, the
pure-word dry pass, guard-predicate narrowing, static index and size
checking, the shaped-method dispatch model, store-identity shapes.
It drives the SAME dispatch machinery core drives, over carriers
instead of payloads. The kernel conventions in
[core/go/CLAUDE.md](../../core/go/CLAUDE.md) apply to this module
verbatim — the payload seal, signature ordering, no zero-value
overload, canonical `*Type` pointers, the payload-presence predicates
— and that guide is their single home.

Since the compiler lane was removed (2026-10-16,
design/COMPILER-LANE-REMOVAL.0.md) check is the top of the kernel chain
— `core -> check`, with `basic`, `lang` and `cmd` above it. Two pieces
the compiler's recorder used to supply through the S3 seam live here
now as plain analysis: the scalar constant fold (`scalar_fold.go`,
`tryFoldScalarConst`) and the code-effect analysis (`code_effect.go`,
`AnalyseCodeEffectCarrier`). The seam itself is gone.

Three rules specific to this module:

- **No upward imports.** check requires core ALONE (`go.mod` names no
  other boru sibling). Never write a basic or lang symbol here.
- **Downward into core, only through slots.** check's analysis and
  check-mode behaviors are handed to core at init — `AnalysisImpl`
  (S1, 10 slots) and `CheckBraid` (S9), both installed from
  `check_recovery.go` (`installAnalysisImpl` / `installCheckBraid`).
  Core-side tests pin each NAMED inactive default
  (`TestInactiveAnalysisImpl`, `TestInactiveCheckBraid`); a new slot
  follows the same pattern — an anonymous default replaced at init is
  unreachable and fails the merged ADR-008 gate.

- **The carrier lattice is NOT here any more** (ADR-013's 2026-08-08
  amendment; reasoning in
  [design/legacy/BASIC-CHECK-CUT.0.ignore](../../design/legacy/BASIC-CHECK-CUT.0.ignore)). The join family, the body runners, guard narrowing, the
  carrier constructors, dead-overload detection and the deduping
  diagnostic emitters moved down to `core/go` — they were pure
  functions over core types, and `basic` needs them to carry its
  control words' analysis halves without importing this module. What
  remains here is the analysis PASS: the fn / loop body models
  (memoised per call shape, recursion-bailing, quota-capped, Kleene-
  iterated), dispatch modelling and recovery, and the standalone
  diagnostic passes. `AnalyseFnBody` and `AnalyseLoopBody` are
  installed into the two newest S1 slots for exactly that reason.
  Before adding a helper here, ask whether its operands are all core
  types — if so it belongs below, not in this module.
Tests: `cd check/go && go test ./...` — a flat single package, no
sub-packages and no module Makefile. From the repo root `make test`
and `make test-race` fan out over check/go with the rest of
`MODULES` / `RACE_MODULES`. Because check's behavior is only observed
end-to-end through the pipeline above it, a semantic change wants
`cd lang/go && go test ./...` and the langspec check gates
(`TestCheckRunFalsePositive`, `TestCheckAccuracyRatchet`,
`TestCheckTypeSoundness` in test/go/langspec) too.
Coverage: `make cover-gate-check` is check/go's standalone gate (its
own suite alone, floor a ratchet toward 100), on top of the repo-wide
merged ADR-008 `make cover-gate`. The floor was RE-BASED at the
2026-08-08 carrier-lattice move: 492 statements left for core, and
because they were better covered than check's average their departure
lowered the ratio without losing a single covered statement — the
measurement universe changed, so the pre-move floor is not comparable
(same reasoning as the kernel's standalone floor at the Stage 4 cut). It ratchets
UP from the re-based value; never lower it again.
