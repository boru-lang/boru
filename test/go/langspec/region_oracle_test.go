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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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
//   - the twin-carrier class: a top-level `def` of a COMPUTED value whose
//     twin replays the check-pass binding — the analysis's carrier or
//     prototype (`[Integer]`, a Log.logger prototype with empty fields) —
//     where the lowering pushed the runtime value. IsConcrete reads the
//     compound as concrete, so no OpBindGlobal partner is emitted and the
//     registry holds the prototype for the rest of the run. Unobservable
//     today only because every read of such a def is baked; a LIVE read
//     (the generic lane's, a dynamic body's) would see the prototype. Filed
//     for the twin lowering, not this lane.
//   - a predicate-typed param the check pass claims optimistically and the
//     runtime scan rejects (`f 5` with n:Even): an ERROR row on both lanes,
//     and the descriptor records the plan the check pass made.
var regionOracleFindings = map[string]string{
	"edge-quote-1.tsv:L64 size@1:17":  "twin-carrier: def b [add 1 2] replays [Integer]",
	"module-log.tsv:L52 typeof@1:49":  "twin-carrier: def l (Log.logger \"http\") replays the logger prototype",
	"module-log.tsv:L65 typeof@1:45":  "twin-carrier: def s (Log.span \"op\") replays the span prototype",
	"module-log.tsv:L71 log-end@1:96": "twin-carrier: def s (Log.span \"m\") replays the span prototype",
	"module-log.tsv:L76 typeof@1:47":  "twin-carrier: def c (Log.counter \"x\") replays the counter prototype",
	"module-log.tsv:L84 log-end@1:67": "twin-carrier: def a (Log.span \"a\") replays the span prototype",
	"fnpred.tsv:L50 f@1:80":           "a predicate param (n:Even) claimed by the check pass, rejected by the runtime scan; an ERROR row on both lanes",
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
	compiler.RegionOracle = true
	defer func() { compiler.RegionOracle = false }()

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read %s: %v", specDir, err)
	}
	tl := &regionOracleTally{outcomes: map[string]int{}, examples: map[string][]string{}, findings: map[string]string{}}
	var rows, compiled int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, ferr := os.Open(filepath.Join(specDir, e.Name()))
		if ferr != nil {
			t.Fatalf("open %s: %v", e.Name(), ferr)
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			input := strings.TrimSpace(parts[0])
			rows++
			a := newDifferentialInstance(t)
			row := fmt.Sprintf("%s:L%d: %s", e.Name(), lineNum, input)
			disarm := a.ArmRegionOracleHook(func(ev lang.RegionOracleEvent) { tl.note(row, ev) })
			_, wasCompiled, _ := a.RunCompiled(input)
			disarm()
			if wasCompiled {
				compiled++
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error in %s: %v", e.Name(), err)
		}
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
	t.Logf("region collect oracle: %d rows, %d compiled, %d descriptors executed: %s", rows, compiled, total, strings.Join(parts, ", "))
	for _, k := range []string{"under-claimed", "unbound"} {
		if n := len(tl.examples[k]); n > 0 {
			t.Logf("%s, first %d:\n  %s", k, n, strings.Join(tl.examples[k], "\n  "))
		}
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
