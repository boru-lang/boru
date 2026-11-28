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
| Call regions: a native whose last act is running a body returns the run as a driven loop of one round — the inputs sealed and inert, the body stepped where the word stands — whose residual replaces the region as the word's results and is stepped again | `loop.go` `CallRegion`, `callDriver` | `Engine.lean` `Driver`, `driveLoop` (a driver of one round that answers with its residual; `Corespec/Fixture.lean` `callq`) | `Theorems/Loop.lean` (`a_call_region_*`); `divergent.tsv` (Go column); search |
| Function identity: a function is equal only to itself — in unification and in a disjunct's reduction — a reference carries the identity of the one function bound to its name, and a wrapper is a new function | `compare.go` `sameFnIdentity`, `registry.go` `aggregateIdent`, `value.go` `NewFunction` | `Value.lean` (`fn … ident`), `Compare.lean` `sameValue`, `Match.lean` `Defs.refIdent` | `Theorems/UnifyOrder.lean` (`function_identity`); `unify.tsv`; `dispatch.tsv`; search |
| The dispatch order: which of a name's several definitions a call takes — more arguments first, then the greater expectation at the first position that differs (a pattern before its family's bare type, a type before every type above it, a branch root before `Any`), then a piped barrier, then the newest definition; one order however the definitions were made, and the order a reference lists them in | `signature.go` `CompareSignatures`, `SortSignatures`, `registry.go` `aggregateDispatch` | `Match.lean` `Sig.order`, `Defs.insertSig`, `Defs.lookup?` | `Theorems/Overload.lean`; `dispatch.tsv`; search |
| A call's arguments: an UNNAMED parameter's argument is not bound — it is the frame's own stack data, the frame's first tokens in signature order, resolved where the call was made and skipped by every stepping loop (the frame's argument span); the return check lets as many values pass beneath the declared returns as there are unnamed parameters, and drops them (NUR404, as the kernel has it: a count, whatever the values are). The ARGS LIST: every argument of the call, in signature order, pushed beside the frame and popped with the frame's baseline — by the close, by an error, by a flow signal that leaves the frame | `engine.go` `execFnDefSig`, `stepPastOpenParen`, `openArgSpan`, the return check in the frame's close; `core_helpers.go` (the frame skeleton); `value.go` `NewFrameOpenSpan`, `ReturnCheckInfo.UnnamedCount`; `argsstack.go`; `registry.go` `PopFrameArgs` | `Value.lean` `frameOpen`, `openSpan`, `returnCheck`; `Engine.lean` `execMatch`, `closeFrame`, `popNames`; `Match.lean` `Defs.pushArgs`, `popArgs`, `topArgs?` | `Theorems/Frame.lean`; `dispatch.tsv`; `divergent.tsv` (Go column); search |
| The frame's close: the ENTRY SNAPSHOT — how many bindings every name holds once the frame has bound its captures and parameters — and the truncation to it, so what a body binds ends with the call and a binding it shadowed is read again; the close only unbinds (a name the body unbound stays unbound); then the args list, the baseline and ONE binding of each of the frame's names (NUR406, as the kernel has it: by count, so a body that unbound its parameter costs the caller a binding); a LEAF frame — one whose body names no binding word, nor a name bound to a binding word's function — records nothing and undoes nothing (NUR384, as the kernel has it); the question is asked when an INSTALLED function is made and at every call of a function VALUE; a signal that leaves the frame closes it the same way | `engine.go` `stepDefCleanup`, `truncateFrameDefs`; `fn_frame.go` `FrameTailSpec.Snapshot`, `unwindFrameTail`; `fn_capture.go` `frameStateWords`, `bodyNeedsFrameState`, `fnValueIsBindingWord`; `core_helpers.go` `buildFnBodyHandler` and `engine.go` `execFnDefSig` (when it is asked) | `Value.lean` `defCleanup`, `Impl.boru` (`frameState`); `Match.lean` `Defs.depths`, `truncateTo`; `Engine.lean` `popNames`, `execMatch`, `bodyNeedsFrameState`, `standsForBindingWord`; `Corespec/Notation.lean` `install` | `Theorems/Locals.lean`; `dispatch.tsv`; `divergent.tsv` (Go column); search |
| The residual rule: the pending containers a body leaves are evaluated in its frame, before anything is unbound — unless the function is anonymous and its body is one bare token, whose container leaves the frame as written and is evaluated by what consumes it | `fn_frame.go` `ResidualEvalsInFrame`, `BodyEvalsResidual`; `engine.go` `stepDefCleanup` | `Value.lean` `Impl.boru` (`anonymous`), `defCleanup` (`evalResidual`); `Engine.lean` `bodyEvalsResidual`, `evalFrameResidual` | `Theorems/Locals.lean` (`the_residual_is_evaluated_first`, `an_anonymous_bare_container_is_deferred`); `dispatch.tsv`; `divergent.tsv` (Go column); search |
| Tail calls: the specification NESTS every call, and a call that is the last thing a body does has no rule of its own. The kernel's elision — the caller's frame closed first, the callee's frame in its place — is an economy that must not change an answer; where it does is NUR407, and there the specification's answer is the nested one | `engine.go` `probeTailCall`, `tcoEligible`, `elideTailFrame`, `teardownFrameState`; `fn_frame_probe.go` | `Engine.lean` `execMatch` (no elision: the frame is always spliced inside the caller's) | `Theorems/Tail.lean`; core/go `TestCoreSpecTailCallsUnobservable` (every corpus row with elision off); `dispatch.tsv`; `divergent.tsv`; search |
| The function-spec reader: a written list of triples — parameters, returns, a body — read into signatures. A parameter list element by element: `name:Type`, `name?:Type`, `name:Type/q`, the keyword `name:kw/q`, a bare type name or type, an Integer, Boolean or String the slot admits exactly, a map as an unnamed Map pattern, `|` the barrier, `?` the optional mark; a parameter name is a word's name. A declaration resolved to a type and, where it says more, a pattern (a scalar its family, a union `Any`, a list or map its family; a string that could never name a type the string, one shaped like a type name an error; a name bound to a bare type that type, and bound to any other type body `Any` with no pattern — NUR411, as the kernel has it; `none` nothing — NUR408, as the kernel has it). Returns: several by position, a named return its type, a part that is not a list the list of itself. The unnamed path takes fewer declarations than the named (NUR409, as the kernel has it) | `fn_def.go` `ParseFnDef`; `fn_params.go` `ParseFnParams`, `ParseFnReturns`, `ResolveSigType`, `unwrapNamedReturn`, `keywordParam`, `looksLikeTypeName`; `word_name.go` `ValidateWordName` | `FnSpec.lean` `parseTriples`, `parseParams`, `parseReturns`, `resolveSigType`, `boundType`, `validName` | `Theorems/FnSpec.lean`; `divergent.tsv` (Go column); search |
| The return check's patterns: a returned value must be of the declared type and unify with the declared pattern | `engine.go` `validateReturnTypesIn`; `value.go` `ReturnCheckInfo.ReturnPatterns` | `Value.lean` `Impl.boru` (`returnPatterns`), `returnCheck`; `Engine.lean` `closeFrame` | `Theorems/FnSpec.lean` (`a_return_pattern_is_checked`); `divergent.tsv` (Go column); search |
| The definition install: a plain value is bound; a function is compiled under the name — an older definition of the name whose parameter types it repeats is taken off (a redefinition replaces, other types stack), every optional parameter is expanded into signatures whose bodies call the function by name with the type's base value (none where the type has no base value or the parameter's pattern refuses it — NUR410, as the kernel has it), and each body is asked then whether its frames record | `core_helpers.go` `InstallDef`, `installFnDef`, `compileFnSigs`, `ExpandOptionalSigs`, `omittedDefaultValue`, `BaseValue`, `FnDefsOverlap` | `FnSpec.lean` `install`, `expandOptional`, `baseValue`, `overlap` | `Theorems/FnSpec.lean`; `dispatch.tsv`; `divergent.tsv` (Go column); search |
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
- an applied mark, a function from another registry;
- the function-spec reader's arms the notation cannot write: the
  parser's own `{name: Type}` pair (and so a NAMED parameter with a
  pattern), a parenthesised annotation, which is RUN to give its type, a
  bare `Type/q` atom, a dotted annotation, a type a program defined,
  generics; an optional parameter that is unnamed (its added signature
  reads the args list through words the fixture does not have);
- of the install: a function that came from another registry and is
  rebound as a wrapper, an un-definition (`FnUndef`), a class type, the
  `/s` definition; and the compile cache — a second install of one
  function under one name reuses the first's answers, where the model
  asks again;
- the frame-state analysis beyond a name and its alias: a wrapper whose
  one body word is a binding word (the trivial delegation `import` and
  `unpack` make), a word-extension clone, a splice macro whose payload
  names one, a sugar marker that lowers to one;
- the RESOURCE side of tail calls: that a chain of them costs no tape (the
  model has fuel, not a tape ceiling);
- the dispatch order's two inputs the fixture cannot reach: a pattern
  against a bare type across two overloads of one word, and `CoreDefault`
  (every fixture word has one signature, and `fn(` declares types only);
  and `Extends`;
- a loop's own step budget, its context layer, and the
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
