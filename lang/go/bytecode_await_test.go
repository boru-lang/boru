package lang

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// await branch bodies compile per element (CompileStoresBodyList — the spawn
// store-body pattern applied to each parallels element) and run via RunUnit
// on their per-branch forks. Pinned here: compiled parity across the value /
// def-body / raising-branch / full-mode shapes, plus the per-element
// interpreter fallback for a branch the store-body compile declines (a
// Module construction in the body) — the base program's zero-interp-entry
// end state is the p6/concurrent-fork-bodies-on-vm frontier pin.
func TestAwaitCompiledBranchParity(t *testing.T) {
	cases := []string{
		`import "boru:time-util" TimeUtil.await [[1 add 2] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await [[def x 5 x add 1] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await [[raise bad_input "boom"] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await {mode:"full"} [[raise bad_input "boom"] [3 mul 4]]`,
		// The winner-takes-all modes (first/any), GRADUATED (NUR067): their
		// result is the winning branch's whole residual — 0-or-more values,
		// a count that can EXCEED any static seat — and it is now RECORDED as
		// a runtime-variadic REGION (eventFlags.variadicRegion), the same one
		// slot a value-producing loop's region already uses. Three counts, so
		// the region is pinned across the whole range: THREE values where the
		// static seat is one, ONE (the sole surviving branch), and ZERO.
		`import "boru:time-util" TimeUtil.await {mode:"first"} [[1 2 3]]`,
		`import "boru:time-util" TimeUtil.await {mode:"any"} [[raise bad_input "boom"] [3 mul 4]]`,
		`import "boru:time-util" TimeUtil.await {mode:"first"} [[]]`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotC, compiled, err := a.RunCompiled(src)
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if !compiled {
				t.Fatal("await program must run compiled")
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotI, err := b.RunInterp(src)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
				t.Errorf("parity: compiled %v != interp %v", gotC, gotI)
			}
		})
	}
}

// TestAwaitWinnerRegionRefusesFixedArityConsumers — the negative half of
// NUR067's graduation. The region is REPRESENTED now (the wholesale
// MarkUncompilable is gone), so what refuses is each position that genuinely
// needs a STATIC count, one at a time and for its own stated reason:
//
//   - a collecting paren / call operand — layoutOperands sees the lw.variadic
//     slot and refuses, the same wording a loop region gets (this is the
//     shape whose 1-seat layout WAS the live miscompile: `size [(await
//     {mode:'any'} [[7 8]])]` compiled a stranded 7 and a 1-element list
//     where the interpreter answers 2);
//   - a promotion — `def x (await …) x` would store exactly nout values into
//     a frame slot while the run delivers a runtime count;
//   - the dead-result drop — `def _ (await …)` would pop exactly one.
//
// Each refusal below is a SOUND interpreter fallback, so parity is asserted
// alongside it: a refusal that changed the answer would be no better than the
// miscompile it replaced.
func TestAwaitWinnerRegionRefusesFixedArityConsumers(t *testing.T) {
	for _, tc := range []struct{ src, reason, want string }{
		{`import "boru:time-util" size [(TimeUtil.await {mode:"any"} [[7 8]])]`,
			"consumes loop results", "[2]"},
		{`import "boru:time-util" 99 TimeUtil.await {mode:"first"} [[1 2 3]]`,
			"residual shape beyond Stage 1 (call result above a literal)", "[99 1 2 3]"},
		{`import "boru:time-util" def x (TimeUtil.await {mode:"first"} [[1 2 3]]) x`,
			"variadic region promoted to a frame slot", "[2 3 1]"},
		{`import "boru:time-util" def _ (TimeUtil.await {mode:"first"} [[1 2 3]]) 5`,
			"variadic region result discarded", "[2 3 5]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, compiled, err := a.RunCompiled(tc.src)
			if compiled {
				t.Fatal("a fixed-arity consumer of the winner region must refuse: the runtime count is not the static seat")
			}
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("refusal reason drifted: want %q, got %v", tc.reason, err)
			}
			// The fallback answers what the INTERPRETER answers, and the
			// expected value is written out: a refusal that changed the
			// answer would be no better than the miscompile it replaced,
			// and `[99 1 2 3]` in particular is the exact shape the
			// pre-guard trailing-apply lowering got wrong ([1 2 99 3]).
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotI, err := b.RunInterp(tc.src)
			if err != nil {
				t.Fatalf("RunInterp: %v", err)
			}
			if got := fmt.Sprintf("%v", gotI); got != tc.want {
				t.Errorf("interpreter oracle %s, want %s", got, tc.want)
			}
			gotF, err := a.Run(tc.src)
			if err != nil {
				t.Fatalf("Run (fallback): %v", err)
			}
			if fmt.Sprintf("%v", gotF) != fmt.Sprintf("%v", gotI) {
				t.Errorf("fallback parity: %v != interp %v", gotF, gotI)
			}
		})
	}
}

// TestAwaitRefusedBranchInterpretsPerElement — an element the store-body
// compile declines (a Module construction has no operand provenance) keeps
// its raw list: THAT branch runs on the interpreter (an Engine.Run entry
// appears) while the program still runs compiled with interpreter-identical
// results. The sibling compiled branch is unaffected — per-element, sound.
func TestAwaitRefusedBranchInterpretsPerElement(t *testing.T) {
	const src = `import "boru:time-util" TimeUtil.await [[9 (module [export "X" {a:1}]) drop] [3 mul 4]]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu      sync.Mutex
		engRuns int
	)
	disarm := a.ArmInterpEntryHook(func(e InterpEntry) {
		if strings.Contains(e.Seam, "Engine.Run") {
			mu.Lock()
			engRuns++
			mu.Unlock()
		}
	})
	defer disarm()
	gotC, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("RunCompiled: %v", err)
	}
	if !compiled {
		t.Fatal("the program itself must run compiled — only the refused ELEMENT interprets")
	}
	mu.Lock()
	runs := engRuns
	mu.Unlock()
	if runs == 0 {
		t.Error("the Module-bearing branch must fall back to an interpreter sub-engine")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, err := b.RunInterp(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
		t.Errorf("parity: compiled %v != interp %v", gotC, gotI)
	}
}

// The C1 effect fence on a BRANCH unit's internal_error (the runParallelBranch
// twin of RunCompiled's runtime-bail arm): with no observable effect the
// branch re-runs its raw tokens on the interpreter and the program matches
// the interpreter exactly; after an effect the re-run is blocked — the output
// is emitted exactly once and the branch surfaces the internal error as its
// error value (the L-DUP doctrine: no-duplicate-effects beats parity).
func TestAwaitBranchBailBeforeEffectFallsBack(t *testing.T) {
	const src = `import "boru:time-util" TimeUtil.await [[def i (zz-inst) i.m 5 42] [3 mul 4]]`
	a := zzShapedInstance(t)
	a.SetOutput(&bytes.Buffer{})
	gotC, compiled, err := a.RunCompiled(src)
	if err != nil || !compiled {
		t.Fatalf("RunCompiled: compiled=%v err=%v", compiled, err)
	}
	b := zzShapedInstance(t)
	b.SetOutput(&bytes.Buffer{})
	gotI, err := b.RunInterp(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
		t.Errorf("effect-free branch bail must re-run on the interpreter: compiled %v != interp %v", gotC, gotI)
	}
}

func TestAwaitBranchBailAfterEffectSurfaces(t *testing.T) {
	const src = `import "boru:time-util" TimeUtil.await [[print "once" def i (zz-inst) i.m 5 42] [3 mul 4]]`
	a := zzShapedInstance(t)
	var out bytes.Buffer
	a.SetOutput(&out)
	gotC, compiled, err := a.RunCompiled(src)
	if err != nil || !compiled {
		t.Fatalf("RunCompiled: compiled=%v err=%v", compiled, err)
	}
	if out.String() != "once\n" {
		t.Errorf("output = %q, want exactly one %q (no duplicate from a branch re-run)", out.String(), "once\n")
	}
	if s := fmt.Sprintf("%v", gotC); !strings.Contains(s, "internal") {
		t.Errorf("fenced branch must surface the internal error as its value, got %v", gotC)
	}
}
