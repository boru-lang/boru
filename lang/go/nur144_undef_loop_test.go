package lang

import (
	"fmt"
	"strings"
	"testing"
)

// NUR144's fence: an `undef` of a pre-loop module binding inside a
// top-level loop body is DROPPED by the compiled program — no twin, no op —
// so the binding survives the loop where the interpreter pops it. Pinned as
// the divergence it is, so the fix that places (or refuses) the transition
// retires this test with the NUR entry. Both seats of the read, a stack
// operand and a routed forward slot.
func TestNUR144UndefInLoopBodyIsDropped(t *testing.T) {
	for _, src := range []string{
		`def k 5 end def f fn [[][Integer][k add 2]] end for 2 [ f  undef k ]`,
		`def k 5 end def go fn [[][Integer][add k 1]] end for 2 [ go  undef k ]`,
	} {
		gotC, compiled, errC, _, errI := runBothEngines(t, src)
		if !compiled || errC != nil || len(gotC) != 2 || fmt.Sprint(gotC[0]) != fmt.Sprint(gotC[1]) {
			t.Errorf("%q: NUR144 says the compiled run keeps the binding (two equal answers): compiled=%v got=%v err=%v", src, compiled, gotC, errC)
		}
		if errI == nil || !strings.Contains(errI.Error(), "k") {
			t.Errorf("%q: the interpreter pops the binding and the second iteration fails on k: %v", src, errI)
		}
	}
}
