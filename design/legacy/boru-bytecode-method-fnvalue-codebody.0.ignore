# boru Bytecode — deep-dive: method-fn-value apply with a code body (`module-rand.tsv:38`)

Status: design / diagnostic, validated against the live tree (June 2026, after
Stage A landed at refusalCeiling 10). This is the next-step investigation for the
method-fn-value-apply row the completion guide flags as priority #2. It **sharpens
the guide's description** (the orphan is the code body, not the apply result) and
finds the row is **larger than its "medium risk" billing** — it needs a new VM
mechanism, not a recording tweak. Read before attempting it.

---

## 0. The row

`lang/spec/module-rand.tsv:38`:

```
import "boru:rand"  def r (Rand.with-seed 2)  r.list-of [Rand.int 0 10] 3  →  [7 5 4]
```

`r` is a seeded `Rand` instance — an `OrderedMap` of trivial-delegation FnDef
wrappers, each carrying its own sub-registry closing over a private `randState`
(`lang/go/modules/rand.go::buildRandExportsForState`). `r.list-of` is the
`rand-list-of` wrapper; it takes a **code body** (`NoEvalArgs[0]`) and a count, and
runs the body N times against the instance's generator.

Refuses: `residual value of unknown provenance`.

---

## 1. The boundary (what compiles vs refuses) — measured

| program | result | why |
|---|---|---|
| `r.int 0 100` | **compiles** | `CALL_NATIVE_POLY get` (retrieve method) + `CALL_DYNAMIC` (apply to scalars) |
| `r.string "abc" 5` | **compiles** | same shape, scalar args |
| `Rand.list-of [Rand.int 0 10] 3` (top-level) | **compiles** | `Rand` is STATIC → records a real closure: `PUSH_CLOSURE rand-list-of$body` + `CALL_NATIVE rand-list-of` |
| **`r.list-of [5 6 7] 3`** | **compiles** ✅ | `[5 6 7]` (plain values) materialises as a const; `CALL_DYNAMIC` applies the fn-value to `[5 6 7], 3`; runtime dispatches `rand-list-of` via the instance sub-registry. **Proves the dynamic-apply + sub-registry + RNG-faithfulness path all work.** |
| `r.list-of [Rand.int 0 10] 3` | **refuses** | the code body auto-evaluates and orphans (below) |

The `r.list-of [5 6 7] 3` row is the key discriminator: everything except the
code-body handling already works, end to end, faithfully.

---

## 2. The exact orphan (corrects the completion guide)

Instrumented residual for `r.list-of [Rand.int 0 10] 3` at Finalize:

```
[0] parent=Any   dynamic=true  carrier=true  hasProducedBy=true   (the r.list-of fn value — from CALL_NATIVE_POLY get)
[1] parent=List  concrete=true                hasProducedBy=false  str=[Integer]   ← THE ORPHAN
[2] parent=Integer                            (the count 3)
```

The guide says "the checker applies it (yielding the `[Integer]` return type) but
never reaches `carrierResults`/`RecordCall`." **Sharper truth:** the fn value DOES
have provenance (the `get`). The apply is never collapsed into one event — the
residual stays `[dynfn, body, 3]`, which `resolveDynamicApply` classifies as a
leading-dynamic `OpCallDynamic`. The blocker is residual entry **[1]**: the body
`[Rand.int 0 10]` **auto-evaluated to `[Integer]`** (its `Rand.int` word became an
Integer carrier) and so has no provenance and cannot materialise.

**Why it auto-evaluates:** in check mode `r` (= `Rand.with-seed 2`) is recorded as
a runtime `CALL_NATIVE` (RNG side effect — not const-folded), so `r.list-of` is a
runtime poly-`get` typed `Any`. A dynamic `Any` value is **not** a Function at the
pointer (`engine.go:2896` only auto-applies a concrete `TFnDef`/`TFunction`), so
`r.list-of` does **not** dispatch `rand-list-of` during the check run. The body
list is left unconsumed on the stack and `autoEvalStack` evaluates it. (For
`[5 6 7]` the same auto-eval is the identity — no words — so it survives as a
materialisable const; that is the only reason that row compiles.)

So: the compiler never learns the fn value is `NoEvalArgs`, because the value is
dynamic. That single fact gates every fix below.

---

## 3. Why the obvious fixes are each UNSOUND (the real difficulty)

The interpreter passes the body **raw** (`Eval=true`, unevaluated) and lets the
dispatched fn's `NoEvalArgs` decide: `rand-list-of` runs it as a body; a
non-`NoEvalArgs` fn would `autoEvalList` it. The compiled program must reproduce
exactly this **runtime** decision — but at compile time the fn value is `Any`, so
the compiler cannot pick eval-vs-body. Each static choice breaks the other case:

- **Bake a CLOSURE** (like top-level `Rand.list-of`): correct for a `NoEvalArgs`
  body word; **wrong** if the runtime fn evaluates its list arg (it expects values,
  gets a closure). Unsound for a dynamic apply.
- **Bake a `Quoted` list const**: a `Quoted` list is **never** `autoEvalList`-ed at
  runtime, so a non-`NoEvalArgs` fn that should evaluate it diverges. Unsound.
- **Bake a raw `Eval=true` list const**: would be faithful (runtime `execMatch`
  decides eval-vs-body via the real `NoEvalArgs`) — but it **cannot bake**:
  `isInertConst` rejects a list containing `Word` tokens (`Rand.int`).

The faithful behaviour needs the **raw `Eval=true` body to reach `callDynamic` at
runtime**, where `execMatch` already replicates the interpreter's `NoEvalArgs`
handling. The compiler does not need to know `NoEvalArgs` — it only needs to defer
the decision to runtime by passing the raw body.

---

## 4. The viable design (a new VM mechanism, ~Stage-A-sized or larger)

Store the raw body tokens in a dedicated **Program side-pool** (like `Fallbacks` /
closures), NOT as an inert-const operand. A dynamic-apply op reconstructs the
`Eval=true` body list at runtime and applies the fn value, so `callDynamic` →
`execMatch` makes the faithful eval-vs-body call.

Concretely:

1. **Record-time capture (the hard part).** Grab the **raw** body list
   (`[Rand.int 0 10]`, `Eval=true`, tokens intact) at the point it is pushed
   (`OnPushLit` / `stepLiteral`), BEFORE `autoEvalStack` mangles it, and remember it
   keyed so the dynamic-apply residual position can find it. The auto-eval replaces
   the value in place and mints a new carrier ID, so the link from the pushed raw
   list to the residual `[Integer]` must be established at push time (e.g. a
   pending-raw-body map keyed by tape position / value identity), not recovered
   afterward.
2. **Pool + opcode.** Add a `RawBodies [][]Value` pool and an `OpCallDynamicBody`
   (or extend `OpCallDynamic` with a raw-body operand). At runtime it pushes the
   reconstructed `Eval=true` list, then performs the dynamic apply over
   `[fnvalue, rawbody, count]`.
3. **`resolveDynamicApply` integration.** When the residual is
   `[dynfn, <pending-raw-body>, args…]`, emit the body-carrying op instead of
   refusing on the unmaterialisable `[Integer]`.
4. **VM.** `callDynamic` already dispatches the fn value via `execMatch`, which
   honours `NoEvalArgs` — so the body runs as a body for `rand-list-of` and would be
   `autoEvalList`-ed for a non-`NoEvalArgs` fn. **No RNG-faithfulness work is
   needed** beyond this: §1's `r.list-of [5 6 7] 3` proves the runtime sub-registry
   dispatch already draws faithfully (the body's `Rand.int` resolves to the
   instance generator inside `rand-list-of`'s handler).

**Soundness gate.** Pass the body **raw `Eval=true`** (never `Quoted`, never a
closure) so the runtime `NoEvalArgs` decision is the single source of truth — this
is what makes it faithful for *any* dynamic fn, not just `rand-list-of`. Add a
negative test: a dynamic apply over a code-body list whose runtime fn does NOT have
`NoEvalArgs` must evaluate the list (compiled == interpreter).

**Files.** `eng/go/bytecode.go` (`RawBodies` pool + `OpCallDynamicBody`),
`eng/go/emit.go` (record-time raw-body capture + `resolveDynamicApply`),
`eng/go/engine.go` (capture hook at `stepLiteral`/`OnPushLit`), `eng/go/vm.go`
(`callDynamic` body op), `lang/go/bytecode_findings_test.go` (positive + the
non-`NoEvalArgs` negative).

**Verification.** `module-rand.tsv:38` → `[7 5 4]` compiled == interpreter; the
`r.list-of [5 6 7] 3` / `r.int` / `r.string` rows stay green; full
`make verify-bytecode` (differential + property fuzz + `-race` + `borudebug`),
0 divergences; `refusalCeiling` 10 → 9.

---

## 5. Honest scope assessment

The runtime is already faithful (§1) — this is NOT an RNG-faithfulness problem, a
correction to the "medium risk (RNG-draw faithfulness)" billing. The real cost is
the **record-time capture of the raw body before check-mode auto-eval** plus a new
pool + opcode. That capture is fiddly engine surgery (auto-eval replaces the value
in place), and the op is a genuine VM addition. Net: a self-contained but
**Stage-A-or-larger** feature, best given its own session. The validated entry
point is the pending-raw-body capture at `stepLiteral`; the rest follows the
existing closure/fallback-pool pattern.
