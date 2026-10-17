# borueng spec coverage — author's report

This report accompanies the new TSV files added under
`borueng/test/spec/`. It does NOT propose fixes; per the brief, it
records what each new file pins down, then catalogues every
*observed deviation* between borueng's Go engine and the language
design as documented in `design/*.md` (notably
`LANGREF.10.md`, `ENGINE.10.md`, `ENGINE-UNIFIED-ALGO.8.md`,
`SIGNATURES.10.md`, and `TYPES.10.md`).

All rows in every file pass under
`cd test/go && go test ./engspec/`. Where a row encodes
behaviour that contradicts the docs, the deviation is called out
both inline (in the row's `note` column) and below.

---

## 1. New spec files

| File | Principle |
|---|---|
| `numbers.tsv` | Integer + Decimal arithmetic, contagion, three op kinds (commutative/non-commutative/unary), parens, end, custom fns. Replaces the old separate `arithmetic.tsv` and `decimals.tsv`. |
| `none.tsv` | `null` / None bottom-type round-trip and sig rejection |
| `mirror.tsv` | Both split-classes of a 2-arg call, pinned against sig-orientation regressions |
| `multivalue.tsv` | Multiple values residual on the stack are the result |
| `nested-forward.tsv` | Function words are structural boundaries; parens nest |
| `empty.tsv` | Empty list / empty string handling without panics |
| `code-splice.tsv` | List-bound def splices its body inline at use site |
| `leftright.tsv` | Strict left-to-right with no operator precedence |
| `mismatch-stop.tsv` | Type-directed forward halts on mismatch, residue stays |
| `shadow.tsv` | Def-stack shadowing semantics |
| `typeliteral.tsv` | Type-name words push type-literal values |
| `resolution.tsv` | Word resolution priority order |
| `lists-inert.tsv` | Lists are inert at tokenize; auto-eval at consume / EOR |

The pre-existing files cover: `literals`, `arithmetic`, `dispatch`,
`multiarg`, `barrier`, `stack`, `quote`, `list`, `end`, `paren`,
`def`, `fn`, `fn-typed`, `args`, `autoeval`, `insertforward`,
`markmove`, `force`, `pattern`, `errors`, `strings`. Together with
the new files, the spec set now has **35 files** and **396 rows**;
all pass.

---

## 2. Deviations from the documented design

Every deviation below is encoded in at least one passing row in
the corresponding TSV file. The deviation pattern is consistent:
the engine's actual behaviour is *more permissive* than the docs
imply, in ways that can mask bugs in user code.

### 2.1 Type literals satisfy concrete-scalar sigs (typeliteral.tsv)

**Doc claim** (LANGREF / TYPES): `Integer`, `String`, `Boolean`,
`Decimal` are *type literals* — values whose Data is nil — and
should not be confusable with the concrete scalar payload.

**Observed**: `match.go::matchSignature` rejects `Data == nil`
type literals only for `TMap` and `TList`:

```go
if tok.Data == nil && tok.Parent.Equal(expectedType) &&
    (expectedType.Equal(TMap) || expectedType.Equal(TList)) {
    break // reject type literals for concrete Map/List
}
```

For scalar types this guard does NOT fire — so:
- `tag Integer` matches the `[Integer]` sig (returns `"specific"`).
- `add Integer 1` succeeds; the handler's `AsInteger()` silently
  returns `0` for the type literal, producing `0 + 1 = 1`.
- `add Integer Integer` returns `0`.

**Impact**: A typo where the user intended a number but typed a
type-name word silently treats the type literal as the zero value
of its scalar payload. The error never surfaces.

**Pinned by**: `typeliteral.tsv` rows L62–L68 (rendering quirks)
and L72–L74 (silent-zero arithmetic).

### 2.2 Renderer asymmetry on type literals (typeliteral.tsv)

**Doc claim**: A type literal is the *type* as a value; printing
it should show the type, not a default-zero scalar.

**Observed**: The spec runner's `renderValue` (in
`test/go/engspec/engspec_test.go::renderValue`) goes through the
`v.Parent.Matches(TInteger)` branch first. For a type literal
(`Data == nil`), `AsInteger()` returns `0, false` — so the int
branch happily formats "0":

```
Integer  → "0"
Decimal  → "0"
String   → ""
Boolean  → "false"
```

By contrast, types whose Parent doesn't match a scalar leaf fall
through to the default `v.String()` and print the type path:

```
Number   → "Scalar/Number"
Atom     → "Scalar/Atom"
List     → "Node/List"
Map      → "Node/Map"
Any      → "Any"
None     → "null"        (special-cased branch)
```

The asymmetry is purely a renderer issue — the underlying values
are correctly type-literal — but the output is misleading and the
spec runner can't write a row that reliably distinguishes "the
Integer type literal" from "the integer 0".

**Pinned by**: `typeliteral.tsv` rows L62–L68.

### 2.3 Function words are structural boundaries — no implicit nesting (nested-forward.tsv)

**Doc claim** (LANGREF "Nested forward collection"):
> "When one word is waiting for arguments and encounters another
> word, the inner word collects *its* arguments first. The inner
> word's result then becomes an argument to the outer word."

**Observed**: `match.go` explicitly stops the forward scan when
the next token is a registered word:

```go
// 1.4: function word — boundary, stop.
if e.registry.Lookup(ww.Name) != nil {
    break
}
```

So `add 1 mul 2 3`:
- `add` scans forward, matches `1` for sig[0] (fwd=1), then sees
  `mul` and breaks at fwd=1.
- `add` requires sig[1], stack is empty, no match → `signature_error`.

The "inner word collects first" behaviour described in the docs
only works when the inner is wrapped in parens — the engine's
`preEvalParens` pre-evaluates them in the forward-scan window so
the matcher sees a concrete value.

**Impact**: Every example in LANGREF that shows nested forward
without parens (e.g. `def Point record [x:number y:number]`) is
the docs assuming the documented model; borueng's engine requires
parens. The `def Point record [...]` form *does* work, but only
because `def`'s sig has a `TWord` slot for the name (which captures
without dispatch) and its second slot is `TAny` (which accepts the
record's output by stack-fall-through after `record [...]` runs).

**Pinned by**: `nested-forward.tsv` rows L37–L41 (errors) and
L44–L49 (paren-wrapped successes).

### 2.4 List-body def splice happens before later tokens evaluate (code-splice.tsv)

**Doc claim** (LANGREF "Lists are quotation"):
> "When a defined word's body is a list, its elements are spliced
> into the token stream on use. This is how `def` creates reusable
> code fragments."

What the docs *imply* is that splice + later-token evaluation
compose naturally. What they don't say is that the splice is
*eager*: the body's tokens land in the stream IMMEDIATELY, ahead
of any later tokens.

**Observed**: `def x 5 def doub [dup add] doub x` errors:
- `def x 5` registers x = 5.
- `def doub [dup add]` registers doub.
- `doub` splices `[dup add]` at the pointer.
- `dup` runs against an empty stack (5 was consumed by def, x
  hasn't been processed yet because doub's splice is between
  doub's position and x).
- `dup` requires one stack value → `signature_error`.

The intuitive reading "doub doubles the next argument" is wrong
— the body splices into the program text, so it sees whatever is
already on the stack at splice time, not what comes after.

The workaround is `5 doub` (push first, then splice) or wrap the
inner call in a fn with named params (which DOES forward-collect
because the synthesised native is forward-prec).

**Pinned by**: `code-splice.tsv` rows L67–L80 (basic), L93
(parens isolate), and the failure row at L90.

### 2.5 Auto-eval errors are silently swallowed at consume time (lists-inert.tsv)

**Observed**: `engine.go::execMatch` calls `autoEvalList` on each
list arg, but the error is *discarded*:

```go
evaluated, err := e.autoEvalList(match.Args[i])
if err == nil {
    match.Args[i] = evaluated
}
// err != nil → match.Args[i] is unchanged (the original list)
```

So `length [add]`:
- length consumes `[add]`.
- autoEvalList sub-runs `[add]` → `signature_error`.
- The error is dropped; `match.Args[0]` keeps its original
  1-element form.
- length handler returns `1`.

Whereas `[add]` alone (residual at end-of-Run) errors loudly via
`autoEvalStack` — the same `autoEvalList` call, but the error is
NOT swallowed. Two different error-handling policies for the same
operation.

**Impact**: A typo or undefined word inside a list arg silently
leaves the typo in place. Static analysis can't catch it because
no diagnostic surfaces.

**Pinned by**: `lists-inert.tsv` rows L74–L76 (silent swallow),
L79–L81 (loud propagation at end-of-Run). The doc deviation: the
docs don't acknowledge the split policy at all.

### 2.6 `def` shadows registered builtins via the same def stack (resolution.tsv, shadow.tsv)

**Doc claim** (TYPES, ENGINE): names live in two separate stacks
— `r.types` for type bindings and `r.defStacks` for value /
function bindings. Resolving a Word at the pointer goes through
priority: type-stack → def-stack → registered native → boolean /
type-name → undefined.

**Observed**: The behaviour is correct, but two consequences are
under-discussed:

1. `def add 99` makes the registered native `add` *unreachable*
   from any ordinary call site until the def is popped. Because
   the def-stack lookup runs before the function-lookup, the def's
   integer wins; the engine then runs through `stepLiteral`
   (literal push), not function dispatch.

2. So `add` (the symbol) flips between two completely different
   semantics depending on the def stack state. There is no warning
   at the def site, no warning at the use site.

**Impact**: Library authors can't rely on a builtin name remaining
callable after user code runs. Pin the rule so any future scoping
work (e.g. lexical-scope defs) can preserve or change this with
explicit intent.

**Pinned by**: `resolution.tsv` rows L62–L66, `shadow.tsv` rows
L43–L48.

### 2.7 The two split-classes of a 2-arg call (mirror.tsv)

**Doc claim** (CLAUDE.md "Argument Ordering"): one rule at every
arity — the forward phase fills sig positions in written order up
to the barrier, the stack phase fills the rest top-down. For a
2-arg forward-prec word `f` that yields two classes: `f a b`,
`b f a` and `b a f` all bind sig=[a,b], while `a f b` and `a b f`
bind sig=[b,a]. Neither class is a special case or a "swap form" —
they differ only in which operand the split leaves in sig[0]. The
phase-4 handler convention has every binary math handler compute
`args[1] - args[0]` (for sub) so the infix spelling reads as
written: `10 sub 3 = 7`.

**Observed**: This works as documented; the deviation is
*social*, not engine-level. The classes are documented in prose in
CLAUDE.md, but spec rows for them were spread across `numbers.tsv`
and `strings.tsv` and easy to miss. `mirror.tsv` now collects both
classes for `sub` (integer) and `concat` (string) so a regression
in sig orientation is caught immediately.

**Pinned by**: `mirror.tsv` rows L34–L43 (no behavioural
deviation; documentation regression-test only).

### 2.8 No /N modifier in the spec runner (parity gap)

**Doc claim** (LANGREF "Word Modifiers"): `/N`, `/Nf`, `/Ns` modify
word dispatch to require exactly N args.

**Observed**: The spec runner's `tokenizeSpec` only parses `/s`
and `/f`. The `/N` family would require extending the tokenizer
(strip a numeric suffix) and the engine already accepts an
`ArgCount` field on `WordInfo`, so the support is half-built.
This is a parity gap, not a deviation — but worth noting because
no spec row currently exercises `/N`.

**Pinned by**: nothing — outside the scope of the current set.

### 2.9 Type-stack priority not exercised (parity gap)

**Doc claim**: types and defs live in separate stacks; `type Foo …`
binds Foo on the type stack, which beats a `def Foo` on the value
stack.

**Observed**: The spec runner doesn't register a `type` word, so
the type stack is empty in every test. Type-stack priority is
unobservable from the current spec set.

**Pinned by**: nothing — would need a registered `type` word.

### 2.10 Empty-string concat collapses the two split-classes (empty.tsv)

**Doc claim**: the two split-classes of a 2-arg call bind the
operands to opposite sig positions (§2.7). For non-commutative `f`
the two classes therefore give different results.

**Observed**: For `concat`, both classes give `"x"` when one of the
args is `""`. The reason is that `""` is the identity for string
concatenation, so the orientation stops mattering. Other rows in
`mirror.tsv` (non-empty strings, `"ab" "cd"`) keep the two classes
distinguishable. This is a degenerate-input observation, not an
engine deviation.

**Pinned by**: `empty.tsv` rows L74–L77.

---

## 3. Surprises that are NOT deviations

These behaviours look surprising at first read but match the docs
exactly. Including them so future readers don't waste cycles
re-investigating.

- **`def` consumes both args without leaving residue**: `def x 5`
  pushes nothing onto the stack; the value 5 was an arg to def,
  not a literal push. Confirmed via `shadow.tsv` rows L51–L52.

- **Auto-eval at end-of-Run runs sub-engines on residual lists**:
  `[1 add 2]` alone evaluates to `[3]`. Confirmed via
  `lists-inert.tsv` rows L62–L66.

- **`(empty body)` is a clean no-op**: `def noop [ ] noop`
  produces an empty stack, NOT an error. The splice removes the
  word and inserts nothing. Confirmed via `code-splice.tsv` row
  L101.

- **Booleans and `null` round-trip through token + render
  symmetrically**: `tokenizeSpec` recognises `true` / `false` /
  `null` as bare words and converts them to typed values; the
  renderer prints them in the same form. Confirmed across many
  files.

- **Forward-collect treats `mul` as a boundary even though it
  doesn't try to dispatch the inner word as data**: see §2.3.
  This is the same rule as "function names can't be data" — to
  treat a function name as data, use `quote`.

---

## 4. Coverage gaps that are intentional

- **No tests for typed lists `[:Integer]` or typed maps
  `{:String}`**: the spec runner's tokenizer has no syntax for
  these, and the spec set focuses on the unified-dispatch core.
  Typed-collection sigs are covered in `lang/go/test/*.tsv` against
  the full parser.

- **No tests for `if` / `for` / `do` / `var`**: those words are
  not registered in the spec runner. Control flow lives in the
  full engine and its tests.

- **No tests for the carrier / static-type-check mode**: the spec
  runner uses runtime mode only. CheckMode is exercised via Go-
  level tests in `core/go/carrier*.go` and `lang/go/test/`.

- **No tests for module / import / export**: spec runner has no
  module subsystem.

- **No tests for template strings or interpolation**: tokenizer
  doesn't parse backticks.

- **No tests for the `/N` modifier or its combinations**: see
  §2.8.

These all live in the larger boru test corpus
(`lang/go/test/*.go` and `lang/go/test/*.tsv`); the borueng spec subset is
deliberately focused on the dispatch / value / type-lattice core.

---

## 5. Test-row count summary

| Category | Files | Rows |
|---|---|---|
| Pre-existing | 21 | ~190 |
| Added in this round | 14 | ~206 |
| **Total** | **35** | **396** |

All 396 rows pass under `cd test/go && go test ./engspec/`.

---

## 6. Suggested follow-ups (not implemented)

In priority order:

1. **Make `match.go` reject type literals for scalar leaves too**,
   not just Map and List (§2.1). One-line change to add `||
   expectedType.Matches(TScalar)` to the existing guard.

2. **Make `renderValue` check `IsTypeLiteral` first** before
   diving into the scalar-Matches branches (§2.2). Renders type
   literals as their type path uniformly.

3. **Stop swallowing `autoEvalList` errors in `execMatch`** (§2.5).
   Either propagate (matches end-of-Run policy) or tag the diag-
   nostic and continue with the unevaluated list — but NOT swallow.

4. **Document the function-word-as-boundary rule in LANGREF**
   (§2.3). The current "Nested forward collection" example
   (`def Point record [...]`) accidentally works; rephrase to
   either drop the unified narrative or add an explicit
   "use parens to nest forward calls" caveat.

5. **Document the eager-splice rule for list-body defs** (§2.4).
   Same surface — LANGREF currently presents def + list as if it
   were lazy.

None of these are in scope for the current task; they're recorded
here as a punch list for the maintainers.
