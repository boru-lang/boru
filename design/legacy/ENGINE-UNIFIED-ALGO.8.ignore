# Unified Core Engine Algorithm (Proposal)

> **⚠️ LEGACY / HISTORICAL — retained for project history.**
> This is a pre-unification *proposal*. Its "current engine behavior" review
> describes a multi-path dispatcher that no longer exists, and several named
> entities here were never the shipped design or have since been removed:
> `MatchSignature`/`bestSigForForward`/`shouldResolveForwardEarly`/
> `couldProduceType` as separate scoring paths, the feature-flagged planner,
> and — notably — the claim below that `flexibleMatch` "supports argument
> reordering (permutations for small arity)": **the shipped matcher never
> permutes arguments** (`eng/go/signature.go`: `FlexibleMatch` — "Arguments
> are never permuted"). What actually shipped is the single
> `BarrierPos`-driven `matchSignature`. For the current model see
> `eng/go/CLAUDE.md` ("Signature Ordering"), `design/SIG-ORDER-REFACTOR.10.md`,
> and `design/FORWARD-COLLECTION-PHASES.10.md`.

This note reviews the current engine behavior and proposes a single unified algorithm
for signature-driven prefix/forward/infix execution with precedence.

## Current engine architecture (review)

The implementation currently splits the core behavior across several interacting passes:

1. **Immediate prefix dispatch** via `MatchSignature` with `flexibleMatch` over the
   top-`N` resolved stack values (`signature.go`).
2. **Suffix fallback** via `Forward` markers inserted by `insertForward` (`engine.go`).
3. **Heuristic signature selection for forwards** in `bestSigForForward`, including
   peek-ahead scoring bonuses (`engine.go`).
4. **Type-directed collection + overload switching** while collecting forward args in
   `stepLiteral` (`engine.go`).
5. **Early forward resolution** via `shouldResolveForwardEarly`/`couldProduceType`
   (`engine.go`).
6. **Termination controls** using explicit `end` and implicit mismatch termination
   (`stepEnd`, `implicitEnd`, `curryOrPrefix` in `engine.go`).

This works and is feature-rich, but behavior is distributed across multiple heuristics
that all participate in one semantic concern: *find the best function invocation plan
from typed values, word definitions, and precedence*.

## Type-signature matching today

### Strengths

- `Type.Matches` supports hierarchical subtyping (`String/Proper` matches `String`).
- `flexibleMatch` supports argument reordering (permutations for small arity).
- `signatureScore` gives deterministic tie-breaking by arity + specificity.
- Forward collection can switch overloads as more forward information arrives.

### Friction points

- Matching policy is split between `MatchSignature`, `bestSigForForward`, and
  `shouldResolveForwardEarly`, with different local scoring logic.
- Prefix and forward are modeled as separate control paths that later converge.
- Precedence is enforced as a local defer rule during collection, not from a single
  global notion of binding power.
- Future type prediction (`couldProduceType`) is useful, but currently isolated from
  normal overload selection.

## A more unified algorithm

Use a **single invocation planner** based on *pending call frames* and *candidate
signatures with incremental constraints*.

### Core model

Represent each encountered function word as a frame:

- `word`, `position`, `mode` (normal/forced prefix/forced forward)
- `minBP` (binding power / precedence floor)
- `candidates[]` where each candidate has:
  - signature pointer
  - bound stack args (prefix)
  - bound forward args
  - unmatched argument slots
  - score (same unified formula)

Instead of choosing prefix first and forward later, each candidate can bind from:

- available resolved stack values (prefix), and
- future produced values (forward),

subject to precedence and type compatibility.

### Execution loop (high level)

1. Read token.
2. If literal/value: attempt to bind it to the **highest-priority open frame** that can
   accept it; else push as plain value.
3. If word/function: open a new frame with all valid signature candidates.
4. Repeatedly reduce frames that are *ready* (all required slots bound, or explicitly
   terminated by `end`).
5. Execute reduced frame, push results as values, and continue.

Precedence falls out naturally if each frame has binding power and only captures forward
values while no tighter frame to its right is active.

### Unifying match logic

Replace multiple matching helpers with one matcher used in all states:

- `matchCandidate(candidate, availablePrefixValues, nextTokenTypePrediction)`
- uses one compatibility function (`Type.Matches` + stable assignment solver)
- uses one scoring function (arity/specificity/prefix-use/adjacency/precedence)

For argument assignment, use a deterministic bipartite matching (Hungarian/DFS augmenting
paths is enough for small arities) rather than separate positional + permutation code.
This keeps flexibility but removes mode-specific branches.

### Unified end conditions

A frame is finalized when one of these becomes true:

- candidate has all required args,
- `end` closes nearest frame,
- no candidate in frame can accept the next token class.

Then pick best surviving candidate, execute, or curry if incomplete and curriable.
This absorbs current `implicitEnd`, `shouldResolveForwardEarly`, and `curryOrPrefix`
into one state transition.

## Benefits

- **Single source of truth** for overload selection.
- **Consistent semantics** across prefix/forward/infix and modifier modes.
- **Fewer heuristics** and less duplicated scoring logic.
- **Easier proofs/tests**: frame transitions can be table-tested.
- **Extensible** for optional/named args later.

## Migration strategy

1. Extract a standalone `planner` package with frame/candidate state and matching.
2. Port existing `MatchSignature` + `bestSigForForward` scoring into one scorer.
3. Gate with feature flag (`engine.unifiedPlanner`) and run integration tests in both
   modes for parity.
4. Remove old forward-specific heuristics after parity on SQL/boru fixtures.

## Practical recommendation

Yes — there is a more unified algorithm, and the frame/candidate planner is the most
natural fit for this engine because the language already has:

- typed overloads,
- forward collection,
- precedence,
- partial application (`curry`).

Those are exactly the features that benefit from a single incremental constraint solver
instead of split prefix-vs-forward control flow.


## Migration status

As of the current implementation, the unified planner path is now the default
engine behavior for forward signature selection (no feature flag split). The
legacy forward-signature scoring path has been removed from runtime dispatch.

