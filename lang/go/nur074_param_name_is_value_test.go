package lang

import (
	"fmt"
	"testing"
)

// TestNUR074ParamNameIsPartOfTheValue pins NUR074's resolution: a function's
// parameter NAME is part of what it does, so canon renders it and `deq` — its
// content compared as canon — tells two spellings apart. A parameter is a
// frame binding on the def stack, and boru resolves a free name at call time
// through that stack (design/FUNCTION-VALUE-SCOPE.0.md §7.4: a name bound
// inside fn F "is visible — via boru's dynamic scoping — to any fn F reaches
// on the call stack"). Renaming one changes what every callee reading that
// name sees: the two `f`s below differ only in their parameter's name and
// answer 5 and 1. Alpha-equivalence is therefore not behavioural equivalence
// in boru, and a rendering or an equality that erased the name would equate
// functions that behave differently.
func TestNUR074ParamNameIsPartOfTheValue(t *testing.T) {
	const g = `def x 1 end def g fn [[] [Any] [x]] end `
	for _, c := range []struct{ src, want string }{
		// The behaviour: the name is what the callee sees.
		{g + `def f fn [[x:Any] [Any] [g]] end f 5`, "[5]"},
		{g + `def f fn [[y:Any] [Any] [g]] end f 5`, "[1]"},
		// …so the value keeps it: renamed params are not deq, on either lane.
		{`([x:Number] => [mul x x]) deq ([y:Number] => [mul y y])`, "[false]"},
		{`(fn [[x:Any] [Any] [x]]) deq (fn [[y:Any] [Any] [y]])`, "[false]"},
		// Negatives: the same spelling is deq, and a rebinding of one
		// function is deq to itself — the name that matters is the
		// parameter's, not the binding's (NUR031).
		{`([x:Number] => [mul x x]) deq ([x:Number] => [mul x x])`, "[true]"},
		{`def a ([x:Number] => [mul x x]) end def b a/v end a/v deq b/v`, "[true]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
}
