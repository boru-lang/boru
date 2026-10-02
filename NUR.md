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
| [NUR334](#nur334) | A read after a computed keep-defs body: a returned `/v` splice at a tail call or behind a word, and a spliced word read twice or nested, stay loud | main's 50 uncovered statements (2026-09-28); narrowed four times |
| [NUR336](#nur336) | A paren apply over a data member: loop continuations beyond a counted loop's first statement, a word collecting the paren forward, a type value before the stop, stay loud | main's 50 uncovered statements (2026-09-28); narrowed four times |
| [NUR356](#nur356) | A native that raises by value inside a literal handed to a call matched at run time raises before the no-match; a computed arm's pending literal defers | the NUR351/352 pass (2026-09-29); narrowed twice |
| [NUR359](#nur359) | A computed run re-stepping an argument-taking fn value defers compiled (loud) | the NUR355 pass (2026-09-29); narrowed 2026-09-30 |
| [NUR361](#nur361) | A gradual read nested in an inline body that holds a fn defers compiled (loud; was silent) | the NUR356 pass (2026-09-29); narrowed 2026-09-30 |
| [NUR364](#nur364) | A failed call over a pending literal in a fn body renders a wider operand window compiled (notes only) | the NUR235 pass (2026-09-30) |
| [NUR366](#nur366) | A stored fn whose stamp declined and whose result count breaks its declared returns raises internal_error compiled; the interpreter raises the return contract's type_error | downstream: the voxgig-boru/decision migration (2026-10-02) |
| [NUR367](#nur367) | A local rebound in a loop body and read bare in an arm after the loop, holding a fn: compiled returns the fn uncalled (`for`/`while`, silent) or raises (`each`/`for-each`); the interpreter calls it | downstream: the voxgig-boru/decision migration (2026-10-02) |
| [NUR368](#nur368) | A fn value obtained at run time and held in a local, applied at the main program: `print (41 f)` prints the argument and the call lands on the next value (silent); a 0-arg one bound through a paren reads back undefined | downstream: the voxgig-boru/decision migration (2026-10-02) |
| [NUR369](#nur369) | A fn value passed as a param and called from an `each` callback, when the callee re-enters the same fn: the outer loop applies the inner call's fn compiled (silent) | downstream: the voxgig-boru/template migration (2026-10-02) |
| [NUR370](#nur370) | A fn param called on a def made in the same `each`/`var` body from a gradual read raises internal_error DISPATCH_GENERIC compiled | downstream: the voxgig-boru/template migration (2026-10-02) |
| [NUR371](#nur371) | A def in an `if` arm of an imported fn's fold body clobbers a same-named local of the caller compiled (`undefined_word`) | downstream: the voxgig-boru/template migration (2026-10-02) |
| [NUR372](#nur372) | A module fn whose fold body reads its param raises `undefined_word` on its 5th–8th compile in one process compiled (library-scale repro) | downstream: the voxgig-boru/template migration (2026-10-02) |
| [NUR373](#nur373) | A fn whose result is an ungrouped `r.list-of` over its random-source param draws once and repeats the value compiled (silent); the `[List]`-declared twin is refused by a checker false positive | downstream: the voxgig-boru/aless migration (2026-10-02) |
| [NUR374](#nur374) | A `Test.check-prop` generator whose body is a grouped `r.list-of` raises `undefined word: r` compiled (the element body does not see the generator's `r`); the check pass refuses a var-bound `r` there | downstream: the voxgig-boru/aless migration (2026-10-02) |
| [NUR375](#nur375) | A callback that binds an `r.int` draw with `def` and leaves the bindings raises internal_error "a landed fn value takes arguments (NUR298)" compiled; in a `check-prop` generator the property fails | downstream: the voxgig-boru property-generator rewrites (2026-10-02) |
| [NUR376](#nur376) | A fn-local name read inside a `do {k: [expr]}` map value leaves a later same-named `var`/`def` binding undefined compiled (`undefined_word`), even in an unrelated fn | downstream: the voxgig-boru/stats migration (2026-10-02) |
| [NUR377](#nur377) | `boru:test` mints its record types from a fresh ID counter: a user type made after `import "boru:test"` fails its own return contract (`expected Box, got Box`) and `is` answers false compiled (silent) | downstream: the voxgig-boru/bloom-filter and stats migrations (2026-10-02) |

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

**Status:** Allowed · **Verdict:** maintainer, 2026-07-22

**Rule:** the six arithmetic words (`add`/`sub`/`mul`/`div`/`mod`/`pow`)
are total within every scalar type and every Micron kind (REFERENCE.md
§"Within-type operations").

**Divergence:** `Boolean` is the one scalar family excluded, for all six
ops: `add true false` raises `[boru/type_error]: add: arithmetic is not
defined on Boolean`.

**Why allowed:** Boolean carries the logical words (`and`/`or`/`xor`/
`not`) instead; every candidate arithmetic (C-style promotion, GF(2)) is
arbitrary, and would be silently accepted where a loud error teaches
the logical vocabulary. The refusal is a **registered** `[Boolean
Boolean]` signature with a pinned message (the `setMicron` precedent),
not an opaque dispatch miss; check mode mirrors it; and the signatures
are CoreDefault, so `refine Boolean` can still extend a word by
specificity.

**Evidence:** `lang/go/native/native_scalar_ops.go` —
`booleanArithHandler` / `booleanArithError` / `booleanArithReturns` and
the six erroring signatures. REFERENCE.md §"Within-type operations"
("**`Boolean`** arithmetic is a **defined error**").
`lang/spec/scalar-micron-ops.tsv` (all six pinned as errors);
`lang/spec/open-words.tsv` (the refine escape and its negative twins).

---

## NUR001 — `convert Boolean` coerces by presence, not content {#nur001}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-22; re-affirmed
2026-07-31 (`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

**Rule:** `convert <ScalarType> <String>` parses the string's content:
`convert Integer "42"` → `42`, `convert Float "1.5"` → `1.5`.

**Divergence:** `convert Boolean "false"` → `true`. Boolean conversion
applies the truthiness rule — `false`, numeric zero in any leaf
(`0`/`0.0`/`0d0`) and `""` are false — and never inspects a String's
characters.

**Why allowed:** `convert Boolean` shares **one** coercion rule
(presence) with `if`-condition truthiness and `make Boolean`, specified
in `design/TRUTHINESS.0.md`; parsing content would fork truthiness into
two rules, a worse non-uniformity. Content parsing is an opt-in: `convert Boolean {truthy: true}` parses the YAML
tokens (`y`/`yes`/`true`/`on`, `n`/`no`/`false`/`off`,
case-insensitive), falls back to presence for anything else, and is
inert for non-Boolean targets.

**Evidence:** `lang/go/native/native_type.go` — `coerceBooleanTruthy`
and the `truthy` option plumbing. REFERENCE.md ("**`convert Boolean` is
presence coercion; `{truthy: true}` opts into YAML parsing**", and §`if`:
"the exact same rule as `convert Boolean` and `make Boolean`").
`lang/go/native/native_type_convert_seam9_test.go` (both modes, positive
and negative); `lang/go/native/integration_coverage_test.go` (`'false'
convert Boolean` → `true`).

---

## NUR002 — Value enumeration exhausts finite domains; Boolean is the built-in instance {#nur002}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-22; rewritten
2026-07-31 to state the general rule rather than a Boolean special case

**Rule:** exhaustive coverage of any **finite** domain needs no default
branch. For a scalar scrutinee a default-less `case` proves
exhaustiveness through type clauses, `[is T]` predicates,
comparison-predicate / refinement interval unions, or — when the domain
is finite — by enumerating its values. An infinite scalar can never be
covered by enumeration (`case n [1 … 2 …]` cannot cover `Integer`).

**Divergence (apparent):** Boolean is covered by its values: `case b
[true 1 false 0]` is statically exhaustive with no default, and `case b
[true 1]` is a `case_not_exhaustive` check error (`uncovered: false`).

**Why allowed:** the rule, not a special case: Boolean is the built-in
two-value pseudo-enum. Enumeration coverage follows from cardinality (a
sound proof) through the channel enum members use (`def Color (red/q tor
…)`). Enums specialise disjunct types, so the principle is **finite
disjunct exhaustiveness** — documentation should present Boolean that
way. Unscheduled follow-on (`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`):
finite dependent scalar types join the same channel, declarable by
enumeration (`{2,3,4}`-style) and represented symbolically.

**Evidence:** REFERENCE.md §"`case` — dispatch and exhaustiveness"
("**Boolean, by `true` and `false`**", with the negative example).
`lang/spec/case.tsv` §6 (true+false cover Boolean, beside the union and
enum coverage rows that share the mechanism).

---

## NUR003 — `and`/`or` select an operand; the rest of the boolean family returns strict Boolean {#nur003}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-22; re-affirmed
2026-07-31 (`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

**Rule:** the boolean family returns strict `Boolean`: `not`, `xor`,
`any`, `all` and the `boru:logic-util` gates (`nand`/`nor`/`xnor`/`iff`/
`implies`) coerce by truthiness and yield `true` or `false`.

**Divergence:** `and` and `or` are value-selecting short-circuit
connectives that return whichever operand decided the result: `1 and 2`
→ `2`, `false 5 and` → `false`, `0 9 or` → `9`.

**Why allowed:** deliberate Lisp/Python semantics — the operand form
composes directly (`x or default`, with `otherwise` as the None-aware
variant), and a strict Boolean is one `not not` or comparison away. It
is documented at the word table, and check mode types the result
precisely (`foldOrJoin` folds statically decided selections, otherwise
the join of the operand types), so static analysis never degrades to
`Any`.

**Evidence:** `lang/go/native/native_boolean.go` — `andHandler` /
`orHandler` (operand return) vs `notHandler` / `boolBinaryNative` /
`anyHandler` / `allHandler` (strict), and `foldOrJoin`. REFERENCE.md
§Boolean ("**`and` / `or` return an operand, not a coerced boolean.**").
`design/TRUTHINESS.0.md` §"The connectives" — short-circuiting,
evaluation order, which operand returns, static typing.

---

## NUR004 — Boolean, Atom and Bytes have no lattice subtypes {#nur004}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-22; clarified
2026-07-31

**Rule:** the scalar branch families carry structural leaves: `String`
has `EmptyString`/`ProperString`, `Number` has `Integer`/`Float`/
`BigInteger`/`BigDecimal`, `Micron` its twelve kinds.

**Divergence:** `Scalar/Boolean`, `Scalar/Atom` and `Scalar/Bytes` are
leaf-less children of `Scalar` (no `True`/`False` nodes); Bytes is
registered from the language layer rather than in `builtinDecls`.

**Why allowed:** vacuous — nothing requires or dispatches on leaves.
The lattice holds **structural** leaves; `true`/`false` are
**value-level** members of a finite domain (NUR002), so `True`/`False`
leaves would be in the wrong layer. Value-level machinery covers what
subtypes would: `case` literal
coverage (not for Bytes — its domain is infinite), DepScalar refinements
(Integer, Float, Number, String, Boolean, Atom and Bytes each declare
themselves refinement bases, so `(Boolean gte true)` is the true-only
subset and `def Hi (Bytes gte (convert Bytes "m"))` constructs), and a
nominal split minted with `refine Boolean` / `refine Bytes`, which
dispatches by specificity.

**Evidence:** `core/go/typetable.go::builtinDecls` (the Scalar branch
layout). `core/go/depscalar.go` (`DeclareRefinementBase`, read by
`canonicalBaseType`). `lang/spec/case.tsv:75` (true+false cover
Boolean); `lang/spec/edge-types-1.tsv:82-85` /
`lang/spec/open-words.tsv:26-29` (`refine Boolean` dispatches).
`lang/go/native/native_bytes.go:23` (the `Scalar/Bytes` registration).

---

## NUR005 — String `add` is the sole cross-type exception to same-type arithmetic {#nur005}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-31
(`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

**Rule:** scalar arithmetic is same-type — the six words are "applied
within a type, never across it" (REFERENCE.md §"Within-type
operations"). A cross-type pair has no signature and raises
`[boru/signature_error]`; a registered refusal raises a coded error
(`type_error` for `Big`⊕`Float`, Boolean arithmetic, cross-kind Microns,
`mul` on two Qions; `arith_error` for a Qion currency mismatch).

**Divergence:** `add` carries `[String Scalar]` / `[Scalar String]`
overloads that stringify the other operand (`add "x" 5` → `'5x'`), while
Atom `add` is `[Atom Atom]`-only and Bytes `add` `[Bytes Bytes]`-only.

**Why allowed:** concatenation-with-coercion is the commonest string
operation, and the coercion is total and canonical. The overloads need at least one String operand,
so they never concatenate two non-Strings: `add true 1` is a dispatch
miss (`signature_error`), not NUR000's registered `type_error`, while
two non-Strings of the same type keep their within-type arm (`add 1 2` →
3, `add a/q b/q` → `'ba'`). The String/Atom/Bytes occurrence-package
parallel is documentary, not an architectural grouping; an Atom is a
name and Bytes are raw octets, so stringifying would manufacture bugs.

**Evidence:** REFERENCE.md §"Within-type operations" states the
exception at the rule ("The **sole language-level exception** is
`String` `add` …"). `lang/go/native/native_math.go`
— the `[TString TScalar]` / `[TScalar TString]` overloads and the
"string-or-bust" comment; `native_scalar_ops.go` / `native_bytes.go` —
the within-type Atom and Bytes signatures. `lang/spec/arithmetic.tsv`
§3 — the concat battery with the `add true 1` / `add true false`
negatives.

---

## NUR011 — `eq` is identity for compounds, value for scalars {#nur011}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-23; `req`
follow-on recorded 2026-07-31

**Rule (as the verdict states it):** for Scalars, `eq` and `deq` are the
same and based on values; for Nodes and Ideals, `eq` is by reference and
`deq` by value.

**Divergence:** one word, two equality principles — `eq` compares
scalars by value but lists/maps/XML/instances by container identity
(`["a"] eq ["a"]` → false), so on compounds it disagrees with
`cmp`-equality.

**Why allowed:** identity ("the same container?", cheap and
aliasing-aware) and deep equality ("the same values?") are Scheme's
`eq?`/`equal?`, collapsed to two levels because they coincide on
scalars. Value-oriented words key on `deq`; `eq` is the aliasing probe.
Ideal kinds with no second level have `eq` = `deq`:
by value for handle-less `Error` (equal code, message, payload), `Word`
and the declared type values (`class` and refinements, `enum`/
disjunction, `fnsig`/`surface`, uninstantiated `gen`), compared
nominally; by reference for opaque handles (`Timeout`/`Interval`, the
`Module` descriptor, a sealed host `ExtensionPayload`).

`Store` and `Function` have both levels and take the rule as written
(`eq`: the `*StoreInstanceInfo` / identity token; `deq`: entries /
content as canon). A third word `req` (reference identity for all
values, constant-time where Bytes `deq` is O(n)) is an unowned
follow-on, not a non-uniformity.

**Evidence:** `eng/go/compare.go` — `ExactEqual` (scalar arm shared with
`DeepEqual` via `scalarFamilyEqual`, so the two cannot drift on a
scalar; `sameContainer` for compounds). `core/go/compare.go` — the
carve-out arms (`opaqueIdealExactEqual` / `opaqueIdealDeepEqual`,
`storeDeepEqual`, `errorInfoEqual`, `hostPayloadIdentity`,
`sameFnIdentity` / `fnStructurallyEqual`), each pinned by
`core/go/compare_nur031_test.go`. REFERENCE.md §Comparison ("**`eq` is
identity for compounds; `deq` is structural — by design**");
EXPLANATION.md §"Type ordering" ("**Two equalities, one rule.**");
`design/legacy/LISP-ANALYSIS.5.ignore`. `lang/spec/compare-restrict.tsv`
(per-kind rows); `lang/spec/module-array.tsv` (the collection words'
`deq` basis).

---

## NUR013 — Two ordering regimes: a lawful total order and IEEE relationals {#nur013}

**Status:** Allowed · **Verdict:** maintainer, 2026-08-02 (accepting
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

**Rule:** one ordering answer per value pair within one word family.

**Divergence:** `cmp`/`tcmp`/`sort` give NaN a slot (greatest; two NaNs
tie) while `lt`/`lte`/`gt`/`gte` apply IEEE unordered (always false):
`nan eq nan` → false but `nan cmp nan` → 0. Signed zeros mirror it:
`-0.0 cmp 0.0` → -1 while `-0.0 eq 0.0` → true and `-0.0 lt 0.0` →
false.

**Why allowed:** NUR024's semantic/deterministic split. The relationals
answer a mathematical question and obey IEEE-754 (§5.11: NaN false, ±0
equal). The total order must be total and antisymmetric or `sort` is not
a function — exactly IEEE §5.10 `totalOrder`. boru conforms for its one
quiet NaN (a single `nan` literal, sign unobservable — `nan -1.0 mul`
renders `nan` — no payload): above `+inf`, tying with itself. Ordering
by NaN sign and payload is vacuous, since no boru program can produce
them. The total
order slots −0 before +0 (`sort [0.0 -0.0]` → `[-0.0 0.0]`), with
Integer `0` and `0d0` at +0 so the cross-leaf triangle stays transitive;
the relationals' matching signed-zero carve-out is part of this
acceptance.

**Evidence:** `eng/go/compare_scalar_behaviors.go` (the NaN slot and
the Signbit tiebreak). `eng/go/compare.go` (the relational
unordered/signed-zero guards). `eng/go/compare_nan_test.go`,
`eng/go/compare_zero_test.go` (both regimes, positive and negative).
`lang/spec/float-special.tsv` (signed-zero and NaN sections),
`lang/spec/edge-scalars-2.tsv` (cmp/sort rows).
`design/IEEE-754-COMPLIANCE.8.md` §5.10; `design/TYPE-ORDERING.10.md`
§"NaN in the total order".

---

## NUR014 — Cross-leaf numeric magnitude equality depends on the leaf pair AND the value {#nur014}

**Status:** Allowed · **Verdict:** maintainer, 2026-08-02 (accepting
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

**Rule:** leaves of one family compare by magnitude: `1 cmp 1.0` → 0,
`1 eq 1.0` → true.

**Divergence:** whether the collapse holds depends on the leaf pair and
the value. Within one pair: Float↔BigDecimal collapses for dyadic
magnitudes (`0d0.5 eq 0.5` → true) and fails otherwise (`0.1 eq 0d0.1` →
false, an exact big.Rat compare). At one value: `9007199254740993 eq
9007199254740992.0` → true (Integer↔Float via float64) but
`0d9007199254740993 eq 9007199254740992.0` → false (BigInteger↔Float
exact).

**Why allowed:** it is honest — the Float `0.1` is the binary64 value
0.1000000000000000055511151231257827…, not one-tenth — and every
collapse that can hold exactly does (`1 eq 0d1`, `0d0.5 eq 0.5`).
Rounding BigDecimal through float64 would equate distinct values,
defeating BigDecimal and the exactness design that makes Big⊕Float
arithmetic an error.
Python agrees (`Decimal('0.1') == 0.1` → False).

**Evidence:** `eng/go/compare_scalar_behaviors.go` —
`numberCompareBehavior.Compare` and `toRatExact` (comments cite the
Python precedent). REFERENCE.md:195-200 (the user-facing statement).
`lang/spec/bignum.tsv:47-63` (collapses that hold and `0.1 eq 0d0.1` →
false); `lang/spec/edge-scalars-1.tsv:24-25` (both `cmp` directions).

---

## NUR018 — Store and Error are excluded from `make` {#nur018}

**Status:** Allowed · **Verdict:** maintainer, 2026-08-02 (accepting
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

**Rule:** `make` instantiates the structural type-kinds; the kernel guide
groups Record, Options, Table, Class, Store, Error and the Micron family
as the structural set (eng/go/CLAUDE.md §"Where a Type Lives" rule 4 —
now clarified as a statement of kernel residence, not of
`make`-constructibility).

**Divergence:** `make Store {}` and `make Error {message:"x"}` raise
`[boru/unsupported]: make: unsupported target type`; Store and Error
construct only through their dedicated words.

**Why allowed:** `make` instantiates **schema-bearing** kinds against
their declared shape. Store and Error have no schema, and constructors
a bare `make` cannot honour: a Store IS its position in the context
machinery (`StoreInstanceInfo`'s parent chain and COW layers,
established by `eng/go/registry.go`'s context words — a detached `make
Store {}` would have to invent its parent), and an Error's identity is
its passage through `raise`/`trap` (`describe raise`: "construct an
Ideal/Error"), which stamps code and context. The exclusion is a loud
coded error.

**Evidence:** `core/go/core_make.go` — `registerKernelIdeals` (:815)
registers no Ideal for Store or Error, so the target falls through to
`MakeConvert` (:1066), whose default arm (:1112) raises; `isTypeLike`
(:31) is not the gate. `eng/spec/make.tsv` (both exclusions pinned as
ERROR). eng/go/CLAUDE.md rule 4 (the clarification). REFERENCE.md
(`make` states the exclusion and names the dedicated constructors).

---

## NUR019 — `slice` is a core sequence word, not a String straggler {#nur019}

**Status:** Allowed · **Verdict:** maintainer, 2026-08-02 (accepting
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore`)

**Rule:** the string vocabulary moved to `boru:string-util`; moved words
are not available unqualified (lang/go/CLAUDE.md §"Package layout").

**Divergence:** `slice` stays core and unqualified, and `boru describe`
files it under `list`, not `string`.

**Why allowed:** `slice` is not a String-family word but a core
**sequence** word — nine unqualified signatures over String, List and
Bytes, kin of `size`/`take`/`reverse`, which stayed core for the same
reason; moving it would split one polymorphic word. The misleading
filings (REFERENCE's string table, the describe category text) were
fixed with the verdict; `list` stands as the sequence home.

**Evidence:** `lang/go/native/natives.go:372-385` and
`lang/go/native/native_bytes.go` (one polymorphic word); `boru describe
slice`. REFERENCE.md:1160 (the string-table row names it a core
sequence word, see NUR019). `lang/go/native/help/help_categories.go`
(the string category points at core `slice`).
`lang/spec/edge-scalars-3.tsv:45-53`, `corpus-core.tsv:119`,
`corpus-structures.tsv:14` (string and list behaviour; the two-argument
negative-start form at `edge-scalars-3.tsv:47,52`).

---

## NUR020 — `print` stays in core; every other IO word is namespaced {#nur020}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-31
(`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

**Rule:** the IO vocabulary lives in `boru:io` (`IO.printstr`,
`IO.read`, `IO.write`, …); moved words are not available unqualified.

**Divergence:** `print` alone stays in core, unqualified.

**Why allowed:** `print "Hello, World"` must be a complete first
program, with no `import`, as in practically every mainstream language.
That outweighs family symmetry for exactly one word; everything programmatic
(`printstr`, streams, `read`/`write`, `trace`) still demands the import.
A second unqualified IO word would need its own NUR.

**Evidence:** `lang/go/native/native_print.go` and `register.go` (the
single core IO registration); `io_module.go` (everything else).
`lang/go/CLAUDE.md` §"Package layout" ("only `print` stays in core");
ADR-004 §Consequences argues `print`'s forwardness, a separate question.
`boru -e 'print "Hello, World"'` runs with no import; HOWTO.md uses it
unqualified throughout.

---

## NUR022 — `del` covers a fraction of `set`'s containers {#nur022}

**Status:** Allowed · **Verdict:** maintainer, 2026-08-14 (the container
gap itself was fixed 2026-08-02 under the 2026-07-31 resolve-by-fix
verdict, `design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

**Rule (as restated 2026-08-14):** the storage-column words cover the
same **keys** — a key `set` can write, `del` can remove. Slots are out
of scope: a declared Class field and a List index are positions, not
keys.

**Divergence:** `set` can write a declared Class field and `del` cannot
remove it (nor a List index: the List refusal names pop / shift /
`ArrayUtil.remove-at`). Otherwise both words carry the same eleven
containers and key shapes, 19 signatures each; Class, Micron and the
Lists refuse through registered signatures with their own messages.

**Why allowed:** the inverse of writing a slot is writing another value,
not deleting it — an instance missing a declared field would not satisfy
its type, and removing a List index shifts the tail. One line, drawn
twice. `del` earns its place because an absent key and a
key bound to `none` are distinct through `has`/`size`/`keys`/`eq`:
`({a:1 b:2} del b) eq ({a:1 b:2} set b none)` → false. Deliberately not
closed here: through `get` the two are indistinguishable (both `typeof`
`None`, both `eq none`) — that belongs to the **sentinel-values
programme**, which decides whether a distinct miss sentinel exists and
does not reopen this record.

**Evidence:** `lang/go/native/native_del_symmetry_test.go` (same
container set and key shapes, failing in both directions).
`lang/spec/flex.tsv` §12 (the per-container contract and refusals).
`eng/go/store_tombstone_test.go` (Store's copy-on-write `CowDel`).

---

## NUR024 — Two orderings by design: semantic (`cmp`) and deterministic (`tcmp`) {#nur024}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-31
(`design/legacy/NUR-RESOLUTION-PLAN.0.ignore`)

**Rule:** one comparison vocabulary, one totality regime.

**Divergence:** `cmp`/`lt`/`lte`/`gt`/`gte` raise `[boru/incomparable]`
across families (`cmp true 1` errors), `eq`/`neq`/`deq` are total (`1 eq
"1"` → false), and `tcmp` is an unrestricted total order — so `cmp` and
`tcmp` answer differently for the same pair.

**Why allowed:** two orderings, deliberately. **Semantic** ordering
(`cmp` and the relationals) asks "which is greater, as values in one
domain?" and rejects meaningless comparisons, catching a real type
error where it is cheapest. **Deterministic** ordering (`tcmp`) is a
stable total order over everything (signature order, map-key walks,
reproducible sorts) and never rejects. Equality is total in both because "the same value?" has an
answer across families (no) where "which is greater?" does not. An ADR
stating the split is a recorded candidate (ADR candidate 5 in the
resolution plan).

**Evidence:** REFERENCE.md §Comparison — the ordering words are
"**family-restricted**" (:1202-1206), `tcmp` is "the **unrestricted**
total order" (:1208), and "Only the **ordering** words restrict"
(:1214-1216). `eng/go/compare.go` (the family restriction);
`eng/go/compare_types.go` (tcmp's Rank-based order).
`lang/spec/compare.tsv`, `lang/spec/compare-restrict.tsv` (both regimes,
positive and negative).

---

## NUR039 — `slice` with a negative start silently ignores its end argument {#nur039}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-30

**Rule:** an argument is honoured or refused, never ignored;
out-of-domain indices clamp predictably (`slice 5 6 "abc"` → `''`,
`slice 0 5 "-"` → `'-'`).

**Divergence:** a negative start collapses `slice start end s` to the
two-argument "drop N from the end" form and discards `end`; the
count-from-the-end convention is documented, the dropped `end` is not:

```
slice -3 -1 'abcde'   →  ab
slice -3  2 'abcde'   →  ab
slice -3  5 'abcde'   →  ab
slice  1  3 'abcde'   →  bc     (the positive form honours end)
```

**Why allowed:** callers avoid the spelling by clamping — a negative
start (say, a failed `indexof`'s `-1`) is usually a call-site bug, not
an intent to count from the end — and honouring `end` or refusing the
combination would change a core sequence word. The acceptance rests on
an upstream guard: `cut` rejects `lo < 1` before slicing. That guard is
load-bearing — `cut-chars-rng` (utils/cut.boru:329-335) clamps only the
end, so a start of 0 reaching it would reproduce this collapse.

**Evidence:** the four calls above. `utils/cut.boru:165,180`
(`cut-point` / `cut-span` reject `lo < 1`) and
`utils/tests/cut_test.boru` (the rejection and clamping at both ends).
No spec row pins the three-argument collapse. NUR019 (where `slice`
belongs) is an independent question.

---

## NUR040 — `set` quotes a bare computed key where `get` refuses it {#nur040}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-30

**Rule:** sibling accessors treat their key argument the same way, and a
program that means a variable's value does not silently get its name.
The intended split (lang/go/CLAUDE.md): `dot`/`dotr` quote a bare word
as a literal field name, `get`/`getr` evaluate it.

**Divergence:** `set` has a quoting `Atom/q` key slot that `get` lacks:

```
def k "aa"   {} set k 1     →  {k:1}      # the NAME was stored
def k "aa"   {} set (k) 1   →  {aa:1}     # the VALUE
def k "aa"   {aa:1} get k   →  1          # get EVALUATES k
```

`boru check` reports only `unused_def: def k is never used` (which
neither alternative triggers); in real code, a loop builds its whole map
under one literal key.

**Why allowed:** the asymmetry leaks from the deliberate `dot`/`get`
split, and the quoting slot has a real purpose (`set name value store`
reads well). Making `set` evaluate its key would change a core word that
shipped code relies on. Optional improvement: a check advisory when a
bare word in a quoting slot is also a live binding.

**Evidence:** the three calls above. Avoiders: `utils/` (`(quote k)`
at all 117 literal keys) and `lang/go/modules/cli.boru` (`(quote …)` at
75 sites, `set (nm) …` at 8; house rule at cli.boru:53-54). Reliers:
`lang/go/modules/vault_tui.boru` (shipped, `//go:embed`-ed; 77
bare-word key sites, e.g. `state set screens …`), `kg/report.boru:334`,
`design/examples/apps/todo-tui.boru:52` and the linguist samples.
lang/go/CLAUDE.md:303-316 (the "**`dot` / `dotr` vs `get` / `getr`
(CRITICAL)**" bullet). Since 2026-08-21 `has` evaluates its key like
`get` (maintainer direction; `lang/spec/corpus-core.tsv`, incl. the
`ERROR:undefined_word` negative), so this covers `set` alone.

---

## NUR046 — `boru fmt` is not idempotent: one pass is not a fixed point {#nur046}

**Status:** Allowed · **Verdict:** maintainer, 2026-07-30

**Rule:** a formatter is idempotent — `fmt(fmt(x)) == fmt(x)` — so a
`make fmt` target converges and a check can be a single-pass diff.

**Divergence:** a `def name fn [[params] [Returns] [body]]` whose header
does not fit the width lays out differently on pass 1 and pass 2; passes
2..n are identical.

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

Cause: root-level newlines emitted by pass 1 change how pass 2 segments
statements. Costs: a `fmt` inside `all:` never converges in one run;
statement boundaries stop being visible (pass 1 joins two statements,
pass 2 indents one as a continuation); and non-canonical input is
untested, since the formatter's gates run on the canonical corpus.

**Why allowed:** formatting does not change behaviour — all 995 `utils/`
cases pass either way, and every affected file still passes `boru check`
— so the cost is tidiness, not correctness. Running `fmt` twice is not
the fix (the pass-1 layout is the unreadable one); the fix, when
scheduled, carries a guard: format every `.boru` twice, second pass a
no-op, with a non-canonical fixture.

**Evidence:** the repro above; the 2026-08-02 sweep — 19 of 122 `.boru`
files non-idempotent (all 12 `utils/*.boru`, shipped
`lang/go/modules/cli.boru`, `sift.boru`, `vault_tui.boru`, two
`design/examples`, two `editors/linguist/samples`), while all 11
`utils/tests/*_test.boru` sit at their fixed point. `utils/Makefile` keeps `fmt` out of `all`, citing
this record. `kg/Makefile:11,25-28` puts `fmt` in `all` and claims
idempotence — false in general, harmless because kg's sources are at
their fixed point; `kg/README.md` and `make fmt-docs` assume it too. The cause:
`design/legacy/NUR-EFFORT-TRIAGE.0.ignore:139-148`.

---

## NUR062 — Numeric marker letters are lowercase-only while every other letter in a literal is case-flexible {#nur062}

**Status:** Allowed · **Verdict:** maintainer, 2026-08-14 (recorded
2026-08-11 from the maintainer's PR #339 decision)

**Rule:** one lexical convention per kind of thing — letters inside a
numeric literal are either case-significant or not.

**Divergence:** the four marker letters are lowercase-only (`0x`, `0o`,
`0b`, `0d`; `0XFF` raises `[boru/syntax_error]: numeric prefix must be
lowercase: 0XFF`), while hex digits and the exponent marker take either
case (`0xff == 0xFF`, `1e3 == 1E3`). The rule governs literals only: in
a name position (`0XFF/q`, `0XFF/r`, `0XFF/t`, a bare map key
`{0XFF: 1}`) either case is a name, and `0d12` / `0D12` both decode as
lenient text through `StructUtil.parse`, since `0d` is not a DATA
numeric.

**Why allowed:** a marker spells syntax; digits and exponents are
content. `0XFF` is almost always a typo, while `0xAB`/`0xab` and
`1E3`/`1e3` carry no such signal — the refusal is a diagnostic.

**Evidence:** `parser/spec/parse.tsv` (8 refusal rows, and 5
name-position rows under §"the lowercase-only rule governs numeric
LITERALS"), `parser/spec/data.tsv` (4 refusal rows, and 3 rows under
§"the 0d big-number prefix is not a DATA numeric"),
`parser/spec/lex.tsv` (2 token rows) — each re-rendered by both port
runners. REFERENCE.md §"Numeric literals" (rule and scope). Both editor
grammars (tree-sitter, pygments) reject uppercase markers.

---

## NUR070 — `if` reads a List condition as CODE while every other truthiness consumer coerces it {#nur070}

**Status:** Allowed · **Verdict:** maintainer, 2026-08-15 (recorded
2026-08-14, surfaced by NUR053's fix)

**Rule:** one truthiness model, applied by every construct that coerces
a value to a Boolean (`design/TRUTHINESS.0.md`, the One Truthiness
Model).

**Divergence:** a List. `make Boolean` and `convert Boolean` coerce it
by presence; `if` runs it as a code body and takes the truthiness of
what it leaves, so the same bound value gets opposite answers:

```
def xs [0]   if xs ['T'] ['F']        # → 'F'     the body runs, yields 0, falsy
def xs [0]   convert Boolean xs       # → true    non-empty list is present
def xs [0]   make Boolean xs          # → true

def xs []    if xs ['T'] ['F']        # → [boru/runtime_error]: if: condition
                                      #   produced no value
def xs []    convert Boolean xs       # → false
def xs []    make Boolean xs          # → false
```

Every other shape (Map, `none`, String, every numeric leaf) agrees.

**Why allowed:** a List in a condition position is CODE — what a
concatenative language means by a bracketed body there; `if [ … ]
[then] [else]` spells a computed condition, and the truthiness model
governs values, not code positions. The accepted cost: the model has one
shape-shaped hole, and `if xs` over a bound list stays a sharp edge.
Not taken: coercing an already-evaluated condition while a literal stays
code (the spellings stop being interchangeable), or naming the
ambiguity in the empty-case error.

**Evidence:** `lang/spec/edge-scalars-3.tsv` (the NUR053 domain block:
the Map/None agreement rows and the three List rows showing the split).
`core/go/core_helpers.go` `CoerceBoolean` (the presence rule the two
constructors share). `design/TRUTHINESS.0.md` §2 (states the domain and
names this split). `lang/go/context_boundary_differential_test.go` (the
"if code-body condition" row).

---

## NUR334 — a read after a computed keep-defs body: the loud remainder {#nur334}

**Status:** Pending (loud) · **Recorded:** 2026-09-28 · **Narrowed:** 2026-09-30 (four times)

A unit returning a `/v`-read splice now takes a per-call-site result island
(compiler `call_result_island.go`, eng `callResultRestart`), and a live read
inside a spliced word's body an island of the spliced body at its firing
(`spliceBody`). Still a loud compiled defer where the interpreter answers:

- the call at a fn body's tail (the interpreter's tail call depends on the
  caller's context), a tail replacement, a word before the call on its
  level (`def k 3 end k f …`), or a lazy list read written after the call;
- a spliced word read more than once or nested (`[w]`, `(w)`), redefined,
  or preceded by a word that may collect it.

Pinned by lang `TestNUR334CallResultEdges`, `TestNUR334SplicedWordEdges`.

**Verdict (maintainer, 2026-09-29):** resolve by fix — compile every remaining shape and agree.

---

## NUR336 — a paren apply over a data member: the loud remainder {#nur336}

**Status:** Pending (loud) · **Recorded:** 2026-09-28 · **Narrowed:** 2026-09-30 (four times)

A two-argument lambda's landing lead in a counted `for` body's first
statement resumes the interpreter's own loop on the stopping iteration
(`LoopCont`, eng `loopContRestart`); a multi-result def's results are all
promoted (`multiOutToSlots`); a statement that took an earlier value or
pushed a literal late starts its island after the taking word
(`lateStart`, `NoteParenStack`). Still a loud compiled defer:

- a loop continuation with body or carried defs, a `while`/condition loop,
  nested loops, a stop in a later body statement, or an effect before it;
- a word that collects the paren forward (`drop print (q.f 7)`,
  `size [(q.f 7)]`), root `drop [(m.f 7)]`;
- a type value written or read before the stop in a unit
  (`Integer end (q.f 7) drop drop` — a silent wrong answer before, now loud).

Pinned by lang `TestNUR336LoopContinuationEdges`, `TestNUR336LateStartEdges`.

**Verdict (maintainer, 2026-09-29):** resolve by fix — compile every remaining shape and agree.

---

## NUR356 — a literal handed to a call matched at run time: the remainder {#nur356}

**Status:** Pending (a wrong error; one designed defer) · **Recorded:** 2026-09-29 · **Narrowed:** 2026-09-30 (twice)

No-match notes now render the literal as written (`CallWindows`,
`PolySplit.Words`), native effects are declared (`core.CompileSideEffect`,
censused) and an effectful native in such a literal declines "(NUR356)"; a
poly only some of whose overloads have an effect is guarded at run time
(`vm:quiet-poly-effect`). What remains:

- a native that raises by VALUE inside a literal handed to a call matched
  at run time raises before the interpreter's no-match:
  `def h fn [[] [Any] [3]] end def s fn [[] [String] ["abc"]] end each (h) [convert Integer (s)]`
  (compiled `convert`'s error, interpreted `each`'s), and `get (l) 5` out of
  range likewise. Declining every native call there breaks real programs;
  owed: evaluate the literal only after the match, or a pre-match guard.
- a computed arm holding a pending literal (`if c (mk) [0]`) is a designed
  runtime defer (only carriers reach the arm at compile time).

**Verdict (maintainer, 2026-09-29):** resolve by fix — render as written; classify native effects.

---

## NUR359 — a computed run applying an argument-taking fn value defers {#nur359}

**Status:** Pending (loud) · **Recorded:** 2026-09-29 · **Narrowed:** 2026-09-30

A plain computed run whose only re-stepped values are zero-argument fns now
re-steps them in place (VM `plainReSteps`, `doReStep`), so
`for 2 [do (mk)] 7` over `quote [f/v]` with `f` breaking answers `[7]`.
Still the loud `vm:dyn-body-plain` defer: a run holding a fn that takes an
argument (`TestNUR359ArgTakingFnStaysLoud`).

**Verdict (maintainer, 2026-09-29):** resolve by fix — compile it.

---

## NUR361 — a nested gradual read holding a fn defers {#nur361}

**Status:** Pending (loud; was silent) · **Recorded:** 2026-09-29 · **Narrowed:** 2026-09-30

A gradual read nested in a body the unit or the root runs inline (a `var`
body in an `each` callback, an arm, a loop body) is now guarded where it
happens (compiler `read_site_guard.go`): data values compile and agree, and
a fn value raises the loud NUR123 defer instead of the silent `[[fn v]]`.
An exact compile needs an island that resumes mid nested body. Root
`var [[] v]` is loud likewise. Pinned by lang
`TestNUR361NestedGradualReadGuarded`.

**Verdict (maintainer, 2026-09-29):** resolve by fix, first priority (the only silent wrong answer open): guard the read, or decline soundly.

---

## NUR364 — a failed call over a pending literal in a fn body renders a wider window {#nur364}

**Status:** Pending (notes only) · **Recorded:** 2026-09-30 · pre-existing at 474954ff1

```
def g fn [[x:Integer][Any][{f: (1 add 1)} mul x]] end g 3
  interpreted   the argument was {f:(1 add 1)}
  compiled      the arguments were 3 and {f:(1 add 1)}
```

The user-call twin (`h {f: (1 add 1)} x` in a fn body) splits the same way.
Code and caret agree.

---

## NUR366 — a declined stored fn that breaks its declared return count raises internal_error compiled {#nur366}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/decision migration)

```
# lib.boru, imported from a FILE, so `tail-arity` is a stored fn unit; its
# stamp declines: "stored fn: bare read of `r` may hold a fn the
# interpreter dispatches as a word (NUR279)"
def tail-arity fn [[m:Map] [Any] [ def r (m get "x") 10 r ]]
export "L" { tail-arity: tail-arity/v }

import "<dir>/lib.boru" L.tail-arity {x: 5}
  interpreted   [boru/type_error] tail-arity: expected 1 return value(s), got 2 — [10 5]
  compiled      [boru/internal_error] bytecode: internal: dynamic frame replay tail-arity:
                result count 2 differs from the declared 1; the compiled runtime cannot
                execute it (pc=0, src 0:0) — "this is a compiler defect"
```

With a fn in `x` (`(n:Integer => [n add 1])`) the tail read takes the `10`
and both lanes answer `[11]`. A STAMPED fn that breaks the same contract
(`def two-plain fn [[m:Map] [Any] [ 10 (m get "x") ]]`, same file) raises
the type_error on both lanes. So the compiled caller's dynamic frame
replay over the declined unit sees the count breach but reports it as its
own internal failure, where the contract's error is owed. (Both lanes also
render the caller's column against line 1 of the module file; that part
agrees.)

**Proposed verdict:** resolve by fix — the replay raises the return
contract's `type_error` with the interpreter's text. The stamp decline
behind it is a refusal, tracked separately in
[design/COMPILABLE-SUBSET.md](design/COMPILABLE-SUBSET.md) §5 ("open
refusals recorded 2026-10-02").

---

## NUR367 — a loop-rebound local read bare in an arm after the loop returns its fn uncalled compiled {#nur367}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/decision migration)

```
def f ([] => [42])
def g fn [[m:Map] [Any] [ def r 0 for 1 [def r (m get "x")] end if (m has "x") [r] [0] ]]
def o (g {x: f/v})
print (o/v typeof)
  interpreted   Integer    (the arm's bare `r` calls f, per ADR-011)
  compiled      Function   (the fn comes back uncalled; `boru check`: 0 errors)
```

The same body with the rebinding in other loops (`r` rebound in the loop
body, then the arm read after the loop, `x` holding `f`):

| loop around `def r (m get "x")` | interpreted | compiled |
|---|---|---|
| `for 1 [ … ] end` | `Integer` | `Function` (silent) |
| `def k 0 while [k 1 lt] [ … def k (k 1 add)]` | `Integer` | `Function` (silent) |
| `[1] each [ … 0] drop` | `Integer` | `[boru/type_error] g: expected 1 return value(s), got 2 — [[0] 42]` |
| `def ys ([1] each [ … 0])` | `Integer` | `` [boru/internal_error] bytecode: internal: dynamic-scope read of a dispatching binding `r` `` |
| `[1] for-each [ … ]` | `Integer` | `[boru/type_error] g: expected 1 return value(s), got 2 — [[] 42]` |

The lanes agree when the rebinding is straight-line (`def r 0 def r (m
get "x") if … [r] [0]`), when the read after the loop is the body's tail
(`… end r`), when `x` holds data, and on `r/v` in the arm. The init does
not matter: `def r {a: 1}`, `def r None` and a gradual `def r (m get
"y")` diverge alike, as does a read in the else arm or inside `def z (if
… [r] [0])`. The read is a gradual read nested in an arm, the case
[NUR361](#nur361)'s read-site guard makes loud; here the guard does not
fire, so the `for`/`while` rows are a silent wrong answer beside the one
NUR361's verdict names.

Live instance: voxgig-boru/decision's `unique` hit policy ended `if
(match-count 1 eq) [result] [ … ]` after a `for` that rebinds `result`.
On 64c5ab2 its stamped unit returned a stored fn `then` uncalled, while
the `first` and `priority` paths (declined under NUR279, so interpreted)
called it. The library now reads `result/v` on every path, because a
stored `then` is data under its own contract, so it no longer depends on
either lane.

**Proposed verdict:** resolve by fix — guard the post-loop read
(dispatch a fn as the interpreter does) or decline soundly, as NUR361's
verdict asks for its family.

---

## NUR368 — a run-time fn value in a local, applied at the main program, applies late compiled {#nur368}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/decision migration)

```
def inc (n:Integer => [n add 1])
def out2 ({x: inc/v} get "x")      # a fn value obtained at run time
print (41 out2) 7
  interpreted   prints 42, leaves [7]
  compiled      prints 41, leaves [8]   (`boru check`: 0 errors; exit 0)
```

The compiled lane prints the argument and the deferred call of `out2`
then takes the next value. Without the trailing `7`, `print (41 out2)`
prints `41` and only then raises the NUR123 defer (`` internal_error:
gradual read `out2` holds a fn the interpreter dispatches here and the
unit could not re-step ``), so the defer is loud only after a wrong
effect, and silent whenever a value follows. A lead returned by a fn instead
(`def give fn [[m:Map] [Any] [m get "x"]]`, `def out2 (give {x:
inc/v})`) gives the same two rows. A 0-arg fn returned by a fn and bound
through a paren reads back undefined:

```
def give fn [[m:Map] [Any] [m get "x"]]
def f42 ([] => [42])
def out (give {x: f42/v})
def r (out)
print (r/v)
  interpreted   prints 42
  compiled      [boru/undefined_word] undefined word: r   (at the `def r`)
```

The lanes agree on `41 out2/v apply` (in `print`, in a `def`, or followed
by more values), on `(41 out2) 7 add`, on `print (out)` over a returned
0-arg fn, when the value is passed to a fn whose param is declared
`g:Function`, and when the lead is statically a fn (`def out2 inc/v`). The other spellings of
the same call decline to compile; they are listed in
[design/COMPILABLE-SUBSET.md](design/COMPILABLE-SUBSET.md) §5 ("open
refusals recorded 2026-10-02"). The `print (41 out2)` read is a gradual
read the root runs inline, the case [NUR361](#nur361)'s guard covers; it
fires, but after `print` has consumed the paren's value.

Downstream: voxgig-boru/decision's evaluators return a fn stored in a
rule's `then` or a leaf's `result` as data, for the caller to apply; its
docs now name `apply` and a `Function`-typed param as the spellings that
agree.

**Proposed verdict:** resolve by fix — apply the run-time lead where the
paren closes (as the interpreter does), or decline before any effect of
the enclosing call runs; bind `def r (out)` as the interpreter does.

---

## NUR369 — a re-entered fn-value param called from an `each` callback applies the inner call's fn compiled {#nur369}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/template migration)

```
def tj fn [ [xs:List body:Function] [List] [ (xs each [ var [[x] (body x) ] ]) ] ]
def b2 fn [ [c:Integer] [Integer] [ c mul 10 ] ]
def b1 fn [ [c:Integer] [Integer] [ (tj [7 8] b2/v) size ] ]
print (tj [1 2] b1/v)
  interpreted   [2, 2]
  compiled      [10, 20]    (`boru check` clean; exit 0)
```

`b1`'s call re-enters `tj` with `b2`; when it returns, the outer loop's
callback applies `b2` (the inner frame's `body`) instead of its own `b1`.
A silent wrong answer. voxgig-boru/template's block renderers had this
shape whenever blocks nested (a liquid `for` inside a `for` rendered one
outer iteration); the library now lowers every block to a named generated
fn and passes no fn values.

**Proposed verdict:** resolve by fix — each activation's callback reads its
own frame's param.

---

## NUR370 — a fn param called on a def made in the same `each`/`var` body raises DISPATCH_GENERIC compiled {#nur370}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/template migration)

```
def apply-each fn [ [xs:List f:Function] [List] [
  (xs each [ var [[e] def x (e get "k") (f x) ] ])
] ]
print (apply-each [{k:1} {k:2}] (fn [ [c:Any] [Any] [ c ] ]))
  interpreted   [1, 2]
  compiled      [boru/internal_error] bytecode: internal: DISPATCH_GENERIC at f: the live
                plan claims 1 forward of 1 where the record claimed 0 of 1; the compiled
                runtime cannot execute it (vm:generic-claim-drift)
```

`(f x/v)`, `(f (x))` and `(f (e get "k"))` answer `[1, 2]` on both lanes.

**Proposed verdict:** resolve by fix.

---

## NUR371 — a def in an `if` arm of an imported fn's fold body clobbers the caller's same-named local compiled {#nur371}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/template migration)

```
# mod.boru (imported from a file)
def collect fn [ [n:Integer] [List] [
  def out (flex [])
  def res (do {k:[""]} (iota n) [ var [[i acc]
    if (i eq 1) [ def _ (out push i)  do {k:[""]} ] [ acc ]
  ] ] fold)
  slice 0 (out size) out
] ]
def work fn [ [s:String] [String] [ if (s eq "zz") [ convert String ((collect 3) size) ] [ add "?" s ] ] ]
export "M" { work: work/v }

# main.boru
import "<dir>/mod.boru"
def f fn [ [s:String] [String] [ def out (M.work s) `out=${out}` ] ]
print (f "ab")
  interpreted   out=ab?
  compiled      [boru/undefined_word] undefined word: out
```

`work` only reaches `collect` in an arm never taken. Renaming either
`out`, or reading it with a plain `print (out)` instead of the template
string, makes the lanes agree.

**Proposed verdict:** resolve by fix — a callee's defs never touch the
caller's frame.

---

## NUR372 — a fold body reading its fn's param raises undefined_word on the 5th–8th compile in a process {#nur372}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/template migration) · **library-scale repro; not isolated below it**

`template.aql` at voxgig-boru/template `bea732e`, whose `split-args`
folds over the characters of its param `s` reading `s` inside the fold
body. Calling `Template.compile` twelve times on a liquid template with a
filter argument (`{{ v | append: "a" }}`), each in
`do [(… Template.compile).program size)] error [get "message"]`:

```
  interpreted   5635 (all twelve)
  compiled      5635 ×4, then `undefined word: s` ×4, then 5635 ×4
```

A cut-down standalone module did not reproduce it. The library's current
`split-args` folds over `StringUtil.split "" s` and reads no outer name in
the fold body, and agrees on every call.

**Proposed verdict:** resolve by fix (owed: a minimal repro).

---

## NUR373 — a fn whose result is an ungrouped `r.list-of` over its random-source param repeats the first draw compiled {#nur373}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/aless migration) · **silent**

```
import "boru:rand"
def g fn [[r:Map] [Any] [ r.list-of [r.int 0 999] 4 ]]
print (g (Rand.with-seed 7))
  interpreted   [550, 417, 502, 54]
  compiled      [550, 550, 550, 550]
```

The compiled call draws once and repeats that value. The same happens
with `[r.float]` (`[0.9188921592527635, 0.9188921592527635, …]`), with
`r:Any`, with a trailing `end`, with a computed count
(`[[n:Integer r:Map] [Any] [ r.list-of [r.int 0 999] n ]]`), and when the
fn is called from a `Test.check-prop` generator (`[ (g r) ]`), where
every property still passes on the repeated lists — only a value-level
diff shows it. Grouping the call (`[ (r.list-of [r.int 0 999] 4) ]`) or
binding it first (`def xs (r.list-of [r.int 0 999] 4) xs`) agrees with
the interpreter.

Related, not a divergence: with the declared return `[List]` instead of
`[Any]`, the pre-flight check refuses the program — `type_error: g:
return value 1: expected List, got Integer` — for the grouped body too,
which runs and answers `[550, 417, 502, 54]` on both lanes under
`-no-check`. A checker false positive over `r.list-of`'s result.

**Proposed verdict:** resolve by fix.

---

## NUR374 — a `check-prop` generator whose body is a grouped `r.list-of` raises `undefined word: r` compiled {#nur374}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/aless migration)

```
import "boru:test"
def p (Test.check-prop "p" [(r.list-of [r.int 0 999] 4)] [ var [[ops] (ops size) eq 4 ] ] 3 7 0)
print (p)
  interpreted   {"name": "p", "ok": true, "runs": 3, …, "error": null}
  compiled      {"name": "p", "ok": false, "runs": 1, …, "error": error(undefined word: r)}
```

`r.list-of` runs its element body (`[r.int 0 999]`) by itself (on a
pooled interpreter, see [design/COMPILABLE-SUBSET.md](design/COMPILABLE-SUBSET.md)
§5), and there the generator's random source `r` is not bound. The
property reports a failure the interpreter does not. The same call in a
named fn whose PARAM is `r` (`def ints fn [[n:Integer r:Map] [List] [
(r.list-of [r.int 0 999] n) ]]`, generator `[ (ints 4 r) ]`), or in a
lambda handed to a `Function`-typed param, agrees. The check pass has
the same blind spot for a var-bound `r`: `print (rs each [ var [[r]
(r.list-of [r.int 0 999] 4) ] ])` over `def rs [(Rand.with-seed 7)]` is
refused by the pre-flight check (`undefined_word: undefined word: r` at
the element body), and answers `[[550, 417, 502, 54]]` on the
interpreter.

**Proposed verdict:** resolve by fix — the element body sees the
bindings of the code that calls `r.list-of`, on both lanes and in the
check pass.

---

## NUR375 — a callback that binds a draw with `def` and leaves the binding raises "a landed fn value takes arguments" compiled {#nur375}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru property-generator rewrites)

```
import "boru:rand"
def rs [(Rand.with-seed 1)]
print (rs each [ var [[r] def a (r.int 0 9) def b (r.int 0 9) [a b] ] ])
  interpreted   [[5, 7]]
  compiled      [boru/internal_error] bytecode: internal: a landed fn value takes
                arguments: the interpreter's re-step applies it here over the value
                written after it, and no compiled apply re-steps it (NUR298); the
                compiled runtime cannot execute it (pc=5, src 3:34)
```

`boru check` is clean and the program compiles; the compiled runtime
then fails at the first `r.int`. In a `Test.check-prop` generator the
same body (`[ def a (r.int 0 9) def b (r.int 0 9) [a b] ]`, or `[ def xs
(r.list-of [r.int 0 999] 4) xs ]`) does not stop the program: the
property reports `ok: false` with that error after one run, where the
interpreter passes. The same body in a lambda handed to a fn whose param
is `cb:Function` (`(cb (Rand.with-seed 1))`) answers `[5, 7]` on both
lanes, and so does the generator `[ [(r.int 0 9) (r.int 0 9)] ]`. The
error text names NUR298 (closed 2026-09-27); this is a shape its fix
does not reach.

**Proposed verdict:** resolve by fix.

---

## NUR376 — a name read inside a `do {k: [expr]}` map value leaves a same-named callback binding undefined compiled {#nur376}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/stats migration)

```
def f fn [[x:Integer] [Map] [ def r (x add 1) do {k: [r]} ]]
def g fn [[xs:List] [List] [ xs each [var [[r] r]] ]]
print (f 1)
print (g [1 2])
  interpreted   {"k": 2}
                [1, 2]
  compiled      {"k": 2}
                each: element 0: [boru/undefined_word]: undefined word: r (at g's `r`)
```

A fn-local name read inside a `do {…}` map value makes a later binding
of the same name unresolvable — here a `var` in a later, unrelated fn's
`each` callback; a def-bound name does the same (`def f fn [[xs:List]
[Map] [ def fs xs do {k: [fs get 0]} ]]`, then `def g fn [[xs:List]
[List] [ def fs xs fs each [var [[x] (x add 1)]] ]]`: `g [1 2]` answers
`[2, 3]` interpreted, `undefined word: fs` compiled). `boru check` is
clean. voxgig-boru/stats met it three ways (`Stats.mode`, `Stats.ols`
after `Stats.linreg`, `Stats.zscores` after `Stats.mode`), so one failing
call broke later, unrelated ones; it now writes plain map literals
(`{k: (expr)}`), which agree. A `do {…}` map also runs every value body
on the interpreter at run time (design/COMPILABLE-SUBSET.md §5).

**Proposed verdict:** resolve by fix.

---

## NUR377 — `boru:test` mints its record types from a fresh ID counter; a type made after it compares as a different type compiled {#nur377}

**Status:** Pending · **Recorded:** 2026-10-02 · measured at 64c5ab2 · surfaced downstream (the voxgig-boru/bloom-filter and stats migrations) · **silent** (`is`)

```
# lib.boru
def Box class { v: 0 }
def mk fn [ [n:Integer] [Box] [ make Box {v: n} ] ]
def mk-any fn [ [n:Integer] [Any] [ make Box {v: n} ] ]
export "L" { mk: mk/v, mk-any: mk-any/v, Box: Box }

# main.boru
import "boru:test"
import "./lib.boru"
print (L.mk 1)
  interpreted   Class/Box{v:1}
  compiled      [boru/type_error] mk: return value 1: expected Box, got Box
print ((L.mk-any 2) is L.Box)
  interpreted   true
  compiled      false
```

`BuildTestModule` (`lang/go/modules/test.go`) builds its sub-registry
with `newDefaultRegistry()` and never calls
`modReg.Types.AdoptSeqFrom(parent.Types)`, so the record types its
preamble mints draw IDs from a fresh counter and collide with types
minted elsewhere in the program. e69b9ac35 ("Fix minted-type ID
collisions across sibling registries") added the call to the module-body
path and to `parse`, `model`, `matrix-util`, `time-util`, `io`, `net` and
`minilang`, not to `boru:test`. The compiled return check and `is` look
the declared type up by ID (`core.CanonicalType`) and find `boru:test`'s
type; the interpreter compares the nodes directly. With `./lib.boru`
imported BEFORE `boru:test` both lanes answer `Class/Box{v:1}`, which is
the workaround every affected library documents.

**Proposed verdict:** resolve by fix — `BuildTestModule` adopts the
parent's type-ID sequence like its siblings.

---
