# Full compilation — the total lowering design

**Status:** the programme of record, in implementation. **Recorded:**
2026-08-25 as a proposal; adopted and under way since 2026-08-26. Stage 2 is
landed; Stages 3 and 4 are partly landed (Stage 4's bind twins reached their
end state on 2026-09-02 — rollback-and-replay is the only regime); Stages 5
to 9 are not started beyond instruments and worklist measurements. §10's
stage table carries the per-stage detail and is the authority on what has
landed; the running state-of-play is
[FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md).
**Provenance:** the directive that closes the question
`design/COMPILE-DECLARATION-MODEL.0.md` left open: interpreter islands are
not acceptable, the interpreter is not an escape hatch, failure to compile
is a hard error, and compiled code must behave exactly as interpreted code
with the checker aligned with both. This note designs the compiler that
satisfies those four sentences.

> Authority: this note is the DESIGN, not the record of what is built —
> §10's stage table and the handoff are that.
> `design/COMPILABLE-SUBSET.md` remains the
> statement of the current subset; `design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md`
> remains the record of the doctrine this note extends. Where this note and
> the code disagree, the code wins. Code citations were verified against the
> tree at the recording date and drift as the tree does.

---

## 0. The claim, in one paragraph

The compiler becomes **total** by changing what its worst verdict is. Today
the recorder's worst verdict is *refuse* — `MarkUncompilable`, a one-way
latch (`compiler/go/emit.go:1366`), after which the interpreter owns the
whole program. Under this design the worst verdict is *lower generically*:
every dispatch the checker cannot resolve statically compiles into opcodes
that make the interpreter's own decisions at runtime — collection, signature
matching, application, name resolution — by calling the same kernel routines
the interpreter calls, over runtime values instead of tape tokens. Where the
checker has proof, today's typed lowering is kept unchanged. Nothing ever
re-enters `Engine.Run`; nothing ever re-steps a token; the only executors
are the VM and the Go handlers it calls. Parity is not an aspiration bolted
on afterwards: the generic lane is *defined* as "the interpreter's decision
procedure, compiled", sharing its code, so the two lanes can only diverge
where they fail to share — and the differential gates police exactly that
seam. This is the architecture Factor shipped when it retired its
interpreter, the one Chez Scheme and SBCL and Julia embody, and the one the
partial-evaluation literature says is forced: for a language with dynamic
semantics, the worst binding-time verdict is *dynamic*, and the answer to
dynamic is **residual code, never refusal**.

---

## 1. Terms of the mission — precise definitions

The four sentences of the directive, made checkable:

- **T1 — Totality.** For every source program the interpreter accepts,
  `CompileCheck` returns a `*Program`. `compile_refused` (Stage J,
  `lang/go/boru.go:1079-1099`) becomes an internal invariant violation, not
  a result. The `BORU_COMPILE_FALLBACK` hatch retires at the end state.
- **T2 — No islands.** At runtime, a compiled program never re-enters the
  interpreter's stepping machinery over tokens — neither recorded *source*
  tokens (`OpFallback`, `eng/go/vm.go:1166-1199`; drift-window
  `OpCallDynamicMixed`, `compiler/go/drift_window.go:31`) nor runtime
  *value* windows (`OpCallDynFrame`/RetReplay, `eng/go/vm.go:1071`;
  the `callDynamic` non-closure arms, `eng/go/vm.go:702-715`;
  raw-token bodies through `RunResolved`), and never bails to a
  whole-program interpreter re-run (`vmDefer`, `eng/go/vm_defer.go:16`).
  What remains allowed is what the runtime-independence doctrine already
  blesses (§3): kernel routines over data — matching, application, typed
  bind, name lookup — and **runtime compilation** producing Programs the VM
  runs.
- **T3 — Behavioral parity.** For every program, the compiled run and the
  interpreter produce the same observables: residual values, effects and
  their order, error identity — code, message text, blame position, and
  *which* of two possible raise texts (§6.2). The one standing exception
  stays the step-budget metering divergence (`design/COMPILABLE-SUBSET.md`
  §7), which needs a ruling either way (§11, O2).
- **T4 — Checker alignment.** The checker never blocks compilation (its
  worst answer is a dynamic carrier plus advisory diagnostics), its
  diagnostics are identical whether or not a program will be compiled, and
  every fact it feeds the typed lane is sound. Statically-definite errors
  compile into code that raises the identical error at the identical moment
  (the `OpTrap` discipline), so "the program is wrong" is expressed by the
  program raising, not by the toolchain refusing.

Non-goals, explicitly: no language-semantics changes in service of
compilation (Factor changed its language to get there — mandatory effect
declarations, `call(` — boru does not; §4.1); no serialized-artifact story
(blocked independently — a Program pins sub-registries by reference,
`design/STAGE3-INLINING-DESIGN-ROUND.0.md`); no speculative typed lowering
that would need deoptimization (§6.9).

---

## 2. Where the system actually is — the measured gap

Three measurements define the distance to T1/T2. The first two are known;
the third is this note's correction to the frame.

**2.1 The ledgered frontier is 153 rows, 127 refusing.** The
`lang/spec/frontier/*.tsv` ledger pins 127 interpreter-works/compiler-refuses
rows across 38 `failsWith` reasons plus 26 must-compile witnesses
(`test/go/langspec/frontier_spec_test.go:700-738`; counts measured from the
`frontierCompileLedger` map itself). Grouped by missing capability (family
letters used throughout this note; the A–D/I split below carries one fewer
row than the ledger — a one-row re-audit of that pool is owed):

| Family | Rows | Essence |
|---|---:|---|
| A. Computed-fn / closure provenance | 45 | fn values whose shape the checker cannot know: Church/SKI/CPS chains, `FnUtil` results, computed closures at argument slots, curried chains |
| B. Fn value as data operand (Stage 3) | 22 → 9 | `is`/`eq`/`deq`/`canon`/`for-each` receiving a fn value the gate assumes would be re-stepped. **The `for-each` row graduated 2026-09-10** (the forty-fourth increment) and its cause was the same shape as family G's `each` rows one word over: `for-each` declared no `CallableSpec` at all, so its body never compiled to a closure — and a word whose body does not compile cannot admit a fn-valued operand, because the gate's premise (the handler re-steps it) is then true. Giving it each's spec minus the three flags its own handler does not earn (BodyOut 0, no `EmptyBodyErrors`, no `CrossCollectionTokenShape`) plus the missing `lambdaCallbackInputs` case compiles both forms native, and takes every for-each BODY off the interpreter with them. **12 graduated 2026-08-27**: `eq`/`neq`/`deq`/`canon` only READ one, and had merely never declared `CompileReadsFn` — no representational change was needed (§6.3). The 8 `is` rows COMPILE if the gate is relaxed and, until 2026-08-28, still must not have been: measured, the predicate body ran on the interpreter inside the handler, so admitting them would have hidden an island rather than removed one. **The body now runs on the VM** (§6.3, RESOLVED — the cause was a cross-program decline in `runUnitNested`, not the representation), and **all 8 GRADUATED 2026-08-28** into `lang/spec/fnpred.tsv` §7 with two shapes the ledger never carried. Family B is closed |
| C. Multi-dynamic-result residual | 11 | two dynamic results live at once; the static seat model cannot address them |
| D. Provenance totalization | 13 | operands with no producing event: `$module`/namespace synthetics, cross-registry captures |
| E. Check-diagnostics sentinel | 8 → 6 | the checker rejects programs the interpreter runs (`lang/go/boru.go:459-472`). **Two graduated 2026-09-10** (the forty-ninth increment), and the interpreter did not run them either — `FnUtil.flip 5` / `FnUtil.curry 5` raise `uncalled_function` on BOTH lanes, so the error IS the program's specified result. `execFnDefLiteral`'s arm refused the whole program on the grounds that "there is no call here to compile": true, and beside the point, because what a RuntimeMirror needs is something that RAISES IDENTICALLY, not a call. A DEFINITE failure on the uncaught top line — every operand a plain concrete const the runtime match examines unchanged — now bakes the interpreter's own error into a terminal `OpTrap` and the diagnostic becomes a mirror the pipeline compiles past. The screen is deliberately narrower than the word-dispatch trap's: there is no rematch op for a fn VALUE reached as a call, so it only has to be sound |
| F. Dispatch recovery | 5 → 3 | `unmatched dispatch recovered at apply/…` — recovered windows have no trap lowering yet. **Two graduated 2026-09-10** (the forty-sixth increment), and not by giving them a trap: their window holds a REACH, and a deferred expression EXPANDS at dispatch time, so no STATIC trap may be baked over the unexpanded token. That is an argument against the trap and not against the runtime REMATCH, which reads no static tag at all — it re-runs the match over the values the interpreter's own dispatch examines. A deferred window now routes there instead of declining outright: it defers where the expansion matches (the flex witnesses) and raises byte-identically where it does not. The line it keeps is structural — a window carrying sigError's REORDER HINT is derived from tape state the runtime rebuild cannot reproduce, so it still declines |
| G. Working islands | 7 → 5 → 7 | compile and run correctly; ledgered only because the program embeds `OpFallback`. **2 graduated 2026-08-27** (list `each`, Function and lambda forms): `lambdaCallbackInputs` had a MAP case and no LIST case, so the lambda lowering declined at its first gate — a missing case, not the "modelled fn-value callback frame" the ledger asked for. **5 more graduated 2026-08-28** (list `fold`/`scan`, Function and lambda, seeded and unseeded): they needed no frame either — both handlers hand `(accumulator, element)`, and the two engines disagree only because the MAP path binds POSITIONALLY (`CallBoruFn`) while the LIST path runs its inputs as a STACK (`InvokeBody` → `RunResolved`), where `MatchSignature` fills top-down. One per-word permutation at the closure bind (`ClosureInStackPair`) reconciles them (§6.3). The family is down to the `apply` island and the full-stack-word row. Separately, the full-stack sub-frontier's PERMUTING row (`(1 add 2) (3 add 4) 1 roll`, ledgered under "residual shape beyond Stage 1") graduated 2026-09-10 (the forty-third increment): the program residual now REBUILDS when its order is not the order the events produced it in — every simulated-stack entry spills to a frame local and the residual is pushed back exactly as recorded, the program-residual twin of the call-site `spillSeat`. **Two JOINED 2026-09-10** and it is a promotion, not a regression: the forty-eighth increment's two maybe-raising `do … error` rows stopped REFUSING and now compile — with the island, because the region representation it found IS the island's own simulated slot. They were moved to the main corpus and moved back by the batch gate (island ceiling 0), and are ledgered "islanded" here, where the remaining step is nameable: seat the region without re-entering the interpreter | **The full-stack code-body row GRADUATED 2026-09-11** (the fifty-second increment): `FoldFullStack`'s exactness condition is per-UNIT, and the "top unit" half of its gate was inherited from where the machinery was first needed rather than from the argument. A compiled body unit has its own stack discipline exactly as the top unit does — `[10 20] each [1 2 3 depth]` is 4 on both lanes, the element in the body's stack and the collection not — so the gate now asks for the CURRENT unit's root frame. The class SPLIT rather than graduating whole, and the variation differential is what said so: the first cut turned four working islands into REFUSALS, because a body unit has no residual REBUILD (the top unit's seatResidualRebuild re-pushes a permutation; a body unit's seating refuses a result above a literal). A body unit therefore folds only over entries needing no rebuild — consts and locals. The event-produced occurrence stays a working island and keeps the family's count; graduation of the remainder = the residual rebuild inside a body unit. The frame half stays load-bearing: an open FRAGMENT holds events no scope's residual has reconciled. NUR131's callable screen is per-entry and unaffected |
| H. `while` | 6 → 0 | no structured lowering; body words splice tape-coupled tokens. **Four graduated 2026-09-09** (the thirty-seventh increment): a while records on the COUNTED loop's own frame — an unbounded `FOR_SETUP`/`FOR_NEXT`, the condition fragment lowered at the head of every iteration, a falsy value leaving through `FLOW_BREAK`. **The fifth graduated 2026-09-10** (the forty-second increment): a STATICALLY EMPTY condition (`while [] [1]`) is not an approximation to stand aside for — the region holds no tokens, so the interpreter's first round provably nets nothing and raises before the body runs once, and the compiled program raises the byte-identical error through a terminal `OpTrap`. **The LAST graduated 2026-09-10** (the fifty-first increment) and `frontier-while.tsv` is deleted: its refusal was not the loop's — it was the body's `if` — and it was two gates, not one. The computed-arm lowering said DIVERGES and tested "has no out", which a 0-netting arm and a diverging one both fail; only the first is a problem, because a diverging arm never reaches the merge at all, so the single slot is exact for every path that ARRIVES. Behind it, the layout gate assumed the eager arm is on TOP — true for the WRITTEN spelling `if c [t] (expr)`, false for an arm filled from the VALUE STACK, which was there before the `if` and so sits UNDER its own condition, owing no swap. **The family is CLOSED** |
| I. Variadic no-static-seat | 5 → 0 | `await first/any` winner residuals: 0..N results exceed any static seat (NUR067). **The REPRESENTATION graduated 2026-09-10** and it did not need §6.6's generalized mark region: a value-producing LOOP's region is already ONE simulated slot standing for a runtime count, and the VM's stack ops are already count-agnostic, so the recorder now turns await's variadic-spread residual into that same region (`callVariadicRegion` → `eventFlags.variadicRegion`, `lw.variadic` at lowering). The plain-residual row compiles. The two CONSUMING rows graduated in the same batch, with their `for` twins, on two closing ops: **OpSeatBelowMark** (a mark opened before the region's event, an inert prefix lifted down to it) and **OpMakeListToMark** (the run collected into one List, element count taken from the mark). **The family is CLOSED** — `frontier-await-winner.tsv` is deleted. One EDGE remains, narrower than the refusal it replaced: a region whose run may carry a CALLABLE is not consumable, because only the interpreter re-steps one (`await {mode:'first'} [[5 g/v]]` is 6 interpreted, `[5 fn]` as a region). The bare loop residual has the same divergence and had it before this work — NUR129 |
| J. Pinned miscompiles | 2 | bare `Function`-param read — both lanes wrong vs the 2026-08-15 ruling |
| K/L. Context layer; conditional fn shadow | 2 | `context` needs a frame the inline stream lacks; `installDef` overlap-removal defeats rollback |

**2.2 The ledger is a sample, not the universe.** The recorder holds ~96
`MarkUncompilable` sites plus lowering declines. Measured at the Stage-1
baseline (2026-08-25): **96 recorder call sites** (compiler/go 64, check/go
11, basic/go 10, core/go 7, lang/go 2, plus 2 test fixtures) and **78
lowerer/`Finalize` declines**, normalizing to **161 distinct reason
templates** — a third more than this note first estimated, and a majority
of them built at run time from a word name rather than written as literals.
The frontier ledger pins 38 `failsWith` substrings, of which only 33 are
source reason strings at all (the other five are harness verdicts) — so the
ledger exercises about a fifth of the surface. Whole gate
families have zero ledgered rows: `args`/`__pa` context words, anonymous
dispatch, quoted-operand words (`usurp`/force-arity/ref), `dynamic input` /
`unannotated or opaque word` (`compiler/go/emit.go:4880/:4889`),
`for:` multi-value bodies (`emit.go:4101`), mid-body dynamic apply
(`emit.go:3481`), multi-arg chained apply, splice/interp-string/XML
runtime-computed parts, the `mini` hooks, the typed-def construction family
(`basic/go/native_definition.go:801-1732`). And the recorder is not the
only latch: whole-program refusal also latches in `CompileCheck` itself —
`SuppressedRuntimeError` (a word deliberately lenient in check mode whose
runtime raise the pass could not model: an orphan `gen`, an `unpack` of a
missing key — `lang/go/boru.go:474-482`), `AmbiguousGradualSplit` (set
inside `Engine.MatchSignature`, `core/go/engine.go:8393-8413`; latched at
`boru.go:483-491`), and `Finalize`'s own post-recording declines
(`boru.go:503-506`; §5). Totality must be proven against the **whole gate
inventory**, with generated differential sweeps as the oracle (the
690-program sweep that found 24 divergences the ~30 hand-picked rows
missed — `design/HIGHER-ORDER-FUNCTIONS.0.md` §9g), not against the 153
rows.

**2.3 "Islands are at zero" is true only of `OpFallback`.** The live system
still re-enters the interpreter at runtime on the compiled path:

- `OpCallDynFrame` + RetReplay re-steps a runtime **value window** through a
  pooled Engine so `execFnDefLiteral`'s landing rule decides
  (`eng/go/vm.go:1071, 2326`) — sanctioned as "not an island" because the
  window holds values, not source, but it is `Engine.Run` mid-program.
- The drift window (`OpCallDynamicMixed`) steps a window **containing the
  word token itself** live, with interpreter forward collection
  (`compiler/go/drift_window.go:31`) — literal source re-stepping.
- The `callDynamic` family's non-closure arms island `[fn, args…]` windows
  (`eng/go/vm.go:702-715`); `OpCallDynamicTrailing` is hard-bounded to
  arity 1 (`vm.go:720`) precisely because the general case has no compiled
  form.
- Raw-token code bodies fall through `InvokeBody` to `RunResolved`
  (`core/go/invoke.go:21`; `eng/go/vm_defer.go:43`).
- Predicate and membership types run boru code **inside signature matching
  itself**: `v.Is(t)` on a predicate type routes through `PredicateUnifier`
  (which holds a `*Registry` — `core/go/unify_predicate.go:20-26`) to
  `Registry.RunPredicate` (`core/go/registry.go:1917`) → `CallBoru`, a
  sub-engine tape run for any unstamped predicate body — reachable from
  every "kernel" matching site, the VM's included.

  **CONFIRMED BY MEASUREMENT 2026-08-27**, not just by reading: the
  `InterpEntry` hook on a compiled `def Pos fnpred n:Integer [n gt 0]
  def f fn [[x:Pos] [Integer] [x]]  f 5` records
  `InvokeCallback:callboru` + `CallBoru`, both UNATTRIBUTED, with no `is`
  and no ledger row anywhere in the program. `RunPredicate` does reach for
  the VM first (`InvokeCallbackFn`: "the VM when the body compiled to a
  unit, `CallBoru` otherwise") — the body is simply never a unit, so the
  reach always falls through. This is the concrete shape of the residue,
  and §6.3 records why it also blocks family B's 8 `is` rows from a
  graduation that would otherwise look clean.
- A callback arriving while the main registry is mid-run routes to the
  interpreter by *structure*, not analysis: `CanHostVM` declines
  (`core/go/registry.go:677-686`) and `InvokeCallback` falls to `CallBoru`
  (`core/go/invoke.go:49-52`).
- ~15 named `vmDefer` sites resolve runtime surprises by **re-running the
  whole program on the interpreter**, fenced by the C1 effect fence
  (`lang/go/boru.go:1264`) — dyn-scope misses, poly no-match/NOut drift,
  user-poly table drift, rematch-matched, shaped methods, splice-active,
  dyn-frame count mismatches.

T2 therefore has a second, unledgered debt: the **live island residue** and
the **defer valve**. This note treats both as first-class targets with their
own ratchets (§9), because a total compiler whose runtime quietly re-runs
programs on the interpreter has not left the status quo.

**THE RESIDUE IS NOW MEASURED, 2026-08-28, and it was never zero.**
`test/go/langspec`'s `TestInterpEntryCensus` runs EVERY corpus row compiled
with Stage 1's `InterpEntry` hook armed and counts the entries that are
UNATTRIBUTED — interpreter execution the end-state invariant does not permit:

```
7603 rows, 7180 ran compiled, 184 with unattributed interpreter entries
  Engine.Run              501      InvokeCallback:callboru   28
  CallBoru                275      vm:island-resolved        21
  vm:island                66      RunResolved               31
  runPooledSub             37                    959 entries total
```

**163 as of the same day**, after §6.3's predicate finding was RUN rather
than reasoned about (`eng/go/vm_foreign_unit.go`): `InvokeCallback:callboru`
28 → 4, and `Engine.Run` / `CallBoru` each −24, because one declined
predicate dragged all three seams behind it. The seam attribution below is
otherwise unchanged, and its first row is now nearly retired.

**Read that against `TestCompiledCoverage`'s `0 islanded`.** Both numbers are
correct and they measure different things. The island ceiling counts programs
whose DISASSEMBLY embeds an `OpFallback` span; it cannot see a `CallBoru` made
inside a native handler, because no opcode records one — and that is precisely
where interpretation survives (§6.3's predicate finding: every predicate
dispatch takes `InvokeCallback:callboru` with no ledger row and no island
flag). So **"0 islanded" is a claim about the metric, and 184 is the claim
about the runtime.**

This matters for the stage plan, not just for bookkeeping. **T2 is not
satisfiable while this number is non-zero, whatever the `OpFallback` ceiling
says**, so Stage 9 cannot honestly flip to total on the island ceiling alone.
The census is a DOWNWARD ratchet like `refusalSiteCeiling` — it only falls,
and a rise wants a design note rather than a bigger constant.

The seam spread is also the work-list, and it is not one problem. Sampling the
rows that produce each seam attributes the 959 entries to named stages rather
than leaving them a number:

| seam | what produces it (sampled) | retires with |
| --- | --- | --- |
| `InvokeCallback:callboru` | **predicate types**, every sampled row — `def Pos (fn [[n:Integer] [Boolean] [n gt 0]])` at a param, a typed def, or a return. **28 → 4: RETIRED for predicates 2026-08-28** (§6.3); the 4 that remain are a different, unsampled population | §6.3 predicate bodies as units |
| `CallBoru` | the same predicate rows, plus module fn bodies that `raise`, plus `Test.check-prop`. **275 → 251** with the predicate half gone | §6.3, then §6.8 |
| `RunResolved` | `do` with a RAW-TOKEN body — `do [1 2 (if b [] [9 9])]`, `do [for 3 [1]]` | §6.8 units-not-tokens |
| `Engine.Run` | the Reach/lens apply (`p $.name apply`, `[10 20 30] $.1 apply`) and `do` with a MAP body whose entries are code (`do {n:[a add 1]}`) | §6.8, plus a lens-apply lowering |
| `runPooledSub` | the same lens-apply rows, and `Vm.run` / `canon` round-trips | §6.8 / §6.7 (runtime compilation) |
| `vm:island-resolved` | the fn-value apply chains — `compose`/`twice`/`stage`, `fnsig`-typed params, module fn-value boundary rows | §6.3 universal fn values |
| `vm:island` | OpFallback spans that DO execute: a fn value read from a container (`m.double 21`), `do`/`error` rows, `StructUtil.parse/v` | §6.3 + §6.10 |

Two readings worth stating. First, **§6.3's predicate work is the single
largest attributable cluster** — it owns `InvokeCallback:callboru` outright and
much of `CallBoru` — which is the same conclusion the family-B `is` refusal
reached from the opposite direction, and it is why that refusal must stand
until the bodies compile. That cluster is now retired, and the two independent
measurements that converged on it are the reason it was found: the seam
attribution said "predicates own this seam", the family-B refusal said "the
predicate body is not a unit", and reading the seam they BOTH blamed turned up
a four-line decline in the nested runner rather than the representation work
both had scheduled (§6.3, RESOLVED). Second, **`Engine.Run` and `RunResolved` together are
the `do`/code-body family**, so Stage 6's handler migration is a bigger
contributor to the live residue than its ledger row (H, 6 rows) suggests: the
ledger counts rows that REFUSE, and this counts rows that COMPILE and then
interpret anyway.

---

## 3. Why the two-lane shape is forced, and why it is already legal

**3.1 Refusal is not an available verdict.** Rice's theorem (Rice 1953,
*Trans. AMS* 74(2):358–366) makes every non-trivial semantic property of
boru programs undecidable — "this lead is always the same fn", "this slot
never sees a fn value", "this body's result count is fixed". A compiler
that must accept every program therefore cannot demand static resolution
everywhere. Partial-evaluation theory says what it must do instead: a
compiler is the interpreter specialized to the program (Futamura 1971,
*Systems, Computers, Controls* 2(5):45–50), and binding-time analysis is
**congruent** — its worst verdict is *dynamic*, whose answer is **residual
code that makes the decision at runtime**, never rejection (Jones, Gomard
& Sestoft, *Partial Evaluation and Automatic Program Generation*, 1993).
boru's recorder is a hand-written first Futamura projection whose dynamic
verdict currently aborts. The fix is not more static power — control-flow
analysis for higher-order programs is EXPTIME-complete at k≥1 and still
incomplete (Van Horn & Mairson, ICFP 2008) — the fix is giving the dynamic
verdict its residual code. That is the whole design.

**3.2 The doctrine already blesses the runtime kernel.** The
runtime-independence invariant C4 bans *interpreter execution*, not runtime
decisions: "no interpreter execution of any program the compiler accepts,
on any default path" with enumerated carve-outs
(`design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md:123-135`), and its
stated method is "sound runtime re-dispatch — never static best-guess
baking" (`:16-19`). Kernel `MatchSignature` at VM time is shipped doctrine
(`OpCallNativePoly`, `eng/go/vm.go:485-590`; `OpCallUserPoly`, `:604`;
`OpDispatchRematch` re-matching "over the word's LIVE registry binding");
runtime name resolution is shipped (`OpLookupDynScope`/`OpBindDynScope`);
runtime **compilation** is shipped (`StampDetachedSig`,
`compiler/go/stamp_runtime.go:60`; the bounded JIT restamp, `:192`;
`Vm.run` compiling runtime-supplied source under fork isolation). And
`COMPILE-REFUSAL-SURVEY.0.md` already measured the corollary: after those
opcodes drove islands 102→0, **not one live refusal is a
dispatch-resolution problem**. What refuses today is everything *around*
dispatch: putting operands on the stack (collection), naming their homes
(provenance), typing their results, and applying fn values with the right
application model. §6 gives each of those its mechanism.

Two of the doctrine's own rulings this note must **supersede rather than
inherit**, and says so where it does. C4 records the fail-safe decline
seams ("decline → interpreter, never the reverse") as *permanent by
design* — §6.10 retires them anyway, a deliberate overturn T2 forces, with
a named compiled replacement per seam: the seams were permanent because
their only landing pad was the interpreter, and T2 removes the landing
pad. And C3's R3 defers registry-aware (predicate-faithful) matching in
compiled dispatch to "a separate later design, not assumed"
(`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md:109-117`) — §6.3's
predicate-unit inventory and §6.10's retirement row are that design.

**3.3 The precedents all have this shape.** Full compilation of a dynamic
language without an interpreter escape hatch has been shipped repeatedly,
always with the same three components — a *total baseline lowering*, a
*shared runtime kernel*, and a *resident compiler for runtime-supplied
code*:

- **Factor** is the closest precedent and the only dynamic concatenative
  language to reach total compilation: interpreter removed in 0.91 (2007);
  a non-optimizing compiler that lowers every quotation by a single
  left-to-right pass (totality is structural — every element has a
  template); an optimizing compiler whose refusal is *soft* (falls to the
  base compiler, never to an interpreter); one universal callable protocol
  (`call` generic over quotation/curried/composed — `curry`/`compose` are
  2-cell data, no codegen); eval as the resident compiler ("the parser
  always compiles code into quotations"); redefinition honored by
  compilation units + dependency-indexed repatching (Pestov et al., DLS
  2010, 43–56). Notably, pre-2008 Factor had boru's exact anti-pattern —
  redefinition decompiled dependents back to the interpreter — and the fix
  was making the cheap compiler total.
- **Chez Scheme / SBCL**: compile-only Lisps. `eval` invokes the compiler
  (Chez's `current-eval` is `compile`; SBCL "essentially a compiler-only
  implementation"); top-level redefinition honored from compiled code by
  one indirection — symbol value/fdefn cells (Dybvig, ICFP 2006, §3).
- **Julia**: the checker-gates-tiers model whole: Kleene-iterated inference
  whose ⊤ is `Any`; inference failure emits a *compiled* generic-dispatch
  call (`jl_apply_generic` — the same routine from every tier), never
  refusal, never interpretation (Bezanson et al., OOPSLA 2018). Its
  world-age rule is the caution, not the template: it *changed the
  language's* redefinition visibility to license inlining (Belyakova et
  al., OOPSLA 2020). boru's interpreter sees rebinds at the next dispatch,
  so boru compiles the live def-stack read instead (§6.5).
- **Baseline compilers** are the totality mechanism in the JIT world: a
  translator "derived very simply from an interpreter by having the
  interpreter save its action-routine code in a buffer rather than
  executing it" is total by construction (Deutsch & Schiffman, POPL 1984)
  — the modern forms compile bytecode 1:1 to code that calls shared
  builtins (V8 Sparkplug; JSC Baseline). Measured value of exactly this
  transformation: ~1.5–3× over interpretation (D&S: 0.686 vs 1.0; JSC:
  3.97→1.71 ns/op), bought purely by deleting fetch-decode, doing identical
  semantic work.
- **Self** supplies the missing piece for a no-interpreter world:
  deoptimization lands in *unoptimized compiled* code (Hölzle, Chambers &
  Ungar, PLDI 1992) — proof that no interpreter is needed as a landing pad.
  boru's analogue: every `vmDefer` site retires onto the generic lane
  (§6.10).
- The **counterexamples** transfer nothing: Stalin/MLton achieve totality
  by closed-world bans (no `eval`, no reload) boru cannot adopt;
  LuaJIT/HotSpot keep interpreters for native-code economics (startup,
  memory, deopt simplicity) that mostly vanish for an AOT-to-bytecode VM.

**3.4 Engagement with the prior note.** `COMPILE-DECLARATION-MODEL.0.md`
proposed two things. Its §4.1 declaration triple
(`tapeBound` / `needs` / `env`, constraints C1–C4) is *adopted* here as the
handler-contract vocabulary (§6.8) — the fn-util lesson stands: what a Go
handler does with an operand is an irreducible declaration. Its §4.2
(typed islands — give `OpFallback` a result contract, re-tier
`islandCeiling` into a budget) is **rejected by the directive**, and this
note argues it is also unnecessary: the seven working island rows it aimed
at are ordinary `each`/`fold`/`scan` calls that the universal fn-value lane
(§6.3–6.4) compiles natively, and the island cascade it aimed to soften
disappears when nothing islands. Its §4.3 warning — two agreeing lanes are
a bug detector, and collapsing them loses the gate — is answered in §8:
the interpreter remains the reference oracle in the harness (a sanctioned
carve-out), the lanes share *kernel routines* but not *drivers*, and the
differential corpus moves to generated sweeps, which is where its power
already was.

---

## 4. The architecture: one kernel, two lanes

```
                      ┌────────────────────────────────────────────┐
   source ──parse──►  │  check pass (carrier abstract interpreter)  │
                      │  = compiler front end (comptime; sanctioned)│
                      └──────────────┬─────────────────────────────┘
                                     │ per dispatch event
                     proof available │              no proof
                          ┌──────────▼───────┐   ┌──────▼───────────────┐
                          │   T-lane (typed) │   │  G-lane (generic)    │
                          │  today's lowering│   │  compiled decision   │
                          │  CALL_NATIVE,    │   │  procedure: collect, │
                          │  CALL_USER, folds│   │  match, apply, bind  │
                          └──────────┬───────┘   └──────┬───────────────┘
                                     └───────┬──────────┘
                                             ▼
                          one Program; one VM; one shared kernel
                     (MatchSignature, Apply, Collect, TypedBind, Lookup,
                      DefTable ops, error builders — the same functions
                      the interpreter calls)
```

- **The decision is per event, not per program.** Today one unprovable site
  latches the whole program to the interpreter. Under totality the recorder
  cascade (`recordDispatchOutcome`,
  `compiler/go/compiler_dispatch_record.go:53`) keeps its existing
  preference order — method apply, folds, closures, dyn-body, poly — and
  its terminal arm becomes *emit generic*, not `MarkUncompilable`. A
  program is a mosaic of T-lane and G-lane events; a G-lane event's
  successors return to the T-lane the moment the checker has proof again
  (guard narrowing already re-enters typed knowledge mid-block — the
  occurrence-typing pattern, Tobin-Hochstadt & Felleisen, ICFP 2010).
- **The lanes share kernel code literally, not by re-implementation.** The
  repo has already paid for this lesson twice: the `applyHandler`
  mark-not-call episode (a second dispatch path produced three lane
  divergences; the fix was removing the fork) and the stage-3 rule "a
  dispatch seam may not have two recording paths"
  (`design/STAGE3-INLINING-DESIGN-ROUND.0.md:162-164`). A generic opcode
  that *re-implements* matching or application would reintroduce drift one
  level down. The obligation is structural: G-lane opcodes call the same
  `core` functions the Engine calls (`MatchSignature`, `signature.go:175`;
  `SigTypeMatches` incl. the dynamic-carrier rule, `:297-341`;
  `InvokeCallbackFn`; `RunTypedBind`; `Registry.Lookup` +
  `aggregateDispatch`; the `RuntimeNoMatch` diagnostic builders), and the
  Engine is refactored to call the newly extracted routines (§6.2) so
  there is exactly one implementation of every decision.
- **No speculation, no deopt — which demands a sharper eligibility rule
  than "checker-proven".** A fact proven against a *mutable binding* is a
  bet under §6.5's live-rebinding semantics: the recorded `NOut` and Impl
  identity are claims a rebind can falsify (`eng/go/vm.go:575-585`,
  `:646-650`), and no site-local replacement can repair the
  **continuation** — the downstream T-lane code laid out for the stale
  result count — in an in-flight frame: a restamp serves *future* entries
  only, and a site-local re-dispatch executes the new arm but leaves the
  stale layout. So T-lane eligibility is *binding-stable* proof: facts
  rooted in sealed natives, module-sealed words, unit-locals, and
  structure the checker can prove no reachable rebind touches (the
  soundness classification, §6.9). A binding-*sensitive* dispatch — and
  the width-consuming region downstream of it — lowers to the
  G-lane/mark-region form, which is count-generic by construction and
  therefore valid across any rebind. Runtime staleness guards then never
  need an interpreter landing pad: future entries recompile (§6.7's
  restamp policy); in-flight frames were never speculated on. This is the
  Self stance minus speculation: the generic lane is the landing pad, and
  it is compiled code.

The formal frame for T4 (§6.9): the checker stays an *abstracted
interpreter* over carriers — the pattern of Van Horn & Might, "Abstracting
Abstract Machines" (ICFP 2010) — with ⊤ = dynamic carrier always a sound
answer (Cousot & Cousot, POPL 1977), so checker totality is free; typing is
**erased** (zero runtime footprint; Bracha, OOPSLA 2004 workshop), and lane
boundaries are Matthews–Findler *lump* boundaries — representation-
identical, conversion-free, check-free (POPL 2007). Every published cast
discipline (natural/transient/concrete) observably diverges from an erased
reference semantics (Chung et al., ECOOP 2018; Vitousek et al., POPL 2017),
so erasure is not a choice among equals: it is the only point compatible
with T3. What gradual-typing performance folklore indicts is guarded casts
(Takikawa et al., POPL 2016), which this design never introduces; the cost
model is Julia's — dynamic-dispatch cost on unproven sites, recovered by
caching (§7).

---

## 5. The total lowering algorithm (recorder side)

The per-event decision procedure that replaces refusal. `E` is the dispatch
event the check pass just resolved (or failed to resolve) over carriers.

```
LowerEvent(E):
  1  if E is a check-mode word (def/import/type/macro/Test):
       run at compile time as today (front-end carve-out, §3.2);
       emit binding TWINS for anything the runtime must re-see (§6.5):
       def → OpBindDef, undef → OpUndef, so VM-time def order is real.
  2  if E folds (const/scalar/module/full-stack under exactness proofs):
       fold as today (folds still record events — anchors stay).
  3  if E is statically DEFINITE-ERROR (checker proves the raise):
       emit OpTrap carrying the interpreter's full structured diagnostic
       (existing discipline: TryRecordUnmatchedDispatchTrap, PolyNoMatchSpec).
  4  if every operand resolves and the checker proved ONE signature:
       T-lane: CALL_NATIVE / CALL_USER / closure unit, as today.
  5  if operands resolve but the signature choice is runtime-only:
       T-lane poly: OpCallNativePoly / OpCallUserPoly, as today —
       EXTENDED to variable pop-windows via the region descriptor (§6.2),
       closing the smaller-arity-overload unsoundness
       (compiler_dispatch_record.go:544).
  6  otherwise — the former MarkUncompilable arm:
       G-lane: emit the statement's compiled collection form (§6.2):
         PUSH operand descriptors in written order
         (literals as consts; paren groups as compiled sub-fragments;
          word references as name tokens; fn values as universal closures)
         then OpCollect+OpDispatchGeneric / OpApply per §6.2–6.4.
       Refusal is no longer reachable from this arm.
```

Structural constraints inherited from the recorder and kept: one recording
path per seam (strictly additive probes only); the probe-then-real closure
compile pattern and its drift hazards; `Rollback` cannot unwind compiled fn
units (`emit.go:3269`) — G-lane emission must be as latch-free as T-lane
recording. Step 6 is total by construction (every token class has a
descriptor form — the Deutsch–Schiffman argument).

**The same rule must bind the lowering phase, or the procedure is not
total.** Recording is only half the pipeline: `Finalize` and the lowerer
decline today *after* recording succeeded (`emit.go:7545-7592` —
`resolveDynamicApply`, residual operand resolution, mark-window
verification, `seatResults`; ~31 decline strings in `lower.go`, e.g.
`spillSeat`/`layoutOperands` at `:386`/`:1512` and the fragment-depth
ceiling at `:2041`; `stamp_runtime.go:139-153` records the
clean-probe-then-Finalize-declines class as production-reachable). And two
latch classes are not dispatch events at all: the fn-literal capture latch
during body recording (`emit.go:1830`) and the *retroactive* stored-handler
invalidation, where a later event invalidates an earlier bake
(`emit.go:2460`). Under totality every one of these becomes a **demotion,
never a refusal**: the recorder keeps a region descriptor for *every*
statement — T-lane ones included — so a statement whose typed lowering
declines at Finalize re-lowers in descriptor (G-lane) form; a retroactive
latch demotes the earlier statement the same way (its descriptor is still
held); typed-def construction shapes take their §6.8 lowering. The
totality claim is therefore a **pipeline** property — "no refusal string
reachable from `Finalize`", not merely "no `MarkUncompilable` in the
recorder" — and §9's census is scoped accordingly.

The rest of §6 specifies the runtime mechanisms step 5–6 rely on, in
dependency order.

---

## 6. The mechanisms

### 6.1 What the interpreter actually decides at runtime — the split

Binding-time analysis of the Engine loop (`core/go/engine.go:1375-1490`,
`stepWord` `:2658`, `Engine.MatchSignature` `:8381`) yields a clean split.
**Static, per token, always:** the token *region* of every statement (its
hard syntactic delimiters — `end`, parens, the statement commit); dispatch
modifiers (`/v`, `/u`, `/q`); sugar/reach/paren lowering; macro expansion.
Barrier geometry and token word-vs-value class are static *per binding
set* — they follow the live signatures, so under rebinding they are
re-derived, not re-recorded (§6.2's revalidation rule).
**Runtime-irreducible:** (1) overload selection over unknown value types —
including predicate params that run boru code inside matching; (2) the
forward/stack **split itself** under dynamic operands (forward drift);
(3) zero/multi-value expression widths; (4) fn-value claims, arities, and
result counts; (5) def/undef and dynamic scope; (6) partial application
(`curryOrStack`'s curry lists, `engine.go:8159`); (7) runtime-supplied
code. The G-lane is exactly the compilation of (1)–(7); everything static
is decided at record time and baked into the descriptors below. This is
the compile-time/run-time line the whole design draws, and it is the
literature's line: the *domain* of every dispatch is static even when the
verdict is not.

### 6.2 The compiled collection machine — region descriptors

The single deepest finding of the code review: signature matching is
already runtime-shared, but **argument collection is not**, and collection
is value-dependent. The pinned §9d case
(`test/go/langspec/frontier_spec_test.go:562-573`): a function-valued
argument reaching a leading collection must RAISE where a trailing-window
model would apply — and which of two raise texts fires is, in the ledger's
own words, "selected by collection state the window does not carry". The
ledger calls it "not repairable at run time". That verdict is about the
*window* model — an arity-N pop with no history. The repair is at compile
time: carry the history.

**What the Stage-2 probe changed here.** Four agents read the collection
machinery against this section (the F1 investigation, 2026-08-25). F1
**holds** — the extraction is possible — but three of this section's
load-bearing claims were wrong, and the corrections change what gets
built:

- **A per-STATEMENT descriptor cannot exist.** A statement's extent is not
  statically bounded, because the statement's own operands can move it: a
  paren group at position 0 that `def`s a *function* word creates a
  forward-collection barrier at position 1 that did not exist when the
  scan began, which then suppresses the pre-evaluation of the group at
  position 2. Which slots a statement owns is an *output* of running that
  statement. The unit is therefore a **region** — everything to the next
  hard delimiter (`end`, `)`, EOF), whose boundaries are syntactic and
  cannot shift — and collection *returns a cursor* saying where it
  actually stopped.
- **There is no single collection algorithm to extract.** Forward
  collection is *three* loops over the same tokens with three different
  stop-condition sets and three different position countings:
  `resolveForwardArgs` (`core/go/engine.go:1876-2232`, once per dispatch,
  over the union of viable barriers — it evaluates), the per-candidate
  scan inside `Engine.MatchSignature` (`:8507-8749`, once per candidate
  signature, over that signature's own `BarrierPos` — it classifies), and
  the arrival loop in `stepLiteral` (`:3843-4162`, once per arriving
  value). `design/FORWARD-COLLECTION-PHASES.10.md` documents two of the
  three; the per-candidate scan is not in that note, and it is the one
  that actually matches. They are not refinements of one another.
- **Planning is not pure, and the named "hard case" was the wrong one.**
  Overload *pruning* is itself an evaluation site: `pruneViable`
  (`:1919-1934`) calls `SigArgMatches`, which for a predicate-typed slot
  runs a boru body through `RunPredicate`. That is already a live
  divergence — **NUR102**, an effectful predicate observed running four
  times interpreted and twice compiled — found by this probe, in a place
  this section never looked. Meanwhile the mid-scan sugar expansion F1
  named as "the hard case" is, in phase 1, a gated syntactic lowering
  that explicitly refuses to fire for the wrong overload; the *unguarded*
  twin inside the per-candidate loop (`:8719-8731`) is the one that
  mutates the tape and survives a `MatchSignature` returning nil.
- **Slot indices are not stable across evaluation.** A zero-value paren
  collapse slides the next token into the evaluated slot; a multi-value
  collapse leaves extras to be re-examined as later positions.

The consequence for the interface: it is wider than an "abstract slot
sequence". What both adapters must implement is a **mutable, spliceable,
live-length token window plus a host evaluator callback** — honestly, a
second tape. That is a real answer to F1, but it is a bigger object than
this section first described, and the VM adapter has to build it rather
than index a frozen array.

**The VM's window needs no adapter — it IS a Tape (measured 2026-08-26).**
The paragraph above concludes that both adapters must implement "a mutable,
spliceable, live-length token window plus a host evaluator callback —
honestly, a second tape", and that the VM "has to build it rather than index
a frozen array". Both hold, and the building is `NewTape`. A `Tape` has no
Engine reference — `buf`, `gapStart`, `gapEnd`, `forwards`, and nothing else
— so one constructed over a slice of runtime values satisfies
`CollectWindow` directly, with no adapter type at all
(`TestStandaloneTapeSatisfiesCollectWindow`). The planned second window
implementation is therefore deleted from this stage's worklist before it was
written.

That is worth more than the code it saves. A re-implementation would have to
AGREE with the original on splice and gap semantics, and
`collect_kernel.go`'s first stated property is that "the window mutation IS
the interface between the phases" — precisely the thing a parallel
implementation is most likely to get subtly wrong, and in a way the
differential would catch only where a test happens to splice. Sharing the
type makes the agreement structural. What the VM adapter still owes is the
`collectHost` half: the EVALUATIONS, which is where its real work was
always going to be.

**Mechanism.** For every G-lane region the recorder emits a **region
descriptor** — a static table entry (a new `Program` side table, peer to
`Dispatches`) recording what the interpreter would have read off the tape,
with the extent as an output rather than an input:

```
RegionDesc {
  lead:      word name | apply | fn-value slot     // what dispatches
  slots:     []SlotDesc                            // written order, to the
                                                   // next HARD delimiter
    SlotDesc { class: const | local | event | group(fragment idx)
               | wordRef(name) | typeNode(id)
               quote: none | /q | /v | /u          // static modifiers
             }
  barrier:   per-candidate-sig BarrierPos geometry
             // static only for a record-time-resolved word lead;
             // derived live for value leads and after rebinding (§6.5)
  seal/mods: dispatch-control facts the tape carried
}
// OpCollect returns (claimed window, parked sig, CURSOR) — the cursor is
// where collection actually stopped, which its own evaluations may have
// moved. Execution resumes there, not at a statically computed next slot.
```

and a matching opcode pair:

- `OpCollect(desc)` — runs the **extracted collection routine** over the
  descriptor: walks the slots in written order exactly as
  `resolveForwardArgs` + the arrival loop walk the tape
  (`engine.go:1876-2232`, `:3992-4062`), evaluating `group` slots by
  *calling their compiled fragments* only where a viable overload consumes
  the position (preserving the interpreter's conditional-evaluation order),
  applying `/q` Word→Atom in place, honoring the strict forward barrier
  (a bare function word acts as a forward-collection barrier —
  `fnWordBarrierAt`, `core/go/engine.go:2234-2242`), and producing either
  a claimed window + parked signature, or the interpreter's own stranded-
  forward / no-match raise — with the same *text selection*, because the
  state that selects the text is the collection machine's register state,
  fed from the descriptor. That state is wider than a pair of scalars:
  `strandedForwardError` (`core/go/engine.go:6878-6916`) draws on the
  forward's identity and blame position, the missing-count arithmetic, the
  paren-scope scan boundary, and a live `barrierReceiverWord` probe that
  can add a third text variant — or *commit* the forward instead
  (`:6825-6866`). Most of that is register state of the one machine,
  which is why the mechanism is "extract the machine", never "enumerate
  the texts". But **not all of it is** — falsifier F2 fired on this
  claim (§11), and the descriptor must additionally carry the
  **enclosing value-stack residual** `reorderCandidates` reads (up to
  four values, which may belong to an EARLIER statement and change the
  candidate notes), the **`voidGroups`** record of paren groups that
  collapsed to zero values (which changes the error *code*, not merely
  its text), the live `barrierReceiverWord` probe's answer, and the
  forward-parens suggestion's suppression condition. Finite and
  carryable, but four items wider than this section first claimed.
- `OpDispatchGeneric(desc)` — completes the statement:
  the word-policy gate first (the same `WordChecker` every named VM
  dispatch already consults — `vmContext.gateWord`, `lang/go/boru.go:418-427`
  — raising the identical permission error before any matching or entry),
  then live `Registry.Lookup` + `aggregateDispatch` on the lead (honoring
  rebinding, §6.5), kernel `MatchSignature` over the claimed window
  (variable window — the arity is an output of collection, not an input,
  closing the fixed-pop unsoundness), then handler call / unit entry /
  curry-list construction (`curryOrStack`'s compiled twin) / the
  interpreter's exact no-match raise.

**The two-phase model was solving a problem this model does not have
(measured 2026-08-30).** The recorder side landed as a two-phase capture —
extent and Tokens at collection time, `SlotDesc.Source` at `RecordCall` —
because a region SLOT is a written-order TOKEN while a dispatch's OPERANDS
are sig-order ARGS, so filling Source appeared to need a join between the
two. Three implementations of that join were built and reverted. Model 1
read tape positions and got `[-1 -1]` for `add 1 2`, because
`rearrangeForForward` (`core/go/engine.go:3419`) moves forward args BEFORE
the word during collection, destroying written order by dispatch time.
Model 2 keyed on `funcIdx`, which MOVES during collection (0 → 1 across two
arrivals). Model 3 counted arrivals, and both measurements built to validate
it were themselves wrong — each caught by its own internal consistency
check, neither by its headline number.

The census that settled it asks what none of the three asked: **is the join
needed?** Over the corpus — 14521 regions, 69474 tokens, leads included
(`test/go/langspec` `TestRegionSlotTokenKinds`):

| token kind | count | share | of which LEAD |
|---|---:|---:|---:|
| word | 31682 | 45.6% | 8974 |
| const | 16847 | 24.2% | 2755 |
| active | 10601 | 15.3% | 2155 |
| group | 9830 | 14.1% | 550 |
| atom | 483 | 0.7% | 87 |
| type | 24 | 0.0% | 0 |
| mod | 7 | 0.0% | 0 |

Zero unclassified, and **not one of the seven kinds is an operand lookup**. A
const or an atom interns its own token — the token IS the value. A type
interns the canonical registry node. A group becomes a conditionally-run
compiled fragment. A `mod` is not an operand at all: the collection walk
CONSUMES the marker (`CollectArrival`'s `win.Remove(valIdx+1)`), and it is
already captured as the slot's `Quote`. And a word is derived LIVE, which is
this section's own `wordRef(name)` class and which the descriptor model
mandates anyway — a word's class is contextual, so a record-time freeze is
the miscompile `region_desc.go`'s type doc opens by refusing.

`active` is a correction to this census's own first version, which folded it
into `const` and so over-reported what the model reaches by more than
double. A compound literal whose MEMBERS are active tokens is not its own
value: `[true true true]` is a list of three WORDS until they run, and an
interpolated string is a template until its holes do. Freezing one as a
const is the frozen-class mistake one level down, INSIDE the literal, and it
puts `active` with `group` — a fragment in disguise — rather than with
`const`. The live probe below is what caught it.

So the join came from treating the descriptor as a record of what a dispatch
CLAIMED. It is a record of what the region CONTAINS; deciding what is
claimed is `OpCollect`'s job, live. Phase B, as a phase, does not exist.

> **CORRECTED 2026-09-03, and the correction is the difference between a
> SEARCH and an INDEX.** Phase B exists and has landed
> (`compiler/go/region_complete.go`) — but not as the join three models were
> reverted for. Those searched a written-order slot for its already-resolved
> operand. The rule makes searching unnecessary: matching fills sig positions
> from the forward tokens in written order, so over the LEADING positions the
> two orders are the same order and slot *i* is sig position *i*. The source
> is then `ops[i]`, a direct index.
>
> What the reverted models really got wrong was ASSUMING that, and the
> checked version is what this note's own measurement asked for: completion
> walks the slots against the operands, compares each by VALUE IDENTITY, and
> stops at the first that does not correspond. Where it stops is `NFwd`.
> Three consequences the corpus then showed, none of them predictable from
> the design:
>
> - **A word slot's answer is decided by where its BINDING lives, not by where
>   the dispatch sits.** Inside a fn body the analysis binds params and
>   body-local defs into the def stack so the body can be analysed, so they
>   resolve during the pass — but the emitted body reads them from the FRAME
>   or bakes them, and at run time the def stack holds no such binding. The
>   discriminator is the closure-capture rule verbatim
>   (`Defs.Depth(name) > FnBaselines[top][name]`), because captures and
>   descriptors ask the same question. A frame-bound name takes `SlotLocal`
>   when the operand names its slot (a param, a loop iterator) and STOPS the
>   claim otherwise (a body-local `def`, whose computed form is promoted to a
>   local only after completion). A module-scope name read from inside a fn
>   body stays LIVE — that is the `k` pair, the shape OpCollect exists for.
>   Measured: of 6361 claimed word-token slots only **848 are live wordRefs**;
>   5513 are frame slots.
> - **A word slot resolves against the DISPATCH's registry.** The recorder's
>   own `es.reg` is the last registry `BindRegistry` saw, which after a call
>   into a boru-implemented module is that module's sub-registry; resolving a
>   later main-registry dispatch's words there finds nothing and silently
>   shortens the claim. The registry rides the Phase-A offer, and must never
>   reach `RegionDesc` — a Program is shared, and a run may be handed a
>   different registry fork.
> - `SlotConst` 16596, `SlotLocal` 5513, `SlotWordRef` 848, `SlotEvent` 1,
>   `SlotType` 0, `SlotGroup` 0 over 38734 descriptors from 5629 programs.
> - The descriptor rides its call EVENT and is appended to `Program.Regions`
>   by the LOWERER, like `DispatchSpec`. A recorder-side table is not
>   rollback-safe: measured, moving the append removed 55 descriptors that
>   belonged to discarded loop-analysis rounds.
>
> Gated inert by `TestRegionTableWellFormed` over the whole corpus. Nothing
> reads the table yet.

**`wordRef` was specified here and missing from the implementation.** The
`SlotDesc` sketch above lists `wordRef(name)` among the classes;
`region_desc.go`'s `SlotSource` enum shipped without it, and that omission
is the whole reason a word slot appeared to need a join — with no source of
its own, the only way to give it one was to match it against an already
resolved operand. `SlotWordRef` is now a member, it addresses no table (the
name is in `Token`), and `CaptureRegionSlots` finishes a word slot outright.
What that reaches, on the same corpus:

| completable at capture | regions | share | slots |
|---|---:|---:|---:|
| every slot is const / atom / type | 1899 | 13.1% | 2850 |
| …adding wordRef (no group, mod or active) | **4859** | **33.5%** | **9546** |

A third of the corpus's regions therefore have a descriptor the model can
express with no further machinery, up from an eighth. What is still owed is
a compiled fragment for the two kinds that need one — `group` (21.2% of
regions carry one) and `active` — and a decision on the `mod` marker, which
is 7 slots and not an operand.

**The written-order rule holds at 99.83%, measured live rather than read off
the docs.** A temporary probe in `RecordCall` compared each captured
region's slots against the args the dispatch actually received, over the
whole corpus gate (`TestSpecCompiledOrFallback`). The rule under test is
CLAUDE.md's: matching fills sig positions from the FORWARD tokens in WRITTEN
order, then fills every remaining position from the value stack — so for
`k = min(slots, args)`, `slot[i]` must be `args[i]` for every `i < k`,
whatever the split.

| outcome | count | share of simple regions |
|---|---:|---:|
| prefix matches, every arg came forward | 2125 | 14.7% |
| prefix matches, further args from the STACK | 12327 | 85.1% |
| MISMATCH | 25 | 0.17% |

Two things follow, and the second is the one that changes the build.

First: **the mixed forward/stack dispatch is the MAJORITY, not an edge** —
85% of simple regions take some args from the value stack. A `RegionDesc`
describes only the forward half, so `OpCollect` must fill the remaining sig
positions from the runtime stack. That is not a gap in the model (the rule
says exactly this) but it does mean a region descriptor alone never
determines the operand set.

Second: **the lowerer must VERIFY the prefix, not assume it.** 25 cases in
14477 do not satisfy it — a `none` word rendering against a `None` value, a
record type against its expanded object form, and a handful whose slots and
args genuinely disagree. The lowerer holds both the slots and the args at
the routing decision, so comparing them costs nothing and turns the
ordering rule from an assumption into a checked precondition. Three
reverted models assumed an ordering; the one that survives checks it.

**Capture must stay side-effect-free, and that is a constraint on where
sources get filled.** `es.intern` NEVER pools compounds: two source `[1]`
literals must stay two constants, because `eq` on compounds is identity. A
region is offered to the recorder on EVERY execution of its dispatch, so a
capture that interned would append a fresh const per loop iteration —
unbounded const-table growth, silent. `SlotWordRef` is safe to fill at
capture precisely because it costs nothing and cannot drift; const, type and
group sources must be filled once, by the lowerer, not once per execution.

**What routes, measured, and what the routing predicate must NOT do.** A
first cut of the region-dispatch lowering was built and reverted; the
numbers it produced are the design brief for the next one.

The predicate it tested: the lead is a word, every slot is const / atom /
type / wordRef, and — the precondition, CHECKED rather than assumed —
`slot[i] == args[i]` over the overlap. Attributed over the corpus (7634
rows, 82670 mono dispatches), of the 40070 dispatches that reach it:

| outcome | count | share |
|---|---:|---:|
| routable | 3045 | 7.6% |
| declined: **inside a fn unit** | **30873** | **77.0%** |
| declined: prefix mismatch | 2871 | 7.2% |
| declined: slot kind (group / active / mod) | 1699 | 4.2% |
| declined: no region (a stack-only dispatch) | 1162 | 2.9% |
| declined: no args | 420 | 1.0% |

**The fn-unit exclusion is four fifths of the gap** — not one restriction
among five. It is there because a word slot inside a compiled fn body names
a PARAM, which lives in the frame rather than in the def stack the VM's live
lookup reads, so a live re-derivation would miss it. Lifting it is worth
more than every other relaxation combined, and it is bounded work: a
name-to-slot map per unit, and a word slot that resolves to a param lowering
as `SlotLocal` rather than `SlotWordRef`.

**And the predicate must scan only the CLAIM, not the region.** Requiring
every slot to be routable measured **+9.9%** on
`BenchmarkPerfCompile/arith_chain64` (interleaved A/B, four rounds of two
compiled binaries; `PerfCheck/arith_chain64` and `PerfCompile/for_tight`
were flat, which is the tell — the cost appears only where the recorder is
active and the region is long). The cause is structural rather than
incidental: a region runs to the next hard delimiter, so it is as long as
its STATEMENT, and a 64-term arithmetic chain hands its first dispatch a
128-token region. Classifying all of them, once per dispatch, is quadratic.

Caching the classification on the slot does not fix it — the capture walk is
itself per-dispatch — and neither does moving the scan to lowering, since
each dispatch has its own descriptor. What fixes it is noticing that the
scan is unnecessary: **a slot beyond the recorded claim is not this
dispatch's operand.** It matters only if a LIVE collection claims further
than the recording did, and that case is already covered — the arity check
defers, and a slot the runtime cannot resolve defers too. So the predicate
scans `slots[0:NFwd]`, and `Validate` permits `SlotNone` at `i >= NFwd`,
documented as "beyond the recorded claim; the runtime defers if collection
reaches it". That is the one place the invalid zero is not a defect, and it
is worth stating rather than discovering.

> **LANDED 2026-09-03.** `RegionDesc.NFwd` and the `Validate` relaxation are
> in the tree, and the corpus says the bound is not a micro-optimisation:
> **22958 of 65959 span slots are claimed**, so 60% of every region a
> descriptor describes is beyond its own dispatch. The claim's shape is just
> as skewed — 17650 descriptors claim nothing forward at all, 5292 claim a
> prefix, 15792 claim the whole span.
> A reader that assumed its window IS the dispatch window would be wrong far
> more often than right.
>
> The relaxation has a boundary, and it needs one: an unsourced slot beyond
> the claim is legal, an unsourced slot carrying an INDEX is still a lowerer
> writing past its own claim, and `NFwd` itself is range-checked against the
> slot count.

One correction to how this was measured, because it nearly caused a wrong
decision. The first comparison used a benchmark file recorded earlier in the
session, and it reported the FIRST-WINS capture fix — a change that
strictly does less work — as 1.4% to 7.9% SLOWER across every case. That is
impossible, and it is what exposed the baseline as stale rather than the
change as costly. Only the interleaved A/B above is load-fair, and the
+9.9% figure is that one. A performance number from a file measured under a
different machine load is not a measurement.

**WHERE A REGION DISPATCH MAY BE USED IS CONSTRAINED BY THE LANGUAGE, not
just by what the descriptor can express.** A first executing slice was built
end to end — descriptor claim, routing predicate, `OpDispatchRegion`, a VM
path modelled on `callPolyIn` — and it worked: `10 sub 3` lowered to

```
0000 PUSH_CONST  k1   ; 10 (Integer)
0001 DISPATCH_REGION r0   ; sub fwd=1/2 slots=1 (region)
```

one push for the stack half, the forward operand `3` taken from the
descriptor, answering 7. It was reverted anyway, because two tests that were
doing their job said the target was wrong.

`TestEmitSplitFormsIdentical` (`lang/go/bytecode_emit_test.go`) states the
rule: *"One split rule, one bytecode per ASSIGNMENT: every split of the same
sig-order assignment lowers identically."* `1 add 2`, `1 2 add` and
`add 2 1` all assign sig[0]=2, sig[1]=1, and must produce the same program.
CLAUDE.md says the same thing at language level — *"a call form only chooses
where the split falls"* — so the forward/stack split is SURFACE SYNTAX, and
the compiler deliberately normalises it away.

A region descriptor carries that split, in `NFwd`. Routing a forward or
mixed dispatch and leaving the stack-only form on `CALL_NATIVE` therefore
lets syntax survive into the bytecode, and the three spellings diverge. This
is not fixable by widening the routing: a stack-only dispatch has no region
to capture (a region IS a forward-collecting dispatch), and synthesising an
empty one still yields a different push sequence. The only shape that
preserves the rule is to push every operand as before and use the descriptor
solely for re-derivation — which spends a live lookup and a live match on
every dispatch to buy nothing the baked signature was not already giving.

**So the target is not "dispatches that can be described". It is dispatches
the ordinary lowering CANNOT HANDLE.** The invariant above does not apply to
them, because there is no compiled form for the routed one to be identical
to, and the live re-derivation buys exactly what it costs.

But §6.5's latches are NOT one class, and treating them as one was this
note's first mistake. Only the FROZEN-READ shape is a region's business:
`module binding k rebound after a fn unit baked its value` refuses because a
unit baked a value whose token classification a live re-derivation would
change — `region_desc.go`'s `k` pair, where the same token is a value slot
or a collection barrier depending on the binding. That is exactly what
OpCollect answers.

> **CORRECTED 2026-09-02.** Right about that shape, wrong as a description
> of the gate, which covers three structurally different divergences and
> only one of them is a region's. In `def x 1 … [x add y] … def x 2` the
> token `x` PRECEDES the word, so it is a value-stack operand that no
> forward-collecting window ever sees — its slot is an ordinary
> `PUSH_CONST`. And a splice payload bakes an INSTRUCTION SEQUENCE, which no
> descriptor re-derives. OpCollect answers the `k` pair and nothing else
> here. (It also does not exist yet: `Program.Regions` has zero writers and
> zero readers, and `OpCollect`/`OpDispatchGeneric` appear only in
> comments.)

Its stored-handler twin is a different failure. `NotifyNameRebound`
(`compiler/go/emit.go:3145`, the stored-handler arm) refuses because
module-scope def sites
execute only in the CHECK pass, so by VM time the def table already holds
the pass-final binding and calls sequenced BEFORE the rebind read the wrong
definition. Nothing about that program's split is value-dependent, and no
amount of live re-matching restores program-order state. **The bind twins
(§5.6) fix it; a region dispatch cannot.**

Two further facts from that slice, both worth keeping:

- **A region route must REPLACE a special form's lowering, never run
  alongside it.** `lowerCall`'s special forms (typed bind, dyn-apply, drift
  window, dyn-method, make-list, make-map, splice, interpolation, XML) each
  build their own operand window. The slice skipped the forward operands
  while one of those arms still emitted its own opcode expecting them, and
  the result was a wrong ANSWER rather than a crash: a corpus assertion came
  back "expected 3, got 2".
  Stated as "a routed call must be a plain dispatch" — which is how the
  first version of this note put it — the constraint would be WRONG, and
  wrong in the direction that matters most: T2 names the drift window's
  `OpCallDynamicMixed` as an island to delete, and the paragraph above makes
  that island Stage 4's target. A rule excluding `dynMixed` from routing
  would forbid the one route this stage most needs. The real constraint is
  about co-existence, not eligibility: whichever arm owns the operand
  layout must be the arm that emits.
- **A MODULE-QUALIFIED dispatch cannot route on a bare word.** A module word
  re-matches over its OWN sub-registry's signatures — `PolyRef` carries that
  registry for exactly this reason — while a descriptor names only the word.
  Resolving it against the dispatch registry finds the wrong overload set or
  none, and the module-gate parity tests catch it.

**AND THE DECLINE SITES ARE COLD — so "route what the ordinary lowering
cannot handle" needs one more step to be actionable.** Tallied over the
corpus (7638 rows, each compiled and its refusal reason collapsed to a
shape):

| refusal shape | rows |
|---|---:|
| check diagnostics | 191 |
| check error | 134 |
| parse error | 47 |
| def-bound computed fn apply (closure shape unknown) | 1 |
| unconsumed fn-value carrier in residual | 1 |

374 refused of 7638, and 372 of those are programs that are SUPPOSED to
fail. The compiler's own refusal surface on this corpus is **two rows**, and
neither is a rebind-staleness latch: `module binding … rebound after a fn
unit baked its value` and its stored-handler twin fire **zero** times.

That is not an argument that the latches do not matter — they are
correctness under rebinding, and a corpus of literal programs will not
exercise them. It is an argument about what can be VALIDATED: a region
dispatch built for those sites has no corpus evidence to stand on, and this
stage has already spent three attempts on models that passed everything
available to them.

So the target that is both correct and measurable is the third one:
**the mixed-window island**. `OpCallDynamicMixed` islands its token window
through an interpreter sub-run (`eng/go/vm.go` `islandRun`, reached from
`drift_window.go`'s model), which is interpretation running INSIDE compiled
code — the thing this document exists to remove, not a fallback outside it.
It is also one of the four mechanisms the interpreter-entry census still
counts at 43 ("a fn value in a container at the pointer"), so progress on it
is visible in a number that already exists. The region descriptor is the
right replacement for that island precisely because such a program has no
ordinary compiled form to be identical to — the split-identity rule above
does not bind, and the live re-derivation is the whole point.

**THE ISLAND TARGET, NAMED DOWN TO THE ROW.** The interpreter-entry census
(7638 rows, 7215 compiled, 43 with unattributed entries, ceiling 43) puts
`vm:island` at 12 entries and `vm:island-resolved` at 8. Its largest single
file cluster is `path-modifier.tsv` — **7 rows, every one of them islanding**
— and with `BORU_LOG_CENSUS_ROWS=1` they are:

```
L17  def m {a:add/v} end  m.a/u 1 2
L18  def m {s:sub/v} end  m.s/u 10 3
L19  def o {m:{a:add/v}} end  o.m.a/u 1 2
L20  def m {a:add/v} end  (m.a)/u 1 2
L52  def m {s:sub/v} end  usurp (forward-args (m.s)) 10 3
L53  def m {s:sub/v} end  force-arity 2 (usurp (m.s)) 10 3
L55  def m {a:add/v} end  force-arity 2 (usurp (forward-args (m.a))) 1 2
```

One shape, seven spellings: **a fn value read out of a container, dispatched
under a modifier** (`/u`, `forward-args`, `force-arity`). That is exactly the
"fn value in a container at the pointer" mechanism the census attribution
assigns to Stage 4, and exactly what `emit.go`'s stamping note predicted —
*"of the island rows this seam leaves, roughly four in five are a fn read out
of a container … `def m {a:add/v}  m.a/u 1 2`"*. L17 is that example
verbatim.

**And the island is NOT the drift window** — that guess came from the census
seam name, and the disassembly refutes it. All seven lower to the same
shape: the wrapper words fold into `CALL_NATIVE_POLY`, and the apply is a
plain `CALL_DYNAMIC`.

```
0000 PUSH_CONST  k1   ; {s:fn …} (Map)
0001 PUSH_CONST  k0   ; s/q (Atom)
0002 CALL_NATIVE_POLY p0   ; dot/2 (poly)
0003 CALL_NATIVE_POLY p1   ; usurp/1 (poly)
0004 PUSH_CONST  k2   ; 10 (Integer)
0005 PUSH_CONST  k3   ; 3 (Integer)
0006 CALL_DYNAMIC /2 ; apply fn-value
```

So the island is `callDynamic`'s third tier (`eng/go/vm.go:834-838`). That
function tries a native apply, then the Apply kernel's frame entry, then
islands. A modifier-wrapped native falls past both: it is not a plain native
because `vmNativeApplicable` excludes it, and it carries no compiled unit
because it is a native rather than a user fn.

**The exclusion is one clause, and it is there because admitting these
already produced a wrong answer once.** `vmNativeApplicable`
(`eng/go/vm.go:1382-1392`) reads `IsNativeWordFnDef(fd) && !fd.ArgsReversed
&& RegisteredWordIsNative(r, fd.Name)`, and `ArgsReversed`'s own doc records
the incident: *"a param-type comparison admitted `m.s/u 10 3` to a VM fast
path that then answered -7 against the interpreter's 7"*. The flag exists so
the decline is possible at all, because `sub(Number, Number)` reversed is
still `sub(Number, Number)` and the swap is invisible to inspection.

**But the cause is narrower than the decline.** `tryNativeFnApply` resolves
its signatures registry-first:

```go
if inner := reg.Lookup(fnDef.Name); inner != nil {
    sigs = inner.Signatures
} else if len(fnDef.Signatures) > 0 {
    sigs = fnDef.Signatures
}
```

For a wrapper that is the wrong source — the wrapper keeps the wrapped
word's NAME, so the lookup finds the UNWRAPPED signatures and the reversal
is dropped. That is precisely the -7. `UsurpFunction`
(`core/go/core_ref.go:110-140`) builds a value whose own `Signatures` are
complete and self-contained: each carries a Go handler
(`usurpDispatchHandler`) that re-dispatches the original, declared
`RunInCheck`. Dispatching a wrapper through the VALUE'S signatures rather
than the registry's is therefore well-defined where dispatching it through
the registry's is not.

**And reading that handler settles it: it does not dispatch at all — it
returns TOKENS.** `usurpDispatchHandler` (`core/go/core_ref.go:333-354`)
builds

```
( a0 … a(N-1-B)   orig   a(N-1) … a(N-B) )
```

— an open paren, the stack-part args in order, the ORIGINAL fn value, the
forward-part args reversed, a close paren — and returns that sequence as its
result. The engine then steps it. That is what "its Go handler expects the
engine's collection around it" means, stated exactly: the handler's output is
a paren group, and a group has to be EVALUATED by something with a tape. The
VM has none, so it islands.

So these rows are not primarily a §6.2 collection problem. They are a §6.8
one — **the tape-coupled handler class** — and the two meet here, because the
window the handler returns IS a region: a paren group whose lead is a fn
value, with a specific forward/stack split. The collection machine is the
right executor for it; the handler returning tokens is the coupling that has
to go first.

**Which also explains why only the CONTAINER spelling islands.** The
`UsurpFunction` comment notes that `RunInCheck` lets the carrier compiler
step the re-dispatch and compile the original call directly — *"`usurp (valof
f) a b` lowers exactly like `f b a`"*. That works when the wrapped value is
statically known. In all seven rows it is read out of a map (`m.a`, `o.m.a`),
so at check time it is dynamic, the carrier compiler cannot step it, and the
token window survives to run time with nothing but the interpreter able to
walk it.

**The purity holds, for all three wrapper families, and by inspection.** Both
handlers take `(args []Value, _ map[string]Value, _ []Value, _ *Registry)` —
three ignored parameters, so no context data, no extra values, no registry —
and read nothing but `len(args)`, the captured `origBarrier`, and the
captured `orig`:

```go
usurpDispatchHandler:      ( args[0 … n-b-1]   orig   args[n-1 … n-b] )
rebarrierDispatchHandler:  ( args[n-1 … b]     orig   args[0 … b-1]   )
```

`rebarrier` covers `forward-args` / `stack-args` (`ForceForwardFunction` /
`ForceStackFunction`) and `force-arity` (`ForceArityFunction`), so between
the two every one of the seven rows is accounted for. Each is a pure
RESHUFFLE: a paren group whose lead is a fn VALUE, a stack part before it, a
forward part after it, and the split fixed entirely by `n` and
`origBarrier` — both static at record time, since the arity is the recorded
call's and the barrier is baked into the wrapper at construction.

**And working the permutation through collapses it further than a descriptor
— to a PERMUTATION OF THE ARG VECTOR, independent of the barrier.** Apply
CLAUDE.md's argument-order rule to each window (forward tokens fill sig
positions `0 … k-1` in written order, the rest come off the stack top-first):

```
usurp,     any b:  sig[i] = args[n-1-i]     — a full REVERSAL
rebarrier, any b:  sig[i] = args[i]         — the IDENTITY
```

The barrier cancels out of both. That is not obvious from the handlers and
it is the whole economy of the fix, so it is checked rather than asserted —
three derivations (b=0, b=1, b=n) and then the runtime, with a case chosen
to come out WRONG if the claim were backwards:

```
m.s/u 10 3                          →  7    usurp reverses
usurp (forward-args (m.s)) 10 3     →  7    …at any inner barrier
usurp (stack-args (m.s)) 10 3       →  7    …at any inner barrier
force-arity 2 (usurp (m.s)) 10 3    →  7    …and under composition
forward-args (m.s) 10 3             → -7    rebarrier alone is the IDENTITY
10 3 stack-args (m.s)               →  7    …in either call form
```

`-7` is the load-bearing row: `sub` computes `args[1] - args[0]`, so an
identity mapping must answer `3 - 10`. Had rebarrier reversed too, it would
read 7 like the others and the whole table would prove nothing.

So the seven rows need neither a tape nor a region: they need the arg vector
permuted and the WRAPPED value dispatched. `RegionDesc`'s `LeadFnValue` is
still the right shape for the general tape-coupled handler — a paren group
led by a fn value — but this particular family is a special case that
resolves statically, and building the general machine to serve it would be
building past the evidence.

**What blocks it is one missing field.** `orig` and `origBarrier` are Go
closure captures inside the handler, so the VM cannot see what a wrapper
wraps: it can tell THAT a value reverses (`ArgsReversed`) but not what to
dispatch instead. Exposing the wrapped value on `FnDefInfo` is what turns
this from an island into a permutation and a call.

**WHAT IS LEFT AT 36, named and split.** After the wrapper lane graduated, the
island seams stand at `vm:island` 5 and `vm:island-resolved` 8, and the rows
behind them are two different problems rather than a residue of one:

```
vm:island (5) — a fn value read out of a CONTAINER, dispatched at the pointer
  class.tsv:L122   def C class {op:(fn …)}  def c (make C {})  c.op 21
  fn-value.tsv:L19 def m {f: (fn …)}  3 m.f 2
  fn-value.tsv:L28 def m {f: (fn …)}  m.f 'x'
  usurp.tsv:L33    def ops {rev: (usurp (valof sub2))}  ops.rev 10 3
  usurp.tsv:L45    def ops {rev: (usurp sub2)}  ops.rev 10 3

vm:island-resolved (8) — a fn value crossing a MODULE boundary
  module-fnvalue-boundary.tsv L24, L32, L33, L51
```

The second cluster is not a hole: the apply seam's foreign-unit decline is
deliberate, and the reasoning is recorded against it.

The first has ONE cause, and the disassembly says so. Two of the five reach
`CALL_DYNAMIC` and one reaches `CALL_DYNAMIC_MIXED`, so the site differs —
but every one of them fails at the same test inside `dynApplyEnter`
(`eng/go/vm_dyn_apply.go:132`):

```go
ref := compiler.CompiledRef(sig)
if ref == nil || ref.Prog != vc.p || ref.Unit < 0 || … { return nil }
```

A fn value read out of a container carries no `CompiledRef` to a unit of
this program, so the Apply kernel cannot enter it and the island answers.
Unwrapping does not help even where it applies: `usurp.tsv:L45` DOES reach
the new unwrap path, resolves to `sub2`, and still declines — because it is
`sub2`'s own value that lacks the ref, not the wrapper.

So the next target is not another dispatch site. It is the fn-stamping seam
that gives a container-read value its unit — the one whose own note
predicted this cluster (*"roughly four in five are a fn read out of a
container … So descending into list and map consts is the rest of the
win"*). The question to answer first, by reading rather than measuring: why
the stamp does not reach these five, when that descent is already
implemented.

**F2's carried-state list is INCOMPLETE — by three readers, all
region-local.** Probed 2026-08-26, enumerating what the no-match raise path
actually reads rather than trusting the list. F2 fired once and widened this
list by four items; it is short by at least three more:

- `IsFnShapeTypedBindingContext` (`core/go/engine.go:759`) — appends the
  *"this is a typed-binding context expecting a function value — did you mean
  `X/q`?"* suggestion to a `signature_error` (`:3286`), and separately GATES
  `PolyNoMatchProbe` entirely (`:446`), so it changes whether the poly probe
  runs at all and not merely how the text reads. It needs the enclosing
  collector's Forward, that sig's arg-0 type, the MAP VALUE sitting at
  `FuncIndex - CollectedArgs`, and a live `ResolveTypedName` on the
  constraint word.
- `pendingForwardFunc` (`:806`) — the enclosing collector's NAME, which
  tailors an undefined-word hint when it is `def` (`:842`).
- `polyReachBound` — read into the same poly probe alongside the residual
  (`:450`), so the reach bound rides with `reorderCandidates`' four values
  rather than being implied by them.

**They do not falsify the region bound, and the reason is the invariant
above.** All three key on the ENCLOSING COLLECTOR'S FORWARD, and at most one
Forward is ever live per paren scope — so the collector they find is the
region's own, never an earlier region's, and its collected args sit adjacent
to its `FuncIndex` and are in-region with it. The one-forward-per-scope
invariant therefore does double duty: it bounds the stranded-forward scan AND
it bounds these. The list needs extending; the bound does not.

One candidate ruled OUT, which is worth recording so it is not re-derived:
`insufficientArgsError` appends a `"stack: "` note built by
`describeStackTypes` (`core/go/boru_error.go:315`) over the tape window
`[pointer-3, pointer+4)` — bounded, but stopping at neither a paren nor a
region boundary, and rendering `word(name)` / `atom(name)` payloads rather
than types alone. That WOULD be extra-regional state. It is unreachable: its
sole production call site (`engine.go:7509`) carries a proof-carrying
`//covergate:allow` as a defensive arm unreachable via the harness, so the
note never reaches a user and needs no carrying. If that arm ever becomes
reachable, this paragraph is the falsifier.

**Correction (2026-08-26): that widened state cannot live in the
DESCRIPTOR.** The list above says the descriptor "must additionally carry"
the value-stack residual, the `voidGroups` record, the `barrierReceiverWord`
answer and the suggestion's suppression condition. It cannot, and the reason
is structural rather than stylistic: a `Program` is SHARED — every run
spawns branch goroutines that `RunUnit` the same Program's units on their
forks (`lang/go/bytecode_concurrency_test.go`) — so a descriptor field
written during collection races, and one frozen at record time selects the
wrong error for every later execution. Both failures are silent.

Every item on that list is runtime-varying, which is what makes the split
forced. Whether a paren fragment collapses to zero values depends on what
the fragment computes, so it can differ between two executions of one
region. The enclosing residual is whatever the stack holds now. And this
section already calls `barrierReceiverWord` a LIVE probe — a live answer
cannot be a static field.

So the descriptor splits in two: `RegionDesc` holds what is true of every
execution (the lead, the slots, the position), and a per-invocation
`RegionState` holds the raise-selection facts, built fresh by `OpCollect`.
That does not weaken the region bound — every one of those facts is still
region-LOCAL, by the one-forward-per-scope invariant above. Local and
per-invocation are different properties, and this section conflated them.

**Live revalidation, because boundaries are binding-dependent.** The
forward scan's extent is derived from the *live* binding's signatures
(per-signature `forwardLimit` = `BarrierPos`), and even a token's
word-vs-value class can change when a name is rebound. A descriptor that
froze record-time classification would therefore collect wrongly after a
rebind (§6.5). So slots carry the underlying **token identity** alongside
their record-time class, the descriptor spans the statement's full
syntactic region (hard delimiters — `end`, close-paren, statement commit —
bound it regardless of bindings), and `OpCollect` re-derives class and
scan extent against the live binding set using the shared routine — the
same re-derivation the interpreter performs on every execution. Paren
groups stay pre-compiled fragments (their boundaries are syntactic and
cannot shift); what revalidation changes is only which slots a given live
signature set claims.

**Why an `end`-bounded extent is enough for the stranded-forward state —
PROVED 2026-08-26, and it needed proving.** This section bounds a region by
hard delimiters and then assumes the descriptor carries the state selecting
the stranded-forward raise text. Those two claims do not obviously compose.
`strandedForwardError` (`core/go/engine.go:6543`) selects its Forward by
scanning BACKWARD from the pointer, stopping at an OpenParen — **`end` is
not a stop condition**. A second Forward parked in an earlier region would
therefore be reachable by a scan inside this one, and a region-bounded
descriptor would not carry it. Given F2 already fired on this section's
carried-state list, completeness here is argued, not assumed.

The text selection is real, and `end` is what makes it:

```
g 1 def x 5 x       → "g is still waiting for 1 argument(s) when `def`
                       begins its own dispatch"      (strandedForwardError)
g 1 end def x 5 x   → "cannot call `g` — no signature matches"
                     + "the argument was 1 (an Integer)"     (no-match raise)
```

`stepEnd` (`:6604`) runs the *identical* backward scan and REMOVES the
Forward it finds. So nothing is left to strand once the `end` has stepped —
but `stepEnd` removes only the NEAREST one, so this is sound only if at most
one Forward is ever live per paren scope. It is, and the argument is short
because the parking surface is:

- `insertForward` (`:3452`) is the SOLE parking site (`:3798` only rewrites
  an existing marker in place), and it has exactly two callers.
- `:2786`, the bare-function-word dispatch. Guarded: both strict-barrier
  sites (`:2253` for `/u`, `:2564` for a bare word) run
  `commitBarrierForward` and then `strandedForwardError` BEFORE dispatch, so
  a live forward is either committed away or raises. Neither reaches a
  second park.
- `:4982`, inside `execFnDefLiteral` — a function **value** forward-
  collecting. This one is NOT gated by the bare-word barrier, and is the
  case worth checking. It cannot fire either: for the value to reach
  `execFnDefLiteral` the arrival loop must let it through, and with a
  forward parked the arrival routes it into that forward's slot instead.
  `g 1 (mk 10) 3` claims the function as `g`'s argument (`g` then fails
  inside its own body); `g 1 h 3` meets the barrier and strands.

So the backward scan provably cannot cross a region boundary. Two
consequences Stage 4 has to honour rather than rediscover:

- **The invariant is held by the strict barrier and the arrival loop
  TOGETHER**, not by either alone. An `OpCollect` adapter that reproduced
  the barrier but not the arrival routing would park a second forward and
  silently change which raise text fires — a parity break with no existing
  test to catch it.
- **That second caller is held only by the arrival loop**, which is the one
  collection loop with ZERO samples on both sides of the Stage 2 CPU-share
  gate (§11, F1b). The gate's blind spot and this load-bearing path are the
  same path, so Stage 4 work touching arrival needs purpose-built tests; no
  benchmark in the tree exercises it.

What is guarded, and what is not. The scan half — that `end` drains the
forward, which is what makes the two texts exclusive — is pinned white-box
in `core/go/region_extent_test.go`, and the two texts themselves are pinned
as `lang/spec/forward-barrier.tsv` §9 (they were previously indistinguishable
there: both are `signature_error`, and every existing row matched only
`ERROR:signature`). The ARRIVAL half has no corpus row, and the reason is a
Stage 4 datum in its own right: **the shape does not compile today**.
`g 1 (mk 10) 3` refuses with *computed closure at a word's argument slot (its
apply did not collapse)* — it is a §9-family fn-value refusal — so it cannot
be a spec row, which must satisfy both lanes. It is measured by hand (`1 3`
interpreted) and becomes a row when Stage 3's fn values make it compile.
That the invariant Stage 4's descriptor rests on is currently witnessed only
by a shape Stage 3 has yet to close is worth carrying forward rather than
forgetting.

**Live revalidation has a witness now, and today's answer to it is a
REFUSAL.** Probed 2026-08-26. This section asserts that a token's
word-vs-value class can change on a rebind, so a descriptor freezing
record-time classification would collect wrongly; the PR #406 review pressed
the same point. Both are right, and the divergence is one line of source
apart:

```
def w fn [[a:Any b:Any][Any][a]] end
def k 5 end
def go fn [[][Any][w k 1]] end
go                                       -> 5          k is a VALUE: collected
… def k fn [[][Integer][9]] end  go      -> raises     k is a WORD: barrier
```

Same body, same source position, same descriptor. A descriptor that froze
`k` as a value slot would answer `5` where the interpreter raises — a silent
wrong answer, not a crash, which is the failure mode this whole section
exists to prevent.

Two things the probe adds to the section's account. First, where the rebind
precedes the call in source order the CHECKER catches it — `go`'s body is
analysed against the post-rebind binding and the program never compiles
(`fn_body_error`), so the lanes agree without anything re-deriving at
runtime. That agreement is not evidence the compiled lane revalidates.
Second, where the rebind sits BETWEEN two calls of the same region, so the
checker cannot fold it, the compiled lane REFUSES, by name:

```
def r1 (go) end  def k fn [[][Integer][9]] end  def r2 (go) end
  -> bytecode compilation refused: module binding k rebound after a fn unit
     baked its value      (compiler/go/emit.go:3159, the fn-unit arm)
```

That is precisely one of the interim rebind-staleness latches §6.5 says the
bind twins delete. So the current architecture's answer to live revalidation
is not a frozen classification and not a re-derivation — it is a refusal
standing in for both. `OpCollect` re-deriving class and extent against the
live binding set is what lets that refusal go, and this pair is its
acceptance test: the compiled lane must ANSWER both spellings, `5` and the
barrier raise, rather than declining the second.

**The shared-vs-divergent map — Stage 2's actual worklist.** The three
loops are not merged; each decision they share is given one home, and each
difference is classified as intentional or as drift. The probe mapped
them:

| Decision | Phase-1 plan walk | Per-candidate scan | Status |
|---|---|---|---|
| structural boundary, `end`, `)` | `scanBoundaryToken` | *was* an inline copy | **unified** (landed) |
| forward/stack split point | `BarrierPos` + `/s` `/f` | identical arithmetic | **unified** (landed) — the source already called its copy a "mirror" |
| open paren `(` | evaluates when a viable overload consumes the position | always a hard boundary | intentional: phase 1 pre-evaluates, the scan must not |
| fn-word barrier | union-over-viable-sigs test | per-signature, and `specAt` can claim an `FnDefInfo` into an `Any` slot and walk PAST the function word | **latent, not live** (tested 2026-08-25): an `Any`-slot word followed by a function word raises the identical strict-barrier error in both lanes, because phase 1 commits or strands before the scan's weaker test is consulted. The divergence is real in the code and masked by ordering — unify deliberately, do not treat as a live bug |
| reach call-head | exempts if ANY viable sig wants `Function` | only this signature | **latent, not live** (tested 2026-08-25): a `Function`-valued member reached through dot access selects the `Function` overload identically in both lanes, as does an `Integer`-valued one. Same masking as the barrier row |
| sugar expansion | gated on viability, and MAKES the Angle head/use-site choice | refuses Angle; splices every other kind ungated | **resolved** (read 2026-08-25): not a contradiction. Angle is the only kind whose lowering depends on the viable set, and both walks refuse to commit that choice per-candidate. Every other kind has a single deterministic lowering, so expanding it early is idempotent and yields the tokens the next dispatch would have produced anyway |
| interp-string / XML / paren-expr / splice | dedicated arms that pre-evaluate the form IN PLACE | no arms — and needs none | **resolved** (tested 2026-08-25): an interp-string in a `String`-typed forward slot yields the identical value in both lanes. Phase 1 has already replaced the form with a plain value by the time the scan runs, so the scan sees no form to have an arm for. **The tape mutation IS the interface between the phases** — the VM adapter must perform the same in-place pre-evaluation, or the scan meets forms it cannot classify |
| `/q` quote slots | four checks at different stages | a fifth, differently conditioned | NOT duplicates: different questions, do not merge |

**Re-seat 1 of 3 landed (2026-08-26): the phase-1 plan walk.** The seam is
`core/go/collect_kernel.go` — a `collectWindow` (live-length, spliceable;
the interpreter's `*Tape` satisfies it as written) plus a `collectHost`
splitting the EVALUATIONS, which mutate the window and may raise, from the
CLASSIFICATIONS, which are pure questions against the live binding set. The
plan walk moved onto it verbatim: the extracted function is TEXTUALLY
IDENTICAL to the method it replaced modulo the mechanical host
substitution, which is the strongest evidence available that the re-seat is
behaviour-neutral, and `resolveForwardArgs` is now a two-line seat.

The gate F1b re-specified onto the deterministic instruments reads clean:
allocation counts are IDENTICAL on all eight interpreter shapes — 841,
4641, 7648, 6876, 1625, 1104, 6041, 16345 allocs/op before and after, not
merely under ceiling. Interface dispatch costs nothing here, as the probe
predicted.

**Re-seat 2 of 3 landed (2026-08-26): the per-candidate scan.** Same
discipline, same result: the extracted `collectCandidateScan` is textually
identical to the block it replaces modulo the host substitution, and
`MatchSignature` — already at gocyclo 87 against a cap of 70 — sheds 250
lines to a single call. The scan needed four host methods the plan walk did
not (`lookupWord`, `analysisCompiling`, `reachFnWouldClaim`,
`expandSugarTokens`), which is the seam telling the truth about what the
two loops actually share: the window, `defTop`, and nothing else. Note what
it did NOT need — no arm for an interp-string, an XML literal or a paren
expression. That absence IS the window-mutation property, seen from the
consuming side.

The one lowering the scan commits is a sugar expansion, and the seam makes
the rule explicit rather than incidental: a lowering that is a function of
the MARKER alone may commit; one that is a function of the VIABLE SET may
not. That is why the viable slice is a parameter of `expandSugarAt` rather
than host state, and why the Angle marker stays a boundary here and is
decided at arrival.

The allocation gate needs one honest footnote. The 64-iteration alloc guard
reported +1 alloc/op on three of eight shapes after this re-seat, and the
instinct — "exact instrument, so investigate" — was right to follow but the
finding was measurement, not code: escape analysis over the whole package
is byte-identical between the two states (715 sites, zero diff), and a
200-iteration benchmark of the same shape reports the SAME count for both
(7649/7649) where the 64-iteration guard reported 7648/7649. The per-op
count is simply not deterministic to ±1 for these shapes, so the guard's
integer division lands either side of it depending on the iteration total.
Ceilings are untouched. Worth recording because §11's F1b re-specified this
gate onto the alloc guard precisely for being exact, and it is exact at the
scale that matters — a per-dispatch regression — not at ±1.

**Re-seat 3 of 3 landed (2026-08-26), and it corrects this section.**
`stepLiteral` is NOT a collection loop end to end, which is what "the
arrival loop in `stepLiteral`" above implied. It is four things in
sequence: form expansion; a standalone-value path (splices, dispatch
modifiers, shaped-method dispatch, the recorder push); the arrival MATCH
decision; and commit-and-maybe-dispatch bookkeeping. Only the third is
collection.

Re-seating the whole of it was measured and rejected: it would have needed
some SEVENTEEN further host methods — `recorder`, `trace`, `inFnFrame`,
`sealFnValue`, `rearrangeForForward`, `pendingForwardIdx`, `stepSugar`,
`forceStackWord`, `fnValueWouldWiden` and the rest. An interface that wide
is not a seam; it is the Engine wearing a different name, and the whole
point of the seam is that a SECOND implementation can satisfy it from
different material. So the extraction takes the decision and leaves the
dispatches: `collectArrival` returns a VERDICT — collect, dispatch this
0-arg reach-read fn, close the window at this barrier, or implicit-end —
and the host performs whichever dispatch it names. It needed ZERO new host
methods.

The two in-place window edits it does own are exactly the ones a match
verdict depends on (the `/q` Word→Atom conversion and the `/v` marker
consumption), and the host re-reads the window afterwards to see them. The
one addition to `collectWindow` is `Remove`, because a deletion should not
have to be spelt as a degenerate splice.

The generalisable rule, and the reason this is a correction rather than a
shortcut: **the collection kernel decides, the host dispatches.** A loop
that both classifies and enters functions is two responsibilities, and the
seam is the line between them. Stage 4's adapter inherits that line for
free — it implements four verdict responses, not seventeen engine
internals.

The host methods were deliberately UNEXPORTED while nothing outside core
could implement the seam. **That is done: the seam is EXPORTED** —
`CollectHost`, its methods, the three loop entry points and the helper
types they name (`ViableSig`, `FwdKind`) — and `eng/go/collect_seam_host_test.go`
is a foreign host driving all three from outside core, which is what makes
"sufficient from outside" a fact rather than a claim.

The coverage worry that argued for keeping them private was answered rather
than deferred, and the answer is worth stating because it decides where
Stage 4's adapter LIVES. Exporting the interface renames methods that
already exist on `*Engine` and are already covered: zero new core
statements. A struct of function fields would have added one delegator per
method — statements core's own suite must then cover for a seam only an
outside caller uses. What still holds is the narrower constraint: no arm of
the SHARED routines may exist only for the VM, because an arm core's own
suite cannot reach is dead code in its profile whoever calls it. The
adapter therefore lives in `eng`, above `cover-gate-core`, and is covered
by eng's tests driving these same entry points.

The per-candidate scan and the arrival loop re-seat separately, for the
reason §10 gives: a differential failure has to name one re-seat to be
worth anything.

Only the first two were textually identical and could be unified by
inspection. Every remaining row needs the difference argued before it is
touched — which is why this is slow work rather than a mechanical sweep,
and why the three loops re-seat separately (§10). Seven of the eight are
now classified with evidence; the sugar row is the one still unbounded.

**What the probes add up to, and it is a constraint rather than a
comfort.** Five experiments over the four suspected rows all found the
lanes agreeing. The divergences the code review identified are real *in
the code* and masked *at the language level* — by phase ORDER. Phase 1
commits or strands before the scan's weaker per-signature tests are ever
consulted, and phase 1 pre-evaluates forms out of existence before the
scan would have to classify them. So the ordering is not an
implementation detail to be tidied during extraction: it is load-bearing
for correctness, and the extracted machine must preserve it exactly. An
adapter that ran the classifying scan against un-pre-evaluated slots, or
that consulted the per-signature barrier test first, would expose every
one of these differences at once.

**A hazard that turned out not to be one — recorded because the
correction is the useful part.** The per-candidate scan splices sugar
expansions into the tape (`core/go/engine.go:8731`) *ungated*, and the
mutation survives a `MatchSignature` that returns nil — the repo's own
test asserts the marker expands after `sig == nil`
(`core/go/engine_stage5b_test.go:1088-1093`). Four lines earlier the
`SugarAngle` arm refuses the identical splice. Read quickly that is a
self-contradiction, and this note first recorded it as one.

Reading both comments properly, it is not. What the Angle arm refuses to
commit is a **choice**: the head/use-site form depends on the viable
overload set, and a scan running once per candidate must not decide it —
which is why the choice belongs at arrival. Every other sugar kind has a
single deterministic lowering, so expanding it early produces exactly the
tokens the next dispatch would have produced, and the mutation surviving
is harmless. Phase 1 gates all kinds because it is the phase that *makes*
the Angle choice and runs once per dispatch, where uniform gating is free.

The extraction still has to preserve this, but as a rule rather than a
repair: **a shared expansion step may commit any lowering that is a
function of the marker alone, and none that is a function of the viable
set.**

**Why this escapes the recorded "DO NOT RETRY".** Three attempts to
compile at the concrete-mismatch recovery site were differential-reverted
because "the recovery fires for reasons (forward-collection state, arity,
coercion) the param guard does not replicate"
(`design/VOXGIG-COMPILE-LEAVES.1.md:610-616`). Those attempts *committed a
bet* at compile time and guarded it with a check that lacked the
collection state; the descriptor mechanism exists to *carry* that state
and re-run the same routine — a replayed decision, not a guarded guess.
F2 is the falsifier if that distinction fails in practice.

**The extraction obligation.** The collection algorithm today reads *and
mutates* the tape (sugar expansion mid-scan, `engine.go:8731`;
`evalParenGroupAt` runs code in place). The routine must be factored to
operate over an abstract slot sequence — values, unevaluated fragments,
name tokens — with the Engine supplying a tape-backed adapter and the VM a
descriptor-backed one. This is the note's largest single work item and its
riskiest (§10 stages it first; §11 F1 falsifies on it). The prize is
double: the G-lane gains faithful collection, and the Engine's own loop
becomes a client of the shared routine — after which collection *cannot*
drift between lanes. The functional-derivation literature is explicit that
this factoring is where parity is cheapest (Ager, Biernacki, Danvy &
Midtgaard, BRICS RS-03-14, 2003): the VM's dynamic kernel should *be* the
interpreter's routines factored out, not a re-implementation.

Two consequences worth naming. First, the drift-window island
(`OpCallDynamicMixed`) and the mid-body/multi-arg chained-apply refusals
are all instances of "collection state missing at runtime" — they lower to
`OpCollect` forms and their islands are deleted. Second, `args`/`__pa` and
`context` (families K and the unledgered context-word gates) stop being
"context-dependent words": the descriptor's frame arms the existing DynEnv
args bracket (`eng/go/vm.go:450-483`) **per region** instead of
program-wide, and a context-frame opcode pair gives auto-evaluated bodies
their own layer exactly where the interpreter's sub-engine push did.

### 6.3 Universal fn values — open closure conversion

Every fn value the program can create carries, from creation, everything
application needs — so no apply site can lack a compiled target (Reynolds
1972's global-apply totality, realized as *open* closure conversion, not a
closed tag table, because runtime-compiled code must slot in; Minamide,
Morrisett & Harper, POPL 1996, is the formal warrant that one uniform
`∃env.(code × env)` convention covers every closure):

- **Executable ref per signature**: a `CompiledFn` unit, a native `Impl`,
  or a *lazily-compiled* body — creation stamps the body's tokens and
  defining registry; first application compiles via the detached-stamp path
  (`StampDetachedSig`) and patches, Factor's lazy-JIT-entry precedent. The
  dual-representation hook already exists (`SigImpl.BoruImpl.Compiled`,
  `core/go/sigimpl.go:56-61`).
- **Captures with homes**, including the currently-unsolved cross-registry
  case. Today a capture is a bare `{Name, Value}` snapshot
  (`CapturedBinding`, `core/go/value.go:882-885`; the capture rule is
  `ComputeCaptures`, `core/go/fn_capture.go:312`; closure operands resolve
  in `compiler/go/callable_words.go:470-480`). The *proposed* extension —
  nothing shipped does this yet — tags capture operands with their defining
  registry, completing the shipped halves (`CompiledFn.Reg` + `enterUnit`
  curReg swap, `compiler/go/bytecode.go:970`, `eng/go/vm.go:1403-1414`)
  and retiring the foreign-module island (family D's cross-module rows,
  the `filter A.big` working island).

  **Measured 2026-08-27, and the blocker is not the captures.** The
  `filter A.big` row compiles today to *one instruction* — `0000 FALLBACK
  b0 ; filter (nin=0)` — the whole call islanded, right answer by the
  mechanism the mission forbids. Its predicate has NO lexical captures at
  all: what it needs is the free word `lim` resolved in module A, not in
  the caller. So capture-tagging is not what unblocks this row.

  What actually blocks it is `foreignFnHome`
  (`compiler/go/callable_words.go:373`), which declines any fn whose
  `fd.Registry` differs from `r`, and the decline is correct as the code
  stands. **Probed 2026-08-27 for the exact reason, because "pass the other
  registry" looks like a parameter change and is not:** for the `A.big`
  case, `r.Check != fd.Registry.Check` *and*
  `r.Check.Emit != fd.Registry.Check.Emit`. A module sub-registry carries
  its OWN CheckState and its OWN EmitState.

  So the job is not "compile the body against `fd.Registry`" — that would
  record the unit into a different program's unit table and const pool,
  where the caller's `OpPushClosure` cannot reference it. The job is to
  **split the two roles the registry currently plays**: which registry
  RESOLVES names during body compilation, and which EmitState RECORDS the
  resulting unit. The unit must land in the caller's table; its dispatches
  must resolve in the foreign registry, at compile time as well as at run
  time.

  That split is the Phase-2 work, and it is worth stating separately from
  the capture extension because the two are independently useful:
  registry-threading retires the island for capture-free foreign
  predicates (the measured row), capture-tagging retires it for foreign
  closures that also capture.

  **The registry half alone is NOT sufficient, and a throwaway prototype
  proved it rather than arguing it (2026-08-27).** Threading `fd.Registry`
  through `compileClosureBody` while pointing ONLY the foreign
  `CheckState.Emit` at the caller's `EmitState` does produce a
  natively-compiled unit — the island goes away, `FALLBACK` disappears from
  the disassembly. The answer is wrong: `[[]]` against the interpreter's
  `[[3 4]]`, and the compiled body is

  ```
  fn f0 filter$body/1 (locals=1) [e]:
    0000 PUSH_CONST  k0   ; false (Boolean)
    0001 RET
  ```

  The predicate CONST-FOLDED to `false`, folding away the per-element
  parameter along with the free word — a silent wrong answer, which is
  exactly what `foreignFnHome` was there to prevent.

  **LANDED 2026-08-27; the fold was reading the wrong CheckState.** Pointing
  the foreign `Emit` at the caller is half a share: the body compiles under
  the FOREIGN module's carriers, params and recorder, so `e` — the caller's
  per-element param — has no carrier there and the analysis reaches a
  constant it has no business reaching. Sharing the WHOLE CheckState is the
  fix, and it is not new machinery: `shareCheckStateFrom` is what
  `execFnDefLiteral` already uses to run a module fn's body on the
  importer's engine, restore contract and nesting idempotence included. It
  is now exported as `check.ShareCheckStateFrom` and
  `compiler/go/callable_words.go`'s body-lambda path uses it, so the two
  roles land where §6.3 said they must:

  | role | registry | why |
  | --- | --- | --- |
  | BINDINGS | `fd.Registry` (foreign) | `lim` inside A's predicate must be A's `lim`; `StartFnCompile` on that registry stamps `CompiledFn.Reg`, and the VM's `curReg` swap carries it to run time |
  | ANALYSIS | caller's `CheckState` | params, carriers and the RECORDER — the unit must land in the CALLER's program, which is where `OpPushClosure` references it |

  Measured after: `filter A.big [1 2 3 4]` compiles with no `FALLBACK` and
  answers `[[3 4]]` on both lanes, with A's `lim` compiling as its own unit
  returning 2. The row graduated out of the frontier ledger into
  `lang/spec/module-fnvalue-boundary.tsv` §4.

  `foreignFnHome` therefore survives as a QUESTION every caller must answer
  — compile it in its own home, or decline — rather than as a blanket
  refusal. The extras/hook path (walk's ascend slot) still answers DECLINE:
  its shared-token-shape hooks have no per-hook registry to swap to.

  **The same question reaches MODULE-SCOPE MUTABLE CAPTURES, and asking it
  there exposed a SIXTH silent miscompile** — same family as NUR101's five,
  found the same way, by running the shape rather than reasoning about it.
  `moduleScopeMutableCaptures` rides a module-scope flex cell or class
  instance as a closure slot, and it was looking the name up in the CALLER.
  For a foreign body that is the wrong scope, and when both modules bind the
  name it silently swaps the cell:

  ```
  import module [def acc (flex [1 2 3]) def big fn [[e:Map] [Boolean]
    [(size acc) lt (e dot value)]] export "A" {big: big/v}] end
  def acc (flex []) filter A.big [1 2 3 4]

  interpreted  [[4]]           <- A's acc, size 3
  compiled     [[1 2 3 4]]     <- the CALLER's acc, size 0
  0007 PUSH_CLOSURE f0  ; closure filter$body/2   (capturing l0)
  ```

  The lookup now uses `fd.Registry` for a foreign body. A foreign cell has
  no producing event in the CALLER's emit tables, so `resolveOperand`
  declines and this row falls back — sound, parity restored, and ONE island
  remains where the shape needs a registry-tagged operand for a foreign
  module-scope instance. That is the follow-up the frontier ledger named;
  the parity fence is `lang/go`'s
  `TestForeignClosureCaptureResolvesInItsOwnRegistry`.

  Worth naming as method: the first version of this increment carried a
  REFUSAL for the shape (`len(captures) > len(fd.Captured)` → decline) with
  a comment saying no corpus row exercised it. Writing the row to cover the
  refusal is what found the miscompile underneath it. A guard whose
  justification is "nothing measures this" is a request to go measure it.

  The RUNTIME halves needed nothing, as recorded: `CompiledFn.Reg` is
  stamped for module-preamble fns (`compiler/go/emit.go:7771`) and the VM
  swaps `curReg` on unit entry (`eng/go/vm.go:1403-1414`). The
  cross-registry closure unit rides exactly that path.

  **The pattern, third instance in this stage.** A documented blocker turned
  out to be a different blocker once run. §6.4's "the interpreter is wrong"
  was the compiler's model of it; the closure-render refusal was actually
  guarding bare-name dispatch; and here "why does the body analysis fold at
  all" had nothing to do with folding under a swapped registry — the
  analysis was simply looking at a CheckState with no carrier for `e`. Each
  time, the prototype that RAN found it and the reasoning that preceded it
  did not.
- **Quote state and dispatch-control state** (`/q` polarity, sealed/applied
  bits) — so the quote-lambda screen (`check/go/check_fnbody.go:388`)
  becomes routing, not refusal: a `/q`-slot lambda delivered as a callback
  stays DATA, because the shared collection routine applies the same
  non-binding rule the runtime applies. The screen's *refusal* retires; its
  *knowledge* moves into the value.
- **Identity per the interpreter's semantics** — `eq` is identity token,
  `deq`/`canon` are content, module fn values box pointers (family B's
  entire premise is that these words only *read*; with a first-class
  compiled representation the Stage-3 gates delete). The byte-identity
  mandate makes the interpreter's current allocation/identity behavior the
  de-facto spec: compiled closures must reproduce it, and any future
  closure caching/sharing (Keep, Hearn & Dybvig, Scheme Workshop 2012)
  requires a Lua-5.2-style spec loosening *first* (§11, O3).

  **LANDED 2026-08-27 for `eq`/`neq`/`deq`/`canon`, and it did NOT need the
  universal representation.** The premise held — those words only read —
  but the conclusion attached the graduation to work that turned out to be
  unnecessary for it. `RecordCallOperands` refuses a fn-valued operand at
  any word that has not declared what it does with one; `eq`/`neq`/`deq`/
  `canon` had simply never declared. They now carry `CompileReadsFn`, the
  fn rides as an ordinary const operand, and twelve ledger rows graduated
  into `lang/spec/compare-restrict.tsv` and `lang/spec/fn-value.tsv` §10.

  The fact that makes it sound was measured, not assumed: **the identity
  token survives the const bake.** `FnDefInfo.ident` is a pointer minted
  once per authored function and copied by value, so an interned const
  carries it — `f/v eq g/v` is false for two identically-bodied functions
  and `a/v eq b/v` is true for two bindings of one, on both lanes. The
  graduated negatives pin both directions, plus rebinding, a container
  read, and callability after a comparison.

  **What this says about the family-B count.** Of §2.1's 22 rows, 12 were
  a missing declaration, not a representational gap. The remaining 8 are
  `is` against a predicate type, and those are genuinely this section's:
  `is`'s TYPE slot INVOKES the predicate through `RunPredicate` →
  `CallBoru` (its value slot is already `FnInertArgs`), so they need the
  predicate-body-as-unit work below, not a flag. Before attributing a
  refusal family to a representational change, check whether the word has
  merely failed to say what it does.

  **Family G's list `each` went the same way, 2026-08-27.** Two of the
  seven working-island rows — `each dbl/v [1 2 3]` and
  `each ([x:Integer] => [x add 1]) [1]` — were ledgered as needing "a
  modelled fn-value callback frame". They needed no frame. Tracing the
  decline showed the lowering never reached the frame question:
  `lambdaCallbackInputs` (`compiler/go/callable_words.go`) switches per
  word, had a MAP case for `each`, no LIST case, and fell through to its
  catch-all `ok=false` — the FIRST gate in `tryRecordLambdaClosure`. The
  list convention was then measured off the interpreter rather than
  inferred from the map twin:

  ```
  def show fn [[e:Any][Any][typeof e]]  each show/v [1 2 3]
    → [Integer Integer Integer]        one input, the bare element
  ```

  One `ClosureInValue` element carrier, and all of it compiles — including
  a heterogeneous list (the join carrier distributes per alternative), an
  empty list, and a capturing lambda. The mismatched-param case
  (`each ([x:String] => [x]) [1 2 3]`) still declines and both lanes
  agree, which is the check that the admission did not widen too far.

  **`fold`/`scan` stay islanded, and the reason is now located exactly —
  the ARITY-1 BOUNDARY.** They were attempted alongside `each` and the
  attempt failed usefully.

  A compiled closure binds its inputs POSITIONALLY: `invokeClosureOn`
  (`eng/go/vm.go`) fills the unit's leading param slots from the handler's
  `inputs` slice in order. The interpreter presents them in the CALLBACK
  CONVENTION for the container, and the two conventions differ:

  | form | sig[0] | sig[1] |
  | --- | --- | --- |
  | MAP fold/scan | ACCUMULATOR | entry (KeyVal) |
  | LIST fold/scan | ELEMENT | accumulator |

  Measured with AMBIGUOUS param types, so it is a real ordering convention
  and not a by-type assignment that merely looks like one:
  `fold ([x:Any y:Any] => [x]) {a:1} 0` answers the seed, while
  `fold ([x:Any y:Any] => [x]) [7] 0` answers the element. The MAP form
  already agrees on both lanes — ambiguous types included — because the
  handler's input order happens to match its convention.

  **With one input no convention can disagree**, which is why list `each`
  compiles. With two, the list form's do:

  ```
  fold ([e:Integer a:Integer] => [a mul 10 add e]) [1 2 3] 0
    interpreted  123      a accumulates: 0 → 1 → 12 → 123
    positional    60      the pair arrives swapped
  ```

  Nine shapes diverged that way. Supplying the carriers in the other order
  changed NOTHING, and that is the most useful thing measured: carriers
  only TYPE the body. The unit's param slots come from the LAMBDA's own
  declared order, and what lands in each is the handler's push order — so
  both carrier spellings produced 60.

  Admitting a 2-input list callback therefore means making the two orders
  agree at the seam that actually decides it: the handler's input order for
  that form, or a per-word permutation at the closure bind. That is the
  "modelled fn-value callback frame" the ledger asked for, and the ledger
  was right about `fold`/`scan` in exactly the way it was wrong about
  `each`. Fence: `lang/go`'s `TestListFoldCallbackOrderFence`.

  **Two probe lessons, each of which bought a wrong admission.**

  1. **A typed probe cannot establish positional order**, because the
     matcher reassigns by type. `fn [[a:Integer b:String]…] fold f/v
     [1 2 3] 'seed'` matching, and its swap not matching, reads like "`a`
     is the element" and proves nothing of the kind — the matcher put the
     Integer in whichever slot was declared Integer. Use SAME-TYPED or
     `Any` params, or make no positional claim. The first reading of this
     convention was built on that probe and was wrong; the correction then
     over-shot the other way, asserting the accumulator is `sig[0]` for
     BOTH containers, which the ambiguous-type probe above disproves. Both
     errors came from reasoning about the matcher instead of running a body
     that reports which argument it received.
  2. **The accumulator-stability guard was unnecessary.** The
     accumulator's runtime type after step 1 is the BODY'S RETURN, not the
     seed's, which looked like it needed a guard. It does not: the body
     binds the accumulator at its DECLARED PARAM type, not the carrier's
     tag. `fold ([acc:Scalar kv:KeyVal] => [if (acc is Integer) ['I']
     ['S']]) {a:1 b:2 c:3} 0` answers `'S'` on BOTH lanes — the
     accumulator really does go Integer → String mid-fold and the compiled
     body sees it — and `typeof acc` answers `Scalar` on both. A step whose
     accumulator fails the declared param no-matches identically in both
     lanes, and an OVERLOADED callback is already refused
     (`lambdaHookCompatible` wants exactly one own signature). Written from
     reasoning alone, that guard would have refused valid programs for a
     hazard that is not there.

  The pair is worth stating together: **the guard I was going to add was
  not needed, and the blocker I thought was absent was real.** Both were
  settled by running the shapes, and neither by reading the code.
- **Partial applications as data**: `curryOrStack`'s curry list gets a
  compiled twin — a curried value is `(base fn, held args)`, applied by the
  shared apply; no per-composition codegen (Factor's `curried`/`composed`
  cells).
- **Predicate and membership-type bodies join the same inventory.** Today
  `v.Is(t)` on a predicate/DepScalar type runs the predicate's boru body
  through `Registry.RunPredicate` → `CallBoru` — a tape run *inside the
  matcher*, reachable from every matching site including the VM's
  (`core/go/unify_predicate.go:21`, `core/go/membership.go:3-6`). Under
  the universal representation a predicate body carries a lazily-stamped
  unit like any other fn value, and `RunPredicate` executes it through the
  VM invoker, mid-match raise semantics preserved. This is the
  registry-aware matching that doctrine item R3 deferred to "a separate
  later design, not assumed" — this section is that design (§3.2).

  **MEASURED 2026-08-27, and the measurement stopped a graduation that
  would have been laundering.** Family B's remaining 8 rows
  (`function value reaches is (Stage 3)`) compile the moment
  `RecordCallOperands` stops refusing a predicate-type operand: all ten
  shapes probed — both `fnpred` spellings, the `None`-signalling body, the
  input-type gate, a predicate reaching a free word — answer correctly on
  BOTH lanes with no `FALLBACK` in the disassembly. A predicate is exactly
  ONE input, so the arity-1 boundary that blocks `fold`/`scan` does not
  apply. It reads like a clean graduation.

  It is not one, and a program-level `FALLBACK` check cannot tell:
  **`OpFallback` is invisible to a `CallBoru` made INSIDE a native
  handler**, which is precisely where the predicate runs. `RunPredicate`
  reaches the body through `InvokeCallbackFn`, whose contract is "the VM
  when the body compiled to a unit, `CallBoru` otherwise". Measured
  through the `InterpEntry` hook (Stage 1's own instrumentation, built for
  this question), the body is NEVER a unit — every predicate invocation
  takes the interpreter arm, emitting `InvokeCallback:callboru` +
  `CallBoru`, both UNATTRIBUTED.

  So relaxing the gate would trade an honest whole-program refusal for a
  compiled program with an interpreter island hidden inside a handler —
  the one outcome the directive rules out, and worse than the refusal
  because it removes the row from the census while the interpretation
  stays. The refusal stands until predicate bodies compile to units.

  **The control is the part that matters beyond this decision.** A
  predicate reached through ORDINARY DISPATCH — `def f fn [[x:Pos] …]  f 5`
  — takes the same interpreter arm today, with no `is`, no gate, and no
  ledger row. The same holds at a typed def. So this is **PRE-EXISTING
  live-island debt in compiled programs**, not something the gate
  prevents: the gate only keeps the census from under-reporting it. It
  belongs with T2's "live island residue" debt in §2.3, and it is a
  second, independent reason predicate-body units are Stage-3 work rather
  than a flag. Pinned by `lang/go`'s
  `TestPredicateBodyRunsOnInterpreter`, which fails — with the graduation
  instructions — once the bodies do compile.

  **RESOLVED 2026-08-28 — and the stated CAUSE above was wrong.** "The body
  is never a unit" was an inference from the seam, not a reading of it. The
  bodies compiled. `def Positive fn […]` stamped a detached unit at type
  construction (`core/go/core_type.go` → `CompiledRuntime.StampDetached` →
  `StampDetachedFn`) and recorded `Stamped:true` in the stamp ledger —
  `-compile-report` said compiled, the runtime interpreted, and the two
  had no shared gate to disagree at.

  The decline was one line, `eng/go/vm.go`'s `runUnitNested`:

  ```go
  if !ok || ref.Prog != vc.p { return nil, false, nil }
  ```

  A DETACHED ref is compiled on an isolated `ForkConcurrent` into its own
  standalone one-unit `Program` — that is what makes it detached — so
  `ref.Prog != vc.p` is true for every one of them, always. The mid-run
  nested path therefore rejected the entire class by construction, while
  `InvokeCallback`'s contract read "the VM when it compiled to a unit
  (nested in a live run, or fresh on an idle registry)". Only the idle half
  was ever real: a callback firing AFTER `RunProgram` returned (serve-raw
  on a connection, a spawned process) found `CanHostVM()` true and took
  `RunUnit`; a callback reached from INSIDE a live run found it false, fell
  to the nested runner, and was declined. Predicates are the second kind,
  every time.

  The fix is `eng/go/vm_foreign_unit.go`: host the foreign program's unit
  in a nested `vmContext` bound to ITS program instead of declining it. Four
  pieces of state stay shared with the enclosing run because each is a
  runaway guard a per-callback reset would defeat — `steps` (copied in,
  handed back), `frameDepth` (seeded, so recursion THROUGH a detached
  callback hits the same ceiling and fails as `tape_exhausted` rather than
  overflowing the Go stack), `ceiling` and `stepLimit`. Three are
  deliberately NOT shared: `dynBinds`, `islandEng` (the enclosing island
  engine may be mid-flight — exactly the re-entrancy its contract forbids),
  and `foreignInvokers`. The panic guard is local rather than borrowed from
  the enclosing `runVMEntry`, so a bailout inside one callback degrades
  THAT callback to `CallBoru` through `InvokeCompiled`'s C1 fence instead
  of aborting the whole enclosing program.

  Measured, corpus-wide: unattributed interpreter-entry rows **184 → 163**,
  `InvokeCallback:callboru` **28 → 4**, and `Engine.Run` and `CallBoru` each
  −24 — the three fell together because one declined predicate dragged all
  three seams behind it. `TestSpecCompiledDifferential` is unchanged: every
  row that moved onto the VM still answers what `RunInterp` answers. The
  pin flipped to `TestPredicateBodyRunsOnTheVM` and now fails if the
  interpreter arm comes back.

  **List `fold`/`scan`: the same shape, one seam lower (2026-08-28).** The
  ledger asked for "a modelled fn-value callback frame" and §6.3 read the
  divergence as an arity boundary — one input cannot disagree, two can. Both
  descriptions were downstream of the real mechanism, which is neither arity
  nor carriers: **both handlers hand `(accumulator, element)`**, and the two
  engines assign it differently because the MAP path calls the lambda
  POSITIONALLY (`mapBody.callLambda` → `CallBoruFn`, args in sig order) while
  the LIST path goes through `InvokeBody`, whose interpreter arm runs the
  inputs as a STACK (`RunResolved`) where `MatchSignature` fills top-down.
  Same order in, opposite assignment out.

  So the fix is ONE per-word permutation at the closure bind
  (`ClosureInStackPair`), not a frame. Five ledger rows graduate into
  `lang/spec/higher-order.tsv` §12 with four `Any`-param rows that report the
  convention directly.

  Worth naming precisely: these rows never REFUSED. They **islanded** — the
  right answer, produced by the interpreter, inside a program the coverage
  metric calls compiled. That is why the graduated pin asserts the
  DISASSEMBLY: its parity half passed for the entire time the island existed,
  so a value-only assertion here proves nothing and would keep passing the day
  someone re-islands the callback.

  **The Apply kernel, first increment (2026-08-28).** §6.4 asks for "one
  dedicated, arity-carrying Apply kernel routine, seamed where the interpreter
  applies". What was there instead: the dynamic-apply opcodes islanded their
  callee through a sub-engine, because a runtime fn value carrying no compiled
  unit has nothing else to run. **75 corpus rows do that**, in programs whose
  disassembly says `fallbacks=0` and which `TestCompiledCoverage` reports as
  "0 islanded" — the same blindness the predicate seam had, one opcode family
  over.

  Two halves, and each was got wrong once first.

  **The unit has to ride the VALUE.** A dynamic apply's callee is unknown at
  compile time by construction, so `stampFnConst` compiles a fn-value CONST's
  body to a unit at the const chokepoint (`resolveOperand`). In-program via
  `compileStoredFnUnit`, not `StampDetachedSig`: the detached form isolates a
  refusal neatly and costs a `ForkConcurrent` plus a full compile pass PER fn
  const — measured at 6x on `lang/go/modules` and a transient OOM. The refusal
  the cheap form exposes is handled where it belongs: `fnUnitRec.stampOnly`
  makes a stamp unit's Finalize refusal produce a trap stub and a dropped ref
  instead of refusing the program, because a stamp is an optimisation and must
  never be able to refuse anything.

  **A fn application is a CALL, not a body.** `enterBodyUnit` brackets a
  per-body context frame for the three seams that re-enter the VM; `OpCallUser`
  is deliberately not among them, and neither is the interpreter's own
  `execFnDefLiteral`. Routing the apply through the callback seam therefore
  added a frame the interpreter does not, and it showed up as a **silent wrong
  answer** — `TestContextBoundaryDifferential/paren-grouped_map-slot_lambda_method`,
  a `context set` escaping on one engine and not the other. The kernel now
  models `OpCallUserPoly`, the existing precedent for entering a unit the loop
  only learns at RUN time: match, pop args into frame locals, re-check the
  param contract, push a frame. `dynApplyEnter` hands that decision back to the
  loop (`*dynEnter`) because frames, locals and pc are the loop's.

  Three declines are load-bearing rather than defensive. A **QUOTED** fn is
  data — the island got that free from the tape, and a direct frame push does
  not (`((mk) 2)` answered `[3]` against the interpreter's `[fn (Integer) 2]`).
  A **cross-program** ref cannot be a frame of this program. A **shape
  mismatch** between the matched sig and its unit is compile/run drift.

  Two collisions the const chokepoint exposed, both of which narrowed the
  increment honestly. Its analysis leaks diagnostics into the enclosing
  program, and an error diagnostic REFUSES it — `TruncateDiagnostics` drops
  exactly what the attempt added, since a speculative compile's findings are
  not the program's. And descending into containers first read as OUT, on the
  grounds that stamping a fn nested in a map const mutates the shared
  `*BoruImpl` of a value inside USER DATA — precisely what `StampFnValue`'s
  cloning form exists to avoid. The evidence was a model action's `{gen: …}`
  stamped here first, after which `stampActionFn`'s clone-and-name found a ref,
  declined, and the stamp report lost its attribution.

  That reading of the clone contract was wrong, and it is the fourth documented
  blocker in this work whose stated CAUSE named the wrong lane while the
  symptom was real (§6.4, NUR101 and §7's "the body is never a unit" are the
  others). The clone protects a CALLER's input from the model module's stamp;
  it says nothing about the compile pass stamping a const it baked itself. What
  actually broke was ATTRIBUTION — one report line, not one semantics — and it
  is fixed where it was lost: `StampFnValue`'s already-stamped early return now
  records the event before returning. Descent landed. The next section is what
  it cost.

  Measured across the increments: census **184 → 131**, `vm:island` **66 → 39**,
  `vm:island-resolved` **21 → 10**, corpus 7622 rows / 7248 compiled / 0
  islanded / 0 refused, differential unchanged throughout.

  **Container descent, and the mask it nearly shipped.** Of the island rows
  the top-level stamp leaves, roughly four in five are a fn read out of a
  container — `def m {f: (fn …)}  m.f 5`, `def ops {f: inc/v}  ops.f 5`, a
  class field method. Descending into list and map consts takes them (census
  141 → 131). It also made three variation-sampler buckets REFUSE that
  previously compiled (pass 403 → 401, refused 33 → 36), and their names
  located the cause precisely: "dynamic-scope def `files` of unpromoted
  computed value" and "module binding files rebound after a stored handler
  captured it as a dep" both indict the ENCLOSING compile, not the stamped
  body. `compileStoredFnUnit` analyses against the LIVE emit state, so it
  leaks through two channels past the diagnostics `TruncateDiagnostics`
  already restores: `dynScopeNames`, whose entries make Finalize install an
  `OpBindDynScope` twin in every binding unit — so the enclosing program's own
  `def` must now lower a dynamic bind it may have no promoted value for — and
  `storedHandlerDeps`, whose records make a later rebind of that name refuse
  the whole program through `NotifyNameRebound`.

  The first draft ledgered both buckets and argued the trade was worth it:
  WITHOUT the descent the same seed **miscompiles** — a flex map captured by a
  mount handler loses its identity across loop iterations, and the compiled run
  raises `expected a FlexMap, got FlexMap` where the interpreter round-trips —
  and a refusal is a program that does not run, while a miscompile is a program
  that runs and lies.

  **That argument was wrong, and its wrongness is the lesson.** The descent was
  not FIXING the flex-map divergence; it was MASKING it. A stamp is an
  OPTIMISATION: it declines for a capturing fn, for a body whose lowering
  refuses, for a body needing a dynamic-scope rescue, and whenever runtime
  stamping is unarmed. A wrong answer that is correct only while an
  optimisation happens to apply is a wrong answer waiting to come back — and it
  would come back SILENTLY, because nothing in the gate ties that seed's
  correctness to the stamp firing. Graduating the pin on that basis would have
  deleted the only record that the defect exists.

  So both channels are closed at the source and the pin is restored:

  - `dynScopeNames` is snapshotted and restored, and a stamp that added a name
    is DECLINED. A successful stamp genuinely reads through `OpLookupDynScope`,
    so the name cannot be dropped under a live ref; the rule is instead that a
    stamp which would change the enclosing program's own lowering is not worth
    taking. Declining leaves the apply island — exactly the behaviour the
    differential already validates.
  - A stamp ref is marked `optional`. `NotifyNameRebound` still poisons it (the
    apply islands, as before the stamp existed) but no longer escalates to the
    program-level refusal. That escalation is right for a STORE-SITE ref, whose
    `CallBoru` fallback would read the pass-final binding; for an optimisation
    ref, islanding IS the whole remedy.

  Result: pass 403, refused 33 — the pre-descent numbers, both buckets gone,
  their frontier rows deleted — with the census increment kept, and the
  flex-map divergence pinned where divergences belong rather than hidden behind
  an optimisation that happened to fire.

  **The descent's own blind spots, found by finishing the cluster.** The
  paragraph above names three shapes and the descent takes two of them; the
  third — "a class field method" — it never reached, and neither did it reach a
  fn behind a MODIFIER WRAPPER. Both are containers the walk does not recognise
  as containers, and each needs a different key:

  - A class type carries its methods as FIELD DEFAULTS, and the type operand
    leaves `resolveOperand` BEFORE the const chokepoint's stamp. The node there
    is BARE (`IsBareTypeNode` means `Data == nil`), so the schema is not on the
    value: it has to be resolved the way `make` resolves it, through
    `ResolveTypeLiteralDef` against the live registry, and read through
    `AllFields` so an inherited default is reached through the parent chain.
  - A modifier wrapper (`usurp`, `stack-args`, `forward-args`, `force-arity`)
    is a container of exactly one fn. It REBUILDS the value with a Go handler on
    every own sig, so `storedSigEligible` refuses them all; the boru body sits
    one level down, behind `FnDefInfo.Wraps`.

  The wrapper case settles a question the apply site could not. `def sub2 fn […]
  def ops {rev: (usurp sub2)}` disassembles to `fns=0`: the map const folds the
  wrapper in, and nothing in the program compiles `sub2`'s body, because `sub2`
  is never called by name either. No widening of the apply gate could have taken
  that row — there was no unit to enter. Reading the bytecode said so in one
  line; the seam name (`vm:island`) had suggested a dispatch-site problem for
  two increments.

  Which is the correction worth keeping: the five rows left at census 36 were
  filed as "one cause". Probing the decline arms one row at a time gives FOUR —
  two stamp-reach holes (these), one deliberate `MatchFnSig` decline on a
  genuine type mismatch, and one `CALL_DYNAMIC_MIXED` window that needs the
  split rule and therefore Stage 5. A shared symptom is not a shared cause, and
  a seam name is not a diagnosis.

  **The native-callback seam, and what the CallBoru column turned out to be.**
  `core/go/invoke.go` carried two near-identical entries: `InvokeCallbackFn`
  (offer the body to the VM, fall back to `CallBoru`) and `CallBoruFn` (the same
  defining-registry routing, straight to `CallBoru`). Seven words used the
  second — `filter`'s Function form, the map-lambda `each`/`fold` bodies, core
  `walk` / `StructUtil.walk`, `IO.mount`'s fileops handlers, `boru:parse`'s
  matcher and action callbacks, and the fn-util words. Its comment gave the
  reason: *fixing WHERE free words resolve must not also change WHICH engine
  resolves them.* That is the right scope fence for
  design/FUNCTION-VALUE-SCOPE.0.md's change and precisely the fence this work
  removes, so all seven now call `InvokeCallbackFn` and `CallBoruFn` is deleted
  — a second dispatch path is a divergence source in its own right, and
  design/FN-VALUE-OPEN-WORK.0.md records two it caused.

  The measurement is the interesting half. `CallBoru` had sat at **251** through
  four consecutive fixes — visibly the largest block of interpretation debt in
  the census, and the reason this increment was picked next. The flip moved it
  to 238 and the row count by exactly ONE. So the 224 that remain were probed:

  | origin | entries |
  |---|---|
  | `Test.property`'s generator/property calls | 224 |
  | `core_helpers` foreign-registry fn dispatch | 8 |
  | `InvokeCallback`'s own interpreter fallback | 6 |

  It was never broad debt. It is two or three `module-test.tsv` rows amplified
  by an iteration count — `Test.property` runs its bodies ~100 times per row,
  the seam counts INVOCATIONS, and the ceiling counts ROWS. Those bodies are raw
  QUOTATIONS: the module already routes a compiled sig through `InvokeCallback`
  and only falls back when the property was written as `[body]` tokens, which
  carry no unit to offer. Compiling them is Stage 7's runtime-compilation work,
  not a seam flip, and it is worth perhaps three rows.

  **The lesson generalises past this seam.** A census whose counts mix per-row
  and per-invocation events cannot be read as a priority order — a large number
  can be one row in a loop, and a small one can be a whole family. The row count
  is the ratchet for exactly that reason; the seam spread is a shape, and every
  seam worth acting on has to be attributed before it is worth acting on.

  **The Apply kernel reaches LENSES, and the attribution is what found it.**
  A `Reach` is a callable value exactly as a fn value is — `p $.name apply`,
  `each $.name people`, `filter $.on xs`, `ArrayUtil.sortby $.age people`,
  `StructUtil.getpath $.a.b m` — and every one of them funnels through a single
  primitive, `core.ApplyReach`, which lowered the lens to a `[recv dot key …]`
  token chain and ran it on a pooled sub-engine. Per application. That is an
  interpreter entry inside a native handler: the exact shape the OpFallback
  ceiling cannot see, since these programs disassemble with `fallbacks=0`.

  It was picked by reading the per-file table, not by guessing: `reach.tsv`, 14
  rows, every one through `runPooledSub` and nothing else. A probe confirmed all
  14 reached `ApplyReach` before a line was written.

  **Writing a second walker in Go was the wrong fix, and the codebase says so
  in its own voice.** `getpathReachHandler` routes here deliberately — "so
  per-segment getr strictness and computed keys behave exactly as bare `m.a.b`
  — the same primitive `apply` uses". One dispatch path is the rule. So the
  path stays and moves onto the VM instead.

  The chain is already a one-parameter function body: bind the receiver, run the
  dots. So the unit is compiled from *exactly the tokens `ApplyReach` would
  otherwise have interpreted*, with the receiver value replaced by a reference
  to the bound parameter — no hand-lowering, no second model of what a segment
  means. Everything downstream (dep freshness, the JIT re-stamp, the effect
  fence, the internal-error degrade) is the `CompiledRuntime` seam's, unchanged.

  The unit is cached on the Reach PAYLOAD, as a pointer field, which is the same
  trick `*BoruImpl` plays for a signature's compiled ref: every copy of the value
  shares one cache, so a lens in `each $.name people` compiles once rather than
  per element. That mattered — a per-application stamp is a fork and a whole
  compile pass, the cost that sank the first per-const stamping attempt outright.
  Canon renders a Reach from its `Segments`, so the field takes no part in
  equality or serialisation.

  One shape differs from the interpreted chain: the receiver arrives as a NAMED
  parameter rather than a stack push. That was settled by measurement rather
  than argument — the whole corpus, the spec differential, the variation sweep
  and the frontier ledgers are unchanged. **Census 130 → 114**, `runPooledSub`
  36 → 11, `reach.tsv` gone from the cluster table entirely; `path-modifier.tsv`
  now leads it at 13 rows, every one through `vm:island`.

  **And it split a ratchet, which is the more useful finding.** The defer census
  rose 5 → 6: `5 $.name apply` — a lens on a receiver its first segment cannot
  read — reaches `CALL_NATIVE_POLY` with no match and the VM defers. That census
  is monotone-DOWN by rule, so the increment could not simply land.

  The rule is right and the instrument was imprecise. `deferCeiling`'s own prose
  says what it counts: a bail "resolving it by asking the caller to re-run the
  whole program on the interpreter". Measured, all five of its bails do exactly
  that — the row returns `wasCompiled=false`. This one does not: `ApplyReach`
  has a complete fallback of its own, catches the `internal_error`, runs the
  interpreted chain for that ONE application, and the program finishes
  **compiled**. Two different events under one number.

  So the walk now sorts each bail by what happened to its row. `deferCeiling`
  keeps its exact meaning and its exact number — **5, unchanged** — and the
  locally-resolved kind ratchets separately from 1. That is not relaxing a gate
  to fit a change; the strict number is untouched and is now measured precisely
  instead of approximately, and the weaker event is counted rather than hidden
  inside it.

  Why the VM cannot raise natively here: the site has a designed native-raise
  arm, gated on a `PolyNoMatchSpec` — a faithfulness proof the check pass
  records only at a dispatch it actually watched FAIL. A lens body is analysed
  with an `Any` receiver, so its dispatch never fails at check time and no proof
  exists to record. Both counts reach 0 at Stage 9 by the same work, not by a
  lens-specific fix.

  **A note on what the coverage gate found.** After the kernel took every
  reachable shape, `callDynApplyTop`'s ISLAND success return stopped being
  covered — fourteen probed sources (usurp-built values, runtime-returned fns,
  bodies whose stamp should decline) all took the compiled path. It is pinned
  with a constructed decline rather than a `//covergate:allow`: "no program I
  could write reaches this" is the same reasoning that hid a silent miscompile
  earlier in this work, and it is not a proof of unreachability.

  **The graduation, landed and PROVEN honest.** With the body on the VM the
  refusal in `RecordCallOperands` had nothing left to defend, and the arm
  that refused a predicate-type OPERAND is gone. What the first attempt
  lacked was not a better argument but a detector: a program-level
  `FALLBACK` check cannot see a `CallBoru` inside a handler, so "it compiles
  and answers correctly" was compatible with an island. The interp-entry
  census is that detector, and it is decisive here — **10 rows joined the
  compiled corpus (7603 → 7613, compiled 7229 → 7239) and the census did not
  move: 163 before, 163 after.** Had any of the new rows interpreted its
  predicate, the count could only have risen. Two shapes graduated that the
  frontier file never carried: the input-type gate (`"x" is Even` → `false`
  without running the body) and the type literal itself
  (`Even is Even` → `true`).

  The ledger's stated cause was wrong in the same way §6.4's and NUR101's
  were: "a predicate type's constraint IS a function value, so `is` walks
  that value into the Stage 3 gate". It does not. A predicate NODE is a bare
  type literal that rides as DATA; the body runs through the callback seam,
  never the tape, so the re-step the gate defends against was never in play.
  The gate was right by accident, for a reason nobody had written down.

  **The hazard the fix widened, closed with it.** `ClosurePayload` carried a
  `Unit` and no program. That was sound for as long as a closure could only
  be invoked under the program that pushed it — which was the case exactly
  because foreign units never ran nested. Hosting them ends it: while a
  detached unit runs, the registry's `Invoker` points at the FOREIGN
  program's context, so a closure belonging to the enclosing one would index
  the wrong `Fns` table. Out of range degrades (the local panic guard); a
  VALID index naming a different body is a silent wrong answer, which is the
  class this work exists to eliminate. The payload now carries its program
  (`ClosurePayload.Prog`, boxed as `any` — core sits below compiler, the same
  shape `BoruImpl.Compiled` uses) and `invokeClosureOn` routes by it through
  the same nested hosting. Reachability today is narrow — it needs a stamped
  body that also invokes an externally-supplied fn value, which Stage 3 has
  not enabled — so this is closed BEFORE Stage 3 opens it rather than after.

  **The pattern, ninth instance.** A documented blocker is a hypothesis
  until RUN. The verdict here was right — relaxing the `is` gate on top of
  a hidden island would have been laundering — and the reason given for it
  was wrong, which made the remedy look like "Stage 3 representation work"
  when it was a four-line predicate in the nested runner. Reading the seam
  the measurement blamed, rather than trusting the measurement's account of
  it, is what separated the two. The graduation the pin names — admit a
  predicate-type operand in `RecordCallOperands`, move the 8 `is` rows into
  `lang/spec/fnpred.tsv` — is now unblocked on its stated condition.

With this representation, family A's "unknown provenance / closure shape
unknown" dissolves for the *value* half: an unknown fn value is not a hole
in the trace, it is a runtime value with a universal calling convention.
The checker's carrier for it is the typed existential — dynamic as to
shape, concrete as to "applicable".

### 6.4 The Apply operation and the NUR101 application model

One dedicated, arity-carrying **Apply** kernel routine, seamed where
`execFnDefLiteral` sits today (`core/go/engine.go:5092`), used by both
lanes (the maintainer-ruled "dedicated Apply Op" — NUR077, `NUR.md:80`).
Its contract implements the application model that NUR101 established:

- **Name-read lead** (`def k (FnUtil.const 7)` … `k 99`, `(k 99)`): WORD
  dispatch — the runtime binding always applies. Lowering: `OpDispatchGeneric`
  through the live def stack (§6.5). Interp answer `7`; the compiled lane
  must produce `7` by *doing the same lookup*, not by baking the value.
- **Anonymous/event value in value position**: data unless applied by the
  placement rules; a named Go-impl fn value stays data. Lowering: push; the
  descriptor's placement facts decide.
- **Computed groups** (`(mk 1) 2`): **CORRECTED 2026-08-27 — this bullet
  had it backwards in both halves, and the correction is load-bearing for
  the rest of the stage.** It read: the compiled lane is already the
  specified one, the interpreter is wrong, and "no compiler change is
  wanted". Measured against `RunInterp`, the interpreter was right and the
  compiler carried FIVE silent miscompiles, in both directions
  (design/PAREN-RESTEP-RULE.0.md). NUR101 was fixed compiler-side only, and
  the interpreter ended that work byte-identical to where it started.

  The rule, and Apply must implement it exactly: a Function a paren PLACED
  is re-stepped into a CALL exactly when it leads **two or more survivors**
  of an enclosing group that closes with a paren rewind — a user paren, an
  fn frame, or an `if` / `for` / `do` body. The program top level, list
  literals and map literals do not rewind. So `(mk 1) 2` places
  (`fn (Integer) 2`) and `((mk 1) 2)` applies (`3`); placement is the
  one-survivor case, and the enclosing group is a SECOND decision taken one
  paren out, not a context that modifies the first.

  The two decisions are recorded at the collapse — `ParenPlacedFnIDs` and
  `ParenReSteppedFnIDs` (`core/go/check_state.go`) — because the paren
  structure is erased before the residual lowering runs, which is why
  `resolveDynamicApply` had to guess and guessed wrong both ways. **Apply
  inherits that constraint**: it cannot re-derive place-vs-call from the
  residual it is handed, so the descriptor must carry the fact. Four shapes
  refuse today for exactly this reason (a paren-bounded carrier apply
  consumed where the residual lowering does not reach), and they are this
  stage's to graduate: `RecordDynApply` must admit an EVENT lead, which
  `DynApplyLeadEligible` declines.

  The doc-level consequence, also corrected: T3's oracle is **the
  interpreter as it is**, not "the interpreter once NUR101 lands". Family J
  and the affected parts of A need no interpreter change to graduate.
- **Curried chains stage**: each application step is its own Apply event
  (the intermediate closure is a first-class value), eliminating
  "miscompile mechanism E" (`emit.go:7067-7076`) structurally.
- **Under-application and count mismatches** follow the interpreter's own
  rules (`(1 2 (mk 4))` → `1 6`; return-count trims and their exact error
  taxonomy) because Apply calls the same `CallBoru` return-enforcement
  helpers (`core/go/registry.go:1585-1752`)

  **LANDED 2026-08-28, ahead of the Apply kernel and without it.** Two
  things had to be measured first, and both moved the target.

  1. **§6.4's own refusal inventory was stale.** This section said "four
     shapes refuse today … `RecordDynApply` must admit an EVENT lead,
     which `DynApplyLeadEligible` declines". Enumerated against
     `RunInterp`, the EVENT-lead shapes (`((mk 1) 2)`, `(mk 1) 2`, the
     def-bound and trailing spellings, the curried chain) ALL COMPILE —
     increments 2–3 graduated them. **Exactly one** Apply shape refused:
     the wider window, `(1 2 (mk 4))`.
  2. **A SEVENTH silent miscompile sat next to it**, unledgered, found by
     probing the family rather than the refusal. `(9 1 2 add2/v)` — a
     CONCRETE 2-arg callee under a 3-wide window — answered `[3, 9]`
     compiled against the interpreter's `[9, 3]`. The survivor came out
     ABOVE the result instead of below it. The event path had an arity
     gate; the concrete path had none at all, so nothing was watching.

  Both are one defect: the lowered apply consumed the WHOLE window
  regardless of what the callee takes. The fix is to consume the callee's
  own arity and leave the rest, which is what the interpreter does —
  `RecordDynApply` now reports how many window values it CONSUMED, and the
  collapse site removes only that suffix of the window. The arity is sound
  by construction (`producerReturnedClosureArity` answers only for a unit
  with exactly one closure out-op; a single-sig concrete callee otherwise),
  so a branch-varying factory or an overloaded callee never reaches the
  trim — they decline earlier or here.

  The NARROWER window is the opposite shape and still refuses: the
  interpreter leaves the fn UNAPPLIED (`(5 (mk2 10))` → `[5, fn]`) and
  nothing models that. **That refusal must MARK, not merely decline** —
  measured, a quiet decline let the collapse site's `RegisterTrailingApply`
  fallback lower the window anyway and answer a silent `15`. Both arms
  share ONE `MarkUncompilable` site on purpose: the refusal-site census is
  a downward ratchet, and it caught the second site immediately (97 against
  a ceiling of 96) even though the shapes refused are strictly fewer.

  Four rows graduate into `lang/spec/fn-value.tsv` §11, including the
  concrete-callee ordering row that had no ledger entry because nothing
  knew it was wrong. — with `CallBoru`'s internal
  tape run replaced by unit entry for compiled bodies (it is already
  tape-free at its *interface*; the inside migrates per §6.8). Fidelity
  cuts both ways: Apply reproduces `execFnDefLiteral`'s landing rule *as
  it is*, asymmetries included — the foreign-frame path is trim-only,
  count never enforced (the documented frame-path asymmetry,
  `eng/go/vm.go:2316-2344`) — so the replacement for RetReplay never
  raises where the interpreter trims.
- **Tail position is part of the contract.** The interpreter runs
  constant-space loops through dynamically-dispatched callees; a G-lane
  Apply that entered every unit by nested VM activation (`runUnitNested`,
  `eng/go/vm.go:397-410`, Go-stack re-entrant) would grow a frame per
  iteration — an unbounded-loop divergence in exactly the programs
  totality admits. The descriptor marks tail position; Apply in tail
  context enters units by frame replacement (the `OpTailCallUser`
  discipline) or by trampolining through the dispatch loop; a
  deep-tail-loop compiled-vs-interpreted gate joins §9's parity suite.

`RetReplay`, `OpCallDynFrame`, and the `callDynamic` island arms retire
onto this routine: the body-tail window becomes an ordinary Apply on a
universal value; the "runtime decides applicability" rule is Apply's first
branch (a non-callable value stays data — faithful either way, as
`OpCallDynamic` already is).

### 6.5 Generic word dispatch, rebinding, and the def twins

`OpDispatchGeneric`'s lookup half honors hot rebinding *by construction*:
it reads the same `DefTable` stacks the interpreter reads
(`core/go/registry.go:1095-1243`), at dispatch time. No world-age epoch, no
semantic change — boru's interpreter sees a rebind at the next dispatch,
so the compiled lane does too, because it performs the lookup. The
per-name `DefTable.Gen` dispatch cache (`registry.go:1070-1088`) is the
inline-cache seed (§7).

For this to be sound inside compiled programs, VM-time definition order
must be real: the **bind twins** already named in the recorder's interim
comments (`emit.go:2460/2474` — "until the §5.6 bind twins make VM-time
def order real"). Every check-mode `def`/`undef`/module install that
affects runtime-visible bindings emits a twin op so the VM's registry state
at instruction *i* equals the interpreter's tape state at the corresponding
token. This was expected to delete the unledgered **rebind-staleness gates** — the
frozen-read refusals (`NoteFrozenRead`/`NotifyNameRebound`) and the interim
stored-handler latches (`emit.go:2460-2478`) — family L (conditional fn
shadow: the refusal site is `core/go/core_helpers.go:165`, and the
ledger's graduation note asks for exactly this — "a runtime dispatch
respecting the conditional binding",
`test/go/langspec/frontier_spec_test.go:206-207`), and the NUR037
fn-local-fn refusal (the name is looked up live, so the never-executed-def
hazard disappears). Family F — dispatch *recovery* — is §6.9(1)'s to
close, not the twins'.

> **MEASURED 2026-09-02, after the twins landed and flipped: that paragraph
> is WRONG, for all four gates. None of them is the twins' to delete.** The
> reason is one sentence, and it applies to each of them differently: the
> twins fix WHERE THE REGISTRY IS, and these gates defend against WHAT IS IN
> THE BYTECODE. Rollback-and-replay makes VM-time binding state equal the
> interpreter's tape state at the corresponding token; it does not un-bake a
> const, un-inline a splice, or re-resolve a call target the lowering
> already chose.
>
> Frozen-read: the stale value is a `PUSH_CONST` inside a compiled unit —
> verified by disassembly, with the twin already replaying beside it —
> so deleting the gate yields a SILENT wrong answer (`1 1` for the
> interpreter's `1 2`). Stored-handler: needs a runtime LOOKUP at the call,
> which is §6.9's `OpDispatchGeneric`; without it the fallback reads the
> pass-final binding. Family L: the check pass records NO transition for a
> rolled-back arm, so there is nothing to replay — and the rollback now makes
> the registry (outer overload) disagree with the baked `OpCallUser` (shadow
> unit), where the old default at least had them agree. NUR037: fn-body defs
> are excluded from the ledger BY CONSTRUCTION (`FnBodyDepth > 0`), a
> deliberate exclusion that took it from 69254 entries to 6110, so no twin
> exists to place.
>
> All four re-file under §6.9's lookup half, and family L and NUR037
> additionally need a BINDER half that makes a conditionally- or
> frame-locally-bound name registry-visible at VM time. The gate-by-gate
> measurement is in the handoff's "The payoff gates, measured".
>
> **MEASURED AGAIN 2026-09-04 (Stage 4b): the FROZEN-READ gate leaves that
> list too.** It was never a lookup problem: the stale `PUSH_CONST` is in a
> unit the MEMO reused at a call site whose bindings had moved
> (`FnAnalysisKey` omits the enclosing bindings a body reads). The memo is
> now binding-sensitive — each unit records the `DefTable.Gen` of every
> enclosing binding it baked, and a later call site whose generation has
> moved compiles a fresh unit — so `1 2` compiles with parity, and so do
> the type, call-target, `/v` and do-body rebinds. The refusal survives
> only for a unit whose reference ESCAPES into a value. The same stage
> found and closed the leaked-state re-run of `do`/each/fold/scan bodies
> (twelve further default-lane miscompiles) with a body re-run
> environment. The remaining three gates are unchanged by it. See the
> handoff's "Stage 4b".

**WHICH TRANSITIONS ACTUALLY NEED A TWIN — enumerated 2026-08-30, because the
raw site count is misleading by an order of magnitude.** Grepping `r.Defs`
mutation points during a check pass ranks `native_behave.go` first with 18,
`modules.go` second with 11, and `generics_instantiate.go` third with 7 — and
two of those three need no twin at all. Read the sites, not the counts:

| site | sites | verdict |
|---|---|---|
| `core_helpers.go` `installDef` / `UninstallDef` | 7 | **twin** — the def/undef core |
| `carrier_join.go` `InstallJoinedDefs` | 5 | **twin** — the branch-arm push, NUR110's site |
| `core_type.go` `PushType` / `PushTypeAdopted` | 2 | **twin**, paired with the retirement half `BindingSandbox` already partitions |
| `word_extend.go` `InstallWordExtension` / `TransplantExtension` | 2 | **twin** — a dispatch binding pushed WITHOUT passing through `installDef`; this row is a correction, see below |
| `modules/modules.go` export installs | 11 | no — `Install*Exports` are TEST-SETUP helpers, and their own comments say so ("equivalent to what happens when boru code runs `import`"). The real path is `installExports` → `InstallDef`, so a module namespace binding IS a `def` |
| `native_behave.go` | 18 | no — every one is `Push("a", …)` + `defer Pop("a")`, a behaviour body's parameter binding |
| `guard_narrow.go` | 4 | no — `ApplyGuardNarrowing` / `ApplyComplementNarrowing` each return a restore func that pops what they pushed |
| `generics_instantiate.go` | 7 | no — `PushGenBindings`/`PopGenBindings` and `InstantiateSchema`'s key are balanced pairs, and type instantiation is a compile-time product |

So it is **three transition kinds**, not thirty: `def`, `undef`, type install —
the branch-arm push and the module namespace install both being `def` rather
than kinds of their own. A fourth, `def-replace`, was added by measurement
rather than reading: see below. A fifth, `sig-undef`, was SPLIT out of
`def-replace` by a live probe (2026-08-31): a signature-specific undef
removes one matching entry — possibly MID-stack, delta -1 per removal, the
note carrying the removed entry — where a redefinition's replace nets zero;
conflated, a two-overload fn's sig-undef took a name from depth 2 to 1 while
recording delta 0, and the corpus lacks the shape so the composition gate
could not see it (the synthetic rows now supply it, with the locked no-op
counterpart, which notes nothing at all).

**CORRECTION — `word_extend.go` IS a site, and this table said otherwise.** The
first version of this row read "no direct `Defs` mutation, it reaches the table
through `installDef`". It does not: `InstallWordExtension` and
`TransplantExtension` push clones directly (`word_extend.go:365`, `:466`), so a
source-level `def add fn […]` extension and every extension transplanted at
import were invisible. Four further sites were missing for the same reason —
each bypasses the function the reading assumed was the funnel:

| missed site | shape it made invisible |
|---|---|
| `InstallJoinedDefs` (never instrumented at all) | `if c [def x 1] []` — the branch-arm population NUR110 is about |
| `word_extend.go:365`, `:466` | `def add fn […]`, and import-time transplants |
| capitalised `undef` → `Defs.PopEntry` | `def P (refine Integer)  undef P` |
| signature undef → `UninstallFnSigs` → `Defs.Set` | removing one overload of a word |
| `installDef`'s module-wrapper rebind (early return) | `def mymin MathUtil.min` |

Together they were **1750 transitions, 23% of the population** — 5705 → 7455,
before the rolled-back-body exclusion below took it to 7453. The lesson is the
one this section already carries about counts, turned on its author: a reading
of the call graph is a hypothesis, and only instrumenting it is evidence.

**The module row is in that table because the READING got it wrong and the
MEASUREMENT corrected it**, which is the reason to measure before building
rather than after. Instrumenting the eleven `modules.go` sites recorded ZERO
transitions across the whole corpus — including `import "boru:sift"`, the
deepest row there is — because those functions are not on the import path at
all. A count of call sites is not a count of transitions, and neither is a
plausible reading of them.

**Measured over `lang/spec`** (`TestBindLedgerCensus`), 7644 rows — as of the
2026-08-31 oracle closure and the sig-undef split (ten phantom
truncated-region entries removed; two of the old def-replace entries were
locked-match sig-undef NO-OPS and no longer note; the corpus contributes no
top-level loop-join entries and no real sig-undef removal — the synthetic
rows supply both):

	rows with transitions   4290
	transitions total       7441
	  def                    6361
	  type-install           1048
	  undef                    30
	  def-replace               2
	  sig-undef                 0   (corpus; synthetic-covered)
	deepest single row         36   —  unpack 'boru:math-util' sqrt 16.0

**What the ledger must EXCLUDE, and it is most of what a naive instrument
catches.** The first wiring recorded 69254 transitions, and 49382 of them were
incoherent — the depths did not compose, which
`TestBindLedgerDepthsCompose` reports by replaying each name symbolically. Three
exclusions, each measured rather than assumed:

- **`FnBodyDepth > 0`** — a fn body's `def` is a FRAME-LOCAL, pushed per call
  and popped by the frame teardown, which never reaches `UninstallDef`. 69254 →
  6110.
- **shadowing installs** (`installDef`'s `!shadow`) — `InstallFrameBinding`'s
  own contract is that it "shadows — never removes — an outer same-named
  binding", so a macro or fn PARAMETER is not a transition that outlives the
  pass. 6110 → 5705, and incoherence 274 → 2.
- **`BindDefReplace`** — the last two were one shape: `def f fn […] def f fn […]`.
  `installDef`'s overlap filter DROPS the colliding entry before pushing, so the
  net depth is unchanged. A twin replaying that as a plain push lands one level
  too deep and a later `undef` exposes the wrong binding — the hazard this
  section names, and family L's refusal with it. It is a kind of its own so the
  twin reproduces the replace instead of inferring it. Incoherence 2 → 0.
- **speculative installs in a ROLLED-BACK body** (`RolledBackBodyDepth`) — an
  install inside any `keep=false` run of `runCarrierBodyDefsAdds` is undone by
  that function's own truncation on the way out. The binding the pass LEAVES is
  whatever `InstallJoinedDefs` puts back afterwards, or nothing at all; the
  speculative install is a third thing, and recording it double-counts. Wider
  than `CondBodyDepth` on purpose — a condition fragment is truncated too, even
  though it is not conditional.

**AND THE CORPUS COULD NOT SEE THAT LAST ONE**, which is the part worth
carrying forward. `lang/spec` contains exactly three `if …[def …]` rows and all
three are inside a fn body, where `FnBodyDepth` suppresses them. So
`TestBindLedgerDepthsCompose` reported a clean **0 incoherent over 7644 rows**
while the shape this census exists to size — NUR110's own, a top-level
branch-arm `def` — was recorded TWICE at the same depth on every occurrence. A
twin built against that ledger would have pushed the binding twice, and the
differential would have reported it much later as a mysterious divergence.

`TestBindLedgerBranchArmDepthsCompose` closes the blind spot with synthetic
top-level rows (both arms, one arm, a pre-existing outer binding, two `if`s in
sequence, and a `while` body). Verified to FAIL without the exclusion — eight
incoherent transitions across the six rows — and pass with it. A corpus is
evidence about the programs it contains and silence about the rest; when the
population being measured is a shape the corpus lacks, the gate has to supply
it.

**THE STRONGER ORACLE IS LANDED AND GREEN** (2026-08-31,
`test/go/langspec/bind_ledger_live_oracle_test.go`, `TestBindLedgerLiveDepths`,
**no allowance**). The composition check is self-contained by construction
(§ the file header), but `Boru.NativeRegistry()` exposes the live registry, so
the ledger's final depth per name is compared against the depth the check pass
ACTUALLY left — the assertion the inert-twin step needs. First run: 9
mismatches over 4291 rows, in two apparent classes; closing them took THREE
fixes, because each apparent class hid a different mechanism than its reading
suggested:

- **The gensym-temp class (8 rows) was not macro construction — it was two
  snapshot/restore-TRUNCATED evaluation regions recording installs their own
  restore tears down.** `expandMacroWith` runs the template body with its
  locals live (`def t (gensym)`) and then `r.Defs.Restore(snap)`s them away;
  the "construction-time expansion" was the DYNAMIC-HELP example eval
  (`makeDynamicEval`), which fires mid-pass from the fn-registration hook,
  runs example code against the live registry — expanding the freshly-defined
  macro — and restores its defs on exit. Both now bracket with
  `CheckState.SuppressBindLedger` (the `RolledBackBodyDepth` arm): what makes
  an install unrecordable is the truncation, the same rule the body runner
  already carried.
- **The module-io row's multi-pass hypothesis was tested FIRST and
  disproven** — the doubling reproduces inside a single pass, and at plain
  runtime. The real mechanism: `narrowDynamicUses`' analysis push (a dynamic
  binding tightened at a typed use) has no top-level popper, so the pass LEFT
  it — a genuine defect the oracle exposed, not an oracle artifact: a later
  `Run` on the same instance read the leaked `dynamic(Ideal)` carrier instead
  of the bound Lock. Fixed by a guarded pass-end pop
  (`CheckState.PassEndCleanups`, run LIFO by the Begin closer; the pop guards
  on the entry still being on top, so a narrowing truncated with its branch or
  buried under a later real rebind is left alone). Neither the ledger nor the
  oracle was bent to match — the pass was leaking.
- **Running the oracle over the SYNTHETIC rows found a ninth-plus-one the
  corpus run could not:** the while row in `syntheticBranchArmSources` was
  MALFORMED (`while (n gt 0) …` hands while a Boolean; its `(List, List)`
  signature refuses it), so the row never exercised a loop and "passed"
  composition while measuring nothing. Corrected, it showed
  `AnalyseLoopBody`'s final joined post-loop pushes bypassing the ledger
  entirely (raw `r.Defs.Push`, no note). The final round's installs are now
  ledgered — the loop's "may run zero times" join is `InstallJoinedDefs`' own
  one-branch rule, so it records like one.

After the three fixes the oracle reads **0 mismatches over the corpus and the
synthetics**, and it is a standing gate with no allowance — a gate that
reports a number it tolerates teaches the next author to tolerate it. The
census shifts 7453 → 7443: the ten phantom truncated-region entries leave, the
loop-join entries arrive.

Three things that shape the twin work. `def` is 85% of it, so its twin is the
one whose cost matters and the other three can be straightforward. `undef` and
`def-replace` are 34 transitions COMBINED across the entire corpus — rare enough
that theirs can be the simple, obviously-correct ones. And the deepest single
row is 36, not thousands: with frame-locals excluded, no corpus program performs
more than a few dozen twin-replayable transitions, so per-transition cost is not
a budget concern.

**Positions are the other half of a twin, and getting them right took three
tries**, each wrong in a way worth recording because the next author will reach
for the same two sources. The value's own `Pos` is the VALUE token (`def x 1`
gives 1:7, the `1`). `CurWordPos` has already MOVED by install time whenever the
body was analysed first (`def f fn [… [def y x y]]` gives the INNER `def`).
Staging the site in `CheckState.PendingBindPos` and clearing it on every note
let a SUPPRESSED body-local note steal the position its enclosing `def` had
staged. Save/restore around each install is what nesting actually needs, and
`TestBindLedgerEntriesArePositioned` pins that every entry names a real site.

**The partial twins that already exist**, and what changes about them.
`OpBindGlobal` is the value half under TODAY's keep-the-installs regime: it
`SetAt`s into the kept check-pass slot at the recorded depth, deliberately never
a push, so shadowing depth and undef behaviour match the interpreter. Under the
twin regime that inverts — the slot is gone after the rollback, and the twin
pushes at its source position (`GlobalBindSpec.Push`, stamped by lowerDynBind
when the regime is armed). `OpBindDynScope` makes a frame binding
registry-visible and is unaffected.

**Staging, and it must not be inverted.** Emit the twins FIRST, while the
check-pass installs are still kept, so every twin is inert; assert that the
replayed binding state matches what the pass left; THEN flip to
rollback-and-replay. Rollback-first breaks every program until the last
transition has a twin, and offers no intermediate state where the differential
means anything — the same reasoning that made Stage 2 land its three re-seats
separately.

**Status (2026-08-31): the flip itself is BUILT and corpus-green behind
`BORU_TWIN_REGIME=1`** (default off — byte-identical until flipped): recorder
stamp `Program.TwinRegime` + Finalize's full-placement refusal, lang's
`SnapshotBindings` → `RestoreBindingsForReplay` rollback at the between-phases
point (module ledger pass-final — imports run once), `OpBindTwin` →
`core.ApplyBindTwin` per kind with the carrier-class skip pairing computed
defs to their Push-mode `OpBindGlobal`. The regime lane
(`test/go/langspec/bind_twin_regime_test.go`) measured 6411 corpus rows
compiled vs 6416 under keep on landing, ZERO divergences; the 5-row cost was
first labeled "4 island-discarded do-body defs + 1 each-body def", but
instrumentation showed ALL FIVE record through the closure path (no island
fires), so the four do-body rows were recovered the §6.5-faithful way —
do-body twin ADOPTION (`CallableSpec.BodyOnceKeepsDefs` on `do`;
`EmitState.AdoptBodyTwins` places the suspended body twins as real twin ops
after the closure call, replaying the captured entries), never island
re-execution, which the retained-mints partition makes unsound for
capitalised defs. The each-body def then closed via ARM-RESIDENT TWINS
(OpBindResident: per-element runtime-value installs inside the compiled
per-invocation unit — install through the interpreter's own installer, no
unwind trail; the var pair's both halves in-arm; a cross-request
interpreter-parity oracle, bind_multirun_parity_test.go, measuring install
count/values/order/definedness landed FIRST), bringing the regime to FULL
CORPUS PARITY with the default recorder: 6416 rows compiled, zero
regime-only refusals, zero divergences. The handoff
(design/FULL-COMPILATION-HANDOFF.0.md) carries the remaining default-flip
checklist and the payoff deletions.

**Status (2026-09-01): FLIPPED. Rollback-and-replay is the ONLY regime.**
`BORU_TWIN_REGIME`, `compiler.TwinRegimeEnabled`, `EmitState.twinRegime`
and `Program.TwinRegime` are deleted; every compiled request snapshots
before its check pass and rolls back before its run; every placed
`OpBindTwin` replays; a global bind can only PUSH (`GlobalBindSpec.Push`
and the VM's SetAt arm are gone, and `DefTable.SetAt` with them). The
flag-armed corpus lane retired into the default differential, which
inherits its 6410 floor. The flip was measured before it was made — a
rehearsal running every module suite with the flag forced on had reduced
the exposure to three annotation-only goldens — which is what let it be a
deletion rather than a migration; the same rehearsal surfaced and closed
NUR116 (a default-lane miscompile the regime was the sound side of) and
bounded NUR115 — discharged 2026-09-02: `foldaxis` now analyses its body
(`foldaxisReturnsFn`, fold's fixed point one rank down) and carries
`BodyMultiRunKeepsDefs`, measured by six parity-oracle rows whose negative
control reproduces the silent loss. The payoff-list latch deletions are NOT in the flip: the
rehearsal measured the stored-handler dep-rebind refusal as load-bearing
with the regime on (its removal yields a VM internal_error and a
fallback, not correct compiled code), so each gate goes only after its
own `-force-compile` measurement, and the frozen-read family needs §6.9's
runtime-lookup half first.

Three consistency obligations come with the twins, and they interlock.

**Replay, never re-execution.** Before the flip the compiled path
deliberately *kept* the check pass's `RunInCheckMode` installs and ran the
Program on that registry; replaying the same transitions through twins
would have double-applied them (duplicate `DefTable`
entries, so a later `undef` exposes the duplicate instead of the prior
binding). The twin regime therefore rolls the *runtime-visible binding
transitions* back to the pre-check snapshot before `RunProgram`, and each
twin re-installs the **identical binding object** the check pass produced
— the same `FnDefInfo`, the same module instance — at its source position;
nothing runs twice, and compile-time-only products (minted type IDs, macro
expansions, const folds) stay baked under the front-end carve-out. A
module import in particular executes **once**, in the front end; its twin
re-binds the already-produced instance, so module fn-value identity
(§6.3's pointer-based `eq`) has a single referent and a Program's pinned
sub-registries stay the only instance.

**The rollback primitive already exists — and is too COARSE to reuse.**
Probed 2026-08-26. `Registry.SnapshotForCompile` / `RestoreForCompile`
(`core/go/compile_sandbox.go:42,91`) already implement exactly the
snapshot-and-roll-back this regime needs, and `RunCompiledStrict` already
applies them — but only when the front end FAILS: a `CompileCheck` error and
an uncompilable-program refusal each restore, while an error out of
`eng.RunProgram` returns directly and leaves the check-pass installs in place
(`lang/go/boru.go:1225-1231`). So it is the compile/check-failure paths, not
every error path — the function's own doc comment overstates this, and a
program that compiles and then raises at runtime keeps its installs. Either
way the SUCCESS path keeps them, which is why the twins have nothing to
replay onto today. So the rollback half of this design is an existing
primitive applied at a different point, not new machinery. Three caveats
before reusing it:

- **It restores `r.Types`** (`compile_sandbox.go:107`), which would discard
  the minted type IDs this section requires to stay BAKED under the
  front-end carve-out. The twin regime needs a NARROWER snapshot —
  runtime-visible bindings (`r.Defs`) and the module ledger — leaving macro
  expansions and const folds alone. Reusing `RestoreForCompile` wholesale
  would roll back the compile-time products the twins are specifically not
  supposed to replay.
- **But `r.Types` cannot be excluded WHOLESALE either.** A capitalised
  `undef` executing on the check pass RETIRES a lattice node
  (`basic/go/native_definition.go:1389-1400`: `PopEntry` then
  `r.Types.Retire(entry.TypeDef)` when this binding minted it). Restore
  `r.Defs` alone and the type BINDING comes back while its ID stays retired
  — a live binding pointing at a dead node, before the VM has reached the
  twin at that `undef`'s source position. So the narrow snapshot has to
  separate the two things `r.Types` holds: minted IDs are RETAINED, runtime
  RETIREMENTS are rolled back and re-applied by their twins. That is a
  partition of the TypeTable, not an exclusion of it, and it is the part of
  this design with the least existing machinery to lean on.
- **The dispatch-cache invalidation comes with it.** `RestoreForCompile`
  drops every `dispatchCache` entry because `DefTable.Clone` starts a fresh
  generation timeline at 0, so a gen-0 entry cached before the rollback
  could be served for a name whose restored binding differs
  (`compile_sandbox.go:96-106`). A narrow rollback inherits that hazard
  exactly — the twins reinstall bindings under names the cache may already
  hold — so the cache reset is part of the regime, not an artifact of the
  wide snapshot.

Measured alongside: there is no double-install TODAY, on either lane.
`def x 1 end def x 2 end undef x end x` answers `1` compiled and
interpreted, and `def x 1 end undef x end x` is `undefined_word` on both. The
single-install path is what makes `undef` expose the prior binding correctly,
which is precisely why adding a second install without the rollback would
break it — the hazard this section names is real, and currently latent only
because nothing replays.

**Stamp freshness is taken against the post-replay world.** Dependency
snapshots recorded during the check pass (`DepSnap` over `(Depth, Gen)`,
`compiler/go/stamp_runtime.go:155-165`) would read stale against a
rolled-back-and-replayed `DefTable`; under the twin regime dep snapshots
are taken — or re-based — at twin-execution time, so the first invoke does
not mass-restamp.

**Effect order.** The check pass currently executes module imports,
effects included, before any VM instruction (`lang/go/boru.go:1017-1023` —
the C1 fence is armed before the check pass for exactly this reason),
which violates T3's ordering for a program with a runtime effect *before*
an effectful import. Under totality the front-end pass must be
**observationally silent** — the doctrine's O1 effect-freedom obligation
enforced rather than assumed — with every effectful half of a check-mode
word given a runtime twin at its source position. The one effect the
twins cannot carry is module-body execution itself: an effect-dependent
*export* must exist before the checker can type against it, so the body
runs in the front end. Until Stage 7's compile-module-bodies work moves
that execution to the twin position, import-time effect ordering remains
a **named front-end carve-out with a documented T3 exception** — which is
today's behavior, made explicit (O4). Hot code loading
then stops being "an interpreter-path feature"
(`design/HOT-CODE-LOADING.0.md:94-99`): a swap's cost is a G-lane lookup or
a JIT restamp — de-optimization, not decompilation.

### 6.6 Production-order results, mark regions, and provenance

Families C, D, I share one cause: the static seat model — every value must
have a compile-time address (event/const/local) — cannot address values
whose existence or count is runtime-only. The G-lane drops the requirement:
**within a G-lane region, the stack is the address**. Values live where the
statement machine put them, in production order; `planValueDefLocals`-style
promotion applies only at the region's typed borders.

- Family C (two dynamic results live at once, the NUR038 seal twins)
  dissolves: both results simply sit on the stack.
- Family I (0..N winner residuals, NUR067) gets the **generalized mark
  region**: `OpStackMark`/`OpDropToMark` exist in two narrow shapes; the
  general form is a mark with *no static count*, closed by ops that consume
  "everything since the mark" (variadic collect, splice, residual merge).
  This is the interpreter's own model — the tape region before the pointer
  *is* the value stack — finally given a compiled twin.

  **Partly overtaken by measurement (2026-09-10).** The *producing* half
  needed none of this. `OpDropToMark` truncates to `stack[:m]` and
  `OpPopMark` keeps whatever is above the mark, so the mark ops were never
  0-or-1-specific at the VM at all; and a value-producing loop's region
  already IS "one recorded slot, runtime count" on the compile side. Family
  I therefore graduated by REUSING that representation — `awaitVariadicResult`
  returns one variadic-spread carrier, `callVariadicRegion` records the event
  as a region, `lowerCall` marks `lw.variadic` — and the wholesale
  `MarkUncompilable` is gone.

  The *consuming* half is where the generalized region does earn its keep,
  and it is a T-lane question the loop rows ask identically. It landed the
  same day, as two CLOSING OPS over the mark plumbing that already existed:
  **`OpSeatBelowMark`** lifts a residual's inert PREFIX down to the mark, so
  `99 for 3 [i]` and `99 await {mode:'first'} [[]]` compile — fixed values
  BENEATH a run whose length is never named — and **`OpMakeListToMark`**
  collects `stack[mark:]` into one List, so `size [(for 3 [i])]` and
  `size [(await {mode:'any'} [[7 8]])]` compile. Family I is closed.

  What the generalized region is still owed is the SHAPES these two ops do
  not reach: a run with fixed values on BOTH sides (`[9 (for 3 [i]) 8]`), a
  region consumed by an arbitrary word rather than a list literal, and a
  region that is not adjacent to its consumer. Each of those is a place where
  "the stack is the address" would answer directly and a static seat still
  cannot.
- Family D's synthetics (`$module`, namespace reads) become `wordRef`
  slots resolved by the same runtime lookup as any name; "operand of
  unknown provenance" is not an answerable question in the G-lane because
  it is never asked. Provenance remains the *T-lane's* discipline — where
  it is the soundness story for folds and seats and stays exactly as
  `COMPILABLE-SUBSET.md` §2 states it.

### 6.7 Runtime-supplied code: the resident compiler

Every no-interpreter system in the survey ships its compiler in the
runtime; islands are the only alternative ever tried, and they are banned.
boru is unusually well-positioned: **runtime compilation is already
shipped** — fork-isolated (`ForkConcurrent` + fresh `CheckState` +
`BeginCompilePass`), dependency-stamped (`DepSnap` over `(Depth, Gen)`),
invalidation-bounded (`RestampMaxTries=3`), and exercised by `Vm.run` and
the JIT restamp. The eng module already imports the compiler (the module
chain permits it — "compiler → eng" is dataflow, not import direction), so
the VM may invoke compilation without any layering change.

The G-lane extends this from "detached fn" to every runtime-code shape:
computed `do` bodies (the maintainer directive is on record: "`do` must
ALWAYS compile — natively … correctness via interpreter fallback is not
enough", `design/DO-STRUCTURE-COMPILATION.0.md:252-254`), computed splices
and interp-string/XML code parts, `mini` hook expansions, module bodies at
import (retiring the "module-load" attributed interpreter entry, or
re-declaring it as a front-end carve-out — §11, O4). Two rules keep it
sound and total:

- **Isolation**: never a mid-run `CompileCheck` on the executing registry
  (the shipped rule); compiled-at-runtime units enter by the same universal
  convention as AOT units.
- **Induction**: runtime compilation must itself never refuse — and
  "never refuse" is a property of the whole pipeline, not the recorder
  alone. The recursive call is to the *same* total procedure, so what the
  induction actually establishes is this: each compile of a finite body
  terminates and, given §5's demotion rule at `Finalize`, produces a
  runnable Program. A computed body is *not* structurally smaller than its
  producer — eval chains can construct larger bodies — so total compile
  work is bounded by **execution**, not program size, which matches
  interpreter cost semantics: the interpreter also pays per execution of
  computed code. Its preconditions are explicit and carried by F5: §5's
  Finalize totality, an answer for the front end's own ceilings (O5's
  check-budget exhaustion; the fragment-depth ceiling, `lower.go:2041`),
  and the eligibility/policy gates around detached stamping. Cost is the
  known trade — compile-on-eval is ~interpretation cost per op for
  run-once code (§7) — bounded by the **planned** Phase 6 JIT
  detached-unit cache: named as the graduation at `emit.go:6569`, scoped
  in `RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md`'s Phase 6 (body identity
  keys on the structural `FnAnalysisKey` precedent, never `Value.ID`), and
  **not yet built** — an explicit Stage 7 dependency.
- **Staleness policy at the end state**: the bounded restamp
  (`RestampMaxTries = 3`) exists because its exhaustion arm lands on
  `CallBoru` — tape interpretation — fine under today's valve, banned
  under T2, and "G-lane re-dispatch" is no answer for a boru body (it
  selects the live arm; *executing* the arm still needs the unit the
  budget just refused). Under totality the restamp is **unbounded but
  memoised**, keyed structurally per `(body key, dep Gen)` via the unit
  cache: a rebind costs one compile per *world*, not per invoke, and the
  rebind-between-every-invoke pathology degrades to compile-per-invoke ≈
  interpreter cost (§7 prices it) — never to a wrong answer or an
  interpreter run. The budget deletes with the valve.
- **Registry mutation inside runtime-compiled code.** A computed body may
  itself contain `def`/`import`/`type`/macro use. The rule is §6.5's twin
  regime applied to the *executing* registry: the fork's check pass is
  observationally silent and its runtime-visible transitions twin into
  the unit, applied to the live registry at unit-run time in source
  order; compile-time identities the fork minted (types, macro
  expansions) are re-minted/reconciled onto the live registry under the
  same single-instance rule, with the interpreter's own behavior for the
  same eval-time definitions as the parity oracle. Macro bindings visible
  to the fork's expansion are the live registry's at fork time — a
  mid-run rebind after the fork is the same race the interpreter has.
- **Re-entrant hosting**: the busy-registry callback route (`CanHostVM`
  declining → `CallBoru`, `core/go/registry.go:677-686`,
  `core/go/invoke.go:49-52`) is a *structural* decline that no recorder
  totality touches; its retirement is nested VM hosting on the running
  registry — the mid-run `NestedRunner` seam already exists — named in
  §6.10's table.

One genuine pressure point from the survey: systems that refuse islands
also refuse *lexical* eval (Chez/SBCL/Hermes evaluate against top-level
environments only). boru's eval-class words that see local scope are
covered because the compiled lane can **reify the environment** — the
DynEnv machinery exists precisely to make frame bindings registry-visible —
armed per-region by the descriptor (§6.2), not program-wide.

### 6.8 Handler contracts: retiring the tape-coupled handler class

The VM refuses handler results that carry tape tokens (`screenResults`,
`eng/go/vm.go:217-223`, over the `tapeCoupled` predicate at `:171`) —
correctly: "return tokens for the engine to re-step"
is the one interpreter capability with no compiled meaning. Totality
requires that no reachable handler *needs* it. The instrument is the
adopted declaration triple (§3.4): every signature declares, per operand,
`tapeBound` (tri-state, unset = refuse at registration — constraint C1),
`needs` (a set: `FnDefInfo | RawTokens | CompiledUnit | Any`), and `env`
(`None | Captured | Live`). The migration:

- Handlers with `needs: CompiledUnit`-capable slots receive **units**
  (compiled at record time when the body is written down; at runtime via
  §6.7 when computed). `InvokeBody`'s raw-token fall-through arm retires;
  bodies run through `enterBodyUnit` under the VM invoker — the modelled
  callback frame the HOF audit names as the `InvokeBody` island's
  graduation criterion (`design/HIGHER-ORDER-FUNCTIONS.0.md:921`),
  generalized here from the list-Function rows to every code-body word.
- `env: Live` handlers (dyn-scope, `parselang`-class) get the per-region
  DynEnv arming.
- Structured-lowering words that still splice (`while` — family H) get
  structured lowerings (`WHILE_SETUP`/`WHILE_NEXT` on the `FOR_SETUP`
  precedent: per-iteration condition fragment, accumulating body values on
  a mark region, break/continue as jumps, truthiness via the kernel rule).
  **Landed 2026-09-09** (the thirty-seventh increment) with no new opcode:
  `RecordWhile` records the loop on `FOR_SETUP`/`FOR_NEXT` over an
  unbounded count and a scratch iterator; the condition fragment lowers at
  the head of every iteration and a falsy value exits through `FLOW_BREAK`.
- Any handler that genuinely re-steps its operand on the tape
  (`tapeBound: Yes` — today's honest `var`-class answer) must be
  **rewritten to a structured or unit-taking form**. That is a finite,
  enumerable worklist (the triple's registration assert produces it), and
  it is the part of totality that is handler work rather than compiler
  work. Nothing in the survey suggests a shortcut exists. **Measured
  2026-08-25** (`test/go/langspec/declaration_census_test.go`): of 522
  signatures in the default registry, 172 are declaration-relevant — they
  take a code body, quote an operand, declare a callable convention, or
  can receive a Function-typed operand — and **114 of those carry no
  compile declaration at all** (code-body 59, quoted 44, fn-operand 11).
  That is the Stage-6 worklist, and it is a floor rather than a total,
  since importing modules registers more. Each one is a place the recorder
  runs on the zero value's assumption, which is precisely what C1's
  tri-state exists to stop being silent about.
- The **check-lenient words** behind the `SuppressedRuntimeError` latch
  (`lang/go/boru.go:474-482` — an orphan `gen`, an `unpack` of a missing
  key: lenient in check mode, strict at runtime) are their own per-word
  migration item, because neither of §6.9's mechanisms covers them as-is —
  the pass could not even *model* the runtime raise, so a trap has nothing
  proven to carry and the recorded stream was built on the lenient answer.
  Each such word either gets a strict check-mode twin (model the raise;
  then the trap/builder discipline applies) or a generic lowering of the
  affected statement. The latch's own census enumerates the list.

### 6.9 Checker totality and lane alignment

**A precondition this section did not know it had (measured 2026-08-26,
NUR105).** Every alignment claim below assumes the two passes analyse the
same program. They do not. A function VALUE constructed in ARGUMENT
position — all three anonymous spellings, `=>` and `fn` and `afn` — has its
body analysed by NOBODY on the plain pass:

```
$ boru check cb.boru      # each ([e:Any] => [nosuchw e]) [1 2 3]
check: 0 error(s), 0 warning(s), 0 info

$ boru run cb.boru
error: each: element 0: [boru/undefined_word]: undefined word: nosuchw
```

That is a false negative, not a fork: the identical body is flagged the
moment it is `def`-bound, so the checker can plainly do the analysis. A
code BLOCK argument and a named fn REFERENCE are both analysed; POSITION
decides it, not spelling. `each`, `filter`, `fold`, every `service`
handler, every comparator — the callback-as-argument shape is how boru does
higher-order work, and it is the shape §12 and §13 build the server story
on.

Two consequences for this section. First, it is the FIRST of NUR103's three
faults and has to land before the rest of 6.9 can be trusted: a fork
argument about diagnostics is meaningless while one pass is not reading the
code. Second, the ratchets are measuring a smaller universe than they claim
— the diagnostic-parity count, the check-accuracy pins, and the frontier
census all read a plain pass that skipped these bodies.

**Fixed 2026-08-26**, and the shape of the fix is a finding in its own
right. The body is analysed by the same pass `InstallFnDef` already ran,
but QUEUED at construction and DRAINED at END OF PASS, beside
`RescueForwardRefDiagnostics`. Three attempts were needed and each failure
named a property this section has to respect:

1. **The construction site is too early.** Every forward reference is still
   unbound there, a recursive self-call most of all, so `def fact fn [… [n
   mul (fact (n sub 1))]]` reported `mul: got (Integer, Atom)`. The
   `undefined_word` behind it is rescued at end of pass; its CONSEQUENCE is
   not. Any totality work that moves an analysis earlier inherits this.
2. **A body must be analysed in the scope it was WRITTEN in.** Draining
   against one registry called every module-scope name in mini-redis's
   handler lambdas undefined. The Stage-4 descriptor adapter carries the
   same obligation.
3. **A speculative analysis that cannot run must report nothing.** The
   drain analyses bodies in ISOLATION, so one that reads the caller's stack
   cannot run at all — the Church-numeral row raised the
   strict-forward-barrier error on a program that answers 5.
   `fn_body_error` is the analyser reporting that IT could not proceed, and
   6.9's traps have to make the same distinction: a definite error in the
   PROGRAM compiles to a trap, a failure of the ANALYSIS does not.

Whole corpus green afterwards, ratchets included — which also settles the
"moves in both directions" worry above: it moved neither.

Four changes, all conservative in the abstract-interpretation frame:

1. **Delete the whole-program "check diagnostics" sentinel**
   (`lang/go/boru.go:459-472`). A statically-definite error compiles to
   code that raises the interpreter's error at the identical moment,
   catchably (family E's `FnUtil.flip 5` rows, where *the error is the
   specified result*) — in two forms, because diagnostic *content* is
   value-dependent even when failure is not: `OpTrap` with the baked
   structured diagnostic only where every operand is concrete, and the
   runtime error-builder over the live window otherwise. The trap path
   already declines carrier operands for exactly this reason — the rich
   diagnostic (received-argument note, per-candidate verdicts) must be
   built over the *concrete* runtime value or the lanes diverge
   (`core/go/engine.go:9182-9197`), and the runtime rematch is the shipped
   mechanism that builds it live. Could-match dynamics compile to G-lane
   dispatch that raises or succeeds as the runtime decides. Recovered
   windows (family F) become traps or G-lane rematch — `OpDispatchRematch`
   is the in-tree template, and §6.2's descriptor state is what
   distinguishes this from the three differential-reverted recovery-site
   attempts the "DO NOT RETRY" record pins. The fabricated `assume-sig`
   recovery windows, which corrupt the tape model, are replaced by honest
   G-lane regions, not admitted. With the sentinel gone, the C2 error
   oracle's check-error trigger is unreachable: C2 narrows to parse-error
   programs — which never reach either executor — and retires as a
   production path at Stage 8, surviving only inside the harness (§8).
   **What a definite error implies for the code AFTER it (decided
   2026-08-26; NUR103's second fault).** The checker already models a
   provably-failing call as DIVERGENCE — the ReturnsFn returns an empty
   residual, the mechanism `1 div 0` uses (`design/CHECK-ACCURACY-RATCHET.10.md`).
   What it does not model is the consequence. An empty residual means
   `def h2 (cur set "f" None)` binds NOTHING, so a later read of `h2` is
   reported as an `undefined_word` — a diagnostic at a different line,
   about a different name, than the fault. That is how NUR103's mini-redis
   refusal came to name `h2` at `mini-redis.boru:210` when the site that
   actually reproduces is the HDEL handler at :230.

   The rule this stage adopts: **after a provably-divergent expression, the
   rest of the region is UNREACHABLE.** Report the divergence ONCE, at its
   site, and suppress its downstream consequences rather than re-deriving
   them as findings about names. That is what an empty residual already
   means — `Never` — and it is what the compiled lane will DO: the trap or
   runtime error-builder raises, and nothing after it executes. A checker
   that reports consequences the compiled program can never reach is
   describing a program that does not exist.

   Two things this does not license. It does not suppress the divergence
   itself, which stays a finding at its own site. And it does not extend
   past the region: a divergence inside one branch arm says nothing about
   the arm beside it, which is why the unit is the region §6.2 defines and
   not the statement.

2. **Classify every checker shortcut** as `advisory-only` vs
   `sound-for-lowering` (the soundiness rule — Livshits et al., CACM 2015).
   The quota/recursion bails are widenings and must widen only to
   interpreter-enforced facts or to dynamic (the precedent is already in
   the tree: the accuracy quota is disabled while compiling,
   `check/go/carrier.go:2751-2767`). Success typings are the published
   model of this stance (Lindahl & Sagonas, PPDP 2006); the difference —
   boru's typed lane *trusts* its facts — is exactly why the
   classification must be explicit.
3. **Collapse or prove diagnostic-neutral every `!Compiling` fork.**
   **Measured 2026-08-26** (`test/go/langspec/diagnostic_parity_test.go`):
   **318 of 7,568 corpus rows** already produce different FINDINGS —
   errors and warnings — from a plain check than from a compile-armed
   one, with a further 260 differing only on informational advisories.
   The split matters: some advisories are pass-specific by design
   (`module_body_executed_in_check` warns that `boru check` executed a
   module body the user did not ask it to run, and under compilation that
   execution is the program's own), so counting them would demand the
   wrong thing. Classified by what the USER sees, the 318 split three ways: **5** rows
   clean to `boru check` and refused by the compiler — NUR103's class, the
   one a user cannot diagnose; **41** where the armed pass drops a
   diagnostic but refuses, so the finding still reaches the user through
   the refusal reason (the documented `no_signature` suppression, which
   exists precisely so the specific reason is not masked by the generic
   sentinel); and **272** where a check finding vanishes and the program
   compiles anyway. That last group is the surprise: on 272 corpus rows
   the checker is STRICTER than the compiler, so `boru check` reports
   errors on programs that compile and run. Stage 8's work is therefore
   two different jobs — 5 rows of invisible divergence, and a
   272-row false-positive surface — not one undifferentiated 318.

   **First row discharged, same day: 5 → 4, and 318 → 317 → 318.** The gate found a
   one-line instance the four manual reductions of the mini-redis shape had
   all missed — a field read from a structural-record parameter,
   `edge-dispatch-3.tsv:L56`. It was not a `!Compiling` fork at all. An
   INLINE record parameter's field type words (`o:{pretty:Boolean}`) were
   never resolved to types, where the named `type R record {…}` spelling had
   them resolved by `record`'s own dispatch; the schema-bearing param carrier
   copies the pattern verbatim, so the field read narrowed to
   `dynamic(Word)`, and the step loop — which classifies by parent type
   alone — then dispatched that CARRIER as a token, with no `WordInfo` and
   hence no name. Only the compile pass builds the precise carrier, which is
   the whole reason the divergence looked like a fork. Fixed by resolving the
   field words at sig install (`ResolveSigRecordFields`) and by making a
   word-typed carrier DATA in `stepWord`; NUR103 carries the trace. Two
   lessons for the remaining rows: the measurement found in one run what
   reasoning had missed four times, and "armed-only" does not imply "a fork
   is responsible".

   The parity count then went back to 318, and the arithmetic is worth
   spelling out because it is the ratchet's first real test. NUR104 added
   three spec rows and one of them — an `ERROR:no signature matches` row —
   diverges: `no_signature/f` on the plain pass, nothing on the armed one,
   program compiles. That is the documented suppression, the class already
   carrying 41 rows, and it is invisible to the user (both lanes surface
   the identical error through the CLI pre-flight, and both refuse the
   call). So the ceiling rose by one against a corpus three rows larger. A
   ratchet that may only ever fall would forbid adding ERROR rows to the
   corpus, which is exactly the wrong incentive; what it has to forbid is a
   divergence nobody accounted for.

   Plain-only findings
   (`no_signature` suppressed while compiling, `unreachable_branch`, the
   `module_body_executed_in_check` info); armed-only findings
   (`redundant_guard`, `case_not_exhaustive`, and — the NUR103 shape —
   `undefined_word` on programs plain check calls clean); and 57 rows
   where one plain diagnostic becomes **two** under compilation. Some of
   these are deliberate and documented. None of them was measured, and
   the user-visible consequence is that `boru check` can report a program
   clean that the compiler refuses, with no way to see why.
   (static-if reduction, loop spread, closure surfacing, `no_signature`
   emission itself, `check_recovery.go:1018,1126`). T4 requires one
   checker behavior regardless of the consumer. The open budget question —
   step-budget exhaustion during check has no emit-anyway story
   (`core/go/check_state.go:250`) — is O5 in §11.
4. **State the alignment property formally** so it can be tested: (a)
   *erasure* — the checker's **inferred** facts (carriers, joins,
   narrowings) have zero runtime footprint: discarding them changes
   nothing, in either lane, with `=` rather than "up to cast errors"
   (the degenerate gradual guarantee — Siek et al., SNAPL 2015).
   Deliberately out of scope: **source annotations**, which in boru are
   runtime dispatch inputs — a signature's declared types feed
   `MatchSignature`, and a typed def validates at bind time — so they are
   program semantics both lanes must honor identically, not checker
   metadata to erase; (b)
   *abstraction soundness* — every carrier fact over-approximates every
   run (both lanes — one execution semantics as far as the checker is
   concerned); (c) *lane coherence* — `obs(VM(compile p)) = obs(Engine p)`
   over the declared observable alphabet, checked per-program by the
   differential harness, which is translation validation in the Pnueli
   sense (TACAS 1998) with the interpreter as reference oracle.

### 6.10 Retiring the escape valves

The end-state runtime has **no interpreter landing pad**. The worklist is
the ~15 `vmDefer` sites plus the C1 fence, each with a named replacement:

| Escape site | Replacement |
|---|---|
| dyn-scope miss / dispatching / active-token | G-lane lookup raises the interpreter's own error |
| poly no-match (canonical error cases) | kernel no-match raise (`PolyNoMatchSpec` already proves window rebuildability — make it total via the descriptor) |
| NOut drift / user-poly table drift | never speculated on under §4's binding-stability rule — the site and its continuation are G-lane; future entries restamp (§6.7) |
| rematch-**matched** ("the doctrine's landing pad") | execute the matched arm natively — the R1 item the plan already names |
| shaped-method claims | `OpCallDynMethod` completes natively via Apply |
| RetReplay count mismatch | Apply reproduces `execFnDefLiteral`'s landing rule as-is — trim-only on the foreign-frame path (`eng/go/vm.go:2316-2344`), never a new enforcement the interpreter lacks |
| splice-active | generalized mark regions |
| callback `internal_error` degrade (`core/go/invoke.go:56-64`) | propagate — with no second lane there is nothing to degrade *to* |
| predicate body inside matching (`RunPredicate` → `CallBoru`) | lazily-stamped predicate units run via the VM invoker (§6.3) |
| busy-registry callback (`CanHostVM` decline → `CallBoru`) | re-entrant VM hosting on the running registry (the `NestedRunner` seam, §6.7) |
| JIT-declined / restamp-exhausted bodies → pooled sub-engine | unbounded memoised restamp (§6.7); the analysis-decline class dissolves with §5's Finalize totality |

This retirement list deliberately **overturns C4's recorded permanence**
of the fail-safe decline seams ("permanent by design",
`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md:123-135`): those seams were
permanent because their only landing pad was the interpreter, and
refusing to land there meant refusing the program. T2 removes the landing
pad; the table is the argument that each seam can land compiled instead.

The C1 effect fence exists to make re-runs safe; when there are no re-runs
it reduces to error propagation. `runtimeShouldFallback`
(`lang/go/boru.go:1298`) and the whole `RunCompiled`-vs-`RunCompiledStrict`
split collapse into one semantics — strict *is* the semantics.
`OpFallback`, its lowerer, `islandRun`, `runIslandResolved`, the drift
window, and the P7 machinery delete (the long-promised deletion), each
removal pinned by the engine-entry census (§9).

---

## 7. Performance

The G-lane must beat the interpreter for the design to be an upgrade rather
than a shuffle. The evidence says it will, for the classical reason:
per-op semantic work is identical, and the fetch/decode/collection-planning
overhead is paid once at compile time.

- Baseline-vs-interpreter, same semantic work: ~1.5–3× across four
  independent systems (Deutsch–Schiffman 1984: 1.46×; JSC baseline: 2.3×
  per-op; SpiderMonkey and Sparkplug corroborate at whole-workload scale).
  The G-lane is *better* placed than those baselines: collection planning
  (phase-1 overload pruning over the token window) is precomputed into the
  descriptor, whereas the interpreter re-derives it per execution.
- boru's own T-lane measures 21–48× over the interpreter
  (`design/PERF-BASELINE.10.md`, Stage 6). The mosaic program keeps that
  wherever the checker has proof; G-lane events pay live lookup + kernel
  match — the components the interpreter also pays.
- Caching, staged after correctness: the per-name `Gen`-keyed dispatch
  cache is the monomorphic inline cache; a PIC (Hölzle, Chambers & Ungar,
  ECOOP 1991) is the polymorphic extension. Factor's measured caution
  applies — its PIC bought only 5–10% at runtime (the big win was
  elsewhere) — so measure before building (§11, F4).
- The known costs, priced: per-region DynEnv arming (bounded by region, not
  program); lazy first-apply compilation (Factor's trade — compile cost
  ~interpretation cost for run-once bodies, amortized by the planned
  structural unit cache, §6.7); the rebind-per-invoke pathology — a
  dependency rebound between every invoke forces a compile per invoke,
  which memoised restamp floors at ≈ interpreter cost for the same
  program, the correct floor; alloc ceilings stay gated
  (`lang/go/bytecode_allocguard_test.go`) and the facade-inlining
  discipline holds.

The step-budget divergence (`COMPILABLE-SUBSET.md` §7) remains the one
place the lanes are observably different programs at the ceiling; O2 asks
for the ruling totality forces.

---

## 8. Two lanes as a bug detector — keeping the gate while closing the gap

The prior note's warning stands: NUR101 was *found* by lane disagreement,
with the interpreter wrong. Collapsing to one semantics would lose the
detector. This design keeps it, on three legs:

1. **The interpreter survives as the reference oracle** — a sanctioned
   carve-out (check-mode front end, the C2 error oracle, the differential
   harness), just no longer a runtime lane. `RunInterp` remains callable
   under the harness and the explicit opt-out.
2. **Shared kernel ≠ shared driver.** The Engine's tape walk and the VM's
   instruction walk remain independent drivers of the shared routines;
   drift can still arise in the drivers and descriptor construction, and
   that is precisely what the differential corpus exercises. What sharing
   removes is the *silent re-implementation* class of drift — the class
   that produced the applyHandler episode.
3. **The corpus shifts from hand-picked to generated**, which is where its
   power already was (the 690-program sweep; the property fuzzer). The
   frontier ledger's rows graduate into ordinary spec rows; the ledger
   mechanism itself remains for any future regression, and the EMI-style
   move (mutate dynamically-dead code; re-phrase the same binding across
   call forms) extends it (Le, Afshari & Su, PLDI 2014; McKeeman 1998).

---

## 9. Ratchets and gates — how progress is measured

New ratchets, alongside the existing ones (all monotone, all in-tree):

- **`engineEntryCeiling`** — runtime `Engine` entries observed during
  compiled corpus runs, built on the shipped interpreter-entry attribution
  seam (`ArmInterpEntryHook`, `core/go/interp_entry.go:78`, and the
  unattributed-entry assertions in `lang/go/frontier_cases_test.go:120-140`)
  extended from attribution to census — the gate the completion plan names
  `TestNoInterpreterExecution` is planned there, not yet in the tree.
  Start = current live residue (§2.3); end = 0 outside carve-outs. This is
  T2's number. **Measured 2026-08-25 and now in the tree**
  (`test/go/langspec/engine_entry_census_test.go`, riding the
  compile-or-fallback walk): **505** unattributed interpreter runs across
  7,568 corpus rows, by route `Engine.Run×505` (the ground truth — every
  tree-walk emits it exactly once) with `CallBoru×282`,
  `vm:island×63`, `runPooledSub×37`, `InvokeCallback:callboru×34`,
  `RunResolved×31`, `vm:island-resolved×21` naming how it was reached.
  The 84 VM islands are the point: `islandCeiling=0` passed the whole time,
  because it counts only `OpFallback` spans in a disassembly. The two island
  chokepoints had to be given their own seams to be nameable at all —
  `runIslandResolved` was a hand-rolled copy of `core.RunResolved` that had
  dropped its seam emit, so every island reported the generic `Engine.Run`.
- **`deferCeiling`** — `vmDefer` activations on the corpus; end = 0, then
  the mechanism deletes. Measured 2026-08-25: **5** — `vm:poly-nout-drift×3`,
  `vm:poly-no-match×2`, both named in §6.10's retirement table.
- **Refusal-site census** — the recorder's `MarkUncompilable` call sites,
  counted by source scan (`test/go/langspec/refusal_site_census_test.go`):
  **96** at the Stage-1 baseline. This is the STATIC half — machinery that
  exists rather than machinery that fired — so it keeps falling while the
  corpus's runtime refusal count sits at zero. The lowerer/`Finalize` and
  `CompileCheck` layers surface as reasons in the corpus census and as
  pinned frontier rows; end = no refusal string reachable from `Finalize`
  (§5), not merely an empty recorder.
- **Frontier ledger** — rows graduate by the stale-arm mechanism, never
  hand-edits; family counts per §2.1 are the per-stage scorecard.
- **Parity gates** — `make verify-bytecode` (byte-identical differential
  incl. error taxonomy), the property fuzzer, whole-suite
  `--compile`==`--no-compile`, executed censuses (compile-time censuses
  are blind), and hand-pinned off-corpus regressions with no-`FALLBACK`
  disassembly asserts — all existing, all kept green throughout; the
  server-concurrency corpus (§13) joins them as it lands.
- **The performance register** (§14) — the permanent, host-keyed record
  every stage writes its before/after measurements into.

---

## 10. Staging

Ordered by dependency; each stage is separately shippable, gated, and
reversible. Rows-moved counts are the ledgered minimum (the unmeasured
universe closes alongside).

| Stage | Work | Closes | Risk |
|---|---|---|---|
| **0** | Adopt the declaration triple (COMPILE-DECLARATION-MODEL Stages 0–2: delete the dead flag, introduce `{tapeBound, needs, env}` under C1–C4, assert over every signature) | 0 rows; produces the §6.8 handler worklist | low |
| **1** | Instrument: engine-entry census + defer census + refusal-reason census; declare the observable alphabet for T3 | 0 rows; makes T2 measurable | low |
| **2** | **Extract the collection kernel** (§6.2): factor the THREE collection loops over the shared window+evaluator interface and re-seat the Engine on them — **three separate re-seats, landing separately**, since the differential cannot say which one broke otherwise. Gate: full differential green, allocation ceilings unmoved, CPU-profile share unmoved (NOT wall clock — see F1b) | 0 rows; unblocks everything | **high** — F1 · **Engine side LANDED 2026-08-26** in three commits; §6.2 records what each re-seat cost and where the third one corrected this note. **Gate discharged**: full differential green, allocation ceilings unmoved, merged `cover-gate` 100.0%, and CPU-profile share unmoved (F1b, §11 — every anchor within ±0.18pp against ±0.9–3.0 spread). The second adapter is Stage 4's, by the `cover-gate-core` inversion below |
| **3** | Universal fn values (§6.3, predicate units included) + the Apply kernel (§6.4, tail discipline included); retire `OpCallDynFrame`/`callDynamic` islands onto Apply. **NUR101's half of the interpreter-fix precondition is discharged and was never an interpreter fix**: measured 2026-08-27, the interpreter was already correct and the compiler carried five miscompiles (§6.4, design/PAREN-RESTEP-RULE.0.md). What remains of O1 is NUR078 alone. Also inherited from that work: four refusals whose graduation IS this stage — `DynApplyLeadEligible` must admit an EVENT lead — and three error-lane divergences pinned as measured (NUR107/108/109) that Apply's error contract has to settle | A (45), B (22), J (2), and five of G's seven (the fn-value island rows; `filter A.big` LANDED 2026-08-27 — not with registry-tagged captures, which its predicate does not have, but with the CheckState share of §6.3; the full-stack-in-body row with Stage 4's descriptor folds) | medium |
| **4** | Statement descriptors + `OpCollect`/`OpDispatchGeneric` (§6.2, §6.5) + bind twins; recorder step-6 flips from refuse to generic for word dispatch; delete drift-window islanding | F (5), L (1), K (1), most unledgered dispatch gates, §9d | medium |
| **5** | Production-order regions + generalized marks (§6.6) | C (11), D (13), I (5) | medium |
| **6** | Handler migration per the triple (§6.8): units-not-tokens, `while` lowering, per-region DynEnv, `args`/`__pa`/`context` frames | H (6), context/tape-bound gate families | medium — wide but enumerable |
| **7** | Runtime compilation everywhere (§6.7): computed bodies, splices, module bodies; the structural unit cache built here if not before (a hard dependency); unbounded memoised restamp; induction preconditions documented and fuzzed | eval-class gates | medium |
| **8** | Checker totality (§6.9): sentinel deletion, traps for definite errors, `!Compiling`-fork collapse, soundiness classification | E (8) | medium |
| **9** | Retire the valves (§6.10): defer sites → native answers; delete `OpFallback`/P7 machinery, the fence's re-run half, the fallback hatch; flip `CompileCheck` to total; `compile_refused` becomes a structured `internal_error` return (panics stay forbidden outside init-time registration) | T1, T2 complete | low by then |

The dependency spine is 2 → {3,4} → 5 → 9; stages 6–8 are parallel tracks
off it — **with one inversion the probe found**. `cover-gate-core` holds
`core/go` to 100% coverage *by its own suite alone*
(`Makefile`'s `CORE_GATE_FLOOR`), so any branch of the shared routine that
only the VM adapter reaches is dead code in core's profile and fails the
gate. The descriptor adapter therefore **cannot land until Stage 4 exists
to exercise it**: Stage 2 ships the interface and the Engine re-seat, and
the second adapter arrives with its client, not before. Two further
implementation constraints from the same probe: `MatchSignature` already
sits at gocyclo 87 / gocognit 211 under a `//nolint` against caps of
70/200, so a merged routine cannot be one function — **corrected
2026-08-31: that reading was itself paid down by re-seat 2 (54d8830);
the live measure is 68/131, UNDER the caps, the `//nolint` is deleted,
and `gocognit` ratchets 200 → 170**. The constraint's surviving form is
its inverse: the planner now has TWO gocyclo points of headroom, so
Stage-4 work that grows it extracts rather than expands, on written
triggers — (T1) a Stage-4 change needs new arms in the planner or would
breach a cap → extract the block it grows (stack-phase fill first, the
inside-forward fill SEPARATELY — its weaker per-position test is
behavior, not duplication), textually identically modulo parameter
substitution, in its OWN commit landing beside (never merged with) the
semantic commit, under the Stage-2 gate set; (T2) the admission-agreement
census (the Tier-1 instrument: engine-planner fill vs the kernel
`MatchSignature` at signature.go the G-lane opcodes call, with the
opposite window conventions mapped explicitly) ledgers a divergence that
needs a shared predicate → unify that predicate divergence-by-divergence,
one commit and one spec row per closure, never wholesale — these
closures move the interpreter's own answers, so the differential is
blind to them by construction and the spec rows are the gate; and the two largest
collection loops are 13.4% and 18.0% of interpreter CPU, which is the
budget any re-seat has to stay inside. **Read those two figures with
their basis in mind** — the probe recorded them without one, and "share"
means two different numbers depending on whether the denominator is all
samples or `Engine.Run`'s cumulative time. The 2026-08-26 gate run (F1b,
§11) records both explicitly, and on file: on the dispatch-dense set
`resolveForwardArgs` and `MatchSignature` are 15.3% and 10.0% of *total
samples*, 34.4% and 22.4% of *interpreter* CPU. Compare like with like,
or the budget moves under you. Nothing lands without its differential gate; every stage's admission
sweep is generated, not hand-picked (§8.3). Until a statement's enabling
mechanism lands, step 6's arm remains **partial** for it: such statements
keep refusing, loudly — a Stage-4 G-lane statement whose runtime-only
result width feeds a static seat still declines until Stage 5's regions
land. T1 is a Stage-9 property, not a rolling one.

---

## 11. Open questions (O) and what would falsify this (F)

**O1 — NUR101/NUR078 interpreter fixes. HALF CLOSED 2026-08-27, and the
way it closed is the finding.** This asked whether Stage 3's oracle was
safe, on the premise that both NURs needed interpreter changes to
ruled-but-unimplemented semantics.

NUR101 needed none. Measured against `RunInterp`, the interpreter already
implemented the ruled rule and the COMPILER carried five silent
miscompiles, in both directions — the register, this document's §6.4, and
the ruling built on them all had it backwards. The first implementation of
the ruling as written deleted `fnReturnPark`'s survivor-count clause and
broke seven suites; the clause was restored and the fix landed
compiler-side only (design/PAREN-RESTEP-RULE.0.md).

Two lessons carry into the remaining stages, both cheap and both
non-optional:

1. **Measure the oracle before building on a claim about it.** Every
   statement in this document of the form "the compiled lane is already
   the specified one" is a hypothesis until run against `RunInterp`.
2. **`RunInterp` is the oracle, and only `RunInterp`.** Stage J flipped
   `lang.Run` to the compiled path; 96 parity assertions across five files
   went on reading it as their interpreter side, comparing the compiler
   against itself and passing unconditionally (NUR106). That hole is what
   let the five miscompiles sit under a 100%-covered suite. It is swept,
   but nothing yet prevents the next Run-like flip from re-opening it.

What remains of O1 is **NUR078 alone** — and its own record notes the
ruling as written names `/r`, a modifier ADR-011 collapsed into `/v`, so
it needs re-spelling before it can be implemented as amended.

**O2 — the step budget.** Totality makes the per-instruction metering the
only live metering. Ruling needed: keep the documented one-directional
divergence against the oracle, or re-meter to match. Same for the
lane-divergent tape/stack ceilings the fn-value work measured.

**O3 — closure identity latitude.** If future optimization wants closure
caching/sharing, the spec must first grant Lua-5.2-style latitude;
until then the interpreter's allocation behavior binds.

**O4 — module-body execution at import.** Compile module bodies (Stage 7)
or declare import-time execution a front-end carve-out. This note leans
compile; the carve-out is coherent but must be stated.

**O5 — check-time budget exhaustion.** A program whose *check* pass
exhausts the budget has no emit-anyway story; totality needs one (likely:
widen-to-dynamic at the frontier of the exhausted region — but that must
be designed, not assumed).

**O6 — concurrency.** The runtime is one-registry-per-goroutine; detached
stamping runs under `ForkConcurrent`'s owning-goroutine contract
(`compiler/go/stamp_runtime.go:57-59`), and a `RestampBox` serialises
concurrent invokers of a shared signature across registries
(`:189-198`). Under "runtime compilation everywhere" plus
restamp-as-the-staleness-answer, the design owes rules for: registry
affinity of G-lane lookup and restamp under `spawn`/`await`, restamp-box
sharing when different callers' registries disagree, `DefTable.Gen` cache
reads as §7's inline-cache seed, and which `-race` gates each stage keeps
green.

**F1 — the collection kernel cannot be extracted. TESTED 2026-08-25:
HOLDS, with corrections.** The probe (four agents over the collection
machinery) found the extraction possible but this note's description of it
wrong in three ways — the unit is a region and not a statement, there are
three collection loops and not one, and planning is not pure. §6.2 now
carries the corrected form and the wider interface it forces (a mutable
spliceable window plus a host evaluator, not an abstract slot array). The
falsifier stays live for the *implementation*: if that interface cannot be
built without behavior change, the design degrades to window dispatch,
which §9d proves unfaithful. Stage 2 is first for this reason, and its
gate is the full differential on the *interpreter* side alone.

**F1b — the extracted routine slows the interpreter. TESTED 2026-08-25:
DOES NOT FIRE — but it exposed that the gate was unmeasurable.** The
probe built the abstraction (a token-window accessor plus the host
evaluator callbacks the corrected §6.2 demands) against a scratch copy of
the tree and measured it on this repo's own interpreter benchmarks: zero
extra allocations, no measurable time. Interface dispatch is not the
risk.

What the measurement did establish is that **Stage 2's gate as written —
"prove no interpreter regression (perf + full differential)" — cannot be
discharged on a shared container**. Two runs of *byte-identical code*
differed by 5.4% geomean, with five of six shapes reporting
"statistically significant" changes between −16% and +9%; at higher
repetition counts the instrumented build measured 12.9% *faster* than
baseline on the most collection-dense shape, which is obviously spurious.
Wall clock on this class of host cannot resolve the effect the falsifier
is about. The gate is therefore re-specified onto the deterministic
instruments — the allocation ceilings, which are exact, and CPU-profile
share, which is comparative — with wall clock recorded to the register
(§14) rather than gating. A gate that fires at random is not a gate.

**The re-specified gate, MEASURED 2026-08-26: PASSES.** `6d8f2f7`
(pre-extraction) against `89a7931` (the branch tip, all three re-seats
landed), same host, alternating runs, `n=3` profiles a side. Share of
*interpreter* CPU — each anchor's cumulative time over `Engine.Run`'s,
which is the basis that divides out any uniform shift in how much of the
process is interpreter at all:

| Anchor | `6d8f2f7` | `89a7931` | Δ | run-to-run spread |
|---|---|---|---|---|
| `stepWord` | 89.01% | 88.90% | −0.11 | ±2.98 |
| `MatchSignature` | 22.45% | 22.63% | +0.18 | ±2.12 |
| `resolveForwardArgs` | 34.44% | 34.61% | +0.17 | ±2.16 |
| `stepLiteral` | 12.02% | 12.16% | +0.14 | ±0.88 |

Every movement is an order of magnitude below the spread of the runs it
was measured from, so the honest reading is **no resolvable change**, not
"a 0.17-point regression". Three details matter for whoever re-runs this
at Stage 4:

- **The anchors are the CALLERS, not the seam.** `collectForward`,
  `collectCandidateScan` and `collectArrival` exist only after the
  extraction, so they compare to nothing; they are called by these three,
  and whatever they cost lands here. `resolveForwardArgs` is now
  `(inline)` in the tip's profile with `collectForward` carrying the
  identical cumulative time — the two-line seat costs nothing, which is
  the closest thing to direct evidence that the re-seat was free.
- **The benchmark had to change to make the gate sensitive.**
  `BenchmarkPerfWords` — the obvious choice, since it is the *collection
  word* suite — is the wrong instrument: its 500-element inner work
  swamps dispatch, putting the loops at 3–5% of samples where a real
  regression hides inside the spread. Measured there, the same comparison
  reads +0.40/+0.43/−0.27 against noise bands of 1.4–1.6 — the same
  verdict, at a quarter of the resolution. The dispatch-dense interpreter
  set (`BenchmarkParens`, `BenchmarkBytecodeBaseline`, both on
  `RunInterp`) puts them at ~34% and ~23% of interpreter CPU instead.
- **`collectArrival` is not covered by this gate.** It has *zero*
  samples in either profile and is not inlinable, so the arrival path is
  simply not hot in any benchmark this repository has. That is a gap in
  the instrument, not a pass: a regression there would be invisible here.

The instrument is committed as `bench/register/cpushare.sh` and appends
to the register (§14) rather than living in the note, precisely so Stage
4 — which touches all three loops again — re-runs it instead of
re-deriving it. Rows are on file for both commits above.

**F2 — error identity needs state outside the collection machine.
TESTED 2026-08-25: FIRED.** This note claimed the raise-selection state
was exactly the collection machine's register state. It is not, and the
counterexample is two statements:

```
$ boru -no-check -no-compile -e 'def one fn [[a:Integer][Integer][a]] end
      def k w:Integer => [mul 2 w] end one k'
  = note: candidate `one (Integer)` takes 1 argument, but none were supplied

$ boru -no-check -no-compile -e 'def one fn [[a:Integer][Integer][a]] end
      def k w:Integer => [mul 2 w] end "zz" end one k'
  = note: the argument was 'zz' (a ProperString)
  = note: candidate `one (Integer)` — argument 1: expected Integer, got 'zz' (a ProperString)
```

Same failing dispatch, same collection registers — a different
diagnostic, because `reorderCandidates` reads up to four values off the
**enclosing value stack** and the residual `"zz"` belongs to a *previous
statement*. It is in no descriptor, no claimed window, and no
`ForwardInfo`. A second counterexample moves the error **code** rather
than its text: `e.voidGroups`, appended when a paren group collapses to
zero values, is consulted before anything else and turns
`signature_error` into `def_error`.

This does not abandon the design: the extra state is finite and
carryable, and the compiler already ships a precedent for handing
error-rebuild state to the VM (`PolyNoMatchSpec`). But §6.2's
*inventory* was wrong and is corrected there — and that precedent's own
escape, deferring to the interpreter on drift, is exactly what T2
forbids, so the widened form has to be complete rather than
best-effort.

**F3 — a `tapeBound: Yes` handler that cannot be rewritten.** If some
handler's semantics genuinely require re-stepping its operand on the live
tape (not a unit, not a reified env), T2 and totality collide on that word
and the directive itself must arbitrate. No such word is currently known;
`var` — the historical example — has a structured lowering.

**F4 — G-lane slower than the interpreter.** If a shipped Stage-4 generic
region measures slower than the tree-walker on like-for-like programs, the
premise "descriptor precomputation ≥ interpreter's re-derivation" is wrong
in this VM; the design still holds semantically but the performance story
reverts to T-lane coverage pressure. Benchmark at Stage 4, not at the end
— into the register (§14), so the verdict is host-keyed and durable.

**F5 — runtime compilation breaks the induction.** §6.7's induction rests
on named preconditions: §5's Finalize totality, an answer for O5's
check-budget exhaustion, the fragment-depth ceiling (`lower.go:2041`), and
the eligibility/policy gates around detached stamping. If any
runtime-compile path can still reach a shape that refuses — or if the
memoised-restamp policy cannot hold its cost floor — totality has a hole;
the fix is extending the G-lane (or the demotion rule) to that shape, and
the fuzzer must hunt for it explicitly.

---

## 12. Worked example — a callback server, statement by statement

The sharpest acceptance test for this design is the workload boru's own
benchmarks already care about: a Node-style callback server. The tree
contains its two poles today. `bench/networking/echo_boru.boru` — a
`Net.serve-raw` handler whose body the callback-compilation seam runs on
the VM — measures **~65,100 req/s compiled vs ~884 interpreted (~74×),
within ~1.7× of hand-written Go** (`bench/networking/README.md`). And
`design/examples/apps/mini-redis.boru` is a full protocol server (custom
codec, patrun-routed commands) with the same architecture. Both compile
**because they avoid the Node idioms**. Add the two most Node-shaped moves
— composed middleware and a `while` read loop — and today the *whole
program* refuses, landing every request back at ~884 req/s. That cliff is
the design's target.

The program (real surface syntax; `auth`/`route` bodies elided):

```
import "boru:net"
import "boru:fn-util"

def auth  fn line:String String [ … pass or reject … ]        # S1
def route fn line:String String [ … dispatch on the verb … ]  # S1
def handle (FnUtil.compose route/v auth/v)                    # S2

def ln
  (Net.serve-raw {tcp:8080}                                   # S3
      (fn sock:Socket Any                                     # S4
          [def nl (convert Bytes "\n")
            while [ … more input … ]                          # S5
              [def line (convert String (Net.recv-until sock nl))  # S6
                def reply (handle line)                       # S7
                Net.send-bytes (convert Bytes reply) sock     # S8
              ]
          ]
      ))
```

Statement by statement:

| Stmt | Interpreter semantics | Today | Under this design |
|---|---|---|---|
| S1 | typed fns; bodies checked, units built | compiles (T-lane `CALL_USER` units) | unchanged |
| S2 | `compose` receives two fn *values* (`/v`), stores them, returns a composed fn value — invoked later from Go, never on the tape | **refuses**: `function-valued operand at FnUtil.compose (Stage 3)` — the missing `CompileStoresFn`-class declaration, the fn-util defect | declaration triple says `tapeBound: No` (§6.8, Stage 0); the composed value is data over two units — Factor's `composed` cell (§6.3, Stage 3) |
| S3 | `serve-raw` stores the handler fn; the Go accept loop owns it | compiles; store-site callback stamping is shipped (`RuntimeStampingEnabled`) | unchanged, minus the `CallBoru` fallback arm |
| S4 | the connection callback: a closure, invoked per connection **after the program ends** | compiles; `InvokeCallback` → `RunUnit` — this seam *is* the measured ~74× | unchanged; the busy-registry decline arm retires (§6.7) |
| S5 | `while [cond] [body]`: per-iteration condition re-eval, body values accumulate, `break`/`continue` | **compiles** (the thirty-seventh increment, 2026-09-09: the condition loop on the counted loop's frame — `RecordWhile`, a per-iteration condition fragment, a falsy exit through `FLOW_BREAK`; was **refuses**: `code-body word while (Stage 2)` — family H) | structured `WHILE` lowering (§6.8, Stage 6) |
| S6, S8 | socket words, typed operands | compile (T-lane `CALL_NATIVE` — the echo bench proves it) | unchanged |
| S7 | `(handle line)`: a **name-read lead** — WORD dispatch; the live binding of `handle` applies, every request | **refuses**: `def-bound computed fn apply (closure shape unknown — Stage 1)` — the NUR101 wall; this is the Express-middleware idiom, 100% refused today | §6.4 Apply + §6.5 generic dispatch: live def-stack lookup, then unit entry — G-lane, cacheable (§7) — Stage 3/4 |
| S9 (hot swap) | a reload re-runs `def handle (FnUtil.compose route2/v auth/v)`; the next request sees v2 because S7 re-resolves the name | rebind gates refuse or the program was interpreted anyway | bind twins + live lookup make it native; in-flight connections finish on the old unit (frame-boundary cutover, §4), the next S7 dispatch sees v2; staleness costs one memoised restamp per world (§6.7) |

And the runtime path, end to end: the callback stamps at its store site
(shipped); steady-state dispatch invokes the unit with the registry idle
between events (shipped — the echo number); a handler that synchronously
triggers *another* callback hits the busy-registry route, which is the
re-entrant-hosting item (§6.7/§6.10) — the one genuinely new runtime
mechanism this program needs; a recv timeout raises the same
`[boru/timeout]` error with the same blame, propagated, never re-run
(§6.10); and the read loop runs constant-space under Apply's tail
discipline (§6.4). Under the design the program compiles **whole**: S7 is
the only G-lane event in it, paying one live lookup per request against
an otherwise fully typed lowering.

---

## 13. The server-concurrency test corpus

**The goal is that every callback case compiles — all of them, fully.**
Each case below is a graduation gate, not an aspiration: it passes only
when it runs with zero runtime `Engine` entries (the §9 census armed for
the whole run), byte-identical protocol transcripts against the
interpreter oracle, and green under `-race`. Echo and mini-redis prove
the *sequential* callback path; nothing in the tree pins the concurrent
and lifecycle cases — and the corpus's absence is already visible as CI
noise: `TestTuiServeAllViewersGoneQuits` (a viewer-lifecycle race) flaked
on this very PR's doc-only diff, and `TestServeStepShutdownDrains` (a
debug-server shutdown drain) flaked under CPU contention during the
Stage-1 verification run — both pass instantly standalone. Two
independent server-lifecycle timing tests failing for load rather than
logic, inside one work session, is the argument for case 11 below. Build the corpus on the existing apps
(`bench/networking/`: echo, `echo_redis`, `echo_s3`;
`design/examples/apps/`: mini-redis, mini-s3, todo-api), one case per
concurrency shape:

1. **Steady-state sequential dispatch** — echo. **Built and measured
   2026-08-25** (`test/go/servercorpus/callback_census_test.go`): a boru
   program starts a TCP echo server, connects to itself, exchanges three
   framed messages over the per-connection handler and closes. Compiled
   result and interpreted result are identical, and the compiled run
   performs **zero** unattributed interpreter runs — the handler executes
   entirely on the VM. This case is therefore already at its end-state
   ceiling rather than ratcheting toward one, which answers the plain
   form of "does a callback server compile completely?" with yes. The
   cases below are the shapes that answer is not yet known for.
2. **Protocol framing / codec re-entry** — mini-redis. **Built and
   measured 2026-08-25** (`test/go/servercorpus`), and the result is the
   sharpest evidence in this note: the app **does not compile at all**,
   and runs 51 unattributed interpreter runs. The reason is a single
   check diagnostic — `undefined_word: h2` at
   `design/examples/apps/mini-redis.boru:210`. The interpreter binds and
   resolves that name; the checker does not, and the whole-program
   sentinel converts that into a refusal.

   **Diagnosed 2026-08-26** (NUR103), by delta-debugging the app rather
   than writing reductions: of the fourteen registered handlers, `HDEL`
   alone reproduces — and `HSET`, the site the message NAMES, does not.
   Three faults compose, and the first is not a parity nicety but a
   COVERAGE HOLE: **`boru check` does not analyse a service-handler body
   at all**. A bare undefined word inside one is reported clean by `boru
   check` and refused by the compiler, so a typo in a request handler
   ships. The compile pass must read that body — it records it into a
   compiled callback unit — so every divergence follows from that one
   asymmetry. On top of it, a call the checker models as DIVERGENT
   silently unbinds its `def` (so the later read is undefined), and the
   `no_signature` that would name the failing call is suppressed while
   compiling — which is why the escaping diagnostic is at a different
   line, about a different name, than the cause. Twelve self-contained
   lines reproduce it; NUR103 carries them.

   So the gap between "a callback server compiles" and "a realistic
   callback server compiles" is not, in this instance, any of the
   higher-order machinery this note spends its length on. It is frontier
   family E — one checker limitation, tripping the sentinel §6.9(1)
   deletes. Echo compiles to zero because it never trips it. That is an
   argument for sequencing Stage 8 earlier than its position in §10
   suggests: it is cheap relative to the spine and it is what stands
   between a real server and the compiled lane today. The diagnosis
   sharpens that argument rather than softening it: fault 1 is a
   user-facing checker hole that wants fixing whether or not full
   compilation lands, and fault 2 — should a provably-divergent value
   expression bind a `Never` carrier instead of nothing? — is a question
   the totality model has to answer anyway.

   Extend the case with partial frames (the codec's `{need:1}` path
   re-entered mid-message) and pipelined commands once it compiles.
3. **Concurrent connections** — N simultaneous clients, handlers on
   multiple goroutines: the one-registry-per-goroutine rule under load;
   the O6 design item's test bed.
4. **Nested synchronous callback** — a handler that calls a service whose
   reply invokes another boru callback before the first returns: the
   busy-registry route (`CanHostVM` decline), i.e. §6.10's re-entrant
   hosting row, exercised deliberately.
5. **Handler re-entrancy** — a handler whose body applies itself (or its
   own composed chain) recursively.
6. **Hot swap under load** — rebind the routed handler mid-traffic:
   in-flight connections complete on the old unit, the next dispatch sees
   the new one, and restamp cost is one compile per world (§6.7) —
   asserted by counting compiles, not just answers.
7. **Fan-out** — `spawn`/`await` inside a handler; `await first`/`any`
   winner residuals (family I's mark regions) driving a reply.
8. **Deadlines and timers** — recv timeouts and timer callbacks: the
   `[boru/timeout]` error identity and blame position, compiled vs
   interpreted.
9. **Error paths** — a raising handler per connection: same error, same
   catchability, connection teardown identical, and no whole-program
   re-run behind it (the C1 fence's retirement made observable).
10. **Long-lived connection / soak** — hours-scale run: constant-space
    read loops (tail discipline, §6.4), no drift-triggered degrade to
    interpretation ever (`engineEntryCeiling` stays 0 for the whole
    soak), GC stability.
11. **Session/viewer lifecycle** — the TUI-serve class: all clients
    disconnecting, reconnecting, half-closed sockets — pinned
    deterministically, replacing today's timing-sensitive test.

Every case doubles as a **register workload** (§14): the corpus is both
the correctness gate and the performance instrument, so a stage that
graduates a case also records what that graduation cost or bought.

---

## 14. The performance register

The measurement discipline today is good at *relative* answers
(`benchstat` before/after, the alloc-ceiling gates in `make test` —
`design/PERF-BASELINE.10.md`) and bad at *longitudinal* ones: snapshots
live as prose in design notes, and `bench/networking/README.md` already
carries both warnings this section exists to fix — "absolute req/s track
the box", and a superseded row that had silently measured the wrong lane.
A ten-stage compiler rebuild needs a permanent, host-honest record.

**The register.** `bench/register/` holds two committed, append-only
JSONL files:

- `hosts.jsonl` — one record per distinct host: `host` (the id — a short
  stable hash over the normalized identity tuple: CPU model, physical
  core count, memory, OS name and major version, architecture,
  virtualization class), plus the full spec the id was derived from —
  CPU model string, cores/threads, RAM, storage class, OS and kernel
  versions, bare-metal/VM/container, CPU governor where known, and a
  free-text label.
- `measurements.jsonl` — one line per measurement:
  `{ts, commit, host, surface, workload, metric, value, unit, n,
  spread, benchtime, flags, go, os_version}`. `os_version` and `go` ride
  on every row because a host drifts under a stable id (patch levels,
  toolchains). `surface` is one of **`check`** (the static pass —
  `BenchmarkPerfCheck`), **`compile`** (emit + lower cost —
  `BenchmarkPerfCompile`), **`interp`** (interpreted execution),
  **`exec`** (compiled execution — the Stage-6 suite), and **`e2e`** (the
  §13 server workloads: req/s, µs/round-trip). All three implementation
  surfaces — interpreter, checker, compiler — are first-class, so a
  change that buys execution speed by spending check time is *visible*.

**Mechanics.** A `make bench-register` target runs the suites and the
§13 workloads, derives the host id, and appends rows; a verify step
asserts the files are append-only (existing lines byte-identical — the
kg digest discipline applied to measurements). The register **records,
never gates**: execution time is too noisy to fail CI on, so the
deterministic alloc ceilings remain the only perf *gates*, and the
register is the memory. A pinned CI runner class may contribute rows
under its own host id; developer boxes contribute under theirs.

**Reading it.** Absolute comparisons are valid only within one host id;
across hosts, only ratios travel (compiled/interpreted, boru/Go,
check-cost/exec-cost) — so rows record the absolutes and reports derive
the ratios. A small boru tool (`bench/register/report.boru`, in the kg
pipeline's dogfooding tradition) renders per-`(host, workload)` time
series over commits and flags a value outside the trailing window's
spread. Permanence is the point: rows are never edited or deleted — a
measurement discovered to be wrong is *superseded by a new row* naming
it, the same discipline the frontier ledger uses — so "did Stage 4's
descriptors slow the interpreter?" (F1b) and "what did Stage 3 buy on
mini-redis?" stay answerable years later, on the hosts that measured
them.

Every stage in §10 lands with register rows on at least one host: the
before/after pair is part of the stage's deliverable, and F4's Stage-4
benchmark is simply the first mandated pair.

---

## 15. Related work

**In-tree:** `COMPILABLE-SUBSET.md` (the subset this note totalizes);
`COMPILE-DECLARATION-MODEL.0.md` (the declaration triple, adopted; typed
islands, rejected); `COMPILE-REFUSAL-SURVEY.0.md` (dispatch opcodes
necessary but insufficient — the finding §6.2/§6.6 answer);
`RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md` (the doctrine and the defer
worklist); `HIGHER-ORDER-FUNCTIONS.0.md` (§9d and the §9g generated-sweep law);
`FUNCTION-VALUE-SCOPE.0.md` (the env axis); `NUR.md` NUR101/NUR078
(NUR037 survives outside `NUR.md` — the ledger note at
`design/HIGHER-ORDER-FUNCTIONS.0.md:1197`. NUR067 did too, in the
`frontier-await-winner.tsv` ledger entry, and is CLOSED as of 2026-09-10:
both the file and the entry are deleted, and its record is the
frontier_spec_test.go note that replaced them plus this note's §6.6 and the
handoff's thirty-ninth-to-forty-first increments);
`DO-STRUCTURE-COMPILATION.0.md` (the "always compile" directive);
`HOT-CODE-LOADING.0.md`; `STAGE3-INLINING-DESIGN-ROUND.0.md` (one
recording path; the third architecture); `VOXGIG-COMPILE-LEAVES.1.md`
(the differential-reverted recovery-site attempts — the "DO NOT RETRY"
record §6.2 answers); `AOT-COMPILE.0.md` and
`INTERPRETER-TIERED-EXECUTION.0.md` (adjacent, unimplemented tiers).

**Systems:** Pestov, Ehrenberg & Groff, "Factor: A Dynamic Stack-based
Programming Language", DLS 2010, 43–56. Dybvig, "The Development of Chez
Scheme", ICFP 2006. MacLachlan, "The Python Compiler for CMU Common Lisp",
LFP 1992. Bezanson et al., "Julia: Dynamism and Performance Reconciled by
Design", PACMPL 2(OOPSLA):120, 2018. Belyakova et al., "World Age in
Julia", PACMPL 4(OOPSLA):207, 2020. V8 Sparkplug (v8.dev/blog/sparkplug);
JSC "Speedometer 2 and the JavaScriptCore baseline" (webkit.org/blog/10308);
Hermes (facebook/hermes). Siskind, Stalin; MLton
(mlton.org/WholeProgramOptimization) — the closed-world contrast.

**Theory:** Futamura 1971 (repr. HOSC 12(4):381–391, 1999). Jones, Gomard
& Sestoft 1993. Reynolds, "Definitional Interpreters for Higher-Order
Programming Languages", 1972 (repr. HOSC 11(4):363–397). Danvy & Nielsen,
"Defunctionalization at Work", PPDP 2001. Johnsson, FPCA 1985. Appel,
*Compiling with Continuations*, 1992; Shao & Appel, LFP 1994. Minamide,
Morrisett & Harper, "Typed Closure Conversion", POPL 1996. Shivers,
CMU-CS-91-145, 1991. Van Horn & Might, "Abstracting Abstract Machines",
ICFP 2010; Van Horn & Mairson, ICFP 2008. Deutsch & Schiffman, POPL 1984.
Chambers & Ungar, PLDI 1989. Hölzle, Chambers & Ungar, ECOOP 1991; PLDI
1992. Kleffner, MS thesis, Northeastern, 2017. Diggins, "Typing Functional
Stack-Based Languages", 2008. Pöial, EuroFORML 1990. Rice, Trans. AMS
74(2):358–366, 1953. Ager, Biernacki, Danvy & Midtgaard, BRICS RS-03-14 /
PPDP 2003. Leroy, POPL 2006. Pnueli, Siegel & Singerman, TACAS 1998;
Necula, PLDI 2000. McKeeman, DTJ 10(1), 1998; Yang et al. (Csmith), PLDI
2011; Le, Afshari & Su (EMI), PLDI 2014.

**Checker totality:** Cousot & Cousot, POPL 1977; PLILP 1992. Cartwright &
Fagan, PLDI 1991; Wright & Cartwright, TOPLAS 1997. Lindahl & Sagonas,
PPDP 2006. Siek & Taha, Scheme Workshop 2006; Siek et al., SNAPL 2015.
Wadler & Findler, ESOP 2009. Takikawa et al., POPL 2016. Vitousek, Swords
& Siek, POPL 2017. Chung et al., ECOOP 2018. Herman, Tomb & Flanagan,
HOSC 2010. Tobin-Hochstadt & Felleisen, ICFP 2010. Matthews & Findler,
POPL 2007. Bracha, OOPSLA 2004 workshop. Hackett & Guo, PLDI 2012.
Castagna et al. (Elixir set-theoretic gradual types), ⟨Programming⟩ 8(2),
2024. Livshits et al., "In Defense of Soundiness", CACM 58(2), 2015.
Darais et al., "Abstracting Definitional Interpreters", ICFP 2017.
