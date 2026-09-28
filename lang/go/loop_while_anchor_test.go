package lang

import "testing"

// TestWhileEmptyConditionAnchorsAtTheOperand pins NUR130: a `while` whose
// condition produces no value raises at the CONDITION operand on both lanes
// — the compiled terminal trap's anchor, and now the interpreter's too
// (ForCont.CondPos), where the spliced tape pointer used to underline the
// token after the loop, or nothing.
func TestWhileEmptyConditionAnchorsAtTheOperand(t *testing.T) {
	for _, src := range []string{
		"while [] [1] end 5",
		"while [] [1]",
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

// TestLoopNamedFnValueResidualDeclinesSoundly pins NUR129's narrowing: a
// value-producing loop whose body leaves a NAMED fn value is the
// interpreter's re-step at the next iteration (uncalled_function at the
// value); the check pass notes the same raise and the compile declines, so
// the program answers by fallback exactly as the interpreter does — while
// a factory's anonymous closure per iteration is parked data on both lanes
// and keeps its compile.
func TestLoopNamedFnValueResidualDeclinesSoundly(t *testing.T) {
	src := "def g fn [[x:Integer] [Integer] [x add 1]]  for 2 [g/v]"
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	requireParity(t, src, gotC, errC, gotI, errI)
	if compiled {
		t.Errorf("%q: the named value's re-step is the check pass's decline; it compiled", src)
	}
	requireEngineParity(t, "def mk fn [[n:Integer][Function][(fn [[x:Integer][Integer][x add n]])]] for 2 [(mk 1)]", true)
}
