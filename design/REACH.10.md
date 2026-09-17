# Reach — a first-class node for dot-access sugar

**Status:** design / implementation plan — **Phases A–E + the §11 deferred
work (F–J) landed (green)**
**Scope:** lift dotted access (`m.a.b`, `a."x".c`, `a.(expr).b`, `a!.x`) from
its current flat `get`-chain (a `ParenExpr` after the paren-nesting work) to a
**first-class, quotable, round-trippable `Reach` value** with per-segment
operator and literal-or-computed keys.
**Predecessor:** `legacy/PAREN-REPRESENTATION.9.ignore` §6/§9 (the "dotted access is the
residual" finding) and `legacy/LISP-ANALYSIS.5.ignore` §2/§8 #5 (uniform code-as-data).
**Decisions (this plan):** first-class runtime value · motivation = code-as-data
**and** programmatic access · computed keys supported in the node.

> **Naming.** The concept is **`Reach`** (the constructor word is **`reach`**),
> reading as "reach into `m` for `a.b`". It is deliberately *not* "Path"
> (clashes with the unrelated `Scalar/Path` filesystem type) and *not* "Access"
> (evokes permissions/security, the wrong concept).

> **Implementation status (landed, green):**
> - **A** `Ideal/Reach` type (FixedID 29) + `ReachInfo`/`ReachSeg` payload +
>   `NewReach`/`IsReach`/`AsReach`.
> - **B** parser emits a `Reach` for dot-chains; Stage-1 lowering evaluator at
>   all four `ParenExpr` sites — exact semantic parity across the six §5
>   auto-eval contexts, getr strictness, computed/string/numeric keys, paren
>   receivers, and `size m.a` grouping. `codequote m.a` captures it
>   (`typeof → Reach`).
> - **C** `canon` round-trips a `Reach` to `m.a.b` / `m!.x` / `m.'k'` /
>   `m.(expr)` / `(expr).k`; a Quoted reach wraps `(codequote …)`. `Format`
>   makes `Value.String()` match.
> - **D** `reachConvertBehavior` (`convert Map` → `{receiver, segments}`,
>   `convert List` → keys); the **`reach`** constructor builds an *inert*
>   (Eval=false) lens; an `isEvalReach` gate keeps inert/quoted reaches as data
>   while parsed ones auto-evaluate.
> - **E** *no code needed.* The Step-7 `dotchain` +17% (ParenExpr form) is
>   **erased by the representation change itself** — a compact `Reach` node
>   parses cheaper than a 9-token get-chain, more than offsetting the Stage-1
>   lowering. Same-session A/B (`dotchain`, median of 5): **Reach 132.7µs vs
>   the pre-paren marker baseline 136.3µs (~3% faster)**, vs the ParenExpr
>   regression ~168µs. The **Stage-2 direct-walk is therefore unnecessary** and
>   intentionally not done (it would require moving get/getr access into eng —
>   large and risky for no gain), mirroring the paren-work Step 5 decision.
>
> **Deferred work — now LANDED (green), Phases F–J:**
> - **F  Receiverless reach** — a `$` receiver (the reserved sentinel) parses
>   to a receiverless, inert Reach: a detached lens `$.name` that evaluates to
>   itself. `canon` round-trips it. All-`$` names (`$`, `$$`, …) are reserved
>   as user words (`ValidateWordName`), mixed-`$` names ($path) stay legal.
>   (Chosen over leading `.a.b`, which is ambiguous — `.` is a whitespace-
>   insensitive standalone token, so `. a . b` ≡ `.a.b` has no syntactic way to
>   mark "no receiver".)
> - **H  apply + rebind** — `eng.ApplyReach(r, info, recv)` evaluates a reach's
>   segments against a receiver (the lens "get", honoring getr strictness +
>   computed keys). `p $.name apply` rebinds + evaluates (stack form —
>   since NUR098's fix, 2026-08-24, `apply` is stack-only in both
>   overloads, so the earlier forward spelling `apply $.name p` refuses);
>   `rebind $.name p` composes an inert bound lens (rebind stays
>   forward-eligible).
> - **J  Lens-as-Function** — a receiverless reach is an arity-1 accessor in
>   higher-order positions: `each $.name people`, `filter $.active xs` (reads
>   the element, not the {key,value} wrapper), `sortby $.age people`. Each word
>   gains an `[Reach, …]` overload delegating to `ApplyReach`.
> - **I  getpath/setpath accept a Reach** (full native) — `getpath $.a.b m`
>   reads via `ApplyReach`; `setpath $.a.b v m` is an immutable NESTED set
>   walked natively (`setReachNative`) — preserves siblings, creates
>   intermediates, handles list indices + computed keys. The dotted-**string**
>   `setpath` now shares the same native setter too: `voxgigstruct.SetPath`
>   returned the innermost sub-node instead of the updated root (every nested
>   set silently dropped the outer structure), so both forms route through
>   `setReachNative`.
> - **G  `reach` constructor encoding** — the key list is NOT evaluated and
>   encodes one segment per key: bare key = get, `!` marks the next key getr
>   (`reach m [a !b]` ≡ m.a!.b), `(expr)` = a deferred computed key.

> The dot-chain is the last non-uniform corner of "code as data": today
> `m.a.b` is an *idiom* (`( m get a get b )`), not a *node* — you can't quote
> it, the printer can't round-trip it as `m.a.b`, and a walker must
> pattern-match the get-chain. This plan makes a reach a real value:
> inspectable, constructible, applicable (a lens), and rendered back as
> `m.a.b`. It also removes the Step-7 `dotchain` +17% (no more
> `ParenExpr`→marker round-trip on access).

---

## 1. Current grammar (confirmed)

`convertTopLevelItems` (parse.go) accepts: a **receiver** (`isChainReceiver` —
any value/word/string/number/paren group) followed by one or more **segments**,
each `.key` → `get` or `!.key` → `getr`, where the **key** is a literal
(word/string/number) *or* a computed `(expr)`. The whole chain lowers to a
single `ParenExpr([recv get/getr k …])`. A `/`-modifier applies to the **whole
result** (`/u`/`/s`/`/f`/`/N` → prefix words; `/r`/`/q` → suffix `__DM`).

Verified forms: `a.x.c`, `a."x".c`, `a.(k).c`, `m.(key)`, `a.0`, `a!.x`,
`(m.a).b`. `get` is lenient (missing → `None`); `getr` is strict (missing →
error).

`Scalar/Path` (filesystem: `PathInfo{Parts []string, Abs bool}`) is unrelated
and untouched; the data-access concept is the new `Ideal/Reach`. The
`boru:struct-util` list-path words keep their names (`getpath`/`setpath`/
`inject`) and gain the ability to accept a `Reach`.

---

## 2. The type and payload

A new kernel-declared type (the parser emits it, so eng must know it at parse
time — like `Word/__PE` for `ParenExpr`), in the **Ideal** branch for
first-class value semantics:

```
Ideal/Reach             // typeof → Reach; inspectable, quotable, a lens
```

Payload (eng `payload.go`, sealed marker):

```go
type ReachInfo struct {
    Receiver []Value     // tokens of the receiver expression (e.g. [Word("m")],
                         //   or a ParenExpr for (expr).k); evaluated to the base.
                         //   Empty = a receiverless reach (an accessor/lens; §7, §11).
    Segments []ReachSeg
    Eval     bool        // evaluate-by-default (like list Eval); quote/codequote suppress
}

type ReachSeg struct {
    Getr     bool        // false = get (lenient) · true = getr (strict)
    Computed bool        // true → Key is a sub-expression evaluated at reach time
    KeyLit   Value       // literal key (Atom / String / Integer) when !Computed
    KeyExpr  []Value     // computed-key tokens when Computed (e.g. (expr))
}
```

`FixedID`: next free slot in the documented `5000–9999` kernel/language range
(Module 5000, ModuleExport 5001 → e.g. **5002**); add to the
`fixedid_stability_test.go` snapshot. Behavior: a custom `TypeBehavior` whose
`Format` renders the canonical dotted surface (§6) plus an `IdealConverter`
projecting to an inspectable map (§7).

---

## 3. Parser change

In `convertTopLevelItems`, the dot-chain branch builds a `Reach` value instead
of a `ParenExpr` get-chain:

- **Receiver** — `emitPrimary` the first item into `Receiver` tokens (a word,
  scalar, or a nested `ParenExpr` for `(expr).k`).
- **Each segment** — record `Getr` (`.` vs `!.`) and the key: a literal
  (`KeyLit` — the word becomes an Atom; string/number as-is) or, for a paren
  key, `Computed=true` with `KeyExpr` = the paren's tokens.
- **Modifiers** — unchanged: the `pfx`/`sfx` tokens wrap the resulting `Reach`
  value exactly as they wrap the group today (`/u`→`usurp (reach)`,
  `/r`→`(reach) __DM`). The node is a single value, so the existing wrap logic
  applies verbatim.

Stays within the single-pass converter — no new rewrite pass.

---

## 4. Evaluation

A `Reach` with `Eval=true`, when **stepped** at the pointer / **collected
unquoted** as a forward arg / **left on the stack** at end of Run,
auto-evaluates to the reached value — mirroring list `Eval`, so `m.a.b` stays
eager exactly as today. `quote`/`codequote` suppress it (§7).

Two implementation stages:

1. **Correctness-first (lower to get-chain).** Evaluate by materialising the
   `recv get/getr k …` token span (computed keys splice their `KeyExpr`) and
   running it in place — the same proven mechanism `ParenExpr` uses
   (`expandParenExpr`/in-place collapse). Guarantees identical get/getr
   semantics, recorder transparency, and the four paren contracts.
2. **Perf (direct walk).** Replace the lowering with a direct evaluator: eval
   `Receiver` → base, then per segment call the `get`/`getr` handler directly
   with the literal/evaluated key. Removes the Step-7 `dotchain` +17% (no marker
   round-trip). Gate behind the spec suite; ship only if benchmarks confirm.

Semantics are **unchanged** — the node lowers to / calls the same `get`/`getr`.

That identity is a **contract on the SEGMENT, not only on the chain**: `m.k`
and `m get k/q` differ in spelling only, for every `k` a user can write. It
held for `def`, `fn`, `if`, `case`, `true` … and failed for the parser's
reserved VALUE literals — `none`, `end`, `inf`, `-inf`, `nan` — which
resolved to their values before the chain saw the segment, so no `dot`
signature matched while `get` read the field (NUR066). A segment spelled as
a bare name is now the NAME, derived from what `parseWord` produced rather
than from a list, so a new literal cannot silently re-open the hole. The
punctuation-spelled markers are excluded (they are not word names), which is
why the grammar hands `;` its own text instead of rewriting it to `end`:
`m.end` reads a field, `m.;` is a stray terminator and still an error.

---

## 5. Preservation contract (does this keep the existing sugars?)

**Yes — by construction**, because eager access is preserved the same way lists
preserve theirs: `Eval=true` + auto-evaluation. Bare `m.a.b` yields the
*reached value*, not a `Reach`; only `quote`/`codequote` produce the node.
Everything else falls out of the node being a **single forward token** (exactly
what the `ParenExpr` was):

| Sugar | Preserved? | Why |
|---|---|---|
| `m.a.b` → value (eager) | ✅ | `Eval=true` auto-eval |
| `size m.a` ≡ `size (m.a)` (grouping) | ✅ | one token in the forward window |
| `/r` `/u` `/q` `/N` on a reach | ✅ | `pfx`/`sfx` wrap unchanged (wraps the node) |
| `!.` strictness, string/numeric/computed keys, `(expr).k` receiver | ✅ | captured per-segment / in `Receiver` |

**The obligation (and the one real regression risk):** the `Eval` auto-eval
must fire in **every context** the current get-chain evaluates in, or a `Reach`
leaks as *data* where a value is expected. These contexts are the Phase-B parity
gate and must each be tested:

1. stepped at the pointer (statement position)
2. collected unquoted as a forward arg
3. end-of-`Run` stack drain (`autoEvalStack`)
4. as a **list element** — `[m.a m.b]`
5. as a **map value** — `{x: m.a}` (via `autoEvalMap`)
6. as a fn arg / inside a fn body

**One non-runtime change (expected churn, not a break):** `canon` now renders
dotted access back as `m.a.b` (the round-trip — §6) instead of the get-chain
idiom. Golden/spec rows pinning the *old* canon of a dotted expression update —
the same flavor of churn as the parser-test updates in the paren flip.

---

## 6. Canon round-trip (the code-as-data win)

`canon`/`Format` renders a `Reach` back to its surface: `m.a.b`; getr → `!.`;
string key → `."x"`; computed key → `.(expr)` (canon of `KeyExpr`);
`ParenExpr` receiver → `(expr).k`; receiverless → leading `.a.b`. Guarantee
`read ∘ print` round-trips: `canon (codequote m.a.b)` → `m.a.b`. Add to the
printer round-trip audit (LISP-ANALYSIS §8 #10). This is what makes a quoted
reach *walkable* (one node with `Receiver` + `Segments`) rather than an idiom.

One rendering follows from §4's lowering identity rather than from the
surface: the **end marker inside a reach prints as `;`, never as `end`**. A
bare `end` in reach-token position is a WORD — a field name in a segment, an
ordinary word in a paren body — so spelling the marker `end` would re-parse
as that word and silently change the program. The marker can only have come
from a `;`, and `;` is what re-parses to it (NUR066, resolved 2026-08-15).

---

## 7. First-class surface — a `Reach` is a lens

Because the answers were **first-class value** + **programmatic access**, the
unifying frame is that a detached `Reach` is a **lens/optic** (a structured
getter, and with the write ops, a getter+setter). Operations, grouped and
prioritized:

**Construct**
- `reach m [a b]` — build programmatically (literal segments, all `get`).
  A richer list encoding expresses getr/computed segments (e.g. `!name` = getr,
  `(expr)` = computed) — surface TBD (§11).
- `codequote m.a.b` — capture a reach from source (non-evaluated node).

**Apply — the lens core** *(the reason to make it first-class)*
- evaluate (already how `m.a.b` works) → the value at the reach.
- **rebind the receiver:** apply a reach to a *different* value — the
  reusable-accessor use, one `.name` reach applied across many records
  (`people each .name`). Requires receiverless reach (§11).
- read/write/update **reuse** `boru:struct-util` rather than inventing words:
  teach `getpath`/`setpath`/`inject` to accept a `Reach`, giving the lens trio
  `getpath r m` (read), `setpath r v m` (immutable set), `update r fn m`
  (functional over), plus `delpath`/`has`.

**Inspect / decompose**
- `inspect` / `convert Map r` → `{receiver, segments:[{op,key}…]}`;
  `keys r` → key list; `length r`; `last r` (final key); `parent r` (drop the
  last segment).

**Compose**
- `append r k` / extend; concat two reaches; `rebind r x` (swap receiver).
  Composition is what lets a macro/optimizer walk and rewrite reaches.

**Identity / render**
- structural `eq`/`cmp` (same receiver + segments); `canon` → `m.a.b`.

| Op | Form | Notes |
|---|---|---|
| construct | `reach m [a b]`, `codequote m.a.b` | programmatic / from source |
| read | `getpath r m`, evaluate | reuses struct-util |
| set | `setpath r v m` | immutable update; reuses struct-util |
| update | `update r fn m` | functional over |
| exists | `has r m`, `delpath r m` | |
| inspect | `inspect r`, `convert Map r`, `keys`/`length`/`last`/`parent` | |
| compose | `append r k`, concat, `rebind r x` | |
| identity | `eq`/`cmp`, `canon` | structural; round-trips |

**Cautions:** (a) don't duplicate `struct-util` — read/write/update *extend* the
existing path words to accept a `Reach`; (b) keep `Scalar/Path` (filesystem)
and `Ideal/Reach` (data access) **distinct** — shared verbs may accept both
shapes, the types stay separate.

---

## 8. Touchpoints

- `eng/go/types.go` + `typetable.go` — declare `Ideal/Reach`, FixedID.
- `eng/go/payload.go` — `ReachInfo` + `ReachSeg` markers.
- `eng/go/value.go` — `NewReach`, `AsReach`, `IsReach`; behavior.
- `eng/go/parser/parse.go` — `convertTopLevelItems` dot-chain → `Reach`.
- `eng/go/engine.go` — main-loop / forward / `stepLiteral` evaluate an `Eval`
  `Reach` (Stage 1 lowering; Stage 2 direct walk); honor `Quoted`/`Eval`.
- `eng/go/canon.go` — round-trip rendering.
- `lang/go/native/` — `reach` constructor word; `apply`-on-reach; teach
  `getpath`/`setpath`/`inject` to accept a `Reach`.
- `lang/go/native/help/` + `describe` — document `reach` and the node.
- snapshots: `fixedid_stability_test.go`, fnmodel golden (new `reach` word).

---

## 9. Test plan

- **Parse**: every §1 form → a `Reach` with the right receiver/segments (op +
  literal/computed); modifiers wrap it.
- **Eval parity (the §5 contract)**: `a.x.c`, `a."x".c`, `a.(k).c`, `a.0`,
  `a!.x`, `(m.a).b` match today — in all six auto-eval contexts (statement,
  forward arg, end-of-Run, list element, map value, fn arg/body).
- **getr strictness** preserved (missing → error; `get` → `None`).
- **Round-trip**: `canon (codequote m.a.b)` → `m.a.b`; string/computed/getr
  segments render; `read ∘ print` idempotent.
- **First-class**: `reach m [a b]` builds an equivalent node; `getpath`/
  `setpath` accept a `Reach`; `inspect` projects the structure.
- **Quotability**: `codequote m.a.b` is a non-evaluated `Reach`; bare `m.a.b`
  still evaluates eagerly.
- **Negative**: malformed `reach` args; computed key that errors propagates;
  `Reach` type-literal no-panic (`TestTypeLiteralNoPanic`).
- **Perf**: re-run the Step-7 `dotchain` benchmark — Stage 2 erases the +17%.

---

## 10. Phasing (each gated green)

| Phase | Deliverable |
|---|---|
| A | Type + payload + `NewReach`/accessors + behavior stub (construct in tests; no parser change). Lattice/FixedID snapshot. |
| B | Parser emits `Reach` for dot-chains; **Stage-1 lowering** evaluator. Full suite green at semantic parity (the activating flip; pin all six §5 contexts). |
| C | Canon round-trip + quotability (`Eval`/`Quoted`); spec rows. |
| D | First-class surface: `reach` constructor, `apply`, `getpath`/`setpath` accept `Reach`, `inspect`/`convert`. |
| E | **Stage-2 direct-walk** evaluator; re-benchmark (erase the +17%). |

A–C deliver the code-as-data dividend (quotable, round-trippable reach); D the
programmatic capability; E the perf optimisation.

---

## 11. Risks & open questions

1. **Type placement** — the parser (eng) emits `Reach`, so the type is
   eng-declared even though it is "Ideal/first-class" (precedent: `Word/__PE`
   ParenExpr is eng-declared; `Ideal/Module` is lang-declared but the parser
   does *not* emit it — here it does).
2. **Two access-ish types** — `Scalar/Path` (filesystem) vs `Ideal/Reach`
   (data access). Distinct types; `getpath`/`setpath` accepting a `Reach` is
   additive (a third accepted shape), not a replacement.
3. **Eager-by-default vs value** — `m.a.b` must stay eager (the §5 contract);
   confirm `autoEvalStack` and the list/map auto-eval paths all evaluate an
   `Eval`, non-`Quoted` `Reach`.
4. **Computed-key side effects / ordering** — a computed key `(expr)` evaluates
   at reach time, left-to-right; Stage-1 lowering gets this free, Stage-2 must
   replicate.
5. **Receiverless reach (RESOLVED — Phase F).** The surface is the reserved
   `$` sentinel receiver: `$.a.b` → a receiverless lens, `people each $.name`,
   `sortby $.age people`, `setpath $.config.timeout 30 cfg`. A leading `.a.b`
   was rejected as the surface because `.` is a whitespace-insensitive
   standalone token (`. a . b` ≡ `.a.b`), leaving no syntactic way to mark "no
   receiver"; `$` is unambiguous and reserved (all-`$` names are un-definable).
6. **`reach` segment encoding (RESOLVED — Phase G).** The (un-evaluated) key
   list encodes ops: bare key = get, a `!` marker element = the next key is
   getr, `(expr)` = a deferred computed key.
7. **Scope of `getpath`/`setpath` unification (RESOLVED — Phase I).** The
   fuller option: native `Reach` handling. `getpath` reads via `ApplyReach`;
   `setpath` does a native immutable nested set (correct nesting, unlike the
   dotted-string form).

**Recommended first slice:** Phases **A + B** (type + parser flip + Stage-1
lowering) — lands the structural node at exact semantic parity, gated like the
paren flip; then C (round-trip/quotability) is the code-as-data payoff and the
rest is additive.
