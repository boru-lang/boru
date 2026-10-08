package lang

import (
	"fmt"
	"testing"
)

// module_fn_unit_registry_test.go pins NUR143's close (2026-09-24): the
// enclosing-binding snapshot taken at a fn unit's open reads the unit's
// OWN registry (compiler StartFnCompile's fnReg — a module fn's
// sub-registry), not the recorder's binding. The binding follows the
// running engine: after a module's body ran on a sub-engine it was that
// engine's registry (a nested import's, when the module imports one of
// its own — boru:sift imports three), and once BindRegistry restores it
// is the program's; in neither is the module-scope flex a module fn's
// body reads a binding, so the read baked as a fresh clone of the check
// pass's snapshot — the interpreter reads the binding, and a fill from
// one call was invisible to the next (`[1 0]` for `[1 1]`, silent). The
// langspec region oracle ledgered the two boru:sift descriptors by name;
// they read the binding live now and the entries are retired.

const modFlexUnit = `import module [def reg (flex {}) end def put fn [[k:String v:Integer][Integer][def _s (reg set (k) v) end v]] end def peek fn [[k:String][Integer][reg get k]] end def count fn [[][Integer][size (keys reg)]] end export "R" {put: put/v peek: peek/v count: count/v}] end `

var moduleFnUnitRegistryRows = []struct {
	label, src, want string
}{
	{"a module fn reads the flex another module fn filled", modFlexUnit + `R.put 'a' 1  R.put 'b' 2  R.peek 'b'`, "[1 2 2]"},
	{"the flex's keys after the fills", modFlexUnit + `R.put 'a' 1  R.count`, "[1 1]"},
	{"the flex read before any fill is the module's empty one", modFlexUnit + `R.count`, "[0]"},
	// The module's body imports a module of its own first (sift imports
	// three), so the recorder's binding after the import is neither the
	// program's nor this module's registry, restore or no restore.
	{"a nested inline import before the flex", `import module [import module [def z fn [[][Integer][1]] end export "Z" {z: z/v}] end def reg (flex {}) end def put fn [[k:String v:Integer][Integer][def _s (reg set (k) v) end v]] end def peek fn [[k:String][Integer][reg get k]] end def count fn [[][Integer][size (keys reg)]] end export "R" {put: put/v peek: peek/v count: count/v}] end R.put 'a' 1  R.count`, "[1 1]"},
	{"a nested native import before the flex", `import module [import "boru:string-util" end def reg (flex {}) end def put fn [[k:String v:Integer][Integer][def _s (reg set (k) v) end v]] end def peek fn [[k:String][Integer][reg get k]] end def count fn [[][Integer][size (keys reg)]] end export "R" {put: put/v peek: peek/v count: count/v}] end R.put 'a' 1  R.peek 'a'  R.count`, "[1 1 1]"},
	{"the sift module itself", `import "boru:sift"  Sift.define m {family:'kv' detect:{path:["/proc/meminfo"]}} end  Sift.detect "/proc/meminfo"`, "[m]"},
}

func TestModuleFnUnitReadsModuleScopeFlexAfterBody(t *testing.T) {
	for _, row := range moduleFnUnitRegistryRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(row.src)
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != row.want {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
	}
}
