package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

const stage5Mod = `import module [def fact fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [fact (n sub 1) (acc add n)]]] export "MU" {fact: fact/v}] `

// TestModuleRecursionGetsTCO pins the Stage-5 reality discovered by
// tracing: module preamble fns are InstallFnDef'd in the MODULE
// registry, so intra-module tail recursion dispatches through
// execMatch inside CallBoru's sub-engine and Stages 1-4b already apply
// — one CallBoru boundary crossing per entry, O(1) tape inside. The
// counters live on the MODULE registry (each import universe has its
// own); the exported fn value carries it.
func TestModuleRecursionGetsTCO(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	values, err := parser.Parse(stage5Mod + `MU.fact 50 0 MU.fact/v`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := native.NewTop(reg).Run(values)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 2 || res[0].String() != "1275" {
		t.Fatalf("res = %v, want [1275 <fn>]", res)
	}
	fd, ok := core.FnDefOf(res[1].Data())
	if !ok || fd.Registry == nil {
		t.Fatalf("MU.fact/v did not return a module fn value: %T", res[1].Data())
	}
	if reg.TCO.Detected != 0 {
		t.Errorf("main registry detected %d; module recursion runs on the module registry", reg.TCO.Detected)
	}
	// Depth 50: the entry call crosses via CallBoru (not a tail call,
	// no enclosing frame); the first in-body call has no frame above
	// the unwrapped CallBoru body (declined); the remaining 49 recurse
	// frame-to-frame and replace.
	if fd.Registry.TCO.Detected != 49 || fd.Registry.TCO.Replaced != 49 {
		t.Errorf("module registry detected/replaced = %d/%d, want 49/49",
			fd.Registry.TCO.Detected, fd.Registry.TCO.Replaced)
	}
}

// TestModuleRecursionDeepConstantSpace runs the module chain at depth
// 10000: every frame-to-frame call replaces, so the sub-engine's tape
// stays O(1) (the eng-level taxonomy test pins replacement ⇒ O(1);
// the counter here pins that replacement actually fires through the
// module boundary at depth).
func TestModuleRecursionDeepConstantSpace(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	values, err := parser.Parse(stage5Mod + `MU.fact 10000 0 MU.fact/v`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := native.NewTop(reg).Run(values)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 2 || res[0].String() != "50005000" {
		t.Fatalf("res[0] = %v, want 50005000", res)
	}
	fd, _ := core.FnDefOf(res[1].Data())
	if fd.Registry == nil || fd.Registry.TCO.Replaced != 9999 {
		t.Fatalf("module replaced = %d, want 9999", fd.Registry.TCO.Replaced)
	}
}

// TestModuleKillSwitchPropagates: a host disabling elision on the
// parent registry expects module frames to nest too — RunModuleBody
// copies TCO.Disable onto the module registry.
func TestModuleKillSwitchPropagates(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.TCO.Disable = true
	values, err := parser.Parse(stage5Mod + `MU.fact 50 0 MU.fact/v`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := native.NewTop(reg).Run(values)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res[0].String() != "1275" {
		t.Fatalf("result with Disable = %v, want 1275 (semantics never depend on TCO)", res[0])
	}
	fd, _ := core.FnDefOf(res[1].Data())
	if fd.Registry == nil {
		t.Fatal("no module registry on the exported fn")
	}
	if fd.Registry.TCO.Elided != 0 || fd.Registry.TCO.Replaced != 0 {
		t.Errorf("module fired %d/%d with the parent kill switch on, want 0/0",
			fd.Registry.TCO.Elided, fd.Registry.TCO.Replaced)
	}
	if fd.Registry.TCO.Detected != 49 {
		t.Errorf("module detected = %d, want 49 (detection stays on under Disable)", fd.Registry.TCO.Detected)
	}
}
