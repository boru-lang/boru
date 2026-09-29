package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR332StrandedWordNoPanic pins the root of NUR332's panic. The check
// pass's failed-dispatch recovery (checkModeAssumeSig) assumes a best-fit
// overload even when the site cannot supply its full window, and handed that
// SHORT window to the overload's ReturnsFn — the analysis half of a matched
// call, which reads its operands positionally: `if` stranded in a clause
// list read args[0] of nothing, and the recovered panic surfaced as an
// internal_error where the interpreter reports the user's no-match. A short
// window now never reaches a ReturnsFn (check's declaredReturnCarriers), and
// every ReturnsFn the recovery reached this way guards its own window too.
// Each word below panicked stranded in at least one of these positions.
func TestNUR332StrandedWordNoPanic(t *testing.T) {
	words := []string{"if", "case", "error", "while", "for", "fold", "inner", "outer", "as", "behave", "__arm"}
	shapes := []string{"%s", "1 %s", "if (1 eq 1) [%s] [0]", "case 7 [[lt 10] %s]", "[1 2] each [%s]"}
	for _, w := range words {
		for _, shape := range shapes {
			src := fmt.Sprintf(shape, w)
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("%q: the compile pass panicked: %v", src, rec)
					}
				}()
				_, reason, _, err := mustNew(t).CompileCheck(src)
				if msg := reason + " " + fmt.Sprint(err); strings.Contains(msg, "internal engine error") || strings.Contains(msg, "runtime error") {
					t.Errorf("%q: the check pass must not fail internally: %s", src, msg)
				}
			}()
		}
	}
}

// TestNUR332StrandedIfCompilesToItsError pins NUR332's close: the program
// compiles, and raises the interpreter's signature_error byte for byte. The
// stranded `if` is a clause BLOCK of the `case` — a bare word the handler
// hands back for the tape to re-step at the case — and the case sits bare
// (nothing beneath it, nothing after it), so the compile desugar's sealed arm
// meets exactly what the interpreter's re-step meets. The no-match inside a
// SEALED branch arm compiles to a trap the arm raises when it runs (the
// `if` word's literal arms: the interpreter runs them over their own tokens
// alone), where only a top-level trap was recorded before.
func TestNUR332StrandedIfCompilesToItsError(t *testing.T) {
	for _, src := range []string{
		`case 7 [[lt 3] "low" [lt 10] if (1 eq 1) ["mid"] ["x"] "high"]`,
		`case 7 [[lt 3] "low" [lt 10] if (1 eq 1) ["mid"] ["x"] "high"] end 5`,
		`case 7 [[lt 3] "low" [lt 10] if]`,
		`(case 7 [[lt 3] "low" [lt 10] if "z"])`,
		`case 7 [[lt 10] if "z"] end 1 2 3`,
		`case [7] [[lt 10] add "z"]`,
		// The arm trap over the `if` word's own sealed arms.
		`if (1 eq 1) [if] [0]`,
		`if (1 eq 1) [if (2 eq 2)] [0]`,
		`5 if (1 eq 1) [add 1] [0]`,
		`5 if true [add 1] [0]`,
		`def c true end 5 if c [add 1] [0]`,
		`1 if true [dup] [0]`,
		`if (1 eq 2) [0] [dup]`,
		`if (1 eq 1) [add] [0] 1`,
		`if (1 eq 1) [1 add] [0]`,
		`if (1 eq 1) [dup 3 add] [0]`,
		`def f fn [[x:Integer][Any][5 if (x eq 1) [add 1] [0]]] end f 1`,
		`5 if (1 eq 1) [add 1]`,
		`5 if [1 eq 1] [add 1]`,
		`5 (if (1 eq 1) [add 1] [0])`,
		`def b [add 1] end 5 if (1 eq 1) b [0]`,
		`5 if (1 eq 1) [if (2 eq 2) [add 1] [0]] [0]`,
		`[1 2] each [if (1 eq 1) [add 1] [0]]`,
		`def c (1 eq 1) end 5 if c [add 1] [0] 7`,
		`5 if (1 eq 2) 3 [add 1]`,
		`5 if [true] [add 1] [0]`,
		`5 if [false] [0] [add 1]`,
		// The arm is not taken: the program answers.
		`5 if (1 eq 1) 3 [add 1]`,
		`5 if [false] [add 1] [0]`,
		`case 2 [[lt 3] "low" [lt 10] if "z"]`,
		`case 20 [[lt 3] "low" [lt 10] if "z"]`,
		`if (1 eq 1) [0] [dup]`,
		`if (1 eq 2) [dup] [0]`,
		`def f fn [[x:Integer][Any][5 if (x eq 1) [add 1] [0]]] end f 2`,
		// A block naming a value, or a function of no operands, steps alike
		// wherever it is stepped.
		`def foo 5 end 1 case 7 [[lt 10] foo "z"]`,
		`def g fn [[][Integer][5]] end 1 case 7 [[lt 10] g "z"]`,
		`1 case 7 [[lt 10] Integer "z"]`,
		`1 case 7 [[lt 10] add/v "z"]`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR332ReSteppedBlockDeclines pins the soundness half of the close: a
// clause block that is a bare FUNCTION word re-steps AT THE CASE over the
// values beneath it and the tokens after it, so the desugar's sealed arm is
// no model of it unless the case sits bare. Where the pass decides the
// clause the run takes, the case compiles as that re-step (NUR342 —
// TestNUR342ReSteppedBlockCompiles, where the `g` rows that answered [1 5]
// compiled for the interpreter's [101] on main 2ad4620 now agree). A case
// it cannot decide and that does not sit bare still declines — the
// clause-list `if`'s re-stepped arms likewise — rather than compile the
// sealed arm.
func TestNUR332ReSteppedBlockDeclines(t *testing.T) {
	const reStep = "case: a clause block that is a bare function word"
	for _, c := range []struct{ src, why, want string }{
		// A code-body scrutinee runs at the case: its desugar decides nothing.
		{`1 2 case [7] [[lt 10] add "z"]`, reStep, "[3]"},
		// A scrutinee only the run knows, with values after the case.
		{`def f fn [[n:Integer][Any][case n [[lt 10] add "z"] n n]] end f 5`, reStep, "[10]"},
		{`def f fn [[][Integer][7]] end 1 2 case (f) [[lt 10] add "z"]`, reStep, "[3]"},
		// A guard the pass does not run: an effect, a word outside the pure
		// comparison family, a raise, a word match.
		{`1 2 case 7 [[print "x" lt 10] add "z"]`, reStep, "[3]"},
		{`1 2 case 7 [[drop 5 lt 10] add "z"]`, reStep, "[3]"},
		{`1 2 case 7 [[lt "a"] add "z"]`, reStep, "ERROR:cannot order Integer and ProperString"},
		{`1 2 case 7 [Integer add "d"]`, reStep, "[3]"},
		{`1 2 case 7 [[eq [1]] add "z"]`, reStep, "[1 2 z]"},
		{`1 2 case 7 [[eq add/v] add "z"]`, reStep, "[1 2 z]"},
		{`def c [1] end 1 2 case 7 [[eq c] add "z"]`, reStep, "[1 2 z]"},
		// A guard naming no word the pass knows.
		{`1 2 case 7 [[lt nosuchword] add "z"]`, reStep, "ERROR:undefined word: nosuchword"},
		// The pass decides that NO clause is taken, and its own coverage
		// finding (case_not_exhaustive) stops the compile first.
		{`1 2 case 20 [[lt 10] add]`, "check diagnostics", "[1 2]"},
		// Inside an arm of a fn body over a param, the case is not bare: the
		// arm's model runs on no frame of its own.
		{`def f fn [[x:Integer][Any][if (x gt 0) [case x [[lt 10] dup "z"]] [0]]] end f 1`, reStep, "ERROR:cannot call `dup`"},
		// A list CONDITION runs inline over the values beneath the `if`: no
		// sealed arm.
		{`1 if [true [dup] [0]]`, "unmatched dispatch recovered at dup", "[1]"},
		{`1 if [false [0] [dup]]`, "unmatched dispatch recovered at dup", "[1]"},
		{`1 if [dup] ["a"] ["b"]`, "unmatched dispatch recovered at dup", "[a]"},
		// The clause-list `if` re-steps its chosen body at the `if`: its arms
		// are captured unsealed, and the no-match keeps its decline.
		{`if [true [dup] [0]]`, "unmatched dispatch recovered at dup", "ERROR:cannot call `dup`"},
		{`if [false [0] [add]]`, "unmatched dispatch recovered at add", "ERROR:cannot call `add`"},
		// A loop body is no sealed arm either.
		{`1 for 3 [dup]`, "unmatched dispatch recovered at dup", "ERROR:cannot call `dup`"},
	} {
		prog, reason, _, err := mustNew(t).CompileCheck(c.src)
		if prog != nil || err != nil || !strings.Contains(reason, c.why) {
			t.Errorf("%q: want the decline %q, got program=%v reason=%q err=%v", c.src, c.why, prog != nil, reason, err)
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if sub, isErr := strings.CutPrefix(c.want, "ERROR:"); isErr {
			if errI == nil || !strings.Contains(errI.Error(), sub) {
				t.Errorf("%q: interpreter %v / %v, want an error containing %q", c.src, gotI, errI, sub)
			}
		} else if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
	}
}

// TestJoinedElementBodyRaisesCompiled pins the second compile refusal of
// 2026-09-28: an each body over a list mixing an Integer and a type literal
// (the element's joined carrier, Integer tor Type) declined "unmatched
// dispatch recovered at add" — the body's SUSPENDED model run latched the
// program's failure over a dispatch the body's own unit compile records as
// the runtime re-matching poly. That run now decides nothing, and the poly
// carries the optimistic dispatch's exact layout (PolyRef.Split), so the VM
// re-matches each element and raises the interpreter's error on the one
// that misses, byte for byte, prefix and all.
func TestJoinedElementBodyRaisesCompiled(t *testing.T) {
	for _, src := range []string{
		`[1 Integer] each [add 1]`,
		`[Integer 1] each [add 1]`,
		`[1 {}] each [add 1]`,
		`[1 {a:1}] each [add 1]`,
		`[1 Integer] each [add 1] end 5`,
		`[[1 Integer]] each [each [add 1]]`,
		`[1 Integer] fold [add] 0`,
		// The body matches every element: the program answers (it declined
		// before too, over the same suspended latch).
		`[1 "a"] each [add 1]`,
		`[1 2.5] each [add 1]`,
		// The happy twins, unchanged.
		`[1 2] each [add 1]`,
		`[1 Integer] each [typeof]`,
		`[1 Integer] each [is Type]`,
		`[1 Integer] each [dup]`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestJoinedElementBodyRaisesAcrossSpellings: the optimistic dispatch's
// exact layout (PolyRef.Split) rides only a record whose word the source
// wrote at its position (wordWrittenAt). That probe reads columns in RUNES,
// as the parser counts them, and takes a comma as the token separator it
// is — a comma-spelled body and a multibyte rune earlier on the line keep
// the interpreter's per-element signature_error (Codex review of #520).
func TestJoinedElementBodyRaisesAcrossSpellings(t *testing.T) {
	for _, src := range []string{
		`[1 Integer] each [add, 1]`,
		`"é" drop [1 Integer] each [add 1]`,
		`"日本" drop [1 Integer] each [add, 1]`,
	} {
		requireCompiledParity(t, src)
	}
}
