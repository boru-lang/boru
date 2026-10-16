# Forward-greediness "stranded operand" advisory

**Status:** accepted. Check-mode advisory, info severity, non-gating; enabled
by default under `boru check`.

## The gotcha

`1 2 add 3 mul end` evaluates to **5**, not the RPN-expected 9. `add` is
forward-eligible, so at the pointer it forward-collects the next token `3` and
takes `2` from the stack top (`3+2=5`), **stranding the `1`** below. `mul` then
multiplies `5 * 1 = 5`. The DX report flags this; the question was whether a
warning when "values are left unchanged on the stack after a word runs" would
help, and what its impact would be.

A naive "any leftover value" warning is unusable — in a concatenative language
leftover stack values are the norm (multiple results, stack idioms, pipelines).
This feature implements a **sharpened** predicate instead.

## The predicate

In check mode, when a word dispatch is **mixed** (forward-collected ≥1 arg AND
took ≥1 stack arg — i.e. the split falls inside the signature), flag it iff a **sibling
operand** is stranded: a value sitting on the stack just below the deepest stack
arg the word consumed, **in the same scope**, **whose type matches that consumed
slot**. The sibling-type test is what separates the genuine gotcha (a stranded
`Number` under `add`) from a deliberately-kept value of an unrelated type.

Discrimination:

| Source | Dispatch | Flag? |
|---|---|---|
| `1 2 add 3` | add: fwd `3` + stack `2`, `1` (Number) stranded | ⚠️ yes |
| `10 sub 3` | sub: fwd `3` + stack `10`, nothing below | no (infix form) |
| `1 2 3 add` | add takes top two from stack, no forward | no |
| `"hi" 2 add 3` | add: fwd `3` + stack `2`, `"hi"` (String ≠ Number) below | no |
| `(1 2 add) 3 mul` | grouped | no |

Implementation: `core/go/engine.go::checkForwardStrandsOperand`, called from
`stepWord` after the forward/stack counts are known. Gated on
`Check.IsActive()`; zero cost in normal runs. Emits code
`forward_strands_operand` (defaults to **info** severity → advisory, never
fails a check).

## Measured impact (false-positive rate)

Run `boru check` across real corpora and count `forward_strands_operand`:

| Corpus | Size | Advisories | Notes |
|---|---|---|---|
| Spec `.tsv` inputs | 5,713 expressions | **8 (0.14%)** | every hit is a genuine `X Y op Z …` forward-greedy pattern |
| Repo `.boru` files | 65 files | **0** | |
| voxgig trie + bloom `.boru` | 20 files, ~4.4k LOC | **0** | (these libs deliberately avoid the gotcha) |

All 8 spec hits classified:
- `1 2 add 3`, `1 2 add 3 add` — the canonical pattern at the call site.
- 4× a `fa3` fixture whose **body** `[a b add c add]` contains the pattern
  (the advisory points inside the body, col 58).
- The advisory's own suggested fix resolves them: grouping the body as
  `[(a b add) c add]` → 0 hits.

**Precision ≈ 100%** (8/8 hits are the real pattern; 0 innocent idioms flagged),
**noise ≈ 0.14%** of expressions. Many hits are *benign* — with a commutative op
the stranded operand doesn't change the result (`a b add c add` = `a+b+c`
anyway) — which is exactly why **info** severity (a readability nudge, not an
error) is the right level.

## Conclusion

- The literal "warn on any unchanged stack value" idea is a non-starter
  (false-positive catastrophe in a stack language).
- The sharpened **mixed-dispatch + sibling-operand** predicate is quiet enough
  to ship as a check-mode advisory: it caught the canonical gotcha and fired on
  nothing innocent across ~10k LOC of real code + 5.7k spec expressions.
- Limits: it only covers the forward-greedy subset (a deeper stranded value with
  *no* forward collection, e.g. `1 2 3 add`, is intentionally not flagged), and
  it relies on the sibling-type heuristic. It does not remove the underlying
  concatenative forward/stack ambiguity — it surfaces the most surprising case.

## Tests

- `lang/go/forward_strand_advisory_test.go` — fires on the gotcha at the
  top level, quiet on infix/stack/idiomatic forms and on the grouped fix,
  and asserts it is non-gating (0 errors). **In-body detection was
  deliberately dropped** (`TestForwardStrandAdvisory_QuietInBody`):
  bodies analyse against carriers, and running with concrete example
  args reintroduced the dynamic-help FP class.
