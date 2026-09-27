package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR263CommittedBakeRaisesTheReport pins NUR263's close. The pass
// matches `fold` optimistically over a fn's declared-Any result and bakes
// the overload it picked over the compiled body closure (tryRecordClosure);
// the handler is robust to the sibling collection, and a live value no
// overload takes makes it refuse with a bare signature_error, where the
// interpreter raises the word's full report. The dispatch's exact layout
// now rides to the baked call (core optimisticLayout, SigRef.Split): on the
// handler's refusal the VM lays the operands out as the interpreter's tape,
// the body slot as its token list, and raises the plan's report. Both lanes
// agree, notes and help included, in every exact position; a List or a Map
// still folds.
func TestNUR263CommittedBakeRaisesTheReport(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[][Any]["s"]] end 0 fold [add] (mk)`,
		`def mk fn [[][Any][5]] end (0 fold [add] (mk))`,
		`def mk fn [[][Any][5]] end [0 fold [add] (mk)]`,
		`def mk fn [[][Any][5]] end def r (0 fold [add] (mk)) end r`,
		`def mk fn [[][Any][5]] end fold [add] (mk) 0`,
		`def mk fn [[][Any][5]] end (mk) 0 fold [add]`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || codeOf(errI) != "signature_error" || !strings.Contains(fmt.Sprint(errI), "= note: the arguments were") {
			t.Errorf("%q: want the interpreter's full report, compiled=%v, got %v %v", src, compiled, gotI, errI)
			continue
		}
		if fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) {
			t.Errorf("%q:\ncompiled    %v %v\ninterpreted %v %v", src, gotC, errC, gotI, errI)
		}
	}
	for _, src := range []string{
		`def mk fn [[][Any][[1 2]]] end 0 fold [add] (mk)`,
		`def mk fn [[][Any][{a:1 b:2}]] end 0 fold [add] (mk)`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != "[3]" || fmt.Sprint(gotI) != "[3]" {
			t.Errorf("%q: 3 on both lanes, compiled=%v, got %v %v / %v %v", src, compiled, gotC, errC, gotI, errI)
		}
	}
	// The bake stands: the committed CALL_NATIVE over the body closure.
	if dis := compileDisasm(t, `def mk fn [[][Any]["s"]] end 0 fold [add] (mk)`); !strings.Contains(dis, "PUSH_CLOSURE") || !strings.Contains(dis, "CALL_NATIVE s") {
		t.Errorf("the closure bake stays committed; got:\n%s", dis)
	}
}
