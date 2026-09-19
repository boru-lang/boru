package langspec

import (
	"errors"
	"flag"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// compiled_defect_test.go — the corpus-wide BAIL ledger.
//
// A program that does not compile has always been counted here
// (compile_failures.tsv, per spec file). Its sibling was not, because it did
// not look like a failure: a program that COMPILED and then abandoned the
// compiled run — a VM/lowering soundness assertion, a designed defer, a
// recovered handler panic, a foreign Go error — was rolled back and re-run on
// the interpreter, so the lanes agreed and the gates saw nothing.
//
// Nothing re-runs (2026-09-19). Those rows now show the defect, and a
// differential gate would read it as a NEW miscompile, which it is not: it is
// an OLD one, finally visible. compiledDefect is how a gate tells the two
// apart, and bailDefectCeiling is the ratchet that keeps the count honest —
// a bug count, never a budget, and the worse of the two ledgers because the
// program was judged compilable and the judgement did not hold.
//
// The bail CENSUS (engine_entry_census_test.go, deferCeiling) counts defer
// SITES firing. This counts corpus ROWS whose result is the defect. They
// measure different things and both must reach zero.

// bailDefectCeiling is the number of corpus rows whose compiled run reports a
// defect instead of a result. Lower it with the fix; raise it only in the
// change that regressed, naming what moved.
//
// 2026-09-19: set when the interpreter fallbacks were removed. Not new bugs —
// each row was already bailing and being answered by a silent re-run.
const bailDefectCeiling = 0

var bailDefects = struct {
	mu      sync.Mutex
	rows    map[string]string
	reasons map[string]int
}{rows: map[string]string{}, reasons: map[string]int{}}

// compiledDefect reports whether errC is the compiled lane reporting a
// COMPILER defect rather than the program's own result, and books it. The
// marker is compiledRunError's note, which only the compiled runtime attaches
// — an internal_error the PROGRAM raised carries no such note.
func compiledDefect(t testing.TB, key, detail string, errC error) bool {
	var ae *core.BoruError
	if !errors.As(errC, &ae) || ae.Code != "internal_error" {
		return false
	}
	marked := false
	for _, n := range ae.Notes {
		if strings.Contains(n, "this is a compiler defect") {
			marked = true
			break
		}
	}
	if !marked {
		return false
	}
	bailDefects.mu.Lock()
	defer bailDefects.mu.Unlock()
	bailDefects.rows[key] = detail
	bailDefects.reasons[bailReasonOf(ae.Detail)]++
	return true
}

// bailReasonOf strips a bail's position suffix so the ledger groups by site
// rather than by source location.
func bailReasonOf(detail string) string {
	if i := strings.Index(detail, " (pc="); i >= 0 {
		detail = detail[:i]
	}
	return detail
}

// TestMain asserts the ledger after the package's walks have run. A FILTERED
// run (-run, or BORU_SPEC_FILES over one family) walks a subset and would
// undercount, so the assertion is made only on a whole-package walk — the
// same rule the per-file compile-failure ledger applies.
func TestMain(m *testing.M) {
	code := m.Run()
	if !filteredRun() {
		os.Stderr.WriteString(bailDefectReport())
		if code == 0 && len(bailDefects.rows) != bailDefectCeiling {
			code = 1
		}
	}
	os.Exit(code)
}

func filteredRun() bool {
	if f := flag.Lookup("test.run"); f != nil && f.Value.String() != "" {
		return true
	}
	return os.Getenv("BORU_SPEC_FILES") != ""
}

func bailDefectReport() string {
	bailDefects.mu.Lock()
	defer bailDefects.mu.Unlock()
	n := len(bailDefects.rows)
	var b strings.Builder
	b.WriteString("\ncorpus rows that compile and then bail: ")
	b.WriteString(strconv.Itoa(n))
	b.WriteString(" against a ceiling of ")
	b.WriteString(strconv.Itoa(bailDefectCeiling))
	switch {
	case n > bailDefectCeiling:
		b.WriteString("\n  ABOVE the ceiling — a regression: programs that completed no longer do.\n")
	case n < bailDefectCeiling:
		b.WriteString("\n  BELOW the ceiling — the ratchet tightened; lower bailDefectCeiling to ")
		b.WriteString(strconv.Itoa(n))
		b.WriteString(" in this change.\n")
	default:
		b.WriteString("\n  At the ceiling.\n")
	}
	keys := make([]string, 0, len(bailDefects.reasons))
	for k := range bailDefects.reasons {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if bailDefects.reasons[keys[i]] != bailDefects.reasons[keys[j]] {
			return bailDefects.reasons[keys[i]] > bailDefects.reasons[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		b.WriteString("    " + strconv.Itoa(bailDefects.reasons[k]) + "  " + k + "\n")
	}
	return b.String()
}
