# Implementation plan — acting on the Quint and Roc surveys

**Status:** plan (`.0`, draft). No code change. Nothing here is an accepted
decision; the waves are a proposal for a maintainer to accept, reorder, or
reject. Companion to [`legacy/QUINT-COMPARISON.0.ignore`](legacy/QUINT-COMPARISON.0.ignore) and
[`legacy/roc-in-boru-report.0.ignore`](legacy/roc-in-boru-report.0.ignore).

**Method.** Eleven work items, each scoped by an independent pass that
read the code and ran the binary, then sequenced, then criticised. Claims
marked **[ran it]** were reproduced against a binary built from this tree.
Where a scoping pass refuted the brief it was given, the refutation is
kept — §9 lists three.

---

## 1. What changed while the surveys were being written

Two things arrived after `legacy/QUINT-COMPARISON.0.ignore` was drafted, and both
move the plan's centre of gravity.

**`legacy/roc-in-boru-report.0.ignore` landed** with its own ranked list (A1–A14) and
four reproduced defects. It is not a parallel queue — it converges with
the Quint survey on four items, and independent convergence is the
strongest prioritisation evidence available here:

| Quint survey | Roc report | Resolution |
|---|---|---|
| code lookup surface (P7) | **A9** | one item; both descend from R4 |
| effect envelope (P8) | **A8** (`Effects:`/`Raises:` in `describe`) | A8 is the cheap first step of P8 |
| inert permission surfaces (§6.2) | **A1** + §7.1 | A1 is a security defect and outranks it |
| CLI hygiene (P1) | **A3** + §7.3 | `check` cannot fail on a warning — same item |

**NUR079 was recorded** (`NUR.md`, 2026-08-18, Pending), surfaced by the
Roc study. It is the single most serious item on the board and it did not
exist when the Quint survey was written:

```
boru run --no-check --perms read-only direct.boru   exit 1  requests 0
boru run --no-check --perms read-only main.boru     exit 0  requests 1
boru run            --perms read-only main.boru     exit 0  requests 2
```

Move an `import "boru:net"` plus a `Net.fetch` one file deeper and every
profile — including the shipped `read-only`, `sandbox` and `compute` — is
silently bypassed, as is `--deny=network.connect`. `runModuleBodyCover`
inherits every capability seam **except policy**, and each gate resolves
`HostPolicy(r)` and treats nil as allow. The pre-flight check pass then
executes the body a *second* time, so the leak doubles.

This displaces what the Quint survey called P1. A diagnostic tool that
reports green over an unchecked file is a wrong answer; a declared
security boundary that does not hold, while `boru policy explain`
affirms it does, is a worse one.

---

## 2. The ordering principle

> **Order by the blast radius of a silent wrong answer over its cost,
> subject to two hard rules: anything that makes a *gate* blind ranks
> above anything that gate would catch, and no item whose own spec says
> "measure first" is built before its measurement lands.**

The Quint survey proposed "shipped verbs with silent-pass defects outrank
RFC-stage modules." That is directionally right and wrong about the unit.
The defects are not distributed one per item — the worst three were found
*underneath* items scoped as something else:

- NUR079, under an inventory job (§1).
- A `where`-key fail-open, under the same inventory job (§4, W-POLICY).
- A four-line vacuous-pass bug in `Test.check-prop`, under a feature item.

And the survey's heuristic is right at the bottom of the board, but not
for the reason it gave. `w9` (effects) and `w11` (SMT) belong last not
because they are RFC-stage but because **their own scoping passes
concluded the evidence to justify them does not exist yet**, and both
wrote themselves a measurement gate to prove it.

---

## 3. Wave 0 — governance (human only)

Everything here needs a human with `workflow` scope. It is the only work
on the board an agent cannot carry.

**⚠️ Do not follow `ci/README.md` as written.** It says to
`cp ci/ci.yml .github/workflows/ci.yml`. The live workflow has since
gained a `no-binaries` job, a TypeScript lane, `make -C kg verify`, a core
standalone coverage gate, bytecode race gates, a WASM build and vulncheck
— **none of which exist in the staged file**. Copying it deletes six
working jobs. Merge steps by hand, after a real
`diff -u .github/workflows/ci.yml ci/ci.yml`, making an explicit keep/drop
call on every step present only in the staged file, then `git rm -r ci`.

Three actions, in this order:

**0a. Run the in-language suites in CI.** This is the cheapest, highest
value step and it is not in either survey. `Makefile:32`'s `MODULES` list
excludes `utils/` and `kg/`, and CI runs only `make -C kg verify` (the
digest freshness check, not the tests). Yet these are the **largest
in-tree consumers of exactly the surfaces this plan rewrites**: `utils/`
runs `boru test` over its suite end-to-end, and both `utils/` and `kg/`
run `boru check` and `boru fmt` over ~25 files each. A regression in any
of them lands green today. Adding `make -C utils check test` and
`make -C kg check test` costs seconds, needs only the built CLI, and adds
no coverage obligation. **Sequence this before W-CLI, W-TEST and W-SEED,
not alongside.**

**0b. Add the coverage gate as a parallel job — do not append it.** The
live workflow's own comment records that the maintainer already knows
`make cover-gate` is absent and already declined it on CI-budget grounds
("the merged gate re-profiles all 13 modules (~20 min) and would dominate
the job… a separate call about CI budget"). Appending serialises it behind
vet/lint/test/parity/wasm. Adding it as its own job with `needs:` nothing
puts wall-clock cost at zero on the critical path and answers the budget
objection rather than overriding it. Note this roughly doubles per-PR
compute — a real decision for a one-maintainer project, so state the
runner-minutes delta when making it. (Drop `cover-gate-parser` from the
list: `parser-parity`, already in live CI, begins by invoking it.)

**0c. Make `build-and-test`, `no-binaries` and the new gate job required
status checks on `main`.** `ci/README.md` records that today "a red CI does
not block a merge — which is how the cover-gate and langspec-ratchet
failures reached `main` in the first place." Until this is true, **every
gate this plan adds is documentation, not enforcement.**

---

## 4. Wave 1 — close every verified silent wrong answer

Entry: 0a done; 0b/0c done or explicitly waived in writing. Each item
lands in days.

| Item | Scope | Why here |
|---|---|---|
| **W-NUR079** | Carry the parent policy, attenuated, into `runModuleBodyCover` (the `Vm.run-sandbox` pattern NUR079 names). Ship *with* updated built-in profiles — `sandbox`/`read-only`/`compute` all set `modules.words.default: deny`, so gating without that refuses every multi-file program. | §1. Highest severity on the board. |
| **W-PERMS-RIDER** | Accept `--perms`/`BORU_POLICY` on `check`, `describe`, `repl` and LSP. Three lines — `cmd/go/internal/test/test.go` already does `permsflags.Register` + `Resolve`, and `BORU_POLICY` fallback is built in. | The other half of NUR079's remedy. Split out of the effects item, which would have stranded it in Wave 4. |
| **W-POLICY** | (i) type `boru policy explain`'s arguments; (ii) validate `where` keys at profile-compile time; (iii) implement `maxBytes` as `≤`. | `whereKeyMatches` returns `true` for any absent arg, and its doc comment promises `maxBytes` is "handled as actual ≤ value" while the body has **no `maxBytes` handling at all** — it calls `matchOne`, which is equality [ran it]. So a rule's restriction is silently deleted by a key the schema does not recognise, and `explain` affirms ALLOW. |
| **W-CLI** | `flag.FlagSet` for `check`; variadic `check`; shared directory discovery for `check`/`fmt`; extract `build.go`'s interleaved-parse loop; fix `install -r`. | `boru check a.boru b.boru` exits 0 while the second file has errors [ran it]. `boru check -h` fails. `CLI.md` documents `-r`/`-s` for `check`, which the parser never reads. `boru install m -r URL` contacts the default registry — a *documented* invocation. |
| **W-PBT-GUARD** | Guard `runs < 1` in `runCheckProp`. | ~4 lines. Today `Test.check-prop … 0` with a body of literally `[false]` reports `{ok:true, runs:0}` and `Test.report` prints `pass`. Cheapest risk reduction available. |
| **W-GATES** | Move the severity gate to `test/go/docexamples` beside `errorcodes_test.go`, reusing its seven-module `codeSourceRoots`; make the mint-pattern check fail closed. | Two gates are blind: the severity table scans only `core/go` and `lang/go/native`, and `case_not_exhaustive` is minted through a helper matching none of the four mint patterns. Gate blindness outranks what the gate would catch. |
| **W-SPELLING** | `= note: reads as: …` on the two forward-greediness advisories. | S, no dependencies, no corpus churn. Quint's "print the normal form". |
| **W-FRONTMATTER** | Front-matter schema, `_test.go` gate, `designindex` tool, `make design-index`, ratchet at 0; **start the backfill here**. | `_test.go`-only: zero CI edits, zero ADR-008 obligation, so the `workflow`-scope blocker never applies. Its risk is rebase churn against a fast-moving `design/` — an argument for starting *earlier*, not later. |

**Done:** the nested-import bypass is closed and `check`/`describe`/`repl`
honour a profile; an unrecognised `where` key fails at load naming the key;
`explain` and the enforcer agree on numeric predicates;
`boru check a.boru b.boru` exits 1 if either errors; `boru install m -r URL`
reaches URL; `Test.check-prop … 0` raises; both gates are green against a
corrected tree; `make design-index` produces a table.

---

## 5. Wave 2 — complete the shipped verbs, land the gates

Entry: Wave 1 merged, Wave 0 complete. **W-POLICY-INVENTORY and any new
ratchet are worth building only after 0c** — a gate that cannot block a
merge is a comment.

W-CLI tail (fmt directories; the guard sweep) · **W-DESCRIBE-JSON**
(`describe --json`, all five modes, with `returns` and `returns_inferred`
separated — see §9) · **W-CODES** (the code lookup surface, after
describe's `kind` exists) · **W-TEST** (`-run`, `-json`) · **W-SEED**
(after W-TEST on `test.go`) · **W-PBT-REPORT** (distinct-inputs, witnesses,
`Rand.frequency`/`Rand.such-that`) · **W-POLICY-INVENTORY** (generated
enforcement inventory, seeded allowlist, then `clock` / `MaxStepBudget` /
`MaxSubEngineDepth`, each wiring PR deleting its own allowlist row).

**Sequencing that is not optional.** Five items declared themselves
independent and are not:

- **W-CLI → W-EFFECTS.** Both replace `check.go`'s hand-rolled flag loop.
  W-CLI owns the swap; W-EFFECTS registers onto the FlagSet it built.
- **W-TEST → W-SEED.** Both rewrite `runFile`'s signature in `test.go`.
- **W-DESCRIBE-JSON → W-CODES.** The code lookup is a sixth `kind` in
  describe's JSON schema.
- **W-PBT-GUARD → W-TEST → W-PBT-REPORT.** All three touch `runCheckProp`
  and `report()`.
- **The kg mutex.** `make -C kg verify` is live CI and CLI.md plus 23
  design docs are digested inputs. Five items rebuild the bundle. **Never
  two kg-rebuilding PRs in flight** — make "does this change a kg-digested
  input?" an explicit per-item field, or CI red-flags it late.
- **NUR ids.** Nine items each want "the next free id". Assign a block per
  item at wave start or the first two to merge collide.

Wave 2 is **serialization-bound, not effort-bound.** With one reviewer and
those chains, wall clock is the longest chain plus the kg mutex — not
total effort divided by parallelism. W-POLICY-INVENTORY touches neither
`test.go` nor `describe`, so it is the genuinely parallel lane.

---

## 6. Waves 3 and 4 — measure, then decide

**Wave 3 produces numbers, not features.** W-EFFECTS steps 1–4 (an opaque
`core.EffectSet` facet, the alphabet owned by `lang/go/policy`, the
accumulator, the `CheckResult` field) plus its corpus measurement; and the
refinement track's M0 (an obligation ledger — record and report every
*declined* obligation, prove nothing new) and M1 (a constant-interval
domain, whose M1c makes the **already-shipped** index-out-of-range proof
see refined parameters for the first time).

**Done:** two numbers exist that do not exist today — what fraction of the
effect surface a declared facet actually covers, and what fraction of real
refinement obligations an interval domain discharges across `lang/spec`
and `utils/`. Both written down.

**Wave 4 is gated and may never open.** The CLI surfaces for effects, and
comparison-guard narrowing, only if Wave 3's numbers justify them in
writing. If the discharge rate is near zero because real boru code barely
uses cross-expression refinements, **stop** — M0's ledger and M1c's free
index-OOB win still leave the tree better, and the track closes honestly.
That is a success condition, not a failure.

---

## 7. What not to do

**SMT discharge of `refine` predicates — permanently declined.** The Quint
survey ranked this first. Three independent reasons killed it, any one
sufficient, and the third is decisive:

1. Vendored Z3 is C++, therefore cgo, therefore `GOOS=js/wasm` cannot link
   it — and `wpg/wasm` is a real target, so `boru build` would stop being
   pure Go.
2. An SMT-LIB subprocess adds a PATH dependency to a toolchain with
   **zero** `os/exec` in core/check/basic/lang/eng/compiler/parser, and
   makes `boru check` give different verdicts on different machines.
3. **boru refinement bounds are evaluated to constants at definition
   time** — `def m 3  def P (refine Integer gt m)` becomes
   `(Integer gt 3)`. The free-variable relational fragment that SMT exists
   to decide **is not expressible in the surface syntax.**

Reason 3 means the Quint survey's headline recommendation was aimed at a
gap the language cannot currently express. Record the decision; do not
revisit without a language change. *An interval domain over constants —
Wave 3's M1 — captures the reachable value at a fraction of the cost.*

Also declined, each with its reason:

- **Edge-biased default generators.** Silently changing a documented
  uniform draw; already composable in-language.
- **Automatic `ok:false` / any vacuity threshold.** `distinct-inputs: 2`
  over 1000 runs is correct for a Boolean generator. Report, never verdict.
- **`check --show-calls` / REPL `/calls`.** 44,280 dispatch events while
  checking one 1,112-line file; the repo already rejected an unconditional
  per-dispatch string at ~5% of interpreter CPU.
- **An MCP server for `describe`.** The transport is ~120 unexported lines
  inside a credential-brokering command that must stay at 100% coverage,
  and an agent host can already run `boru describe --json`.
- **`mutate` / `system-info` globals.** No defensible binding, and
  deleting them is *unsafe* — four shipped profiles name them and
  validation would then reject those profiles. Allowlist with reasons.
- **A blanket positional-guard sweep of all 30 subcommands.** `fmt`, `do`,
  `test`, `run`, `vault exec` and `serve` are legitimately variadic; a
  guard there is a regression.
- **The "rfc doc cites a dead path" gate.** 734 dangling backticked paths
  across 150 docs, led by pre-module-cut locations that were correct when
  written. Enforcing it means falsifying as-built records. Resolve only
  declared `code:` anchors.

**Design traps that each look like the obvious approach:** no central
name→arity table in the CLI helper (ADR-012 forbids exactly that name
list); no `Test.filter` boru word for a CLI-only concern; no splitting
authored example lines on `;#` (ADR-007); never emit the `"..."` example
sentinel or the name-keyed `inferReturns` guess as a machine-readable
`returns`; no gate inside `EffectiveClock` (it seeds `boru:rand` at
module-build time and would blank every log timestamp); do not flip
`whereKeyMatches`'s vacuous pass in place (that would make a *deny* rule
with a where on an unmeasured arg start matching everything — close it at
profile-compile time instead); do not build the enforcement gate as a
`pol.Check` call-site scan (only 10 `PolicyRefusal` call expressions
exist, 8 passing a variable).

---

## 8. Cost

One S, seven M, three L as specced — but almost every scoping pass
independently reported that **the driver is ADR-008, not the code.** The
changes are small and local; pairing a positive and a negative test across
the affected packages is what turns an S into an M.

| Wave | Agent build | Maintainer review | Calendar |
|---|---|---|---|
| 0 | — | ~0.5 day (human only) | hours |
| 1 | ~3 weeks | ~1 week | ~2 weeks |
| 2 | ~7 weeks | ~3 weeks | ~6–8 weeks |
| 3 | ~3 weeks | ~1 week | ~2–3 weeks, then a decision |
| 4 | ~4 weeks | ~1.5 weeks | only if measured |

Everything shipping is roughly a quarter of sustained work. That is not
absorbable alongside other development, and pretending otherwise is how a
plan becomes a backlog. The honest ladder:

- **Waves 0 and 1 are the committed scope.** If nothing else ships, every
  verified silent wrong answer on this board is closed.
- **Wave 2 is the intended scope**, one item at a time, §5's chains
  respected.
- **Wave 3 is a gated option** producing two numbers and no features.
- **Wave 4 is a hypothesis** that may never open.

One capacity note: the test-writing tails absorb well by an agent and
poorly by a reviewer, because there is nothing to disagree with — you
either trust the pairing discipline or re-derive it. Batch those reviews;
do not interleave them with behaviour-change reviews.

---

## 9. Corrections to the briefs, and defects recorded in passing

The scoping passes were given briefs drawn from `legacy/QUINT-COMPARISON.0.ignore`.
Three were wrong, and the corrections are kept here rather than quietly
absorbed:

1. **"No subcommand rejects unconsumed positionals" is false.** There are
   30 `NArg()` sites, many strict-arity (`attach`, `model`, `build`, ~10
   in `vault`). The survey's grep tested the literal `NArg() > 1`, which
   nobody writes. *This is the fourth instance in this work of one
   mistake: measuring with an ad-hoc pattern instead of the artefact's own
   rule.* It is the strongest argument the plan has for W-FRONTMATTER and
   the generated inventories.
2. **`core.SeverityFor` already exists** (`core/go/check_state.go:613`,
   exported, "so consumers can tag custom codes"). A scoped step to build
   it was deleted; its two dependents are unblocked today.
3. **`describe`'s return column is not live either.** Found while checking
   the Roc report's placeholder-examples defect:
   `describe add` advertises `Float` for a call `check` correctly types
   `String`, because `native_help.go` uses a name-keyed `inferReturns`
   heuristic instead of the signature's `Returns`. This is why
   W-DESCRIBE-JSON must separate `returns` from `returns_inferred` rather
   than publishing the guess as fact — recorded in
   `legacy/QUINT-COMPARISON.0.ignore` §6.7.

**Latent defects found while scoping, not acted on here.** Each needs its
own record and decision:

- `describeStackTypes` byte-slices UTF-8.
- `Test.summary` counts skipped cases as passed.
- **PBT sub-seeds are `seed + i`**, so base seeds 0 and 1 share 99 of 100
  samples — and this is a *published replay contract*. This is the most
  dangerous of the three: W-SEED's headline is reproducibility, and `-s N`
  invites users to sweep consecutive seeds. **W-SEED should not land
  before this is at least recorded with a decision.**

**One tension to name rather than entrench.** `checkCodeSeverity` is a
~200-entry string-literal table in the *kernel* holding content owned by
layers above it (`micron_name`, `vault_usage`, `net_error`,
`case_not_exhaustive`). `errorcodes.go` states the intended discipline —
"the kernel owns the mechanism and its own codes, and each layer above
registers its own" — and severity does not follow it. W-CODES builds new
public surface on that table. Either consume it without widening its
public contract, or add a layered `RegisterSeverity`. Do not mint an ADR
(the preamble reserves that for maintainer instruction); record it in
`design/`.

---

## 10. The one thing to do first

**Wave 0a: put `make -C utils check test` and `make -C kg check test` into
CI.** Seconds of runtime, no coverage obligation, no workflow-scope
subtlety beyond the edit itself — and it guards the largest body of
boru-written boru in the tree, which is precisely what W-CLI, W-TEST,
W-SEED and W-PBT-REPORT are about to rewrite. Doing it after those items
is doing it too late.

*If the answer must be code rather than CI:* **W-NUR079**, then
**W-POLICY** (iii) → (i) → (ii). NUR079 is a declared security boundary
that does not hold; and until `boru policy explain` types its arguments it
disagrees with the enforcer in both directions, which is the tool every
profile author reaches for the moment the fail-open fix lands.
