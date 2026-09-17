# Edge-spec expansion — compiler divergences found

Status: **miscompiles closed, refusals owed** (2026-07-04) — all four
§1–§4 divergences below no longer produce a WRONG value: the compiler
now REFUSES each shape, and `RunCompiled` (`lang/go/boru.go`) **silently**
re-runs the refused program on the interpreter so the user still gets an
answer — nothing in the run says the compile was refused. That re-run is
machinery absorbing a known defect; it is not a resolution, and it is not
a second acceptable outcome, and its silence indicts it rather than
excusing it: a compile failure that hides itself is the harder one to
find. The contract is that all valid code compiles, so each of these four
refusals is an open DEFECT against it, owed the correct lowering and
tracked to closure in §Disposition. `--force-compile` surfaces a loud, precise reason. Each
reproducer and its non-over-refusal siblings are pinned by
`lang/go/bytecode_edge_findings_test.go`
(`TestEdgeFinding*`); the whole-corpus differential, or-fallback,
metafallback partition, and status census all stay green (0 new
refusals on the live corpus — none of these shapes is a corpus row).
Landing notes are inline under each section.

Original status: **findings**. While adding ~2,000 edge-case spec
rows (the `lang/spec/edge-*.tsv` families), genuine
compile≠interpret divergences surfaced — the exact class of latent
miscompile the expansion was meant to flush out. Each is a shape the
compiler lowers to a WRONG result where it must compile correctly. A
refusal stops the wrong answer, but a refusal is itself a defect against
that requirement — never a sanctioned second branch.
The rows are dropped from the corpus (they cannot be pinned under the
0-divergence gate) and recorded here as reproducers for the
correct-lowering fix (with a refusal as interim containment only).

## 1. Forward collection reaches across an error-handler residual

```
5 do [raise aa "x"] error [drop 9] add 1
```

- Interpreter: `5 10` — the `error [drop 9]` handler leaves `5 9` on the
  stack, then `add 1` forward-collects `1` and takes `9`, computing
  `9 + 1 = 10` and stranding `5`.
- Compiled: `14 1` — the compiled path mis-schedules the `add`
  operands across the `do…error` boundary (appears to compute `5 + 9`).

Root-cause family: the residual-window operand accounting across a
reified-error boundary — adjacent to the statement-boundary absorption
class the M2c landing already closed for method dispatch
(`design/legacy/CHECKER-BYTECODE-COMPLETION-PLAN.0.ignore`, Phase 3.5 log). The
fix is to model the handler residual as a barrier the `add` window
cannot cross; refusing the shape only contains the defect until that
lands.

**Landing (interim refusal).** Root cause pinned precisely: the `error` island
output is `dynamic(Any)`, which BLOCKS `add`'s `[Number Number]` forward
overload, so check-mode matchSignature falls to the `[Scalar Scalar]`
catch-all in ALL-STACK form — reaching PAST the dynamic top-of-stack to
the deeper leading residual (`5`) and stranding the forward token that
the interpreter (concrete operand) would collect. `eng/go/engine.go`
`refuseForwardStackDrift` refuses, in compile mode only, a
forward-eligible dispatch that matched all-stack with a DYNAMIC
top-of-stack operand, a NON-dynamic deeper operand, and an atomic-literal
forward token immediately after the word. `mul 2` / `sub 1` (forward-
collect their token, fwdCount>0), a String forward token (routes to a
different overload), and the no-leading-residual / concrete-`do` forms
are all left compiling.

## 2. Applied member-fn boundary leaves the function unapplied

```
def d fn [[n:Integer] [Integer] [n mul 2]] def m {double: d/r} m.double 21 eq 42
```

- Interpreter: `true` — `m.double 21` auto-applies the parked `d/r` to
  `21` → `42`, then `eq 42` → `true`.
- Compiled: `fn d(Integer) false` — the compiled path leaves the fn
  value unapplied on the stack, so `eq 42` compares the function to
  `42` (false) with the fn stranded.

Root-cause family: the fn-value-call boundary (plan P4 / M2). The M2c
method-shape work models statement-position *0-return* method calls;
this is a *value-returning* member fn (`m.double 21` feeding `eq`) —
the shape-annotated dispatch must extend to a mid-expression member
apply; refusing is containment, not a fix.

**Landing (interim refusal).** The read `m.double` surfaces a parked user-fn
that AUTO-APPLIES the moment `21` lands on it; the compiler instead lets
the downstream `eq` steal `21` and applies the stranded fn at the
residual tail to the wrong value. Because the read's static type is
`dynamic(Any)`, the fn-ness is invisible downstream, so a get-family read
of a fn-valued container member is TAGGED at record time
(`eng/go/emit.go` `readsFnMember` / `noteMemberFnRead`) and
`eng/go/engine.go` `refuseStrandedMemberFn` refuses (compile mode only) a
dispatch whose deepest stack operand sits directly above such a tagged
value. The bare statement-tail apply `m.double 21` never reaches the
guard (nothing dispatches above the fn) and keeps lowering to the
trailing apply; non-fn member reads and `c/r` param-fn applies (a
different boundary, M2a) are untouched.

## 3. Paren-arrived value run as an else body (from edge-forward)

```
def n 5 if (n eq 0) [99] (range 2 4)
```

- Interpreter: `2 3` — the parenthesised `(range 2 4)` arrives as the
  else *body* and is executed, leaving its elements.
- Compiled: `[2 3]` — the compiled path treats the arrived value as a
  list result rather than an executed else body.

Root-cause family: the computed-else branch modeling for a paren group
whose value is itself a runnable body — the boundary between "else
value" and "else body" under forward arrival. Fix: model the
body-execution semantics; refusing the ambiguous computed-else-body
shape only contains the defect in the interim.

**Landing (interim refusal).** `if3ReturnsFn`'s body-vs-value split gates on
`IsConcrete`, but the paren-arrived range is a non-concrete list CARRIER,
so it took the value path and pushed the list while the interpreter's
`spliceArg` executes ANY plain list arm. The carrier's static typed-ness
does not predict the split (`range` is statically `[:Integer]` yet
returns a plain, spliced list), so `lang/go/native/native_control.go`
`refuseComputedBranchBody` refuses (compile mode only) any TList-conforming
value-path arm — but ONLY when the condition can actually SELECT it
(`condSelectsArm`): a constant condition makes the opposite arm dead, so
`def n 0 if (n eq 0) [99] (range 2 4)` (else unreachable) keeps compiling.
Scalar / paren-scalar arms and literal `[…]` bodies are unaffected.

## Disposition

All four (§1–§4) are **soundness** items: the compiler produced a wrong
value where it must produce the right one. They are lower-frequency than
the corpus rows (none appears in the 3,875-row base corpus), and each had
a clear interim containment — refuse the shape — that stops the wrong
value at the cost of the row itself, pending the correct lowering.

**Contained (step 1 — the wrong value stopped).** Each is now a
compile-mode refusal, and `RunCompiled` re-runs the refused program on
the interpreter so the user still gets an answer while the defect stands
— see the per-section landing notes and
`lang/go/bytecode_edge_findings_test.go`. Every guard is compile-mode-only
(the interpreter and plain `--check` are byte-identical to before) and is
gated tightly against over-refusal (each landing note records the
non-refused siblings, all pinned as `mustCompileWithParity` negatives). No
corpus row changed status — the full `verify-bytecode` battery
(differential + or-fallback + combinations + property fuzz), the
metafallback partition, and the status census stay green.

**Owed (step 2 — the actual fix).** Modelling the correct lowering
natively — the barrier-crossing operand accounting (§1), the mid-expression
member-fn apply / value-returning method dispatch (§2, extends the M2c
statement-position shape work), the executed-else-body semantics (§3), and
the compiled per-call args projection for unnamed params (§4) — is what
closes these four. Each refusal above is a defect against "all valid code
compiles, no exceptions" and stays open until its lowering lands; step 1
only bought time.

## Checker-precision rows removed to hold the sacred pins

Six edge rows were dropped from the corpus not because they fail, but
because pinning them would move a **sacred checker pin** that the
check-by-default safety case depends on. Each runs correctly; each is
a candidate for a checker-precision follow-up.

- **`[None] get 0` → None** (edge-containers-4): the checker flags this
  correct row — a genuine value-row **false positive** (the FP pin is
  0). A stored/read `None` at a concrete list index is the trigger.
- **Type-algebra precision gaps** (would raise the type-soundness pin
  from 7): `if/1 [true 7]`; `P tor Q` (union of two newtypes);
  `tnot Never` → Any; `tnot Any` → Never; a predicate-fn-as-type
  return admitting a passing base value. In each the checker's static
  stack type differs from the runtime value's type — the same
  "checker-more-precise-than-runtime / type-algebra" class the seven
  standing soundness pins already document, not a wrong-acceptance.

These belong to the same checker-precision track as
`legacy/checker-precision-fronts.0.ignore`; folding them in is a pin-raise
decision for a maintainer, kept out of this test-expansion landing.

## 4. `args.N` accessor diverges inside a compiled fn body

```
def f fn [[Integer String] [String] [args.1]] f 1 "hi"
```

- Interpreter: `'hi'` — `args.1` reads the call's second argument.
- Compiled: divergent — the VM's CALL_USER frame binds params to
  frame locals and does not maintain the interpreter's per-call args
  stack, so `args.N` inside a compiled body reads wrong. The emitter
  refuses a bare `args` word (a long-standing, still-open refusal); the
  dotted `args.N` form slipped past that guard and compiled to a
  divergent result — a worse defect than a refusal, but both are defects.

Interim containment: extend the `args`/`__pa` compile refusal to the
`args.N` reach form so the body stops miscompiling. The fix that closes
this is the compiled per-call args projection for unnamed params
(§Disposition). Reproducer dropped from the corpus.

**Landing (interim refusal).** Root cause pinned to UNNAMED params: `args.N`
folds soundly to `PUSH_LOCAL N` when every param is NAMED
(`recursion.tsv:35,36`, still compiling), but an unnamed param flows
through the body STACK, so folding `args.N` over the projection strands
that live input (a divergent value/count). `runFnBodyOnce`
(`eng/go/carrier.go`) records `CheckState.ArgsFrameUnnamed` for the frame
under analysis, and the `args` projection in `specialWordResults` refuses
(compile mode only) when it is set. Named-only frames keep compiling.
