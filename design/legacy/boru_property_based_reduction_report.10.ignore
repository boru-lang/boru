# boru as a Substrate for Property-Based Test Input Reduction

## Executive Summary

This report summarizes a discussion about whether property-based testing (PBT) can be modeled as generation plus reduction over a structured, measurable program representation, and whether boru is a suitable embedded language for that purpose.

The central recommendation is:

> Use boru as a **counterexample description calculus**, not merely as a JSON generator. Define a restricted pure profile of boru, lower surface syntax into a canonical strict-stack intermediate representation (IR), assign an MDL-style cost model to that IR, and implement failure-preserving rewrites over the IR.

This gives a practical route toward what might be called **Kolmogorov-style shrinking**, without relying on uncomputable Kolmogorov complexity. The generated input is the denotation of a program; the shrinker reduces the program, not the value directly.

The proposed framing is:

```text
Find a lower-cost boru generator program p'
such that:

  eval(p') produces a valid test input
  property(eval(p')) still fails
  cost(p') < cost(p)
```

This is best described as:

> **Counterexample shrinking as failure-preserving program compression.**

---

## 1. Background: Property-Based Testing

Property-based testing systems usually have two major components:

1. **Generation**  
   Produce test inputs from some input space.

2. **Shrinking / reduction**  
   Once a failing input is found, search for a simpler failing input.

Classic approaches include:

- QuickCheck-style random generators and hand-written shrinkers.
- SmallCheck-style bounded exhaustive enumeration.
- Delta debugging and hierarchical delta debugging.
- Integrated shrinking via rose trees.
- Internal shrinking over choice traces, as in Hypothesis-style systems.
- Grammar-aware reduction for structured inputs.
- Solver-assisted and search-guided generation.

Most real implementations are partly heuristic because the property under test is usually a black-box predicate. The shrinker can only ask:

```text
Does this candidate still fail?
```

It usually cannot inspect the semantics of the bug.

---

## 2. Mathematical Model of Generation and Shrinking

A simple model of property-based testing is:

```text
X = set of possible test inputs
P : X -> {pass, fail}
μ = probability distribution over X
≺ = "simpler than" relation on X
```

Generation samples:

```text
x ~ μ
```

Shrinking searches for:

```text
x' such that:
  P(x') = fail
  x' ≺ x
```

This can be modeled as black-box constrained minimization:

```text
minimize c(x)
subject to P(x) = fail
```

where `c` is a cost or complexity function.

However, direct value-level shrinking has limitations. It can lose the structure that explains how a value was generated.

A better model for advanced systems is:

```text
C = space of generator traces or programs
decode : C -> X
P : X -> {pass, fail}
```

Then shrinking searches for:

```text
minimize cost(c)
subject to P(decode(c)) = fail
```

This model naturally supports internal shrinking, replayability, and validity-by-construction.

---

## 3. Kolmogorov Complexity and Practical Substitutes

The ideal notion of complexity is Kolmogorov complexity:

```text
K(x) = length of the shortest program that outputs x
```

But exact Kolmogorov complexity is uncomputable.

For a real testing system, the practical version is:

```text
K_G(x) = min {|p| : eval_G(p) = x}
```

where:

- `G` is a chosen generator language.
- `p` is a generator program.
- `|p|` is the cost of that program under a concrete cost model.

This is not true Kolmogorov complexity. It is a computable, language-relative description complexity.

This connects closely to:

- Minimum Description Length (MDL)
- Program-size complexity
- Term rewriting
- Well-founded orders
- Program synthesis
- Levin-style search
- Grammar-based compression

For PBT, this suggests:

> The simplest counterexample is not necessarily the smallest value. It is the shortest useful description of a failing value.

---

## 4. Why This Has Not Become Mainstream

Pieces of this idea already exist:

- Hypothesis uses internal choice representations and reduces those.
- Falsify adapts internal shrinking ideas for Haskell.
- Grammar-aware reducers shrink structured inputs.
- Delta debugging reduces test cases by removing components.
- Compiler testing often uses specialized reducers over ASTs or programs.

However, a full MDL/program-compression model is not common in everyday PBT because:

1. **Exact Kolmogorov complexity is uncomputable.**
2. **A concrete cost model is representation-dependent.**
3. **Human simplicity and description length are not identical.**
4. **A generator calculus becomes a second programming language.**
5. **Arbitrary host-language generators are opaque.**
6. **Global minimization is expensive.**
7. **Compression can destroy the failure.**
8. **Many everyday test cases shrink well enough with simpler methods.**
9. **The right representation must balance usability, inspectability, typing, determinism, and speed.**

The opportunity is strongest for highly structured domains:

- JSON-like inputs
- API request bodies
- protocol traces
- event logs
- state-machine histories
- generated ASTs
- generated programs
- database fixtures
- distributed-system schedules

---

## 5. Why boru Looks Promising

boru appears well-suited as a prototype substrate for this research direction, especially because it is designed as an embedded language whose features and words can be enabled or disabled.

Relevant properties include:

### 5.1 JSON-Like Data Representation

boru can represent structured data such as scalars, lists, maps, and records. This makes it a natural candidate for representing JSON-like test inputs.

Instead of reducing only expanded JSON like:

```json
{
  "users": [
    {"id": 0, "role": "admin"},
    {"id": 1, "role": "admin"},
    {"id": 2, "role": "admin"}
  ]
}
```

the reducer can work on a compact generator description such as:

```boru
{
  users: iota 3 each [
    {id: args.0, role:"admin"}
  ]
}
```

The second representation preserves the structure of the input.

### 5.2 Strict Stack Mode

boru can be rewritten into a strict stack form. This is essential.

Human-friendly surface syntax is useful for authoring, but reduction should happen over a canonical strict-stack IR. This avoids ambiguity caused by flexible argument ordering or forward collection.

Surface syntax:

```boru
add 1 2
1 add 2
1 2 add
```

should normalize to a canonical IR such as:

```text
PUSH 1
PUSH 2
CALL add
```

The cost model and rewrite rules should operate on this canonical IR, not on raw source text.

### 5.3 Embedded Profile

Because boru is embedded and can disable most features and words, it can expose a restricted profile specifically for test generation and reduction.

A pure profile can disable:

- IO
- network
- time
- randomness
- imports
- mutation
- unbounded recursion
- concurrency
- host-specific effects

This gives the reducer deterministic replay.

### 5.4 User-Defined Words

User-defined words can encode generation semantics. This is powerful because generators can remain readable and domain-specific.

For example:

```boru
def gen-user [
  {
    id: gen-id,
    role: gen-role,
    active: gen-bool
  }
]
```

However, the reducer must know whether user-defined words are transparent, opaque, frozen, or primitive generator words. This is discussed in the gotchas below.

### 5.5 Homoiconicity and Quotations

If boru can represent programs as values through quotations, then generator programs can be manipulated as structured data. This is very useful for reduction, rewriting, and compression.

### 5.6 Type and Schema Support

boru's type, record, and unification mechanisms can help enforce validity-preserving reductions.

A candidate can be accepted only if:

```text
eval(candidate) is valid
property(eval(candidate)) fails
```

This keeps the reducer from minimizing to meaningless generation errors.

---

## 6. Main Recommendation

Use boru in four layers:

```text
Layer 1: Surface boru authoring
Layer 2: Pure embedded BORU-G profile
Layer 3: Canonical typed strict-stack IR
Layer 4: Failure-preserving reducer over IR
```

The reducer should not mutate expanded JSON values directly. It should mutate the generator program.

The key invariant is:

```text
Every accepted rewrite must:
  1. preserve validity
  2. preserve failure
  3. strictly lower program cost
```

This gives a deterministic, terminating reducer.

---

## 7. Proposed BORU-G Profile

Define a restricted profile, here called **BORU-G**, for generation and reduction.

### Allowed

```text
pure literals
lists
maps
records
pure arithmetic
pure string operations
pure list operations
quotation
do/eval of pure quotation
type checks
schema checks
unify
transparent user-defined words
declared primitive generator words
```

### Disabled or Restricted

```text
read
write
fetch
sqlite
now
sleep
timeout
interval
await
random host calls
imports during reduction
undef
mutation
unbounded recursion
unbounded loops
capability-dependent effects
```

### Required

```text
strict stack mode
deterministic evaluation
bounded evaluation fuel
declared stack effects
stable cost model
stable pretty-printer
word transparency annotations
canonical IR normalization
```

---

## 8. User-Defined Word Policies

User-defined words are a central design point. The reducer should not treat all words the same.

Use explicit policies:

```text
Transparent
  The reducer may inline and rewrite the body.

Opaque
  The reducer treats the word as atomic and may only shrink its arguments.

PrimitiveGenerator
  The word has custom generation, cost, and shrink semantics.

Frozen
  The reducer may not touch it.
```

Example:

```text
gen-user: Transparent
uuid: Opaque
gen-int: PrimitiveGenerator
security-token: Frozen
```

Without this distinction, the reducer will either be too weak or too destructive.

---

## 9. Cost Model

The cost model should be defined on canonical IR, not on source text.

A useful starting point is a weighted AST/IR cost:

```text
null, true, false, 0, ""     cost 1
small integer n              1 + bit_length(abs(n) + 1)
string s                     1 + encoded_length(s)
list constructor             1 + sum(child costs)
map constructor              1 + key costs + value costs
word call                    declared word cost
quotation                    1 + body cost
user-defined transparent word body cost included
opaque word declared cost included
primitive generator declared cost included
```

Important:

```boru
iota 1000
```

should be much cheaper than a literal list of one thousand integers.

This is the core of the MDL-style approach.

---

## 10. High-Level Reduction Algorithm

The top-level algorithm is:

```pseudo
function reduce_boru_generator(surface_source, property, options):
    profile = build_reduction_profile(options)

    ast = parse_boru(surface_source)

    checked_ast = check_embedded_profile(ast, profile)

    strict_ir = lower_to_strict_stack_ir(checked_ast, profile)

    strict_ir = normalize_ir(strict_ir, profile)

    typed_ir = infer_stack_effects_and_types(strict_ir, profile)

    result = evaluate_and_test(typed_ir, property, profile)

    if result != Fail:
        return NotReducible("initial generator does not fail")

    reduced = reduce_ir(typed_ir, property, profile)

    pretty = pretty_print_as_boru(reduced, profile)

    return ReductionResult(
        ir = reduced,
        source = pretty,
        cost = cost(reduced, profile),
        value = eval(reduced, profile)
    )
```

---

## 11. Main Reducer Loop

The reducer uses deterministic greedy descent, with an optional bounded best-first search.

```pseudo
function reduce_ir(initial_ir, property, profile):
    current = initial_ir
    current_cost = cost(current, profile)

    seen = Set()
    seen.add(fingerprint(current))

    step = 0

    while step < profile.max_reduction_steps:
        step += 1

        candidates = generate_candidates(current, profile)

        candidates = filter_candidates(candidates, current, seen, profile)

        candidates = sort_by_reduction_priority(candidates, profile)

        accepted = false

        for candidate in candidates:
            fp = fingerprint(candidate)

            if fp in seen:
                continue

            seen.add(fp)

            if candidate_is_acceptable(
                candidate,
                property,
                profile,
                current_cost
            ):
                current = normalize_ir(candidate, profile)
                current_cost = cost(current, profile)
                accepted = true
                break

        if accepted:
            continue

        improved = bounded_best_first_search(
            current,
            property,
            profile,
            seen
        )

        if improved.exists:
            current = improved.program
            current_cost = cost(current, profile)
            continue

        break

    return current
```

This does not guarantee global minimality, but it gives predictable, terminating reduction.

---

## 12. Candidate Acceptance

A candidate is acceptable only if it is cheaper, valid, deterministic, and still failing.

```pseudo
function candidate_is_acceptable(candidate, property, profile, current_cost):
    if not stack_well_typed(candidate, profile):
        return false

    if not effect_safe(candidate, profile):
        return false

    if not deterministic(candidate, profile):
        return false

    if cost(candidate, profile) >= current_cost:
        return false

    result = evaluate_and_test(candidate, property, profile)

    return result == Fail
```

Evaluation errors should usually be treated as invalid candidates, not failures.

```pseudo
function evaluate_and_test(ir, property, profile):
    eval_result = safe_eval(ir, profile)

    if eval_result.kind == Error:
        return Invalid

    value = eval_result.value

    if not satisfies_output_contract(value, profile):
        return Invalid

    prop_result = safe_run_property(property, value, profile)

    if prop_result == Fail:
        return Fail

    if prop_result == Pass:
        return Pass

    return Invalid
```

---

## 13. Candidate Generation

Candidate generation should be staged:

```pseudo
function generate_candidates(ir, profile):
    candidates = []

    candidates += structural_deletion_candidates(ir, profile)
    candidates += literal_shrink_candidates(ir, profile)
    candidates += collection_shrink_candidates(ir, profile)
    candidates += map_record_shrink_candidates(ir, profile)
    candidates += stack_simplification_candidates(ir, profile)
    candidates += quotation_shrink_candidates(ir, profile)
    candidates += user_word_candidates(ir, profile)
    candidates += generator_semantic_candidates(ir, profile)
    candidates += compression_candidates(ir, profile)
    candidates += type_directed_base_candidates(ir, profile)

    return candidates
```

Candidate order matters. Prefer:

```text
lower cost
lower estimated evaluation cost
higher-priority rewrite family
earlier source span
lexicographic fingerprint
```

---

## 14. Rewrite Families

### 14.1 Structural Deletion

Try removing subprograms while preserving stack validity.

Examples:

```text
unused value + drop       -> nothing
optional map key          -> removed key
list prefix + suffix      -> prefix
list prefix + suffix      -> suffix
dead quotation code       -> smaller quotation
```

### 14.2 Literal Shrinking

Examples:

```text
42        -> 0
42        -> 1
42        -> 21
"abcdef"  -> ""
"abcdef"  -> "abc"
true      -> false
```

### 14.3 List Shrinking

Examples:

```text
[1,2,3,4] -> []
[1,2,3,4] -> [1,2]
[1,2,3,4] -> [3,4]
[1,2,3,4] -> [1,2,4]
```

### 14.4 Map and Record Shrinking

Examples:

```text
remove optional fields
shrink field values
replace record with schema default
replace map with smaller valid map
```

### 14.5 Stack Simplification

Examples:

```text
dup drop        -> identity
swap swap       -> identity
quote body do   -> body, if safe
constant calls  -> folded literal, if cheaper
```

### 14.6 Quotation Shrinking

Shrink code inside quotations while preserving compatible stack effects.

Examples:

```text
collection each [body]     -> shrink collection
collection each [body]     -> shrink body
collection where [pred]    -> shrink predicate
fold seed [body]           -> shrink seed or body
```

### 14.7 User-Defined Word Rewrites

Depending on word policy:

```text
transparent word     -> inline and rewrite
opaque word          -> shrink arguments only
primitive generator  -> use custom reducer
frozen word          -> do not alter
```

### 14.8 Generator-Semantic Rewrites

Examples:

```text
gen-int {min:0,max:1000}   -> gen-int {min:0,max:1}
gen-list len elem          -> smaller len
one-of choices             -> simpler branch
frequency choices          -> lower or remove weights
gen-map schema             -> remove optional generated fields
```

### 14.9 Compression Rewrites

This is the MDL-inspired layer.

Examples:

```text
[0,1,2,3,4]       -> iota 5
["x","x","x"]     -> repeat 3 "x"
large repeated AST -> let-bound repeated subtree
range-like strings -> generated range expression
```

Compression should be bounded because it can become expensive.

---

## 15. Bounded Best-First Search

Greedy reduction can get stuck. Add a bounded best-first pass.

```pseudo
function bounded_best_first_search(current, property, profile, seen):
    frontier = PriorityQueue()
    frontier.push(current, priority = cost(current, profile))

    best = current
    budget = profile.best_first_budget

    while not frontier.empty() and budget > 0:
        budget -= 1

        p = frontier.pop_min()

        candidates = generate_candidates(p, profile)
        candidates = filter_candidates(candidates, p, seen, profile)
        candidates = sort_by_reduction_priority(candidates, profile)

        for c in candidates:
            fp = fingerprint(c)

            if fp in seen:
                continue

            seen.add(fp)

            if not stack_well_typed(c, profile):
                continue

            if evaluate_and_test(c, property, profile) == Fail:
                if cost(c, profile) < cost(best, profile):
                    return Improved(c)

                frontier.push(c, priority = cost(c, profile))

    return NoImprovement
```

This is a way to escape local minima without attempting full global optimization.

---

## 16. Optional Exact Final Pass

For very small programs, exact minimization under a cost bound may be possible.

```pseudo
function exact_minimize_under_bound(current, property, profile):
    current_cost = cost(current, profile)

    for target_cost in range(0, current_cost):
        for candidate in enumerate_programs_of_cost(target_cost, profile):
            if not stack_well_typed(candidate, profile):
                continue

            if evaluate_and_test(candidate, property, profile) == Fail:
                return candidate

    return current
```

This gives true optimality only under the chosen DSL, cost model, and bound.

---

## 17. Important Gotchas

### Gotcha 1: Do Not Reduce Surface boru Directly

Surface boru is for humans. Reduction should happen after:

```text
surface boru -> canonical strict stack IR
```

Otherwise, flexible syntax can make local rewrites non-local or ambiguous.

### Gotcha 2: Forward Collection Must Be Resolved First

Human-friendly argument ordering and forward collection should be desugared before reduction. The reducer should only see explicit stack effects.

### Gotcha 3: Cost Must Include Environment

If user-defined words hide complexity, the cost model must account for them.

Options:

```text
transparent word: cost includes body
opaque word: declared cost
primitive generator: declared semantic cost
frozen word: fixed cost
```

### Gotcha 4: Effects Must Be Excluded

A reducer requires deterministic replay. Disable or tightly control:

```text
IO
network
time
randomness
mutation
imports
concurrency
host callbacks
```

### Gotcha 5: Errors Should Not Become Counterexamples

Generation errors should usually mark a candidate invalid. They should not count as property failures unless the property explicitly tests error values.

### Gotcha 6: Compression Can Hide Readability

A short description is not always a good bug report. The pretty-printer should prefer readable descriptions when costs are close.

### Gotcha 7: Higher-Order Words Can Explode Candidate Search

Words such as `each`, `where`, `fold`, and similar combinators create many possible rewrites. Use staged priority and budgets.

### Gotcha 8: Description Complexity and Value Complexity Differ

A large generated value may have a small description:

```boru
iota 100000
```

The reducer should optimize description cost, not expanded value size.

### Gotcha 9: Strict Termination Requires Monotonic Cost

Every accepted rewrite must strictly lower cost. Otherwise, the reducer can loop.

### Gotcha 10: Global Minimality Is Expensive

The reducer can guarantee a local normal form under its rewrite strategy. It generally cannot guarantee the globally shortest failing program unless it performs bounded exhaustive search.

---

## 18. Example Reduction

Initial failing generator:

```boru
{
  users: [
    {id:0, role:"admin", active:true},
    {id:1, role:"admin", active:true},
    {id:2, role:"admin", active:true},
    {id:3, role:"admin", active:true}
  ],
  mode:"strict",
  retries:10
}
```

Possible reduction sequence:

```text
remove retries
remove mode
shrink users from length 4 to length 2
shrink users from length 2 to length 1
shrink id values
remove active if optional
compress repeated role field
```

Possible final result:

```boru
{
  users: [
    {id:0, role:"admin"}
  ]
}
```

If at least two users are required for failure, a more compact final result might be:

```boru
{
  users: iota 2 each [
    {id: args.0, role:"admin"}
  ]
}
```

The second is not smaller as expanded JSON, but may be smaller and more explanatory as a generator program.

---

## 19. Suggested Prototype Plan

### Phase 1: Pure Strict-Stack Reducer

Implement:

```text
BORU-G profile
surface -> strict IR lowering
stack-effect inference
cost model
basic local rewrites
failure-preserving greedy loop
```

Goal:

```text
Given a failing pure boru generator,
produce a smaller pure boru generator that still fails.
```

### Phase 2: Domain-Specific Generator Words

Add primitive generator semantics:

```text
gen-int
gen-string
gen-list
gen-map
one-of
frequency
schema-value
```

Each primitive gets:

```text
stack effect
cost
custom shrink candidates
pretty-printer support
```

### Phase 3: Compression Layer

Add synthesis of shorter descriptions:

```text
literal list -> iota/range/repeat
repeated subtrees -> let sharing
schema defaults
string repetition
map common structure
```

### Phase 4: Best-First and Exact Small Search

Add:

```text
bounded best-first search
candidate cache
final exact pass for small programs
evaluation budget reporting
```

### Phase 5: Evaluation

Compare against:

```text
plain JSON shrinking
delta debugging
Hypothesis-style choice-trace shrinking
grammar-aware reduction
boru program reduction
```

Measure:

```text
final cost
expanded value size
human readability
number of property evaluations
time to shrink
validity preservation rate
failure stability
```

---

## 20. Final Recommendation

boru is a credible and promising substrate for this idea, especially because:

```text
it is embedded
features can be disabled
strict stack mode exists
JSON-like inputs are natural
user-defined words can encode generation semantics
programs can be normalized before reduction
rewrite rules can operate on stack IR
```

The key recommendation is:

> Do not attempt to shrink arbitrary full boru source directly. Define a pure, deterministic, typed, strict-stack BORU-G profile and reduce its canonical IR.

The architecture should be:

```text
boru surface syntax
  -> embedded pure profile check
  -> strict-stack canonical IR
  -> type/effect validation
  -> MDL-style cost model
  -> failure-preserving rewrite engine
  -> readable boru pretty-printer
```

The first milestone should be modest:

```text
Produce a deterministic, replayable, lower-cost failing boru generator.
```

Only after that should the system attempt more ambitious MDL-style compression and small-program synthesis.

The research contribution is strong if framed as:

> **Failure-preserving reduction of embedded generator programs for structured property-based testing.**

or more succinctly:

> **Counterexample shrinking as failure-preserving program compression.**