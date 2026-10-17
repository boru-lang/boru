package main

// pipeline_test.go drives every specgen mode end to end and pins the
// frontier classifier and the modes' failure exits — the tests the
// compiler lane's removal cut along with the compile buckets
// (design/COMPILER-LANE-REMOVAL.0.md), restored against the two-bucket
// frontier (type-check, runtime) and the three-bucket extend layer (pass,
// type-check, runtime).

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

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

func TestClassifyFrontierKnownInputs(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)

	cases := []struct {
		input      string
		wantClass  frontierClass
		wantDetail string // substring of the detail
	}{
		// Unparseable → check stage, with the parser's error code.
		{"'unterminated", classCheck, "syntax_error"},
		// An error-severity diagnostic → check stage, with the stable
		// diagnostic code as the detail.
		{"frobnicate", classCheck, "undefined_word"},
		// A function word that strands a parked forward under the strict
		// forward barrier → check stage (`not` cannot feed off `dup`).
		{"0 not dup", classCheck, "signature_error"},
		// Checks clean, yet errors at run.
		{"0 true not lt", classRuntime, "incomparable"},
		// A genuinely passing program reaches the runtime stage and is
		// tagged as a non-passing *value* (the extractor rejected it).
		{"0 1 add", classRuntime, "non-passing-value"},
	}
	for _, c := range cases {
		cl, detail := classifyFrontier(a, c.input)
		if cl != c.wantClass {
			t.Errorf("classifyFrontier(%q) class = %d, want %d (detail %q)", c.input, cl, c.wantClass, detail)
		}
		if !strings.Contains(detail, c.wantDetail) {
			t.Errorf("classifyFrontier(%q) detail = %q, want substring %q", c.input, detail, c.wantDetail)
		}
	}
}

// TestClassifyFrontierBareErrorDetail pins the classifier's arm for a
// runtime error whose message has no [boru/<code>] prefix: the program
// checks clean yet fails at run, errorClass yields the generic "ERROR:",
// and the classifier substitutes the "error" detail.
func TestClassifyFrontierBareErrorDetail(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetClock(specClock)

	cl, detail := classifyFrontier(a, bareErrorProgram)
	if cl != classRuntime {
		t.Fatalf("class = %d, want classRuntime — %s no longer fails at RUN; "+
			"pick a new code-less specimen", cl, bareErrorProgram)
	}
	if detail != "error" {
		t.Errorf("detail = %q, want the generic \"error\" — %s now carries a "+
			"code, so it can no longer stand in for an UNCODED failure; pick a "+
			"new specimen (see bareErrorProgram)", detail, bareErrorProgram)
	}
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

// checkFrontier runs frontier extraction and verifies the two buckets
// partition exactly the independently recomputed set of minimal failing
// prefixes (fails while its immediate prefix passes).
func checkFrontier(t *testing.T, dir, passing string, passSet map[string]bool) {
	t.Helper()
	ck := filepath.Join(dir, "fail-check.tsv")
	ro := filepath.Join(dir, "fail-runtime.tsv")
	runSpecgenMain(t, "-frontier", "-max", "2",
		"-passing", passing, "-check-out", ck, "-runtime-out", ro)

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
		{"check", ck}, {"runtime", ro},
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

// checkExtendLayer runs the extend-by-one mode and verifies its three
// buckets partition exactly the candidates (every length-2 passing root
// extended by one atom), and that passing rows record the canonical
// interpreter value.
func checkExtendLayer(t *testing.T, dir, passing, pass1 string, passSet, pass1Set map[string]bool) {
	t.Helper()
	p5 := filepath.Join(dir, "ext-passing.tsv")
	ck5 := filepath.Join(dir, "ext-fail-check.tsv")
	ro5 := filepath.Join(dir, "ext-fail-runtime.tsv")
	runSpecgenMain(t, "-extend5",
		"-passing", passing, "-len123", pass1,
		"-pass-out", p5, "-check-out", ck5, "-runtime-out", ro5)

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
		{"pass", p5}, {"check", ck5}, {"runtime", ro5},
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

// TestModeIOFailureExits pins each mode's read- and write-side failure
// exits: a missing input or an unwritable output exits 1.
func TestModeIOFailureExits(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.tsv")
	badOut := filepath.Join(dir, "no-such-dir", "out.tsv")

	// A tiny but valid passing file for the write-side failures.
	passing := filepath.Join(dir, "passing.tsv")
	if err := os.WriteFile(passing, []byte("#kept: 1 of 1 scanned (max-len 1)\n1\t1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		fn   func()
		want int
	}{
		{"extractPassing missing -in", func() { extractPassing(missing, badOut, 4) }, 1},
		{"extractPassing bad -out", func() { extractPassing(passing, badOut, 4) }, 1},
		{"extractFrontier missing -passing", func() { extractFrontier(missing, badOut, badOut, 4) }, 1},
		{"extractFrontier bad outputs", func() { extractFrontier(passing, badOut, badOut, 4) }, 1},
		{"extendFive missing -passing", func() { extendFive(missing, missing, badOut, badOut, badOut) }, 1},
		{"extendFive missing -len123", func() { extendFive(passing, missing, badOut, badOut, badOut) }, 1},
		{"extendFive bad outputs", func() { extendFive(passing, passing, badOut, badOut, badOut) }, 1},
		{"writeFrontierFile bad path", func() { writeFrontierFile(badOut, "check", "t", nil, nil, nil, classCheck, 4) }, 1},
		{"writeLen5Pass bad path", func() { writeLen5Pass(badOut, nil, nil, nil) }, 1},
		{"writeLen5Fail bad path", func() { writeLen5Fail(badOut, "check", nil, nil, nil, fiveCheck) }, 1},
	}
	for _, c := range cases {
		if got := exitCode(t, c.fn); got != c.want {
			t.Errorf("%s: exited %d, want %d", c.name, got, c.want)
		}
	}
}

// exitCodeAsync drives fn with osExit swapped to a stub that records
// the code and kills only the CALLING goroutine (runtime.Goexit) — an
// exit inside a worker goroutine must not take the test process down
// with it, and the mode function then drains normally through wg.Wait.
// Returns -1 when nothing exited. fn itself runs on the test goroutine,
// so callers must hand the mode function valid output paths (an exit on
// the test goroutine would end the test as a failure).
func exitCodeAsync(t *testing.T, fn func()) int {
	t.Helper()
	prev := osExit
	t.Cleanup(func() { osExit = prev })
	var code int64 = -1
	osExit = func(c int) {
		atomic.StoreInt64(&code, int64(c))
		runtime.Goexit()
	}
	fn()
	return int(atomic.LoadInt64(&code))
}

func writeTSV(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Worker-goroutine construction failures across all three parallel
// modes, plus the workers<1 floor (numCPU stubbed to 0).
func TestWorkerLangNewFailures(t *testing.T) {
	prevCPU, prevNew := numCPU, langNew
	t.Cleanup(func() { numCPU, langNew = prevCPU, prevNew })
	numCPU = func() int { return 0 } // also drives the single-worker floor
	langNew = func() (*lang.Boru, error) { return nil, errors.New("no lang") }

	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 1 of 1 scanned (max-len 1)\n1\t1\n")
	out := func(n string) string { return filepath.Join(dir, n) }

	if got := exitCodeAsync(t, func() { extractPassing(passing, out("p.tsv"), 4) }); got != 1 {
		t.Errorf("extractPassing worker lang.New failure exited %d, want 1", got)
	}
	if got := exitCodeAsync(t, func() {
		extractFrontier(passing, out("c.tsv"), out("r.tsv"), 1)
	}); got != 1 {
		t.Errorf("extractFrontier worker lang.New failure exited %d, want 1", got)
	}
	if got := exitCodeAsync(t, func() {
		extendFive(passing, passing, out("5p.tsv"), out("5c.tsv"), out("5r.tsv"))
	}); got != 1 {
		t.Errorf("extendFive worker lang.New failure exited %d, want 1", got)
	}
}

// extendFive's worker also constructs a native registry (for canonical
// expected values); its failure arm exits 1.
func TestExtendFiveRegistryFailure(t *testing.T) {
	prevCPU, prevReg := numCPU, newNativeRegistry
	t.Cleanup(func() { numCPU, newNativeRegistry = prevCPU, prevReg })
	numCPU = func() int { return 0 }
	newNativeRegistry = func(...func(*core.Registry)) (*core.Registry, error) {
		return nil, errors.New("no registry")
	}

	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 1 of 1 scanned (max-len 1)\n1\t1\n")
	out := func(n string) string { return filepath.Join(dir, n) }
	if got := exitCodeAsync(t, func() {
		extendFive(passing, passing, out("5p.tsv"), out("5c.tsv"), out("5r.tsv"))
	}); got != 1 {
		t.Errorf("extendFive registry failure exited %d, want 1", got)
	}
}

// TestFrontierCodelessDiagnosticFallback drives the classifier's fallback
// for an error-severity diagnostic with no code, which no real check
// emits, through the checkSource seam: every frontier candidate lands in
// the type-check file under the generic "error" code.
func TestFrontierCodelessDiagnosticFallback(t *testing.T) {
	prevCPU, prevCheck := numCPU, checkSource
	t.Cleanup(func() { numCPU, checkSource = prevCPU, prevCheck })
	numCPU = func() int { return 0 }
	checkSource = func(*lang.Boru, string) (lang.CheckResult, error) {
		return lang.CheckResult{Diagnostics: []lang.CheckDiagnostic{{Severity: lang.SeverityError}}}, nil
	}

	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 1 of 1 scanned (max-len 1)\n1\t1\n")
	checkOut := filepath.Join(dir, "check.tsv")
	extractFrontier(passing, checkOut, filepath.Join(dir, "runtime.tsv"), 1)

	// Every candidate (each single atom except the passing "1") lands in
	// the check file with the "error" fallback code.
	n := 0
	err := forEachDataRow(checkOut, func(input, expected, note string, _ int) {
		n++
		if expected != "error" && note != "error" {
			t.Errorf("row %q classified as (%q, %q), want the \"error\" fallback code", input, expected, note)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := len(alphabet) - 1; n != want {
		t.Errorf("check file has %d rows, want %d", n, want)
	}
}

// TestExtendFiveRuntimeBucket drives the extend layer's runtime arm: a
// root whose extension checks clean yet fails at run. `0 true not lt`
// is such a program (no shared ordering between Integer and Boolean), so
// a passing file holding the one root `0 true not` — with no shorter
// rows to subtract — puts it in the runtime bucket.
func TestExtendFiveRuntimeBucket(t *testing.T) {
	dir := t.TempDir()
	passing := writeTSV(t, dir, "passing.tsv", "#kept: 1 of 1 scanned (max-len 3)\n0 true not\t0 false\n")
	short := writeTSV(t, dir, "len123.tsv", "#kept: 0 of 0 scanned (max-len 2)\n")
	runtimeOut := filepath.Join(dir, "runtime.tsv")
	extendFive(passing, short, filepath.Join(dir, "pass.tsv"), filepath.Join(dir, "check.tsv"), runtimeOut)

	found := false
	for _, r := range decodeFile(t, runtimeOut) {
		if r.input == "0 true not lt" {
			found = true
			if r.expected == "" {
				t.Errorf("runtime row %q carries no detail", r.input)
			}
		}
	}
	if !found {
		t.Errorf("runtime bucket is missing %q", "0 true not lt")
	}
}

// TestClassifyFrontierCodelessCheckError drives the classifier's fallback
// for a check that fails with an error carrying no [boru/<code>] prefix:
// the detail is the generic "error".
func TestClassifyFrontierCodelessCheckError(t *testing.T) {
	prev := checkSource
	t.Cleanup(func() { checkSource = prev })
	checkSource = func(*lang.Boru, string) (lang.CheckResult, error) {
		return lang.CheckResult{}, errors.New("no code here")
	}
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	if cl, detail := classifyFrontier(a, "0"); cl != classCheck || detail != "error" {
		t.Errorf("classifyFrontier = (%d, %q), want (classCheck, \"error\")", cl, detail)
	}
}
