# Measurements behind the no-copy review

Companion to [VALUE-NO-COPY.0.md](VALUE-NO-COPY.0.md). Measured on
2026-10-09 on main at `6780a6030`, Go 1.24.7, linux/amd64, 4 cores, with
the benchmark suites that ship in the repo plus scratch experiments that
touched nothing under the repo root. Every procedure, script and
benchmark source is in
[handover/interpreter-perf/README.md](handover/interpreter-perf/README.md),
and the two experiments are saved as patches in
[handover/interpreter-perf/patches/](handover/interpreter-perf/patches/).
Timings on this box move by about 5% between passes; the copy counts and
sizes are exact.

## 1. What a Value costs to move

`unsafe.Sizeof` from a scratch module importing `core`:

| struct | bytes | note |
|---|---|---|
| `core.Value` | 104 | 7 pointer facets + ID string + Data interface + Parent + 9 flag bytes |
| `ListPayload` | 24 | boxed into the `Data` interface: one allocation per list mint |
| `MapPayload` / `IntPayload` | 8 | a non-small integer payload boxes with an 8-byte allocation |
| `StrPayload` | 16 | |
| `SrcPos` | 32 | behind `pos *SrcPos`, shared by copying the pointer |
| `CapturedBinding` | 120 | holds a Value by value |
| `FnDefInfo` | 232 | boxed payload of a Function value |
| `Loop` | 872 | pooled; holds `Results []Value` |

History: `Value` was 184 bytes before the legacy speed plan's #1A series,
then 152, now 104. At 184 bytes `runtime.duffcopy` was measured at 15%
flat; the share is higher today (below) because the other costs that
series removed were larger.

## 2. How much CPU the copies take today

`go test -cpuprofile` over the repo's end-to-end benchmarks (1 s each),
flat shares of the copy, zero and allocation runtime routines:

| routine | FnKinds interp | Stage6 interp | Stage6 compiled | PerfWords |
|---|---|---|---|---|
| `runtime.duffcopy` (struct copy) | 19.8% | 18.8% | 19.3% | 13.4% |
| `runtime.duffzero` (struct zero) | 2.8% | 2.4% | 0.5% | — |
| `runtime.memclrNoHeapPointers` | 1.3% | 1.8% | 2.0% | 3.7% |
| `runtime.memmove` | 1.2% | 1.3% | 1.4% | 1.3% |
| copy + zero total | **25%** | **24%** | **23%** | **18%** |
| `runtime.mallocgc` (cumulative) | 9.4% | 9.2% | 13.3% | 14.7% |
| GC mark worker (cumulative) | 8.9% | 11.7% | 18.4% | 19.8% |

The copy share is the same on the compiled lane as on the interpreter:
the VM moves Values by value exactly as the tree-walker does.

Who calls `duffcopy` (edge share of its flat time, Stage6):

| caller | interp | compiled |
|---|---|---|
| `(*Type).IsAncestor` (value-receiver `In()`/`Out()` on a `*Type`) | 21.6% | 29.8% |
| `Value.Is` (value receiver) | 5.1% | — |
| `resolvedIndicesBeforeInto`, `PlanMatch`, `scanBoundaryToken`, `CollectCandidateScan`, `CollectForward` (matcher reads tape cells by value) | ~16% | — |
| `stepToken(val Value)`, `stepLiteral`, `stepWord`, `EffectiveResolved`, `Run`, `evalParenGroupAt`, `commitBarrierForward` (step loop) | ~20% | — |
| `NewValueRaw` (returns the struct) | 3.2% | 5.0% |
| `Tape.Splice` + `Tape.Set` | 2.3% | — |
| `(*vmContext).run`, `tapeCoupled`, `stampFnResultPos`, `invokeClosureOn` (VM) | — | 26% |
| `towerApply`, `isConcreteNumber`, `AsInteger`, relational/numeric handlers (by-value params) | — | ~12% |

So about a quarter of the copying is one lattice walk calling value-receiver
accessors through a pointer, a quarter is the step loop reading tape cells
by value, a sixth is signature matching, and the rest is spread across
handlers and constructors.

## 3. Blast radius of a representation change

Grep counts over non-test Go (717 files):

| what | count |
|---|---|
| mentions of `Value` | 12,740 |
| `[]Value` | 5,550 |
| functions taking a `Value` by value | 3,380 |
| functions taking a `*Value` | 89 |
| functions returning a `Value` | 686 |
| native handler signatures `args []Value` | 870 |
| `map[string]Value` | 640 |
| `Value{}` zero literals | 808 |
| struct fields of type `Value` | ~88 |
| value-receiver methods on `Value` | 23 (9 pointer-receiver) |
| test files mentioning `Value` | 981 |

Exact count of by-value copies: in a scratch worktree a zero-size
`noCopy` field (Lock/Unlock methods) was added to `Value`, and `go vet
-copylocks` then reports every copy the compiler makes. This is also the
enforcement tool the refactor can use (the gate is "vet reports zero").
The marker (`patches/vet-nocopy-marker.patch` in the handover folder) is a
zero-size field placed first in the struct: `Value` stays
104 bytes with it, so it can live in the shipped code.

| module | copies (all) | copies (non-test) |
|---|---|---|
| core/go | 10,627 | 5,509 |
| lang/go | 11,196 | 4,902 |
| compiler/go | 3,992 | 1,698 |
| basic/go | 1,604 | 1,361 |
| check/go | 1,677 | 709 |
| eng/go | 2,047 | 606 |
| parser/go | 408 | 56 |
| cmd/go | 68 | 46 |
| test/go | 160 | 0 |
| **total** | **31,779** | **14,887** |

Non-test copies by kind: 9,944 call arguments passed by value, 1,949
by-value parameters and receivers declared, 925 assignments, 682 composite
literals, 676 returns, ~600 range variables, 120 func literals. The ten
files with the most: `compiler/go/emit.go` 1,013, `core/go/engine.go`
778, `eng/go/vm.go` 367, `lang/go/native/native_storage.go` 315,
`check/go/carrier.go` 302, `core/go/core_make.go` 294,
`basic/go/native_control.go` 291, `core/go/value.go` 289,
`basic/go/native_definition.go` 252, `lang/go/native/native_array.go` 251.

## 4. Synthetic representation benchmark

`rep/` runs one interpreter-shaped op (splice a body of 8 tokens onto a
tape, push each, dispatch an add per pair, store 7 results back) over the
real `core.Value` in five representations. Min of 5 runs, ns per op:

| representation | ns/op | allocs/op | what it models |
|---|---|---|---|
| by value, 104-byte struct, accessor calls | 380 | 0 | today |
| by value, direct field reads | 277 | 0 | today without by-value accessors |
| `*Value`, `new` per mint, direct reads | 747 | 7 | the policy with GC-allocated values |
| `*Value`, 1024-slot slab, direct reads | 462 | 0 (728 B/op amortised) | the policy with chunked allocation |
| `*Value`, per-op arena, direct reads | 62 | 0 | the policy with region allocation freed per frame |
| 32-byte header by value, direct reads | 55 | 0 | a header shrink, no policy change |

Primitive costs: copying 8 Values 12.3 ns vs 8 pointers 4.9 ns; minting an
integer by value 36 ns (payload box amortised to 7 B/op) vs minting it on
the heap 112 ns (120 B/op, one allocation).

Reading: a pointer representation removes the moves but adds an
allocation per value, and one heap allocation costs about three times
what the moves it saves cost. It only wins when values come from an arena
that is freed in bulk, which needs an ownership model (values escape into
bindings, containers and captures, so a frame arena cannot free them
without escape analysis). A 32-byte by-value header reaches the same point
as the arena without any ownership question.

## 5. Experiment: pointer receivers on the accessor methods

In a scratch worktree, 20 of the 23 value-receiver methods on `Value`
were switched to pointer receivers (`Pos`, `AscribedType` and `String`
left as they are because of method-expression and non-addressable uses),
which needed a local variable at seven non-test call sites. The change is
`patches/pointer-receivers.patch` in the handover folder; 105 call sites
in 30 test files need the same local before every module's tests compile
again (`pointer-receivers.test-sites.txt` beside it). The benchmarks live
in the `lang/go` root package, whose tests compile with the patch as it
is. Stage6, min of 3, after/before:

| shape | interp | compiled |
|---|---|---|
| arith_chain64 | 0.83 | 0.85 |
| compare_loop | 0.95 | 0.84 |
| if_scalar | 1.02 | 0.81 |
| for_tight | 0.93 | 0.91 |
| each_list | 0.88 | 0.92 |
| map_get | 0.82 | 0.88 |
| recursion_nontail | 0.84 | 0.88 |
| recursion_tail | 0.98 | 0.90 |
| **geomean over 16** | **0.888** | |

`IsAncestor` disappears from the `duffcopy` callers; the remaining copy
time (17.8% flat) is the step loop and the matcher reading tape cells by
value, `NewValueRaw` returning the struct, and handler parameters.

## 6. Reading the numbers together

- Copy and zero traffic is 23–25% of CPU on both lanes. That is the upper
  bound any no-copy change can win on throughput, about 1.3×, before
  paying for whatever replaces the copies.
- Roughly 11% of total time goes away with a 25-line mechanical change
  (pointer receivers), and the next tranche (tape cells and matcher reads
  by pointer, handler args by pointer) is the same kind of change on a
  larger surface: the step loop and `PlanMatch` paths named above.
- A full `*Value` representation with heap-allocated values would give
  back more than the 25% unless values are arena-allocated or the header
  shrinks first; the alloc ceilings `TestInterpAllocCeilings` and
  `TestCompiledAllocCeilings` enforce would have to rise by the number of
  values minted per op.
