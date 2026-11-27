# What the specification covers

The goal is the whole kernel. This page is the honest account of how far
that has got: what is specified and checked, and what `core/go` does
that the specification does not describe yet. A mechanism is listed as
covered only if it is modelled **and** exercised — by the corpus, by a
theorem, or by the differential search.

## Specified and checked

| Mechanism | Kernel source | Where | Checked by |
|---|---|---|---|
| The builtin lattice: nodes, parents, ranks, wire ids | `typetable.go` `builtinDecls` | `TypeTable.lean` (generated) | `verify-typetable`; `Theorems/Lattice.lean` |
| Conformance, chains, common ancestor, specificity | `types.go` | `Lattice.lean` | theorems; corpus |
| Bare-name and path resolution (ADR-012 rule 4) | `resolve.go`, `types.go` | `Lattice.lean` | corpus; `builtin_names_resolve` |
| Scalars: Integer, BigInteger, BigDecimal, Float, String (Empty/Proper), Boolean, Atom, None | `value.go`, `payload.go` | `Value.lean`, `Number.lean` | corpus |
| Type literals; plain and typed lists and maps; disjuncts; paren expressions | `value.go` | `Value.lean` | corpus; search |
| Words and their modifiers as data; the paren, end, dispatch-modifier and forward markers | `value.go` | `Value.lean` | corpus |
| Function values: overloads, parameters, patterns, `/q` and type-arg slots, the barrier, native / boru / full-stack implementations | `signature.go`, `fnsig.go` | `Value.lean`, `Match.lean` | corpus; search |
| The quoted occurrence flag (as set on a bound list) | `value.go`, `core_helpers.go` | `Value.lean`, `Engine.lean` | search |
| Canonical rendering of all of the above | `canon.go`, `value.go` | `Canon.lean`, `Number.lean` | `canon.tsv`; search |
| The total order: family comparers, rank fallback, structural compare | `compare.go`, `compare_types.go`, `compare_scalar_behaviors.go` | `Compare.lean` | `data.tsv` (through disjunct order); search; `Theorems/UnifyOrder.lean` |
| A disjunct's canonical alternatives | `core_helpers.go` `SimplifyDisjunctAlts` | `Compare.lean` | `data.tsv`; search |
| Unification: the roots and `Any`, disjuncts, the Node literal, list and map families, same-or-subtype, open map unification, the word-resolution prepass | `unify.go`, `unify_list.go`, `unify_map.go`, `unify_disjunct.go`, `match.go` | `Unify.lean` | `unify.tsv` (the predicate through `unifyq`, the meet itself through `meetq`); search; theorems |
| The binding store: per-name stacks, lookup across a name's function bindings | `deftable.go`, `registry.go` | `Match.lean` | corpus |
| Slot admission, type-literal rejection, patterns | `signature.go`, `match.go` | `Match.lean` | `dispatch.tsv`; search |
| The per-candidate scan and the plan (`/q` preference, inside-a-collection rule, zero-argument fallback) | `collect_kernel.go`, `collect_plan.go` | `Match.lean` | `dispatch.tsv`; search; `Theorems/ArgumentRule.lean` |
| The call-site modifiers `/s`, `/f`, `/N` | `collect_plan.go`, `engine.go` `effectiveForwardLimit` | `Match.lean` | `dispatch.tsv`; search |
| The `/v` modifier: the bound value, placed or collected, never called | `engine.go` `stepWordVal`, `core_ref.go` `ResolveRef` | `Engine.lean` | `dispatch.tsv`; search |
| The `/u` modifier: the bound function with its argument order reversed — the wrapper (parameters reversed, all forward, a handler that re-dispatches the original laid out for ITS barrier), `/uv` the wrapper as a value, `illegal_ref` on a name that is not a function | `engine.go` `stepWordUsurp`, `core_ref.go` `usurpOver`, `usurpDispatchHandler` | `Engine.lean` `usurpOver`, `stepWord` | `Theorems/ArgumentRule.lean` (`usurp_*`); `dispatch.tsv`; search |
| Closures: a function built at run time captures what the enclosing call frame bound — the frame baseline (none in a leaf frame), the capture rule (bound, deeper than the baseline, not a parameter or a body-local), captures installed beneath the parameters and removed with them, capture through nested closures | `fn_capture.go` `ComputeCaptures`, `bodyNeedsFrameState`, `registry.go` `PushFnBaseline`, `engine.go` `execFnDefSig` | `Engine.lean` `computeCaptures`, `bodyNeedsFrameState`, `execMatch`; `Match.lean` `Defs.pushBaseline` | `Theorems/Closure.lean`; `divergent.tsv` (Go column); search |
| An ANONYMOUS function at the pointer: called when something it can take stands beside it, otherwise a value — never `uncalled_function` | `engine.go` `execFnDefLiteral` | `Engine.lean` `dispatchFnValue` | `divergent.tsv` (Go column); search |
| Driven loops: a looping word hands the engine a driver; each iteration is a sealed region of its own — the driver's inputs, inert, then the body — whose residual goes to the driver and is never stepped again; the driver's results stand in the loop's place | `loop.go` `LoopDriver`, `StartLoop`, `stepMoveDriven`, `spliceDrivenIteration` | `Engine.lean` `Driver`, `driveLoop`, `runRegion` | `Theorems/Loop.lean`; `divergent.tsv` (Go column); search |
| Counted and conditional loops: a round is `mark body… move` with no paren, so the body runs on the caller's stack; the move keeps what the round left (a pending container evaluated there, the round's bindings live), rebinds the index and replays the body under a new mark, or puts every round's values where the loop stood — stepped from there; the index's scope (the levels a round bound above it end with the round, the index with the loop); a condition region's LAST value decides, and none is an error; a move whose mark is gone is dropped | `engine.go` `stepMark`, `stepMove`, `stepMoveCont`, `stepMoveWhile`, `collectLoopRegion`, `popIterLevels`; `value.go` `Loop` | `Value.lean` `mark`, `move`, `Loop`; `Engine.lean` `stepMove`, `collectRegion`; `Match.lean` `Defs.popTo`, `Defs.freshMark` | `Theorems/Counted.lean`; `divergent.tsv` (Go column); search |
| Flow control: a signal a word raises is resolved after the step, on the nearest counted or conditional loop whose move stands ahead on the tape — break splices the completed rounds' values and unbinds the index, continue drops the round and leaves the pointer on the move; the call frames the signal escapes are closed; a signal passes through a group (abandoned whole unless a loop inside it takes the signal), a container literal, a call frame's residual and a driven loop; with no loop it is `flow_error` | `flowctrl.go`; `engine.go` `handleLoopBreak`, `handleLoopContinue`, `exitWithFlowCtrl`, `evalParenGroupAt`, `containerEscaped`; `fn_frame.go` `unwindLiveFrames`; `sealed.go` `escapeSealed` | `Match.lean` `Defs.flow?`, `raiseFlow`; `Engine.lean` `handleFlow`, `unwindFrames`, `groupFlow`, and the signal check after every step and nested run | `Theorems/Flow.lean`; `divergent.tsv` (Go column); search |
| The two moves that are no loops: the `if` continuation — a condition region whose last value picks the branch that takes the region's place, an error when it leaves none — and the plain mark and move, which replays the mark's body once; a flow signal passes both | `engine.go` `stepMove`, `stepMoveIf`, `regionLast`; `value.go` `IfCont`, `MarkInfo.Body` | `Value.lean` `Loop.branch`, `Loop.replay`; `Engine.lean` `stepMove` | `Theorems/Branch.lean`; `divergent.tsv` (Go column); search |
| The one truthiness rule: presence, not content | `core_helpers.go` `CoerceBoolean` | `Truth.lean` `coerceBoolean` | `Theorems/Counted.lean` (`truth_is_presence`, `runs_while_true`); `divergent.tsv` (Go column); search |
| Arity widening of a value call: a function value whose collection has completed is sealed to the stack unless a wider overload could take the next token in a position it collects forward — then it is planned again in full (and NUR401, as the kernel has it: planned again though the wider overload does not fill) | `engine.go` `fnValueWouldWiden`, `forwardClaimProbe` | `Engine.lean` `wouldWiden`, `stepLiteral` | `divergent.tsv` (Go column); `dispatch.tsv` (the word beside it); search |
| Function identity: a function is equal only to itself — in unification and in a disjunct's reduction — a reference carries the identity of the one function bound to its name, and a wrapper is a new function | `compare.go` `sameFnIdentity`, `registry.go` `aggregateIdent`, `value.go` `NewFunction` | `Value.lean` (`fn … ident`), `Compare.lean` `sameValue`, `Match.lean` `Defs.refIdent` | `Theorems/UnifyOrder.lean` (`function_identity`); `unify.tsv`; `dispatch.tsv`; search |
| The dispatch order: which of a name's several definitions a call takes — more arguments first, then the greater expectation at the first position that differs (a pattern before its family's bare type, a type before every type above it, a branch root before `Any`), then a piped barrier, then the newest definition; one order however the definitions were made, and the order a reference lists them in | `signature.go` `CompareSignatures`, `SortSignatures`, `registry.go` `aggregateDispatch` | `Match.lean` `Sig.order`, `Defs.insertSig`, `Defs.lookup?` | `Theorems/Overload.lean`; `dispatch.tsv`; search |
| A failed plan: retried with every argument from the stack — for a native word, a function literal and a forced arity, and NOT for a word defined in boru, whose synthetic fallback answers it (NUR398, as the kernel has it) | `engine.go` `matchForDispatch`, `registry.go` `fnFallbackSig` | `Engine.lean` `answersItself`, `dispatchWord` | `dispatch.tsv`; `divergent.tsv` (Go column); search |
| The stack-form probe | `signature.go` `MatchSignature` | `Match.lean` | search |
| The step loop: literals and arrival, word resolution, word literals, undefined words | `engine.go` | `Engine.lean` | corpus; search |
| Parking and collecting a forward; completion and the stack-only re-dispatch | `engine.go` | `Engine.lean` | corpus; search; theorems |
| The phase-1 plan walk: group pre-evaluation, overload pruning | `collect_kernel.go` `CollectForward` | `Engine.lean` | `dispatch.tsv`; search |
| The strict forward barrier and the barrier commit | `engine.go` | `Engine.lean` | `dispatch.tsv`; search; theorems |
| Paren groups: collapse, re-step, void groups and `no_value_error` | `engine.go` | `Engine.lean` | corpus; search |
| `end`; curry-or-stack, including the curry list; orphaned forwards | `engine.go` | `Engine.lean` | corpus; search |
| Running a match: argument evaluation, result splice, full-stack handlers | `engine.go` `execMatch` | `Engine.lean` | corpus; search |
| A NAMED function value at the pointer: it dispatches as its word would, is sealed to the stack once its forward arguments are in, is `uncalled_function` when it matches nothing it could have been called on, and is PLACED — not called — when a paren group collapses to it alone | `engine.go` `execFnDefLiteral`, `fnReturnPark` | `Engine.lean` | `dispatch.tsv`; search |
| Boru-bodied calls: the frame, named parameters, cleanup, the return check | `core_helpers.go`, `engine.go` | `Engine.lean` | `dispatch.tsv`; search |
| Container literals as sealed regions; map member evaluation; the residual sweep | `sealed.go`, `engine.go` | `Engine.lean` | `data.tsv`; search |
| Inert type-shape resolution at consumption and in the residual | `engine.go` | `Engine.lean` | search |

Only an error's taxonomy **code** is specified. Messages, source
positions, hints and did-you-mean suggestions are not.

## Not specified yet

Each line is a kernel mechanism with no counterpart here. Most also have
no expression in the `core/spec` notation, so specifying them means
extending the notation and its fixture in all three runners first.

**The step loop**
- an applied mark, a function from another registry, unnamed parameters
  and the frame argument span, the args stack, the truncation of a
  frame's body-local definitions, tail-call frame elision, return
  patterns;
- the dispatch order's two inputs the fixture cannot reach: a pattern
  against a bare type across two overloads of one word, and `CoreDefault`
  (every fixture word has one signature, and `fn(` declares types only);
  and `Extends`;
- call regions; a loop's own step budget, its context layer, and the
  attribution of a body's fault; where a `flow_error` is reported (only
  its code is specified); `move_error`, for a move whose mark was never
  stepped (no token stream a word builds has one);
- splice markers, sugar markers, reaches, interpolated strings, XML;
- macros and the raw-capture slots (`FormArgs`, `RawParens`,
  `NoEvalArgs`);
- the step budget's accounting, the tape growth ceiling, tracing.

**Types and values**
- every type beyond the builtin table: minting and installation, typed
  defs, refinement and predicate types, dependent scalars, negation,
  enums, records, options, tables, classes, resources, stores, generics
  and type parameters, bounded types, the Micron family, the flexible
  containers;
- the unifier's arms for those, and the retagging of a meet's result;
- the equality family (`eq`, `deq`, identity) and hashing.

**Everything that is not the run**
- the analysis pass: carriers, check state, diagnostics;
- modules, capabilities, policy hooks, contexts, processes.

## Next on the proof side

- make the machine reduce inside Lean's kernel, so the finite theorems
  need no `native_decide`;
- prove the argument rule for all tapes at the level of `planSig`, not
  only for the instance families in `Theorems/ArgumentRule.lean`;
- prove conformance a partial order for every well-formed table, not only
  the builtin one.
