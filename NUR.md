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
| [NUR389](#nur389) | A source module's owner token is unique only among its importer's modules: a module body's `def` on a core word over a type of a module it imports is admitted — and replaces that module's own tuple — when the two hold the same import ordinal, and is `extend_owner` otherwise (always, for a program) | the `Registry.ModuleScope` removal: checking what enforces the rule the flag was for (2026-11-27) |
| [NUR390](#nur390) | A `/uv` value is not collected by a waiting word, where a `/v` value is: `bothq 1 negq/uv` strands the collection, and `bothq negq/uv 1` silently binds the two the other way round | the Lean core specification, modelling `/u` (2026-11-27) |
| [NUR391](#nur391) | A word with more than one definition is not `eq` to itself: every reference to it (`f/v`) is a new function, so `f/v eq f/v` is false once `f` has an overload | the Lean core specification, modelling function identity (2026-11-27) |
| [NUR392](#nur392) | An unmatched `(` is refused at the top level and in a container literal, collected as a value in a driven loop's body, and handed to the return check in a function's body | the Lean core specification, modelling driven loops (2026-11-27) |
| [NUR393](#nur393) | Typed containers are `eq` by their child constraint alone: `[:Integer 1] eq [:Integer 2]` is `true` (`deq` is `false`), and `[:Integer 1] tor [:Integer 2]` keeps one of them | the core/ts unifier port, differential against core/go (2026-11-27) |
| [NUR394](#nur394) | A disjunct held inside another value is compared as it prints: two alternatives that render alike are one, though they differ in structure or hold two different functions | the core/ts unifier port, differential against core/go (2026-11-27) |
| [NUR395](#nur395) | The total order is not transitive between a typed list and plain lists of its size, so a disjunct's canonical form depends on the order its alternatives were written in | the core/ts unifier port, differential against core/go (2026-11-27) |
| [NUR396](#nur396) | A function a function's body hands back as its only result is refused as an uncalled call: `def pk1 fn [[x:Map][Any][x.f]]` over `{f: add/v}` raises `uncalled_function` where `m.f` at the top level is the function | the core/ts engine port, checked against core/go and the Lean model (2026-11-27) |
| [NUR398](#nur398) | A word defined in boru is not retried from the stack when its forward plan fails, where the same function written as a literal is, and the same word under a forced arity is: `"k" [] ov 5` is `signature_error`, `"k" [] ov/2 5` is `1 5` | the Lean specification's differential fuzzer over overloaded words, against core/go (2026-11-27) |
| [NUR400](#nur400) | A full-stack word standing directly in a loop's round or an `if`'s condition ends the construct: `for 3 [depth]` runs one round and answers `0`, with no error, and leaves the index bound (`for 3 [depth] i` is `0 0`); `if [depth] ["yes"] ["no"]` takes neither branch and answers `0` | the Lean specification of the counted loop and of the `if` continuation, checked against core/go (2026-11-27) |
| [NUR401](#nur401) | A function value with a narrower and a wider definition drops the argument it collected when the next value fits one slot of the wider definition but the wider definition does not fill: `m.g 5 6` is `5 106` where the word `f 5 6` is `105 6` | the Lean specification's differential fuzzer over `/u` on overloaded words, against core/go (2026-11-27) |
| [NUR402](#nur402) | A counted loop's index wraps past the Integer range and the loop runs on: `for [9223372036854775806 9223372036854775807 2] [i]` runs to the step limit, where `9223372036854775807 add 1` is `integer_overflow` | review of the Lean specification of the counted loop, checked against core/go (2026-11-27) |
| [NUR403](#nur403) | An empty list built over a nil slice is not a list to the kernel's readers: `NewList(nil)` and `NewList([]Value{})` render alike and compare equal, but as a loop's body the first is ONE token (itself) and the second none | the Lean specification's differential fuzzer over call regions, against core/go (2026-11-27) |
| [NUR404](#nur404) | A function with an unnamed parameter drops a value its body COMPUTED: `def f fn [[Integer][String][drop "a" "b"]]` answers `b` for `f 5`, with no error, where the same surplus in a function of named parameters is a `type_error` | the Lean specification's model of the frame's argument span, against core/go (2026-11-27) |
| [NUR405](#nur405) | The frame-pop word `__pa` is still registered though no frame has emitted it since 2026-10-08, and a program can call it: in a function's body it pops the frame's arguments and capture baseline, and the frame's own close then pops the ENCLOSING call's — `args` there answers "not inside a function", inside a function | the Lean specification's model of the args list, against core/go and the shipped words (2026-11-27) |
| [NUR406](#nur406) | A function whose body unbinds its own parameter unbinds its CALLER's binding of that name when it closes: with `def g fn [[n:Integer][Integer][undef n 7]]`, a caller `fn [[n:Integer][Integer][g 2 drop n]]` given `5` reads the global `n` after the call, and at the top level `n` is undefined after `g 5`; `boru check` reports nothing | the Lean specification's model of the frame's close, against core/go and the shipped words (2026-11-27) |
| [NUR407](#nur407) | Tail-call elision changes an answer: a callee that unbinds its own parameter reads the caller's binding of that name — unless the call is the last thing the caller does, when the caller's is already unbound: with `def n 8 ; def g fn [[n:Integer][Integer][undef n n]]`, `fn [[n:Integer][Integer][g 7]]` given `5` answers `8` and `fn [[n:Integer][Integer][g 7 add 0]]` answers `5` | the Lean specification's machine, which nests every call, against core/go (2026-11-27) |
| [NUR408](#nur408) | A parameter or a return declared as the VALUE `none` is no constraint at all: `def f fn [[x:none][Integer][7]]` takes `5` and `"a"`, and `fn [[x:Any][none][x]]` returns anything, where `x:7` takes only `7` and a return `[7]` only `7` | the Lean specification's model of the function-spec reader, against core/go and the shipped words (2026-11-28) |
| [NUR409](#nur409) | A parameter written WITHOUT a name takes fewer declarations than one written with a name: `[1.5]`, `[(Integer tor String)]`, `[none]`, `[[1 2]]` and a lowercase name bound to a type are each `function spec: invalid parameter` (or `unknown type`), where `x:1.5`, `x:(Integer tor String)`, `x:none`, `x:[1 2]` and `x:t` are read | the Lean specification's model of the function-spec reader, against core/go and the shipped words (2026-11-28) |
| [NUR410](#nur410) | `?` on a parameter whose type has no base value does nothing, silently: `def f fn [[n?:Any][Any][n]]` is defined without complaint and `f` is `no signature matches`; the same for `Function`, a union, and a literal the base value is not (`n?:7`) | the Lean specification's model of the definition install, against core/go and the shipped words (2026-11-28) |
| [NUR411](#nur411) | A lowercase name bound to a union or a typed list is accepted as a parameter's or a return's declaration and constrains nothing: with `def u (Integer tor String)`, `fn [[n:Any][u][n]]` returns `true` and `fn [[n:u][Any][n]]` takes it, where the union written in place, and a capitalised `def U`, both refuse it | the Lean specification's model of the function-spec reader, against core/go and the shipped words (2026-11-28) |
| [NUR412](#nur412) | A function VALUE all of whose parameters are unnamed takes its stack arguments BOTTOM-UP — and so an unbound function literal of unnamed parameters cannot be called forward at all (`fn [[Integer String][List][args]] 5 "a"` is left uncalled, where `x:`/`y:` or a `def` makes it `[5 'a']`): `7 "a" fn [[Integer String][List][args]]` is `[7 'a']` and `"a" 7 fn […]` is not called, the opposite of the same function with named parameters and of the same function called by name; and a usurped one matched that way is not run — `def f fn [[Integer String][List][args]] ; "a" 7 f/u` returns its own argument (`<fn>: expected List, got Integer`) | the Lean specification's differential fuzzer, against core/go and the shipped words (2026-11-28) |
| [NUR413](#nur413) | `/u` does not carry a signature's raw-capture slots with the parameters it reverses: the wrapper drops the two no-evaluation slots, so a code body is EVALUATED before the word is handed it (`quote/u [add 1 2]` is `[3]`, `for/u [print i] 2` is `undefined word: i`, `if/u [print "no"] [print "yes"] true` prints both), and keeps the raw-paren and form slots at the positions they had before the reversal, which are other parameters' positions now | the Lean specification's model of the raw-capture slots, against core/go and the shipped words (2026-11-28) |
| [NUR414](#nur414) | A paren expression that ARRIVES at a waiting word — a name bound to one, stepped — is captured as code when the word's signature has a raw-paren or form slot ANYWHERE, also at an ordinary slot of it; written at that same slot it is evaluated, because the plan asks the position and the arrival asks the signature | the Lean specification's model of the raw-capture slots, against core/go (2026-11-28) |
| [NUR415](#nur415) | The one word that takes a data splice's MARKER where every other word takes its values — the binder, so that `def y w` makes `y` the same splice — is known by its NAME: `def w word [1 2] ; def y w ; y` is `1 2`, and with `def mydef def/v`, `mydef y w ; y` is `2 1` — `def`'s own function under another name binds the first value and leaves the second | the Lean specification's model of splice markers, against core/go and the shipped words (2026-11-28) |
| [NUR416](#nur416) | A splice marker has no canonical form: `canon` of one is the engine's debug dump, `word()({[1 2]})`, which no parser accepts (ADR-015), with the payload in the engine's value print (`word(x)` for a word); core/ts prints `word(undefined)` for the same value | the Lean specification's model of splice markers, against core/go, core/ts and the shipped words (2026-11-28) |
| [NUR417](#nur417) | An unapplied function value of no parameters is CALLED where it is stepped if `fn` made it and is DATA if `=>` made it: `(fn [[] [Integer] [42]])` is `42` and `([] => [42])` is the function, so `def a (fn [[] [Integer] [42]]) ; a/v` is `42` — the name is bound to the result — where `def a ([] => [42]) ; a/v` is the function; ADR-016 lists the difference as a defect, and `apply` was made uniform while the difference itself stayed | the Lean specification's model of functions of no parameters, against core/go and the shipped words (2026-11-28) |
| [NUR418](#nur418) | Unbinding a type does not release its name: `def Foo 1 ; undef Foo ; def Foo 2` is `type_error: name part "Foo" in "Foo" conflicts with an existing type name`, where `def Foo 1 ; def Foo 2` — the same redefinition without the `undef` — is accepted; the name's reservation outlives its binding, so a type's name can be rebound but never unbound and reused | the Lean specification's model of type installation, against core/go and the shipped words (2026-11-28) |
| [NUR419](#nur419) | A pending list or map is evaluated before a parameter's PATTERN judges it but not before a parameter's NAMED TYPE does: `def f fn [[n:[:Integer]][Any][n]] ; f [1 add 2]` is `[3]`, and with `def L [:Integer]` the same function written `n:L` is `no signature matches` for the same call — though `[1 add 2] is L` is `true`; the same for a named map shape and `{a:(1 add 2)}` | the Lean specification's model of type installation, against core/go and the shipped words (2026-11-28) |
| [NUR420](#nur420) | An optional parameter of a named type is given its type's BASE VALUE without the type being asked whether that value is a member: `def One 1 ; def g fn [[n?:One][Any][n]] ; g` calls `g 0`, which no signature takes but the one that made the call, and runs until the tape is exhausted; the same for `def Big (Integer gt 10)`; the inline `n?:1` adds no such signature and is a plain `no signature matches` | the Lean specification's model of type installation, against core/go and the shipped words (2026-11-28) |
| [NUR421](#nur421) | A singleton or structural type passes for a VALUE at a slot of the type it sits under, and is read as that type's zero: `def One 1 ; One add 1` is `1`, `def Seven 7 ; Seven mul 3` is `0`, `def S "abc" ; size S` is `0` — silently — where the builtin in the same place raises (`Integer add 1` is `no signature matches`); the exemption that admits it was written for a record's name handed to `refine Table` | the Lean specification's model of type installation, against core/go and the shipped words (2026-11-28) |
| [NUR422](#nur422) | The meet of two named types depends on their ORDER and on which of them is named: with `def One 1` and `def Foo (Integer tor String)`, `One unify Foo` is `One` and `Foo unify One` is no meet; the same union written in place gives the VALUE `1` either way; and `tand`, documented as the same meet, gives `1` for all four | the Lean specification's model of type installation, against core/go and the shipped words (2026-11-28) |
| [NUR423](#nur423) | A union met inside a typed container comes back as a ZERO: with `def U (Integer tor String)`, `(quote [U]) unify [:Integer]` is `[0]` — the element's meet is `Integer`, and the kernel hands back the union's payload retagged Integer, which every reader takes for 0 (`['']` under `[:String]`); core/ts and the Lean specification answer `[Integer]` | the Lean specification's differential fuzzer over type installation, against core/go, core/ts and the shipped words (2026-11-28) |
| [NUR424](#nur424) | `true`, `false` or an unbound name written AFTER a word is not taken at a slot of a named type that admits it: with `def Foo (String tor Boolean) ; def f fn [[n:Foo][Any][n]]`, `f true` is `no signature matches (got Word)` where `true f` and `f (true)` are `true` — and where the same union written in place of the name takes `f true` | the Lean specification's differential fuzzer over type installation, against core/go and the shipped words (2026-11-28) |
| [NUR425](#nur425) | A typed container over a SINGLETON's name is dead as a parameter's or a return's declaration: with `def One 1`, `def f fn [[m:[:One]][Any][9]] ; f [1]` is `no signature matches` and a return declared `[:One]` rejects `[1]` ("expected [:word(One)]") — though `[1] is [:One]` is `true`, the same list under a name of its own (`def L [:One]`, `m:L`) takes `[1]`, and over a union's name the declaration works | review of the Lean specification's model of type installation (PR #581), checked against core/go and the shipped words (2026-11-28) |

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

A leaf frame's close still takes ONE binding of each parameter, so an
`unpack` over the parameter's own name leaves the PARAMETER bound after
the call (measured 2026-11-27):

```
def f fn [[a:Integer m:Map][Integer][unpack [a] m  a]] ; f 5 {a: 9} ; a
  boru do          9 5      (`a` after the call is the argument, 5)
def f fn [[a:Integer m:Map][Integer][def z 0 unpack [a] m  a]] ; f 5 {a: 9} ; a
  boru do          9, then [boru/undefined_word]: undefined word: a
```

`core/spec/dispatch.tsv` pins the rule at the kernel's level with a
fixture binder the frame analysis does not list (`bindq`: five rows, the
parameter that outlives its call among them), and `core/spec/divergent.tsv`
the two rows where a body or a caller that DOES name a binding word undoes
the same binding.

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

## NUR389 — a source module's owner token is unique only among its importer's modules {#nur389}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on d1db0f8 plus the `Registry.ModuleScope` removal, through `boru run -no-check` · **silent** on the admitted side

**Rule:** a signature merged onto a core word is anchored by a nominal type
its author owns, and replaces no tuple its author does not own
(design/OPEN-WORDS.1.md R1, R2). At the def the check is strict — "the
author must own the anchor where the signature is WRITTEN"
(`requireOwnedAnchor`, core/go/word_extend.go) — and it is a property of
the def: a module body answers the same wherever it is imported.

**Divergence:** ownership is a string compare of owner tokens
(`sigHasOwnedAnchor`), and a source module's token is `"module#"` plus
`parent.Modules.NextID()` (`runModuleBodyCover`,
lang/go/native/native_module_module.go): a counter of the IMPORTING
registry (`ModuleRegistry.seq`, core/go/modules.go), which starts at zero
in every module body's own sub-registry. The Nth module a registry imports
and the Nth module that module's body imports both carry `module#mod_N`,
so the outer one passes for the owner of the inner one's types. Its def is
admitted, and a def of the same tuple replaces the inner module's
transplanted signature. A program's token is `program` and never collides,
so the program is always refused.

```
# m.boru
def T (refine Integer)
def add fn [[a:T b:T] [Integer] [1]]
def mk fn [[n:Integer] [T] [def v:T n v]]
export "M" {add: add/v mk: mk/v T: T}

# o.boru
import "./m.boru"
def add fn [[a:(M.T) b:(M.T)] [Integer] [2]]
def ask fn [[] [Integer] [add (M.mk 0) (M.mk 0)]]
export "O" {ask: ask/v}

# z.boru
export "Z" {x: 1}

# each line: boru run -no-check -e '<line>', in the files' directory
import "./m.boru"  def add fn [[a:(M.T) b:(M.T)] [Integer] [2]]
  error: [boru/extend_owner]: def add: a core word can be extended only with at least one NOMINAL argument type the extending scope owns per signature — [T T] has none
import "./o.boru"  print (O.ask)
  2
import "./z.boru"  import "./o.boru"  print (O.ask)
  error: import: ./o.boru: [boru/extend_owner]: def add: a core word can be extended only with at least one NOMINAL argument type the extending scope owns per signature — [T T] has none
import "boru:math-util"  import "./o.boru"  print (O.ask)
  error: import: ./o.boru: [boru/extend_owner]: def add: a core word can be extended only with at least one NOMINAL argument type the extending scope owns per signature — [T T] has none
```

The first line is the program writing o.boru's def, and is refused. In the
second o.boru is the program's 1st module and m.boru is o.boru's 1st: the
def is admitted, and `2` is its body — m.boru's own tuple, which answers
`1` when o.boru has no line 2, was replaced. In the third o.boru is the
2nd module. A native module draws from the same counter, so in the fourth
an unrelated `boru:` import flips the answer. With `import "./z.boru"`
also first in o.boru the two ordinals meet again and the third answers
`2`. Inline modules (`import module […]`) answer the same way. The default
pre-flight agrees on the first two and, a failed file import being opaque
to check, reports the other two as `undefined word: O`.

The program's refusal is pinned (lang/spec/open-words.tsv §2, "a program
def cannot re-anchor on ANOTHER module's type"); nothing pins a module
body over an imported module's type either way, and OPEN-WORDS.1 §4 gives
a mint's owner as "the minting scope's module id" without saying where the
id is unique. The same class as NUR377, for owner tokens instead of type
IDs.

**Proposed verdict:** resolve by fix — draw a module's id from the
importing tree's sequence, as its minted type IDs already are
(`Types.AdoptSeqFrom`), so that the token names one module and the verdict
stops depending on import position. What the verdict then IS for a module
over a type of a module it imports is the maintainer's call: refused like
the program's (R1 as written), or admitted as the transplant's re-export
carveout admits a module-minted anchor (`sigHasModuleAnchor`). The second
needs a check of its own for R2: today only R1 stands between a def and
another module's transplanted tuple (`mergeExtensionSigs` lets a direct
def replace any unlocked one).

---

## NUR390 — a `/uv` value is not collected by a waiting word, where a `/v` value is {#nur390}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through the core/spec notation and through `boru do` · silent on one shape, loud on the other

**Rule:** a reference denotes data, and data ARRIVES at a collection that is
waiting for it (the `/v` rule, NUR050). `name/uv` is documented as `name/u`
left as inert data — "mirrors /v" (`stepWordUsurp`, core/go/engine.go).

**Divergence:** `/v` delivers its value through the literal step when a
parked forward in the scope is still collecting (`deliverValRead`). `/uv`
sets the cell and advances the pointer unconditionally, so the wrapper is
left BEHIND the pointer, on the stack side, and never arrives:

```
run bothq 1 negq/v        fn [[a:Integer][Integer][]]     collected as bothq's second argument
run bothq 1 negq/uv       ERROR:signature_error           the forward is stranded
run boomq negq/v          ERROR:fixture_boom              collected
run boomq negq/uv         ERROR:signature_error
run bothq negq/v 1        1                               slot 0 the function, slot 1 `1`
run bothq negq/uv 1       fn [[a:Integer][Integer][]]     SILENT: `1` fills slot 0, the wrapper is taken from the stack as slot 1
```

Through the CLI, with `def k fn [[a:Any b:Any][Any][b]]`: `k sub/v 1` is
`1`, and `k sub/uv 1` binds the two the other way round. The comment at the
`/v` site records this very defect as fixed there ("the former
unconditional set+advance left the value BEHIND the pointer, so the forward
stranded at end of run"); the `/uv` branch kept the old tail.

The Lean specification models `/uv` as the kernel has it
(`spec/Boru/Core/Engine.lean`, the `/u` arm of `stepWord`).

**Proposed verdict:** resolve by fix — `/uv` takes `/v`'s delivery
(`deliverValRead`), so the two references differ only in the value they
denote.

---

## NUR391 — a word with more than one definition is not `eq` to itself {#nur391}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through the core/spec notation and through `boru do` · silent

**Rule:** a value is equal to itself; a function's identity is the function
(`eq` compares functions by identity, `deq` by content — NUR031).

**Divergence:** a reference to a function word (`f/v`, `ref`, an export
map's value) is built from the name's dispatch aggregate, and the aggregate
inherits an identity only when the name has ONE definition
(`aggregateIdent`, core/go/registry.go: "the sole entry's, or none when the
union spans several"). With two or more, every read mints a new identity
(`NewFunction`), so two reads of the same name are two different functions:

```
def f fn [[n:Integer][Integer][n]]  (f/v eq f/v)                                         true
def f fn [[n:Integer][Integer][n]]  def f fn [[s:String][String][s]]  (f/v eq f/v)      false
…the same two defs…                                                 (f/v deq f/v)     true
…the same two defs…                                                 (f/v unify f/v)   ~unify-fail
…the same two defs…   def g f/v                                     (g/v eq g/v)      true
```

Adding an overload to a word changes whether the word is equal to itself;
binding the reference once (`g`) restores it. The core/spec notation shows
the same: `def f fn( … ) ; def f fn( … ) ; unifyq f/v f/v` is `false` where
the one-definition row is `true`.

The Lean specification models the rule as the kernel has it
(`spec/Boru/Core/Match.lean` `Defs.soleIdent?` / `refIdent`).

**Proposed verdict:** resolve by fix — a name's aggregate has an identity
that is stable while its set of definitions is unchanged (cached with the
aggregate, or derived from the entries' identities), so a reference read
twice is one function.

---

## NUR392 — an unmatched `(` is refused in two places and a value in two others {#nur392}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through the core/spec notation · silent in a loop body, a different error in a function body

**Rule:** one construct, one verdict — a malformed program is refused the
same way wherever the malformed part stands (NUR387: "a structure's close
pairs with its own open", and an unmatched `(` is a scan boundary in a
literal exactly as at the top level).

**Divergence:** the refusal of an open paren that is never closed is made
in two places only: the top-level drain (`engine.go`, "unmatched opening
parenthesis") and the sealed region of a container literal (`sealed.go`).
A driven loop's region and a call frame have no such check, so the marker
is left in what they collect:

```
run (                                     ERROR:syntax_error
run [ ( ]                                 ERROR:syntax_error
run { a: ( }                              ERROR:syntax_error
run eachq [q 1 ] [q ( ]                   [1 (]                 collected as a value, no error
run eachq [q 1 ] [q negq ( ]              [-1 (]
run def f fn( n Any Any [ ( ] ) ; f 1     ERROR:type_error      the return check counts it as a result
```

An unmatched `)` is refused in all four places (`run eachq [q 1 2 ] [q ) ]`
is syntax_error). core/ts refuses the function-body case as a
syntax_error (core/spec/divergent.tsv); it has no driven loops.

The Lean specification models the rule as the kernel has it
(`spec/Boru/Core/Engine.lean` `runRegion`, against `runSealed`).

**Proposed verdict:** resolve by fix — a loop's region and a call frame
refuse an open paren left unmatched in their body as the top level and a
literal do.

---

## NUR393 — typed containers are `eq` by their child constraint alone {#nur393}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through the core/spec notation and through `boru do` · silent

**Rule:** `eq` compares values; two values that differ are not equal, and a
disjunct keeps every alternative that is not equal to another (ADR-015).

**Divergence:** `valuesEqualDefault` (core/go/equal.go) compares two typed
lists, and two typed maps, by their CHILD CONSTRAINT only — the elements
they retain are never read:

```
[:Integer 1] eq [:Integer 2]                     true
[:Integer 1] deq [:Integer 2]                    false
([:Integer 1] tor [:Integer 2])                  [:Integer 1]
({:Integer a:1} tor {:Integer a:2})              {:Integer a:1}
run d( [: Integer 1 ] [: Integer 2 ] )           [:Integer 1]          (core/spec notation)
```

A second effect in the same place: resolution asks that equality whether
an alternative "changed", so a typed container's retained words are never
resolved and a disjunct nested in it is never canonicalised:

```
run d( [: Any foo ] )                 [:Any foo]                  (a plain list's `foo` becomes `foo/q`)
run d( [: 5 d( 0 bar 2 ) ] )          [:5 (0 tor bar tor 2)]      (unsorted)
```

Both readings are defensible and they disagree: as a TYPE, `[:Integer 1]`
is `[:Integer]` and what it retains is not part of it (unification of the
two answers `[:Integer]`); as a VALUE the two hold different elements, and
`deq` says so. `eq` and `tor` take the first reading of a thing written as
the second.

core/ts follows core/go here since its unifier port. The Lean
specification does NOT: it compares the retained elements
(`spec/Boru/Core/Compare.lean` `valuesEqual`), and its differential search
does not generate the shape.

**Proposed verdict:** resolve by fix — a typed container that retains
elements is equal to another only when the elements are; one that retains
none is the type, and equal by its constraint.

---

## NUR394 — a disjunct inside another value is compared as it prints {#nur394}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through the core/spec notation · silent

**Rule:** equality is structural, and a function is equal only to itself
(NUR031).

**Divergence:** a disjunct reached as a member of another value — an
alternative that is itself a disjunct — falls to `valuesEqualDefault`'s
last arm, which compares the two payloads' `%v` rendering. Two members
that print alike are therefore one alternative, whatever they hold:

```
run d( d( d( 1 ) ) d( 1 ) )           1          two different structures, one rendering
run d( d( fn( n Integer Integer [ n ] ) ) d( fn( n Integer Integer [ n ] ) ) )
                                      fn [[n:Integer][Integer][n]]      two functions, deduplicated
run d( fn( n Integer Integer [ n ] ) fn( n Integer Integer [ n ] ) )
                                      fn [[n:Integer][Integer][n]] tor fn [[n:Integer][Integer][n]]
```

The last row is the rule: unwrapped, the same two functions are two
alternatives.

core/ts follows core/go here since its unifier port. The Lean
specification does NOT (it compares structure), and its differential
search does not generate the shape.

**Proposed verdict:** resolve by fix — a disjunct is compared by its
alternatives, as a list is by its elements.

---

## NUR395 — the total order is not transitive across typed and plain lists {#nur395}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through the core/spec notation · silent

**Rule:** `CompareValues` is a total order (design/TYPE-ORDERING.10.md), and
a disjunct's canonical form is a function of its set of alternatives
(ADR-015; NUR385).

**Divergence:** two plain lists of one size compare member by member; a
typed list against a plain list of that size falls to canonical TEXT. The
three comparisons do not compose — `[foo/q]` < `[(1)]` < `[:Any 0]` <
`[foo/q]` — so sorting a disjunct's alternatives has no one answer, and
the insertion sort returns whichever the written order leads it to:

```
run d( [q d( 1 ) ] [q foo ] [: Any 0 ] )      [foo/q] tor [(1)] tor [:Any 0]
run d( [: Any 0 ] [q d( 1 ) ] [q foo ] )      [(1)] tor [:Any 0] tor [foo/q]
run d( [q foo ] [: Any 0 ] [q d( 1 ) ] )      [:Any 0] tor [foo/q] tor [(1)]
```

One set of alternatives, three canonical forms.

core/ts runs the same sort and matches core/go. The Lean specification
states the order it has (`spec/Boru/Core/Compare.lean` `compareValues`) and
disagrees with core/go on these rows; its differential search does not
generate the shape.

**Proposed verdict:** resolve by fix — a typed list and a plain list
compare by one rule that composes (member by member, the typed one's
constraint as a tie-break), so the order is total and the canonical form
unique.

---

## NUR396 — a function a body hands back alone is refused as an uncalled call {#nur396}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `boru do` and the core/spec notation · loud

**Rule:** a function value with nothing it can take beside it is a value; a
NAMED one is refused (`uncalled_function`) only when something stood where
its arguments would have (`execFnDefLiteral`: "a failed forward-form call …
recognised as a call, not a bare value reference").

**Divergence:** the candidates are gathered by `upcomingArgs`
(core/go/engine.go), which walks the tape after the function up to the next
`end`, close paren, mark or move — and counts everything before it. In a
call frame the function body's last value is followed by the frame's own
cleanup marker (and its return check), which are not values and are not
boundaries, so they are counted: a named function that is the body's only
result always has "a candidate argument", and is refused.

```
def m {f: add/v}
m.f                                            fn add(…)                    the function, a value
def pk1 fn [[x:Map][Any][x.f]]      pk1 m      [boru/uncalled_function]: call to 'add' matched no signature
def pk1 fn [[x:Map][Any][x get "f"]] pk1 m     [boru/uncalled_function]: call to 'add' matched no signature
def mk fn [[][Function][add/v]]     mk         fn add(…)                    a /v reference is delivered, not dispatched
```

In the core/spec notation: `run def mk fn( n Any Any [ fn( m Any Any [ m ] ) ] ) ; mk 5`
is `uncalled_function`, where `run fn( m Any Any [ m ] )`, the same in a
group, and `run bothq 1 fn( m Any Any [ m ] )` leave the function as a
value (core/spec/dispatch.tsv). An anonymous function is never refused, so
a closure handed back is unaffected.

core/ts and the Lean specification reproduce the kernel
(`spec/Boru/Core/Engine.lean` `dispatchFnValue`: the `upcoming` window).

**Proposed verdict:** resolve by fix — a frame's cleanup marker and return
check end the window, as a close paren does: a function a body hands back
alone is the body's result.

---

## NUR398 — a word defined in boru is not retried from the stack when its forward plan fails {#nur398}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `boru do` and the core/spec notation · loud

**Rule:** one dispatch for every callee. `matchForDispatch`
(core/go/engine.go) states it: the plan is matched over the forward tokens
and the stack, and when that fails for a callee with forward-collecting
signatures the match is tried again with every argument taken from the
stack (`ForceStack`). The step for a function value at the pointer says the
same in its own words — "Function values at the pointer dispatch with the
same forward+stack matching as words".

**Divergence:** the function a NAME dispatches to carries a synthetic
zero-argument fallback whenever one of its definitions has a body
(`aggregateDispatch` → `fnFallbackSig`). `PlanMatch` returns that fallback
when no real signature matches, so the match "succeeds", the retry never
runs, and `stepWord` then raises `signature_error` for the fallback it was
handed. A native word has no fallback and is retried; a function LITERAL
has none either; a forced arity (`/N`) skips the fallback inside the
matcher, so the same word under `/2` is retried; and a body of no tokens
gets no fallback at all (`len(s.Body()) > 0`).

```
def ov fn [[a:Any b:Scalar][Any][1]]

"k" [] ov 5                                  [boru/signature_error]: cannot call `ov`
"k" [] ov/2 5                                1 5      the same word, arity forced
"k" [] ov/s 5                                1 5      the same word, forced to the stack
"k" [] fn [[a:Any b:Scalar][Any][1]] 5       1 5      the same function, as a literal
"k" [] ov                                    1        nothing written after it
"k" 4 ov 5                                   k 1      a Scalar on top: the plan holds
```

The forward `5` fits `a`, the list on top of the stack is no `Scalar`, so
the plan — one argument forward, one from the stack — fails; with both
taken from the stack (`a` the list, `b` the string) the call matches. Which
of the two the caller gets depends on whether the function was reached by a
name, and on whether that name was given an arity.

In the core/spec notation: `run def ov fn( a Any b Scalar Any [ 1 ] ) ; null [q ] ov 5`
is `signature_error` (core/spec/dispatch.tsv) where
`run null [q ] fn( a Any b Scalar Any [ 1 ] ) 5` and the `ov/2` form are
`1 5` (core/spec/divergent.tsv: core/ts retries for none of the three).

The Lean specification reproduces the kernel
(`spec/Boru/Core/Engine.lean` `answersItself`).

**Proposed verdict:** resolve by fix, one of two ways, and the choice is
the maintainer's. (a) The fallback is not a match: `matchForDispatch`
retries when the plan hands back only the fallback, so a boru-defined word
behaves as a native word and as its own literal do. (b) The retry is the
special case: it was written for a `CallBoru` sub-engine's deepest-first
inputs, and a call whose forward argument fits but whose stack half does
not is refused for every callee. (a) is the smaller change and removes the
name/literal difference without refusing any program that runs today.

---

## NUR400 — a full-stack word in a loop's round or an `if`'s condition ends the construct {#nur400}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `boru do` and the core/spec notation · silent

**Rule:** a counted loop runs its body once for each index of its range
(`for 3 [i]` is `0 1 2`), and unbinds its index when it ends.

**Divergence:** a round is `mark body… move` on the tape, with no paren
around it, and the move finds its loop by finding its mark. A FULL-STACK
word's handler returns "the complete replacement for everything from base
through the pointer" (`execMatch`, core/go/engine.go:
`e.Tape.Splice(base, e.Pointer+1-base, results...)`), where base is the
start of the paren scope — so the splice takes the loop's mark with the
values. The move then finds no mark and is "removed quietly" (`stepMove`:
"Mark was removed from the stack"), which is the path a finished loop's
stray move takes. The loop ends after that round, with no error; the values
beneath the loop have been drawn into the round; and the index is never
unbound, because the loop never reaches its end.

```
for 3 [i]              0 1 2
for 3 [depth]          0             one round of three
for 3 [i depth]        0 1           one round
9 8 for 3 [depth]      9 8 2         one round; the 9 and 8 are now the round's
for 3 [depth] i        0 0           the index is still bound after the loop
def i 77
for 3 [depth]
i                      0 0           …over the caller's own `i`
```

Every construct that runs between a mark and a move is affected the same
way — a conditional loop, in either region, and an `if`, whose condition
is such a region:

```
if [0] ["yes"] ["no"]          no
if [depth] ["yes"] ["no"]      0             neither branch: the condition's value is all that is left
4 if [depth] ["yes"] ["no"]    4 1
if [(depth gt 0)] ["yes"] ["no"]   no        in a group, the group is the word's scope: unaffected
def n 0
while [depth] [def n (n add 1) (if (n gt 2) [break]) n]      0      the loop never runs
```

`depth`, `pick`, `roll` and `stack` are the shipped full-stack words
(`7 for 2 [0 pick]` is `7 7`, one round). In the
core/spec notation: `run forq 3 [q depthq ] i` is `0 0`
(core/spec/divergent.tsv, the NUR400 block). A driven loop is not affected:
its region is sealed by a paren, which is the full-stack word's base.

The Lean specification reproduces the kernel (`spec/Boru/Core/Engine.lean`
`execMatch`, the full-stack arm; `stepMove`, a move whose mark is gone).

**Proposed verdict:** resolve by fix — the replacement stands for the
VALUES of the scope, and a loop's mark in that stretch is not one of them.
Where the mark then stands is the choice to make: (a) in place, so the
values beneath the loop stay beneath it and `9 8 for 3 [depth]` is
`9 8 2 2 2`; or (b) ahead of the replacement, so everything the handler
returned is the round's and it is `9 8 2 0 0`. (a) keeps "what a round
leaves is what its body pushed" and is what a reader of the loop expects.

---

## NUR401 — a function value drops the argument it collected when widening does not fill {#nur401}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `boru do` and the core/spec notation · silent

**Rule:** one dispatch for every callee: "Function values at the pointer
dispatch with the same forward+stack matching as words" (core/go/engine.go,
the function-value step). A call that has collected its argument fires with
it.

**Divergence:** when a WORD's collection completes it is rewritten to `/s`
and fires over what it holds. A function VALUE is sealed the same way,
except that `fnValueWouldWiden` first asks whether a wider overload could
take the next token "against EVERY forward-eligible slot of each wider sig"
— and if any single slot admits it, the value is left unsealed and planned
again in full. That test is necessary, not sufficient: it never asks whether
the wider overload would then FILL. When it does not, the re-plan falls back
to the narrower overload — which, planning afresh, collects the NEXT token
forward, and the argument it had already collected is left on the stack.

```
def f fn [[a:Integer][Any][(a add 100)]]
def f fn [[a:String b:Integer][Any][b]]
def m {g: f/v}

f 5 6          105 6        the word: f(5), then 6
m.g 5 6        5 106        the same function as a value: the 5 is left, and 6 is taken
m.g 5 "x"      5            the designed case: the wider definition fills — f("x", 5)
```

`6` fits the wider definition's `b:Integer`, so the call is planned again;
`[a:String b:Integer]` does not take `6` and `5`; `[a:Integer]` then takes
`6`. Nothing is raised. Which argument a call uses depends on whether the
function was reached by its name.

In the core/spec notation, through a `/u` wrapper (a function value):
`run def ov fn( a Integer b String Any [ 2 ] ) ; def ov fn( a Integer Any [ 1 ] ) ; ov/u 5 6`
is `5 1` where the word `ov 5 6` is `1 6` (core/spec/divergent.tsv and
dispatch.tsv).

The Lean specification reproduces the kernel (`spec/Boru/Core/Engine.lean`
`wouldWiden`, and the unsealed re-plan in `stepLiteral`).

**Proposed verdict:** resolve by fix — widening is decided by the wider
PLAN, not by one slot: the value is left unsealed only when a wider overload
matches over the collected arguments and the forward window, and otherwise
fires sealed with what it holds, as its word does.

---

## NUR402 — a counted loop's index wraps past the Integer range and the loop runs on {#nur402}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go through `boru do` and the core/spec notation · silent

**Rule:** Integer arithmetic that leaves the range is an error
(`9223372036854775807 add 1` is `[boru/integer_overflow]: … does not fit in
the Integer range`), and a counted loop runs its body once for each index
of its range.

**Divergence:** the loop advances its own index with an unchecked `int64`
addition (`stepMoveCont`, core/go/engine.go: `cont.Current += cont.Step`)
and then asks whether the index is short of the bound. An advance that
passes the top of the range wraps to the bottom, where it IS short of the
bound, so a range that is exhausted starts over from the other end and the
loop runs until something stops it.

```
for [9223372036854775805 9223372036854775807 1] [i]     9223372036854775805 9223372036854775806
for [9223372036854775806 9223372036854775807 2] [i]     [boru/evaluation_limit] after ten million steps
def n 0
for [9223372036854775806 9223372036854775807 2] [(if (n gt 2) [break]) def n (n add 1) i]
                                                        9223372036854775806 -9223372036854775808 -9223372036854775806
```

A step of 1 always lands on the bound and stops; any other step can pass
it. The same holds counting down past the bottom of the range.

In the core/spec notation:
`run def c 'x' ; def c 5 ; rangeq 9223372036854775806 9223372036854775807 2 [q negq c undef c ]`
is `signature_error` — a second round ran — where the step-1 range beside
it is `-5` (core/spec/divergent.tsv, the NUR402 block).

The Lean specification reproduces the kernel (`spec/Boru/Core/Engine.lean`
`stepMove`: the advance is `wrapInt64`).

**Proposed verdict:** resolve by fix — an advance that leaves the Integer
range has passed the bound, and the loop ends there, as it does one step
earlier. No error is owed: the range is exhausted, which is how every loop
ends.

---

## NUR403 — an empty list built over a nil slice is not a list to the kernel's readers {#nur403}

**Status:** Pending · **Recorded:** 2026-11-27 · measured on core/go directly (no reproduction through the shipped words was found) · silent

**Rule:** a value is what it renders as and compares as; an empty list is
an empty list. core/go/CLAUDE.md, "No Zero-Value Overload (CRITICAL)": a
field must not use the Go zero value "to mean both 'the author omitted
this' AND a valid explicit value".

**Divergence:** `NewList(elems)` stores the slice it is given, and
`ReadList.IsNil()` — "no backing data" — is `elems == nil`. So the empty
list a handler builds as `NewList(out)` over a slice nothing was appended
to is a different thing from `NewList([]Value{})` to every reader that
tests `!lst.IsNil()` before treating a value as a list: `BodyTokens` and
`Loop.bodyTokens` (a loop's body), and the same test in `core_flex.go`,
`core_make.go`, `core_type.go`, `carrier_body.go`, `canon.go` and
`engine.go`.

```go
a, b := NewList(nil), NewList([]Value{})
CanonValue(a), CanonValue(b)     // "[]", "[]"
CompareValues(a, b)              // 0
len(BodyTokens(a))               // 1 — the body is the list itself, as a value
len(BodyTokens(b))               // 0 — the body is no tokens
```

As a driven loop's body the nil-backed list is therefore PUSHED once a
round where the other runs nothing. The core/spec fixture met it: its
`eachq` answered a loop of no rounds with `NewList(d.out)` over a nil
`out`, and that answer, used as another loop's body, was a value —
`eachq [q 1 2 ] ( eachq [q ] [q boomq ] )` was `[1 [] 2 []]` where
`eachq [q 1 2 ] [q ]` is `[1 2]`. The fixture now starts `out` empty; the
kernel is as it was.

No shipped word was found to hand a nil-backed list to a body position
(`each (filter [false] [1 2]) [1 2]` and six like it answer as `each []
[1 2]` does), so this is recorded at the level it was measured: the
kernel's own API.

The Lean specification does not reproduce it: a list there has no slice to
be nil.

**Proposed verdict:** resolve by fix — `NewList` (and `NewEvalList`) store
an empty slice for a nil one, so `IsNil` means only what its callers need
it to mean, "this is not a plain list's payload".

---

## NUR404 — an unnamed parameter's allowance drops a value the body computed {#nur404}

**Status:** Pending · **Recorded:** 2026-11-27 · measured through the shipped words · silent

**Rule:** a function returns what it declares: a body that leaves more
values than the declared returns is a `type_error` ("expected 1 return
value(s), got 2"). design/LANGREF.10.md's one sentence on the subject —
"Unnamed parameters list the expected types positionally", with a body
that "reads the stack" (REFERENCE.md) — says nothing of an exception.

**Divergence:** an unnamed parameter's argument enters the frame as a
value, and the body may leave it unread; the return check therefore lets
up to as many EXTRA values pass as the signature has unnamed parameters,
and removes them from the bottom of the frame (`core/go/engine.go`, the
return check in the frame's close: `extra > rc.UnnamedCount`). The
allowance is a COUNT. Nothing ties the values it removes to the
arguments, so when the body has consumed an argument, a value the body
computed is removed in its place:

```
def f fn [[Integer][String]["a" "b"]] ; f 5        → type_error: expected 1 return value(s), got 2 — ['a' 'b']
def f fn [[Integer][String][drop "a" "b"]] ; f 5   → b      ('a' is dropped, silently)
def f fn [[Integer Integer][Integer][100 200 300]] ; f 5 6            → type_error
def f fn [[Integer Integer][Integer][drop drop 100 200 300]] ; f 5 6  → 300  (100 and 200 dropped)
def f fn [[n:Integer][String][n drop "a" "b"]] ; f 5                  → type_error (named: no allowance)
```

The error text shows the model the code has in mind — it reports the
count and the values ABOVE the first `UnnamedCount`, "the unnamed-arg
allowance is spent from the BOTTOM" — and the model holds only while the
arguments are still where they entered. `core/spec/divergent.tsv` pins
three rows of it at the kernel's level (`fn( _ Any Any [ negq/s 7 ] )`
over `5` answers `7`; the `-5` the body computed is gone).

The Lean specification reproduces it (`closeFrame`, the return check's
`unnamed` count), as the kernel has it.

**Proposed verdict:** resolve by fix — the allowance covers the unnamed
arguments themselves: the values removed beneath the returns must be the
frame's own argument cells, still unconsumed (the frame knows where its
argument span ends; a body that has stepped below it has spent the
allowance). A surplus the body computed is then the `type_error` it is
everywhere else. The alternative — Allowed, with the rule written down as
"a function of N unnamed parameters may leave up to N values unreturned" —
keeps a silent drop in a language whose return check exists to refuse
one.

---

## NUR405 — the retired frame-pop word `__pa` is callable, and unbalances the frame state {#nur405}

**Status:** Pending · **Recorded:** 2026-11-27 · measured through the shipped words · the error it causes is reported far from its cause

**Rule:** inside a function `args` is the call's arguments, and a function
built in a body captures what the body's frame bound (REFERENCE.md). A
frame's bookkeeping — the args list, the capture baseline — is the
kernel's: `core/go/fn_frame.go` closes a frame "in one Go-side pass"
(`stepDefCleanup` → `PopFrameArgs`).

**Divergence:** until 2026-10-08 that pop was a WORD, `__pa`, that every
frame's tail emitted. The frame's `DefCleanup` marker has done the pop
itself since, and nothing emits the word — but `basic/go`
(`native_definition.go`) still registers it, over the same
`PopFrameArgs`, and a leading underscore is an ordinary word character.
So a program can call it, and a call is one pop the frame's close does
not know about:

```
def g fn [[m:Integer][Integer][m]] ;      def f fn [[n:Integer][List][g 9 drop args]] ; f 5   → [5]
def g fn [[m:Integer][Integer][__pa m]] ; def f fn [[n:Integer][List][g 9 drop args]] ; f 5   → args_error: args: not inside a function
def mk fn [[x:Integer][Any][fn [[n:Integer][Integer][n add x]]]] ;      def h (mk 10) ; h 5   → 15
def mk fn [[x:Integer][Any][__pa fn [[n:Integer][Integer][n add x]]]] ; def h (mk 10) ; h 5   → signature_error: cannot call `add`
```

In the first pair `g`'s body pops its own arguments and `g`'s close pops
`f`'s; in the second the baseline is gone before the inner function is
built, so it captures nothing and `x` is unbound when it runs. Neither
error names `__pa`. `boru describe __pa` answers "not described";
`design/SIGNATURES.10.md` lists it as "Internal: pops args stack", and
`design/COMPILER-LANE-REMOVAL.0.md` already notes that it "is registered
and nothing emits it".

`core/spec/dispatch.tsv` pins nine rows of it and
`core/spec/divergent.tsv` two (the fixture word is the same
`PopFrameArgs`), and the Lean specification reproduces them
(`Theorems/Frame.lean`, `pa_pops_what_the_close_pops`).

**Proposed verdict:** resolve by removal — unregister `__pa` in
`basic/go` and `test/specfix`: no frame needs it. The corpus rows that
use it as "a word of no results" (`bothq __pa …`) take another such word,
and the rows that pin the double pop go with it.

---

## NUR406 — a body that unbinds its own parameter unbinds the caller's binding of that name {#nur406}

**Status:** Pending · **Recorded:** 2026-11-27 · measured through the shipped words · silent (`boru check`: 0 errors)

**Rule:** a parameter is bound for the call and unbound when it ends, "so
they do not leak" (design/LANGREF.10.md, "Named parameters are
automatically undefined after the body executes"); a call leaves the
caller's bindings as it found them — `def n 1 ; def f fn
[[n:Integer][Integer][7]] ; f 5 ; n` is `7 1`.

**Divergence:** the frame's close takes its own names off by COUNT, not
by depth: one binding of each parameter and capture, whatever the name
holds by then (`core/go/engine.go` `stepDefCleanup`, the
`UninstallFrameBinding` loop over `DefCleanupInfo.Names`). The body-local
truncation before it only ever unbinds DOWN to the entry snapshot. So
when the body has unbound its own parameter — `undef n` — the close's one
pop lands on whatever `n` is beneath: the caller's.

```
def n 1 ; def f fn [[n:Integer][Integer][7]] ; f 5 ; n               → 7 1
def n 1 ; def f fn [[n:Integer][Integer][undef n 7]] ; f 5 ; n       → 7, then undefined_word: n
def n 1 ; def g fn [[n:Integer][Integer][undef n 7]] ;
          def f fn [[n:Integer][Integer][g 2 drop n]] ; f 5           → 1    (f's own n, 5, is gone: it reads the global)
```

In the last program a function loses its own parameter to a function it
called. `boru check -e` on the second reports `0 error(s)` and types the
result `Integer Integer`.

`core/spec/dispatch.tsv` pins three rows of it at the kernel's level (the
`undef` section; `core/ts` has the same count-based close, so the three
runners agree), with the row beside them that shows the count is the
whole story: `undef n def n 9` in the body leaves ONE binding for the
close to take, and the caller's `n` is intact. The Lean specification
reproduces it (`Engine.lean` `popNames`; `Theorems/Locals.lean`,
`the_close_takes_one_binding_of_each_name`).

**Proposed verdict:** resolve by fix — the close unbinds each of its
names down to the depth the name had BEFORE the frame bound it (the
snapshot one step earlier than the one `truncateFrameDefs` uses), so a
name the body already unbound costs the caller nothing. The alternative
is to refuse `undef` of a parameter in the body it belongs to.

---

## NUR407 — tail-call elision changes what a program answers {#nur407}

**Status:** Pending · **Recorded:** 2026-11-27 · measured through the shipped words, and on core/go with elision switched off · silent, and a WRONG VALUE in one form

**Rule:** tail-call elimination is a resource guarantee, not a change of
meaning. `core/go/engine.go`, where it is done: "A declined call nests as
before — correctness never depends on firing"; `core/go/fn_frame.go`, on
the gate: the caller's "bindings are then rebound before the callee body
runs, so a dynamic read anywhere in the callee chain sees exactly what it
would have seen under nesting".

**Divergence:** when a call is the last thing a body does and the callee
installs every name the caller's close removes, the caller's frame is
closed BEFORE the callee runs — its parameters are unbound and the
callee's bound in their place. That is the same as nesting for as long as
the callee's bindings stay where they are. A callee that unbinds its own
parameter (`undef n`) uncovers what is beneath: under nesting the
caller's binding, under elision whatever lay beneath the caller's.

```
def n 8 ; def g fn [[n:Integer][Integer][undef n n]] ;
          def f fn [[n:Integer][Integer][g 7 add 0]] ; f 5      → 5      (nested: g reads f's n)
          def f fn [[n:Integer][Integer][g 7]] ; f 5            → 8      (last in f: g reads the global)
          def f fn [[n:Integer][Integer][(g 7)]] ; f 5          → 8
def g fn [[n:Integer][Integer][undef n n]] ; def f fn [[n:Integer][Integer][g 7]] ; f 5
                                                                 → undefined_word: n     (nested: 5)
```

With `Registry.TCO.Disable` set the kernel answers `5` in every form. The
unbinding need not be the callee's own: a function further down the chain
that unbinds `n` twice shows the same difference.

`core/spec/divergent.tsv` pins the two forms — its only rows where the
Lean specification, which nests every call, agrees with `core/ts` (which
only nests) and not with `core/go` — and
`core/go` `TestCoreSpecTailCallsUnobservable` runs every corpus row with
elision off and holds those two, and no others, to the nested answer.

It is the same opening as NUR406 — `undef` reaching a binding a FRAME
owns — seen from the caller's side.

**Proposed verdict:** resolve by fix, with NUR406 — a parameter or capture
of an open frame is not `undef`'s to remove: it ends with its frame (the
`undef` is an error, or unbinds nothing). Then no body can uncover what
is beneath a frame's own binding, the eager close is indistinguishable
from nesting again, and the frame's close has exactly its own bindings to
take. The alternative that keeps `undef` as it is — the gate declining
whenever the callee can unbind — cannot be decided before the call: the
unbinding may be anywhere in what the callee runs.

---

## NUR408 — a parameter or a return declared as the value `none` constrains nothing {#nur408}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (`boru check`: 0 errors)

**Rule:** "Types are values and values are types (ADR-010), so a literal
declares a return that must equal it" (REFERENCE.md, on the output-sig);
the same of a parameter — `x:7` admits `7` and nothing else.

**Divergence:** `ResolveSigType` (`core/go/fn_params.go`) gives every
scalar literal its family as the slot's type and the literal as the
slot's PATTERN — Integer, Float, Boolean, String, Atom. `none` is not in
that list: it is not a bare type node (that is `None`), not a scalar of
the five, and falls to the function's last line, `return TAny, nil` — the
slot is `Any` with no pattern.

```
def f fn [[x:7][Integer][7]] ; f 5          → signature_error
def f fn [[x:None][Integer][7]] ; f 5       → signature_error        (the TYPE None)
def f fn [[x:none][Integer][7]] ; f 5       → 7                      (the VALUE none: anything)
def f fn [[x:none][Integer][7]] ; f "a"     → 7
def f fn [[x:Any][7][x]] ; f 5              → type_error: expected 7, got Integer
def f fn [[x:Any][none][x]] ; f 5           → 5                      (a return of anything)
```

A function declared to return `none` — the natural way to write "returns
nothing useful" — checks nothing.

The Lean specification states the arm as the kernel has it
(`FnSpec.lean` `resolveSigType`, the `.none` case); the corpus notation
cannot write the value `none` inside a specification, so no row pins it.

**Proposed verdict:** resolve by fix — `none` in a declaration is the
literal it is: the slot's type is `None` and the pattern `none`, as `7`
is `Integer` with the pattern `7`. (The reader's last line then has
nothing legitimate left to catch, and could refuse what reaches it.)

---

## NUR409 — a parameter written without a name takes fewer declarations than a named one {#nur409}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · loud

**Rule:** "Unnamed parameters list the expected types positionally"
(design/LANGREF.10.md); a named parameter is `name:type`. The name is
the only thing the two spellings differ by.

**Divergence:** the reader has two paths (`core/go/fn_params.go`
`ParseFnParams`). A `name:Type` pair hands its type to `ResolveSigType`,
which reads every declaration there is. An element with NO name is
matched by the loop's own `switch`, which lists a few shapes — a type
name, a type literal, an Integer, a Boolean, a String, a dotted
annotation — and refuses the rest:

```
def f fn [[x:1.5][Integer][7]]                  read
def f fn [[1.5][Integer][7]]                    function spec: invalid parameter: 1.5
def f fn [[x:(Integer tor String)][Integer][7]] read
def f fn [[(Integer tor String)][Integer][7]]   function spec: invalid parameter: paren([…])
def f fn [[x:none][Integer][7]]                 read
def f fn [[none][Integer][7]]                   function spec: invalid parameter: none
def f fn [[x:[1 2]][Integer][7]]                read
def f fn [[[1 2]][Integer][7]]                  function spec: invalid parameter: [1 2]
def t Integer ; def f fn [[x:t][Integer][x]]    read
def t Integer ; def f fn [[t][Integer][dup drop]]   function spec: invalid type "t"
```

`ParseFnReturns` had the same split and closed it — its comment says a
named and an unnamed return "are one declaration written two ways" and
that reducing only one "is the exact defect this change exists to
remove". The parameters still have it.

`core/spec/divergent.tsv` pins two rows of it at the kernel's level (a
union and `none` in a parameter list), and the Lean specification
reproduces the two paths (`FnSpec.lean` `parseParams`).

**Proposed verdict:** resolve by fix — an unnamed element that is not
one of the loop's markers (`?`, `|`, a `/q` atom) goes to
`ResolveSigType`, as a named one's type does.

---

## NUR410 — `?` on a parameter whose type has no base value does nothing, silently {#nur410}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent at the definition

**Rule:** a parameter marked optional may be left out of a call, and is
then its type's base value (REFERENCE.md, "the `x?:Type` optional
suffix"; `core/go/core_helpers.go` `ExpandOptionalSigs`).

**Divergence:** the install makes one extra signature for each way of
leaving optional parameters out, and SKIPS a way whose left-out
parameter has no usable default — a type `BaseValue` does not know
(`Any`, `Function`, a union, `Scalar`), or a pattern the base value does
not satisfy. The skip is deliberate (the code explains the infinite
self-recursion it replaced) and it is silent: the function is defined,
the `?` is accepted, and the call it promised does not exist.

```
def f fn [[n?:Integer][Integer][n]] ; f              → 0
def f fn [[n?:Any][Any][n]] ; f                      → signature_error: no signature matches
def f fn [[n?:Function][Any][n]] ; f                 → signature_error
def f fn [[n?:(Integer tor String)][Any][n]] ; f     → signature_error
def f fn [[n?:7][Integer][n]] ; f                    → signature_error   (the base value 0 is not 7)
```

`core/spec/divergent.tsv` pins two rows at the kernel's level
(`n?:Any`: the call, and the function's signatures — one, not two), and
the Lean specification reproduces the skip (`FnSpec.lean`
`expandOptional`).

**Proposed verdict:** resolve by fix — a `?` that can make no signature
is an error where the function is defined ("parameter `n` cannot be
optional: `Any` has no base value"), so the mark either works or is
refused.

---

## NUR411 — a lowercase name bound to a union or typed list is a wildcard declaration {#nur411}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (a declared constraint is not enforced)

**Rule:** a declared parameter type selects, and a declared return is
checked: `def f fn [[n:Any][(Integer tor String)][n]] ; f true` is a
`type_error`, and so is the same function over `def U (Integer tor
String)` — "expected U, got Boolean".

**Divergence:** a name that is not a registered type name is looked up on
the def stack (`core/go/fn_params.go` `LookupDefType`), and what it is
bound to is turned into a slot by `ResolveDefType`. That function knows
records, options and bare type nodes; every other type body — a union, a
typed list, a typed map, a function — falls to its last line,
`return TAny, nil, nil`. The inline spelling of the same bodies goes
through `ResolveSigType`, whose comments record this exact degradation
being closed there ("a silent wildcard that dispatched EVERYTHING").

```
def f fn [[n:Any][(Integer tor String)][n]] ; f true              → type_error
def U (Integer tor String) ; def f fn [[n:Any][U][n]] ; f true    → type_error: expected U, got Boolean
def u (Integer tor String) ; def f fn [[n:Any][u][n]] ; f true    → true
def u (Integer tor String) ; def f fn [[n:u][Any][n]] ; f true    → true
def f fn [[n:[:Integer]][Any][n]] ; f ["a"]                       → signature_error
def u [:Integer] ; def f fn [[n:u][Any][n]] ; f ["a"]             → ['a']
def u Integer ; def f fn [[n:u][Any][n]] ; f "a"                  → signature_error   (a bare type IS enforced)
def u 7 ; def f fn [[n:u][Any][n]] ; f 8                          → error at the definition: `u` is no type
```

`core/spec/divergent.tsv` pins three rows at the kernel's level, through
a declared return, and the Lean specification reproduces the arm
(`FnSpec.lean` `boundType`).

**Proposed verdict:** resolve by fix — `ResolveDefType` hands a type body
it does not itself know to `ResolveSigType`, so a name bound to a union
means the union. (If a lowercase name is not meant to name a type at
all, the fix is the error `def u 7` already gets; either way the
wildcard goes.)

---

## NUR412 — an all-unnamed function value takes its stack arguments bottom-up {#nur412}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · one form silent (a function that is never run)

**Rule:** the one argument rule (CLAUDE.md, "one rule, no exceptions"): a
call fills its positions from the forward stack, "then fills every
remaining position from the value stack in reverse — top of stack first".
"Two-argument words are not a special case", and neither is a function
whose parameters have no names.

**Divergence:** a function VALUE standing at the pointer is first planned
by that rule; when the plan fails, `execFnDefSig` falls back to the
"legacy pure-stack path" (`core/go/engine.go` `ExecFnDefSigStackMatch`,
`stackMatchSig`), whose comment states the two orders: "a signature with a
named param reads the stack top-down (param 0 is the top), an all-unnamed
one reads its window bottom-up". design/SIG-ORDER-REFACTOR.10.md lists it
as the one bottom-first path left, kept for module wrappers.

```
def f fn [[Integer String][List][args]] ; "a" 7 f       → [7 'a']           (by name: top first)
def f fn [[Integer String][List][args]] ; 7 "a" f       → signature_error
7 "a" fn [[x:Integer y:String][List][args]]             → not called        (named: top first)
"a" 7 fn [[x:Integer y:String][List][args]]             → [7 'a']
7 "a" fn [[Integer String][List][args]]                 → [7 'a']           (all unnamed: BOTTOM first)
"a" 7 fn [[Integer String][List][args]]                 → not called
```

The same path takes a USURP wrapper of such a function, and there it does
worse than read the wrong way: it splices a frame from the wrapper
signature's body, and a wrapper has none — its work is its handler. The
function is never run, and the frame's top argument is what comes back:

```
def f fn [[Integer String][List][args]] ; 7 "a" f/u     → [7 'a']
def f fn [[Integer String][List][args]] ; "a" 7 f/u     → type_error: <fn>: return value 1: expected List, got Integer
```

(with a declared return of `Any` the second is silently `7`).

The same path is what applies a function LITERAL that was never bound,
and there it shows in the ordinary forward call too. Forward arguments
are seated on the stack in the order the top-first rule reads, the
all-unnamed signature is read bottom-up, and the call does not happen;
and signatures are tried in the order they were WRITTEN, a nullary one
first if it was written first, where a bound function tries the one of
more arguments first:

```
fn [[x:Integer y:String][List][args]] 5 "a"             → [5 'a']
fn [[Integer String][List][args]] 5 "a"                 → a 5 fn (Integer, String)     (not called)
def f fn [[Integer String][List][args]] ; f 5 "a"       → [5 'a']
fn [[][Integer][1] [n:Integer][Integer][2]] 5           → 5 1      (the nullary one, written first)
fn [[n:Integer][Integer][2] [][Integer][1]] 5           → 2
def f fn [[][Integer][1] [n:Integer][Integer][2]] ; f 5 → 2
```

`core/spec/divergent.tsv` pins three rows at the kernel's level. The Lean
specification does NOT reproduce it: it states the one rule, and its
answers there are core/ts's — with NUR407's, the only ledger rows where
the specification sides with core/ts.

**Proposed verdict:** resolve by fix — the fallback reads top-first for
every signature, and a wrapper met there runs its handler (or the
fallback is confined to the module wrappers it was kept for). The
alternative, Allowed, would put a second argument order into the one
rule for one kind of value.

---

## NUR413 — `/u` does not carry a signature's raw-capture slots {#nur413}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (a code body is run that the word was to be handed as code)

**Rule:** `/u` is the bound function "with its argument order reversed":
`f/u a b c` is `f c b a` (`core/go/core_ref.go` `UsurpFunction`), and its
own comment promises that "every original behaviour (barriers, quoting,
return checks, closures, module scope) is preserved; usurp only reverses
which caller argument lands in which original slot".

**Divergence:** a signature marks a slot to take its operand as written in
four per-position fields — `NoEvalArgs` (a list unevaluated),
`NoEvalMapArgs` (a map), `RawParens` (a paren expression as code),
`FormArgs` (any form). `usurpOver` builds the wrapper's signature from a
copy of the original's with the parameters reversed, and:

- sets `NoEvalArgs` and `NoEvalMapArgs` to nil, with the comment "the
  original sig re-applies them on re-dispatch". It cannot: the WRAPPER is
  what the call matches, so the wrapper's `execMatch` evaluates the list
  or map before the original is ever handed it;
- leaves `RawParens` and `FormArgs` as copied — indexed by the ORIGINAL's
  positions on a signature whose parameters are now in the opposite
  order. With one parameter the index is the same and the mode survives;
  with two, the wrapper captures at the position the original evaluates
  and evaluates at the position the original captures.

Through the shipped words, where the no-evaluation slots are the code
bodies of `quote`, `for`, `if` and the other words that take one:

```
quote [add 1 2]                          → [add 1 2]
quote/u [add 1 2]                        → [3]
for 2 [print i]                          → 0 1
for/u [print i] 2                        → undefined word: i     (the body is run before there is a loop)
if true [print "yes"] [print "no"]       → yes
if/u [print "no"] [print "yes"] true     → no yes                (both branches run, then the chosen one's result)
```

At the kernel's level, with a form slot second of two (`form2q a b`):

```
form2q 1 x                               → [1 x]          (x bound to 5: the form slot takes the word)
form2q/u x 1                             → [1 5]          (should be the line above)
form2q p( addq 3 4 ) p( addq 1 2 )       → [7 (addq 1 2)]
form2q/u p( addq 1 2 ) p( addq 3 4 )     → [7 3]          (should be the line above)
```

`core/spec/dispatch.tsv` pins the list and map modes' rows (core/ts drops
them the same way), `core/spec/divergent.tsv` the paren and form modes',
and the Lean specification reproduces both halves (`Engine.lean` `usurpOver`,
`Theorems/Raw.lean` `nur413_usurp_does_not_carry_the_modes`).

**Proposed verdict:** resolve by fix — the wrapper carries all four
fields with their indices reversed, as it already carries the
parameters, their patterns and `/q` (`reverseParams`). The re-dispatch
then hands the original an operand that is still as it was written, at
the slot that takes it so.

---

## NUR414 — a paren expression that arrives is captured by the signature, not the slot {#nur414}

**Status:** Pending · **Recorded:** 2026-11-28 · measured at the kernel's level (`core/spec`), not through the shipped words · silent (code where a value was expected)

**Rule:** the raw-capture fields are per POSITION: "RawParens marks arg
positions where a forward ParenExpr is captured RAW", "FormArgs marks arg
positions captured as a raw FORM" (`core/go/value.go`). A slot the
signature does not mark is an ordinary slot, and an ordinary slot is
handed the value a paren expression evaluates to.

**Divergence:** the question is asked twice, and only once of the
position. The plan walk, over the tokens written after the word, asks the
position (`rawParenForward(fn, pos)`, `rawFormForward(fn, pos)`). The
arrival — a paren expression that reaches the pointer while a word is
parked waiting for its arguments — asks the parked word's SIGNATURE
(`core/go/engine.go` `pendingForwardWantsRawParen`):

```go
return fwd.Sig != nil && (len(fwd.Sig.RawParens) > 0 || len(fwd.Sig.FormArgs) > 0)
```

so a signature with a raw-paren or form slot anywhere keeps a paren
expression that arrives at ANY of its slots. Its sibling
`hasPendingForwardFormArg`, which decides the same thing for a WORD,
asks the parked forward's next slot (`fwd.Sig.FormArgs[fwd.CollectedArgs]`),
which is the position.

A paren expression arrives, rather than being written, when a name bound
to one is stepped — a binding made by a host, a parameter a host call
bound, a loop's element. With `form2q a b` (the form slot second) and
`raw2q a b` (no paren or form slot), and `x` bound to `(addq 1 2)`:

```
form2q p( addq 1 2 ) 1      → [3 1]              (written at the ordinary slot: evaluated)
form2q x 1                  → [(addq 1 2) 1]     (arriving at the same slot: captured as code)
raw2q x 1                   → [3 1]              (a signature with no such slot: evaluated)
```

The plan walk has its own looseness of the same kind: it keeps a paren
expression when ANY overload of the word marks the position, and the
overload that then runs may not be that one — it is then evaluated at
arrival instead, unless that overload marks some other position.

`core/spec/divergent.tsv` pins three rows, and the Lean specification
reproduces the arm (`Engine.lean` `pendingWantsRawParen`,
`Theorems/Raw.lean` `nur414_an_arriving_paren_is_kept_by_the_signature`).
No reproduction through the shipped words was found: `afn` and `quote`
are the shipped signatures with a raw-paren slot, and `afn`'s other slot
is itself a no-evaluation slot.

**Proposed verdict:** resolve by fix — `pendingForwardWantsRawParen` asks
the parked forward's next slot, as `hasPendingForwardFormArg` does.

---

## NUR415 — the binder that takes a splice's marker is known by its name {#nur415}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (a name bound to one value where the splice was meant)

**Rule:** a word bound to a DATA splice, where another word is collecting
an argument, is read as the group `(w)`: `f w ≡ f (w)`
(`core/go/engine.go`, the `stepWord` substitution; `collect_kernel.go`,
the plan walk). The one deliberate exception is a BINDER: "`def y xs`
with xs splice-bound must rebind the marker itself — the new name ALIASES
the splice" (`bindsReferent`). A function is the same function under any
name it is bound to: `def mydef def/v` makes `mydef` call as `def` does,
and the frame analysis already reads the FUNCTION for exactly that reason
(`fnValueIsBindingWord`, "a name bound to a binding word's VALUE …
installs into this frame exactly as the word itself would").

**Divergence:** the exception is decided by the collecting word's name —

```go
func bindsReferent(name string) bool { return name == "def" }
```

— on the argument that "def is frozen (reserved_word), so the name is a
reliable identity". The name is reserved; the function is not confined to
it. Under any other name, and for any other word that binds, the operand
is the group:

```
def w word [1 2] ; def y w ; y                        → 1 2      (y is the splice)
def w word [1 2] ; def y (w) ; y                      → 2 1      (the group: y is 1, and 2 is left)
def mydef def/v ; def w word [1 2] ; mydef y w ; y    → 2 1      (def's own function: the group)
```

`core/spec/dispatch.tsv` pins the `bindq` and `mydef` rows — core/ts has
the same test — and the Lean specification reproduces it
(`Engine.lean`, `stepWord` and `collectWalk`; `Theorems/Splice.lean`
`nur415_the_binder_is_known_by_its_name`).

**Proposed verdict:** resolve by fix — `bindsReferent` asks whether the
collecting word's function IS a binder (the identity test
`fnValueIsBindingWord` already makes), so `mydef y w` aliases the splice
as `def y w` does. Whether a host binder other than `def` should alias
too (lang's `unpack`) is the same question NUR384 asks of the frame
analysis, and wants the same answer.

---

## NUR416 — a splice marker has no canonical form {#nur416}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · visible (a debug dump where source was promised)

**Rule:** ADR-015, "Canon always round-trips": "`canon v` renders boru
source that re-parses to a value `deq` to `v`. Every value; no exempt
kinds. A rendering no parser accepts — a debug spelling, a struct dump, a
pointer address — is a defect, not a display choice".

**Divergence:** `canon` (`core/go/canon.go`) has no arm for a splice
marker (`Word/__SP`) and falls through to the engine's own value print: a
struct dump of the marker, with the payload printed by `Value.String`
rather than by canon — so a word in it is `word(x)`, the spelling
`canon.go` itself calls "the debug spelling ADR-015 calls a defect". A
marker stays on the stack whenever one is taken as a value and not
stepped: `w/v`, a member of a quoted list.

```
def w word [1 2] ; w/v            → word()({[1 2]})
def w word [1 2] ; canon w/v      → "word()({[1 2]})"
def w word ["a" x] ; w/v          → word()({['a' word(x)]})
```

The two ports do not print the same dump: core/ts answers
`word(undefined)` for each of these. `core/spec/divergent.tsv` pins two
rows of the kind; the Lean specification states the Go spelling for a
payload of lists, maps, words, paren expressions and scalars
(`Canon.lean` `debugValue` — a string between single quotes with nothing
escaped, a paren expression as `paren([…])`), not for one that holds a
function, and its differential fuzzer compares only what stands before a
marker's dump.

**Proposed verdict:** resolve by fix — canon gets an arm for the marker
in both ports: the construction that makes one over the payload's canon,
as a function value renders as its `fn` construction. (What that source
is — `word [1 2]` re-parses to a marker that fires where it stands — is
ADR-015's question for a value that acts when stepped, and the answer
chosen for a function value is the precedent.)

---

## NUR417 — a function value of no parameters is called or kept by where it came from {#nur417}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (a name bound to a call's result where the function was meant)

**Rule:** ADR-016, "Arity and origin never change function behaviour":
"Every function behaves the same way whatever its arity and wherever it
came from — named or anonymous, module or local, boru or native." It names
this very case as one of two defects standing when it was written:
"`execFnDefLiteral` treats a 0-arg **anonymous** value as data where a
named one dispatches."

**Divergence:** the gate is still there (`core/go/engine.go`
`execFnDefLiteral`):

```go
if ((fnDef.Anonymous && !applied) || fnDef.Macro) &&
    fwdCount == 0 && len(positions) == 0 {
    e.Pointer++
    return nil
}
```

What changed since ADR-016 is `applied`: an explicit `apply` now calls
either kind (`MarkApplied`, `takeAppliedMark` — whose comment records that
without it "the gate let ORIGIN decide … which ADR-016 forbids"). The
difference the ADR named was not removed, only made answerable: a
function value of no parameters that nobody applied is called where it is
stepped if `fn` made it, and is data if `=>` made it.

```
fn [[] [Integer] [42]]                     → 42
[] => [42]                                 → fn
(fn [[] [Integer] [42]])                   → 42
([] => [42])                               → fn
[(fn [[] [Integer] [42]])]                 → [42]
[([] => [42])]                             → [fn]
def a (fn [[] [Integer] [42]]) ; a/v       → 42       (a is bound to the RESULT)
def a ([] => [42]) ; a/v                   → fn a
def a fn [[] [Integer] [42]] ; a/v         → fn a     (collected by `def`, it was never stepped)
```

So a `fn` value of no parameters cannot be produced by an expression and
then bound, stored in a list or handed to a word that hands it back: the
moment it is stepped with nothing waiting for it, it is spent. A word
that collects one and returns it shows the same thing — `dupq f/v` calls
each copy of a named one and leaves two functions of an anonymous one.

`core/spec/dispatch.tsv` pins the named kind through `fn( RTYPE [ body ]
)` (all three runners agree), `core/spec/divergent.tsv` the anonymous kind
and the application mark through the `afn0q` and `applyq` fixtures, and
the Lean specification reproduces the gate (`Engine.lean`
`dispatchFnValue`; `Theorems/ZeroArg.lean`
`nur417_where_it_came_from_decides`).

**Proposed verdict:** resolve by fix — one answer for both, and the
anonymous one is the answer that keeps a function a value: unapplied, a
function of no parameters is data where it stands, and a NAME bound to it
or an `apply` calls it. (The other uniform answer — both are called where
they stand — would make `([] => [42])` unbindable too.)

---

## NUR418 — unbinding a type does not release its name {#nur418}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · loud (a `type_error` at the redefinition)

**Rule:** `undef` undoes `def` (REFERENCE.md's word table: "`undef` —
Remove the latest definition"): the name's newest binding is removed and
what it shadowed, or nothing, is left. A name with no binding is free.

**Divergence:** a type's name is reserved as well as bound, and `undef`
removes only the binding. `InstallType` registers every part of the name
(`core/go/core_type.go`, the `RegisterPart` loop at its end) and
`validateTypeName` refuses a name with a registered part — unless the
name is a type binding NOW, which is what lets a redefinition through:

```
def Foo 1 ; def Foo 2 ; 2 is Foo            → true      (a redefinition stacks)
def Foo 1 ; undef Foo ; Foo                 → undefined word: Foo
def Foo 1 ; undef Foo ; def Foo 2           → type_error: name part "Foo" in "Foo" conflicts with an existing type name
def Foo Integer ; undef Foo ; def Foo String → the same
```

So the one sequence that leaves a name unbound is the one after which it
cannot be bound. `undef` retires the minted node (`TypeTable.Retire`) and
leaves `Types.parts` alone; nothing ever removes a part but the analysis
pass's own rollback (`ForgetTypePartsSince`).

`core/spec/divergent.tsv` pins it through the `typeq` fixture, and the
Lean specification states it (`TypeInstall.lean` `registerPart`;
`Theorems/TypeInstall.lean` `nur418_unbinding_does_not_release_the_name`).

**Proposed verdict:** resolve by fix — a part is reserved while a binding
that registered it is live: `undef` of a type binding releases the parts
no other live type binding of the registry holds.

---

## NUR419 — a named type does not have its pending operand evaluated {#nur419}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · loud (`no signature matches`) but wrong

**Rule:** NUR235's, as fixed: a pending list or map standing at a slot is
not yet the value the callee receives, so a slot that judges a
container's CONTENTS has it evaluated first (`core/go/match.go`
`patternOperand`). And a name is its content: `def L [:Integer]` gives
the typed list a name, and `x is L` is `x is [:Integer]`.

**Divergence:** the evaluation is keyed on the slot's PATTERN. A typed
list or a map shape written in the parameter list is a pattern; the same
shape under a name is the slot's TYPE (the minted node, `FnParam.Type`),
with no pattern — and the node's membership rule
(`BindingBodyUnifier.Match`) is handed the operand as it stands:

```
def f fn [[n:[:Integer]][Any][n]] ; f [1 add 2]                    → [3]
def L [:Integer] ; def f fn [[n:L][Any][n]] ; f [1 add 2]          → no signature matches (got List; nearest [L])
def L [:Integer] ; [1 add 2] is L                                  → true
def L [:Integer] ; def f fn [[n:L][Any][n]] ; f [3]                → [3]
def f fn [[n:{a:Integer}][Any][n]] ; f {a:(1 add 2)}               → {a:3}
def M {a:Integer} ; def f fn [[n:M][Any][n]] ; f {a:(1 add 2)}     → no signature matches
```

The check pass reports the same `no_signature`, so the two lanes agree
with each other and disagree with `is`. A union with a container among
its alternatives is the same case (`DisjunctUnifier.Match`).

`core/spec/divergent.tsv` pins it, and the Lean specification states it
(`Match.lean` `patternNeed` reads `Sig.pattern?`, which a named type is
not; `Theorems/TypeInstall.lean`
`nur419_a_named_types_pending_operand_is_not_evaluated`).

**Proposed verdict:** resolve by fix — `patternOperand`'s question is
"does this slot's verdict read a container's contents", and a slot whose
type is a node with a content-deciding rule over a container reads them:
ask the node (`TypeContentOf`) where there is no pattern.

---

## NUR420 — an omitted parameter of a named type is given a base value the type refuses {#nur420}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · loud, late (the tape's growth ceiling)

**Rule:** an optional parameter (`n?:T`) adds a signature without it
whose body calls the function with `T`'s base value (`core/go`
`ExpandOptionalSigs`, `omittedDefaultValue`). A parameter
whose type has no base value, or whose own pattern refuses the base
value, adds nothing — `def g fn [[n?:1][Any][n]] ; g` is `no signature
matches`, because 0 is not 1.

**Divergence:** the refusal is the PATTERN's. A parameter of a named
type has the node for its type and no pattern, and the base value is
read off the lattice: `One` (`def One 1`) sits under Integer, so its base
value is 0, and nothing asks `One` whether 0 is a member. The added
signature's body calls `g 0`; the one-parameter signature refuses 0; the
signature of no parameters takes the call again:

```
def One 1 ; def g fn [[n?:One][Any][n]] ; g            → tape_exhausted (the 396718-entry ceiling)
def Big (Integer gt 10) ; def g fn [[n?:Big][Any][n]] ; g   → the same
def g fn [[n?:1][Any][n]] ; g                          → no signature matches
def Z 0 ; def g fn [[n?:Z][Any][n]] ; g                → 0       (the base value happens to be the member)
def U (Integer tor String) ; def g fn [[n?:U][Any][n]] ; g  → no signature matches (a union has no base value)
```

The Lean specification states the expansion as the kernel makes it
(`FnSpec.lean` `expandOptional`; `Theorems/TypeInstall.lean`
`nur420_an_omitted_parameter_of_a_named_type_is_given_a_base_value_unasked`).
No corpus row pins it: the answer is a resource limit.

**Proposed verdict:** resolve by fix — the expansion asks the parameter's
type, not only its pattern, whether the base value is a member
(`v.Is(node)`), and adds nothing when it is not: the inline form's answer.

---

## NUR421 — a singleton or structural type passes for a value of the type it sits under {#nur421}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (a wrong number)

**Rule:** a type is not a value of itself: a type literal is refused at
every slot that reads a payload (`core/go/signature.go`
`rejectsTypeLiteral`, whose comment gives the reason — "handlers that
read its payload via AsX() would silently pull the zero value. That used
to make programs like `addq Integer 1` quietly compute `addq 0 1`
instead of raising").

**Divergence:** the refusal has an exemption for a node "carrying
recorded type CONTENT" — written so that `refine Table R` still collects
a record's name where it collected the record's body — and every
singleton and structural type is such a node. It is minted under the
node its content sits at, so the slot's lattice test admits it, and the
handler reads the payload it does not have:

```
def One 1 ; One add 1                                     → 1        (0 + 1)
def Seven 7 ; Seven mul 3                                 → 0
def L [:Integer] ; size L                                 → 0
def S "abc" ; size S                                      → 0
def One 1 ; def f fn [[n:Integer][Integer][n add 1]] ; f One   → 1
Integer add 1                                             → no signature matches
def P (refine Integer) ; … P at an Integer slot           → refused (a newtype records no content)
```

The singleton is neither read as its literal (`One add 1` is not 2) nor
refused. A union's node sits under `Type/Disjunct` and is refused by the
lattice test, so only the kinds minted under a value type show it.

`core/spec/divergent.tsv` pins it through `typeq` and the Integer-slot
fixtures, and the Lean specification states it (`Match.lean`
`rejectsTypeLiteral`; `Theorems/TypeInstall.lean`
`nur421_a_singleton_passes_for_a_value`).

**Proposed verdict:** resolve by fix — the exemption is for a slot that
WANTS a type's content (a `Node`-typed type argument such as `refine
Table`'s), not for every slot: keep the refusal at a concrete-payload
slot whatever the node records, and let the words that take a named
structure declare it (a type-arg slot).

---

## NUR422 — the meet of two named types depends on their order {#nur422}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (a different answer for the swapped operands)

**Rule:** unification is the MEET of its two operands: it does not
depend on their order, and it is a function of what they denote.
`core/go/unify_disjunct.go` states the disjunct's rule so — "the meet is
the disjunct of the meets of its alternatives with that value — the set
intersection, and what `tand` answers for the same pair" — and
REFERENCE.md names `tand` the intersection.

**Divergence:** two named types are settled by the LEFT operand's rule
(`dispatchUnifier` walks from each denotation in turn and takes the first
Unifier). A union's rule (`DisjunctUnifier.Unify`) puts the other operand
to its alternatives; a singleton's or a structural type's
(`BindingBodyUnifier.Unify` → `unifyMembership`), facing another TYPE,
"settles by the structural rule" — the lattice, where no minted type is
under another:

```
def One 1 ; def Foo (Integer tor String)
One unify Foo                      → One        (Foo's rule: Integer takes the singleton)
Foo unify One                      → unify-fail (One's rule: the lattice)
One unify (Integer tor String)     → 1          (the union in place is a VALUE to One's rule)
(Integer tor String) unify One     → 1
One tand Foo  /  Foo tand One      → 1
def L [:Integer] ; def Foo ([:Integer] tor String)
L unify Foo                        → [:Integer]
Foo unify L                        → unify-fail
```

(An infix `a unify b` hands `b` to the first slot, so the kernel's left
operand is the one written second.) Three answers for one pair of types —
the type, the value, nothing — by order and by whether the union has a
name.

`core/spec/divergent.tsv` pins the kernel's answers, and the Lean
specification states them (`Unify.lean` `unifyInner`, the two-named-types
arm; `Theorems/TypeInstall.lean`
`nur422_the_meet_of_two_named_types_depends_on_their_order`).

**Proposed verdict:** resolve by fix — a type-level pair in which either
side is a union node is the union's (try both operands' rules for the
one that owns a type-level answer, as the both-bare-nodes case of
`dispatchUnifier` already does for `OptNum unify None`), and the answer
for a singleton against a union is one thing whichever is named.

---

## NUR423 — a union met inside a typed container comes back as a zero {#nur423}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · silent (a wrong value)

**Rule:** the meet of a list with a typed list is, element by element,
the meet of the element with the child type
(`unifyTypedListWithConcrete`); and `Integer tor String` against
`Integer` is `Integer` — which is what the same pair answers outside a
container, and in a plain map (`{a:(Integer tor String)}` against
`{a:Integer}` is `{a:Integer}`).

**Divergence:** each element's meet is passed through
`reparentSwappedElem` (`core/go/unify_list.go`), written for a VALUE
narrowed into a newtype: "Unify of a concrete element against a
bare-refine child returns the TYPE LITERAL … Return the source element
reparented to the canonical minted type instead." Its test is
`IsBareTypeNode(&unified) && IsConcrete(&src)` — and a union VALUE is
concrete. So when a union element meets the child as a single bare type,
the union's payload is retagged with that type and handed back: a value
whose type is Integer and whose payload is a `DisjunctInfo`. Every
reader takes it for the type's zero:

```
def U (Integer tor String) ; (quote [U]) unify [:Integer]     → [0]
(Integer tor String) unify Integer                            → Integer
core/spec:  meetq [q d( Integer String ) ] [: Integer ]       → [0]      core/ts: [Integer]
            meetq {q a: d( Integer String ) } {: Integer }    → {a:0}    core/ts: {a:Integer}
            meetq [q d( String Boolean ) ] [: Scalar/String ] → ['']     core/ts: [String]
            meetq [q d( 1 2 ) ] [: Integer ]                  → [(1 tor 2)]   (the meet is no bare type: untouched)
```

Found by the specification's differential fuzzer
(`spec/tools/diff-fuzz.py`) once it generated named unions inside lists.
core/ts and the Lean specification answer the meet; the five rows are in
`core/spec/divergent.tsv` with the specification on core/ts's side.

**Proposed verdict:** resolve by fix — the retag is for a plain value:
`reparentSwappedElem` returns `unified` untouched when the source is
itself a type (`IsTypeBody(&src)`), in the typed-list and the typed-map
walk alike.

---

## NUR424 — a word literal written after a word is not claimed by a slot of a named type {#nur424}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · loud (`no signature matches`) but wrong

**Rule:** the one argument rule (CLAUDE.md): a call form only chooses
where the split falls — `f a ≡ a f`. And a named type is its content:
`def Foo (String tor Boolean)` admits what `(String tor Boolean)` admits.

**Divergence:** the plan-time scan of the tokens after a word
(`core/go/collect_kernel.go` `CollectCandidateScan`) does not resolve
`true`, `false` or an unbound name: it asks the slot whether "a Boolean"
— "an atom" — would fit, with a PROBE that has the type and no payload
(`booleanProbeDatum`, `atomProbeDatum`: `Datum{Parent: TBoolean}`). A
builtin slot answers by the lattice and is satisfied. A named type's
membership rule treats a value with no payload as a TYPE — "the type
itself, not an inhabitant" (`DisjunctUnifier.matchR`,
`matchMembership`) — and asks the lattice whether Boolean is under the
minted node, which nothing is:

```
def Foo (String tor Boolean) ; def f fn [[n:Foo][Any][n]]
f true          → no signature matches (got Word; nearest [Foo])
true f          → true
f (true)        → true
f "a"           → a
true is Foo     → true
def f fn [[n:(String tor Boolean)][Any][n]] ; f true     → true     (the union in place is a pattern)
def T true ; def f fn [[n:T][Any][n]] ; f true           → no signature matches
def T true ; def f fn [[n:T][Any][n]] ; true f           → true
```

The check pass reports the same `no_signature`. Found by the
specification's differential fuzzer once it drew a function of a named
type called on `true` (`spec/tools/diff-fuzz.py`).

`core/spec/divergent.tsv` pins it, and the Lean specification states it
(`Match.lean` `scanAdmits`, the probe; `Theorems/TypeInstall.lean`
`nur424_a_word_literal_is_not_claimed_by_a_slot_of_a_named_type`).

**Proposed verdict:** resolve by fix — for a slot whose type has a
content-deciding membership rule (`HasConstraintUnify`), the scan asks
with the VALUE the word literal denotes (`true`/`false` are known at
plan time; an unbound name is its atom), as it already does for every
literal token that is not a word.

---

## NUR425 — a typed container over a singleton's name is dead in a declaration {#nur425}

**Status:** Pending · **Recorded:** 2026-11-28 · measured through the shipped words · loud (`no signature matches`, or a `type_error` on a return) but wrong

**Rule:** a name bound to a type is that type wherever a type is read:
`def One 1 ; [1] is [:One]` is `true`, because `[:One]` is a list of the
type whose one member is 1.

**Divergence:** a typed container written as a parameter's or a return's
declaration has its child resolved when the specification is read, by a
cascade of its own (`core/go/generics_unify.go` `ResolveSigChildParam`):
a name that is a type binding is replaced by the binding's body **only if
`IsTypeBody` accepts the body**. A union, a typed container and a bare
type are type bodies; a literal is not — it is admitted as a type's body
by the separate `IsLiteralTypeBody` test the installer runs. So over a
singleton's name, or a list or map literal's, the child is left the WORD,
and the pattern is a container of an unresolved name:

```
def One 1 ; [1] is [:One]                                    → true
def One 1 ; def f fn [[m:[:One]][Any][9]] ; f [1]            → no signature matches (got List; nearest [List])
def One 1 ; def f fn [[m:{:One}][Any][9]] ; f {a:1}          → no signature matches
def One 1 ; def f fn [[n:Any][[:One]][n]] ; f [1]            → type_error: return value 1: expected [:word(One)], got List
def One 1 ; def L [:One] ; def f fn [[m:L][Any][9]] ; f [1]  → 9        (the installer resolves the child)
def U (Integer tor String) ; def f fn [[m:[:U]][Any][9]] ; f [1]   → 9  (a union's body is a type body)
def Shp {a:1} ; def f fn [[m:[:Shp]][Any][9]] ; f [{a:1}]    → no signature matches
```

The return's message prints the unresolved word. Found in review of the
specification's model, which had resolved the child as a literal's shape
is resolved everywhere else; the kernel's answers are pinned in
`core/spec/divergent.tsv` and stated in the Lean specification
(`FnSpec.lean` `resolveSigChild`; `Theorems/TypeInstall.lean`
`nur425_a_typed_container_over_a_singletons_name_is_dead_in_a_declaration`).

**Proposed verdict:** resolve by fix — the cascade's test is "is this
name a type binding", which `TopTypeBody` already answers: substitute the
body whatever its kind, as the installer's own resolution of `def L
[:One]` does.

---
