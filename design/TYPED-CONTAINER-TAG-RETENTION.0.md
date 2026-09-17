# Typed-Container Tag Retention — the enabler for `{:T}` write enforcement

**Status:** Proposed. Design only; no compiler code changed by this document.
The maintainer has chosen **full enforcement** of `{:T}` writes (reject a
non-conforming write at BOTH check and runtime), which is impossible under the
current value model. This document specifies the value-model change that makes it
possible.

Companion reading: `legacy/TYPED-CONTAINER-ELEMENT-PRECISION.0.ignore` (D2 — reads/writes,
the doc this one unblocks), `REFINE-NEWTYPE-VS-SUBSET.10.md` (the reparent
machinery a subtype representation would reuse), `legacy/INTERPRETER-SPEED-PLAN.10.ignore`
(why `Value` is size-optimized — constrains the representation).

---

## Why the existing D2 doc is blocked (discovered this cycle)

`legacy/TYPED-CONTAINER-ELEMENT-PRECISION.0.ignore` assumed a `{:T}` value **retains** its
element type after construction ("the result stays statically `{:Integer}`"). It
does not. Three measured facts:

1. **Construction drops the tag.** `def m:{:Integer} {a:1}` runs
   `Unify({a:1}, {:Integer})`. The typed-vs-concrete branch
   (`unify_map.go:86` → `unifyTypedMapWithConcrete` → `unifyMapValues`,
   `unify_fold.go:65`) returns `NewMap(result)` — a **plain** concrete map. Probe:
   `IsConcrete=true, IsTypedMap=false, child=none`. The list twin
   (`unify_list.go:109 unifyTypedListWithConcrete`) is identical.
2. **The runtime never carries `{:T}`.** Immutable `set` (`setMapHandler`) returns
   `NewMap(out)` — untagged. A `{:T}` fn PARAM is bound to whatever concrete map is
   passed (a plain map). (Flex-`{:T}` was separately blocked/panicked here too;
   both are now fixed — see "Typed flex nodes".)
3. **So writes cannot be rejected at runtime, and the concrete top-level example
   cannot be rejected at all.** `def m:{:Integer} {a:1} (m set "b" "wrong")` has no
   element type at the `set` site. The only place a `{:T}` element type is live is
   **inside a `{:T}` param body**, and only in check mode (Part A of D2 threads it
   into the body carrier).

The maintainer's requirement — reject `(m set "b" "wrong")` at check AND runtime —
therefore needs the value itself to **carry `{:T}` as a concrete value**. That is
this document.

## What "tag retention" must deliver

A concrete `{:T}` container is a value that is simultaneously:

- **Concrete and readable** — `IsConcrete(v)` is true and `AsMap(v)`/`AsList(v)`
  return the payload, so every existing read/op (`get`, `size`, `for`, `.k`, dot-
  chains, comparison, rendering) is unchanged. This rules out the
  `ChildTypeInfo`-backed carrier form (`Data` would be `ChildTypeInfo`, `AsMap`
  returns nil — it is not concrete). See "Representation" for why the `Entries`
  field is not the vehicle.
- **Element-typed** — the element constraint `T` is recoverable at the value from a
  cheap, non-payload channel, so `set`/`setpath`/`merge`/`inject` can check a write
  and reads can narrow to strict `T`.

The invariant this buys (the whole point): **construction AND every write are
enforced ⇒ a `{:T}` value provably holds only `T` ⇒ reads are soundly strict `T`,
and a provably-disjoint dispatch is a real static error.**

## Representation — the linchpin decision

Three candidates, judged against the two facts above (`Value` is size-optimized;
concrete reads need `AsMap`/`AsList`) and against census/dispatch ripple.

### Option 1 — minted lattice subtype per element type (reparent)

Mint a `*Type` for each distinct `{:T}` (`Parent = TMap`, element constraint
carried by a `typedMapBehavior{elem}` via `MintTypeWithBehavior`,
`typetable.go:542`). `def m:{:T} body` **reparents** the concrete map to it
(`ReparentValue`, the refine/newtype path). `set`/reads recover `elem` from
`v.Parent`'s behavior.

- **Pros:** values stay concrete (`AsMap` works); reuses the reparent + custom-
  Behavior machinery; typed containers become *real types* — fits boru's "def binds,
  make instantiates" model and makes `typeof` honest.
- **Cons:** **broad census ripple.** Every `{:T}` value's `Parent` changes from
  `Map` to a minted subtype → touches dispatch admission, `typeof`, comparison
  ordering (Rank of a minted node), and the serialised `Value.ID`. Mint/dedup/
  retire/canonicalize per element type (incl. disjunct/nested children) is real
  surface. Highest blast radius; most "correct" long-term.

### Option 2 — one pointer field on `Value` (RECOMMENDED)

Add `elem *Type` to `Value`, **behind a pointer** exactly like the existing
`pos`/`tmeta`/`dynFrom` fields (nil for the overwhelming majority of values). A
concrete `{:T}` map is a normal `MapPayload` value with `elem` set; `Parent` stays
`TMap`.

- **Pros:** **lowest census ripple** — `Parent` stays `Map`, so dispatch, `typeof`,
  comparison, and `Value.ID` are all unchanged *except where we deliberately consult
  `elem`* (the write-check and, later, strict reads). Values stay concrete (`AsMap`
  works). Consistent with the established "one nil-cheap pointer" pattern the perf
  work already uses; a single extra 8-byte pointer, nil on the hot path.
- **Cons:** must thread `elem` through `CloneValue`, `ReparentValue`, `WithPos`, and
  the value constructors that should propagate it (`set`'s result). Must **not** let
  `elem` leak into equality or rendering (two maps with equal entries stay `Equal`
  and `String()`-identical regardless of `elem`) — otherwise `compare.tsv`/assert
  differentials churn. A disjunct/nested child needs `elem` to point at a real
  constraint type (mint a small carrier type or store a `*Type` whose behavior holds
  the disjunct — reuse the `{:T}` type-literal's existing child Value).

### Option 3 — `ChildTypeInfo.Entries` as the standard concrete form — REJECTED

Make `{a:1 :Integer}`-style `ChildTypeInfo{Child, Entries}` the representation for
all concrete `{:T}` values. Rejected: such a value is **not** `IsConcrete` and
`AsMap` returns nil, so every read/op would need a parallel entries-reading path
(enormous blast radius); the surface syntax is barely-supported and parses
confusingly (`{a:1 :Integer}` lexes as `{a:{1:Integer}}`).

### Recommendation

**Option 2** (pointer field). It reaches full enforcement with the **smallest
census surface** because `Parent` stays `Map` — the change is *additive and opt-in
at the consult sites*, not a lattice-wide reparent. Option 1 is the cleaner
long-term model but pays a whole-corpus reparent ripple up front; defer it unless
`typeof`-honesty or type-level dispatch on `{:T}` becomes a requirement.

## Enforcement points (Option 2)

1. **Construction (already the gate; now also SETS `elem`).**
   `unifyTypedMapWithConcrete` / `unifyTypedListWithConcrete` currently validate
   every element and return `NewMap`/`NewList`. Change: return the concrete
   container **with `elem = childType`**. This is the single origin of the tag.
   (The def-handler and `typed_bind.go` construction checks already run this Unify.)
2. **`set` (Map + List) — check + retain.** In `setMapHandler`/`setListHandler`:
   when the receiver's `elem` is set and the written value does not `Unify` with it
   → `type_error` at runtime; on success the returned copy **keeps `elem`**. Mirror
   in a check-mode `ReturnsFn` (the diagnostic surfaces inside a called `{:T}` param
   body — verified: an ungated `set` ReturnsFn diagnostic *does* surface at
   `FnBodyDepth=1`). This is the class-instance precedent (`setClassInstanceReturns`
   + `setClassInstanceHandler`) applied to `elem`.
3. **`setpath` / `merge` / `inject` — deep writes.** These delegate to
   voxgigstruct with no element check. Phase them AFTER `set` (documented, not
   silently skipped): a deep write into a `{:T}` container must re-validate the
   touched leaves against `elem`. `merge`/`inject` need a walk of the merged result.
4. **Reads — tighten to strict `T`.** Once the invariant holds, D2 Part B's read can
   move from `dynamic(T)` to strict `(T tor None)`. Land this LAST, behind the
   differential.

## Phasing (each phase validates against the census before the next)

- **Phase R1 — the field + construction origin.** Add `elem *Type` (pointer,
  nil-default), thread through `CloneValue`/`ReparentValue`/`WithPos`, set it in the
  two `unifyTyped*WithConcrete` origins. NO behavior change yet (nothing consults
  `elem`). Gate: census/differentials **byte-identical** (the field is inert). This
  de-risks the core-model touch in isolation.
- **Phase R2 — `set` write-check + result retention (runtime + check).** The
  maintainer's example now errors at check AND `boru run`. Gate: census green (valid
  corpus has no non-conforming writes); new pins for reject + conforming-keeps-tag.
- **Phase R3 — `setpath` deep-write enforcement (DONE); `merge`/`inject` deferred.**
  `setReachNative` enforces the element tag at the leaf write, at runtime, for
  arbitrary nesting (construction tags nested containers recursively, so the
  current-level `data` already carries the governing element type) and keeps the
  tag on the rebuilt copy; `setpathReturns` mirrors the SHALLOW (single-segment)
  case at check time (a deep path is runtime-only — compiled + interpreted raise
  identically, census byte-identical). `merge`/`inject` go through voxgigstruct and
  DROP the tag (result is a plain untagged map), so there is NO soundness hole (an
  untagged result makes no false `{:T}` claim) — only an invariant-completeness gap
  (merging into a `{:T}` loses the type). Enforce+retain across a deep merge needs a
  walk of the merged result; deferred (not silently skipped).
- **Phase R4 — exact/strict reads — ATTEMPTED, REVERTED (blocked).**
  `d2TypedContainerBound`'s single-type branch binds `child.Parent` (the
  element's lattice SUPERTYPE — a `{:Integer}` read narrows to `dynamic(Number)`,
  `{:String}` to `dynamic(Scalar)`) because a type literal's `.Parent` is its
  supertype. The obvious fix — bind the exact denoted type
  (`NewDynamicCarrier(DenotedTypeNode(child))`) — was tried and **reverted**: it
  DIVERGES the compiled/interpreted differential
  (`edge-containers-1.tsv:L114` — `def r (each [drop [9]] [0 0]) (r get 0) eq
  (r get 1)`: compiled `true`, interpreted `false`, 2 mismatches). The
  exact element type let a downstream `eq` fold over an each-produced typed list
  commit differently than the interpreter. So the supertype narrowing is
  imprecise but load-bearing; making reads exact needs the downstream
  dispatch/fold fixed FIRST (likely the each-result element typing + the eq
  compiled fold), then re-attempt. Strict `(T tor None)` reads are gated behind
  this. R2/R3's write-checks are already EXACT (they unify against the child
  value directly), so write soundness does not depend on R4.

Phase R1 is the make-or-break core-model step; if its census is not byte-identical,
the representation is leaking (into equality, rendering, `typeof`, or dispatch) and
must be corrected before any enforcement lands.

## Interaction with the shipped D2 A+B

D2 Part A (`typedContainerCarrier`, `core_helpers.go`) and Part B
(`d2TypedContainerBound`, `native_storage.go`) are in the tree, census-green. They
thread/narrow the **carrier** element type for `{:T}` PARAM bodies in check mode —
independent of tag retention (a param carrier already holds `ChildTypeInfo`). Tag
retention is what extends precision + enforcement to **concrete** `{:T}` values and
to **runtime**. A+B stay; R4 tightens B's reads once R2 makes them sound.

## Risks

- **Representation leak (Phase R1).** If `elem` reaches equality/rendering/`typeof`/
  dispatch, the census churns corpus-wide. Mitigation: R1 lands the field INERT and
  asserts byte-identical census before anything consults it.
- **Result-retention gaps.** A write path that builds a fresh container and forgets
  to carry `elem` silently drops enforcement downstream. Mitigation: centralise the
  "copy map/list preserving `elem`" in one helper; audit every `NewMap`/`NewList`
  that copies a receiver.
- **`elem` provenance for disjunct/nested children.** `{:(A tor B)}` / `{:[:Integer]}`
  need `elem` to reference a real constraint. Mitigation: reuse the child Value the
  `{:T}` type literal already carries (mint a member type if a bare `*Type` is
  insufficient).
- **Flex — DONE (see "Typed flex nodes" below).** `flex` only toggles mutability,
  so a `{:T}` flex is now a first-class, write-enforced, mutability-preserving node.
  The former panic (Appendix A) is fixed and the enforcement covers the in-place
  mutation API (`set` / `append` / `push` / `unshift`).

## Validation surface (every phase)

- `test/go/langspec`: `TestCompiledCoverage` (byte-identical census — R1 must not
  move it at all; R2+ only on buggy programs, which the corpus lacks),
  `TestSpecCompiledDifferential`, `TestVariationDifferential`, `TestOnlyMetaFallsBack`.
- `make fmt && make vet && make lint && make test && make cover-gate`.
- The 6-library voxgig four-surface harness.
- New pins (`lang/go/bytecode_typed_container_test.go`): a concrete `{:Integer}`
  retains its tag through a conforming `set`; a non-conforming `set` is a
  `type_error` at check AND raises at runtime; an untyped `Map` is unchanged;
  `typeof`/`String()`/equality of a tagged map are identical to the untagged map.

## Critical files

- `eng/go/value.go` — the `Value` struct + `elem` field; `CloneValue`; the
  `AsMap`/`IsConcrete` invariants (must stay true for a tagged value).
- `eng/go/unify_map.go` (`unifyTypedMapWithConcrete`:160) / `eng/go/unify_list.go`
  (`unifyTypedListWithConcrete`:109) / `eng/go/unify_fold.go`
  (`unifyMapValues`:65) — the construction origins that must SET `elem`.
- `eng/go/carrier.go`, `eng/go/core_boundedtype.go`, `typed_bind.go` — reparent /
  typed-bind paths that must thread `elem`.
- `lang/go/native/native_storage.go` — `setMapHandler`/`setListHandler` +
  `ReturnsFn`s (R2); `getNodeReturns`/`getIntKeyReturns` (R4).
- `lang/go/native/setpath.go`, `merge.go`, `inject.go` — R3.

## Typed flex nodes (the flex layer — DONE)

`flex` only toggles mutability; it must never drop the `{:T}`/`[:T]` element
contract. A `{:T}` flex is now first-class:

- **No panic.** The former `def m:{:Integer} (flex {a:1})` nil-deref (Appendix A)
  came from the typed-vs-concrete unify arm calling `AsMap`/`AsList` on a
  **check-mode flex carrier** (`(flex …)` is a `Data=nil` carrier there;
  `AsMap`→nil → `unifyMapValues(nil).Keys()`). Fixed: `unifyTyped{Map,List}
  WithConcrete` guards `!IsConcrete` and tags the carrier gradually;
  `unifyCarrierVsTyped` (scoped to genuine Flex{Map,List} carriers — a general
  dynamic carrier must still narrow/reject) handles the bare-carrier fallback.
- **Flex-preserving unify.** The typed-vs-concrete arm now returns a `NewFlexMap`
  / `NewFlexList` (not a plain immutable `NewMap`/`NewList`) when the concrete
  side is flex, carrying `elem`. `FlexDeepCopy` preserves `elem` too, so flexing an
  already-typed container keeps enforcement.
- **Write enforcement (in-place + copy).** Every write — `set`/`setpath` (immutable
  AND flex) + the flex grow API (`append`, `push`, `unshift`, incl. the list-concat
  spread) — routes the written value through `d2AdoptTyped`, which `Unify`s it
  against `elem`. That both enforces AND **recursively re-tags**: writing an
  UNTYPED `{y:2}` into a `{:{:Integer}}` stores it tagged `{:Integer}`, so a later
  write into IT is enforced. (Scoped to container element types — a scalar element
  stores the value byte-identical, no census churn.) This closed a real soundness
  hole the adversarial verify found: the write path used to validate-but-not-retag,
  so nested `{:{:T}}`/`[:[:T]]` subtrees written via set/grow became freely typed —
  the hole existed in the committed immutable `set`/`setpath` too, now fixed.
- **node↔flex round-trip.** `FlexDeepCopy` AND `NodeDeepCopy` preserve `elem`, so
  `flex`/`node` only toggle mutability and never drop the contract.
- **Three-surface agreement — over an open compiler defect.** Interpret
  validates the concrete flex body + enforces writes; there is no compiled
  typed-container bind, so `markTypedContainerDefUncompilable` refuses a
  `{:T}` def over a non-concrete (flex) body and the runtime SILENTLY routes
  the refused def to the interpreter instead (`--force-compile` surfaces the
  refusal; census stays byte-identical). That routing is scaffolding
  absorbing the refusal, not a fallback the design leans on and not a reason
  the refusal is tolerable: a missing compiled bind is an unimplemented case
  — an open defect, owed a fix and tracked to closure (see "Owed fix" below)
  — and routing it away silently hides the failure instead of fixing it.

Known, acceptable limits (precision, not soundness):
- **Flex writes are RUNTIME-enforced; check is conservative.** A flex node is
  mutable by reference, so `boru check` does not statically flag a bad flex write
  (`def m:{:Integer} (flex {a:1}) (m set b/q "wrong")` checks clean, runs-rejects) —
  the runtime is the source of truth. Immutable writes DO have a top-level check
  mirror (`d2CheckWrite`); flex does not, by design.
- **Construction over a flex carrier can't be statically validated** — a `(flex …)`
  check-mode residual is an abstract carrier; `def m:{:Integer} (flex {a:"s"})`
  checks clean but the interpreter rejects the concrete body. Gradual, sound.
- **A `{:T}` param is a plain typed map statically.** `def f fn [[m:{:Integer}]
  [FlexMap] …]` over a flex arg: the compiler analyses `set` on the param as
  returning `Map` (the param type carries no flex-ness), so a declared `[FlexMap]`
  return is a static mismatch — correct compiler strictness; declare `[Map]`/`[Any]`.

Owed fix: a compiled typed-container bind (an `OpBindTyped`-style container
kind), so a typed-flex def compiles instead of being refused and routed away.
This is not an optimization to schedule when convenient — every def the
compiler refuses today is a defect, tracked to closure. Done is a language
that compiles as a developer expects: ALL valid code compiles, no exceptions.

## Codex-review hardening (rounds 1–3)

The automated reviewer surfaced tag-loss holes that the original phasing left
open. Each is a place where a value flowed OUT of a typed container (or INTO a
`{:T}` param) through a path that rebuilt the container and dropped `elem`, so a
DOWNSTREAM write escaped enforcement. All fixes keep the census byte-identical
(the tag is a hidden field; both compiled and interpreted run the same handler).

- **Round 1 — immutable list-mutators + chained residual.** `push`/`unshift`
  over an immutable `[:T]` list bypassed the write-check; the `set` residual
  dropped the tag so a chained `((xs set 0 v) set 1 w)` lost enforcement. Fixed
  via `d2AdoptTyped` (enforce) + `d2RetainElem` (retain) on every mutator.
- **Round 2 — deep `setpath` / `merge` / flex check-mirror / flex-param compile
  divergence.** `setReachNative` now validates the value at EVERY set point (not
  just the leaf); `merge` re-tags the rebuilt result via `d2ReTagContainer`; the
  flex `set` residual retains the tag; a `{:T}` flex param dispatches through a
  DYNAMIC carrier (`typedContainerCarrier`/`paramBodyCarrier` set `Dynamic`) so
  the compiled dispatch rematches at runtime instead of preselecting a diverging
  handler.
- **Round 3 — the fundamental param-boundary hole + three tag-loss copies.**
  1. **Param binds the arg UNTAGGED** → a write inside the fn body escaped
     enforcement (BOTH a plain and a flex argument). `RetagTypedContainerParam`
     re-tags a concrete `{:T}`/`[:T]` arg with the param's element constraint at
     every binding site (`execFnDefSig`/`CallBoru`/`InstallFnDef` in the
     interpreter, `checkParamContract` on the compiled path) — so the body write
     is enforced identically in both. Dispatch already element-checks the arg and
     rejects a non-conformer before binding, so the re-tag `Unify` cannot fail.
  2. **`inject`** rebuilds a plain node through `valueToAny`/`structConvert`;
     `d2ReTagContainer` re-validates + re-tags it against the data template's
     `{:T}` (mirrors `merge`). The reject branch is unreachable through source (a
     `{:T}` container can't be CONSTRUCTED holding a marker), pinned by a direct
     handler test.
  3. **Flex chained check-residual** now retains the tag (`setFlexListReturns` /
     `flexGrowReturns` via `d2RetainElem`), so a second chained flex write is
     flagged in check too.
  4. **Copy / reorder stdlib ops retain the source tag.** Every op that returns a
     SUBSET or REORDER of one typed container preserves its element types, so the
     rebuilt copy must keep `elem`: `reverse`, `take`, `shed`, `unique`, `sort`
     (list AND map — a `{:T}` map sorted by value is still `{:T}`), `sortby`
     (both forms), `slice` (all three arities, via `sliceRetain`), and `filter`
     (all four forms, list and map). The audit deliberately EXCLUDES ops that
     transform or combine element types — `map`/`each` (transform), `fold`/`scan`
     (reduce), `where`/`grade` (indices), `flatten` (structural: a `[:[:T]]`
     flattens to `[:T]`), `concat`/`append` (combine — already per-element
     enforced via `d2AdoptTyped`), and `clone` (already preserves `elem`, since
     `CloneValue` copies the Value struct and only replaces `Data`/`ID`).
- **Round 4 — the args-stack escape + the last copy ops + read-narrowing shape.**
  1. **The args stack (`args.N`) bound the RAW arg**, so a body write via `args.0`
     bypassed the contract even though the named binding retagged — a second
     compiled-vs-interpreted divergence (compiled retags its locals in
     `checkParamContract`; the interpreter did not retag the args stack).
     `RetagTypedContainerArgs` retags the whole arg slice at each interpreter
     fn-entry (`InstallFnDef` closure, `execFnDefSig`, `CallBoru`) up front, so the
     named binding, the args stack, AND unnamed body-token pushes all see the
     tagged value. Copy-on-write — no allocation when no param is a typed
     container (the leaf fast path stays allocation-free).
  2. **`remove-at`/`at`/`replicate`/`compress`** are subset/gather/mask copies that
     preserve element types → retain the source `[:T]` tag; **`insert-at`** adds a
     NEW element → enforce it via `d2AdoptTyped` (store the retagged value) AND
     retain the tag. `expand` is correctly EXCLUDED — it injects `Integer(0)`
     fillers for false mask slots, so retaining a non-Integer source tag would be
     an unsound false claim; `member`/`indices` return Booleans/indices, not
     source elements.
  3. **The immutable `set` check residual is now a PROPER typed carrier** when the
     receiver is tagged (`d2TypedListResidual`/`d2TypedMapResidual`): the element
     type rides in BOTH `ChildTypeInfo.Child` (so a read from `(xs set i v)`
     narrows via `getIntKeyReturns` instead of degrading to `dynamic(Any)`) AND the
     `elem` pointer (so a chained write into the residual stays enforced —
     `d2CheckWrite`/`ElemConstraint` read the pointer, not the child). Setting only
     the child (the first attempt) silently broke round-3 #3's chained-write
     enforcement; both fields are load-bearing. Census stays byte-identical — the
     exact-read-narrowing divergence R4 hit (an each-produced list) does not
     recur for the set-residual path (verified by the differential + a 100-seed
     variation run, whose only failures are pre-existing unledgered refusal
     buckets, present on the baseline too — themselves open defects owed
     fixes, not a clean result).
- **Round 5 — the values projection + two check-mirror gaps.**
  1. **`vals`** projects a `{:T}` map's values to a list — every value is
     element-typed, so the result is `[:T]`; `valsHandler` retains the tag
     (`elem` is container-kind agnostic) and a `valsReturns` typed-list residual
     mirrors it in check. `keys` stays untagged (keys are always `String`).
  2. **The list-concat `append` overload** (`[TList, TFlexList]`, the more-specific
     pick for a list source) had NO `ReturnsFn`, so check never preflighted the
     per-element enforcement `appendListHandler` runs at runtime —
     `def f:[:Integer] (flex [1]) append [2 "bad"] f` passed check then raised.
     `appendListReturns` validates each provably-known spread element and retains
     the flex tag.
  3. **The `setpath`/`dynamicContainerKind` residual** is now a dynamic TYPED
     carrier when the receiver is tagged (`d2DynamicTypedResidual`): the element
     rides in both the child payload (read narrowing) and the `elem` pointer
     (chained-write enforcement), preserving the dynamic modality — the same
     both-fields shape as the immutable `set` residuals, extended to the dynamic
     `setpath`/`merge` residual family.
- **Round 6 — flex reference identity + the flex/node conversion residuals.**
  1. **The param retag broke flex identity** (a regression the round-3 #1 retag
     introduced). `RetagTypedContainerParam` ran `Unify`, which for a flex arg
     rebuilds a DETACHED `NewFlexList`/`NewFlexMap` — so a body write mutated a
     copy and the caller's flex was untouched, violating flex's reference
     semantics. Fixed by `RetagTypedContainerValue` (the shared core, used by
     BOTH the interpreter and the compiled `checkParamContract`, so they agree):
     a flex arg is retagged **in place** (header copy + `SetElemConstraint`, the
     `FlexListData`/`FlexMapData` pointer stays shared); a plain arg is still
     re-unified. Sound because dispatch already element-checks a flex arg and
     rejects a non-conformer before binding — no re-validate, no rebuild. Both
     paths agreeing was load-bearing: fixing only the interpreter opened a
     compiled-vs-interpreted divergence (`[99 2 3]` vs `[1 2 3]`).
  2. **`flexReturns`/`nodeReturns` emitted bare carriers**, so a top-level write
     into a typed flex/node result (`append "bad" (flex xs)` for `xs:[:Integer]`)
     wasn't diagnosed in check even though runtime raises. Both now retain the
     source element tag (`d2RetainElem`), so the check-mode write mirror
     (`d2CheckWrite` reads `ElemConstraint`) fires.

  (Round 8 closed the nested-flex gap this note originally described — see below.)
- **Round 7 — the index-merge governing operand + merge check residual.**
  1. **The list↔map index-merge handlers picked the wrong governing tag.** Both
     `mergeListMapHandler` and `mergeMapListHandler` build a LIST result (the
     map's integer-keyed values are written INTO the list), so the LIST operand's
     `[:T]` governs — but they re-tagged against `d2typedMergeOperand`, which
     prefers the MAP patch. So `{:String}{"0":"bad"} merge [:Integer][1]`
     validated the result against `String`, succeeded, and returned a `[:String]`
     list instead of rejecting `"bad"` against the destination's `Integer`
     contract — a soundness hole. Fixed to re-tag against the list operand
     (`args[0]` for list-map, `args[1]` for map-list). `d2typedMergeOperand` stays
     for the same-kind generic `merge` (both operands are the result's kind).
  2. **`mergeReturns` emitted plain dynamic carriers.** A chained write after a
     same-kind typed merge (`def r (merge m {b:2}) (r set c/q "bad")` for
     `m:{:Integer}`) wasn't diagnosed in check. It now returns
     `d2DynamicTypedResidual` over the governing operand, so the residual carries
     the child + `elem` and the chained write is flagged.
- **Round 8 — nested flex retag + the last two check residuals.**
  1. **A nested-typed flex param left its children untagged** (the round-6
     limitation, promoted to a P1 soundness hole): `def f fn [[m:{:{:Integer}}]
     …] [(m.a set y/q "bad")]]; f (flex {a:{x:1}})` succeeded because the header
     retag tagged only the outer map, so the inner `m.a` write bypassed the
     `{:Integer}` contract. `retagFlexElem` now RECURSIVELY retags a flex's
     EXISTING children in place — `FlexDeepCopy` makes the whole tree flex, so
     every child is a mutable flex container whose header tag is written back into
     the shared store. Identity is preserved (no rebuild) AND every depth
     enforces; interpreter and compiled agree (shared `RetagTypedContainerValue`).
  2. **The index-merge overloads and `clone`** now carry typed check residuals
     (`mergeListMapReturns`/`mergeMapListReturns` over the list operand;
     `cloneReturnsFn` via the both-fields typed residual for a plain list/map and
     `d2RetainElem` preserving the exact Parent for a flex/other kind), so a
     chained write after an index merge or a clone is diagnosed in check —
     runtime already retained the tag (`d2ReTagContainer` / `CloneValue`).
- **Round 9 — flex-child identity on the WRITE path + list-copy check residuals.**
  1. **`d2AdoptTyped` rebuilt an already-flex child** (the same flex-identity
     regression, now on the write path). Writing a flex child into a nested-typed
     flex container (`d:{:{:Integer}} set a c` where `c = flex {x:1}`) ran
     `Unify`, whose flex branch allocates a fresh store — so a later mutation
     through `c` was invisible through `d.a`. Now, after `Unify` VALIDATES, a flex
     child is retagged IN PLACE via the exported `RetagFlexElem` (the same helper
     round 8 introduced for params); a plain child keeps the rebuilt tagged value.
  2. **The list-copy check residuals dropped `elem`.** `ReturnsPreserveListAt`
     built a typed-list carrier (`ChildTypeInfo.Child`) but never copied the
     source's `ElemConstraint` pointer, and `d2CheckWrite` consults only the
     pointer — so `(reverse xs) set 0 "bad"` for `xs:[:Integer]` wasn't diagnosed
     at check despite the tagged runtime result. Fixed centrally in
     `ReturnsPreserveListAt` (copies the source `ElemConstraint`), covering every
     op that uses it (reverse / take / shed / sort / …).
- **Round 10 — the last two check-mirror gaps.** The plain immutable `push` /
  `unshift` grows and the list `slice` overloads had NO `ReturnsFn` (unlike their
  FlexList / reverse-take counterparts), so a bad top-level grow
  (`def xs:[:Integer] [1] (push "bad" xs)`) and a chained write after a grow or
  slice weren't diagnosed at check even though the runtime rejected them.
  `plainListGrowReturns` (d2CheckWrite + typed residual) and `sliceListReturns`
  (typed residual over the data-list arg) fill them; the string `slice` overloads
  are deliberately left bare (a string result carries no element tag).
