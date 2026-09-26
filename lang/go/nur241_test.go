package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR241WordLedArrivalDeclines pins NUR241's close as a sound decline:
// the first `append` in `acc "x" append acc (m.path) append` has two forward
// tokens, the word `acc` and a paren. The interpreter's planner evaluates the
// paren before it commits a window, and when the value misses the slot
// ('p' at a FlexList slot) it prunes to the all-stack window and appends "x".
// The check pass planned the paren unevaluated: the word-led window is the
// deferred plan (preferWordSig), and the paren's gradual value arrived in the
// FlexList slot optimistically, so the compiled run committed acc into
// m.path's slot and raised `cannot call append` where the interpreter
// answers. Such an arrival — a compiling pass's deferred word-led window, a
// value its slot cannot prove, a narrower window fitting the stack beneath
// the word — flags the gradual split, NUR228's discipline, and the program
// declines loudly. A window that is not word-led (`1 2 add (m.v)`), one with
// nothing beneath the word for a narrower window (`add x (m.v)`), and one the
// value is proven for compile as before.
func TestNUR241WordLedArrivalDeclines(t *testing.T) {
	for _, r := range []struct{ src, interp string }{
		{`def acc (flex []) end def mk fn [[tag:String][Function][([m:Any] => [acc (tag) append acc (m.path) append])]] end def h (mk "x") end walk {mode: "depth"} {a:1 b:2} h/v ; acc`, "[{a:1 b:2} ['x' '' 'x' 'a' 'x' 'b']]"},
		{`def acc (flex []) end def h fn [[m:Any] [Any Any] [acc "x" append acc (m.path) append]] end h {path: "p"}`, "[['x' 'p'] ['x' 'p']]"},
		{`def acc (flex []) end def h fn [[m:Map] [Any Any] [acc "x" append acc (m.path) append]] end h {path: "p"}`, "[['x' 'p'] ['x' 'p']]"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || compiled || !strings.Contains(fmt.Sprint(errC), "forward/stack split depends on a gradual operand") {
			t.Errorf("%s: want the gradual-split decline, got %v compiled=%v err=%v", r.src, gotC, compiled, errC)
		}
		if gotI, errI := mustNew(t).RunInterp(r.src); errI != nil || fmt.Sprint(gotI) != r.interp {
			t.Errorf("%s: interp got %v / %v, want %s", r.src, gotI, errI, r.interp)
		}
	}
	for _, r := range []struct{ src, want string }{
		{`def x 5 end def h fn [[m:Map] [Any] [add x (m.v)]] end h {v: 3}`, "[8]"},
		{`def acc (flex []) end def h fn [[m:Any] [Any] [append (m.path) acc]] end h {path: "p"}`, "[['p']]"},
		{`def acc (flex []) end def h fn [[m:Any] [Any] [acc (m.path) append]] end h {path: "p"}`, "[['p']]"},
		{`def h fn [[m:Map] [Any] [1 2 add (m.v)]] end h {v: 3}`, "ERROR:expected 1 return value(s), got 2"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
