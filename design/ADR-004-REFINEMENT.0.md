# ADR-004 refinement — argument-handling categories

**Status: PROMOTED.** Written under NUR023's 2026-08-14 verdict as the
material for a refined ADR-004, and promoted on explicit maintainer
instruction 2026-08-15: ADR-004 now carries an amendment replacing "the
sole exception is the Forth stack vocabulary" with the four categories,
and cites this note. The ADR is the rule; this remains the reasoning,
the measurements and the rejected alternatives behind it. The three
questions in §5 stay open — promotion did not decide them.

**Why it exists.** ADR-004 as accepted (2026-06-09) is six sentences:
every word ships forward-collecting; the sole exception is the Forth
stack vocabulary. NUR023 found two argument-taking words outside that
"sole exception" (`apply`'s `[Function]` overload, `__casematch`) and a
0-arg guidance split between two documents. Both were closable as
documentation debt and have been closed — the two words are pinned in
REFERENCE.md's closed list with their rationales, and the 0-arg passages
are reconciled. What could not be closed that way is the underlying
gap: **ADR-004 describes a default but not a system.** It names one
category ("forward") and one exception ("the stack vocabulary"), while
the implementation has four categories, a sentinel, a per-call-site
override pair, and an orthogonal quoting axis. A rule that cannot
express `apply` is a rule that will keep generating NUR records.

The canonical, code-adjacent statements of the mechanism remain
`core/go/CLAUDE.md` §"Signature Ordering" and `lang/go/CLAUDE.md`
§"Argument Ordering"; this note does not replace them. It supplies what
an ADR needs and they do not: the **categories**, the **rationale**, and
the **rule for choosing**.

---

## 1. Barrier positions

Every `Signature` carries one integer, `BarrierPos`, which is the
position of the `|` marker in the signature. It defines a single split:

> Positions `[0 .. BarrierPos-1]` are **forward-eligible** — filled from
> forward tokens in source order, falling back to the stack. Positions
> `[BarrierPos .. N-1]` are **stack-only**. Stack consumption is always
> top-down: `sig[i]` reads the top, `sig[i+1]` the next-deeper.

| Value | Meaning |
|---|---|
| `BarrierAllForward` (`-1`) | **Unset sentinel.** "No `|` was specified." Resolved once at registration. |
| `0` | All positions stack-only. `[\| a b c]`. |
| `N` (= `TotalArgs()`) | All positions forward-eligible. `[a b c \|]`. |
| `0 < B < N` | Mixed: forward fills the leading `B`, the stack fills the rest. |

**The sentinel is resolved at REGISTRATION, before any consumer reads
it** — `-1` becomes `TotalArgs()`, so every downstream read of
`BarrierPos` sees an explicit value. That is what keeps the "no
zero-value overload" rule (`core/go/CLAUDE.md`) intact: `0`
unambiguously means stack-only, because "unspecified" was spelled `-1`
and is already gone by the time anything matches against it.

It is **not**, however, resolved in one place. There are five
resolution sites, and a refined ADR should either say so or the code
should centralise them:

| Site | Resolves for |
|---|---|
| `Registry.upsertFnDef` (`core/go/registry.go`) | ordinary word registration — the main path |
| `compileFnSigs` (`core/go/core_helpers.go`) | compiled fn signatures |
| `compileFnDef` (`core/go/engine.go`) | anonymous / constructed fn values |
| `NewWordExtension` (`core/go/word_extend.go`) | an open-words extension's sigs |
| `TransplantExtension` (`core/go/word_extend.go`) | an extension cloned onto another registry |

The duplication is benign today — every site applies the identical
`BarrierPos == BarrierAllForward → TotalArgs()` rule — but it is the
shape that lets the rule drift: a sixth construction path that forgets
it would hand a consumer a raw `-1`, which reads as neither "all
forward" nor "all stack". **Centralising these into one normalizer is a
concrete follow-up this note recommends**, and doing it first would let
the refined ADR state the single-boundary invariant as fact rather than
as intent. (This note originally claimed the single boundary outright;
that claim was wrong, and the 2026-08-14 review caught it.)

There is **no per-word "stack default" mode.** A stack-only signature
must set `BarrierPos: 0` explicitly at its registration site. The
`ForwardArgs` flag and the `RegisterStackOnly` method that once provided
one were retired in the BarrierPos cleanup. (NUR023's own text describes
the sentinel as resolving "to `len(Args)` or `0` based on the
`forwardArgs` flag" — that is **stale**; the flag no longer exists and
`-1` always resolves to `TotalArgs()`. Corrected here.)

### The 0-arg case

At zero arguments `TotalArgs()` is `0`, so `-1` and `0` normalize to the
**byte-identical** stored signature. Both spellings are therefore
correct and neither is a default the other violates. `design/go-modules/
README.10.md` and `RUNTIME.10.md` now say so; a refined ADR should say
it once rather than leave two documents to be compared. The choice is
idiom: `0` reads as "there is nothing to collect", `-1` as "this word is
ordinary". Prefer `0` for constants and clock words, matching the
existing majority.

---

## 2. The categories

A word occupies exactly one of four categories, and the category is a
property of each **signature**, not of the word — which is why a word
can be mixed overall.

### 2.1 Forward-eligible (the default)

`BarrierPos == TotalArgs()`. The canonical call form is
`word arg1 arg2 …`, and written order matches declared order. New words
ship here. This is ADR-004's cultural default and it is unchanged by
this refinement.

### 2.2 Mixed-barrier

`0 < BarrierPos < TotalArgs()`. Forward fills the leading positions and
the stack supplies the rest. This category is not an exception to the
default — it is the default *parameterised* — and it is far more common
than ADR-004's silence suggests. Measured over a default registry
(174 words carrying an `FnDefInfo`, 493 argument-taking signatures):

| Per-SIGNATURE barrier | Count |
|---|---|
| all-forward (`BarrierPos == TotalArgs()`) | 380 |
| **intermediate (`0 < BarrierPos < TotalArgs()`)** | **97** |
| all-stack (`BarrierPos == 0`) | 16 |

Those 97 intermediate-barrier signatures are spread over **19 words**:
`as`, `default`, `dot`, `dotr`, `error`, `exposes`, `extends`, `get`,
`getr`, `guard`, `has`, `is`, `of`, `or`, `otherwise`, `tand`, `teq`,
`tis`, `tor`.

**Do not confuse this with the word-level count.** A frequently-quoted
figure — 20 words — comes from `precedenceShape`, which classifies a
WORD as mixed whenever its overloads do not all agree about sourcing.
That is a different question, and the two answers differ by exactly one
word: the 20 are these 19 plus **`apply`**, whose signatures are
all-stack (`0`) and all-forward (`N`) with **no intermediate barrier at
all**. So `apply` is a mixed-OVERLOAD word that is not a mixed-BARRIER
word. A refined ADR needs both notions and should name them separately:

- **mixed-barrier** — a property of ONE signature (97 of 493).
- **mixed-overload** — a property of a WORD whose signatures disagree
  (20 words, `apply` included), and the thing `boru describe` must
  report accurately, since it is what makes a single equivalence chain
  unstatable for that word.

> **Corrected 2026-08-24 (NUR098's fix).** `apply`'s `[Reach Any]`
> overload moved from `BarrierPos: -1` to `0`, so the word is now
> uniformly stack-only and the corpus has ZERO mixed-overload words —
> the 20-vs-19 discrepancy this section measures is retired (both
> counts read 19). The two notions remain worth naming, with `apply`
> as the historical example; the describe renderer keeps its split
> banner for any future word whose overloads disagree.

Either count carries the argument: a rule with one exception does not
describe a system where a fifth of all argument-taking signatures sit
between the two poles it names.

### 2.3 Stack-only

`BarrierPos == 0`. Every argument comes from the stack. **This is a
closed list**, pinned in REFERENCE.md §"Stack manipulation", and adding
to it "needs the same justification weight as a new init-time panic"
(ADR-004, as amplified by NUR023). The list has two tiers:

- **The Forth vocabulary** — `dup`, `drop`, `swap`, `over`, `rot`,
  `nip`, `tuck`, `dup2`, `drop2`, `swap2`, `over2`, `pick`, `roll`,
  `depth`, `stack`. Their entire meaning *is* the stack; there is no
  argument to name because the operand is the shape of the stack itself.
- **Semantic necessity** — two non-shuffle words, admitted under
  ADR-004's own exception clause ("whose entire meaning *is* the
  stack"), each with a stated reason:
  - `apply` (the `[Function]` overload): `args… fn apply` means "take
    the function off the stack and apply it to the preceding values."
    Forward collection would force callers to write the function's
    arguments *after* the function, fighting the left-to-right flow the
    word exists to serve. Consequence, documented: `apply f/r 5` raises;
    the spelling is `5 f/r apply`.
  - `__casematch`: the `case` desugar's internal match probe (each
    clause lowers to `if (v match __casematch) …`), always fed by the
    synthesized chain's stack discipline. The `__` prefix marks it
    internal by convention — **not** by enforcement, which is why it is
    user-reachable and therefore had to be pinned rather than waved off.

**The admission test a refined ADR should state.** A first draft of
this note offered a single criterion — "the word's *meaning* must be
the stack arrangement" — and the 2026-08-14 review pointed out that it
**rejects `__casematch`, one of the two entries it had just admitted**.
That is fatal to a one-criterion test, and worth stating plainly rather
than patching: `__casematch`'s meaning is `UnifyR(match, value) →
Boolean`, which is not about the stack at all. Its justification is
about its *call site*, not its semantics.

So the closed list has **two** admission criteria, and an entry must
name which one it claims:

1. **Semantic** — the word's meaning IS the stack arrangement. The
   Forth vocabulary qualifies; so does `apply`'s `[Function]` overload
   (the operand order *is* the point: the function arrives after the
   values it consumes) — and, since NUR098's fix (2026-08-24), the
   whole word: the `[Reach Any]` overload joined it at
   `BarrierPos: 0` on the same ground. Stack form merely reading
   better is NOT sufficient, and neither is "this is how it is
   currently called".
2. **Desugar-internal** — the word exists only as the target of a
   compiler/desugar expansion that constructs every call site itself,
   so no user-authored call form is being constrained. `__casematch`
   qualifies: `case` lowers each clause to `if (v match __casematch) …`
   and the synthesized chain supplies the stack discipline.

Criterion 2 is deliberately narrow, and it is NOT "it is only called
internally" — that phrasing would admit anything a library happens to
call in stack form. The claim is stronger: **every** call site is
generated, so the registration constrains no one. It is also the weaker
criterion of the two, and a refined ADR should say what follows from
that: a criterion-2 word is a candidate for becoming forward-eligible
the moment its desugar stops being the only caller. `__casematch` is
reachable and describable today (the `__` prefix is convention, not
enforcement), which is precisely why it must be pinned in the list and
not waved through.

### 2.4 Quoting slots (the orthogonal axis)

`QuoteArgs` and `NoEvalArgs` are **not** categories of argument
sourcing; they cross-cut all three above and are frequently confused
with them, which is reason enough for the ADR to name the distinction.

- **`QuoteArgs[i]`** — position `i` captures an upcoming Word as an
  `Atom` during forward collection, without evaluating it (`def name …`,
  `get key map`, `set key val store`). It affects collection.
  boru-defined fns declare the same capability as `name:Atom/q`.
- **`NoEvalArgs[i]`** — position `i` suppresses list auto-evaluation in
  `execMatch` (`fn` bodies, `if` / `for` branches and bodies, `do`
  bodies, and the higher-order code-body slots — `each`, `fold`,
  `scan`, `outer`, `inner`). It does **not** affect collection or
  Word→Atom conversion.
  > Note for anyone copying this list: `lang/go/CLAUDE.md`'s Quotation
  > System section names `call` among the code-body words. That is
  > **stale** — the only registered `call` today is `boru:service`'s
  > synchronous request word (`[Map Service]` / `[Map Service Map]`,
  > no `NoEvalArgs` slot). Verified 2026-08-14; the guide wants the
  > same correction.

A refined ADR should state the invariant: quoting decides *what a
collected token becomes*; `BarrierPos` decides *where the argument comes
from*. They compose freely.

---

## 3. The chaining rationale

ADR-004 asserts the forward default as a cultural preference. The
deeper reason — the one that makes it a *design* decision rather than a
style choice — is composition, and it is worth stating because it also
explains why the per-call-site levers are the right escape hatch and
per-word flips are not.

**The mirror equivalence — for ALL-FORWARD signatures.** When
`BarrierPos == TotalArgs()`, `f a b ≡ b f a ≡ b a f`. All three are the
same call: collection moves forward until the barrier, then backward
from the stack prefix. This is what lets a value flow into a word from
either direction without the word knowing which happened — the property
every pipeline relies on.

The qualifier is load-bearing and this note must not drop it, because
the categories above are exactly the cases where the chain fails: with
`BarrierPos == 1`, `f a b` cannot supply the second, stack-only
argument; with `BarrierPos == 0` it supplies none. The note's own
examples are the counterexamples — `or false true` raises
`insufficient_args` and `apply f/r 5` raises `signature_error`. Stating
the chain unqualified would reprint the very spelling the
`precedenceShape` fix (§4) removed from the help output, which is how
the first draft of this note had it before the 2026-08-14 review.

**What holds universally** is the weaker statement, and it is the one a
refined ADR should lead with: **full stack form always dispatches**,
because a forward-eligible position accepts a stack value too. That is
why it is the spelling `writePrecedenceMixed` falls back to for a word
whose overloads disagree.

**Where the split falls decides the binding.** `a f b` is `k=1`: `b`
fills `sig[0]`, `a` fills `sig[1]`. It is a *different split of the same
operands*, not a different rule and not a two-arg special case, which is
why `10 sub 3 = 7` while `sub 10 3 = -7` (and `sub 1 3 = 2`). The same
thing happens at every arity — `c f a b` and `f c a b` differ for
exactly this reason. Non-commutative operators read naturally when the
split leaves one operand on each side of the word, and the handler
convention (`args[1] OP args[0]`) exists to make that reading the
natural one; that is why infix is house style for them
(`STYLE-GUIDE.md` §S2).

**Why the levers are per-call-site.** `/s` forces stack (limit 0) and
`/f` forces forward (limit N) at a single call, and grouping `( … )`
contains a call. Because the levers are per-site, a word's declared
category stays a stable fact a reader can rely on. A per-word flip would
make the same spelling mean different things in different programs,
which is precisely the non-uniformity this register exists to prevent —
so "per-word flips are rejected" is not conservatism, it is the same
argument that motivates the closed list.

---

## 4. Diagnostics should explain the category

NUR023's verdict asks that diagnostics *explain why a word occupies its
category* rather than merely reporting a failed dispatch. Half of this
has landed and is worth recording as the precedent:

`boru describe` previously branched on a single binary `ForwardArgs`
flag, so **every mixed-barrier word printed the full forward
equivalence chain — including a spelling it refuses.** `apply` advertised
`apply x y <=> y apply x <=> y x apply` while `apply f/r 5` raised;
`or` advertised a chain while `or false true` raised `insufficient_args`.
The fix (`precedenceShape` / `writePrecedenceMixed` in
`lang/go/native/help/help.go`) classifies a word by whether its
signatures *agree* about sourcing, and renders the disagreeing case as a
per-group split plus the one spelling that satisfies every row — full
stack form, which always dispatches because a forward-eligible position
accepts a stack value too.

What remains, and what a refined ADR would license: a **dispatch-failure
diagnostic that names the category**. Today `apply f/r 5` raises a bare
`signature_error`; it could say that `apply`'s `[Function]` overload is
stack-only and suggest `5 f/r apply`. The information exists in the
signature — the diagnostic simply does not consult it. This is the same
shape as NUR066's proposed `dot` diagnostic, and the two should probably
share a mechanism.

---

## 5. What promotion changed in ADR-004

Promotion landed 2026-08-15 as a 20-word amendment — ADR-004 sits
exactly at the 111-word cap. What that buys is point 1 outright and
point 2 by naming; **point 3 does not fit an ADR entry and was not
attempted**, which is the cap working as designed: the rationale lives
in §3 here, and the amendment's citation is what carries a reader to
it. The default does not change at all.

1. **"The sole exception is the Forth stack vocabulary" becomes false
   and is replaced** by the four categories, with stack-only defined as
   a closed list of two tiers plus the admission test in §2.3.
2. **Mixed-barrier is named as a first-class category**, not an
   unmentioned middle ground — with §2.2's measurement as the evidence
   that it is ordinary rather than exceptional: **97 of 493 signatures**
   carry an intermediate barrier, spread over 19 words. (Not the
   frequently-quoted 20; that is the word-level mixed-OVERLOAD count,
   and §2.2 sets out why the two differ by exactly `apply`.)
3. **The rationale is stated** (§3), so the forward default reads as a
   composition argument rather than a preference, and the rejection of
   per-word flips follows from it instead of standing alone.

Unchanged: new words ship forward-eligible; the levers are the
call-site modifiers and grouping; a stack-only registration carries
init-time-panic justification weight.

**Open for the maintainer**, and deliberately not decided here:

- Whether `apply`'s `[Function]` overload should be *kept* (pinned, as
  now) or *removed* in favour of requiring `/s` at the call site.
  NUR023's original verdict listed both. Keeping it is the status quo
  and is now documented; removing it is a breaking change to a
  documented spelling.
- Whether the 0-arg idiom should be *stated as a preference* (`0` for
  constants) or left explicitly free. This note recommends the former
  only weakly — the byte-identical normalization means nothing is at
  stake beyond readability.
- Whether the category-naming diagnostic (§4) belongs in the ADR at all,
  or is an implementation follow-up the ADR merely permits.

---

## Evidence

- `core/go/registry.go` — `upsertFnDef`, the main sentinel-resolution
  site; the doc comment recording that `ForwardArgs` /
  `RegisterStackOnly` were retired. The other four sites are tabled in
  §1.
- `core/go/signature.go` — `MatchSignature` (the sig-selection entry
  point) and `sigArgMatches` / `sigTypeMatchesAsType` (the per-position
  match, including the `TypeArgs` slot rule); `core/go/engine.go` —
  `(*Engine).MatchSignature` and the forward-collection planning around
  it. Note that `core/go/match.go` does **not** hold the split rule
  despite its name: it holds the pattern checks (`patternsOk`,
  `forwardPatternRejects`, `OpenUnifyMap`). `lang/go/CLAUDE.md` still
  cites the pre-four-piece-split location `core/go/match.go`, which no
  longer exists; that citation wants the same correction.
- `core/go/CLAUDE.md` §"Signature Ordering" — the top-first convention,
  stated normatively.
- `lang/go/CLAUDE.md` §"Argument Ordering" — the call-form worked
  examples and the `/s` `/f` levers.
- `REFERENCE.md` §"Stack manipulation" — the pinned closed list, both
  tiers, with `apply` and `__casematch`'s rationales.
- `lang/go/native/help/help.go` — `precedenceShape`,
  `writePrecedenceMixed`; `lang/go/native/help/precedence_test.go` pins
  all three shapes in both directions.
- `design/FORWARD-COLLECTION-PHASES.10.md` — the two-phase collection
  model (plan-time token walk, run-time arrival loop) that a refined ADR
  should point at rather than restate.
- `NUR.md` §NUR023 — the record this note discharges the drafting half
  of.
