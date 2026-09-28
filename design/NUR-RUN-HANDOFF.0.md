# The reverse-order NUR run — its handoff log (2026-09-25/26)

**Split out of [FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md)
on 2026-09-26**, when that log crossed the repository's 1 MB file limit
(`scripts/check-no-binaries.sh`). These are the entries the reverse-order
run over the Non-Uniformity Register (PR #505) wrote, one per record it
closed, newest first; "the handoff log's entry of that date" in the FIXED
rows NUR.md gained in that run names an entry here. Read it as a
continuation of that log: its doctrine, and every entry before and after
the run, stay there.

## A None read joins its union; run-dependent annotations decline: NUR324-326, NUR311's third path, gate64's fallout (2026-09-28)

Gate64 on 326a0ee was green but for three ledger findings, all fallout of
NUR323's one matching rule: lang/native's three `MatchSignature` stand-ins
were Data-nil values the rule now refuses at a List slot (they are
carriers); the dispatch census's `emit`, `gt` and `lt` divergences stopped
occurring, so their ledger entries went; and specgen's "runs clean, the
checker errors" row `[None] get 0` now checks clean, replaced by an
untaken branch's undefined word.

- **NUR311's third path.** A user call's contract window renders the
  attempted window too; it records which operands were written and the
  prefix it fills from, and the VM stops at a non-concrete written operand
  as the rematch and the poly do: `9 f m.b 3` names 9.
- **NUR324 (an interpreter change).** `7 f m.b` over `x:Maybe` answered
  `[[7] None]` interpreted and raised compiled. The union's Match took the
  None literal, a value at dispatch, for "the type itself", and
  `rejectsTypeLiteral` admitted it only at a None slot; the inline
  `(m.b) is (Integer tor None)` was already true. The union now admits it
  where it admits `none`, so both lanes answer `[7 [None]]`. The ruling is
  invited in the record. Refusing it on both lanes was the alternative, and
  would need a None-literal carrier distinct from `none`'s: returning the
  None node itself from the missing read collided in the emitter's
  provenance (every None literal shares the node's ID), and a strict None
  carrier cannot tell the literal from `none`.
- **NUR325 (loud).** `x:(1 add 2)` is the value pattern 3, but the check
  pass ran the annotation over carriers, built an Integer slot, and the
  bind twin replayed that fn, so `f 4` answered `[[4]]` compiled; `(typeof
  5)` the same over a Type carrier. A carrier annotation with no type
  content now notes NUR308's run-dependent latch, and `fn` declines as a
  compile-time word: no new decline site.
- **NUR326.** `x:(tnot Integer)` fell to ResolveSigType's TAny tail on both
  lanes and bound 5. It constrains through the pattern now, and the pass
  keeps tnot's exact negation as the pattern instead of a carrier its match
  admitted 5 at.

## A type value is no value of its Parent: NUR323 found and closed, NUR311 closed (2026-09-27)

Probing NUR311's reach-led windows over other type-valued members found
silent wrong answers, recorded as NUR323 (silent on main too). `def m {e:
Integer} end sub m.e 3` answered `[3]` compiled for the interpreter's
`signature_error`; so did `m.e add 1`, `f Integer` into an Any or Type
parameter, `if c [Integer] [5]`, `[m.e] get 0 add 1` and an `each` element.

- **The model.** A type literal's Parent is its supertype: the `Integer`
  node's is Number. Every check-mode widening of a value to "a carrier of
  its Parent" made the type a Number value, and `sub` committed its Number
  handler over it. `core.ValueCarrier` widens a type literal to a Type
  carrier and the None literal to the None carrier. The member and element
  reads, the generic unit's argument, the join, the list element type and
  element join, the typed list, the fold seed, clone, the spec undef, the
  unit memo and the kept read take it.
- **Strict, not gradual.** A gradual Type reached `mini`'s Function slot
  (Function is a Type), and `mini`'s check model then errored. A strict
  Type carrier takes exactly the Type and Any slots, as the run's type
  literal does. `is` and `typeof` fold nothing over it, as over `typeof`'s
  own Type carrier: `m.e is Number` stays true.
- **The VM's matcher.** `core.MatchSignature`'s positionalMatch refused a
  type literal only at Map and List slots; the interpreter's plan refuses
  it wherever `rejectsTypeLiteral` does. So `x add 1` over `f Integer`
  re-matched add's Number overload at run time. It now refuses the same
  slots. A poly in a fn unit that meets a type with no faithful raise plan
  is the loud `vm:poly-no-match` defer; compiling its raise is owed.
- **NUR311.** The interpreter's report walks the written operands and stops
  at the first that is no concrete value, then falls to the stack prefix.
  The VM rebuilds that tuple (`core.AttemptedTuple`) from the rematch's
  recorded prefix (`NoteRematchPrefix`, `DispatchSpec.Prefix`) and from the
  poly plan's `StackTuple` (`PolyNoMatchSpec.NFwd`). `mini m.e 'ab'` agrees
  byte for byte. The first cut deferred instead, which surfaced an
  `internal_error` where users had the right error with different notes.
  The plan-less best-effort report keeps its full window (`9 sub m.e 3`).
- **Pins.** lang `TestNUR323TypeValueIsNoValueOfItsParent`,
  `TestNUR311TupleStopsAtATypeOperand`, `TestNUR229To231OnTheMergedTree`;
  eng `TestRematchStoppedTuple`, `TestPolyNoMatchRaiseStopsAtATypeOperand`;
  core `TestValueCarrier`; compiler `TestW8DispatchRematchDeclines`.

## The merge of main's #517: guarded fast paths over the branch's dispatch (2026-09-27)

Main's #517 added a poly inline cache, seeded picks, shape specialisation
of Map/List params, and proven narrower returns under a declared `Any`. Six
conflicts, each resolved to keep both sides:

- **CALL_NATIVE_POLY** takes the seeded pick and the cache first. A window
  none of them matches still asks the interpreter's plan (NUR242's
  `polySplitRaise`), then NUR147's arity retry. `polyDispatch` takes the
  window size it pops, since the retry narrows it below `pr.Arity`.
- **PolyRef** keeps `Split` and `DynBodyPlain` beside `Seed`/`SeedTags`, and
  the region's `PolyNOutRegion` count.
- **Probes** over the new fast paths agreed or declined loudly: fn values
  nested in shaped Maps and Lists, the cache across operand types, and
  seeded picks over fn units.

Gate63 on the merge was red on four checks:

- **Three branch pins** met shape specialisation. `f {k: …}` specialises m on
  the map's shape, so `do m.k` lowers as a checked `CALL_NATIVE`, and a
  gradual read is strict there. The disassembly pins run with
  specialisation off and also pin the specialised form. The two-value map
  run keeps its generic-unit defer and agrees on the specialised lane.
- **Type soundness 7 -> 8.** Main's narrowing made `def rpt fn [[] [Any] [do
  [def Big Integer 15 is Big]]]` Boolean, but a `do` traps its body's raise
  into an Error value, and rpt's second call answers one. A fn body
  holding a `do` narrows nothing (`bodyTrapsErrors`). Main has the same
  hole; no row of its corpus reaches it.

## NUR317 closed: an undecided condition's union takes the re-step (2026-09-27)

The second half's re-step left one form silent. Over an undecided condition
the pass keeps the join's union carrier in the `do`'s outputs, and nothing
re-stepped it: `h true` over `[[c:Boolean][Integer][do [if c [l/v] [0] 5]]]`
raised the frame's count error over `fn l 5` for the interpreter's 6, and
`def c m.c do [if c [g/v] [0] 5]` over a map member was `fn g 5`.

- **A candidate, not a flag.** The program may take a union output itself,
  through a residual's runtime-conditional apply (`do [if c [l/v] [0]] 5`
  lowers `CALL_DYNAMIC /1`) or a guarded landing after the call (`do [if c
  [l/v] [0]]`). So a union output makes the call a candidate
  (`eventFlags.unionReStep`), and the lowering decides.
- **In a fn unit** whose residual takes no apply of its own (no body-tail
  apply, frame replay or `apply` chain), the call takes `SigRef.ReStep`.
- **At the program** the residual's apply is resolved only after the
  events lower. The call takes its own SigRef, and `flagRootUnionReSteps`
  flags it once the residual resolves to no apply.
- **A noted landing** after the call (`landingAfter`) keeps every re-step
  flag off: the landing re-steps the one value itself. The first cut missed
  this and deferred `def c true do [if c [l/v] [0]]`, one of NUR317's
  closed-half rows.
- **Pins.** The union rows of `TestNUR317DoReStepsItsResults`; compiler
  `TestUnionReStepCandidates`.

## A `do`'s results re-stepped where it stood: NUR317's second half (2026-09-27)

The interpreter's `do` hands its body's whole residual back to the step
loop, which re-steps it where the `do` stood. A placed fn value the body
left (an `if` body arm's one survivor) dispatches there, over the results
after it. The check pass's own run of the body decides a constant `if`
(`ReduceStaticIf`) and dispatches the taken arm's fn inside the body, so
the `do`'s modelled outputs are the re-step's (`[Integer 5]`). Nothing the
program recorded after the call applied the fn the compiled body handed
back: `do [if c [g/v] [0] 5]` was `fn g 5` for `7 5`.

- **The flag.** Such a call carries `SigRef.ReStep`. It applies to a
  closure `do` whose compiled body may leave a fn value
  (`fnUnitRec.mayReturnFn`) while no modelled output may be one
  (`noteClosureReStep`). It also applies to the same body when the
  closure declines and the dyn-body backstop takes it (the probe's
  `noteBodyReStep`).
- **The prefix seat.** A dyn body's run seated beneath its prefix whose
  outputs may hold a fn gets its own `SigRef`, flagged by
  `seatRegionPrefix` when that seat takes the residual. `1 do [if c
  [(mkf)] [0] 5]` was `1 fn g 5`: the first half's mark window re-stepped
  only a lead, and the prefix hid it.
- **The VM.** `doReStep` re-steps the results through the island where
  that is exact: every value it dispatches takes no argument, or nothing
  lies beneath the results in the frame and the unit ends after the call.
  Anywhere else, or at another count than the program seats, it is a
  designed defer (`vm:do-restep`).
- **A regression the ledger caught.** The first cut flagged every run the
  region plan armed, and `7 do [if true inc/v [2]]` bailed. The residual's
  own trailing apply takes that run, and the plan is armed before the
  residual knows it, so the prefix seat flags the call's own `SigRef` only
  when it seats.
- **Open, silent.** A fn body over an undecided condition keeps the join's
  union in the `do`'s outputs, and nothing re-steps it: `h true` over
  `[[c:Boolean][Integer][do [if c [l/v] [0] 5]]]` raises the frame's count
  error for the interpreter's 6. A union output cannot take the flag, since
  the program applies one itself in other layouts (`do [if c [l/v] [0]] 5`
  lowers `CALL_DYNAMIC /1`), and that apply is resolved only after the
  events lower.
- **Gate61's fallout on the merge.** Main's specialisation closure pushed
  `BuildFnBodyReturnsFn` to gocognit 174. The unit compile is now a
  method (`fnUnitCompile.compile`). The compile-defect ledger is 339:
  `fnvalue_param_render_test.go`'s `app sq/v` compiles through a
  specialised unit with parity. `lowerCall` stays under gocyclo through
  `siteSigRef`.
- **Pins.** lang `TestNUR317DoReStepsItsResults`; eng `TestDoReStep` and
  `TestTakesArgsAtPointer`; compiler `TestNoteClosureReStep`,
  `TestValueMayBeFn`, `TestBodyReStepNote`, `TestReStepsResults` and
  `TestDisassembleReStep`.

## The merge of main's #516: call-site specialisation on a branch where a bare fn name calls (2026-09-27)

Main's #516 numbered its new records NUR234–NUR236, the numbers the branch
had used; the branch's three became NUR320–NUR322 in the merge, and main
keeps its own. The code conflicts compose both sides: a user call's event
carries the branch's word position and main's argument window, a fn unit
seats main's spec guards beside the branch's bound slots, and `CompiledFn`
carries the branch's refused fn reads beside main's guards and fallback.
The fallout, fixed in the merge:

- **A fresh literal has no identity to guard.** The branch pushes a fn
  literal with a fresh identity each time a fn or closure unit runs it
  (NUR288), so main's identity guard on such a literal never holds, and
  its specialised unit only ever took the fallback. A literal passed from
  inside a fn body's analysis or a closure unit does not specialise
  (`freshLiteralContext`). A `/v` reference and a root literal still do.
- **A bare fn name calls (NUR078).** Main's first two graduated rows,
  `(k inc)` and `(k g)`, pass a fn by its bare name. On main that is the
  fn (`[4]`); on this branch the name calls at every slot, so the
  interpreter calls `inc` over nothing and raises `signature_error`. Both
  compile paths decline it, as the generic path did before. The graduated
  list takes the `/v` spellings, which specialise and answer `[4]`.
- **Two generic pins graduate.** NUR246's `[[7 (5 f/v)]]` over a fitting
  closure and NUR249's branch-arm `[[(k 5)]]` over `c/v` compile through a
  specialised unit and agree (`[[7 6]]`, `[[7 5]]`). Their decline and
  raise pins now run with specialisation off.
- **Main's records on the merged tree.** NUR234 is loud here: the
  branch's NUR123 close models the bare `g` as its call, so the generic
  unit declines where main answered `[20]`; the `/v` spelling is `[20]` on
  both lanes. NUR235 and NUR236 stand as main recorded them.
- **Pins.** check `TestSpecialisableFnArgFreshLiteral`; lang
  `TestApplyShapesBareFnWordArgDeclines` (both paths and the interpreter's
  code, with NUR234's row) and the NoSpec pins with agreeing rows in
  `nur246_test.go` and `nur249_test.go`.

## A sweep of placed fn values under re-steps: NUR318, NUR319; NUR313's placed data (2026-09-27)

A sweep crossing eleven placed-fn producers (branch arms, parens, `/v`
reads, a member read, `case`, `do`, `for`) with twenty-three contexts that
re-step or collect found nineteen silent rows. Two were regressions the
first cut of NUR317 made (it had dropped `callResultPlaced`'s branch case):
`5 if c [(mkf)] [0]` and `[if c [(mkf)] [0] 5]`. The case is back over
`branchPlacedBy`, which reads the fragment's own event, so a placed branch
is data wherever a user call's parked result is. With it, the NUR313
lead decline is unreachable and gone. `if c [(mkl)] [0] 5` now compiles as
the data it is. NUR124's re-step deopt skips a placed branch too, which
closes the fn-body form (`fn l 5` listed as 6, present on main).

- **NUR318 (present on main).** `def m {f: g/v} m.f/v` fired g. The reach
  group's `/v` marker rides after its close, and the arrival models fired
  an all-0-arg member on landing. They stand aside for a marker
  (`modifiedAsData`). A quoted fn carrier reaching `dup` is the concrete fn
  value's decline (`quotedFnCarrier`). A fn body's replay leaves `/v`
  values as they stand.
- **NUR319 (present on main).** `def r (if c [g/v] [0]) r 5` left the read as
  data. A def read of a placed branch's union is a re-step
  (`unionLeadReStepped`, `mayBeFnUnsettled`).

Four rows stay: NUR317's open half (two), and two forms that now decline
loudly (`5 m.f/v`, `m.f/v dup`). Pinned by `lang/go/nur318_319_test.go`,
the rows added to `lang/go/nur312_314_test.go`, check `TestModifiedAsData`,
and compiler `TestNUR318QuotedFnCarrier`, `TestNUR319UnionReadReStepped` and
`TestNUR317PlacedBranchNotReStepped`.

## Re-steps that undo a placement: NUR313, NUR314 and NUR317 (2026-09-27)

Gate58 on 9835ff5 (NUR312–NUR314) was green except gocyclo: the NUR313
declines had pushed `resolveDynamicApply` to 71. They now live in one
helper, `branchLeadDecline`, called once after every apply arm; a placed
branch's value leading a residual apply declines loud in the carrier and
branch arms too, and the function sits at 69.

Probing NUR313's placement under the words that re-step a value found
NUR317 (present on main): a `do`, whose results the step loop re-steps at
the call, and a paren with two or more survivors undo a body arm's
placement. The pass's join of a data arm and a fn arm is a union carrier
that no callable test read (`core.UnionMayBeFn` now does, at the landing
and at the paren's re-step mark); the delivery count took a `do`'s re-step
for the branch's own when the branch ran suspended inside the body
(`branchPlacedHere` now also asks `parenReSteppedFn` and
`inResidualToCallerUnit`); and NUR271's settled-lead rule seated every dyn
body's dynamic lead under its own sibling as the body's to settle, where a
lead the body parked is the caller's to re-step. No model fact separates
the two (nor a body whose count differs from the model's), so a settled
lead now re-steps from the mark where Finalize armed the window
(`dynBodySettledOp`, `OpCallDynMixedFromMark`): the island steps the body's
actual results as the caller does. A paren-re-stepped union lead applies
conditionally (`unionLeadReStepped`), and the NUR313 lead decline asks for
an arm that may leave a fn (`placedArmsMayBeFn` — `[0] [9]` never does). Twelve forms agree now; `(if c [(mkf)] [0] 5)`
declines, loud.

**Open, silent (NUR317's second half).** `do [if c [g/v] [0] 5]`, `do [if
c [l/v] [0] 5]` and `do [if c [g/v] [g/v]]`: the closure compiles the
body, the pass models the `do`'s outputs AFTER the caller's re-step (it
dispatched the concrete g — `[Integer 5]`), and the compiled `do` returns
them before it. The fix is the caller's re-step at run time, at the `do`'s
return — NUR314's loop-exit shape: on the island where the results are
isolated, a designed defer elsewhere.

NUR314 gained one defer: in a fn unit a named fn result's re-step meets the
frame's tail, where the interpreter raises for a no-match and the island's
tape just ends (`def f fn [[][Any][for 1 [(mkl)]]] end f`,
`namedDispatchingFn`). Pinned by `lang/go/nur312_314_test.go`,
`lang/go/nur317_test.go`, core `TestUnionMayBeFn`, compiler
`TestBranchLeadDecline` / `TestPlacedArmsMayBeFn` /
`TestNUR317PlacementUndone` / `TestDynBodySettledOp` and eng
`TestNamedDispatchingFn`.

## Container element typing: NUR315 and NUR316 (2026-09-27)

A research sweep of how the check pass types container elements (asked:
could a container that only ever holds one type be read as `[:T]` and shed
its deopts?) found two silent answers, both present on main, confirmed on
the interpreter differential:

- **NUR315.** A flex container's element types are joined over the writes
  the pass sees on the container; a user fn it is passed to may write
  through its parameter's alias, which the body analysis over a plain
  carrier never records. Passing a shaped flex container to a user fn now
  poisons its shape and every shape reachable from it
  (`PoisonFlexShapes`, `StoreShapeInfo.Poison`, `KeysPoisoned`), so reads
  after the call take the dynamic(Any) hatch.
- **NUR316.** `each`'s result typed its elements by the body value's type
  and dropped its gradual mark; so did `fold`, `for`, `while`, `window`,
  `pairs`, and the closure compiler's list callback inputs, which read a
  typed list's child strictly. A later body committed a direct op over a
  String or a None. `core.CarrierTypedListOf` keeps a gradual element as a
  dynamic child, and `check.ElementCarrierOf` hands a body that child
  gradual; the body re-matches at run time.

The same sweep's answer to the question itself: a known non-fn child type
already drops the landing and `DEOPT_IF_FN` after a literal-key read of a
declared `[:T]`, and a declared `[:T]` param's `each` body compiles its
dispatch direct; a computed-key read discards the child type. Plain
List/Map are immutable, so a per-value element join at construction is
sound; flex containers need the alias analysis NUR315 stands in for. Pinned
by `lang/go/nur315_316_test.go`, core `TestCarrierTypedListOf` /
`TestStoreShapePoison`, check `TestElementCarrierOf` /
`TestPoisonFlexShapes`.

## Fn values the interpreter parks or re-steps: NUR312, NUR313, NUR314 (2026-09-27)

A differential sweep over factories that return fn values through `Any`
(a named no-argument `g`, an argument-taking `l`, an anonymous lambda)
found three silent answers, all present on main, where the compiled lane
and the interpreter disagreed about whether a fn value is stepped:

- **NUR312.** A forward-drift window's placed value rode into the island
  inside a paren (NUR287), and a fn that takes nothing fired inside it
  before the paren could park it: `2 (mkf) add 3` answered [2 10] for the
  interpreter's signature_error. The island starts after the last placed
  value now, over the window beneath it as resolved inputs
  (`placedWindowSplit`, `runIslandResolved`); a value beneath it that is
  not plain data is a designed defer (`vm:mixed-placed-fn`).
- **NUR313.** `if` splices a body arm inside its own paren, which parks a
  one-survivor fn; the check pass re-stepped the joined value and the
  lowering landed or applied it (`def c true if c [(mkf)] [0]` was 7 for
  `fn g`). A branch whose value-yielding arms are all one-value bodies is
  placed (`branchPlaces`: no landing note, `callResultPlaced` for a
  fn-capable value); a branch with one such body and a value arm lands on
  the value arm's own path (`splitLanding`, `lowerArms`,
  `lowerComputedBranch`); a split branch whose placed arm may leave a fn
  declines where it leads a residual apply (`splitArmMayBeFn`, through
  the residual lowering's own reason, no new site).
- **NUR314.** A loop's end splices its results back and steps them; the
  compiled loop kept a dynamic fn result as data (`for 2 [(mkf)]` was `[fn
  g fn g]` for [7 7]). At the loop's exit (`FOR_NEXT` done, a break's
  `flowSignal`) a result that would dispatch is settled as the interpreter
  settles it (`loopExitReStep`): an isolated loop — nothing beneath its
  results in the frame, its exit the end of its unit — steps them on the
  island; any other is a designed defer (`vm:loop-result-restep`). `for 1
  [(mk0)] 5` defers where it answered before (loud; its compile is owed).

The first cut of NUR313 counted every body-armed branch result as parked,
and `TestNUR292StatementIslandRunsTheList` declined (a string result "above
a literal"); placement is a question only for a value that may be a fn, and
the rule asks exactly that. A research sweep of container element typing
the same day found two more silent records, pending: NUR315 (a FlexList
written through a fn parameter's alias) and NUR316 (`each` drops its body
value's gradual mark, so a later body commits a direct op). Pinned by
`lang/go/nur312_314_test.go`, compiler `TestBranchPlaces` /
`TestSplitArmMayBeFn`, and eng `TestPlacedWindowSplit`.

## A handler island's do over a computed or def-bound body (2026-09-27)

NUR301's and NUR300's count islands took the do before the handler only
over a literal body, so `(do b error [drop 5 6]) add 1` deferred where the
literal form answered. `doBefore` (shared by `handlerRun` and `islandRun`)
admits any body whose token is the one after the do (`doBodyAfter`): a
computed body by its argument site, and a closure the pass compiled from a
def-bound list's read or a paren by the site `RecordClosureCall` now keeps
in `closureBodySites`. Not in `argSites`: noting a closure call's argSites
made `callRun` plan runs over literal do bodies, and NUR286's and NUR298's
landing rows met NUR297's guard (measured, then scoped). Pinned by lang
`TestNUR301HandlerRunsNotSeatedAsOne` (seven more agreeing rows) and
compiler `TestDoBeforeComputedBody`.

## A root container's position: the whole-program list literal compiles (2026-09-27)

A program that is one list, map or paren parsed to a container with no
position: the val rule sites it with its opening token, but both parsers'
root switch dropped `rootPos` for the explicit list, typed list and map
branches, and passed the root paren unsited. Every consumer that finds a
statement by its token lost the program: NUR222's root list-literal do and
NUR301's `[do [raise oops 'x'] error [drop 9 8]]` deferred where `1 [...]`
answered. Both parsers stamp the root container now (parser/go `Parse`,
parser/ts `convert.ts` — the TS twin in lockstep, its suite at 100%), and
`shape.tsv` pins the root list, map, typed map and paren positions (two new
rows, 28 -> 30 in both runners). Both programs answer on both lanes.

## NUR301's owed count island: a literal handler's two-value run answers (2026-09-27)

The checked single seat (5cb5591) deferred where a literal handler's
island left two values. NUR300's error count island now covers the island
too: `countPoint` takes a strip island whose threaded value a do left over
a literal body, with the literal handler after the word (`islandRun`), and
writes the four tokens as the run. The lowering seats the island in
`Program.FallbackCounts` beside the span's `CheckOne`; the VM hands a run
the check refuses to `stopRestart` instead of deferring. `1 [do [raise oops
'x'] error [drop 9 8]]`, `(do … error [drop 5 6]) add 1` and the fn form
answer on both lanes. A first-token list, a do in a loop and a statement
that also binds keep the loud defer. Pinned by lang
`TestNUR301HandlerRunsNotSeatedAsOne` (eight agreeing rows, three defers)
and compiler `TestIslandCountPoint`.

## The merge of main's #515: NUR232 and NUR233 closed, NUR229–231 answered on the merged tree (2026-09-27)

Main's #515 numbered its new records NUR225–NUR233, the numbers the branch
had used; the branch's nine became NUR302–NUR310 first (9ef107c), and main
keeps its own. The merge (7f9828d) carries both sides; its fallout, fixed
after it:

- **Two fixes for one bug.** Main's NUR226 and the branch's NUR237 closed
  the same silent read (a split-bound name rebound in an arm, read as the
  program's result) two ways: main seats the join from a registry read and
  installs the arm's store; the branch made every later root def of the
  name registry-visible. The merged tree keeps main's seat and hands arm
  stores to main's install (`loopSplitRebind` skips an arm-carried def);
  the seat's carried-undef decline gives way where the undef exposes the
  split's own binding (`splitUndefExposesLive`), whose miss on the skipped
  path is the interpreter's `undefined_word` at the read's own position.
- **NUR232, general.** Main's install covered split names; every root arm
  store installs now (`rootArmInstall`), because the join's twin replays a
  carrier and nothing else left the arm's binding for the next request.
  The merge surfaced it: the branch lowers `dup` into locals, so main's
  `def x (5 dup drop)` row is no split here and fell into NUR232.
- **NUR233.** A static region's first-value bind over results the planner
  promoted to locals binds from its local (`isPromoted`); the splice found
  the stack empty.
- **NUR301's single seat.** Marking every strip island a region declined
  `def ok (do b error [drop false])`, the mini-s3 shape that compiled
  before (a regression from c7d0243, unpushed). A strip over a demoted
  region is one value again (`settleDemotedStrips`), and a strip island is
  a checkable region: a single seat takes its run under the runtime count
  check (`FallbackSpan.CheckOne`), deferring loudly on a two-value path.
- **NUR229–231.** Answered on the merged tree by the run's case and
  dispatch work; NUR231's no-match notes still differ, recorded as NUR311
  (loud).
- **Test fallout.** Main's tests that collided by name with the branch's
  are renamed (both run); main's predicate tests use `fnpred` (NUR099); the
  bridge test pins main's NUR227 return contract; main's runtime-bind test
  calls `RecordRuntimeDispatch` (the branch's NUR231 generalisation).
- **Pins.** lang `TestNUR232RootArmInstall`, `TestNUR237SplitUndefAfterAJoin`,
  `TestNUR229To231OnTheMergedTree`, `TestNUR233PromotedSplitBind`; NUR301's
  test (four checked-seat defers, three agreeing rows); compiler
  `TestRootArmInstall`, `TestSplitUndefExposesLive`,
  `TestSettleDemotedStrips`.

## NUR301 found and closed: an error handler's caught run seated as one value (2026-09-27)

Probing NUR300 found its literal and nested twins, silent on main and here.
`[do [raise oops 'x'] error [drop 5 6]]` answered `[5 [6]]` for `[[5 6]]`, a
def's group bound the top value, a value beneath went above the run, and
`[do [do [raise oops 'x'] error (mk)] error (mk)]` answered `[5 [6]]` too.

- **The island.** A literal handler the closure path refuses runs as an
  interpreter island (`TryRecordFallback`), which appends whatever the
  interpreter left. A strip word's island is a region and a run now
  (`variadicRegion`, `regionMayBeFn`, `dynBodyRun`), so a fixed consumer
  declines loudly.
- **The nested do.** A computed handler's run is a run (`dynBodyRun`), so
  a `do` whose literal body ends in one is a region. Where the closure path
  sees the run but declines the shape, it notes the run for the dyn-body
  backstop (`noteBodyRun`), as its capture arm did. A strip over a growing
  region stays one.
- **Compiled on the way.** A value beneath a computed handler's run takes
  the prefix island: `3 do [raise oops 'x'] error (mk)` is `[3 5 6]`, and a
  handler's fn value re-steps over the 3 (`[103]`). NUR300's value-beneath
  pin, a loud decline, now agrees.
- **Pins.** `lang/go/nur301_handler_island_test.go` (13 agreeing rows, 7
  loud declines); NUR300's value-beneath row flipped.

## A call run over its stack operands: NUR296's and NUR222's owed forms (2026-09-27)

An effect before a stop that took its operand off the stack (`"x" print/s`)
had no call run, so its island would run it twice, and the stop deferred.
NUR222's count island met the same refusal at `1 add 2 [1 do […] drop]`.

- **The run.** `callRun` now takes the tokens right before the word that
  hold the arguments it took off the stack, the top nearest the word, as
  well as the ones after it it took forward (`10 sub 3` is one run of three
  tokens). A stack operand an event left must come from a paren, or from a
  word whose own call run ends right before this one (`runBefore`): `3 4
  add print/s` is one run of four tokens.
- **Nested plans.** A paren's own plan inside a call run is the run's
  (`substPlan.holds`), so the island writes the run once and the paren
  never.
- **Answers.** Every such form prints once and answers as the interpreter
  does, output compared: `"a" print/s 5 if (mk) …`, `3 4 add print/s …`,
  `(g) print/s …`, the def-group and fn-body forms, and `1 add 2 [1 do
  […] drop]` (`[3 []]`).
- **Still loud.** An operand beneath the statement (`"x" end print/s`), or
  one written apart from the word (`"x" "y" print/s print/s`, inside a
  def's group).
- **Pins.** The six pins whose loud row was `print/s` each gain an
  agreeing row and take `end print/s` (or the apart form) as their loud
  row; compiler `TestCallRunOffTheStack`, `TestSubstPlanHolds`.

## NUR300 found and closed: a computed error handler's run is a region (2026-09-27)

Probing NUR282's owed handler count found it silent, on main and here. A
computed error handler's run leaves 0-or-more values the interpreter splices
and steps; the compiled lane marked the result with the shrinking variadic
mark and seated it as one value. `[do [raise oops 'x'] error (mk)]` over a
handler of `[drop 5 6]` answered `[5 [6]]` for `[[5 6]]`. A def's group
bound the top value, a value beneath was seated above the run, a map
literal took one value, and a handler's fn value stayed data.

- **The fix.** `recordDynBodyCall` records a computed handler's run as a
  variadic region, as it does a computed `do` body's
  (`StripsUnconsumedInput` joins `BodyOutResidual` in the region test). A
  residual seats the run whole, a single-value seat takes it under the
  runtime count check, and a layout that cannot place it declines.
- **What stays.** A run of one plain value answers as before, and so do a
  loop's and a branch's arms and a fn's return-count error.
- **The count island.** Where the check refuses a run, the statement
  runs again with the run written over the do, its body, the word and the
  handler (`handlerRun`); the events inside those four tokens plan nothing
  (`inSpan`). The list literal, the def's group, `(do … error (mk)) add 1`
  (`[5 7]`, which the old lowering reached by accident) and a handler's fn
  value all answer as the interpreter does. A map literal's value, which
  no island reaches, defers.
- **Pins.** `lang/go/nur300_handler_run_test.go` (22 agreeing rows, the
  map literal's checked defer, the value-beneath decline), compiler
  `TestErrorCountPoint`, `TestCountPointNeedsARun` and `TestInSpan`.

## NUR282's def-group seat compiles (2026-09-27)

`def f fn [[b:List][Any][def ok (do b) ok]] end f (quote [5 6])` raises f's
return-count error interpreted. The do's count island was planned, and then
`planDeopts` dropped it with every point of the unit: each def among the
names an island spells must bind registry-visibly, and a do's run has no
re-pushable home. But `ok`'s def stands after the token its island resumes
at, so the island makes it itself.

- **The rule (`markIslandMadeDefs`).** A def at the unit's top level after
  the latest island's token, of a name no closure child seeded, is
  island-made (`emitDynBind.islandMade`): every island runs it again before
  it reads the name, so it takes no registry-visible bind for them and asks
  no re-pushable source. The mark is per def site; the first cut dropped
  the whole name (`dropIslandMadeDefs`), which an earlier def of it
  refused.
- **Answers.** A run of `[5 6]` raises the interpreter's return-count error,
  `[]` answers `undefined word: ok`, and `[g/v]` answers `[7]`: the island
  writes g in the do's place and steps it.
- **Still loud.** A gradual read of the name after the group has an island
  of its own that resumes at the read and reads the name from the frame;
  there the def must bind, and the run's checked value has no re-pushable
  home yet (`def ok (do b) ok add 1`, `def ok 1 end def ok (do b) ok`).
- **Pins.** `TestComputedDoBodyCheckedOneDefers` (two defer rows now agree,
  two new loud rows) and compiler `TestMarkIslandMadeDefs`.

## NUR282's wrong-count single seat compiles (2026-09-27)

`(do b) add 1` over a two-value body raises the interpreter's return-count
error. The compiled seat took the run as one value under a runtime check
and deferred on any other. The do's count island (NUR222's) now covers a
computed body too. Where the check refuses a run and the island seats, the
statement runs again with the run written in the do's place, and it answers
the interpreter's error.

- **The body.** `countPoint` takes a computed body, a read or a paren,
  whose one argument site is the token right after the do (`doBodyAfter`).
  The dyn-body backstop now notes its call's argument sites.
- **The check.** The VM uses a pure check (`dynBodyOneRefuses`), since the
  defer's own check notes a bail.
- **A regression on the way.** The stop's own run marked its do for the
  stash (`substSeq`), which stashed a region's result and broke NUR210's
  `do (mk) end x` shapes. The stop's own run is written from the stop, so
  `substSeq` skips it.
- **Pins.** Two rows of `TestComputedDoBodyCheckedOneDefers` agree now, and
  NUR207's `j j` row compiles (af33f18).

## NUR282's `j j` compiles (2026-09-27)

A def-bound name over a factory's fn value dispatches at each read, and the
interpreter parks what a boru fn returns. So `def j (mk) end j j` over `[]
=> [42]` is `[42 42]`. The compiled residual declined the first result as a
dynamic value that might apply to the entries above it. The one result of a
def-bound name's dispatch is placed now (`claimParked`), and its apply
carries the claim (`DynMethodSpec.Parks`). The claim is set only where the
residual relies on it, recorded through the DynMethods index each shaped
apply lowered to (`dynMethodAt`), so no program that compiled before
changes. The VM defers on a callee that is not a boru fn, a native whose
result the interpreter might step on (`parksResult`). gate52 found
`parksResult`'s `len(fd.Signatures) == 0` unpinned in the arity gate: an
emptiness test of the overload list, like NUR238's, where the count of params
never enters. `eng/go/vm.go` is pinned at 24 (75aed2d).

## The register resolved: NUR296 and NUR282 by the scope ruling (2026-09-27)

With NUR296's effect form compiled, the two records still OPEN were NUR296
and NUR282, and neither holds a silent form. Re-probed on be11252: NUR282's
arg-taking lambda runs and its wrong-count seat defer loudly, `j j`
declines, and every NUR296 form either compiles or defers. By the scope
ruling a refusal is no answer divergence, so both are FIXED by their loud
declines and defers, as NUR250 and NUR223 were. The compiles stay owed and
are listed in each record.

- **NUR296.** A stop on a later iteration of a counted loop or in a
  condition loop, a paren substituted inside a loop, and an effect that took
  an operand off the stack. `for`'s `[start end step]` range makes a later
  iteration's continuation expressible.
- **NUR282.** The arg-taking lambda run, the wrong-count single-value seat,
  a computed error handler's count, a rebound parameter or fn, and `j j`.

NUR255's Status line read OPEN beside a FIXED index row and a fix
paragraph; it reads FIXED now. No record in NUR.md or the archive reads
OPEN.

## NUR296's effect form compiles; NUR299 asks the top operand (2026-09-27)

**NUR296's effect form.** An effect before the stop in its statement (`print
"a" 5 if (mk) ["big"] ["small"]`, `print "a" [(l.0 true)]`) ran in the
compiled code and would have run again in the statement's island, so the
island deferred. The recorder now keeps where each argument of a call was
written (`argSites`: a read by its site, a computed value by its event, a
literal by its token). A bare call whose arguments fill the tokens right
after its word, in written order and nothing else, is a call run
(`callRun`). The island writes it as the run it left: an effect as no token
(`RestartNone`), a one-value call as that value. The effect prints once on
both lanes, output compared. A written run is no collection barrier to a
later one (`writtenOver`), and a list literal never collects forward
(`barrierFree`). The first also let NUR222's count island take `(g) 1 do
[...] drop`, which is `[5]` on both lanes now. A call that took an operand
off the stack (`"x" print/s`) is no run and still defers; the pins that had
held `print "x"` as their loud row hold that now. A call run's value is
placed, as a paren's is. The interpreter parks a returned fn value, so one
that would dispatch where the island writes it defers (`RestartSubst.Placed`).
An adversarial read of be11252, before its push, found the island stepping
it: `mk2 5 [(l.0 true)]` over a List-taking lambda answered a type_error.

**NUR299 asks the top operand.** gate48 (0475f76) was green but for one
corpus row: `Log.dump 0 get "trace-id" get` (module-log.tsv) declined, since
the drift guard's all-stack latch asked whether any stack operand was
unproven, and dump's Any result lies beneath the `0`. A concrete top is
what a later candidate's stack slot meets too, as the check side's drift
rule has it, so the latch now asks the top operand only. The row compiles
again; the one-operand window is its own top.

## NUR222's consumer compiles; NUR299 keyed on the drift models (2026-09-27)

**NUR222's consumer.** A word that consumed a caught body's phantom as its
operand (`1 do [(1 add 1) drop] drop`) bailed with a CALL_NATIVE underflow:
the pass models a body that may raise as the Error it would leave, and a
clean run leaves nothing. The do's call now checks the run's count at run
time (`SigRef.CountCheck`, `CountClaim`). A miss re-runs the statement on
the interpreter from its first token, the do's count island (`SigRef.Count`,
`stopRestart`): the events before the stop are written as their values, and
the do word with its body list as the run the call left (`RestartResults`),
so the body never runs twice. `notePhantomConsumers` marks the consumed do
events over the root, every fragment and each unit as it closes (a unit is
planned long before the root, which first left the unit form deferring).
The planner takes a do over its literal body's closure, outside loops, with
only stepless values before it on its level (`countPoint`). The seats no
island takes defer loudly (`vm:do-count`); among them is `(g) 1 do [(1 add
1) drop] drop`, which answered `[1]` for `[5]` silently on main and here. A
do inside a top-level list literal defers too: the literal's token carries
no position, so no statement island can find the do (every island kind
shares this).

**NUR299, keyed on the drift models.** gate47 on ebe6e34 was green but for
the arity gate: NUR299's cut asked a one-operand all-stack match by its
parameter count, which ADR-016 forbids. The case is the forward-drift
guard's (NUR305's record says an all-stack match is that guard's), so the
guard now asks it directly. Where neither the drift window nor the check
side's decline (`DeclineForwardStackDrift`, which now reports whether it
declined) takes an all-stack dispatch, `allStackWindowAmbiguous` latches the
split when a stack operand is unproven and a later candidate collects past
the word. A suspended region's dispatch asks nothing, since its body runs
as an island (`do [mk mk add 1]` compiles as before). The arity pin is back
at 8.

## NUR296's consumed forms; the barrier rule; NUR299 found and closed (2026-09-27)

**NUR296's consumed forms.** A paren's value the statement consumed before
the stop, as in `[(g) drop (l.0 true)]` or a branch guard's condition or
unselected arm, left the island nowhere to read it. The lowering now keeps
a copy in a slot of its own, stored and pushed back right after the call
(`stashSubst`, for every event a planned island substitutes: `substSeq`),
and the island reads the slot where the value is held nowhere else
(`restartSubstSrcs`).

**The barrier rule.** Stashing gave the bare-word substitution a source,
and `if true [(m.f y)] [0]` went silent: `[43]` for `[42 42]`. A function
word is a collection barrier and a value is not, so an overloaded `m.f`
fires its zero-argument overload at `y`, where the island's `(m.f 42)`
took the 42. A word-led run written as a value (a bare call, or a do and
its body list) is written only where every token before it on its level is
a scalar literal (`inertBefore`); a paren is collectable either way. The
same hazard sat latent in fa2f00c wherever such a value was held.
`[(m.f k) (l.0 true)]`, which that substitution had answered, defers now.

**NUR299.** Probing the barriers found a one-operand split. `(mk) do [5]`
over a factory of `[1 2]` is `[[1 2] 5]` interpreted: the List misses
do's Map overload, and the List overload takes the `[5]`. The check pass
filled the Map overload from the stack with its Any carrier. That answered
`[1 2 [5]]` on the branch, silent since 13aa881's region seat dropped the
poly claim, and bailed on main. Over a member fn it raised `cannot call
do` on both. NUR305's guard skipped all-stack matches, leaving them to the
forward-drift guard, which takes only multi-operand words. It now asks a
one-operand all-stack match too, and the compile declines. A first cut
that asked every all-stack match declined the drift window's own forms
(`mk mk add 1`, `(5 do [(mk)]) add 1`), which answer at run time.

## NUR286 closed; NUR297 and NUR298 found and closed (2026-09-27)

**NUR286's compile.** A guarded landing over its own values beneath takes
the statement island (NUR242) where one is planned, at the root and at a
unit's own depth (`planLandingRestarts`, `planUnitRestarts`). A landing
with no word after it carries only that island (`seatWordlessRestart`).
The island must not run a `do` twice, so a do over a literal body is read
two ways (`doBody`). A body of reads runs again whole. A body of one call
ran in the compiled code, so the island writes the do's result over the do
word and its body list, a run of two tokens (`RestartSubst.Span`). The
interpreter puts a do's result back on the tape in the do's place and
steps it there, as it steps the token. The result is the landed value, read
off the stack's top: its promotion's store comes after the landing
(`landedIdx`).

**NUR297 (the branch's own bug).** The island stepped a paren's written
value as a token. The interpreter parks a paren's value that would
dispatch (`fnReturnPark`), so a paren's fn value applied over the values
beneath where the interpreter placed it: `[5 (g) (l.0 true)]` over a
factory of a lambda answered `[[6 true]]` for `[[5 fn (Integer) true]]`,
from b89fbfa. Such a value is a designed defer now
(`vm:restart-parked-fn`); data is written as before. The runs are written
last to first, in token order (`pathLess`).

**NUR298.** Inside a def's operand group the interpreter's re-step applies
a landed fn to the literal after it before the def takes the group's
first value. The compiled lane left the fn to the residual arms, which met
it after the def: `def j (5 do [(mk)] 7) end 1 j` answered `[fn (Integer)
7 1 5]` for `[8 1 5]`. That is silent on main too, and a string or list
after the fn is the same. A bisect put the plain `end j` form's regression
at 51b1a62: NUR266's def-read crossing refused the residual's apply over
the 7 because j's later read sits past the `end`. The crossing now counts
only the entry right after the lead. The check pass notes a def group's
landing as a COLLECTING one (`LandingNextCollect`, `inDefGroup`). Its op
(`LandingCollects`) re-steps nothing itself, joins the root guard, and
takes the statement island where no arm applies the value; with an effect
before the stop it defers (`vm:landing-collects`). A first cut noted every
collecting landing; it broke paren-bounded member calls (`(fs.b 10)`) and
the prop specs, whose applies other models own, and was narrowed.

**Found on the way.** `stampDeoptRet` stamped a unit's landing islands only
when the unit had no deopt points (an `||`), so `[5] each [m get "f"
drop]`, whose each body has both, ran an island with no RET. Both stamps
run now, and the three each-body forms compile: the bail ledger is back to
31. gate45 (on 1510ad9) was red on that ledger alone (34 against 31).

## NUR296's loop forms, NUR286's fn-body form and NUR289's caret (2026-09-27)

**NUR296's loops.** A stop inside a counted loop restarts the loop's
statement when it fires on the loop's FIRST iteration: the island runs the
loop from its start, so no earlier iteration may have run. The VM checks
it at run time (`RestartFirst`, `firstIteration`): each enclosing loop's
index slot still holds its constant start (`firstIterGuard`; a condition
loop keeps the invariant-read rule, `restartSpanReruns`). On that first
iteration an event WRITTEN after the stop has not run, whatever order the
pass recorded it in — a body's `def q 1` after the landing was recorded
first (`restartRunsReadsOnly`'s position filter). A paren substituted
inside a loop would carry one iteration's value into every other, so a
loop's call still blocks the island (`loopFree`). `for 2 [[(l.(i) true)]]`
and `for 2 [[(l.0 true)] def q 1 end]` compile; a branch guard in a counted
loop takes the same island; a stop on the second iteration is the designed
defer `vm:restart-later-iteration`.

**NUR286's fn-body form.** `def g fn [[][Any][(5 do [(mk)]) typeof]] end
g` failed on the unit's return count — an error the interpreter does not
raise. A fn unit's own-depth landing over its own values takes the root's
guard (`guardUnitLandings`) unless an event applies the value, its call's
frame parks it, or the unit's whole-frame replay re-steps it — an entry of
the residual's top `dynFrameW`, by the event or by its promoted slot
(`frameReplays`; the first cut missed the promotion and broke two forms
that answered 6). A unit with a tail apply chain or a trailing apply keeps
today's arms.

**NUR289's caret.** The walk raised the strict-rule strand at the word
after the landing; the interpreter raises where the landed fn stands on
its tape. `LandingWord.ValPos` carries the position the recording pass saw
the value at, else the landed call's (`landedPos`): `do [mk] typeof` and
`do [(mk)] typeof` agree to the column. `do [l.0] typeof` lands at the
read on the interpreter, whose value carries the read's token, where the
pass holds a carrier: the caret there points at the `do`.

## NUR292 closed: a branch guard's statement island, and the paren substitution (2026-09-27)

**The mechanism.** A branch guard (`__condguard`, `__codeguard`) defers on
a computed condition or value arm that is a list the interpreter runs as
code — a condition inline over the values beneath the `if`, an arm spliced
in parens. Its STATEMENT now runs again on the interpreter, the island
NUR242 built for a landing: the planner (`planGuardRestarts` at the root,
`planUnitRestarts` in a unit, both through `guardPoint`) keys each guard by
its branch and kind (`guardKey`), and the lowering seats the island on the
guard's own `SigRef` (`SigRef.Restart`, a `StmtIsland`) where the walk
proved it; the VM's CALL_NATIVE takes it on the guard's designed defer
(`guardRestart`).

**The paren substitution.** A call before the stop no longer blocks the
island when it opens a paren whose value the compiled code still holds: a
paren seals the stack off, so the call took nothing from outside it, and
it leaves exactly its value when its first token is the call and it takes
every later token as one operand (`parenOf`). The island writes that value
in the paren's place (`restartSubsts`, `RestartSubst`, the VM's
`substIsland`), read where the stop holds it — the guard's own operand,
a promoted slot, a frame-region entry (`restartSubstSrcs`, `heldAt`) —
through parens and list literals (a token path, `substToken`). The same
serves the landings and paren applies: NUR296's first witness, `[(g) (l.0
true)]`, compiles.

**What it took, measured by the probes.** The branch's recorded position
is its condition VALUE's, which an earlier statement may have written
(`def c (mk) end 5 if c …` restarted the `def`): the statement and the
walk's depth now anchor at the `if` word (`CondCheckPos`,
`restartAnchor`). The both-computed branch lowerings keep their operands
on the walk's stack past the jump, so an arm guard there read stale
indices — loud, never silent, as the VM refused them — and now sees the
stack the run holds on its path (`emitCodeGuardOver`). A 46-program sweep
over the guard and landing forms: 34 agree, 10 stay loud by design (NUR296),
2 are the check pass's own refusals, none silent. gate43 on 41efce6 was
green but for gocyclo (`Finalize` at 72 over 70), fixed here by moving the
restart stamping out (`stampUnitRestarts`, `stampRootRestarts`).

## NUR242 and NUR219 closed: the statement island; NUR296 recorded (2026-09-27)

**The mechanism.** A root landing whose `/q` claim has no compiled answer
— nested in a list literal or a branch arm, over values beneath, before a
wider residual — or a paren apply whose method is data at run time
deferred. The interpreter can run the whole STATEMENT again from its first
token whenever the compiled code ran nothing a second run would repeat, so
such a stop restarts its statement (compiler's `landing_restart.go`,
`LandingWord.Restart`, `DynMethodSpec.Restart`, the VM's
`statementRestart`). The plan is made at Finalize for the root and at a
unit's finish through the deopt environment every point shares: the
statement's first token (`statementToken`), no operand deferred past it
(NUR207's `deoptDeferred`), only re-runnable events before the stop
(`restartRead`: member reads, shuffles, a literal's assembly, island-pure
words). The walk seats the stack's depth at the statement's first op
(`noteRestartDepths`); a unit's island seats its unnamed params from their
slots, and the root's the program residual's earlier entries from wherever
it keeps them (`RestartSrc`: a constant, a slot, a stack entry).

**What it took, measured by the probes.** A fire before the paren apply,
and data before it, restart at the landing, before the word's call runs.
Code bodies restart in their parent's frame; loops only over a
loop-invariant stop — a read of constants or slots no enclosing loop steps,
no bind in the span — which fires on the first iteration or never. Two
wrong turns the probes caught before they landed: a promoted earlier result
dropped from the island's prefix (`(mk) end 7 m.f true` answered `[7
true]`, silent), and an each body's `drop` re-run without the element (a
unit's unnamed params are read at the statement's start, not the stop's).

**Also.** The landing's `uncalled_function` raises at the value's own
token (NUR219's caret sibling). What no island can run again — a user call
before the stop, an index-dependent read in a loop, a bind in the loop's
span — is NUR296, loud.

## NUR283 closed: the report over an inexact layout; the shuffle-only body counts its depth (2026-09-27)

**NUR283.** A committed optimistic bake (and a poly) raised the word's
report only over an EXACT layout — a boundary on both sides of the
operands — because the VM rebuilds the interpreter's tape from the
operands alone. The interpreter's plan also reaches what lies between the
operands and each boundary, so the layout now carries it
(`layoutSurround`): constants beneath (`Beneath`), scalar literals after,
closed by a bare function word (`After`). The VM lays the whole tape out
(`planSplitOver`); a plan that takes one of those tokens is a dispatch the
program never assembled, and defers (`vm:split-plan-reaches`). `0 fold
[add] (mk) 7` lists the 7 on both lanes.

**Gate41 (620926b) was red, three ways.** (1) NUR222's shuffle-only claim
admitted `do [drop]`: `do` runs its body isolated, so `drop` finds nothing
and raises, which `do` nets as an Error — the claim now simulates the
body's stack (`shuffleEffect`) and admits only a body that never takes
past its own values and ends empty; `def f fn [[Integer] [Any] [do [drop]
7]] end f 5` compiles and raises the interpreter's count error again. (2)
The aritygate: shuffleOnlyBody's overload-list guard (native_control.go
1 → 2) and the list re-step's quote-slot reads (vm_list_restep.go, new,
2), both argument-rule sites. (3) The compile-defect ceiling 342 → 341:
`[1 2 3] each [ do [ 9 drop ] ]` and `[do [3 drop] 7]` compile; the NUR242
decline row is `[do [3 4 add drop] 7]` now.

## NUR222's witnesses compile: a shuffle-only `do` body has no phantom (2026-09-27)

A value-less `do` body is latched as the one Error a caught raise would
leave, because the pass cannot prove the body never raises — and a clean
run leaves nothing, so a consumer of the phantom bailed (`1 do [1 drop]
drop`, CALL_NATIVE underflow) and its fixed seats declined. A blanket
consumer decline had been reverted: it also declined a loop that always
raises. The body of the witnesses, though, is literals and plain stack
shuffles, and `do` runs it isolated: each shuffle meets only the values
the body pushed, exactly as in the pass's run, which raised nothing. Such
a body (`shuffleOnlyBody`: the closed shuffle set, still the registered
all-Any natives) nets what the pass saw, so no latch is armed. The
consumer answers `[]` and the two fragment witnesses `[2]`, compiled. A
body with any other word keeps the latch (`1 do [(1 add 1) drop] drop`
still bails — the open part).

## NUR219's root and list forms closed; NUR295 found and closed (2026-09-27)

**How it was found.** Probing NUR219 (`m.f true`, a `/q` fn read from a
factory's map) in the shapes around it: `[m.f true]` answered `[[fn h(Atom)
true]]` — and so did `[m.g 5]` for a plain `g`, `[[fn g(Integer) 5]]` for
`[[6]]`. Any gradual fn element of a list literal was assembled as data;
a folded literal map's member was the one shape that answered.

**NUR295.** The interpreter evaluates a list literal, so a fn element
re-steps over its neighbours. The recorder flags such an element
(`listReStepSpec`: possibly a fn, not placed, not a def read) and the list
lowers to `OpMakeListReStep`: data assembles as before; a fn re-steps the
window through the island when the elements after it are literals and the
others step as themselves (the island over the VALUES is then the list's
evaluation of the TOKENS); anything else defers. The element's landing
stands aside for the list (`eventApplies`, `listOwnsLandings`) — without
it, NUR286's guard deferred `[5 m.g]`.

**NUR219.** The landing now carries a COLLECTED word (value-bound, or the
reserved `true` / `false`), whose claim target seals over the folded
value's push. A fn quoting its first slot enters over the atom — `m.f
true` is `[true]`, `def k 2 end m.f k` `[k]` — and a list's island takes
the word (`ListReStepSpec.Words`). The unsealed forms (values beneath, a
wider residual, a paren, a fn body) are a designed defer now instead of
`uncalled_function`.

## NUR292's plain list forms compile; NUR294 found and closed (2026-09-27)

**How it was found.** Gate39 (the NUR290–293 batch) went red in two
places. The bail ledger: module-sift.tsv L112 compiled and then hit
NUR292's guard — boru:sift's column dedupe, `if (ArrayUtil.member [k]
acc) [acc] [push k acc]`, whose condition is `member`'s Boolean MASK
(`[true]` / `[false]`). The interpreter runs it inline and branches on
its last value; the compiled branch had read the mask's truthiness,
always true, so the dedupe never pushed a column — silent on main, the
row's `size` hiding it. And the generated sweep: `__arm`'s seven cells
had no seeds (a code-body word is a sweep row, internal or not).

**NUR292's plain forms.** The inline run over a list of plain values
(numbers, strings, booleans, atoms, lists) only places them: the branch
reads the last and drops the rest, and an empty list raises "if:
condition produced no value". The condition's guard became its own word,
`__condguard` (`BranchRecord.CondCheck`), answering exactly that; an arm's
one-value plain list is that value (the paren splice places it), which
`__codeguard` answers. The "no value" error had no position on the
interpreter (`stepMoveIf` spliced the move out before the error read the
pointer); both lanes raise it at the `if` now. A list that dispatches (a
word, a fn value) keeps the defer, and so does an arm list of another
count.

**NUR294.** Probing the arm forms: `9 if true (mk) ["f"]` over a
List-typed `[1 2]` answered `[1 9 2]` — the root's trailing apply rotated
the arm's run as one value. Wider: `9 do [do (mk)]` the same, `[9 do [do
(mk)]]` `[1 [9 2]]`, `do [do (mk)] add` a signature_error for 3, a
literal body taken by the backstop with the same results — a RUN (a
computed body's values) was one value wherever it was not the dyn-body
call's own result. `runOperand` now names a run wherever it surfaces (a
dyn-body run not proven one plain value, a closure call whose unit leaves
one — `closureRun`, a branch an arm of which is one, `error` over one);
the trailing apply declines it; a closure body holding one is a region
(one out: `variadicRegion`, so the region rules — prefix seat, collect,
the fixed-count declines — apply) or declines; a closure that declines
before its probe (a homeless capture) probes anyway and tells the
backstop (`noteBodyRun`), which records the literal body as a computed
one. `__arm` runs the compiled closure the VM hands it for a literal body
(its sweep seed answered the closure). Sweep: 377 run shapes, 0 silent.

## NUR282's gradual-body region: `do (mk)` over an Any result compiles (2026-09-27)

The sweeps' loud rows included `do (mk)` over a declared-Any result that
returns two values: `poly dispatch do: result count 2 differs from the
recorded claim 1`. The recorder already marks such a run a variadic
region (the dyn-body backstop's poly arm sets the same flags as the
List-typed CALL_NATIVE's), and the lowerer seats it as one; only the poly
op's result-count claim stood in the way. A region's poly op now commits
no claim (`PolyNOutRegion`; the VM skips the count check for it), so the
region's own seat rules own the count, as for the CALL_NATIVE twin. A
fixed consumer still declines; a computed error handler's count stays a
defer (its result is variadic but no region).

## NUR292 found by a probe sweep of conditionals; its silent half made loud; NUR293 closed (2026-09-27)

**How it was found.** A sweep of conditionals and code-body words over a
user fn's Any result (240 programs), then of computed `if` arms (126):
eight silent rows, all a list where `if` runs code. The arm is main's.

**The divergence.** `if` runs a list condition inline — its words take the
values beneath the if — and splices a list arm in parens, whatever
produced the list; the compiled branch held the list as a value (`if
(mk) …` over `[false]` answered "t" for "f"; an Any arm over `[1 2]`
answered `[[1 2]]` for `[1 2]`).

**What landed.** A value condition or value arm the pass holds abstractly
that may be a list (not an arm typed List, which keeps its own path) is
guarded by the lowering: `__codeguard` runs over the condition before the
jump and over an arm's value on the path that takes it, passing any other
value and raising a designed defer on a list. A first cut put the guard in
the pass's model as a `[v __codeguard]` body; the guard's call result
landed inside the arm, so a flex member fn arm raised where the
interpreter lands it at the merge (NUR280's pin caught it), and the guard
moved to the lowering. After: 0 silent of 192; the list forms are loud.

**NUR293, found on the way.** A List-typed computed arm compiled as `[do
<arm>]`, and `do` traps a body error as a value where the arm splice
raises it (`if true (mk) ["f"]` over `quote [1 div 0]` answered
`error(division by zero)`). It runs through `__arm` now, `do` without the
trap.

**What is open.** Compiling the list forms: the condition's inline run
over the live stack, and the arm's paren splice once its list-or-value
choice is made at run time.

## NUR291 found and closed by a probe sweep of type words (2026-09-27)

**How it was found.** A sweep of type words over a user fn's Any result
(`as`, `unify`, `is`, `make`, typed defs, `convert`, `typeof`, `case`,
`teq`, the comparisons): one silent row of 196, `case (mk) […]` over a
list. The arm is main's.

**The divergence.** `case` runs a list scrutinee as a code body and
dispatches on its last result; the compiled desugar matched the list
itself (`o` for `i`), for a fn's result and for a parameter alike.

**The fix.** Where the pass holds the scrutinee abstractly and it may be a
list, the desugar records case's scrutinee step as a run-time call ahead
of its chain: `__casesubject` (forward form) applies CaseHandler's rule;
`__casestack` (stack form) passes a non-list and defers on a list, where
CaseHandler would read the clause list as the scrutinee. A first cut
that declined instead put nine corpus rows (`error [dot code case […]]`)
back on the compile-failure ledger and was replaced before it landed.

## NUR290 found and closed by a probe sweep of typed defs (2026-09-27)

**How it was found.** A sweep of typed defs, an annotation over a body the
check pass holds as a carrier (a user fn's Any result, a member read, a
list element, a Number or Scalar result): 127 of 420 answered silently
wrong, 91 of 119 over the Any result alone. The arm is main's.

**The divergence.** `def x:Integer (mk) end x` answered `[Integer]` where
the interpreter answers 42, and `[String]` where it refuses. Unify over a
carrier wider than the annotation takes the narrower side, the
annotation's own type content, and the typed def's general arm bound it;
a fn-shape or negation annotation admits a carrier outright, and the run
bound its value unchecked.

**The fix.** Unless the carrier's own type proves membership (a static
carrier under a plain lattice node), the pass binds a carrier and records
OpBindTyped over `TypedBindRunMembership`, the run's unify with the
interpreter's refusal. `Any` is the untyped def. A value that may be a fn
under an annotation a fn may inhabit declines through the
compile-time-word arm (the census holds: no new site). After: 0 silent of
420, 193 declines where there were 197.

## NUR288 and NUR289 found and closed by a probe sweep (2026-09-27)

**How they were found.** After NUR287, three combinatorial sweeps ran over
both lanes, printing only programs that compile and answer differently:
3360 word × prefix × forward-operand programs, 736 fn-valued factories in
landing and apply contexts, and 180 control-flow and shuffle shapes over
dynamic values. The first two came back clean once NUR287's halves landed;
the third found two silent divergences, both present on main.

**NUR288.** A fn literal in a fn body is a new function per evaluation,
and the compiled body pushed one pooled const (`mk mk eq` true for false).
An unbound, capture-free fn literal materialised in a fn unit is
freshened like a compound body literal, and the VM's fresh pushes
re-identify a fn value (`WithFreshFnIdentity`).

**NUR289.** The landing's walk planned a fn value's authored signature,
whose `BarrierAllForward` scanned nothing forward, so `do [mk] typeof`
over an Any-slot lambda parked the fn where the interpreter strands it
(strict rule). The walk plans over `installedSigView`.

## NUR287 found and closed: a forward-drift window over dynamic operands only (2026-09-27)

**The divergence.** Probing NUR282's `j j` found `def j (mk) end j j add
1` answering `[84 1]` compiled for `[42 43]`, and the shape needs no fn at
all: `def mk fn [[][Any][42]] end mk mk add 1`. The check pass's match over
two dynamic carriers took `add (Bytes, Bytes)` all-stack (it precedes
`[Number Number]` in the overload list), so the poly re-match never saw the
written 1; over three carriers it took `add (Map, Any, Service)` and the
claim failed at run time. Silent, present on main.

**The fix, as recorded.** The forward-drift window already models a
forward-eligible word matched all-stack under a dynamic top with a literal
after it (the island re-steps the verbatim tokens), and the decline it
mirrors fires where the window cannot; both required a concrete operand
beneath the top. A dynamic top is the whole precondition now. Probing the
window's reach found `[7 mk add 1]` answering `[7 [43]]` (present before):
a list literal's element run passes the TERMINAL gate on its own tape, but
the literal assembles a fixed count, so the window stands aside inside an
inline context region (`InInlineCtxBoundary`) and the decline answers.

**Measured.** Ten shapes agree (two and three carriers, `sub`, `add` with
nothing written, a concrete operand on either side, `if` and `do` bodies, a
String pair, `j j add 1`); `[mk mk add 1]`, `[7 mk add 1]` and `mk mk add 1
drop` decline. The check suite, the compiler suite and the commit gate's
lanes pass.

**The word half (11b0572).** Probing the window's reach found `def k 1 end
mk mk add k` answering `[84 1]` too: the interpreter's forward phase
collects a value-bound word as its value, but both guards recognised only
a literal after the word. Core's `ForwardOperandValue` yields the literal
itself or a plain word's bound literal at the word's position, and the
window carries that value (`def s "x"` likewise, `[42 42x]` on both lanes).

## NUR284 closed: a folded member fn applied in a binder's frame reads its dynamic scope (2026-09-27)

**The divergence.** `def m {c: ([x:Any] => [k])} end def h fn [[][Any]
[def k 7 m.c 1]] end h` is 7 interpreted — the member's lambda reads `k`
from the def stack where it is applied, inside h — and was `undefined
word: k` compiled; so was the param form, and `run m.c` (a fn that applies
its argument) raised a count error, then NUR218's `is still waiting for 1
argument(s)`.

**The fix, as recorded.** The stored unit read `k` live already
(`LOOKUP_DYN_SCOPE`), but h bound `k` as a frame local. Each stored-dep
live read keeps its name and read position (`storedLiveReads`), and
`Finalize` (`seatStoredLiveReads`) puts a name into `dynScopeNames` when
some fn that binds it reaches the reading value (`AnonScopeReachable`,
NUR257's binder model), so every binder installs it registry-visibly.
NUR218's interpreter route (`fnReadCallUser`) seats the CALL_USER's
arguments as resolved values beneath the word (`runIslandResolved`), where
it stepped them as tokens.

**Measured.** The three witnesses and eight neighbours agree; a typo and a
dropped binding stay the interpreter's `undefined_word`, where the check
pass stops. The lang root, compiler and eng suites pass.

## NUR286's silent root half made loud: a landing over its own values that nothing compiled re-steps (2026-09-27)

**The divergence.** A paren whose `do` run lands an argument-taking fn
over the paren's own values is the interpreter's re-step of that fn over
them: `def j (5 do [(mk)]) end j` binds 6. The landing never consumes a
stack operand (NUR175's rule), and where a def took the group's first
value (the 5, the fn spilling), a later word took the fn, or the island's
flat window collected a value written after the group, no compiled apply
re-stepped it: `[fn (Integer) 5]`, `[5 Function]`, `[5 10]` — silent,
present on main.

**The fix, as recorded.** The first guard (a root landing over values
beneath that no apply takes) fired where a def-bound read's re-step had
merged its own beneath into the value's note, and six lang tests broke.
The cut that held keeps the value's FIRST note apart (`landingOwn`: what
followed, beneath, the id; never a def-bound read's), records the root
landings whose own step had values beneath (`noteRootBeneathLanding`),
and once `Finalize` knows the residual's apply guards each one that the
interpreter does not park (`callResultPlaced`), the residual apply does
not re-step as the interpreter does (`residualApplies`: a trailing lead;
a leading lead alone; a window entry last, under a function word, or
whose own step collected a value-bound word), and no event applies
(`eventApplies`). The VM's landing raises a designed defer under the
guard when the fn takes an argument (`landedFnTakesArgs`,
`compiler.ClosureTakesArgs`).

**Measured.** The witnesses raise; `(5 do [(mk)])`, `5 do [(mk)]`, `(5 do
[(mk)]) add 1`, `3 (5 do [(mk)])` (one- and two-argument lambdas), the
factory of 7, the nullary lambda, `def j (5 (mk)) end j`, `7 m.f k` and
the curried factory's `2 r 3` answer as before. The lang root suite, the
compiler and eng suites pass. Over-approximation, pinned: a fn that would
not match the values beneath is loud too (`("a" do [(mk)])`).

**Open.** The compile (a runtime-variable region a def could take), and a
fn body's form, which fails on the unit's return count.

## Main's NUR217 closed, NUR285 found and closed, NUR286 recorded: a shuffled fn def and a fn def read inside a body (2026-09-27)

**The divergences.** Of NUR217's six witnesses, four already agreed; the
stack shuffle's (`def j ((mk) dup drop) end j`) was an internal error, and
so was `def j (1 dup drop) end j add 1` over plain data (`BIND_GLOBAL splice
underflow`). Probing it found NUR285, silent on main: a root def bound to a
factory's argument-taking fn and read inside a root code body over values
beneath it (`do [10 j]` answered `[10 fn j(Integer)]` for 11), loud in a fn
body, and a `/v` read of a def whose value lives in a frame local rendered
`fn` for `fn j`.

**The fixes, as recorded.**
- NUR217: `SplitEventRegionBind` declines when a later event consumed the
  rest (`restConsumed`); a multi-output source's slots are promoted when
  the def binds the first result and every other is consumed
  (`restAllConsumed`); the root read's consumer matches the read's result
  index; a push-tested read with no push is tested before its statement
  when it is the consumer's deepest operand; and the shuffled copy carries
  the factory's shape claim (`shuffledFrom`, probing the shuffle's
  `ReturnsIdentity` over markers).
- NUR285: a code body at the root keeps its deopt points when the root's
  own statements bind every captured name (`rootCapturesBound`), and the
  island installs the captured fn (`DeoptSpec.Install`); a push-tested read
  the residual holds counts only the residual entries after it as deferred
  (`residualAfterRead`); the frame replay's word reads take the data lookup
  (`replayWordLookups`); a def's rename reaches the value's frame local
  (`GlobalBindSpec.WriteSlot`).
- The fold's fence stands only for a lambda's or a stored body's result:
  with NUR285's close every read of a code body's folded result is
  measured, so `def j (do [l.0]) end j` answers 42 and the sweep's `if` ×
  container code-body variants compile. NUR286 (silent, open) is a def of a
  paren over a `do` run whose fn takes an argument; its lambdas are never
  folded, so the fence never covered it. A guard on the root landing over
  values beneath turned it loud but fired where root deopt islands,
  def-bound methods and claimed factories already answer, and was reverted.

## Main's NUR216 and NUR218 closed: a def read's island dispatches the name, and a fn argument in a bare-read slot runs on the interpreter (2026-09-27)

**The divergences, both silent.** On the branch (and at the pushed head), a
Function-declared factory's result def-bound to `j` and read in a code body
or fn body answered `fn j` for 42 (`do [j]`, `each [drop j]`, a fn's body),
and `typeof j` / `j eq j` answered where the interpreter's barrier raises
(NUR216). A container member's lambda passed to an `Any` parameter and read
bare in the callee was data where the interpreter dispatches it and raises
`cannot call h` (NUR218).

**The fixes, as recorded.**
- NUR216: the body read's data lookup reaches the binding's installed copy,
  which `installFnDef` rebuilt without the compiled stamp, so the frame entry
  declined and the island stepped the VALUE, which parks. A def read's
  shaped method now carries `DynMethodSpec.DefRead`, and its island steps the
  WORD. And a read of a name def-bound to a Function carrier is a barrier in
  the pass as a registered fn word is, so the collected rows decline on the
  interpreter's strand error.
- NUR218: a named unit with a declared contract records the parameter slots
  it reads bare (`namedUnitFnReadParams`), and CALL_USER runs a call with a
  fn in one of them on the interpreter (`fnReadCallUser`); data runs the unit.

## NUR257 closed: an anonymous fn value's reader identity (2026-09-27)

**The divergence.** The dynamic-scope rescue keeps an undefined-word finding
unless a binder of the name reaches the reading fn. An anonymous fn value
had no name to be reached by: a list member run as a callback inside the
binder (`def m [([x:Any] => [k])] end def h fn [[][List] [def k 7 [1] each
m.0]] end h`) or a behaviour exercised there was a false finding and
declined, and main's #511 merge had made the folded map member answer
optimistically, a false negative at the root.

**The fix, as recorded.** The value's body position is its identity (core
`anon_reader.go`): registered at queueing with its span, a finding inside it
names it, and the rescue and the compiler's dynamic-scope operand ask the
call graph under it. `AnalyseFnBody` adds an edge from the named fn under
analysis whenever it enters a registered body; `behave`'s check-mode half
notes the value per capability, seeing a member carrier through the tag
the pass now keeps for get-family reads (`FnMemberReads`), and a dispatch
over a value of the target in a named fn adds one. The interim folded rule
is gone. Found probing, pre-existing: NUR284 (a folded member applied in a
binder's frame raises compiled).

## NUR263 closed: an optimistic closure bake raises the interpreter's report (2026-09-27)

**The divergence.** `def mk fn [[][Any]["s"]] end 0 fold [add] (mk)` raised
fold's no-match on both lanes, but compiled without the notes or help: the
pass bakes the overload it picked over the compiled body closure, and the
handler's refusal of a value no overload takes is a bare signature_error
(NUR165's). The dyn-body seat, which would re-match, arms DynEnv program-wide
and was rejected for these words.

**The fix, as recorded.** `execMatch` publishes an optimistic match's exact
layout for its own record (core `optimisticLayout`, over NUR242's
`CurLayout`); `RecordClosureCall` attaches it to the committed call as
`SigRef.Split` for a token-list body. On the handler's refusal the VM puts
the body's token list back in the closure's slot, plans the operands as the
interpreter's tape (`nativeSplitRaise` / `splitNoMatch`, shared with
NUR242's poly arm) and raises the report when no signature fits. The
operands are copied first — the body's calls reuse the scratch buffer.
`optimisticOuter` moved to core `optimistic_match.go` beside it (the
aritygate pins its three window reads). An inexact layout — a value beneath
the operands, a token after them — keeps the bare refusal: recorded as
NUR283.

## NUR264 closed: a trap under an optimistic match is the outer word's rematch (2026-09-27)

**The divergence.** `def mk fn [[][Any][5]] end each (mk) [dup]` raised
`each`'s no-match interpreted and `dup`'s compiled. The check pass matches
`each` optimistically over the declared-Any result and evaluates the data
list `[dup]` under the match, where the inner failure is definite and was
recorded as an unconditional top-level trap. The interpreter evaluates the
list only once `each` matches.

**The fix, as recorded.** `execMatch` publishes an optimistic dispatch as
`CheckState.OptimisticOuter` (outermost wins) around its argument
evaluation; a trap recorded under it becomes the outer word's
`DISPATCH_REMATCH` carrying the trap as `DispatchSpec.OnMatch`. The VM
raises the outer no-match when the live values do not match and the inner
error when they do; an unplannable split still defers. The window lists
the stack run top down and the written operands in written order: the
written count is recorded by `rearrangeForForward` (the word's pointer and
position, the forward count), since a re-stepped word has every operand
beneath it — reading the positions alone rendered `each 5 [1 add]`'s tuple
reversed. Pinned by lang `TestNUR264TrapUnderAnOptimisticMatch` (seven
witnesses, full-error parity).

## NUR275 closed: the root's leading apply over a def read is the binding's word dispatch (2026-09-27)

**The divergence.** `def g m.f end g 5` over a member lambda `h` whose body
returns a String raised `g: return value 1: …` at the read interpreted and
`h: …` at the value's `h/v` token compiled. The interpreter's `def` renames
the fn value to the def's name (installFnDef) and dispatches the read as a
named call; the compiled root applied the value through the Apply kernel
with no named head, so the frame took the value's own name and anchor.

**The fix, as recorded.** Finalize seats a named head (`DynApplyHead`) at
the root's leading `OpCallDynamic` when the lead is a root def read the
NUR207 plan recorded — its name, the read's position, the window's count,
`Leading` — and emits the op at the read. The Apply kernel's frame contract
takes the head's name (`headNamedContract`, eng). A no-match names the
binding (`cannot call `g``) and a count error too. A zero-argument value
still fires and leaves its window (`g 5` over `g0/v` is [7 5]); `Leading`
is what keeps the window after the result on the island path.

## NUR282's latch half: a live-seated read passes the kept-defs latch as a carrier (2026-09-27)

**The problem.** Main's kept-defs latch declines the first observer of a
binding after a computed keep-defs body — sound, but it also declined the
reads the branch seats LIVE (a root read after a root computed body, a
unit's own def after one in the unit), which observe nothing stale. The
merge had tried exempting those reads and backed out: a list or map literal
over the name folded the check model's pre-body value (`… each b xs drop
[t]` answered [[0]] for [[3]]), because the live read's value stayed
concrete.

**The fix.** Both halves at once:
- the latch's read check waits for the tag hook that follows a def read
  that may be seated live (`noteKeptDefsRead`), and a read NoteLiveRead
  seats live clears it (`keptReadSeatedLive`); anything else becomes an
  observer at the next recorded event, dispatch or Finalize
  (`flushKeptRead`), the pattern main's `flushGradualRead` uses;
- the seated read's value becomes a CARRIER of its type, so every consumer
  — a list or map literal, a def, a native call — assembles from the live
  lookup instead of folding a constant. A read that may hold a fn stays an
  observer: a fn value is interned as a const, which the carrier would not
  stop.

**The lambda proof narrowed.** A first cut of NUR282's parking proof
admitted a LONE arg-taking lambda. With the latch passing the read after
it, `do (mk) x` over `[([n:Integer] => [n add 1])]` went silent ([fn 5] for
6): the lambda takes a token written after the run in its statement. Only
zero-argument anonymous lambdas are proven now, and the lone arg-taking one
is the plain check's loud defer.

**Measured.** Over the 340 probe programs, 63 more answer the interpreter's
way than on the merged tree, and none is silent. NUR203's and NUR210's
rebinding witnesses compile again (`TestDynamicKeepDefsBodyLeaksToTheFn`,
`TestKeptDefsLiveReadsCompile`, the restored NUR210 rows). A parameter the
body may rebind, and a fn it may redefine, still decline.

## NUR213 and NUR281 closed, NUR282 narrowed: a computed run is re-stepped or checked plain (2026-09-27)

**NUR281 (silent, on main and the branch).** A map literal's value after a
computed keep-defs body at the root was const-folded off the emit path, so
the kept-defs latch never saw the read: `[1 2] each (mk) end {a: x}` over
`[def x 5 1]` answered `{a:99}` for `{a:5}`. When the latch arms at the root
the compiler now generalises every root value binding
(`generaliseRootValues`, the speculative undef's transition), so the fold
stands aside and the latch declines the read, as it does the list twin.

**NUR213 (main's, silent).** A computed `do` run that leaves a fn value is
re-stepped by the interpreter and was data compiled. Two halves:
- a run ALONE at the program's end that may leave a callable is the prefix
  island's, with an empty prefix: `do (mk)` over `[g/v]` is 7 on both lanes;
- a run seated anywhere else — values beneath it, entries after it, a fn's
  result — compiles under a runtime check that none of its values is one
  the interpreter re-steps (`DynBodyPlain`, the VM's `vm:dyn-body-plain`
  defer). This replaced the seat rule's declines, main's `dynRegionNotLast`
  among them: "keep bails on the ledger rather than declining over-wide".
  A first cut that declined those seats statically (a value beneath, a fn's
  result) was sound but declined `f (quote [1 add 2])`; the check keeps it.

**NUR282 (the merge's compile losses), two shapes.** A run proven to leave
only PARKING fn values — a lone anonymous lambda, or zero-argument ones only
— is no callable region (`bodyParksFnValues` over main's
`provenBodyTokens`); an arg-taking lambda beside a value takes it inside the
run, which the probe caught (`[5 lam]` is 6), so such a run is not proven.
And the plain check compiles plain runs the seat rule declined.

**Measured.** 311 probe programs — the merge's 199 and a run × context
matrix of 120 (eight runs, fifteen contexts) — answer the interpreter's way
or fail loudly on the compiled lane; no silent row is left. The lambda
token's shape was read off the pass, not assumed: a quoted `(A => B)` is a
paren group of one item, the inner group `[A, sugar(lambda), B]`.

## The merge of main's #514: two closes of NUR207 and NUR210 composed (2026-09-26)

**What main brought.** Main's #514 closed NUR207 and NUR210/NUR211 its own
way, and recorded NUR213 and NUR216–NUR218; the branch's colliding NUR213
and NUR216–NUR218 were renumbered NUR277–NUR280 before the merge (main keeps
its numbers). For NUR207: a parking member folds to its lambda behind an
escape fence, a gradual carrier claims a shape where the pass can prove one,
`apply` over a gradual lead is the pending apply, and a proven fn at an
`Any` parameter declines. For NUR210: a computed `do` run is a variadic
REGION, a single-value seat takes one runtime-checked value (the VM's
`vm:dyn-body-one` defer on any other run), and a keep-defs word over a
computed body arms the KEPT-DEFS LATCH, whose first later observer declines.
For NUR211: a decline (`rematchWindowMatches`).

**How they were measured.** Nine textual conflicts, but the merge was
semantic: both sides had closed the same records. 199 probe programs — every
row of both sides' tests for these records, and the merge's own probes of
the soundness boundary — ran on the branch, on main and on the merged tree.
Main's rules turned out SOUND where the branch's closes were not:
- a NAMED 0-arg fn value in a computed run fires across the statement's
  `end` (`do (mk) end x` over `[g/v]`: [7 5] interpreted, the branch's
  [fn g 5], silent), which NUR266's statement-boundary seat missed;
- a list or map literal over a name a computed keep-defs body leaked bakes
  the pre-body value (`… each b xs drop [t]`: [[3]] interpreted, the
  branch's [[0]], silent), which NUR203's live seat missed;
- a parameter or a fn a computed body rebinds is read stale (`do b drop
  do b` over a body rebinding `b`; `def h …` rebinding a root fn);
- `9 do (mk) drop` seated the 9 above the run.
Main's rules decline every one. So they stand, and the branch's mechanisms
stay where main's rules do not fire.

**The composition.** Three seams, each measured:
- The branch's split-aware rematch replaces main's NUR211 decline: `3 for
  (mk)` raises the interpreter's signature_error on both lanes (main's core
  test adapted to `TestS5BTrapCarrierWindowMatchRecordsTheSplit`,
  `rematchWindowMatches` retired).
- The branch's prefix island seats a run it can re-step whole: the lowering
  drops the single-value check a list literal's demotion armed on the
  island's own region, so `[9 do (mk 5)]` over `[n n]` is [[9 5 5]] and
  `[9 do (mk)]` over `[g/v]` is [[10]], where main deferred; `9 do (mk)`
  and `[9 do (mk)]` compile where main declined.
- A demoted run is one checked value to the island's list-literal decline
  (`runOperand`), so a fn body's `[9 do b]` compiles as main's does; and a
  member read's own `/v` (`m.f/v`, noted with no name by the branch's
  NUR280) does not trip the fold's fence.
One more seam was tried and backed out: exempting from the latch the reads
the branch seats live (a unit's own def, a root read). It re-opened the
literal bake above, silently, so the latch stands whole.

**The result.** The merged tree answers every probe the interpreter's way
or declines loudly, except NUR213's four residual shapes (silent on both
sides) and a new find, NUR281 (silent on both sides: a MAP literal's value
at the root after a computed `each` is never noted to the latch). What the
composed tree declines that the branch answered is NUR282. Ledgers, measured
on the merged tree: compile declines 342 = 342 (two reasons swap), bails
36 -> 31 (main's claim compiles three of the branch's NUR207 guard rows with
parity and declines two), the sweep's failing seeds 2 -> 1 (`if` × container
graduates) and its failing call forms 292 -> 290. Both sides' tests were
adapted to the composed behaviour, each change named in its doc comment.

## NUR222's fragment bails made loud declines; a static-zero division is a definite raise (2026-09-26)

**The divergence (main's NUR222).** A value-less `do` body inside a branch
fragment compiled and died at run time: `if [do [1 drop] true] [2] [3]`
underflowed DROP, and `if [true] [do [1 drop] 2] [3]` underflowed
STORE_LOCAL. The interpreter answers `[2]`. The check pass models a
non-empty body with an empty residual as the one Error a caught raise
leaves, and latches that count runtime-variable. A clean run leaves
nothing, so every fixed seat of the value pops something that is not
there.

**The fix.** The latch's value-less use is now its own event flag
(`eventFlags.catchPhantom`: a catch-latched call with one recorded result,
where the latch's multi-value use records N). A fixed seat of the phantom
declines: a promotion to a frame slot, a dead result's drop, and both
spills (`spillSeat`, `seatResidualRebuild`). Where the latched count is
absorbed, it keeps compiling: the program residual (`do [1 drop]`), and an
arm whose out IS the do.

**A static-zero division is a definite raise.** `returnsDivMod` models it
as a raise-like empty residual, but noted no raise. It now notes one, as
`raise` does (`NoteDefiniteRaise`). Its `do` then nets exactly the one
Error it catches, not the latched phantom, and a def after it is never
made. `(do [1 0 div]).code` keeps compiling under the new guards, and
`def x (do [(0 div 0) "a" "b"] error [dot code]) x` now compiles with
parity: a frontier decline graduates, as NUR134 graduated
`(raise aa "m")`.

**What was tried and backed out.** Declining every word that consumes the
phantom as an operand closed `1 do [1 drop] drop`, a pre-existing top-level
bail. It also declined designed consumers:
- the `do … error […]` catch form;
- a fallible multi-value body's clean N;
- `for 1 [do [for 2 [raise 'x']] drop i]`, whose loop always raises but the
  pass cannot prove it, and which compiled right.

So a word consuming the phantom still bails. That is NUR222's open part:
the latched count wants seating at run time, as the mark window does for a
do-catch region.

**Ledgers.** Unit-suite compile 340 -> 342 and bail 37 -> 36. The rows:
- the two NUR222 pins, which decline (+2);
- do_empty_residual_test.go's `[1 2 3] each [ do [ 9 drop ] ]`, which
  moves from a bail to a decline (+1, and the bail ledger's -1);
- the graduated S9 row, which now compiles (-1).

**A correction.** The merge of main's #513 wrote NUR224's new status onto
NUR222's section, because both carried the same "Pending" sentence. Both
sections now carry their own.

## NUR210 closed by the root's generalisation, NUR276 found and fixed, NUR223 declined loudly (2026-09-26)

**NUR210's last silent reads.** After a computed `do` body at the root, a
read the bare-read rule did not reach folded the value the pass held
before the body. Over `[def x 5 7]` after `def x 99`:
- `do (mk) end [x]` answered `[7 [99]]` for `[7 [5]]`;
- `{a: x}`, `def y x end y` and a fn unit's `[x]` did the same.

`do`'s check half now generalises every root value binding in place when it
meets a computed body at the root (`generaliseRootValues`, basic). It uses
the speculative undef's own transition, `core.GeneraliseSpecUndef`, so every
later read is live and the bindings stay. Its scope:
- **At the root only.** Inside a fn body the frame's leak already reads
  live (NUR203).
- **Not fn values, types or frame bindings.** Those are the transition's
  own exclusions.

Measured against the tree before it: every probe that changed moved from a
silent wrong answer to the interpreter's, and no new decline appeared. It
is pinned by lang `TestNUR210ComputedBodyGeneralisesTheRoot`.

**NUR276 (found, pre-existing on main, fixed).** Probing the
generalisation's neighbours: `do (mk) end s size` declined, and so did
`… end for n [1]`, with NUR121's "fn-value lead's argument was collected
by a later dispatch". The engine's collection-hazard walk marked the run
because a later statement collected a value above it. The run's re-step
could never have taken that value: its forward collection stops at the
`end`.
- **The fix.** The engine keeps the statement ends an analysis pass steps
  (`stmtEnds`). The walk skips a candidate that a statement end separates
  from the collected value.
- **A def-bound value.** Its reads share one value and none of their
  positions, so it answers by EVERY read's position (`defReads`). That is
  `defReadCrossed`'s rule from NUR266: one read before the end proves
  nothing.
- **What stays.** With no end between them, a lambda run does take the
  value at its re-step, and `do (mk) s size` still declines, soundly.

Pinned by lang `TestNUR276RunIsNoHazardAcrossAStatementEnd` and core
`TestCollectionHazardStopsAtAStatementEnd`.

**NUR223 (main's; silent → loud).** `while` analyses its body before its
condition. That order lets the condition read the body's carried rebinds,
but it makes a name the CONDITION rebinds read stale in the body:
`def t 0 end while [def t (t add 1) (t lt 3)] [t] end t` answered `[0 0
3]` for `[1 2 3]`.
- **The decline.** A condition whose analysis moved the binding shape
  (NUR212's `bindingShape` probe) now marks the loop uncompilable. It is
  conservative: a fresh name the body never reads declines too.
- **The compile cut, recorded.** The two analyses want one carried scope
  at a joint fixed point.
- **Ledger.** The unit-suite compile ledger moves 338 -> 340, with both
  rows named. Read-only conditions keep compiling with parity.

**NUR275's trace, recorded.** A root def-bound member value applied by
name raises under the value's name and token compiled. `installFnDef`
renames the value on the interpreter, and the fn units' word-read
accounting (`wordReadNames`, `wordReadPos`) has no root counterpart. The
cut is to give the root lead arm that accounting.

## The merge of main's #513: NUR190's two closes composed, NUR274 found and fixed (2026-09-26)

**What main brought.** Main's #513 closed NUR190's `/q` claim its own way,
kept a binding `if` condition's defs (NUR212's follow-up), and recorded
NUR219–NUR224. Fourteen files conflicted. On a merge main keeps its
numbers, so this branch's NUR219–NUR224 were renumbered to NUR268–NUR273 in
a commit ahead of the merge: the records, their index rows (each noting the
old number), every reference, and the three test files named for them.

**NUR190, closed twice.** This branch's close hands a claimed landing to
the interpreter: an island where the word is in the body at the landing's
depth, and a skip past the word's call and the paren apply elsewhere.
Main's close seals the landing's claim target where the word's
argument-free call and the residual `OpCallDynamic /1` follow it at once
(`LandingWord.Skip`), then enters the fn's `/q` overload over the word as
an atom and stays compiled. The two layouts barely overlap: main seals the
residual apply, and this branch's skip rides the paren apply (the shaped
method op). The merged landing tries main's claim FIRST, then the island,
then the skip, and defers only with none of the three.
- Main expected `m.f typeof` and `m.f y 5` to defer. They take the island
  and answer `[typeof]` and `[y 5]` on both lanes, and are parity rows now.
- Main's `m.g z` row expected the interpreter's 7. With this branch's
  NUR078 a bare `z` calls at every slot, so it is the named no-match on
  both lanes. That was already pinned here; main's row is dropped.
- Main's run-loop field `landingSkip` and this branch's method of the same
  name cannot share a struct, so the method is `landingSkipCapture`.

**NUR274 (found and fixed).** Main's return-contract row for the claim
failed on the merged tree on its caret alone. The interpreter anchors a fn
value's contract error at the value it re-stepped: for a member read, the
value's own `h/v` token (1:66). The compiled frame blamed the residual
apply's op (1:128). The same held for every Apply-kernel entry over a
member read (`m.f 5`, `5 m.f`, `(m.f 5)`, `m get 'f' 5`: 1:95 for 1:69),
and main answered them all at the value.
- **Why.** This branch's NUR118 moved a nested frame's contract error to
  the CALL. That is right for a named call, and for a name read, whose op's
  debug entry is the read. Over a member read it blamed the op.
- **The fix.** The Apply kernel hands the frame the applied value's
  position (`dynEnter.at` → `vmFrame.retAt`), unless the op carries a
  named head (`applyAnchor`).
- **Parity held and gained.** The name reads keep theirs: `ap m.f`'s
  `(g 5)` (1:125), `def g h/v end g 5` (1:109) and `h/v apply 5` (1:95).
  Measured shape by shape against the merged tree and main.

**NUR275 (recorded, open).** A root def-bound member value applied by name
(`def g m.f end g 5`) raises under the name at the read interpreted (`g:`
at 1:109) and under the value at its token compiled (`h:` at 1:69). Main
answers exactly the same, and so did this branch, at 1:99. It is fenced by
`TestRootDefBoundMemberFnContractPending`.

**Main's new records on the merged tree.**
- NUR220, NUR221 and NUR224 answer as the interpreter does through this
  branch's earlier closes: NUR279's decline of a stored unit's bare fn read,
  NUR207's root read, and NUR118's call anchor, whose witness NUR224 is.
  They are marked FIXED and pinned by
  `TestMain513RecordsClosedOnTheMergedTree`.
- NUR219 (loud), NUR222 (loud) and NUR223 (silent) stay open.

**Both sides kept.** The value-less `do` both latches its runtime-variable
count (this branch's NUR242 `do` half) and counts for main's binding
condition (`ValuelessDoBodies`). The reset list and the lifecycle
classification carry both `RaiseWatches` and `ValuelessDoBodies`. The
`seatLandingWord` argument order is main's.

## NUR210's rebinding half for a bare read closed, with NUR266 and NUR267 found and closed (2026-09-26)

**The divergence (NUR210's rebinding half).** A computed `do` body at the
root runs in the root's scope, so a later read of a name sees its defs and
undefs. `def x 99 end do (mk) end x` over `[def x 5]` bailed compiled
(`CALL_DYNAMIC underflow`) where the interpreter answers 5, and over
`[undef x]` bailed where it raises `undefined word: x`.

**Three defects stood in the way, two of them silent in their own right.**
- **The read was baked** as the check pass's binding. `noteDynKeepDefsLeak`
  now latches `rootDynLeak` at the top frame. A later root read of a value
  binding seats live (`NoteLiveRead` → `LOOKUP_DYN_SCOPE`), on the registry
  the body installed into.
- **NUR266 (new; silent).** The residual's leading apply took the body's run
  as a lead over a read written after the statement's `end`. Over a run
  ending in a lambda, `do (mk) end x` answered 6 for the interpreter's `[fn
  (Integer) 5]`. The statement-boundary rule (`crossesStatementEnd`, NUR187)
  could not place a def read, because one binding's reads share a value. The
  recorder now keeps every read's position (`defReadPos`), and an entry whose
  reads all sit past a boundary after the lead proves the crossing.
- **NUR267 (new; silent).** A computed body's `undef` did not take effect:
  `[undef x x]` over `def x 99` answered 99 for a caught `undefined word:
  x`. The run-time token-body stamp compiles the body as a detached fn unit,
  whose frame does not model an unbind it did not make. A body with an
  `undef` anywhere in it is no longer stamped (`bodyUndefs`), and runs on the
  interpreter.

**Why they had to land together.** Measured one at a time on this tree:
- NUR266's proof alone turned the `[def x 5]` witness from a loud bail into
  a silent `[99]`: the apply stood aside over the baked read.
- The live read without NUR267 turned the `[undef x]` witness into a silent
  `[99]`: the stamp had lost the unbind.

Only the three together answer every witness.

**What stays open (silent, pre-existing — NUR210 stays OPEN).** The live
read covers a bare root read. Any other read still bakes the value the check
pass held before the body, over `[def x 5 7]`: `do (mk) end [x]` is `[7
[99]]` for `[7 [5]]`, and so are a map member and a `def` forward operand.
The list literal seats the live read and then folds itself to a constant.
The sound cut is to generalise root values in place after such a body, as a
placed speculative undef does.

**What stays open (loud).** These are NUR266's open half:
- `x do (mk) end x` declines. It answered `[5 6]` silently before; the run
  between two reads of one value needs the prefix island to close right
  after the run.
- `do (mk) end [x]` and `do (mk) x` over a body that nets nothing bail at
  `CALL_DYNAMIC` as before, because the dynamic apply's window counts the
  run as one value.

**Pins.** Lang `TestNUR210ComputedBodyRebindsTheRoot`,
`TestNUR266RunLeadStopsAtTheStatementEnd` and
`TestNUR267ComputedBodyUndefTakesEffect`; compiler `TestNUR267BodyUndefs`.

## NUR242's `fold` program closed, NUR263–NUR265 recorded, NUR265 closed: the exact layout (2026-09-26)

**The divergence.** `def Box class {data: Any} end def b (make Box {data:
"s"}) end 0 fold [add] b.data` is the interpreter's signature_error.
Compiled it bailed at the poly's no-match ("CALL_NATIVE_POLY no match for
fold", internal_error, "please report it"). The same held for `b.data 0 fold
[add]`, in a group, a list, a fn body, a branch and under `do … error`.

**Why.** The check pass cannot match `fold` over a field declared Any. Its
recovery records the widest overload's window as the dyn-body backstop's
poly re-match. At a run-time no-match the VM had two ways to raise, and
neither applied:
- no record-time probe (`PolyNoMatchSpec`) was taken on this path;
- the arity screen declines any word with an overload narrower than the
  window, and fold has a 2-operand one.

The screen is right to decline. The poly's operands are in signature
order, so the VM could not tell a written operand from a stack one, and the
split decides what the interpreter's plan claims.

**The fix.**
- **The layout (core, `dispatch_layout.go`).** The recovery publishes the
  window's EXACT layout on `CheckState.CurLayout` while it records, and
  restores it after. The one record it describes reads it through
  `LayoutFor`, keyed on the operand slice itself. Exact means:
  - the operands are the failed-dispatch tape's own values (by identity);
  - they are contiguous on each side of the word;
  - a statement or group boundary, or the tape's end, closes both sides.

  The interpreter's tape at the same failure is then exactly [stack
  operands, word, written operands].
- **The record (compiler).** The dyn-body poly carries the written count
  (`PolyRef.Split`).
- **The raise (eng, `polySplitRaise`).** The no-match lays the operands out
  as that tape. It then runs the interpreter's own plan (`CollectForward`,
  `PlanMatch`) through `planSplit`, now shared with NUR211's split rematch.
  - No plan raises the interpreter's report over the tape, byte for byte.
  - A plan keeps the op's path, because the interpreter dispatches there.
  - A walk the host cannot drive keeps it too.
- **The report (`NoMatchOverWindow`).** It takes `sigError`'s attempted
  window (NUR172) now. It named the forward tokens alone, so `b.data 0 fold
  [add]` named one argument where the interpreter names two. The routed
  dispatch (`DISPATCH_GENERIC`) raised through it with the same drift.
  Core's `TestNoMatchOverWindow` had pinned the old tuple for `1 "x" w
  true`; it pins the interpreter's `true`, `'x'` now.
- **The negative.** A window with a value beside it that the dispatch did
  not take (`0 fold [add] b.data 5`, whose report lists the 5) has no exact
  layout, and keeps the sound defer.
- **Not taken.** An execMatch-side publish (the successful optimistic
  match) was built and dropped. No record reads it, and it cost every
  compiling dispatch a layout walk.

**Found on the way.**
- **NUR263 (open, loud).** fold's closure bake over a declared-Any fn result
  raises a bare signature_error, with none of the interpreter's notes.
- **NUR264 (open, loud, the wrong error).** An error inside a matched
  signature's data list, auto-evaluated under an OPTIMISTIC static match over
  a declared-Any operand, is a top-level trap that raises before the outer
  word's no-match. `each (mk) [dup]` and `filter (mk) [gt 1]` over 5 both hit
  it. The fix needs a conditional trap, because suppressing it would decline
  the List runs that agree today.
- **NUR265 (closed).** The check pass PANICKED on `filter (mk) [gt 1]`. The
  const fold ran `gt`'s handler over a recovery's one-operand window: index
  out of range, caught by the top-level guard. `concreteHandlerEval` now
  declines a window shorter than its signature's arity. The fold tests'
  `statefulSig` declares the one operand its callers pass.

**Pins.**
- lang `TestNUR242FoldNoMatchIsTheInterpreters` (fifteen parity rows, the
  two report notes, the negative) and `TestNUR265CheckPassCompletes`;
- core `TestDispatchLayoutIsExactOrNothing` and `TestSigOrderPositions`;
- eng `TestPolySplitRaiseIsTheInterpretersPlan`;
- compiler `TestNUR265HandlerRunsOverItsArity`.

**Ledgers (gate25, measured row for row against 44f7eb2).**
- Engine entries 186 → 191 and interpreter entry rows 39 → 41: code-bodies.tsv:L213
  and each-variants.tsv:L207 (`each [if [gt 1] …]`) panicked the check pass
  like NUR265's filter and did not compile; they compile now, with the body on
  the interpreter per element.
- Diagnostic parity 354 → 355: L213's plain-check no_signature/gt, which the
  panic masked.
- Arity pins: compiler_dispatch_record.go 1 → 2 (the handler's full arity),
  and dispatch_layout.go pinned at 1 (a plain word, no `/N` modifier).
- `CheckState.CurLayout` is classified as a scoped transient in the
  lifecycle test.
- The unit-suite ledgers are unchanged: none of the new rows bails or
  declines.

## NUR210's silent half closed: the prefix island (2026-09-26)

**The divergence (main's record).** A computed `do` body, one a fn
returned, seated its run wrong under anything beneath it or around it:
`def mk fn [[][List][quote [1 2]]] end 9 do (mk)` answered `[1 9 2]` for the
interpreter's `[9 1 2]`. Probing found the same class wider. `[9 do (mk)]`
gave `[1 [9 2]]`, `[do (mk)]` gave `[1 [2]]` and `size [do (mk)]` gave
`[1 1]`. A body placing a lambda last (`[5 ([x:Integer] => [x add 1])]`) gave
`[5 fn]` for `[9 6]` beneath a 9, and `[5 [fn]]` for `[[6]]` in a list. All
of these were silent.

**Why.** The interpreter's `do` splices a computed body's results back and
RE-STEPS them. Each value lands above what lies beneath, and a trailing fn
value applies over it. The run the body left is `[5 fn]`, and the re-step
applies the fn over the 5. The recorder models the dyn-body CALL_NATIVE's run
as its one recorded seat. So every layout that put something beneath it or
around it read the run as one value. The trailing apply rotated the run's
top under the 9, and `MAKE_LIST` counted one element.

**The fix (`compiler/go/prefix_island.go`).**
- **The plan.** It is armed only when no other mark plan owns the frame.
- **The mark.** A mark opens before the run's CHAIN: the dyn-body event and
  the calls producing its operands, one contiguous block ending at the run.
  The scalar constants beneath the run are pushed at the mark.
- **The close.** `CALL_DYN_MIXED_FROM_MARK` re-steps `[prefix…, run…]`
  through the interpreter's own machinery.
- **A list literal.** It opens an outer mark first and collects the island's
  results into one List (`MAKE_LIST_TO_MARK`), so it holds the run's count.
- **Force-promotion.** The residual's run is excused from it
  (`excuseIslandRegion`), because a promoted run stores one value.
- **What declines.** A list over a run the island cannot seat declines. That
  covers a list in a fn body, and a run reached through a branch arm
  (`[9 if c [do (mk)] [3]]` answered `[1 [9 2]]`). The trailing apply no
  longer rotates a run, so an unplanned one declines as a result above a
  literal.
- **What is a run.** It is a new record-time flag, `eventFlags.dynBodyRun`:
  a dyn-body event over a COMPUTED body whose results ARE its body's
  residual (`do`'s list form, whose `CallableSpec` returns the whole residual
  and leaks its defs). A literal body the backstop took was modelled exactly
  by the pass. gate23 caught the first cut islanding fn-value.tsv L332 and
  L333 (`[(do [f/v])] join ','`), which were right natively: the entry
  censuses rose 186 → 189 and 39 → 41. The backstop's variadic mark is
  wider still. `each` and `fold` over a dyn body
  answer one value, and keying on that mark declined
  `[(each hold.f [1 2]) (each hold.f [3 4])]`, which compiled correctly
  before. The ledger caught it (339 against 338).
- **Beside NUR067's region prefix.** The older prefix seating
  (`planRegionPrefix`) stands aside for a run. It seats without a re-step,
  so `7 do [if true inc/v [2]]` left `[7 fn inc]` for `[8]` once the
  trailing apply stopped rotating runs (lang `TestBranchFnValueAgrees`).

**Still open: the rebinding half (loud).** `def x 99 end def mk fn
[[][List][quote [def x 5]]] end do (mk) end x` bakes the read of `x` as the
check pass's 99. At the root, `noteDynKeepDefsLeak` stands aside, and the
run then leads a dynamic apply that underflows (`internal_error`). A read
after a keep-defs dyn body must read live. The run's own place as a leading
apply's lead is a separate question: a fn the splice leaves can
forward-collect what follows it.

**Pins.** lang `TestNUR210ComputedDoRunBeneathAndCollected` has twelve
same-answer rows on both lanes and checks the lowering (the marks, the
island, the collect). The loud declines are asserted through CompileCheck
and not booked.

## NUR211 closed: the rematch plans the written split (2026-09-26)

**The divergence (main's record, loud).** `def mk fn [[][List][quote [i]]]
end 3 for (mk)` is the interpreter's `signature_error`. Compiled it was an
`internal_error`: "DISPATCH_REMATCH at for matched at run time where the
static model failed".

**Why.** The check pass's no-match over a window that holds a carrier (the
paren's List result) records a runtime rematch. The rematch read its window
flat. Window[i] was sig position i, and the window lists the stack run first,
then the operands written after the word. So `3 for (mk)` was matched as
`for 3 [i]`, which fits. The interpreter's forward phase takes the written
operand first. A List where the count goes stops that phase, the stack's 3
fills the count, and the body finds nothing beneath: no match.

**The fix.** The check pass counts the window's written operands and hands
the count to the recorder (`RecordDispatchRematchValues`'s `nFwd`). The
count rides as `DispatchSpec.NFwd`. With any written operand, the VM plans
the window as `DISPATCH_GENERIC` does: the stack run bottom up, the word,
then the written operands, through `core.PlanMatch` over the region host
(`rematchSplitMatches`). The planned signature is then matched strictly
over the values at its positions, because the plan alone is optimistic.
It takes a written record by its base, and gate23 caught the first cut
deferring record.tsv L155 (`use (mk)`, a refined record another
refinement's param refuses) where the flat match had raised. A match
defers, as before. A plan the host cannot run (a written template string
it would have to evaluate) defers too. No match raises the diagnostic the
rematch always built over the written tuple. A window with no written
operand keeps the flat match.

**Pins.** lang `TestNUR211StackFormCountOverAComputedBody`: both witnesses
and `3 each (mk)` on both lanes with the interpreter's `signature_error`,
and the written forms as negatives. eng `TestDispatchRematchPlansTheWrittenSplit`
covers the split's no-match, the flat match it replaces, a written match
that defers, and a window the host cannot plan. compiler's rematch test
adds the forward count's range guard.

## Main's #512 merged (2026-09-26)

**The conflicts.** Main's #512 conflicted with the branch in nine files.
One of them was code. Main moved if3's decided-condition arm into
`ifTakenArmReturns`, so that the clause-list `if` can lower its decided
clauses through it. The helper keeps this run's NUR243 rule: a taken arm
that nets nothing is a 0-value statement. Main's new decline stays as main
wrote it: a taken arm that leaves a fn value. With that decline disabled on
the merged tree, both sides compiled the fn's APPLICATION silently. `if
[true] [g/v] [1]` answered `[5]` for the interpreter's `[fn g]`, and `10 if
[true] [g/v] [1]` answered `[15]` for `[10 fn g(Integer)]`.
`TestLoopIndexUnwoundByCaughtError` keeps this run's NUR206 close, and
`code-bodies.tsv` keeps both sides' rows: main's L141 compiles as the
hosted splice, and this run's L142 is NUR154's decline.

**The register.** Main's NUR210, NUR211 and NUR212 keep their numbers. This
run's NUR210, NUR211 and NUR212 (all closed) are NUR260, NUR261 and NUR262
everywhere they are cited.

**Main's one test that failed.** `TestClauseListIfDeclinesLoudly` expected
`if [true []]` to decline with "branch produces no value". NUR243 compiles
it, and it answers `[]` on both lanes. The row is in
`TestClauseListIfCompilesWithParity` now, beside `if [true []] end 5` and
`if [true [] false [1]]`.

**The ledgers.**
- **The compile-failure site and disposition censuses, 90 → 91.** Main's
  `ifTakenArmReturns#1` is the fn-value decline. Main's own count held at 91
  because the site moved from if3. This run had retired that site's 0-netting
  half.
- **Unit-suite compile, 339 → 338.** Main's hosted splice compiles
  `TestComputedForBodyDeclines`' first witness.
- **`compile_failures.tsv`, code-bodies 2 → 1.** L141 compiles, and L142
  declines.
- **Engine entries 185 → 186 and interpreter entry rows 38 → 39.** L141's
  hosted loop runs once on the VM's island, which was main's own +1 on
  each.

## NUR255, NUR258 and NUR259 closed: the unnamed args and the lambda's anchor (2026-09-26)

**The count the frame keeps.** The interpreter pushes an anonymous lambda's
unnamed args beneath its body. Its return check then takes the declared
count off the top and drops up to that many unconsumed args from the
bottom, so `(0 ([0] => [1]))` nets 1. The check pass seated the analysed
residual whole, the pushed arg included, so the call's model said two
values where the unit returns one. A list after it then underflowed at
STORE_LOCAL: `[(0 ([0] => [1])) 7]` bailed where the interpreter answers
`[[1 7]]`.

Two routes carry an anonymous lambda's call. A paren's re-step goes through
`spliceAnonCheckResult`. A literal dispatch goes through
`BuildFnBodyReturnsFn`, whose anonymous arm drops the declared count to
leave the unit a placeholder contract. Both now trim as the frame does
(`trimUnnamedArgs`). A residual the unnamed args cannot account for stays
whole: the interpreter raises its count error there, and so do both lanes.

**NUR258, found on the way and closed (loud, pre-existing).** When a body
leaves FEWER values than its count, the unconsumed unnamed args ARE the
return on the interpreter. `def f fn [[Integer] [Integer] []] end [(f 3) 7]`
raised `got 0` compiled. It was narrower than it looked. A non-empty body's
residual already carried the unnamed args, which the unit trims. Only an
EMPTY body lost them, because `AnalyseFnBody` returned nothing for it. Its
residual is its unnamed args now (`emptyBodyResidual`).

**NUR259, found on the way and closed (loud on both lanes).** The count
error of a literal lambda anchored at its first argument compiled and
nowhere interpreted, with identical text. The interpreter anchors a frame's
return check at the fn value's own position, and a lambda built in place
has none. The compiled call's anchor fell back to the first argument
(NUR118's rule for a word with no position), which is never where the
interpreter anchors a literal lambda. An anonymous call takes no fallback
now (`callSite.anonymous`).

## Main's #511 merged; NUR257 recorded (2026-09-26)

**The conflicts.** Main's #511 conflicted with the branch in twelve files.
Main's `RecordRuntimeBindDispatch` is folded into this branch's
`RecordRuntimeDispatch`, which its callers now reach with no results.
`NoteRuntimeDefDispatch` arms the same latch, and its unit decline and
end-of-pass part check are kept as main wrote them. `behave`'s String form
carries both the branch's `ReturnsFn` and main's compile effects. Main's
NUR209 kept its number, and this branch's became NUR256.

**The one test that failed, and why.** Main's
`TestBehaveOverContainerMemberCompiles` stores a map member's fn as a
behaviour whose body reads a fn-local `k`. On the merged tree it was a check
error, `undefined word: k`. Main's recorder raised the same finding while
stamping the member, and truncated it there. The survivor came from this
branch's NUR105 close (2026-09-25): a folded map member's fn is queued for
the end-of-pass body check, and an anonymous value's drained run has no call
graph identity, so the dynamic-scope rescue could not clear it. Main never
analysed that position, and it flags the same body in a list literal or in
place.

**The rule taken.** For the folded position only, a name some fn binds is
not a finding of the drained run (`PendingFnBody.Folded`,
`dropAnonymousBinderReads`). A name nothing binds still is (NUR105's typo
row), and every other anonymous position keeps main's verdict. A blanket
rule over every anonymous body was measured first and dropped: it cleared
`[1] each ([e:Any] => [x])` at the root when an unrelated fn binds `x`, a
finding main keeps. NUR257 records the question the drain still cannot
ask, and the call-graph edge that would let it.

**Two more found measuring the ledgers.** Regenerating the sweep showed two
call-form variants bailing, `def` × container · suffix-def and · splice. The
merge did not cause them: this branch's NUR207 close did, after the last
regeneration, and they only fail in the direction lane. A guard over a root
read that leads its own dynamic apply bailed on any fn value. It now bails
on a no-match alone, as the island's `NoMatchOnly` rule does, and both
variants answer 6 on both lanes. Separately, the ADR-008 run on 7cae708 found
`namedFnCountError` (NUR252) unreachable: every path into the shaped method
apply's guard enforces a boru fn's count first (the island, the foreign
arm's contract, the entered frame's RET). It is deleted rather than covered.

**Ledgers, measured on the merged tree, each named row for row.** Corpus
bail 5 → 0 and runtime defers 3 → 0: main's five closes are the run's five,
and the two main kept (NUR190) the run had closed. Diagnostic parity 352 →
354: main's two edge-quote rows, nothing else. Sweep seed failures 4 → 2 and
call-form failures 284 → 292: the two seeds main graduated count their call
forms now, eight of which decline here. Unit-suite compile 339 holds, and
bail 36 → 37 for the new no-match guard pin.

## NUR247 and NUR249 closed, NUR254 recorded and closed, NUR255 recorded: the run-time count (2026-09-26)

**The collect in a fn frame.** NUR247 and NUR249 both wanted the count
decided at run time: a mark before the apply, the assembly counting from
it. The machinery existed for the program root, a list literal over a
region alone (`OpStackMark` / `OpMakeListToMark`, NUR067's consuming half),
and a fn unit had only the prefix plan. Three changes:

- A fn unit plans its own collect (`planRegionCollectUnit`, beside
  `planRegionPrefixUnit`; one mark client per unit).
- The collect takes a list over a RUN of adjacent regions, with the mark
  before the first. The list's operands are recorded top-first, so the
  shape matches them in reverse event order. `[(for 2 [i]) (for 2 [i])]`
  compiles now, as do `[(k 5) (k 6)]` and a mix of both.
- An `apply`-word event, or an apply under a named head, becomes a region
  only when the plan arms (`applyWordRegion`, `planRegionCollectOver`).
  Its op is then count-agnostic (`collectedApplies`), never a one-result
  form.

So `[(5 f/v apply)]` and `[(k 5)]` in a fn frame answer as the interpreter
does. Every other consuming layout keeps the one-result raise, a bail on
the ledger by the maintainer's rule. Four of NUR246's fn-frame declines
compile now too. Its pins moved to the parity rows, with a `[7 (5 f/v)]`
negative kept.

**NUR254, found on the way (silent).** A NUR246 negative row I wrote,
`[(n ([0] => [1])) 7]` over n = 0, compiled `[[0 fn 7]]` for the
interpreter's `[[1 7]]`, on the committed tree too. The lambda's value
pattern met a carrier at its re-step, and the check pass parked it
statically. In place and in a list alone the outer paren's close
re-recorded the apply; with the 7 after it, the close's
collects-past-the-close arm recorded nothing. The re-step now records the
trailing dynamic apply itself when every refusal is a pattern over a
non-concrete value (`undecidedPatternWindow`, `recordUndecidedApply`),
with an active recorder only: the first cut flagged a pass whose recorder
was suspended and declined the in-place rows. The witness declines now,
as a region beside a const does.

**NUR255, recorded (loud).** The root's concrete variants, `[(0 ([0] =>
[1])) 7]`, bail at `STORE_LOCAL`. The call site stores two results where
the lambda's call unit returns one, and the lambda's stored-body unit pushes
its unnamed param beside the result. Left open.

**Ledgers.** lang `compileDefectCeiling` 340 -> 339 and
`bailDefectCeiling` 38 -> 36, each composition named at the constant.

**Pins.** lang `TestNUR247ApplyWordParkCount`,
`TestNUR249ZeroArgCarrierUnderAWindow` and `TestNUR246ParkedApplyCount`
(rewritten), `TestNUR254UndecidedPatternPark`; core
`TestNUR254UndecidedPatternWindow`, `TestNUR254RecordUndecidedApply`.

## NUR245 closed, NUR253 recorded and closed: differing arm shapes, a void branch's phantom (2026-09-26)

**NUR245's open half: arms that disagree on the fn's shape.** The routed op
already did what such a call needs. It plans the live binding's signatures
over the window, raises the interpreter's no-match byte for byte, and enters
the live signature's own unit (`specFnUnit`, by declaration site). Only the
join's model stood in the way. A decided condition's model is now the
running arm's fn whatever the other arm declares. Undecided arms that differ
only in a parameter or return type agree on everything the routed op's
claim fixes (`claimCompatible`), and join to a WIDENED model: the then
arm's fn with each type joined and no declaration site, so the unit a call
site compiles from it is never entered. The first placement's units, which
call sites no longer compile, are compiled at the second placement.
`def m {e: false} if (m 'e' get) [f:Integer 7] [f:String 8] end 3 f`
raises the interpreter's `cannot call f`, `"x" f` answers 8, and each arm's
return contract answers under the family's name. Arms that differ on the
claim (an arity, a pattern, a return count) keep a sound decline.

**NUR253, found on the way.** The decided `3 f` row's compiled report listed
a `None` argument. An `if` whose arms leave nothing returns a phantom None
while recording is live: the result it registers so `RecordCall` elides its
dispatch, never seated. The phantom sits on the pass's tape, and two readers
took it for a value. `FoldFullStack` counted it (`if true [def k 1] [] end
depth` answered `[1]` compiled for `[0]`, silent), and a definite
no-match's TRAP serialized `sigError` over it. The fold skips it now: a
shuffle's index counts the run's entries, with the phantom kept in place.
A no-match report's stack prefix is the run's (`runPrefix`), which the
trap's `sigError` and the rematch's written tuple both read. Two pinned
declines in `TestSpeculativeUndefIsPlacedAndReadLive` changed reason with
it: the rematch had declined over the phantom as its only operand, and now
declines with no tuple to render, so the recovery's failure stands.

**NUR241 and the real programs.** Gate18 (on b7e384d) failed only on
`TestRealProgramsCompile`: the word-led latch declined mini-s3.boru and
echo_s3.boru at `slice from (add 1 upto) data`. The "narrower window" it
found was slice's two-operand overload over the two collected values alone,
which is arity pruning over forward operands, not the stack-style reading
the planner prunes to. A fitting window must now draw at least one value
from the stack beneath the word. Both programs compile again, and NUR241's
witnesses still decline.

**Ledger.** lang `compileDefectCeiling` 339 -> 340: the NUR253 pin's
out-of-range `pick` over a phantom, a pinned decline where the interpreter
raises.

**Pins.** lang `TestNUR245BothArmsDefineOneFn` (rewritten: the differing
types compile, the differing arity declines),
`TestNUR253ZeroOutPhantomIsOnNoRunsStack`; core
`TestSpecFamilyBothArmsJoin` (decided and widened models),
`TestNUR253RunPrefixSkipsPhantoms`, and `TestNUR241NoAmbiguity`'s
arity-pruning negative.

## NUR252 closed, NUR242's count program fixed: a foreign fn value's count (2026-09-26)

Probing NUR242's shaped method apply turned up a silent twin. A module
export fn VALUE whose body leaves the wrong count, read off a
factory-returned map (`def mk fn [[] [Map] [{f: M.inc/v}]] end def m (mk)
end m.f 5` over `inc [[n:Integer] [Integer] [n 1]]`), answered `[5 1]`
compiled where the interpreter raises `inc: expected 1 return value(s),
got 2`. The member read leads the residual's dynamic apply, whose foreign
arm (`dynApplyForeign`) hosts the module's stamped VALUE unit. That unit
declares no contract of its own (the Apply kernel's frame carries the
applied value's contract for exactly that reason, `applyRetContract`), and
the arm ran it under the trim discipline. It runs it under the named
discipline now, as the interpreter's cross-registry dispatch runs a foreign
value strictly, and checks the results against the applied value's
declared contract. Recorded and closed as NUR252.

Its record-literal twin (`def m {f: M.inc/v} end m.f 5`) took the shaped
method apply (`OpCallDynMethod`) and bailed with "result count 2 violates
the host-registered shape claim 1": the op read any count that missed its
claim as a host registration's violation. A single-signature boru fn
value's miss is the interpreter's own count error, so the op raises that
(`namedFnCountError`, `NamedFnReturnCount`'s text) and `do [m.f 5]`
catches it on both lanes. That is NUR242's count program; the three
re-step landings, the overloaded member's paren apply and `fold` stay open.

**A refinement of NUR207's residual island.** The merged corpus showed two
rows (callbacks.tsv L159, patrun.tsv L41: `def h (find …) end h {…}`)
islanded where they had compiled natively. The read leads the residual's
leading-form dynamic apply, and over a window the value matches that apply
IS the word dispatch's answer. Such a point deopts only on a no-match now
(`DeoptSpec.NoMatchOnly`), which the value apply parks and the word
raises.

**Pins.** lang `TestNUR252ForeignValueKeepsItsCount` (the three raising
shapes, beside a value that keeps its count under each).

## NUR241 closed as a sound decline: the word-led arrival (2026-09-26)

`acc "x" append acc (m.path) append` compiled to a wrong `cannot call
append`. The first `append`'s forward window is the word `acc` and a
paren. The interpreter's planner evaluates the paren first and prunes to
the all-stack window when the value misses its slot. The check pass planned
the paren unevaluated, the word-led window was its deferred plan
(`preferWordSig`), and the paren's gradual value arrived in the FlexList
slot optimistically.

The fix is the restriction the record asked for, at the arrival.
`insertForward` marks a deferred word-led plan's Forward
(`ForwardInfo.WordLed`: a Word next, no name capture at the chosen
signature's first slot). A compiling pass flags the gradual split when an
unproven value arrives into such a window and a narrower window fits the
stack beneath the word, over any of the word's signatures
(`narrowerWindowFits`). The program declines loudly, NUR305's discipline.
`1 2 add (m.v)` (not word-led), `add x (m.v)` over an empty frame (no
narrower window), and a proven arrival compile as before.

The first cut over-declined, and thirteen lang unit tests caught it. The
word-led token's own arrival (`pf d`, `helper opts`, `zp n`)
is the gradual first operand of every `f x`, so only an operand the plan
took past the word asks. A boru fn's synthesized 0-arg Fallback fits any
stack, and `PlanMatch` never plans a window from one, so neither a Fallback
nor a 0-arg signature counts as a narrower window (`g k (id 5)` over two
2-arg overloads compiles). The ledger moves by the three pinned witnesses
alone (336 -> 339, all "forward/stack split depends on a gradual
operand").

**Pins.** core `TestNUR241WordLedArrivalIsAmbiguous`,
`TestNUR241NoAmbiguity`, `TestNUR241WordLedPlanIsMarked` (core's own suite
covers the new code at 100%), and lang `TestNUR241WordLedArrivalDeclines`.
A run-time re-plan of the window, which would compile the program, stays
open.

## NUR207 and NUR208 closed: the root's gradual def read, a branch of fn values (2026-09-26)

Main's two new records were silent wrong answers on the merged tree, so
they came first.

**NUR207: the root had no plan for a gradual def read.** A bare read of a
def-bound value the pass types gradually is the interpreter's WORD dispatch
whenever the binding holds a fn. Inside a fn unit that read was already
planned (NUR123), and every witness answered correctly there. At the root,
`NoteWordRead` returned early, so the read lowered to a data push:
`def j (mk) end j` answered `[fn j]` for 42, and `r 'x' 3` parked where the
word raises `cannot call r`.

The root now records these reads (`noteRootWordRead`) and `Finalize` plans
them (`planRootWordReads`) with the fn units' own machinery:

- **A residual read** is tested once the residual is laid out
  (`seatRootResidualReads`). Where the laid-out residual is the
  interpreter's state at the read, the point is an island from the read's
  token over the values beneath it (`DeoptSpec.Beneath`). Its residual ends
  the run (`Program.Deopts`, `Program.Body`, the VM's `deoptEntry`).
- **A consumed read** takes `deoptStatementStart` and `deoptDeferred` over
  a pseudo unit record of the root. Deferral is asked of a push-tested point
  too, since the root lays literals out at the program's end: `7 j typeof`
  would otherwise have pushed j over nothing and answered `[Integer]`.
- **The binding.** The root's def writes its value (`bindGlobal`), where the
  interpreter's `def` installs a fn. So the island installs the read's
  value under its name in place of the write for its run (`bindRootRead`).
  Stacked on top, the no-match listed the candidate twice.
- **Everything else is a guard** that raises loudly when the value is a
  fn: `7 j typeof`, `5 [j]`, `(j)`, `def k j`. The global bind's own
  re-push is flagged a binding push now, so a push-tested point is never
  taken by the def.

**NUR208: two record-side misses.** The residual's may-be-fn lead arm
applied a branch result without asking placement; it asks
`leadPlacedNotRead` now, as its siblings do. `apply` over a branch result
was elided by the registered-output arm, since the identity result carries
the branch's id. The lead now registers the word's pending application
(`mayBeFnBranchResult`), and it lowers as `OpCallDynApplyTop`.

**Pins.** lang `TestNUR207RootGradualDefRead` and
`TestNUR208BranchOfFnValues`, positive and negative. Docs: NUR.md (NUR207
and NUR208 FIXED), the handover, this entry.

## The merge of main's #510 (2026-09-26)

Main's #510 (S2b declared, runtime defers 51 → 7, real programs 62 of 62,
sweep failures 13 → 3) conflicted with the run in eleven files.

- **Fixed on both sides.** NUR158 and NUR170 were each closed twice. The
  merge keeps main's fixes, which go further. The modifier natives wrap a
  compiled closure itself (`wrapCompiledClosure`), and a gradual macro lead
  records the module's runtime fn-dispatch, so `emit m.up {a:1}` compiles
  where the run declined it. The run's tests for both still pass, and
  `TestEmitDynamicLeadIsNotRoutedAsData` now requires the compile.
- **Both changes kept.** `core/go/bind_twin_apply.go` carries the run's
  type-name check and part reservation, and main's re-adoption of a node
  the check pass retired (class.tsv L98–L101).
- **Numbers.** Main's new NUR208 keeps its number. The run's NUR208 (a
  loop's iterator surviving a trapped raise, FIXED) is NUR251 now; the
  register, this log, the handover and one test comment say so. Main's
  NUR207 and NUR208 are open records in the index.
- **Main's tests over the run's fixes.** A predicate's refusal over a
  concrete candidate is a static check error (NUR141), so the plain-error
  test's predicate row is a run-time one now. A make field's refusal is a
  type_error (NUR310), so the typed-def wrap carries the code. fn's
  0-argument refusal (NUR091) adds two `gen`-chain `def` forms to the S2b
  census (32 → 34, 58 → 60).
- **The ledgers, measured on the merged tree.** Unit-suite compile 335
  (the base's 298, main's +4, the run's +34, less `emit m.up {a:1}`) and
  bail 34 (main's −7 and the run's −6 overlap in the predicate
  validate-FAIL). Corpus bail 5 and runtime defers 3 (main's rows less
  NUR190's). Sweep failures 4, since the run declines `case 2 M.cl`, which
  main pinned as a divergence. Call-form failures 284 and call-form crashes
  0. Arity pins: engine.go 30, native_macro.go 5. SWEEP_STATUS.md is
  regenerated.

**Found at the merge.** Main's NUR207 and NUR208 are silent wrong answers
on the merged tree: a def-bound gradual fn read at the program root is
pushed as data, and a paren-placed branch of fn values is applied. They are
the run's next records.

## NUR248 closed: one matcher for a type literal at a slot (2026-09-26)

The interpreter disagreed with itself. Every dispatch refuses a bare type
literal at a concrete-payload slot (`rejectsTypeLiteral`), except an
anonymous lambda re-stepped at a paren's close. That case fell to
`ExecFnDefSigStackMatch`, whose test was `SigTypeMatches` alone. The VM's
`MatchFnSig` had the mirror defect: it tested the node's Parent, its
SUPERTYPE, and so refused `Integer` at a `t:Type` slot. Both ask
`stackSlotAdmits` now (`SigArgMatches` plus the refusal outside a type-arg
slot). Core's own suite covers it.

## The silent halves of NUR249, NUR247 and NUR250 (2026-09-26)

A named fn-typed carrier applied over a paren window can turn out 0-arg at
run time. It then fires over nothing and leaves its window beside its
result: n+1 values where the record seats one. A consumed layout took the
wrong count silently under a no-contract fn (`[(k 5)]` compiled `[7 [5]]`).

A static mark cannot separate that lead from the comparator convention's
`(a b comp)`. So the plan's operand scan marks a named-head apply whose
result a later event consumes (`markDynOneResults`, eventFlags
`dynOneResult`), and the op seats `DynApplyHead.OneResult`. Its 0-arg arm
raises when the run leaves anything but one value.

That is a loud bail on the ledger instead of a silent wrong answer, by the
maintainer's rule at `runtime_defer_ledger_test.go`. The unit-suite bail
ceiling rises 36 → 38 with the two witnesses, named beside it. In place,
the n+1 values stay the interpreter's answer.

**NUR247's silent half.** The same scan marks an `apply`-word event a later
event consumes. It takes the word's existing one-result form
(`OpCallDynApplyOne`), so a lead that parks raises where `[(5 f/v apply)]`
answered `[5 [fn]]` in a no-contract fn. The Church-encoding and CPS rows
are unaffected, since their applies net one value. The bail ceiling
rises 38 → 40 with two witnesses.

**NUR250, found closing NUR247** (pre-existing, silent). A bare fn-param
read before `apply`, `(5 f apply)`, is a call on the interpreter (NUR078):
`f` fires at the word and `apply` meets its result. The compiled lane
handed the value to the word. Such a lead is no longer the word's pending
apply: it takes the dynamic lead's existing decline. The compile-defect
ceiling rises 330 → 332 with two witnesses.

## NUR246 and NUR239 closed; NUR245's same-shape half; NUR247–NUR249 recorded (2026-09-26)

**NUR246 was a silent wrong answer, not only a loud one.** A paren-bounded
fn-value apply whose lead the window does not fit PARKS on both lanes:
- An anonymous or `/v`-delivered value that matches nothing is data.
- The paren nets the window AND the value.
- The recorder claimed one result.

In a fn frame that surfaced as a return-count raise. At top level every
fixed layout over the park was silently wrong: `[(5 lam/v)]` compiled
`[5 [fn]]`, and a map value, an interpolation and a reordered residual
(`1 (5 lam/v) 3` compiled `[5 1 fn 3]`) the same.

`recordDynApply` now marks such an apply a variadic REGION unless its count
is fixed. The count is fixed in three cases:
- a bare read of a named binding, which raises its no-match;
- a named fn value baked as a constant under a trailing window
  (`raisesTrailNoMatch`);
- a window that provably fits (`applyWindowFits`).

The loop region's rules do the rest. It seats in place, or collects through
the region mark when a top-level list literal takes it alone (`[(5 lam/v)]`
compiles to the right answer now). Every other fixed layout declines,
including all fixed layouts in a fn frame.

The `apply` word stays unmarked. Its lead is gradual by construction, and
marking it declined 14 Church-encoding and CPS programs of
`bytecode-migrated.tsv`. It is recorded as **NUR247**.

Covering the fit proof turned up **NUR248**: a type literal under a
lambda's value pattern matches on the interpreter and parks on the VM
(`(Integer ([0] => [1]))`, silent, pre-existing).

**NUR245, arms that agree on the fn's shape.** The family was already
speculative (NUR244), so its calls route live. Three pieces were added:
- **The join's model.** The join pushes a model instead of the
  payload-less carrier: the running arm's fn under a decided condition,
  the then arm's otherwise, when the shapes agree.
- **The decided running arm's twin.** That arm's def is noted as a taken
  arm's, so its bind twin keeps the live registry current. Without the
  note the routed op met an unbound name.
- **The undecided other arm's unit.** It is compiled where it is placed,
  under the family's name.

Arms that disagree on the shape keep the standing decline.

**NUR239's binding half.** The label came from the island's INTERPRETER
return check, not a VM RET. The island stepped the fn value, which
dispatches through its `def`'s baked handler under the def's name. The
interpreter's `(k 5)` dispatches the word `k`. The VM's 0-arg island arm
now does the same, over a frame binding of the head's name
(`InstallFrameBinding` / `UninstallFrameBinding`). Its count on the way out
is **NUR249**, recorded OPEN: a named fn-typed carrier that turns out 0-arg
under a window leaves n+1 values, silently wrong in a no-contract fn. The
static mark cannot separate it from the comparator convention.

**The compile-defect ledger** rises 319 → 330 with the new witnesses, named
beside the ceiling: NUR246's eleven fixed layouts and NUR245's
differing-shape witness, less the one both-arms program that compiles now.

## The merged ADR-008 gate on d493ef4; NUR246 recorded (2026-09-26)

The merged gate measured 80248/80250, down from 148 uncovered statements
at the start of the coverage work. Three items remained, all closed now:
- **`deoptStatementStart`'s final decline.** NUR322's case now takes the
  direct spliced consumer that used to reach it. A deopt unit test reaches
  it with a top-level read whose consumer stands inside an earlier paren.
- **`parkedWindow`'s trailing arm.** NUR238 restricted the FnDefInfo
  caller to leading heads, so only the closure arm reaches it: `(5 f/v)
  size` over a param-held capturing lambda, pinned in NUR238's test.
- **A stale pragma.** `If2ReturnsFn`'s literal-false arm is reached by
  NUR244's `if [false] …` row, so its false proof is gone.

**NUR246**, found while covering that arm (pre-existing): a parked trailing
window leaves n+1 values where the call event claims one, so a list literal
over it assembles the wrong count.

## NUR244 closed; NUR245 recorded; CI on 3306b85 (2026-09-26)

**NUR244.** The parser case was one instance of a wider bug. A fn def in
an arm that may not run was bound past the merge, and later calls ran it:
- **The arm a decided condition skips.** A literal, def-bound or folded
  Boolean condition bracketed neither arm (`condKnown`). Their join is the
  undecided join, which keeps a fn value as it is (condBoundCarrier wraps
  only plain values).
- **An else-less if's arm.** `If2ReturnsFn` bracketed nothing, even for an
  undecided condition.

`armsKnownToRun` brackets every arm except the one a decided condition
takes. A fn def in a bracketed arm is a speculative family: `3 f` past the
merge raises undefined_word where the arm did not run. A redefinition in a
skipped arm now compiles.

The `parse` macro declines for a speculative family's name read outside
any speculative arm. It reads the binding as a value, so it declines
through the family's value-read site. At run time the interpreter resolves
an unbound name as a kind (NUR109's rule), and no op re-resolves one. A
parse in the defining arm itself still compiles. The decline goes through
an existing site on purpose: both compile-failure censuses count sites,
and a new one fails them.

A redefinition in a skipped arm now compiles, so
`frontier-conditional-fn-shadow.tsv:15` graduated to `control.tsv` §9.
The diagnostic-parity ledger rises 350 → 352 with it (CI's langspec shard 3
on d493ef4; both rows named beside the ceiling). The two graduated rows carry
the constant-condition `unreachable_branch` advisory that only the plain pass
emits, the ledger's largest shape. The frontier file was never in that walk.
Its decided-condition twin moved from `mustFailToCompile` to
`mustCompileWithParity` in the edge-finding test. The taken arm's
redefinition still declines.

**NUR242, the `do` half.** A `do` body that runs to nothing without a
definite raise nets 0 values, or 1 caught Error. The check pass modelled 1.
`DoListReturnsFn` now latches the count runtime-variable. The unnamed-param
prefix seats at unit start for any variadic call or dynamic-body run. The
list literal and the fixed-width replay window, which need the count,
decline instead of bailing. The list literal's fence reads its own mark,
`eventFlags.catchVariadic`, set only where the catch latch is consumed. A
first cut fenced every variadic result, and that declined two programs the
suite compiles: a dynamic-body `[(do [f/v])]` and a lambda over a dynamic
map, whose counts are fixed in practice.

**NUR245**, found on the way (pre-existing, a compile defect): when both
arms define the same fn, the join is a payload-less carrier, and a call
past the merge fails to compile.

**NUR240** needed no change of its own. NUR238's value-trail no-match
already gives `(true 5 M.dec)` inside an arm the interpreter's
`uncalled_function`, where the unit trap does not reach. It is pinned on
both lanes.

**NUR241**, diagnosed but still open. The divergence is a forward split,
not the walk hook. The runtime planner pre-evaluates the paren `(m.path)`
and prunes the forward window on its value. The check pass defers the
window (`preferWordSig` → `bestDeferred`) and parks the word, and the
paren's gradual Any arrives optimistically. The fix belongs at the
arrival.

**CI on 3306b85** failed two jobs:
- `cover-gate-core`: core's own suite never reached `noteCallWindow`. A
  core unit test drives it now, and another reaches the splice arm whose
  stale pragma went in the previous commit.
- The arity gate: two new comparisons, each pinned with its rationale.
  `ClosureCallsAtLanding` is the argument rule over an empty supply.
  `valueTrailNoMatch` asks whether a value has any own signature.

## NUR238 closed; the merged ADR-008 gate's last blocks (2026-09-26)

**NUR238.** A value applied as a trailing window it does not fit now
follows the interpreter's re-step of the value. An anonymous value parks
as data, and a named one raises `uncalled_function`, at the top level
(which now seats its apply heads, `Program.DynApplyName`) and in a fn
alike. A bare read under a frame binding keeps NUR107's `signature_error`.

**The merged gate at 852a10a** measured 80176/80191, down from 148
uncovered statements at the start:
- **Main's #509** left 7 blocks: `runsBodyOnRegistryAtModuleScope`'s
  effect re-test, which is deleted (its only caller switches on the
  effect); `bodyRebindsBoundName`'s arms; `soleSigParamsNominal`'s
  untyped slot; `lowerLoop`'s unpromoted event start.
- **Older main code** that newly showed up after the merge left 4 blocks:
  `RecordLoop`'s range operand check and `valueRefsName`'s paren arm.
- **This run** left 3 blocks: NoteLoopFresh's guard, an unplaceable
  signature forward, and a window value with no identity.
- **A stale pragma**: core/go/engine.go's cross-registry 0-arg splice arm,
  now reached. The pragma is removed.

All are covered by unit tests.

## NUR243 closed; NUR239's anonymous half; NUR244 recorded (2026-09-26)

**NUR243.** Three valid programs compile now:
- A constant branch whose taken arm leaves no value is recorded as a
  0-value statement: a phantom None result, marked zeroOut. Its guard's
  covergate pragma had a false proof; it was removed in the previous
  commit.
- The loop carry no longer returns on a branch-join pre binding. Its
  comment blamed the loop index, which never reaches it. The loop reuses
  the branch's cell, so no init reads the pre early, and the post-loop
  binding stays bound-checked: a zero-iteration loop's read raises
  undefined_word, as the interpreter's does.
- NUR109's parser arm checks the parser operand alone. Its bound-slot test
  is deleted: a parser is a fn value the join never carries, and a
  branch-carried kind name fails the check pass first.

**NUR239.** The anonymous half is fixed: a nameless fn value's return check
names its frame `<fn>` (`core.FnValueFrameName`). The binding half stays
open.

**NUR244**, found probing NUR109's arm (pre-existing, a silent wrong
answer): a parser bound only on a skipped arm, spelled as an inline `fn`
literal, bakes as a value and runs compiled. The interpreter finds no
binding and raises `parse_unknown_lang`. NUR109 catches only a promoted
call-result parser.

## The three silent wrong answers closed: NUR321, NUR322, NUR237 (2026-09-26)

**NUR321.** A fn value's closure unit is shared by every value over the
same body and inputs, because the memo key holds neither the name nor
the anonymity. `lambdaUnit` read every fn value as the anonymous `=>`
flavour, so the landing parked a `fn` literal's named value where the
interpreter calls it.

Splitting the memo key was tried and reverted. `AnalyseFnBody` keeps its
own memo, keyed by the unit name, and the unit memo and the analysis memo
must move in lockstep. A split key opened a fresh unit whose analysis hit
the old summary and recorded nothing, which declined `m.g 3` as "body
result of unknown provenance".

So the name rides on the push, as the return contract does:
`ClosureRetSpec.Named` becomes `ClosurePayload.Named`.
`ClosureCallsAtLanding` fires a named closure over a nullary unit, and
the bridge reads `ClosureIsAnonymous`.

**NUR322.** The per-read deopt ordered a read and its consumer by source
position. A spliced word's expansion carries its DEFINITION's positions,
so the point declined and the read kept its slot push. A consumer from
outside the body is now ordered by the event stream.

**NUR237.** A top-level read of an S5 loop-split name reads the registry.
The program's residual resolves after the events lower, so its rescue
came too late for the arm's def (`loopSplitRebind`: every later root def
of such a name is registry-visible).

## NUR320 closed, main's #509 merged, and the merged ADR-008 gap covered (2026-09-26)

**NUR320.** A compiled user call's param-contract no-match now reports
the interpreter's window: the written run, which a bare read ends, filled
from the stack beneath to the smallest arity. The check pass derives it at
the user fn's dispatch, over its own tape, with sigError's own derivation
(`noteCallWindow` → `rematchWritten`). It takes it at the FIRST step,
because that is where the run fails: its plan sees every operand but a
speculative slot's. A speculative plan offers no window. The offer is
keyed and held like the region offer (`NoteCallWindow`, held at the
ReturnsFn's entry by `HoldRegion`). So a forward collection's force-stack
re-step keeps the first step's offer, and the callee's body analysis
cannot overwrite it.

The record maps each window value to where the run holds it:
- an argument, by identity;
- a definite scalar, by value;
- an event result, by its seat: a promoted slot, else its depth beneath
  the call's operands on the simulated stack;
- a local read whose binding had not moved when the window was offered.
  That is judged at the offer, because the callee's analysis moves a
  same-named param's generation.

The lowering writes `CallWindows[pc]`, and the VM reads it when the
contract fails (`callWindowAt`). A value with no home keeps the argument
tuple. The residue is a local rebound between the read and the call.

**The merge.** Main's #509 (S2b's first cut) merged cleanly apart from
three ledgers, each composed from both sides' moves:
- compileDefectCeiling: base 291, main +7, this run +19, so 317;
- bailDefectCeiling: 36;
- the arity gate's engine.go: base 29, NUR078 −2, main +1, so 28.

The merged tree then compiled one program it should not have.
`def h fn [[k:Function][Any][typeof k/v]]  def f fn [[g:Function][Any][h
g]]  f ([] => [42])` answered `[Function]` compiled, where the
interpreter raises signature_error. Main's single-overload recovery
(`TryRecordRecoveredUserFn`) built its window from the fallback walk,
which admits any word, so it bound `g`. But under NUR078 a bare fn-bound
name calls wherever it is written, so the interpreter's matcher stops at
it and h gets nothing. The recovery now refuses a forward word the
planner's own fn-binding rule says calls (`forwardFnCall`). The program
declines as `TestWordReadDispatchFailsToCompile` pins, and the ledger is
back at 317.

**The merged ADR-008 gap.** Four agents covered the blocks the merged
profile left: the PR's 73 statements, a stale pragma, and main's own 80.
All of it is tests, apart from a few provable simplifications:
- emit.go: `RecordTypeInstall`'s keep-defs guard, `bailPoint`'s start
  fallback and `RecordArgsProjection`'s lookup;
- check: `method_shape.go`'s `LookupWord` fallback;
- the VM: `reStepLanding`'s `dynApplyEnter` arm, and the render-only
  no-match bridge now takes no invoke.

One pragma was added, at check_recovery.go's window-size bound, with its
proof. One pragma was removed because its proof was false:
native_control.go's constant-branch guard is reachable. The agents' probes
found fifteen divergences, all measured and recorded OPEN as NUR321, NUR322 and NUR237–NUR243:
- three silent wrong answers (NUR321, NUR322 and NUR237);
- a trailing-apply family (NUR238);
- diagnostic names and codes (NUR239, NUR240);
- a walk-hook capture (NUR241);
- eight compile-then-bail programs (NUR242);
- three over-declines (NUR243).

## NUR308's type half compiled — the run-time type install (2026-09-26)

**The record.** NUR308's first cut left a TYPE over a refinement whose
bound only the run knows declining the compile at its three sites (a
named install, an inline typed def, an inline signature type:
`DeclineUnknownRefinement`, one census site, 91 → 92), with the plan as its
disposition: install the type at run time and check against it there.

**The fix.** The type installer notes a body holding an unknown-bound
refinement, in a union, negation or typed container's child too, and the
def's dispatch records the body operand and `OpBindTypeRun` in the def's
place. The run installs the type through the interpreter's own
`InstallType` (a mint, or Never adopted for an empty interval), and the
node the pass minted forwards to the run's. That node is the one every
compiled reference names: a type operand pushes the run's node, and a
signature slot or typed-bind spec asks membership, unification, rendering
and equality through it. So named params, returns, class fields, generic
bounds, `is` and `T eq Never` agree without re-planning a single
signature. The def's type twin is written back and replays nothing. A
typed def records the run's own check (`TypedBindRunMembership`, against
the node or the inline constraint the run computed). An overload set over
such a type re-matches at run time, because the pass's match over an
unknown bound admits every value. The pass decides nothing over an
unknown bound anywhere: an intersection keeps it, and a complement admits.

An inline parameter or return type over such a bound compiles the same
way. In a compile pass its pattern is an anonymous node the run forwards,
through an unnamed `OpBindTypeRun`, to the node it mints from the
refinement it computed. The anonymous node takes the run's rendering, so
the no-match notes agree. A refinement with any unknown bound now decides
nothing in the pass, its known side included: a verdict the known side
gave alone was baked with the placeholder rendered.

Three shapes still decline, through the existing compile-time-word site:
- an inline interval over such a bound, which the run may find empty (the
  interpreter's slot is then `Never` itself);
- a typed container's child;
- a fn body's per-call type def.

The retired site takes both censuses back to 91. The frontier row and
oracle row for a type def in an each body decline one step earlier, as
the code-body word.

Found on the way, both pre-existing. NUR310: a make field's refusal was a
plain error, bare interpreted and a compiler defect compiled; it is a
type_error on both lanes now, CLOSED. NUR320: a compiled direct call's
contract no-match reports every argument where the interpreter reports its
attempted window. The notes differ and the code and head agree. Recorded
OPEN with a proposed fix: NUR122's written-run carried to user calls.

## NUR009 closed — Bytes a refinement base, a computed bound the run's (2026-09-26)

**The record.** Bytes, the one ordered scalar leaf the comparison words
would not refine, held open on the maintainer's verdict: close it through
the refinement-base CAPABILITY — a type declares its participation — never
by adding one more leaf to the resolver's list.

**The fix.** `core.DeclareRefinementBase` stamps the capability where each
owner registers its type (core its six leaves, basic Bytes), and
`canonicalBaseType` reads the declaration. Making Bytes whole on both lanes
needed three more: the inline-signature resolver slots a refinement at its
own base (a Bytes one was a wildcard), a refinement renders as one over
Bytes' Formatter (every one printed `Bytes<?>`, and the const pool merged
two by that key), and `convert Bytes <String>` folds over a const, since a
Bytes bound is never a literal. Pinning it found NUR308 — the check pass
built a refinement over a computed bound's CARRIER and the compiler baked
it (`3 is (Integer gt (size "abc"))` true compiled): a constructor over an
unknown bound now records as a run-time call, a type over one declines the
compile loudly, and the pass decides no membership over one — and NUR309,
an inline refinement return refusing an abstract residual at check time
that the named twin defers. Both CLOSED.

## NUR026 closed — one escape vocabulary, one malformed-escape report (2026-09-26)

**The record.** Templates took the quoted-string escape vocabulary on
2026-08-15; a malformed `\x` / `\u` still read literally in a template
while a quoted string refused it, and the record held that residual open
for want of an error channel in the template's lex matcher.

**The fix.** A tabnas matcher can return a bad token, so boru owns the
escape check in both ports with one definition (`escapeFault`): the
template matcher and a new `string_escape` matcher ahead of jsonic's string
lexer refuse a malformed escape alike, naming the escape. Probing the
matrix also found the quoted form split between the ports (NUR306) and the
vocabulary short of the braced `\u{…}` form in templates and, in Go, of
surrogate pairing (NUR307); the shared escape readers now read both. 34
escape rows in parse.tsv, parser coverage 100% in both ports.

## NUR060 closed — the parser parity ledger is empty again (2026-09-26)

**The record.** `parser/spec/divergent.tsv` carried nine classes of source
the Go and TS parsers rendered differently (a 2,587-source probe sweep).

**The fix.** Each class got the rule its agreed neighbours implied, in both
ports: a bodiless `=>` where the arrow folds is refused on the arrow; a
typed list child with no value is an empty element; a `]` never closes a
list no `[` opened; TS no longer throws for an empty unclosed group ahead
of its converter, so both report the first fault in source order; the value
converters refuse an unclosed member group; a bare `/` modifier is a
syntax_error; an empty `${}` no longer eats its `}` and contributes nothing.
24 rows moved or added to parse.tsv, 12 re-rendered, the ledger pinned at 0
in both runners; parser coverage 100% in both ports; `make parser-parity`
green.

## NUR063 closed — boru:scry ships the seven, the debug copies deprecated (2026-09-26)

**The record.** The maintainer ruled `boru:scry` canonical for the seven
self-knowledge words `boru:debug` shipped first, with the debug copies
frozen behind the same handlers and deprecated on a stated timeline; the
record stayed open until scry shipped with the notice.

**The fix.** `lang/go/modules/scry.go`: one constructor (`selfKnowledge`)
builds the seven for both modules — a surface supplies only the word's name
in errors and the unknown-word code. `describe` marks each `Debug.*` copy
deprecated, naming its `Scry.*` twin and the removal release (the first
minor release after the one that ships scry). `lang/spec/module-scry.tsv`
covers every export (ADR-003) and pins the two surfaces equal under `deq`;
check-accuracy pins the file's two runtime-only unknown-word rows. Found on
the way: frontier-do-catch.tsv row 25 still expected the pre-NUR072
`word(if)` rendering of a fn body in a map — updated to the canon spelling.

## NUR064 closed — add patterns bind as receive clauses do (2026-09-26)

**The record.** A `receive` clause's pattern routes on its scalar fields and
binds its `name:Type` fields into the body; the design had a service `add`
pattern route only, and the code in fact refused a typed field outright. The
maintainer deferred the choice to when both modules were built — they are.

**The fix.** One clause pattern (`splitClausePattern`), one guard (a routed
handler's slots decide whether it takes the request; a declining one falls
to a slot-free catch-all or raises `no_match`), one binding (frame bindings
around the clause's code — the `receive` body, the `add` handler). Layers
keep their own slots. The checker analyses the handler's fn literal before
`add` names the slots, so `add`'s check half notes each slot-reading token
(`SlotBoundReads`, name and position) and the undefined-word rescue excuses
exactly those. Both lanes agree on every pinned shape; the handler's stamp is
unaffected (a body the stamp declines interprets, per body).

## NUR305 closed — the gradual window declines (2026-09-26)

**The divergence.** Found probing `send` beside a `receive` (NUR064):
`def v (whereis "x") v send {a: 1} "nobody"` answers `[None]` interpreted
and raised signature_error compiled. `send (Any, Pid)`'s scan stops at the
String and fills the Pid slot from the stack; the check pass's dynamic
carrier matched it optimistically, so the compiled window was ONE forward
token plus v — the window the runtime takes only when v is a Pid. Measured
on main at 3b5db68 too.

**The fix.** `PlanMatch` notes an unproven stack match; with some forward
tokens taken and a later candidate whose own scan collects past the stop
token (`laterCandidateCollectsPast`), a compiling pass sets
`AmbiguousGradualSplit`, the latch the reverse case already used, and the
program declines loudly. All-stack matches stay the forward-drift guard's.
A speculative claim (a function word admitted at an Any slot for its
dispatch's result) does not count: the first cut declined kg/main.boru,
whose `var` body puts `__varundef` after an `and` over a `lt` result the
pass types `Scalar tor Boolean`.
Faithfully compiling such a window would take the drift window's island
generalised to mixed forms, and only a terminal window could absorb its
variadic result; left for the full-compilation line.

## NUR065 resolved — one set of guarantees for both classifier spellings (2026-09-26)

**The record.** `boru:state`'s classification role has two spellings in the
design (the module is unbuilt): the `classes:` table got define-time
alphabet closure, a frozen `{event raw}` payload and the `state_bad_class` /
`state_class_gap` diagnostics; the `classify:` fn form got none of them.
The maintainer deferred it to the state-machine line's open question #7.

**The resolution.** The run's goal is every record resolved, so open
question #7 is decided in design/STATE-MACHINES.0.md: the fn form declares
`yields:` (checked against `events:` at define time), returns a class atom
the machine wraps in the same frozen payload, and takes the same
diagnostics. Only the mapping inside the fn stays opaque.

## NUR302, NUR303, NUR304 closed — the fixpoint ledger is empty (2026-09-26)

**The divergences.** The canon fixpoint gate that landed with NUR072
ledgered 33 parse.tsv rows in three kinds: template strings and XML `${}`
holes in the debug `interp(…)` / `interp-xml(…)` forms (NUR302, 30 rows), a
map key needing quotes rendered bare (NUR303, 2), and a typed tag before an
XML literal fusing into an angle sugar (NUR304, 1).

**The fixes.** Both ports: `canonTemplate` / `canonXmlTmpl` render the
source (template escapes, XML escapes, holes over their tokens' canon);
`canonKey` quotes a key that would not lex back as itself; `joinCanonParts`
puts a comma between a bare capitalised token and a `<`. The ledger is
EMPTY (pinned at 0 in both runners): every parse.tsv row that parses reaches
its canon fixpoint in Go and in TS. The divergent ledger's two template rows
took the new spellings (Go folds `\`${}` to an empty literal, TS keeps an
empty hole — still a divergence, now spelled `\`\`` and `\`${}\``).

**Also.** `hasWordModifiers` is gone (a plain word renders bare, so nothing
asks); `FirstOwnSig` has its own core test now that core's predicate path no
longer reads one overload; cover-gate-core is back at 100%.

**Pins.** core `TestNUR302TemplateCanonIsSource`,
`TestNUR302XmlTmplCanonIsSource`, `TestNUR303MapKeysCanonAsTheirKey`,
`TestNUR304CommaSeparatesAnAngleReceiver`,
`TestNUR072SequenceRulesSpellMarkersAndFolds`,
`TestNUR072DisjunctCanonSpellsItsMembers`, `TestCanonFallbackIsTheDebugForm`,
`TestFirstOwnSig`; parser `TestParserCanonFixpoint` (Go and TS, empty
ledger).

## NUR072 closed — canon spells the sugar and the word; a fixpoint gate in both ports (2026-09-26)

**The divergence.** ADR-015 requires canon to render source that re-parses
to the same value. Three sugar kinds rendered their debug dump
(`sugar(mini m 'src')`, `sugar(type-bound [[w]])`, `sugar(lambda)`), a plain
word rendered `word(foo)` (the bare-word question the record left open), and
TS rounded a `/N` arity above 2^53.

**The fix.** ADR-015 answers the bare-word question: `word(foo)` re-parses
as the `word` splice over a group, bare `foo` as the Word — so words render
bare. The lambda marker is `=>` and its fold group renders without parens
(it re-folds); a mini literal renders `+name'src'` with the lexer's escapes
(the delimiter is not part of the value); the type bound `name/t`; a group
modifier after its group by a sequence rule (`(1 2) /s` — a fourth kind the
record thought unwritable). TS carries `/N` as a bigint. Go canon gained the
disjunct arm TS had. Both ports, the shared corpora rewritten row by row
where the only difference was the new spelling.

**The gate.** `TestParserCanonFixpoint` and its TS twin re-parse every
parse.tsv row's canon; 33 rows fail it and are ledgered in
parser/spec/canon-fixpoint.tsv against NUR302 (template strings, XML
holes), NUR303 (map keys needing quotes) and NUR304 (a typed tag before an
XML literal) — none of them NUR072's kinds.

**Also moved.** The lang bail ceiling 37 -> 36 (NUR273's refusal is no
compiler defect), the FnModel golden (`behave`'s ReturnsFn, NUR076), the
body-sig test's residual (NUR272: the seam answers CallBoru's residual).

**Pins.** core `TestNUR072SugarKindsSpellTheirSource`,
`TestNUR072UnspellableSugarKeepsTheFallback`; parser
`TestParserCanonFixpoint` (Go and TS); parse.tsv's new `/N` row.

## NUR074 resolved — a parameter's name is part of the function (2026-09-26)

**The record.** `canon` renders parameter names, so `([x:Number] => [mul
x x])` and `([y:Number] => [mul y y])` render — and, since canon is a
function's `deq`, compare — differently; the record took them to be
behaviourally indistinguishable.

**The measurement.** They are not, in general: a parameter is a frame
binding on the def stack, visible to every callee through boru's dynamic
scoping (FUNCTION-VALUE-SCOPE §7.4). `def x 1  def g fn [[] [Any] [x]]`,
then `def f fn [[x:Any] [Any] [g]]  f 5` is 5 and the `y`-named `f` is 1,
on both lanes. Erasing the name from canon, or from `deq`, would equate
functions that behave differently.

**The resolution.** No code change: the name is part of the value, and
canon and `deq` keep it uniformly. CONTENT-ADDRESSING.0.md §4.2 step 3
(de-name parameters) is withdrawn as unsound.

**Pins.** lang `TestNUR074ParamNameIsPartOfTheValue`.

## NUR075 closed — `eq` gets deq's capability (2026-09-26)

**The divergence.** `deq` consults a per-type `DeepEqualer` at DeepEqual's
terminal verdict and `behave deq/q` installs one; `eq` had no counterpart,
so the two halves of one word family were extensible on different terms.

**The fix.** `core.ExactEqualer`, consulted by `exactEqualCapability` at
ExactEqual's terminal `false` — DeepEqualer's placement, so it is additive
and reaches the same pairs (measured with the DeepEqualer fixtures: a
non-pointer host payload reaches both terminals) — and `behave eq/q` with
deq's shape, validator (`validateEqualitySig`) and body runner
(`runEqualityBody`), both now shared. No kernel identity arm moved.

**Pins.** core `TestExactEqualCapabilityAnswersAtTheTerminalPoint`,
`…DeclineAndFailure`, `…IsAdditive`; lang/native `TestBehaveEqSlotInstalls`,
`TestBehaveEqSeam`, the eq wrong-shape and untyped-param rows.

## NUR076 closed — the check pass notes a `behave make` (2026-09-26)

**The divergence.** `behave` does not run in check mode, so a type whose
own constructor (`behave make/q (fn Any P [make P {a: 42}])`) ignores its
source was still schema-validated at `make P {bogus: 1}`: two check errors
for a program that runs (Class/P{a:42}), a pre-flight refusal, and a
compiled-lane decline at the same check.

**The fix.** `behave`'s ReturnsFn is its check-mode half: it validates the
call with the handler's own `behaveTarget` and, for `make`, notes the target
in `CheckState.BehaveMakers`; `core.HasMaker` reads the note. Nothing is
installed during analysis — a wrapper would put user bodies within reach of
analysis-time rendering and comparison, and the other seven slots change
only computed values. The program now checks clean and compiles.

**Pins.** lang `TestNUR076BehaveMakeIsVisibleToCheck`, core
`TestNUR076BehaveMakerIsVisibleToCheck`; `describe behave`'s note updated.

## NUR100 closed — the predicate is a one-value application; the poly decline keys on a re-step (2026-09-26)

**The divergence.** ADR-016 forbids deciding behaviour by a function's
arity, and two sites did. `RunPredicate` admitted a function as a predicate
only if its first signature took exactly one parameter ("predicate must take
exactly one argument", raised at the use), and `tryRecordPoly` declined a
poly window whenever the word registered an overload taking FEWER operands
(`smallerArityOverload`).

**The fix, §1.** Membership is a one-value APPLICATION: `MatchFnSig` over
`[candidate]` — the matcher every call takes — picks the signature, and a
candidate no signature takes is not a member. The whole overload set is
consulted (it used to be the first alone), a value pattern selects, and a
predicate that cannot take one value answers "does not satisfy" on every
use; the declaration is not refused, because that refusal would be a count
again. `PredicateInputType` became the overloads' common input so the typed
slot's pre-filter consults what `is` consults.

**The fix, §2.** The count stood in for a declaration. The VM's poly
re-match already retries narrower windows (NUR147); what it cannot do is
re-step a result, which `apply`'s `[Function]` overload does. The decline
now fires when an overload declaring `CompileResteps` is reachable over the
dynamic operands (`restepOverloadReachable`). Every decline the corpus
measured was such an `apply` window, so coverage is unchanged.

**Found on the way — NUR272.** A predicate body that leaves its unnamed
input beneath its verdict (`fnpred [[Integer] [true]]`) answered one value
through CallBoru and two through the compiled seam, whose stored unit is
compiled count-agnostic; `0 is Z` split true/false. `InvokeCompiled` now
applies CallBoru's discard with the signature it holds
(`trimUnconsumedUnnamed`).

**Found on the way — NUR273.** The predicate branch of a typed def raised
its refusal as a plain Go error on both lanes, and the compiled run's error
boundary books any non-BoruError as a compiler defect (`internal_error` plus
the defect note). It is a `type_error` now, as the typed def's other
refusals are.

**Ledgers.** aritygate: registry.go 3 -> 2, compiler_dispatch_record.go
2 -> 1, the NAMED DIVERGENCE blocks retired; the gate also flagged the
NUR190 landing skip's `NArgs` read in lower.go, removed (the operand count
it duplicated remains). langspec `bailDefectCeiling` 46 -> 44, the NUR190
rows' ledger move that the previous commit left un-ratcheted.

**Pins.** lang `TestNUR100PredicateIsAOneValueApplication`,
`TestNUR272CallbackSeamDiscardsUnconsumedUnnamed`,
`TestNUR273PredicateRefusalIsATypeError`; compiler
`TestRestepOverloadReachable`; `lang/spec/fnpred.tsv` §8.

## NUR190 closed — the landing's island and skip take the `/q` capture (2026-09-26)

**The divergence (the contained half).** A dynamic fn value under a
function word its `/q` slot claims: the interpreter captures the word as an
atom and never runs it; the compiled code calls the word after the landing
and the residual arm applies the value over its result, so the landing's
walk deferred loudly (`vm:landing-quote-claim`, fn-value.tsv L317/L318 on
the runtime-defers ledger — the maintainer's containment of 2026-09-24).

**The fix.** Two answers, by placement. Where the word is in the body at
the landing's own depth (the program — whose tokens now ride on the
recording, `SetRootBody` — a user paren, a fn or closure body in an island
environment, `planLandingDeopts`), the landing carries an ISLAND
(`LandingWord.Deopt`/`Opens`/`Island`, built by `landingIsland`): the value
and the body from the word on go to the interpreter over the (empty) frame
region, and the run continues at the unit's RET or the program's end
(`landingDeopt`, a `dynEnter` jump). Elsewhere — a branch arm, a loop body,
a literal's member — every compiling landing is followed at once by the
word's single call and the paren apply over it, so the landing carries a
SKIP past both (`seatLandingSkip`, `LandingWord.SkipTo`), and the capture
runs over the value and the word alone (`landingSkip`). A blanket decline
of island-less walks was measured first and dropped: 125 corpus rows
(member reads under `eq` and friends inside fn bodies) would have declined.

**Ledgers.** fn-value.tsv L317/L318 left runtime_defers.tsv (the defer
census 7 -> 5) and run one landing island each (engine entries 183 -> 185,
interp-entry rows 36 -> 38, `vm:island-resolved` 7 -> 9); the lang bail line
40 -> 37.

**Pins.** lang `TestNamedFnCandidatesOpenShapes` (every placement).

## NUR271 closed — a dyn body settles its own lead (2026-09-26)

**The divergence.** `do [m.f 5]` over a factory's map was 6 interpreted and
`CALL_DYNAMIC underflow` compiled: the dyn body (tryRecordDynBody) ran the
body with the interpreter's semantics and left [6], but the model's [fn, 5]
residual had the program's residual arm apply the lead again. Loud,
pre-existing.

**The fix.** `dynBodySettledLead`: a residual lead with another result of
the same dyn-body dispatch above it stands aside in resolveDynamicApply —
the window was the body's; a lone dyn-body lead over a later token keeps the
apply (`do [m.f/v] 5` is 6).

**Pins.** lang `TestNUR271DynBodySettlesItsOwnLead`.

## NUR270 closed — a landed lead is no apply-event lead (2026-09-26)

**The divergence.** The gradual apply event (`recordGradualApplyEvent` →
OpCallDynApplyOne) applies its lead to the one value beneath. A bare
member read is no inert lead: `3 kv.v apply` over an inc member claims the
3 at the read's own step and apply raises over the 4 (interpreted), where
the event applied the member once and answered `{x:4}`. Silent,
pre-existing.

**The fix.** A lead whose producing event carries a re-step landing
(`landingAfter`) is refused by the event unless a `/v` read delivered it
or a user paren placed it (`nd (m get "inc") apply` — the re-step ran in
the sealed paren); it takes the dynamic-lead decline, and the callback's
other strategies answer.

**Pins.** lang `TestNUR270LandedLeadIsNoApplyEventLead`.

## NUR269 closed — the dynamic apply reads the anonymous park (2026-09-26)

**The divergence.** A map-each lambda's `kv.v` over a `([] => [5])`
member is the lambda interpreted (the ANONYMOUS-0-ARG PARK) and was 5
compiled: the whole-frame replay's lone token went to `dynApplyEnter`,
which entered the lambda's stamped unit over the empty window. The landing
reads the park; the apply entries did not.

**The fix.** `replayLeadParks` over the replay's lead, ahead of the Apply
kernel — not inside `dynApplyEnter`, whose other callers include the shaped
method's NAME reads (`def r (mk)  r` fires: a word dispatch is no value
re-step; the first draft parked it and `TestBodyLocalWordReadParity` caught
it). It exposed a silent twin: `[kv.v apply]` compiled to the same code as
`[kv.v]` — apply's identity result carries the lead's id and the
registered-output arm elided it, dropping the Applied mark. An `apply`
over a lone gradual lead is now exempt from that arm
(`loneGradualApplyLead`), so it reaches the dynamic-lead decline the apply
block already had — routed through the existing site, since the
compile-failure site and disposition censuses count `MarkUncompilable`
lines and a second site read as new debt — and the callback's other
strategies answer 5.

**Pins.** lang `TestNUR269DynamicApplyParksAnonymousZeroArg`.

## NUR268 closed — a callback body's word read runs the source value (2026-09-26)

**The divergence.** A fn value or lambda handed to a higher-order word
compiles to a callback body unit (`tryRecordLambdaClosure`) whose bare read
of a gradual param pushed the slot: `def g fn [[f:Any] [Any] [f]]  each
g/v [([] => [1]) 7]` was `[fn f 7]` compiled for the interpreter's `[1 7]`;
the lambda spelling, `x typeof`, a gradual list param, `(f)` and the
map-iteration fold's accumulator likewise. Silent, pre-existing.

**The fix.** NUR279's slot list is computed for every closure unit, and a
fn-typed read no apply lowering credited joins the gradual reads (a
callback body's reads are not accounted). The closure push carries its
callback VALUE (`callbackSourceSpec` → `ClosureRetSpec.Source` →
`ClosurePayload.Source`), and `closureSourceStep` on the token seam hands
an invocation with a fn in a listed slot to the interpreter's run of that
value — stepped over the inputs on the TOKEN seam, `InvokeCallbackFn` on
the fn-VALUE seam — with the closure's runtime captures bound. Data runs
the unit.

**Pins.** lang `TestNUR268CallbackParamReadIsTheWord`; compiler
`TestFnReadRefused`, `TestCallbackSourceSpec`.

## NUR279 closed — a stored fn's word read declines its unit or its call (2026-09-26)

**The divergence.** A fn value applied through a container member runs its
STORED unit (`compileStoredFnUnit`, `storedfn$body`), compiled once under
the declared param types. A bare read of a frame binding that holds a fn
is the interpreter's word dispatch (NUR123), and the stored unit had none
of the routes a named call's unit has for it — no per-call re-analysis
under the argument's runtime type (that is what compiles `g ([] => [42])`
to a frame replay), no seated body tokens for a deopt, no residual replay
(`noteClosureBodyReplay` takes member reads only). So `def g fn [[f:Any]
[Any] [f]] def m {g: g/v} m.g ([] => [42])` was 42 interpreted and `fn f`
compiled; a `f:Function` param the same; `[x f]` a count error for 6;
`[[f]]` `[[fn f]]` for `[[42]]`. Silent, pre-existing.

**The fix.** Two halves. The unit DECLINES where no argument makes the read
data (`storedUnitFnRead`): a fn-typed read no apply lowering credited, a
binding read both bare and `/v`, a gradual CAPTURE read in the residual. A
gradual PARAM the body reads is data for every argument but a fn, and
declining it would decline every `m k get` over an `Any` param (measured:
the lens and handler suites) — declining its residual read sent nine
corpus rows' data calls (`m.p 5` over `[x:Any] [x]`, fn-value.tsv L122–
L135, L229/L230, module-parselang.tsv L119) to the interpreter, the
engine-entry census 183 -> 197 — so the unit lists the slot instead
(`CompiledFn.FnReadParams`) and every seam that runs a stored unit —
`dynApplyEnter`, `dynApplyForeign`, `invokeFnValue`, `invokeCompiled`
(`CompiledFnRef.RefusesArgs`) — refuses an argument list with a fn there,
so that call takes the interpreter's dispatch. Data through the same member
runs the unit (`m.g 5`), and the per-call routes — a module member, a
def-bound value — keep their compiled frame replay.

**Pins.** lang `TestNUR279StoredFnParamReadIsTheWord`.

## NUR280 closed — a member reference is its word twin (2026-09-26)

**The divergence.** `/v` yields the binding's value whoever reads it, but a
MEMBER read's value was quoted where its word twin's is not:
execFnDefLiteral's peek consumed the group's marker and set `Quoted`, and
the quote rode into a paren's survivor, a callback slot and a branch
result. `each (m.f/v) [1 2 3]` stepped the value per element as data
interpreted (`[fn fn fn]`) and applied it compiled; `(m.f/v 5)` was `fn 5`
for the word twin's 6; `def g (m.f/v) end g 4` was 5 interpreted and `fn 4`
compiled; `[1 2 3] each [m.f/v]` applied the member compiled. Silent both
ways, pre-existing.

**The fix.** The peek DELIVERS: marker consumed, pointer past the value,
no quote — position keeps it inert, exactly as it keeps `inc/v` — and under
the check pass a value-read note (`NoteValRead`), so NUR186's frame-tail
decline and the code-body replay read the delivery as inert. Three
check-side stand-ins stopped leaking their quote: the forward scan tags the
value it quoted for its marker (`ReachGroup`) and the arrival delivers any
such value unquoted, a carrier included (the flex twin of `if true m.h/v
[2]` was data compiled — a NUR078 regression, loud before it); a bare read
of a binding that may hold a fn un-quotes the stand-in (the run routes a
bound fn through Lookup whatever its quote); the NUR277 marker drop notes
the carrier's delivery beside its quote, so `noteClosureBodyReplay` stands
aside for it (`[1 2 3] each [m.f/v]` declines loudly now — "result above a
literal" — where the word twin compiles).

**Found alongside, fixed with it.** A DYNAMIC member at a branch arm —
`if true m.h [2]` over a flex, bare spelling included — is re-stepped by the
interpreter after `if` returns (a 0-arg member fires: 1), and compiled to
the member itself. `RecordBranch` marks a dynamic arm `MayBeFn`
(`branchArmMayBeFn`), and `lowerComputedBranch` — the eager-arm merge the
check pass takes for a computed arm — emits the guarded landing the general
merge already emitted (NUR159). The measured matrix: ten shapes × word /
map / flex / module twins agree on both lanes, compiled; the loud declines
left are the word twin's own (`inc/v 5`), the member forms the corpus
already declined (`5 m.f/v`, `def ret fn [[][Function][m.f/v]]` over a
container), and the body shape above.

**Pins.** lang `TestNUR280MemberRefIsItsWordTwin`,
`TestNUR280DynamicBranchArmLands`; core `TestFnValueDispatchModLeavesInert`
(the delivery is unquoted).

## The merge of main's #508, NUR206 closed by it (2026-09-26)

Main's #508 (the apply chain for callbacks.tsv L125, the withdrawn hosted
`for` body, core's own coverage back at 100%, NUR206 recorded) merged
into the reverse-order NUR run. Four conflicts, all bookkeeping: the
register's rows (main's NUR206 row slots in beside the run's FIXED NUR205),
a comment in `bindDynScopeMode` (both sides folded the same unused
wrapper), the lang compile-defect ledger and the corpus ledger's
code-bodies row, whose two sides compose (main's row 141 and the run's row
142 both decline; callbacks.tsv leaves the ledger with main's L125). The
lang ledger composes to 309 / 40 from 54200b3's 299 / 40 — main's +5 / +1
from the merge base (286 / 45) and the run's +5 / −1 — and no program
changed verdict in the merge: the merged ledger differs from the run's by
exactly main's five `for: body not captured` witnesses and its one
apply-chain bail.

**NUR206 closed by the merge.** Main recorded it as the interpreter's loop
index surviving an error the enclosing `do` catches (`def i 99 end do [for
3 [raise oops 'x']] error [drop] end i` was 0 interpreted, 99 compiled). The
run had already fixed that defect as NUR208 (NUR251 since the merge of
main's #510) — the fault return unwinds the
live loops — so on the merged tree all three witnesses are 99 on both
lanes. Main's pin asserted the divergence "so the close is noticed"; it is
flipped to `TestLoopIndexUnwoundByCaughtError`, with an undefined-name
negative.

## NUR078 closed — a bare fn name calls at every slot (2026-09-26)

**The divergence.** ADR-011's clause-2 amendment (2026-08-17, re-affirmed
2026-08-26) struck the exception "a bare fn name before a `Function`-typed
slot resolves as a reference"; the engine still implemented it in four
sites (FN-VALUE-OPEN-WORK.0 §3.2): stepWord's TFunction intercept (A),
`hasPendingForwardExpectingFunction` (B), `sigWantsFunctionAt` (C) and the
ReachGroup arrival's `ConformsTo(TFunction)` exemption (D). `h zero`
answered `h zero/v`'s 42 while the same name before an `Any` slot was a
call/barrier error — the slot type, not the modifier, decided.

**The fix.** A–D are deleted together. The plan's word arm
(`CollectCandidateScan`) claims a fn binding BY VALUE only for a `/v` word;
the check pass's stand-ins count as fn bindings too — a fn-typed carrier (a
factory's result, a `g:Function` param) and a dynamic binding at a
Function-typed slot — or the compiled lane collected by value what the
interpreter calls (`def f (mk 10) end each f [1 2 3]` answered `[[11 12
13]]` compiled for the interpreter's `signature_error`; it declines now).
With C and D gone the NUR038 call-head question has one answer: a
reach-read fn that WOULD CLAIM the next token is a call head, a claim-less
one an operand, at any slot (`ReachCallHeadBarrierOn` lost its viable-sig
arguments). The `/v` spelling had to reach every place the bare one did:
the forward scan's pre-evaluation consumes a reach's `/v` marker (it was
counted as an argument — `mini M.dbl/v 'ab'` took the marker for mini's
String); `CollectArrival` delivers a marker-quoted reach value UNQUOTED and
untagged, as `stepWordVal` delivers `inc/v` (quoted, the interpreter's token
seam stepped it as data while the compiled lane applied it — `[1 2 3] each
M.inc/v`, `if true m.f/v [2]`); `placeModifierOperand` wraps a reach after
`/u /s /f /N` sugar in a paren so the modifier takes the value (`m.a/u`);
`appliedTransducer` un-quotes mini / emit / parse transducers; and
`landingNextForWord` reads a `/v` word after a landed value as a collected
value (`m.g z/v` raised a false `uncalled_function` compiled; 7 on both
lanes now). The VM landing's Function-typed claim arm (`vm:landing-claim`)
is unreachable and gone, which dissolves NUR190's Function-typed half:
`m.g z` is the named no-match on both lanes. `unused_def` needed no move —
the `/v` read's ResolveRef records the use. Migrated: `path-modifier.tsv:67`
(now `ERROR:cannot call `wa``), the sweep's four module-export seeds
(`filter` / `force-arity` / `forward-args` / `usurp` × module-export — the
sweep holds at 234 call-form declines, 0 invalid seeds), and every spec and
unit row that passed a callback bare. The arity gate's engine.go pin drops
29 -> 27 with B and C.

**Found on the way, not fixed here.** (1) A `/v`-quoted member read inside
a PAREN keeps its quote to the consumer: `each (m.f/v) [1 2 3]` and `fold
(m.f/v) …` step it as data interpreted and apply it compiled, `def g
(m.f/v) end g 4` is 5 interpreted and `[fn 4]` compiled — pre-existing
(measured on the committed head), NUR280. (2) A member-read fn applied
over a fn argument whose body reads a GRADUAL param bare: `def g fn [[f:Any]
[Any] [f]] def m {g: g/v} m.g ([] => [42])` is 42 interpreted and `fn f`
compiled — pre-existing, NUR279.

**Pins.** lang `TestNUR078BareFnNameCalls` (the `/v` spellings at every
slot, both lanes; the bare spellings raise, and a carrier-bound bare name
declines or raises, never answers), `TestFunctionSlotArgIsNotUnused`
(re-homed), `TestNamedFnCandidatesOpenShapes` (`m.g z` / `m.g z/v`),
`TestWordReadDispatchParity` / `…FailsToCompile`; eng
`TestReStepLandingWalk`.

## NUR079 closed — one policy for a program and its modules (2026-09-26)

**The divergence.** Half (i) (2026-08-18) put the importer's policy on a
module body's registry. Half (ii) was open: a FILE module import applied
none of the checks a native import applies, `boru check` took no
permission flags, and the run's pre-flight executed module bodies before
the run's policy was even resolved.

**The fix.** `checkFileModuleImport` in `loadFileModule` — `modules.import`
with `{module: <ref>, kind: "file"}` and the ref's subscope
`install:false`, keyed like the NUR045 per-export gates. `modules.Resolve`
now passes `kind: "native"`: policy where-predicates pass VACUOUSLY on an
absent arg, and the first cut's `kind: ["file"]` admission in the built-in
profiles let `boru:net` through `sandbox` until the native check carried
its kind. Profiles: `sandbox` (so `read-only`, `client`), `compute`, `gen`
admit `kind: file` — the body runs under the importer's profile. Refusals
are coded through `PolicyRefusal` on both paths (a coded native refusal is
returned unwrapped by `resolveNativeMod`). CLI: `boru check` registers
`permsflags` (Opts.Policy, the `--emit` path too), `run`/`build` resolve the
policy before the pre-flight and call `PreflightPolicyAt`, `describe` builds
its registry with `permsflags.EnvPolicy()`, the language server's check and
completion instance likewise (an unresolvable policy is the init
diagnostic). The check pass mirrors a refused TOP-LEVEL import
(`mirrorImportRefusal`): `RecordTrapErr` at the `import` word
(`CheckState.CurWordPos`) plus a RuntimeMirror diagnostic; the two codes got
error severity in the table. Found on the way, not fixed here: code after
ANY top-level trap that names an undefined word declines to compile
(`raise "x" foo`), so a refused import whose names are then used declines
rather than trapping — loud, and the pre-flight reports the refusal first.

**Pins.** lang `TestNUR079FileModuleImportPolicy`,
`TestNUR079CheckReportsRefusedImport`; cmd `TestCheckHonoursPermissionFlags`,
`TestPreflightRunsUnderPolicy`, `TestDescribeHonoursEnvironmentPolicy`,
`TestComputeDiagnosticsEnvPolicyFailure`.

## NUR089 closed — a lambda param binds as the run binds it (2026-09-26)

**The divergence.** The same curried combinator, passed the same two
functions, checked clean with `/v` references and drew `no_signature:
cannot call g … got (Integer)` with inline `=>` lambdas; both ran to 14.

**The fix.** The record's "misbinding" was not one: `g` held the second
lambda. `RunFnBodyOnce` bound params and captures with a raw `Defs.Push`,
and the run binds them through `core.InstallFrameBinding`, which compiles a
fn value's authored signatures into dispatch-ready ones (`installFnDef`). A
named fn's were compiled at its `def`; an inline lambda's authored
signature — afn leaves it authored, because a VALUE dispatches from that
form — carries no argument types, so the body's `g x` matched nothing,
whatever `x` held. `bindFrameValue` binds a concrete fn value the run's
way; other values keep the plain push. The construction-time body check
installFnDef triggers is memoised per body, so a re-bound lambda is not
re-analysed.

**Pins.** lang `TestInlineLambdaChecksLikeReference` (the minimal pair and
the S pair, both spellings, and the String negative); check
`TestRunFnBodyOnceCallsFnValueParam` / `…Capture`.

## NUR278 closed — the quoted lead is data on every arm (2026-09-25)

**The divergence** (recorded and closed the same day, found closing
NUR096). `c.op/v 5` over a class member typed by a fn shape answered `6`
compiled for the interpreter's `fn (Integer) 5`; the window spellings `3
c.op/v 2` and `3 4 c.op/v` applied too, and so did NUR277's map member in
those two spellings.

**The fix.** NUR277 quoted the value a standalone `/v` follows and taught
the DYNAMIC lead arm to honour the quote. A class member read is a
fn-shape-typed carrier, not dynamic, so it took the Function-carrier lead
arm, which now skips a quoted lead as well; and `fnLikeResidual` — the one
classifier the trailing apply and both verbatim window islands share —
answers false for a quoted value. Class spellings compile and agree; the
map window twins decline at the existing "call result above a literal"
limit (as `5 m.f/v` does) and answer by fallback.

**Pins.** lang `TestClassMemberValueMarkerIsData` (nine agreeing shapes,
the unmarked reads among them; the two map window twins through
`requireEngineParity`).

## NUR096 closed — the check applies a fn-shape member (2026-09-25)

**The divergence.** A class field typed by a fn SHAPE holds a function at
run time, and both lanes re-step it over its argument window — `c.op 10` is
`[10 10]` for `def T fnsig [[Integer] [Integer Integer]]`. The plain check
held the member carrier and its argument, `[T Integer]`: invisible for a
one-result shape (the soundness comparison is top-aligned) and a type
violation for a two-result one, which is why the multi-return rows could not
live in `class.tsv`.

**The fix.** `tryFnShapeTypedWindow` (check/go/method_shape.go), first in
the plain-check fn-value models: a non-dynamic carrier typed by a fn shape
with one signature of plain parameters, followed by that many
evaluation-fixed tokens that fit, is replaced by one carrier per declared
return (a dynamic one for an Any return). The claim is the named shape's
own content, or — for an anonymous shape, whose carrier is typed by the
bare FunctionSignature node — the signature `getObjectReturns` notes by the
carrier's id at the read (`FnShape` grew `Returns` / `ReturnsKnown`, so an
empty list can mean "returns nothing"). The compile pass is untouched: its
member models already lowered the apply. Stands aside for a paren that
places the fn, `/v`, an unfit or missing argument, arity 0, and several
signatures or an optional / patterned / quoted parameter.

**Found on the way.** `c.op/v 5` over the same class member answers
`fn (Integer) 5` interpreted and `6` compiled — the class-instance twin of
NUR277's map fix, pre-existing (measured with this change stashed).
Recorded as NUR278 and taken next.

**Pins.** lang `TestPlainCheckModelsFnShapeMemberApply` (checked stack and
both lanes, with the paren and unfit-argument negatives); check
`TestFnShapeTypedWindowNamedShape`, `…NotedClaim`, `…Declines`,
`TestFnShapeOfSpecDeclines`; `class.tsv`'s two multi-return rows.

## NUR099 closed — a capitalised fn body is refused (2026-09-25)

**The divergence.** One spelling carried two jobs: a fn body meant a
callable under a lower-case name and a membership test under a capitalised
one, routed by counting its parameters (`isPredicateFnValue`, ADR-016's
arity-keyed exception). So `def K fn [[a:Any b:Any][Any][a]] end K 1 2`
bound a type nothing could inhabit and exited 0 with `K 1 2` on the stack,
and `def I x:Integer => [add 1 x] end I 5` placed a type node for a call.
`fnpred` (2026-08-25) gave predicates their own door; the arity route stayed
until the corpus had moved.

**The fix.** `InstallTypeBody` refuses a capitalised name over an
UNDECLARED fn body — a fn definition or a compiled closure under Function
(`isFnBodiedValue`) — with `def_error`, naming both ways out (`def K fnpred
…`, or a lower-case name). A type literal that merely sits under Function (a
module's exported member type, `MiniLang.Re`) keeps binding as an alias.
`isPredicateFnValue` is deleted, `PredicateInputType` answers only for a
declared predicate, and unify's predicate-reference arms key on
`IsDeclaredPredicateFn`. `def` installs during the check pass, so both
lanes refuse at the declaration. The corpus moved to `fnpred`: the spec
rows (record, edge-types-2, compare, refine-flex, fnpred), behave.tsv, the
Go test sites, core's fixtures (`MarkPredicateFn`), and the design examples'
and editor samples' `def New fn …` constructors (now `def new fn …`,
exported under the same `New` key). `stranded_type_call` stays for the one
fn-bodied type node left, a declared predicate written as a call; its note,
REFERENCE.md, CLI.md and OPEN-WORDS say so.

**Found on the way — a rollback leak.** The lang ledger's one new decline
(`code-body word if` in `def til fn [… for (xs size) [def idx i if … [break]
[] def seen (seen add 1)] …]`) was not NUR099's: NUR214's fresh carry
shifted the loop rounds' event order, and `EmitState.Rollback` reissued the
discarded round's seqs without dropping the notes keyed by them. The routed
`add`'s GENERIC flag landed on the next round's `if` branch event, so the
double-record guard stopped eliding the `if`. Rollback now drops every
seq-keyed note past the checkpoint (`eventInfo`, the re-step and landing
notes, `argsProjSeq`, beside the placeholder marks it already dropped) and
`constKeep` past the const pool's cut.

**Pins.** core `TestNUR099CapitalisedFnBodyRefused`,
`TestNUR099IsFnBodiedValueArms`; lang `TestStrandedTypeCallRefusedAtDeclaration`,
`TestLoopRoundRollbackDropsEventNotes` (fails without the eventInfo drop);
`lang/spec/fnpred.tsv` §5 (the legacy spelling and the combinator refused,
the lower-case combinator callable) and `edge-types-2.tsv` (the lambda
spelling refused).

## NUR112 closed — the plain check's stored member (2026-09-25)

**The gap** (narrowed earlier the same day to a checker-model item). A fn
value stored in a map and read as a member dispatches over what follows on
both lanes (`def m {a:size/v}  m.a [1 2 3]` is 3), but the PLAIN check
read the member as a dynamic Any — the compile pass's model, whose arrival
the shaped method model claims — and so claimed the member and its
argument as the residual, `[dynamic(Any) List]` where one Integer runs.

**The fix.** `getNodeReturns` reads a stored FnDefInfo member as the value
itself when the recorder is not armed, so the plain pass re-steps and
dispatches it as the run does; the compile pass is untouched (the armed
recorder keeps the dynamic carrier and `NoteMethodShape`). Measured over
the corpus: the accuracy ratchet, the type-soundness gate and the
diagnostic-surface parity unchanged.

**Pins.** lang `TestPlainCheckAppliesStoredFnMember` (four programs: the
checked residual, parity, the interpreter's value).

## NUR215 closed — the conditionally bound name's miss (2026-09-25)

**The divergence.** A name bound only on some paths — an arm's def with no
pre binding (NUR110's branch-carried cell), a fresh def in a loop that may
run zero times (NUR214's) — is read by its own unit through the
bound-checked cell, but a fn reading it DYNAMICALLY reaches the VM's
`OpLookupDynScope`, whose miss bailed (`internal_error`: dynamic-scope
read miss) where the interpreter raises `undefined_word`: `def c (1 gt 2)
end if c [def k 5] [] def g fn [[][Any][k]] end g`, and the loop twin.

**The fix.** The Program carries the cells' names (`CondBoundNames`,
filled by `noteCondBound` from the branch's unbound arm and from
`NoteLoopFresh`), and a dynamic-scope miss on one is the interpreter's
`undefined_word`, raised at the read — the rule `SpecUndefNames` and
`LiveReadNames` already follow. A path that ran the binding installs the
name (the arm's registry twin, the loop body's carried dyn-scope bind), so
the read finds it.

**Pins.** lang `TestCondBoundDynamicReadRaises` (both shapes both ways).

## NUR214 closed — the loop's fresh cell (2026-09-25)

**The divergence** (found closing NUR205). A FRESH def inside a loop that
may run zero times bound anyway on the compiled lane: `def n (0 add 0) end
for n [def x 5] x` answered `5` (the read const-folded the body's value)
and `for n [def x (n add 5)] x` an empty stack (the body's promoted slot,
never stored) where the interpreter raises `undefined_word`; `while` and a
fn body the same. NUR110's loop twin — NUR110 measured only a static `for
0`, which the pass prunes.

**The fix**, on the loop-carried mechanism NUR110's branch fix came from.
The loop join (`AnalyseLoopBody`) gives a fresh name bound in a loop that
is not proven to run a post-loop binding of its own, a carrier
(`JoinCarriers(v, v)`, `loopFreshCarriable` screening out types, fn values
and modules), and `NoteLoopFresh` seats it in the unit's cell for the
name with NO init: from the second analysis round the body's def stores
into the cell and installs the name per iteration (the carried def's
dyn-scope bind), and the joined carrier's reads load the cell
bound-checked. The carrier's twin replays nothing ahead of the loop. NUR204's
index guard (`boundLocals[pre] == name`) read a fresh cell as a loop index
and refused a second loop's re-carry (`for n [def x i] for 2 [def x (x add
10)] x` declined); `loopFresh` tells them apart.

**Pins.** lang `TestLoopFreshDefZeroTrips` (thirteen shapes: zero trips
raise on both lanes; a loop that ran reads its last binding; nested,
container, fn-body and while forms; the re-carry). Found beside it and
closed with it: NUR215.

## NUR205 closed — the module replay's own bind (2026-09-25)

**The divergence** (main's record, found by the sweep's callback cells).
An `import` is a compile-time word: the check pass runs it and the
compiled program replays its one bind — for an import inside a loop body
or an `if` arm, the JOIN's twin, placed at the construct's position. Where
that replay is not the interpreter's bind the answer was silently wrong:
`for 2 [import module [def acc (flex []) export "M" {acc: acc}] end M.acc
push 1 end size M.acc]` was `[[1 1] 1 [1 1] 2]` compiled for the
interpreter's `[[1] 1 [1] 1]` (the interpreter re-imports per iteration,
so the inline module's state is fresh each time), and — measured closing
it — a loop that may run zero times (`def n (0 add 0) end for n [import …]
M.a`, `while [false] [import …] M.a`) and an arm that may not run (`if c
[import "boru:math-util"] [] MathUtil.$name`) answered a value where the
interpreter raises `undefined_word`.

**The fix.** The one twin the replay cannot stand for is noted with the
recorder SUSPENDED, so it keeps no placement and the program declines at
the twin regime's full-placement gate — the existing site, so the
compile-failure site census does not grow. The loop (check/go
`AnalyseLoopBody`, `loopModuleUnplaceable`) withholds a module bind when
the loop is not proven to run, or when the body's import mints a NEW
instance per run: the last two analysis rounds' namespaces share no export
map (an inline `import module […]`), where a `boru:` module the loader
caches answers the same map. The `if` (basic `installArmJoins`, both
forms) withholds it on an arm that may not run — either arm of an
undecided condition, the arm a concrete Boolean does not take.

**Edges kept.** A cached `boru:` import in a loop that runs (`for 2
[import "boru:math-util" end MathUtil.$name]`), and the arm a decided
condition takes (`if true [import …]`, the vary if-then transform), keep
their one replay and agree.

**Found beside it, recorded as NUR214:** the same zero-trip loop binds a
FRESH value def anyway — `def n (0 add 0) end for n [def x 5] x` answers
`5` compiled for the interpreter's `undefined_word` — NUR110's loop twin.

**Pins.** lang `TestModuleBindInLoopOrArmDeclines` (six declines, the
interpreter's answers) and `TestModuleBindReplayStandsWhereItIsTheBind`
(three parity rows); the sweep's two NUR205 pins retired (the for-body
variants decline now).

## The merge with main's #507 — the colliding register numbers and the gate's fallout (2026-09-25)

**The merge.** Ten conflicts, every one resolved by keeping both sides:
both new opcodes (OpBindFnType, OpBindDynScopePeek); the static-index fold
keeps NUR124's fn-element guard and then main's args-projection retraction;
the emit state keeps NUR203's dynamic keep-defs leak beside main's
run-time bind latch; the keyword form inherits the base's CompileDiverges
and takes main's S2a quote declaration (a bit set); the frame's error path
(NUR201's fault-return unwind) supersedes `unwindFrameTailOnError`, whose
teardown already calls main's `UninstallFrameBinding`. **The register:**
main's new NUR205 keeps its number; this run's NUR205, NUR206 and NUR207
(all closed) are NUR211, NUR212 and NUR277 everywhere they are cited (NUR261,
NUR262 and NUR277 since the merge of main's #512, where main's NUR211 and
NUR212 kept their numbers).

**The fallout, fixed.** `TestCheckProp_ShrinksFailingInput` shrank to 40
for 10: NUR077's recorder fix records the generator's bound faithfully
(`r.int 0 1000` was recorded without its 1000), so the program-level
reducer now improves the form — but only narrows the generator — and its
answer stood where the value-level reducer used to run; the value-level
reducer now finishes a program-level shrink and wins when it costs less.
The arity gate: the predicate memo key and the pure-body screen read no
arity (the key walks the signatures; the screen reads RunPredicate's one
guaranteed parameter), and StackForm's replay arities are pinned as the
argument rule they are. The sentinel gate: NUR128's foreign-registry test
reads `Registry.SameHome`. `late_binding` (NUR097) is registered as a
kernel code. `fn`'s describe data and the fn-model golden carry NUR091's
0-argument signature (its def-form mirrors included).

**Measured and re-pinned, every move named.** The lang ledger 299 / 40
(the two sides compose from the merge base's 291 / 44; NUR205's six
declines). The langspec: engine entries 165 -> 183 and interp-entry
census rows 24 -> 36 (thirteen rows the run ADDED enter on known seams —
callbacks, fn-value and user-types rows pinned for NUR166, NUR261,
NUR168, NUR158 and NUR167 — and code-bodies L77 leaves; measured with
BORU_LOG_CENSUS_ROWS=1 against 00ec530); type-soundness 4 -> 7 (three
rows the run added: NUR201's two `do [g]` pins and the parked module
closure); code-bodies.tsv 1 -> 2 (L142 declines soundly, NUR154); bail
rows 51 -> 46 (the fnpred rows and record.tsv L178 are static check
errors since NUR141; runtime_defers.tsv's fnpred line deleted); the sweep
13 -> 15 failures and 245 -> 234 call-form failures (thirty-four
graduations, eighteen NUR205 declines, NUR162's two, and three sound
declines owed a look — `def` × container under fn-/lambda-body, `afn` ×
factory under module-body); SWEEP_STATUS.md refreshed. NUR109's decline
learnt the call's own arm (`slotStoredInScope`): a def in the same arm
dominates the dispatch, so the vary if-then / if-else wraps of
module-parse.tsv L41 compile again.

## NUR134 closed — the unit trap and the caught Error (2026-09-25)

The module export's no-match inside a `do` body was reported by a FOURTH
dispatch: the body's residual carried the failed call's wreckage out of
the caught bracket as the do's results and the enclosing tape re-stepped it
at the uncaught top level. `DoListReturnsFn` now watches its body
(`CheckState.RaiseWatches`): a definite no-match or a `raise` at the body's
own level marks the watch, the do's model is one Error carrier, and the defs
the body made after the raise are rolled back. The compiled do-body unit
raises in place: `RecordUnitTrapErr` records a unit-scoped trap at the
unit's root frame (first wins), `MarkUncompilable` ignores the dead tail
while a trapped unit is open, finish drops the tail (its twins join
`supersededTwins`), and `fragDiverges` / `eventDivergesDeep` treat a trap
as the fragment's end (no RET). Found and fixed with it, pre-existing and
silent: `def x 0 do [raise bad_input "boom" def x 1] error [dot code] x`
compiled to 1 for the interpreter's 0 (the keep-defs model leaked the def
the raise skipped). Pin: `TestModuleNoMatchInDoBodyIsCaught`.

## NUR101 closed — the paren re-step pair, already landed (2026-09-25)

A register-only close: the 2026-08-27 verdict (compiler-side, the
`ParenPlacedFnIDs` / `ParenReSteppedFnIDs` pair read at the collapse)
landed that day and the four shapes it left refusing graduated with the
curried chain on 2026-09-22 (`[((mk 1) 2)]` is `[[3]]` on both lanes).
Measured today: `(mk 1) 2` places, `((mk 1) 2)` re-steps, `def h (mk 1)
end  h 2` is 3, the standing measurement passes. The record stayed
Pending only for want of an update.

## NUR102 closed — one predicate run per dispatch (2026-09-25)

An effectful `fnpred` body ran four times interpreted (the pending word's
four planning phases — collection, candidate scan, arrival, final match —
each asking the predicate type) and twice compiled (the poly re-match and
the entry guard behind it). `RunPredicate` memoises its run-time verdict
per (predicate, candidate) until an effect may have moved the basis
(`ClearPredMemo` at a dispatch commit and a statement end; an ID-less
interpreter predicate keys on its boru body), and the VM skips
`checkParamContract` behind a runtime poly re-match. Found with it and
fixed: a CARRIER candidate against a predicate-typed arm was rejected by
the lattice walk at analysis, so `we (f 2)` committed the Integer arm and
answered int-arm for the interpreter's even-arm — the predicate unifier
admits a check-mode carrier of its input type, the arm stays reachable,
the call goes poly. Also closed today: NUR104 (Allowed — the install-time
resolution of inline record fields is the uniform rule; four spellings
pinned on both lanes) and NUR103 resolved by the diagnostic-surface gate
(the mini-redis instance survives only in the full module context; the
`undefined_word` class is ledgered, `fn_body_error` graduated). Pins:
`TestPredicateRunsOncePerDispatch`, `TestRecordTypeSpellingsAgree`.

## NUR109 closed — the parser name bound on a branch (2026-09-25)

Re-derived with NUR110 closed: the interpreter's `parse_unknown_lang` is
the consistent answer for a parser name bound only in a branch that did
not run. The compiled lane still raised `parse_error` because the QUOTED
atom resolved through the registry to the arm's own `Parse.parser` event
(a fn-valued arm def is not carried by the branch join — `carryBranchJoin`
leaves fn values alone — so no bound-check guarded it) and the lowering
pushed the promoted slot verbatim: the zero slot. No op re-resolves a name
at run time, so `lowerCall` DECLINES a `parselang-fn-dispatch` whose
parser operand is a slot a branch arm's promoted event fills (the
lowerer's `rootSeqs`) or a bound-checked carried slot (`boundSlots`); the
interpreter answers on both twins. Pin: `TestParseFnDispatchMissParity`.
Also narrowed today: NUR112 (the extension is irrelevant; the member
read's widening is the designed model at `getNodeReturns`), left pending
on Stage 8 with its retirement condition.

## NUR077 closed — the Apply op (2026-09-25)

The StackForm's dedicated `Apply{Arity}` op lands (`lang/go/stackform`):
recorded at `execFnDefSig`'s nameless `OnCall("")`, replayed as the
window's rotation (`swap` / `rot`) plus a statement end that resolves the
re-stepped value from the stack — the interpreter's own binding rule. Two
recorder defects under it: `recordDispatch` credited a dispatching
Function result a skip it never spends (the credit ate the application's
first argument — `(m.f 7 2)` recorded `7 apply/2`), now it counts only
results that push; and a replayed fn result dispatched at the pointer
before the rotation, so every `Call` flattens with a statement end behind
it (parks a Function result, inert otherwise). Hole 2 — the `apply` word's
double recording — declines (`Call.ReStep`, `ErrApplyReStep`): replayed as
written it ran the fn twice (7 for 6). Pins: the three witnesses and a
two-argument member replay; the three double-recorded shapes decline; the
literal-accounting pin holds.

## NUR097 closed — the late-binding hint (2026-09-25)

The Allowed verdict's mitigation exists: `late_binding` (info). The check
pass records each named fn's body reads (`CheckState.FnReads`, from
`recordUse` under the fn-name stack) and every root def site
(`RootDefSites`, the def-NAME token InstallAndRecordDef stages — a fn
value carries no position), and `EmitLateBindingHints` reports, with the
unused-def pass, a name bound by a root def BEFORE the fn and re-bound by
one AFTER it. A name first defined after the fn is a forward reference and
hints nothing — the first cut hinted those, and the diagnostic-surface
sweep caught them as compile-only (the compile surface re-analyses the body
at the call, where the late name resolves). `CheckState.Clone` deep-copies
both maps (the lifecycle pin).

## NUR123 closed — the guarded gradual read (2026-09-25)

The last open shape, a gradual captured read with a value pending beneath
it (`5 j typeof`), kept its slot push silently where the point planner
declined (`[5 Function]` for `[5 Integer]`, masked by the frame's count).
A point no island can serve — a deferred start, or an environment the unit
cannot bind — demotes to a GUARD (`deoptPoint.bail`, `bailPoint`,
`DeoptSpec.Bail`, seated without a deopt environment): `OpDeoptIfFn` tests
the value where the statement begins and raises a designed defer when it
holds an appliable fn, passing the slot push through otherwise. Loud in the
bail ledger where it was silent. Pin: `TestGradualReadWithPendingValueGuards`.

## NUR124 closed — the value-delivered window parks (2026-09-25)

The fifth witness (`(g/v 5)` over a String-only g): the interpreter leaves a
`/v`-delivered fn the window does not fit as DATA beside its window, the VM
raised `cannot call `g`` through the nameless builder. `callDynTrailTop`
parks a value-delivered head (the recorder seats no name for it) that
matches nothing, in WRITTEN order (`DynApplyHead.Leading` /
`WrittenFirst`; a nameless head is seated for the flag), so both lanes
raise the frame's count error over `[fn g(String) 5]`; a matching window
still applies. Every other witness of the record agrees on both lanes or
declines loudly (measured). Pin: `TestDynApplyHeadNameNamelessArm`, a
parity pin now.

## NUR128 closed — the export-time analysis (2026-09-25)

`export` queues each exported fn value for the end-of-pass body check on
the importing pass (`NoteFnBodyPendingIn`), and the drain shares that
pass's check state into the module registry for the analysis
(`CheckBraid.ShareCheckStateFrom`), so a module fn gets the
declaration-shaped run a top-level fn gets at construction, in the
registry it was written in: its dead branch warns whether or not anyone
calls it (`TestModuleExportedFnBodyAnalysedAtExport`). The run keeps
only its structural findings (`unreachable_branch` — a real app's module
fns came back with false `no_signature` / `undefined_word` otherwise) and
bypasses the pass's call-shape summaries (`CheckState.ForceFnReanalysis`).
NUR128 is FIXED.

## NUR129 closed — the reach survivor's iteration (2026-09-25)

The dynamic loop residual that turns out callable is a reach group's
survivor — a member fn read the pass cannot type; the collapse records it
(`ReachSurvivorFnIDs`, NUR260's mark widened to dynamic survivors) and
`RecordLoop` declines a body that leaves one beside the named-fn hazard
(`loopHazards`), so `for 2 [m.f]` over `{f: g/v}` answers the
interpreter's `uncalled_function` by fallback where it compiled `[fn g fn
g]`; typed member reads and paren residuals keep their compiles
(`TestLoopReachSurvivorDeclinesSoundly`). NUR129 is FIXED.

## NUR141 closed — the predicate runs for real (2026-09-25)

`RunPredicate` runs an effect-free predicate over a CONCRETE candidate for
real under analysis, the const fold's own discipline (mode suspended, the
def table restored, an erroring run admits as before), so the check pass's
plan refuses `f 5` over `n:Even` exactly as the runtime does; a carrier
stays admitted. The region oracle's `over-claimed` entry is retired
(`TestPredicateAdmissionAgreesWithTheRuntime`). NUR141 is FIXED.

## NUR171 closed — the lens's own token (2026-09-25)

`lowerReach` anchors a segment's `dot` at the segment's key token when the
receiver carries no position — the lens unit's synthesized receiver — so
the compiled no-match raise inside `5 $.name apply` is positioned (`1:5`,
the key; the interpreter's `1:1` is the receiver) and
`knownPositionLoss["reach.tsv:L52"]` is retired
(`TestLensNoMatchKeepsAPositionCompiled`). NUR171 is FIXED.

## NUR203 closed — the dynamic body's leak (2026-09-25)

A keep-defs word over a DYNAMIC body (a List param, a def-bound quoted
list) inside a fn leaks the body's defs into the fn's frame, and the pass
cannot know which names: every name the unit value-defined before the
dispatch is taken as leaked (`noteDynKeepDefsLeak`, from
`tryRecordDynBody` — `keepLeakNames` and the new `dynLeakNames`, so a
later read seats live whatever its compiled home, and a carried slot is
refreshed from the registry). `def f fn [[b:List xs:List][Integer][def t
0 each b xs drop t]] end f (quote [def t (t add 1) t]) [1 2 3]` is 3 on
both lanes (`TestDynamicKeepDefsBodyLeaksToTheFn`). NUR203 is FIXED.

## NUR204 closed — the lexical index scope (2026-09-25)

A body def of a counted loop's OWN index rebinds the iteration's binding
and nothing else, on both lanes. Interpreter: `ForCont.IterDepth` records
the index level's depth at entry and `popIterLevels` pops the body's
levels with the iteration and the index level with the loop (done, break
and fault-unwind paths); the check pass's loop analysis pops its bind
names to their pre-push depths and neither joins nor carries them
(`isBindName`). Compiler: `lowerDynBind` stores the def into the loop's
index slot (`storeBindInto`, the twin marked written back; `loopCtx`
carries `iterSlot`) instead of declining, and `NoteLoopCarried` skips the
loop's own bind variable. `def i 0 end for 3 [def i 9] end i` is 0
everywhere, `for 3 [def i (i add 1) i]` leaves `1 2 3 0`
(`TestForIndexDefInBodyIsTheIterations`). NUR204 is FIXED.

## NUR260 closed — the reach group's survivor (2026-09-25)

A reach group never parks: its collapse re-steps the lone survivor over
the values beneath, a call result included. The check pass now records a
fn-typed CARRIER survivor at the collapse (`CheckState.ReachSurvivorFnIDs`,
beside the concrete named value's `ReachGroup` tag), and the residual
lowering's placed-call gate exempts it unless an enclosing user paren
placed the same value, so `5 M.ff` and `M.ff 5` over a module fn returning
`inc/v` are 6 on both lanes and `5 (M.ff)` stays parked
(`TestModuleFnNamedValueThroughReachApplies`). NUR260 is FIXED.

## NUR081 closed — one contract for the family (2026-09-25)

`Test.skip`'s property form applies `Test.check-prop`'s argument contract
before it parks the property: `runs` below 1 and `max-shrinks` below 0
raise `range_error` blamed on `Test.skip` (`requirePropCountFor`,
lang/go/modules/test.go), where the skip used to record `{ok:true
runs:0}` for counts its sibling refuses. Pinned by two `module-test.tsv`
rows. NUR081 is FIXED.

## NUR082 closed — one walk for the tree commands (2026-09-25)

`pathutil.WalkSources` is the one tree walk `fmt`, `check` and `test`
share — the `.boru/` skip and the filename suffix live in it — so `boru
test`'s discovery no longer descends into the package directory
(`TestDiscoverSkipsPackageDir`). NUR082 is FIXED.

## NUR083 closed — the script's own directory (2026-09-25)

`run` and `debug` anchor a script's relative imports at the script's own
directory, as `check` and `build` do: `lang.Options.BaseDir` (applied to
the registry by `lang.New`) is set from the script path in run.go and
debugcmd.go, and both pre-flights go through `PreflightColorAt` with the
same anchor; `-e` and the REPL keep the process cwd. `boru run sub/m.boru`
succeeds from the parent directory
(`TestRunAnchorsRelativeImportsAtTheScript`). NUR083 is FIXED.

## NUR084 closed — fmt answers -h (2026-09-25)

`boru fmt -h` (and `--help`, `help`) prints its usage to stdout and exits
0, as `check` does, instead of reading `-h` as a file to format
(`TestFmtHelpExitsZero`). NUR084 is FIXED.

## NUR088 closed — six spellings, one form (2026-09-25)

`elideFnTriple` reads a `fn` wrapper as `params… ret body` — the body is
the last list, the return the token before it (bracketed or bare), the
params everything else (one bracketed list or a bare run) — and
`elideBareTripleRet` handles the wrapperless spelling, so each of the five
non-target spellings of a single-param signature formats to `fn x:Integer
Integer [mul 2 x]`; the irreducible shapes pass through
(`TestFormatCollapsesEverySingleParamFnSpelling`). NUR088 is FIXED.

## NUR091 closed — the fn that took nothing (2026-09-25)

`fn`'s silent path was the synthesized 0-argument fallback: with the
triple's `(tnot List)` rejecting the input and the spec-list form finding
no list, `def f fn List Any [1]` constructed nothing and stranded its
operands, exit 0. `fn` carries an explicit 0-argument signature now, whose
handler raises `signature_error` naming the rule, so the declaration
fails at the declaration on both lanes whatever sits in its output slot
(two `fn-triple.tsv` rows); the signature is declared `CompileDiverges`
and `def`'s synthesized keyword forms inherit that bit (the declaration
census holds at 94). NUR091 is FIXED. Merged with main the same day: the
lang ledger 291 -> 304 compile failures (sound declines, every one loud)
and 44 -> 38 bails, code-bodies 5 -> 6 (NUR154's sound decline), the
sweep at its ceilings, the commit gate green.

## NUR092 closed — the stale arm consults the corpus (2026-09-25)

`TestVariationDifferential`'s stale arms re-check the UNSAMPLED rest of the
corpus (lazily, once) before calling a ledger bucket or a pinned variant
stale (`ledgerStale`, `TestLedgerStaleConsultsFullBreadth`), so an
unrelated spec row that displaces a seed from the hash-ordered sample can
no longer instruct the author to delete a live class's entry. NUR092 is
FIXED.

## NUR096 measured — the production registry's inert member (2026-09-25)

The record's retirement test was tried and failed one lane deeper: with
the two multi-return fnsig rows in `class.tsv`, the default runner and
`TestCheckTypeSoundness` agree on `10 10` (the check pass now models the
apply), but `TestSpecProd` — the production registry, no check pass —
leaves the multi-return member inert (`fn [[x:Integer] [Integer Integer]
[x x]] 10`) where the single-return rows apply. Rows withdrawn; NUR096
stays pending on that measurement.

## NUR105 closed — the folded map member (2026-09-25)

The last position. A map-literal member is a check-mode CONST FOLD
(`constFoldContainerVal`, a concrete sub-run with the pass OFF), so a
lambda written there was queued by nobody; the fold now queues every fn
value inside the folded constant (`noteFoldedFnBodies` →
`NoteFnBodyPending`) and the end-of-pass drain analyses it like the
list twin — `def m {k:([x:Any] => [nosuchw 1])}` reports `undefined_word`
at 1:23. NUR105 is FIXED.

## NUR119 closed — the param's name on both lanes (2026-09-25)

Verified on the record's witnesses after the fn-value seam work of this
run: `(app (z:Integer => [mul 3 z]))` and the `Any`-typed twin render `fn
g(Integer)` on both lanes, and `app sq/v` declines on the compiled lane
under the render gate rather than answering under another name
(`TestFnValueReadThroughParamRendersAlike`). NUR119 is FIXED.

## NUR122 closed — the read's own token (2026-09-25)

**The divergence.** A compiled fn-value apply had no name and no named-dispatch semantics: `f (z:String => [z]) 5` raised a return-count error compiled for the interpreter's `cannot call \`g\``, `f ([] => [42]) 5` agreed on the message and not the position (1:50 / 1:43), and a returned lambda's `(g x)` named an empty head at the unit's position (1:80) for the interpreter's `g` at 1:64. Recorded 2026-09-05; the NUR123 deopt increments closed the first two messages, and the paren-window spelling stayed open.

**The fix.** The last piece is the paren-window spelling's anchor. The recorded dyn-apply event (`recordDynApply`) and the residual trailing apply (`rec.dynTrailPos`) take the lead's READ position the unit noted (`wordReadPos`, from noteWordRead) when the lead value carries none — the check pass's carrier for a bare fn-typed binding read — so the runtime raise stamps at the `g` the interpreter blames, where the unit's position used to render "source position unknown". A first cut stamped the position onto the carrier itself and was reverted the same day: the residual layout's statement-boundary test (`crossesStatementEnd`) read the new position and applied a no-return lambda's leaked body residual at the main level (the sweep's `def` × container body variants crashed with `CALL_DYNAMIC underflow`). Every witness — the bare and paren spellings, the returned lambda, the position-only row — agrees on both lanes, message and position; `def ap fn [[g:Function][Any][(g 5)]]  ap ([x:Integer] => [x 1])` reads 1:31 on both (NUR118's fn-value seam, closed with it).

**Measured:** the closure-capture, no-match window, reach-marker, named-value, poly no-match, rematch, paren-trailing and fn-value suites green.

**Pins.** lang `TestFnValueSeamAnchorsAtTheRead` (five same-verdict rows). Docs: NUR.md (NUR122 FIXED; NUR118's second flavour closed), the handover, this entry.

## NUR114 closed — the token's own text (2026-09-25)

**The divergence.** `def x:(Integer gt 10) 5 x`: same code, message, row and column on both lanes, but the interpreter underlined the token (`^^^` under `def`) and the compiled lane one character — `stampAt` had no token text to copy, the debug table being row/col only. Recorded 2026-08-30.

**The fix.** Verified retired on the current tree: the recorder's event positions carry the parser's `SrcPos.Src`, the debug table keeps it and `stampAt` copies it onto the error (the carry the record costed against the table's size), so the witness renders identically. No code change in this entry.

**Measured:** the witness byte-identical on both lanes, caret included.

**Pins.** lang `TestTypedDefUnifyErrorCaretIsTheToken`. Docs: NUR.md (NUR114 FIXED — verified), the handover, this entry.

## NUR080 closed — the brand in both orders (2026-09-25)

**The divergence.** With `def UserId (refine Integer)  def b:UserId 9`, the compiled lane answered `typeof b` as `Integer` (the binding lost its brand) when `typeof 9` came first, and `typeof 9` as `UserId` (the literal gained one) when `typeof b` came first — the const-pool entry for the literal being reparented rather than a fresh value minted. Recorded 2026-08-18 from the Roc comparison study.

**The fix.** Verified retired on the current tree rather than fixed here: both orderings agree on both lanes (`Integer UserId true`; `UserId Integer`) — the typed-bind path mints its own branded copy (`ReparentValue` returns a fresh by-value copy) and the literal keeps its own. The record asked for both orders as acceptance rows and a spec section; both are added.

**Measured:** the two orders byte-identical on both lanes.

**Pins.** lang `TestTypedDefLiteralKeepsItsBrandInBothOrders`, user-types.tsv §NUR080 (two rows). Docs: NUR.md (NUR080 FIXED — verified), the handover, this entry.

## NUR130 closed — the condition's own token (2026-09-25)

**The divergence.** `while [] [1] end 5`: both lanes raise `runtime_error: while: condition produced no value`, the compiled terminal trap at the condition operand (1:7) and the interpreter at its spliced tape pointer — the `5` after the loop (1:14), or no position at all for `while [] [1]`.

**The fix.** The interpreter moves to the better anchor (the record's first direction): the `while` continuation carries the condition operand's position (`ForCont.CondPos`, seated by the `while` handler), and `stepMoveWhile` raises there. `if [] [1] [2]` declines the compile ("condition body produces no value"), so its anchor stays the interpreter's alone.

**Measured:** the core and basic vets clean; the two witnesses byte-identical on both lanes.

**Pins.** lang `TestWhileEmptyConditionAnchorsAtTheOperand`. Docs: NUR.md (NUR130 FIXED; NUR129's record narrowed the same day — the named witness declines soundly through the check pass's own diagnostic, pinned by `TestLoopNamedFnValueResidualDeclinesSoundly`), the handover, this entry.

## NUR146 closed — the frame's names (2026-09-25)

**The divergence.** `def k 5  for 2 [ if (k eq 5) [undef k] [] ]  9`: both lanes raise `undefined_word: k` at 1:21, but only the interpreter adds `did you mean \`i\`?` — its suggestion pool reads the registry, where the loop variable is a def; the compiled frame keeps `i` in a slot the registry never sees.

**The fix.** `localNameCandidates` (eng/go/vm.go) hands the running unit's slot→name table — a fn unit's `CompiledFn.LocalNames`, the main unit's new `Program.LocalNames` — to `UndefinedWordDiagWith` at every VM undefined_word site. The tables now name a loop variable (`AnalyseLoopBody` names the local it registers through the new `EmitRecorder.NameLocal` seam; the inactive default is pinned with its siblings), a carried or arm-bound def (`emitUnit.nameSlots` / `boundLocals`, merged at unit close by `unitSlotNames` and padded at finalize by `fillSlotNames`), beside the params and captures they already held; spill temps stay anonymous.

**Measured:** the core, check, compiler and eng vets clean; the inactive-recorder pins green; the witness renders the same help line on both lanes.

**Pins.** lang `TestCompiledUndefinedWordSuggestsFrameNames`. Docs: NUR.md (NUR146 FIXED), the handover, this entry.

## NUR135 closed — the last pop (2026-09-25)

**The divergence.** `TypeTable.Retire` deleted a minted node from the ID index whenever a popped binding had minted it, counting nothing: the same node pushed twice under one name — a bind twin replaying one captured entry per element — was retired by the FIRST pop, and the surviving level's node was gone ("unresolvable type operand"). Measured 2026-09-11 by the cross-request parity oracle; the arm-resident type twin worked around it by re-minting per element.

**The fix.** `DefTable.HoldsType` (core/go/deftable.go) reports whether any live entry, under any name, still binds a node; `UninstallType` and `PopLiveBinding` retire a minted node only when none does. The record's second face — a popped name's parts stay reserved — is the reservation rule NUR167's close documented as the language's on both lanes, unchanged.

**Measured:** the core retirement, uninstall and speculative-undef tests green.

**Pins.** core `TestMintedNodeRetiredByTheLastPop`. Docs: NUR.md (NUR135 FIXED), the handover, this entry.

## NUR142 closed — the family of a refinement (2026-09-25)

**The divergence.** `def S (refine FlexMap)  def w:S (flex {a:1})  w eq w` was `false` — on both lanes, and for a refined flex list, a refined plain map and `[w] deq [w]` alike: the register's rule is one equality per family, and a refinement is a member of the container family (dispatch admits it, `is` says so), yet `ExactEqual` and `DeepEqual` reached their container arms through `nodeFamily`, which folds exactly the kernel's flex and weak-flex nodes and returns a user-minted tag unchanged, so the arms were skipped and the terminal `false` answered. Recorded 2026-09-15 by the COLLECT oracle; `core.SameContainer` carried the oracle meanwhile.

**The fix.** `containerFamily` (core/go/equal.go) walks a USER-minted tag (`Origin == OriginUserDef`) to its nearest kernel ancestor and then applies the exact fold; the six equality arms (`ExactEqual`, `DeepEqual`, the deq-key scans) ask it. Ordering keeps the exact `nodeFamily`: a refinement may carry its own Comparer through `behave`, which the LCA walk must reach before any fold (`TestCompareSignaturesUserComparerOnPatterns` and `TestX5OrderedCompareErrorPath` said so when the first cut folded ordering too). `same_container_test.go`'s fence assertion is retired into its positive form.

**Measured:** the core comparison and equality suites green; the five rows answer identically on both lanes.

**Pins.** lang `TestRefinedContainerIsEqToItself` (five rows, the interpreter's own answers), refine-flex.tsv §3 (five rows). Docs: NUR.md (NUR142 FIXED), the handover, this entry.

## NUR118 closed — the call's own token (2026-09-25)

**The divergence.** `def h fn [[][Integer][1 2]] end h`: `type_error: h: expected 1 return value(s), got 2` on both lanes, anchored at the call word interpreted (1:33) and at the body's first value compiled (1:23); `def h fn [[n:Integer][String][n add 1]] end h 1` anchored at `h` (1:45) interpreted and at the argument `1` (1:47) compiled; the `do`-wrapped service shape NUR147's close made compile rendered "source position unknown". Pre-existing at the merge base — the corpus lanes compare a position's presence, not its value.

**The mechanism.** Two halves. The check pass recorded a user call with the FIRST ARGUMENT's position (`recordUserCallOrApply`'s `pos`), so the CALL_USER instruction's debug entry was the argument's, and empty for a 0-argument call. And the VM's RET stamped a nested frame's contract error at the callee's own last instruction (`stampAt(err, curDebug, pc)`), where the interpreter's ReturnCheck marker anchors at the call.

**The fix.** `callAnchor` (check/go/check_fnbody.go) seats the call WORD's position on the recorded call, the argument's only where the word carries none; the RET stamps a nested frame's contract error at the frame's return address in the CALLER's debug table (`retUnit` / `retPC-1`). Return count, return type and a runtime no-match over a gradual argument all anchor at the word on both lanes. One consequence surfaced in the sweep and is fixed with it: a 0-argument call's event position used to be empty, and a no-return lambda's leaked body residual (`def zzvlam ([] => [… def f m.f end f 5]) zzvlam` hands the caller its body's `f` and `5`, attributed to the call event) answered the residual layout's statement-boundary test (`residualPos`, NUR187's rule) with its own token positions, so the `end` between them settled the lead as data; with the call word on the event, both entries read as the call and the lead arm applied one over the other at the caller (`CALL_DYNAMIC underflow`). A call delivers its results resolved and the interpreter never re-steps a returned fn over the values returned beside it, so the dynamic-lead arm stands aside for a lead whose entries above it are outputs of the same call (`siblingCallOutputs`); a caller-written entry above a returned closure (`((w 5) 4)`, the paren rewind) still applies. (A first cut made `residualPos` prefer the value's own position and was reverted: it broke exactly that paren rewind.)

**Measured:** the affected lang tests green (the no-match window, rematch, poly no-match, island-burndown, do-defer, closure-capture and predicate pins).

**Pins.** lang `TestReturnContractErrorAnchorsAtTheCall` (five same-verdict rows). Docs: NUR.md (NUR118 FIXED — recorded 2026-09-04, its proposed verdict taken), the handover, this entry.

## NUR158 closed — the closure at the wrap (2026-09-25)

**The divergence.** `def mk fn [[k:Integer][Function][([a:Integer b:Integer] => [(a sub b) add k])]]  def mk2 fn [[][Map][{a:(mk 100)}]]  def m (mk2)  m.a/u 10 3` was 93 interpreted and `illegal_ref: usurp requires a function value, got Function` compiled — a capturing closure read out of a map is a `ClosurePayload` at the wrap words' poly seat, not the `FnDefInfo` their assertions want.

**The fix.** `rebarrierResult` (stack-args / forward-args), `forceArityHandler` and `usurpHandler` bridge the closure to its fn definition (`ClosureAsFnDef`) before asserting; the wrapper stores that definition and the re-dispatch runs the closure. All four modifier spellings and the word forms agree.

**Pins.** lang `TestWrapWordsBridgeACompiledClosure`, fn-value.tsv §16. Docs: NUR.md (NUR158 FIXED), this entry.

## NUR157 closed — two references to one predicate (2026-09-25)

**The divergence.** `def Pos fnpred n:Integer [n gt 0]  def T fnsig [[xs:[:Pos]] [Boolean]]  ((fn [[xs:[:Pos]] [Boolean] [true]]) unify T)` was `~unify-fail` under a registry and admitted without one: the armed pre-pass resolved one pattern's `Pos` to its predicate and ran it against the OTHER pattern's `Pos` — a membership test where a type comparison was meant. `[:Pos] unify [:Pos]` failed the same way.

**The fix.** One arm in `unifyInner`'s pre-pass: when BOTH sides resolve (`resolvePredicateRef`) to the same predicate type — atoms, bare nodes, or the predicate's fn value, which is what a `[:Pos]` pattern's child holds — they unify as the type. Negatives (`[:Pos]` vs `[:Neg]`) and membership rows are unchanged. The `unify` word stays uncompilable, so the compiled lane's answer is the fallback's.

**Pins.** lang `TestSamePredicateReferencesUnify`, fnpred.tsv's closing section. Docs: NUR.md (NUR157 FIXED), this entry.

## NUR154 closed — the computed clause list (2026-09-25)

**The divergence.** `def mk fn [[n:Integer][List][quote [1 'one' 'many']]]  case 1 (mk 0)` was `'one'` interpreted and `case_error: clause list must be a concrete list` compiled — raised by a TERMINAL trap the check pass recorded (`CaseReturnsFn`: any non-code-body clause operand trapped), where under analysis a fn's returned list is a carrier whose shape only the run knows.

**The fix.** The trap is recorded for a CONCRETE non-list only (`case 1 5` still raises on both lanes); a carrier or dynamic operand yields the dynamic result and the dispatch's own gate declines the computed clause list ("code-body word case (Stage 2)"), so the program falls back and answers. The corpus ledger's `knownDivergences` pin for code-bodies.tsv:L142 and the sweep's NUR154 pin are retired (`sweepFailureCeiling` 26 → 27; the sweep's pin-fixture test seeds its own entry now that the register holds no open sweep pin). **Scoped (same day, the milestone batch).** The trap fires for a KNOWN non-list — a scalar or a bare type node (`case 1 Integer`, the island-burndown pin) — not only a concrete one: the gate is "not a carrier, not dynamic". The compile-failure ledger records code-bodies.tsv:L142 as the decline it now is (7 → 8).

**Pins.** lang `TestCaseComputedClauseListDeclines`. Docs: NUR.md (NUR154 FIXED as a sound decline), this entry.

## NUR147 closed — the seat's other arities (2026-09-25)

**The divergence.** `def svc (service {})  add {} ([r:Map state:Any] => [1]) svc  call {} svc  call {} svc` answered `1 1` by whole-program fallback and raised an internal error under `-force-compile`: the check pass matched the second `call` with the first's gradual residual standing in for the three-operand overload's third Map, the record committed to three operands, and the seat re-matched only at that count when the run's third value was an Integer (`vm:poly-no-match`).

**The fix.** On a no-match at the recorded count `callPoly` retries every other arity some non-fallback overload takes, from the recorded count down, over the same stack top (`polyHasArity`), and dispatches the first match — the interpreter's own pick over the live values. `1 1` compiled, no defer. **The `do`-wrapped shapes (same day, the milestone batch).** `TestDoDeferFallsBackNotTrapped`'s three bail witnesses compile and answer as the interpreter now; the fn-body one exposed NUR118.

**Pins.** lang `TestPolySeatRetriesOtherArities`. Docs: NUR.md (NUR147 FIXED), this entry.

## NUR172 closed — the attempted window (2026-09-25)

**The divergence.** `5 $.name apply`: both lanes raised `signature_error: cannot call dot — no signature matches the arguments`, but the NOTES described two failures — interpreted `the argument was 5 (an Integer) … takes 2 arguments, but 1 was supplied`, compiled `the arguments were name (an Atom) and 5 (an Integer) … argument 2: expected Module, got 5`. The record put the fix on the interpreter: its report described the window the collection managed to FILL, and the compiled poly window is the truer report.

**The fix.** `core.attemptedWindow` replaces the fill-else-prefix pair in `sigError`: the forward candidates, a bare word right after the pointer counted as the Atom when some overload's first slot is `/q` (`ReorderForwardCandidates` stops at a word), and, when those are fewer than the smallest overload's arity, the stack prefix beneath, in signature order — the poly window's own layout. `5 $.name apply` and `5 dot name` now read identically on both lanes; `'x' add` (a pure stack no-match) is unchanged. The corpus ledger's `knownDiagDrift` pin for reach.tsv:L52 is retired. **The compiled twin (same day, the milestone batch).** The check pass's record-time written tuple (`rematchWritten`) derives the same window over its carrier-aware forward walk (`attemptedWindowOver`), feeding the poly no-match spec and the runtime-rematch trap; the trap's render bound became an index tuple (`DispatchSpec.Written`, replacing the `WrittenOff`/`NWritten` pair) because a mixed tuple — a forward operand and the stack value beneath it — is not a contiguous slice of a window that lists the stack run first. `(x add 1)` over a gradual x, `[1 2] each [dup mul]` under a variadic lead and the former bounded edge `9 1 x add` now render byte-identically (`TestPolyNoMatchDeeperStackShapeRaisesCanonical` replaces the internal-error pin).

**Pins.** lang `TestNoMatchWindowIsTheAttemptedOne` (three rows' headline and notes byte-equal across the lanes). Docs: NUR.md (NUR172 FIXED; NUR171's record notes the two anchors tried), this entry.

## NUR170 closed — the dynamic emit lead (2026-09-25)

**The divergence.** `import "boru:emitlang"  def m {up: (fn [[value:Any opts:Map] [String] ['UP']])}  emit m.up {a:1}` was `UP` interpreted and `signature_error: cannot call emitlang-auto — the arguments were {a:1} (a Map) and fn (Any, Map)` compiled — the sweep's `emit` × container pin.

**The mechanism.** Not a slot: the ROUTE. Under analysis `m.up` is a dynamic Any carrier, not a Function to the macro's fn-operand test, so the `emit` macro committed the lead to the data route (`emitlang-auto <lead> <opts>`, the disassembly's `CALL_NATIVE_POLY emitlang-auto/2` over the fn and the map) where the interpreter's expansion reads the concrete value at run time and takes the fn route (`fn data opts end`).

**The fix.** The macro degrades a DYNAMIC lead under analysis as it degrades a not-concrete Function lead — an advisory and a dynamic String carrier — so the program declines loudly ("residual value of unknown provenance") and the fallback runs the emitter; a wrong answer became a compile failure, the sweep pin is retired and its seed counts under `sweepFailureCeiling` (25 → 26). The bound data lead (`def d {a:1} emit d`) and the kind lead compile as before; `mini` and `parse` already declined a dynamic lead loudly.

**Pins.** lang `TestEmitDynamicLeadIsNotRoutedAsData`. Docs: NUR.md (NUR170 FIXED as a sound decline), this entry.

## NUR277 closed — the marker's intent on the value (2026-09-25)

**The divergence.** `def m {f: (fn [[a:Integer] [Integer] [a add 1]])}  m.f/v 5` was `fn (Integer) 5` on the interpreter (the marker says data; the 5 strands) and `6` compiled; `5 m.f/v` was `5 fn (Integer)` for `6`. Found closing NUR262; pre-existing.

**The mechanism.** The disassembly was `CALL_NATIVE_POLY dot; PUSH_CONST 5; CALL_DYNAMIC /1` — Finalize's residual layout applying a dynamic lead, not the shaped member apply. Under the check pass the member read is a DYNAMIC Any carrier: execFnDefLiteral's marker peek (which quotes the concrete value at run time) needs `fnDefAtPointer`, which fails on a carrier, so the marker reached the pointer standalone and was dropped as a no-op, and the pass's residual held an unquoted dynamic lead beside the 5.

**The fix.** The standalone-marker drop quotes the dynamic or carrier value the marker follows — the peek's rule applied to the value the pass holds — and `resolveDynamicApply` honours the quote: the dynamic-lead arms and the `mayBeFn` arm skip a quoted lead, the "dynamic value precedes residual args" boundary exempts one (data beside its neighbours), and `trailingApply` declines a quoted trailing value. `m.f/v 5` compiles to the data layout; `5 m.f/v` declines at the existing "call result above a literal" residual limit and the fallback answers as the interpreter.

**Measured:** the compiler and core builds; the NUR262/207 pin green. The broader suites run in the next batch. **Scoped (same day, the milestone batch).** The drop quotes a dynamic or carrier value only; a concrete Function value is the peek's at run time on both lanes. A quoted fn-typed carrier passes the residual render gate as data — `(f MathUtil.sqrt/v) 16.0` compiles again (its render divergence stays the measured-open NUR119 shape `TestClosureCaptureOpenShapes` pins).

**Pins.** lang `TestReachValueMarkerIsNoArgument` (four more same-verdict rows, the trailing decline's parity, the interpreter's own answers), fn-value.tsv §15 (three rows). Docs: NUR.md (NUR277 FIXED), the handover, this entry.

## NUR262 closed — the marker is no argument (2026-09-25)

**The divergence.** `import module [ def up1 fn [[value:Any] [String] ['UP']] export "M" {up1: up1/v} ]  def g M.up1/v` raised `signature_error: cannot call def — no signature matches the arguments … none were supplied` on both lanes, where `def g M.up2/v` (an Integer first parameter), `def g (M.up1/v)`, `def g up1/v` and the bare `M.up1/v` all bind or answer. NUR163's record carried it as a side note.

**The mechanism.** The parser emits a dotted path's `/v` as the reach followed by a dispatch-modifier marker (`Word/__DM`). In `def`'s forward window the plan asks whether the reach is a CALL HEAD that would claim what follows (`ReachCallHeadBarrierOn` → `ReachFnWouldClaimOn`), probing the next token through `ForwardClaimProbeOn` — which had no arm for a marker: it fell to the literal arm and was offered to the fn's first parameter. `Any` matched it, so the reach read as a call head about to claim its own marker, a barrier, and the window collected nothing.

**The fix.** `ForwardClaimProbeOn` answers `probeNone` for a dispatch-modifier marker: it qualifies the value before it and is never an argument. The reach is one datum again and the value binds as the bare `/v` word always did. Found beside it and recorded as NUR277: a `/v`-marked MAP member with arguments beside it (`m.f/v 5`) is applied on the compiled lane and data on the interpreter (the checker's shaped member apply collects the window past the marker); pre-existing.

**Measured:** the core and lang root suites green; the langspec gates over module-fnvalue-boundary.tsv at their ceilings.

**Pins.** lang `TestReachValueMarkerIsNoArgument` (twelve same-verdict rows, the interpreter's own answers for three), module-fnvalue-boundary.tsv §5 (four rows). Docs: NUR.md (NUR262 FIXED, NUR277 recorded, NUR260's record traced), the handover, this entry.

## NUR261 closed — the named value's no-match on the seam (2026-09-25)

**The divergence.** `def h fn [[a:Integer b:Integer] [List] [[a b]]]  0 fold h/v [1 2]`: step 0 answers `[0 1]`, step 1 offers that List to `a:Integer` and no signature matches — `fold: step 1: uncalled_function: call to 'h' matched no signature` interpreted, `[fn (Integer, Integer)]` compiled (the value itself). A no-match at step 0 raised on both lanes; `scan` parks on both.

**The mechanism.** The token seam's `unmatchedLambdaBody` — the arm for a callback body unit carrying the value's own param contract that no step matches — applied NUR155's rule (an unmatched typed LAMBDA at its call site is data, the interpreter's own answer for an anonymous value) to every closure, a named value's included; step 0 took the fn-VALUE arm (`invokeFnValueClosure`), which steps the value on the interpreter and raises there.

**The fix.** A closure compiled from a NAMED value carries the def's name (`ClosurePayload.RetName`); the arm raises the word's `uncalled_function` for it — the interpreter's own detail and hint (execFnDefLiteral's) — and keeps the data rule for an anonymous lambda.

**Measured:** the eng and lang root suites green; the langspec gates over callbacks.tsv at their ceilings. **Anchored (same day, the milestone batch).** The raise carries the reference token's position (`ClosurePayload.RetPos`, `h/v` at 1:60) — the corpus lane had flagged the compiled raise as positionless.

**Pins.** lang `TestNamedValueNoMatchOnTheSeamRaises` (eight same-verdict rows and the interpreter's own verdicts), callbacks.tsv §11 (five rows). Docs: NUR.md (NUR261 FIXED), the handover, this entry.

## NUR163 closed — the value's own signatures (2026-09-25)

**The divergence.** With a module exporting `dbl` (`fn [[src:String opts:Map] [String] [src add src]]`), `mini M.dbl 'ab'` and `mini (M.dbl) 'ab'` raised `mini_bad_signature: every signature must start with the standard prefix [src:String opts:Map …]` while `def g M.dbl/v end mini g 'ab'` answered `abab`; `emit M.up {a:1}` and `parse M.p 'xy'` the same way. The record filed it as the member read hiding the fn's signatures. Measured closer: `mini (dbl/v) 'ab'` fails identically with no module at all. What a `/v` read, a member read in place and a paren deliver is `Registry.Lookup`'s dispatch AGGREGATE, which carries a synthesized 0-arg fallback signature (`Signature.Fallback`, an empty parameter list); the def-bound spelling stores the own signatures (installDef's rebind arm) and passed. The contracts iterated `fnDef.Signatures` and the fallback failed the prefix check.

**The fix.** `MiniLangFnSigWhy`, `EmitLangFnSigWhy`, `ParseLangFnSigWhy`, `MiniLangFnFilterShaped` and both callers of `miniPartialFromSigs` read `FnDefInfo.OwnSigs()`. The filter-shaped `/v` value (`mini (f3/v)`) used to crash the partial's build once past the probe — `index out of range [2]` over the fallback's parameters — and builds over the own signatures now. Interpreter-side, both lanes. The record's side note — `def g M.up/v end` raising `cannot call def` for an export whose first parameter is `Any` — is a forward-window quirk of its own (the reach's value reaches the pointer ahead of its dispatch-modifier marker inside `def`'s window), recorded as NUR262.

**Measured:** the lang native and modules suites green; the langspec gates over module-minilang.tsv, module-emitlang.tsv, module-parse.tsv and module-parselang.tsv at their ceilings.

**Pins.** lang `TestMiniLanguageValueIsTheFn` (eleven same-verdict rows, the interpreter's own answers for five), the `§fnv` / `§10` closing sections of module-minilang.tsv (five rows), module-emitlang.tsv (three) and module-parselang.tsv (two). Docs: NUR.md (NUR163 FIXED, NUR262 recorded), the handover, this entry.

## NUR162 closed — the 0-arg apply at a paren's tail (2026-09-25)

**The divergence.** `(def dbl word ([] => [1]) end 5 dbl)` — a `word` bound to a 0-arg fn VALUE, spliced at a paren's tail — is `5 fn` on the interpreter (the value is data; ADR-016's gate parks a 0-arg lambda value) and PANICKED the compiler's disassembler (`disasmUnit`'s `OpCallNative` arm over a `SigRef` whose `Sig` is nil); the VM dereferenced the same entry at run time (an internal_error). The module-body form the same. The sweep's two `word` × lambda variants carried it as its crash ceiling.

**The mechanism.** The trailing fn-value apply (`RecordDynApply`) trims its window to the callee's own arity; a 0-arg callee trimmed it to NOTHING, so the record carried `dynApply: 0`, and the lowering — which routes an apply by `dynApply > 0` — emitted it as a plain native call under a signature entry with no signature.

**The fix.** The window trim declines a 0-arg callee ("trailing fn-value apply of a 0-arg callee (the value is data at the tail)") at the recorder's ONE failure site — the compile-failure site census is a downward ratchet, and the insufficient-args arm already shares it — so the program falls back and answers `5 fn`; `RecordDynApplyName` declines an empty window up front; the lowering keeps a belt ("fn-value apply over no argument (no signature to lower under)"); and the disassembler names a signature-less entry ("NO SIGNATURE — recorder defect") instead of crashing, so the classifier that reads it never goes down with it again. The sweep's two variants decline loudly now: `sweepVariantCrashCeiling` 2 → 0, `sweepVariantFailureCeiling` 171 → 173 (a crash turned into a sound decline, not new debt).

**Measured:** the compiler and eng suites green; the generated sweep's crash count 0.

**Pins.** lang `TestZeroArgTailApplyNeverCrashes` (the two rows decline with the fallback's `5 fn`; the unbracketed twin compiles). Docs: NUR.md (NUR162 FIXED), the handover, this entry.

## NUR166 closed — the value's own frame (2026-09-25)

**The divergence.** `def g fn [[n:Integer] [Any] [do [args]]]  each g/v [1 2]` was `[[1] [2]]` on the interpreter and `[error(args: not inside a function) error(…)]` compiled; `do [args] size` gave `1 1` for `0 0`. The recorder lowers the def-bound value's body as the `each` site's closure unit (`each$body/1`, params `[n]`), the native enters it through the token seam (`invokeClosureOn` → `applyClosure`), and the seam pushed no args list — `pushRootArgs`, the DynEnv args bracket for a fn VALUE's root frame, bracketed only the units the fn-value seam hosts (the token seam's value arm, RunUnit, a foreign ref). `args` read directly in the body already answered: it folds to the param locals at compile time. A `do` body inside the value reads the list at RUN time (the program is DynEnv), and found the enclosing frame's — none at the top level.

**The fix.** `applyClosure` brackets a closure unit that carries a param contract — `CompiledFn.Params` non-empty: a named fn's body or a lambda's, compiled at the callback slot with the value's own params — with `pushRootArgs` over `args[:NArgs]`, the frame the interpreter's dispatch opens for the value. A quotation body (`each [do [args]] xs`) carries no contract and runs in the caller's frame, reading the caller's args, on both lanes (`def w fn [[x:Integer] [Any] [each [do [args]] [x 5]]]  w 7` is `[[7] [7]]`; the fn-value twin `[[7] [5]]`). No-op outside a DynEnv program.

**Found beside it, recorded as NUR261:** a NAMED fn value driving `fold` whose signature stops matching past step 0 is parked as data compiled (`unmatchedLambdaBody`'s NUR155 rule, meant for an anonymous value) where the interpreter raises `uncalled_function`; pre-existing at the merge base.

**Measured:** the eng suite and the lang root suite green; the langspec gates over callbacks.tsv at their ceilings.

**Pins.** lang `TestFnValueCallbackHasOwnArgs` (twelve same-verdict rows, the interpreter's own answers for four), callbacks.tsv §10 (five rows). Docs: NUR.md (NUR166 FIXED, NUR261 recorded), the handover, this entry.

## NUR167 closed — the analysis is not a call (2026-09-25)

**The divergence.** `def f fn [[n:Integer] [Integer] [def T (class {}) n]]  each f/v [1 2]` raised `type: name part "T" in "T" conflicts with an existing type name` at each's element 1 on the interpreter and at element 0 compiled; `each f/v [1]` raised compiled where the interpreter answered `[1]`. The conflict is a name-PART reservation, not the node: `validateTypeName` asks `IsKnownPart`, and a fn-body mint reserves its part for the registry's lifetime (the frame teardown pops the binding and keeps the part — `undef T` inside the body retires the node and keeps it too), so the interpreter's second call conflicts by rule. The check pass's analysis of the body reserved the part as a call would, and the run's first call — the interpreter, through the callback seam, since the lazy stamp declines the body (bodyHasReplayHazard) — collided with the pass's leftover.

**The fix.** `RunFnBodyOnce` snapshots the reservations before the body runs (`core.TypePartsSnapshot`) and forgets, at its def unwind, every part the body reserved for a binding it popped (`core.ForgetTypePartsSince`; a part whose binding is still live is kept, and the node stays in the ID index — the sandbox's "mints retained"). Two more fell out and were closed with it. (1) With the pass no longer conflicting against its own second analysis of the body, a fn UNIT with such a body compiles — and `RecordTypeInstall` recorded nothing outside an arm-resident bracket (the ledger keeps fn-body transitions out, so there is no twin either), so the unit DROPPED the mint: `f 1 f 2` answered `[1 2]` for the interpreter's raise. Three shapes were tried. A new `MarkUncompilable` in the recorder broke the compile-failure site census (a downward ratchet: 92 sites over a ceiling of 91) and every `do` body with a type install (the keep-defs unit is exactly where the adopted twin replays). A screen at the unit admissions (`core.BodyHasReplayHazard`, the lazy stamp's, moved to core) kept the census but declined the generated sweep's fn-wrapped variants of every type-defining seed and every `import`-bearing one — 291 declined call forms over a ceiling of 197. What stands is the generic lowering both gates ask for: the recorder's fn-unit arm records the install with the entry the check pass pushed (`emitDynBind.fnType`; the hook now carries the entry — `EmitRecorder.RecordTypeInstall(name, entry, pos)`), and the lowering emits `OpBindFnType` (`Program.FnTypeBinds`): the VM checks the name against the run's reservations exactly as the front door does (`core.TypeNameFree`, shared with the twin replay — a live same-named binding is a redefinition and passes), reserves it (`core.ReserveTypeParts`), and binds the check-time node as an ADOPTED entry on the dyn-bind trail, so the frame's RET pops it as it pops the unit's value defs. That is the interpreter's observable exactly: the first call answers, the second conflicts; a stored body run in a CallBoru frame (`Test.check-prop`'s gen body, `bytecode_checkprop_units_test.go`) conflicts on its second trial where it used to decline the compile; a `do` body nested in a fn (`def rpt fn [[] [Any] [do [def Big Integer 15 is Big]]]  (rpt) (rpt)`, `bytecode_replayhazard_test.go`) leaves the binding on the trail for the fn's RET, the leak-then-pop. A ROOT `do` body (FnBodyDepth 0) still records nothing: the adopted twin replays it. The adopted flag is deliberate: the interpreter mints a fresh node per call, but that node is unobservable past the raise its second call is, while the unit's `OpPushType` reads bake the check-time node and an `undef T` in the body must not retire it. (2) A ROOT `def T (class {})` after the call kept its reservation from check time (the twin regime retains mints), so the callback's mint conflicted at element 0 where the interpreter's root def conflicts: `BindingSandbox` now captures the reservations, both restores free the parts of the bindings they roll back, and the type twin (`applyTwinPush`) re-checks its name against the run's reservations at its own position and reserves it again; `ApplyBindTwin` returns that raise and `OpBindTwin` stamps it. `ApplyResidentTypeBind`'s doc, which leaned on the retained reservation, is unaffected: it enters at `InstallTypeBody`, past the front door.

**Measured:** the core, check, compiler, eng and lang root suites green; the langspec gates over user-types.tsv, class.tsv, callbacks.tsv and the bind-replay sandbox at their ceilings (user-types.tsv's check-accuracy pin rose 1 → 6 for the five new error rows the checker cannot flag statically — the conflict is a run-time fact of the second call); the compile-failure site census unchanged at 91; the generated sweep's call-form debt back under its ceiling (SWEEP_STATUS.md refreshed). The direct call of such a body (`f 1`, `f 1 f 2`) compiles natively now where it was the pass's own self-conflict before.

**Pins.** lang `TestFnBodyTypeMintIsTheCalls` (twenty-one same-verdict rows — the callback, direct-call, nested-do and check-prop shapes — and the interpreter's own verdicts), user-types.tsv's closing section (eight rows), the two lang tests above re-pointed at native parity. Docs: NUR.md (NUR167 FIXED), the handover, this entry.

## NUR168 closed — the def's name on the value (2026-09-25)

**The divergence.** `def mk fn [[k:Integer][Function][([s:String] => [s])]]  def f (mk 1)  each f/v [1 2 3]` rendered `fn f(String)` three times on the interpreter and `fn (String)` three times compiled: installDef names every fn value a `def` binds, and the compiled lane named only a produced CLOSURE at its promoted store (StoreNames, the twenty-ninth increment) — a factory's capture-free lambda is baked as a const and carried no name. Two neighbours on the same value surfaced while closing it, both pre-existing at the merge base: `do [f/v]` read `fn f(String) or (String)` compiled (the root's dyn-scope bind and its global write-back each installed the value, and Registry.Lookup unions the entries of a name), and `do [f 'z']` was an internal error for the interpreter's `z` (the do-body's callee read lowered to OpLookupDynScope, which defers on a fn definition — "vm:dyn-scope-dispatching"). The capturing twin agreed on all three, which is why they hid.

**The fix.** (1) RecordDynBind seats the def name on a `producedConstLambda` as on a produced closure, and `nameClosureValue` renames a bare `FnDefInfo` as it renames a `ClosurePayload`; `bindGlobal` names what it writes. (2) A write-back paired with a dyn-scope bind of the same def carries `GlobalBindSpec.AfterDynScope`; the VM then adopts the bind's install — `adoptDynBind` takes the entry off the dyn-bind trail so no unwind pops it, and the write pushes nothing — one entry under the name, as the interpreter's one `def` leaves. The install is bindDynScope's, through `core.InstallDef`, so a redefinition's overlap removal holds too (`def f (mk 1)  def f (mk 2)  do [f/v]` is one signature). (3) `RecordDynMethod` routes a dyn-scope callee read to the data-position twin (`OpLookupDynScopeData`), the rule the closure-tail apply already applied.

**Measured:** the lang unit ledger 285 / 44 unchanged; the compiler, eng and lang root suites green; the langspec gates over fn-value.tsv at their ceilings.

**Pins.** lang `TestDefBoundFnValueIsNamed` (nineteen rows with the same verdict on both lanes and the interpreter's own answers for five), fn-value.tsv §14 (seven rows, the display render through `join`). Docs: NUR.md (NUR168 FIXED), the handover, this entry.

## NUR176 closed — the 0-arg lead's window (2026-09-25)

**The divergence.** `def z fn [[] [Integer] [7]]  def inc fn [[n:Integer] [Integer] [n add 1]]  def h fn [[k:Function] [Integer] [(k inc/v)]]  h z/v` was 8 on the interpreter (z fires over nothing, inc dispatches over the 7 beneath it) and `signature_error` on the compiled lane: the leading one-arg window `(k x)` lowers to `OpCallDynTrailTop` over `[x, k]`, and a lead none of whose overloads takes an argument raised the no-match a 1-arg window owes (NUR107's rule). `(k 5)` and a 0-arg lambda under `(g 3)` were the interpreter's count error for the same raise.

**The mechanism.** The interpreter dispatches a bare read of a fn-typed binding as a WORD where it stands: a 0-arg fn fires with nothing, its result lands, and the window's other tokens are then stepped on their own — a leading window's arguments after the result (a fn value dispatching over it, a literal beside it), a trailing window's beneath it. The op bound the whole window as the lead's arguments and asked the match to admit it.

**The fix.** `callDynTrailTop` asks `FnValueOnlyZeroArgSigs` (the screen the re-step landing already runs, NUR175) of a NAME-read lead and hands such a window to the island in its written order: `DynApplyHead` carries a `Leading` bit now (set by `RecordDynApplyLead`), so the island runs `lead args…` for `(k x)` and `args… lead` for `(1 2 c)`, with the lead marked applied (`FnDefInfo.Applied`) so the island's re-step dispatches an anonymous 0-arg value exactly as the word dispatch did. The island IS the interpreter's residual semantics over a token window — the record's "second dispatch path inside an op" was already there as the op's last resort — so the change is one arm before the no-match. An event-produced or `/v`-delivered lead has no name to dispatch under and keeps the no-match.

**Measured:** the lang unit ledger 285 / 44 unchanged; the compiler, check, eng and lang root suites green. No corpus row carries the shape.

**Pins.** lang `TestApplyShapesZeroArgLeadResolves` (the three rows and their neighbours with the same verdict on both lanes; the interpreter's own answers). Docs: NUR.md (NUR176 FIXED), the handover, this entry.

## NUR191 closed — the module fn's return contract (2026-09-25)

**The divergence.** `import module [def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] def d1 fn [[x:Integer][Integer][(mk x) 3]] export "M" {d1: d1/v}]  M.d1 10` was 13 on the interpreter and the frame's count error on the compiled lane; the same fn on the main registry raised the count error on both. Probing the family found the root beneath the park: `def d1 fn [[x:Integer][Integer][x 3]]` as a module fn answered `[10 3]` — the CallBoru seam only type-checked the aligned tail of the residual (enforceCallBoruReturns, NUR069's fix) and handed the whole residual back, never the count the spliced frame's ReturnCheck raises. The residual `[fn 3]` then reached the caller's tape through execMatch's result splice, where the parked closure re-stepped over the 3 (the frame path parks a fn frame's Function return — fnReturnPark — and a handler's results were re-presented from the first argument's index). The compiled module fn in a whole-program compile enforces the count (checkReturnContract's frame discipline, "the documented frame-path asymmetry" it names), so the divergence was the interpreter's module path against everything else.

**The fix, in three parts.** (1) A NAMED fn call through the seam enforces the frame's return count BEFORE the types, as the frame does: `CallBoruStrict` (core registry.go) is CallBoruNamed with the count check (`NamedFnReturnCount`, skipped inside a predicate call and in check mode) ahead of enforceCallBoruReturns; execFnDefLiteral's cross-registry arm takes it in check mode and `InvokeCallbackStrict` at run time, buildFnBodyHandler's foreign-registry arm takes it too. The callback seams (InvokeCallbackFn, InvokeCallback, the fn-VALUE apply) keep the CallBoru discipline — the count trimmed, RetTrim — which is NUR120's contract for a lambda handed to a native. (2) The stamped path agrees: `InvokeCompiledStrict` on the CompiledRuntime seam runs the unit as a named entry (`runUnit`/`runUnitNested` with `named`, the nested runner's `namedUnitRef` wrapper), and the root RET of such a run takes `checkReturnContract` with the frame's count discipline (`vmContext.rootRetNamed`) instead of checkCallBoruContract's trim; the token seam clears both flags as it cleared one. (3) A user fn's single returned closure delivered as a HANDLER RESULT is parked where it lands (execMatch after the splice: `sig.FnFrame() != nil`, one value that would dispatch at the pointer, stepped past) — the frame path's rule; `3 M.d1 10` over a factory-shaped module fn is `[3 fn (Integer)]` on both lanes, `(M.d1 10) 3` parks, `((M.d1 10) 3)` and `3 (M.d1 10) apply` apply.

**What leaned on the leniency.** Three fixtures declared one return and left more: boru:repl's `repl-eval-line` (`st set history …` left the store beneath its reply; the preamble drops it now), the dyn-env drift test's `srv` (three values; it declares `[Any Any Any]` now) and a process test's `worker` (`send` leaves nothing; it declares `[]` now). Each would have raised on the main registry's frame path all along.

**Found on the way, recorded:** NUR260 — a module fn returning a NAMED fn value read through its reach group with a value beneath (`5 M.ff` over `def ff fn [[][Function][inc/v]]`) is 6 on the interpreter (the reach group never parks: its collapse re-steps the named survivor, and a name always calls) and `[5 fn inc(Integer)]` on the compiled lane, which seats the value as data; `5 (M.ff)` and the main registry's `5 ff` park on both. Pinned pending (`TestModuleFnNamedValueThroughReachPending`).

**Pins.** lang `TestModuleFnReturnContractIsTheFrames` (module_fn_return_contract_test.go: NUR191's witnesses and the count contract's neighbours with the same verdict on both lanes — an error compared by code and detail, the positions differing by lane per NUR118 — the interpreter's own answers, the main-registry twins), `TestModuleFnStampedAtLoadAndRerouted`'s decliner probe (the count error on both interpreters), core `TestNoCompiledRuntimeDeclines` (the strict seam's inactive default); edge-modules-2.tsv §12's seven rows. Docs: NUR.md (NUR191 FIXED, NUR260 recorded), the handover, this entry.

## NUR198 closed — the None receiver's bare-word key (2026-09-25)

**The divergence.** `def f fn [[m:Map][Any][m.b.c]] end f {a:1}` was the interpreter's `undefined word: c` and the compiled lane's None; `((make C {}).x get 0).y` the same. `m.b` over `{a:1}` is None on both lanes, so the question was the second hop over a run-time None.

**The mechanism.** The accessor table's `[Key | None]` row — "chained-read propagation", the row that makes `none get 'c'` and `None dot (quote x)` None — carried no QuoteArgs, so it never collected a BARE-WORD key: the matcher parked `dot` on a pending forward over the None receiver and the word `c` stepped on its own as undefined_word (a no-match over any other receiver raises `cannot call dot`; only None parked). The check pass mirrored the park for a static None (`none.c` failed to compile at the same undefined_word) and modelled a run-time None behind a typed receiver as a member read, which the compiled chain answered None — the compiled lane held the rule, and the row's own promise.

**The fix.** The atom row quotes the key: `{TAtom, TNone}` with QuoteArgs beside the `{TAny, TNone}` row for an evaluated key, on `dot` (`get` strips the quote as it strips every other row's) and on its strict twin `dotr` (`m.b!.c` raises the family's `getr: parent is None` on both lanes, never undefined_word). `m.b.c` over `{a:1}` is None on both lanes, so are `none.c`, a None behind an `Any` param, a list element's and an Error value's missing member, and `5 m.b.c` keeps the 5 beneath.

**Measured:** accessor.tsv, control.tsv and fn-locals-scope.tsv under every langspec gate at their ceilings; no corpus row moved.

**Pins.** lang `TestDotChainOnMissingMemberResolves` (map_literal_flex_member_test.go: the three shapes and their neighbours as parity rows, the interpreter's own answers); accessor.tsv's None section (five new rows), REFERENCE.md's absence example. Docs: NUR.md (NUR198 FIXED), the handover, this entry.

## NUR197 closed — the loop region's residual, and two twins found and closed (2026-09-25)

**The divergence.** `for 2 [[(i add 1)] i]` was the interpreter's `undefined word: i` and the compiled lane's `[[1] 0 [2] 1]`; `{a:(i add 1)}` and `{a:(flex [i])}` likewise, and the same loop inside a fn the interpreter's undefined_word for the compiled lane's count error. With an OUTER `i` bound the interpreter read that one (`def i 9  for 2 [[(i add 1)] i]` was `[[10] 0 [10] 1]`).

**The mechanism.** The interpreter's `for` runs its body in place on the top tape (the mark/move continuation), and a literal the body leaves stays PENDING until the end-of-run sweep (autoEvalStack) — by which time the loop has unbound `i`. A fn frame evaluates its body's residual in-frame (ResidualEvalsInFrame); a loop region had no such moment. The compiled lane assembles the literal where it stands, with the iteration's `i`, which is the answer a reader expects and the rule the record was directed at.

**The fix.** The loop region is the literal's frame: `Engine.collectLoopRegion` — shared by the `for` continuation and the `while` body region (stepMoveCont, stepMoveWhile) — evaluates a pending residual container when it collects the iteration's output, with the iterator still bound; every other value, a typed container's inert shape included, is collected as it stood and resolved by the end-of-run sweep as before. `for 2 [[(i add 1)] i]` is `[[1] 0 [2] 1]` on both lanes, the outer binding never read, the while body's literal its round's (`[[2] [3]]`), and the fn shape the fn's own count error on both.

**Two twins found probing the neighbours, both closed.** NUR209 (NUR256 since the merge of main's #511): a `do` body that is ONE container literal over the loop variable — `for 2 [do [[i]]]`, `for 2 [do [{a:i}]]` — answered `error(undefined word: i)` per iteration on the COMPILED lane for the interpreter's `[0] [1]`: the token body's closure compile passed `bodyInFrame` false, so AnalyseFnBody analysed it as an anonymous lambda whose single bare literal DEFERS, recorded no assembly, the closure declined on the unknown provenance, and the dyn-body backstop baked `[[i]]` as a const (a word inside a nested compound is an inert const member) the handler re-ran through the interpreter — where the loop's `i` is a frame slot the registry never held; a multi-token body (`do [[i] 5]`) compiled to a closure assembling the list. A token body compiles in-frame now (recordClosureDispatch's token site passes true — the InvokeBody seam's sub-engine sweeps the residual at its end with the bindings live, never deferred; the lambda-value sites keep `!fd.Anonymous`), and `for 2 [do [[i]]]` lowers to `PUSH_CLOSURE do$body … PUSH_LOCAL; MAKE_LIST`. NUR202, NUR201's loop twin: a `for` loop abandoned by a raise the caller traps left its ITERATOR installed on the interpreter — `def i 9  do [for 2 [raise 'x']]  i` read 0, `for 3 [if (i eq 1) [raise 'x'] []]` under the same trap read 1 — where the compiled loop keeps `i` in a frame slot and reads 9. `Engine.faultReturn` unwinds every live loop's iterator (`unwindLiveLoops`: a `for` continuation whose mark has stepped and whose move the pointer has not reached, as handleLoopBreak uninstalls it) BEFORE the frames: a loop inside a live frame installed its iterator after the frame's entry snapshot, so the frame's truncation has nothing left to pop for the name, and a loop enclosing a live frame keeps its iterator beneath that snapshot, where only the loop walk reaches it. Both lanes read 9; with no outer binding both raise undefined_word (the compiled lane at the check pass); a while loop installs no iterator and its body's def leaks by design on both lanes.

**Measured:** control.tsv, accessor.tsv, fn-locals-scope.tsv and the module families under every langspec gate at their ceilings; the lang unit ledger 285 / 44 unchanged; core, eng, compiler, check, basic, the lang root package, lang/go/test and the modules package green.

**Pins.** lang `TestLoopBodyResidualLiteralResolves` (the shapes and their neighbours as parity rows — the bare and nested literal, the outer binding, the while body, a `do` and a fn around the loop, the do-body twin's shapes and its lowering: a closure assembling the list, no `word(i)` const) and `TestLoopIteratorTornDownOnTrappedRaise` (the iterator after the trap: nested loops, the loop inside and around a frame, a callback raising inside, a range loop's own name, the `error` handler, the while twin), both in map_literal_flex_member_test.go; control.tsv §3's five rows and §7's one. Docs: NUR.md (NUR197 FIXED, NUR208 — NUR251 since the merge of main's #510 — and NUR209 recorded and FIXED), the handover, this entry.

## NUR201 closed — the frame's error path (2026-09-25)

**The divergence.** Recorded while closing NUR199: `def t 0  def g fn
[[][Integer][def t 9 raise 'x']]  do [g]  t` was the interpreter's
`[error(x) 9]` and the compiled lane's `[error(x) 0]`, silently, and so
was the param twin `def g fn [[t:Integer][Integer][raise 'x']]  do [g 9]
t`. The interpreter's raise abandoned the sub-engine's tape with the
callee's frame still open on it — its `__DC __pa undef…` tail unstepped —
so the callee's body-local def, its params and captures, and its per-call
Args list and FnBaseline all stayed on the registry when `do` trapped
the error and resumed. The VM's frame unwinds with the error
(`vmContext.run`'s deferred unwind), and CallBoru's sub-run — the fn-VALUE
seam's frame — tears its own frame down inline whether or not the body
erred; only the SPLICED frame, the interpreter's ordinary fn dispatch,
leaked. The record's verdict pointed at the interpreter, and that is where
the fix is.

**The mechanism.** `Engine.faultReturn` is the one exit every run-loop
error takes (the trace's pause-before-unwind hook); it now tears down
every fn frame the error leaves OPEN on the tape — `unwindLiveFrames(0,
len)`, the same walk a break/continue discarding a region takes: a frame
whose open paren has stepped and whose matching close is at or past the
pointer, unwound innermost first through `unwindFrameTail` (the `__DC`
truncation, the `__pa` Args/FnBaseline pop, the force-forward undef pairs;
the ReturnCheck skipped, an aborted body having nothing to check). Every
frame beneath the raise goes with it — `do [h]` over an `h` that calls
`g` restores the root's `t`, not h's 7 — because the error abandons the
whole tape, not one frame of it. The residual-eval error at the
DefCleanup marker (a computing body's pending container raising —
`{a:(raise 'x')}` as the body's last token) used to replay its tail AT
the marker (`unwindFrameTailOnError`, PR #260); that frame is one of the
live frames the fault return now unwinds, so the marker-site replay is
retired — kept, it would run the tail twice, and the second run pops the
CALLER's args entry and its same-named binding. `TestRunErrorUnwindsFrameOnceAfterResidualError`
pins the once.

**What does not change.** A `do` body's own def under the same raise
still leaks on both lanes (`def t 0  do [def t 5 raise 'x']  t` is 5,
NUR199's row): a `do` body is not a frame, and its sub-engine's tape has
no frame open when the raise abandons it. The top-level program error
unwinds too, which the REPL sees: a raise inside a fn no longer leaves
the callee's locals bound for the next line.

**Measured:** the lang unit ledger 285 / 44 unchanged (the pinned rows
moved from a pending pin to parity rows, none declines or bails); the
unit suites of core, eng, compiler, check, basic and the lang root
package green; the commit gate's smoke corpus and fn-locals-scope.tsv
at their ceilings with no row moving — the corpus carried no trapped
raise inside a frame with a def before it, which is why the divergence
was recorded from a probe and not from a row.

**Pins.** lang `TestCalleeDefTornDownOnTrappedRaise` (keep_defs_leak_test.go:
the shape and its neighbours — the param twin, nested frames, the raise
in a paren group, a callback body, a residual container and a native
error, a lambda value and an applied fn value, the trap inside a fn frame
and a loop, an `error` handler — as parity rows, and the interpreter's
own answers: the callee's local, param and args list gone, the frame
beneath the raise reading its own); core `TestRunErrorUnwindsLiveFrame`
and `TestRunErrorUnwindsFrameOnceAfterResidualError`
(fn_frame_unwind_test.go, over a hand-built frame beneath a caller
binding of the param's name — replayed, and replayed once);
fn-locals-scope.tsv §12's three new rows; lang/go/test's
`TestResidualErrorTearsDownFrame` unchanged (the residual-eval twin, now
served by the fault return). Docs: NUR.md (NUR201 FIXED), the handover,
this entry.
