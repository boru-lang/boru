# boru Macros — Implementation Plan

**Status:** design / implementation plan
**Scope:** a user-level macro system for boru — `macro` definition, a
quote-template surface with `unquote`/`splice`, expansion in both
interpreter and (future) compiled modes, and a staged path to hygiene.
**Predecessor:** `legacy/LISP-ANALYSIS.5.ignore` §5 + §8 (Tier-1 items #1–3) names
macros as the single biggest unrealized LISP dividend. This plan turns
that recommendation into a concrete build order.

> The thesis from the analysis still holds: a macro system is **assembling
> existing primitives** (`__SP` splice, `NoEvalArgs`, `quote`, the marker
> tape, closure capture, `DefCleanup` teardown) into a coherent surface —
> not inventing new machinery. This plan is mostly *exposure and
> sequencing*, with two genuinely new pieces (raw-form capture and a
> provenance-aware expander) and one tiny standalone primitive (`gensym`).

---

## 0. TL;DR

A macro is **an `fn` the expander runs on unevaluated token forms, whose
returned token list is spliced into the call site.** Everything else is
existing boru:

| Lisp | boru surface | Backed by (existing) |
|---|---|---|
| `defmacro` | `macro [[params] [template]]` | `fn` + auto-`NoEvalArgs` + expand-time splice |
| `` ` `` quasiquote | `quote [ … ]` (template region, default-data) | `quote` / `Eval=false` |
| `,` unquote | `unquote x` (insert one **grouped** node) | `(expr)` / `preEvalParens` collapse |
| `,@` unquote-splicing | `splice xs` (insert **flattened** elements) | `word` / `__SP` (`NewSplice`) |
| `gensym` | `gensym` (fresh non-colliding atom) | **new**, trivial (registry counter) |

Two new ingredients only: **raw-form capture** on the input side (because
`NoEvalArgs` does *not* suppress `preEvalParens` or forward word→atom),
and a **provenance-aware expander** that walks the template resolving
`unquote`/`splice` (and, later, renames for hygiene). The `unquote`/`splice`
boundary doubles as the hygiene provenance marker — Lisp's hardest
implementation problem is handed to us by the surface syntax.

---

## 1. Background facts this plan leans on

All established in `eng/go/CLAUDE.md`, `lang/go/CLAUDE.md`, and verified
empirically on this branch:

1. **One mutable tape.** The engine steps a single `e.stack` with
   `e.pointer`; "forward tokens" are just later indices. There is no
   separate forward list.
2. **`__SP` splices ahead of the pointer.** `NewSplice` (`Word/__SP`),
   when stepped in `stepLiteral`, is replaced by its payload (a list
   contributes its top-level elements; any other value contributes
   itself) and **re-stepped against the live stack**. This already
   rewrites the tape in front of the cursor — it *is* the expansion
   engine.
3. **`NoEvalArgs` is not enough for macro capture.** Per the quotation
   rules it suppresses only `autoEvalList`; it does **not** stop forward
   collection, word→atom conversion, or `preEvalParens`. A macro needs a
   stronger "capture the next forward form raw" mode.
4. **`preEvalParens` fires for normal forward words, not for raw capture.**
   So a paren'd macro argument like `(x gt 10)` must be captured as tokens,
   while the *same* paren emitted into the expansion is later collapsed by
   `preEvalParens` when the expanded `if` dispatches — i.e. evaluation is
   correctly deferred to the generated code.
5. **Blocks leak defs.** Empirically `(def x 1) x → 1` and
   `do [def x 1] x → 1`: parens and `do` both write into the same
   `r.Defs`. There is no per-block scope. **Hygiene therefore needs
   teardown, not just renaming.**
6. **`do` is an error boundary; parens are not.** Empirically
   `(1 div 0) 99` halts, but `do [1 div 0] 99 → error(div by zero) 99`
   (caught, reified, execution continues). A macro **expander** must
   propagate errors loudly — do not route expansion through `do`'s
   catch-and-reify.
7. **Closure capture already pins free names to a definition env.**
   `ComputeCaptures` / `FnDefInfo.Captured` snapshot enclosing-scope
   bindings at construction. This is the exact tool for macro
   referential transparency (problem #2).
8. **`DefCleanup` / `__pa` already scope-and-undef body locals** at fn
   exit. This is the exact tool for containing introduced bindings under
   boru's leaky block scope.
9. **Single-pass, left-to-right.** The parser does one walk (no rewrite
   passes); compilation to IR (when it lands) is a separate later stage.
   Left-to-right gives **define-before-use staging for free**.

---

## 2. Surface design

### 2.1 Defining a macro

```
def unless macro [[cond body] [
  quote [ if unquote cond [] unquote body ]
]]
```

`macro` is `fn`'s expand-time sibling. It produces a `Function` value
flagged `Macro: true` with two behavioural differences baked in:

- **All params are implicitly raw-capture** (§2.3): operands arrive as
  unevaluated token forms.
- **The body runs at expansion time** and its returned token list is
  **spliced** into the call site (via `__SP`), rather than becoming a
  value on the stack.

`def name macro [...]` binds it like any other word; the stepper consults
a macro flag on the resolved `FnDefInfo` (§3) before normal dispatch.

### 2.2 The template

The template is an ordinary `quote [ … ]` region — **default-data**, the
polarity flip from normal boru (default-eval). Inside it:

- bare tokens → literal code of the expansion;
- `unquote x` → evaluate `x` in macro scope, insert the value as **one
  grouped node** (a compound form comes back wrapped as a paren group, so
  the generated word collects it as a single forward arg);
- `splice xs` → evaluate `xs` to a list, insert its **elements**
  flattened into the surrounding sequence.

`splice` is an alias for the existing `word`/`__SP`. `unquote` is the
`(expr)`-collapse given a readable name (we prefer explicit words over
overloading "paren inside quote means evaluate", consistent with boru's
explicit-words ethos — `usurp`/`stack-args` precedent).

**Optional sugar (Phase 2+):** a `` `[ … ] `` quasiquote form reusing the
`InterpString` lexer machinery but emitting a `[]Value` token list instead
of a String (see LISP-ANALYSIS §8 #2). `unquote`/`splice` remain the
escapes. Deferred — the word form ships first and proves the model.

### 2.3 Raw-form capture (the one new input primitive)

Macro params need to grab the next forward *form* — an atom, a literal, a
`(…)` paren group, or a `[…]` list — as **raw tokens**, with no eval, no
word→atom coercion driven by `/q`, and crucially **no `preEvalParens`**.

Implementation: a new `Signature` capture mode (working name `FormArgs
map[int]bool`, parallel to `NoEvalArgs`/`QuoteArgs`) that, during forward
collection, captures the next bounded form as a token-list value and
suppresses `preEvalParens` for that position. `macro` sets it on every
param. This is the interpreter analogue of Lisp's reader handing the macro
unevaluated s-expressions.

> Note the marker-tape wrinkle (LISP-ANALYSIS §2): a paren group on the
> tape is `OpenParen … CloseParen`, not a nested sub-list. Raw capture
> must grab the **whole balanced span** as one form. This is the same
> non-uniformity `WalkBodyWords` already copes with; the macro capture
> helper should reuse / extend that span-walking logic rather than
> re-deriving it.

---

## 3. Interpreter-mode execution

This is the primary target and works today's model with no phase split.
A macro is a runtime fexpr whose action is "splice code."

Walk for `unless (x gt 10) [print "small"]` (idiomatic version, `def x 5`):

```
tape:  ▸unless ( x gt 10 ) [print "small"]
```

1. **`stepWord` reaches `unless`, sees `Macro: true`** → macro path.
2. **Raw-capture** the forward args (no eval, no `preEvalParens`):
   `cond ← (x gt 10)` tokens, `body ← [print "small"]`. Bind in macro
   scope.
3. **Run the template body**; the expander walks
   `[ if unquote cond [] unquote body ]`:
   - `if` → literal; `unquote cond` → `( x gt 10 )` (grouped);
     `[]` → literal; `unquote body` → `[print "small"]`.
   - Expansion: `if ( x gt 10 ) [] [print "small"]`.
4. **Return as `__SP`**; the splice replaces the `unless …args` span ahead
   of the pointer and rewinds the cursor:
   ```
   tape:  ▸if ( x gt 10 ) [] [print "small"]
   ```
5. **Normal stepping resumes.** `if` is a normal forward word, so
   `preEvalParens` now collapses `( x gt 10 ) → false`; `if` runs the else
   block → prints `small`. Evaluation of the condition happened in the
   *generated* code, exactly as intended.

### 3.1 Expansion caching

Naively, a macro inside a loop re-expands every iteration. Cache the
expansion keyed on **call-site token identity/position** (the macro word's
source `Pos`), so it expands once and re-splices the cached token list.
This is the interpreter's miniature of "compile once," and the seam where
interpreter and compiled modes converge. Invalidate on redefinition of the
macro.

---

## 4. Compiled-mode integration (when the IR backend lands)

A strict stack-machine IR is frozen — it cannot splice ahead of the
program counter without embedding a runtime compiler. So expansion **must
move to compile time**; the design is unchanged, only the *timing* and the
*memoization* move:

- **Compile pass** walks tokens; on a macro-tagged word it **runs the
  expander now**, takes the returned tokens, and **continues compiling them
  in place**. The macro's output re-enters the compiler and is lowered like
  hand-written code.
- **Run pass** executes IR containing **no macros and no `__SP`** — they
  were expanded away.
- **Staging.** The compiler must be able to *execute* boru to run
  transformers (the evaluator is present at compile time). Macros are
  **define-before-use**: a macro must be compiled and runnable before the
  compiler reaches its first use. Single-pass left-to-right makes this
  natural; forward references to macros are an error (unlike runtime fns,
  which resolve via the registry at call time). Lisp's `eval-when` is the
  reference point.

The template, `unquote`, `splice`, and hygiene are **mode-agnostic**. Only
when-it-runs (step time vs compile time) and re-step-vs-lower differ.

---

## 5. Hygiene

The naive design is **fully unhygienic**, and boru's leaky block scope (fact
5) makes capture sharper than in Lisp — a bare `def` in an expansion
pollutes the caller. Staged plan:

### 5.1 Phase A — `gensym` (manual, Common-Lisp style)

A `gensym` word minting a fresh never-colliding atom (registry counter →
`tmp$G<n>`). Capture-free temporaries today, even for hand-written `word`
macros. The `myor` capture bug and its fix:

```
# BUGGY (introduced `tmp` captures user's `tmp`):
def myor macro [[a b] [ quote [ def tmp unquote a  if tmp [tmp] [unquote b] ] ]]
def tmp 42
myor false tmp        # → false   (WRONG: user's tmp captured)

# FIXED with gensym:
def myor macro [[a b] [
  def g (gensym)
  quote [ def unquote g unquote a  if unquote g [unquote g] [unquote b] ]
]]
myor false tmp        # → 42      (user-origin `tmp` untouched)
```

`gensym` is standalone and tiny — ship it first (LISP-ANALYSIS roadmap #1,
moves hygiene D→C).

### 5.2 Phase B — automatic hygiene (Scheme-style)

Two classic problems, two existing boru tools, with provenance free from
the syntax:

- **Provenance for free.** Tokens *outside* `unquote`/`splice` are
  template-origin; tokens *inside* are user-origin. The expander already
  has this boundary — it's the marking Scheme has to synthesize.
- **#1 introduced-name capture → auto-rename + teardown.** Rename every
  **template-origin binder** to a fresh name (same effect as `gensym`,
  author writes nothing); leave user-origin tokens alone. Because blocks
  leak (fact 5), also **wrap introduced binders in `DefCleanup` teardown**
  (fact 8) so renamed temps vanish at expansion end — the analogue of
  Scheme expanding into a `let`.
- **#2 referential transparency → capture-pin.** Pin **template-origin
  free words** (`if`, `not`, …) to their meaning in the macro's
  *definition* environment via `ComputeCaptures` (fact 7), so a use-site
  `def if …` can't hijack them. User-origin words stay dynamically
  resolved, as intended.

This is `syntax-rules`-grade hygiene built from `ComputeCaptures` +
`DefCleanup` + the `unquote`/`splice` provenance — **less new machinery
than Scheme needed**, because the parts Scheme invents already exist under
other names.

---

## 6. Build order

| Phase | Deliverable | Effort | Depends on | Status |
|---|---|---|---|---|
| 0 | `gensym` word | S | — | **LANDED** (1a) |
| 1 | `macro` definer + `FormArgs` raw capture + `unquote`/`splice` + interpreter expansion + expansion cache | M | 0 | **LANDED** (1a–1e; `legacy/MACROS-PHASE1.10.ignore`) |
| 2 | `macroexpand` introspection (recursive / macroexpand-all); loud expansion-error surface | S–M | 1 | **LANDED** |
| 3 | `` `[ … ] `` quasiquote sugar (InterpString-reuse) | M | 1 | **DEFERRED** |
| 4 | automatic hygiene — auto-rename template-origin binders (#1) | L | 1 | **LANDED** (#1; #2 free-word capture-pin + teardown deferred — refinements) |
| 5 | compiled-mode expander (staging) | L | 1; IR backend | **interpreter staging LANDED + tested; compiled-mode BLOCKED on the IR backend** (`legacy/MACROS-PHASE5.5.ignore`) |

**Smallest shippable slice:** Phase 0 + Phase 1 — `gensym` plus an
unhygienic-but-real `macro`. That alone moves all metaprogramming that
needs unevaluated operands **out of Go and into boru**, the single biggest
"become more of a LISP" step.

---

## 7. Touchpoints

- **`eng/go/value.go` / `FnDefInfo`** — add `Macro bool`; `Signature` gains
  `FormArgs map[int]bool`.
- **`eng/go/engine.go::stepWord`** — macro branch: raw-capture args, run
  template, splice result; expansion cache keyed on `Pos`.
- **forward collection / `matchSignature`** — honor `FormArgs`: capture a
  balanced form, suppress `preEvalParens` for that position.
- **expander (new, e.g. `eng/go/macro_expand.go`)** — walk a template
  resolving `unquote`/`splice`; later, hygiene rename + capture-pin. Reuse
  `WalkBodyWords` span logic for the marker tape.
- **`__SP` / `NewSplice`** — output path (unchanged; `splice` aliases it).
- **`lang/go/native/native_control.go` (or a new `native_macro.go`)** —
  register `macro`, `unquote`, `splice` (alias `word`), `gensym`,
  `macroexpand`.
- **`ComputeCaptures` (`fn_capture.go`)** — reused for Phase 4 free-word
  pinning.
- **`DefCleanup` / `__pa`** — reused for Phase 4 introduced-binder teardown.
- **Compiled backend** (Phase 5) — expander invoked in the compile walk;
  staging / define-before-use enforcement.

---

## 8. Test plan

Pair every positive with a negative (per repo test discipline). TSV spec
`lang/spec/macro.tsv` plus Go tests:

- **Expansion correctness:** `unless` (idiomatic + faithful `do`/`splice`
  variant); `macroexpand` shows the expected token list.
- **`unquote` vs `splice`:** grouped-node vs flattened; negative —
  `splice` of a compound condition mis-dispatches (proves the distinction
  is load-bearing).
- **Raw capture:** args arrive unevaluated; a paren'd arg is **not**
  `preEvalParens`-collapsed at capture but **is** in the expansion.
- **`gensym`:** uniqueness across calls; the `myor` capture bug
  (unhygienic) and its `gensym` fix (both as rows — negative + positive).
- **Hygiene (Phase 4):** introduced binder doesn't capture a same-named
  user var; introduced binder torn down (not leaked); template free `if`
  survives a use-site `def if …`.
- **Errors are loud:** an error during expansion **propagates** (not
  swallowed like `do`); a macro forward-reference (define-after-use) errors
  in compiled mode.
- **Interaction:** macros nested in `NoEvalArgs` bodies (`if`/`for`);
  macro-producing-macro; expansion cache correctness under loops + macro
  redefinition.

---

## 9. Risks & open questions

1. **Marker-tape non-uniformity (LISP-ANALYSIS §2).** Raw-form capture and
   the expander must walk paren/dotted **markers**, not clean sub-lists.
   Mitigate by reusing `WalkBodyWords`; long-term, the "structural
   code↔list view" (LISP-ANALYSIS §8 #5) would make macros dramatically
   simpler and is the natural follow-on.
2. **Hygiene × leaky scope.** Renaming alone is insufficient; teardown is
   mandatory. Confirm `DefCleanup` can be injected around an arbitrary
   spliced span, not only fn bodies.
3. **`macro` vs `fn` dispatch.** The stepper must branch on `Macro` before
   normal forward collection (operands must arrive raw). Ensure module
   FnDef wrappers and the trivial-delegation short-circuit don't bypass the
   macro flag.
4. **Staging / bootstrapping (Phase 5).** Running transformers at compile
   time needs an evaluator present; define a clear `eval-when`-style stage
   boundary before the IR backend hardens.
5. **`eval`/`read` adjacency.** Macros that *return* code pair naturally
   with first-class `eval`/`read` (LISP-ANALYSIS §8 #4). Not a dependency,
   but co-designing avoids two incompatible "code value" notions.

---

## 10. Relationship to legacy/LISP-ANALYSIS.5.ignore

This plan implements that note's headline Tier-1 recommendation (§8 #1–3)
and its roadmap rows 1–3 + 9. The key refinement from subsequent design
work: the **`unquote`/`splice` boundary is reused as the hygiene
provenance marker**, which (combined with `ComputeCaptures` and
`DefCleanup`) lets boru reach automatic hygiene with less new machinery than
Scheme — directly advancing the analysis's closing claim that the macro
system is "assembling existing primitives rather than inventing new ones."
