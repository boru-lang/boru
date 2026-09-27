package lang

import "testing"

// TestNUR294ARunIsNeverOneValue pins NUR294: values whose count is a run's —
// a computed `do` body's, a literal body's whose residual ends in one, a
// branch's whose arm is one — were seated as the ONE value the pass
// models. With a value beneath, the trailing apply rotated the run as if it
// were one value, and a promotion stored one of its values: `9 do [do (mk)]`
// over [1 2] answered [1 9 2]. A run is a region now (runOperand,
// closureResidualRuns, eventFlags.closureRun): the prefix seat or the island
// takes it where it can, and every fixed-count seat declines.
func TestNUR294ARunIsNeverOneValue(t *testing.T) {
	mkL := `def mk fn [[][List][[1 2]]] end `
	mkA := `def mk fn [[][Any][[1 2]]] end `
	for _, c := range []struct{ src, want string }{
		{mkL + `9 if true (mk) ["f"]`, "[9 1 2]"},
		{`def mk fn [[][List][[1 2 3]]] end 9 if true (mk) ["f"]`, "[9 1 2 3]"},
		{mkL + `9 if false ["f"] (mk)`, "[9 1 2]"},
		{mkL + `def b true end 9 if b (mk) ["f"]`, "[9 1 2]"},
		{mkL + `def b true end 9 if b (mk) (mk)`, "[9 1 2]"},
		{`def mk fn [[][List][[]]] end 9 if true (mk) ["f"]`, "[9]"},
		{mkL + `9 do [do (mk)]`, "[9 1 2]"},
		{mkA + `9 do [do (mk)]`, "[9 1 2]"},
		{mkA + `9 do [do [do (mk)]]`, "[9 1 2]"},
		{mkL + `9 do [if true (mk) ["f"]]`, "[9 1 2]"},
		{mkL + `def b true end 9 do [if b [do (mk)] ["f"]]`, "[9 1 2]"},
		{`def mk fn [[][Any][[5]]] end 9 do [do (mk)] add`, "[14]"},
		{`__arm [1 add 2]`, "[3]"},
		{`def body (quote [1 add 2]) end __arm body`, "[3]"},
		{`[do [1] error [drop 0]]`, "[[1]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	// A fixed-count seat over a run declines, where it answered wrongly.
	for _, src := range []string{
		mkA + `[9 do [do (mk)]]`,
		mkA + `def x (do [do (mk)]) end x`,
		mkA + `do [do (mk)] add`,
		mkA + `9 do [do (mk)] error [drop 0]`,
	} {
		_, compiled, _, _, _ := runBothEngines(t, src)
		if compiled {
			t.Errorf("%s: a fixed-count seat over a run must decline, it compiled", src)
		}
	}
}
