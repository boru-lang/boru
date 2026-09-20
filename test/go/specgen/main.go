// Command specgen deterministically enumerates every boru token
// sequence up to a fixed length over a small, documented alphabet of
// syntax atoms, evaluates each through the production language layer,
// and emits a `.tsv` spec row (`input<TAB>expected[<TAB>note]`) capturing
// the canonical result — or the stable error class — for that sequence.
//
// Why this exists. The hand-written specs under lang/spec/ pin
// behaviour a human thought to test. This generator instead pins the
// behaviour of EVERY combination of a representative alphabet, so the
// result is an exhaustive, machine-checkable contract: a frozen
// snapshot of exactly what the interpreter produces for each sequence.
//
// The generated file is its own self-contained spec
// (test/go/specgen/syntax-matrix.tsv) with a dedicated runner
// (syntax_matrix_test.go) that asserts the three DETERMINISTIC trust
// properties on every row:
//
//   - interpreter correctness: re-evaluating the row reproduces the
//     recorded canonical value (or the recorded error class), so the
//     interpreter is pinned deterministic against the snapshot;
//   - compiler parity: every row the bytecode compiler accepts produces
//     a result IDENTICAL to the interpreter — the compiler is trustworthy
//     on the whole combination space, not just a curated sample;
//   - checker determinism: the type checker yields the same diagnostics
//     and residual stack on repeated runs, and never panics.
//
// It is deliberately NOT dropped into lang/spec/: that directory's
// suites carry curated, corpus-keyed accuracy ratchets (checker false-
// positive pins, compiled-compile failure ceilings) that an exhaustive ~137k-row
// matrix would swamp with pin churn rather than signal. The existing
// generated parity fixture (bytecode-combinations.tsv) is special-cased
// out of those same ratchets for exactly this reason; a dedicated
// runner keeps this matrix's gates explicit and independent.
//
// The evaluation recipe (parse → fresh DefaultRegistry → q-fixtures →
// parse func → module resolver → frozen clock → NewTop().Run) is a
// deliberate mirror of test/go/langspec/langspec_test.go: same engine,
// same canonical rendering (core.Canon), same frozen instant, so a row
// this tool writes is a row the runner reproduces exactly.
//
// A second mode derives the PASSING SUBSET — the rows that clear all
// three pipelines (interpret to a value, type-check clean, and compile to
// a result identical to the interpreter) — into its own file. That is the
// curated "golden" set a consumer can trust to run, check, and compile.
//
// Usage:
//
//	go run ./specgen [-max N] [-out path]                     # generate the full matrix
//	go run ./specgen -extract -in full.tsv -out passing.tsv   # derive the passing subset
//
//	-max      maximum sequence length to enumerate (default 4)
//	-out      output file; empty writes to stdout (default empty)
//	-extract  extraction mode: read -in (a full matrix) and write the passing subset to -out
//	-in       input full-matrix path (extraction mode)
//
// The Makefile target `spec-gen` regenerates both
// test/go/specgen/syntax-matrix.tsv and syntax-matrix-passing.tsv.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/go/vary"
	"github.com/boru-lang/boru/test/specfix"
)

// Test seams (design/TEST-SEAMS.10.md): tests swap these to observe
// the tool's exit arms without killing the test process, to drive the
// single-CPU worker floor, the instance-construction failures, and the
// compiled-divergence branches (reachable otherwise only via a genuine
// compiler bug).
var (
	osExit      = os.Exit
	numCPU      = runtime.NumCPU
	langNew     = func() (*lang.Boru, error) { return lang.New() }
	runCompiled = (*lang.Boru).RunCompiled
	// newNativeRegistry / compileCheck: the registry constructor cannot
	// fail on a healthy build, and CompileCheck can never return a
	// Program alongside a check error (the checker runs first) — the
	// seams make those arms drivable.
	newNativeRegistry = native.DefaultRegistry
	compileCheck      = (*lang.Boru).CompileCheck
	// varySweep: the healthy build has no reachable divergence (the do-unit
	// registry-replay class is fixed), so the vary mode's diverged-report arm
	// is drivable only by swapping the sweep.
	varySweep = vary.SweepSeeds
)

// alphabet is the fixed, ordered set of syntax atoms the enumeration
// ranges over. Each entry is ONE syntactic element; "up to N elements"
// means every sequence of 1..N of these joined by spaces (N defaults to
// 4). The set is deliberately small and representative rather than
// exhaustive over the whole vocabulary — the cross product grows as
// len(alphabet)^N (19^4 ≈ 130k rows at N=4), so the goal is to cover
// every value family and every operator arity with the fewest atoms,
// not to list every word.
//
// Only core words present in the default production registry are used
// (verified: module-only words like abs/min/concat resolve to
// undefined_word without an import, and so are excluded by design).
//
// Coverage rationale, by group:
//
//	values  — one of each leaf family plus the two structural literals
//	          ([]list and the falsy/none/zero edges that drive
//	          short-circuit, truthiness, and integer-division corners)
//	unary   — words that consume one stack value (dup/drop reshape the
//	          stack; not/size compute over a value)
//	binary  — words that consume two: arithmetic (add/sub/mul),
//	          comparison (eq/lt), boolean (and), and the pure stack
//	          shuffle (swap)
//
// Keep this list in sync with the header comment emitted into the .tsv.
var alphabet = []string{
	// values (8)
	"0", "1", "2.5", "'x'", "true", "false", "none", "[1 2]",
	// unary words (4)
	"dup", "drop", "not", "size",
	// binary words (7)
	"add", "sub", "mul", "eq", "lt", "and", "swap",
}

// specClock freezes time so any clock-seeded behaviour is reproducible,
// matching test/go/langspec/langspec_test.go's specClock exactly.
var specClock = capabilities.FixedClock{T: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}

// run evaluates one input through a fresh production registry and
// returns the resulting stack — a faithful copy of langspec_test.go's
// Run closure so generated rows reproduce under the real spec runner.
func run(input string) ([]core.Value, error) {
	values, err := parser.Parse(input)
	if err != nil {
		return nil, err
	}
	reg, err := newNativeRegistry()
	if err != nil {
		return nil, err
	}
	specfix.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	return native.NewTop(reg).Run(values)
}

// errorClass extracts a stable, message-independent substring from an
// engine error so a generated ERROR row pins the error *kind* rather
// than its (volatile) human text. Engine errors render as
// "[boru/<code>]: <detail>"; the code (e.g. signature_error,
// undefined_word, div_by_zero) is the stable part. Anything without the
// bracketed code falls back to the generic "ERROR:" sentinel, which the
// spec runner treats as "any error".
func errorClass(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "[boru/") {
		if end := strings.IndexByte(msg, ']'); end > len("[boru/") {
			return "ERROR:" + msg[len("[boru/"):end]
		}
	}
	return "ERROR:"
}

// eachSeq yields every alphabet sequence of length 1..max as a token
// slice, in a fully deterministic order: shorter sequences first, then
// odometer order over alphabet indices (last position varies fastest).
// The slice is reused between calls — copy it if you need to retain it.
func eachSeq(max int, emit func(toks []string)) {
	for n := 1; n <= max; n++ {
		idx := make([]int, n)
		for {
			toks := make([]string, n)
			for i, a := range idx {
				toks[i] = alphabet[a]
			}
			emit(toks)

			// odometer increment over base-len(alphabet) digits.
			pos := n - 1
			for pos >= 0 {
				idx[pos]++
				if idx[pos] < len(alphabet) {
					break
				}
				idx[pos] = 0
				pos--
			}
			if pos < 0 {
				break
			}
		}
	}
}

// sequences yields every alphabet sequence of length 1..max as a joined
// source string plus its element count — the form the full-matrix
// generator writes.
func sequences(max int, emit func(src string, n int)) {
	eachSeq(max, func(toks []string) { emit(strings.Join(toks, " "), len(toks)) })
}

func main() {
	max := flag.Int("max", 4, "maximum sequence length to enumerate")
	out := flag.String("out", "", "output .tsv path; empty writes to stdout")
	extract := flag.Bool("extract", false, "extraction mode: read -in and write the passing subset to -out")
	frontier := flag.Bool("frontier", false, "frontier mode: write minimal failing prefixes split into -check-out and -compile-out")
	in := flag.String("in", "", "input full-matrix .tsv path (extraction / frontier mode)")
	passing := flag.String("passing", "", "passing-subset .tsv path (frontier mode)")
	checkOut := flag.String("check-out", "", "output path for type-check-fail prefixes (frontier / extend5 mode)")
	compileOut := flag.String("compile-out", "", "output path for compile-fail prefixes (frontier / extend5 mode)")
	runtimeOut := flag.String("runtime-out", "", "output path for runtime-fail prefixes (frontier / extend5 mode)")
	extend5 := flag.Bool("extend5", false, "extend5 mode: append one atom to every length-4 passing row and split the length-5 results")
	len123 := flag.String("len123", "", "length-1..3 passing-subset path (extend5 mode, used to isolate the length-4 passing rows)")
	passOut := flag.String("pass-out", "", "output path for the length-5 passing combinations (extend5 mode)")
	mismatchOut := flag.String("mismatch-out", "", "optional output path for compiler/interpreter mismatches (extend5 mode)")
	varyMode := flag.Bool("vary", false, "vary mode: classify structured variations of passing lang/spec rows (test/go/vary)")
	seedDir := flag.String("seed-dir", "../../lang/spec", "seed corpus directory (vary mode)")
	varyOut := flag.String("vary-out", "", "output DIRECTORY for the vary classification files (vary mode)")
	varySeeds := flag.Int("vary-seeds", 0, "deterministic seed sample size; 0 = the whole corpus (vary mode)")
	flag.Parse()

	if *varyMode {
		if *varyOut == "" {
			fmt.Fprintln(os.Stderr, "specgen -vary requires -vary-out (an output directory)")
			osExit(2)
		}
		runVarySweep(*seedDir, *varyOut, *varySeeds)
		return
	}

	if *frontier {
		if *passing == "" || *checkOut == "" || *compileOut == "" || *runtimeOut == "" {
			fmt.Fprintln(os.Stderr, "specgen -frontier requires -passing, -check-out, -compile-out and -runtime-out")
			osExit(2)
		}
		extractFrontier(*passing, *checkOut, *compileOut, *runtimeOut, *max)
		return
	}

	if *extend5 {
		if *passing == "" || *len123 == "" || *passOut == "" || *checkOut == "" || *compileOut == "" || *runtimeOut == "" {
			fmt.Fprintln(os.Stderr, "specgen -extend5 requires -passing, -len123, -pass-out, -check-out, -compile-out and -runtime-out")
			osExit(2)
		}
		extendFive(*passing, *len123, *passOut, *checkOut, *compileOut, *runtimeOut, *mismatchOut)
		return
	}

	if *extract {
		if *in == "" || *out == "" {
			fmt.Fprintln(os.Stderr, "specgen -extract requires both -in and -out")
			osExit(2)
		}
		extractPassing(*in, *out, *max)
		return
	}

	w := bufio.NewWriter(os.Stdout)
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "specgen: create %s: %v\n", *out, err)
			osExit(1)
		}
		defer f.Close()
		w = bufio.NewWriter(f)
	}
	defer w.Flush()

	writeHeader(w, *max)

	var rows, values, errs int
	curLen := 0
	sequences(*max, func(src string, n int) {
		if n != curLen {
			curLen = n
			fmt.Fprintf(w, "#\n# §%d  %d-element sequences\n", n, n)
		}
		stack, err := run(src)
		var expected, note string
		if err != nil {
			expected = errorClass(err)
			note = fmt.Sprintf("%d-elem rejected", n)
			errs++
		} else {
			expected = core.Canon(stack)
			note = fmt.Sprintf("%d-elem → %d value(s)", n, len(stack))
			values++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", src, expected, note)
		rows++
	})

	fmt.Fprintf(os.Stderr, "specgen: %d rows (%d value, %d error) over %d atoms, max length %d\n",
		rows, values, errs, len(alphabet), *max)
}

// writeHeader emits the leading comment block describing the file's
// provenance and the alphabet, so a reader of the .tsv knows it is
// generated and how to regenerate it.
func writeHeader(w *bufio.Writer, max int) {
	fmt.Fprintf(w, "# boru Language Specification: Syntax Combination Matrix (GENERATED)\n")
	fmt.Fprintf(w, "# Format: input<TAB>expected<TAB>note\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# DO NOT EDIT BY HAND. Regenerate with:\n")
	fmt.Fprintf(w, "#   cd test/go && go run ./specgen -max %d -out ./specgen/syntax-matrix.tsv\n", max)
	fmt.Fprintf(w, "# (or `make spec-gen` from the repo root).\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# Every sequence of 1..%d atoms drawn from a fixed alphabet, evaluated\n", max)
	fmt.Fprintf(w, "# through the production language layer. `expected` is the canonical\n")
	fmt.Fprintf(w, "# core.Canon rendering of the result stack; `ERROR:<code>` rows pin the\n")
	fmt.Fprintf(w, "# error CLASS (the stable [boru/<code>] tag), not its message text.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# These rows are exhaustive over the alphabet, so they form a frozen\n")
	fmt.Fprintf(w, "# contract checked by syntax_matrix_test.go: the interpreter reproduces\n")
	fmt.Fprintf(w, "# every row, the bytecode compiler matches the interpreter on every row\n")
	fmt.Fprintf(w, "# it accepts, and the type checker is deterministic — all without drift.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# Alphabet (%d atoms): %s\n", len(alphabet), strings.Join(alphabet, " "))
	fmt.Fprintf(w, "#\n")
}

// dataRow is one parsed data row of a generated matrix file.
type dataRow struct {
	input    string
	expected string
	n        int // element count (1..4), parsed from the "N-elem …" note; 0 if absent
}

// frontCodeMarker is the header directive that flags a file's data rows as
// FRONT-CODED. A front-coded data row is `reuse<TAB>suffix[<TAB>extra]`,
// where reuse is the count of leading BYTES the row's input shares with the
// previous data row's input; the full input is prevInput[:reuse]+suffix.
// Because the derived files are emitted in trie-DFS order (siblings of a
// shared prefix are adjacent), most rows reuse ~80% of the prior input, so
// this elides the bulk of the repeated prefix while keeping the file plain
// text, one row per line, and the trailing field (golden value / failure
// detail) greppable. Plain files (no marker) are read verbatim.
const frontCodeMarker = "# format: front-coded"

// detailLegendMarker flags that a front-coded file's trailing field is a
// DETAIL CODE, not the detail text. The distinct details (a handful per
// file — the failure reasons) are enumerated once in the header as
// `# detail <N>: <text>` lines, and each row stores only the integer N.
// This collapses the heavily-repeated, sometimes-long compile failure strings (the
// fail-compile file had 19 distinct details over 60k rows) to one byte or
// two per row. The decoder maps the code back to its text. Files without
// this marker carry their trailing field verbatim.
const detailLegendMarker = "# format: detail-legend"

// detailLegendPrefix leads each legend entry line: `# detail 3: <text>`.
const detailLegendPrefix = "# detail "

// frontCode returns the byte-level front coding of cur against prev: the
// number of shared leading bytes, and cur with that prefix removed.
// Reconstruct with prev[:reuse]+suffix.
func frontCode(prev, cur string) (int, string) {
	m := 0
	for m < len(prev) && m < len(cur) && prev[m] == cur[m] {
		m++
	}
	return m, cur[m:]
}

// forEachDataRow invokes fn for every data row of a generated matrix file,
// skipping comment/blank lines and transparently decoding the front-coded
// format (detected by frontCodeMarker among the header comments) and the
// optional detail legend (detailLegendMarker). For a PLAIN file fn receives
// (input, expected, note); for a FRONT-CODED file note is "" and expected
// is the row's trailing field — the golden value, or the failure detail
// (legend-decoded back to its text when a legend is present), absent → "".
// line is the 1-based source line number.
func forEachDataRow(path string, fn func(input, expected, note string, line int)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	frontCoded := false
	detailLegend := false
	legend := map[int]string{}
	prev := ""
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimRight(sc.Text(), " \t")
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") {
			trimmed := strings.TrimSpace(raw)
			switch {
			case trimmed == frontCodeMarker:
				frontCoded = true
			case trimmed == detailLegendMarker:
				detailLegend = true
			case detailLegend && strings.HasPrefix(trimmed, detailLegendPrefix):
				rest := strings.TrimPrefix(trimmed, detailLegendPrefix)
				if i := strings.Index(rest, ": "); i > 0 {
					if code, e := strconv.Atoi(rest[:i]); e == nil {
						legend[code] = rest[i+2:]
					}
				}
			}
			continue
		}
		parts := strings.Split(raw, "\t")
		if frontCoded {
			if len(parts) < 2 {
				return fmt.Errorf("malformed front-coded row %q in %s:%d", raw, path, line)
			}
			reuse, errc := strconv.Atoi(strings.TrimSpace(parts[0]))
			if errc != nil || reuse < 0 || reuse > len(prev) {
				return fmt.Errorf("bad reuse count %q in %s:%d", parts[0], path, line)
			}
			input := prev[:reuse] + parts[1] // suffix kept verbatim (no trim — it may lead with a space)
			prev = input
			extra := ""
			if len(parts) >= 3 {
				extra = strings.TrimSpace(parts[2])
				if detailLegend {
					code, e := strconv.Atoi(extra)
					if e != nil {
						return fmt.Errorf("bad detail code %q in %s:%d", extra, path, line)
					}
					d, ok := legend[code]
					if !ok {
						return fmt.Errorf("detail code %d not in legend in %s:%d", code, path, line)
					}
					extra = d
				}
			}
			fn(strings.TrimSpace(input), extra, "", line)
			continue
		}
		if len(parts) < 2 {
			return fmt.Errorf("malformed row %q in %s:%d", raw, path, line)
		}
		note := ""
		if len(parts) >= 3 {
			note = strings.TrimSpace(parts[2])
		}
		fn(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), note, line)
	}
	return sc.Err()
}

// readDataRows parses the data rows (input, expected, element count) of a
// generated matrix .tsv, transparently decoding the front-coded format.
func readDataRows(path string) ([]dataRow, error) {
	var rows []dataRow
	err := forEachDataRow(path, func(input, expected, note string, _ int) {
		n := 0
		if len(note) > 0 && note[0] >= '1' && note[0] <= '9' {
			n = int(note[0] - '0')
		}
		rows = append(rows, dataRow{input: input, expected: expected, n: n})
	})
	return rows, err
}

// extractPassing reads the full matrix at inPath and writes, to outPath,
// only the rows that pass interpret + check + compile — the curated
// "golden" subset every downstream consumer can trust to run, type-check
// clean, and compile to an identical result. When maxLen > 0 only rows of
// that element count or shorter are considered (so a length-3 subset can
// be cut from the length-4 matrix). Work is fanned across NumCPU workers
// (each with its own reused checker/compiler instances); output preserves
// the input order, so the file is deterministic.
func extractPassing(inPath, outPath string, maxLen int) {
	rows, err := readDataRows(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extract: %v\n", err)
		osExit(1)
	}

	pass := make([]bool, len(rows))
	var next int64 = -1
	var checked, kept, scanned int64

	workers := numCPU()
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ai, err1 := langNew()
			ac, err2 := langNew()
			if err1 != nil || err2 != nil {
				fmt.Fprintf(os.Stderr, "specgen -extract: lang.New: %v %v\n", err1, err2)
				osExit(1)
			}
			ai.SetClock(specClock)
			ac.SetClock(specClock)
			for {
				idx := int(atomic.AddInt64(&next, 1))
				if idx >= len(rows) {
					return
				}
				r := rows[idx]
				if maxLen > 0 && r.n > maxLen {
					continue // out of the requested length window
				}
				atomic.AddInt64(&scanned, 1)
				if strings.HasPrefix(r.expected, "ERROR:") {
					continue // error rows are never "passing"
				}
				atomic.AddInt64(&checked, 1)

				// 1. interpret → a value (no runtime error)
				gotI, errI := ai.RunInterp(r.input)
				if errI != nil {
					continue
				}
				// 2. check → zero error-severity diagnostics
				res, errChk := ai.Check(r.input)
				if errChk != nil || res.Summary.Errors != 0 {
					continue
				}
				// 3. compile → accepted AND result matches the interpreter
				gotC, wasCompiled, errC := runCompiled(ac, r.input)
				if !wasCompiled || errC != nil {
					continue
				}
				if renderAny(gotC) != renderAny(gotI) {
					continue
				}
				pass[idx] = true
				atomic.AddInt64(&kept, 1)
			}
		}()
	}
	wg.Wait()

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extract: create %s: %v\n", outPath, err)
		osExit(1)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	writePassingHeader(w, int(scanned), int(kept), maxLen)
	prev := ""
	for i, r := range rows {
		if pass[i] {
			reuse, suffix := frontCode(prev, r.input)
			fmt.Fprintf(w, "%d\t%s\t%s\n", reuse, suffix, r.expected)
			prev = r.input
		}
	}

	fmt.Fprintf(os.Stderr, "specgen -extract: %d/%d non-error rows pass interpret+check+compile (%d rows in length window)\n",
		kept, checked, scanned)
}

// renderAny renders a compiled/interpreted result stack ([]any) to a
// stable string for the compiler-parity comparison. Shared with
// syntax_matrix_test.go's gates.
func renderAny(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " ")
}

// writePassingHeader emits the leading comment block of the passing
// subset file. maxLen is the length window (0 = the whole matrix).
func writePassingHeader(w *bufio.Writer, scanned, kept, maxLen int) {
	fmt.Fprintf(w, "# boru Language Specification: Syntax Combination Matrix — PASSING SUBSET (GENERATED)\n")
	fmt.Fprintf(w, "%s\n", frontCodeMarker)
	fmt.Fprintf(w, "# Format: FRONT-CODED. Each data row is reuse<TAB>suffix<TAB>expected,\n")
	fmt.Fprintf(w, "# where reuse = the count of leading bytes the input shares with the\n")
	fmt.Fprintf(w, "# previous row's input and the full input = prevInput[:reuse]+suffix.\n")
	fmt.Fprintf(w, "# The note column is constant (interpret+check+compile) and so is hoisted\n")
	fmt.Fprintf(w, "# out of every row into this header.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# DO NOT EDIT BY HAND. Regenerate with `make spec-gen` (specgen -extract mode).\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# The subset of the combinations that clear ALL THREE pipelines:\n")
	fmt.Fprintf(w, "#   1. interpret — the program runs and leaves a value (no runtime error);\n")
	fmt.Fprintf(w, "#   2. check     — the type checker reports zero error-severity diagnostics;\n")
	fmt.Fprintf(w, "#   3. compile   — the bytecode compiler accepts it AND its result is\n")
	fmt.Fprintf(w, "#                  identical to the interpreter's.\n")
	fmt.Fprintf(w, "#\n")
	if maxLen > 0 {
		fmt.Fprintf(w, "# Scope: combinations of length 1..%d only.\n", maxLen)
		fmt.Fprintf(w, "#\n")
	}
	fmt.Fprintf(w, "# `expected` is the canonical core.Canon value carried over verbatim from\n")
	fmt.Fprintf(w, "# the full matrix. Every row here is a trusted, three-way-agreed program;\n")
	fmt.Fprintf(w, "# syntax_matrix_test.go re-verifies the invariant on each one.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# %d combinations pass (%d in the length window).\n", kept, scanned)
	fmt.Fprintf(w, "#\n")
}

// frontierClass is the stage at which a minimal failing prefix fails.
type frontierClass int

const (
	classCheck   frontierClass = iota // the type checker reports an error (compiler declines)
	classCompile                      // checker clean, but the compiler will not compile it
	classRuntime                      // checker clean and it compiles, yet it is not a passing program
)

// classifyFrontier runs one prefix through CompileCheck on a reused
// instance and returns the stage it fails at, a short detail (the check
// error code, or the compiler's compile failure reason), and whether the compiler
// emitted a Program. CompileCheck runs the checker BEFORE any lowering
// and returns a nil Program on any error-severity diagnostic — so a
// check-stage failure can never also be `compiled`. The caller asserts
// that (the "checker runs first" confirmation) on the returned bool.
func classifyFrontier(a *lang.Boru, src string) (frontierClass, string, bool) {
	prog, reason, res, err := compileCheck(a, src)
	compiled := prog != nil
	if err != nil {
		return classCheck, sanitizeNote(reason), compiled // parse error / check run error
	}
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			code := d.Code
			if code == "" {
				code = "error"
			}
			return classCheck, code, compiled
		}
	}
	if !compiled {
		return classCompile, sanitizeNote(reason), compiled // checker clean, Stage-1 could not lower it
	}
	// Checker clean AND it compiled, yet it is not a passing program — run
	// it to capture why: almost always a runtime error the static checker
	// cannot predict (e.g. `incomparable`), occasionally a value the
	// passing-extractor rejected.
	if _, errRun := a.RunInterp(src); errRun != nil {
		detail := strings.TrimPrefix(errorClass(errRun), "ERROR:")
		if detail == "" {
			detail = "error"
		}
		return classRuntime, detail, compiled
	}
	return classRuntime, "non-passing-value", compiled
}

// sanitizeNote makes a reason string safe for a single TSV note column.
func sanitizeNote(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "uncompilable"
	}
	return s
}

// readInputSet reads a generated matrix file into a set of its input
// strings (used as the "passing" membership oracle).
func readInputSet(path string) (map[string]bool, error) {
	rows, err := readDataRows(path)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(rows))
	for _, r := range rows {
		m[r.input] = true
	}
	return m, nil
}

// extractFrontier finds the minimal failing prefixes of the matrix — the
// sequences that FAIL while their immediate prefix PASSES, i.e. the exact
// point at which a passing program first breaks when one more atom is
// appended. Every non-passing ("remaining") row truncates to exactly one
// such prefix (its first break point; the tokens after it are discarded),
// so this set is the deduped catalogue of distinct failure points.
//
// Each prefix is then classified by the stage it fails at and written to
// the matching file: -check-out for type-check failures (the checker
// rejects it), -compile-out for compile failures (the checker passes but
// the bytecode compiler will not lower it), and -runtime-out for runtime
// failures (the checker passes AND it compiles, yet it errors at run — a
// fault the static stages cannot predict).
//
// As it goes it verifies the "compiler runs the checker first" contract:
// no prefix that the checker rejects is ever emitted as a Program.
func extractFrontier(passingPath, checkOut, compileOut, runtimeOut string, max int) {
	passing, err := readInputSet(passingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -frontier: %v\n", err)
		osExit(1)
	}

	// 1. Collect the frontier: fails, immediate prefix passes.
	var cands []string
	eachSeq(max, func(toks []string) {
		s := strings.Join(toks, " ")
		if passing[s] {
			return
		}
		prefixOK := len(toks) == 1 // the empty prefix trivially "passes"
		if len(toks) > 1 {
			prefixOK = passing[strings.Join(toks[:len(toks)-1], " ")]
		}
		if prefixOK {
			cands = append(cands, s)
		}
	})

	// 2. Classify in parallel (each worker reuses one compiler instance —
	//    safe because the alphabet has no state-mutating word).
	classes := make([]frontierClass, len(cands))
	notes := make([]string, len(cands))
	var checkFirstViolations int64
	var idx int64 = -1
	workers := numCPU()
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, e := langNew()
			if e != nil {
				fmt.Fprintf(os.Stderr, "specgen -frontier: lang.New: %v\n", e)
				osExit(1)
			}
			a.SetClock(specClock)
			for {
				k := int(atomic.AddInt64(&idx, 1))
				if k >= len(cands) {
					return
				}
				cl, detail, compiled := classifyFrontier(a, cands[k])
				classes[k] = cl
				notes[k] = detail
				if cl == classCheck && compiled {
					atomic.AddInt64(&checkFirstViolations, 1) // checker rejected it yet a Program was emitted
				}
			}
		}()
	}
	wg.Wait()

	// 3. Write the three files in input order.
	nCheck := writeFrontierFile(checkOut, "type-check", "check-fail", cands, classes, notes, classCheck, max)
	nCompile := writeFrontierFile(compileOut, "compile", "compile-declined", cands, classes, notes, classCompile, max)
	nRuntime := writeFrontierFile(runtimeOut, "runtime", "runtime-fail", cands, classes, notes, classRuntime, max)

	fmt.Fprintf(os.Stderr, "specgen -frontier: %d minimal failing prefixes — %d type-check, %d compile, %d runtime\n",
		len(cands), nCheck, nCompile, nRuntime)
	fmt.Fprintf(os.Stderr, "specgen -frontier: checker-runs-first check: %d prefixes rejected by the checker were ALSO emitted as a Program (want 0)\n",
		checkFirstViolations)
}

// writeFailRows appends, to a header already written to w, the DETAIL
// LEGEND block (`# detail N: <text>` for each distinct detail, in
// first-appearance order) followed by the FRONT-CODED data rows
// (reuse<TAB>suffix<TAB>code, code = the detail's legend index). sel(k)
// selects the rows of this file; cands[k]/notes[k] are the input/detail.
// The caller must have emitted both the frontCodeMarker and the
// detailLegendMarker in the header.
func writeFailRows(w *bufio.Writer, cands, notes []string, sel func(int) bool) {
	// Assign codes to distinct details in first-appearance order.
	code := map[string]int{}
	var order []string
	for k := range cands {
		if !sel(k) {
			continue
		}
		if _, ok := code[notes[k]]; !ok {
			code[notes[k]] = len(order)
			order = append(order, notes[k])
		}
	}
	for i, d := range order {
		fmt.Fprintf(w, "%s%d: %s\n", detailLegendPrefix, i, d)
	}
	fmt.Fprintf(w, "#\n")
	prev := ""
	for k := range cands {
		if !sel(k) {
			continue
		}
		reuse, suffix := frontCode(prev, cands[k])
		fmt.Fprintf(w, "%d\t%s\t%d\n", reuse, suffix, code[notes[k]])
		prev = cands[k]
	}
}

// writeFrontierFile writes the candidates of one class to path in
// FRONT-CODED form with a detail legend (reuse<TAB>suffix<TAB>code; the
// constant `<tag>:` disposition prefix is hoisted to the header and the
// detail text is enumerated once in the legend), and returns the count.
func writeFrontierFile(path, kind, tag string, cands []string, classes []frontierClass, notes []string, want frontierClass, maxLen int) int {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -frontier: create %s: %v\n", path, err)
		osExit(1)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	n := 0
	for _, cl := range classes {
		if cl == want {
			n++
		}
	}
	writeFrontierHeader(w, kind, tag, n, maxLen)
	writeFailRows(w, cands, notes, func(k int) bool { return classes[k] == want })
	return n
}

// writeFrontierHeader emits the leading comment block of a frontier file.
// maxLen is the length window the prefixes were drawn from.
func writeFrontierHeader(w *bufio.Writer, kind, tag string, n, maxLen int) {
	fmt.Fprintf(w, "# boru Language Specification: Minimal Failing Prefixes — %s FAILURES (GENERATED)\n", strings.ToUpper(kind))
	fmt.Fprintf(w, "%s\n", frontCodeMarker)
	fmt.Fprintf(w, "%s\n", detailLegendMarker)
	fmt.Fprintf(w, "# Format: FRONT-CODED with a DETAIL LEGEND. Each data row is\n")
	fmt.Fprintf(w, "# reuse<TAB>suffix<TAB>code, where reuse = the count of leading bytes the\n")
	fmt.Fprintf(w, "# prefix shares with the previous row's prefix (full prefix =\n")
	fmt.Fprintf(w, "# prevPrefix[:reuse]+suffix) and code indexes the `# detail N:` legend\n")
	fmt.Fprintf(w, "# below. The constant `%s:` disposition prefix (and the documentary\n", tag)
	fmt.Fprintf(w, "# full-matrix result) are hoisted out of every row.\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# DO NOT EDIT BY HAND. Regenerate with `make spec-gen` (specgen -frontier mode).\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# A MINIMAL FAILING PREFIX is a sequence that fails while its immediate\n")
	fmt.Fprintf(w, "# prefix passes — the exact atom at which a passing program first breaks.\n")
	fmt.Fprintf(w, "# Every non-passing combination truncates to one of these (the trailing\n")
	fmt.Fprintf(w, "# atoms after the break are discarded) and DUPLICATES ARE REMOVED, so this\n")
	fmt.Fprintf(w, "# is the deduped catalogue of distinct failure points.\n")
	fmt.Fprintf(w, "#\n")
	if maxLen > 0 {
		fmt.Fprintf(w, "# Scope: prefixes drawn from combinations of length 1..%d.\n", maxLen)
		fmt.Fprintf(w, "#\n")
	}
	switch kind {
	case "type-check":
		fmt.Fprintf(w, "# These prefixes FAIL THE TYPE CHECKER: `boru check` reports at least one\n")
		fmt.Fprintf(w, "# error-severity diagnostic. Because the compiler runs the checker first\n")
		fmt.Fprintf(w, "# (lang.(*Boru).CompileCheck), every prefix here is also declined by the\n")
		fmt.Fprintf(w, "# compiler — none is ever lowered to bytecode.\n")
	case "runtime":
		fmt.Fprintf(w, "# These prefixes PASS THE TYPE CHECKER and COMPILE to bytecode, yet they\n")
		fmt.Fprintf(w, "# ERROR AT RUN — a fault the static stages cannot predict (e.g. comparing\n")
		fmt.Fprintf(w, "# values of incomparable types). The note gives the runtime error class;\n")
		fmt.Fprintf(w, "# the interpreter and compiler agree on it (it is not a compiler bug).\n")
	default:
		fmt.Fprintf(w, "# These prefixes PASS THE TYPE CHECKER but the bytecode compiler will not\n")
		fmt.Fprintf(w, "# lower them (CompileCheck returns no Program); the note gives the first\n")
		fmt.Fprintf(w, "# offender / compile failure reason. They run correctly via the interpreter.\n")
	}
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# %d prefixes.\n", n)
	fmt.Fprintf(w, "#\n")
}

// ---- extend5: the length-5 layer -----------------------------------------
//
// extendFive takes every LENGTH-4 PASSING row as a prefix, appends each
// alphabet atom once (so the prefix becomes a 5-part combination), and
// splits the results into the same four buckets — passing, type-check
// fail, compile fail, runtime fail.
//
// Why only the length-4 passing rows? A length-5 combination carries NEW
// information (not already cataloged at length <=4) only if its length-4
// prefix passes: if that prefix already failed, the length-5 failure is
// captured by a shorter minimal failing prefix that is already in the
// length-1..4 files. So extending the length-4 passing rows by one atom
// yields exactly the deduplicated length-5 layer — every length-5
// combination whose failure (or success) is not already explained by a
// shorter prefix. Each fail row here is therefore itself a length-5
// minimal failing prefix.

type fiveClass int

const (
	fivePass fiveClass = iota
	fiveCheck
	fiveCompile
	fiveRuntime
	fiveMismatch // compiled result diverges from the interpreter — a compiler bug, reported separately
)

// firstErrorCode returns the code of the first error-severity diagnostic.
func firstErrorCode(res lang.CheckResult) string {
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			if d.Code != "" {
				return d.Code
			}
			return "error"
		}
	}
	return "error"
}

func hasErrorDiag(res lang.CheckResult) bool {
	for _, d := range res.Diagnostics {
		if d.Severity == lang.SeverityError {
			return true
		}
	}
	return false
}

// freshDivergence re-runs one program on a FRESH lang instance (no shared
// state — the gold standard the langspec differential gate uses) and
// reports whether the bytecode compiler genuinely disagrees with the
// interpreter: one errors where the other doesn't, or they return
// different values. Returns ("", false) when they agree (or the program
// does not compile in isolation), so a reused-instance artifact can never
// be mistaken for a compiler bug.
func freshDivergence(s string) (string, bool) {
	a, err := langNew()
	if err != nil {
		return "", false
	}
	a.SetClock(specClock)
	gotI, errI := a.RunInterp(s)
	gotC, wasC, errC := runCompiled(a, s)
	if !wasC {
		return "", false
	}
	if (errC != nil) != (errI != nil) {
		return sanitizeNote(fmt.Sprintf("error-divergence interpreted-errored=%v compiled-errored=%v", errI != nil, errC != nil)), true
	}
	if errC != nil {
		return "", false // both errored → they agree
	}
	if renderAny(gotC) != renderAny(gotI) {
		return sanitizeNote(fmt.Sprintf("interpreted=[%s] compiled=[%s]", renderAny(gotI), renderAny(gotC))), true
	}
	return "", false
}

// canonOf renders one input's canonical (core.Canon) value via a reused
// native registry — the same recipe as the full matrix — or "" when the
// input does not parse or does not run cleanly.
func canonOf(reg *core.Registry, s string) string {
	values, perr := parser.Parse(s)
	if perr != nil {
		return ""
	}
	out, rerr := native.NewTop(reg).Run(values)
	if rerr != nil {
		return ""
	}
	return core.Canon(out)
}

func extendFive(passingPath, len123PassingPath, passOut, checkOut, compileOut, runtimeOut, mismatchOut string) {
	passingRows, err := readDataRows(passingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extend5: %v\n", err)
		osExit(1)
	}
	short, err := readInputSet(len123PassingPath) // length-1..3 passing
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extend5: %v\n", err)
		osExit(1)
	}

	// Length-4 passing roots = full passing minus the length-1..3 passing.
	var roots []string
	for _, r := range passingRows {
		if !short[r.input] {
			roots = append(roots, r.input)
		}
	}
	// Candidates: each root extended by one atom, in deterministic order.
	cands := make([]string, 0, len(roots)*len(alphabet))
	for _, root := range roots {
		for _, a := range alphabet {
			cands = append(cands, root+" "+a)
		}
	}

	classes := make([]fiveClass, len(cands))
	notes := make([]string, len(cands))
	expects := make([]string, len(cands))
	var idx int64 = -1
	var nPass, nCheck, nCompile, nRuntime, nMismatch int64

	workers := numCPU()
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			la, e := langNew()
			if e != nil {
				fmt.Fprintf(os.Stderr, "specgen -extend5: lang.New: %v\n", e)
				osExit(1)
			}
			la.SetClock(specClock)
			// A reused native registry produces the canonical (core.Canon)
			// value for passing rows — same recipe as the full matrix.
			reg, e2 := newNativeRegistry()
			if e2 != nil {
				fmt.Fprintf(os.Stderr, "specgen -extend5: registry: %v\n", e2)
				osExit(1)
			}
			specfix.RegisterQFixtures(reg)
			reg.SetParseFunc(parser.Parse)
			modules.InstallResolver(reg)
			native.SetHostClock(reg, specClock)

			for {
				k := int(atomic.AddInt64(&idx, 1))
				if k >= len(cands) {
					return
				}
				s := cands[k]
				prog, reason, res, cerr := la.CompileCheck(s)
				switch {
				case cerr != nil:
					classes[k], notes[k] = fiveCheck, sanitizeNote(reason)
					atomic.AddInt64(&nCheck, 1)
				case hasErrorDiag(res):
					classes[k], notes[k] = fiveCheck, firstErrorCode(res)
					atomic.AddInt64(&nCheck, 1)
				case prog == nil:
					classes[k], notes[k] = fiveCompile, sanitizeNote(reason)
					atomic.AddInt64(&nCompile, 1)
				default:
					gotI, errI := la.RunInterp(s)
					if errI != nil {
						classes[k], notes[k] = fiveRuntime, strings.TrimPrefix(errorClass(errI), "ERROR:")
						atomic.AddInt64(&nRuntime, 1)
						continue
					}
					gotC, wasC, errC := runCompiled(la, s)
					if !wasC || errC != nil || renderAny(gotC) != renderAny(gotI) {
						// Divergence on the reused instance — re-verify on a
						// FRESH lang (matching the langspec differential gate)
						// to rule out a reused-instance artifact. Only a
						// divergence that survives a fresh, isolated run is a
						// genuine compiler bug; otherwise the candidate passes.
						if note, real := freshDivergence(s); real {
							classes[k], notes[k] = fiveMismatch, note
							atomic.AddInt64(&nMismatch, 1)
							continue
						}
					}
					classes[k], expects[k] = fivePass, canonOf(reg, s)
					atomic.AddInt64(&nPass, 1)
				}
			}
		}()
	}
	wg.Wait()

	nP := writeLen5Pass(passOut, cands, classes, expects)
	nC := writeLen5Fail(checkOut, "type-check", cands, classes, notes, fiveCheck)
	nK := writeLen5Fail(compileOut, "compile", cands, classes, notes, fiveCompile)
	nR := writeLen5Fail(runtimeOut, "runtime", cands, classes, notes, fiveRuntime)
	nM := 0
	if mismatchOut != "" {
		nM = writeLen5Fail(mismatchOut, "mismatch", cands, classes, notes, fiveMismatch)
	}

	fmt.Fprintf(os.Stderr, "specgen -extend5: %d length-4 passing roots × %d atoms = %d length-5 combinations\n",
		len(roots), len(alphabet), len(cands))
	fmt.Fprintf(os.Stderr, "specgen -extend5: passing=%d  type-check-fail=%d  compile-fail=%d  runtime-fail=%d  compiler-mismatch=%d\n",
		nPass, nCheck, nCompile, nRuntime, nMismatch)
	fmt.Fprintf(os.Stderr, "specgen -extend5: wrote %d / %d / %d / %d (+%d mismatch) rows\n", nP, nC, nK, nR, nM)
	if nMismatch > 0 {
		fmt.Fprintf(os.Stderr, "specgen -extend5: NOTE %d fresh-verified compiler/interpreter divergences (see mismatch file)\n", nMismatch)
	}
}

// writeLen5Pass writes the passing length-5 combinations as
// input<TAB>expected<TAB>note.
func writeLen5Pass(path string, cands []string, classes []fiveClass, expects []string) int {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extend5: create %s: %v\n", path, err)
		osExit(1)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	n := 0
	for _, cl := range classes {
		if cl == fivePass {
			n++
		}
	}
	writeLen5Header(w, "passing", n)
	prev := ""
	for k, cl := range classes {
		if cl == fivePass {
			reuse, suffix := frontCode(prev, cands[k])
			fmt.Fprintf(w, "%d\t%s\t%s\n", reuse, suffix, expects[k])
			prev = cands[k]
		}
	}
	return n
}

// writeLen5Fail writes the length-5 minimal failing prefixes of one class
// in FRONT-CODED form with a detail legend (reuse<TAB>suffix<TAB>code). The
// disposition is implied by the file itself, so it is not stored per row.
func writeLen5Fail(path, kind string, cands []string, classes []fiveClass, notes []string, want fiveClass) int {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -extend5: create %s: %v\n", path, err)
		osExit(1)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	n := 0
	for _, cl := range classes {
		if cl == want {
			n++
		}
	}
	writeLen5Header(w, kind, n)
	writeFailRows(w, cands, notes, func(k int) bool { return classes[k] == want })
	return n
}

// ---- vary: structured variations of passing corpus rows -------------------
//
// runVarySweep drives test/go/vary (the shared transform table + dual-
// pipeline classifier — also the engine behind langspec's standing
// TestVariationDifferential gate) at triage breadth and writes the four
// classification files under outDir:
//
//	vary-pass.tsv          variants that compile natively with parity
//	vary-declined.tsv       compile failures + islands (the NEW-frontier feed)
//	vary-diverged.tsv      compiler/interpreter divergences — MISCOMPILES
//	vary-interp-reject.tsv variants the interpreter (or checker) rejects — discards
//
// Outputs are triage artifacts, deliberately NOT checked in (the same
// no-lang/spec-pollution rule as the syntax matrix: the CI gate recomputes
// its variants at test time, so there is no snapshot to drift).
func runVarySweep(seedDir, outDir string, nSeeds int) {
	seeds, err := vary.LoadSeeds(seedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "specgen -vary: %v\n", err)
		osExit(1)
	}
	sample := vary.Sample(seeds, nSeeds)
	variants := varySweep(sample, func(done, total int) {
		if done%50 == 0 || done == total {
			fmt.Fprintf(os.Stderr, "specgen -vary: %d/%d seeds\n", done, total)
		}
	})

	name := map[vary.Outcome]string{
		vary.Pass:         "vary-pass.tsv",
		vary.Declined:     "vary-declined.tsv",
		vary.Islanded:     "vary-declined.tsv",
		vary.Diverged:     "vary-diverged.tsv",
		vary.InterpReject: "vary-interp-reject.tsv",
		vary.CheckReject:  "vary-interp-reject.tsv",
	}
	files := map[string]*bufio.Writer{}
	counts := map[vary.Outcome]int{}
	skippedSeeds := 0
	for _, v := range variants {
		if v.Transform == "seed" {
			if v.Res.Outcome != vary.Pass {
				skippedSeeds++ // base not vary-eligible; ratchets own it
			}
			continue
		}
		counts[v.Res.Outcome]++
		fn := name[v.Res.Outcome]
		w, ok := files[fn]
		if !ok {
			f, err := os.Create(outDir + "/" + fn)
			if err != nil {
				fmt.Fprintf(os.Stderr, "specgen -vary: create %s: %v\n", fn, err)
				osExit(1)
			}
			defer f.Close()
			w = bufio.NewWriter(f)
			defer w.Flush()
			fmt.Fprintf(w, "# specgen -vary classification (GENERATED, triage artifact — do not check in)\n")
			fmt.Fprintf(w, "# Format: variant-src<TAB>outcome: detail<TAB>transform(seed-file:line)\n#\n")
			files[fn] = w
		}
		fmt.Fprintf(w, "%s\t%s\t%s(%s:%d)\n",
			v.Src, sanitizeNote(v.Res.Outcome.String()+": "+v.Res.Detail), v.Transform, v.Seed.File, v.Seed.Line)
	}

	fmt.Fprintf(os.Stderr, "specgen -vary: %d seeds (%d skipped non-passing) × %d transforms\n",
		len(sample), skippedSeeds, len(vary.Transforms()))
	fmt.Fprintf(os.Stderr, "specgen -vary: pass=%d declined=%d islanded=%d diverged=%d interp-reject=%d check-reject=%d\n",
		counts[vary.Pass], counts[vary.Declined], counts[vary.Islanded], counts[vary.Diverged],
		counts[vary.InterpReject], counts[vary.CheckReject])
	if counts[vary.Diverged] > 0 {
		fmt.Fprintf(os.Stderr, "specgen -vary: NOTE %d divergences — MISCOMPILES, see vary-diverged.tsv\n", counts[vary.Diverged])
	}
}

// writeLen5Header emits the leading comment block of a length-5 file.
func writeLen5Header(w *bufio.Writer, kind string, n int) {
	fmt.Fprintf(w, "# boru Language Specification: Length-5 Layer — %s (GENERATED)\n", strings.ToUpper(kind))
	fmt.Fprintf(w, "%s\n", frontCodeMarker)
	if kind != "passing" {
		fmt.Fprintf(w, "%s\n", detailLegendMarker)
	}
	fmt.Fprintf(w, "# DO NOT EDIT BY HAND. Regenerate with `make spec-gen` (specgen -extend5 mode).\n")
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# Every LENGTH-4 PASSING row, extended by one atom into a 5-part\n")
	fmt.Fprintf(w, "# combination, then split by outcome. Only length-4 passing rows are\n")
	fmt.Fprintf(w, "# extended: a length-5 combination whose length-4 prefix already failed\n")
	fmt.Fprintf(w, "# is captured by a shorter minimal failing prefix in the length-1..4\n")
	fmt.Fprintf(w, "# files, so this layer is the deduplicated set of genuinely new length-5\n")
	fmt.Fprintf(w, "# outcomes. Each fail row is itself a length-5 minimal failing prefix.\n")
	fmt.Fprintf(w, "#\n")
	// Front-coded rows: reuse<TAB>suffix<TAB>extra, full input =
	// prevInput[:reuse]+suffix (see frontCodeMarker). For fail kinds extra is
	// a `# detail N:` legend code; for passing it is the expected value.
	switch kind {
	case "passing":
		fmt.Fprintf(w, "# Format: FRONT-CODED, extra = expected. These clear interpret + check +\n")
		fmt.Fprintf(w, "# compile; `expected` is the canonical core.Canon result.\n")
	case "type-check":
		fmt.Fprintf(w, "# Format: FRONT-CODED + legend, extra = detail code. The type checker\n")
		fmt.Fprintf(w, "# rejects these; the compiler, which runs the checker first, declines them.\n")
	case "runtime":
		fmt.Fprintf(w, "# Format: FRONT-CODED + legend, extra = detail code. These check clean AND\n")
		fmt.Fprintf(w, "# compile, yet error at run (detail = the runtime error class).\n")
	case "mismatch":
		fmt.Fprintf(w, "# Format: FRONT-CODED + legend, extra = detail code. COMPILER/INTERPRETER\n")
		fmt.Fprintf(w, "# DIVERGENCES: these check clean and compile, but the bytecode result\n")
		fmt.Fprintf(w, "# differs from the interpreter (re-verified on a fresh, isolated\n")
		fmt.Fprintf(w, "# instance). detail shows both results. These are candidate compiler bugs,\n")
		fmt.Fprintf(w, "# NOT part of the four-way split — surfaced by the length-5 sweep.\n")
	default:
		fmt.Fprintf(w, "# Format: FRONT-CODED + legend, extra = detail code. These check clean but\n")
		fmt.Fprintf(w, "# the compiler will not lower them (detail = the compile failure reason).\n")
	}
	fmt.Fprintf(w, "#\n")
	fmt.Fprintf(w, "# %d rows.\n", n)
	fmt.Fprintf(w, "#\n")
}
