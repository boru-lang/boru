# Checker Completion — the guaranteed-runtime-error mirror sweep

**Status:** implemented, 2026-07-07; design refits (§7) landed the same
day. Outcome record (§6) is authoritative; §7 supersedes the two
workaround mechanisms §1 and §5 describe (CaughtBodyDepth suppression,
the blanket `!Compiling` gating).

## 1. Motivation

The check-accuracy ratchet held 0 false positives against 5,078 value
rows, but stayed silent on 241 of 786 ERROR rows (~31%). A classification
of those 241 (probing each representative shape through `boru check` vs
`boru do`) showed roughly **half were statically decidable** with
machinery the tree already had — the checker knew the fault (a concrete
key provably missing, a provably out-of-range index, a failing concrete
assertion) but either reported it below Error severity, modelled it
silently (divergence with no diagnostic), or never looked.

The unifying discipline this sweep installs: **a fault the analysis can
PROVE will occur at runtime is a check-time error, with the runtime's own
code and detail wherever the analysis holds the exact runtime value.**
Three reachability gates keep it sound:

- `FnBodyDepth` (existing) — a fn body runs only if called; per-call
  analysis fires only from real call shapes.
- `NestedBodyDepth` (new, raised by `RunCarrierBodyWithDefs`) — every
  branch / loop / quotation body is conditionally reached; a diagnostic
  that is only sound for unconditionally-reached code must not fire there
  (`if (n eq 0) [raise "zero"]` is a working guard, not an error).
- `CaughtBodyDepth` (new, raised by `doListReturnsFn`) — `do [body]`
  TRAPS every body error at runtime and surfaces an Error value, so a
  guaranteed-error mirror inside the region is not a program error
  (`do [{a:1} !. b] error [dot message]` is a working program).
  `CheckAddUniqueDiagnostic` and `emitIndexOOB` honour it.

`eng.CheckAtUncaughtTopLevel` composes the gates (plus `!Compiling`, so
the recording pass stays byte-identical).

## 2. Tier 1 — contract mirrors

- **Return COUNT conformance** (`checkBodyReturnConformance`,
  eng/go/core_helpers.go): the static twin of the `__RC` arity rule. A
  residual provably longer than declared + the unnamed-arg discard
  allowance flags the runtime's `"expected N return value(s), got M"`
  (shared `returnCountErrorText`). A variadic spread (0-or-more values)
  and a Function/FnDef residual (a possibly-unapplied fn-value call the
  static model over-counts — emit.go's cluster-E shape) skip. The
  all-concrete-call EMPTY residual on the top-level straight line also
  flags: the body either diverged (a raise) or under-returns, and the
  runtime errors either way. Kernel pins:
  eng/go/return_count_check_test.go.
- **Strict-accessor static misses** (getr means REQUIRED): a concrete key
  provably absent from a concrete map (`getrNodeReturns`, re-proved
  against the container — never inferred from the None carrier, because a
  present key holding `none` produces the same carrier shape and
  succeeds), from a CLOSED class schema (`getrObjectReturns`), or from a
  sealed module-export map (`moduleExportGetrReturns`, alongside its
  compiled-path trap); a concrete list read with a non-integer key; a
  statically-None strict-read parent (`getrNoneReturns`). `unpack`
  flags a key missing from a PROVEN (concrete) source — the check-mode
  stub getter's misses are not evidence (`unpackSource` now reports
  provenness). `index_out_of_range` promoted to SeverityError (every
  emit site is a provable OOB whose every consumer — getr / at / set /
  ArrayUtil edits — errors at runtime); `set` on a concrete list runs
  `CheckListIndex` (`setListIndexReturns`).
- **Unconditional top-level raise** (`raiseReturns`): `raise` always
  raises (even malformed args raise raise_error), so a dispatch on the
  uncaught top-level straight line is a program that provably errors
  (`unconditional_raise`, detail carrying the concrete code/message when
  known).

## 3. Tier 2 — constant-fold and schema mirrors

- **Arithmetic faults** (native_math.go / native_helpers.go): concrete
  int64 operands are re-run through the SAME checked guards the handlers
  use (`addIntFault`/`subIntFault`/`mulIntFault`/`powIntFault` mirror the
  intFn bodies exactly, same operand order) — int64 overflow flags the
  runtime's `integer_overflow`, pow's negative exponent and div/mod by a
  static zero flag `arith_error` with the runtime message. The zero-
  divisor divergence model (`returnsDivMod`) now also covers a BigDecimal
  zero. The Big⊕Float mix is TYPE-decidable (`checkBigFloatMix` — strict
  operand leaves fix the refusal, no value can save it), as is `convert`
  of a proven Float into a Big target (`convertScalarReturns`).
- **Sealed class writes** (`setClassInstanceReturns`): an unknown field is
  a guaranteed `sealed_field`; a concrete value is re-run through
  `MakeClassFieldValue` — the byte-identical write-time check. Schema
  resolution via `TopTypeBody` only: an `undef`'d class or an
  instantiated generic (whose minted node carries no ClassTypeInfo
  payload) stays runtime-only.
- **Type-literal ordering** (`orderingDeterminate`): a bare type NODE is
  determinate — the literal IS the runtime operand — so `List lt Map`,
  `5 cmp Scalar` flag `incomparable` through the existing
  OrderingReturnsFn handler run (now routed through
  CheckAddUniqueDiagnostic for the caught-body gate).

## 4. Tier 3 — the pure-handler dry pass

`eng.DryPassReturns` / `DryPassWrap` (eng/go/drypass.go): for a word
whose handler is a PURE function of its arguments, runtime failure on
concrete args is check-time failure on the same args. The dry pass runs
the real handler once (top-level straight line, all args concrete — a
strict none-shape canonicalised to the `none` sentinel so payload probes
agree with runtime), and mirrors a handler error into a diagnostic with
the runtime's own code + detail. Wired on: Assert.equal / not-equal /
ok / match (boru:test — assert-throws runs a BODY and stays runtime-only),
BinUtil.hex-decode / base64-decode, ArrayUtil.insert-at / remove-at,
MatrixUtil.create, StructUtil.parse / reify (reify gated on
statically-known target + concrete source), Debug.parse, Vm.parse, and
the always-erroring outside-a-template `unquote` / `splice`.

Precedents this generalises: miniMicronLitReturns' lenient dry pass,
parseSpecReturns, CheckMicronConstruction, getrMicronReturns.

## 5. Soundness ledger

Every mechanism is gated on something the analysis PROVES it holds:
concreteness of the exact runtime value, a type-decidable refusal over
strict static types, or a closed schema. The two false positives the
first accessor cut introduced (`do [{a:1} !. b] error [dot message]` —
the trapped-error shape) drove the CaughtBodyDepth gate; the final
ratchet run is back to 0/5,078. The compile pass is excluded everywhere
(`!Compiling`) except the pre-existing shared residual models, so the
recorded programs are unchanged (differential green).

## 6. Outcome record

Measured on this tree (merged main @ 69d2ea2 + this sweep):

| Metric | before | after |
| --- | --- | --- |
| Unflagged ERROR rows | 241 / 786 | **137 / 786** |
| False positives | 0 / 5,078 | **0 / 5,078** |
| Error-corpus coverage | 69.3% | **82.6%** |
| Compile refusals (gate) | 21 | 21 (unchanged) |
| Islands / differential | 0 / green | 0 / green |

Remaining 137 (documented residual classes, not regressions):
registration/builder STATE (DSL double-registers in loops, grammar
seals, Log span state, Luhn builders); malformed sources fed to
REGISTERED parsers/emitters (the parser resolves the source in the
shell); net/socket and context-store state; abstract-carrier dispatch
limits (micron cross-kind cmp, convert-Bytes ordering — the
comparer-aware family-disjointness walk stays future work; dynamic
module values); value-dependent generic instantiation (`Box of
[Integer]` writes — no schema payload on the minted node);
fn-predicate types (check-mode `.Is` is deliberately lenient for them);
and divergence needing BRANCH-fold propagation (`f 0` reaching a raise
through a folded condition, plus Assert.throws' body run).

## 7. Design refits — classification over suppression

Review of the first cut identified the two suppression mechanisms as
design debt: CaughtBodyDepth *silenced* findings (discarding
information, and leaving other diagnostic families inconsistently loud
inside `do` bodies — `do [undefined-word] error [dot code]` is a WORKING
program the checker still error-flagged), and the blanket `!Compiling`
gating made the check and compile passes report DIFFERENT diagnostics
for the same source (Vm.compile's surfaced diagnostics were a subset of
`boru check`'s). Three refits replace both workarounds with structure:

1. **`CheckState.BeginCompilePass()`** — the compile-pass arming ritual
   (fresh EmitState, the Compiling flag, fn-memo drop) extracted into
   one shared helper used by lang's `CompileCheck` and boru:vm's
   `Vm.compile`. The hand-rolled copy is how Vm.compile shipped without
   the Compiling flag.
2. **`CheckDiagnostic.RuntimeMirror`** — mirrors are classified at the
   emit site (`CheckAddUniqueDiagnostic` stamps all its callers; the
   direct emitters — the return count/type conformance mirrors,
   `emitIndexOOB` — stamp explicitly). The compile refusal loops
   (`CompileCheck`, `Vm.compile::hasCheckError`) skip mirrors: only
   MODEL-UNDERMINING errors (undefined_word, no_signature — the
   recording is a guess) refuse. Every `!Compiling` sprinkle in the
   mirror emitters is deleted; the two passes now report identical
   diagnostics, and guaranteed-error rows COMPILE their exact error
   path (trap / VM RET / the same handler) instead of falling back.
3. **`CheckDiagnostic.CaughtAtRuntime`** — `AddDiagnostic` re-attributes
   centrally: inside an error-trapping region (CaughtBodyDepth) every
   error-severity finding is downgraded to info and stamped, uniformly
   across ALL families. The finding stays visible ("this expression
   always raises, and the trap is here"); the false error verdict is
   gone — including the previously-latent `do [undefined-word]` false
   positive, now pinned by TestCaughtRegionReattribution. Dedupe
   helpers skip caught entries so a caught info never masks a later
   real emission of the same finding.

Pins: eng/go/drypass_test.go (BeginCompilePass, caught re-attribution
at the emit seam), lang/go/checker_refit_test.go (refusal narrowing,
check/compile diagnostic agreement, per-family re-attribution).
Ratchet unchanged by the refits: 137 unflagged / 0 false positives.
