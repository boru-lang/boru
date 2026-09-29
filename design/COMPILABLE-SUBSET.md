# The Compilable Subset — a positive specification

The bytecode compiler (`eng/go/emit.go`, `lower.go`, `vm.go`, `bytecode.go`)
is **the carrier type-checker run with a recording side effect**: every
dispatch the checker resolves in a typed region is recorded as a classified
event, and `Finalize` linearises the trace into a `Program`. Anything the
recorder cannot prove it can lower **faithfully** stops there — and every such
stop is a **defect**: an unimplemented or unproven case, owed a fix and
tracked to closure. Done is a language that compiles the way a developer
expects — ALL valid code compiles, no exceptions. The interpreter is **not** a
fallback for the compiler, and as of 2026-09-19 it cannot become one: every
mechanism that re-ran a failed program on it has been removed, so a program
that does not compile is a plain error.

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

A **compile failure** is a breach of that contract, not a second branch of it.
When `CompileCheck` returns `(nil, reason)`, the `reason` names the first
offending construct — which is to say it names the defect, and that defect is
owed a fix and tracked to closure.

**The containment machinery is gone (2026-09-19).** `RunCompiled` used to roll
the registry back to its pre-check snapshot and **silently** re-run the
program on the interpreter: an answer came back and nothing in the run said
the compile had failed. So did `Run`, the CLI's try mode (with a warning), the
`BORU_COMPILE_FALLBACK=1` hatch, a runtime bail after the program had already
started, the fn-value seam, the detached-callback invoker, and an `await`
branch. All of them are removed. A program that does not compile returns
`compile_failed` naming the construct; a compiled program that bails returns
the defect, annotated as one.

What that buys is not correctness — the answers were right — but visibility.
A failure that hides itself is the harder one to find and the easier one to
bank on, and the day the removal landed it surfaced three miscompiles the
fallback had been absorbing. The debt it made visible is counted, in
`test/go/langspec/compile_failures.tsv`, `lang/go/compile_defect_test.go` and
`lang/go/test/compile_defect_test.go` — ratchets on a bug count, never budgets.

Compilation is sound by construction + the differential and property gates (§8);
there is no independent proof. A compile failure does not produce a *wrong*
answer, but "not wrong" is not the bar here — compiling is.

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
| └ call-site SPECIALISED unit (2026-09-27) | the call passes a CONSTANT fn (a `/v` reference, a lambda literal) to a NAMED `Function` param of a single-signature, non-generic, capture-free fn whose home is the dispatching registry; the fn arg has an identity and no captures, generic spec, macro splice or modifier wrap, and shares that home. The body compiles with the param bound to the fn as the interpreter's frame binding binds it (`InstallFrameBinding` — the body's reads dispatch, error and render as the param), so a call through it records the fn's own unit and types its result by the fn's declared return. It declines — and the pass re-runs with specialisation off, from the registry as it was — when the specialised body's residual misses the declared returns, when a call through the param passes an argument the callee's contract may refuse at run time (an untyped one, or one whose type or bound lies outside the param's), or when a fn VALUE the body constructs captures the param. The retry holds the first pass's output and never repeats an unrepeatable effect (a counted effect, a stdin read): such a pass that needed the retry does not compile (NUR236) | the call records `CALL_USER` into the specialised unit (no generic unit is compiled beside it); `CompiledFn.SpecGuards` checks each guarded arg's fn identity (`core.ExactEqual`) at entry, before any of the body runs, and on a failed guard applies `SpecFallback` — the fn itself — to the signature args in an island over them as resolved stack data (an escaped break/continue is the enclosing loop's; otherwise a tail call leaves the frame through `RET`). A code body naming the param binds it registry-visible at entry (`placeFnParam`). The instance switch is `SetCallSiteSpecialisation`; a source that never names `Function` compiles in one pass with it off |
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
  be a typed non-dynamic carrier (a computed `keys` result). Since S1a
  (2026-09-19, [FULL-COMPILATION-REPLAN.0.md](FULL-COMPILATION-REPLAN.0.md)
  §5) `each`/`fold`/`scan`/`filter` declare `CompileDynBody`, so a
  gradual-Any operand — the COLLECTION (`c:Any` at run time a List or a
  Map, where the pair-vs-KeyVal convention is ambiguous) or the CALLBACK (a
  class field, a map field, a dynamic key, a factory result, a code body
  read from a flex) — no longer refuses at the ambiguous-overload gate: the
  site lowers to a CALL_NATIVE poly re-match over the word's own overloads
  (`tryRecordDynBody`, arming DynEnv mode) and the handler picks the
  overload the live value matches, running the callback through the
  RunResolved seam. That seam is counted by the interp-entry census
  (52 → 102 rows) and the engine-entry census (366 → 489), the G-lane-first
  landing the re-plan names, which S1b retires by lowering the re-matched
  overload's body natively — its first increment (2026-09-19) does so for a
  fn VALUE callback: the VM's body seam dispatches the value natively
  (`eng/go/vm_fnvalue_seam.go` — the interpreter's match in the token seam's
  top-down order, the value's unit hosted at its home with the token seam's
  return discipline), the unit stamped at first application and memoised on
  the value (`compiler.LazyStampFnSig`; a registry-mutating body is never
  stamped lazily), and the fn-VALUE seam (InvokeCallbackFn) stamps the same
  way. A TOKEN body read at run time still runs through RunResolved (S3).
  Still refusing: `for-each` (nets no result, so
  the dyn-body seat cannot hold it) and `walk` (its own code-body gate) over
  the same shapes, captures on the extras/hook path, and unreachable
  captures. A multi-overload lambda at the four words takes the same seat.
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
- **Fn-operand words that RE-STEP their operand** (the pilot's former open
  half, 7 of the census's fn-operand signatures — declared `CompileResteps`
  on 2026-09-25, S2a): `apply [Function]` — the recorder owns it by NAME
  (`recordCallElided`'s fn-value elision, the pending-apply window,
  `OpCallDynTrailTop`) and no flag says "applies its operand through the
  Apply kernel"; `CompileReadsFn` would be a permissive lie (a top-level
  fn-typed carrier would bake a `CALL_NATIVE` whose handler leaves the marked
  fn as DATA — the S1 fn-value line owns the word). `mini` / `parse` /
  `emit` value forms (`(Function String Map)`, `(Function String)`,
  `(Function Map Any)`, `(Function Any)`, `(Function Any Any)`) — the
  handler returns a SPLICE, `<fn> <src> <opts> end`, that applies the fn on
  the tape: a token-returning macro, the brief's REWRITE class, not an
  admission (`CompileReadsFn`/`CompileStoresFn` promise the fn is never
  invoked on the tape). What they now declare is the REFUSAL itself:
  `CompileResteps` (core/go/value.go), read by `RecordCallOperands` before
  any inert-slot admission and by the two quoted-operand gates before
  `quotedKeySig` / `quoteInertOK` — the triple's `tapeBound: Yes` as a
  flag rather than the zero value's silence. The check pass still models
  the expansion (`RunInCheck`) and the recorder still refuses the residual
  (`mini`/`emit` over a computed fn: "residual value of unknown
  provenance") or records the parser dispatch (`parse`,
  `recordParseLangFnDispatch` + `FnDataArgs`); the same flag sits on the
  by-name modifier forms, `valof` and the kind-quoted `mini`/`parse`/`emit`
  overloads, whose result is a wrapper, a parked binding or a splice.
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
  `lang/spec/fn-value.tsv` §8). Widened 2026-09-22 (S1b's apply shapes):
  the window records wherever the lead is a slot of the RECORDING UNIT —
  inside a branch arm, a loop body, a `do` body, a callback lambda
  (`each$body` / `fold$body`; the former nesting and closure-unit
  exclusions are gone) — and its one argument may be a gradual value
  whose DECLARED bound excludes Function (a callback lambda's `e:Integer`
  param, narrowed from the callback's Any element carrier by
  `narrowLambdaInputs`) or an INERT fn value (`(f g/v)`, a lambda
  literal: `RecordDynApplyLead` binds it to the lead's Function param as
  the interpreter's forward collection does; a bare fn WORD still
  declines — NUR123's word dispatch). What still declines: a gradual
  `x:Any` argument (the Church-chain family, §5.8 of the legacy HOF note),
  and a 0-arg runtime lead is NUR176 (loud on both lanes, a value only on
  the interpreter). Pinned by `TestApplyShapesParity`. The `apply` WORD at
  the MAIN program over a lead the check cannot type compiles since the
  dynamic-lead group (2026-09-22): a gradual lead over one receiver is the
  apply EVENT (`OpCallDynApplyOne` — one result or a loud defer), a
  produced fn-typed carrier is the program unit's pending apply, lowered as
  the whole-residual `OpCallDynApplyTop` (the word's own semantics: a
  closure of the window's arity runs VM-native, a 0-arg one fires above
  the window, a wider one under-applies through the re-step); pinned by
  `top_level_apply_test.go`. A fn-valued CONTAINER MEMBER applied through
  a paren-bounded call, `(fs.b 10)`, compiles over a produced closure too
  (the container-member calls, 2026-09-22): a map literal's computed
  closure member is no longer const-folded (`OpMakeMap` assembles the map,
  as `OpMakeList` always did), a fn-typed carrier member tags the read as
  a member-fn read, `each` over a fn VALUE callback types its result by the
  fn's declared return, and the paren lead admits a statically fn-typed
  value beside the member tag; a `dynamic(Any)` lead — a flex field on the
  compile pass, where the shape is not threaded — still declines; pinned by
  `container_member_call_test.go`. The def-split spelling
  (`def r (f x) f r`) graduated the same day: `checkModeParenFnCollapse`
  killed its checker false positive on the plain surface, and
  `replayIsBodyTail`'s `windowReadsID` arm (a dyn-bind of a value the
  window reads is not a reorderable event) armed its body tail — both
  spellings now compile natively (completeness-review §9.8). Pinned by
  `TestChainedForwardApplyCompiles` / `TestMultiArgChainedApplyFailsToCompile` /
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
  `TestQuoteLambdaCallbackParity` + `eng/go/quote_lambda_screen_test.go`. A paren over a PRODUCED closure — a compiled factory call's result, or
  an earlier apply's — re-stepped by the paren's rewind over the data
  arguments its closure provably takes compiles as the same apply event,
  recorded AT THE COLLAPSE (the curried chain, 2026-09-22: core's
  `parenProducedLeadApplyIdx` over the recorder's `ProducedLeadApplies`
  seam, which holds the window to the closure's declared arity and param
  types and types the result), so `(((mk3 1) 2) 3)` is three events and
  `((mk 1) 2) mul 10` no longer leaks its window to `mul` (NUR178). It
  stands aside for a def-read lead (the read model's), inside an
  unnamed-param frame (NUR180) and for a collapse on behalf of a pending
  forward collection (nothing re-steps there); a window the closure might
  not take — wider, narrower, a type that does not conform, a gradual that
  may be a fn — stays with the residual classifier, which declines it
  loudly. Pinned by `curried_chain_test.go`.

  A fn-valued CONTAINER MEMBER read as the LAST token of a CODE BODY
  (`each [ops.inc] xs`, `each [M.tbl.inc] xs`) compiles since the
  quotation-body container reads (2026-09-22): the interpreter's reach
  collapse re-steps the member over the element beneath, and the
  code-body closure unit now takes the whole-frame replay
  (`OpCallDynFrame`) a named fn unit already took for the identical
  residual (`noteClosureBodyReplay` — the trigger is the check pass's
  fn-member tag on the top value, never a bare fn-typed carrier, which is
  a WORD dispatch, NUR123). Inside ANY unit a paren-PLACED value is data
  (NUR182): the fn-unit replay skips it, the layout re-pushes it, and
  `each [(m.f)] xs` is a list of the fn — except a `do` body's placed
  value with siblings, which its caller re-steps and which stays
  declined. What still declines: a member read in a WORD's forward slot
  inside a fold body (`0 fold [add ops.inc] xs`). Pinned by
  `quotation_body_member_read_test.go`.

  A module-scope DEF bound to such a member, read BARE inside a code body
  (`def f tbl.inc end each [f] xs`, `each [dup f] xs`, `do [5 f]`), and a
  `/v`-MARKED reach group under the `apply` word (`5 M.inc/v apply`,
  `"s" M.inc/v apply`) compile since the quotation-body def reads
  (2026-09-22, NUR156): the def read is the interpreter's WORD dispatch
  under the binding name — the replay carries the name, a match applies
  natively, a no-match raises `cannot call `f`` through the word island,
  which dispatches a name the registry already binds through that
  binding — and the check model of `apply` delivers its value unquoted,
  as the runtime handler does. What still declines or diverges: the
  same def read at the MAIN program (`5 f`) and inside a named fn unit
  (NUR123's open points, pinned), and a dynamic-scope def of the apply's
  result inside a loop body (`while […] [def i (i M.inc/v apply)]`, the
  dynamic-scope def family). Pinned by `quotation_body_def_read_test.go`.

  A call result is PARKED where it lands on the compiled lane too,
  whatever route the call took (2026-09-23, NUR181): a shaped method call
  over a def-bound factory closure (`def r (mk3 1) end 2 r 3` is
  `[2 fn]`), a compiled fn-value apply of one (`2 ((mk3 1) 3)`), and a
  named factory's result above a literal (`5 (mk 3)`, which declined
  "call result above a literal") all seat as the parked pair with parity,
  the mixed and trailing windows included (`7 (r 3) 2`, `r 3 4`). The
  recorder resolves the callee's closure unit from the method value's own
  producer, and the program residual's ordering treats a parked result as
  data. What still declines: a bare read of the def-bound closure short
  of its arity (`def r ((mk3 1) 2) end r`, the read's statement window)
  and the shuffles the interpreter re-steps at the shuffle (`5 (mk 3) 1
  roll`, NUR124's timing axis at the main program). Pinned by
  `def_bound_closure_park_test.go`.

  A typed word over a trailing paren apply's Any result inside an
  UNNAMED-param frame (`xs each [(2 (mk 1)) mul 10]`, `def f fn
  [[Integer][Integer][(2 (mk 1)) mul 10]]`) compiles with parity since the
  recovery's window (2026-09-23, NUR180): the checker's unmatched-dispatch
  recovery lays the window it poly-records out FORWARD-FIRST, as the
  interpreter's matcher does — the written argument fills the leading
  position, the stack the rest — where it used to prefer the stack and
  took the frame's input for the written `10`. A paren's TRAILING fn
  value forward-collects the tokens past the `)` on both lanes since the
  trailing value's re-step (2026-09-23, NUR184): `(2 (mk 1)) 10` is `[2
  11]`, `(2 (mk 1)) 10 20` `[2 11 20]` (the mixed island's interpreter
  seals a compiled closure at its collection's completion), `(2 inc2/v)
  10 mul` 24, `xs each [(2 (mk 1)) 10]` `[[11 11 11]]`, and a paren
  closing under a pending forward hands its survivors to it — `0 fold
  [add (2 (mk 1))] xs` is 6, the fn-unit `10 mul (2 (mk 1))` 21, `10 mul
  (2 (mk 1)) 5` `[20 6]`. The collapse records the trailing apply only
  for a lead the interpreter dispatches INSIDE the paren (a bare read of
  a fn-typed binding, an `apply`-owned lead) or a value nothing after the
  close can collect; a forward's leftover fn value is marked and never
  applied over later values. Still DECLINING, pinned
  (`TestParenTrailingFnSoundCompileFailures`): the leftover at the main
  program (`10 mul (2 (mk 1))`), a leftover with nothing beneath it (`def
  r (2 (mk 1)) end r` — def takes the 2, the closure parks) and a list
  literal whose trailing element the rewind re-steps (`[(2 (mk 1)) 10]`),
  and a numeric word after the follower whose poly record collected the
  re-step-marked carrier (`(2 (mk 1)) 10 mul`, 22 interpreted — the
  closure takes the 10 first). A `/v` read of a def-bound
  closure is placed on both lanes (NUR185, 2026-09-23): `def c (mk 3) end
  2 c/v 10` used to island the window and apply the closure; it declines
  at the render gate now (the value renders under the def's name). A
  def-bound computed fn READ as the whole of a closure body over the
  element beneath — `def a5 (mk 5) end each [a5] [1 2 3]`, `0 fold [s]
  xs` over a two-argument `s`, `filter [g1] xs`, a fn-util wrapper `each
  [h] xs` — is the interpreter's word dispatch over the frame's values
  (2026-09-24, the def-bound computed fn read at a closure body's tail):
  the check pass stands aside when the statement's window is short but
  the frame holds exactly the arity, and the body lowers the read's live
  lookup plus `OpCallDynTrailTop` at the claimed arity with the name
  seated, so a no-match raises `cannot call `a5`` alike. Declining to the
  island, as before: a deeper frame (`0 fold [s] xs` over a one-argument
  `s`, the check's short window). A fn-body-LOCAL computed def read the
  same way from a nested closure compiles too (NUR192, 2026-09-24): the
  frame's dynamic-scope bind pushes the closure value the lookup reads
  and pops it with the frame (`def g fn [[xs:List][List][def a5 (mk 5)
  each [a5] xs]] end g [1 2 3]`), and the raw-body read under a later
  word islands to the same pushed closure. A fn body's computed fn def
  SHADOWING an enclosing frame's computed fn of the same name DECLINES
  (the interpreter's install outlives the call; the compiled push-and-pop
  cannot model it), pinned in `TestDefFnBodyTailShadowSoundCompileFailures`.
  Inside a `do` body the same read is the word's dispatch over the body's
  EMPTY frame or its written tokens (NUR193, 2026-09-24): `7 do [a5]` is
  `[7 error(cannot call …)]` on both lanes (the caught raise), `do [a5 7]`
  12 — a closure a compiled program binds by `def` dispatches as the named
  fn it is inside every island, and `do`'s result model takes the
  computed-body hatch over such a carrier. A written operand the contract
  does not take (`each [a5 "s" add] xs` is `['6s' '7s']`: the matcher
  falls back from the token to the element; `do [a5 "s"]` the caught
  raise) agrees too (NUR194, 2026-09-24): the shape claim carries the
  wrapper's parameter types and the read window declines a token that
  does not fit, so the body islands to the bridged dispatch; at the top
  level `(a5 "s")` declines the program to the interpreter's raise.

  A BRANCH whose arm is a fn VALUE returns a value the interpreter
  re-steps, and the compiled lane re-steps it the same way since the
  branch result's re-step (2026-09-23, NUR159): a named 0-arg fn fires at
  the merge (`if true one/v [2]` is 1 — at the main program, in a paren,
  a def, a fn body and a code body), a 0-arg lambda parks, and an
  arg-taking fn takes the token after it (`if true inc/v [2] 5` is 6) or
  the values beneath (`7 if true inc/v [2]` is 8, `1 7 if true inc/v [2]`
  `[1 8]`, `xs each [if true inc/v [2]]` `[2 3]`), a paren placing the
  lone value (`7 (if true inc/v [2])` is the pair) and an enclosing paren
  re-stepping it (`(7 (if true inc/v [2]))` is 8). The merge widens the
  fn arm's type to Word, so the recorder's branch event carries the fact
  (`MayBeFn`, with an arg-taking flag) and every collapse-side and
  residual-side gate asks it. Still DECLINING, pinned
  (`TestBranchFnValueSoundCompileFailures`): a later dispatch that
  collected the branch's value (`7 if true inc/v [2] add 1`, 9
  interpreted — inc over 7 first), an arg-taking value interior to the
  residual past a statement boundary (`7 if true inc/v [2] ; 3`, `[8 3]`),
  a list literal's element with siblings (`[7 if true inc/v [2]]`), a fn
  body's residual over a param (`def g fn [[m:Integer][Any][m if true
  inc/v [2]]] end (g 7)`, 8), and a def bound to an arg-taking result
  (`def x (if true inc/v [2]) x 5`, 6 — the interpreter dispatches the
  NAME as a word). A statement boundary ends a value's collection on both
  lanes (NUR187, the same day): `m.f ; 5` is `[fn 5]`, `if true inc/v [2]
  ; 5` the same, and `7 m.f ; 3` — the fn over the 7 beneath, the 3 the
  next statement's — declines loudly where it islanded to `[7 4]`.

  A NAMED fn value's re-step raises or parks as the interpreter's does
  since the named fn value's candidates (2026-09-23, NUR186): with a
  CANDIDATE — a value beneath, a FUNCTION word after (a word bound to a
  value is collected instead: `m.f k` with `def k 2` is 3), or a fn
  frame's tail markers after the body's last token — and no matching
  overload it raises
  `uncalled_function` (`def m {f: M.inc} def g fn [[][Any][m.f]] end (g)`,
  `m.f three`, `"s" M.inc`), with none it stays data (`m.f`, `m.f ; 5`,
  `do [m.f]`, `[1] each [drop m.f]`); a 0-arg member fires either way.
  Under a FUNCTION word a DYNAMIC fn value is re-stepped over that word
  by the landing's overload walk (2026-09-23, NUR190): the interpreter's
  own plan over the value and the word decides — the zero-argument
  fallback fires (`m.f z` with a nullary and a unary overload is `[42
  0]`), an anonymous fn parks (`m.l z` is `[fn lam(Integer) 0]`), an
  Any-typed slot's claim strands (`m.a z` raises `signature_error` on
  both lanes); a `/q` capture RUNS where the lowering laid the word's
  argument-free call and the residual apply out right after the landing
  (2026-09-26: the landing enters the capturing overload over the word as
  an atom and resumes past both — `m.f z` is `[z]`, fn-value.tsv
  L317/L318) and DEFERS loudly elsewhere (`m.f typeof`, `m.f y 5`), as a
  Function-typed reference does (2026-09-24, the maintainer's call — a
  compile-time decline would be over-wide, no static model telling a `/q`
  slot from a typed slot's barrier; the reference's claim waits on
  NUR220), and the corpus keeps any such row on the runtime-defers ledger,
  `runtime_defers.tsv` (empty since 2026-09-26). A
  CONCRETE named fn at a fn or lambda frame's tail (`def mk fn
  [[][Function][M.inc]] end (mk)`, the original witness) DECLINES the unit
  with the interpreter's raise pinned beside it — the unit-level trap is
  the follow-on — and so does dispatch wreckage a word collected inside a
  body (`[M.inc typeof]`). A native poly that collected a container
  member's fn value written before the word declines (NUR188: `7 m.f
  typeof`, Integer interpreted; `typeof m.f` stays), a paren-PLACED member
  is data in every residual arm (NUR189: `7 (m.f)` and `1 7 (m.f)` are the
  placed pair), a function word after a value ends its collection (NUR187:
  `7 m.f three` is `[8 3]`, declining where it islanded to `[7 4]`; `7 m.f
  k` stays `[7 3]`), and a fn body's whole-frame replay never crosses a
  `;` (`[M.inc ; 5]`).

  A TYPED lambda callback over a HETEROGENEOUS list is applied only to the
  elements its signature admits on both lanes since the typed callback's
  contract (2026-09-23, NUR155): `each ([x:Integer] => [typeof x]) [1 'a'
  [2] {b:1} true none]` is `[Integer fn fn fn fn fn]` — a lambda the
  compiler lowers as the word's own body unit (each$body, fold$body) is
  matched against its recorded contract per element by the VM's token
  seam, a list body's no-match leaving the fn value as the element's
  result and a map body's raising the map arm's signature_error. The
  `apply` word over a factory's baked fn const under a dirty stack (`7 5
  (mk) apply`, NUR160) and over an empty window (`(mk0) apply` fires a
  0-arg closure, `(mk) apply` leaves a wider one as data) compiles with
  parity the same day. Still diverging, pinned (NUR186): a MODULE fn's
  value at a factory body's tail (`def mk fn [[][Function][M.inc]]`),
  which the interpreter's landing dispatches and raises on, and the
  compiled unit returns.

- **Compile refusals recorded 2026-09-28 — all five compile with parity since
  the same day's defect pass** (moved from NUR.md under its scope ruling;
  kept here as the record of how each closed):
  - A seeded generator's method read through a map member (formerly
    NUR331): FIXED — the refusal ("undefined word: rand-int") masked a
    wrong answer, the check-time MODEL of `Rand.with-seed` folded into the
    container; the model's registry is marked (`Registry.ShapeModel`) and
    never baked, the fold declines an event-produced binding, and a folded
    fn value is queued in its home registry (`FnHome`). `m.r.int 0 100` is
    `[61]`; `do [m.r.int 0 100]` stays a sound decline (shaped method apply).
    NUR341 records the literal twin that was still a wrong answer on main.
  - An `if` stranded in a `case` clause list (formerly NUR332): FIXED — the
    recovery (`checkModeAssumeSig`) handed a best-fit overload's ReturnsFn a
    SHORT window and `if3ReturnsFn` read `args[0]` of nothing (a recovered
    panic). `declaredReturnCarriers` never calls a ReturnsFn with fewer
    operands than it declares, twelve ReturnsFns guard their own window, and
    a definite no-match inside a sealed `if` arm records an arm-scoped trap
    (`RecordArmTrapErr`), so the program raises the interpreter's
    signature_error byte for byte.
  - `def P (refine Integer) def drop fn [[x:P] [] []] end do [1 drop] depth`:
    FIXED — `shuffleDispatchSafe` admits a user overload that only consumes
    its arguments; `[0]` on both lanes.
  - `[1 Integer] each [add 1]`: FIXED — the suspended probe no longer latches
    the program's failure, and the optimistic dispatch's exact layout rides
    its poly record (`PolyRef.Split`), so the VM raises the interpreter's
    per-element signature_error.
  - `def mk fn [[][Any][7]] end def j (mk) end j 5 j typeof` (and `j 5 [j]`,
    `j (range 0 3)`): FIXED — `residualCallableExempt` lets the residual
    rebuild seat a gradual root read (its NUR207 guard owns the fn case) and
    a call result held once; the pick/roll twins still decline.
- **Open refusals recorded 2026-09-28:**
  - A `case` clause block that is a bare FUNCTION word (NUR342, a wrong
    answer on main before) compiles where the check pass decides the taken
    clause (a known scalar scrutinee, pure scalar guards), or where the
    case sits bare on the root stream or in its own fn frame (2026-09-29).
    It still declines for a code-body scrutinee, a guard with an effect, a
    raise, a word match or a non-comparison word, or a run-only scrutinee
    with values around a non-bare case: that remainder is owed a lowering
    of a run-time-chosen re-step.
  - CLOSED 2026-09-29: a parked paren-bounded fn-value apply over a
    container member with an effect (`({a:(print 1)} lam/v) print 2`,
    NUR337's remainder) compiles — the decline was a false NUR121
    collection-hazard mark on a forward-only collection.
- **Open refusal recorded 2026-09-29:**
  - A trap under a dispatch whose operand is a run-time-counted region (a
    loop, or a count-varying branch) written after the word in a paren —
    `each (for 1 [mk]) [x/u]` — declines with "rematch operands include a
    variadic loop result" (NUR340, a wrong answer on main before): the
    interpreter spreads the region into the call's arguments and the
    re-match window has no fixed count to read. The region beneath the word
    (`if c [99] [1 2] each [x/u]`) still compiles and agrees.
  - A region BENEATH the word that may be empty — a 0-or-1 branch merge, a
    loop — declines the same way (`if c [] [1] each [x/u]`, the Codex review
    of #521): the push-and-swap seat needs a region proven non-empty.
  - A `Function` param passed on bare to a recursive call —
    `def h fn [[g:Function n:Integer][Integer][if (n lte 0) [0] [(g n) add (h g (n sub 1))]]] end h inc/v 5`
    — declines with "unmatched dispatch recovered at h" (formerly NUR234: the
    generic unit answered `[20]` where the interpreter raises
    `signature_error`, the bare name being a word dispatch of `g`). The
    compile of the interpreter's raise is owed.
  - A source naming `Function` whose compile pass makes an effect the retry
    cannot repeat (a counted effect — a file write, a network send — or a
    stdin read, in a module body or a check-mode word) and whose call-site
    specialisation declines does not compile ("call-site specialisation
    declined after an unrepeatable check-pass effect"; formerly NUR236). The
    effect happens once, never twice; the cure is a specialisation that
    declines less often or unwinds in place without a re-run.
  - `for 2 [("s" lam/v) (2 add 3)]` declines with "branch leaves extra
    values" since the NUR344 fix (2026-09-29), as its literal twin
    `for 2 [("s" lam/v) 5]` always did: a for body leaving several values
    has no compiled seat. It had compiled through an end-of-run island.
  - A dispatch that takes a member read the run may find callable beside a
    value written after it (`8 m.f 7 eq`, NUR349 — fixed at the root and in
    a plain unit) declines where no landing can guard it: inside a branch
    arm (`if c [8 m.f 7 eq] [0]`), a loop body (`for 1 [8 m.f 7 eq]`), or a
    unit with an apply chain or trailing apply. Some of these were silent
    wrong answers on main (`if c [8 m.f 7 eq] [0]` was `[8 false]` for
    `[true]`).
  - `Rand.map-from` over a literal schema whose body binds a name declines
    ("twin regime: a bind transition has no stream placement", NUR330,
    2026-09-29): the generator's bodies are run-time token bodies the
    compiled program does not model, and the def outlives the call on the
    interpreter. This includes bodies whose binding is never read later
    (`def k 5 Rand.map-from {a:[def k 1 k]} drop 0`), which compiled and
    agreed before — the price of refusing the silent case. Bodies that bind
    nothing compile as before.
  - A module fn whose frame skips the args list (NUR346's leaf elision) and
    reads `args` through a word macro bound after it declines with
    "context-dependent word args" (2026-09-29, the Codex review of #522):
    one compiled unit serves both the importer's call (real args, via
    CallBoru) and the module's own (the empty list), and the VM keeps the
    args stack on the program's registry where the module's `args` reads its
    own. The importer-side call compiled correctly before; the inside-module
    call was a silent wrong answer. Owed: per-registry args frames in the VM.
- **Open refusals recorded 2026-09-29 (the body-map pass):**
  - `Rand.map-from` over a schema held as a carrier (a Map or List param)
    whose body binds a name the calling fn does not define declines
    ("residual value of unknown provenance" / the arm-read decline): the
    interpreter keeps that def past the fn's frame and the compiled frame
    unwinding does not. This includes `f {a:[1]} k` over such an `f`, which
    compiled before. Where the fn defines the name first it compiles.
  - A map-from body that reads an open counted loop's index (by name, or any
    computed or opaque body inside a counted loop) declines, as does a body
    that escapes with break/continue outside any compiled loop at the root
    (NUR353): the index lives in a frame slot a run-time token body cannot
    resolve, and the interpreter's flow_error position depends on its tape.
  - `def w2 word [1 2] end def mk fn [[][List][quote [print "q" w2/v]]] end def w word [do (mk) 3] w w`
    halts the check pass ("undefined stack entry at position 2").
  - `def k 5 size {a:[undef k 1]} k` is refused by the check pass
    ("undefined word: k") where the interpreter raises undefined_word at run
    time.

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
