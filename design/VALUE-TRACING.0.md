# VALUE-TRACING — infectious value provenance and the `boru:origin` module

Status: **Discovery draft** (2026-07). No code shipped. This note designs a
runtime **value-provenance** facility: mark a value *traceable*, and the engine
records — in a log held outside the value — every state transition it takes
part in, propagating the mark to everything derived from it, so an ancestry
graph can be walked in both directions to answer *"why does this value have
this actual value?"* and *"where did this come from?"*

It is grounded in seams that already exist in the tree (verified against the
kernel, the VM, the module layer and the CLI as of 2026-07) and it is staged so
a small, honest Phase 1 ships on those seams before anything expensive is
attempted.

Per repo policy (`ADR.md` header, `lang/go/CLAUDE.md` § "ADRs — only on
explicit instruction") this stays a `design/` note. No ADR entry is proposed.

---

## 1. The idea, and what it buys

boru already has a strong **control-flow** debugger. `boru debug`
(`design/BORU-DEBUGGER.0.md`, every phase shipped) gives source-line stepping,
line/word breakpoints, data watchpoints, break-on-error, post-mortem, a bounded
time-travel ring, deterministic `replay`, a DAP adapter and remote stepping.
The `boru:debug` module (`design/DEBUG-MODULE.0.md`) adds 27 words for printing,
introspection, memory and perf.

Not one of them answers a **dataflow** question. Faced with

```boru
def total (compute-invoice order)   # 7, and it should be 12
```

every existing tool answers *"where is execution now, and what is in scope?"*
The user's actual question is *"which chain of operations produced this 7?"*
Today that is answered by setting a breakpoint upstream and stepping forward
repeatedly, guessing where the divergence began. Provenance answers it in one
step:

```boru
import "boru:origin"
Origin.why total
# add 3 4          invoice.boru:41
#   ├─ 3  ← mul 1 3           invoice.boru:38   (line-item subtotal)
#   └─ 4  ← Order.shipping    invoice.boru:12   (marked here)
```

The observation that makes this tractable in *this* codebase specifically:
**boru already builds exactly this graph — at compile time.**
`EmitState.producedBy` (`eng/go/emit.go:587`) is

```go
producedBy map[string]producer   // value ID → producing (event seq, result idx)
```

keyed on `Value.ID`. The bytecode emitter depends on it for correct code
generation. What follows is the runtime twin of machinery the codebase already
trusts — and the identity field it keys on already exists on every `Value`,
already propagates through copies, and is *currently unused and empty at
runtime*.

### 1.1 What "infectious" means here

If `a` is traced, then `add a 1` is traced. If `a` is stored in a map, read
back out, passed to a fn, captured by a closure, or shipped through a
sub-engine, the value that comes out the other side is traced. The mark is not
a lexical property of a variable; it rides the *value*, by value, wherever it
goes.

That is what makes the ancestry graph complete without the engine having to
model aliasing, escape analysis, or a heap.

---

## 2. What already exists — reuse, do not rebuild

### 2.1 `Value.ID` — a per-value identity that is switched off at runtime

`eng/go/value.go:1211` — the `Value` struct opens with `ID string`. IDs are
minted by `GenerateID` (`value.go:1579`): a 2-character category prefix
(`S_` Scalar, `N_` Node, `W_` Word, `T_` Type/Object) followed by 12 lowercase
hex digits, drawn lock-free from an atomic monotone counter through a
splitmix64 finalizer, and reproducible via `SetIDSeed` (the determinism
`Options.Seed` relies on).

The decisive fact is the gate in `NewValueRaw` (`value.go:1669`):

```go
func NewValueRaw(t *Type, data Payload) Value {
	v := Value{Parent: t, Data: data}
	if data == nil || checkPassDepth.Load() > 0 {
		v.ID = GenerateID(IDPrefixForType(t))
	}
	return v
}
```

Bare lattice values (`data == nil`: type literals, carriers) always mint,
because canon, unify and the type registry read their IDs at runtime. **Concrete
runtime values do not** — the mint is elided outside a check/compile pass, which
the doc comment at `value.go:1620` records as ~21% of all interpreter
allocations. The same comment records a full audit
(`design/legacy/INTERPRETER-PYTHON-PARITY.10.ignore` Phase B) finding **no run-mode reader
of a concrete value's ID**.

Confirmed independently for this note: `eng/go/equal.go` and `eng/go/compare.go`
never read `.ID`; `canon.go` reads it only for type-name lookup
(`TypeNameByID`); `inspect` never surfaces it; and outside type handling the
only ID-as-map-key use is `emit.go:7107`.

So there is a 16-byte, copy-propagating, semantically inert identity slot
already sitting on every `Value`, empty at runtime, waiting.

### 2.2 One dispatch choke point per engine

**Interpreter.** `Engine.execMatch` (`eng/go/engine.go:3358`) is where every
word consumes its arguments and produces its results:

```go
results, err := match.Sig.dispatchHandler()(match.Args, ctx, nil, e.registry)   // :3649
...
stampResultPos(results, cur.pos)                                               // :3663
```

`match.Args` is the consumed input tuple; `results` is the produced output
tuple. `stampResultPos` (impl at `engine.go:923`) **already mutates that
results slice in place** to back-stamp the call-site position — the precedent
that a provenance hook at the same seam is a sanctioned kind of edit. A
`fullStack` variant of the same handler call sits at `engine.go:3594`.

**Compiled VM.** `eng/go/vm.go` has three `dispatchHandler()` sites:
`vm.go:518` (`OpCallNativePoly`), `vm.go:1127` (the foreign/module-fn invoker),
and `vm.go:1627` (`OpCallNative`). User-fn calls (`OpCallUser`,
`OpCallUserPoly`, the `OpCallDyn*` family) decompose into those, and `island()`
(`vm.go:181`) re-enters the interpreter for fallbacks.

### 2.3 The arm-able observability hook pattern

`eng/go/coverage.go` is the template, and it should be copied in shape rather
than re-invented:

- a holder struct on `Registry` carrying `atomic.Pointer[func(...)]`, so arming
  is race-free and the unarmed path is **one atomic load**;
- a plain (non-atomic) field as a cheaper pre-filter — `noteVMCoverage`
  short-circuits on a `coverID == ""` compare *before* the atomic load, keeping
  the VM's hot loop to one branch per instruction;
- `ArmCoverageHook(fn) func()` (`coverage.go:90`) returns the disarm closure,
  paired with `defer` / `t.Cleanup`;
- propagation into forks by `ForkConcurrent`'s shallow copy (`eng/go/fork.go:38`)
  and into module sub-registries by `InheritObserveHooks`
  (`eng/go/interp_entry.go:102`).

Siblings following the same discipline: `interpHook` / `bailHook`
(`interp_entry.go`), `Registry.Effects *EffectLedger` (`effects.go`), the stamp
log (`stamp_report.go`), and the registry-level debug trace with its
`debugParent` chain (`registry.go:295-320, 437-543`).

### 2.4 Region scoping has an exact precedent

`Test.cover [body]` (`lang/go/modules/test_coverage.go:204-218`) is the shape
`Origin.region [body]` should copy verbatim:

```go
Args:       []*native.Type{native.TList},
NoEvalArgs: map[int]bool{0: true},
...
body, err := native.RequireConcreteList(args[0], "Test.cover")
cover  := activeCover(parent)
disarm := parent.ArmCoverageHook(cover.record)
defer disarm()
_, e := native.New(r).Run(body.Slice())
```

### 2.5 What must NOT be reused

`eng/go/trace.go` is taken — it holds `RunTrace` / `TraceColorize`, the
step-trace renderer behind `IO.trace` and `Debug.trace`. The kernel file for
this work is **`eng/go/provenance.go`**. See §8.1 for the naming collision this
implies at the language level too.

---

## 3. Design principles

1. **Do not grow `Value`.** It is 88 bytes (measured), copied by value on every
   stack push, argument and tape cell, and under active size pressure
   (`design/legacy/INTERPRETER-SPEED-PLAN.10.ignore` #1A). Its eight one-byte fields pack
   with zero padding, so a ninth bool costs 8 bytes of padding; a new pointer
   field costs 8 bytes outright. Either is ~9% growth on the hottest copy in
   the system, permanently, for a feature that is off by default.
2. **Zero cost when unarmed.** One atomic load per dispatch, guarded by a plain
   field compare, per the `coverage.go` pattern. `TestInterpAllocCeilings` and
   `TestCompiledAllocCeilings` run inside `make test` and pin allocs/op *and*
   bytes/op; an unarmed trace must not move either.
3. **Never change execution mode silently.** Tracing instruments the compiled
   VM as well as the interpreter, rather than forcing the interpreter the way
   `boru debug` does. A program that behaves differently under observation is a
   debugging tool that lies.
4. **Bound everything, and say what was dropped.** Infectious taint over a long
   run makes everything traced. Every bound is explicit and every truncation is
   reported (§7.3). Silent truncation is the failure mode this repo cares most
   about avoiding.
5. **Never retain a `Value` in the log.** Capture a rendered summary. Retaining
   values pins mutable payloads alive (defeating GC) and lets a later in-place
   mutation corrupt what is supposed to be a snapshot.
6. **Element-level taint before container-level taint.** Phase 1 ships the
   semantics that falls out of the mechanism for free, and names the exact
   sites a later phase would need (§5.3). It does not pretend to cover them.
7. **No panics** (ADR-005). Every store operation is nil-safe and returns
   errors.

---

## 4. Marking — reuse the ID field

### 4.1 The mechanism

A **traced** value is one whose ID was minted with a distinguished prefix:
`S~` / `N~` / `W~` / `T~` in place of `S_` / `N_` / `W_` / `T_`. The predicate
is O(1), allocation-free, and needs no map probe:

```go
// IsTraced reports whether v carries a provenance identity. Traced IDs are
// minted with a '~' in the prefix's second byte, which no other ID producer
// emits, so the test is a length check and one byte compare.
func IsTraced(v Value) bool {
	return len(v.ID) == 14 && v.ID[1] == '~'
}
```

Because the ID is an ordinary field of the `Value` struct, it rides every copy.
That single fact buys, with **no further hooks**:

| Path | Why it just works |
|---|---|
| `def name v` then `name` | `DefTable` stores the `Value`; reads push a copy carrying the ID |
| element of a list / map | `ListPayload.Elems []Value` and `MapPayload` hold `Value`s inline |
| fn argument | args enter the frame as resolved `Value`s |
| closure capture | `FnDefInfo.Captured` holds `CapturedBinding{Name, Value}` |
| sub-engine, `each`/`fold` body, paren group | values are passed, not re-created |
| `ForkConcurrent` branch | the fork's stack holds the same `Value`s |
| across the interpreter/VM boundary (`island()`) | same `Value`s on both sides |

### 4.2 Why the mint must share `idState`

The traced mint **must draw from the same `idState` counter** that `GenerateID`
uses, differing only in the prefix it formats. A second, independent counter
could produce the same 12 hex digits as the ordinary stream. That is harmless
for provenance (the prefix differs) but not for `producedBy`, which keys on the
raw ID string: if a check or compile pass runs later in the same process over
values that carry runtime-minted IDs, a collision becomes a wrong provenance
edge in the *compiler*, which `emit.go:1424-1432` describes as "an active
miscompile".

Sharing the counter also preserves `SetIDSeed` determinism: a fixed seed and a
fixed tracing configuration reproduce the same ID stream, so trace transcripts
are golden-testable.

### 4.3 The collision argument, verified

Every type-ID producer writes `_` at index 1 and nothing else:
`formatFixedID` (`eng/go/typetable.go:964-979`) and `TypeTable.mintID`
(`:303-320`) both switch over `Scalar`/`Node`/`Word` and default to `T_`.
`GenerateID` takes its prefix from `IDPrefixForType` (`value.go:1600`), which
returns the same four constants. Therefore `v.ID[1] == '~'` is unambiguous.

### 4.4 Honest hazards

These are consequences of the mechanism, not defects, but each must be modelled
explicitly rather than discovered later:

- **`CloneValue` mints a fresh ID** for mutable containers (`clone.go:57-61`,
  `withPayload`). A clone is genuinely a distinct value, so it correctly starts
  a new node — but the tracer must record an explicit `clone-of` edge, or the
  chain silently ends at the clone.
- **`dup` yields two values with one ID** (`emit.go:1420` documents the same
  ambiguity on the compile side). For provenance this is *correct*: they are
  the same value, so they share a node. It is only a problem if a consumer
  assumes node ↔ stack-slot is one-to-one.
- **Type literals and carriers always mint.** Tracing must be restricted to
  `IsConcrete(v)` (`eng/go/util.go`, re-exported via `native/aliases.go`), so
  the mark can never land on a lattice node and perturb canon, unify or the
  type registry.
- **Traced runs mint IDs that untraced runs do not.** This is an observable
  difference between running with and without tracing. Nothing at runtime reads
  a concrete value's ID (§2.1), so it is not a behavioural difference — but it
  is an allocation difference, and it is the subject of open question Q5.

### 4.5 Alternatives considered and rejected

| Option | Cost | Verdict |
|---|---|---|
| A 9th one-byte field on `Value` | +8 bytes (padding) — 88 → 96 | Rejected: ~9% on the hottest copy, permanently, for an off-by-default feature |
| A new nilable pointer field (`prov *provID`), following the `pos`/`elem`/`asc` pattern | +8 bytes — 88 → 96 | Rejected for the same reason. It is the cleanest design on paper and the right fallback **if** the ID scheme is ever found to be load-bearing somewhere the audit missed |
| Steal a bit from `Origin OriginKind` | 0 bytes | Rejected: `Origin` is compared as a whole in several places; a masked read is a latent bug farm |
| Side map from `Value` address to node | 0 bytes on `Value` | Rejected: `Value` is copied by value, so it has no stable address |
| Global side map keyed on a mint-everything ID | Reinstates the ~21% allocation the mint gate removed | Rejected |

---

## 5. Propagation

### 5.1 The interpreter hook

One site, in `execMatch`, immediately alongside `stampResultPos`:

```go
if ps := e.registry.provStore(); ps != nil {      // one atomic load — see §7.1
	ps.noteDerivation(e.registry, match.Name, match.Args, results, cur.pos)
}
```

`noteDerivation` (in `eng/go/provenance.go`) does:

1. scan `args` for `IsTraced`; return immediately if none — this is the common
   case even under an armed tracer;
2. for each result: if it is already traced (an identity word, `dup`,
   `Debug.tap`, or a handler returning an input verbatim), record a
   **pass-through** event on the existing node and leave the ID alone;
   otherwise mint a traced ID onto it and create a **derivation** node;
3. record parent→child edges from every traced arg to every newly minted
   result, and one derivation event carrying the word name, the source position
   and the captured renders.

Mutating `results[i].ID` is safe. `Value` is a struct, so the elements of
`results` are copies — writing an ID cannot reach the tape (the splice happens
afterwards), a const pool, or a stored binding. `stampResultPos` already
mutates the same slice for the same reason. The one aliasing case is a handler
that returns its own `args` slice; then `results[i]` *is* `match.Args[i]`, which
is precisely the pass-through case step 2 skips.

### 5.2 The VM hooks

The same helper at `vm.go:518`, `vm.go:1127` and `vm.go:1627`, with one
non-obvious constraint at the third.

**`OpCallNative` delivers args from a shared, reused scratch buffer**
(`vm.go:1598-1605`):

```go
var args []Value
if vmFreshArgsPerCall {
	args = make([]Value, n)
} else {
	if cap(argScratch) < n { argScratch = make([]Value, n) }
	args = argScratch[:n]
}
```

The comment above it calls this "the dominant per-CALL_NATIVE allocation on the
compute path", and notes that compiled-reachable natives must not retain the
slice. **The provenance hook must obey the same rule**: read the traced IDs out
synchronously, copy anything it keeps, and never store the slice or a subslice
of it. The `-tags borudebug` seam `vmFreshArgsPerCall` exists precisely to
localise a violator, and a Phase-1 test should run the compiled corpus under it.

Results must be stamped **before** the `append` onto the operand stack, mirroring
the interpreter's stamp-before-splice ordering.

### 5.3 The known gap: container construction

Container construction does **not** flow through a dispatch handler on either
engine. The sites are:

| Site | Engine | What it builds |
|---|---|---|
| `autoEvalList` (`engine.go:4245`, `NewList(result)`) | interpreter | a list from evaluated elements |
| `autoEvalMap` | interpreter | a map from evaluated values |
| `IsInterpString` branch in the Run loop | interpreter | a string from interpolated holes |
| `OpMakeList` (`vm.go:1439`), `OpMakeMap` (`:1455`) | VM | the compiled twins |
| `OpInterp` (`vm.go:1462`), `OpInterpXml` (`:1487`) | VM | compiled interpolation |

Phase 1 therefore ships **element-level taint**, which is what the mechanism
gives for free and is defensible on its own terms:

> A traced value stored in a container keeps its traced ID inside
> `ListPayload.Elems` / `MapPayload`. The container itself is not traced.
> Reading the element back out (`items.2` → `dot` → `execMatch`) yields the
> traced element, so the chain continues through the container without the
> container being part of it.

This answers "where did this element come from?" perfectly and "why does this
list look like this?" not at all. Container-level taint is a **named Phase-2
item** with the five sites above as its exact work list. Interpolated strings
are the one case where the gap bites in practice (`` `total: ${t}` `` loses the
link to `t`), so `OpInterp` and the interp-string branch are the two to do
first if only some are done.

### 5.4 Reads

The dispatch hook already produces the most informative read event: *"traced
value X was consumed by word W at file:line"*. That is what a user means by
"where was this read?"

A weaker second read event — a traced value fetched out of a binding — has
exactly one clean hook: `DefTable.Top` (`eng/go/deftable.go:71`), the single
function that all ~12 `Defs.Top` call sites in `engine.go` funnel through. It
is deliberately **not** Phase 1: name lookup is a much hotter path than
dispatch, and the marginal information is small.

### 5.5 Mutations

In-place mutation of a traced receiver (`FlexList` append, `Store` COW via
`CowSet`, `set` on a flex map) happens *inside* a handler, so the dispatch hook
sees the call. The receiver's ID is unchanged by an in-place mutation, so the
tracer appends a **mutation** event to the existing node rather than creating a
new one — which is exactly the "state transitions of the value" the idea asks
for. A COW `set` that returns a new container produces a new node plus a
`derived-from` edge, which is also correct.

---

## 6. The data model

### 6.1 In-memory

```go
// eng/go/provenance.go

// ProvKind classifies one recorded transition.
type ProvKind uint8

const (
	ProvBirth   ProvKind = iota // Origin.mark made this value traceable
	ProvDerive                  // produced by a dispatch from traced inputs
	ProvPass                    // returned verbatim by a dispatch (dup, tap)
	ProvMutate                  // mutated in place by a dispatch
	ProvClone                   // CloneValue produced a distinct copy
	ProvBind                    // bound to a name by def
)

// ProvEvent is one transition. It holds NO Value — only a rendered capture,
// so a mutable payload stays collectable and a later in-place mutation cannot
// rewrite history.
type ProvEvent struct {
	Seq     uint64   // process-monotonic; the cross-sub-engine ordering
	Kind    ProvKind
	Word    string   // dispatched word ("" for a birth)
	Row     int      // SrcPos row; 0 = unknown
	File    string   // resolved from the executing Registry's BaseFile
	Inputs  []string // traced IDs consumed
	Capture string   // truncated render of the value AT this event
	Elided  bool     // Capture was truncated
}

// ProvNode is one traced value.
type ProvNode struct {
	ID       string
	TypeName string
	Parents  []string // traced IDs this value was derived from
	Children []string // traced IDs derived from this value
	Events   []ProvEvent
	Dropped  uint32   // events discarded by the per-node cap
}

// ProvStore is the log. It hangs off the Registry beside the other
// observability holders and is shared by pointer into forks and module
// sub-registries.
type ProvStore struct {
	mu      sync.Mutex
	nodes   map[string]*ProvNode
	ring    []ProvEvent // bounded global event ring
	seq     atomic.Uint64
	dropped uint64      // events the ring discarded
	cfg     ProvConfig
}
```

`Parents` and `Children` are both materialised at edge-creation time, so
traversal is O(1) per hop in **both** directions — that is the whole point of
the design. "Why is this 7?" walks `Parents`; "what did this shipping cost
affect?" walks `Children`.

### 6.2 The monotonic sequence is a bonus

`ProvStore.seq` is a process-monotonic counter incremented at every event,
across every engine, sub-engine and fork. `design/BORU-DEBUGGER.0.md` §6.4 names
"a global monotonic step clock spanning sub-engines" as one of the three things
missing for full time-travel, alongside sub-engine trace propagation (since
shipped, via `SetDebugTrace`) and a state-restore seam. This work delivers the
first for free. It is not a commitment to full time-travel; it is worth
recording so the opportunity is not lost twice.

### 6.3 Capture policy

The default capture is a **truncated render**: `Value.String()` (or
`CanonValue` for a re-parseable form) capped at ~120 bytes with `Elided` set
when the cap bites. Three levels, selected at arm time:

| Level | Records | Use |
|---|---|---|
| `shape` | type name, container length, depth | secret-bearing runs; the disk default |
| `summary` *(default)* | type name + truncated render | ordinary debugging |
| `full` | type name + untruncated render | small deterministic repros |

`full` still renders rather than retaining the `Value` — principle 5 holds at
every level. Rendering a deep container is not free, which is why `summary`'s
cap is a byte budget rather than a depth limit.

---

## 7. Performance and memory

### 7.1 Unarmed

The holder is an `atomic.Pointer[ProvStore]`, for the same reason
`coverage.go` uses one: arming must be race-free against forks that are already
running. So the unarmed cost is **one relaxed atomic load and a nil compare per
dispatch** — on amd64 and arm64 an acquire load is a plain move with no fence.

Note the honest difference from `noteVMCoverage`: coverage gets a *second*,
cheaper gate from the plain `coverID == ""` field compare, because a registry
knows at module-load time whether it is a coverage target. Provenance has no
equivalent per-registry tag — any dispatch can consume a traced value — so
there is no two-level gate to copy. What it has instead is that the load is
**per dispatch**, while `noteCoverage` already pays one **per step**, which is
strictly more frequent. That is the precedent for calling this affordable.

The gate for Phase 1 is empirical, not argued: `TestInterpAllocCeilings` and
`TestCompiledAllocCeilings` must be **unchanged** (not raised) with the code in
place and unarmed, and `make bench` compared with `benchstat` must show no
significant regression on `BenchmarkStage6` and `BenchmarkBytecodeBaseline`.
If the compare is measurable, the fallback is to fold the check into an
existing branch rather than to raise a ceiling.

### 7.2 Armed

The dominant armed costs, in order:

1. **The arg scan** — O(nargs) byte compares per dispatch. Cheap, and it
   short-circuits: a dispatch with no traced args does nothing else.
2. **The ID mint** — one `GenerateID` (one allocation) per newly traced result.
   This is the ~21% allocation the mint gate removed, reinstated *only* for
   values on a traced lineage.
3. **The capture render** — the real cost, and the reason `summary` caps by
   bytes. A traced map with 10 000 entries must not render 10 000 entries on
   every event.
4. **Store insertion** — one map write plus two slice appends per derivation,
   under a mutex (forks share the store).

The mutex is the one genuine scalability concern: a heavily-forked concurrent
program with a wide traced lineage will contend. Phase 1 accepts a single
mutex and *measures* it; sharding by ID prefix is the obvious escape and is
deliberately not built speculatively.

### 7.3 Memory, and the unbounded-taint problem

Infectious taint has an unbounded worst case by construction: mark one value
early in a long-running program and eventually everything descends from it.
Four bounds, all explicit, all reported:

- **A bounded global event ring** (`ProvConfig.MaxEvents`, default ~100 000)
  with `ProvStore.dropped` counted and surfaced by `Origin.stats`. A report
  built on a ring that dropped events says so.
- **A per-node event cap** (`ProvConfig.MaxEventsPerNode`, default ~64) with
  `ProvNode.Dropped` per node, so one hot value in a loop cannot starve the
  ring.
- **Region scoping** — `Origin.region [body]` arms only for the body's dynamic
  extent, the `Test.cover` pattern (§2.4). This is the recommended default
  usage and should be what the documentation leads with.
- **Generation depth** (`ProvConfig.MaxDepth`, default unlimited) — stop
  propagating past N derivations from a root. Blunt, but it is the only bound
  that attacks the taint itself rather than the log.

GC retention is bounded by construction: nodes hold strings, never `Value`s, so
a traced 100 MB map contributes ~120 bytes per event and is collected as soon
as the program drops it.

### 7.4 What this does *not* solve

A trace that has dropped events cannot always answer "why". The honest position
is that `Origin.why` reports the chain it has and marks where the chain was
truncated, and `Origin.stats` reports the drop counts, so the user knows to
re-run with a tighter `Origin.region` or a larger ring. A provenance tool that
silently returns a shorter chain is worse than one that says it lost the thread.

---

## 8. The module surface

### 8.1 Naming — why `origin`, not `trace`

Three unrelated things in this codebase are already called *trace*:

| Existing | Means |
|---|---|
| `IO.trace` | print the step-by-step stack evolution of a body |
| `Debug.trace` | the same, re-exported as the discoverable home |
| `Log.trace` | the lowest severity level (OTel TRACE) |

A fourth meaning is one too many. `boru:trace` / namespace `Trace` was
considered and **rejected**: it is technically legal — the namespaces are
disjoint, and ADR-001 governs shadowing of *core words*, which this would not
do — but a reader landing on `Trace.why` has no way to know whether it relates
to `Debug.trace` or `Log.trace`, and it would have forced the awkward
non-word `Trace.trace`.

The module is therefore **`boru:origin`**, namespace `Origin`. It reads better
at the call site (`Origin.why total`, `Origin.root total`) and collides with
nothing in the word namespace.

Two consequences worth stating plainly rather than leaving as loose ends:

- **The concept is still *tracing*.** The note is `VALUE-TRACING.0.md`, the
  kernel predicate is `IsTraced`, and a value on a recorded lineage is a
  *traced* value. Only the **module** is named `origin` — for the question it
  answers, not the mechanism it uses. That split is deliberate; it is not
  leftover naming.
- **`Origin` already means something in Go.** `OriginKind` classifies where a
  `*Type` was registered, and is read as `Value.Origin` / `Signature.Origin`
  (`eng/go/typetable.go:10`). There is no boru-level clash, and this work's
  kernel types are `Prov*` (`ProvStore`, `ProvNode`, `ProvEvent`, `ProvKind`),
  so nothing actually collides — but a reader of `eng/go` should not be
  surprised twice by the same word.

Whatever else changes, **there must be no `Origin.origin`** — which is why the
root-finding word in §8.2 is `Origin.root`.

### 8.2 The words

Module id `boru:origin` in code, documented as `boru:origin` — the resolver still
matches `strings.HasPrefix(path, "boru:")` (`native_module_module.go:743`) while
the docs run ahead of the engine, exactly as
`test/go/docexamples/docexamples_test.go:69-70` already allows for other
modules.

| Word | Shape | Does |
|---|---|---|
| `Origin.mark` | `Any ~> Any` | Mark and **return the value unchanged** — tap-shaped like `Debug.tap`, so it drops into a pipeline without restructuring it |
| `Origin.marked?` | `Any ~> Boolean` | Is this value traced? |
| `Origin.id` | `Any ~> String` | The traced id, or `None` |
| `Origin.region` | `List ~> Any` | Run the body with tracing armed; disarm on exit (`NoEvalArgs`) |
| `Origin.on` / `Origin.off` | `Map ~> None` / `None ~> None` | Explicit arm/disarm with config, for REPL and long-running use |
| `Origin.why` | `Any ~> List` | The derivation chain, nearest-first: the ancestor events that produced this value |
| `Origin.root` | `Any ~> List` | The root marked value(s) this descends from |
| `Origin.uses` | `Any ~> List` | Forward: the values derived from this one |
| `Origin.events` | `Any ~> List` | This value's own event log |
| `Origin.graph` | `Any ~> Map` | The ancestry subgraph as `{nodes, edges}` — the data the tooling renders |
| `Origin.explain` | `Any ~> String` | A rendered human explanation tree, as a String so it can be embedded |
| `Origin.stats` | `None ~> Map` | Node count, event count, **dropped counts**, estimated bytes |
| `Origin.checkpoint` | `Pathon ~> None` | Flush the log to disk |
| `Origin.load` | `Pathon ~> Map` | Read a checkpoint back as data (pure — this is what makes offline tooling possible) |
| `Origin.clear` | `None ~> None` | Drop the store |

Composition is the point:

```boru
import "boru:origin"

Origin.region [
  def order (Origin.mark (load-order 42))
  def total (compute-invoice order)
  print (Origin.explain total)
]
```

### 8.3 Obligations this incurs

- **ADR-003** — every export needs at least one row in
  `lang/spec/module-origin.tsv`, gated by
  `test/go/langspec/coverage_test.go::TestModuleExportCoverage`. Words whose
  behaviour is not hermetically expressible (`Origin.checkpoint` writes a file)
  go in `hermeticExempt` with a justification, or are driven through the
  in-memory `FileOps`.
- **ADR-001** — no export may shadow a core word. None of the above does.
- **`BarrierPos: -1`** on every inner native registered into the module
  sub-registry (`lang/go/CLAUDE.md` § "Module FnDef Wrappers", gated by
  `lang/go/modules/wrapper_dispatch_test.go`).
- `registerDocs("boru:origin", …)` in `lang/go/modules/docs_origin.go` (gated by
  `TestModuleExportDocs`) and a `moduleCatalog` row in
  `lang/go/native/help/help_render.go` (gated by
  `TestModuleCatalogMatchesModules`).

---

## 9. Checkpoints and the boru-written tooling

### 9.1 On-disk format

Append-only **NDJSON**: one header line carrying the schema version, the run's
ID seed and the capture level, then one line per event. Nodes are reconstructed
by replay, so a truncated file is still readable up to its last complete line —
the property that matters when a checkpoint is written by a program that then
crashed.

```
{"v":1,"seed":42,"capture":"shape","started":"..."}
{"seq":1,"kind":"birth","id":"S~a1b2...","type":"Integer","cap":"42","row":12,"file":"invoice.boru"}
{"seq":2,"kind":"derive","id":"S~c3d4...","in":["S~a1b2..."],"word":"mul","cap":"126","row":38,"file":"invoice.boru"}
```

`Origin.load` parses it back into the same `{nodes, edges}` shape `Origin.graph`
returns, so **online and offline analysis run the same code**.

### 9.2 The tooling, in boru

`kg/` is the precedent: 14 `.boru` modules, 6 test suites, driven by a Makefile
target that is literally `boru main.boru`. The trace tooling follows it —
`tools/origin/*.boru` with a `main.boru` entry point — and renders three views
from `Origin.load`'s output:

1. **explanation tree** — the default; the `Origin.explain` rendering, offline;
2. **DOT** — for graphviz, for large graphs;
3. **mermaid** — for pasting into docs and issues.

Writing it in boru is not decoration: it dogfoods `boru:origin` against a real
graph workload, and it keeps the renderers out of the Go binary. A `boru trace`
CLI subcommand is deliberately **not** proposed for Phase 1 — a script that
reads a checkpoint file has no CLI surface to maintain.

### 9.3 Debugger integration

`boru debug` already has the session, the prompt and the pause. A `why <expr>`
command at the prompt — evaluate the expression in the existing child engine,
then render `Origin.why` over the result — is a small addition to
`cmd/go/internal/debugger` and is the single highest-value integration. It
turns "stop where it went wrong" and "explain how it got that way" into one
tool. Phase 3.

---

## 10. Policy and safety

**Captured renders can contain secrets.** This is the sharpest edge in the whole
design: a trace of a request handler will capture tokens, passwords and personal
data, and `Origin.checkpoint` writes them to a file.

The stance:

- in-memory default is `summary` (truncated render), which is what makes the
  tool useful;
- **disk checkpoints downgrade to `shape` unless the caller explicitly opts in**
  (`Origin.checkpoint path {capture: summary}`), so the dangerous thing requires
  a deliberate act;
- `Origin.on {capture: shape}` is the documented posture for anything running
  against production data;
- checkpoint writes go through the `FileOps` capability like every other file
  effect, are gated by the existing `fileops` policy scope, and **must call
  `Registry.NoteEffect()`** so the compiled-mode fallback fence (`effects.go`)
  knows an observable effect escaped and cannot silently re-run the program.

**Module gating is free.** `modules.Resolve` (`lang/go/modules/modules.go:63`)
already checks `pol.Installed("modules")`, `pol.Check("modules","import",…)` and
the per-module subscope, so a profile can deny `boru:origin` outright with no new
machinery. `KnownScopes` (`lang/go/policy/policy.go:108`) has no `debug` scope —
the one `DEBUG-MODULE.0.md` §8 proposed was never added — so a dedicated
`trace` scope would be new policy surface. Whether that is wanted is Q2.

---

## 11. Phases

**Phase 1 — the kernel seam and a minimal module.**
New: `eng/go/provenance.go` (the store, `IsTraced`, the traced mint,
`noteDerivation`, arm/disarm, `InheritObserveHooks` participation),
`lang/go/native/origin_module.go` (the natives), `lang/go/modules/origin.go`
(`BuildOriginModule`), `lang/go/modules/docs_origin.go`,
`lang/spec/module-origin.tsv`.
Modified: `eng/go/engine.go` (one hook in `execMatch`), `eng/go/vm.go` (three
hooks), `eng/go/registry.go` (the holder field), `eng/go/interp_entry.go`
(`InheritObserveHooks`), `eng/go/fork.go` if the shallow copy needs help,
`lang/go/modules/modules.go` (the map entry),
`lang/go/native/help/help_render.go` (the catalog row), `REFERENCE.md`.
Words: `mark`, `marked?`, `id`, `region`, `why`, `root`, `uses`, `stats`,
`clear`.
Ships element-level taint only (§5.3), documented as such.

**Phase 2 — completeness and persistence.** Container-level taint at the five
sites in §5.3 (interpolated strings first). Binding-read events via
`DefTable.Top` (§5.4). `Origin.events`, `Origin.graph`, `Origin.explain`,
`Origin.checkpoint`, `Origin.load`.

**Phase 3 — tooling and integration.** `tools/origin/*.boru`. The `boru debug`
`why <expr>` command (§9.3).

**Beyond.** Store sharding if the mutex measures badly (§7.2); using the
monotonic sequence for the debugger's full time-travel (§6.2).

---

## 12. Test discipline

Per `lang/go/CLAUDE.md` and ADR-008, and pairing every positive with a negative:

- **Unarmed is free** — a test asserting `TestInterpAllocCeilings` /
  `TestCompiledAllocCeilings` pass **unchanged**, plus a `benchstat` comparison
  recorded in the Phase-1 commit message.
- **Propagation positive** — `add (Origin.mark 3) 4` produces a traced result
  whose `Origin.why` names `add`; **negative** — `add 3 4` with tracing armed but
  nothing marked produces an untraced result and zero events.
- **Infection through every path** — one row each for binding, container
  element, fn argument, closure capture, sub-engine body, fork.
- **Identity words** — `dup` on a traced value yields two values sharing one
  node, not two nodes; `Origin.mark` twice does not create two nodes.
- **The container gap is pinned as a negative**, so Phase 2 changing it is a
  visible, deliberate diff rather than an accident.
- **VM parity** — the same program traced compiled and interpreted yields the
  same graph; and the compiled corpus runs green under `-tags borudebug`
  (`vmFreshArgsPerCall`) to prove the hook does not retain `argScratch`.
- **Bounds are honest** — a run that overflows the ring reports a non-zero
  `dropped` from `Origin.stats`, and `Origin.why` marks the truncated chain.
- **Determinism** — same seed, same program, byte-identical trace transcript.
- **Policy refusal** — importing `boru:origin` under a denying profile fails with
  the documented error and leaks no state.
- **Secrets** — `Origin.checkpoint` without an explicit capture level writes
  `shape` records containing no payload bytes.
- Spec-style rows for every export in `lang/spec/module-origin.tsv` (ADR-003);
  host-level behaviour in Go tests beside `eng/go/provenance.go`.

---

## 13. Open questions for the maintainer

1. **The `~` prefix (§4).** Reuse `Value.ID` with a distinguished prefix
   (0 bytes, slightly clever), or add a nilable `prov *provID` pointer field
   (+8 bytes on an 88-byte struct copied on every push, cleaner to read)?
   *Leaning: the prefix.* The audit says concrete-value IDs are inert, the
   collision argument is verified, and 9% on the hottest copy is a real,
   permanent cost for a feature that is off by default.
2. **An `origin` policy scope.** Is per-module gating via the existing
   `modules` scope enough, or should `origin` join `KnownScopes` so a profile
   can permit the import but deny arming? *Leaning: `modules` is enough for
   Phase 1; revisit if `Origin.on` (as opposed to `Origin.region`) proves
   popular.*
3. **Container-level taint (§5.3).** Is element-level taint an acceptable
   shipping semantics, or is a traced list a hard requirement? Interpolated
   strings are the case where the gap is most visible.
4. **Is an NUR record owed?** A traced run mints value IDs an untraced run does
   not (§4.4). Nothing at runtime reads them, so it is not a behavioural
   divergence — but "the same program allocates differently under observation"
   is exactly the shape of thing `NUR.md` exists to record. *Leaning: record it
   once Phase 1 lands and the effect is measured, not before.*
5. **Scope of the `boru:debug` overlap.** Should this be a new module at all, or
   should the words join `boru:debug` as a sixth surface? *Leaning: separate —
   `boru:debug` is already 27 words across five surfaces, and provenance has its
   own lifecycle, policy story and on-disk format.*
