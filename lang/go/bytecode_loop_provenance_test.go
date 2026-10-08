package lang

import (
	"fmt"
	"testing"
)

// A loop body ending on a MODULE INSTANCE bound by a module-scope def was
// the minimal in-repo shape of a value the for-lowering could not seat — no
// producing event, no frame local, not materialisable as a const — and this
// test pinned the "for: body result of unknown provenance" compile failure in both
// net arms. GRADUATED 2026-09-05: a module-family value reads LIVE
// (dynScopeRescue's module-family arm, OpLookupDynScope) and a root def of
// one needs no bind op (lowerDynBind), so both shapes compile with parity.
// The for-lowering's residual settlement (residualStands) keeps its pins
// through the residual-order hazard rows in analysis_order_test.go.
func TestForBodyModuleValueCompiles(t *testing.T) {
	cases := []struct{ name, src string }{
		{"multi-value residual", `def m (module [export "X" {a: 1}]) for 2 [9 m]`},
		{"single-value", `def m (module [export "X" {a: 1}]) for 2 [m]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck: %v", err)
			}
			if prog == nil {
				t.Fatalf("must compile natively (the module value reads live); declined: %q", reason)
			}

			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotC, compiled, err := b.RunCompiled(c.src)
			if noteCompileDefect(t, c.src, gotC, err) {
				return
			}
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if !compiled {
				t.Fatal("the program must run on the compiled lane")
			}
			i, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotI, err := i.RunInterp(c.src)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if fmt.Sprintf("%v", gotC) != fmt.Sprintf("%v", gotI) {
				t.Errorf("lane parity: compiled %v != interp %v", gotC, gotI)
			}
		})
	}
}
