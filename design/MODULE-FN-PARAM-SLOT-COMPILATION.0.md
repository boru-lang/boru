# Module-fn param-slot compilation — focused follow-up plan

Goal: let a CLOSURE inside a MODULE-fn body capture that fn's params (notably a
`[comp:Function]` comparator) so the comparison-sort algorithms force-compile.
Currently they refuse at the outer `each` with "unreachable capture comp". This is
the next sort-cluster leaf after the comparator lowering (`OpCallDynTrailTop`,
landed 1d765fff) and the make-list-in-fn-body leaf (landed f28cc36a).

Soundness invariant (non-negotiable): `compile == interpret`, 0 miscompiles. The
langspec differential is BLIND to off-corpus shapes (it missed two real miscompiles
this session) — every step ships a hand-pinned `RunCompiledStrict==Run` off-corpus
regression + the voxgig `--compile`==interpret sweep, gated by `make verify-bytecode`.

## 1. Root cause (traced to the architectural bottom)

A closure (the outer `each`) capturing a name resolves the capture to a re-pushable
operand via `EmitState.resolveOperand` (emit.go) — an event, a frame-LOCAL SLOT, or
an inert const. For `[comp:Function]`:

- `ComputeCaptures` (fn_capture.go) returns `r.Defs.Top("comp")`, which is the
  **FnDef value** `InstallFnDef` pushed (core_helpers.go:725 `r.Defs.Push(name,
  NewFnDef(entry))`; note `TFnDef` conforms to `TWord` in the lattice, so an
  `AsWord` on it reads empty — the earlier red herring "Word('')").
- `resolveOperand(FnDef)` needs a home. A TOP-LEVEL fn's body is compiled as a
  **unit with param slots** (`buildFnBodyReturnsFn` → `StartFnCompile`,
  core_helpers.go:537; params registered in `emitUnit.localByID` at emit.go:1476),
  so `comp` resolves to its slot — my top-level repros compile. A concrete inert
  comparator (`cmp/r`) also const-bakes. But an ABSTRACT Function carrier with no
  slot has no home → refuse.

**The divergence (confirmed by the comp-install stack trace):** a MODULE fn
(`Sort.insertion`, a FnDef WRAPPER value reached by dot-access) dispatches through
`execFnDefLiteral` (engine.go:3920) → `execFnDefSig` (engine.go:4660) → `CallBoru`
(registry.go:1238), which binds params via `InstallFrameBinding` on the **def-stack
only — NO unit, NO param slots**. A NAMED top-level fn instead dispatches through its
registered sig's `ReturnsFn` = `buildFnBodyReturnsFn`, which `StartFnCompile`s the
body as a unit with param slots. Instrumented proof: at the outer-each capture the
enclosing unit had ZERO named param slots (`unitNames=[]`).

So: **module-fn bodies are never compiled as param-slot units during CompileCheck
re-entry**, so their `Function` params have no slot for a closure to capture.

## 2. The fix — route module-fn compile re-entry through the param-slot unit path

Two candidate approaches; (A) is preferred (reuses the proven path), (B) is the
fallback if (A)'s dispatch surgery proves too broad.

### (A) Compile the module-wrapper's body as a unit (reuse buildFnBodyReturnsFn)
When a module-wrapper FnDef with a real boru body (not a trivial-delegation wrapper —
`isTrivialDelegationBody` is false; `comp` lives in a real body) is dispatched in
COMPILE mode (`Check.Compiling`), drive its `buildFnBodyReturnsFn` (which already
exists on its sub-registry sig) so the body is `StartFnCompile`'d as a unit with
param slots + `SetUnitParamTypes`, instead of the `execFnDefSig`/`CallBoru`
def-stack run. The each closure then captures `comp` via its param slot.

- Investigate first: WHY does `execFnDefLiteral`/`execFnDefSig` take the CallBoru
  body-run instead of the registered ReturnsFn unit-compile for a module wrapper?
  (The named path uses `execMatch`'s `match.Sig.RunInCheckMode` intercept at
  engine.go:2514 to call the ReturnsFn; the FnDef-value path at 2995 →
  execFnDefLiteral may bypass it.) The fix likely routes the module-wrapper
  dispatch through the same ReturnsFn intercept the named path uses.
- The sub-registry threading must be preserved (the body runs in `fnDef.Registry`
  so module-private words resolve — see lang/go/CLAUDE.md "Module FnDef Wrappers").

### (B) Give the CallBoru body-run param SLOTS (a local param-slot unit)
If routing through the ReturnsFn is too invasive, have the compile-mode CallBoru /
execFnDefSig path open a lightweight `StartFnCompile` unit for the body so named
params get `localByID`/`localByName` slots (and `InstallFnDef` of a Function param
ALSO registers the param's slot id), so `resolveOperand` / the name-fallback
(`resolveCapturedParam` + `emitUnit.localByName`, drafted this session — reverted as
inert without slots) resolves the capture. The drafted name-fallback is the
right shape; it only needs the enclosing unit to actually HAVE the param slots.

## 3. Soundness — the specific hazards

1. **Don't change interpret-mode dispatch.** Gate every new path on
   `Check.Compiling`. The interpreter must keep running module fns exactly as today
   (the repos' own gate is compile==interpret via fallback — never regress the
   interpret 44/48).
2. **Module sub-registry fidelity.** A module fn's body resolves module-private
   words in `fnDef.Registry`. Any unit-compile must analyse/record in that registry,
   not the main one (else a private word becomes undefined → false refuse or, worse,
   a wrong resolution). Mirror `execFnDefSig`'s `capturedReg` threading.
3. **Captured-comparator apply correctness.** Once `comp` captures soundly, the
   inner `(prev key comp)` apply lowers via `OpCallDynTrailTop` (already landed) —
   verify the captured slot value flows to it with the right arg order (the existing
   off-corpus comparator regression covers ordering; add a MODULE-fn variant).
4. **Recursion / re-entry.** A module fn calling itself (or mutual) must not loop the
   unit-compile (memoise on the existing `FnAnalysisKey`, as `buildFnBodyReturnsFn`
   already does at core_helpers.go:482).

## 4. Mandatory regressions (the differential is blind to all of these)
- A module fn with a `[comp:Function]` param + an `each`/nested-`each` body that
  applies `comp` → force-compiles native (no FALLBACK island) AND
  `RunCompiledStrict==Run`, over BOTH a `cmp/r` and a module-defined comparator.
- The SAME shape at TOP LEVEL must still compile (no regression).
- A module fn whose body references a module-PRIVATE word inside the closure →
  compile==interpret (sub-registry fidelity).
- A trivial-delegation module wrapper must stay on its fast `execMatch` path
  (no behaviour change) — `wrapper_dispatch_test.go` stays green.
- The voxgig `--compile`==interpret sweep over all 48 files: still 0 miscompiles.

## 5. Staged steps (each its own gate-clean commit)
0. Reproduce minimally OUTSIDE the sort lib: define a small module inline
   (`module […]`) with a `[comp:Function]` fn whose body nest-`each`-applies comp,
   import + call it. (Top-level repros compile; this must reproduce the refuse, so
   the fix can be validated without the whole sort lib.)
1. Confirm the dispatch divergence (A's investigation): instrument which path
   (ReturnsFn unit-compile vs execFnDefSig/CallBoru) a module-wrapper dispatch takes
   in compile mode, and identify the exact branch (engine.go:2995 / execFnDefLiteral)
   that bypasses the ReturnsFn intercept.
2. Land approach (A) or (B), gated on `Check.Compiling`, sub-registry-faithful.
3. Re-sweep the sort algorithms; comp-capture clears for ALL comparison sorts.

## 6. After comp-capture — the residual sort chain (do NOT forget; flips need ALL)
sort_smoke calls all 16+ algorithms. Even with comp-capture cleared, each still
chains through (root-caused this session): **branch-multi-value** (an `if` arm
running several statements nets >1 value — "branch leaves extra values (Stage 2
lowers single-result branches)"), **swap-at / convert List** over an Array
(surfaced as "check diagnostics"), plus the distribution sorts' own leaves. A
sort-FILE flip lands only when every algorithm's whole chain clears. The comp-capture
fix is the highest-leverage next step (shared by every comparison sort), but it
flips 0 files alone.

## 7. Effort / risk
Approach (A) is module-dispatch surgery — load-bearing for ALL module calls; a wrong
move silently breaks every `pkg.word` dispatch, and the differential is blind to the
off-corpus shapes. Treat as a dedicated multi-step effort with the full gate +
off-corpus regressions at each step, not a quick patch. This is the right scope for a
focused follow-up session.

## 8. ATTEMPTED + VALIDATED the unification — sound, fixes comp-capture, but REGRESSES run-spec (recursion)

Implemented approach (A) end-to-end and ran the full gate. Findings (the unification
ATTEMPT is reverted; the insights stand):

- **The unify point is engine.go:4179** (execFnDefLiteral's sub-registry branch). A
  TRIVIAL-delegation wrapper already routes through `execMatch` (→ carrierResults →
  the matched sig's ReturnsFn = unit-compile); the NON-trivial boru body routed to
  `execFnDefSig`/`CallBoru` (def-stack, no unit). Routing the non-trivial body through
  `execMatch` IN CHECK MODE — wrapped in `e.shareCheckState(fnDef.Registry)` so the
  body's `buildFnBodyReturnsFn` compiles its unit into the MAIN program and
  `RecordUserCall` references it — is the change.
- **It IS SOUND**: `make verify-bytecode` GREEN (0 miscompiles). Trivial/each/make
  module fns compile correctly; **the sort comp-capture leaf CLEARS** (the inline-
  module repro advances past it). `shareCheckState` is ESSENTIAL — without it the unit
  lands in the sub-registry's emit and every module-fn call refuses "user fn call
  (Stage 3)" (a coverage regression `make test` caught).
- **BUT it REGRESSES `TestRunSpecHarnessCompiles`** (a positive test: `Test.run-spec`
  must compile native). Under the unit path, `test-describe` refuses "unmatched
  dispatch recovered at test-describe" — the RECURSIVE-code-body wall. The inline
  `CallBoru` path AVOIDED this because the recursion terminates DATA-DRIVEN during
  inline analysis (empty `subs`), so test-describe never compiles as a self-
  referential unit. **So the two paths exist partly BECAUSE inline handles recursion
  that unit-compile does not.**

### Revised fix: PROBE-AND-FALLBACK (preferred) or co-implement recursive closures
A naive unify regresses the recursive test framework. Two sound ways to land it:
1. **Probe-and-fallback** (lower risk, preferred): at engine.go:4179 in check mode,
   PROBE the unit-compile (execMatch in a throwaway emit state, like the closure
   probe at callable_words.go:262). If it records cleanly, COMMIT it (sort gets param
   slots). If it refuses (run-spec's recursion), fall back to the inline `CallBoru`
   path (run-spec keeps compiling). Both sound; no regression. The intricacy is the
   throwaway-state + replay at the dispatch level — model it on the existing closure
   probe.
2. **Co-implement recursive-code-body closure compilation** so test-describe compiles
   as a self-referential unit (memoise the in-progress unit on FnAnalysisKey so the
   recursive dispatch resolves to it instead of recovering). Larger; unblocks the
   test framework too — but that is the doc's separate multi-session feature.

Mandatory: `TestRunSpecHarnessCompiles` MUST stay green, AND the sort comp-capture
must clear, in the SAME change. The probe-and-fallback achieves both with the least
risk to the load-bearing module-dispatch path.

## 9. LANDED — fn-dispatch UNIFICATION (commit c5ff7bd9). One code path, all modes.
The design flaw is fixed: every function — named, bare-word, trivial-delegation module
wrapper, AND a real module-fn body — dispatches through `execMatch`, so a fn body is
compiled the SAME way regardless of how the fn was reached (a param-slot unit via the
matched sig's ReturnsFn). The trivial/non-trivial split in execFnDefLiteral is gone
(isTrivialDelegationBody removed). Sub-registry fidelity via match.Reg + shareCheckState;
INTERPRET byte-identical (buildFnBodyHandler runs a FOREIGN-registry body in its home
registry via CallBoru — same execution, one dispatch path). GATE: verify-bytecode GREEN +
crossdiff GREEN (0 interpret divergences) + fmt/vet/lint. The module comp-capture leaf
CLEARS. FOLLOW-UP (contained, still refusing — an open defect): recursive `test-describe` hits the
recursive-closure limit the inline path masked — TestRunSpecHarnessCompiles re-scoped to
compile==interpret, reducibleCeiling 2->3. NEXT: §10 recursive-code-body closures (restore
native run-spec) + the sort chain (comp-capture is cleared; the `fn call operand of unknown
provenance` next leaf = passing a module-fn VALUE as a call arg, then branch-multi-value /
swap-at / convert × 16 algorithms).

## 10. SORT CHAIN — leaves after comp-capture (this session cleared 2)
CLEARED this session: comp-capture (the dispatch unification, c5ff7bd9) and
module-fn-value-as-arg (`xs M.sort M.by-num`, the comparator baked as a const operand,
e3f925ce). `sort.boru` (the LIBRARY) now force-compiles clean. Remaining per-algorithm
leaves (from sort_smoke, all 11 comparison sorts), in priority order:

1. **Def-bound dynamic apply** (HIGHEST LEVERAGE — root of ~9/11). Masked as
   "code-body word each (Stage 2)"; the real reason (probe-unmasked) is
   "closure each$body: unapplied fn-value in body residual (dynamic apply not
   lowered)" AND its sibling "stack discipline: result operand of lt/gt is not on
   top". ROOT CAUSE: a comparator apply `(a b comp)` BOUND TO A DEF-LOCAL
   (`def c ((arr get i) (arr get j) comp)` → used by `if (c gt 0)`) is an
   INTERMEDIATE, not the body's trailing residual. OpCallDynTrailTop lowers ONLY the
   trailing residual (emit.go:1532 reads TrailingApplyArity for the bodyStk TOP; the
   paren-collapse registration is engine.go:5640). A def-bound apply is never
   lowered, so `comp` stays unapplied (#1) or its result isn't laid out on top for
   the consuming compare (#2 — layoutOperands, lower.go:929). FIX: lower the dynamic
   apply as an INTERMEDIATE value-producing event seatable to a frame local (like
   user-fn-call results seat via lower.go planValueDefLocals/lowerUserCall). Affects
   bubble, cocktail, pancake, bitonic (#1) + selection, gnome, shell, odd-even, cycle
   (#2). The minimal repro: `def f fn [[comp:Function][List][ def arr (make Array
   [5 3]) [0] each [ var [[i] def c ((arr get i)(arr get i) comp) if (c gt 0)[1][0] ]]
   ]] cmp/r f` — the apply ALONE compiles; `def c (apply); if (c gt 0)` refuses.
2. **branch reads enclosing computation (Stage 3)** — insertion.
3. **branch leaves extra values (Stage 2 lowers single-result branches)** — comb.
4. **Recursive test framework** (`test-test`/`test-describe`) — the *_test.boru files;
   the §8/§9 recursive-code-body-closure follow-up (shared with run-spec).

Leaf #1 is the highest-leverage next target (root of ~9/11 comparison sorts, and the
same def-bound-dynamic-apply lowering covers both its manifestations).

## 11. SORT CHAIN PROGRESS — def-bound dynamic apply LANDED (baaf45b7); 13/30 algorithms compile
The def-bound-dynamic-apply fix (§10 leaf #1) recording the trailing fn-value apply as a
RecordDynApply EVENT cleared 8/11 comparison sorts. **13 of the 30 smoke-test algorithms
now force-compile**: bead, bogo, bubble, case-insensitive, cocktail, cycle, gnome,
insertion, is-sorted, natural, odd-even, reverse, selection, shell. Gate green
(verify-bytecode + crossdiff + make test, 0 miscompiles).

Remaining leaves (by frequency), precisely characterised:
1. **"branch leaves extra values (Stage 2 lowers single-result branches)"** (8: bucket,
   comb, counting, pancake, pigeonhole, radix-lsd, tim). A branch arm `[ arr i j swap-at
   end 0 ]` leaves an EVENT (swap-at — a side-effecting in-place swap whose array result
   is ignored) BELOW a trailing CONST (`0`, the arm's real result). lower.go:1069's
   multi-value arm case requires EVERY residual value to be event-produced on the sim
   stack, but a const isn't simulated, so [array_event, 0_const] (len(vm)=1 != residualN=2)
   refuses. FIX: extend the multi-value arm lowering to seat an event-then-trailing-const
   residual (drop/keep the leading side-effect event, push the const on top) — sound
   because the enclosing each result is discarded (only swap-at's side effect matters) and
   the interpreter leaves the same whole-arm residual.
2. **"branch reads enclosing computation (Stage 3)"** (5: bitonic, intro, quick, slow,
   stooge) — a branch arm references a value computed in the enclosing scope.
3. **"body leaves extra values (Stage 3 lowers in-order results)"** (3: heap/sift-down,
   merge, sort/merge-sort) — a fn body (not a closure) nets >1 in-order value.
4. **check diagnostics** (1: radix-msd).

Leaf #1 (branch-leaves-extra-values, 8 algorithms) is the next highest-leverage target.

## 12. Leaf #2 (branch-leaves-extra-values) — REFINED root cause (deeper than trailing-const)
Instrumented the real refusal: it is NOT the swap-at trailing-const hypothesis. The
failing arm has out=opEvent, residualN=1, len(lw.vm)=0 — a branch arm whose single
result is a value computed in the ENCLOSING scope. Minimal repro (no swap-at, no
dynApply): `def f fn [[n:Integer][Integer][ def g (n add 5) def gg (if (g lt 1) [1] [g])
gg ]] 3 f` — the else-arm `[g]` references the value-def `g`, which lives on the PARENT
sim stack, unreachable from the arm's OWN fragment sim (lowerFragment resets lw.vm=nil
per fragment). The dynApply sibling `def c (a b comp); if (c gt 0) [c] e` is the same
shape.

ROOT CAUSE: `g` is NOT promoted to a frame local (instrumented: promoted=false), so it
stays on the parent sim and the arm can't re-push it. planValueDefLocals (lower.go:676)
DOES promote value-defs — but the promotion isn't firing for a value-def referenced
ONLY inside a branch arm. TWO-PART FIX: (1) promote such a value-def (the
fragRef/valueDef gate must fire for an arm-only-referenced value-def), AND (2)
re-resolve the fragment's `out` operand (captured at RECORDING time, pre-promotion, so
still an opEvent) to its local before the arm switch — drafted at lowerFragment but
inert until (1) lands, since `out` isn't in lw.promoted. NEXT: instrument why the
valueDef/fragRef promotion gate skips an arm-only-referenced value-def (a build snag in
the planValueDefLocals instrumentation blocked the final read; the gate at lower.go:676
+ the fragRef/fragInternal computation are the place to look). This leaf (8 algorithms)
is the same "branch arm reads an enclosing value-def" shape as the Stage-3
"branch reads enclosing computation" leaf (5 algorithms) — likely one fix clears both 13.

## 13. SORT CHAIN — 27/30 algorithms compile (this segment cleared 10)
"Do stage 3" + "push on" cleared, each gated green (verify-bytecode + crossdiff + make
test, 0 miscompiles), each with a hand-pinned off-corpus regression:
- **Dead value-def drop** (e2b742e5): a `def _ (expr)` never read whose result is a
  USER-call or an `if` MERGE was left on the stack ("body leaves extra values Stage 3");
  now dropped (the interpreter binds it off the residual). Cleared quick, merge, slow,
  stooge, bitonic, sort, counting, pigeonhole.
- **Multi-value arm with trailing const** (0afb8f87): an `if` arm leaving residualN>1
  COUNTED events below a trailing const compiles as a variadic multi-value (fragMulti).
  Cleared heap, intro.

## 14. SORT CHAIN — 28/30 (radix-lsd CLEARED)
**radix-lsd CLEARED** (e565f7b0). The §13 note's leftover-source guess was WRONG: with
working seq→word instrumentation (a temporary `dbgDumpFrag` walking `frag.events` +
`fnRecs[unit].name`, since the lowerer has `es *EmitState`/`fnRecs`, not a `.events`
field) the leftover vm[0] is **`list-max`** — `def mx (lst list-max)`, NOT ensure-non-neg.
`mx` is a USER-call value-def CAPTURED BY THE each-closure (`if (pl lte mx)` reads it
across the closure floor). planValueDefLocals saw it `refs=1 valueDef=true fragRef=true
fragInternal=true`, so: promoteUser was false (only forceOrder/refs>=2 triggered for a
user call) and deadValueDef false → the FIRST switch case `isUser && !promoteUser &&
!deadValueDef` fired and LEFT list-max loose on the sim. A NATIVE captured value-def
already promotes via the `valueDef` trigger at the promotion case; only the user-call
equivalent was shadowed (the deliberately-narrow Test.run-spec guard). FIX: a closure
capture can only re-push from a frame local (promoteOperand rewrites closureCaps at
OpPushClosure) — never a transient sim slot — so a captured user-call value-def MUST
promote. New `eachClosureCap` walks each event's operands for closure caps → `captured`
set; `promoteUser` now includes `captured[seq] && valueDef`. Gated GREEN (verify-bytecode
0 miscompiles + crossdiff 0 divergences + make test sole-failure the pre-existing
ratchet). Off-corpus regression `bytecode_captured_valuedef_test.go` (inline module —
list-max is module-private) compiles native + RunCompiledStrict==Run; confirmed it
refuses with the exact `branch leaves extra values` leaf when the new clause is disabled.

## 15. SORT CHAIN — 29/30 (bucket CLEARED)
**bucket CLEARED** (091f3a65). Despite the "THREE-level-nested each" framing below, the
refusal had nothing to do with nesting depth — it was a value-def bound to an `if` result
read TWICE. Instrumented `layoutOperands`'s `set` refusal (seq→word map): the refusing
`set` is `bcount set bi ((bcount get bi) add 1)` — key=`bi`=`def bi (if (raw gte n)
[(n sub 1)] [raw])` (an if-result, `promoted=false`), read by BOTH `bcount get bi` AND the
`set` key, but only the `add` result was on the sim. Root cause: planValueDefLocals
`continue`d past EVERY evBranch result (only ever dead-dropping a zero-ref merge) — it
never promoted an if-result, so `bi`'s single sim copy was consumed by the first use and
the second couldn't seat ("operands of set not adjacent on top"). FIX: promote a
single-value (`branchSingleValue`: 2-arm, both arms residualN<=1) if-result value-def read
>=2 times (or cross-fragment) to a frame local, like a multiply-read call result;
lowerEvents stores the merge after the branch, references re-push from the slot. The
lower-time store hook REFUSES if the merge turns out variadic/diverged — a
wrong store is impossible, and the island it leaves behind is an open defect,
not a sanctioned landing place. Gates GREEN (verify-bytecode 0
miscompiles + crossdiff 0 divergences + make test sole-failure the pre-existing ratchet;
24-algo sweep NATIVE=23 MISCOMPILE=0). Off-corpus regression
`bytecode_branch_valuedef_test.go` (top-level + each-body; confirmed it refuses with the
exact leaf when the promotion is disabled).

REMAINING 1 (radix-msd only):
1. ~~**bucket**~~ — CLEARED (§15). [historic: "operands of set not adjacent on top" — NOT
   the nested-each depth it first looked like, but a multiply-read `if`-result value-def.]
2. **radix-msd** — recursion-through-closure barrier CLEARED (a109cc7c), algorithm still
   blocked one leaf deeper. The "code-body word each" was masking
   **SELF-RECURSION THROUGH A CLOSURE BODY** (msd-go calling itself inside an each-body
   `if`). Root cause: `recordClosureDispatch`'s throwaway PROBE used a fresh
   `NewEmitState`, losing the enclosing in-progress unit, so the recursive call MISSED the
   fnUnits memo and re-compiled the enclosing fn in the throwaway (which re-hit the same
   closure and never registered its residual → "fn go: body result of unknown
   provenance"). The non-closure recursive path already works because the recursive call
   shares the state where the fn's key is registered — the fnUnits HIT *is* the recursion
   guard. FIX: `forkForProbe` seeds the probe with the enclosing fnUnits/fnRecs/units
   (fresh slice backing; shared recs read-only on the hit path) so the recursive call HITS
   the reserved unit. Sound/general (each+fold+scan verified, verify-bytecode +
   crossdiff GREEN); off-corpus regression `bytecode_recursive_closure_test.go`.
   REMAINING msd-go leaf: with recursion compiling, the each-closure probe now fails at
   **"unmatched dispatch recovered at sub"** — ROOT-CAUSED (instrument the recovery sites
   at engine.go:6764/6820/6845): at the failing `(counts get (v add 1)) sub 1` the
   resolved operand carrier is **`Scalar`**, not `Integer` (site 6845, suspended=false),
   so `sub`'s numeric (`Number`) sig can't match. `counts = make Array (iota N each
   [drop 0])` has Integer elements; `counts get` should yield Integer. Isolated to the
   recursion: the SAME `counts` captured into the SAME each with the SAME `counts get …
   sub 1` but WITHOUT the recursive `… go` call keeps the element **Integer** and COMPILES;
   adding the recursive call widens it to **Scalar**. So the recursive call contaminates
   the carrier FIXED-POINT for the enclosing fn — a captured Array's element type widens to
   Scalar in the round the recursion forces. A distinct, deeper leaf (carrier inference
   under recursion); NOT to be rushed (a wrong widening fix risks unsoundness the
   differential is blind to off-corpus). Deferred with the precise repro above.

## 16. radix-msd — FULLY ROOT-CAUSED as a 4-LAYER carrier-inference cascade (deferred)
Instrumented end-to-end (ADD-DBG on `ReturnsAddConcat`, SUB-DBG on the recovery sites,
PROV-DBG on the fn-finish residual, narrowArgsToParams). The failing `(hi sub lo)` at row 3
is the TAIL of a cascade that all starts from ONE source — `counts get` returns gradual
`Any`:
1. **Array `get` over a non-concrete carrier returns gradual `Any`.** `counts = make Array
   [0 0 0]` is a CARRIER (not concrete) in check mode; the Array get sig hardcodes
   `Returns: [TAny]` (native_storage.go:191) and even the Node int-key `getIntKeyReturns`
   bails to `NewDynamicCarrier(TAny)` for a non-concrete container. Array carriers DON'T
   track an element type, so `counts get 0` → gradual `Any` (confirmed: `[ADDCONCAT]
   args=[Any(dyn=true) Integer]`).
2. **`add` over a gradual `Any` widens to gradual `Scalar`.** `lo add (gradual Any)` matches
   the concat overload `[Scalar String]` optimistically (the gradual Any fills String), so
   `ReturnsAddConcat` → `NewDynamicCarrier(TScalar)`. So `blo`/`bhi` = gradual Scalar.
3. **The compile-path genArgs do NOT narrow a widened arg to the declared param type.** The
   recursive `… go` passes the gradual Scalar `blo` as `hi`. The carrier-RESULT path
   narrows via `narrowArgsToParams` (core_helpers.go:577), but the body-UNIT-compile path
   (line ~532) generalises with `NewCarrier(a.Parent)` — a STRICT Scalar — and does NOT
   narrow to `hi:Integer`. So the recursive re-analysis compiles go's body with hi=Scalar →
   `(hi sub lo)` = Scalar sub Integer → "unmatched dispatch recovered at sub".
   - A genArgs narrowing (mirror narrowArgsToParams; gate `genSpec == nil` — narrowing a
     GENERIC param's type VARIABLE breaks instantiation, TestEmitGenericInstantiation) is
     SOUND (verify-bytecode + crossdiff GREEN) and FIXES layer 3 — but only EXPOSES layer 4.
4. **Branch-merge residual loses provenance under recursion.** With layer 3 fixed, go's
   OUTER-if merge (an Array) has `hasProducedBy=false` at fn-finish → "fn go: body result of
   unknown provenance" (same class as the §14 closure-residual issue, here for DIRECT
   recursion + the gradual flow). A clean Integer direct-recursion (no get/add) already
   compiles, so this is specifically the gradual-carrier merge.

ROOT: fixing layer 1 (Array carriers track element type so `counts get` → Integer) collapses
ALL FOUR — no gradual Any, no Scalar, no param widening, no provenance loss — and is the
right fix, but it is a broad, corpus-wide carrier change (every Array get). Each layer is
its own risky carrier change; the partial genArgs fix (layer 3) is sound but has NO standalone
algorithm win (radix-msd still refuses at layer 4), so it was NOT landed — chaining
speculative carrier changes at session end is exactly the unsoundness risk the invariant
guards against. DEFERRED as a dedicated carrier pass (start at layer 1: Array element-type
tracking + an Array-get ReturnsFn).

### Layer-4 mechanism CONFIRMED + layer-1 soundness wall (second investigation)
Re-applied the genArgs narrowing and instrumented `RecordBranch`/fn-finish: layer 4 is a
RECURSION-BAIL provenance gap that genArgs itself triggers. genArgs narrows the recursive
arg back to `Integer`, so the recursive self-call's analysis key now MATCHES the enclosing
in-flight key → `AnalyseFnBody`'s in-flight path BAILS returning `NewCarrier(declared)` (no
producedBy). Instrumentation shows the enclosing fn's outer-`if` Array merge is then NOT
recorded as a branch in the compile pass (`BR-ENTRY` shows only Disjunct/Integer merges, no
Array) and go's body residual is an unprovenanced Array → "fn go: body result of unknown
provenance". So genArgs trades layer 3 ("unmatched dispatch recovered at sub") for layer 4 —
NEITHER compiles. Verified genArgs has no standalone win: a probe recursive fn with a
widened arg but a simple (non-branch-merge) residual compiles WITH AND WITHOUT genArgs, and
every branch-merge-residual shape hits layer 4 — so genArgs unblocks nothing and was
reverted again. Layer-1 soundness wall: an Array is MUTABLE, so a statically-tracked element
type is only sound as an UPPER BOUND that `set` widens; `make Array (list)` → `Array<elem>`
read by get would mis-type any array a later `set` stores a wider value into (compile !=
interpret). So even layer 1 needs gradual element types + set-widening, i.e. a real
parameterised-mutable-Array-carrier feature — genuinely multi-session, not a leaf. Tree
left clean at 29/30.

sort_smoke needs ALL 30 to flip the file; **29/30** — only **radix-msd** remains, blocked by
the layer-1 root above. NOT a lowering leaf; a dedicated carrier-inference pass.
