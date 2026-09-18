// The single live census of compiled-coverage state.
//
// Three surfaces report on how much of the spec corpus the bytecode compiler
// covers: the refusal/island ceilings (TestCompiledCoverage), the re-scoped P7
// tier partition (TestOnlyMetaFallsBack), and the generated status document
// (TestCompiledStatus). They used to walk the corpus independently and emit
// only to t.Logf, so the live numbers lived nowhere durable — they had to be
// transcribed by hand into the ceiling const comments, which then drifted.
//
// This file is the one walk. gatherCensus compiles every value row ONCE,
// caches the tallies, and hands the same struct to all three tests, so the
// numbers are computed in exactly one place and the corpus is compiled once
// rather than three times. The ceiling assertions in the two gate tests
// self-verify the disposition logic here: if a count drifts from what those
// tests counted inline before, a ceiling trips.
//
// The walk is the package's parallel corpus walk (walk_test.go) in its
// TB-less form, specWalkFilesErr: every file is tallied into a census of
// its own on a worker and the partials are folded together in directory
// order afterwards, so every count and every list is exactly what the
// one-goroutine walk produced, whichever worker saw a file first.
package langspec

import (
	"sort"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// census is the whole-corpus tally: the per-row disposition (native /
// islanded / refused / static check-error) plus the re-scoped-P7 tier
// partition over every not-fully-native row. A field added here is folded
// in add as well — that is how one file's partial reaches the whole.
type census struct {
	rows     int
	compiled int // native + islanded
	islanded int // subset of compiled that still embeds an OpFallback island
	checkErr int // statically invalid in BOTH engines — not a refusal
	refused  int // nil Program, no check error

	refusalBuckets map[string]int // normaliseReason -> count, over refused rows only
	refusedRows    []refusedRow   // one entry per refused row, in corpus order

	// Re-scoped P7 partition (design/legacy/boru-bytecode-completion.0.ignore §3) over the
	// not-fully-native rows (refused OR islanded): tier 1 interpreter-only
	// (permanent), tier 2 reducible (TODO), allowlisted error rows, and the
	// remaining compute frontier.
	interp      int
	reducible   int
	errorRows   int
	computeGap  int
	computeRows []string
	tier1By     map[string]int
	tier2By     map[string]int
	computeBy   map[string]int
}

// refusedRow identifies one spec row the bytecode compiler refused to lower —
// enough for TestRefusalsAreFailures to fail on it by exact source and point a
// contributor at the offending row.
type refusedRow struct {
	file   string // spec basename, e.g. "apply.tsv"
	line   int    // 1-based line within the file
	input  string // the row's source (the trimmed first TSV column)
	reason string // the whole-program refusal reason
}

var (
	censusOnce sync.Once
	censusVal  *census
	censusErr  error
)

// gatherCensus returns the cached corpus census, computing it on first call.
func gatherCensus(t *testing.T) *census {
	t.Helper()
	censusOnce.Do(func() { censusVal, censusErr = computeCensus() })
	if censusErr != nil {
		t.Fatalf("gather census: %v", censusErr)
	}
	return censusVal
}

// newCensus is an empty census with its histograms allocated.
func newCensus() *census {
	return &census{
		refusalBuckets: map[string]int{},
		tier1By:        map[string]int{},
		tier2By:        map[string]int{},
		computeBy:      map[string]int{},
	}
}

// computeCensus walks every lang/spec/*.tsv value row through CompileCheck and
// classifies it (tallyFile). It builds instances directly (rather than via
// newDifferentialInstance) so it needs no *testing.T and can return IO errors
// instead of FailNow-ing inside sync.Once. Each file's partial is folded in
// sorted-name order — os.ReadDir's order, the order the one-goroutine walk
// tallied in — so refusedRows and computeRows read in corpus order.
func computeCensus() (*census, error) {
	var (
		mu    sync.Mutex
		parts = map[string]*census{}
		first error // the first instance failure, which voids the census as it always did
	)
	err := specWalkFilesErr(func(file string, rows []specRow) {
		part, err := tallyFile(rows)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if first == nil {
				first = err
			}
			return
		}
		parts[file] = part
	})
	if err != nil {
		return nil, err
	}
	if first != nil {
		return nil, first
	}
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	c := newCensus()
	for _, name := range names {
		c.add(parts[name])
	}
	return c, nil
}

// tallyFile is one file's share of the census. The disposition logic mirrors
// the two gate tests exactly (their ceilings are the safety net): a
// check-error row is statically invalid in both engines, an islanded Program
// still re-enters the interpreter at run time, and a nil Program with no
// check error is a refusal. The tier partition classifies every NOT-fully-
// native row — refused or islanded — first by the permanent/reducible word
// allowlists (classify), then as an allowlisted error row, else as a
// compute-frontier gap.
func tallyFile(rows []specRow) (*census, error) {
	c := newCensus()
	for _, r := range rows {
		if len(r.Cells) < 2 {
			continue
		}
		input := r.Input
		expectErr := strings.HasPrefix(strings.TrimSpace(r.Cells[1]), "ERROR:")
		c.rows++

		a, err := lang.New()
		if err != nil {
			return nil, err
		}
		a.SetClock(specClock)
		prog, reason, res, cerr := a.CompileCheck(input)

		// A refusal from a pass that substituted a fn-carrier read
		// (Stage 1) classifies with the check-diagnostics sentinel it
		// refused behind before the substitution landed: RunCompiled
		// keeps the same silent interpreter fallback for this class,
		// so it is not a hard refusal (the frontier compile ledger
		// tracks its precise reasons row by row).
		if cerr != nil || reason == "check diagnostics" ||
			(prog == nil && res.FnCarrierReadSubstituted) {
			c.checkErr++
			continue
		}
		islanded := prog != nil && strings.Contains(prog.Disassemble(), "FALLBACK")
		if prog != nil {
			c.compiled++
			if islanded {
				c.islanded++
			}
		} else {
			c.refused++
			c.refusalBuckets[normaliseReason(reason)]++
			c.refusedRows = append(c.refusedRows, refusedRow{
				file: r.File, line: r.Line, input: input, reason: reason,
			})
		}
		if prog != nil && !islanded {
			continue // fully native — outside the tier partition
		}
		// Refused or islanded: classify into the P7 partition.
		//
		// A refused/islanded row whose SPEC expects an error is a
		// correct-error row: the checker refuses (or islands) so the
		// interpreter raises the matching taxonomy, and the full-corpus gate
		// confirms parity. The spec's ERROR: marker is the authoritative
		// signal — it distinguishes these from value rows that happen to share
		// a refusal reason (`x/v` illegal_ref vs `mini re` dynamic output both
		// refuse "...unknown provenance"). errorRowReason stays as a secondary
		// signal for the few error reasons that are intrinsically diagnostic.
		//
		// This disposition is checked BEFORE tier-2, so an error row is never
		// mis-counted as reducible compiler debt merely because its source
		// mentions a tier-2 word. (Example of an ERROR row that previously
		// refused: `mini re 'a'` with no import — the interpreter raises
		// mini_unknown_lang; these compile to a top-level OpTrap now, but a
		// nested occurrence still declines the trap and refuses here.) Note
		// macro.tsv:45 (`def loopy (macro [[a] [quote [loopy unquote a]]])
		// macroexpand (loopy 1)`, ERROR:expansion too deep) no longer reaches
		// this branch at all: it COMPILES to a terminal macroexpand_error
		// OpTrap (carrier.go), so it is counted as a compiled row. Tier 1 stays
		// first: a `Vm.run` row is genuinely interpreter-only even when it errors.
		tier, name := classify(input)
		bucket := normaliseReason(reason)
		// A row whose SOURCE mentions a tier-2 word but whose actual blocker is
		// the SOUNDNESS frontier (a dynamically-fetched fn value with lost
		// provenance) is attributed to the COMPUTE frontier, not word-specific
		// debt. The tier-2 bucket is for rows refused by a WORD-CLASS gap the
		// compiler does not model (rootCause "coverage": code-body higher-order
		// words like flex's `walk`, the test harness's `test-check-prop`). usurp
		// itself COMPILES (`add/u 1 2` → 3); the path-modifier rows `m.a/u 1 2`
		// refuse on "operand provenance" (rootCause "soundness") — usurp of a
		// MAP-STORED fn, the exact dynamic-fn-application frontier its non-usurp
		// sibling `m.a 1 2` also refuses on, and which the usurp `why` explicitly
		// excludes from its residual ("quote/codequote or a non-fn target"). They
		// surfaced INTO this partition only because the checker false-positive
		// fixes (15b9fb1: flex / stored-fn-ref) moved them from check-error
		// (uncounted) to refused value rows — real debt newly visible, not a
		// compiler regression, and it belongs to the frontier they actually hit.
		soundnessFrontier := tier == 2 && rootCause(bucket) == "soundness"
		switch {
		case tier == 1:
			c.interp++
			c.tier1By[name]++
		case expectErr || errorRowReason(reason):
			c.errorRows++
		case tier == 2 && !soundnessFrontier:
			c.reducible++
			c.tier2By[name]++
		default:
			c.computeGap++
			r := bucket
			if reason == "" {
				r = "island (OpFallback span)"
			}
			c.computeBy[r]++
			c.computeRows = append(c.computeRows, firstN(input, 88)+" — "+r)
		}
	}
	return c, nil
}

// add folds one file's census into c: every count and histogram summed,
// every list appended.
func (c *census) add(p *census) {
	c.rows += p.rows
	c.compiled += p.compiled
	c.islanded += p.islanded
	c.checkErr += p.checkErr
	c.refused += p.refused
	addCounts(c.refusalBuckets, p.refusalBuckets)
	c.refusedRows = append(c.refusedRows, p.refusedRows...)
	c.interp += p.interp
	c.reducible += p.reducible
	c.errorRows += p.errorRows
	c.computeGap += p.computeGap
	c.computeRows = append(c.computeRows, p.computeRows...)
	addCounts(c.tier1By, p.tier1By)
	addCounts(c.tier2By, p.tier2By)
	addCounts(c.computeBy, p.computeBy)
}

// addCounts sums src's histogram into dst.
func addCounts(dst, src map[string]int) {
	for k, n := range src {
		dst[k] += n
	}
}

// firstN truncates a spec input for the compute-gap row log.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
