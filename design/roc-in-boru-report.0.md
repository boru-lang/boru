# Roc Considered Against boru — What to Learn, What to Adopt, What to Decline

## Scope

This report answers a question asked of the boru project in August 2026:
compare boru to **Roc** (roc-lang.org) and identify what boru can learn or
adopt.

It is a point-in-time analysis report in the lineage of
`LISP-ANALYSIS.5.md`, `RACKET-ANALYSIS.5.md`, `unison-in-boru-report.0.md`
and `verse-in-boru-report.0.md`. **No code changes are decided here**;
every recommendation is a candidate for its own design note.

It supersedes §4 of `rust-zig-roc-faber-in-boru-report.0.md` (July 2026)
on the Roc axis. That section is not merely thin — it is now **stale in
its mechanism**, because Roc replaced its effect system, its polymorphism
system, its error type and its default execution lane in the interval.
§8 lists the corrections one by one.

**Method and provenance.** The material was produced by an eight-axis
agent sweep, each axis independently re-checked by an adversarial verifier
against the tree and against Roc's primary sources; recommendations the
verifier could not sustain were dropped. Roc characterisations cite the
URL actually fetched (2026-08-18). boru characterisations cite a repo path
or a command. **The four defects in §7, and every boru count quoted in this
report, were re-reproduced by hand** against `cmd/go/bin/boru` built from
the working tree, with the commands shown — those are measurements, not
agent output. Everything else is an argued reading and should be read as
one.

A note on Roc's own currency: Roc's site is mid-rewrite and its
maintainers say so — the new-compiler mini-tutorial states that
"everything on roc-lang.org is referring to the old compiler and the old
design; we haven't updated any of it yet." Where the site and the
repository disagree, this report follows the repository.

**Primary sources** (fetched 2026-08-18; the Roc side of every claim below
traces to one of these):

| Source | Used for |
|---|---|
| `raw.githubusercontent.com/roc-lang/roc/main/docs/mini-tutorial-new-compiler.md` | The new-compiler syntax and semantics: `main!`/`|args|`, the `!` naming convention, `Try`/`Ok`/`Err`, `?`, `match`, `var $x`, `for … in`, `where` constraints, `expect`/`dbg`/`crash`, the app header, "sound, decidable, principal" inference, and the old→new table |
| `raw.githubusercontent.com/roc-lang/roc/main/README.md` | Status ("not ready for a 0.1 release yet"), the compiler rewrite, funding and community |
| `github.com/roc-lang/roc` — issue **#7458** ("Move to Static Dispatch in place of Abilities"), and `src/eval`, `llvm_compile/`, `machine_code_shim/` | The removal of abilities; the default execution lane |
| `roc-lang.org/fast` | Reference counting with opportunistic in-place mutation, platform-chosen allocators, the async-IO state machine, the sub-1s/sub-100ms build target "through caching" |
| `roc-lang.org/friendly` | Diagnostic style, zero-configuration `roc fmt`, `--allow-errors`, `expect`, the capitalisation convention |
| `roc-lang.org/functional` | Immutability, `var $x`, managed effects — **note this page still teaches the removed `Task` model**, and is cited here only as the cautionary staleness example |
| `roc-lang.org/different-names` | The stdlib naming policy (`fold`, `keep_if`, `drop_if`, `first`, `map2`, `?`) |
| Roc's FAQ | The permanent exclusion of refinement, higher-kinded and rank-N types, and the compile-time reasoning behind it |
| `rtfeldman.com` — the Rust→Zig retrospective | The rewrite rationale (already analysed in `rust-zig-roc-faber-in-boru-report.0.md` §1) |

`roc-lang.org/tutorial`, `/platforms` and `/plans` returned **404** on
2026-08-18, which is itself evidence for §3.8's staleness finding.


## 1. What Roc is now — the 2026 reset

Roc is a fast, friendly, functional language in the Elm/ML lineage, still
pre-0.1, run by a US 501(c)(3) foundation on donations. That much the
prior report had right. Almost everything below it changed:

| Was (prior report's Roc) | Is (2026) |
|---|---|
| Effects via `Task`, a monadic description composed then run by the platform | `Task` **removed**. Purity is **inferred** and carried by the type arrow — `->` pure, `=>` effectful. The `!` name suffix (`main!`, `echo!`) is a *naming lint*, not the mechanism. Effect polymorphism is excluded by design |
| Polymorphism via **abilities** with deriving | Abilities **removed** (roc-lang/roc#7458). Static dispatch with inline constraints: `stringify : a -> Str where [a.to_str : a -> Str]` |
| `Result` with `Ok`/`Err` | `Try(ok, err)`, plus a postfix `?` early-return operator. Exhaustiveness checking is **not yet ported** to the new compiler, so the inferred-union payoff is presently unavailable |
| Compiles to machine code via LLVM/wasm | The Zig compiler's **default lane is `src/eval`, a LIR interpreter**, with `llvm_compile/` and `machine_code_shim/` as siblings. Roc has converged on boru's own two-tier shape |
| Sugar: backpassing (`<-`) | **Removed.** Absent from the new tutorial and from roc-lang/roc's own `all_syntax_test.roc`; basic-cli's migration note reads "Replace deprecated backpassing, use new `?` syntax" |
| Purity absolutism taxes casual scripting | Partly relaxed: headerless application modules get a built-in Echo Platform, so `main! = |_args| echo!("Hello, World!")` is a complete program; `var $x` locals and `for … in` loops exist alongside `fold` |

Two facts the prior report never recorded, and which reframe the whole
exercise:

- **Roc's FAQ permanently excludes refinement types**, higher-kinded
  polymorphism and rank-N types, naming *"an exponential increase in
  compile times"* and the preservation of decidable principal inference as
  the price it refuses to pay. boru ships exactly that feature. On the
  type-system axis the two languages are not ahead and behind — they sit
  at deliberately opposite ends of one trade, and Roc named it out loud.
- **Roc's sharpest current claim is about *analysis*, not types**: *"when
  you run `roc check` or `roc build`, none of your dependencies —
  including platforms — are permitted to perform arbitrary I/O operations
  on your system."* That is the claim boru cannot make, and §7.1 shows why.

The consequence: the interesting question is no longer "what does Roc have
that boru lacks". It is **"what does Roc publish at build time that boru
only knows at run time"** — and, uncomfortably, "what does Roc's *tooling*
promise that boru's tooling quietly breaks".


## 2. The one-paragraph answer

boru consistently computes the right answer and then throws it away at the
boundary where a person or an agent would see it. The checker proves a
signature mismatch and drops the received-argument note, the per-candidate
verdict and the declaration span that `boru run` prints one subcommand
over. The policy system knows every gated capability and no `describe`
line names one. 249 error codes are registered and 33 are documented. The
registry knows a module's bytes and records no digest. Roc's transferable
lesson is a posture, not a feature: **publish the answer at build time,
make the artifact its own identity, and back a soft rule with a hard
consequence.** boru already owns the harder half of each of those three and
has not connected the last wire.


## 3. Axis by axis

### 3.1 Effects, purity, and the host boundary

**Roc.** Purity is inferred and compiler-enforced; effects originate only
in the single platform an application names; the standard library is pure.
The platform owns `main` and supplies the allocator, which is what lets a
platform author pick a domain-specific memory strategy (skip deallocation
for short scripts, arena-per-request for a server).

**boru.** A genuinely enforced *runtime object-capability* system —
capability install/deny, seven shipped profiles, argument-level
`where` predicates, blame chains (`boru policy explain`), attenuating
sub-engines — with no static face at all. No `describe` line names an
effect; `boru check` has no policy flag; effectfulness is not in any
signature.

**Verdict.** Same thesis, opposite ends, and boru's *mechanism* is the
finer-grained of the two: Roc's platform boundary is an `exposes` list
with no per-call policy. What transfers is not the mechanism but the fact
that Roc **answers at build time**. The urgent item on this axis, though,
is not a feature — it is §7.1.

### 3.2 Errors

**Roc.** Failure lives in the return type (`Try(Ok, Err)`), `?` early-
returns an `Err`, `crash` is a separate deliberately-unrecoverable channel,
and the error set is an inferred open tag union.

**boru.** Failure is out of band: any word may unwind, `do [...] error
[...]` is the reification boundary, and `Error` is a value-like Ideal
carrying a code atom. **249 codes are registered** (48 kernel + 201 lang —
`core.ErrorCodes()` at run time), **33 appear in `REFERENCE.md`'s "Common codes" table**, none
appears in any `describe` output, and there is no `boru explain`.

**Verdict.** boru is *ahead* on machine dispatch — a registered,
layer-owned code namespace with a bidirectional documentation gate is
something Roc has no equivalent of (Roc has named diagnostic banners, no
codes, no `roc explain`) — and behind on discoverability. The transferable
idea is naming the failure set where the reader already is. Roc's `?` does
**not** transfer (§5).

### 3.3 Types

**Roc.** Sound, decidable, principal inference; annotations optional
everywhere; no subtyping; refinement types permanently excluded.

**boru.** A hierarchical nominal lattice with ad-hoc overloading (`add`
carries nine signatures), predicate refinements, generics as interned
memoised type construction, Ideals as type-kinds. Inference is per-call-
shape abstract interpretation, which is *more* precise than HM at a call
site and cannot generalise.

**Verdict.** Not a gap — an incompatible trade. "Annotations optional" is
unreachable while overloading exists, and `design/BIDIRECTIONAL-CHECKER.0.md`
§5 already forbids type-directed overload resolution for a reason
(the compiler *is* the checker). What transfers is Roc's *day-to-day
payoff*: you see types you never wrote. boru is better placed than Roc to
deliver the discovery half, because its signature table is live in the
registry — the answer is computed, not curated.

### 3.4 Performance and memory

**Roc.** Reference counting with opportunistic in-place mutation; a
zero-parse memcpy-speed disk cache; the platform supplies the allocator;
build time is a stated product property ("almost always under 1 second …
through caching").

**boru.** Interpreter plus bytecode VM. The emitter refuses what it cannot
prove it can lower faithfully, and the runtime **silently** re-runs a refused
program on the interpreter. That machinery keeps miscompiles out — validated by
`design/MISCOMPILE-HUNT-FINDINGS.0.md` — but it is not an architecture to boast
of: every refusal is a program that did not compile, which is a failure, and the
silence means nothing in the run reports it. The target is that all valid code
compiles, with no exceptions. Compiled mode is on by default
since the P7 endgame — `CLI.md` still said the interpreter was the default,
and described the flip as *future*, until this report's PR corrected it.

**Verdict.** Roc converged on boru's two-tier shape, so this is no longer
codegen versus tree-walker — it is **front-end amortization**, and there
boru currently loses: nothing caches a compiled `Program`, and `boru build`
bakes *source*, so a shipped binary recompiles its whole program on every
execution. Perceus/FBIP does **not** transfer (§5). The cacheable lowered
artifact does.

### 3.5 Syntax and ergonomics

**Roc.** Makes non-obvious properties visible in the token stream (`!`
effectful, `$` reassignable, Capitalised = module/type), forbids
reassignment and shadowing, and — the important part — backs a *soft* rule
with a *hard* consequence: a warning **plus a non-zero exit code**, so
that "you can quickly write something with shadowing if you want but the
non-zero exit code prevents it from ending up in production code because
CI will fail."

**boru.** Already owns the visibility mechanism where it matters —
capital-initial is not a convention in boru, it *is* a type binding. But
its sugar surface is larger than Roc's ever was, `fmt` is not idempotent
(NUR046), and **every non-error `check` path exits 0**, so boru ships a
`warning` severity that nothing can act on (§7.3).

**Verdict.** One importable idea: the reconciliation — soft locally, hard
in CI. Every *sigil* borrowing is blocked by boru's own lexer (§5).

### 3.6 Tooling and DX

**Roc.** `roc docs --serve`, `roc fmt --check/--stdin`, `roc test
--verbose/--watch/-j`, `roc check --watch`, a documented piped-REPL stream
contract (results→stdout, diagnostics→stderr, no banner), inline `expect`.

**boru.** A 7,421-row executable spec across 121 TSV files, ADR-008 100%
coverage gating, `boru test` with `--coverage` and HTML reports,
genuinely-executed prose doctests, a citation gate, a self-describing
binary.

**Verdict.** Mixed, not "boru ahead". boru leads decisively on the
executable spec, coverage and true doctests — Roc is candid that `expect`
is a weak contract ("these are *not* production assertions"). It trails on
generated docs, watch mode, formatter modes and runner ergonomics. The
sharpest self-inflicted wound is that `boru check` — the surface editors
show — renders a strictly poorer diagnostic than `boru run --no-check`
does for the same defect, dropping a layer the runtime already builds.

### 3.7 Packaging and supply chain

**Roc.** No registry, no accounts, no lockfile. A dependency is a URL whose
final path segment *is* a BLAKE3 hash, verified during streaming
extraction, with per-package and per-graph size budgets. A package is pure
declarations and cannot perform I/O in any phase.

**boru.** The full ceremony of a curated ecosystem — `register`, `login`,
bcrypt passwords, bearer tokens, `publish`, `pack`, `install` — over an
artifact path that verifies nothing: no hash, no signature, no lockfile.

**Verdict.** boru has the *inverse* of Roc: **ceremony without mechanism**,
which is worse than no registry, because logging in manufactures an
impression of attribution. The single transferable idea is that **the
digest, not the name-and-version, is the dependency's identity** — and
`design/CONTENT-ADDRESSING.0.md` §4.1's table already records the artifact
digest's blockers as `nothing`.

### 3.8 Project strategy

**Roc.** A heavy outward layer: three-word positioning with an arguing page
per word, "not ready for a 0.1 release yet" as the README's first line, a
foundation with a public funding goal and named sponsors, a CI-built
examples gallery. It pays a visible staleness tax for it, and admits so.

**boru.** A world-class *inward* layer that Roc has no equivalent of: the
Diátaxis manual, 16 ADRs, a Non-Uniformity Register with mandatory
recording and delete-on-resolve, a machine-verifiable knowledge graph, a
self-documenting binary. Almost no outward layer.

**Verdict.** Inverted profiles. boru's honesty machinery is structurally
stronger than Roc's *and invisible*; Roc's positioning is legible *and
stale*. The cheap moves are nearly free; the expensive Roc artifacts
(foundation, chat server, code of conduct, donate button) are theatre at
boru's current size and should be declined explicitly, not by omission.


## 4. What to adopt — ranked

Ordered by leverage. "Endorsed by" counts how many independent axes
arrived at the item; independent convergence is this report's strongest
confidence signal.

| # | Idea | Effort | Endorsed by | Novelty vs prior report |
|---|---|---|---|---|
| **A1** | Stop `check`/`describe`/`repl`/LSP executing dependency code with ambient authority; gate module bodies under policy | small→medium | 2 axes | NEW |
| **A2** | Fix the typed-def const-pool divergence (§7.2) and add the missing spec rows | medium | 2 axes | NEW |
| **A3** | `boru check --pedantic` — a severity-promoting exit code | **small** | 2 axes | NEW |
| **A4** | Make `describe`'s worked examples engine-verified; narrow AGENTS.md's no-drift claim until they are | **small** | 2 axes | adjacent R5 |
| **A5** | Record an artifact digest at `install`; hash registry tokens | medium | 1 axis | executes CONTENT-ADDRESSING.0 §4.4 |
| **A6** | Make `check` say what `run` already says (spans, candidate verdicts, notes) | medium | 3 axes | NEW |
| **A7** | Doc↔registry gates: every documented word, scope and code must resolve in the live registry | medium | 5 axes | NEW |
| **A8** | `Effects:` and `Raises:` lines in `describe`, seeded from the gate sites and the error-mint sites | small (step 1) | 2 axes | REFINES R1/R2 |
| **A9** | `boru explain <code>`; generate the code table from the registry | medium | 1 axis | REFINES R4 |
| **A10** | Amortize the front end: bake the compiled `Program` into `boru build`; make compilation cost-aware | large | 1 axis | REFINES R8, re-sequenced |
| **A11** | The cheap outward layer: repo description, playground URL, status above the fold, `examples/`, CONTRIBUTING | **small** | 2 axes | NEW |
| **A12** | Settle `fmt` (close NUR046), then add `--check` and `--stdin`, keeping zero *style* options | medium | 2 axes | NEW |
| **A13** | Show declared and inferred types back to the user (`describe` return column; `describe <Type>`) | medium | 2 axes | overlaps R5 |
| **A14** | REPL piped-stream contract: results→stdout, diagnostics→stderr, no banner | **small** | 1 axis | NEW |

### A1 — the analysis commands must not run untrusted code with ambient authority

Roc's guarantee is structural: a package is pure declarations, so `roc
check` cannot perform dependency I/O. boru's position is the exact
inverse, and it is not theoretical — see §7.1 for the reproduction.

Two phases. **Phase 1 (small, do first):** register the existing
`cmd/go/internal/permsflags` on `check`, `describe` and `repl` — today
`cmd/go/internal/check/check.go` contains the string `perms` zero times —
and pass a resolved policy in `cmd/go/internal/lsp/diagnostics.go` instead
of an empty `lang.Options{}`. Honour `BORU_POLICY`, which `REFERENCE.md`
already documents as an environment fallback and which `check` ignores.
**Phase 2 (medium, separate change):** two halves, and the second is the
load-bearing one. *(a)* Gate the *file-module* import path the way
`modules.Resolve` already gates natives — `Installed`,
`Check("modules","import",…)`, per-module `install:false` — keyed on the
declared ref. *(b)* **Install the parent's policy on the module
sub-registry.** Gating the import alone is not sufficient:
`runModuleBodyCover` (`lang/go/native/native_module_module.go`) builds a
fresh sub-registry and deliberately inherits `Output`, `ErrOutput`,
`Input`, the effect ledger, observe hooks, runtime stamping, `HostFileOps`,
`CapMemFileOps`, the `ModuleInheritedCaps` seams, host formats and
extensions, `ParseFunc`, `BaseDir` and the TCO switch — **and no policy at
all**. Since every gate resolves `HostPolicy(r)` on the registry it runs
on and treats `nil` as allow (`checkFetchPolicy`: `if pol == nil { return
nil }`), a permitted `./lib.boru` still performs the denied network call.
That is the mechanism behind §7.1's measurement, and it means the policy
must ride into the child — attenuated, never widened, the way
`Vm.run-sandbox` already attenuates. Separately, compose a hard **egress
floor** over whatever the user asked for during check, so an analysis pass
can never reach the network whatever the profile says. Add `boru check
--no-exec-imports`, which is nearly free because `importFileHandler`
already degrades gracefully when a module fails to load.

### A3 — `boru check --pedantic` (the cheapest item in this report)

The exit decision in `cmd/go/internal/check/check.go` is
`if !soft && res.Summary.Errors > 0`. Promote *inside* the existing guard —
`if !soft && (res.Summary.Errors > 0 || (pedantic && res.Summary.Warnings+res.Summary.Infos > 0))` —
so `--soft` keeps meaning "never gate" and the two flags compose instead of
fighting. The condition occurs **twice**, in the JSON branch and the text
branch of `RunColor`; both must change together, or `--json --pedantic`
disagrees with `--pedantic`.

It matters out of proportion to its size because **boru currently ships a
`warning` severity that nothing can ever act on** (§7.3), and every
advisory boru has built or will build inherits that ceiling. `--soft`, the
only lever `CLI.md` documents, points the *other* way. Roc's
reconciliation is exactly right and worth copying verbatim in spirit:
soft on the developer's machine, hard in CI.

### A4 — make the no-drift claim true

`AGENTS.md` tells every agent and contributor that `describe` output
"cannot drift from the code the way prose can". That is true of
signatures, precedence and dispatch order. It is **not** true of worked
examples: `boru describe add` ships the line `add true false   ;# ...`,
a literal placeholder, and it is not alone (`sub`, `mul`, `div`, `mod`,
`pow`, `refine`, `var`, `undef`, `for`, … each ship one or two).

The generator already evaluates through the real engine; the defect is
that it swallows the engine's error and falls through to a Go-side
heuristic. Record the error code instead, so the line reads
`add true false ;# error [boru/type_error]`, and close the two exclusions
in the example verifier that exempt hand-authored examples from being
re-run. Until then, scope the AGENTS.md claim to what is actually gated.

This ranks above much larger items because no-drift documentation is
boru's headline differentiator against Roc's openly-stale marketing pages.
A single visible placeholder in the tool's flagship output undermines the
claim the whole strategy rests on.

### A7 — doc↔registry gates (five axes found the same class)

Build a gate in the shape of the existing `test/go/docexamples/`
error-code gate: every backticked word name in a `REFERENCE.md` word table
must resolve in the live registry; every scope name any doc mentions must
be in `policy.KnownScopes`; every diagnostic code the checker can emit
must appear in a table and vice versa. Then fix what it finds — the
compounding case is the sharpest: a reader follows `REFERENCE.md`, runs
`boru policy test sandbox fileio.read`, gets ALLOW and exit 0, and
concludes the sandbox permits file reads. `fileio` is not a scope, and
`sandbox` denies `fileops.read`. Adding `policy.IsKnownScope` and calling
it from the CLI's scope splitter closes that one.

*Sequencing constraint:* tagging the docs' code fences ` ```boru ` so the
fmt gate can see them would **corrupt** `REFERENCE.md` today, because
`make fmt-docs` rewrites the canonical `fn [[a][b][c]]` form and collapses
aligned `# returns` comments. That half must wait for A12.

### A8 — one facet, two alphabets

Treat effects and raises as the same feature, as
`design/local-reasoning-for-global-properties-in-boru-report.0.md` §6
already proposes. **Step 1, shippable alone:** add structured `Effects`
and `Raises` fields to the help entry and render them, so
`boru describe boru:net:fetch` gains `Effects: network.connect` and
`boru describe div` gains `Raises: arith_error`. Seed both mechanically —
effects from the `policy.Check(scope, op, …)` gate sites, raises from the
`r.BoruError(code, message, word)` construction sites whose third argument
is the word name; the existing error-code gate already scans exactly those
sites. Gate the committed table with a drift test.

The vocabulary is already closed and enumerable: 13 policy scopes
(`policy.KnownScopes`) and 249 registered codes. What is missing is a place to write it down.
Today `boru describe div` says "Integer division by zero is an error"
without naming `arith_error`, so a reader who wants a `case` arm has to
provoke the failure and print the result.

**Step 2, only if step 1 earns it:** propagate a raise-set through the
joins the checker already performs, behind a non-gating `check --raises`
tier. Keep it advisory — an open error world is settled boru design, and
the point is locality, not checked exceptions. Mark over-approximation
honestly (`@body`, `@unknown`) rather than reporting the ambient grant,
or the facet is noise on the first non-trivial program.


## 5. What not to adopt

The declines matter as much as the adoptions, and several are backed by a
shipped failure rather than an opinion.

- **Roc's `!` effect suffix, or any sigil, as boru's effect marker.**
  `!` is a token terminator in boru's grammar — `def go!` lexes as `def
  go` plus an undefined `!`, and `boru fmt` rewrites it — and it already
  means *strict* in `!.`, a loudness axis rather than an effect axis. The
  `/x` modifier namespace is worse: every existing modifier describes the
  **call site**, so `/e` would be a category error. In Roc the `!` is only
  a warning-enforced lint anyway; the load-bearing marker is the `=>`
  arrow and the inference behind it.
- **Roc's `$` mutable-local sigil.** `def $path 7` binds successfully
  today — `$`-prefixed names are live user identifiers — the all-`$` form
  is reserved as the receiverless-Reach sentinel, and boru already owns
  the word `var` for something else. More fundamentally, boru's mutability
  lives in the **value's type** (FlexMap, Store, class instance), not in
  the binding, so a name-level marker would be wrong in the common case.
- **A `/?` or `try` call-site fallibility marker (Roc's `?`).** Polarity
  inversion. Roc's `?` exists because propagation is *not* the default
  there. In boru propagation **is** the default, so a marker meaning "let
  the error out" would mark every call and mean nothing; the shaped
  inverse already exists and is spelled `do [...]`. The spelling is also
  taken — `CLI.md` records that `?` is a fixed lexer token and that `tty?`
  could not be dispatched and had to be renamed. A decline backed by a
  shipped failure is worth writing down.
- **Open tag unions / row variables for inferred error and effect sets.**
  Roc needs open rows because its inference is *modular*. boru's checker
  is whole-program and per-call-shape, so it can compute the exact
  **closed** union at each site with no new abstract domain. Adding open
  unions would import rows, unification variables and generalisation for a
  problem boru does not have, and would collide with `case`
  exhaustiveness, which is closed-domain and interval-based.
- **Roc's exclusion of refinement types** — the inverse of a normal
  decline. Roc forwent them for compile-time reasons; boru ships them and
  keeps the cost bounded by a two-tier split worth stating honestly:
  DepScalar refinements are decided statically, while fn-bodied predicate
  types are deferred entirely to run time. That split is the real trade,
  and it is a boru capability Roc has permanently given up — not a boru
  deficiency. (Its price is §7.4.)
- **`x.to_str()` static dispatch as a call spelling.** Roc needed static
  dispatch because it has no overloading; it is a *substitute* for
  signature dispatch, and it has already replaced one mechanism
  (abilities) in that slot. In boru `.` already means `get`/`getr`
  navigation with member auto-invoke, and the dot chain is the sole
  structural exemption from the strict forward barrier. The separable half
   — *discovery*, "what can I do with an Integer?" — is A13.
- **Implicit structural conformance** (Roc's `where` without a
  declaration). It works in Roc because a method belongs to exactly one
  nominal type's associated block. Under boru's open multimethods anyone
  may add an overload for anyone's type, so structural conformance would
  shift with import order; `design/SURFACES.10.md` §6 already rejected it
  for that reason. The transferable part is the friendliness of the
  diagnostic, not the mechanism.
- **Uniqueness/refcount in-place mutation (the prior report's R10).** Not
  implementable against boru's value representation, and the tree has
  already been forced the *opposite* way. A uniqueness bit cannot live on
  `core.Value` — it is copied by value into every tape cell, argument and
  local, so a copied bit carries no aliasing information and there is no
  per-object header. `ListPayload` is a value-typed struct boxed in a Go
  interface and is physically unwritable through a copy — which is exactly
  why the mutable twin is a pointer type. And the bytecode emitter already
  had to *add* copies (`OpPushConstFresh`) after miscompile mechanism A.
  `flex` already delivers the win explicitly.
- **Platform-chosen allocators.** Go's allocator and GC are not pluggable
  and there is no host that owns `main`. The transferable half — amortize
  setup across a boundary you control — is already implemented twice and
  merely undocumented as a performance story (the engine pool plus tape
  reload is an arena-per-sub-run in all but name).
- **Roc's inline `expect` with build-mode erasure.** boru already has
  `boru test`, `boru:test`, `--coverage`, `--coverage-min` and an HTML
  report; the gap is *placement*, not capability. The erasure trick
  actively must not transfer: making a construct behave differently by
  engine mode is precisely the divergence class `make verify-bytecode`
  exists to prevent — and Roc has not settled it either (`--opt=speed`
  "does not discard `expect`s yet").
- **A version solver and semver ranges.** Roc's solver is cheap only
  because there is no trust decision inside the solve — every candidate
  URL carries its own verified digest. boru has neither property yet.
  Exact pins plus a recorded digest buy reproducibility at none of a
  resolver's cost. Revisit if an ecosystem forms.
- **Non-blocking compilation as the default.** Roc *wants* "roc will still
  run your program even if you have compile-time errors" and has not
  shipped it — the passage sits inside an HTML comment marked `TODO not
  implemented yet`, and the old site calls `--allow-errors` "only
  partially completed, and often errors out". boru already has the
  property by construction via `--no-check`, and additionally keeps the
  checked default, which is the better polarity for a language whose
  default surface is `boru do '…'`. Add `--allow-errors` as an opt-in that
  reports *and* runs; never flip the default.
- **A third-party native platform/host tier.** Roc knowingly ships
  prebuilt native host binaries inside the hash-verified bundle and
  dlopens third-party dylibs on the `roc glue` path. boru's position —
  sealed at compile time, no FFI, the compiled-in native module list as
  the trust boundary — is strictly stronger and should stay. It should be
  *stated* rather than left implicit, because it also means boru can never
  match Roc's arbitrary-domain platform story.
- **The community and funding apparatus** (foundation, donations, chat
  server, code of conduct, governance document, sponsors page).
  Load-bearing for Roc; theatre at boru's current size. Even Roc declines
  the heavyweight version of its own process: "having a formal process
  (like a RFC system) would be more heavyweight than it's worth." State
  the absence deliberately and name the trigger for revisiting it — a
  second regular contributor, not a star count.
- **The argued-marketing-page model as the primary docs investment.** Roc
  pays a visible staleness tax and admits it in writing. boru should write
  one argued `WHY.md` and stop, with the rule that every claim cites a
  repo path, a command or a count, and every count is re-measured before
  publishing. boru's docs investment pays in the *generated* surfaces,
  precisely because they cannot drift once A4 lands.


## 6. Pitfall watchlist

| Pitfall | Roc's experience | boru's exposure |
|---|---|---|
| An inferred effect/raise set that over-approximates becomes noise; one that **gates** breaks boru's polarity | Roc sidesteps it structurally: purity is one bit, no effect polymorphism, and even a mis-annotation is a *warning* | `each`/`fold`/`filter`/`do`/`apply` are inherently effect-polymorphic; `MODULE-SECURITY.0` §7.3 enumerates the escape hatches. The gating half is *already* firing: `unconditional_raise` is SeverityError and refuses to run a script that prints then raises |
| A doc-example gate with a cheap escape hatch degrades into a skip list | Roc is candid that `expect` is a weak contract | boru's prose doctest gate does this **right** (a tracked map that may only shrink, dead entries fail loudly). Copy that discipline verbatim for A4 — never an inline exemption comment |
| Speculative sugar is cheap to add, expensive to retract | backpassing, `Task` and abilities were each added then removed | boru's sugar surface is already larger than Roc's ever was; two pieces carry the same profile — the `+kind<src>` minilang whose canon does not round-trip (NUR072), and the XML literal's silent second claim on `<`/`>` |
| Documentation and performance folklore rot faster than code | roc-lang.org still teaches the removed `Task` — and says so | Six design notes say `Value` is 72 bytes; it is **104** (measured), ungated, and `core/go/tape.go` derives a shipped growth ceiling from a figure 56% too high. A no-drift claim is worth more than Roc's disclaimer *only while it is true* |
| Registry ceremony without integrity is worse than no registry | Roc shipped no index, no accounts, one hash | §3.7 |
| Runtime-only refusals train users to distrust `check` | Roc enforces purity at typing time, so a clean `roc check` means something | `boru check` reports 0/0/0 on a program guaranteed to raise `[boru/not_sendable]` — the exact example `EXPLANATION.md` uses to teach the guarantee |
| Compile-by-default without amortization is a latency regression | Roc attaches a number to build time and backs it with a cache | P7 flipped the default to compile-try and nothing amortizes it; a `boru build` binary recompiles its whole program on every execution |
| Ecosystem machinery before there is an ecosystem | Roc's apparatus is load-bearing against ~25 recurring sponsors | Once a missing-artifacts list is written down the temptation is to fill it. At boru's size the items that pay are the repo description, the status block, `examples/`, and a CONTRIBUTING that states the real gates |


## 7. Defects surfaced while verifying

These were found while checking claims for this report, and each was
reproduced by hand against `cmd/go/bin/boru` built from the working tree.
They are recorded here because they bear directly on the comparison — each
is a place where boru's answer to a Roc guarantee is weaker than boru's own
documentation says.

### 7.1 Policy does not reach inside a file-module body, and the check pass runs it anyway

Under the `read-only` profile, a top-level `import "boru:net"` is refused.
The identical import **one file deeper** is not:

```
# lib.boru
import "boru:net"
def sneaky (Net.fetch "http://127.0.0.1:8797/VIA-IMPORT")
export "Lib" {x: 1}

# main.boru
import "./lib.boru"
print 1
```

Against a local listener:

```
boru run --no-check --perms read-only direct.boru   exit 1  hits 0   # refused
boru run --no-check --perms read-only main.boru     exit 0  hits 1   # NOT refused
boru run           --perms read-only main.boru      exit 0  hits 2   # check pass fetches too
```

Three findings in one: (a) a gated word inside an imported file-module
body is **not policed at all** — the profile is bypassed by moving the
import one file deeper; (b) the pre-flight check pass executes the body a
*second* time, so even a profile that did gate it would already have
leaked; (c) `--deny=network.connect` on `main.boru` likewise does not stop
it, though it correctly refuses the same call at top level.

Separately, on the analysis commands themselves — each made a real
outbound HTTP request from an imported module body:

```
boru check main.boru                       exit 0  hits 1
BORU_POLICY=sandbox boru check main.boru   exit 0  hits 1
boru describe ./lib.boru                   exit 0  hits 1
boru repl  (piped `import "./lib.boru"`)   exit 0  hits 1
boru check --perms sandbox main.boru       exit 1  — "open --perms: no such file or directory"
```

`boru check` registers no policy flag at all (`cmd/go/internal/check/check.go`
contains the string `perms` zero times) and ignores `BORU_POLICY`, which
`REFERENCE.md` documents as an environment fallback and which demonstrably
works on `boru do`. A module body that reads a host file and then sends it
is a complete exfiltration primitive, and the LSP runs this path
automatically on file open.

To boru's credit, the `module_body_executed_in_check` info advisory
already tells the truth in one clause — "a network send and a stdin read
are not modelled and still do" — so the behaviour is known. It is the
*reachability* of the sandbox that is broken, not the project's awareness.

This is recorded as **NUR079**.

### 7.2 A typed def over an Integer literal loses (or leaks) its newtype brand under the compiler

```
def UserId (refine Integer)
def b:UserId 9
print (typeof 9)      print (typeof b)      print (b is UserId)
```

| | `typeof 9` | `typeof b` | `b is UserId` |
|---|---|---|---|
| `--no-compile` (interpreter) | `Integer` | `UserId` | `true` |
| default / `--force-compile` | `Integer` | **`Integer`** | **`false`** |

And in the other direction — with `typeof b` evaluated first, the bare
literal acquires a brand it never had:

| | `typeof b` | `typeof 9` |
|---|---|---|
| `--no-compile` | `UserId` | `Integer` |
| default | `UserId` | **`UserId`** |

`boru check` reports `0 error(s), 0 warning(s), 0 info`, and
`--force-compile` does not refuse — it compiles and answers wrongly. This
is a silent compile≠interpret divergence **on the default execution path**,
on the one guarantee that is boru's differentiator against Roc's type
system: nominal newtype identity in the lattice. It looks like the const-
pool entry for the literal being reparented rather than a fresh value
being minted; `design/MISCOMPILE-HUNT-FINDINGS.0.md` §B's July-2026 update
says in as many words that "The static/concrete path is untouched." A
String newtype (`def Name (refine String)`) did **not** reproduce it.

This is recorded as **NUR080**.

### 7.3 `boru check` cannot fail on a warning

```
$ boru check -e 'def y 1  2'
check: 1:5: [warning] unused_def: def y is never used
check: 0 error(s), 1 warning(s), 0 info
exit=0
```

Every non-error path exits 0, so `warning` is a severity nothing can act
on, and `--soft` — the only lever `CLI.md` documents — points the other
way. This is what A3 fixes with a one-line change to the exit condition.

### 7.4 `boru describe` ships placeholder examples

`boru describe add` includes the line `add true false   ;# ...`. The
placeholder is present for at least ten core words, against an `AGENTS.md`
claim that `describe` output "cannot drift from the code the way prose
can". This is what A4 fixes.


## 8. Corrections to `rust-zig-roc-faber-in-boru-report.0.md`

One sentence each; each correctable in place.

**About Roc:**

1. §1/§4 describe Roc as compiling to machine code via LLVM or wasm; the
   new Zig compiler's **default lane is a LIR interpreter** (`src/eval`),
   with LLVM and machine-code shims as siblings — Roc converged on boru's
   two-tier shape.
2. §6.1's Polymorphism cell ("abilities + deriving") and §4.1's
   "Abilities-with-deriving ↔ Ideal func-fields" convergence are stale:
   **abilities were removed** and replaced by static dispatch with inline
   `where` constraints.
3. §6.1's Errors cell should read `Try(Ok, Err)` with a postfix `?`, and
   must carry the caveat that **exhaustiveness checking is not yet ported**
   to the new compiler.
4. §6.1's Effects cell hides a complete mechanism change: purity is now
   **inferred**, carried by `->` vs `=>`; `Task` is removed; `!` is a
   naming lint; effect polymorphism is excluded by design.
5. §6.1's Compat cell ("none pre-1.0 (churn)") is now wrong, and the
   correction **flips the comparison against boru**: Roc increments the
   alpha number on every breaking change and ships `roc bump` to diff a
   package's public API, while boru's stated rule is "Bump patch when
   publishing anything", which encodes nothing.
6. §4 never mentions Roc's **permanent exclusion of refinement types**,
   which inverts the naive reading of the type-system axis.
7. §4 never mentions Roc's sharpest claim and boru's sharpest gap — that
   `roc check`/`roc build` perform **no dependency I/O**.
8. §4.3's "purity absolutism taxes casual scripting" is partly stale: Roc
   added headerless application modules with a built-in Echo Platform.
9. §4.3's backpassing example is cited without a primary source; the two
   artifacts that do support it are `all_syntax_test.roc` (no `<-`) and
   basic-cli's migration note.

**About boru:**

10. R1 ("inferred error sets") is mis-scoped twice: boru already computes
    divergence and already models the trapping boundary, so the remaining
    work is propagation plumbing, not recovery of a discarded fact — and
    the sets should be **closed** per-call-site, not open rows (§5).
11. R2 and §4.2 cite `'cap_denied`; that code is a **phantom**. The real
    codes are `boru/permission_denied` and `boru/capability_not_installed`,
    which have different remedies.
12. Every boru atom in §4.2/§4.3/§6.2 spelled with a leading apostrophe
    (`'io_error`, `'bad_input`) is **doubly wrong**: `'` is boru's *string*
    delimiter, so the spelling is a syntax error, and the atom form is
    `io_error/q`. `io_error` itself is one of the phantom codes the
    error-code gate removed.
13. §4.1's polarity claim ("CLI all-on") is half true: capability
    installation *is* policy-conditional; what is actually open is the
    **no-policy default** for fileops and sqlite.
14. R10 (uniqueness-based FBIP) is under-specified in a way that makes it
    look landable; §5 gives the three representation facts that block it.
15. §1.1's "`Value` shrink 184→72 bytes" is stale — it is **104 bytes**
    today (measured), ungated, and the growth is exactly the four
    nil-by-default facet pointers.
16. The report's reading of `boru build` as AOT is wrong: the build
    embeds **source**; `design/AOT-COMPILE.0.md` states plainly that "No
    bytecode serialization exists today".
17. R8 is gated on the MODULE-CACHE singleton decision, but the
    `boru build` half is independent of module singleton semantics — and
    `CONTENT-ADDRESSING.0` §4.1 records the **artifact digest's blockers as
    `nothing`**, so the supply-chain half is entirely unblocked.
18. §6.1's boru Agent-surface cell is true for signatures and dispatch
    order but **not for worked examples** (§7.4).
19. §6.1's boru Compat cell says "~11K-row executable spec"; it is
    **7,421 rows** across 121 TSV files (13,328 lines including comments).
20. §6.3's "keep one surface + canonical `fmt`" treats the formatter as a
    mitigation already held; `fmt` is **not idempotent** (NUR046).
21. §4.1's "failures must be loud" convergence holds for `boru run` and
    **not** for `boru check`, which is what drives the LSP.
22. §4/§6 contain **no packaging axis** — even though Roc's
    URL-plus-content-hash model is arguably its most directly transferable
    idea for boru at this stage — and **no project-strategy axis** at all.


## 9. Closing

boru's philosophical nearest neighbour remains Zig; its *architectural*
nearest neighbour was Roc, and after the Zig rewrite it is more so than
before — two-tier execution, refuse-rather-than-miscompile, friendly
diagnostics as a stated value. But the axis on which the two languages
have most to say to each other is no longer semantics. It is **what the
toolchain promises before the program runs**.

Nine of the fourteen recommendations here are a flag, a document, or a
call site. Four of the items in §7 are defects with reproductions. Only
three (A10, A8 step 2, A12) are real engineering. Against that, the two
things worth protecting hardest are the ones boru already does better than
Roc and that nothing here should erode: **the executable spec as the
behavioural contract, and the Non-Uniformity Register as a standing, dated
inventory of every place the language contradicts itself.** Roc has no
equivalent of either, and its own marketing pages are the cautionary case
for what happens when the honesty layer is prose.

The failure mode to guard against while acting on this report is spending
the novelty budget on ecosystem machinery — a code of conduct governing
nobody, a semver solver for a registry with no users, an effect sigil that
cannot be retracted — while the small fixes to `check` and `describe` stay
unmade.
