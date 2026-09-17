# Full compilation: progress assessment and forecast

**Status:** assessment, point in time. **Recorded:** 2026-09-14, against
`main` at `c34a2fb` (increment 59). **Audience:** the maintainer, and any
Claude session that has to plan the next stretch of this project. It
answers two questions the running log does not: how likely the project is
to reach the end state the directive names, and how much work is left,
measured in the units Claude actually spends.

Everything below was measured on this tree or read from the gates, the
git history and the GitHub record, not transcribed from the design's own
status lines. Where a number comes from a document rather than a run, the
document is named. The design is
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md); the running log is
[FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md); the
current-state page is [SESSION-HANDOVER.0.md](SESSION-HANDOVER.0.md).
This note does not replace any of them.

---

## 0. The verdict, in one paragraph

By the instruments the project reports against every day, it is three
quarters done: the frontier ledger has fallen from 127 refusing rows to
29, and the interpreter-entry census from 184 rows to 28, in seventeen
days of work. By the instruments the mission is actually defined against,
it is less than a quarter done: the compiler still carries 92 refusal
sites and 114 undeclared handlers, the runtime still has its 17 defer
sites, the fallback hatch and `OpFallback` machinery, and the design's
central mechanism, the generic lane (`OpCollect` / `OpDispatchGeneric`),
exists only in comments and an inert descriptor table. The corpus-level
finish is likely (roughly 70% within a month, 90% by year end, with
rulings on the rows that are specified interpretation). The literal
end state the directive names (T1 totality and T2 no islands, for every
program rather than for the corpus) is unlikely on any near horizon and
roughly a coin flip by year end even with an amended carve-out list. The
remaining effort is not 29 rows; it is four unbuilt stages, and it is
larger than everything spent so far.

### 0.1 The ruling, received after this note was written (2026-09-14)

Section 8's first recommendation asked for a written definition of done.
The maintainer gave one the same day:

> done is a language that compiles, as a developer expects. all valid
> code compiles, no exceptions

That is T1 exactly as the design states it (§1 of
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md)), with no carve-out list.
What it changes in this note:

- **The T1 half of outcome B is settled; B versus B′ is not.** O4 is
  answered NO for compilation, and "carve-out" is no longer a disposition
  a refusal site can take. The choice between B and B′ turns on T2's
  attributed set (the first open question below), which the ruling does
  not address, so this note records neither as chosen. Plan against B's
  figures as the upper bound: §6.1 plus the whole of §6.2, a further 65
  to 115 session-days after corpus-native, and about 25% by year end,
  rising as the generic lane lands.
- **Every refusal site has three legal dispositions**: a generic
  lowering, a trap raising the interpreter's own error, or deletion. The
  §8 census pass is therefore not a triage but an assignment, and the
  generic lane (§6.2, B1) is the critical path, because it is the only
  mechanism that gives sites the first disposition in bulk.
- **"Valid" is decided by the interpreter**, so the checker may never be
  the reason a program fails to compile: the "check diagnostics"
  sentinel (Stage 8's T1 half, §6.2 B5) is inside done, and so is
  Stage 7 (§6.2 B3), since computed code is code.
- **Two questions the ruling leaves open**, put to the maintainer the
  same day: whether words whose answer IS the engine's behaviour
  (`boru:debug`'s stepper and profiler, `RunTrace`) may still interpret
  at run time inside a compiled program, which is T2's attributed set
  and a runtime question rather than a compilation one; and whether
  `boru check` agreeing with `boru run` (T4, §4.3's 279 rows) is inside
  "as a developer expects".

The current-state page,
[SESSION-HANDOVER.0.md](SESSION-HANDOVER.0.md), carries the ruling and
the next two increments it orders.

---

## 1. What "success" means, and why there are two universes

The design defines the mission as four terms (§1):

- **T1, totality:** `CompileCheck` never refuses; `compile_refused` and
  the `BORU_COMPILE_FALLBACK` hatch retire.
- **T2, no islands:** a compiled program never re-enters the interpreter
  over tokens or value windows, and never bails to a whole-program re-run.
- **T3, parity:** compiled and interpreted runs agree on every observable.
- **T4, checker alignment:** the checker never blocks compilation and
  reports the same findings whichever lane consumes them.

Every number the project reports daily measures a **sample**: the
`lang/spec` corpus (7,798 rows today, 7,475 of them compilable). The
frontier ledger counts corpus-adjacent rows that refuse; the interp-entry
census counts corpus rows that run compiled and interpret anyway. T1 and
T2, by contrast, are claims about the **whole gate inventory**: every
`MarkUncompilable` site, every lowerer decline, every handler that has not
declared what it does with its operands, every `vmDefer` site. The design
says this itself (§2.2): "Totality must be proven against the whole gate
inventory, with generated differential sweeps as the oracle, not against
the 153 rows."

So the project has two progress curves, and they disagree. The sample
curve is steep and nearly at its floor. The inventory curve has barely
moved. Any forecast has to say which curve it is about.

---

## 2. Where the project is, measured

### 2.1 The gates

Values as measured on this tree on 2026-09-14 (`test/go/langspec`), with
the Stage-1 baseline and the end state each gate's own comment names.

| instrument | what it measures | baseline | now | end state | moved by |
|---|---|---:|---:|---:|---|
| `frontierCompileLedger` | corpus-adjacent rows the compiler refuses | 127 (08-26) | **29** | 0 | 98 rows |
| `interpEntryRowCeiling` | corpus rows that run compiled and still enter the interpreter unattributed | 184 (08-28) | **28** | 0 debt (attributed entries permitted) | 156 rows |
| `refusalCeiling` / `islandCeiling` | corpus rows refused / with an `OpFallback` span | 0 / 0 | **0 / 0** | 0 / 0 | pinned before the project began |
| compiled rows | corpus rows that produce a Program | 7,180 | **7,475** of 7,798 (323 statically invalid) | all | corpus growth |
| `refusalSiteCeiling` | `MarkUncompilable` call sites in the source | 96 (08-25) | **92** (ceiling 93) | 0 at Stage 9 | 4 sites |
| lowerer / `Finalize` declines | decline sites after recording | 78 (08-25) | not re-measured; 161 reason templates in total | none reachable | unknown |
| `undeclaredHandlerCeiling` | declaration-relevant signatures with no compile declaration | 114 of 172 (08-25) | **114** | 0 at Stage 6 | 0 |
| `deferCeiling` | `vmDefer` activations on the corpus walk | 5 (08-25) | **5** (`poly-nout-drift` ×3, `poly-no-match` ×2; one more resolved locally); 17 `vmDefer` sites in `eng/go` | 0 at Stage 9, then the mechanism deletes | 0 |
| `engineEntryCeiling` | unattributed `Engine` entries on the corpus walk | 505 (08-25) | **281** live (ceiling never lowered from 505); `CallBoru` ×240 of them, most being `Test.property`'s ~100 invocations per row | 0 at Stage 9 | 224 entries |
| `diagnosticParityCeiling` | corpus rows whose findings differ between the plain and the compile-armed check | 318 (08-26) | **320** (4 armed-only, 37 carried by the refusal reason, 279 lost under compilation) | 0 at Stage 8 | +2 |
| twin-placement frontier | body shapes the rollback-and-replay regime cannot yet replay | 4 (09-02) | **2** | 0 | 2 shapes |
| opcodes | declared in `compiler/go/bytecode.go` | | 53 | | |
| `OpCollect` / `OpDispatchGeneric` | the generic lane's two opcodes | 0 | **0 declared** (comments only); `Program.Regions` carries 38,734 inert descriptors over 5,629 programs, mono-native dispatch only | live | unbuilt |
| escape valves | `OpFallback`, the drift window (`OpCallDynamicMixed`), `OpCallDynFrame`, `vmDefer`, `compile_refused`, `BORU_COMPILE_FALLBACK` | all live | **all live and still emitted** | deleted at Stage 9 | 0 |

Three readings of that table matter more than any single row.

1. **The two ratchets that fell are both corpus samples.** Everything
   that measures machinery rather than rows (refusal sites, undeclared
   handlers, defers, valves) is within noise of its baseline. Four refusal
   sites retired in seventeen days is the honest rate at which row-level
   work retires inventory.
2. **`engineEntryCeiling` at 505 is a ceiling, not a measurement.** It was
   never lowered after the baseline; the live count is 281. Entries fell
   44% while census rows fell 85%, because what remains is concentrated:
   `Test.property` runs its raw-quotation bodies about a hundred times per
   row, so two or three `module-test.tsv` rows account for most of the
   240 `CallBoru` entries. Read the entry count as a shape, and the row
   census as the score; and lower the ceiling to 281 in the next
   increment that touches the file, since a ceiling that sits 80% above
   the live value cannot catch a regression.
3. **The zeroes were zero before the project started.** `refusalCeiling`
   and `islandCeiling` reached 0 under the previous programme, and the
   whole point of this one (§2.3 of the design) is that those zeroes were
   blind to 184 rows of live interpretation. Do not read them as progress.

### 2.2 The stages

The design's §10 stage table, with what has actually landed. "Landed" here
means gated and merged; "narrow form" means a mechanism exists for the
shapes the corpus contained and not for the general case the design
describes.

| stage | work | status on this tree |
|---|---|---|
| 0 | declaration triple, dead-flag deletion | **landed** (2026-08-26) |
| 1 | the censuses and the observable alphabet | **landed**; the interp-entry census (08-28) is the instrument the whole line has run on since |
| 2 | extract the collection kernel; three Engine re-seats | **landed** (08-26), gate discharged (allocation ceilings, CPU share, cover-gate) |
| 3 | universal fn values + the Apply kernel; retire `OpCallDynFrame` / `callDynamic` islands | **mostly landed**: Apply kernel, predicate units, container stamping, lenses, shaped-method apply, closure bridge, produced-closure apply, tail-apply collapse. **Open:** the Church `csucc` family (a bare read beneath a paren-bounded apply, NUR123's remaining half), the parser-combinator captures, the NUR038 member-arrival seal, the carrier-lead island. `OpCallDynFrame` and the `callDynamic` non-closure arms still exist and are still emitted |
| 4 | statement descriptors + `OpCollect` / `OpDispatchGeneric` + bind twins; step 6 flips refuse to generic; drift window deleted | **half landed.** The bind twins are landed and flipped (09-02, rollback-and-replay is the only regime; two placement shapes remain). The dispatch half is **not built**: Phase B writes descriptors for mono-native dispatch only, the VM `CollectHost` adapter declines every evaluation, and Stage 4a-2's measurement (09-03) showed `OpCollect` must reproduce the planner's signature selection from the descriptor and carry raise-selection state, which is the hard part. Stage 4b (09-04) re-filed the frozen-read class under a binding-sensitive unit memo, so the `k` pair compiles without the lane; what stays filed under `OpDispatchGeneric` is the lookup half for escaping units, the stored-handler latch, family L's conditional fn shadow and NUR037. The line has not returned to it since. The drift window is live |
| 5 | production-order regions + generalized marks | **narrow form**: a value-producing loop's region and `await`'s variadic residual are one simulated slot with a runtime count; `OpSeatBelowMark` and `OpMakeListToMark` close the prefix and list-collect shapes; increments 57 and 59 seat `[inert…, REGION]` and `[REGION, inert…]`. **Not built:** a run with inert values on both sides, a region consumed by an arbitrary word, a non-adjacent consumer, the split-rule window (`3 m.f 2`), and any region that may carry a callable (NUR129) |
| 6 | handler migration per the triple: units not tokens, `while`, per-region DynEnv, `args` / `__pa` / `context` frames | **started by rows, not by the worklist**: `while` (37), `for-each` (44), `Log.with-span`, `Assert.throws`, `ArrayUtil.foldaxis` declared. The declaration census still reads 114 undeclared of 172; `context` (family K) and the check-lenient words are untouched |
| 7 | runtime compilation everywhere: computed bodies, splices, module bodies; the structural unit cache; unbounded memoised restamp | **not started.** The unit cache the design names as a hard dependency is not built; O4 (module bodies at import) and O5 (check-budget exhaustion) are unruled; F5 is untested |
| 8 | checker totality: sentinel deletion, traps for definite errors, `!Compiling` fork collapse | **instrumented only**: the diagnostic-parity gate and its user-impact split exist; the two `uncalled_function` traps (increment 49) are the first definite-error traps. 4 armed-only rows, 37 suppression rows and 279 false-positive rows stand where they stood |
| 9 | retire the valves; delete `OpFallback` / P7 machinery / the fence's re-run half / the hatch; flip `CompileCheck` to total | **not started** |

The dependency spine the design draws is 2 → {3,4} → 5 → 9 with 6, 7, 8 as
parallel tracks. On this tree the spine is complete through 3 (mostly),
half of 4, and the narrow slice of 5 that the corpus asked for.

---

## 3. How it got here: effort and rate

Measured from the git history between the design's adoption (`8181f78`,
2026-08-26) and `c34a2fb` (2026-09-11). The three days since are idle on
this project; the one open PR is unrelated.

| quantity | value |
|---|---|
| calendar days of activity | 17 |
| commits | 283 (about 40 merges) |
| pull requests merged | 46 (#406 to #451), every one merged by the maintainer the same day, usually within one to three hours |
| Claude sessions with trailers | 5 (16, 43, 50, 77 and 1 commits); plus the untrailed 08-29/30 work (66 commits, PRs #411 to #414) |
| non-documentation lines inserted | 46,753 (compiler/go: 16,545) |
| design and NUR lines inserted | 12,705 |
| numbered increments | 59 (numbering began with the closure-capture line on 09-05; the Stage 2, 3 and 4 work before it was four multi-day increments) |
| increments built and then rejected, reverted or parked | at least 9 (about one in seven) |
| NUR records opened | 29 (NUR101 to NUR139); 9 compile-related still pending: NUR110, 122, 123, 124, 128, 129, 130, 134, 135 |
| silent miscompiles found and fixed along the way | at least 12 (five in #448 alone, two on 09-05, five at NUR101) |
| median code insertion per non-merge commit | 158 lines |
| review load | Codex review on every PR; the maintainer's own review comments on most |

The two sample curves, from the log's own figures:

```
frontier ledger rows
  127 (08-26) → 102 (09-04) → 84 (09-05) → 77 (09-07) → 47 (09-08)
      → 42 (09-09) → 32 (09-10) → 29 (09-11)
interp-entry census rows
  184 (08-28) → 130 → 62 → 46 (08-29) → 33 (08-30)
      → 33 (09-09) → 32 (09-10) → 29 → 28 (09-11)
```

Two things the curves say. The census fell from 184 to 33 in three days
(the Apply kernel, container stamping, lenses, parselang: one mechanism
retiring ten to twenty rows at a time) and then **sat at 33 for eleven
days** while the line worked the ledger; the last five rows took two
increments each. The ledger fell slowest first and fastest in the
closure-capture line (84 → 47 in three days), which was a single family
with one root cause worked systematically. Both curves are now in the
regime where **each increment retires one to three rows and about one
increment in seven retires nothing**.

The observed unit of Claude effort is a **session**: three to four days of
continuous work, 16 to 77 commits, ending when the context is exhausted or
handed over. Increments per session have risen from four (the twins line)
to thirty-four (increments 26 to 59), which says the recent increments are
small, not that the work got faster: the median increment in that last
session changed under 200 lines of code.

---

## 4. What remains

### 4.1 Tier A: the corpus sample (the 29 and the 28)

The 29 ledger rows by refusal reason, with the mechanism each needs and
whether that mechanism is within reach of a row-level increment:

| rows | reason | what graduates them | reach |
|---:|---|---|---|
| 7 | `body result of unknown provenance` (Church numerals ×3, parser combinators ×4) | the bare-name read beneath a paren-bounded apply (NUR123's open half); captured alternation parsers; `if` arms netting a fn carrier | row-level, but the Church rows need all three at once |
| 6 | `islanded` | a carrier-lead apply whose arriving value carries a unit (×2); the maybe-raising `do … error` region seated without the island's slot, which is Stage 5 (×2); a def-bound closure read inside a data list (×2) | half row-level, half Stage 5 |
| 4 | `fn-value-call boundary` / `auto-dispatches` (the NUR038 seal) | the member-arrival model admitting a stack form, a computed first argument, an anonymous member, and the mixed-arity overload | row-level, one model |
| 3 | `check diagnostics` | the L-DO rows are check-rejected on a failed module fn-value dispatch (NUR134's class) before any lowering; the `/v` hold is a deliberate Stage 1 choice | Stage 8 or a ruling |
| 2 | `no stream placement` (twin placement shapes 1 and 2) | shape 1 is parked with three measured constraints (a cached import installs nothing; `transplantWordExtensions` is a second twin source; the recorder is suspended in both passes); shape 2 needs an op that rebuilds a type per element | structural |
| 2 | `redefined inside a conditional / fn body` | a runtime dispatch that respects a conditional binding: NUR110's third binding state, three attempts recorded and rejected | structural, blocked on NUR110 |
| 1 | `no layer to hand out` (family K) | a context-frame opcode pair bracketing inline-lowered regions | Stage 6 |
| 1 | `unmatched dispatch recovered at dot` | the trap or rematch accepting a recovered window through a Function param | row-level |
| 1 | `computed fn shadows a live binding` | Defs and the carrier table agreeing about a name | row-level |
| 1 | `code body reads a def-bound compiled closure` | a `do` body applying a compiled closure without re-running on the interpreter | Stage 7 slice |
| 1 | `capture ParseLang … unreachable at a call site` | a bakeable operand home for a body-imported namespace | row-level |

The 28 census rows by mechanism (from the census header's own table,
adjusted for increments 44, 57 and 59):

| rows (approx.) | mechanism | what retires it | reach |
|---:|---|---|---|
| 8 | `Test.invoke` / `Test.prop` / `Test.check-prop` bodies written as raw quotations | compiling quotation bodies at runtime, a Stage 7 slice; "worth perhaps three rows" per the census, since the seam counts invocations | Stage 7 |
| 5 to 7 | a callable read out of a container behind a modifier wrapper (`usurp`, `/u`, force-arity), or the split-rule window `3 m.f 2`, or the park rule for an unmatched callee | the modifier words lowering to a dispatch; production-order regions for the split rule | Stage 5 |
| 4 to 6 | `do` over a body whose residual count is not static, beyond the two seated shapes | inert values on both sides of a run; an event above the run; a second region above it | Stage 5 |
| 4 | a fn value crossing a module boundary, applied inside | hosting a detached ref as a frame of the running program (a deliberate decline today), plus flow-sentinel bodies | structural |
| 4 | server and mount callbacks (a live socket, a fileops map) | re-entrant VM hosting on a busy registry (`CanHostVM` decline → `CallBoru`), §6.10's table | structural |
| 3 to 4 | misc: `canon` / `Vm.run` round trips, a Rand generator, one fn-value row | runtime compilation (§6.7) | Stage 7 |

Read together: of the 57 sample items, roughly **20 are row-level** (a
predicate, a model widening, a declaration), roughly **25 need a Stage 5
or Stage 7 slice** that does not exist yet, and roughly **12 are
structural** (re-entrant hosting, detached refs, NUR110's third binding
state, the import-in-multi-run twin). The row-level share will fall at the
current rate. The rest will not fall until someone builds the slice, and
the last session's log shows the line already brushing against that
boundary (increment 58 parked; the `do [7 for 3 [1] 8]` shape declined;
NUR129's callable region left open).

### 4.2 Tier B: the mission's machinery

This is the work T1 and T2 are made of, and none of it is visible in the
sample curves.

- **The generic lane (Stage 4, dispatch half).** The design's whole claim
  is that the worst verdict becomes "lower generically" rather than
  "refuse". That requires `OpCollect` running the extracted collection
  kernel over a descriptor at runtime, and `OpDispatchGeneric` selecting
  the signature the planner would have selected, from the descriptor and
  never from the bare window (the dispatch-agreement census states that
  contract). Stage 4a-2 measured the first acceptance test (the frozen
  read `k` pair) and found the call site is mono: the recorded signature
  was chosen from the frozen value, so a live operand alone reproduces
  nothing. F2 fired: raise selection needs state outside the collection
  machine (`reorderCandidates` reads the enclosing stack, `voidGroups`
  changes the error code). Descriptors exist for mono-native dispatch
  only; the five other record families record no descriptor. Stage 4b
  (09-04) then superseded 4a-2's revised order: the frozen-read class was
  the unit memo's problem, not a routing one, the program-wide
  `frozenReads` map is gone, and the `k` pair compiles today through a
  binding-sensitive memo. What stays filed under the lane is §6.9's lookup
  half for the shapes the memo cannot re-record: escaping units, the
  stored-handler latch (a module-scope def site executes only in the check
  pass, so a stored handler reads the pass-final binding), family L's
  conditional fn shadow, and NUR037's fn-local fn. The lane's first
  acceptance test is one of those pairs, not the `k` pair. This is the
  largest single piece of unbuilt work, and it is the piece that would
  retire refusal sites in bulk rather than one at a time.
- **Handler migration (Stage 6).** 114 signatures take a code body, quote
  an operand, or can receive a Function value and declare nothing. Each is
  either a declaration (`for-each` took one increment) or a handler
  rewrite (`eachrank` walks raw tokens and has no seam to declare over).
  The census is a floor: importing modules registers more. Family K's
  `context` layer and the check-lenient `SuppressedRuntimeError` words
  are their own items.
- **Runtime compilation everywhere (Stage 7).** Quotation bodies, computed
  `do` bodies, splices, interp-string and XML code parts, `mini` hooks,
  module bodies at import. Its stated hard dependency, the structural unit
  cache keyed on `FnAnalysisKey`, is not built. Two open rulings gate it
  (O4, O5) and F5's induction argument is untested.
- **Refusal-site retirement.** 92 `MarkUncompilable` sites and the
  lowerer's declines, 161 reason templates in all, of which the ledger has
  ever exercised about a fifth. Whole gate families have zero ledgered
  rows (`args` / `__pa`, anonymous dispatch, quoted-operand words, `for:`
  multi-value bodies, mid-body dynamic apply, splice and interp-string
  runtime parts, the typed-def construction family). The generic lane
  absorbs many of these; the rest are one increment each, and the
  corpus cannot see them, so each needs its own off-corpus pin.
- **Checker totality (Stage 8), the T1 half.** Delete the whole-program
  "check diagnostics" sentinel; compile definite errors to traps or the
  runtime error builder; the 4 armed-only rows and the 37 suppression
  rows. The 279 "lost under compilation" rows are checker false positives
  and belong to T4, not T1.
- **Valve retirement (Stage 9).** 17 `vmDefer` sites, each with a named
  native replacement in §6.10; `OpFallback`, its lowerer, `islandRun`,
  the drift window and the P7 machinery deleted; the C1 fence reduced to
  error propagation; `compile_refused` becomes a structured internal
  error; `BORU_COMPILE_FALLBACK` deleted. Mechanical once everything above
  lands, and impossible before.
- **The off-corpus oracle.** T1 has to be proven against generated sweeps
  (§8.3). The variation lane exists and has been the single most productive
  bug finder in the project; a generated dispatch-shape sweep does not yet
  exist as a gate.

### 4.3 Tier C: T4's remainder

279 corpus rows where `boru check` reports an error the compiler then
compiles past. This is a checker precision programme with its own ratchet
(`CHECK-ACCURACY-RATCHET.10.md`), separable from T1/T2, and not on the
current line's path at all.

### 4.4 Tier D: rulings nobody has made

Each of these changes what "done" means, and each is cheap to rule and
expensive to leave open:

- **O2**, the step budget: keep the documented metering divergence or
  re-meter.
- **O4**, module bodies at import: compile them, or declare import-time
  execution a front-end carve-out (the census already attributes
  `module-load`).
- **O5**, check-time budget exhaustion: there is no emit-anyway story.
- **The attributed set.** The census header argues, correctly, that the
  honest end state is "every entry attributed", not "no entries": a step
  counter, a profiler, a tracer must interpret to answer. That argument
  has been applied word by word (`boru:debug`, `RunTrace`); it has not
  been ruled as policy, and the busy-registry callback route and the
  detached-ref decline are candidates for it.
- **NUR110**'s third binding state, which blocks two ledger rows and any
  general answer to conditional rebinding.
- **O1's remaining half**, NUR078, which needs re-spelling before it can
  be implemented.

---

## 5. Probability of success

These are judgement estimates, calibrated on the rates in §3, the
structure of the remainder in §4, and one piece of history the tree
records: this is the fourth completion programme run against this
compiler in ten weeks. `legacy/P7-ENDGAME.10.ignore` landed on 2026-07-04 claiming
runtime independence; `legacy/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.ignore` (07-13)
and `legacy/REFUSAL-CLOSURE.0.ignore` (07-15) each reached their own ratchet finish
lines ("census 6000/6000 native, refusals 0, bails 0"); and
`FULL-COMPILATION.0.md` (08-25) opened by measuring that those zeroes hid
184 rows and 959 entries of live interpretation. Each programme's finish
line was true of its instrument and false of the runtime. The current
instruments are materially better (the runtime census, the variation
lane, the static censuses), which is why the corpus estimates below are
high; the inventory estimates are low for the same reason the earlier
programmes' finish lines moved.

| outcome | definition | by 2026-10-15 | by 2026-12-31 | eventually, at sustained effort |
|---|---|---:|---:|---:|
| **A. corpus-native** | ledger 0; census debt 0; refusals and islands 0 | 45% | 80% | 90% |
| **A′. corpus-native with rulings** | as A, with rows that are specified interpretation attributed or ledgered under a ruling | 70% | 90% | 95% |
| **B. T1 + T2 as written** | no refusal path, no unattributed entry for any program, valves and hatch deleted, no new carve-outs | under 5% | 25% | 55% |
| **B′. T1 + T2 with an amended carve-out list** | as B, after the Tier D rulings name what interpretation is permitted | 5% | 45% | 75% |
| **C. T3 maintained** | no known miscompile on `main` at any merge | 90%, ongoing | 90% | 90% |
| **D. T4 as written** | diagnostic parity 0, sentinel deleted | under 5% | 20% | 50% |

The reasoning behind the two rows that matter:

**Why A′ is high.** The remaining sample is small, enumerated, and every
row has a named mechanism. The line has shown it can retire a family in
three days once the root cause is found, and the maintainer merges within
hours. The gap between A and A′ is the twelve structural items in §4.1:
some will graduate by ruling rather than by code, which the design permits
and the census header already practises.

**Why B is low.** Four stages are unbuilt and one of them (the generic
lane) has already had its first design premise falsified by measurement
twice (F1's three-loops correction, F2's external raise state, Stage
4a-2's mono call site). The static inventory has moved 4% in the time the
sample moved 80%. The row-level method that produced the sample curve
cannot produce the inventory curve: each refusal site the corpus does not
reach needs its own off-corpus pin, and there are about 130 of them. And
the base rate of this compiler's completion programmes is that the last
step reveals a residue the instrument could not see; T2's own definition
now includes "runtime compilation must itself never refuse", an induction
nobody has tested. B′ is higher because most of what would keep B from
closing (busy-registry hosting, module bodies at import, the debugger, the
step budget) is interpretation that has a legitimate specification, and a
ruling converts it from debt to contract.

**Why C is high but not higher.** The gates are strong (byte-identical
differential, the variation lane, the property fuzzer, ADR-008 coverage)
and every miscompile found in this line was found before merge or within a
day. But the rate is not zero: about one miscompile per five increments,
and every one was in code the corpus could not reach. Widening the typed
lane one shape at a time is exactly the activity that produces them; the
generic lane, which shares the kernel, is the design's answer, and it is
the part not built.

---

## 6. Remaining effort, in the units Claude spends

A **session-day** below is one Claude session working for roughly one
day: on this project that has meant 15 to 25 commits, one to eight
increments depending on their size, and the fmt / vet / lint / test /
cover-gate cycle on each. Seventeen calendar days have consumed about 20
session-days (two sessions overlapped in the second week). The ranges are
wide on purpose; the low end assumes no negative results, the high end
assumes the observed one-in-seven rate and one falsified premise per
stage.

### 6.1 Tier A: to outcome A′

| package | content | session-days |
|---|---|---:|
| A1 | the row-level remainder: NUR038's arrival model, the recovered-dot trap, the computed-fn shadow, the ParseLang capture, the carrier-lead unit entry, the data-list closure read | 4 to 6 |
| A2 | Stage 5 generality: a run with inert values on both sides, an arbitrary consumer, a non-adjacent consumer, the split-rule window, modifier words lowering to a dispatch | 4 to 8 |
| A3 | re-entrant VM hosting on a busy registry; a detached ref hosted as a frame (server, mount, module-boundary rows) | 3 to 6 |
| A4 | quotation bodies compiled at runtime for `Test.*` and the `canon` / `Vm.run` round trips, the first Stage 7 slice | 3 to 5 |
| A5 | NUR110's third binding state (both conditional-shadow rows) and the two twin-placement shapes | 3 to 6 |
| A6 | rulings and attributions for what is specified interpretation; the ledger and census moved to their end-state form | 1 to 2 |
| | **total** | **18 to 33** |

At one session at a time and three to four session-days per session, that
is five to nine sessions and **three to six calendar weeks**.

### 6.2 Tier B: from A′ to B′

| package | content | session-days |
|---|---|---:|
| B1 | the generic lane: descriptors for every record family, `OpCollect` over the kernel, `OpDispatchGeneric` reproducing the planner's selection, `RegionState` for raise selection, the dispatch-agreement census as its gate, the `k` pair as acceptance | 15 to 25 |
| B2 | Stage 6: the 114 undeclared handlers, declared or rewritten; `context` frames; the check-lenient words | 12 to 20 |
| B3 | Stage 7: the structural unit cache, computed bodies, splices and code parts, module bodies (after the O4 ruling), the unbounded memoised restamp, O5's answer | 10 to 20 |
| B4 | refusal-site retirement: the sites the generic lane does not absorb, each with an off-corpus pin; the lowerer's declines converted to lowerings or traps; a generated dispatch-shape sweep as the gate | 15 to 25 |
| B5 | Stage 8, the T1 half: sentinel deletion, definite-error traps, the 4 armed-only and 37 suppression rows | 5 to 10 |
| B6 | Stage 9: 17 defer sites replaced natively, `OpFallback` / drift window / P7 deleted, the fence collapsed, `CompileCheck` flipped, the hatch deleted | 8 to 15 |
| | **total** | **65 to 115** |

That is 18 to 30 sessions and, at the cadence observed, **three and a half
to six months** of calendar time after Tier A. The packages are not
independent: B1 must precede most of B4 and all of B6, and B3 must precede
the induction B6's flip depends on. Two sessions could run B2 beside B1
because they touch different modules, which is the only real
parallelism available; everything else contends for `compiler/go`.

### 6.3 Tier C: T4's remainder

10 to 20 session-days, separable, and not needed for B′.

### 6.4 The overall picture

| | session-days | as a share of the whole |
|---|---:|---:|
| spent (08-26 to 09-11) | about 20 | 12 to 20% |
| to A′ (corpus-native with rulings) | 18 to 33 | |
| to B′ (T1 + T2 with a carve-out list) | a further 65 to 115 | |
| total to B′ | **about 100 to 170** | |

By effort, the project is somewhere between an eighth and a fifth of the
way to the end state the directive names (20 of about 100 to 170), and
about half of the way to the corpus-native milestone the daily
instruments measure (20 of about 40 to 55). Expect the
estimate to move by a third in either direction when B1 lands, because
the generic lane is the one package whose cost the tree has not yet
measured at all.

---

## 7. What would move these numbers

Down (faster, more likely):

- **A ruling on the attributed set and O4.** It removes the busy-registry,
  detached-ref and module-load items from the debt in one stroke, and it
  is the difference between B and B′.
- **The generic lane landing on the `k` pair.** If `OpDispatchGeneric`
  can reproduce the planner's selection from a descriptor, B4 collapses
  from "one increment per site" to "route the site to the lane".
- **Two sessions in parallel on B1 and B2**, which the module split
  permits.

Up (slower, less likely):

- **A falsified premise inside B1.** The design has had three so far. The
  expensive one would be F2's widened form failing: raise selection needing
  state the descriptor cannot carry.
- **Miscompiles in the generic lane's shared kernel**, which would put
  interpreter behaviour in play, where the differential is blind by
  construction (the design's own warning at §10, T2).
- **The registry race.** `fatal error: concurrent map read and map write`
  has killed the suite twice (timer bodies, `serve-raw`); the fix is a
  fence at the spawn seam and is nobody's increment yet. A flaky
  full-corpus gate slows every merge.
- **Coverage cost.** ADR-008's 100% floor makes every seam a tested seam,
  which is why the miscompile rate is as low as it is, and why a
  five-line predicate costs a day.
- **Context exhaustion.** Sessions end at three to four days; every
  handover costs a re-derivation, and the log records two frontier notes
  in a row pointing one layer off the real gap for exactly that reason.

---

## 8. Recommendations

1. **Decide which end state is the target, in writing.** Rule O2, O4 and
   the attributed set now. Until then every forecast has two answers and
   every "is this row debt?" question is re-argued per row.
2. **Build the generic lane's first executing slice next**, on the `k`
   pair, in the handoff's revised order. It is the only package that
   changes the inventory curve's slope, and the longer the row-level line
   runs without it, the more shapes get bespoke typed-lane answers that
   the lane will later have to subsume.
3. **Stop taking rows whose fix is a Stage 5 or Stage 7 slice as
   row-level increments.** Increment 58 (parked) and the `do [7 for 3 [1]
   8]` decline are the signal. Build the slice; the rows follow.
4. **Make the static ceilings fall by plan, not by accident.** Assign each
   of the 92 refusal sites a disposition (generic lowering, trap, delete,
   or carve-out with a ruling) in one census pass. Four sites in seventeen
   days is what happens when the corpus is the only thing that can move
   them.
5. **Fence the registry at the spawn seam** before the next full-corpus
   gate run dies on it.
6. **Keep the negative-result discipline.** One increment in seven has been
   built and rejected; every one of those left a measurement that saved a
   later session a day. That is the right rate for this kind of work, not
   a defect.
7. **Plan in session-days and expect handovers.** The current handover
   pair (SESSION-HANDOVER for state, the log for history) is working;
   the report above should be refreshed at the end of each tier, not each
   increment.

---

## 9. Method

Every figure in §2.1 was produced by running the named test on this tree
(`go test ./ -run <Test> -v` in `test/go/langspec`) or by a source count
(`grep -c`) on 2026-09-14; the constants were read from the gate files
and compared with the live output. Effort figures come from `git log`
between `8181f78` and `c34a2fb`, with `Claude-Session` trailers grouped
per session and `--shortstat` per commit excluding `design/`, `kg/` and
`NUR.md`. The ledger's reason breakdown is a `grep` over the
`frontierCompileLedger` map; the census mechanism table is the header of
`interp_entry_census_test.go`, adjusted for the three increments that
post-date it. PR cadence is from the GitHub record for #406 to #452. The
probabilities in §5 and the session-day ranges in §6 are the author's
estimates from those inputs and are labelled as such; nothing in §5 or §6
is a measurement.
