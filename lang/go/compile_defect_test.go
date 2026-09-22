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
	compileDefectCeiling = 283 // 284 -> 283 on 2026-09-22: the branch-carried def (compiler/go/branch_carried.go) — a name an `if` arm binds, read after the merge, loads from a frame slot; one unit-suite program that declined on that read compiles
	bailDefectCeiling    = 32
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
