# Formal Verification for boru — a layered plan

**Status:** draft / proposal (`.0`). A first machine-checked **seed of
milestone 6 now exists** at [`formal/lean/BoruCore.lean`](formal/lean/BoruCore.lean)
(Lean 4.15.0, no Mathlib): a deep embedding of the binary-word + forward-collection
fragment that *proves* source-spelling equivalence, the `end`-barrier
negative result, determinism, and the type-lattice order — and
cross-validates against the engine. The rest of `FORMAL-SPEC.md` §10–§11
names a mechanized model only as an aspiration. This note turns that aspiration into a concrete, staged
plan and — crucially — connects it to machinery the repo *already*
ships: the carrier-based static checker (`eng/go/check.go`,
`CheckState`), property-based testing (`boru:rand`, `test.check-prop`,
`shrink/`), and the `StackForm` IR with its proven
`Eval(Compile(src)) ≡ Run(src)` equivalence. Written against
`claude/formal-methods-boru-hdubtm` @ `6b6e20d`.

Companion reading: `FORMAL-SPEC.md` (the operational semantics this plan
mechanizes), `legacy/CARRIER-STATIC-TYPECHECK-REPORT.10.ignore` (the abstract
interpreter this plan generalizes), `legacy/PBT-PLAN.10.ignore` (the random layer
this plan sits above), `boru-bytecode-*.md` (the second backend that
makes compiler-correctness worth proving).

---

## 1. Two halves of the problem

"Formal methods for boru" splits cleanly into two questions that need
different tools:

- **Half A — analysis *of the language*.** Is the boru *language*
  well-behaved? Type soundness, determinism, confluence of forward
  collection, correctness of the `StackForm` compiler. The object of
  study is the semantics in `FORMAL-SPEC.md`.
- **Half B — analysis *of programs*.** Is *this boru program* correct,
  safe, total, effect-bounded? The object of study is user code.

The two halves meet at one mechanism — the **carrier**. boru's static
checker already runs the *real evaluator* over abstract "carrier" values
that hold type information instead of concrete data; that is textbook
**abstract interpretation** (`legacy/CARRIER-STATIC-TYPECHECK-REPORT.10.ignore`).
The central thesis of this document:

> Because boru *runs one evaluator over abstract values*, both program
> analysis and (with care) program verification reduce to **choosing an
> abstract domain** and a **discharge backend**. The expensive,
> boru-specific work is concentrated in two places — modelling forward
> collection, and making first-match dispatch sound over non-singleton
> carriers — and is paid once, not per analysis.

---

## 2. The verification pyramid — strength vs. cost

Methods are not interchangeable; they differ along
**(strength of guarantee) × (size of trusted base)**. Order them as a
pyramid, each layer catching what the one above is too expensive to
cover.

| Layer | Guarantee | Trusted base (TCB) | Cost | Status |
|---|---|---|---|---|
| **PBT** | none — finds bugs on sampled inputs | the test harness | ~free | **shipped** |
| **Abstract interpretation** (`boru check`) | *sound over-approx*: proves absence of a bug class, with false positives | the analyzer + its (informal) soundness argument | medium | **type domain shipped** |
| **SMT-backed refinements** | proves specific verification conditions | VC-generator **+ all of Z3** | high | proposed |
| **Lean** | machine-checked deductive proof | the Lean kernel only | very high | proposed |

Two principles govern how the layers relate:

1. **Lean is the apex, not a wider base.** It does not *replace* the
   cheaper layers — it *certifies* them. Its unique job is to close the
   trust gap the others leave open ("the checker is sound", "the
   compiler is correct") by turning those claims into theorems.
2. **Lean relocates trust, it does not remove it.** Even a complete Lean
   development still trusts (a) the Lean kernel, (b) that the Lean
   semantics faithfully models boru's *intended* behaviour — the
   **spec-adequacy problem** — and (c) that the theorem *statements* say
   what we mean. (b) is discharged not by proof but by cross-validating
   the model against the executable `*.tsv` conformance suite and the
   real engine. This is why PBT and the TSV corpus remain load-bearing
   even in a fully mechanized world.

---

## 3. The carrier as a generic abstract interpreter

`CheckState` today specializes the carrier to a *type* lattice. The
single most leveraged refactor in this plan is to **generalize the
carrier to a pluggable abstract domain**: an interface supplying a
lattice (`⊑`, `join`, `bottom`, `top`), per-word transfer functions, and
a widening operator. The existing step-loop becomes a reusable abstract
interpreter; every analysis below is then *a domain*, not a new
interpreter.

| Domain | Catches | Notes |
|---|---|---|
| **Types** (today) | signature/arity/return mismatches | `boru check` |
| **Nullness / `None`-flow** | `none` reaching a slot that can't accept it | ties to `Absent`/optional-field lowering (`FORMAL-SPEC` §3.4, §5.2) |
| **Interval / range** | out-of-bounds `get`, overflow reachability | makes `INTEGER-OVERFLOW-STRATEGY` / `IEEE-754-COMPLIANCE` checkable |
| **Taint** | untrusted input reaching an effectful word | security; feeds `PERMISSIONS.10` |
| **Effect / capability** | which effect classes a program may dispatch | *reuses the shipped PBT transparency lattice* (Transparent/Generator/Frozen/Opaque) — static counterpart to §7.2 capability checks |

### 3.1 Two boru-specific soundness obligations

Any carrier domain must respect both, or it is unsound:

1. **First-match dispatch under non-singleton carriers.** At runtime a
   *more specific* concrete value may select a *different* overload than
   a broad carrier would. A carrier that is a disjunction (or any
   non-singleton abstraction) must be treated as *possibly selecting
   any* matching overload, with the results **joined**. This is the
   central correctness obligation and the subtlest one.
2. **Termination.** Loops and recursion need **widening to a common
   ancestor**, or disjunction carriers explode (the carrier report flags
   path/union blow-up). The standard bounded-precision bargain.

### 3.2 Stack-effect analysis — the concatenative tradition, with a twist

boru is concatenative, and concatenative languages have a mature static
**stack-effect** lineage (Cat, Kitten — row-polymorphic stack typing).
Because every word declares typed input/output signatures, a checker can
*compose* signatures down a tape and flag underflow/overflow/arity
mismatch statically. The boru-specific twist is **forward collection**
(`FORMAL-SPEC` §6.4): a word's stack effect depends on how many forward
tokens it consumed and on the barrier rules (`|`, `/f`, `/s`, `/N`,
`end`). Modelling that is novel work, but it catches exactly the
"stranded operand" class the `FORWARD-STRAND-ADVISORY.10.md` and
`FORWARD-COLLECTION-TRAPS.0.md` notes describe.

### 3.3 Refinement types — boru is already a liquid-types language

`FORMAL-SPEC` §5.2 defines **dependent scalar refinements**:
`Base op constraint`, value-sensitive subset types. `def Pos refine
Number gt 0` is a predicate subtype — the surface syntax of
**refinement / liquid types** (LiquidHaskell, F\*, Dafny). Today those
predicates are checked *dynamically* at construction boundaries. To
check them *statically* you discharge implications (e.g. body output
`> 0` given `x > 0`) — an SMT obligation. Wiring a Z3 backend to
discharge refinement predicates gives boru **lightweight functional
verification in its existing syntax, with no new language surface**: the
contracts already *are* the types. This is the single highest-value
differentiator in the middle of the pyramid.

---

## 4. Half A — mechanizing the language in Lean

### 4.1 The deep embedding

Build a **deep embedding** of the parser-free abstract machine
(`FORMAL-SPEC` §4 + §6 — skip surface syntax; this answers the spec's
own open question §11.5):

- Lean datatypes for the abstract syntax (`Tape`, `Item`, `Value`,
  `WordCall`, …).
- The type lattice and the typing judgment (`FORMAL-SPEC` §5).
- An inductive `Step` relation for the small-step rules (§6), and
  `Eval` as its reflexive-transitive closure.

The spec is already in inference-rule form, so this is *transliteration,
not invention*. The cost is the **mechanization tax**: Lean forces you
to pin down everything paper rules gloss — the §6.4 forward-collection
stopping condition, the §5.6 "prefer signatures that avoid consuming a
following function word" heuristic, error-propagation order, and the
`Absent`/optional-field unification. Surfacing that underspecification is
a *feature* (it resolves several §11 open questions) and the real time
sink.

### 4.2 The metatheorems

State and prove what the spec currently only asserts:

- **Progress + preservation** — a well-typed tape does not get stuck
  except at a declared `error(c, m)`.
- **Determinism** of the sequential core (modulo the concurrency words
  of §7.4, which are deliberately non-deterministic).
- **Forward-collection confluence** — the spec's own claim that
  `10 3 sub`, `10 sub 3`, `sub 3 10` all yield `args[0]=3, args[1]=10`
  becomes a *theorem*, not three TSV rows.
- **Compiler correctness** — `Eval(Compile(src)) ≡ Run(src)` for
  `StackForm`, currently an example-test, is the natural first Lean
  theorem and de-risks the `boru-bytecode-*` effort (a second backend
  raises the value of a proven equivalence sharply).

### 4.3 The connection problem (the crux)

A Lean model proves things about *the Lean model*, not the Go engine.
Three bridges, in increasing strength:

1. **Lean-as-spec, cross-validated.** Differential-test the Go engine
   against the Lean model via the `*.tsv` corpus. Cheap; weak link
   (untested inputs can diverge).
2. **Lean-as-reference-implementation.** Lean 4 compiles to native; make
   the Lean evaluator the executable reference and Go the optimized
   engine validated against it. Strong; large architectural commitment.
3. **Translation validation.** Each compile/run emits a certificate a
   Lean-checked validator accepts — per-run guarantee without proving
   the whole compiler. Usually the best effort/strength trade-off.

---

## 5. Half B — verifying individual programs (the carrier → Lean path)

This is the part that unifies the document. The question: can the
carrier mechanism *verify* a specific program, not just type-check it?
Yes — by swapping the carrier domain for **symbolic terms** and emitting
Lean.

### 5.1 Carrier with a symbolic-term domain

Run the check-mode loop with a domain whose elements are *symbolic
expression trees*:

- symbolic input `x` → carrier `Var "x"`
- `add 1 2` → `Add (Lit 1) (Lit 2)` (or directly Lean `+`)
- `if c a b` → `Ite c⟦⟧ a⟦⟧ b⟦⟧`, or fork the state with a path condition

Running a function over symbolic inputs yields a **closed-form term
denoting its output as a function of its inputs**. That term
pretty-prints to Lean — both the model *and* the proof skeleton.

### 5.2 Worked example

```boru
def clamp fn [[x:Number lo:Number hi:Number] [Number] [
  if (x lt lo) [lo] [if (x gt hi) [hi] [x]]
]]
```

Symbolic output carrier: `ite (x<lo) lo (ite (x>hi) hi x)`. Emitter
output:

```lean
def clamp_model (x lo hi : Int) : Int :=
  if x < lo then lo else if x > hi then hi else x

theorem clamp_in_range (x lo hi : Int) (h : lo ≤ hi) :
    lo ≤ clamp_model x lo hi ∧ clamp_model x lo hi ≤ hi := by
  unfold clamp_model; split <;> split <;> omega
```

The carrier wrote both definition and proof skeleton; `omega` closes it.
Note the free win: a parameter typed `Pos = refine Number gt 0` emits as
the hypothesis `(h : x > 0)` — **refinement types become Lean
preconditions automatically.**

### 5.3 The catch — and why b2 rides on b1

`clamp_model` is a *re-encoding* of `clamp` in Lean math. Mapping boru
`add`→Lean `+` is **not** obviously faithful — boru has an
integer-overflow strategy and IEEE-754 floats, so float-`add` is *not*
`Int.+`. If the shallow mapping is wrong, the theorem is about a fiction.

So the translation carries a **per-word adequacy obligation** against the
deep semantics of §4:

```
∀ w.  ⟦ symtransfer(w) ⟧  =  Eval(w)
```

The good news:

- It is a **fixed, finite** library — one lemma per *native word*, not
  per program.
- Once discharged, adequacy of `f_model` for **any** user program
  follows by induction over the tape. `f_model = ⟦Eval prog⟧` is then a
  theorem.

This **collapses the b1/b2 distinction**: the symbolic carrier is a
*verified front-end to Lean* (a b1 "the analyzer is sound" result), and
that is exactly what makes per-program verification (b2) cheap and
automated. Pay once for the word library; afterwards each program is
"symbolically execute → emit a model adequate by construction →
discharge the VC with Lean automation."

### 5.4 Boundaries to respect

1. **Loops / recursion.** No finite closed-form term for unbounded
   recursion. Emit a *recursive* Lean `def` and prove by induction, or
   require a user-supplied invariant / decreasing measure (same bargain
   as Dafny / F\*). Cleanest for straight-line and bounded code.
2. **Effects.** A function touching `Σ` is not a pure `Int → Int`.
   Restrict the pure-model mode to **Transparent** words (the PBT
   transparency policy already identifies exactly the words with clean
   denotations); model the rest in a Lean state/IO monad if needed.
3. **Spec adequacy.** Per-word lemmas still rest on `Eval` faithfully
   modelling the Go engine — discharged by §4.3 cross-validation, not by
   proof.

### 5.5 Emit from `StackForm`, not the raw tape

`StackForm` already has a machine-checked `Eval(Compile(src)) ≡
Run(src)`. That makes it a strictly better emitter source than the
surface tape: linearized, and the surface→IR step is already
equivalence-checked, so the emitter handles only the clean IR and the
adequacy proof rides on an established equivalence.

### 5.6 Two realizations (a TCB fork)

- **Go carrier emits Lean text** — reuse the existing engine; fast to
  prototype. Cost: the pretty-printer and emitted `f_model` enter the
  TCB *unless* paired with a per-program adequacy check `f_model =
  ⟦Eval prog⟧` (translation validation).
- **Symbolic evaluator *in* Lean as a reflection tactic over the deep
  embedding** — adequacy by construction (proof by reflection);
  strongest, but rebuilds the carrier in Lean. The two share the
  *design* (same transfer functions); the Lean one is the certified
  copy.

Recommendation: prototype with the Go emitter to prove ergonomics (the
auto-discharged `clamp` proof is the compelling demo), but treat the
**per-word adequacy library over the deep embedding as the real
deliverable** — it is what turns a nice transpiler into a sound
verifier, and the same word-denotation lemmas are what a
certificate-checking SMT path (Z3 emits certificates checked in Lean)
would reuse.

---

## 6. Roadmap

Sequenced for maximum signal per unit effort; each milestone is
independently valuable and nothing changes default behaviour.

| # | Milestone | Layer | Effort | Payoff |
|---|---|---|---|---|
| 1 | Type-lattice laws + source-spelling equivalence as **PBT properties** via `test.check-prop` | PBT | low (infra exists) | catches spec/impl drift today; no new toolchain |
| 2 | **Generalize `CheckState` carriers** to a pluggable abstract domain interface | AI | medium | unlocks nullness/interval/taint/effect as *domains* |
| 3 | **Effect/capability inference** domain reusing the transparency lattice | AI | low | static sandbox decisions; feeds `PERMISSIONS.10` |
| 4 | **Stack-effect checker** over composed signatures + forward-collection model | AI | medium | catches "stranded operand" statically |
| 5 | **SMT discharge of `refine` predicates** (Z3) | SMT | high | turns existing refinement syntax into real verification |
| 6 | **Lean deep embedding** of the scalar+refinement+record fragment; prove progress+preservation; cross-validate vs `eng/spec/*.tsv` | Lean | high | the soundness result the spec implies; answers §11.5 — *seed landed: [`formal/lean/BoruCore.lean`](formal/lean/BoruCore.lean)* |
| 7 | **Per-word adequacy library** + symbolic-carrier emitter; worked `clamp` end-to-end | Lean | high | sound per-program verification (b2); reused by certificate-checked SMT |
| 8 | **`StackForm` compiler-correctness theorem** in Lean | Lean | medium | de-risks the bytecode backend |

A defensible first concrete deliverable is **milestone 6** scoped to the
scalar + refinement + record fragment (no concurrency, no I/O): it is
self-contained, exercises the genuinely hard part (refinements +
records), answers an existing open question, and is the substrate every
later Lean result builds on.

---

## 7. Open questions

1. **Smallest maintained Lean model** (restates `FORMAL-SPEC` §11.5):
   parser-free abstract machine only, or parser + machine? This plan
   assumes parser-free first.
2. **Connection bridge** (§4.3): cross-validation, Lean-as-reference, or
   translation validation — and can they be staged (start with 1, earn
   3)?
3. **Carrier domain interface shape**: what is the minimal Go interface
   (`Join`, `Widen`, `Transfer`, `Leq`) that serves types, intervals,
   nullness, taint, *and* symbolic terms without leaking domain
   specifics into the step-loop?
4. **First-match soundness**: is "join over all overloads a non-singleton
   carrier could select" precise enough, or is a more refined
   abstraction (e.g. guarded disjunctions carrying path conditions)
   needed to avoid crippling false positives?
5. **Effect modality in Lean**: state monad, free monad, or a coarse
   effect-label algebra keyed off the transparency policy?
6. **Refinement predicate fragment**: which `op constraint` predicates
   are in the decidable SMT fragment (linear arithmetic, strings) vs.
   pushed to Lean for manual discharge?
