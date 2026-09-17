# Enforced container refinements via word-keyed `behave` advice

Status: **design note / implementation plan — no code yet.** Grounded in the
live tree at `ab8f28e`. Companion reading: `BEHAVIORS.10.md` (the `behave`
word), `REFINE-NEWTYPE-VS-SUBSET.10.md` (the two refine forms),
`FLEX-NODES.10.md` (the mutable-container column), `USURP.10.md` (why the
`ref` family is NOT this mechanism).

## 1. Motivation and surface

Node containers should be extensible with **enforced behavioral invariants**:
a List that holds unique elements only, a List kept in a custom sort order, a
Map ordered by key. The invariant is not merely *checked* — it is
**maintained**: constructing the container normalises it, and every edit
re-establishes it.

The surface is aspect-oriented: a plain nominal refine names the type, and
**word-keyed advice** installed via `behave` intercepts the container words
that establish or perturb the invariant:

```boru
def Uniq (refine List)
behave make/q (fn [[v:Uniq] [Uniq] [v unique]])
behave push/q (fn [[v:Uniq] [Uniq] [v unique]])

make Uniq [3 1 2 1]          ; → [3 1 2], typeof → Uniq
(make Uniq [1 2]) push 1     ; → [1 2], still Uniq
(make Uniq [1 2]) push 3     ; → [1 2 3], still Uniq
```

```boru
def Sorted (refine List)
behave make/q (fn [[v:Sorted] [Sorted] [v sort]])
behave push/q (fn [[v:Sorted] [Sorted] [v sort]])

(make Sorted [1 3]) push 2   ; → [1 2 3]
```

The pointcut is (word × type), the advice is a normalize-after function, and
the base handler keeps doing exactly what it does today — advice runs on its
result. A rejecting invariant raises from the advice body and surfaces as the
word's error:

```boru
def NonEmpty (refine List)
behave pop/q (fn [[v:NonEmpty] [NonEmpty]
  [if (v size eq 0) [raise would_empty "NonEmpty cannot be emptied"] [v]]])
```

### Why interception, and not a type capability or predicate refine

Three dead ends were verified before landing on this design:

- **Predicate refinement is validation-only and scalar-only.** The dependent
  machinery (`DepScalarInfo`, `depscalar.go`) carries scalar bounds
  (`Integer gt 10`) evaluated per-value with no access to sibling elements —
  it cannot express "all elements unique", and nothing in the refine path
  *coerces or maintains* anything; `Match` is a read-only membership test at
  the `is`/dispatch/return boundaries.
- **Builtin words are frozen.** `def make …` is refused
  (`native_definition.go`'s `reservedWordError` on `IsBuiltinWord`), so
  "wrap `make` in boru" is not available; interception must be a hook the
  builtin handlers themselves consult.
- **The `usurp`/`ref` family is value-level.** `usurp`, `force-arity`,
  `stack-args` wrap a *function value* (argument permutation / barrier
  adaptation) and never touch a registry entry — not a word-interception
  primitive.

What the tree *does* have is `behave`
(`lang/go/native/native_behave.go`): a closed table of capability slots
(`compare`, `canon`, `nodify`, `unify`) that attaches a boru fn body to a
**user** type by additively wrapping its `TypeBehavior` (`userBehavior`),
running the body through a sub-engine with re-entrancy guards, and inferring
the target type from the fn's first parameter. That is precisely the AOP
substrate this feature needs — it is only missing word-keyed slots and the
handler-side consult.

## 2. Decisions

Locked with the user, in order of consequence:

1. **Enforcement is the point.** Not just a type you can assert with `is` —
   the container maintains its invariant through construction and edits.
2. **Advice shape = normalize-after.** The base operation runs first; the
   advice fn `(v:T => T)` receives the result already subtype-tagged and
   returns the normalized value, or raises to reject. One uniform shape for
   every pointcut; no `proceed`, no before/after pair. (Full around-advice —
   inputs plus an explicit proceed — was considered and rejected for v1:
   proceed-as-value, re-entrancy, and compiler interactions triple the
   surface for no motivating use case.)
3. **Pointcuts v1 = `make` + the copy-returning edit column.** Verified
   inventory of what actually exists:
   - List refines: `make`, `set`, `push`, `unshift`, `pop`, `shift`,
     `insert-at`, `remove-at`.
   - Map refines: `make`, `set` (there is no core map-key-removal word).
   - `append` is **Flex-only** (`{TAny,TFlexList}` etc., `native_flex.go`) and
     `remove` is entity/table CRUD (`natives.go` → `remove.go`) — neither is a
     plain-container pointcut.
4. **Flex refines are out of scope in v1.** Normalize-after is only sound for
   the copy-returning column: an in-place FlexList mutation is visible through
   every aliased handle *before* advice could run, so a shared handle could
   observe a state violating the invariant. The
   `builtinBaseOf(T).Equal(TList) || .Equal(TMap)` gate (below) excludes
   `refine FlexList` naturally (its builtin base is `TFlexList`).
5. **Tag decay rule.** Edit words preserve the subtype tag and re-run advice;
   **derivation** words (`each`, `filter`, `sort`, `concat`, `slice`,
   `convert`, `flex`, …) return plain `List`/`Map` — which they already do
   (`NewList` hardcodes `Parent=TList`, `value.go:1227`). The tag survives
   exactly where the invariant is enforced; everywhere else the value honestly
   decays to its base type rather than carrying an unverified claim. This
   matches the nominal-newtype philosophy of bare refine.
6. **Surface = extend `behave`.** No new word. The `behaviors` table already
   has the `{validate, install}` entry shape word slots need; the target type
   is inferred from the advice fn's first param exactly as `behave compare`
   infers it today.
7. **Weak references are out of scope.** They are a value-identity + GC axis,
   not a structural invariant — a separate design if ever pursued.

## 3. What works today vs. what is missing

Verified against the tree (file:line anchors current at `ab8f28e`):

**Already working — the nominal-subtype half:**

- `def Uniq (refine List)` mints a genuine List subtype: `refineBareHandler`
  (`lang/go/native/native_type.go:398-421`) has no scalar guard;
  `InstallType`'s prefab rename + `installBareRefineUnifier`
  (`eng/go/core_type.go:400-436`); nominal `bareRefineUnifier.Match`
  (`eng/go/unify_refine.go:29-36`) — a plain List is NOT a Uniq, only a
  Uniq-tagged value matches.
- A subtype-tagged container **dispatches through every List/Map word**: all
  container probes use `ConformsTo` walks, none exact-Equal
  (`signature.go:255-337`, `native_storage.go`, `native_flex.go:96-116`), and
  sig-specificity ordering already ranks a subtype sig above its base
  (`sigorder.tsv:41-59`).
- `behave` already accepts refined subtypes as targets
  (`lang/go/test/behave.tsv:329-337`) and refuses builtin types
  (`native_behave.go:136-138`) — the right polarity for this feature.
- The typed-def path already constructs tagged containers:
  `defTypedHandler`'s refine-bare branch reparents via `ReparentValue`
  (`native_definition.go:552-588`).

**Missing — the construction and preservation half:**

- `make Uniq [3 1 2]` **errors** ("make: unsupported target type Uniq"): the
  tag-on-construct branch in `MakeHandler` is gated
  `base.ConformsTo(TScalar)` (`eng/go/core_make.go:576-590`, gate at :579), so
  a node-family refine falls through `MakeNodeHandler`'s exact-`Equal` switch
  (`core_flex.go:295-372`) to `MakeConvert`'s default error
  (`core_make.go:957`).
- Copy-returning mutators **drop the tag**: `setListHandler` and friends
  rebuild results with `NewList`, which hardcodes `Parent=TList`
  (`value.go:1227-1229`). A Uniq would decay to List after one `set` even
  before advice enters the picture.
- No word-keyed slot exists in the `behaviors` table, and no handler consults
  a type for word advice.

## 4. Mechanism

Four coordinated pieces. The layering constraint that shapes them: `behave`
and `userBehavior` are **lang**-side, but `make` lives in **eng**
(`core_make.go`) while the mutator handlers are lang-side — so the capability
interface must live in eng, discovered the same way `Comparer` is.

### 4.1 eng capability: `WordInterceptor` (new file `eng/go/word_advice.go`)

```go
// WordInterceptor is the optional TypeBehavior capability behind word-keyed
// behave advice. Implemented lang-side by *userBehavior.
type WordInterceptor interface {
    // InterceptWord runs the advice installed for word on result (already
    // subtype-tagged). ok=false means no advice installed for word.
    InterceptWord(word string, result Value, r *Registry) (Value, bool, error)
}

// ContainerRefineTag returns the canonical *Type when v is tagged with a
// user refine whose builtinBaseOf is exactly TList or TMap; nil otherwise.
func ContainerRefineTag(r *Registry, v Value) *Type

// ApplyWordAdvice walks result.Parent's canonical chain up to (not
// including) the first OriginBuiltin ancestor looking for a WordInterceptor
// carrying advice for word. In check mode it runs NO advice (see §4.4).
func ApplyWordAdvice(r *Registry, word string, result Value) (Value, error)

// RetagAndAdvise: if receiver carries a container-refine tag, reparent
// result to it (ReparentValue) and ApplyWordAdvice. Identity otherwise —
// plain receivers are byte-identical to today.
func RetagAndAdvise(r *Registry, word string, receiver, result Value) (Value, error)
```

The walk mirrors the `Comparer` LCA walk (`compare.go:74-91`) and `Sizer`
(`size.go:17-24`):
`for t := CanonicalType(r, result.Parent); t != nil && t.Origin != OriginBuiltin; t = t.Parent`.
`CanonicalType` is mandatory at every step — `behave` writes through the
canonical pointer (see eng/go/CLAUDE.md "Canonical `*Type` Pointers").
`builtinBaseOf` already exists (`core_make.go:599-608`) and is package-private
to eng, which is where these helpers live. The gate is **`Equal`**, not
`ConformsTo` — that is what keeps Flex refines out (FlexList *conforms to*
List but is not `Equal` to it).

### 4.2 `behave` word-keyed slots (lang, `native_behave.go`)

The `behaviors` table keeps its exact `{validate, install}` shape; word
entries share one constructor:

```go
func wordAdviceEntry(word string) behaviorEntry {
    return behaviorEntry{
        validate: validateWordAdviceSig,
        install: func(u *userBehavior, body []eng.Value) {
            if u.wordBodies == nil { u.wordBodies = map[string][]eng.Value{} }
            u.wordBodies[word] = body
        },
    }
}
```

registered for `make`, `set`, `push`, `unshift`, `pop`, `shift`, `insert-at`,
`remove-at`. `validateWordAdviceSig` enforces the shape `[[T] [T-or-Any]
[body]]` — exactly one param with a declared type (the target, extracted like
`validateCompareSig` does) and one return equal to T or `Any` (the `Any`
escape mirrors `validateUnifySig`: `ParseFnReturns` degrades user types
without a registry). In `behaveHandler`, after the existing `OriginBuiltin`
refusal, word entries additionally require `builtinBaseOf(target)` to be
exactly `TList` or `TMap` — `behave push` on a scalar refine errors
("must be a refine of List or Map"). The unknown-name error text is updated to
list the new names; the table stays closed.

`userBehavior` gains:

```go
wordBodies   map[string][]Value // word → advice body tokens
adviceTarget *eng.Type          // T from the word-advice sig
adviceParam  string             // declared first-param name; "" → "a"
inWordAdvice map[string]bool    // per-word re-entrancy guard
```

`InterceptWord` delegates to `prev` when it has no body for the word (the
additive-wrapper discipline every other capability follows); on re-entry it
falls through returning the value unchanged — the behave precedent
(Format/Nodify/Unify guards): while the advice body runs, *it* is the
enforcement authority, so a `make Uniq` inside Uniq's own make-advice
terminates instead of looping.

`runWordAdviceBody` mirrors `runUnifyBody` (`native_behave.go:441-509`)
exactly: push the param as a def (the declared name, so `fn [[v:Uniq] …]`
bodies read naturally; `"a"` fallback preserves the compare/canon
convention), run via `eng.NewTop(r).Run(tokens)` (the behave sub-engine
precedent — NOT `InvokeBody`/`CallBoru`, keeping all five capability runners on
one seam), wrap an error as `behave <word> <Type>: …` so the calling word
surfaces it. Output validation, in this order:

1. result tagged `adviceTarget` or a subtype → accept as-is;
2. result in the **base family** (a plain List/Map, e.g. from a `[v unique]`
   or `[v sort]` body) → **reparent to `adviceTarget`** so natural bodies
   re-tag without ceremony;
3. anything else — a *sibling* refine of the same base (which also
   `ConformsTo` the base, hence the ordering above), a wrong family, a type
   literal, a carrier — errors
   (`behave make Uniq: body returned …, expected a value in the Uniq family`).

### 4.3 The seams

**`make` (eng, `core_make.go:576-590`).** Inside the existing
`canon.Origin == OriginUserDef` block, a sibling to the scalar gate:

```go
if base != nil && (base.Equal(TList) || base.Equal(TMap)) {
    // srcVal must be concrete and conform to base, else a clear error
    out, err := NodeDeepCopy(srcVal)   // immutable snapshot; nested flex normalised
    tagged := ReparentValue(out, targetType)
    advised, err := ApplyWordAdvice(reg, "make", tagged)
    return []Value{advised}, nil
}
```

Note `MakeHandler`'s existing short-circuit at :564 (source already conforms
to target → pass through) means re-making an already-tagged Uniq skips advice
— correct, it was normalized at construction; a plain List never conforms to
Uniq (nominal), so it always reaches the new branch. `make`'s check-mode
carrier already comes out subtype-typed via `ReturnsFreshInstance(0)`
(`native_make.go:38`).

**Typed-def (lang, `native_definition.go:571-573`).** `def x:Uniq [1 1 2]` is
the *other* construction path; after `ReparentValue(body, def)` it runs
`eng.ApplyWordAdvice(r, "make", tagged)` (skipped under `r.Check.IsActive()` —
the check path binds a carrier anyway) so the binding is `[1 2]`. Without
this, a typed def would mint an unenforced Uniq and the invariant would be a
lie.

**The eight mutator return sites (lang).** Each becomes
`RetagAndAdvise(r, "<word>", receiver, result)`:

| word | handler / result-build anchor | receiver |
|---|---|---|
| set (Map) | `setMapHandler`, `native_storage.go:251` | `args[2]` |
| set (List) | `setListHandler`, `native_storage.go:761` | `args[2]` |
| push / unshift | `listops.go:29` / `:72` | `args[1]` |
| pop / shift | `listops.go:48` / `:91` — advise the **list** result only | `args[0]` |
| insert-at / remove-at | `native_array.go:970` / `:998` | `args[2]` / `args[1]` |

Sig matching needs no change: a Uniq-tagged list satisfies the `TList` slots
and does not match the higher-ranked `TFlexList` overloads.

**Static parity (`ReturnsFn`).** Those sigs statically declare
`Returns:[TList/TMap]`, so a check-mode carrier for
`(make Uniq …) push 1` would decay to List — failing a declared `[Uniq]` fn
return and hiding the tag from the emit-pass probe (§4.4). A small lang-side
`ReturnsPreserveContainerTag(receiverPos int, fallback *Type) ReturnsFn`
(pattern: `flexReturns`, `native_flex.go:91-104`) returns a carrier of the
receiver's `ContainerRefineTag` when present, else the fallback; installed on
the eight sigs. `remove-at` already uses `ReturnsPreserveListAt(1)` — the
helper composes (element-type preservation for plain receivers, tag carrier
for refined ones).

### 4.4 Bytecode compiler: three belts

Soundness rides on the differential gate as always; the compiled path must
never run an advice-carrying dispatch natively. Three belts, in increasing
specificity:

1. **Already true (verified).** Any program *containing* a `behave` call
   latches whole-program uncompilable today: the Atom-form sig carries
   `QuoteArgs{0:true}` → the quoted-operand refusal (`emit.go:2218-2236`),
   and the String-form has a bare `TFunction` operand → the function-operand
   screen (`:2283-2301`); `MarkUncompilable` latches (`:558-563`). So the
   same-program case needs **zero** new compiler work.
2. **Check-mode marking.** `ApplyWordAdvice` never runs advice in check mode;
   if advice exists for the word and the emit pass is active, it calls
   `es.MarkUncompilable("word-advice container op at " + word)` and returns
   the result unchanged.
3. **Emit-pass operand probe.** The hole left by 1–2 is the
   persistent-registry case: `a.Run("…behave…")` installs advice, then
   `a.RunCompiled("(make Uniq [1 2]) push 1")` on the same instance — program
   2 contains no `behave`. A new `anyWordAdviceCarrier(args, outs)` case in
   `recordCallRefusal` (beside `anyDynamicCarrier`, `emit.go:~2237`) walks
   each carrier's non-builtin `Parent` prefix for a `WordInterceptor` with any
   installed slot — this is why the interface must be eng-visible. It works
   because `ReturnsFreshInstance` and `ReturnsPreserveContainerTag` keep the
   carriers subtype-typed.

Plus one fix: `markRefineDefUncompilable`'s `IsConcrete(body)` fast-path
(`native_definition.go:280-294`) would let a check-time-folded
`def x:Uniq [1 1]` bake an **un-advised** const; the call sites (:453, :572)
pass the resolved def type and skip the fast-path when it carries word advice.

## 5. Staged implementation

Each stage lands independently green under
`make fmt && make vet && make lint && make test`.

1. **`make` constructs container refines + tag preservation (no advice).**
   `word_advice.go` skeleton (the walk finds no interceptor yet, so landing
   the `ApplyWordAdvice` calls is safe), the `core_make.go` construct branch,
   the eight retag return-sites, `ReturnsPreserveContainerTag` + sig wiring.
   Spec rows pin: construction + tag survival through every pointcut word,
   derivation decay (`each` → List), flex decay (`flex (make Uniq …)` →
   FlexList), source-shape errors, `refine FlexList` still erroring, typed-def
   tagging.
2. **Word slots + `WordInterceptor` + advice at `make` + compiler belts.**
   The behave table entries, validator, `userBehavior` fields, body runner;
   check-mode marking; `anyWordAdviceCarrier`; typed-def advice +
   `markRefineDefUncompilable` fix. A Go test pins the persistent-registry
   fallback: `Run(install)` then `RunCompiled(use)` reports not-compiled with
   interpreter-equal results (pattern: `bytecode_computedmap_test.go:84`).
   Spec rows: make-advice normalizes / stays tagged / typed-def advised /
   advice raises to reject / wrong-family output refused / builtin target
   refused / scalar refine refused as advice target / unknown slot refused.
3. **Advice at the mutator seams.** The Stage-1 retag calls become full
   `RetagAndAdvise`. Spec rows: push/set advice re-fires and stays tagged; a
   `Sorted` ordering invariant; Map `set` advice; `pop` advice rejecting
   ("would empty"); advice composing across chained edits; un-advised
   pointcuts still just retagging; sibling-refine advice output rejected;
   re-entrancy fall-through.
4. **Docs.** `REFERENCE.md` currently has no `behave` section — add one
   covering all slots (compare/canon/nodify/unify + word advice), the decay
   rule, and the enforcement idiom. Extend `BEHAVIORS.10.md` with the
   word-advice chapter. Help text: the `behave` entry in
   `help/help_type.go:277-281` + examples; check `doc_completeness_test.go`.
   No ADR (repo rule — this is discovery).

Spec rows live in `lang/go/test/behave.tsv` (the Go-only suite, where every
behave row already lives). The TS engine runs only the shared kernel corpus
`eng/spec/*.tsv` and has no `behave` word — **no TS work**.

## 6. Edge cases (decided)

- **Advice fires only on the top-level tagged receiver.** Elements keep their
  own tags; editing an outer map does not re-run an inner Uniq's advice.
- **`pop`/`shift`** advise the remaining-list result only, never the popped
  element; a rejecting advice turns the removal into the word's error.
- **Flex adoption drops the tag** (`FlexDeepCopy` rebuilds via `NewFlexList`)
  — consistent with the decay rule; pinned by spec row.
- **`eq`/`cmp` of Uniq vs plain List**: pin the current cross-Parent
  semantics (`nodeFamily` is deliberately exact; the pair orders via
  `compareTypes`) with rows — comparison semantics do not change.
- **Wire**: user types have no FixedID, so the tag does not survive
  serialization — identical to scalar refines today; documented, no new work.
- **Rendering**: a Uniq prints as a plain list unless the user also installs
  `canon` (the wrapper delegates Format down the chain) — pinned.
- **Advice bodies observe outer defs** beyond the pushed param — the same
  exposure compare/canon/unify bodies already have; accepted precedent.
- **`def x:Uniq […]` runs make-advice** — the other construction path must
  enforce the same invariant.

## 7. Verification sketch

```bash
boru -e 'def Uniq (refine List) behave make/q (fn [[v:Uniq] [Uniq] [v unique]]) behave push/q (fn [[v:Uniq] [Uniq] [v unique]]) make Uniq [3 1 2 1]'
# → [3 1 2]
boru -e '… (make Uniq [1 2]) push 1'            # → [1 2]
boru -e '… typeof ((make Uniq [1 2]) push 1)'   # → Uniq
boru -e '… def f fn [[x:Uniq] [Uniq] [x push 9]] f (make Uniq [1 9])'  # nominal param/return pass
boru -e '… f [1 2]'                             # → ERROR: plain List is not a Uniq
```

Plus the behave spec files, the Stage-2 `RunCompiled` fallback Go test, a
no-panic row passing type literals through the new seams (repo discipline),
and the full pre-commit gate per stage. (The dedup word is `unique`,
`native_array.go:143`; `distinct` is the query-DSL clause.)

## 8. Future work (explicitly out of v1)

- **Flex-refine enforcement** — needs an advice model sound under aliasing
  (advise-before-commit, or a lock-normalize-release discipline on the
  in-place payload).
- **Around advice / before-validation** — if a use case demands rejecting
  *before* the base op runs (cheaper than build-then-raise).
- **Native compilation of advice-carrying dispatches** — an interpreter
  island threading the advice body, once the Stage-5 island machinery grows
  capture support (see `legacy/boru-bytecode-capture-threaded-islands.0.ignore`).
- **Wiring `Hasher`** — set-like dedup at O(n) instead of the O(n²) a naive
  `unique` body implies; also what a real `Set` type would want.
- **Weak references** — a value-identity/GC axis, unrelated to structural
  invariants; separate design if ever pursued.
