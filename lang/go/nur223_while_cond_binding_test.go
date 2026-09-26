package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR223WhileConditionBindingDeclines pins NUR223: a `while` whose
// CONDITION binds a name declines the compile, loudly (a booked compile
// defect), where it compiled to a silent wrong answer — the body is analysed
// before the condition, so its read of the name resolved the pre-loop
// binding (`[0 0 3]` for `[1 2 3]`). The decline is conservative: a
// condition that binds a fresh name the body never reads declines too.
func TestNUR223WhileConditionBindingDeclines(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def t 0 end while [def t (t add 1) (t lt 3)] [t] end t`, "[1 2 3]"},
		{`while [def k 1 false] [7] end k`, "[1]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if compiled || !strings.Contains(fmt.Sprint(errC), "NUR223") {
			t.Errorf("%q: a binding condition declines (NUR223), got compiled=%v %v %v", c.src, compiled, gotC, errC)
		}
		requireCompileDefect(t, c.src, gotC, errC)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: the interpreter answers %s, got %v %v", c.src, c.want, gotI, errI)
		}
	}
	// Negative: a condition that only READS compiles with parity — a body
	// rebinding what the condition reads is exactly what the body-first
	// order serves.
	for _, src := range []string{
		`def n 0 end while [n lt 3] [def n (n add 1) n] end n`,
		`def t 0 end while [t lt 3] [def t (t add 1)] end t`,
		`while [false] [1]`,
	} {
		requireEngineParity(t, src, true)
	}
}
