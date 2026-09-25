package test

import (
	"errors"
	"flag"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// compile_defect_test.go — this package's compile-defect ledger, the twin of
// lang/go/compile_defect_test.go (which cannot be shared: separate package,
// separate test binary).
//
// These are LANGUAGE tests. They ask what a program means — what `is` reports
// for a fn shape, what a `behave` unifier returns, what a typed def installs —
// and the tree-walking interpreter is the language's reference engine and the
// oracle every compiled lane is measured against.
//
// Until the interpreter fallbacks were removed they could ask `Run` and get
// the reference answer whether or not the program compiled, because `Run`
// re-ran it on the interpreter in silence. runReference keeps the question
// answerable and makes the silence impossible: when the program does not
// compile it says so, COUNTS it here, and reads the meaning off the reference
// engine explicitly. The count is a ratchet on a bug count, never a budget.
const (
	// Set 2026-09-19, the change that removed the interpreter fallbacks.
	// Not new bugs: each one was already there and counted by nothing.
	// 111 -> 102 on 2026-09-25 (the S2 declarations, the recovery's
	// dyn-body route and the run-time bind): nine language tests answer
	// on the compiled lane now — the behave rows (the quoted behaviour
	// name bakes as an inert const), the unpack-over-a-param rows (the
	// run-time bind), a bare-`args` read and the strict-Any fold rows.
	refDefectCeiling = 102
)

var refDefects = struct {
	mu      sync.Mutex
	n       int
	reasons map[string]int
}{reasons: map[string]int{}}

// runReference runs src the only way there is — compiled — and returns what
// the program MEANS. A program that compiles answers for itself. A program
// that does not has hit a compiler defect: the defect is booked, and the
// meaning is read off the reference engine, named as such at the call site
// rather than supplied by a fallback nobody could see.
func runReference(t *testing.T, a *lang.Boru, src string) ([]any, error) {
	t.Helper()
	out, err := a.Run(src)
	if !bookRefDefect(t, src, err) {
		return out, err
	}
	return a.RunInterp(src)
}

// runReferenceErr is runReference for a test that expects the program to
// FAIL with a particular error: a compile failure is not that error, so it is
// booked and the program's own failure is read off the reference engine.
func runReferenceErr(t *testing.T, a *lang.Boru, src string) error {
	t.Helper()
	_, err := a.Run(src)
	if !bookRefDefect(t, src, err) {
		return err
	}
	_, err = a.RunInterp(src)
	return err
}

// bookRefDefect records a compile failure or a compiled-runtime bail and
// reports whether it took one of those arms.
func bookRefDefect(t *testing.T, src string, err error) bool {
	t.Helper()
	var ae *core.BoruError
	if !errors.As(err, &ae) {
		return false
	}
	bail := ae.Code == "internal_error" && hasDefectNote(ae)
	if ae.Code != "compile_failed" && !bail {
		return false
	}
	detail := strings.TrimPrefix(ae.Detail, "bytecode compilation FAILED: ")
	if i := strings.Index(detail, " — this is a compiler defect"); i >= 0 {
		detail = detail[:i]
	}
	kind := "COMPILE  "
	if bail {
		kind = "BAIL     "
	}
	refDefects.mu.Lock()
	defer refDefects.mu.Unlock()
	refDefects.n++
	refDefects.reasons[kind+detail]++
	return true
}

func hasDefectNote(ae *core.BoruError) bool {
	for _, n := range ae.Notes {
		if strings.Contains(n, "this is a compiler defect") {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	code := m.Run()
	if f := flag.Lookup("test.run"); f == nil || f.Value.String() == "" {
		os.Stderr.WriteString(refDefectReport())
		if code == 0 && refDefects.n != refDefectCeiling {
			code = 1
		}
	}
	os.Exit(code)
}

func refDefectReport() string {
	var b strings.Builder
	b.WriteString("\ncompile-defect ledger (lang/go/test): ")
	b.WriteString(defectItoa(refDefects.n))
	b.WriteString(" against a ceiling of ")
	b.WriteString(defectItoa(refDefectCeiling))
	b.WriteString("\n  A ratchet on a BUG COUNT. Below it: programs now compile that did not —\n")
	b.WriteString("  lower it in this change. Above it: a regression.\n")
	keys := make([]string, 0, len(refDefects.reasons))
	for k := range refDefects.reasons {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if refDefects.reasons[keys[i]] != refDefects.reasons[keys[j]] {
			return refDefects.reasons[keys[i]] > refDefects.reasons[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		b.WriteString("    " + defectItoa(refDefects.reasons[k]) + "  " + k + "\n")
	}
	return b.String()
}

func defectItoa(n int) string {
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
