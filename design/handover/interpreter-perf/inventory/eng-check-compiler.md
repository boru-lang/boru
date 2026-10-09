# No-copy policy inventory: eng/go, check/go, compiler/go

Read-only review of the tree at `/home/user/boru` (2026-10-09). Line numbers are from the current files. The policy under consideration: "Value structures should never need to be copied. They can be modified in situ as needed by any stage, but copying is forbidden."

## Scope note

`eng/go` today holds only the bytecode VM (`vm.go`, `vm_*.go`, `compiled_runtime_vm.go`, `region_host.go`, `region_oracle.go`). The aliases, `util.go` (`WithPos`, `ReparentValue`, `CanonicalType`), `typed_bind.go` and the parser bridge all live in `core/go` now; the generated facade was retired (`eng/go/piece_map.tsv` header). `ReparentValue` has **zero direct callers** in eng/check/compiler; the VM reaches it only through `core.RunTypedBind` / `RunTypedBindCons` (`eng/go/vm.go:5355,5358`). Core-side helpers that these modules lean on and that are themselves copies: `core.NewTypeLiteral(t)` is `return *t` (a by-value copy of the canonical lattice node, `core/go/value.go:2803`), `core.StripAscribed` / `WithPos` / `WithPosAt` / `WithFreshFnIdentity` / `MarkApplied` / `NewDynamicCarrierValue` / `CarrierOfLiteral` mutate a by-value parameter and return it, `DefTable.Top` returns the binding's `Body` by value (or a `NewTypeLiteral` copy for a type binding, `core/go/deftable.go:84-97`), `ReadList.Slice()` copies the element slice (`core/go/value.go:51`), and `NewTapeWith`/`Tape.Reload` copy their input slice (`core/go/tape.go:160-166`).

## Summary

### Counts of explicit sites (non-test files)

| module | whole-Value copy-to-modify (`x := y` then field write, by-value param mutated and returned, `*p` deref copy, pointer write into a copy) | payload-struct copy-to-modify (FnDefInfo / Signature / ClosurePayload copied, edited, stored back) | helper-mediated copies (StripAscribed, WithPos/WithPosAt, SetPos on a copy, CloneValue*, NewTypeLiteral, ValueCarrier-family re-mints) | `[]Value` slice copies (append-nil/empty, copy(), .Slice(), make+element copy) | by-value struct fields / maps holding Values |
|---|---|---|---|---|---|
| eng/go | 19 (vm.go 2050, 2417-2430, 2549, 2902-2903, 3416, 4795-4800, 4832-4837, 4962-4968, 5540-5550, 5603-5614, 6004, 6805-6813, 1378, 1396/1205, 893-903; vm_dyn_words.go 107-113, 117-165, 246; vm_dyn_apply.go 214-224; vm_fnvalue_seam.go 182-190) | 7 (vm.go 568-606, 893-903, 2029-2030, 2792-2800, 965-966, 5011-5020; vm_fnvalue_park.go 149-163) | StripAscribed 19, WithPos/WithPosAt 8, SetPos 5, CloneValueKeeping+WithFreshFnIdentity 3, NewTypeLiteral 3 | append-nil 42, copy() 8 (6 on Values), .Slice() 4, make+gather 53 | vmFrame.locals, vmContext.restartLocals, refusedRun.results, flowOrigin.results, dynEnter.locals, flowEscape.residual, regionHost.span (7 holders; all per-run) |
| check/go | 14 (carrier.go 307-452 toCarrier, 690-697, 1916-1918, 2020-2022, 3497-3500; check_fnbody.go 646, 867-868, 1073-1074; check_fnmodel.go 142-143, 167, 197; call_site_spec.go 122-123; method_shape.go 151-155/180-184/539-547/725-739 (4 twins); store_shape.go 232-235) | 0 | CloneValue 1 (a no-op for fn payloads, see below), WithPos 4, NewTypeLiteral 6, ValueCarrier/CarrierOfLiteral/NewDynamicCarrierValue/GenBindingCarrier/UnionCarrierForType 13; NewCarrier(T)/NewDynamicCarrier(T) 75 (constructions, not copies) | append-nil/empty 15, copy() 1, .Slice() 6, make+gather 30 | fnUnitCompile.body/returnPatterns; maps: FnSummaries map[string][]Value, loop joins map[string]Value (carrier.go 2536-2588), genBindings |
| compiler/go | 9 (emit.go 5425-5427, 5454-5456, 10241-10246, 10272-10277, 20966-20971; kept_defs.go 103-118, 390-395; body_map.go 163; compiler_dispatch_record.go 356) | 2 (stamp_runtime.go 264-330; vm-side view twins none) + 3 table copy-on-writes that are not Values (lower.go 5648, poly_raw.go 131, vm_dyn_apply.go 85/131) | WithPos 4, SetPos 1, NewTypeLiteral 2, ValueCarrier 3; NewCarrier 20, NewValueRaw 2 (constructions) | append-nil 14, copy() 5 (0 on Values), .Slice() 12, make+gather 17 | **50 struct-field lines**: Program.Consts/Body, CompiledFn.Body/SpecFallback/ParamPatterns/ReturnPatterns, SpecGuard.Fn, RestartSrc.Val, CallWindowOperand.Value, SlotDesc.Token, ClosureRetSpec.Source/Patterns, LandingWord/StmtIsland/DynMethodSpec/DeoptSpec islands, LoopCont.Body/After, NativeSplit.Body/Beneath/After, PolySplit.Beneath/After/Words, PolyRef.Raw, CompiledFnRef.Captures, SigRef.SpliceOuts, EmitState.consts/rootBody/origByID/specFnFirst/memberFnReads/fnMemberFields/rootParenStacks/rootStmtStacks/spliceOuts/foldedBodies, fnUnitRec.specFns/specFallback/paramVals/outOpsVals/body, pendingApply.fn, emitCall.leadAt, emitDynBind.val, pendingTypeRun.body, valBind.lit, substPlan.val, loopCont.elems/after, lowerer.landingBody, RegionState.StackResidual/ReachBound, userPolyPlan.outs, pendingWindow.win/prefix, callWinOp.value |

Field-mutation lines on Values (the `.Carrier = `, `.Dynamic = `, `.Quoted = `, `.Data = `, `.ID = `, `.SetPos(`, `.SetDynFrom(` writes): eng 24, check 66, compiler 18. In check/go roughly 45 of the 66 are writes to a carrier that was constructed two lines earlier (`c := core.NewCarrier(t); c.Dynamic = true`) — construction, not copying — and are not listed as copy sites below.

### The five hardest risks of in-place mutation in these modules

1. **The VM constant pool (`compiler.Program.Consts`).** `OpPushConst` pushes `p.Consts[in.Arg]` by value (`vm.go:4760`), `OpLookupDynScope` pushes `curReg.Defs.Top(name)` by value (`vm.go:5866-5905`), `OpPushLocal` pushes `locals[i]` by value (`vm.go:4775,4784`). Everything downstream then mutates the pushed copy: `StripAscribed` at every delivery boundary (args, locals, list/map elements, binds, RET), `Quoted = true` on list params at frame entry (`vm.go:4967, 5549, 5612`, `vm_dyn_apply.go:223`, `vm_fnvalue_seam.go:186`), `Quoted = false` on applied fn values (`vm.go:2549, 2902, 3102, 4412`, `vm_dyn_words.go:246`), `SetPos` stamps (`vm.go:1378, 1396, 5017, 6357, 6806`), fn/closure RENAME under the binding name (`vm.go:4800` → `vm_dyn_words.go:117-165`; `nameFrameFns vm.go:568-606`), `MarkApplied`/`appliedLead` (`vm.go:2903, 3416`), typed-bind reparent (`vm.go:5355/5358` → `core.ReparentValue`). In place, each would write into the pool entry, so the next execution of the same op (a loop iteration, a second call of the unit, a second `def` of the same `/v` read, a fork sharing the Program) would see a quoted/renamed/reparented/position-stamped constant. `constPoolKey` dedups by canon so one pool entry serves every source literal with the same rendering (`emit.go:16021-16065`); `internUnpooled` (`emit.go:15974`) exists exactly because a reparented bind "must never merge with a source literal of the same canon". The fresh-push family (`OpPushConstFresh`, `OpPushConstFreshLocal`, `RestartSrc.Fresh`) already deep-clones per evaluation (`vm.go:4634, 4769, 2300`) to give the interpreter's one-instance-per-evaluation semantics; compile-time `StampCompiledRef` (`emit.go:4851-4863`) is the one sanctioned in-place write into a pooled const, and it is restricted to pre-publication (`stamp_runtime.go:258-262` explains the race otherwise).

2. **Check-mode def reads are tagged on a COPY and mutated through a pointer.** Core copies the binding (`core/go/engine.go:2952 tagged := v` and `:3248 tagged := top`, "Tag a COPY and write it back") and hands `&tagged` to `check.tagCheckModeDefRead` (`check_recovery.go:82-96`: `top.SetDynFrom(name)`), `EmitState.NoteLiveRead` / `seatLiveRead` (`emit.go:7463-7560`: `v.ID = GenerateID(...)`), `NoteValReadLive` / `keptReadSeatedLive` (`kept_defs.go:103-118`: `*v = WithPos(NewCarrier(v.Parent), *v)` — the read becomes a carrier) and `computedLeakGradual` (`kept_defs.go:390-395`: `v.Dynamic = true`). With `DefTable.Top` returning the binding itself, every read would re-ID the binding (the ID is the key of `producedBy`, `defReads`, `origByID`, `liveReadIDs`, the bind ledger), tag it `dynFrom`, and on a seated live read REPLACE the concrete binding with a carrier for the rest of the program.

3. **Carrier derivation and identity re-minting in the checker.** `toCarrier` (`carrier.go:307-452`) strips `Data` on a copy of the token and keeps the token's ID so `origByID`/`Materialise` (`emit.go:9657-9664`, `5391-5460`) can recover the concrete literal — the original MUST survive; the narrowing pass copies the binding to clear two facets before intersecting (`carrier.go:1916-1918`, `2020-2022`) and re-stamps the fresh intersection with the old ID (`:1950`) while pushing it as a SEPARATE binding level that is popped later; `freshResidual` (`check_fnbody.go:864-871`), `nur068ReturnCarrier` (`check_fnmodel.go:142-143`), `call_site_spec.go:122-123`, `recordFnValueApplyFallback` (`check_fnbody.go:1073-1074`), `tryFoldParkedMemberFn` (`compiler_dispatch_record.go:356`) and the de-collision loops (`emit.go:10431-10441, 11528`; `compiler_dispatch_record.go:1559`) all exist because ONE payload must carry TWO recorder identities at once (a memoised residual reused per call, a member read at several sites, a folded read). In place, those become one identity, and `producedBy` resolution collapses distinct productions onto one event.

4. **Type literals are copies of canonical lattice nodes.** `NewTypeLiteral(t) = *t`: `OpPushType` (`vm.go:5053`), `seatPrefix`/`seatDeoptPrefix` (`vm.go:2315, 4457`), `DefTable.Top` for a type binding, `resolveTypeNameArgs` (`carrier.go:3499-3500`, then `WithPos` onto it), disjunct alternatives (`carrier.go:983, 1025, 2342`), `emit.go:11434`, `eager_literal.go:102`. Any in-place `SetPos`/`Quoted`/`SetAscribed`/`ID` write on a literal that IS the canonical node would corrupt the node every registry, `behave`, dispatch and the type table share. `eng/go/CLAUDE.md` "Canonical *Type Pointers" documents the inverse hazard (orphan copies whose `Behavior` does not follow the canonical node); the policy flips it into an aliasing hazard, and `CarrierOfLiteral` (`core/go/carrier_new.go:47-50`, `lt := lit; NewCarrier(&lt)`) manufactures exactly such an orphan today.

5. **Program tables handed to the interpreter.** `LandingWord.Island`, `StmtIsland.Island`, `DynMethodSpec.Island`, `DeoptSpec.Island`, `LoopCont.Body/After`, `Program.Body`, `CompiledFn.Body`, `FallbackSpan` tokens, `RegionDesc.Slots[i].Token`, `PolySplit.Beneath/After`, `NativeSplit.*`, `RestartSrc.Val`, `CallWindowOperand.Value` are shared by every run and every `ForkConcurrent` of a Program; the lowerer makes most islands SUB-SLICES of one backing array (`lower.go:4106-4111`, `lowerer.landingBody = es.rootBody`, `lw.landingBody[r.token:]`). The VM copies before each island run (`vm.go:2147, 2276, 4216, 4277, 4313, 4337, 4377, 6133`; `vm_dyn_words.go:209-210`; `vm_do_restep.go:33`; `region_host.go:99-103`; `vm_poly_nomatch.go:190`) and copies results back out of the pooled sub-engine (`vm_defer.go:76`, `vm_list_restep.go:63`: "the island's results are its pooled engine's buffer, which the next island run reuses — a loop's lists aliased the last one"). The tape copies its input today (`core/go/tape.go:160-166`), so the slice copies are belt-and-braces; under a pointer model the interpreter's token writes (`stepWord`'s pos threading, `Tape.Set` of a tagged read, quote flips) would land in the tables.

### Copies that exist purely for performance (and the defensive copies they force)

- `argScratch` reuse in `OpCallNative` (`vm.go:5120-5128`, `vm_args_release.go`) avoids a per-call allocation; it forces `splitArgs = append(nil, args...)` (`vm.go:5150`) and `nativeSplitRaise`'s `window := append(nil, args...)` (`vm_poly_nomatch.go:287`).
- The pooled island sub-engine (`r.TakeSubEngine`, `Tape.Reload`) forces `res = append(nil, res...)` (`vm_defer.go:76`) and `elems = append(nil, results...)` (`vm_list_restep.go:63`).
- `regionHost.span` is a reusable buffer (`region_host.go:85-88`).
- Core's `tagged := top` copies (engine.go:2952, 3248) exist to keep `&top` from escaping to the heap on every def read; the mutation through the pointer is in check/compiler.
- `ReadList.Slice()` copies made only for a read-only walk (API shape, not need): `carrier.go:2150, 3383`; `check_fnmodel.go:529`; `check_recovery.go:408`; `store_shape.go:138`; `body_map.go:117-118` (two copies, one wrapped in `NewList` for a predicate); `emit.go:10537` (`len(lst.Slice()) == 0`), `emit.go:20071`; `vm_token_body.go:112` (`len(lst.Slice()) == 0`), `:115`; `vm_dyn_body_one.go:104`; `stamp_runtime.go:532, 543`; `kept_defs_scan.go:65`; `landing_restart.go:1320`.
- Per-dispatch signature views (`installedSigView vm.go:2792`, `fnDispatchView vm_fnvalue_park.go:149`, `closureSigView`) copy a fn value's Signature slice to resolve `BarrierAllForward` and sort — a per-dispatch cost that normalising at construction would remove.

### Two findings beyond the inventory

- `check_fnbody.go:646 out[i] = core.CloneValue(bv)` is documented as "Cloned for a fresh ID", but `CloneValue` returns a Function value UNCHANGED (its `FnDefInfo` payload hits the immutable `default` arm of `core/go/clone.go:155-158`), so the return carrier shares the residual's ID. The comment is stale; nothing mints a fresh ID there.
- Pointer-held Values already exist and work because nothing mutates them after construction: `FnParam.Pattern *Value`, `FnSig.ReturnPatterns []*Value`, `CompiledFn.ParamPatterns/ReturnPatterns`, `ClosureParamSpec.Patterns`, `ClosureRetSpec.Source *Value`, `core.BranchRecord.ThenValue/ElsValue *Value`, `Value.elem *Value`, `EmitState.foldedBodies map[*core.Value]bool`. Backing-array / address identity is relied on at `emit.go:13890 (&b[0] == &body[0])`, `callable_words.go:720 (&out[0] == &inputs[0])`, `compiler_dispatch_record.go:679 (&inner.Signatures[i] == sig)`; `check_fnbody.go:329-333` deliberately keeps BOTH a copied body (`bodyCopy`, for the unit compile) and the uncopied slice (`bodyRef`, whose backing array identifies the construction to `PendingClosureApply`). No `==` on whole Values exists in the three modules (equality goes through `core.ValuesEqual`/`ExactEqual`/IDs). `core.Value{}` is returned with `ok=false` in ~95 places (check 45, compiler 43, eng 7) — a return convention convertible to nil — and is a SEMANTIC sentinel at `vm.go:4633` (`locals[slot].Parent == nil` = "not yet seated"), `vm.go:4781` (`IsUnboundSlot`, branch-carried zero slot), `vm.go:5812` (`ApplyResidentBind` undef arm), `method_shape.go:974, 998` and `taken_landing.go:41` (`NoteLandingNext(..., core.Value{})`). `ListPayload{Elems []Value}` is a struct boxed in the `Payload` interface, so a by-value copy of a list Value already ALIASES its element array — "by value" is one level deep today, and `CloneValue` is what gives independence.

### What construction would replace the load-bearing copies

- Pool pushes: compile compound and fn literals as construction code (`OpMakeList`/`OpMakeMap`/`OpPushClosure` over scalar consts) — the interpreter already constructs per evaluation — instead of cloning a pooled instance. Scalar consts can only be shared by pointer if no facet or flag is ever written on them.
- Delivery strips and quote flips: make the dispatch ascription (`as T`) and quote-ness properties of the OPERAND SLOT (a stack-cell flag, the call-site token), not of the Value.
- Names and anchors: move the binding name (`FnDefInfo.Name`, `ClosurePayload.RetName/RetPos/Render`) and the position stamps out of the value into the binding / the op's debug table, so `nameClosureValue`, `nameFrameFns`, `dropFnPos`, `stampFnPos`, `stampFnArgPos` disappear.
- Recorder identities: key productions by `(event seq, result index)` in the recorder instead of a Value-resident `ID`, which removes every `x := y; x.ID = GenerateID(...)` re-mint; or keep `ID` but mint result carriers by construction (`NewValueRaw(parent, sharedPayload)` plus facets) rather than by copying the residual.
- Type literals: push a type REFERENCE value (node pointer + token position) so `NewTypeLiteral`'s `*t` copy is never made and the lattice node is never written through a literal.
- Program tables: hold immutable token DESCRIPTORS and construct the island's Values per run (what `newRegionHost`/`regionOracleWindow` already do for region slots), and keep the interpreter from writing into tokens it did not construct.
- Def reads: `DefTable.Top` returning the binding itself is only safe if every reader that today mutates its copy (seatLiveRead, SetDynFrom, carrier replacement, rename, quote flips) constructs a fresh read value from the binding's parts.

Per-file tables follow. Columns: `line | site | kind | load-bearing? | what shares the original | replacement under the policy`.

## eng/go/vm.go

| line | site (function, one-line what) | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 152 | `vmFrame.locals []core.Value` — a CALL_USER frame keeps the caller's locals slice while the callee runs on a fresh `nl` | by-value-flow | yes | the caller's locals are restored at RET (`locals = f.locals`, 6028); the callee's `nl` is a copy of the args (4962, 5540, 5603) so the callee's `Quoted`/rename writes never reach the caller | frames hold the caller's slice unchanged; only the arg copies into `nl` are at issue (see those rows) |
| 215 | `vmContext.restartLocals` — alias of the running frame's locals while a dynamic op runs | by-value-flow | no | the slice header only | none needed |
| 440, 544 | `RunProgram`/`RunUnit`: `make([]core.Value, NumLocals)` locals, `make(…, 0, MaxStack)` stack; zero `Value{}` slots are the "unseated" sentinel (`Parent == nil`, 4633) and the branch-carried "unbound" sentinel (`IsUnboundSlot`, 4781) | by-value-flow (zero sentinel) | yes | the sentinel tests read fields of the zero Value | nil pointer as the sentinel |
| 568-606 | `nameFrameFns`: `locals[i] = nameClosureValue(locals[i], name)`; `fd.Name = name; locals[i].Data = fd` — a fn value bound for a named param is renamed in the frame slot | explicit-copy (payload struct copied, renamed, stored into the slot copy) | yes | the caller's stack cell, the const pool entry or the def binding the arg came from; the interpreter renames the BINDING's copy (`core/go/core_helpers.go:108 fnDef.Name = name` on a local copy) and a value bound under two names renders under each | keep the display name on the binding/frame slot, not in the payload |
| 743-747 | `applyNativeFnValueTopDown` caller: `rev[len-1-i] = v` reversal of args | slice copy (reorder) | no | nothing is mutated | none |
| 773-776 | top-down arg gather `args[i] = inputs[len-1-i]` | slice copy | no | — | none |
| 784, 838, 973 | result/park assembly `append(append(nil, inputs...), res/body)` | slice copy | no | fresh result slice | none |
| 893-903 | `closureSourceStep`: `src := *cl.Source`; `bound := append(nil, fd.Captured...)`; `bound[i].Value = cl.Captures[i]`; `src.Data = fd` — the callback's source fn value is re-created with THIS closure's captures | explicit-copy (deref + payload copy) | yes | `ClosureRetSpec.Source` is one pointer in the Program's `ClosureRet` table shared by every closure pushed at that pc; each instance has its own captures | construct a fresh fn Value from parts (FnDefInfo with its own Captured slice); the per-instance captures make a shared Source impossible |
| 965-966 | `body = core.Value{Parent: body.Parent, Data: cl, Quoted: body.Quoted}` after `cl.Render = …` — re-mint of a closure with its render cached | explicit-copy (re-mint from parts) | unsure | `body` may be a pooled const; `Render` is a pure cache, but writing it in place into a shared Program const would race forks | compute Render at OpPushClosure (construction) |
| 1153 | `frameArgsList`: `NewList(append(nil, args...))` — the per-call `args` list pushed on `r.Args` | slice copy | yes | the frame locals `nl` are rebound by `OpStoreLocal` during the body; the `args` list must keep the call's values (NUR350) | the list must be built from the args at call time — this is construction |
| 1200-1205, 1479-1482 | poly windows `window[i] = stack[top-i]` then `stampFnArgPos(window, pr.FnArgPos)` | slice copy + SetPos on copies | yes (positions only) | the stack cells / locals / consts keep their own position; a binding "keeps no position of the value it binds" (vm_dyn_words.go:117-125, NUR347) | anchor the `/v` read position on the call site, not the value |
| 1303 | `callPoly`: `args[i] = core.StripAscribed(args[i])` on the gathered window | helper copy | yes | the same value sits in a local/const/binding (`def y (m as T)` then two dispatches of `y`); the ascription is per-value today and must survive the first delivery | ascription as an operand-slot property |
| 1378 | `stampFnPos`: `v.SetPos(debug[pc]); return v` on the by-value param | explicit-copy | yes | `v` may be a pooled const handed back by an identity word; the const and the binding (`dropFnPos`) must not carry the stamp | resolve anchors from the op's debug table at raise time |
| 1396 | `stampFnArgPos`: `args[i].SetPos(at[i])` on the window copies | explicit-copy | yes | as 1200 | as 1200 |
| 1579 | `invokeClosurePositional(vc.r, fnVal, append(nil, args...))` | slice copy | no | the callee copies into its own locals anyway (vm_dyn_apply.go:214) | none |
| 1646-1650, 3210-3214 | `iargs[i] = args[n-1-i]` reversed gather for a modifier chain | slice copy | no | — | none |
| 1704-1710, 2607-2612, 2660-2662, 3244-3246, 4006-4008 | island assembly `island := make(...)` + fnVal/args/`fb.Tokens` | slice copy | no today (the tape copies its input); yes for `fb.Tokens` under a pointer model (`Program.Fallbacks` table) | `Program.Fallbacks[i].Tokens` is shared by every run | island tokens as immutable descriptors constructed per run |
| 2029-2030, 2792-2800 | `landingWalk`: `own := fnDef; own.Signatures = fnDef.OwnSigs(); own = installedSigView(own)` — `installedSigView` copies the Signature slice to resolve `BarrierAllForward` ("the const is shared program state") | explicit-copy (payload) | yes | the fn value's payload in the pool/binding; the sentinel barrier must stay for rendering/introspection, the resolved one is per dispatch | resolve the barrier at construction of the fn value (what `compileFnDef` does for defs) |
| 2050-2052 | `landingWalk` no-match: `parked := v; parked.Quoted = true; stack[top] = parked` | explicit-copy | yes | `v` is the landing op's input (a `/v` read const or a local); parking is a stack-cell state | a quote bit on the stack cell |
| 2121, 2447, 3337, 3390, 3464 | `WithPosAt(NewWord/NewAtom(...), pos)` fresh tokens for islands | construction | no | — | none |
| 2147-2152, 2276 | `landingDeopt`/statement islands: `prefix := append(nil, stack[frameBase:top]...)`, `append(nil, island...)` from `lword.Island` / `is.Island` | slice copy | no today; yes for the tables under a pointer model | `Program.LandingWords`/`StmtIsland` islands are sub-slices of ONE backing array shared with `Program.Body` and every other island (lower.go:4106-4111) | per-run construction from descriptors |
| 2294-2320 | `seatPrefix`: `RestartConst && Fresh` → `WithFreshFnIdentity(CloneValueKeeping(src.Val, nil))`; `RestartConst` → `src.Val` by value; `RestartType` → `NewTypeLiteral(ForwardedType(t))` | explicit-copy (deep clone / node copy) | yes | `RestartSrc.Val` is a Program table entry; a fresh literal must be one instance per evaluation (NUR336 seated literals); the type literal is a copy of the canonical node | construct the literal per run; a type-reference value instead of a node copy |
| 2374 | loop continuation `Results: append(nil, stack[lp.base:lp.iterBase]...)` into `core.Loop` | slice copy | yes | the VM truncates/reuses the stack after; the continuation outlives the frame | the continuation must own its results — construction |
| 2376-2380 | `tokens` assembled from `lc.Body`/`lc.After` (`LoopCont` table) + island | slice copy | as 2147 | `Program.LoopConts` table | as 2147 |
| 2405-2432 | `substToken`: `out := append(nil, toks...)`; `t := out[i]; t.Data = ParenExprPayload{Toks: sub}; out[i] = t` (2417-2421) and `t.Data = ListPayload{Elems: sub}` (2430) — a nested token's payload replaced by the substituted token list | explicit-copy | yes | `toks` are the island/program tokens (shared tables); the substitution is per run | construct a fresh paren/list token from parts with the original's pos |
| 2549, 3102 | `fnVal.Quoted = false` on the local copy of `stack[top]` before an apply | explicit-copy | yes | the local/stack/const keeps the construction-time quote ("the LOCAL push carries the STORED value verbatim — including the construction-time quote", 2540-2548) | quote-ness on the delivery slot |
| 2558-2561, 2826-2829, 3017-3020, 3327-3330, 3533-3536 | arg gathers top-down from the stack | slice copy | no | — | none |
| 2704-2709 | `parkedWindow` rotation into a fresh slice | slice copy | no | — | in-place rotation |
| 2902-2903 | `applyHandler` mirror: `fnVal.Quoted = false; fnVal = core.MarkApplied(fnVal)` (`fd.Applied = true` on a payload copy) | explicit-copy | yes | the stack/binding/const value must stay un-applied; `apply` is per call | "applied" as a dispatch flag |
| 2939-2942 | `applyReStep` inputs reversal | slice copy | no | — | none |
| 3416 | `appliedLead`: `fd.Applied = true; v.Data = fd; return v` | explicit-copy | yes | as 2902 | as 2902 |
| 3501 | `callDynamicMixed`: `window := append(nil, stack[base:]...)` | slice copy | yes (decline path) | the stack region is handed back untouched when the island declines / the window is stepless | construct the island input; keep the stack as the source of truth |
| 3649-3650 | `callDynFrame`: `prefix`/`tokens` copies of the stack | slice copy | yes (decline path, as 3501) | the stack region is re-used by the fallback paths (3715-3720) | as 3501 |
| 3720 | no-match tuple `uncalledAt(..., append(append(nil, prefix...), tokens[1:]...))` | slice copy | no | diagnostic only | none |
| 4031 | `refusedRun.results` | by-value-flow | no | per-run holder | none |
| 4109 | `bindDynScope`: `v := StripAscribed(stack[top])` then `InstallDef`/`InstallFrameBinding` | helper copy | yes | as 1303 (the stack cell is consumed, but the value's other holders keep the ascription) | ascription on the operand slot |
| 4216, 4277, 4313, 4337, 4377 | deopt/statement islands: `append(nil, body[spec.Token:]...)`, `append(nil, tokens...)`, `append(nil, stack[base:]...)` | slice copy | as 2147 | `CompiledFn.Body`/`Program.Body`/`DeoptSpec.Island` tables; the stack region is replaced by the results | as 2147 |
| 4413-4420 | `var written *core.Value; written = &top.Body` where `top` is the `DefEntry` COPY from `TopEntry`; the entry is popped and `*written` re-pushed by the restore closure | by-value-flow (pointer into a copy) | unsure | the def table entry is popped before the pointer is used, so aliasing the entry itself would likely still work | none required; verify the restore order |
| 4440-4457 | `seatDeoptPrefix`: `RestartConst` → `src.Val` by value; `RestartType` → `NewTypeLiteral(ForwardedType(t))` | by-value-flow / node copy | yes | `DeoptSpec.Seat` table; the canonical type node | as 2294 |
| 4477-4485 | `deoptPrefix` gather from locals/stack | slice copy | no | — | none |
| 4511 | `bindGlobal`: `curReg.Defs.Push(gb.Name, StripAscribed(v))` with PEEK semantics (the stack is not popped on the fast path) | helper copy | yes | the same runtime value is simultaneously the def binding and the next word's stack operand; later quote/rename/pos writes on either must not reach the other | none without changing def semantics (binding and operand would alias) |
| 4633-4634 | `seatConstLocal`: `locals[slot] = WithFreshFnIdentity(CloneValueKeeping(p.Consts[idx], p.ConstKeep[idx]))` on first read per call; `Parent == nil` zero sentinel | explicit-copy (deep clone) + zero sentinel | yes | `Program.Consts` is shared across calls and forks; one instance per call, shared within the call (`def tree {…}` read twice) | compile the literal as construction code run once per call into the slot |
| 4760 | `OpPushConst`: `stack = append(stack, p.Consts[in.Arg])` | by-value-flow | yes | the pool entry serves every push site with the same canon (dedup) and every run/fork of the Program; the pushed copy is mutated downstream (strips, quotes, renames, pos, reparent) | scalar consts shared only if never written; compounds/fns constructed |
| 4769 | `OpPushConstFresh`: `WithFreshFnIdentity(CloneValueKeeping(p.Consts[i], p.ConstKeep[i]))` | explicit-copy (deep clone) | yes | the pool; per-evaluation identity (`(mk) eq (mk)` false, NUR288 fn identity) with the enclosing bindings' containers KEPT shared (`ConstKeep`) | construction code for the literal's spine; embedded binding reads stay reads |
| 4775, 4784 | `OpPushLocal`/`OpPushLocalBound`: `stack = append(stack, locals[i])` | by-value-flow | yes | a local read twice yields two independent stack values; the second read must see the STORED value verbatim (2540-2548) while the first copy was unquoted/renamed/stripped | the downstream writes must move off the Value (quote/asc/name/pos as slot properties) |
| 4795-4802 | `OpStoreLocal`: `stored := StripAscribed(stack[top]); stored = vc.nameStoredClosure(stored, name); locals[slot] = stored` | explicit-copy (strip + rename) | yes | the popped stack value may also be a pooled const (a `/v` read baked as PUSH_CONST) or a value bound under another name ("The copy in this slot is renamed; the pooled const keeps its own payload") | name on the binding; ascription on the slot |
| 4832-4838 | `OpMakeList`: `elems := make; copy(elems, stack[top-n:]); elems[i] = StripAscribed(elems[i])` | explicit-copy (element copies + strip) | yes (strip) / no (copy) | the stack cells are popped; the strip must not reach other holders of the same value | construct the list from the popped cells; ascription elsewhere |
| 4962-4968, 5540-5550, 5603-5614 | CALL_USER(_POLY) frame entry: `nl := make; copy(nl, sigArgs)`; `nl[i] = StripAscribed(nl[i])`; `nl[i].Quoted = true` for list params | explicit-copy | yes | the caller's stack/local/const list (a const list pushed by PUSH_CONST would be quoted in the pool for every later push; a caller's local list would become quoted after the call) | a "quoted" bit on the frame slot, or lower list-param reads to a quoted push |
| 4997-4998 | `OpPushClosure`: `caps := make; copy(caps, stack[top-nc:])` into `ClosurePayload.Captures` | slice copy (into a constructed payload) | yes | captures are the closure's own state; the stack cells are popped | this is construction of the closure |
| 5011-5020 | `v := core.Value{Parent: TFunction, Data: cl}`; `cl.RetTypes… = spec…; v.Data = cl; v.SetPos(spec.Pos); v = nameClosureValue(v, spec.DefName)` | construction (fresh Value edited before publication) | no | — | none |
| 5053 | `OpPushType`: `stack = append(stack, NewTypeLiteral(ForwardedType(t)))` — `*t` copy of the canonical node | explicit-copy (node copy) | yes | the canonical lattice node shared by every registry, `behave`, dispatch and `Types.byID` | a type-reference value (node pointer + pos) |
| 5120-5135 | `OpCallNative`: `argScratch` reuse (release build), `args[i] = StripAscribed(stack[top-i])` | helper copy into a scratch buffer | yes (strip, as 1303); the scratch reuse is performance | the stack cells; natives must not retain `args` (vm_args_release.go contract) | ascription on the slot |
| 5150 | `splitArgs = append(nil, args...)` — "the handler's body runs may reuse the scratch buffer" (SigRef.Split, NUR263) | slice copy (forced by the scratch reuse) | yes | `argScratch` | per-call args allocation (what the borudebug build does) |
| 5355, 5358 | `OpBindTyped`: `RunTypedBind(Cons)(r, spec, StripAscribed(stack[top]))` → `core.ReparentValue` copies with `Parent` rebound (`core/go/typed_bind.go:46,77`) | helper copy (reparent) | yes | the stack value's other holders and the pool keep the BASE tag (`typeof 42` → Integer vs `typeof x` → Pos; `constPoolKey` keys on the type ID so a reparented survivor does not donate its Parent) | re-mint the bound value from the payload (`NewValueRaw(def, v.Data)`) — a construction sharing the immutable payload |
| 5812 | `ApplyResidentBind(curReg, rb.Name, true, core.Value{})` undef arm | by-value-flow (zero sentinel) | no | — | nil |
| 5835 | `ApplyResidentBind(curReg, rb.Name, false, StripAscribed(stack[top]))` with PEEK (`rb.Pop` false) | helper copy | yes | as 4511 (binding and stack operand alias) | as 4511 |
| 5866-5905 | `OpLookupDynScope(Ref)`: `v, ok := curReg.Defs.Top(name)` (binding copy; a `NewTypeLiteral` copy for a type binding) / `core.ResolveRef`; `stack = append(stack, v)` | by-value-flow | yes | the def binding; the pushed value is later quoted/unquoted, renamed under a second def, pos-stamped, stripped | construct the read value from the binding's parts |
| 6004 | `OpRet`: `stack[i] = StripAscribed(stack[i])` for `i >= stackBase` | helper copy (in place on cells) | yes | the callee's residual cells may be its locals/consts pushed by value; in place on the originals it would strip them | ascription on the slot |
| 6023-6028 | RET: `f := frames[len-1]` (vmFrame struct copy), `locals = f.locals` | by-value-flow (frame struct, not a Value) | no | — | none |
| 6133 | loop-result re-step `runIslandResolved(reg, nil, append(nil, res...))` where `res = stack[base:]` | slice copy | yes | the island's results replace the stack region the input aliases | construct the island input |
| 6250 | `flowOrigin.results` holds a native's result slice | by-value-flow | unsure | ownership of a handler's result slice | none required |
| 6341 | `hostedResidual`: `res := append(nil, stack[:base]...)` + a stand-in | slice copy | no | fresh result | none |
| 6355-6358 | `flowStandIn`: `v := NewNone(); v.SetPos(at)` | construction | no | — | none |
| 6596 | `checkParamContract`: `pat := *pp` (deref copy of `CompiledFn.ParamPatterns[i]`) passed by value to `OpenUnifyMap`/`Unify` | by-value-flow (deref copy) | unsure | the pattern is a Program table entry; `Unify` may return the pattern side, which a later reparent must not write through (the "Typed-Def Reparent" rule in eng/go/CLAUDE.md) | pass the pointer; keep `Unify`'s result from being mutated |
| 6805-6813 | `makeMap`: `vals := stack[len-n:]` (ALIAS); `stampFnArgPos(vals, spec.FnPos)` writes pos into the stack cells; `om.Set(k, StripAscribed(vals[i]))` | explicit-copy (strip into the map entry) + in-place pos stamp on cells | yes (strip) / no (stamp: the cells are popped) | the map entries must not alias the popped cells' other holders | construct entries; ascription on the slot |
## eng/go/vm_dyn_words.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 74-76 | `nameStoredClosure` → `nameClosureValue` | explicit-copy (delegates) | yes | see 117-165 | see 117-165 |
| 107-113 | `dropFnPos`: `v.SetPos(core.SrcPos{})` on the by-value param, returned | explicit-copy | yes | the value on the stack / in the pool keeps its construction-token position; "A binding keeps no position of the value it binds" | anchors kept per op/binding, not on the value |
| 117-165 | `nameClosureValue`: `fd.Name = name; v.Data = fd` / `cl.RetName = name; cl.RetPos = {}; cl.Render = …; v.Data = cl` | explicit-copy (payload) | yes | "The copy in this slot is renamed; the pooled const keeps its own payload, as the interpreter's binding copies do" — `def f (mk 1)  def g f/v` must render as `fn f` and `fn g`; a closure bound for a param renders under the param's name while the stored value keeps its own | binding-side display name; render computed from (unit, name) at format time |
| 209-210 | `callDynFrameWords`: `prefix := append(nil, stack[frameBase:base]...)`, `tokens := append(nil, region...)` | slice copy | yes (decline path) | the stack region is returned untouched when no entry converts (`return nil, false`) or the region is not live | construct the island input |
| 238, 253 | `tokens[i] = core.WithPosAt(core.NewWord(w.Name), w.Pos)` replacing a fn value in the island copy by its word | construction | no | the region keeps the fn value | none |
| 246 | `fnv.Quoted = false; InstallFrameBinding(reg, w.Name, fnv)` on the `closureAsWord` result (the stack value's copy when it is not a closure) | explicit-copy | yes | the token region keeps the quoted value ("A QUOTED fn is data to a value re-step, but a word read dispatches the BINDING") | quote-ness on the slot |
| 281 | no-match tuple `args := append(nil, region[1:]...)` | slice copy | no | diagnostic | none |
| 338-350 | `closureAsWord`: returns `v` unchanged or a bridged FnDefInfo view (`closureFnDef`) | construction | no | — | none |
| 421-422 | bridged sig `Impl = core.Go(func(a) { return invoke(append(nil, a...)) })` | slice copy | yes | the interpreter's handler `args` slice is a scratch the callee must not retain | per-call args allocation on the interpreter side |
| 480 | `callWindowAt`: `win := make(...)` assembly | slice copy | no | — | none |

## eng/go/vm_dyn_apply.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 37 | `dynEnter.locals []core.Value` — the fresh locals of the entered unit | by-value-flow | no | per-entry holder | none |
| 85-87 | `headNamedContract`: `named := *fn; named.Name = head.Name` | table copy-on-write (`CompiledFn`, not a Value) | yes | `Program.Fns` table | per-frame contract override (what `vmFrame.retFn` already is) |
| 131-135 | `applyRetContract`: `ov := *unit; ov.Name/Returns/ReturnPatterns/Decl = …` | table copy-on-write (`CompiledFn`) | yes | `Program.Fns` table | as 85 |
| 214-224 | `dynApplyEnter`: `locals := make; copy(locals, args); locals[i] = StripAscribed(locals[i]); locals[i].Quoted = true` | explicit-copy | yes | the caller's args (stack/locals/consts) | quote/ascription on the slot |

## eng/go/vm_fnvalue_seam.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 61-64, 147-150 | `args[len-1-i] = v` reversal of the inputs | slice copy | no | — | none |
| 182-190 | `deliverArgs`: `a = StripAscribed(a); a.Quoted = true; out[i] = a` | explicit-copy | yes | the native's `inputs` (the callback seam's stack order values) | as frames |
| 231 | `pushRootArgs`: `reg.Args.Push(NewList(append(nil, args...)))` | slice copy | yes | the per-call `args` list must not alias the unit's locals, which the body rebinds | construction of the args list |

## eng/go/vm_fnvalue_park.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 102-103 | window assembly `resolved ++ fnVal ++ forward` for the region host | slice copy | no | — | none |
| 149-163 | `fnDispatchView`: Signature slice copied, `Params` copied, `BarrierAllForward` resolved, `NormalizeSig`, `SortSignatures` | explicit-copy (payload) | yes | the fn value's published signatures (pool/binding/container element) — their order and sentinel barrier are observable through rendering and introspection | normalise and sort at construction of the fn value; cache the view on the payload |

## eng/go/vm_list_restep.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 23, 26 | `elems := append(nil, stack[base:]...)`; `tokens := append(nil, elems...)` — the list's storage and the island's token list diverge at 40 (`tokens[i+1] = WithPosAt(NewWord(…))`) | slice copy | yes | the list keeps the folded value where the island steps the word (NUR219) | construct both from the stack region |
| 63 | `elems = append(nil, results...)` — "the island's results are its pooled engine's buffer, which the next island run reuses — a loop's lists aliased the last one" | slice copy (forced by the pooled sub-engine) | yes | the pooled sub-engine's result buffer | per-run result allocation on the island side |
| 67 | `elems[i] = StripAscribed(elems[i])` | helper copy | yes | as OpMakeList | ascription on the slot |

## eng/go/vm_markwindow.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 47-49 | `vmMakeListToMark`: `elems[i] = StripAscribed(v)` into a fresh element slice | explicit-copy (strip) | yes (strip) | as OpMakeList | as OpMakeList |
| 74-76 | `vmSeatBelowMark`: `prefix`/`region` copies, then `append(stack[:m], prefix...)` + region | slice copy | yes (slice aliasing: the destination overlaps the source) | the stack's own backing array | in-place rotation; no Value semantics involved |

## eng/go/vm_mixed_plan.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 75-76 | result assembly from window halves | slice copy | no | — | none |
| 85-89 | `legacyReMatchAgrees`: `laid := append(nil, below...)` + reversed args | slice copy | no | probe only | none |
| 93-114 | `mixedPlanArgs`: `fnDispatchView(fd)` (see vm_fnvalue_park.go:149), `args[i] = window[pos]` | payload copy + gather | yes (view) / no (gather) | as fnDispatchView | as fnDispatchView |

## eng/go/vm_poly_nomatch.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 57 | `core.AttemptedTuple(fn, append(nil, written[:i]...), stackTuple)` — AttemptedTuple appends into its first arg | slice copy (capacity aliasing) | no | diagnostic tuple | none |
| 89-93 | `tupleAt` gather | slice copy | no | — | none |
| 190 | `splitBeneath`: `out := append(nil, sp.Beneath...)`; live slots substituted into the copy | slice copy | yes | `PolySplit.Beneath` is a Program table shared by every run; the substitution is per run | construct the beneath slice per run from the descriptor |
| 254-262 | `splitReportTape`: tokens gathered from a Tape, words substituted, `NewTape` | construction | no | report only | none |
| 287-288 | `nativeSplitRaise`: `window := append(nil, args...); window[sp.BodyAt] = sp.Body` | slice copy | yes | `args` may be the reused `argScratch`; `sp.Body` is a table entry | per-call args allocation |

## eng/go/vm_rematch.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 35-38 | window gather top-down | slice copy | no | — | none |
| 115-118 | `args[i] = h.win.At(at)` from the host window | slice copy | no | — | none |
| 140-147 | `planSplitOver`: `toks` assembled from `beneath`/`stackTop`/`written`/`after` (`DispatchSpec` tables + stack) into a region host | slice copy | no today; yes for the tables under a pointer model | `Program.Dispatches` tables | construct per run |
| 177 | `rematchStoppedTuple`: `AttemptedTuple(fn, append(nil, written[:i]...), prefix)` | slice copy | no | diagnostic | none |

## eng/go/vm_spec_guard.go, vm_splice_outs.go, vm_foreign_unit.go, vm_fnvalue_zeroarg.go, vm_token_body.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| vm_spec_guard.go 41-45 | `specFallbackInputs` reversal | slice copy | no | — | none |
| vm_splice_outs.go 22 | `spliceOutsSeat`: `kept := make(...)` filter of `got` | slice copy | no | — | none |
| vm_foreign_unit.go 207-210 | `shapeInputs` reversal for `ClosureInStackPair` | slice copy | no | — | none |
| vm_fnvalue_zeroarg.go 83 | `append(append(nil, inputs...), res...)` result assembly | slice copy | no | — | none |
| vm_fnvalue_zeroarg.go 106-110 | `kept := append(nil, args...)`; `out := stack[:base:base]` then appends | slice copy (capacity aliasing: `args` is a sub-slice of `stack`) | yes (slice-level only) | the stack's backing array | none at the Value level |
| vm_token_body.go 72 | `flowEscape.residual []core.Value` | by-value-flow | no | per-run holder | none |
| vm_token_body.go 112, 115 | `len(lst.Slice()) == 0` (a copy made for a length test) and `tokens := lst.Slice()` for the unit key | slice copy (API shape) | no | the stored body list's elements | `lst.Len()` / read-only view |

## eng/go/vm_defer.go, vm_do_restep.go, vm_dyn_body_one.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| vm_defer.go 67-69 | `runIslandResolved`: `input := make; copy(input, inputs); copy(input[len:], tokens)` then `e.Run(input)` (which copies into the tape again, `core/go/tape.go:160-166`/Reload) | slice copy (double) | no | the tape copies anyway | hand the two slices to the tape loader |
| vm_defer.go 76 | `res = append(nil, res...)` — the pooled sub-engine's result buffer is reused by the next island | slice copy (forced by pooling) | yes | `r.TakeSubEngine()`'s buffer | per-run result allocation |
| vm_do_restep.go 33 | `runIslandResolved(reg, nil, append(nil, results...))` | slice copy | no today | the native's results; the tape copies | none |
| vm_dyn_body_one.go 101-106 | `splicePayload`: `l.Slice()` — a list's elements spliced onto the stack | slice copy | yes | the list's element array (a container element spliced onto the stack and then mutated by delivery must not alter the list) | construct the splice from a read-only view; the delivery writes must move off the Value |

## eng/go/vm_generic.go, region_host.go, region_oracle.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| vm_generic.go 123-125 | `toks` = resolved frame values ++ `NewWord(d.Word)` ++ slot tokens, for the region host | slice copy | as region_host.go 99 | `RegionDesc.Slots[i].Token` table | as region_host.go 99 |
| vm_generic.go 180-215 | `args[i] = tok` / `tok = v` with `v, ok := reg.Defs.Top(wi.Name)` — a forward word resolved to its binding (by value) | by-value-flow | yes | the def binding | construct the read value |
| vm_generic.go 221 | `args[i] = StripAscribed(args[i])` | helper copy | yes | as vm.go:1303 | ascription on the slot |
| vm_generic.go 311-318 | `samePattern`: `up = fn.ParamPatterns[i]` (pointer), `core.ExactEqual(*up, sp)` read-only | by-value-flow (deref read) | no | — | none |
| region_host.go 85-88 | `regionHost.span` reusable buffer ("valid only until the caller splices") | by-value-flow (performance) | no | per-host buffer | none |
| region_host.go 99-103 | `newRegionHost`: `toks = append(toks, d.Slots[i].Token)` — table tokens copied into the host window, which the collection then evaluates/splices | slice copy | yes | `Program.Regions[i].Slots` is shared by every run; the window is mutated per run | per-run construction of window tokens from slot descriptors (this is that; the Token stored in the table is the copy source) |
| region_oracle.go 52-58 | `regionOracleWindow`: `toks[i] = stack[top-i]` or the slot token | slice copy | as 99 | as 99 | as 99 |

## check/go/carrier.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 54-61 | `NewCarrierTypedListLen`: fresh typed-list carrier, `ct.Len = &ln; v.Data = ct` on the fresh value | construction | no | — | none |
| 75-82 | `ReturnsPreserveListAt`: fresh carrier, `out.SetElemConstraint(ec)` copies the ARG's elem pointer | construction (facet pointer shared) | no | the `elem *Value` facet is shared by pointer already | none |
| 110-116, 147-153 | `NewElementCarrier` / `ElementCarrierOf`: fresh carriers from a type / `data.Data.(ChildTypeInfo)` | construction | no | — | none |
| 159-200 | `joinedElementCarrier`: reads `p.Elems` (alias) and map values; joins fresh carriers | construction | no | read-only | none |
| 205-225 | `ParamInputCarrier`: `UnionCarrierForType(t)` fresh disjunct; `di.Declared = true; dv.Data = di` on it | construction | no | — | none |
| 293-297 | `elementJoinCarrier`: `ValueCarrier(el)` / `NewCarrier(el.Parent)` — carrier minted FROM an existing value (reads `Parent`/`Dynamic` only) | construction | no | — | none |
| 307-452 | `toCarrier(v)`: ~12 early `return v` arms, else `v.Carrier = true; v.Data = nil; return v` — the token stripped to a carrier ON A COPY, keeping ID/pos/Quoted/Eval | explicit-copy | yes | the concrete token (program slice / list element / ReturnsFn result) must survive: `RememberOriginal` keeps the original by the SAME ID (`emit.go:9657-9664`) and `Materialise` (`emit.go:5391`) recovers it for the lowerer; the stripped token's ID doubles as the key | a carrier constructor taking (id, parent, pos, flags) so the carrier is built, not copied; the ID-sharing contract with `origByID` stays |
| 479-490 | `StripToCarriers(in)`: `out := make(…)`; `out[i] = v` or `toCarrier(v)` | slice copy | yes | `in` is the program's token slice (engine.Run input); `SetRootBody` snapshots it (`emit.go:8480`) and the VM re-runs `body[spec.Token:]` from the CONCRETE tokens | construct the check-mode token list; keep the program tokens immutable |
| 588 | `CarrierResults`: `folded.ID = out[0].ID` — a fresh folded const adopts the recorded result identity | identity adoption on a fresh value | no (as a copy) | — | shows ID is a production key, not an instance key |
| 603 | `partOut[i].ID = out[i].ID` — partition-joined carriers re-stamped with the recorded IDs | identity adoption on fresh values | no | — | as 588 |
| 690-697 | `args` projection: `top` is `r.Args.Top()`'s by-value result; `top.ID = GenerateID(…)` then `RecordArgsProjection(r, lst.Slice(), top, pos)` | explicit-copy (ID minted on the copy) | yes | the `r.Args` entry keeps its (empty) ID; the projection event needs an identity of its own | mint the identity on the record |
| 787-791, 1473-1477 | ReturnsFn results → `toCarrier(v)` per element into a fresh slice | explicit-copy (of fresh ReturnsFn values) | no | the raw results are discarded | in-place strip of the fresh results would be fine |
| 811-812, 824-834, 850-856 | fresh `NewCarrier(t)` with `Dynamic` set | construction | no | — | none |
| 958-962 | `out[i].Carrier = true; out[i].Dynamic = true` on the result carriers (fresh slice from the ReturnsFn / declared returns) | in-place on fresh values | no | — | none |
| 981-989, 1018-1027, 2340-2344 | `alts[i] = NewTypeLiteral(t)` (canonical-node copies) inside `NewDisjunct(alts)`; `NewDynamicCarrierValue(...)` sets flags on the fresh disjunct | explicit-copy (node copies stored in a payload) | yes (node) | the canonical lattice nodes; `CarrierOfLiteral` later takes `&lt` of a copy of these alternatives | type-reference values as disjunct alternatives |
| 1524-1531 | `alternativeCarriers`: `CarrierOfLiteral(lit)` per flattened alternative (core: `lt := lit; NewCarrier(&lt)` — the carrier's Parent is the address of a LOCAL COPY, an orphan `*Type`) | by-value-flow (orphan pointer by construction) | unsure | identity is by ID only; the orphan reads canonical `tmeta` through the shared pointer but a `behave` write to `Behavior` would not propagate | a carrier whose Parent is the canonical node (what a pointer model would give) |
| 1551-1552 | `disjunctCombos`: `row := make; copy(row, c); row[i] = alt` | slice copy | no | fresh rows | none |
| 1666 | `HasUnknownRefinement(NewTypeLiteral(t))` — read-only node copy for a predicate | explicit-copy (node) | no | read-only | pass the node |
| 1894-1952 | narrowing-through-use: `cur, ok := r.Defs.Top(name)` (binding copy); `bound := cur; bound.Dynamic = false; bound.SetDynFrom("")`; `narrowed := TandValues(bound, NewCarrier(slot))` (fresh); `narrowed.ID = cur.ID`; `pushed := NewDynamicCarrierValue(narrowed)`; `r.Defs.Push(name, pushed)` (popped later) | explicit-copy (two facets cleared on a copy) + identity adoption | yes | the existing binding level stays as it is (the push is an analysis artifact popped at pass end, 1953-1960); `producedBy` must still resolve the name to its original producer, hence the ID adoption | `TandValues` taking the facets as parameters; the narrowed carrier constructed (as now) |
| 2020-2024 | reachability probe: `bound := args[j]; bound.Dynamic = false; bound.SetDynFrom(""); IsNeverShape(TandValues(bound, NewCarrier(st)))` | explicit-copy | yes | `args` are the dispatch operands, used after the probe with their facets intact | a probe API ignoring the facets |
| 2150 | `valueTreeHasCarriers`: `for _, e := range l.Slice()` read-only walk | slice copy (API shape) | no | — | `l.Get(i)` |
| 2186-2297 | `ReturnsFreshInstance`: fresh carriers (`NewCarrier(…)`, `c.Data = rt; c.Dynamic = true`); 2297 `out[i] = args[m]` passes the dynamic target through | construction / by-value pass-through | no | — | none |
| 2310-2313 | `ReturnsStatic`: minted per call ("Identity is how the pipeline tracks values", 2330-2336) | construction | no | — | none |
| 2378-2395, 2417-2420 | numeric/concat result carriers | construction | no | — | none |
| 2889-2895 | `declaredReturnBail` | construction | no | — | none |
| 2969 | `AnalyseFnBody`: `r.Args.Push(NewList(append(nil, args...)))` | slice copy | yes | the caller's `args` slice is still the dispatch's operand list; the analysis's `args` projection must not alias it | construction of the args list |
| 3190-3202 | `AnalyseFnBody`: `args = append(nil, args...)` on first Undefined atom; `args[i] = WithPos(NewDynamicCarrier(TAny), args[i])` | slice copy (copy-on-first-change) + construction | yes | the caller's args keep the undefined atoms (`NoteDefRead` keyed on them; the dispatch record uses them) | construct the sanitized list (what this is) |
| 3268-3270 | `NewVariadicCarrier(NewTypeLiteral(TNever))` / `NewCarrier(TAny)` | construction (node copy inside) | no | — | type-reference value |
| 3383 | `for _, e := range lst.Slice()` read-only walk | slice copy (API shape) | no | — | `Get(i)` |
| 3449-3456 | `stripZeroOutResiduals`: `filtered := make(…)` | slice copy | no | — | none |
| 3461-3470 | `carrierStacksEqual`: per-element `core.ValuesEqual` (no `==` on Values) | by-value-flow note | no | — | none |
| 3484-3507 | `resolveTypeNameArgs`: `out = append([]core.Value{}, args...)` on first change; `lit := NewTypeLiteral(t); lit = WithPos(lit, a); out[i] = lit` | slice copy + node copy + pos copy | yes | the caller's `args` keep the Word tokens; the canonical node must not receive the token's pos | type-reference value {node, pos}; construct the resolved arg list |

## check/go/check_fnbody.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 47, 237, 310-316, 788, 812 | `patterns []*core.Value`, `paramPatterns[i] = p.Pattern`, `declaredReturnPatterns := append([]*core.Value(nil), s.ReturnPatterns...)` | by-value-flow (pointer-held, read-only) | no | `FnParam.Pattern` / `FnSig.ReturnPatterns` are already pointer-shared with `CompiledFn`, `ClosureParamSpec`, `fnUnitRec` | precedent for pointer-held, never-mutated Values |
| 329-333 | `bodyCopy := append(nil, s.Body()...)` for the unit compile, `bodyRef := s.Body()` kept uncopied because its backing array identifies THIS construction to `PendingClosureApply` (`emit.go:13879-13895`, `&b[0] == &body[0]`) and `foldedBodies` (`emit.go:1371`) | slice copy + address identity | unsure | the sig's body slice (shared by every copy of the fn value — "every copy of such a fn … shares the body's backing array") | under a pointer model `b[0] == body[0]` gives the same identity; the copy for the compile looks defensive (AnalyseFnBody runs on a tape copy) |
| 474-528 | `genArgs[i]` = `recordSchemaCarrier` / `typedContainerCarrier` / `narrowToDeclaredParam` / `ParamInputCarrier` / `core.ValueCarrier(a)` / `NewCarrier(a.Parent)` | construction (from the arg's parts) | no | — | none |
| 569-690 | declared-return carriers: `GenBindingCarrier` (fresh), `NewCarrier(TAny)+Dynamic` (615-617), `UnionCarrierForType` (660), `refinedDeclaredReturn` (676), `NewCarrier(t)` (676-686) | construction | no | — | none |
| 646 | `out[i] = core.CloneValue(bv)` — a concrete closure in the body residual surfaced as the declared Function return, "Cloned for a fresh ID" | helper copy (a NO-OP for a Function payload: `clone.go` default arm returns `v` with the same ID) | no | the residual value's ID is shared with the return (the comment's claim is false) | if a fresh identity is wanted, mint one by construction; otherwise delete the call |
| 755 | `NewCarrier(TAny)` | construction | no | — | none |
| 864-871 | `freshResidual`: `c := v; c.ID = GenerateID(IDPrefixForType(c.Parent)); out[i] = c` per residual value ("a shallow copy — a payload is the same value on every call, only the identity is the call's own") | explicit-copy (ID re-mint) | yes | the memoised body residual (`FnSummaries`) is shared across every call shape; each call's results need their own `producedBy` identity | per-production identity in the recorder; or construct result carriers from (parent, shared payload) |
| 896-897, 926-927 | `NewCarrier(TAny)+Dynamic` collapse carriers | construction | no | — | none |
| 1014-1020 | `outs = append(nil, outs...); outs[0] = fresh` | slice copy (copy-on-write) | no | the caller's `out` slice | none |
| 1070-1074 | `recordFnValueApplyFallback`: `fresh = outs[0]; fresh.ID = GenerateID(TFunction)` — a fn-valued result re-identified so a later `apply` resolves to THIS apply's event | explicit-copy (ID re-mint) | yes | `outs[0]` keeps the dispatch's identity | per-production identity |
| 1076-1081, 1152-1153 | `fresh = NewCarrier(parent); fresh.Dynamic = outs[0].Dynamic` | construction | no | — | none |
| 1083-1086 | `window[len-1-i] = a` reversal | slice copy | no | — | none |
| 1187-1190 | `genArgs[j] = ParamBodyCarrier(p)` | construction | no | — | none |
| 1330-1338, 1363, 1390 | `NewCarrier(pt)` / `NewCarrier(bv.Parent)` with `Dynamic = a.Dynamic` | construction | no | — | none |
| 782 | `fnUnitCompile.body []core.Value` (= `bodyCopy`) | by-value-flow | no | the unit's own token slice | none |

## check/go/check_fnmodel.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 50-67, 88-95 | `recordSchemaCarrier` / `recordReturnCarrier`: `core.Value{ID: GenerateID(…), Parent: TMap, Carrier: true, Dynamic: true, Data: RecordTypeInfo{Fields: pm.M}}` | construction (shares the pattern's `*OrderedMap`) | no | the schema map is immutable by convention | none |
| 101-105 | `returnPatternAt` → `*core.Value` | pointer read | no | — | none |
| 142-143 | `nur068ReturnCarrier`: `sv := bv; sv.ID = GenerateID(TMap)` — a residual record carrier surfaced as the return under its own identity | explicit-copy (ID re-mint) | yes | the residual keeps its identity; "the schema payload itself is immutable and shared" | per-production identity |
| 167, 197 | `pat := *p.Pattern` deref copies used read-only (`AsChildType`, `IsTypedMap`) | by-value-flow (deref copy) | no | — | pass the pointer |
| 174-182, 201-209 | `NewTypedMap(ci.Child)` / `NewCarrierTypedListValue(ci.Child)` + fresh ID + flags ("a zero ID collapses two params onto one slot": the compiler keys frame slots by `Value.ID`) | construction (the pattern's child Value copied into a new `ChildTypeInfo`) | no | — | none |
| 225-235 | `NewDisjunct(SimplifyDisjunctAlts(di.Alternatives))`; `dv.Carrier = true; ndi.Declared = true; dv.Data = ndi` | construction | no | — | none |
| 245-307 | `narrowArgsToParams`: `out = append(nil, args...)` on first change; `out[i] = rc / nc / NewCarrier(pt)` | slice copy (copy-on-first-change) + construction | yes | the caller's `args` are recorded as the dispatch operands after the body analysis | construct the narrowed list (what this is) |
| 529 | `walk(lst.Slice())` read-only walk | slice copy (API shape) | no | — | `Get(i)` |
| 557-560 | `probe := alt; if IsBareTypeNode(alt) { probe = CarrierOfLiteral(alt) }` | by-value pass-through + carrier construction | no | — | none |
| 590-593 | `NewCarrier(p).Is(exp)`, `TandValues(NewCarrier(p), NewCarrier(exp))` throwaway probes | construction | no | — | none |

## check/go/check_recovery.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 44-50 | `drainUndefinedAtoms`: `und := e.Tape.At(i)` (cell copy, read); `c := NewCarrier(TAny)` replaces the atom on the tape | construction | no | — | none |
| 82-96 | `tagCheckModeDefRead(e, top *core.Value, …)`: `top.SetDynFrom(name)`; `Recorder().NoteLiveRead(top, name, pos)` (→ `seatLiveRead`: `v.ID = GenerateID(…)`; `keptReadSeatedLive`: `*v = WithPos(NewCarrier(v.Parent), *v)`; `computedLeakGradual`: `v.Dynamic = true`) — all through a pointer to core's COPY of the binding (`core/go/engine.go:3248 tagged := top`) | explicit-copy (pointer write into a copy) | yes | the def table's binding: its ID keys `producedBy`/`defReads`/`origByID`/the bind ledger; `dynFrom` tags the READ; a seated read must not turn the binding into a carrier for later reads and for the final program | construct the read's value from the binding (fresh ID, facets) in every case |
| 447 | const fold: `RunContainerSub(r, append(nil, items...), false)` | slice copy | yes | `items` are the literal's program tokens; the sub-run may `Tape.Set` cells (the tape copies, but the ownership is the program's) | construct the fold input |
| 479, 523-525 | `NewCarrier(TAny)` / per-declared-return carriers | construction | no | — | none |
| 718-721 | `out := NewCarrier(TAny); out.Dynamic = true; out.ID = …; out = WithPos(out, e.Tape.At(anchor))` | construction + pos copy from the tape cell | no | — | none |
| 948-950, 1738-1741 | `args[i] = e.Tape.At(p)` windows for recovery | slice copy (cells copied) | no | read-only; the recovery then `Splice`s cells (replacement, not mutation) | none |
| 1719-1723 | `cs := r.Check` (`*CheckState` pointer copy) | not a Value | no | — | none |

## check/go/method_shape.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 151-155, 180-184, 539-547, 725-739 | `args[i] = e.Tape.At(p); args[i].Eval = false; args[i].Undefined = false` — tape cells copied into a dispatch window with two flags cleared | explicit-copy | yes | the tape cells keep `Eval` (a parser list that auto-evaluates) and `Undefined`; every `return false` path leaves the tape for the next model to read | a window representation with a per-slot "as data" mark instead of clearing flags on the Value |
| 157-159, 422, 803-808, 863-867 | fresh dynamic carriers spliced onto the tape / returned | construction | no | — | none |

## check/go/store_shape.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 84-108, 124-141 | `MintFlexShapeCarrier` / `MintFlexListShapeCarrier`: `NewStoreShapeCarrier(…)`; `ss.RecordKey(k, AdoptShapeValue(fv, depth+1))` / `ss.RecordVal(…)`; `l.Slice()` read-only walk (138) | construction (+ one API-shape slice copy) | no | — | `Get(i)` |
| 205-219 | `AdoptShapeValue`: returns `v` or a fresh carrier | by-value pass-through | no | — | none |
| 228-241 | `ShapeFieldRead`: `core.NewDynamicCarrierValue(v)` — core sets `Carrier`/`Dynamic` on the param COPY of a shape-recorded value | explicit-copy (in core, on this module's behalf) | yes | the value recorded in `StoreShapeInfo` (joined and re-read by every later read/write of the shape) | construct the read carrier from (parent, payload) |

## check/go/spec_fn_unit.go, call_site_spec.go, drypass.go, check_ordering.go, dispatch_hooks.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| spec_fn_unit.go 28, 32 | `s := &fnDef.Signatures[sigIdx]`, `core.FnHome(caller, &fnDef)` on the by-value `fnDef` param | by-value-flow (pointer into a param copy) | no | read-only | none |
| spec_fn_unit.go 38-49 | `paramPatterns[i] = p.Pattern` (pointers), `inputs[i] = ParamInputCarrier(t)`, `declaredReturnPatterns := append([]*core.Value(nil), …)` | construction / pointer copies | no | — | none |
| spec_fn_unit.go 56 | `body := append(nil, s.Body()...)` for the specialised unit compile | slice copy | unsure | as check_fnbody.go:329 | as check_fnbody.go:329 |
| call_site_spec.go 107, 120 | `specArgs = append(nil, genArgs...)` copy-on-first-change | slice copy | no | the generalised args stay for the plain unit key | none |
| call_site_spec.go 122-124 | `c := a; c.ID = GenerateID(IDPrefixForType(c.Parent)); specArgs[i] = c` — the fn-valued arg under a distinct identity for the specialised unit | explicit-copy (ID re-mint) | yes | `a` is the call's operand with the dispatch's identity; the spec unit's key/record must not collide | per-production identity |
| drypass.go 185-195 | `dryPassOperands`: `out = append(nil, args...)` on first change; `out[i] = WithPos(NewNone(), a)` | slice copy + construction | yes | the caller's args keep their carriers (the dry pass is a probe) | construct the probe operands (what this is) |
| drypass.go 43, 51 | handler signatures `named map[string]core.Value` | by-value-flow (map) | no | — | none |
| check_ordering.go 36 | `NewCarrier(result)` | construction | no | — | none |
| dispatch_hooks.go 73 | `return core.Value{}, false` inactive default | by-value-flow (zero sentinel) | no | — | nil |
## compiler/go/emit.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 543, 931, 2189, 2222-2229, 2372, 2463, 2596, 6583, 19004 | `emitCall.leadAt`, `emitDynBind.val`, `pendingApply.fn`, `fnUnitRec.specFns/specFallback/paramVals/outOpsVals/body`, `deoptPoint.island`, `pendingTypeRun.body`, `valBind.lit` — recorder snapshots of values seen during the check pass | by-value-flow (snapshot) | yes | the pass keeps mutating its working values after the snapshot (narrowing pushes, ID re-mints, carrier stripping); the record must keep the then-state, and `specFallback`/`specFns` become `CompiledFn.SpecFallback`/`SpecGuard.Fn` program tables | snapshots must be constructed values (frozen descriptors) if the live value may change |
| 1371 | `foldedBodies map[*core.Value]bool` keyed by `&body[0]` (token address) | by-value-flow (address identity) | yes | every copy of a folded member fn "shares the body's backing array" — the fence recognises it by address whatever ID it carries | pointer identity of the first token works unchanged under a pointer model |
| 1491, 8475-8481 | `rootBody []core.Value`; `SetRootBody`: `es.rootBody = append(nil, body...)` — the program's concrete tokens before the check pass runs | slice copy (snapshot) | yes | the tape copy the pass mutates; `rootBody` becomes `Program.Body`, `lowerer.landingBody` and every island sub-slice (`lower.go:857, 4106-4111`) | program tokens held immutable; islands as descriptors |
| 1586, 12499, 12550 | `fnMemberFields map[string]map[string]core.Value` — fn-valued fields of a constructed instance, keyed by the instance ID | by-value-flow (snapshot) | unsure | the instance's field values (an `OrderedMap` the pass may later mutate in check mode) | — |
| 1594, 13270, 13962, 13995 | `memberFnReads map[string]core.Value` — a fn value read out of a container, later consulted by ID | by-value-flow (snapshot) | unsure | the container element | — |
| 1665-1670, 11945-11972 | `rootParenStacks` / `rootStmtStacks map[core.SrcPos][]core.Value`: `append(nil, stack...)` snapshots of the check stack at a position ("A position's first note is its token's own") | slice copy (snapshot) | yes | the stack values are mutated by the pass afterwards (narrowing, re-mints, strips); the restart machinery needs the then-state | frozen descriptors |
| 1676, splice_outs.go 39 | `spliceOuts map[int][]core.Value`: `append(nil, outs...)` | slice copy (snapshot) | yes | `outs` are re-minted/narrowed later (10431-10441) | frozen descriptors |
| 1877, 16021-16065 | `consts []core.Value`; `intern(v)`: `es.consts = append(es.consts, v)` by value at interning time; `constPoolKey` = `Parent.ID + canon` dedups scalars so one entry serves every push site; compounds pool by ID; fn values never pool | by-value-flow (the pool) | yes | (a) later check-pass mutation of the interned token (toCarrier strip, tags) must not reach the pool; (b) every dedup'd push site shares one entry; (c) `internUnpooled` (15974) exists because "the bind may carry a reparented tag the literal must not inherit" | pool entries as immutable descriptors; compounds/fns compiled as construction code |
| 1893, 9657-9664, 2670 | `origByID map[string]core.Value`; `RememberOriginal`: `es.origByID[v.ID] = v` — the concrete literal snapshotted BEFORE `toCarrier` strips it (same ID) | by-value-flow (snapshot) | yes | the stripped carrier (same ID) replaces the concrete token in the check run; `Materialise` (5391-5460) rebuilds consts from this snapshot | if stripping were in place, this snapshot is the one copy that cannot be avoided — or the carrier must be constructed instead of stripped |
| 1978, 7379-7386 | `specFnFirst map[string]core.Value`: first fn value seen per name, compared by own-sig shapes | by-value-flow (snapshot) | no | shape comparison only | none |
| 2236, 2240, 14522, 8430, 8500 | `fnUnitRec.paramPatterns/returnPatterns []*core.Value`, `fnOpContract.patterns`, `SetUnitParamTypes/SetUnitReturnPatterns` | by-value-flow (pointer-held, read-only) | no | shared with `FnParam.Pattern` / `FnSig.ReturnPatterns` | precedent |
| 2805-2809 | probe state: `copy(p.fnRecs, es.fnRecs)`, `copy(p.units, es.units)`, `copy(p.openUnitRecs, …)` | slice copies of pointers/ints (not Values) | no | — | none |
| 3614-3617 | `e := entry` (`core.DefEntry` copy holding `Body core.Value` + `TypeDef *Type`); `fnType: &e` on the event | explicit-copy (entry snapshot) | yes | the def table entry changes later (undef/redefine); the event records the install-time entry | frozen descriptor |
| 3817, 3888 | `elem := NewCarrier(f.firstElemType / elemType)` | construction | no | — | none |
| 4383-4385, 4784-4790 | `inputs[i]` param carriers for a stored/closure body compile | construction | no | — | none |
| 4710, 4780, 12819-12822 | `tokens := lst.Slice()` / `body = append(body, lst.Get(j))` — a stored body list's tokens copied for a unit compile (`fnUnitRec.body`) | slice copy | yes | the body list is a const / a container element the interpreter lane still runs; the unit keeps its own token slice | the unit could reference an immutable token list |
| 4851-4863 | `StampCompiledRef`: `a.SetCompiled(ref)` on the shared `*BoruImpl` of an interned fn const — the ONE sanctioned in-place write into the pool ("Mutates the shared *BoruImpl pointer, so the interned const reflects it"), restricted to pre-publication consts (`stamp_runtime.go:258-262`) | in-place (already policy-shaped) | yes | every holder of the fn value sees the stamp — intended | none; note the publication fence it relies on |
| 5391-5460 | `Materialise`: `orig, ok := es.origByID[v.ID]; return orig` for a carrier; copy-on-first-change `elems = append(nil, d.Elems...)`; `nv := v; nv.Data = ListPayload{Elems: elems}` (5425-5427) / `nv.Data = MapPayload{M: nm}` (5454-5456) | explicit-copy (container re-mint with members materialised) | yes | the checked literal keeps its stripped members for the analysis; the materialised value is a NEW const (`ConstOperand(es.intern(...))`) | a constructor from (id, parent, payload, pos) — construction, not a struct copy |
| 5564 | `kept = append(nil, stk[:i]...)` phantom filter | slice copy | no | — | none |
| 7095-7099 | list elements gathered by `rl.Get(i)` | slice copy | no | read-only | none |
| 7463-7560 | `NoteLiveRead(v *core.Value, …)` / `seatLiveRead`: `v.ID = GenerateID(IDPrefixForType(v.Parent))` through the pointer to core's COPY | explicit-copy (pointer write into a copy) | yes | the def binding's ID (see check_recovery.go:82) | construct the read value |
| 8864-8870 | `applyWindowFits`: `sigArgs[i] = args[len-1-i]` | slice copy | no | — | none |
| 9766-9768 | `depth` fold: `n := NewInteger(…); n.ID = …` + `append(append(nil, preserved...), n)` | construction + slice copy | no | — | none |
| 9821-9829 | `pick`/`roll` folds: `picked := preserved[idx]`; `append(append(nil, preserved...), picked)` — one value appears twice on the residual with ONE ID | by-value-flow (duplicate) | no | the interpreter's `pick` re-pushes the same Value by value too (same ID, same payload) | a pointer model yields the same identity |
| 10241-10246, 10272-10277 | `RecordTypedBind(Run)`: `sp := spec` (`TypedBindSpec` copy for the event, not a Value); `out.ID = GenerateID(…)` on the by-value `out` param, `setProduced(out, seq)`, returned to the caller as the binding | explicit-copy (ID re-mint) | unsure | `out` may alias the input `in` when `Unify` returned the input unchanged (DepScalar keeps the base tag); the operand was already resolved by `in.ID` before the mint | per-production identity |
| 10431-10441, 11520-11530 | de-collision: `outs[i].ID = GenerateID(…)` when a result collides with an earlier product (and is not an arg); `outs[i].ID` minted for an empty-ID module value | in-place on the ReturnsFn's fresh slice | no (as a copy) | — | shows IDs are production keys |
| 11185-11212 | stored-body lists: `elems := lst.Slice()`; `rebuilt[j] = carrier or elems[j]`; `ConstOperand(es.intern(WithPos(NewList(rebuilt), a)))` | slice copy + construction (+ pos copied from the token) | yes | the original list `a` stays the interpreter lane's operand; the VM lane gets the rebuilt const | construction (what this is) |
| 11434 | `a = core.NewTypeLiteral(t)` — a bare type-name word resolved to a node copy for operand resolution | explicit-copy (node) | yes (node) | the canonical node | type-reference value |
| 13599-13608, 13676-13680 | `append(append(nil, b.ThenStk...), b.ElsStk...)`; `*p` derefs of `BranchRecord.ThenValue/ElsValue *core.Value` | slice copy + deref reads | no | read-only | none |
| 14320-14323 | `shuffleSource`: marker integers | construction | no | — | none |
| 15961-15968 | `internType`: `TypeRef{Name, ID}` — types pooled as DESCRIPTORS, not Values | construction | no | — | already policy-shaped; `OpPushType` re-resolves the canonical node at run time |
| 17936 | `copy(names, rec.locals)` | strings | no | — | none |
| 18984-18986 | `NewCarrier(TAny)+Dynamic` apply collapse | construction | no | — | none |
| 20966-20971 | `anchored := append(nil, window...); anchored[i].SetPos(names[i].Pos)` — window values re-anchored at their READ for a tail-anchoring test | explicit-copy (SetPos on copies) | yes | the window values are the recorded dispatch window (lowered into `CallWindowOperand.Value`); their positions must stay the producers' | pass per-slot anchors to `replayIsBodyTailAnchored` |
| 7846-7910 | `emitCheckpoint` rollback: `es.consts = es.consts[:cp.consts]` and index-map trims after a declined probe | by-value-flow (pool snapshot by length) | yes | the probe's consts are discarded wholesale | none (length-based) |
| 4130-4150 | `freshenConst` / `constKeep` bookkeeping for `OpPushConstFresh` | by-value-flow (pool policy) | yes | documents that the interpreter constructs a body literal per evaluation while embedded binding reads stay shared | compile literals as construction code |

## compiler/go/bytecode.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| 860, 902-909 | `PolyRef.Raw map[int]core.Value`, `PolySplit.Beneath/After []core.Value`, `Words map[int]core.Value` | by-value-flow (program table) | yes | every run and every `ForkConcurrent` of the Program; `splitBeneath` substitutes per run into a copy (vm_poly_nomatch.go:190) | immutable descriptors |
| 1036 | `NewClosure`: `core.Value{Parent: TFunction, Data: ClosurePayload{…, Captures: captures, …}}` | construction | no | — | none |
| 1142 | `CompiledFnRef.Captures []core.Value` | by-value-flow | yes | the stored ref's captures (a detached stamp's construction-time values) | construction at stamp time (what it is) |
| 1291 | `SigRef.SpliceOuts []core.Value` | by-value-flow (table) | yes | shared by runs | descriptors |
| 1375-1377 | `NativeSplit.Body core.Value`, `Beneath/After` | by-value-flow (table) | yes | `nativeSplitRaise` substitutes `sp.Body` into a copied window | descriptors |
| 1675, 1964, 2023, 2040-2041, 2564 | `DynMethodSpec.Island`, `LandingWord.Island`, `StmtIsland.Island`, `LoopCont.Body/After`, `DeoptSpec.Island` | by-value-flow (table; sub-slices of ONE backing array shared with `Program.Body`, `lower.go:4106-4111`) | yes | every island, `Program.Body`/`CompiledFn.Body` and every run share the tokens; the VM copies before each island run | immutable token descriptors; the interpreter must not write into tokens it did not construct |
| 1727 | `Program.Consts []core.Value` (+ `ConstKeep`, `ConstLocals` 1880-1891) | by-value-flow (the pool) | yes | see risk 1 | scalars shared only if never written; compounds/fns constructed |
| 1775, 2430 | `Program.Body`, `CompiledFn.Body []core.Value` | by-value-flow (table) | yes | islands slice them | descriptors |
| 2103 | `RestartSrc.Val core.Value` (+ `Fresh`) | by-value-flow (table) | yes | `seatPrefix` clones a Fresh const per evaluation and pushes a non-fresh one by value | construction per run |
| 2184 | `CallWindowOperand.Value core.Value` | by-value-flow (table) | yes | the recorded window value baked for the no-match report | descriptor |
| 2301, 2320, 2849-2854 | `CompiledFn.ReturnPatterns/ParamPatterns []*core.Value`, `ReturnPattern(k) *core.Value` | by-value-flow (pointer-held, read-only) | no | shared with the sig's patterns | precedent |
| 2453, 2485 | `CompiledFn.SpecFallback core.Value`, `SpecGuard.Fn core.Value` | by-value-flow (table) | yes | the guard compares runtime args against the recorded fn value; `specFallbackInputs` hands SpecFallback to the callback seam | descriptor / identity token |
| 2581-2586 | `PolyRef.RenderWindow`: `out := append(nil, window...); out[i] = v` (Raw literal substituted for the report) | slice copy | yes | `window` may be the VM's stack/scratch | construct the report window (what this is) |

## compiler/go/callable_words.go, body_map.go, code_effect.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| callable_words.go 251 | `spec := *sig.Callable` (`CallableSpec` copy, not a Value) | table copy | no | — | none |
| callable_words.go 360 | `bodyToks := bodyList.Slice()` for the closure-body compile | slice copy | yes | the lambda's body list is a const / token the interpreter lane still runs; `fnUnitRec.body` keeps the copy | immutable token list shared by reference |
| callable_words.go 519-525 | `callbackSourceSpec`: `out := *ret` (`ClosureRetSpec` copy); `out.Source = &src` — the address of the by-value `src` param escapes into the `ClosureRet` program table | by-value-flow (pointer to a private copy) | yes | `src` is the recorded callback fn value; the table must hold its own instance (the VM later copies `*cl.Source` and substitutes captures, vm.go:893) | an existing pointer-held Value; its pointee is a private copy by construction |
| callable_words.go 540, 555-560 | `ClosureParamSpec.Patterns []*core.Value`, `paramSpecPatterns` | by-value-flow (pointer-held, read-only) | no | shared with `FnParam.Pattern` | precedent |
| callable_words.go 715-724 | `if &out[0] == &inputs[0] { out = append(nil, inputs...) }; out[i] = check.ParamInputCarrier(pt)` — copy-on-first-change keyed on backing-array identity | slice copy + address identity | yes | the caller's `inputs`; the gradual re-generalisation is per unit | construct the unit's inputs (what this is) |
| callable_words.go 1275-1301 | `NewCarrier(acc.Parent)` / `core.ValueCarrier(acc)` / `check.ElementCarrierOf(data)` / `keyValCarrier` | construction (from parts) | no | — | none |
| callable_words.go 1378-1396 | `pairCarrier` / `keyValCarrier`: `NewValueRaw(TMap/TKeyVal, MapPayload{M: om})` | construction | no | — | none |
| body_map.go 117-118 | `w.seq(body.Slice())`; `BodyHasSentinelDeep(r, core.NewList(body.Slice()))` — two read-only copies of a list's tokens, one wrapped in a fresh list for a predicate | slice copy (API shape) | no | — | read-only view |
| body_map.go 163 | `quotedData`: `v.Quoted = false` on the by-value param before the data walk | explicit-copy | yes | `v` is a quoted list token of the body (program token); the walk is read-only, so the copy only protects the token's quote | walker parameter "treat as data" instead of clearing the flag |
| code_effect.go 115-116 | `c := NewCarrier(TList); c.Data = CodeEffectInfo{Out: out, Analysed: true}` | construction | no | — | none |

## compiler/go/compiler_dispatch_record.go, drift_window.go, eager_literal.go, kept_defs.go, kept_defs_scan.go, kept_live_deopt.go, landing_restart.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| compiler_dispatch_record.go 350-358 | `tryFoldParkedMemberFn`: `member, ok := readFnMemberValue(args)` (a copy of the container element); `member.ID = GenerateID(…)`; `es.noteFolded(member)`; `outs[0] = member` | explicit-copy (ID re-mint) | yes | the container element keeps its ID; each folded read site needs its own identity (`foldedMembers` keyed by it, `foldedBodies` by the body address) | per-production identity |
| compiler_dispatch_record.go 1353-1360, 1434-1440 | fallback span: `span := make(…)` with `NewWord(word)` + the args by value; `ns` stack-form reorder | slice copy (snapshot into a `FallbackSpan` program table) | yes | the args are the pass's operands, mutated later (narrowing, re-mints); the span must hold the then-values | frozen descriptors |
| compiler_dispatch_record.go 1555-1561 | de-collision `outs[i].ID = GenerateID(…)` on the fresh outs | in-place on fresh values | no | — | none |
| drift_window.go 136 | `wordTok := core.WithPos(core.NewWord(w.Name), e.Tape.At(e.Pointer))` interned as a const | construction (+ pos copied from the tape cell) | no | — | none |
| drift_window.go 167-175 | `NewDynamicCarrier(TAny)` result; `f := es.eventInfo[seq]; f.variadicResult = true` (flag struct, not a Value) | construction | no | — | none |
| eager_literal.go 91-105 | probe window: `window[k] = es.consts[op.idx]` (pool entries copied, read-only) / `NewTypeLiteral(t)` (node copy, read-only) | slice copy + node copy | no | read-only probe | pass references |
| kept_defs.go 103-118 | `keptReadSeatedLive(v *core.Value)`: `*v = WithPos(core.ValueCarrier(*v), *v)` / `*v = WithPos(NewCarrier(v.Parent), *v)` — the pointee (core's binding copy) REPLACED by a carrier carrying the read's pos | explicit-copy (pointer write into a copy) | yes | the def binding must stay concrete for later reads and the final program (see check_recovery.go:82) | construct the read value |
| kept_defs.go 352-376 | `NoteValReadLive(v *core.Value, …)` → `keptReadSeatedLive`, `computedLeakGradual`, `seatLiveRead` | explicit-copy (pointer writes) | yes | as 103 | as 103 |
| kept_defs.go 390-395 | `computedLeakGradual`: `v.Dynamic = true` through the pointer | explicit-copy (pointer write) | yes | as 103 | as 103 |
| kept_defs_scan.go 65 | `return lst.Slice(), true` — a list's tokens copied out for a scan | slice copy (API shape) | no | read-only | read-only view |
| kept_live_deopt.go 705-710 | `out := make(…)`; body tokens with a nested list's tokens spliced in — a new island body | slice copy (construction of a table) | yes | the unit's `Body` is a program table; the spliced variant is a new island | descriptors |
| landing_restart.go 120 | `substPlan.val core.Value` — the const an island writes for a folded read | by-value-flow (table) | yes | becomes `RestartSrc.Val` | descriptor |
| landing_restart.go 1319-1320 | `l.Slice()` — an Eval list's tokens handed back as a landing island's tokens | slice copy | yes | the list literal's storage (a const); the island table must not alias it | descriptors |
| landing_restart.go 1867-1868 | `loopCont.elems/after []core.Value` | by-value-flow (table) | yes | becomes `LoopCont.Body/After` | descriptors |

## compiler/go/lower.go, poly_raw.go, splice_outs.go, stamp_runtime.go, stored_fn_proof.go, unit_memo.go, user_poly.go, call_window.go, region_desc.go

| line | site | kind | load-bearing? | what shares the original | replacement under the policy |
|---|---|---|---|---|---|
| lower.go 857 | `lowerer.landingBody []core.Value` (= `es.rootBody` or a unit's `Body`); islands are `lw.landingBody[r.token:]` sub-slices (4106-4111 `spec := *c.dynMethod; spec.Island = lw.landingBody[r.token:]`) | by-value-flow (ONE backing array behind `Program.Body` and every island table) | yes | all islands and the program body alias each other's tokens | immutable token descriptors |
| lower.go 1587 | `code := *lw.code` (`[]Instr` header copy) | not a Value | no | — | none |
| lower.go 1655-1663 | `tail := make(…)` island tail assembly (`inner` + `NewCloseParen()` + `toks[i+1:]`) | slice copy (construction of a table) | yes | the tokens come from the body list / program | descriptors |
| lower.go 5645-5652, poly_raw.go 125-134 | `out := *sp; out.Live/Words = …; return &out` — `PolySplit` copy-on-write | table copy (not a Value) | yes | the `PolyRef` table references the split | per-site tables |
| poly_raw.go 10-24, 114-124 | `raw map[int]core.Value`, `words map[int]core.Value` built from consts/tokens | by-value-flow (table) | yes | `PolyRef.Raw`/`PolySplit.Words` | descriptors |
| splice_outs.go 35-40 | `es.spliceOuts[seq] = append(nil, outs...)` | slice copy (snapshot) | yes | `outs` re-minted later | frozen descriptors |
| stamp_runtime.go 264-330 | `StampFnValue`: `sigs := make; copy(sigs, fd.Signatures)` ("so the stamp never writes through a shared pointer of a published value"); `na := Impl.Clone(); na.SetCompiled(ref); sigs[i].Impl = na`; `fd.Signatures = sigs; out := v; out.Data = fd` | explicit-copy (payload + Value header) | yes | a PUBLISHED fn value with concurrent readers (`stamp_runtime.go:258-262`: mutating the shared `*BoruImpl` from a store word would race) | construct the stamped value from parts (what it does); the header copy only carries ID/pos/flags — a constructor would replace it |
| stamp_runtime.go 532, 543 | `decls.Slice()` / `l.Slice()` read-only walks | slice copy (API shape) | no | — | `Get(i)` |
| stored_fn_proof.go 140-146 | `vals[i] = es.consts[op.idx]` proof window | slice copy (pool entries, read-only) | no | — | references |
| unit_memo.go 415-431 | `bindCarrier(v)`: `return v` for a carrier, else a fresh carrier from `v.Parent` / `ValueCarrier(v)` | construction | no | — | none |
| user_poly.go 45, 175-207 | `userPolyPlan.outs []core.Value`; `j := NewCarrier(bound); j.Dynamic = true; plan.outs[pos] = j` | construction | no | — | none |
| user_poly.go 304 | `body := append(nil, s.Body()...)` for the arm unit compile | slice copy | unsure | as check_fnbody.go:329 | as check_fnbody.go:329 |
| user_poly.go 310-315 | `genArgs[i] = check.ParamBodyCarrier(p)`; `pats []*core.Value` | construction / pointer-held | no | — | none |
| call_window.go 31-35, 60 | `pendingWindow.win/prefix []core.Value`, `callWinOp.value core.Value` — the dispatch window as the record resolved it | by-value-flow (snapshot → `CallWindowOperand.Value`) | yes | the window values are the stack values the pass keeps mutating | frozen descriptors |
| region_desc.go 154 | `SlotDesc.Token core.Value` ("READ, never dispatched") | by-value-flow (table) | yes | `newRegionHost` copies it into a per-run window | descriptor |
| region_desc.go 183, 195 | `ClosureRetSpec.Patterns []*core.Value`, `Source *core.Value` | by-value-flow (pointer-held) | no (Patterns) / yes (Source: its pointee must be a private instance, see callable_words.go:523) | — | precedent |
| region_desc.go 313, 335 | `RegionState.StackResidual []core.Value`, `ReachBound core.Value` — probe inputs | by-value-flow (per-probe) | no | — | none |

## Files with no Value-copy hits

eng/go: `compiled_runtime_vm.go`, `vm_args_debug.go`, `vm_args_release.go`, `vm_callback_unit.go`, `vm_poly_barrier.go`, `vm_poly_cache.go`, `vm_poly_seed.go`. check/go: `check_make.go`, `indexcheck.go`. compiler/go: `arm_pending.go`, `branch_carried.go` (its `seed = &init` at 203 is an `EmitOperand`, not a Value), `call_result_island.go`, `compiler_dispatch_record.go` beyond the rows above, `dispatch_hooks_install.go`, `do_restep.go`, `dyn_body_one.go`, `fit_restart.go`, `fn_local.go`, `kept_defs.go` beyond the rows above, `prefix_island.go`, `read_site_guard.go`, `region_capture.go`, `region_complete.go`, `region_oracle.go`, `region_record.go`, `region_route.go`, `region_validate.go`, `root_read_statement.go`, `stored_live.go`, `taken_landing.go` (zero-sentinel only), `user_poly_plan.go`.
