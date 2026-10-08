package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR299OneOperandSplitDeclines pins NUR299's close. A one-operand word
// over a gradual value beneath it and a literal after it splits at run time
// by the value. `(mk) do [5]` over a factory of `[1 2]` is `[[1 2] 5]`
// interpreted: the List misses do's Map overload, and the List overload
// takes the `[5]` written after the word. The check pass filled the Map
// overload from the stack, so the poly `do` ran the stack value as the body.
// That answered `[1 2 [5]]` on this branch (13aa881's region commits no count
// claim) and bailed on main. Over a fn value (`m.f do [(g)]`) it raised
// `cannot call do` compiled. The split is ambiguous, so the compile declines
// (NUR305's discipline, widened to the one-operand all-stack match); a
// concrete value beneath compiles as before.
func TestNUR299OneOperandSplitDeclines(t *testing.T) {
	const g = `def g fn [[][Any][5]] end `
	for _, c := range []struct{ src, want string }{
		{`def mk fn [[][Any][[1 2]]] end (mk) do [5]`, "[[1 2] 5]"},
		{`def mk fn [[][Any][[1 2]]] end ` + g + `(mk) do [(g)]`, "[[1 2] 5]"},
		{`def m {f: ([x:Integer] => [x add 1])} end ` + g + `m.f do [(g)]`, "[fn (Integer) 5]"},
		{`def m {f: ([x:Integer] => [x add 1])} end ` + g + `[m.f do [g]]`, "[[fn (Integer) 5]]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreted %v %v, want %s", c.src, gotI, errI, c.want)
		}
		if compiled || errC == nil || !strings.Contains(errC.Error(), "forward/stack split depends on a gradual operand") {
			t.Errorf("%s: declines compiled; got compiled=%v %v %v", c.src, compiled, gotC, errC)
		}
	}
	agreeOnBothLanes(t, `[1 2] do [5]`, "[[1 2] 5]")
	agreeOnBothLanes(t, g+`7 do [(g)]`, "[7 5]")
}
