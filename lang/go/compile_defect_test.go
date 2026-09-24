package lang

import (
	"errors"
	"flag"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// compile_defect_test.go — the unit-test compile-defect ledger.
//
// A program that does not compile has hit a BUG
// (design/COMPILABLE-SUBSET.md §1). Until the interpreter fallbacks were
// removed, a unit test could assert such a program's ANSWER: the compiled
// entry point quietly re-ran it on the interpreter and handed back the right
// value, so the defect left no mark and the two lanes "agreed" — the
// agreement was the fallback's, not the compiler's.
//
// Nothing falls back now. A test whose program does not compile sees the
// compile failure, and what is left to assert is that the failure is reported
// as the defect it is: a compile_failed error, never a wrong answer and never
// a different error. noteCompileDefect asserts exactly that and COUNTS it.
//
// The count is a RATCHET ON A BUG COUNT, never a budget. It moves DOWN with
// the change that compiles those programs and only UP with a regression named
// beside the ceiling below — the same contract as the corpus ledger
// (test/go/langspec/compile_failures.tsv). Without it, teaching a parity
// helper to tolerate a compile failure would quietly stop these tests
// reporting a compile REGRESSION, which is the coverage the fallback's
// removal must not cost.

// The two ceilings. Every unit is an open compiler defect, and both are
// ceilings on BUGS, never budgets: lower one with the fix, raise it only in
// the change that regressed, naming what moved.
//
//   - compileDefectCeiling: programs the emitter cannot lower at all. The
//     run reports compile_failed and never starts.
//   - bailDefectCeiling: programs that COMPILE and then abandon the compiled
//     run — a VM/lowering soundness assertion, a designed defer, a recovered
//     handler panic, a foreign Go error. These are the worse half: the
//     program was judged compilable and the judgement did not hold.
const (
	// Set 2026-09-19, the change that removed the interpreter fallbacks.
	// These are not new bugs: every one of them was already there, answered
	// by a silent re-run on the interpreter and counted by nothing.
	compileDefectCeiling = 287 // 286 -> 287 on 2026-09-24 (NUR199 closed, the keep-defs body): one NEW unit-suite program that fails to compile loudly — TestDoBodyDefLeaksToTheEnclosingScope's fn-frame edge `def f fn [[][Integer][def t 0 for 3 [do [def t 5]] t]] end f` (5 interpreted), a fn whose RESULT is the name a `do` body inside its loop leaks: the loop over a count-agnostic body leaves the fn's residual a variadic loop value the RET cannot seat ("result is a variadic loop value"); it answered 0 before this change (the loop-carried slot never saw the do's def — the divergence NUR199 records), so a loud decline is the sound direction, added as a pin, not a regression. Before: 282 -> 286 on 2026-09-24 (NUR196 closed, the escaped body ends the iteration): four NEW unit-suite programs that fail to compile loudly — TestLiteralBodyFlowThroughFnResolves' fallback-parity rows, a fn whose body is a bare `break` run by natives the compiler still declines under a loop: `for 3 [[1 2] for-each [f] i] 99` (code-body word for-each), `for 3 [0 fold [f] [1 2 3] i] 99` (the fold's branch leaves extra values) and `ArrayUtil.eachrank 0 [f] [[1 2] [3 4]]` / `ArrayUtil.eachrank 1 [fl] …` under the same loop (code-body word eachrank), each [99] interpreted and answered by fallback; they pin the interpreter lane's escape arms (core.BodyEscaped ends the iteration) and are added as pins, not a regression. Before: 283 -> 282 on 2026-09-24 (the selective freshen): one unit-suite program that FAILED to compile now compiles — TestPR225P1CompileFailures' `def c [9] def mk fn [[] [List] [[c]]] ((mk) get 0) eq c`, a fn body literal embedding an enclosing binding's container, whose fresh push clones the literal's spine and keeps the embedded member (Program.ConstKeep, core.CloneValueKeeping); both identities hold on both lanes (fn_body_embed_keep_test.go). Before: 282 -> 283 on 2026-09-24 (the written argument's fit, NUR194 closed): one NEW unit-suite program that fails to compile loudly — TestDoBodyReadWrittenNoMatchParity's `(a5 "s")` (a written argument the def-bound closure's parameter does not take; the interpreter raises the no-match), declined at the shaped read's window now that the shape claim carries the wrapper's parameter types; added as a pin, not a regression; the two bail witnesses of the same shape inside bodies now agree with the interpreter (see bailDefectCeiling 39 -> 37). Before: 281 -> 282 on 2026-09-24 (the frame's closure bind, NUR192 closed): the nested-def witness of the previous increment COMPILES now (the frame's dynamic-scope bind pushes the closure value, TestDefFnBodyTailParityAndNoEntry's native rows) and two NEW unit-suite programs fail to compile loudly — TestDefFnBodyTailShadowSoundCompileFailures' witnesses, a fn body's computed fn def shadowing an enclosing frame's computed fn of the same name (`def a5 (mk 1)  def g fn [[xs:List][List][def a5 (mk 5)  each [a5] xs]]  g [1 2 3]  each [a5] [1 2 3]`, `[[6 7 8] [6 7 8]]` interpreted: the interpreter's install drops the overlapping outer closure at the same depth and the redefinition outlives the call, which the compiled frame's push-and-pop cannot model), bound before or after the fn's definition; net one more, added as pins, not a regression. Before: 280 -> 281 on 2026-09-24 (the def-bound computed fn read at a closure body's tail): one NEW unit-suite program that fails to compile loudly — TestDefFnBodyTailNestedDefSoundCompileFailure's witness, a fn-body-LOCAL computed def read from a nested closure body (`def g fn [[xs:List][List][def a5 (mk 5)  each [a5] xs]] end g [1 2 3]`, [[6 7 8]] interpreted), which the native used to run as a raw body on the interpreter and raise a false `undefined word: a5` (NUR192, present on main); the check pass now stands aside for the read inside the closure unit and the unapplied-fn guard declines the body, so the program falls back whole and answers; added as a pin, not a regression. Before: 279 -> 280 on 2026-09-23 (the trailing value's re-step, NUR184): one unit-suite program MOVED from the bail line to this one — TestParenApplyLowering's non-member fence `def q (if true [([y:Integer] => [y add 1])] [([y:Integer] => [y sub 1])]) add 1 (q 5)` (7 interpreted), whose poly record of `add` collected the re-step-marked branch closure with the 1 and bailed at run time (`CALL_NATIVE_POLY no match for add`); a native poly record that collects a re-step-marked fn carrier declines now (polyCallDeclineReason), so the same program fails earlier and louder. Not a new defect: see bailDefectCeiling 34 -> 33. Before: 280 -> 279 on 2026-09-22 (the quotation-body container reads): a code-body closure unit takes the whole-frame replay for a tagged fn-member read at its tail and a paren-placed value is data inside any unit (NUR182) — TestEdgeFindingDynamicFnValueApplyBodyTail's mid-body row (`[nd (m get "inc") print "after"]`, the interpreter's own count error on both lanes) and the unit suite's other placed-member rows graduated, against the three NEW sound-failure witnesses TestQuotationBodyMemberReadSoundCompileFailures adds; net one fewer. Before: 284 -> 280 on 2026-09-22 (the curried chain): three unit-suite programs GRADUATED to parity rows — TestCurriedFactoryCompiles's three-level fence `(((mk3 1) 2) 3)`, TestFnValueAutoApplyCompileFailures's nested-factory chain and NUR101's list-element fence `[((mk 1) 2)]` — and the four NEW sound-failure witnesses TestCurriedChainSoundCompileFailures adds (`((mk 1) 2 3)`, `((mk 1) "s") 7`, a gradual argument under NUR121, the def-bound chain's read-model decline) are offset by the other rows those three tests and their neighbours stopped declining; net four fewer. Before: 283 -> 284 on 2026-09-22 (S1b-3 re-landed): one NEW unit-suite program that fails to compile loudly at the check — TestStoredFnValueNeverSilentlyWrong's witness, a stored closure reading the loop iterator `i` after the loop (the interpreter raises undefined_word on the same read) — added as a pin, not a regression; on the same day S1b's apply shapes graduated one program (the literal-read row inside a branch arm) and NUR177's fix none. Before: 284 -> 283 on 2026-09-22: the branch-carried def (compiler/go/branch_carried.go) — a name an `if` arm binds, read after the merge, loads from a frame slot; one unit-suite program that declined on that read compiles
	bailDefectCeiling    = 44  // 40 -> 44 on 2026-09-24 (NUR196 closed, the escaped body ends the iteration): four NEW unit-suite programs that compile and bail loud where the interpreter raises — TestLiteralBodyFlowThroughFnResolves' loop-less rows `each [f] [1 2 3]`, `0 fold [f] [1 2 3]`, `scan [f] [1 2 3]` and `filter [f] [1 2 3]` over a fn whose body is a bare `break`: the native ends its iteration on the escaped body (core.BodyEscaped) and returns no result, and with no loop to resolve the flag the VM's resolveEscapedFlow after the native call raises the designed internal error RunCompiled's callers defer to the interpreter on (NUR195's contract), for the interpreter's `break outside loop` (and, over `filter`, its former "must produce a Boolean, got __CP" — the cleanup marker the escaped run left on its tape, gone with the fix). Before each's arm the compiled lane raised each's own no-result error; a bail for a false error, added as pins. Before: 37 -> 40 on 2026-09-24 (NUR195 closed, the escaped flow after a native call): three NEW unit-suite programs that compile and bail loud where they ANSWERED before — TestComputedBodyFlowSentinelDefers' `each (mk) [1 2 3]` over a computed `[break]`, its `continue` twin and `fold (mk) [1 2 3] 0` over `[break]`, each the interpreter's `break outside loop` and, on the compiled lane, `[[1 2 3]]` / `[3]` silently until the VM read the flag a native's body run leaves set; now the loop-less flow's designed internal error, which RunCompiled's callers defer to the interpreter on. A silent value traded for a counted bail. Before: 36 -> 37 on 2026-09-24 (the foreign-home fn value at the apply seam): one NEW unit-suite program that compiles and bails — TestForeignFnValueApplyClaimKeepsIsland's `5 (m 'f' get) apply` over a TWO-return module fn, the pre-existing apply-one bail (`apply over a gradual lead netted 2 value(s), not the one the model committed`, the same on main), written to witness the seam's result-count decline standing aside before the island. Before: 37 -> 36 on 2026-09-24 (the recorder's registry after a module body): one unit-suite program that compiled and bailed at run time now DECLINES at compile time — TestUnmatchedDispatchTrapCarrierDisjoint's "one Point candidate for two Point slots" (`import module [def Point class {…} def add fn [[a:Point b:Point] …] export "Pointer" {…}]  def p0 (make Pointer.Point {x:1 y:2})  add p0 1`, the interpreter's `cannot call add`), whose top-level `add` over a module Point and an Integer bailed on the poly no-match (`CALL_NATIVE_POLY no match for add`) while the recorder sat on the module registry after the import; with the recorder's registry following the running engine again (BindRegistry's restore), the check pass's unmatched-dispatch recovery declines it, and the program falls back to the interpreter's own raise. Before: 39 -> 37 on 2026-09-24 (the written argument's fit, NUR194 closed): the two witnesses booked the same day (`do [a5 "s"]`, `each [a5 "s" add] [1 2]`) COMPILE AND AGREE now — the shaped read's window declines the unfit written token, the body islands, and the island dispatches the word as the interpreter does. Before: 37 -> 39 on 2026-09-24 (the do body's read, NUR193 closed): two NEW unit-suite programs that compile and then bail loudly — TestDoBodyReadWrittenNoMatchBails' witnesses, a def-bound computed fn read with a WRITTEN operand its contract does not take (`do [a5 "s"]`, `each [a5 "s" add] [1 2]`), where the shaped read's window claims the written token as the argument and the run bails on the claim while the interpreter's matcher falls back to the frame or raises into `do`'s Error value (NUR194, present on main; the `do` row was `[fn a5(Integer) s]` there, silent); added as pins, not a regression. Before: 34 -> 37 on 2026-09-24 (NUR190's open halves deferred): three MORE unit-suite programs that compile and then bail loudly — TestNamedFnCandidatesOpenShapes's `m.f y`, `m.f z` and `m.q z` (a `/q` slot CAPTURES the following word; `[y]` and `[z]` interpreted), which used to compile `[42 42]` and `[42 0]` silently (and `[z]` by coincidence) while the walk stood aside; the walk now defers at the landing as the Function-typed reference does, the maintainer's call (a decline would be over-wide: no static model tells a `/q` slot from a typed slot's barrier), and the corpus keeps such rows on the runtime-defers ledger. Before: 33 -> 34 on 2026-09-23 (the landing's overload walk, NUR190): one NEW unit-suite program that compiles and then bails loudly — TestNamedFnCandidatesOpenShapes's witness `m.g z` (a dynamic fn value whose Function-typed slot takes the following word's REFERENCE, 7 interpreted), where the wordless landing raised a false uncalled_function and the walk now bails at the landing because the word's call is compiled after it and cannot be skipped; added as a pin, not a regression. Before: 34 -> 33 on 2026-09-23 (the trailing value's re-step, NUR184): TestParenApplyLowering's non-member fence no longer compiles and bails — it declines at the poly record (see compileDefectCeiling 279 -> 280, the same program). Before: 33 -> 34 on 2026-09-22 (the curried chain): one NEW unit-suite program that compiles and then bails loudly — TestCurriedChainPendingCollection's witness, `0 fold ([a:Integer e:Integer] => [a add ((mk 1) e)]) xs`, whose paren collapses on behalf of add's pending collection (the survivors become add's arguments, nothing re-steps, the interpreter raises signature_error) and whose compiled poly re-match bails with the same no-match — measured identical on a worktree at 917ecf0; added as a pin, not a regression. Before: 32 -> 33 on 2026-09-22 (the dynamic-lead group): one NEW unit-suite program that compiles and then bails loudly — TestTopLevelGradualApplyDefers' witness, a 2-arg fn under a top-level gradual `apply` (`3 5 m.f/v apply`), which the one-result event form (OpCallDynApplyOne) defers exactly as it does inside a unit (TestGradualApplyDefers); added as a pin, not a regression
)

type defectLedger struct {
	mu      sync.Mutex
	n       int
	bails   int
	reasons map[string]int
}

var compileDefects = defectLedger{reasons: map[string]int{}}

// noteCompileDefect records that src does not compile, after asserting the
// compiled lane reported it plainly. It returns true when it took the defect
// arm, so a caller can skip a comparison that has no compiled side to make.
func noteCompileDefect(t *testing.T, src string, gotC []any, errC error) bool {
	t.Helper()
	code := codeOf(errC)
	if code != "compile_failed" && !isBailDefect(errC) {
		return false
	}
	if len(gotC) != 0 {
		t.Errorf("%q: a run that reported a defect returned a result too: %v", src, gotC)
	}
	compileDefects.mu.Lock()
	defer compileDefects.mu.Unlock()
	if code == "compile_failed" {
		compileDefects.n++
		compileDefects.reasons["COMPILE  "+compileDefectReason(errC)]++
	} else {
		compileDefects.bails++
		compileDefects.reasons["BAIL     "+compileDefectReason(errC)]++
	}
	return true
}

// isBailDefect reports whether errC is the compiled runtime abandoning a
// program it had already judged compilable. compiledRunError marks exactly
// this class with its note, so the marker is the classifier — a plain
// internal_error the PROGRAM raised is not one.
func isBailDefect(err error) bool {
	var ae *core.BoruError
	if !errors.As(err, &ae) || ae.Code != "internal_error" {
		return false
	}
	for _, n := range ae.Notes {
		if strings.Contains(n, "this is a compiler defect") {
			return true
		}
	}
	return false
}

// requireCompileDefect is noteCompileDefect for a caller that has already
// established the program does not compile: the compiled lane MUST report
// compile_failed, and anything else — an answer, a different error — is a
// failure here rather than a silently unbooked row.
func requireCompileDefect(t *testing.T, src string, gotC []any, errC error) {
	t.Helper()
	if !noteCompileDefect(t, src, gotC, errC) {
		t.Errorf("%q: a program that does not compile must report compile_failed, got %v / %v", src, gotC, errC)
	}
}

// NoteCompileDefect is noteCompileDefect for the external test package in
// this directory (package lang_test), which is compiled against this test
// build of package lang and so shares the one ledger.
func NoteCompileDefect(t *testing.T, src string, gotC []any, errC error) bool {
	t.Helper()
	return noteCompileDefect(t, src, gotC, errC)
}

// compileDefectReason strips the fixed prose off a compile_failed detail so
// the ledger groups by the emitter's reason, which is what names the defect.
func compileDefectReason(err error) string {
	d := ""
	var ae *core.BoruError
	if errors.As(err, &ae) {
		d = ae.Detail
	}
	d = strings.TrimPrefix(d, "bytecode compilation FAILED: ")
	if i := strings.Index(d, " — this is a compiler defect"); i >= 0 {
		d = d[:i]
	}
	return d
}

// TestMain asserts the ledger after the package's tests have run. A FILTERED
// run (-run) walks a subset and would undercount, so the assertion is made
// only on a whole-package run — exactly how the corpus ledger treats
// BORU_SPEC_FILES.
func TestMain(m *testing.M) {
	code := m.Run()
	if !filteredRun() {
		os.Stderr.WriteString(compileDefectReport())
		if code == 0 && (compileDefects.n != compileDefectCeiling || compileDefects.bails != bailDefectCeiling) {
			code = 1
		}
	}
	os.Exit(code)
}

func filteredRun() bool {
	f := flag.Lookup("test.run")
	return f != nil && f.Value.String() != ""
}

func compileDefectReport() string {
	var b strings.Builder
	b.WriteString("\ncompile-defect ledger\n")
	b.WriteString(line("programs that do not compile", compileDefects.n, compileDefectCeiling))
	b.WriteString(line("programs that compile and then bail", compileDefects.bails, bailDefectCeiling))
	b.WriteString("  Each line is a ratchet on a BUG COUNT. Below the ceiling: programs now\n")
	b.WriteString("  compile or complete that did not — lower it in this change. Above it: that\n")
	b.WriteString("  is a regression.\n")
	keys := make([]string, 0, len(compileDefects.reasons))
	for k := range compileDefects.reasons {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if compileDefects.reasons[keys[i]] != compileDefects.reasons[keys[j]] {
			return compileDefects.reasons[keys[i]] > compileDefects.reasons[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		b.WriteString("    ")
		b.WriteString(itoa(compileDefects.reasons[k]))
		b.WriteString("  ")
		b.WriteString(k)
		b.WriteString("\n")
	}
	return b.String()
}

func line(what string, live, ceiling int) string {
	verdict := "  (at the ceiling)"
	if live < ceiling {
		verdict = "  BELOW the ceiling — lower it to " + itoa(live)
	} else if live > ceiling {
		verdict = "  ABOVE the ceiling — a regression"
	}
	return "  " + what + ": " + itoa(live) + " against " + itoa(ceiling) + verdict + "\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
