# Module-fn body compilation: Direction A is falsified — the gate failure is a grab-bag of checker imprecisions, not one optimism lever

Status: **`.3` — Direction A attempted and empirically falsified.** This
note records the measured result of implementing `.2`'s recommended
"Direction A" (make the checker admit `Any` to typed parameters) and
running it against the real decision gate. It changes `.2`'s
recommendation: Direction A does **not** move the gate, because the
decision module's bodies fail check-analysis for *many independent*
reasons, only one of which is `Any`-vs-typed-param. Read `.0`/`.1`/`.2`
first.

No code from this pass was landed; the §5a memo-key commit
(`6665be3`) remains the only engine change on the branch.

---

## 1. What was implemented (Direction A)

Per `.2` §4-A, the single principled lever: in `signature.go::sigTypeMatches`,
treat a strict `Carrier{Any}` as the gradual unknown — match it
optimistically (the not-disjoint rule) exactly like an explicitly `Dynamic`
carrier and a declared-`Any` return:

```go
if v.Dynamic || (v.Carrier && v.Parent != nil && v.Parent.Equal(TAny)) {
    bound := v; bound.Dynamic = false
    return !isNeverShape(TandValues(bound, NewCarrier(t)))
}
```

This is sound and principled (under gradual typing `Any` *is* the dynamic
type), and it does reduce errors — but nowhere near enough.

## 2. The measurements (this is the result)

Two probes: `check decision.boru` **directly** (trigger 1 — construction-time
analysis), and `check decision_smoke_test.boru` with a value-threading probe
that runs module-fn bodies in check mode (trigger 2 — the actual §5b gate
failure).

| Scenario | Baseline (§5a) | + Direction A |
|---|---|---|
| `check decision.boru` directly (trigger 1) | 39 errors | **35 errors** |
| `check decision_smoke_test` + threading probe (trigger 2 = the gate) | 29 errors | **29 errors** |

**Direction A removes 4 of 39 errors on the direct check and ZERO of the
29 on the gate.** The premise of `.2` Direction A — that checker optimism
would let module bodies analyse cleanly — is false for this module.

## 3. Why: the trigger-2 failures are independent checker gaps

Instrumenting the `no_signature` emission under the threading probe gives the
exact arg shapes. The 29 errors are a cascade from a handful of *distinct*
imprecisions, none of which is the `Any→typed` case Direction A fixes:

| Failing call | Observed args | Real cause |
|---|---|---|
| `eval-pred-all/any/not` | `[Map, Atom(dynamic)]` | `children = quote (pred get "children")` — the checker types **`quote (paren-expr)` as `Atom`**, but at runtime the paren evaluates to a `List`. `Atom` is *provably disjoint* from `List`, so even optimistic matching correctly rejects it. |
| `all` / `any` | `[Map(carrier)]` | the `(children each […]) all` result is typed `Map`, not `List` — `each`-result element/shape typing over the mis-typed `children`. |
| `make` | `[Atom, Map]` | a bare type-name atom where the `make` type slot wants a type literal. |
| `get` | `[Atom(dynamic), Word]` | an **unresolved bare `Word`** reaches `get` as the receiver. |
| `eval-cond` | `[Atom(dynamic), Map]` | same `quote`-as-`Atom` mis-model feeding `c`. |

The dominant one is **`quote` modelling**: `quote (expr)` is typed as the
word-quoting result (`Atom`) regardless of what `expr` produces. Fixing it
means teaching the checker that `quote` over a paren/expression carries the
expression's *result* type, not `Atom` — a real change to `quote`'s
check-mode return modelling, unrelated to `CheckState` ownership or to
`Any` optimism. Behind it sit `each`-result typing, `make`-arg typing, and
bare-`Word`-receiver resolution — each its own fix.

**There is no single lever.** Direction A was the cheapest plausible one and
it does not work. Closing the gate via "make the checker analyse these
bodies cleanly" is an open-ended, multi-front checker-precision program, and
the decision module was never written to type-check under it — its own
direct `check` carries 39 errors at baseline; it is correct only because it
is **trusted/black-boxed on import** (the gate checks the importing suites,
where construction-time body analysis is not surfaced, hence 0 errors).

## 4. Revised recommendation: Direction B (trust imported bodies)

The empirical result re-points the design at `.2` §4-**B**: do **not** surface
diagnostics from an imported module's fn bodies as parent errors. The
module's contract is exactly "trust it; it was checked by its author" — and
its 39 baseline direct-check errors show it is *meant* to be black-boxed,
not deeply re-analysed by every importer.

Concretely:

- §5b still threads the pass into the module body so the body is **recorded
  for compilation** (no concrete-fold, no side-effect leak), BUT
- diagnostics emitted while analysing a body whose registry ≠ the
  top-level pass are tagged (a `ModuleBodyDepth` on the shared `CheckState`,
  incremented in the threading block) and **dropped/downgraded at end of
  pass** — the same shape as the existing
  `FnBody`/`RescueForwardRefDiagnostics` mechanism, widened from
  `undefined_word` to the imported-body case.
- The remaining guard is then **`--force-compile`'s `bytecode == interp`**
  check: a suppressed-but-imprecise analysis that bakes wrong bytecode shows
  up as DIVERGES, not as a diagnostic. So Direction B's risk is concentrated
  there and must be verified per compilable suite — which the decision gate
  already does (`decision_unit_test`, `decision_smoke_test`).

This matches reality (imported modules are trusted), is bounded (one
diagnostic-scoping mechanism, not N checker fixes), and keeps ordinary
in-program checking untouched.

### Open question for Direction B (the real risk)

Does the §5b-recorded module body, with its imprecise carriers (the
`quote`-as-`Atom`, the `each`-as-`Map`), compile to bytecode that still
matches the interpreter? If the imprecision only affects *diagnostics* (the
`assuming best-fit candidate` recovery still records a runnable call), the
compiled output may match and Direction B works. If the imprecision corrupts
the recorded carriers enough to bake wrong ops, Direction B fails the
`bytecode == interp` check and the `quote`/`each` typing must be fixed after
all (a scoped subset of Direction A, driven by actual divergences rather
than by zeroing the diagnostic count). **This is the first thing to measure
when starting Direction B**: re-apply the §5b threading + diagnostic
suppression and run `--force-compile` on the two compilable suites; a green
`bytecode == interp` there is the go/no-go.

## 5. Status / sequence

1. **§5a registry-discriminated memo keys** — *landed* (`6665be3`).
2. ~~Direction A (checker admits `Any` to typed params)~~ — **attempted,
   falsified (§2-3). Not landed.** Useful only as a tiny independent
   precision nudge (39→35 on the ungated direct check); it does not serve
   the gate and was reverted to keep the branch honest.
3. **Direction B (suppress imported-body diagnostics) + §5b threading** —
   the re-pointed next step. Gate it from commit one on
   `--force-compile`'s `bytecode == interp` for `decision_unit_test` and
   `decision_smoke_test` (§4 open question).

## 6. How to reproduce these measurements

```bash
git clone https://github.com/voxgig-boru/decision /tmp/decision
cd /home/user/boru/cmd/go && go build -o bin/boru ./boru
# trigger 1 (direct): error count
./bin/boru check /tmp/decision/decision.boru 2>&1 | grep -oE '[0-9]+ error\(s\)'
# trigger 2 (gate): needs the §5b threading (value-thread capturedReg.Check =
# e.registry.Check around the CallBoru in execFnDefSig when the parent is
# check-active) to run module bodies in check mode, then:
./bin/boru check /tmp/decision/test/decision_smoke_test.boru
```
