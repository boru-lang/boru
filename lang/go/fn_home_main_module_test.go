package lang

import (
	"fmt"
	"testing"
)

// A fn value carries the registry that MINTED it, so its free words resolve
// at home whichever module applies it — in BOTH directions. The module→main
// direction landed with design/FUNCTION-VALUE-SCOPE.0.md rule 1 (2026-08-15);
// the main→module direction did not, because only module EXPORTS were
// stamped with a home: a main-file fn carried nil, and FnHome's nil arm handed
// it whatever registry happened to be running — the module's. Measured before
// the fix (NUR152): row 0 was `cannot call add` interpreted and 6 compiled; row
// 1 was 105 on BOTH engines — main's `pub` silently reading the MODULE's
// `secret`, invisible to any differential.
func TestMainFnValueAppliedInsideModuleResolvesAtHome(t *testing.T) {
	rows := []string{
		// the module has no `secret`: main's must resolve
		`import module [def run fn [[f:Function][Integer][(f 5)]] export "M" {run: run/v}] end
def secret 1 end
def pub fn [[x:Integer][Integer][x add secret]] end
M.run pub/v`,
		// the module has its OWN `secret`: main's fn must still read main's
		`import module [def secret 100 end def run fn [[f:Function][Integer][(f 5)]] export "M" {run: run/v}] end
def secret 1 end
def pub fn [[x:Integer][Integer][x add secret]] end
M.run pub/v`,
		// lambda form of the same
		`import module [def secret 100 end def run fn [[f:Function][Integer][(f 5)]] export "M" {run: run/v}] end
def secret 1 end
M.run ([x:Integer] => [x add secret])`,
		// a main-file FACTORY applied inside the module: the closure it returns
		// is minted in main's frame, so its unit compiles at main too (the
		// fn-value unit's home, not the emitter's registry mid-foreign-compile).
		// The module hands the closure back and main applies it: applying it
		// inside the module body is a compile error today ("unmatched dispatch
		// recovered at apply"), a separate defect owed its own fix
		`import module [def secret 100 end def run2 fn [[f:Function][Function][(f 5)]] export "M" {run2: run2/v}] end
def secret 1 end
def mk fn [[k:Integer][Function][([y:Integer] => [y add k add secret])]] end
1 (M.run2 mk/v) apply`,
		// the documented direction, as the control: a module fn applied from main
		// reads the MODULE's secret
		`import module [def secret 100 end def pub fn [[x:Integer][Integer][x add secret]] export "M" {pub: pub/v}] end
def secret 1 end
(M.pub 5)`,
	}
	want := []string{"[6]", "[6]", "[6]", "[7]", "[105]"}
	for i, src := range rows {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
		if errI != nil {
			t.Errorf("row %d: interpreter error: %v", i, errI)
			continue
		}
		if got := fmt.Sprint(gotI); got != want[i] {
			t.Errorf("row %d = %s, want %s", i, got, want[i])
		}
	}
}
