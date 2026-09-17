# eng/go — Kernel CLAUDE.md

The `eng` module is the boru kernel: types, values, signatures,
matching, the step loop, and the parser bridge. This file
documents conventions specific to this module — anything that's
language-wide rather than lang-specific lives here.

For language-layer conventions (jsonic integration, registry
stacks, helper API discipline, panic prevention) see
`lang/go/CLAUDE.md`.

The kernel is now FOUR modules on a hard chain: `core/go` (the
interpreter core) → `check/go` (the type checker / analysis pass)
→ `compiler/go` (the recorder, lowering, the bytecode emitter) →
`eng/go` (the bytecode VM, the parser bridge, and the generated
facades over the other three). The check-mode and compile/emit
machinery documented below therefore LIVES in `check/go` and
`compiler/go` (design/legacy/ENG-FOUR-PIECE.0.ignore), and each has its own
module guide; this file stays the single home of the shared kernel
conventions, which apply to all four modules verbatim.

## Single-Pass Parsing (CRITICAL)

boru source is converted from jsonic items to engine values in
one left-to-right walk. Do not introduce post-conversion rewrite
passes that re-walk the value stream — they accumulate complexity
(paren-expansion ordering, data-context vs word-context divergence,
nested-container recursion) and entangle the parser with handler
semantics. When a syntax form needs sugar, prefer a token-level
substitution (an `AltSpec.A` callback that emits a `jsonic.Text`
marker — see `;` → `"end"`, `=>` → `"afn"`, `|` → `"|"` in
`parser/grammar.go::setupValRule`) so the conversion path stays
linear. Behaviour the marker can't express directly belongs in the
registered word's handler, not in a separate parser stage.

## Check-mode guaranteed-error mirrors (classification + gates)

A check diagnostic that mirrors a GUARANTEED runtime error (a strict-
accessor static miss, a provable index OOB, an unconditional raise, a
dry-passed pure handler's failure — design/legacy/CHECKER-COMPLETION.0.ignore) is
classified and gated rather than sprinkled with ad-hoc suppressions:

- **`CheckDiagnostic.RuntimeMirror`** — stamped by every mirror emitter
  (`CheckAddUniqueDiagnostic` does it for all its callers; direct
  emitters set it explicitly). The compile pipeline's error-diagnostic
  refusal (`CompileCheck` / `Vm.compile`) SKIPS mirrors: the finding's
  model is exact — the program compiles and raises the identical error
  (trap / VM RET / the same handler) — so only MODEL-UNDERMINING errors
  (undefined_word, no_signature: dispatch didn't resolve) refuse.
  Check and compile passes therefore report the SAME diagnostics.
- **`CheckDiagnostic.CaughtAtRuntime`** — stamped CENTRALLY by
  `AddDiagnostic`: inside an error-TRAPPING region (`do [...]` raises
  `CheckState.CaughtBodyDepth`) every would-be-error finding is
  downgraded to info, uniformly across families. The information
  survives; the false error verdict does not. Dedupe helpers skip
  caught entries so they never mask a real emission elsewhere.
- **Reachability** — a mirror that claims "the program errors" must be
  unconditionally reached: `CheckState.FnBodyDepth` (fn bodies run only
  if called) and `CheckState.NestedBodyDepth` (raised by
  `RunCarrierBodyWithDefs` — every branch / loop / quotation body is
  conditionally reached). `CheckAtUncaughtTopLevel(r)` (drypass.go)
  composes them.

For PURE handlers over concrete args, wire `DryPassReturns` /
`DryPassWrap` instead of a bespoke ReturnsFn. Compile passes arm
themselves through ONE helper — `CheckState.BeginCompilePass()` (fresh
EmitState, the Compiling flag, fn-memo drop); never hand-roll the
ritual, that is how Vm.compile shipped without the Compiling flag.

## Per-Call Stacks (Args / DefSnapshot / FnBaselines)

Every fn-body entry pushes onto three coordinated stacks; every fn
exit pops the same three. Misalignment (forgetting one) breaks
nested calls in subtle ways, so add the push and the matching pop
together when you touch one of these sites.

| Stack | Field | Push site | Pop site | Read site |
| --- | --- | --- | --- | --- |
| Args list | `Registry.Args` | `InstallFnDef` handler / `execFnDefSig` body splice / `CallBoru` | `__pa` token in the synthesized body tail / `CallBoru` inline cleanup | `args` native word |
| Body-local def cleanup | (per-call `defSnapshot` local) | same | `DefCleanupInfo` marker (`stepDefCleanup`) / `CallBoru` inline cleanup | closure capture analysis (via `FnBaselines`) |
| Enclosing-fn baseline | `Registry.FnBaselines` | same (alongside `defSnapshot`) | piggybacks on `__pa` and on `CallBoru`'s inline cleanup | `ComputeCaptures` (`fn_capture.go`) |

A `break`/`continue` escaping a live spliced frame discards the
frame's tail before it executes; the flow resolvers therefore run
`unwindLiveFrames` (`fn_frame.go`) over the discarded region, which
replays each open frame's canonical tail (`__DC`, `__pa`, the
force-forward `undef` pairs, innermost-first) so all three stacks
stay balanced. Without it the dead callee's args list shadows the
caller's `args` for the rest of the loop.

The baseline is what closure-capture detection consults at fn-
construction time: an inner-fn body Word whose `Defs.Depth(name) >
TopFnBaseline()[name]` lives inside an enclosing fn (param or
body-local) and is captured; depth ≤ baseline means module / global
scope and the reference stays dynamic. See lang/go/CLAUDE.md
"Closures and Capture" for the language-level semantics.

## Signature Ordering (CRITICAL)

There is exactly **one** argument-positioning convention in this
kernel, and it holds at every arity:

> A signature binds its args **in sig order**. Matching fills
> positions from the **forward stack** — the tokens after the
> word, in written order — up to that signature's barrier, then
> fills every position still empty from the **value stack** in
> reverse: the next position takes the top of the stack, the one
> after it the next-deeper value, and so on.

Two-arg words are **not** a special case; there is no "swap
form". The single source of truth is `MatchSignature`
(`core/go/engine.go`). Args flow through every dispatch path in
this order with **no reordering at any handoff** — never swap,
reverse, or re-permute args between MatchSignature and the
handler.

Two consequences worth naming, because they surprise readers:
all args on the value stack yields **Forth order** (top is
sig[0]); all args written forward yields **written order**, which
for a non-commutative handler reads backwards — `sub 1 3` is `2`,
since `sub` computes `args[1] - args[0]`.

Concretely:

- `Signature.Args[0]` (native kernel words): top of stack.
- `FnSig.Params[0]` (boru `def fn […]` definitions, module FnDef
  wrappers): also top of stack. Empirical: boru source `def f fn
  [[a:Integer b:String]…]` called as `f 1 "x"` (forward form)
  binds `a=1` (sig[0], first forward arg) and `b="x"` (sig[1]).
  Stack form `"x" 1 f` is mirror-equivalent: top `1` → a, deeper
  `"x"` → b.
- `args[i]` in every handler — registered native, InstallFnDef
  closure, CallBoru, execFnDefSig — is the i-th sig position from
  the top.
- `args.N` boru accessor: returns `args[N]` directly.
- Body-token push (unnamed params via CallBoru / InstallFnDef /
  execFnDefSig): appends `args[0]…args[N-1]` in order, no
  reversal. Body frame contains `args[0]` at the bottom and
  `args[N-1]` on top. **Arguments enter the frame RESOLVED** — the
  step region starts after them (`FrameOpenInfo.ArgSpan` on spliced
  frames, `Engine.startAt` on sub-engine runs), so an argument with
  active step semantics (a Function value, an `__SP` marker) is
  inert data exactly like a named binding; it acts only where the
  body uses it. See design/legacy/ARG-SEMANTICS-UNIFICATION.0.ignore.

There is **no exception path**. Anything that looks like a
"reordering" elsewhere is either:

- `matchSignature`'s top-down stack walk
  (`stackVal := resolved[len(resolved)-1-j]`), which is how the
  one convention is implemented (not a reordering of args, but
  the rule itself).
- `rearrangeForForward` (engine.go:1124), which fixes the stack
  layout AFTER forward collection so that matchSignature's
  top-down walk produces the correct sig assignment. Part of
  matchSignature's machinery; not a separate path.
- A handler that, by convention, computes `args[1] OP args[0]`
  (notably non-commutative math like `sub`). That's how the
  handler INTERPRETS its received args for natural source
  reading — args are still delivered in sig order.

Forward-argument gathering itself is TWO phases — a plan-time token
walk (`resolveForwardArgs` + `matchSignature`) and a run-time arrival
loop for parked Forwards (`stepLiteral`'s collection block, committed
early at statement boundaries by `commitBarrierForward`). They enforce
coordinated stop conditions that can drift apart — read
`design/FORWARD-COLLECTION-PHASES.10.md` before touching either phase.

### Surface form recommendation

Every surface form is the SAME ONE split rule — collection moves
forward until the barrier, then continues on the value stack in
reverse. Forward args fill sig positions 0..k-1 in written order;
the remaining positions fill from the stack prefix, top-down.
There is **no "swap form"** and **no two-arg special case**: both
phrasings are legacy misunderstandings and must not appear in
code, comments, or docs.

Where the split falls is the only thing a call form chooses. For
sig order (a, b, c) with barrier 3, all of
`f a b c ≡ c f a b ≡ c b f a ≡ c b a f` are the same call. At two
args the same holds: `a f b` is just k=1 (b → sig[0], a → sig[1]),
so `a f b ≡ f b a ≡ a b f` (e.g. `10 sub 3 ≡ sub 3 10 ≡
10 3 sub = 7`). What trips readers is that `a f b` and `f a b` are
DIFFERENT assignments — different splits of the same operands —
so `sub 10 3 ≡ 3 sub 10 ≡ 3 10 sub = -7`. Nothing about that is
specific to arity 2; it is the rule reading operands where
they sit.

Two surface conventions follow (STYLE-GUIDE.md §S2):

- **Infix form for words read as infix operators** — `add`, `sub`,
  `mul`, `div`, `mod`, `pow`, `and`, `or`, `lt`, `lte`, `gt`,
  `gte`, `eq`, `neq`. Write `1 add 2`, `10 sub 3`, `n lte 1`: one
  operand on the stack, one forward. Binary handlers compute
  `args[1] OP args[0]` precisely so this split reads as written.
- **Forward form `f a b c` for everything else** — declared param
  order matches written argument order (sig[0]=a, sig[1]=b, …).
  Reach for a stack-side split only where a pipeline has already
  left the operands on the stack.

### Trivial-delegation wrapper short-circuit

Module FnDef wrappers built via `makeXxxFnDef` / `wrapXxxFnDef`
helpers have a body of `[Word(inner-native-name)]` — pure
delegation to an inner native of the same name. `execFnDefLiteral`
detects this shape via `isTrivialDelegationBody` and short-circuits
to direct `execMatch` dispatch on the inner native's matched sig,
bypassing CallBoru and body-token execution entirely. Args flow
straight through; no reordering, no body splicing, no sub-engine.
boru fns defined in module preambles (named params, real bodies —
e.g. `decision.cond`) take the longer CallBoru path, where their
named params bind to args[i] in i-order without any push reordering.

When adding a new dispatch path (a new way to invoke an
`FnDefInfo` or `FnSig`), match `matchSignature`'s convention and
route through the existing helpers rather than reimplementing
arg-position logic.

## No Zero-Value Overload (CRITICAL)

A struct field MUST NOT use the Go zero value (`0`, `""`, `false`,
`nil`) to mean both "the author omitted this" AND a valid
explicit value. The two meanings are indistinguishable at the
call site and inevitably drift apart when one path needs the
"omitted = default" rule and another path needs the "explicit
zero" interpretation. We hit this exactly with `BarrierPos`,
where `[| a b]` from boru source could not be expressed because
`0` was being silently promoted to `len(Args)` for omitted-field
callers.

When a field needs an "unset" state distinct from a real value:

- **For ints**: use an out-of-domain sentinel and document it.
  `-1` is the canonical choice when the field's valid domain is
  non-negative (`BarrierPos`, `CheckState.StepBudget`,
  `WordInfo.ArgCount`). Initialize the sentinel in the
  constructor (e.g. `NewRegistry`) so the Go zero never reaches
  consumers.
- **For pointers / interfaces**: `nil` IS unambiguously "unset"
  when the type has no zero-value inhabitant — fine to use.
- **For booleans**: don't try to overload. Add a second field
  (`PathonInfo.Abs` plus an `AbsSet bool` if both states matter)
  or split into an enum.
- **For strings**: empty string is rarely a valid user value,
  but if it could be, use a `*string` or a separate `XSet bool`.

Resolve the sentinel **exactly once, at a single boundary** —
the registration / construction point — so every downstream
consumer sees a fully-normalised value. Don't sprinkle the
default substitution across every reader; that's how the
ambiguity sneaks back in. Current examples:

- `upsertFnDef` (`registry.go`) — sentinel-resolves
  `Signature.BarrierPos == -1` to `len(Args)` or `0` based on
  the `forwardArgs` flag, then stores. Every read of
  `BarrierPos` after this is an explicit value.
- `NewRegistry` (`registry.go`) — initialises
  `CheckState.StepBudget = -1` so the Go zero on a freshly-
  constructed registry doesn't get mistaken for "abort
  immediately." The engine's check-mode loop substitutes
  `DefaultCheckStepBudget` only for the sentinel; an explicit
  user-set `0` is honored as "abort on first step."

When you find a field that violates this rule, treat the audit
as the BarrierPos cleanup precedent: pick a sentinel, audit all
construction sites (an AST-based rewrite is fine for bulk),
move resolution to a single boundary, remove every conditional
that substituted on the zero value.

## Payload-presence vs value-mode (CRITICAL)

Do NOT write `v.Data == nil` in consumer code. The raw probe is an
overloaded smell: across the kernel it has meant three different
things — "v is a bare type literal", "v is not a concrete value", and
"there is no payload to read" — and those intents genuinely **diverge**:

- A list/map **carrier** (`NewCarrier(TList/TMap)`) carries a
  `ChildTypeInfo` payload, so `v.Data != nil`, yet it is NOT concrete.
  That non-nil payload is load-bearing: it's how a carrier satisfies
  `positionalMatch`'s concrete-list/map rule (`signature.go`).
- The `None` **type literal** has `Data == nil` like every other
  literal, but dispatch treats `None` as a *value* (handlers return
  `NewTypeLiteral(TNone)` for "no value found").

So `Data == nil` is neither the negation of `IsConcrete` nor the same
as `IsTypeLiteral`. State the intent with a named predicate (all in
`util.go`, re-exported via `native/aliases.go`):

| Intent | Predicate |
| --- | --- |
| v is its own lattice node (naming, ordering, identity dispatch) | `IsBareTypeNode(v)` — exactly `Data == nil && !Carrier`; INCLUDES None/Any/Never |
| v may be used as a type **constraint** (None excluded) | `IsTypeLiteral(v)` |
| recover a type's declared structure | `core.TypeContentOf(v)` — a bare node's `TypeBody` stamp, or v itself when it already IS type content |
| v carries a real, readable payload | `IsConcrete(v)` |
| handler needs a concrete list/map | `RequireConcreteList/Map(v, op)` |

Since the Stage 2 flip every NAMED type evaluates to a bare node, so
node-ness alone is NOT a constraint key: a kind that enforces
membership through a Unifier (DepScalar, predicate, disjunct,
negation, FnUndef, binding-body) must be routed by
`core.HasConstraintUnify`, or the constraint is never run
(design/legacy/TYPE-REPRESENTATION.1.ignore §N3 — the typed-def reparent arm's
gate is the model).

The regression gate `data_nil_gate_test.go::TestNoRawDataNilProbes`
forbids `.Data == nil` outside a small allowlist of files that
legitimately test payload presence at the lowest level (the
value/predicate/classifier home, the rendering and comparison
primitives, and carrier construction). Add a predicate, not an
allowlist entry. `value_mode_test.go` pins the predicate table across
every value mode.

## Sealed Payload (CRITICAL)

`Value.Data` is a sealed interface: `eng.Payload`. Only types
with the unexported `payloadMarker()` method satisfy it, and the
method is only definable in this package. The interface also
requires `IsTypeContent(owner *Value) bool` — the ONE
type-recognition seam (design/legacy/TYPE-REPRESENTATION.1.ignore §N4):
`IsTypeBody` asks the payload instead of enumerating shapes. The seal closes the
historical `Data interface{}` hole — `Value{Parent: TInteger,
Data: "hello"}` is a **compile error**.

Payload variants live in `eng/go/payload.go`. Two flavours:

1. **Wrapper variants** — for Go built-in types we can't add
   methods to: `IntPayload{N: int64}`, `StrPayload{S: string}`,
   `BoolPayload{B: bool}`, `FloatPayload{F: float64}`,
   `AtomPayload{Name: string}`, `PathonPayload{Info: PathonInfo}`,
   `MicronPayload{Fields *OrderedMap}`,
   `ListPayload{Elems: []Value}`, `MapPayload{M: *OrderedMap}`,
   `ParenExprPayload{Toks: []Value}`,
   `InterpStringPayload{Parts: []InterpPart}`,
   `TimePayload{T: any}`, `DurationPayload{D: any}`,
   `TimezonePayload{Loc: any}`, `MaterializerPayload{M: Materializer}`,
   `NonePayload{}`, `ExtensionPayload{Body: any}`.

2. **Direct variants** — eng-defined struct/pointer types with
   `payloadMarker()` added in payload.go: `WordInfo`,
   `ForwardInfo`, `MarkInfo`, `MoveInfo`, `ReturnCheckInfo`,
   `DefCleanupInfo`, `FrameOpenInfo`, `ModuleDesc`, `FnDefInfo`, `FnUndefInfo`,
   `DisjunctInfo`, `ChildTypeInfo`, `CodeEffectInfo`, `RecordTypeInfo`,
   `OptionsTypeInfo`, `TableTypeInfo`, `TableData`,
   `ClassTypeInfo`, `ClassInstanceInfo`, `*StoreInstanceInfo`,
   `*StoreShapeInfo`,
   `ResourceTypeInfo`, `ResourceInstanceInfo`,
   `*TimeoutInfo`, `*IntervalInfo`,
   `ErrorInfo`, `CalDurationData`, `DepScalarInfo`,
   `PathonInfo`, `MicronTypeInfo`, `noneSentinel`.

When adding a new kernel-known payload shape, register the
marker in `payload.go` (either as a wrapper struct or with a
`func (Foo) payloadMarker() {}` line) AND add its `IsTypeContent`
arm in `payload_typecontent.go` — constant for type-only payloads,
value-state-derived for the shared variants (`MapPayload` reads the
Implicit flag, `ExtensionPayload` its nested host-type-body marker).
Without both the compiler refuses to put it in a Value.

For plugin/host-supplied payloads, use `ExtensionPayload` — its
`Body any` is the explicit escape hatch the kernel does NOT
inspect. Two accommodations, both identity-only and neither
reading a field (from the retired NUR031's equality work):
`moduleDescIdentity` asserts `Body` to the kernel-owned
`*ModuleDesc` for POINTER IDENTITY — the Module descriptor is an
identity-equal opaque handle — and `hostPayloadIdentity` compares
two `Body` boxes with `==` so a sealed host payload is at least
`eq`/`deq` to ITSELF (an uncomparable body answers false rather
than panicking). Both live in `core/go/compare.go`; `handleKind`
in `compare_deqkey.go` mirrors the descriptor half for the
bucketed scans. A host type wanting a structural `deq` installs
the `DeepEqualer` capability, which is consulted first.

## Type Behavior

Every `*Type` carries a `Behavior` field of type
`TypeBehavior`:

```go
type TypeBehavior interface {
    Match(v Value, t *Type) bool
    Format(v Value) string
    Equal(a, b Value) bool
}
```

`DefaultBehavior` is the kernel's no-op: `Match` delegates to
`v.Parent.ConformsTo(t)`, `Format` delegates to `v.String()` (with
the dispatch carefully avoiding re-entry), `Equal` delegates to
`valuesEqualDefault`. Every type registered through the kernel
paths gets `DefaultBehavior` if the caller doesn't supply one.

Types with semantics the kernel can't infer (time formatting,
matrix rendering, predicate-type matching, refinement-type
matching, plugin types) supply a custom Behavior:

- `basic/go/native_temporal.go` — Time family Behaviors.
- `basic/go/types_timer.go` — Timeout/Interval Behaviors.
- `basic/go/types_bytes.go` — Bytes Behavior (render/order/size/bake).
- `basic/go/types_handles.go` — Patrun/Pid/Service Behavior shells
  (the matcher/service state stays in lang and implements the
  delegation interfaces there).
- `lang/go/modules/matrix.go` — Tensor/Matrix/Vector Behavior.

The dispatch in `Value.String` walks the Parent chain so
descendants of a type with a custom Behavior inherit it — e.g.
`Node/Map/Inspect` (descendant of `Node/Map`) inherits the
`Node/Map` map-formatting Behavior without per-subtype
registration.

Optional capability interfaces (`Comparer`, `Hasher`, `Walker`,
`IdealConverter`) let a type opt into extra operations without
expanding the required `TypeBehavior` interface.

`IdealConverter` (`convert_ideal.go`) gives an Ideal value a
conversion to a Map or List via the `convert Map <v>` / `convert
List <v>` words. Dispatch walks the value's Parent chain for the
nearest implementation (like the `Comparer` walk); the base `Ideal`
behavior is the terminal fallback returning `{}` / `[]`, so every
Ideal — built-in or user-defined, in Go or boru — is convertible.
Return `ErrNoConverter` to decline and keep walking. Concrete
projections are installed for Class instances (→ fields), Store
(→ entries), Error (→ {message}), Table (→ rows /
columnar map), and — in their owning packages — Module
(→ descriptor), Tensor/Matrix/Vector (→ nested
rows / {shape,values}), Fetch Request/Response (→ their map), and
Timeout/Interval (→ {id,ms}). Each is a behavior whose `Format`
delegates to the kernel default so rendering is unchanged.

## Where a Type Lives (kernel/domain boundary)

**Rule**: a type stays kernel-declared (in
`eng/go/typetable.go::builtinDecls` and `eng/go/types.go`'s `T*`
constants) **iff** one of these holds:

1. The parser emits it directly: `Integer`, `Float`, `String`,
   `Boolean`, `Atom`, `None`, `List`, `Map`.
2. The interpreter loop branches on it structurally: `Word`,
   `Forward`, `Mark`, `Move`, `OpenParen`, `CloseParen`, `End`,
   `ReturnCheck`, `DefCleanup`, `ParenExpr`, `InterpString`,
   `Module`.
3. It is a meta-type used by `is`/`typeof`/`inspect`: `Type`,
   `Function`, `FnUndef`, `Disjunct`, `Enum`. (`FnDef` — `Word/__FN`
   — was collapsed into `Function`; ADR-011.)
4. It is a structural type used by `make`/`record`/`class`:
   `Record`, `Options`, `Table`, `ChildType`, `Class`,
   `Resource`, `Store`, `Error`. This grouping is about **kernel
   residence**, not `make`-constructibility: `Store` and
   `Error` are deliberately not `make` targets (NUR018) —
   Stores are minted by the context machinery and Errors by
   `raise`.

   The `Scalar/Micron` structured-scalar family splits across the
   boundary (the Resource/Entity precedent): the IDENTITIES —
   paths, FixedIDs, positional Ranks, interval labels — stay in
   `builtinDecls`, together with the sealed payloads, their
   accessors, and Pathon's construction plumbing
   (`micron_kernel.go`, `core_make.go`), while the CONTENT — the
   twelve leaf validators, the literal grammars, the family
   Behavior/Comparer, `iso4217.go`, and the `-on` naming rule —
   lives in `basic/go/micron.go` and plugs back in through
   capabilities (rule 5): the family Ideal is registered per
   registry by `basic.InstallMicronIdeals`, the `-on` rule rides
   the `SubtypeNamer` Behavior capability, typed-def minting asks
   `MicronSubtypeMinter`, and the display backstop renders through
   `RegisterMicronRenderBridge`. The kernel names no Micron leaf.

Everything else — domain types like `Date`, `DateTime`,
`CalDuration`, `Matrix`, `Timeout`, `Interval`,
`Fetch{Function,Request,Response}`, all user types, all plugin
types — **flows through `RegisterType`**:

```go
// In the type's owning package:
var TFoo = registerFooType()

func registerFooType() *eng.Type {
    t, err := eng.Builtin.RegisterType(
        "Ideal/Foo",   // path
        N,             // stable FixedID from the documented per-module range
        "my:module",   // owner id (eng.OwnerKernel for kernel-shipped types) — design/OPEN-WORDS.1.md §4
        fooBehavior{}, // optional; nil → DefaultBehavior
    )
    if err != nil {
        panic(fmt.Sprintf("foo: register: %v", err))
    }
    return t
}
```

The var-initialiser pattern (rather than `init()`) is important:
package-level vars that reference TFoo (signature slices in
particular) need TFoo non-nil at slice-init time. Go resolves
var-init dependencies before declaration order.

If something would naturally live in `eng/` but the language
layer needs it (e.g. type registration policy, payload-marker
rules), the concern is **language-wide** and belongs here, not
duplicated in lang.

### Two categories — pick by lifetime, not by "is it a user type"

`RegisterType` is for types that are **global and
wire-stable**: one identity for the whole process, a FixedID baked
into serialised `Value.ID`s (matrix, time, fetch, the timers,
third-party plugins). It is NOT the path for a type that is
**module-scoped or behaves like a boru `def`** — those are
per-registry, have no FixedID, and must not bloat the builtin name
index or the FixedID snapshot.

For the second category, host-Go code routes through the **same
installer the `def` word uses** (`InstallType`), so a Go-defined type
and a source-defined one are indistinguishable — same unifier, canon,
dispatch wiring. Do NOT hand-roll `MintType` + a bespoke `TypeBehavior`
for these; reach for:

| Host need | Use | boru twin |
| --- | --- | --- |
| Any `def`-expressible body (alias, refine, union, negation, record, object, schema, …) | `(*Registry).DefineType(name, body)` → `*Type` | `def Name body` |
| A value/type union | `(*Registry).DefineEnum(name, alts…)` | `def Name (v0 tor v1 …)` |
| Membership as a **Go func** (the one shape `InstallType` can't express) | `(*Registry).DefineMemberType(name, parent, fn)` (mints + binds) or `r.Types.MintMemberType(...)` (mint only — module-scoped, reached via an export, like boru:io's `StreamKind`) | `def Name (refine Base …)` |

`MemberBehavior(func(Value) bool)` is the shared kernel wiring under
`MintMemberType`: one predicate yields Match, Unify, canon delegation
and `is`/dispatch agreement — the same `matchMembership` /
`unifyMembership` contract the boru predicate path (`predicateUnifier`)
routes through (`eng/go/membership.go`). Embedders get all of this on
the public surface: `lang.Boru.DefineType` / `DefineTypeFromSource` /
`DefineEnum` / `DefineMemberType`.

## FixedID Allocation

FixedIDs are baked into serialised Value IDs
(`eng/go/typetable.go::formatFixedID` produces a 14-char ID
embedded in `Value.ID`). Changing an existing type's FixedID is
a wire-compatibility break.

Documented per-module ranges (see
`TypeTable.RegisterType` doc):

```
   1-99       eng kernel builtins
   100-999    reserved for future eng-internal builtins
   1000-1999  basic/go — Scalar/Time family (1000-1003), Scalar/Bytes (1009)
   2000-2999  boru:matrix (module-owned; delivered by BuildMatrixModule)
   3000-3999  Fetch family (retired to boru:net module mints)
   4000-4999  Timeout, Interval (retired to boru:time-util module mints)
   5000-9999  kernel/language band: Module 5000 + KeyVal 5002 (eng
              builtinDecls, explicit external-band Ranks),
              MiniLangCompiled 5003 (boru:minilang),
              Patrun 5004 / Pid 5007 / Service 5008 (basic/go);
              5001, 5005, 5006 retired — never recycled
   10000+     host / third-party plugin types
```

The regression gate is
`lang/go/test/fixedid_stability_test.go::TestFixedIDStability` — it
snapshots every known FixedID and fails on drift. Adding a new
externally-registered type means:

1. Pick a FixedID from your module's reserved range.
2. Add the path → FixedID entry to the snapshot.
3. The test asserts your type registers at the expected ID.

## Type Lattice Fields

`Type` carries one field populated at registration time:

- `Rank int` — the **unified lattice rank**: one integer giving the
  total order `CompareValues` / `compareTypes` use for every cross-
  type ordering. The scheme:

  | Band | Kernel positional | External / user |
  |---|---|---|
  | Any / None / Never | `1·10¹⁰` | — (degenerate roots) |
  | Scalar branch | `2·10¹⁰`-band | `2.1·10¹⁰` (`externalBandFor`) |
  | Node branch | `3·10¹⁰`-band | `3.1·10¹⁰` |
  | Ideal branch | `4·10¹⁰`-band | `4.1·10¹⁰` |
  | Word branch | `5·10¹⁰`-band | `5.1·10¹⁰` |
  | Type branch | `6·10¹⁰`-band | `6.1·10¹⁰` |

  Kernel types get a positional Rank from `builtinDecl.Rank`
  (positional: parent + depth-scaled offset, so a child always
  ranks above its parent and siblings run least-to-most complex
  — laid out on `typetable.go::builtinDecls`). User types
  (`MintType`) and external builtins (`RegisterType`)
  share a single Rank per branch via `externalBandFor`, so they
  sort AFTER every kernel positional type in the same branch.
  Same-Rank ties break in `compareTypes` by depth → name → id;
  `rankOf` (`compare_types.go`) walks the parent chain as a
  fallback for a `*Type` assembled without one.

## Comparison & Ordering

`CompareValues` runs a three-stage cascade:

1. **LCA Comparer walk.** Walks the lowest common ancestor up the
   parent chain looking for a `Comparer` capability
   (`numberCompareBehavior`, `stringCompareBehavior`,
   `booleanCompareBehavior`, `atomCompareBehavior`,
   `wordCompareBehavior`, `scalarCompareBehavior`). The first
   Comparer found owns the result. Each Comparer can return
   `ErrNoComparer` to opt out (DepScalar payloads do this so
   numeric Comparers don't read DepScalarInfo as a zero float).
2. **Rank fallback** via `compareTypes` (Rank → depth → name →
   ID). Reached when no Comparer applies — cross-family pairs,
   cross-Micron-kind pairs, etc.
3. **Structural compare** (`compareStructural`) when types are
   identical: lists by length-then-element-wise, maps by length-
   then-sorted-keys-then-values, others by `CanonValue` lex.

**Type-literal-first rule.** Every family `Comparer` opens with
`litVsConcreteOrder(a, b)` — when exactly one side is a bare type
literal (`Data == nil && !Carrier`), it sorts FIRST. Both-literal
pairs delegate to `litVsLitOrder` → `compareTypes` so they order by
lattice Rank. The rule lives in the per-family Comparers (not
`scalarCompareBehavior`, which handles cross-family pairs where
Rank must own the result); the Micron family applies it inside
its family Comparer (`micronBehavior.Compare` — Pathon pairs keep
the verbatim `comparePathons` order) so the rule stays inside the
family without leaking.

Result is a strict total order over distinct lattice nodes, with
one deliberate value-level equivalence: cross-leaf numeric
magnitude (`1 cmp 1.0 → 0`). Full design at
`design/TYPE-ORDERING.10.md`; verification at
`lang/spec/compare.tsv`.

## Value's Method Surface Is Deliberately Tiny

`Value` exposes these methods (plus the `AsConcreteX` handler
accessors):

- `Is(t *Type) bool` — canonical dispatch. Routes through
  `t.Behavior.Match(v, t)`.
- `String() string` — `fmt.Stringer` interface.
- `TypeBody() (Value, bool)` / `SetTypeBody(body Value)` — the type
  NODE's declaration stamp (the Stage 2 flip,
  design/legacy/TYPE-REPRESENTATION.1.ignore §N2): `installTypeBinding` stamps
  the declared content at mint time, and consumers recover it through
  `core.TypeContentOf`.

Every former `IsX` / `AsX` accessor is now a free function in
this package. `Value.AsInteger()` → `eng.AsInteger(v)`. The
lang-layer aliases re-export them so `engine.AsInteger(v)` works
from lang/* packages.

Do NOT add new methods to `Value`. Add free functions instead.
Methods on `Value` accumulate API surface and become coupling
points; free functions are equally callable and can be moved
between packages without affecting the kernel.

## Type installation

A capitalised `def Foo body` installs a type binding (the
TYPE-UNIFORM syntax: `def` binds, `make` instantiates, `refine`
constructs — the legacy `type`-binder / `object` / `record` /
`table` / `untype` words were removed in Phase 3, and the `type`
constructor was renamed to `refine`).

The single source of truth is `eng/go/core_type.go::InstallType`. It
validates `body` is a valid type body, mints the lattice identity
via `TypeTable.MintType` for every declaration kind EXCEPT an alias
(`def Foo Integer`), which ADOPTS the canonical aliased node via
`PushTypeAdopted` (`Minted=false`), binds it in the single `DefTable`
(`PushType`, carrying the minted `*Type`), and stamps the declared
content onto the node (`installTypeBinding` → `SetTypeBody`).
`def`'s handler delegates here for capitalised names regardless of
which layer (eng or lang) registered `def` — do not fork the logic.
`undef` of a capitalised name pops the binding and retires the type
only when the binding MINTED it (`DefEntry.Minted`); an adopted
alias node is left in the lattice.

If you need to extend the installation policy (a new name shape, an
extra validation rule), modify `InstallType` so every layer picks
it up.

## Canonical `*Type` Pointers (CRITICAL)

Because `type Type = Value` a "type literal" is dual: a `Value`
with `Data == nil` AND the canonical `*Type` registered in
`TypeTable.byID`. Code that takes `&v` of a stack-local type-
literal Value (or `NewTypeLiteral(t)`'s by-value return) gets a
NON-canonical pointer. The orphan compares Equal via ID, but
mutations to fields like `Behavior` — which `behave` writes
through the canonical pointer — do not propagate to it. LCA walks
landing on the orphan miss any user-installed Comparer.

**The discipline:** whenever a `*Type` might have been obtained
via `&v` from a by-value Value, route it through
`eng.CanonicalType(r, t)` (re-exported as `native.CanonicalType`)
before storing it or using it as an identity key. The helper looks
up the canonical pointer via `r.Types.LookupByID(t.ID)` and falls
back to `t` for types with no registered canonical (degenerate
roots, test fixtures with empty IDs).

Current call sites that must canonicalize:

- `core_type.go::InstallType` — bare type-literal body parent.
- `fn_params.go::LookupDefType` — type-name → body lookup.
- `fn_params.go::ResolveDefType` — bare-literal Value → `*Type`.
- `lang/native/native_type.go::refineBareHandler` — `MintRefinePrefab`
  parent.

See `design/legacy/TYPE-CANONICALIZATION.10.ignore`.

## Typed-Def Reparent

Every typed-def path that rewraps a value's `Parent` to a refine
subtype's `*Type` routes through `eng.ReparentValue(v, def)` (re-
exported as `native.ReparentValue`). The helper returns a fresh
by-value copy with `Parent` rebound — the `Payload`/`Pos`/`Quoted`/
`Carrier`/`Eval` fields are preserved.

The invariant the helper codifies: NEVER mutate `Parent` on a Value
that was returned by `Unify` when Unify could have swapped to a
type-literal side (`Unify(1, Foo-literal)` returns `Foo-literal`,
not `1`, when Foo is a strict subtype of Integer). The refine-bare
reparent originally got this wrong and stored the Foo type literal
as the binding instead of the integer body.

Current reparent callsites:

- `defTypedHandler` predicate-type branch.
- `defTypedHandler` refine-bare branch.
- `defTypedHandler` FnUndef branch.
- `RunTypedBind` (typed_bind.go) — the compiled mirror of the first
  two: `OpBindTyped` re-runs the predicate/refine validate + reparent
  over the runtime value for a typed def whose body was DYNAMIC at
  compile time (the interpreter branches above record the spec via
  `EmitState.RecordTypedBind`).
- (the class-type branch uses `eng.MakeObject` to construct an
  instance rather than reparenting — different shape, same intent.)

## Refine ↔ Def Constructor Protocol

`refine BaseType` (the bare 1-arg form) and `def Foo BaseType` (the
alias form, no `refine` word) MUST produce indistinguishable Values
at the binding site if the protocol is field-coincidental — both
have `Data == nil` and the same `*Type` ancestry. They are NOT the
same:

- `def Foo BaseType` — alias. Foo's body IS the input type
  literal. `typeof x` for `def x:Foo …` resolves to the input.
- `def Foo refine BaseType` — fresh subtype. Foo gets a minted
  lattice node with `Parent = BaseType`; the body bound to Foo is
  the new lattice's type literal.

The protocol channel between the two forms:

- `TypeTable.MintRefinePrefab(parent) *Type` — what
  `refineBareHandler` emits for the bare 1-arg form.
- `IsRefinePrefab(v) bool` — what `InstallType` matches to take
  the rename-and-bind path. Anything else falls through to the
  alias path.

Do NOT detect the prefab via inline field probes (`Origin ==
OriginUserDef && Name == ""`). Use the named helpers so the
intent is legible at the call site.

## Refine matching: newtype (bare) vs subset (predicate)

`refine` builds two different things, and they match by **one**
predicate — `v.Is(t)` (routed through the type's Behavior) — applied
**symmetrically** at every boundary: fn-param dispatch
(`signature.go::sigTypeMatches`), the `is` word, and the fn **return**
check (`engine.go`, which uses `v.Is(exp)` for exactly this reason).
Never reintroduce a boundary that asks a different question (e.g. a raw
`v.Parent.ConformsTo(exp)` on returns) — that is the param/return
asymmetry that `design/REFINE-NEWTYPE-VS-SUBSET.10.md` removed.

- **Bare refine** (`def Pos (refine Integer)` — no payload,
  `IsBareTypeNode(body)`): a **nominal newtype**.
  `bareRefineUnifier.Match` is nominal — a value is a `Pos` only if its
  tag is `Pos` or a subtype (`v.Parent.ConformsTo(t)`, NOT the base type).
  A plain `Integer` is not a `Pos`; construct one with `def x:Pos 42`
  (Unify/reparent — a separate path from Match). Symmetric-strict, like
  Haskell/Rust/Go newtypes.
- **Predicate refine** (`def Big (Integer gt 10)` — body carries
  `DepScalarInfo`, `body.IsDepScalar()`; that probe keys the INSTALL
  branch only — the NAME now evaluates to the node, where the test is
  `core.HasConstraintUnify` and the bounds are the node's recorded
  content): a **subset type**.
  `depScalarUnifier.Match` is value-sensitive — base-family membership
  AND the self-contained predicate (`depScalarCheck`, no registry).
  Admitted at params and returns iff the predicate holds; the value
  keeps its base tag (no reparent). Symmetric, like Ada subtypes /
  refinement types.

Builtins and classes keep `DefaultBehavior` / nominal matching, where
`v.Is(t)` coincides with `v.Parent.ConformsTo(t)` on concrete values
— so routing returns through `v.Is` left them unchanged. The
catch-all structural/singleton kinds (record shapes, `def One 1`,
typed-container literals) carry `BindingBodyUnifier` since the Stage 0
fix, so `is` and dispatch consult one structural rule there too.

## Standalone coverage parity with eng/ts

The kernel proves itself with its OWN suite on both implementations —
see `design/ENG-COVERAGE-PARITY.0.md` for the contract. Two ratchet
gates enforce it (floors only rise, target 100% on both):

- `make cover-gate-eng` — eng/go by eng/go's tests alone
  (`ENG_GATE_FLOOR`), on top of the repo-wide merged ADR-008 gate.
- `make test-ts` — the eng/ts suite with its line-coverage floor
  (`TS_GATE_LINES`); Go statements ≡ TS lines is the parity metric.

The standalone corpus lanes live in `corpus_standalone_test.go`
(interpret / check / compile-or-fallback over `eng/spec` with the
`specfix` fixtures); when you raise standalone coverage, raise the
matching floor in the same PR.
