# ENG-COVERAGE-PARITY.0 — the standalone 100%/100% program for the kernel twins

**Status:** In progress · **Started:** 2026-08-04 (maintainer
instruction: "Both the go and ts coverage of eng needs to be 100% and
it needs to be standalone without relying on tests of other modules.
Go and ts need total parity.")

## The contract

1. **Standalone.** eng proves itself with its OWN suite, on both
   implementations. The repo-wide ADR-008 gate keeps measuring the
   merged cross-suite profile (lang's suites legitimately cover eng
   statements there); the standalone gates measure eng/go by eng/go's
   tests alone and eng/ts by its own `node --test` suite.
2. **Parity of metric.** Go's gate unit is the covered STATEMENT
   (covergate over a `-coverpkg`-instrumented profile, pragma
   allowlist for provably-unreachable guards); TS's gate unit is the
   covered LINE (node:test's built-in coverage, `node:coverage`
   ignore comments reserved for the same provably-unreachable class).
   Go statements ≡ TS lines is the parity equivalence.
3. **Target 100% on both; floors are ratchets.** Until the target is
   reached, each gate enforces a floor that ONLY RISES:
   - Go: `make cover-gate-eng`, floor `ENG_GATE_FLOOR` (root
     Makefile). Current: **84** (measured 84.6%, 9,776/11,552
     statements — RE-BASED at the four-piece Stage 4 cut, which
     moved the interpreter core's statements and their kernel test
     files to `core/go`; the pair with `make cover-gate-core`
     supersedes the pre-cut single-gate floor of 89 —
     design/legacy/ENG-FOUR-PIECE.0.ignore). The core twin is DONE: `make
     cover-gate-core` holds **100** (13,978/13,978 statements,
     104 proof-carrying allowlist exclusions).
   - TS: `make test-ts`, floor `TS_GATE_LINES` (root Makefile).
     Current: **96** (measured 96.80% SOURCE lines; branches 91.9%,
     functions 94.8% recorded but not yet gated).
     The denominator is source only — `--test-coverage-exclude=
     '**/*.test.ts'`. node:test instruments every file it loads,
     including the test files themselves, so without the exclusion
     the gate counted its own test code as covered source. Go's
     `-coverpkg` metric never counts `_test.go` statements, so the
     equivalence in (2) below only holds with the exclusion; the
     floor RE-BASED 97 → 96 when it landed, for the same reason
     `ENG_GATE_FLOOR` re-based at the four-piece cut — the
     measurement universe changed, so the old number is not
     comparable (same commit: 97.06% counting test files, 96.80%
     source only).
   - TS core: `make test-ts-core`, floor `TS_CORE_GATE_LINES` (root
     Makefile). Current: **62** (measured 62.06% source lines over the
     whole `core/ts` package). This is the twin of `cover-gate-core`
     and completes the set: each of the four pieces now has a
     standalone gate on both implementations.
   Raise the floor in the same PR that raises coverage; lowering
   either floor is a build break by intent.

   **The parity instrument is now core-level too.** `core/spec/*.tsv`
   is replayed by `core/go/corespec_test.go` and
   `core/ts/src/corespec.test.ts` — two runners sharing no code, each
   implementing the corpus's parser-free notation independently
   (neither core has a parser, which is the point of the core cut).
   Unlike `eng/spec` it is a SPEC, not an agreement set: the `expected`
   column is written from REFERENCE.md rather than from either
   engine's behaviour, so a row can legitimately fail on BOTH engines.
   That is precisely the class of defect `eng/spec` is structurally
   blind to — design/legacy/CORE-GO-TS-DEFECTS.0.ignore documents 22 of them that
   the 1808-row engine differential is green across.
4. **Parity of corpus.** The shared `eng/spec` corpus remains the
   behavioral contract both engines replay row-for-row (the
   cross-engine differential). Corpus growth serves both engines'
   coverage at once and is the preferred instrument wherever a gap is
   expressible as rows; engine-local tests carry the rest.

## What already stands (2026-08-04)

- **The standalone corpus lanes** (`eng/go/corpus_standalone_test.go`):
  interpret, check, and compile-or-fallback over the full `eng/spec`
  corpus on a bare `eng.NewRegistry` + the `specfix` fixture words —
  with a mechanism-only stand-in for the content layer's Micron Ideal
  (Pathon rows construct through the kernel's own `MakePathon`; the
  six Emailon/Urlon validator rows skip via `specfix.ErrSkipRow` and
  stay covered by the full harness in `test/go/engspec`).
- **`eng/go/specfix`** — the TSV runner (formerly
  `test/go/specrunner`) and the fixture-word registrations (formerly
  private to `test/go/engspec`) as one eng-owned package, shared by
  the standalone lanes, the engspec harness, and the cross-engine
  differential. Fixture declarations were upgraded to carry the REAL
  declared models where the compiled lane needs them (the stack words'
  `ReturnsIdentity` wiring — an empty `Returns` compiled splice bodies
  to programs whose operands vanished).
- **A kernel soundness fix the lanes exposed**: the interpolation bake
  probe (`evalInterpParts`) tested only the hole's own Carrier flag, so
  a hole evaluating to a CONTAINER holding a carrier (`${[1 addq 2]}`
  with a non-folding word) baked the check-time type-tag render as a
  constant string. The probe is now recursive
  (`valueCarriesCarrier`); with lang's folding words the old bake was
  coincidentally sound, which is why it survived until a
  non-folding fixture ran under the compiled lane.
- **The compiled lane honours the VM's soundness-bailout contract**:
  an `internal_error` from `RunProgram` falls back to a fresh
  interpreter run, mirroring lang's compiled-by-default entry.
- **specfix meets ADR-008 as production code** (2026-08-05): the
  fixture words' guard arms the corpus never reaches are pinned by
  `eng/go/specfix_probe_test.go` (class types, dep scalars, table
  types, and a signature-less fn constructed through the kernel's
  exported API and bound via the def table), the runner's rendered-skip
  arm and the map walk's error arms by specfix's own unit tests, and
  the provably-unreachable remainder (raw NoEval list slots are always
  concrete; sole-caller pre-checks; runtime-only handlers never see
  non-concrete containers) carries proof-carrying `//covergate:allow`
  pragmas. The probes also flushed out and removed a broken typed-nil
  guard in the fixture map walk. Merged repo gate back at 100%.

## The remaining gap — Go (3,061 statements, per-file)

Concentrated in the compile pipeline and the paths only rich programs
drive: `emit.go` (~557), `carrier.go` (~355), `engine.go` (~300),
`lower.go` (~284), `core_helpers.go` (~190), `callable_words.go`
(~154), `vm.go` (~114), then a long tail (`fn_params`, `registry`,
`method_shape`, `unify_*`, `weak_flex`, `trace`, `engine_pool`,
`module_gate`, `word_extend`, `generics_*`, …).

Staged plan:

1. **Control-flow fixtures.** `specfix` has no `if` / `case` / `for`:
   the branch/loop recording machinery (`RecordBranch`, the
   loop-carried analysis, the join machinery) is unreachable
   standalone. Add minimal fixture control words that orchestrate the
   SAME eng mechanisms basic's handlers do (the mechanism calls are
   eng-exported; the fixtures are registrations, exactly the
   engspec pattern).
2. **Compiled program batteries.** Eng-local differential batteries
   (crafted fn/branch/loop/closure programs over the fixture
   vocabulary, compiled vs interpreted) targeting `emit`/`lower`/`vm`/
   `carrier` — the standalone twin of lang's bytecode clusters.
3. **Corpus growth (parity instrument).** Gaps expressible as rows go
   into `eng/spec` so BOTH engines earn them; every added row runs
   through the cross-engine differential by construction.
4. **Seam tails.** The remaining arms currently reached only by lang's
   seam suites get eng-local seam tests (same design/TEST-SEAMS.10.md
   conventions), or a pragma where the guard is provably unreachable.

## The remaining gap — TS (~14% of lines, per-file)

The engine core is strong (compile/error/signature/resolve/lower/
capability at 100%; match 99%, emit 98%, vm 95%, engine 94.5%). The
gap concentrates in `parser/convert.ts` (72.7%), `parser/grammar.ts`
(77.9%), `parser/errors.ts` (48.0%), `spec-fixture.ts` (72.9%),
`canon.ts` (71.8%), `value.ts` (89.5%), `parser/xml.ts` (83.5%).

Staged plan: a parser-focused unit campaign first (convert/grammar/
errors are the divergence-prone surface the cross-engine differential
can only catch when a row happens to trip it), then canon/value, then
the fixture harness; corpus growth from the Go side lifts here too.
Gate the branches/functions metrics once lines reach 100.

### Fixture-parity backlog (2026-08-05)

The TS fixture guard probes (`src/fixture-probe.test.ts` — the twin of
`eng/go/specfix_probe_test.go`) surfaced fixture behaviors that
diverged from the Go reference on paths no corpus row reaches.
ALIGNED (2026-08-05, probe rows moved into the shared table):

- `get`: the atom-key sigs now quote their key; every miss returns the
  None TYPE literal (was the `none` value on list misses and a
  dispatch failure on map keys).
- `do`: a raising list body surfaces as an Error VALUE carrying the
  bare detail — TS gained the error-value kind (`ErrorInfo` /
  `newErrorValue` in value.ts, the `error(<message>)` canon arm) the
  hatch needs.
- `refine Record`: the at-least-one-field and per-element pair guards
  now match the Go messages.

- the 1-arg bare `refine`: the base type passes through unchanged (the
  paired `def` mints the subtype); a non-type argument raises the Go
  message.

The probe-surfaced backlog is CLOSED (the object-with-parent
constructor path remains a fixture gap on both engines' corpora, not
a divergence). Where expressible, corpus rows should follow so the
differential guards the aligned behaviors permanently.

## Ratchet log

| Date | Go floor | TS floor | Note |
|---|---|---|---|
| 2026-08-04 | 86 | 85 | Gates introduced; standalone lanes + specfix landed. |
| 2026-08-05 | 87 | 85 | specfix guard probes + pragmas (fixtures at 100%); merged repo gate restored to 100%. |
| 2026-08-05 | 88 | 85 | Stage-4 wave 1: unit probes for the pure-function tail (SizeOf, table/JSON render, Disassemble, the unify families, fn-spec params/returns, RunPredicate, CallBoru, registry arms) — 316 statements. |
| 2026-08-05 | 89 | 85 | Stage-4 waves 2–4: fixture if/for (count + range) + doq closures (specfix/control.go), the eng-local battery (value, compiled, and check lanes; 78 rows, most VM-executed) — 542 statements total closed. |
| 2026-08-05 | 89 | 88 | TS wave 1: the parse-error translation campaign (errors.ts 48→100), the canon tails (71.8→98.5), and the parser breadth battery (convert.ts 72.7→82.2) — 85.68→88.53 lines. |
| 2026-08-05 | 89 | 90 | TS wave 2: the legacy hand-rolled tokenizer (dead since the jsonic parser became the single path; its value contract predates ADR-012 opacity) removed after an oracle experiment proved it diverges by design; fixture guard probes landed and the fixture-parity backlog recorded — 88.53→90.90 lines. |
| 2026-08-05 | 89 | 91 | TS wave 3: three fixture-parity alignments (get quoting + None-literal misses, do's error-value hatch with the new ErrorInfo value kind, refine Record guards) and 20 parse-battery rows (generics angle sugar, underscore/0d numerics, escape tails) — 90.90→91.38 lines. |
| 2026-08-05 | 89 | 92 | TS wave 4: the 1-arg bare refine port closes the fixture-parity backlog; 19 battery rows for map-shorthand modifier keys and the XML family (entities, error taxonomy, interpolation holes, comments) — 91.38→92.37 lines. |
| 2026-08-05 | 89 | 93 | TS waves 5–6: value.ts predicate-table probes; battery rows for arrow folds, angle composition, interpolation segments, unterminated containers, and the map-value dot-chain folds — 92.37→93.09 lines. |
| 2026-08-05 | 89 | 94 | TS wave 15: coretype.ts to 100% — the boolean-coercion table over every value mode, and `is` realigned to Go's terminal Unify (unifiesValue mirror: literal-vs-concrete admission `Integer is 5`, symmetric list templates, EXACT nested-map unify vs the open top-level pattern, enum alternatives via unifyDisjunct) with 40+ rows all pinned against the Go engine — 93.96→94.14 lines. |
| 2026-08-05 | 89 | 94 | Merged ADR-008 gate restored to 100% (32 statements of battery-wave debt): the control-edge battery (empty-value branches, computed-list arm refusal, doq def/param bodies, disjunct loop captures via the `dd` probe binding, the range-parse taxonomy), the declgrammar all-ops test, doq's unreachable word-deref arm deleted, and 7 proof-carrying pragmas. TS waves 16–17: sugar/type/declgrammar/vm/bytecode/canon/match/registry/value all to 100% lines — 94.14→94.86. |
| 2026-08-05 | 89 | 94 | Binding-inertness parity closed, pinned by 9 new `lists-inert.tsv` corpus rows: TS's plain `def` sig dropped its extra NoEval (the value slot evaluates like Go's plainDef — `def b [1 addq 2]` binds `[3]`), TS fn param binding quotes list args (quoteListArg — core_helpers.go's rule), and the corpus rows exposed a GO-internal hole the same day: the VM's CALL_USER/CALL_USER_POLY delivery loops did not quote list params, so compiled renders dropped the `(quote …)` wrapper the interpreter produces. Fixed in vm.go with the poly arm pinned by TestCompiledUserPolyListParamQuoted (canon assertions — the display render hides the flag). TS 94.86→94.96 lines. |
| 2026-08-05 | 89 | 95 | TS wave 19: engine.ts forward/marker/data-eval arms — container base values `(quote [])`/`{}`/`/q`, parked-marker fire shapes (`5 addq 3 ;`, `addq 1 2`), interp forward args, multi-result paren map values (`{a:(1 2)}` → `{a:[1 2]}`), xml stringify of non-xml elements, orphan-move/sugar-token/recursion-induction probes — all engine rows pinned against Go — 94.96→95.10 lines. |
| 2026-08-05 | 89 | 95 | TS wave 20: the fixture's legacy word-modifier chain deleted (atomicValue / parseModifierSuffix / newWordWithModifiers plus the colon-split and constrained-word def paths — every colon-typed name arrives as a MAP token, so the string paths were unreachable), and a parity fix the recon surfaced: the map-form typed def now resolves its VALUE like Go's dispatch (`def x:null null` binds the atom on both engines). spec-fixture.ts 88.25→95.12 — 95.10→95.92 total lines. |
| 2026-08-05 | 89 | 96 | TS wave 21: the xml edge taxonomy — duplicate/unterminated/empty attributes, spaced and malformed closing tags, nested-child error propagation, multi-line cursor tracking, and interpolation-hole scanning (quoted strings, nested braces, unparseable inner programs), 12 rows byte-identical on both engines. xml.ts 89.07→98.41 — 95.92→96.21 total lines. |
| 2026-08-05 | 89 | 96 | TS wave 22: interp-literal escape decoding (the full escape table incl. the unknown-escape passthrough), base-prefixed integers (0x/0o/0b, Go's post-prefix underscore allowance `0x_1`, the misplaced/invalid taxonomy, int64 overflow), data-context specials (`[inf -inf nan]`), empty list slots, and xml-in-data-context — 16 rows byte-identical on Go. convert.ts 88.36→91.47 — 96.21→96.63 total lines. |
| 2026-08-05 | 89 | 96 | TS wave 23: empty/comment-only sources, dangling reach dots, list-pair maps, the mini-literal sugar, and the nesting-depth pair — the deep-nesting probe surfaced that TS's prescan refuses at its documented TS-safe 500 bound (the tabnas rule engine overflows the JS stack near ~900) where Go's live guard fires at 10,000 with the same taxonomy; the TS converter's 10,000 backstop is provably unreachable behind the prescan and carries the sanctioned node:coverage ignore. convert.ts 91.47→92.13 — 96.63→96.72 total lines. |
| 2026-08-05 | 89 | 96 | TS wave 24: a real grammar divergence closed — TS's pelem rule silently DROPPED comma holes inside parens (`(1,,2)` parsed as `paren([1 2])`) where Go raises the empty-element error; pelem now records null holes, interior/leading holes raise Go's taxonomy, and a trailing hole derails the paren close with Go's exact unmatched-paren text. 15 more Go-pinned rows (optional map keys driving the disjunct simplifier, unclosed angle, leading-dot numerics, 0d taxonomy, negative/list-context base-prefixed ints). convert.ts 92.13→93.47 — 96.72→96.92 total lines. |
| 2026-08-05 | 89 | 96 | TS wave 25: the Reach canon arm ported (canonReach — dotted surface with plain/getr segments, the `$` lens, quoted/computed keys, paren receivers), closing a render gap where TS canon printed `[object Object]` for every reach value; six Go-pinned chain rows. canon.ts 91.56→99.68 — total lines steady at 96.92 (the new arm's lines offset the rows' gains). |
| 2026-08-05 | 89 | 97 | TS wave 26: arrow folds in list/reach contexts (incl. the reach-interior fold `a.b => [1]` and the map-value refusal), computed/quoted/numeric map keys, and the optional-key sibling leak pinned as a SHARED quirk (both engines wrap the sibling identically — tabnas's K map rides across pairs). grammar.ts 92.15→93.33 — 96.92→97.06 total lines. |
| 2026-08-05 | 89 | 97 | Go wave: `eachq` — the battery's lambda-hook Callable (quotation AND Function-value forms over the InvokeBody seam, with the element-carrier model in its ReturnsFn), the standalone driver for tryRecordLambdaClosure / lambdaHookCompatible / lambdaCallbackInputs; 8 battery rows all VM-executed. eng/go standalone 90.1→90.3 (callable_words 65→69). |
| 2026-08-05 | 84 (re-base) + core 80 | 97 | Four-piece Stage 4 cut: the interpreter core and ~120 kernel test files moved to core/go. The Go gate re-bases over eng/go alone (measured 84.6%) and gains a twin, `cover-gate-core` (floor 80, measured 80.5%); the pair covers the same statements the old single gate did, both ratcheting to 100 (design/legacy/ENG-FOUR-PIECE.0.ignore). |
| 2026-08-06 | 84 + core 100 | 97 | Stage-5 core campaign: three test waves (eleven parallel agent batteries + a final sweep) close all 2,833 post-cut uncovered statements in core/go's own suite — the process registry, trace layout, value render/classify families, the store clone graph, compile-pass arming, ParseFnParams/ResolveSigType, the dispatch-trap and step-loop arms via stub recorders and slot stubs, the unify/canon tail, and 181 scattered edge arms. Four pragmas whose guards became covered were stripped; zero new pragmas. cover-gate-core: 80.5 → 100.0 (13,978/13,978), CORE_GATE_FLOOR ratchets to 100. |
| 2026-08-06 | 84 + core 100 | 96 (re-base) | TS coverage denominator corrected to SOURCE ONLY (`--test-coverage-exclude='**/*.test.ts'`). node:test was counting the 19 `*.test.ts` files as instrumented source, and several of them carry uncovered lines (check.test.ts 82.1%, crossdiff.test.ts 88.5%, fixture-compiled.test.ts 89.8%), so the gate was measuring its own test code — not the Go metric, which never counts `_test.go` statements. Same commit measured both ways: 97.06% with test files (which is what let the 97 floor pass), 96.80% source only. Floor re-bases 97 → 96 over the corrected universe and ratchets from there; no coverage was lost. |
| 2026-08-07 | 84 + core 100 | 96 + ts-core 62 | The TS core cut. `core/ts` (@boru-lang/core) extracted as the twin of core/go, with the same boundary: core owns CheckState/CheckDiagnostic/EmitRecorder/AnalysisImpl, the check piece owns toCarrier/stripToCarriers/carrierResults. Two upward imports inverted through the AnalysisImpl seam core/go already declared; NAMED inactive defaults pinned by core/ts/src/seams.test.ts. Zero upward imports and zero dependencies, enforced structurally (the package declares no dependency on @boru-lang/eng). New gates `make test-ts-core` (floor 62, the honest figure over the full package — the stage-1 71 covered only the seven files that suite loaded) and the core-level parity corpus `core/spec` (35 rows, green on both engines from independent runners). One real divergence fixed en route: the TS engine pattern-matched the compiler's Operand union in three places, which core/go structurally cannot do; core now asks via EmitRecorder.constValueOf. eng/ts 4059/4059, cross-engine 1808/1808, 0 divergences. |
