package langspec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFuzzSeedBase: the seed base is a non-negative integer, and anything
// else falls back to 0 — the default corpus, never a silently odd seed.
func TestFuzzSeedBase(t *testing.T) {
	for _, c := range []struct {
		env  string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"2026101900", 2026101900},
		// rejected: negative, fractional, not a number
		{"-5", 0},
		{"1.5", 0},
		{"tomorrow", 0},
	} {
		t.Setenv("BORU_FUZZ_SEED_BASE", c.env)
		if got := fuzzSeedBase(); got != c.want {
			t.Errorf("BORU_FUZZ_SEED_BASE=%q: got %d, want %d", c.env, got, c.want)
		}
	}
}

// errTB records Errorf instead of failing, so a test can assert that a call
// reported an error.
type errTB struct {
	testing.TB
	errs []string
}

func (e *errTB) Errorf(format string, args ...any) {
	e.errs = append(e.errs, fmt.Sprintf(format, args...))
}

// TestRecordFuzz: with BORU_FUZZ_FINDINGS set, every record is one JSON
// line, appended, a zero iteration included; unset, nothing is written; and
// a path that cannot be written fails the test rather than dropping the
// finding.
func TestRecordFuzz(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "findings.jsonl")
	t.Setenv("BORU_FUZZ_FINDINGS", path)
	recordFuzz(t, map[string]any{"kind": "internal_error", "test": "T", "seed": int64(2026101907),
		"iter": 0, "program": `"a" add 1`, "min": `"a" add 1`, "error": "boom"})
	recordFuzz(t, map[string]any{"kind": "summary", "test": "T", "failures": 1})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), data)
	}
	var finding map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &finding); err != nil {
		t.Fatalf("line 1 is not JSON: %v", err)
	}
	if finding["kind"] != "internal_error" || finding["seed"] != float64(2026101907) ||
		finding["iter"] != float64(0) || finding["min"] != `"a" add 1` || finding["error"] != "boom" {
		t.Errorf("finding round-tripped wrong: %v", finding)
	}

	// Unset: nothing is written, the earlier file untouched.
	t.Setenv("BORU_FUZZ_FINDINGS", "")
	recordFuzz(t, map[string]any{"kind": "summary"})
	if after, _ := os.ReadFile(path); string(after) != string(data) {
		t.Errorf("an unset BORU_FUZZ_FINDINGS wrote: %q", after)
	}

	// Unwritable: the record is reported, not dropped.
	t.Setenv("BORU_FUZZ_FINDINGS", filepath.Join(dir, "missing", "findings.jsonl"))
	tb := &errTB{TB: t}
	recordFuzz(tb, map[string]any{"kind": "summary"})
	if len(tb.errs) != 1 {
		t.Errorf("an unwritable BORU_FUZZ_FINDINGS reported %d errors, want 1: %v", len(tb.errs), tb.errs)
	}
}
