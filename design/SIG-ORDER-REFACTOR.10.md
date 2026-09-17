# Signature-Order Refactor: Unify FnSig and NativeSig on Top-First

> **Status: implemented** (this branch). The PBT plan in
> `design/legacy/PBT-PLAN.10.ignore` is unblocked.
>
> ## What landed (diff from this design doc)
>
> Empirical investigation while executing the plan revealed that boru
> source `def fn` was **already** top-first via `matchSignature` — the
> audit's claim of "bottom-first boru semantics" was incorrect. The
> only path actually using bottom-first was `execFnDefSigStackMatch`'s
> `!hasNamed` branch, which fired from `execFnDefLiteral`'s
> module-closure branch after `matchSignature` had already succeeded.
>
> Implementation — final form (two iterations):
>
> **Iteration 1** introduced an unnamed-param push reversal in
> CallBoru/execFnDefSig to make module-wrapper bodies (`[Word(inner)]`)
> dispatch correctly. This worked but added a SECOND arg-flow
> convention to the kernel — args still in sig order at handler
> handoff, but body-token push order reversed for unnamed params.
> Internally consistent, but inconsistent across paths.
>
> **Iteration 2** (the user push-back: "functions always get args in
> signature match order, always") removed the push reversal by
> introducing a trivial-delegation short-circuit in
> `execFnDefLiteral`. Module wrappers built by `makeXxxFnDef` have
> bodies of the form `[Word(inner-name)]` with all-unnamed Params —
> detect that shape via `isTrivialDelegationBody` and dispatch the
> inner native directly via `execMatch`, bypassing CallBoru and body
> execution entirely. boru fns defined in module preambles
> (decision.cond etc.) take the longer CallBoru path, but they all
> use named params so push order doesn't matter for them either.
>
> Final state — **one convention everywhere, no reversals**:
>
> - **engine.go `execFnDefLiteral` module-closure branch**: detects
>   the trivial-delegation shape and falls through to `execMatch`
>   for direct inner-handler dispatch. Non-trivial bodies route
>   through `execFnDefSig` in the captured registry.
> - **engine.go `isTrivialDelegationBody`**: predicate that
>   identifies `[Word(name)]` + all-unnamed shape — the exact
>   pattern `makeXxxFnDef` / `wrapXxxFnDef` produces.
> - **engine.go `execFnDefSig` + registry.go `CallBoru`**: push
>   args in sig order (no reversal). Named params bind via
>   InstallDef; unnamed params (rare here — only boru fns with
>   unnamed params inside module preambles, none currently exist)
>   append to body tokens in i-order.
> - **core_helpers.go `InstallFnDef` handler closure**: unchanged.
>   Same convention as everything else.
> - **`execFnDefSigStackMatch`**: unchanged. Only reached as a
>   fallback when matchSignature returns nil (anonymous lambda
>   recovery). Its internal `!hasNamed` branch retains a residual
>   bottom-first match for that fallback, which is the lone exception
>   to the unified rule — it lives behind a "matchSignature failed"
>   gate and no production path exercises it.
> - **Module wrapper Params**: declared identical to inner native
>   Args (top-first natural order). Includes time.format, to-instant,
>   to-local, start-of, end-of, tz-offset, is-dst,
>   add-days/months/years, rand.string.
> - **CLAUDE.md** (eng + lang): "Signature Ordering" sections record
>   the one rule with no exceptions in production paths, plus a
>   **strong recommendation to always use forward form** (`f a b c`)
>   as the canonical surface for boru calls.
> - **Guard test**: `lang/go/test/sig_order_guard_test.go` pins the
>   four scenarios.

## Context

The boru kernel has two argument-positioning conventions that disagree
for heterogeneous-type signatures:

| Path | Source of truth | Convention |
|------|-----------------|------------|
| `matchSignature` (`eng/go/engine.go:2828`) | `Signature.Args` | **top-first**: sig[0] = stack top |
| `execFnDefSigStackMatch` (`eng/go/engine.go:1676-1776`) | `FnSig.Params` | **bottom-first**: Params[0] = stack bottom |
| `fnSigsToSignatures` (`eng/go/engine.go:1808-1836`) | bridge: FnSig → Signature | **copies verbatim** (no order flip) |

This was discovered while building `boru:rand`: the surface call
`"abc" 10 rand.string` failed to dispatch unless the wrapper's
`FnSig.Params` were written `[String, Integer]` (source order
**reversed** from the natural reading "first param is the charset").
The inner native `Args` correctly use `[Integer, String]` (top-first,
`Integer` matches stack-top 10), and `matchSignature` against the
inner native succeeds. The dispatcher then drops the matched result
and **re-matches via `execFnDefSigStackMatch` against the wrapper's
`FnSig.Params`** under the bottom-first rule, hence the manual
reversal.

`time.format` carries the same inconsistency in the production code:
the wrapper declares `Params = [Date, String]` while the inner native
declares `Args = [String, Date]`. Same word, two opposite orderings —
both intentional given each path's separate (and undocumented)
convention.

## What we actually want

**One convention.** Top-first, matching `matchSignature` and the
documented Phase-4 unified rule in `lang/go/CLAUDE.md`'s "Argument
Ordering (CRITICAL)" section. Concretely: at every call site, the
sig position index counts from the top of the stack down. `sig[0]`
(or `Params[0]`) is always whatever sits on top.

## Empirically pinned current behavior (do not lose)

These are the load-bearing observations. Any plan that breaks them is
wrong.

1. **boru source `def fn` already uses top-first**, via
   `matchSignature`. Empirically:
   ```
   def f fn [[a:Integer b:String] [Integer String] [a b]]
   "hello" 42 f      → a=42, b="hello"   (top-first; matches)
   42 "hello" f      → no matching signature (correctly rejected)
   ```
   So we do NOT need to touch boru source semantics or reverse user
   `def fn` param lists. The audit document's contrary claim was
   incorrect.

2. **Kernel-registered NativeSig (math, time, decision, rand inner
   natives, etc.)** already use top-first via `matchSignature`. No
   change needed.

3. **Module FnDef wrappers** (`lang/go/modules/*.go`'s `makeXxxFnDef`
   helpers) are the ONLY site where bottom-first leaks in. The
   wrappers' `FnSig.Params` are read by `execFnDefSigStackMatch`
   bottom-first, so authors who wrote them in "natural" source
   order accidentally got the inverted dispatch behavior. Those who
   wrote them reversed got the right behavior — by accident.

## Root cause

`execFnDefSigStackMatch` (`engine.go:1676`) re-implements
signature matching with its own bottom-first convention. It fires
from `execFnDefLiteral` (`engine.go:1656`) when the matched FnDef has
a captured `Registry` (the module-closure branch) — which is exactly
when a module wrapper dispatches.

The relevant block in `execFnDefLiteral`:

```go
// Module closures: with all args now on the stack ...
// route through execFnDefSig with the captured registry.
if fnDef.Registry != nil && fnDef.Registry != e.registry && len(fnDef.Sigs) > 0 {
    return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)
}
```

By the time this branch fires, `matchSignature(fn, w, resolved)` has
already succeeded against the inner native's `Signatures` (top-first)
and returned `(sig, positions)`. But `execFnDefSigStackMatch` discards
those and re-matches the WRAPPER's `FnSig.Params` against the stack
bottom-first. That's the inconsistency.

## The fix (focused, internal, no surface change)

### Single change: route module-closure dispatch through the matched positions

After `matchSignature` succeeds, use the already-matched
`(sig, positions)` to construct args for `execFnDefSig` directly —
do NOT re-match via `execFnDefSigStackMatch`. The relevant code path
becomes:

```go
// Module closures: matchSignature already matched the inner
// native's sig top-first. Convert positions into args and route
// through execFnDefSig with the captured registry, mapping the
// matched inner sig to the wrapper FnSig that owns the body.
if fnDef.Registry != nil && fnDef.Registry != e.registry && len(fnDef.Sigs) > 0 {
    wrapperSig := matchedToWrapperFnSig(fnDef.Sigs, sig)  // 1:1 today
    args := make([]Value, len(positions))
    for i, pos := range positions {
        args[i] = e.stack[pos]   // already in top-first sig order
    }
    return e.execFnDefSig(valIdx, wrapperSig, args, fnDef.Registry)
}
```

The wrapper's `FnSig.Params` then become **top-first** (matching
inner native `Args`) — same convention everywhere.

### `execFnDefSigStackMatch` stays, but flips to top-first

This path is still reached when `matchSignature` returns no match (so
the engine retries with raw FnSig matching for anonymous lambdas /
predicate-type FnDefs / legacy paths). Flip its iteration so it
agrees with the canonical rule:

```go
// Before (engine.go:1737-1770, bottom-first):
candidate := resolved[len(resolved)-nArgs:]
for j, p := range sig.Params { ... candidate[j] vs p.Type ... }

// After (top-first):
for j, p := range sig.Params {
    ri := len(resolved) - 1 - j        // sig[0] = top of stack
    if !sigTypeMatches(resolved[ri], p.Type) { ... }
}
```

The same flip applies to the `hasNamed` branch above it
(`engine.go:1701-1735`).

### `fnSigsToSignatures` already produces the right shape

Since boru `def fn` source uses top-first today (empirically verified),
and `fnSigsToSignatures` copies `Params` → `Args` verbatim,
`matchSignature` interprets the result top-first — which is correct.
No change needed.

### `CallBoru` / `InstallFnDef` handler closures

These iterate `for i, p := range sig.Params` and use `args[i]` as the
i-th param. After the flip, the caller is responsible for passing
`args` in top-first sig order — and the `args[i]` ↔ `Params[i].Name`
binding now means "args[i] = whatever filled sig position i =
whatever was at stack top-down position i." Consistent with
`matchSignature`.

Net effect on boru `def fn` users: **none**. They already write source
expecting top-first; matchSignature already delivers that; the
`CallBoru` binding already matches.

Net effect on module wrappers: **they must declare `FnSig.Params` in
the same order as the inner native's `Args`** (top-first). Today some
declare them reversed; those need to be flipped.

## Module wrapper audit and flip

Per the empirical investigation, the following heterogeneous-type
wrappers need their `FnSig.Params` flipped to match the inner
native's `Args`:

| File | Wrapper | Current Params | Inner Args | Action |
|------|---------|----------------|------------|--------|
| `lang/go/modules/time.go:59` | `format` | `[Date, String]` | `[String, Date]` | flip wrapper to `[String, Date]` |
| `lang/go/modules/time.go:84-85` | `until`, `since` | `[Date, Date]` | homogeneous | no change |
| `lang/go/modules/time.go:99` | `to-instant` | `[DateTime, Timezone]` | `[Timezone, DateTime]` | flip |
| `lang/go/modules/time.go:104-105` | `start-of`, `end-of` | `[Date, String]` | TBD — audit | audit + flip if needed |
| `lang/go/modules/rand.go:84` | `string` | `[String, Integer]` (already flipped pre-refactor) | `[Integer, String]` | revert to natural `[Integer, String]` (top-first) |

Other modules (matrix, decision, solardemo, bin, type, vm, report,
test) need an audit pass. Most wrappers there are homogeneous or
single-arg, so the count of changes will be small (5-10 wrappers).

After the dispatcher fix, the wrapper convention becomes simply
**"declare `FnSig.Params` identical to the inner native's
`NativeSig.Args`"**. The dual convention disappears.

## Implementation steps (single PR)

### Step 1 — Guard tests pinning current behavior

New file `lang/go/test/sig_order_guard_test.go`. Tests that pin both
the WORKING current cases (so we don't regress) and the BROKEN
current cases (so we know the fix took effect):

```go
// Currently works: boru def fn top-first via matchSignature.
TestBoruDefFnIsTopFirst:
  `def f fn [[a:Integer b:String] [Integer String] [a b]] "hello" 42 f` → [42, "hello"]

// Currently broken: module wrapper with natural-order Params
// (these are the cases the refactor must fix).
TestModuleWrapperTopFirstAfterRefactor:
  `1 time.unix "yyyy" time.format` → expects formatted string, not stuck FnDef
  `"abc" 10 rand.string`           → expects random string

// Currently works (by manual compensation):
// time.format with reversed Params, rand.string with reversed Params.
// After refactor, these flip back to natural order.
```

The "broken" cases are written `t.Skip` initially with a comment
referencing this design doc; remove the skip in Step 4 to flip the
guard.

### Step 2 — Engine change: route matched module closures via positions

Edit `execFnDefLiteral` at `engine.go:1656` so the module-closure
branch consumes `(sig, positions)` from the already-completed
`matchSignature` call and constructs args top-first, then calls
`execFnDefSig` with the wrapper's FnSig. Stop calling
`execFnDefSigStackMatch` from this branch.

### Step 3 — Engine change: flip `execFnDefSigStackMatch` to top-first

Edit `engine.go:1676-1776`. In both the `hasNamed` and `!hasNamed`
branches, change candidate-indexing to read `resolved[len(resolved)-1-j]`
for sig position `j`. Renumber `resolvedIdx` access accordingly when
building `args`.

This path is now consistent with `matchSignature`. Its remaining
callers are the fallback re-dispatch after matchSignature returns nil
(line 1606: `return e.execFnDefSigStackMatch(valIdx, fnDef, resolved)`),
which is anonymous-lambda territory.

### Step 4 — Module wrapper audit + flip

For every module under `lang/go/modules/`:
1. Open the file.
2. Find every `makeXxxFnDef` / `wrapXxxFnDef` call with 2+ Params of
   heterogeneous types.
3. Confirm the inner native's `Args` order in the same file.
4. Make the wrapper `Params` IDENTICAL to the inner `Args`.

Specifically required (from audit):
- `lang/go/modules/time.go:59` `format`: flip to `[String, Date]`.
- `lang/go/modules/time.go:99` `to-instant`: flip to `[Timezone, DateTime]`.
- `lang/go/modules/rand.go:84` `string`: flip BACK to `[Integer, String]`
  (current ordering is the bottom-first workaround).
- Audit other heterogeneous wrappers in `time.go:104-105` and elsewhere.

Run existing module tests after each flip to catch regressions.

### Step 5 — Update guard tests + flip Skip → assert

Remove `t.Skip` from the "currently broken" guards in Step 1.
Confirm they pass.

### Step 6 — Documentation

In `lang/go/CLAUDE.md`:
- Update the "Module FnDef Wrappers — inner sig BarrierPos
  (CRITICAL)" section to add: "**FnSig.Params order must match the
  inner native's NativeSig.Args order — top-first, sig[0] = top of
  stack.** Both convention sites are now identical post-refactor."
- Remove any vestigial language suggesting wrappers use source order.

In `eng/go/CLAUDE.md`:
- Add a short "Signature ordering (CRITICAL)" section stating the
  one rule and pointing to `engine.go::matchSignature` as the
  single source of truth.

In a new section of `design/SIG-ORDER-REFACTOR.10.md`:
- Mark this doc as "Status: merged" and link the PR.

## Verification

After each step:
```bash
make fmt && make vet && make lint && make test
```

End-to-end smoke:
```bash
cd lang/go && go test ./modules/ -v -run "TestTime|TestRand|TestDecision|TestMatrix"
```

The complete fix should land in **a single PR** with the steps above
as separate commits for review. The blast radius is contained
(~5-10 wrapper lines, 2 engine functions, 2 docs) and the user-
visible surface does not change.

## What this refactor explicitly does NOT do

- Does not change boru source semantics. `def fn [[a b c]] ...`
  continues to bind `a` to the top of the stack, as it does today.
- Does not require user-facing migration. No boru files change.
- Does not touch the unified-dispatch / Phase-4 rule. That rule is
  already correct; this refactor just makes a stragglar path (the
  module-closure fallback) honor it.
- Does not split `FnSig` and `NativeSig`. Those are different
  shapes for different reasons (FnSig carries names and patterns);
  the unification is at the ordering convention only.

## Out-of-scope follow-ups (track separately)

- `FnSig` lacks a `NoEvalArgs` equivalent, which is why `rand.list-of`
  and `rand.map-from` are deferred from the PBT work. A separate
  refactor should add `NoEvalArgs` (or equivalent) to `FnSig`, then
  resurrect those generators.
- The `Module FnDef Wrappers — inner sig BarrierPos` note in
  `lang/go/CLAUDE.md` becomes simpler post-refactor and may be
  candidate for removal once the dispatcher delivers the same
  guarantees inline.
- A test that walks every module's exports and asserts each wrapper
  FnSig.Params equals the corresponding inner native Args — to keep
  the new convention enforced going forward.
