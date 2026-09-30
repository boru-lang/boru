package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// effect_census_test.go pins which module natives declare an observable
// effect (core.CompileSideEffect, NUR356): a random draw, the clock, the
// terminal, the network, the vault, a test's record, a debug print, a
// sub-run — and which stay quiet.

func TestEffectCensusModules(t *testing.T) {
	parent, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	quiet := map[string]bool{"rand-with-seed": true, "vm-parse": true, "vm-check": true, "vm-compile": true}
	groups := map[string][]native.NativeFunc{
		"rand":      randNativesForState(newRandState(1)),
		"debug":     debugNatives(),
		"dashboard": dashboardNatives(),
		"step":      stepNatives(),
		"codec":     codecNatives(),
		"socket":    socketNatives(),
		"test":      testNatives(parent),
		"cover":     coverNatives(parent),
		"tui":       tuiTier1Natives(),
		"tui-run":   tuiRunNatives(),
		"tui-serve": tuiServeNatives(),
		"vault":     vaultNatives(),
		"vm":        vmNatives(parent),
	}
	for g, fns := range groups {
		if len(fns) == 0 {
			t.Errorf("%s: no natives", g)
		}
		for _, n := range fns {
			if got := n.CompileEffect.Has(core.CompileSideEffect); got == quiet[n.Name] {
				t.Errorf("%s %s: effect %v", g, n.Name, got)
			}
		}
	}
	clock := map[string]bool{"time-now-local": true, "time-today": true, "time-today-utc": true, "elapsed": true}
	for _, n := range timeNatives(native.MintTemporalModuleTypes(parent)) {
		if got := n.CompileEffect.Has(core.CompileSideEffect); got != clock[n.Name] {
			t.Errorf("TimeUtil %s: effect %v, a clock read %v", n.Name, got, clock[n.Name])
		}
	}
	for _, n := range append(tuiWidgetNatives(), tuiUtilNatives()...) {
		if n.CompileEffect.Has(core.CompileSideEffect) {
			t.Errorf("tui %s is a pure constructor", n.Name)
		}
	}
}
