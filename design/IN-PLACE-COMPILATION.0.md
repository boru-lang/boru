# In-place compilation — the interpreter writes compiled call cells into its own tape

**Status:** prototype **built**, 2026-10-06. Interpreter mode only, off by
default: `BORU_INPLACE=1` (or `verify`) in the environment,
`--options inplace:on|verify|off` on the CLI, `lang.Options.InPlace` for a
host. §1–§11 are the design as written before the build; **§12 records
where the build departs from it, how it was verified, and what it
measured.** The maintainer answered the first question — *no rearrangement
of the arguments into canonical Forth order before the compiled cell is
written; forward matching is sufficient* — and the build follows it; the
other defaults (§2) stay open for the maintainer to overrule (§11).

> **Scope note.** This is a performance mechanism *inside the tree-walking
> interpreter*: how a matched call is executed on the tape. It is a
> different axis from the full-compilation project's definition of done
> (T1–T4, `design/SESSION-HANDOVER.0.md`) and changes nothing about the
> compile-mode refusal contract — a program `--compile` refuses is still a
> failure, and the interpreter is still not a fallback. The interpreter
> remains the semantic oracle for the compiled lane, so the mechanism's
> contract is: **results, errors and carets are byte-identical with it on
> and off.** Only `IO.trace` output and the engine's step count may differ.
> It is also finer-grained than, and composes with,
> `design/INTERPRETER-TIERED-EXECUTION.0.md` (which promotes whole hot fn
> bodies to the VM): in-place compilation speeds up every dispatch the
> tree-walker still performs, including the top-level statement stream.

## 1 What a call costs today, measured

The tape is a gap buffer of 104-byte `core.Value` cells (`core/go/tape.go`).
A dispatch is one call to the kernel's planner (`PlanMatch` through
`resolveForwardArgs` → `CollectForward`) and then a sequence of tape edits
that move the arguments into the order a stack-only re-match reads. Traced
with `IO.trace` on the baseline (`ed8a805ed`):

```
1 add 2                                  add 1 (mul 2 3)
  0 [ ^1 add 2 ]                           0 [ ^add 1 paren([word(mul) 2 3]) ]
  1 [ 1 │ ^add 2 ]                         1 [ add →add(0/2) │ ^1 6 ]        forward→ add(Number, Number)
  2 [ 1 add →add(0/1) │ ^2 ]  forward→     2 [ 1 add →add(1/2) │ ^6 ]        collect add 1/2
  3 [ 1 2 │ ^add/s ]          collect 1/1  3 [ 6 1 │ ^add/s ]                collect add 2/2
  4 [ ^3 ]                    stack add    4 [ ^7 ]                           stack add(Number, Number)
```

Step by step, for `add 1 2 7 8 9` (the shape that showed the cost on
2026-10-05 — 7 structural edits and 10 cross-gap copies, against 1 splice
and 0 copies for `1 2 add end`):

| what                                            | where                                   | tape edits                              |
|-------------------------------------------------|-----------------------------------------|-----------------------------------------|
| plan the window, choose `add(Number, Number)`    | `stepWord` → `resolveForwardArgs` → `PlanMatch` | — (first plan)                   |
| park the forward marker after the word          | `insertForward`                         | `Insert` (gap move to the pointer)      |
| `1` arrives: move it before the word            | `stepLiteral` arrival branch            | `Remove`, `Insert`, `Set` (new marker)  |
| `2` arrives: move it before the word            | same                                    | `Remove`, `Insert`                      |
| complete: drop marker, rewrite word to `add/s`  | same, `CollectedArgs >= ExpectedArgs`   | `Remove`, `Set`                         |
| reorder so the first forward arg is on top      | `rearrangeForForward`                   | `Set` × 2                               |
| re-step `add/s`: plan again, stack-only         | `stepWord` → `PlanMatch`                | — (second plan)                         |
| run the handler, compact, splice `[2 1 add]`→`[3]` | `execMatch` → `spliceMatchResults`   | `Set` × k (compaction), `Splice`        |

Every `Remove`/`Insert`/`Splice` is `O(gap distance)` — cheap when the gap
already sits at the pointer, a full copy of the tail the first time a
statement edits far from it — and the compaction in `spliceMatchResults`
re-copies the span. The CPU profile of the loop fixture (2026-10-05)
puts `runtime.duffcopy` (104-byte Value copies) at 36% flat and the
planner at 54% cumulative (`CollectCandidateScan` 44%, `CollectForward`
28%); the second plan at completion is part of that 54%.

## 2 Decisions

| # | question                                     | decision for the prototype                                                                                                 | source |
|---|----------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|--------|
| 0 | reorder the arguments before compiling?      | **No.** The arguments stay where forward matching found them; the call cell binds them in signature order from their positions. | maintainer, 2026-10-06 |
| 1 | lifetime of the compiled cell                | **One execution.** The cell is executed by the very next step and replaced by its results. A call-site *plan* cache that survives replays is phase 3 (§9). | default |
| 2 | what the cell holds                          | **The matched dispatch**: signature, handler owner, argument values in signature order, the word's position. Executed by `execMatch` — the same path every dispatch takes today. Not VM bytecode: `core` cannot import `eng`/`compiler`. | default |
| 3 | argument capture                             | **Values present at match time.** Argument post-processing (auto-evaluation of `Eval` lists/maps, ascription strip, option defaults) still happens when the cell executes, in `execMatch`, exactly where the re-step does it today. | default |
| 4 | which cell, where results land               | **The last cell of the call's span in tape order** holds the call: the last forward argument when there is one, else the word (so a nullary word and a call fed only from the value stack compile *into the word's cell*). Results replace that one cell: one result is a `Set`; none makes it a nop; several are one `Splice` of that single cell. | maintainer's rule, made total |
| 5 | what nop is invisible to                     | Nop cells persist until the region they sit in is collapsed, replayed or drained. Every scan that indexes the tape skips them (§4). The prototype runs only on the plain run lane: analysis passes and the stack-form recorder keep today's code paths. | default |
| 6 | which words                                  | Phase 1: every **word** dispatch whose signature is not `FullStack`, including user fns (their frame tokens replace the cell by one splice) — natives are simply the common case. Value-called functions (`execFnDefLiteral`) and macros keep the legacy path. | default, widened: nothing in the mechanism is native-specific |
| 7 | the oracle                                   | Results, errors and carets byte-identical. `IO.trace` shows the new cells (`module-io.tsv:37` pins only the value); `Debug.steps` is pinned by shape only (`module-debug.tsv:10`). | default |
| 8 | success                                      | Tape edits and copies per call, interpreter ns/op and allocs/op on the pinned shapes, and the full spec corpus run interpreted both ways (§7, §8). | default |

## 3 The mechanism

### 3.1 The rule

When a word's call is **fully matched** — the planned signature has every
slot filled, whether a slot came from the forward stack (tokens written
after the word, arriving at the pointer) or from the value stack (resolved
cells below the word) — the engine moves, removes and splices nothing. It
overwrites cells, each with `Tape.Set`, which is `O(1)` and never moves the
gap:

1. the **last cell of the span** becomes the **call cell**: the compiled
   call (§3.2);
2. **every other cell the call claimed** — the word, its forward marker,
   the other forward arguments, the value-stack arguments — becomes a
   **nop cell**;
3. the pointer is set to the call cell.

The next step executes the call cell: the handler runs over the matched
arguments and the results replace the cell, with the pointer left on the
first result — exactly where today's splice leaves it (the first
argument's index collapses to the same cell once the span is spliced
away). The three cases:

| results | edit                                    | pointer                         |
|--------:|-----------------------------------------|---------------------------------|
| 1       | `Set(cell, r)`                          | stays on `r`, re-stepped        |
| 0       | `Set(cell, nop)`                        | the nop arm advances it         |
| k ≥ 2   | `Splice(cell, 1, r₁…rₖ)` — a fn frame   | stays on `r₁` (the frame's `(`) |

Indices stay stable through the whole call: no `funcIdx--`/`fwdIdx--`
bookkeeping after removals, because there are no removals.

### 3.2 Two new cell kinds

Both are engine markers under `Word/__IN`, so every predicate that already
treats `TInternal` as "not a value" (`isEngineMarker`,
`IsRecordableLiteral`, `ForwardLiteralOperand`, the `ReorderCandidates`
break) classifies them as markers by default. Each then gets explicit arms
where a marker would otherwise *stop* a scan that should pass over a nop
(§4).

**Nop** — `TNop = mustType("Word/__IN/__NP")`, payload `NopInfo{}`. The
payload is a zero-size struct, so `Data != nil` (no predicate can mistake
the cell for a bare type node, as it could a payload-less marker) and
boxing it allocates nothing. One package-level `nopCell` value is written
everywhere; `IsNop(v)` is `v.Parent.Equal(TNop)`.

**Call** — `TCall = mustType("Word/__IN/__CL")`, payload `*CallInfo`:

```go
type CallInfo struct {
    Name  string      // the dispatched word, for tracing and errors
    Sig   *Signature  // the matched signature (the plan's, see R1)
    Reg   *Registry   // MatchResult.Reg: a module delegation's owning registry, nil = main
    Args  []Value     // in SIGNATURE order — forward args in written order, then stack args top-down
    Stack int         // how many of Args came from the value stack
    Lo    int         // the first claimed cell: the span is [Lo, cell]
    argv  [4]Value    // Args' backing store for calls of up to four arguments: one allocation per call
}
```

`NewCall` stamps the cell with the **word's** source position (`pos`), so
`currentPos()`, `stampErrPos` and `stampResultPos` — which all read the
cell at the pointer — report the call exactly as they report the word
today. `IsCall`, `AsCall` as for the other marker kinds; `TraceColorize`
renders a nop as a dim `·` and a call as `⟨name arg…⟩` in the forward
marker's colour.

### 3.3 Worked examples

`·` is a nop cell, `⟨…⟩` a call cell with its arguments in signature
order, `^` the pointer.

**A — flat forward call** `add 1 2 7 8 9`:

```
^add 1 2 7 8 9                     plan: add(Number, Number), two forward slots
add →add(0/2) ^1 2 7 8 9           Insert the marker                      (1 Insert)
add →add(1/2) 1 ^2 7 8 9           1 arrives: count it, leave it           (1 Set, the marker's count)
·   ·         · ^⟨add 1 2⟩ 7 8 9   2 arrives, 2/2: complete               (4 Sets)
·   ·         · ^3 7 8 9           execute                                (1 Set)
·   ·         · 3 ^7 8 9           3 is re-stepped as a literal, as today
```

One `Insert`, six `Set`s, one plan. Today: seven structural edits, three
`Set`s, two plans.

**B — mixed call** `1 add 2` (one slot from the value stack):

```
1 ^add 2                           plan: sig[0] ← 2 (forward), sig[1] ← 1 (stack)
1 add →add(0/1) ^2                 Insert
· ·   ·         ^⟨add 2 1⟩         complete: the stack cell is claimed too   (4 Sets)
· ·   ·         ^3
```

Signature order is the one rule of `CLAUDE.md`: forward slots first in
written order, then the value stack top-down. `10 sub 3` is
`⟨sub 3 10⟩` = `args[1] − args[0]` = 7; `sub 1 3` is `⟨sub 1 3⟩` = 2.

**C — a paren operand** `add 1 (mul 2 3)`: the planner evaluates a group
*before* it commits a window (`evalParenGroupAt`, the trace in §1 shows
`^1 6` after step 0). The inner call completes in place inside the group
and its nops are swept away by the group's own collapse, so the outer
call never sees them:

```
^add 1 ( mul 2 3 )                 plan walk meets the group → evaluate it in place
 add 1 ( · · · ^⟨mul 2 3⟩ )        inner call complete                    (4 Sets)
 add 1 ( · · · 6 )                 inner call executed                    (1 Set)
^add 1 6                           stepCloseParen collapses the group: Splice(openIdx, 6, 6)
                                   — the nops leave with the markers; the plan resumes
```

then exactly as A. `add 1 mul 2 3` without the parens is a strict-barrier
error today and stays one.

**D — value-stack call** `1 2 add`:

```
1 2 ^add                           plan: stack sig[0] ← 2 (top), sig[1] ← 1
· · ^⟨add 2 1⟩                     the word's own cell holds the call      (3 Sets)
· · ^3                                                                     (1 Set)
```

Today: `execMatch` compacts and `Splice(0, 3, 3)`.

**E — nullary and zero-result** `now`, `1 drop`:

```
^now        →  ^⟨now⟩      →  ^1696…
1 ^drop     →  · ^⟨drop 1⟩  →  · ^·     (the call cell becomes a nop; the nop arm steps over it)
```

**F — a user fn** `fib 10`: the handler is `buildFnBodyHandler`'s, whose
results are the frame tokens `( 10 … body … __DC __pa undef/f n __RC )`.
They replace the one call cell by one `Splice(cell, 1, frame…)`; the
pointer lands on the frame's `(`, as it does today. Inside the frame, calls
leave nops; the frame's `)` collapses the region to its results and the
nops go with it.

**G — the statement-boundary commit** (`commitBarrierForward`): `if (gt 3
2) [7] def q 5 q` plans `if` with three slots, collects two, and `def`
begins its own dispatch, which commits the parked `if` with the two
arguments it holds (trace step 4 in today's engine: `[7] false │ ^if/s`):

```
if →if(2/3) false [7] ^def q 5 q
·  ·        ·     ^⟨if false [7]⟩ def q 5 q       (4 Sets; the pointer moves back to the call cell)
·  ·        ·     ^def q 5 q                      (zero results — the condition was false — so a nop)
```

The commit already re-matches with the exact claimed arity
(`MatchSignature` over the claimed values); it writes the call cell from
that match instead of removing the marker, rewriting the word to `/s`,
rearranging and re-stepping.

### 3.4 The layout invariant and the one helper that reads it

Under in-place arrival the tape around a parked forward is

```
[ … stack args … ] WORD MARKER a₀ ·* a₁ ·* … aₖ₋₁ ^
```

the collected arguments stay **after** the marker, in arrival order, with
only nop cells (from nested calls that already completed) between them;
the claimed value-stack arguments sit **below** the word, found as today by
`resolvedIndicesBeforeInto(tape, funcIdx, buf, StackArgs)`. Two facts make
the layout safe to read by scanning:

- every value stepped while a forward is pending is either collected by
  it or ends it (`CollectArrival`'s three non-collect verdicts dispatch,
  commit or implicit-end), so the non-nop, non-marker cells between the
  marker and the pointer *are* the collected arguments;
- the in-place writes only ever touch cells at or below the pointer, so
  **no nop exists ahead of the pointer** and the forward scans
  (`CollectForward`, `CollectCandidateScan`, `scanBoundaryToken`,
  `ReorderForwardCandidates`) never meet one. Verify mode (§5) asserts it.

One helper serves every reader of the layout:

```go
// claimedCells reports the cells a parked forward at fwdIdx has claimed:
// its word and marker, the StackArgs resolved cells beneath the word, and
// the first CollectedArgs non-nop non-marker cells after the marker up to
// limit (exclusive). The last entry of fwdArgs is the call cell on
// completion. A pending marker met in the window ends the walk.
func (e *Engine) claimedCells(fwdIdx, limit int, buf []int) (stackArgs, fwdArgs []int)
```

Its callers are exactly the readers that today compute positions from
`FuncIndex − k` on the assumption that the arrivals were moved before the
word: `EffectiveResolved` (the exclusion set), `curryOrStack` (its
exclusion set and curry list), `commitBarrierForward` (the claimed
region), the completion branch of `stepLiteral`, `IsFnShapeTypedBindingContext`
(def's typed-name map, "sitting at FuncIndex − CollectedArgs"), and —
analysis-only, so untouched in phase 1 — `noteWordLedArrival` /
`narrowerWindowFits` and the recorder's `OnPushLit` loop at completion.

### 3.5 Executing the call cell

`stepCall` is a new arm of the step loop beside `IsForward`:

```go
case IsCall(val):
    info, _ := AsCall(val)
    match := &MatchResult{Sig: info.Sig, Args: info.Args, Name: info.Name, Reg: info.Reg,
        Cell: e.Pointer, SpanLo: info.Lo}
    return e.execMatch(match)
```

`MatchResult` gains `Cell int` (−1 = legacy, the zero value must therefore
be set by a constructor or the field inverted to `InPlace bool`) and
`SpanLo int`. Inside `execMatch` the only changes are at the three places
that read positions:

- **result placement** — `placeResults(cell, results)` (§3.1's table)
  instead of `spliceMatchResults` when the match is in place;
  `ParkResult` and the fn-frame single-closure park still advance the
  pointer by `len(results)` / 1 afterwards;
- **the tail-call probe** — `probeTailCall` requires the call region to be
  contiguous (`sortedIndices[k] == pointer − n + k`). In place, the region
  is `[SpanLo, cell]` and is contiguous *by construction* (everything
  between is claimed or nop), so the probe takes the span instead of the
  index list; the frame-tail pattern it then matches (`)* __DC __pa
  (undef name)* [__RC] )`) lies ahead of the pointer and is unchanged.
  Full replacement splices `[FrameOpen, CloseIdx]` and takes the nops
  with it; shell elision deletes the marker run as before;
- **`FullStack` signatures** (`depth`, `pick`, `roll`, …) replace the
  whole scope from the nearest `(`; they decline in-place at dispatch and
  keep today's path.

Everything else in `execMatch` — argument post-processing, the policy
gates, `ClearPredMemo`, TCO eligibility, the handler call, position
stamping — runs unchanged, so the cell is "the dispatch, deferred by one
step", not a second dispatch path.

### 3.6 Where the call cell is written

Three sites complete a match today; phase 1 converts the two hot ones and
the barrier commit, and makes the cold ones correct by *normalising* first:

| site                                                            | today                                                                     | phase 1                                                                 |
|-----------------------------------------------------------------|---------------------------------------------------------------------------|-------------------------------------------------------------------------|
| `stepWord`, `fwdCount == 0` (value-stack call, nullary)         | `execMatch` immediately                                                   | **in place**: nops over the stack cells, the call in the word's cell     |
| `stepLiteral`, `CollectedArgs >= ExpectedArgs`                  | remove marker, `/s` rewrite, rearrange, re-step (second plan), splice     | **in place** from `claimedCells` with `fwd.Sig` (R1)                     |
| `commitBarrierForward`                                          | exact-arity re-match, then the same remove/rewrite/rearrange/re-step      | **in place** from its own re-match                                       |
| `stepEnd`, `implicitEnd`, `stepCloseParen`'s forward loop, `resolveOrphanedForwards` → `curryOrStack` | re-match with fewer args, else curry list or `/s` retry | `normalizeForwardLayout(fwdIdx)` moves the collected cells before the word (today's arrival moves, done late), then the legacy code runs unchanged; converted in phase 2 |
| value callee at `FuncIndex` (`fnDefAtPointer`, the `sealFnValue` seal) | arm the seal, rearrange, re-step the Function value                  | normalise, then legacy; phase 2                                           |

Arrival itself (`stepLiteral`'s collection branch) stops moving the value:
it counts the arrival (`Set` of the marker, as today's non-final arrival
already does), clears the value's `ReachGroup` tag in place, and advances
the pointer past the value. The `/q` Word→Atom conversion and the `/v`
marker consumption inside `CollectArrival` already write the window in
place.

### 3.7 Steps and the step budget

A call costs **one more step** than today (the cell's own step). Nops cost
none on the hot path: they are written at or below the pointer, and the
pointer moves back over a region only when that region is being replayed
(`stepMove`/`stepMoveCont` re-splice a fresh body), collapsed
(`stepCloseParen`) or committed (§3.3 G, where the cells between the call
and the barrier word are the call's own). The default budget is 10⁷
steps; `Debug.steps` is pinned by type only.

## 4 What a nop must be invisible to

Rule: a nop is **transparent** — never a value, never a barrier, never
stepped as a literal, never rendered as a result. The table is the audit
of every tape reader found in `core/go` (`grep -n "Tape.At\|Prefix(\|Snapshot()\|TakeAll()"`);
each needs exactly the treatment named.

| reader                                                                                      | treatment            | why                                                                                  |
|---------------------------------------------------------------------------------------------|----------------------|--------------------------------------------------------------------------------------|
| `Run`'s step loop, `stepCloseParen`'s re-evaluation loop, `resolveOrphanedForwards`' retry loop | `IsNop → Pointer++` arm | each has a `default: stepLiteral` that would push the marker as data                 |
| `stepLiteral` entry                                                                         | advance and return   | belt and braces for a nop reaching it                                                |
| `resolvedIndicesBeforeInto`, `resolvedStackBeforeFrom`, `EffectiveResolved`, `curryOrStack`'s resolved loop, `commitBarrierForward`'s claimed loop | skip, like `IsForward`/`IsMark`/`IsMove` | "not considered for argument matching"                        |
| `pendingForwardIdx`, `hasPendingForwardQuoteArg`, `stepEnd`/`commitBarrierForward`/`strandedForwardError` marker scans | unchanged | they look for `IsForward`/`IsOpenParen` only; a nop is neither                  |
| `Tape.forwards` counter                                                                     | unchanged            | `Set` already decrements on a Forward being overwritten                              |
| `stepCloseParen`: the ReturnCheck results loop and the final collapse                       | skip nops            | a frame's `( … )` results exclude them; the collapse splice then discards them       |
| `collectLoopRegion`, `stepMoveWhile`'s condition read, `handleLoopBreak`'s `Cont.Results`  | skip nops            | an iteration's results are its values only                                           |
| `sweepResidual`/`autoEvalStack`, the end-of-run drain (`TakeAll`), `exitWithFlowCtrl`'s `TakeAll` | `Tape.DropWhere(IsNop)` once, before `cleanMarks` | one linear compaction instead of a `Remove` per nop; the residual the caller sees has none |
| `ReorderCandidates` (the "received arguments" view of `sigError` via `runPrefix`)            | skip *before* the marker break | today's break on `TInternal` would truncate the diagnostic at a nop → error text would differ (R4) |
| `ReorderForwardCandidates`, `attemptedWindowOver`'s forward half                            | unchanged            | they scan ahead of the pointer, where no nop exists (§3.4)                           |
| `probeTailCall`                                                                             | span form (§3.5)     | contiguity by construction                                                           |
| `stepDefCleanup`'s `EvalResidual` scan, `isPendingResidualContainer`, `unwindLiveFrames`, `findCloseParenAfter`, `voidGroups` scan | unchanged | they test for specific kinds a nop is not                                 |
| `stepPastOpenParen` (`FrameOpenInfo.ArgSpan`)                                               | unchanged            | frame arguments are fresh copies spliced by the handler                              |
| `Debug.stack` (`EngineState.Stack`, registry.go), `RunTrace`                                 | render               | `TraceColorize` gets `IsNop`/`IsCall` arms; the backtrace builder filters nops       |
| `IsSteplessValue`/`IsSteplessWindow`                                                        | unchanged (false)    | a nop is not a value; islands receive VM windows, which hold none                    |
| analysis-only readers (`noteStatementStack`, `noteParenStack`, `noteCollectionHazardsBelow`, `narrowerWindowFits`, `noteWordLedArrival`, `bareCallContext`, `RegionRecorder`) | unchanged | the prototype is off under an analysis pass                           |

The kernel's forward scans (`collect_kernel.go`) get no nop arm: a nop
ahead of the pointer is an invariant violation, which verify mode reports
as an `internal_error` rather than papering over.

## 5 Gating

- `Registry.InPlaceCalls InPlaceMode` with `InPlaceOff` (zero), `InPlaceOn`,
  `InPlaceVerify`; `lang.Options.InPlaceCalls` sets it; the CLI spells it
  `--options inplace:on` / `inplace:verify` (`lang/go/options.go`'s strict
  schema); `BORU_INPLACE=1|verify` is read by `lang.New` when the option is
  unset, so benchmarks, the spec runner and `bench/interp/run.sh` can
  switch lanes without plumbing (the same precedent as
  `BORU_NO_STRICT_BARRIER`).
- `e.inPlaceOn()` is `Registry.InPlaceCalls != InPlaceOff &&
  !Registry.analysisActive() && e.recorder == nil`. Analysis passes step
  carriers through this same code and record positions
  (`RegionRecorder`, `noteCollectedLandings`, `forwardSplit`); the
  stack-form recorder emits `OnPushLit` from the rearranged layout. Both
  keep the legacy paths until the layout readers are converted (phase 2).
- Per-call declines (legacy path): `FullStack` signatures, macros
  (`execMacro` runs before planning anyway), a value callee at
  `FuncIndex`, and any site in §3.6's "normalise" rows.
- VM islands run interpreter sub-engines through the same code
  (`ReuseTape`, `isIsland`); the flag applies there too, and the drain's
  `DropWhere(IsNop)` is what keeps an island's residual nop-free.
- **Verify mode** executes in place and additionally (a) runs the legacy
  completion's re-plan over a scratch slice laid out as
  `rearrangeForForward` would lay it, comparing the chosen signature by
  identity with the plan's (R1), (b) asserts `claimedCells` finds exactly
  `CollectedArgs` cells, and (c) asserts no nop sits ahead of the pointer
  at a forward scan. A disagreement is counted in
  `Registry.InPlaceStats{Calls, Verified, Disagreed, Declined}` and that
  call falls back to the legacy completion, so a run always finishes; a
  test asserts `Disagreed == 0` over the corpus.

## 6 Soundness risks, and the check for each

- **R1 — the plan's signature versus the re-plan's.** Today a forward call
  is matched twice: `PlanMatch` chooses `fwd.Sig` from the window, and the
  `/s` re-step chooses again over the rearranged stack. `CollectArrival`
  verifies every arrival against `fwd.Sig`'s slot, so `fwd.Sig` always
  fits the values; the question is whether the second plan ever prefers a
  *different* overload that also fits (the `ForwardInfo.Sig` field is
  documented "for direct execution on completion", which is what in-place
  does). `commitBarrierForward` already insists on the exact claimed
  arity for the same reason. Check: verify mode's comparison over the spec
  corpus and the 62 real programs; a disagreement is a finding to record
  (a NUR, since it is an answer the two lanes could differ on), and the
  prototype can take option B — re-plan over a virtual sig-ordered window
  without touching the tape — if the semantics are ruled to be the
  re-plan's.
- **R2 — an index assumption the audit missed.** The §4 table and the
  `FuncIndex` reader list (§3.4) come from grepping `core/go`; a reader
  that computes "the value below the word" by arithmetic rather than by
  scan would silently read a nop. Check: the differential corpus
  interpreted both ways (§8), plus `TestTapeForwardCountInvariant`-style
  recounts in verify mode.
- **R3 — tail calls.** The probe's contiguity rule changes form (§3.5).
  Check: `Registry.TCO.{Detected, Elided, Replaced}` are equal both ways
  on `BenchmarkStage6`'s recursion rows and the TCO tests; a shape that
  nests under in-place where it replaced before is a defect.
- **R4 — errors and carets.** The call cell carries the word's position;
  `ReorderCandidates` skips nops. Check: every `ERROR:` row of the spec
  corpus compares the full rendered error, caret window included.
- **R5 — memory.** Nops accumulate in a flat top-level stream (at most one
  per claimed token, no growth: `Set` never grows the tape) and inside a
  region until its collapse or replay. A long statement sequence holds its
  nops to the drain. Check: `Tape.Len()` at end of run on the fixtures;
  if it matters, a periodic `DropWhere` at `stepEnd` when nothing is
  pending below (the statement-boundary stack is then values only).
- **R6 — observability.** `IO.trace` and `Debug.stack` show the new cells
  (allowed, Decision 7) — the renderer must not show a nop as a value.
  `Debug.steps`' count rises by one per call (shape-pinned only).
- **R7 — the one-shot seal** (`sealFnValue`/`sealFnValueIdx`) is armed by
  index at completion for a value callee. Phase 1 normalises before that
  site, so the index it stores is today's; phase 2 re-derives it from
  `claimedCells`.

## 7 Measurement

Instruments:

- `Tape.Stats{Sets, Inserts, Removes, Splices, Moved}` — always-on integer
  counters (`Moved` counts the Values `MoveGap` copies), read by a test
  that prints per-fixture totals for both lanes;
- `Registry.InPlaceStats` (§5);
- `BenchmarkStage6`'s `/interp` rows under `BORU_INPLACE=1`
  (`lang/go/bytecode_stage6_bench_test.go`) and a `boru-interp-inplace`
  column in `bench/interp/run.sh` (`BORU_NO_COMPILE=1 BORU_INPLACE=1`);
- `TestInterpAllocCeilings` run with the flag (a twin table, same
  ceilings).

Baseline (2026-10-05, `ed8a805ed`): `add 1 2 7 8 9` 7 structural edits /
10 copies; the loop profile in §1; interpreter 16–17× slower than the VM
on `fib`/loops (`bench/interp/README.md`); allocation ceilings as pinned in
`lang/go/interp_allocguard_test.go` (`for_tight` 7750 allocs / 2.95 MB per
200 iterations, `arith_chain64` 950 / 600 KB).

Targets for phase 1, each measured and written back into this note:

| measure                                               | target                                                             |
|-------------------------------------------------------|--------------------------------------------------------------------|
| tape edits for a fully-forward call of *n* arguments  | 1 `Insert` + (*n* + 2) `Set`s + the result `Set`; 0 `Remove`, 0 `Splice` for ≤ 1 result |
| cross-gap copies on the hot path                      | 0 after the first park in a statement                              |
| plans per forward call                                | 1 (was 2)                                                          |
| `for_tight`, `arith_chain64`, `compare_loop` interp ns/op | −25 % or better                                                |
| allocs/op, bytes/op per `TestInterpAllocCeilings` shape | ≤ today's (the call payload replaces the per-dispatch `MatchResult` + `Args` allocations and the rearrange work) |
| results and errors over the spec corpus, interpreted  | identical, both lanes                                              |
| `TCO.{Detected, Elided, Replaced}` on the recursion rows | equal                                                           |

## 8 Verification

- Unit tests in `core/go` for every new arm and helper, positive and
  negative paired (`AGENTS.md`): `claimedCells` over the four layouts (flat,
  mixed, interleaved nops, zero collected); `placeResults` for 0/1/k; the
  nop arm of each loop; each scan of §4 with a nop in the window and
  without; `normalizeForwardLayout` producing today's layout byte for byte
  (compare against a legacy-mode run of the same tokens).
- `TestInPlaceDifferential` (`lang/go`): the langspec smoke corpus and
  `BORU_SPEC_FILES`-selected families run interpreted with the flag off,
  on and verify; results and rendered errors compared; `Disagreed == 0`.
  A full-corpus run (`make test-spec` with `BORU_INPLACE=1`) before the
  flag's default is ever discussed.
- `TestRealProgramsCompile`'s 62 programs run interpreted both ways.
- Coverage: ADR-008's 100 % holds for the new arms (both modes are
  exercised by the suite; `//covergate:allow` only with a proof); the
  commit gate on every commit, `cover-gate.yml` dispatched before merge.
- Every divergence found is a **NUR record** before it is fixed
  (`NUR.md`'s rule; Pending does not block).

## 9 Phases

**P1 — the hot path (this note).** The two cells; in-place arrival; the
three in-place completion sites and the normalising cold sites (§3.6);
the §4 readers; gating; instruments; tests. Lands dark.

**P1b — no marker insert.** The one remaining structural edit per forward
call is `insertForward`'s `Insert` of the marker after the word. Folding
the marker into the word's own cell (a collecting-word cell carrying
`*ForwardInfo`, so arrivals mutate the count in place with no allocation,
where today each arrival boxes a new `ForwardInfo`) makes a call zero
structural edits and zero gap motion. It touches every `IsForward` /
`IsWord(At(funcIdx))` reader, so it is its own slice, measured
separately.

**P2 — the cold completions in place.** `curryOrStack`'s re-match with
fewer arguments, the curry list, value callees and the seal (§3.6's
normalise rows) written as call cells; the analysis-only layout readers
converted so the flag can also run under a check pass (which is what the
compiled lane's interpreter sub-engines would need).

**P3 — a call-site plan cache (the inline cache).** The profile's largest
item is the planner, and a loop body is re-spliced from the same tokens on
every iteration (`ForCont.Body`, a fn's `FnSig.Body()`), so a call site's
plan — keyed by the word token's shared `*SrcPos`, guarded by
`Defs.Gen(name)` (the dispatch cache's own counter, `registry.go`) for the
word and for every def-bound operand in its window, and by the planned
slot types for value-stack operands — can be reused: skip `CollectForward`
and `PlanMatch`, park the cached signature, and let arrival and in-place
completion run as in P1. A site whose window contains a group operand
needs the group's result type in the key (two overloads may differ on that
slot), and a polymorphic stack site needs a per-type entry — a
polymorphic inline cache. This is the step that turns the tree-walker's
hot bodies into a sequence of guarded compiled cells; it is exactly the
"F3 inline cache" of `design/legacy/INTERPRETER-PYTHON-PARITY.10.ignore`,
realised on the tape. It needs the maintainer's answer to question 1.

## 10 Files

| file                                   | change                                                                                                  |
|----------------------------------------|---------------------------------------------------------------------------------------------------------|
| `core/go/types.go`                     | `TNop`, `TCall` under `Word/__IN`                                                                        |
| `core/go/value.go`                     | `NopInfo`, `CallInfo`, `NewNop`/`nopCell`, `IsNop`, `NewCall`, `IsCall`, `AsCall`                        |
| `core/go/tape.go`                      | `Stats`, `DropWhere(func(Value) bool)`                                                                   |
| `core/go/signature.go`                 | `MatchResult.Cell`, `MatchResult.SpanLo`                                                                 |
| `core/go/engine.go`                    | `inPlaceOn`, step-loop arms, `stepCall`, `placeResults`, `claimedCells`, `normalizeForwardLayout`, the §3.6 sites, the §4 readers, `IsFnShapeTypedBindingContext` |
| `core/go/fn_frame_probe.go`            | `probeTailCall` span form                                                                                |
| `core/go/region_diag.go`               | `ReorderCandidates` skips nops                                                                           |
| `core/go/trace.go`                     | render nop and call                                                                                      |
| `core/go/registry.go`                  | `InPlaceCalls`, `InPlaceStats`; `EngineState` builders filter nops                                       |
| `lang/go/boru.go`, `lang/go/options.go`| `Options.InPlaceCalls`, `inplace:` option, `BORU_INPLACE`                                                |
| `lang/go/*_test.go`                    | `TestInPlaceDifferential`, the alloc-ceiling twin, `BenchmarkStage6` reading the env                     |
| `bench/interp/run.sh`, `README.md`     | the `boru-interp-inplace` column                                                                         |
| `design/FORWARD-COLLECTION-PHASES.10.md` | a dated amendment: phase 2's arrival no longer moves the value under the flag; keep the two phases' stop conditions in sync as before |
| `NUR.md`                               | one record per divergence found                                                                          |
| `kg/project/boru-project.jsonic`       | this note registered (done with this note)                                                               |

## 11 Open for the maintainer

1. **Lifetime (question 1).** Phase 3's plan cache is where the planner's
   54 % goes; it needs the ruling that a cached plan may be reused under
   the stated guards, and a decision on group-operand sites (key by result
   type, or decline).
2. **R1's rule.** Is the dispatch the *plan's* signature (`fwd.Sig`, what
   in-place executes directly) or the *re-plan's* (today's observable
   behaviour)? The prototype executes the plan's and measures the
   disagreement count; if any disagreement is found, the maintainer rules
   which lane is right and the record goes to `NUR.md`.
3. **Trace rendering.** `·` and `⟨name args⟩`, or hide nops in
   `IO.trace` so a trace reads as today's?
4. **The `/s` form.** With completion no longer re-stepping a `/s` word,
   the user-written `name/s` modifier keeps its meaning (stack-only
   dispatch at plan time) and the engine's internal `forceStackWord`
   rewrite disappears from the hot path. Is `ForceStack` as an *internal*
   mechanism something to retire once P2 lands?

## 12 The prototype as built (2026-10-06)

### 12.1 Where the build departs from §3–§10

| planned | built | why |
|---|---|---|
| `TNop`, `TCall` lattice nodes under `Word/__IN` (§3.2) | both cells are plain `Word/__IN` (`TInternal`) values told apart by payload: `NopInfo{}` (zero-size — writing one never allocates) and `*CallInfo` | every predicate that treats `TInternal` as a marker already classifies them; no lattice change |
| `CallInfo{Name, Sig, Reg, Args, Stack, Lo, argv[4]}` | `CallInfo{match MatchResult}`, embedded in a record sized to the call (`callInfo1`/`2`/`3` carry the operand storage; four or more allocate the slice): the cell carries the very `MatchResult` `execMatch` runs | one allocation per call, of the bytes the legacy lane spends on a match and its argument slice in two; a fixed three-slot record cost +15 % bytes on `fib` |
| `MatchResult.Cell` | `MatchResult.InPlace`, `MatchResult.SpanLo` | the zero value stays legacy |
| one `claimedCells` helper (§3.4) | `inPlaceOperands` (the non-nop cells after the marker), `claimedStack` (the resolved cells below the word), `excludeInPlaceClaims` (the readers' exclusion set) | each reader needs a different slice of the claim |
| nops persist until a region's drain, `Tape.DropWhere(IsNop)` (§4, R5) | **a call cell drops its own nops when it executes** (`dropCallSpan`: the span `[SpanLo, cell)` is all nops in the common case, and one `Splice` beside the gap removes it), so a nop lives one step. What a mark interrupts — a value-stack operand claimed from below a loop's mark — goes with the loop region the iteration's collection replaces, or `Tape.DropNopsIn` drops it at a group's collapse (`compactGroup`), the run's end (`cleanMarks`, `exitWithFlowCtrl`), or every 64 nops in a statement (`compactNops`: between the innermost `(` and the pointer, declined while a call cell stands at the pointer, a forward is pending there or the value-callee seal is armed) | measured (§12.6): every dispatch's backward scans (`EffectiveResolved`, the pending-forward probes, the commit probe) walk the nops below the pointer, so a periodic pass alone cost more than it saved — at a 64-nop period `arith_chain64` ran 22 % slower than the legacy lane, at an 8-nop period 12 % faster, and with the eager drop 20 % faster |
| the barrier commit compiled in place (§3.3 G, §3.6) | **normalised and left on the legacy path**, like the other cold sites | the verify lane found the commit is exactly where the plan and the re-plan differ by design (§12.3) |
| R1 by assumption; verify mode executing in place (§5, §6) | the completion executes the plan's signature only while (a) the word still names the binding it was planned against — `ForwardInfo.Gen`, read before the plan walk, against `Defs.Gen` — and (b) the signature still matches the arrived operands the way the re-plan tests them — `planHolds`: slot types, type literals, `/q` names, and every pattern judged as a *stack* operand, NUR235's pending-container evaluation included. Otherwise `completeLegacy` normalises the forward and replays the last arrival through the legacy completion. Verify mode runs the legacy lane and compares each re-plan with its plan | (b) is not optional: the plan skips structural map patterns and non-concrete patterns at forward slots (`patternsOk`'s forward leniency), and the re-plan enforces them |
| `Registry.InPlaceStats` (§5) | `core.InPlaceStats`, process-wide atomics | a module sub-registry runs on other goroutines |
| a `boru-interp-inplace` column in `bench/interp/run.sh` (§7) | `BenchmarkInterpFixtures` (`lang/go`) | the CLI has had no interpreter-only lane since 2026-09-19, so `run.sh`'s interpreter column times the compiled lane (noted in `bench/interp/README.md`) |
| zero results: `Set(cell, nop)`, the nop arm advances (§3.1) | the cell is removed (`Splice` of the one cell, which sits at the gap once its span is dropped); the pointer stands on the cell after it, as after the legacy splice | a `break` outside any loop reports at the cell the pointer stands on, and a nop has no position: the first build left the pointer on one, and the full lang suite run with the switch on caught it (`TestNUR355FlowOutsideLoop`, `TestNUR358ListLiteralIsNoLoop`, `TestNUR365ParenIsNoLoop`, `TestNUR355FileModuleFlowSource`); `TestInPlaceZeroResultPosition` pins it in core |

As planned: value callees and `FullStack` signatures keep the legacy path;
the switch is never honoured under an analysis pass or a stack-form
recorder; a module sub-registry inherits its parent's switch. §3.7's step
cost is finer than stated: a forward call costs the same steps as before
(the call cell's step replaces the `/s` re-step), a value-stack or nullary
call one more.

### 12.2 Verification

| what | size | result |
|---|---|---|
| core/spec `run` rows with the switch on and in verify mode (`TestCoreSpecInPlace`) | 425 rows | identical renderings |
| lang/spec, each row interpreted three times (`TestInPlaceDifferential`, `BORU_INPLACE_DIFF=1`) | 8,684 rows; 8,672 compared — 12 nondeterministic (time, randomness, minted ids) excluded when two legacy runs disagreed | 0 divergent; 3 rows differ only in a printed step trace (`module-debug.tsv:51,54,57`, decision 7) |
| the repository's real programs, interpreted from their own directories (`TestInPlaceRealPrograms`, same switch) | kg/tests and utils/tests: 20 programs, 1,213 boru:test cases | identical result, output, error stream, error and report |
| every lang/go and basic/go package with `BORU_INPLACE=1` | the whole suites | green |
| the langspec gate corpus, all nine shards, with `BORU_INPLACE=1` | every shard test | green and every gate at its regression ceiling, except one census the switch changes the subject of (below) |
| TCO counters (R3) | `s 200`, `s2 1000 0`, `fib 15`, mutual `ev 501` | `Detected`/`Elided`/`Replaced` equal on both lanes |
| core's own suite (`make cover-gate-core`) | 18,612 statements | 100 % |

The one langspec test the switch turns red is
`TestDispatchAdmissionAgreementCensus`, and only its staleness check: the
census probes every dispatch the run commits, and on the legacy lane each
forward call commits twice — at its plan and again at its `/s` re-step.
In place there is no re-step, so 360,499 probed dispatches become 190,193,
and the ledgered `kernel-no-match/def` window (`fn-triple.tsv:134`, `def f
fn T Any [3]`), which only the re-step ever presented, is no longer seen.
The answers are identical; the census is an instrument of the legacy
completion and would be re-ledgered if the switch ever became the default.

What the oracles found while building — none of it ever reached a lane.
The first full differential flagged 24 rows: the plan's forward-slot
pattern leniency (15 rows; fixed by `planHolds`), `execFnDefLiteral`'s
"alone inside a reach-lowered group" neighbour test reading a nop (6 rows
of `fn-value.tsv`; fixed by `prevNonNop`), and the three `module-debug.tsv`
rows that print a step trace, which differ by design (decision 7) once the
trace renders the new cells rather than their raw payloads
(`kernelFormatDefault`'s nop and call arms). Review found a word re-bound
during its own plan walk (the generation is now read before the walk), and
the full suites the zero-result pointer above. The measurements found the
first compaction policy's cost (§12.1, §12.6).

### 12.3 R1, measured

The verify lane over lang/spec compared 71,657 re-plans with their plans:
71,635 chose the plan's signature and 22 did not.

- **15 — the plan no longer held.** Structural map patterns and `tnot
  List` inputs the plan skips at forward slots (`corpus-core.tsv`'s
  create/load/remove/update, `edge-dispatch-3.tsv:63,65,68`,
  `fn-triple.tsv:72–74`, `record.tsv:155`). `planHolds` hands exactly these
  to the legacy completion — they are the 15 re-plan fallbacks of §12.4 —
  so the lanes agree.
- **7 — barrier commits where the plan held and the re-step chose a longer
  overload**, claiming a value-stack operand the plan had left alone:
  `5 if (1 eq 1) [99] def x 1` plans `if(Any, Any)` and re-plans
  `if(Any, Any, Any)` with `5` as the else (`forward-barrier.tsv:41,42,88`,
  `edge-forward-2.tsv:79`, `each-variants.tsv:86` ×3). The rows pin the
  re-plan's answer ("a stack value claimed as the else is consumed"), so a
  commit cannot execute its plan: it stays on the legacy path.

§11's question 2 therefore has a measured answer for the prototype: away
from a commit, the guarded plan *is* the dispatch; at a commit the
re-plan is, as the corpus pins it. Whether the commit should keep that
second choice is still the maintainer's question.

### 12.4 What the mechanism did over lang/spec

77,293 call cells executed: 71,556 forward completions and 5,737
value-stack or nullary calls. 74 forwards were normalised — 58 at a
barrier commit, 1 at an implicit end (`usurp.tsv:78`), 15 handed back by
`planHolds` — and none at a statement end, a paren close, the end of input
or a loop's collection: under the strict forward barrier the plan refuses
before such a forward can park, so core's own tests drive those sites
(`TestInPlaceNormalizeForwardsIn` and the rest). No rebinding fallback
fired in the corpus; core and lang tests construct one. 219,388 nops were
written and dropped, nearly all by the executing call that wrote them; the
statement compaction never had to run (`TestInPlaceLoopLeftoverNops` is the
shape that needs it).

### 12.5 Tape edits per call

`TestInPlaceTapeEdits` (core fixture; `Tape.Stats`, whole run):

| program | legacy: Insert / Remove / Splice, cells moved | in place |
|---|---|---|
| `addq 1 2` | 3 / 3 / 1, 12 | 1 / 0 / 1, 5 |
| `fourq 1 2 3 4` | 5 / 5 / 1, 24 | 1 / 0 / 1, 7 |
| `5 flexq 1` (one value-stack operand) | 2 / 2 / 1, 7 | 1 / 0 / 1, 5 |
| `addq ( addq 1 2 ) 3` | 6 / 8 / 2, 26 | 2 / 2 / 2, 12 |
| `1 2 sumq` (a value-stack call) | 0 / 0 / 1, 3 | 0 / 0 / 1, 4 |

The in-place Splice is the executing cell's drop of its own nops; the
nested row's Inserts and Removes are its paren markers'. §7's structural
targets hold for forward calls: one Insert, no Remove, no second plan. A
value-stack call gains nothing in tape work — the legacy lane already runs
it with one splice and no plan of its own at completion — and pays a step
and a few Sets for its cell; it is compiled in place only because the
rule (§3.1) is uniform.

### 12.6 Performance

Measured 2026-10-06 in this container (4-core Xeon, 2.1 GHz): three
interleaved rounds of each configuration — the base commit's test binary
(`3060f7553`), this build with the switch off, and with it on — two
samples a round, benchstat over the six (p ≤ 0.05 shown; `~` is no
significant change).

**`BenchmarkStage6`, interpreter rows** (`lang/go`), base → switch on:

| row | time/op | allocs/op | B/op |
|---|---:|---:|---:|
| `arith_chain64` | 593.9 µs → 454.6 µs, **−23.5 %** | 841 → 519, −38.3 % | −2.2 % |
| `compare_loop` | 2.127 ms → 1.767 ms, **−16.9 %** | 4,642 → 3,633, −21.7 % | −3.0 % |
| `if_scalar` | 3.964 ms → 3.307 ms, **−16.6 %** | 7,649 → 5,637, −26.3 % | −1.9 % |
| `if_listcond` | 4.046 ms → 3.482 ms, **−13.9 %** | 8,647 → 6,835, −21.0 % | −1.8 % |
| `for_tight` | 4.063 ms → 3.414 ms, **−16.0 %** | 6,876 → 5,267, −23.4 % | +1.8 % |
| `each_list` | 1.146 ms → 1.051 ms, ~ | 1,625 → 1,316, −19.0 % | +0.6 % |
| `fold_int` | 888.9 µs → 901.8 µs, ~ | 1,104 → 992, −10.1 % | +3.2 % |
| `map_get` | 5.851 ms → 4.867 ms, **−16.8 %** | 6,041 → 4,834, −20.0 % | +1.3 % |
| `string_join` | 2.734 ms → 2.551 ms, ~ | 16,350 → 16,140, −1.3 % | ~ |
| `recursion_nontail` | 10.30 ms → 8.52 ms, **−17.3 %** | 15,460 → 11,440, −26.0 % | +0.7 % |
| `recursion_tail` | 44.60 ms → 35.44 ms, **−20.5 %** | 73,850 → 54,830, −25.8 % | −0.2 % |
| `do_body` | 1.432 ms → 1.150 ms, **−19.7 %** | 2,140 → 1,831, −14.4 % | +0.9 % |
| **geomean** | **−14.8 %** | **−21.1 %** | −0.1 % |

**`BenchmarkInterpFixtures`** (`bench/interp/fixtures`, parse amortised,
no check pass), base → switch on:

| fixture | time/op | allocs/op | B/op |
|---|---:|---:|---:|
| `fib` (recursive `fib 24`) | 6.462 s → 5.301 s, **−18.0 %** | 10.36 M → 7.58 M, −26.8 % | +1.1 % |
| `loopsum` (`for` over 100,000) | 3.928 s → 3.501 s, **−10.9 %** | 3.50 M → 2.80 M, −20.0 % | ~ |
| `nestloop` (300 × 300) | 3.472 s → 2.969 s, **−14.5 %** | 2.72 M → 2.09 M, −23.2 % | +0.0 % |
| **geomean** | **−14.5 %** | **−23.4 %** | +0.4 % |

**Switch off** against the base: no row of either benchmark moves
significantly (geomean −1.2 % and −1.3 % time, allocations identical, bytes
+1.0 % and +1.5 % — the engine's new fields and the two `ForwardInfo`
fields every parked forward boxes).

**The real programs** (`TestInPlaceRealPrograms`, one run per lane, two
runs): 156.9 s → 140.7 s (−10.3 %) and 158.0 s → 146.3 s (−7.4 %) for the
twenty; kg's `codegraph_test.boru`, which builds the project graph
interpreted, 121.3 s → 106.4 s and 119.5 s → 109.8 s (−8 to −12 %). The
sub-second programs move within run-to-run noise either way.

**`TestInterpAllocCeilings`** with the switch on: every shape under its
ceiling, allocations down on all eight (e.g. `for_tight` 6,877 → 5,267,
`arith_chain64` 841 → 519).

**The nop policy** (§12.1), the same harness, three policies against the
legacy lane — time/op geomean:

| policy | Stage6 interp | fixtures | `arith_chain64` |
|---|---:|---:|---:|
| compact every 64 nops written | −4.2 % | −10.3 % | **+22.4 %** |
| compact every 8 | −8.3 % | −10.0 % | −11.7 % |
| **drop a call's own nops when it executes** (built) | **−11.1 %** | **−13.8 %** | **−20.4 %** |

(The final numbers above add the sized call records, which took `fib`'s
bytes from +14.6 % to +1.1 %.)

**Where the time goes now.** `loopsum`'s CPU profile with the switch on:
the planner (`PlanMatch` → `CollectCandidateScan` / `CollectForward`) is
45 % of the run — called once per call now, where the baseline profile
spent 55 % on it called twice — and 104-byte `Value` copies
(`runtime.duffcopy`) 26 % (29 % before). The in-place machinery itself —
arrival and completion, most of it the call record's allocation and
`planHolds`' re-check, and the call cell's span drop — is 4 % of `loopsum`
and 12 % of `fib`.

### 12.7 Reading

The mechanism does what §3 set out: a forward call is planned once, its
operands never move, its completion is one call cell and its cleanup one
`Splice` beside the gap; allocations fall by a fifth to a quarter and the
interpreter by about 15 % on the benchmark shapes, 7–12 % on the real
programs. It falls short of §7's −25 % target because phase 1 removes the
*second* plan, not the first: the planner is still the largest cost, and
§9's P3 (a call-site plan cache keyed by the word token, guarded by
`Defs.Gen`) is the step aimed at it. P1b (the marker folded into the word's
cell) would remove the last `Insert` per call; it is worth less than P3 now
that the profile is measured. A value-stack call gains nothing (§12.5) and
could keep the legacy immediate execution without loss.

The prototype stays off by default. Before it could be the default: the
maintainer's ruling on §11's question 2 for barrier commits (§12.3), the
admission census re-ledgered for a lane without re-steps (§12.2), and a
step-budget note — a value-stack call costs one more step than before
(§12.1).

### 12.8 Found on the way

- `design/LANGREF.10.md`'s "Partial application via `def ... end`" examples
  (`def add5 add 5 end`, `def greet add "hello " end`, and the composition
  that uses them) raise `signature_error` on both lanes: the strict forward
  barrier makes `add` begin its own dispatch while `def` waits. The section
  documents behaviour the language no longer has.
- `bench/interp/run.sh`'s `boru-interp` column times the compiled lane:
  `BORU_NO_COMPILE` is gone (CLI.md). The README now says so and points at
  `BenchmarkInterpFixtures`.
