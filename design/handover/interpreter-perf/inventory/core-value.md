# core/go — value-side copy inventory (read-only review)

Scope: `core/go` non-test files assigned to this reviewer — value.go, clone.go, payload.go, core_flex.go, core_helpers.go, sugar.go, macro_expand.go, core_make.go, convert_ideal.go, unify*.go (13 files), canon.go, carrier_*.go (4), guard_*.go (2), deadsig.go, record_typed_def.go, compare*.go (4), typetable.go, types.go, type_run_install.go, typebehavior.go, typed_bind.go, core_type.go. Three adjacent files were read because the named patterns are DEFINED or driven there and the inventory is incomplete without them: util.go (`WithPos`, `WithPosAt`, `ReparentValue`, `CanonicalType`), resolve.go (`ResolveWordsDeep`, the prepass every `Unify` runs over both operands), equal.go (`ValuesEqual`, the `==`-on-Parent gate). engine.go / sealed.go / loop.go / tape.go / registry.go / fork.go / fn_capture.go / deftable.go / invoke.go / engine_pool.go / compile_sandbox.go are the other reviewer's; where a value-side site exists only because of a tape-side mutation, the table says so and names the file.

Line numbers are from the current tree (commit as checked out on 2026-10-09). "Policy" below means the maintainer's proposal: one `Value` instance per value, mutated in situ by any stage, never copied.

## 1. Summary

### 1.1 Counts of explicit copy sites by kind (in-scope files)

| kind | count | where the mass is |
| --- | --- | --- |
| A. Value header copied so the copy can be changed (`x := v; x.F = …`, value-receiver "with" helpers, `WithPos` on an existing value) — copy of an EXISTING value, not a freshly minted one | 24 | util.go `WithPos`/`WithPosAt`/`ReparentValue`; value.go `WithFreshFnIdentity`, `SetAtomReferent`, `StripAscribed`, `String` (`inner := v`); clone.go `withPayload`; core_helpers.go `arg.Quoted = true` ×2, `RetagFlexElem`; macro_expand.go `q.Quoted = true`; core_make.go `FreshenDefault`; unify.go `MarkPredicateFn`; unify_fold/unify_list/unify_map carrier `SetElemConstraint` ×3; resolve.go disjunct re-simplify; carrier_join.go `out := a` in `JoinCarriersInner`; carrier_new.go `NewDynamicCarrierValue`, `ReturnsIdentity`; guard_narrow.go then-/else-arm narrowing (×2) |
| A'. field writes on a FRESHLY constructed value (construction, not copy; listed for completeness, not counted above) | ~22 | core_flex.go `WithPos(NewFlexMap(..), v)` etc. ×9, sugar.go ×5, carrier_join.go `JoinCarriers` flag flips, carrier_new.go, carrier_spread.go, unify_list/unify_map `out.SetElemConstraint` on the rebuilt container |
| B. `ReparentValue(` call sites | 5 (+ definition) | core_make.go L777, L1087; unify_list.go L147; typed_bind.go L44, L70 |
| C. `CloneValue` / `CloneValueKeeping` | 0 calls inside core outside clone.go; 9 real call sites repo-wide (classified in §1.3) | — |
| D. `[]Value` slice copies (`make`+`copy`, `append([]Value(nil), …)`, `ReadList.Slice()`, element-by-element rebuilds) | 41 | fn-body skeleton/expansion (core_helpers L425, L484-486, L505-506, L537-538, L595), carrier_body L147-148 (double copy), macro_expand L180-181 and six `Slice()` reads, resolve.go three rebuilds, unify_list L107-108/L159, core_make ×6, convert_ideal ×4, `ReadList.Slice` itself |
| E. `OrderedMap` copies (re-`Set` loops, `cloneOrderedMap`, `FreshenDefault`, `FlexDeepCopy`/`NodeDeepCopy`) | 26 | value.go `AllFields` ×3 + `FlatInstanceFields` + `AsMap` typed-entries arm; clone.go; core_flex.go ×4; convert_ideal.go ×3; core_make.go ×5; macro_expand.go ×2; resolve.go; unify_fold/map/options ×6; core_helpers `omittedDefaultValue` |
| F. payload struct copied out of `Value.Data`, modified, re-boxed (or re-minted with `NewValueRaw(parent, modified)`) | 21 | value.go `WithFreshFnIdentity`, `SetAtomReferent`, `NewFunction` (Signatures clone), `NewClassType`/`NewResourceType`; core_helpers `installDef` (`fnDef.Name`), `installFnDef` (`entry := fnDef`), `compileFnSigs` (×3 struct copies per sig), `WrapperUnderName`; core_type.go `InstallTypeBody` ×5; core_make `FreshenDefault` ×2; unify.go `MarkPredicateFn`; resolve.go; record_typed_def.go `wrapped := *sig`; typed_bind.go `s := *spec`; clone.go TableData/ErrorInfo |
| G. type-literal duality: `NewTypeLiteral(t)` returns `*t` | 1 definition + 27 call sites in scope; 24 `&v` orphan-pointer sites; 12 `CanonicalType` repair sites | see §1.2 risk 1 and the per-file tables |
| H. `Value{}` / nil-Parent used as a semantic sentinel (excluding the ubiquitous `return Value{}, err`) | 12 | value.go `GetOk`, `StoreInstanceInfo.Get`, `IsUnboundSlot`, `TypeBody`/`ElemConstraint`/`AtomReferent` "none" returns, `ClassTypeInfo.BinaryLayout` "zero Value otherwise"; payload.go `StoreShapeInfo.Vals` "Parent == nil means nothing recorded"; core_make.go `MakeWithOpts` `Parent.Equal(nil)` as "unassigned"; core_helpers L243 `NoteSpecFnDef(.., Value{}, ..)`; carrier_new.go L26 ID-less `Value{Parent: TAny, Carrier: true}`; depscalar.go `Value{Parent: TNone}` |
| I. struct fields / maps holding `Value` by value (the implicit flows a `*Value` conversion would hit) | ~45 fields | listed per file; the heaviest are the payload structs in value.go/payload.go (`ChildTypeInfo`, `DisjunctInfo`, `CapturedBinding`, `StoreInstanceInfo.Data`, `Loop.*`, `MarkInfo.Body`, `ReachInfo`, `InterpPart.Expr`, `ClosurePayload.Captures`, `OrderedMap.vals`) and the Behavior-held declaration copies in unify_*.go |

Value is compared with Go `==` nowhere in core (it is not a comparable type: `ListPayload` holds a slice; compare.go L473-479 says so explicitly). Identity today is: ID string (type nodes, check-mode provenance), `*fnIdent` pointer (functions), backing-array / `*OrderedMap` / payload-pointer identity (containers, compare.go `sameContainer`), `a.Parent == b.Parent` pointer gates in equal.go L78 and compare.go L278. No `map[Value]`; the one Value-keyed structure is `DeqIndex` (by key string).

### 1.2 Top 5 risks of in-place mutation in these files

1. **Type literals ARE node copies, and the program handles them as values.** `NewTypeLiteral(t)` (value.go L2803-2808) returns `*t`; `typeof` (core_type.go L98), `pathof` (L76), the None/Absent "fill" literals (core_make.go L89-91, unify_map.go L138/L330, unify_options.go L93, core_helpers.go L1342/L1362), `runSigPattern` (type_run_install.go L198) all hand a NODE COPY to user data, maps, sig patterns and the tape. Today the copy absorbs every header write: `WithPos(NewTypeLiteral(TType), src)` (sugar.go L135), `Carrier`/`Dynamic` flips (carrier_join.go L111-112/L448-449 after `ValueCarrier`, carrier_new.go L140-151, guard_narrow.go L248), the engine's position stamping on tape cells (engine.go, other reviewer). Under the policy the one instance IS `TType`/`TNone`/`TInteger` in `TypeTable.byID`, so the first `WithPos` or flag flip rewrites the lattice node for every later reader. The policy cannot hold on type literals without separating "the node" from "an occurrence of the node" (a reference cell that carries pos/flags), or without a hard immutability rule on bare nodes plus moving all occurrence metadata off `Value`. The upside is equally large: the whole orphan-`*Type` class (`&v` at value.go L2818/L2845, carrier_new.go L48-49, guard_narrow.go L125-126, core_make.go L752/L1066, 24 sites) and its repair helper `CanonicalType` (12 sites) disappear, and `Type.Equal` (types.go L322-350, whose ID-string compare the file itself measured at ~7% of interpreter CPU) collapses to pointer equality. A latent imprecision caused by the copies is in carrier_join.go L28-42: `CommonAncestorType` keys `seen` by `*Type` POINTER, so the orphans `TypeNodeOf` produces at L193-195 never match the canonical chain and the cap-widening can over-widen (an Integer/Pos literal pair widens to Number instead of Integer).
2. **Shared program tokens vs tape-time mutation.** Every token sequence that is RE-RUN copies itself before running because the engine mutates tape cells (ReturnCheck/fn-value pos stamping — the comment at core_helpers.go L408-412 says the per-call copy "is mandatory"; `Quoted`, `FailedDispatch`, `ReachGroup`, `Undefined`, `Applied` flags are header writes): the fn-body skeleton (core_helpers.go L425, L505-506) and non-leaf expansion (L595), the macro expansion cache (macro_expand.go L37-49 re-splices one cached `[]Value` per application) and template body (L180-182), carrier-body analysis runs (carrier_body.go L147-148, re-run per fixpoint round), `ResolveFieldType`'s sub-run over a schema literal (core_make.go L1328-1337), the synthesized optional-param bodies that embed a default VALUE token (core_helpers.go L1464), `MarkInfo.Body`/`Loop.Body` replay (value.go L1592, L1671). With one instance per token, a call stamps the call site onto the function's own body marker, `ForkConcurrent` engines (which share the handler and the skeleton) race on it, and a loop's second iteration sees the first's flags. This is the largest hazard and it is jointly owned with the tape/engine reviewer: the only replacement is to move every tape-time write off `Value` into tape-cell metadata, after which sharing token pointers is safe.
3. **Dispatch-scoped header state on shared values.** A second header over one payload is how a binding or param differs from the caller's value in a flag/tag/Parent without touching it: `arg.Quoted = true` at param bind (core_helpers.go L500, L561) and macro-operand bind (macro_expand.go L172 — the same form must exist quoted for the scope and unquoted for splicing, L170 vs L171-173); `StripAscribed` at every arg-delivery boundary (value.go L2267-2273) paired with `as`'s attach; `RetagFlexElem` (core_helpers.go L670-682: "v's header with the tag set — the FlexListData pointer stays shared"); the check-mode carrier tags (unify_fold.go L111-112, unify_list.go L154-155, unify_map.go L205-207); `ReparentValue` at the five typed-bind/make/element sites. In place, the caller's binding, the container element, the pooled const or the def body acquires the flag: observable through canon/print (`(quote …)`, canon.go L225-227), `typeof`/`is` (reparent), typed-container write enforcement (elem tag) and `eq`. Replacement needs per-binding attributes on `DefEntry`, a param-binding wrapper, or genuine re-minting from parts (`NewValueRaw(parent, v.Data)` + flags), which the policy permits but which is exactly the "re-mint from an existing value" pattern.
4. **Check-mode identity is an ID string chosen on a copy.** Guard narrowing pushes a scoped SHADOW carrier that keeps the source binding's ID (guard_narrow.go L229-230, L245-250, L400) and pops it after the arm; `joinBranchDef` keeps the ID only when both arms narrowed (carrier_join.go L218-224) and otherwise `JoinCarriersInner` MINTS a distinct one from `out := a` (L145-153, with the comment explaining why reusing a's identity collided with a's binding); `ReturnsIdentity` re-mints IDs for duplicated outputs on a struct copy (carrier_new.go L182-184: "the ID write below is local to v"); `HasContainerIdentity`'s doc (compare.go L445-450) states the check pass clones on EVERY container read so operand provenance gets fresh IDs (native_storage.go L1449/L1586/L1595/L1627). Under the policy a then-arm would have to narrow the live binding in place and un-narrow it on exit (fragile across Kleene rounds and sandbox rollbacks), and every "fresh identity" site becomes a real construction; the emitter's `producedBy` maps keyed by ID would need the pointer as the key instead. This is tractable but touches the compiler's provenance model wholesale.
5. **Declarations are triplicated by copy, and some copies are mutated.** A type's content is stored three times by value: `typeMeta.Body` (heap copy made by `SetTypeBody`, value.go L2125-2128), the `DefEntry` body (`installTypeBinding`, core_type.go L346-351), and the Behavior-held copy (`BindingBodyUnifier.body`, `NegationUnifier.inner`, `PredicateUnifier.constraint`, `DepScalarUnifier.depInfo`, `DisjunctUnifier.Alternatives` — the last SHARES the slice, unify_disjunct.go L102-108); each `make` of a class instance takes `TypeRef: &objType` (core_make.go L261, L310) — a fresh heap `ClassTypeInfo` per instance with its own `cachedAllFields`. Collapsing these to one instance is a clear win, but only if the declaration is immutable; today `InstallTypeBody` (core_type.go L420-427, L440-447, L465-474, L480-488) and `installDef` (core_helpers.go L276-296) rewrite `info.Name`/`info.Type` on a payload copy and re-mint, and `installDef`/`installFnDef` rename the fn payload copy (core_helpers.go L98, L760-763). In place, `def c (class {…})  def Foo c` renames c's own value to `Class/Foo`, and `def a (f/v)` renames f's payload to `a` (visible in `print f/v`; canon deliberately excludes Name). The fix is to move the binding name off the payload onto the `DefEntry` — which NUR031's identity work already half did by making `eq`/canon name-independent.

### 1.3 CloneValue / CloneValueKeeping — every caller in the repo (non-test)

| caller | class |
| --- | --- |
| core/go/clone.go L42-44 / L57-60 (definitions); no call inside core outside this file (compare.go L445 is a comment) | — |
| lang/go/native/clone.go L19 `cloneHandler` → `CloneValue(args[0])` | **user-facing semantics** — the `clone` word; `cloneReturnsFn` L22-40 types the result as a fresh carrier so `(clone p) eq p` is false |
| lang/go/native/native_process.go L335 `target.Send(CloneValue(msg))` | **defensive copy / isolation** at the process boundary ("the receiver can never observe the sender mutating a shared container"; stateful containers are refused before this) |
| lang/go/native/native_storage.go L1449 `NewDynamicCarrierValue(CloneValue(child))` | **snapshot for identity** (check mode: a fresh ID for a disjunct child read) |
| lang/go/native/native_storage.go L1586, L1595 (`CloneValue(val)` on a fn-valued field read) | **snapshot for identity** (check mode; "cloned for a fresh ID — the stored value's ID is shared with the map field and would collide in operand-provenance tracking") |
| lang/go/native/native_storage.go L1627 (`CloneValue(val)` on a concrete list/map field read) | **snapshot for identity** (same reason; plus the concrete fold) |
| check/go/check_fnbody.go L646 `out[i] = core.CloneValue(bv)` | **snapshot** — plain-check surfaces a returned closure as a concrete fn value with its own identity |
| eng/go/vm.go L2300 `WithFreshFnIdentity(CloneValueKeeping(src.Val, nil))` | **compiled-fn-unit freshening** (statement-island restart prefix, `RestartConst` with `Fresh`) |
| eng/go/vm.go L4634 `seatConstLocal` | **compiled-fn-unit freshening** (one fresh instance per call for a multi-read body literal, `OpPushConstFreshLocal`) |
| eng/go/vm.go L4769 `OpPushConstFresh` | **compiled-fn-unit freshening** (per push; `Program.ConstKeep` names the embedded enclosing-binding members the spine clone must share) |
| lang/go/native/aliases.go L512, basic/go/aliases.go L507 | re-exports, not calls |
| compiler/go/bytecode.go L375/L393/L1886, emit.go L1571/L4136/L16119 | comments documenting the ConstKeep contract, not calls |

What `cloner` preserves on the header (clone.go L74-78): everything except `Data` and `ID` — Parent, Quoted, Eval, Carrier, Dynamic, pos, elem, asc, modns, dynFrom, tmeta. Immutable payloads (scalars, times, every type body, `FnDefInfo` — so a cloned fn keeps its `*fnIdent` and stays `eq`), markers are shared; `ExtensionPayload` clones only through `DeepCloner`. The VM layers `WithFreshFnIdentity` on top precisely because `CloneValue` keeps fn identity.

### 1.4 Copies that exist purely for performance (or are pure cost)

- `ResolveWordsDeep`/`ResolveWordsDeepR` (resolve.go L75-155) rebuilds BOTH operands of every `Unify`/`UnifyR`/`unifyWithin` call: a concrete list or map operand is re-sliced and re-mapped even when nothing resolves (only the disjunct arm has a `changed` short-circuit, L105-107). Called from unify.go L39-40, L86-87, L100-101.
- `ReadList.Slice()` (value.go L51-55) copies for consumers that only read: canon.go L223, L790-791; value.go L4537; compare_types.go L113; guard_narrow.go L60; core_make.go L133, L820, L839, L1329; unify_list.go L107-108 (then `unifyZip` copies again) and L159; carrier_body.go L148 copies the copy (`copy(tokens, elems.Slice())`).
- `FnUndefUnifier` mints a transient `FnUndef` Value per `Match`/`Unify` (unify_fnundef_named.go L64, L100) only to pass specs to `fnUndefMatchesFnDefR`.
- `snapshotPredicateState` clones the `TypeTable` maps per `RunPredicate` (util.go L391-409 → typetable.go L629-660).
- `Type.Equal` ID-string compare and the tmeta/In fast paths exist only because literals are copies (types.go L306-350).
- Every `TypeBehavior` method, every `Is*/As*` free function and every unify handler takes `Value` by value (104 bytes; value.go L1912-1917 cites ~15% duffcopy); every `v.Data.(FnDefInfo)` assertion copies the ~200-byte payload struct (value.go L1802-1815, L4481-4483; unify.go L232-243, L139-141; compare.go L336-341).
- Defensive `append([]Value(nil), …)` in convert_ideal.go L227, L239, L257, L292 and the `VisibleEntries` snapshot per `storeDeepEqual` (compare.go L947).
- `DeqIndex.vals` (compare_deqkey.go) and `UnifyError.A/B` (unify_error.go L18-22) hold value copies for diagnostics/indexing.

### 1.5 Two structural facts the refactor plan should start from

- The header's pointer facets (`tmeta`, `pos`, `dynFrom`, `elem`, `asc`, `modns`; value.go L1918-1975) already split into two groups: `tmeta` is SHARED by every copy and mutated in place (`SetName`/`SetBehavior`/`SetTypeBody`/`SetOwner`, `labelIntervals`, `forwardType` in type_run_install.go L93-100, the `init()` Behavior installs) — the design the policy wants, one level down. The other five are COPY-LOCAL: `SetPos`, `SetDynFrom`, `SetElemConstraint`, `SetAscribed`, `WithModuleNS` replace the pointer on the copy only. Those five setters are where the semantics change under the policy.
- Container identity for `eq` is NOT the header: a plain List is identified by its backing array (compare.go L478), a Map by `*OrderedMap` (L467), Xml by `Attr` pointer + `Cren` array (L503-513), flex/weak nodes and Stores by payload pointer, a fn by `*fnIdent`, a type node by ID. So the codebase already treats "the Value header" as a disposable view and puts identity in the payload. A `*Value` policy could make the header the identity, but then `dup`/binding reads must hand out the same pointer (they would) and every "fresh identity" site in §1.2.4 must mint instead of copying.

## 2. Per-file inventory

Column key — kind: `explicit-copy` (a Value, payload struct, Value slice or Value-bearing map copied so the copy can differ or be stored separately) / `by-value-flow` (a place a `*Value` conversion would hit: a field holding Value by value, a sentinel, an `&v` orphan, a snapshot that relies on independence) / `construction` (listed only where it looks like a copy but mints a fresh value). load-bearing: yes = the original must stay unchanged because something else shares it; no = in-place mutation would behave identically; unsure = depends on callers outside this module or on a tape-side rule.

### value.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 41 | `ReadList.GetOk` returns `Value{}` on miss | by-value-flow (zero sentinel) | no | — | nil `*Value` |
| 51-55 | `ReadList.Slice()` — `make`+`copy` of the element slice | explicit-copy (slice) | no for every in-scope caller (all read-only: canon L223/L790, IsTypeValue L4537, compare_types L113, guard_narrow L60, core_make L133/L820/L839/L1329, unify_list L107-108/L159, macro_expand L196/L249/L362/L406/L501, resolve L81); carrier_body L148 copies it again before running it | the list payload's backing array (every Value sharing the payload) | expose the slice directly (`Elems()`); callers that run tokens build their tape from pointers |
| 100-105 | `OrderedMap.Keys()` copies the key order | explicit-copy (strings, not Values) | no | the map | unchanged (not a Value copy) |
| 178-190 | `ChildTypeInfo{Child Value; Elements []Value; Entries []ChildEntry; Len *int}` | by-value-flow | — | the child constraint is copied into every typed-container header, every `NewTypedList(child)` rebuild (resolve.go L125/L131/L137/L143, unify_list L88, unify_map L112) | `Child *Value`; Elements/Entries become pointer slices |
| 195-198 | `ChildEntry{Key, Value Value}` | by-value-flow | — | `AsMap` L4165-4170 builds a throwaway `OrderedMap` from Entries on every typed-map read | `Value *Value` |
| 227 | `FnParam.Pattern *Value` | by-value-flow (already a pointer; the pointee is a heap copy made by fn_params/lang at sig build) | — | shared by every `FnSig` copy (`compileFnSigs` L816/L830, `ExpandOptionalSigs` L1429); dereferenced into fresh copies at core_helpers L628, L708, L1377, L1459 | pointer stays; derefs disappear |
| 261 | `FnSig.ReturnPatterns []*Value`; 1274-1280 `ReturnCheckInfo.ReturnPatterns []*Value` | by-value-flow (pointers) | — | shared between sig and the per-call ReturnCheck marker | unchanged |
| 340 | `FnSig.Patterns map[int]Value` | by-value-flow | — | the constructor-convenience mirror of Params; copied with every Signature struct copy (map header shared, elements by value) | `map[int]*Value` |
| 776-779 | `StoredBodySpec{Params []FnParam}` | by-value-flow | — | copied onto every signature at registration (comment L~700) | unchanged |
| 813 | `CallableSpec.Inputs func(args []Value) []Value` | by-value-flow | — | handler closures receive/return Values by value | `[]*Value` |
| 1049 | `FnDefInfo.Wraps *Value` (pointer to the wrapped fn value) | by-value-flow | — | 1812 `UnwrapModifierChain` does `base = *fd.Wraps` (read copy) | walk pointers |
| 1069, 1192-1195 | `FnDefInfo.Captured []CapturedBinding{Name, Value Value}` — the closure environment, "shallow snapshot" (lang/go/CLAUDE.md Closures): pointer-backed payloads share state, reassignments don't affect captures | by-value-flow (SNAPSHOT that relies on independence of the HEADER) | yes | the outer binding's value at construction time; `InstallFrameBinding(r, cb.Name, cb.Value)` installs the same copy per call (core_helpers L491, L549; macro_expand L176); `capturedBindingsEqual` (compare.go L1262) compares captures by DeepEqual | `Value *Value` pointing at the binding's instance; the snapshot semantics survive (a rebind pushes a new instance) EXCEPT header mutations on the captured instance (Quoted/elem/asc/pos) would now show through the closure |
| 1165-1171 | `NewFunctionIdentified(info FnDefInfo, id)` sets `info.ident` on the by-value payload before boxing | explicit-copy (payload struct) | no (fresh) | — | construction |
| 1178-1186 | `WithFreshFnIdentity(v)` — `fd := v.Data.(FnDefInfo); fd.ident = &fnIdent{}; v.Data = fd; return v` | explicit-copy (header + payload, copy-to-modify) | yes | the pooled const in `Program.Consts` (eng/vm.go L2300, L4634, L4769 apply it to a `CloneValueKeeping` result); "a fn literal written in a fn body is a new function on every evaluation" (NUR288) | mint: `NewFunctionIdentified(fd, NewFnIdentity())` built from the const's payload parts, never touching the const |
| 1222-1247 | `FnDefInfo.OwnSigs` — `out := make([]Signature…)` filtered copy when a Fallback sig exists | explicit-copy (Signature structs, Value-bearing) | no (read view) | the dispatch aggregate / the authored slice | return a filtered view of pointers or iterate with a predicate |
| 1301-1319 | `DisjunctInfo{Alternatives []Value}` | by-value-flow | — | the slice is SHARED by every re-wrap: `NewDisjunct(disj.Alternatives)` (unify.go L273, unify_disjunct L124/L146), `installDisjunctUnifier` (unify_disjunct L102-108), `TypeBody` copy, `DefEntry` body — one backing array under 4+ headers | `[]*Value`; the slice sharing is already pointer-like |
| 1321-1323 | `NegationInfo{Inner Value}` | by-value-flow | — | copied into `NegationUnifier.inner` (unify_negation L133-139) and re-wrapped per call (L105, L127) | `Inner *Value` |
| 1339 | `ClassTypeInfo.BinaryLayout Value` "zero Value otherwise" | by-value-flow (zero sentinel inside a payload) | — | read by lang's Bytes codec | `*Value`, nil = none |
| 1347-1365 | `ClassTypeInfo.AllFields` — merges parent+own fields into a fresh `OrderedMap` (re-`Set` loop), cached in `cachedAllFields` | explicit-copy (OrderedMap) | no (values are immutable type constraints; the merge is a different map) | `Fields` maps of the class chain | same construction over `*Value` entries; note the cache pointer lives in a by-value payload (see core_make L261) |
| 1385-1392 | `ClassInstanceInfo.AllFields` — fresh copy of `Fields` | explicit-copy (OrderedMap) | no (consumers render: `formatFieldBag` L4400 area) | the instance's live `Fields` | return the live map (as `FlatInstanceParts` L3793 already does) |
| 1411-1427 | `ResourceTypeInfo.AllFields` | explicit-copy (OrderedMap) | no | as 1347 | as 1347 |
| 1453 | `StoreInstanceInfo.Data map[string]Value` (COW layer) | by-value-flow | — | `CowSet` layers (core_helpers L1004-1041) store `val` by value; `clone.go` L214-225 clones per layer | `map[string]*Value` |
| 1467-1482 | `StoreInstanceInfo.Get` returns copies and `Value{}` on miss | by-value-flow (zero sentinel) | — | — | `*Value`, nil |
| 1515-1538, 1540-1543 | `VisibleEntries() []StoreEntry{Key, Value}` — sorted snapshot of the visible keyset | explicit-copy (snapshot) | no (read view; `storeDeepEqual` compare.go L947 and `storeEntryMap` convert_ideal L172 consume it) | the store layers | `[]StoreEntry{Key, *Value}` |
| 1558-1572 | `StoreInstanceInfo.Set` stores `val` by value AND rewrites `childStore.Parent/ParentKey` through the nested store's payload POINTER | by-value-flow + an existing in-place mutation of a shared payload | — | the nested store value, wherever else it is held | unchanged (already in situ) |
| 1592-1606 | `MarkInfo{Body []Value}` "original content to replay" | by-value-flow (parsed/re-run tokens) | yes (replay) — tape reviewer | the mark's paired move re-inserts the body per iteration (loop.go) | tokens must become immutable or the tape must stop writing headers |
| 1608-1610 | `SpliceInfo{Data Value}` | by-value-flow | — | `stepLiteral` splices the payload's elements onto the tape (engine.go) | `Data *Value` |
| 1621-1626, 1650-1721 | `MoveInfo{Cont *Loop}`; `Loop{Body, Results, WhileCond []Value; mark, open, close, move Value; one [1]Value; toks []Value}` | by-value-flow (loop.go owns the behaviour) | yes for `Body` (replayed each iteration) and the reusable synthetic tokens (`mark/open/close/move` "minted once … reused by every iteration"); `Results` is scratch | loop.go | tape reviewer |
| 1750-1753 | `IfCont{Then, Else []Value}` | by-value-flow | yes (spliced per firing) | engine.go `stepMoveIf` | tape reviewer |
| 1757-1778 | `ModuleDesc{Exports map[string]*OrderedMap; Src *Registry}` | by-value-flow (pointer maps) | — | shared by every namespace of one module (`ModuleNSInfo.Module` L2229 holds the descriptor Value by value, boxing a `*ModuleDesc`) | unchanged |
| 1802-1821 | `UnwrapModifierChain` — `base = *fd.Wraps` | explicit-copy (read) | no | — | pointer walk |
| 1896-2000 | `Value` struct: `Parent *Type`, `tmeta *typeMeta` (SHARED by copies, mutated in place: L2084-2156, 2296), `pos`, `dynFrom`, `elem *Value`, `asc *Type`, `modns` (COPY-LOCAL: replaced on the copy by L2170, L2186, L2209, L2260, L2244), nine one-byte flags (copy-local) | by-value-flow (the duality itself) | — | every copy of a type node shares `tmeta` — the design already gives "in situ" semantics for type metadata; the five copy-local facets and the flags are what the policy would change | see §1.5 |
| 2016-2078 | `typeMeta{Body *Value; RefinementBase, RunForward *Type; …}` | by-value-flow | — | `Body` points at a HEAP COPY of the declaration (L2125-2128) | point at the one declaration instance |
| 2115-2122 | `TypeBody()` returns `*v.tmeta.Body` — "The returned Value is a copy; the stored content is written once at install and never mutated" | explicit-copy | yes by contract (declaration immutability); consumers: `TypeContentOf` L2138, `depScalarContent` (unify_dep L184-194), `hasUnknownRefinement` (type_run_install L34-38), `MakeClassFieldValue` (core_make L549-555), `IsValueOfType` (core_type L186-188) — all read-only locals | `tmeta.Body`, the `DefEntry` body, the Behavior copies (§1.2.5) | return the pointer; enforce declaration immutability |
| 2125-2128 | `SetTypeBody(body)` — `b := body; Body = &b` heap snapshot at install | explicit-copy | no under the policy (the pushed entry body and the stamp are the same declaration) | `installTypeBinding` (core_type L346-351), `mintRunType` (type_run_install L173) | store the pointer |
| 2138-2149 | `TypeContentOf` — `v.Data.IsTypeContent(&v)` passes the address of the by-value param | by-value-flow (`&v` orphan handed to the payload seam; `FnDefInfo`/`ClosurePayload` read the owner's dispatch identity) | no (read) | — | passes the real pointer |
| 2158-2174 | `Pos()` copies `*v.pos`; `SetPos` allocates `&p` (a `SrcPos`, not a Value) | by-value-flow | — | positions are minted once at parse and THREADED by pointer copy (`WithPos`, util.go L321) | `SetPos` becomes the only write; `WithPos` on an existing value is the hazard (§1.2.1/2) |
| 2176-2193 | `DynFrom`/`SetDynFrom` (check-mode, copy-local) | by-value-flow | unsure (check-mode narrowing-through-use tightens a copy pushed as a shadow) | the dynamic carrier's binding | in-place write would retag the binding for every later read — intended by the narrowing doctrine? needs the check reviewer |
| 2198-2216 | `ElemConstraint()` returns `*v.elem` copy; `SetElemConstraint(c)` stores `&c` (heap copy of the constraint) on the COPY only | explicit-copy (`&c`) + copy-local facet | yes at the carrier/flex retag sites (core_helpers L670-672; unify_fold L111-112; unify_list L154-155; unify_map L205-207), no at the construction sites (core_flex L48/L67/L138/L157/L406/L408/L426/L428; unify_list L173; unify_map L225) | see §1.2.3 | a typed VIEW over one store needs a second header: mint a new header from parts (`NewValueRaw(v.Parent, v.Data)` + elem) or carry the tag on the binding |
| 2221-2250 | `ModuleNSInfo{Name; Module Value}`; `WithModuleNS(v,…)` sets `v.modns` on the copy and returns it | explicit-copy (copy-to-modify) | no (lang's `NewModuleNamespace` passes a fresh `NewMap(fields)`) | — | set in place on the fresh map |
| 2252-2273 | `AscribedType`/`SetAscribed`/`StripAscribed(v)` — the `as` ascription is attached to a stack copy and stripped on a copy at every arg-delivery boundary ("never leaks into handlers, bindings, containers, or results") | explicit-copy (copy-to-modify) | yes | the binding/container element `v` was read from; concurrent forks; a failed dispatch would leave the flag on the binding | carry the ascription on the stack cell / dispatch plan, not on the Value |
| 2471-2481 | `NewValueRaw` — construction; eager ID only for `data == nil` or during a pass | construction | — | — | — |
| 2608-2612, 2673-2677 | `NewEvalList`/`NewEvalMap` set `Eval` on the fresh value | construction | — | — | — |
| 2697-2700 | `NewImplicitMap` — `entries.Implicit = true` mutates the CALLER's `*OrderedMap` | by-value-flow (existing in-place mutation of a shared payload) | — | the parser's map | unchanged |
| 2750-2758 | `AtomReferent` returns `*ap.Referent` copy / `Value{}` | explicit-copy + zero sentinel | no (metadata only: equality/canon ignore it, payload.go L99-102) | — | return the pointer |
| 2761-2771 | `SetAtomReferent(v, ref)` — `snap := ref; ap.Referent = &snap; v.Data = ap; return v` — "a snapshot of what its name refers to" at quote time | explicit-copy (copy-to-modify + snapshot) | unsure: the snapshot is documented but is metadata; the atom header copy is incidental (`quote` constructs a new atom anyway) | the referent binding's value | `Referent *Value` to the binding's instance; the atom is constructed fresh |
| 2799-2801 | `IsUnboundSlot` — the zero `Value` is the VM frame-slot sentinel ("no value the engine mints is the zero Value") | by-value-flow (zero sentinel) | — | eng vm `seatConstLocal` L4633 `locals[cl.Slot].Parent == nil` | nil pointer — strictly simpler |
| 2803-2808 | `NewTypeLiteral(t)` — `return *t` | explicit-copy — THE duality | yes (§1.2.1): 27 in-scope call sites hand the copy to user data, sig patterns, fills and the tape; the canonical node is in `TypeTable.byID` | the lattice node | return `t` itself once bare nodes are immutable and occurrence metadata (pos, Quoted, Carrier) lives off the Value |
| 2814-2819 | `TypeNodeOf` — `return &v` (orphan pointer to the param copy) | by-value-flow (`&v` orphan) | no for the renderers; the orphan reaches `CommonAncestorType` (carrier_join L193-195) whose `seen` map is pointer-keyed → over-widening | — | the canonical pointer |
| 2843-2847 | `ValueType` — `return &v` for bare nodes | by-value-flow (`&v` orphan; used by compare.go L60-62, convert_ideal L32/L48, guard_predicate L133, record_typed_def L51, core_boolean…) | no where consumers use `Equal` (ID) or `CanonicalType`; the `aType == nil` / `a == b` pointer tests (compare_types L24, unify_lca L47) just lose the fast path | — | canonical pointer |
| 2914-2925 | `IsNoneShape` — `(&v).Equal(TNone)` | by-value-flow (`&v`) | no | — | — |
| 2985-2991 | `newMarkerValue` — `Value{Parent: t}`, ID only during a pass | construction | — | — | — |
| 3024-3030 | `NewReach(info)` sets `info.unit` on the payload copy; `ReachInfo{Receiver []Value; Segments []ReachSeg{KeyLit Value; KeyExpr []Value}; unit *lensUnit}` (payload.go L212-231) | by-value-flow (parsed tokens inside a payload; `unit` deliberately pointer-shared across copies) | yes for the tokens (`lowerReach` re-lowers per evaluation — engine) | the parsed program | tape reviewer |
| 3035-3042 | `NewReachFromKeys` — `[]Value{receiver}` | construction | — | — | — |
| 3059-3062 | `InterpPart{Lit; Expr []Value}`; payload.go L184-189 `XmlCren{Expr []Value}` | by-value-flow (parsed tokens re-evaluated per use, `EvalInterp`/`EvalXmlInterp` in engine.go) | yes (program tokens) | the parsed literal | tape reviewer |
| 3154-3158 | `NewMark(id, body…)` — `b := make; copy(b, body)` | explicit-copy (slice) | yes (replay body) — tape reviewer | the mark's region | — |
| 3200-3225 | `NewFunction` — `info.ident` minted if nil; `info.Signatures = append([]Signature(nil), …)` cloned before stamping `Anonymous` "so a caller's authored signatures are never written through" | explicit-copy (Signature slice) | unsure — the stamp is idempotent and only fires for anonymous defs whose sigs `afn` just built; no evidence anything reads an un-stamped original | the caller's `Signatures` backing array | stamp in place |
| 3332-3337, 3345-3349 | `NewClassType`/`NewResourceType` set `info.Type = t` on the payload copy | explicit-copy (payload struct) | no when the caller's `info` is transient; see core_helpers L276-296 / core_type L440-447 for the callers that re-mint a user-held body | — | construction |
| 3360-3391 | `NewStore*` | construction | — | — | — |
| 3425-3435 | `NewError` — `ErrorInfo.Data` shares the `BoruError`'s map | by-value-flow | — | clone.go L139-144 deep-clones it | unchanged |
| 3774-3791 | `FlatInstanceFields` — "returns a fresh copy … callers that mutate must copy first" | explicit-copy (OrderedMap) | unsure: `objectConvertBehavior.ToMap` (convert_ideal L103-108) wraps it in a plain Map; every plain-map write is copy-returning (lang) and `flex` deep-copies, so aliasing the live `Fields` looks safe, but `AsMutableMap` consumers would then write into the instance | the instance's `Fields` | return the live map (as `FlatInstanceParts` L3793-3809 does) |
| 4042-4084 | `AsList` — shared view (`ReadList{elems: lp.Elems}`), except weak lists (`wd.snapshotElems()` strong snapshot) and `MaterializerPayload` rows | by-value-flow (shared) / explicit snapshot for weak | yes for the weak sweep ("one consistent world per operation") | — | unchanged |
| 4086-4102 | `AsMutableList` — shared slice | by-value-flow (shared) | — | — | — |
| 4147-4174 | `AsMap` — shared `*OrderedMap`; typed-map `Entries` arm builds a throwaway `OrderedMap` (L4165-4170); weak map `snapshot()` | explicit-copy (typed-entries arm), snapshot (weak) | no / yes (weak) | — | typed entries as a view |
| 4187-4196 | `Value.String` — `inner := v; inner.Dynamic = false` to render the bound of a dynamic carrier | explicit-copy (copy-to-modify) | no | — | render via a helper that takes the flag as a parameter |
| 4521-4559 | `IsTypeValue` — `elems.Slice()` | explicit-copy (read) | no | — | iterate the view |
| 3833-3844 | `payloadOf[P]` and every `v.Data.(X)` assertion copy the payload struct out of the interface (FnDefInfo ≈ 200 bytes) | by-value-flow (interface boxing) | — | — | pointer payloads for the large structs |
### clone.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 42-44 | `CloneValue(v)` → `CloneValueKeeping(v, nil)` | explicit-copy (the universal deep clone; callers classified in §1.3) | yes — `clone` word semantics (`(clone p) eq p` false), process-boundary isolation, compiled per-call freshening | the source value, wherever held | clone IS construction under the policy: build fresh headers from parts; the only forbidden idiom here is `withPayload`'s "start from the original header" |
| 57-60 | `CloneValueKeeping(v, keep)` — keep-set returns an EMBEDDED binding's container as-is (L81-83) | explicit-copy with a sharing exception | yes (`def c [9]  def mk fn [[] [List] [[c]]]`: outer fresh per call, member stays the binding's instance) | `Program.ConstKeep` (compiler) | same |
| 74-78 | `cloner.withPayload(v, data)` — `v.Data = data; v.ID = GenerateID(..); return v` — keeps Parent/Quoted/Eval/Carrier/Dynamic/pos/elem/asc/modns/dynFrom/tmeta | explicit-copy (header copy-to-modify) | yes (the clone must be a distinct value with a fresh ID) | the original | `NewValueRaw(v.Parent, data)` + explicit flag copy (construction from parts) |
| 85-87 | nil `Data` (type literal / carrier) → `return v` (a by-value copy of the node) | explicit-copy (duality) | no | the node | return the same pointer |
| 89-93 | `ListPayload`/`MapPayload` → `cloneSlice`/`cloneOrderedMap` (L161-197) rebuild containers element by element | explicit-copy (container + recursive header copies) | yes (independence) | — | container construction over freshly minted element headers |
| 95-126 | `*FlexListData`/`*WeakFlex*Data` cycle-safe via `seen map[any]any` keyed by payload pointer | explicit-copy | yes | — | same |
| 128-137 | `*StoreInstanceInfo` → `cloneStore` (L205-236: detaches Parent/ParentKey, re-links children, clones prototype chain, copies tombstones); `ClassInstanceInfo` → `cloneObject` (L242-247); `TableData` — `nd := p` struct copy + `Rows` clone | explicit-copy (payload struct + containers) | yes | — | same |
| 139-144 | `ErrorInfo` — `nd := p`, `Data` map cloned | explicit-copy | yes (raise payload map independence) | — | same |
| 146-152 | `ExtensionPayload` — `DeepCloner` or share | explicit-copy / share | per host type | — | — |
| 154-157 | default: scalars, time family, type bodies, `FnDefInfo` (identity token kept), markers are SHARED | by-value-flow (shared payload; header still copied by Go) | — | — | under the policy the header would be shared too — `clone` of a scalar would return the same instance, which is fine only if scalar headers are never mutated (pos stamps!) |

### payload.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 41-58 | `Payload.IsTypeContent(owner *Value)` seam | by-value-flow (`&v` orphan passed at core_helpers L1143, value.go L2144) | no | — | real pointer |
| 66-90 | scalar wrapper structs boxed by value in the interface | by-value-flow (payload copy on every assert/re-box) | — | — | — |
| 103-106 | `AtomPayload.Referent *Value` — heap snapshot pointer (see value.go L2761) | by-value-flow | unsure (metadata) | — | point at the binding instance |
| 115 | `ListPayload{Elems []Value}` — struct value boxed; `p := v.Data.(ListPayload)` yields a slice header copy sharing the array; re-boxing a modified `ListPayload` (core_make L514, clone L90) | by-value-flow (two Values can share one array with different `len`; `sameContainer` L478 defines list identity by the array) | — | every header over the array | `[]*Value`; identity moves to the Value if desired |
| 119 | `MapPayload{M *OrderedMap}` pointer-backed → header copies alias the map; plain-Map `set` must copy the OrderedMap (lang) | by-value-flow | — | — | unchanged |
| 127 | `FlexListData{Elems []Value}` pointer-backed store | by-value-flow | — | `retagFlexChildrenInPlace` (core_helpers L697) already writes retagged element HEADERS back into the shared store | `[]*Value` |
| 137-141, 198-202 | `XmlElementPayload{Attr *OrderedMap; Cren []Value}` (immutable; identity = Attr pointer + Cren array, compare.go L503-513), `FlexXmlData` | by-value-flow | — | — | `[]*Value` |
| 151-189 | `XmlInterpPayload{Tmpl XmlTmpl}` with `XmlCren.Expr []Value` holes, `XmlAttrTmpl.Parts []InterpPart` | by-value-flow (program tokens evaluated per use) | yes (tape reviewer: `EvalXmlInterp`) | the parsed literal | — |
| 206 | `ParenExprPayload{Toks []Value}` — the engine re-expands per evaluation (`expandParenExpr`, engine.go) | by-value-flow (program tokens) | yes (tape reviewer) | the parsed group; macro operands (macro_expand L103-105) | — |
| 212-231 | `ReachInfo{Receiver []Value; Segments []ReachSeg{KeyLit Value; KeyExpr []Value}; unit *lensUnit}` — "a POINTER, so every copy of the Reach value shares one cache" | by-value-flow (tokens by value; cache pointer-shared) | yes for tokens | `convert Map` copies them out (convert_ideal L227/L239/L257) | — |
| 235 | `InterpStringPayload{Parts []InterpPart}` | by-value-flow (tokens) | yes (`EvalInterp`) | — | — |
| 297-303 | `NewExtension` — eager ID construction | construction | — | — | — |
| 440-449 | `GuardFactInfo{Toks []Value; Prev Payload}` — a check-mode wrapper the paren evaluator puts on a Boolean carrier (the carrier's `Data` is re-boxed around the group's original tokens) | by-value-flow (the Boolean carrier header is copied and its Data replaced — engine/check side) | unsure | `extractGuardClauses` L53-54 reads `gf.Toks` directly (shared slice) | wrapper stays; the token slice is already shared |
| 459-489 | `*StoreShapeInfo{KeyTypes map[string]Value; Vals Value (zero = nothing recorded); DeclaredVal *Type}` — "every Value copy of the carrier aliases the SAME shape, mirroring the runtime aliasing of the mutable container it stands for" | by-value-flow (pointer payload by design; `Vals` zero sentinel) | — | all carrier copies of one store | the aliasing model the policy generalises; `Vals *Value` |
| 499-602 | `ClosurePayload{Captures []Value; RetPatterns []*Value; Source *Value; Ident FnIdentity (copied with the value); RetTrim, SigMatched "set on the VALUE at the seam, never on the stored closure"}` | by-value-flow + a documented copy-to-modify discipline at the VM seams (eng, other reviewer) | yes (the stored closure must not carry seam flags) | the closure stored in a container/binding vs the copy handed to a handler | seam flags move to the call frame |
| 608-612 | `NewStoreShapeCarrier` — fresh carrier with a fresh shape | construction | — | — | — |

### core_flex.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 31-83 | `FlexDeepCopy` — rebuilds maps/lists/xml as FRESH flex containers at every depth; scalars/functions/Ideals pass through unchanged (shared); `WithPos(NewFlexMap(om), v)` and `SetElemConstraint` on the fresh result | explicit-copy (container rebuild; element headers copied by value) | yes by design ("a flex node must never alias the entries of an immutable source", design/FLEX-NODES) | the immutable source and every binding of it | container construction over shared element POINTERS — but then a header write on a shared scalar element (pos stamp when it lands on the tape) shows in the immutable source |
| 90-111 | `flexXmlChildren` — same for Attr/Cren | explicit-copy | yes | — | same |
| 119-189 | `NodeDeepCopy` — identity fast path when nothing flex is inside (L120-122, returns v), else rebuilds plain containers | explicit-copy | yes ("an immutable container can never change underneath through a live flex handle") | the flex source | same |
| 193-240 | `containsFlex` | read | — | — | — |
| 261-287 | `AdoptIntoFlex` — deep-copies a plain container stored into a flex tree (never-mixed invariant), shares flex/weak handles | explicit-copy (via FlexDeepCopy) | yes | the stored plain value's other holders | same |
| 296-298 | `AdoptIntoNode` → `NodeDeepCopy` | explicit-copy | yes | — | same |
| 315-447 | `MakeNodeHandler` — `target := &targetVal` (L333, orphan used only for `Equal`); every `out = WithPos(out, srcVal)` / `out.SetElemConstraint(..)` (L401-409, L423-430, L443) is on a freshly built result | by-value-flow (`&v`) + construction | no | — | — |

### core_helpers.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 55-328 (98) | `installDef` — `fnDef, ok := body.Data.(FnDefInfo)` payload copy, then `fnDef.Name = name` | explicit-copy (payload copy-to-modify) | yes: `def a (f/v)` must not rename f's own value (`FormatFnDef` L4494 prints the Name; `print f/v`); canon excludes Name (NUR031) precisely because the payload carries a per-binding fact | f's binding, any container holding the fn value | move the registered name onto the `DefEntry`; the payload stops carrying a binding name |
| 243 | `NoteSpecFnDef(r, name, Value{}, body, …)` — zero Value as "no dropped entry" | by-value-flow (sentinel) | — | — | nil |
| 270-299 | class body: `info, _ := AsClassType(body)`; `info.Name = …` (L276/L279); `def := body.Parent` (L292); `body = NewClassType(def, info)` (L296) re-mints | explicit-copy (payload copy-to-modify + re-mint) | unsure: the body is usually the transient `class {…}` result, but a lowercase-bound class value (`def c (class {…}); def Foo c`) would be renamed in place (renders `object<Class/Foo>`) | c's binding | separate the bound name from the payload (as above) |
| 330-368 | `WrapperUnderName` — `NewFunction(FnDefInfo{Signatures: append([]Signature(nil), inner.Signatures...), ident: inner.ident})` | explicit-copy (Signature slice snapshot of the inner aggregate) | no (the aggregate is rebuilt per `Lookup` anyway; identity token shared) | — | share the slice |
| 385-612 | `buildFnBodyHandler` — construction-time skeleton: L425 `skeleton = append(skeleton, s.Body()...)` copies the body tokens once | explicit-copy (slice) | yes (the skeleton is per-signature shared state; see L505-506) | `s.Body()` (the `BoruImpl.Body`, shared by every binding of the fn) | tokens immutable + tape writes off-Value |
| 484-486 | per call: `buf := make([]Value, n+len(args)); argsCopy := buf[n:]; copy(argsCopy, args)` — the lazily-exposed `args` list | explicit-copy (slice) | unsure: isolates the `args` word's list from the dispatcher's collected slice; under pointers a slice copy is cheap and still needed only if the dispatcher reuses `args` | execMatch (engine) | keep a pointer-slice copy (not a Value copy) |
| 500, 561 | `arg := args[i]; if list && !Quoted { arg.Quoted = true }; InstallFrameBinding(r, p.Name, RetagTypedContainerParam(p, arg))` — the named param binding is quoted while the args stack (copied at L486 BEFORE the write) and unnamed body pushes (L507) keep the unquoted header | explicit-copy (copy-to-modify) | yes: the caller's list value and `args.N` stay unquoted; canon/print renders `Quoted` as `(quote …)` (canon.go L225-227) so an in-place write is user-visible on the caller's binding | the caller's binding, `args.N`, unnamed-param tokens | a "quoted" attribute on the frame binding (DefEntry) rather than the Value |
| 505-507 | `out := buf[:n:n]; copy(out, skeleton); out[1+k] = args[i]` — per-call copy of the shared skeleton "mandatory: execMatch's stampResultPos mutates the returned slice (ReturnCheck Pos, fn-value pos), and ForkConcurrent engines share this handler" (L408-412) | explicit-copy (slice) | YES (§1.2.2) | the skeleton; concurrent forks | engine must stop writing tape cells, or per-call minting of the few mutable markers |
| 537-539 | `argsCopy := make; copy(argsCopy, args); argsList := NewList(argsCopy)` | explicit-copy (slice) | unsure (as L484) | — | pointer-slice copy |
| 595 | `result = append(result, s.Body()...)` — per-call copy of body tokens into the frame expansion ("append COPIES them … s.Body() is never mutated here") | explicit-copy (slice) | yes (§1.2.2) | `BoruImpl.Body` shared by every call and binding | same as 505 |
| 624-629 | `RetagTypedContainerParam` — `*p.Pattern` deref | explicit-copy (read) | no | — | pointer |
| 644-661 | `RetagTypedContainerValue` — flex arg → `RetagFlexElem` (header copy); plain arg → `Unify(pat, arg)` REBUILDS the container (unify_list/unify_map) so the param's map/list is a different container from the caller's (`eq` false between them today); `unified.Quoted = arg.Quoted` on the rebuilt value | explicit-copy (via Unify rebuild) | yes as written (the caller's plain container is untouched); note the rebuild already breaks container identity across the param boundary | the caller's binding | in-place tagging would restore identity but leak the tag (§1.2.3) |
| 670-682 | `RetagFlexElem` — `out := v; out.SetElemConstraint(elemType)`; "v's header with the tag set — the FlexListData/FlexMapData pointer stays shared" | explicit-copy (copy-to-modify) | yes: the caller keeps an untagged header over the same store; an in-place tag would make the caller's flex map enforce `{:T}` writes after the call | the caller's binding(s) | second header = re-mint from parts (`NewValueRaw(v.Parent, v.Data)` + elem) or a tag on the binding |
| 685-702 | `retagFlexChildrenInPlace` — writes retagged CHILD headers back into the shared store (`om.Set`, `fd.Elems[i] = …`) | by-value-flow: an existing IN-PLACE mutation of shared contents (the caller's nested children are tagged today while the top-level header is not) | — | — | evidence the independence of the top-level copy is not consistently relied on |
| 704-710 | `isTypedContainerParam` — `pat := *p.Pattern` | explicit-copy (read) | no | — | pointer |
| 721-737 | `RetagTypedContainerArgs` — copy-on-write `out = make; copy(out, args)` only when a retag happens | explicit-copy (slice) | unsure: protects the dispatcher's `args` from retagged elements (a stack-only retry would otherwise see them — harmless) | execMatch | pointer-slice copy or in place |
| 758-786 | `installFnDef` — `entry := fnDef; entry.Name = name; entry.Signatures = compileFnSigs(...); r.Defs.Push(name, NewFunction(entry))` — the binding gets a NEW Function value (compiled, renamed) while the source value keeps its authored sigs; `ident` shared | explicit-copy (payload copy-to-modify) | yes: the compiled handlers close over the install registry `r` (L385-399) so the compiled form is per-binding/registry; `fnIdent.dispatch` (value.go L1144-1150) already caches a compiled form per token | the source fn value (a literal, a `/v` reference, a module export) | keep authored `FnDefInfo` immutable; hold the compiled dispatch in the `DefEntry` or the `*fnIdent` cache |
| 794-850 | `compileFnSigs` — `s := sig`, `fnDefCopy := fnDef`, `cs := s` struct copies captured by the handler closure (captures snapshot: `fnDefCopy.Captured`) | explicit-copy (Signature/FnDefInfo structs) | yes for the closure (the handler must see the captures as of install); the Signature copies are the compiled sigs (new objects) | the authored `FnSig` slice (`ExpandOptionalSigs` output) | compiled sigs are constructions; captures by pointer |
| 867-925 | `UninstallFnSigs` — `stack = append([]Value(nil), stack...)` before filtering; `rm := stack[j]` handed to the bind ledger as `DefEntry{Body: rm}` | explicit-copy (slice; snapshot of the removed entry) | unsure: the ledger entry must reproduce the removed binding on replay, so it must not be mutated after removal | the def stack (`r.Defs.Set` replaces it anyway) | pointer-slice copy; ledger holds the pointer |
| 1004-1041, 1065-1098 | `CowSet`/`CowDel` — new Store layers hold `val` by value in `map[string]Value`; `childStore.Parent = newStore` (L1016) mutates the nested store's payload in place | by-value-flow (payload-level COW; existing in-place pointer rewrite) | — | — | `map[string]*Value` |
| 1126-1144 | `IsTypeBody` — `v.Data.IsTypeContent(&v)` | by-value-flow (`&v` orphan) | no | — | pointer |
| 1239-1283 | `SimplifyDisjunctAlts` — `live`/`out` fresh slices of the alt copies, `sort.SliceStable(out, …)` | explicit-copy (slice; sorts the copy, not the input) | no (callers hand it fresh slices; `NewDisjunct(SimplifyDisjunctAlts(du.Alternatives))` in carrier_new L146 relies on the INPUT being left in authored order? — `tor` canonical order is applied one layer up, so the Behavior's slice is already canonical) | the unifier's `Alternatives` slice | sort a pointer slice |
| 1325-1372 | `BaseValue`/`BaseValueForConstraint` — `NewTypeLiteral(TNone)` as the base VALUE of None/disjunct-without-literal | explicit-copy (duality; literal as data) | yes (§1.2.1) | TNone | occurrence cell |
| 1376-1397 | `omittedDefaultValue` — `IsOptionsType(*p.Pattern)`; copies the schema's concrete defaults into the synthesized map by value WITHOUT `FreshenDefault` (contrast unify_options L162) | explicit-copy (OrderedMap) + by-value-flow (aliasing of a mutable default across calls — pre-existing inconsistency) | unsure | the Options schema's default values | freshen like `FillConcreteOptionDefaults`, or share deliberately |
| 1399-1500 | `ExpandOptionalSigs` — `expanded = append(expanded, sig)` FnSig copies; `Pattern: p.Pattern` pointer shared (L1429); `Unify(bv, *p.Pattern)` (L1459); `body = append(body, bv)` embeds the synthesized default VALUE as a token of a generated body (L1464) that is re-stepped per call | explicit-copy (FnSig) + by-value-flow (a Value embedded in a shared token list) | yes (§1.2.2: the embedded default token is pushed onto the tape per call) | the generated overload's body | tape reviewer |
### sugar.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 43-51 | `SugarInfo{Head Value; Items []Value}` — parsed payload | by-value-flow (program tokens inside a marker) | yes (a marker in a loop body is re-expanded per iteration) | the parsed program | — |
| 92-104 | `sugarBoundWord` → `WithPos(NewWord(name), src)` | construction | — | — | — |
| 110-168 | `SugarExpansion`: L123/L129 `WithPos(NewInteger/NewWord/NewString, src)` fresh; **L135 `WithPos(NewTypeLiteral(TType), src)`** stamps a position on a TYPE-NODE COPY; L136 `append([]Value{tt, of}, info.Items...)` copies the marker's Items into a fresh ParenExpr; L152 `info.Head` emitted as a token; L158 `NewEvalList(info.Items)` SHARES the Items slice as a list payload | explicit-copy (duality + slice) | L135: yes (§1.2.1 — under the policy it would stamp `TType` itself); L136/L158: no (read-only consumers; the eval list produces a new list) | `TType`; the marker's Items | an occurrence cell for the literal; share Items |
| 175-190 | `stepSugar` — `src := e.Tape.At(valIdx)` copy of the tape cell (pos donor), `e.Tape.Splice` | by-value-flow (tape reviewer) | — | — | — |
| 201-212 | `placeModifierOperand` — `reach := tape.At(at); tape.Set(at, WithPos(NewParenExpr([]Value{reach}), reach))` wraps the cell into a fresh paren token | construction (the reach Value copied into the new list) | no | — | pointer into the new list |

### macro_expand.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 23-51 | `execMacro` — memoized `expanded []Value` from `Registry.macroCache` is re-wrapped as `NewSplice(NewList(expanded))` for EVERY application; the splice step copies the list elements onto the tape (`stepLiteral`, engine.go) | by-value-flow (a CACHED token slice shared across applications and loop iterations) | YES (§1.2.2): if the tape held the cached pointers, stamping/flagging would corrupt the cache for the next application | the macro cache | cache must become immutable tokens, or be cloned per splice (which is the copy the policy forbids) |
| 71-127 | `scanMacroOperands` — `inner = append(inner, v)` tape cells copied into a fresh `ParenExpr` (`pe.pos = t.pos`, L103-105); `operands = append(operands, t)` tape-cell copies | explicit-copy (slice of tape cells) | no (operands are raw forms consumed by the expansion; under pointers the tape cells themselves would be the forms — but see L171-173) | the tape | pointers |
| 165-174 | `bindings[p.Name] = operands[i]` (unquoted form for the `unquote` fast path, L328-330) AND `q := operands[i]; q.Quoted = true; InstallFrameBinding(r, p.Name, q)` — the same form installed quoted in the def scope | explicit-copy (copy-to-modify; one form in two flag states) | yes: `unquote <param>` must splice the UNQUOTED form into generated code while the scope binding must be inert data | the operand form | a quoting wrapper (an inert marker value around the form) or a "quoted" attribute on the frame binding |
| 176 | `InstallFrameBinding(r, cb.Name, cb.Value)` — captured values installed by value | by-value-flow | yes (capture snapshot; see value.go L1192) | — | pointer |
| 180-182 | `body := make([]Value, len(sig.Body())); copy(body, sig.Body()); New(r).Run(body)` — the template body is copied before the sub-engine runs it | explicit-copy (slice) | yes (§1.2.2): the template is the macro's signature body, reused per expansion; the sub-engine mutates its tape | `BoruImpl.Body` | tape reviewer |
| 192-199 | `template := res[len(res)-1]`; `tmpl = l.Slice()` | explicit-copy (read) | no | — | view |
| 209-216 | `collectTemplateBinders`/`expandTemplate` walk; L292 `out = append(out, l.Slice()...)` splices a list's elements; L301-303 `gw := NewWord(g); gw.pos = t.pos` | explicit-copy (read) / construction | no | — | — |
| 325-341 | `resolveTemplateEscape` — `New(r).Run([]Value{operand})` runs a COPY of the operand form in a sub-engine | explicit-copy (1-element slice) | yes (the sub-run mutates the cell; `operand` is a form that may be spliced elsewhere) | `toks` | tape reviewer |
| 346-354 | `asTemplateNode` — new Word from an atom, `w.pos = v.pos` | construction | — | — | — |
| 358-394 | `expandNested` — rebuilds List/ParenExpr/Map (`nl.Quoted = t.Quoted` on the fresh list) | explicit-copy (container rebuild) | no (the expansion is a new program) | — | construction |
| 399-434 | `ExpandMacroForm` — `fnDef, ok := bound.Data.(FnDefInfo)`; `&fnDef` (pointer to the local payload copy) passed to `expandMacroWith` | explicit-copy (payload copy; read) | no | — | pointer to the binding's payload |
| 441-451 | `macroByName` returns `FnDefInfo` by value | explicit-copy (payload; read) | no | — | — |
| 458-533 | `expandAllMacros`/`expandAllNested` — rebuild | explicit-copy (container rebuild) | no | — | construction |

### core_make.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 67-170 | `MakeRecordR` — `result.Set(key, val)` stores provided field values by value (payload shared); L89-91 `noneVal := NewTypeLiteral(TNone)` stored as a FIELD VALUE when the constraint admits None; L133 `elems.Slice()` | by-value-flow + explicit-copy (duality as data) | L89: yes (§1.2.1 — a canonical `TNone` pointer inside user data); rest no | TNone | occurrence cell / a `none` VALUE instead of the literal |
| 220-266 | `makeClassInstance` — L243 `result.Set(key, FreshenDefault(constraint))` (per-instance mutable defaults); **L261 `TypeRef: &objType`** — the address of the by-value PARAM: every instance carries its own heap copy of `ClassTypeInfo` (with its own `cachedAllFields`, filled per copy) | explicit-copy (payload struct per instance) | no — pure duplication; `info.Type` (canonical `*Type`) is the real identity | the class declaration | store the canonical `*Type` and read the schema via `TypeContentOf(node)` |
| 281-314 | `makeResource` — same (`TypeRef: &resType` L310) | explicit-copy | no | — | same |
| 319-326 | `MakeClassInstance` — `vals[0]` | explicit-copy (read) | no | — | — |
| 446-517 | `FreshenDefault(v)` — `out := v` (keeps v's ID, flags, pos), `Data` replaced per arm: weak clones (L457-463), `ninfo := info` + fresh Fields (L475-477), `nsi := *si` + fresh Data map, prototype shared (L483-488), fresh OrderedMap (L496-501), fresh elems slice (L507-514) | explicit-copy (header copy-to-modify + container rebuild) | yes: "each make gets its own FlexList instead of aliasing the schema's single one" (the Python mutable-default trap); note it does NOT re-mint the ID (unlike clone) | the schema default (class field, Options default) | `NewValueRaw(v.Parent, freshData)` + flag copy — construction from parts |
| 542-589 | `MakeClassFieldValue` — `val = ResolveWordValue(val)` (may return a NEW value), `constraint = body` (`TypeBody` copy, L549-555), returns `val` (shares) or the Unify result | explicit-copy (read locals) | no | — | — |
| 687-800 | `MakeHandler` — L690 swap; **L752 `targetType := &targetVal`** orphan `*Type` (repaired by `CanonicalType` only for user types, L765-766); L754 `return []Value{srcVal}` (shares); **L777 `ReparentValue(conv, targetType)`** | by-value-flow (`&v` orphan) + explicit-copy (reparent) | reparent: yes — `def x 5  def y (make Pos x)` must leave x an Integer | x's binding / the stack value | mint a fresh Value from `conv`'s parts with Parent=def |
| 808-870 | `MakeTableR` — `rows.Slice()`, `rowElems.Slice()`, fresh `result` maps/`resultRows` | explicit-copy (read + fresh containers) | no | — | views + construction |
| 972-1035 | `MakeWithOpts` — `var targetVal, srcVal, optsVal Value` with `targetVal.Parent.Equal(nil)` / `srcVal.Parent.Equal(nil)` meaning "not yet assigned" (L973-983) | by-value-flow (zero-Value sentinel) | — | — | nil pointers |
| 1037-1099 | `MakeScalarHandler` — L1066 `targetType := &targetVal` orphan; L1068 `return []Value{srcVal}`; **L1087 `ReparentValue(conv, canon)`** | by-value-flow + explicit-copy | reparent: yes (as L777) | the source value | mint from parts |
| 1101-1126, 1128-1146 | `MakeObjHandler`, `MakeScalarOptsHandler` — delegate | — | — | — | — |
| 1148-1217 | `MakeConvert` — constructs; the `makerCapability` arm returns whatever the host Maker returns | construction | — | — | — |
| 1219-1274 | `MakeFieldValueR` — `val = ResolveWordValue(val)`; returns `val`/`out`/`unified` (operand or rebuild) | by-value-flow | no | — | — |
| 1276-1348 | `ResolveFieldType` — returns `tv`/`top` (def-table bodies by value → the schema holds a copy of a binding's body); L1311 `NewTypeLiteral(t)`; **L1328-1337 `input := make([]Value, …)` copies a schema list's elements into a token list run by a sub-engine** | explicit-copy (duality; slice for a sub-run) | L1328: yes (§1.2.2 — the schema literal persists; the sub-run mutates its tape); the def-body copy: unsure (a later `undef` pushes/pops bindings, never mutates, so sharing the pointer is safe) | the schema literal; the def table | tape reviewer; pointer sharing |

### convert_ideal.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 31-60 | `ConvertIdealToMap/List` — `ValueType(v)` orphan walk | by-value-flow (`&v`) | no | — | canonical pointer |
| 103-116 | `objectConvertBehavior.ToMap/ToList` — `FlatInstanceFields` fresh map → `NewMap(fm)` / `orderedMapValues` | explicit-copy (OrderedMap + values slice) | unsure (see value.go L3774: plain-map writes are copy-returning, so aliasing the live `Fields` is probably safe) | the instance's `Fields` | wrap the live map |
| 122-136 | `ClassFields`/`objectFieldMap` — fresh copy | explicit-copy (OrderedMap) | unsure (lang `items`/`transform` projections) | — | live map |
| 139-146 | `orderedMapValues` — fresh `[]Value` of the map's values | explicit-copy (slice) | no | — | pointer slice |
| 155-176 | `storeConvertBehavior` — `storeEntryMap` builds a map from `VisibleEntries()` | explicit-copy (snapshot → map) | no (a plain Map result) | the store layers | same construction over pointers |
| 184-207 | `errorConvertBehavior.ToMap` — copies `ei.Data` entries into a fresh map | explicit-copy (OrderedMap) | no | the error's raise map | same |
| 218-263 | `reachConvertBehavior` — `NewList(append([]Value(nil), info.Receiver...))`, `s.KeyExpr` copies (L227, L239, L257); `sm.Set("key", s.KeyLit)` | explicit-copy (program-token slices copied into user data) | unsure: prevents user data from sharing the reach's token backing array; under pointers the user list would hold the TOKEN instances (a quoted-param pass would set `Quoted` on a reach token) | the Reach's parsed tokens | tape/immutability rule decides |
| 287-314 | `tableConvertBehavior` — `append([]Value(nil), td.Rows...)` (L292), per-column `vals` slices | explicit-copy (defensive slice copies) | no | the table rows | pointer slices |
### unify.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 28-40, 82-90, 99-103 | `Unify`/`UnifyExplain`/`UnifyExplainR`/`unifyWithin` — `a = ResolveWordsDeep(a); b = ResolveWordsDeep(b)` REBUILDS both operands (resolve.go, below) on every call | explicit-copy (deep rebuild; perf) | yes as written: the operands are sig patterns, def bodies, user containers that must not be rewritten by resolution; in place would be a one-time canonicalisation — but the R/unarmed asymmetry (a user type resolves to its node only when `r != nil`, else degrades to an Atom, resolve.go L45-53) makes "whoever resolves first wins" order-dependent | the pattern/body/container | resolve once at construction (parse/def time) so Unify never needs a prepass |
| 144-151 | `MarkPredicateFn(v)` — `info.Predicate = true; return NewValueRaw(v.Parent, info)` (fresh ID, loses v's pos/flags; `ident` shared) | explicit-copy (re-mint from an existing value) | no (`fnpred` marks the fn literal it just received) | — | set in place |
| 232-243 | `sameFnConstruction` — identity via `&ai.Signatures[0] == &bi.Signatures[0]` ("FnDefInfo copies share the Signatures backing array") then `a.ID == b.ID` | by-value-flow (identity THROUGH the payload copy) | — | — | pointer identity of the Value / `*fnIdent` |
| 272-275 | `unifyDisjunctR` — `(&val).Equal(TAny)` orphan; `NewDisjunct(disj.Alternatives)` fresh header over the SHARED alts slice (also L146 in unify_disjunct.go) | by-value-flow | no | the disjunct's alternatives slice | pointer slice |
| 299-451 | `unifyInner` — `HasConstraintUnify(&a)`/`(&b)` orphans (L353-354); `return a`/`b`/`other` — Unify returns one of its OPERANDS by value in every narrowing arm (L327, L403, L412-414, L590-608) | by-value-flow (the unified value IS an operand copy) | no — under pointers the same instance comes back, which is what every consumer already assumes (payload shared) | — | — |
| 549-582 | `unifyObjectType` — `(&other).ConformsTo(oi.Type)` orphan | by-value-flow | no | — | — |
| 584-614 | `unifySameOrSubtype` — returns operands | by-value-flow | no | — | — |
| 621-626 | `denotedType` — `return &v` for an ID-bearing bare node; a hand-built ID-less `Value{Parent: T}` falls to `v.Parent` because "&v has no lattice identity to compare against" | by-value-flow (`&v` orphan; sentinel for ID-less literals) | no | — | canonical pointer; ID-less literals disappear |

### unify_binding.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 15-19, 84-90 | `BindingBodyUnifier{body Value}` — the declaration body stored BY VALUE in the node's Behavior (installed at core_type L690) | by-value-flow (third copy of the declaration, §1.2.5) | — | `tmeta.Body`, the `DefEntry` body | `body *Value` to the one declaration |
| 39-57 | `matchR` → `unifyWithin(v, u.body, r)` — the body copy is `ResolveWordsDeep`-rebuilt per membership test | explicit-copy (perf, via resolve) | no | — | resolve once |
| 70-78 | `Unify` → `unifyMembership(...)` returns the unified candidate (operand or rebuild) | by-value-flow | no | — | — |

### unify_dep.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 22-27, 110-117 | `DepScalarUnifier{baseType *Type; depInfo DepScalarInfo}` — payload copy in the Behavior | by-value-flow (declaration copy) | — | the body's `DepScalarInfo` | pointer |
| 62-88 | `Unify` — returns `v` (candidate) or `unifyDepScalar` → `NewValueRaw(aType, combined)` (L152, fresh interval) | by-value-flow / construction | no | — | — |
| 94-104 | `depScalarContent` — `v.TypeBody()` copy | explicit-copy (read) | no | — | pointer |
| 132-186 | `unifyDepScalar` — returns `other` (operand) or fresh | by-value-flow | no | — | — |

### unify_disjunct.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 19-23, 102-108 | `DisjunctUnifier.Alternatives []Value` — "copy of the disjunct's alternatives" per the comment, but `installDisjunctUnifier` stores the SAME slice as the body's `DisjunctInfo` (and as the `TypeBody` stamp and the `DefEntry` body) | by-value-flow (one backing array under several headers) | — | all four holders | `[]*Value`; already pointer-like |
| 51, 96 | `DisjunctInfo{Alternatives: d.Alternatives}` re-wrap per Match/Unify | explicit-copy (payload struct around a shared slice) | no | — | pass the slice |
| 118-147 | `unifyDisjunct` — L123 `(&val).Equal(TAny)` orphan; L124/L146 `NewDisjunct(disj.Alternatives)` fresh header, shared alts; returns `val`/`unified` | by-value-flow | no | — | — |

### unify_error.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 18-22, 56-58 | `UnifyError{A, B Value}` — operand copies kept for diagnostics | by-value-flow (snapshot) | no | — | pointers (the operands are not mutated after a failed unify) |
| 40-53 | `withPath` — fresh error copying A/B | explicit-copy | no | — | — |

### unify_fnsig.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 13-31 | `unifyFnUndefShape` — returns the `fn` operand | by-value-flow | no | — | — |

### unify_fnundef_named.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 32-36, 109-115 | `FnUndefUnifier{sigs []FnSigSpec}` (`FnParam.Pattern *Value` inside) | by-value-flow (declaration copy) | — | the body's `FnUndefInfo.Sigs` slice (shared) | pointer |
| 64, 100 | `NewValueRaw(TFnUndef, FnUndefInfo{Sigs: f.sigs})` — a transient FnUndef VALUE minted per `Match`/`Unify` to call `fnUndefMatchesFnDefR` | construction (perf only) | no | — | pass the specs directly |
| 78-104 | `Unify` — returns `candidate` | by-value-flow | no | — | — |

### unify_fold.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 18-53 | `unifyFlexLiteral` — returns operands | by-value-flow | no | — | — |
| 61-71 | `unifyZip` — `out := make([]Value, n)` of unify results | explicit-copy (result slice; elements are operand copies or rebuilds) | no | — | pointer slice |
| 76-79 | `constAt`/`sliceAt` closures over Values | by-value-flow | no | — | — |
| 92-114 | `unifyCarrierVsTyped` — `out := carrier; out.SetElemConstraint(ct.Child); return out` (check-mode: a `(flex …)` residual meeting `{:T}`) | explicit-copy (copy-to-modify) | unsure: the carrier is usually the value being BOUND (consumed), but it may be a def binding read by name (`def w:{:T} m`), whose later writes would then be enforced in the checker | the carrier's binding | re-mint from parts or tag the binding |
| 120-131 | `unifyMapValues` — fresh `OrderedMap` → `NewMap` | explicit-copy (container rebuild) | no (the unified map is a new value by contract) | — | construction over pointers |

### unify_lca.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 44-98 | `dispatchUnifier` — `denotedType(a)` orphans; `aType == nil` pointer tests; walks `t.Parent` from an orphan (its Parent pointers are canonical; `t.Behavior()` reads the shared tmeta, so the orphan still dispatches correctly) | by-value-flow (`&v`) | no | — | canonical pointers |

### unify_list.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 16-118 | `unifyListFamily` — returns operands (L45, L54); `NewTableType(uRec)` / `NewTypedList(unified)` fresh (L77, L88); L107-108 `aLst.Slice()`/`bLst.Slice()` then `unifyZip` → `NewList(result)` | explicit-copy (read slices) / construction | no | — | views |
| 139-148 | `reparentSwappedElem(src, unified, r)` — `ReparentValue(src, CanonicalType(r, ValueType(unified)))` for a bare-refine child: the result list holds RETAGGED COPIES of the caller's elements | explicit-copy (reparent) | yes: `def xs [1 2]  f xs` with `xs:[:Pos]` must not turn xs's own elements into `Pos` (`typeof (xs get 0)`) | the caller's list elements (shared payloads) | mint fresh element headers from parts |
| 150-175 | `unifyTypedListWithConcrete` — L154-155 `out := concrete; out.SetElemConstraint(childType)` (carrier copy-to-modify, check mode); L159 `lst.Slice()`; L164-166 reparent loop writes into the FRESH `result`; L167-173 fresh `out` + tag | explicit-copy (carrier copy) + construction | carrier copy: unsure (as unify_fold L111); rest no | — | — |

### unify_map.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 14-133 | `unifyMapFamily` — returns operands (L49, L58, L65, L74); L55/L71 `NewOptionsType(info.Fields)` fresh header over the SAME `Fields` map; L112 `NewTypedMap(unified)` fresh | by-value-flow (two headers, one `*OrderedMap`) | no | — | — |
| 137-188 | `unifyConcreteMaps` — L138 `absentVal := NewTypeLiteral(TAbsent)` (literal as the missing-key operand); fresh `result` → `NewMap` | explicit-copy (duality; container rebuild) | L138: no (never stored — Absent results are dropped L158/L168/L182); rebuild: no | — | — |
| 201-227 | `unifyTypedMapWithConcrete` — L205-207 carrier copy-to-modify; L209-220 `unifyMapValues` then `om.Set(k, reparentSwappedElem(sv, uv, r))` writes retagged copies into the FRESH map; L221-224 `res = NewFlexMap(om)` re-wraps the fresh `OrderedMap` under a FlexMap header (a flex source yields a NEW flex container here — plain-arg param binding never reaches this because `RetagTypedContainerValue` handles flex first); L225 tag on fresh | explicit-copy (carrier copy; reparent; rebuild) | reparent: yes (as unify_list L147); rebuild: by contract | the source map's values | mint element headers from parts |
| 235-258 | `unifyFieldBags` — fresh `result` | explicit-copy (container rebuild) | no | — | construction |
| 262-268 | `unifyRecordTypes` — `NewRecordType(result)` | construction | — | — | — |
| 291-345 | `unifyRecordSchemaCarrierVsMap` — returns `rec` (operand); L330 `absentVal := NewTypeLiteral(TAbsent)` | by-value-flow | no | — | — |

### unify_negation.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 22-69 | `unifyNegation` — L26 `(&val).Equal(TAny)` orphan; L27/L47/L66 `NewNegation(neg.Inner)` fresh header around a COPY of `Inner`; returns `val` | by-value-flow + explicit-copy (Inner copied into each fresh negation) | no | — | `Inner *Value` |
| 84-88, 133-139 | `NegationUnifier{inner Value}` — declaration copy in the Behavior | by-value-flow | — | the body's `NegationInfo.Inner`, `tmeta.Body`, the `DefEntry` body | pointer |
| 105, 127 | `NegationInfo{Inner: n.inner}` re-wrap per call | explicit-copy (payload struct) | no | — | — |

### unify_options.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 8-69 | `unifyOptionsFamily` — L29 `NewOptionsType(opts.Fields)` shares the Fields map; **L59 `result.Set(key, defVal)` places the schema's DEFAULT VALUE into the unified map by value WITHOUT `FreshenDefault`** (contrast L162); returns a fresh `NewMap(result)` | by-value-flow (a mutable default aliased across calls — pre-existing hazard, header-level too under the policy) | unsure (inconsistent with `FillConcreteOptionDefaults`) | the Options schema | decide: freshen or share deliberately |
| 87-110 | `optionsDefault` — returns `alt`/`v` (shares); L93 `NewTypeLiteral(TNone)` as the default VALUE for an optional field | explicit-copy (duality as data) | L93: yes (§1.2.1) | TNone | occurrence cell / `none` value |
| 128-175 | `FillConcreteOptionDefaults` — L150-155 copies existing entries into `filled` (fresh map, elements by value), L162 `FreshenDefault(def)` (per-call default independence), L171-174 `NewFlexMap(filled)`/`NewMap(filled)` | explicit-copy (OrderedMap + FreshenDefault) | yes for the freshen ("injecting the schema's exact Value would let one caller's `set` leak into later calls and into the schema itself") | the schema default | container construction + minted headers |
| 184-202 | `unifyOptionsField` — returns `cVal`/operand | by-value-flow | no | — | — |

### unify_predicate.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 24-29, 137-144 | `PredicateUnifier{registry *Registry; constraint Value}` — the predicate fn VALUE copied into the Behavior (its `Signatures` backing array is what `resolvePredicateRef`/`sameFnConstruction` (unify.go L170-240) use to find the type for a body copy) | by-value-flow (declaration copy; identity through the shared slice) | — | the `DefEntry` body (the same fn value) | pointer; identity by `*fnIdent` or pointer |
| 51-85 | `Match` → `p.registry.RunPredicate(p.constraint, v)` (registry.go; snapshots the TypeTable per call — util.go L391-409) | by-value-flow (perf) | no | — | — |
| 99-106 | `Unify` — returns `RunPredicate`'s output (a value-transforming predicate returns a NEW value; typed_bind L44 then reparents it) | by-value-flow | no | — | — |

### unify_refine.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 24-53 | `bareRefineUnifier` — no Value copies; `Match` reads `v.Parent` (nominal) | — | — | — | — |
### canon.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 66-72, 75-335 | `Canon`/`CanonValue` — pure render; L223 `lst.Slice()` (read), L284-299 reads alternatives, L322 `fd, _ := v.Data.(FnDefInfo)` payload copy (read) | explicit-copy (read-only slices/payloads) | no | — | views |
| 225-227, 645-646 | canon renders `v.Quoted` as `(quote …)` / `(codequote …)` — the `Quoted` header flag is USER-VISIBLE output | by-value-flow (evidence: any in-place `Quoted` write — core_helpers L500/L561, macro_expand L172 — would change `print`/canon of the caller's value) | — | — | — |
| 656-704 | `canonFnDef` — `sigs := fd.OwnSigs()` (may copy), `sig := &sigs[i]`, `sig.Body()` | explicit-copy (read) | no | — | view |
| 773-798 | `canonSugar` — `l.Slice()[0]` | explicit-copy (read) | no | — | view |

### carrier_body.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 104-217 (147-148) | `runCarrierBody` — `tokens := make([]Value, elems.Len()); copy(tokens, elems.Slice())` (a copy of a copy) before `sub.Run(tokens)` | explicit-copy (slice ×2) | YES (§1.2.2): the body list is the user's branch/loop literal, re-run by every analysis round and by the runtime; the sub-engine's tape mutates cells | the parsed program; every re-analysis | tokens immutable + tape writes off-Value; then a pointer slice (no element copies) |
| 206-215 | `adds[k] = top` — the top binding of every grown name is captured by value, then `r.Defs.Truncate(k, before)` pops it; `InstallJoinedDefs` re-pushes the joined form | by-value-flow (a snapshot of a POPPED binding) | no — under pointers the instance simply outlives the pop | — | `map[string]*Value` |

### carrier_join.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 28-42 | `CommonAncestorType` — `seen map[*Type]bool` keyed by POINTER; fed orphans from `TypeNodeOf` at L193-195 | by-value-flow (latent imprecision CAUSED by node copies: an orphan never matches the canonical chain, so the walk over-widens for a subtype/supertype literal pair when the cap triggers) | — | — | the pointer policy fixes this for free |
| 53-70 | `FlattenAlternatives` — returns `[]Value{v}` (the literal copy) or `NewTypeLiteral(v.Parent)` | explicit-copy (duality) | no (read) | — | pointers |
| 97-115 | `JoinCarriers` — `a = ValueCarrier(a)` (fresh); `out := JoinCarriersInner(a, b)`; L111-112 `out.Carrier = true; out.Dynamic = true` on the fresh join | construction | — | — | — |
| 126-202 (145-153) | `JoinCarriersInner` — same-Parent collapse: `out := a; out.Carrier = true; out.Data = nil; out.ID = GenerateID(..)` — "the merged carrier is a NEW value (an if/loop result), not arm a. Keeping a's ID lets the result COLLIDE with a's own binding … Mint a distinct identity" | explicit-copy (copy-to-modify) | yes: `a` is an arm's residual / a live local's value; in place it would strip the payload of the binding | the arm's binding | the code already wants a mint: `NewCarrier(a.Parent)` + copy of pos/Dynamic (construction) |
| 186-188 | `combined := append([]Value(nil), FlattenAlternatives(a)...)` | explicit-copy (slice) | no | — | pointer slice |
| 199-201 | `v := NewDisjunct(alts); v.Carrier = true` | construction | — | — | — |
| 218-224 | `joinBranchDef` — `out.ID = a.ID` on the fresh join when the arms only NARROWED (identity preserved by ID string) | construction + by-value-flow (identity = ID) | yes (the compile seat of the binding stays reachable) | — | under pointers, "same identity" would mean returning the binding's own instance — but the join is a different (widened) value, so the ID-sharing trick becomes a pointer-level contradiction; needs the provenance model keyed off the binding, not the value |
| 239-245 | `narrowedSameBinding` — `pre.ID == v.ID` | by-value-flow (identity by ID) | — | — | pointer compare |
| 314-391 | `installJoinedDefs` — `BranchJoin{Pre, Joined Value}` snapshots (`j.Pre, j.HasPre = r.Defs.Top(k)`), pushes `j.Joined` | by-value-flow (recorder snapshot) | no | — | pointers |
| 398-407 | `condBoundCarrier` — `JoinCarriers(v, v)` mints a payload-less carrier with a fresh ID via the L145 copy path | construction (through a copy idiom) | no | — | `NewCarrier(v.Parent)` |
| 412-454 | `JoinCarrierStacks` — `out := make([]Value, n)`; `joined.Carrier/Dynamic = true` on the fresh join (L448-449) | construction | — | — | — |

### carrier_new.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 22-28 | `NewCarrier(t)` — fresh; L26 `ChildTypeInfo{Child: Value{Parent: TAny, Carrier: true}}` hand-built ID-less child | construction + by-value-flow (ID-less sentinel literal) | — | — | — |
| 47-50 | `CarrierOfLiteral(lit)` — `lt := lit; return NewCarrier(&lt)` — the carrier's Parent is the ADDRESS OF A LOCAL COPY of the node (an orphan `*Type` that escapes to the heap) | explicit-copy + `&v` orphan | no — it exists ONLY because the literal is by-value; downstream `CanonicalType` repairs it | — | `NewCarrier(lit)` with the canonical pointer |
| 65-75 | `ValueCarrier(v)` — `c := NewCarrier(v.Parent); c.Dynamic = v.Dynamic` | construction | — | — | — |
| 80-84 | `NewDynamicCarrierValue(bound)` — `bound.Carrier = true; bound.Dynamic = true; return bound` | explicit-copy (copy-to-modify of the param) | unsure: the in-scope caller (native_storage L1449) passes a fresh `CloneValue`; other lang callers unknown | the caller's bound | in place when the caller owns the value |
| 92-106 | `NewCarrierTypedList(elem)` / `NewCarrierTypedListValue(child)` — `v := NewTypedList(child); v.Carrier = true` (the child carrier is copied INTO `ChildTypeInfo.Child`) | construction (+ by-value child) | — | — | `Child *Value` |
| 117-130 | `CarrierTypedListOf` — fresh | construction | — | — | — |
| 140-151 | `UnionCarrierForType` — `dv := NewDisjunct(SimplifyDisjunctAlts(du.Alternatives)); dv.Carrier = true` | construction (new slice over the unifier's alts) | — | — | — |
| 170-190 | `ReturnsIdentity` — `v := args[m]` ("struct copy: the ID write below is local to v"); `v.ID = GenerateID(..)` for a DUPLICATED source index (`dup`/`over`) so each output has its own emit identity | explicit-copy (copy-to-modify) | yes: the source arg's own provenance must be untouched; the N outputs must be distinct identities | the source carrier (an arg / binding) | mint N fresh carriers from v's parts |

### carrier_spread.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 24-27 | `SpreadPayload{Elem Value}` | by-value-flow | — | — | `Elem *Value` |
| 36-41 | `NewVariadicCarrier` — fresh | construction | — | — | — |
| 60-89 | `FoldVariadicArms` — `elems = append(elems, e)` / `NewTypeLiteral(v.Parent)` (L68, L75); `elem := NewTypeLiteral(TNever)` (L81); `UnionType(elem, e)` (core_boolean) builds new | explicit-copy (duality; slices) | no | — | pointers |

### guard_narrow.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 45-148 | `extractGuardClauses` — L54 `elems = gf.Toks` (shared); L60 `list.Slice()`; L92-104 `tv := elems[i+2]`, `tv = e.Body` (def-table body copy), `tv = NewTypeLiteral(t)`; **L124-127 `gt := tv; guardType = &gt`** — an ORPHAN pointer to a local copy stored in `GuardClause.Type`, later used as a carrier Parent (`NewCarrier(c.Type)` L220) and in `TandValues(cur, NewTypeLiteral(c.Type))` / `negateTypeResolved(NewTypeLiteral(c.Type))` (L241, L370); only the DepScalar arm canonicalises (L123) | explicit-copy (duality) + `&v` orphan | no — artefacts of by-value literals; the orphan Parent on the narrowed carrier is exactly the hazard eng/go/CLAUDE.md warns about | — | canonical pointers end the orphan class |
| 209-308 | `ApplyGuardNarrowing` — L220 `narrowed := NewCarrier(c.Type)` fresh; L229-230 `narrowed.ID = cur.ID` (scoped SHADOW keeps the binding's ID: "is-narrowing is static-only … the narrowed carrier must keep the source's value ID"); L241 `meet := TandValues(cur, NewTypeLiteral(c.Type))` — `TandValues` may return an OPERAND or one of `cur`'s own alternatives (core_boolean L140-176) — then L245-250 `narrowed = NewCarrier(ValueType(meet)); narrowed.ID = cur.ID` or `meet.Carrier = true; narrowed = meet; narrowed.ID = cur.ID`; L293 `r.Defs.Push(c.Name, narrowed)`, popped by the restore closure (L300-307) | explicit-copy (copy-to-modify of a value that may be the live binding or a sub-value of its disjunct) + identity-by-ID | yes: a then-arm shadow must not retag the enclosing binding (restored after the arm; re-analysed per Kleene round) and must not flip `Carrier` on an alternative stored inside `cur`'s `DisjunctInfo` | `cur` (the binding), its alternatives slice | shadows stay distinct instances (mint from parts); the ID-sharing trick needs a provenance key other than the value |
| 316-420 | `ApplyComplementNarrowing` — L370 `negateTypeResolved(NewTypeLiteral(c.Type))`; L371 `narrowed := TandValues(cur, complement)` (may return `cur` itself or an alternative); L385-389 `narrowed = NewCarrier(ValueType(narrowed))` or `narrowed.Carrier = true`; L390 `ValuesEqual(narrowed, cur)` early-out; L400 `narrowed.ID = cur.ID`; push/pop | explicit-copy (copy-to-modify; as above) | yes (as above) | `cur` and its alternatives | as above |

### guard_predicate.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 20-56 | `predicateImpliedType` — `sig = &fn.Signatures[i]` pointer into the `Lookup` aggregate; reads `boru.Body` | by-value-flow (read) | no | — | — |
| 60-102 | `predicateBodyImpliedType` — `toks = []Value{toks[0], inner[0], …}` normalised copy (L73); reads | explicit-copy (read slice) | no | — | pointer slice |
| 109-134 | `guardTripleType` — `tv = e.Body` (def body copy), `tv = NewTypeLiteral(t)`, `tv.Is(TNone)`, `CanonicalType(r, ValueType(tv))` (orphan repaired — the good citizen) | explicit-copy (duality) + `&v` repaired | no | — | canonical pointer |
| 155-164 | `stripGuardMarkers` — fresh filtered slice | explicit-copy (read) | no | — | pointer slice |

### deadsig.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 11-15 | `DeadSig{Sig, ShadowedBy Signature}` — Signature struct copies in the diagnostic | explicit-copy (Signature, Value-bearing) | no | — | pointers into the sorted copy |
| 83-107 (87-89) | `DeadSignatures` — `sorted := make([]Signature, len(sigs)); copy(sorted, sigs); SortSignatures(sorted)` before scanning | explicit-copy (Signature slice) | yes: the input is the fn's own `Signatures` (authored order is user-visible through canon's `OwnSigs` render and is the dispatch order the matcher uses); sorting in place would reorder it | the fn value's signatures | sort an index/pointer slice |

### record_typed_def.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 39-62 | `RecordTypedDefMake` — `t := CanonicalType(r, ValueType(typeArg))` (orphan repaired); `carrier := NewCarrier(t)` fresh; `es.RecordCall(.., []Value{typeArg, body}, []Value{carrier}, ..)` operand copies to the recorder (keyed by ID) | construction + by-value-flow (recorder snapshot) | no | — | pointers |
| 66-77 | `typedDefMakeSig(sig, name)` — `wrapped := *sig` (Signature struct copy; `Patterns`/maps shared) with a wrapping `Impl` that prefixes `def <name>:` to make's error | explicit-copy (Signature) | yes: make's registered signature must keep its own handler; the per-def wrapper is a distinct dispatch object | the registered `make` sig | a wrapping Signature is construction; only the `*sig` struct-copy idiom is a copy |
| 83-96 | `objectMakeSig` — pointer into the aggregate | by-value-flow | no | — | — |

### typed_bind.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 20-103 (31, 44, 65, 70, 79, 94) | `RunTypedBind` — L31 `r.RunPredicate(*spec.Cons, v)` deref copy of the compiled spec's constraint (read); **L44 `out = ReparentValue(out, CanonicalType(r, spec.Def))`**, **L70 `ReparentValue(v, def)`** ("reparent a COPY of the value to the newtype"); L65 `Unify(v, NewTypeLiteral(root))`; L79/L94 return the Unify result (v itself or a rebuild) | explicit-copy (reparent ×2; duality) | reparent: yes — `v` is a runtime value the program may hold elsewhere (`def x 5  def y:Pos x` must leave x an Integer) | x's binding / the VM stack cell | mint from parts (`NewValueRaw(def, v.Data)` + flags) |
| 108-114 | `RunTypedBindCons` — `s := *spec; s.Cons = &cons; s.Describe = cons.String()` | explicit-copy (spec struct + `&cons` heap copy of the popped constraint) | yes: `spec` is the compiled `Program`'s `TypedBindSpec`, shared across runs; writing the run's constraint into it would leak run state into the program | the compiled program | pass `cons` as a parameter to `RunTypedBind` instead of copying the spec |
### compare.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 57-108 | `compareValuesClassified` — `aType := ValueType(a)` orphans; `nodeFamily` folds | by-value-flow (`&v`) | no | — | canonical pointers |
| 140-166 | `lowestCommonAncestor` — depth-aligned walk from (possibly orphan) nodes, `Equal` by ID ("a type literal is a by-value copy of its node, so ValueType() may hand us a copy whose address differs") | by-value-flow | no | — | pointer compare suffices |
| 219-275 | `scalarFamilyEqual` — reads | — | — | — | — |
| 276-293 | `scalarSemanticEqual` — **L278 `a.Parent == b.Parent`** POINTER gate before the Behavior dispatch: a value whose Parent is an orphan copy (CarrierOfLiteral carriers, a `&targetVal` reparent target when `CanonicalType` returns nil) fails the gate and falls to the LCA walk | by-value-flow (pointer identity leaking through copies) | — | — | exact under canonical pointers |
| 296-416 | `ExactEqual` — identity semantics: fn by `*fnIdent` (L336-341, L1195-1202), type bodies structural (L344-346), scalars by value, containers by `sameContainer` (L360-368), flat instances by `Fields` pointer (L377-381), handles by payload pointer (L780-808) | by-value-flow (identity lives in payloads, never in the header) | — | — | §1.5: the header could become the identity |
| 418-432 | `SameContainer` — exported identity test | by-value-flow | — | — | — |
| 445-460 | `HasContainerIdentity` — doc: "check mode's values are copies by construction (a container read hands back a CloneValue so the emitter's operand-provenance tracking gets a fresh ID), so it can never model runtime container identity" | by-value-flow (the check pass DEPENDS on copies with fresh IDs) | yes (§1.2.4) | — | provenance keyed by pointer |
| 463-520 | `sameContainer` — plain List identity = `&av.Elems[0] == &bv.Elems[0]` (empty lists are "the single empty list", L475-478); Map = `*OrderedMap`; flex/weak = payload pointer; Xml = `Attr` pointer + `Cren` array (L503-513); "must not apply `==` to a Payload directly: ListPayload holds a slice" | by-value-flow (identity by backing array — two headers over one array are one list; a copied header is still `eq`) | — | — | if identity moves to `*Value`, `dup`/reads must return the same pointer and every fresh-identity site must mint |
| 524-554 | `deqListElems`/`deqMapEntries` — shared views (`rl.elems`) | by-value-flow (shared) | — | — | — |
| 556-778 | `DeepEqual` — structural; L607/L629 render fallback `a.String() == b.String()` | — | — | — | — |
| 780-808, 898-920 | `opaqueIdealExactEqual`/`opaqueIdealDeepEqual` — Store/Timeout/Interval by pointer (`av == bv`); `ErrorInfo` by fields; `WordInfo` by Go `==` on the payload struct (L805, L914) | by-value-flow (payload struct equality) | — | — | — |
| 862-874 | `hostPayloadIdentity` — `a.Body == bp.Body` pointer | by-value-flow | — | — | — |
| 932-960 | `storeDeepEqual` — `a.VisibleEntries(), b.VisibleEntries()` snapshots per comparison | explicit-copy (snapshot; perf) | no | — | iterate layers |
| 1166-1202 | `closureIdentSeqOf`/`sameFnIdentity`/`FnIdentityKey` — identity via the `*fnIdent` pointer "copied by every derivation that keeps the function the same function" | by-value-flow (pointer identity one level below the header) | — | — | the model the policy wants, already present for fns |
| 1232-1268 | `fnStructurallyEqual`/`capturedBindingsEqual` — canon + `DeepEqual` over `Captured[i].Value` | by-value-flow | — | — | — |

### compare_deqkey.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 63-322 | `DeqKey`/`deqKeyAtDepth` — string keys; `keys := append([]string(nil), m.Keys()...)` (strings) | read | no | — | — |
| 416-485 | `DeqIndex{vals []Value}` — `x.vals = append(x.vals, v)` keeps copies of every added value for the pairwise confirm | by-value-flow (snapshot index) | no | — | `[]*Value` |

### compare_scalar_behaviors.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 56-66 | `litVsConcreteOrder` — raw `Data == nil && !Carrier` probes | by-value-flow | — | — | — |
| 76-78 | `litVsLitOrder` — `compareTypes(&a, &b)` ("the two values are by-value copies of their lattice nodes, so &a and &b ARE the nodes for compareTypes' purposes"); `compareTypes` L24 `a == b` pointer fast path misses for orphans, then Rank/depth/name/ID | by-value-flow (`&v` orphans; correct but slower) | no | — | pointer-equal fast path hits |
| 484-491 | `init` — `TNumber.ensureTMeta().Behavior = …` in-place tmeta writes on canonical nodes | by-value-flow (already in situ via shared tmeta) | — | — | — |
| rest | per-family `Compare(a, b Value)` by value; no copies | — | — | — | — |

### compare_types.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 23-37 | `compareTypes` — `if a == b` pointer fast path (orphans miss), then Rank/depth/name/ID | by-value-flow | no | — | — |
| 107-130 | `compareListView`/`compareMapView` — `AsMutableList` shared, weak list `lst.Slice()` (a swept snapshot) | explicit-copy (snapshot for weak) | yes for weak (sweep consistency) | — | — |
| 158-189 | `compareMapEntries` — `ak := append([]string(nil), am.Keys()...)` (strings) | read | no | — | — |

### equal.go (adjacent; read because it is the `==` gate)

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 72-83 | `ValuesEqual` — **L78 `a.Parent == b.Parent`** pointer gate for pluggable `Behavior.Equal`: an orphan Parent on either side skips the type's own equality and falls to the default switch | by-value-flow (pointer-vs-copy ambiguity decides which equality runs) | — | — | exact under canonical pointers |
| 90-98 | `valuesEqualDefault` — carriers "conservatively equal"; two literals `a.Equal(&b)` (orphan, ID compare) | by-value-flow | no | — | — |
| 182-186, 222-226 | `ChildTypeInfo.Child` compared by `Child.Parent.Equal` + `ValuesEqual(aCT.Child, bCT.Child)` | by-value-flow | — | — | — |
| 190-196, 230-236 | `*StoreShapeInfo` pointer identity ("ONE shape is minted per container and every carrier copy aliases it … the pointer IS the identity") | by-value-flow (the aliasing model the policy generalises) | — | — | — |
| 197-204, 238-248 | `fmt.Sprintf("%v", a.Data) == fmt.Sprintf("%v", b.Data)` render fallbacks copy payloads into fmt | explicit-copy (render) | no | — | — |

### typetable.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 163-183 | `labelIntervals` — writes `In`/`Out` through `ensureTMeta()` on the canonical nodes (shared by every copy) | by-value-flow (in-situ metadata via shared tmeta) | — | — | — |
| 337-348 | `Adopt(t)` — registers an EXISTING pointer; "a same-ID re-adoption keeps the first pointer (canonical identity)" | pointer-preserving | — | — | the model to extend |
| 371-389 | `MintType` — `def := &Type{…}` heap node, `tt.byID[def.ID] = def` | construction | — | — | — |
| 400-410 | `MintRefinePrefab`/`IsRefinePrefab` — the prefab reaches boru as `NewTypeLiteral(def)` (a copy); `InstallTypeBody` L565-575 recovers the canonical pointer via `LookupByID(body.ID)` and then writes `def.ensureTMeta().Name` AND `body.ensureTMeta().Name` (the second write is redundant: same tmeta) | by-value-flow (duality; evidence that tmeta already gives in-situ semantics) | — | — | the copy disappears |
| 476-572 | `RegisterType` — construction, `tt.byID[id] = def` | construction | — | — | — |
| 585-590 | `Retire(def)` — deletes from `byID`; values tagged with a retired node keep a valid Parent pointer (copies outlive the registry entry); `CanonicalType` then returns the orphan `t` | by-value-flow (dangling-but-valid pointers after `undef`) | — | every value whose Parent is the retired node | unchanged (pointer model already) |
| 596-626 | `idSnapshot`/`readmitRetired` — pointer maps | — | — | — | — |
| 629-660 | `Clone` — copies the maps, shares the `*Type` pointers ("defs are immutable once minted"), COPIES the mint counter (rollback sandbox) | explicit-copy (maps; perf per `RunPredicate`) | yes for the counter semantics (sandbox mints must not advance the parent's IDs) | — | — |
| 950-1010 | `registerBuiltin` — `def := &Type{…}` | construction | — | — | — |
| 1051-1121 | `MintTestType` — test-only | — | — | — | — |
| 1126-1137 | `mustBuiltinType` — degenerate `&Type{tmeta: …}` placeholder on init error | construction | — | — | — |
| 1148-1191 | `CloneDynamic` — map copies for a concurrent fork, pointers shared, counter SHARED | explicit-copy (maps) | yes (fork isolation of the index maps; shared node identity) | — | — |

### types.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 52 | `type Type = Value` | the duality | — | — | — |
| 113-137 | `TypeNames map[string]*Type` — canonical pointers; `resolveWordValue` wraps them as `NewTypeLiteral(t)` copies (resolve.go L50-52) | by-value-flow | — | — | hand out the pointer |
| 142-153 | `TypeNameByID` — by ID | — | — | — | — |
| 223-255, 262-267 | `ConformsTo`/`PathSubtype` — pointer receivers called on `&v` orphans (`v.ConformsTo(t)` in `defaultBehavior.Match`, typebehavior.go L241); `IsAncestor` (L129-146) uses In/Out labels or `Equal` (ID) — orphan-safe | by-value-flow | no | — | — |
| 306-351 | `Equal` — pointer fast path, shared-tmeta fast path, In-label fast path, then 14-char ID string compare — "a type literal is a by-value copy of its lattice node — same ID, different address — so a raw pointer compare would miss copies"; "~7% of interpreter CPU was this string compare" | by-value-flow (the cost of copies, measured) | — | — | `t == other` once literals are canonical pointers |

### type_run_install.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 62-89 | `RunTypeInstall` — `node := CanonicalType(r, spec.Node)`; `forwardType(node, target)` and `node.ensureTMeta().Name = …` mutate the pass-minted node IN PLACE (through shared tmeta) so every compiled reference (copies of the literal, sigs, typed-bind specs) follows the forward | by-value-flow (an existing in-situ mutation that works BECAUSE tmeta is pointer-shared; header copies of the node do not need updating) | — | — | the model to extend to the header |
| 93-100 | `forwardType` — writes `RunForward`/`Behavior` on tmeta | in situ | — | — | — |
| 104-112 | `ForwardedType` — pointer walk | — | — | — | — |
| 138-146 | `forwardedOperand` — `ForwardedType(&v)` orphan; `t != &v` compares against the orphan; `NewTypeLiteral(t)` copy of the run's node | by-value-flow (`&v`) + explicit-copy (duality) | no | — | pointer |
| 152-175 | `mintRunType` — `CanonicalType(r, &body)` orphan repair (L157); `t.SetTypeBody(body)` heap copy (L173) | by-value-flow + explicit-copy (declaration snapshot) | no under immutability | — | store the pointer |
| 184-199 | `runSigPattern` — `NewTypeLiteral(node)` returned as the inline sig pattern (stored via `&` into `FnParam.Pattern` by the sig builder) | explicit-copy (duality into a sig) | yes (§1.2.1 class) | the pass-minted node | pointer |

### typebehavior.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 75-95, 108-194 | `TypeBehavior{Match(v Value, t *Type); Format(v Value); Equal(a, b Value)}`, `Comparer`, `Unifier(a, b Value, r)` — every capability takes `Value` by value (104-byte copies per dispatch; the `t *Type` may be an orphan `&v`) | by-value-flow (interface contract) | — | — | `*Value` parameters — an API change for every Behavior in lang/basic/plugins |
| 195-216 | `HasConstraintUnify(t)` — called with `&a`/`&b`/`&t` orphans (unify.go L353-354, core_type L179); reads via shared tmeta so orphans answer correctly | by-value-flow | no | — | — |
| 237-244 | `defaultBehavior.Match` — `v.ConformsTo(t)` on the by-value param (addressable local) | by-value-flow | no | — | — |
| 275-299 | `behaviorWrapper{prev}` / `PrevBehavior` — no Values | — | — | — | — |

### core_type.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 54-80 | `PathOf(t)` — `start = &t` orphan (L60); `elems = append(elems, NewTypeLiteral(d))` (L76) → `NewList(elems)`: a USER-VISIBLE List of node copies | explicit-copy (duality as user data) | yes (§1.2.1: under the policy the list holds the canonical nodes; any header write through the list — pos stamping when an element is pushed onto the tape — mutates the lattice) | the lattice | occurrence cells |
| 94-99 | `TypeOf(v)` — `NewTypeLiteral(v.Parent)` returned to the program on every `typeof` | explicit-copy (duality as user data) | yes (as above) | the lattice | occurrence cells |
| 167-290 | `IsValueOfType` — L179 `HasConstraintUnify(&t)` orphan; L186-188 `t = content` (`TypeContentOf` → `TypeBody` copy) local; L280 `v.Is(&t)` passes the orphan as the type to `Behavior.Match` (Behaviors compare by Behavior identity / ID, so correct) | by-value-flow (`&v`) + explicit-copy (read) | no | — | pointers |
| 346-351 | `installTypeBinding` — `def.SetTypeBody(pushed)` (heap copy #1) + `r.Defs.PushType(name, def, pushed)` (copy #2 in the entry) | explicit-copy (declaration snapshots) | no under declaration immutability | the Behavior-held copy #3 (unify_*.go) | one instance, three pointers |
| 401-706 | `InstallTypeBody` — payload copies rewritten and RE-MINTED: Micron L419-427 (`info.Name = name; info.Type = def; NewValueRaw(def, info)`), class L440-447 (`info.Name`, `NewClassType(def, info)`), schema L465-474 (`info.Name`, `info.Type`), surface L480-488 (`info.Name`, `info.Type`) | explicit-copy (payload copy-to-modify + re-mint) | unsure: the source body is usually transient; a lowercase-bound body (`def c (class {…}); def Foo c`) would be renamed in place under the policy and render as `object<Class/Foo>` | the user's bound body value | move Name/Type onto the DefEntry/node; keep the body pristine |
| 546-549 | predicate branch: `named := fd; named.Name = name; compiledRuntime.StampDetached(r, named, …)` — a renamed payload COPY exists only so the stamp EVENT carries the type name while the bound payload's Name stays EMPTY (canon ordering, compare.tsv §predicate-kind) | explicit-copy (payload copy-to-modify) | yes: the binding's payload must keep `Name == ""` because `canonFnDef` renders predicate bodies and ordering depends on it | the bound predicate value | pass the name to `StampDetached` as a parameter |
| 565-575 | refine prefab: `def := r.Types.LookupByID(body.ID)` (recovering the canonical pointer from the copy's ID); `def.ensureTMeta().Name = name; body.ensureTMeta().Name = name` (redundant double write, same tmeta) | by-value-flow (duality; in-situ metadata already) | — | — | one write |
| 599-608, 620-634, 650-664 | disjunct / fnsig / negation / DepScalar branches: `installXUnifier(def, di.Alternatives / fu.Sigs / ni.Inner / di, name)` then `installTypeBinding(r, name, def, body)` | by-value-flow (declaration copy #3) | — | — | pointer |
| 665-675 | alias: `canon := CanonicalType(r, &body)` (orphan repaired); `r.Defs.PushTypeAdopted(name, canon, body)` — the entry holds the canonical pointer AND a by-value copy of the same node as its body | by-value-flow (duality in one entry) | no | — | one pointer |
| 676-692 | catch-all: `installBindingBodyUnifier(def, body, name)` (copy #3) + `installTypeBinding` | by-value-flow | — | — | pointer |

### util.go (adjacent; definition site of the named helpers)

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 26-45, 162-164 | `IsTypeLiteral`/`IsConcrete`/`IsBareTypeNode` — raw `Data == nil`/`Carrier` probes | by-value-flow (read) | — | — | — |
| 321-324 | `WithPos(v, src)` — `v.pos = src.pos; return v` ("the interpreter THREADS a position by copying the pointer") | explicit-copy (copy-to-modify) | yes whenever `v` is an existing shared value (a type literal — sugar.go L135; a const; a def body); no when `v` was just constructed (core_flex ×9, sugar ×5, macro) | the shared value | allowed only on fresh constructions; occurrence metadata for shared tokens must live in the tape cell |
| 326-332 | `WithPosAt(v, p)` — same, only when `v` has no position | explicit-copy | as above | — | as above |
| 348-351 | `ReparentValue(v, def)` — `v.Parent = def; return v`; "NEVER mutate Parent on a Value that was returned by Unify when Unify could have swapped to a type-literal side" — the copy is what makes the Unify-swap mistake unreachable | explicit-copy (copy-to-modify) — the sanctioned primitive | yes at all 5 in-scope call sites (plus lang's `defTypedHandler` ×3): the source is a live binding/element/stack value | the source value | `MintRetagged(v, def)`: `NewValueRaw(def, v.Data)` + Quoted/Eval/Carrier/pos/elem copy — construction from parts; the Unify-swap guard then has to be a check on `v` (not a type literal) rather than a copy |
| 369-377 | `CanonicalType(r, t)` — resolves an orphan `*Type` to `byID`'s pointer; falls back to `t` for ID-less/retired nodes | the orphan-repair helper | — | — | unnecessary once literals are canonical pointers (keep the ID-less fallback for ad-hoc types) |
| 391-409 | `snapshotPredicateState` — `r.Types.Clone()` + context snapshot + `analysisSnapshot` per `RunPredicate` | explicit-copy (maps; perf) | yes (sandbox rollback semantics) | — | — |
| 480-485 | `FlattenDisjunctAlts` — returns the disjunct's `Alternatives` slice ITSELF or `[]Value{v}` | by-value-flow (shared slice handed to callers that `append`) | — | the disjunct | pointer slice (same aliasing caveat) |

### resolve.go (adjacent; the prepass every `Unify` runs)

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 33-54 | `resolveWordValue` — returns `v`, a def-table body (`tv`, by value), `NewTypeLiteral(t)` (node copy) or `NewAtom(name)` | explicit-copy (duality) / by-value-flow | no | — | pointers |
| 75-87 | lists: `lst.Slice()` + `resolved := make(...)` + `NewList(resolved)` — rebuilt even when nothing resolved | explicit-copy (container rebuild; perf) | yes as written (the operand must not be rewritten — see unify.go L28-40 row); in place would canonicalise once but is order-dependent across armed/unarmed callers | the pattern / body / user container | resolve at construction time; drop the prepass |
| 96-113 | disjuncts: `alts := make(...)`, `changed` short-circuit (L105-107, via `ExactEqual`), else `nd := d; nd.Alternatives = SimplifyDisjunctAlts(alts); out := v; out.Data = nd; return out` | explicit-copy (payload + header copy-to-modify) | yes as written (a stored `{a?:T}` disjunct pattern keeps its authored alternatives) | sig patterns, def bodies | as above |
| 121-143 | typed list/map: `NewTypedList(child)` / `NewTypedListWithElements(child, elems)` / `NewTypedMapWithEntries(child, entries)` rebuilds (elements/entries preserved and resolved deep) | explicit-copy (container rebuild) | yes as written | — | as above |
| 145-153 | maps: fresh `OrderedMap` → `NewMap` | explicit-copy (container rebuild; perf) | yes as written | — | as above |

## 3. Closing notes for the refactor plan

- The policy as stated covers the `Value` header. Three other copy layers exist and are NOT forbidden by it, but the inventory above shows where they interlock: (a) payload STRUCT copies on every `v.Data.(X)` assert and re-box (`FnDefInfo`, `ClassTypeInfo`, `DisjunctInfo`, `ListPayload`), (b) container copies (`[]Value`, `*OrderedMap`) that implement plain Map/List copy-returning `set` and the flex/never-mixed invariants (core_flex.go), (c) `Signature`/`FnSig` struct copies (compileFnSigs, ExpandOptionalSigs, deadsig, record_typed_def). Converting `Value` to a pointer while leaving (a) by value keeps every "copy the payload, change Name/Type/ident, re-box" site; converting (a) to pointer payloads removes them but makes payload mutation shared too.
- The sites that would behave IDENTICALLY in place (safe first moves): `Value.String`'s `inner := v` (L4193), `MarkPredicateFn`, `WithModuleNS`, `NewDynamicCarrierValue` on freshly cloned input, every `WithPos`/`SetElemConstraint`/flag write on a freshly constructed value, the read-only `Slice()` consumers, `AllFields`/`FlatInstanceFields` read views, `DeqIndex.vals`, `UnifyError.A/B`, `BranchJoin.Pre/Joined`, `carrier_body` `adds`.
- The sites that need a NEW CONSTRUCT before the policy can hold: (1) an occurrence cell (or tape-cell metadata) for type literals and for every re-run token sequence; (2) a per-binding attribute store (`DefEntry`) for `Quoted`, the registered Name, the typed-container tag on flex params, and macro-operand quoting; (3) a stack-cell/dispatch-plan slot for `as` ascriptions and `ClosurePayload` seam flags; (4) a provenance key that is not the value's ID for check-mode shadows and joins; (5) a `MintRetagged`/`MintCarrierFrom` constructor family so `ReparentValue`, `FreshenDefault`, `JoinCarriersInner`, `ReturnsIdentity`, `RetagFlexElem` and `cloner.withPayload` build from parts instead of starting from a copy.
