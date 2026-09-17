# Open Words — scoped `def` extension of existing words

> **Revised (2026-07-23):** the SAFETY MODEL described in §2.3/§3.3 —
> locked-first match ordering plus the user-type/waiver admission
> rules — is superseded by **[OPEN-WORDS.1.md](OPEN-WORDS.1.md)**
> (ownership-anchored signatures, natural specificity ordering).
> The merge/scoping/transplant model in the rest of this note is
> unchanged and still current.

Status: **IMPLEMENTED (rev 1 model)**. Landed as described in §2, with
the open questions resolved to their leans; see "Implementation notes"
at the end for the decisions, the mechanism as built, and where each
piece lives. Pinned by `lang/spec/open-words.tsv` and
`lang/go/test/reserved_words_test.go`. §6 migration 1 is DONE — the
temporal add/sub overloads (and the duration/timezone/time-of-day
TYPES, renamed CalendarDuration / ClockDuration) moved to
boru:time-util via the transplant path; the MatrixUtil migration
remains follow-up work (host-registration route — see the
Implementation notes). Discussion artifact per the ADR rule (design
notes capture discovery; no ADR entry without explicit maintainer
instruction).

Rev 1 replaces rev 0's dedicated `extend`/`overload` word with plain
**`def` on an existing word**, scoped like every other `def`, plus an
**export-transplant** channel for modules. Rev 0's append-only global
model is condensed in Appendix A. The naming question rev 0 carried
(`extend` vs `extends` confusability) dissolves — no new word exists.

One sentence: `def add fn [[a:Matrix b:Matrix] [Matrix] […]]` merges
a signature into `add` **in the current scope** — function body,
module body, or top level — and a module makes its merged signatures
available to its importer by exporting the word normally, which the
import machinery recognises and transplants one level up.

## 1. Problem

(unchanged from rev 0)

boru's dispatch is type-directed and openly polymorphic — `add`
already covers numeric addition, string concatenation, Bytes
concatenation, and Date/Duration arithmetic through one signature
list. But the *right to contribute to that list* is closed: only Go
code in `lang/go/native`, at registry build time, can append
signatures (`RegisterNativeFunc` — how `native_bytes.go` adds the
Bytes `add` overload and `native_math.go:130-158` carries the
temporal ones). Nothing at the boru level can:

- `def add …` raises `[boru/reserved_word]` — built-in words cannot
  be redefined, and there is no separate append path.
- `boru:matrix-util` — a first-party module — could not give its own
  flagship type addition: `(matrix) add (matrix)` is a
  `signature_error`, and the module ships **`mat-add`** instead. The
  `mat-` prefix is the workaround made visible.
- `design/BEHAVIORS.10.md` §"Single dispatch on the LCA" already
  concedes the gap, naming this exact example: cross-type addition
  (Date + CalendarDuration) "would need either a multimethod-style
  extension or the user attaching the impl to the LCA themselves."

The temporal overloads on core `add`/`sub` live in `native_math.go`
today **because there is nowhere else to put them** — the types must
be global (ordering, equality, wire-stable FixedIDs, cross-module
producers), but the signatures are a time-util concern stranded in
core by the missing mechanism.

## 2. The model

### 2.1 `def <word> <fn>` — merge, not replace

`def` on a name that already resolves to a word takes the fn's
signatures and **merges** them into the word's signature list:

- a signature whose argument-type tuple **exactly matches** an
  existing *unlocked* signature **replaces** it (in place, keeping
  its match-order position);
- a signature matching a **locked** signature's tuple is an error —
  locked signatures can never be replaced;
- any other signature **appends** (after the existing list).

The result is not an in-place mutation of the word: `def` constructs
a **word clone** — the base word's full signature list plus the
merge — and binds it through the ordinary `DefTable` shadow stack.
Everything else about `def` then applies unchanged: innermost
binding wins, `undef` pops back to the previous state, sub-engines
inherit the binding.

### 2.2 Scopes — one mechanism, three ranges

Because the clone rides the normal `def` machinery, the scoping the
proposal asks for falls out of machinery that already exists:

- **Inside a function body**: a body-local binding, torn down by the
  existing `DefCleanup` tail at fn exit — literally "clone the word,
  add the sig, fully undef the clone at exit". No new lifecycle.
- **Inside a module body**: a binding in the module's sub-registry
  `DefTable` — module-private by default (confirmed: module-body
  defs are invisible to importers). The module's own exported fns
  keep seeing the extension forever, because exported wrappers close
  over the sub-registry (`Registry: subReg`) — which is exactly what
  lets an exported word use module-private helpers today.
- **Top level**: a binding in the program's root registry — visible
  to the rest of the program and every sub-engine (`do`, `each`,
  `await`), like any top-level `def`.

### 2.3 Locked signatures and sealed words (host-only)

Every natively registered signature carries `Locked`. Locked
signatures can never be replaced or removed, and they keep **first
position in match order** (see §4.2 for what that buys). Locking is
not a boru language ability — it is a property of the Go
registration layer, like capability flags.

A second, stronger tier: a small set of **sealed words** cannot be
def-merged *at all*, because the engine special-cases them **by
name** and a shadow would break the identity the kernel relies on.
The known members today: `def` itself (`engine.go::bindsReferent` —
"def is frozen (reserved_word), so the name is a reliable identity")
and `make`; the literals `true`/`false`/`none` are name-cased too
but are not words a `def` could target. Sealing replaces today's
blanket `reserved_word` guard (`native_definition.go`), which
currently protects every builtin: the guard *relaxes* to "sealed
words only + locked-signature rules" rather than disappearing.

### 2.4 Export transplant — one level, opt-in transitivity

A module makes its merged signatures available to importers by
exporting the word **normally** (the word-clone value in an export
map). At import, the machinery recognises the export as a word
extension — the clone carries provenance (base word name + which
signatures were added), detected via a named-helper protocol like
`IsRefinePrefab`, never by field probes — and, when the base name
resolves in the **importing registry**, installs the merge there as
an implicit top-level `def` of that word.

Properties that fall out:

- **One level only.** The transplant lands in the importing registry
  and stops. If module A imports B (receiving B's transplants in A's
  sub-registry), importing A does **not** carry B's extensions —
  unless A itself exports the word, which re-transplants everything
  visible on A's clone (A takes ownership). Transitivity is opt-in
  by re-export, never ambient.
- **The firewall idiom.** An importer that wants a module's exports
  *without* its word extensions wraps the import in a literal
  module:

  ```
  import module [
    import "./foo.boru"
    export "Foo" { …just the things wanted… }
  ]
  ```

  The inner import transplants into the *inline module's*
  sub-registry; the wrapper exports no words; nothing reaches the
  real program. The firewall is not a new feature — it is the
  one-level rule composed with inline modules.
- **Consent replaces the orphan rule.** rev 0 needed an
  orphan-instance advisory because contributions installed
  themselves. Here nothing crosses a module boundary without an
  explicit `export` on one side and an explicit `import` on the
  other — importing a module *is* consenting to its word
  extensions, and the firewall is the selective opt-out. The
  advisory is dropped.

## 3. Review — plausibility

The mechanics are stronger than rev 0's, because every piece maps to
machinery that already exists and is already tested:

1. **Scoping is free.** Body-local defs with cleanup tails, module
   sub-registry privacy, top-level bindings, `undef` unwinding,
   sub-engine inheritance — all existing `DefTable` behaviour. rev 0
   had to invent a lifecycle; rev 1 inherits one.
2. **Module-closure execution is free.** A transplanted signature's
   handler is a boru fn closed over the module sub-registry — the
   exact shape of today's exported FnDef wrappers, so module-private
   helpers work with no new dispatch path.
3. **The locked-first ordering theorem.** With locked signatures
   pinned to the front of match order, an unlocked addition can
   never pre-empt a locked match — so **no previously-valid call
   changes its dispatch**, even when the new signature's tuple
   overlaps a locked one. rev 0 needed a universal unify-non-overlap
   check to get this; rev 1 gets the dispatch half by ordering
   alone, and only needs conflict rules among *unlocked* signatures.
4. **The firewall composes from existing parts** — no new syntax.
5. **`undef` works.** Because a transplant is an implicit `def`,
   `undef add` at the importer pops the imported extension — rev 0's
   in-place mutation had no retraction story at all.

## 4. Review — gaps and sharp edges

Ordered by how much they bite.

### 4.1 Merge-vs-redefine for plain user words (REPL hazard)

Today `def f fn […]` twice **replaces f wholly** — the standard
REPL/iterate idiom. Under merge semantics, redefining `f` with a
*changed parameter tuple* would **append**, leaving the stale
signature live: `def f fn [[a:Integer] …]`, then `def f fn
[[a:String] …]` — the Integer overload silently survives, and old
call sites keep working when the author believes they replaced the
fn. That is a real usability regression for interactive work.

Recommendation: merge semantics trigger only when the target word
**carries at least one locked signature** (i.e. natives — the words
that actually need extension); plain user fns keep today's
whole-replacement shadowing. A user word could opt into mergeability
later if a need appears. OPEN: whether module-provided words (FnDef
wrappers, which delegate to natives with locked sigs) should count —
lean yes, they carry locked sigs transitively.

### 4.2 Forward collection can shift inside the extending scope

The locked-first theorem covers **dispatch**, not **collection**. A
new signature widens what the word can forward-collect, so a
previously-valid line can *parse differently* inside the scope:
`add 1 true 2` — today `true` stops collection (no signature takes
it); with a fn-scoped `[Integer Boolean]` merge, `true` is
collected. This is inherent to type-directed collection and arguably
the intent ("in my scope, add takes these shapes") — but it must be
documented as a property, and it is the strongest argument for the
scoping being *narrow by default* (fn > module > top level), which
the model already provides. The `boru check` advisory machinery
(`forward_strands_operand` precedent) can flag lines whose collection
differs from the base word's.

### 4.3 Closure capture leaks the scope (decide, don't discover)

Body-local defs are **captured** by fns constructed inside the body
(`FnDefInfo.Captured` — existing rule). A lambda built inside a
fn-scoped extension therefore carries the extended `add` after the
scope exits, so "applies only inside the function" is softened by
closures. This is consistent — it is the same reasoning that lets a
module's exported fns keep the module's extensions — and the
recommendation is to **allow it and document it** as the closure
rule applied uniformly. The alternative (excluding word-clones from
capture) would make a lambda behave differently inside vs outside
its constructing scope, which is worse. OPEN only if the capture
list's shallow-snapshot semantics interact badly with very large
clones (see 4.8).

### 4.4 Transplant collisions — last-wins or loud?

A imports B and C; both export an extension of `add` with the same
exact unlocked tuple. Pure def-stack semantics say the later import
shadows (innermost/latest wins) — consistent, unwindable via
`undef`, but *silent spooky action between two files that never
mention each other*. Recommendation: **loud `[boru/extend_conflict]`
at the second transplant** when the same tuple arrives from a
different module than the one that installed it; identical
provenance (diamond re-import) is idempotent and quiet.
Direct user `def` at top level still shadows freely — the error is
for module-vs-module collisions only, where no human is standing at
the point of conflict. OPEN: whether a check-mode advisory +
last-wins would suffice instead.

### 4.5 Sealed-word inventory

§2.3's sealed set (`def`, `make`) was found by grepping the engine's
name-special-cases — the real inventory needs auditing across every
kernel file (splice/`word`, `quote`, `end` are lexical/marker-level
and may not need sealing; `bindsReferent` and friends do). The
relaxation of the blanket `reserved_word` guard must land **after**
that audit, with a spec row per sealed word pinning the refusal.

### 4.6 Clone fidelity is all-or-nothing

A word clone must carry the *complete* per-signature dispatch
metadata — `BarrierPos`, `QuoteArgs`, `NoEvalArgs`, `RawParens`,
`FormArgs`, handler pointers for locked sigs (delegating to the
native, module-wrapper style). Any field dropped in the copy is a
behavioural fork between base word and clone that only surfaces at
the call shapes that read that field. The `isTrivialDelegationBody`
short-circuit and `matchSignature` need to treat the clone
identically to the base for the locked subset — pin with
before/after byte-identical spec rows on every call form.

### 4.7 Checker and bytecode compiler

The checker is registry-driven and follows `def` scoping already, so
scope-varying signatures work in principle; the diagnostics
(`extend_conflict`, the 4.2 advisory) are new. The bytecode
compiler's fold sites for foldable words must consult the **scoped
binding** rather than any baked signature table; a merged clone whose
added sig the compiler cannot yet lower is an **open defect** — the
compiler owes that case a lowering, and the gap is tracked to closure
rather than designed around. Until it is closed the fold site
declines and the fallback-island machinery silently routes the call
to the interpreter, which hides the defect instead of answering it.
Making every merged clone compile is the one real implementation cost
outside the registry (same as rev 0).

### 4.8 Cost

The clone is built once at `def` time (it is a value); the per-call
cost inside fn bodies is the binding push/pop the cleanup machinery
already pays for body-local defs. A `def add …` executed on every
call of a hot fn re-*constructs* the clone per call, though —
memoise the constructed clone on the definition site (the fn body
token), or hoist the recommendation: extend at module/top level,
bind results in fns.

### 4.9 Recognition rule at export

Transplant triggers when (a) the exported value is a word-extension
clone (provenance marker), and (b) its base name resolves to a word
in the importing registry. If the base name is absent in the
importer (extension of a module word the importer never imported),
the export degrades to a plain namespaced binding (`Foo.add`) — no
error, no transplant. The namespaced binding arguably should exist
in *all* cases alongside the transplant (harmless, and lets an
importer call `Foo.add` explicitly); lean yes.

## 5. Prerequisites

### 5.1 Locked flag + sealed set

Host-side only; §2.3. Relaxing `reservedWordError` is localized
(`lang/go/native/native_definition.go:265`).

### 5.2 Per-module nominal identity — mintID collision (FIXED)

The conflict analysis assumes one module's minted `Foo` is never
another module's `Foo`. That held in doctrine but was broken in
implementation — `TypeTable.mintID` used a strictly per-table
counter, so the Nth mint in any two sibling registries got the same
ID, making a `refine Integer` from one inline module teq-identical
to a `refine String` from another (and to the first top-level mint
after the import), with `is` accepting values across the boundary.

**Fixed alongside this note:** the mint counter is shared **per
registry tree** — module sub-registries adopt the importing tree's
counter (`TypeTable.AdoptSeqFrom`, wired in `RunModuleBody` and
`BuildIOModule` for its StreamKind mint), concurrent forks share it
(`CloneDynamic`), rollback sandboxes copy it (`Clone`) so discarded
mints don't shift later IDs (which keeps a check-mode pass and a
plain run minting identical IDs — the type-soundness ratchet
compares the two by identity). Deliberately per-tree rather than
process-global so dynamic IDs stay a deterministic function of the
program. Pinned in `eng/go/mintid_test.go` and
`lang/spec/module-instance.tsv` §7. Known residual: two *unrelated*
engines in one process can still mint colliding IDs; hosts
exchanging Values across engines is out of scope.

## 6. Migration candidates

Once landed, in order of payoff:

1. **Temporal `add`/`sub` overloads** (`native_math.go:130-158`) →
   `def add fn …` merges in `boru:time-util`'s body + `export` of
   `add`/`sub`. The Time *types* stay globally registered; `boru:io`
   imports time-util so mtime arithmetic keeps working.
2. **`MatrixUtil.mat-add`/`mat-mul`/`mat-emul`** → merged `add`/
   `mul` signatures on `[Matrix Matrix]` (keep `mat-*` as deprecated
   aliases one release).
3. **Future micro types** (`Scalar/Micro/*`): `add [Money Money]`
   etc., merged and exported by their defining modules from day one.

## 7. Spec obligations (when implemented)

Per the paired-negative discipline:

- Merge at each scope: works inside; **invisible outside** (fn exit,
  module boundary, engine without the import) — negative row each.
- Locked-tuple replacement → error row; sealed word (`def def …`,
  `def make …`) → error row.
- Locked-first: a previously-valid call form byte-identical
  before/after a merge whose tuple overlaps a locked sig.
- Replacement of an *unlocked* tuple: in-scope replaced, `undef`
  restores.
- Export transplant: importer's bare word gains the sig; a *second*
  engine without the import errors identically to today; transitive
  case (A imports B; importing A does NOT carry B's) — negative row;
  re-export case — positive row; firewall idiom row.
- Transplant collision from two modules → `ERROR:extend_conflict`
  (pending 4.4 decision).
- Closure capture: lambda constructed inside a fn-scoped merge keeps
  it after exit (pending 4.3 decision) — pin whichever way decided.
- REPL redefinition: `def f` twice on a plain user fn still replaces
  wholly (pending 4.1 decision).

## 8. Open questions

1. 4.1 — merge-trigger rule (lean: locked-sig-bearing words only).
2. 4.3 — closure capture of word-clones (lean: allow, uniform rule).
3. 4.4 — transplant collision policy (lean: loud error across
   modules, silent shadowing for direct user defs).
4. Does `undef` on a transplanted word need to distinguish "pop my
   local def" from "pop the import's transplant"? (Stack order says
   no — it unwinds in reverse install order — but the UX of
   undef-ing an import's contribution deserves a look.)
5. Should the namespaced form (`Foo.add`) exist alongside the
   transplant (§4.9)? Lean yes.

## Implementation notes (as landed)

The mechanism turned out lighter than §2 assumed, because the kernel
already had the load-bearing pieces: natives and user defs share ONE
binding store (`Registry.Defs`), and `Registry.Lookup` already unions
signatures across a name's def stack (`aggregateDispatch`). So a "word
clone" is an ordinary def-stack entry:

- **`Signature.Locked`** (`eng/go/value.go`) — stamped on every sig in
  `Registry.Register` (the native/host path; user `def`s never reach
  it). `CompareSignatures` sorts locked strictly first — the §3.3
  theorem holds by ordering alone, across arity too (a longer unlocked
  tuple never pre-empts a shorter locked prefix match). Locked entries
  are also exempt from `InstallDef`'s overlap-drop and from targeted
  `undef … fnsig` removal.
- **`FnDefInfo.Extends`** — the clone's provenance marker (base word
  name), detected only via `IsWordExtension`. `Lookup` stops
  aggregating at a clone (it carries the full base list), which is what
  makes unlocked-tuple replacement effective and `undef` restore the
  exact previous state. `aggregateDispatch` propagates the marker so a
  `name/r` reference stays recognisable at export.
- **`eng/go/word_extend.go`** — sealed set + `InstallWordExtension`
  (the def-merge; compiles added sigs through the same
  `compileFnSigs` pipeline `InstallFnDef` uses) +
  `TransplantExtension` (the import-side merge; `Signature.Origin`
  carries the module ref for the §4.4 conflict/idempotence rules).
- **lang wiring** — `defWordExtension` in
  `lang/go/native/native_definition.go` (merge trigger: target's
  aggregate has a locked sig AND the body is an fn — §4.1's lean,
  which includes module-wrapper rebindings); `undef` pops a clone but
  still refuses the bare native; `transplantWordExtensions` in
  `native_module_module.go` runs in every export-install path.

Resolved open questions: **4.1** locked-sig-bearing words only (module
wrappers count — their rebindings carry locked sigs); **4.3** closures
capture clones (uniform rule, pinned); **4.4** loud
`[boru/extend_conflict]` module-vs-module, silent shadowing for direct
user defs, diamond re-import idempotent by module-ref provenance
(inline modules get a per-instance `inline#<id>` origin, so two inline
modules collide loudly); **Q4** `undef` unwinds in reverse install
order, no import/local distinction; **Q5** the namespaced `Foo.add`
binding exists alongside the transplant.

**Module-scope user-type rule (added post-rev-1, maintainer
instruction).** A MODULE may extend a CORE word only with at least one
USER-MINTED argument type per signature — a type the module creates
with `refine` / `class` (`Origin == OriginUserDef`). Builtin types do
NOT qualify, neither the kernel ones (`add [Boolean Boolean]`,
`add [Integer Map]`) nor the external builtins the boru: modules and
host plugins register globally (`Date`, `Matrix`, `Fetch`, `Timeout`);
a builtin-only tuple raises `[boru/extend_user_type]`. Rationale: such
a tuple would change what core calls mean for every importer
(`add 1 {}` suddenly working because of an import) and breaks forward
compatibility the day core or a first-party module claims the tuple
as a locked signature. A type ALIAS (`def MyDate Date`) does not mint
and cannot launder a builtin past the rule; a refine of a builtin
(`def MyDate (refine Date)`) is a genuine user identity and
qualifies. Enforced at the module-body `def` (`Registry.ModuleScope`,
set by `RunModuleBody`) so the module author sees the refusal
immediately — even a module-PRIVATE builtin-only extension is
refused, since it breaks on the same future claim — and re-checked at
transplant as defence in depth. Top-level programs are unrestricted
(the author is standing at the point of change), and the rule does
not apply to extending module-provided words (wrapper rebindings),
which are versioned with the dependency that owns them.

Consequence for the §6 migrations: a migration qualifies for the
transplant path exactly when the module also takes ownership of the
TYPES. Both are DONE this way:

- Migration 1: TimeOfDay / Duration / CalendarDuration /
  ClockDuration / Timezone moved out of the global builtin table
  (former FixedIDs 1004-1008) into boru:time-util as per-import mints
  (`MintTemporalModuleTypes`, the StreamKind pattern), and the
  temporal add/sub overloads ride the module's exported word-extension
  clones (`TemporalArithmeticExtensions` + `NewWordExtension`). Only
  Date / DateTime / Instant (and the Scalar/Time root) remain core.
- Migration 2: the Tensor family (Tensor / Matrix / Vector, former
  FixedIDs 2000-2002) moved into boru:matrix-util the same way
  (`MintTensorTypes` + `TensorArithmeticExtensions`): import
  transplants [Matrix Matrix] overloads onto bare add / sub / mul,
  with mat-add / mat-sub / mat-mul kept as aliases backed by the same
  handlers. The Ideal-kind surface is namespaced now — `make
  MatrixUtil.Matrix [[1 2][3 4]]`, `refine MatrixUtil.Matrix {rows:R
  cols:C}` — since the bare names left the builtin index with the
  types.

In both cases the minted types are what satisfy the user-type rule. A
future migration whose types must STAY global builtins would instead
go through the host registration layer (Go-side `Register`, locked
signatures) in the owning module's builder.

The remaining module-owned globals followed (no word extensions
needed — pure type moves): the Fetch family (3000-3002) →
boru:net (`MintFetchTypes`, exported as Net.Fetch / Net.Request /
Net.Response); Timeout / Interval (4000-4001) → boru:time-util
(joined `MintTemporalModuleTypes`, exported as TimeUtil.Timeout /
TimeUtil.Interval); and the three self-registered module carriers
MiniLangCompiled (5003) / ParseGrammar (5005) / Model (5006) →
per-import mints in their own builders. Still global by necessity:
Bytes and Node/Xml (parser-produced), Date / DateTime / Instant
(cross-module producers), Module / ModuleExport / KeyVal (core
machinery), and Patrun (its words — including a locked overload on
core add — are core vocabulary; moving it is a feature decision).

Two properties the batteries pin
(`lang/go/test/module_extend_test.go`, open-words.tsv §6–§7): file
modules re-RUN per import (no module cache), so a re-imported module
re-mints its user types — a diamond import appends a fresh-minted
tuple quietly, and two modules' same-shaped user-typed extensions
COEXIST (each anchored to its own mint, per-tree mintID §5.2) rather
than conflict. `[boru/extend_conflict]` is therefore unreachable from
boru source today; the guard is pinned at the API level
(TestModuleExtendTransplantConflictDirect) because it must hold for a
future module cache (shared mints across importers) and for
host-constructed clones.

§4.5 sealed inventory as audited: `def` (`bindsReferent`, forward-hint
and macro-expander name checks), `make` (`autoEvalMap` gating keys on
`match.Name`), `word` (splice-paren expansion in `carrier.go`).
`true`/`false`/`none`/`inf`/`nan` stay covered by `reservedLiterals`.

§4.6 clone fidelity is free: the clone copies whole `Signature`
structs, so `BarrierPos`/`QuoteArgs`/`NoEvalArgs`/`RawParens`/
`FormArgs`/handlers ride along by value.

§4.7 as observed: the checker follows scope through the ordinary
registry lookup (a merged call type-checks in scope, flags out of
scope); the bytecode recorder treats an added sig like any boru fn —
local merges compile with interpreter parity, but a transplanted
(foreign-registry) sig still REFUSES under `-force-compile` ("user fn
call"). That refusal is an **open defect**, not a settled outcome: a
transplanted signature is valid boru and is owed a lowering like any
other. Under `-compile` the refused program is silently re-run on the
interpreter — scaffolding that absorbs the defect and, by being
silent, conceals it. The §4.7 work item stands until the recorder
compiles a transplanted sig outright.

§4.8 stands as designed: the clone is rebuilt per `def` execution —
extend at module/top level rather than in a hot fn body.

## Appendix A — rev 0 (superseded): the `extend` word

rev 0 proposed a dedicated word (`extend`, alternates `overload` /
`augment`) with append-only semantics, a universal unify-non-overlap
check, registry-wide installation, contributions riding the
`ModuleDesc` and installing automatically at import, and an
orphan-rule advisory for contributions touching only shared types.
rev 1 subsumes it: `def` is the surface (no new word, dissolving the
`extend`/`extends` confusability question), scoping replaces
registry-wide installation, locked-first ordering replaces the
universal non-overlap check for the locked set, and explicit
export/import consent replaces the orphan advisory. The pieces of
rev 0 that survive unchanged: locked signatures (strengthened by the
sealed-word tier), the module sub-registry closure story, the
checker/compiler cost analysis, and the migration list.
