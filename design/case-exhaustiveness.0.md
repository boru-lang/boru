# Case exhaustiveness — static coverage checking for `case`

Status: landed (July 2026). Maintainer decisions: hard errors; then a
second round (same month) refining three rules — the newtype boundary
is nominal (base does not cover newtype), a dynamic scrutinee REQUIRES
a default, and predicates are meaningful (interval / is-coverage).

## The rule

`boru check` requires a `case`'s clauses to be **exhaustive** over the
scrutinee's static type. A default-less `case` whose clause matches do
not provably cover the scrutinee is an **error-severity** finding,
`case_not_exhaustive`, which gates `boru check`, refuses `boru run` at
preflight, and refuses compilation (it is deliberately NOT a
`RuntimeMirror`: no-match is not a runtime error — the runtime keeps the
spec-pinned produce-nothing behaviour — so the finding is a model-level
judgement like any other type error).

The trailing default is **not required when the type disjunctions are
met**: the clauses cover

- every alternative of a declared union (`def IS (Integer tor String)`,
  `x:IS`) — including through nested unions and a union type used
  directly in match position;
- every member of an enum (`enum`-style atom unions);
- Boolean's `true` **and** `false` (the opaque `Scalar/Boolean` leaf is
  synthetically decomposed — it is not a `true tor false` disjunction in
  the lattice);
- the `none` alternative of an optional (`Integer tor none`) via a
  `none` clause;
- a plain type via a clause naming it or an ancestor (`Integer` under
  `Number`);
- a **concrete** scrutinee via a clause that provably matches the value
  (`case 2 [1 'a' 2 'b']` is exhaustive; `case 9 [1 'a' 2 'b']` is the
  error).

## Soundness direction

Coverage is proven in the sound direction only — a clause is credited
with covering an alternative only when every runtime inhabitant of the
alternative provably matches it. Three coverage channels compose:

- **value/type matches** cover by lattice containment restricted by the
  **newtype boundary** (`caseTypeCovers`): a user-minted alternative (a
  `refine` newtype, a predicate refinement, a class) is covered only
  within its minted lineage — `Integer` does NOT cover a `Pos`
  alternative, and a `Pos` clause does not cover an `Integer`
  scrutinee. `Any` is exempt (the written catch-all). Named
  unions/enums expand through `UnionCarrierForType`; a concrete match
  covers a concrete alternative by `UnifyR` itself, so numeric-leaf
  exactness (`2.0` vs `2`) is decided identically to the runtime.
- **`[is T]` predicates** cover by the runtime is-relation (the plain
  lattice walk): `[is Pos]` covers a `Pos` alternative nominally, and
  `[is Integer]` is the explicit family check.
- **comparison predicates and DepScalar refinement matches** cover
  numeric domains by **interval union with EXACT int64 bounds** (a
  float64 representation collapses integers above 2^53 — PR #287 P2):
  `[gt 3]` and `[lte 3]` together cover Integer, `Big` (`Integer gt
  10`) plus `[lte 10]` complete the domain, an `[eq c]` point bridges
  a single gap. The Integer domain is exactly int64, so totality is
  covering [MinInt64, MaxInt64] with ℤ-adjacency. **Float and Number
  are never interval-total** — `nan` is a Float inhabitant no ordered
  comparison matches (PR #287 P1) — though a concrete non-`nan` float
  is still point-checked against the intervals (mixed int/float
  comparisons use the float64 projection, mirroring the runtime's
  cross-leaf Number comparison). A DepScalar interval is tagged with
  its refinement's BASE family and never credited outside it (`Big`
  admits no Float at runtime — PR #287 P2). Recognized shapes:
  `[gt/gte/lt/lte/eq <finite numeric literal>]` and DepScalar Lo/Hi
  bounds; non-finite bounds are rejected (a `[gt nan]` predicate is
  not an interval).

Unrecognized predicates and paren expressions stay opaque (no credit),
so "cannot prove exhaustive" findings can be conservative (they demand
a default the runtime might never need), but the checker never wrongly
*proves* a case exhaustive.

## The dynamic rule, opt-outs, and stability

- A **gradual `dynamic(T)` scrutinee REQUIRES a trailing default or an
  `Any` clause** — with no static type to prove coverage against, the
  clause list must carry its own catch-all. A code-body scrutinee
  (`case [body] […]`) computes its value and degrades to dynamic, so
  the same rule applies (emitted in caseReturnsFn's code-body branch on
  both check paths). The historical no-match/no-default
  produce-nothing behaviour is therefore no longer expressible in a
  check-clean program; the ENGINE still produces nothing when an
  unchecked run falls through, pinned by
  TestCaseRuntimeNoMatchProducesNothing (the library Run path does not
  gate on the checker) and exercised compiled-vs-interpreted by the
  differential fuzzer's no-default genCaseStmt.
- A **concrete scrutinee inside a fn body is skipped**
  (`CheckState.FnBodyDepth > 0`): `AnalyseFnBody` re-runs bodies per
  call shape with the actual argument bound (`f 9` re-analyses with
  `x=9` even for an `:Any` param), so a value-level finding there
  describes one call, not the code. Declared type domains (carriers)
  are checked everywhere; under-coverage is caught by the
  construction-time generalized analysis, and a narrowed call-shape
  domain can only shrink the uncovered set (coverage is monotone).
- Findings dedupe by **code+position** — per-call-shape re-analysis
  would otherwise re-emit with narrowed details; the first
  (generalized) finding is authoritative.
- The pass runs in `caseReturnsFn` BEFORE the plain-check /
  compile-desugar paths diverge, so both passes report identical
  findings (the same-diagnostics contract, core/go/CLAUDE.md).

## The advisory duals (info, non-gating)

The same coverage computation yields two advisories, per the
`redundant_guard` precedent ("gate on wrongness, advise on smell"):

- `case_unreachable_clause` — a clause an earlier clause fully subsumes
  (`Integer` after `Number`, `5` after `Integer`, a duplicate literal,
  `[gt 5]` after `[gt 3]` by interval containment, `[is Pos]` after
  `[is Integer]`). Deliberately clause-vs-clause only — a domain-based
  rule would be unstable under call-shape narrowing.
- `case_redundant_default` — a trailing default made dead by full
  coverage. Emitted only over a DECLARED union domain
  (`DisjunctInfo.Declared`), the one alternative set that is stable and
  author-intent-backed.

## What changed where

- `lang/go/native/case_exhaustive.go` — the coverage pass.
- `lang/go/native/conditional.go` — the `caseReturnsFn` hook (and a
  stale header fix: the compile desugar never required a trailing
  default; only the code-body-scrutinee sub-path does).
- `core/go/registry.go` — severity classifications.
- `lang/spec/case.tsv` — the no-match/no-default row was replaced by a
  dynamic-scrutinee-with-default row (no check-clean spelling of the
  produce-nothing shape exists any more); §6 pins the check-clean
  exhaustive shapes as accuracy canaries, including interval totality,
  the DepScalar complement, and the nominal newtype rows. NOTE: a
  SINGLE-clause default-less case hard-refuses compilation ("if:
  else-branch not captured" — an else-less 2-arg if is not a capturable
  branch shape), which the compile-or-fallback corpus gate reports as a
  divergence; spec rows for the nominal newtype semantics therefore use
  a two-newtype union (`(Pos tor Neg)`) so the desugared chain has a
  real else arm.
- `test/go/langspec/check_run_fp_test.go` — pin 104 → 182: every
  generated default-less `case` that runs clean is now a SANCTIONED
  check-vs-run divergence (178 of 182 divergent programs are this
  class; the other 4 are the pre-existing `if` dead-branch residue).
- `lang/go/test/case_exhaustive_check_test.go` — positive/negative
  pins for every domain shape, the opt-outs, severities, and dedupe.

## Result-typing note

`caseBranchJoin` (plain check) has always omitted a None contribution
for a missing default — optimistic against the runtime's produce-nothing
no-match. Under exhaustiveness checking that optimism is retroactively
justified: a default-less case either proves exhaustive (the no-match
path is dead) or errors. The compile desugar's 0-or-1 variadic model is
unchanged.
