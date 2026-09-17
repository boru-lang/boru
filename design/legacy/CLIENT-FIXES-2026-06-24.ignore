# Client-library fixes — voxgig-boru trie / decision / bloom-filter

**Date:** 2026-06-24
**Upstream branch:** `claude/boru-client-issues-6b8new` (boru-lang/boru)
**Driven by:** the three client verification reports —
[`trie/BORU-MAIN-VERIFICATION.md`](https://github.com/voxgig-boru/trie/blob/main/BORU-MAIN-VERIFICATION.md),
[`decision/boru-main-verification-2026-06-23.md`](https://github.com/voxgig-boru/decision/blob/main/boru-main-verification-2026-06-23.md),
[`bloom-filter/boru-backend-report.md`](https://github.com/voxgig-boru/bloom-filter/blob/main/boru-backend-report.md).

This note tells each client project **what changed upstream, what is now
fixed, what remains, and how to re-pin.**

---

## Upstream fixes landed

| Commit | Fix | Clients it unblocks |
|---|---|---|
| `f247557` | **`None` / type-literal template interpolation.** `` `got ${x}` `` with a `None` (or `typeof`) value rendered `"String"` and corrupted `raise` codes (`raise_error` instead of `bad_input`). | bloom-filter §1 (the `make-validates-input` interpreter failure) |
| `f247557` | **`convert` return-type modeling.** `convert Float x` modeled its result as the bare `Float` *type literal*, so downstream `mul`/`div`/`add` raised a false `no_signature`. Now returns a *value* of the target type (like `make`). | bloom-filter §2, decision (`convert` in `decision.boru`) |
| `1b7b9ae` | **`OpInterp`.** Interpolated strings with runtime-computed holes (`` `got ${x}` `` for a fn param, `` `${1 add 2}` ``, `` `${typeof x}` ``) now compile to bytecode instead of refusing. | all three (smoke/print paths) |
| `a0604d7` | **`fold [push]` false positive.** `push`'s `[Any List]` overload declared no `Returns`, so a fold accumulator widened to `Any` and the next `push` failed `no_signature`. | decision (`no_signature: push`), trie |
| `a0604d7` | **`for` multi-value-body miscompile.** `for 2 ['e' 'f']` compiled to `[f f]` instead of `[e f e f]`; now islands faithfully. | (correctness, all) |
| `fc47452` | **fold body element carrier.** A fold over a `List<Any>` (e.g. `convert List <Array>`) typed its element as a *strict* `Any`, so a body word (`add`, `BinUtil.popcount`) failed. Now uses a dynamic carrier like `each`/`filter`. | bloom-filter smoke (the `popcount` fold) |
| *(gradual-Any)* | **explicitly-`Any` params and returns become gradual (dynamic) carriers.** A value of static type exactly `Any` (a `:Any` param, a `[Any]`-returning helper) is "type unknown", not "the Any root" — it now poly-matches a concrete slot instead of failing `no_signature`. This is the "unify Any with concrete params" wishlist item. | trie (node walkers — `get`/`find-kid`/`trie-insert`), decision |

All land with the full upstream suite green (`make fmt && make vet && make
lint && make test`), and a new verified all-words/structures corpus
(`lang/spec/corpus-*.tsv`, ~407 rows) that gates interpret + check + compile
across the whole language surface.

---

## Per-client status (after the fixes)

Measured from each repo root (so the relative `import "./*.boru"` resolves).

> **FINAL (this branch):** all six client modules — `radix`, `trie`, `tst`,
> `burst`, `decision`, `bloom` — and EVERY one of their test suites (unit /
> spec / prop / smoke) now `boru check` with **0 errors**. The per-suite tables
> below record the reduction history; the closing fixes are documented under
> "Polymorphic dynamic dispatch …" near the end. The sections below are kept as
> the historical record of how the count came down.

### bloom-filter — ✅ fully clean

| Suite | interpret | check | compile |
|---|---|---|---|
| `bloom_unit_test`  | ✅ | **0** | ✅ |
| `bloom_unit_spec`  | ✅ | **0** | ✅ |
| `bloom_prop_test`  | ✅ | **0** | ✅ |
| `bloom_prop_spec`  | ✅ | **0** | ✅ |
| `bloom_smoke_test` | ✅ | **0** | ✅ |

All five suites now interpret, check (zero errors), and compile cleanly. The
§1 interpreter regression and §2 `no_signature` false positives are gone.

**Action:** adopt this upstream and move the `test/divergence/` pin off
`c44d994` once the branch merges. No library changes needed.

### decision — ⚠️ 4 / 5 clean

| Suite | check |
|---|---|
| `decision_unit_test` / `_spec` | **0** |
| `decision_prop_test` / `_spec` | **0** |
| `decision_smoke_test` | 16 (residual, see below) |

The unit/spec/prop suites are clean (the `push` / `convert` fixes). The smoke
suite's residue is the **dynamic-dispatch + namespace class** (below).

### trie — ⚠️ interpret green; check sharply reduced

Interpreting is fully green (all 11 suites). The gradual-`Any` change cut the
check error counts sharply:

| Suite | before | after |
|---|---:|---:|
| `trie_prop_test`  | — | **0** |
| `trie_unit_test`  | 17 | **5** |
| `trie_unit_spec`  | 25 | **5** |
| `trie_prop_spec`  | — | 6 |
| `burst_unit_test` | 25 | **6** |
| `radix_unit_test` | 66 | **31** |
| `tst_unit_test`   | 70 | ~31 |
| `*_smoke_test`    | high | high |

The residue is two narrower cases the gradual-`Any` change doesn't reach
(emergent in the full transitive analysis, not reproducible in isolation): a
`set` over a dynamic node and a `do {…}` map body referencing a same-named
param (`kids`) — the trie report's "resolve `do{}` quotation params" item — plus
the namespace-exposed-word noise the smoke suites carry at top level.

---

## The big lever — explicitly-`Any` dispatch (now fixed)

The dominant class was **dispatch on an explicitly-`Any`-typed value**: a `:Any`
param or a `[Any]`-returning helper bound a *strict* `Any` carrier, and a word
needing a concrete receiver (`get`, `add`, a user walker like
`find-kid`/`trie-insert`) failed `no_signature` against it — even though it
dispatches fine at runtime.

```boru
def g fn [[v:Any] [Any] [v get "x"]] end   g {x:1}   # was: no_signature: get  →  now: clean
```

This is the trie report's wishlist item **“unify `Any` with concrete params”**.
It is now fixed (the gradual-`Any` change): an explicitly-`Any` param/return
binds a **dynamic (gradual)** carrier — the same treatment `each`/`filter`/`fold`
give untyped list elements — so dispatch poly-matches. The full upstream suite
(check accuracy, type soundness, differential, the new corpus) stays green.

## The remaining residue

Two narrower families are left, both deferred (they need their own focused
pass, and neither reproduces in isolation — they are emergent from the full
transitive analysis):

1. **`do {…}` quotation params and namespace-exposed words.** `undefined_word`
   on a `do {…}` map body that references a same-named param (trie's `kids`),
   and on a type/word only known through an export (decision's `DTable`,
   `eval-table`/`decide` left as `uncalled_function`). The static checker does
   not yet thread these.
2. **`set` over a freshly-dynamic node** inside a deeply nested walker
   expression — a downstream consequence of the gradual carriers that the
   poly-record path does not yet cover for the mutating `set`.

   **Root cause (diagnosed).** `set` has mixed return *arity* across its
   overloads: the in-place mutators (Array/Object/Store/Class) return **0**
   values; the value-returning twins (Map/List/Flex) return **1** (the updated
   node). Over a dynamic receiver (`(nd "kids" get)` → `Any`), the checker
   matches a 0-return overload and `carrierResults` synthesises **0** values.
   When the result is then *consumed* — trie's `((nd "kids" get) set ch child)
   … mk-node` — that missing value cascades into a false `undefined_word` on
   the consumer's bound param and a `no_signature` on the consuming call (and,
   transitively, the `kids`/`mk-node` errors in `trie.boru`). At runtime the
   receiver is a concrete `Map`, so `set` returns 1 value and the code is
   correct.

   **Why it is not a one-line fix.** The correct static arity is
   *runtime-receiver-dependent* (0 for an in-place container, 1 for a
   value-returning one) and `carrierResults` cannot know whether the result
   will be consumed. Modelling 1 value (or a 0-or-1 variadic) unconditionally
   removes the trie false positives but **breaks** the in-place-mutation
   compile coverage that depends on a 0-output `set` over a dynamic Store
   receiver (`context get __sys get fs set mem true` — pinned by
   `TestSetOverDynamicReceiverPolyCompiles`): a phantom residual then refuses
   the poly lowering. A sound fix needs runtime-arity-aware modelling (the
   value is consumable iff the runtime overload returns it), which is the
   deferred precision pass — not a static result-arity tweak.

## Checker `unused_def` on reference-exported words (landed)

The client check reports' largest *warning* category — the trie report's
wishlist item **#1, "trace `/r` reference-exports as usages"** — is now fixed.
A word bound into an export map by reference (`export "X" { make: impl/r }`,
the canonical public-API form) or as a bare value (`{ Color: Color }`) was
flagged `unused_def` precisely *because* it was public: the use-tracker counted
only dispatch/`Lookup` uses, not a reference resolution. The fix records the use
at the single resolution chokepoints:

- `ResolveRef` (`eng/go/core_ref.go`) — covers `name/r`, the `ref` word, and
  in-map reference values uniformly.
- The collecting export handler's `resolveModuleExport` and the top-level
  no-op `export` handler (`lang/go/native`) — a standalone module file
  (`boru check trie.boru`) reaches the no-op, which now records each export-map
  value as a use of its def in check mode.

Effect on the clients (modules + suites): `unused_def` warnings fell from **133
to 10** — `trie.boru` 31→0, `burst.boru` 32→0, `decision.boru` 14→3, `bloom.boru`
9→1. The fix is sound by construction (a genuinely unreferenced def is still
flagged — pinned by `lang/go/unused_def_export_test.go`'s negative half) and
leaves the checker's error-frontier ratchet untouched. The residue is body-
local defs reassigned/read inside a `for` body (`found`, `best-pri`, `midkids`)
plus a const-fold ordering quirk on certain 0-arg-fn export combinations —
both emergent (not reproducible in isolation), the same wall the families below
hit.

## Checker error-level false positives — why they stay (the Any frontier)

The error-level diagnostics (`no_signature`, `undefined_word`,
`uncalled_function`) on this generic, dynamically-dispatched code are **not** a
severity bug to demote away. The dominant `no_signature … assuming best-fit
candidate for analysis` is the deliberately-tracked **Any frontier**
(`test/go/langspec/check_accuracy_test.go::pinnedAnyFrontierRows`): a dispatch
whose operand is statically `Any` (a value pulled from `get`, an untyped node
walked recursively). The maintained methodology *shrinks* that frontier by
improving **precision** (flow-narrowing `get`, declaring core-word `Returns`,
unifying `Any` with concrete params — the gradual-`Any` change above was one
such step) while keeping it visible at **error**, not by demoting the severity
(which would hide it). The remaining false positives are emergent from the full
transitive analysis — they do not reproduce in isolation — so they need that
focused precision pass, exactly as the two residue families above. Demoting
severity was considered and rejected on this basis.

### Call-time recursive re-analysis suppression (landed)

The recursive slice of the cascade is now fixed. When a fn body is re-analysed
under a self-call's narrowed arg shape (a different `AnalyseFnBody` memo key, so
it does not bail), its error-level body diagnostics (`no_signature`,
`undefined_word`, `uncalled_function`, `branch_error`) are **suppressed** — the
outer, non-recursive analysis of the same body tokens already reports any real
defect, while the narrowed re-run can spuriously fail dispatch. Tracked per fn
NAME (`CheckState.FnNameInflight`), so only genuine same-fn recursion is
suppressed, never a nested helper call (a real error in a helper, or in a
recursive fn's own first analysis, is still reported — pinned by the two
negative halves of `lang/go/recursive_reanalysis_test.go`).

Effect on the clients (`boru check`): `radix.boru` 53→23, `tst.boru` 62→36,
`burst.aml` 30→4 errors. All ratchets (`pinnedAnyFrontierRows`,
`pinnedTypeSoundnessViolations`), the differential + property gates, and the
full suite stay green.

### Dynamic carrier vs the Node family (landed) — `trie.boru` now 0 errors

The non-recursive slice was a **matching** bug, now fixed. A gradual (dynamic)
carrier matches a slot unless its bound is *provably disjoint*, and the
disjointness probe reused `tand` — but `tand(List, Node)` returns `Never` even
though `List ⊑ Node` (a container-family meet bug). So a value whose declared
return narrowed to a dynamic `List`/`Map` carrier (a fn result derived from a
dynamic arg) failed to match a `Node`-typed `get`/`set` receiver — `get`'s
List/Map access goes through its `[key, Node]` overloads (there is no `List`
receiver sig), so the dynamic List/Map receiver was rejected. This drove
`trie.boru`'s `fuzzy-go`/`kid-items`/`get`/`build-row` cluster.

`sigTypeMatches` now checks conformance in **both directions** (`X ⊑ t` — the
value IS a t; or `t ⊑ X` — it MIGHT be, the gradual optimism) before the `tand`
probe, which still settles the cross-family disjoint cases (`dynamic(Integer)`
vs `String`). Looser, never tighter — sound by the dynamic-modality discipline,
pinned by `eng/go/dynamic_carrier_match_test.go` (with disjoint negatives) and
`lang/go/dynamic_container_get_test.go`. All accuracy ratchets, the differential
+ property gates, and the full suite stay green.

Effect: **`trie.boru` 9→0**, `radix.boru` 23→20, `tst.boru` 36→26, `burst.boru`
4→2. The cumulative client error-level reduction across the six modules is now
**≈177 → ≈60**.

### Options / Record value vs a Map/Node slot (landed) — `bloom.boru` 10→1

`Options` and `Record` are structural keyword/field-map types but are lattice-
rooted under `Ideal`, so an `opts:Options` carrier does **not** `ConformsTo`
`Node` — `get`/`size` over it (whose List/Map access uses the `Node` receiver)
failed `no_signature` even though the value is a map and the suite runs. `bloom`'s
`opts "n" get` / `opts "p" get` were the case (and the `convert`/`mul`/`div`/
`log` chain that consumed those results cascaded from it). `sigTypeMatches` now
matches an Options/Record-conforming value against any Map/Node-family slot,
aligning the check-mode carrier with the runtime value (whose payload Parent is
`TMap`). Pinned by `lang/go/dynamic_container_get_test.go`.

Effect: **`bloom.boru` 10→1**.

### Body params narrowed to their declared type (landed) — 4/6 modules clean

A fn body was analysed against the (gradual) ARG shapes a call threaded in, not
its DECLARED param types. A mutual-recursion cycle (decision's `eval-pred-all`
↔ `eval-pred`) passes a `get`-result `dynamic(Any)` into a `children:List`
param; the body then saw `children` as `dynamic(Any)`, so `each` over it matched
the **map-each** overload (`[TList, TMap] → [TMap]`) instead of list-each (→
List), and the downstream `all`/`any` (`[TList]`) failed `no_signature` against
the spurious `Map`. `buildFnBodyReturnsFn` now narrows each gradual arg whose
bound is strictly broader than its declared param type to a dynamic carrier of
that param type before body analysis (sound — the arg already passed the param
match, so a disjoint arg never reaches here; precision is preserved for
concrete/conforming args). Pinned by `lang/go/param_narrowing_test.go`.

Effect: **`decision.boru` 2→0, `bloom.boru` 1→0, `burst.boru` 2→0**, `radix.boru`
20→11, `tst.boru` 26→15. **Four of the six client modules now check fully clean**
(trie, burst, bloom, decision); cumulative error-level across all six is now
**≈177 → ≈26**.

### Branch merges stay gradual (landed) — `tst.boru` 15→0, 5/6 clean

`JoinCarriers` (the `if`/loop/case arm merge) dropped the gradual modality: a
merge of a concrete arm and a dynamic arm produced a STRICT disjunct, which a
later concrete-typed consumer rejected. The node-rebuild walkers' "default-or-
self" rebind `def nd (if (nd eq none) [<fresh node>] [nd])` over a `nd:Any`
param merged a concrete `Map` (the constructor) with the gradual `nd` into a
strict `Disjunct(None|Map|…)`, so the downstream `nd:Map` rebuild helper
(`with-end-val`, `set-edge`) failed `no_signature`. `JoinCarriers` now keeps the
result dynamic when either arm is dynamic (the same gradual contagion a dynamic
operand already spreads through a dispatch result). Pinned by
`lang/go/join_carriers_dynamic_test.go` (with a strict-union negative).

Effect: **`tst.boru` 15→0**. **Five of the six client modules now check fully
clean** (trie, tst, burst, bloom, decision); cumulative error-level across all
six is now **≈177 → ≈11** (only `radix.boru` remains).

### Fold accumulator inherits the seed's gradual modality (landed) — radix 11→4

`foldAccCarrier` built the accumulator from the seed's TYPE but dropped the
gradual flag: a `(do {…})` seed (whose result is dynamic Any) fell to the
default `NewCarrier(Any)` — a STRICT Any accumulator — so every `acc … get`/
`add` in the body errored. It now preserves the seed's dynamic modality (radix's
`common-prefix-len`, seeded with `(do {n: …, ok: …})`). Pinned by
`lang/go/fold_dynamic_seed_test.go`.

Effect: **`radix.boru` 11→4**. Cumulative error-level across the six modules is
now **≈177 → ≈4** (only radix's node splitter remains).

### Mixed-arity refinement over a dynamic receiver (landed) — radix 4→3

`dynamicReachableReturns` BAILED whenever a word had any overload of a different
arity, disabling result-disjunct refinement for every mixed-arity word. `slice`
(1/2/3-arg) over a gradual receiver then committed to the single matched
overload — `dynamic(List)` for a String-or-List value — so a String consumer of
the slice result failed no_signature. It now SKIPS different-arity overloads and
unions the reachable same-arity returns into a `String|List` disjunct (still
bails only on a reachable multi-/ReturnsFn overload it cannot reduce). Pinned by
`lang/go/slice_dynamic_test.go` (with an Integer negative). Clears radix's
edge-split key (`{} set ((lrest slice 0 1)) …`, where the sliced label is a
get-result of unknown type) and its `midkids` cascade.

### Polymorphic dynamic dispatch — receiver narrowing, return widening, param re-bind, builder merge (landed) — radix/tst node walkers 0, ALL six modules clean

The last residue — `radix.boru`'s recursive edge splitter and `tst.boru`'s
node-rebuild walkers reached through their `Map`/`set`/`make` surface — closed
with four interlocking gradual-dispatch fixes. All validated against the pinned
ratchets (false positives **114→105**, type-soundness held at **12**), the
differential / property / `-race` / `borudebug` bytecode gates (`make
verify-bytecode`), and the full suite.

1. **Polymorphic-slot use does not narrow a dynamic binding.**
   `narrowDynamicUses` tightened a dynamic carrier to whichever overload's slot
   matched — but for a word whose arg position varies across overloads
   (`slice`'s data arg is `String` in one 3-arg form, `List` in the other), the
   value could legitimately dispatch to either at run time, so narrowing to one
   is unsound. `slotIsPolymorphic` now skips the narrowing when a second
   equally-reachable overload constrains that position differently. Pinned by the
   existing `lang/go/slice_dynamic_test.go`; this fixed radix's `lab` being
   wrongly narrowed to `List` after the first `slice`, which then mistyped
   `(newlabel slice 0 1)` as a `List` key and failed `set`'s String-key Map sig.

2. **A binary word over two FULLY-UNKNOWN operands widens to the gradual union
   incl. Any.** `dynamicReachableReturns` bailed (no refinement) when a reachable
   overload had a `ReturnsFn` (e.g. `add`'s numeric sig), so `add` of two
   dynamic-`Any` operands committed to whichever overload matched first
   (`Instant`, a temporal sig) — disjoint from the `String` consumer it fed
   (radix-delete's merged label `(lab (gpair get 0) add)` → `set-edge`'s
   `newlabel:String`). When ALL operands are dynamic-`Any`, the unknown-return
   overload now folds in as the `Any` alternative, making the result gradual.
   Gated to all-unknown so a partially-concrete call keeps its precise union.
   Pinned by `lang/go/client_node_builder_test.go::TestAddTwoUnknownOperandsGradual`.

3. **A dynamic arg disjoint from a declared concrete param is analysed at the
   param's type.** `narrowArgsToParams` only re-bound a gradual arg whose bound
   was strictly BROADER than the param; a sibling/disjoint bound (a dynamic
   `List`/node-field fed to `mk-tnode`'s `ch:String`) passed through unchanged,
   so the body's typed operations failed. The param annotation is the
   authoritative contract for body analysis, so a dynamic arg is now re-bound to
   `dynamic(paramType)` whenever its bound is not already within the param. Pinned
   by `lang/go/param_narrowing_test.go::TestParamNarrowingDisjointDynamicArg`.

4. **A None-vs-value branch merge is gradual.** The optional/builder sentinel
   `if (nd eq none) [build-node] [nd]` merges a built node with the still-`none`
   receiver. Reached through a gradual `:Any` param the merge was already
   dynamic, but a DIRECT call passing a concrete `none` (tst's `TstMap.set` on an
   empty map → `none key val tst-insert`) produced a STRICT `Disjunct(None|Map)`
   whose None alternative made every node-rebuild `get` fail. `JoinCarrierStacks`
   now marks a both-arms-real None-vs-value position dynamic (a PADDED/variadic
   None — `if cond [98]` — keeps its precise shape). Pinned by
   `lang/go/client_node_builder_test.go::{TestNoneBuilderMergeGradual,TestVariadicIfStaysPrecise}`.

Plus a **panic fix** surfaced by these: a `none` fold-seed promoted to a dynamic
carrier reaches the fn-body memo keying with a nil `Parent` (a None root node IS
its own lattice node). `FnAnalysisKey` and the generalised-args build now guard
the nil-Parent root via `carrierTypeName`; pinned by
`eng/go/fn_analysis_key_test.go::TestFnAnalysisKeyNilParentArg` and
`lang/go/client_node_builder_test.go::TestNoneDynamicCarrierNoPanic`.

**Result: all six client modules (`radix`, `trie`, `tst`, `burst`, `decision`,
`bloom`) AND every one of their test suites check with 0 errors.** The
deliberately-tracked Any-frontier rose 255→303 (informational only — `t.Logf`,
not a gate) as the cost of the added optimism; the GATED accuracy ratchet
improved over the same corpus.

### Client-side options until that lands

1. **Keep the current pins** (`c44d994` for the bytecode-capable reference)
   for `trie` and the `decision` smoke suite — interpreting is fully green
   there while the check residue is worked off.
2. **Adopt the branch for the now-clean suites.** bloom-filter (all suites)
   and decision's unit/spec/prop suites check clean and can drop their
   `--soft` / `continue-on-error` gates.
3. Where a library function takes an untyped node, a **concrete annotation**
   (`nd:Map` instead of `nd:Any`) checks clean today (see the `find-kid`
   contrast above) — narrowing the public signatures where the shape is
   actually known removes the false positives without waiting on upstream.

---

## Compilation (`--force-compile`) note

`OpInterp` (`1b7b9ae`) removed the interpolated-string refusal, so suites that
previously stopped there now advance. The remaining `--force-compile` refusals
are the test framework's **code-body words** (`each`, `test-test`, `do` at
“Stage 2”) — the tracked emitter-coverage work (decision report §6). Every one
of those refusals is a **defect**: valid code the emitter cannot yet lower,
owed a fix and tracked to closure, never a sanctioned outcome. Under
`--compile` the runtime **silently** re-runs a refused program on the
interpreter, so every suite still produces correct output — and the silence is
the worst of it: a failure to compile that hides itself. The interpreter is not
a fallback for the compiler and is not allowed to become one; that re-run is
scaffolding absorbing these open defects, not a sanction for any of them.

### Residual emitter fix — raise-arm divergence (landed)

A focused emitter false-refusal in this set is now fixed: a fn whose `if`-chain
bottoms out in `raise` (the **`apply-op`** shape — an op-dispatch chain whose
default branch raises) was wrongly flagged **variadic-returning**. The variadic
accounting (`branchVariadicResult` / `eventDivergesDeep`, `eng/go/emit.go`)
counted the `raise` arm as a 0-value merge contributor, so the `if` looked like
a runtime-variable 0-or-1 result; every site consuming the Boolean as a
fixed-arity operand (`Assert.equal`, `print`, `add`) then refused with
“consumes loop results”. A diverging arm never reaches the merge, so the
surviving arm's value is unconditional — `raise` now joins `break`/`continue`/
tail-call in the divergence checks, matching the shallow `fragDiverges`. The
change only **relaxes** an over-marking (a genuine `if c [n] []` 0-or-1 still
refuses fixed-arity consumption — itself an open defect, just not one this fix
reaches), so it is coverage-only and sound by the differential + property gates.

Effect on the clients: the three `apply-op-*` cases in `decision_unit_test`
now force-compile. The remaining suite refusals are unchanged in kind — `each`
over a `var`-block body (`var` splices onto the tape, an unlowered code-body
case), `do {…}` map bodies and namespace-exposed words (the deferred checker
family — `check diagnostics`), and `test-check-prop` (the PBT framework). Each
is an open defect owed a fix. They still produce correct output under
`--compile` only because the runtime silently re-runs them on the interpreter —
containment for those defects, not a reason they may stay refused.

Regression guards: `lang/spec/corpus-structures.tsv` (`classify`, an
op-dispatch chain whose default branch raises, result consumed by `add`) and
`lang/go/bytecode_raisediverge_test.go` (positive + the negative genuine-
variadic that must still refuse).

---

## Re-pin checklist

```bash
# Build the branch tip
REF=<branch-tip-sha>
curl -fsSL "https://codeload.github.com/boru-lang/boru/tar.gz/$REF" | tar -xz -C /tmp/boru --strip-components=1
( cd /tmp/boru/cmd/go && GOFLAGS=-mod=mod go build -o /tmp/boru-bin ./boru )

# From each client repo ROOT (so ./*.boru imports resolve):
/tmp/boru-bin test/<suite>.boru            # interpret
/tmp/boru-bin check test/<suite>.boru      # check

# compile: a refusal here is SILENTLY re-run on the interpreter, so a green
# line is not proof it compiled. Use --force-compile to see the refusals;
# each one is an open defect, not a pass.
/tmp/boru-bin --compile test/<suite>.boru
```
