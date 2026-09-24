// sweep_gate_test.go — the generated sweep's gates: S0 of
// design/FULL-COMPILATION-REPLAN.0.md (FULL-COMPILATION-REVIEW.0.md §3.5).
//
// The corpus is a sample and under-measures by construction: 710 rows of
// ordinary idioms took the compile-failure count from 0 to 113. The sweep
// (test/go/sweep) is the instrument the corpus is not — one program per
// declaration-relevant word × operand kind, hand-written in seeds.tsv,
// classified through the dual pipeline and re-embedded in every call form
// of vary's transform table. Its word × kind matrix is committed as
// SWEEP_STATUS.md (informational, like COMPILED_STATUS.md; refresh with
// `make sweep-status`), and its counts are the gates here: every one an
// END STATE of 0 and a REGRESSION ceiling that is the live value, BOTH
// ways, asserted under a corpus filter too because none of them is a
// corpus count. A divergence is a miscompile and fails every lane unless
// pinned to its NUR in sweepKnownMiscompiles, itself pinned both ways.
package langspec

import (
	"os"
	"strings"
	"testing"

	native "github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/test/go/sweep"
	"github.com/boru-lang/boru/test/go/vary"
)

const sweepStatusFile = "SWEEP_STATUS.md"

// The REGRESSION ceilings (lanes_test.go; every end state 0), each the live
// value on the day it was set, moving only with the cell that moved it named
// here. The instrument's own holes (empty, invalid, crashes) are at their
// end state and stay there; the compiler's debt is the rest. History: set
// 2026-09-18 with the sweep's first run on main after PR #472 — 53 words,
// 305 cells, 138 passing; the 44 compile failures and 5 islands are the
// fn-value and code-body families the corpus already names, plus the ones
// only the sweep sees (SWEEP_STATUS.md lists every cell).
const (
	sweepEmptyCeiling          = 0   // word × kind cells with no seed and no n/a probe — holes in the instrument
	sweepInvalidCeiling        = 0   // seeds the interpreter rejects — a seed to fix, or an n/a to claim with a probe
	sweepFailureCeiling        = 29  // valid seeds that FAIL to compile or hard-error in CompileCheck — every one a BUG. 30 -> 29 on 2026-09-23 (NUR181, the def-bound closure park): the `word` × factory seed (`def dbl word (mk) end 5 dbl`) compiles with parity — the program residual's ordering treats a parked result as data, and a splice's payload is marked re-stepped so the trailing arm applies it. Before: 31 -> 30 on 2026-09-22 (the dynamic-lead group): the `apply` × container seed (`def m {f: ([n:Integer] => [n add 1])} end 5 m.f/v apply`) compiles — the gradual apply EVENT records at the program level too — and all fourteen of its call-form variants pass with it. Before: 44 -> 36 on 2026-09-19 (S1a): each/fold/scan/filter × factory and × container poly re-match. 36 -> 31 on 2026-09-19 (the fallback removal): five seeds that used to be classified as failures now compile and RUN — the classifier read the try-mode fallback's error as a compile failure, and with one outcome it reads the real one
	sweepIslandCeiling         = 2   // valid seeds that compile with an interpreter island: inner ×2. 5 -> 2 on 2026-09-19 (S1a): scan × lambda, named-fn and module-export lower to a poly re-match instead of an island
	sweepCrashCeiling          = 0   // valid seeds an engine PANICS on or never answers — recovered or abandoned by the classifier; the worst kind of defect
	sweepVariantFailureCeiling = 197 // call-form variants of passing seeds that fail to compile (islanded or check-reject). 201 -> 197 on 2026-09-23 (the placed value inside a code body): four variants GRADUATE — `afn` factory · do-body and · do-catch, `word` lambda · do-body and · do-catch — a factory's returned closure or a lambda value placed inside a do body is data on both lanes and the body compiles (they declined on the twin regime's bind placement); their each-body twins still decline (an arm-resident def of unknown provenance). Before: 200 -> 201 on 2026-09-23 (NUR159, the branch result's re-step): the `if` × named-fn seed (`def one fn [[][Integer][1]] end if true one/v [2]`) GRADUATED from DIVERGED to passing, so its fourteen call forms are counted for the first time: thirteen pass and one — for-body — declines in the conditional-redefinition family (`fn 'one' redefined inside a conditional body`), an existing decline, not new debt; the same increment turned the two DIVERGED `def` × container variants (fn-body and lambda-body, a `CALL_DYNAMIC underflow` error divergence) into passing ones, which changes no failure count. Before: 195 -> 200 on 2026-09-23 (NUR181, the def-bound closure park): four variants of passing seeds compile now (`afn` factory and container · prefix-stack, `parse` named-fn and module-export — a parked result above a literal seats), and the `word` × factory seed GRADUATED from failing to passing, so its fourteen call forms are counted for the first time: nine decline in existing families (the closure render, a lambda body's unapplied fn value, the twin regime, a branch leaving extra values, a conditional redefinition, the trailing apply's quote state) — 195 - 4 + 9, no new debt. Before: 194 -> 195 on 2026-09-22 (NUR156): the `apply` × module-export seed (`5 M.inc/v apply`) GRADUATED from DIVERGED to passing, so its fourteen call forms are counted for the first time, and one — each-body — declines in the twin-regime family (a bind transition with no stream placement inside a multi-run body), an existing decline, not new debt. 200 -> 194 on 2026-09-22 (the container-member calls): six variants PASS — force-arity, forward-args and usurp × container, each under paren-group and module-body — the paren-bounded apply of a modifier-wrapped container member records through the fn-typed lead admission (recordParenLeadingApply admits a lead whose static type is a fn beside the member-read tag). Before: 200 = 200 on 2026-09-22 (S1b's apply shapes + NUR177): two variants PASS (afn factory · module-body — the transitive render rule, unitRenderKnown; afn container · suffix-def) and two DIVERGED variants became loud declines (afn container · prefix-stack — NUR161, closed as a side effect of NUR177's fresh residual identities; def container · module-body — the SWAP-underflow internal_error, now "fn-value application bounded by a paren"), so the count is unchanged and the silent divergences are gone. Before: 200 -> 206 on 2026-09-19 (S1a), then 206 -> 200 on 2026-09-19 (the fallback removal), same cause as sweepFailureCeiling: eleven cells started passing and brought 154 new variants, six of which fail — each/filter/fold/scan × factory and scan × named-fn under for-body (the factory redefined inside the loop, the conditional-shadow compile failure), and scan × module-export under each-body (a twin-regime placement) — and no variant that passed before fails now (the sets were diffed)
	sweepVariantCrashCeiling   = 2   // call-form variants an engine PANICS on or never answers: word/lambda under paren-group and module-body (NUR162) — its own ceiling, so a crash can never hide inside the failure count
)

// sweepPin is one known miscompile: the NUR that records it, and the
// divergence it shows — the prefix of vary's detail, both lanes' answers —
// so a program whose WRONG ANSWER CHANGES is a new finding, not a known one.
type sweepPin struct{ nur, detail string }

// sweepKnownMiscompiles keys a diverging program — a cell's seed or one of
// its call-form variants, by source — to its pin. Pinned both ways: an
// unlisted divergence is a NEW miscompile and fails every lane; a listed
// one that stops diverging is retired with its fix; a listed one that
// diverges differently fails until the change is explained and re-pinned.
var sweepKnownMiscompiles = map[string]sweepPin{
	// Surfaced 2026-09-19 by the removal of the interpreter fallbacks: the
	// whole-program re-run had been answering this row correctly, so the
	// classifier never saw the divergence underneath.
	`import "boru:emitlang" end def m {up: (fn [[value:Any opts:Map] [String] ['UP']])} end emit m.up {a:1}`: {
		"NUR170 — a container-read fn value at a word's operand slot arrives transposed with the Map beside it",
		"error divergence: compiled [boru/signature_error]: cannot call `emitlang-auto` — no signature matches the arguments"},
	// The first run of the sweep, 2026-09-18 — every one recorded in NUR.md.
	`import module [def cl fn [[][List][[1 'one' 2 'two' 'many']]] export "M" {cl: cl/v}] end case 2 M.cl`: {
		"NUR154 — `case` lowers its clause list as a static literal, so a clause list a module fn returns is never read",
		"error divergence: compiled [boru/case_error]: case: clause list must be a concrete list of match/block pairs (optional trailing default)"},
}

// sweepInventory is the matrix's rows: every declaration-relevant word of
// the default registry (the declaration census's relevant()), classed Body
// when any of its relevant signatures takes a code body, a callable or a
// fn operand, and Quoted when they only quote an operand.
func sweepInventory(t *testing.T) map[string]sweep.Class {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	inv := map[string]sweep.Class{}
	for _, name := range reg.RegisteredWordNames() {
		fd := reg.Lookup(name)
		if fd == nil {
			continue
		}
		for i := range fd.Signatures {
			ok, why := relevant(&fd.Signatures[i])
			if !ok {
				continue
			}
			if why != "quoted" {
				inv[name] = sweep.Body
			} else if _, seen := inv[name]; !seen {
				inv[name] = sweep.Quoted
			}
		}
	}
	return inv
}

func TestGeneratedSweep(t *testing.T) {
	t.Parallel()
	inv := sweepInventory(t)
	seeds, err := sweep.Seeds()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sweep.Orphans(inv, seeds) {
		t.Errorf("seeds.tsv: %s %s is not a cell of the matrix — the word is not declaration-relevant in the default registry, or the kind is not one its class admits; delete the line", s.Word, s.Kind)
	}

	cells := sweep.Run(inv, seeds, nil)
	seen := map[string]bool{}
	for _, c := range cells {
		switch c.Status() {
		case sweep.StaleNA:
			t.Errorf("%s: the n/a probe is ACCEPTED by the interpreter, so the language expresses this cell — drop the n/a and make it a real seed: %s", c.Key(), c.Seed.Src)
		case sweep.Diverged:
			sweepMiscompile(t, c.Key()+" (the seed)", c.Seed.Src, c.Base.Detail, seen)
		}
		for _, v := range c.Variants {
			if v.Res.Outcome == vary.Diverged {
				sweepMiscompile(t, c.Key()+" · "+v.Transform, v.Src, v.Res.Detail, seen)
			}
		}
	}
	for src, pin := range sweepKnownMiscompiles {
		if !seen[src] {
			t.Errorf("stale sweepKnownMiscompiles pin — this program no longer diverges; retire the entry with the fix that closed it (was: %s):\n  %.160s", pin.nur, src)
		}
	}

	counts := sweep.Count(cells)
	variantFailures := counts.Variants[vary.Declined] + counts.Variants[vary.Islanded] + counts.Variants[vary.CheckReject]
	variantCrashes := counts.Variants[vary.Panicked] + counts.Variants[vary.Hung]
	gateAssert(t, "sweep empty cells", counts.Cells[sweep.Empty], 0, sweepEmptyCeiling, true,
		"word × operand-kind cells of the generated sweep with no seed program — holes in the instrument (test/go/sweep/seeds.tsv)", true)
	gateAssert(t, "sweep invalid seeds", counts.Cells[sweep.Invalid], 0, sweepInvalidCeiling, true,
		"seed programs the interpreter rejects — fix the seed, or claim n/a with a probe", true)
	gateAssert(t, "sweep compile failures", counts.Cells[sweep.Failed]+counts.Cells[sweep.CheckReject], 0, sweepFailureCeiling, true,
		"valid seed programs that FAIL to compile — every one a BUG; SWEEP_STATUS.md lists them", true)
	gateAssert(t, "sweep islands", counts.Cells[sweep.Islanded], 0, sweepIslandCeiling, true,
		"valid seed programs that compile with an interpreter island", true)
	gateAssert(t, "sweep crashes", counts.Cells[sweep.Panicked]+counts.Cells[sweep.Hung], 0, sweepCrashCeiling, true,
		"valid seed programs an engine PANICS on or never answers — recovered or abandoned by the classifier so the sweep goes on", true)
	gateAssert(t, "sweep call-form failures", variantFailures, 0, sweepVariantFailureCeiling, true,
		"call-form variants of passing seeds that fail to compile or island", true)
	gateAssert(t, "sweep call-form crashes", variantCrashes, 0, sweepVariantCrashCeiling, true,
		"call-form variants of passing seeds an engine PANICS on or never answers", true)
	t.Logf("generated sweep: %d cells — pass %d, failed %d, islanded %d, diverged %d, panicked %d, hung %d, check-reject %d, invalid %d, n/a %d, empty %d; %d call-form variants — pass %d, declined %d, islanded %d, diverged %d, panicked %d, hung %d, interp-reject %d, check-reject %d",
		len(cells), counts.Cells[sweep.Pass], counts.Cells[sweep.Failed], counts.Cells[sweep.Islanded], counts.Cells[sweep.Diverged], counts.Cells[sweep.Panicked], counts.Cells[sweep.Hung], counts.Cells[sweep.CheckReject], counts.Cells[sweep.Invalid], counts.Cells[sweep.NotApplicable], counts.Cells[sweep.Empty],
		sumVariants(counts), counts.Variants[vary.Pass], counts.Variants[vary.Declined], counts.Variants[vary.Islanded], counts.Variants[vary.Diverged], counts.Variants[vary.Panicked], counts.Variants[vary.Hung], counts.Variants[vary.InterpReject], counts.Variants[vary.CheckReject])

	want := sweep.Render(cells)
	if os.Getenv("BORU_WRITE_SWEEP") != "" {
		if err := os.WriteFile(sweepStatusFile, []byte(want), 0o644); err != nil {
			t.Fatalf("write %s: %v", sweepStatusFile, err)
		}
		t.Logf("wrote %s (%d bytes)", sweepStatusFile, len(want))
		return
	}
	got, err := os.ReadFile(sweepStatusFile)
	if err != nil {
		t.Fatalf("read %s: %v (run `make sweep-status` to generate it)", sweepStatusFile, err)
	}
	if string(got) != want {
		// Informational, as COMPILED_STATUS.md is: the COUNTS above are the
		// gate, both ways; the list is the ledger the later steps read, and
		// `make sweep-status` refreshes it.
		t.Logf("%s is stale (the sweep moved) — refresh with `make sweep-status`", sweepStatusFile)
	}
}

// sweepMiscompile reports one diverging program: an error unless pinned
// with the divergence it shows.
func sweepMiscompile(t testing.TB, where, src, detail string, seen map[string]bool) {
	t.Helper()
	// A compiled run that BAILED is not a divergence the sweep just found:
	// it is the defect the interpreter re-run used to absorb, and it is
	// counted in its own ledger (compiled_defect_test.go). The classifier
	// hands us rendered text rather than the error, so the marker
	// compiledRunError attaches is what identifies it.
	if strings.Contains(detail, "this is a compiler defect") {
		// Classified, not counted: the ceiling counts CORPUS rows, and a
		// sweep seed is not one. The sweep's own gates carry its numbers.
		seen[src] = true
		directionFailure(t, "%s: the compiled run bails — %s", where, sweepBailReason(detail))
		return
	}
	seen[src] = true
	pin, known := sweepKnownMiscompiles[src]
	switch {
	case !known:
		t.Errorf("MISCOMPILE — %s diverges from the interpreter:\n  program: %s\n  %s\ntriage: shrink and fix, or record the non-uniformity in NUR.md and pin the program with its divergence in sweepKnownMiscompiles (never leave a divergence unpinned)", where, src, detail)
	case !strings.HasPrefix(detail, pin.detail):
		t.Errorf("MISCOMPILE CHANGED — %s is pinned to %s as\n  %s\nand now diverges as\n  %s\nA different wrong answer is a new finding: re-read the NUR, and re-pin only with the change explained there", where, pin.nur, pin.detail, detail)
	default:
		directionFailure(t, "%s: known miscompile (%s)", where, pin.nur)
	}
}

// sweepBailReason pulls the VM's own message out of the rendered divergence
// text, so the ledger groups sweep bails by site like every other row.
func sweepBailReason(detail string) string {
	i := strings.Index(detail, "bytecode: internal: ")
	if i < 0 {
		return "bail (site not named in the rendered detail)"
	}
	r := detail[i:]
	if j := strings.Index(r, " (pc="); j >= 0 {
		r = r[:j]
	}
	return r
}

// TestSweepMiscompilePinsTheDivergence pins sweepMiscompile's three arms:
// an unpinned divergence is an error, a pinned one whose divergence matches
// is not, and a pinned one that diverges DIFFERENTLY is an error again.
func TestSweepMiscompilePinsTheDivergence(t *testing.T) {
	t.Parallel()
	// The seed is the register's oldest open pin (NUR154); the `if` ×
	// named-fn cell that seeded this test until 2026-09-23 agrees now
	// (NUR159 FIXED).
	src := `import module [def cl fn [[][List][[1 'one' 2 'two' 'many']]] export "M" {cl: cl/v}] end case 2 M.cl`
	pinned := sweepKnownMiscompiles[src].detail

	rec := &recTB{}
	seen := map[string]bool{}
	sweepMiscompile(rec, "x (the seed)", "1 add 2", "value divergence: compiled [4] vs interp [3]", seen)
	if len(rec.errs) != 1 || !strings.HasPrefix(rec.errs[0], "MISCOMPILE — x (the seed)") || !seen["1 add 2"] {
		t.Errorf("unpinned: errs = %q", rec.errs)
	}

	rec = &recTB{}
	sweepMiscompile(rec, "case/module-fn (the seed)", src, pinned+" (and then some)", seen)
	if rec.failed || len(rec.logs) != 1 || !strings.Contains(rec.logs[0], "known miscompile (NUR154") || !seen[src] {
		t.Errorf("pinned, matching: errs = %q logs = %q", rec.errs, rec.logs)
	}

	rec = &recTB{}
	sweepMiscompile(rec, "case/module-fn (the seed)", src, "value divergence: compiled [2] vs interp [1]", seen)
	if len(rec.errs) != 1 || !strings.HasPrefix(rec.errs[0], "MISCOMPILE CHANGED — case/module-fn (the seed) is pinned to NUR154") {
		t.Errorf("pinned, changed: errs = %q", rec.errs)
	}
}

func sumVariants(c sweep.Counts) int {
	n := 0
	for _, v := range c.Variants {
		n += v
	}
	return n
}
