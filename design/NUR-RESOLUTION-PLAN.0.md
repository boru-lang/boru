# NUR Uniformity Resolution Plan (Expanded Working Draft)

> **Status:** Living engineering document.
>
> This is intentionally **not a summary**. It is the current working
> plan derived from the design discussions and is intended to become
> the implementation roadmap. Decisions recorded here were applied to
> `NUR.md` on 2026-07-31 (see the applying commit); where an item
> below and the register disagree, the register is now current and
> this draft is the history.

## Project assumptions

- boru is under active development.
- There are no production users.
- Uniformity and architectural consistency take precedence over
  backwards compatibility.
- Each active NUR will end in one of four states:
  - **Allowed by design**
  - **Modify**
  - **Resolve by fix**
  - **Postpone / Investigation**
- Every decision is checked against `ADR.md`.
- Where an ADR is incomplete, the ADR should be refined rather than
  allowing implementation drift.

## Numbering reconciliation note (2026-07-31, applied with this draft)

The decisions below for **NUR-014 through NUR-018 do not correspond to
the records currently carrying those numbers in `NUR.md`** (the file's
NUR014 is cross-leaf numeric magnitude equality; NUR015–017 are
retired numbers; NUR018 is the Store/Error `make` exclusion). Those
five decisions therefore were **not** applied to the register and are
retained here, unmapped, for maintainer re-numbering:

- "NUR-014: Retire — superseded by insertion-order Map and SortedMap"
  — matches no current record.
- "NUR-015: Keep — default invariance; explicit variance only where
  provably safe" — matches no current record (variance has no NUR).
- "NUR-016: Allowed — Boolean arithmetic remains illegal; diagnostics
  should recommend the logical operators" — this is the file's
  **NUR000**, already Allowed with that substance; the
  diagnostics-recommendation refinement is noted there via
  `design/TRUTHINESS.0.md` §6.
- "NUR-017 / NUR-018: only lossless numeric widening implicitly;
  narrowing always explicit; the same model uniformly for
  user-defined numeric types" — matches no current record; likely a
  future NUR/ADR on numeric widening.

Every other item below mapped cleanly and was applied.

## Cross-cutting design work

### Truthiness design document — DONE

A dedicated design document was required because truthiness spans
multiple NURs. It exists now: **`design/TRUTHINESS.0.md`**, defining
the single truthiness model, boolean coercion, conditionals, the
operand-returning connectives, short-circuit/evaluation-order
semantics, the typing interaction, the complete consuming-construct
registry, examples, and rationale.

**Architecture note:** a new ADR should define **One Truthiness
Model** as a language principle (ADR candidate 1 below; added to
ADR.md only on explicit maintainer instruction).

## Per-record decisions

### NUR-001 — Allowed by design

Current rationale remains valid. Additional work agreed (now done):
the Truthiness design document exists; the ADR is a recorded
candidate; every truthiness consumer is documented
(`design/TRUTHINESS.0.md` §7), with a standing obligation covering
future conditional constructs.

### NUR-002 — Rewrite (applied)

The wording was too Boolean-specific. The general rule is now the
record's own statement:

> Exhaustive coverage of any finite domain does not require a default
> branch.

Boolean is simply a built-in two-value pseudo-enum; enums are
specialisations of disjunct types; documentation should explain finite
disjunct exhaustiveness rather than treating Boolean as special.

Dependent-types discussion (recorded in the register as follow-on
work): finite dependent scalar types also define finite domains.
Ergonomics proposal — declare finite dependent types by enumerating
values (`{2,3,4}`) rather than forcing range predicates
(`Integer >=2 and <=4`). Implementation investigation — statically
known finite sets should avoid materialisation (symbolic/range
representations); free variables require symbolic representation
regardless.

### NUR-003 — Allowed (applied)

The truthiness document covers the operand-returning behaviour of
`and`/`or`, short-circuit semantics, evaluation order, the returned
operand, and the type-checking interaction.

### NUR-004 — Allowed (applied)

Clarified in the record: the distinction between the lattice subtype
hierarchy and value-level finite sets, and why Boolean's values belong
at the value layer rather than as structural lattice leaves.

### NUR-005 — Rewrite wording (applied; now Allowed)

Clarified: the general scalar rule is same-type arithmetic;
`String` `add` is the **sole language-level exception**; references to
Atom and Bytes are documentary comparisons, not architectural
groupings. REFERENCE.md now states the exception at the rule.

### NUR-009 — Architectural remediation (recorded; stays Pending)

Bytes exposes a deeper ownership problem. Proposed architectural rule:
all globally visible descendants of `Node` or `Scalar` belong in
`eng`; core modules (`boru:*`) may define module-owned descendants;
`lang` must not define additional global Node/Scalar descendants
except through an explicit NUR. Likely migrations: Bytes, Time, Date,
DateTime, Instant; review remaining scalar descendants individually.
New ADR required describing ownership of the kernel type hierarchy
(ADR candidate 2).

### NUR-010 — Resolve by fix — **RESOLVED (this session)**

`pow` with a negative exponent now returns a coded error
(`[boru/arith_error]: pow: negative exponent -1`, with a Float hint)
instead of an uncoded runtime error. Record deleted from the register
per the deletion discipline; the resolving commit names NUR010.

### NUR-011 — Modify (recorded for later design)

Introduce a complete equality family:

- **`eq`** — convenience equality: scalars by value, compounds by
  identity (current behaviour).
- **`deq`** — deep structural equality (current behaviour).
- **`req`** — reference equality: pointer identity only, uniformly
  for compounds and scalars (NEW; to be designed).

This separates three notions many languages conflate. Performance
note: Bytes equality may be O(n); `req` gives constant-time identity.
Documentation should compare with JavaScript, Python, Ruby, and the
Lisp family. (Applied to the register's NUR011 record on 2026-07-31;
the `req` design travels with the equality work re-opened under
NUR031.)

### NUR-012 — Resolve by fix — **RESOLVED (this session)**

Natural total ordering implemented in `comparePathons`
(`eng/go/compare_scalar_behaviors.go`): (1) drive/volume, (2) absolute
before relative, (3) forward lexical segment comparison, (4) shorter
prefix first — `sort` now yields the filesystem-walk order
(`/e a a/b/c a/z b/a`). The `add` join was verified to prevent
duplicate separators by construction (segments are structural) and to
refuse an absolute right operand loudly; both pinned in
`lang/spec/scalar-micron-ops.tsv` §8 and `lang/spec/micron.tsv` §7.
Docs updated (REFERENCE.md §Ordering, design/TYPE-ORDERING.10.md).
Record deleted. Investigations retained (not commitments): anchor
removal operators, `mod`, HTTP module interaction, parameterised
Pathon/Urlon route-parameter types (speculative).

### NUR-013 — Investigation (recorded; stays Pending)

Compare boru behaviour with IEEE-754 `totalOrder`; conform where
practical.

### NUR-020 — Allowed (applied)

`print` remains in core; the argument is now written down: it
supports the expected "Hello World" learning experience and matches
common language expectations.

### NUR-022 — Resolve by fix (recorded; stays Pending)

Bring `del` into symmetry with `set`. First investigation: confirm
boru distinguishes absent key from present-key-with-`none`. A separate
**sentinel values** design programme is opened (globally unique
singletons; user- and system-defined sentinels; interaction with
containers, equality, and option-like APIs) — its own design document,
because it potentially affects many facilities. NUR-022 must not
depend on that investigation.

### NUR-023 — ADR refinement (recorded; stays Pending)

ADR-004 is incomplete. It should describe barrier positions, argument
handling categories, stack-only behaviour, and the chaining rationale.
Diagnostics should explain why words occupy different categories
rather than merely reporting failure. (ADR candidate 4.)

### NUR-024 — Allowed (applied)

Two distinct orderings, stated explicitly: **semantic** (`cmp`, `lt`,
`gt` — reject meaningless comparisons) and **deterministic** (`tcmp` —
implementation purposes such as deterministic signature ordering).
Both the NUR record and an ADR should explain the separation (ADR
candidate 5).

### NUR-025 — Documentation fix — **RESOLVED (this session)**

REFERENCE.md §Comments now documents the real forms (`#`, `//`,
`/* */`) and explicitly states `## ##` does not exist; the new
`lang/spec/comments.tsv` pins every form, the `##`-is-a-line-comment
behaviour, and the unterminated-`/*` syntax error. Record deleted.

### NUR-026 — FIX (root cause + decision recorded; stays Pending)

**Root cause (2026-07-31):** the escape-set divergence between quoted
strings and templates is an implementation accident. The backtick was
deleted from jsonic's `StringChars`/`MultiChars` so templates could
carry `${…}` interpolation, forcing a hand-rolled template scanner
whose `processTemplateEscapes` is a minimal six-case switch — never
brought to parity with jsonic's full escape set that quoted strings
still ride.

**Fix decision:** do not use the jsonic string lexer as-is. Use a
**custom unified string lexer** — a vendored copy of jsonic's string
lexer extended to handle backtick templates and interpolation. One
lexer: full escape set across all string-literal forms, one escape
vocabulary in one place, correct template parsing. Retires
`processTemplateEscapes`.

## The NUR-029 split (applied)

NUR-029 was an umbrella over seven sibling-form divergences (G8–G13b,
`design/BORU-SHARP-EDGES.0.md`). Re-verified 2026-07-30: **G8, G11,
G13a no longer reproduce** (fixed by unrelated work — the design note
now says so); **G9, G10, G12, G13b still reproduce**. The umbrella was
split into four per-item records:

- **NUR048 (G9)** — `case` DEFAULT arm mis-collects. **FIX —
  RESOLVED (this session)**: a match-position word bound to a function
  value (provably never a genuine match) now heads an open-call
  default arm that runs isolated with the case value pushed first,
  exactly like a matched arm (`caseDefaultStart`/`isCaseOpenCallHead`,
  mirrored in the checker, desugar, and exhaustiveness passes; pinned
  in `lang/spec/case.tsv` §7). Record deleted.
- **NUR049 (G10)** — `(dot message)` receiverless in an `error`
  handler. Root principle: the paren barrier is one-directional
  today. **FIX**: make it symmetric — a group completes from its own
  contents only; `(dot message)` then fails deterministically and
  statically. Also fix `design/examples/apps/todo-tui-client.boru`
  and test its error arms. Compatibility check before landing:
  sanctioned point-free patterns that consume enclosing stack values
  need an explicit alternative (e.g. `$`-receiver forms) if they
  exist.

  *(Superseded 2026-08-02 — see the register, which is current. The
  one-directional premise is RETRACTED: the group is already sealed
  dynamically in every probed context, so there is no backward reach to
  close, and the record was retitled. What remains is that the failure
  is not STATIC — `error` handler bodies are wholly unchecked — plus
  the engine diagnostics that recommend the broken parenthesised
  spelling, and two shipped examples to repair
  (`todo-tui-client.boru` and `design/examples/todo/audit.boru:29`).
  The compatibility sweep came back clean: no sanctioned point-free
  pattern relies on backward reach.)*
- **NUR050 (G12)** — `/r`-parked fn vs `Function` param. **FIX —
  RESOLVED (this session, ADR-011)**: the repro was pinned (a
  collection-barrier failure plus a checker misdiagnosis, not a
  runtime type mismatch), option 2a landed — `Word/__FN` collapsed
  into `Type/Function` (one function type; FixedID 23 retired; TS
  twin in lockstep) — and `/r`-marked words now feed forward
  collection as references. `wa {x:1} some-fn/r` dispatches;
  module-export refs answer `is Function`; `/N` reaches through
  dotted wrappers; the `__FN` diagnostic leak is gone. Option 1
  (force a reference annotation) stayed rejected. Record deleted.
- **NUR051 (G13b)** — a type-literal map value refuses to
  bytecode-compile ("body result of unknown provenance"). Root
  cause: the emitter had no provenance representation for a bare
  type node in data position. **FIX — RESOLVED (this session)**:
  `RecordMakeMap` / `recordMakeListInner` now intern a bare type node
  member as a canonical-ID type operand (`OpPushType`) instead of
  refusing; `{r: None}`, `[a None]`, `{k: Integer}` compile exactly
  as they interpret (parity rows in `lang/spec/bytecode-migrated.tsv`;
  the `None` → `none` workaround guidance is retired). Record
  deleted. `0 eq Integer` → false remains correct and is not evidence
  against types-as-values (`Integer eq Integer` → true — singleton
  value, structural equality).

## Proposed ADR-010 — Types are values (added to ADR.md as Proposed)

A type literal is a first-class **singleton value** everywhere; all
layers (interpreter, checker, compiler) honour it identically; a
compiler refusal on a type-literal-as-value is a bug; the emitter must
give bare type nodes interned operand identity in data position. Full
text in `ADR.md` §ADR-010. Motivated by NUR051; reinforced by NUR050.

## Review status of NUR-030 upward

The committed register marked NUR-030 through NUR-047 **Allowed**; for
this review all were re-opened for fresh consideration on their
merits, and the register was updated where the review's verdict
differs. Outcomes:

- **NUR-030** (`group` render-key fold) — re-opened, UNRESOLVED. The
  root cause is Map keys being rendered strings. Maintainer proposal
  to explore: String-only grouping keys. Alternatives: (a) status
  quo, (b) String-only keys, (c) grouped pairs. Deeper question:
  Map-key identity language-wide.
- **NUR-031** (opaque-value equality) — re-opened **in part**. The
  Store/Error/Timeout/Interval resolutions stand; Function/FnDef/Word
  dispatch-rejection is accepted as current behaviour;
  **Module self-inequality is an open defect** (silent reflexivity
  violation). The NAMESPACE half resolved by construction with the
  NUR038 facet refactor (a namespace is a plain Map: `M eq M → true`);
  the `Ideal/Module` DESCRIPTOR half remains open.
  Standing requirement: every value — functions and modules included
  — falls under equality, at minimum reflexive. Mechanism awaits
  NUR050/ADR-010 and the Behavior-routing ADR; track together.

  *(Superseded 2026-08-02 — see the register, which is current. The
  namespace half landed 2026-08-01 in commit `d8f93d3`, not 2026-07-31;
  the DESCRIPTOR half is RESOLVED as of 2026-08-02; and NUR050 is
  resolved and retired, so there is nothing left to track alongside.
  What NUR031 still tracks is Function/Word identity — including the
  namespace `deq` residue, since a namespace exporting functions is not
  `deq`-reflexive.)*
- **NUR-037** (fn-local fn undefined when compiled) — re-opened as an
  open defect. Mechanism: closure-capture gap (distinct from G12's
  type identity, same first-class-values family). The compiled run
  disagrees with the interpreted one, so this is a correctness defect
  sitting on top of a compile gap that is already a defect in its own
  right: valid code must compile, and every refusal is an
  unimplemented or unproven case owed a fix. Fix: capture enclosing
  local fn bindings. A check-time refusal is at best interim
  containment — it trades a wrong answer for a known gap, and closes
  nothing.

  *(Framing corrected: this item previously appealed to a "slow, not
  wrong" standard. That doctrine is withdrawn — failing to compile is
  a failure, not a tolerated outcome, so a refusal here would not
  discharge the record.)*
- **NUR-038** (module-export statement misfire) — **RESOLVED
  2026-08-01** (record deleted; number retired). Two halves landed:
  (1) the facet refactor proposed here (2026-07-31): `Ideal/
  ModuleExport` (FixedID 5001) retired; `import` binds a plain export
  Map carrying the kernel `ModuleNSInfo` facet, exports ride raw, the
  wrapper-masking bug class closed (NUR031's namespace-equality half
  resolved by construction). The retest then proved the original
  root-cause hypothesis wrong — the misfire reproduced with a
  `def`-bound plain map (no modules) — and traced the true mechanism:
  a COMPLETED forward collection re-steps its callee stack-only via
  the `/s` word rewrite, which a function VALUE cannot carry, so the
  re-plan reopened the window forward-first and swallowed the next
  statement. (2) The engine fix (2026-08-01): a one-shot stack-only
  SEAL for completed value-called collections (`Engine.sealFnValue`),
  the reach-collapse CALL tag + arrival gate (an unmarked dot-read fn
  that would collect is a barrier like its bare-word twin; explicit
  data spells `/r`; 0-arg-only fns are property calls dispatching in
  place — the NUR035 deferral narrowed accordingly), and ReachGroup
  provenance preserved through `stepOpenParen`. The claim tests are
  sig-aware, so arity widening (`concat parts {sep}` — the wider
  overload's swap slot) and higher-order Function operands
  (`usurp (m dot a)`) keep their pre-fix dispatch. Pinned in
  `lang/spec/fn-value.tsv` §6 + `lang/go/test/nur038_seal_test.go`;
  the `end` house rule is retired as a requirement
  (`utils/README.md`).
- **NUR-039 through NUR-047** — re-examined; no decision recorded in
  this review differs from the file, so their Allowed records stand.

## Additional ADR candidates

1. **Truthiness** — One Truthiness Model as a language principle
   (`design/TRUTHINESS.0.md`).
2. **Ownership of the global Node/Scalar hierarchy** (NUR009).
3. **Semantic ad-hoc polymorphism / generic function philosophy** —
   with references to CLOS, Julia, Rust traits, and Swift protocols
   as inspiration.
4. **Barrier positions and argument handling** (the ADR-004
   refinement, NUR023).
5. **Semantic vs deterministic ordering** (NUR024).
6. **Module provenance as a Value facet** (NUR038) — ADR-level,
   recorded in its NUR record; **implemented 2026-07-31** (wrapper
   retired, facet live), awaiting explicit maintainer instruction
   before an ADR entry is written. (The companion "one Function type"
   candidate became **ADR-011**, Accepted, with NUR050's resolution.)

Per the ADR.md rule, none of these becomes an ADR entry until the
maintainer explicitly instructs it; ADR-010 was so instructed and is
recorded as **Proposed**.
