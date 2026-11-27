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
> [`design/legacy/COMPILABLE-SUBSET.ignore`](design/legacy/COMPILABLE-SUBSET.ignore) §5 — not
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

> **Compiled lane removed 2026-10-16; its records discharged 2026-11-27.**
> The compiler lane is gone
> ([design/COMPILER-LANE-REMOVAL.0.md](design/COMPILER-LANE-REMOVAL.0.md))
> and the interpreter is the one execution path, so the "two lanes" of the
> scope ruling above are history. By the maintainer's ruling of 2026-11-27
> every Pending record whose divergence existed only against the compiled
> lane was deleted as Resolved, and each record that also made a claim
> about the interpreter, or about `boru check` against the run, was
> re-measured on the interpreter and kept only as far as it still
> reproduces.

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
| [NUR366](#nur366) | A return-contract error raised by a FILE-module fn is drawn over the wrong file's text: the run draws the caller's position over the module's source, `boru check` the module's position over the caller's (code and message are right) | downstream: the voxgig-boru/decision migration (2026-10-02); narrowed 2026-11-27 to the interpreter's rendering |
| [NUR373](#nur373) | The check pass refuses a `[List]`-declared fn whose result is `r.list-of` over its `r:Map` random-source param (`expected List, got Integer`); the run answers the list | downstream: the voxgig-boru/aless migration (2026-10-02); narrowed 2026-11-27 to the check false positive |
| [NUR374](#nur374) | The check pass refuses a `var`-bound random source read in `r.list-of`'s element body inside an `each` callback (`undefined word: r`); the run answers | downstream: the voxgig-boru/aless migration (2026-10-02); narrowed 2026-11-27 to the check false positive |
| [NUR377](#nur377) | `boru:test` mints its record types from a fresh ID counter: a user class is `teq` to an unrelated `Test` record type and its instances `tis` it (silent) | downstream: the voxgig-boru/bloom-filter and stats migrations (2026-10-02); narrowed 2026-11-27 to what the interpreter shows |
| [NUR379](#nur379) | A fn param or an `unpack` name may be a built-in word's name, where `def` and a `var` name raise `reserved_word`: the param replaces the built-in in the body and in every fn the body calls, `unpack` for the rest of the program; `boru check` is silent | downstream: the voxgig-boru interpreter-entry pass (found 2026-10-02, recorded 2026-10-05); restated 2026-11-27 |
| [NUR381](#nur381) | A container literal a `var` body or a `=>` body leaves as its value is evaluated after the body's names are unbound: a name read in it resolves to a same-named outer binding (silent) or raises `undefined_word`; a `fn` body keeps it in scope | downstream: voxgig-boru/trie (found 2026-10-02, recorded 2026-10-05); restated 2026-11-27 |
| [NUR384](#nur384) | A name a fn-body `unpack` binds outlives the call when the body names no other binding word (a new name stays bound, an outer one stays rebound) and is call-local when it does; `boru check` reads it as call-local | the interpreter fn-kind inlining pass (2026-10-08); narrowed 2026-11-27 |
| [NUR385](#nur385) | A disjunct is reduced to its canonical alternatives only when resolution rebuilt one of them: `d( 5 5 Integer )` stays as written, `d( [q 1 ] 5 5 Integer )` is deduplicated and ordered | the Lean core specification's differential search (2026-11-27) |
| [NUR386](#nur386) | The kernel's `Unify` of two disjuncts answers their first matching alternative, not their meet (`tand` answers the disjunct); the empty disjunct unifies with neither itself nor `Never` | the Lean core specification, `Theorems/UnifyOrder.lean` (2026-11-27) |
| [NUR387](#nur387) | An unmatched `(` inside a container literal takes the literal's sealing paren for its close: `[ negq ( ]` raises no_value_error and `[ negq ( 5 ]` syntax_error where `negq (` and `negq ( 5` raise signature_error | the Lean core specification's differential search (2026-11-27) |
| [NUR388](#nur388) | A function VALUE whose parked collection is refused at arrival is re-planned in full and loops to the step limit — `( tyq/v 'x' )` is evaluation_limit — where the same function called as a word is a signature_error | the Lean core specification's differential search (2026-11-27) |

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

## NUR366 — a file-module fn's return-contract error is rendered over the wrong file's text {#nur366}

**Status:** Pending (notes only) · **Recorded:** 2026-10-02 · **Narrowed:** 2026-11-27 (the compiled replay's `internal_error` went with the compiled lane; what the record had noted of the interpreter's own rendering remains) · measured at d1db0f8 · surfaced downstream (the voxgig-boru/decision migration)

**Rule:** a diagnostic's position is drawn over the source it was taken
in, and a position in another file names that file — as the second label
below does.

**Divergence:**

```
# lib.boru
def tail-arity fn [[m:Map] [Any] [ def r (m get "x") 10 r ]]
export "L" { tail-arity: tail-arity/v }

# main.boru
import "./lib.boru" L.tail-arity {x: 5}

boru -no-check main.boru
  error: [boru/type_error]: tail-arity: expected 1 return value(s), got 2 — [10 5]
    --> 1:21
    1 | def tail-arity fn [[m:Map] [Any] [ def r (m get "x") 10 r ]]
                            ^ tail-arity: expected 1 return value(s), got 2 — [10 5]
    --> <dir>/lib.boru:1:28
    1 | def tail-arity fn [[m:Map] [Any] [ def r (m get "x") 10 r ]]
                                   - the declaration of `tail-arity` expects 1 return value(s)

boru check main.boru     (and plain `boru main.boru`, which its pre-flight check stops)
  check: 1:36: [error] type_error: tail-arity: expected 1 return value(s), got 2 — [10 Integer]
    1 | import "./lib.boru" L.tail-arity {x: 5}
                                           ^^^^^^^^^^
```

The code and the message are right. The run's first label is the CALL's
position (`main.boru` 1:21, the `L`) drawn over line 1 of `lib.boru`; the
check's is the module body's first token (`lib.boru` 1:36, the `def`)
drawn over line 1 of `main.boru`, with no file named. For `def two fn
[[m:Map] [Any] [ 10 (m get "x") ]]` on line 2 of the module, called as
`L.two {x: 5}` at 4:4 of the caller, the run draws `--> 4:4` over lines
2–4 of the module file and the check draws the module's `2:29` over the
caller's blank line 2. An error raised INSIDE a module fn's body is
labelled with the module file and drawn over it (`def boom fn [[m:Map]
[Any] [ raise oops/q "x" ]]` on the module's line 1: `-->
<dir>/lib.boru:1:30`), and the same fn defined in the calling file is
drawn right — so what loses its file is the contract's call-site label on
the run, and on the check every diagnostic taken in a file-module fn's
body (an `unused_def` warning for a `def` on line 3 of a module is drawn
at `3:7` of the importing file likewise).

**Proposed verdict:** resolve by fix — the call-site label keeps the
caller's source, and a check diagnostic taken in a module body names the
module file.

---

## NUR373 — the check pass refuses a `[List]`-declared fn whose result is `r.list-of` over its `r:Map` param {#nur373}

**Status:** Pending (a check false positive) · **Recorded:** 2026-10-02 · **Narrowed:** 2026-11-27 (the compiled lane's repeated draw went with the lane; the false positive the record had noted beside it remains) · measured at d1db0f8 · surfaced downstream (the voxgig-boru/aless migration)

**Rule:** the check pass refuses only a program the run would refuse.
`boru <file>` runs it as a pre-flight, so a false positive stops a correct
program.

**Divergence:**

```
import "boru:rand"
def g fn [[r:Map] [List] [ r.list-of [r.int 0 999] 4 ]]
print (g (Rand.with-seed 7))
  boru check <file>       2:28: [error] type_error: g: return value 1: expected List, got Integer
  boru <file>             the same error, from the pre-flight check; nothing runs
  boru -no-check <file>   [550, 417, 502, 54]
```

The grouped body (`[ (r.list-of [r.int 0 999] 4) ]`) is refused alike,
and a `[String]` declaration is refused as `expected String, got Integer`
where the run raises `expected String, got List`: the pass types the
member call's result as an Integer. Clean on the check, with the same
list on the run: the declared return `[Any]`, the param typed `r:Any`,
the result bound first (`def xs (r.list-of [r.int 0 999] 4) xs`), and a
module-level `r` read from a 0-arg fn.

**Proposed verdict:** resolve by fix, in the check pass.

---

## NUR374 — the check pass refuses a `var`-bound `r` read in `r.list-of`'s element body inside an `each` callback {#nur374}

**Status:** Pending (a check false positive) · **Recorded:** 2026-10-02 · **Narrowed:** 2026-11-27 (the compiled `check-prop` failure went with the compiled lane; the check pass's refusal the record had noted beside it remains) · measured at d1db0f8 · surfaced downstream (the voxgig-boru/aless migration)

**Rule:** as [NUR373](#nur373) — the check pass refuses only a program
the run would refuse.

**Divergence:**

```
import "boru:rand"
def rs [(Rand.with-seed 7)]
print (rs each [ var [[r] (r.list-of [r.int 0 999] 4) ] ])
  boru check <file>       3:39: [error] undefined_word: undefined word: r     (the element body's `r`)
                          3:39: [error] no_signature: cannot call `dot` — no signature matches the arguments; got (Atom, Word) …
  boru <file>             the same two errors, from the pre-flight check; nothing runs
  boru -no-check <file>   [[550, 417, 502, 54]]
```

The run reads the `var`'s `r` in the element body; the pass does not see
the binding there. It refuses the def-bound spelling too (`var [[r] def
xs (r.list-of [r.int 0 999] 4) xs ]`). Clean on the check: an element
body that reads no `r` (`[1]`), a draw outside a body (`var [[r] (r.int 0
999) ]`), the same `var` at the top level (`(Rand.with-seed 7) var [[r]
print (r.list-of [r.int 0 999] 4) ]`), a named fn whose PARAM is `r`
(declared `[Any]`; declared `[List]` it meets NUR373), and a program that
also has a top-level `r` — the pass is then silent, with a different `r`
in reach. Probably the check pass's face of [NUR381](#nur381) (the
element body is a literal the pass still holds when the `var` names are
gone); read from the shape, not proven.

**Proposed verdict:** resolve by fix — the element body sees the bindings
of the code that calls `r.list-of`, in the check pass as on the run.

---

## NUR377 — `boru:test` mints its record types from a fresh ID counter: a user type is `teq` to an unrelated `Test` type {#nur377}

**Status:** Pending · **Recorded:** 2026-10-02 · **Narrowed:** 2026-11-27 (the compiled return check and `is`, which looked the declared type up by ID, went with the compiled lane; the colliding IDs remain, and the interpreter shows them through `teq` and `tis`) · measured at d1db0f8 · surfaced downstream (the voxgig-boru/bloom-filter and stats migrations) · **silent**

**Rule:** type identity is the lattice node's ID, and an ID is unique
across a registry tree — "one counter per registry TREE"
(`core/go/typetable.go`, `mintID`). A module sub-registry whose minted
types can reach the importing program adopts the importer's counter
(`AdoptSeqFrom`).

**Divergence:**

```
import "boru:test"
def A class { v: 0 }
def a (make A {v: 1})
print (A teq Test.TestSet)       true      two unrelated types, one ID
print (a tis Test.TestSet)       true
print (a is Test.TestSet)        false
print (A tcmp Test.TestSet)      1
```

The same four answers with the class made BEFORE the import; `boru check`
reports nothing either way. Of four classes made in a row after the
import, the first three are `teq` to `Test.TestSet`, `Test.PropertySpec`
and `Test.TestResult` in turn and the fourth to none of the six; `A teq
B` is `false`.

`BuildTestModule` (`lang/go/modules/test.go`) builds its sub-registry
with `newDefaultRegistry()` and never calls
`modReg.Types.AdoptSeqFrom(parent.Types)`, so the record types its
preamble mints and exports (`Test.TestCase`, `TestSet`, `TestSpec`,
`TestResult`, `PropertySpec`, `PropertyResult`) draw IDs from a fresh
counter, and the importing program's own mints repeat them. e69b9ac35
("Fix minted-type ID collisions across sibling registries") added the
call to the module-body path and to `parse`, `model`, `matrix-util`,
`time-util`, `io`, `net` and `minilang`, not to `boru:test`.

**Proposed verdict:** resolve by fix — `BuildTestModule` adopts the
parent's type-ID sequence like its siblings.

---

## NUR379 — a fn param or an `unpack` name may be a built-in word's name, where `def` and a `var` name raise `reserved_word` {#nur379}

**Status:** Pending · **Recorded:** 2026-10-05 (found 2026-10-02 by the voxgig-boru interpreter-entry pass) · **Restated:** 2026-11-27 — the interpreter no longer raises on the param (a fn frame has closed with its cleanup marker, not an `undef` per param, since 571a8a77d, 2026-10-08), so the first record's three answers for one program are now one; what remains is that the rule it was recorded against is kept by two binders and not by two others · measured at d1db0f8 · **silent** unless the replaced built-in then fails

**Rule:** a built-in word's name is not rebound to a value: `def` and
`undef` raise `reserved_word` (REFERENCE.md, "Core bindings are frozen;
core words are open"), and so does a `var` name, which desugars to `def`.
One binding store, one admission rule.

**Divergence:** a fn param (the `fn` and the `=>` form) and an `unpack`
name are bound without that guard, and `boru check` accepts both.

```
def sub 3                    [boru/reserved_word] def sub: 'sub' is a built-in word and cannot be redefined
5 var [[sub] sub]            the same error                     (both: run and check)

def g fn [[sub:Function] [List] [ [5] each [ var [[e] (e 3 sub/v apply) ] ] ]]
print (g add/v)
  run     [8]                (2026-10-05: reserved_word, from the frame's closing `undef sub`)
  check   0 errors

def h fn [[a:Integer b:Integer] [Integer] [a sub b]]
def g fn [[sub:Function] [Integer] [h 5 3]]
print (h 5 3)  print (g add/v)  print (h 5 3)
  run     2  8  2            (inside g's call, h's `sub` is g's param)
  check   0 errors

def h fn [[a:Integer b:Integer] [Integer] [a sub b]]
def g fn [[sub:Integer] [Integer] [h 5 3]]
print (g 4)
  run     [boru/type_error] h: expected 1 return value(s), got 3 — [5 4 3]
  check   0 errors

unpack [sub] {sub: 5}
print sub  print (10 sub 3)
  run     5, then 10 with `5 3` left on the stack   (`sub` is 5 for the rest of the program)
  check   0 errors
```

So a param named after a built-in replaces the built-in for the call —
in the body and, by dynamic scope, in every fn the body calls — and the
check pass does not model it; `unpack` replaces it at the top level for
good. A param may even be named `if`, `def` or `true` (`def g fn
[[def:Integer] [Integer] [ 1 ]] print (g 4)` prints `1`).

**Proposed verdict:** resolve by fix — one admission rule for the four
binders. Either a param and an `unpack` name raise `reserved_word` as
`def` and `var` do (this record's first verdict), or a scoped binder may
shadow a built-in and `var` joins the params; a top-level `unpack` should
raise on either reading.

---

## NUR381 — a container literal a `var` or `=>` body leaves as its value is evaluated after the body's names are unbound {#nur381}

**Status:** Pending · **Recorded:** 2026-10-05 (found 2026-10-02 in the voxgig-boru/trie migration) · **Restated:** 2026-11-27 — re-measured on the interpreter, where the shape is wider than first recorded: neither the `if` arm, the fold nor the map is needed · measured at d1db0f8 · **silent** with a same-named outer binding, `undefined_word` without one

**Rule:** a name a scope form binds is in scope for the whole of its
body, a literal the body leaves as its value included. A `fn` body keeps
it: `def x 100  def f fn [[x:Integer] [Any] [[x]]]  f 5` is `[5]`.

**Divergence:** a `var` body and a `=>` body do not. The literal is still
pending when the names are unbound, and its members are evaluated by
whatever takes it next (a loop's region, the end-of-run sweep), where the
name resolves to a same-named outer binding or to nothing:

```
def x 100  5 var [[x] [x]]                      [100]        (`[5]` is intended)
5 var [[x] [x]]                                 [boru/undefined_word] undefined word: x
def x 100  def l ([x:Integer] => [[x]])  l 5    [100]
def l ([x:Integer] => [[x]])  l 5               [boru/undefined_word] undefined word: x
def x 100  5 var [[x] x {n: (x add 1)}]         5 {n:101}    (the bare read sees the var; the literal does not)
print "before" 5 var [[x] print "body-start" [(print "in-literal" 1)] print "body-end"]
                                                prints before, body-start, body-end, in-literal
```

A literal a word takes inside the body is evaluated in scope (`def x 100
5 var [[x] print [x]]` prints `[5]`, and `def y [x] y` answers `[5]`),
which is why the usual callback shapes work. The program this record was
found on:

```
def acc {n: 100}
def count fn [ [xs:List] [Map] [
  {n: 0} xs [ var [[x acc] if true [ {n: ((acc "n" get) 1 add)} ] [acc] ] ] fold
] ]
print (count [7 8 9])
  boru <file>     {"n": 101}       (`{"n": 3}` is intended)
  without the top-level `def acc`:
                  fold: step 0: [boru/undefined_word]: undefined word: acc
```

`boru check` agrees with the run on the bare shapes — it refuses `5 var
[[x] [x]]` with the same `undefined_word` and is silent when an outer `x`
exists — but is clean on a literal inside an `if` arm, where the run
raises: `5 var [[x] if (x gt 0) [ [x] ] [ [0] ] ]` is `undefined_word` on
the run and 0 errors on the check, and so is the fold program without its
top-level def.

**Proposed verdict:** resolve by fix — the `var` body's cleanup and the
`=>` frame's close collect a pending literal before the names go, as the
`fn` frame's does (core/go/CLAUDE.md, "Container literals are sealed
regions of the tape": a pending literal "evaluates where a word takes it,
where a loop's region or a frame's cleanup collects it, or at the
end-of-run sweep"); the check pass follows.

---

## NUR384 — a name a fn-body `unpack` binds outlives the call when the body names no other binding word {#nur384}

**Status:** Pending · **Recorded:** 2026-10-08 · **Narrowed:** 2026-11-27 (the VM's `undefined_word` went with the compiled lane; the interpreter's side is re-measured, and it disagrees with itself and with the check pass) · measured at d1db0f8 · **silent**

**Rule:** a binding made in a fn body is local to the call — `def g fn
[[d:Map][Integer][def b 7 d.a add b]]  g {a:1 b:2}  b` raises
`undefined_word` at the last `b`, and with an outer `def b 9` answers
`8 9` (lang/spec/fn-locals-scope.tsv).

**Divergence:**

```
def g fn [[d:Map][Integer][unpack [a b] d  a add b]]  g {a:1 b:2}  b
  boru do          3 2      (`b` is still bound after the call)
  boru check -e    1:68: [error] undefined_word: undefined word: b
def b 9  def g fn [[d:Map][Integer][unpack [a b] d  a add b]]  g {a:1 b:2}  b
  boru do          3 2      (the outer `b` stays rebound)              check: 0 errors

def g fn [[d:Map][Integer][def z 0 unpack [a b] d  a add b]]  g {a:1 b:2}  b
  boru do          [boru/undefined_word]: undefined word: b            check: the same
def b 9  def g fn [[d:Map][Integer][def z 0 unpack [a b] d  a add b]]  g {a:1 b:2}  b
  boru do          3 9      (the outer `b` is restored)                check: 0 errors
```

The same `unpack` is call-local or not by whether the body happens to
name a binding word. A body naming none is a leaf frame and takes no
cleanup snapshot, and `unpack` binds from Go, outside that rule's sight:
`frameStateWords` (`core/go/fn_capture.go`) leaves it out on purpose, to
agree with the VM that has since been removed. The def-bound lambda
(`def g ([d:Map] => [unpack [a b] d  a add b])`), the fn value applied
through `each` (`each g/v [{a:1 b:2}]  b` is `[3] 2`) and through `apply`
(`{a:1 b:2} g/v apply  b` is `3 2`) leak alike. As a file the first
program is refused by the pre-flight check and answers `3 2` under
`-no-check`.

**Proposed verdict:** resolve by fix — list `unpack` among the
frame-state words, so a fn-body `unpack` is call-local on every frame as
`def` is and as the check pass already reads it; or rule the leak the
discipline, and make the frame with a binding word and the check pass
agree with it.

---

## NUR385 — a disjunct is canonicalised only when resolution rebuilt an alternative {#nur385}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `scripts/core-probe.sh` (the core/spec notation) · silent

**Rule:** a value's canonical form is a function of the value (ADR-015): two
disjuncts with the same alternatives render the same.

**Divergence:** `resolveWordsDeep` (core/go/resolve.go) re-simplifies a
disjunct — duplicates dropped, an alternative absorbed by a wider bare type
dropped, the rest put in the total order — only "when an alternative
changed", and decides "changed" with `ExactEqual`, which for a container is
IDENTITY (compare.go `sameContainer`). A scalar or a type literal comes back
as itself, so the disjunct is left as written; a non-empty plain list or a
plain map is rebuilt, so the same disjunct with one more alternative is
reduced. Two empty lists count as identical, so an empty list does not
trigger it either.

```
run d( 5 5 Integer )            5 tor 5 tor Integer        as written: duplicate kept, unordered
run d( [q 1 ] 5 5 Integer )     Integer tor 5 tor [1]      reduced and ordered
run d( [q ] [q ] 1 1 )          [] tor [] tor 1 tor 1      an EMPTY list does not trigger it
run d( {q } {q } 1 1 )          1 tor {}                   an empty map does
```

The source-level constructor `tor` simplifies always, so the shape is
reachable through the paths that build a disjunct raw (`NewDisjunct`: the
`{a?:T}` desugar, a stored shape, a host). The Lean specification models
the rule as the kernel has it (`spec/Boru/Core/Unify.lean`
`disjunctAltRebuilt`).

**Proposed verdict:** resolve by fix — simplify whenever a disjunct is
resolved, so the canonical form does not depend on container identity.

---

## NUR386 — the kernel's `Unify` of two disjuncts is their first alternative, not their meet {#nur386}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go (`UnifyR`) and through `boru do` · the VALUE half only; the predicate is right

**Rule:** one unifier — the meet of two values has one answer, and `v` unified
with `v` is `v`.

**Divergence:** `unifyDisjunct` (core/go/unify_disjunct.go) returns the
result of the first alternative that unifies with the other side. When the
other side is itself a disjunct that result is one alternative, so the meet
of a disjunct with itself is its first member, where `tand` — which goes
through `TandValues` — answers the disjunct:

```
UnifyR(Integer tor String, Integer tor String)   Integer
UnifyR(5 tor 6, 5 tor 6)                         5
boru do '(Integer tor String) tand (Integer tor String)'    Integer tor String
```

The same walk gives the EMPTY disjunct no alternative to match, so it
unifies with nothing but `Any` — not with itself, and not with `Never`,
though both denote the empty set and `Never` unifies with `Never`:

```
run unifyq d( ) d( )      false        (pinned in core/spec/unify.tsv)
UnifyR(d(), Never)        fails
UnifyR(Never, Never)      Never
```

Pinned as the kernel has it by the Lean specification
(`spec/Boru/Core/Theorems/UnifyOrder.lean`:
`disjunct_meet_is_first_alternative`,
`unifies_refl_except_empty_disjunct`).

**Proposed verdict:** resolve by fix — a disjunct against a disjunct is the
disjunct of the pairwise meets (what `TandValues` already computes), and the
empty disjunct folds as `Never`.

---

## NUR387 — an unmatched `(` inside a container literal closes on the literal's sealing paren {#nur387}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `scripts/core-probe.sh` · loud on both sides, the CODE differs

**Rule:** the kernel receives values, not text: an open paren with no close
is a scan boundary, and the dispatch that met it fails on its own terms
(core/spec/dispatch.tsv, "an UNMATCHED `(` is a scan boundary, not a syntax
error"). The same tokens answer the same wherever they stand.

**Divergence:** a container literal is evaluated as a sealed region of the
tape — its elements between a paren pair of the region's own
(core/go/sealed.go). An unmatched `(` among the elements therefore HAS a
close: the region's. The inner group collapses, the region is left
unclosed, and the error is the region's rather than the word's:

```
run negq (              ERROR:signature_error
run [ negq ( ]          ERROR:no_value_error      the stolen pair is an EMPTY group, blamed on negq
run negq ( 5            ERROR:signature_error
run [ negq ( 5 ]        ERROR:syntax_error        negq fires over (5); the region never closes
```

The Lean specification models the region with its real paren pair, so it
reproduces all four (`spec/Boru/Core/Engine.lean` `runSealed`).

**Proposed verdict:** resolve by fix — the region's close should pair only
with the region's open (a `LoopOpenInfo` close matched by identity, not by
nearest open paren), so an unmatched `(` in a literal is the scan boundary
it is at the top level.

---

## NUR388 — a function value refused at arrival is re-planned forever {#nur388}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `scripts/core-probe.sh` · loud, but as the wrong error and only at the step limit

**Rule:** arity and origin never change function behaviour (ADR-016): a
function called through its word and the same function called as a value
answer the same. And a plan never claims what the arrival will decline
(core/go/collect_kernel.go, the type-literal guard's own comment).

**Divergence:** two things meet. The per-candidate scan admits a forward
literal when `SigArgMatches(...) || expectedType.Equal(TAny)`, so a
TYPE-ARG slot declared `Any` is planned over a concrete value that the
arrival decision (`CollectArrival`, `SigArgMatches` alone) then declines.
For a WORD that refusal ends in a stack-only re-dispatch and a
signature_error. For a function VALUE `curryOrStack` only moves the pointer
back onto the value — "a value callee re-steps and re-plans in full" — so
it plans the same forward, is refused the same way, and never leaves:

```
run tyq 'x'                 ERROR:signature_error      the word
run tyq/v 'x'               fn [[t:Any][Any][]] 'x'    the value, placed
run ( tyq/v 'x' )           ERROR:evaluation_limit     the value, re-stepped by the collapsing group
run bothq 7 tyq/v 2         ERROR:evaluation_limit     the value, re-stepped as bothq's result
```

The Lean specification reproduces the loop — it has the same two rules —
and runs out of fuel on the same programs (`spec/Boru/Core/Match.lean`
`scanAdmits`, `spec/Boru/Core/Engine.lean` `curryOrStack`).

**Proposed verdict:** resolve by fix, on both halves — the scan's `Any`
escape should not apply to a type-arg slot, and a value callee whose
collection was refused should be sealed to the stack for its re-step, as a
completed one already is.

---
