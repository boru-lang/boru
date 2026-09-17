# boru Bytecode — Completing Runtime Independence (P5–P7)

Status: design. Companion to `design/boru-bytecode-plan.0.md` (which records
the landed P0–P4). This doc specifies the **remaining** work to reach the
goal: a compiled `Program` executes entirely in the VM, with the `OpFallback`
island and the whole-program fallback **deleted**.

## Where we are

P0–P4 landed (all gate-clean: differential + whole-corpus at 0 divergences,
race-clean, alloc ceilings held). The runtime-independence machinery exists:

- **Seam / re-entrancy.** `eng/go/invoke.go::InvokeBody` + `Registry.Invoker`;
  `vmContext.run(startUnit, locals, stack)` is the re-entrant VM loop
  (`eng/go/vm.go`). A code body runs as a compiled closure via
  `vmContext.invokeClosure`, never the interpreter.
- **Closures.** `OpPushClosure` / `ClosurePayload` / `CompiledFn.NCaptures`;
  `callableWords` + `tryRecordClosure` (a throwaway-`EmitState` **probe**
  compiles speculatively, so a refusal falls back to the island untouched).
  Closure capture via `ComputeCaptures`.
- **Runtime dispatch.** `OpCallNativePoly` / `PolyRef` / `tryRecordPoly` /
  `vmContext.callPoly` — the kernel's own `MatchSignature` selects the
  overload at run time. `OpCallDynamic` / `vmContext.callDynamic` /
  `tryNativeFnApply` — the fn-value-call boundary (closures VM-native,
  trivial-delegation methods VM-native, user-fn bodies islanded as a
  backstop, non-callables left untouched).
- **Ratchets.** `test/go/langspec/compiled_coverage_test.go`:
  `refusalCeiling` (rows that produce no `Program`) and `islandCeiling`
  (compiled programs still containing an `OpFallback`). Both are downward;
  P7 is gated on both reaching **0**.

**Method fields (`m.f`) — unnamed-fn const members (LANDED, 527 → 521).** A map
whose field is an inline fn (`{f: (fn …)}`) could not const-bake because
`isInertConst` rejected the fn member, so the receiver was unresolvable and
`m.f args` refused as "opaque output". Allow an UNNAMED fn value as a const
compound MEMBER (`isInertConstMember`): immutable code is safe inside a
read-only const map/list. Then `m.f args` compiles as it already did for scalar
fields — a `CALL_NATIVE_POLY get` returns the fn (dynamic), and the existing
fn-value-call boundary (`applyDynamic` → `OpCallDynamic`) applies it. A NAMED
ref field (`{b: f/r}`, Name="f") is kept non-const: applied through the island
sub-engine it re-dispatches by name and forward-collection of the trailing arg
diverges, so it falls back faithfully instead (a pre-existing island subtlety,
not introduced here). Bare top-level fn values stay non-const (the apply /
closure case). Files: `eng/go/emit.go` (`isInertConstMember`).

**Guard-if statement (`if cond [raise]`) — 0-value-then if (LANDED, 521 → 519).**
A 2-arg `if` whose then-branch produces 0 values (a `raise` guard, a 0-value
word like `set`/`printstr`, or a break/continue) refused as "then-branch
produces no value". But such an if produces 0 values on BOTH paths (true→0 or
diverge, false→0) — it's a statement guard, not a variadic 0-or-1 result.
`if2ReturnsFn` still types it `[None]` (so RecordCall's double-record guard
elides the dispatch), but `RecordBranch` marks the seq `zeroOut`: the lowerer
emits no merge slot, and Finalize's residual reconciliation skips the phantom
None. The trailing statement (`… def q (10 div n) q`) then lowers cleanly. The
broader variadic-statement-if (a value-producing then used as a discarded
statement; a computed else) stays refused — it needs true 0-or-1 residual
modeling. Files: `eng/go/emit.go` (RecordBranch zeroOut + residual skip),
`eng/go/lower.go` (lowerArms no-slot), `lang/go/native/native_control.go`.

**Concrete-args dynamic-output core builtins (LANDED, 519 → 514).** A core
builtin native with CONCRETE args but a declared-Any (dynamic) output — e.g.
`unify` ([Any,Any]→[Any,Boolean], a 2-result word that can't poly) — refused as
"opaque output". But concrete args mean the checker RESOLVED the sig by real
matching (not widening), so the dynamic output is just a declared-Any return,
not a best-guess sig: a plain CALL_NATIVE bakes faithfully. `dynOutNativeOK`
(carrier.go) gates it (concrete args, dynamic output, core sig, no
meta/fn-value/code-body), and RecordCall's `anyDynamicCarrier(outs)` refusal
gains a `forceDynOut` bypass. The dynamic result is still registered, so a
downstream TYPED consumer of it refuses via the dynamic-input guard — contained.
Cleared 18 opaque-output rows (−5 net; cascades moved to other buckets). Files:
`eng/go/carrier.go` (dynOutNativeOK), `eng/go/emit.go` (RecordCall forceDynOut).

**Module inner natives via dot-access (LANDED, 514 → 459).** A module word
called `Pkg.word` (`StructUtil.clone`, `StructUtil.jsonify`, …) trivially
delegates to an inner native registered in the module's sub-registry; those
natives have no `Returns` annotation, so their Any output is dynamic and they
refused as "opaque output". But the dispatch IS sound to bake: the interpreter
dispatches the inner native via `execMatch` on the main engine (the wrapper's
trivial delegation), so a `CALL_NATIVE` with the main registry is identical.
`dynOutNativeOK` now accepts the inner sig, verified by `isModuleInnerSig`
(pointer-membership in a loaded module's wrapper sub-registry) — which excludes
usurp synthetics. The IsBuiltinWord gate stays on the CORE path: a user
`def ifu (usurp if)` makes `r.Lookup("ifu")` return the usurp-MODIFIED `if` sig
(pointer-equal), so without that gate ifu baked and returned a tape-coupled
result (the differential caught it). Cleared the struct-util / module-word
opaque cluster: −55 refusals, +48 compiled differential rows. Files:
`eng/go/carrier.go` (dynOutNativeOK + isModuleInnerSig).

Current ratchets (live, 2026-06): **11 refused / 0 islanded**. The detailed
remaining-row breakdown and per-cluster recommendations now live in
`design/checker-compiler-architecture-review.0.md` (Completion guide + §11),
which is the canonical status doc; re-run `go test -run TestCompiledCoverage -v`
for the live histogram. The figures below are the historical P0→459 ramp this
doc recorded:

Historical ratchets: **459 refused / 15 islanded** (from 651 / 115 at P0;
616 / 29 before P5; 598 / 26 after P5; 580 → 568 → 565 across carrier-identity;
555 after predicate-type provenance; 545 after if value-else; 542 after case;
538 after multi-return / 0-return / anonymous-lambda fns; 527 after apply of a
fn value; 521 after unnamed-fn map/list members; 519 after 0-value-then if
guard; 514 after concrete-args dynamic-output core builtins; 459 after module
inner natives via dot-access). Compiled rows 1706 → 1908, 0 divergences
throughout. The `case`
desugar dropped the island count 26 → 15 (islanded case rows now compile
natively).

Stage-2 lowering edges also closed this pass: a 3-arg `if` whose else arm is
a plain VALUE (`if cond [then] 42` — literal / local / type, not a `[…]`
body) now compiles (the else arm lowers to a single push). A COMPUTED else
(a paren result eagerly on the stack before the branch) and a variadic
STATEMENT-if (`if cond [raise …]` used as a guard, result discarded) still
refuse — both need stack juggling the single-result branch lowering doesn't
do yet.

## The recorder/lowerer model (shared context for the work below)

The emitter (`eng/go/emit.go`) records a linear trace of `emitEvent`s and
lowers it to bytecode. The two load-bearing data structures, and their
**single-value assumptions** that several remaining items must relax:

- `producedBy map[string]int` — value **ID → producing event seq**. One seq
  per ID. `resolveOperand(v)` consults it first (then frame locals, then a
  bare-type-node `OpPushType`, then `materialise`+`isInertConst` const).
- `lowerer.vm []int` — the simulated operand stack: **one entry per event
  result** (the producing seq). `lowerCall` matches an operand's `fromSeq`
  against `lw.vm` positions to emit pushes / a `SWAP`, asserting the result
  is where the stack discipline expects it.

Both assume **each event produces exactly one value, consumed exactly once**.
Multi-result calls and a value referenced more than once break that — see P5
and the stack-discipline item.

---

## P5 — Multi-result lowering (refusals ~44 + dup-body islands)

**Status — partially landed (616/29 → 598/26).** The `(seq, idx)` foundation
(steps 1–3 below) plus **0-result and N-result native calls** landed
gate-clean: `producedBy` is now `map[string]producer{seq,idx}`, the lowerer's
simulated stack is `[]vmSlot{seq,idx}`, an `emitOperand` carries a `resIdx`,
and `RecordCall` records side-effect (`set`/`raise`/`drop`/`printstr`/`sleep`)
and genuine multi-result words instead of refusing `len(outs) != 1`. The
empirical breakdown corrected the plan's framing: the "multi-result" refusal
bucket (34 rows) was dominated by **0-result** side-effect calls, not N-output
calls — those drove most of the −18 refusals.

**Multi-RETURN / 0-return / anonymous-lambda fns — LANDED (542 → 538).**
`StartFnCompile` no longer requires exactly one declared return; the fn unit's
single `outOp`/`hasOut` became `outOps []emitOperand` (the body residual in
stack order), `RecordUserCall` takes `outs []Value` and registers each with its
result index, `emitUserCall`/`lowerUserCall` carry `nout` (N result slots
pushed, deepest-first), and the `Finalize` fn-unit tail reconciles the N result
operands against the simulated stack (`lowerer.reconcileResults`, the fn-unit
mirror of the program-residual reconciliation) before the `OpRet` (whose
per-`Returns` type/count check was already N-capable). Empirical corrections to
the plan's "~10, mechanical" framing:

- The bucket was dominated by **anonymous lambdas** (`([n] => [n add 1]) 5`):
  `buildFnBodyReturnsFn` nils an anonymous fn's `Returns`, so it presented as
  "0 declared returns" and the old `len(declared) != 1` refusal caught it. The
  interpreter does NOT count-enforce a 0-declared fn (`def f fn [[x][][x]] f 5`
  → 5), so the count check now applies ONLY when returns are declared
  (non-empty); an undeclared fn's body residual (0 or N values) is taken as-is.
- The call-recording path only fired `RecordUserCall` for `len(out) == 1`, so
  0/N-return calls were never recorded at all (the result carriers had no
  producing event → "unknown provenance" downstream). Both the declared-N and
  the inferred (anonymous / 0-return) branches now record. The empty-body-
  residual case keeps the check-mode `[Any]` approximation and stays unrecorded
  → refuses downstream, unchanged.
- The `RecordCall` double-record guard (a dispatch a structured hook already
  recorded must not be re-refused) checked `len(outs) == 1`; generalised to
  `len(outs) > 0` so a multi-return user-fn call isn't refused as "user fn
  call (Stage 3)" by the generic path.
- Tail marking stays single-result only (a multi-return tail boundary lowers
  as a plain `CALL_USER`); an all-arms-tail branch body is tracked as diverged
  so it emits no unreachable RET.

The residual 3 "multi-return fn" refusals are **count-mismatch error rows**
(`def r2 fn [[n] [Integer] [n n]] r2 1` — declared 1, body 2): the body count
differs from the DECLARED returns, which the interpreter raises as a
return-count error. Refusing beats miscompiling here, but the rows still do
not compile, so all three stay open. A few rows that
previously refused here now reach a LATER refusal (apply / fn-value, if-branch),
which the remaining items below own.

**Deferred (still refused — open defects, not closed items):**

- **dup-body islands** (`each [dup add]`): `dup` returns `[args[0], args[0]]` —
  the SAME `Value.ID` twice (`spliceMatchResults` does not re-mint), so the two
  outputs collapse in the ID-keyed `producedBy` and the operand layout refuses
  the consume as "not adjacent." This is the **carrier-identity** item's
  territory (the next section): a multiply-emitted identical value needs
  distinct ids (or a value-def local / `DUP`), not the `(seq, idx)` machinery.

**Symptom.** `X returns N values (Stage 1 lowers single-result calls)`
(`emit.go::RecordCall`, `len(outs) != 1`), `fn … without exactly one declared
return` (`StartFnCompile`), and the `dup`-bodied higher-order islands
(`each [dup add]` — `dup` is 1-in-2-out, so its closure body refuses).

**Root cause.** The single-value model: `producedBy[id] = seq` cannot say
*which* of an event's N results an id is; `lw.vm` has one slot per event.

**Plan.**

1. **Track the result index.** Change the producer registration to
   `producedBy[id] = (seq, idx)` (a small struct, or a parallel
   `producedIdx map[string]int`). `setProduced` records every `outs[i]` with
   `idx=i`. `resolveOperand` returns the index in the `emitOperand`
   (`fromSeq` plus a new `fromIdx`).
2. **Multi-slot `lw.vm`.** Make `lw.vm` entries `(seq, idx)`. A multi-result
   event pushes N entries (`idx = 0..N-1`, deepest-first matching the
   handler's result order). `lowerCall`'s position checks compare `(seq,
   idx)` pairs.
3. **Record multi-result calls.** `RecordCall` stops refusing `len(outs) !=
   1`; it records the event and registers all outs. `OpCallNative` already
   pushes every handler result, so the VM side needs no change for natives.
4. **Multi-return fns.** `StartFnCompile` accepts `len(declared) != 1`;
   `CompiledFn.Returns` already a slice; the `OpRet` check loops over it
   (it already does); the fn-unit residual reconciliation
   (`Finalize`/`lowerFragment`) generalises from one result to N.
5. **Residual.** The program-residual reconciliation in `Finalize` already
   walks a `residual []Value`; extend it to consume N results of one event.

**Files.** `eng/go/emit.go` (the bulk — `emitOperand`, `setProduced`,
`resolveOperand`, `lowerCall`, `Finalize`, `lowerFragment`, `StartFnCompile`),
`eng/go/carrier.go` (`RecordCall` refusal). No VM change for native
multi-result; CALL_USER multi-return already returns a slice.

**Risk (the plan's top hotspot).** `lowerCall`'s stack-discipline logic is
deeply single-result; every `case 0/1/2/default` and the `SWAP` insertion
must be re-derived for `(seq, idx)` operands. Land behind the full gate;
revert if any divergence. **Recommended first** of the remaining items — it
is foundational (value-def locals and dup bodies build on it) and purely
mechanical (no fn-value/type subtlety).

---

## Carrier-identity for `make` + value-def locals (stack-discipline ~24)

**Status — LANDED (598/26 → 565/26), gate-clean across three commits.**
All three steps below shipped; the dup-body case (the deferred P5 item) came
with it. What actually landed, with the empirical corrections:

- **Step 1 — make carrier-identity (598 → 580, FP 122 → 114).** `make`'s
  constructor sigs moved from `ReturnsIdentity(0)` to a new
  `ReturnsFreshInstance(0)` (`eng/go/carrier.go`). The correction to the plan:
  returning the type literal with a **fresh ID** is NOT viable — a type
  literal is dual (its `Value.ID` IS the type's lattice identity), so a fresh
  ID severs the literal↔type link and `ValueType` can no longer resolve the
  made type (conformance fails, soundness regressed +10). The fix returns a
  fresh **value carrier** of the made type (`ValueType` + `NewCarrier`, for
  both bare type nodes AND structural type bodies like a class's
  `ObjectTypeInfo` literal). This is also more faithful than the type literal
  — conformance sees the instance's type — which DROPPED 8 false positives.
- **Step 2 — value-def locals (580 → 568).** `OpStoreLocal`
  (`eng/go/bytecode.go` + `vm.go`) plus a frame-0 promotion pre-pass
  (`planValueDefLocals` in `eng/go/lower.go`): a single-result native-call
  result referenced more than once — counting the program residual — is
  promoted to a frame local. The producing event emits `STORE_LOCAL` instead
  of leaving its result on the stack; references (rewritten in place to local
  operands) and promoted residual values re-push via `PUSH_LOCAL`.
- **Step 3 — dup carrier-identity (568 → 565).** A duplicating stack word
  (`dup` `(0,0)`, `over` `(0,1,0)`) returned the same `Value.ID` for several
  outputs; `ReturnsIdentity` now gives each output of a **repeated** source
  index a fresh ID, leaving the source's own provenance untouched. This is
  the dup-body case (`each [dup add]`) — a simple identity fix, no `OpDup`
  opcode needed.

**Deferred / not needed:** `OpDup` (step 3's "optional fast path") — the
distinct-id fix made the dup body compile without it; fn-unit value-def
promotion (the pre-pass is frame-0; no spec row needs it).

**Symptom.** `stack discipline underflow at eq/cmp/deq/is` for
`def a (make Array [1 2]) a eq a`, `(make P {}) (make P {}) eq`,
`(make Foo 1) is Foo`.

**Root cause (two distinct problems, found this session).**

1. **`make` carrier dedup.** Two identical `make P {}` calls produce carriers
   the check pass treats as the **same value id**, so `producedBy` collapses
   both eq-operands onto the LAST make's seq — the first instance is lost.
   `make` is impure (each call is a fresh instance), so its result carrier
   must be **uniquely identified per call**, unlike a pure const.
   (`NewValueRaw` already mints a fresh `ID` at run time; the dedup is on the
   check-pass CARRIER, upstream — investigate the make `ReturnsFn` /
   carrierResults memoization.)
2. **Value-def locals.** A `def`-bound COMPUTED value (an event result, not a
   const) referenced **more than once** is on the VM stack only once.
   `def a (make…) a eq a` needs `a` computed once, stored, and re-pushed.

**Plan.**

1. **Unique make carriers.** Ensure the check-pass result carrier of `make`
   (and any impure constructor) gets a fresh id per call so two identical
   calls are distinct events with distinct `producedBy` seqs. This alone
   fixes `(make P {}) (make P {}) eq` (two genuine operands → normal `case 2`).
2. **`OpStoreLocal` + value-def locals.** A multiply-referenced event result
   (count its operand references during a lowering pre-pass) is assigned a VM
   local: emit `OpStoreLocal slot` right after its event, and
   `resolveOperand` returns that `localSlot` for every reference. Re-pushes
   become `PUSH_LOCAL`. (This is the plan's deferred "F4 value-def receivers"
   generalised to any computed def value, not just const-materialisable ones.)
3. **`OpDup` (optional fast path).** For the adjacent same-value-twice shape
   (`a eq a`), once identity is correct, a `DUP` before a binary op is sound
   and cheaper than a local. NOTE: a `DUP` is **unsound before step 1** —
   it was tried this session and diverged on `(make P {}) (make P {})`
   precisely because the two distinct makes shared a carrier id.

**Files.** `eng/go/carrier.go` / the make `ReturnsFn` (carrier identity),
`eng/go/emit.go` (`OpStoreLocal`, the reference-count pre-pass,
`resolveOperand`), `eng/go/bytecode.go` + `eng/go/vm.go` (`OpStoreLocal`,
optional `OpDup`).

**Risk.** Carrier identity is load-bearing for the checker (CSE, memoization,
typing); changing it could shift diagnostics. Do step 1 in isolation, measure
the differential + check-accuracy gates, before step 2. **Recommended after
P5** (the reference-count/locals machinery is cleaner once `lw.vm` already
carries `(seq, idx)`).

---

## Fn-values-on-the-stack (`apply` + higher-order fn args, refusals ~148)

**Scope correction (from a probe of the bucket).** The "~148" was the whole
heterogeneous dynamic/opaque-output bucket; the genuine fn-VALUE rows are
~34: `fn/r apply` (named/anon ref applied to args, ~10 — the core), `apply
$.path receiver` (a Reach lens, a DIFFERENT sig, ~7), a fn value reaching an
introspection word (`typeof (inc/r)`, `Positive tcmp Positive`,
`TypeUtil.arityof (fn …)`, ~12), and higher-order afn args
(`each ([kv] => …)`, ~3).

**The shortcut DOES work — corrected (LANDED, 538 → 527).** The earlier note
claimed the re-step "does not fire in check mode," but that was an artefact of
returning a CARRIER. The fix is two lines: give `apply`'s `[Function]` sig
`ReturnsFn: ReturnsIdentity(0)` so it returns the fn VALUE **concrete** (not
widened to Any), and elide `apply`'s own dispatch in `RecordCall` (a `word ==
"apply"` + FnDef-arg guard beside the get/getr elision). A concrete fn value
landing back on the check stack IS re-stepped by `stepLiteral` →
`execFnDefLiteral` exactly as at runtime, so the fn dispatches against its
preceding stack args and records as an ordinary `CALL_USER` (its body compiled
by the normal `buildFnBodyReturnsFn` path). No new opcode, no closure push, no
stack-form `CALL_DYNAMIC`, no Finalize-residual handling — the whole "full
machinery" below proved unnecessary (an `OpCallDynamicStack` prototype compiled
0 rows and was removed). Covers `inc/r apply`, the stack-form 2-arg
`sub2/r apply` (sig position 0 = top, so `10 3 sub2/r apply = -7`), the 0-arg
`z/r apply`, and the anonymous-lambda `f/r apply`. Files: `native_ref.go`
(apply sig), `eng/go/emit.go` (RecordCall elision). The `m.f` method-through-map
and `apply $.path` Reach-lens rows are separate items.

**Original plan (superseded by the shortcut above, kept for context):** the fn
VALUE was to become a closure on the stack and `apply` lower to a stack-form
application —

**Symptom.** `unannotated or opaque word apply` for `5 inc/r apply`,
`z/r apply`, `f/r apply`; and any higher-order word handed a fn VALUE
(`each [idg] …`). The fn value can neither bake (it is code, not
`isInertConst`) nor resolve provenance.

**Root cause.** There is no way to put a runtime `FnDefInfo` value on the VM
stack. The closure machinery exists (`OpPushClosure`) but is only emitted for
code BODIES, not for fn VALUES referenced by name.

**Plan (REFINED — ready to implement; the Finalize-residual variant sidesteps
the args-inaccessibility blocker found below).**

The naive "FnDefInfo operand → closure at the apply site" does NOT work,
because `apply`'s sig is `[TFunction]` (BarrierPos 0): apply consumes ONLY the
fn; the call args (`5` in `5 inc/r apply`) sit BELOW the fn on the stack and
are never apply's args, so neither apply's handler nor a ReturnsFn can see
them. But at FINALIZE the whole residual `[5, inc-fn]` is in hand — so handle
it there, the way P4 already handles the fn-LEADING residual:

1. **Preserve + elide `apply`.** Give `apply`'s `[TFunction]` sig a ReturnsFn
   that returns args[0] unchanged (the FnDef stays concrete — `toCarrier`
   keeps `FnDefInfo`), and elide its `RecordCall` (add an `apply` case beside
   the get/getr elision in `emit.go::RecordCall`) so it neither refuses nor
   records. The FnDef then flows into the program residual as `[…args, fn]`.
2. **Finalize: residual-trailing FnDef.** When the residual ENDS with a
   concrete `FnDefInfo` for a single-sig user/anon fn, take its arity `N =
   len(Signatures[0].Params)`; the `N` residual entries before it are the call
   args. Compile the fn body to a closure unit, then emit: push the `N` arg
   operands, `OpPushClosure`, stack-form `CALL_DYNAMIC N`. (A multi-sig fn, or
   an arity that doesn't match the residual, refuses → fallback.)
3. **Compile the fn body to a unit.** `Signatures[0]` carries `Body []Value`,
   `Params []FnParam` (names), `Returns []*Type` — feed them to
   `StartFnCompile`+`AnalyseFnBody` exactly like the direct-call path
   (`core_helpers.go:367-399`), probe-guarded like `tryRecordClosure`. Use
   GENERALISED arg carriers (the residual arg types) so the unit doesn't
   constant-fold one call's values.
4. **Stack-form `CALL_DYNAMIC` (VM).** The existing `callDynamic`
   (`vm.go:205`) is residual-form (`fnVal = stack[base]`, args after). Add a
   stack-form path (`fnVal = stack[top]`, args = `stack[top-N:top]`) — a flag
   on `OpCallDynamic` or a sibling opcode. It then reuses callDynamic's
   closure → `invokeClosure` route VM-native.
5. **Value-vs-call.** A fn value bound by `def` is captured at compile time and
   never reaches runtime as an apply, so only the literal `… fn/r apply`
   residual shape triggers this; the differential gate is the backstop.

**Verified findings (this session's probe).** The bucket's genuine fn-VALUE
rows are ~34, of which `fn/r apply` (named/anon ref) is the ~10-row core. The
elision-only shortcut was implemented and ruled out (the re-step does not fire
in check mode — the fn lands in the residual unresolved). The `apply $.path`
rows are Reach LENSES (`applyReachHandler`, a different sig) and a separate
item. Introspection (`typeof (inc/r)`, `tcmp Function`, `arityof`) wants the
fn-value as a CALL_NATIVE operand, not a closure — a smaller follow-on.

**Files.** `eng/go/emit.go` (apply elision + Finalize residual-trailing-FnDef +
closure compile, reusing `compileClosureBody`/probe), `eng/go/bytecode.go` +
`vm.go` (stack-form CALL_DYNAMIC), `lang/go/native/native_ref.go` (apply sig
gains `ReturnsFn: ReturnsIdentity(0)` so the FnDef survives check mode).

**Benefit beyond apply.** This is what lets `callDynamic` stop islanding
user-fn applies (it can compile the body to a unit and run VM-native), and it
de-islands higher-order words handed a named/anonymous fn value. **The biggest
single refusal unlock.**

**Risk.** The value-vs-call ambiguity; probe-compiling arbitrary fn bodies
(captures, recursion, generics — generics already compile per-instantiation);
the stack-form calling convention.

---

## Predicate-type operands (refusals ~157, "operand provenance")

**Status — LANDED (565/26 → 555/26).** Both steps below shipped. Empirical
correction: the operand is ALWAYS a carrier at the type-algebra site (step 2
is the load-bearing half, not step 1) — `MakeDepScalarSig` is already
`RunInCheckMode`, so `Integer gt 10` produces a concrete `DepScalar` during
the check pass, but `toCarrier` strips its `DepScalarInfo` to a bare base
carrier (preserving the `Value.ID`) before it reaches `tcmp`. The constructor
now records the concrete predicate against its ID
(`EmitState.RememberOriginal`); the same-ID strip then recovers it via
`origByID`/`materialise`, and `isInertConst` admits `DepScalarInfo` so it
bakes into the const pool. The "operand provenance" bucket (which is broader
than predicate types — also generics/module/class) dropped 159 → 142.

**Symptom.** `operand of unknown provenance … at tcmp/teq/lt` for
`(Integer gt 10) tcmp (Integer gt 10)`, `(Integer gt 10) lt (Integer gt 20)`
— type-algebra and comparison over predicate (refinement) types.

**Root cause.** `(Integer gt 10)` is a `DepScalarInfo` value (a self-contained
subset type: base family + predicate, no registry per eng CLAUDE.md). At the
operand site it is a check-mode **carrier** with no recovered original, so
`resolveOperand` → `materialise` fails.

**Plan.**

1. **Const-bake concrete predicate types.** Add `DepScalarInfo` to
   `typeBodyConstOK` / `isInertConst` (`eng/go/emit.go`) so a CONCRETE
   predicate-type value bakes into the const pool (the payload is self-
   contained — base + predicate, no registry, no canonical-pointer hazard).
2. **Carrier provenance for predicate types.** Where the operand is a CARRIER
   (the common case), record the concrete predicate value in `origByID`
   (the `RecordStrip` mechanism) when `(Integer gt 10)` evaluates at check
   time, so `materialise` recovers it. Investigate whether the bounded-type /
   predicate construction path can carry its concrete value through the
   strip the way top-level literals do.

**Files.** `eng/go/emit.go` (`isInertConst`/`typeBodyConstOK`,
`materialise`/`origByID`), the predicate-type construction site
(`core_boundedtype.go` / `native_type.go`) for the strip hook.

**Risk.** Predicate types carry a `*Type` (the minted node) plus the
`DepScalarInfo` body; ensure baking by value stays canonical for matching
(`v.Is(t)`). Lower-leverage per line of effort than fn-values; sequence late.

---

## `case` clause compilation (islands ~16)

**Status — LANDED (545/26 → 542/15), gate-clean.** A `caseReturnsFn`
desugars `case v [m0 b0 …]` to the nested `if` chain `if (v m0 __casematch)
[b0] [rest]` by calling `if3ReturnsFn` with constructed list args, so the
branches record in the enclosing scope and nested clauses ride the else
BODY. Three findings, each load-bearing:

- **Match faithfulness.** `case` matches via `UnifyR`, which is LENIENT for a
  bare-refine newtype — `case 5 [Pos …]` matches `Pos` (`def Pos refine
  Integer`) — whereas `5 is Pos` is nominal-FALSE. A desugar to `is` diverges;
  the fix is an internal `__casematch v m` → `UnifyR(m, v)` the guard emits for
  non-predicate clauses (a code-body predicate clause `[pred]` stays
  `(v pred…)`).
- **No probe — static classification.** The fall-back shapes (a COMPUTED case
  value referenced in every clause; a no-default tail → variadic) are
  classified WITHOUT running the desugar: a new side-effect-free
  `EmitState.OperandRepushable(v)` (mirrors `resolveOperand`: const/local/type
  yes, computed-event no) plus the static odd-length-clause check. A
  non-compilable shape returns the prior dynamic-Any WITHOUT marking
  uncompilable, so the island / fallback keeps owning it. (Probing
  mid-recording can't roll back cleanly — a def-only `Defs.Restore` leaves
  narrowing pollution; a full `RestoreForCompile` clones `Types`, breaking
  canonical-pointer identity — which is why the probe is avoided entirely.)
- **The double-record bug (the real blocker).** `case` is in `fallbackWords`,
  so after `caseReturnsFn` recorded the branch, `tryRecordFallback` ALSO
  islanded it — the extra FALLBACK event sat unconsumed on the simulated
  stack ("residual shape … call results reordered"). Fixed by an early
  `producedBy[outs[0].ID]` guard in `tryRecordFallback` (mirroring
  `RecordCall`): a dispatch a structured ReturnsFn already recorded is never
  also islanded. This is what flipped the result from a +7 regression to a
  net win — the islanded case rows now compile natively (islands 26 → 15).

**Symptom (was).** `code-body word case (Stage 2)` — `case v [m1 b1 m2 b2 …
default]` islanded (a structured match/block clause list, not a single body).
islands (a structured match/block clause list, not a single body).

**Root cause.** `case` is a multi-way dispatcher (`native_control.go::
caseClauses`), not a one-body higher-order word, so it fits neither the
closure model nor a single CALL.

**Plan — desugar to a branch chain.** `case` is an n-way `if`: for each
`(match, block)` pair, test `v` against `match` and, on success, run `block`;
the trailing lone clause is the default. The branch-lowering machinery already
exists (`ArmBranchCapture` / `RecordBranch` / the `JMP_IF_FALSE`/`JMP`
lowering). Add a `RecordCase` that:

1. Resolves `v` to an operand (computed once → a value-def local per the
   stack-discipline item, since `v` is tested against every clause).
2. For each clause, records a guard `v is match` (a type match) or `v eq
   match` (a value match — `caseClauses` decides which) and the block as a
   captured fragment, chaining `JMP_IF_FALSE` to the next clause.
3. Records the default block (or `None`) as the final else.

It lowers exactly like a nested `if`, producing pure control flow + native
calls — no island.

**Files.** `eng/go/emit.go` (`RecordCase`/`lowerCase`, reusing branch
lowering), `eng/go/carrier.go` (route `case` to `RecordCase`),
`lang/go/native/native_control.go` (expose the match-kind per clause so the
recorder emits `is` vs `eq`).

**Risk.** Matching a clause `match` faithfully (type vs value vs predicate —
mirror `caseClauses` exactly); requires the value-def-local for `v`. Sequence
after the stack-discipline item.

---

## P7 — delete the fallback (the goal)

**Hard precondition:** both ratchets at **0** — a new
`TestEveryRowCompiles` (every `.tsv` value row yields a non-nil `Program`)
green, AND `islandCeiling == 0`. Until then deletion would turn refusals into
errors.

Note the island backstop survives in two non-obvious places that the items
above must each retire: the higher-order **island** (P5 dup + the
closure-probe refusals), the `callDynamic` user-fn **island** (fn-values-on-
the-stack lets it compile the body to a unit), and the whole-program
**fallback** (every row compiles).

**Deletions:**

- `vm.go` — `OpFallback` case + `runFallback` + `islandEng`; the `callDynamic`
  island branch (after fn-values-on-the-stack makes it VM-native).
- `bytecode.go` — `OpFallback`, `FallbackSpan`, `Program.Fallbacks`, disasm.
- `emit.go` — `RecordFallback`, `evFallback`, `lowerFallback`.
- `carrier.go` — `tryRecordFallback`, `islandPureWords`, `fallbackWords`,
  `bodyFreeForFallback`, and the `carrierResults` island call (closures + poly
  now cover their cases).
- `lang/go/boru.go` — `RunCompiled`'s `a.Run(src)` whole-program fallback
  becomes `Compile` + `RunProgram`, surfacing a compile error as an error.

Re-baseline perf/alloc afterwards (the fallback path's snapshot machinery may
simplify).

---

## Recommended sequencing

Each lands gate-clean (differential + full-corpus 0 divergences, race, alloc,
lint) and lowers a ratchet monotonically.

1. **Multi-result lowering (P5).** ✅ DONE (616/29 → 598/26). Foundational,
   mechanical; the `(seq, idx)` operand model the next items reuse.
2. **Carrier-identity for `make` + value-def locals.** ✅ DONE (598/26 →
   565/26). Cleared the stack-discipline bucket (make-vs-make/type via fresh
   value carriers, value-def `OpStoreLocal`, dup distinct-ids) and dropped 8
   false positives.
3. **Predicate-type operands.** ✅ DONE (565/26 → 555/26). Predicate
   constructor records its concrete value (`RememberOriginal`); `isInertConst`
   admits `DepScalarInfo`. (Done out of order — small, self-contained, low
   risk — while fn-values is the larger remaining semantic item.)
4. **`case` clause compilation.** ✅ DONE (545/26 → 542/15). Desugar to a
   nested `if` chain (`__casematch` for faithful matching, `OperandRepushable`
   for static classification, a `tryRecordFallback` `producedBy` guard to stop
   the double-record). Islanded case rows now compile natively.
4b. **Multi-return / 0-return / anonymous-lambda fns.** ✅ DONE (542/15 →
   538/15). N-result fn units (`outOps`, `RecordUserCall(outs)`, `nout`,
   `reconcileResults`); count-enforced only for declared returns. The deferred
   P5 follow-on; unblocked anonymous lambdas applied directly.
5. **Fn-values-on-the-stack (`apply`).** ✅ DONE (538/15 → 527/15). Two-line
   shortcut: `apply` returns the fn concrete (`ReturnsIdentity(0)`) so the check
   engine re-steps it into an ordinary CALL_USER; elide apply in RecordCall. The
   stack-form-closure machinery in the original plan proved unnecessary. The
   `m.f` method-through-map and `apply $.path` Reach rows remain.
6. **Predicate-type operands.** ✅ DONE (done out of order, see above).
7. **P7 deletion** — once both ratchets hit 0.

## Verification discipline (unchanged contract)

Run after EACH item (the existing `make verify-bytecode` sequence):

1. `make fmt && make vet && make lint && make test`.
2. `TestCompiledCoverage` — `refusalCeiling` and `islandCeiling` only ever
   move DOWN; `TestEveryRowCompiles` is the P7 gate.
3. `TestSpecCompiledDifferential` (raise `minCompiledRows`),
   `TestSpecCompiledOrFallback` — **0 divergences** (values + error taxonomy).
4. `-race` on both concurrency gates.
5. `bytecode_allocguard_test.go` — the monomorphic CALL_NATIVE hot path
   unchanged; no new hot-loop allocation.

**Gate-clean-or-defer per item:** if an item can't stay gate-clean, revert it
and re-document — the ratchets record what still refuses/islands. Commit each
landed item separately with its before/after ratchet numbers.
