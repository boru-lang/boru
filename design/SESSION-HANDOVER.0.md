# Session handover — the full-compilation project

**Purpose.** The one page a fresh session reads to know where the project
stands *right now*. It is deliberately short and deliberately
CURRENT-STATE-ONLY: the per-increment narrative, the measurements and the
lessons live in [FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md),
which is an append-only log and the wrong place to look for "what is true
today". Update this file at the end of every increment.

Last updated: **2026-09-20**.

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
names, the user-facing message and the `compile_failed` error code, and the
2026-09-20 sweep finished the tree: **260 distinct "refus" identifier forms
→ 65, 4380 occurrences → 665**, every survivor an ENTITLED refusal (policy
denials, vault and proxy security, weak containers, option validation, exit
ranges, capability gates, `await` isolation, signature matching), where the
word is the right one. Do not add more in the compilation sense.
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

## The interpreter fallbacks are gone (2026-09-19, PR #476 — MERGED)

Read this first: it changes what every other number on this page means.

The maintainer ruled: *"remove all interpreter fallbacks, flags, modes, etc.
Failure to compile is a plain bug only."* That is the scaffolding
[COMPILABLE-SUBSET.md](COMPILABLE-SUBSET.md) §1 has always called scaffolding,
brought forward from its planned Stage-9 retirement. **PR #476 MERGED
2026-09-19 (`ba64e11` on `main`).** The vocabulary and dead-machinery sweep it
left owing landed on 2026-09-20 — see "The sweep that finished it" below.

**What went.** Ten mechanisms. The count is the finding — nobody had them in
one list:

1. the `"check diagnostics"` carve-out in `RunAutoValues`;
2. the fn-carrier-read carve-out beside it;
3. `BORU_COMPILE_FALLBACK=1`, the one-release hatch;
4. the runtime-bail arm (roll back, re-run the whole source);
5. `Run`'s own explicit fallback;
6. the CLI's try mode and warning, with `CompileOff`/`CompileTry`/
   `CompileForce`, `--compile`, `--no-compile`, `--force-compile` and their
   three environment variables;
7. the fn-value token seam's degrade;
8. the detached-callback retry (`InvokeCompiled` → `CallBoru`);
9. an `await` BRANCH's re-run of its raw tokens;
10. the REPL's per-line re-run and the HTTP exec handler's per-request one.

The C1 effect fence went with them: it counted output escaping the check pass
so a re-run could be blocked before duplicating it, and there is no re-run to
block.

**What stayed, and why it is not an exception.** `Vm.run`'s sub-engine
(`modules.CompiledSubRun`). It executes source constructed at RUNTIME, so
there is no ahead-of-time program to compile — "compiling" it is
parse+compile+run at run time, which IS the interpreter. That is tier 1 of
`compiled_metafallback_test.go`'s partition, the one category the project has
reasoned is genuinely irreducible. Removing it made a LANGUAGE WORD refuse
programs the interpreter runs (two `canon` rows, caught by CI on the
INTERPRETED spec gate). The compile there is an opportunity, not an
obligation.

Interpreter ISLANDS also stay, and the number is on the record because the
removal was built and measured before being reverted: unit-test compile
defects **284 → 292**, five pinned tests regressed on `error [...]` handlers
and quoted `do` bodies. An island is a compiled program with a COUNTED,
ratcheted interpreted span (`islandCeiling` 0 on the corpus), so removing it
buys no honesty and costs a core control-flow family. Stage 9 deletes it,
after those shapes lower natively — the order is the point.

**The contract now.** One outcome. A program compiles and its bytecode runs,
or it does not compile and that is an error naming the construct. Three
refinements the corpus forced, each of which had been getting it wrong in the
same direction — claiming the compiler's failure as the program's verdict:

- a failed CHECK PASS is a compile failure, not the program's error (the pass
  crashes on `each [if [gt 1] ['big'] ['small']] [1 2 3]`, which runs clean
  interpreted). A PARSE error is still the program's own;
- a blocking check DIAGNOSTIC names the compile failure rather than standing
  in as the verdict — the checker flags code the program never reaches;
- a handler's `internal_error` is the PROGRAM's. Only `core.IsVMDefer` marks
  the compiler's.

**Four ledgers carry the debt.** All ratchets on a bug count, never budgets:

| ledger | counts | at |
|---|---|---:|
| `test/go/langspec/compile_failures.tsv` | corpus rows that do not compile, per spec file | 53 |
| `lang/go/compile_defect_test.go` | unit-test programs: do not compile / compile then bail | 284 / 32 |
| `lang/go/test/compile_defect_test.go` | language tests answered on the reference engine | 111 |
| `test/go/langspec/compiled_defect_test.go` | corpus rows that compile and then bail | 52 |

The last is new and was needed: a program that COMPILED and then abandoned the
run used to be re-run, so the lanes agreed and five differential gates saw
nothing. `compiledDefect` classifies; `bookCompiledDefect` counts, and only
the corpus walk calls it, so the ceiling means one thing.

**Two numbers that look like improvements and are not.** Sweep compile
failures 36 → 31 and call-form failures 206 → 200: five seeds were classified
as failures because the classifier read the try-mode fallback's error as a
compile failure. They always compiled.

**What it exposed, which is the point.** Four genuine miscompiles the fallback
had been absorbing: `unresolvable type operand` after an `undef` (twice), a
`STORE_LOCAL` stack underflow in a net-zero `do` body, and **NUR170** (a fn
value read out of a Map and handed to a word taking `(Any, Map)` arrives
TRANSPOSED). **NUR171** and **NUR172** are the fifth and sixth, both at
the same site (`5 $.name apply`) and neither a miscompile. NUR171 is
position-only: the compiled no-match diagnostic carries no source position,
because neither the recorded `PolyNoMatchSpec` nor the debug table has one to
stamp. NUR172 is the two lanes' NOTES describing different argument windows —
`CALL_NATIVE_POLY` holds both operands and reports a type mismatch on the
second, while the interpreter never filled the forward slot and reports an
arity failure over one. There the compiled text is the accurate one, so the
fix points at the interpreter's matcher, not the VM; weakening the compiled
note to match would buy uniformity with a worse diagnostic.

Each is pinned in its own `pinLedger` (`knownPositionLoss`, `knownDiagDrift`),
not in `knownDivergences`: that map is checked by every corpus gate and its
entries must diverge on all of them, while a PRESENTATION drift is visible
only where presentation is asserted — which is the compile-or-fallback gate
alone. Both ledgers carry the same retirement half: a pin that stops drifting
on a full walk fails the gate.

## The sweep that finished it (2026-09-20)

What PR #476 left owing was TEXT and DEAD MACHINERY, not behaviour, and both
are now gone. Nothing in this section changes a gate value.

- **The `vmDefer` messages.** All 28 occurrences across `vm.go`,
  `vm_generic.go` and `vm_rematch.go` said "deferring to the interpreter"
  (or "to interpreter", or "deferred to"), false in every one. They say what the site actually does now — *the compiled
  runtime cannot execute it* — and the one gate pin that asserted the old text
  (`eng/go/compile_pipeline_cov_test.go`) was RE-READ rather than rewritten:
  its stated reason ("the interpreter re-runs and owns the canonical error")
  was false, exactly one test reaches it (`TestFnReturnCountDivergenceParity`,
  a `vm:poly-nout-drift` on `cvar2`), and it is now a ceiling of ONE pinned to
  that site's own text, with the real fix named — a `DeferAlt` at the site.
- **The C1 effect fence was DEAD MACHINERY, not merely a stale comment.**
  `ArmEffectFence` had zero non-test callers and nothing read the count: the
  arms went in #476 and the writer-wrapping half stayed, wrapping writers for
  nobody. The writer half and its three tests are deleted. `EffectLedger` /
  `NoteEffect` survive, re-documented as the observability seam they became —
  security-relevant tests read the count to prove a denied request never
  reached the network. `TestCheckPassIsEffectFree` was re-instrumented on the
  OUTPUT BUFFER, which is strictly stronger than the counter it replaced.
- **~200 stale comments.** Every "whole-program fallback", "sound interpreter
  fallback" and "falls back to the interpreter" that named removed machinery.
  The per-callback path (`InvokeCallback` with no stamped unit → `CallBoru`)
  and interpreter ISLANDS are still live, so those comments were kept — an
  explicit exclusion list, not a blanket sweep.
- **The `refus*` vocabulary: 269 distinct identifier forms → 77, 5447
  occurrences → 740** (measured 2026-09-20 over `[A-Za-z_][A-Za-z0-9_]*`
  tokens; see the correction below — the figures first published for this
  were wrong). Every survivor is an ENTITLED refusal, where the word is
  correct: policy denials, vault and proxy security, weak containers, option
  validation, exit ranges, capability gates, help renders, `await` isolation,
  `del`/process/debugger rejections. **`compiler`, `check`, `eng` and `basic`
  contain the string `refus` exactly zero times.** 131 test names, 50
  identifiers and 6 files renamed; `refusalCeiling` became `failureCeiling`,
  `refusalSiteCeiling` → `compileFailureSiteCeiling`,
  `refusalDispositionCeiling` → `compileFailureDispositionCeiling`; four
  EXPORTED symbols moved (`RefuseCarriedUndef`, `RefuseSpeculativeUndef`,
  `RefuseForwardStackDrift`, `RefuseStrandedMemberFn` → `Decline*`); the
  corpus TSVs' 133 `REFUSES:` descriptions became `DOES NOT COMPILE:`.
  Verb forms became *declines* (a code path declining to lower is a fact, not
  a claim of entitlement); nouns became *compile failure*.

  **THE MECHANISM IS UNTOUCHED, and this was only ever vocabulary:** 92
  `MarkUncompilable` sites and 34 `vmDefer` sites stand, and the ledgers do
  not move. Not one additional program compiles because of this work.
- **The dead `BORU_COMPILE_FALLBACK` references**, including two live
  `t.Setenv` calls on a variable nothing reads.

**A CORRECTION, and the method lesson under it (2026-09-20).** The first
published figures for this sweep — "260 → 65, 4380 → 665, every survivor
entitled" — were WRONG, and the count and the claim failed together for one
reason. The counting regex was
`[A-Za-z_][A-Za-z0-9_]*[Rr]efus[A-Za-z0-9_]*`, which requires at least one
character BEFORE `refus`: every identifier that STARTS with `refus`/`Refus`
was invisible to it, at both ends of the measurement. Hidden that way were
`refuseUndef`, `refuseStrandedMemberFn`, `refuseArrival`,
`refuseForwardStackDrift`, four exported `Refuse*` symbols, and — worst —
two GATE NAMES, which is the first thing the doctrine says to fix. So the
sweep reported itself finished while roughly 185 compilation-sense
occurrences stood, and the "every survivor is entitled" claim was false.
**A measurement that cannot see a class of its subject will report that
class as absent.** Sanity-check the instrument against a case you KNOW is
there before trusting a count — the bug was one anchor character, and it
survived a full review round because every number it produced looked
plausible.

**The method lesson this sweep paid for.** A blanket regex over a vocabulary
is a *refactor of claims*, and it breaks them two ways. It rewrote history
(past-tense narrative describing the OLD behaviour became false), and it
mislabelled: "the `group` word refuses non-string keys" is an entitled
decision, and renaming it a compile failure asserts something untrue — worse
than the word it replaced. Both needed a hand-built exclusion list. It also
hit 40 CODE identifiers (a local named `refusal` became `compile failure`,
with a space), which the compiler caught at once — the cheap half.

**Still owed on this line.** The island machinery, at Stage 9, after the
shapes it covers lower natively — the order is the point.

**A blocker this sweep uncovered: the knowledge graph cannot be rebuilt.**
`make -C kg graph` dies at `pc=8` with `DISPATCH_GENERIC at ev: the walk needs
an evaluation this host cannot perform` — the generic lane's EVALUATING HOST,
which [FULL-COMPILATION-REVIEW.0.md](FULL-COMPILATION-REVIEW.0.md) §2 names as
one of the three unbuilt cores. Verified A/B against a clean worktree at
`ba64e11`: **identical failure on unmodified `main`**, so it is pre-existing,
not this sweep's. `make -C kg verify` still PASSES on an untouched tree, which
is why nobody has hit it: it only bites when a cited document changes, and
then there is no way to make it green again.

This is the fallback removal's first real cost, and it is the intended one
made visible: a boru program that hits a compiler defect can no longer be run
at all, and the KG generator is such a program. The CLI has no interpreter
flag by design.

**The workaround, until the evaluating host lands.** Run the generator on the
REFERENCE ENGINE from a throwaway Go test — `lang.New()` then
`a.RunInterp(string(src))` with the working directory set to `kg/` — and
delete the test afterwards. Two traps, both paid for here: put the harness in
an EXISTING package (a new package changes the go-tree digest the graph
hashes, so the graph you just built is stale the moment you delete it), and
regenerate AFTER every document edit is final.

**The gate is OFF as of 2026-09-20 (maintainer's call).** Gating documentation
edits on a generator that cannot run blocks every doc change in the repo, so
`kg-verify` is deactivated in `scripts/ci-steps.sh` and the matching
commit-gate lane in `scripts/commit-gate.sh`. Both carry the reason and the
one-line re-activation; `make -C kg verify` and `graph` are untouched and
still run by hand. RE-ACTIVATE THEM WITH THE EVALUATING HOST — the gate is
worth having back, and the graph it guards is the fastest orientation in the
repo. CLAUDE.md and AGENTS.md say the same so a fresh session is not misled
into thinking a doc change owes a rebuild it cannot perform.

**Three method lessons this increment paid for.** They are the reusable part:

- **A fence is only as good as the arm it guards.** Removing the effect fence
  before the seams it protected left three fences reading clear and re-running
  unconditionally; the `await` test caught it printing `once` twice. An
  unguarded arm looks exactly like a passing one.
- **A booking that skips work is a fallback wearing a different hat.**
  Teaching a parity helper to tolerate a compile failure stops it reporting a
  compile REGRESSION, which is why every tolerant branch books against a
  ceiling; and injecting that booking as an early `return` in front of a
  helper's interpreter oracle swallows the oracle — six helpers reported that
  the ORACLE had moved when what had moved was the test.
- **A flag's meaning can expire under a census.** The bail census sorted on
  `wasCompiled`, which was equivalent to "did the program survive" only while
  a bail re-ran the whole program. CI called the result a regression; sorting
  on the defect class restored both ceilings EXACTLY (8 and 1), which is the
  proof that only the bucketing had moved.

## NUR174: the landing's SEAT, corrected — read this first (2026-09-20)

The section below describes NUR173's fix as merged in #478. Its mechanism
stands; **its recording site does not**, and [NUR174](../NUR.md#nur174)
replaced it the same day.

NUR173 recorded the landing at the REACH-GROUP COLLAPSE. That made the model a
**whitelist of producers**, and `m get 'f'` — the same member read written as a
word call — had no collapse to see it, so it answered 42 interpreted and `fn h`
compiled exactly as `m.f` had.

The fix was not a second recording site. `noteReStepLanding` is called FROM
`stepLiteral`, in the branch whose next act is `execFnDefLiteral`, and a value
the loop PARKED never reaches it — so the model was already standing where the
interpreter decides. The gate is now the value's own callability;
`CheckState.ReachReSteppedFnIDs`, `recordReachGroupReStep` and the core-side
plumbing are DELETED.

> A model that stands where the decision is made does not need to be told who
> brought the value.

A narrower alternative WAS built before this was believed — recording at
`spliceMatchResults`, the site NUR173 itself predicted — and measured at 20,495
landing ops against the broad gate's 20,710, over one compile of every spec
row (the reach-only gate emitted 4,363). A 1% saving, because nearly every fn-typed
carrier the pass steps arrived from a dispatch splice. The economy argument
evaporated on measurement.

**Three rungs of `execFnDefLiteral` came with it**, each found by a probe
written against the interpreter's source rather than by running the corpus, and
each a wrong answer on its own: the ANONYMOUS-0-ARG PARK (a lambda value that
matched nothing is data — `module-fn.tsv:L47`), a DISPATCH MODIFIER (`m.f/v`
answered 42 against `fn h`), and a value still ALONE INSIDE A LIVE REACH GROUP
(where the group's job is to produce the value, not call it — NUR035). The
first was reproduced under the NARROW gate too, which is what settled the
design: a producer list protected against none of them.

**Engine entries TIGHTENED 422 -> 420** — the park takes two curried-chain rows
off the interpreter that were paying a `RunResolved` entry to reach the same
"stays data". Every other gate is at its ceiling and the sweep did not move.
Twelve new rows in `lang/spec/fn-value.tsv` §9 prove it.

Still open from NUR173's list: a collectable token after the survivor, a
variadic producer's region top, the sweep's two `def container` CRASH cells.

## NUR173 is FIXED, and this page's own first account of it was wrong (2026-09-20)

Read this before picking up the fn-value line. Its RECORDING SITE is superseded
by NUR174 above; everything else below stands.

**The defect.** `m.f` is not a dot operator at the tape level: it lowers to the
REACH GROUP `( m dot f )`. That collapse never parks — an unmarked dot-read of
a function is a CALL (NUR038) — so the rewind lands ON the one value it leaves
and `stepLiteral` RE-STEPS it, dispatching a callable one. The check pass holds
a CARRIER there and steps past it as data. `recordParenReStep` excludes reach
groups (its contract is the more-than-one-survivor case) and every
fn-value-call arm of `resolveDynamicApply` needs a second residual entry, so a
lone survivor reached no arm at all and the program pushed the runtime fn as
DATA. `def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end m.f` was 42
interpreted and `fn h` compiled — silently, on `main`, with no gate lifted.

**Two corrections this page owes.**

1. NUR169 said "no case for `count == 1`". That MECHANISM is right — the seat
   is one function out, at `recordParenReStep`'s exclusion and
   `resolveDynamicApply`'s `len(residual) >= 2`.
2. This page's earlier entry said *"not the paren — bare `m.f` and `(m.f)`
   diverge identically"*. **Both spellings lower to the same paren**, so that
   control varied nothing. A control that cannot vary its variable proves
   nothing, and this is the week's second measurement bug of that shape (the
   first was a counting regex blind to its own subject, PR #478). **Vary the
   axis at the level the MACHINE works at, not the level the source is written
   at.**

**The fix, and why it is a runtime one.** Declining instead — a carrier
receiver whose member type still admits a Function — was built and measured at
**53 -> 282** compile failures: nearly every computed container is `Map`-of-
`Any` to the check pass, so there is no static answer. The decision belongs to
the value, so the collapse records what it alone knows
(`CheckState.ReachReSteppedFnIDs` — the part NUR174 replaced), `check`'s
`noteReStepLanding` NOTES the
producing event as owing a landing, and `OpReStepLanding` — emitted right after
that event's own op — islands an unquoted appliable value ALONE. `Run` over one
token IS `stepLiteral`'s re-step, so a matching signature runs and a
non-matching fn stays data with no no-match raised. The note consumes nothing,
splices nothing and DECLINES nothing, which is what lets it sit last without
disturbing the three models above it.

**It stands aside four ways, and none of them declines:** a collectable token
written after the survivor (the alone-island cannot take it — `Cli.parse
{name:"x"} ["x"]` ran an export's body with its parameters unbound), a
MULTI-result event (which of several the rewind lands on is not this model's
to guess), a variadic producer's runtime-variable region, and a value with no
producing event to hang the op on. Declining those was measured at **53 ->
181** corpus compile failures, because `def x m.y` is the commonest shape in
the language — which is also why the op is emitted at the top of
`seatCallResults`, the one moment the result is on the stack on every path,
promoted to a frame slot or not.

**Cost, measured.** Every gate unchanged — the four ledgers, the 92
`MarkUncompilable` sites, and the generated sweep diffed cell by cell against a
clean-worktree baseline (zero regressions, zero movement). The sweep does not
move because it has no statement-tail member read off an EVENT-RESULT
container. What proves the fix is **twelve new rows in `lang/spec/fn-value.tsv`
§8**, every one of which answered wrongly before it.

**One more trade the gates named, and it is worth keeping.** The landing's first
VM draft islanded every applied value — the interpreter's own one-token re-step,
semantically exact — and the censuses counted 21 extra interpreter entries for
it (engine entries 422 -> 442, interp-entry rows 78 -> 100). An island IS an
interpreter entry, and this project counts every one. The op now takes
`callDynTrailTop`'s ladder instead — native apply, then `dynApplyEnter` into the
matched compiled unit, island only as a last resort — and both gates return to
their ceilings. The last row to come back was `module-rand.tsv:L16`
(`[10 20 30] r.one-of`), which already compiled correctly and was paying an
island for nothing: a module DELEGATION wrapper is asked `MatchFnSig(v, nil)`
here, unlike in `noMatchIfSigged`, because a wrong "no" costs a landing this
model would have skipped anyway where a wrong "yes" costs an interpreter entry
on every read of one.

**Three holes remain, all named in [NUR173](../NUR.md#nur173):** the `get`-WORD
twin (`m get 'f'` is not a reach group, so nothing records its landing), a
collectable token written after the survivor, and a variadic producer's region
top. The sweep's two `def container` CRASH cells (`CALL_DYNAMIC underflow`,
`SWAP underflow`) are still open; the FIRST draft of this fix closed both, so
the shape is reachable from here, but which stand-aside holds them is
unmeasured — and this page has already been wrong once today about a cause it
had not measured.

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
