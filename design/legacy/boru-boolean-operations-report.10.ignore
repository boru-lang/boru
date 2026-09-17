# Boolean Operations in boru

## Scope

This report surveys how boru implements its boolean operators
(`and`, `or`, `not`, `xor`, `nand`, `implies`), where each operator is
overloaded, how argument collection interacts with concatenative
evaluation, and what short-circuiting behaviour (if any) the language
provides. Recommendations follow.

Sources reviewed:

- `lang/go/internal/engine/native_boolean_and.go`
- `lang/go/internal/engine/native_boolean_or.go`
- `lang/go/internal/engine/native_boolean_not.go`
- `lang/go/internal/engine/native_boolean_xor.go`
- `lang/go/internal/engine/native_boolean_nand.go`
- `lang/go/internal/engine/native_boolean_implies.go`
- `lang/go/internal/engine/native_helpers.go` (`registerBinaryBoolWord`)
- `lang/go/internal/engine/conditional.go` (`isTruthy`, `if`)
- `lang/go/internal/engine/signature.go` (`BarrierPos`, scoring)
- `lang/go/internal/engine/match.go` (barrier/forward-collection logic)
- `lang/doc/design/LANGREF.10.md` §"Boolean Words" and §"if"
- `lang/go/test/basic.tsv`, `lang/go/test/sigmatch.tsv`, `lang/go/test/curry_test.go`

---

## 1. The Boolean Word Set

| Word      | Arity | Signature(s)                                       | Forward? | BarrierPos |
|-----------|-------|----------------------------------------------------|----------|------------|
| `not`     | 1     | `[boolean] -> [boolean]`                           | yes      | 0 (none)   |
| `and`     | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `xor`     | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `nand`    | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `implies` | 2     | `[boolean, boolean] -> [boolean]`                  | yes      | 0          |
| `or`      | 2     | `[boolean, boolean] -> [boolean]` **and** `[any, any] -> [disjunct]` | yes | 1 on both  |

All six are registered with `BarrierPos: BarrierAllForward` (i.e.
forward-eligible after sentinel resolution), so the standard
concatenative mirror applies — they can appear in any of the prefix /
mixed / forward positions described in `lang/CLAUDE.md` §"Argument
Ordering". For example, `and` accepts:

```
true and false       # forward: sig[0]=true, sig[1]=false
true false and       # all-prefix: sig[0]=true, sig[1]=false
```

`and`, `xor` and `nand` share a single helper,
`registerBinaryBoolWord` (`native_helpers.go:29`), which bakes in the
`[TBoolean, TBoolean]` signature and wraps a pure Go lambda
(`a && b`, `a != b`, `!(a && b)`). `implies` is written inline but
structurally identical, returning `!left || right`. `not` is the sole
unary case (`[TBoolean] -> [TBoolean]`).

---

## 2. Overloading

### 2.1 `or` — logical OR **and** type disjunction

`or` is the only overloaded boolean operator. It carries two
signatures (`native_boolean_or.go:34-59`):

1. `[TBoolean, TBoolean] -> [TBoolean]` — ordinary logical OR.
2. `[TAny, TAny] -> [disjunct]` — builds a `DisjunctInfo`
   (union type), flattening nested disjuncts so that
   `A or B or C` collapses to a single three-alternative union
   rather than a nested tree. Widening (`CarrierDisjunctCap`) and
   subtype subsumption are applied via `JoinCarriers`.

Dispatch uses signature specificity. Because `TBoolean` is more
specific than `TAny` in the Scalar hierarchy, two concrete booleans
select the boolean handler; anything else (type literals, numbers,
records…) falls through to the disjunct handler. The test file
`sigmatch.tsv:147-153` documents this behaviour explicitly:

```
true or false          => true              # boolean signature wins
String or None         => Scalar/String|None # disjunct signature
```

Both signatures are tagged `BarrierPos: 1`. A comment at
`native_boolean_or.go:5` notes that matching `BarrierPos` on the
boolean signature is required so the scoring bonus
(`signature.go:338-341`, `500_000 + (MaxArgs-BarrierPos)*10_000`) does
not accidentally prefer the less-specific disjunct rule. The barrier
also prevents `a or b or c` from being collected as a single
three-arg greedy grab by the outer `or`.

### 2.2 All other boolean words — boolean only

`and`, `not`, `xor`, `nand`, `implies` are **not overloaded**. They
declare only a `TBoolean` signature, so passing any other type
produces a signature-match failure rather than, say, a truthy-coerce.
`1 and true` is an error, whereas `if 1 …` succeeds (see §4 below).

This is asymmetric: `or` is the one operator willing to work on
non-booleans, and what it does there (build a type union) has no
runtime analogue on `and` (which would ideally build an intersection).

---

## 3. Short-Circuiting

### 3.1 There is none for `and`/`or`/`implies`

boru's boolean operators **never short-circuit**. The concatenative
dispatch pipeline collects both forward and stack arguments
(`match.go`, `engine.go` execution loop) before invoking a handler.
By the time `and`, `or`, `nand`, `xor`, or `implies` receive their
`args` slice, both operands have already been fully evaluated.

Consequences:

- `false and (1 div 0)` evaluates `1 div 0` first and errors out.
- `true or expensiveComputation` still runs `expensiveComputation`.
- There is no difference in cost between `false and cheap` and
  `false and expensive`.

The BORU-CODE-REVIEW-REPORT and LANGREF make no mention of
short-circuit semantics; a repository-wide search for `short-circ` turns
up only unrelated engine-internal optimisations.

### 3.2 Where lazy evaluation *does* exist: `if`

`if` (`conditional.go`) is the only truly lazy control word. It
declares `NoEvalArgs: {0: true, 1: true, 2: true}`, so list-form
branches are not auto-evaluated before dispatch; the handler then
returns only the matching branch for the engine to splice in. The
LANGREF calls this out explicitly (`LANGREF.md:2318-2323`):

```
if true 1 [10 div 0]    => 1    # no division error
if false [10 div 0] 2   => 2    # no division error
```

So in today's boru, if you want short-circuiting you write
`if cond [then] [else]`, not `cond and then` or `cond or else`.

### 3.3 Apparent precedence is really argument collection

`LANGREF.md:1075` claims `true or false and false => true` because
"`and` binds first". There is no real operator-precedence table; the
effect comes from concatenative dispatch order combined with
`BarrierPos=1` on `or`. `and` (barrier 0) greedily consumes the two
booleans immediately to its right, producing `false`, which the outer
`or` then combines. The "precedence" note is descriptive, not a
language feature the parser knows about.

---

## 4. Truthiness vs. Strict Booleans

`isTruthy` (`conditional.go:10-41`) defines a richer notion of
truth used by `if`:

- `true` / `false` — direct
- integer: non-zero
- `None` / empty list / empty map / `""` — falsy
- non-empty list / map / string — truthy
- `"true"` / `"false"` strings — mapped
- anything else — truthy if `valToString` is non-empty

**Crucially**, the boolean operators *do not* consult `isTruthy`.
They require `TBoolean`. So the two notions of "truth" diverge:

```
if 1 "yes" "no"    => 'yes'      # isTruthy(1) is true
1 and true         => ERROR      # signature mismatch
```

Users must explicitly `convert x boolean` before feeding non-boolean
data to `and`/`or`/etc. There is no implicit coercion, nor a value-
returning `or` (Python-style `x or default`).

---

## 5. Interaction With Other Features

- **Currying works**. `def and_true [and true] end false and_true`
  → `false` (`basic.tsv:557-558`, `curry_test.go:183-195`). The
  bracketed body defers execution, and the resulting one-arg word
  threads the curried `true` into the second slot via the mirror rule.
- **Static type-check mode** dispatches the same signatures over
  carrier values (`engine.go:261`), so the disjunct branch of `or`
  produces disjunct carriers at check-time, enabling union-type
  narrowing in `if` branches. `and`/`xor`/`nand`/`implies` produce a
  `TBoolean` carrier.
- **Record/table values** never match the boolean signature of `or`,
  so they always produce a disjunct. This is deliberate but means
  there is no way to write a single word that means "logical OR of
  two booleans *or* union of two types" with consistent error
  behaviour — a user mistake (e.g. forgetting to `convert`) silently
  produces a type value instead of a boolean.
- **Disjunct flattening order** (`native_boolean_or.go:18-30`)
  preserves source order: left alternatives first, then right. That
  matters for `unify`, which tries alternatives in order.

---

## 6. Gaps and Surprises

1. **No short-circuit**. Most notable gap. Every other mainstream
   language (including Scheme, Forth with `IF`, Factor, Joy) offers
   short-circuit AND/OR.
2. **Strictly typed operands**. `and`/`or`/etc. do not accept truthy
   non-booleans, even though `if` does. This is inconsistent and
   surprises users porting from dynamic languages.
3. **`or` does two unrelated things**. Its type-disjunct meaning
   relies on the dispatcher picking the right signature. When the
   user *intended* booleans but passed (say) a map and a `None`,
   they silently get a disjunct instead of a clear error.
4. **Missing symmetric operators**. `nand` exists, but `nor` and
   `xnor` / `iff` do not — despite `nand` and `nor` being equally
   natural. There is also no intersection counterpart to the
   disjunct form of `or` (no `and`-as-type-intersection).
5. **No value-returning `or`**. `x or default` returning the first
   truthy value is a widely used idiom and is absent.
6. **No Boolean short-circuit even under static check**. Because the
   handler sees both args, the check-mode narrowing from `and` does
   not get the positive/negative flow-typing that `if` provides —
   missing an opportunity for tighter types in patterns like
   `x isType Integer and x gt 0`.
7. **Documentation ambiguity**. `LANGREF.md:1075` says "`and` binds
   first"; newcomers may read this as Pratt-style precedence. It is
   actually an artefact of `BarrierPos` on `or`.

---

## 7. Recommendations

### 7.1 Introduce short-circuit forms

Two viable shapes, not mutually exclusive:

- **Lazy list operand**: overload `and`/`or` with signatures
  `[TBoolean, TList] -> [TBoolean]` and mark the second arg with
  `NoEvalArgs`. When the LHS suffices to decide the result, return
  it; otherwise `do` the list body. Mirrors `if` and is idiomatic
  for a concatenative language. Examples:

  ```
  false and [expensiveCheck]   => false         # body not run
  true  or  [loadFallback]     => true          # body not run
  ```

- **New words**: `andalso`, `orelse` (Erlang style) or `&&`, `||`.
  Implement them as forward-collecting words with two signatures,
  one of which wraps the RHS in a sub-engine `do` and runs it only
  when needed. This keeps the strict `and`/`or` intact for users
  who want eager evaluation (useful for effectful code).

In either case, extend `implies` the same way — `a implies b`
should not evaluate `b` if `a` is false.

### 7.2 Offer a truthy-coercing family

Add `andt` / `ort` / `nott` (or reuse `&&`/`||` from 7.1) that run
`isTruthy` on their arguments and return booleans. Alternatively,
add implicit coercion only for lists and maps where there is little
ambiguity. The divergence between `if`'s truthiness and boolean
operators' strictness is a real wart.

### 7.3 Value-returning forms

Port the Python/Lua idiom:

- `coalesce a b` — returns `a` if `isTruthy(a)`, else `b`.
- `either a b` — returns first non-`None` of `a`, `b`.

These round out the "pick a default" patterns without overloading
`or` any further.

### 7.4 Fill in the symmetric operators

Add `nor` and `xnor` (aka `iff`) alongside `nand`, each three-line
wrappers around `registerBinaryBoolWord`. The cost is trivial and
the symmetry is worth it for logic-heavy code
(decision tables, type predicates).

### 7.5 Separate `or` from disjunction

The dual meaning of `or` is elegant in print but muddies dispatch
and error messages. Options, ordered from least to most invasive:

1. Keep `or`, but add a dedicated `union` word for type disjunction
   and reserve `or` for booleans once code has migrated.
2. Emit a check-mode warning when `or` dispatches to the disjunct
   handler on values that were inferred to be `TBoolean` at some
   earlier point — catches "I forgot `convert boolean`" bugs.
3. Leave `or` as-is but return a richer error from the boolean
   handler when a signature mismatch is fed to disjunct — something
   like `or: expected [boolean, boolean] or two type values, got …`.

### 7.6 Flow typing through `and`

Today `if (x isType Integer) [x add 1]` narrows `x` in the
then-branch. `x isType Integer and x gt 0` does not, because
`and`'s handler is opaque to the flow-typing pass. A short-circuit
`and` (7.1) could feed the narrowing restore into the RHS body,
mirroring the existing `applyGuardNarrowing` machinery in
`conditional.go`.

### 7.7 Documentation

- Replace "`and` binds first" with a concrete argument-collection
  explanation plus a note that there is **no** operator precedence.
- Explicitly document that boolean operators are strict (eager)
  and require `boolean` operands, and point users to `if` for
  lazy evaluation.
- Document `or`'s dual dispatch with a worked example of when each
  signature wins.

---

## 8. Disambiguating `or`: Options and Trade-offs

The dual dispatch of `or` (logical OR and type disjunction through a
single word) is ergonomic but invites silent misuse: a missing
`convert boolean` causes the disjunct handler to swallow what the
user intended as a boolean expression. Four approaches, ordered from
least to most invasive:

1. **Split the words.** Keep `or` strictly boolean; introduce `union`
   (or a `|` operator) for type disjunction. Cleanest end-state;
   breaks the pervasive `String or None` syntax used in tests and
   docs and requires a migration.
2. **Tighten `or`'s second signature.** Replace `[TAny, TAny]` with
   `[TType, TType]` plus carrier / disjunct variants, so mixed
   operands (e.g. `true or String`) fail signature matching instead
   of silently flattening. Low-risk — existing disjunct call sites
   already pass type values.
3. **Context-driven dispatch.** Treat a type literal, existing
   disjunct, or carrier as a hard signal for the disjunct handler;
   two `TBoolean` values as a hard signal for logical OR; *error*
   on mixed. Preserves today's surface syntax.
4. **Check-mode warning.** When static carriers infer a boolean
   earlier in the program but `or` still dispatches to disjunct,
   emit a diagnostic. Catches the "forgot `convert boolean`" class
   without runtime behaviour change.

**Recommendation:** combine options 2 and 4. They preserve
`String or None`, eliminate the silent fall-through, and leave a
clean migration path to option 1 later if the language owners want
to retire the overload entirely.

---

## 9. Alternative Design: Strict Boolean Ops + Explicit Type Builders

A more radical reshape — worth considering because it resolves the
ambiguity for good and gives the type system a first-class builder
vocabulary:

- **Infixable boolean operators** (`and`, `or`, `not`, `xor`, `nand`,
  `implies`) become **strictly boolean-returning**. They coerce
  non-booleans via `isTruthy` (the same rule `if` already uses), so
  the "two notions of truth" divergence (§4) disappears.
- **Type construction moves to named builder words**, taken as
  arguments a list of types rather than overloading infix operators.

User-proposed seed set:

```
any [A B C]                          => A|B|C           (disjunction)
all [A B C]                          => A&B&C           (conjunction)
without [from:A remove:B]            => A without B     (difference)
```

This cleanly splits the two worlds: `or` is always a boolean OR,
`any` is always a union.

### 9.1 Additional builders needed to complete the algebra

**Close the set algebra**

- `never` / `bottom` — the empty type, the natural result of
  `all [A B]` on disjoint operands and of `without [from:T remove:T]`.
  Required for exhaustiveness checks and as the carrier-narrowing
  sentinel after `without`.
- `complement Type` — "any value that isn't T". Equivalent to
  `without [from:Any remove:T]` but worth a name (mirrors boolean
  `not`, and appears often enough — e.g. `complement None` for
  not-null — to justify the shortcut).
- `literal Value` — pin a concrete value as a type. boru already
  treats `42 is 42` as true; this builder just lifts that
  capability into the type algebra. Foundation for enums and
  discriminated unions.

**Parametric containers (word forms of the existing `[:T]`/`{:T}`
sugar, for naming consistency with `any`/`all`/`without`)**

- `listof Type` — homogeneous typed list.
- `mapof Type` (or `mapof [Key Value]` once keyed maps land) —
  typed map.
- `setof Type` — if sets become first-class; mainly a distinction
  from `listof` with respect to ordering and duplicates.
- `tuple [T0 T1 ...]` — fixed-arity heterogeneous list. boru already
  accepts `[1 "a" true] is [Integer String Boolean]`; this gives
  the construct a canonical name.
- `optionof Type` — sugar for `any [T None]`; could coexist with
  the `?` suffix.

**Record / structural manipulation**

- `pick Record [keys]` — keep only the listed fields.
- `omit Record [keys]` — drop the listed fields.
- `partial Record` — every field becomes `optionof`.
- `required Record` — strip optionality from every field.
- `extend Record Record` — structural merge of two record types.
- `tableof Record` — align naming with existing `table R`.

**Nominal and parametric**

- `newtype Name Type` — branded wrapper that does *not* unify with
  its underlying type (distinguishes `UserId` from `Integer`).
  Complements the current structural `type` alias.
- `generic [params:[X Y]] Body` — introduce type variables for
  reusable templates (e.g. `Pair<A,B>`).
- `apply Template [Types]` — instantiate a generic to concrete
  types.
- `rec Name [Body]` — recursive / self-referential definitions
  for trees and linked lists (`rec Tree [{val:Any children:(listof
  Tree)}]`).

**Refinement**

- `refine Type [Predicate]` — subtype by value predicate
  (`refine Integer [gte 0]`). With this in place, `enum` becomes a
  thin sugar over `any [(literal a) (literal b) ...]`, and numeric
  ranges become `refine Integer [between 0 100]`.

**First-class function types**

- `signature [args:[Type] returns:[Type]]` — the arrow type as a
  value. Currently `fn` bodies encode this implicitly at definition
  time; a standalone builder lets function types flow through the
  system the way record types do (for higher-order APIs, typed
  callbacks, etc.).

### 9.2 Coverage check

With the seed three plus the additions above, the core algebra is
complete:

| Concept                  | Operator / Builder                    |
|--------------------------|---------------------------------------|
| Union (∪)                | `any [...]`                           |
| Intersection (∩)         | `all [...]`                           |
| Difference (∖)           | `without [from: remove:]`             |
| Complement (¬)           | `complement T`                        |
| Top (⊤)                  | `Any` (exists)                        |
| Bottom (⊥)               | `never`                               |
| Unit                     | `None` (exists)                       |
| Literal / singleton      | `literal v`                           |
| Homogeneous container    | `listof` / `mapof` / `setof`          |
| Heterogeneous tuple      | `tuple [...]`                         |
| Option / nullable        | `optionof T` (or `T?`)                |
| Record                   | `record [...]` (exists)               |
| Record reshape           | `pick` / `omit` / `partial` / `required` / `extend` |
| Table                    | `tableof R` (exists as `table R`)     |
| Nominal                  | `newtype Name T`                      |
| Structural alias         | `type Name T` (exists)                |
| Parametric               | `generic [...]`, `apply`              |
| Recursive                | `rec Name [...]`                      |
| Refinement               | `refine T [p]`                        |
| Arrow                    | `signature [[args] [returns]]`        |

### 9.3 Knock-on effects

- **Boolean operators become total.** Since every value has an
  `isTruthy` interpretation, `and`/`or`/`not`/… stop erroring on
  non-booleans. Users coming from Python, Lua, JavaScript find this
  natural; users who relied on today's strict typing lose the
  compile-time guard and must migrate to explicit `convert boolean`
  or a linter rule.
- **Type expressions become uniformly prefix.** Today's
  `String or None` reads left-to-right; `any [String None]` is
  longer but parses without specificity rules or `BarrierPos`
  tricks — and the entire builder family shares a single grammar.
- **Static check mode simplifies.** The carrier dispatch for `or`
  no longer has to hedge between two signatures; `any`/`all`/
  `without`/etc. carry only type values, so their handlers are
  pure type algebra.
- **Short-circuiting lands naturally.** With booleans always
  returning booleans and coercing truthy values, §7.1's list-form
  lazy operands (`false and [expensive]`) fit without overloading
  worries, because the builder family has been lifted out of the
  infix space.
- **Migration cost is real.** Every `(T or None)` in the corpus —
  `basic.tsv`, `LANGREF.md`, field types throughout — must become
  `any [T None]` or `optionof T`. A codemod on the parser's
  disjunct construction path can automate most of it.

### 9.4 Recommendation

This alternative is strictly more expressive and avoids the
overloading problem by construction. If the project is willing to
absorb a migration, the target design is:

1. Make infix boolean operators strict about *returning* booleans
   but lenient about *accepting* any value (via `isTruthy`
   coercion).
2. Introduce the builder family above, starting with
   `any` / `all` / `without` / `never` / `literal` / `optionof` /
   `listof` / `mapof` / `tuple`, which cover ~90% of current
   usage.
3. Retire `or`'s disjunct signature behind a deprecation warning;
   remove it once the corpus has migrated.
4. Layer `pick` / `omit` / `partial` / `newtype` / `rec` /
   `generic` / `refine` / `signature` in a second wave, as demand
   surfaces — none of them unblock the core disambiguation.

The first wave alone delivers the win the report is after: a clean
boolean algebra, a clean type algebra, and no overloading between
them.

---

## 10. Summary

boru's boolean layer is small, regular, and conceptually clean:
forward-collecting words with strict `[boolean, boolean]`
signatures, plus `not`. The one overload is `or`, which doubles as a
type-union constructor via a `[any, any]` signature guarded by
`BarrierPos`. The notable omission is short-circuit evaluation:
`and`, `or`, and `implies` all eagerly evaluate both operands
because the concatenative dispatcher resolves arguments before
invoking handlers. `if` fills this gap via `NoEvalArgs`, so the
machinery already exists — extending it to `and`/`or` is the single
most impactful improvement. Secondary improvements round out the
operator set (`nor`, `xnor`, `coalesce`), reconcile the strict /
truthy divergence with `if`, and make `or`'s dual role less
ambiguous.
