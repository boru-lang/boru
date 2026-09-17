package lang

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	eng "github.com/boru-lang/boru/eng/go"
)

// Stage 6 island reuse (design/legacy/boru-bytecode-plan.0.ignore Stage 6): the VM
// reuses ONE sub-engine across every OpFallback in a RunProgram,
// reloading its tape in place rather than allocating a fresh engine+tape
// per island — a hot fallback island in a loop is no longer a
// compiled-mode regression (do_body: ~2x faster, allocations halved).
// Correctness rides on the reused engine carrying NO state across island
// executions: each island in a loop, fed a VARYING threaded input, must
// produce its own result, identical to the interpreter.
func TestCompiledIslandReuseNoStateLeak(t *testing.T) {
	cases := []string{
		// Varying threaded receiver per iteration (iota i): each island
		// over a different list — a leaked tape/stack would corrupt these.
		`for 4 [each [mul 2] (iota i)]`,
		// Repeated identical island in a tight loop (the do_body shape).
		`for 3 [do [add 1 2]]`,
		// Chained islands (each -> size) in a loop, varying.
		`for 4 [size (each [add 1] (iota i))]`,
	}
	for _, src := range cases {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		out, compiled, err := a.RunCompiled(src)
		if err != nil || !compiled {
			t.Fatalf("%q: compiled=%v err=%v", src, compiled, err)
		}
		b, _ := New()
		iout, _ := b.RunInterp(src)
		if len(out) != len(iout) {
			t.Fatalf("%q: length %d != %d (%v vs %v)", src, len(out), len(iout), out, iout)
		}
		for i := range out {
			if out[i] != iout[i] {
				t.Fatalf("%q at %d: compiled=%v interpreter=%v", src, i, out, iout)
			}
		}
	}
}

// Stage 5 concurrency (design/legacy/boru-bytecode-plan.0.ignore §Stage 5): a
// compiled Program and all its tables (Code, Consts, Types, Sigs,
// Fallbacks, Fns, Debug) are IMMUTABLE after compile, so concurrent
// executions can share one *Program. All mutable VM state — the
// operand stack, locals, frames, loop state — is allocated per call
// inside RunProgram, and each goroutine runs against its OWN forked
// registry (ForkConcurrent), so no two executions touch the same
// mutable scope.
//
// This is the race-detector gate: run under `go test -race`. A shared
// Program driven from many goroutines must produce identical results
// with no data race. Programs span the compiled surface — straight-line
// natives, a counted loop, a user fn with tail recursion, a baked
// interpreter island, and a threaded computed-receiver island.
func TestCompiledConcurrencyRaceFree(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`add 1 (mul 2 3)`, "7"},
		{`for 4 [i mul 2]`, "0 2 4 6"}, // one residual value per iteration
		{`def f fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [f (n sub 1) (acc add n)]]] f 10 0`, "55"},
		{`each [mul 2] [1 2 3]`, "[2 4 6]"},    // baked island
		{`each [mul 2] (iota 4)`, "[0 2 4 6]"}, // threaded island
		// await: per-branch fork units (CompileStoresBodyList) — every program
		// run spawns branch goroutines that RunUnit the SHARED Program's units
		// on their forks, so this is the nested-concurrency stamp gate.
		{`import "boru:time-util" TimeUtil.await [[1 add 2] [3 mul 4]]`, "[3 12]"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.src, func(t *testing.T) {
			// Compile ONCE; the resulting Program is shared read-only.
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck(%q): %v", c.src, err)
			}
			if prog == nil {
				t.Fatalf("%q did not compile (reason %q) — concurrency gate needs a compiled program", c.src, reason)
			}

			const goroutines = 16
			const iters = 40
			var wg sync.WaitGroup
			errs := make([]error, goroutines)
			mismatch := make([]string, goroutines)
			for g := 0; g < goroutines; g++ {
				wg.Add(1)
				go func(g int) {
					defer wg.Done()
					for i := 0; i < iters; i++ {
						// Each execution gets its own isolated registry;
						// the *Program is shared across all of them.
						fork := a.registry.ForkConcurrent()
						out, rerr := eng.RunProgram(prog, fork)
						if rerr != nil {
							errs[g] = rerr
							return
						}
						if got := renderResidual(out); got != c.want {
							mismatch[g] = got
							return
						}
					}
				}(g)
			}
			wg.Wait()
			for g := 0; g < goroutines; g++ {
				if errs[g] != nil {
					t.Fatalf("goroutine %d: %v", g, errs[g])
				}
				if mismatch[g] != "" {
					t.Fatalf("goroutine %d: got %q, want %q", g, mismatch[g], c.want)
				}
			}
		})
	}
}

// renderResidual renders a residual engine stack the way convertResults
// does, joined into one comparable string.
func renderResidual(vs []core.Value) string {
	out := convertResults(vs)
	parts := make([]string, len(out))
	for i, v := range out {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " ")
}

// The concurrency race regression pin (CI -race, PR #233): an ORDINARY
// top-level fn's unit must carry NO Reg override — the VM runs it on whatever
// registry RunProgram is handed (a ForkConcurrent fork per goroutine), so a
// baked shared-registry pointer would defeat isolation and race on
// ensureInvoker. Only a FOREIGN module sub-registry fn is stamped. Without the
// progReg discriminator every fn carried the shared compile registry and
// TestCompiledConcurrencyRaceFree tripped the detector.
func TestOrdinaryFnUnitsCarryNoRegOverride(t *testing.T) {
	src := `def f fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [f (n sub 1) (acc add n)]]] each [mul 2] [1 2 3] f 3 0`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("did not compile: reason=%q err=%v", reason, cerr)
	}
	if len(prog.Fns) == 0 {
		t.Fatal("expected at least the recursive fn + each-body units")
	}
	for i := range prog.Fns {
		if prog.Fns[i].Reg != nil {
			t.Errorf("unit %d (%s): ordinary fn carries a Reg override — a shared registry pointer would race under ForkConcurrent",
				i, prog.Fns[i].Name)
		}
	}
}
