// bind_twin_emission_test.go gates §6.5's inert-emission stage: every
// compiled program's bind-twin table equals the check pass's own bind ledger,
// entry for entry.
//
// The twins are fired from NoteBindTransition itself — the ledger's single
// funnel, after its suppressions — so equality here is not a tautology: what
// this catches is a RECORDER-LIFECYCLE hole, an entry noted while a swapped
// or isolated recorder was installed (the dynamic-help eval's IsolateEmit, a
// module sub-pass, a fork), which would silently drop a twin the ledger
// keeps. A twin table missing one transition is a registry the
// rollback-and-replay flip would reproduce wrongly — the same class of
// late-surfacing divergence the depth-composition and live-depth oracles
// exist to catch early.
//
// NO ALLOWANCE, same as the oracles: fix the recorder lifecycle, never the
// number.
package langspec

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// twinRowMismatch compiles one row and reports a mismatch line, or "" when
// the row does not compile or the tables agree.
func twinRowMismatch(src, where string) string {
	a, err := lang.New()
	if err != nil {
		return fmt.Sprintf("%s lang.New: %v", where, err)
	}
	prog, _, res, _ := a.CompileCheck(src)
	if prog == nil {
		return ""
	}
	s := src
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	if len(prog.BindTwins) != len(res.BindLedger) {
		return fmt.Sprintf("%s twins=%d ledger=%d  %s", where, len(prog.BindTwins), len(res.BindLedger), s)
	}
	for i, tw := range prog.BindTwins {
		ld := res.BindLedger[i]
		if tw.Kind != ld.Kind || tw.Name != ld.Name || tw.Depth != ld.Depth || tw.Pos != ld.Pos {
			return fmt.Sprintf("%s entry %d: twin=%+v ledger=%+v  %s", where, i, tw, ld, s)
		}
	}
	return ""
}

// TestBindTwinsEqualLedger runs the emission gate over the corpus and the
// synthetic rows.
func TestBindTwinsEqualLedger(t *testing.T) {
	t.Parallel()
	for _, src := range syntheticBranchArmSources {
		if bad := twinRowMismatch(src, "synthetic"); bad != "" {
			t.Errorf("%s", bad)
		}
	}

	var mu sync.Mutex
	compiled, withTwins, mismatched := 0, 0, 0
	var worst []string
	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		src := r.Input
		a, aerr := lang.New()
		if aerr != nil {
			return
		}
		prog, _, res, _ := a.CompileCheck(src)
		if prog == nil {
			return
		}
		where := fmt.Sprintf("%s:%d", r.File, r.Line)
		bad := ""
		if len(prog.BindTwins) != len(res.BindLedger) {
			bad = fmt.Sprintf("%s twins=%d ledger=%d  %s", where, len(prog.BindTwins), len(res.BindLedger), src)
		} else {
			for i, tw := range prog.BindTwins {
				ld := res.BindLedger[i]
				if tw.Kind != ld.Kind || tw.Name != ld.Name || tw.Depth != ld.Depth || tw.Pos != ld.Pos {
					bad = fmt.Sprintf("%s entry %d: twin=%+v ledger=%+v", where, i, tw, ld)
					break
				}
			}
		}

		mu.Lock()
		defer mu.Unlock()
		compiled++
		if len(prog.BindTwins) > 0 {
			withTwins++
		}
		if bad != "" {
			mismatched++
			if len(worst) < 15 {
				worst = append(worst, bad)
			}
		}
	})
	sort.Strings(worst) // the mismatch worklist reads the same whichever worker saw a row first

	t.Logf("bind-twin emission: %d compiled rows, %d with twins, %d mismatched", compiled, withTwins, mismatched)
	for _, w := range worst {
		t.Logf("    %s", w)
	}
	if compiled == 0 || withTwins == 0 {
		t.Fatal("no compiled row carried a twin table — the gate is measuring its own wiring")
	}
	if mismatched > 0 {
		t.Errorf("%d rows where the program's twin table diverges from the pass's ledger: "+
			"a recorder-lifecycle hole ate or duplicated a twin (§6.5)", mismatched)
	}
	_ = core.BindDef // keep the core import for the shared helpers' package
}

// twinOpIdxs collects each OpBindTwin instruction's table index from one code
// unit, in instruction order.
func twinOpIdxs(code []compiler.Instr) []int {
	var idxs []int
	for _, in := range code {
		if in.Op == compiler.OpBindTwin {
			idxs = append(idxs, int(in.Arg))
		}
	}
	return idxs
}

// TestBindTwinOpsArePlacedOrderedSubset gates the PLACEMENT half of the inert
// emission: every OpBindTwin instruction indexes a real table entry, and the
// placed indices are STRICTLY INCREASING within each code unit — the stream
// order is the table's (= the ledger's = the pass's) order. Placement is a
// SUBSET of the table by design at this stage: a transition noted while
// recording was suspended (an each/fold body's leaking def) has no stream
// home until its twin becomes arm-resident, and an op recorded inside a
// discarded island vanishes with it — the FLIP is where every unplaced twin
// must either gain an op or decline the program, and this gate is what that
// compile failure logic will tighten.
func TestBindTwinOpsArePlacedOrderedSubset(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	rowsWithOps, opsTotal, twinsTotal, bad := 0, 0, 0, 0
	specWalk(t, func(t testing.TB, r specRow) {
		if len(r.Cells) < 2 {
			return
		}
		src := r.Input
		a, aerr := lang.New()
		if aerr != nil {
			return
		}
		prog, _, _, _ := a.CompileCheck(src)
		if prog == nil {
			return
		}
		units := [][]compiler.Instr{prog.Code}
		for i := range prog.Fns {
			units = append(units, prog.Fns[i].Code)
		}
		rowOps, rowBad, sawOp := 0, 0, false
		for _, code := range units {
			idxs := twinOpIdxs(code)
			if len(idxs) > 0 {
				sawOp = true
			}
			rowOps += len(idxs)
			for i, idx := range idxs {
				if idx < 0 || idx >= len(prog.BindTwins) {
					rowBad++
					t.Errorf("%s:%d: BIND_TWIN arg %d outside the %d-entry table",
						r.File, r.Line, idx, len(prog.BindTwins))
				} else if i > 0 && idx <= idxs[i-1] {
					rowBad++
					t.Errorf("%s:%d: BIND_TWIN order broken (%d after %d) — the stream must "+
						"replay transitions in the pass's order", r.File, r.Line, idx, idxs[i-1])
				}
			}
		}

		mu.Lock()
		defer mu.Unlock()
		twinsTotal += len(prog.BindTwins)
		opsTotal += rowOps
		bad += rowBad
		if sawOp {
			rowsWithOps++
		}
	})
	t.Logf("bind-twin placement: %d rows with ops, %d ops placed over %d table entries", rowsWithOps, opsTotal, twinsTotal)
	if rowsWithOps == 0 || opsTotal == 0 {
		t.Fatal("no OpBindTwin was placed anywhere — the placement is not wired")
	}
	if bad > 0 {
		t.Errorf("%d placement violations", bad)
	}
}
