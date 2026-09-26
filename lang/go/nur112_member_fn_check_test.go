package lang

import (
	"fmt"
	"testing"
)

// TestPlainCheckAppliesStoredFnMember pins NUR112's close. A fn value stored
// in a map and read as a member dispatches over what follows on both lanes;
// the PLAIN check read the member as a dynamic Any (the compile pass's model,
// whose arrival the shaped method model claims) and so claimed the member
// AND its argument as the residual — two values where one runs. On a plain
// check the member now reads as the stored fn value itself, and the pass
// applies it as the run does.
func TestPlainCheckAppliesStoredFnMember(t *testing.T) {
	for _, c := range []struct{ src, stack, value string }{
		{`def m {a:size/v}  m.a [1 2 3]`, "[Integer]", "[3]"},
		// The record's own program: the member parked the native before a
		// boru overload extended the name.
		{`def Pos (refine Integer) def m {a:size/v} def size fn [[n:Pos] [Integer] [200]] end def v:Pos 3 m.a v`, "[Integer]", "[3]"},
		{`def m {f: ([n:Integer] => [n add 1])} m.f 5`, "[Integer]", "[6]"},
		{`def m {f: ([a:Integer b:Integer] => [a sub b])} 10 3 m.f`, "[Integer]", "[-7]"}, // the stack binds top first: a=3, b=10
	} {
		res, err := mustNew(t).Check(c.src)
		if err != nil || fmt.Sprint(res.Stack) != c.stack {
			t.Errorf("%q: checked %v (err %v), want %s", c.src, res.Stack, err, c.stack)
		}
		requireEngineParity(t, c.src, true)
		if got, err := mustNew(t).RunInterp(c.src); err != nil || fmt.Sprint(got) != c.value {
			t.Errorf("%q: interpreted %v / %v, want %s", c.src, got, err, c.value)
		}
	}
}
