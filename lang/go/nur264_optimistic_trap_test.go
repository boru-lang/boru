package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR264TrapUnderAnOptimisticMatch pins NUR264's close: the check pass
// matches `each` / `filter` OPTIMISTICALLY over a declared-Any operand (its
// type is wider than the slot's) and auto-evaluates the data list — `[dup]`,
// `[gt 1]` — whose inner dispatch fails statically, so it recorded a
// top-level trap and the compiled program raised the inner word's no-match
// unconditionally. The interpreter evaluates the list only once the outer
// word MATCHES: over 5 or a Map it raises each's no-match first. The trap is
// now the outer word's runtime rematch (core CheckState.OptimisticOuter,
// compiler recordGuardedTrap): its no-match raises the outer error, and a
// match raises the recorded inner one (DispatchSpec.OnMatch). Both lanes
// raise the same error, notes and caret included, for either runtime value.
func TestNUR264TrapUnderAnOptimisticMatch(t *testing.T) {
	for _, c := range []struct{ src, word string }{
		{`def mk fn [[][Any][5]] end each (mk) [dup]`, "each"},
		{`def mk fn [[][Any][{a:1}]] end each (mk) [dup]`, "each"},
		{`def mk fn [[][Any][5]] end filter (mk) [gt 1]`, "filter"},
		{`def mk fn [[][Any][5]] end each (mk) [1 add]`, "each"},
		{`def mk fn [[][Any][[1 2]]] end each (mk) [dup]`, "dup"},
		{`def mk fn [[][Any][[1 2]]] end filter (mk) [gt 1]`, "gt"},
		{`def mk fn [[][Any][[1 2]]] end each (mk) [1 add]`, "add"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || len(gotC) != 0 || len(gotI) != 0 || codeOf(errI) != "signature_error" {
			t.Errorf("%q: want signature_error on both lanes, got compiled=%v %v %v, interp %v %v", c.src, compiled, gotC, errC, gotI, errI)
			continue
		}
		if !strings.Contains(detailOf(errI), "cannot call `"+c.word+"`") {
			t.Errorf("%q: the interpreter's error names %s: %v", c.src, c.word, errI)
		}
		if fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: compiled\n%v\ninterpreted\n%v", c.src, errC, errI)
		}
	}
	// The guard is the outer word's rematch, with the inner error attached.
	if dis := compileDisasm(t, `def mk fn [[][Any][5]] end each (mk) [dup]`); !strings.Contains(dis, "DISPATCH_REMATCH") {
		t.Errorf("the trap is guarded by each's rematch; got:\n%s", dis)
	}
}
