package lang

import (
	"runtime"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
)

// allocsPerOp runs fn a fixed number of times and returns the mean heap
// allocations per call. Allocation counts are deterministic for a given
// compiled Program, so a small fixed sample is exact and fast (no
// benchmark auto-scaling).
func allocsPerOp(fn func()) int64 {
	allocs, _ := allocStatsPerOp(fn)
	return allocs
}

// allocStatsPerOp is allocsPerOp with the allocated BYTES per call too.
// Byte counts catch the regression class alloc counts miss: one big
// allocation per op (e.g. a fresh ~1024-entry tape per body invocation)
// moves the needle by a single alloc but hundreds of KB.
func allocStatsPerOp(fn func()) (allocs, bytes int64) {
	const iters = 64
	fn() // warm up (lazy one-time allocations)
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for i := 0; i < iters; i++ {
		fn()
	}
	runtime.ReadMemStats(&m1)
	return int64(m1.Mallocs-m0.Mallocs) / iters, int64(m1.TotalAlloc-m0.TotalAlloc) / iters
}

// Compiled-mode allocation guard (design/legacy/boru-bytecode-plan.0.ignore Stage 6
// verification). Allocations per RunProgram are DETERMINISTIC, so they
// are the hard regression signal (execution time is GC-noisy and only
// advisory). Each compute/island shape has a ceiling pinned slightly
// above its measured allocations; a change that regresses compiled-mode
// allocation — e.g. removing the island sub-engine reuse, or adding a
// per-dispatch allocation — trips the ceiling here, in `make test`, long
// before anyone runs the benchmarks. Lower a ceiling when an
// optimization reduces allocations; never raise one without a documented
// reason.
func TestCompiledAllocCeilings(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc guard runs a micro-benchmark; skipped under -short")
	}
	if eng.VMArgsDebugBuild {
		// The ceilings are calibrated for the release build's per-run args
		// scratch reuse; -tags borudebug allocates a fresh args slice per
		// CALL_NATIVE on purpose, so its higher counts are expected.
		t.Skip("alloc ceilings are release-build calibrated; skipped under -tags borudebug")
	}
	guards := []struct {
		name    string
		setup   string
		src     string
		ceiling int64
	}{
		{"arith_chain64", "", arithChain(64), 210},
		{"compare_loop", "", `for 200 [gt i 100]`, 1150},
		{"if_scalar", "", `for 200 [if (gt i 100) [1] [0]]`, 1150},
		{"if_listcond", "", `for 200 [if [i gt 100] [1] [0]]`, 1150},
		{"for_tight", "", `for 200 [add (mul i 3) 7]`, 1950},
		{"recursion_nontail", `def s fn [[n:Integer] [Integer] [if (n lte 0) [0] [n add (s (n sub 1))]]] end`, `s 200`, 2450},
		{"recursion_tail", `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] end`, `s2 1000 0`, 13000},
		// Island reuse keeps a hot island in a loop at interpreter-level
		// allocation; a regression that re-allocates a sub-engine per
		// island would sharply raise this (the pre-engine-pool level was
		// ≈4800, and a per-island sub-engine would roughly double that).
		// Island reuse keeps this at interpreter-level allocation; the
		// per-body CONTEXT FRAME the VM pushes (vmContext.enterBodyUnit) adds
		// one alloc per invocation on top. Ceiling lowered 500 → 400 when the
		// frame landed: at 500 the frame consumed two thirds of the headroom
		// SILENTLY, which is precisely the objection
		// design/verse-report-defects-investigation.0.md §B blocker 3 raised
		// against an earlier attempt. A tighter ceiling is what makes the next
		// increment visible.
		{"do_body", `def body [add 1 2]`, `for 100 [do body]`, 400},
		// The three rows blocker 3 asked for by name: "the guard table has NO
		// each/fold/filter row, so the largest regression class is uncovered."
		// These are the words whose bodies run ONCE PER ELEMENT, so a
		// per-invocation cost multiplies here in a way `do` cannot show.
		{"each_scalar", "", `each [add 1] [1 2 3 4 5 6 7 8 9 10]`, 110},
		{"fold_scalar", "", `fold [add] [1 2 3 4 5 6 7 8 9 10] 0`, 110},
		{"filter_scalar", "", `filter [gt 2] [1 2 3 4 5 6 7 8 9 10]`, 120},
	}
	for _, g := range guards {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if g.setup != "" {
			if _, err := a.RunInterp(g.setup); err != nil {
				t.Fatalf("%s setup: %v", g.name, err)
			}
		}
		prog, reason, _, err := a.CompileCheck(g.src)
		if err != nil || prog == nil {
			t.Fatalf("%s did not compile (reason %q) — alloc guard needs a compiled program", g.name, reason)
		}
		reg := a.registry
		got := allocsPerOp(func() {
			if _, err := eng.RunProgram(prog, reg); err != nil {
				t.Fatalf("%s: %v", g.name, err)
			}
		})
		if got > g.ceiling {
			t.Errorf("%s: %d allocs/op exceeds ceiling %d — compiled-mode allocation regressed", g.name, got, g.ceiling)
		} else {
			t.Logf("%s: %d allocs/op (ceiling %d)", g.name, got, g.ceiling)
		}
	}
}
