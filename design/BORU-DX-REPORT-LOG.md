# boru Developer Experience Report: Implementing `boru:log`

## Context

This report documents the experience of implementing `boru:log` — a
logging / OpenTelemetry-abstraction / provider-hook module — across five
phases (logging, contextual loggers, provider hooks, traces, metrics).
Unlike the `boru:decision` DX report (`BORU-DX-REPORT.5.md`), which was a
*pure-boru* attempt, `boru:log` is a **Go-backed capability module** in the
mould of `boru:io` / `boru:net` / `boru:rand`. So the friction profile here
is complementary: it is the experience of a **Go module author** wiring
native words into the engine, plus a **boru author** writing the `.tsv`
spec rows and driving the CLI — not someone composing fns in pure boru.

The module shipped at ~1,500 lines of Go across five files plus a 40-row
spec and a 12-test Go suite. Everything that was attempted was
buildable — there were no hard blockers of the kind the decision report
hit (def leakage, list auto-eval). The findings below are about
**learnability cliffs and authoring ergonomics**, not missing
capability. Verified against `cmd/go/bin/boru` on branch
`claude/boru-log-module-design-xynnpg`.

## Summary

| # | Finding | Severity | Suggested direction |
|---|---------|----------|---------------------|
| 1 | Newline is not a statement separator; forward-collection silently merges statements | **High** (learnability) | A diagnostic when a dotted word forward-collects across a likely statement boundary; promote `;` in the tutorial's first module example |
| 2 | A reserved token (`end`) cannot be a dotted key/export | Moderate | Treat the post-dot identifier as a literal key even when it is a reserved word; or reject a reserved-named export at registration with a clear error |
| 3 | Stack-form vs forward-form silently re-binds a multi-arg call | Moderate | A "did you mean forward form?" hint when a trailing typed arg goes unmatched; the convention is documented but invisible at the call site |
| 4 | `get` field-access reads right-to-left (`container key get`), the inverse of dotted `m.k` | Moderate | A pipeline-friendly field accessor, or surface the `container key get` idiom in REFERENCE "Maps and access" |
| 5 | Go module per-import state must be closure-captured, not stored on the registry capability | Moderate (Go author) | Document the `rand`-style closure-capture rule in lang/go/CLAUDE.md "Module FnDef Wrappers" |
| 6 | Instance-returning constructors need a check-mode `ReturnsFn` to avoid false positives | Minor (Go author) | Document the `randWithSeedReturns` shape pattern as the canonical recipe |
| 7 | Spec `Canon` rendering quirks (single-quoted strings, `name/q` atoms) | Minor | A one-paragraph "reading Canon output" note next to the spec-runner docs |

The standout positive: the **native-module pattern, capability seams, and
test/ratchet discipline are excellent** — they made a five-signal
observability module a copy-adapt-verify exercise rather than a research
project. Details below.

---

## What worked well

### 1. The native-module pattern is a reliable template

`boru:io`, `boru:net`, and `boru:rand` are near-perfect references. The
shape — build a sub-registry, register natives, wrap each as an FnDef,
return a `ModuleDesc` — is uniform enough that I could read one module
and write another. `makeModuleFnDef` handles multi-signature wrappers; a
new module is "list the natives, map inner names to dotted exports, add a
`docs_<name>.go` table." There was no point where I had to invent
structure.

### 2. Capability seams make host integration uniform and trivial

The three things a logger must touch — the clock (for timestamps), the
output writer, and policy (for gating) — are all reached the same way:

```go
ts  := EffectiveClock(r).Now()      // freezable via SetHostClock for tests
w   := r.ErrOutput                  // where the console sink writes
pol := HostPolicy(r)                // nil ⇒ allow-all; else .Check(...)
```

`EffectiveClock` made the whole module deterministic under a frozen
clock for free — record timestamps, span ids, everything. Adding the new
`CapLogSinks` capability followed the existing `CapClock`/`CapFileOps`
pattern exactly. **This is the best part of the engine's design from a
host-author's seat.**

### 3. The instance/closure pattern composes — I used it three times

`Rand.with-seed` returns an `OrderedMap` of method closures over private
state. I reused that exact shape for **loggers** (`Log.with` → methods
closing over name+attrs), **spans** (`Log.span` → methods over span
state), and **instruments** (`Log.counter` → an `add` method over
instrument state). One pattern, three features, zero new machinery. That
is a sign of a well-factored core.

### 4. Policy extension was one line

Adding a `log` scope was literally appending `"log"` to
`KnownScopes`; `canInstall` then made `install:false` work automatically,
and `pol.Check("log", "emit", …)` / `Installed("log")` did the gating.
A sandbox can now disable telemetry egress without touching anything
else. Clean, orthogonal extension.

### 5. `boru describe` + the docs table keep documentation honest

`registerDocs("boru:log", {...})` plus `TestModuleExportDocs` means every
export *must* have a one-line doc or CI fails — and `boru describe boru:log`
renders the whole surface from live data. I never had to hand-maintain a
separate doc list; the test caught the two exports I forgot.

### 6. The spec runner + ratchets are strong guardrails

A `.tsv` file dropped in `lang/spec/` is auto-discovered and run in
**both** TCO modes, **plus** in check mode by three ratchets
(false-positive, type-soundness, compiler-coverage). These caught real
things: a `flex` in a spec row tripped the compiler-coverage ratchet
(`TestOnlyMetaFallsBack`), and my value-dependent error rows moved the
unflagged-error ratchet — each forcing a *documented* decision rather
than silent drift. Once understood, this is contributor DX done right:
the guardrails explain themselves in their own pin comments.

### 7. No-panic + negative-test discipline produced a robust module

The `recover()`-based type-literal test and the "pair every positive with
a negative" rule are enforced by gates, so robustness wasn't optional. My
`AsConcrete*` guards plus the no-panic test caught two handlers that
would have panicked on a bare type literal.

---

## Friction found

### Finding 1: Newline is not a statement separator (high learnability cost)

**Severity:** High — this cost me the most time, and would hit any new
module author the same way.

Statements must be separated by `;`; a newline is whitespace. Worse, the
symptom is not a parse error — a side-effecting word **forward-collects
across the line break** and silently does the wrong thing:

```boru
import "boru:log"
Log.add-sink memory/q
Log.sinks                # → [console]   ← memory was NOT attached
```
```boru
import "boru:log" ; Log.add-sink memory/q ; Log.sinks   # → [console memory]  ✓
```

The first form *runs clean and returns a plausible-looking result*. I
spent real time convinced `add-sink` had a state bug before discovering
the two statements had merged: `Log.add-sink` forward-collected past
`memory/q` into the next expression. In a different shape the same
over-reach surfaces as the (excellent, but after-the-fact) hint:

```
forward args for get may have run into the next word; group the call in
parens so its RESULT becomes the argument — (get …). `end` / `;` only
ends the statement — it does NOT turn a following word into a nested call.
```

**Why it bites:** every other curly/expression language treats a newline
as at least a soft terminator. A concatenative stack language legitimately
does not — but the *failure mode is a wrong answer, not an error*, which
is the dangerous combination.

**Suggested direction:** a check-time diagnostic when a word
forward-collects across a source line boundary into another statement-like
expression ("`Log.add-sink` collected `Log.sinks` from the next line; did
you mean to end the statement with `;`?"). Failing that, lead the
tutorial's first *multi-statement* example with `;` and call the rule out
explicitly — the current examples are mostly single-statement, so the
cliff is invisible until you write real code.

### Finding 2: A reserved token cannot be a dotted key/export (`end`)

**Severity:** Moderate — silently un-addressable export name.

I wanted `Log.end SPAN`. It is unreachable: `Log.end` lexes as
`Log get end`, and `end` is the statement-terminator token, so the
statement ends with `get` starved of its key:

```boru
{a:1}.end        # error — stack: ( Map >>>word(get)<<< end )
```

The same blocks a `Span.end` *method*. I renamed to `Log.end-span` and
`Span.finish`. The collision is invisible until runtime — nothing at
module-registration time warns that an export named `end` (or any reserved
token: `;`, `end`, …) is dead on arrival.

**Suggested direction:** at the dot-to-get conversion site, the post-dot
identifier is *always* a key — treat a reserved word there as a literal
key (`x.end` → `x get "end"`), consistent with the literal-key semantics
that already fixed Issue 4 of the decision report. If that is undesirable,
reject a reserved-named export when the module is built, with a clear
error, so the author learns at registration rather than at call.

### Finding 3: Stack-form vs forward-form silently re-binds a call

**Severity:** Moderate — a real footgun for multi-arg side-effecting words.

The "forward by default" rule is well documented, but at the *end of a
pipeline* a stack-form call to a 2-arg word binds the args the other way
with no error:

```boru
l.info "req" {path:"/x"}    # forward: body="req", fields={path:"/x"}   ✓
"req" {path:"/x"} l.info    # stack:   the [Any,Map] sig can't match
                            #          ({path} is top → body, "req" → fields:Map ✗),
                            #          falls back to [Any]: body={path}, "req" stranded
```

Because the level word is overloaded (`[Any]` and `[Any, Map]`), the
stack form doesn't error — it matches the *shorter* signature and strands
the message on the stack. I hit this writing my own tests and momentarily
thought the merge logic was broken. For a non-commutative builtin it is
the documented `sub 10 3 = -7` vs `10 sub 3 = 7` distinction; for an
overloaded module word it manifests as a *silently dropped argument*.

**Suggested direction:** when a trailing forward-eligible, type-matching
argument is left unconsumed at a statement boundary next to a word that
has a longer signature it *almost* matched, emit a "unmatched argument —
did you mean forward form `word a b`?" diagnostic in check mode.

### Finding 4: `get` field-access reads inside-out relative to `m.k`

**Severity:** Moderate — constant low-grade friction writing spec rows.

Dotted access reads left-to-right (`m.k.j`), but the pipeline form of the
same access is **right-to-left**: the working idiom is `container key get`,
while the natural-looking `container get key` is the *swap* form and fails:

```boru
Log.dump 0 get "severity-text" get      # ✓  list→[0]→record→["severity-text"]
Log.dump get 0                          # ✗  no matching signature for get
```

Every time I assembled a `Log.dump 0 get "x" get` chain in a spec row I
had to stop and re-derive the order. The dotted form `(Log.dump).0` isn't
available on a runtime value mid-pipeline, so the `get`-chain is the only
option and it inverts the intuition the dot syntax just taught.

**Suggested direction:** either allow dotted access on a parenthesised
runtime value (`(Log.dump 0).severity-text`), or add a pipeline-first
accessor (`Log.dump 0 get  "severity-text" pluck`), and document the
`container key get` idiom prominently in REFERENCE "Maps and access" —
it's the form everyone writing data-processing pipelines needs.

### Finding 5: Module state must be closure-captured, not stored on the registry (Go author)

**Severity:** Moderate — non-obvious, cost me a debugging cycle.

My first design stored the per-import sink registry on the engine
registry via a capability and re-fetched it each call
(`LogSinkRegistryFor(r)`). It didn't persist: `Log.add-sink` wrote to one
instance and `Log.sinks` read a fresh one. The trivial-delegation dispatch
runs the inner native against the live registry, but a capability *write*
from inside that handler did not survive to the next dotted call. The fix
was the `boru:rand` pattern — capture one `*LogSinkRegistry` in the word
closures at module-build time:

```go
lsr := native.NewLogSinkRegistry()
natives := LogModuleNativeFuncs(lsr)   // every handler closes over lsr
```

`rand` does exactly this (its `randState` is captured, not stored), so the
precedent exists — but lang/go/CLAUDE.md's "Module FnDef Wrappers" section
documents *dispatch* (the `BarrierPos: -1` rule) without stating the
**state-ownership** rule: per-import mutable state lives in the closures,
not on the registry.

**Suggested direction:** add a sentence to "Module FnDef Wrappers": *a
module's per-import mutable state must be captured by the native closures
(see `boru:rand`'s `randState`); do not store it under a registry
capability and re-fetch — capability writes from a dot-dispatched handler
are not guaranteed to be visible to the next call.*

### Finding 6: Instance-returning constructors need a check-mode `ReturnsFn` (Go author)

**Severity:** Minor — easy once you know the pattern; opaque until then.

`Log.with` / `Log.span` / `Log.counter` return an `OrderedMap` of method
closures. Without a check-mode `ReturnsFn`, the static checker sees a
shapeless `Map` carrier and a downstream `l.info` can't resolve the method
wrapper — a false positive. The fix is a `ReturnsFn` that builds a
shape-only instance (mirroring `randWithSeedReturns`). It works, but I only
knew to do it because I'd read the `rand` source; nothing flagged the
missing shape.

**Suggested direction:** document `randWithSeedReturns` as the canonical
"constructor that returns a methods-map" recipe in the same CLAUDE.md
section, so instance-returning module words get check-mode precision by
default.

### Finding 7: `Canon` spec-rendering quirks

**Severity:** Minor — a one-time learning step per author.

Spec expected-columns are matched against `eng.Canon`, whose conventions
surprised me row by row: strings render single-quoted (`'http'`, not
`"http"`), atoms render `name/q` (`console/q`), and an empty result is an
empty column. Each was a quick fix-and-retry, but a short "reading Canon
output" table next to the spec-runner docs would remove the guesswork.

---

## Contributor-experience note: the ratchets are good, and a little opaque

Three check-mode ratchets gate every spec addition. They are genuinely
valuable — they forced me to *document* why three `Log.set-level loud/q`
-style rows are runtime-only errors the static checker can't predict, and
why a `flex`-using row trips the compiler-coverage ceiling. Demanding that
documentation is the right behaviour — though the trip itself records a row
that does not compile, which is a defect owed a fix, not a settled state. The friction is purely discovery: a first-time
contributor adding spec rows will see `unflagged error rows rose to 183
(pin 181)` and not immediately know that the correct action is *"raise the
pin with a documented reason"* vs *"fix a regression."* The pin comments
themselves are the best documentation; a one-line pointer to them from the
"Test discipline" section of CLAUDE.md would shorten the learning curve.

---

## Overall assessment

For a **Go-backed capability module**, boru's authoring experience is
strong: the module pattern, capability seams, instance/closure idiom,
policy model, self-documenting `describe`, and the spec/ratchet discipline
turned an ambitious five-signal observability module into a steady
copy-adapt-verify loop with no architectural blockers. The
decision-report era's hard blockers (def leakage, list auto-eval) are
gone, and it shows.

The remaining friction is concentrated in **the boru surface syntax's
silent failure modes** — newline-is-not-a-separator (Finding 1) and
stack-vs-forward re-binding (Finding 3) both produce *wrong answers
instead of errors*, which is the costliest kind of DX paper-cut. Both are
consequences of legitimate concatenative-language design choices; the fix
is not to change the semantics but to **make the failure loud** with a
targeted check-time diagnostic. The `end`-token collision (Finding 2) and
the `get` ordering (Finding 4) are smaller, well-bounded issues with
clear fixes. None block writing real modules — but closing the "silent
wrong answer" gap would be the single highest-leverage DX improvement for
the next person who builds one.
