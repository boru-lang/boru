# Forward Collection Is Two Phases

> **Superseded in part by the strict forward barrier**
> (design/STRICT-FORWARD-BARRIER.0.md), now the default. A bare function
> word beginning its own dispatch is a barrier of ANY arity: the
> *wait-through / speculative arrival* described below — where a parked
> forward waits for a following fn word's RESULT — no longer happens;
> such a parked forward is STRANDED (`signature_error`) instead. What
> survives is the statement-boundary **commit** (`commitBarrierForward`):
> a parked word with a real overload consuming exactly the args it already
> holds still fires at the boundary. Read the sections below as the
> mechanism they describe, with the wait-through arrivals now replaced by
> a strand. The sole non-barrier is dot-access navigation (`m.a`).

Status: **documents shipped behaviour** (the statement-boundary commit
and template evaluation landed in `00cb7a7`; the speculation recording
— ForwardInfo.Speculative/SpeculativeAt, the trace marker, the
`speculative_forward_commit` advisory, and the `/u` barrier hook —
followed in the same June 2026 series). This note is the map of how
forward arguments are actually gathered — written because the
guard-pre-emption bug (bloom-filter report #1) survived as long as it
did on the assumption that there is ONE code path enforcing the
collection barriers. There are two, and they can diverge. Keep this
note current when touching either.

## The question

The argument-order rule (TUTORIAL §3) says forward collection stops at
a barrier: `end`, `)`, another function word, type mismatch. So how
could a parked `if` sit through a following `def` — letting the next
statement run before a guard's raise, and swallowing the next
statement's result as a phantom else? Surely one code path checks the
barrier?

No. Forward-argument gathering is split across two phases that live in
different functions, walk different representations, and — before the
fix — enforced different stop conditions. The rule as documented
described only the first phase.

## Why two phases are forced

A forward argument can be an **expression**: in `mul 2 (add 3 4)` the
value `7` does not exist when `mul` dispatches. The engine is a single
linear pass over the token tape (the stack IS the tape; there is no
AST to pre-bind call shapes), so the only option is to *park* the word
and collect the value when it materialises. Parking splits gathering
into:

1. a **plan-time token walk** over unstepped source tokens, and
2. a **run-time arrival loop** over values reaching the pointer.

Both are load-bearing. Neither can absorb the other: the walk cannot
know an expression's value, and the arrival loop cannot see source
structure that has already been stepped away.

## Phase 1 — the plan-time walk

`stepWord` → `resolveForwardArgs` (engine.go:902, the structure-first
lazy scan of `design/legacy/LAZY-ARG-RESOLUTION.10.ignore`) → `matchSignature`.
The walk looks at the tokens after the word, prunes non-viable
overloads on concrete literals, pre-evaluates paren groups (and, since
`00cb7a7`, interpolated template strings) only at positions some
still-viable overload consumes, and picks a signature.

Phase-1 stop/skip conditions:

- `end`, `)`, engine markers — hard stop.
- A paren group no viable overload consumes — left raw, hard stop.
- A concrete literal that rules out every overload — type prune.
- A word def-bound to a **data** `__SP` splice marker — rewritten in
  place to `ParenExpr([w])` and reprocessed through the paren branches
  (the `f w ≡ f (w)` equivalence; see "Splice-bound words" below).
- **Any other word — counted as one optimistically-filled position and
  the walk continues** (`staticForwardType`, engine.go:1114, classifies
  it `fwdBoundary` but the scan does `pos++`).

That last line is deliberate and load-bearing: `def x (expr)` needs
the planner to look *past* the name `x` — a word — to find and
pre-evaluate the body paren at position 1. The cost: for
`if (cond) [then] def …` the planner counts `def` as a plausible
filler for the else slot, keeps the 3-arg overload viable, and parks
`Forward{ExpectedArgs: 3}` (`insertForward`, engine.go:2103).

When every selected position is resolvable *now*, there is no parking
at all — the word dispatches inside phase 1. This is why
`range 2 6 def x 99 x` was always correct (two literal args, 2-arg
overload complete at the barrier) while the same shape with a paren
condition was not: **same surface syntax, different phase, different
rules.**

## Phase 2 — the arrival loop

Once parked, the word no longer sees tokens. The collection block in
`stepLiteral` routes every VALUE that reaches the pointer into the
next slot (`CollectedArgs++`); on `CollectedArgs == ExpectedArgs` the
completion path (engine.go:2284) drops the marker, force-stacks the
word, and runs `rearrangeForForward` so the first-collected argument
lands on top — sig order for the matcher's top-first read.

Phase-2 stop conditions, before the fix:

- an arriving value that mismatches the next slot's type —
  `implicitEnd` (engine.go:3423);
- an explicit `end` token — `stepEnd` (engine.go:3554);
- end of program — the leftover-forward drain.

And that is the whole list. A function word never *arrives* — it goes
through `stepWord` and dispatches itself — and `stepWord` had no idea
a Forward was pending below it. Nor could type mismatch help when the
waiting slot is `Any` (an `if` else): everything matches `Any`. So
for

```boru
def f fn [[n:Integer] [Integer] [
  if (n eq 0) [raise "zero"]
  def q (10 div n)
  q
]]
```

the parked `if` sat at 2/3 while `def` ran first — the division by
zero pre-empted the guard's raise — and in
`if (n eq 0) [99] add 1 2` the `3` arrived as a perfectly legal else
and was discarded. The failure also hid well: the visible symptom was
the *next statement's* error, which reads as a bug in user code, not
an engine ordering problem.

The same lens explains the sibling template bug fixed in the same
commit: an InterpString is an expression, but phase 1 classified the
raw token by its internal type, so every typed overload was pruned and
`` raise `bad: ${x}` `` mis-dispatched. Expressions must be evaluated
by the phase that meets them, not type-tested as tokens.

## The fix — statement-boundary commit

No third path. `stepWord` is the one chokepoint every dispatching
function word already passes through, so it now asks first: *is there
a parked forward in this paren scope that could already fire?*
(`commitBarrierForward`, engine.go:3474):

1. Nearest pending Forward, scanning down to an open-paren boundary —
   the same scan `stepEnd` performs.
2. **Satisfiability probe** over the forward's CLAIMED args only
   (collected + claimed stack), via `MatchSignature` on the
   collection-order slice. Deliberately narrower than the whole-scope
   match: an implicit commit must not reach below what the word
   claimed.
3. On a match, commit with the **completion path's** mechanics: drop
   the marker, force-stack the word, `rearrangeForForward`, re-step.

Step 3 explicitly does NOT reuse `curryOrStack` (engine.go:4242): its
rearrange is gated on `StackArgs > 0`, so a purely-arrival forward
(`if` with a paren condition: 2 collected, 0 stack) would re-dispatch
with its collected args in reverse — cond and then swapped. The
completion path rearranges unconditionally; the commit mirrors it.
(That gate remains a sharp edge inside `curryOrStack` for any future
caller that reaches it with `StackArgs == 0` and 2+ collected args —
the explicit-`end` paths do not today, because an `end` visible to the
phase-1 walk demotes the arity before parking.)

The satisfiability gate is what preserves deferred-argument flows:

| Form | Behaviour |
|---|---|
| `if (c) [t] def q (…)` | guard fires FIRST, then `def` runs |
| `if (c) [t] add 1 2` | guard fires; `3` stays on the stack (no phantom else) |
| `if (c) [t] (e)` | unchanged — a paren is not a word; its result arrives as the else |
| `if (c) [t] [e]` / `… 42` | unchanged — literals arrive as the else |
| `add 1 def x 5 x` | unchanged — `add` can't fire on one arg, so it keeps waiting |
| `range 2 6 def …` | unchanged — completed in phase 1, never parked |
| explicit `end` / `;` | unchanged — `stepEnd` as before |

One deliberate change beyond the bug: chaining an else-if by letting a
following bare `if`'s *result* arrive as the outer else
(`if c [a] if c2 [b] [e]`) now commits the outer `if` first. The form
appears nowhere in the spec corpus or docs — the documented else-if is
the single-list clause form `if [c1 b1 c2 b2 … else]` — and the old
behaviour ran the inner `if` eagerly even when `c` was true, which is
the same pre-emption disease this fix exists to cure.

## Invariants (keep these true)

- Phase 1 owns *which overload and which token positions*; phase 2
  owns *when values fill them*. Any stop condition added to one phase
  must be reconciled with the other — this divergence is exactly how
  the guard bug happened.
- A token kind that denotes an expression (paren group, template,
  reach) must be EVALUATED by the phase that encounters it, never
  type-tested raw against slots.
- An implicit commit (barrier or mismatch) must re-dispatch through
  the completion layout: `rearrangeForForward` before the top-first
  re-match. First-collected = sig[0].
- The commit probe matches claimed args only; widening it would let a
  statement boundary consume stack values the word never claimed.

Verification: `lang/spec/control.tsv` §6 pins the guard ordering, the
false-guard path, the kept statement result, and the paren-else
non-barrier; `lang/spec/error.tsv` pins the template forms. The
behaviour ships with the full eng + lang suites green.

## Speculation is recorded (the follow-up unification)

The "record the plan-time stop conditions on ForwardInfo" follow-up
was investigated and implemented; this section replaces the original
Residual with what was actually found.

**The single divergence point.** The plan accepts a word as an
operand in matchSignature's word branch when the name has a Defs
binding and `sigArgMatches(…) || expectedType.Equal(TAny)` holds.
Registered natives and def'd fns ARE Defs entries (Registry.Lookup
reads `r.Defs.Stack`), so an Any-typed slot — an `if`'s else — plans
any defined function word as an operand, and the nominal "function
word — boundary" check a few branches later is unreachable for Any
slots. Plan says *operand*; runtime says *operator*. Everything else
(`/q` name capture, FormArgs, TFunction-typed slots, value defs) is
consistent by construction.

**ForwardInfo now records it.** When the plan fills a forward slot
with a word bound to a dispatching definition (an FnDefInfo binding,
at a non-TFunction slot), matchSignature reports the first such slot
and insertForward stores it as `Speculative bool` +
`SpeculativeAt int` (the bool guards the int, so the struct's zero
value means "none" — No-Zero-Overload rule). Consumers:

- **Trace** renders parked speculative forwards as `→if(2/3 spec@2)`,
  making the divergence visible in step traces.
- **Check mode** emits the non-gating `speculative_forward_commit`
  info advisory when a speculative forward actually commits at a
  statement boundary — exactly the else-less-guard shape, where an
  explicit `[]` else or `end` states the intent. It is structurally
  silent on the definition idiom (below).
- The index is plan-side bookkeeping only: under zero/multi-value
  paren collapse the arrival count can drift from plan positions, so
  nothing may use it for slot arithmetic, and the
  commitBarrierForward scan stays UNCONDITIONAL — the barrier commit
  doubles as zero-value-collapse recovery (Trap 1) where the plan had
  no speculative word at all.

**The commit needs a real overload consuming EXACTLY the claimed
args.** boru-bodied fns carry a synthetic 0-arg Fallback in their
aggregate dispatch table (it exists to raise a clean "no matching
signature" error); the first probe implementation matched it, which
committed a waiting call to its own failure — `g 1 def x 5 x` errored
at the boundary instead of letting def's result feed g's second slot.
The probe now requires a non-Fallback signature whose arity equals
the claimed args: an exact smaller overload commits (`1 5`, mirroring
plan-time typed-overload behaviour), anything else keeps waiting.
The full combination matrix lives in `lang/spec/forward-barrier.tsv`
(boundary kinds × condition shapes × polarity, arrival follows,
silencers, the exact-arity rule, known swallow shapes, chained
guards, end-of-program drain).

**Why the arrival loop has nothing further to honour.** Arrivals
cannot jump past the speculative word (there are no tokens between
the last planned value and it), so the only meaningful runtime event
for a speculative slot is the word itself reaching a dispatch point —
and every dispatch point passes the commit probe: bare words and
def'd fns through stepWord's hook; dot-access through the lowered
`get` word; and `/u` through its own hook (below). `/q`, FormArgs,
and TFunction-slot words arrive as data and are excluded from the
speculation predicate by construction.

**Speculative fill is a FEATURE, not only a hazard.** `def name fn
[…]` — the language's core definition idiom — IS the speculative
pattern working: `fn` is planned as def's Any-slot operand, def parks
at 2 expected, the barrier probe FAILS (def has no 1-arg overload),
fn dispatches, and its Function result arrives to complete def. This
is the proof that the deeper unification — refusing dispatching words
as Any-slot operands at selection time — is wrong: it would break
every `def name fn […]` in the corpus, and the carve-out ("boundary
only when a smaller arity is satisfiable") needs two-pass cross-sig
matching plus exposes the stack phase to claiming values the word
never owned. Rejected.

**Corrections to earlier claims.**

- An earlier draft named execFnDefLiteral (Function values
  auto-dispatching) as a barrier-hook gap. It is NOT one: its only
  call site sits inside stepLiteral's no-pending-forward branch, so
  it is structurally unreachable while a forward pends.
- The REAL gap was the `/u` (ForceUsurp) path, which dispatched via
  `stepLiteral()` directly: `if (c) [t] sub/u 10 3` swallowed the
  usurped wrapper into the else slot as data. stepWordUsurp now
  commits the pending forward before dispatching (`/ur` is exempt —
  it deliberately produces inert data, a legitimate arrival).
  Pinned in `lang/spec/usurp.tsv`.

**Known edges, deliberately left.**

- `/r` pushes the referenced Function and steps past it
  (`pointer++`): in an argument position it is an arrival; a pending
  forward whose slot it doesn't fit simply stays pending. Pre-existing
  behaviour, unchanged.
- A parked forward whose word token was replaced by a non-word can
  never barrier-commit (commitBarrierForward requires `IsWord` at
  FuncIndex) — defensive guard, not a reachable shape today.

## Splice-bound words expand in both phases (`f w ≡ f (w)`)

A word def-bound to a **data** `__SP` splice marker (`def vs word
[2,3]` — payload holds only values) occupies a forward-argument
position exactly as the written paren group `(w)` would: `add2 vs` ≡
`add2 (vs)` → both args from the spread. The rule is enforced by the
SAME transform at both phase sites — the token/value is rewritten in
place to `ParenExpr([w])` and the existing paren machinery (evaluation
gating, zero/multi-value collapse, raw-capture collection) takes over:

- Phase 1: `resolveForwardArgs`' walk, before the `staticForwardType`
  fall-through.
- Phase 2: `stepWord`'s Defs-substitution branch, gated on
  `pendingForwardIdx() >= 0`. Inside the wrapped group the OpenParen
  barrier hides the pending forward, so the re-stepped word takes the
  standalone splice-fire path — no recursion.

Because both sites delegate to one mechanism (the paren path), this
rule cannot drift between phases the way the original stop conditions
did. Exemptions, all deliberate: structural-capture slots
(`capturesForward` — /q takes the word's NAME, form/raw/type slots the
raw token); **code**-bearing splices (`spliceIsData` false — `def
inc word [1 add]` is a Forth-style macro whose tokens must run against
the LIVE stack; paren isolation would change its meaning, so it stays
a boundary word and `f (p)` is the explicit opt-in); and **binders**
(`bindsReferent` — `def y xs` collects the raw marker so the new name
ALIASES the splice; expansion there would lose the referent, and code
splices already alias at def by skipping expansion, so data and code
splices behave identically at binders. `def y (xs)` forces expansion —
the one place `f w` and `f (w)` differ, by design). Pinned in
`lang/spec/word-splice.tsv` §7.

Related: `design/legacy/LAZY-ARG-RESOLUTION.10.ignore` (phase 1's structure-first
scan), `design/FORWARD-COLLECTION-TRAPS.0.md` (zero-value arrivals and
bare-word keys — other costs of the same architecture),
`design/FORWARD-STRAND-ADVISORY.10.md` (the check-mode advisory for
mixed-form stranding).
