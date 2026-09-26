package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR210ComputedDoRunBeneathAndCollected pins NUR210's silent half. A
// computed `do` body (a body a fn returned) runs to its residual, and the
// interpreter splices the results back and re-steps them: each value lands
// above what lies beneath, and a trailing fn value applies over it. The
// compiled lane seated the run as its one recorded value. The trailing apply's
// rotation answered [1 9 2] for `9 do (mk)`'s [9 1 2], a list over the run
// assembled [1 [9 2]] and [1 [2]], and a lambda the body placed last stayed
// data. The prefix island re-steps the run through the interpreter's own
// machinery, over the constants beneath it or inside the list that collects
// it.
func TestNUR210ComputedDoRunBeneathAndCollected(t *testing.T) {
	mk := func(body string) string { return `def mk fn [[][List][quote [` + body + `]]] end ` }
	for _, c := range []struct{ src, want string }{
		{mk(`1 2`) + `9 do (mk)`, "[9 1 2]"},
		{mk(`1 2`) + `9 8 do (mk)`, "[9 8 1 2]"},
		{mk(`5 ([x:Integer] => [x add 1])`) + `9 do (mk)`, "[9 6]"},
		{mk(`([x:Integer] => [x add 1])`) + `9 do (mk)`, "[10]"},
		{mk(`7`) + `9 do (mk)`, "[9 7]"},
		{mk(``) + `9 do (mk)`, "[9]"},
		{mk(`1 2`) + `[9 do (mk)]`, "[[9 1 2]]"},
		{mk(`1 2`) + `[do (mk)]`, "[[1 2]]"},
		{mk(`1 2`) + `size [do (mk)]`, "[2]"},
		{mk(`5 ([x:Integer] => [x add 1])`) + `[do (mk)]`, "[[6]]"},
		{mk(`1 2`) + `[9 do (mk)] size`, "[3]"},
		{mk(`1 2`) + `def xs [9 do (mk)] end xs xs`, "[[9 1 2] [9 1 2]]"},
	} {
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: the interpreter answers %s, got %v / %v", c.src, c.want, got, err)
		}
	}
	// The lowering is the island: a mark before the run's chain, the
	// interpreter's re-step over the window, and a list's collect from its
	// own mark.
	if dis := compileDisasm(t, mk(`1 2`)+`9 do (mk)`); !strings.Contains(dis, "STACK_MARK") || !strings.Contains(dis, "CALL_DYN_MIXED_FROM_MARK") {
		t.Errorf("9 do (mk) islands its run over the prefix; got:\n%s", dis)
	}
	if dis := compileDisasm(t, mk(`1 2`)+`[9 do (mk)]`); !strings.Contains(dis, "MAKE_LIST_TO_MARK") {
		t.Errorf("[9 do (mk)] collects the island's results; got:\n%s", dis)
	}

	// Negative: a list over a run the island cannot seat (inside a fn body,
	// or reached through a branch) declines loudly, never assembling the
	// wrong count; the interpreter's answers stand.
	const runtimeCount = "list literal over a result of runtime-variable count"
	requireLoudDecline(t, mk(`1 2`)+`def f fn [[][List][[9 do (mk)]]] end f`, runtimeCount, "[[9 1 2]]")
	requireLoudDecline(t, mk(`1 2`)+`def f fn [[][List][[do (mk)]]] end f`, runtimeCount, "[[1 2]]")
	requireLoudDecline(t, mk(`1 2`)+`def c true end [9 if c [do (mk)] [3]]`, runtimeCount, "[[9 1 2]]")
	// …and the seatings that never needed the island keep their answers.
	requireEngineParity(t, mk(`1 2`)+`do (mk) end 9`, true)
	requireEngineParity(t, `def ops [quote [1 add 2]] 9 do (ops get 0)`, true)
}
