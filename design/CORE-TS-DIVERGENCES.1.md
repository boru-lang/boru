# CORE-TS-DIVERGENCES.1 — 135 measured core-level divergences, and where they hid

**Status:** 137 MEASURED · 119 CLOSED · 18 PINNED (2026-08-08) · **Ledger:**
[core/spec/divergent.tsv](../core/spec/divergent.tsv) · **Programme:**
[GO-TS-PARITY.0.md](GO-TS-PARITY.0.md)

Sibling: [legacy/CORE-GO-TS-DEFECTS.0.ignore](legacy/CORE-GO-TS-DEFECTS.0.ignore), the 2026-08-06
read-only defect hunt. That one found 22 defects by READING the two cores
against each other. This one found 135 by RUNNING them, and the two sets
barely overlap — which is the first thing worth recording.

## Why none of these were visible

Three instruments were green throughout, and each is blind to this class for
a different reason:

| instrument | rows | why it missed them |
|---|---:|---|
| `crossdiff` (engine) | 1808 | hard-fails only when both engines SUCCEED with different values. Most rows below are error-vs-error or error-vs-success. |
| `parser-crossdiff` | 1765 | parser-level; never reaches the step loop. |
| `core/spec` | 158 | every row was written to a shape the corpus notation could already build, and the notation could not build these. |

The fourth instrument is the one that found them: **the coverage report**.
`core/ts` was at 88.2% and `core/go` at 100%, and the parser half of this
programme had already established the rule —

> **An uncovered branch in one port is where a divergence hides**, because
> nothing has ever compared the two engines there.

Eight agents read each uncovered region of `core/ts` against its Go twin and
proposed candidate expressions; a probe stage ran them through both engines.
139 candidates, 138 well-formed, **135 divergent**. That hit rate is the
finding: the uncovered surface was not merely untested, it was *wrong*.

## The ten classes

Ordered by severity — what a user would actually suffer.

### 1. An empty paren group in the forward window — 25 rows, **ALL CLOSED**

The only class that produces a different **value** rather than a different
error. Go treats `( )` as a no-value operand: `no_value_error`, or a stack
operand supplements it. `core/ts` silently drops the empty group and
**re-associates** the operands.

```
run 7 8 addq ( ) ( 5 )      go: 15 5           ts: 7 13
run 7 negq ( ) ( 5 )        go: -7 5           ts: 7 -5
run addq ( ) ( 5 ) 6        go: no_value_error ts: 11
```

Same source, different arithmetic. Everything else here is a taxonomy or
render difference; this one was a wrong number.

**Closed.** A void group cannot fill the slot it sits in, so it STOPS
forward collection: the word falls back to stack form where a stack operand
can supply the arg, and raises `no_value_error` where none can. `core/ts`
scanned PAST it, letting the next group slide into the empty slot.

The stop then exposed a second defect it had been hiding for as long as it
existed: `preEvalParens` derived a group's result count as
`length - (before - 1)`, arithmetic that only holds for a two-token `( )`
and goes NEGATIVE for every longer group. While the void branch merely
`continue`d that was invisible; the moment it began to `break`, 22 rows in
`core/ts` and 11 in `eng/ts` went red at once. `evalParenAt` now returns the
count instead of the caller re-deriving it.

### 2. The strict forward barrier — 51 rows, **30 CLOSED**

`REFERENCE.md:364` lists "another function word" as a forward-collection
barrier, and `design/STRICT-FORWARD-BARRIER.0.md` makes it uniform: a parked
forward that cannot commit with the args it already holds is **stranded** —
a `signature_error`, not a wait-through. `core/ts` has no such rule, so at an
`Any` slot it waits through the inner dispatch and the outer word **fires**.

```
run boomq negq 5            go: signature_error   ts: fixture_boom
run boomq nosuchword        go: undefined_word    ts: fixture_boom
run boomq ( negq 5 )        both: fixture_boom          <- the grouped form AGREES
```

That last line is the point: the rule exists precisely to make the grouped
and ungrouped spellings behave differently, and in `core/ts` they did not.

**Closed for the 30 rows where the parked word has claimed NOTHING** — which
is exactly where Go's `commitBarrierForward` returns false on its first test
("Nothing collected yet — no smaller-arity dispatch to commit"), so the port
matches Go where it fires. Go's commit half, which fires a parked word that
CAN dispatch with its claimed args before declaring the barrier, is not
ported: it needs the tape rearrangement Go performs and a word carrying a
shorter real overload than the parked plan assumed.

Widening it without that half is wrong, and MEASURABLY so — the first
attempt stranded `def h fn […]` at 1-of-2 args and turned 44 `eng/ts` rows
red. The 21 rows that remain here are a different root cause anyway: an
UNDEFINED word in the forward window, where Go raises `undefined_word` and
`core/ts` collects the bare Word as data at an `Any` slot.

**8 of those are now closed too, and the route matters.** The obvious rule —
REFUSE any surviving Word at a non-Word/Atom slot — closes 17 and breaks 7
`eng/ts` rows, because `def inc fn […]` needs the keyword to reach its slot.
The right lever is DEFERRAL, not refusal: any Word that survives resolution
now defers the dispatch, so the engine steps it. A registered one dispatches
and its result arrives (already the rule); an unregistered one raises
`undefined_word`, which is what Go does — its forward plan resolves an
unknown word to an Atom (`engine.go:8177`), but the plan is SPECULATIVE and
the token is still stepped.

The 13 that remain are container-evaluation ORDER (which of two failing map
values surfaces first), a different question. And `null` still has no arm in
`resolveForwardToken` where Go plans it as an Atom — a divergence the sweep
found that no ledger row currently reaches.

### 3. A type-mismatched paren operand — 14 rows, error CODE

Both refuse; they disagree about the code. Go: `signature_error` (the operand
does not fit the slot). `core/ts`: `no_value_error` (it treats the group as
having produced nothing usable). A caller matching on the code behaves
differently.

### 4. The WORD `end` versus the end MARKER — 11 rows, **9 CLOSED**

`;` is the marker and both engines handle it identically (pinned in
`dispatch.tsv`). The bare word `end` is a different thing: `core` has no word
by that name, so Go raised `undefined_word` while `core/ts` resolved the word
to the marker and silently applied barrier semantics — `run end 1` was the
value `1` here.

**Closed.** `core/ts`'s `isOpenParen` / `isCloseParen` / `isEnd` each carried
an extra "…or a Word named `(` / `)` / `end`" fallback that Go's predicates
never had — a leftover from the legacy fixture tokenizer, which produced bare
words where the parser produces markers. All three now test the vType alone.
The 9 rows moved to `dispatch.tsv`; the 2 that remain are class 2 in
disguise.

### 5. The builtin type-name table — 9 rows, **4 CLOSED**

**The crash is closed.** `Word` names a lattice branch, and its type literal
shares that branch's vType. `core/ts`'s `isWord()` tested the vType *alone*,
so `stepWord` resolved the name, wrote the literal back to the same slot, and
the step loop re-entered `stepWord` on it — ending in an uncoded
`AsWord: not a word value` that escaped the BoruError taxonomy entirely.
`isWord()` now also requires word DATA, which is exactly what separates a
word from its type.

**The path form and `Cidron` are closed too.** Go's
`ResolveBuiltinTypeName` tries the leaf table first and then the full PATH
(`core/go/resolve.go:23`); `core/ts` had only the leaf half, so
`Scalar/String` was `undefined_word` here. The path arm admits only paths
the builtin table declares, so `Scalar/Nope` still does not mint a type.
`Cidron` was simply absent from the TS table and is now declared.

**Still open:** `Module` (another table entry) and `Word/__ED`, which now
RESOLVES and still diverges — for a reason worth writing down, because the
canon is not at fault. `canon` renders the End type literal `__ED`
correctly; the STEP LOOP never lets it get there. `isEnd` tests the vType
alone, so the payload-less literal is taken for the marker and `stepEnd`
DROPS it.

That is exactly the conflation `isWord()` had, one arm over: a type literal
of a marker type is not the marker. And the obvious fix — requiring a
payload in `isEnd` too — takes **11 `eng/ts` rows** with it, because that
module builds a payload-less End somewhere. Measured, then reverted. One
ledger row is not worth a red module, and the row now carries the diagnosis
so the next attempt starts from the answer. A measurement worth keeping: registering every type by its
leaf name — which is literally what Go's `TypeTable.RegisterType` does —
is WRONG. It made `core/ts` resolve `__ED`, and `run __ED` is
`undefined_word` on both engines. So the lookup `stepWord` consults is a
filtered one, and the `!internal` guard in `indexDecl` belongs there.

### 6. A bare marker as a map value — 9 rows, **ALL CLOSED**

An END or paren marker where a map value goes. Go drops the key or refuses
the paren markers outright; `core/ts` stored the marker as **data** and
rendered it — `{ a: ; }` was `{a:end}` against Go's `{}`.

**Closed.** A bare marker is a PROGRAM, not data, and running it is what
drops the key: an End alone leaves no residual. `evalMapValue` enumerated
only paren-exprs and words as programs, so the marker fell through its
`return v` tail. Go reaches the same place through `AutoEvalMap`'s general
sub-engine tail, which has no such enumeration to fall out of.

### 7. Canon of a bare paren marker — 8 rows, **ADJUDICATED: Go is wrong**

Go canons an `OpenParen` as the **empty string**, so a list holding one
renders with a gap and the token vanishes; `core/ts` renders `(`.

**The `go` column is not the reference here, and Go's own source says so.**
`core/go/canon.go:104` fixed exactly this defect for the CLOSE paren —
*"Canon had no arm, so the payload-less fallthrough rendered it as the empty
string and `1 )` canon'd to `1 `, dropping the token silently"* — and the
same change fixed the End marker. The OPEN paren was left alone because, as
that comment says, *"an unmatched OPENING paren never reaches here, because
that is a parse error rather than a value"*. The corpus notation can build
one where the parser cannot, and the fallthrough is still there.

So these 8 rows stay pinned with `core/ts` **unchanged**. Making TS reproduce
the fallthrough would be applying a known defect to the port. This is the
third time on this programme the "reference by convention, not by proof"
warning has paid — after the typed-container tag and the `end` marker on the
parser side.

### 8. An unclosed paren in the value stream — 6 rows, **ALL CLOSED**

Go completed the pending dispatch and reported `signature_error`; `core/ts`
reported `syntax_error`. The core engine receives **values, not text**, so it
has no syntax to be wrong about — the question resolved itself once the
mechanism was found. An unmatched `(` is a scan BOUNDARY: Go's forward scan
stops at one it cannot resolve and lets the dispatch fail on its own terms.
`core/ts` called `evalParenAt`, whose `findMatchingClose` miss throws.

### 9. BigDecimal sign and scale — 2 rows, **BOTH CLOSED**

```
bigdec -0.0                 go: -0d0.0     ts: 0d0.0    (was)
bigdec 0e5                  go: 0d000000   ts: 0d0      (was)
```

A bigint has no `-0`, so the sign of `-0.0` was lost the moment the
significand was built. `Decimal` now carries an explicit `negZero` flag
beside the coefficient, exactly as `apd` carries `Negative`. And a zero
significand short-circuited its exponent away; it now grows its trailing-zero
run like any other value, because the scale is part of the identity rather
than noise to normalise. Seven rows in `canon.tsv` pin both edges.

### 10. Error ORDER inside containers — **5 CLOSED**

Which of two failing map values surfaces first, and whether a map argument
is evaluated at all.

**Closed for the runtime-map half.** The Eval gate applies where a map is
CONSUMED as an argument, not only at the end-of-run residual sweep.
`core/ts` gated the residual path (an earlier commit) and not the
consumption path, so a `{q …}` map passed to a word still had its values
evaluated — `boomq {q a: p( nosuchword ) }` raised `undefined_word` from
inside the map where Go hands the handler the map as given and lets it
raise its own error. Both arms of `autoEvalArgs` now carry the same gate
the list arm always had.

**And the eval-map ordering, 7 more rows.** Go evaluates a map ARGUMENT
before the handler runs, so a failure inside the map is reported instead of
being masked by the handler's own error. Two defects made `core/ts` fire
first: a bare WORD map value went through `resolveWordsDeep`, which leaves
an unresolvable name ALONE, where Go runs it in a sub-engine; and the
NESTED-map arm carried no `Eval` gate, so a runtime map inside an eval one
was evaluated anyway.

`autoEvalMapValues` and `evalMapValue` are two parallel implementations of
the same idea, and every defect in this class was in the one that had been
kept simpler — the `Eval` gate has now been added to it in three separate
commits, each time in a different arm. That is what a duplicated rule costs.
Merging them is the right follow-up and is not done here.

## What is closed, and what is deliberately not

**119 of the 135 are closed** — the whole of classes 1, 3, 6, 8 and 9, most
of class 5, 9 of the 11 in class 4, 38 of the 51 in class 2, and 12 of class
10.
A further 8 (class 7) are ADJUDICATED with `core/ts` unchanged, because Go
is the one that is wrong there. Each was a small, local defect with an
unambiguous Go twin to read against, and each moved its rows OUT of the
ledger into the spec file they belong in, which is the mechanism working as
designed.

**The remaining 16 are not fixed.** Classes 1, 2 and 5 are real feature work in
`core/ts` — the barrier is a whole rule with its own design note, the empty-
paren handling is a rewrite of the forward window's operand planning, and the
type-name table is a data gap plus a path resolver. Fixing them piecemeal
while the ledger was still growing would have meant re-measuring after every
step and losing the shape of the finding.

What IS done is the thing that makes them impossible to lose: every one is a
row in `core/spec/divergent.tsv`, both columns recorded, **each runner
asserting its own column**, and a row whose two columns become EQUAL fails.
So a fix cannot land silently, and a regression cannot either.

## An eleventh class, found the moment the corpus could reach it

`core/spec` had no way to install a def BINDING, so the engine's entire
def-substitution surface was unreachable from it — the reason those paths
sat uncovered was the NOTATION, not a missing capability. A leading
`def NAME <item> ;` clause fixes that, and the first two rows written
against it diverged:

```
run def b [ addq 1 2 ] ; b      go: [3]      ts: 3
run def b [ 1 2 ] ; b           go: [1 2]    ts: 1 2
```

Go SUBSTITUTES an unquoted eval-list binding as a value, which then
auto-evaluates. `core/ts` SPLICES it as a code body and runs it in place.
The quoted spelling agrees on both, so the disagreement is exactly about
what an UNQUOTED list binding means.

`core/ts/src/engine.test.ts` **baselines** the TS answer, in a test called
"splices an unquoted eval list as a code body" — the second time on this
branch a per-engine unit test has pinned one engine's behaviour as the
contract, after the map-evaluation one. Both were written before the
shared corpus existed, and neither was wrong to write; they were just never
checked against the other engine. That is the whole argument for the
corpus in one sentence.

## User-defined functions: reached, and agreeing

**Landed.** `fn( NAME PTYPE RTYPE [ body ] )` builds a boru-bodied
function value, bound through the def clause, and all eight rows AGREE on
both engines first time — arity and param-type enforcement, a computed
argument, the forward barrier applying to an fn word, and a fault in the
BODY surfacing from the body.

Two prerequisites had to be found, and neither was a missing capability:

1. The fn must be **installed** (`InstallFnDef`), never assembled by hand.
   Only installation builds each Signature's body-splicing Handler
   (`buildFnBodyHandler`), and the engine calls that handler
   unconditionally — a hand-built `FnDefInfo` panics into
   `internal_error`. Installation also resolves the `BarrierAllForward`
   sentinel and derives `MaxForwardArgs`, so it subsumes the three
   partial fixes that preceded it.
2. The fixture registry needs **`__pa` and `undef`**, which every fn
   body's frame tail emits (`AppendFrameTail`, `fn_frame.go:193`) and
   which **basic** registers, not core. The MECHANISMS are core's own
   (the args stack, the def table); only the words are basic's, so these
   are fixtures rather than a port.

Worth stating plainly: this closed a real parity gap — user-defined
function semantics had NO rows at all before, on either engine — and moved
`engine.ts` coverage by nothing. `core/ts` reaches fn dispatch by a
different route than `dispatchFnDef`, which is now the interesting
question and was invisible while nothing exercised functions.

## An earlier stall, kept for the route it rules out

The def clause proved the point it was built to test: `core/spec` could not
BIND a name, so the whole def-substitution surface was unreachable, and the
fix was notation rather than engine. The same question applies to the
largest uncovered block left — fn dispatch (`dispatchFnDef`,
`analyseFnBody`, ~120 lines of `engine.ts`) — and the answer is the same
shape: the corpus cannot build a FUNCTION VALUE.

An `fn( NAME PTYPE RTYPE [ body ] )` form was built and **reverted**, and
the measurements are worth keeping because they are most of the work:

- The TS half works. `def inc fn( n Integer Integer [ addq n 1 ] ) ; inc 5`
  dispatches to `6`, and the refusal rows (`inc 'x'`, bare `inc`) already
  agree with Go.
- The Go half does not. Three fixes took it from "matches nothing" to
  "matches and crashes":
  1. `BarrierPos: BarrierAllForward` is a SENTINEL resolved to `len(Args)`
     at REGISTRATION (`upsertFnDef`). A value pushed straight onto the def
     stack never registers, so it stays `-1` and matches nothing. Spell the
     position out.
  2. `NormalizeSig` must be called by hand, for the same reason.
  3. `FnDefInfo.Registry` must be set — a boru-bodied fn runs its body
     against the registry it was defined in.
- After all three, `Run` still failed with an `internal_error` wrapping a
  nil-pointer dereference. **Found: the crash is at
  `engine.go:3480`, calling `match.Sig.DispatchHandler()` — which is
  `nil`.** `Boru(body)` sets only `BoruImpl.Body`; the body-splicing
  `dispatch` Handler is built by `buildFnBodyHandler`, and only
  **`InstallFnDef`** calls it. So the fn must be INSTALLED, not assembled
  and pushed: `InstallFnDef` is what resolves the barrier sentinel,
  derives `MaxForwardArgs`, and builds the handler, which makes fixes 1–3
  above redundant rather than merely insufficient.
- **The real blocker, found on the third pass: `__pa`.** With the fn
  installed through `InstallFnDef`, `inc 5` reaches dispatch and then
  fails with `undefined word: __pa`. `AppendFrameTail`
  (`core/go/fn_frame.go:193`) emits that word into every boru fn body's
  frame tail, and it is registered in **`basic/go/native_definition.go`** —
  the BASIC layer, not core. A bare core registry cannot run a
  boru-bodied fn at all.

So the earlier framings were both wrong, in opposite directions. It is not
an unported capability: `dispatchFnDef` exists in both engines. It is not
purely a notation limit either: the corpus needs a `__pa` FIXTURE WORD
alongside the `fn(` form, and `__pa` pops the per-call args frame, so
writing it means porting a small piece of basic-layer semantics into both
runners and keeping them in step. That is a real, bounded piece of work
with a name — which is what two rounds of reverts bought.

It was reverted rather than shipped because the Go half CRASHING while the
TS half works would have written six rows recording my fixture bug as an
engine divergence — the same trap `quoteq` and `patq` set earlier on this
branch, and the reason the probe exists. Whoever picks this up starts from
"find the missing FnDefInfo setup", not from zero.

## The `go` column is not the reference by proof

It is the reference by convention. On the parser side, two of the five
original divergence classes turned out to have **Go** wrong. Class 7 here has
now been adjudicated the same way and Go lost — its own comment describes the
defect. Class 8 (a *syntax* error from an engine that never sees syntax)
still deserves the same treatment before anyone assumes which side moves.

## What this did to coverage

Pinning the 135 rows took `core/ts` from 88.20% to **90.71%**, and
`engine.ts` from 69.08% to **76.80%** — without a line of new engine code.
That is the rule restated from the other end: the uncovered surface and the
divergent surface were the same surface.

Closing 119 of them kept it there (90.31%): a row that moves from
`divergent.tsv` to a spec file still runs, so the coverage it bought does
not come back. The small dip from 90.73% is the barrier short-circuiting
paths those rows used to walk — the rows still pass, they just reach the
refusal sooner.

## What the closures cost, and the rule they taught

Two of the four fixes broke a DOWNSTREAM module, and both for the same
reason: `eng/ts` has hand-rolled fixtures that build values the parser never
builds — a map with no `Eval` flag, words named `(` and `)`. Tightening
`core/ts` to match Go exposed them.

- The map eval-gate (previous commit) broke 3 `eng/ts` rows. **Correct fix:**
  the fixture now builds an EVAL map, which is what the parser builds.
- Dropping the word-shaped `(`/`)` fallback broke 39. **Correct fix: none
  yet** — the fallback is restored for those two and kept dropped for `end`,
  which had no such user. The asymmetry is ugly and is recorded as such,
  because the alternative was either leaving a module red or reverting a
  real parity fix.

The generalisable part: **a fixture that constructs values by hand is a
second, unversioned parser**, and every divergence it hides is invisible
until the engine stops being lenient. `eng/ts`'s fixtures are the last
users of the legacy tokenizer, and they are why `core/ts` still accepts two
spellings of a paren marker.
