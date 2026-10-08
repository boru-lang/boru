# Silent forward-collection traps (DX investigations)

**Status:** investigations only — no code change. Two silent-failure traps
surfaced by the voxgig trie/bloom DX reports (`design/legacy/VOXGIG-BORU-REPORTS.5.ignore`,
bloom #3 and trie #6), both reproduced on the current build. Each is a case
where forward collection does something locally reasonable but globally
surprising, and fails **quietly** rather than loudly — the dominant cost the
DX reports call out.

---

## Trap 1 — a zero-value expression in an argument slot is silently skipped

### Symptom (bloom #3)

Binding the result of a void-returning call derails the *next* word:

```boru
def Bits (refine Object {})
def mark fn [ [i:Integer b:Bits] [] [ b 1 (convert String i) set ] ]
def b (make Bits {})
def _ (mark 5 b)
"ok" print            # error: no matching signature for print
```

### Scope (verified)

Broader than mutators or the `_` name — it is **any** zero-value expression in
**any** forward word's argument slot:

| Input | Result | |
|---|---|---|
| `def x (noop)` then `"ok" print` | `no matching signature for print` | any void fn |
| `def x (noop) 42` then `x` | `x = 42` | **silently binds the next token** |
| `def x () 42` then `x` | `x = 42` | empty paren `()` behaves identically |
| `def x noop 42` then `x` | `x = 42` | bare void word (no paren) too |
| `add 1 (noop) 5` | `6` | not `def`-specific |
| `add 1 (5 drop) 2` | `3` | any zero-value expression |
| `add 1 (if false [9] []) 2` | `3` | an empty `if` branch yields **zero** values |

where `def noop fn [[] [] []]`.

### Root cause

Forward collection counts **values arriving at the pointer**, not tokens. In
`stepLiteral`'s collection block (`eng/go/engine.go:2006-2049`) each value that
reaches the pointer while a `Forward` marker is pending is collected and
`CollectedArgs++` (`:2049`); the word fires when `CollectedArgs >=
ExpectedArgs`.

A zero-value expression produces **no arrival** — the paren simply collapses to
nothing and the pointer advances to the next real token, which is then
collected into that slot. Paren evaluation (`stepCloseParen` /
`evalParenGroupAt`) is architecturally decoupled from arg-slot accounting, so
the collecting word cannot tell *"my argument vanished"* from *"that was a
side-effect statement and this next token is my argument."* Hence the
mislocated failure: with a following token, the word binds the wrong one and a
*later* word starves; with none (`… end`), the word itself starves
(`no matching signature for def`).

### Why a blanket runtime error is wrong

"Zero values in an argument slot → error" is too aggressive: `(if c [v] [])`
with the empty branch taken legitimately yields zero values, so
`add 1 (if c [t] []) 2` (conditionally contribute a term) is reasonable boru and
would start erroring.

### Recommendation (deferred — investigation only)

The two cases separate cleanly:

- **Binding words (`def`/`var`):** binding a name to a zero-value expression is
  never useful and today fails confusingly (wrong token bound, or an error two
  statements later). A clear, located error here is unambiguously better and
  safe — e.g. `def: (mark 5 b) produced no value to bind to "_" — give the call
  a return value, or call it as a bare statement`.
- **General words:** leave runtime behaviour (the conditional-term pattern is
  valid); add a **check-mode advisory** when a *void-declared* fn's result is
  consumed as a forward argument (return-type info is available statically).
  Non-gating, consistent with `forward_strands_operand`.
- **Docs:** note the trap + workarounds.

Implementation catch: the zero-value paren never reaches the collector, so a
binding-error must be raised at paren-collapse / forward-completion time by
consulting the pending `Forward`. Contained, but on the hot path; and "which
words are binding" is a language-design call with test blast radius (e.g. the
`addq 1 ( )` spec row, which currently expects a generic signature error).

---

## Trap 2 — a bare word as a `get` key is a literal key, like JS `.key`

### Resolution: document only — this is intended, JS-equivalent semantics

This is **not a bug** and needs no behaviour change. It is the direct analogue
of JavaScript member access, with `()` playing the role of `[]`:

| JavaScript | boru | meaning |
|---|---|---|
| `xs.i` | `xs get i` | literal key/property named `i` |
| `xs[i]` | `xs get (i)` | computed key — the **value** of `i` |

So `xs get i` looks up the key `"i"`, and `()` forces evaluation just as `[]`
does in JS. The trie author hit it because the mental model wasn't documented,
not because the behaviour is wrong. The resolution is a docs line, nothing more.

### Symptom (trie #6)

```boru
def xs [10 20 30]
def i 1
xs get i        # => None      literal key "i" — like xs.i
xs get (i)      # => 20        computed — like xs[i]
xs get 1        # => 20        literal index
```

### Scope & root cause (verified)

`get` is overloaded with an **`[Atom …]` key family** alongside the
`[Integer …]` / `[String …]` families (`describe get`):

```
[ [Integer Array]   Any ]   [ [String Node]  Any ]
[ [Integer Node]    Any ]   [ [Atom   Node]  Any ]   ← captures a bare word
[ [Atom    Object]  Any ]   [ [Atom   Store] Any ]   …
```

The `[Atom …]` signatures exist on purpose: they make `m get key` (and the
desugaring of `m.key`) work, where `key` is meant as a **literal name**, not a
variable. So during forward collection a following **bare word** is captured as
an `Atom` (the Word→Atom path in the collector, `eng/go/engine.go:2010-2017`)
and matched against `[Atom Node]` — *before* the def stack is ever consulted.
Consequences, all confirmed:

- `xs get i` → the atom key `"i"`, absent from the list → `None`.
- `xs get i` with `i` **undefined** → still `None` (not `undefined word`!) — the
  variable is never looked up, so the usual strict-undefined rule never fires.
- `m get a` where `a` is **both** a defined value (`0`) and a map key (`a:99`) →
  `99` — the literal-key interpretation wins; the binding is shadowed.

All three are exactly the JS `.key` semantics: a literal name, evaluated
against the container, with no variable resolution and no error on a missing
key. The `i`-undefined-still-`None` case is `xs.i` returning `undefined` in JS,
not a defect; the `a`/`99` case is `m.a` reading property `a`, not the variable
`a`. The only disambiguator — and the JS-`[]` analogue — is parens: `xs get
(i)`.

### Resolution

**Documentation only.** No runtime change, no check advisory (that would warn
on correct, idiomatic code). Add a docs line wherever `get`/`set`/dot-access is
explained:

> A bare word key is a **literal** name — `xs get key` is `xs.key`. To use a
> variable (or any expression) as the key/index, wrap it in parens:
> `xs get (i)` is `xs[i]`.

A worked `i`-vs-`(i)` example next to the `get` reference closes the gap the
trie author hit.

---

## Common thread

Both surfaced as "bare word does something I didn't expect," but they are
different in kind:

- **Trap 1 is a real silent failure.** A vanished argument is reached past, and
  the result is a wrong binding or a mislocated error — the class the DX reports
  flag as costliest. Worth a fix (loud error for binding words; advisory
  elsewhere), in the spirit of the shipped `forward_strands_operand` advisory
  (`design/FORWARD-STRAND-ADVISORY.10.md`).
- **Trap 2 is working as intended** — JS `.key` vs `[expr]`, with `()` for `[]`.
  The only gap is that the mental model wasn't written down. Documentation
  closes it; no code change.

The shared lesson is narrower than "forward collection is surprising": where
behaviour is genuinely wrong (Trap 1) make it loud; where it is a correct but
unstated convention (Trap 2) write it down.
