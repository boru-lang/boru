# Session handover — the full-compilation project

**Purpose.** The one page a fresh session reads to know where the project
stands *right now*. It is deliberately short and deliberately
CURRENT-STATE-ONLY: the per-increment narrative, the measurements and the
lessons live in [FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md),
which is an append-only log and the wrong place to look for "what is true
today". Update this file at the end of every increment.

Last updated: **2026-09-15**, when the maintainer ruled the definition of
done (below, 2026-09-14), the assessment note merged (#453), the disposition census
merged (#455), the generic lane's first slice merged (#456), the poly
seats merged (#457) and the COLLECT oracle merged (#458); the twin-carrier
fix merged (#459); the first ROUTED dispatch (increment 64, #460) is in
review and the native seat (increment 65) is built on it. Increments 1–63
are on `main`.

---

## Definition of done (ruled by the maintainer, 2026-09-14)

> done is a language that compiles, as a developer expects. all valid
> code compiles, no exceptions

Read as a checkable contract, this is T1 exactly as
[FULL-COMPILATION.0.md](FULL-COMPILATION.0.md) §1 states it, with **no
carve-out list**:

- Every program the interpreter accepts produces a `Program`.
  `compile_refused` is not a result, and the `BORU_COMPILE_FALLBACK`
  hatch retires.
- A refusal site has exactly three legal dispositions: a generic
  lowering, a trap that raises the interpreter's own error at the same
  moment, or deletion. "Carve-out" is not one of them.
- "Valid" is decided by the interpreter. The checker may never be the
  reason a program fails to compile, so the whole-program "check
  diagnostics" sentinel goes (Stage 8's T1 half is inside done).
- Computed code is code: runtime-supplied bodies compile too (Stage 7 is
  inside done, and "runtime compilation must itself never refuse" is part
  of the contract).

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

For the forecast (probability of reaching each end state, remaining effort
in session-days, and what remains by tier) read
[FULL-COMPILATION-ASSESSMENT.0.md](FULL-COMPILATION-ASSESSMENT.0.md),
dated 2026-09-14; refresh it at the end of each tier, not each increment.

| gate | value | direction |
|---|---|---|
| `frontierCompileLedger` rows | **29** | down only |
| interp-entry census rows | **28** | down only, fails in BOTH directions |
| refusal ceiling / island ceiling / type-soundness pin | 0 / 0 / 0 | pinned |
| `minCompiledRows` | 6410 | up only |
| `diagnosticParityCeiling` / `armedOnlyCeiling` | 320 / 4 | down only |
| twin-placement frontier shapes | **2** (1 and 2; 3 and 4 graduated) | — |
| `refusalSiteCeiling` | **92** (lowered from 93 to the live value, 2026-09-14) | down only |
| refusal-disposition census (`TestRefusalDispositionCensus`, ceiling 92) | **92 sites: generic 87, trap 1, delete 4**; by retiring stage 3×21, 4×19, 5×26, 6×9, 7×11, 8×2, 9×4 | every site has a one-line row; pinned in BOTH directions, so a retired site lowers the ceiling |
| `engineEntryCeiling` / `deferCeiling` | **281** (was 505, lowered 2026-09-14) / 5 | down only |
| region table (`TestRegionTableWellFormed`, `descFloor` 4000) | **124401** descriptors with increment 61 (76280 with increment 60 and its review corrections, 51372 before it); the floor is unchanged | floor, up only |
| collect oracle (`TestRegionCollectOracle`) | **47633 reproduced** of 72490 executed (floor 47000), `diverged-value` **2** with increment 63 (47627 and 8 with 62 after its review; 47464 before review counted the zero-arg claim); 3 findings ledgered by name (NUR141, NUR143 ×2; NUR140's six retired by 63) | floor up only; the ledger pinned in BOTH directions |

Increments 1–66 are on `main`. The most recent landings: #460 (increment
64, the first routed dispatch), #461 (increment 65, the native seat routes)
and #462 (increment 66, the routed dispatch raises its own diagnostics),
all 2026-09-15.

## What is in flight

**Increment 67, the speculative undef refuses (2026-09-15, built on 66).**
The binder half's first slice, by measurement: NUR144's loop-body undef
was one member of a class — an `undef` of an enclosing binding from inside
any speculative region (a branch arm, a loop, each or while body, an error
handler, a `do` inside a loop, a fn body) is one the check pass keeps in
its model (the wrapped-undef FP class), so the compiled program never
popped the binding and every later read stayed the pass's bake; a `while`
whose condition read the name never terminated. The recorder now refuses
at the carried-undef site (one site, two hooks — the handler passes the
fact, since the recorder's registry can be a module's; NUR144 is resolved
and retired per the register's contract, the class having been NUR145 for
one commit), every row falls back with parity, in-region undefs and
never-bound names still compile, and no corpus row is touched. What the
binder half owes — the placed transition and the live reads — is stated in
the handoff's section and the site's disposition row.

**Increment 66, the routed dispatch raises its own diagnostics (2026-09-15,
built on 65).** A no match, a strict-barrier strand and an unbound slot
are raised from the op's window instead of deferred: the interpreter's
`sigError` / `strandedForwardError` / `undefinedWordError` derivations
moved onto the seam (core/go/region_diag.go) with the engine's methods as
seats, so the error is byte-identical where a defer's fallback could be
fenced into an internal error. Measured after the review of #461 put the
memo's key back, no program reaches these arms today — every such rebind
is diagnosed at check first, the escaped unit's included — so the raise
stands for the shape the check pass cannot see, pinned at the seam and
by the seven shapes that now refuse at check. Every gate unchanged. The
review of #462 found the VM's errors never named the FILE the
interpreter's do (`stampAt` had no file arm, and every stamp site handed
it the program's registry rather than the unit's): every VM error inside
an imported module rendered a bare position; fixed under the
interpreter's rule, pinned on both lanes. Narrative: the
sixty-sixth-increment section of FULL-COMPILATION-HANDOFF.0.md.

**Increment 65, the native seat routes (2026-09-15, built on 64).** The
same routing decision at `RecordCall` and `RecordPolyCall`: a fn-unit
native dispatch with a live word slot over a drivable span lowers
`DISPATCH_GENERIC` with no committed unit, and the op's native arm calls
the live handler when it is in the record's own set — the mono record's
one signature (`GenericSpec.Impl`) or the poly record's live table
(`GenericSpec.LiveSet`, CALL_NATIVE_POLY's discipline, where 660 of the
677 first measured live). Drivability tightened — no list or map literal
in the span, whose contents the interpreter evaluates on arrival. 676
corpus dispatches route at the head (677 before the review's declines;
floor 600 in `TestRegionTableWellFormed`), every gate
unchanged, and no defer site fired over the corpus. Found off the corpus
and closed in the same PR: a routed slot is a dynamic-scope read, so a
routed name joins `routedNames` (the binder's channel beside
`dynScopeNames`) and every frame binding of it — a fn body's `def` before
the call, a param of the name, a top-level loop's carried rebind — lowers
the registry-visible `BIND_DYN_SCOPE` twin the routed read resolves
(both shapes answered the module binding through the frame that
shadowed it); a read of a name a loop already carries keeps its
committed call; and NUR144 records the neighbour that is the binder
half's (an `undef` inside a top-level loop body is dropped). The review of #461 found three more
defers meeting the effect fence and closed them: routing retires only the
escaping latch's note, the memo's key stays (a rebind the check pass sees
re-records the unit); a value-dependent divergent word is never routed;
a lead the dispatch registry does not hold (a module native through its
wrapper) keeps its committed call, and the descriptor carries the
registry its lead resolves in. Narrative: the
sixty-fifth-increment section of FULL-COMPILATION-HANDOFF.0.md.

**Increment 64, the first ROUTED dispatch (2026-09-15, built on 63).**
`OpDispatchGeneric` executes a user-fn dispatch inside a fn unit through
its descriptor when the claim carries a live word slot over a drivable
span: the word is looked up at every execution, the forward tokens are
collected by `CollectForward` and matched by `core.PlanMatch` — the
engine's plan matcher, moved onto the collection seam textually so both
hosts read one implementation — and the committed unit is entered when
the live match is its shape, a live native called, everything else a
named designed defer. Routing retires the frozen note of each read it
makes live, so the memo and the escaping latch stop guarding a bake the
VM no longer consults. Two corpus sites route; the `k` pair routes in
every spelling, the escaped unit included, and the differential, the
coverage triple, the region table and the oracle are unchanged. The
review of #460 found seven ways the op trusted the record where the live
walk could disagree — a body-local callee, a full-stack native, a `/v`
operand, the lead's modifiers, the claim's extent, unit identity without
patterns, an effectful native's result count — each reproduced, fixed
and pinned (the section's "The review's seven"); no corpus gate moved.
Narrative, the census that chose the shape, and the arms: the
sixty-fourth-increment section of FULL-COMPILATION-HANDOFF.0.md.

**Increment 63, the twin-carrier fix (2026-09-15, in review).** The
oracle's first finding that was not the oracle's own, closed before
anything is routed: a root `def` of a COMPUTED compound (`def b [add 1
2]`, `def s (Log.span "m")`) replayed the check pass's MODEL of the
value because the `OpBindGlobal` write-back was gated on the shallow
`IsConcrete`. The gate is now `rootBindWritesBack` (compiler/go/lower.go),
read by both mirrors, and it asks PROVENANCE: a computed value's binding
is exact only for an inert scalar fold; a compound, a carrier, a handle
writes back. The six twin-carrier divergences are gone (their ledger
entries retired; the lane pins the ledger both ways — what remains is
NUR143's two), the differential and the coverage triple are unchanged,
and the class is pinned ACROSS
REQUESTS (`def s (Log.span "m")` compiled, then `Log.end-span s` in the
next request raised span-mismatch before). Review corrected two edges:
the twin's replay skip is now the write-back's own PAIRING
(`BindTransition.WrittenBack`, set by the lowering) rather than a shape
re-derived in core — a written-back compound had been installed twice —
and the scalar exemption is the payload kinds with no interior (a Micron
is inert but has fields). Narrative and table: the sixty-third-increment
section of FULL-COMPILATION-HANDOFF.0.md.

**Increment 62, the COLLECT oracle (2026-09-15, in review as #458).** The
first EXECUTION of the region table: `OpCollect`, emitted under
`compiler.RegionOracle` (off by default, byte-identical bytecode), walks
every descriptor live in the VM with the kernel's own collection routine
and reports through `Registry.ArmRegionOracleHook` whether the walk
reproduces the record; the corpus lane (`TestRegionCollectOracle`, 64s)
tallies 72492 executed descriptors — 47464 reproduced, 16110 declined
(the host's limit), 8911 under-claimed (safe), 7 findings ledgered by
name in both directions. The first walk found and fixed a latent host
defect (the paren span without markers, an infinite loop) and a Phase B
misdescription (top-level loop iterators as live words), and found the
TWIN-CARRIER class (item 1c, closed by 63 — NUR140 resolved). Review (#458) corrected the oracle
three ways — a zero-arg candidate is a zero-length claim, the lane rejects
an error the interpreter does not raise, and agreement is IDENTITY (the
`eq` word's rule) rather than structure — and identity surfaced two more
registered divergences: NUR142 (a refined container is `eq` to nothing,
not even itself; `core.SameContainer` is the identity test exported for
the oracle) and NUR143 (a fn-body read of a module-scope flex is a fresh
clone of the check pass's snapshot). Re-measured: 47627 reproduced of
72490, 8 divergences ledgered (NUR140 ×6, NUR143 ×2), 1 over-claim
(NUR141). Narrative and table: the sixty-second-increment section of
FULL-COMPILATION-HANDOFF.0.md.

**Increment 61, the poly seats (2026-09-14, in review as #457).**
`RecordUserPolyCall` and `RecordPolyCall` claim their Phase-A captures:
the user poly takes the published `(callWord, wordPos)` pair beside the
`(word, pos)` it carried (its `word` is the VM's re-match name, not the
dispatched token; its `pos` is `args[0]`'s); the native poly already had
the word's position as `pos` at every call site and rides `emitCall.region`
with no new parameter. Inert and measured inert: differential 6556 rows,
0 mismatches. The region table goes 76280 -> 124401 descriptors, and a
record-time tally by seat says the native POLY seat (66331 claims) is as
large as the mono seat (65400) while the user-poly seat claims 2 in the
whole corpus. Narrative and table: the sixty-first-increment section of
FULL-COMPILATION-HANDOFF.0.md. Review correction (Codex on #457): a held
offer belongs to its HOLDER — a nested native record under the same
(word, row, col) completes from the pool alone, never the outer user
call's held offer; pinned at the seam and over a two-source program.

**Increment 60, the generic lane's first slice (2026-09-14, in review as
#456).** Phase B's `completeRegion` claims at the USER-CALL seat as well
as the mono-native one: `RecordUserCall` carries the dispatching word and
its token position (a new `CurCallWord` beside `CurCallPos` in the check
state, captured at `BuildFnBodyReturnsFn` entry before body analysis
overwrites the cursor), and `lowerUserCall` appends the claimed
descriptor to `Program.Regions`. The join-key finding that made it
necessary: the event's `pos` is `args[0].Pos()`, not the word's, so the
seat could not have looked its offer up. Inert by construction (no
opcode reads the descriptors), and measured inert: differential 6556
rows, 0 mismatches; coverage 7475 compiled, 0 islanded, 0 refused,
unchanged. The region table goes 51372 -> 76277 descriptors (claimed
28324/84454 -> 55976/143676 slots). The narrative and the full tally
are the sixtieth-increment section of FULL-COMPILATION-HANDOFF.0.md.
Review corrections landed the same day (Codex on #456): the recovery
hook now publishes the word cursor, and the user-fn ReturnsFn HOLDS its
offer at entry (`EmitRecorder.HoldRegion`) because the offer pool is
keyed by row and column only and cannot tell two sources apart; both
pinned; the corpus table moved by three descriptors (76277 -> 76280),
so the seams are real and rare.
Increment 59 (the region-suffix seat, census 29 -> 28) merged on
2026-09-11 as `c34a2fb`; the assessment merged as #453; the census
as #455 (`6ea8ac1`).

**Next, under the ruling above, in order:**

1. The measurement PRs, in two halves:
   - **1a, DONE 2026-09-14** (#455): `engineEntryCeiling` lowered to its
     live 281 (measured three times), `refusalSiteCeiling` to its live
     92, and every `MarkUncompilable` site given one of the three legal
     dispositions in
     `test/go/langspec/refusal_disposition_census_test.go`, gated in
     both directions, pinned at 92 in both directions, and keyed
     syntactically (file, enclosing function, ordinal). The tally is in
     the gate table above. What it says about the plan: 87 of 92 sites
     are GENERIC lowerings, and by retiring stage the weight sits on
     Stage 5 (26 sites, regions: "of unknown provenance" in its many
     spellings), Stage 3 (21, the Apply kernel and universal fn values)
     and Stage 4 (19, the generic dispatch lane and the lookup half).
     Stage 7 owns 11. Only one site is a trap (an `if` condition that
     nets no value) and four delete (an internal invariant, the recorder
     method itself, two fixtures).
   - **1c, CLOSED by increment 63: the twin-carrier class.** A
     top-level `def` of a COMPUTED value (`def b [add 1 2]`, `def l
     (Log.logger "http")`) lowered to `STORE_LOCAL` + `BIND_TWIN`, and the
     twin replayed the CHECK-PASS binding — a carrier `[Integer]`, a module
     prototype with empty fields — because `IsConcrete` read the compound
     as concrete and no `OpBindGlobal` partner was emitted. The write-back
     is now decided by provenance (`rootBindWritesBack`: a computed
     value's binding is exact only for an inert scalar fold), the six
     corpus rows reproduce (NUR140 resolved; the two `diverged-value`
     left are NUR143's), and the class is pinned across requests in
     `lang/go/bytecode_globalbind_test.go`.
     One residual trust: a native that MODELS a scalar result over
     concrete args would keep the model; none is in the corpus, and a
     live read of one diverges by value under the oracle.
   - **1b, OPEN: the lowerer and `Finalize` decline census.** Those
     declines are a different mechanism (a decline reason returned from
     a lowering, not a recorder latch): the site census deliberately does
     not scan them, and FULL-COMPILATION-ASSESSMENT.0.md §2.1 still
     records them as "not re-measured" (78 at the Stage-1 baseline, 161
     reason templates with the recorder's). They need the same
     enumeration and the same three dispositions. About a session-day;
     it does not block item 2 and can run beside it.
2. The generic lane's first executing slice. NOT on the `k` pair and NOT
   through `frozenReads`: Stage 4b (FULL-COMPILATION-HANDOFF.0.md, "What
   this corrects in the plan") superseded the 4a-2 order, the
   program-wide `frozenReads` map is gone, and the `k` pair compiles
   today through the binding-sensitive unit memo. What Stage 4b left
   filed under `OpDispatchGeneric` is §6.9's lookup half for the shapes
   the memo cannot re-record: escaping units, the stored-handler latch
   (`NotifyNameRebound`'s stored-handler arm, where a module-scope def
   site executes only in the check pass), family L's conditional fn
   shadow, and NUR037's fn-local fn. Pick one of those pairs as the
   acceptance test, widen Phase B's descriptors beyond mono-native
   dispatch, then `OpCollect`, then `OpDispatchGeneric` reproducing the
   planner's selection from the descriptor. Land inert first, as the
   twins did. (This item was first written against the 4a-2 order and
   corrected in review the same day: a "next increment" sentence
   inherits its author's last reading, which is process rule 3 below.)
   **Progress:** the inert widening is increments 60 and 61, and the
   first EXECUTION is 62 (above): `OpCollect` walks every descriptor live
   as an oracle and 65% of the corpus's executed descriptors reproduce
   exactly, the rest classified; the twin-carrier fix (1c) is 63, so a
   live read no longer meets a model. The first ROUTED dispatch is 64:
   `OpDispatchGeneric` at the user seat inside fn units, the escaping-unit
   `k` pair answered by the same bytecode across a rebind; 65 gives the
   op the native seat (676 corpus dispatches routed, no defer fired).
   Next on the same op: the diagnostics it still defers (the
   strict-barrier strand, the no-match) by extracting their builders from
   tape state as PlanMatch was extracted; then the binder half.
3. The row-level remainder in parallel only where a row exposes a
   mechanism the lane needs; a row whose fix is a Stage 5 or Stage 7
   slice waits for the slice.

## Increment 58 is PARKED. If you pick it up

The row is `[10 20] each [drop import "boru:math-util" end MathUtil.cbrt 2]`.

**Measured, not guessed.** The bridge sees **1 BindDef twin, 0 def-site
events**. The twin is an ordinary `BindDef` — the frontier note that said
otherwise is corrected. But the event cannot simply be recorded at
`installExports`, because **that runs with the recorder suspended in both
passes**. Any fix starts from three constraints, all found by measurement or
review rather than by reading:

1. **A cached repeat import installs NOTHING.** `ensureExportsBound` guards on
   `!r.Defs.Has(name)`, and `lang/spec/edge-modules-1.tsv` pins the
   consequence (`StringUtil.$module eq StringUtil.$module` → `true`, "a cache
   no-op"). A resident op that installs unconditionally per element is a
   miscompile. This also explains why a TWO-element loop shows ONE twin.
2. **`transplantWordExtensions` is a second twin SOURCE.**
   `core.TransplantExtension` calls `NoteBindTransition` directly, never
   through `InstallDef`, so modules exporting word extensions
   (`boru:time-util`, `boru:matrix-util`, `boru:net`) make twins a
   namespace-install funnel never sees.
3. **Which suspension is it, and what are the two runs?** Both
   `installExports` calls report `recorderActive false`, and only one reaches
   a concrete `EmitState` — with `armResidentDepth 0`. Identify both before
   writing code. If the import genuinely never re-runs under recording, the
   event must be synthesized where the TWIN is noted, which is a *different*
   design from increment 53's, not the same one.

Rejected, with reasons on the PR: minting a fresh module instance per element
(it breaks the cache-no-op row above).

## Other candidates, ranked

- **The remaining region shape**: an inert value on BOTH sides of the run
  (`do [7 for 3 [1] 8]`) declines — the mark plan seats the prefix and the
  suffix arm is a separate screen, and nothing has yet asked them together.
- **Twin-placement shape 2** — a type def in a multi-run body whose expression
  READS the element. Needs an op that REBUILDS the type per element rather
  than re-installing one captured body (`typeInstallElementIndependent` is
  the screen that currently declines it).
- **The remaining 28 interp-entry census rows.** The census header in
  `test/go/langspec/interp_entry_census_test.go` carries its own seam table
  saying where they sit and which are ATTRIBUTED (specified interpretation,
  e.g. `boru:debug`) rather than debt.
- Recorded-not-done: tasks on the fn-analysis memo (NUR128 and the
  pass-scoping), NUR129/130/131's open edge, NUR134, the `codeMintPatterns`
  blind spot, the registry-spawn race.

## Process rules this line has paid for

The first is the one that cost the most, four times in one session:

1. **Re-run the gate that owns the edit — including when the edit looks
   cosmetic.** `make fmt && make vet && make lint && make test`, in order,
   all four. A refactor made *while* fixing something else is itself the
   trigger to re-run everything. Four separate red CIs traced to skipping
   this: the variation lane, `gocyclo`, the ADR-008 statement floor, and a
   stale knowledge graph on a commit that contained nothing but a design
   note.
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

## Instruments

- `TestInterpEntryCensus` — the only thing that sees a compiled program whose
  BODY is interpreted. `-force-compile` reports SUCCESS for those.
- `TestVariationDifferential` — transforms each corpus row; catches what a
  new corpus row's own gates cannot. Runs in `make test` and the merged
  coverage gate, NOT in the fast per-PR checks.
- `TestMultiRunBindParityOracle`, `TestFrontierSpec*`, `TestCorpusThreeModes`.
- A temporary `println` at the decision site beats reading the code. Several
  increments' real causes were found that way and only that way.
