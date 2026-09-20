# Full compilation — the plan re-examined: realism, strategy, algorithm

**Status:** review, point in time. **Recorded:** 2026-09-17, against
`claude/admiring-cerf-x3iv96` at `658fc85` (`main` at `2144fbe`, increment
73, plus PR #471's eight commits: the corpus expansion, `boru build`'s
compile preflight, islands 15 → 12, NUR152's fn-value home, the sentinel
audit). **Asked for:** the maintainer's direction of the same day — *revisit
the full compilation plan and make sure it is realistic; review again the
strategy and algorithmic approach.*

The design is [FULL-COMPILATION.0.md](FULL-COMPILATION.0.md). The
2026-09-14 forecast, [FULL-COMPILATION-ASSESSMENT.0.md](FULL-COMPILATION-ASSESSMENT.0.md),
is the baseline this note re-measures three days later. The running log is
[FULL-COMPILATION-HANDOFF.0.md](FULL-COMPILATION-HANDOFF.0.md). This note
restates none of them: it says what the three days measured, whether the
plan as staged can reach the ruled definition of done, where the strategy
and the algorithm need correcting, and what the corrected staging is. The
staging correction is also written into the design itself as §10's dated
amendment, so the design stays the authority on what is planned.

Every number below was produced by a run on this tree (the langspec
gates, with `BORU_LOG_CENSUS_ROWS=1` for the row listings) or by a source
count; where a figure is the assessment's it is dated.

---

## 0. The verdict

**The architecture is right and the plan is not yet realistic.** The
design's thesis — the compiler's worst verdict becomes *lower generically*,
generic code is the interpreter's own decision procedure over shared kernel
routines, refusal becomes unreachable — has survived 73 increments and every
falsifier that fired (F1 with corrections, F2, the mono call site) without
being refuted, and the three days since the assessment built the first
executing slice of its central mechanism. But three measurements, all new
since 2026-09-14, say the plan as *staged* would not reach "all valid code
compiles, no exceptions" on the schedule the assessment gave, or by the
route the increments have been taking:

- **The daily instruments measure the corpus, not the language.** About
  710 rows written to exercise ordinary idioms — callbacks, code bodies,
  fn-local scope, module composition, the `each`/`fold`/`map`/`filter`
  family — took the corpus from 0 refusals, 0 islands and 28
  interpreter-entering rows to **113, 12 and 54**, and exposed five silent
  miscompiles (NUR152, 154, 155, 156, and NUR153's two regimes). The
  "corpus-native" milestone the assessment put at 70% within a month was a
  property of the old corpus.
- **The generic lane landed and the inventory did not move.** Seven
  increments (60–66) built descriptors for every dispatch family, an oracle
  that executes 473,151 of them live, and `OpDispatchGeneric` routing 676
  corpus dispatches — and the refusal-site census stayed at 92, the
  undeclared-handler census at 114, and the recorder's terminal arm is still
  `MarkUncompilable`. The lane grew from the typed end (binding-sensitive
  sites the typed lowering already compiled) rather than from the refusing
  end, for two structural reasons §3 names, and its own runtime added ten
  named defer arms that land on the whole-program interpreter re-run the
  doctrine forbids.
- **The debt is fn values and code bodies, and the plan has no bulk
  mechanism for either.** Of the 113 refusals, 59 are a fn value the
  emitter cannot follow and 23 are a code body it cannot lower; all 12
  islands and 23 of the 54 census rows are fn values; all five miscompiles
  are fn-value lowerings. Stage 3's "universal fn values" is a per-shape
  sequence of increments, and Stage 6's handler worklist has not moved
  since it was measured on 2026-08-25.

What follows from that, in one line each: change the instrument before
trusting any curve again (§2, §3.5); build fn values as one representation
and handler migration as a census-driven sweep, in parallel (§3.3, §3.4);
finish the generic lane as a *baseline* — total by token class, its own
runtime total, the terminal arm flipping per family — rather than as an
exception path for rebinding (§4); and re-stage on that order with the
gates that make each step measurable (§5). The re-estimate is in §2.3:
roughly 105 to 175 session-days remain to the ruled end state, against the
assessment's 83 to 148, and the year-end probability for T1 + T2 as written
is nearer 15% than the 25% the assessment gave.

---

## 1. What three days changed — measured

| instrument | 2026-09-14 (assessment) | 2026-09-17 (`658fc85`) | what moved it |
|---|---:|---:|---|
| corpus rows | 7,798 | **8,512** | six new spec files: each-variants 141, fold-map-filter 148, code-bodies 142, callbacks 108, fn-locals-scope 100, module-composition 74 |
| rows that compile | 7,475 | **8,053** (346 check-error rows) | |
| corpus refusals (`refusalGate`) | 0 | **113** (80 coverage, 32 soundness, 1 correct-error) | the new rows; the gate was raised to the measured value, its comment recording that the 0 was true only of one-liners that write every callback literally at the call site |
| `OpFallback` islands (`islandGate` 0) | 0 | **12** (15 before increment 3 of PR #471) | the new rows; every one a fn VALUE callback |
| interp-entry census rows (ceiling 27) | 28 | **54** | the new rows |
| engine-entry census (ceiling 277) | 281 | **379** unattributed (Engine.Run ×379, CallBoru ×253, RunResolved ×40, runPooledSub ×34, vm:island ×23, InvokeCallback:callboru ×13, vm:island-resolved ×9) | the new rows |
| compiled differential mismatches | 0 | **5** (NUR154 ×1, NUR155 ×1, NUR156 ×3) | the new rows |
| diagnostic parity / armed-only | 320 / 4 | **358 / 16** | the new rows (`unused_def`, `fn_body_error` classes) |
| type-soundness violations (pin 0) | 0 | **5** | the new rows |
| `MarkUncompilable` sites | 92 | **92** | — |
| undeclared handlers (of 172 relevant, 522 signatures) | 114 | **114** | — |
| `vmDefer(` calls in `eng/go` | 17 | **28** | 11 are the generic lane's own arms (ten names) |
| routed dispatches (`OpDispatchGeneric`) | 0 | **676** (floor 600) | increments 64–65 |
| region oracle, descriptors executed | 47,110 reproduced | **473,151** executed: 446,999 reproduced (94.5%), 9,016 under-claimed (1.9%), 17,130 declined (3.6%), 1 over-claim, 5 value divergences | increment 62 |
| refusal-site dispositions | assigned | 87 generic (stage 5: 26, stage 3: 21, stage 4: 19, stage 7: 11, stage 6: 9, stage 8: 1), 1 trap, 4 delete | the disposition census (`TestCompileFailureDispositionCensus`) |
| work landed | — | `main`: 17 commits, +10,997 / −1,492 non-doc lines, increments 60–73; this branch: 8 commits, +2,931 / −279 | one session each |

Two readings. The rows that fell (refusals, islands, census) did not fall:
they were re-measured on a corpus that finally contained the shapes real
code uses, and every count went up. And the machinery that the design's
end state is defined against — sites, handlers, valves — is exactly where
the assessment left it, after the single largest mechanism in the plan
landed its first slices.

---

## 2. Is the plan realistic against the definition of done?

The ruling (2026-09-14, in SESSION-HANDOVER.0.md): *done is a language
that compiles, as a developer expects; all valid code compiles, no
exceptions.* No carve-outs; "valid" is the interpreter's verdict; computed
code is code. The question is whether the plan, as staged in §10 of the
design and as worked since 08-26, reaches that on any stated horizon.

### 2.1 The corpus is a sample, and it under-measures by construction

The design says so itself (§2.2: totality is proven against the whole gate
inventory, not the rows) and the assessment said so again (§1: "the sample
curve is steep and nearly at its floor; the inventory curve has barely
moved"). What PR #471 adds is the *rate* at which a targeted sample
re-inflates. The six new files were not adversarial; they are the shapes a
developer writes on day one — a callback read from a container, a fn
returned from a factory and handed to `each`, a `do` over a body a fn
returned, a module export applied from main. Per file:

| file | rows | refusals + islands + census rows it contributes |
|---|---:|---|
| callbacks.tsv | 108 | 8 census rows (7 islands), refusals in the fn-value buckets |
| code-bodies.tsv | 142 | 7 census rows (all `RunResolved`: raw-token bodies), the `correct-error` row, NUR154 |
| each-variants.tsv | 141 | 3 census rows (islands), NUR155, the HOF ambiguous-overload bucket |
| fold-map-filter.tsv | 148 | 4 census rows (islands), fn-value refusals |
| module-composition.tsv | 74 | 2 census rows, NUR156 ×3 |
| fn-locals-scope.tsv | 100 | armed-only diagnostic rows, dyn-scope refusals |

That is roughly **one open defect per six targeted rows and one silent
miscompile per 140**. The language surface the corpus samples is 251 core
words plus 264 exports across 11 loadable modules — 522 signatures in the
default registry, 172 of them taking a body, a quoted operand or a fn value
— and the pre-expansion corpus had, per the gate's own comment, exercised
the fn-value and code-body operand kinds almost only in their literal
spellings. A second expansion of the same size aimed at the next idioms
(module bodies, `context`, services, splices, computed bodies) should be
expected to do the same again. **Until the instrument is a generated sweep
over the word inventory and the operand kinds (§3.5), no corpus number is
evidence about "all valid code", and no forecast built on one is
realistic.**

### 2.2 The inventory did not move when the lane landed

The assessment's recommendation 2 was to build the generic lane's first
executing slice next, on the grounds that it is "the only package that
changes the inventory curve's slope". It was built — increments 60 to 66,
in about two session-days — and the slope did not change: 92 sites, 114
handlers, the terminal arm untouched. §3.1 and §4 say why in detail; the
short form is that the lane as built routes dispatches the typed lowering
*already compiled* (a user or native call inside a fn unit whose claim
carries a module-scope word, so the word is looked up live), because
routing anything else collides with the split-identity invariant or needs
an evaluating host that does not exist. The lane cannot become the
terminal arm until it can describe every token class a refusing statement
contains, and the refusing statements are precisely the ones whose operands
are fn values, code bodies and unknown-width results — the three things the
descriptor host declines. So the inventory curve is gated on Stages 3, 5
and 6 as much as on Stage 4, and the spine "2 → {3,4} → 5 → 9 with 6, 7, 8
parallel" understates the coupling.

### 2.3 The re-estimate

The assessment's figures, re-based on what the expansion and the lane's
first slices measured. A session-day is as it defined it (one Claude
session working about one day; 15 to 25 commits). The ranges are wide
deliberately; the low end assumes no negative result, the high end the
observed one-in-seven rate and one falsified premise per package.

| package | assessment (09-14) | this note (09-17) | why it moved |
|---|---:|---:|---|
| to corpus-native (ledger, refusals, islands, census all 0) | 18–33, on the 7,798-row corpus | **30–50**, on the 8,512-row corpus | 113 + 12 + 54 items where there were 57; the fn-value and code-body families need their mechanisms (§3.3, §3.4) before their rows move |
| generic lane, complete (B1) | 15–25 | **25–40** | the evaluating host, the lane's runtime totality and the fn-value lead are measured as the remaining three cores (§4.3); the first slices cost ~2 session-days for the easy half |
| handler migration (B2) | 12–20 | **25–40** | zero movement in 23 days is the honest rate signal; 114 handlers at 3–5 per session-day, with the `do`/`Test` quotation bodies needing Stage 7 first |
| runtime compilation (B3) | 10–20 | **10–20** | unchanged; the unit cache is now on the critical path of B1 and the fn-value convention, so it lands earlier, not cheaper |
| refusal-site retirement (B4) | 15–25 | **10–20** | smaller: with fn values and handlers done as programmes, most of the 87 "generic" sites retire with their family rather than one at a time |
| checker sentinel and traps (B5) | 5–10 | **5–10** | unchanged |
| valves (B6) | 8–15 | **8–15** | unchanged |
| **total remaining** | **83–148** | **~105–175** | |

At the observed cadence (about one session-day per calendar day, with a
second session possible on B2 because it lives in `basic/go` and `lang/go`),
that is **four to six calendar months** to the ruled end state, and about
two to three to corpus-native on the expanded corpus — *if* the instrument
is changed first, since otherwise "corpus-native" will be re-measured
upward again by the next expansion.

Probabilities, as the assessment gave them and labelled the same way
(judgement, calibrated on the rates above):

| outcome | by 2026-10-31 | by 2026-12-31 | by 2027-03-31 |
|---|---:|---:|---:|
| corpus-native on the expanded corpus | 20% | 55% | 85% |
| T1 + T2 as written, on a generated-sweep instrument | under 5% | **15%** (assessment: 25%) | 45% |
| T3 maintained (no known miscompile on `main` at any merge) | 85% | 85% | 85% |

T3's row is lowered from the assessment's 90% because the expansion found
five miscompiles that the 100%-covered suite and every ratchet had passed,
and a generated sweep will find more before it finds fewer.

---

## 3. Strategy review — five findings

### 3.1 The lane grew from the wrong end, and the reason is structural

The design's step 6 is "the former `MarkUncompilable` arm becomes: emit
the statement's compiled collection form". What landed is the opposite
direction: `routeRegion` (`compiler/go/region_route.go`) selects
dispatches the committed `CALL_USER` / `CALL_NATIVE` *already handled* — a
user or mono-native call inside a fn unit with a live module-scope word
slot, a speculative family's lead, a generalised undef's slot — and
replaces their typed lowering with a routed one so the word resolves at
run time. That is the `k` pair, the `OpDispatchGeneric` acceptance test
the design named, and it was the right first slice for one reason the
design gives (§6.2: the reverted first cut proved a routed `10 sub 3`
diverges from `10 3 sub` because the descriptor carries the split, so
routing may only target what the ordinary lowering cannot handle).

But "what the ordinary lowering cannot handle" was then read as "sites
with a rebinding hazard" rather than "sites that refuse", and the two are
almost disjoint on the corpus: the rebind latches fire on zero corpus rows
(§6.2's own tally), while the 113 refusing rows are fn values, code bodies
and provenance. The routing predicate confirms the gap from the other side
— `regionDrivable` admits only scalar value tokens and plain words, no
container, no group, no interpolation, no modifier, no event beyond the
claim — because the VM's descriptor host *declines every evaluation*. So
the lane today can route only statements whose every operand is a
constant or a word, and those statements never refused.

**What to change.** The terminal arm flips *per token class*, not per
site: the milestone is "the descriptor host can drive every slot kind the
corpus's regions contain" (const, atom, type, wordRef — done; local —
done; group and active — 29% of tokens, not done; fn value as lead — not
done), and at that milestone every refusal whose statement is
descriptor-complete takes the lane. That is a different work order from
the disposition census's "generic, stage N" per site: it is one host, then
a sweep. §4.3 sizes the host. And the split-identity worry dissolves
once it is stated precisely: `TestEmitSplitFormsIdentical` is a property
of statements whose lead binding is record-time-stable, where the split is
surface syntax; for a *live* lead the split is semantics (a rebinding to a
word with another barrier changes which spelling collects what), and the
descriptor is right to carry it. The invariant constrains the typed lane,
not the lane that exists because the binding is not stable.

### 3.2 The defer valve has become the lane's landing pad

The doctrine (SESSION-HANDOVER.0.md, 2026-09-16): the silent whole-program
re-run on the interpreter is scaffolding around a known defect, and a
failure that hides itself is worse. The lane's runtime, as built, has ten
named `vmDefer` arms — `vm:generic-unbound`, `-declined`, `-speculative`,
`-claim-drift`, `-word-form`, `-unbound-slot`, `-full-stack`,
`-foreign-native`, `-nout-drift`, `-foreign-unit` — every one of which
lands there, and increment 73's second half made a defer raised inside a
`do` body *propagate* so that the fallback "completes". The `vmDefer(` call
count in `eng/go` went from 17 to 28 while the plan's Stage 9 says the
mechanism deletes. The design's own words are the standard: "the generic
lane is the landing pad, and it is compiled code" (§4); "a defer is slow,
never wrong" (the op's header) is the framing the maintainer corrected.

Some of those arms are honest measurement — the census counts each, and
"the slice's remaining shapes are measured, not guessed" is a legitimate
method. But the plan has to say what each arm becomes, and none of them
may become permanent:

| arm | becomes |
|---|---|
| `unbound`, `unbound-slot` (a lead or slot no binding names) | a raise: the interpreter's `undefined_word`, already built for the slot case (increment 66); the `def`-lead hint is the one tape-only layer, and it is a diagnostic text, not a reason to interpret |
| `declined` (the walk needs an evaluation) | the evaluating host (§4.3) |
| `speculative`, `claim-drift` (the live plan claims other than the record) | a count-generic downstream: the claim's width feeds a mark region (Stage 5), so a different live claim is representable; until then this is the one case the design *motivated* the lane with, and the lane cannot answer it |
| `word-form` (a modified word in the claim) | the modifier handled in the window as the arrival loop handles it (`/q`, `/v` consumption are already the kernel's two in-place edits) |
| `full-stack`, `foreign-native`, `nout-drift` | the native called with the resolved stack presented; a native overload the record did not take runs when its result count is known after the call, as `CALL_NATIVE_POLY` already does |
| `foreign-unit` (a boru signature the program holds no unit for) | compile the unit now, at the value's home, memoised — Stage 7's detached stamp, which is why Stage 7 is on the spine (§5) |

A ratchet should hold this: the count of `vm:generic-*` arms may only
fall, and each retirement names its compiled replacement. The same
discipline the refusal-site census applies to `MarkUncompilable`.

### 3.3 Fn values are the debt, and "universal" is still a list of shapes

The numbers: 40 of 113 refusals are fn-value shapes outright
(function-valued operand 19, fn value reaches word 7, apply over a dynamic
lead 3, `each$body` result above a literal 3, computed closure at an
argument slot 2, unapplied fn value in a body residual 2, application
bounded by a paren 2, apply not at the tail 1, curried chain 1) and 19
more are a higher-order word over a gradual collection whose callback the
recorder cannot commit (`each` 13, `fold` 3, `filter` 2, `scan` 1); all 12
islands are "the callback is a fn VALUE rather than a literal body"
(COMPILABLE-SUBSET.md §5's table); 23 of the 54 census rows island a fn
value (callbacks ×8, fold-map-filter ×4, module-fnvalue-boundary ×4,
each-variants ×3, fn-value ×2, module-composition ×2); and every miscompile
the expansion found is a fn-value lowering — a clause list a fn returned
baked static (NUR154), a typed callback run without the per-element match
(NUR155), a module export's `apply` not firing (NUR156), a stored `=>`
value with two interpreter regimes (NUR153), and the home a value carries
(NUR152, fixed on this branch).

The design's §6.3 is the right answer and has never been built as one
thing: *every fn value carries, from creation, everything application
needs* — an executable ref per signature (a unit, a native impl, or a
lazily-compiled body stamped at first application), and its home. What
exists is a sequence of representations, each one increment: the const
stamp for a def-bound literal, the produced-closure apply, the closure
bridge, the container descent, the module-export mint, the usurp
permutation, the foreign-home decline. Each was correct for its shape and
each was where the next miscompile lived, because a callback seam that
does not know the representation falls to the interpreter (`vm:island`)
or applies the wrong thing (NUR155). NUR152's home stamp and the sentinel
audit's `HasHome` / `FnHomeLookup` / `HomeExportedFn` are the first pieces
of one convention — the *home* half. The *unit* half is the work.

**What to change.** Build the convention at the value, not at the seams:
`NewFunction` (or the Apply kernel's first branch) guarantees that a fn
value applied anywhere either carries a unit of the running program or
obtains one *now*, compiled at its home, memoised by body key and
dependency generation — the detached stamp the design already ships for
handlers, made universal. Every callback seam (`each`, `fold`, `filter`,
`service`, `walk`, the Apply kernel, `InvokeCallbackFn`) then has one
question to ask, and the island arms (`callDynamic`'s third tier,
`RetReplay`, the foreign-unit decline) delete rather than narrow. The
acceptance test is the family, not a row: islands 12 → 0, the 59
fn-value refusals → 0, the 23 census rows → 0, NUR154–156 closed by the
mechanism. NUR153 must be ruled first (which of a stored `=>` value's two
interpreter regimes is the specification), because the stamp can match
only one and today's by-name admission encodes neither.

### 3.4 Handler migration has not started as a programme

Stage 6 is the plan's most enumerable work: 172 declaration-relevant
signatures, 114 with no declaration (code-body 59, quoted 44, fn-operand
11), each one either a declaration (`for-each` took one increment) or a
rewrite of a handler that returns tokens. It is 23 of the 113 refusals
(code-body word 19, dyn-scope `def` inside `do` bodies 3, a `for` body not
captured 1), 14 census rows of raw-token bodies (`code-bodies.tsv` ×7,
`bytecode-migrated.tsv` ×4, `control.tsv` ×2, `word-splice.tsv` ×1), and
the eight `Test.*` quotation-body rows are the same defect one module over.
The `do` note's own name for the shipped answer — "the dyn-body strategy:
the program compiles, the body is interpreted" — is the defect stated.

It has not moved because it belongs to no line: the increments follow
rows, rows live in `compiler/go`, and a handler declaration lives in
`basic/go` or `lang/go` under a word's author. The assessment's B2
estimate (12–20 session-days) assumed a rate that has not been observed
at all. **What to change:** make it a census-driven sweep with its own
session and a ceiling that must fall in every PR touching a listed word;
it is the one package that can run in parallel with the compiler line
without contending for `compiler/go`, and it retires census seams
(`RunResolved`, most of `CallBoru`) that no compiler-side work can.

### 3.5 The instruments must change before the curves mean anything

The strongest assets in the tree are the ones that found every miscompile:
the byte-identical differential, the variation lane, the region oracle, the
interp-entry census. The weakest is the corpus they run over, which is
hand-written and — §2.1 — under-samples exactly the operand kinds the
mission is about. The design's §8.3 already says the corpus "shifts from
hand-picked to generated"; nothing has been built toward it.

**What to build (S0 in §5):** a generator over the word inventory × the
operand kinds × the call forms — for every declaration-relevant signature,
a program in each of: literal body, fn value from a `def`, fn value from a
factory, fn value read from a container, fn value from a module export,
computed body, inside a fn body, inside a module, forward / stack / mixed
spelling, paren-bounded — run through the same differential and census
lanes, with the expected answer the interpreter's. Tens of thousands of
programs; a first run that reports hundreds of defects is the honest
number, and the ratchets then run over *that*. A coverage-matrix gate
(every relevant signature × every operand kind has a row) makes "no
exceptions" checkable, which today it is not. Six to ten session-days,
and the only way to know when the project is done.

---

## 4. The algorithm reviewed — the generic lane as designed and as built

### 4.1 What the design asks

Per G-lane region: a static `RegionDesc` (lead, slots in written order to
the next hard delimiter, per-candidate barrier geometry, seal facts) and
two opcodes. `OpCollect` runs the *extracted* collection routine over the
descriptor — the same three loops the interpreter runs (the plan walk, the
per-candidate scan, the arrival decision), evaluating group slots by
calling compiled fragments only where a viable overload consumes the
position, applying `/q` in place, honouring the strict barrier — producing
a claimed window plus parked signature or the interpreter's own raise,
with the raise-selection state carried per invocation in a `RegionState`.
`OpDispatchGeneric` completes it: the word-policy gate, live
`Registry.Lookup` + `aggregateDispatch`, kernel `MatchSignature` over the
claimed window (variable width), then handler call, unit entry, curry, or
the exact no-match raise. Totality is by token class: every token has a
descriptor form.

### 4.2 What exists

- **Descriptors for every dispatch family** (Phase B claims the user,
  poly-user, native and poly-native records; increments 60–61), riding the
  event and appended by the lowerer, validated over the corpus; the
  written-order rule checked rather than assumed (99.83% measured, the
  lowerer verifying the prefix); `NFwd` bounding the claim (22,958 of
  65,959 span slots claimed).
- **The shared kernel, seated on both sides:** `CollectForward`,
  `CollectCandidateScan`, `CollectArrival` extracted with the Engine as a
  client (Stage 2, textually identical modulo the host); `PlanMatch` seated
  on the seam in increment 64 so both hosts read one matcher.
- **The oracle**, which walks every descriptor live on the corpus and
  compares with the record: 94.5% reproduced, 1.9% under-claimed (the safe
  direction), 3.6% declined (a group or interpolation the host cannot
  evaluate), one over-claim and five value divergences, both ledgered with
  mechanisms. This is the single best piece of evidence that the
  descriptor + live-kernel model *reproduces the interpreter's claim*,
  and it is the kind of evidence the earlier programmes never had.
- **The routed op** for drivable spans: the window laid out as the tape at
  the word; the plan walk and the matcher over it; the committed unit
  entered or the record's native called; a no-match, a strand and an
  unbound slot raised byte-identically from the window (increment 66);
  every binding of a routed name inside a frame lowering a dyn-scope twin
  so the live read sees what the interpreter sees.
- **The bind twins** (rollback-and-replay, the only regime), the
  speculative undef and speculative fn families placed and routed
  (increments 67–72), a stored handler reading its deps live (71).

### 4.3 The three unbuilt cores, sized

1. **The evaluating host.** The VM's `CollectHost` declines every
   evaluation. The corpus's regions are 45.6% word, 24.2% const, 15.3%
   *active* (a compound literal whose members run on arrival — a fragment
   in disguise) and 14.1% *group* (a paren whose fragment runs only if a
   viable overload consumes its position); 21.2% of regions carry a group.
   Driving those means re-entrant fragment execution *from inside* the
   collection walk — a compiled sub-fragment called at the moment the
   interpreter would call `evalParenGroupAt`, with the window spliced by
   its result, the conditional-evaluation order preserved (a group is not
   run when no viable overload wants it), effects ordered as the
   interpreter orders them, and the fragment's own dispatches possibly
   G-lane again. The VM already runs units nested and already has the
   mark/region machinery for width; what it lacks is the host method and
   the fragment table keyed from the slot. This is the largest remaining
   piece and the one with the most parity risk, and it needs its own
   falsifier: a generated sweep of `w (g …) …` shapes over every overload
   set in the registry, with an effect inside the group, differential
   gated. Estimate 10–15 session-days; the design's F1/F2 discipline
   (build, measure, correct the note) applies.
2. **The lane's runtime totality.** "The claim is the record's": the live
   plan must claim exactly the record's forward slots and arity, or the op
   defers, because the code after it was laid out for the record's stack
   effect. That is correct as a *constraint* and fatal as a *design* — the
   case the lane exists for is a rebinding that changes the claim, and
   today that case defers. The answer is the design's own §6.6: a G-lane
   statement's downstream is count-generic (a mark region), so a different
   live claim is representable. Stage 5 is therefore not after Stage 4; it
   is inside it. Together with the arm-by-arm table in §3.2, 10–15
   session-days.
3. **Leads that are not a stable word:** a fn value as lead
   (`LeadFnValue`), a callee with captures (they ride as trailing operands
   the routed op has no plumbing for), a poly native record (commits to no
   one implementation), a full-stack native, a foreign unit. Each is an
   arm of the op and each has a named answer; the fn-value lead is where
   §3.3's convention and the lane meet, and it should be built once, on
   the Apply kernel, and called from both. 5–10 session-days.

### 4.4 Is the approach right?

Yes, with two corrections and one measurement owed.

- **Build it as a baseline, not an exception path.** The design's own
  argument (§3.3, Deutsch–Schiffman; Factor's non-optimising compiler) is
  that totality is *structural* when every token has a template. The
  increments have instead widened the typed lane one shape at a time —
  which is exactly the activity that produced the miscompile rate the
  assessment measured (about one per five increments) and the five this
  branch found: every one of them is a bespoke typed lowering (a static
  clause bake, a callback convention that skips the match, a module-export
  const that never applies), and none is in shared-kernel code. Rule for
  the next fifty increments: a new shape lands on the G-lane first
  (correct by sharing), and takes a typed lowering later, by proof, under
  the differential. That inverts the current habit and is the single
  cheapest way to lower the miscompile rate.
- **The split-identity invariant is a typed-lane property** (§3.1). Stated
  as such, it stops being the reason the terminal arm cannot flip.
- **Performance is unmeasured for routed dispatches.** F4 ("the G-lane
  slower than the interpreter") was to be benchmarked at Stage 4, into the
  register; 676 corpus dispatches route today and no register row exists.
  Measure before the evaluating host lands, because that host is where the
  cost will be.

Two further points the review confirms rather than changes. *Parity by
construction* holds where the sharing is real: the dispatch-agreement
census (planner vs kernel matcher over every committed corpus dispatch, a
two-way ledger) is the right gate, and it is the gate a re-implementation
would have made impossible. And the *Apply kernel* (§6.4) is the correct
seam for fn values: NUR101's place-vs-call rule is recorded at the
collapse, the arity-consuming apply is the interpreter's, and the
remaining application defects (NUR154–156) are in what reaches Apply,
not in Apply.

---

## 5. The revised staging

Written into FULL-COMPILATION.0.md §10 as the 2026-09-17 amendment. The
old stage numbers are kept where the content is the same; what changes is
the order, the coupling, and the gates. S2 runs beside the rest.

| step | content | gate that makes it measurable | session-days |
|---|---|---|---:|
| **S0** | the generated sweep (word inventory × operand kinds × call forms) through the differential and census lanes; the coverage-matrix gate; every ratchet re-based on the sweep | the matrix has no empty cell; the sweep's defect list is the ledger every later step retires from | 6–10 |
| **S1** | fn values as one convention (Stage 3 closed): unit-or-stamp-at-home at the value, memoised by body key and dep generation (the Stage 7 unit-cache slice comes here); the Apply kernel's first branch; the callback seams reduced to one question; NUR153 ruled first | islands 12 → 0; fn-value refusals 59 → 0; the 23 fn-value census rows → 0; NUR154–156 closed by mechanism; `vm:island` / `vm:island-resolved` seams deleted | 15–25 |
| **S2** (parallel) | handler migration as a sweep (Stage 6): the 114 declared or rewritten; `do` and code-body words on units; `Test.*` quotation bodies compiled (needs S3's runtime compile of a computed body) | `undeclaredHandlerCeiling` 114 → 0 falling per PR; code-body refusals 23 → 0; `RunResolved` and the `Test.*` `CallBoru` census rows → 0 | 25–40 |
| **S3** | runtime compilation (Stage 7): computed bodies, splices and code parts, `canon` / `Vm.run` round trips, module bodies at import (the ruling's "computed code is code" answers O4: compile); O5 answered | the 6 round-trip census rows → 0; F5's induction fuzzed | 10–20 |
| **S4** | the generic lane completed (§4.3): the evaluating host with its own falsifier; the lane's runtime total (every `vm:generic-*` arm retired to a raise or a compiled path, with Stage 5's count-generic downstream built as part of it); the non-word leads on Apply; the routed-dispatch perf row in the register; then the terminal-arm flip per family, by the disposition census's stages | oracle under-claim and declined classes → 0; the dispatch-agreement ledger empty; `vm:generic-*` arms 10 → 0; `MarkUncompilable` sites 92 → the trap and delete rows only | 25–40 |
| **S5** | Stage 5's remaining generality (inert values on both sides of a run, arbitrary and non-adjacent consumers, the split-rule window) | the provenance refusals 15 → 0; NUR129's callable region closed | 10–15 |
| **S6** | Stage 8's T1 half: the "check diagnostics" sentinel deleted, definite errors trapped, the armed-only rows | `armedOnlyCeiling` 16 → 0; the correct-error row → 0; `SuppressedRuntimeError` latch gone | 5–10 |
| **S7** | Stage 9: the valves — every `vmDefer` site, `OpFallback` and its lowerer, `islandRun`, the drift window, the fence's re-run half, `compile_failed`, the hatch | engine-entry census 0; `deferCeiling` and the mechanism deleted; `CompileCheck` total | 8–15 |
| | **total** | | **~105–175** |

Dependencies: S0 first, because every later gate is defined over it. S1
before S4's non-word leads and before S2's `Test.*` half. S3's unit cache
inside S1; the rest of S3 before S2 finishes and before S4's foreign-unit
arm. S4 before S7. S2 runs beside S1/S3/S4 in another session. S5 and S6
slot in when their families are next on the sweep's defect list.

Calendar, at the observed cadence: S0–S3 (corpus-native on the expanded
corpus, and most of the runtime residue) in two to three months; the
whole in four to six, ending in the first quarter of 2027 at sustained
effort with two sessions in parallel for the middle stretch.

---

## 6. Rulings the plan needs, with a recommendation each

| ruling | why it blocks | recommendation |
|---|---|---|
| **NUR153** — which of a stored `=>` value's two interpreter regimes (deferred residual on the tape; evaluated in the live frame through a native seam) is the specification | S1's stamp can match one; the by-name admission today encodes neither | the tape rule (a lambda's single container residual defers) everywhere; the `service` seam evaluates through the same rule; `TestServiceAddStampsComputedMapHandler`'s pinned answer changes — **ruled so by the maintainer on 2026-09-18** |
| **O2** — the step budget | the last documented one-directional divergence; totality makes per-instruction metering the only metering | re-meter the VM to the interpreter's count on the corpus and gate the divergence at 0; a ceiling is a semantics |
| **O4** — module bodies at import | the ruling answers it ("computed code is code") but the design still says "leans compile" | record it as decided: compile, S3 |
| **O5** — check-time budget exhaustion | no emit-anyway story; S3's induction needs one | widen to a dynamic carrier at the exhausted region's frontier, so the region lowers G-lane; never a refusal |
| **the attributed set** — may `boru:debug`'s stepper, the profiler, `RunTrace` interpret inside a compiled program | T2's end state is "every entry attributed", and only these three have a specification that *is* the engine's behaviour | yes for exactly those, named in the census's attributed list, and nothing else — the busy-registry route and the detached-ref decline are defects, not members |
| **NUR110** — the third binding state (a branch-arm `def` leaks, shadows and exists iff the arm ran) | two ledger rows and any general answer to conditional rebinding; three attempts recorded | the speculative-family mechanism of increment 70 is that third state for fns; extend it to value defs rather than open a fourth attempt |
| **NUR078** — the `/r` respelling | O1's remaining half | respell to `/v` as ADR-011 collapsed it and implement as amended |

---

## 7. What this note does not change

The architecture (one kernel, two lanes, no interpreter landing pad at the
end state); the interpreter as T3's oracle and as the check-mode front end;
the ratchet discipline (monotone, in-tree, named when moved); the process
rules the log earned; the negative-result rate as a feature. The design's
falsifiers stay live, and F4 is the one this note asks to be run next.

---

## 8. Method

Gate values are from the `test/go/langspec` run on `658fc85` recorded in
PR #471 (the ten classified reds, unchanged by the audit commit), with the
row listings from `BORU_LOG_CENSUS_ROWS=1` runs of `TestCompiledCoverage`
and `TestInterpEntryCensus` on the same head. Source counts are `grep`
over the tree: `vmDefer(` calls and `vm:*` seam names in `eng/go`, the
disposition table in `refusal_disposition_census_test.go`, the ledger map
in `frontier_spec_test.go`. The language surface is `boru describe` (251
core words; 11 loadable modules with 264 exports) and the declaration
census's 522/172/114. Effort figures are `git log 4945889..origin/main`
and this branch's log. The estimates in §2.3 and §5 and the probabilities
are the author's judgement from those inputs and are labelled as such;
nothing in §2.3's second table or §5's last column is a measurement.

## 9. What landed the same day for velocity (2026-09-17)

The maintainer asked what would raise development velocity and then to
implement every suggestion; this records what each became, so the next
session does not re-derive them. Measured costs before: the langspec
package 22–29 minutes, the pre-commit cycle about an hour, CI 28–31
minutes as one sequential job, the direction gates red on every push with
the log parsed by hand to prove nothing new broke.

| item | what landed |
|---|---|
| one spec file as a five-second experiment | `BORU_SPEC_FILES=<names or globs>`: every corpus walk in `test/go/langspec` (`specEntries`, one seam) and the interpreter oracle (`specfix.RunDir`) visit only the named files; absolute counts are reported, not asserted (since P0 of [FULL-COMPILATION-REPLAN.0.md](FULL-COMPILATION-REPLAN.0.md), 2026-09-18, the per-file compile-failure ledger `compile_failures.tsv` asserts under the filter too). The ten gates over `callbacks.tsv`: 6.5 s; the oracle: 2 s |
| regression lane vs direction lane | `lanes_test.go`: every ratchet has an END STATE (the design's number) and a REGRESSION ceiling (the last merged value, falls only); the default lane asserts the ceiling and blocks, `BORU_DIRECTION_GATES=1` asserts the end state and is red by design. `knownDivergences` keys the five miscompiles to their NUR numbers, pinned both ways; the ten tests re-based; CI's `direction-gates` job renders the table into every PR's summary |
| shard and reorder CI | five parallel jobs (checks, module suites, four langspec shards from `shards.tsv` with `TestLangspecShardsPartition` keeping it complete, the post-test gates, the direction lane) over one composite setup action |
| incremental coverage gate | `cover-profile` profiles every module before failing and caches a module's profile on `scripts/cover-key.sh`'s digest of its dependency closure; `cover-gate` runs the check even after a red module |
| one `make ci-local` that is `ci.yml` | `scripts/ci-steps.sh` is the single definition; the workflow calls it per step and `make ci-local` runs the sequence |
| shrink the handover surface | `SESSION-HANDOVER.0.md` trimmed to 199 lines as the entry point (its running detail moved verbatim into the log); `make gate-status` prints every gate against both numbers and refreshes `GATE_STATUS.md`; `make status-static` the instant censuses |
| work by mechanism | recorded as process rules 9–11 on the handover page and in §10.1's two standing rules |
| the second parallel line | [HANDLER-MIGRATION-LINE.0.md](HANDLER-MIGRATION-LINE.0.md), a session brief with `make handler-worklist` (the 114 signatures, one per line) and the per-word procedure |
| the flakes | the kg `check` target: not a flake — `boru check` anchors relative imports on the file where `boru` anchors on the cwd, so every `tests/*_test.boru` lost its `./util.boru` import silently (28 phantom errors per file); `boru check --base DIR` now anchors where run does and the target passes. `TestModelWatchForkNoRace`: 40 of 40 under concurrent load, not reproduced, left as is. The registry race at the spawn seam: every spawn seam already forks and NUR152's `FnHome` keeps a parent-minted callback on the fork, which is the mechanism both recorded witnesses had; `TestTimeoutBodyAppliesParentFnOnItsFork` pins it under the race detector |

### 9.1 The three-minute contract (2026-09-17, later the same day)

The maintainer then set a ceiling: **three minutes at most for standard
CI, and three minutes for the commit gate.** Measured before: the 12.5
minute run above, whose shape was four langspec shards of six minutes
(each test walking the corpus on one core while three idled), a
test-modules job of six minutes (`cmd/go/internal/vault` alone 231 s, all
of it scrypt at the production work factor), a checks job of 3.5 minutes
(golangci-lint 127 s in sequence), the gates job of six (the borudebug
corpus differentials 193 s), and the direction job of eleven, which
re-ran ten corpus walks to print a table the regression shards already
had the numbers for. What landed:

| where | what | mechanism |
|---|---|---|
| every corpus walk | one shared parallel walk | `test/go/langspec/walk_test.go`: `specWalk`/`specWalkFiles` parse the selected spec files once and hand them to one worker per CPU, largest file first; a body gets a `testing.TB` whose `Fatal`/`Skip` abort the row, not the test, and a panic names its row. Sixteen walks converted; every gate value unchanged; race-clean under the detector on a corpus subset; `BORU_SPEC_WORKERS=1` is the sequential form |
| every langspec test | `t.Parallel()` | the one exception is `TestRegionCollectOracle`, which sets the package-global `compiler.RegionOracle` and runs alone, before the parallel batch |
| `cmd/go/internal/vault` | the scrypt work factor is a variable the package's tests lower | 293 s → 14 s; the format pins 2^15 and nothing outside a test file may assign it |
| vet, lint | every module in parallel | `scripts/each-module.sh`, output captured per module; golangci-lint's cache in the per-job build cache, so a warm run is seconds |
| CI | small jobs, each under the ceiling, setup per job | `.github/actions/setup` takes inputs (`node`, `tools`, `build-cli`, `cache-name`) so a job installs only what it runs; the module cache is one shared entry keyed by the dependency set, the build cache one entry per job; test-modules split into test-core, test-lang (root and rest), test-cmd; the gates split by what they need installed; borudebug its own job; five langspec shards |
| the direction lane | no job — a table from the shards | every shard appends its gates' rows (`BORU_GATE_SUMMARY`) and uploads them; the `gate-table` job renders the one table into the run's summary. `make test-direction` is the lane as a local run; `make gate-status` refreshes the committed table, now sorted so it does not churn |
| the kernel | the unify registry threaded, a package-global stack gone | the parallel walks were the first to race on `core.unifyRegistryStack`, the slice every `UnifyExplainR` pushed and every goroutine's registry-less `Unify` read (class construction, conditionals, the `unify` word — hot paths, so any concurrent program raced too). The registry is now a parameter through the whole unify recursion and the `Unifier` interface; pinned under the detector by `TestUnifyRegistryArmedConcurrentNoRace` (403 reports before, none after) and in CI's race gates. One deliberate change fell out and is pinned: a predicate body's dispatch is unarmed like top level (it used to be armed by whatever unify was in flight on ANY goroutine); one pre-existing oddity the threading kept verbatim is NUR157 |
| the commit gate | `make commit-gate` | `scripts/commit-gate.sh`: gofmt, vet and lint on the touched modules in parallel; the touched modules' unit tests (the changed packages of `lang/go` and `cmd/go`); the langspec gates over a smoke corpus (eleven files, one per family that has bitten) plus every spec file the change touched, under `BORU_SPEC_FILES`; the knowledge graph when docs or tooling changed. Each lane prints its time; a breach of the ceiling is a warning whose fix is in the script or the tests, never in a skipped lane |

The first run under the new layout (`a9cb212`) was 4 min 33 s with
every cache key new — every job cold — and eighteen green jobs; the
long pole was the checks job compiling golangci-lint from source and
linting on a cold cache (268 s), with one shard and the borudebug job
at the ceiling. With the lint release binary downloaded instead, the
race gates as their own job and nine shards balanced on an idle
measurement, the second run (`a4f0734`, caches warm) was **2 min 24 s**
with twenty green jobs, the longest a shard at 130 s. The log
(FULL-COMPILATION-HANDOFF.0.md) carries both per-job tables.

One checker defect the kg investigation exposed and this note only
records: a relative import that resolves to nothing is silent in check
mode — the namespace's words come back `undefined_word` one by one instead
of one `import` error at the line that failed. It belongs with the
checker-precision programme (§3.5, Tier C).
