# §6 scoping: the blocker is one checker root cause — absent-key `get` over example-map params reads `None`

Status: **`.5` — §6 scoped; it is NOT a bytecode-compiler task.** The
test-framework bodies refuse to compile for the SAME reason the module's
own `check` carries errors: a single, measurable checker-precision root
cause. Fixing it removes **18 of decision.boru's 39** baseline check errors
in one principled change — the largest lever found across `.2`–`.5`
(Direction A removed 4). Read `.0`–`.4` first. No code landed; the branch
keeps §5a (`6665be3`) + the `.0`–`.5` record.

---

## 1. §6 as framed in `.0` is a misdiagnosis

`.0` §6 said the framework's code-body words (`test-test`, `test-describe`,
`run-cases`' computed `for`) must "compile as closures past the Stage-2
code-body limit" — i.e. a bytecode-compiler-capability task. They do not.

Instrumenting the refusal (`MarkUncompilable`) while the module bodies are
threaded into the check pass (so their results are carriers, not concrete
folds) shows the test bodies refuse via:

```
[uncompilable] unmatched dispatch recovered at eval-pred-all   (×3)
[uncompilable] unmatched dispatch recovered at eval-pred-any   (×3)
[uncompilable] unmatched dispatch recovered at get             (×3)
[uncompilable] unmatched dispatch recovered at make            (×2)
[uncompilable] unmatched dispatch recovered at eval-cond / all / any / …
[uncompilable] code-body word test-test (Stage 2)              (×1)
```

`unmatched dispatch recovered` is the check-mode `no_signature` recovery
(`engine.go:6201`): a dispatch inside the trusted module body matched no
signature, so the recorder marks the program uncompilable. The `(Stage 2)`
line is the *outer* `test-test` refusing **because its body refused** — not a
closure-compilation limit. The framework words already declare `CallableSpec`
and compile fine; what fails is the dispatches **inside** the module bodies
they invoke.

## 2. Root cause: example-map params + absent-key → `None`

Tracing one failing dispatch to ground (`(m get "xs") all`, reproduced
standalone with no decision module):

```boru
def g fn [[m:Map] [Any] [(m get "xs") all]]   # check → no_signature for all
```

The chain:

1. At install time a generic fn body is analysed with **synthesised example
   values** for its params (the dynamic-help example generator —
   `lang/go/native/native_definition.go:626`). The example value for a `Map`
   param is the fixed-key literal **`{a:1,b:2}`** (`help/help.go:402`).
2. `m get "xs"` looks up `xs` in `{a:1,b:2}` — **absent** — so
   `getNodeReturns` (`native_storage.go:421`) returns a **`None` carrier**
   ("statically-absent key reads as None").
3. `None` is *provably disjoint* from `List`/`Map`/`Boolean`/…, so the next
   dispatch (`all`, `each`, `eval-pred-*`, `make`) matches no signature →
   `no_signature` → uncompilable.

So an abstract `Map` (or `List`) parameter is modelled by a concrete sample
with **known, finite keys**, and every access to a key the sample lacks
collapses to `None`. The decision module's fns read caller-supplied keys
(`c.field`, `pred get "children"`, `input.(…)`, `c.op`, `c.value`) that are
never in `{a:1,b:2}` / `{c:3,d:4}` — so each read poisons the rest of the
body. This is the true source of decision.boru's **39 baseline direct-check
errors** (`.3` §1) and the §5b/§6 cascade.

### `.3`'s attribution was wrong

`.3` §3 blamed `quote (paren-expr)` being typed `Atom`. The trace shows the
carrier is actually a **`None`** (and elsewhere `dynamic(Any)`) carrier from
absent-key `get`, not `Atom` from `quote` — removing `quote` from the repro
reproduces the failure unchanged. `quote` is identity here; `get`-over-
example-map is the mechanism.

## 3. The measured lever (39 → 21)

One-line experiment in `getNodeReturns`: return `dynamic(Any)` instead of
`None` for an absent key (the gradual-unknown reading — an abstract param's
key *might* be present at runtime):

| decision.boru direct `check` | errors |
|---|---|
| baseline (§5a) | **39** |
| absent-key → `dynamic(Any)` | **21** |

**18 of 39 removed** — vs Direction A's 4. The remaining 21 are the *next*
layer: user-fn dispatch (`eval-pred` recursion, `eval-table-*`, `find-node`)
over `dynamic(Any)`/`Any` carriers — i.e. admitting a gradual carrier to a
typed **user-fn** param + joining reachable-overload returns (a scoped,
principled Direction-A variant, now driven by real refusals rather than by
zeroing a count).

## 4. The sound fix (not the experiment)

Returning `dynamic(Any)` unconditionally is **unsound for real literal maps**
— `{a:1} get "b"` must stay statically `None` (the `unpack`-default and
None-propagation contracts depend on it). The distinction is **provenance**:

- a **real concrete literal** with fully-known keys → absent key is `None`;
- a **synthetic example / abstract carrier** standing in for an arbitrary
  parameter → absent key is `dynamic(Any)`.

Two ways to make the provenance legible:

- **(A) Represent abstract Map/List params as carriers, not examples.** The
  install-time generic analysis would bind `m:Map` to `NewCarrier(TMap)`
  (non-concrete). `getNodeReturns` already returns `dynamic(Any)` for a
  non-concrete container (`native_storage.go:413-414`) — *no change to `get`
  needed*. Risk: the example generator exists precisely because some analyses
  want concrete sample values; this must not regress dynamic-help output (the
  examples are still fine for help; only the **check/compile** body analysis
  should use carriers).
- **(B) Tag the synthetic example map** so `getNodeReturns` returns
  `dynamic(Any)` for an absent key on an example-sourced map but `None` on a
  genuine literal. More local to `get`, but threads a provenance flag through
  the value.

(A) is the cleaner of the two — it fixes the cause (the representation), not
the symptom (one accessor) — and it naturally covers `List` params and every
other accessor, not just `get`. It should be measured the same way: the
decision direct-check error count and, under the §5b threading probe, the
`unmatched dispatch` refusal count on the two compilable suites.

## 5. Revised plan

1. **§5a memo keys** — *landed* (`6665be3`).
2. **§6a — abstract-param representation (this note's root cause).** Make
   generic/abstract fn-body analysis use Map/List **carriers**, so absent-key
   `get` is `dynamic(Any)`, not `None`. Target: decision.boru direct check
   39 → ~21; verify no regression in the in-repo corpus + dynamic-help.
3. **§6b — gradual carrier into typed user-fn params + reachable-return
   join** (the remaining 21: `eval-pred` recursion, `eval-table-*`). A scoped
   Direction-A variant, driven by the residual refusals.
4. **§5b threading + Direction-B diagnostic policy** (`.4`) — now that the
   bodies analyse cleanly, thread the pass in and suppress any residual
   imported-body diagnostics; gate on `bytecode == interp`.
5. **`evCallUser` promotion** (`.0` §7) — last.

§6 is therefore a **checker-precision** workstream (steps 2–3), not a
bytecode-compiler one. The compiler refuses only because the checker can't
resolve the dispatches; fix the resolution and the existing closure path
compiles the framework bodies.

## 6. Reproduction

```bash
cd /home/user/boru/cmd/go && go build -o bin/boru ./boru
# the standalone root-cause repro:
printf 'def g fn [[m:Map] [Any] [(m get "xs") all]]\ng {xs:[1 2 3]}\n' > /tmp/t.boru
./bin/boru check /tmp/t.boru          # → no_signature for all
# the lever (experimental getNodeReturns: absent key → NewDynamicCarrier(TAny)):
#   decision.boru direct check 39 → 21 errors.
```
