# Carrier-Based Static Type Checking for boru

## Executive summary

This report evaluates the feasibility of static type checking for boru using a **carrier-value abstract interpretation** approach:

- Use non-concrete values ("carriers") that carry only type information.
- Execute programs symbolically over carriers instead of concrete values.
- Choose matching signatures, propagate return carrier types, and merge alternatives via disjunctions.
- Explore branches and function signatures compositionally, with type refinement and error recovery.

### Bottom line

The approach is **feasible and well-aligned with boru's current runtime architecture**, but it must be implemented as a bounded abstract interpreter (with joining/widening/memoization) to avoid path and union explosion.

---

## Theoretical foundation: abstract interpretation

This approach is **abstract interpretation** — a well-understood formal framework. The mapping between the standard theory and boru's carrier proposal:

| Abstract Interpretation Concept | boru Carrier Equivalent |
|---|---|
| Abstract domain | Type lattice (boru hierarchy + disjunctions) |
| Abstract transfer functions | Signature -> return type mappings |
| Join operation | Disjunction (`A` or `B`) |
| Widening operator | Collapse to common ancestor type |
| Fixed-point iteration | Loop/recursion analysis |

This is good news — the theory is mature, and soundness/termination properties are well-characterized. The checker is not a novel invention but an instance of a proven technique applied to boru's specific semantics.

---

## Scope and question

We evaluate whether the following strategy can provide static typing for boru:

1. Introduce a dynamic type carrier value.
2. Invoke words/functions by signature only (no concrete execution), pushing return carriers.
3. Use disjunction types for ambiguous return types.
4. Evaluate all branches (`if`, etc.) and join results via disjunction.
5. Analyze each defined function/signature separately.
6. Refine carrier types from checks/guards when possible.
7. Prefer most specific types.
8. On mismatch, log error, continue, and coerce to expected type for forward progress.

---

## Current boru properties relevant to this design

### 1) Signature-driven dispatch already exists

boru currently resolves calls by selecting a matching signature from pre-sorted candidates ("first match wins"). `MatchSignature` applies type matching and structural pattern checks (`Unify` for patterns) at call time.

Signature matching is primarily type-driven. The core function `sigTypeMatches(v Value, t Type)` in `signature.go:161-181` uses `Parent.Matches(t)` as its first check. However, it also examines `Data` in specific cases: metatype detection (`Data == nil` for type literals, `Data.(ObjectTypeInfo)` for custom types), and `IsRecordType()`/`IsTableType()`/`IsOptionsType()` discrimination. A carrier value would need to represent these compound type distinctions, not just primitive Parent.

The unified `matchSignature` function in `match.go:27` (note: there is no separate `MatchSignatureReversed` — a single function with a `nearestFirst` flag handles both directions), `SortSignatures`, `signatureScore`, `Type.Matches()` hierarchy check, and `Unify` are all largely reusable. The metatype and compound-type cases require the carrier to encode whether a value is a type literal, record schema, or options map.

Implication: a static analyzer can reuse most of the matching logic against abstract carrier stacks, but carriers must encode more than bare Parent for compound types.

### 2) Unification and disjunction already exist as language concepts

boru has:

- `Unify(a,b)` rules for scalar/list/map compatibility and narrowing.
- Disjunct handling (`unifyDisjunct`) for alternatives.
- `tor` behavior that forms disjunctions from any two operands.

Implication: the language already has the primitives needed to represent and merge uncertain static types.

### 3) Branch runtime semantics are explicit and lazy

`if` currently evaluates condition and executes **only the selected branch** at runtime. A static analysis can intentionally evaluate both branches and join stack effects, which is a standard abstract interpretation dual to lazy runtime branching.

### 4) Function return type declarations are already modeled

Function signatures include declared return types (`FnSig.Returns`), and runtime inserts/checks return markers.

Implication: static checking can enforce/verify return summaries and derive diagnostics before execution.

### 5) Dynamic evaluation constructs exist (`def` splicing, `do`)

- `def` list bodies are spliced into token stream on use.
- `do` evaluates list/map-embedded code through sub-engines.

Implication: full static certainty is impossible in general without conservative assumptions or staged analysis.

### 6) Forward collection is stateful and incremental

Forward collection uses type information to decide which tokens to collect, but it is not simple lookahead. The engine inserts `Forward` markers onto the stack (`engine.go:933-950`) and collects arguments incrementally as the pointer advances through `stepLiteral` (`engine.go:953-1056`). Each collected token is moved before the function word, and when all expected args arrive, `rearrangeForForward` reorders them into signature order. Orphaned forwards are resolved after the main loop (`resolveOrphanedForwards`, `engine.go:391-438`).

Implication: the abstract interpreter must simulate this forward-collection state machine, not just "look ahead." Pending forwards create additional state dimensions in the analysis.

### 7) Argument equivalence simplifies analysis

For a word `f` with args `a` (sig[0]) and `b` (sig[1]), the equivalence is `f a b` <=> `b f a` <=> `b a f` — NOT `f a b` <=> `a f b`. Forward args claim the lowest sig indices first; stack args (nearest-to-word first) fill the remainder. Moving an arg from forward to stack reverses it to the far (deepest) stack position. For 3 args: `f a b c` <=> `c f a b` <=> `c b f a` <=> `c b a f`. This means the abstract interpreter can normalize to canonical (all-forward) form, and the result type is position-independent within an equivalence class.

### 8) No return type annotations exist on signatures

`NativeSig` (`nativefunc.go:12-37`) declares input types (`Args []Type`) but has no return type field. Return types are implicit in handler code. This is a critical infrastructure gap: the checker cannot determine what a word returns without either (a) adding explicit return type annotations to every `NativeSig`, or (b) maintaining a separate return-type table. User-defined functions (`FnSig`) do declare `Returns` types, but native words — the majority of the vocabulary — do not.

---

## Mapping proposed algorithm (a–h) to boru

### a) Carrier value

**Feasible.** Introduce an abstract `Carrier`:

- `Type`: current abstract type (may be disjunct).
- `Origin`: source location/provenance.
- `Constraints`: optional path constraints/guards.
- `Diagnostics`: accumulated soft errors.

No concrete payload required.

### b) Signature-only invocation and return propagation

**Feasible but with caveats.**

At each call site:

1. Read abstract stack prefix/suffix requirements.
2. Determine candidate signatures that could match carrier types.
3. For each candidate, compute output stack effect as carriers.
4. Join candidate outcomes.

Caveat: uniqueness is not guaranteed when input carriers are broad/disjunctive.

### c) Disjunction for multi-type returns

**Feasible and natural.**

When multiple signatures/branches produce distinct types for same stack position, join with disjunct union. If union grows too large, widen to parent type.

### d) Evaluate all branches and join

**Feasible; expected for static analysis.**

For branch nodes (`if`, loops with conditionals, etc.):

- Evaluate all reachable branches under current abstract state.
- Join resulting abstract stacks.
- Preserve/merge diagnostics.

Flow typing falls out naturally: branch analysis with type narrowing is just abstract interpretation with path sensitivity. `if [x is Integer] [...]` narrows the carrier in the then-branch.

### e) Analyze each function signature separately

**Strongly recommended.**

Perform interprocedural summary analysis:

- Build summary per signature: input abstract pattern -> output stack effect + diagnostics.
- Memoize summaries and compute fixpoint for recursive cycles.

### f) Prune types via guards

**Feasible and high-value.**

Where boru expressions encode type checks, narrow carrier type on true-path and complement on false-path (if representable).

### g) Prefer most specific types

**Required for precision.**

Use existing subtype hierarchy and specificity ranking. Preserve narrowest carrier type that remains sound.

### h) Error-tolerant continuation

**Feasible and practical for tooling.**

If mismatch occurs:

- Emit diagnostic with expected vs actual abstract type.
- Continue with coerced expected carrier (or joined fallback) to avoid cascade abort.

This supports IDE/lint workflows.

---

## Scalability analysis

### Sources of blow-up

1. **Branch/path explosion**: each conditional multiplies states.
2. **Overload ambiguity**: broad carriers can match many signatures.
3. **Union growth**: repeated joins increase disjunct size.
4. **Optional signature combinations**: boru already expands optional params combinatorially (`2^N` subsets).

### Why disjunction growth is naturally limited

Looking at actual boru signatures:

- `add` has 6 signatures via `registerBinaryMathWord` + extra sigs (`native_math_add.go`): `[TNumber,TNumber]`, `[TScalar,TScalar]`, `[TCalDuration,TDate]`, `[TClkDuration,TDateTime]`, `[TClkDuration,TInstant]`, `[TClkDuration,TDate]`. These produce 6 distinct return types (Integer, Decimal, String, Date, DateTime, Instant). However, for non-temporal inputs the return types collapse to `{Integer, Decimal, String}` — still bounded.
- Comparison words (`lt`, `gt`, `eq`, etc.) have 7 signatures each but all return `Boolean` — one return type.
- Array operations (`native_array_core.go`, 22 signatures) return primarily `TList` or `TInteger` — 2 return types.
- Most words have 1-2 distinct return types across all their signatures.

Three mitigation strategies prevent explosion:

1. **Width cap**: Limit disjunctions to N alternatives (e.g., 8). Beyond that, widen to the nearest common ancestor. `Integer | Decimal | String | Boolean` -> `Scalar`.

2. **Subsumption**: `Integer | Number` -> `Number` (parent absorbs child). `Integer | Integer` -> `Integer`.

3. **Lazy expansion**: Only split disjunctions when the word actually dispatches differently. If all alternatives map to the same return type, no expansion occurs. Example: `add(Carry<Number>, Carry<Number>)` matches one signature `[TNumber,TNumber]` and returns `Carry<Integer|Decimal>` (see intra-signature value-dependence below). The width stays bounded.

### Loop and recursion termination

The type lattice has finite height (bounded hierarchy depth + width-capped disjunctions). Fixed-point iteration:

- **`each`**: Analyze the body once with the element type. Result is a list of whatever the body produces. One pass.
- **`fold`**: Start with accumulator type `T`, run body, get `T'`. If `T' = T`, done. If not, iterate with `T|T'`. Bounded by lattice height.
- **Recursion**: Same fixed-point approach. For factorial, base case gives `Carry<Integer>`, recursive case applies `mul [I,I]->I`, fixed point reached in one step.

### Complexity bound

Worst case is **polynomial, not exponential**, because widening prevents unbounded disjunction growth. The lattice height is `O(depth_of_type_hierarchy * width_cap)`, which is small and constant for boru's hierarchy.

### Can it scale?

Yes, with standard abstract-interpretation controls:

- **Widening**: cap union cardinality and collapse to ancestor types.
- **Join aggressively**: merge equivalent abstract states at control-flow joins.
- **Memoization/fixpoints**: cache `(pc, abstract stack shape, env)` states.
- **Budgeting**: cap explored signatures/paths/depth per function.
- **Summary-based interprocedural analysis**: avoid re-analyzing same bodies.
- **Timeout/fallback**: degrade to coarse types (`Any`, `Scalar`, etc.) under pressure.

Without these controls, worst-case behavior can be exponential. With them, it is bounded and practical.

---

## Comparison with other static typing strategies

| Approach | Fit for boru | Why |
|---|---|---|
| Hindley-Milner | Poor | Assumes parametric polymorphism, no subtyping, single principal type. boru has ad-hoc overloading, subtype hierarchy, first-match dispatch. |
| Abstract interpretation (this proposal) | Excellent | Follows boru's own execution model. Same signature matching, same dispatch order. |
| Flow typing | Falls out naturally | Branch analysis with type narrowing is just abstract interpretation with path sensitivity. `if [x is Integer] [...]` narrows the carrier in the then-branch. |
| Gradual typing | Partial overlap | boru's approach is stronger — it tracks precise types where known and degrades to `Any` only at escape hatches, rather than using `?` throughout. |
| Factor's stack checker | boru goes further | Factor checks stack arity (`( a b -- c )`) but not value types. boru's carriers track concrete types AND handle dispatch. Factor lacks ad-hoc overloading. |
| Annotation-heavy checker only | Moderate | Simpler implementation and predictable performance, but weaker precision on existing dynamic features and higher user annotation burden. |

**Best fit for boru today:** carrier-based analysis with bounded abstract interpretation.

---

## Key gotchas and gaps

### Severe (fundamentally hard)

1. **`do` on dynamically-constructed lists.** `do` evaluates a list as code. If the list is a literal (`do [dup add]`), the checker can analyze it. If it's computed at runtime (`ops get 0 do`), the checker sees `Carry<Any>` and cannot know what code runs. Must conservatively return `Carry<Any>` or flag as uncheckable.

2. **`context get` / `context set`.** The context store uses copy-on-write prototype chains (`StoreInstanceInfo` in `registry.go`). `context get x` traverses the prototype chain at runtime. Without whole-program tracking of all `set` calls on all paths, the result is `Carry<Any>`. This is a fundamental escape hatch.

3. **Runtime `def` rebinding.** `def x 1` then later `def x "hello"` changes the type of `x`. DefStacks (`registry.go`) are per-name stacks supporting shadowing. Conditional defs create disjunctions. The abstract interpreter must track `def` bindings as part of its state:
   ```
   if [cond] [def x 1] [def x "hello"]
   # x : Carry<Integer|String>
   ```
   Complication: sub-engines created by `each`, `fold`, `do` share the same Registry and DefStacks. A `def` inside an `each` body mutates shared state visible to subsequent iterations and the parent scope (unless scoped by `fn` cleanup markers). The checker must model this shared mutable state.

4. **Intra-signature value-dependent return types.** `registerBinaryMathWord` (`native_helpers.go:50-79`) creates a single `[TNumber, TNumber]` signature whose handler internally branches: if both args are `TInteger`, it calls `intFn` (returning Integer); otherwise `fn` (returning Decimal). The carrier checker sees one signature match but two possible return types. This pattern affects all arithmetic words (`add`, `sub`, `mul`, `div`, `mod`, `pow`). Resolution requires either splitting signatures explicitly or adding return-type annotations with input-type conditions.

5. **No return type annotations on NativeSig.** The `NativeSig` struct has no return type field. The checker cannot derive output types from handler code. This is the single largest infrastructure prerequisite — every native word (126 `NativeSig{}` definitions across 76 files) needs return type metadata before the checker can function.

6. **Mark/Move control flow mechanism.** `if` and `for` do not simply return branch results — they splice tokens onto the stack using mark/move pairs (`conditional.go:67-77`, `forloop.go:106-148`). The `if` handler returns a sequence `[Mark, condTokens..., Move(IfCont)]` that the main engine loop processes incrementally. The abstract interpreter cannot simply "evaluate both branches and join" — it must understand the mark/move token-splicing protocol, or implement an equivalent branch-aware abstraction that bypasses it.

### Moderate (tractable with effort)

7. **Higher-order words with non-literal bodies.** `each`, `fold`, `outer` take code bodies as lists and create sub-engines per iteration (`New(reg)` in `native_array_higher.go`). For literal bodies (`each [dup add]`), analysis is straightforward — analyze the body once with the element type. For bodies passed from elsewhere (a function parameter that happens to be a list), the checker cannot know the stack effect. `fold`'s return type is particularly challenging: it returns the accumulator, whose type depends on the initial value AND what the body produces (`native_array_higher.go:112-114`).

8. **Forward collection state machine.** Forward collection is not simple lookahead. The engine places `Forward` markers on the stack and collects args incrementally as execution proceeds (`engine.go:953-1056`). When a word has both `[Integer]` and `[Integer, String]` signatures, the engine tries longer signatures first, looks forward for a `String` match, falls back if not found. The abstract interpreter must simulate this stateful process, including the `rearrangeForForward` reordering step.

9. **Module imports.** Cross-file analysis requires parsing and analyzing imported files, building module-level type environments, resolving `utils.helper` to exported types. Module sub-engines use isolated registries (`native_module_module.go:31-49`) with export maps. Standard for static analyzers but adds implementation scope.

10. **Unbounded stack.** Loops can push unbounded values. Mitigation: compute the stack effect (net push/pop per iteration) rather than tracking every iteration's values.

11. **Query system type aliasing.** Query operations like `from` return `newValue(TList, qb)` where `Data` is a `QueryBuilder`, not a real `[]Value` list (`query.go:382`). The Parent says `TList` but standard list operations would fail on it. The checker must handle these "disguised" types that share Parent with concrete types.

### Low (manageable)

12. **List auto-evaluation.** The checker must track `Eval`/`Quoted` flags to know which lists will be auto-evaluated. Mirrors existing engine logic directly.

13. **Barrier positions in signatures.** The `|` syntax in forward collection creates barriers. The abstract interpreter must respect these, but the logic is purely structural.

14. **`isTruthy` is value-dependent.** For guard-based narrowing (`if [x is Integer] ...`), the checker can narrow types. But `isTruthy` itself depends on values: 0 is false for integers, non-zero is true; empty strings are false, non-empty true (`conditional.go:10-41`). The checker must approximate truthiness from types.

15. **Error modeling**: should distinguish hard unsoundness from soft recoverable diagnostics.

16. **Determinism under overload ambiguity**: static analyzer must define deterministic join/prioritization policy compatible with runtime dispatch order.

---

## The checker as a modified engine

What makes this approach particularly natural for boru is that **the checker IS a boru engine with a different value representation**. You're not building a separate formal system — you're running the same dispatch, same matching, same signature priority, just with carrier values instead of concrete values. The engine's existing `matchSignature` (in `match.go`), `sigTypeMatches`, `SortSignatures`, `Type.Matches()`, and `Unify` are largely reusable.

The abstract interpreter could be structured as a modified `engine.Run()` where:

- **Literals** push `Carry<their_type>` instead of concrete values.
- **Word execution** calls the signature matcher, reads the return type from annotations (see prerequisite below), pushes `Carry<return_type>` instead of calling the handler.
- **`if`** must handle the mark/move mechanism abstractly: instead of splicing tokens, fork the abstract state, evaluate both branches, and join the resulting stacks.
- **`for`** similarly handled: analyze the body with the iterator type, compute fixed-point on accumulator type.
- **`def`** updates an abstract def-binding map, tracking type disjunctions for conditional defs.
- **Forward collection** must be simulated: track pending abstract forwards and collect carrier-typed args.

### Prerequisite: return type annotations

For this to work, every `NativeSig` must be augmented with return type information. Currently, 126 native signatures across 76 files declare only input types. The implementation plan should include a Phase 0 that adds a `Returns []Type` field to `NativeSig` and annotates all existing native words. This is mechanical but non-trivial in scope.

### What can and cannot be shared with the runtime engine

| Component | Reusable? | Notes |
|---|---|---|
| `matchSignature` / `sigTypeMatches` | Yes | Core type matching works on Parent |
| `SortSignatures` / `signatureScore` | Yes | Scoring is type-only |
| `Type.Matches()` / `Unify` | Yes | Type hierarchy and unification |
| `positionalMatch` | Yes | Type checking of arg positions |
| Mark/Move mechanism | No | Must be replaced with abstract branch/loop handling |
| `stepLiteral` forward collection | Partially | Type-matching part reusable; stack mutation is not |
| `autoEvalList` / `autoEvalMap` | No | These execute concrete code |
| `execMatch` handler calls | No | Replaced by return-type lookup |
| Sub-engine creation | No | Replaced by abstract sub-analysis |

This means the checker **automatically stays in sync with dispatch logic** as words are added — the same `NativeSig` definitions that drive runtime dispatch drive static checking. However, return type annotations must be maintained alongside handler implementations.

---

## Performance considerations

### Analysis cost per construct

| Construct | Cost model | Concern level |
|---|---|---|
| Straight-line code | O(n) in token count | Low |
| Signature matching | O(S) per call site, S = signature count | Low — sorted, first-match-wins |
| `if` branches | 2x per branch (both paths) | Moderate — nesting multiplies |
| `for` loops | Fixed-point iterations bounded by lattice height | Low with widening |
| `each`/`fold` body | One abstract analysis per body + fixed-point | Moderate |
| `def` environment | Key in memoization state | High — see below |
| Forward collection | Must simulate state machine per call | Moderate |
| Module imports | One analysis per imported file | Low if cached |

### Key performance risks

**Memoization key space.** The standard abstract interpretation cache key is `(program counter, abstract stack shape, abstract environment)`. In boru, the environment includes DefStacks — a shared mutable `map[string][]Value` in `registry.go`. DefStacks can be modified by sub-engines (each/fold/do share the same Registry). This makes the environment component of the cache key large and mutable, reducing cache hit rates. Mitigation: scope the environment to only names referenced in the current function.

**Sub-engine fan-out.** `each` creates a sub-engine per iteration (`New(reg)` in `native_array_higher.go:29`). For abstract interpretation, this is one abstract sub-analysis per higher-order word call. If bodies are complex or nested (each within each), the cost multiplies. Mitigation: summarize body effects once per distinct input type, then apply the summary.

**Forward collection state dimensions.** Pending Forward markers add state to the analysis. With N pending forwards, the state space grows. In practice, boru rarely has more than 1-2 pending forwards at a time, so this is bounded.

**Step limits as natural bounds.** The runtime engine limits steps to 22222 (top-level) or 2222 (sub-engine) in `engine.go:268-270`. The abstract interpreter can use similar bounds as a safety net, degrading to `Carry<Any>` if analysis exceeds a step budget.

**Signature scoring is pre-computed.** `SortSignatures` runs at registration time (`signature.go:313-331`), not per call. The checker benefits from the same pre-sorted order — no per-call sorting cost.

### Comparison with known abstract interpreter performance

**TAJS** (Type Analysis for JavaScript) analyzes programs of ~1000 LOC in seconds with similar dynamic-language challenges. boru programs are typically shorter. **Pyright** handles Python files of thousands of lines interactively. Given boru's shallow type lattice (max depth 4: `Scalar/Time/Duration/CalDuration`) and bounded vocabulary, per-file analysis should complete in milliseconds for typical programs.

---

## Prior art and existing implementations

### Concatenative language type systems

**Factor's stack checker.** Factor uses stack effect declarations `( a b -- c )` to verify arity at compile time but does not track value types. boru's carrier approach goes further by tracking types through dispatch. Factor's checker demonstrates that concatenative control flow (quotation passing, higher-order combinators) is tractable for static analysis. Source: `factor-lang.org`, stack checker documentation.

**Cat language (Christopher Diggins, 2006).** Concatenative language with a static type system using stack-polymorphic type variables. Cat demonstrates Hindley-Milner adaptation for concatenative semantics: type schemes like `forall A B. (A int int -- A int)` capture stack effects. Cat lacks ad-hoc overloading and subtyping, so its type system is simpler than what boru requires. Relevant as proof of concept for concatenative type inference.

**Kitten language (Jon Purdy).** Concatenative language with Hindley-Milner inference extended with row-polymorphic stack types. Handles higher-order functions (quotation types). Again, no ad-hoc overloading — Kitten uses parametric polymorphism exclusively. Demonstrates that HM can handle quotation/composition but would not handle boru's first-match dispatch.

### Abstract interpreters for dynamic languages

**TAJS — Type Analysis for JavaScript (Jensen, Møller, Thiemann, ECOOP 2009).** The closest academic analogue to boru's proposal. TAJS is an abstract interpreter for full JavaScript, handling: prototype-based dispatch (cf. boru's signature dispatch), dynamic property access (cf. `context get`), `eval` of computed strings (cf. `do` on computed lists), higher-order functions (cf. `each`/`fold` with code bodies). TAJS uses a lattice of abstract values with widening, flow-sensitive analysis, and call-graph construction. It demonstrates that abstract interpretation scales for dynamic languages with careful engineering. Key lesson: TAJS handles `eval` by flagging it as uncheckable — the same approach proposed here for dynamic `do`.

**Pyright (Microsoft).** Static type checker for Python, implemented as flow-sensitive abstract interpreter. Handles: ad-hoc overloading via `@overload` decorators (cf. boru's multi-signature words), union types (cf. disjunctions), type guards (cf. `is` checks in `if` branches), value-dependent narrowing for literal types. Pyright processes thousands of lines per second interactively. Relevant as a production-quality example of abstract interpretation on a dynamic language with overloading.

**Flow (Meta/Facebook, 2014).** JavaScript type checker using flow-sensitive typing with path sensitivity and union types. Handles similar patterns to boru: conditional type narrowing, union types that split and rejoin at control flow points. Flow's "refinement types" correspond to boru's guard-based narrowing proposal.

**RPython type inference (PyPy project).** Abstract interpretation over a restricted Python subset. Demonstrates that abstract interpretation handles dynamic features effectively when the language is sufficiently constrained. boru's bounded type lattice and finite vocabulary make it more constrained than full Python, suggesting good results.

### Industrial abstract interpreters

**Astrée (Cousot et al., ESOP 2005).** Industrial abstract interpreter for C used at Airbus. Demonstrates that abstract interpretation scales to hundreds of thousands of lines of code with careful widening strategies. Key lesson: the widening operator design is critical — too aggressive loses precision, too conservative risks non-termination. boru's shallow lattice (max depth 4) makes widening straightforward compared to C's complex numeric domains.

---

## Recommended implementation plan

### Phase 0 — Return type annotations

- Add `Returns []Type` field to `NativeSig` struct.
- Annotate all 126 existing native signatures across 76 files.
- For intra-signature value-dependent returns (arithmetic), use conditional return types: e.g., `[TNumber,TNumber]` returns `TInteger` when both inputs are `TInteger`, else `TDecimal`.
- Validate annotations against handler behavior with tests.

### Phase 1 — Core abstract engine

- Add `Carrier` abstraction and abstract stack.
- Reuse signature matching logic over abstract types.
- Produce diagnostics on mismatches.
- Handle the mark/move control flow abstractly (fork/join instead of token splicing).

### Phase 2 — Control flow and unions

- Add branch splitting/joining.
- Implement disjunct union normalization + widening caps.

### Phase 3 — Function summaries

- Analyze `def/fn` per signature.
- Memoize summaries and compute fixpoints for recursion.

### Phase 4 — Refinement and precision

- Add guard-based narrowing/pruning.
- Add path-sensitive state where beneficial.

### Phase 5 — Tooling mode

- Non-fatal error recovery (continue with expected type).
- Rich diagnostics with provenance and suggested fixes.

---

## Verdict

- **Is it possible?** Yes, with prerequisites. The largest prerequisite is adding return type annotations to all native signatures (Phase 0).
- **Can it scale?** Yes, if implemented with abstract interpretation controls (join/widen/cache/budget). boru's type lattice is bounded (max depth 4, ~60 builtin types) and the vocabulary is finite.
- **Is exponential explosion possible?** Yes in worst-case; manageable in practice with bounded analysis. Worst case is polynomial with widening, not exponential.
- **Compared to alternatives?** This approach is the most compatible with boru's current runtime semantics and type machinery. Similar approaches have been proven at scale by TAJS (JavaScript), Pyright (Python), and Astrée (C).
- **What are the hard boundaries?** The main escape hatches (`do` on computed lists, `context get`, dynamic `def` with shared sub-engine mutation) are exactly the constructs you'd expect to be hard to type-check in any dynamic language — they don't invalidate the approach, they just define its boundary.
- **What is the realistic scope?** For straight-line code, function calls, branching, and loops over typed data, the carrier approach gives precise results with no explosion. Intra-signature value-dependent returns (arithmetic) and the mark/move control flow mechanism add implementation complexity beyond what a naive "modified engine" description suggests, but both are tractable with known techniques.
