# The no-copy policy for Values: impact, refactor plan, performance estimate

Policy under review: *"Value structures should never need to be copied.
They can be modified in situ as needed by any stage, but copying is
forbidden."*

**Status: proposal, not started.** Review of main at `6780a6030`
(2026-10-09); no code was changed. The decisions in §7 are open, and
nothing in §6 should start past phase 2 before they are made. The policy
supersedes the plan in issue #531 ("borrow for reads, shrink the header,
then split Value from Type"), whose first two steps survive here as phases
1 to 3.

Companion documents:

- [VALUE-NO-COPY-MEASUREMENTS.0.md](VALUE-NO-COPY-MEASUREMENTS.0.md) — the
  profiles, the copy counts and the benchmarks every number below comes
  from.
- [VALUE-NO-COPY-EXCEPTIONS.0.md](VALUE-NO-COPY-EXCEPTIONS.0.md) — the
  sanctioned exceptions: every copy allowed to exist once the policy
  holds, and when each transitional one expires.
- [handover/interpreter-perf/inventory/](handover/interpreter-perf/inventory/)
  — four per-module inventories (`core-engine.md`, `core-value.md`,
  `eng-check-compiler.md`, `basic-lang-parser-cmd.md`), ~870 catalogued
  sites with line numbers at `6780a6030`. They were produced by four
  parallel read-only review passes; spot-check a row before relying on it.
- [handover/interpreter-perf/README.md](handover/interpreter-perf/README.md)
  — how to reproduce every measurement, the two experiment patches, and
  the probe corpora with their outputs on `6780a6030`.
- [INTERPRETER-PERF-HANDOVER.0.md](INTERPRETER-PERF-HANDOVER.0.md) — the
  handover of the session that wrote this review.

## 1. Verdict

1. **The policy is three changes wearing one sentence.** (a) A
   representation change: `core.Value` is a 104-byte struct that the
   compiler copies at 15,235 non-test sites that `go vet` can count
   (32,141 with tests), plus the bulk copies of `[]Value` it cannot;
   "never copied" means every one of them becomes a pointer. (b) A storage
   change: the state that a copy protects today is per-occurrence state
   that lives inside the Value (position, quoted, eval, ascription,
   check-mode tags, names); "modify in situ" is only sound once that
   state moves out of the shared value into the slot that holds it. (c)
   A language change: boru's list and map words return an updated copy
   and leave the receiver untouched; in-situ modification there is a new
   language, not a refactor. (a) and (b) are refactors with a clear plan.
   (c) is a decision the maintainer has to make explicitly.
2. **Copying is worth attacking.** Struct copy and zero routines are
   23–25% of CPU on the interpreter and on the compiled VM alike
   (measurements §2). Nothing else in the profile is that large and
   that mechanical.
3. **The literal end state is not the fast one.** A pointer Value that is
   heap-allocated per mint costs about three times what the copies it
   saves cost (measurements §4). The policy only pays off when
   values are arena-allocated or when the header shrinks first. A 25-line
   mechanical change (pointer receivers on the accessor methods) already
   buys 11% on the Stage6 suite (§5 there); the first three phases below
   are estimated at 20–30% less time; the final pointer switch is
   neutral at best unless scalars are interned.
4. **Recommended shape.** Adopt the policy as an *ownership rule*: a
   Value has one owner, every other holder references it, and nothing
   writes through a shared reference. Enforce it with two checks,
   ratcheted per module: `go vet`'s copylocks analyzer, which a
   zero-size `noCopy` field turns on for `Value` and which reports every
   copy of a single `Value`, and a small analyzer for the bulk copies
   copylocks cannot see, `copy` and variadic `append` over `[]Value`. Reach
   it in phases, each measured on the shipped benchmarks, and decide the
   final representation (§6, phase 4) on the numbers after phase 3 rather
   than now.

## 2. What the policy means in this codebase

**Two meanings of "copy".** (i) The Go struct copy: every by-value
parameter, return, assignment, range variable, slice element store and
tape cell write copies 104 bytes. This is cost, never semantics, and it
is the kind vet can count. (ii) The semantic copy: a Value copied so the
copy can differ from the original (`WithPos`, `StripAscribed`,
`ReparentValue`, `SetAscribed` on a copy, `v2 := v; v2.Quoted = true`),
a container whose element array is duplicated (`ReadList.Slice()`,
`make`+`copy`, `CloneValue`), or a snapshot taken for independence
(captures, forks, the debugger ring, `send`). These are where behaviour
lives.

**Two kinds of owner.** Before execution, the program tokens have one
owner at a time: the parser mints them, the checker annotates them, the
compiler records them. In-situ modification there is sound and is the
annotated-AST model the policy describes. At run time a Value is shared
the moment it is published: bound by `def`, stored in a container,
captured by a closure, pushed twice by `dup`, interned in the VM's
constant pool, or read from the lattice as a type literal. After
publication, an in-place write is visible through every holder.

**The reading this review adopts.** "Never copied" = the struct is
referenced, not duplicated, wherever it flows. "Modified in situ" = a
stage may write a Value it uniquely owns (a token before execution; a
fresh result before it is published) and may never write one it shares.
"Construction" (minting a new Value from parts, including a new container
whose elements are shared) is not copying. The reports flag the sites
where this reading changes behaviour; §4 lists them.

## 3. Where copies happen today

Counts are from `go vet -copylocks` with the marker (all Go, non-test,
every module in `go.work`; bulk copies through `copy` and `append` are
not in them) and from the four inventories (explicit sites, non-test).
`test/specfix` (337 copies) and `calc/go` (11) were counted after the
inventories were written and are not inventoried; `tools/piecetool`,
`test/solardemo` and `wpg` have none.

| module | struct copies (vet) | explicit copy-to-modify and container copies (inventory) |
|---|---|---|
| core/go | 5,509 | engine side ~120 sites: 31 occurrence-state copy-modify-writebacks, 19 token-run copies onto a tape, 14 snapshot/clone copies, 11 binding and type-node read copies, 9 pool/scratch aliasing copies. Value side ~330 rows: 24 header copy-to-modify (`WithPos`, `ReparentValue`, `StripAscribed`, `WithFreshFnIdentity`, carrier narrowing and joins), 5 `ReparentValue` calls, 21 payload copies re-boxed through `NewValueRaw` (fn renames at install, `InstallTypeBody`, `FreshenDefault`), 41 element-array copies, 26 `OrderedMap` rebuilds, 27 `NewTypeLiteral` node copies with 24 `&v` orphan-pointer sites and 12 `CanonicalType` repairs, 12 semantic `Value{}` sentinels, ~45 struct fields holding Values by value |
| lang/go | 4,902 | ~140 sites: ~74 `Slice()`, ~39 `make`+`copy`, 6 `CloneValue`, ~18 copy-then-flag, 4 re-mints |
| compiler/go | 1,698 | 9 whole-Value copy-to-modify, 2 payload copies, ~50 struct fields holding Values by value, 36 slice copies |
| basic/go | 1,361 | 67 sites: 35 `Slice()`, 19 `make`+`copy`, 12 `ReparentValue`/`WithPos`, 1 copy-then-flag |
| check/go | 709 | 14 whole-Value copy-to-modify (carrier derivation), 13 carrier re-mints, 52 slice copies |
| eng/go (the VM) | 606 | 19 whole-Value copy-to-modify, 7 payload copies, 19 `StripAscribed`, 8 `WithPos`, 107 slice copies, 7 per-run by-value holders |
| parser/go | 56 | constructs only; 1 `withPos` on a just-minted value |
| cmd/go | 46 | 2 copies, 4 snapshot-retaining fields |
| test/specfix | 337 | not inventoried |
| calc/go | 11 | not inventoried |

Seven families, in the order the plan treats them:

1. **Mechanical traffic.** Tape cells (`Tape.At/Set/Insert/Splice/
   MoveGap/grow`), stack cells, `args []Value`, returns, value receivers.
   Pure cost. The profile puts a quarter of it in one lattice walk
   (`IsAncestor` calling value-receiver `In()`/`Out()` through a pointer),
   a quarter in the step loop reading cells by value, a sixth in the
   matcher (`PlanMatch`, `resolvedIndicesBeforeInto`, `scanBoundaryToken`,
   `CollectCandidateScan`).
2. **Occurrence state copied, modified, written back.** ~31 sites in the
   engine, 19 in the VM, 14 in the checker, 9 in the compiler, ~18 in
   natives. The fields: `pos`, `Quoted`, `Eval`, `ReachGroup`,
   `Undefined`, `FailedDispatch`, `asc`, `Dynamic`/`dynFrom`, `ID`
   re-mints, `FnDefInfo.Applied`, `ClosurePayload.SigMatched`/`RetTrim`,
   fn names and return anchors. Load-bearing: the original is a binding,
   a pool constant, a program token or a canonical type node.
3. **Token runs copied onto a tape.** Fn bodies, loop and while bodies,
   `if` arms, `case` blocks, pending-literal elements, paren items, splice
   payloads, program load, and the VM's island copies before each run.
   Load-bearing only because execution rewrites cells in place; with
   pointer cells and pointer replacement these become pointer copies.
4. **Snapshots.** `ForkConcurrent`'s DefTable clone, `CompileSandbox`,
   the predicate sandbox, `withRecoveryRaw`, captures
   (`CapturedBinding.Value`), the debugger's 64-deep `Tape.Snapshot()`
   ring, `send`'s deep clone, `FnUtil.memoize`/`curry`, `const`'s
   exemplar. They rely on the copy being independent.
5. **Container element arrays.** ~110 `Slice()` calls, ~60 `make`+`copy`
   gathers, `CloneValue` (the `StructUtil.clone` word, `send`, the VM's fresh-push
   family `OpPushConstFresh`, `CloneValueKeeping` for compiled fn units).
   This is boru's value-semantics column: `set`, `push`, `pop`, `shift`,
   `unshift`, `merge`, `setpath`, `sort`, `reverse`, `take`, `unique`,
   `create`, `listAll` all return an updated copy
   (`native_storage.go:2078-2116`, `listops.go:49-122`, `merge.go:42-97`,
   `setpath.go:240-302`, `native_sort.go:48-63`).
6. **Pure performance or redundant.** Handler-side `make`+`copy` before
   `Run` while the engine copies again (`native_control.go:803/835/1631`,
   `native_definition.go:1054`, `native_process.go:592`, `test.go:304/357/
   500`, ...), read-only `Slice()` walks (~30), double copies
   (`SpliceExpand` over `Slice()`, `stepMove`'s body copy before
   `Splice`, the dead `MarkInfo.Body` copy in `stepMoveCont`), by-value
   boxing of `FnDefInfo` (232 bytes per `.(FnDefInfo)` assertion, 7
   sites) and `ForwardInfo` re-mints per collected arg. Deletable today.
7. **Already-pointer-held values.** `FnParam.Pattern *Value`,
   `FnSig.ReturnPatterns []*Value`, `CompiledFn.ParamPatterns`,
   `BranchRecord.ThenValue/ElsValue`, `Value.elem *Value`,
   `EmitState.foldedBodies map[*Value]bool`. They work because nothing
   writes them after construction: the existence proof for the model.

`CloneValue` has nine real callers outside its own file, and they sort
cleanly: the `StructUtil.clone` word (user-facing semantics,
`clone.go:19`; it copies mutable containers and returns scalars and other
immutable payloads shared, so `(StructUtil.clone 1) eq 1` is true); `send`
(isolation at the process boundary, `native_process.go:335`); four
check-mode reads that clone a container for a fresh provenance ID
(`native_storage.go:1449/1586/1595/1627`); one stale call in
`check_fnbody.go:646` whose comment promises a fresh ID that a Function
payload never gets (the immutable arm of `clone.go` returns it unchanged);
and the VM's fresh-push family (`vm.go:2300/4634/4769`), which deep-clones
a pooled literal per evaluation to match the interpreter's
one-instance-per-evaluation rule. Two more facts from the value side shape
the design: `Value` is not comparable and is never compared with `==`, and
container identity for `eq` is already the payload pointer (backing array,
`*OrderedMap`, `*fnIdent`, node ID), never the header. The header is
already treated as a disposable view; the policy makes that official.

One structural fact underlies all of it (`inventory/eng-check-compiler.md`,
findings): `ListPayload{Elems []Value}` is boxed in the `Data`
interface, so a by-value copy of a list Value already *aliases* its
element array. "By value" is one level deep today; `CloneValue` is what
gives independence, and `def b a` stores a struct copy whose slice header
shares `a`'s backing array (nothing writes into it today, which is the
only reason it is unobservable).

## 4. What in-situ modification breaks

Each class names what shares the original and the observable change.
Line numbers are in the inventories.

- **H1. Occurrence state leaks into the shared original.** `f/v apply`
  leaves the binding's `FnDefInfo.Applied` set; a dot-read fn stays
  `ReachGroup`-tagged inside its map (NUR035/NUR038 say the tag must
  never ride into a binding or container); `quote x` quotes `x`'s binding
  for every later read (`natives.go:515-526`); a list argument becomes
  `Quoted` in the caller's binding and the capture walk then skips it
  (`core_helpers.go:496-500`); every read of `x` reports the def site's
  position; `def n 5  def p:Pos n  n typeof` answers `Pos`
  (`native_definition.go:1024/1540/1596/1660`); `const n` re-tags every
  element sharing the instance (`native_const.go:76/94`); `v as T` leaks
  the match-time ascription into handlers and containers
  (`native_type.go:753-755`); `dup`, `over`, `pick` cells stop being
  independent (`native_stack.go:141-199`). Names ride on payload copies
  too: `installDef`/`installFnDef` rename the fn payload copy under the
  binding name (`core_helpers.go:98, 760-763`), so in place `def a (f/v)`
  renames `f` and `def Foo c` renames the class bound to `c`; a parameter
  bind sets `Quoted` on its copy (`core_helpers.go:500, 561`) and a macro
  operand needs the same form quoted and unquoted at once
  (`macro_expand.go:170-173`).
- **H2. Program tokens rewritten by execution.** `[1 add 2]` evaluated
  once becomes `[3]` for every later run; `/q` collection turns a Word
  token into an Atom (`collect_kernel.go:896-899`); `forceStackWord`
  forces a body's word to `/s` for the next call (`engine.go:4600-4604`);
  `Eval=false` flips (`engine.go:3826/7459`). In the VM the constant pool
  is the program: `OpPushConst` pushes `p.Consts[i]` by value
  (`vm.go:4760`) and everything downstream strips, quotes, renames,
  stamps or reparents the copy (`vm.go:4967, 5549, 5612, 2549, 2902,
  4800, 5355`); `constPoolKey` dedups scalars so one entry serves every
  same-canon push site (`emit.go:16021-16065`), and the lowerer makes
  islands sub-slices of one backing array shared by every run and every
  `ForkConcurrent` (`lower.go:4106-4111`). The check pass runs over the
  same parsed tokens, possibly twice (`boru.go:480-520` specialisation
  retry; `SetRootBody` retains them for `/q` resumption).
- **H3. Canonical type nodes written through literals.** `NewTypeLiteral
  (t)` is `*t` (`value.go:2803`); `DefTable.Top` of a type binding, `OpPushType`,
  disjunct alternatives and carrier resolution all hand out node copies,
  then stamp `pos`/`Quoted`/`asc`/`ID` on them. One object per node would
  stamp the lattice node every registry, `behave` and dispatch shares.
  The orphan `&v` problem that `CanonicalType` exists for disappears, and
  flips into this aliasing problem. The value side counts 27
  `NewTypeLiteral` call sites, 24 `&v` orphan sites and 12 `CanonicalType`
  repairs; `typeof`, `pathof`, the None/Absent fill literals and sig
  patterns all hand node copies to user data. On the other side of the
  fix, `Type.Equal` (`types.go:322-350`) becomes pointer equality
  everywhere: today it settles most comparisons by pointer, by the
  shared `tmeta` or by the interval label, and falls back to comparing
  ID strings only for unlabelled nodes (minted, refined and orphan
  copies). The ~7% of interpreter CPU its comment cites is the cost
  before those fast paths, not a remaining one; what the fallback costs
  now is unmeasured. `CommonAncestorType`'s pointer-keyed `seen` set also
  stops over-widening on orphans (`carrier_join.go:28-42, 193-195`).
- **H4. Recorder identities.** Check-mode def reads are tagged on a copy
  and mutated through a pointer (`engine.go:2952, 3248` →
  `check_recovery.go:82-96`, `emit.go:7463-7560`, `kept_defs.go:103-118`
  where the read is replaced by a carrier, `kept_defs.go:390-395`);
  `toCarrier` strips `Data` on a copy and keeps the ID so `origByID` can
  recover the literal (`carrier.go:307-452`); residuals, member reads and
  folded reads need one payload under two recorder identities
  (`check_fnbody.go:864-871`, `check_fnmodel.go:142-143`,
  `compiler_dispatch_record.go:356`). In place, a seated live read turns
  the concrete binding into a carrier for the rest of the program and
  distinct productions merge.
- **H5. Snapshot semantics.** Forks, sandboxes and rollbacks assume the
  check pass may scribble on copies while the interpreter later runs on
  pristine bindings (`CompileSandbox` 54-57/95-112, `DefTable.Clone`,
  `util.go:391-407`); the debugger's `back` renders from retained
  snapshots ("the snapshot is the truth, the registry is NOT rewound",
  `debugger.go:66/160/440`); `send` deep-clones so a receiver never sees
  the sender mutate (`native_process.go:335`); `append f f` is defined
  only because `Slice()` snapshots before the grow
  (`native_flex.go:318-323`).
- **H6. Concurrency.** `ForkConcurrent` isolates forks by cloning the
  DefTable (`fork.go:39-40`); the leaf frame skeleton is shared by every
  call and fork and copied per call precisely because `stampResultPos`
  mutates it ("The per-call COPY is mandatory", `core_helpers.go:465-476`);
  the compiled-dispatch cache is shared by every copy of a fn value.
  Shared mutable Values turn these into `-race` failures.
- **H7. The language.** Lists and maps have value semantics
  (`def a [1 2 3]  def b a  b set 0 9  a` is `[1 2 3]`); flex lists, flex
  maps and class instances mutate in place by design; stores are
  copy-on-write. `d2AdoptTyped` is the one deliberate in-place re-tag,
  special-cased because a copy broke visibility through a flex handle
  (`native_storage.go:560-590`). The policy generalises that special case
  to every list and map word unless "construct a new container, share
  the elements" is allowed.

## 5. The design the plan converges on

Separate the three things that live in one struct today:

| today in `Value` | where it goes | why |
|---|---|---|
| `Parent`, `Data`, identity | the **value**: `*Value`, immutable once published, shared freely | the thing the policy is about |
| `pos`, `Quoted`, `Eval`, `ReachGroup`, `Undefined`, `FailedDispatch`, `asc`, `Dynamic`/`dynFrom`, `Carrier`, fn names and return anchors, `Applied`/`SigMatched` | the **occurrence**: a slot record beside the pointer in the tape cell, stack cell, arg slot, binding entry, capture entry, pool entry | every H1/H2/H4 site is a write of occurrence state through a shared value |
| `ListPayload.Elems`, `OrderedMap`, `FlexListData`, stores | the **contents**: constructed per evaluation, elements shared by pointer | value semantics stays, element copies stop |

Consequences that fall out:

- A tape cell becomes `{V *Value; occurrence}`; evaluation *replaces the
  pointer* and never writes through it. Token-run copies (family 3) turn
  into pointer copies, and the interpreter can no longer write into a
  body it did not construct (H2 closes by construction).
- A type literal is a reference value (node pointer + occurrence), so
  `NewTypeLiteral` stops copying the node and `CanonicalType` retires
  (H3 closes; the duality `type Type = Value` can stay or be split, see
  §7).
- Dispatch ascription (`as T`) and quote-ness become properties of the
  operand slot, which is what "match-time-only, stripped at every
  delivery boundary" already says they are; the 19 `StripAscribed`
  calls in the VM and the strips in `execMatch`/fn-param binding go away.
- Recorder identity is keyed by (event, result index) or minted by
  construction, not by rewriting `Value.ID` on a copy (H4).
- Snapshots (H5) are either immutable shared objects (nothing to
  snapshot) or an explicit, named copy-on-write at the mutation site: the
  debugger ring, `send`, `StructUtil.clone`, `memoize` stay as deliberate
  constructions of equal values, which the policy should name as its
  sanctioned exceptions.
- Every "copy then retag" helper becomes a constructor that mints from
  parts: one family (`MintRetagged`, `MintCarrierFrom`) replaces
  `ReparentValue`, `FreshenDefault`, `JoinCarriersInner`'s `out := a`,
  `ReturnsIdentity`, `RetagFlexElem` and `cloner.withPayload`; binding
  names, flex param tags and macro-operand quoting move onto `DefEntry`;
  check-mode shadows and joins get a provenance key that is not the
  Value's ID string.
- `tmeta` already follows the policy one level down: one `*typeMeta` is
  shared by every copy of a type node and mutated in place by `SetName`,
  `SetBehavior`, `SetTypeBody`, `SetOwner`. The five copy-local facet
  setters (`SetPos`, `SetDynFrom`, `SetElemConstraint`, `SetAscribed`,
  `WithModuleNS`) are exactly the ones whose semantics change.
- The two copy checks are the enforcement: `go vet`'s copylocks for
  single-value copies and the bulk-copy analyzer for `copy` and variadic
  `append` over `[]Value`. Once a module reaches zero on both it stays
  there.

## 6. Refactor plan

Each phase ships on its own, behind `make commit-gate`, `make
cover-gate`, the langspec gates and the benchmark anchors (`BenchmarkStage6`,
`BenchmarkFnKinds`, `BenchmarkPerfWords`, `TestInterpAllocCeilings`,
`TestCompiledAllocCeilings`), with benchstat against the previous phase.

**Phase 0 — enforcement and anchors (2 days).** Add the zero-size
`noCopy` field to `Value` (it costs nothing at run time; the handover
folder's `patches/vet-nocopy-marker.patch` is that change, and `Value`
stays 104 bytes with it) and a `make vet-copies` target that counts
copylocks findings per module, for every module `go.work` lists, and
fails when a module's count rises above its recorded ceiling. Two more
pieces make that ratchet hold:

- **Take copylocks out of the ordinary vet lanes first.** With the marker
  in place, the plain `go vet ./...` that `make vet` and the commit
  gate's vet lane run (`scripts/commit-gate.sh`), and golangci-lint's
  `govet` under the `standard` preset (`.golangci.yml`), would all fail on
  the 32,141 baseline findings. Those lanes run with copylocks disabled
  (`go vet -copylocks=false`, and `govet` configured to match), and
  `make vet-copies` becomes the only place copylocks runs, against its
  ceilings.
- **Count the bulk copies too.** copylocks does not see `copy` or a
  variadic `append` over `[]Value`, which are exactly the tape and
  argument-array copies the policy targets. A small `go/analysis` pass
  that reports both when the element type is `core.Value` gives the
  ratchet a second count.

Record the benchmark baselines. This is the ratchet every later phase
pulls on.

**Phase 1 — pass by pointer on the hot paths (1 week, measured 11%,
estimated 15–20% with the rest).** Pointer receivers on 20 of the 23
accessor methods, as `patches/pointer-receivers.patch` in the handover
folder does (seven non-test call sites on a non-addressable value need a
local, and so do 105 sites in 30 test files, listed beside the patch;
`Pos`, `AscribedType` and `String` stay by value until their
method-expression and non-addressable uses are rewritten). `Tape.At` returning `*Value`
for immediate reads, with the rule that a cell pointer dies at the next
`MoveGap`/`grow`; `stepToken`, `stepLiteral`, `stepWord`,
`scanBoundaryToken`, `PlanMatch`, `resolvedIndicesBeforeInto`,
`CollectCandidateScan`, `EffectiveResolved` taking `*Value`; `IsAncestor`
reading `tmeta` directly. In the VM: `vmContext.run`, `tapeCoupled`,
`stampFnResultPos`, `invokeClosureOn`, the relational and numeric
handlers' by-value parameters. Handler `args []Value` stays. No
behaviour change; the differential langspec gates are the oracle.

**Phase 2 — delete the redundant copies (1 week, 2–5%, mostly
allocation).** Family 6: the handler-side pre-`Run` copies (the engine
copies again), the read-only `Slice()` walks (add an iterator or
`Elems()` view), the double copies in `SpliceExpand`, `stepMove` and
`stepMoveCont`, `FnDefInfo` as a pointer payload (`*FnDefInfo`), the
`ForwardInfo` re-mint, the double ID mints on fresh carriers, the
per-dispatch signature views (`installedSigView`, `fnDispatchView`:
normalise at construction). Lower the alloc ceilings as each lands.

**Phase 3 — occurrence state out of the Value (4–6 weeks, the design
step).** Introduce the slot record for tape cells, stack cells, arg
slots, binding entries (`DefEntry`), capture entries and the VM's locals
and pool entries. Move `pos` stamping, `Quoted`/`Eval`, `ReachGroup`,
`Undefined`, `FailedDispatch`, `asc`, `Dynamic`/`dynFrom`, fn names and
anchors (`nameFrameFns`, `nameClosureValue`, `stampFnPos`), `Applied`/
`SigMatched` into it; replace the ~70 copy-modify-writeback sites with
slot writes; re-key the recorder on (event, index); make `DefTable.Top`
return the binding and its slot rather than a copy; make `NewTypeLiteral`
a node reference. The header that remains is `Parent`, `Data`, `tmeta`,
`elem`, `modns` and the type-node flags: about 40 bytes, which cuts the
remaining copy traffic by ~2.6× on its own (measurements §4, the
32-byte row). Gate: the families 2 and 3 tables in the inventories go
empty; vet counts for `engine.go`, `vm.go`, `carrier.go`, `emit.go`,
`kept_defs.go`, `native_definition.go`, `native_type.go` drop to the
mechanical kind only. This phase is where the `-race` suite and the
debugger ring need the sanctioned-copy list from §5.

**Phase 4 — the representation (4–8 weeks, decide after phase 3).** Turn
the remaining by-value flows into `*Value`: tape and stack cells, `args
[]*Value` (870 handler signatures, mechanical), list elements
`[]*Value`, `map[string]*Value` (640), struct fields (~88), `Value{}`
sentinels to `nil` (808; ~95 are `ok=false` return conventions, twelve in
core and six in the VM and method-shape tables are semantic "unbound"
markers that need an explicit sentinel), and the ~17,000 test-side copies. Allocation
model: intern small integers, booleans, `none`, the empty string and
atoms as singletons (no mint for the common scalars); mint everything
else on the heap and measure; keep a per-run slab in reserve. The alloc
ceilings must be re-anchored to the new count of values minted per op.
Go/no-go on the phase-3 benchstat: the synthetic benchmark says
heap-per-mint is ~2.7× slower than by-value on the copy-dominated loop
and slab allocation ~1.7× slower, while a 40-byte by-value header is
within noise of an arena; if phase 3 lands near the arena line, the
pointer switch buys consistency (no copies to police) rather than speed,
and that is a legitimate reason to do it, but it should be chosen with
the numbers on the table.

**Phase 5 — the language decision (1–2 weeks of spec work, any time
after phase 3).** Either keep list and map value semantics by
construction (a word returns a new container whose elements are the
receiver's own) or move them to in-place mutation like flex containers (a
spec change touching every row that relies on the receiver staying put).
Value semantics without copying a `Value` needs the elements to be
references, which is phase 4, or a persistent container representation
that shares structure between versions. While the header stays a
by-value struct, building the new container copies each unchanged
element's header, a bulk copy the exceptions list sanctions until
then. `StructUtil.clone`, `send`, `memoize` and the
debugger ring are explicit constructions of equal values either way.

Order and dependencies: 0 → 1 → 2 can land in any interleaving; 3
depends on 1 (the slot type reuses the pointer plumbing); 4 depends on 3
(a pointer representation with occurrence state still inside the Value
reproduces every H1–H4 bug at once); 5 is independent but should not
land before 3, since phase 3 removes the element-array copies that make
the value-semantics column cheap.

## 7. Decisions needed from the maintainer

1. **Is constructing a new container that shares its elements a
   "copy"?** The plan assumes no. If yes, list and map value semantics
   cannot survive and phase 5 is forced to in-place. Sharing elements
   for real needs reference elements (phase 4) or a persistent container
   representation, so the answer here and decision 3 go together.
2. **Lists and maps: value semantics or in-place?** Today value
   semantics (`set` returns an updated copy); flex containers already
   mutate in place. Recommendation: keep value semantics by construction.
3. **Is phase 4 required, or is a ~40-byte immutable header passed by
   value an acceptable end state?** Recommendation: decide after phase
   3 with benchstat in hand.
4. **Sanctioned copies.** The full list, with sites, invariants and
   expiry phases, is [VALUE-NO-COPY-EXCEPTIONS.0.md](VALUE-NO-COPY-EXCEPTIONS.0.md): two
   language-level copies (`StructUtil.clone`, `send`), seven named constructions,
   seven reference-copy families that are not Value copies, ten
   transitional allowances tied to phases, and seven copies that are
   explicitly not exceptions. Approve or amend it.
5. **The type duality.** Keep `type Type = Value` with a node-reference
   literal, or split `Type` from `Value` (the legacy speed plan's option
   1). Phase 3 needs one of them chosen.

## 8. Performance estimate

| step | basis | expected change in time |
|---|---|---|
| copy and zero traffic today | profiles | 23–25% of CPU, both lanes |
| phase 1, accessor receivers only | measured in a scratch worktree | −11% geomean (interp −0 to −18%, compiled −8 to −23%) |
| phase 1 complete (step loop, matcher, VM run loop by pointer) | the remaining `duffcopy` callers are those functions | −15 to −20% cumulative |
| phase 2 | allocation counts in the inventories | −2 to −5% more |
| phase 3 (40-byte header, no writebacks, no `StripAscribed`, no fresh-push clones) | synthetic 104-byte vs 32-byte rows; VM clone sites | −20 to −30% cumulative from today |
| phase 4 with heap-allocated values, no interning | synthetic heap row | +10 to +40% (slower) relative to phase 3 |
| phase 4 with small-scalar interning and a slab | synthetic slab row, interned mints free | −5 to +10% relative to phase 3 |

Risks that move the estimate: GC mark time (already 9–20% cumulative)
rises with object count under phase 4; the alloc ceilings and the
30-second per-variant `TestVariationDifferential` budget are sensitive
to allocation; `-race` runs get slower with more pointer traffic. The
upsides not in the table: phase 3 removes a bug class (H1–H4) rather than
a cost, and the vet ratchet keeps it removed; type identity by pointer
retires `Type.Equal`'s remaining ID-string fallback (unmeasured since its
fast paths landed) and the `ResolveWordsDeep` prepass that rebuilds both
`Unify` operands on every call (`resolve.go:75-155`).
