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

> **Recording a non-uniformity is mandatory; a Pending record does NOT
> block the PR.** When a non-uniformity surfaces — in code review, in a
> design note, or during coding and debugging — it is recorded here
> immediately with status **Pending**. That recording is not optional and
> not subject to maintainer instruction (unlike an ADR entry); what
> requires the maintainer is the **Allowed** verdict — the same reviewed
> discipline as a `//covergate:allow` entry
> (`design/COVERAGE-ALLOWLIST.10.md`). A record is discharged by becoming
> **Resolved** (the divergence is removed) or **Allowed** (an explicit,
> argued acceptance), and it may stay Pending across many merges: the
> register's job is that a divergence is never lost or silently
> baselined, not that work stops until it is settled.

**Statuses:**

- **Pending** — not yet discharged. Either not yet argued to a verdict
  at all, or argued to a **verdict of "resolve by fix"** whose fix has
  not landed: a record directed at a fix stays Pending until the
  divergence is actually gone, because only Resolved and Allowed
  discharge it. Does not hold up a merge. Every Pending record must also
  appear in the open list below.
- **Allowed** — a deliberate divergence, kept. The record states the
  uniform rule, the divergence, the rationale, and the evidence that
  pins it (docs and tests), so the acceptance cannot silently rot.
- **Resolved** — the divergence was removed. The record is **deleted**
  and its number retired (see above), so a record only ever appears in
  this file as Pending or Allowed; `git log -S NURnnn` recovers a
  retired number's history.

> **Scope ruling (maintainer, 2026-09-17) — what this register records.**
> This register records **answer divergences only**: cases where two lanes
> disagree about a program's result. It does **not** record compile
> refusals. The two are different defects with different ledgers, and the
> distinction decides when a record is Resolved.
>
> So when a miscompile is fixed by making the shape REFUSE, the divergence
> is gone — the lanes agree — and the record is **Resolved** and deleted,
> even though the shape does not compile. That is not "resolving by
> refusing", and it does not soften the compilation contract: failure to
> compile is still a failure, and the refusal is still a defect owed a
> fix. It is tracked where compile defects belong — the refusal gates, the
> census, and the open-defect taxonomy in
> [`design/COMPILABLE-SUBSET.md`](design/COMPILABLE-SUBSET.md) §5 — not
> here. Reading a Resolved record as still-open because its shape refuses
> conflates the two ledgers and double-counts the same work.
>
> (Recorded after exactly that mistake: NUR149 was briefly reopened on the
> grounds that its shape still refuses. The reopening was withdrawn.)

> **Pruned 2026-09-29.** Every resolved record was deleted as the rules
> above require, and the pending entries were rewritten in compact form.
> A retired record's text is recovered with `git log -S NURnnn`; those
> resolved before 2026-09-27 also keep their full bodies in
> [design/NUR-ARCHIVE.0.md](design/NUR-ARCHIVE.0.md).

> **Spelling note (2026-08-19).** The modifier `/r` was renamed **`/v`**
> and the word `ref` became **`valof`** (see [ADR.md](ADR.md), ADR-011).
> Records dated before that day quote the old spellings verbatim, because
> that is what was observed and argued at the time; `/r` no longer parses,
> so read `/r` as `/v` and `ref` as `valof` when replaying their
> transcripts. The behaviour those records describe is unchanged by the
> rename — except where a record says otherwise.

---

## Pending non-uniformities (the open list)

The live list of records whose status is **Pending** — the standing
inventory of known, argued-or-unargued divergences. An entry leaves this
list only by becoming **Resolved** (the record is then deleted) or
**Allowed** in its record below — keep the two in sync in the same commit.

| # | Title | Surfaced by / provenance |
|---|-------|--------------------------|
| [NUR235](#nur235) | A typed-map param pattern over an inline map literal with a computed member: both lanes refuse the call, the `signature_error` notes differ | call-site specialisation's investigation (2026-09-27) |
| [NUR329](#nur329) | A no-match over a refused reach-led forward operand names the reach's value interpreted, the stack operand compiled (notes only) | the NUR328 sweep (2026-09-28) |
| [NUR330](#nur330) | A `def` inside a Rand.map-from / Rand.list-of generator body outlives the call on the interpreter only (silent) | the interp-entry census, module-rand.tsv (2026-09-27) |
| [NUR334](#nur334) | A read after a computed keep-defs body: the shapes the live-read deopt does not serve stay loud | main's 50 uncovered statements (2026-09-28); remainder after the live-read deopt (2026-09-29) |
| [NUR336](#nur336) | A paren apply over a member that is data at run time: the shapes the statement island does not serve stay loud | main's 50 uncovered statements (2026-09-28); remainder after the NUR336 pass (2026-09-29) |
| [NUR343](#nur343) | `each` over a single-count branch whose value may be a List or an Integer defers at run time (loud) | the NUR340 pass (2026-09-29) |
| [NUR344](#nur344) | A parked fn value is not applied to a paren or splice result that arrives after it (silent) | the NUR342/NUR337 pass (2026-09-29) |
| [NUR346](#nur346) | A leaf fn body that never mentions `args`, reached as a computed body, reads the real args compiled (silent) | the interp-entry census pass (2026-09-29) |
| [NUR347](#nur347) | Closure and lambda contract errors render a different name or caret compiled (loud; message only) | the interp-entry census pass (2026-09-29) |
| [NUR348](#nur348) | Two computed-body shapes the compiled runtime defers (loud) | the live-read deopt pass (2026-09-29) |
| [NUR349](#nur349) | A non-paren member apply followed by an infix word raises compiled (loud) | the NUR336 remainders pass (2026-09-29) |

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
(`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`). The single coercion rule this
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
investigate (`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):

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
(`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`). The operand-return semantics —
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
(the domain is infinite). DepScalar refinement construction does —
Bytes declares itself a refinement base since NUR009 closed
(`def Hi (Bytes gte (convert Bytes "m"))`) — and so does the
nominal-split route, `refine Bytes`.

### Why allowed

Vacuous rather than divergent: no kernel mechanism requires a scalar
family to have leaves, and nothing dispatches on their presence. There
is no useful structural split of Boolean — `True`/`False` subtypes would
duplicate what value-level machinery already provides uniformly (`case`
literal coverage per NUR002, and DepScalar refinements: `(Boolean gte
true)` *is* the true-only subset, since Boolean is one of the
refinement bases — Integer, Float, Number, String, Boolean, Atom and
Bytes each declare themselves one, `DeclareRefinementBase`). Users who want a
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

- `core/go/typetable.go::builtinDecls` — the Scalar branch layout.
- `core/go/depscalar.go` — Boolean declared a refinement base
  (`DeclareRefinementBase`, read by `canonicalBaseType`; `Boolean gte
  true` constructs).
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
`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

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

The Ideal half carries argued carve-outs, settled by the equality work
of the retired NUR031 (`git log -S NUR031` for its reasoning) and
recorded here now that it owns them. Two kinds have no second level to
offer, so their `eq` and `deq` coincide from opposite directions:

- **By VALUE, both** — a *value-like* Ideal with no handle behind it.
  `Error` (two independently raised errors with equal code, message and
  payload are `eq`) and `Word` (a name plus its `/`-modifiers), joined
  by the declared **type values** — a `class` and refinements of one, a
  disjunction/`enum`, a `fnsig`/`surface`, an uninstantiated `gen`
  schema — which are immutable declarations compared nominally.
- **By REFERENCE, both** — an opaque handle whose identity IS its value:
  `Timeout`/`Interval`, the `Module` descriptor (2026-08-02), and a
  sealed host `ExtensionPayload` (an `IO.open` file handle, a lock, a
  watcher, an mmap), which the kernel compares as a box and never reads
  into.

`Store` and `Function` are the two that DO have both levels, and they
take the rule as written: `eq` is reference identity — a Store's
`*StoreInstanceInfo`, a function's identity token — and `deq` is deep
value: a Store's own entry projection, a function's content as canon.

All of these are the rule applied, not departures from it — but the rule
as quoted above does not say so, and this record is where a reader looks
first.

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
  lead-in (added with this verdict); `design/legacy/LISP-ANALYSIS.5.ignore` (the original
  argument).
- `core/go/compare.go` — the carve-out arms themselves
  (`opaqueIdealExactEqual` / `opaqueIdealDeepEqual`, `storeDeepEqual`,
  `errorInfoEqual`, `hostPayloadIdentity`, `sameFnIdentity` /
  `fnStructurallyEqual`), and `core/go/compare_nur031_test.go`, which
  pins each of them.
- `lang/spec/compare-restrict.tsv` — the per-kind rows, including the
  code and type values; `lang/spec/module-array.tsv` — the collection
  words' `deq`-basis battery pins the value side of the rule.

**Modification recorded (maintainer, 2026-07-31,
`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):** the two-level model is to grow
into a complete equality family with a third word — **`req`**,
reference equality (pointer identity only, uniformly for compounds
and scalars) — separating three notions many languages conflate:
convenience equality (`eq`), deep structural equality (`deq`), and
reference identity (`req`). Performance note: Bytes `deq` may be
O(n); `req` gives a constant-time identity probe. Documentation
should compare the model with JavaScript, Python, Ruby, and the
Lisp family. The `req` design travelled with the equality work of the
retired NUR031 and is now unowned: it is a third WORD, not a
non-uniformity, so no record tracks it. This record's allowance is
unchanged.

---

## NUR013 — Two ordering regimes: a lawful total order and IEEE relationals {#nur013}

**Status:** Allowed · **Date:** 2026-08-02 (recorded Pending
2026-07-22; the 2026-07-31 investigation verdict discharged below;
verdict: maintainer, accepting the recommendation in
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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

- `core/go/core_make.go` — `registerKernelIdeals` (:815) is where the
  omission lives: it registers Ideals for Object, Resource, Record,
  Micron and Table and registers none for Store or Error, so
  `reg.Ideals.For`/`Match` return nil for those two and the target
  falls through to `MakeConvert` (:1066), whose default arm (:1112)
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
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

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
2026-07-22; verdict: maintainer, via `design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

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

**Status:** Allowed · **Date:** 2026-08-14 (the container gap was
RESOLVED BY FIX 2026-08-02; the surviving slot asymmetry is allowed —
see the verdict at the end) · **Recorded:** 2026-07-22 ·
**Surfaced by:** full-repo uniformity review

**Rule (as restated by the 2026-08-14 verdict):** the storage-column
words cover the same **keys** — a key that `set` can write, `del` can
remove. **Slots are out of scope**: a declared Class field and a List
index are positions, not keys, and the inverse of writing a position is
writing a different value, not removing the position.
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
`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):** bring `del` into symmetry with
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

### Verdict (maintainer, 2026-08-14): Allowed — slots are not keys

One asymmetry survives on purpose: **`set` can write a declared Class
field and `del` cannot remove it.** Under the rule as originally
worded that was still a divergence; the verdict is that the WORDING
was wrong, not the behaviour.

A class field is a **slot**, not a key. The inverse of writing a value
to a slot is writing a different value, not deleting the slot — an
instance missing a declared field would no longer satisfy its own
type. The same reading is what makes the List refusal correct (`set`
replaces at an index; removal shifts the tail, which is a different
operation), so the line is not a special case for Class: it is the
same line drawn twice. The rule at the top of this record is
therefore restated as "a **key** that `set` can write, `del` can
remove", with slots explicitly out of scope, and the record is
**Allowed**.

What remains true and is deliberately NOT closed here: the `get` hole
recorded in the investigation step above — an absent key and a
present-`none` key are indistinguishable through the reader — belongs
to the **sentinel-values programme**, which the 2026-07-31 verdict
opened as its own design line. That programme decides whether a
distinct miss sentinel exists; it does not reopen this record.

---

## NUR024 — Two orderings by design: semantic (`cmp`) and deterministic (`tcmp`) {#nur024}

**Status:** Allowed · **Date:** 2026-07-31 (recorded Pending
2026-07-22; verdict: maintainer, via `design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

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

> **Family note (2026-08-21).** `has` — historically on the quoting side
> with `set` — was moved to the evaluating side by maintainer direction:
> its key now evaluates exactly as `get`'s (QuoteArgs stripped from every
> `has` sig, core and the `boru:net` extension alike; pinned in
> `lang/spec/corpus-core.tsv` incl. the `ERROR:undefined_word` negative).
> The divergence this record accepts now covers `set` alone.

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
decision it then invalidates". `design/legacy/NUR-EFFORT-TRIAGE.0.ignore:139-148`
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

## NUR062 — Numeric marker letters are lowercase-only while every other letter in a literal is case-flexible {#nur062}

**Status:** Allowed · **Date:** 2026-08-14 · **Recorded:** 2026-08-11 ·
**Surfaced by:** the maintainer's decision on PR #339 ("only lowercase should
be valid for numeric syntax prefixes"); flagged for this register by the
PR #339 review (Codex P1)

**Rule:** one lexical convention per kind of thing. Letters inside a numeric
literal are either case-significant or they are not.

**Divergence:** they are now both. The four MARKER letters are lowercase-only
— `0x`, `0o`, `0b` and the big-number `0d`, so `0XFF` raises
`[boru/syntax_error]: numeric prefix must be lowercase: 0XFF` — while every
other letter a numeric literal can contain stays case-flexible:

```
0xff  ==  0xFF        hex DIGITS take either case
1e3   ==  1E3         the exponent marker takes either case
0XFF  ->  syntax_error    but the base marker does not
```

So `0XFF` is refused and `0xFF` accepted, yet `1E3` and `1e3` are equally
valid, and `0xAB` and `0xab` are the same value. A reader cannot derive one
from the other; each has to be learned.

**Scope, measured.** The rule governs numeric LITERALS only. A run in a NAME
position is not a literal and behaves identically in both cases — a quoted
atom (`0XFF/q`), a `/r` word reference (`0XFF/r` -> `word(0XFF)`), a
type-bound (`0XFF/t`), and a bare map key (`{0XFF: 1}`) are all names, never
numbers, exactly as their lowercase spellings are. The `0d` family is not a
DATA numeric in either case, so `0d12` and `0D12` both decode as lenient text
through `StructUtil.parse`. Both boundaries are pinned by rows rather than
left to prose: `parser/spec/parse.tsv` §"the lowercase-only rule governs
numeric LITERALS" and `parser/spec/data.tsv` §"the 0d big-number prefix is not
a DATA numeric".

**Evidence:** `parser/spec/parse.tsv` (8 refusal rows + 5 name-position rows),
`parser/spec/data.tsv` (4 refusal rows + 3 `0d` rows), `parser/spec/lex.tsv`
(2 token rows), each re-rendered independently by both port runners.
`REFERENCE.md` §"Numeric literals" states the rule and its scope.

**Documentation status:** stated in REFERENCE.md; both editor grammars
(tree-sitter, pygments) reject uppercase markers so highlighting cannot
advertise a literal the language refuses.

**Verdict (maintainer, 2026-08-14): Allowed** — as proposed. The asymmetry
is deliberate: a marker is a *spelling of syntax* while digits and exponents
are *content*. `0XFF` is a typo for `0xFF` far more often than it is anything
a user meant, whereas `0xAB` vs `0xab` and `1E3` vs `1e3` carry no such
signal, so refusing the first while accepting the others is a diagnostic, not
an inconsistency. The rule is already stated in REFERENCE.md §"Numeric
literals" with its scope, pinned by refusal and name-position rows in
`parser/spec/` that both port runners re-render independently, and both
editor grammars reject uppercase markers so highlighting cannot advertise a
literal the language refuses. No code or documentation change follows from
this verdict — the record closes as it stands.

---

## NUR070 — `if` reads a List condition as CODE while every other truthiness consumer coerces it {#nur070}

**Status:** Allowed · **Date:** 2026-08-15 · **Recorded:** 2026-08-14 ·
**Surfaced by:** implementing NUR053's fix (measuring whether the three
consumers really share a domain once `convert Boolean`'s slot was
widened)

**Verdict (maintainer, 2026-08-15): Allowed — a List in a condition
position is CODE.** That is what a concatenative language should mean by
a bracketed body there, and the One Truthiness Model governs *values*,
not code positions: `if [ … ]` running its condition is the language's
way of spelling a computed condition, not an accident to be coerced
away. The two readings genuinely differ, and the code reading is the
intended one.

What the allowance costs, stated so it cannot rot: `design/TRUTHINESS.0.md`
§2 must say the model has one shape-shaped hole — a List reaching `if`
is executed, so the "every consumer agrees" claim holds for Map, None,
String and the numeric leaves and NOT for List — and `if xs` where `xs`
holds a list stays a sharp edge for anyone who expected presence
coercion. The §2 amendment landed with NUR053's fix and already states
both the domain and this split, with the measured opposite-answer table.
The spec rows in `lang/spec/edge-scalars-3.tsv` pin it in both
directions, so the accepted behaviour is executable rather than merely
described.

**Rule:** one truthiness model, applied by every construct that coerces
a value to a Boolean — `design/TRUTHINESS.0.md`, the One Truthiness
Model. NUR053's premise, and the sentence this record corrects, was that
"`if` and `make Boolean` accept any value".

**Divergence:** they do not agree on a **List**. `make Boolean` and
`convert Boolean` coerce a list by PRESENCE (non-empty → true); `if`
does not coerce it at all — it runs it as a **code body** and takes the
truthiness of what the body leaves. The two readings give opposite
answers on the same bound value, and the disagreement is not confined to
an edge case:

```
def xs [0]   if xs ['T'] ['F']        # → 'F'     the body runs, yields 0, falsy
def xs [0]   convert Boolean xs       # → true    non-empty list is present
def xs [0]   make Boolean xs          # → true

def xs []    if xs ['T'] ['F']        # → [boru/runtime_error]: if: condition
                                      #   produced no value
def xs []    convert Boolean xs       # → false
def xs []    make Boolean xs          # → false
```

Every other source shape agrees. Measured across the three consumers:
Map (`{}` → false, `{a:1}` → true), `none` → false, empty String →
false, and the numeric leaves including the Big ones (NUR055's rows)
all give the same answer through `if`, `convert Boolean` and
`make Boolean`. The List is the sole shape where the consumers split.

**Why it is not simply a defect in `if`:** the code-body condition is a
deliberate, documented form — `if [ … ] [then] [else]` runs its
condition, which is how a computed condition is spelled, and the
compiled path models it (the "if code-body condition" row in
`lang/go/context_boundary_differential_test.go`). The non-uniformity is
not that the form exists; it is that **the same value gets two different
readings depending on how it reaches `if`**, with nothing at the call
site to distinguish them: a literal `[ … ]` is unambiguously code, but a
bound name holding a List is read as code too, where the value reading
is at least as plausible.

**Evidence:** `lang/spec/edge-scalars-3.tsv` (the NUR053 domain block
now pins both sides — the Map/None agreement rows and the three List
rows showing the split); `core/go/core_helpers.go` `CoerceBoolean` (the
presence rule the two constructors share); `design/TRUTHINESS.0.md` §2
(amended by NUR053's fix to state the domain, and to name this split).

**Documentation status:** newly documented by this record and the
TRUTHINESS.0.md §2 amendment; before them, nothing stated that `if`'s
domain differs from the constructors' for one shape, and NUR053's own
text asserted the opposite.

**Proposed verdict:** argue or fix, and the options are genuinely
balanced.

- **Allowed** — a List in a condition position is code, full stop; that
  is what a concatenative language should mean by it, and the truthiness
  model governs values, not code positions. Cost: `design/TRUTHINESS.0.md`
  must say the model has one shape-shaped hole, and the `if xs` case
  stays a trap for anyone holding a list in a variable.
- **Fix by distinguishing the spellings** — a LITERAL list condition
  stays code (unchanged), while a condition that arrives as an already-
  evaluated VALUE coerces. This is the reading that makes `if xs` agree
  with both constructors, and it is what a user who wrote `def xs []`
  almost certainly meant. Cost: the two spellings stop being
  interchangeable, and the distinction has to survive the compiled path
  as well as the interpreter.
- **Fix by widening the error** — keep the code-body reading but make
  the empty case a diagnostic that names the ambiguity rather than the
  bare "condition produced no value".

This record does NOT block NUR053, which is resolved: the constructor
pair now shares a domain exactly, and this is the residue that pairing
them revealed.

---

## NUR235 — a typed-map param pattern rejects an inline map literal with a computed member, and the lanes word the error differently {#nur235}

**Status:** Pending. Recorded 2026-09-27 in the investigation that led to
call-site specialisation; present on `main` at c8bce66.

```
def h fn [[m:{f:Integer}][Any][m.f]] end  h {f: (1 add 1)}
  interpreted   signature_error: cannot call `h` … the argument was {f:paren([…])} (a Map)
                … candidate `h (Map)` — argument 1: … does not satisfy its declared pattern {f:Integer}
  compiled      signature_error: cannot call `h` … the argument was {f:2} (a Map)
                … expected: h (Map) or (no args)
```

Both lanes refuse the call — the error CODE agrees — but the notes differ:
the interpreter matches the pattern against the literal before its paren
members are evaluated, and the compiled lane reports the evaluated map and
no candidate. The same holds for `{:Integer}` and for a lambda member
(`h {f: ([x:Integer] => [x add 1])}` against `{f:Function}`); a map bound
first (`def mm {f: …}  h mm`) matches on both lanes. Whether the
interpreter's refusal is itself the language defect (a typed-map pattern
unusable with an inline literal of computed members) is a question for the
maintainer, not recorded here as settled.

---

## NUR329 — a no-match over a refused reach names the stack operand compiled {#nur329}

**Status:** PENDING, notes only (the code, message and caret agree) ·
**Recorded:** 2026-09-28 · **Surfaced by:** the NUR328 sweep (eight rows).

```
def m {a: 1} end def f fn [[x:Type][Any][[x]]] end 7 f m.a
  interpreted   … = note: the argument was 1 (an Integer)
  compiled      … = note: the argument was 7 (an Integer)
```

The interpreter's report walks the written operands at its failure point,
where the reach's paren has already run, so it names the reach's value
(1). The check pass's walk (`rematchWrittenSplit`) stops at the reach's
paren, so the recorded tuple is the stack prefix (7). A fix records the
reach's result as a written operand, which the VM's NUR311 stop rule then
cuts where the run's value is no concrete value. The same shape gives
`x:None`, `x:[:Maybe]` and `x:N` over a refused reach or a refused `none`.

---

## NUR330 — a def inside a rand generator body outlives the call on the interpreter only {#nur330}

**Status:** Pending. Recorded 2026-09-27 (the interp-entry census's
module-rand.tsv row); pre-existing.

`Rand.map-from` and `Rand.list-of` run each generator body on the shared
registry with no def cleanup — the token-body contract every code-body word
shares (a `do` body's `def` survives the `do` alike, NUR202) — so a `def`
inside a body rebinds the name for the rest of the program. The compiled
program does not see it: the recorder treats the rand words' body operands
as opaque, so a later read of the name is the binding it recorded before
the call.

```
import "boru:rand"  def k 5 Rand.map-from {b:[def k 1 k]} k
  interpreter   [{b:1} 1]
  compiled      [{b:1} 5]
import "boru:rand"  def k 5 Rand.list-of [def k 1 k] 1 k
  interpreter   [[1] 1]
  compiled      [[1] 5]
```

`do [def k 1] k` is `[1]` on both lanes: its body is joined into the model
(the do$body unit's BIND_DYN_SCOPE and the dyn read after it). The cure is
the same for the rand words — join a body that defines a name into the
model, or decline to compile one — and moving map-from's bodies onto the
InvokeBody seam (2026-09-27) neither causes nor cures it: the old pooled
run diverged identically.

---

## NUR334 — a read after a computed keep-defs body: the loud remainder {#nur334}

**Status:** Pending (loud) · **Recorded:** 2026-09-28 · **Narrowed:** 2026-09-29

A read seated live after a computed keep-defs body (`do (mk) end x`, where
`mk` returns a quoted body that defs `x`) is a deopt point that tests the
name's registry binding and hands the statement to the interpreter when the
compiled statement cannot take it (compiler `kept_live_deopt.go`, eng
`liveDeopt`). Three shapes place no point and keep the lookup's loud defer
(internal_error compiled, the interpreter answers):

- a unit returning a `/v`-read splice to its caller ("tape-coupled deopt
  result") — the caller's splice of the returned value has no compiled seat;
- a read whose unit cannot make its island names registry-visible (a `def`
  inside a later `for` body);
- a read inside a spliced word's body (`def w word [do (mk) end x] w`).

Pinned by lang `TestLiveDeoptSpliceReturnedStaysLoud`,
`TestLiveDeoptUnservedStaysLoud`.

---

## NUR336 — a paren apply over a data member: the loud remainder {#nur336}

**Status:** Pending (loud) · **Recorded:** 2026-09-28 · **Narrowed:** 2026-09-29

A shaped paren apply `(m.f …)` whose member is data at run time takes its
statement's island, which re-steps the statement as the interpreter does
(eng `parenMissesWindow` / `namedMissRaise`, compiler `landing_restart.go`).
These shapes still end in a loud compiled defer where the interpreter
answers:

- a statement opening with a def of a computed value followed directly by
  the paren, `def k (3 dup) (m.f 7)`, and its fn-body twin (a paren right
  after the def is expanded to markers before the engine reaches it, so no
  stack is recorded there);
- a lead that went through a landing (`W (m.f y) 9` over a lambda of two
  Integers) — the VM cannot tell whether the landing fired;
- an island that dispatches a VM-bound lambda by name again,
  `k (g 7) (g 8) drop` over `def g m.f/v` (`BIND_GLOBAL` keeps the raw value
  where the interpreter's `def` compiles its signatures).

Pinned in lang `nur335_336_statement_island_test.go`.

---

## NUR343 — a List-or-Integer branch at `each` defers at run time {#nur343}

**Status:** Pending (loud) · **Recorded:** 2026-09-29

```
def c true end each (if c [[1]] [3]) [2]
  interpreted   [[1]]
  compiled      internal_error: DISPATCH_REMATCH at each matched at run time
                where the static model failed (the vm:rematch-matched defer)
```

Also `(if c [[1]] [3]) each [2]` and trapping bodies. The check pass decides
`each` cannot match a branch result that may be a List or an Integer and
records a re-match with nothing to run on a match; the run matches. Both
arms leave one value, so NUR340's variable-count region path is not
involved.

---

## NUR344 — a parked fn value is not applied to a paren or splice result {#nur344}

**Status:** Pending (a silent wrong answer) · **Recorded:** 2026-09-29

A fn value parked by a paren apply whose window did not fit is applied by
the interpreter to a paren or splice result that arrives after it; the
compiled lane leaves it unapplied.

```
def lam ([x:Integer] => [x add 100]) end ("s" lam/v) (2 add 3)
  interpreted   [s 105]
  compiled      [s fn lam(Integer) 5]
```

Likewise `({a:1} lam/v) (5)` and `def w word [5] end ("s" lam/v) w`; the
literal twin `("s" lam/v) 5` agrees. A loud neighbour:
`({a:1} lam/v) do [lam/v]` raises a compiled internal_error (the NUR286
defer) where the interpreter answers `[{a:1} fn lam(Integer) fn lam(Integer)]`.

---

## NUR346 — a leaf body's `args` reads the real args compiled {#nur346}

**Status:** Pending (a silent wrong answer) · **Recorded:** 2026-09-29

```
def mk fn [[] [List] [quote [args]]] end
def w fn [[x:Integer] [Any] [each (mk) [x 5]]] end w 7
  interpreted   [[] []]
  compiled      [[7] [7]]
```

The interpreter's leaf path pushes an empty args list for a body that never
mentions `args`; the VM always pushes the real args in DynEnv programs.

---

## NUR347 — closure and lambda contract errors: name and position {#nur347}

**Status:** Pending (loud; the message's name or caret differs) · **Recorded:** 2026-09-29

- `def m {f: ([x:Integer] => [x x])} each m.f [1]` and `… fold m.f [1] 0`:
  "source position unknown" compiled, 1:40 interpreted.
- `each (fn [[x:Integer][Integer][x x]]) [1]`: the contract error names ``
  compiled, `<fn>` interpreted.
- `def mk fn [[k:Integer] [Function] [(fn [[a:Integer][String] [a add k]])]] end ((mk 1) 5)`:
  `` at 1:36 compiled, `<fn>` at 1:37 interpreted; its lambda twin
  `([a:Integer] => [a k])` reports 1:36 compiled, "source position unknown"
  interpreted.

eng `applyFrameName` (2026-09-29) settled the frame and token-seam paths;
the closure paths above still differ.

---

## NUR348 — two computed-body shapes the compiled runtime defers {#nur348}

**Status:** Pending (loud) · **Recorded:** 2026-09-29

```
def x 0 end def mk fn [[][List][quote [def x [1 2]]]] end do (mk) end x.0
  interpreted   [1]
  compiled      internal_error: do over a computed body left 0 value(s)
                where a single-value seat consumes it
def w word [1 2] end do [w/v]
  interpreted   [1 2]
  compiled      internal_error: tape-coupled handler result at do
```

---

## NUR349 — a non-paren member apply before an infix word raises compiled {#nur349}

**Status:** Pending (loud) · **Recorded:** 2026-09-29

```
def mk fn [[] [Map] [{f: ([x:Integer] => [x add 1])}]] end def m (mk) end 1 m.f 7 add
  interpreted   [9]
  compiled      signature_error: cannot call `add` (4 arguments)
```

Also `3 1 m.f 7 add add` (`[12]` interpreted).

---
