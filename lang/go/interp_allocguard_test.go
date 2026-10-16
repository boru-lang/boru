package lang

import (
	"runtime"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// Interpreter-mode allocation guard. Allocations per interpreted execution of a
// pre-parsed value stream are deterministic, so they are a hard
// regression signal for the dispatch machinery (matchSignature, forward
// collection, per-call stacks) that stays on the interpreted path.
// Each shape has a ceiling pinned slightly above its measured
// allocations; a change that adds a per-dispatch allocation trips the
// ceiling here, in `make test`, long before anyone runs the benchmarks.
// Lower a ceiling when an optimization reduces allocations; never raise
// one without a documented reason.
func TestInterpAllocCeilings(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc guard runs a micro-benchmark; skipped under -short")
	}
	// byteCeiling catches the regression class the alloc COUNT misses: one
	// big allocation per op. The pre-engine-pool interpreter allocated a
	// fresh ~1024-entry tape (~164KB) per body invocation — each_list ran
	// at ~17MB/op with barely more allocs than today. A change that
	// removes sub-engine pooling trips the byte ceiling immediately.
	guards := []struct {
		name        string
		setup       string
		src         string
		ceiling     int64
		byteCeiling int64
	}{
		{"arith_chain64", "", arithChain(64), 886, 600_000},
		// Every ceiling but string_join's LOWERED 2026-10-15 by its measured
		// drop from no-copy phase 3 (phase 4, design/VALUE-NO-COPY.0.md §9):
		// a value's datum is shared, scalars and markers mint none, and the
		// engine's per-step markers allocate their payload with their datum.
		//
		// compare_loop / if_scalar / for_tight / map_get / string_join LOWERED
		// 2026-10-12 by their measured drop (no-copy phase 2,
		// design/VALUE-NO-COPY.0.md §9): a loop's continuation mark no longer
		// copies the body it never replays, one allocation per iteration.
		{"compare_loop", "", `for 200 [gt i 100]`, 4368, 1_850_000},
		{"if_scalar", "", `for 200 [if (gt i 100) [1] [0]]`, 7539, 3_700_000},
		{"for_tight", "", `for 200 [add (mul i 3) 7]`, 6908, 2_950_000},
		// each / fold LOWERED 2026-10-08 (1950 -> 1400, 1360 -> 870): the loop
		// runs on the tape (core loop.go) — one sealed region per element,
		// the loop's tokens minted once — instead of a pooled sub-engine run
		// per element, whose entry, inputs and result copy allocated.
		{"each_list", `def xs100 ` + intList(100), `each [mul 2] xs100`, 1301, 400_000},
		{"fold_int", `def xs100b ` + intList(100), `fold [add] xs100b 0`, 811, 300_000},
		{"map_get", `def mg {a: {b: {c: {d: 1}}}}`, `for 100 [mg.a.b.c.d]`, 6288, 3_300_000},
		{"string_join", `def ws ['alpha' 'beta' 'gamma' 'delta']`, `for 50 [join '-' ws]`, 18290, 2_050_000},
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
		vals, err := parser.Parse(g.src)
		if err != nil {
			t.Fatalf("%s parse: %v", g.name, err)
		}
		reg := a.registry
		got, gotBytes := allocStatsPerOp(func() {
			if _, err := native.NewTop(reg).Run(vals); err != nil {
				t.Fatalf("%s: %v", g.name, err)
			}
		})
		if got > g.ceiling {
			t.Errorf("%s: %d allocs/op exceeds ceiling %d — interpreter allocation regressed", g.name, got, g.ceiling)
		} else if gotBytes > g.byteCeiling {
			t.Errorf("%s: %d bytes/op exceeds ceiling %d — interpreter allocation regressed", g.name, gotBytes, g.byteCeiling)
		} else {
			t.Logf("%s: %d allocs/op (ceiling %d), %d bytes/op (ceiling %d)", g.name, got, g.ceiling, gotBytes, g.byteCeiling)
		}
	}
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
