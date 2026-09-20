// The COLLECT oracle over the whole corpus — the first EXECUTION of the
// region table (design/FULL-COMPILATION.0.md §6.2, §6.5; the OpCollect doc).
//
// Every value row is compiled with compiler.RegionOracle armed, so the
// lowerer places an OpCollect before each dispatch Phase B described, and
// run compiled; the VM walks every descriptor live and reports whether the
// walk reproduces the record (core.RegionOracleEvent). This lane tallies
// the outcomes. Two of them are findings and fail the lane: an OVER-claim
// (the record says the dispatch took more forward than the live walk would
// give it — the miscompile direction) and a VALUE divergence (a live word
// slot's binding is not what the lowering pushed — a frozen read seen at
// run time). The rest are counted and logged: reproduced, under-claimed
// (Phase B stopped early — the safe direction), declined (the host cannot
// evaluate a group or an interpolation) and unbound (the lead has no live
// binding in the registry the VM consulted).
//
// The lane is its own walk rather than a rider on the differential because
// it changes what is COMPILED (the oracle ops), and the differential's
// count must stay the default lane's.
package langspec

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// regionOracleExampleCap bounds how many rows each finding kind names in the
// failure; the tally carries the totals.
const regionOracleExampleCap = 12

// regionOracleReproducedFloor is the ratchet on the outcome that matters:
// how many executed descriptors the live walk reproduced. UP only — a fall
// means a seat stopped describing, or the walk stopped agreeing.
const regionOracleReproducedFloor = 47000 // 47110 (2026-09-15, the sixty-second increment)

// regionOracleFindings is the LEDGER of the two finding kinds — every
// over-claim and every value divergence the lane knows, keyed by row and
// site, each with the mechanism behind it. Pinned in BOTH directions: a
// finding not in the ledger fails the lane (a new disagreement), and a
// ledger entry the walk no longer produces fails it too (the entry is stale
// and must be retired with the fix that retired it). The two kinds:
//
//   - a predicate-typed param the check pass claims optimistically and the
//     runtime scan rejects (`f 5` with n:Even): an ERROR row on both lanes,
//     and the descriptor records the plan the check pass made (NUR141).
//   - a fn-body read of a MODULE-SCOPE flex binding pushed as a fresh
//     clone (`PUSH_CONST_FRESH`) of the check pass's snapshot, not the
//     binding: `keys sift-catalog` in boru:sift's `Sift.kinds`. The keys
//     agree today because the check pass performs the same mutations the
//     run does (a dry-passed `set` on a concrete flex) and a mutation
//     between requests makes the next compile DECLINE (the memo's
//     materialisation guard); the operand is still not the object the
//     interpreter reads (NUR143). Found the moment the agreement test
//     became identity (review of #458).
//
// RETIRED (the sixty-third increment, NUR140 resolved): the twin-carrier
// class — six rows where a top-level `def` of a COMPUTED compound (`def b
// [add 1 2]`, `def l (Log.logger "http")`, the span and counter handles)
// replayed the check pass's MODEL of the value (`[Integer]`, a module
// prototype) because IsConcrete read the model as a real value and no
// OpBindGlobal write-back was emitted. The write-back is now decided by
// provenance (compiler/go/lower.go's rootBindWritesBack), every one of the
// six reproduces, and the class is pinned across requests in
// lang/go/bytecode_globalbind_test.go.
var regionOracleFindings = map[string]string{
	"module-sift.tsv:L69 keys@1078:16": "module-flex snapshot: `keys sift-path-detect` reads a fresh clone of the check pass's flex, not the binding (NUR143)",
	"module-sift.tsv:L74 keys@1042:55": "module-flex snapshot: `keys sift-catalog` reads a fresh clone of the check pass's flex, not the binding (NUR143)",
	"fnpred.tsv:L50 f@1:80":            "a predicate param (n:Even) claimed by the check pass, rejected by the runtime scan; an ERROR row on both lanes",
	// NUR152's reverse-direction rows: a MAIN-program fn (`pub`, or the `=>`
	// lambda) applied INSIDE a module fn (`M.run`). Its unit is compiled at
	// its home (main) from inside `run`'s foreign compile, and its free word
	// `secret` is a LIVE word-slot read against the unit's registry at run
	// time (the program's, by enterUnit) rather than a baked const — so the
	// record carries the name and the walk the resolved value. Both engines
	// answer 6; the finding is the read's provenance, not its value.
	"module-composition.tsv:L142 add@1:139": "NUR152: main's `secret` read live by name inside a foreign-compiled unit; both engines answer 6",
	"module-composition.tsv:L143 add@1:158": "NUR152: main's `secret` read live by name inside a foreign-compiled unit (module has its own `secret`); both engines answer 6",
	"module-composition.tsv:L144 add@1:148": "NUR152: the lambda form of L143; both engines answer 6",
}

type regionOracleTally struct {
	mu       sync.Mutex
	outcomes map[string]int
	examples map[string][]string
	// findings collects every over-claim and value divergence by ledger key
	// (the row's file and line, the word and its position), for the
	// two-way pin against regionOracleFindings.
	findings map[string]string
}

// ledgerKey names one finding the way the ledger does: file:Lline word@row:col.
func ledgerKey(row string, ev lang.RegionOracleEvent) string {
	return fmt.Sprintf("%s %s@%d:%d", strings.SplitN(row, ": ", 2)[0], ev.Word, ev.Pos.Row, ev.Pos.Col)
}

func (tl *regionOracleTally) note(row string, ev lang.RegionOracleEvent) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.outcomes[ev.Outcome]++
	switch ev.Outcome {
	case "over-claimed", "diverged-value":
		tl.findings[ledgerKey(row, ev)] = fmt.Sprintf("%s\n      %s @%d:%d NFwd %d: %s", row, ev.Word, ev.Pos.Row, ev.Pos.Col, ev.NFwd, ev.Detail)
	case "under-claimed", "unbound":
		if len(tl.examples[ev.Outcome]) < regionOracleExampleCap {
			tl.examples[ev.Outcome] = append(tl.examples[ev.Outcome],
				fmt.Sprintf("%s\n      %s @%d:%d NFwd %d: %s", row, ev.Word, ev.Pos.Row, ev.Pos.Col, ev.NFwd, ev.Detail))
		}
	}
}

func TestRegionCollectOracle(t *testing.T) {
	// No t.Parallel(): compiler.RegionOracle is a package global the
	// lowerer reads at every emit site, so no other test may compile while
	// it is armed. The WALK is specWalk, parallel across files: the oracle
	// hook is a holder on each instance's own registry
	// (core.Registry.ArmRegionOracleHook), inherited only into the
	// instance's module sub-registries, so every row's events reach the
	// tally (mutex-guarded) through the row's own instance. The registry a
	// unify is armed with is threaded through the kernel's unify calls
	// (core/go/unify.go) — it once sat on a package-global stack two
	// workers raced on, which this walk was the first to expose — so no
	// two rows share any state now.
	compiler.RegionOracle = true
	defer func() { compiler.RegionOracle = false }()

	tl := &regionOracleTally{outcomes: map[string]int{}, examples: map[string][]string{}, findings: map[string]string{}}
	var mu sync.Mutex
	var rows, compiled, errored int
	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		input := r.Input
		mu.Lock()
		rows++
		mu.Unlock()
		a := newDifferentialInstance(t)
		row := fmt.Sprintf("%s: %s", r.Key(), input)
		disarm := a.ArmRegionOracleHook(func(ev lang.RegionOracleEvent) { tl.note(row, ev) })
		_, wasCompiled, errC := a.RunCompiled(input)
		disarm()
		if !wasCompiled {
			return
		}
		mu.Lock()
		compiled++
		mu.Unlock()
		if errC == nil {
			return
		}
		// An error under the oracle must be the PROGRAM's own — the
		// interpreter raises it too (the differential's error-parity
		// half, applied here because the default lane never arms the
		// oracle) and it is never the VM's internal one. A row that
		// errored was swallowed by the first draft and its descriptors
		// after the fault went uncounted, inside the floor's slack
		// (found in review of #458).
		mu.Lock()
		errored++
		mu.Unlock()
		// A compiled BAIL is the defect the interpreter re-run used to
		// absorb — counted in its own ledger, not reported here as though
		// the oracle had found it.
		if compiledDefect(errC) {
			return
		}
		if strings.Contains(errC.Error(), "internal_error") {
			t.Errorf("%s: an internal error under the oracle: %v", row, errC)
			return
		}
		if _, errI := newDifferentialInstance(t).RunInterp(input); errI == nil {
			divergence(t, "region-oracle", r.Key(), fmt.Sprintf("errored under the oracle where the interpreter does not: %v", errC))
		}
	})
	for _, k := range []string{"under-claimed", "unbound"} {
		sort.Strings(tl.examples[k]) // the sample reads the same whichever worker saw a row first
	}

	keys := make([]string, 0, len(tl.outcomes))
	for k := range tl.outcomes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var total int
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		total += tl.outcomes[k]
		parts = append(parts, fmt.Sprintf("%s %d", k, tl.outcomes[k]))
	}
	t.Logf("region collect oracle: %d rows, %d compiled (%d erroring, each the program's own), %d descriptors executed: %s", rows, compiled, errored, total, strings.Join(parts, ", "))
	for _, k := range []string{"under-claimed", "unbound"} {
		if n := len(tl.examples[k]); n > 0 {
			t.Logf("%s, first %d:\n  %s", k, n, strings.Join(tl.examples[k], "\n  "))
		}
	}
	if filteredCorpus() {
		return // a subset carries neither the ledger's rows nor the floor
	}
	// The findings, two ways against the ledger.
	for key, detail := range tl.findings {
		if _, known := regionOracleFindings[key]; !known {
			t.Errorf("a finding the ledger does not know — the live walk disagrees with the record in the direction that miscompiles:\n  %s\n  (add it to regionOracleFindings with its mechanism, or fix it)", detail)
		}
	}
	for key, why := range regionOracleFindings {
		if _, seen := tl.findings[key]; !seen {
			t.Errorf("ledger entry %q (%s) no longer reproduces — retire it with the change that fixed it", key, why)
		}
	}
	if n := tl.outcomes["reproduced"]; n < regionOracleReproducedFloor {
		t.Errorf("reproduced %d is below the floor %d — a seat stopped describing, or the walk stopped agreeing", n, regionOracleReproducedFloor)
	}
	if total == 0 {
		t.Error("the oracle executed no descriptor — the lane is not armed")
	}
}
