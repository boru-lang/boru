# Bidirectional Typing for the boru Checker — Design Proposal

Status: **`.0` proposal — not started.** This is a discovery note, not a
commitment. It evaluates reframing the carrier checker as a bidirectional type
system, scopes the safe subset, and traces the consequences for the bytecode
compiler (which *is* the checker plus a recording side effect, so the two move
together). No code has been written against this.

> Authority: the code is authoritative for behaviour; this page is rationale and
> plan. Where it cites line numbers (`carrier.go:2324`, …) they are anchors at
> time of writing and may drift — re-grep before relying on them.

Related prior art (read alongside): `legacy/CARRIER-STATIC-TYPECHECK-REPORT.10.ignore` (the
carrier abstract-interpretation foundation), `legacy/checker-compiler-architecture-review.0.ignore`
(the checker↔compiler coupling, largely landed), `COMPILABLE-SUBSET.md` (the
compiler's refusal contract), `legacy/checker-accuracy-review.10.ignore` (loop/guard/fn
analysis), `REFINE-NEWTYPE-VS-SUBSET.10.md` (the one-predicate `v.Is(t)` boundary
rule).

---

## 1. Executive summary

The checker today is a **carrier abstract interpreter**: it runs the program in
check mode over type-only "carrier" values, reusing the runtime dispatch
machinery *verbatim* (`boru.go:186` — "the actual runtime dispatch, matching, and
forward-collection machinery is reused verbatim so checker and runtime stay in
absolute parity"). Type information is derived by an informal pile of
propagation rules concentrated in `carrier.go` (**2,630 LOC**): forward
propagation through dispatch, narrowing at guards, joining at control-flow
merges, and a bounded loop fixpoint (`AnalyseLoopBody`, `carrier.go:1962`;
`AnalyseFnBody`, `carrier.go:2324`).

**Bidirectional typing** (Dunfield & Krishnaswami, *ACM Computing Surveys* 2021)
is the discipline of splitting that into two cooperating modes — **synthesis**
(`e ⇒ T`, the type comes *out* of a term) and **checking** (`e ⇐ T`, a known
type is pushed *in*) — with each syntactic form assigned to exactly one mode and
a single "subsumption" boundary where types are compared. It is the standard way
to make a checker's propagation rules *uniform* instead of case-by-case.

**Bottom line.** A *scoped* adoption is the highest-leverage volume reduction
available for the checker: it would replace much of `carrier.go`'s bespoke
propagation with two short rules per form, consolidate every type comparison to
one `Unify` site, shrink the `AnalyseFnBody` inference path, and — because the
compiler rides the checker — **widen the compilable subset** (more closures
compile to `PUSH_CLOSURE` instead of `OpFallback` islands). But it carries one
**hard constraint** that bounds what may be adopted: boru's runtime dispatches on
*argument values on the stack*, never on an expected result type, so
**return-type-directed overload resolution — bidirectional typing's signature
capability — is forbidden.** Using it would split the checker from the runtime
and therefore the compiler from the interpreter, breaking the soundness
foundation (`COMPILABLE-SUBSET.md` §1). This is a deliberate, staged rewrite,
not a bolt-on.

---

## 2. The problem

`carrier.go` has grown into the checker's largest single file because "derive
the type here" is answered differently in every context:

- **Forward propagation** — a word's result carrier (from its matched signature
  `Returns`) becomes the input to the next dispatch. This is the bulk and it is
  fine.
- **Narrowing** — a guard `x is T` refines `x`'s carrier in the guarded branch
  (`RefineIntoCarrier`).
- **Joining** — `if`/`case` arms produce carriers that must be merged
  (`JoinCarriers`).
- **Widening / fixpoint** — `AnalyseLoopBody` re-runs a loop body up to a fixed
  number of rounds joining carriers until "stable" (a hand-rolled widening; see
  the separate item in `legacy/checker-accuracy-review.10.ignore`).
- **Inference for unannotated bodies** — `AnalyseFnBody` re-analyses an
  anonymous lambda's body against bound carrier args because the lambda's static
  `Returns` is the conservative `[Any]` (see `lang/go/CLAUDE.md` "Lambda
  Syntax", the `Anonymous` flag read only in check mode).

Each is individually justified, but together they are *ad hoc*: there is no
single rule saying "in this direction, types flow thus." That is precisely the
shape bidirectional typing exists to regularise, and why the literature reports
large checker-code reductions and better error locality when it is adopted.

---

## 3. The bidirectional idea (in brief)

Two judgments, distinguished by which way type information flows:

- **Synthesis** `Γ ⊢ e ⇒ T` — read the type *out* of the term. Elimination
  forms (variable use, application, projection) synthesize.
- **Checking** `Γ ⊢ e ⇐ T` — push a *known* type *into* the term. Introduction
  forms (a bare lambda, a constructor, an empty literal) are checked.

Two rules connect the modes, and they are the **only** places types are
compared:

1. **Subsumption (mode switch):** in checking mode at a synthesizing form,
   synthesize `e ⇒ S` and verify `S` is compatible with the expected `T`. The
   single subtype/unification site.
2. **Annotation:** `(e : T)` synthesizes `T` and checks `e ⇐ T` — the escape
   hatch letting a checked form appear where synthesis is required.

The payoff: each form has one short rule in one mode; the annotation burden is
far lower than fully-explicit typing while staying decidable; and errors surface
at the failing mode switch rather than at a distant constraint solve.

---

## 4. Mapping onto boru (a concatenative adaptation)

boru is not a tree of lambda-calculus expressions; it is a sequence of stack
effects. The textbook intro/elim split does not transcribe directly, so we adopt
the *discipline*, not the rules. The carrier stack is the typed state.

| Bidirectional concept | boru realisation |
|---|---|
| **Synthesis** `e ⇒ T` | The forward carrier flow we already have: each word, given the synthesized carriers on top of the stack, produces result carriers from its matched signature's `Returns`. **Synthesis ≈ today's forward propagation.** |
| **Checking** `e ⇐ T` | The contexts where an expected type is already known and `carrier.go` currently hand-narrows: signature argument slots, **fn return annotations** (`def f fn [[…][String][…]]` → body checked `⇐ String`), typed-list/map element positions (`[:Integer]`), and `def x:T` bindings. |
| **Subsumption (mode switch)** | The single `Unify` / `v.Is(t)` boundary. boru already has `Unify` as the meet operation and a one-predicate boundary rule (`REFINE-NEWTYPE-VS-SUBSET.10.md`); bidirectional gives it **one canonical call site per boundary** instead of many. |
| **Annotation** | Already first-class: `def x:T`, param `:T`, return slots, `X/t` bounds. boru is, in effect, already annotation-rich — a favourable starting point. |

The sharpest concrete win is the anonymous lambda. Today it carries
`Returns=[Any]` and triggers the `AnalyseFnBody` inference pass. As an
*introduction form* it should be **checked** against the function type pushed in
from context: `each [x => x mul 2]` over a `[Integer]` pushes `Integer` into the
param, so the body is analysed with a concrete param type and **no separate
inference pass for the common case**.

---

## 5. The hard constraint (what bounds the proposal)

boru's defining invariant is that **the checker reuses runtime dispatch
verbatim** (`boru.go:186`). The runtime selects an overload from the *argument
values/types on the stack* (`matchSignature`) — it has **no notion of an
expected result type**. Therefore:

> **Type-directed overload resolution is forbidden.** The checker must never use
> a pushed-in expected type to *select which signature/dispatch fires*.

This is non-negotiable because:

1. A checker that resolved overloads by expected return type would resolve
   dispatches the **interpreter cannot reproduce**, breaking verbatim reuse.
2. Since the compiler is the checker-with-recording (`COMPILABLE-SUBSET.md` §1),
   the **compiled program would then diverge from the interpreter** — a direct
   soundness break, the one thing the whole compile/refuse contract rests on.

So checking mode may push types *inward to validate and to inform body
analysis*, but never to *change which dispatch is chosen*. This fences off
exactly the feature bidirectional systems are most celebrated for, and it is the
reason this is a careful subset, not a wholesale port.

**Adoptable (in scope):** push param types into lambda/fn bodies; check fn
bodies against declared return types; check container literals against typed
element positions; the single subsumption `Unify` site.

**Forbidden (out of scope):** any rule where the expected type disambiguates a
dispatch, picks among overloads, or admits a value the runtime's value-directed
match would reject.

---

## 6. Impact on the bytecode compiler

Because the compiler *is* the checker plus a recording side effect, §4–§5 are
also the compiler's story. Tracing it through:

### 6.1 Unaffected (the soundness load-bearers survive)

- **Provenance model.** Operand provenance (const / event / local / type /
  closure; `RememberOriginal`, `setProduced`) is about *where values flow on the
  stack* — orthogonal to check-vs-synthesize *direction*. Untouched.
- **Differential gate.** Bidirectional changes how types are *derived*, not what
  the interpreter *does*; `Run` is unchanged, so `RunCompiled` byte-identity and
  `compiled_differential_test.go` keep their premise.
- **Evaluation order.** Direction of type flow ≠ order of evaluation. boru stays
  left-to-right; the recorder stays hooked to the evaluation traversal, so
  `Finalize`'s linearisation still matches the interpreter tape — *provided*
  checking mode only annotates subterms with an expected type and never reorders
  which dispatch sites are visited (a property to guard; see §8).

### 6.2 Improved

- **Compilable subset widens.** Lambdas checked against a pushed-in function type
  are analysed with **concrete param types**, yielding fewer dynamic carriers →
  fewer `OpFallback` islands → more real `PUSH_CLOSURE` closures. The
  `SiteCounts` shift from `dynamic`/`meta` toward `mono`. Measurable via the
  `minCompiledRows` floor and the property fuzzer.
- **Refusal taxonomy consolidates.** Several refusals — `anyDynamicCarrier`, the
  opaque-output cases, `dynOutNativeOK` — are all "the forward pass could not
  produce a concrete type here", i.e. the **synthesize-failed** case. In a
  bidirectional framing these collapse to one question asked at the subsumption
  boundary, where `mono`/`poly`/`dynamic` classification already wants to live.
- **`AnalyseFnBody` shrinks**, and since the compiler rides the checker, the
  closure-unit construction path shrinks with it.

### 6.3 Endangered (the §5 constraint, restated for the compiler)

If type-directed overload resolution leaked in, the recorder would capture a
dispatch the interpreter never makes → compiled ≠ interpreted. The differential
gate would *catch* it, but only after building something architecturally
incompatible. The migration must therefore treat §5 as an invariant enforced by
review and by a targeted gate (§8), not merely a convention.

---

## 7. Migration plan (staged, lowest-risk first)

The goal is to validate the discipline on a small surface before touching the
forward-propagation core. Each stage is independently shippable and gate-clean.

- **Stage 0 — instrument & baseline.** Record current `carrier.go` LOC, the
  `SiteCounts` distribution over the spec corpus, `minCompiledRows`, and the
  count of `AnalyseFnBody` invocations. These are the success metrics (§9).
- **Stage 1 — explicit `⇐` for fn returns.** Reframe the fn **return check** as
  an explicit checking judgment "body `⇐` declared return", routed through the
  existing `Unify`/`v.Is(t)` boundary. No new dispatch behaviour. Prove it shares
  the runtime's boundary predicate (the `REFINE-NEWTYPE-VS-SUBSET.10.md` rule)
  and changes no result. **This is the toe-in-the-water; if it cannot be done
  without diverging from runtime, the proposal stops here.**
- **Stage 2 — push param types into lambda bodies.** Where a lambda is consumed
  by a word whose signature fixes the function type (e.g. `each`/`fold`/
  `filter`), check the lambda `⇐` that function type and analyse its body with
  concrete params. Expect `AnalyseFnBody` invocations to drop and compiled-row
  count to rise. Pin both.
- **Stage 3 — typed container element checking.** `[:T]` / `{:T}` literals and
  `def x:T` bindings as explicit `⇐` rules.
- **Stage 4 — unify the propagation core (optional, largest).** Only after 1–3
  prove out: reframe the forward propagation + narrowing + join rules as the
  synthesis half, collapsing the bespoke `carrier.go` paths. This is where the
  bulk of the volume reduction is — and the bulk of the risk.

Stages 1–3 are additive/reframing and reversible; Stage 4 is the rewrite. We
may legitimately stop after any stage: the value is monotonic.

---

## 8. Risks & mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Type-directed dispatch leaks in (§5) | **Critical** — soundness | Review invariant + a gate asserting the checker selects the *same* signature the interpreter does for every spec row (extend the differential gate to compare *selected sig*, not just result). |
| Recorder traversal order diverges under checking mode (§6.1) | High | Keep the recorder hooked to the evaluation walk; checking mode supplies an expected-type *context* only. Property-fuzz `RunCompiled` vs `Run` (already exists) catches order divergence as a result mismatch. |
| `carrier.go` rewrite (Stage 4) destabilises the loop fixpoint / guards | High | Defer Stage 4 until 1–3 are green; treat it as a separate proposal with its own baseline. The widening rework is already tracked in `legacy/checker-accuracy-review.10.ignore` and could land first/independently. |
| Volume "win" evaporates after counting new rule scaffolding | Medium | Stage 0 baseline + per-stage LOC measurement; abandon a stage that doesn't pay. (Honest precedent: the lattice-encoding and unify-dedup work in this branch came out net-neutral on LOC — efficiency/consolidation, not raw reduction. Hold this proposal to the same honesty.) |
| Conflict with verbatim-reuse invariant blocks Stage 1 | Medium | That is the *point* of making Stage 1 the gate: if return-checking can't be expressed as a `⇐` over the shared boundary without forking from runtime, the proposal is not viable and we learn it cheaply. |

---

## 9. Success criteria

The proposal succeeds iff, at the chosen stopping stage, all hold:

1. `make fmt/vet/lint/test` green, including `compiled_differential_test.go` and
   the property fuzzer (`BORU_FUZZ_*`).
2. The extended differential gate confirms identical *signature selection*
   between checker and interpreter on every spec row (the §5 invariant, made
   executable).
3. Net checker LOC (`carrier.go` + new rule files) is **down**, or — stated
   honestly per §8 — if it is only flat, the win is reclassified as
   consolidation/locality and that is said plainly.
4. `minCompiledRows` is **up** (subset widened) and `AnalyseFnBody` invocation
   count is **down** over the corpus.
5. Diagnostic locality improved: errors attributable to the failing mode switch,
   measured on a sample of mistyped programs.

If (2) cannot be met, the proposal is rejected regardless of the other metrics.

---

## 10. Alternatives considered

- **Do nothing.** Defensible: boru's hierarchy is shallow and the checker works.
  The cost is `carrier.go` continuing to accrete special cases. This proposal is
  only worthwhile if the volume/locality pressure is real.
- **Constraint-based inference (Algorithm W / union-find).** Rejected: boru has
  **no unification variables** — `Unify` is a structural meet/membership, not
  Robinson unification — so the union-find inference machinery has nothing to
  manage. (Established in the checker literature review on this branch.)
- **Incremental optimisations only** (the items already landed this branch:
  cached lattice depth + O(1) interval subtype tests + consolidated unify
  traversal). These improve *efficiency* and *consolidation* but leave the
  ad-hoc propagation shape intact. They are complementary, not a substitute.

---

## 11. Open questions

1. Can the fn-return check (Stage 1) be expressed as a `⇐` over the existing
   `v.Is(t)` boundary with **zero** change to selected dispatch? (Gating
   question — answer before any further stage.)
2. For higher-order words, where exactly is the "expected function type" known
   early enough to push into a lambda — at `matchSignature` time, or only after
   forward collection resolves the driving word? (Interacts with
   `FORWARD-COLLECTION-PHASES.10.md`.)
3. Does pushing param types in change the **order** in which a lambda body's
   dispatches are recorded relative to the interpreter? (Must be no; see §8.)
4. Should the subsumption site reuse `Unify` directly, or a thin `check(S ⇐ T)`
   wrapper that delegates to it, to keep one name for the boundary?

---

## 12. References

- Dunfield & Krishnaswami, *Bidirectional Typing*, ACM Computing Surveys 54(5),
  2021. The survey; design principles and the synthesis/checking discipline.
- Pierce & Turner, *Local Type Inference*, 1998. The origin of the modern
  checking/synthesis split.
- Internal: `legacy/CARRIER-STATIC-TYPECHECK-REPORT.10.ignore`,
  `legacy/checker-compiler-architecture-review.0.ignore`, `COMPILABLE-SUBSET.md`,
  `legacy/checker-accuracy-review.10.ignore`, `REFINE-NEWTYPE-VS-SUBSET.10.md`,
  `FORWARD-COLLECTION-PHASES.10.md`.
