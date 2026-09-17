# voxgig-boru compile leaves — trie suites, post-3-fix state (continuation of .1)

Continuation of `VOXGIG-COMPILE-LEAVES.1.md`, scoped to the **trie** repo after
the three Stage-D fixes landed (branch `claude/voxgig-boru-baseline-pctxto`,
commit `3e94429`: the `AnalyseFnBody` quota skip, the `RecordDynBind` capID
override, and the `doListReturnsFn` fallible-multi-value refusal). Those fixes
made the four `*_prop_spec` + `trie_unit_spec` suites compile natively; the
rest still REFUSE and are **silently** re-run on the interpreter by the runtime
scaffolding (every suite stdout-byte-identical interpreter-vs-`--compile`, all
green — which is precisely why the failures hide: nothing in a green run says
the compile was refused). Each of those refusals is an **open defect owed a
fix**, not a resting place; DONE is every suite COMPILED. This note records, with fresh traces, the **exact
unmasked root cause** of each of the three refusals that remain, so a focused
follow-up can implement them one at a time behind the usual gates
(`verify-bytecode` byte-identical + off-corpus `RunCompiledStrict`-vs-`Run`
regression + `--compile`==interpret sweep + `cover-gate` 100%).

Current trie split (boru branch build, `203ea2f` + the 3 fixes):

| Suite | `--force-compile` | Unmasked reason |
|---|---|---|
| `burst_prop_spec` `radix_prop_spec` `trie_prop_spec` `trie_unit_spec` | native | — |
| `burst_unit_test` `radix_unit_test` `trie_unit_test` `tst_unit_test` | fallback | **L-DO** (below) |
| `trie_prop_test` | fallback | **L-EACH** |
| `trie_smoke_test` | fallback | **L-JOIN** |

Every "code-body word X (Stage 2)" string is the closure-probe mask
(`callable_words.go` `recordClosureDispatch` → the probe's `probe.Reason` is
swallowed on decline). Unmasked by an env-gated print of `probe.Reason` at the
`!probeOk` decline (temporary, reverted). Each reduces to one leaf:

---

## L-DO — fallible multi-value `do` under a catch (variable arity)

Surfaces in the four `*_unit_test` codec-roundtrip cases:

```boru
def msg (do [ ((s TrieSet.encode) TrieMap.decode) "no-raise" ]
         error [ var [[e] convert String (e "message" get) ] ])
```

The `do` body nets **2** values on no-raise (`[decode-result, "no-raise"]`) but
**1** Error on raise (`do` catches into one Error). `error`'s sig is
`[TList, TAny]` (BarrierPos 1) — it consumes exactly ONE stack value (the
do-result) plus the handler, so the no-raise path leaves a **stray**
decode-result on the stack below `error`'s result while the raise path leaves
none. The static VM stack model can't seat a region whose depth depends on
whether a runtime raise fired. This is the refusal `doListReturnsFn` now emits
("do: fallible multi-value body under a catch (variable arity — Stage 3)"); the
pre-fix behaviour was a `STORE_LOCAL` underflow (seating the 2-value residual at
a fixed slot count that the 1-value caught path underflowed).

**What compiling it needs.** A variable-arity residual model for `do`/`error`:
the do-unit RETs its actual residual (already count-agnostic — see
`compileClosureBody`'s `BodyOutResidual` declared-nil path), and the
`do…error` lowering must track a runtime-variable stack depth across the
catch merge (or normalise the do residual to a fixed count before `error`
consumes it — e.g. an explicit "keep top-N" the emitter inserts when the
consumer arity is known). Not a bug; a genuine Stage-3 VM feature. Highest
leverage — closes 4 suites.

---

## L-EACH — forward-operand accounting across a dynamic/island residual

Surfaces in `trie_prop_test`'s `each`. Refused by
`engine.go::refuseForwardStackDrift` ("forward operand accounting across a
dynamic/island residual (Stage 3)"). This is a **soundness guard** — it stops a
miscompile, which is the one thing worse than a refusal — but it is still an
open defect: it refuses when the checker's all-stack
operand match (forced because the top-of-stack operand is `Dynamic`, blocking
the narrower forward overload) would DIVERGE from the interpreter's runtime
forward collection once that operand is concrete. Refusing beats miscompiling,
but it is not a resolution — the row does not compile, so the defect is open.
Compiling it requires modelling the dynamic-operand forward
collection in the compiled path so check-mode and the VM agree on which trailing
literal the word grabs — a real accounting feature, and one this leaf is owed.
One suite.

---

## L-JOIN — recursive branch-join accumulator, operand provenance (FULLY TRACED)

Surfaces in `trie_smoke_test` via `TstSet.longest-prefix` → the recursive
`longest-t` (and `radix.boru`'s `node-at`). Minimal off-corpus repro:

```boru
def rec fn [
  [nd:Any key:Any consumed:Any best:Any] [Any] [
    if (nd eq none) [best] [
      def pc (consumed "x" add)
      def best2 (if (nd "end" get) [pc] [best])   # join of pc & best
      best2 consumed key (nd "mid" get) rec        # self-call: best2 → best slot
    ]
  ]
]
(rec none "hi" "" none) print end
```

Refused by `RecordUserCall` → `resolveOperand` fails on the `best2` operand
("fn call operand of unknown provenance"). It is NOT in `producedBy`,
`localByID`, or `capID`.

**Pointer/id trace** (temporary instrumentation of `RecordDynBind`,
`RecordBranch`, and the `RecordUserCall` decline; reverted). For one compiled
unit of `rec`, the SAME source `def best2 (if …)` binds repeatedly with
different ids AND types, and the branch records repeatedly, because the
recursive-return fixpoint re-analyses the body several times:

```
DEF best2  id=T_314…  parent=Any            # iteration A (pre-join)
BRANCH     seq=5  out=T_497…  parent=Disjunct   # producedBy[T_497…]=5
DEF best2  id=T_497…  parent=Disjunct       # best2 := branch out (matches)
BRANCH     seq=12 out=T_ef4…  parent=Disjunct   # producedBy[T_ef4…]=12
DEF best2  id=T_ef4…  parent=Disjunct       # best2 rebound (next iteration)
PROVFAIL   argi=3  id=S_dad…  parent=Integer prod=false local=false
```

The self-call's `best2` operand is captured as `S_dad…(Integer)` — an id/type
from YET ANOTHER iteration (the carrier-analysis evaluation of `if`, whose
`JoinCarriers` mints its own id via the global `GenerateID` counter and infers
`Integer` in that iteration's type context), matching NONE of the branch outputs
recorded in `producedBy` (all `T_…` Disjuncts). So the operand and the
provenance record come from **different fixpoint iterations with independently
minted ids and divergent inferred types**.

Why the simple cases compile: when the two `if` arms have the SAME parent type,
`JoinCarriers` (`carrier.go:2881-2891`) collapses and mints one id, and the
carrier pass and emit pass converge on it; when the arms differ (here
`pc:String` vs `best:None` → `Disjunct`, later narrowed to `Integer`), the
passes diverge in both id and type. `GenerateID` is a global monotonic counter
(`value.go:1412`), so it is non-deterministic across passes — but making it
deterministic is NOT sufficient, because the arm ids and inferred types
themselves differ per iteration.

**What compiling it needs.** Either (a) unify the operand-capture pass with the
provenance-recording pass for recursive bodies (one fixpoint iteration is the
source of truth for both the value flowing to the self-call and the
`producedBy` entry), or (b) make the whole recursive-body re-analysis
identity-stable across iterations (stable value ids AND a converged join type
before operands are captured). Both are core recursive-analysis /
`EmitState.producedBy` changes with real regression surface against the
byte-identical differential; a fix must ship with an off-corpus
`RunCompiledStrict`-vs-`Run` regression on the repro above. This is the only
one of the three that blocks LIBRARY code (`tst.boru`, `radix.boru`), not just a
test file — and it is a genuine provenance BUG, not a missing feature.

---

## Order of attack (leverage / risk)

1. **L-DO** — 4 suites, self-contained to the do/error lowering + VM residual
   model. Highest leverage.
2. **L-JOIN** — 1 suite but the only LIBRARY-code blocker and a real bug;
   requires recursive-fixpoint provenance coordination.
3. **L-EACH** — 1 suite; dynamic-forward-collection accounting.

Each must land behind: `verify-bytecode` (byte-identical differential),
a hand-pinned off-corpus `RunCompiledStrict`==`Run` regression (the differential
is structurally blind to these recursive-trie / fallible-do / dynamic-each
shapes), the trie `--compile`==interpret sweep (green output, not merely
"compiles"), and `cover-gate` 100%.

---

## L-DUP — a chained whole-program duplication miscompile (found while probing)

Attempting the SAFEST possible way to close L-JOIN — a behaviour-preserving boru
restructure of `longest-t` (push the `end`-node choice into the arms, so no
branch-join is fed to the recursion) — makes `tst.boru`'s `longest-t` compile
natively (verified in isolation). But in the FULL `trie_smoke_test.boru` (four
imports, ~30 top-level statements, a mix of now-compiling and still-falling-back
sections) it converts a hidden, contained refusal into a broken compile: `--compile`
emits the **entire program output twice** (61 lines vs the interpreter's 31),
while `--no-compile` is correct. Reverted immediately.

This is the chained-leaf hazard from `.1`/`.0` again: clearing an outer refusal
unhides a deeper one. **Root cause now confirmed** (`--force-compile` on the
refactored full smoke): the compiled run prints every section, then raises
`[boru/internal_error]: bytecode: internal: dynamic-scope read miss for` \`np\`
at the BurstMap `decode` section (`src 261:13`) — a NEW deeper leaf (**L-NP**: a
fold/var-body local `np` in `burst.boru` misses under the compiled dynamic-scope
path, the same class the `.1` critical-finding flagged). `RunCompiledReason`
(`lang/go/boru.go:750-768`) catches that runtime bail via `runtimeShouldFallback`
and re-runs the WHOLE program on the interpreter — but `RestoreForCompile` only
rolls back registry SCOPES, it cannot un-`print` the compiled run's
already-emitted output, so every side effect is doubled. On the CURRENT library
this never fires because L-NP is masked by an EARLIER compile-time refusal
(L-JOIN); clearing L-JOIN unmasks L-NP, whose runtime bail then duplicates. The duplication is **structure-dependent** — it does NOT
reproduce with the four imports + the tst section alone, nor with a minimal
recursive-fn + a few prints; it needs the fuller mix of compiled and
fallback top-level statements. So it is a program-level (top-level residual /
partial-compile) miscompile, distinct from L-JOIN's operand provenance, and it
is a HARD `compile==interpret` violation that must be fixed before L-JOIN can be
closed by EITHER a compiler fix or a library refactor.

**Consequence for sequencing.** "Fully compile the trie suites" is not a matter
of clearing three independent leaves; it is a CHAIN per program (as `.1` L0-L7
already found for the other repos), and each newly-compiling region can unhide a
deeper miscompile the fallback was masking. Every step must re-run the trie
`--compile`==`--no-compile` byte-identical sweep over the WHOLE suite (not just
the changed construct) plus `verify-bytecode`, because the differential corpus
is blind to these multi-section program shapes. The current
4-native/5-refused state is byte-identical everywhere; it must stay so at
every step. Byte-identical is the floor, not the goal: the five refusals are
five open defects, and the state is only done at 9-native.


**Two distinct fixes are entangled here.** (i) L-NP — make the fold/var-body
`np` local resolve under the compiled dynamic-scope path (or refuse it at compile
time, restoring a clean pre-output fallback). (ii) The `RunCompiledReason`
runtime-bail fallback is itself unsound for side-effecting programs: once the
compiled run has emitted observable output, re-running the whole source
DUPLICATES it. A sound contract must either prove the program compilable
end-to-end BEFORE any effect (so a runtime bail is impossible), or buffer/guard
effects, or propagate the internal_error instead of silently re-running. The
differential corpus is pure-value, so it never exercises the emit-output-then-bail
path — which is why this ships latent.
