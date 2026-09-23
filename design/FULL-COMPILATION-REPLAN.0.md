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

| step | content | gate | old | **new** | **2026-09-19, after S1a** |
|---|---|---|---:|---:|---:|
| **P0** | per-file compile-failure ratchets | a filtered run asserts its own subset | — | **1** | done (1) |
| **S0** | the generated sweep; the coverage-matrix gate; every ratchet re-based | no empty matrix cell | 6–10 | **4–7** | 2–4 left (1 spent) |
| **S1a** | gradual-Any collection overload commitment | the 19 higher-order rows compile | — | **3–6** | done (1) |
| **S1b** | fn values as ONE convention; the Apply kernel; the unit cache | islands 10 → 0; fn-value failures → 0 | 15–25 | **10–17** | 9–15 |
| **S2a** | the 35 declaration-only handlers (quoted 28, fn-operand 7) | `undeclaredHandlerCeiling` 94 → 59 | — | **5–8** | 3–5 |
| **S2b** | the 59 code-body handlers, on units | ceiling 59 → 0; code-body failures → 0 | — | **10–16** | 9–15 |
| **S3** | runtime compilation: computed bodies, splices, module bodies | round-trip census rows → 0 | 10–20 | **8–16** | 8–16 |
| **S4** | the lane completed; the terminal arm flipped per family | `vm:generic-*` 10 → 0; 92 sites → trap or delete | 25–40 | **18–30** | 18–30 |
| **S5** | provenance generality | provenance failures 15 → 0 | 10–15 | **8–13** | 8–13 |
| **S6** | checker totality, T1 half | `armedOnlyCeiling` 16 → 0 | 5–10 | **4–8** | 4–8 |
| **S7** | every valve deleted; `CompileCheck` total | engine entries 0; `deferCeiling` gone | 8–15 | **5–10** | 5–10 |
| | **total remaining** | | 105–175 | **75–130** | **66–116** |

**The project got about 1.4× faster, not 50×.** That is the honest
reading and it is the number to plan against. A fiftyfold speedup on a loop
that was already a minute, and a twofold speedup on the loop that actually
blocks, cannot produce more than this.

**S0 started 2026-09-18**, the same day P0 landed, with its instrument:
`test/go/sweep` generates one program per declaration-relevant word ×
operand kind (53 words, 305 cells, from a hand-written seed table — a
valid program for `def` or `walk` cannot be synthesised from a signature)
and runs each through `vary`'s dual-engine classifier and its fourteen
call forms, in about 15 s; `TestGeneratedSweep` gates the counts both
ways and `SWEEP_STATUS.md` is the matrix. The first run: 138 cells pass,
44 fail to compile, 5 island, 3 diverge; 1,932 call-form variants, 200
failing, 2 panicking. It found NUR159–163 in one afternoon — three
miscompiles, one compiler panic, one interpreter non-uniformity — which
is the §9 prediction ("S0 will probably raise them") coming true on its
first day. What S0 still owes: the module exports as rows (264
signatures across 11 modules), signature-level cells, and every corpus
ratchet re-based on the sweep's defect list.

**S1a landed 2026-09-19**, in one session-day against the three to six
estimated, on the mechanism §4 predicted: no unit cache, no Apply kernel.
`each`, `fold`, `scan` and `filter` declare `CompileDynBody`, so a dispatch
whose gradual-Any operand — the collection, or the callback read from a
class field, a map field, a dynamic key or a factory — leaves two
overloads reachable records a poly re-match over the word's own overloads
instead of refusing at the ambiguity gate; the handler picks the overload
the live value matches. All nineteen rows compile with parity, and the
gate held more behind them than the nineteen: corpus compile failures
113 → 60, islands 10 → 0, compute gaps 104 → 56, reducible rows 17 → 3,
the sweep's failing cells 44 → 36 and its islands 5 → 2, `kg/main.boru`
and two frontier rows graduated. The trade is the one §4 named as the
G-lane-first landing: the re-matched callback runs through the
RunResolved seam, so the interp-entry census rose 52 → 102 rows and the
engine-entry census 366 → 489 — measured row by row against `main`, all
fifty-one entering rows the ones the gate released — and S1b retires them
by lowering the re-matched overload's body natively. `for-each` (nets no
result) and `walk` (its own code-body gate) keep the refusal. Found on the
way: NUR164, a callback mismatch inside a handler raised a plain Go error
the compiled-by-default lane read as its own bug.

**S1b's first two increments landed 2026-09-19** (the handoff log has
both). The first made the fn-value seam native; the second resolved a
computed fn value at a forward slot — the collection seat and the `/v`
read consult the fn-carrier side table, so `each f/v xs` over a factory's
result dispatches instead of refusing — and made a fn-VALUE CLOSURE a fn
value at every callback seam, matched against its own signature before
its unit runs. Compile failures 60 → 53, the census 77 → 78 and engine
entries 419 → 422 on the one newly-compiling wrapper row. The estimate
below is NOT refreshed for them: a step's numbers move at the end of the
step, not per increment (SESSION-HANDOVER.0.md's rule), and S1b's owed
half — the apply shapes, the wrappers, provenance — is the design-bound
part the estimate was built on.

**Re-estimated 2026-09-19, at the end of S1a (the last column).** Three
session-days went on P0, S0's first increment and S1a against 8–14
estimated for those slices, but only S1a's beat is evidence, and S1a was
the step this note already called cheap. What moved each number: S1b's
islands are already 0 and its ambiguity commitment is gone, and 48 of
the 60 remaining failures are its family (provenance 16, dispatch
recovery 9, fn value reaches word 7, the apply shapes 16) — all
design-bound, so a modest cut, plus the 102 census rows to retire, which
is the unit cache itself; S2a's declaration work has been measured twice
(20 handlers in a day on 09-18, four with the full gate fallout on
09-19); S2b's dyn-body seat is a G-lane-first landing for any code-body
word, but arming DynEnv refused 53 rows on 09-19, so the seat is not
free; S3–S7 have no new evidence. The design-bound steps were not
extrapolated from S1a's beat: the project got about 1.4× faster, not
fifty.

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

**Refreshed 2026-09-19, after S1a** (66–116 remaining, §6's last column):

| | session-days | finishes |
|---|---:|---|
| low | 66 | around 19 December 2026 |
| high | 116 | around 26 February 2027 |

Year end is about 73 session-days away, so the low end now reaches it
with a week's margin; the midpoint of 91 does not. T1 + T2 by year end:
**about 30%**. It moves because the low end crossed into the year, not
because the biggest step moved — S4 is unchanged and design-bound, and
it is where the range's width lives. Two things could move it further:
the DynEnv "unpromoted computed value" refusal, now a named cost (it
blocked S1a's alternative fix and is three of the 60 failures), would
widen the dyn-body seat's reach and compress S2b; and an early unit
cache from S1b makes S2b's 59 handlers declaration work like S2a's.

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

## 11. Re-estimated 2026-09-22, after S5's first slice (the branch-carried def)

**Measured on this branch, at the head that closes NUR110.** Three
session-days since the 2026-09-19 column of §6: 09-20 (F3, the vocabulary
finish; NUR173), 09-21 (NUR174/175; the nine false-path witnesses; the
handover made current), 09-22 (S5's first slice — the seventeen-row
arm-binding cluster and NUR110, one mechanism, one day). Of those, the
F-line and NUR173–175 were not in §6's table at all: the maintainer's
fallback ruling and three fn-value-seam miscompiles found while reading
ahead. That is the shape §9 predicted — the instruments raise the bill
before they lower it — and it is the reason the total below barely moves
while three days were spent.

**Where the debt stands** (the generated census, `COMPILED_STATUS.md`,
not re-derived):

| measure | 09-17 | 09-19 | 09-21 | **09-22** |
|---|---:|---:|---:|---:|
| corpus rows that fail to compile | 113 | 53 | 62 | **45** |
| of which operand provenance (S5) | 15 | 16 | 25 | **8** |
| of which the fn-value family (S1b: value reaches word, apply shapes, hof2, curried, function-valued operand) | ~40 | ~28 | ~23 | **~23** |
| compute gaps | 107 | 49 | 58 | **41** |
| unit-suite programs that do not compile (`lang/go`) | — | 284 | 284 | **283** |
| language tests answered on the reference engine (`lang/go/test`) | — | 111 | 111 | 111 |
| engine entries / interp-entry rows (T2) | 379 / 54 | 422 / 78 | 422 / 80 | 422 / 80 |
| real programs compiling | 31/62 | 34/62 | 34/62 | 34/62 |
| known miscompiles open on `main` (T3) | 5 | 0 | 1 (NUR110, live) | **0** |

The 45 that remain, by mechanism: the fn-value family ~23 (S1b — the
apply shapes, a fn value reaching a word or an operand slot, hof2, the
curried chain), provenance 8 (S5 — the rest of Stage 5's generality: inert
values on both sides of a run, non-adjacent consumers), dispatch recovery
2, the twin regime's unplaced transitions 2, loop results as a branch
result 2, a dynamic-scope def of an unpromoted computed value 3, and five
singletons (a code-body word, a quoted-operand word, an uncaptured `for`
body, opaque output, the one correct-error row).

**Two numbers the corpus count does not show, and which now dominate.**
The two unit-suite ledgers (283 programs that do not compile, 111 answered
on the reference engine) are larger than the corpus's 45 and are not in
§6's table; and T2's engine-entry census (422) has not moved since S1b's
second increment. Reaching 0 on the corpus is "corpus-native", the review's
first package; T1 is every program, and T2 is the seam census. The
estimate below is against the ruled end state (T1 + T2), as §7 was.

**The re-estimate, per step** (the 09-19 column, and what moved it):

| step | 09-19 | **09-22** | why |
|---|---:|---:|---|
| S0 | 2–4 | **2–4** | untouched: the module exports as rows, signature-level cells, the ratchets re-based on the sweep |
| S1b | 9–15 | **8–14** | NUR173–175 landed the fn-value seam's landing model on the way (three miscompiles, none in the table); the ~23 fn-value rows, the 80 census rows and the 422 entries are its owed half and are design-bound — a modest cut |
| S2a | 3–5 | **3–5** | no evidence since |
| S2b | 9–15 | **9–15** | 12 of the 28 real programs fail on `test-describe`/`test-cover`, code-body words on units; unchanged |
| S3 | 8–16 | **8–16** | the 21 code-body census rows; unchanged |
| S4 | 18–30 | **18–30** | design-bound and unchanged — and now on a second critical path: the knowledge-graph generator dies at `DISPATCH_GENERIC at ev`, the evaluating host, and the kg gate is off until it lands |
| S5 | 8–13 | **3–6** | the frame-slot mechanism landed and took 17 of the 25 provenance rows; 8 remain, on the same seat |
| S6 | 4–8 | **4–8** | the four `undefined_word` witnesses the checker cannot flag are the T4 shape made concrete; unchanged |
| S7 | 5–10 | **5–10** | unchanged |
| | **66–116** | **60–108** | |

**Calendar and probability.** At five session-days a week from 2026-09-22:

| | session-days | finishes |
|---|---:|---|
| low | 60 | around 15 December 2026 |
| midpoint | 84 | around 20 January 2027 |
| high | 108 | around 18 February 2027 |

Year end is about 71 session-days away, so the low end reaches it with two
weeks' margin and the midpoint misses it by about three. T1 + T2 by year
end: **about 30%**, unchanged from §7's refresh — the low end came in by
six days and the width did not, because the width lives in S4 and S1b, and
neither moved. By the end of February 2027: about 65%; by the end of March
2027: about 80%.

**What would move it, in order of leverage.** (1) S1b's apply shapes as one
mechanism — 23 rows and most of the 80 census rows are one family, and the
seam's landing model is now in place. (2) The evaluating host (S4): it is
the only thing that restores the kg gate, and it is where the generic lane's
parity risk lives; starting it early is cheaper than starting it last.
(3) A second session on S2a/S2b, the one line that does not contend for
`compiler/go`. (4) The unit-suite ledgers need the same per-family census
the corpus has (`COMPILED_STATUS.md`'s buckets over `lang/go`'s 283), or
their number will be re-measured upward the way the corpus's was.

**Calibration.** S5's first slice: one session-day, seventeen rows, a
recorded miscompile closed, one new opcode, four regressions found and
fixed by the corpus gates on the way (each now a rule in the code). The
family-level rate the handover page's process rule 9 names — ten to twenty
rows per family increment — held; the row-level rate did not apply.

## 12. Re-estimated 2026-09-23, at the end of S1b's apply-shape run

**Measured on `main` at cb97cd2 (PR #491).** Two session-days since §11:
eight merges, all on S1b's fn-value family — the apply shapes (#484), the
dynamic-lead group (#485), the container-member calls (#486), the curried
chain (#487), the quotation-body reads with NUR156 and NUR181 (#488), the
recovery's window (#489), the trailing value's re-step (#490) and the
typed callback's contract (#491). Every one was reached by probing a
pinned row's neighbours before reading code, and every one found at
least one silent miscompile on `main` that no ledger held: eleven closed
in the two days (NUR156, 178–185, 155, 160), one recorded open (NUR186).
That is §9's prediction running the other way — the instruments raised
the bill on 09-17 and 09-22, and a family worked as one mechanism paid it
down faster than the row rate.

**Where the debt stands** (`COMPILED_STATUS.md`, `GATE_STATUS.md`, the
two unit ledgers and the real-program gate, not re-derived):

| measure | 09-19 | 09-21 | 09-22 | **09-23** |
|---|---:|---:|---:|---:|
| corpus rows that fail to compile | 53 | 62 | 45 | **21** |
| of which operand provenance (S5) | 16 | 25 | 8 | **6** |
| of which the fn-value family (S1b) | ~28 | ~23 | ~23 | **2** |
| compute gaps | 49 | 58 | 41 | **16** |
| unit-suite programs that do not compile / compile and bail (`lang/go`) | 284 / — | 284 / — | 283 / — | **280 / 33** |
| language tests answered on the reference engine (`lang/go/test`) | 111 | 111 | 111 | 111 |
| engine entries / interp-entry rows (T2) | 422 / 78 | 422 / 80 | 422 / 80 | **408 / 75** |
| real programs compiling | 34/62 | 34/62 | 34/62 | **36/62** |
| known miscompiles LIVE on `main` (T3) | 0 | 1 | 0 | **0** |
| silent miscompiles recorded and pinned pending | 4 | 4 | 4 | **3** (NUR154, NUR159, NUR186) — **2** after the branch result's re-step later the same day (NUR159 closed, NUR187 found and closed; NUR154 and NUR186 remain) |
| `knownDivergences` (corpus rows diverging) | 5 | 5 | 2 | **1** (NUR154) |

The 21 that remain, by mechanism: provenance 6 (S5 — the rest of Stage
5's generality), a dynamic-scope def of an unpromoted computed value 4,
loop results as a branch or body result 2, the twin regime's unplaced
transitions 2, the fn-value family 2 (an apply not at a body's tail, a
paren-bounded apply with a dynamic value before its args), dispatch
recovery 1, a code-body word 1, a quoted-operand word 1, an uncaptured
`for` body 1, the one correct-error row.

**What the census says about the rest of S1b.** The 75 interp-entry rows
are 21 code-bodies rows (S3: a quoted token body produced at run time —
a fn returning `quote [add 1]`, a flex member, a def-bound branch result —
handed to each/fold/do/filter), 8 fold-map-filter and 8 callbacks rows (a
lambda literal INSIDE a quotation body, or a factory's closure produced
inside an each body — the value stepped at the body's tail), and 4 each
of fn-value, module-fnvalue-boundary and bytecode-migrated. The fn-value
half (~27 rows) is what S1b still owes; the code-body half is S3's.

**The re-estimate, per step** (the 09-22 column, and what moved it):

| step | 09-22 | **09-23** | why |
|---|---:|---:|---|
| S0 | 2–4 | **2–4** | untouched: the module exports as rows, signature-level cells, the ratchets re-based on the sweep |
| S1b | 8–14 | **4–8** | the corpus's ~23 fn-value rows fell to 2 in two session-days; what is left is the census's ~27 fn-value rows (the same mechanisms one level down, inside quotation bodies), the two corpus rows, NUR186 (the landing's family: a NAMED fn value re-stepped at a factory tail; NUR159, its branch-arm twin, closed later the same day), and the unit-suite ledger's fn-value programs |
| S2a | 3–5 | **3–5** | no evidence since; undeclared handlers still 94 (quoted 28, fn-operand 7) |
| S2b | 9–15 | **9–15** | 12 of the 26 real programs that do not compile fail on `test-describe`/`test-cover`, code-body words on units; unchanged |
| S3 | 8–16 | **8–16** | the 21 code-body census rows; unchanged |
| S4 | 18–30 | **18–30** | design-bound and unchanged; the kg gate stays off until the evaluating host lands |
| S5 | 3–6 | **3–5** | 8 → 6: two provenance rows compiled as side effects of the fn-value increments; the seat is the same |
| S6 | 4–8 | **4–7** | armed-only 9 → 8 (a false `undefined word` gone with the recovery's window); the four `undefined_word` witnesses stand |
| S7 | 5–10 | **5–10** | unchanged; engine entries 422 → 408 moved with S1b, not with S7's valves |
| | **60–108** | **56–100** | |

**Calendar and probability.** At five session-days a week from 2026-09-23:

| | session-days | finishes |
|---|---:|---|
| low | 56 | around 8 December 2026 |
| midpoint | 78 | around 12 January 2027 |
| high | 100 | around 10 February 2027 |

Year end is about 70 session-days away, so the low end reaches it with
three weeks' margin and the midpoint misses it by about a week and a
half. T1 + T2 by year end: **about 40%**, up from §11's 30% — the low end
came in by four days and, for the first time, the width narrowed (S1b's
range halved). The width still lives in S4 and S2b, and neither moved. By
the end of February 2027: about 75%; by the end of March 2027: about 85%.

**What would move it, in order of leverage.** (1) The landing model for a
NAMED fn value (NUR186; NUR159's branch-arm half closed later the same
day — the branch result's re-step, which also found and closed NUR187,
the residual's statement boundary): the interpreter's re-step of a named
fn at the pointer ALWAYS dispatches — a name always calls, ADR-011 — and
raises on a no-match, where the landing leaves the value as data; one
rule closes the record and the main-program half of NUR124. (2) The
census's fn-value rows as one mechanism (a lambda literal or a produced
closure stepped at a quotation body's tail), the same probing method that
took the corpus's 23 rows. (3) The evaluating host (S4): still the only
thing that restores the kg gate, still the widest step. (4) A second
session on S2a/S2b, the one line that does not contend for `compiler/go`.

**Calibration.** Eight increments in two session-days, each one family:
24 corpus compile failures and 25 compute gaps closed, 14 engine entries
and 5 census rows retired, 36 of 62 real programs compiling (from 34),
eleven silent miscompiles closed and one found. Every increment ran the
full unfiltered corpus before merging and moved no ceiling the wrong way;
the one thing the commit gate missed (the arity gate under `test/go`,
which it does not run for a change under `core/go`) was caught by CI and
repaired in the next merge. The family-level rate held at the top of its
range — the rows fell faster than the row rate because each mechanism
carried its neighbours — and the row-level rate did not apply.
