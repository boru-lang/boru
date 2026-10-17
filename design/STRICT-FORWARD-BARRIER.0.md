# Strict Forward Barrier

Status: **shipped as the default.** A bare function word beginning its own
dispatch is a forward-collection barrier, uniformly, regardless of arity.
`BORU_NO_STRICT_BARRIER=1` restores the legacy wait-through behaviour as a
transitional escape hatch (slated for removal). This note records the
motivation, the final rule, and the migration.

## The shipped rule (uniform)

> **Every bare function word that begins its own dispatch is a barrier.**
> A parked forward that cannot commit with the args it already holds is
> STRANDED — a `signature_error`, not a wait-through. The rule does not ask
> "how many args does the word collect": a nullary `context`/`gensym` and a
> binary `add` are both function dispatches, so both strand a waiting
> forward. Group the call in parens so its RESULT becomes the argument:
> `print (add 1 2)`, `(context) eq (context)`, `not (not 5)`.

The **sole exemption is structural, not arity-based**: a dot-access chain
(`m.a`, `MathUtil.now`) is implicitly-parenthesized navigation — it produces
exactly one value and collects nothing further, so it feeds forward
collection like any other value and is never a barrier (`size m.a` works).
A dot-access that resolves to a FUNCTION is still a function: given trailing
args it dispatches (`L.f x` collects `x`), so parenthesize `(L.f)` when you
want the value, not the call. (An earlier revision tried an arity-based
exemption — "a word that collects 0 forward tokens is transparent" — and it
was rejected as fragile special-casing: functions are treated uniformly, only
the navigation *syntax* is grouped.)

### Two sharp edges the migration surfaced

- **`(fn […])` auto-invokes a 0-arg overload.** A Function value with a
  nullary overload, reaching the pointer with no args, fires the 0-arg sig
  (the "value vs call" edge, lang/go/CLAUDE.md). So `def h (fn […])` where the
  fn has a `[]` overload binds `h` to the 0-arg *result*, not the FnDef —
  wrapping is WRONG there. Bind it with the keyword form `def h fn […]`
  instead (the keyword slot captures `fn` and installs the FnDef directly).
- **The `word` splice is forward-form only.** `(word [body])` fires the
  `__SP` inside the paren, so splice-def must use the keyword form
  `def name word [body]`, never a paren.

Both are why `def`'s keyword forms (below) are load-bearing under the strict
rule, and why the eng-kernel spec fixture grew the same `fn`/`word` keyword
forms the lang layer synthesizes.

## History: the opt-in prototype

The rule first shipped behind `BORU_STRICT_BARRIER=1` (default off). The
sections below record that prototype's motivation and fallout measurement;
the rule is now the default and the gate variable inverted
(`BORU_NO_STRICT_BARRIER`).

## The question

Today a function word beginning its own dispatch is a forward-collection
barrier *only when honouring the barrier is possible*: `commitBarrierForward`
(engine.go) commits the nearest parked forward iff a real overload
consumes exactly the args it already holds. When the parked word CANNOT
fire — `print` holding 0 of 1, a 2-arg-only fn holding 1 — it keeps
waiting, and the boundary word's **result** arrives into the open slot.
That wait-through is what makes these work without parens:

```boru
print add 1 2        ;# add's result feeds print
size iota 3          ;# iota's result feeds size
not not 5            ;# unary chaining
typeof gensym        ;# a nullary fn as an argument
```

The wait-through exists only for slots an unknown token cannot be pruned
against — in practice `Any` slots and untyped user-fn params. Typed
native slots (`get`, `add`) already fail loudly at plan time with the
"group the call in parens" hint. So the same surface shape is legal or
illegal depending on the *waiting* word's overload set, and adding an
overload to a word can silently re-bind existing call sites downstream
(`print add 1 2` works precisely because print has no 0-arg overload —
give it one and the line changes meaning).

The strict rule under test: **a function word never feeds forward
collection.** When a fn word begins dispatch and the pending forward
cannot commit, raise the same signature error the typed plan-time path
gives, instead of waiting.

## The prototype

`BORU_STRICT_BARRIER=1` (engine.go: `strictForwardBarrier`,
`strandedForwardError`, called from the two `commitBarrierForward` call
sites — `stepWord` and `stepWordUsurp`). One exemption: engine-internal
boundary words (`__`-prefixed frame-tail words) are not source-level
statement boundaries. There is deliberately NO def exemption — see the
ruling below.

## The ruling: `def foo add 1 2` is an error; keyword slots carry the idiom

An earlier revision exempted `def` — first by name, then via a
`Signature.RestArgs` "statement-rest slot" attribute — so that ANY
function word could keep feeding def's body slot. Both were rejected:
`def foo add 1 2` silently binding `3` is exactly the wait-through
class the strict rule exists to kill, and a Go-only signature
attribute violates the language's expressibility principle — boru has
macros, so whatever def can declare, a boru-authored binder must be
able to declare too.

What survives instead is the **KEYWORD slot** — the language-native
way to say "this argument position matches the literal word `fn`":

> A signature position with `/q` (QuoteArgs) AND a concrete **Atom
> pattern** admits exactly one literal word, matched by raw token
> NAME, binding-agnostically ("capture trumps binding" — the same
> rule every /q capture follows). Any other token makes the
> signature non-viable at that position.

def declares the fn-definition FORM as an ordinary overload
(native_definition.go):

```
[ name:Atom/q  fn:Atom/q=fn  sigs:List ]     ;# def name fn [in out body …]
```

All three operands resolve at plan time — nothing parks, no
wait-through — so `def f fn [...]` is pure structural dispatch and
works identically under the strict rule, while `def x add 1 2`,
`def s size [1 2 3]`, and `def x:T add 1 2` are stranded (write
`def x (add 1 2)`). This is Scheme's syntax-rules literals arriving
in boru signatures: the same mechanism is what user macros/binders
need for DSL keywords (`for x in xs […]`).

Three kernel seams make keyword slots sound (all in this change):

1. **`patternsOk` quote-slot fix** (core/go/match.go): a /q position's
   Atom pattern matches the raw token name (Word or Atom). It must
   NOT resolve the binding first — `fn` is Defs-bound to an
   FnDefInfo, which would never unify with `Atom("fn")`; the
   pre-existing resolve-then-unify order made keyword patterns
   dead-on-arrival.
2. **Token-aware capture** (`capturesForwardToken`, engine.go): the
   plan scan's function-word barrier consults the TOKEN — a keyword
   slot captures only its own literal; every other word stays a
   barrier. Without this, one keyword sig would disable the barrier
   at its position for every def statement (the cross-barrier
   pre-evaluation bug class).
3. **Keyword viability pruning** (`resolveForwardArgs`): the keyword
   overload's larger arity must not widen the pre-evaluation scan —
   `def g (fn […]) (g 3)` must not evaluate `(g 3)` before the 2-arg
   def binds g. A signature whose keyword slot cannot match the token
   at its position is pruned before any group evaluation.

### Constructor keyword forms (DONE) — the def-idiom corpus is strict-clean

Every closed-set constructor now has a synthesized def keyword
overload, so all the `def name <ctor> …` idioms are structural
dispatch and survive the strict rule. The forms are mirrored
MECHANICALLY from each constructor's own live signature table
(`registerDefKeywordForms`, `native_definition.go`), so a constructor
that grows an overload propagates to def automatically:

- **Plain forms** — `fn`, `fnsig`, `refine`, `class`, `surface`,
  `enum`, `quote`, `word`: `[name/q ctor/q …ctor-args]`.
- **Gen chains** — `gen [params] <tail> …` for tails `fn`, `class`,
  `refine`, `fnsig`: `[name/q gen/q params:List tail/q …tail-args]`.
  The `def Name<T>` angle sugar desugars to exactly this token shape
  (`parser/parse.go`). The chain handler runs `gen` first, then
  re-applies the tail's own evaluation policy to its operands (a
  schema field like `{v:T}` or `(Self of [T])` references the gen
  placeholders, so its evaluation is DEFERRED until the placeholders
  are bound — `defFormVia`).

The result: the whole `def name <ctor> …` family is structural
dispatch, so none of it strands under strict. (This is NOT the same
as the whole spec corpus being strict-clean — see "Remaining
strict-mode fallout" below. An earlier revision of this note claimed
zero corpus fallout; that was a mismeasurement against a stale binary.
The def-idiom family is clean; the non-def wait-through idioms are
not, and the dot-access barrier below adds more.)

Two constructors are deliberately NOT mirrored:

- **`make`** — the one INSTANCE constructor (per-call fresh identity,
  `OpMakeMap` recorder events). Routing it through the composite form
  hides the inner dispatch from the bytecode recorder and loses the
  operand provenance compiled programs depend on. `def p make P {…}`
  keeps the wait-through path (it is a value bind, not a strict-rule
  hazard — `make`'s result is a value, and the following statement is
  a separate dispatch). A keyword form needs recorder plumbing first.
- **`quote`'s word-capture sig** — its `[Atom/q]` overload is itself
  a structural (/q) slot, and `hasStructuralSlots` excludes such base
  sigs from the mirror (a shifted unpatterned /q slot would capture
  any word two-plus positions out for every def statement, disabling
  the function-word barrier there — the `macroexpand (twice 5)` after
  a `def` regression). `def a quote foo` spells as `def a foo/q`;
  `def xs quote [1 2 3]` (the list sig) mirrors fine.

The keyword slots rest on the three kernel seams from the previous
change (patternsOk keyword match, capturesForwardToken, keyword
viability pruning) plus one new invocation seam: `eng.DispatchSig`
runs a captured constructor's own signature over the operands after
the keyword, so the constructor's handler stays the single
implementation.

### Dot-access is barrier'd too (DONE)

A dot-access chain (`Rand.int …`, `m.a.b`) is a forward-collection
barrier just like a plain function word: `print Rand.int 0 10` strands
under strict; write `print (Rand.int 0 10)`. This closed a real gap —
a Reach lowers to a paren-wrapped `( recv dot key )` span, so the
per-word `stepWord` barrier check runs INSIDE that paren and never sees
the outer parked forward, and the chain's result silently fed the
forward. Two gated hooks fix it (`engine.go`): the forward-collection
scan treats a Reach as a barrier (does not pre-evaluate it into the
collecting word's slot — the collecting word parks), and the
statement-level Reach branch runs the same commit-or-strand check as
`stepWord` before the paren wrap, where the parked forward is still in
scope. Both are gated on `strictForwardBarrier`, so default behaviour
is byte-identical.

## Remaining strict-mode fallout (lang/spec, TestSpecProd)

Default (gate off): green. Under `BORU_STRICT_BARRIER=1`:

- After the def constructor keyword forms (but before the dot-access
  barrier): **~38 rows**. The def-idiom family is clean; what remains
  is the genuinely non-def wait-through — unary `Any` collectors fed
  by a following call (`size iota 3`, `not not 5`, `typeof iota 3`),
  nullary fn words as arguments (`context eq context`, `typeof
  gensym`), `quote` feeding a slot, `def s context` (a bare nullary
  word as a def body), and the `context`/store idioms.
- After the dot-access barrier landed: **~181 rows**. The jump is
  correct — every `X.y` dot-access used in a forward position (a
  pervasive idiom across the spec) was silently bypassing the rule
  before and is now caught. These are genuine wait-through sites, not
  regressions; each strands with the intended parens-hint error.

Every failure is the intended kind (stranded-forward signature error
with a parens hint); no silent meaning changes. The migration is
mechanical (wrap the collected call in parens), but it is ~181 spec
rows plus any doc/README examples — the real cost of flipping the
default (see "If adopted" #3).

## Assessment

For the strict rule:

- Uniformity: typed and `Any` slots behave identically; one teachable
  rule ("a function word never feeds forward collection; group with
  parens") and the existing good error message.
- Robustness: whether `f X …` nests no longer depends on the *waiting*
  word's overload set, so growing an overload can't silently re-bind
  distant call sites.
- The speculative-commit machinery's hazard class (guard pre-emption,
  phantom else — FORWARD-COLLECTION-PHASES.10.md) loses its remaining
  "keeps waiting" arm.

Against:

- It outlaws unary prefix chaining (`not not 5`, `size iota 3`) and
  nullary-fn arguments (`typeof gensym`) — the places where the
  wait-through is genuinely pleasant. Note the tension: the same
  mechanism that makes `print add 1 2` fragile is the one that gives
  boru its partial Polish-notation feel on `Any` slots. A stricter
  language is a more parenthesised language.
- The checker must mirror the rule for words whose binding kind is
  unknown at check time (forward references) — stays gradual there.

## Follow-ups

**Done:**

- ~~def story~~ / ~~keyword overloads for the other closed-set
  constructors~~ — the keyword-slot mechanism and the def constructor
  forms (see the ruling and the constructor-forms section).
- ~~Surface keyword slots in `boru describe`~~ — `help.SigInfo.Keywords`
  renders a keyword slot as `fn/q` / `gen/q` / `<ctor>/q` instead of a
  bare `Atom`, so `boru describe def` distinguishes every overload
  (`native_help.go::sigKeywordSlots`, `help.go::writeSigs`). A
  capture-any `/q` slot (no pattern, e.g. `quote`'s `[Atom]`) stays
  bare.
- ~~The boru-authored surface~~ — a `/q` param whose atom names no type
  is a KEYWORD slot: `def between fn [[a:Integer in/q b:Integer] …]`
  matches only the literal word `in` (`fn_params.go::keywordParam`,
  spec `lang/spec/keyword-slot.tsv`). This is the source spelling of
  the def constructor forms — the syntax-rules-literal mechanism, for
  user binder/DSL keywords — so keyword slots are now expressible in
  the language itself, not a Go-only capability. (In `afn` the keyword
  MATCHES correctly but the captured atom leaks onto the stack like any
  afn unnamed param — a pre-existing afn arg-handling limitation,
  orthogonal to keyword slots; the `fn` form is the canonical spelling.)
- ~~Extend the check to the dot-access dispatch path~~ — the dot-access
  barrier (above) fires in the check/compile passes too (shared step
  loop), so `boru check` and the compiler agree with the interpreter.

**Not done — and why:**

- **`make` keyword form** — deliberately left paren-only. `make` is the
  one INSTANCE constructor (fresh per-call identity, `OpMakeMap`
  recorder events, `ReturnsFreshInstance`). The composite-form seam
  `eng.DispatchSig` calls the handler directly, bypassing `execMatch`'s
  check-mode intercept — so it misses the make-body auto-eval
  (`RecordMakeMap`), the fresh-instance carrier, AND the `RecordCall`
  that registers the instance's per-value provenance. A `make` routed
  that way binds an instance of "unknown provenance" and any downstream
  `(p0 add p1)` refuses to compile (`TestMergedWordSeam_ClassTuple`). A
  correct make keyword form must route the inner construction through
  `execMatch` (or desugar `def name make T {…}` → `def name (make T
  {…})` at the parser). Both are more than the payoff: `def p (make P
  {…})` already works everywhere, and under strict that paren is the
  one-token cost. So make stays the single constructor without a
  keyword form, by design.

- **Check-mode diagnostic (vs hard error)** — the strand is a hard
  `signature_error` in ALL modes (interp, check, compile), which is
  already CONSISTENT (check and compile both fail at the same point).
  Downgrading it to a "diagnostic + continue" in check mode — the usual
  guaranteed-error-mirror treatment — is UNSOUND here: continuing past
  the strand re-enables the wait-through the rule forbids, so the check
  pass would compute the non-strict result while reporting a strand.
  The barrier can't cleanly downgrade-and-continue, so the abort is
  correct; left as-is.

**The remaining decision:**

- **Flip the default** vs keep the env gate. The corpus is NOT
  strict-clean (~181 rows, see "Remaining strict-mode fallout"), so
  flipping is a real migration: parenthesise ~181 spec rows plus any
  doc/README examples, pin the new errors as negative rows, revisit
  `forward-barrier.tsv` §6's "keeps waiting" rows, and (optionally) add
  a `fmt` auto-fix that inserts the parens. This is a language-default
  semantics change for all users — it removes the partial-Polish feel
  (`not not 5`, `size iota 3`) — so it is a maintainer decision, not a
  mechanical follow-up.
