# Which compile refusals are still live — a measured survey

This note answers one question with evidence rather than with the refusal
strings in the source: **would a bytecode opcode that performs
interpreter-identical signature matching remove the remaining compilation
refusals?**

The answer is **no**, and the interesting part is why: that opcode already
exists, it already did its job, and not one of the refusals still reachable
today is a dispatch-resolution problem.

Measured against a binary built at `1a44dbb`. Every row below is a program
that was actually run; `REFUSED` means the default path printed
`bytecode compilation refused, ran on the interpreter (slower): …`.

> **Scope warning.** Each row is ONE specimen, not a class proof. A shape
> that compiles in this spelling may still refuse in another — these
> results bound what is *definitely* still live, not what is definitely
> fixed. Read `design/COMPILABLE-SUBSET.md` §5 for the intended rules; it
> is the statement of intent, this is a snapshot of behaviour.


## 1. The premise: runtime signature matching is already in the VM

Three opcodes do it today:

| Opcode | What it does |
|---|---|
| `OpCallNativePoly` | "the SAME first-match the interpreter takes — then calls the matched handler … so the VM selects faithfully at run time instead of islanding through a sub-engine (plan P3)" |
| `OpCallUserPoly` | the user-fn mirror: bakes every same-arity overload's body unit, VM re-runs `MatchSignature` at entry |
| `OpDispatchRematch` | terminal re-match hand-off |

`sigTypeMatches` (`signature.go:269`) is the single funnel for "the
interpreter's `matchSignature` arms, `positionalMatch`, **the VM's
guarded-native contract and poly re-match**" — one predicate, both engines.

It worked. `islandCeiling` fell 102 → 36 → 29 → 26 → 15 → … → **0**, with
poly re-match credited at P3 and P5, and `refusalCeiling` is **0**: no spec
row refuses. Dispatch resolution was the previous frontier and it has been
crossed.


## 2. What the four "no poly re-match" strings actually mean

They read like wiring gaps. They are not.

| Source | Reality |
|---|---|
| `callable_words.go:253` | Wired. A `CompileDynBody` word **declines into** `tryRecordDynBody`, which polys. Only a word lacking that effect refuses. |
| `core_helpers.go:1262` | Wired. `planUserPolyDispatch` calls `tryCompileUserPolyArms`; the message fires only when the **bake declines an arm**. |
| `core_helpers.go:1264` | Wired. Same path, predicate-hazard trigger. |
| `carrier.go:2422` | **Not a refusal site.** A doc comment on `dynamicReachableOverloadCount` describing the first row. |

"no poly re-match" means *poly re-match was attempted and could not be
constructed for this shape*. Verified — the canonical shapes those comments
describe all compile:

```
each   over a gradual-Any collection   COMPILES
fold   over a gradual-Any collection   COMPILES
scan   over a gradual-Any collection   COMPILES
filter over a gradual-Any collection   COMPILES
fn-predicate overload dispatch         COMPILES     (the comment's own `classify -3`)
multi-overload user fn, gradual arg    COMPILES     (the comment's own `g (id 5)`)
```


## 3. The live refusals

### 3a. Full-stack words — the largest class

```
depth                     REFUSED: residual value of unknown provenance
1 2 depth                 REFUSED: residual value of unknown provenance
1 2 (depth) add 0         REFUSED: operand of unknown provenance … at add
1 2 3 pick 1              REFUSED: residual value of unknown provenance
1 2 3 pick 0              REFUSED: residual value of unknown provenance
1 2 3 roll 2              REFUSED: residual value of unknown provenance
```

Contrast the fixed-arity shuffles, which compile: `swap`, `rot`, `over`.

**Not a matching problem.** `depth` alone — no overloads in play, nothing
ambiguous — still refuses. The blocker is §2 of `COMPILABLE-SUBSET.md`:
every dispatch argument must resolve to a known operand, and a full-stack
word's result has no producing event the compiler can address. A
signature-matching opcode has nothing to contribute; what this needs is a
lowering that models the word's output as an event with provenance.

> **GRADUATED (2026-08-03)** for provably-exact stacks:
> `EmitState.FoldFullStack` statically folds the dispatch — depth bakes
> the count as a concrete const, pick re-pushes the picked entry
> (event targets promoted to value-def locals), roll re-seats the true
> permutation — no opcode, no event, the elision IS the lowering. Rows
> in `lang/spec/corpus-core.tsv`; the remaining sub-frontier (a roll
> permuting two EVENT results — beyond the Stage-1 residual ordering)
> is ledgered in `frontier-full-stack.tsv`. Inexact contexts (variadic
> regions, dynamic n, nested frames, out-of-range n) still refuse — they
> are the open remainder of this leaf, not its settled edge.

### 3b. Multi-overload user fn whose arms declare different returns

```
def id fn [[x:Any][Any][x]]
def g fn [[a:Integer][Integer][1] [a:String][String]["s"]]
g (id 5)
→ REFUSED: gradual-Any arg to multi-overload user fn `g`: ambiguous dispatch, no poly re-match
```

Also fires for `Integer` vs `Any` returns. The identical program with both
arms returning `String` **compiles**, as does the 3-overload version, the
2-param version, and the recursive version.

The gate is `userPolyArmShapeOK`, which requires every arm's `Returns` to
match the committed contract. It is deliberate: the call site records ONE
output type, so mismatched arms make that record a lie for whichever arm
the VM selects, and downstream compiled code was typed against it.

Closing it means recording the call's output as the **join** of the arms'
returns. That is principled but hands downstream a dynamic value, which is
itself a refusal trigger — so the net coverage change could be zero or
negative, and it modifies the return-typing model that produced two defects
in `verse-report-defects-investigation.0.md` (C's arity widening, F's
dropped return pattern).

> **GRADUATED 2026-08-03** (completeness-review §9.11): the join landed —
> `userPolyPlan.outs` records a dynamic carrier at the arms' common
> ancestor, `userPolyArmShapeOK` relaxed to count + nil-ness agreement,
> and the first-match-partition widening now PRESERVES the recorded
> identity (that orphaning, not the join itself, was the real blocker).
> Both shapes above compile natively; the risk note held — no
> previously-compiling row regressed. Arms with differing return COUNTS
> keep the refusal (`bytecode_poly_join_test.go`).

### 3c. Multi-overload user fn with a quoted (`Atom/q`) param slot

```
def g fn [[a:Atom/q][String]["atom"] [a:Integer][String]["int"]]
g (id 5)
→ REFUSED: … no poly re-match
```

Same gate, different clause: "the runtime window re-match binds plain
values only".

> **GRADUATED (measured 2026-08-03):** this class now COMPILES — the
> `a:Atom` + `a:Integer` twin over `(id 5)` runs natively, pinned as
> the control row in `lang/spec/frontier/frontier-poly-join.tsv`. §3a,
> §3b and §3d remain live and are now ledgered
> (`frontier-full-stack.tsv`, `frontier-poly-join.tsv`,
> `frontier-do-error-arity.tsv`) so their graduations are measured.

### 3d. `do […] error […]` + a trailing expression

```
do [1 div 0] error [drop]
2 add 3
→ REFUSED: error: handler nets no value — the single-output island model would leave the stack one short
```

Deliberate, and recent — this is the C fix in
`verse-report-defects-investigation.0.md`. It replaced a leaked
`internal_error`. Note it cannot become a spec row under either state (it
refuses fixed, miscompiles unfixed), which is why its regression lives in
Go tests.


## 4. Specimens that COMPILE though §5 lists their class as refusing

Recorded because they suggest §5 has drifted, not because any one of them
proves a class is closed:

| Specimen | §5 entry it sits under |
|---|---|
| `def g fn [[a:Integer][Integer][args.0]] g 5` | context-dependent word (`args`) |
| `def ifu (usurp if)  ifu true [1] [2]` | quoted-operand word |
| `def a2 (force-arity 2 add)  a2 1 2` | quoted-operand word |
| multi-overload fn built inside a fn (captures) | `owner.Captured != 0` decline |
| zero-return arms over unnamed params | zero-return residual decline |
| `def nd (m dot f) print "after" nd` | mid-body dynamic fn apply |
| `import "boru:math-util"` then `MathUtil.sqrt` | compile-time word |
| a `macro` expansion | compile-time word |

`COMPILABLE-SUBSET.md` says of itself: "If they disagree, the code wins and
this page is stale — fix it." These rows are the evidence that some of §5
is now stale; confirming each class (rather than each specimen) is the work
that would justify editing it.


## 5. Conclusion

Of the four refusal classes still reachable, **none** is a
signature-matching failure:

- 3a is provenance and lowering;
- 3b is the return-typing contract;
- 3c is operand shape at a quoted slot;
- 3d is the island's arity model.

A general "match like the interpreter" opcode cannot reach any of them, for
a structural reason: matching operates on operands already on the stack,
and these refusals are about not being able to *put* an operand there, or
about not being able to *type* the result once it is. Forward collection
compounds it — `eng/go/CLAUDE.md` describes gathering as "a plan-time token
walk (`resolveForwardArgs` + `matchSignature`)", and compilation is
precisely the act of discarding that token stream. This is why
`OpCallNativePoly` takes "Arity values on top of the stack": arity is
already fixed, and it re-picks the **overload**, not the **argument set**.

Ranked by what each would buy:

1. **3a, full-stack words** — the only class with several distinct words
   behind it, and the only one whose fix (an event-producing lowering for
   `depth`/`pick`/`roll`) adds a capability rather than relaxing a
   soundness gate.
2. **3c, quoted slot in a poly arm** — narrow, and plausibly a small
   extension to the arm-shape rule.
3. **3b, mismatched arm returns** — real, but its fix trades against
   downstream dynamic-input refusals and touches the riskiest model.
4. **3d** — leave it; it is a deliberate refusal replacing a wrong answer.

With `refusalCeiling` at 0, none of these is measurable on the corpus
today, so any of them is a speculative widening: worth doing for programs
outside the corpus, not worth doing to move a gate.
