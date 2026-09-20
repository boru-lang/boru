# The paren re-step rule — what actually decides place-vs-apply

**Status:** MEASURED 2026-08-27 · supersedes NUR101's "place uniformly"
ruling of 2026-08-26, which was written against an unmeasured premise.

## 0. Why this document exists

NUR101 asked one question — *does a computed function applied inside an
enclosing group place, or dispatch?* — and was ruled **place uniformly** on
2026-08-26. The first implementation of that ruling deleted
`fnReturnPark`'s survivor-count clause (`closeIdx != idx+2`), on the stated
reasoning that "the survivor count was never the right question".

**It is the right question.** Deleting the clause turned
`(x:Integer => [x mul 2] 5)` from `10` into `fn (Integer) 5` and broke seven
suites with it. The clause was reinstated the same day. What follows is the
rule as MEASURED against `RunInterp`, the tree-walking oracle, rather than
as assumed.

## 1. The rule

> A Function value that a paren **placed** is re-stepped into a **call**
> exactly when it leads **two or more survivors** of an enclosing group that
> closes with a paren rewind.

Both halves are load-bearing, and each is one clause of `fnReturnPark`:

- **survivor count.** `stepCloseParen` sets `e.Pointer = openIdx + park`.
  With exactly one survivor the park returns 1 and the pointer steps PAST
  the Function — it is placed, never re-stepped. With more than one the park
  returns 0, the rewind lands ON the Function, and `stepLiteral` dispatches
  it. Placement is therefore the ONE-survivor case, and always was.
- **which groups rewind.** Only `stepCloseParen` performs the rewind. User
  parens, fn frames and `if` / `for` / `do` bodies close through it; the
  program top level, list literals and map literals do not.

## 2. The measurement

Against `RunInterp` (`lang.Run` is NOT the interpreter post-Stage-J — see
§4), with
`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]]`:

| program | enclosing rewind? | interpreted |
|---|---|---|
| `(mk 1) 2` | no — program residual | `fn (Integer) 2` — **places** |
| `((mk 1) 2)` | yes — the outer paren | `3` — **applies** |
| `[(mk 1) 2]` | no — a list literal | `[fn (Integer) 2]` — **places** |
| `[((mk 1) 2)]` | yes — the inner paren | `[3]` — **applies** |
| `if true [(mk 1) 2]` | yes — the arm's frame | `3` — **applies** |
| `for 2 [(mk 1) 2]` | yes — the body frame | `3 3` — **applies** |
| `do [(mk 1) 2]` | yes — the body frame | `3` — **applies** |
| `case 1 [1] [(mk 1) 2]` | no — the arm is a literal | `[fn (Integer) 2]` — **places** |

The rule predicts every row. NUR101's framing — "placement depends on
enclosing context" — was a correct OBSERVATION with the wrong subject: the
context does not modify a placement decision, it IS a second, separate
decision, taken one paren out.

### 2.1 The bare call — MEASURED 2026-09-05

The table above places a fn a PAREN produced. A fn a bare USER CALL returns
is parked by the same rewind logic, and the compiled lane had never asked:
its residual lowering applied every lead a paren had not placed, so the
first row below compiled to `8` on the default lane, exit 0, older than
every stage on the full-compilation branch. Against the interpreter, with
`def mk fn [[] [Function] [([y:Integer] => [y add 1])]]`:

| program | interpreted | compiled, before |
|---|---|---|
| `mk 7` | `fn (Integer) 7` — **places** | `8` |
| `mk 7 add 1` | `fn (Integer) 8` — **places** | `9` |
| `mk2 1 7` (mk2 takes an arg) | `fn (Integer) 7` — **places** | `8` |
| `m.p 5 7` over `{p: mk/v}` (the arrival apply) | `fn (Integer) 7` — **places** | `8` |
| `def f fn [[] [Any] [MathUtil.sqrt/v]]  f 16.0` | `fn sqrt(Number) 16.0` — **places** | `4.0` |
| `(mk 7)` | `8` — **applies** (the paren, two survivors) | `8` |
| `do [mk 7]` | `8` — **applies** (the body frame rewinds) | `8` |
| `do [mk] 7` | `8` — **applies** (a NATIVE word's returned fn auto-applies to what follows) | `8` |
| `def g fn [[] [Any] [mk 7]]  g` | `type_error … got 2 — [fn (Integer) 7]` (the return check runs BEFORE the frame would rewind) | `8` |
| `def g fn [[] [Any Any] [mk 7]]  g` | `8` — **applies** (the count passes, then the frame rewinds) | refused |

So the rule has a second clause the first section did not need: **a user
fn's single result is placed data where it lands**, and only a rewind (a
paren, a body frame) over two or more survivors, or a read that dispatches
(a bare name, a member read), turns it into a call. A NATIVE word's returned
fn is different — its delivery re-steps it — and a MULTI-output user call's
leading fn was rewound inside its own frame before returning, which the
check model does not perform, so the caller-side apply arm stands in for
that rewind.

The compiled twin is `callResultPlaced` (`compiler/go/emit.go`): a
single-output user call's result, or a user member's arrival-apply result,
that no enclosing paren re-stepped and no read delivered is laid out as
data by `resolveDynamicApply`, and inside a fn unit is not a "possible
unapplied call" for the frame replay, so the count mismatch raises the
interpreter's type_error. Pinned both ways in
`lang/go/returned_closure_park_test.go`; the `[Any Any]` row still refuses
because the rewind-after-count-check shape is not modelled, and that
refusal is an open defect owed a fix.

## 3. What the compiler was doing

Five silent miscompiles, in both directions, all one root cause: **the paren
structure is erased before the residual lowering runs**, so
`resolveDynamicApply` sees the same `[carrier, 2]` residual for `(mk 1) 2`
and for `((mk 1) 2)` and must guess.

| program | interpreted | compiled (before) |
|---|---|---|
| `(mk 1) 2` | `fn (Integer) 2` | `3` — applied what the interpreter placed |
| `(mk2 5) 10` | `fn (Integer) 10` | `11` — same |
| `[((mk 1) 2)]` | `[3]` | `[fn (Integer) 2]` — placed what the interpreter applied |
| `if true [(mk 1) 2]` | `3` | `fn (Integer) 2` — same |
| `if true [((mk 1) 2)]` | `3` | `fn (Integer) 2` — same |

The two directions are not two bugs. `((mk 1) 2)` at the top level compiled
CORRECTLY only by accident: the outer paren collapses, the pair reaches the
program residual, and the carrier arm applies it there — the right answer
by the wrong mechanism, which is why the same paren one level down inside a
list or an arm produced the opposite answer.

## 4. How five miscompiles survived a 100%-covered parity suite

Stage J flipped `lang.Run` from the tree-walking interpreter to the
COMPILED path, and a refused program is silently re-run on the interpreter.
That re-run is scaffolding absorbing a known defect, not a fallback the
design may lean on: the silence is what makes it dangerous. `RunInterp` is
the oracle; `Run` is not one any more.

**96 parity assertions across 5 files still read `…Run(src)`** and compare it
against `RunCompiled`. Every one of them compares the compiled lane against
itself and passes unconditionally. `TestFactoryApplyCompiles` is the clean
example: it asserts `(mk2 5) 10` is `[11]` on "both lanes", and the
interpreter has never answered `11` for that program.

This is the finding that matters most. A parity harness that silently stops
comparing lanes is worse than no harness: it converts a whole class of
divergence into a green check. Any future flip of a Run-like entry point
must be accompanied by a mechanical sweep of its oracle uses.

**And the first pass of that sweep was itself incomplete** — a review bot
caught it, which is the same lesson twice. The pattern used keyed on ONE
naming convention (`gotI`) and missed `errI`, `eI`, `gi`/`ei` and an
effect-parity closure: 21 assertions in the same files kept comparing the
compiled lane to itself. A convention-keyed sweep is not a sweep. The audit
that finds the last one is "every `.Run(` in a file that also calls
`RunCompiled`, read individually". Completing it surfaced two more
divergences (NUR108, NUR109) on top of NUR107.

## 5. What landed here

The interpreter is UNCHANGED — §1 is what it already did. Everything below is
the compiler learning it.

**The mechanism, and the reason it has to exist.** The paren structure is
erased before the residual lowering, so `resolveDynamicApply` sees the same
`[carrier, 2]` for `(mk 1) 2` and for `((mk 1) 2)` — one placed, one applied,
byte-identical at that point. Nothing downstream can recover the difference,
so the fact is recorded where it is still known: at the collapse.

`CheckState` now carries a matched pair, and together they ARE §1:

- `ParenPlacedFnIDs` — the carriers a paren PLACED (one survivor; the park
  returns 1 and the pointer steps past). Already existed for member reads;
  the park now records every carrier it places.
- `ParenReSteppedFnIDs` — the carriers an enclosing paren's rewind LANDED ON
  and will re-step into a call (more than one survivor; the park returns 0).
  New, written by `recordParenReStep` immediately after the park is read,
  from the same post-removal indices.

`leadPlacedNotRead` then reads "placed, not arrived through a read that
dispatches, **and not re-stepped one level out**". Three facts, all recorded,
none inferred.

The individual changes:

1. **`fnReturnPark`'s survivor clause is restored** and its header rewritten to
   say why the count is the question. No behaviour change from `main`.
2. **`recordParenReStep`** records the park's negative twin (above), under the
   park's own exclusions — a reach-lowered group's re-step is its dispatch, not
   a user paren's — and the same "might be callable" test the auto-dispatch
   guard uses, so both ends agree on what the rewind would have called.
3. **`resolveDynamicApply`'s carrier arm** applies a placed lead only when the
   re-step record says an enclosing paren claimed it. `((mk 1) 2)` and
   `((mk2 5) 10)` keep compiling natively; `(mk 1) 2` and `(mk2 5) 10` — which
   compiled to the WRONG answer — now refuse instead. That stops the wrong
   answer, but it is not the fix: both shapes are valid code that must
   compile, and each refusal is an open defect owed one.
4. **Its dynamic arm** was over-refusing a def-bound read (`def h (find …)  h
   {…}`): a bare name always calls, whatever mark its value carries from
   wherever it was built. It now shares `leadPlacedNotRead`'s conjunction
   instead of testing placement alone.
5. **`RecordBranch.resolveArm` refuses an arm whose residual LEADS with a
   re-stepped fn** (`residualLeadReStepped`) — narrower than
   `closureResidualHasUnappliedFn`, because an arm's SOLE carrier is
   legitimately placed data on both lanes (`if c [(mk 1)]`).
6. **`RecordMakeListInner` refuses a list whose LEAD element the re-step record
   claims.** This is where the record earns its keep: `[(mk 1) 2]` arrives with
   byte-identical elements and must keep compiling, so only the recorded fact
   can separate them.

Result on the 16-shape probe: **0 value divergences**, down from 5, with
`((mk 1) 2)`, `((mk2 5) 10)`, `[(mk 1) 2]`, `for`/`do` bodies and
`((inc/v) 7)` all still compiling natively. Divergences are not the only
count that matters, though: the shapes that now refuse are outstanding
work rather than a finished state.

The standing measurement is `lang/go/nur101_paren_restep_test.go`
(`TestParenReStepRule`): for every shape the rule classifies, the compiled
lane never answers differently from `RunInterp`. Where it refuses instead,
that is the test recording an unimplemented case — an open defect to be
tracked to closure, not a second outcome the contract allows.

## 6. Graduation

**Two of the four graduated 2026-08-27 with Stage 3's first increment, and
neither needed the Apply kernel.** The refusals were called "the same shape in
four positions". They are two different questions, and the answer to both
turned out to be a record the collapse already takes:

- **A LAYOUT question** — will anything re-step this value where it now
  sits? At a program residual, no, and the records already prove it.
  `placedNotReStepped` (`compiler/go/emit.go`) lets the residual refusal loop
  skip such a value and simply lay it out, where before it refused, reading
  the ABSENCE of a record as evidence of a hazard. `(inc/v) 7` and
  `(tbl get k) 5` compile natively on that alone, and
  `frontier-nur038-seal.tsv`'s explicit-paren row graduated into
  `lang/spec/fn-value.tsv` §6 with them. A CONCRETE fn value joined the
  record as its fourth placement shape for this reader; the two apply arms
  gate on `Carrier` and `Dynamic` respectively and never see one, so the
  widening was layout-only.
- **A RENDERING question** — the second loop refused any unconsumed fn-value
  carrier because "a VM closure renders unlike the interpreter's
  `FnDefInfo`". **Measured, that fear does not materialise** for a carrier a
  paren placed and nothing re-stepped: `(mk 1) 2`, `(mk2 5) 10`, `(mk 5) 10`
  and a bare `(mk2 5)` all render byte-identically on the two lanes. They are
  parity rows now, and `(mk2 5)` left `TestFactoryApplyCompiles`'s negative
  list — whose own parity assertion had never fired for it.

  What the blanket refusal was actually holding back is two OTHER hazards,
  found by relaxing it and watching what broke:

  1. **A bare-NAME read of a def-bound placed closure must DISPATCH**, not
     sit as data (ADR-011). `def mk … def f (mk 7)  f` compiled `[fn]`
     against the interpreter's `[7]` the moment the refusal came off — a
     live divergence the relaxation introduced and a `defReads` exclusion
     removes. Placement is a LAYOUT fact; a read that calls is a DISPATCH
     fact, and this loop needs both.
  2. **A captured closure baked as a CONST loses its closure state**
     (`TestEmitFnValueData`). The placed-and-unread carriers reaching this
     loop do not take that path.

  Both are excluded explicitly rather than assumed away. What remains
  blocked is §6.3's universal fn value, an open defect owed a fix, and
  the residue is now visible instead of hidden behind one refusal
  covering three things at once.

The two remaining arm/list shapes (`if true [(mk 1) 2]`, `[((mk 1) 2)]`) need
the apply RECORDED rather than skipped, which needs `RecordDynApply` to admit
an EVENT lead — `DynApplyLeadEligible` declines it today. When that lands,
`TestParenReStepListElementCompileFailure` graduates to a parity row and
`ParenReSteppedFnIDs` can retire in favour of the recorded event.

The ratchet for what already graduated is
`TestParenReStepPlacedLayoutCompiles`, pinned POSITIVE on purpose: the rule
test lets a refusal pass without failing, which is what makes it easy to
extend and useless for catching a regression back to one.

## 7. Four records this opened

- **NUR106** — the vacuous parity harness (§4). The sweep landed; the guard
  that stops it recurring has not.
- **NUR107** — a callee no overload of which matches raises `signature_error`
  interpreted and returns data compiled. Surfaced by the sweep, on the very
  test that had pinned that claim as non-reproducing. VM-side fix, its own
  increment.
- **NUR108** — the compiled lane's diagnostics point somewhere else: a
  `BIND_TYPED` validate failure carries no position at all, and a
  statically-failing typed-def trap blames the value where the interpreter
  blames the `def`. Same code, same message. **RESOLVED 2026-08-30** — the
  record carried no position because the typed-name map is synthesised by the
  `name:Type` desugar and is 0:0 at every typed-def site;
  `CheckState.CurWordPos` publishes the dispatching word's position (what
  `stampErrPos` gives the interpreter) and the def handler falls back to it.
  Both halves now agree, row and column, and the fence in
  `TestTypedDefBindCompiles` graduated into an equality. What is left is the
  caret WIDTH, recorded separately as NUR114.
- **NUR109** — `parse` over an unbound def-scoped parser name is `parse_error`
  compiled and `parse_unknown_lang` interpreted. The compiled answer is the
  specified one; the interpreter still falls back to the kind-name miss.

All three divergences are error-lane, and all three were behind NUR106. That
is the pattern worth carrying forward: the oracle hole was in the files most
specifically about the lane it hid.
