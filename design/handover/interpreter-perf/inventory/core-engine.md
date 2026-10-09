# core/go — engine-side inventory for the "Values are never copied" policy

Scope: `engine.go`, `sealed.go`, `loop.go`, `tape.go`, `registry.go`, `fork.go`,
`fn_capture.go`, `deftable.go`, `invoke.go`, `engine_pool.go`, `compile_sandbox.go`,
`fn_frame.go`, `fn_frame_elide.go`, `fn_frame_probe.go`, `dispatch_explain.go`,
`dispatch_layout.go`, `dispatch_probe.go`, `dispatch_slots.go`, `resolve.go`,
`contextstack.go`, `store_shape_state.go`, `storage_helpers.go`, plus the engine-side
helpers those files lean on (`collect_kernel.go`'s tape edits, `argsstack.go`,
`optimistic_match.go`, `fn_def.go`, `trace.go`, `pending_literal.go`) and
cross-references into files owned by the other reviewer where an engine site's
judgement depends on them (`core_helpers.go` buildFnBodyHandler /
RetagTypedContainer*, `value.go` NewTypeLiteral / StripAscribed / NewMark /
ReadList.Slice / SetAtomReferent, `util.go` WithPos, `fn_value_compile_cache.go`).
Line numbers are from the current tree (HEAD as of 2026-10-09). Read-only review; no
files in the repo were touched.

Legend for `kind`: **explicit-copy** = a Value (or a `[]Value` run / a payload struct
carrying Values) is copied so the copy can be changed or stored separately;
**by-value-flow** = a place a `*Value`-per-instance conversion would hit even though no
copy is written there (a field or map holding a Value by value, a `Value{}` sentinel, an
ID/field comparison standing in for identity, a pool or scratch that reuses Value slots,
or code that relies on a copy being independent). **construction** = a fresh Value minted
from parts (already policy-compatible; listed only where it replaces a token or where the
surrounding copy matters).

---

## Summary

### Counts of explicit copy sites by kind (engine-side files only)

| kind | count | representative sites |
| --- | --- | --- |
| Occurrence-state copy-modify-writeback on a tape cell or arg slot (`pos`, `Quoted`, `Eval`, `ReachGroup`, `Undefined`, `FailedDispatch`, `asc`, `Dynamic`/`DynFrom`, `FnDefInfo.Applied`, `ClosurePayload.SigMatched/RetTrim`, `Data` wrap) | 31 | engine.go 2701, 2952-2957, 3160, 3224, 3247-3252, 3320, 3396, 3761, 3826, 4603, 4897, 5007, 6231-6233, 6251, 7094, 7459-7460, 7837, 8420, 9460, 9921, 1441, 10316, 1387-1390; collect_kernel.go 353-355, 898, 932, 950-952; invoke.go 84, 103-104; dispatch_slots/check tagCheckModeDefRead on `&tagged` |
| Binding / canonical-node read copies (`Defs.Top` → `e.Body` or `*t`, `ResolveRef`, `NewTypeLiteral`) | 11 | deftable.go 94, 96, 109; engine.go 3158, 3170, 3319, 2862 (ResolveRef), 3636, 7482/7592 (`.(FnDefInfo)` payload copy); resolve.go 51 |
| Token-run copies onto a tape (fn body, loop body, mark body, quotation elements, paren items, reach receiver, splice payload, program load) | 19 | engine.go 1731-1733, 1990, 2008, 4730, 4803, 4811, 4879, 5300, 5633-5636, 5661, 7637, 9027-9029, 9087-9088, 9274, loop.go 277-279, sealed.go 118-119/158-159, invoke.go 423, registry.go 1896, engine_pool.go 49-51, tape.go 164-165/208 |
| Fresh container construction replacing a pending literal (re-evaluated per call) | 9 | engine.go 5287-5294, 5322, 6030, 5154-5158, 3820-3824/7433-7448, 8906-8911, 10969-10980; resolve.go 81-86, 108-111, 127-131 |
| Snapshot / clone copies for sandboxes, forks and debuggers | 14 | deftable.go 318, 349, 431, 451, 530 (Clone); fork.go 40; compile_sandbox.go 54-57; util.go 391-407 (predicate sandbox); tape.go 477/484; registry.go 608, 715; engine.go 1818, 2524-2538, 11569-11572; sealed.go 178/225 |
| Pool / scratch aliasing copies (result of a reused tape or buffer copied out) | 9 | sealed.go 58-59; invoke.go 411-412; engine_pool.go 61; registry.go 1934; loop.go 102; engine.go 7578-7579, registry.go 1851, core_helpers.go 484-486/538 (argsCopy); engine.go 626 |
| Derived-structure copies of a fn value's payload (compiled dispatch form) | 3 | engine.go 7339-7392 compileFnDef (`compiled := sig; compiled.Params = append(...)`), 6458 (`rfn := *fn`), registry.go 1134-1138 (`top.Data = fnDef` + Replace) |
| Read-only convenience copies (`.Slice()`, `.Keys()`, diagnostic tuples, recorder operand lists) — performance only | ~24 | engine.go 3800, 4586, 6118, 6460, 7078, 6064, 10288, 10997, 11357, 4506 (rrValues), 10684-10692 (resolvedScratch); fn_capture.go 167, 377, 393; registry.go 2369, 744; trace.go 81, 132; fn_def.go 74; dispatch_layout.go 166, 214; optimistic_match.go 103; dispatch_explain.go 100/104 |

Total explicit sites catalogued: ~120, of which ~56 are judged LOAD-BEARING, ~14 unsure,
the rest incidental (identical under in-place mutation) or pure performance.

### Top 5 risks of in-place mutation in these files

1. **Occurrence state lives in `Value` fields.** `pos`, `Quoted`, `Eval`, `Undefined`,
   `ReachGroup`, `FailedDispatch`, `asc`, `Carrier`/`Dynamic`/`dynFrom`, and the payload
   one-shots `FnDefInfo.Applied` / `ClosurePayload.SigMatched` / `RetTrim` describe *this
   occurrence* of a value on the tape, not the value. Every read of a binding
   (`Defs.Top`, deftable.go 94/96), of a container element (`dot`/`get` results), of a
   module export, of a captured value and of a type node (`NewTypeLiteral` = `*t`) is a
   104-byte copy precisely so that the ~30 per-occurrence writes listed below
   (engine.go 2957, 3160, 3224, 3320, 3826, 5007, 6231, 7094, 7459, 7837, 8420, …;
   collect_kernel.go 353, 932, 950) never reach the shared original. The comments say so
   explicitly: "the Quoted-transience discipline", "the tag must never ride into a
   binding or a container" (NUR035/NUR038), "a one-shot signal must never ride into a
   binding" (`Applied`), "Tag a COPY and write it back" (3247). Under in-place mutation a
   dot-read fn is tagged `ReachGroup` forever in its map, `f/v apply` marks the binding's
   `FnDefInfo` as Applied for every later `f/v`, a list argument becomes `Quoted` in the
   caller's binding (core_helpers.go 500/561, registry.go 1882 — closure-capture walks
   and the residual sweep then skip it), and every read of `x` reports the def site's
   position. **Nothing in the engine can replace these without an occurrence layer**:
   the tape cell (and the arg slot, the capture slot, the args-list element) must carry
   the per-occurrence flags and the position beside a pointer to the value.
2. **Program tokens are re-run and the tape consumes/rewrites its cells.** fn bodies
   (`sig.Body()` appended per call, engine.go 7637, registry.go 1896, core_helpers.go
   425/595), loop bodies (`Loop.Body`/`WhileCond`, 9087-9088, 9220-9224; `MarkInfo.Body`,
   9027), `if` arms (`IfCont.Then/Else`, 9274), quotation lists (`bodyTokens` returns
   `lst.elems` directly, loop.go 318), pending container literals (elements re-stepped per
   evaluation, 5300, sealed.go 118-119; members 5982/6030), `ParenExpr` items (1990, 4803,
   5633) and reach receivers (5661) are all spliced onto the tape as copies and the cells
   are then *rewritten*: `forceStackWord` replaces a word with its `/s` form (4600-4604),
   `/q` conversion replaces a Word with an Atom (collect_kernel.go 896-899), a data-splice
   word is rewritten to a `ParenExpr` (collect_kernel.go 469-473), `Eval=false` after
   evaluation (3826, 7459), `Quoted=true` on list params. Replacing a cell's *pointer* is
   policy-compatible; writing the flag through the pointer is not, because the second
   execution of the body would see `[1 add 2]` frozen to `[3]`, a word already forced to
   stack mode, a list already quoted.
3. **Concurrency.** `ForkConcurrent` (fork.go 39-40) shallow-clones the `DefTable` so a
   fork "observes the parent's bindings as of the fork call; its later writes stay
   private" — with shared Value objects every in-place flag/pos write on either side is a
   data race under `-race`. `buildFnBodyHandler`'s leaf skeleton is shared by every fork
   ("The per-call COPY is mandatory: execMatch's stampResultPos mutates the returned
   slice, and ForkConcurrent engines share this handler", core_helpers.go 465-470;
   engine.go 1387-1390 stamps `pos` and re-mints the ReturnCheck in that copy). The
   `compiledFnDefFor` cache (fn_value_compile_cache.go) and `BoruImpl.compiled` are shared
   across every copy of a fn value; `rfn := *fn` (engine.go 6458) exists so a check-mode
   recovery can retarget `Registry` without corrupting the cached form.
4. **Sandboxes and rollbacks assume the check pass may scribble on copies.** `CompileSandbox`
   (compile_sandbox.go 54-57, 95-112), `DefTable.Clone` / `SnapshotEntries` /
   `RestoreEntriesSnapshot` / `ChangedSince` (deftable.go 318-440, 523-544), the predicate
   sandbox (util.go 391-407) and `withRecoveryRaw` (engine.go 11565-11584) restore
   *entries* or *cells* and rely on the restored Values being untouched by the region.
   The check pass writes `top.Quoted=false` (3224), `DynFrom` (check_recovery.go 82-88 via
   `&tagged`, 3247-3250), `Dynamic=true` (compiler kept_defs.go 394 via `&tagged`, 2952),
   `FailedDispatch` (7094), `GuardFactInfo` payload wrapping (2701) onto read copies today;
   under in-place mutation those survive every rollback. The sandbox's own comment records
   this exact failure once already for `CheckState` ("The old by-value copy reasoning…
   no longer holds").
5. **Type literals are node copies.** `NewTypeLiteral(t)` returns `*t` (value.go 2803-2808);
   `Defs.Top` for a type binding (deftable.go 94), `stepWord` (3158-3160, 3319-3320) and
   `resolveWordValue` (resolve.go 51) place a *copy* of the canonical lattice node and
   then stamp the read site's `pos` on it (and, downstream, `Quoted`/`Eval`/`asc`). With
   one object per node the canonical `TypeTable.byID` node would be stamped with
   positions and occurrence flags by every mention. (The converse is a gain: the orphan
   `&v` non-canonical `*Type` problem that `CanonicalType` papers over disappears when a
   type literal *is* its node.)

### Copies that exist purely for performance (would be deleted, not replaced)

- Tape cell traffic: `Tape.At` returns a 104-byte copy (tape.go 284; the
  `pendingForwardIdx` comment measured 36% duffcopy on a flat program, the `typeMeta`
  comment ~15%), `Set`/`Insert`/`Splice` copy in (300, 393, 441), `MoveGap` moves cells
  across the gap (316, 321), `grow` copies both halves (350-353). A `[]*Value` tape turns
  every one of these into an 8-byte move.
- Double copies: `SpliceExpand` copies `elems.Slice()` which already copied (engine.go
  4729-4730); `stepMove` copies `markInfo.Body` and then `Splice` copies again (9027-9029);
  `stepMoveCont` stores a *dead* copy of the body in each new `NewMark` (9087; value.go
  3155-3156) while `Loop.Body` already keeps it; staging buffers `loopTokens`,
  `peScratch`, `rrValues/rrReordered` are each copied once more by `Splice`/`Set`;
  `RunResolved` builds `input` (engine_pool.go 49-51) and `NewTapeWith` copies it again
  (tape.go 164-165); `CallBoru` appends the body (registry.go 1896) and the tape copies it
  again; `fn_def.go` 74 copies the body out of the source list at construction.
- Read-only walkers that copy: `ReadList.Slice()` in `WalkBodyWords`/`walkBodyValue`
  (fn_capture.go 167), `CollectBodyLocalDefs` (377, 393), `exprHasEffect` (engine.go 6118),
  `predicateBodyPure` (registry.go 2369), `resolveWordsDeep` (resolve.go 81), the trace
  renderer (trace.go 81, 132), `BodyTokens` (invoke.go 423).
- Payload-by-value boxing: every `v.Data.(FnDefInfo)` assertion copies a ~300-byte struct
  out of the interface (engine.go 7295-7305 `fnDefAtPointer`, 7482, 7592, 7836, 8384,
  8416, 10038, …); `ForwardInfo` is copied out by `AsForward` and re-minted on every
  collected argument (5103, `NewForward(fwd)`); `ChildTypeInfo`/`DisjunctInfo` are copied
  to replace one field (5154, resolve.go 108).
- Fresh carriers re-IDed after `NewCarrier` already minted an ID (7216-7217, 9538-9539,
  9653-9654, 9764-9765, 9829-9830, 5481).

### The cross-cutting shape the maintainer should plan for

Every load-bearing site below reduces to one of three facts:

- (A) a **per-occurrence attribute** is stored in the Value (`pos`, `Quoted`, `Eval`,
  `ReachGroup`, `Undefined`, `FailedDispatch`, `asc`, check-mode `Carrier`/`Dynamic`/
  `dynFrom`, `Applied`, `SigMatched`/`RetTrim`) — needs a slot/cell record;
- (B) a **token run is re-executed** and the executing copy is consumed or rewritten —
  needs the tape cell to be a *slot* distinct from the token (pointer replacement is fine,
  field writes through it are not), or needs the re-executed structures to be immutable
  by construction;
- (C) a **snapshot semantics** (fork, sandbox, capture, args list, `recoveryRaw`,
  `MarkInfo.Body`) relies on copy independence — needs either immutability of the shared
  object or an explicit copy-on-write at the mutation site.

Construction-from-parts already covers the evaluated-container family (`NewList(result)`,
`NewMap(out)`, `NewTypedList…`, `NewAtom`, `NewForward`, the frame markers): those sites
mint fresh Values and would be unchanged under the policy.

---

## engine.go

| line | site (function, one-line what) | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 626 | `runPrefix`: `kept := append([]Value(nil), prefix[:i]...)` — filtered copy of `Tape.Prefix` (a zero-copy VIEW of the tape buffer, tape.go 459) | explicit-copy | yes (aliasing) | the tape's backing array: appending to the view would write into the gap | still a slice copy under `[]*Value`; not a Value copy |
| 1091 | `attemptedWindowSplit`: `atom := NewAtom(w.Name); atom.pos = tape.At(pointer+1).pos` | construction | no | — | unchanged |
| 1163 | `noteCallWindow`: `win = []Value{}` non-nil-empty sentinel | by-value-flow | no | — | `[]*Value{}` |
| 1375-1399 | `stampResultPos`: `vals[i].pos = pos` (Function results) and `vals[i] = NewReturnCheck(rc)` with `rc.Pos` set — mutates the handler's result slice | explicit-copy (field write on result copies) | **yes** | the leaf-frame SKELETON shared by every call of the fn and by every ForkConcurrent engine (core_helpers.go 465-476: "The per-call COPY is mandatory: execMatch's stampResultPos mutates the returned slice"); the ReturnCheck's zero `Pos` is the "stamp later" protocol (fn_frame.go FrameTailSpec.Pos doc) | the ReturnCheck can be minted per call from parts (`NewReturnCheck` with the call-site pos); the Function-result `pos` needs an occurrence slot |
| 1441 | `stripTapeAscriptions`: `e.Tape.Set(j, StripAscribed(e.Tape.At(j)))` on frame returns | explicit-copy | no | frame return cells are the body's own results; a binding never carries `asc` (every delivery strips) | in-place `asc=nil` on the cell's value is behaviourally identical; `asc` is occurrence state anyway |
| 1731-1733 | `Run`: `prog = make(...); copy(prog, input); resolveAtomReferents(e.Registry, prog)` — private copy of the program so referent stamping never mutates the caller's slice | explicit-copy | **yes** (top level only) | the caller's parsed program (`RunProgram` re-runs, `check` then `run`, test fixtures); note the copy is SHALLOW: `resolveAtomReferents` 1616-1633 recurses into `ListPayload.Elems` and stamps nested lists IN PLACE through the shared backing array, so nested atoms are already mutated in the caller's program today | `SetAtomReferent` (value.go 2761-2771) mints the stamped atom by copy; under the policy the referent would be written into the program's atom (Data payload) — acceptable only if atom referents are allowed to persist across runs (they are a snapshot of the binding at load time, re-run semantics change) |
| 1744-1748 | `Run`: `e.Tape.Reload(prog)` / `NewTapeWith(prog, …)` copy the program onto the tape | explicit-copy (tape.go 164-165, 208) | **yes** (B) | the program/body slice: tape cells are consumed, replaced and flag-rewritten | a `[]*Value` tape still copies the pointers; flag writes must move to the cell |
| 1818 | `Run` trace: `snapshot := e.Tape.Snapshot()` | explicit-copy | no (unless the trace host retains) | — | `[]*Value` snapshot; debugger would see live objects |
| 1990, 4803 | `stepToken`/`stepLiteral`: `Splice(…, e.expandParenExprScratch(items)...)` — `ParenExprPayload.Toks` copied between fresh markers onto the tape | explicit-copy | **yes** (B) | the ParenExpr token (program; map-member groups re-run per evaluation) | cells as slots |
| 2008, 4811 | `expandReach(info)` → `lowerReach` 5659-5693: `out = append(out, info.Receiver...)`, `seg.KeyLit` appended, fresh `dot`/`dotr` words via `WithPos(NewWord(…), segAnchor)` (5678-5680) | explicit-copy + construction | **yes** (B) for Receiver/KeyLit | the Reach node's own tokens, re-lowered on every evaluation; the lowered cells get consumed (`dot` strips asc, `Eval=false` on the key) | cells as slots; the dot words are already minted fresh |
| 2524-2538 | `evalParenGroupAt` (check mode): `guardToks = append(guardToks, v)` snapshot of the group's tokens before the collapse destroys them | explicit-copy | unsure | the tokens are also spliced/removed by the collapse; stored into `GuardFactInfo.Toks` on the result carrier for guard narrowing | pointer snapshot survives removal, but any in-place flag write during the group's run would be visible in the guard fact |
| 2633, 2643 | `result.pos = v.pos` on a freshly evaluated InterpString/Xml value | construction | no | — | unchanged (pos still an occurrence attribute) |
| 2699-2702 | `res := e.Tape.At(scanIdx); res.Data = GuardFactInfo{Toks, Prev: res.Data}; e.Tape.Set(scanIdx, res)` — wraps a Boolean carrier's payload | explicit-copy (copy-modify-writeback) | unsure → yes when the survivor is shared | normally the dispatch's own fresh carrier; a 2+-token group that reduces to a def-bound carrier (`(x nop)`) would wrap the BINDING's payload | GuardFactInfo belongs on the cell, or mint a fresh carrier from parts carrying Prev |
| 2771-2822 | `stepWordUsurp`: `v := ResolveUsurp(…)` (fresh usurp wrapper, core_ref.go 447-457) then `v.pos = val.pos` | construction + occurrence write | no (wrapper is fresh) | — | pos → occurrence slot |
| 2776-2779, 2808-2811, 2911-2914, 3401-3403 | fresh `Undefined` placeholder atoms (check mode) | construction | no | — | unchanged |
| 2862 | `stepWordVal`: `v, ok := ResolveRef(…)` — `top` copy of `DefEntry.Body` (core_ref.go 26-52) or a fresh `NewFunction(*fnDef)` wrapper | explicit-copy (binding read) | **yes** (A) | the def binding; every later read; closure captures of the same name | needs occurrence slot for the writes at 2952-2957 |
| 2907 | `deliverValRead(WithPos(cv, val), val)` — check-mode fn-carrier side-table value copied with the read's pos | explicit-copy | **yes** (A, check mode) | the per-pass side table (`CheckFnCarrierBind`), read by every mention of the name | occurrence slot |
| 2952-2954 | `tagged := v; NoteValReadLive(&tagged, …); v = tagged` — check mode tags a COPY ("&v toward the recorder would heap-allocate every /v read"); `NoteValReadLive` may set `v.Dynamic = true` (compiler/go/kept_defs.go 394) | explicit-copy (copy-modify) | **yes** (A/C, check mode) | the binding (and the compile sandbox's rollback) | per-read carrier minted from parts, or occurrence-level `Dynamic` |
| 2957 | `v.pos = val.pos` then `deliverValRead` → `e.Tape.Set(e.Pointer, v)` | occurrence write on the read copy | **yes** (A) | the binding; error positions of every other read | occurrence slot |
| 3158-3160 | `stepWord` type-binding read: `push := NewTypeLiteral(entry.TypeDef)` (= `*t`, value.go 2803-2808) then `push.pos = val.pos` | explicit-copy (canonical node copy) | **yes** (A) | the canonical lattice node (`TypeTable.byID`, shared `tmeta`) | a type literal must be the node pointer; pos → occurrence slot |
| 3170 | `top, ok := e.Registry.Defs.Top(w.Name)` — value binding read (deftable.go 96 returns `e.Body` by value) | explicit-copy (binding read) | **yes** (A) | the def binding, captures, the args list, containers holding the same value | occurrence slot for every write that follows (3224, 3247, 3252→execMatch 3761/3826, stepLiteral 5007, buildFnBodyHandler 500) |
| 3196-3199 | `pe := NewParenExpr([]Value{val}); pe.pos = val.pos; e.Tape.Set(…)` — data-splice word rewritten as `(w)` | construction (cell replacement) | no | — | pointer replacement of the cell is fine |
| 3224 | `top.Quoted = false` (check mode: a quoted fn-carrier binding is read unquoted) | occurrence write on the read copy | **yes** (A/C) | the binding (NUR280 comment); the compile sandbox | occurrence slot |
| 3247-3250 | `tagged := top; CheckBraid.TagCheckModeDefRead(e, &tagged, …)` → `top.SetDynFrom(name)` + `NoteLiveRead(top,…)` (check/go/check_recovery.go 82-96) | explicit-copy (copy-modify) | **yes** (A/C, check mode) | the binding: `DynFrom` is "the binding NAME a dynamic carrier was resolved from"; the same object bound under a second name (`def y x`) would carry the wrong name | occurrence-level `dynFrom` |
| 3252 | `e.Tape.Set(e.Pointer, top)` — the read copy enters the tape | explicit-copy | **yes** (A) | as 3170 | — |
| 3319-3320 | `lit := NewTypeLiteral(t); lit.pos = val.pos` (builtin type name) | explicit-copy (canonical node copy) | **yes** (A) | the builtin canonical node | as 3158 |
| 3396 | `cv = WithPos(cv, val)` (check-mode fn-carrier read) | explicit-copy | **yes** (A, check) | the side table | occurrence slot |
| 3550-3553, 6605-6611, 6643-6647, 7021-7033 | `match.Args[i] = e.Tape.At(pos)` / `args[j] = e.Tape.At(…)` — the handler's `args []Value` built from tape cells | explicit-copy (arg delivery) | **yes** (A) for the cells that are binding copies; no for fresh results | the tape cells are consumed by the splice, but their values may be shared with a binding/container; `execMatch` then writes 3761/3826/3840/3850 and handlers may retain args (`def` binds args[1]; `set` stores) | `args []*Value` with the occurrence attributes stripped at delivery (the strip IS the arg-delivery contract) |
| 3636 | `ForwardOperandValue`: `WithPos(top, t)` — binding copy with the token's pos for the drift guards | explicit-copy | no (read-only consumer) | — | occurrence slot |
| 3761 | `execMatch`: `match.Args[i] = StripAscribed(match.Args[i])` | explicit-copy (value-receiver clears `asc`) | no | the arg copy only; an ascribed value cannot be a binding (every delivery strips) | in-place `asc=nil`; `asc` is occurrence state (`as` word result) |
| 3800 | `WalkBodyWords(lst.Slice(), …)` (check mode use-scan) | read-only copy | no | — | drop the copy |
| 3820-3824 | `evaluated, err := e.autoEvalList(match.Args[i], true); match.Args[i] = evaluated` (and AutoEvalMap 3807-3813) | construction replacing the pending literal in the ARG slot | **yes** (B) that the ORIGINAL stays pending | the pending literal token in a fn/loop body is evaluated per call | already construction; the token itself is untouched — keep |
| 3826 | `match.Args[i].Eval = false` | occurrence write on the arg copy | **yes** (A/B) | for a NoEval slot (code body passed to `if`/`for`/`do`) the arg IS the program's pending list token; `isPendingResidualContainer` (8824-8839) keys on `Eval` — clearing it on the token would stop the next encounter (loop re-run) from auto-evaluating | occurrence slot |
| 3840 | `match.Args[i] = WithPos(NewDynamicCarrier(TAny), match.Args[i])` (check-mode Undefined placeholder) | construction | no | — | unchanged |
| 3850 | `FillConcreteOptionDefaults(pat, match.Args[i])` (unify_options.go 128, other reviewer) may build a defaults-filled map | construction (cross-ref) | unsure | the caller's map (a binding's copy) | construction |
| 4051-4053 | `info.Loop.recName, recArity, recorder = …` — writes THROUGH the `*Loop` in the mark's payload | by-value-flow (pointer payload) | no | — | unchanged |
| 4103 | `e.stampHandlerResults(results)` → 1375-1399 | see 1375 | **yes** | — | — |
| 4454, 7532, 7682 | splice compaction `e.Tape.Set(dst, e.Tape.At(i))` | explicit-copy (cell move) | no | — | pointer move |
| 4506-4535 | `rearrangeForForward`: cells extracted into `rrValues`, permuted into `rrReordered`, written back with `Set` | explicit-copy (permutation through engine scratch) | no | pooled sub-engines retain the buffers → `PutSubEngine` clears them (registry.go 665-668) | permute pointers |
| 4586 | `resolvedStackBeforeFrom`: `stack = append(stack, e.Tape.At(i))` for FullStack handlers | read-only copy | no | — | `[]*Value` |
| 4600-4604 | `forceStackWord`: `nw := NewWordModified(name, argc, true, false); nw.pos = &pos; e.Tape.Set(idx, nw)` — re-mints the word with `ForceStack` | construction (cell replacement) | **yes** that it is NOT an in-place write | the program's word token (fn body re-run would find itself already `/s`) | keep as replacement; note `&pos` allocates a new SrcPos per call (perf) |
| 4607-4647 | `insertForward`: fresh `NewForward(ForwardInfo{…})` | construction | no | — | unchanged; a pointer payload would let 5103's re-mint become an in-place counter update |
| 4729-4730 | `SpliceExpand`: `out := make(…); copy(out, elems.Slice())` (double copy) | explicit-copy | **yes** (B) that the payload is not spliced directly; the second copy is perf | the splice payload (`def m word [...]`), a Forth macro re-expanded on every mention | one pointer copy; cells as slots |
| 4872-4879 | `expanded := SpliceExpand(info.Data); markReStepped(el); Splice(valIdx, 1, expanded...)` | explicit-copy | **yes** (B) | as above | — |
| 4896-4898 | `prev := e.Tape.At(valIdx-1); prev.Quoted = true; e.Tape.Set(valIdx-1, prev)` (check mode: quote the carrier before a `/v` marker) | explicit-copy (copy-modify-writeback) | unsure | a def-bound carrier read `m.f/v` → the carrier may be the binding's copy | occurrence slot |
| 5007-5026 | `stepLiteral` collection: `val := e.Tape.At(valIdx); val.ReachGroup = false; e.Tape.Remove(valIdx); … e.Tape.Insert(insertIdx, val)` | explicit-copy (copy-modify, move) | **yes** (A) | the collected value is a copy of a container element / module export / binding tagged at 7837; "the tag must not ride into the binding" | occurrence slot |
| 5103 | `e.Tape.Set(fwdIdx, NewForward(fwd))` — `ForwardInfo` copied out by `AsForward`, counters bumped, re-minted | explicit-copy (payload copy-modify-remint) | no | — | pointer payload mutated in place (policy-friendly) |
| 5154-5158 | `resolveInertTypeShape`: `nd := ci; nd.Child = rc; out := v; out.Data = nd` | explicit-copy (payload + value copy) | **yes** (B) | the typed-container token (`[:Foo]` in a body / a def-bound shape) must stay unresolved so a later consumption under a different `Foo` binding resolves afresh (the "freeze discipline") | construction from parts (`NewTypedList…`), as resolve.go 118-142 already does |
| 5192 | `autoEvalStack`: `e.Tape.Set(i, result)` — residual replaced by its evaluation | construction | no | — | unchanged |
| 5287-5294 | `autoEvalList` stepless path: `out := elems.Slice(); out[i] = StripAscribed(el); return NewList(out)` — "a FRESH list over a copy of the elements … two calls' values are not one (`(mk) eq (mk)` is false)" | explicit-copy + construction | **yes** (language semantics: container identity per evaluation) | the pending literal's elements; `eq` identity of lists | `NewList` over a `[]*Value` copy of the element pointers keeps the identity semantics; the per-element `StripAscribed` must become an occurrence-level strip |
| 5298-5301 | `input := elems.elems; if !sealsLiterals { input = elems.Slice() }` — elements handed to the sealed region (copied onto the tape by sealed.go 118-119/159) or to a sub-engine (copied by NewTapeWith) | explicit-copy | **yes** (B) | the literal's elements, re-evaluated per call | cells as slots |
| 5317-5322 | `result[i] = StripAscribed(result[i]); resolveInertTypeShape; out := NewList(result)` — result is the region's residual COPY (sealed.go 58-59) | construction | no | — | unchanged |
| 5481 | `EvalXmlInterp`: `out := NewCarrier(TXml); out.ID = GenerateID(…)` | construction (double ID mint) | no | — | drop the re-mint |
| 5633-5636, 5645-5657 | `expandParenExpr` / `expandParenExprScratch`: items copied between fresh markers | explicit-copy | **yes** (B) | ParenExpr items (program) | cells as slots |
| 5661 | `lowerReach`: `out = append(out, info.Receiver...)` | explicit-copy | **yes** (B) | Reach node tokens | cells as slots |
| 5713 | `expandReach`: `span[0].ReachGroup = true` on the FRESH open marker | construction | no | — | unchanged |
| 5848 | `foldedReferenceIdentity`: `folded.ID = result[0].ID` (check mode) — rewrites the identity of the const-fold's value | explicit-copy (ID write on a fresh value) | no (fresh) | — | unchanged, but shows ID is a mutable field standing in for identity |
| 5982 | `AutoEvalMap`: `out.Set(resolvedKey, v)` — scalar member copied into the NEW map | explicit-copy + construction | no (the new map is this evaluation's own; the pending map token stays pending) | — | `OrderedMap` of pointers |
| 6025-6030 | `out.Set(k, StripAscribed(mv))` on the new map; `res := NewMap(out)` | explicit-copy on the fresh map | no | — | occurrence-level strip at the store boundary |
| 6064-6067 | recorder: `vals := make(…); vals[i], _ = out.Get(k)` | read-only copy | no | — | drop |
| 6118, 6122 | `exprHasEffect`: `walk(lst.Slice())`, `mp.Keys()` | read-only copy | no | — | drop |
| 6231-6233 | `takeAppliedMark`: `fd.Applied = false; val.Data = fd; e.Tape.Set(valIdx, val)` — one-shot flag cleared on a COPY of the `FnDefInfo` payload and re-boxed | explicit-copy (payload copy-modify) | **yes** (A) | the fn value's payload is shared by the binding/container; `MarkApplied` (8415-8423) sets the flag on the `apply` word's RESULT copy only — in place, `f/v apply` would leave the binding Applied | `Applied` becomes an occurrence flag; `FnDefInfo` as a pointer payload must then never carry it |
| 6245-6252 | `execFnDefLiteral`: `val := e.Tape.At(valIdx); if val.ReachGroup { val.ReachGroup = false; e.Tape.Set(valIdx, val) }` | explicit-copy (copy-modify-writeback) | **yes** (A) | as 5007 | occurrence slot |
| 6256 / 7295-7305 | `fnDefAtPointer`: `val = WithPos(bridged, val)` — a compiled closure bridged to a FRESH FnDef value that is deliberately NOT written back ("the TAPE keeps the closure itself … a parked copy could escape into a binding and keep applying through a finished run's invoker", Codex P2 #444) | construction; relies on the cell being a copy | **yes** (C) | the closure value in its binding/container | keep the bridge as a side value; never write through the cell |
| 6346-6349, 6356 | `fn = compiledFnDefFor(reg, fnDef)` → `compileFnDef` 7339-7392: `compiled := sig; compiled.Params = append([]FnParam(nil), sig.Params...)`, `NormalizeSig(&compiled)`, `SortSignatures(out)` into a NEW `FnDefInfo` | explicit-copy (derived dispatch form of the payload) | **yes** | the authored `Signatures` slice is read by `OwnSigs` consumers (canon, inspect, targeted undef, overlap) and is the cache key (`sameList` by backing array, fn_value_compile_cache.go 32-37) | this is a derived structure, not a Value copy; keep (out of the policy's letter), but note it copies `Params` per compile |
| 6458-6460 | `rfn := *fn; rfn.Registry, _ = FnHome(…)`; `UncalledRaisePos(…, append(append([]Value{}, resolved...), upcomingArgs...))` | explicit-copy (FnDefInfo copy; diagnostics slice) | yes for `rfn` (the compiled form is CACHED and shared — mutating `Registry` in place would corrupt other dispatches); no for the slice (protects `resolvedScratch` from append aliasing) | the `compiledFnDefFor` cache | pass the registry separately |
| 7078 | `candidates := append(append([]Value{}, resolved...), upcomingArgs...)` | read-only copy (protects the scratch buffer) | no | — | `[]*Value` copy |
| 7092-7095 | `fv := e.Tape.At(valIdx); fv.FailedDispatch = true; e.Tape.Set(valIdx, fv)` (check mode wreckage marker) | explicit-copy (copy-modify-writeback) | unsure → yes when shared | a fn value read from a container (`m.f`) — in place the stored element would read as wreckage to `defWordExtension` | occurrence slot |
| 7211-7217 | `recordUndecidedApply`: argVals copies; fresh `out` carrier with `out.ID`/`out.pos` | construction | no | — | unchanged |
| 7424 | `execFnDefSig`: `args[i] = StripAscribed(args[i])` | explicit-copy on the arg slice | no | — | occurrence strip |
| 7433-7448 | `evaluated := AutoEvalMap/autoEvalList(args[i]); args[i] = evaluated` | construction | **yes** (B) that the token stays pending | pending literal in a body | keep |
| 7459-7460 | `args[i].Eval = false; args[i].Undefined = false` | occurrence writes on arg copies | **yes** (A/B) | as 3826 | occurrence slot |
| 7482, 7592 | `captures = fd.Captured` via `e.Tape.At(valIdx).Data.(FnDefInfo)` — whole payload struct copied out of the interface | by-value-flow (payload boxing) | no (read) | — | pointer payload |
| 7498 / 7577-7580 | `args = RetagTypedContainerArgs(sig.Params, args)` (core_helpers.go 721-735 copy-on-write clone; `RetagFlexElem` 671-672 copies the flex HEADER to attach `elem`; `RetagTypedContainerValue` 659 `unified.Quoted = arg.Quoted`); `argsCopy := make(…); copy(argsCopy, args); e.Registry.Args.Push(NewList(argsCopy))` | explicit-copy | **yes** (A) for the retag (`elem` is a per-binding constraint attached to a header copy sharing the flex store — the caller's untagged value must stay untagged); yes (C) for `argsCopy` (argsstack.go 56-58: "vals must stay unchanged while the entry is on the stack: the caller hands over a copy it owns") | the caller's flex/list value; the `args` word's list | `elem` becomes a binding-slot attribute; the args list becomes a `[]*Value` the frame owns |
| 7597, 7608 | `InstallFrameBinding(e.Registry, cb.Name, cb.Value)` / `(p.Name, RetagTypedContainerParam(p, args[i]))` — captures and params pushed as bindings (deftable.go 119 stores by value) | explicit-copy (binding install) | **yes** (A) with the `Quoted=true` list-param write in the handler twins (core_helpers.go 496-500, 561; registry.go 1880-1884) | the caller's list value (`def xs [1 2]  f xs`): `Quoted` on the shared object would make `WalkBodyWords` skip it (closure capture analysis), the sweep skip it, `isPendingResidualContainer` false | binding-slot `Quoted` |
| 7611 | `tokens = append(tokens, args[i])` — unnamed args spliced at the frame head | explicit-copy | no (frame-head cells are inert: `FrameOpenInfo.ArgSpan` skips them) | — | `[]*Value` |
| 7637 | `tokens = append(tokens, sig.Body()...)` — "append COPIES them into tokens' backing array, and sig.Body() (the shared BoruImpl.Body) is never mutated here" | explicit-copy | **yes** (B) | `BoruImpl.Body` shared by every call, every copy of the fn value, every fork | cells as slots |
| 7624-7636 | `AppendFrameTail` / `NewCloseParen` | construction | no | — | unchanged |
| 7831-7838 | `tagReachCollapsedFn`: `v := e.Tape.At(idx); v.ReachGroup = true; e.Tape.Set(idx, v)` | explicit-copy (copy-modify-writeback) | **yes** (A) | the dot-read's result is a copy of the container element/module export; "The tag is transient, exactly like Quoted … so it never rides into a binding or a container" | occurrence slot |
| 8415-8423 | `MarkApplied`: `fd.Applied = true; v.Data = fd; return v` (shared by `apply`'s handler, its check model, the VM op) | explicit-copy (payload copy-modify) | **yes** (A) | the binding's FnDefInfo | occurrence flag |
| 8508-8609 | `commitBarrierForward`: `resolved = append(resolved, v)` probe copies; `forceStackWord`; rearrange | read-only copy + see 4600 | no / yes | — | — |
| 8880-8911 | `stepDefCleanup` EvalResidual: `ev := AutoEvalMap/autoEvalList(v); ev.Eval = false; e.Tape.Set(i, ev)` | construction | no (fresh) | — | unchanged |
| 8924 | `e.Tape.Set(markerIdx, spentFrameMarker)` — a package-global Value copied into the cell ("One shared Value — the cell is overwritten by copy, and nothing keys on a marker's identity", fn_frame.go 235-240) | by-value-flow | **yes** (C) | every spent frame would point at the one global; any later write through a cell (a `pos` stamp, a Set of a field) corrupts all frames | mint a spent marker per frame, or make the global immutable by construction |
| 9027-9029 | `stepMove`: `body := make(…); copy(body, markInfo.Body); Splice(markIdx, …, body...)` (double copy) | explicit-copy | **yes** (B) for the replay; the explicit copy is redundant with Splice's | `MarkInfo.Body` (the original to replay) | one pointer copy; cells as slots |
| 9083-9088 | `stepMoveCont`: `tokens = append(tokens, NewMark(id, body...)); tokens = append(tokens, body...); …Splice` — `Loop.Body` replayed per iteration (NewMark copies the body AGAIN into the new mark, value.go 3155-3156, a dead copy since `cont.Body` keeps it) | explicit-copy | **yes** (B) | `Loop.Body` shared across iterations | cells as slots; drop the dead MarkInfo.Body copy |
| 9113, 9207, 9384, 9443(loop.go) | `Splice(markIdx, …, cont.Results...)` — accumulated results copied onto the tape | explicit-copy | no | — | `[]*Value` |
| 9154 | `collectLoopRegion`: `cont.Results = append(cont.Results, v)` — region cells copied into `Loop.Results` (a by-value field) | explicit-copy | no | — | `[]*Value` field |
| 9182, 9247 | `var condResult Value; condResult = e.Tape.At(j)`; `condResult.Parent == nil` as "no value" | by-value-flow (zero sentinel) | no | — | nil pointer test |
| 9220-9224 | `spliceWhileRegion`: `NewMark(id, region...)` + `append(tokens, region...)` — `WhileCond`/`Body` replayed | explicit-copy | **yes** (B) | `Loop.WhileCond` / `Loop.Body` | cells as slots |
| 9274 | `stepMoveIf`: `Splice(markIdx, …, branch...)` — `IfCont.Then/Else` arm tokens copied onto the tape | explicit-copy | **yes** (B) | the arm lists (built by `if`'s handler from the program's arm literals; re-run inside loops) | cells as slots |
| 9452-9461 | `stepOpenParen`: `np := NewOpenParen(); np.ReachGroup = e.Tape.At(e.Pointer).ReachGroup; e.Tape.Set(e.Pointer, np)` — the program's `(` replaced by a fresh marker | construction (cell replacement) | no | — | pointer replacement |
| 9505-9570, 9650-9705, 9757-9789, 9791-9869 | `recordParen*Apply` (check mode): `argVals` copies, fresh `out` carriers (`out.ID = GenerateID; out.pos = …`), `e.Tape.Set(first/leadFn/w.lead/lastIdx, out)` | construction + read-only copies | no | — | unchanged |
| 9918-9924 | `evalTrailingContainer`: `v.Eval = !fits; return v, fits` → `e.Tape.Set(idxs[…], ev)` (check mode marks a NoEval container consumed by clearing `Eval` on the tape copy) | explicit-copy (copy-modify-writeback) | **yes** (A/B) | the pending literal token (program) | occurrence slot |
| 9942-9943, 10977-10978 | `ev.Eval = false; ev.pos = v.pos` on fresh evaluations | construction | no | — | unchanged |
| 10288 | `stepCloseParen` ReturnCheck: `results = append(results, e.Tape.At(j))` | read-only copy | no | — | drop |
| 10316 | `e.stripTapeAscriptions(openIdx+1+extra, closeIdx)` → 1441 | see 1441 | no | — | — |
| 10358 | `last := Value{}` sentinel; `es.ApplyPending(last.ID)` on the zero | by-value-flow | no | — | nil guard |
| 10496-10620 | `recordParenReStep`/`markReStepped`/`markForwardLeftover`/`noteTrailingDeferred`/`placeTrailingDeferred`: CheckState maps keyed by `v.ID` (`ParenReSteppedFnIDs`, `ForwardLeftoverFnIDs`, `TrailingDeferredFnIDs`, `ParenPlacedFnIDs`; also `WordReadFnIDs` 3023, `ReachSurvivorFnIDs` 7853, `StoodAsideLandingIDs` 4251) | by-value-flow (ID = identity shared by every copy) | **yes** (documented hazard) | `fnReturnPark` 7913-7970: "ParenPlacedFnIDs … is keyed by value ID, and an ID travels with a binding … `def h (mk 1) end h 2` now declines"; a pointer identity would make these marks stickier still unless keyed by occurrence | key the placement/re-step marks by tape occurrence, not by value identity |
| 10684-10692 | `EffectiveResolved`: `resolved := e.resolvedScratch[:0]; append(resolved, v)…; e.resolvedScratch = resolved` — reusable snapshot of the stack consumed by `PlanMatch` only; set aside per sealed region (sealed.go 274-284) | explicit-copy into a reused scratch | no (read-only) but a pool that reuses Value slots | the holder's match reads a VIEW of this buffer while a pattern evaluates a literal (sealedHold) | `[]*Value` scratch; the view-aliasing discipline stays |
| 10822-10827 | `curryOrStack`: `elems := make(…); append(elems, NewWord(w.Name)); append(elems, e.Tape.At(i)…); Splice(startIdx, collectedCount+1, NewList(elems))` — curry list over consumed cells | construction | no | — | `NewList` over pointers |
| 10907-10923 | `matchForDispatch`: `wDeep := w; wDeep.ForceStack = true` (WordInfo copy, not a Value) | — | no | — | — |
| 10958-10980 | `evalPatternOperand`: `ev := AutoEvalMap/autoEvalList(v); ev.Eval = false; ev.pos = v.pos; e.Tape.Set(i, ev); patternEvalDid = true` (NUR235) | construction replacing the pending operand before the match | **yes** (B) that the token is not evaluated in place | the pending literal token | keep |
| 10993-11007 | `SigOrderArgs`: reordered copy of args | read-only copy | no | — | `[]*Value` |
| 11357-11420 | `TryRecordUnmatchedDispatchTrap`: `vals = append(vals, v)` with `v = top` binding copies; `rematchRenderTuple` 11596 compares `window[j].ID == v.ID` | read-only copy; ID-identity | no / by-value-flow | — | pointer identity could replace the ID compare |
| 11552-11584 | `NoteRecoveryRaw` / `withRecoveryRaw`: `recoveryRaw map[int]Value` keeps the RAW pending literals a check-mode recovery replaced on the tape; `withRecoveryRaw` swaps them back (`evaluated[i] = e.Tape.At(i); e.Tape.Set(i, raw)`) for the report and restores | by-value-flow relying on copy independence (C) | **yes** (check mode) | the raw literal copy must be the UNEVALUATED token; if the recovery evaluated the token in place there is no raw to render | raw tokens are program tokens; keep them immutable and swap cell pointers |
| 3023, 4251, 4323-4332 | `noteWordRead` / `noteCollectedLandings` / `noteDefReadPos`: `defReads map[string][]SrcPos` keyed by `collected.ID` — "a binding's reads share one value, which carries no read's position of its own" (4337-4353) | by-value-flow | no (check mode) but documents that a read's position is NOT on the value | — | an occurrence-level pos would make `defReads` unnecessary |
| 8359 | `ForwardClaimProbeOn`: `Value{Parent: TFunction, Data: FnDefInfo{}}` — a synthesized probe literal | by-value-flow (inline struct literal) | no | — | construction |

Explicit-copy helpers from other files that the engine's judgements depend on (not
re-listed per site): `NewTypeLiteral` returns `*t` (value.go 2803-2808); `StripAscribed`
is a value-receiver that clears `asc` on the copy (value.go 2267-2272); `WithPos`/
`WithPosAt` copy and set `pos` (util.go 321-333); `NewMark` copies its body (value.go
3154-3158); `ReadList.Slice()` copies (value.go 51-55); `SetAtomReferent` copies the atom
and snapshots the referent (`snap := ref; ap.Referent = &snap`, value.go 2761-2771).

## sealed.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 52 | `sealedDriver.Next`: `return nil, Value{}, false, nil` | by-value-flow (zero sentinel) | no | — | nil |
| 57-61 | `Collect`: `d.out = make(…); copy(d.out, residual)` — "the slice is the loop's, valid for the call, and the result is the caller's — the copy runPooledSub made of a sub-engine's result stack" | explicit-copy (pool aliasing) | **yes** (C) for the slice | `Loop.Results` of the per-depth Loop, reset by the next evaluation at the same depth (loop.go 415) | still a slice copy under `[]*Value` (pointers copied, Values not) |
| 100-113 | `takeSealed`: one `*Loop` per nesting depth, minted once and reused; `drv.kind, drv.out, drv.done = kind, nil, false` | by-value-flow (pool) | no | — | unchanged |
| 117-121 | `sealedRegionTokens`: `lp.mint(0)`; `out := append(buf[:0], lp.mark, lp.open); append(out, toks...); append(out, lp.close, lp.move)` — the Loop's four minted tokens and the literal's elements copied into `e.loopTokens` | explicit-copy | **yes** (B) for `toks` (the pending literal's own elements, 5298); the minted tokens are reused Values whose `pos` points INTO `lp.Pos` (loop.go 301-303) | the literal's elements; the Loop's tokens | cells as slots; minted tokens could be referenced, not copied, if nothing writes through the cell |
| 158-159 | `evalSealed`: `e.loopTokens = sealedRegionTokens(…); e.Tape.Splice(at, 0, e.loopTokens...)` (second copy by Splice) | explicit-copy | as above | — | one pointer copy |
| 178, 225 | trace `e.Tape.Snapshot()` | explicit-copy | no | — | `[]*Value` |
| 263-290 | `sealedHold`: sets aside `resolvedScratch`, `parenEvalDepth`, `patternEvalMake/Did`, `voidGroups`, `recoveryRaw`, `recorder`; `drv.resolved` is the region's own resolved buffer | by-value-flow (scratch that reuses Value slots; "the holder's match reads its resolved stack through a view of this buffer") | **yes** that the holder's view is not rebuilt underneath it | the dispatch mid-match holding the literal | same swap with `[]*Value` buffers; `recoveryRaw map[int]*Value` |

## loop.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 102 | `DriveLoop`: `in := append([]Value(nil), inputs...)` — "The driver may reuse its inputs buffer across iterations; the seam below keeps the slice it is handed" | explicit-copy (slice aliasing) | yes (C) for the slice | the driver's inputs buffer | `[]*Value` copy |
| 137, 242 | `callDriver.Next` / `putCallLoop`: `Value{}` sentinels; `drv.inputs, drv.body, drv.out = nil, Value{}, nil` | by-value-flow | no | — | nil |
| 192 | `CallRegion`: `lp.toks = loopRegionTokens(lp.toks, lp, inputs, body)` — pooled Loop's reusable token run | explicit-copy into a pooled buffer | no | — | `[]*Value` |
| 205-250 | `takeCallLoop` / `putCallLoop` pool: `lp.Results[:0]`, `clear(lp.toks)`, `clear(lp.Results[:cap])` — "a pooled Loop must not pin the call's subject or its residual" | by-value-flow (pool reusing Value slots) | no | — | nil-ing pointers |
| 270-280 | `loopRegionTokens`: `out := append(buf[:0], lp.mark, lp.open); append(out, inputs...); append(out, lp.bodyTokens(body)...); append(out, lp.close, lp.move)` | explicit-copy | **yes** (B) for the body tokens: `bodyTokens` 316-323 returns `lst.elems` DIRECTLY ("the splice copies them into the tape; the list is never written") — the quotation list's own elements are replayed per element and the tape consumes/rewrites them | the quotation list (`[n mul 2]`, a program literal or def-bound list); the driver's resolved inputs | cells as slots |
| 286-309 | `mint`: `lp.mark = NewLoopMark(id, lp)`, `lp.close`, `lp.move`, `lp.open` minted once; `lp.mark.pos, lp.move.pos = &lp.Pos, &lp.Pos` (301), `lp.close.pos = &lp.Pos` (303) — tokens whose `pos` POINTS INTO the Loop struct so a reused call Loop's copies "take the new call's position" | by-value-flow (Value fields `mark/open/close/move` held by value; pos-pointer-into-struct trick that relies on the tape holding COPIES whose `*SrcPos` outlives the call) | **yes** (C) — documented: "its tokens point at the Loop's Pos whatever it holds now, as the Loop is reused call after call" | the pooled `Loop` (`callLoops`, `e.sealed`) | if the cell references the minted token itself, the trick still works (the pos pointer is on the token) but the token is then shared by every live region of that Loop — only one is live at a time by construction (sealed.go 95-99) |
| 320-321 | `bodyTokens`: `lp.one[0] = body; return lp.one[:]` — a non-list body copied into the Loop's own `[1]Value` | explicit-copy (by-value field) | no | — | `[1]*Value` |
| 395-404, 415-421 | `stepMoveDriven`: `lp.Results = lp.Results[:0]`; `collectLoopRegion` copies the region's cells into `lp.Results`; `lp.Driver.Collect(lp.Iter-1, lp.Results)` hands the driver the loop's OWN buffer ("valid only for the call") | explicit-copy + by-value field | yes (C) — a driver that keeps results must copy (sealedDriver does; callDriver keeps it because Finish/Splice follow at once, 145-153) | `Loop.Results` | `[]*Value` |
| 443, 457-458 | `Splice(markIdx, …, results...)`; `e.loopTokens = loopRegionTokens(…); Splice(…, e.loopTokens...)` | explicit-copy | as 270-280 | — | — |
| 520-545 | `abandonDrivenLoop` / `wrapLoopFault`: read `info.Cont` through the move's payload pointer | by-value-flow (pointer payload) | no | — | unchanged |

## tape.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 164-165 | `NewTapeWith`: `buf := make([]Value, initial); copy(buf, vals)` — "The input is copied, mirroring Engine.Run's copy of its program" | explicit-copy | **yes** (B) | the program / body / inputs slice handed in (every Run, every pooled sub-run, every CallBoru) | `[]*Value` buffer: pointer copy only; the cells must then never be written through |
| 208 | `Reload`: `copy(t.buf, vals)` — "vals is copied, so the caller's slice is never mutated" | explicit-copy | **yes** (B) | as above | as above |
| 280-284 | `At`: out-of-range → `Value{}`; else `return t.buf[t.phys(i)]` (104-byte copy per read) | by-value-flow + perf | no (sentinel) / perf | the never-panic ADR-005 contract relies on every `IsX` predicate answering false on the zero Value | nil-safe predicates for a nil pointer |
| 300, 393, 441 | `Set` / `Insert` / `Splice`: cells copied in; `forwards` counter maintained from the writes | explicit-copy | no | — | pointer stores |
| 312-325 | `MoveGap`: `copy` cells across the gap and `zero` the vacated run | explicit-copy (perf) | no | — | pointer moves |
| 350-353 | `grow`: both halves copied into the new buffer | explicit-copy (perf) | no | — | pointer copies |
| 410, 435, 488 | `t.buf[…] = Value{}` "release references" | by-value-flow (zeroing) | no | — | nil |
| 459 | `Prefix`: `return t.buf[:end]` — zero-copy VIEW of the live buffer | by-value-flow (aliasing) | yes (callers must not append into it: engine.go 626) | the tape | same discipline with `[]*Value` |
| 477, 484 | `CopyRange` / `Snapshot` fresh copies (trace, debugger, `CurrentStack`) | explicit-copy | no (read-only contract: "the live tape is never exposed or mutated", registry.go 706) | — | a pointer snapshot exposes live objects to the host |
| 496-499 | `TakeAll`: ownership transfer of the buffer ("the tape must not be edited afterwards") | by-value-flow | yes (C) — every pooled-engine caller copies the result out because the next `Reload` overwrites it (engine_pool.go 60-62, invoke.go 396-412, registry.go 1934) | the pooled tape | the result slice still aliases the buffer; keep the copies (of pointers) |

## registry.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 370-381, 515-541, 642-670 | `enginePool` / `TakeSubEngine` / `PutSubEngine`: pooled engines keep `rrValues`/`rrReordered` (cleared at 665-668 "release any Values still held"), but `resolvedScratch`, `loopTokens`, `peScratch`, `excludeScratch` are NOT cleared and pin values across reuses | by-value-flow (pool reusing Value slots) | no (leak-ish) | — | `[]*Value` scratch; clear all of them |
| 385-392 | `callLoops` pool (see loop.go) | by-value-flow | no | — | — |
| 598-612 | `RunningEngineStates`: `EngineState{Stack: e.Tape.Snapshot(), …}` | explicit-copy | no | — | `[]*Value` |
| 706-750 | `CurrentStack`: `snap := e.Tape.Snapshot()` (715); filtered `out = append(out, v)` (744) — "the live tape is never exposed or mutated" (706) | explicit-copy | no (read-only contract) | — | the host would see live objects |
| 1134-1138 | `upsertFnDef`: `fnDef.Signatures = append(fnDef.Signatures, sigs...); …; top.Data = fnDef; r.Defs.Replace(name, top)` — copy-modify-replace of the top binding's fn value; `append` on a backing array that other copies of the value may share (a `f/v` wrapper taken earlier: `NewFunction(*fnDef)` shares the slice) | explicit-copy (payload copy-modify) | yes (identity of the registration) — but the shared-backing-array append is an existing aliasing hazard | every copy of the fn value | pointer payload mutated in place |
| 1145 | `Signatures: append([]Signature(nil), sigs...)` | explicit-copy (sig slice) | no | — | — |
| 1382, 1471, 1489, 1513 | handler signature `ctx map[string]Value` | by-value-flow (map of Values) | no | — | `map[string]*Value` |
| 1427-1448 | `InitRootContext`: `Data: make(map[string]Value)`; `fsStore.Set("impl", NewTypeLiteral(TNone))` — a canonical node COPY stored in a Store | explicit-copy (node copy into a map) | yes (A) | the canonical `TNone` node | store the node pointer |
| 1577, 2119, 2142, 2147, 2274, 2289-2293, 2434-2437 | `Value{}` sentinel returns | by-value-flow | no | — | nil |
| 1839-1847 | `callBoruNamed`: `r.PushFnBaseline(r.Defs.Snapshot())` (depth map, not Values) | — | — | — | — |
| 1850-1856 | `args = RetagTypedContainerArgs(sig.Params, args)`; `argsCopy := make(…); copy(argsCopy, args); r.Args.Push(NewList(argsCopy))` | explicit-copy | **yes** (C) | the caller's args slice (the handler's `args`); the `args` word's list must be stable for the frame's life | `[]*Value` the frame owns |
| 1862 | `InstallFrameBinding(r, cb.Name, cb.Value)` — captured snapshot installed as a binding | explicit-copy (binding install of a by-value capture) | **yes** (A/C) | `CapturedBinding.Value` snapshot; the binding store | binding slot holds a pointer; capture semantics discussed under fn_capture.go |
| 1880-1884 | `arg := args[i]; if list && !arg.Quoted { arg.Quoted = true }; InstallFrameBinding(r, p.Name, RetagTypedContainerParam(p, arg))` — list params quoted on a COPY | explicit-copy (copy-modify) | **yes** (A) | the caller's list value (a def-bound list passed as an arg would become Quoted everywhere) | binding-slot `Quoted` |
| 1887 | `tokens = append(tokens, args[i])` (unnamed args as the inert prefix, `StartAt`) | explicit-copy | no | — | `[]*Value` |
| 1896 | `tokens = append(tokens, sig.Body()...)` — the shared body copied into the sub-engine's program (then `NewTapeWith` copies again) | explicit-copy | **yes** (B) | `BoruImpl.Body` | cells as slots |
| 1930-1942 | `result, err := sub.Run(tokens); if len(result) > 0 { result = append([]Value(nil), result...) }` — pooled-tape aliasing copy | explicit-copy | yes (C) for the slice | the pooled engine's tape (next Reload overwrites) | `[]*Value` copy |
| 1992 | deferred residual sweep: `result[i], err = sweep.autoEvalResidual(result[i])` | construction | no | — | unchanged |
| 2053 | `result[i] = StripAscribed(result[i])` (frame results) | explicit-copy | no | — | occurrence strip |
| 2256-2353 | `RunPredicate`: `predMemo` keyed by `predMemoKey(constraint, candidate)` (a rendered key, 2223); analysis arm `r.Defs.Snapshot()/Restore` (depth map) | by-value-flow (memo keyed by rendering, not identity) | no | — | — |
| 2369 | `predicateBodyPure`: `pure(lst.Slice())` | read-only copy | no | — | drop |
| 2402-2410 | `runPredicateBody`: `snapshotPredicateState` / `restorePredicateState` (util.go 391-407): `Types.Clone()`, `Contexts.Snapshot()`, `analysisSnapshot()` | explicit-copy (sandbox) | **yes** (C) | the registry's types/contexts during a predicate body run ("a mischievous predicate body can't mutate r.types or the context stack out from under the surrounding program") | the context snapshot is a pointer-slice copy already; the TypeTable clone copies type Values — in-place writes to type nodes during the body would survive the restore |

## fork.go / compile_sandbox.go / engine_pool.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| fork.go 39-40 | `fork := *r; fork.Defs = r.Defs.Clone()` — shallow Registry copy + DefEntry clone ("The fork observes the parent's bindings and context as of the fork call; its later writes stay private") | explicit-copy (snapshot for goroutine isolation) | **yes** (C, concurrency) | every bound Value now shared between two goroutines; any occurrence/flag/pos write on either side is a data race; `DefTable.Clone`'s own doc already limits the guarantee to non-pointer-backed values | bindings must be immutable objects (copy-on-write at the mutation site) or forks must deep-copy |
| fork.go 83-84 | `fork.enginePool = nil; fork.callLoops = nil` — pooled engines/loops pin the parent registry and its tokens | by-value-flow (pool) | no | — | unchanged |
| fork.go 59-60 | `fork.Contexts.Push(r.Contexts.Top())` — a private child layer over the parent's store chain (COW) | by-value-flow (StoreInstanceInfo.Data map[string]Value shared by pointer) | no | — | `map[string]*Value` |
| compile_sandbox.go 54-57 | `defs: r.Defs.Clone()`, `types: r.Types.Clone()`, `ctx: r.Contexts.Snapshot()`, `check: r.Check.Clone()` | explicit-copy (rollback snapshot) | **yes** (C) | the check pass's in-place writes on read copies (engine.go 3224, 3247, 2952, 7094, 2701, 9921) would survive `RestoreForCompile` 95-112 if they hit the shared binding objects; the file's own comment records this failure for `CheckState` | immutable bindings or COW at the mutation sites |
| compile_sandbox.go 62-84 | `modLoaded[k] = v` (ModuleDesc by value, 66), `builtins`, `caps` (83) map copies | explicit-copy (non-Value or pointer maps) | no | — | — |
| engine_pool.go 49-51 | `RunResolved`: `input := make(…); copy(input, inputs); copy(input[len(inputs):], tokens)` (then the tape copies again) | explicit-copy | **yes** (B) for `tokens` (a body's `BodyTokens` copy, invoke.go 420-425) | — | one pointer copy |
| engine_pool.go 60-62 | `runPooledAt`: `res = append([]Value(nil), res...)` — "Run's return value aliases the tape's backing array" | explicit-copy (pool aliasing) | yes (C) for the slice | the pooled tape | `[]*Value` copy |

## fn_capture.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 87 | `bodyNeedsFrameState`: `walk(SpliceExpand(info.Data))` | read-only copy (double) | no | — | drop |
| 167, 377, 393 | `walkBodyValue`/`CollectBodyLocalDefs`: `lst.Slice()`, `outer.Slice()`, `decls.Slice()` | read-only copies | no | — | drop |
| 295-323 | `ComputeCaptures`: `seen := map[string]Value{}`; `v, ok := r.Defs.Top(w.Name)` (binding copy); `seen[w.Name] = v`; `out[i] = CapturedBinding{Name: n, Value: seen[n]}` — "enclosing-fn-local bindings SNAPSHOTTED at fn-construction time" (value.go 1057-1072); the TCO soundness argument reads "captures are construction-time snapshots, reinstalled per call" (fn_frame_elide.go 14) | explicit-copy (snapshot into a by-value field `CapturedBinding.Value`) | **yes** (C) | the enclosing frame's binding; rebinding by `def` pushes a NEW entry so pointer capture keeps the snapshot semantics for REBINDING, but every in-place field write (the `Quoted=true` param install on the same object, pos stamps, `Eval=false`) becomes visible through the closure | capture the pointer; forbid field writes on bound objects (same occurrence-layer fix) |
| 415-434 | `MergeCaptures`: `seen[cb.Name] = cb.Value` dedupe copy | explicit-copy | no | — | `map[string]*Value` |

## deftable.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 10-14 | `DefEntry{Body Value; TypeDef *Type; Minted bool}` — the binding store holds the Value BY VALUE | by-value-flow | **yes** (A/C): this is the one copy that makes every read independent of the tape | every reader (`Top`, `TopEntry`, `Stack`, `Entries`, `Clone`) | `Body *Value` plus occurrence-free bindings |
| 85-96 | `Top`: `return NewTypeLiteral(e.TypeDef), true` (94, node copy) / `return e.Body, true` (96, binding copy) — THE read seam ("Canonical read for what does this name resolve to right now") | explicit-copy | **yes** (A) | the binding / the canonical node | return the pointer; every caller's subsequent write (engine.go 2957, 3160, 3224, 3247-3252, 3320, 3826, 5007, …) must move to the occurrence slot |
| 100-109 | `TopEntry`: DefEntry copy (Body by value) | explicit-copy | yes (as Top) | — | pointer |
| 113-119, 122-141 | `Push` / `PushType` / `PushTypeAdopted`: `DefEntry{Body: v}` stores the caller's Value by value (`def x y` makes two independent copies) | explicit-copy (binding install) | **yes** (A/C) | the caller's value (`def y x`: y and x become one object under the policy — fine unless either side's occurrence flags are written) | pointer |
| 236-246 | `Replace`: `ds[len(ds)-1].Body = v` (used by `is` carrier-narrowing and `upsertFnDef`) — a REPLACEMENT, not a mutation | by-value-flow | no | — | pointer replacement |
| 283-299 | `Set`: `entries[i] = DefEntry{Body: b}` | explicit-copy | no | — | — |
| 310-320 | `Entries`: `copy(out, ds)` caller-owned snapshot | explicit-copy | no (read-only) | — | — |
| 338-355 | `SnapshotEntries`: `copy(cp, ds)` per name + gens | explicit-copy (sandbox) | **yes** (C) | `RestoreEntriesSnapshot` 417-440 "restores CONTENT exactly" — in-place writes inside the region would NOT be undone | immutable entries |
| 393-411 | `sameDefEntry`: `x.ID != y.ID || x.Parent != y.Parent || x.pos != y.pos`, then `SameContainer`/`CanonValue` — field-level identity of two Value COPIES (NUR330 `ChangedSince`) | by-value-flow (identity by fields) | yes (semantics) | — | pointer equality would be simpler and stricter |
| 445-453 | `Stack`: `bodies[i] = e.Body` snapshot | explicit-copy | no | — | — |
| 523-544 | `Clone`: `copy(cp, st)` per name — "DefEntry values are copied SHALLOWLY … It does NOT make the bound VALUE independent — a FlexMap / FlexList / Store / class-instance Value holds a pointer to shared state … For CONCURRENT forks it is not [right] — `await` therefore DECLINES a reachable mutable container at the branch boundary" | explicit-copy (snapshot for fork + sandbox) | **yes** (C) | ForkConcurrent, CompileSandbox | under pointers the same decline rule would have to cover EVERY value, or bindings must be immutable |

## invoke.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 81-86 | `InvokeCallbackBody`: `cl.RetTrim = true; body = Value{Parent: body.Parent, Data: cl, Quoted: body.Quoted}` — REBUILDS the closure value from three fields (drops ID, pos, asc, elem, Carrier/Dynamic) to set a per-call mark | explicit-copy (payload copy + partial rebuild; the anti-pattern `ClosureSigMatched`'s comment warns about) | **yes** (A) | the closure value in its binding/container; `RetTrim` is a per-delivery contract (CallBoru discipline), not a property of the closure | occurrence/delivery flag; at minimum a whole-value copy |
| 99-106 | `ClosureSigMatched`: `cl.SigMatched = true; out := v; out.Data = cl` — "A whole-value copy with the payload replaced, never a rebuild from selected fields" | explicit-copy (payload copy-modify) | **yes** (A) | the closure value; `SigMatched` is a per-call seam mark ("the seam that hands the args has matched the closure's own signature") | occurrence/delivery flag |
| 303 | `MarkShapeModel`: `inst.Keys()`; `fd.Registry.ShapeModel = true` through the payload's pointer | read-only + pointer write | no | — | — |
| 396-404 | `runPooledSub`: `out := make(…); copy(out, res)` — "The result copy is mandatory — Engine.Run's result slice aliases the engine's tape" | explicit-copy (pool aliasing) | yes (C) for the slice | the pooled tape | `[]*Value` copy |
| 420-425 | `BodyTokens`: `return lst.Slice()` / `[]Value{body}` — "Mirrors what the handlers extracted via AsList(body).Slice() before splicing" | explicit-copy | **yes** (B) | the body list's elements (re-run per invocation; copied again by `RunResolved`/`NewTapeWith`) | pointer copy; cells as slots |

## fn_frame.go / fn_frame_elide.go / fn_frame_probe.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| fn_frame.go 109-118 | `NewFrameOpen` / `NewFrameOpenSpan`: fresh markers | construction | no | — | unchanged |
| fn_frame.go 212-233 | `AppendFrameTail`: fresh `NewDefCleanup` (carrying the frame's `Snapshot map[string]int`, `Names`) and `NewReturnCheck` (zero Pos = "stamp later", see engine.go 1387) | construction | no (fresh per call) — except the leaf SKELETON built once at core_helpers.go 395-437 and copied per call (`copy(out, skeleton)` 505-508) | the skeleton is shared by every call and fork | the per-call copy must stay a copy of the markers, or the ReturnCheck must be minted per call |
| fn_frame.go 235-240 | `var spentFrameMarker = NewDefCleanup(DefCleanupInfo{SkipCleanup: true})` — one shared Value copied into every spent frame's cell (engine.go 8924) | by-value-flow | **yes** (C) | every spent frame | per-frame mint, or an immutable shared object nothing writes through |
| fn_frame.go 269-314, 316-336 | `unwindLiveFrames` / `unwindFrameTail`: read-only tape scans; `_ = e.stepDefCleanup(v, -1)` (333) replays the marker's registry effects | — | no | — | — |
| fn_frame_elide.go 53-225 | `tcoEligible` / `returnsConform` / `teardownFrameState` / `elideTailFrame`: reads `AsDefCleanup(e.Tape.At(…))` (payload copy), `dcInfo.FrameOn`, `Defs.TruncationCoveredBy`; tape `Splice` of the marker run | by-value-flow (payload copies out of cells) | no | — | pointer payloads |
| fn_frame_probe.go 85-180 | `probeTailCall`: `scan.Names = dc.Names` ("The marker's own slice, not a copy"), `scan.ValuesBelow`, `isPendingResidualContainer(v)` reads `Eval`/`Quoted` of cells | by-value-flow | no | — | — |

## dispatch_explain.go / dispatch_layout.go / dispatch_probe.go / dispatch_slots.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| dispatch_explain.go 30-38, 100, 104 | `CandidateFailure{Got Value, Pattern Value}` — diagnostic snapshots of the written tuple and the rejecting pattern | by-value-flow (Value fields) | no | — | pointers |
| dispatch_layout.go 27-66, 84, 157-166, 214 | `DispatchLayout{args []Value, Beneath, After []Value, Words map[int]Value}` published on `CheckState.CurLayout`; `writtenPub{args}`; `words[i] = tok`; `beneath = append([]Value{v}, beneath...)` (O(n²) prepend copies) | by-value-flow + read-only copies | no (check mode records) | — | `[]*Value`; drop the prepend copies |
| dispatch_layout.go 69, 112 | `WrittenFor` / `LayoutFor`: `&p.args[0] != &args[0]` — a dispatch's `match.Args` slice identified by its FIRST ELEMENT'S ADDRESS | by-value-flow (slice identity through a Value array address) | **yes** (identity trick) | the `match.Args` slice built at engine.go 3550/6645 | would still work for a `[]*Value` array, but the identity should be the `MatchResult` itself |
| dispatch_layout.go 157, 187 | `tok.ID != args[i].ID`; `readsDataBinding`: `top.ID == v.ID` — a binding copy and its tape occurrence recognised as the same value by ID | by-value-flow (ID shared across copies) | yes (semantics) | — | pointer equality |
| dispatch_probe.go | none (positions are tape indices) | — | — | — | — |
| dispatch_slots.go 77, 172 | `TagCheckModeDefRead func(e *Engine, top *Value, …)` — takes a pointer to the ENGINE'S LOCAL COPY (engine.go 3247 "Tag a COPY and write it back: taking &top toward the opaque S9 slot would make every interpreter def read heap-allocate top"); the check side writes `top.SetDynFrom(name)` (check_recovery.go 82-88) | by-value-flow (pointer to a stack temp) | **yes** (A) | the binding | occurrence-level `dynFrom` |
| dispatch_slots.go 143 | `inactiveConcreteEvalOnce … return Value{}, false` | by-value-flow | no | — | nil |

## resolve.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| 42-56 | `resolveWordValue`: `return NewTypeLiteral(t)` (51, canonical node copy); `tv` from `r.TopTypeBody` (a stored body copy); fresh `NewBoolean`/`NewAtom` | explicit-copy (node copy) | **yes** (A) when the result lands in a pattern/signature that is later mutated (pos, `Quoted`); otherwise the copy is where the orphan `&v` *Type problem originates — a pointer model would REMOVE that class | the canonical node | reference the node |
| 78-87 | `resolveWordsDeep` list: `elems := lst.Slice(); resolved := make(…); return NewList(resolved)` (double copy) | explicit-copy + construction | **yes** (B) that the SOURCE pattern/list stays unresolved (resolution depends on the registry at consumption time — the "freeze discipline") | the pattern literal in a signature / typed-def body | construction over pointers; drop the extra Slice |
| 95-113 | disjunct: `alts := make(…)`; `nd := d; nd.Alternatives = SimplifyDisjunctAlts(alts); out := v; out.Data = nd` ("The Data replacement keeps the Parent (Enum subtypes) and every other field intact") | explicit-copy (payload + value copy) | **yes** (B) | the disjunct type body / pattern | construct a fresh disjunct from parts (needs a constructor that preserves Parent and the other fields) |
| 118-142 | typed list/map: `NewTypedList…`, `elems := make(…)`, `entries := make([]ChildEntry…)` | construction | no | — | unchanged |
| 144-152 | map: `result := NewOrderedMap(); result.Set(key, resolveWordsDeep(val, r)); NewMap(result)` | construction | no | — | unchanged |

## contextstack.go / store_shape_state.go / storage_helpers.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| contextstack.go 26-45 | `Push`: `child := &StoreInstanceInfo{TypeName, Prototype: parent}` — `Data` left nil; writes go through `CowSet` which builds a WHOLE NEW layer rather than mutating this map | by-value-flow (`map[string]Value` layers; every `context set` copies the value into a fresh layer's map, `Get` returns a copy, value.go 1451-1485) | no for the layer protocol (it is layer replacement, not Value mutation) | — | `map[string]*Value` |
| contextstack.go 87-95 | `TopData` returns the top map to handlers (`ctx map[string]Value`, engine.go 3988) | by-value-flow | no | — | — |
| contextstack.go 123-148 | `UpdateChain`: relinks `*StoreInstanceInfo` prototypes (pointer payloads, no Value copies) | — | no | — | — |
| contextstack.go 150-166 | `Snapshot` / `Restore`: `copy(out, cs.stack)` of `*StoreInstanceInfo` pointers (predicate sandbox, compile sandbox) | explicit-copy (pointer slice) | no (not Values) | — | — |
| store_shape_state.go 12-24 | `RecordKey`: `KeyTypes map[string]Value`; `JoinCarriers(existing, v)` mints a joined carrier (join-only, "repeat writes widen, never replace") | by-value-flow + construction | no | — | `map[string]*Value` |
| store_shape_state.go 29-33, 44-56, 60-70, 74-80 | `Value{}` sentinels for `Vals`/returns; `s.Vals = v`; `s.Vals = JoinCarriers(s.Vals, v)` | by-value-flow (Value field + zero sentinel) | no | — | nil |
| store_shape_state.go 83-99 | `CloneShape`: `cp.KeyTypes[k] = v` — "entries — and any nested shape pointers inside them — still shared … Entry-level sharing is safe because all mutation is join-only" | explicit-copy (map copy, entries shared) | no | — | — |
| storage_helpers.go | `GetKey` reads only (`fmt.Sprintf("%v", v.Data)` fallback) | — | no | — | — |

## Engine-side helpers outside the listed files (used by stepLiteral / resolveForwardArgs / execFnDefSig)

| file:line | site | kind | load-bearing? | what shares the original | replacement under the policy |
| --- | --- | --- | --- | --- | --- |
| collect_kernel.go 341-356 | `CollectForward` group arm: `res := win.At(scanIdx); win.Remove(scanIdx+1); res.Quoted = true; res.ReachGroup = true; win.Set(scanIdx, res)` — a `/v`-marked group's value quoted+tagged on the CELL copy | explicit-copy (copy-modify-writeback) | **yes** (A) | the group's value is a dot-read result (container element / module export copy) | occurrence slot |
| collect_kernel.go 383, 402 | `result.pos = tok.pos; win.Set(scanIdx, result)` on fresh in-place evaluations | construction | no | — | occurrence pos |
| collect_kernel.go 469-474 | `pe := NewParenExpr([]Value{tok}); pe.pos = tok.pos; win.Set(scanIdx, pe)` — a splice-bound word rewritten to `(w)` ("the rewrite is a TAPE MUTATION that outlives this dispatch") | construction (cell replacement) | no (pointer replacement is fine) | — | unchanged |
| collect_kernel.go 896-900 | `CollectArrival` `/q`: `atom := NewAtom(w.Name); atom.pos = val.pos; win.Set(valIdx, atom)` — Word→Atom conversion replaces the cell | construction | no (replacement) | — | unchanged |
| collect_kernel.go 926-953 | `val.Quoted = true` (marked) … `val.Quoted = false; val.ReachGroup = false; win.Set(valIdx, val)` — transient flags toggled on the cell copy of a reach-collapsed fn | explicit-copy (copy-modify-writeback) | **yes** (A) | the fn value in its container/export | occurrence slot |
| argsstack.go 56-67, 139-150 | `PushLazy(vals []Value)`: "vals must stay unchanged while the entry is on the stack: the caller hands over a copy it owns"; `Top` materialises `NewList(vals)` once; `stack []Value` holds list Values by value | by-value-flow (C) | yes for the slice ownership | the frame's args copy (core_helpers.go 484-486) | `[]*Value` the frame owns |
| core_helpers.go 395-437, 465-476, 484-508 | `buildFnBodyHandler` leaf path: memoised `skeleton` (frame open, arg placeholder cells, `s.Body()...`, tail markers) copied per call: `buf := make([]Value, n+len(args)); argsCopy := buf[n:]; copy(argsCopy, args); … out := buf[:n:n]; copy(out, skeleton); out[1+k] = args[i]` — "The per-call COPY is mandatory: execMatch's stampResultPos mutates the returned slice (ReturnCheck Pos, fn-value pos), and ForkConcurrent engines share this handler. Nothing keys on the shared tokens' Value.IDs" | explicit-copy | **yes** (B/C, concurrency) | the skeleton shared by every call and fork | cells as slots + per-call ReturnCheck mint |
| core_helpers.go 496-500, 561, 574 | `arg := args[i]; if list && !Quoted { arg.Quoted = true }; InstallFrameBinding(r, p.Name, RetagTypedContainerParam(p, arg))` | explicit-copy (copy-modify) | **yes** (A) | the caller's list value | binding-slot `Quoted` |
| core_helpers.go 536-540, 567, 595 | non-leaf path: `argsCopy` for `Args.Push`; `result = append(result, args[i])`; `result = append(result, s.Body()...)` | explicit-copy | yes (B/C) as above | — | — |
| core_helpers.go 624-635, 647-660, 666-676, 721-735 | `RetagTypedContainerParam/Value/FlexElem/Args`: `Unify(pat, arg)` builds a re-tagged copy (`unified.Quoted = arg.Quoted` 659); `out := v; out.SetElemConstraint(elemType)` (671-672) copies the flex HEADER to attach `elem` while sharing the store ("Retag the header in place — the FlexListData pointer stays shared"); `RetagTypedContainerArgs` clones the args slice copy-on-write (729-730) | explicit-copy (header copy carrying a per-binding constraint) | **yes** (A) | the caller's value: `elem` is the PARAM's contract, not the value's | `elem` on the binding slot |
| value.go 2761-2771 | `SetAtomReferent`: `snap := ref; ap.Referent = &snap; v.Data = ap; return v` — snapshots the referent and copies the atom | explicit-copy (snapshot) | yes (C): "a snapshot of what its name refers to" at load/quote time | the binding | referent as a pointer to the binding's object would track rebinding — a semantics change |
| fn_value_compile_cache.go 32-37, 46-62 | `sameList`: cache validity by backing-array identity (`&a[0] == &b[0]`) of `fnDef.Signatures` / `fnDef.Captured`; "copies of one value share their arrays and hit" | by-value-flow (identity of slices across Value copies) | yes | every copy of the fn value | with a pointer payload the cache key becomes the payload pointer |
| optimistic_match.go 103 | `vals := append(append(make([]Value, 0, n), match.Args[nFwd:]...), match.Args[:nFwd]...)` → `OuterMatch.Vals` (check mode runtime-rematch record) | read-only copy | no | — | `[]*Value` |
| fn_def.go 72-78 | `bodyElems = _lst.Slice()` — the fn body copied out of the source list literal into `FnSig`/`BoruImpl.Body` | explicit-copy | no (the source literal is consumed by `fn`'s dispatch; the copy decouples the body from the literal's backing array) | — | could share the elements |
| fn_def.go 156, 166 | `sigDeclPos`: `m.Keys()`, `l.Slice()` | read-only copies | no | — | drop |
| trace.go 81, 132 | `TraceColorize` / `TraceHandler`: `_lst.Slice()` | read-only copies; `RunTrace` runs the copy in a sub-engine | no | — | drop / `[]*Value` |
| pending_literal.go 134-146 | `pendingWalk.literal`: `elems.Get(i)`, `m.Keys()` reads | read-only | no | — | — |

---

## Summary (repeated for the hand-back)

**Counts.** ~120 explicit copy sites catalogued across the engine-side files: 31
occurrence-state copy-modify-writebacks on tape cells / arg slots (`pos`, `Quoted`,
`Eval`, `ReachGroup`, `Undefined`, `FailedDispatch`, `asc`, check-mode `Dynamic`/`DynFrom`,
`FnDefInfo.Applied`, `ClosurePayload.SigMatched`/`RetTrim`, `GuardFactInfo` wrap); 11
binding/canonical-node read copies (`Defs.Top` returns `e.Body` / `*t`, `ResolveRef`,
`NewTypeLiteral`); 19 token-run copies onto a tape (fn bodies, loop/while/mark bodies, `if`
arms, quotation elements, pending-literal elements, paren items, reach receivers, splice
payloads, program load/reload); 9 fresh-container constructions that replace a pending
literal per evaluation (already policy-compatible); 14 snapshot/clone copies for forks,
sandboxes and debuggers; 9 pool/scratch aliasing copies (pooled tapes, `Loop.Results`,
args lists); 3 derived copies of a fn payload (`compileFnDef`, `rfn := *fn`,
`upsertFnDef`); ~24 read-only convenience copies (`.Slice()`, `.Keys()`, diagnostic
tuples) that are performance-only. About 56 sites are load-bearing, ~14 unsure, the rest
incidental or perf.

**Top 5 risks of in-place mutation.** (1) Occurrence state lives in Value fields: every
binding/element/export/type-node read is a copy so that the ~30 per-occurrence writes never
reach the shared original — in place, a dot-read fn stays `ReachGroup`-tagged in its map,
`f/v apply` leaves the binding Applied, a list argument becomes `Quoted` in the caller's
binding (closure-capture walks and the residual sweep then skip it), every read of `x`
reports the def site's position. (2) Program tokens are re-run and the tape rewrites its
cells (`forceStackWord`, `/q` Word→Atom, splice-word→`ParenExpr`, `Eval=false` after
evaluation, `Quoted` on params): pointer replacement of a cell is fine, a field write
through it freezes `[1 add 2]` to `[3]` or forces a word to `/s` for the body's next call.
(3) Concurrency: `ForkConcurrent` isolates forks by shallow-cloning the DefTable; the leaf
frame skeleton is shared by every call and fork and is copied per call precisely because
`stampResultPos` mutates it; the compiled-dispatch cache is shared by every copy of a fn
value. (4) Sandboxes/rollbacks (`CompileSandbox`, `DefTable.Clone`/`SnapshotEntries`/
`RestoreEntriesSnapshot`, the predicate sandbox, `withRecoveryRaw`) assume the check pass
can scribble on copies (`top.Quoted=false`, `DynFrom`, `Dynamic`, `FailedDispatch`,
`GuardFactInfo`) and the interpreter later runs on pristine bindings. (5) Type literals are
node copies (`NewTypeLiteral(t)` = `*t`): with one object per node, every mention would
stamp the canonical lattice node with read positions and occurrence flags (while the
orphan `&v` non-canonical pointer problem would disappear).

**Performance-only copies.** The 104-byte tape cell traffic (`Tape.At/Set/Insert/Splice/
MoveGap/grow`), the double copies (`SpliceExpand` over `Slice()`, `stepMove`'s body copy
before `Splice`, the dead `MarkInfo.Body` copy in `stepMoveCont`'s `NewMark`, the
`loopTokens`/`peScratch`/`rrValues` staging buffers, `RunResolved`+`NewTapeWith`,
`CallBoru`'s body append + tape copy, `fn_def.go`'s body copy), the read-only `.Slice()`
walkers, the by-value boxing of `FnDefInfo` (a ~300-byte copy per `.(FnDefInfo)`
assertion) and `ForwardInfo` (re-minted per collected arg), and the double ID mints on
fresh carriers.

**Shape of the fix.** Every load-bearing site is one of: (A) a per-occurrence attribute
stored in the Value → needs a cell/slot record beside a `*Value`; (B) a re-executed token
run whose executing copy is consumed or rewritten → tape cells must be slots (pointer
replacement allowed, field writes not) or the re-run structures immutable; (C) a snapshot
semantics (fork, sandbox, capture, args list, `recoveryRaw`, `MarkInfo.Body`,
`spentFrameMarker`) relying on copy independence → immutable shared objects or explicit
copy-on-write at the mutation site. Construction-from-parts already covers the evaluated-
container family and the frame/forward markers.
