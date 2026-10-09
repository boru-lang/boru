# No-copy policy inventory — basic/go, lang/go, parser/go, cmd/go

Read-only review against the current tree (2026-10-09). Line numbers are from
`grep -n` / `sed -n` on the files as they stand. Scope: every non-test `.go`
file in `basic/go`, `lang/go` (native/, modules/, capabilities/, stackform/,
formatter/, root), `parser/go`, `cmd/go`. Eng/VM-side files were not
inventoried; a handful of core facts are cited because every judgment below
depends on them.

## Summary

### Core facts the judgments rest on (cited, not inventoried)

| Fact | Evidence |
|---|---|
| `ReadList.Slice()` **always allocates and copies** the element slice (shallow struct copies of each `Value`). | core/go/value.go:51-55 |
| `WithPos`, `ReparentValue`, `StripAscribed` are parameter-copy-then-return helpers; `SetAscribed`, `SetPos`, `SetElemConstraint` mutate through a pointer receiver (in place on whatever variable they are called on). | core/go/util.go:321-350, core/go/value.go:2260-2272 |
| `NewValueRaw(t, data)` mints a fresh struct; it is construction, not copying. | core/go/value.go:2471-2480 |
| `DefTable.Push` stores `DefEntry{Body: v}` — a **struct copy** of the bound Value into the name's stack; `Top` returns a copy out. A `ListPayload{Elems}` copy shares the backing array; a `MapPayload{M}` copy shares the `*OrderedMap`. | core/go/deftable.go:11-15, :113 |
| `Engine.Run(input)` **copies its input** into the tape (`Tape.Reload` / `NewTapeWith` copy; "the caller's slice is never mutated"); `Tape.Splice` copies the replacement run into the gap buffer. Loop re-iteration reads `cont.Body` and splices a copy ("no separate bodyCopy is needed"); `Loop.bodyTokens` deliberately returns the list's own `elems` without copying. | core/go/engine.go:1635 (lines 93-113 of the fn), core/go/tape.go:185-224, :416-440, core/go/engine.go:9080-9094, core/go/loop.go:312-321 |
| Captures are `CapturedBinding{Name, Value}` — a struct snapshot taken at fn construction; installed per call through `InstallFrameBinding` → `DefTable.Push` (another copy). | core/go/value.go:1192-1195, core/go/fn_capture.go:322,434, core/go/core_helpers.go:37-39, :491, :549 |
| `SetAtomReferent` stores `&snap` where `snap := ref` — a pointer to a private copy of the referent. | core/go/value.go:2761-2770 |
| `installDef` does `fnDef, ok := body.Data.(FnDefInfo); fnDef.Name = name` — mutates a copy of the (struct, interface-boxed) payload. | core/go/core_helpers.go, installDef body |
| `core.ParseFnDef` stores a fn body as `Boru(bodyElems)` where `bodyElems = _lst.Slice()` — a copy of the parsed list's elements. | core/go/fn_def.go:50 (+22-41) |
| The trace callback receives `e.Tape.Snapshot()` — a fresh copy per fire. | core/go/engine.go:474, :1818-1821 |

### Counts of explicit copy sites (non-test files)

"Explicit" = a `Value` or `[]Value` duplicated so the duplicate can be changed,
stored, or executed separately. Constructions from parts (`NewValueRaw`,
`NewList(fresh)`, `NewWord`) are **not** counted; they are what the policy
wants. Handler `named map[string]Value` signature parameters (534 textual
hits) are also not counted — they are by-value flows, listed once below.

| Module | `ReadList.Slice()` copies | `make`+`copy` / `append([]Value…)` slice copies | `ReparentValue` / `WithPos` / `WithPosAt` re-mints | `CloneValue` | Param/local struct copy then field write (`v := args[i]; v.X = …`, `SetAscribed`/`SetElemConstraint` on a copy) | Total explicit |
|---|---|---|---|---|---|---|
| basic/go | 35 | 19 | 12 (7 `ReparentValue`, 5 `WithPos`) | 0 | 1 (`conditional.go:480`) | **67** |
| lang/go/native + stackform | ~50 | ~31 | 4 (`ReparentValue` ×3, `WithPos` ×1) | 6 | ~13 | **~104** |
| lang/go/modules | 24 | 8 | 0 | 0 | 5 | **37** |
| lang/go root (boru.go) | 0 | 0 (engine copies, in core) | 0 | 0 | 0 — but the parsed `values` slice is **reused across two compile passes** and retained by the recorder | 1 reuse site |
| lang/go/capabilities, tuikit, formatter | 0 (`copy(` hits are `[]byte`/cells/strings) | 0 | 0 | 0 | 0 | **0** |
| parser/go | 0 | 0 | 1 (`withPos`, on a value the parser just minted) | 0 | 3 (`mv.Eval = true` on fresh locals) | **4**, none of them re-mints an existing Value |
| cmd/go | 0 | 2 (`resolvedData` filter copy; `exec.go` boxing copy) | 0 | 0 | 0 | **2** + 4 snapshot-retaining fields |

Roughly **215 explicit sites**. By kind: ~110 `Slice()` copies (the single
largest family; about half are read-only scans that need no copy at all and
would be trivially replaced by a borrowed view), ~60 slice rebuilds (the
immutable list words and the "copy a body before `Run`" idiom), ~17
re-mints (`ReparentValue`/`WithPos`), 6 deep clones, ~22 param-copy-then-flag
sites.

### Answers to the aliasing / semantics questions (with evidence)

1. **`def a [1 2 3]  def b a  b set 0 9  a` → `a` is observed as `[1 2 3]`.**
   `set` on a plain List is copy-returning: `setListHandler` builds
   `out := make([]Value, n)`, copies the elements, writes `out[idx] = val`,
   and returns `NewList(out)` (lang/go/native/native_storage.go:2078-2116;
   the column rule is documented at :82-85 "List (immutable — copy-returning,
   completing the column rule: Map and List both return the updated copy)").
   `b` is also unchanged — the updated list is only on the stack. The
   **binding does alias**: `def b a` stores a struct copy of `a`'s Value
   (DefTable.Push) whose `ListPayload.Elems` slice header points at the
   SAME backing array as `a`'s. Nothing writes into that array today (every
   List word rebuilds), so the aliasing is unobservable. Under in-place
   mutation of elements (or an in-place `set`), `a` and `b` would both
   change.

2. **Maps: same answer, same mechanism.** `setMapHandler` builds a fresh
   `OrderedMap`, copies every key, sets the new one, returns `NewMap(out)`
   (native_storage.go:734-765). The binding copy shares the `*OrderedMap`
   pointer, so an in-place `OrderedMap.Set` would alias `a` and `b`.

3. **FlexList / FlexMap / Store / Class instance: in place, aliasing by
   design.** `setFlexListHandler` / `setFlexMapHandler` (:1258, :1102)
   write through the pointer payload; `push`/`pop`/`shift`/`unshift` have
   Flex overloads that "mutate IN PLACE … returning the same node"
   (natives.go:293-298, listops.go:124-192). Store `set` is copy-on-write
   through `CowSet` (native_storage.go:1879-1887 → core_helpers.go:1004).
   `def b (flex a)` then `b set 0 9` IS visible through `a`'s flex handle.

4. **`clone`** is `CloneValue(args[0])` (lang/go/native/clone.go:19) — a
   deep, independent copy of mutable payloads; scalars/fns/type bodies are
   shared. `(clone p) eq p` is false (fresh ID). It is the one word whose
   entire semantics is copying; under the policy it would have to become a
   deep CONSTRUCTION (rebuild each container from its parts) — which is
   what `CloneValue` already does structurally; the policy's prohibition
   would have to carve out an explicit "construct an equal value" word.

5. **`def x y`** copies `y`'s Value struct into `DefEntry.Body`
   (DefHandler: `body := args[1]` native_definition.go:840 →
   `InstallAndRecordDef` :379 → `InstallDef` → `r.Defs.Push`). Today a
   later `def x:Foo y` reparent (`ReparentValue(body, def)`
   native_definition.go:1024/1540/1596/1660) or `x as T` ascription
   (`out := args[1]; out.SetAscribed(target)` native_type.go:753-754) acts
   on a COPY and `y`'s binding keeps its Parent/asc. Under one-instance
   in-place mutation, `def x:Foo y` would re-tag `y` too (`y typeof` → Foo),
   and `const y` (native_const.go:76/94) would re-tag `y`'s binding and every
   container element that shares the instance with the singleton type.

6. **`dup`** returns `[]Value{args[0], args[0]}` (basic/go/native_stack.go:141).
   The two tape cells are then independent struct copies (the engine splices
   results by copying). `over`, `tuck`, `dup2`, `over2`, `pick` do the same
   (:153, :165, :169, :181, :199). Under a pointer model the two cells would
   be the same instance; any word that mutates its operand in place (an
   in-place `set`, `as`, `quote`, `apply`, `ReparentValue`) would change both
   copies. Today only the pointer-backed payloads (Flex/Store/Array/Object)
   alias through `dup`, which is the documented behaviour.

7. **Captures** are struct snapshots (`CapturedBinding.Value`) taken at fn
   construction (basic `FnConstruct` → `core.ComputeFnValueCaptures`,
   native_definition.go:2008-2010). "Reassignments (`def n new-value`)
   don't affect captures" (lang/go/CLAUDE.md "Capture semantics: shallow
   snapshot") holds because `def` PUSHES a new entry rather than mutating
   the old Value; that would survive a pointer model. What would NOT survive:
   an in-place reparent/ascribe/quote of the outer binding after capture
   would be visible inside the closure, and a closure installed per call via
   `InstallFrameBinding` would hand the SAME instance to every call.

8. **`quote`**: `quoteWordHandler`/`quoteAnyHandler` do `v := args[0];
   v.Quoted = true` (natives.go:515-526) — a copy-to-modify of the arg; the
   atom's referent snapshot is `SetAtomReferent` (pointer to a private copy).
   In place, `quote x` would set `Quoted` on the binding's instance (every
   later read of `x` arrives quoted), and the referent would alias the
   binding.

### Top 5 risks of in-place mutation in these modules

1. **Code bodies are reused data.** Loop bodies (`for`/`while`/`each`/`fold`/
   `receive`/`spawn`/`watch` callbacks, behaviour bodies), `fn` bodies
   (`FnSig` Boru impl), `if`/`case` arms and `do` lists are `[]Value` token
   runs that the engine steps by REPLACING tape cells with results. Every
   handler here hands the engine a copy (or relies on `Engine.Run`/
   `Tape.Splice` copying). If tape cells became pointers to the list's own
   elements, the first iteration/call would overwrite the body with its
   results (`[1 add 2]` → `[3]`), breaking the second iteration, the next
   call of the fn, and every later `def`-splice. Sites: forloop.go:27-54,
   whileloop.go:24-48, native_control.go:803/835/1631/881, conditional.go:
   spliceArg, native_process.go:205/423/441/592, io_watch.go:124,
   native_temporal_await.go:362, native_behave.go:212/1016/1018,
   native_map_iter.go:75, walk_core.go:162, modules/test.go:304/357/500,
   modules/debug.go:349/389/440/563. These are load-bearing as a CLASS even
   though each individual handler copy is redundant with the engine's.

2. **Def bindings, container elements and stack cells share instances.**
   `def b a`, `m set k v`, `xs push v`, `dup`, `over`, `pick`, `stack N`,
   map-literal evaluation, `each` element delivery and fn-param binding all
   store the SAME Value in two places today by copying the struct. Any
   in-place mutation of a Value's own fields — `ReparentValue` at a typed def
   (native_definition.go:1024/1540/1596/1660), `const` (native_const.go:76/
   94), `as` (native_type.go:754), `quote` (natives.go:516/525), `apply`
   (native_valof.go:499), `appliedTransducer` (native_macro.go:680),
   `SetElemConstraint` on a result that is still the input's instance — would
   leak into the other holders. The typed-def reparent is the most visible:
   `def n 5  def p:Positive n  n typeof` would answer `Positive`.

3. **The check/compile pass runs over the same parsed tokens as the
   interpreter and may run twice.** `lang/go/boru.go:480-520` reuses
   `values` for a second `compilePass` after a call-site-specialisation
   retry, and `es.SetRootBody(values)` (:537) retains the slice so a
   `/q` claim can "resume the interpreter in them". The check pass strips
   literals to carriers (`analysisStripToCarriers`, core) and check-mode
   handlers flip `Carrier`/`Dynamic`/`Quoted` on their operands
   (conditional.go:480, native_macro.go:1166/1239, native_unpack.go:266,
   native_valof.go:476, native_array.go:1824). In place, the retry pass and
   the retained root body would see carrier-ified, flag-mutated tokens.

4. **Message passing and the debugger depend on independent snapshots.**
   `send` deep-clones the message (native_process.go:335) so a receiver
   can never observe the sender mutating a shared container — a process
   isolation guarantee, not an optimisation. The debugger's time-travel
   ring keeps one `Tape.Snapshot()` per visited line (debugger.go:66,
   :160, :440) and renders past stacks from it ("the snapshot is the truth,
   the registry is NOT rewound", :654); with shared instances, `back` would
   show the present.

5. **Flex self-reference and typed re-tagging rely on snapshotting before
   mutating.** `append f f` is well-defined only because `src.Slice()`
   snapshots before the grow (native_flex.go:318-323); `d2AdoptTyped`
   returns a `Unify`-rebuilt copy for nested typed containers but
   deliberately keeps the caller's FLEX instance and re-tags it in place
   (native_storage.go:560-590, "#9, Codex round 9") — the one place the
   codebase already chose in-place over copy, and it had to be special-cased
   because the copy had broken visibility through the original handle.
   The policy generalises that choice everywhere; every "copy-returning
   column" word (set/push/pop/shift/unshift/merge/setpath/sort/reverse/take/
   unique/create/listAll) then changes user-visible value semantics from
   "returns an updated copy, receiver untouched" to in-place.

### Copies that exist purely for performance (no semantic role)

- Every handler-side `make`+`copy` of a body immediately passed to
  `New(r).Run(...)`: `Engine.Run` copies again (core engine.go Run lines
  93-113). Sites: native_control.go:803, :835, :1631; native_definition.go:
  1054; native_type_gen.go:161; native_process.go:592; native_temporal_await.
  go:362; native_temporal_timeout.go:17-18; native_module_module.go:374;
  log_span.go:418; modules/debug.go:389/563; modules/debug_step.go:43;
  modules/test.go:304/357/500/226 (test_coverage). These are belt-and-braces
  today; deleting them changes nothing. (forloop.go:34/54 and whileloop.go:
  28-48 are the same pattern — core/go/engine.go:9082-9085 says so
  explicitly — but there the copy also feeds `Loop.Body`, which the engine
  re-splices.)
- `spliceArg` (conditional.go:6-20) exists to avoid a double copy; its own
  element-wise copy is required only because the paren markers are
  prepended/appended.
- `Loop.bodyTokens` (core) already skips the copy; basic's `RunForLoop`/
  `RunWhileLoop` still make three.
- Read-only `Slice()` scans (`for _, e := range lst.Slice()`) in query.go,
  native_misc.go, report.go, test_coverage.go, gex.go, native_string.go,
  native_object_record.go, debug.go:659/690, native_control.go:1682/1793,
  native_definition.go:1832, native_type_gen.go:129 — allocation only;
  a borrowed `Get(i)` loop is identical.
- `FnUtil.memoize` caches a copy of the result slice (fn.go:550) and
  `curryLevel` copies `bound` per level (fn.go:566-575) so sibling partials
  never share a backing array — these are defensive aliasing guards, not
  speed.

---

## Per-file tables

Columns: `line | site (function, one-line what) | kind | load-bearing? | what shares the original | replacement under the policy`.
"explicit-copy" = a copy made so it can be changed/stored/executed;
"by-value-flow" = a place where Value-by-value semantics is relied on
implicitly (a pointer conversion would hit it).

### basic/go/native_stack.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 141 | `dupHandler` returns `{args[0], args[0]}` | by-value-flow | yes | the two resulting tape cells; everything either cell is later stored into | A pointer model needs dup to CONSTRUCT a second value from the first's parts (payload-sharing for pointer-backed kinds is today's contract) — i.e. dup becomes the one sanctioned "re-mint from parts" |
| 153 | `overHandler` returns `args[1]` twice | by-value-flow | yes | same as dup | same |
| 165 | `tuckHandler` returns `args[0]` twice | by-value-flow | yes | same | same |
| 169 | `dup2Handler` returns `args[1], args[0]` twice | by-value-flow | yes | same | same |
| 181 | `over2Handler` returns `args[3], args[2]` twice | by-value-flow | yes | same | same |
| 199 | `pickHandler` `append(stack, stack[len-1-n])` | by-value-flow | yes | the picked cell and the new top | same |
| 189 | `depthCheckFullStack` `append(append([]Value(nil), stack...), carrier)` | explicit-copy | no (check-mode shape; the engine replaces the stack with the result) | the live check stack | return `append(stack, …)` after the engine guarantees it owns the slice; or construct the residual list |
| 203, 212 | `PickCheckFullStack` same idiom | explicit-copy | no | same | same |
| 233 | `RollCheckFullStack` `out := append([]Value(nil), stack...)` then `out[len-1] = carrier` | explicit-copy | unsure — the copy is written at index len-1; if the engine still reads `stack` after the call, in-place would clobber its top | the check-mode stack slice | write the carrier into a fresh residual rather than a copied stack |
| 211-219 | `rollHandler` rebuilds `result` from `stack` slices | by-value-flow (reorder, no copy of cells beyond the slice rebuild) | no | — | none needed |
| 1-120 | every handler takes `named map[string]Value` | by-value-flow | — | handler signature | pointer map |

### basic/go/native_definition.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 379-430 | `InstallAndRecordDef(r, name, value Value, …)`: `value = elem` (region split), then `InstallDef(r, name, value)` → `DefTable.Push` struct copy | by-value-flow | yes | the stack cell the def consumed, any container element the same Value came from, and the binding | the binding would hold the instance; a def would then have to be followed by nothing that mutates the instance (reparent/ascribe/quote) |
| 561 | `DefFormVia`: `ctorArgs = append([]Value(nil), ctorArgs...)` then `ctorArgs[i] = evaluated` | explicit-copy | yes | `args[offset:]` is a window onto the dispatcher's args slice, which the gen-chain re-dispatch reads again | construct a fresh operand slice from parts (already what it does once copied); cannot write into `args` in place without the dispatcher's consent |
| 840 | `DefHandler`: `body := args[1]` → `InstallAndRecordDef` | by-value-flow | yes | see 379 | — |
| 1024 | predicate typed def: `out = ReparentValue(out, def)` (copy with `Parent=def`) | explicit-copy (re-mint) | **yes** | `body` (= `args[1]`) is also the Value a `def n 5 … def p:Pos n` read from `n`'s binding; `out` becomes `p`'s binding | needs a value CONSTRUCTED as `(def, body.Data)` — `NewValueRaw(def, body.Data)` keeps the payload shared but mints a new identity (loses pos/flags, which `ReparentValue` keeps). In-place would re-tag `n` |
| 1054 | `evalParenAnnotation`: `copy(body, toks)` then `New(r).Run(body)` | explicit-copy | no (Engine.Run copies again) | the ParenExpr payload's `Toks` | drop the copy |
| 1094 | `defRunMembershipBind`: `bound := body`; `bound = unified` | by-value-flow | no | — | — |
| 1343 | `DefTypedHandler`: `body := args[1]` | by-value-flow | yes | the binding | — |
| 1540 | refine-subtype def: `ReparentValue(body, def)` passed as the bound value; comment at 1492-1500 explains WHY a copy ("reparent a COPY of the body … Mutating the Unify result would store its by-value type literal") | explicit-copy (re-mint) | **yes** | `body`'s originating binding/container; the `Unify` result may BE the type-literal side | as 1024; the Unify-swap hazard means the construction must take `body.Data`, never the unify result |
| 1596 | typed-container / general typed def: `unified = ReparentValue(unified, reparent)` | explicit-copy (re-mint) | yes | `unified` may be `body` itself (UnifyR returns its concrete side by value) → the source binding | construct from parts |
| 1653-1660 | `defNarrowedBodyBind`: `bound := body` … `bound = ReparentValue(bound, reparent)` (check-mode carrier) | explicit-copy (re-mint) | yes (check mode) | the carrier under analysis, which the recorder keys by ID | construct a fresh carrier |
| 1820 | `varHandler`: `body := elems.Slice()[1:]` | explicit-copy | no — the result is spliced by the engine (copied again) | the `var` spec list | borrow `elems` view; but the spliced tokens then ARE the list's elements (risk 1) |
| 1832 | `for _, decl := range decls.Slice()` | explicit-copy | no (read-only scan) | — | iterate with `Get(i)` |
| 1839, 1857, 1865, 1880 | `WithPos(NewWord("def"), decl)` etc. — pos threaded onto a freshly minted word | re-mint of a FRESH value | no | — | `w := NewWord(..); w.SetPos(decl.Pos())` — construction |
| 1858 | `result = append(result, declElems.Slice()[1:]...)` | explicit-copy | yes-as-class (risk 1): the declaration value tokens are spliced onto the tape and stepped; the `var` list literal is reusable (loop body, fn body) | the `var` declaration list's elements | tape must not alias list elements; construct tokens |
| 1909 | `FnHandler`: `elems := _lst.Slice()` → `FnConstruct` → `core.ParseFnDef` (which `Slice()`s each body again) | explicit-copy | **yes** | the `fn [...]` spec list literal — a fn body inside a loop/fn is re-constructed on every evaluation from the same literal | the FnSig body must be constructed from the literal, not alias it, OR the engine must never write into a body list (today it writes into tape COPIES) |
| 2166 | `AfnHandler`: `bodyElems = lst.Slice()` | explicit-copy | yes (same as 1909) | the lambda body literal | same |
| 2255, 2286 | `FnpredHandler` / `FnsigHandler`: `spec := _lst.Slice()` | explicit-copy | yes (bodies for fnpred; sig-only for fnsig → no) | the spec literal | same / drop |
| 2008-2010 | `FnConstruct`: `core.ComputeFnValueCaptures` snapshots captured bindings into `fnDef.Captured` | by-value-flow | yes | the outer fn's param/local bindings | captures would alias the outer bindings' instances |
| 2139 | `afn`: `body := args[0]` | by-value-flow | no | — | — |

### basic/go/native_control.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 408-412 | `doReturns`: `body := args[0]`; if word, `body = v` (binding read) | by-value-flow | no | — | — |
| 524, 710, 731 | `shuffleOnlyBody(bl.Slice(), r)`, `tokensMayRaise(bl.Slice(), r)` | explicit-copy | no (read-only scans) | — | borrow |
| 803 | `DoEvalList`: `copy(input, elems)` then `sub.Run(input)` | explicit-copy | no — `Engine.Run` copies | the `do` list literal | drop; the engine copy remains (core) |
| 831 | `doEvalDataList` stepless fast path: `return append([]Value(nil), elems...)` | explicit-copy | **yes** — the returned slice becomes the evaluated map entry's list (`do {a:[1 2]}` stores it); without the copy the stored list would alias the source literal's elements | the `do` map literal's inner list (reused every time the `do` runs) | construct the result list from parts (it IS a fresh list; the elements are shared scalars) |
| 835 | `doEvalDataList`: `copy(input, elems)` before `sub.Run` | explicit-copy | no | — | drop |
| 844 | `DoEvalMapValue`: `doEvalDataList(r, _lst.Slice())` | explicit-copy | no (callee copies / engine copies) | — | borrow |
| 881-884 | `ifCondTokens` (runtime if): `condSlice := _lst.Slice()`; `NewMark(id, condSlice...)` and `tokens = append(tokens, condSlice...)` — the mark STORES the cond tokens and the same tokens are spliced to run | explicit-copy | **yes** — the mark keeps `condSlice` as the re-mark template while the spliced copy is stepped; see also conditional.go:59-64 | the `if` cond list literal; the mark payload | the mark must hold a template that is never the tape's cells — construction per splice, or the engine must step a per-iteration construction |
| 1033-1035 | `If3ReturnsFn` (check): `v := args[1]; thenValue = &v; thenStk = []Value{v}` | by-value-flow (pointer to a local copy) | yes (check mode) | the arm operand; `&v` outlives the handler | would become `&args[1]` — the arg's own instance |
| 1069-1071 | same for the else arm | by-value-flow | yes | same | same |
| 1589 | `IfClauseHandler`: `ifClause(_lst.Slice(), …)` | explicit-copy | yes (feeds spliceArg/ifCondTokens — risk 1) | the clause list literal | construct |
| 1619 | `CaseHandler`: `CaseClauses(r, v, lst.Slice())` | explicit-copy | yes (clause bodies are run/spliced) | the case clause list | construct |
| 1631 | `caseSubject`: `copy(input, lst.Slice())` then `Run` | explicit-copy | no (double copy) | — | drop |
| 1653 | `ArmSpliceHandler`: `v := args[0]` returned | by-value-flow | no | — | — |
| 1682, 1793 | `armHoldsSteppingLiteral` / `plainElems`: `Slice()` scans | explicit-copy | no | — | borrow |
| 1828 | `ifClauseReturns`: `elems := _lst.Slice()` | explicit-copy | no (check-mode read) | — | borrow |
| 1907 | `ifClauseStaticReturns`: `elems := _lst.Slice()` | explicit-copy | no | — | borrow |
| 1952-1953 | synthesizes `nested := NewWord("if"); nested.SetPos(...)`; `rest := NewList(append([]Value(nil), elems[2:]...))` | SetPos on fresh (construction) + explicit-copy | copy: yes (the new list becomes a nested arm the check pass walks; must not alias the clause literal's tail) | the clause list | construct from parts (already a fresh list; element sharing is the question) |
| 2026, 2036 | `ForCountHandler`/`ForRangeHandler`: `body := args[1]` → `RunForLoop` | by-value-flow | yes (the body list goes into `Loop.Body` after copy — forloop.go) | the loop body literal | — |
| 2035 | `rangeSpec := _lst.Slice()` | explicit-copy | no (parsed to ints) | — | borrow |
| 2113, 2140 | `ParseRange(lst.Slice())` in check-mode for analysis | explicit-copy | no | — | borrow |
| 2325 | `doCatchBodyReturns`: `seeded := append([]Value{NewCarrier(TError)}, body.Slice()...)` → `RunCarrierBody(r, NewList(seeded))` | explicit-copy | yes (check-mode: the seeded body is run as a carrier body; the literal must stay unseeded) | the `error [...]` handler body literal | construct the seeded body from parts |
| whole file | 23 handlers take `named map[string]Value` | by-value-flow | — | — | pointer map |

### basic/go/conditional.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 6-20 | `spliceArg`: element-wise copy of a code-body list into `( … )` wrapped tokens (comment: avoids `Slice()` + `append` double copy) | explicit-copy | **yes** (risk 1): the result is spliced onto the tape and stepped; the arm literal re-runs on every `if` evaluation | the `if`/`for` branch list literal | tape cells must be constructed per splice |
| 45-64 | `ifClause`: `_lst.Slice()` for the cond; `NewMark(id, condSlice...)` stores the tokens in the mark payload AND splices them | explicit-copy | **yes** — the mark's stored copy is the re-mark template | the cond literal; the mark | as native_control.go:881 |
| 54 | `IfCont{Then: thenBranch, Else: elseBranch}` stored in the move marker | by-value-flow (slices of Values held in a marker payload) | yes | the arm token runs built by spliceArg | — |
| 126-134 | `caseNormalizeClauses`: `out = append(out, NewList(append([]Value(nil), rest...)))` — wraps the open-call default tail into a fresh list | explicit-copy | yes (the synthesized default body is run; the clause literal keeps its flat shape for the next evaluation) | the case clause list literal | construct |
| 331, 781 | `caseNormalizeClauses(r, lst.Slice())` | explicit-copy | yes (feeds the above) | same | construct |
| 480 | `caseStaticGuard(r, v, m Value)`: `v.Carrier = false` on the parameter copy | explicit-copy (param copy then field write) | **yes** (check mode): the scrutinee carrier on the check stack must stay a carrier; only the local probe is de-carriered | the check-stack scrutinee, the recorder's view of it | construct a concrete probe value from `v.Parent`/`v.Data` |
| 489 | `caseStaticGuard`: `toks := ml.Slice()` (foldable predicate run) | explicit-copy | no (run through a sub-engine) | — | borrow |
| 872, 883 | `caseGuardTokens` / `caseBlockTokens`: `append([]Value{v}, ml.Slice()...)` | explicit-copy | yes (the tokens are built into an `if` chain that is run) | the clause predicate/block literals | construct |
| 931 | `runCaseBody`: `RunResolved(r, []Value{v}, lst.Slice())` | explicit-copy | no (RunResolved → engine copies) | — | borrow |

### basic/go/whileloop.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 24-25 | `condSlice := condLst.Slice()`, `bodySlice := bodyLst.Slice()` | explicit-copy | no in isolation (28/30 copy again) | — | drop one layer |
| 28, 30 | `condCopy`/`bodyCopy` → `Loop{Body: bodyCopy, WhileCond: condCopy}` | explicit-copy | **yes as a class**: `Loop.Body`/`WhileCond` are the templates `spliceWhileRegion` re-splices each iteration (engine.go:9201); under the current engine the splice copies so even aliasing the literal's elems would be fine (loop.go:312-321), but under in-place stepping the template must be distinct from the tape | the `while` cond/body literals; the Loop continuation | a per-iteration construction of the region from an immutable template |
| 43-48 | `NewMark(id, condCopy...)` stores the cond in the mark; `condTokens` copied again and spliced | explicit-copy | yes (mark template vs executed run) | the mark payload | same |

### basic/go/forloop.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 27 | `bodySlice := _lst.Slice()` | explicit-copy | no (copied again at 34, 54) | — | drop |
| 34 | `bodyCopy` → `Loop.Body` | explicit-copy | yes as a class (see whileloop 28) — core says "no separate bodyCopy is needed because Tape.Splice copies" (engine.go:9082-9085) | the `for` body literal; `cont.Body` re-spliced per iteration | immutable template + per-iteration construction |
| 50-54 | `NewMark(id, bodySlice...)` + `bodyTokens` copy spliced | explicit-copy | yes (mark template vs executed run) | the mark payload | same |
| 30 | `InstallDef(r, iterName, NewInteger(start))` | construction | no | — | — |

### basic/go/native_type_gen.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 129 | `for _, entry := range lst.Slice()` | explicit-copy | no (scan) | — | borrow |
| 161 | `copy(body, toks)` then `sub.Run(body)` (F-bound param expression) | explicit-copy | no (engine copies) | the ParenExpr `Toks` | drop |
| 137, 155 | `r.Defs.PushType(p.Name, p.Node, NewTypeLiteral(p.Node))` | construction | — | — | — |

### basic/go/native_const.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 44 | `v := args[0]` | by-value-flow | — | — | — |
| 76 | interned hit: `ReparentValue(v, entry.TypeDef)` | explicit-copy (re-mint) | **yes** — `v` is the exemplar the user still holds (a def binding, a container element); only the RESULT is the singleton-typed value | the exemplar's binding/container | construct `(TypeDef, v.Data)` |
| 88-91 | `exemplar := v` captured in the membership predicate closure (`DeepEqual(x, exemplar)`) | by-value-flow | yes — the predicate compares against a snapshot; an in-place change to the exemplar would change the type's membership | the exemplar value | the predicate would need a constructed, frozen exemplar |
| 94 | `ReparentValue(v, node)` | explicit-copy (re-mint) | yes | as 76 | as 76 |

### basic/go/case_exhaustive.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 176 | `caseClauseMatch{resolved Value}` | by-value-flow (struct field holds a Value) | no (analysis-only record) | — | pointer field |
| 861 | `caseNormalizeClauses(r, lst.Slice())` | explicit-copy | yes (see conditional.go:126) | the clause list | construct |

### basic/go/micron.go (+ native_temporal.go, types_timer.go, types_bytes.go)

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 551, 605, 660, 677, 937, 1245, 1338, 1449, 1757, 1932, 2074, 2180, 2231, 2321 | `NewValueRaw(T…, MicronPayload{…})` | construction | — | — | none needed (this is the policy's preferred shape) |
| 2338 | `micronInstantiate`: `out[i] = WithPos(out[i], data)` — pos threaded onto the value just constructed | re-mint of a FRESH value | no | — | `out[i].SetPos(data.Pos())` |
| 2420 | `makeMicron…`: `ReparentValue(out[0], kind)` on the freshly constructed micron | re-mint of a FRESH value | no | — | construct with `kind` as Parent |
| native_temporal.go:96-124, types_timer.go:20/26 | `core.NewValueRaw(...)` constructors | construction | — | — | none |
| 282 | `t.SetBehavior(micronBehavior{kind: t})` on a registered type node | in-place (pointer receiver on the canonical node) | — | — | already policy-shaped |

### lang/go/native/natives.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 515-517 | `quoteWordHandler`: `v := args[0]; v.Quoted = true; captureAtomReferent(v, r)` | explicit-copy (param copy then flag) | **yes** — `args[0]` is the /q-captured atom just minted by the dispatcher (fresh), so in isolation incidental; but `quote x` with the TAny sig (525) quotes a value read from a binding/container, and that holder must stay unquoted | for 525: the binding or container element `x` resolves to; the stack cell | construct a quoted twin from parts (`NewValueRaw(v.Parent, v.Data)` + `Quoted`), or move `Quoted` out of the Value into the tape cell |
| 524-526 | `quoteAnyHandler`: same | explicit-copy | yes | as above | as above |
| 533-546 | `captureAtomReferent` → `SetAtomReferent(v, bound)` (core stores `&snap` of a copy) | by-value-flow | yes — the referent is a snapshot "what a quoted name referred to" | the binding `bound` came from | the referent would alias the binding |
| 612 | `wordHandler`: `NewSplice(args[0])` wraps the value in an `__SP` marker | by-value-flow (Value boxed inside a payload) | yes — `def name word [body]`: the marker's payload is spliced UNEVALUATED at every reference (lang/go/CLAUDE.md "`word` splices"); the body must never be the tape's cells | the def binding holding the marker; every splice site | the splice must construct tokens from the template each time (core `stepLiteral`) |
| 677 | `stackCollectHandler`: `copy(items, stack[len-n:])` → `NewList(items)` | explicit-copy | **yes** — the stack cells remain on the stack (`append(stack, NewList(items))` keeps them) and the list now holds the same values: two holders | the N stack cells and the new list's elements | construct the list from parts; the cells and elements would otherwise be one instance each |
| 693 | `stackCollectCheckFullStackFn`: `append(append([]Value(nil), stack...), carrier)` | explicit-copy | no (check shape) | — | append to owned slice |
| 462-497 | `slice*Handler`: `valueToSliceArg` → `voxgigstruct.Slice` → `sliceResult` rebuilds Values from `any` | explicit-copy (round-trip rebuild) | yes — `slice` must return a new list; elements that are not JSON-shaped pass through `valueToAny` as the SAME Value (helpers.go:28 `result[i] = elem`) | the source list's elements | construct the result list from parts (element sharing is the by-value question) |
| 196-240 | `update`/`remove`/`load`/`create` table forms "always build a NewList" | construction over borrowed rows | yes (copy-returning column) | the table rows | construct |
| 293-298 | push/pop/unshift/shift: "Plain lists keep the immutable new-copy semantics", Flex "mutates IN PLACE" | documented contract | yes | — | the policy flips the plain column to in-place (user-visible) |

### lang/go/native/native_storage.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 82-85 (sig doc) | "List (immutable — copy-returning, completing the column rule: Map and List both return the updated copy)" | documented contract | yes | — | policy changes the language |
| 734-765 | `setMapHandler`: fresh `OrderedMap`, copies every entry, `out.Set(key, val)`, `NewMap(out)`; `SetElemConstraint(elem)` on the fresh result | explicit-copy (container rebuild) | **yes** — receiver untouched is the contract; `def a {…} def b a  b set k v` leaves `a` as is | `args[2]`'s `*OrderedMap` is shared by every holder of that map Value (bindings, elements, captures) | in-place `m.Set` on the shared `*OrderedMap` would be visible through every holder; a constructed new map keeps the contract |
| 2078-2116 | `setListHandler`: `out := make([]Value, n)`; element copies; `out[idx] = val`; `NewList(out)`; `SetElemConstraint` on the fresh result | explicit-copy | **yes** — see summary answer 1 | `args[2]`'s `ListPayload.Elems` backing array, shared with every holder | construct the new list from parts |
| 772-790 | `delMapHandler` (same rebuild) | explicit-copy | yes | same | same |
| 560-590 | `d2AdoptTyped`: returns `Unify`'s rebuilt copy for nested typed containers, EXCEPT a flex child which is re-tagged IN PLACE (`core.RetagFlexElem`) "so a later mutation through the original child would be visible" | explicit-copy + deliberate in-place | yes — the comment documents a bug the copy caused (#9 round 9) | the caller's flex handle | this site is the policy's precedent: in-place re-tag of the one instance |
| 675, 693, 704, 762, 787, 2112 | `SetElemConstraint(elem)` on `res`/`v` — always a value just constructed (`NewList`/`NewMap`/`NewCarrier…`) | in-place on FRESH | no | — | already construction |
| 703 | `v.Carrier = true` on a fresh typed-map carrier | in-place on fresh | no | — | — |
| 1074 | `setClassInstanceHandler`: `val := args[1]` stored into the instance's field map | by-value-flow | yes — the instance stores a copy; in place, the stack/arg instance and the field are one | the arg's other holders | — |
| 1102-1130, 1258-1290 | `setFlexMapHandler`/`setFlexListHandler` write through pointer payload, return `args[2]` | in-place (by design) | — | all holders of the flex handle | already policy-shaped |
| 1302-1306 | `setXmlAttrHandler`: `val := args[1]; if !String { val = NewString(...) }; fd.Attr.Set(...)` | by-value-flow | no | — | — |
| 1449 | check-mode element read: `NewDynamicCarrierValue(CloneValue(child))` | explicit-copy (deep) | yes (check mode) — "Cloned for a fresh ID (the stored value's ID is shared with the map field and would collide in operand-provenance tracking)" (:1580-1582) | the stored container element; the recorder's provenance map keyed by ID | the recorder needs identity minted per read-site, not per value — a constructed carrier |
| 1586, 1595, 1627 | `getNodeReturns` (check): `CloneValue(val)` for closure-bearing fn wrappers, plain `FnDefInfo` reads, and concrete list/map members | explicit-copy (deep) | yes (check mode, same ID-collision reason) | the map/list member | as above |
| 1737-1770 | `getNodeHandler`: returns `list.Get(i)` / `m.Get(k)` by value | by-value-flow | **yes** — the element now lives in the container AND on the stack; every later in-place mutation of the stack value would reach into the container | the container's element | element reads would hand out the instance; `get` must construct… or nothing downstream may mutate |
| 1879-1887 | `setStoreHandler` → `CowSet(store, key, args[1], reg)` stores `args[1]` in a new COW layer's `map[string]Value` | by-value-flow | yes | the arg's other holders | — |

### lang/go/native/listops.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 49-55 | `pushHandler`: `list := _lst.Slice()`; `result := make(len+1)`; `copy`; `result[len] = newElem`; `d2RetainElem(NewList(result), args[1])` | explicit-copy ×2 | **yes** — copy-returning contract; `args[1]`'s backing array is shared by its holders | the receiver list's elements/holders | construct from parts |
| 65-75 | `popHandler`: `Slice()` + `copy(newList, list[:n-1])`; `popped := list[n-1]` | explicit-copy ×2 | yes | same | construct |
| 96-103 | `unshiftHandler`: `Slice()` + `copy(result[1:], list)` | explicit-copy ×2 | yes | same | construct |
| 112-122 | `shiftHandler`: `Slice()` + `copy(newList, list[1:])`; `shifted := list[0]` | explicit-copy ×2 | yes | same | construct |
| 124-192 | Flex variants write `fd.Elems` in place | in-place | — | all holders | already policy-shaped |
| 190 | `shiftFlexHandler`: `fd.Elems = append([]Value(nil), fd.Elems[1:]...)` — rebuilds the backing array rather than re-slicing | explicit-copy | **unsure/yes** — a re-slice `fd.Elems[1:]` would keep the shifted element reachable through any older slice header (e.g. a `ReadList` snapshot taken by `AsList`) and let a later `append` overwrite it; the copy severs that | earlier `ReadList` views of the same backing array | `fd.Elems = fd.Elems[1:]` is in-place but aliases; the policy must accept aliasing of views |

### lang/go/native/list.go, create.go, update.go, remove.go, load.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| list.go:65-68 | `listAllHandler`: `rows := Slice(); copy(result, rows); NewList(result)` — double copy | explicit-copy ×2 | yes (returns a new list) | the table's rows | construct from parts (one copy is already redundant) |
| list.go:77 | `listFilterHandler`: `rows := Slice()`, `matched = append(matched, row)` | explicit-copy + by-value element flow | yes (new list of the same rows) | the rows | construct |
| create.go:69, 92-96 | `createHandler`: `rows := Slice()`; `result := make(len+1); copy; result[len] = args[0]` | explicit-copy ×2 | yes | the table rows; the new record | construct |
| update.go:68, remove.go:68, load.go:68 | `rows := _lst.Slice()` then build a new row list | explicit-copy | yes (new list) | the rows | construct |

### lang/go/native/merge.go (+ struct_module.go, setpath.go)

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| merge.go:42-50 | `mergeListMapHandler`: `Slice()` + `copy(result, list)`; patches `result[idx]`; `d2ReTagContainer(r, args[0], NewList(result), …)` ("The original list is unchanged.") | explicit-copy ×2 | **yes** — documented | the list operand's holders | construct |
| merge.go:90-97 | `mergeMapListHandler`: same | explicit-copy ×2 | yes | same | construct |
| struct_module.go:50-56 | `merge` registrations "Both index-merge forms always build a NewList"; the `[Any Any]` form is `voxgigstruct.Merge` over `valueToAny` round-trips | documented contract | yes | — | construct |
| struct_module.go:75-83, setpath.go:240-268 | `setpath` on a list: `cp := make(n)`, element copies, recursion into `cp[idx]`, `cp[idx] = tchild`, `NewList(cp)` + `SetElemConstraint` on the fresh result | explicit-copy | yes ("R3: keep [:T] on the copy") | the container's holders | construct |
| setpath.go:296-302 | map branch: fresh `OrderedMap` + `SetElemConstraint` | explicit-copy | yes | same | construct |
| struct_module.go:92 | `clone` → clone.go:19 `CloneValue(args[0])` | explicit-copy (deep, by definition) | **yes** — the word's meaning | everything | the one word that must stay a deep construction |
| struct_module.go:210-224 | `d2DynamicTypedResidual`: flags/`SetElemConstraint` on a freshly minted carrier | in-place on fresh | no | — | — |

### lang/go/native/native_array.go, native_sort.go, flatten.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| native_array.go:946-958 | `reverseHandler`: `elems[i] = list.Get(n-1-i)` into a fresh slice; `d2RetainElem(NewList(elems), args[0])` | explicit-copy | yes ("pure reorder — retain the source tag"; source untouched) | the source list | construct |
| :963-993 (24-30 of fn) | `takeHandler`: `elems[i-start] = list.Get(i)` | explicit-copy | yes | same | construct |
| :1051-1078 | `uniqueHandler`: `result = append(result, elem)` | explicit-copy | yes | same | construct |
| native_sort.go:48-63 | `sortListHandler`: `out[i] = lst.Get(i)`; `sort.SliceStable(out, …)`; `d2RetainElem(NewList(out), args[0])` | explicit-copy | **yes** — sorting in place would reorder the source list for every holder | the source list | construct |
| native_sort.go:77-90 | map sort: `kv{v Value}` pairs | by-value-flow | no | — | — |
| flatten.go:14-56 | `flatten`: `valueToAny` → `voxgigstruct.Flatten` → `structConvert` rebuild | explicit-copy (round-trip) | yes (new list) | source elements pass through `valueToAny` as the same Value for non-JSON kinds | construct |
| native_array.go:1731 | higher-order body check model: `input = append(input, vals...); input = append(input, bodyList.Slice()...)` then `sub.Run` | explicit-copy | no (engine copies) | — | borrow |
| :1824-1825 | `out.Carrier = true; out.Dynamic = true` on a freshly built accumulator carrier | in-place on fresh | no | — | — |
| :1957 | `elems = append(elems, rl.Slice()...)` (check-mode element join) | explicit-copy | no | — | borrow |
| :2269 | `lanes[i] = ri.Slice()` (outer/inner lanes) | explicit-copy | unsure — lanes are read by the driver; `d.cellRight = right.Slice()` (loop_drivers.go:254) likewise | the operand lists | borrow if the driver never writes lanes (it does not appear to) |
| :1559-1572 | `eachHandler` → `StartLoop(reg, "each", newEachListDriver(reg, "each", args[0], dataList, true), nil)`; the driver stores `body Value` and core's `Loop.bodyTokens` returns the list's OWN elements (no copy) | by-value-flow | **yes** (risk 1): the body literal's elements are the splice source every iteration; only the tape's copy keeps them unstepped | the `each [body]` literal | per-iteration construction |

### lang/go/native/loop_drivers.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 26, 84-86, 124-126, 168, 234-244, 400-405, 518-522, 598-599, 700-704 | driver structs hold `body`, `acc`, `root`, `src`, `result`, `pairOp`, `aggOp` as `Value` fields | by-value-flow | yes — `d.acc = res[len(res)-1]` (:104, :147, :352, :576, :738-740) snapshots the body's result as the next accumulator; the same Value is also the body's residual the engine may still hold | the body residual / the init value (`[] fold [push] xs` — the init list is `d.acc` and every push returns a new one) | pointer fields; the accumulator would be the body's result instance |
| 254 | `d.cellLeft, d.cellRight = left, right.Slice()` | explicit-copy | unsure (read-only lanes) | the right operand | borrow |

### lang/go/native/native_flex.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 157 | `flexReturns` (check): `v := NewCarrier(TFlexMap); v.Data = ss.CloneShape()` | in-place on fresh + shape clone | no | — | — |
| 299 | `appendXmlHandler`: `elems := src.Slice()`; each adopted via `AdoptIntoFlex` and appended to `fd.Cren` | explicit-copy | yes-ish — `AdoptIntoFlex` rebuilds a flex child from a plain element; the source list stays plain | the source list | the adopt is a construction already |
| 318-323 | `appendListHandler`: "Slice() snapshots before the grow, so `append f f` (self-concat) is well-defined" | explicit-copy | **yes** — self-append over the same backing array would otherwise read its own appended elements | the flex list's own `Elems` when `args[0]` and `args[1]` are the same node | the snapshot IS a construction of the read set; keep as a length-bounded iteration (`n := len; for i < n`) which needs no copy |

### lang/go/native/native_type.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 753-755 | `asHandler`: `out := args[1]; out.SetAscribed(target); return out` | explicit-copy (param copy then facet write) | **yes** — "every arg-delivery boundary … strips it, so the ascription selects exactly one dispatch and never leaks into handlers, bindings, containers" (core value.go asc doc). `x as T` must not ascribe `x`'s binding | the binding/container `args[1]` was read from; the stack cell | the ascription cannot be a field of the one instance; it has to ride the tape cell / dispatch request, or be a constructed twin |
| 789-803 | `asReturns` (check): `c := NewCarrier(target); c.SetAscribed(target); WithPos(c, v)` (fresh) and `out := v; out.SetAscribed(target)` (copy) | construction + explicit-copy | yes (check mode, same reason) | the check-stack carrier | as above |
| 388, 405-406, 467 | `target := args[0]`, `base := args[0]; arg := args[1]` (reads) | by-value-flow | no | — | — |
| 1140, 1154 | `guardHandler`/`baseHandler`: `val := args[0]` returned / `&v` taken for a type literal ("the denoted lattice node is &v") | by-value-flow | yes for `&v`: a pointer to a LOCAL copy of a type literal is a non-canonical `*Type` (core `CanonicalType` doc explains the hazard) | the canonical lattice node | the pointer model would make `&v` the canonical node itself — an improvement, not a break |
| 1724-1725, 1750, 1764 | `convert*Handler`: `src := args[…]` → `convertTo` builds a new value | by-value-flow | no | — | — |

### lang/go/native/native_behave.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 212 | `behave` install: `body := append([]Value{}, sig.Body()...)` stored in the `userBehavior` slot | explicit-copy | **yes** — the behaviour body is run on every compare/canon/… call (`runBehaviorBody`); it must never be the fn's live signature body nor the tape | the fn value's `FnSig` Boru body | immutable template + per-run construction |
| 227 | `target.SetBehavior(ub)` on the canonical type node | in-place | — | — | policy-shaped |
| 300 | `fnVal := args[1]` | by-value-flow | no | — | — |
| 787 | `userUnify`: `top = core.ReparentValue(top, u.unifyTarget)` — reparents the unify result to the behaviour's target | explicit-copy (re-mint) | yes — `top` is the behaviour body's residual (a fresh value usually, but may be one of the inputs `a`/`b` returned unchanged) | possibly the unify operands' holders | construct `(target, top.Data)` |
| 1016, 1018 | `runBehaviorBody`: `append([]Value(nil), body...)` (stepless) / `RunPooledTop(r, append([]Value{}, body...))` | explicit-copy | 1016 yes (the returned slice becomes the result stack; must not be the stored body); 1018 no (RunPooledTop → engine copies) | the stored behaviour body | 1016: construct; 1018: drop |

### lang/go/native/native_process.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 205 | `spawn`: `tokens = make(...); copy(tokens, bodyList.Slice())` — the process body | explicit-copy ×2 | yes as a class (body run in another goroutine's engine; the literal may spawn again) | the `spawn [body]` literal | construct per spawn |
| 335 | `send`: `target.Send(CloneValue(msg))` — "Deep-copy at the boundary so the receiver can never observe the sender mutating a shared container" | explicit-copy (deep) | **yes** — process isolation | every container reachable from `msg` | must stay a deep construction (an isolation copy is semantic) |
| 404 | `receive` clause parse: `elems := ll.Slice()` | explicit-copy | no (scan) | — | borrow |
| 423, 441 | `after`/pattern clause bodies: `copy(body, bodyList.Slice())` into `recvAfter.body` / `clause.body` | explicit-copy ×2 | yes as a class (clause bodies re-run per message) | the clause literals | construct |
| 584 | `recvBinding{val Value}` | by-value-flow | — | — | pointer |
| 592 | `runClauseBody`: `copy(tokens, body)` then `New(r).Run(tokens)` | explicit-copy | no (engine copies) | — | drop |

### lang/go/native/native_temporal_await.go, native_temporal_timeout.go, io_watch.go, log_span.go, native_module_module.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| await.go:121 | `branchBody`: `return body.Slice()` | explicit-copy | no (consumer copies again at 362) | — | borrow |
| await.go:340 | `interpretBranchBody(reg, _lst.Slice())` | explicit-copy | no | — | borrow |
| await.go:362 | `copy(input, body)` then `sub.Run` | explicit-copy | no (engine copies) | — | drop |
| await.go:548 | `len(lst.Slice()) == 0` | explicit-copy | no (`Len()` suffices) | — | `lst.Len()` |
| timeout.go:17-18 | `RunTimerCallback`: `input = make(len(_lst.Slice())); copy(input, _lst.Slice())` — two Slice() calls | explicit-copy ×3 | no (engine copies) — but the callback literal re-runs on every interval tick, so the class matters | the timer callback list | drop the handler copies; the engine copy is the one that counts |
| io_watch.go:124 | `copy(tokens, bodyList.Slice())` → watcher info body | explicit-copy ×2 | yes as a class (body re-run per fs event) | the `watch` body literal | construct per event |
| log_span.go:418 | `New(r).Run(append([]Value(nil), body.Slice()...))` | explicit-copy ×2 | no | — | drop |
| native_module_module.go:374 | `copy(input, elems)` then run the module body | explicit-copy | no (engine copies) | — | drop |

### lang/go/native/native_macro.go, native_valof.go, stackform/eval.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| macro.go:336, 370 | `macro` spec: `elems := spec.Slice()`; `bodyElems = bl.Slice()` stored in the macro definition | explicit-copy | yes (macro body is a template expanded per use) | the macro literal | construct per expansion |
| macro.go:594-622 | `NewSplice(NewList([]Value{partial}))` / `NewSplice(NewList(hookToks))` — splice markers wrapping freshly built token lists | construction | — | — | — |
| macro.go:678-682 | `appliedTransducer(fn Value)`: `fn.Quoted = false; return fn` | explicit-copy (param copy then flag) | yes — the stored transducer (a container member / binding) keeps its `Quoted` | the binding/map member the fn was read from | `Quoted` must leave the Value (tape-cell attribute) or be constructed |
| macro.go:1166 | `fn.Dynamic = false` on a parameter copy before `rec.RecordCall` (check) | explicit-copy | yes (check mode: the carrier on the check stack keeps `Dynamic`) | the check-stack carrier | construct a non-dynamic twin |
| macro.go:1234-1239 | `ops := append([]Value(nil), args...); ops[0].Dynamic = false` | explicit-copy | yes (same) | the dispatcher's `args` | construct |
| valof.go:476-477 | `applyReturns`: `out[0].Quoted = false; out[0] = markApplied(out[0])` on `ReturnsIdentity(0)`'s result (a fresh slice holding a copy of `args[0]`) | explicit-copy (copy then flag) | yes (check mode) | the check-stack fn carrier | construct |
| valof.go:492-499 | `applyHandler`: `v := args[0]; v.Quoted = false; markApplied(v)` | explicit-copy | **yes** — "a 0-arg lambda stored in a map or list stops being data" if the stored instance were unquoted/marked Applied; `f/v apply` must mark only the stack value | the binding `f`, any container holding the fn | the Applied mark and `Quoted` must live outside the one instance |
| stackform/eval.go:76-80 | `v := o.V; v.Quoted = true` for fn PushLits (stamp set) | explicit-copy | yes (the stackform program's `PushLit.V` stays unquoted for the next run) | `stackform.PushLit{V}` (stackform.go:45) | construct a quoted twin |
| eval.go:126 | `lst := NewList(inner); lst.Quoted = true` | in-place on fresh | no | — | — |
| eval.go:256-259 | `unquotedCopy(v)`: `v.Quoted = false; return v` | explicit-copy | yes (same program-reuse reason) | the PushLit | construct |
| stackform.go:45 | `PushLit{V core.Value}` — a compiled form holding literal Values re-pushed on every run | by-value-flow | **yes** — a stackform program is run many times; each run pushes a COPY of `V` | the compiled form | the form would push the same instance every run; any in-place mutation of a pushed literal corrupts the program |

### lang/go/native/native_misc.go, query.go, native_query.go, transform.go, jsonify.go, helpers.go, native_math.go, native_map_iter.go, walk_core.go, native_object_record.go, native_string.go, native_unpack.go, native_make.go, native_error_raise.go, io_filetype.go, io_stream.go, clone.go, native_inspect.go, format.go, fileio.go, sqlite*.go, io_*.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| native_misc.go:687, 867, 886, 903 | `RunModuleBody(r, _lst.Slice())` | explicit-copy | no (engine copies) | — | borrow |
| native_misc.go:705, 842, 852, 891 | `installRenamedExports(r, desc, _lst.Slice())` | explicit-copy | no (scan) | — | borrow |
| native_misc.go:962 | `parallel`: `elems := _lst.Slice()` (branch bodies dispatched to sub-engines) | explicit-copy | no | — | borrow |
| native_misc.go:258-262 | `readReturns`: `c := NewCarrier(TAny); c.Dynamic = true` | in-place on fresh | no | — | — |
| native_misc.go:621 | `referentHandler`: `v := args[0]` read | by-value-flow | no | — | — |
| query.go:468, 561, 579, 818, 1098, 1225, 1244, 1311 | SQL clause builders: `elems := _lst.Slice()` read-only | explicit-copy | no | — | borrow |
| query.go:1037, 1344 | `val := elems[i]` / `e := elems[i]` reads | by-value-flow | no | — | — |
| query.go:406 | `NewValueRaw(TList, MaterializerPayload{M: qb})` | construction | — | — | — |
| native_query.go:396 | `using`: `elems := lst.Slice()` read | explicit-copy | no | — | borrow |
| transform.go:87, jsonify.go:87, helpers.go:28 | `valueToAny`/`valueToAnySer`/`valueToSliceArg`: `Slice()` then convert each element; helpers.go:28 `result[i] = elem` re-emits the Value itself inside `[]interface{}` | explicit-copy + interface boxing | no for the copy; the boxing is a by-value flow (`any` holds a struct copy) | — | borrow; boxing would hold the pointer |
| native_math.go:478 | `doEvalList(r, lst.Slice())` | explicit-copy | no | — | borrow |
| native_map_iter.go:75 | `mapBody{tokens: bl.Slice()}` — body tokens for map iteration words | explicit-copy | yes as a class (body re-run per entry) | the body literal | construct per entry |
| walk_core.go:162 | `h.tokens = bl.Slice()` (walk hook body) | explicit-copy | yes as a class | the hook literal | construct per visit |
| walk_core.go:36-67 | `walkFrame{node, parent Value}`, `walkHook{fn, body, sigFn Value}`, `{val Value}` | by-value-flow | — | — | pointer fields |
| walk_core.go:210 | `NewValueRaw(TMap, MapPayload{M: om})` | construction | — | — | — |
| native_object_record.go:34 | `for _, elem := range elems.Slice()` | explicit-copy | no | — | borrow |
| native_object_record.go:96 | `NewValueRaw(TClass, ClassTypeInfo{…})` | construction | — | — | — |
| native_string.go:304 | `doConcat`: `for _, e := range elems.Slice()` | explicit-copy | no | — | borrow |
| native_unpack.go:266 | `val.Dynamic = true` on a local read from a container (check) | explicit-copy (local then flag) → bound via `bindUnpackEntry` | yes (check mode: the container member stays non-dynamic) | the source map's member | construct |
| native_unpack.go:324 | `for _, el := range names.Slice()` | explicit-copy | no | — | borrow |
| native_make.go:97, 99 | `out[0].SetElemConstraint(...)` on the result of the base ReturnsFn (fresh carrier) | in-place on fresh | no | — | — |
| native_error_raise.go:178 | `ae := MakeBoruError(...); ae.Data = data` | in-place on fresh | no | — | — |
| io_filetype.go:49, io_stream.go:68 | `core.ReparentValue(core.NewAtom(name), kind)` — reparent of a just-minted atom | re-mint of a FRESH value | no | — | `NewValueRaw(kind, AtomPayload{…})` |
| clone.go:19 | `CloneValue(args[0])` | explicit-copy (deep) | yes (the word) | — | deep construction |
| clone.go:37-52 | `cloneReturnsFn`: fresh carriers "a clone is a new value (`(clone p) eq p` is false)" | construction | — | — | — |
| native_inspect.go:119/123/180/307, format.go:348, fileio.go:359, sqlite.go:319, sqlite_stub.go:286, io_handle.go:176, io_lock.go:87, io_watch.go:156, io_mmap.go:151 | `NewValueRaw(...)` mints | construction | — | — | — |
| native_service.go:59 | `serviceState.state Value // the private state: a flex map, mutated in place` + `:128/:339` return it, `:439 state := s.state` | by-value-flow | yes — the service state is intentionally one pointer-backed instance; returning `s.state` by value hands out a struct copy sharing the flex store | handlers that receive `state` | pointer field; already in-place through the payload |

### lang/go/native/internal/patrun/patrun.go, lang/go/capabilities/*, lang/go/tuikit/*

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| patrun.go:477, 493, 504 | `copy(vsc, vs)` | `[]string` copies | not Values | — | out of scope |
| capabilities/handles.go:236, 318; capabilities.go:1333; locks.go:256; native/io_mount_handle.go:198/209; io_binary.go:111/114 | `copy(grown, …)` | `[]byte` copies | not Values | — | out of scope |
| tuikit/frame.go:67, diff.go:37 | cell copies | not Values | — | — | out of scope |
| native_patrun.go:39-40 | `{raw, val Value}` entries | by-value-flow | — | — | pointer fields |

### lang/go/modules/test.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 304, 357, 500 | `native.New(r).Run(body.Slice())` for `describe`/`it`/`run` bodies | explicit-copy | no (engine copies) — but `it` bodies are closed over and re-run (`run.runCase` closures) so the class matters | the test body literal | borrow; engine copy remains |
| 345 | `body := args[1]` captured in a Go closure for `runCase` (compiled path) | by-value-flow | yes — closure holds a snapshot of the fn value | the arg | — |
| 579 | `for _, p := range pathList.Slice()` | explicit-copy | no | — | borrow |
| 616-628 | `property`: `gen := args[1]; prop := args[2]; gen.Quoted = true; gen.Eval = false; prop.Quoted = true; prop.Eval = false; m.Set("gen", gen)…` — param copies, flagged, stored in a new map | explicit-copy (copy then flag then store) | **yes** — "Mark bodies Quoted so subsequent consumers … don't auto-evaluate them"; the args are the user's body literals which stay evaluable elsewhere | the literal lists (`args[1]`, `args[2]`) | construct quoted twins |
| 828 | `bodyTokensOf`: `return nil, lst.Slice(), nil` | explicit-copy | no | — | borrow |
| 1369 | results table: `copy(rows, run.results)` → `NewValueRaw(TList, TableData{Rows: rows})` | explicit-copy | yes — `run.results` keeps growing after the snapshot; the table must be stable | `run.results` | construct the table rows (a snapshot IS the semantics) |
| 1478 | `InvokeBody(parent, body, append([]native.Value(nil), inputs.Slice()...))` | explicit-copy ×2 | no (InvokeBody copies into a tape) | — | borrow |

### lang/go/modules/debug.go, debug_step.go, debug_dashboard.go, vm.go, scry.go, rand.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| debug.go:88-89, vm.go:162-163 | `lst := NewList(tokens); lst.Quoted = true` (parse results) | in-place on fresh | no | — | — |
| debug.go:139 | `v := args[0]` read | by-value-flow | no | — | — |
| debug.go:349, 389, 563 | `RunTrace`/`sub.Run(append([]native.Value(nil), body.Slice()...))` | explicit-copy ×2 | no (engine copies) | — | borrow |
| debug.go:440 | `stackform.Compile(sub, append([]native.Value(nil), body.Slice()...))` | explicit-copy ×2 | unsure — the compiled form retains `PushLit{V}` copies of the literals (stackform.go:45); with pointers the form would reference the body's own elements | the `Debug.compile [body]` literal | construct the form's literals |
| debug.go:538 | `body := args[0]` → `RunCarrierBody` | by-value-flow | no | — | — |
| debug.go:659, 690 | `Slice()` scans for size/walk | explicit-copy | no | — | borrow |
| debug_step.go:43 | `runStepped(r, append([]native.Value(nil), body.Slice()...))` | explicit-copy ×2 | no | — | borrow |
| debug_step.go:187 | `v := stack[pointer]` read | by-value-flow | no | — | — |
| debug_dashboard.go:43 | `renderDashboard(r, widgets.Slice())` | explicit-copy | no | — | borrow |
| scry.go:76 | `WalkBodyWords(body.Slice(), …)` | explicit-copy | no | — | borrow |
| scry.go:224-226 | `body := NewList(append(nil, sig.Body()...)); body.Quoted = true` — exposes a fn's body as a quoted list | explicit-copy (copy of the fn body into a fresh list) | **yes** — the returned list is user data; it must not alias the fn's live body tokens (which `fnDefCopy`/dispatch read) | the fn value's `FnSig` body | construct |
| rand.go:448 | `bodyTokens := body.Slice()` for `Rand.list-of [body] N` (re-run N times) | explicit-copy | yes as a class | the body literal | construct per iteration |

### lang/go/modules/fn.go, parselang.go, model.go, parse.go, report.go, gex.go, minilang_query.go, test_coverage.go, type.go, tui_mirror.go, vault.go, net_codec.go, fmtrule.go, matrix.go, tui_run.go, emitlang.go, minilang_xpath.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| fn.go:305-308 | `FnUtil.const`: `x := args[0]` captured by the returned Go handler, returned on every call | by-value-flow | **yes** — every call returns a COPY of the snapshot; a pointer model returns the same instance each call (and the instance is also whatever `args[0]` was read from) | the captured value; each call's result holder | construct per call |
| fn.go:449-450 | `partial`: `fn := args[0]; bound := args[1]` captured; `full := make(0, n)` built per call | by-value-flow | yes (same) | same | same |
| fn.go:550 | `memoize`: `copy(stored, vals); cache[key] = stored` | explicit-copy | **yes** — the cache must not alias the result slice the caller mutates/consumes; cache hits return `stored` (then the caller gets the cache's instances — already an aliasing hazard under pointers) | the call's result slice; every later hit | construct per hit |
| fn.go:566-575 | `curryLevel`: `copy(next, bound)` per level "so sibling partial applications never share a backing array" | explicit-copy | yes (documented aliasing guard) | sibling partials | construct |
| parselang.go:310 | `copy(resolved, args); resolved[0] = NewString(src)` | explicit-copy | yes — the dispatcher's `args` must not be rewritten (the fallback path re-reads it) | `args` | construct a fresh operand slice |
| parselang.go:336, vault.go:389, tui_mirror.go:51 | `c := NewCarrier(t); c.Dynamic = true` | in-place on fresh | no | — | — |
| parselang.go:579-586 | `copy(sigs, fd.Signatures)`; patch `Returns`; `fd.Signatures = sigs; bv.Data = fd` — re-boxes a modified FnDefInfo copy into a copy of the fn Value | explicit-copy (payload struct copy + Value copy) | yes — the original fn value (a binding/export) keeps its sigs | the fn's holders | construct a new fn Value from parts |
| parselang.go:458 | `fnVal := args[0]` | by-value-flow | no | — | — |
| model.go:328-333 | `stampActionFn`: `fd.Name = name; fnVal.Data = fd` on the param copy, then `compiler.StampFnValue` | explicit-copy (payload copy then re-box) | yes — the caller's fn value keeps `Name == ""` | the fn's holders | construct |
| parse.go:83 | `{fn native.Value}` struct field | by-value-flow | — | — | pointer |
| parse.go:555, 597 | `fns = lst.Slice()` / `entries = lst.Slice()` scans | explicit-copy | no | — | borrow |
| report.go:138, 154, 221 | `rows.Slice()` scans for rendering | explicit-copy | no | — | borrow |
| gex.go:115, minilang_query.go:49/82 | `Slice()` scans/conversions | explicit-copy | no | — | borrow |
| test_coverage.go:139, 157, 169 | `coverWalk*(lst.Slice(), rows)` | explicit-copy | no | — | borrow |
| test_coverage.go:226 | `native.New(r).Run(body.Slice())` | explicit-copy | no | — | borrow |
| type.go:177-178 | `latticeNode`: `node := v; return &node` — pointer to a LOCAL copy of a bare type node | by-value-flow | yes — a non-canonical `*Type` (see core `CanonicalType`); under pointers `&v`… would become the real node | the canonical lattice node | the pointer model fixes this |
| type.go:717 | `anon.SetName("brand:"+tag)` on a freshly minted refine prefab | in-place on fresh | no | — | — |
| net_codec.go:175 `{msg Value}`, :631/:725 `v := args[0]` | by-value-flow | no | — | — |
| fmtrule.go:67, 103 | `node := args[0]` reads | by-value-flow | no | — | — |
| matrix.go:59, 968 | `copy(shape…)`, `copy(a, m.Data)` | `[]int`/`[]float64` copies (TensorData.DeepClone) | not Values | — | out of scope |
| tui_run.go:90-91, emitlang.go:148, minilang_xpath.go:84 | struct fields holding `Value` | by-value-flow | — | — | pointer fields |

### lang/go/boru.go, parse.go, formatter/rules_boru.go, stackform/stackform.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| boru.go:480-520 | `CompileCheck`: `values, _ := parser.Parse(src)`; `compilePass(src, values, false)` and, on a specialisation retry, `compilePass(src, values, true)` over the SAME slice | by-value-flow (parsed program reused across two check passes) | **yes** — the second pass must see the pristine tokens; today it does because `engine.Run` copies them into the tape and check-mode flag flips happen on copies | the retained `values`; `es.SetRootBody(values)` (:537) | re-parse per pass, or a per-pass construction of the token run from an immutable parse |
| boru.go:537 | `es.SetRootBody(values)` — the recorder retains the parsed tokens so "a top-level landing's `/q` claim resumes the interpreter in them" | by-value-flow | yes | the compiled program | same |
| boru.go:541, :348, :983 | `engine.Run(values)` — core copies into the tape | (core) | — | — | — |
| boru.go:988 | `convertResults(result []core.Value) []any` | by-value-flow (boxing) | no | — | — |
| parse.go:14 | `Parse(src) ([]core.Value, error)` | construction | — | — | — |
| formatter/rules_boru.go:72-73 | `var table *core.Value; v := vals[i]; … table = &v` | by-value-flow (pointer to a loop-local copy) | no | — | pointer to the element |
| stackform/stackform.go:45 | `PushLit{V core.Value}` | by-value-flow | yes (see stackform/eval.go rows) | the compiled form | — |

### parser/go/parse.go, xml_literal.go

The parser never re-mints an existing Value: all 69 `core.New*` calls in
parse.go (and 5 in xml_literal.go) construct from source text. The hits are:

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 83-88 | `withPos(v core.Value, pos) core.Value { if pos.Row != 0 { v.SetPos(pos) }; return v }` — parameter copy, stamped, returned; every caller passes a value it just built (229, 237, 247, 258, 273, 282, 423, 1237) | explicit-copy (param copy, immediately replaces the original) | no — the original temporary is discarded | — | `v.SetPos` on the local; a pointer model makes this literally in place |
| 256, 665, 681 | `mv.Eval = true` on the map value just returned by `convertMapData` | in-place on a fresh local | no | — | — |
| 421 | `reachInfo.Receiver, reachInfo.Eval = nil, false` on a fresh `ReachInfo` payload | in-place on fresh | no | — | — |
| 566 | `core.NewDispatchMod(core.DispatchModInfo{Val: v, Quote: q})` — the `/v` operand Value boxed inside another Value's payload | by-value-flow | yes — the modifier wraps the token; the token is only reachable through the wrapper | the wrapper | pointer inside the payload |
| 1562-1563 | `bound := core.NewList([]core.Value{core.NewAtom(name)}); core.NewSugar(core.SugarInfo{Items: []core.Value{bound}})` | construction (nested) | — | — | — |
| 644, 697, 708, 714, 725, 728, 798-2128 (48 sites) | `return core.Value{}, err` — zero-Value error sentinels | by-value-flow (zero sentinel) | no | — | `nil` pointer |
| 2108 | `body := src` (a string, not a Value) | — | — | — | — |
| xml_literal.go:17 | `{V core.Value}` part struct; :377/389/393 `return core.Value{}, false` | by-value-flow | no | — | pointer / nil |

### cmd/go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| internal/debugger/debugger.go:66 | `histEntry{stack []native.Value}` — one retained `Tape.Snapshot()` per visited line, ring of 64 (`historyCap` :58) | by-value-flow (retained snapshot; the copy is made by core) | **yes** — time travel (`back`) renders past stacks from these; "the snapshot is the truth, the registry is NOT rewound" (:654) | the live tape | the ring needs constructed snapshots; with shared instances every entry would show the present |
| :160 | `curStack []native.Value` "Storing the reference is free — the engine already allocated the snapshot" | by-value-flow | yes (post-mortem `bt` after unwind) | the fault-time tape | same |
| :440, :464 | `s.curStack, s.curPointer = stack, pointer`; `reg.Defs.SnapshotEntries()` (def-table snapshot for post-mortem) | by-value-flow | yes | — | same |
| :943-964 | `resolvedData`: `vals := make(0, n)`; appends the non-marker cells of a snapshot into a new slice | explicit-copy | no (the snapshot is already a copy; this is a filter) | — | borrow/filter |
| :1066-1070 | `setWatch`: stores the binding's RENDERING (`map[string]string`) as the compare baseline | string snapshot | not a Value copy | — | — |
| dap.go:66, :172 | `tokens []native.Value` passed through to `RunProgram` | by-value-flow | no | — | — |
| internal/exec/exec.go:207-208 | `resp.Stack = make([]any, len(stack)); copy(resp.Stack, stack)` — boxes each result Value into `any` for JSON | explicit-copy (boxing) | no (serialised immediately) | — | box pointers |
| internal/repl/repl.go:121, meta.go:20 | `lastStack []native.Value` / `MetaContext.Stack` retain the last result slice for meta commands | by-value-flow | unsure — the next evaluation builds a new slice, so no in-place hazard unless a meta command mutates | the previous run's result | — |
| internal/test/test.go:264, 279 | read-only walks of a result stack | by-value-flow | no | — | — |
| genhelp/main.go:161 | `prog := []native.Value{…}` construction | construction | — | — | — |
| internal/vault/*, attach.go:172, buildrt.go:393 | `copy(` on `[]byte`/`[]string` | not Values | — | — | out of scope |
