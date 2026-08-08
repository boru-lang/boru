# GO-TS-PARITY.0 — full functional parity on core, parser and basic

**Status:** IN PROGRESS (started 2026-08-08) · **Instruction:** "full
functionality parity go and ts on modules core, parser and basic, with
100% test coverage on go and ts", and — the steer that shapes the whole
approach — "use shared tsv spec files as much as possible to establish
parity".

Sibling notes: [TS-PARITY-AUDIT.0.md](TS-PARITY-AUDIT.0.md) (the audit
that built the parser stream oracle), [CORE-TS-COVERAGE.0.md](CORE-TS-COVERAGE.0.md),
[BASIC-CHECK-CUT.0.md](BASIC-CHECK-CUT.0.md) (the dependency cut that
preceded this).

## The measurement, and why the obvious one misleads

| module | go | ts | shared corpus |
|---|---|---|---|
| core | 100% | 88.20% | `core/spec`, 158 rows |
| parser | 100% | **100%** | `parser/spec`, 535 rows, ledger 9 rows (both engine limits) |
| basic | 100% | 100% *of the 15 words ported* | `basic/spec`, 45 rows |

Two numbers that look like progress and are not:

- **`basic/ts` at 100%** is 100% of the 15 words ported (the stack
  vocabulary plus `do`), not of basic's surface. The floor is a ratchet on
  the SURFACE, not on the percentage.
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

`parser/spec/divergent.tsv` is the parity DEBT ledger: one row per
divergence, both columns recorded, each runner asserting its OWN column.
Shrink-only. A fixed divergence MOVES to `parse.tsv` rather than being
deleted, and a row whose two columns are equal FAILS — otherwise the file
stops being an honest debt list. It reached zero on 2026-08-08 and then
took nine rows back, every one a property of the two ENGINES rather than
of either port: one nesting-depth limit, and eight shapes where the TS
rule engine gives up at its iteration bound and cannot name the token it
gave up on.

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
- **The rule-step cap.** The tabnas TS rule engine bounds its main loop
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
  firing early. Both engines now give the same CODE and differ only in
  text, recorded as eight ledger rows.
- **Nesting depth.** Go guards at 10,000 levels; TS refuses at 500,
  because the tabnas rule engine recurses per level and blows the JS call
  stack near 900 — before any converter counter can fire. The TS bound
  converts an uncontrolled `RangeError` into the promised
  `evaluation_limit`. Measured while recording it: 600 parses, 1,000
  overflows. This is the one divergence that is a property of the RUNTIME
  rather than of either port, and the only remaining row in
  `parser/spec/divergent.tsv`. Closing it means making the TS parser
  iterative, not raising a constant.

## Coverage, and where it must come from

Both Go modules gate at 100% by their OWN suite (`cover-gate-core`,
`cover-gate-parser`), on top of the merged ADR-008 gate. The TS gates
ratchet: `TS_CORE_GATE_LINES` (88), `TS_PARSER_GATE_LINES` (**100**, both
halves of the module now gated identically), `TS_BASIC_GATE_LINES` (100, a
surface ratchet).

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
which is module-internal rather than package-public (`index.ts` exports
`parse` and `SrcPos` and nothing else).

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

## Open

- core/ts to 100%: `engine.ts` (~69%) is the bulk, and what remains there
  is whole CAPABILITIES rather than stray branches — fn definitions
  (`dispatchFnDef`, `analyseFnBody`), check mode (`checkModeAssumeSig`,
  `dispatchFnDefCheck`), and interp strings plus XML
  (`evalInterpString`, `substituteInterp`, `resolveXmlTmpl`). Each needs
  the capability reached before a row can exercise it, in the order the
  table above forces.
- basic/ts: everything past the stack vocabulary, in the order the table
  above forces.
- NUR059: canon still renders sugar tags, `/r` and `/N` word modifiers,
  and paren groups in debug spelling. Both engines agree, so it is render
  quality rather than parity — pinned by corpus rows so a one-sided fix
  fails loudly.
