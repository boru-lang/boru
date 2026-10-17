# `usurp` — Argument-Order Reversal as a Higher-Order Word (v0)

> **Status (implemented).** This document is the original v0 design
> sketch; the shipped word matches it, including the `def ifu (usurp if)`
> alias idiom below. The authoritative, executable contract is the spec
> suite — `lang/spec/usurp.tsv` (with `lang/spec/ref.tsv`,
> `lang/spec/apply.tsv`, `lang/spec/modifiers.tsv`). One clarification on
> the surface form:
>
> **`usurp` accepts either a bare name OR a function value** — two
> overloads:
> - `[Atom]` (by name): `usurp if <else> <then> <cond>` — captures the
>   word and resolves it to its bound function (companion of the `/u`
>   suffix). Shares `ref`'s rules: unbound → `undefined_word`, non-fn →
>   `illegal_ref`.
> - `[Function]` (by value): `usurp (ref if) …` / `usurp (if/r) …`.
>
> A *bare* `usurp if/r` does not resolve (the ref-word isn't evaluated as
> a forward arg) — use `usurp if` or `usurp (if/r)`. The `/u` suffix
> (`if/u`, `if/ur`) is the pure word-modifier form. Binding the wrapper
> to a name and calling it (`def ifu (usurp if)  cond ifu …`) works:
> `InstallFnDef` preserves the wrapper's own (body-less) handler so the
> call re-dispatches the wrapped fn.

## Problem

boru's unified dispatch rule maps a value sitting on the **stack**
(a prefix operand) into the function's **last** signature slot,
while forward tokens fill the leading slots. This is the right
default for most words, but it makes the natural cond-first reading
of `if` bind incorrectly:

```
cond if [then] [else]
```

Here `if`'s signature is `[cond, then, else]`. `[then]` and
`[else]` are forward tokens, so they fill `cond` and `then`; the
stack value `cond` falls into the trailing `else` slot. Concretely:

```
true if [99] [88]   →   88      # NOT 99
```

The condition is silently ignored — it lands in the `else`
position. This is the §6.2 issue from the DX report.

We classify `cond if …` as **incorrect usage** rather than
something to detect and error on (detection would require special-
casing one word in core dispatch). What we want instead is a small,
**general** primitive that lets a user opt into stack-first /
reversed binding for *any* word — turning the mis-binding into a
correct one by construction.

## Solution: `usurp`

`usurp` is a higher-order word:

```
usurp <fn>   →   <fn'>      # fn' is fn with its argument order reversed
```

It takes a function (by name, e.g. `usurp if`, or by value, e.g.
`usurp (ref if)`) and returns a **new function** whose signatures have
their parameters reversed. Because dispatch maps a stack-prefix value to the last
slot, reversing the parameters makes that slot the *first* logical
argument — so the cond-first form binds correctly:

```
def ifu (usurp if)
cond ifu fail pass            # cond → if's condition slot ✓
#  ≡  if cond pass fail
```

Worked example (3-arg `if`, sig `[cond, then, else]`):

- `usurp if` builds a wrapper with reversed params
  `[p0, p1, p2] = [else, then, cond]` and a body that re-invokes
  `if` with the params in reverse: `if p2 p1 p0`.
- Dispatching `cond ifu fail pass` fills `p0=fail`, `p1=pass`
  (forward) and `p2=cond` (stack), then runs `if cond pass fail` —
  `cond=cond`, `then=pass`, `else=fail`. Correct.
- Plain `if cond pass fail` is completely unaffected.

### Why this design

The decisive property is that **nothing in the kernel changes** —
no parser, `WordInfo`, `matchSignature`, deferred-collection, or
check-mode edits. `usurp` synthesises an ordinary `FnDef` wrapper
and the existing function-dispatch machinery does the rest. This is
strictly smaller than the alternative considered (a `/u` suffix
*modifier* that would have threaded a new flag through every core
dispatch path).

The wrapper **re-dispatches the original function value** through a
small Go handler (`usurpDispatchHandler`, `core/go/core_ref.go`)
rather than delegating through a synthesized boru body. (The v0
design used a boru body, `if p2 p1 p0`; the handler form replaced
it when stack-only and mixed-barrier originals were brought into
scope — see "Scope and limitations".) The handler receives the
wrapper's reversed args and lays them out *around* the original
according to the ORIGINAL signature's barrier, so the original
collects them exactly as a direct call would. Per-position
semantics (NoEval code bodies, quoted atoms, patterns) are then
applied by the original signature itself on re-dispatch.

## Mechanics

`usurp` reads the target's `FnDefInfo` and emits a new one.
`FnDefInfo` carries ONE full-fidelity signature slice —
`Signatures []Signature`, where `type Signature = FnSig` (see
`design/FUNCTION-MODEL.10.md`). The wrapper walks
`fnDef.OwnSigs()`: the authored overloads with any synthetic
fallback filtered out.

For each source signature (`N` params), `UsurpFunction`
(`core/go/core_ref.go`) builds one reversed `Signature`:

| Source field | Wrapper field |
| --- | --- |
| `Params` (names, types, patterns) | reversed |
| `Returns` | copied unchanged (return type is independent of arg order) |
| `BarrierPos` | `N` (the wrapper itself is all-forward: `usurped a b c`) |
| `Handler` | `usurpDispatchHandler(orig, origBarrier)` — re-dispatches the original, laying args out per the original's own barrier |
| index-keyed metadata (`NoEvalArgs`, `NoEvalMapArgs`, `QuoteArgs`, `TypeArgs`, `Patterns`) and `Body`/`ReturnsFn` | dropped — they would be mis-indexed on the wrapper, and the original sig re-applies them on re-dispatch |

Each wrapped sig is normalized (`normalizeSig` rebuilds the
positional `Args`/`Patterns` mirrors from `Params`), the set is
sorted, and the result is returned as a fresh Function value with
its own single-slice `FnDefInfo`. Bound via `def ifu (usurp if)`,
it dispatches through the uniform `execMatch` path like any other
function.

### Preserving code-body (NoEval) semantics — CRITICAL

`if`'s branches are unevaluated code bodies. Because the wrapper
re-dispatches the original — rather than binding args to wrapper
params and re-forwarding them through a boru body — the branch
lists reach `if`'s own collection raw, and its `NoEvalArgs` keep
them unevaluated through to the handler's mark/move. The hazard
this section originally tracked (a naive boru-body wrapper
auto-evaluating `[then]`/`[else]` at param-binding time) is
structurally avoided by the handler design. The spec rows in
`lang/spec/usurp.tsv` — including a list-valued condition — pin
this behaviour.

## Scope and limitations

- **All-forward-eligible signatures** (`BarrierPos == len(Args)` —
  the common case, incl. `if`, math ops, typed user fns) are
  supported. Typed words work because the wrapper's reversed
  parameter **types** drive forward type-collection
  (`def subr (usurp sub)` ⇒ `subr a b ≡ sub b a`).
- **Stack-only and mixed-barrier signatures** (`BarrierPos == 0` or
  `0 < BarrierPos < N`) are also supported. The re-dispatch handler
  lays the reversed args out *around* the original according to its
  barrier — positions `0…B-1` after the original (forward), positions
  `B…N-1` before it (stack, top-first) — so a stack-only or mixed
  original receives its args in the right place and reverses
  faithfully. (Earlier this was a stated limitation: the handler used
  a fixed forward layout, so a stack-only original could not collect
  the args and the wrapper was left inert. See
  `usurpDispatchHandler` in `core/go/core_ref.go` and the barrier rows
  in `lang/spec/usurp.tsv`.)
- **Pattern / Optional params** reverse positionally with the param
  list; index-keyed metadata is remapped. Any shape that cannot be
  faithfully reversed errors rather than mis-handling silently.

## Usage notes

- **Bind the result**: `def ifu (usurp if)`, then `cond ifu fail
  pass`. Inline `cond (usurp if) fail pass` (a bare function value
  mid-stack) is not supported — bind it, like an alias.
- **Reversal is total**: every argument flips. With `ifu`, branches
  read **else-first** (`cond ifu <else> <then>`). That is the
  documented, intended consequence of a general reverse primitive,
  not a special case for `if`.

## Implementation sketch (for when this is built)

- New native `usurp` in `lang/go/native/native_usurp.go`, two
  overloads: `[Atom]` with `QuoteArgs{0:true}` (capture an upcoming
  word name → `r.Lookup`) and `[Function]` (a function value);
  both `RunInCheckMode: true`.
- Core helper `reverseFnDef(inner, innerName) (Value, error)`
  implementing the table above; error on word-not-found,
  not-a-function, barrier sigs, and irreversible shapes; add to the
  type-literal no-panic gate.
- Register in the natives aggregator; add a `help` entry with
  examples and a `REFERENCE.md` / `HOWTO.md` note documenting the
  `def ifu (usurp if)` cond-first idiom.

### Tests (positive + negative)

- `def ifu (usurp if)`: `true ifu 88 99 → 99`, `false ifu 88 99 →
  88`; `if true 99 88 → 99` unchanged.
- list condition: `[1 gt 0] ifu fail pass → pass` (NoEval +
  mark/move survive the wrapper).
- 2-arg no-else `cond ifu pass ≡ if cond pass`; clause-list `ifu
  [clauses] ≡ if [clauses]`.
- typed word: `def subr (usurp sub)`, `subr a b ≡ sub b a`.
- check mode produces sane carriers (body delegation reuses `if`'s
  `ReturnsFn`).
- negatives: usurp a non-word, usurp a barrier-sig word, type-
  literal input → no panic.
