package langspec

import (
	"errors"
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
//
// 52 -> 54 on 2026-09-24 (NUR190's open halves deferred, the maintainer's
// call): fn-value.tsv:L317/L318 — a `/q` slot CAPTURES the following word —
// bail at the landing's walk (vm:landing-quote-claim) where they passed by
// coincidence; booked by choice on the per-file ledger of deferred
// compilation failures (runtime_defers.tsv, runtime_defer_ledger_test.go),
// which names every row this count counts.
// 54 -> 51 on 2026-09-25 (the strict-Any dyn-body recovery): fold-map-filter.tsv
// L246–L248 run natively where they bailed at the rematch trap
// (runtime_defers.tsv's fold-map-filter line deleted).
// 51 -> 46 on 2026-09-25 (NUR141, the merge with the reverse-order NUR run):
// fnpred.tsv L34, L38, L43, L46 and record.tsv L178 — `def q:Even 5`, a
// predicate type's own rejection — are the static check error the run
// raised, now that the check pass runs a PURE predicate over a concrete
// candidate; the programs never run compiled (runtime_defers.tsv's fnpred
// line deleted, record.tsv's lowered to 2).
// 46 -> 44 on 2026-09-26 (NUR190 closed): fn-value.tsv:L317/L318 — the `/q`
// slot that CAPTURES the following word — no longer bail at the landing's
// walk: the landing hands the capture to the interpreter's island (or its
// skip) and the rows answer natively (runtime_defers.tsv's fn-value line
// deleted).
// 44 -> 39 on 2026-09-26 (NUR233, found compiling NUR231's type half): a
// make field's refusal is a type_error on both lanes, no longer a plain
// error the compiled run books as a defect — edge-dispatch-3.tsv L59,
// generics.tsv L56, module-struct.tsv L100 and record.tsv L97/L98 answer
// the interpreter's error (runtime_defers.tsv's generics and record lines
// deleted, edge-dispatch-3's and module-struct's lowered). make's OTHER
// refusals — an unknown or missing field, a source of the wrong shape —
// are the same class and still bail.
// 51 -> 7 on 2026-09-26 (the plain-error disposition, the retired-node
// replay, the type-name reach): forty rows were a handler's plain Go error
// (convert, make, a predicate type's rejection, the module words' own
// guards) that compiledRunError booked as a defect while the interpreter
// surfaced it untouched; class.tsv L98–L101 read a node the check pass's
// `undef` retired and the type-install twin did not re-adopt; convert-ideal
// L33 and edge-scalars-3 L218 bailed at vm:poly-no-match for want of a
// faithful-raise plan. Left: flex.tsv L228/L230/L236 (vm:poly-nout-drift),
// edge-quote-1 L28 / edge-quote-3 L56 (a tape-coupled Word from get) and
// fn-value.tsv L317/L318 (NUR190, booked by choice).
// 39 -> 5 on 2026-09-26 (the merge of main's #510 with the reverse-order
// NUR run): main's 51 -> 7 above and the NUR run's 51 -> 39 overlap (the
// plain-error disposition subsumes the fnpred and make-refusal rows the run
// had already moved), and the run's NUR190 close takes fn-value.tsv
// L317/L318 off main's seven. Left: flex.tsv L228/L230/L236
// (vm:poly-nout-drift) and edge-quote-1 L28 / edge-quote-3 L56 (a
// tape-coupled Word from get).

// 7 -> 2 on 2026-09-26 (the re-stepped word node, the flex write shapes):
// edge-quote-1 L28 / edge-quote-3 L56 compile the interpreter's re-step of a
// word node read out of a list (the read emits nothing, the word's own
// dispatch records — here add's no-match trap), and flex.tsv L228/L230/L236
// commit the FlexMap `set` through the member's recorded write shape. Left:
// fn-value.tsv L317/L318 (NUR190, booked by choice).
// 5 -> 0 on 2026-09-26 (the merge of main's #511 with the reverse-order
// NUR run, measured on the merged tree): main's 7 -> 2 above closes exactly
// the run's five (edge-quote-1 L28 / edge-quote-3 L56, flex.tsv
// L228/L230/L236), and the two main left (fn-value.tsv L317/L318) the run
// had already closed with NUR190. Left: none.
const bailDefectCeiling = 0

var bailDefects = struct {
	mu      sync.Mutex
	rows    map[string]string
	reasons map[string]int
}{rows: map[string]string{}, reasons: map[string]int{}}

// compiledDefect reports whether errC is the compiled lane reporting a
// COMPILER defect rather than the program's own result. The marker is
// compiledRunError's note, which only the compiled runtime attaches — an
// internal_error the PROGRAM raised carries no such note.
//
// It only CLASSIFIES. Counting is bookCompiledDefect's job and has exactly
// one caller, the corpus walk, so the ceiling means one thing: corpus rows.
// Every other gate asks this question about rows the corpus walk has already
// counted, and a second booking would inflate the number without naming a
// second defect.
func compiledDefect(errC error) bool {
	var ae *core.BoruError
	if !errors.As(errC, &ae) || ae.Code != "internal_error" {
		return false
	}
	for _, n := range ae.Notes {
		if strings.Contains(n, "this is a compiler defect") {
			return true
		}
	}
	return false
}

// bookCompiledDefect classifies and COUNTS. The corpus walk
// (TestSpecCompiledOrFallback) is its only caller.
func bookCompiledDefect(key, detail string, errC error) bool {
	if !compiledDefect(errC) {
		return false
	}
	var ae *core.BoruError
	errors.As(errC, &ae)
	bailDefects.mu.Lock()
	defer bailDefects.mu.Unlock()
	bailDefects.rows[key] = detail
	bailDefects.reasons[bailReasonOf(ae.Detail)]++
	return true
}

// bailDetailOf is bailReasonOf over an error: the surfaced BoruError's
// detail without its position, or the error text when it is not one.
func bailDetailOf(err error) string {
	var ae *core.BoruError
	if errors.As(err, &ae) {
		return bailReasonOf(ae.Detail)
	}
	return bailReasonOf(err.Error())
}

// bailReasonOf strips a bail's position suffix so the ledger groups by site
// rather than by source location.
func bailReasonOf(detail string) string {
	if i := strings.Index(detail, " (pc="); i >= 0 {
		detail = detail[:i]
	}
	return detail
}

// assertBailDefectLedger is called by the corpus walk once it has finished.
// A FILTERED walk (BORU_SPEC_FILES over one family) sees a subset and would
// undercount, so it only REPORTS there — the same rule the per-file
// compile-failure ledger applies to its corpus-wide sum.
func assertBailDefectLedger(t testing.TB) {
	t.Helper()
	report := bailDefectReport()
	if os.Getenv("BORU_SPEC_FILES") != "" {
		t.Logf("%s  (a filtered walk: reported, not asserted)", report)
		return
	}
	bailDefects.mu.Lock()
	n := len(bailDefects.rows)
	bailDefects.mu.Unlock()
	if n != bailDefectCeiling {
		t.Error(report)
		return
	}
	t.Log(report)
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
