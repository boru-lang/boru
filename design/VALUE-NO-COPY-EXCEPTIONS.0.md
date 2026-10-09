# Sanctioned exceptions to the no-copy policy

**Status: proposal, awaiting the maintainer's approval** (decision 4 of
[VALUE-NO-COPY.0.md](VALUE-NO-COPY.0.md) §7).

Companion to [VALUE-NO-COPY.0.md](VALUE-NO-COPY.0.md). Every copy of a
Value that is allowed to exist once the policy holds is named here, with
the site, the reason the copy is right, the invariant that keeps it safe,
and whether it is permanent or expires with a phase of the plan. Anything
the two copy checks report that is not covered by an entry below is a
defect: `go vet -copylocks` for copies of a single `Value`, and the
bulk-copy check phase 0 adds for `copy` and variadic `append` over
`[]Value`, which copylocks cannot see. Line numbers are from main at `6780a6030`; the per-module
inventories in
[handover/interpreter-perf/inventory/](handover/interpreter-perf/inventory/)
hold the full tables.

## The rules the exceptions are measured against

- **R1. One owner until published.** A Value is owned by whoever minted it
  until it is published: bound by `def`, stored in a container, captured,
  pushed to a second cell, interned in a pool, or handed to a host.
- **R2. Immutable after publication.** No field of a published Value is
  written through any reference. In-situ modification is permitted only
  on program tokens by the pipeline stages before execution begins, and
  on a fresh value before it is published.
- **R3. Occurrence state lives in the slot.** Position, quoted, eval,
  ascription, check-mode tags, binding names and dispatch flags belong to
  the tape cell, stack cell, arg slot, binding entry or pool entry that
  holds the value, never to the value.
- **R4. What "copy" means.** Duplicating a Value struct, or the payload
  graph under it, is a copy. Minting a new Value from parts is
  construction. Copying an array of references or of cell records is
  neither.
- **R5. Mutable-by-design payloads are shared, not copied.** FlexList,
  FlexMap, class instances, Store layers, Table rows and an Error's data
  map mutate in place; that is their semantics. `StructUtil.clone` and `send` are the
  only ways to obtain an independent one.
- **R6. Every exception is written down.** Permanent entries carry the
  class letter; transitional entries name the phase that retires them.

## Class A. The language says "copy" (permanent)

| id | site | why the copy is right | invariant | status |
|---|---|---|---|---|
| A1 | `StructUtil.clone` from `boru:struct-util` (there is no unqualified `clone` word): `CloneValue` (`lang/go/native/clone.go:19`, exported at `struct_module.go:92`); host bodies through `DeepCloner` (`core/go/clone.go:146-152`) | the word's meaning is a deep, independent copy of a mutable container: for a list or map `xs`, `(StructUtil.clone xs) eq xs` is false and nested mutable payloads cannot alias; scalars and other immutable payloads come back shared, so `(StructUtil.clone 1) eq 1` is true | the cloner builds an equal value graph with fresh identities for the mutable payloads and shares the immutable ones; its header step (`cloner.withPayload`, `clone.go:74-78`) becomes a constructor, not a struct copy | permanent |
| A2 | `send` at the process boundary (`lang/go/native/native_process.go:335`) | a message crosses a task boundary; a receiver must never observe the sender mutating a flex container or store it still holds | a message is either deep-cloned at the boundary or provably immutable; nothing else crosses tasks | permanent, unless ownership transfer of messages is designed later |
| A3 | serialization and deserialization: export bundles, keyrings, `jsonify`/`nodify`, the wire readers, the wasm playground | an external representation is constructed and values are reconstructed on read | bytes are not Values; a read mints new values | permanent (not a Value copy; listed to stop the argument) |

## Class B. Constructions the policy must allow by name (permanent)

| id | site | why | invariant | status |
|---|---|---|---|---|
| B1 | one instance per evaluation of a literal: pending list and map literals (`core/go/engine.go:5287-5294, 5322, 6030, 5154-5158`), fn-body literals (`(mk) eq (mk)` is false, `TestFnBodyContainerLiteralIdentity`), the scalar-literal fast path, the VM's fresh-push family (`eng/go/vm.go:2300, 4634, 4769` with `Program.ConstKeep`) | the spine of a literal is a new container each time it is evaluated; a binding's container embedded in it stays the binding's instance | elements are shared by reference; only the spine is new; the deep clone that implements this today (`CloneValueKeeping` + `WithFreshFnIdentity`) is the transitional means, construction code (`OpMakeList`/`OpMakeMap` over shared consts) the end state | permanent as a rule; the clone implementation expires at phase 3/4 (D7) |
| B2 | the value-semantics column: `set`, `push`, `pop`, `shift`, `unshift`, `merge`, `setpath`, `sort`, `reverse`, `take`, `unique`, `create`, `listAll` and kin (`native_storage.go:2078-2116`, `listops.go:49-122`, `merge.go:42-97`, `setpath.go:240-302`, `native_sort.go:48-63`, `native_array.go:946-1078`, `list.go:65`, `create.go:92`) | a word returns a new container and leaves the receiver untouched | the receiver is never written. With reference elements (phase 4) or a persistent container representation, the new container's elements are the old ones and no `Value` is copied. While the header is a by-value struct, building the new element array copies each unchanged element's header: a bulk copy sanctioned until one of those lands | permanent if decision 2 keeps value semantics; void if lists and maps go in-place |
| B3 | self-append over one flex list, `append f f` (`native_flex.go:318-323`) | the read set must be fixed before the receiver grows | a length-bounded read of the elements, which needs no copy at all; the current `Slice()` snapshot is the transitional form | permanent as a read; the snapshot expires at phase 2 |
| B4 | a function value per construction: `WithFreshFnIdentity` on each evaluation of a fn literal (NUR288), a closure source re-created with this closure's captures (`vm.go:893-903`) | identity is per construction, the body is shared | mint a new Function value sharing `FnDefInfo`'s body with its own identity and captures | permanent |
| B5 | Store layers: `CowSet` (`native_storage.go:1879-1887` → `core/go/core_helpers.go:1004`), tombstones travelling with a layer | `set` on a store creates a new layer with a prototype link | a layer holds references; the prototype chain is shared; no Value is duplicated | permanent (the design is copy-on-write of the layer structure) |
| B6 | retag constructors: a new value with a different `Parent`, ascription or carrier shape built from an existing value's parts (the `MintRetagged`/`MintCarrierFrom` family that replaces `ReparentValue`, `FreshenDefault`, `JoinCarriersInner`'s `out := a`, `ReturnsIdentity`, `RetagFlexElem`, `cloner.withPayload`) | `def p:Pos n` must not retype `n`; a typed bind, a refine, a carrier join and a flex retag each need a value that shares the payload and differs in tag | written as construction (`NewValueRaw(parent, v.Data)` plus facets), never as `x := v; x.Parent = t`; the result has its own identity | permanent; the copy-returning helpers that do this today expire at phase 3 (D6) |
| B7 | `d2AdoptTyped`'s in-place retag of a flex child (`native_storage.go:560-590`) | a later mutation through the original flex handle must stay visible | this is the policy's normal case (in situ on a mutable-by-design payload), not an exception; listed because its comment calls it special | permanent |

## Class C. Reference and cell copies that are not Value copies (permanent)

| id | site | what is copied | invariant | status |
|---|---|---|---|---|
| C1 | tape cells: program load (`engine.go:1731-1733`; `tape.go:160-166, 185-224`), `Splice`/`Insert`/`MoveGap`/`grow`, body re-splice per call and per iteration (`engine.go:7637, 9027-9029, 9083-9088`; `registry.go:1896`; `core_helpers.go:425, 595`), paren expansion (`engine.go:5633-5657`), sealed regions (`sealed.go:118-119, 158-159`) | cell records `{V *Value; occurrence}` | a cell's value is never written through; evaluation replaces the pointer; a body's tokens come back intact after every run | permanent |
| C2 | pool buffers: results copied out of pooled tapes, engines and loops because the next reload overwrites them (`engine_pool.go:60-62`; `invoke.go:396-412`; `registry.go:1930-1942`; `sealed.go:57-61`; `loop.go` `takeCallLoop`/`putCallLoop`; `Tape.TakeAll` `tape.go:496-499`) | arrays of references | a released buffer is cleared so it pins nothing (today `resolvedScratch`, `loopTokens`, `peScratch` and `excludeScratch` are not cleared, `engine_pool.go:642-670`; phase 2 fixes that) | permanent |
| C3 | table snapshots: `DefTable.Clone`/`SnapshotEntries`/`RestoreEntriesSnapshot` (`deftable.go:318-544`), `ForkConcurrent` (`fork.go:39-40`), `CompileSandbox` (`compile_sandbox.go:54-57, 95-112`), the predicate sandbox's TypeTable maps (`util.go:391-409`), `withRecoveryRaw` (`engine.go:11565-11584`), fn baselines (`registry.go:1839-1847`) | binding entries and table maps (references plus slot attributes) | a check pass or a sandbox never writes through a shared value (R2), so rollback is restoring the table | permanent |
| C4 | observation snapshots: trace fires (`engine.go:474, 1818`; `sealed.go:178, 225`), the debugger ring of 64 (`cmd/go/internal/debugger/debugger.go:66, 160, 440`), `RunningEngineStates` and `CurrentStack` (`registry.go:598-612, 706-750`), the REPL's `lastStack`, the debugger's `resolvedData` | arrays of cell records | "the live tape is never exposed or mutated" (`registry.go:706`) stays the contract; a past snapshot shows past cells with present values, which is exact for immutable values and shows present contents for flex containers and stores, as the registry is not rewound either (`debugger.go:654`) | permanent, with that limit written into the debugger's help |
| C5 | captures: `CapturedBinding{Name, Value}` (`value.go:1192`; `fn_capture.go:322, 434`), installed per call by `InstallFrameBinding` (`registry.go:1862`) | a reference to the value bound at capture time | `def` pushes a new entry rather than writing the old one, so "rebinds don't affect captures" holds without a copy; the lang guide's "the captured Value is a struct copy" is rewritten | permanent (a reference, not a copy) |
| C6 | the per-call `args` list (`argsCopy`: `engine.go:7578-7579`, `registry.go:1851`, `core_helpers.go:484-486, 538`; `PushLazy` in `argsstack.go`) | a list constructed from the call's references | the list is the call's own; its elements are shared | permanent |
| C7 | matcher scratch: `resolvedScratch`/`EffectiveResolved` (`engine.go:10684-10692`), `rearrangeForForward` (`engine.go:4506-4535`), `sealedHold` (`sealed.go:263-290`) | buffers and permutations of references | the sealed-region rule stays: a holder's view is never rebuilt underneath it | permanent |

## Class D. Transitional allowances (each expires at a named phase)

Until the phase named, these copies stay, under one rule: the copy is
made from a value the code owns, and nothing is ever written through a
shared pointer instead.

| id | site | what it protects today | retired by |
|---|---|---|---|
| D1 | `StripAscribed` at every delivery boundary (`value.go:2267-2273`; 19 VM sites; `execMatch`, fn-param binding, `vmMakeListToMark`) and `as`'s ascribe-on-copy (`native_type.go:753-755`) | the match-time ascription never reaches a handler, binding or container | phase 3: `asc` becomes an operand-slot property |
| D2 | position stamping on copies: `WithPos`/`WithPosAt`/`SetPos`-on-copy, `stampResultPos`, `stampFnPos`, `stampFnResultPos`, and the per-call leaf frame skeleton copy that exists because of it ("the per-call COPY is mandatory", `core_helpers.go:465-476`) | a read reports its own site, not the def site; the shared skeleton stays clean | phase 3: `pos` moves to the cell; the skeleton becomes shared and immutable |
| D3 | flag flips on copies: `Quoted` (`quote` `natives.go:515-526`, param bind `core_helpers.go:500, 561`, macro operands `macro_expand.go:170-173`), `Eval`, `ReachGroup`, `Undefined`, `FailedDispatch`, `FnDefInfo.Applied` (`apply` `native_valof.go:499`), `ClosurePayload.SigMatched`/`RetTrim` (`engine.go:3826, 7459, 4600-4604`; `collect_kernel.go:896-899`; `vm.go:2549, 2902, 4967, 5549, 5612`) | a flag set for one occurrence never reaches the binding, the pool constant or the program token | phase 3: occurrence flags on the slot |
| D4 | check-mode tags on copies: `tagged := v` with `SetDynFrom`/`seatLiveRead`/`computedLeakGradual` (`engine.go:2952, 3248`; `check_recovery.go:82-96`; `kept_defs.go:103-118, 390-395`), `toCarrier` (`carrier.go:307-452`), narrowing copies (`carrier.go:1916-1918, 2020-2022`), ID re-mints on copies (`carrier_new.go:182-184`; the de-collision loops `emit.go:10431-10441, 11528`), container clones for provenance IDs (`native_storage.go:1449, 1586, 1595, 1627`) | the interpreter later runs on pristine bindings; one payload can carry two recorder identities | phase 3: recorder keyed by (event, index); carriers get their own slot |
| D5 | `NewTypeLiteral(t)` returning `*t` (`value.go:2803`; 27 call sites), the 24 `&v` orphan sites and the 12 `CanonicalType` repairs | a stamp on a literal never reaches the canonical lattice node | phase 3: a node-reference literal; `CanonicalType` retires |
| D6 | the copy-returning retag helpers: `ReparentValue` (`core_make.go:777, 1087`; `unify_list.go:147`; `typed_bind.go:44, 70`; `native_definition.go:1024, 1540, 1596, 1660`), `FreshenDefault`, `JoinCarriersInner`'s `out := a`, `ReturnsIdentity`, `RetagFlexElem`, `cloner.withPayload`, `const`'s interned re-mint (`native_const.go:76`) | the exemplar the user still holds keeps its type | phase 3: the B6 constructor family |
| D7 | the VM's deep clones per evaluation (`CloneValueKeeping` + `WithFreshFnIdentity`, `vm.go:2300, 4634, 4769`) and `seatConstLocal`'s one-per-call clone | one instance per evaluation while literals are pooled constants | phase 3/4: literals compile to construction code |
| D8 | fn payload copies for names and homes: `installDef`/`installFnDef` renames (`core_helpers.go:98, 276-296, 760-763`), `nameFrameFns`/`nameClosureValue` (`vm.go:568-606, 4800`; `vm_dyn_words.go:117-165`), `rfn := *fn` for the registry rebind (`engine.go:6458`), `InstallTypeBody`'s name writes (`core_type.go:420-488`) | `def a (f/v)` does not rename `f`; a cached compiled form is not corrupted | phase 3: names and homes on `DefEntry`; the registry passed beside the fn |
| D9 | `FnDefInfo` copied by every `.(FnDefInfo)` assertion (232 bytes; `engine.go:7295-7305, 7482, 7592, 7836, 8384, 8416, 10038`), `ForwardInfo` re-minted per collected arg (`engine.go:5103`), per-dispatch signature views (`vm.go:2792`; `vm_fnvalue_park.go:149`) | nothing; cost only | phase 2: pointer payloads, views normalised at construction |
| D10 | the Value header itself passed by value (~40 bytes after phase 3: `Parent`, `Data`, `tmeta`, `elem`, `modns`, flags) | the representation until the pointer switch | phase 4; becomes permanent if decision 3 declines phase 4 |

## Class E. Not sanctioned (listed so they are not mistaken for exceptions)

| id | site | why it goes |
|---|---|---|
| E1 | handler-side `make`+`copy` before `Run` (`native_control.go:803, 835, 1631`; `native_definition.go:1054`; `native_process.go:592`; `native_temporal_await.go:362`; `native_temporal_timeout.go:17-18`; `modules/test.go:304, 357, 500`; `modules/debug.go:389, 563`; `forloop.go:34, 54`; `whileloop.go:28-48`) | the engine copies the program into the tape itself; these are double copies (phase 2) |
| E2 | read-only `Slice()` walks (~30 across `query.go`, `native_misc.go`, `report.go`, `canon.go`, `fn_capture.go`, `carrier.go`, `kept_defs_scan.go`, `vm_token_body.go`) and the double copies `SpliceExpand` over `Slice()`, `stepMove`'s body copy before `Splice`, the dead `MarkInfo.Body` copy in `stepMoveCont` | an element view or iterator replaces them (phase 2) |
| E3 | `dup`, `over`, `tuck`, `pick`, `stack N` independence by copy (`native_stack.go:141-199`; `natives.go:677`) | two cells share one immutable value (R2) |
| E4 | `SetAtomReferent`'s private copy (`value.go:2761-2770`) and `resolveAtomReferents` stamping nested lists in place through the shared array (`engine.go:1616-1633`) | the referent is an occurrence or binding attribute (phase 3) |
| E5 | `FnUtil.memoize`'s cached copy and `curryLevel`'s per-level copy (`modules/fn.go:550, 566-575`), `const`'s exemplar snapshot (`native_const.go:88-91`) | immutable-after-publication makes the cache and the exemplar safe to share; a curry level constructs its own bound list |
| E6 | `ResolveWordsDeep` rebuilding both `Unify` operands (`resolve.go:75-155`), `FnUndefUnifier`'s transient values (`unify_fnundef_named.go:64, 100`), `snapshotPredicateState`'s per-predicate map clone beyond the table snapshot | rebuild only what resolves; mint once |
| E7 | the stale "cloned for a fresh ID" at `check_fnbody.go:646` | `CloneValue` returns a Function unchanged; the comment promises what does not happen |

## How to use this list

- A finding of either copy check is matched to an A–D entry by site or
  it is a defect. Phase 0's `make vet-copies` records both counts per
  module, for every module `go.work` lists; the ceilings only go down.
- When a D entry's phase lands, its row moves to class E in the same
  change and the ceilings drop by its sites.
- A new copy needs a row here before it is merged, with the class, the
  invariant and, for class D, the phase that retires it.
