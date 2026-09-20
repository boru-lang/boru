package lang

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestTierFeasibilityProbe validates the two load-bearing claims of
// design/INTERPRETER-TIERED-EXECUTION.0.md §Implementation findings with
// the machinery as it exists today:
//
//  1. A synthetic call `fnname __tier0` compiles against a pre-installed
//     dynamic-carrier binding — the emitter routes the argument read
//     through dynScopeRescue (runtime name lookup) and compiles the fn
//     as a normal unit, with no new emit machinery.
//  2. The resulting Program executes with a REAL argument bound under
//     the reserved name, returning the same value the interpreter
//     computes — when invoked OUTSIDE an in-flight interpreter run
//     (RunProgram's entry guard; relaxing it for nested tier dispatch
//     is T1's critical path and deliberately NOT exercised here).
//
// PROBE RESULT (2026-07): the shape does NOT yet compile — the emitter
// declines with "fn call operand of unknown provenance" (an fn-call
// operand must trace to a producing event or a local slot; a
// pre-installed dynamic binding has neither). This test PINS that
// compile failure as the current seam behavior: T1's first task is to give the
// tier's synthetic-call entry a real operand provenance, at which point
// this pin flips into the positive compile+run+differential assertion
// (kept below in comments as the target shape).
func TestTierFeasibilityProbe(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunInterp(`def tpfib fn [[n:Integer] [Integer] [if (n lte 1) [n] [(tpfib (n sub 1)) add (tpfib (n sub 2))]]]`); err != nil {
		t.Fatalf("install fn: %v", err)
	}

	// Pre-install the reserved argument name as a dynamic carrier so the
	// compile pass sees a gradual binding (not a foldable constant).
	core.InstallDef(a.registry, "__tier0", core.NewDynamicCarrier(core.TInteger))
	defer core.UninstallDef(a.registry, "__tier0")
	prog, reason, _, err := a.CompileCheck(`tpfib __tier0`)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog != nil {
		t.Fatal("the tier synthetic call now COMPILES — flip this probe to the positive assertion: " +
			"bind __tier0 to a real value, eng.RunProgram, and differential-check against the interpreter " +
			"(see design/INTERPRETER-TIERED-EXECUTION.0.md T1)")
	}
	if reason != "fn call operand of unknown provenance" {
		t.Fatalf("compile failure reason drifted: %q — update the pin (and the tier design's task list)", reason)
	}
}
