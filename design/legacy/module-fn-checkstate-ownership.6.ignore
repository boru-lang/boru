# §6a-A attempted: the fix is real but ENTANGLED with the dynamic-help eval's dual role

Status: **`.6` — §6a-A (abstract-param carriers) implemented three ways; all
break the compilation corpus.** The `.5` root cause is confirmed and the fix
*works* for the target (decision.boru direct check 39 → 0–3), but it cannot be
landed in isolation: the dynamic-help synthetic example eval's diagnostics are
**load-bearing** for both construction-time checking and the bytecode
compilation-coverage corpus. No code landed; the branch stays at §5a
(`6665be3`) + the `.0`–`.6` record.

Read `.5` first.

---

## 1. Confirmed: the source is the dynamic-help example eval, not the checker

`.5` located the cause as `get` over an example map (`{a:1,b:2}`) returning
`None` for an absent key. Tracing the trigger to ground: the analysis that
emits decision.boru's 39 direct-check errors is the **dynamic-help example
generator** (`native_help.go::EnableDynamicHelp` → `makeDynamicEval` →
`GenerateDynamicExamples`). It fires from `OnRegisterHook` for every fn
registered after `MarkReady`, runs `f <example-args>` through the body **in
check mode** (sharing the registry), and the body's dispatch failures against
the synthetic `{a:1,b:2}` stand-in become check diagnostics. `Suspend()`
stops only the bytecode RECORDING, not the diagnostics.

So decision.aml's 39 "errors" are entirely synthetic-example artifacts — the
module type-checks fine against real arguments (which is why importing it
leaks **zero** diagnostics: `check decision_smoke_test.boru` = 0 errors, the
gate stays green). The §6a-A principle — an abstract `Map` param is a carrier,
not the literal `{a:1,b:2}` — is correct.

## 2. Three implementations, each measured

| Approach | decision direct check | what broke |
|---|---|---|
| **(a) carrier construction analysis** in `InstallFnDef` (the literal §6a-A: analyse every fn body at construction with `NewCarrier(param)` args) | 39 → 22 | recursion cascade (`fact` unbound pre-binding → `mul` no_sig); moved post-binding → fixed that, but the carrier-arg summaries collide with the compile path's `genArgs` key → **langspec coverage regressed** (TestOnlyMetaFallsBack/CompiledStatus/CompiledCoverage) |
| **(b) drop ALL help-eval diagnostics** (isolate the eval fully) | 39 → 0 | coverage-SAFE, but the two construction-check tests that rely on the eval's side-channel fail: `TestCheckUncalledFnBodyTypoStillFlagged` (uncalled fn body typo), `TestForwardStrandAdvisory` (in-body strand) |
| **(c) drop only the INPUT-DEPENDENT codes** (`no_signature`, `uncalled_function`, `branch_error`, `fn_body_error`; keep `undefined_word` / `forward_strands_operand` / `unbound_param`) | 39 → 3 | the construction-check tests pass, but **coverage regresses again**: refusals 30 > ceiling 26, and a spec row changes behavior ("Assert.throws: body did not throw") |

## 3. Why it is entangled — two load-bearing roles

The dynamic-help eval is doing **two jobs at once**, and the corpus depends on
both:

1. **Documentation** (its intended job): produce example output for help text.
2. **Construction-time body checking** (accidental side-channel): because it
   runs each fn body in check mode at registration, its diagnostics are the
   *only* thing that checks a fn that is **defined but never called**. Two
   tests pin this — an uncalled fn's body typo (`zzyzx`) and a fn-body
   forward-strand advisory.

And its diagnostics **gate compilation**: `CompileCheck` refuses a program
with any error-severity diagnostic. So a help-eval error pushes a langspec row
from "compiled" to "check-error"; removing it pushes the row forward to a real
compile **refusal** instead — moving 4 rows past the refusal ceiling. The
compilation-coverage corpus (`test/go/langspec`) is calibrated to the EXACT
diagnostic set the eval currently produces. "Keep all" (clean) and "drop all"
(b) are both internally consistent; any **partial** set (a, c) reclassifies
rows and even changes runtime behavior.

The behavioral change in (c) — a row that should throw no longer does — is the
red line: partial suppression is not merely a coverage-ceiling recalibration,
it alters what compiles and how it runs. So (c) is unsafe, (a) regresses
coverage, and (b) drops a real capability (uncalled-fn checking).

## 4. The real shape of the fix (a separate project)

§6a-A is sound but requires **decoupling the eval's two roles**, not a
diagnostic filter:

1. **Make the documentation eval hermetic.** It should run in a fully isolated
   check state (snapshot+restore diagnostics, not just suspend Emit) so it
   NEVER contributes to the program's diagnostics, compilation gating, or
   coverage. This is approach (b)'s isolation, done properly.
2. **Add real construction-time body checking** as a first-class pass —
   post-binding (so recursion resolves) and against **carrier** args (so
   abstract `Map`/`List` params read `dynamic(Any)`, the §6a-A principle).
   This replaces the eval's accidental side-channel and serves the `zzyzx` /
   strand tests soundly.
3. **Recalibrate the langspec compilation-coverage corpus** in lockstep: with
   the synthetic-example errors gone, some rows legitimately compile further
   and some refuse for real reasons; the ceilings and the per-row tier
   expectations must be re-baselined, and the "did not throw" row investigated
   (it likely depended on a synthetic error forcing the interpreter fallback —
   a latent soundness gap the eval was masking).

Step 3 is the expensive part: it is a deliberate re-baseline of a 2830-row
compilation corpus, and the "did not throw" row suggests at least one genuine
compile-soundness gap hidden behind a synthetic error. That makes §6a a
corpus-recalibration project, not a localized checker fix — the opposite of
what `.5` projected.

## 5. Recommendation

The decision GATE is already green (concrete-fold), and decision's
direct-check false positives do **not** affect it (trusted on import). So
§6a's only consumer is the eventual §5b/§6 (sound module-body compilation),
which is itself gated on the larger checker-precision + corpus work. Given
that, the options are:

- **Shelve §6a** until §5b/§6 is scheduled as a unit — it is not independently
  load-bearing, and landing it now means re-baselining the compilation corpus
  for no gate benefit. *(Recommended.)*
- **Do the decouple-and-rebaseline project** (§4 steps 1–3) as an explicit,
  separately-reviewed change — sound, but it touches the help eval, adds a
  construction-check pass, and re-baselines `test/go/langspec`, including
  investigating the masked "did not throw" soundness gap.

What is NOT advisable is any partial diagnostic filter on the help eval (a/c):
it silently reclassifies compilation and changed observable behavior in
testing.

## 6. Reproduction

```bash
cd /home/user/boru/cmd/go && go build -o bin/boru ./boru
# the synthetic-eval source of the false positives:
printf 'def g fn [[m:Map] [Any] [(m get "xs") all]]\n' > /tmp/d.boru
./bin/boru check /tmp/d.boru            # → no_signature for all (from the help eval's {a:1,b:2})
# isolating the eval (drop its diagnostics) clears decision but fails:
#   lang/go/test  TestCheckUncalledFnBodyTypoStillFlagged, TestForwardStrandAdvisory
# partial suppression clears those but fails:
#   test/go/langspec  TestCompiledCoverage (refusals 30 > 26), TestOnlyMetaFallsBack,
#                     TestCompiledStatus, and a spec row's Assert.throws
```
