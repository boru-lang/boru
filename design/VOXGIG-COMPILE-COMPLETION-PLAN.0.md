# boru Bytecode Compiler — Completion Plan (adversarially-verified)

Supersedes the leaf framing in `VOXGIG-COMPILE-LEAVES.0.md`. Produced by a
15-agent review (3 architecture/roadmap maps → 6 per-leaf root-cause passes →
6 adversarial soundness reviews), every claim reproduced against a binary
built from the working tree (incl. the uncommitted leaf-#1 fix). The headline:
**the adversarial pass refuted the proposed quick-fix for 5 of the 6 leaves** —
each "read-side" fix either no-ops, over-refuses, or silently miscompiles, and
in every case the langspec differential is *blind* to the divergence.

---

## 0. What "complete" means — two surfaces, one contract

| Surface | Live state | Is it the frontier? |
|---|---|---|
| **Internal langspec census** | `refusalCeiling=0`/`islandCeiling=0` reached historically; **31 rows** refuse now (`COMPILED_STATUS.md`: 3520 rows / 3286 compilable / 3255 native / 0 islanded / 31 refused). These ratchets are **informational, not gates** (`compiled_coverage_test.go:170-185`). The count tracks corpus *growth*, not regression. | No — byte-identity holds here, but the 31 refusals are open defects, not a settled state; Stage J (P7) is the formal endpoint. |
| **External voxgig `--force-compile`** | **11 PASS / 28 REFUSE** (8 passes are pure-def libs; only 3 test files compile: bloom/decision/stats `_prop_test`). | **Yes — this is the live completion target.** |

**The hard contract: every valid program compiles, and compiled output is
byte-identical to interpreted output.** Byte-identity is enforced by `make
verify-bytecode` (`TestSpecCompiledDifferential` / `OrFallback` /
`TestPropertyDifferential` / combination matrix / `-race` / `-tags borudebug`)
plus `crossdiff` (Go-vs-TS) and `test-ts`, all currently green. A refusal
breaches the first half of that contract: it is a **defect** — an unimplemented
or unproven case, owed a fix and tracked to closure — never an advisory outcome
and never a second, sanctioned branch. `RunCompiled`/`RunCompiledReason`
currently re-run a refused program on the interpreter so the user still gets an
answer; that is **scaffolding absorbing a known defect**, not something the
design leans on. A *miscompile* is the more dangerous defect, because it is
silent and returns a wrong answer — but a refusal is a failure too. Done is a
language that compiles, as a developer expects: all valid code, no exceptions.

Where a sound compile is not yet implemented, refusing is the required
containment (a tracked defect beats a silent wrong answer), which is why
several regressions below pin "must still refuse" shapes. Those pins guard
against miscompiling; they do not make the refusal a target state.

### The discipline that dominates this whole effort

**The langspec differential cannot see voxgig shapes** (recursive tries,
captured comparators, check-prop bodies, union receivers are absent from the
curated corpus). The leaf-#1 read-side-only miscompile (`i mul a` → `[0 5 10
15 20]` vs `[0 4 8 12 16]`) passed `verify-bytecode` clean and was caught only
by a hand-written `RunCompiledStrict`-vs-`Run` test. Therefore:

> **Every leaf fix MUST ship a hand-pinned `RunCompiledStrict == Run`
> regression for its exact off-corpus shape, AND (for coverage fixes) a
> `prog.Disassemble()` assertion that no FALLBACK island appears** — because
> `--compile` **silently** absorbs a refusal by re-running it on the
> interpreter, so a parity test alone passes on the interpreter and hides the
> defect — and the silence is what makes it dangerous rather than harmless:
> nothing in the run says the compile failed (leaf-3 finding).

Gate every commit: `make fmt && make vet && make lint && make test`, then
`make verify-bytecode` + `crossdiff` + `test-ts` at 0 divergences, then re-run
the voxgig `--force-compile` sweep to measure files flipped.

---

## 1. The central structural fact: Stage D is the bottleneck

The 28 refusals are a *chain* per file — the named refusal word
(`each`/`fold`/`test-test`/`check diagnostics`) **masks** the real inner leaf.
Mapping the live chains shows one mechanism first-gates the large majority:

**Stage D — runtime-native re-dispatch over a dynamically-typed receiver**
(`get`/`set`/`push`/`raise` over a `Dynamic`/`Disjunct`/`Any` carrier). Both
leaf 4 (trie/burst/radix) and leaf 5 (`check diagnostics` smokes + trie fold)
bottom out here. It first-gates ~14–20 of the 28 files. **Difficulty 5.**

The adversarial pass corrected the doc on the machinery and the sites:

- **`OpCallNativePoly` already exists** and `tryRecordPoly` already admits
  `get`/`getr`/`set` over dynamic receivers (`carrier.go:1015`), with the
  mixed-arity 0-vs-1 `set` model (`carrier.go:562-601`) and `PolyRef.Reg`
  sub-registry threading — with passing tests. **No new `OpCallDynamic`
  opcode is needed** (`OpCallDynamic` applies a runtime *function value*, a
  different thing). The doc's "needs OpCallDynamic" is a misdiagnosis.
- The actual blockers are **gating/reachability**, each its own soundness sub-hazard:
  1. **`get` over a strict `Disjunct` receiver** (e.g. `nd: Map|Any` from an
     `if`-union): the recovery discriminator `anyAnyCarrier` (`engine.go:6636`,
     requires `Parent==TAny`) rejects it, so it falls to the unconditional hard
     refuse at `engine.go:6663` **without ever reaching `tryRecordPoly`**. The
     fix site is the *discriminator*, not `tryRecordPoly`.
  2. **`get` latches under a SUSPENDED re-dispatch**: the inner `(nd "kids"
     get) get (ch)` refuses while the outer recovery runs `carrierResults`
     under `Suspend()`, so `es.active()==false` short-circuits the entire
     `engine.go:6633` gate. Widening the carrier predicate is unreachable code
     here — this needs a control-flow rework of the suspended recovery.
  3. **`set`/`push` of a fn-body list literal** (`[key val]`, `[label child]`):
     the real provenance gate is **`e.isTop` in `autoEvalList`
     (`engine.go:3171`)**, NOT `RecordMakeList`'s `frames!=1` guard
     (`emit.go:2181`, which is dead code for this shape). Relaxing `e.isTop`
     naively **miscompiles** `def y (x add 1) [y y]` → compiled `[6,6]` while
     the interpreter raises `undefined word: y` (a def-bound local does not
     resolve inside a nested list literal). The fix must distinguish a
     data-list over live locals from (a) a def-local reference and (b) a
     stateful/NoEval generator body.
  4. **`raise` over dynamic** (decision `apply-op` catch-all): a *diverging
     terminal* (`CompileDiverges`, `Returns:[]`). A value-returning poly
     re-dispatch loses the divergence contract and mis-models the enclosing
     `if`'s branch-result arity. Needs divergence-aware branch modeling.

Because the three leaf-5 files and four leaf-4 files bottom out at **different
sub-mechanisms** (the leaf-5 "single shared bottleneck" claim was refuted),
Stage D is not one change but a small cluster: discriminator widening +
suspend-recovery rework + sound fn-body list provenance + divergence modeling,
each landed with its own `RunCompiledStrict` regression.

---

## 2. Leaf-by-leaf (corrected difficulty / dependency / leverage)

Difficulty and leverage below are the **adversarially-revised** values.
"Flips" = files that turn PASS from this leaf *alone*.

### Leaf 1 — `each`/higher-order body reads enclosing computed def — diff 4, flips 0
- **In-tree fix is SOUND and confirmed** (capID read-side + closureCaps
  value-passing; the carrier-ID collision the docs feared does NOT reproduce —
  2-level nested-loop capture runs `[0,6,12,18,24]` == interpreter). **Commit it.**
- Residual is a *different* bug: `ComputeCaptures` (`fn_capture.go:148`)
  mis-captures a `def` bound **inside** the body (Depth>baseline) as an
  enclosing capture → `resolveOperand` unreachable → refuse.
  - **Part 1 (the over-capture)**: exclude body-local def-names like
    `sig.Params` are. **Adversarial catch**: a naive `WalkBodyWords`-style walk
    *over-excludes* — it descends into nested closure bodies (not yet
    `FnDefInfo`), dropping an inner `def X` from the *outer* capture set and
    **refusing the bubble/insertion-sort shapes it claims to flip**. The walk
    must stop at higher-order-body boundaries (which aren't marked — non-trivial).
  - **Part 2 (comparator application `(a b comp)`)**: this is **byte-identical**
    to the documented `((m.g 3) add 1)` = 8-vs-7 reorder miscompile
    (`engine.go:5616`, Stage G "dynamic value precedes args"). No static
    predicate distinguishes a trailing captured-comparator apply from a leading
    dynamic-fn-value apply for an `Any`-typed array element. **This is the
    unsolved difficulty-4 core** and is required for every comparison sort.
- **Vacuous-test warning**: `each` returns its input collection, so a
  `(3 f)`==`iota 3` test is blind to a body miscompile — **verify through
  `fold`/`scan`**, not `each`.
- Blocked by leaf 4 + leaf 6; flips 0 alone. Land as a langspec-gated
  *correctness* step, not a file-flipping unit.

### Leaf 2 — computed `do{}`/map + narrowed-operand provenance — diff 3, flips 0
Splits into two unrelated, independent fixes:
- **STATS (`fn call operand of unknown provenance`)**: `is`-narrowing
  (`ApplyGuardNarrowing` `carrier.go:2516`, `ApplyComplementNarrowing` `:2556`)
  mints a **fresh `Value.ID`** for the narrowed binding via `NewCarrier`, losing
  the source provenance. **Clean read-side fix** (sound — narrowing is a
  static-only refinement, runtime value is identical): preserve
  `cur := r.Defs.Top(c.Name); narrowed.ID = cur.ID`. Must inherit *full*
  provenance (seq+idx), and pin **both** branches carry the true runtime value.
- **BLOOM (`...at do`)**: `recordMakeListInner`'s builtin-only producer gate
  (`emit.go:2213`) refuses a do-map list element produced by a *module* word
  (`ArrayUtil.where`), even though the bare-value path already accepts it
  (`{set: set-idxs}` compiles, `{set: [set-idxs]}` doesn't, identical output).
  Relax to "resolves to a live `opEvent`" — but **must still refuse a stateful
  generator** (`list-of [Rand.int 0 10] 3` must not bake 3 identical rolls).

### Leaf 3 — binary-op operands not adjacent (bloom `count`) — diff 3, flips 0
- **NOT already done** (the root-cause's "difficulty 1, cleared" was refuted):
  `bloom.boru` passes only because `bloom-count` is exported as a fn-*value*
  (`bloom-count/r`) and **never called** — an uncalled fn body is never lowered.
  Called, it refuses live.
- Real cause: `md`/`kd`/`xd` live inside an `if` **else-arm fragment**, where
  `valueDef`-promotion (`planValueDefLocals`) **never runs** — it's invoked only
  at unit top-level (`StartFnCompile`, `Finalize`), never on inline fragments.
- **Fix**: run value-def promotion on inline fragments
  (`lowerFragment`/`lowerBranch`/`lowerLoop`), allocating from the enclosing fn
  unit's frame-local namespace and emitting the store at the producing event.
  Surfaces in `bloom_smoke`/`bloom_unit_spec` *after* leaf 2 clears.

### Leaf 4 — dynamic-receiver `set`/`push`/`get` — diff 5, flips 0
See §1. Both proposed edits were proven **inert** (no-ops); the true sites are
`autoEvalList` `e.isTop` and the suspended-recovery `active()` short-circuit,
each with an unguarded miscompile. The deepest leaf.

### Leaf 5 — `fold` provenance + the `check diagnostics` gate — diff 4, flips 3
- **The `check diagnostics` mystery is fully solved**: `CompileCheck`
  (`boru.go:297-301`) sets `Check.Emit` + `Compiling`, which **re-enters every
  fn body at each call site** with actual carrier args (to lower it). `boru
  check` (`Emit==nil`) analyses each body once at its `def` site and never
  re-enters → 0 errors. When a re-entered body does `get`/`raise` over a dynamic
  receiver, `checkModeAssumeSig` emits a `SeverityError` `no_signature`
  diagnostic, and the gate refuses with the generic **"check diagnostics",
  masking the real reason** ("unmatched dispatch recovered at get/raise").
- **Part 1 (unmask) — cheap, sound, flips 0, real DX win**: don't add the
  error-severity diagnostic when `es.active()` (`MarkUncompilable` has already
  refused — the defect is recorded and the interpreter path absorbs it; the
  diagnostic is redundant and spurious). Then the true reason surfaces.
  **Adversarial caveat**: the fell-through refuse (`engine.go:6663`) fires for
  *both* recoverable union-receiver dispatch and genuine concrete type errors
  — scope the suppression carefully so `boru check`
  still reports real top-level unmatched dispatch.
- **Part 2** is **Stage D** (the file-flipper) — see §1. The three files bottom
  out at three *different* sub-surfaces (tst_unit = Disjunct-receiver `get`;
  decision_smoke = diverging `raise`; trie_smoke = closure-probe decline), not
  one shared leaf.
- **Part 3** (decision_smoke only): a residual `find-node: body result of
  unknown provenance` (Stage A) surfaces after Stage D.

### Leaf 6 — property-test framework (`test-check-prop`) — diff 3, flips 2
- **Sole cause**: `noEvalBodiesInert` (`emit.go:2392`) rejects a check-prop body
  containing an **interpolated string** `` `...${expr}...` `` — `isInertConst`
  has no `InterpStringPayload` case → `false`. bloom/stats/decision `_prop_test`
  compile only because their bodies are inert-const data. Verified: patching the
  interp strings out makes **sort_prop_test compile and trie_prop_test compile +
  run green** — so this leaf genuinely **flips 2 files** (sweep 11→13).
- **Adversarial catch — naive fix SILENTLY MISCOMPILES**: admitting
  `InterpString` as inert lets a check-prop nested in a **compiled fn** whose
  body interpolates a **frame local** (`` `${pfx}` ``) bake as data and
  re-interpret against the *registry*, where the VM frame local is absent →
  `undefined word: pfx`. Reproduced: interpreter `true`, compiled `false`. The
  proposed `valueRefsName` guard does **not** cover it (gated on
  `sig.Callable!=nil`; `test-check-prop` is `Callable==nil`).
- **Sound fix**: either (a) reuse leaf-1's frame-local set to refuse only
  *frame-local-shadowing* names inside the baked body, or (b) **scope the
  InterpString admission + check-prop bake to module/top-level scope** (where no
  enclosing frame local exists — the only context the soundness argument holds).
  Voxgig's check-props sit at module scope, so (b) flips the 2 files safely and
  is the lower-risk first cut. This couples leaf 6 to leaf-1 infrastructure
  (correcting the "independent" claim).

---

## 3. Langspec Stages A–J status (the internal surface)

LANDED: H, B, G(partial: `bytecode-combinations:74` + mixed-boundary +
patrun-store), D(partial: `reach:38`), I(macro→OpTrap), `def-node-binding:54`,
the three-tier P7 gate (`TestOnlyMetaFallsBack`). PENDING: **A** (variadic
branch/return), **C** (sound module-body compilation — the largest, needs a
corpus re-baseline), **D**(remainder: `module-io:29/30` + the voxgig
dynamic-receiver work), **E** (flex reference cells), **F** (dynamic-scope —
a tiering decision), **G**(remainder: `fn-value:19`, dynamic-precedes-args =
leaf-1 Part 2), **J** (delete the unbounded whole-program interpreter re-run;
the tier-1 `Vm.run` island stays as tracked containment, not as a sanctioned
exception). Tier-1 "irreducible" is currently **empty** — 29 of 31 live
refusals are reducible by a named limitation, and the remaining 2 are unmapped
defects, not exceptions.

**Scope recommendation**: drive the **voxgig real-world corpus** to full
compile as the completion target (it exercises everything the langspec residual
does, on real code). Treat Stage J/P7 as the formal *internal* endpoint to
declare afterward. Stage D is the shared dependency of both.

---

## 4. Recommended sequence (leverage ÷ risk, dependency-respecting)

Each step is its own gate-clean commit with the mandatory `RunCompiledStrict`
regression + no-FALLBACK-island assertion.

0. **Commit the in-tree leaf-1 fix** — sound, gated, locks in progress. (flips 0)
1. **Leaf 5 Part 1 — unmask `check diagnostics`.** Independent, sound, makes
   every downstream refusal name its true reason. (flips 0; DX foundation)
2. **Leaf 2 STATS — carrier-ID preservation.** Clean read-side, independent. (flips 0)
3. **Leaf 6 — InterpString, scoped to module-level check-prop (option b).**
   First real win: **sweep 11→13** (sort/trie `_prop_test`). Add the
   frame-local miscompile guard + nested-fn refuse regression.
4. **Leaf 1 Part 1 — body-local over-capture**, with the higher-order-boundary
   refinement (verified through `fold`/`scan`, not `each`). Correctness
   prerequisite for loop-heavy ops. (flips 0)
5. **Leaf 3 — fragment-level value-def promotion.** Unblocks bloom `count`. (flips 0 alone)
6. **Leaf 2 BLOOM — list-element opEvent gate**, with stateful-generator guard. (flips 0 alone)
7. **Stage D — the project** (leaf 4 + leaf 5 Part 2). Land as a reviewed
   cluster: (a) widen the no-sig recovery discriminator to `Disjunct`/`Dynamic`
   receivers; (b) rework the suspended-recovery `active()` short-circuit;
   (c) sound fn-body list-literal provenance (distinguish data-list / def-local
   / generator); (d) divergence-aware modeling for `raise`. **Unblocks the bulk
   of the 28** as the masking chains collapse. Highest payoff, highest risk.
8. **Leaf 1 Part 2 / Stage G — comparator `(a b comp)` vs `((m.g 3) add 1)`.**
   The unsolved static-predicate problem; required for comparison sorts.
9. **Re-sweep; clear residual per-file chain tails** (decision_smoke find-node
   provenance / Stage A; any remaining fold branch-result rows). Then declare
   Stage J/P7 on the internal surface.

**Why this order**: steps 1–3 are sound, low-risk, and deliver the only
available single-leaf file-flip (leaf 6) plus the diagnostic clarity that makes
the deep work legible. Steps 4–6 are correctness prerequisites that flip 0 alone
but must precede the chains. Step 7 (Stage D) is where most files actually flip,
deliberately scheduled after the cheap wins and after the discipline
(RunCompiledStrict + no-island) is in muscle memory, because its four
sub-hazards are exactly the silent-miscompile class the differential can't see.

---

## 5. Per-leaf mandatory regressions (the differential is blind to all of these)

- **Leaf 1**: a `fold`/`scan` body capturing an enclosing computed def
  (`bx`); a nested-closure inner `def` reusing an outer-captured name (must
  compile correctly, or refuse as a *tracked defect* — never drop the outer
  capture); the `((m.g 3) add 1)`==7 reorder pin after any `engine.go:5616`
  relaxation; a full comparator-driven sort ordering == interpreter; the 2-level
  `[0,6,12,18,24]` carrier-ID non-regression pin.
- **Leaf 2**: `if (x is List) [x size] [x]` over a union param — both branches;
  narrowed→user-call; a stateful `Rand` do-map element baked **once**; a
  generator list that **must still refuse**.
- **Leaf 3**: the `if` else-arm / loop-body `md…xd` interleave —
  `RunCompiledStrict==Run` **and** `prog.Disassemble()` shows no FALLBACK island.
- **Leaf 4/Stage D**: `[y y]` over a def-local must raise `undefined word`
  (not `[6,6]`); `get` over a `Disjunct(Map|None|List)` for present/absent/
  wrong-member; nested `get`-in-suspended-recovery; `set`/`push` re-assembled
  per call inside a fold; statement-position dynamic `set` must not over-collect.
- **Leaf 5**: `raise` over dynamic emits byte-identical Error AND honors
  `CompileDiverges`; `get` over a `None` receiver; a concrete non-container
  receiver must still refuse (guard the discriminator widening).
- **Leaf 6**: check-prop nested in a compiled fn interpolating a frame-local
  must refuse, with the interpreter path absorbing that defect (`ok=true`),
  rather than bake and miscompile (`ok=false`); module-scope
  check-prop referencing module defs / prop-local vars must still compile green.

---

## 6. Open decisions for the maintainer

1. **Completion target**: voxgig full-compile (recommended) vs. internal Stage J
   only vs. both. Drives whether step 9 ends at "28→0 voxgig" or "+P7 declared".
2. **Leaf 6 scoping**: module-only check-prop bake (lower risk, flips the 2
   files) vs. general frame-local-aware guard (more code, future-proofs nested
   check-prop). Recommend module-only first, generalize later.
3. **Leaf 1 Part 1b**: band-aid def-name exclusion vs. the "principled"
   check-mode body-local def cleanup (`defSnapshot`/`DefCleanupInfo`) — the
   latter must keep compiler/interpreter def-leak parity (`leak_q`=302==302) or
   it under-captures a real binding.
4. **Stage F / I sequencing**: dynamic-scope (`recursion:72`) and
   divergent-macro (`macro:45`) are the two hardest open defects — the question
   is when they get fixed, not whether ("permanent tier-1 interpreter-only" is
   not an available answer). Decide whether Stage J is declared with them named
   as outstanding defects or held until they compile — a decision needed before
   Stage J can assert tiers 2+3 at 0.

---

## 7. Progress log

Steps 0–3 (the sound, low-risk wins) landed, gate-clean
(`make fmt/vet/lint/test` + `make verify-bytecode` + the off-corpus
`RunCompiledStrict` regressions, all green). Voxgig sweep: **11 → 13 PASS**
(test-file compiles 3 → 5: `sort_prop_test`, `trie_prop_test` flipped).

- **Prereqs cleared** (pre-existing branch debt, found by the gate, fixed
  separately): a data race in `ForkConcurrent` (the Debug.stack `debugEngines`
  seam was not isolated per concurrent `await` fork — `808a8992`); two
  golangci-lint failures in `engine.go` (`ba57652a`).
- **Step 0** — leaf-1 in-tree fix committed (`d5d3a890`).
- **Step 1** — leaf-5 part 1 unmask (`62a6f7c3`). Subtler than planned: the
  discriminator is `Check.Compiling` (reset per-pass in `Begin`), NOT
  `Emit==nil`/`es.active()` — plain check arms a fresh `Emit` per fn body
  (`IsolateEmit`) and the latch fires under a suspended re-dispatch. The
  `make test` gate caught the first (over-suppressing) attempt via 6 checker
  tests, exactly the over-suppression risk the leaf-5 verdict flagged.
- **Step 2** — leaf-2 STATS carrier-ID preservation (`fc166326`). Clean
  read-side fix; flips 0 alone (the 3 files carry other chain leaves).
- **Step 3** — leaf-6 module-scope check-prop interp-string (`faad5c2f`). First
  file-flip (+2). The naive admission miscompiled a nested too-deep
  `macroexpand` (a standalone ParenExpr is not bakeable data); fixed by mirroring
  `isInertConst` at the top level and admitting InterpString only in member
  recursion. The fn-scope frame-local hazard the verdict warned about is guarded
  by keeping `isInertConst` strict + gating the admission on `len(es.units)==1`.

- **Leaf 3** — fragment-level value-def promotion (`8635e255`). Deeper than
  rated: the promotion decision had to flatten unit + inline-fragment events, and
  three branch/loop-modeling constraints each caught by an existing test the
  first cut regressed — exclude the fragment RESULT event (arm-result + tail
  call), gate fragRef on !fragInternal (tail-call ARG), recurse only into
  single-result fragments (residualN<=1, the variadic arm). bloom-count compiles;
  flips 0 alone.
- **Stage D, 1/4** — poly-recover get/getr over a union (Disjunct) receiver
  (`d8eb3d39`). Widened the no-signature recovery discriminator (anyDisjunctCarrier)
  so a union receiver records OpCallNativePoly (runtime re-match) like the Any
  case; concrete non-container receivers still refuse. tst_unit advances past
  `get`; flips 0 yet (the trie files hit the suspend hazard).

- **Stage D, 2/4** — don't latch a refusal under a suspended re-dispatch
  (`6e3c089e`). The trie inner-get no longer prematurely MarkUncompilable's the
  program; trie advances `get` → `find-kid` (a user-fn-over-dynamic leaf).
  Soundness verified by a 413-program adversarial sweep + a structural proof
  (under suspend zero ops are recorded). Flips 0 alone.
- **Real miscompile FIXED** (off-plan, found by the 2/4 adversarial sweep) —
  runtime param-type guard at CALL_USER (`7c728648`, `1f28b9b7`). A gradual-Any
  value laundered into a concrete user-fn param ran the body unchecked
  (`compile==interpret` VIOLATION); now guarded via `CompiledFn.Params` +
  `sigTypeMatches`, mirroring OpRet's return-check. Higher value than the
  refusal leaves — a live soundness violation. One documented residual
  (inline-`Pattern` params). Verified by a second adversarial sweep that caught
  (and drove the fix of) an `Options`-param over-raise regression.

**Remaining** (each a focused effort): **Stage D 3/4** set/push fn-body
list-literal provenance (the autoEvalList `e.isTop` gate, with the
`[y y]`-over-def-local miscompile hazard); **Stage D 4/4** raise over a dynamic
operand (divergence-aware branch modeling, CompileDiverges); the trie `find-kid`
user-fn-over-dynamic leaf; closing the inline-`Pattern` param-guard residual; then
leaf-1 part 1, leaf-2 BLOOM, leaf-1 part 2 / Stage G comparator. Stage D is where
the bulk of the 26 files flip, as the masking chains collapse.

## 8. Methodology note — adversarial workflows earned their keep

The compile==interpret differential is structurally BLIND to off-corpus shapes
(the voxgig tries, laundered params). This session, hand-written `RunCompiledStrict`
regressions + adversarial-sweep workflows caught what the differential could not:
a real pre-existing miscompile (param-guard-skip), a regression the param guard's
first cut introduced (the `Options` over-raise), and confirmed the suspend-skip
sound across 413 programs. Treat the differential as necessary-not-sufficient;
gate every dynamic-dispatch / VM change with off-corpus adversarial coverage.
