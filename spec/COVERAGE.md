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
| The `/v` modifier: the bound value, placed or collected, never called | `engine.go` `stepWordVal`, `core_ref.go` `ResolveRef` | `Engine.lean` | `dispatch.tsv`; `divergent.tsv` (Go column); search |
| The `/u` modifier: the bound function with its argument order reversed — the wrapper (parameters reversed, all forward, a handler that re-dispatches the original laid out for ITS barrier), `/uv` the wrapper as a value, `illegal_ref` on a name that is not a function | `engine.go` `stepWordUsurp`, `core_ref.go` `usurpOver`, `usurpDispatchHandler` | `Engine.lean` `usurpOver`, `stepWord` | `Theorems/ArgumentRule.lean` (`usurp_*`); `divergent.tsv` (Go column); search |
| Closures: a function built at run time captures what the enclosing call frame bound — the frame baseline (none in a leaf frame), the capture rule (bound, deeper than the baseline, not a parameter or a body-local), captures installed beneath the parameters and removed with them, capture through nested closures | `fn_capture.go` `ComputeCaptures`, `bodyNeedsFrameState`, `registry.go` `PushFnBaseline`, `engine.go` `execFnDefSig` | `Engine.lean` `computeCaptures`, `bodyNeedsFrameState`, `execMatch`; `Match.lean` `Defs.pushBaseline` | `Theorems/Closure.lean`; `divergent.tsv` (Go column); search |
| An ANONYMOUS function at the pointer: called when something it can take stands beside it, otherwise a value — never `uncalled_function` | `engine.go` `execFnDefLiteral` | `Engine.lean` `dispatchFnValue` | `divergent.tsv` (Go column); search |
| Driven loops: a looping word hands the engine a driver; each iteration is a sealed region of its own — the driver's inputs, inert, then the body — whose residual goes to the driver and is never stepped again; the driver's results stand in the loop's place | `loop.go` `LoopDriver`, `StartLoop`, `stepMoveDriven`, `spliceDrivenIteration` | `Engine.lean` `Driver`, `driveLoop`, `runRegion` | `Theorems/Loop.lean`; `divergent.tsv` (Go column); search |
| Function identity: a function is equal only to itself — in unification and in a disjunct's reduction — a reference carries the identity of the one function bound to its name, and a wrapper is a new function | `compare.go` `sameFnIdentity`, `registry.go` `aggregateIdent`, `value.go` `NewFunction` | `Value.lean` (`fn … ident`), `Compare.lean` `sameValue`, `Match.lean` `Defs.refIdent` | `Theorems/UnifyOrder.lean` (`function_identity`); `unify.tsv`; `divergent.tsv` (Go column); search |
| The stack-form probe | `signature.go` `MatchSignature` | `Match.lean` | search |
| The step loop: literals and arrival, word resolution, word literals, undefined words | `engine.go` | `Engine.lean` | corpus; search |
| Parking and collecting a forward; completion and the stack-only re-dispatch | `engine.go` | `Engine.lean` | corpus; search; theorems |
| The phase-1 plan walk: group pre-evaluation, overload pruning | `collect_kernel.go` `CollectForward` | `Engine.lean` | `dispatch.tsv`; search |
| The strict forward barrier and the barrier commit | `engine.go` | `Engine.lean` | `dispatch.tsv`; search; theorems |
| Paren groups: collapse, re-step, void groups and `no_value_error` | `engine.go` | `Engine.lean` | corpus; search |
| `end`; curry-or-stack, including the curry list; orphaned forwards | `engine.go` | `Engine.lean` | corpus; search |
| Running a match: argument evaluation, result splice, full-stack handlers | `engine.go` `execMatch` | `Engine.lean` | corpus; search |
| A NAMED function value at the pointer: it dispatches as its word would, is sealed to the stack once its forward arguments are in, is `uncalled_function` when it matches nothing it could have been called on, and is PLACED — not called — when a paren group collapses to it alone | `engine.go` `execFnDefLiteral`, `fnReturnPark` | `Engine.lean` | search; `divergent.tsv` (Go column) |
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
- an applied mark, a function from another registry, arity widening of a
  value call, unnamed parameters and the frame argument span, the args
  stack, the truncation of a frame's body-local definitions, tail-call
  frame elision, return patterns;
- overload order by slot specificity (`CompareSignatures` beyond arity),
  and `Extends`;
- the counted and conditional loops (`for`, `while`: a mark, a body, a
  move, with no sealing paren), call regions, and flow control — break
  and continue, which a driven loop lets through; a loop's own step
  budget, its context layer, and the attribution of a body's fault;
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
