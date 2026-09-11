# Compilation of `do [ … ]` structures

Investigation of how the boru bytecode compiler lowers the `do` word — the
error-trapping "evaluate this body as code" construct. Covers the two `do`
signatures (`do [List]` and `do {Map}`), every distinct compile strategy the
recorder picks, why each is chosen, and a differential-parity check of
compiled vs. interpreted execution.

> **July 2026 update — the always-compile goal (§7).** Sections 2–5 below
> describe the state when this note was first written; the tranche-1 work
> (see §7, appended) has since closed most of the refusal classes: a
> multi-value body now compiles to a TRUE closure (whole-residual units,
> `BodyOutResidual`), `error`'s ignore-handler compiles via
> `StripsUnconsumedInput`, out-of-order residuals promote to frame locals,
> compile-time-known `word` splices compile natively inside closure bodies,
> and three MISCOMPILES were found and fixed (`do [args]` in a fn,
> empty-body `each []`/`fold []`/`scan []`, and the module-read
> rebind-after-fn divergence). §7 has the current frontier and the runtime
> JIT (Phase E) roadmap for what remains.

All disassembly below is reproducible with:

```bash
cd cmd/go && go build -o bin/boru ./boru
./bin/boru check --emit -e 'do [1 add 2]'      # show the bytecode + site report
./bin/boru run  -force-compile -e 'do [1 add 2]'  # require the VM path (no fallback)
```

## 1. What `do` is

`do` has two signatures (`lang/go/native/native_control.go`):

| Sig | Shape | Semantics |
|-----|-------|-----------|
| `[List]` | `do [body]` | Runs the body's tokens with **no per-call inputs**, **traps any error** and surfaces it as an `Error` **value**, and returns the body's **entire residual stack**. |
| `[Map]` | `do {m}` | Recursively evaluates the map's values as code, returns the rebuilt map. |

The `[List]` sig is the interesting one for compilation. It declares:

- `NoEvalArgs: {0: true}` — the body list is **not** auto-evaluated as an
  argument; it is code held as data.
- `Callable: &CallableSpec{BodyPos: 0, BodyOut: 1, Inputs: () => []}` — it is a
  code-body higher-order word whose body takes 0 inputs and nets 1 result. This
  is what makes the body **closure-compilable**.
- `CompileEffect: CompileFallbackBody` — if the closure can't be built, the
  construct may still compile as a Stage-5 interpreter island rather than
  forcing a whole-program interpreter fallback.

The runtime handler (`doListHandler`) is a single seam:

```go
result, err := InvokeBody(r, args[0], nil)   // eng/go/invoke.go
if err != nil { return []Value{NewError(err)}, nil }  // trap → Error value
return result, nil
```

`InvokeBody` is the interpreter/VM bridge: when `r.Invoker` is set (the VM is
running) the body runs re-entrantly on the VM; when it is nil a pooled
sub-engine runs the reconstructed token stream. Same handler, same result
either way — this is why `do` has one code path across both engines.

## 2. The five compile strategies (observed)

The recorder picks a lowering per call site. `check --emit` reveals which:

### (a) Single-value literal body → **closure** (`PUSH_CLOSURE` + `CALL_NATIVE`)

```
do [1 add 2]
0000 PUSH_CLOSURE f0   ; closure do$body/0
0001 CALL_NATIVE  s0   ; do (List)
fn f0 do$body/0 (locals=0):
0000 PUSH_CONST k1 ; 1
0001 PUSH_CONST k0 ; 2
0002 CALL_NATIVE s1 ; add (Number, Number)
0003 RET
```

The body compiles to its own fn unit `do$body/0`. The dispatch pushes it as a
closure value; the `do (List)` handler invokes it through `InvokeBody → r.Invoker
→ vmContext.invokeClosureOn`, which sees a `ClosurePayload` and runs the unit
directly on the VM — no interpreter sub-engine. Body ends with `RET`
(`callable_words.go::compileClosureBody`).

### (b) Diverging body → **closure with NO `RET`** (structured exception handling)

```
do [raise oops "boom"]        do [1 div 0]
0000 PUSH_CLOSURE f0          0000 PUSH_CLOSURE f0
0001 CALL_NATIVE  s0 ; do     0001 CALL_NATIVE  s0 ; do
fn f0 do$body/0:              fn f0 do$body/0:
0000 PUSH_CONST 'boom'        0000 PUSH_CONST 1
0001 PUSH_CONST oops/q        0001 PUSH_CONST 0
0002 CALL_NATIVE raise        0002 CALL_NATIVE div (Number, Number)
   (no RET — diverges)           (no RET — diverges)
```

`raise` carries `CompileDiverges` (always raises); `div`/`mod`-by-static-zero
carries `CompileValueDiverges` (raises for that operand shape). The recorder
treats a body ending in a divergent terminal as producing no value, so the
closure compiles with **no `RET`** — the raised error propagates out of the VM
and the enclosing `do` handler catches it via `InvokeBody`'s `err` return,
turning it into an `Error` value exactly as the interpreter does. This is the
bytecode form of structured exception handling: a trapping body is *catchable*
rather than uncompilable. (`eng/go/value.go` `CompileDiverges` /
`CompileValueDiverges` docs.)

### (c) Multi-value / empty inert body → **baked const list** (`PUSH_CONST` + `CALL_NATIVE`)

```
do [10 20 30]                 do []
0000 PUSH_CONST k0 ; [10 20 30]  0000 PUSH_CONST k0 ; []
0001 CALL_NATIVE s0 ; do      0001 CALL_NATIVE s0 ; do
```

The closure path requires a **single** output (`BodyOut: 1`); a 3-value body
nets 3 and declines it. But because `[10 20 30]` (and `[]`) is **fully inert
const data** (no words to dispatch), the body bakes as a plain const list and
the `do (List)` handler runs it. At VM time `invokeClosureOn` sees the operand is
a `ListPayload`, **not** a `ClosurePayload`, and falls to
`RunResolved(reg, nil, bodyTokens(body))` — the same pooled-sub-engine run the
no-Invoker interpreter branch takes. Byte-identical either way. (This is the
`noEvalBodiesInert` bake in `carrier.go`.)

### (d) Map body → **poly** (`CALL_NATIVE_POLY`)

```
do {a:(1 add 2)}
0000 PUSH_CONST      k0 ; {a:3}
0001 CALL_NATIVE_POLY p0 ; do/1 (poly)
```

The `do {Map}` sig auto-evaluates its map values *before* the handler runs, so
the operand arrives concrete and the static result is `dynamic(Any)`. It lowers
to `CALL_NATIVE_POLY`, which re-runs the kernel's own `MatchSignature` at run
time (`vmContext.callPolyIn`) — the same first-match the interpreter takes.

### (e) Body the compiler has no representation for → **a REFUSAL, i.e. a defect**

> **Superseded in its framing by §7 and §8 (note added 2026-09-11).** What
> this subsection describes is real and still the mechanism; what it gets
> wrong is calling it acceptable. There is no interpreter tier beneath the
> compiler. A refusal is an ERROR to be closed, and the shapes named below
> were closed by §8.

If the body neither compiles to a closure nor bakes as an inert const nor
islands, the whole program runs on the interpreter silently under
`--compile`, or aborts with the refusal reason under `--force-compile`
(`RunCompiledStrict`, `lang/go/boru.go`). The first of those is the dangerous
one: a silent refusal is a defect that reports itself as success.

**At the time of writing, `do [ … ]` did NOT always natively compile.** The
original text continued "and it doesn't need to, because the fallback is the
interpreter, which produces the identical result" — that reasoning is
rejected: identical results make a refusal SOUND, not acceptable. The body
refused whenever it contained a construct the VM had no representation for —
principally **tape-coupled re-stepping tokens**:

```
def xs [add 1 2]  do [word xs]         # → 3  — COMPILES since §8 (re-measured 2026-09-11)
do [def d [add]  word d 1 2]           # → still refuses; see below
```

Both aborted under `--force-compile` with **`code-body word do (Stage 2)`**
when this was written. Re-measured 2026-09-11, they have come apart, and the
difference is worth keeping:

- the first **compiles** — §8's `CompileDynBody` closed exactly that class;
- the second still **refuses**, but at a DIFFERENT gate — *"twin regime: a
  bind transition has no stream placement"* — so what remains of it is the
  body-local `def`'s twin, not the `word` splice this subsection was written
  about. Its interpreted result is itself an error (`cannot call add — no
  signature matches`), so it is a poor witness for anything now; a
  replacement whose interpreted run succeeds would be worth writing.

The mechanism below is why they originally refused. The
`word` splice (`__SP`) contributes tokens that are re-stepped against the live
stack (`eng/go/CLAUDE.md` "Quotation System" → splice); a `var [[…] …]` block in
the body (`CompileExecutesBody`) likewise splices `def`/`body`/`undef` tape
tokens. Neither can be a compiled closure, so `do` refuses to lower and the
program ran on the interpreter. The guarantee at that point was **"`do` always
runs correctly," not "`do` always compiles"** — and §7 records the maintainer
directive that rejected settling for it. The differential gate proves the two
engines agree on the result either way, which is what makes a refusal sound;
soundness is the floor here, not the goal.

### 2b. `do [ … ] error [ … ]` — the try/catch combinator

`error [handler]` (sig `[List Any]`, `BarrierPos 1`) takes the **preceding stack
value**: if it is an `Error` the handler runs (with the error on the stack) to
produce a fallback, otherwise the value passes through. Paired with `do` it is
boru's try/catch, and it compiles to **two paired closures**:

```
do [raise boom "kaboom"] error [drop "recovered"]
0000 PUSH_CLOSURE f0   ; closure do$body/0      ← body: raises, compiled with NO RET
0001 CALL_NATIVE  s0   ; do (List)              ← traps the raise → Error VALUE on the stack
0002 PUSH_CLOSURE f1   ; closure error$body/1   ← handler; local [_] = the trapped value
0003 CALL_NATIVE  s1   ; error (List, Any)      ← recovers → "recovered"
```

`do`'s handler turns the propagated raise into an `Error` value (strategy (b),
no RET); `error`'s handler closure binds that value as its single unnamed local
`[_]` and runs. Both dispatch as ordinary `PUSH_CLOSURE` + `CALL_NATIVE`. Note
that `error`'s result type merges the passthrough value with the handler result,
so it is `dynamic(Any)`: a word chained after it (`… error [drop 0] add 5`)
lowers to `CALL_NATIVE_POLY` (re-dispatched at run time), still byte-identical to
the interpreter. Verified across `raise`, `div 0`, no-error passthrough, and
trailing-arithmetic variants.

## 3. Static typing of the body (`doListReturnsFn`)

The check-mode return function models the residual so downstream consumers type
correctly:

- Non-empty body that runs to an **empty** residual (`do [raise …]`,
  `do [1 div 0]`) → typed as `Error` carrier, so `do [raise …] dot code` type-checks.
- **Empty** body (`do []`) → stays empty (it did *not* raise); inventing an
  `Error` there would wrongly admit `do [] convert Map`.
- Multi-value literal body → returns the **full** residual stack (arity matches
  the runtime), which also declines the single-output closure/island paths.
- Computed (carrier) body → bounded `dynamic(Any)` escape hatch, and during a
  compile pass keeps refusing to lower (checker precision must not imply compile
  coverage — pinned by `lang/go/code_effect_test.go`).

Because `do` traps errors, `doListReturnsFn` raises `CheckState.CaughtBodyDepth`
around the body analysis so guaranteed-runtime-error *mirrors* inside the body
are downgraded to info rather than reported as program errors (see
`eng/go/CLAUDE.md` "Check-mode guaranteed-error mirrors").

## 4. Differential parity check

`boru run -no-compile` vs. `boru run -force-compile` agree on every form tried
(20+ cases), including nested `do`, `do` over higher-order bodies, `do` bodies
that reference module-level defs, `do` inside a fn body capturing an enclosing
param, and every divergence/trap case:

| Program | Result (both engines) | Strategy |
|---|---|---|
| `do [1 add 2]` | `3` | closure |
| `do [10 20 30]` | `10 20 30` | baked const |
| `do []` | *(empty)* | baked const |
| `do [raise oops "boom"]` | `error(boom)` | closure, no RET |
| `do [1 div 0]` | `error(division by zero)` | closure, no RET |
| `do {a:(1 add 2) b:5}` | `{a:3 b:5}` | poly |
| `do [do [1 add 2] add 10]` | `13` | nested closures |
| `do [if (1 gt 0) [42] [0]]` | `42` | closure |
| `def x 5  do [x add 1]` | `6` | closure (module-scope read) |
| `do [each [mul 2] [1 2 3]]` | `[2 4 6]` | closure over a nested closure |
| `def bump fn [[x:Integer] [Integer] [do [x add 100]]]  bump 5` | `105` | closure **capturing** enclosing param `x` |
| `do [do [raise x "e"]]` | `error(e)` | inner traps → Error, outer returns it |
| `do [raise boom "kaboom"] error [drop "recovered"]` | `recovered` | paired closures (try/catch) |
| `do [1 div 0] error [drop -1]` | `-1` | paired closures (try/catch) |
| `def xs [add 1 2]  do [word xs]` | `3` | refused when this table was written; COMPILES since §8 (re-measured 2026-09-11) |

The capture case disassembles to a `do$body/1` unit with `[x]` local that the
enclosing `bump/1` frame supplies via `PUSH_LOCAL` before `PUSH_CLOSURE` —
lexical capture flows through the closure boundary correctly.

## 5. Summary

> **Superseded on its central claim — read §7 and §8.** This section was
> written before the always-compile directive (§7) and the dyn-body strategy
> (§8), and it describes a refusal as though the interpreter were a
> legitimate tier beneath the compiler. It is not. **The aim is that every
> code form compiles, so a refusal is an ERROR** — a defect with a date on
> it, not a resting place. "Sound refusal" means only "not a miscompile".
> The paragraph below is kept as the record of what was measured at the
> time; where it says the interpreter is the backstop, read: this is what
> had not been built yet.

At the time of this investigation `do [ … ]` did **not** always natively
compile — a body carrying tape-coupled re-stepping tokens (`word` splices,
`var` blocks) refused, and the whole program then ran on the interpreter.
What was guaranteed even then is that `do` always **runs correctly**: the
recorder chooses among four native strategies (single-value closure,
diverging closure with no RET, baked inert-const list, and Map poly) when it
can. §8's `CompileDynBody` has since closed the gap those refusals left — it
is a NATIVE strategy, not a fallback, and the `word xs` row in §4's table
above compiles today. The `do [ … ] error [ … ]` try/catch idiom compiles as
two paired closures. The single `InvokeBody`/`doListHandler` seam is what
keeps the trap-and-return semantics byte-identical across the interpreter and
the VM. No defects were found during this investigation.

**What remains, stated in the terms §7 set.** A program on the dyn-body
strategy COMPILES, but its body is invoked through `InvokeBody` — the
interpreter runs the body inside a compiled program. That is not a refusal
and not a fallback; it is the remaining defect, it is what
`TestInterpEntryCensus` counts, and closing it is the open work.

## 7. The always-compile goal — tranche 1 (July 2026)

Maintainer directive: `do` must ALWAYS compile — natively, for performance
(network servers in boru need full compilation to be credible; correctness via
interpreter fallback is not enough). Measured stakes (200k-iteration hot
loop): a closure-compiled `do` body runs **10.3×** the interpreter; the old
baked-const path (a runtime `RunResolved` sub-engine per call) recovered only
3.4×; a whole-program refusal recovers nothing.

### What landed (branch claude/do-structure-compilation-o662y4)

| Change | Effect |
|---|---|
| **`do [args]` miscompile fix** — the `specialWordResults` args projection read the CLOSURE analysis frame (the CallableSpec inputs, `[]` for do) and const-baked it; the island path reproduced the divergence. Fixed via `EmitState.inClosureUnit` (an `openUnitRecs` stack) + an args/`__pa` screen in `bodyFreeForFallback`. | interp `[7]` / compiled `[]` → refuses honestly, fallback parity |
| **Multi-out closures** (`CallableSpec.BodyOutResidual`) — `do` returns its body's whole residual, so the unit RETs all N values (the VM always supported it; only recorder gates blocked it). `RecordClosureCall` seats N results; `closureResidualExact` screens variadic / mismatched counts. | `do [10 20 30]`: 1.12s → **0.31s** (12× interpreter); class-8 shapes (`def x 5 do [x 1 add 2]`) compile |
| **Out-of-order residual promotion** — the fn-unit finish mirrors Finalize's `forceOrder`: an event result above an inert bottom promotes to a frame local and re-pushes in exact order (was: "result above a literal" refusal). | benefits every fn unit, not just do |
| **`error` ignore-handler closures** (`CallableSpec.StripsUnconsumedInput`) — the runtime identity-probe strip nets one value from the `[error, result]` residual; `stripResidualShapeOK` admits exactly the two nets-one shapes. | `do […] error ["fallback"]` fully closure-lowered; corpus islands stay 0 |
| **Empty-body parity fix** — an empty body's compiled closure now returns its pushed inputs verbatim, matching InvokeBody. | fixed pre-existing miscompiles: `[1 2] each []` compiled to `each_error` against the interpreter's identity map (fold/scan likewise); `error []` compiles as pass-through |
| **Static splice coverage** — the check engine fires `__SP` during body analysis (`word` is a non-emitting projection), so compile-time-known payloads compile natively into closure units. Unblocked by the residual work above; a dedicated token-walk expander prototyped for this proved redundant and was dropped. | `do [word xs]`, bare macro refs, `each [word op]` all force-compile |
| **Frozen-module-read discipline** — `NoteFrozenRead` records a concrete module-scope read baked inside an open unit; `NotifyNameRebound` refuses the program on a later module rebind. Stored-ref units (service handlers, spawn bodies) stay on their precise per-ref poisoning. | fixed a pre-existing miscompile of the DOCUMENTED module-dynamic semantics: `def x 1 def f […x…] f 0 def x 2 f 0` compiled `1 1` vs interpreted `1 2` |

Every gate holds: differential 0 mismatches (5144+ rows), corpus islands 0,
refusals 3 (≤ gate 4), ADR-008 coverage 100%.

### Tranche-1 frontier — CLOSED by Phase E (see §8)

The three classes that remained after tranche 1 (computed bodies `do b`,
`args`-bearing bodies, variadic multi-out bodies) all compile as of Phase E
increments 1–2. Kept for the record; the mechanism is §8.

## 8. Phase E — the dyn-body strategy (July 2026): frontier EMPTY

> "Backstop" here names a universal LAST-RESORT NATIVE STRATEGY, not an
> interpreter tier — a program on it compiles. The defect it leaves is
> narrower and still open: the body is invoked through `InvokeBody`, so the
> interpreter runs the BODY inside a compiled program, which is what
> `TestInterpEntryCensus` counts.

The universal strategy shipped in two increments (commits `7838403` and the
increment-2 follow-up). Mechanism — `CompileDynBody` on do's List AND Map
sigs; `tryRecordDynBody` (funnel specialist after the closure path) records
a plain CALL_NATIVE over the body operand with a VARIADIC-flagged result
(poly re-matched when the operand is gradual), and arms the program-wide
**DynEnv** mode: every def lowers an OpBindDynScope twin, every named unit
param dyn-binds, value-defs always promote (never dead-drop), tail calls
disable, and `Program.DynEnv` makes the VM bracket every CALL_USER frame
with an args-stack push so a body's sub-run reads `args` and the live
name environment exactly as the interpreter provides them. Costs land only
on programs using dynamic code bodies.

**The 19-shape frontier sweep force-compiles with byte parity**: computed
bodies (`do b` over fn params — the server class), `do [args]` (fn and top
level), runtime-computed splice payloads (`def xs [add 1 2] do [word xs]`),
variadic multi-out (`do [1 2 (if b [] [9 9])]`, nested, and
run-then-value), gradual List-vs-Map operands (`do (ops get (i add 0))` —
poly re-match), body-local def+splice, and loops inside do bodies
(`do [for 3 [1] 7]` — intra-event result-ID de-collision in
tryRecordDynBody; the unrolled model repeats one Value whose shared ID
collapsed producedBy).

Two more real miscompiles found and fixed on the way (four total across the
session): the splice-of-computed-payload closure baked the splice as
identity (`[7 8]` vs the interpreter's `7 8`) — the splice fire now poisons
recording for list-possible computed payloads, so the dyn-body backstop
owns the do-body form and the bare top-level form refuses honestly; and the
tranche-1 empty-body/args/module-rebind fixes listed above.

Still refusing honestly, with parity, both non-do fn-contract limits: a
variadic loop value mid-residual in a fn RET (Stage 3), and the bare
top-level computed splice. Gates: differential 0 mismatches, corpus islands
0, refusals ≤ 4, type-soundness 0, ADR-008 coverage 100%.

### Follow-ups — performance only, no coverage gaps

1. **JIT cache layer** — a dyn-body site executes through InvokeBody's
   pooled sub-engine today. A Registry-held cache keyed by
   `CanonValue(body)` + input types (entries carrying a `CompiledFnRef` +
   (name, gen) deps, negative caching) would compile the body once via the
   probe-then-real `compileClosureBody` shape `compileStoredBody` already
   uses, and run it via a `runUnitCross` on a fresh vmContext sharing the
   registry — upgrading hot dyn-body sites from sub-engine execution to
   compiled units. Ship behind `Options.JIT`, default-OFF, soak first.
2. **DynEnv cost profiling** — DynEnv is program-wide once armed; measure
   the OpBindDynScope + args-bracket overhead on programs that mix one
   dyn-body site with hot static code, and consider scoping the mirror to
   the reachable-call subgraph if it shows.
