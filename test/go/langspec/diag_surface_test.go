package langspec

// diag_surface_test.go — the ONE-DIAGNOSTIC-SURFACE gate
// (checker-compiler-completeness-review.0.md §8.4.2, closed §9.10): every
// diagnostic the COMPILE pass emits beyond the plain `check` surface must
// fall in a LEDGERED class below. The design goal is that `boru check`
// (`Check`) and the compile pipeline (`CompileCheck`) report the SAME
// diagnostics; the residual classes here are each adjudicated — either a
// DESIGNED asymmetry (the compile pass instantiates fn units per call site,
// so it can prove facts the plain pass's abstract body analysis cannot) or
// a known tape-model vestige with a named graduation. The ledger follows
// the frontier discipline: a NEW class fails until triaged; a class no
// longer observed fails as stale (delete the entry).
//
// The sweep runs both surfaces over every corpus row (workers bound the
// wall clock). Plain-surface-only diagnostics are NOT gated: the compile
// pass legitimately resolves some plain-pass findings (a unit compile
// binds what the abstract pass could not), and the compile failure pipeline
// already surfaces anything blocking.

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// diagSurfaceLedger pins the known compile-only diagnostic classes by CODE.
// why records the adjudication; graduation names what deletes the entry.
var diagSurfaceLedger = map[string]string{
	"redundant_guard": "DESIGNED call-site precision: the compile pass analyses each fn unit per concrete instantiation (memo key = arg types), so a guard like `x is Integer` inside an x:Any body is provably always-true AT THE ANALYSED CALL — the plain pass analyses the body once, abstractly, where the guard is load-bearing. A warning-severity hint, never blocking. Graduation = §8.4.4 in-body mirrors (bounded concrete re-analysis on the plain surface at monomorphic all-concrete call sites).",
	// unused_def GRADUATED 2026-08-21 (Stage 1, the def-bound computed-fn
	// read): the closure-factory def-stall vestige (`def f (mk 7) f` —
	// the factory's returned-closure event looked un-seated, so the def
	// read looked unbound and the def unconsumed) is gone — the compile
	// lane resolves the read through the fn-carrier side table, so no
	// corpus row shows a compile-only unused_def any more. It REAPPEARED
	// with the 2026-09-17 corpus expansion (a factory's value read at a
	// higher-order word's forward slot: `each f/v xs`, where the compile
	// pass declined the dispatch before it ever credited the read) and
	// graduated again on 2026-09-19 (S1b-2): the collection seat and the
	// `/v` read resolve the side table, the dispatch matches, and the
	// read credits its def on both passes.
	"undefined_word":       "RE-DIAGNOSED 2026-09-19 (S1b-2): the Stage 1 `/v` hold is GONE — a `/v` read of a name def-bound to a computed fn resolves the fn-carrier side table with the bare read's provenance notes, so those rows no longer diverge. Two unrelated witnesses keep the class: `case zed/q [zed \"matched\" \"other\"]` (case.tsv:L97 — the compile pass reads the bare `zed` inside the clause list as a word where the plain pass leaves it an atom-match) and `0 fold [dot value add] bs` over a generic class's field (generics-fn.tsv:L55 — the compile pass reads `value` in the token body as a word rather than the dot's field name). Both are compile-lane token-body reads of a name that is not a binding; non-blocking for RESULTS (the compile failure keeps the silent interpreter re-run). Graduation = a token body's word reads modelled as the interpreter models them, which is S3's runtime compilation.",
	"macro_not_expandable": "compile-pass-only BY CONSTRUCTION: macro expansion (`parse <kind>` over a parser-fn value) is a compile-pipeline stage — the plain pass has no expansion step to fail. Info-severity; the row declines and is interpreted. Graduation = none expected (a designed stage asymmetry); revisit if the class grows past its two parselang witnesses.",
	"type_error":           "one word-splice witness (`def p word [1 add 2] … f p`): the compile pass's splice-body return-count model claims the body nets no value where the plain pass (and the runtime) see the spliced expression's value. Non-blocking on the corpus row (it compiles and runs). Graduation = splice-body return modeling in the unit walk.",
	"case_not_exhaustive":  "one case-over-instantiated-scrutinee witness: per-call instantiation makes the compile pass judge exhaustiveness against the narrowed scrutinee type where the plain pass judges the declared one — the same designed call-site asymmetry as redundant_guard. Graduation = §8.4.4, with redundant_guard.",
}

func TestDiagnosticSurfaceParity(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("diag-surface sweep: skipped in -short")
	}
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := specEntries(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}
	var rows []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") || e.Name() == "bytecode-combinations.tsv" {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			rows = append(rows, strings.TrimSpace(parts[0]))
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner: %v", err)
		}
	}

	type finding struct {
		code, detail, row string
	}
	var mu sync.Mutex
	observed := map[string]int{}
	var unledgered []finding
	deltaRows := 0

	jobs := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < max(2, runtime.NumCPU()-2); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for src := range jobs {
				a, _ := lang.New()
				resA, errA := a.Check(src)
				if errA != nil {
					continue
				}
				plain := map[string]bool{}
				for _, d := range resA.Diagnostics {
					plain[d.Code+"|"+d.Detail] = true
				}
				b, _ := lang.New()
				_, _, resB, errB := b.CompileCheck(src)
				if errB != nil {
					continue
				}
				delta := false
				for _, d := range resB.Diagnostics {
					if plain[d.Code+"|"+d.Detail] {
						continue
					}
					delta = true
					mu.Lock()
					observed[d.Code]++
					if _, ok := diagSurfaceLedger[d.Code]; !ok && len(unledgered) < 10 {
						unledgered = append(unledgered, finding{d.Code, d.Detail, src})
					}
					mu.Unlock()
				}
				if delta {
					mu.Lock()
					deltaRows++
					mu.Unlock()
				}
			}
		}()
	}
	for _, src := range rows {
		jobs <- src
	}
	close(jobs)
	wg.Wait()

	for _, f := range unledgered {
		t.Errorf("NEW compile-only diagnostic class %q — the compile surface emits a diagnostic the plain `check` surface cannot see:\n  detail: %.100s\n  row:    %.120s\ntriage: unify the surfaces, or adjudicate the class in diagSurfaceLedger (designed asymmetry or named-graduation vestige)",
			f.code, f.detail, f.row)
	}
	// The stale-entry half of the ledger is a corpus-wide claim: skipped
	// under BORU_SPEC_FILES, where a class may simply not be in the subset.
	if !filteredCorpus() {
		for code, why := range diagSurfaceLedger {
			if observed[code] == 0 {
				t.Errorf("stale diagSurfaceLedger class %q — no corpus row shows it as compile-only any more; graduate it (delete the entry).\n  was ledgered because: %.140s", code, why)
			}
		}
	}
	t.Logf("diag-surface parity: %d rows swept, %d with compile-only diagnostics; per class: %v", len(rows), deltaRows, observed)
}
