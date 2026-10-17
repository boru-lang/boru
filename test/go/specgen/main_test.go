// Unit and end-to-end tests for the specgen generator. These are
// deliberately UNtagged (unlike syntax_matrix_test.go, which carries the
// `specgen` build tag and replays the full ~137k-row matrix) so they run
// in the ordinary `go test ./...` sweep: they exercise the generator's
// pure helpers directly and drive every CLI mode end-to-end over a tiny
// `-max 2` universe (380 sequences) into t.TempDir(), asserting
// deterministic structural properties rather than golden-copying the
// checked-in matrices.
//
// NOT covered here (would need a seam the production code does not
// expose): the os.Exit error paths — missing-flag usage errors in main()
// and unreadable-input/uncreatable-output exits in the mode functions.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// ---- pure helpers ---------------------------------------------------------

func TestEachSeqOrderAndCount(t *testing.T) {
	var seqs []string
	eachSeq(2, func(toks []string) { seqs = append(seqs, strings.Join(toks, " ")) })

	wantN := len(alphabet) + len(alphabet)*len(alphabet) // 19 + 361
	if len(seqs) != wantN {
		t.Fatalf("eachSeq(2) yielded %d sequences, want %d", len(seqs), wantN)
	}

	// Shorter sequences first: the first 19 are exactly the alphabet, in
	// order; then odometer order with the last position varying fastest.
	for i, a := range alphabet {
		if seqs[i] != a {
			t.Errorf("seqs[%d] = %q, want alphabet atom %q", i, seqs[i], a)
		}
	}
	if got, want := seqs[len(alphabet)], alphabet[0]+" "+alphabet[0]; got != want {
		t.Errorf("first 2-element sequence = %q, want %q", got, want)
	}
	if got, want := seqs[len(alphabet)+1], alphabet[0]+" "+alphabet[1]; got != want {
		t.Errorf("second 2-element sequence = %q, want %q", got, want)
	}
	last := alphabet[len(alphabet)-1]
	if got, want := seqs[len(seqs)-1], last+" "+last; got != want {
		t.Errorf("last sequence = %q, want %q", got, want)
	}

	// Fully deterministic and duplicate-free.
	seen := make(map[string]bool, len(seqs))
	for _, s := range seqs {
		if seen[s] {
			t.Fatalf("duplicate sequence %q", s)
		}
		seen[s] = true
	}
	var again []string
	eachSeq(2, func(toks []string) { again = append(again, strings.Join(toks, " ")) })
	for i := range seqs {
		if seqs[i] != again[i] {
			t.Fatalf("eachSeq not deterministic at %d: %q vs %q", i, seqs[i], again[i])
		}
	}
}

func TestSequencesJoinsWithElementCount(t *testing.T) {
	var got []string
	sequences(1, func(src string, n int) {
		if n != 1 {
			t.Errorf("sequences(1) reported n=%d for %q, want 1", n, src)
		}
		got = append(got, src)
	})
	if len(got) != len(alphabet) {
		t.Fatalf("sequences(1) yielded %d entries, want %d", len(got), len(alphabet))
	}
	for i, a := range alphabet {
		if got[i] != a {
			t.Errorf("sequences(1)[%d] = %q, want %q", i, got[i], a)
		}
	}
}

func TestFrontCode(t *testing.T) {
	cases := []struct {
		prev, cur  string
		wantReuse  int
		wantSuffix string
	}{
		{"", "abc", 0, "abc"},
		{"abc", "abd", 2, "d"},
		{"abc", "abc", 3, ""},
		{"abcdef", "abc", 3, ""},
		{"xyz", "abc", 0, "abc"},
		{"0 dup", "0 drop", 3, "rop"},
	}
	for _, c := range cases {
		reuse, suffix := frontCode(c.prev, c.cur)
		if reuse != c.wantReuse || suffix != c.wantSuffix {
			t.Errorf("frontCode(%q, %q) = (%d, %q), want (%d, %q)",
				c.prev, c.cur, reuse, suffix, c.wantReuse, c.wantSuffix)
		}
		// Round trip: prev[:reuse]+suffix reconstructs cur.
		if got := c.prev[:reuse] + suffix; got != c.cur {
			t.Errorf("frontCode(%q, %q) does not round-trip: got %q", c.prev, c.cur, got)
		}
	}
}

func TestErrorClass(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{errors.New("[boru/div_by_zero]: divided by zero"), "ERROR:div_by_zero"},
		{errors.New("[boru/signature_error]: no matching signature"), "ERROR:signature_error"},
		{errors.New("[boru/x] terse"), "ERROR:x"},
		// An empty code (']' immediately after the prefix) is not stable —
		// falls back to the generic sentinel.
		{errors.New("[boru/]: empty code"), "ERROR:"},
		{errors.New("plain error text"), "ERROR:"},
	}
	for _, c := range cases {
		if got := errorClass(c.err); got != c.want {
			t.Errorf("errorClass(%q) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestSanitizeNote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"tab\tand\nnewline", "tab and newline"},
		{"", "uncompilable"},
	}
	for _, c := range cases {
		if got := sanitizeNote(c.in); got != c.want {
			t.Errorf("sanitizeNote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderAny(t *testing.T) {
	if got := renderAny(nil); got != "" {
		t.Errorf("renderAny(nil) = %q, want empty", got)
	}
	if got, want := renderAny([]any{int64(1), "x", true}), "1 x true"; got != want {
		t.Errorf("renderAny = %q, want %q", got, want)
	}
}

// TestRunKnownSequences pins run() — the generator's evaluation recipe —
// against rows carried verbatim in the checked-in matrix, so the recipe
// and the frozen contract stay in lockstep.
func TestRunKnownSequences(t *testing.T) {
	cases := []struct{ input, want string }{
		{"0", "0"},
		{"[1 2]", "[1 2]"},
		{"0 not", "true"},
		{"0 dup", "0 0"},
		{"0 1 add", "1"},
		{"1 2.5 add", "3.5"},
		{"0 1 swap", "1 0"},
		{"true false and", "false"},
		{"[1 2] size", "2"},
	}
	for _, c := range cases {
		stack, err := run(c.input)
		if err != nil {
			t.Errorf("run(%q): unexpected error %v", c.input, err)
			continue
		}
		if got := core.Canon(stack); got != c.want {
			t.Errorf("run(%q) = %q, want %q", c.input, got, c.want)
		}
	}

	// Error rows pin the error class, not the message.
	if _, err := run("dup"); err == nil {
		t.Errorf("run(%q): want error, got success", "dup")
	} else if got := errorClass(err); got != "ERROR:signature_error" {
		t.Errorf("errorClass(run(%q)) = %q, want ERROR:signature_error", "dup", got)
	}

	// A parse failure surfaces as an error before evaluation.
	if _, err := run("'unterminated"); err == nil {
		t.Errorf("run on an unterminated string literal: want error, got success")
	}
}

// ---- matrix-file decoding -------------------------------------------------

func writeTempSpec(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

type decodedRow struct{ input, expected, note string }

func decodeFile(t *testing.T, path string) []decodedRow {
	t.Helper()
	var rows []decodedRow
	err := forEachDataRow(path, func(input, expected, note string, _ int) {
		rows = append(rows, decodedRow{input, expected, note})
	})
	if err != nil {
		t.Fatalf("forEachDataRow(%s): %v", path, err)
	}
	return rows
}

func TestForEachDataRowPlain(t *testing.T) {
	path := writeTempSpec(t, "plain.tsv", strings.Join([]string{
		"# header comment",
		"",
		"in1\texp1",
		"in2\texp2\tsome note",
		"in3\texp3\t\t", // trailing empty fields are trimmed away
		"   ",           // whitespace-only line is skipped
	}, "\n")+"\n")

	rows := decodeFile(t, path)
	want := []decodedRow{
		{"in1", "exp1", ""},
		{"in2", "exp2", "some note"},
		{"in3", "exp3", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("decoded %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestForEachDataRowFrontCoded(t *testing.T) {
	path := writeTempSpec(t, "fc.tsv", strings.Join([]string{
		frontCodeMarker,
		"0\t0 dup\t0 0",
		"3\trop", // reuses "0 d" → "0 drop", no extra field
		"2\tnot\ttrue",
	}, "\n")+"\n")

	rows := decodeFile(t, path)
	want := []decodedRow{
		{"0 dup", "0 0", ""},
		{"0 drop", "", ""},
		{"0 not", "true", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("decoded %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestForEachDataRowDetailLegend(t *testing.T) {
	path := writeTempSpec(t, "legend.tsv", strings.Join([]string{
		frontCodeMarker,
		detailLegendMarker,
		"# detail 0: first failure reason",
		"# detail 1: second failure reason",
		"# detail nonsense line without colon-int is ignored",
		"0\tdup\t0",
		"0\tdrop\t1",
		"1\trop\t0", // "d"+"rop" = "drop"... prev is "drop", reuse 1 → "d"+"rop"
	}, "\n")+"\n")

	rows := decodeFile(t, path)
	want := []decodedRow{
		{"dup", "first failure reason", ""},
		{"drop", "second failure reason", ""},
		{"drop", "first failure reason", ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("decoded %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

// TestForEachDataRowRejectsMalformed asserts every malformed-row shape is
// rejected with a diagnostic error rather than silently skipped.
func TestForEachDataRowRejectsMalformed(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"plain row without tab", "just-one-column\n", "malformed row"},
		{"front-coded row without tab", frontCodeMarker + "\nsuffixonly\n", "malformed front-coded row"},
		{"non-numeric reuse", frontCodeMarker + "\nx\tsuffix\n", "bad reuse count"},
		{"negative reuse", frontCodeMarker + "\n-1\tsuffix\n", "bad reuse count"},
		{"reuse beyond previous input", frontCodeMarker + "\n0\tab\n7\tc\n", "bad reuse count"},
		{"non-numeric detail code", frontCodeMarker + "\n" + detailLegendMarker + "\n# detail 0: reason\n0\tin\tzz\n", "bad detail code"},
		{"detail code missing from legend", frontCodeMarker + "\n" + detailLegendMarker + "\n# detail 0: reason\n0\tin\t5\n", "not in legend"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTempSpec(t, "bad.tsv", c.content)
			err := forEachDataRow(path, func(_, _, _ string, _ int) {})
			if err == nil {
				t.Fatalf("forEachDataRow accepted malformed file %q", c.content)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), c.wantErr)
			}
		})
	}

	if err := forEachDataRow(filepath.Join(t.TempDir(), "absent.tsv"), func(_, _, _ string, _ int) {}); err == nil {
		t.Errorf("forEachDataRow on a missing file: want error, got nil")
	}
}

func TestReadDataRowsParsesElementCount(t *testing.T) {
	path := writeTempSpec(t, "counts.tsv", strings.Join([]string{
		"a b c\tx\t3-elem → 1 value(s)",
		"a\tERROR:oops\t1-elem rejected",
		"b\ty\tno leading digit here",
		"c\tz",
	}, "\n")+"\n")

	rows, err := readDataRows(path)
	if err != nil {
		t.Fatalf("readDataRows: %v", err)
	}
	wantN := []int{3, 1, 0, 0}
	if len(rows) != len(wantN) {
		t.Fatalf("got %d rows, want %d", len(rows), len(wantN))
	}
	for i, w := range wantN {
		if rows[i].n != w {
			t.Errorf("row %d (%q): n = %d, want %d", i, rows[i].input, rows[i].n, w)
		}
	}
	if rows[1].expected != "ERROR:oops" {
		t.Errorf("row 1 expected = %q, want ERROR:oops", rows[1].expected)
	}

	if _, err := readDataRows(filepath.Join(t.TempDir(), "absent.tsv")); err == nil {
		t.Errorf("readDataRows on a missing file: want error, got nil")
	}
}

func TestReadInputSet(t *testing.T) {
	path := writeTempSpec(t, "set.tsv", "a\t1\nb c\t2\n")
	set, err := readInputSet(path)
	if err != nil {
		t.Fatalf("readInputSet: %v", err)
	}
	if len(set) != 2 || !set["a"] || !set["b c"] {
		t.Errorf("readInputSet = %v, want {a, b c}", set)
	}
	if _, err := readInputSet(filepath.Join(t.TempDir(), "absent.tsv")); err == nil {
		t.Errorf("readInputSet on a missing file: want error, got nil")
	}
}

// ---- diagnostics helpers ---------------------------------------------------

func TestFirstErrorCodeAndHasErrorDiag(t *testing.T) {
	empty := lang.CheckResult{}
	if hasErrorDiag(empty) {
		t.Errorf("hasErrorDiag on empty result = true, want false")
	}
	if got := firstErrorCode(empty); got != "error" {
		t.Errorf("firstErrorCode on empty result = %q, want \"error\"", got)
	}

	warnOnly := lang.CheckResult{Diagnostics: []lang.CheckDiagnostic{
		{Severity: lang.SeverityWarning, Code: "just_a_warning"},
	}}
	if hasErrorDiag(warnOnly) {
		t.Errorf("hasErrorDiag with warnings only = true, want false")
	}

	coded := lang.CheckResult{Diagnostics: []lang.CheckDiagnostic{
		{Severity: lang.SeverityWarning, Code: "skipped_warning"},
		{Severity: lang.SeverityError, Code: "no_signature"},
		{Severity: lang.SeverityError, Code: "later_error"},
	}}
	if !hasErrorDiag(coded) {
		t.Errorf("hasErrorDiag with an error diagnostic = false, want true")
	}
	if got := firstErrorCode(coded); got != "no_signature" {
		t.Errorf("firstErrorCode = %q, want no_signature (first error-severity code)", got)
	}

	uncoded := lang.CheckResult{Diagnostics: []lang.CheckDiagnostic{
		{Severity: lang.SeverityError, Code: ""},
	}}
	if got := firstErrorCode(uncoded); got != "error" {
		t.Errorf("firstErrorCode with empty code = %q, want \"error\"", got)
	}
}

// ---- frontier classification ----------------------------------------------

// ---- end-to-end pipeline over a tiny universe -------------------------------

// TestExtractPassingSkipsNonQualifyingRows feeds extractPassing a
// handcrafted full-matrix file in which each row fails a DIFFERENT gate,
// asserting only the two-way-clean row survives: error rows are never
// passing, a runtime failure fails gate 1 (interpret), and a checker error
// fails gate 2 (check). maxLen=0 exercises the unwindowed whole-matrix path.
func TestExtractPassingSkipsNonQualifyingRows(t *testing.T) {
	in := writeTempSpec(t, "mini-matrix.tsv", strings.Join([]string{
		"# handcrafted mini matrix",
		"0\t0\t1-elem → 1 value(s)",                   // passes both gates
		"dup\tERROR:signature_error\t1-elem rejected", // error row: never passing
		"0 true not lt\tbogus\t4-elem → 1 value(s)",   // errors at run (incomparable)
		// runs clean, but the checker errors (the untaken branch's undefined
		// word; `[None] get 0` served here until NUR323 made it check clean)
		"def f fn [[b:Boolean][Any][if b [nope] [1]]] end f false\t1\tchecker-only error",
	}, "\n")+"\n")
	out := filepath.Join(t.TempDir(), "mini-passing.tsv")

	extractPassing(in, out, 0)

	kept, err := readInputSet(out)
	if err != nil {
		t.Fatalf("readInputSet(%s): %v", out, err)
	}
	if len(kept) != 1 || !kept["0"] {
		t.Errorf("extractPassing kept %v, want exactly {0}", kept)
	}
	if header := readAll(t, out); !strings.Contains(header, frontCodeMarker) {
		t.Errorf("passing output is missing the %q marker", frontCodeMarker)
	}
}

func readAll(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
