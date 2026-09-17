# Module-fn body compilation: Direction B works for diagnostics but exposes §6 as the true blocker

Status: **`.4` — Direction B measured end-to-end; go/no-go is "needs §6".**
This note records the result of implementing `.3`'s Direction B (suppress
imported-module-body diagnostics, gate on `bytecode == interp` instead) and
running it against the real decision gate. The diagnostic-suppression half
works. The threading half, even with perfect suppression, regresses the
gate's compilable suites from *compile* to *refuse*, because it correctly
stops the unsound concrete-fold those suites currently rely on — and nothing
replaces it until §6 (compile the framework's code-body words) lands.

Read `.0`–`.3` first. No code from this pass was landed; the branch keeps
only the §5a memo-key commit (`6665be3`) and the `.0`–`.4` design record.

---

## 1. What was implemented (Direction B)

- `CheckState.ModuleBodyDepth` + `CheckDiagnostic.ModuleBody` tag.
- `AddDiagnostic` tags a diagnostic `ModuleBody` when emitted at
  `ModuleBodyDepth > 0`.
- `CheckState.DropModuleBodyErrors()` drops error-severity ModuleBody
  diagnostics at end of pass, wired in beside `RescueForwardRefDiagnostics`
  in both `Check` and `CompileCheck` (`lang/go/boru.go`).
- `execFnDefSig`: a value-threading probe (env-gated) that points the module
  sub-registry's `Check` at the parent pass's for the `CallBoru`, bumps
  `ModuleBodyDepth`, and copies the mutated state back.

## 2. Result A — suppression works for in-body diagnostics (29 → 14)

With the threading on, `check decision_smoke_test` goes **29 → 14** errors.
Instrumenting `AddDiagnostic` shows a clean split:

| Diagnostic | depth | suppressed? |
|---|---|---|
| `eval-pred-all/any/not`, `all`, `any`, `eval-cond`, `get` (no_signature) | 1 | **yes** (ModuleBody-tagged) |
| `decide`, `eval-tree`, `eval-table`, `with-policy` (uncalled_function) | 0 | no |
| `make`, `collect-table` (no_signature); `DTable` (undefined_word) | 0 | no |

The depth-1 cases — the ones genuinely *inside* a threaded module body — tag
and drop exactly as designed. The mechanism is sound.

## 3. Result B — the residual 14 are depth-0 leakage (value-copy threading is too crude)

The surviving 14 are emitted at `ModuleBodyDepth == 0`, so the tag never
applies. Two sub-classes:

- **Parent-side `uncalled_function`** on `decide`/`eval-tree`/`eval-table`:
  the test's own calls to module fns, where the call arrives as
  `(Word, Map)` — a first arg the parent couldn't resolve. These appear
  ONLY with threading on (baseline = 0), so the value-copy threading's
  copy-back is corrupting parent state or the module fns' return carriers.
- **`undefined_word DTable` at depth 0**: a module-private type referenced
  inside a module fn body (`make DTable {…}`) analysed against the PARENT's
  `Defs` (where `DTable` is unbound) — a **resolution-scope** leak, exactly
  the failure `.0` §4 warned about. The value-copy threading shares the
  check state but lets some body analysis run with the wrong name space.

Both point at the same conclusion `.1` §3.1 already reached: the value-copy
threading is too leaky; the **clean pointer-based `*CheckState`** (`.1` §5b,
with `CheckState.Clone` for the rollback snapshots) is required so depth and
scope are coherent everywhere. This is a known, designed fix — not a new
unknown — but it was not re-applied here because Result C makes it moot for
now.

## 4. Result C (decisive) — threading regresses the compilable suites to REFUSE

Bypassing the check gate entirely (drop *all* errors) and asking the real
go/no-go question — *does the recorded module body compile to bytecode that
matches the interpreter?* — gives, for the two compilable suites:

```
decision_unit_test  --force-compile → error: force-compile: code-body word test-test (Stage 2)
decision_smoke_test --force-compile → (same: refuses)
```

It does **not** diverge — it **refuses**. Threading runs the test framework's
bodies in check mode, where the code-body words (`test-test`, and behind it
`test-describe` / `run-cases`' computed `for`) hit the Stage-2
closure-compile limits (`.0` §6) and refuse.

At **baseline** these same suites report `bytecode ok (== interp)` — but only
because the baseline path runs the module bodies **concretely** and bakes the
results (`.0` §3 footnote: "the compilation of unit_test / smoke that exists
today is the SAME concrete fold … it only looks like compilation"). The gate's
green `bytecode == interp` for these suites **depends on the very
concrete-fold that §5b is designed to eliminate.**

So any sound change that stops the concrete fold — Direction B's threading,
or the clean pointer §5b — necessarily turns these suites from *compile* into
*refuse*, until §6 makes the framework's code-body words actually compilable.

## 5. Conclusion: §6 is the true remaining blocker; §5b and §6 must land together

The dependency chain, now fully measured:

- **§5a** (registry-discriminated memo keys) — *landed*, inert, sound.
- **Diagnostic policy** (Direction B's ModuleBody tag + drop) — *works* for
  in-body diagnostics; necessary; cheap. Re-appliable.
- **§5b threading** — must be the **clean pointer form** (`.1` §5b) to fix the
  §3 depth/scope leakage; the value-copy probe is insufficient.
- **§6** (compile the framework's code-body words: `test-test`,
  `test-describe`, `run-cases`' computed `for`, past the Stage-2 closure
  limit) — **the real blocker.** Without it, §5b regresses the gate's
  compilable suites from compile→refuse, so the gate cannot be green with
  §5b alone.

There is no ordering of §5a/§5b/diagnostic-policy that greens the decision
gate without §6. The earlier notes treated §6 as a follow-on; the
measurement shows it is **co-required** with §5b: the gate's `bytecode ==
interp` for `decision_unit_test`/`decision_smoke_test` is currently
satisfied only by the concrete-fold, and replacing that with sound recording
requires the framework's code-body words to compile.

## 6. Recommended next step

§6 is its own substantial bytecode-compiler task (Stage-2 closure compilation
for `test-test` / `test-describe` / computed-`for`), independent of
`CheckState` ownership. Sequence:

1. Land §6 first (compile the framework's code-body words) on its own,
   verified by the in-repo bytecode suites — **with the concrete-fold still
   in place**, so the compilable decision suites stay green throughout.
2. Then apply the clean pointer §5b + Direction-B diagnostic policy together.
   Now stopping the concrete fold no longer regresses the suites (they
   compile via §6), the in-body diagnostics are suppressed (Direction B),
   and the `bytecode == interp` gate validates the *real* compiled bodies.
3. Re-attempt the `evCallUser` promotion (`.0` §7) last.

Until §6 exists, the soundest landed state is §5a + this design record. The
Direction-B diagnostic mechanism (ModuleBody tag + `DropModuleBodyErrors`)
and the clean pointer §5b are both designed and ready to re-apply once §6
removes the concrete-fold dependency.

## 7. Reproduction

```bash
git clone https://github.com/voxgig-boru/decision /tmp/decision
cd /home/user/boru/cmd/go && go build -o bin/boru ./boru
# baseline: compilable suites are green ONLY via the concrete fold
cd /tmp/decision && BYTECODE_BORU=/home/user/boru/cmd/go/bin/boru bash test/diverge.sh
# with §5b threading (any form), --force-compile on the compilable suites
# refuses at `code-body word test-test (Stage 2)` — §6 is required.
```
