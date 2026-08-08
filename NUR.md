# Non-Uniformity Register (NUR)

A running register of every place where boru — the language or its
implementation — deviates from one of its own uniform rules. A
non-uniformity is any special case: a type treated differently from its
siblings, one member of a word family with an exception, a path that
bypasses a single-source-of-truth mechanism. Uniformity is a core design
value of boru (one parser, one argument-positioning convention, one
binding store, one total order, one truthiness rule); this register is
where every deviation from that value is made visible, argued, and
either eliminated or explicitly accepted.

Records are short, numbered (`NUR000`, `NUR001`, …), and dated, in the
style of [ADR.md](ADR.md). Numbers are **never reused**. A **Resolved**
record is **deleted** from this file — the fix and its rationale live
in the resolving commit, which names the `NURnnn` it closes — and its
number is retired, never reassigned. A gap in the sequence is itself
the record that something was found and fixed, and any external
reference to a deleted `NURnnn` stays unambiguous forever.

> **A newly-encountered non-uniformity is a PR blocker.** When a
> non-uniformity surfaces — in code review, in a design note, or during
> coding and debugging — it is recorded here immediately with status
> **Pending**, and the PR that surfaced it must not merge until the
> entry is either **Resolved** (the divergence is removed) or marked
> **Allowed** (an explicit, argued acceptance). Unlike ADRs, *recording*
> is mandatory on discovery, not on maintainer instruction; what
> requires the maintainer is the **Allowed** verdict — the same reviewed
> discipline as a `//covergate:allow` entry
> (`design/COVERAGE-ALLOWLIST.10.md`).

**Statuses:**

- **Pending** — not yet discharged. Either not yet argued to a verdict
  at all, or argued to a **verdict of "resolve by fix"** whose fix has
  not landed: a record directed at a fix stays Pending until the
  divergence is actually gone, because only Resolved and Allowed
  discharge the block. Blocks the PR that surfaced it. Every Pending
  record must also appear in the pending list below.
- **Allowed** — a deliberate divergence, kept. The record states the
  uniform rule, the divergence, the rationale, and the evidence that
  pins it (docs and tests), so the acceptance cannot silently rot.
- **Resolved** — the divergence was removed. The record is **deleted**
  and its number retired (see above), so a record only ever appears in
  this file as Pending or Allowed; `git log -S NURnnn` recovers a
  retired number's history.

---

## Pending non-uniformities (the blocking list)

The live list of records whose status is **Pending**. A PR that
surfaced (or contains) one of these must not merge while it is listed
here. An entry leaves this list only by becoming **Resolved** or
**Allowed** in its record below — keep the two in sync in the same
commit.

| # | Title | Surfaced by / provenance |
|---|-------|--------------------------|
| [NUR009](#nur009) | Bytes excluded from the DepScalar refinement bases | 2026-07-22 uniformity review |
| [NUR022](#nur022) | `del` covers a fraction of `set`'s containers | 2026-07-22 uniformity review |
| [NUR023](#nur023) | Stack-only registrations outside ADR-004's closed list | 2026-07-22 uniformity review |
| [NUR026](#nur026) | Escape sets diverge between quoted strings and templates | 2026-07-22 uniformity review |
| [NUR030](#nur030) | `group` co-groups deq-distinct keys that render identically | PR #309 review (Codex P1); re-opened 2026-07-31 (was Allowed 2026-07-24) |
| [NUR031](#nur031) | Function/Word values are not `deq` to themselves; `eq` and order key on the binding name | PR #309 review (Codex P2); re-opened in part 2026-07-31 (was Allowed 2026-07-24); module namespace resolved 2026-08-01 by the NUR038 facet refactor, descriptor 2026-08-02 — modulo the fn-export residue this record still tracks |
| [NUR052](#nur052) | Store enumeration reads the top COW layer; lookup walks the chain | 2026-08-02 NUR-EFFORT-TRIAGE probing |
| [NUR053](#nur053) | The truthiness consumers do not share one domain | 2026-08-02 NUR register review |
| [NUR054](#nur054) | Context write boundaries differ between the interpreter and the compiler | 2026-08-02 NUR register review |
| [NUR056](#nur056) | `make`-constructibility is the one capability with no opt-in | 2026-08-02 NUR register review |
| [NUR057](#nur057) | The compiler exempts `set`/`del` by name on an unenforced no-shadow claim | 2026-08-03 lang/eng content audit (`design/LANG-ENG-CONTENT-AUDIT.0.md`) |
| [NUR058](#nur058) | Language-layer guaranteed-error mirrors are emitted unstamped | 2026-08-03 lang/eng content audit (`design/LANG-ENG-CONTENT-AUDIT.0.md`) |
| [NUR059](#nur059) | Several value kinds render in DEBUG spelling inside canon | 2026-08-08 Go/TS canon parity work |

Pending records normally use a compact form (rule / divergence /
evidence / documentation status, plus a proposed verdict where one is
obvious). A record argued to a **resolve-by-fix** verdict keeps the
compact form and appends the verdict, since the fix — not prose — is
what will close it; a record **re-opened from Allowed** keeps the
argued form it already had, with the superseded allowance retained as
data. Expansion to the full argued form is what an **Allowed** verdict
requires.

---

## NUR000 — Boolean arithmetic is a defined error {#nur000}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The six arithmetic words (`add`/`sub`/`mul`/`div`/`mod`/`pow`) are
**total within every scalar type and every Micron kind**
(REFERENCE.md §"Within-type operations"): numbers compute, `String` and
`Atom` carry the occurrence package, `Bytes` mirrors it over byte
subsequences, Microns fall to the field-wise default.

### The divergence

`Boolean` is the single scalar family excluded: `add true false` raises
`[boru/type_error]: add: arithmetic is not defined on Boolean` — for all
six ops.

### Why allowed

Boolean deliberately carries the logical words (`and`/`or`/`xor`/`not`)
instead of arithmetic; every candidate arithmetic semantics (C-style
integer promotion, GF(2)) is arbitrary, and an arbitrary choice would
be silently accepted where a loud error teaches the logical vocabulary.
The exception is implemented as a **registered** `[Boolean Boolean]`
signature that raises with a pinned message (the `setMicron` precedent)
rather than by signature absence, so the failure is specific instead of
an opaque dispatch error; a check-mode mirror (`booleanArithReturns`)
flags concrete Boolean arithmetic statically; and the signatures are
CoreDefault, so a user's `refine Boolean` overload can still extend an
arithmetic word by specificity (the refinement escape).

### Evidence

- `lang/go/native/native_scalar_ops.go` — `booleanArithHandler` /
  `booleanArithError` / `booleanArithReturns`; the six erroring
  `[Boolean Boolean]` signatures.
- REFERENCE.md §"Within-type operations" — "**`Boolean`** arithmetic is
  a **defined error**".
- `lang/spec/scalar-micron-ops.tsv` (all six ops pinned as errors);
  `lang/spec/open-words.tsv` (the refine-extension escape and its
  negative twins).

---

## NUR001 — `convert Boolean` coerces by presence, not content {#nur001}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

`convert <ScalarType> <String>` parses the string's **content**:
`convert Integer "42"` → `42`, `convert Float "1.5"` → `1.5`.

### The divergence

`convert Boolean "false"` → `true`. Boolean conversion applies the
truthiness rule — `false`, numeric zero in any leaf
(`0`/`0.0`/`0d0`) and `""` are false; a String's characters are never
inspected.

One neighbouring defect is recorded separately and does not disturb
this allowance: the language's falsy set also contains `none`, `[]`
and `{}`, but `convert`'s source slot is Scalar-only and refuses all
three with a `signature_error` (NUR053).

### Why allowed

`convert Boolean` shares **one** coercion rule with `if`-condition
truthiness and `make Boolean` (presence, not content); making the
conversion path parse content would fork truthiness into two rules —
a worse non-uniformity than the one it fixes. The three consumers apply
the same rule but do not accept the same *domain*; that separate
divergence is NUR053 and does not disturb this allowance, which is
about content-vs-presence. Content parsing exists as
an explicit opt-in: `convert Boolean {truthy: true}` parses the YAML
tokens (`y`/`yes`/`true`/`on` and `n`/`no`/`false`/`off`,
case-insensitive) and
falls back to presence for anything else; the option is inert for
non-Boolean targets.

### Evidence

- `lang/go/native/native_type.go` — `coerceBooleanTruthy` and the
  `truthy` option plumbing on `convert`.
- REFERENCE.md — "**`convert Boolean` is presence coercion; `{truthy:
  true}` opts into YAML parsing**" and §`if` ("coerces its condition …
  the exact same rule as `convert Boolean` and `make Boolean`").
- `lang/go/native/native_type_convert_seam9_test.go` (both modes,
  positive and negative); `lang/go/native/integration_coverage_test.go`
  (`'false' convert Boolean` → `true` pinned explicitly).

**Review (2026-07-31):** re-affirmed by the maintainer
(`design/NUR-RESOLUTION-PLAN.0.md`). The single coercion rule this
record leans on is now specified once — with every consuming construct
enumerated — in `design/TRUTHINESS.0.md` (the One Truthiness Model);
an ADR stating the model as a language principle is a recorded
candidate there.

---

## NUR002 — Value enumeration exhausts finite domains; Boolean is the built-in instance {#nur002}

**Status:** Allowed · **Date:** 2026-07-22 · **Rewritten:** 2026-07-31
(maintainer — the pre-rewrite record framed this as a Boolean special
case; the rewrite states the general rule instead)

### The uniform rule (as rewritten)

**Exhaustive coverage of any finite domain does not require a default
branch.** For a scalar scrutinee, a default-less `case` proves
exhaustiveness through type clauses, `[is T]` predicates,
comparison-predicate / refinement interval unions, or — when the
scrutinee's domain is **finite** — by enumerating its values. An
infinite scalar can never be covered by literal enumeration
(`case n [1 … 2 …]` can not cover `Integer`).

### Where Boolean sits

Boolean is not a special case: it is the built-in two-value
pseudo-enum. `true` + `false` cover a `Boolean` scrutinee exactly as
enum members cover an enum: `case b [true 1 false 0]` is statically
exhaustive with no default, and `case b [true 1]` is a
`case_not_exhaustive` check error (`uncovered: false`).

### Why this is the rule, not a divergence

Coverage-by-enumeration follows from cardinality, not special
pleading: a domain is enumerable iff it is finite, so the checker's
coverage proof stays in the sound direction throughout. The mechanism
is the one value/type coverage channel enums use (`def Color (red/q
tor …)` covered member by member) — and enums are themselves
specialisations of disjunct types, so the general principle is
**finite disjunct exhaustiveness**, of which Boolean is the built-in
instance. Documentation should present it that way rather than
presenting Boolean as special.

### Follow-on design work (recorded 2026-07-31, not yet scheduled)

Finite **dependent scalar types** also define finite domains, and
should eventually enter the same coverage channel. Two items to
investigate (`design/NUR-RESOLUTION-PLAN.0.md`):

- **Ergonomics** — allow a finite dependent type to be declared by
  enumerating its values (a `{2,3,4}`-style literal domain) rather
  than forcing range predicates (`Integer >=2 and <=4`).
- **Implementation** — when a finite dependent set is statically
  known, avoid materialising large sets; prefer symbolic/range
  representations. Where free variables remain, a symbolic
  representation is necessary regardless.

### Evidence

- REFERENCE.md §"`case` — dispatch and exhaustiveness" — "**Boolean, by
  `true` and `false`** (or the `Boolean` literal)", including the
  negative example.
- `lang/spec/case.tsv` §6 ("true+false cover Boolean", alongside the
  union and enum coverage rows that show the shared mechanism).

---

## NUR003 — `and`/`or` select an operand; the rest of the boolean family returns strict Boolean {#nur003}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The boolean word family returns strict `Boolean`: `not`, `xor`, `any`,
`all`, and the `boru:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/
`implies`) all coerce their inputs by truthiness and yield `true` or
`false`.

### The divergence

`and` and `or` are value-selecting short-circuit connectives: they
return whichever **operand** decided the result, of whatever type —
`1 and 2` → `2`, `false 5 and` → `false`, `0 9 or` → `9`.

### Why allowed

Deliberate Lisp/Python semantics: the operand form composes directly
(`x or default`, with `otherwise` as the None-aware variant), and a
strict Boolean is one `not not` (or a comparison) away. The divergence
is loudly documented at the word table itself, and check mode types the
result precisely — `foldOrJoin` concrete-folds statically-decided
selections and otherwise narrows to the join of the operand types, so
the non-uniform return type never degrades static analysis to `Any`.

### Evidence

- `lang/go/native/native_boolean.go` — `andHandler`/`orHandler` (operand
  return) vs `notHandler`/`boolBinaryNative`/`anyHandler`/`allHandler`
  (strict Boolean); `foldOrJoin` for the check-mode typing.
- REFERENCE.md §Boolean — "**`and` / `or` return an operand, not a
  coerced boolean.**"

**Review (2026-07-31):** re-affirmed by the maintainer
(`design/NUR-RESOLUTION-PLAN.0.md`). The operand-return semantics —
short-circuit behaviour, evaluation order, which operand is returned,
and the interaction with static typing — are specified in
`design/TRUTHINESS.0.md` §"The connectives", which this record now
leans on.

---

## NUR004 — Boolean, Atom and Bytes have no lattice subtypes {#nur004}

**Status:** Allowed · **Date:** 2026-07-22

### The uniform rule

The scalar branch families carry structural leaves: `String` has
`EmptyString`/`ProperString`, `Number` has `Integer`/`Float`/
`BigInteger`/`BigDecimal`, `Micron` has its twelve kinds.

### The divergence

`Scalar/Boolean` and `Scalar/Atom` are leaf-less — direct children of
`Scalar` with no builtin subtypes (no `True`/`False` lattice nodes).
`Scalar/Bytes` is a third leaf-less child, registered from the language
layer (`native_bytes.go`) rather than declared in `builtinDecls`. The
same reasoning covers it with one caveat: of the two value-level
substitutes named below, `case` literal coverage does not reach Bytes
(the domain is infinite) and DepScalar refinement construction is not
available for it either (it is not a supported refinement base —
NUR009). The nominal-split route, `refine Bytes`, does work, and is
what stands in for subtypes here.

### Why allowed

Vacuous rather than divergent: no kernel mechanism requires a scalar
family to have leaves, and nothing dispatches on their presence. There
is no useful structural split of Boolean — `True`/`False` subtypes would
duplicate what value-level machinery already provides uniformly (`case`
literal coverage per NUR002, and DepScalar refinements: `(Boolean gte
true)` *is* the true-only subset, since Boolean is one of the supported
refinement bases — `canonicalBaseType` admits Integer, Float, Number,
String, Boolean and Atom). Users who want a
nominal split can mint it (`refine Boolean`), which participates in
dispatch by specificity like any refinement.

**Clarified (2026-07-31, maintainer):** the two layers this record
separates are the **lattice subtype hierarchy** (structural leaves the
kernel dispatches on — `EmptyString`/`ProperString`, the Number
leaves) and **value-level finite sets** (the inhabitants of a finite
domain — what `case` coverage and DepScalar refinements operate on).
`true`/`false` belong at the value layer: they are the two members of
a finite domain (NUR002, as rewritten), not structural variants of the
type, so minting `True`/`False` lattice leaves would put value
distinctions into the structural layer — the wrong home for them.

### Evidence

- `eng/go/typetable.go::builtinDecls` — the Scalar branch layout.
- `eng/go/depscalar.go::canonicalBaseType` — Boolean listed among the
  supported DepScalar bases (`Boolean gte true` constructs).
- `lang/spec/case.tsv:75` (true+false cover Boolean) and
  `lang/spec/edge-types-1.tsv:82-85` / `lang/spec/open-words.tsv:26-29`
  (`refine Boolean` mints a nominal split that dispatches) — the
  value-level machinery that stands in for subtypes.
- `lang/go/native/native_bytes.go:23` — the `Scalar/Bytes`
  registration, the third leaf-less child.

---

## NUR005 — String `add` is the sole cross-type exception to same-type arithmetic {#nur005}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict and rewritten wording: maintainer, via
`design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

Scalar arithmetic is **same-type arithmetic**: the six words are
"applied within a type, never across it" — REFERENCE.md §"Within-type
operations". A cross-type pair has no signature and raises
`[boru/signature_error]`. Where a signature is *deliberately registered
to refuse*, the failure is instead a coded error with a specific
message — `[boru/type_error]` for `Big`⊕`Float`, for Boolean
arithmetic (NUR000), for a cross-KIND Micron pair, and for several
within-kind Micron restrictions (`mul` on two Qions); `[boru/arith_error]`
for a Qion currency mismatch.

### The divergence

`add` carries `[String Scalar]` / `[Scalar String]` overloads that
stringify the non-String operand (`add "x" 5` → `'5x'`), while Atom
`add` is `[Atom Atom]`-only and Bytes `add` is `[Bytes Bytes]`-only.

### Why allowed

**String `add` is the sole language-level exception to same-type
arithmetic, and it is deliberate.** Concatenation-with-coercion is the
overwhelmingly common string operation, the coercion is total and
canonical (every Scalar has one string render), and the overloads
require **at least one** String operand, so they never manufacture a
concatenation of two NON-String operands: `add true 1` raises
`[boru/signature_error]` — no concat overload matches without a String,
and no within-type arm matches a Boolean/Integer pair either, so the
refusal is a dispatch miss rather than the registered `type_error`
NUR000 installs for Boolean arithmetic. ("String-or-bust" governs the
concat overloads only; two non-String scalars of the SAME type still
have their within-type arm — `add 1 2` → 3, `add a/q b/q` → 'ba'.)

The Pending record's framing — that Atom and Bytes "do not mirror it" —
treated the trio as an
architectural grouping obliged to move together; the verdict is that
the String/Atom/Bytes occurrence-package parallel is a **documentary
comparison, not an architectural grouping**. Nothing requires Atom or
Bytes to adopt a cross-type overload because their within-type
packages mirror String's, and neither has String's coercion case: an
Atom is a name and Bytes are raw octets, so a silent stringify would
manufacture bugs, not ergonomics.

### Evidence

- REFERENCE.md §"Within-type operations" — now states the exception
  **at the rule**: "The **sole language-level exception** is `String`
  `add` … no other word, and no other type — `Atom` and `Bytes`
  included — crosses scalar types" (doc fix landed with this verdict,
  closing the 60-lines-apart contradiction the Pending record flagged).
- `lang/go/native/native_math.go` — the `[TString TScalar]` /
  `[TScalar TString]` overloads and the "string-or-bust" comment;
  `native_scalar_ops.go` / `native_bytes.go` — the within-type
  `[Atom Atom]` / `[Bytes Bytes]` signatures.
- `lang/spec/arithmetic.tsv` §3 — the concat battery, including the
  `add true 1` and `add true false` negatives.

---

## NUR009 — Bytes excluded from the DepScalar refinement bases {#nur009}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the comparison words double as refinement constructors over
the well-known ordered scalar bases (`Integer gte 0`, `String lt "z"`,
`Boolean gte true` — one shared resolver, `canonicalBaseType`).
**Divergence:** `canonicalBaseType` admits Integer/Float/Number/String/
Boolean/Atom and omits Bytes — the one ordered scalar leaf (full
byte-lexicographic Comparer, Sizer, complete occurrence package) denied
refinement construction.
**Evidence:** `eng/go/depscalar.go:157-175`;
`lang/go/native/native_bytes.go:13,138-163`.
**Documentation status:** not found in REFERENCE/ADR/design — either an
unstated deliberate scoping or an omission; needs a verdict.

**Verdict direction (maintainer, 2026-07-31 — architectural
remediation, `design/NUR-RESOLUTION-PLAN.0.md`):** the Bytes omission
exposes a deeper **ownership** problem in the kernel type hierarchy,
and the fix is architectural rather than a one-line addition to
`canonicalBaseType`. The proposed rule: **all globally visible
descendants of `Node` or `Scalar` belong in `eng`**; core modules
(`boru:*`) may define module-owned descendants; the `lang` layer must
not define additional *global* Node/Scalar descendants except through
an explicit NUR. Likely migrations into eng: `Bytes`, `Time`, `Date`,
`DateTime`, `Instant`; the remaining scalar descendants are to be
reviewed individually. A new ADR describing ownership of the kernel
type hierarchy is required before the migration lands (recorded as ADR
candidate 2 in the resolution plan). This record stays Pending until
that remediation (or a narrower argued verdict) closes it.
**ADR-012 (2026-08-03) retargets the remediation's destination:** the
ownership rule is recorded, but the migrations consolidate in the new
`types/go` component with capability opt-ins (the refinement-base
capability closes this record), not in eng — the kernel stays
content-free.

---

## NUR011 — `eq` is identity for compounds, value for scalars {#nur011}

**Status:** Allowed · **Date:** 2026-07-23

### The uniform rule

One word, one equality principle.

### The divergence

`eq` compares scalars by value but lists/maps/XML/instances by
container identity (`["a"] eq ["a"]` → false); `deq` is deep value
equality throughout. Consequence: `eq` disagrees with `cmp`-equality
on compounds (structurally-equal lists are `cmp`-equal but not `eq`).

### Why allowed

The maintainer's rule (2026-07-23, resolving NUR015 in the same
stroke): **for Scalars, `eq` and `deq` are the same and based on
values; for Nodes and Ideals, `eq` is by reference, `deq` is by
value.**

The Ideal half carries argued carve-outs, recorded in NUR031 rather
than here. NUR031's 2026-07-24 verdict minted two: `Error` is a
*value-like* Ideal with no handle, so its `eq` is by VALUE (two
independently raised errors with equal code/message/payload are `eq`);
and `Timeout`/`Interval` are opaque handles whose identity IS their
value, so their `deq` is by REFERENCE. The `Module` descriptor joined
the second group on 2026-08-02 under the same rule (NUR031, "Descriptor
half RESOLVED" — a bolded lead-in inside §"The remainder,
reviewed"). All are
the rule applied to Ideals that have no second level to offer, not
departures from it — but the rule as quoted above does not say so, and
this record is where a reader looks first.

Two equality levels are deliberate — reference identity
answers "is this the same container?" (cheap, aliasing-aware), deep
equality answers "do these hold the same values?" — the Scheme
`eq?`/`equal?` trichotomy collapsed to two levels because scalar
value-identity makes the levels coincide there. Every value-oriented
word keys on `deq` (the collection words since the NUR015 fix); `eq`
remains the aliasing probe.

### Evidence

- `eng/go/compare.go` — `ExactEqual` (scalar arm shared with
  `DeepEqual` via `scalarFamilyEqual`, so eq and deq can never drift
  on a scalar; `sameContainer` identity arms for compounds).
- REFERENCE.md §Comparison ("**`eq` is identity for compounds; `deq`
  is structural — by design**"); EXPLANATION.md §"Type ordering", the "**Two equalities, one rule.**"
  lead-in (added with this verdict); `design/LISP-ANALYSIS.5.md` (the original
  argument).
- NUR031 §"What was resolved" — the `Error` and `Timeout`/`Interval`
  carve-outs and their implementing arms; NUR031 "Descriptor half
  RESOLVED" (2026-08-02) — the `Module` descriptor.
- `lang/spec/module-array.tsv` — the collection words' `deq`-basis
  battery pins the value side of the rule.

**Modification recorded (maintainer, 2026-07-31,
`design/NUR-RESOLUTION-PLAN.0.md`):** the two-level model is to grow
into a complete equality family with a third word — **`req`**,
reference equality (pointer identity only, uniformly for compounds
and scalars) — separating three notions many languages conflate:
convenience equality (`eq`), deep structural equality (`deq`), and
reference identity (`req`). Performance note: Bytes `deq` may be
O(n); `req` gives a constant-time identity probe. Documentation
should compare the model with JavaScript, Python, Ruby, and the
Lisp family. The `req` design travels with the equality work
re-opened under NUR031; this record's allowance is unchanged.

---

## NUR013 — Two ordering regimes: a lawful total order and IEEE relationals {#nur013}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; the 2026-07-31 investigation verdict discharged below;
verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

One ordering answer per value pair within one word family.

### The divergence

`cmp`/`tcmp`/`sort` give NaN a defined slot (sorts greatest; two NaNs
tie) while `lt`/`lte`/`gt`/`gte` apply the IEEE unordered rule (always
false); `nan eq nan` is false while `nan cmp nan` is 0. Signed zeros
now add a mirror-image case in the other direction: `-0.0 cmp 0.0` is
-1 while `-0.0 eq 0.0` is true and `-0.0 lt 0.0` is false.

### The `totalOrder` comparison (the 2026-07-31 verdict, discharged)

IEEE-754 §5.10 `totalOrder` requires
`−qNaN < −inf < negative finite < −0 < +0 < positive finite < +inf <
+qNaN`, with NaNs further ordered by sign and payload. boru's order
was compared against it point by point:

- **NaN slotting — conforming, for boru's observable NaN.** boru
  exposes exactly one quiet NaN: there is a single `nan` literal, sign
  is not observable (`nan -1.0 mul` renders `nan`), and no payload is
  reachable. For a single positive qNaN, `totalOrder` demands exactly
  what boru does — greatest, above `+inf`, tying with itself.
- **NaN sign/payload ordering — impractical, and accepted.** Ordering
  negative NaNs below `−inf` and ordering by payload would require
  making NaN sign and payload observable values in the language, which
  nothing else in boru does and no boru program can produce. The
  divergence is therefore vacuous at the language level; per the
  verdict's own terms it is folded into this record's acceptance.
- **Signed zeros — was nonconforming, now FIXED.** `-0.0 tcmp 0.0`
  answered 0; `totalOrder` requires −0 before +0. The total order now
  slots negative zero first (`sort [0.0 -0.0]` → `[-0.0 0.0]`), with
  Integer `0` and BigDecimal `0d0` slotting as +0 so the cross-leaf
  triangle stays transitive.

### Why allowed

The two regimes are deliberate and are the standard resolution of an
unsatisfiable constraint set — the same architecture NUR024 records as
**semantic** vs **deterministic** ordering:

- **The relationals** (`lt`/`lte`/`gt`/`gte`) answer a *mathematical*
  question and therefore obey IEEE-754: NaN comparisons are false
  (§5.11), and ±0 compare equal. A language that silently ordered NaN
  in `lt` would be wrong by the numeric standard its floats implement.
- **The total order** (`cmp`/`tcmp`/`sort`) answers "give me a lawful,
  deterministic arrangement of these values". It must be total and
  antisymmetric or `sort` is not a function; that requires a slot for
  NaN and a decision on ±0, which is precisely what `totalOrder`
  specifies and what boru now implements.

Because the relationals must keep IEEE ±0 equality while the total
order separates the zeros, the relational path carries an explicit
signed-zero carve-out beside the NaN one. That carve-out is part of
this acceptance, not a new divergence: it is the same
semantic-vs-deterministic split applied to the other special value.

### Evidence

- `eng/go/compare_scalar_behaviors.go` — the NaN slot and the
  Signbit tiebreak (float projection and big-rat paths, keeping
  Integer/BigDecimal zeros at +0).
- `eng/go/compare.go` — the relational unordered/signed-zero guards
  that keep `lt`/`lte`/`gt`/`gte` IEEE-conforming.
- `eng/go/compare_nan_test.go`, `eng/go/compare_zero_test.go` — both
  regimes, positive and negative.
- `lang/spec/float-special.tsv` (signed-zero and NaN sections),
  `lang/spec/edge-scalars-2.tsv` (the cmp/sort rows).
- `design/IEEE-754-COMPLIANCE.8.md` §5.10 — the conformance record
  above; `design/TYPE-ORDERING.10.md` §"NaN in the total order".

---

## NUR014 — Cross-leaf numeric magnitude equality depends on the leaf pair AND the value {#nur014}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

Leaves of the same family compare by magnitude: `1 cmp 1.0` → 0,
`1 eq 1.0` → true.

### The divergence

Whether the collapse holds is decided by the leaf pair **and** by the
value, so it is not a family invariant either way.

- **Value-dependent within one pair.** Float↔BigDecimal collapses for
  every binary-exact (dyadic) magnitude — `0d0.5 eq 0.5` → true — and
  fails for every other — `0.1 eq 0d0.1` → false (an exact big.Rat
  compare of the float's true binary value against the exact decimal).
- **Pair-dependent at one value.** The same magnitudes answer
  differently purely because of the leaves: `9007199254740993 eq
  9007199254740992.0` → true (Integer↔Float compares through a float64
  projection) while `0d9007199254740993 eq 9007199254740992.0` → false
  (BigInteger↔Float compares exactly).

### Why allowed

The divergence is **mathematically honest**: the Float written `0.1`
IS NOT one-tenth — it is the nearest binary64 value,
0.1000000000000000055511151231257827…, and the exact big.Rat compare
reports that truthfully. Every collapse that *can* hold exactly does
hold (`1 eq 1.0`, `1 eq 0d1`, `0d0.5 eq 0.5` — dyadic values convert
exactly), so the family invariant fails only where the mathematics
itself fails. The alternative — rounding BigDecimal through float64 to
force the collapse — would silently equate distinct values, defeating
the reason BigDecimal exists; it would also contradict the
exactness-preserving design that already makes mixed Big⊕Float
arithmetic a defined error. The behaviour is Python's
(`Decimal('0.1') == 0.1` → False), for the same reason.

### Evidence

- `eng/go/compare_scalar_behaviors.go` — `numberCompareBehavior.
  Compare` and `toRatExact` (the in-code rationale comments cite the
  Python precedent).
- REFERENCE.md:195-200 — the user-facing statement of the honest
  result, with the exact-value explanation.
- `lang/spec/bignum.tsv:47-63` — pins both directions: the collapses
  that hold (`0d5 eq 5`, `1 cmp 0d1.0` → 0, `0d0.5 eq 0.5`) and the
  one that must not (`0.1 eq 0d0.1` → false).
- `lang/spec/edge-scalars-1.tsv:24-25` — both `cmp` directions of the
  non-collapse.

---

## NUR018 — Store and Error are excluded from `make` {#nur018}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; verdict: maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

`make` instantiates the structural type-kinds; the kernel guide groups
Record, Options, Table, Class, Store, Error and the Micron family
together as the `make`/`record`/`class` structural set
(eng/go/CLAUDE.md §"Where a Type Lives" rule 4).

### The divergence

`make Store {}` and `make Error {message:"x"}` raise
`[boru/unsupported]: make: unsupported target type` while
Record/Options/Table/Class/Micron are `make` targets — Store and Error
construct only through their dedicated words.

### Why allowed

`make` targets are the **schema-bearing** structural kinds: a
Record/Options/Table/Class/Micron declares a shape, and `make`
instantiates a value against that shape. Store and Error carry no
user-declared schema and their constructors are semantically loaded in
ways a bare `make` cannot honour: a Store IS its position in the
context machinery (`StoreInstanceInfo` carries the parent-chain and
COW-layer state that `eng/go/registry.go`'s context words establish —
a detached `make Store {}` would have to invent an answer to "whose
child is it?"), and an Error's identity is its passage through
`raise`/`trap` (`describe raise`: "construct an Ideal/Error"), so
error construction always flows through the raising path that stamps
code and context. The kernel-guide grouping this record measured
against is about **kernel residence** (where the types live), not
about `make`-constructibility — clarified at the rule itself with this
verdict. The exclusion is loud (a coded `unsupported` error, not a
dispatch miss), and the dedicated constructors are the documented
route.

### Evidence

- `eng/go/core_make.go` — `registerKernelIdeals` (:788) is where the
  omission lives: it registers Ideals for Object, Resource, Record,
  Micron and Table and registers none for Store or Error, so
  `reg.Ideals.For`/`Match` return nil for those two and the target
  falls through to `MakeConvert` (:1088), whose default arm (:1128)
  raises the covered `unsupported target type`. (`isTypeLike` (:31) is
  NOT the gate — it short-circuits on `IsBareTypeNode` and answers true
  for Store and Error exactly as it does for Record.)
- `eng/spec/make.tsv` — negative rows pinning both exclusions
  (`make Store {}` and `make Error {message:'x'}` → ERROR).
- eng/go/CLAUDE.md §"Where a Type Lives" rule 4 — the
  kernel-residence clarification landed with this verdict.
- REFERENCE.md — the `make` documentation states the exclusion and
  names the dedicated constructors.

---

## NUR019 — `slice` is a core sequence word, not a String straggler {#nur019}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22 as "the String family's core straggler"; verdict:
maintainer, accepting the recommendation in
`design/NUR-EFFORT-TRIAGE.0.md`)

### The uniform rule

The string vocabulary moved to `boru:string-util`; moved words are not
available unqualified (lang/go/CLAUDE.md §"Package layout").

### The divergence (as recorded)

`slice` alone stayed core — REFERENCE's string table listed it
unqualified between two `StringUtil.*` rows, and `boru describe` files
it under `list`, not `string`, with the reason stated nowhere.

### Why allowed

The move rule does not apply because **`slice` is not a String-family
word**: it is a core *sequence* word, polymorphic over String, List,
and Bytes (nine unqualified signatures spanning all three), kin of
`size`/`take`/`reverse`, which also stayed core for the same reason.
Relocating it to `StringUtil` would force splitting one polymorphic
word — the List and Bytes overloads cannot live in a string namespace
— which is a semantically worse outcome than the filing confusion this
record flagged. What WAS wrong was the filing: REFERENCE's string
table presented `slice` as if it were an unqualified string word, and
the describe categories did not say where to find it. Both filings are
fixed with this verdict; the `list` category placement stands, because
that is the sequence home.

### Evidence

- `lang/go/native/natives.go:372-385` and
  `lang/go/native/native_bytes.go` — the String+List signature pairs
  plus the Bytes overloads: one polymorphic word.
- `boru describe slice` — all nine signatures, unqualified.
- REFERENCE.md:1160 — the string-table row now carries the "core
  *sequence* word, no import — also slices List and Bytes; filed under
  the `list` describe category, see NUR019" parenthetical (fixed with
  this verdict).
- `lang/go/native/help/help_categories.go` — the string category's
  description now points at core `slice` (fixed with this verdict).
- `lang/spec/edge-scalars-3.tsv:45-53`, `corpus-core.tsv:119`,
  `corpus-structures.tsv:14` — both string and list behaviour pinned;
  the two-argument negative-start form is pinned at
  `edge-scalars-3.tsv:47,52`. NUR039's actual divergence — a negative
  start in the THREE-argument form discarding `end` — is pinned by no
  spec row.

---

## NUR020 — `print` stays in core; every other IO word is namespaced {#nur020}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict: maintainer, via `design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

The IO vocabulary lives in `boru:io` (`IO.printstr`, `IO.read`,
`IO.write`, …); moved words are not available unqualified.

### The divergence

`print` alone stays in core, unqualified — one IO word outside the
namespace the rest of its family lives in.

### Why allowed

The argument, now written down rather than asserted: **`print` in core
is what makes the expected "Hello World" learning experience work.**
`print "Hello, World"` must be a complete first program — no `import`,
no namespace, no explanation of the module system before the first
line of output — and that matches the expectation practically every
mainstream language sets (`print`/`println`/`puts`/`console.log`
reachable from the first line). The pedagogical entry point outweighs
family symmetry for exactly one word; everything programmatic
(`printstr`, streams, `read`/`write`, `trace`) correctly demands the
`boru:io` import, so the capability surface of real programs is
unchanged. The boundary is one word wide and this record is its
argument; a second unqualified IO word would need its own NUR.

### Evidence

- `lang/go/native/native_print.go` and `register.go` — `print` is the
  single core IO registration; `io_module.go` — everything else.
- `lang/go/CLAUDE.md` §"Package layout" — "only `print` stays in core";
  ADR-004 §Consequences argues print's *forwardness* (a distinct
  question, deliberately not revisited here).
- Bare `print` works in a one-line program with no import
  (`boru -e 'print "Hello, World"'`) — the experience this record
  protects; HOWTO.md's recipes use it unqualified throughout.

---

## NUR022 — `del` covers a fraction of `set`'s containers {#nur022}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** the storage-column words cover the same containers — a key
that `set` can write, `del` can remove.
**Divergence (as recorded, now FIXED — see below):** `set` dispatched
over Class, Store, FlexXml, WeakFlexXml, FlexMap, WeakFlexMap, Map,
List, FlexList, WeakFlexList (and carried a registered `type_error`
refusal for the immutable Microns); `del` covered Map and FlexMap
only. The List exclusion was documented (pointing at
pop/shift/remove-at); the Store, Class, FlexList/WeakFlexList and
FlexXml/WeakFlexXml absences were not. `boru describe set` listed 19
signatures, `boru describe del` four.
**Documentation status:** documented — `lang/spec/flex.tsv` §12 now
states the per-container contract, and every refusal carries its own
message.

**Note on the rule (2026-08-02 review):** the rule above was
originally phrased "paired reader/writer words cover the same
containers", which mis-describes the pair: `set` and `del` are both
WRITERS. The reader, `get`,
covers a third and wider set again (Module, Class, Store, Error,
Resource, Xml, Node, Micron, None) — so container coverage is not
uniform across the storage column at all. That wider spread is
context for the verdict below, not a separate record: bringing `del`
into line with `set` is the step that was directed.

**Verdict (maintainer, 2026-07-31 — resolve by fix,
`design/NUR-RESOLUTION-PLAN.0.md`):** bring `del` into symmetry with
`set` across the container set. **First investigation step:** confirm
that boru distinguishes an *absent key* from a *present key bound to
`none`* — the deletion semantics hang on that distinction being real
and observable. Separately, a **sentinel-values design programme** is
opened (globally unique singletons, user- and system-defined
sentinels, their interaction with containers, equality, and
option-like APIs) — it needs its own design document because it
potentially touches many language facilities, but **NUR022 must not
wait on it**: the del/set symmetry fix proceeds independently. Stays
Pending until the fix lands.

### Investigation step (2026-08-02): the distinction is real, with one hole

An absent key and a key bound to `none` are distinguishable, so
deletion is not expressible as `set key none` and the word earns its
place:

| probe | `{a:1}` | `{a:1 b:none}` |
| --- | --- | --- |
| `has b/q` | `false` | `true` |
| `size` | `1` | `2` |
| `keys` | `["a"]` | `["a", "b"]` |

and the two containers are not `eq`: `{a:1} eq {a:1 b:none}` → `false`.

`del` and `set … none` therefore produce different containers:
`({a:1 b:2} del b) eq ({a:1 b:2} set b none)` → `false`.

The hole is **`get`**. Reading an absent key and reading a
present-none key both yield something whose `typeof` is `None` and
which answers `eq none` → true, `deq none` → true, `eq None` → true.
They *render* differently (`None` for the miss, `none` for the
binding — type literal vs value), but no comparison operator
separates them. So the distinction is observable through `has` /
`size` / `keys` / `eq`, and invisible through the reader. That is
the shape the **sentinel-values programme** the verdict opened has to
settle (a distinct miss sentinel would close it); it is recorded here
as context, not as a separate divergence.

### Fix (2026-08-02): the container sets are now identical

`del` dispatches over exactly the eleven containers `set` does, with
the same key shapes (String and Atom for keyed containers, Integer
for indexed) — 19 signatures each. Each container either removes the
slot or refuses with its own message:

| container | `del` |
| --- | --- |
| Map | copy-returning — a new map without the key |
| FlexMap, WeakFlexMap | in place, returns the node |
| FlexXml, WeakFlexXml | removes an **attribute** — the slot `set` writes |
| Store | copy-on-write, via a tombstone layer (`CowDel`) |
| Class | refused — a declared field is sealed |
| Micron | refused — immutable, mirroring `set`'s own refusal |
| List, FlexList, WeakFlexList | refused — names pop / shift / `ArrayUtil.remove-at` |

The refusals are **registered signatures**, not sig-absence, for the
reason `set`'s Micron form is: an absent signature raises an opaque
`signature_error`, a present one raises the specific message, and
negative spec rows can pin it.

The Store form needed new kernel machinery. `CowSet` layers a binding
over the old store because that store may be shared with an enclosing
scope; removal cannot work by subtraction, since there is nothing in
the new layer to leave out. So `CowDel` writes a **tombstone** and
`StoreInstanceInfo.Get` stops there — the key reads absent from the
deleting layer down while the layer that owns it is untouched. Own
`Data` beats a tombstone, so a `set` after a `del` re-binds; clones
carry tombstones, or a cloned prototype chain would resurrect every
deleted key.

Gate: `lang/go/native/native_del_symmetry_test.go` asserts the two
words carry the **same** container set and the same key shapes, and
fails in both directions — so a container added to `set` cannot
silently reopen the gap, and a `del`-only container is caught too.
Behaviour: `lang/spec/flex.tsv` §12; kernel:
`eng/go/store_tombstone_test.go`.

### What is left for a maintainer

One asymmetry survives on purpose and needs a verdict, because under
the rule as literally worded it is still a divergence: **`set` can
write a declared Class field and `del` cannot remove it.**

The argument for allowing it: a class field is a **slot**, not a key.
The inverse of writing a value to a slot is writing a different
value, not deleting the slot — an instance missing a declared field
would no longer satisfy its own type. The same reading is what makes
the List refusal correct (`set` replaces at an index; removal shifts
the tail, which is a different operation). If that slot-vs-key line
is the right one, the rule should be restated as "a **key** that
`set` can write, `del` can remove" with slots explicitly out of
scope, and this record becomes Allowed. Stays **Pending** on that
verdict.

---

## NUR023 — Stack-only registrations outside ADR-004's closed list {#nur023}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** ADR-004 — every word ships forward-eligible
(`BarrierPos: -1`); the only stack-only words are the traditional
Forth vocabulary, pinned as a closed list in REFERENCE; a new
stack-only word "needs the same justification weight as a new
init-time panic".
**Divergence:** two argument-taking words carry `BarrierPos: 0`
(stack-only) with a code-comment-only rationale and are not in the
pinned list — `apply`'s one-argument `[Function]` signature, and
`__casematch`'s two-argument `[Any Any]` signature (user-reachable and
describable: `boru describe
__casematch` prints "Precedence: stack"; the `__` prefix is convention,
not enforcement).

Secondarily, the guidance for **0-arg** words is itself split, and the
registrations follow it inconsistently. ADR-004 says every word ships
forward-eligible (`BarrierPos: -1`) "unless their semantics are
intrinsically about the stack", while `design/go-modules/README.10.md`
:156 and `RUNTIME.10.md`:50-54 both say "zero-arg constants use
`BarrierPos: 0`" and call that "correct" because there is no arg to
collect. Most 0-arg words follow the latter (`now`, `math-pi`/`math-e`,
the clock words, `break`/`continue`, `gensym`, `__folder`/`__file`,
`spacer`); `boru:io`'s `stdin`/`stdout`/`stderr` use `-1` instead. The
split is provably inert — `eng/go/registry.go:1594-1596` normalizes
`-1` to `TotalArgs()`, which is 0 at zero args, so the stored
signatures are byte-identical — but two documents give opposite
defaults for the same case, and that is what the refined ADR has to
settle.
**Evidence** (paths relative to the repo root):
`lang/go/native/native_ref.go:50-67` (`apply`, rationale comment at
:51-54, `BarrierPos: 0` at :67);
`lang/go/native/native_control.go:131-137` (`__casematch`);
REFERENCE.md §"Stack manipulation" (the pinned list);
`lang/go/native/time_async_module.go:26`;
`lang/go/modules/math.go:342` (`math-pi`; `math-e` at :353 is the same
shape); `lang/go/native/native_fileinfo.go:25,33`;
`lang/go/modules/tui_widgets.go:332`;
`lang/go/native/io_module.go:91`.
**Documentation status:** undocumented, no longer contradicted. Until
2026-08-02 it was worse than undocumented: `boru describe apply`
printed "Precedence: forward — looks ahead for arguments first. apply
x y <=> y apply x <=> y x apply", and the stated equivalence is false
for the `[Function]` row (`apply f/r 5` raises a signature_error,
`5 f/r apply` works). The cause was broader than this record's two
words — the renderer branched on a single binary `info.ForwardArgs`
flag, so EVERY mixed-barrier word printed the full forward
equivalence chain including a spelling it refuses. Measured blast
radius: **20 of the 249 describable core words** — `or`, `otherwise`,
`get`, `getr`, `dot`, `dotr`, `has`, `apply`, `guard`, `error`,
`exposes`, `of`, `extends`, `default`, `tor`, `tand`, `teq`, `is`,
`as`, `tis`. `dot` is the subject of NUR049 (16 of its 18 rows are
mixed-barrier); `or` is the plainest: the help advertised
`or x y <=> y or x <=> y x or`, and `or false true` raises
`insufficient_args` while `false true or` → true. Its sibling `and`
carries the `-1` sentinel and is genuinely all-forward, so the two
connectives really do differ — the help just could not say so.

That misreport is **fixed**. `precedenceShape` in
`lang/go/native/help/help.go` classifies a word by whether its
signatures agree about argument sourcing; `writePrecedenceMixed`
renders the disagreeing case as a per-group count plus the one
spelling that satisfies every row — full stack form, which always
dispatches because a forward-eligible position accepts a stack value
too (verified: `5 inc/r apply` → 6, `xs $.1 apply` → the indexed
element, `m a/q dot` → the field). Uniform words are untouched:
`add` still prints the forward chain, `dup` the stack line, and a
module export whose wrapper carries the unnormalized `-1` sentinel
(`ArrayUtil.indices`) still reads as forward.
Tests: `lang/go/native/help/precedence_test.go` pins all three shapes
and both directions — a genuinely uniform word must keep its
unqualified line, and a mixed one must never claim one.

What remains undocumented is the *rule* the diagnostic now reports
around: ADR-004's closed list contains neither exception, and nothing
states why a word occupies its category. That is the gap the verdict
below directs at, and it is why this record stays Pending.

**Verdict (maintainer, 2026-07-31 — resolve by ADR refinement,
`design/NUR-RESOLUTION-PLAN.0.md`):** ADR-004 is **incomplete**, and
the divergences recorded here are symptoms of the gap. The ADR should
be refined — on explicit maintainer instruction, per the ADR-addition
rule — to describe: **barrier positions** (`BarrierPos` and what each
value means), the **argument-handling categories** a word can occupy
(forward-eligible, mixed-barrier, stack-only, quoting slots), the
**stack-only behaviour** and its closed list (including `apply`'s
`[Function]` case or its removal), and the **chaining rationale**
(why forward collection composes the way it does). Diagnostics should
then *explain* why a word occupies its category rather than merely
reporting a failed dispatch. Recorded as ADR candidate 4 in the
resolution plan; this record stays Pending until the refined ADR
either absorbs the exceptions into the documented rule or the
registrations are changed to conform.

---

## NUR024 — Two orderings by design: semantic (`cmp`) and deterministic (`tcmp`) {#nur024}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict: maintainer, via `design/NUR-RESOLUTION-PLAN.0.md`)

### The uniform rule

One comparison vocabulary, one totality regime.

### The divergence

`cmp`/`lt`/`lte`/`gt`/`gte` raise `[boru/incomparable]` across
families (`cmp true 1` errors) while `eq`/`neq`/`deq` are total
(`1 eq "1"` → false) and `tcmp` is an unrestricted total order — two
totality regimes inside one family, with `cmp` and `tcmp` answering
differently for the same pair.

### Why allowed

The language deliberately carries **two distinct orderings**, and the
divergence is that architecture made visible:

- **Semantic ordering** — `cmp`, `lt`, `lte`, `gt`, `gte`. These
  answer "which is greater, *as values in one domain*?" and therefore
  **reject meaningless comparisons**: `cmp true 1` has no semantic
  answer, and a silent cross-family verdict would hide a real type
  error at exactly the moment it is cheapest to catch.
- **Deterministic ordering** — `tcmp`. This answers "give me *some*
  stable, lawful total order over everything" and exists for
  implementation purposes: deterministic signature ordering,
  deterministic map-key walks, reproducible sorts of heterogeneous
  data. It never rejects, because its job is determinism, not meaning.

Equality (`eq`/`neq`/`deq`) is total in both regimes because "are
these the same value?" has an answer across families (no), while
"which is greater?" does not. The two-ordering separation should also
be stated at the architecture level — recorded as ADR candidate 5 in
the resolution plan (semantic vs deterministic ordering).

### Evidence

- REFERENCE.md §Comparison — both regimes documented with
  the rationale: the ordering words are "**family-restricted**" and
  raise `[boru/incomparable]` across families (:1202-1206), `tcmp` is
  "the **unrestricted** total order" (:1208), and the callout at
  :1214-1216 states "different types are simply *not equal* … Only the
  **ordering** words restrict".
- `eng/go/compare.go` (family restriction raising `incomparable`);
  `eng/go/compare_types.go` (tcmp's Rank-based total order);
  `lang/spec/compare.tsv` and `lang/spec/compare-restrict.tsv` — the
  positive/negative batteries pinning both regimes.

---

## NUR026 — Escape sets diverge between quoted strings and templates {#nur026}

**Status:** Pending · **Recorded:** 2026-07-22 · **Surfaced by:** full-repo uniformity review

**Rule:** one escape vocabulary across string literal forms.
**Divergence:** quoted strings (`"…"`/`'…'`) accept jsonic's full
escape set (`\x41` → `A`, plus `\b`, `\f`, …); backtick templates
process only `\n \t \r \\ \` \$` — `size "z\x41z"` → 3 while the same
text in a template → 6 (the escape survives literally).
**Evidence:** `eng/go/parser/parse.go:1681-1714`
(`processTemplateEscapes`); `eng/go/parser/grammar.go:103-104,340-367`.
**Documentation status:** REFERENCE documents the restricted template
set but never states quoted strings accept a superset — the asymmetry
is undocumented.

**Root cause (source investigation, 2026-07-31):** the divergence is
an **implementation accident, not a design choice**. `setupBaseTokens`
(grammar.go:97-104) deletes the backtick from jsonic's `StringChars`
and `MultiChars` so jsonic's built-in string matcher never consumes
templates — necessary because templates need `${…}` interpolation,
which the plain string matcher cannot provide. That forced a
hand-rolled template scanner (grammar.go:340-367), whose
`processTemplateEscapes` reimplements escapes from scratch as a
minimal six-case switch (`\n \t \r \\ \` \$`) with everything else
falling through to "keep literally" — while quoted strings still ride
jsonic's native escape handling and get the full set. Templates were
severed from jsonic purely to bolt on interpolation, and the
replacement escape handler was never brought to parity.

**Verdict (maintainer, 2026-07-31 — resolve by fix,
`design/NUR-RESOLUTION-PLAN.0.md`):** boru shall **not** use the
jsonic JSON string lexer as-is for strings. Instead: a **custom
unified string lexer** — a vendored copy of jsonic's string lexer,
extended to also handle backtick templates (i.e. `${…}`
interpolation). One lexer then (1) preserves the full escape set
across every string-literal form (the rule this record seeks), (2)
makes string processing uniform — one escape vocabulary in exactly one
place — and (3) parses templates, interpolation included, correctly.
This retires the hand-rolled `processTemplateEscapes` path and its
minimal escape set. Stays Pending until the unified lexer lands.

---

## NUR030 — `group` co-groups deq-distinct keys that render identically {#nur030}

**Status:** Pending · **Recorded:** 2026-07-23 · **Surfaced by:**
PR #309 review (Codex P1) · **Re-opened:** 2026-07-31 (maintainer
review, `design/NUR-RESOLUTION-PLAN.0.md`; was Allowed 2026-07-24 —
the allowance's reasoning is retained below as data, not as a verdict)

All samples below are spelled bare (`group …`) for readability; `group`
lives in `boru:array-util`, so every one runs as
`import "boru:array-util"  ArrayUtil.group …` (a bare `group` is
undefined — the name resolves in `describe` to the unrelated
`boru:query` word).

### The uniform rule

The collection words operate on `deq` classes (NUR011 / NUR015): one
group per value, membership by deep value equality.

### The mechanism (clarified 2026-07-31)

`group` is **not higher-order** — it takes no function. Two forms:

- **1-arg** `group [list]`: each element becomes a Map key; collected
  under it is the list of **indices** where that element occurs —
  `group [1 2 3]` → `{1:[0] 2:[1] 3:[2]}`; `group [1 1.0 2]` →
  `{1:[0 1] 2:[2]}` (1 and 1.0 are one `deq` class). The element is
  the key and the index is the bucketed payload, not the other way
  round.
- **2-arg** `group [keys] [values]`: bucket each value under its
  parallel key — `group ['a' 'b' 'a'] [1 2 3]` → `{'a':[1 3] 'b':[2]}`.

### The divergence

`group` returns a Map, and **a Map key is a rendered string**. Two
keys that are `deq`-distinct but render identically therefore share
one Map entry: `group [Integer Integer/q]` (a type literal and a
same-named atom, `deq`-unequal) yields the single group
`{Integer:[0 1]}`.

### The 2026-07-24 allowance (superseded as a verdict)

The fold is forced by `group`'s Map return shape and is arguably
benign: no index is lost — both occurrences are retained under the
shared key — and the same fold is what makes `group` total over the
common **non-reflexive** keys. Two mechanisms produce them: `nan` is
`DeqKeyed` but never `DeepEqual` to itself under the IEEE rule NUR013
records, while everything that reaches `DeepEqual`'s unsupported
fall-through — which NUR031 tracks and `DeqNeverEqual` mirrors — is
never equal to itself, as is any container or instance transitively
holding one. That second set is not closed, and enumerating it has
proved
error-prone; the reliable test is `x deq x`. Values known to be in it:
functions and words, host payloads, `class` type values and refinements
of one, disjunction types (`tor`, and `enum` on top of it), `fnsig` and
`surface` types, and any uninstantiated `gen` schema whatever its base.
Values known NOT to be in it: concrete Record/Options/Table/Micron type
values, and — since NUR034 — the container/root literals `List`, `Map`
and `Any`, which were non-reflexive when this record was written and
now group through `deq` rather than the render fold. The fold gives
`group [nan nan]` → `{nan:[0 1]}` where raising on a render collision
would make grouping NaN-bearing data a hard error. The lossless
`[[rep group] …]` pair shape was rejected as breaking `group`'s Map
shape and every caller.

### Why re-opened

The allowance treats the render fold as forced; the review pushes one
level down: the **root cause is that Map keys are rendered strings**
at all — whatever you group by is flattened to its text render, and
`group` is one symptom of that language-wide fact.

- **Maintainer proposal to explore:** restrict the grouping-key list
  to **Strings**. A String key IS its render — no lossy step, and
  distinct keys can never collide; this divergence could not arise.
- **Costs identified (why it is not a slam-dunk):** the 1-arg form
  loses generality (`group [1 2 3]` works directly today; String-only
  keys force a conversion first), and NaN totality changes character —
  `nan` could not be a key at all, so the non-reflexive-key problem is
  *forbidden* rather than *folded*.
- **Alternatives on the table:** (a) status quo — any value as key,
  lossy render fold, "benign"; (b) String-only keys — no collisions,
  simpler model, ergonomic cost for non-String data; (c) grouped
  pairs `[[rep group] …]` — lossless, breaks the Map shape (rejected
  by the 2026-07-24 record).
- **Deeper question flagged:** whether Map-keys-as-rendered-strings
  is the real thing to reconsider, language-wide.

Next step: a design decision between (a)/(b)/(c) — possibly folded
into a broader Map-key-identity review. Unresolved until then.

### Evidence

- `lang/go/native/native_array.go` — `deqGrouper.add` (the render-key
  fold, commented).
- `lang/spec/module-array.tsv` §3 — `group [Integer Integer/q]` →
  `{Integer:[0 1]}` and `group [nan nan] [1 2]` → `{nan:[1 2]}` pin
  both the collision fold and the non-reflexive fold.
- REFERENCE.md, the ArrayUtil `deq`-membership callout ("`group`'s map
  keys stay rendered strings") — states the render-key fold, but scoped to
  `group` ("`group`'s map keys stay rendered strings") rather than as
  the language-wide fact about Map keys that the re-open argues is the
  root cause. That the general statement is undocumented is part of
  what this record now tracks.

---

## NUR031 — Function/Word values are not `deq` to themselves; `eq` and order key on the binding name {#nur031}

**Status:** Pending · **Recorded:** 2026-07-23 · **Surfaced by:**
PR #309 review (Codex P2) · **Re-opened in part:** 2026-07-31
(maintainer review, `design/NUR-RESOLUTION-PLAN.0.md`; was Allowed
2026-07-24 — the resolved handle equalities below stand) · **Narrowed:**
2026-08-02

The record's previous title, in force since 2026-07-24, was
"Code/opaque values have no value equality" (it was first recorded as
"Opaque Ideals: `eq` and `deq` are both always false, even
self-compare"). That title is retired: Store, Error, Timeout, Interval
and the Module descriptor now have equality, and a module namespace has
`eq` (below). The re-opened part was the Module/Function/Word
remainder; **the Module half is resolved** (namespace 2026-08-01 by the
NUR038 facet refactor, descriptor 2026-08-02), so what remains open is
**Function/Word identity**:

- a fn value is never `deq` to itself, and
- `eq` and `tcmp` key on the **binding name** a function was reached
  through, so two references to one function agree only when they were
  reached through the same name.

Host `ExtensionPayload` values share the same `deq` fall-through, as do
several kinds of type value — `class` types and refinements of one,
disjunction/`enum` types, `fnsig` and `surface` types, and any
uninstantiated `gen` schema (`P deq P` → false for a
`def P class {…}`; likewise `def E enum ['a' 'b']`). None is separately
recorded, and a fix here should cover them all.

### The uniform rule

NUR011: for Nodes and Ideals, `eq` is reference identity and `deq` is
deep value equality. Every value should at least be equal to itself.

### The divergence (as recorded)

When surfaced (PR #309 review), the rule held only for the structural
families (lists, maps, XML, class/resource instances). Every other
Ideal — Store, Error, Timeout, Interval, Function, Module — fell through
both `ExactEqual` and `DeepEqual` to `false`: not `eq` to itself, not
`deq` to itself.

### What was resolved

The **stateful and value-like handles now follow the rule** (this PR):

- **Store** — `eq` is reference identity (the `*StoreInstanceInfo`
  pointer: a store IS its handle), `deq` is its deep entry value (the
  same own-entry projection as `convert Map`, recursed).
- **Error** — a value-like Ideal (an immutable `ErrorInfo`, no
  reference), so `eq` and `deq` both compare its fields (code, message,
  payload map), coinciding like a scalar leaf.
- **Timeout / Interval** — opaque handles whose identity IS their value,
  so `eq` and `deq` are both pointer identity.

Implemented in `eng/go/compare.go` (`opaqueIdealExactEqual` /
`opaqueIdealDeepEqual` / `storeDeepEqual` / `errorInfoEqual`), mirrored
in `eng/go/compare_deqkey.go` (`isDeqComparableHandle` → `DeqUnkeyed`,
scanned pairwise). Verified in `eng/go/compare_nur031_test.go`,
`lang/spec/compare-restrict.tsv`, and `lang/spec/edge-containers-1.tsv`
§8 (whose rows deliberately pinning "Stores have NO identity" were
rewritten — this PR overturns that earlier design decision, exactly
what the NUR process is for).

### The remainder, reviewed (2026-07-31) — the re-opened part

The **code / opaque values** — `Function`/`FnDef`, `Module`/
`ModuleExport`, `Word` — kept the equal-to-nothing behaviour, which
the 2026-07-24 record accepted wholesale. The review splits that
acceptance:

- **Accepted as current behaviour** (correction, 2026-07-31 audit):
  the "rejected at dispatch" claim holds only for BARE operands
  (which auto-invoke before comparison). `eq`/`deq`'s signatures are
  `[Any Any]` and DO admit fn values arriving as **container data** —
  but only for a fn that cannot auto-invoke at the read: a 0-arg fn
  read out of a map (`m.run`) dispatches on the spot, so that spelling
  compares call RESULTS, not functions. Using `f/r` directly — a shape
  the compiler refuses ("function value reaches eq"), so the default
  and `-no-compile` paths both run it interpreted and agree — the
  current answers are:

  ```
  def f fn [[n:Integer] [Integer] [add n 1]]
  def a (f/r)   def b (f/r)          # two BINDINGS of one function

  f/r eq   f/r   →  true     # same binding name
  f/r deq  f/r   →  false    # deq is never-equal for fn values
  f/r tcmp f/r   →  0

  a/r eq   b/r   →  false    # different binding names, one function
  a/r tcmp b/r   →  -1       # …and b/r tcmp a/r → 1: a real order,
  a/r deq  a/r   →  false    #    but keyed on the CANON, i.e. the name
  ```

  `ExactEqual`'s type-body arm (`compare.go:359`) requires Parent
  equality, so before the ADR-011 collapse even the same-name `eq`
  failed when Parents differed (`Word/__FN` vs `Type/Function`) — that
  case is RETIRED (one Function type; NUR050 resolved), which is why
  `f/r eq f/r` is true today. What remains is the rest: `deq` is
  never-equal for fn values at all, and canon/render keys on the
  BINDING NAME (`def a (f/r)` canons `fn a[…]`, `def b (f/r)` canons
  `fn b[…]`), so two references to one function are `eq`-false and
  order apart whenever the names differ — pre-existing, not a collapse
  regression. Function identity — what makes two references "the same
  function" — is exactly this record's open design work. Tolerable
  while the deeper question below is open.
- **Re-classified as an open defect (NOT a benign allowance):**
  `Module`/`ModuleExport` values DO reach `eq`/`deq` and return
  `false` **including against themselves** — a silent violation of
  reflexive equality, the same half-handled-value-kind pattern as
  NUR050 (and the since-resolved NUR051). A wrong answer, delivered
  quietly.

  **Namespace half RESOLVED by construction (2026-08-01, commit
  `d8f93d3`):** the NUR038 facet refactor retired the
  `Ideal/ModuleExport` wrapper — `import` now binds a plain export Map
  (module-namespace facet), so a namespace takes the ordinary Node
  equality arms with no module special-casing at all: `M eq M → true`
  (shared `*OrderedMap` identity), and two namespaces of DIFFERENT
  exports compare `eq → false`, exactly the Map contract.

  The `deq` side inherits that contract *including its limits*. A
  namespace is `deq`-reflexive only when every export is itself
  `deq`-reflexive: `{x:1 y:"two"}`-style exports give `M deq M → true`
  and content-equal-but-distinct namespaces `deq → true`, while a
  module exporting a **function** gives `M deq M → false`
  (`IO deq IO`, `Test deq Test`, `StringUtil deq StringUtil` are all
  false today), because `DeepEqual`'s Map arm recurses into the export
  values and fn values hit its terminal `false`. Exporting any other
  value that reaches that fall-through does the same — a `class` type,
  an `enum`, a `gen` schema (`export {P: P}` for `def P class {…}`
  gives `M deq M → false`, since `P deq P` is false) — while exporting
  a bare type literal does not: `export {L: List}` stays reflexive.
  This is not a
  module defect and not unlanded module work — a plain `{a:1 g:(f/r)}`
  Map behaves identically — it is the Function/Word `deq`
  fall-through below, seen through a namespace. Nothing in `lang/spec`
  pins namespace `deq` either way (its only module eq/deq rows, in
  `frontier/frontier-nur031-module-eq.tsv`, are all `.$module`
  descriptor rows), which is why the over-claim survived.

  **Descriptor half RESOLVED (2026-08-02):** `Ideal/Module` now
  follows the opaque-handle rule the Timeout/Interval arms
  established — an opaque handle's identity IS its value, so `eq` and
  `deq` are both reference identity. `NewModuleInstance` boxes a
  `*ModuleDesc` in its `ExtensionPayload` (the payload wrapper is
  KEPT: four kernel arms key on `ExtensionPayload`, and `ModuleDesc`
  holds a Go map so `==` on the bare struct would panic), and the
  kernel compares that pointer. `M.$module eq M.$module → true`,
  `deq` likewise; descriptors of different modules compare false.
  The Sealed Payload rule is honoured — the kernel asserts the
  payload to its own type for IDENTITY only and never reads a field.
  The module defect this record raised is therefore closed; the
  reflexivity requirement below is met for modules.

**Standing requirement (maintainer, 2026-07-31; module half
discharged 2026-08-02):** every value — functions and modules
included — must eventually fall under equality, at minimum
reflexively (a value is `eq`/`deq` to itself). **Modules satisfy it
for `eq` unconditionally, and for `deq` on the descriptor
unconditionally**; a namespace satisfies `deq`-reflexivity only when
every export does, and the exports that do not (functions, and the
type values that share their fall-through) are the general `deq` gap,
not a module gap. The function-type-vs-value question is
settled (ADR-011: one `Function` type; NUR050 resolved), so what this
record still tracks is function IDENTITY — a stable canon independent
of the binding name, since canon/render today keys on the name a
function was reached through — plus the Behavior routing. The likely
shape — routing `eq`/`deq` through the type's `Behavior` for Ideals
rather than the kernel's hardcoded arms (the future ADR the
2026-07-24 record deferred to) — is this record's own proposal for
that work, not a commitment ADR-011 made. NUR050 is resolved and
retired, so there is nothing left to track alongside; what ADR-011 did
record in its Consequences is the deferral itself — "Two references to
the same function still compare unstably … that is function IDENTITY,
the NUR031 equality work, deliberately not solved here" (ADR.md,
§Consequences; its parenthesised `(f/r) tcmp (f/r)` → -1 example
predates the collapse and no longer runs — see the table above for the
current spellings) — which is the work this record inherits. Note the
Sealed Payload constraint
stands and was honoured by the descriptor fix: module handles are
backed by `ExtensionPayload`, which the kernel deliberately does not
inspect (eng/go/CLAUDE.md "Sealed Payload") — reference identity
compares the boxed pointer without reading a field.

### Evidence

- `eng/go/compare.go` — the resolved handle arms; the type-body arm at
  :359 that answers `eq` true for a same-canon fn pair; the Map arm of
  `DeepEqual` (:517, arm at :587-605) that recurses into export values
  via `deqMapEntries` (:502); and
  `DeepEqual`'s terminal `false`, which is what answers `f/r deq f/r`
  and `IO deq IO`.
- `eng/go/compare_deqkey.go:48` — `DeqNeverEqual`, which MIRRORS that
  fall-through for the bucketed collection scans
  (`unique`/`member`/`indices`/`group`, `native_array.go:939,1140,
  1169,1195`). It is not on the `deq` word's own path.
- `lang/go/native/native_module_types.go` — the plain export Map
  carrying the module-namespace facet, and the `ExtensionPayload`-backed
  descriptor handle.
- `eng/go/compare_nur031_test.go` — `TestNUR031ModuleDescriptorEquality`
  pins the descriptor half.
- `lang/spec/compare-restrict.tsv`, `lang/spec/edge-containers-1.tsv`
  §8, `lang/spec/edge-containers-2.tsv`, `lang/spec/edge-errors-2.tsv`.

---

## NUR039 — `slice` with a negative start silently ignores its end argument {#nur039}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

### The uniform rule

An argument is honoured or refused, never ignored. Out-of-domain
indices elsewhere in the String family clamp predictably
(`slice 5 6 "abc"` → `''`, `slice 0 5 "-"` → `'-'`).

### The divergence

A NEGATIVE start silently collapses `slice start end s` to the
two-argument "drop N from the end" form, discarding `end` entirely:

```
slice -3 -1 'abcde'   →  ab
slice -3  2 'abcde'   →  ab
slice -3  5 'abcde'   →  ab
slice  1  3 'abcde'   →  bc     (the positive form honours end)
```

Three different `end` values, one answer. The negative-index
convention is documented as "count from the end"; that an `end`
argument is then dropped is not.

### Why allowed

The affected spelling is a negative start, which every caller in this
repository can avoid by clamping — and clamping is what a caller wants
anyway, since a negative index is a bug at the call site more often
than an intent to count from the end. The alternative fixes (honour
`end` for a negative start, or refuse the combination) are both
behavioural changes to a core sequence word, which is a larger edit
than the confusion it removes.

The acceptance rests on callers not reaching the spelling, so the
guard that matters is an *upstream* one:

- `utils/cut.boru`'s `cut-span` (:180) and `cut-point` (:165) reject
  `lo < 1` before any range reaches the slicing helpers
  (`cut-err-rng "fields and characters are numbered from 1"`), so no
  negative start is constructed in the first place.
- `utils/tests/cut_test.boru` pins that rejection and the clamped
  behaviour at both ends.

**Correction (2026-08-02 review).** This record previously claimed the
pin was that `cut-chars-rng` "clamps the start explicitly". It does
not: `cut-chars-rng` (utils/cut.boru:329-335) computes
`def a ((rg get 0) sub 1)` with no start clamp and clamps only the
END (`def b (if (hi gt n) [n] [hi])`); its `if (a gte b)` guard is an
empty-range test that a negative `a` against a positive `b` passes
straight through. Replaying its body with `lo = 0` reproduces this
record's own divergence inside the function that was cited as its pin.
The register inherited the error from the source comment above that
function, which mis-described its own body until this review corrected
it (the comment now runs utils/cut.boru:322-328 and says the opposite).
The acceptance survives — the real guard is the upstream `lo < 1`
rejection above, so `cut` is correct today — but the local fragility
is now recorded rather than mis-pinned.

**Correction (2026-08-02 review).** The motivating example previously
given here — `slice (ep add 1) (size tok) tok` where `ep` is `-1` from
a failed `indexof` — does not exhibit this divergence: `(ep add 1)` is
`0`, a NON-NEGATIVE start, and `end` is honoured normally. It illustrates
an off-by-one, not the negative-start collapse. The spelling that does
trigger it is the same call without the `add 1`.

### Evidence

- The four `slice` calls above, verified on the current binary.
- `utils/cut.boru:165,180` (the upstream `lo < 1` rejection) and
  `utils/tests/cut_test.boru`.
- NUR019 records the separate question of where `slice` belongs, and
  its 2026-08-02 verdict is that `slice` is a core **sequence** word,
  not a String-family straggler; this record is an independent defect
  in the same word and takes no position on filing.

---

## NUR040 — `set` quotes a bare computed key where `get` refuses it {#nur040}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** C3 `boru:cli`
scouting

### The uniform rule

Sibling accessors treat their key argument the same way, and a program
that means a variable's VALUE does not silently get its NAME.
lang/go/CLAUDE.md states the split it intends: `dot`/`dotr` quote a
bare word as a literal field name, `get`/`getr` evaluate it.

### The divergence

`set` carries the quoting `Atom/q` slot that `get` does not, so the
same bare-word spelling means opposite things:

```
def k "aa"   {} set k 1     →  {k:1}      # the NAME was stored
def k "aa"   {} set (k) 1   →  {aa:1}     # the VALUE
def k "aa"   {aa:1} get k   →  1          # get EVALUATES k
```

`boru check` reports no ERROR for the first line. It does emit
`[warning] unused_def: def k is never used` — which is the tell, since
that warning appears for neither alternative spelling — but nothing
names the actual hazard. The failure mode in real code is a map built
entirely under one literal key: every iteration of a loop overwrites
`{k:…}`, and only an unused-binding warning hints at it.

### Why allowed

The asymmetry leaks from a distinction that is deliberate and
load-bearing elsewhere — `dot`/`dotr` quote a bare key, `get`/`getr`
evaluate one (lang/go/CLAUDE.md, "dot / dotr vs get / getr"). Making
`set` match `get` is a behavioural change to a core word, which is a
larger and riskier edit than the confusion it removes. The quoting slot
has a real purpose (`set name value store` reads well).

The standing improvement, not required by this allowance: a check-mode
advisory when a bare word passed to a quoting slot is ALSO a live
binding — the one case where the two readings differ and the author
almost certainly meant the value. The `unused_def` warning above is an
accidental partial signal of exactly that condition.

### Evidence

- The three calls above, and the three `boru check` runs behind the
  warning claim.
- **The two files this record was scouted from never pass a bare word
  to `set`'s key slot**, so neither depends on which way the ambiguity
  resolves: `utils/` spells every LITERAL key `(quote k)` (117 sites, 0
  exceptions), and `lang/go/modules/cli.boru` uses `(quote …)` at its
  75 literal-key sites (42 distinct names) and the parenthesised value
  form (`set (nm) …`) at its 8 computed ones. Its house rule at
  cli.boru:53-54 states the convention: "a computed map key is always
  parenthesised (`m set (k) v`) — a bare `k` stores the literal name
  \"k\", with no diagnostic at all."
- **Elsewhere the repo does rely on the quoting reading**, which is the
  real reason the fix is riskier than the confusion:
  `lang/go/modules/vault_tui.boru` — shipped, `//go:embed`-ed — has 77 bare-word key
  sites (`grep -oE '\bset +[a-z][a-zA-Z0-9_-]*'`; 75 excluding the two
  that follow a `-`-suffixed word) (`state set screens …`, `state set status …`),
  and `kg/report.boru:334`, `design/examples/apps/todo-tui.boru:52` and
  the linguist samples do the same. Making `set` evaluate its key would
  change all of them.
- lang/go/CLAUDE.md:303-316 — the "**`dot` / `dotr` vs `get` / `getr`
  (CRITICAL)**" bullet inside §"Parser Customization" (a bolded
  lead-in, not a section) — the deliberate split this record's
  divergence leaks from.

---

## NUR046 — `boru fmt` is not idempotent: one pass is not a fixed point {#nur046}

**Status:** Allowed · **Recorded:** 2026-07-30 · **Verdict:** maintainer, 2026-07-30 · **Surfaced by:** the C3 utils
suite (`utils/`)

### The uniform rule

A formatter is idempotent. `fmt(fmt(x)) == fmt(x)`, so "formatted"
is a property a file either has or does not, a `make fmt` target converges,
and a formatting check can be a single-pass diff. `make fmt-docs` and
`kg/Makefile`'s restored `fmt` target both rely on this.

### The divergence

On a `def name fn [[params] [Returns] [body]]` whose header
does not fit the width, the FIRST pass and the SECOND pass produce different
layouts. It converges at pass 2 — passes 2..n are identical — so the fixed
point exists; one application simply does not reach it.

```boru
# m.boru, as hand-written:
def cat-format fn [[line:String k:Integer numbered:Boolean ends:Boolean] [String] [
  def body (if ends [(join "" [line "$"])] [line])
  join "" [body "\n"]
]]
```

```
$ boru fmt m.boru && cat m.boru          # pass 1
def cat-format fn
  [[line:String k:Integer numbered:Boolean ends:Boolean] [String] [
  def body (if ends [(join "" [line "$"])] [line]) join "" [body "\n"]
]]

$ boru fmt m.boru && cat m.boru          # pass 2 — different, and stable
def cat-format fn
[[line:String k:Integer numbered:Boolean ends:Boolean] [String]
      [def body (if ends [(join "" [line "$"])] [line]) join ""
          [body "\n"]
      ]
  ]
```

The blast radius is in §Evidence below; program output is unchanged in
every affected file, and every one still passes `boru check`.

**Why it matters:** three ways.

1. A `fmt` target inside an `all:` target never converges in one run, so
   `make all` always leaves a dirty tree — which is why `utils/Makefile`
   deliberately keeps `fmt` OUT of `all` and says so, the same posture
   `kg/Makefile` held while NUR028 was open.
2. Pass 1 joins two statements onto one line (`… [line]) join "" [body …`)
   and pass 2 re-indents a statement as though it continued the previous
   one. Both are legal — boru is whitespace-insensitive — but a reader
   cannot tell statement boundaries by eye any more, which is most of what
   a formatter is for.
3. It is a fixed-point bug in the same component as the resolved
   superlinear blow-up, in a shape that blow-up's gate would not have
   caught: that gate compared old-binary and new-binary output on the
   *repo's already-canonical* corpus, where pass 1 is already the fixed
   point. Non-canonical input is the untested axis.

### Documentation status

`kg/Makefile:25-28` claims idempotence in so
many words — "the formatter is idempotent, so once they are canonical
this is a no-op on the tree and `make all` leaves nothing to commit" —
with `fmt` inside its `all` target (kg/Makefile:11). kg's own sources
happen to sit at their fixed point, so no dirty tree results today, but
the written claim is false in general. `kg/README.md` and `make
fmt-docs` likewise treat a single `fmt` run as producing canonical
form.

**The mechanism (corrected 2026-08-02).** This record originally
proposed that "the first pass measures widths against a pre-wrap layout
decision it then invalidates". `design/NUR-EFFORT-TRIAGE.0.md:139-148`
(the NUR046 bullet; the cause statement at :140-141) investigated and
found otherwise: the true cause is **re-parse
statement-segmentation drift** (root-level newlines emitted by pass 1
change how pass 2 segments statements). The width-memoisation framing
is retired.

**The standing fix, when scheduled:** a regression guard belongs with
it — format every `.boru` in the repo TWICE and require the second pass
to be a no-op, with at least one deliberately non-canonical fixture,
since the already-canonical corpus cannot detect this.

### Why allowed

Formatting does not change behaviour — all 995 cases in `utils/` pass either
way, verified — so what the non-idempotence costs is a clean tree and
readable sources, not correctness. It converges at the second pass, so a `fmt` target
that ran twice would be stable; the reason not to paper over it that way is
that the intermediate layout runs statements together on one line, which is
most of what a formatter is for.

### Evidence

- The repro above, and the **repo-wide sweep** (re-run 2026-08-02 on
  the current binary): of the 122 tracked `.boru` files, **19 are
  non-idempotent** — all 12 `utils/*.boru` programs, three SHIPPED
  library modules (`lang/go/modules/cli.boru`, `sift.boru`,
  `vault_tui.boru`), two `design/examples` programs and two
  `editors/linguist/samples`. All 11 `utils/tests/*_test.boru` suites
  ARE at their fixed point after one pass, which is the qualitative
  split that makes the divergence easy to miss.
- `utils/Makefile` keeps `fmt` OUT of its `all` target and its comment
  names this record and explains why — the same posture `kg/Makefile`
  held while its own formatter blocker was open — so the tree cannot
  silently start churning on every build.
- `kg/Makefile:11,25-28` — the idempotence claim named above, the one
  place the property is asserted rather than assumed.

**Correction (2026-08-02 review).** This record previously said the
non-idempotence hits "all six programs" in `utils/` with "the five
`tests/*.boru` suites" already at their fixed point. Those counts were
accurate on 2026-07-30 when the record was written (the tree then held
six programs and five suites) and have since drifted: it is 12 and 11,
and the blast radius reaches shipped `lang/go/modules/*.boru`, a scope
the record never mentioned. An **Allowed** record carries "the evidence
that pins it … so the acceptance cannot silently rot"; this evidence
had rotted by a factor of two.

---

## NUR059 — Several value kinds render in DEBUG spelling inside canon {#nur059}

**Status:** Pending · **Recorded:** 2026-08-08 · **Surfaced by:** closing
the Go/TS parser parity ledger (`parser/spec/divergent.tsv` to zero)

**Rule:** `CanonValue` renders canonical boru **source** — the string it
produces parses back as the same value. Every other container type tag now
obeys it: `[:Integer]`, `[:Integer 1 2]`, `{:String}`, `{:Integer a:1}`.
**Divergence:** several kinds fall through to `Value.String`'s debug form,
which is not boru syntax and does not parse back:

```
[:Box<Integer>]   canon ->  [:sugar(angle Box [word(Integer)])]
foo/r             canon ->  word(foo)                  — the /r is LOST
foo/2             canon ->  word(foo)                  — the /2 is LOST
(1 add 2)/q       canon ->  paren([1 word(add) 2]) /q
[:Integer]        canon ->  [:Integer]                 (correct)
foo/q             canon ->  foo/q                      (correct)
```

Each has no source-form renderer, so the generic value path yields the
debug spelling. The `/r` and `/2` cases are the worst of them: the
modifier is not merely spelled oddly, it is **dropped**, so the canon says
something the source did not.

**Evidence:** `parser/spec/parse.tsv` pins the current output for each of
these. Found by `scripts/parity-probe.sh` sweeping the language surface;
the same sweep caught the dispatch-mod marker rendering two DIFFERENT
debug spellings (Go `word()({false true})`, TS `word(undefined)`), which
was a genuine parity defect and is fixed — these are the residue where
both engines agree on a wrong render.
**Both engines AGREE**, so this is not a parity defect and correctly does
not sit in `divergent.tsv` — it is a render-quality gap that the parity
work made visible by fixing every neighbouring case.
**Proposed verdict:** fix — canon needs a source-form renderer for each
of these (`Box<Integer>` for `SugarAngle` and the other sugar kinds, the
`/r` and `/N` word modifiers off `WordInfo`, and the paren group), on both
engines. Until then the corpus rows are the pin: a fix to one engine alone
fails loudly.

## NUR052 — Store enumeration reads the top COW layer; lookup walks the chain {#nur052}

**Status:** Pending · **Recorded:** 2026-08-02 · **Surfaced by:**
NUR-EFFORT-TRIAGE probing of NUR022 (del/set symmetry)

**Rule:** a container's enumeration agrees with its lookup — the keys
a Store *shows* (`size`, `convert Map`) are the keys it *answers for*
(`get`, `has`). (`each` has no Store signature at all, so iteration is
not a third enumeration path here; a Store is walked by converting it
first, which is the projection at issue.)
**Divergence:** Store enumeration reads only the newest copy-on-write
layer while lookup walks the full prototype chain:

```
$ boru do 'context set a/q 1  context set b/q 2
           print (size (context))            # 1
           print ((context) get a/q)         # 1   — lookup sees a
           print ((context) has a/q)         # true
           print (convert Map (context))'    # {"b": 2} — enumeration does not
```

Two sets, two live keys by lookup, one key by enumeration.
**Evidence:** the session above (verified 2026-08-02, current binary);
`eng/go/value.go::StoreInstanceInfo.Get` (prototype-chain walk) vs
`eng/go/convert_ideal.go::storeEntryMap` (own-entry projection).
**Documentation status:** NUR031's resolved Store-equality arm
describes `deq` as "the same own-entry projection as `convert Map`" —
so the asymmetry also leaks into which Stores compare `deq`-equal.
Not otherwise documented.
**Proposed verdict:** investigate then fix or argue — either
enumeration walks the chain (with masking, so a child layer's key
shadows its parent's), or lookup is documented as deliberately
chain-walking while enumeration is own-layer (and the words' docs say
which they use). Any `del`-symmetry work under NUR022 must land on
whichever answer is chosen (a tombstoned key must be invisible to
BOTH).

**NUR022's `del` landed 2026-08-02 and satisfies that constraint**
without deciding the record. `CowDel` writes a tombstone into a NEW
layer whose own `Data` is empty, so a deleted key is invisible to
lookup (Get honours the tombstone) and invisible to enumeration
(there is no own entry to enumerate) — verified:

```
$ boru do 'context set a/q 1 context set b/q 2 context del b/q end
           print ((context) has b/q)        # false
           print (convert Map (context))    # {}
           print (size (context))           # 0
           print ((context) has a/q)'       # true — still reachable by lookup
```

The `{}` / `0` in that session is this record's divergence, not the
delete: enumeration under-reports `a` exactly as it did before. So the
tombstone is neutral here — it does not deepen the split and does not
close it, and whichever answer this record takes, the tombstone
follows it for free (a chain-walking enumeration would need to honour
`Deleted`, which is the same predicate `Get` already applies).

---

## NUR053 — The truthiness consumers do not share one domain {#nur053}

**Status:** Pending · **Recorded:** 2026-08-02 · **Surfaced by:** NUR
register review (auditing NUR001's rationale)

**Rule:** one truthiness model, applied by every construct that
coerces a value to a Boolean. `design/TRUTHINESS.0.md` (the One
Truthiness Model) enumerates the consumers and states that
"`convert Boolean` and `make Boolean` apply the same presence rule";
NUR001's allowance leans on exactly that shared rule.
**Divergence:** the consumers share the *rule* but not the *domain*.
`if` and `make Boolean` accept any value; `convert Boolean`'s source
slot is Scalar-only, so three members of the language's own falsy set
raise instead of coercing:

```
$ boru do 'print (make Boolean [1])'          # true
$ boru do 'def xs [1]  print (convert Boolean xs)'
  error: [boru/signature_error]: cannot call `convert` — no signature matches the arguments
$ boru do 'print (make Boolean none)'         # false
$ boru do 'print (if none ["T"] ["F"])'       # F
$ boru do 'print (convert Boolean none)'
  error: [boru/signature_error]: cannot call `convert` — no signature matches the arguments
```

The same holds for `[]` and `{}`. No `convert Boolean` signature admits
a List, Map or None SOURCE: with a Scalar target only `[Scalar Scalar]`
and `[Scalar Map Scalar]` (the options form) match, and both are
Scalar-sourced. (`boru describe convert` does list `[Bytes List]` and
`[List Bytes]`, but those are other targets — `convert Bytes xs` works.)
**Evidence:** the session above (verified 2026-08-02, current binary);
`lang/go/native/native_type.go` — `convert`'s Scalar-typed source
slots and `coerceBooleanTruthy`; `design/TRUTHINESS.0.md:58-67` — the
statement this contradicts, whose own example is `make Boolean [1]`.
**Documentation status:** mis-documented rather than undocumented. Three
places assert the shared rule without the domain caveat:
`design/TRUTHINESS.0.md` §2, REFERENCE.md's `convert Boolean`
description ("judges only presence — empty String / `0` / `none` /
empty collection are false"), and the SHIPPED help text
(`boru describe convert`: "pure presence coercion: empty String / 0 /
none / empty collection are …"). NUR001 (Allowed) rests its rationale
on the same assertion; its allowance is about *content vs presence* and
survives — this record is the domain question it was silently
carrying.
**Proposed verdict:** argue or fix — either widen `convert Boolean`'s
source slot to `Any` so the three consumers coincide (the uniform
answer, and `convert` already has a total presence rule to apply), or
state the Scalar-only domain at TRUTHINESS.0.md and at `convert`'s
documentation and argue why conversion is narrower than coercion.

**Note (2026-08-02): the RULE is now pluggable; the DOMAIN split is
untouched.** `CoerceBoolean` gained a `Truther` capability walk, so a
type can define what its values mean in a boolean position instead of
inheriting the family cascade's render-based guess (see the capability
work landed with this review). That reaches all three consumers
uniformly — `convert`'s own path, `coerceBooleanTruthy`, delegates to
`CoerceBoolean` after its YAML-token check, so there is exactly one
truthiness implementation and the capability applies to every caller
of it. What the capability does NOT do is widen `convert Boolean`'s
source slot: a List, Map or None source still fails to match a
signature and never reaches the rule at all. That is this record's
divergence, and it stands unchanged.

---

## NUR054 — Context write boundaries differ between the interpreter and the compiler {#nur054}

**Status:** Pending · **Recorded:** 2026-08-02 · **Surfaced by:** NUR
register review (sweeping for divergences described in code but not
recorded)

**Rule:** the execution engines agree. `lang/go/boru.go:911-916` states
the `RunCompiled` contract as "identical results either way, the flag
only changes the execution engine", and its `LIMITATION` note
immediately below claims "the step budget is **the one place** this is
NOT byte-for-byte transparent". `design/COMPILABLE-SUBSET.md`:7-9 (the
preamble, restated at §1 "The contract", :23-35) states the fallback
discipline: where the compiled path
cannot lower faithfully it must **refuse** the unit and fall back, so a
divergence is "slow, not wrong". NUR037 was a violation of that
discipline, resolved by making the compiler refuse. NUR051 is the
contrary
precedent and sharpens the point — there the refusal ITSELF was the
recorded defect ("anything that runs interpreted must also compile"),
and ADR-010 resolved it by mandating the emitter intern nested type
literals rather than decline them.
**Divergence:** which call forms open a context write boundary is not
the same on the two engines; the compiled path does not refuse, it
answers differently; and this is reachable on the **default** path, so
the step budget is not the one place:

```
$ cat cb.boru
case 1 [ 1 [ context set y 1 5 ] 2 [ 6 ] ]
print (context has y)

$ boru run cb.boru               # true   — default (compiled)
1 5                              #   (residual stack, identical on both)
$ boru run -no-compile cb.boru   # false  — interpreted
1 5
```

The interpreter's answer is the intended one, so the default path is
the wrong one. `lang/go/context_boundary_differential_test.go` pins
four such forms as `wantDiverge`: a `case` clause body, an `otherwise`
list argument, a `def name [list]` auto-evaluation, and an unused fn
list argument.
**Evidence:** the session above (verified 2026-08-02, current binary);
`lang/go/context_boundary_differential_test.go:61-64` (the
`wantDiverge` field and its required `why`) and its four marked rows
at :109-131; EXPLANATION.md §"Store and context", the "Two caveats"
paragraph, tells users about the split ("a
`case` clause body and an auto-evaluated list are boundaries when
interpreted but not when compiled … The interpreter's answer is the
intended one");
`design/verse-report-defects-investigation.0.md:1203-1206` states
"the interpreter is itself inconsistent about which call forms are
context boundaries. That inconsistency has to be settled."
**Documentation status:** documented as behaviour in EXPLANATION.md
and pinned as expected in a differential test, but never argued: the
test's own header says a row that starts agreeing "is good news", so
the divergence is treated as a temporary shortfall with no verdict.
**Proposed verdict:** investigate then fix or argue. Unlike NUR037
this is not a refusal — the compiled unit runs and writes to a
different store — so the "slow, not wrong" escape does not apply as
recorded. Either the compiler brackets the four forms, or the
compiled path refuses units containing them (the NUR037 mechanism —
noting that NUR051/ADR-010 is precisely why refusal is the weaker
option here: there a refusal was itself ruled a bug), or the
interpreter's boundary set is itself narrowed to what the compiler can
honour and the change is argued at the language level. The
`verse-report` note asks for the interpreter's own inconsistency to be
settled first; that ordering looks right.
Whichever way it lands, `boru.go`'s "the one place" wording needs
correcting — either to name this as the second exception, or to
disappear because the exception did.

**Related, deliberately not folded in:** the step-budget divergence the
same comment records (the interpreter meters per tape token, the VM per
bytecode instruction, so at the ceiling a long-but-terminating program
may complete compiled and abort interpreted). That one is
one-directional, argued in place, and pinned by
`TestStepBudgetNoSpuriousLimit`; it is a candidate for its own
**Allowed** record rather than part of this defect.

---

## NUR056 — `make`-constructibility is the one capability with no opt-in {#nur056}

**Status:** Pending · **Recorded:** 2026-08-02 · **Surfaced by:** NUR
register review (auditing the TypeBehavior capability surface)

> Numbered 056, not 055: NUR055 was opened and resolved earlier in this
> same review (`cde2d3f` then `b5051be` — Big numeric values reading as
> uniformly falsy), and a resolved record's number is retired forever.

**Rule:** a type says how it participates in a kernel operation
through its Behavior — one mechanism, reachable from Go via a
capability interface and from boru via a `behave` slot.

**Divergence:** every kernel operation follows that rule except
construction. Ordering, rendering, membership, unification, hashing,
walking, projection, const-baking, truthiness, deep equality and size
are all capability-dispatched, and seven of them have `behave` slots
(`compare`, `canon`, `nodify`, `unify`, `truthy`, `deq`, `size`).
`make` has neither: its scalar arm is a closed switch over the kernel
leaves, and its Ideal arm dispatches through a *registry* of Ideal
kinds rather than through the type's Behavior.

```
$ boru do 'def C (refine Float) behave make/q (fn [[String] [C] [1.0]])'
  error: behave make: unknown behavior name;
         known: canon, compare, deq, nodify, size, truthy, unify
```

So a user type can define what it MEANS in every operation that
consumes it, and nothing about how it is BUILT. A Go-side Ideal can
(via `Ideal.Instantiate`), which makes this also a Go-vs-boru
asymmetry, not only a missing capability.

**Evidence** (paths relative to the repo root):
`eng/go/core_make.go` — `MakeConvert`'s `default:` arm raising
"make: unsupported target type" is the closed scalar switch;
`MakeObjHandler`'s `reg.Ideals.For(targetVal).Instantiate` is the
Go-only Ideal hook. `lang/go/native/native_behave.go` — the
`behaviors` table, seven entries, no construction slot.
`eng/go/typebehavior.go` — the capability interfaces that do exist.

**Documentation status:** undocumented. `boru describe behave` now
lists the seven installable slots, and nothing states that
construction is not among them or why.

**Proposed verdict:** argue or fix. The fix is a `Maker` capability
(`fn [[Any] [T]]`, dispatched by `MakeConvert` before its switch and
by `MakeObjHandler` before the Ideals registry) plus a `behave make/q`
slot, which would make the capability surface complete. The argument
for allowing it is that construction is not a property of a value —
there is no receiver to dispatch on, only a target type and an
arbitrary source — so it belongs to the type's CONSTRUCTOR
registration rather than to its value Behavior, and the Ideals
registry is the right home. If that argument is taken, it should be
stated where the capability list is documented, since the list is
otherwise read as exhaustive. Note that this record does NOT block on
the sentinel-values programme or on NUR018 (`Store`/`Error`
deliberately not being `make` targets) — those decide WHICH types
construct, this decides WHO gets to say how.

---

## NUR057 — The compiler exempts `set`/`del` by name on an unenforced no-shadow claim {#nur057}

**Status:** Pending · **Recorded:** 2026-08-03 · **Surfaced by:**
lang/eng content audit (`design/LANG-ENG-CONTENT-AUDIT.0.md`)

**Rule:** a kernel special case keyed on a word NAME is sound only
when the name reliably denotes the kernel registration. The
sealed-word tier exists to guarantee exactly this ("the words the
engine special-cases BY NAME, where a shadow binding would break the
identity the kernel relies on" — `eng/go/word_extend.go:27-42`), and
the one other name-keyed compile bypass verifies binding identity
instead of trusting the name (`flex`:
`eng/go/callable_words.go:799-808` checks signature pointer identity
against the live registry binding).

**Divergence:** the quote-arg compile gate and the poly-admission gate
both exempt `set` and `del` purely by name — `eng/go/emit.go:4499`
(`… && word != "set" && word != "del" …`) and
`eng/go/carrier.go:1538-1548` — on the stated ground that "`set`
cannot be shadowed (it is a builtin)" (`emit.go:4509`). But
`sealedWords` contains only `def`/`make`/`word`; `set` and `del` are
extendable (`InstallWordExtension` refuses only sealed names), so an
extension of `set`/`del` reaches the name-keyed exemptions the
comment claims impossible.

**Evidence:** `eng/go/emit.go:4499-4510`;
`eng/go/carrier.go:1538-1548`; `eng/go/word_extend.go:38-42` (the
sealed set); `lang/go/native/native_storage.go:26,224` (the `set` /
`del` registrations).

**Documentation status:** the no-shadow claim exists only in the
`emit.go` comment; nothing documents `set`/`del` as sealed (they are
not).

**Proposed verdict:** resolve by fix — either add `set`/`del` to
`sealedWords` (documenting them as kernel identities) or key both
exemptions on binding identity like the `flex` gate.

---

## NUR058 — Language-layer guaranteed-error mirrors are emitted unstamped {#nur058}

**Status:** Pending · **Recorded:** 2026-08-03 · **Surfaced by:**
lang/eng content audit (`design/LANG-ENG-CONTENT-AUDIT.0.md`)

**Rule:** a check diagnostic that mirrors a GUARANTEED runtime error
carries `CheckDiagnostic.RuntimeMirror` — stamped by
`CheckAddUniqueDiagnostic` for all its callers, set explicitly by
direct emitters — so the compile pipeline's error-diagnostic refusal
skips it and the check and compile passes report the SAME diagnostics
(eng/go/CLAUDE.md §"Check-mode guaranteed-error mirrors";
`eng/go/micron.go:2314`).

**Divergence:** lang-layer mirror emitters call raw
`r.Check.AddDiagnostic` without the stamp.
`lang/go/native/native_array.go:1712-1722` emits `fold_error` for the
statically-empty no-init fold — its own comment calls it "fold's own
GUARANTEED runtime error … with the byte-identical runtime message".
`lang/go/native/native_definition.go:1150-1180` emits `type_error`
for a failed typed-def unify whose non-check branch raises the
identical `fmt.Errorf`. Unstamped, these error-severity diagnostics
make the compile pass refuse where the finding's model is exact — the
divergence the RuntimeMirror classification exists to prevent. (The
deliberate, documented exception stays fine:
`case_exhaustive.go:650-669` bypasses the dedupe helper knowingly;
the warning-severity `unreachable_branch` emissions in
`native_control.go` are not mirrors.)

**Evidence:** `eng/go/check.go::AddDiagnostic` (no RuntimeMirror
stamping); `eng/go/micron.go:2314::CheckAddUniqueDiagnostic` (the
stamping helper); `lang/go/native/native_array.go:1712-1722`;
`lang/go/native/native_definition.go:1150-1180`.

**Documentation status:** the contract is documented
(eng/go/CLAUDE.md) but only as guidance; no gate checks that lang
emitters comply.

**Proposed verdict:** resolve by fix — route the lang mirror sites
through `CheckAddUniqueDiagnostic` (or stamp explicitly), then audit
the remaining direct `AddDiagnostic` callers in lang
(`native_array.go:1583`, `native_macro.go:277`,
`native_type_gen.go:349`, `native_patrun.go:197`,
`native_module_module.go:80`) for mirror-vs-model-undermining
classification, and consider extending a gate over lang emitters.

