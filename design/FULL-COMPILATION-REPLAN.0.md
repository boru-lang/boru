# Full compilation — re-planned and re-estimated after the velocity work

**Dated 2026-09-18**, measured on PR #471 head `fb094de`. Supersedes the
session-day numbers in
[FULL-COMPILATION-REVIEW.0.md](FULL-COMPILATION-REVIEW.0.md) §2.3 and the
`session-days` column of [FULL-COMPILATION.0.md](FULL-COMPILATION.0.md)
§10.1. It does **not** supersede the review's architecture findings or its
S0–S7 content; those stand. What changes is the ORDER of two steps, the
SPLIT of two more, and every number.

---

## 1. Why re-estimate at all

The review's 105–175 session-days were measured **before the work that
made a session cheaper**. The commit timestamps settle it:

| commit | when | what |
|---|---|---|
| `1cf63cf` | 2026-09-17 19:31 | the review, and its 105–175 estimate |
| `7178699` | 2026-09-17 21:41 | two lanes, the corpus filter, sharded CI |
| `a9cb212` | 2026-09-18 00:39 | the three-minute contract, parallel walks |

So the estimate was taken two hours before the first of the two velocity
commits and five hours before the second. Every number in it prices a
verification loop that no longer exists.

## 2. What actually got faster, and what did not

Measured, not remembered:

| loop | before | after | factor |
|---|---:|---:|---:|
| the pre-commit ritual (`fmt` + `vet` + `lint` + `test` + `cover-gate`) | ~60 min | `make commit-gate` **66 s** | ~50× |
| CI, end to end | 12 min 30 s | **1 min 58 s** – 2 min 51 s | ~6× |
| the whole langspec package | ~25 min sequential | **730–805 s** in one process | ~2× |
| ten gates over one spec family | minutes | **6 s** (`BORU_SPEC_FILES`) | ~20× |
| `cmd/go/internal/vault` | 293 s | 14 s | 21× |

**The binding constraint moved, and it did not move far.** The commit gate
is fifty times cheaper because `cover-gate` left the per-commit loop
entirely — it is nightly and pre-merge now, which is a scope change, not a
speedup. What a change to the recorder actually has to clear is the **full
unfiltered langspec corpus**, because that is the only thing that asserts
the regression ceilings, and it went from about 25 minutes to about
12 minutes. Two times, not fifty.

That distinction governs this whole document. The project's iteration loop
did not get two orders of magnitude faster. The part of it that was already
cheap got cheaper, and the expensive part halved.

## 3. The one change that would move the binding constraint — P0

`BORU_SPEC_FILES` restricts a corpus walk to named files and **reports**
absolute counts rather than asserting them, because a subset's count is not
comparable to a whole-corpus ceiling. The consequence is that the six-second
loop proves nothing about the ratchets, so every real change waits twelve
minutes.

It is also how this session shipped two regressions into a working tree on
2026-09-18. Declaring `CompileQuoteInert` on `set`/`del` broke
`lang/spec/as.tsv` 52–54 and moved the compile-failure ceiling 113 → 116.
Filtered runs over `as.tsv` were green throughout, because they report.
An adversarial review agent found it; the author's own testing did not.

**Fix: per-file ratchets.** One compile-failure count per spec file,
committed, asserted by filtered runs as well as whole-corpus runs. A
filtered run over `as.tsv` would then have failed in six seconds instead of
passing, and the whole-corpus ceiling becomes the sum it always was.

This is the highest-leverage remaining velocity work, because it converts
the binding gate from twelve minutes to six seconds for the majority of
iterations. It is roughly one session-day and it pays back for the rest of
the programme.

**Landed 2026-09-18**, the same day, on `main` after PR #471 merged
(`4ed08d2`). `test/go/langspec/compile_failures.tsv` is the ledger — one
line per spec file, sorted, a note column for what moved it, a file absent
meaning zero — and `TestCompiledCoverage` asserts it BOTH ways for every
file the run walked, under `BORU_SPEC_FILES` exactly as over the corpus; the
corpus-wide ceiling is its sum (`compile_failure_ledger_test.go`).
Measured on four cores: the whole-corpus gate 41 s; a filtered run over
`callbacks.tsv` 1.7 s, green against its line of 25; the incident replayed
— a failing row appended to `as.tsv`, a filtered run over `as.tsv` — **red
in 1.1 s**, naming `as.tsv:L84` and its reason. The 113 sit in six files,
every one from the corpus expansion: fold-map-filter 30, code-bodies 27,
callbacks 25, each-variants 13, fn-locals-scope 10, module-composition 8.
What the ledger does NOT yet carry is islands and the other corpus-wide
counts (the same `callbacks.tsv` run reports 5 islands and asserts none);
the mechanism is per file and generic, and S0's re-basing of every ratchet
is where extending it belongs.

## 4. The debt, re-measured

Census on `fb094de`: **8513 rows — 8054 compiled (10 islanded), 346
check-errors, 113 fail to compile.** Root causes: **80 coverage, 32
soundness, 1 correct-error, 0 scheduling, 0 opcode.** Nothing is blocked on
machinery that does not exist; every one is an unimplemented or unproven
case.

Three clusters of exactly nineteen dominate:

| rows | cluster | reading |
|---:|---|---|
| 19 | code-body word with a non-inert body | S2's code-body half |
| 19 | function-valued operand (Stage 3) | S1's core |
| 19 | `each`/`fold`/`filter`/`scan` over a gradual-Any collection: ambiguous overload, List vs Map | **its own mechanism** |
| 15 | operand provenance | S5 |
| 9 | dispatch recovery guessing | S1 / S4 |
| 7 | function value reaches word (Stage 3) | S1 |
| 25 | the long tail, 1–3 rows each | mixed |

**The third nineteen is the finding this re-plan acts on.** The review
folds those rows into "fn values", and they are not a fn-value LOWERING
problem: the callback lowers fine. The higher-order word cannot commit to
an overload because the COLLECTION operand is statically `Any`. That is a
type-commitment question one operand to the left, and it plausibly needs
neither the unit cache nor the Apply kernel that make S1 expensive.
Seventeen percent of the board, on one mechanism, probably cheap. It is
carved out as **S1a** and sequenced ahead of S1 proper.

## 5. What changes in the plan

Four changes. Everything else in the review's S0–S7 stands.

1. **P0 is new and goes first** — per-file ratchets (§3).
2. **S1a is carved out of S1** — the gradual-Any collection overload
   commitment, 19 rows, ahead of the fn-value convention.
3. **S2 splits in two, and only half of it is a sweep.** The handler
   census is 94: quoted 28, fn-operand 7, code-body 59. The first 35 are
   declaration-only work of the kind measured on 2026-09-18. The 59
   code-body signatures are **not** a sweep — they need "code-body words on
   units", which is a mechanism that depends on S1b's unit work. Treating
   them as one stage hid a dependency.
4. **S2's progress is not measured in compile failures.** A declaration
   converts a failure into a compile only where the recorder refused *for
   want of the declaration*. On 2026-09-18 the handler census moved 114 → 94
   and compile failures moved by zero. That is the expected shape of
   enabling work, and judging it by the failure count will always read as
   stagnation. `undeclaredHandlerCeiling` is its gate.

## 6. The re-estimate

Applied per step, with the speedup weighted by how gate-bound that step is.
Design-bound work barely moves; sweep and deletion work moves most.

| step | content | gate | old | **new** |
|---|---|---|---:|---:|
| **P0** | per-file compile-failure ratchets | a filtered run asserts its own subset | — | **1** |
| **S0** | the generated sweep; the coverage-matrix gate; every ratchet re-based | no empty matrix cell | 6–10 | **4–7** |
| **S1a** | gradual-Any collection overload commitment | the 19 higher-order rows compile | — | **3–6** |
| **S1b** | fn values as ONE convention; the Apply kernel; the unit cache | islands 10 → 0; fn-value failures → 0 | 15–25 | **10–17** |
| **S2a** | the 35 declaration-only handlers (quoted 28, fn-operand 7) | `undeclaredHandlerCeiling` 94 → 59 | — | **5–8** |
| **S2b** | the 59 code-body handlers, on units | ceiling 59 → 0; code-body failures → 0 | — | **10–16** |
| **S3** | runtime compilation: computed bodies, splices, module bodies | round-trip census rows → 0 | 10–20 | **8–16** |
| **S4** | the lane completed; the terminal arm flipped per family | `vm:generic-*` 10 → 0; 92 sites → trap or delete | 25–40 | **18–30** |
| **S5** | provenance generality | provenance failures 15 → 0 | 10–15 | **8–13** |
| **S6** | checker totality, T1 half | `armedOnlyCeiling` 16 → 0 | 5–10 | **4–8** |
| **S7** | every valve deleted; `CompileCheck` total | engine entries 0; `deferCeiling` gone | 8–15 | **5–10** |
| | **total remaining** | | 105–175 | **75–130** |

**The project got about 1.4× faster, not 50×.** That is the honest
reading and it is the number to plan against. A fiftyfold speedup on a loop
that was already a minute, and a twofold speedup on the loop that actually
blocks, cannot produce more than this.

**Conditional further reduction.** If P0 lands and filtered runs assert,
the four gate-bound steps (S0, S2a, S4, S7) should compress a further
15–25%, taking the range to roughly **65–115**. Not banked here, because it
has not been measured.

## 7. Calendar and probability

At five session-days a week from 2026-09-18:

| | session-days | finishes |
|---|---:|---|
| low | 75 | early January 2027 |
| high | 130 | late March 2027 |

Year end is about 75 session-days away, so **only the absolute low end with
zero overrun reaches it**. T1 + T2 by year end: **about 20%**, against the
review's 15%. It has barely moved, and it has barely moved for the reason
§2 gives — the binding gate halved, it did not collapse.

Two things would change that materially: P0 landing and delivering its
compression, and a second session running the S2a track in parallel, which
is the only stage that does not contend for `compiler/go`.

## 8. Calibration — what one session actually delivered

2026-09-18, one session, for anyone checking whether these numbers are
invented:

- handler census 114 → 94 (20 signatures: 4 dispatch-modifier value forms,
  16 `set`/`del` quoted receivers), one new `CompileEffect` flag
  (`CompileQuoteKey`), one by-name recorder key retired.
- NUR153 ruled and closed; it moved four gates — islands 12 → 10, engine
  entries 379 → 366, compute gaps 107 → 104, interp-entry census 54 → 52 —
  attribution measured across three heads rather than inferred.
- NUR158 filed from a live miscompile the declaration exposed.
- Four ratchets tightened to their live values; one rule breach repaired.
- About five full corpus runs, five commit gates, two `verify-bytecode`
  runs, one merged coverage gate, three knowledge-graph rebuilds.

Under the pre-velocity loop the same verification would have cost roughly
six hours of gating against today's two and a half, most of it backgrounded.

**And zero of the 113 compile failures were fixed.** That is the calibration
that matters: a productive day on the enabling tracks moves the headline
number not at all. The headline number moves in S1a, S1b, S2b and S4, and
nowhere else.

## 9. What would move these numbers

- **S0 will probably raise them.** The corpus is a sample; 113 is what one
  hand-written corpus found. The estimate has already gone up once after
  measurement (83–148 → 105–175) and a generated sweep is measurement.
  A rise is the instrument working, not the plan failing.
- **S1b is the overrun risk.** It is the largest design-bound block and it
  needs the unit cache, which is S3 work pulled forward. If the memoisation
  by body key and dep generation does not hold, S1b and S3 merge and the
  high end moves past 130.
- **S4's terminal-arm flip is per token class, then per family, never per
  site.** If a family turns out to need per-site work, the 18–30 is wrong.

## 10. Rules this line inherits

Unchanged from the review and the handover page, restated because they are
what keeps the numbers honest:

- `RunInterp` is the oracle, never `Run` (NUR106).
- New shapes land on the G-lane first; `vm:generic-*` arms may only fall.
- An end state never moves. A regression ceiling only falls, and only with
  the row that moved it named in its comment.
- A ceiling is a ratchet on a BUG COUNT, never a budget. Lowering one by
  deleting a corpus row rather than by compiling it is never allowed.
- A program that does not compile has hit a bug. It is not a "refusal".
