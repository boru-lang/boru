# GO-TS-PARITY.0 — full functional parity on core, parser and basic

**Status:** IN PROGRESS (started 2026-08-08) · **Instruction:** "full
functionality parity go and ts on modules core, parser and basic, with
100% test coverage on go and ts", and — the steer that shapes the whole
approach — "use shared tsv spec files as much as possible to establish
parity".

Sibling notes: [legacy/TS-PARITY-AUDIT.0.ignore](legacy/TS-PARITY-AUDIT.0.ignore) (the audit
that built the parser stream oracle), [legacy/CORE-TS-COVERAGE.0.ignore](legacy/CORE-TS-COVERAGE.0.ignore),
[legacy/BASIC-CHECK-CUT.0.ignore](legacy/BASIC-CHECK-CUT.0.ignore) (the dependency cut that
preceded this).

## The measurement, and why the obvious one misleads

| module | go | ts | shared corpus |
|---|---|---|---|
| core | 100% | 99.57% | `core/spec`, 373 rows + a 16-row ledger |
| parser | 100% | **100% lines / branches / functions** | `parser/spec`: 698 parse, 27 raw-lexer, 56 data-decode, 18 generated-depth, and 26 structural-shape rows; a live 9-row ledger |
| basic | 100% | 100% *of the 17 words ported* | `basic/spec`, 69 rows |

Two numbers that look like progress and are not:

- **`basic/ts` at 100%** is 100% of the 17 words ported (the stack
  vocabulary plus `do`, `error` and `if`), not of basic's surface. The floor is
  a ratchet on the SURFACE, not on the percentage.
- **Both crossdiffs agree on every row** — `parser-crossdiff` IDENTICAL
  over 1765, `crossdiff` 1808 agree / 0 divergences — and did so on day
  one. That is not evidence of parity. It means the engines do not
  disagree about what the corpora COVER, so **the parity gap is the
  uncovered surface**, and growing the corpora is the instrument rather
  than a formality.

The honest surface measure is the export gap: core/go exposes ~2,989
functions and types, core/ts ~192. parser is the exception — 4,380 Go
source lines against 5,021 TS — which is why parser reached an empty
divergence ledger inside a day and core will not.

## What the corpora are, and the rule that makes them work

Three corpora, each read by TWO runners that share **no code**:

| corpus | go runner | ts runner |
|---|---|---|
| `core/spec` | `core/go/corespec_test.go` | `core/ts/src/corespec.test.ts` |
| `parser/spec` | `parser/go/parserspec_test.go` | `parser/ts/src/parserspec.test.ts` |
| `basic/spec` | `basic/go/basicspec_test.go` | `basic/ts/src/basicspec.test.ts` |

The independence is the point: shared scaffolding hides the same bug from
both engines. Each runner re-implements the row notation and builds its
own fixture registry, and any asymmetry between the two fixtures shows up
as a false divergence — so the fixtures are kept in step deliberately.

`core/spec` gained the same ledger on 2026-08-08 —
`core/spec/divergent.tsv`, 135 rows, same two-runner semantics — after a
sweep of core/ts's uncovered regions turned up 135 divergences in 138
well-formed candidates. `design/CORE-TS-DIVERGENCES.1.md` has the ten
classes. That hit rate is the rule at the top of this note restated as a
number: the uncovered surface was not merely untested, it was WRONG.

`parser/spec/divergent.tsv` is the parity DEBT ledger: one row per
divergence, both columns recorded, each runner asserting its OWN column.
Shrink-only. A fixed divergence MOVES to `parse.tsv` rather than being
deleted, and a row whose two columns are equal FAILS — otherwise the file
stops being an honest debt list. It reached zero on 2026-08-08, briefly
took eleven measured rows back, and was empty again by 2026-08-09. Eight
rule-step-limit shapes now preserve the offending token from the TS rule
subscriber, two decimal/underscore lexer boundaries are claimed by matching
boundary shims, and the former depth-501 gap is covered more honestly by
the generated `nesting.tsv` boundary matrix than by a multi-kilobyte
literal.

Corpus-zero did not mean language-zero: a 2,587-source probe sweep the
same day measured 55 divergences (~2.1%) on inputs OUTSIDE the corpus —
trailing-`=>` fold loss, two accept/reject splits, recovery-token detail,
error precedence, and an internal type-name leak — and follow-up probing
found an empty-`${}` template-fold class the sweep's seed missed. The
ledger carried one representative row per class (9 rows) and 50
probe-AGREED neighbors were promoted into `parse.tsv`. **All nine classes
are fixed in both ports (NUR060, 2026-09-26)**: each row moved to
`parse.tsv` with neighbours, and the ledger is empty again. The safe DATA-decode seam had two asymmetries no shared row
could express — TS reordering integer-like map keys, and Go alone wrapping
sign+separator runs like `+_1` as numbers — and both were DEPENDENCY
defects, fixed upstream in jsonic v0.6.0 / parser v0.8.0 (ADR-014) and now
pinned by rows in `data.tsv` rather than described in prose. See
parser/spec/README.md §"The current debt" for the class table.

`scripts/parity-probe.sh` is how a row gets written: it runs a candidate
through both engines and prints AGREE with the shared render, or DIFFER
with both. **Authoring an expected column from one engine's behaviour is
how a divergence gets baselined as a contract**, which is the failure
this whole apparatus exists to prevent.

## What the probe found that the crossdiff could not

Recorded because the pattern generalises: a differential that hard-fails
only when both engines SUCCEED with different values is blind to two
engines that both fail, or both render debug output.

| defect | why crossdiff missed it |
|---|---|
| `TBigInteger`/`TBigDecimal` had types but no constructor and no render arm in core/ts | both engines "succeeded"; the render was wrong on one |
| `1e400` → `inf` in TS, `float_overflow` in Go | a GAP (one errors), which crossdiff permits |
| typed containers dropped the tag (Go) / leaked `word(...)` (TS) | both rendered; neither was source |
| `1 ;` canon'd to a bare `1` in Go, barrier silently gone | both "succeeded" |
| `/q` marker: `word()({false true})` vs `word(undefined)` | both erroring-by-rendering |
| stray `)` dropped to empty in Go | both "succeeded" |
| `1_e5` → 100000 in Go, REJECTED in TS | a GAP, permitted |
| a RUNTIME map had its values evaluated in TS, not in Go | no eng/spec source builds a non-eval map, so the row did not exist |
| a map value with ZERO residuals kept its key in TS, dropped it in Go | same |
| `<a b=${}/>` folded to `""` in Go, stayed a hole in TS | both rendered; neither was source |
| `[Map<]`, `[(1]` ACCEPTED by TS, rejected by Go | TS produced an empty stream, so there was no value to disagree about |
| a `${…}` conversion error rendered its halves in the opposite order | both errored, with the same code |

In two of five original divergence classes the **`go` column was not the
reference**. The ledger header warned that it was "the reference by
convention, not by proof"; that warning was load-bearing.

## The blocking structure (the finding that scopes the rest)

`basic/ts` cannot advance much further on its own. Every remaining word
is gated on a core/ts capability that does not exist yet. Measured, not
guessed:

| basic word group | needs from core/ts | present? |
|---|---|---|
| stack (14 words) | value constructors, `returnsIdentity`, `fullStack` | **DONE** |
| `do` (runtime half) | a sub-engine run, `newErrorValue` | **DONE** |
| `const` | member types (`MintMemberType`), the type table, `reparentValue`, `canonicalType`, `BoruErrorHint` | no |
| `do` / `if` / `case` / `for` | the CARRIER LATTICE — `joinCarriers`, `runCarrierBody*`, `applyGuardNarrowing` — for their analysis halves | no |
| `def` / `undef` / `var` / `fn` / `afn` / `fnsig` | `installType`, `installFnDef`, the def store's type bindings | no |
| type-generics (`gen`/`extends`/`default`/`of`) | schemas, type params, instantiation | no |
| content types (temporal, micron, bytes, handles, resource) | `makeObject`, class/resource instances, Behaviors, the capability table | no |

So the order is forced: **core/ts first, basic/ts follows.** Each
increment is (1) port a core capability, (2) pin it with `core/spec` rows
in both runners, (3) port the basic words it unblocks, (4) pin those with
`basic/spec` rows. `fullStack` is the worked example — core/ts had no
such dispatch mode, so `depth`/`pick`/`roll` waited for it, and the
corpus carried no rows for them in the meantime. **Writing rows against a
capability that does not exist encodes the gap as a contract**; the
absence of those rows was itself the honest record.

## Deviations that are not bugs

- **BigDecimal scale.** Go's payload is an `apd.Decimal`, TS's was a
  binary64 — so `0d1e400` overflowed to an unparseable `0dInfinity` and
  `0d1e-400` underflowed to **zero**, silently. Closed by giving core/ts
  an exact decimal. Recorded here because the first attempt documented it
  as "trailing-zero scale alone", which was false, and the review caught
  it: a deviation note that understates the deviation is worse than none.
- **Uncoded errors.** `pick`/`roll` out of range raise `fmt.Errorf` in
  Go, so they surface as `non_boru`. The TS port throws a plain `Error`
  to match EXACTLY. Giving TS a coded error would look like an
  improvement and be a divergence. That the rest of the layer is coded
  and these two are not is a non-uniformity in its own right.
- **Value identity.** Go's `ReturnsIdentity` mints a fresh `Value.ID` for
  a duplicated source index so `dup`'s outputs stay distinct for the
  bytecode emitter. core/ts Values carry no ID and there is no TS
  compiler to consume one, so that half is absent rather than stubbed.
- **The resolved rule-step cap (NUR061).** The tabnas TS rule engine bounds its main loop
  and, on reaching the bound, STOPS: the trailing-token check then sees
  `#ZZ`, so nothing is thrown and the partial root is returned. Shapes
  that leave a group open with a terminator that cannot close it —
  `[Map<]`, `{a: (1}`, `[1 (2]` — hit it, because the TS val rule's
  implicit-null alternate matches the `]`, backtracks, and the enclosing
  elem re-pushes forever. TS was ACCEPTING these: `[Map<]` parsed to an
  empty value stream, `Map<]` to a bare `word(Map)`. `parse()` now
  watches the step count through a `sub.rule` subscriber and raises when
  the parse ended exactly at the library's own bound — EQUALITY, not
  `>=`, so a library change stops the guard firing rather than starts it
  firing early. The subscriber's `ctx.t0` retains the exact closer and
  location, so the guard now raises the byte-identical Go detail and rich
  metadata; all eight rows moved into `parse.tsv`.
- **The resolved nesting gap (NUR061).** The first diagnosis blamed the
  tabnas rule engine and capped TS at 500. Directly parsing 10,000 levels
  proved the engine is iterative; the overflow was the boru conversion
  walk around 900. TypeScript conversion now uses an explicit bottom-up
  work stack while retaining Go's logical depth accounting. Both accept
  lists/maps through 10,000, reject 10,001 cleanly, and root parentheses
  accept 9,999 because the implicit item frame also counts. Independent
  generators in both runners pin list, map, paren, typed, and mixed shapes
  in `parser/spec/nesting.tsv` without recursively rendering deep values.
- **The resolved numeric-separator gap (NUR061).** Both ports had treated
  `_` beside `x`/`e` as if it were between digits, despite REFERENCE.md's
  rule. Both ports now validate actual digits in the literal's base and use
  matching high-priority shims for the dependency-specific decimal token
  boundaries.
  A 66-case shared TSV sweep pins fraction, exponent, base, sign, and
  malformed-name boundaries.

## Coverage, and where it must come from

Both Go modules gate at 100% by their OWN suite (`cover-gate-core`,
`cover-gate-parser`), on top of the merged ADR-008 gate. The parser TS gate
now requires **100% lines, branches, and functions**, with an import manifest
that prevents an unimported production file from escaping Node's coverage
universe. CI installs Node dependencies and runs the combined
`parser-parity` target (both standalone gates plus `parser-crossdiff`). The
other TS gates remain line ratchets: `TS_CORE_GATE_LINES` and
`TS_BASIC_GATE_LINES`.

The discipline that matters: **coverage comes from corpus rows, not from
per-engine unit tests.** When core/go's canon grew arms for typed
containers, the marker values and the dispatch modifier, `cover-gate-core`
went red — and the fix was `core/spec` rows with new expression kinds
(`bigint`, `bigdec`, `end`, `closeparen`, `typedlist`, `typedmap`,
`dispatchmod`) in both runners, not Go tests. Every such row lifts both
engines at once, which is the whole reason the corpus is the instrument.

One consequence worth stating: a row can legitimately fail on BOTH
engines. `core/spec` and `basic/spec` are SPECS, not differentials — the
expected column is the documented contract — which is exactly the defect
class an agreement-only corpus is structurally blind to.

### The narrow exception, and why it is narrow

`parser/ts` reached 100% with 165 new corpus rows and TWO unit-test files:
`guards.test.ts` and `convert-guards.test.ts`. They exist because Go
carved the same exception first, in `parser/go/grammar_seam5b_test.go`,
and gave the reason: the jsonic grammar actions and lex matchers are
ordinary closures, and a guard that no source can provoke — a rule with
no parent, a matcher invoked with no rule, a converter arm for a node the
grammar never builds — has to be called directly with a synthetic rule.
Go gets that in-package; TS arranges it by exporting from `convert.ts`,
which is module-internal rather than package-public. The package surface now
also carries the Go host seams (`LexTokens`, config/data safe parsing,
Plainify, and ConvertParsedNumber); converter test hooks remain
unexported from `index.ts`. (`GuardMake`/`guardMake` was in that list
until the 2026-08-10 tabnas upgrade retired it **in both ports**: its
whole signature existed to run a caller's constructor under the Go
construction mutex, and the mutex went when upstream made the instance
counter atomic. The TS twin was a pass-through mirroring it with no
production caller, so deleting only Go's would have left a no-op propped
up for symmetry's sake — ADR-014.)

The rule is: an arm belongs in a guard file only if no source text can
reach it. Several arms started in `guards.test.ts` and MOVED OUT to
`parse.tsv` once the probe found a shape that reached them — the
map-value dot chains most notably, where the top-level rows that looked
like they exercised the `dotchain` rule did not, because everywhere but a
map value a flat `a.b` is folded by `convertTopLevelItems` instead.

Two structural facts put arms in the unreachable class, and both are
worth knowing before adding to a guard file: the grammar wraps every
number token in a `NumberVal`, so the RAW number and boolean arms of the
value converters are for nodes that never exist; and `parseWord` carries
its own numeric classification that jsonic's number lexing always
shadows.

### The general lesson from the sweep

**An uncovered branch in one port is where a divergence hides.** Every
defect above was found by probing the regions the coverage report called
uncovered, and the reason is structural: nothing has ever compared the two
engines there. Coverage and parity are not two goals that happened to be
pursued together — chasing the first is a search strategy for the
second.

## What `core/spec` grew to reach the engine

`core/spec` was 84 rows of scalars and one-word dispatch, which is why
`engine.ts` sat at 68%: the corpus could not express a CONTAINER. The
notation now has three bracket forms (`core/spec/README.md`) — `[ … ]` /
`{ … }` for the PARSER's containers, `[q … ]` / `{q … }` for the
runtime's, and `p( … )` for a paren-EXPRESSION value as against the paren
markers — plus `;` for the end marker.

The bare-versus-`q` distinction is the whole subject, and it is where
core/ts was wrong twice: `deepEvalData` descended into a map REGARDLESS
of its Eval flag (Go gates both the list and the map arm on
`Eval && !Quoted`), and a map value that evaluated to zero residuals kept
its key as an empty list where Go drops it. `core/ts/src/engine.test.ts`
had BASELINED the first — a per-engine unit test pinning one engine's
behaviour as the contract, which is the failure mode the two-runner
corpus exists to prevent.

Worth recording as a caution: the `;` rows do NOT reach
`completeForwardPartial`, because `findPendingMarker` returns nothing for
the shapes the current fixture vocabulary can build. They earn their place
by pinning the marker semantics on both engines, but they are not the
coverage they look like — check what a row actually REACHES rather than
what it appears to exercise.

## What closed core/ts's engine gap

`engine.ts` went 68% → 99% (file), core/ts 90.66% → 99.58%, in the order
the capability table forces. Six defects surfaced on the way, each found
by probing an uncovered region rather than by reading code:

| defect | what was wrong |
|---|---|
| `/q` word capture | core/ts coerced a quoted forward Word to an Atom UNCONDITIONALLY. Go coerces only when the raw Word does not already fill the slot, so an `Any`-typed `/q` slot hands the handler a live Word — measured across the slot lattice: `Any` → the Word, `Atom`/`Scalar` → the Atom, `Word`/`String`/`Integer` → no match at all. |
| map-argument values | core/ts resolved names EVERYWHERE via `resolveWordsDeep`; Go resolves them in exactly two INERT shapes and treats every other value as a program. `{a: [q x]}` rewrote a data list's words, and `{a: )}` was carried into the consuming word instead of faulting. |
| unfilled forward marker | core/ts returned the marker itself as a program residual — `forward(bothq,1/2)` — where Go raises `signature_error`. Reachable whenever a word between the marker and its operands produces no residual, which every fn body's frame tail does. |
| check-mode placeholder | the lenient undefined-word arm advanced the pointer past the placeholder it had just written, so nothing downstream saw it and a pending forward stranded under a pass whose whole job is to keep going. Go leaves its pointer alone and says so in as many words. |
| `dataEqual`'s bigint arm | dead: bigint is a JS value type, so the identity test above it already held. Removed rather than covered. |
| two `boomq { a: ) }` ledger rows | filed under the strict forward barrier; really the map-argument defect. Closed, ledger 18 → 16. |

Two of the six were found only because a test that CLAIMED to exercise a
path did not: `idq true` takes the direct dispatch, not the marker, because
the matcher resolves a keyword before the slot test. Check what a row
reaches, not what it looks like it exercises — the same caution the `;`
rows earned above.

The fixture vocabulary grew with the corpus, and each addition was forced
by a specific unreachable region rather than added for symmetry: `qanyq`
(an `Any`-typed `/q` slot), `tyq` (a type-ARG slot), `tpatq` (a
type-literal pattern on a stack slot), `bothq` (two gradual forward slots
— one-arg words cannot reach the collection loop's middle).

## The blind spot, named and measured

The corpora were built to compare what both ports could already do, and
that is exactly their limit. Asked why the shared specs never revealed
that core/ts's unifier is a 40-line predicate against core/go's ~2,000-line
per-family dispatcher, the answer was structural: **no corpus row reached
unification at all.** core/spec could name seven builtin type literals and
no type CONSTRUCTOR; basic/spec covers 17 words and the main unify client
(`case`) is unported *because* unify is; eng/spec exercises `is` in 607
rows that all agree, because the corpus was written from what the engines
already did.

Two instruments closed the hole. `unifyq` plus `d( )` / `[: ]` / `{: }`
opened the CORE unifier to core/spec — 70 of 70 agreed over the shared
subset (the port is faithful where it is implemented) and 23 of 35 differed
the moment a constructor appeared. Then a 980-source sweep opened the
ENGINE surface: **136 verified divergences, 131 distinct sources, not one
covered by an eng/spec row** — 102 wrong answers, 26 gaps, 3 code diffs.
[legacy/ENGINE-BLIND-SPOT.0.ignore](legacy/ENGINE-BLIND-SPOT.0.ignore) is the account.

Three things from it are worth carrying even if the tables are never read:

- **The crossdiff is blind in exactly the direction a missing capability
  fails.** It hard-fails only when both engines succeed and the values
  differ. A capability that is ABSENT does not return a wrong value — it
  refuses, and a refusal against a success is a GAP, which is logged and
  PERMITTED. 26 of 131 divergences are gaps; nine of those were uncoded
  host crashes, the class the instrument treats most leniently.
- **Double-blind AGREE is worse than either.** `case` is absent from
  BOTH fixtures, so both answer `undefined_word` and the differential
  reports agreement over a word neither engine ran. The comparison words
  (`gt`/`lt`/`eq`) are likewise absent from both, which makes core/go's
  entire dependent-scalar unifier untestable by construction.
- **A measurement is only as good as its harness.** `boru run -e` is the
  full lang layer with a DIFFERENT `is` from the one the crossdiff
  compares. One of the two divergences that seeded the sweep —
  `5 is ( refine Integer )` — is `true`/`true` on the path the crossdiff
  actually uses, and was withdrawn. Rebuild the harness, do not reach for
  the CLI.

## Open

- **core/ts's last 0.4%**: nine line-ranges in `engine.ts`, all in the
  same class — arms whose preconditions the current fixture vocabulary
  cannot construct. The forward-collect type mismatch needs a marker
  parked at a slot the resolved value then fails, which the deferral rule
  makes unreachable; `completeForwardPartial`'s fire arm needs
  `collected ≥ expectedForward` on a marker that is by definition still
  collecting. These want a proof-carrying exemption of the kind ADR-008
  gives Go (`//covergate:allow`) and TypeScript has no equivalent for —
  that mechanism is the next thing to design, not more tests.
- **basic/ts**: everything past the stack vocabulary, the escape hatch
  and `if`. The next word is `case`, and it is blocked on a SUBSYSTEM
  rather than a primitive: `CaseClauses` matches a clause by `UnifyR`,
  and core/ts has `isValueOfType`/`unifiesValue` over a value subset
  rather than the unifier. Porting unification is its own piece of work
  and should be scoped as one. `for` needs the loop fixed point, and
  `break`/`continue` do nothing but set the flow-control flag only `for`
  reads.

  What `if` demonstrated is worth carrying forward: it was blocked on a
  named PRIMITIVE (the Move continuation), the primitive took an hour,
  and porting it immediately exposed a core/ts gap the corpus had never
  reached — `coerceBoolean` was missing Go's render-as-"false" tail, so
  `if [ false [1] [2] ]` took the THEN branch. **Porting a word is
  itself an instrument**, and a cheaper one than sweeping for coverage.
- **The empty-body question**, now met twice: `do [ ]` and
  `do [ … ] error [ ]` are both absent from `basic/spec` because Go's
  `InvokeBody` and the TS sub-engine disagree about an empty body under
  the fixture registry. One open question, two rows waiting on it.
- NUR059: canon still renders sugar tags, `/r` and `/N` word modifiers,
  and paren groups in debug spelling. Both engines agree, so it is render
  quality rather than parity — pinned by corpus rows so a one-sided fix
  fails loudly.
