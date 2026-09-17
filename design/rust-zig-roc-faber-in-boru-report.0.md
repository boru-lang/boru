# Rust, Zig, Roc, Faber — Language Lessons Considered Against boru

> **Superseded on the Roc axis (2026-08-18).** Roc replaced its effect
> system, its polymorphism system, its error type and its default
> execution lane after this report was written, so §1, §4 and the Roc
> column of §6.1 are stale in mechanism, not merely thin.
> **Read [`roc-in-boru-report.0.md`](roc-in-boru-report.0.md) instead**
> for Roc; its §8 lists 22 corrections to this document, boru-side ones
> included (`'cap_denied` is a phantom code, the `'atom` spellings are
> syntax errors, `Value` is 104 bytes not 72, `boru build` is not AOT,
> the spec is 7,421 rows not ~11K).
>
> The Rust, Zig and Faber **language comparisons** stand. Two claims inside
> them do not: §2.1's "`describe` … is generated from the live engine, so it
> cannot drift" and §5.1's "describe output *cannot drift* because it is the
> runtime" are true of signatures, precedence and dispatch order but **false
> of worked examples**, which are hand-authored or fall back to a Go-side
> heuristic — `boru describe add` ships a literal `;# ...` placeholder
> (new report §7.4).

## Scope

This report answers two questions asked of the boru project in July 2026:

1. Review Richard Feldman's article **"rust-to-zig"**
   (rtfeldman.com/rust-to-zig, the retrospective on rewriting the Roc
   compiler from Rust to Zig) and identify ideas boru can reuse and
   pitfalls boru can avoid.
2. Examine **Rust, Zig, Roc, and Faber** (faberlang.dev) *as languages*,
   in the same manner, compared to boru.

It is a point-in-time analysis report in the lineage of
`LISP-ANALYSIS.5.md`, `RACKET-ANALYSIS.5.md`,
`elixir-types-in-boru-report.10.md`, and
`fsharp-units-in-boru-report.0.md`. **No code changes** are proposed as
decided here; every "idea" is a candidate for its own design note. boru
claims below were checked against the tree at the time of writing
(cited files); Rust/Zig/Roc characterisations are from general
knowledge of those languages; Faber characterisations are sourced
solely from faberlang.dev (fetched 2026-07-17; the project is young —
v1.1.1, first appeared 2024 — so treat its claims as claims).

The consolidated recommendation and pitfall tables are in **§6**.


## 1. The article: Roc's Rust→Zig compiler rewrite

Feldman's post is a retrospective on rewriting the Roc compiler
(~300K lines) from Rust to Zig over 487 days. Headline reasons: Zig's
~35ms incremental rebuilds (vs 3.4s for Rust on comparable code), an
ecosystem that assumes granular allocators and struct-of-arrays
layouts, and a striking empirical retrospective: classifying thousands
of GitHub issues (with Claude) showed that in **both** the Rust and
Zig compilers, nearly all memory-corruption bugs were
**miscompilations** — the compiler emitting wrong code for user
programs — not compiler-implementation memory bugs. "Picking a
different row would have made no appreciable difference to the
project." The post also covers their zero-parse disk cache (32-bit
indices instead of pointers → cached artifacts reload at ~memcpy
speed), hot code loading, string-interpolation patterns in `match`,
deterministic cross-compilation, and the acknowledged costs of Zig:
pre-1.0 churn, no traits, no private fields.

boru is not choosing between Rust and Zig — it is a Go interpreter
whose value is semantics and tooling. The transferable content is
engineering strategy, and the degree of convergent evolution is
striking.

### 1.1 Where boru already validates the article

- **"Miscompilations are the real risk."** boru's own history says the
  same: `design/MISCOMPILE-HUNT-FINDINGS.0.md` records 23 confirmed
  `--compile != interpret` divergences — all logic errors (const-pool
  aliasing, typed-local stores dropping refinement tags, container
  fn-member auto-dispatch), zero memory bugs. And boru already runs the
  defence Roc's data implies: `make verify-bytecode` (whole-corpus
  differential over the ~11K-row `lang/spec/*.tsv`, combination
  matrix, alloc ceilings, `-race` gates, dual-build args-aliasing
  gate) and `make fuzz-bytecode` (seeded property fuzz of the
  compilable subset). Those differential gates are a stronger stance
  than Roc's. What sits beside them is not: `MarkUncompilable`
  silently re-runs a refused program on the interpreter, and that
  path is scaffolding absorbing a known defect, not architecture.
  Every refusal is an unimplemented or unproven case, owed a fix and
  tracked to closure; done is a language where all valid code
  compiles, with no exceptions.
- **Data-oriented design, independently arrived at.** Roc restructured
  around SoA and indices; boru did the same species of work in Go: the
  gap-buffer tape (`design/TAPE-DATA-STRUCTURE.10.md`; 166× at
  recursion depth 8000, contiguous cache-friendly regions, O(1)
  indexing) and the `Value` shrink 184→72 bytes
  (`design/INTERPRETER-SPEED-INVESTIGATION.10.md`). Layout work pays
  even under a GC.
- **Resource exhaustion as ordinary errors.** The article praises Zig
  treating allocation failure as a normal error. boru's step budget
  (`[boru/evaluation_limit]`) and tape ceiling
  (`[boru/tape_exhausted]`) are the same move: limits are named,
  catchable values, each naming the resource actually consumed.
- **The index-safety lesson, productized.** With u32 indices,
  "index into the wrong array" is the new use-after-free and the
  borrow checker gives zero help; Feldman wants distinct index
  *types*. That is boru's bare-`refine` newtype
  (`design/REFINE-NEWTYPE-VS-SUBSET.10.md`) — nominal, symmetric,
  required at every boundary.
- **Subtractive design.** ADR-002 (no implicit broadcasting), ADR-005
  (no panics), capability gating, immutable-by-default, macros built
  from `quote`/`unquote` rather than a second system: boru's ADRs are
  mostly subtractions, which the article argues is where valued
  properties come from.

### 1.2 Ideas worth reusing (article)

1. **Cache format follows data representation.** The planned module
   cache (`design/MODULE-CACHE.0.md`, analysis-only) and bytecode
   `Program`s are natural cache artifacts; content-addressed caching
   of compiled modules (resolved path + source hash) slots into the
   `InheritConfig` channel that doc describes. Honest caveat: `boru`
   startup is ~20ms — memcpy-grade loading is not the point at boru's
   scale; avoiding re-parse/re-check/re-run of unchanged modules is.
2. **Hot code loading is nearly free for boru — surface it.** Roc built
   a whole compiler architecture to get dev-time hot reload next to
   optimized prod builds. boru's interpreted/bytecode split *is* that
   split. `serve` and `exec` have no watch/reload today — but the CLI
   already has a watcher precedent in `boru model --watch`
   (`cmd/go/internal/model/model.go`), so a `boru serve --watch` (or
   per-request re-resolution of file modules in the exec service) is
   largely a reuse job and a cheap, visible win.
3. **String-interpolation patterns.** Roc's
   `"/users/${id}/${page}"` match patterns are a good language idea
   with an unusually clean boru landing: a template literal in *type
   position* as a refinement-type constructor — a subset type whose
   membership test also binds captures — composing with signature
   dispatch under the "membership is one question" doctrine
   (EXPLANATION.md), so route-style dispatch falls out of overload
   resolution rather than a new construct.
4. **LLM-classified issue retrospectives as recurring practice.** The
   article classified ~3K issues to ground its safety claim; boru
   already runs agent-driven hunts (the 30-agent miscompile sweep).
   Periodically classifying boru's own issue history by root-cause
   class would show where the *next* verify-gate belongs.
5. **The 35ms lesson, applied to boru's loop.** Go compiles fast; boru's
   analog is user-facing latency. `boru check`/LSP redo work per
   invocation; incremental checking keyed by module hash is the same
   "feedback loop is a feature" investment, prerequisite: the module
   cache.

### 1.3 Pitfalls to avoid (article)

- **Pre-1.0 churn is a tax on users** — the Roc team's loudest
  complaint about Zig. boru's ~11K-row executable spec is already a
  de-facto compatibility contract; make it explicit (spec rows are
  the stability surface; changing one requires a design note).
- **A cache is not a transparent optimization.** Roc could zero-parse
  cache because caching was semantically invisible. boru's module
  cache is *not* — it flips file modules to singleton semantics
  (MODULE-CACHE.0 §3). Decide semantics first, then format.
- **Don't fight yesterday's safety war.** The differentiating
  memory-safety class was small (their 83.6% analysis) and Go already
  covers it for boru. The defect class that actually bites is engine
  divergence and refinement/tag-dropping logic — keep investing in
  differential gates, not theoretical hardening.
- **Named index types inside the engine.** Several found miscompiles
  live where const-pool IDs, slot indices, and tape positions meet;
  if the VM uses bare `int` for these, distinct Go types
  (`type ConstID int32`, …) apply Feldman's own medicine at near-zero
  cost.
- **The rewrite lesson, inverted.** Roc rewrote because the
  architecture was wrong — after prototyping the fix in OCaml. boru's
  equivalent is the design-doc-first culture: settle the semantics on
  paper before an emitter ossifies them, and you avoid needing the
  487-day rewrite. A compiler that *refuses* a case rather than
  guessing at it is no part of that equivalent — a refusal has not
  handled the case, it has logged an unimplemented or unproven one,
  owed a fix and tracked to closure. Done is a language that compiles
  every valid program, with no exceptions.


## 2. Rust vs boru

### 2.1 Convergences

- Rust's newtype pattern (tuple structs) ↔ boru's bare `refine`;
  REFINE-NEWTYPE-VS-SUBSET cites Rust explicitly, and boru's version
  enforces one membership rule at parameters, returns, and `is`.
- Rust's integrated toolchain (cargo/rustfmt/clippy/rustdoc) ↔ boru's
  single binary (`fmt`, `check`, `lsp`, `describe`, …). boru's
  `describe` is stronger than rustdoc in one respect: output is
  generated from the live engine, so it cannot drift.
- Clippy's advisory-vs-error split ↔ `boru check`'s never-gating
  info-severity advisories (`forward_strands_operand`,
  `mixed_form_call` — `design/ERRORS.8.md`).

### 2.2 Ideas

- **Editions** — the standout. Rust ships breaking changes per-crate,
  opt-in, with ecosystem interop preserved. boru analog: stamp spec
  rows and modules (via `boru.json`) with a language edition and let
  `check`/`fmt` migrate. This is the *mechanism* behind the
  stability-contract idea in §1.3: the spec says what the contract
  is; editions say how it evolves without churning users.
- **Visible fallibility.** Rust's `?` marks every fallible call site.
  boru is the inversion — open errors propagate implicitly; `do […]`
  is the opt-out — fine for a scripting language, but nothing today
  tells you what a word *can* raise. Folded into the error-sets
  recommendation (§6, R1) where Zig and Roc converge on the same
  feature.

### 2.3 Pitfalls

- **The upfront-complexity cliff.** Rust makes you learn ownership
  before anything works. boru's one cliff candidate is forward
  collection (the `1 2 add 3 mul` stranding trap); the advisory net
  is the right defence — grow it rather than complicating the
  collection algorithm.
- **Two macro systems.** `macro_rules!` vs proc macros is two
  sub-languages. boru macros are ordinary boru at expansion time with
  hygiene built in (`design/MACROS.8.md`) — protect that unity.
- **Trait coherence (orphan rules).** Global open dispatch forces
  Rust's most confusing restrictions. Directly relevant to
  `design/OPEN-WORDS.0.md`: whatever lands there should keep
  ADR-001's no-shadowing rule as the coherence story, or boru inherits
  the orphan-rule problem.
- **Async coloring.** Rust split its ecosystem into sync/async
  functions. boru's `await`-over-lists model is colorless — keep the
  `PROCESSES.0.md` roadmap colorless too.


## 3. Zig vs boru

Philosophically the closest pair in this report, which the article
obscures by being about implementation choice rather than language
design.

### 3.1 Convergences

- **`comptime` rests on types being first-class values manipulated by
  ordinary code** — verbatim boru doctrine. Zig generics are functions
  returning types; boru's `Box of [Integer]` is ordinary execution
  interning a type (EXPLANATION.md, "Generics as memoised type
  construction").
- **Errors are plain coded values**, and even OOM is just an error ↔
  `Ideal/Error` with code atoms, where resource exhaustion is a named
  catchable value (`evaluation_limit`, `tape_exhausted`).
- **"No hidden allocations" ↔ "no hidden effects"** (capabilities +
  policy profiles; explicit allocator parameter ≈ embedder-provided
  `FileOps`).
- **Arbitrary-width integers (`u7`) rhyme with predicate
  refinements**: a two-bound subset type (`(Integer gte 0)` composed
  with an `lt 128` bound) expresses the *range-constraint intent* of
  `u7` as an Ada-style range subtype, checked at every membership
  boundary. It is not a substitute for the representation side —
  fixed width, storage layout, wrapping/overflow arithmetic — which
  is a different tool; that responsibility lives with the numeric
  tower (R11).
- `defer`/`errdefer` ↔ the `ensure`/`bracket` RFC
  (`design/RESOURCE-SAFETY.0.md`), whose named primary use case
  (undo-on-failure) *is* `errdefer`.

### 3.2 Ideas

- **Error sets — the strongest language-level idea in this report
  (R1).** A Zig function's error set is part of its type (declared or
  inferred). boru errors carry code atoms but nothing enumerates them:
  `describe add` lists signatures yet cannot say what codes a word
  can raise. Inferring per-word raise-sets and surfacing them in
  `describe` ("raises: `'io_error`, `'cap_denied`") and in `check`
  fits ERRORS.8's loud-failures culture exactly — and Roc arrives at
  the same feature from the inference side (§4.2).
- **`errdefer` as a distinct form.** Zig ships two cleanup forms
  because run-always and run-on-failure are different roles. When
  RESOURCE-SAFETY lands, make failure-only cleanup first-class rather
  than making users test the outcome inside one `ensure`.
- **Checked arithmetic as the only sane default.** Zig makes overflow
  loud and defined. boru's overflow Phase 0 (uniform checked erroring
  arithmetic, `design/INTEGER-OVERFLOW-STRATEGY.5.md`) matches;
  Phase 1 (arbitrary-precision `Integer`) would leapfrog Zig entirely
  and deserves priority — "scripting language with silent 64-bit
  edges" is a worse position than either Zig's or Python's.

### 3.3 Pitfalls

- **Lazy analysis hides bugs.** Zig compiles only referenced code, so
  errors sit in uncalled branches — the same shape as the miscompile
  hunt's lesson (the differential was blind off the spec corpus).
  Whole-program `check` and ADR-008 coverage are the right side of
  this; never trade them for check-time speed.
- **Strictness that fights the user.** Zig's hard errors on unused
  locals are widely resented. boru's info-severity, never-gating
  advisory convention is the right calibration: gate on wrongness,
  advise on smell.
- **No privacy.** Feldman missed Rust's private fields in Zig. boru
  modules have privacy by omission (only exports are visible), but
  Class/Record *fields* are all public — as the object system grows,
  keep that true deliberately rather than by default.


## 4. Roc vs boru

### 4.1 Convergences

- **Platform/application split ↔ capabilities + policy.** Roc's
  defining idea — pure application code, all effects provided by a
  host platform — is boru's capability system approached from the
  opposite pole: Roc makes the host mandatory; boru makes the host's
  gates optional (CLI all-on; wasm playground / LLM hosts opt off;
  `capabilities.FileOps` substitutes implementations).
- Roc's optionals-as-tag-unions ↔ `(Integer tor none)` disjunctions
  with the `none`/`None` absence story (REFERENCE.md
  "Disjunctions", "Absence").
- Abilities-with-deriving (Eq, Hash, Sort, Encode) ↔ Ideal
  func-fields (`Equal`, `Format`, `Unify` per type-kind) plus the
  universal total order — boru "derives" sortability/equality once,
  at the lattice level.
- **Neither is curried-by-default** — Roc rejected auto-currying for
  error-message quality; boru calls are driven by fixed-arity
  signatures (known arity is what makes forward collection
  decidable). boru does, however, *support* explicit currying:
  chained `=>` lambdas curry right-associatively (REFERENCE.md,
  the `make-adder` example), so this is a default-shape kinship,
  not a shared prohibition. (The curried-chain entry in
  MISCOMPILE-HUNT-FINDINGS §E is not a language-level rejection — the
  language takes such chains and the interpreter runs them. It is a
  *hole in the compiler*: those chains are refused and silently
  re-run on the interpreter, an open defect owed a fix, not a
  coverage boundary the design is entitled to draw.)
- Shared Elm-lineage "failures must be loud, diagnostics must
  explain" culture ↔ ERRORS.8, hint lines, `boru policy explain`
  blame chains.

### 4.2 Ideas

- **Formalize the platform contract (R2).** Roc platforms declare, in
  types, exactly what a program may do, and programs are checkable
  against it. boru has the pieces — capability flags, policy
  profiles, `check` — but no static composition:
  `boru check --platform <profile>` verifying a program touches only
  words/capabilities that embedding provides would turn runtime
  `'cap_denied` into a check-time answer. Faber's frame types (§5)
  independently point the same direction.
- **Automatic error-set accumulation.** Roc's open tag unions make a
  pipeline's error type the inferred union of its parts' errors,
  zero annotations. The inference half of R1: propagate raise-sets
  through composition so `check` reports "this program can raise
  {`'io_error`, `'bad_input`}".
- **Functional-but-in-place.** Roc's Perceus-style opportunistic
  mutation keeps pure semantics while mutating sole-owner values.
  boru's immutable Nodes pay real copy costs (visible throughout the
  interpreter-speed work). Go has no refcounts, so the lever is
  compiler-side uniqueness/escape analysis in the bytecode VM — a
  large, semantics-preserving optimization that must land behind the
  differential gate (the const-bake saga shows exactly why).

### 4.3 Pitfalls

- **The novelty budget.** Lambda sets — Roc's cleverest inference —
  forced the 487-day rewrite; Roc also redesigned its entire effect
  story (Task → purity inference) years in. The lesson is not "don't
  innovate" but that novel core semantics need an executable contract
  before they ossify. boru's novel core (forward collection,
  refinement dispatch) is spec-pinned — apply the same
  prototype-then-spec discipline to OPEN-WORDS and compiler-frontier
  features.
- **Speculative sugar churns.** Roc added backpassing, the ecosystem
  absorbed it, then it was removed. Sugar is cheap to add, expensive
  to retract; keep sugar proposals in `design/` until they earn spec
  rows.
- **Purity absolutism.** Roc programs can't do anything without a
  platform, which taxes casual scripting. boru's default-open CLI with
  opt-in tightening is the friendlier polarity for a query/scripting
  language — build R2 without ever requiring ceremony for
  `boru do "1 add 1"`.


## 5. Faber vs boru

**What Faber is** (per faberlang.dev, fetched 2026-07-17): a
package-oriented, statically typed language with a Latin behavioural
vocabulary (`functio`, `genus`, `si`, `iace`/`cape`), structural
glyphs (`←`, `→`, `∴`, `≡`, `∪`), type-first declarations
(`textus nomen`, type before name), and nullable unions
(`T ∪ nihil`). Its compiler (Radix, written in Rust) lowers through
HIR/MIR/AIR to multiple backends (Rust — "reviewable Rust" — plus
WASM, TypeScript, Go, WGSL per the features page). Its defining
architectural claim: *"meaning lives in a semantic core — the HIR —
rather than in any particular rendering"*; human-language surfaces
(Latin, Thai, Arabic, Chinese, Hindi, Vietnamese, …) and codegen
targets are both *renderings* of that core. It is explicitly
**agent-ready**: `llms.txt` / `llms-full.txt` indexes, a
`.well-known/agent-skills/` catalog, `faber explain <CODE>` for
diagnostics, a generated corpus (183 terms, 98 alias spellings), an
agent guide whose anti-patterns section forbids "invented" syntax,
capability-based dispatch (`ad`) with **frame types** defining I/O
boundaries to the host, tests alongside source
(`probandum`/`proba`/`adfirma`), and "nine design laws" governing
language decisions. Young project: v1.1.1, first appeared 2024.

### 5.1 Convergences

- **Agent-first self-documentation.** Faber's `llms.txt` + corpus +
  `faber explain` is the same bet boru made with `boru describe` /
  `boru help` generated from the live engine, the AGENTS.md router,
  and the vault's agent-delegation design (scoped capability tokens,
  the `mcp` broker). boru's variant is arguably deeper — describe
  output *cannot drift* because it is the runtime — but Faber's is
  more reachable (static, well-known URLs an agent can fetch without
  installing anything).
- **Semantic core vs renderings ↔ ADR-007.** "No secondary parsing;
  every boru structure is macro-constructable Node data" is the same
  doctrine: meaning lives in canonical Node data; source text, `fmt`
  canonical rendering, and bytecode are renderings of it. Faber
  pushes the doctrine further (multiple *human-language* surfaces);
  boru deliberately keeps one surface.
- **Design laws ↔ ADRs.** "Nine design laws govern every language
  decision" is boru's ADR discipline under another name.
- **Frame types ↔ capabilities/`FileOps`.** Typed I/O boundaries
  between program and host are boru's capability + embedder-interface
  layer; this is the third language in this report (after Roc's
  platforms) to converge on host-boundary formalization, reinforcing
  R2.
- **Multi-target parity is a differential-testing problem.** Faber
  renders one HIR to many backends. boru's version is narrower than
  it first looks: the wasm playground is the *same* Go engine
  compiled with `GOOS=js GOARCH=wasm` (`wpg/Makefile`), not an
  independent backend, and the TS engine's cross-engine differential
  (`test/go/engspec`) deliberately permits *gaps* — TS may raise
  `undefined_word` where Go succeeds — with a documented backlog of
  unported leaves (`design/TS-ENGINE-MICRON-PARITY.0.md`). So today
  the corpus is a parity oracle for the overlapping subset only.
  The direction — an executable corpus as the cross-backend oracle —
  is still the right one, and is exactly what Faber's docs don't
  claim to have; closing the permitted-gap backlog is the price of
  claiming it fully.

### 5.2 Ideas

- **`boru explain <code>` (R4).** Faber ships `faber explain SEM001`
  (as Rust ships `rustc --explain E0308`). boru errors already carry
  stable codes (`[boru/incomparable]`, `[boru/cap_denied]`, …) and
  `boru policy explain` already explains *policy* decisions — but
  nothing explains error codes. A small `explain` surface (or
  `describe boru/incomparable`) rendering cause, example, and fix
  guidance from the same live-engine registry would complete the
  loud-failures story cheaply.
- **A machine-readable docs surface (R5).** `boru describe --json`
  plus a generated `llms.txt`-style index (served by the registry
  and/or checked into `docs/`) would give agents Faber's
  fetch-without-install path while keeping boru's can't-drift
  property, since both render from the live registry.
- **An agent-skills manifest.** boru already has AGENTS.md for
  in-repo agents; a `.well-known/agent-skills/` catalog on the docs
  site (install, REPL basics, vault delegation, policy profiles) is
  the externally-reachable version of the same content.
- **"Reviewable output" as a stated property.** Faber markets
  compiling to *reviewable Rust* — auditability of generated code as
  a feature. boru's analogs (bytecode disassembly, emitter goldens)
  exist as test infrastructure; naming auditability as a user-facing
  property (e.g. a documented `--emit` disassembly surface) is
  cheap and squarely aimed at the same agent-trust niche.

### 5.3 Pitfalls / contrasts

- **Call-form flexibility vs agent predictability.** Faber's core
  agent bet is rigidity: one comment form, no invented syntax, one
  way to write a call. boru bets the opposite — `1 2 add`,
  `1 add 2`, `add 1 2` are all legal — and that flexibility is a
  genuine agent hazard (the stranding trap; the `mixed_form_call`
  advisory exists because humans hit it too). boru's mitigations are
  the right shape — ADR-004 canonical forward form, prescriptive
  AGENTS.md ("write the forward form first"), never-gating
  advisories — and the lesson from Faber is to keep documentation
  *prescriptive* about the canonical form and consider `fmt`
  normalizing toward it, rather than presenting all forms as equal.
- **Rendering multiplicity multiplies the contract.** Every extra
  surface (98 alias spellings, 7 locales) multiplies the spec, the
  docs, and the confusion space — the same cost axis as Roc's
  backpassing. boru's single-surface + canonical-`fmt` position is
  worth keeping deliberately.
- **Ecosystem-shaped claims from a young project.** Faber's most
  interesting claims (semantic-core rendering, multi-backend
  lowering) are architecture; its corpus is small and its parity
  story unstated. Watch it as a fellow traveller in the agent-ready
  niche, not as proven prior art.


## 6. Cross-language synthesis

### 6.1 Design axes

| Axis | Rust | Zig | Roc | Faber (per site) | boru |
|---|---|---|---|---|---|
| Polymorphism | traits (coherence rules) | none (hand vtables) | abilities + deriving | `implendum` interfaces | signature overloading on the lattice; Ideals as per-kind capability records |
| Errors | `Result` + `?` | coded values, typed error sets | open tag-union results | `iace`/`cape`, `T ∪ nihil` | coded `Error` values at `do[]` boundaries; no sets *yet* (R1) |
| Effects | ambient + `unsafe` | ambient, allocators explicit | platform-provided, pure app | `ad` dispatch + frame types | capability-gated, policy-scoped, host-substitutable |
| Metaprogramming | two macro systems | comptime, no macros | none (deliberate) | semantic staging | hygienic macros = ordinary boru at expansion time |
| Safety mechanism | borrow checker | runtime checks + explicitness | purity + RC | static types + reviewable output | immutability + sub-engine isolation + Go GC |
| Compat mechanism | editions | none pre-1.0 (churn) | none pre-1.0 (churn) | none stated (v1.1) | ~11K-row executable spec; no edition mechanism *yet* (R3) |
| Agent surface | rustdoc/`--explain` | docs | docs | llms.txt, skills, `explain`, corpus | `describe`/`help` from the live engine; AGENTS.md; vault/mcp (R4, R5) |

### 6.2 Consolidated recommendations

Ordered by leverage; "source" names the language(s) whose experience
motivates it. Each is a candidate for its own design note, not a
decision.

| # | Idea | Source | Landing zone | Effort |
|---|---|---|---|---|
| R1 | Inferred **error sets**: per-word raise-set inference surfaced in `describe` ("raises: …") and `check` ("can raise {…}") | Zig (declared sets) + Roc (inferred unions) + Rust (`?` visibility) | checker + describe registry | medium |
| R2 | **Checkable platform/embedding contract**: `boru check --platform <profile>` verifying a program against a host's capability + word surface | Roc (platforms) + Faber (frame types) | check + policy machinery | medium |
| R3 | **Editions** layered on the executable spec: per-module language edition in `boru.json`; spec rows stamped; `fmt`/`check` migrate | Rust (editions) + Zig/Roc (churn cost as cautionary) | spec + tooling | medium |
| R4 | **`boru explain <error-code>`** rendered from the live registry, completing `policy explain` | Faber (`explain SEM001`) + Rust (`--explain`) | cmd/go, small new surface | small |
| R5 | **Machine-readable docs surface**: `describe --json` + generated `llms.txt`-style index + agent-skills manifest | Faber | describe + docs/registry | small |
| R6 | **`boru serve --watch`** / per-request file-module re-resolution (dev hot reload; bytecode stays the prod lane) | Roc rewrite (hot loading) | cmd serve/exec, reusing the `boru model --watch` watcher | small |
| R7 | **String-template patterns** as capture-binding refinement types unified with dispatch | Roc (match interpolation) | types + matcher | large |
| R8 | **Content-addressed compiled-module cache** (explicitly gated on the MODULE-CACHE singleton-semantics decision) | article (zero-parse) | MODULE-CACHE.0 + bytecode | medium |
| R9 | **Failure-only cleanup** (`errdefer` role) made first-class in the `ensure`/`bracket` RFC | Zig | RESOURCE-SAFETY.0 | folds into RFC |
| R10 | **Uniqueness-based in-place updates** in the bytecode VM (FBIP), behind the differential gate | Roc (Perceus) | bytecode VM | large |
| R11 | **Arbitrary-precision Integer** (overflow Phase 1) prioritized | Zig contrast (defined loud overflow) | numeric tower | medium (already proposed) |
| R12 | **LLM-classified issue retrospectives** as a periodic practice steering verify-gate investment | article methodology | process | small |
| R13 | **Named Go index types** for const-pool/slot/tape indices in the VM | article (index hazard) | eng/go + lang/go internals | small |

### 6.3 Pitfall watchlist

| Pitfall | Source | boru exposure point |
|---|---|---|
| Global open dispatch → coherence/orphan pain | Rust | `design/OPEN-WORDS.0.md` — keep ADR-001 as the coherence rule |
| Function coloring splits the ecosystem | Rust async | keep `PROCESSES.0.md` colorless like `await` |
| A cache silently changes semantics | article | MODULE-CACHE singleton decision must precede R8 |
| Pre-1.0 churn taxes downstream users | Zig, Roc | R3; spec rows as the stability surface |
| Too-clever core inference ossifies then forces rewrite | Roc lambda sets | prototype-then-spec for novel semantics |
| Speculative sugar added-then-removed | Roc backpassing | ADR/design-note discipline for sugar |
| Hostile strictness (gating on smell) | Zig unused-locals | keep advisories info-severity, never gating |
| Unexercised paths hide bugs (lazy analysis / off-corpus blindness) | Zig + boru's own miscompile hunt | whole-program check, ADR-008, widen the differential corpus |
| Surface multiplicity multiplies the contract | Faber aliases/locales | keep one surface + canonical `fmt` |
| Flexibility as an agent hazard | Faber contrast | prescriptive docs for the canonical call form; consider `fmt` normalization |

### 6.4 Closing observation

boru's philosophical nearest neighbour is **Zig** (one language at both
stages, types as values, coded errors, explicit effects); its
architectural nearest neighbour is **Roc** (hosted effects, friendly
diagnostics, functional core); **Rust**'s transferable lessons are
mostly ecosystem mechanisms (editions above all); and **Faber** is the
first fellow traveller aimed at the same *agent-ready* niche boru
occupies — its ideas are mostly ones boru already holds in stronger
(live-engine) form, but its packaging of them (`llms.txt`, `explain`,
skills manifests) is worth adopting. The three recommendations
endorsed by more than one language independently — R1 (error sets:
Zig + Roc + Rust), R2 (host contracts: Roc + Faber), R3 (editions:
Rust, with Zig and Roc as the cautionary tales) — are where this
report's confidence is highest.
