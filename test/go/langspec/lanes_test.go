// lanes_test.go — the corpus filter and the two-lane gates.
//
// Two velocity mechanisms, both switched by environment so that no gate's
// DEFAULT behaviour changes silently:
//
//   - BORU_SPEC_FILES=callbacks.tsv,fold-*.tsv restricts every corpus walk
//     to the named lang/spec files (comma-separated basenames or globs). A
//     gate over one family then takes seconds instead of the whole
//     8,500-row walk, and reports per row exactly as it would over the
//     corpus. Under the filter every ABSOLUTE count — a ceiling, a floor, a
//     both-ways ledger — is reported and not asserted, because a subset has
//     no meaningful count; the per-row verdicts are what the filter is for.
//     A name or glob that selects no file is an error (specEntries), never
//     an empty, passing walk.
//
//   - BORU_DIRECTION_GATES=1 arms the DIRECTION lane. Every ratchet in this
//     package has two numbers: the END STATE the design names (type-soundness
//     violations 0, …) and the REGRESSION CEILING, the live value at the
//     last merge, which only falls. The regression lane — the default, what
//     `make test` and the per-PR CI run — fails when a value RISES above its
//     ceiling (a change put debt back) or, for the both-ways ratchets, FALLS
//     below it (the ratchet tightened and must be lowered so it keeps
//     catching regressions). The direction lane additionally fails while a
//     value is above its end state: it is red by design until the work is
//     done, `make test-direction` runs it, and CI renders its table from the
//     regression shards on every run (the gate-table job).
//
//     Raising a regression ceiling is never a fix — it is the record that a
//     change added debt, and it is done only with the row that added it
//     named in the constant's comment, the way every ceiling here already
//     is. Raising an END STATE is not possible: they are the design's.
//
// A gate writes one line per call, and — when BORU_GATE_SUMMARY names a
// file — appends a Markdown table row to it, which is how every CI shard
// contributes to the live gate table in the run's summary and how
// `make gate-status` refreshes test/go/langspec/GATE_STATUS.md.
package langspec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// specEntries lists the lang/spec files a corpus walk visits: every
// os.ReadDir entry, or — under BORU_SPEC_FILES — only the entries whose
// basename matches one of the comma-separated names or globs. Every walk in
// this package reads the directory through it, so the filter has one seam.
// A pattern that selects no file is an error, as is one that is not a
// pattern: a misspelt name or glob would otherwise walk nothing, assert
// nothing and pass, which is exactly the silent green the per-file ledger
// exists to end (found in review of PR #472).
func specEntries(specDir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(specDir) // the one direct read; every walk goes through this function
	if err != nil {
		return nil, err
	}
	pats := specFilePatterns()
	if len(pats) == 0 {
		return entries, nil
	}
	kept := entries[:0:0]
	selected := make([]bool, len(pats))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		keep := false
		for i, p := range pats {
			ok, err := filepath.Match(p, e.Name())
			if err != nil {
				return nil, fmt.Errorf("BORU_SPEC_FILES: %q is not a file name or glob: %w", p, err)
			}
			if ok || p == e.Name() {
				selected[i] = true
				keep = true
			}
		}
		if keep {
			kept = append(kept, e)
		}
	}
	for i, p := range pats {
		if !selected[i] {
			return nil, fmt.Errorf("BORU_SPEC_FILES: %q selects no file in %s — a misspelt name or glob walks nothing, and a walk of nothing must not pass", p, specDir)
		}
	}
	return kept, nil
}

// specFilePatterns parses BORU_SPEC_FILES; nil when unset or blank.
func specFilePatterns() []string {
	raw := strings.TrimSpace(os.Getenv("BORU_SPEC_FILES"))
	if raw == "" {
		return nil
	}
	var pats []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			pats = append(pats, p)
		}
	}
	return pats
}

// filteredCorpus reports whether a corpus filter is active — the condition
// under which absolute counts are reported rather than asserted.
func filteredCorpus() bool { return len(specFilePatterns()) > 0 }

// directionGates reports whether the direction lane is armed.
func directionGates() bool { return os.Getenv("BORU_DIRECTION_GATES") != "" }

var gateSummaryMu sync.Mutex

// gate checks one ratchet against both of its numbers.
//
// got is the live value; end is the END STATE (the design's number, asserted
// only in the direction lane); ceiling is the REGRESSION ceiling (the last
// merged value, asserted always). bothWays makes a FALL below the ceiling a
// failure too: a ceiling that sits above the live value cannot catch the
// next regression,
// so it is lowered in the change that lowered the value. why is the one-line
// account of what the number measures, for the summary.
func gate(t testing.TB, name string, got, end, ceiling int, bothWays bool, why string) {
	t.Helper()
	gateAssert(t, name, got, end, ceiling, bothWays, why, !filteredCorpus())
}

// gateAssert is gate with the assertion decided by the caller: a count that
// does not depend on the spec corpus — the generated sweep's — asserts
// under a corpus filter too, where a corpus count only reports.
func gateAssert(t testing.TB, name string, got, end, ceiling int, bothWays bool, why string, assert bool) {
	t.Helper()
	status := "at end state"
	switch {
	case got > ceiling:
		status = "REGRESSION"
	case bothWays && got < ceiling:
		status = "ratchet tightened"
	case got > end:
		status = "open"
	}
	if !assert {
		status += " (filtered: not asserted)"
	}
	t.Logf("gate %s: %d (end state %d, regression ceiling %d) — %s", name, got, end, ceiling, status)
	appendGateSummary(name, got, end, ceiling, status, why)
	if !assert {
		return // a subset's count is not the corpus's
	}
	if got > ceiling {
		t.Errorf("gate %s: %d exceeds the regression ceiling %d — a change put this debt BACK; find the rows that moved it (%s)", name, got, ceiling, why)
		return
	}
	if bothWays && got < ceiling {
		t.Errorf("gate %s: %d is BELOW the regression ceiling %d — the ratchet tightened; lower the ceiling to %d in this change so it keeps catching regressions", name, got, ceiling, got)
	}
	if directionGates() && got > end {
		t.Errorf("gate %s: %d is above the END STATE %d — open debt on the direction lane (%s)", name, got, end, why)
	}
}

// appendGateSummary adds one Markdown row to BORU_GATE_SUMMARY, if set.
func appendGateSummary(name string, got, end, ceiling int, status, why string) {
	path := os.Getenv("BORU_GATE_SUMMARY")
	if path == "" {
		return
	}
	gateSummaryMu.Lock()
	defer gateSummaryMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "| %s | %d | %d | %d | %s | %s |\n", name, got, end, ceiling, status, why)
}
