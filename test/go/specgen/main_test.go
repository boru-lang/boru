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
	"flag"
	"fmt"
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

func TestClassifyFrontierKnownInputs(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)

	cases := []struct {
		input        string
		wantClass    frontierClass
		wantDetail   string // substring of the detail
		wantCompiled bool
	}{
		// Unparseable → check stage, before anything can compile.
		{"'unterminated", classCheck, "parse error", false},
		// An error-severity diagnostic → check stage, with the stable
		// diagnostic code as the detail; never emitted as a Program.
		{"frobnicate", classCheck, "undefined_word", false},
		// A function word that strands a parked forward under the strict
		// forward barrier → check stage (`not` cannot feed off `dup`).
		{"0 not dup", classCheck, "check", false},
		// Checks clean but the compiler declines it (an off-corpus shape — a
		// `def` consuming a variadic loop region with a DYNAMIC count; the
		// S5 split needs the static region size, so this stays the stable
		// declining fixture now that the statically-counted sibling
		// graduated 2026-07-17 via the S5 first-value split).
		{`def m {n: 3} def xs (for (m get "n") [1]) xs`, classCompile, "consumes loop results", false},
		// Checks clean AND compiles, yet errors at run.
		{"0 true not lt", classRuntime, "incomparable", true},
		// A genuinely passing program reaches the runtime stage and is
		// tagged as a non-passing *value* (the extractor rejected it).
		{"0 1 add", classRuntime, "non-passing-value", true},
	}
	for _, c := range cases {
		cl, detail, compiled := classifyFrontier(a, c.input)
		if cl != c.wantClass {
			t.Errorf("classifyFrontier(%q) class = %d, want %d (detail %q)", c.input, cl, c.wantClass, detail)
		}
		if !strings.Contains(detail, c.wantDetail) {
			t.Errorf("classifyFrontier(%q) detail = %q, want substring %q", c.input, detail, c.wantDetail)
		}
		if compiled != c.wantCompiled {
			t.Errorf("classifyFrontier(%q) compiled = %v, want %v", c.input, compiled, c.wantCompiled)
		}
	}
}

func TestFreshDivergence(t *testing.T) {
	// Interpreter and compiler agree on a passing program.
	if note, real := freshDivergence("0 1 add"); real {
		t.Errorf("freshDivergence(%q) reported a divergence: %q", "0 1 add", note)
	}
	// A program the compiler declines in isolation can never be a
	// compiler bug.
	if note, real := freshDivergence("0 not dup"); real {
		t.Errorf("freshDivergence(%q) reported a divergence: %q", "0 not dup", note)
	}
	// Both stages erroring is agreement, not divergence.
	if note, real := freshDivergence("dup"); real {
		t.Errorf("freshDivergence(%q) reported a divergence: %q", "dup", note)
	}
}

// ---- end-to-end pipeline over a tiny universe -------------------------------

// runSpecgenMain invokes main() with the given command line. main()
// registers its flags on the global flag.CommandLine, so a fresh FlagSet
// is swapped in for each invocation (and restored afterwards) to allow
// repeated calls within one test binary.
func runSpecgenMain(t *testing.T, args ...string) {
	t.Helper()
	oldArgs, oldFlags := os.Args, flag.CommandLine
	defer func() { os.Args, flag.CommandLine = oldArgs, oldFlags }()
	flag.CommandLine = flag.NewFlagSet("specgen", flag.ExitOnError)
	os.Args = append([]string{"specgen"}, args...)
	main()
}

// TestGeneratorPipelineEndToEnd drives every specgen mode over the tiny
// `-max 2` universe (19 + 361 = 380 sequences) into a temp dir and
// asserts deterministic structural properties: row counts, known-row
// classifications carried verbatim in the checked-in matrix, byte-level
// determinism, and that the derived files partition the space exactly.
// The phases share one temp dir because each mode consumes the previous
// mode's output, exactly like the Makefile's spec-gen target.
func TestGeneratorPipelineEndToEnd(t *testing.T) {
	dir := t.TempDir()

	full := filepath.Join(dir, "full.tsv")
	runSpecgenMain(t, "-max", "2", "-out", full)
	fullByInput := checkFullMatrix(t, full)

	checkGenerationDeterminism(t, dir)

	passing := filepath.Join(dir, "passing.tsv")
	runSpecgenMain(t, "-extract", "-in", full, "-out", passing)
	passSet := checkPassingSubset(t, passing, fullByInput)

	pass1 := filepath.Join(dir, "passing-len1.tsv")
	extractPassing(full, pass1, 1)
	pass1Set := checkLengthWindow(t, pass1, passSet)

	checkFrontier(t, dir, passing, passSet)
	checkExtendLayer(t, dir, passing, pass1, passSet, pass1Set)
}

// checkFullMatrix verifies the generated full matrix: row count, unique
// inputs, per-length split, known rows, and the provenance header. It
// returns the expected column indexed by input for the later phases.
func checkFullMatrix(t *testing.T, full string) map[string]string {
	t.Helper()
	rows, err := readDataRows(full)
	if err != nil {
		t.Fatalf("readDataRows(full): %v", err)
	}
	wantRows := len(alphabet) + len(alphabet)*len(alphabet)
	if len(rows) != wantRows {
		t.Fatalf("full matrix has %d data rows, want %d", len(rows), wantRows)
	}
	fullByInput := make(map[string]string, len(rows))
	n1, n2 := 0, 0
	for _, r := range rows {
		if _, dup := fullByInput[r.input]; dup {
			t.Fatalf("duplicate input %q in full matrix", r.input)
		}
		fullByInput[r.input] = r.expected
		switch r.n {
		case 1:
			n1++
		case 2:
			n2++
		default:
			t.Fatalf("row %q has element count %d, want 1 or 2", r.input, r.n)
		}
	}
	if n1 != len(alphabet) || n2 != len(alphabet)*len(alphabet) {
		t.Errorf("element-count split = %d/%d, want %d/%d", n1, n2, len(alphabet), len(alphabet)*len(alphabet))
	}

	// Known rows, carried verbatim in the checked-in matrix — the tiny
	// universe must agree with the frozen contract on its shared rows.
	known := map[string]string{
		"0":      "0",
		"[1 2]":  "[1 2]",
		"dup":    "ERROR:signature_error",
		"0 dup":  "0 0",
		"0 drop": "",
		"0 not":  "true",
		"0 add":  "ERROR:signature_error",
	}
	for in, want := range known {
		got, ok := fullByInput[in]
		if !ok {
			t.Errorf("full matrix is missing row %q", in)
			continue
		}
		if got != want {
			t.Errorf("full matrix row %q = %q, want %q", in, got, want)
		}
	}

	header := readAll(t, full)
	for _, want := range []string{
		"# boru Language Specification: Syntax Combination Matrix (GENERATED)",
		fmt.Sprintf("# Alphabet (%d atoms): %s", len(alphabet), strings.Join(alphabet, " ")),
		"# §1  1-element sequences",
		"# §2  2-element sequences",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("full matrix missing header line %q", want)
		}
	}
	return fullByInput
}

// checkGenerationDeterminism generates the length-1 matrix twice — once
// via -out, once through the stdout path — and requires identical bytes.
func checkGenerationDeterminism(t *testing.T, dir string) {
	t.Helper()
	g1 := filepath.Join(dir, "g1.tsv")
	runSpecgenMain(t, "-max", "1", "-out", g1)
	g2 := filepath.Join(dir, "g2.tsv")
	func() {
		fout, ferr := os.Create(g2)
		if ferr != nil {
			t.Fatalf("create %s: %v", g2, ferr)
		}
		orig := os.Stdout
		os.Stdout = fout
		defer func() {
			os.Stdout = orig
			if cerr := fout.Close(); cerr != nil {
				t.Fatalf("close %s: %v", g2, cerr)
			}
		}()
		runSpecgenMain(t, "-max", "1") // no -out → stdout
	}()
	if readAll(t, g1) != readAll(t, g2) {
		t.Errorf("generation is not deterministic: file mode and stdout mode differ")
	}
}

// checkPassingSubset verifies the extracted passing subset against the
// full matrix and returns its input set.
func checkPassingSubset(t *testing.T, passing string, fullByInput map[string]string) map[string]bool {
	t.Helper()
	passSet, err := readInputSet(passing)
	if err != nil {
		t.Fatalf("readInputSet(passing): %v", err)
	}
	if len(passSet) == 0 {
		t.Fatalf("passing subset is empty — extraction produced nothing")
	}
	for _, r := range decodeFile(t, passing) {
		fullExpected, ok := fullByInput[r.input]
		if !ok {
			t.Errorf("passing row %q is not a full-matrix input", r.input)
			continue
		}
		if r.expected != fullExpected {
			t.Errorf("passing row %q expected %q, full matrix says %q", r.input, r.expected, fullExpected)
		}
		if strings.HasPrefix(fullExpected, "ERROR:") {
			t.Errorf("passing subset contains interpreter-error row %q", r.input)
		}
	}
	if !passSet["0"] {
		t.Errorf("passing subset is missing the literal %q", "0")
	}
	if passSet["dup"] {
		t.Errorf("passing subset contains %q, an interpreter-error row", "dup")
	}
	if !strings.Contains(readAll(t, passing), frontCodeMarker) {
		t.Errorf("passing subset is missing the %q header marker", frontCodeMarker)
	}
	return passSet
}

// checkLengthWindow verifies the maxLen=1 extraction: a strict, non-empty
// subset of the unwindowed passing set containing only single atoms.
func checkLengthWindow(t *testing.T, pass1 string, passSet map[string]bool) map[string]bool {
	t.Helper()
	pass1Set, err := readInputSet(pass1)
	if err != nil {
		t.Fatalf("readInputSet(pass1): %v", err)
	}
	atomSet := make(map[string]bool, len(alphabet))
	for _, a := range alphabet {
		atomSet[a] = true
	}
	for in := range pass1Set {
		if !atomSet[in] {
			t.Errorf("length-1 passing subset contains multi-element row %q", in)
		}
		if !passSet[in] {
			t.Errorf("length-1 passing row %q missing from the unwindowed subset", in)
		}
	}
	if len(pass1Set) == 0 || len(pass1Set) >= len(passSet) {
		t.Errorf("length-1 window kept %d of %d rows, want a strict non-empty subset", len(pass1Set), len(passSet))
	}
	return pass1Set
}

// checkFrontier runs frontier extraction and verifies the three buckets
// partition exactly the independently recomputed set of minimal failing
// prefixes (fails while its immediate prefix passes).
func checkFrontier(t *testing.T, dir, passing string, passSet map[string]bool) {
	t.Helper()
	ck := filepath.Join(dir, "fail-check.tsv")
	co := filepath.Join(dir, "fail-compile.tsv")
	ro := filepath.Join(dir, "fail-runtime.tsv")
	runSpecgenMain(t, "-frontier", "-max", "2",
		"-passing", passing, "-check-out", ck, "-compile-out", co, "-runtime-out", ro)

	wantFrontier := map[string]bool{}
	for _, a := range alphabet {
		if !passSet[a] {
			wantFrontier[a] = true // the empty prefix trivially passes
		}
	}
	for _, a := range alphabet {
		if !passSet[a] {
			continue // prefix already failed — not a minimal failing prefix
		}
		for _, b := range alphabet {
			if s := a + " " + b; !passSet[s] {
				wantFrontier[s] = true
			}
		}
	}

	gotFrontier := map[string]string{} // input → source bucket
	for _, bucket := range []struct{ name, path string }{
		{"check", ck}, {"compile", co}, {"runtime", ro},
	} {
		if !strings.Contains(readAll(t, bucket.path), "prefixes.") {
			t.Errorf("%s frontier file missing its header count line", bucket.name)
		}
		for _, r := range decodeFile(t, bucket.path) {
			if prevBucket, dup := gotFrontier[r.input]; dup {
				t.Errorf("frontier prefix %q appears in both %s and %s", r.input, prevBucket, bucket.name)
			}
			gotFrontier[r.input] = bucket.name
			if r.expected == "" {
				t.Errorf("frontier prefix %q (bucket %s) has an empty detail", r.input, bucket.name)
			}
		}
	}
	if len(gotFrontier) != len(wantFrontier) {
		t.Errorf("frontier size = %d, want %d", len(gotFrontier), len(wantFrontier))
	}
	for in := range wantFrontier {
		if _, ok := gotFrontier[in]; !ok {
			t.Errorf("frontier is missing minimal failing prefix %q", in)
		}
	}
	for in := range gotFrontier {
		if !wantFrontier[in] {
			t.Errorf("frontier contains %q, which is not a minimal failing prefix", in)
		}
	}
}

// checkExtendLayer runs the extend-by-one mode and verifies its five
// buckets partition exactly the candidates (every length-2 passing root
// extended by one atom), that any mismatch row reproduces on a fresh
// instance, and that passing rows record the canonical interpreter value.
func checkExtendLayer(t *testing.T, dir, passing, pass1 string, passSet, pass1Set map[string]bool) {
	t.Helper()
	p5 := filepath.Join(dir, "ext-passing.tsv")
	ck5 := filepath.Join(dir, "ext-fail-check.tsv")
	co5 := filepath.Join(dir, "ext-fail-compile.tsv")
	ro5 := filepath.Join(dir, "ext-fail-runtime.tsv")
	mm5 := filepath.Join(dir, "ext-mismatch.tsv")
	runSpecgenMain(t, "-extend5",
		"-passing", passing, "-len123", pass1,
		"-pass-out", p5, "-check-out", ck5, "-compile-out", co5,
		"-runtime-out", ro5, "-mismatch-out", mm5)

	wantCands := map[string]bool{}
	for in := range passSet {
		if pass1Set[in] {
			continue
		}
		for _, a := range alphabet {
			wantCands[in+" "+a] = true
		}
	}

	gotCands := map[string]string{} // input → bucket
	passExpected := map[string]string{}
	for _, bucket := range []struct{ name, path string }{
		{"pass", p5}, {"check", ck5}, {"compile", co5}, {"runtime", ro5}, {"mismatch", mm5},
	} {
		for _, r := range decodeFile(t, bucket.path) {
			if prevBucket, dup := gotCands[r.input]; dup {
				t.Errorf("extended row %q appears in both %s and %s", r.input, prevBucket, bucket.name)
			}
			gotCands[r.input] = bucket.name
			if bucket.name == "pass" {
				passExpected[r.input] = r.expected
			}
		}
	}
	if len(gotCands) != len(wantCands) {
		t.Errorf("extend layer covers %d candidates, want %d", len(gotCands), len(wantCands))
	}
	for in := range wantCands {
		if _, ok := gotCands[in]; !ok {
			t.Errorf("extend layer is missing candidate %q", in)
		}
	}

	// Any fresh-verified compiler/interpreter divergence must reproduce on
	// an isolated instance — an empty mismatch bucket is the healthy state.
	for in, bucket := range gotCands {
		if bucket != "mismatch" {
			continue
		}
		if note, real := freshDivergence(in); !real {
			t.Errorf("mismatch row %q does not reproduce on a fresh instance (note %q)", in, note)
		}
	}

	if len(passExpected) == 0 {
		t.Fatalf("extend layer kept no passing rows")
	}
	if got, ok := passExpected["0 0 0"]; !ok {
		t.Errorf("extend layer passing bucket is missing %q", "0 0 0")
	} else if got != "0 0 0" {
		t.Errorf("extend row %q expected %q, want %q", "0 0 0", got, "0 0 0")
	}
	checked := 0
	for in, want := range passExpected {
		if checked >= 5 {
			break
		}
		checked++
		stack, rerr := run(in)
		if rerr != nil {
			t.Errorf("extend passing row %q does not run: %v", in, rerr)
			continue
		}
		if got := core.Canon(stack); got != want {
			t.Errorf("extend passing row %q = %q, want recorded %q", in, got, want)
		}
	}
}

// TestExtractPassingSkipsNonQualifyingRows feeds extractPassing a
// handcrafted full-matrix file in which each row fails a DIFFERENT gate,
// asserting only the three-way-clean row survives: error rows are never
// passing, a runtime failure fails gate 1 (interpret), a checker error
// fails gate 2 (check), and a compiler compile failure fails gate 3 (compile).
// maxLen=0 exercises the unwindowed whole-matrix path.
func TestExtractPassingSkipsNonQualifyingRows(t *testing.T) {
	in := writeTempSpec(t, "mini-matrix.tsv", strings.Join([]string{
		"# handcrafted mini matrix",
		"0\t0\t1-elem → 1 value(s)",                   // passes all three gates
		"dup\tERROR:signature_error\t1-elem rejected", // error row: never passing
		"0 true not lt\tbogus\t4-elem → 1 value(s)",   // errors at run (incomparable)
		"[None] get 0\t[None]\t3-elem → 1 value(s)",   // runs clean, but the checker errors
		"def One (typeof (const 1)) end 1 is One\ttrue\tchecks \u0026 runs clean, but the compiler fails on it", // checks clean, but the compiler declines
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
