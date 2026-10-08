package lang

import (
	"fmt"
	"strings"
	"testing"
)

// diagnostic_parity_fixes_test.go pins the 2026-09-27 closures of the
// diagnostic-parity ledger (test/go/langspec/diagnostic_parity_test.go):
// `boru check` (the plain pass) and CompileCheck's compile-armed pass must
// report the same findings, and reporting them on the armed pass must not
// change what compiles or what the compiled program computes.

// findings renders a pass's error and warning findings as code/word, in
// order — the identity the parity ledger compares.
func findings(ds []CheckDiagnostic) []string {
	var out []string
	for _, d := range ds {
		if d.Severity == SeverityError || d.Severity == SeverityWarning {
			out = append(out, d.Code+"/"+d.Word)
		}
	}
	return out
}

func findingOf(ds []CheckDiagnostic, code, word string) (CheckDiagnostic, int) {
	var first CheckDiagnostic
	n := 0
	for _, d := range ds {
		if d.Code == code && d.Word == word {
			if n == 0 {
				first = d
			}
			n++
		}
	}
	return first, n
}

// armedWord names the word of the first no_signature finding ("" if none).
func armedWord(ds []CheckDiagnostic) string {
	for _, d := range ds {
		if d.Code == "no_signature" {
			return d.Word
		}
	}
	return ""
}

// bothPasses runs the plain check and the compile-armed check over src and
// fails unless their findings agree.
func bothPasses(t *testing.T, src string) (plain, armed CheckResult, compiled bool, reason string) {
	t.Helper()
	plain, err := mustNew(t).Check(src)
	if err != nil {
		t.Fatalf("%q: plain check: %v", src, err)
	}
	prog, reason, armed, err := mustNew(t).CompileCheck(src)
	if err != nil {
		t.Fatalf("%q: compile check: %v", src, err)
	}
	if p, a := strings.Join(findings(plain.Diagnostics), ","), strings.Join(findings(armed.Diagnostics), ","); p != a {
		t.Fatalf("%q: the passes disagree:\n  plain %s\n  armed %s", src, p, a)
	}
	return plain, armed, prog != nil, reason
}

// A no-match the compile pass has DECIDED — a terminal trap or a decline —
// is reported on that pass too, as a RuntimeMirror: the plain pass's
// model-undermining finding, with the compile consequence already carried by
// the recording. A runtime REMATCH is not decided: the live values may match
// and the program continue, so it carries no mirror.
func TestArmedNoSignatureIsARuntimeMirror(t *testing.T) {
	t.Run("a trapped no-match compiles and raises byte-identically", func(t *testing.T) {
		for _, src := range []string{
			`keys 5`,
			`def f fn [[s:ProperString] [String] [s]]  f ''`,
			// Inside a SEALED branch arm (the if word's literal arm) the
			// trap is the arm's, raised when it runs (NUR332).
			`if true [keys 5] [1]`,
		} {
			plain, armed, compiled, reason := bothPasses(t, src)
			if !compiled {
				t.Fatalf("%q: the trap must still compile (%s)", src, reason)
			}
			for _, d := range plain.Diagnostics {
				if d.Code == "no_signature" && d.RuntimeMirror {
					t.Errorf("%q: the PLAIN pass's no_signature stays model-undermining: %+v", src, d)
				}
			}
			n := 0
			for _, d := range armed.Diagnostics {
				if d.Code == "no_signature" {
					n++
					if !d.RuntimeMirror {
						t.Errorf("%q: the armed pass's no_signature must be a RuntimeMirror: %+v", src, d)
					}
				}
			}
			if n != 1 {
				t.Errorf("%q: want one armed no_signature, got %d: %+v", src, n, armed.Diagnostics)
			}
			requireRaisesIdentically(t, src)
		}
	})
	t.Run("a declined no-match keeps its SPECIFIC decline reason", func(t *testing.T) {
		// A loop body is no sealed arm: the no-match there declines.
		src := `for 2 [keys 5]`
		_, armed, compiled, reason := bothPasses(t, src)
		if compiled {
			t.Fatalf("%q: a no-match inside a loop body declines", src)
		}
		if reason != "unmatched dispatch recovered at keys" {
			t.Errorf("%q: the mirror must not mask the recorder's reason as the generic sentinel; got %q", src, reason)
		}
		if d, n := findingOf(armed.Diagnostics, "no_signature", "keys"); n != 1 || !d.RuntimeMirror {
			t.Errorf("%q: want one mirrored no_signature/keys, got %+v", src, armed.Diagnostics)
		}
	})
	t.Run("a void-argument failure is mirrored and traps its own error", func(t *testing.T) {
		// The consumer of a paren group that netted nothing: the trap
		// raises the interpreter's def_error / no_value_error, and the
		// finding (as on the plain pass for `size ()`) is a no_signature
		// mirror that costs the program nothing.
		for _, src := range []string{
			`def f fn [[x:Integer] [] []] def r (f 1)`,
			`def f fn [[x:Integer] [] []] 3 add (f 1)`,
			`size ()`,
		} {
			prog, reason, armed, err := mustNew(t).CompileCheck(src)
			if err != nil || prog == nil {
				t.Fatalf("%q: must compile to its trap: %v (%s)", src, err, reason)
			}
			if d, n := findingOf(armed.Diagnostics, "no_signature", armedWord(armed.Diagnostics)); n != 1 || !d.RuntimeMirror {
				t.Errorf("%q: want one mirrored no_signature, got %+v", src, armed.Diagnostics)
			}
			requireRaisesIdentically(t, src)
		}
	})
	t.Run("a runtime rematch is no guaranteed failure and carries no mirror", func(t *testing.T) {
		// TestDispatchRematchMatchDefers's source: the static model misses,
		// the runtime value matches, and the program returns normally.
		src := `def Pos (refine Integer) def mk fn [[n:Integer][Integer][def y:Pos n y]] def g fn [[p:Pos][Integer][99]] g (mk 5)`
		prog, reason, armed, err := mustNew(t).CompileCheck(src)
		if err != nil || prog == nil {
			t.Fatalf("the rematch shape must compile: %v (%s)", err, reason)
		}
		if _, n := findingOf(armed.Diagnostics, "no_signature", "g"); n != 0 {
			t.Errorf("a rematch must not be reported as a decided runtime failure: %+v", armed.Diagnostics)
		}
		if got, err := mustNew(t).RunInterp(src); err != nil || fmt.Sprint(got) != "[99]" {
			t.Errorf("the interpreter answers 99, got %v [%v]", got, err)
		}
	})
	t.Run("a matching dispatch reports nothing on either pass", func(t *testing.T) {
		for _, src := range []string{`keys {a:1}`, `def f fn [[s:ProperString] [String] [s]]  f 'x'`} {
			plain, armed, compiled, _ := bothPasses(t, src)
			if len(findings(plain.Diagnostics)) != 0 || len(findings(armed.Diagnostics)) != 0 || !compiled {
				t.Errorf("%q: want clean and compiled, got plain %v armed %v compiled=%v", src, plain.Diagnostics, armed.Diagnostics, compiled)
			}
		}
	})
}

// requireRaisesIdentically: the compiled lane raises exactly the
// interpreter's error — the finding is a mirror, not a new behaviour.
func requireRaisesIdentically(t *testing.T, src string) {
	t.Helper()
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	gotI, errI := mustNew(t).RunInterp(src)
	if !compiled || errI == nil || errC == nil || errC.Error() != errI.Error() || len(gotC) != len(gotI) {
		t.Errorf("%q:\n  compiled %v [%v] (compiled=%v)\n  interp   %v [%v]", src, gotC, errC, compiled, gotI, errI)
	}
}

// The assumed dispatch after a no-match runs the callee's body over args the
// real match rejected; its "the call always errors" return finding is that
// cascade, and it no longer surfaces on the armed pass alone
// (word-splice.tsv:L115).
func TestAssumedDispatchReturnCascadeSilent(t *testing.T) {
	src := `def p word [1 add 2] def f fn [[x:Integer][Integer][x mul 10]] f p`
	_, armed, _, _ := bothPasses(t, src)
	if _, n := findingOf(armed.Diagnostics, "type_error", "f"); n != 0 {
		t.Errorf("the assumed dispatch's return cascade must stay silent: %+v", armed.Diagnostics)
	}
	// The genuine finding stands: a body that nets nothing for a real,
	// matched call still always errors, on both passes.
	src = `def f fn [[x:Integer][Integer][]] f 1`
	plain, armed, _, _ := bothPasses(t, src)
	if _, n := findingOf(plain.Diagnostics, "type_error", "f"); n != 1 {
		t.Errorf("%q: the matched call's empty return must still be flagged: %+v", src, plain.Diagnostics)
	}
	if _, n := findingOf(armed.Diagnostics, "type_error", "f"); n != 1 {
		t.Errorf("%q: …on the armed pass too: %+v", src, armed.Diagnostics)
	}
}

// The constant-condition dead-arm warning of a bare Boolean condition is made
// on both passes; the recording pass's lowering is unchanged, so the program
// compiles and computes what the interpreter does.
func TestStaticIfDeadArmWarnsOnBothPasses(t *testing.T) {
	for _, c := range []struct {
		src  string
		warn bool
	}{
		{`if (1 eq 1) [9] [42]`, true},
		{`if (1 eq 2) [9] () 42`, true},
		{`if (1 eq 2) [9]`, true},
		{`def g fn [[n:Integer][Integer][if (3 gt 1) [n] [0]]] g 4`, true},
		// Negatives: a true else-less `if` has no dead arm; an unknown
		// condition proves nothing.
		{`if (1 eq 1) [9]`, false},
		{`def g fn [[n:Integer][Integer][if (n gt 1) [n] [0]]] g 4`, false},
	} {
		_, armed, compiled, reason := bothPasses(t, c.src)
		if _, n := findingOf(armed.Diagnostics, "unreachable_branch", "if"); (n == 1) != c.warn || n > 1 {
			t.Errorf("%q: want warning=%v (once), got %+v", c.src, c.warn, armed.Diagnostics)
		}
		if !compiled {
			t.Errorf("%q: a warning must not decline compilation (%s)", c.src, reason)
		}
		requireCompiledParity(t, c.src)
	}
}

// The case desugar's own nested `if` chain is synthesized — its dead arms are
// not code the user wrote — while a user `if` inside a clause block keeps
// its warning on both passes.
func TestCaseDesugarDeadArmWarnings(t *testing.T) {
	src := `case 7 [[lt 3] "low" [lt 10] "mid" "high"]`
	_, armed, compiled, _ := bothPasses(t, src)
	if _, n := findingOf(armed.Diagnostics, "unreachable_branch", "if"); n != 0 || !compiled {
		t.Errorf("%q: the desugar's synthesized ifs must not warn (compiled=%v): %+v", src, compiled, armed.Diagnostics)
	}
	requireCompiledParity(t, src)
	src = `case 7 [[lt 3] "low" [lt 10] [drop if (1 eq 1) ["mid"] ["x"]] "high"]`
	_, armed, _, _ = bothPasses(t, src)
	if d, n := findingOf(armed.Diagnostics, "unreachable_branch", "if"); n != 1 || d.Row != 1 || d.Col != 36 {
		t.Errorf("%q: the user's own if keeps its warning at 1:36: %+v", src, armed.Diagnostics)
	}
}

// A fn body analysed twice on the compile pass (the dropped summary memo)
// no longer reports its one defect twice.
func TestArmedPassFindingsAreNotDuplicated(t *testing.T) {
	for _, c := range []struct{ src, code, word string }{
		{`def f fn [[][Integer][nosuch 9]] f`, "undefined_word", "nosuch"},
		{`def f fn [[] [Integer] [def a 1 end undef a end a]]  f`, "undefined_word", "a"},
		{`def f fn [[] [Integer] [def x 1 end if true [def x 9] [] end x]]  f`, "unreachable_branch", "if"},
	} {
		_, armed, _, _ := bothPasses(t, c.src)
		if _, n := findingOf(armed.Diagnostics, c.code, c.word); n != 1 {
			t.Errorf("%q: want exactly one %s/%s, got %+v", c.src, c.code, c.word, armed.Diagnostics)
		}
	}
}
