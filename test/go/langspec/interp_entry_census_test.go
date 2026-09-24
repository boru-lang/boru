// interp_entry_census_test.go measures the island the OpFallback ceiling
// cannot see.
//
// `TestCompiledCoverage` reports 0 islanded, and that number is real but
// narrow: it counts programs whose DISASSEMBLY embeds an OpFallback span. It
// cannot see a `CallBoru` made INSIDE a native handler, because no opcode
// records one — and that is where interpretation actually survives. Every
// predicate dispatch takes `InvokeCallback:callboru` today with no ledger row
// and no island flag (design/FULL-COMPILATION.0.md §2.3, §6.3).
//
// So "0 islanded" is a claim about the metric, not about the runtime. This
// census is the claim about the runtime: RUN each corpus row compiled with the
// InterpEntry hook armed — Stage 1 built it for exactly this — and count the
// entries that are UNATTRIBUTED, i.e. interpreter execution the end-state
// invariant does not permit. Check-mode entries are excluded: the compiler
// front end running RunInCheckMode words is attributed by construction.
//
// T2 ("No islands") is not satisfiable while this number is non-zero, whatever
// the OpFallback ceiling says. The ceiling below is a DOWNWARD ratchet, like
// compileFailureSiteCeiling: it only falls.
package langspec

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// interpEntryRowCeiling is the number of corpus rows that, when RUN compiled,
// re-enter the interpreter through an unattributed seam.
//
// First measured 2026-08-28 at 184 of 7180 rows that run compiled (959
// entries). Now 100, after nine changes. The seam spread — the
// shape of the debt, not a second ceiling — and what each moved:
//
//	                         first    (a)    (b)    (c)    (d)    (e)    (f)    (g)    (h)    (i)
//	Engine.Run                 501    477    453    443    433    439    425    400    391    383
//	CallBoru                   275    251    251    251    251    251    238    238    238    238
//	vm:island                   66     66     48     48     39     39     39     39     39     39
//	runPooledSub                37     37     35     35     35     37     36     11     11     11
//	RunResolved                 31     31     31     31     31     31     31     31     31     31
//	vm:island-resolved          21     21     21     11     10     10     10     10     10     10
//	InvokeCallback:callboru     28      4      4      4      4      4      6      6      6      6
//	                          ----   ----   ----   ----   ----   ----   ----   ----   ----   ----
//	rows                       184    163    151    141    131    131    130    114    107    100
//
// (a) Foreign detached units became hostable mid-run
// (eng/go/vm_foreign_unit.go). The InvokeCallback column is that one: those 24
// entries were predicate bodies that HAD compiled to units and ran on the
// interpreter anyway, because the mid-run nested path declined every ref whose
// Program was not the running one — which a detached ref never is.
//
// (b) The Apply kernel (eng/go/vm_dyn_apply.go + compiler stampFnConst): a
// fn-value CONST carries its compiled unit, and a dynamic apply ENTERS that
// unit as a frame instead of islanding the callee. The `vm:island` column is
// that one, and it is the column the OpFallback ceiling can never see — those
// programs disassemble with `fallbacks=0` and the island lives inside the
// dynamic-apply opcode.
//
// (c) The Apply kernel reached CALL_DYN_FRAME's replay window in its simplest
// shape — an empty resolved prefix and a token region that is a fn followed by
// plain data. That is the `vm:island-resolved` column. A non-empty prefix keeps
// the island by contract: the fn stack-collects from the prefix as well as
// forward-collecting the region, so the frame would bind a different arg set
// than the interpreter assembles.
//
// (d) stampFnConst descends into list and map CONSTS, so a fn read out of a
// container carries its unit too. Measured before it did: of the island rows
// the top-level stamp left, roughly four in five were exactly that shape —
// `def m {f: (fn …)}  m.f 5`, `def ops {f: inc/v}  ops.f 5`, a class field
// method.
//
// (e) Emit-state isolation for the stamp — the one column that goes UP, and it
// should be read as the price of a correctness fix rather than a regression.
// stampFnConst now declines a stamp that would register a dynamic-scope name in
// the enclosing program, so a handful of bodies that used to carry a unit take
// the apply island again: Engine.Run +6, runPooledSub +2, no row moved. The
// alternative was keeping a stamp that MASKED a miscompile (see
// design/FULL-COMPILATION.0.md, "Container descent, and the mask it nearly
// shipped"). A seam count is not a score.
//
// (f) The seven native-callback sites that used CallBoruFn — `filter`'s
// Function form, the map-lambda each/fold bodies, core walk / StructUtil.walk,
// IO.mount's fileops handlers, boru:parse's matcher and action callbacks, and
// the fn-util words — now call InvokeCallbackFn, so a stamped body runs on the
// VM instead of never being offered to it. CallBoruFn is deleted: it existed to
// keep those bodies OFF the VM, which is the fence full compilation removes.
//
// (g) The Apply kernel reached LENSES (core/go/reach_unit.go). A Reach is a
// callable value exactly as a fn value is, and every consumer — `apply`, the
// lens forms of each/filter/sortby, getpath/setpath — funnels through one
// primitive, ApplyReach, which ran the lowered `[recv dot key …]` chain on a
// pooled sub-engine per application. It now compiles that chain once, caching
// the unit on the Reach payload, and runs it on the VM. This is the column the
// per-file attribution picked out: 16 rows, one mechanism, found by looking at
// the table rather than by guessing.
//
// Every drop carried Engine.Run with it, because an island is a nested Run.
//
// WHAT THE CallBoru COLUMN ACTUALLY IS, measured by probe at (f) — because it
// had not moved through four fixes and read as the largest block of debt:
//
//	Test.property's generator/property calls   224
//	core_helpers foreign-registry fn dispatch    8
//	InvokeCallback's own interpreter fallback    6
//
// So it is not broad interpretation debt. It is two or three `module-test.tsv`
// rows amplified by an iteration count — Test.property runs its bodies ~100
// times per row, and the seam counts INVOCATIONS while the ceiling counts ROWS.
// Those bodies are raw QUOTATIONS, not fn values: the module already routes a
// compiled sig through InvokeCallback and only falls back when the property was
// written as `[body]` tokens, which carry no unit to offer. Compiling them is
// Stage 7 (runtime compilation everywhere), not a seam flip, and it is worth
// perhaps three rows. Read the seam counts as a shape, never as a priority
// order: a big number here can be one row in a loop.
//
// WHERE THE ROWS ARE, BY MECHANISM. The table used to be by FILE, and the
// ledger's own (j) says why that was the wrong axis: a file table ranks
// CONCENTRATIONS, and a mechanism spread two-and-three-at-a-time across several
// files is invisible in it (Assert.throws was five rows in four files). This
// one is by mechanism, swept row-by-row with the source printed, and it adds up
// to the ceiling exactly. Rewrite it, do not append to it.
//
//	18  A CALLABLE VALUE READ OUT OF A CONTAINER, then applied     vm:island
//	    path-modifier 13, usurp 2, fn-value 2, class 1
//	11  `do` OVER A BODY WHOSE RESIDUAL COUNT IS NOT STATIC        RunResolved
//	    control 4, bytecode-migrated 6, word-splice 1
//	 8  Test.invoke / Test.prop / Test.check-prop                  CallBoru
//	    module-test 5, corpus-modules 3
//	 4  A FN VALUE CROSSING A MODULE BOUNDARY, applied inside      vm:island-resolved
//	    module-fnvalue-boundary 4 — but only ONE is the boundary's doing.
//	    Instrumenting dynApplyEnter's ref gate splits them: one declines
//	    purely because ref.Prog is not the running program (a DELIBERATE
//	    decline — vm_dyn_apply.go says a detached ref cannot be a frame of
//	    this program and hosting it nested would reintroduce the body
//	    bracket the file exists to avoid); one ALSO declines correctly at
//	    MatchFnSig (a 1-param fn with 0 args); and two carry no ref at all
//	    because storedSigEligible declines a flow-sentinel body (`break` /
//	    `continue`). Read a cluster's rows before costing its fix.
//	 4  SERVER / MOUNT CALLBACKS (a live socket, a fileops map)    InvokeCallback:callboru
//	    module-repl 3, module-io 1
//	 4  MISC, one mechanism each                                   Engine.Run
//	    canon 2 (Vm.run over canon output), module-rand 1, fn-value 1
//	    (corpus-modules' Rand.list-of row left at (u))
//
// Engine.Run appears in nearly every row because every other seam runs a nested
// engine; a row with ONLY Engine.Run is the interesting case — nothing
// islanded, the whole body simply interpreted.
//
// reach.tsv led the old table at 14 rows, every one through runPooledSub, and
// it is gone: that is column (g).
//
// THE 18-ROW CLUSTER IS ONE SHAPE WITH FOUR CALLEES, and only the callee
// decides whether it compiles today:
//
//   - a BORU FN in a container (`def m {f:(fn …)}  m.f 5`) — COMPILES, column
//     (k), fourteen rows;
//   - a NATIVE in a container (`def m {a:add/v}  m.a 1 2`) — islands: a native
//     carries no unit for the Apply kernel to ENTER. Measured, and its price
//     recorded in the rejected increment above: one row, bought with a worse
//     error message, because the dyn-apply opcodes are lowered with
//     SrcPos.Row == 0 and a handler raising through the direct path loses its
//     caret. Give those opcodes positions in the lowerer and the trade changes;
//     that is the prerequisite, not the increment.
//   - a MODIFIER WRAPPER over either (`m.a/u`, `usurp (m.s)`) — islands for a
//     different reason, and it is not fixable at the apply site at all: usurp /
//     stack-args / forward-args / force-arity RETURN TOKENS for the engine to
//     re-step. There is nothing for a frame to enter. These need the modifier
//     words themselves to lower to a dispatch (Stage 5), not a wider apply.
//   - a CLASS FIELD METHOD (`def C class {op:(fn …)}  c.op 5`) — one row, the
//     boru-fn case reached through a class instance rather than a map.
//
// So "path-modifier.tsv, 13 rows" was never one fix. It is the native half and
// the token-rewrite half of a cluster whose boru half already left.
//
// (h) boru:debug's body runs are ATTRIBUTED, not compiled — and the distinction
// is the point, because it is the one place "compile everything" is the wrong
// goal. Every one of those sites installs a TRACE HOOK and the word's ANSWER is
// what the hook saw: Debug.steps returns the engine-step count, the profiler
// returns a tally of observed dispatches, the stepper pauses at each step and
// breakpoint. Compiling those bodies would not speed them up — it would empty
// the tally, change the count, and leave the debugger nothing to step through.
//
// So this is interpretation the end state must PERMIT. It gets the same C4
// attribution `module-load` already has ("debug-observe"), which is what this
// census means by attributed: interpretation that is specified, named, and
// therefore not debt. It is NOT an escape hatch — the test is whether COMPILING
// the body would change the word's answer. For a step counter it plainly does;
// for `filter`'s callback it plainly does not, which is why that one compiled.
//
// A number this census cannot reach by compiling alone is worth knowing early:
// some of the remaining rows are of this kind, and the honest end state is
// "every entry attributed", not "no entries".
//
// (i) `Log.with-span NAME [body]` compiles its body. It DECLARES the body slot
// callable (a CallableSpec with 0 inputs and BodyOutResidual, the shape
// Test.describe already uses) and the handler drives the compiled closure
// through InvokeBody, falling back to the token run when the body did not
// compile. All 7 module-log rows, and the file left the cluster table.
//
// It is the same question (h) answers the other way: nothing about a span
// depends on WHICH engine runs its body, so compiling changes no answer. The
// two increments together are the rule — attribute where the engine IS the
// answer, compile where it is not.
//
// AN INCREMENT THAT WAS BUILT, MEASURED AND REJECTED — recorded because the
// measurement is worth more than the code was. Dispatching a callee whose
// matched sig carries its OWN Go handler (a native fn value read out of a
// container, `def m {a:add/v}  m.a 1 2`) directly instead of islanding it:
//
//  1. It first looked like TWELVE rows, 114 -> 102, and was FLATTERING ITSELF.
//     It also took the dispatch-modifier wrappers, and those handlers do not
//     COMPUTE a result — usurp / stack-args / forward-args / force-arity return
//     TOKENS for the engine to re-step (execMatch re-steps a handler's result by
//     default; only Park() opts out). Pushed onto the operand stack as data they
//     tripped screenResults, and ELEVEN corpus rows went from
//     compiled-with-an-island to not compiled at all. A row that stops compiling
//     leaves this census's DENOMINATOR, so the count fell for the worst possible
//     reason. An A/B walk comparing wasCompiled per row found them.
//
//  2. Handing the rewrite's tokens to the island instead fixed that, with zero
//     regressions — and the honest gain was ONE row.
//
//  3. That one row cost error fidelity. A handler that raises through the direct
//     path loses its source position ("source position unknown" where the
//     interpreter points a caret), because the dyn-apply opcodes are lowered
//     with SrcPos.Row == 0 and stampAt has nothing to stamp. The island never
//     needed it: its sub-engine re-ran the tokens and carried their positions.
//     Closing that means giving those opcodes positions in the lowerer, which
//     ripples through error rendering corpus-wide, where content parity is
//     gated.
//
// One row, bought with a worse error message, is not the trade this mission
// makes — the same judgement that un-masked the flex-map miscompile at (e).
//
// (j) `Assert.throws [body]` compiles its body — the same CallableSpec + drive-
// through-InvokeBody shape as (i), at BodyPos 0 with the residual DISCARDED.
// Every corpus row that runs an assertion body left the census: five of them,
// two in module-test.tsv and three in the edge-errors files, which is three
// more than the cluster table showed. The table only lists files at 5+ rows, so
// a word used two-or-three-at-a-time across several files is INVISIBLE in it.
// Worth remembering when reading the table as a work queue: it ranks
// concentrations, not mechanisms, and a mechanism can be spread thin.
//
// It is worth naming why a word whose whole purpose is to OBSERVE AN ERROR is a
// compile and not an attribution, since (h) attributes on exactly that kind of
// reasoning. The distinction is what the word observes. boru:debug observes the
// ENGINE — steps taken, dispatches seen — so the engine is the answer and
// compiling erases it. Assert.throws observes the PROGRAM: whether the body
// raised. A raise is a raise on either engine (the VM traps and returns the same
// boru error the sub-engine run would have), so the answer is engine-independent
// and the body compiles.
//
// (k) The Apply kernel now takes the SHAPED-METHOD apply (OpCallDynMethod) —
// the `m.f 5` shape where a dot-access reads a fn value out of a container and
// applies it. That is the cluster (h) went after and could not close, and this
// is the part of it that was never about the modifier: fourteen rows, all of
// fn-value.tsv and valof.tsv plus one of bytecode-migrated's, 95 -> 81.
//
// callDynMethod had every other apply path — a compiled closure, a
// trivial-delegation native — and fell through to the island for a plain boru
// fn value, even though its unit was compiled and sitting in the same program.
// It now asks dynApplyEnter first, exactly as callDynamic and callDynFrame do.
//
// WHAT MADE IT UNSOUND UNTIL NOW, and this is the increment's real content. A
// stamped fn-value unit compiles COUNT-AGNOSTIC — compileStoredFnUnit goes
// through compileClosureBody with bodyOut 0, whose `declared = nil` leaves
// CompiledFn.Returns empty — so its RET enforces nothing. Entering such a unit
// therefore SKIPPED the fn's declared return contract, which the island path
// applies (the interpreter's __RC runs inside the CallBoru the nested Run
// reaches). Measured on the EXISTING kernel, before this increment touched
// anything:
//
//	def bad fn [[n:Any][Integer][n]] end
//	def mk  fn [[][Function][bad/v]] end
//	((mk) 'str')
//	  interpreted  [boru/type_error] bad: return value 1: expected Integer, …
//	  compiled     'str'
//
// A silent wrong answer on main, not a compile failure — found by asking what the
// entry would skip, not by a failing test. The fix is that the entry CARRIES
// the applied value's contract (dynEnter.retFn, applyRetContract) and the
// frame's RET applies it; lang/spec/fn-value.tsv §12 pins both halves. The
// census rows are downstream of that: they are only takeable BECAUSE the entry
// became contract-faithful.
//
// The shape claim's result half is discharged statically against that carried
// contract (dynMethodClaimOK) — a frame push has no results to count, and the
// RET now guarantees the count the sig declares.
//
// (l) boru:parselang's runtime `parse <fn>` dispatch stops stepping its
// expansion tail in a sub-engine. That tail — the parser value followed by
// `source opts end` — was a whole interpreter run inside a compiled program,
// one per `parse <parser> …` row: all eleven of module-parse.tsv and four more
// elsewhere. 81 -> 66.
//
// It replaces one lane with two, and the question that picks between them is
// not "is this a fn value" but "WHAT KIND of fn value is this":
//
//   - a matched overload carrying a GO handler dispatches directly, the way the
//     interpreter's execFnDefLiteral wrapper branch does (this mirrors the VM's
//     own tryNativeFnApply arm for arm);
//   - a real boru body goes through the callback seam, where it is offered to
//     its compiled unit before CallBoru.
//
// Getting that question wrong is what an earlier attempt did, and the two ways
// it goes wrong are worth keeping, because they look nothing alike:
//
//  1. `Parse.parser` mints a TRIVIAL-DELEGATION wrapper — unnamed params, a body
//     of one Word naming the inner Go native. Through the callback seam,
//     CallBoru splices that body and re-dispatches its word over a frame whose
//     unnamed args sit stack-order, so the inner native's sig positions come out
//     REVERSED and nothing matches:
//     `signature_error: cannot call parse-parser-1 — no signature matches`.
//     Loud, and it had been recorded as an ARG-ORDER mistake. It is not one:
//     [source, opts] is the sig order on every lane, and permuting it would
//     have made this row pass and every other one wrong.
//
//  2. `def myp (Parse.parser g)` REBINDS that wrapper, and InstallDef's
//     module-wrapper branch binds the inner native's Signatures verbatim under
//     the new name — so the value carries GO sigs outright, and the callback
//     seam ran them as though they had a body. SILENT:
//
//     def acc (flex []) … Parse.matcher g lex 5 ([s:String] => [acc push {v:s} …])
//     interpreted  [{v:'hello'}]
//     compiled     []                  the matcher never ran
//
//     Caught by lang/go's TestCompileParseOverEnclosingParserDef, which is a
//     reminder that the census is not the gate — it cannot see a wrong answer,
//     only an interpreter entry, and this change LOWERED it while breaking a
//     program.
//
// The lesson generalises past this word. A fn VALUE is not one kind of thing,
// and a seam that "runs a fn value" has to ask which kind before it picks a
// mechanism. Two plausible-looking argument orders is the symptom of having
// skipped that question — the order was never the variable.
//
// (m) The whole-frame replay window (OpCallDynFrame) admits the Apply kernel
// over a NON-EMPTY resolved prefix, when the callee is all-forward. Two rows,
// 66 -> 64 — small, and worth recording for where the boundary sits rather than
// for the count.
//
// The prefix is the frame-bottom unnamed-param re-push; the token region is
// what the interpreter's pointer would step. The window declined any prefix at
// all because a BARRIER'd callee stack-collects from it as well as
// forward-collecting the tokens, so a frame push would bind a different arg set.
// That is true of a barrier'd callee and only of one: an all-forward callee
// whose params the token args exactly fill cannot reach the prefix, so the
// prefix survives underneath and the unit's result lands on top of it — which
// is precisely the residual the island returns.
//
// What did NOT move says more than what did. `def looper fn [[Function]
// [Integer] [def acc 0 for 5 [(args.0 1) …] acc]]` still islands, because the
// apply sits inside a LOOP body and the window is not the frame's; and `def
// keep fn [[Function] [Function] [args.0]]` still islands because it does not
// apply its argument at all. Neither is a barrier question, so neither is
// reachable from here.
//
// (n) `Debug.trace` and `IO.trace` are ATTRIBUTED, on column (h)'s rule and by
// the same reading of it. Both route through core.RunTrace, whose entire job is
// to print what the interpreter did step by step; compiling the body would not
// speed it up, it would leave nothing to print. Two rows, 64 -> 62, and the
// attribution goes on RunTrace itself so the two words cannot drift apart.
//
// (h) attributed boru:debug's step counter, profiler and stepper and MISSED
// these two, which is worth noting as a property of that kind of fix: an
// attribution applied word-by-word leaves siblings behind. Putting it on the
// shared entry point is what makes it exhaustive.
//
// (o) `do {key:[body]}` stops starting an engine to step values it has already
// computed. Six bytecode-migrated rows, 62 -> 56, and the interesting part is
// that NOTHING needed compiling — the compiler was already doing the work.
//
// The disassembly settles it. `def f fn [[a:Integer] [Map] [ do {n:[a add 1]} ]]`
// lowers to
//
//	PUSH_LOCAL l0 / PUSH_CONST 1 / CALL_NATIVE add / MAKE_LIST / MAKE_MAP / CALL_NATIVE do
//
// so the map reaching DoEvalMapValue is `{n:[6]}` — the addition happened at
// MAKE_LIST. The handler then ran `[6]` in a sub-engine to discover that
// stepping the literal 6 yields 6. That is an interpreter entry inside a
// compiled program buying precisely nothing, and doEvalDataList now returns a
// STEPLESS list as its own residual instead.
//
// The interpreter's lane is untouched, which is what makes this safe rather
// than clever: the same source arrives there as [Word(a) Word(add) 1], which is
// not stepless, so it still runs. The two engines' answers are unchanged; only
// the compiled lane's redundant engine is gone.
//
// Worth generalising from: a census row is not automatically a COMPILER
// problem. The cluster table said "do {map} computed-map bodies" and read like
// Stage 6 work on code bodies. Reading the actual disassembly said the bodies
// were already gone, and the fix was six lines in a handler. Read the
// bytecode before believing a cluster's name.
//
// (p) `ArrayUtil.foldaxis` DECLARES its body callable. Two rows, 56 -> 54, and
// no handler change at all: foldaxisHandler already reduces each lane through
// the same doFold that `fold` uses, and doFold already drives the body through
// InvokeBody. The word was seam-ready and simply never said so.
//
// The spec is fold's, with the inputs taken one rank deeper — a lane is a row
// (or a transposed column), so each step sees two elements of an INNER list,
// never a row. `rank2ElemCarrier` joins EVERY row's elements for that (its
// first-row predecessor typed a mixed-row body from row 0 alone and baked the
// wrong overload — module-array.tsv's mixed-row rows, 2026-09-02), and answers
// the gradual Any for a data argument with no element to take a type from.
// That arm is the `foldaxis 0 [add] []` corpus row, which is also the shape
// the handler short-circuits to `[]`.
//
// Its sibling `eachrank` is NOT here, and the reason is the one that decides
// whether a word is a declaration away or a change away: eachrankHandler slices
// its body into raw TOKENS and walks them itself (eachrankWalk), so it has no
// InvokeBody seam to declare over. Handler first, declaration second — never
// the reverse.
//
// (q) The mixed-apply island (CALL_DYNAMIC_MIXED) skips a STEPLESS window, on
// the same predicate column (o) gave `do {key:[body]}` — now shared, in core.
// One row, 54 -> 53, and the row count understates it: four more rows lost
// their island while keeping other seams, so the vm:island total falls further
// than the ceiling does.
//
// `1 2 3 do [7] error [drop 9] add 1` is the shape. Both bodies compile to
// closures, the arithmetic runs native, and the window the mixed apply islands
// is [1 2 3 8] — four literals. The island exists because the COMPILER could
// not rule out a callable value interior to the window; when the runtime values
// turn out to be plain data, the interpreter places every one of them and hands
// the window straight back.
//
// The predicate moved to core.IsSteplessWindow rather than being copied,
// because eng cannot import basic and a second copy of a rule this sharp is how
// two engines drift. It stays an ALLOWLIST at its new home for the reason it
// was one at its old: an unrecognised shape must be treated as active, or a
// token kind added later silently becomes data and a body stops running.
//
// (r) `ArrayUtil.eachrank` follows foldaxis, and needed the handler change
// foldaxis did not. 53 -> 52.
//
// eachrankWalk sliced the body into raw TOKENS and ran them itself; it now
// threads the body VALUE down and calls InvokeBody at the leaf, which is the
// same seam every other code-body word uses. The declaration alone would have
// done nothing — column (p) named that ordering and this is the case that
// proves it: handler first, declaration second.
//
// Its input carrier is GRADUAL on purpose. The cell type is RANK-dependent —
// `eachrank 0` sees each scalar leaf, `eachrank 1` each innermost list —
// so a precise carrier would mean walking the data's spine by (depth - rank),
// and the corpus now pins both ranks compiling clean against the gradual one.
// Precision nothing needs is a proof obligation nobody asked for.
//
// (s) An EMPTY quotation body stops starting an engine. 52 -> 51, and the
// third instance of the same shape after (o) and (q): a run whose residual is
// its own input.
//
// The row that found it is worth reading, because the row itself is a
// surprise. `walk {mode: "depth"} {a:1 b:[2 3]} (m:Any => [...]) acc` looks
// like a walk followed by a trailing `acc`; walk's four-argument overload
// FORWARD-COLLECTS that `acc` into its optional ASCEND slot, so an empty flex
// list becomes a hook, and the traversal ran it on an engine once per visited
// node. Nothing was wrong with the answer — an empty hook does nothing on
// either engine — but each node paid for a sub-engine to find that out.
//
// So the guard is not really about walk. runQuotationBody is shared, and no
// caller of it needs an engine to discover that running nothing returns what
// it was given.
//
// (t) A stamped unit that RAN and DEFERRED is a bail, not an island. 51 -> 50,
// one row, and the row is the whole point: the census was counting one defer
// twice.
//
// `5 $.name apply` (reach.tsv §7, an ERROR row) stamps its lens, enters the
// unit, and CALL_NATIVE_POLY finds no `dot` for an Integer receiver. The VM
// does exactly what it is designed to do there — vmDefer records the bail and
// returns internal_error so the interpreter can raise the canonical
// signature_error — and ApplyReach then ran the chain unattributed. So the same
// event appeared once in the bail census as the designed defer it is, and again
// here as an island. RunCompiled's top-level arm has named this category since
// C4 ("fallback:runtime-bail"); the nested compiled lanes had not.
//
// What makes it measurable rather than a judgement call is that the SEAM now
// says which decline it was. InvokeCompiled collapsed two outcomes into
// ran=false — a unit that could not be hosted, and a unit that ran and deferred
// — and threw away the internal error that told them apart. It returns that
// error alongside ran=false instead, so bailReplayAttribution attributes the
// replay only when the unit actually ran. A lane that never reached the VM
// declines with a nil error and stays visible as the island it is.
//
// A bail COUNT on the registry was built first and is worth recording as the
// rejected option: it read the ledger's own tally and compared it across the
// attempt. It works, but the tally is SHARED with concurrent forks (the hook
// holder is a pointer, deliberately), so one lane's defer could attribute
// another lane's island — over-attribution, the direction that makes a
// no-unattributed-entry assertion pass vacuously. The error is per-call and
// cannot leak across lanes.
//
// Both compiled lanes take it — ApplyReach and InvokeCallback — because the
// hole is the seam's, not the lens's. This is column (n)'s lesson again: an
// attribution applied at one call site leaves its siblings behind.
//
// A SECOND INCREMENT BUILT, MEASURED AND REVERTED — the 11-row `do` cluster,
// recorded because it rules out the cheap reading of that cluster.
//
// Those rows compile TODAY and island: the closure path probe-compiles the body
// fine and then declines at closureResidualExact (callable_words.go), because a
// BodyOutResidual dispatch seats exactly len(outs) results and a unit whose
// runtime count could differ would mis-seat the simulated stack. `do` then falls
// to tryRecordDynBody, which bakes the body as CONST TOKENS and marks the result
// VARIADIC — and the handler runs those tokens on a sub-engine. That sub-engine
// is the RunResolved entry the census counts.
//
// The obvious move is therefore to keep the compiled unit and borrow the
// backstop's variadic mark: same dispatch, closure operand instead of const
// tokens, result marked variadic so only variadic-absorbing positions consume
// it. It is four lines. It makes things WORSE:
//
//	do [for 3 [1]]
//	  before  compiled, seams Engine.Run+RunResolved, answer 1 1 1
//	  after   compile_failed: "residual shape beyond Stage 1 (call results reordered)"
//
// A row that stops compiling LEAVES THE DENOMINATOR, so the census would have
// fallen for the worst possible reason — the same trap the native-callee
// increment fell into above, caught this time by A/B probing one row before
// writing a test. The two lowerings are not interchangeable at the residual
// planner: the backstop lowers to a plain CALL_NATIVE whose body is a const,
// while the closure form pushes an OpPushClosure operand, and the planner then
// sees the call's results reordered against the residual.
//
// So the cluster is not a gate to relax. Seating a region whose size is known
// only at run time is the Stage 5 machinery (production-order regions and
// generalized marks) by definition, and no amount of marking at the record site
// substitutes for it.
//
// (u) `Rand.list-of` serves a STEPLESS generator body without an engine per
// iteration. 50 -> 49, and it is the FOURTH instance of the shape (o), (q) and
// (s) each found: a run whose residual is its own input.
//
// `Rand.list-of ['a','b'] 2` ran a sub-engine twice to discover that stepping
// two string literals yields two string literals, then took the top of each.
// The handler already had a compiled lane (IsCompiledClosure -> InvokeBody);
// what it lacked was the observation that its INTERPRETER lane sometimes has
// nothing to interpret.
//
// The boundary rows are the increment, not the happy path. n=0 runs the body
// ZERO times, so an empty body is only an error when n > 0 — and the first cut
// guarded on the body alone, which made `Rand.list-of [] 0` raise where both
// engines answer []. Caught by probing the boundary before writing the rows,
// and all four spellings are now corpus rows (module-rand.tsv): stepless-with-n,
// stepless-with-zero, empty-with-zero, empty-with-n.
//
// Worth noting what this says about the shape's reach: it has now appeared in
// basic's `do`, eng's mixed apply, native's shared quotation body, and a MODULE
// handler. It is not a property of any one word, it is a property of running
// placed values, and the next site that starts an engine over a literal window
// is the next instance.
//
// TWO LESSONS. A census whose denominator is "rows that ran compiled" REWARDS a
// change that stops rows compiling: read it against the corpus gate and the
// compile-or-fallback walk, never alone. And the remaining path-modifier rows
// do not need this seam at all — see the cluster note below.
//
// (u) 49 -> 46. A PARKED NATIVE WORD applies on the VM — but only an
// UNWRAPPED one, and the gap between those two sentences is the whole entry.
//
// The apply gate asked `IsDelegationFnDef`: is this a trivial wrapper whose
// every own sig is a `[Word(inner)]` pass-through? That names a SHAPE, and the
// property it stood in for is `no boru body to run in a frame`. A bare `add/v`
// has that property outright — 9 own sigs, every one a Go handler — and
// answered `no`, because it delegates to nobody: it IS the native. So the gate
// now names the property (vmNativeApplicable, at all four apply sites), and
// tryNativeFnApply on the other side already handled the value.
//
// FIRST MEASUREMENT SAID 10 ROWS. IT WAS WRONG, and how it was wrong is the
// part worth keeping. The gate admitted MODIFIER WRAPPERS too — `usurp`,
// `force-arity`, `/u`, `/N` — and those are a different function under the
// same Name: UsurpFunction REVERSES each sig's Params and hands the result a
// Go handler that re-dispatches the original, expecting the engine's
// collection around it. tryNativeFnApply resolves sigs by NAME, so it fetched
// the unwrapped original and dropped the reversal:
//
//	def m {s:sub/v}  m.s/u 10 3
//	  interpreted  7        (usurped: 10 - 3)
//	  compiled    -7
//
// Three path-modifier.tsv rows, caught by TestSpecCompiledOrFallback. The
// count 39 in this slot was measured WHILE those rows were wrong; excluding
// them gives back most of the win, and 46 is the real number.
//
// THE FIX IS A MARKER, NOT A COMPARISON, and that too was measured. The first
// exclusion compared the value's params against the registry's — which cannot
// see the swap, because `sub(Number, Number)` reversed is itself. Every sig
// `sub` has is homogeneous, so the comparison admitted exactly the rows it
// existed to reject. FnDefInfo.ArgsReversed records the reversal at the point
// it happens, and the composing wrappers propagate it: without that,
// `force-arity 2 (usurp (m.s))` rebuilt the value and dropped the flag — one
// more differential row.
//
// WHAT STAYS ISLANDED is therefore every reshaped value, for the same reason a
// user fn does: its handler expects a dispatch it is not being given. That is
// the honest boundary of this increment, not an oversight to close later.
//
// THE GATE ALSO NEEDS THE LIVE NAME, not just the parked value, and the two
// can drift: `def size fn [[n:Pos] …]` EXTENDS a core word (the one rebinding
// `extend_owner` permits), adding a boru overload to a name a value was parked
// from earlier. RegisteredWordIsNative asks that second question, and such a
// value islands exactly as a user fn does.
//
// That half is pinned by a HANDLER TEST (core/go, TestParkedNativeApplyGate),
// not a corpus row, and the reason is worth recording. The corpus row I first
// wrote for it tripped TestCheckTypeSoundness: the checker predicts
// `[dynamic(Any) Pos]` where the program actually leaves `[Integer]` — a
// static/dynamic mismatch on the parked-value-plus-extension shape, unrelated
// to this change and not something to fold into it. The row is gone; the
// observation is in NUR.md so Stage 8 has it.
//
// (v) 46 -> 43. THE `/s` FAMILY, and it was one mechanism, not three rows.
//
// The three path-modifier.tsv rows column (u) left behind — `10 3 m.s/s`, its
// paren twin, and `stack-args (force-arity 2 (m.s))` — did not need any of
// that column's gates widened further. They lower to a DIFFERENT opcode.
// `CALL_DYNAMIC` is what column (u) taught to apply a parked native;
// force-stack lowers to `CALL_DYNAMIC_MIXED`, whose handler islands the whole
// window because the compiler could not rule out a callable interior to it.
// One line of disassembly said so, which is the same lesson column (o) already
// recorded and the one I keep having to relearn: read the bytecode.
//
// The window is `[10 3 fn]` — inert data under a single TRAILING fn — and that
// is not a general re-step, it is the trailing apply. The island runs
// `Run([10 3 fn])`: it places the two literals, then steps the fn, which
// collects them TOP-DOWN. That is exactly callDynTrailTop's binding, so the
// same window handed to tryNativeFnApply in that order answers identically
// with no sub-engine.
//
// Every condition on it is load-bearing. IsSteplessWindow over the PREFIX is
// what rules out a second callable inside the window — the very thing the op
// exists for. The fn must pass vmNativeApplicable, for the three reasons
// column (u) settled. And a decline (no overload takes exactly this many args)
// falls through to the island, which places the leftovers as the interpreter
// does rather than guessing.
//
// Lower it whenever it falls. Raising it means a change put interpretation
// back into compiled programs, which is the one thing the compilation mission
// rules out — so a rise wants a design note, not a bigger number.
//
// (w) 36 -> 33. THE STAMP REACHES TWO MORE KINDS OF CONTAINER — and the
// correction matters more than the count, because the previous entry in this
// ledger asserted the five remaining `vm:island` rows were "one cause, and it
// is not the dispatch site". They are FOUR causes. Probing dynApplyEnter's
// decline arms one row at a time, rather than reading the seam name, splits
// them:
//
//	class.tsv:L122     no-ref        the fn is a class FIELD DEFAULT
//	usurp.tsv:L33,L45  no-ref        the fn is behind a MODIFIER WRAPPER
//	fn-value.tsv:L19   never reached lowers to CALL_DYNAMIC_MIXED
//	fn-value.tsv:L28   no-sig-match  a DELIBERATE decline (String vs Integer)
//
// A shared symptom is not a shared cause. `no-ref` was three rows and two
// mechanisms; the other two rows do not go through that gate at all.
//
// The two this column fixes are both the same SHAPE of hole — stampFnConst
// walks list and map consts, and neither of these is a list or a map:
//
//   - A MODIFIER WRAPPER is a container of exactly one fn. `usurp` rebuilds the
//     value with a GO handler on every own sig, so storedSigEligible declines
//     them all and the walk stamps nothing; the boru body is one level down,
//     behind FnDefInfo.Wraps. `def sub2 fn […]  def ops {rev: (usurp sub2)}`
//     disassembled to `fns=0` — the map const folded the wrapper in and NOTHING
//     in the program compiled sub2's body, because sub2 is never called by name
//     either. So no widening of the apply site could have taken these rows:
//     there was no unit to enter. The walk follows Wraps, and eng's
//     UnwrapModifierChain (column (u)'s marker) hands dynApplyEnter the value
//     the walk stamped.
//
//   - A CLASS TYPE carries its methods as FIELD DEFAULTS, and the type operand
//     returns from resolveOperand BEFORE the const chokepoint's stamp. The node
//     is BARE there (IsBareTypeNode means Data == nil), so the schema has to be
//     resolved the way `make` resolves it — ResolveTypeLiteralDef against the
//     live registry — which is why this is its own entry point rather than
//     another case in the walk. The PARENT chain is walked with it: a subclass
//     inherits its parent's defaults rather than copying them, so stopping at
//     the node would leave every inherited method islanded.
//
// WHAT IS LEFT IN THE COLUMN IS TWO ROWS, AND NEITHER IS A STAMP QUESTION —
// both fn-value.tsv rows now compile their body (`fns=1`) and island anyway:
//
//   - `3 m.f 2` lowers to CALL_DYNAMIC_MIXED with the fn INTERIOR to the window
//     [3 fn 2]. Column (q) takes a stepless window and column (v) takes a
//     stepless prefix under a TRAILING fn; this is neither, it is the SPLIT
//     RULE — one forward token fills param 0, the stack supplies param 1, and
//     203 rather than 302 is the whole content of that sentence. Deriving that
//     binding at the apply site is re-deriving the collection kernel, which is
//     Stage 5's production-order regions by definition; column (l) is the
//     standing warning about guessing an argument order instead.
//
//   - `m.f 'x'` declines at MatchFnSig, CORRECTLY — a String does not fill an
//     Integer param — and the island's answer is the unapplied residual
//     `fn (Integer) x`. Taking it means reproducing the interpreter's park rule
//     for an unmatched callee without an engine, which is a different question
//     from every other row here and worth one row at most.
//
// 33 -> 32 (2026-09-10, the forty-fourth increment): `for-each` gained the
// CallableSpec it never had, so its body compiles to a per-element closure
// instead of riding as a List const the handler interprets. One corpus row
// (fn-value.tsv's module-scope fn as a for-each body word) stops entering the
// interpreter. The ratchet only falls, and this is the fall.
//
// 32 -> 29 (2026-09-11, the fifty-seventh increment): a whole-residual
// dispatch (`do`) takes a body whose residual is COUNT-AGNOSTIC — [inert…,
// REGION], or one region event's whole run — instead of declining it to the
// dyn-body strategy. The three rows are the variadic-branch bodies in
// control.tsv (`def b true  do [1 2 (if b [] [9 9])]` and its siblings): they
// compiled before and they compile now, but the body ran on the interpreter
// through InvokeBody, which is exactly the entry this census exists to count
// and the OpFallback ceiling cannot see.
//
// Read the two numbers together. `TestCompiledCoverage` did not move, because
// nothing about these rows' COMPILE status changed; only which engine ran
// their bodies did. That is the whole argument for keeping this census beside
// the ceiling rather than folding it in.
//
// 29 -> 28 (2026-09-11, the fifty-ninth increment): the same dispatch takes
// the MIRROR of the shape above — a region run FIRST, with nothing but INERT
// operands above it. `do [for 3 [1] 7]` is the row (control.tsv), and it had
// been reading as the same compile failure as `do [7 for 3 [1]]` for as long as one
// sentence described both.
//
// The two halves are not symmetric and the cheap one was the one being
// declined. A fixed value BENEATH a runtime-variable run must be SEATED under
// it, which is what OpSeatBelowMark and the mark plan cost; a fixed value
// ABOVE the run is PUSHED after it and lands on top of however many values
// the run really left, so nothing has to know the length. What the screen
// needed was one predicate widened from "the run reaches the END" to "no
// EVENT above the run" (eventRunThenInert) — no new op, no new plan.
//
// It also graduated a compile failure the census cannot see: a NO-CONTRACT fn whose
// body leaves the same shape (`def f fn [[] [] [for 3 [1] 7]]  f`) declined
// outright, and the pin that held it wrote the wrong reason out in full.
//
// 28 -> 27 (2026-09-16, the seventy-first increment): a stored handler's
// bare read of a module-scope value is seated live at its token
// (NoteLiveRead's stored-dep arm), so the const stamp of a mount handler
// reading a flex map no longer grows dynScopeNames and is TAKEN instead of
// declined. The row is module-io.tsv's `def files (flex {})  IO.mount
// {read: (p:Pathon => [files get `${p}`]) …}`: both handlers stamp, and
// the row's reads and writes run on the VM where they went through
// CallBoru before. The stamp's decline keeps its pin on a nested lambda's
// read (TestStampConstDynScopeDeclineKeepsEnclosingCompile).
// 54 -> 52 on 2026-09-18 (NUR153's ruling, the tape rule everywhere): a
// deferring anonymous `=>` callback no longer enters the interpreter through
// the CallBoru seam to evaluate its residual in the live frame — the sweep
// happens after the frame teardown instead, so two each-variants rows stop
// entering.
const interpEntryRowCeiling = 53 // the REGRESSION ceiling (lanes_test.go; end state 0; fails in BOTH directions): 57 -> 53 on 2026-09-24 (the foreign-home fn value at the apply seam): four rows LEAVE and none enter — callbacks.tsv:L146 (`M.run M.inc 5`, one export passed into another as its callback: Engine.Run 1 + vm:island-resolved 1), module-composition.tsv:L100 (`5 (m 'f' get) apply`, an export fetched by get and applied: CallBoru 1 + Engine.Run 1 + vm:island-resolved 1), module-fnvalue-boundary.tsv:L51 (`apply1 A.pub 5`, a named Function param beside a main-program `secret`: the same three) and module-composition.tsv:L75 (`each ([k:String] => [((M.tbl k get) 4)]) ['inc' 'ten']`, a callback lambda applying a module fn fetched from an exported map, once per element: CallBoru 2 + Engine.Run 2 + vm:island 2) — the applied value's matched overload carries a DETACHED compiled unit (the module fn's own stamp, its Program not the running one), which dynApplyEnter, entering only an in-program unit as a frame, declined to the island; the VM's dynamic-apply sites now host such a unit nested (eng dynApplyForeign, runForeignUnit's discipline), where the module fn's free words resolve at the module. Measured row by row with BORU_LOG_CENSUS_ROWS=1 against the previous head 017ea83 (57): Engine.Run 344 -> 335, CallBoru 239 -> 235, vm:island-resolved 10 -> 7, vm:island 6 -> 4, RunResolved 67, runPooledSub 10, InvokeCallback:callboru 5 (see engineEntryCeiling). Before: 62 -> 57 on 2026-09-24 (the fn-util wrapper at the token seam, and the recorder's registry after a module body): five rows LEAVE and none enter — callbacks.tsv:L154 (`def h (FnUtil.compose inc/v dbl/v) end each h/v [1 2 3]`: the self-contained Go-implemented wrapper applies natively at the body seam, where it was stepped once per element — Engine.Run 3 + RunResolved 3), module-composition.tsv:L94 (`def a5 (M.mk 5) end each [a5] [1 2 3]` over an inline module's factory: the recorder's registry follows the running engine again after the module's body, so the def's shape claim lands on the program registry where the read looks and the body compiles into its unit — Engine.Run 3 + RunResolved 3), and three `$module` reads on an imported native module that the same restore compiles where they fell to the interpreter after the module's body — edge-modules-1.tsv:L60 (`import "boru:test" Test.$module is Assert.$module`: Engine.Run 12 + runPooledSub 8), compare-restrict.tsv:L133 (its `eq` twin: Engine.Run 12 + runPooledSub 8) and module-vault-tui.tsv:L18 (`import "boru:vault-tui" convert List VaultTui.$module`: Engine.Run 1). Measured row by row with BORU_LOG_CENSUS_ROWS=1 against the previous head be610d7 (62), and per file with BORU_SPEC_FILES over the five files on both heads (each file's entries were its one census row's): Engine.Run 375 -> 344, RunResolved 73 -> 67, runPooledSub 26 -> 10, CallBoru 239, nothing else moved (see engineEntryCeiling). Before: 68 -> 62 on 2026-09-24 (the def-bound computed fn read at a closure body's tail): six rows LEAVE and none enter — callbacks.tsv:L83 (`def a5 (mkadd 5)  each [a5] [1 2 3]`), L84 (`fold [add] (each [c] [1 2 3]) 0`, the closure under fold's collect), L155 (`each [h] [1 2 3]`, FnUtil.compose's wrapper), L156 (`each [p] [1 2 3]`, FnUtil.partial's), L158 (`each [d] [1 2]`, FnUtil.memoize's) and each-variants.tsv:L204 (`each [(f 1)] [1 2 3]`, the placed CALL of a def-bound closure, admitted because the closure body's probe fork now carries the top-level computed fn value-defs) — a def-bound computed fn read as the whole of a closure body over the element is the interpreter's word dispatch, lowered as the body-tail trailing apply over the read's live lookup (the check pass standing aside for the short window inside a closure unit), so the body compiles into its unit where the native used to run the raw list through RunResolved once per element. Measured row by row with BORU_LOG_CENSUS_ROWS=1 against the previous head 2ffd07f (68): Engine.Run 392 -> 375, CallBoru 239, RunResolved 90 -> 73 (see engineEntryCeiling). Before: 75 -> 68 on 2026-09-23 (the placed value inside a code body): seven rows LEAVE and none enter — fold-map-filter.tsv:L86 (`0 fold [([a:Integer e:Integer] => [a add e])] [1 2 3]`), L89 (`each [([n:Integer] => [n mul 2])] [1 2 3]`), L240 and L241 (the placed lambda with a capture and over a map accumulator), callbacks.tsv:L75 (`size (each [(mk 2)] [1 2 3])`, a factory's returned closure placed in the body) and bytecode-migrated.tsv:L113/L114 (`do [(f 5) 2] error [dot code]`, a user call's result placed in a do-catch body) — a value a user paren places inside a code body is data on both lanes (the park rule), so the closure body compiles into its unit where the native used to run the raw list through RunResolved once per element. Measured row by row with BORU_LOG_CENSUS_ROWS=1 against the previous head 1ca30b1 (75): Engine.Run 408 -> 392, CallBoru 240 -> 239, RunResolved 105 -> 90 (see engineEntryCeiling). Before: 74 -> 75 on 2026-09-23 (the recovery's window, NUR180): generics-fn.tsv:L55 (`def sumvals gen [T] fn [[bs:[:T]] [Integer] [0 fold [dot value add] bs]] … sumvals xs`) ENTERS as a graduated row — it did not compile at all before (the check pass stopped at a FALSE `undefined word: value`: the recovery's stack-first window took the fold body's two inputs for `dot` and left the written key to be stepped as a word), and now checks clean and compiles; its fold body over a generic element runs as a raw token body at the callback seam (Engine.Run + RunResolved, once per row), the closure probe declining `dot` over `T` — that body's native compile is the row's next cut. Before: the REGRESSION ceiling (lanes_test.go; end state 0; fails in BOTH directions): 77 -> 74 on 2026-09-22 — the quotation-body container reads: NONE of the three rows the increment compiles enters (each-variants.tsv:L205 nets the placed fn as data, fold-map-filter.tsv:L215 applies its const member VM-native through the frame replay's lone-token entry, module-composition.tsv:L98 its module fn through the home's callback seam), and three rows LEAVE — callbacks.tsv:L103 (`each [useit inc/v] [1 2 3]`), callbacks.tsv:L55 (`each ([k:String] => [((cbs k get) 10)]) ['a' 'b']`) and module-composition.tsv:L76 (the same shape over a module map) — because a closure body's PROBE compile no longer leaves an orphaned stamp on a capture-free fn value's sig (compiler undoProbeStamps): the value's unit now carries a Program and the fn-value seams run it VM-native where they fell to the stepping island. Measured row by row with BORU_LOG_CENSUS_ROWS=1 against a worktree at 1ae0a21 (77): three rows left, none entered; Engine.Run 415 -> 406, CallBoru 242 -> 240, vm:island 13 -> 6, InvokeCallback:callboru 7 -> 5 (see engineEntryCeiling). 77 was: 78 -> 77 on 2026-09-22 — the curried chain: callbacks.tsv:L85 (`def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]]  each ([k:Integer] => [((mk k) 100)]) [1 2 3]`) leaves the census — the chain inside the callback lambda's body records its re-stepped produced lead's apply at the collapse (core's parenProducedLeadApplyIdx) and the callback runs as a closure unit, where its body used to fall to the dyn-body CALL_NATIVE whose handler ran it through RunResolved once per element. Measured row by row with BORU_LOG_CENSUS_ROWS=1 against a worktree at 917ecf0 (78): one row left, none entered; the row's three elements were Engine.Run 3 + RunResolved 3, which is engineEntryCeiling's 418 -> 415. 78 was: 77 -> 78 on 2026-09-22 — the dynamic-lead group: module-composition.tsv:L100 (`import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end def m {f: M.inc/v} end 5 (m 'f' get) apply`) FAILED TO COMPILE before ("apply over a dynamic lead") and now compiles as the program-level apply EVENT, whose op applies the fetched MODULE export through the re-step island (Engine.Run 1, CallBoru 1, vm:island-resolved 1): a foreign-home boru fn carries no CompiledRef into this program, so dynApplyEnter declines it — the module-fn seam, the same family as the module-test.tsv rows, not this increment's. One row in, none out (BORU_LOG_CENSUS_ROWS=1 against the 77-row head); the four callbacks rows the same change compiles (L40, L41, L50, L51) enter nothing. A compile failure becoming a compiled row with one attributed seam is the S1a trade in miniature. 77 was: 80 -> 77 on 2026-09-22 — S1b's apply shapes: DynApplyLeadEligible admits a native closure unit with a named frame, so a callback lambda applying its own Function-typed param — `each ([f:Function] => [(f 5)]) [inc/v dbl/v]` — compiles as a closure unit instead of falling to the dyn-body CALL_NATIVE whose handler ran the callback through RunResolved. Measured row by row with BORU_LOG_CENSUS_ROWS=1 against the previous head (80): three rows left, none entered — callbacks.tsv:L54 (`each ([f:Function] => [(f 10)]) reg.handlers`), callbacks.tsv:L105 (`each ([f:Function] => [(f 5)]) [inc/v dbl/v]`) and fold-map-filter.tsv:L200 (`0 fold [add] (each ([f:Function] => [(f 10)]) fs)`), each formerly Engine.Run 1, RunResolved 1. 80 was: 80 on 2026-09-21 — NUR175's two 0-RETURN witnesses, fn-value.tsv:L319/L320 (`def h fn [[] [] []] end … 5 m.f` and its `get` twin). A member applied for its EFFECT returns nothing, which the re-step landing's one-result claim cannot express, so the landing stands aside and the row takes today's residual apply — an island, a seam this census already carries in bulk. Two rows in, none out, measured with BORU_LOG_CENSUS_ROWS=1 against b39be40. NUR174's anonymous-0-arg park removes two interpreter ENTRIES in the same change (bytecode-migrated.tsv:L285, callbacks.tsv:L150 — see engineEntryCeiling) without removing their ROWS, which still enter by other seams, so this ratchet sees only the rise. 78 was: the REGRESSION ceiling (lanes_test.go; end state 0; fails in BOTH directions): 78 on 2026-09-19 — S1b's SECOND increment (a computed fn value resolves at a forward slot). Measured row by row against the S1b-1 head (77): ONE row entered and none left — callbacks.tsv:L154, `import "boru:fn-util" … def h (FnUtil.compose inc/v dbl/v) end each h/v [1 2 3]`, which FAILED TO COMPILE before and now compiles, its fn-util wrapper body still stepped per element (Engine.Run, RunResolved). The other six rows the increment compiles enter nothing at all. A compile failure becoming a compiled row with one attributed seam is the S1a trade in miniature; the wrapper is on S1b's owed list. 77 was: 77 on 2026-09-19 — S1b's first increment, the fn-VALUE seam made native (eng/go/vm_fnvalue_seam.go + the lazy detached stamp, compiler.LazyStampFnSig): a fn value handed to a higher-order word's list arm runs its unit on the VM — stamped at first application, memoised on the value — instead of being stepped on a pooled sub-engine, and a value reaching InvokeCallbackFn (the map arm, filter, walk) stamps lazily and takes the VM path it already had. Measured row by row against the S1a head (102): twenty-five rows left, none entered — fold-map-filter.tsv ×11 (L75, L210, L212–L214, L216–L217, L221, L223–L224, L228), each-variants.tsv ×7 (L193–L198, L201), callbacks.tsv ×6 (L57–L58, L104, L126–L128), module-composition.tsv ×1 (L101) — every one a fn-value callback S1a had released onto the RunResolved seam. What remains, by family: the 21 code-bodies.tsv rows and their kin are TOKEN bodies read at run time (a quoted list from a flex, a fn result, a List param — S3's runtime compilation, not a fn value); callbacks.tsv ×12 and fold-map-filter.tsv ×9 are fn values the seam does not yet reach (a fn result applied inside a token body `[(mk 2)]`, fn-util wrappers, the fn-value islands); module-test.tsv ×5 the boru:test quotation bodies; the rest the round trips and repl rows. Before: 102 on 2026-09-19 — S1a of design/FULL-COMPILATION-REPLAN.0.md, the gradual-Any overload commitment: each/fold/scan/filter declare CompileDynBody, so the corpus rows that DECLINED at the ambiguous-overload gate now compile — as a dyn-body CALL_NATIVE whose handler runs the callback through the RunResolved seam. That is the G-lane-first landing the re-plan names: the rows run compiled and enter the interpreter once each (Engine.Run 1, RunResolved 1 — 76 of the 102 rows are RunResolved entries); S1b retires them by lowering the re-matched overload's body natively. Measured, not inferred: BORU_LOG_CENSUS_ROWS=1 on origin/main (52 rows) against this tree (102) — fifty-one rows entered and one left. Entered, by file: fold-map-filter.tsv ×17 (L75, L86, L89, L210, L212–L214, L216–L217, L221, L223–L224, L228, L239–L242 — class-field and map-field callbacks, fn values in lists, Function params, a fn-local step), code-bodies.tsv ×14 (L122–L124, L138–L140, L143–L145, L150–L151, L153–L154, L156 — quoted code bodies read from a flex, a fn result, a branch, a List or Map param), callbacks.tsv ×10 (L57–L58, L83–L84, L126–L128, L155–L156, L158), each-variants.tsv ×8 (L193–L198, L201, L204), module-composition.tsv ×2 (L94, L101). Left: fold-map-filter.tsv L168 (its fold now compiles natively). 52 was: 54 on 2026-09-17 — 27 before the corpus expansion, which added 27 rows in three clusters: fn-value callbacks (callbacks.tsv ×8, fold-map-filter ×4, each-variants ×3, module-composition ×2 — vm:island), raw-token code bodies (code-bodies.tsv ×7 — RunResolved) and the boru:test quotation bodies (module-test.tsv ×5 — CallBoru); the full row list is one BORU_LOG_CENSUS_ROWS=1 run away. History: 54 (2026-09-17, the corpus expansion) -> 52 (2026-09-18, NUR153 closed: one residual rule at every seam. A stored `=>` value applied through a native seam used to have its residual evaluated in the live frame, which is a CallBoru entry the census counts; ResidualEvalsInFrame gives the tape rule everywhere and CallBoruNamed sweeps the deferred residual after teardown, so two callback rows stop re-entering. The rows are each-variants.tsv callback shapes; BORU_LOG_CENSUS_ROWS=1 names them) -> 0 (Stage 9)

func TestInterpEntryCensus(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	seams := map[string]int{}
	perFile := map[string]int{}
	// fileSeamRows counts ROWS, not entries: file → seam → how many of that
	// file's dirty rows touch that seam at all. Entry counts mislead (see the
	// CallBoru note above — 224 of them were one word in a loop), and the
	// ceiling is a row ratchet, so the attribution that picks the next fix has
	// to be in the same unit the ceiling is.
	fileSeamRows := map[string]map[string]int{}
	rows, dirty, ran := 0, 0, 0
	// censusRows holds the BORU_LOG_CENSUS_ROWS lines until the walk is done:
	// workers see rows in no promised order, and a listing a reader diffs
	// between two runs has to read the same every time, so it is sorted into
	// file-then-line order before it is printed.
	type censusRow struct {
		file string
		line int
		text string
	}
	var censusRows []censusRow

	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		mu.Lock()
		rows++
		mu.Unlock()
		n, seen, ok := runWithEntryHook(t, r.Input)
		if !ok {
			return // declined or check-error: the interpreter owns it by design
		}
		// BORU_LOG_CENSUS_ROWS=1 names every dirty row and the seams it
		// entered through. The file × seam table above ranks CLUSTERS,
		// which is what you want when choosing a mechanism — but once a
		// cluster is chosen you need the rows themselves, and without this
		// the only way to get them was to re-derive them by hand (and get
		// it wrong: `--compile-report` prints a line containing the word
		// "interpreter" for every program, islanded or not, so grepping
		// for it names rows that compile perfectly well).
		// Mirrors BORU_LOG_UNFLAGGED in check_accuracy_test.go.
		var logLine string
		if n != 0 && os.Getenv("BORU_LOG_CENSUS_ROWS") != "" {
			logLine = fmt.Sprintf("CENSUS ROW %s via %s: %s",
				r.Key(), seamBreakdown(rowSeams(seen)), r.Input)
		}

		mu.Lock()
		defer mu.Unlock()
		ran++
		if n == 0 {
			return
		}
		dirty++
		if logLine != "" {
			censusRows = append(censusRows, censusRow{file: r.File, line: r.Line, text: logLine})
		}
		perFile[r.File]++
		if fileSeamRows[r.File] == nil {
			fileSeamRows[r.File] = map[string]int{}
		}
		for s, c := range seen {
			seams[s] += c
			fileSeamRows[r.File][s]++ // once per ROW, whatever c is
		}
	})

	sort.Slice(censusRows, func(i, j int) bool {
		if censusRows[i].file != censusRows[j].file {
			return censusRows[i].file < censusRows[j].file
		}
		return censusRows[i].line < censusRows[j].line
	})
	for _, c := range censusRows {
		t.Log(c.text)
	}

	t.Logf("interp-entry census: %d rows, %d ran compiled, %d with UNATTRIBUTED interpreter entries (ceiling %d)",
		rows, ran, dirty, interpEntryRowCeiling)
	for _, s := range sortedKeys(seams) {
		t.Logf("   seam %-28s %d", s, seams[s])
	}
	for _, f := range sortedKeys(perFile) {
		if perFile[f] >= 5 {
			t.Logf("   file %-34s %d rows   via %s", f, perFile[f], seamBreakdown(fileSeamRows[f]))
		}
	}

	gate(t, "interp-entry census rows", dirty, 0, interpEntryRowCeiling, true,
		"corpus rows that run compiled and still enter the interpreter through an unattributed seam — the OpFallback island ceiling cannot see this (it counts disassembly spans, not a CallBoru inside a handler)")
}

// runWithEntryHook runs one row compiled with the interpreter-entry hook armed
// and returns the unattributed entry count. ok is false when the row did not
// run compiled at all — a compile failure or a static check error, where the
// interpreter owning the program is the designed behaviour and not an island.
func runWithEntryHook(t testing.TB, src string) (int, map[string]int, bool) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetClock(specClock)
	var mu sync.Mutex
	seen := map[string]int{}
	total := 0
	disarm := a.ArmInterpEntryHook(func(ev lang.InterpEntry) {
		if ev.CheckMode || ev.Attribution != "" {
			return
		}
		mu.Lock()
		seen[ev.Seam]++
		total++
		mu.Unlock()
	})
	_, compiled, _ := a.RunCompiled(src)
	disarm()
	mu.Lock()
	defer mu.Unlock()
	return total, seen, compiled
}

// seamBreakdown renders one file's seam→rows map as a stable, compact line —
// "Engine.Run 14, CallBoru 9" — highest first, ties broken by name so a diff of
// two census runs is readable. Empty maps cannot reach here (a file only lands
// in perFile once a row recorded at least one seam).
// rowSeams collapses one row's per-seam ENTRY COUNTS to a once-per-seam map,
// so a row's listing reads like the file table's (which counts rows, not
// entries) rather than double-reporting a seam a row entered twice.
func rowSeams(seen map[string]int) map[string]int {
	out := make(map[string]int, len(seen))
	for s := range seen {
		out[s] = 1
	}
	return out
}

func seamBreakdown(bySeam map[string]int) string {
	names := sortedKeys(bySeam)
	sort.SliceStable(names, func(i, j int) bool { return bySeam[names[i]] > bySeam[names[j]] })
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s %d", n, bySeam[n]))
	}
	return strings.Join(parts, ", ")
}
