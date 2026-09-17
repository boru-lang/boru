# voxgig-boru interpret/check/compile completion — current leaf map (post-merge)

Goal: every voxgig-boru repo (bloom-filter, decision, sort, stats, trie, **template**)
fully `boru run` (interpret), `boru check`, AND `boru run --force-compile` clean.

Baseline on the merged binary (HEAD `53719f21`, my 22 commits + main `cc3fcf63`):
- **interpret**: 44/48 OK (4 `*_prop_spec` are slow property GENERATORS that time out >60s — not errors; raise the timeout or treat as pass).
- **check**: 47/48 OK — only `template.boru` (24 false-positive errors; see L8).
- **compile** (`-force-compile`): 14 COMPILED, 34 REFUSE, **0 ERROR**. `--compile` == interpret for all 14 COMPILED (verified); the 34 REFUSE are silently routed to the interpreter by the runtime scaffolding, which hides 34 failures rather than fixing them. **Each refusal is an open defect owed a fix; DONE is 48/48 COMPILED.**

KEY: each refusing file is a CHAIN of distinct leaves; the refusal text names only
the FIRST surfacing one (and "code-body word X (Stage 2)" is a MASK — the body is
probe-compiled in a throwaway EmitState at `callable_words.go:249-255`, so the real
reason is swallowed). A file flips only when ALL its leaves clear. Gate every fix
with `verify-bytecode` (the differential) + a hand-written off-corpus
`RunCompiledStrict`-vs-`Run` regression (the differential is blind to these shapes —
this caught the param-guard/Options miscompiles) + the voxgig `--compile`==interpret
sweep.

## Real leaves (unmasked by a 9-agent workflow), highest leverage first

### L0 (FULLY TRACED w/ pointer identity) — multi-pass dispatch records AND refuses the same each
Pointer-identity trace of `each [def j (5 add 1) j]`:
- es A (`0x…f180`, **suspended=1, active=false**): a result-type probe pass —
  tryRecordClosure declines at `!es.active()` (callable_words.go:84).
- es B (`0x…f040`, **suspended=0, active=true**): the LIVE compile es —
  tryRecordClosure REACHES recordClosureDispatch and **records the closure**.
- **REFUSE on es B** (the SAME live es, suspended=0): the `execBodyRefsNames`
  refusal (emit.go:2020) fires and latches es B uncompilable.
So the live es BOTH records the each (closure) AND refuses it (RecordCall), across
the two analyses of g's body (ReturnsFn install + compile). A suspend-skip gate
does NOT fix it (the refusal is on the un-suspended es B; verified — no-op).
Discriminator: `each [def j 5 j]` compiles, `each [def j (5 add 1) j]` refuses —
`valueRefsName` (emit.go:2575) flags ANY Word, so the computed `(5 add 1)`'s `add`
trips it; but narrowing it to non-builtins fixes only `(5 add 1)`, NOT the real
voxgig `[def j (cur get 0) j]` (cur is a genuine frame-local capture the CLOSURE
path handles via capture, yet the const-bake-guard refusal still fires).
**The true fix is multi-pass dispatch CONSISTENCY**: the RecordCall refusal must not
latch the program when the same word records via the closure path in the recording
pass — e.g. defer code-body-word RecordCall refusals and reconcile at Finalize, or
have RecordCall skip the latch for a Callable word whose body is closure-compilable
(probe). Intricate + soundness-critical (a wrong skip lets a true-fallback frame-
local body const-bake → silent miscompile). Mandatory regressions: a captured-
frame-local each body that the closure records (must compile), and a non-closure-
able name-body (must still refuse).

### L0-PRIOR (superseded) — premature `execBodyRefsNames` refusal
**Exact mechanism** (traced with instrumentation): an each whose body contains a
value-def (`def j (5 add 1)`) — i.e. the body REFERENCES A NAME — dispatches TWICE:
1. once with `es.active()==FALSE` (a non-recording pre-pass / nested closure probe):
   `tryRecordClosure` declines at the `!es.active()` guard (callable_words.go:84),
   so carrier.go falls through to `RecordCall`, which hits the
   `(sig.Callable != nil && execBodyRefsNames(sig, args))` refusal (emit.go:2020) →
   `MarkUncompilable("code-body word each (Stage 2)")` → LATCHES the program;
2. once with `es.active()==TRUE`: `tryRecordClosure` REACHES recordClosureDispatch
   and RECORDS the closure successfully.
The body IS compilable via the closure path (pass 2 proves it); the refusal exists
only to guard the const-bake FALLBACK (re-running a name-referencing body in a
sub-engine is unsound), but it fires in pass 1 even though pass 2 records via
PUSH_CLOSURE (no fallback). **`each [def j 5 j]` (const value-def, no name-ref in
the computed sense) compiles; `each [def j (5 add 1) j]` refuses** — execBodyRefsNames
is the discriminator.

**Fix direction:** the `execBodyRefsNames` refusal must not LATCH the program when
the closure path will/did record the body — i.e. defer or skip it in the
non-recording pre-pass (the proven suspend-skip precedent, commit 6e3c089e, but the
state here is `!active` from a nested-probe es-swap rather than `suspended>0`, so the
gate must key on the right pass identity). The es-swap interaction (recordClosure
Dispatch swaps r.Check.Emit for the throwaway probe) is the subtlety to nail before
implementing — a wrong gate either fails to fix it or lets a genuinely-fallback
name-body const-bake (silent miscompile). Mandatory: a RunCompiledStrict regression
for a name-referencing each body that the closure path records, AND one for a body
that genuinely falls back (must still refuse). This unblocks the 14 each + 8 test-
test files.

### L0-OLDER (incomplete) — value-def of a COMPUTED expression inside an each-closure body
**`var` is NOT the leaf** — its sig declares `RunInCheckMode: true` and it compiles
(the splice records as value-def locals). The real dominant leaf, unmasked by direct
minimization: a **value-def whose value is a computed paren-expression, inside an
each-closure body**. Precise repro:
- `[1] each [def j 5 j]` → COMPILES (value-def of a const)
- `[1] each [def j (i mul 2) j]` → COMPILES (param-computed)
- `[1] each [def j cur (j get 0)]` → COMPILES (value-def of a capture directly)
- `[1] each [def j (5 add 1) j]` → **REFUSES** (value-def of a computed const)
- `[1] each [def j (cur get 0) j]` → **REFUSES** (value-def of a computed capture-get)

The SAME body `def j (5 add 1) j` compiles as a plain FN body (`def g fn
[[][Integer][def j (5 add 1) j]] (g)` → 6) but refuses as an each-closure body — so
it is the each$body promotion path (compileClosureBody, callable_words.go:31), not
value-defs generally. FURTHER NARROWED: the throwaway closure PROBE (callable_words.go:251)
SUCCEEDS; the refusal is in the post-probe REAL compile (:259) or RecordClosure
Call's collection-operand resolve — i.e. a state-difference between the probe and the
real emit. The next pass must instrument the REAL compileClosureBody call (:259) +
RecordClosureCall, not the probe, to find the exact MarkUncompilable site, then fix
the each$body value-def promotion (leaves-doc carrier-ID hazard applies; mandatory
per-frame-local-kind RunCompiledStrict regression). This is the 14-each + 8-test-test
blocker; `var` was a red herring.

### L0-OLD (WRONG) — `var` block hypothesis (kept for the record; var compiles)
**Status: this is what actually blocks the 14 each + 8 test-test files** (L1 below
cleared a different each-leaf but flipped 0 files — the chains hit `var`). The loop
idiom `iota n each [ var [[t] <body with def/if> ] ]` (sort.boru:98 etc.) refuses
because `var` is a `CompileExecutesBody` macro: its handler desugars to `def t end;
<body>; __varundef t` (native_definition.go:583-654; `__varundef` exists precisely
"to let a var-body compile to a closure unit"). The desugared form IS compilable,
but the compiler can't bake a `CALL_NATIVE(var)` — the VM would re-run the handler
and trip on the tape-coupled tokens (emit.go:2020-2041 refusal). **Fix: macro-inline
compilation** — recognise `var` (and similar splice macros) and compile its spliced
desugaring INLINE (the def-locals as scoped frame locals + the body + __varundef
cleanup) rather than as a native call. Substantial; the highest-leverage remaining
compiler feature (unblocks the 14 each + 8 test-test files once their other chain
leaves also clear). Gate hard: the property fuzzer caught var-block miscompiles
across all three frame-local kinds (param/capture/for-iterator), so an off-corpus
`RunCompiledStrict` regression per kind is mandatory.

### L1 — each over a list-literal with a COMPUTED element (14 files) — FIXED (609d502b)
`[(mk)] each [...]` refuses; `[1 2 3] each` (const) compiles. The collection is a
list-carrier with a non-const element → `resolveOperand` declines (emit.go:690,
not inert-const, not a recorded event). Even `[(mk) (mk)]` at TOP LEVEL refuses
("residual value of unknown provenance"). A list with a NATIVE element
(`[(m get "a")]`, `[1 (2 add 1) 3]`) DOES compile via `RecordMakeList`
(engine.go:3170/3684) — the gap is the **user-fn-call element**. Pattern in the
files: `def specs [ (Test.test …) … ]` then iterate. **Fix: compiler** — record
`OpMakeList` for a list whose elements are user-fn-call results.
Files: all 14 `each`-class.

### L2 — `.field`/`get` on an IMPORTED-module class instance types Dynamic (8 files)
The imported class's declared field types aren't materialised in the consuming
module, so `compiled.field` is Any → `dynamic input`/Stage-2 mask. Inlining the
class into the test file makes it COMPILE (so it's cross-module typing). **Fix:
compiler (materialise imported-class field types) or repo.** Surfaces as `test-test
(Stage 2)`. Files: all 8 `*_unit_test` (the `Test.test`/run-spec harness).

### L3 — `do {k:[expr]}` map-construction provenance (4 files)
A `do`-map whose value block is a branch-merged (`if`) result or an unresolvable
fn-result has no materialisable provenance (emit.go:2140). bloom's `encode` do-map;
stats/trie fold-body do-maps. **Fix: compiler** — materialise a branch-merged /
computed do-map value via `OpMakeMap`/`OpMakeList`. (A partial repo tweak
`set:[x]`→`set:x` preserves interpret but does NOT flip — the other computed values
keep the chain.) Files: bloom_smoke, bloom_unit_spec, stats_smoke, trie_smoke.

### L4 — single-overload USER fn over an Any receiver (3 files)
`(nd find-kid)` where `nd` is Any (from a user fn's `[Any]` return): no-signature
recovery, `tryRecordPoly` declines because `find-kid` is a USER fn not a builtin
(carrier.go:989) → MarkUncompilable (engine.go:6675). **Fix: compiler** — emit a
normal user-fn call (CALL_BORU) to the best-fit candidate; SOUND because the
CALL_USER param guard (added this session) re-checks the receiver at runtime, so a
mismatch raises the same signature_error the interpreter does. Files: trie_unit_spec,
trie_unit_test, tst_unit_test (find-kid/mk-tnode).

### L5 — `push`/`set` over a Dynamic receiver (2 files)
In-place mutators over an Any receiver refuse (emit.go:2063) before element
provenance. **Fix: compiler** — make push/set poly-safe over a Dynamic receiver
(OpCallNativePoly), or repo. Files: burst_unit_test (push), radix_unit_test (set).

### L6 — gradual-Any → multi-overload user fn `apply-op` (1 file)
My cluster-C refusal — an open defect, owed a fix. **Fix: compiler** — record a
user-fn poly that re-matches the overload at runtime; OR repo narrows the
overloads. File: decision_smoke_test.

### L7 (CHECK track) — `parse <kind>` over a runtime-registered grammar (template)
`Parse.register` is a runtime side effect invisible to the static check pass →
false `parse_unknown_lang` → `fn_body_error` → 24 of template.boru's check errors
(and its force-compile `check diagnostics` block + template_smoke `lex-mustache`).
**Fix: compiler** — give `parse-register` (parse.go:275) a `ReturnsFn` that installs
the kind at check time, mirroring `parselang-register`'s `parseRegisterReturns`
(parselang.go:478), with idempotent registration for the compiled double-run.
template.boru ALSO has residual `no_signature`/`unused_def` false positives (some
may resolve once `parse` resolves; the rest are separate checker-accuracy work).

## Recommended sequence
L4 (3 files, clean, sound via param guard) → L7 (unblocks the entire template check
track) → L1 (14 files, biggest) → L3 (4) → L2 (8) → L5 (2) → L6 (1). Re-run the
corpus retest after each; a file flips only when its whole chain is clear.

This is the compiler/checker-completion endgame — a sequenced multi-pass effort.
`compile==interpret` is verified solid for what DOES compile, but that is not the
bar: every remaining REFUSAL is an open defect, and the work is closing all of them
out to COMPILATIONS, plus the template checker accuracy.

### L0 — fix attempts RULED OUT (do not repeat)
1. **Suspend-skip the Stage-2 refusal** (gate MarkUncompilable on `es.suspended==0`):
   NO-OP. The latching refusal is on the LIVE active es (suspended=0), not the
   suspended probe es.
2. **Skip the refusal if the result is already recorded** (`es.producedBy[outs[0].ID]`):
   NO-OP. The refusing dispatch's result carrier has a DIFFERENT ID than the
   closure-recording dispatch's — they are separate re-analyses of the same source
   each, so an ID match never fires.
Open inconsistency for the next pass: `RecordCall` early-returns when `!es.active()`
(emit.go:1882), so the refusal only runs ACTIVE; yet the refusing dispatch did NOT
print a tryRecordClosure entry trace despite `sig.Callable=true` (so it did not
return at the `Callable==nil` guard either). The next pass must instrument the
carrier.go:603 dispatch chain ITSELF (which tryRecord* declined, and why
tryRecordClosure's entry wasn't reached) for the SECOND active dispatch of the same
each — likely a re-dispatch during the value-def's result-type analysis. The fix is
almost certainly to make that re-dispatch take the closure path consistently, or to
not run RecordCall's code-body refusal for a Callable word already lowered as a
closure at this site. Keep the two mandatory regressions (captured-frame-local each
body must compile; non-closureable name-body must still refuse).

## CORRECTED RETEST (post-L0, run from each repo ROOT — CRITICAL methodology)

Earlier "8 compiled / 23 check-diagnostics" was a MEASUREMENT ARTIFACT: the spec
files do `import "./bloom.boru"` resolved relative to CWD, so running from the wrong
dir failed every import → spurious `undefined_word` check errors. Run each file FROM
ITS REPO ROOT (`cd bloom-filter; boru check test/bloom_unit_spec.boru`). Correct state:
- **interpret 44/48** (4 `*_prop_spec` are slow property GENERATORS, >60s timeout — not errors).
- **check 47/48** — only `template.boru` (one `fn_body_error`). THE CHECK TRACK IS FINE.
- **force-compile 14/48**. Refusal breakdown (by leaf, files):
  - 14× `code-body word each` — but the each PATTERN now compiles (L0); the residual
    cause is a MODULE CODE-BODY WORD in the body (`Test.run-spec`).
  - 8× `code-body word test-test`; 2× `test-describe`; (run-spec → test-describe).
  - 5× `unmatched dispatch recovered at find-kid / mk-tnode / lex-mustache` (user-fn poly).
  - 2× `code-body word do`; 1× each `fold`, `set`(dynamic), `push`(dynamic), gradual-Any.
  - 2× real `check diagnostics` (template-family).

### L0 LANDED — commit 5f6561e4 (sound, gated, verify-bytecode + full suite green)
collectBodyLocalDefs excludes body-local `def`/`var` names from ComputeCaptures.
The each-body-local-value-def + var-loop patterns now compile (proven: `each [var
[[s] (s get "name") 0]]` → COMPILES). Flips 0 voxgig FILES (chains), but is the
foundation: the test-file eachs now refuse only on the MODULE code-body word inside.

### DOMINANT REMAINING LEAF — the boru:test framework's code-body words
`Test.run-spec` / `test-test` / `test-describe` (lang/go/modules/test.go:227,275,1411)
DECLARE a CallableSpec (BodyPos 1, BodyOut 0) — so they SHOULD compile via the
closure path — yet refuse "Stage 2". The recursive describe/spec body (run-spec calls
test-describe with a body that recursively calls run-spec) fails the closure compile.
This blocks ~all test files. NOTE: the repos' OWN gate (test/divergence/run.sh) is
INTERPRET + `boru --compile` (which silently routes a refused program to the
interpreter) must SUCCEED AND AGREE — i.e. compile==interpret, which HOLDS for all 48
(0 miscompiles). That gate is BLIND to these refusals; it does not make them
acceptable, and force-compile coverage of the test framework is owed work, not a
stretch. Next pass: unmask test-describe's closure-body refusal (instrument
recordClosureDispatch for word=="test-describe") and compile the recursive spec body.

### ROOT-CAUSED: the test-framework leaf IS the single-overload user-fn recovery leaf
test-describe's closure body fails with Reason `unmatched dispatch recovered at
run-cases` (instrumented compileClosureBody). `run-cases` (test.go:1403) is a
SINGLE-overload boru user fn `fn [[| subject:Scalar cases:List][]…]`. Called with
Any-typed spec data, matchSignature can't statically commit → the engine.go:6664
recovery branch runs. tryRecordPoly DECLINES because its gate (carrier.go ~line 26)
is `if !matchReg.IsBuiltinWord(word) return false` — run-cases is a user `def fn`,
not a registered builtin (test-record IS a sub-registry builtin, so it passes; that's
the asymmetry the 6656 comment notes). So the recovery MarkUncompilable's.

THIS ONE LEAF blocks: the boru:test framework (run-spec → test-describe → run-cases →
~all test files) AND find-kid / mk-tnode / lex-mustache (the 5 direct recovery files).
Highest-leverage remaining leaf by far.

**Fix direction (soundness-critical, next focused pass):** in the engine.go:6664
recovery, when tryRecordPoly declines AND the word is a SINGLE-overload user fn,
record a user-fn dispatch (a plain CALL_BORU to its one sig — no poly re-match needed
since there's only one overload) GUARDED by checkParamContract (the param guard
landed this session). Soundness: at run time the concrete args either match the sole
sig (dispatch == interpreter) or fail the guard (raise == interpreter's no_signature).
A MULTI-overload user fn must STILL refuse (Cluster C) — the guard would raise where
the interpreter dispatches a sibling. Mandatory off-corpus regressions: a single-
overload user fn over Any args (must compile + RunCompiledStrict==interp), and a
multi-overload one (must still refuse — its own open defect). The repos' OWN gate is
compile==interpret, which today holds only because the runtime silently routes the
refused program to the interpreter; this fix closes that defect with native coverage.

### L4 LANDED — commit 913fe6a9 (Any + disjunct recovery, sound, gated)
tryRecordRecoveredUserFn records a guarded CALL_USER for a single-overload arg-bearing
user fn at BOTH the disjunct-straddle (6635) and Any-carrier (6675) recovery sites.
Clears find-kid / mk-tnode / lex-mustache (now at their next chain leaf: "operand of
unknown provenance at do/size"). Net voxgig flips: 0 (deep chains), same as L0/L1.

### test-framework run-cases reaches a THIRD recovery path (engine.go ~6700)
Instrumented: test-describe's body still fails Reason "unmatched dispatch recovered at
run-cases" AFTER L4 — run-cases there is dispatched over CONCRETE-but-statically-
mismatched args (NOT Any/disjunct carriers), so it skips both L4 sites and falls to
the 6700 concrete-mismatch fall-through ("genuine type error" path). Extending
tryRecordRecoveredUserFn to 6700 is plausibly sound (single-overload + the param
contract = dispatch-or-raise == interpreter) BUT is the RISKIEST site: concrete args
mean the param guard must match the interpreter's matchSignature coercion EXACTLY, and
a mismatch is a silent miscompile the differential is blind to. Deferred to a focused
pass with the mandatory off-corpus regression (a single-overload fn over concrete-
mismatched args that the interpreter coerces vs one it rejects). NOTE: even clearing
6700, test-describe likely chains further (test-test, recursive run-spec). The repos'
OWN gate is compile==interpret, which holds only because the runtime silently routes
the refused program to the interpreter; force-compiling the recursive test framework
is owed work, not a stretch goal.

### CURRENT STATE (this session: L0 + L4 landed, sound/gated/committed)
interpret 44/48 (4 slow generators), check 47/48 (only template fn_body_error),
force-compile 14/48. Compile leaves remaining: each/test-test (test framework, deep
chain), do/provenance (2+1), dynamic set/push (1+1), fold (1), gradual-Any multi-
overload (1, refused — an open defect), template check (1). compile==interpret holds
for all 48 (0 miscompiles) and the repos' own gate passes, but only because refused
programs are silently absorbed by the interpreter; the gap is native force-compile
COVERAGE, dominated by the recursive boru:test framework, and every unit of it is a
defect tracked to closure.

### CHECK leaf (template.boru — the only check failure) ROOT-CAUSED
All 24 errors are one cause: `fn_body_error` → `parse_unknown_lang: no parser
"mustache"/"liquid"/"jinja" registered`, raised analysing lex-mustache/liquid/jinja's
bodies (`def _ (parse mustache src)`). Mechanism: `parse <kind>` (native_macro.go:
parseHandler) resolves the kind via parseKindRegistered → checks the ParseLang export's
Fields for "parse_<kind>". The template registers with `Parse.register` (boru:parse,
parse.go:276 parse-register) whose sig has ONLY a Handler — no ReturnsFn — so in CHECK
mode the Handler never runs (carrier.go:489 uses ReturnsFn/static Returns, not the
Handler), the kind is never injected into ParseLang exports, and the fn-body analysis
fails. `ParseLang.register` works because it HAS a check-mode ReturnsFn
(parselang.go:74 parseRegisterReturns → idempotent parseRegisterInstall). The template
imports boru:parselang (required — removing it breaks interp + yields 18 other errors),
so parseNamespaceBound=true makes the check STRICT (no lenient degradation).

**Fix (next pass, soundness-sensitive):** give parse-register a check-mode ReturnsFn
that registers the kind (build spec from args[1] + RegisterHostParser) so the fn-body
analysis resolves `parse <kind>`. RegisterHostParser (parselang.go:200) ERRORS on
double-register, so the check ReturnsFn + the runtime Handler + the compile double-run
need source-identity idempotency — mirror parselang's idents map (same source call →
skip; different → error). Gate with bytecode_register_soundness_test.go +
check_type_value_test.go (both already pin the parselang idempotent-ReturnsFn
contract) + a new template-style Parse.register-in-fn-body check regression. This is
the LAST check-track issue; landing it makes check 48/48.

### template.boru — the 18 post-L7 check errors are ALL the gradual-dispatch limit
L7 cleared the 24 parse_unknown_lang FALSE POSITIVES (the kinds ARE registered at
runtime), which unmasked 18 deeper errors — ALL rooted in statically analysing the
template's DYNAMIC codegen (it lexes via a runtime-registered parser → List<Any>
tokens → dispatches codegen fns on them):
- ~12 `no_signature: assuming best-fit candidate for analysis` — the codegen
  dispatches over statically-ambiguous token types. NOT .Dynamic (so the
  intentional-dynamic suppression I tried does not catch them — reverted); these are
  the recovery diagnostic at engine.go:6734, ERROR severity.
- ~6 `syntax_error: unmatched opening/closing parenthesis` (source position
  unknown) in SIMPLE fns (after-word has no paren-in-string yet errors) — a recovered
  dispatch over the dynamic codegen OVER-COLLECTS forward args in the fn-body
  sub-analysis (carrier.go:2921 sub.Run) and eats a `)`, leaving the `(` unmatched
  (engine.go:995 "phantom unmatched paren"). Analysis-state bleed across fns.
Both are the language's KNOWN gradual-dispatch limit — exactly what sort/CLAUDE.md
documents: "boru check is advisory … known false positives on first-class function
values, which this library threads through every sort." The template is a dynamic
meta-compiler; full static check is structurally beyond targeted fixes. Paths:
(a) a checker DESIGN decision — downgrade the recovery `no_signature` from ERROR to a
non-blocking severity (broad, deserves its own gated pass; many existing tests assert
it as error), AND fix the recovery forward-collection/paren bleed (deep); or
(b) accept advisory check (the repos' own gate is compile==interpret, which HOLDS).

### DEEP-COMPLETION PASS: the test framework can't be force-compiled via recovery
Per the user's "deep compiler completion" choice, I extended the recovery (engine.go
~6700, the CONCRETE-mismatch fall-through) to try poly + the single-overload user-fn
helper, to compile the boru:test chain (run-spec → test-describe → run-cases →
test-invoke → recursive test-describe). Result, level by level:
- **run-cases** (single-overload USER fn): the guarded CALL_USER helper records it
  SOUNDLY (param contract raises == interpreter). ✓
- **test-invoke** (a sub-registry NATIVE over concrete-mismatch): tryRecordPoly
  records an OpCallNativePoly that re-matches OPTIMISTICALLY at run time — UNSOUND.
  **The differential (TestCompiledCombinationParity / TestSpecCompiledOrFallback)
  CAUGHT it**: `is 5 Integer`, `teq Integer Integer`, `flex (Node)` are GENUINE type
  errors the interpreter raises (`signature_error`), but the poly re-match dispatched
  them (compiled=nil/flex_error ≠ interp=signature_error). Concrete mismatches really
  ARE genuine errors; poly can't tell type-inference imprecision from a real error.
  Reverted; verify-bytecode green again.
- **recursive test-describe** (a CODE-BODY word recovery): no sound recovery path
  (poly excludes code-body; not a user fn).

**Conclusion:** the recursive, dynamic boru:test framework cannot be force-compiled via
recovery handling — that is fundamentally unsound (the differential proved it). The
SOUND path is deeper: (a) TYPE-INFERENCE so the spec-data flow gives test-invoke /
run-cases args their real types → they match statically → NO recovery; or (b) a
VM-level GUARDED native call (a CALL_NATIVE carrying the matched sig as a param
contract, like CALL_USER) so a native concrete-recovery raises == interpreter. Both
are multi-session features, NOT a recovery trick. The single-overload user-fn helper
(L4) remains the boundary of what can be compiled without miscompiling; concrete-
mismatch natives/code-body stay refused — unimplemented cases, each an open defect
owed a fix, silently absorbed by the interpreter meanwhile. compile==interpret holds
for all 48 (0 miscompiles).

### dx-report reframing + paren fix + no_signature finding (this pass)
**The template/dx-report.md is the key artifact** (§11–§13): it documents that
template.boru's check errors are CHECKER LIMITATIONS not module defects ("treat boru
check as advisory"), that soundness holds (compile==interpret byte-identical), and
that `parse_unknown_lang` was "the single biggest blocker" — which **L7 fixed**.
Confirmed: ALL 8 library deliverables across the repos check + force-compile CLEAN
except template.boru; the test files' authors DOCUMENT the framework refusals as
expected fallback. So the repos are correct; boru is the limitation — and every one
of those documented refusals is a boru defect owed a fix, not a sanctioned outcome.

- **dx-report updated** (template repo, branch dx-report-boru-update) to reflect L7
  (24 → 18 check errors, parse_unknown_lang: 0).
- **PAREN BLEED FIXED** (commit 016e9ffc): checkModeFallbackPositions gathered past
  the enclosing `)`; depth-tracking break fixed it. template.boru 18 → **12** errors
  (all 6 emergent fn_body_error unmatched-paren gone). Gated, regression added.
- **no_signature severity downgrade — NOT VIABLE** (the user's listed final item).
  The template's 12 remaining no_signature are at the CONCRETE fall-through
  (engine.go:6798), not the Any branch — user-fn dispatches over statically-imprecise
  concrete types that resolve at run time. Downgrading the recovery no_signature to
  info made template check CLEAN (37 info, 0 error) BUT the suite caught it hiding
  GENUINE errors: TestAddGradualConcatReturnTyping (String-concat → Integer param)
  and TestSliceDynamicReceiverRefines (String|List → Integer) assert provable
  mismatches MUST flag. The recovery no_signature legitimately catches genuine type
  errors, INDISTINGUISHABLE from the template's imprecision without confidence/
  provenance tracking. Reverted. **The real fix is TYPE INFERENCE** — give the
  template's get/dynamic-sourced args their correct types so they match statically
  (no recovery, no false positive), NOT a severity change. Deep, multi-session.

### template's remaining 12 no_signature — reproduced + root-caused (this pass)
PAREN FIX landed (016e9ffc): template 18 → 12. The remaining 12 are EMERGENT from
mutual recursion (compile-tagged-seq ↔ liquid-if/liquid-for) over dynamic dispatch —
REPRODUCED minimally: a 2-fn mutual recursion `cts ↔ lif` where the recursive arg
comes from `(blk get "next")` triggers 2 `cts` + 1 `get` no_signature; a single fn
does NOT. The recovering recursive/mutual calls have suppress=0 but FnNameInflight>0
(instrumented) — the codebase's same-fn SuppressBodyErrors (carrier.go:2860, "a
re-run whose args narrowed a param … can spuriously fail dispatch") does NOT catch
them because the error is emitted at the CALL SITE in the OUTER body, not in the
re-entry.

ATTEMPTED FIX (reverted): gate the recovery diagnostic on FnNameInflight[w.Name]==0
(suppress in-flight fn recoveries). SOUND + narrow (verify-bytecode + suites pass;
TestAddGradualConcat/TestSlice genuine errors still flag) and cleared 2 template
errors — BUT PARTIAL/FRAGILE: it suppresses only the WITHIN-BODY recursive call
(w.Name in-flight), not the mutual partner's DEF-TIME analysis (lif analysed at `def
lif`, cts not in-flight) nor the `get` cascade (get over the imprecise recovered
type). Couldn't isolate a clean minimal regression → reverted (no fix without a solid
regression). The REAL fix is mutual-recursion TYPE PRECISION: a mutual call to an
in-flight fn should yield its DECLARED return type so the partner's analysis and the
downstream `get`s stay precise (the declared-return induction hypothesis at
carrier.go:2839 IS used for the SAME key, but the narrowed-arg re-entry has a
DIFFERENT key and re-analyses imprecisely). Deep, multi-session — proper mutual-
recursion fixed-point / group handling.

### template's last 12 — narrowed further: recursive partner's result comes back as an ATOM
Instrumented the `get` recovery in both the minimal mutual repro and template.boru: the
recovering `get`'s RECEIVER is an `[Atom conc=T]` (key `[ProperString]`), NOT a `[Map]`.
So `(blk get "next")` / `(more get "next")` recover because `blk`/`more` — bound to a
recursive/mutual partner's result — are typed ATOM, not the partner's DECLARED `[Map]`.
get(Atom, key) has no compatible sig → bestMatch < 0 → flagged (NOT caught by the
bestMatch>=0 fix, which only handles the args-match case like the self-recursion repro).
A simple `do {x:[(more get "a")]}` over a local Map resolves FINE (0 errors), so it is
NOT general map-literal scoping (dx-report §4) — it is RECURSION-SPECIFIC: the partner's
recovered result is an Atom. Open: WHY the recovery splices an Atom for a declared-`[Map]`
fn instead of its declared return (checkModeAssumeSig SHOULD splice the assumed sig's
declared carriers). Next pass: instrument the recovery's spliced `results` types for the
partner (cts/lif) — if Atom, the recovery is mis-typing a declared-return fn's result
under mutual recursion (the real bug); if [Map], the Atom is an undefined-in-reentry
binding. THEN the fix (preserve the declared return through the recursive cycle) + the
bestMatch>=0 fix together clear the template. compile==interpret holds (0 miscompiles).

### BREAKTHROUGH: gradual-Any forward operand (committed e6757208) — template 12 → 7
Reliable matchSignature instrumentation (effectiveResolved + the forward Word-match at
engine.go:6111) showed the template's pipeline no_signature (gen-program / lex-mustache /
lex-liquid / lex-jinja / engine-known) were NOT recursion artifacts — their String params
were fed Any operands (engine/source/src flowed from `opts get` over an Options receiver),
and sigArgMatches(String, Any) is false → fell to the 0-arg fallback. FIX: accept an
Any-typed forward operand for a concrete param in PURE CHECK (gated !Compiling so compile
still refuses → no miscompile; verify-bytecode PASSES). Cleared all 5 pipeline errors.

### template's last 7 — root-caused to liquid-if/liquid-for return mis-inference
The remaining: compile-tagged-seq(2) + get(4) + gen-program(1, a cascade). ROOT: `def blk
(liquid-if toks i)` then `blk get "next"` fails because `blk` is typed **Atom**, not the
declared **[Map]**. liquid-if/liquid-for are declared `[Map]` (template.boru:676,703) and
EVERY body branch returns `do {…}` (a Map) or `raise` — the bodies are correct and the
runtime is GREEN (5/5). The checker mis-infers the call RESULT as Atom under the
compile-tagged-seq ↔ liquid-if/liquid-for mutual recursion. Confirmed by annotating `def
blk:Map` → the 4 get/compile-tagged-seq no_signature become 2 `type_error: value liquid-if
does not unify with declared type Map` (net 7→3), i.e. the checker genuinely believes
liquid-if returns non-Map. NOT located: which dispatch path produces liquid-if's Atom
result — it does NOT reach stepWord's sig==nil recovery (2227), and wrapping
carrierResults' ReturnsFn output (490) with an assume-guarantee reconciliation did NOT
change it, so liquid-if's result is spliced via a third path (execFnDefSig/CallBoru residual
or spliceFnCheckTail). NEXT: instrument spliceFnCheckTail callers to find where the
named-fn body residual becomes the dispatch result, then reconcile a divergent residual
with the declared concrete return there (assume-guarantee). Gate: verify-bytecode +
TestAddGradualConcat/TestSlice still flag + the template runtime suite stays 5/5 green.

### last-7 FINAL root-cause: liquid-if returns Dynamic-Any → dynamic-receiver dispatch
Instrumented AnalyseFnBody (carrier.go:2982): liquid-if / liquid-for return
`[Any(dyn)]` — a DYNAMIC Any carrier (bails=0, so not the recursion-bail path; the
mutual-recursion body analysis collapses the [Map] residual to a dynamic Any). So
`def blk (liquid-if …)` binds blk = Dynamic-Any, and `blk get "next"` is a `get`
dispatched over a DYNAMIC ANY RECEIVER — which the stack-phase match rejects (the
.Dynamic modality is not honored for a stack receiver). That is exactly the open
"dynamic-receiver native dispatch" project (task #8), NOT a local fix: a stack-phase
gradual-Any relaxation (even gated to .Dynamic only) OVERMATCHES — template went 7→18
(11 cascade errors) because accepting a dynamic operand for every concrete stack param
picks wrong overloads across the whole program. The forward-phase analogue is safe
(committed) because a forward operand is positionally anchored; the stack phase is not.
So the remaining 6 (compile-tagged-seq + get) + 1 cascade (gen-program) require the
dynamic-receiver dispatch work, which must make `get`/accessor dispatch honor a Dynamic
receiver WITHOUT relaxing every stack param — a targeted accessor-family change, not the
blanket stack relaxation. Runtime GREEN 5/5; source correct; compile==interpret holds.

### last-7 — pinned to a dispatch-PATH disconnect (def-value dispatch yields Atom)
Instrumented the forward Word-match (engine.go:6111): `def blk (liquid-if toks i)` binds
`blk` to a **concrete Atom** (BIND blk type=Atom dyn=false carrier=false forWord=def) — so
the `blk get …` receiver is an Atom, no get overload matches, recovery fires. KEY: this is
a DISPATCH-PATH disconnect — liquid-if's AnalyseFnBody/ReturnsFn returns `[Any(dyn)]`
(carrier.go:2982 instrument) but the value bound to blk is a concrete Atom, and liquid-if
NEVER reaches stepWord's sig==nil recovery (engine.go:2227, LIFDISP silent). So the
def-value paren-group `(liquid-if toks i)` is evaluated via a THIRD dispatch path (not
stepWord→ReturnsFn→carrierResults, not spliceAnonCheckResult, not spliceFnValueCheckResult)
that, under the suspended/nested mutual-recursion analysis (fnBodyGuard, carrier.go:2787),
splices a concrete Atom instead of liquid-if's declared [Map] or its Any(dyn) residual.
NEXT: find which path evaluates a def-value paren-group calling a named user fn in check
mode and why it produces an Atom under nested analysis; reconcile to the declared return
there. The accessor-receiver Dynamic-Any fix is INERT until then (blk is an Atom, not a
Dynamic Any, by the time get dispatches). Instrumentation gives inconsistent types across
the two paths — pin the def-value path specifically. Runtime GREEN 5/5; compile==interpret.

### ROOT CAUSE CRACKED: the cluster was a FORWARD REFERENCE, not dynamic dispatch
Instrumented the def handler (native_definition.go:237): `def blk (liquid-if toks i)` binds
blk to a concrete Atom while `def more (compile-tagged-seq …)` binds a Map carrier. The
difference: compile-tagged-seq (template.boru:628) calls liquid-if/liquid-for, which are
defined LATER (676, 703) — FORWARD REFERENCES. boru analyses fn bodies at DEFINITION time, so
compile-tagged-seq's call to the not-yet-registered liquid-if degraded to an undefined-word
Atom, cascading false no_signature through `blk get …`. compile-tagged-seq's SELF-recursion
resolves (in-flight), but a forward ref to a SEPARATE later def does not. The earlier
"Dynamic-Any / dynamic-receiver" reading was a downstream symptom, not the cause.

FIX SHIPPED (template repo): forward-declare liquid-if/liquid-for before compile-tagged-seq
(real defs shadow; runtime unaffected). `boru check` 7 → 1; runtime 5/5 green. So template is
24 → 1 across the session (7 boru fixes took 24→7; the gradual-Any fix 12→7; stubs 7→1).

CHECKER TWO-PASS — explored + REJECTED as unsound. A pre-pass that registers all top-level
fns (so forward refs resolve) made template 24→0 AND needs a per-pass FnSummaries/FnInflight
reset in CheckState.Begin (a real latent reused-registry bug — those caches were NOT reset),
but it is fundamentally TOO BROAD: it cannot distinguish a fn-BODY forward ref (legitimate,
deferred to call time) from a top-level USE-BEFORE-DEF (must flag). It broke
TestCheckTopLevelUseBeforeDefStillFlagged + TestPredicateCheckMode_TypedBindingAccepted.
Keeping only fn defs from the pre-pass didn't fix it (a top-level fn use-before-def is also
masked). The RIGHT checker fix re-types fn-body forward refs WITHOUT a blanket pre-register —
re-analyse a fn body once at end-of-pass when an FnBody-tagged undefined_word resolved to a
now-registered fn, propagating its declared return. Deep; the FnSummaries/FnInflight Begin
reset is a sound standalone sub-fix worth landing on its own.

REMAINING: gen-program (1) — a make-Compiled dispatch over Any args that the gradual-Any
forward fix accepts (gradualAny=true, instrumented) yet still recovers; NOT reproducible even
with the full tpl-compile shape extracted — emergent from the whole 790-line module.

### COMPILE TRACK — complete analysis (session end). CHECK is 48/48 clean; COMPILE: 15/15 libs, 33 test files refuse.
Every refusal is a MarkUncompilable that yields no wrong answer (compile==interpret preserved; 0 miscompiles across 48) —
and every one of the 33 is an OPEN DEFECT owed a fix: the runtime silently routes those programs to the interpreter,
hiding the failure rather than closing it.
Distinct features required (first-refusal histogram over the 33): each×14, test-test×9, do×2, fold×1, not-materialisable×3.
1. SHAPE-AGNOSTIC CLOSURE BODIES for gradual-Any fold/each (callable_words.go:98, dominant).
   Root: a closure body is compiled for ONE input shape (List element vs Map {k,v} entry); a gradual-Any
   collection is shape-ambiguous, so a poly re-match would feed the wrong shape and diverge. CONFIRMED GENUINE
   (not a spurious refusal — and still a defect owed a fix): sort's 16 algorithms each/fold over Any-typed
   INTERMEDIATE lists (built by swaps/partitions) — not source-annotatable (genuinely dynamic). Fix = compile the
   body for BOTH shapes (or a shape-generic body) and let OpCallNativePoly pick List-vs-Map at runtime. test-test×9
   CASCADES from these (test-test HAS a Callable+compiled-closure path, test.go:294; it refuses only because its
   body contains the un-lowerable each/fold).
2. do/error EXCEPTION-HANDLING lowering (emit.go:2041, bloom decode bloom.boru:306-310). `do [body] error
   [handler]` over a name-referencing body = try/catch in bytecode (body+handler closures + a catch op).
3. NOT-STATICALLY-MATERIALISABLE collections (template×3) — a const-sized collection the const pool can't size.
PROVEN NON-VIABLE this session: (a) disabling refusals → silent miscompiles (breaks the invariant); (b)
per-construct source surgery → runtime-risky (a `def x:String` annotation broke a runtime test) + doesn't
converge (libraries' intermediates are genuinely Any); (c) the features are multi-session VM/emitter work.
The five library repos need NO source change — their deployable code is check- AND compile-clean; only the
boru:test-driven test files hit the shared emitter gap. Template repo WAS updated (forward-decl + Any-param).

### LANDED — gradual-Any each/fold/scan over a Dynamic collection (feature #1, the dominant leaf)
The "SHAPE-AGNOSTIC CLOSURE BODIES" feature above was over-stated: for the TOKEN-quotation body
(`each [body]`, what voxgig uses) the body is ALREADY shape-generic — both the List overload and the Map
overload present the closure the bare element/value (`ClosureInValue`); only the LAMBDA form (`each (kv => …)`)
diverges (List=element vs Map=KeyVal). And the `:98` ambiguity gate is INHERENTLY token-only: a TList token
body matches BOTH the {…,TList} and {…,TMap} overloads (count 2 → refuse), while a TFunction lambda matches
only the single {TFunction,TMap} overload (count 1 → never reaches the gate). So no "compile both shapes / poly
re-match" was needed.

**Fix (sound, gated):** (a) `CallableSpec.CrossCollectionTokenShape` (new) declares a word whose token body is
shape-generic across its List/Map overloads — set on each/fold/scan. (b) `callable_words.go` no longer refuses
the gradual case for such a word + a concrete (non-lambda) body: it records the FIRST-reachable (List) overload's
closure ONCE. (c) the committed list handlers (`eachHandler`/`foldWithInit`/`foldNoInit`/`scanHandler`) are made
RUNTIME-ROBUST — when the value is the sibling collection (a Map) at run time they delegate to the map handler
(and vice-versa, defensively), so the SAME compiled closure drives either shape == the interpreter (which picks
the overload by the runtime type). The cross-delegation is unreachable in the interpreter (matchSignature gates a
Map away from a TList sig), so it is live ONLY on the compiled committed-overload path — zero interpreter change.

**Gate:** `make verify-bytecode` GREEN (compile==interpret, 0 miscompiles, incl. -race + borudebug). New off-corpus
regression `lang/go/bytecode_gradual_each_test.go`: each/fold/scan over a Dynamic-Any collection compile NATIVE
(no FALLBACK island) and RunCompiledStrict==Run for BOTH List and Map runtime shapes — incl. ONE compiled fn body
driven by a List AND a Map (the strongest cross-delegation proof) — plus parity-only cases for a lambda-over-Dynamic
and empty collections, which still REFUSE and are silently routed to the interpreter by the harness: open defects, not
coverage. `make fmt/vet/lint` clean. (Pre-existing branch debt unrelated to this change: `TestCheckAccuracyRatchet`
fails identically on baseline — stale pins, falsePos 18 vs 23 + unflagged 205 vs 189; the check path is untouched by
this compile-side fix.)

**Impact:** the gradual-Any "ambiguous overload (List vs Map)" refusal is ELIMINATED. First-refusal `each` dropped
14 → 6 (the 6 residual are the Stage-2 MASKED code-body leaf — an un-lowerable inner word like `do`/`Test.run-spec`,
NOT the gradual-overload leaf). NET voxgig FILE flips: 0 (the chains hold — test-test×9 / do×2 / Stage-2-masked each
still gate), consistent with every prior single-leaf landing. This is the sound FOUNDATION the test-test×9 cascade
sits on (test-test refuses only because its body contains an un-lowerable each/fold — the gradual-each piece of that
is now cleared; the residual is the test framework's recursive run-cases user-fn-poly leaf, ruled multi-session/
unsound-via-recovery above). Remaining frontier (first-refusal, excl. 6 slow prop_spec generators): test-test×9,
each×6 (Stage-2 masked), do×2, dot×1, size×1, apply-op gradual-multi-overload-user-fn×1, set×1, push×1, fold×1
(Stage-2 masked), check-diagnostics×1.

### "BUILD THE BIG FEATURES" PASS — VM guarded CALL_NATIVE infra LANDED; recovery wiring PROVEN UNSOUND, reverted
User direction: build the multi-session features to flip the 33 test files, NOT leaf-grinding. Established findings:

- **`do [...] error [...]` catch-frames ALREADY compile** (feature #2 is largely done) — verified toplevel + fn ok/catch
  paths force-compile. The bloom/do residual is `dot`/`get` over the **Dynamic Error result** of a `do`
  ("unmatched dispatch recovered at dot"): `get "code"` over a do-Error compiles (poly), but `dot code` recovers —
  Error's accessor is a 2-overload `{String|Atom, Error}` set, so it is NOT the single-overload guarded case.

- **VM-level GUARDED CALL_NATIVE infrastructure LANDED (commit f08de09d, sound, gated, unit-tested).** `SigRef.Guard`
  + `checkNativeParamContract` (the native twin of the CALL_USER `checkParamContract`): a guarded CALL_NATIVE re-checks
  the concrete args against the committed sig at run time — dispatch on a match (== interpreter), raise signature_error
  on a miss (== interpreter finding no overload). Additive/inert (Guard defaults false). `vm_guarded_native_test.go`
  pins the contract (hand-built Program): match dispatches, mismatch raises, unguarded control dispatches.

- **WIRING IT AT THE CONCRETE-MISMATCH RECOVERY (engine.go:6787) IS UNSOUND — reverted.** `tryRecordGuardedNative`
  (single-overload-gated, compile-pass-only) recorded a guarded CALL_NATIVE for the recovered native instead of
  refusing. `make verify-bytecode` CAUGHT a miscompile: `is 5 Integer`, `is 'x' Integer`, `teq Integer Integer` —
  the interpreter RAISES signature_error (a FORWARD-COLLECTION/arity failure surfaced as the recovery), but `is`/`teq`
  declare **`(Any,Any)` sigs**, so the type-guard is VACUOUS (Any always passes) → the guarded call dispatched where
  the interpreter raised. ROOT: the concrete-mismatch recovery fires for reasons a PARAM-TYPE guard cannot see
  (forward-collection state / arity / a vacuous Any sig), so committing+guarding diverges from the interpreter. The
  single-overload condition is necessary but NOT sufficient. Reverted the wiring (carrier/engine/emit/lower); kept the
  sound infra commit. **This re-confirms the doc's conclusion: the test framework's sound path is TYPE-INFERENCE** (give
  the spec-data args their REAL types upstream so the dispatch matches statically and NEVER reaches recovery), NOT any
  recovery-site trick. test-invoke is a 2-overload `{Atom|String, List}` native with DISTINCT closures (can't be proven
  handler-equivalent), so neither single-overload-guard nor same-handler-poly applies — type-inference is the only sound
  route. The guarded CALL_NATIVE infra remains the sound VM substrate for the residual recovery cases once type-inference
  shrinks them. NEXT: the spec-data type-inference feature (flow concrete types through the boru:test run-spec/case data
  so run-cases/test-invoke args match statically).

### CONCRETE-MISMATCH RECOVERY (engine.go:6787) IS UNSOUND TO GUARD — proven 3× by the differential. DO NOT RETRY.
Three differential-caught attempts to compile the recovered concrete-mismatch dispatch, all reverted:
1. Guarded CALL_NATIVE (single-overload native): caught `is 5 Integer` / `teq Integer Integer` dispatching where the
   interpreter raises — vacuous `(Any,Any)` sigs + the recovery is a forward-collection/arity failure a type-guard can't see.
2. Guarded CALL_USER (single-overload user fn run-cases, the doc's deferred "riskiest" option): caught
   TestSpecCompiledOrFallback 3 + TestPropertyDifferential 1 divergences. Even a non-vacuous declared-param guard diverges —
   the recovery fires for reasons (forward-collection state, arity, coercion) the param guard does not replicate.
CONCLUSION: the concrete-mismatch recovery CANNOT be compiled by committing+guarding ANY dispatch; the only sound fix is to
PREVENT the recovery (precise types upstream), never record at the recovery site.

### THE STRUCTURAL WALL: test-describe is a RECURSIVE code-body word — needs TWO large features, not one.
test-describe (a Callable code-body word) has a body that dispatches run-spec, which re-dispatches test-describe — a
RECURSIVE higher-order closure. Compiling it needs closure-compilation of a recursive code-body word (a body calling a
not-yet-finished enclosing closure unit), which the current closure compiler cannot lower. So flipping the test-test×9
cluster requires BOTH (a) spec-data type-inference AND (b) recursive-code-body closure compilation — two large,
independent, multi-session features. EVERY one of the 33 refusing files is blocked behind one such large feature (test
framework ×~9-18; sort comparator Stage-G; trie module-word-in-closure-body; do-map / recursive-fn provenance;
not-materialisable) — none is a short chain; all confirmed multi-session. compile==interpret held throughout (0
miscompiles); the soundness invariant was never weakened to chase a flip.

### UNMASKING FINDING: the SORT files are gated by the comparator-each (Stage-G), NOT the test framework.
Instrumented the throwaway closure probe (callable_words.go:262) to surface the swallowed Stage-2 reason.
sort_unit_test's `test-test` body refuses on `code-body word each (Stage 2)` — but that `each` is NOT the test
framework's: it is `Sort.bubble`'s INTERNAL comparison loop, surfaced when the test body calls the algorithm. The
algorithms apply a captured comparator as a TRAILING fn-value: `def c ((arr get i) (arr get (i add 1)) comp)`
(sort.boru bubble-sort:236) — the Stage-G "dynamic value precedes/trails args" leaf (apply a runtime Function operand,
OpCallDynamic ordering). So sort_smoke + sort_unit_test + sort_unit_spec are gated by the SAME comparator-each leaf,
independent of the boru:test recursion. Fixing the trailing-fn-value apply (a bytecode LOWERING feature, plausibly more
tractable than type-inference) would unblock the sort family — a better next target than the test framework for FIRST
file flips. The test-test mask hid this: the recursion wall blocks the _spec files that drive run-spec/test-describe,
but the imperative _test files and _smoke files bottom out at the library's own leaves (comparator / module-word-in-body
/ do-provenance). Re-segment the 33: ~test-framework-recursion files vs library-leaf files — the latter may flip without
the recursion feature.

### COMPARATOR / TRAILING-FN-VALUE LOWERING — a latent MISCOMPILE found+fixed; the lowering scoped.
Attacked the sort comparator leaf (`def c (prev key comp)` — apply a captured `comp:Function` to two values).
- **MISCOMPILE FOUND + FIXED (commit d3dda735).** A closure body whose residual is an unapplied fn-value apply
  (`[1 2] each [(x x comp)]`) COMPILED to `[fn, fn]` while interpreting to `[0 0]` — the each took the unapplied
  comp off the residual top. Off-corpus (no captured-comparator-in-closure shape in the curated corpus), so
  verify-bytecode passed clean while the bug was live. STOP-GAP: such a closure body now REFUSES — containment
  only, mirroring the fn-body unapplied-fn-value refusal + resolveDynamicApply's main-residual refusals. Gated +
  off-corpus regression (bytecode_dynapply_body_test.go). So the comparator-each now REFUSES rather than
  miscompiles — compile==interpret restored for this whole class, with the refusal left as an open defect that
  the lowering below is owed to close.
- **THE LOWERING (compile the apply, the file-flipping step) is a real VM feature, scoped:** the existing
  dynamic-apply machinery (resolveDynamicApply/trailingApply, OpCallDynamic/Trailing) does NOT cover the
  comparator shape: OpCallDynamicTrailing is BOUNDED TO 1 ARG (bytecode.go:163 — >1 arg's non-callable island
  forward-collection would mis-order), trailingApply requires EXACTLY 2 residual values with an EVENT-produced
  fn, and resolveDynamicApply runs only on the MAIN program residual, not fn/closure unit bodies. The comparator
  is a 2-arg trailing apply with COMPUTED (event) args and a CAPTURED (param) fn. So the lowering needs: (a) an
  N-arg trailing dynamic apply (a new opcode, or relax OpCallDynamicTrailing's 1-arg bound for the
  GUARANTEED-callable Function-param case where there is no non-callable island), (b) wiring resolveDynamicApply
  into the unit-body reconciliation (reconcileResults) with the residual VALUES threaded through, (c) exact
  arg-ordering to match the interpreter's top-down bind (soundness-critical — the off-corpus regression's real
  comparator results [0 0]/[1 -1]/[3 -1 1] are the gate). Even then the sort algorithms chain further (make Array,
  swap-at, nested each/var) — so this unblocks the comparator leaf but the sort files need their remaining leaves
  too. A bounded multi-step VM feature; the miscompile fix stops the wrong answer but leaves the defect open — the
  lowering is what closes it.

### COMPARATOR LOWERING — complete sound design (the implementation site + arg-ordering de-risked).
The trailing-fn-value apply is fully designed; the remaining work is a delicate engine-flow change:
- **THE CRUX is ARITY.** `(prev key comp)` consumes exactly 2 args because of the PAREN-GROUP structure; the
  flattened body residual `[prev, key, comp]` has lost it, and comp's runtime arity is unknown statically. So the
  apply MUST be captured at the PAREN-COLLAPSE boundary (engine.go:5603, where the check-mode fn-value-call guard
  already refuses the LEADING-dynamic case `(m.g 3)` "dynamic value precedes args"). The TRAILING case (fn-value
  LAST, args before) is the comparator — currently falls through to the residual refuse/(now-fixed)miscompile.
- **THE OPCODE: OpCallDynTrailTop** (new) — fn on TOP, N args below, apply-or-leave. UNLIKE OpCallDynamicTrailing
  (fn at base, rotates the non-callable residual, 1-arg-bounded), a fn-on-top layout needs NO rotation: the
  non-callable residual [args, fn] is ALREADY the interpreter's trailing order, so it is sound for ANY N. VM:
  `base=len-n-1; fn=stack[top]; args=stack[base:base+n]; if appliable → island.Run([fn]+args) result replaces
  [args,fn]; else leave as-is`.
- **ARG-ORDERING DE-RISKED against real comparator results.** island = [fn] + args-in-stack-order (NO reversal):
  residual [prev,key,comp] → island [comp,prev,key] → comp auto-applies to [prev,key] top-down (b=key, a=prev) →
  body (a cmp b) = (prev cmp key) == interpreter `(prev key comp)`. Verified on `(x 2 comp)`→[1,-1] and
  `(acc x comp)`→[3,-1,1] (bytecode_dynapply_body_test.go expectations).
- **WIRING:** at engine.go:5603, when the paren's LAST recordable value is a Function-typed value and >=1 args
  precede it, record an apply event (a new EmitState.RecordDynApplyTrailing) carrying the args+fn operands and
  arity = count-1; lower it to OpCallDynTrailTop. The Step-1 closure-residual refusal (d3dda735) then catches the
  shapes this path doesn't capture (a non-paren-bounded trailing apply) — containment for a residual open defect,
  not the end state: those shapes are owed their own lowering.
- **REMAINING RISK / SCOPE:** the paren-collapse flow re-encounters the in-paren values after the boundary
  (engine.go:5623 SkipRecorder); inserting the apply recording there without double-recording or breaking the
  residual is the delicate part — a focused engine change, gated by the off-corpus regression + verify-bytecode +
  the sort `--compile`==interpret sweep. Even with this, the sort algorithms chain further (make Array / swap-at /
  nested each-var), so it unblocks the comparator leaf but not necessarily a whole sort file alone.

### LANDED — comparator / trailing-fn-value lowering (commit 1d765fff). The Stage-G leaf is CLEARED.
`(prev key comp)` — a captured/param comparator applied to its preceding args in a fn/closure body — now
COMPILES NATIVELY via OpCallDynTrailTop (fn-on-top, N args, no rotation; args reversed stack→forward so the
fn binds its first param to the TOP arg == the interpreter's paren auto-dispatch). The apply is captured at the
paren-collapse boundary (engine.go RegisterTrailingApply) where the arity is known. Gate: verify-bytecode GREEN
(0 miscompiles); off-corpus regression covers each/fold/scan + asymmetric arg-order + fn-body + 3-arg + a
compiled-closure (lambda) comparator. The arg-order bug was caught mid-build by the asymmetric `(x 2 comp)` case
(compiled [-1 1] vs interp [1 -1]) and fixed. Also fixed a PRE-EXISTING off-corpus MISCOMPILE on the way
(d3dda735, closure body with an unapplied fn-value residual).
NET voxgig file flips: 0 — the sort algorithms chain past the comparator to their other leaves (make Array / set /
nested each-var), each refusing on its own. So the comparator Stage-G leaf is cleared and sound, but a whole sort
file needs its sibling leaves cleared too. Next highest-leverage to actually FLIP sort_smoke: enumerate + clear the
algorithms' remaining leaves (make-Array-from-list provenance, in-place `set`/`swap-at` over an Array, the nested
each-var residual) as a cluster.

### SORT CLUSTER — leaves cleared this pass + the remaining chain (root-caused).
Attacking the sort algorithms to flip sort_smoke (which calls ALL 16+). CLEARED (all sound, gated, committed):
- comparator trailing-fn-value apply (OpCallDynTrailTop, 1d765fff) + dynamic-arg variant (31f02f3c).
- the closure-body unapplied-fn-value MISCOMPILE (d3dda735).
- `make Array [computed]` / a CONSUMED computed list in a fn body (f28cc36a) — the plan's difficulty-5 leaf.
- VERIFIED already compiling: Array `get`/`set`, `arr set j v`.

REMAINING leaves (each its own fix; insertion-sort alone chains through several):
1. **comp-capture (module-fn Function param) — ROOT-CAUSED.** insertion's FIRST leaf: the outer `each` closure
   captures `comp` but resolveOperand declines — instrumented `CAPWALK comp => WORD depth=1 baseline=0`: the
   comparator param of a MODULE fn (Sort.insertion) binds to a bare WORD PLACEHOLDER during CompileCheck's fn-body
   re-entry (baseline=0, as if it were not the fn's own param), so the closure captures an unresolvable Word, not
   the Function carrier. A TOP-LEVEL fn binds `comp` to a proper Function carrier (my repros compile), so this is
   MODULE-FN-SPECIFIC — the module-fn compile-reentry param binding does not install the Function param as a
   resolvable carrier/local. High-leverage (shared by every comparison sort's `[comp:Function …]`), but a deep
   module-fn dispatch fix. NOT a Word-bake (the Word `comp` is the param name; baking it would resolve `comp`
   against the registry at run time → undefined → unsound).
2. **branch-multi-value**: `if (j gt 0) [ <stmt> <stmt> 0 ] [0]` — an `if` arm running several statements nets >1
   value before its result; "branch leaves extra values (Stage 2 lowers single-result branches)".
3. **swap-at / convert List** over an Array (surfaced as "check diagnostics" — likely a re-entry diagnostic).
A sort-file flip needs ALL of these × ALL 16 algorithms (distribution sorts add counting/bucket leaves). The two
HARDEST documented leaves (Stage-G comparator + difficulty-5 make-list) are now cleared; the residual are narrower
but numerous. compile==interpret held throughout (0 miscompiles).

### comp-capture — root-caused to its ARCHITECTURAL BOTTOM (a module-fn compilation limitation).
Pushed the comp-capture leaf to the deepest level via stack-trace instrumentation:
1. `comp` (a `[comp:Function]` param of the MODULE fn Sort.insertion) IS installed correctly as a Function —
   but via InstallFnDef (core_helpers.go:725 `r.Defs.Push(name, NewFnDef(entry))`), which binds a fresh FnDef
   value. (TFnDef conforms to TWord in the lattice, hence the earlier "Word('')" — AsWord on a FnDef is empty.)
2. The closure (the outer `each`) captures `comp`; ComputeCaptures returns that FnDef. resolveOperand needs a
   re-pushable home (event / local slot / inert const). cmp/r-as-comp in a TOP-LEVEL fn is a CONCRETE inert FnDef
   → const-bakes → my repros compile. Sort's comp is an ABSTRACT Function carrier → not bakeable.
3. Attempted fix: a name→slot fallback (`resolveCapturedParam` + emitUnit.localByName) to land the capture on the
   enclosing unit's PARAM SLOT. It CANNOT work — instrumented `CAPFAIL comp unitNames=[]`: the enclosing unit has
   NO named param slots. **Module-fn bodies are NOT compiled as StartFnCompile units with param slots during
   CompileCheck re-entry** — `comp` exists ONLY as a def-stack FnDef binding, with no frame slot for a closure to
   capture. (A top-level fn DOES get param slots, which is why top-level repros compile.) Reverted.
CONCLUSION: clearing comp-capture requires compiling module-fn bodies as proper units with param slots — a large,
high-risk change to module-fn compilation (load-bearing for ALL module dispatch), genuinely multi-session. It also
flips 0 sort files alone (insertion chains further: branch-multi-value, swap-at, convert; × 16 algorithms). The two
HARDEST documented leaves (Stage-G comparator, difficulty-5 make-list) ARE cleared this session; comp-capture is the
next, now root-caused to the architectural bottom for a focused follow-up. compile==interpret held throughout.
