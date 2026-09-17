# design/ — design notes index

This folder holds two kinds of document:

- **Living references** — notes that track the current implementation and
  are kept correct as the code evolves. Read these as authority. They stay
  in `design/` as `.md`.
- **Legacy / historical** — point-in-time reports, completed plans, and
  accepted proposals, retained **purely for project history**. They record
  how the system got to where it is, and may describe designs, function
  names, or behaviours that no longer exist. Do **not** treat them as a
  description of the current engine. They live in **`design/legacy/`** with
  a **`.ignore`** suffix.

> **Layout (2026-09-17).** 165 legacy documents were moved to
> `design/legacy/` and renamed `.md` → `.ignore`. The suffix is the point:
> an `.ignore` file drops out of every extension-scoped search
> (`--include=*.md`, `rg --type md`, `**/*.md` globs, `git ls-files '*.md'`)
> and stops rendering as markdown, so historical material no longer
> surfaces as if it were current. The files are ordinary UTF-8 text, still
> tracked by git and still found by a plain `grep -rn`, so nothing is lost
> — it is a visibility change, not an archive.
>
> A third class exists that this split has no bucket for: **open, unbuilt
> proposals** (`MODULE-SECURITY.0.md`, `STREAM-WORDS.0.md`,
> `RESOURCE-SAFETY.0.md`, `boru-vendor.0.md`, `XML.0.md` and others). They
> are not legacy — nothing about them is finished or superseded — so they
> stayed in `design/`. They are live intent, not history.

The numeric suffix in a filename (`.10`, `.8`, `.0`, …) is a document
revision/era marker, **not** a status flag — a high number does not mean
"current," and several `.10` docs are historical. Use the classification
below, not the suffix.

This index was produced by a forward-args-focused documentation audit; the
argument-ordering / dispatch cluster is classified exhaustively, and the
wider historical material is listed so its status is unambiguous. Topic
specs not listed here (e.g. domain-word specs) were not re-audited in this
pass and carry no implied status either way.

> **Canonical source of truth for argument ordering / forward args** is the
> code-adjacent guide `eng/go/CLAUDE.md` ("Signature Ordering") and
> `lang/go/CLAUDE.md` ("Argument Ordering"), backed by the living references
> below. The single rule: each signature's `BarrierPos` marks the `|`;
> positions before it are forward-eligible (filled from forward tokens in
> source order, else the stack), positions after it are stack-only; the
> stack is consumed top-down (sig[0] = top). This is ONE rule at every
> arity — two-arg words are **not** a special case and there is no
> "swap form"; a call form only chooses where the split falls, so
> `a f b` binds a different pair than `f a b` for exactly the reason
> `c f a b` and `f c a b` bind differently at three args. Surface style: **infix** for the two-arg words
> convention reads as operators (`1 add 2`, `10 sub 3`), **forward
> form `f a b c`** for everything else (`STYLE-GUIDE.md` §S2).

## Living references — argument ordering & dispatch

- `ADR-004-REFINEMENT.0.md` — the four argument-handling categories
  (forward-eligible, mixed-barrier, stack-only, quoting slots),
  `BarrierPos` semantics and its single resolution boundary, the
  stack-only closed list and its admission test, and the composition
  rationale for the forward default. **Draft material for a refined
  ADR-004** (NUR023): its description of current behaviour is accurate
  and current, but it is *discovery*, not an ADR entry — promotion into
  `ADR.md` is a separate maintainer decision.
- `SIGNATURES.10.md` — signature shape, top-first positions, `/q` rules.
- `SIG-ORDER-REFACTOR.10.md` — the §1.4 top-first unification (end state).
- `FORWARD-COLLECTION-PHASES.10.md` — the two-phase collection model.
- `FORWARD-COLLECTION-TRAPS.0.md` — collection edge cases.
- `FORWARD-STRAND-ADVISORY.10.md` — mixed-dispatch / infix-split advisory.
- `FUNCTION-MODEL.10.md` — unified `Signature`/`FnSig`, single dispatch path.
- `USURP.10.md` — `/u` wrapper and `BarrierPos` interplay.
- `REACH.10.md` — dot-access lowering to `get`/`getr` chains.
- `ENGINE.10.md` — core engine model (argument-equivalence principle).
  *Corrected in the latest audit (removed the stale "prefix tried first,
  forward as fallback" framing; added the split-classes note).*
- `LANGREF.10.md` — language reference. *Corrected in the latest audit
  (same forward-args phrasing; anchored `sub` to `args[1] - args[0]`).*

## Legacy / historical — argument ordering & dispatch

These are banner-marked in-file; they name a multi-path dispatcher that no
longer exists (the engine unified onto one `BarrierPos`-driven rule):

- `legacy/ENGINE-UNIFIED-ALGO.8.ignore` — pre-unification *proposal*; its "current
  behaviour" review (incl. the claim that `flexibleMatch` permutes args) is
  obsolete — the shipped matcher never permutes.
- `legacy/SIGNATURE-MATCHING-PSEUDOCODE.10.ignore` — pseudocode for the removed
  prefix-vs-forward two-mode dispatcher, `MatchSignatureReversed`, and the
  `SequentialPlanner` feature flag.
- `legacy/LAZY-ARG-RESOLUTION.10.ignore` — accepted proposal; its eager-probe "current
  behaviour" sections predate the shipped structure-first model.

> Note: `legacy/PORT_OBSERVATIONS.5.ignore` §1.4 quotes the old two-category
> ("forward-collecting vs stack-only") model, but as a *resolved* problem
> statement immediately followed by the current `BarrierPos` resolution —
> it is historical narrative, not a stale live claim.

## Legacy / historical — point-in-time reports, reviews & analyses

- `legacy/BORU-CODE-REVIEW-REPORT.6.ignore`, `legacy/REVIEW-NOTES.10.ignore`,
  `legacy/FOR-LOOP-REVIEW.10.ignore`, `legacy/TYPE-SYSTEM-REVIEW.7.ignore`,
  `legacy/checker-accuracy-review.10.ignore`,
  `legacy/checker-compiler-architecture-review.0.ignore` — dated review snapshots.
- `legacy/BORU-DX-REPORT.5.ignore`, `legacy/BORU-DX-REPORT-LOG.ignore`, `legacy/BORU-DX-REPORT-DEBUG.0.ignore`,
  `legacy/VOXGIG-BORU-REPORTS.5.ignore`, `legacy/VOXGIG-DX-REPORT.5.ignore`,
  `legacy/DESIGN-DX-AND-BYTECODE-STATUS-REVIEW.ignore` — DX experience reports /
  status snapshots (the last consolidates and closes out the DX reports).
- `legacy/BATTERIES-INCLUDED-REPORT.5.ignore`, `legacy/CARRIER-STATIC-TYPECHECK-REPORT.10.ignore`,
  `legacy/STATIC_ANALYSIS_REPORT.10.ignore`, `legacy/checker-loud-diagnostics-report.10.ignore`,
  `legacy/boru-boolean-operations-report.10.ignore`,
  `legacy/boru_property_based_reduction_report.10.ignore`,
  `legacy/jsonic-matcher-rule-access-report.10.ignore`, `legacy/data-last-audit.10.ignore`,
  `legacy/WAT-AUDIT.5.ignore`, `legacy/FORMAL-FINDINGS.0.ignore` — feature/feasibility/audit
  reports pinned to a point in time.
- `legacy/TYPE-REPRESENTATION.0.ignore` — why a type NAME does not always denote its
  type. One lattice (`*Type`), but `IsTypeBody` unions 18 value shapes and
  `InstallType` has 11 branches using 3 strategies to bind a name to one.
  Two strategies work everywhere; the catch-all branch leaves the name
  usable in some positions and not others. Carries the branch table, the
  measured surface consequences, two corrections this audit had to make to
  its own earlier claims, and a staged fix. NUR090 / NUR091, issue #392.
  **Superseded 2026-08-20** by `legacy/TYPE-REPRESENTATION.1.ignore` (landed): the
  kind-split it measures no longer exists on the current tree.
- `legacy/TYPE-REPRESENTATION.1.ignore` — the type-node fusion: every named type
  evaluates to its minted lattice node; structure is recorded content
  (`TypeContentOf`), membership is Match + Unify on node Behaviors.
  **LANDED 2026-08-20 (PR #394)** — all five stages implemented; §9 is
  the implementation record, §6 the pinned semantic deltas. Resolved
  NUR090/093/094, issues #391/#392. Sections 0-8 are the design as
  proposed and measure the pre-flip tree.
- `legacy/FUNCTION-TYPES.0.ignore` — proposal + working prototype for a *declared*
  function type (`fnsig Integer String`, `fn` minus its body), answering
  the audit's "`Function` is opaque" finding. Measures what the change
  buys (a wrong-shaped argument moves from a run-time refusal to a
  check-time error) and what it does not (NUR089 is orthogonal and
  survives it). **Landed 2026-08-19** (in-file status corrected
  2026-08-21): `fnsig` shipped with the same branch that merged the
  note, enforced at check time and at dispatch; §7 lists what
  production still needs beyond it. *(§5 item 3 — the pair form
  rejecting structural named types — retired 2026-08-20 by the
  type-node fusion.)*
- `legacy/HIGHER-ORDER-FUNCTIONS.0.ignore` — an empirical audit of higher-order
  support and combinator expressibility (2026-08-19): what was built and
  run, a comparison against Haskell/Scheme/JS/Factor, and the
  call-vs-value gotchas, plus §4.2 on the six spellings of one signature
  and which of them `boru fmt` collapses. **Point-in-time**, and
  deliberately so — re-assessed twice 2026-08-21 (post type-node fusion
  and valof-flip; its §5.8 chronicles the PR #397 compilability
  campaign stage by stage), it records behaviour that NUR073, NUR078,
  NUR088, NUR089, NUR091, NUR096 and NUR097 are each
  expected to change (NUR085, NUR095, NUR073, NUR087 and NUR086 landed — §5.2 is closed by the
  `/v` totality rule; §5.4's transcripts are corrected in place
  2026-08-24: they were the compiled lane's, NUR073's class), so read
  its §4.2 and §5 against the register rather than as current truth.
  The house rules it applies live in
  [`STYLE-GUIDE.md`](../STYLE-GUIDE.md).
- `legacy/CLIENT-FIXES-2026-06-24.ignore`, `legacy/CLIENT-VERIFICATION-MAIN-2026-06-24.ignore`,
  `legacy/FORCE-COMPILE-CLIENT-COVERAGE.0.ignore` — dated client-library verification
  reports.
- `STDLIB-COVERAGE.10.md`, `legacy/IMPLEMENTATION-STATUS.10.ignore` — coverage/status
  snapshots (inherently point-in-time).
- Comparative "X-in-boru" applicability studies (idea evaluations, not
  specs): `legacy/amop-in-boru-report.0.ignore`,
  `legacy/effect-oriented-programming-in-boru-report.0.ignore`,
  `legacy/elixir-types-in-boru-report.10.ignore`, `legacy/fsharp-units-in-boru-report.0.ignore`,
  `legacy/dynamic-modality-report.10.ignore`, `legacy/LISP-ANALYSIS.5.ignore`,
  `legacy/RACKET-ANALYSIS.5.ignore`, `legacy/RACKET-FEATURES-EXAMPLES.5.ignore`,
  `legacy/PORT_OBSERVATIONS.5.ignore`,
  `legacy/rust-zig-roc-faber-in-boru-report.0.ignore`,
  `legacy/roc-in-boru-report.0.ignore` (the deep Roc pass; supersedes the
  four-language report's §4, which the 2026 Roc rewrite made stale — and
  whose verification pass recorded NUR079 and NUR080),
  `legacy/unison-in-boru-report.0.ignore` (with
  `legacy/unison-hash-identity-probe.0.ignore`, the proof-of-concept pass that
  measured its central proposal and **corrected** it — the probe is
  reproducible via `scripts/hash-identity-probe.sh`; both are
  consolidated into the design note `CONTENT-ADDRESSING.0.md`, which
  is the one to read first),
  `legacy/verse-in-boru-report.0.ignore` (with
  `verse-report-defects-investigation.0.md`, the root-cause follow-up on
  the defects that report's verification pass turned up).

- **`legacy/COMPILE-REFUSAL-SURVEY.0.ignore`** — which compile refusals are still
  reachable, measured by running them rather than by reading the refusal
  strings. Answers "would an interpreter-identical signature-matching
  opcode remove the remaining refusals?" (no: that opcode exists, and none
  of the live refusals is a dispatch-resolution problem), and records
  specimens that now compile though `COMPILABLE-SUBSET.md` §5 lists their
  class as refusing.

- **`legacy/COMPILE-DECLARATION-MODEL.0.ignore`** — the follow-on question that survey
  did not ask: is each higher-order word a special case, or is there a
  general solution? Measures the declaration surface (fifteen
  `CompileEffect` flags, one of them with zero declaration sites) against
  the 153 ledgered frontier rows, finds seven that compile and answer
  correctly but are ledgered as failures purely because they island, and
  proposes collapsing the ten operand-facing flags to three orthogonal
  per-position facts plus a declared result contract for `OpFallback`.
  A **proposal**, not a description of the engine.

## Legacy / historical — completed plans & phase docs

- `legacy/PLAN.10.ignore` (marked complete), `legacy/PERMISSIONS-PLAN.10.ignore`,
  `legacy/PBT-PLAN.10.ignore` (shipped), `legacy/MACROS-PHASE1.10.ignore` (complete),
  `legacy/MACROS-PHASE5.5.ignore`, `legacy/TCO-STAGED.10.ignore` (TCO now a language guarantee;
  see `legacy/TCO.10.ignore`).

## Legacy / historical — boru-bytecode project series

Point-in-time plan/status/deep-dive notes for the bytecode-compiler effort
(each pinned to a refusal-count/commit snapshot):

- `legacy/boru-bytecode-outline.0.ignore`, `legacy/boru-bytecode-plan.0.ignore`,
  `legacy/boru-bytecode-report.0.ignore`, `legacy/boru-bytecode-revisions.0.ignore`,
  `legacy/boru-bytecode-baseline.0.ignore`, `legacy/boru-bytecode-readiness.0.ignore`,
  `legacy/boru-bytecode-runtime-independence.0.ignore`, `legacy/boru-bytecode-completion.0.ignore`,
  `legacy/boru-bytecode-finish-line.0.ignore`, `legacy/boru-bytecode-next-stages.0.ignore`,
  `legacy/boru-bytecode-cluster5-residual-lowering.0.ignore`,
  `legacy/boru-bytecode-stage3-inlining-plan.0.ignore`,
  `legacy/boru-bytecode-stageA-branch-result-modeling.0.ignore`,
  `legacy/boru-bytecode-method-fnvalue-codebody.0.ignore`,
  `legacy/boru-bytecode-final-two-refusals.0.ignore`.

## Legacy / historical — superseded version series & handoff notes

- `legacy/module-fn-checkstate-ownership.0.ignore` … `.6.md` — superseded by `.7.md`
  (the current head of that series).
- `legacy/ACCESSOR-SPLIT-AND-CLEANUP-BUG.ignore` — session handoff / bug note.
