// The re-scoped P7 gate (design/legacy/boru-bytecode-completion.0.ignore §3).
//
// The literal P7 — every spec value row compiles to a fallback-free Program —
// is not reachable for ONE narrow reason: a bytecode compiler is an
// ahead-of-time function `compile : Program -> Instructions`, and a handful of
// words EXECUTE CODE THAT IS COMPUTED AT RUNTIME (`Vm.run` of a runtime string).
// For those there is no static instruction sequence — "compiling" them is
// parse+compile+run at runtime, which is exactly what the OpFallback island
// (the embedded interpreter) does. That, and only that, is irreducible.
//
// EVERYTHING ELSE that declines today is REDUCIBLE — it is declined by a specific,
// nameable limitation of THIS compiler/VM, not by any law. So the partition is
// three tiers, not "meta vs compute":
//
//   - tier 1, interpreter-only (interpreterOnlyWords): executes runtime-computed
//     code. The island is the correct, permanent home. Capped, not ratcheted.
//   - tier 2, reducible-not-yet-compiled (reducibleWords): declined by a named
//     missing compiler/VM feature (macroexpand = compile-time expand + bake;
//     word = splice inline; args.N = frame-local read; flex = reference cells;
//     …). These are TODOs, ratcheted toward 0 like any other work.
//   - the compute frontier: cascades, lowering residuals, DSL bodies, error
//     rows. Ratcheted by computeGapCeiling.
//
// (An earlier version of this file called tier 2 "irreducible meta." That was
// wrong — it laundered unfinished compiler work as impossibility. `args.N`
// proved it concretely: once called "context-dependent, needs an args stack the
// VM frame doesn't keep," it now compiles to a plain PUSH_LOCAL N — the params
// ARE the frame locals. `with-decimal` was the same move. The remaining tier-2
// words are more of the same, just deeper.)
//
// When BOTH ratchets reach 0, only tier 1 falls back, and the unbounded
// tier-1 sub-engine is the only interpreter entry left to narrow
// spans (§3 step 4).
//
// This is the SOLE boundary gate. An earlier parallel gate (meta_fallback_test.go,
// TestMetaFallbackBoundary) curated a permanent-META allowlist and re-walked the
// corpus independently — both the "permanent meta" framing (repudiated here and
// in design §3) and the second walk (the shared census in compiled_census_test.go
// is the one walk) were stale, so it was retired. Its disposition is subsumed:
// total compile failures are ratcheted by the per-file ledger (compile_failures.tsv), and the per-tier breakdown by
// the three ceilings below.
package langspec

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

type allowEntry struct {
	name    string
	pattern *regexp.Regexp
	why     string
}

// interpreterOnlyWords (tier 1) execute code that is constructed at RUNTIME, so
// there is no ahead-of-time instruction sequence to emit: "compiling" them is a
// runtime parse+compile+run, i.e. the interpreter. This is the one genuinely
// irreducible category and the legitimate permanent home of the OpFallback
// island. Kept deliberately tiny — a NEW entry here is a claim of irreducibility
// that must be justified.
var interpreterOnlyWords = []allowEntry{
	{"Vm.run", regexp.MustCompile(`\bVm\.run`),
		"executes runtime-constructed source in a sub-engine — there is no static program to compile; this IS the interpreter"},
}

// reducibleWords (tier 2) decline today because of a specific, NAMED limitation
// of this compiler/VM — not because they cannot be compiled. The `why` records
// exactly what compiling each would take. They are ratcheted toward 0
// (reducibleCeiling); none is a permanent exclusion.
var reducibleWords = []allowEntry{
	{"usurp", regexp.MustCompile(`\busurp\b|/u[rs]?(\b|$)`),
		"REDUCIBLE (mostly COMPILED): usurp is a static arg permutation; its wrapper re-dispatch runs in check mode so the carrier compiler compiles the reversed original call (core_ref.go). Residual: usurp combined with quote/codequote or a non-fn target"},
	{"quote", regexp.MustCompile(`\b(code)?quote\b`),
		"REDUCIBLE: code-as-data; needs the compiler to bake the quoted form as a constant token-list value"},
	{"word", regexp.MustCompile(`\bword\b`),
		"REDUCIBLE (mostly COMPILED): the __SP splice now expands inline at the use site (carrier.go). Residual: a few splices whose inlined body reaches an un-compilable word"},
	{"macroexpand", regexp.MustCompile(`\bmacroexpand\b`),
		"REDUCIBLE (static cases COMPILED): the macro expands at compile time and the token list bakes as a code-as-data const (carrier.go). Residual: expansions with a non-Word/un-bakeable member (parens, type nodes)"},
	{"minilang", regexp.MustCompile(`\bminilang`),
		"REDUCIBLE: registers a sublanguage; static registrations could compile, only runtime-input parsing is interpreter-bound"},
	{"flex", regexp.MustCompile(`\bflex\b`),
		"REDUCIBLE (now COMPILED): the pointer-backed flex node rides as a frame local / closure capture, so compiled mutations hit the SAME cell (corpus-core.tsv:50 landed when core `walk` gained a CallableSpec: the descend hook compiles to a closure with the module-scope flex as a capture, and the empty-flex ascend operand is admitted by provenance — eng/go/callable_words.go emptyFlexHookOperand). Kept for classification: a future flex row declining on a NEW gap would re-surface here"},
	{"canon", regexp.MustCompile(`\bcanon\b`),
		"REDUCIBLE: canonicalisation is a pure function; just unimplemented in the VM"},
	// `args.N` was here ("REDUCIBLE: a frame-local read"); it is now COMPILED —
	// AnalyseFnBody exposes the params as the args projection and
	// tryFoldStaticIndex folds `get N args` to PUSH_LOCAL N (carrier.go). It is
	// the concrete proof that the tier-2 words are reducible, not irreducible.
	{"Test/Assert", regexp.MustCompile(`\b(Test|Assert)\.`),
		"REDUCIBLE: the test/property harness; the candidate bodies compile, the harness accumulates — only random input generation is runtime"},
}

// classify returns the tier a declined/islanded row's source is attributable to:
// "" (compute gap), or the matched tier-1/tier-2 word. Tier 1 is checked first
// so a `Vm.run (canon …)` row counts as interpreter-only, not reducible-canon.
func classify(src string) (tier int, name string) {
	for _, e := range interpreterOnlyWords {
		if e.pattern.MatchString(src) {
			return 1, e.name
		}
	}
	for _, e := range reducibleWords {
		if e.pattern.MatchString(src) {
			return 2, e.name
		}
	}
	return 0, ""
}

// errorRowReason reports whether a compile failure is a deliberately-interpreted
// runtime-error surface: the checker declines so the interpreter raises the
// matching taxonomy (a return-count mismatch, an unpack of a missing key, an
// orphan gen). §3 step 3 dispositions these as allowlisted.
func errorRowReason(reason string) bool {
	return strings.Contains(reason, "suppressed a runtime error") ||
		strings.Contains(reason, "body value count differs") ||
		strings.Contains(reason, "without exactly one declared return")
}

// Three downward ratchets. interpreterOnlyCeiling caps the ONE permanent island
// category (a new tier-1 word is an irreducibility claim that must be argued);
// reducibleCeiling and computeGapCeiling both ratchet toward 0 — when they
// reach it, only tier 1 falls back and the unbounded fallback can be narrowed.
const (
	interpreterOnlyCeiling = 3  // Vm.run / Vm.run-with — execute runtime-computed code. NOW 0: the corpus's last tier-1 row, Vm.run(canon [1 {a:none} x/q]), compiled once OpMakeList let the canon list assemble. Kept at 3 for headroom — a Vm.run of a genuinely RUNTIME string would re-populate it
	reducibleCeiling       = 3  // the REGRESSION ceiling (lanes_test.go; end state 0): 17 -> 3 on 2026-09-19 (S1a: each/fold/scan/filter declare CompileDynBody, and the word-class gaps that were a dynamic callback at one of them poly re-match). Before: 17 on 2026-09-17, every one a row the corpus expansion added — the code-body and higher-order word-class gaps. Falls with the handler migration (FULL-COMPILATION.0.md §10.1 S2). Before the expansion: was 1 — 1 -> 0: corpus-core.tsv:50 (`flex`) COMPILED, the tier-2 P7 floor. Core `walk` gained a CallableSpec (BodyPos 2, BodyOut 0 — visit-only, results discarded; LambdaSharesTokenShape — quotation and lambda hooks both receive the one `{key value path parent depth}` payload carrier), so the descend hook compiles to a closure unit walkClassifyHook drives via InvokeBody; the lambda body's module-scope flex accumulator rides as a closure capture (tryRecordLambdaClosure now applies moduleScopeMutableCaptures, same identity argument as token bodies) so the compiled body mutates the SAME pointer-backed FlexList cell the frame local holds; and the row's trailing `acc` — forward-collected as the 4-arg sig's ASCEND hook — is admitted as a value operand only by the empty-flex provenance proof (eng/go/callable_words.go emptyFlexHookOperand: constructed by the main-registry `flex` native over empty const containers, top-level straight-line, no event recorded since construction — so its classify-time token snapshot is provably a zero-length list on every invocation). Every other ascend shape (non-empty flex, mutated-before-walk, loop-nested, lambda ascend) keeps its compile failure — pinned by lang/go bytecode_findings_test.go TestWalkHookClosureCompiles. PRIOR 3 -> 1: module-test.tsv:48 (`Test.prop`/`Test.run-property`) FLIPPED — Test.check-prop declared CompileRunsBodyIsolated (its gen/property bodies run in isolated CallBoru frames, sound to bake as CALL_NATIVE) and run-property's param retyped `p:Map` -> `p:PropertySpec` (record-schema-typed gets, proven-dynamic operands admitted by dynInputsProven); the whole `_prop_spec` corpus cluster compiles. Only corpus-core.tsv:50 (`flex`) remains tier-2. PRIOR 2 -> 3: the fn-dispatch UNIFICATION (all fns compile their body as a unit via execMatch — design/legacy/MODULE-FN-PARAM-SLOT-COMPILATION.0.ignore §8) moved a second boru:test harness row to fallback. The recursive code-body word `test-describe` (run-spec -> test-describe -> run-spec) hits the recursive-closure-compilation limit the old inline CallBoru path masked via data-driven recursion termination; the rows fall back SOUNDLY (verify-bytecode + crossdiff green). Restoring native compilation is the recursive-code-body-closure follow-up; the ceiling reflects the new reducible debt, NOT a soundness change. PRIOR 1 -> 2: the two genuine WORD-CLASS tier-2 rows are corpus-core.tsv:50 (`flex` — a reference-cell container; its `walk` body is a code-body higher-order word the compiler does not model) and module-test.tsv:48 (`Test.prop`/`Test.run-property` — the test-harness body needs the module-preamble fns to compile as cross-registry closure units). Both rootCause "coverage". The 7 path-modifier.tsv `m.a/u` rows that ALSO surfaced here (checker fix 15b9fb1 cleared their flex/stored-fn-ref false positives, moving them from check-error to declined value rows) are NOT counted as tier-2: they decline on "operand provenance" (rootCause "soundness") — usurp of a MAP-STORED fn, the dynamic-fn-application frontier the compute ceiling already tracks, which usurp's own residual definition excludes ("quote/codequote or a non-fn target"); the census now attributes a soundness-frontier row to the compute frontier even when its source mentions a tier-2 word (compiled_census_test.go). HISTORY: prior comment kept for the audit trail — earlier "back to 1" (the help-eval decouple transiently raised it to 2 then IsolateEmit cleared it): macro.tsv:26 (`def defconst (macro …) defconst answer 42 answer` -> 42) was a check-error pre-decouple, briefly surfaced as a tier-2 "quote" compile failure (the help-eval const-pool contamination made its macro-def dispatch recover), and now COMPILES once the eval is fully hermetic (CheckState.IsolateEmit) — so the lone remaining tier-2 row is again module-test.tsv:38 (Test.run-spec). PRIOR 2 -> 1 when macro.tsv:45 (`def loopy (macro [[a] [quote [loopy unquote a]]]) macroexpand (loopy 1)`, expect ERROR:expansion too deep) was recognised as a correct-error row rather than a reducible-quote TODO: it is a DIVERGENT recursive macro whose non-termination is not statically decidable, so decline-and-let-the-interpreter-raise is the only sound behaviour (design §Stage I). The census now checks the error-row disposition (the spec's authoritative ERROR: marker) BEFORE the tier-2 word bucket, so a correct-error row is never laundered as reducible compiler debt merely because its source mentions `quote`. The lone remaining tier-2 row is module-test.tsv:38 (`Test.run-spec`), whose harness body needs the module-preamble fns (run-spec/run-cases) to compile as closure units — the cross-registry module-body work, not a quick win. 3 -> 2 when the word-splice row (`def vs word [2,3] … 1 add2 vs`, residual [1, 5]) compiled: user-fn-call results now seat to a frame local for an out-of-order residual (lower.go planValueDefLocals/lowerUserCall), so the splice's `add2` result above the literal lays out in order; 4 -> 3 when `def x 5  x/u` (a `/u` usurp of a non-fn binding) compiled to an illegal_ref OpTrap (engine.go stepWordUsurp), leaving the tier-2 usurp bucket for the error-raising path; each remaining entry a named, reducible compiler/VM TODO; 6 -> 4 when codequote compiled (a Quoted ParenExpr bakes as an inert code-as-data const, isInertConst ParenExpr case — the 2 codequote.tsv rows left the quote bucket); 54 -> 52 when structural type-pattern operands started baking (two rows that declined at a reducible word past the now-compiled pattern dropped out); 52 -> 51 when dead value-defs stopped declining (a reducible-word row past a now-dropped dead binding fell out); 51 -> 48 when arity/param/return introspection over a baked fn value compiled (those rows left the reducible bucket); 48 -> 6: re-tightened to the live actual (the ratchet had stalled at 48 while usurp / quote / word / macroexpand / Test-Assert mostly moved to native, draining the bucket); a fixed, small word set with deterministic classification, so kept tight like failureCeiling
	computeGapCeiling      = 58 // the REGRESSION ceiling (lanes_test.go; end state 0): 49 -> 58 on 2026-09-21 — DEBT WRITTEN DOWN, NOT DEBT ADDED, and the only kind of rise this ceiling takes. The fn-locals-scope §6 arm-binding cluster (a `def` inside an `if` arm, read after the `if`) ran the THEN path on all eight of its rows, so a lowering that always picked the then-arm value — or that lost an incoming binding through an empty else — would have retired every one of them and still miscompiled the opposite condition: the cluster proved the wrong thing, confidently. Nine paired FALSE-PATH witnesses are now interleaved (fn-locals-scope.tsv L168, L170, L172, L174, L176, L177, L179, L181, L183; the nested row L175 gets two — outer-else and inner-else). Each was MEASURED before it was written down: the interpreter answers as the row states, and the compiler declines exactly as the row's THEN twin does (`fn f: body result of unknown provenance`, or `operand of unknown provenance … at add`/`at mul` for the two operand spellings). Three of the nine (L172, L174, L181) take the EMPTY arm, the direction a then-arm-always lowering gets wrong the other way round: the join must carry the binding that arrived from before the branch, not synthesise one from the arm that did not run. The bug was always this big; the corpus merely could not see it. The matching per-file move is fn-locals-scope.tsv 10 -> 19 in compile_failures.tsv, whose third column carries the same reasoning. Falls back by nine, and takes the eight originals with it, when the arm-binding JOIN lands (design/SESSION-HANDOVER.0.md, "Where the join seats"). Before: 56 -> 49 on 2026-09-19, S1b-2 (a computed fn value def-bound at the top level resolves at a forward slot and at a `/v` read: the collection seat and stepWordVal consult the fn-carrier side table, so `each f/v xs` over a factory's result dispatches instead of declining "unmatched dispatch recovered") — the seven rows it compiles are callbacks.tsv:L82/L154, each-variants.tsv:L203, fold-map-filter.tsv:L73/L227/L229 and module-composition.tsv:L95 — every one a real-compute row, so the fall is exactly seven. Before: 104 -> 56 on 2026-09-19 (S1a: the fn-value and code-body callbacks at each/fold/scan/filter that the ambiguity gate declined now poly re-match; the 56 left are the same family at the words that do not declare the flag — for-each, walk — and the provenance rows). Before: 107 on 2026-09-17, the corpus expansion's fn-value and provenance compile failures (COMPILABLE-SUBSET.md §5). 107 -> 104 (2026-09-18, NUR153 closed — one residual rule at every seam): three real-compute rows stop declining once a stored `=>` value's residual follows the tape rule wherever it is applied, which is S1's first slice landing early. Attributed by measurement across three heads, not inference: the same gate reports 107 at e04fa21, 104 at 3e15778 and 104 at 5618223, so the fall is NUR153's and the quoted-class commit moved none of it. Falls further with §10.1's S1 and S5. History: ratcheted 17 -> 0 (2026-07-13): the live compute-gap actual is 0 — every remaining compile failure (9 rows) is a knownCompileFailures dispatch-recovery ERROR row (compiled_failures_test.go), and the census dispositions an error row BEFORE the tier partition (compiled_census_test.go), so the compute frontier is empty. The repl served-handler tier and the unnamed-fn-value-param tier recorded in the merge below have both since compiled (the I5 CompiledFn.Reg and I1 unnamed-param frame-flow waves, design/legacy/P7-ENDGAME.10.ignore Addendum 2). PRIOR — was 17 — merge: main's 12 (repl served-handler tier) + 5 module-fnvalue-boundary value rows on the unnamed-fn-value-param tier (compiled_coverage_test.go gate comment; design/legacy/ARG-SEMANTICS-UNIFICATION.0.ignore — callers with an UNNAMED Function param decline and fall back faithfully). PRIOR — was 9 — 9 -> 12: the boru:repl served-handler tier (module-repl 12/16/17/18) became a NAMED compute-frontier compile failure: a foreign-registry (module-preamble) fn whose body CONSTRUCTS a fn value declines to unit-compile (bodyConstructsFn, eng/go/core_helpers.go), because the compiled unit executes against the DISPATCHING registry and the escaped lambda loses module scope (boru:repl's served handler could no longer resolve repl-eval-line — an actual divergence the differential caught once the rows started compiling). Previously rows 16/17/18 hid in the ERROR-ROW disposition ("body value count differs" — repl-close declared [Any] over a 0-value body) and row 12 COMPILED-BUT-DIVERGED undetected (the harness hung on a racy Net.accept spec row before reaching it). Fixing repl-close's declaration surfaced the real frontier; these 4 rows now decline and be interpreted and errorRows drops to 3 (the permanent error allowlist). Compiling them is the cross-registry-unit follow-up (run a foreign fn's unit against fnDef.Registry), the same family as the M6 dynamic-scope tier. PRIOR — 17 -> 9: Stage M2c shaped-instance-method dispatch (design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §6 M2c; eng/go/method_shape.go). The 8 stalled fn-value rows compiled to NATIVE: module-log.tsv:53,54,56,77,78,79,80 (statement-position `l.info "req"` / `c.add 1` / `g.record 21` — the fn-value-call boundary 7) + module-log.tsv:55 (`def c (l.child {y:2})`, the paren-bounded 1; logger-child gained the loggerShapeReturns check shape so the derived child's own method reads resolve too). Mechanism: the accessor ReturnsFn NOTES the resolved delegation-wrapper member against the read's dynamic carrier (CheckState.MethodShapes — the member itself never bakes, the freeze-gate: the runtime instance carries per-call state); when the compile pass steps the annotated carrier at the pointer — exactly where the interpreter auto-dispatches the concrete member — tryShapedMethodDispatch re-runs the ENGINE'S OWN matcher over the same tape and commits only for a pure-forward, contiguous, inert/evaluation-fixed statement window, modelling the dispatch through carrierResults over the member's inner signature and recording a guarded mid-stream OpCallDynMethod (fn operand = the dot-read EVENT, i.e. the runtime value; args = interned consts; spec claims arity + declared result count). The VM applies the runtime method (delegation fast path / island — the proven boundary machinery) and DEFERS via internal_error → interpreter re-run whenever the shape claim fails (non-callable value, result-count mismatch) — the sound escape RunCompiled already owns (runtimeShouldFallback). The 4 container-auto-dispatch rows (module-log:72,73 + module-rand:14,15) stayed DECLINED at the time on the miscompile-E guard [SINCE SUPERSEDED: the arity-0 landing model was built — shapedMethodApplyWindow's all-0-arg path for shaped members, tryMemberFnArrivalDispatch's empty-window claim for pinpointed plain members (the break-2 closure) — and the read guards now skip what the landing models own, re-declining via their guard-owned declines; all four rows compile]. Remainder decomposition: container auto-dispatch 4 (sound guard, since closed by the runtime auto-dispatch model), dispatch recovery 2 (flex:88,95 — G5 path-shape typing), operand provenance 2 (recursion:71,72 — the M6 dynamic-scope tier), island 1. PRIOR — 29 -> 17: Stage M3 + M4 (design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §6). M3 DSL registration (12 rows): module-parse.tsv:14,15,18,37-44 — the `parse <kind>` macro's deferred-kind branch (a Parse.register kind whose grammar is built at run time) now RECORDS a CALL_NATIVE of the new parselang-deferred-dispatch word instead of returning an unrecorded dynamic carrier; at run time the op resolves parse_<kind> against the LIVE export map (populated by the compiled Parse.register) and replays the parse macro's own expansion tail, or raises the byte-identical parse_unknown_lang when a runtime-conditional registration did not execute — the check pass installs NOTHING into the export map (the reverted register-ReturnsFn leak stays closed, pinned by TestDeferredParseCheckObservationFree). module-parse:15 (dynamic input at dot) cleared as a bonus: the parse result now has provenance, so the dot's poly recovery records. module-minilang.tsv:320 — the MiniLang export map gains a growth LEDGER (eng/go/module_export_growth.go): every program-reachable runtime grower (register, register-compiled) has a check twin noting the keys it may install, so a PROVABLY-stable missing-key dot folds to None (`MiniLang.Gen` after registering a NON-filter kind — no member type is ever minted) while noted keys keep the sound fold decline (TestMiniLangAbsenceFoldCompiles pins both directions). M4 definite traps (9 ERROR rows, error allowlist 12 -> 3, not counted here): apply:37,38, open-words:32,83,84,90,100, generics-sugar:37, generics:60 — see the trap batteries. Remainder decomposition: fn-value-call boundary 7 + paren-bounded 1 (the M2c shaped-instance-method stall), container auto-dispatch 4 (the sound miscompile-E guard), dispatch recovery 2 (flex:88,95 — G5 path-shape typing), operand provenance 2 (recursion:71,72 — the M6 dynamic-scope tier), island 1. PRIOR — 56 -> 29: Stage M2 (the fn-value frontier, design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §6 Stage M2): M2a `apply` over a param/captured fn (recursion.tsv:90-92 — the elided apply dispatch registers a PENDING unit apply; the body-tail window lowers OpCallDynApplyTop, applyHandler's unquote-then-apply; any other pending shape declines); M2b path-modifier map-stored fns (path-modifier.tsv:17-20,23-25,28,52-55 — the modifier words' gradual dispatch records OpCallNativePoly so usurp/stack-args/forward-args/force-arity wrap the REAL fn at run time, and the trailing fn-on-top window `10 3 m.s/s` islands verbatim via OpCallDynamicMixed so a BarrierPos-0 wrapper stack-collects exactly as the interpreter); M2c partial (module-log.tsv:62,83 — Log.register declares CompileStoresFn, its pure-literal sink fn bakes as a const); M2d fn-value-as-operand (module-minilang.tsv:306-315 — `is` marks its VALUE slot FnInertArgs, positional because a Function in its TYPE slot is a predicate RunPredicate INVOKES and keeps declining; corpus-core.tsv:134 — walk's ascend-slot LAMBDA compiles to its own closure unit; plus the `/v` dispatch-mod marker now survives the check pass's carrier strip, so inline `(lambda)/v` parks as at runtime). 26 rows native + log-register's error row to the compiled trap side; STALLED remainder reported in the M2 log: module-log 53-56,77-80 need the shaped instance-method dispatch model, module-log 72,73 + module-rand 14,15 stay on the sound miscompile-E auto-dispatch guard. PRIOR — 66 -> 56: Stage M1 (merged-word seam, design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §6): buildFnBodyReturnsFn now shares the check state caller→owner at the ReturnsFn boundary itself (shareCheckStateFrom), so a TRANSPLANTED word-extension sig (open words — a module-defined `add` merged into the importer's bare word) unit-compiles into the MAIN program exactly like every other user-fn dispatch; the entire "user fn call (Stage 3)" bucket (open-words.tsv:72,77,78,82,85,86,87,98,99 + micron.tsv:200) compiled to native CALL_USER units, 10 -> 0, with every other bucket byte-identical (no migration). PRIOR — 86 -> 66: typed-def store-with-reparent (OpBindTyped) compiled the "dynamic refinement reparent/validate is interpreter-only" AND "DepScalar predicate validation is interpreter-only" buckets to zero (RecordTypedBind / RunTypedBind — the interpreter's own RunPredicate / Unify-vs-builtin-ancestor / DepScalar Unify re-run over the runtime value, byte-identical errors, ReparentValue where defTypedHandler reparents; miscompile B's compile failure closed as a compiled bind). Of the 12 open-words.tsv rows in the bucket, 3 compiled outright and 9 progressed to their NEXT frontier (the open-words merged-word dispatch: 6 "user fn call", 3 "dispatch recovery"), and the ratchet re-tightens to the live actual per the failureCeiling convention. PRIOR: operand-provenance cascades, code-body DSL words, Stage-1 lowering residuals, dynamic in/out, 5 islands, user-fn dispatch (+8 from merged PR #142 minilang/parselang corpus rows that fall back faithfully; +1 patrun.tsv — a mutable Patrun dispatch / `add` 3-arg-overload row falls back, the bytecode fix is tracked on a separate branch); 166 -> 137: structural type-pattern operands (`{a:Integer}`, `[Integer String]`, `[Resource Entity]`) now bake as const members, so static is/typeof/size over them compiles; 137 -> 126: generic schema bakes as a const, unblocking make/is/typeof/residual over top-level generic types; 126 -> 122: schema body check admits a type-parameter Word, so typed-list-field generic schemas (`{items:[:T]}`) bake too; 122 -> 119: a function-signature value bakes as pure descriptor data, unblocking fnsig residual/typeof and fnsig-bodied generic schemas; 119 -> 100: a surface type bakes as a const (shared canonical descriptor; the compiled path keeps check-pass mints so it never goes stale), unblocking the surface type-algebra cluster; 100 -> 97: typed-def object construction records its skipped make event, giving the bound instance make-equivalent provenance; 97 -> 90: a single-result value-def referenced zero times drops its dead result (OpDrop) rather than declining as an unconsumed call result; 90 -> 85: a no-capture fn value bakes as a const so it stands as data (residual, map member, introspection operand), with an auto-dispatch-boundary guard keeping a fn-ahead-of-args residual on the fallback path; 85 -> 86: +1 arithmetic.tsv — `add`'s String-concat overloads (`[String Scalar]`/`[Scalar String]`) gained a second Scalar→String coercion row (Boolean and Float operands, both positions) when the old `[Scalar Scalar]` overload was tightened to require a String; the coercion concat still falls back faithfully
)

func TestOnlyMetaFallsBack(t *testing.T) {
	t.Parallel()
	c := gatherCensus(t)
	interp, reducible, errorRows, computeGap := c.interp, c.reducible, c.errorRows, c.computeGap
	tier1By, tier2By, computeByReason := c.tier1By, c.tier2By, c.computeBy

	t.Logf("re-scoped P7 partition: %d interpreter-only (tier 1, permanent), %d reducible (tier 2, TODO), %d error-row, %d COMPUTE GAP",
		interp, reducible, errorRows, computeGap)
	t.Logf("tier 1 — interpreter-only (ceiling %d):", interpreterOnlyCeiling)
	for _, w := range sortedKeys(tier1By) {
		t.Logf("  interp %4d  %s", tier1By[w], w)
	}
	t.Logf("tier 2 — reducible, not yet compiled (ceiling %d):", reducibleCeiling)
	for _, w := range sortedKeys(tier2By) {
		t.Logf("  reduce %4d  %s", tier2By[w], w)
	}
	for _, row := range c.computeRows {
		t.Logf("  gap row: %s", row)
	}
	t.Logf("compute gaps by reason (ceiling %d):", computeGapCeiling)
	for _, r := range sortedKeys(computeByReason) {
		t.Logf("  gap    %4d  %s", computeByReason[r], r)
	}

	gate(t, "interpreter-only rows", interp, 0, interpreterOnlyCeiling, false,
		"rows a word claims are irreducible — justify or compile")
	gate(t, "reducible (tier-2) rows", reducible, 0, reducibleCeiling, false,
		"rows that fail to compile because of a word-class gap the compiler does not model")
	gate(t, "compute gaps", computeGap, 0, computeGapCeiling, false,
		"real-compute rows that fail to compile")
}

// sortedKeys returns a map's keys sorted by descending value then name, for a
// stable most-frequent-first histogram.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
