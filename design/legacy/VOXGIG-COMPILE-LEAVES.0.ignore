# voxgig-boru `--force-compile` remaining leaves — roadmap

Status snapshot (boru `71238d9`, libs pinned `7b1a4fb`):

- **`boru check`**: the entire voxgig-boru corpus (all modules + every test/spec
  file) checks at **0 errors / 0 warnings / 0 info**. Done.
- **`boru --force-compile`**: 3 files compile (bloom/stats/decision `*_prop_test`);
  **~28 still refuse**. Each refusing file is a *chain* of distinct inner leaves
  — clearing one surfaces the next — so a file only flips once ALL its leaves
  are cleared. The refusal text usually names the surfacing code-body word
  (`test-test`/`each`/`fold`/`check-prop` "Stage 2"), which masks the real leaf.

`--compile` **silently** re-runs a refused file on the interpreter, so the
compile==interpret guarantee is never at risk and nothing in the run says the
compile failed — which makes each of these a hidden failure, not an advisory
note. Every refusal below is an open defect owed a fix. Every fix below must be gated by
the bytecode differential (`test/go … TestSpecCompiledDifferential`, 0
divergences), crossdiff, TS-parity, and a compiled-coverage census re-baseline.

## Leaf classes (root-caused), highest leverage first

### 1. `each`/higher-order body reads an enclosing *computed def*
`fn each$body: branch reads enclosing computation (Stage 3)`

`each` lowers its body as a **separate `each$body` unit**. The scope-floor rule
(`eng/go/lower.go:301`, doc at `:247` — *"Stage 2 branch fragments must not read
enclosing computation"*) refuses an operand produced below the floor. **Params
work** (reachable as frame locals); **enclosing computed defs do not**. Verified
by instrumentation: `fragRef` is empty for these because the body isn't the
inline `ev.loop.body` fragment that `planValueDefLocals` promotes — so the
def is never forced to a frame local.

- **Mechanism (confirmed by instrumentation):** `each` compiles its body via
  `compileClosureBody` as a separate `each$body` unit. `ComputeCaptures`
  **already captures** the computed def `a` (`caps=[a]`, same as a param `h`
  that compiles). The break is in `resolveOperand` (`emit.go`): it checks
  `producedBy` (events) BEFORE `localByID`, so the body's reference to `a`
  resolves to the PARENT's `h add 1` event (below the body's scope floor →
  refusal) instead of the capture slot. A param has no `producedBy` entry, so
  it dodges this.
- **Fix is TWO parts — the read-side ALONE is UNSOUND (miscompiles):**
  1. *Read side:* in `resolveOperand`, a capture of the current unit must win
     over an enclosing-unit `producedBy` event (add a per-unit `capID` set,
     marked at the `StartFnCompile` capture loop, checked first in
     `resolveOperand`). This makes it COMPILE.
  2. *Value-passing side (MISSING):* the capture SLOT must be POPULATED with
     `a`'s value at the dispatch site. With only part 1, the body reads the
     wrong slot — verified miscompile `i mul a` → compiled `[0 5 10 15 20]`
     (read `h`=5) vs interpreter `[0 4 8 12 16]`. **The bytecode differential
     did NOT catch this** (the shape isn't in the langspec corpus); only a
     hand-written `RunCompiledStrict`-vs-`Run` regression did. Any attempt MUST
     add such a regression for this exact shape and gate on it, not just on
     `TestSpecCompiledDifferential`.
- **Why even part 1 misread (deeper cause):** the miscompile read `k`'s value
  (the first PARAM slot), not a wrong-but-adjacent value — i.e. `a`'s capture
  value ID resolved to an EXISTING local slot. Strong signal of a **carrier-ID
  collision**: in check/emit mode `a` (an `Integer` carrier from `h add 1`) and
  `k` (an `Integer` param carrier) can share a value ID, so
  `RegisterLocal(cb.Value.ID)` returns the param's slot instead of minting a
  fresh capture slot. So leaf #1 is not merely read+value-passing; it needs
  capture value-IDs to be DISTINCT from param IDs (carrier-identity work) before
  the slot wiring is even addressable. This is why it is genuinely deep.
- Alternative: route `each` through the inline loop-body fragment so the
  existing fragRef-promotion (`lower.go` `planValueDefLocals` /
  `rewritePromotedRefs`) applies — but `each` currently does NOT use
  `RecordLoop` (no `evLoop` event reaches `planValueDefLocals`; confirmed).
- **Library-restructure workaround (verified):** extract the loop into a helper
  fn taking the computed values as PARAMS — the body then reads params, which
  lower fine.
- **Affects:** `indices-for` (→ Bloom.add/contains), `merge`, and most
  loop-heavy ops across all libs; surfaces as `test-test`/`each (Stage 2)` in
  the unit/spec suites.

### 2. computed `do {…}` / map value provenance
`operand of unknown provenance or not statically materialisable at do` ·
`fn call operand of unknown provenance`

A `do`-map (or class-make) whose value comes from a fn-call result the lowerer
cannot provenance. The simple multi-ref shape is now handled (commit `71238d9`,
`promoteUser` refs≥2); the residual `do`-map case is the computed-map frontier.

- **Affects:** `encode`/`decode` (bloom), `bloom_smoke`, `bloom_unit_spec`,
  `stats_smoke`.

### 3. binary-op operands not adjacent
`stack discipline: operands of sub not adjacent on top`

A binary op whose two operands are interleaved on the simulated stack; the
`layoutOperands` adjacency rule (`lower.go:803`) can't seat them.

- **Affects:** `count` (bloom).

### 4. dynamic-receiver dispatch
`dynamic input at push/set` · `unmatched dispatch recovered at get`

`set`/`push`/`get` over a dynamically-typed receiver refuses (the
dynamic-receiver gate fires before any list/element provenance). Needs
poly-dispatch / `OpCallDynamic` for these ops over a dynamic operand.

- **Affects:** `burst`/`radix` unit (push/set), `trie` unit/spec (get).

### 5. branch-result / `fold` provenance, and the `check diagnostics` smokes
`code-body word fold (Stage 2)` · `check diagnostics`

`fold` body result provenance (trie_smoke); and `decision_smoke`/`tst_unit`
refuse `check diagnostics` even though plain `boru check` is clean — the
force-compile check gate finds something the standalone checker doesn't
(investigate that gate's stricter pass first).

### 6. property-test framework lowering
`code-body word check-prop / test-check-prop (Stage 2)`

The property-test harness's higher-order constructs.

- **Affects:** `sort`/`trie` `*_prop_test`.

## Recommended sequence

1 → 2 → 3 → 4 → 5 → 6. Leaf 1 is by far the highest leverage (one fix clears a
leaf *class* across every loop-heavy op in all five libraries). Land each as its
own commit gated by the four bytecode gates above; re-run the corpus
force-compile sweep after each to measure files flipped.

## What landed this effort

- `eng/go/engine.go` `23424770` + `7b1a4fbd` — 5 checker-accuracy fixes →
  full corpus check-cleanliness (mixed_form_call Any-slot gate; unused_def
  ever-used + construction-self-use suppression; opaque-body use-scan;
  Reach-receiver walk; uncalled_function drain-deferral).
- `eng/go/lower.go` `71238d92` — first compile capability: a multi-referenced
  user-call result promotes to a frame local (matches `def`-once semantics).
  Makes `Bloom.make` lower.
