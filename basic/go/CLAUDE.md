# basic/go — Base Language Layer CLAUDE.md

The `basic` module (`github.com/boru-lang/boru/basic/go`, package
`basic`) is the boru **base language layer**: the fundamental words and
the predefined global content types, registered against the kernel. It
sits below `lang`:

```
core/go  ←  basic/go  ←  lang/go  ←  cmd/go
parser/go  ↗
```

**The dependency rule is hard (ADR-013 as amended 2026-08-08): basic
depends on the pieces it actually uses, and on nothing else.** Today
that is `core` and `parser` (the latter test-only) — NOT `eng`, `check`
or `compiler`. No other boru sibling, no host-capability dependency
(sqlite, file ops, formats — those are lang's). The gate is
`depsgate_test.go`; if a change here seems to need more, the design is
wrong, not the go.mod.

### Why nothing above core belongs here

basic defines language **types and words**. Nothing in that job needs
special knowledge of the passes above the interpreter: core's own
primitives are enough, and a word that seems to need more is a word
whose analysis half is filed in the wrong module.

- **compiler: removed (2026-08-07).** basic's `if` / `case` / `for`
  handlers record branch and loop fragments for the bytecode emitter.
  That already went through a core-owned seam, `core.EmitRecorder` —
  except the interface was one method group short (branches and loops),
  so `recorderState` had to downcast to `*compiler.EmitState` to reach
  them. Widening the interface (`TakeFragment`, `RecordBranch`,
  `RecordLoop`, with `core.BranchRecord` and the opaque
  `core.EmitFragmentRef`) deleted the downcast, and the module
  requirement went with it. The seam is honest because an INACTIVE
  recorder is *correct* behaviour: with no compiler linked, nothing is
  recorded and the program still runs, interpreted.

- **check: removed (2026-08-08), by moving code, not by forwarding it.**
  Full reasoning, the moved-symbol table and the gate arithmetic:
  [design/legacy/BASIC-CHECK-CUT.0.ignore](../../design/legacy/BASIC-CHECK-CUT.0.ignore).
  The 2026-08-07 amendment kept check on the grounds that basic's 23
  check symbols were "written in the checker's vocabulary" and that
  routing them through a table would be a mailbox rather than a seam.
  The first half was wrong on inspection. **21 of the 23 were pure
  functions over CORE types** — `core.Value`, `*core.Type`, `r.Defs`,
  and `CheckState`, which core has owned since the four-piece cut. They
  were not the checker's vocabulary; they were the interpreter's,
  sitting in `check/go` for historical reasons. So they moved down, to
  where their types already live:

  | What | Now in |
  |---|---|
  | `JoinCarriers`, `JoinCarriersInner`, `JoinCarrierStacks`, `InstallJoinedDefs`, `CommonAncestorType`, `FlattenAlternatives` | `core/go/carrier_join.go` |
  | `RunCarrierBody`, `…KeepDefs`, `…WithDefs`, `RunCarrierCondBody` | `core/go/carrier_body.go` |
  | `ApplyGuardNarrowing`, `ApplyComplementNarrowing`, `LiteralCondValue`, `BoolWord`, `GuardClause` | `core/go/guard_narrow.go` (+ `guard_predicate.go`) |
  | `NewCarrierTypedList(Value)`, `NewDynamicCarrierValue`, `UnionCarrierForType`, `ReturnsIdentity` | `core/go/carrier_new.go` |
  | `FoldVariadicArms`, `SpreadPayload` | `core/go/carrier_spread.go` |
  | `DeadSignatures`, `DeadSig` | `core/go/deadsig.go` |
  | `RecordTypedDefMake` | `core/go/record_typed_def.go` |
  | `CheckAddUniqueDiagnostic`, `CheckAddUnique` | `core/go/check_state.go` |

  Two knock-on retirements: `core.JoinCarriersHook` and the
  `AnalysisImpl.AddUnique` slot both existed only because their
  subjects lived above core. With the subjects core-resident there was
  nothing left to indirect, so both slots went.

  **The genuine remainder is two symbols**, `AnalyseFnBody` and
  `AnalyseLoopBody` — the analysis *pass* itself: memoised per call
  shape, recursion-bailing, quota-capped, Kleene-iterated to a fixed
  point. They stay in check and are reached through S1 slots as
  `core.RunFnBodyAnalysis` / `core.RunLoopBodyAnalysis`. That is a seam
  and not a mailbox, on the argument the earlier amendment did not have:
  **with no check linked there is no analysis pass at all.**
  `AnalysisImpl.ReturnsFn` returns nil, so no `ReturnsFunc` ever runs
  and neither accessor is reached — the nil defaults are the same
  no-analysis regime every other S1 slot already defines, not a
  different, wrong one. And nil is in-band regardless: `AnalyseFnBody`
  documents an empty result as "the analyser aborted — treat as an Any
  carrier", which every caller here already handles.

If a word you are adding here seems to need something above core, the
question to answer first is which of those two cases it is: a primitive
filed in the wrong module (move it down), or a driver of the pass
itself (a slot). It is not a reason to add a dependency edge.

## What lives here

- **Fundamental words** (each file exposes registration slices that
  lang's `Register` applies at its historical points, and
  `register.go` here composes for kernel-only embedders):
  - `native_stack.go` — the forth-style stack vocabulary (`dup`,
    `swap`, `drop`, `over`, `rot`, `nip`, `tuck`, `dup2`, `swap2`,
    `drop2`, `over2`, `depth`, `pick`, `roll`).
  - `native_definition*.go` — `def`, `undef`, `var`, `fn`, `afn`,
    `fnsig`, `args`, `__pa`, and the synthesized def keyword forms.
  - `native_control.go` + `conditional.go` + `case_exhaustive.go` +
    `forloop.go` — `do`, `if`, `case`, `for`, `break`, `continue`,
    `error`, and the case-exhaustiveness checker.
  - `native_type_gen.go` — the type-generics words (`gen`,
    `extends`, `default`, `of`), coupled to the def keyword forms.
  - `native_const.go` — `const`.
- **Predefined content types** with their type-local logic (the
  ADR-012 rule 1 middle home):
  - `native_temporal.go` — the `Scalar/Time` family (FixedIDs
    1000-1003): registrations, Behaviors/Comparers, constructors,
    and the `boru:time-util` module mints.
  - `micron.go` + `micron_grammar.go` + `iso4217.go` — the
    `Scalar/Micron` structured-scalar family's CONTENT: the twelve
    leaf validators/constructors, the merged literal grammar
    (`MicronFromString`), the family Behavior/Comparer, the
    property tables, the currency table, and the `-on` naming
    rule. The identities stay kernel-declared (core's `builtinDecls`,
    the Resource/Entity precedent); the content plugs in through
    core's capabilities — `InstallMicronIdeals` per registry (called
    by `Register` here, lang's `Register`, module sub-registries,
    and the engspec fixture set), the `SubtypeNamer` /
    `MicronSubtypeMinter` Behavior capabilities, and the render
    bridge.
  - `types_bytes.go` — `Scalar/Bytes` (1009): registration,
    Behavior (render/equality/ordering/size/const-bake), the Go
    bridge wiring, and the constructor/unwrapper pair. The bytes
    WORDS and the binary-frame machinery stay in lang.
  - `types_handles.go` — `Ideal/Patrun` (5004), `Ideal/Pid`
    (5007), `Ideal/Service` (5008): identity + Behavior shells.
    The patrun matcher and service state stay in lang with their
    words and implement the `PatrunFormatter`/`ServiceFormatter`
    delegation interfaces — never an upward import.
  - `types_timer.go` — the Timeout/Interval handle constructors and
    Behaviors (module-minted types, former FixedIDs retired).
  - `types_resource.go` — the `Resource`/`Entity` definitions.
  - `typeinit.go` — the init-time registration-error accumulator
    (ADR-005: record, never panic; surfaced at registry
    construction).

## Conventions

- `aliases.go` re-exports core exactly like
  `lang/go/native/aliases.go` re-exports its own dependencies — word
  files here stay module-agnostic and name everything unqualified. Everything in
  `lang/go/CLAUDE.md` (argument ordering, registry bindings, helper
  API discipline, panic prevention, negative-test pairing) applies to
  this package unchanged; read that guide before changing word
  handlers here.
- Identifiers that lang's word files or test suites still reference
  are exported and re-imported through `lang/go/native/basic_shim.go`.
  When you export a new one, add the shim alias in the same change.
- Word registration order matters for overload appends. lang applies
  this module's slices at its historical points in
  `lang/go/native/register.go`; keep `Register` here consistent with
  that relative order.
- FixedIDs are wire-stable — never renumber
  (`lang/go/test/fixedid_stability_test.go` is the gate).
- The coverage gate (ADR-008) measures this module through the merged
  cross-suite profile: lang's suites legitimately cover statements
  here, exactly as they cover core's.
