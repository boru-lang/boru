# The Compilable Subset — a positive specification

The bytecode compiler (`eng/go/emit.go`, `lower.go`, `vm.go`, `bytecode.go`)
is **the carrier type-checker run with a recording side effect**: every
dispatch the checker resolves in a typed region is recorded as a classified
event, and `Finalize` linearises the trace into a `Program`. Anything the
recorder cannot prove it can lower **faithfully** is refused — and every such
refusal is a **defect**: an unimplemented or unproven case, owed a fix and
tracked to closure. Done is a language that compiles the way a developer
expects — ALL valid code compiles, no exceptions. The interpreter is **not** a
fallback for the compiler and is not allowed to become one; that
`lang.(*Boru).RunCompiled` currently re-runs a refused program on the
interpreter — **silently** — is **scaffolding** absorbing that defect, never a
reason a refusal is acceptable. The silence makes it worse, not better: the
failure hides itself, so nothing in the run says the compile was refused.

This document is the **positive** statement of what compiles and why. The code
expresses the subset as a pile of refusal gates (chiefly `EmitState.RecordCall`
and `isInertConst`); read those for the exact edge, but read **this** first for
the rule each gate is defending. Each gate is also a standing defect report: it
marks a case the recorder cannot yet prove, which is work outstanding, not a
boundary the design has chosen. When you widen the subset, update this file in
lockstep — the gates should be a checklist against a stated rule, not the rule
itself.

> Authority: the code is authoritative for behaviour; this page is the index and
> the rationale. If they disagree, the code wins and this page is stale — fix it.

---

## 1. The contract

The contract has ONE outcome, not two: **every valid source program compiles.**
`CompileCheck` returns a non-nil `*Program`; `RunProgram` executes it and returns
a residual byte-identical to the interpreter's `Run` for the same source (the one
documented exception is §7).

A **refusal** is a breach of that contract, not a second branch of it. When
`CompileCheck` returns `(nil, reason)`, the `reason` names the first offending
construct — which is to say it names the defect, and that defect is owed a fix
and tracked to closure.

The containment machinery is real and is described here accurately, as
machinery: `RunCompiled` rolls the registry back to its pre-check snapshot
(`CompileSandbox`) and **silently** re-runs the program on the interpreter — an
answer comes back, and nothing in the run says the compile was refused. That is
scaffolding around a known bug, and the silence indicts it rather than excusing
it: a failure that hides itself is the harder one to find and the easier one to
bank on. Not a supported outcome, not a success criterion, and never a reason to
leave a construct uncompiled.

Compilation is sound by construction + the differential and property gates (§8);
there is no independent proof. A refusal does not produce a *wrong* answer, but
"not wrong" is not the bar here — compiling is.

---

## 2. The provenance model (why anything compiles at all)

Every dispatch argument must resolve to a known **operand** (`emitOperand`):

| Operand kind | Source | Lowers to |
|---|---|---|
| `opConst` | an inert literal interned in `Program.Consts` (§4) | `PUSH_CONST` |
| `opType` | a bare type node, resolved by canonical ID at run time | `PUSH_TYPE` |
| `opLocal` | a frame-local slot (loop iterator, fn param, value-def local) | `PUSH_LOCAL` |
| `opEvent` | the result of a prior recorded event, live on the VM stack | (already on the stack) |
| `opClosure` | a compiled body unit (a higher-order word's code body) | `PUSH_CLOSURE` |

Provenance rides `Value.ID`: `RecordStrip` / `RememberOriginal` save a literal's
original before the checker strips it to a carrier, and `setProduced` maps each
event output's ID to its producing event. An argument whose ID resolves to none
of the above is **unknown provenance** → refuse. A computed value is on the
simulated stack *exactly once*; a value referenced more than once is either a
re-pushable operand (const/local/type) or gets promoted to a value-def local
(`planValueDefLocals` → `STORE_LOCAL`).

**Provenance across a branch join.** An enclosing local merely READ inside
an `if` arm is narrowed-through-use, which preserves its `Value.ID` — but
`RunCarrierBodyWithDefs` reports the narrowed binding as an arm `add`, and
`InstallJoinedDefs` must NOT re-seat it as a fresh-ID `JoinCarriers` result
(that would strand every reference after the join as an unseated dynamic
carrier — "fn call operand of unknown provenance"). `joinBranchDef` keeps
the shared ID when both arm carriers are the same identity (a narrow-only,
identity no-op merge), preserving the seat; a GENUINE reassignment gives
the arms differing IDs and keeps the fresh-ID join (a branch RESULT must
not collide with an arm's live local). Pinned by
`lang/go/bytecode_ifbranch_operand_test.go` (reduced from the aless
viewer's render fn — voxgig-boru/aless, dx-report 2026-07-21).

---

## 3. What compiles (by construct)

| Construct | Compiles when… | Lowering |
|---|---|---|
| Native call | the checker selected ONE signature (`SiteMono`), all operands resolve, no fn-valued / quoted / code-body / dynamic operand | `CALL_NATIVE` |
| Polymorphic native | the checker widened a dynamic operand to `Any` across overloads (`RecordPolyCall`) | `CALL_NATIVE_POLY` (run-time `MatchSignature`) |
| Literal push | the value is an inert const (§4) | `PUSH_CONST` |
| Type operand | a bare type node with a registered canonical ID | `PUSH_TYPE` (by-ID, never a stale by-value copy) |
| `if` / `case` | each arm's result resolves; arms may diverge (break/continue/tail/`raise`) or be variadic (0-or-1, only the program residual may absorb it) | `JMP_IF_FALSE` + arm fragments + merge |
| Counted `for` | start/step are consts, the body nets ≤1 value/iteration, range is not a runtime-assembled list | `FOR_SETUP` / `FOR_NEXT` + back-edge `JMP` |
| `break` / `continue` | inside a compiled loop | `JMP` to loop end / `FOR_NEXT` |
| User fn (`def f fn […]`) | checked, ≤ the staged return shape; recursion via forward-ref; generics one unit per memoised instantiation | `CALL_USER` / `TAIL_CALL_USER` + `RET` (return-type checked) |
| Higher-order code body | the body compiles to a capture-resolved closure unit and the driving word invokes it via the VM seam | `PUSH_CLOSURE` + `CALL_NATIVE` |
| └ `var`-body that captures/refs an enclosing binding | the `var` cleanup is emitted as the 1-arg-only `__varundef` (not the overloaded `undef`), so the body's dynamic-Any residual can no longer mis-match `undef name fnUndefSpec`'s `TFnUndef` slot in check mode — the cleanup dispatches identically (1-arg unbind) in check and at runtime, so the loop binding never leaks into the capture set | same closure unit; cleanup lowers to a `__varundef` `CALL_NATIVE` |
| Fn-value as DATA | introspection (`typeof`/`arityof`/…) or a residual/member, never an INVOKED fn value | baked const / `OpCallDynamic` at the residual boundary |
| Fn-value APPLIED at a fn-body tail | the body's count-mismatched residual carries a Function-typed **or DYNAMIC** value (a map get over `Any` — the boru:fmt stylesheet driver `def apply fn [nd:Any Any [nd (rules get (Fmt.kind nd))]]`), and the recorded trace proves the window is the body's LAST statement (no event recorded after the window's last producer — `replayIsBodyTail`); an out-of-order residual re-pushes in exact token order via `replayForceOrder` promotion | `OpCallDynFrame` + `RetReplay`: the frame's token region re-steps at the RET under `execFnDefLiteral`'s own runtime rule — a callable value applies exactly as the interpreter's pointer would (forward collection included), a NON-callable value stays data and the RET raises the interpreter's own return-count error |
| Computed list literal | top-level only, every element a core-builtin (deterministic) result or const | `MAKE_LIST n` |
| Computed map literal | every value operand resolves AND the map is CONSUMED in-frame (a word/fn arg, incl. `make`'s body) — not a deferred residual (a bare map tail, evaluated after its frame pops). Sound in fn bodies / branches / loops: `OpMakeMap` re-assembles a fresh map per run, never frozen | `OpMakeMap` (keys ride in `MakeMaps`, values popped) |
| └ list-valued entry (`{n:[expr]}`, the `do {map}` idiom) | the value list's elements all resolve; the list WRAPPER is recorded inline (interleaved per value, in stack order) as a nested `OpMakeList`, bypassing its top-frame guard because it is a consumed operand of the enclosing in-frame `OpMakeMap` | nested `OpMakeList` then `OpMakeMap` |
| Multi-/0-result words | any (P5): the VM pushes every handler result | `CALL_NATIVE` (nout slots) |
| Typed value-def over a refinement (`def x:Pos n`, `def v:Positive x`, `def v:(Integer gt 10) x`) | the constraint is a predicate type / bare-refine newtype / DepScalar subset and the DYNAMIC body operand resolves; the bind runs the interpreter's own validate/reparent (`RunTypedBind`) over the runtime value — raise on failure is the byte-identical plain error, a passing value reparents where `defTypedHandler` reparents. A CONCRETE body keeps the proven const-pool reparent (no `BIND_TYPED` emitted); a statically-failing body stays a check-diagnostics row | `BIND_TYPED` (spec in `TypedBinds`; the STORE stays with the value-def local plan — `STORE_LOCAL` when promoted, `DROP` after validation when the binding is dead) |

---

## 4. The const pool — the load-bearing mutation-safety invariant

`isInertConst` is the whitelist of values that may live in `Program.Consts`. A
pooled const is pushed by the **same pointer-backed `Value`** on every
execution, including each loop iteration, so it MUST be immutable. The whitelist
admits only: scalars, temporal values, predicate/refine `DepScalarInfo`,
structural type bodies (`typeBodyConstOK`), fn-signature descriptors, surfaces,
generic schemas, inert reach lenses, and lists/maps of the same. It MUST NEVER
admit a mutable instance type (`Array` / `Object` / `Store`): one of those in a
pooled const would be corrupted in place by an `set` across iterations.

This is the single sharpest soundness edge. It is pinned by
`eng/go/bytecode_constbake_test.go::TestIsInertConstRejectsMutableInstances`
(mutable instances rejected; immutable scalars/compounds accepted). **Before
adding any type to `isInertConst`, prove it is immutable or that its mutators
can never reach a pooled const, and extend that test.**

Compounds (lists/maps) and type bodies are **never deduped** — `eq` on compounds
is identity, so two source literals must stay two consts with two IDs.

---

## 5. What fails to compile today (the open-error inventory)

> **Islanding: zero, and not negotiable (maintainer, 2026-09-17).** An island
> is a region of a COMPILED program that still runs on the interpreter — the
> fallback in miniature, inside something the compiler has called a success.
> There is no island budget and no island ledger; `islandGate` stays 0 and the
> gate stays red until the count is 0.
>
> The corpus expansion of 2026-09-17 exposed **15 islanding rows**, all one
> family: **the callback is a fn VALUE rather than a literal body, and usually
> returns a compound value.** In full, so they are findable:
>
> | row | shape |
> |---|---|
> | `each-variants.tsv` L126, L127 | `each pair/v [1 2]` — a named fn value returning a List / a Map |
> | `each-variants.tsv` L128, L129, L159 | a `KeyVal` lambda over a Map returning a Map or List |
> | `callbacks.tsv` L104, L105, L106 | a POLY fn value as the callback; a branch-selected fn value; a fn value returning a List |
> | `callbacks.tsv` L54, L75, L85 | a callback read from a container; a factory-built closure passed to `each` |
> | `fold-map-filter.tsv` L87, L88, L200 | `fold` with a Map accumulator; a `Function`-typed accumulator; a list of fn values folded |
> | `fold-map-filter.tsv` L168 | `each` over a grouped result with an `Any` lambda parameter |
>
> This is the same root family as the 113 compile ERRORS the same expansion
> exposed:
> a fn value crossing a boundary the emitter cannot follow. Closing it closes
> both counts at once, which is why it is the highest-leverage target in §5.


**The generated sweep's matrix is the second ledger** (2026-09-18,
[FULL-COMPILATION-REPLAN.0.md](FULL-COMPILATION-REPLAN.0.md) S0):
`test/go/langspec/SWEEP_STATUS.md` lists, for every declaration-relevant
word × operand kind, the cell that fails to compile, islands or diverges,
and every call-form variant that does — 44, 5, 3 and 200 on its first
run, plus 2 call forms the compiler panics on — with `TestGeneratedSweep`
ratcheting the counts. The inventory
below names the mechanisms; the matrix names the programs.

Every entry below is an unimplemented or unproven case — a defect against §1,
owed a fix and tracked to closure, never a sanctioned design outcome.
`RecordCall` and friends latch the program uncompilable on the first of these,
and the §1 scaffolding then re-runs the whole program on the interpreter so the
user still gets an answer while the case is open:

- **Compile-time word** — `RunInCheckMode` (def/import/type/macro/Test). Runs at
  compile time, emits nothing; a residual that depends on its *runtime* error
  (a check-mode-suppressed error) also refuses.
- **Context-dependent word** — `args` / `__pa` (no per-call args stack in a VM
  frame). `FullStack` words (depth/pick/roll) GRADUATED for provably-exact
  stacks (2026-08-03, `EmitState.FoldFullStack`): the dispatch folds
  statically — depth's count bakes as a concrete const, pick re-pushes the
  picked entry, roll re-seats the true permutation — and ELIDES (no opcode,
  no event). The exactness gate is the soundness argument: top frame of the
  top unit, no open mark window, every stack entry a known operand home with
  no variadic producer, concrete in-range `n`. Anything else declines to the
  historical refusal. Pinned by `eng/go/fold_fullstack_test.go` +
  corpus-core rows; the remaining event-permutation sub-frontier is
  ledgered in `frontier-full-stack.tsv`.
- **Fn-INVOKING word** — `apply` of a non-re-stepped value, `is` over a
  predicate fn: their handlers re-step the fn on the tape, which the VM cannot
  honour. Since the type-node fusion
  ([legacy/TYPE-REPRESENTATION.1.ignore](legacy/TYPE-REPRESENTATION.1.ignore) §9) a named
  PREDICATE TYPE evaluates to its minted node rather than the fn value, and
  the recorder refuses that node at the same words via `IsPredicateTypeNode`
  ("function value reaches <word> (Stage 3)") — the refusal surface is
  unchanged, only the carrier moved. (Fn-INTROSPECTION words are exempt —
  they only read the value.) A
  higher-order form over a LAMBDA value (filter/each/fold/scan, and walk's
  hook slots) compiles to a closure unit via `tryRecordLambdaClosure` when the
  lambda has a single own sig and the word has a callback convention; the BODY
  lambda may carry LEXICAL captures (resolved to compiled homes and threaded
  at OpPushClosure — the mini-redis KEYS shape) and the collection operand may
  be a typed non-dynamic carrier (a computed `keys` result). Still refusing:
  multi-overload lambdas, DYNAMIC (gradual) collections (the pair-vs-KeyVal
  convention is ambiguous), captures on the extras/hook path, and unreachable
  captures.
- **Quoted-operand word** — usurp / force-arity / ref-family (results re-stepped)
  — except `get`/`getr`/`set` and module-inner natives over inert atom keys.
- **Dispatch-modifier VALUE form over a computed fn** (the fn-operand pilot
  of [HANDLER-MIGRATION-LINE.0.md](HANDLER-MIGRATION-LINE.0.md), 2026-09-18)
  — `usurp` / `stack-args` / `forward-args` / `force-arity` over a TYPED
  `Function` carrier: a user fn's declared `Function` result (`def r (usurp
  (mk 100))  r 10 3`). The value-form sigs now declare `CompileStoresFn |
  CompileFnHandlerStrict` — the native reads the fn's shape, stores the
  original in the wrapper (`FnDefInfo.Wraps`) for the later re-dispatch, and
  VALIDATES it as an `FnDefInfo` — and the only recorder seat these
  check-mode words reach is the gradual poly record (`recordGradualWrap`),
  which reads no declaration on the compiler side: it lowered the wrap to
  `OpCallNativePoly`, the VM handed the native a `ClosurePayload`, and the
  validation raised `illegal_ref` where the interpreter wraps the closure
  (NUR158). The word's check-mode half honours the declaration by declining
  the poly record, so the residual refuses (`… of unknown provenance`); a
  capture-free returned fn refuses with it, since the carrier cannot tell
  the two apart. The DYNAMIC Function carrier a sibling modifier's gradual
  wrap produced (`usurp (forward-args (m.s))`, path-modifier.tsv:52-55) and
  the dynamic-Any `m.a` read (path-modifier.tsv:17) keep their poly record
  and compile; the `m.a` form over a closure is NUR158's open half.
- **Fn-operand words still undeclared** (the pilot's open half, 7 of the
  census's 11 fn-operand signatures): `apply [Function]` — the recorder owns
  it by NAME (`recordCallElided`'s fn-value elision, the pending-apply
  window, `OpCallDynTrailTop`) and no flag says "applies its operand through
  the Apply kernel"; `CompileReadsFn` would be a permissive lie (a top-level
  fn-typed carrier would bake a `CALL_NATIVE` whose handler leaves the marked
  fn as DATA — the S1 fn-value line owns the word). `mini` / `parse` /
  `emit` value forms (`(Function String Map)`, `(Function String)`,
  `(Function Map Any)`, `(Function Any)`, `(Function Any Any)`) — the
  handler returns a SPLICE, `<fn> <src> <opts> end`, that applies the fn on
  the tape: a token-returning macro, the brief's REWRITE class, not a
  declaration (`CompileReadsFn`/`CompileStoresFn` promise the fn is never
  invoked on the tape). The check pass models the expansion (`RunInCheck`)
  and the recorder refuses the residual (`mini`/`emit` over a computed fn:
  "residual value of unknown provenance") or records the parser dispatch
  (`parse`, `recordParseLangFnDispatch` + `FnDataArgs`).
- **Code-body word** — a `NoEvalArgs` body that is not inert (a computed paren,
  or a body carrying a flow-control sentinel that targets an enclosing frame).
- **Code-body naming a FN-LOCAL fn** (NUR037) — a body word whose current
  binding is a `Function` installed INSIDE the enclosing fn being compiled
  (the ComputeCaptures scope rule: `Depth(name) > TopFnBaseline()[name]`),
  e.g. `def step fn […]` followed by `for-each [step] xs` in the same body.
  The interpreter resolves the name per run through the def stack; a compiled
  program never executed the enclosing body's def, so any admission that
  baked the NAME — the island span, the CALL_NATIVE const-bake, the closure
  probe — turned a working program into `undefined_word`. `bodyRefsFnLocalFn`
  (carrier.go) refused at the `recordDispatchOutcome` seam, gating all three
  paths at once. Since the seventy-second increment the def is PLACED there
  instead (`placeFnLocalDef`: a registry-visible install for the frame, the
  seventieth increment's lowering), so a capture-free local fn compiles on
  every path; a CAPTURING local fn — a closure the placement cannot bake —
  keeps the refusal. Deliberately narrow: module-scope callbacks (`TopFnBaseline`
  nil, or depth ≤ baseline) and fn-local VALUE defs (the closure path's
  lexical captures) keep compiling, as do structured-lowering words (if /
  for) and structurally-desugared dispatches (case's branch chain), whose
  bodies record as inline events with no name bake. Closure capture of
  fn-local fn bindings remains a possible future widening.
- **Fn body redefining a SPECULATIVE-FAMILY name** (NUR149, the
  seventy-third increment) — a capture-free `def NAME fn […]` inside a fn
  body whose NAME is a speculative fn family (a fn a branch arm the model
  could not decide defined — the seventieth increment). A capture-free
  redefinition is normally sound (it takes the compiled `BindDefReplace`
  twin), but the family's dispatches route with a LIVE LEAD, and this
  in-place overlap replace is the family-L leak inside a fn body: the
  drop-then-push leaves the frame's def depth unchanged, so the interpreter
  keeps the shadow past the call while the compiled def lowers to nothing
  and the live lead resolves the arm's binding (whose unit no call site
  compiled) or an unbound name. No compiled twin reproduces a frame-local
  shadow the interpreter does not tear down, so `InstallDef` refuses (the
  family-L block's fourth arm, `specFnJoin`). A DISJOINT signature (a fresh
  frame push, placed by the seventy-second increment) and a NON-family
  overlap keep compiling.
- **Dynamic input / opaque output** — a dynamic carrier reached the site, or the
  checker could not type the result (unannotated / opaque wrapper) — except a
  concrete-args core builtin whose dynamic result is merely a declared-`Any`
  return (`dynOutNativeOK`).
- **Anonymous / fn-value dispatch**, **operand of unknown provenance**, **a fn
  value preceding residual args** (auto-dispatch boundary), and any operand
  shape beyond the lowerer's stack discipline (`layoutOperands` refusals).
  The fn-body BODY-TAIL dynamic apply graduated to `OpCallDynFrame` (§3);
  what still refuses here is the MID-BODY shape — an event recorded after
  the apply window's producer (`[nd (m get "k") print "after"]`), whose
  effects a RET-time replay would reorder — pinned by
  `TestEdgeFindingDynamicFnValueApplyBodyTail` (§6, edge findings) — and
  the **CHAINED forward apply over a MULTI-ARG inner group** (`f (g x y)`):
  the replay window may hold at most ONE applicable value
  (`noteDynFrameReplay`'s `replayApplicables == 1` arm, 2026-08-02 — the
  flat re-push loses the inner group's collapse, so a two-applicable
  window compiled the RET count error where the interpreter applies both
  fns). The ONE-ARG chain graduated 2026-08-03 (the Stage-G single-arg
  increment): `(g x)` with `g` a named param/capture collapses to a
  `RecordDynApply` event at the paren — one-arg leading and trailing
  collection converge inside a sealed named frame, at every runtime arity
  — so `compose`, `twice`, and mid-body `(g x) add 100` compile NATIVELY
  (`EmitState.DynApplyLeadEligible` gates the admission; rows in
  `lang/spec/fn-value.tsv` §8). The def-split spelling
  (`def r (f x) f r`) graduated the same day: `checkModeParenFnCollapse`
  killed its checker false positive on the plain surface, and
  `replayIsBodyTail`'s `windowReadsID` arm (a dyn-bind of a value the
  window reads is not a reorderable event) armed its body tail — both
  spellings now compile natively (completeness-review §9.8). Pinned by
  `TestChainedForwardApplyCompiles` / `TestMultiArgChainedApplyRefuses` /
  `TestTailProofNegatives`; only the sel1 control remains in
  `frontier-chained-apply.tsv`; graduation of the rest = Stage G proper.
- **Quote-typed lambda callback** — a lambda whose param is Atom-typed (a
  /q quote-capture slot, `fn_params.go`) admitted as a HOF callback over a
  COMPUTED collection: the runtime never binds a delivered stack value to
  a /q slot, so the interpreter leaves the lambda as DATA where the
  compiled callback APPLIED it (`each [[k:Atom] => […]] (keys m)` —
  compiled `signature_error` vs interpreted `[fn (Atom)]`). Screened at
  BOTH admissions (2026-08-02): `lambdaHookCompatible`'s quote arm
  (lambda-value bodies) and `quoteParamCarrierBind` at the user-fn
  dispatch record (token bodies, closure units only). Pinned by
  `TestQuoteLambdaCallbackParity` + `eng/go/quote_lambda_screen_test.go`.

The **branch-join narrow-preservation** rule (§2) removed a former
over-refusal here — an enclosing local read inside both `if` arms and
reused after the join now compiles.

---

## 6. The execution-environment seams

- **Fallback islands** (`OpFallback`) re-run a recorded token span through a
  reused sub-engine, threading the operand stack. Soundness rests on island runs
  being non-nested/non-concurrent within a VM run. An island is containment, not
  a lowering: it marks a span the emitter could not yet lower — a defect scoped
  down to that span — and each one is owed a real lowering.
- **`OpCallDynamic`** applies a runtime fn value to trailing residual args (the
  `r.int 0 100` method-field boundary), leaving a non-callable value untouched —
  faithful to the interpreter either way.
- **Resource ceilings** mirror the interpreter: the VM value stack and frame
  depth share the tape's bounded-growth ceiling (`tape_exhausted`); the step
  budget raises `evaluation_limit` (but see §7).
- **Concurrency**: a registry must not be driven by two executions at once. The
  `vmRunning` CAS rejects an overlapping compiled run; an `interpRunActive()`
  check rejects starting a compiled run while an interpreter run is in flight.
  Neither can catch a foreign interpreter run STARTING on another goroutine
  once the compiled run is underway (the VM's own islands re-enter `Engine.Run`
  on the same registry, indistinguishable without goroutine identity) — that
  shape stays caller responsibility: one `*Registry` per goroutine.

---

## 7. The one semantic divergence — the step budget

`Run` meters its step budget per **tape token**; `RunProgram` meters the same
cap per **bytecode instruction**. The compiled stream is leaner than the
expanded token walk, so the VM reaches at least as far as the interpreter before
the cap. The divergence is **one-directional**: a long-but-terminating program
the interpreter would abort with `evaluation_limit` may COMPLETE under
compilation; the reverse never happens. A genuine runaway trips fast in both.

So at the ceiling, `Run` and `RunCompiled` are observably different programs;
everywhere below it they agree. This is the *only* place the opt-in flag is not
result-transparent. Pinned by
`lang/go/bytecode_findings_test.go::TestStepBudgetNoSpuriousLimit`; the property
fuzzer keeps its corpus well under the cap so the divergence never makes it
flaky.

---

## 8. The safety net

Compilation is sound by construction + these gates — keep them green and crank
them when widening the subset:

- **`test/go/langspec/compiled_differential_test.go`** — every spec row the
  emitter accepts must match the interpreter; a `minCompiledRows` floor catches
  a regression that silently refuses everything.
- **`test/go/langspec/compiled_property_test.go`** — generated well-typed
  programs (arithmetic, `if`/`for`/`case`, `each`/`fold`/`scan`/`filter`,
  maps, array/object mutation, closures/captures, apply/usurp, direct named-fn
  calls) checked for the same agreement, with shrinking. Crank via
  `BORU_FUZZ_SEEDS` / `BORU_FUZZ_ITERS`.
- **`-tags borudebug`** (`make verify-bytecode`) — re-runs the differential and
  property gates with a fresh args slice per `CALL_NATIVE`, so a native that
  illegally retains its args slice corrupts nothing and is localised here.
- **`eng/go/bytecode_constbake_test.go`** — pins the §4 mutation-safety
  whitelist.
- **`make verify-bytecode`** — the full bracket: fmt/vet/lint, the gates above,
  `-race` concurrency gates, and the borudebug lane.
