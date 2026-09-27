package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR216ClaimedFnReadIsTheWord pins NUR216's close. A name def-bound to
// a Function-declared factory's result is a FUNCTION WORD on the run
// (installDef makes the value the name's fn). Read inside a code body or a
// fn body, the compiled read reaches the binding's installed copy, which
// carries no compiled stamp, and the shaped method's island stepped the
// VALUE — an anonymous lambda the interpreter parks as data — so `do [j]`
// answered `fn j` for 42; the island dispatches the NAME now
// (DynMethodSpec.DefRead). Read where a pending word collects, the word is
// a forward-collection barrier as a registered fn's is, so `typeof j` and
// `j eq j`, which answered compiled, raise the interpreter's strand error
// in the pass and decline.
func TestNUR216ClaimedFnReadIsTheWord(t *testing.T) {
	const mk = `def mk fn [[][Function][([] => [42])]] end def j (mk) end `
	for _, c := range []struct{ src, want string }{
		{mk + `do [j]`, "[42]"},
		{mk + `[1 2] each [drop j]`, "[[42 42]]"},
		{mk + `def g fn [[][Any][j]] end g`, "[42]"},
		{mk + `j`, "[42]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	for _, src := range []string{mk + `typeof j`, mk + `j eq j`} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled || !strings.Contains(fmt.Sprint(errI), "is still waiting") || !strings.Contains(fmt.Sprint(errC), "is still waiting") {
			t.Errorf("%s: the barrier raises in the pass and the run; got compiled=%v %v %v / %v %v", src, compiled, gotC, errC, gotI, errI)
		}
	}
}

// TestNUR218FnArgumentInABareReadSlot pins NUR218's close. A container
// member's fn arrives at a user fn's `Any` parameter as a gradual carrier,
// so the callee's unit pushes the parameter where the interpreter
// dispatches a fn there as a word. The unit now records the parameter slots
// it reads bare (CompiledFn.FnReadParams on a named unit with a declared
// contract), and CALL_USER hands a call with a fn in one of them to the
// interpreter: both lanes raise its no-match, notes and caret included.
// Data arguments still run the unit.
func TestNUR218FnArgumentInABareReadSlot(t *testing.T) {
	const m = `def m {f: ([n:Integer] => [n add 1])} end `
	for _, src := range []string{
		m + `def g fn [[h:Any][Any][h]] end g m.f`,
		m + `def g fn [[h:Any][Any][h typeof]] end g m.f`,
		m + `def id fn [[x:Any][Any][x]] end def j (id m.f) end j 5`,
		// In a caller's tail position the callee keeps its own frame, so
		// the call can run on the interpreter and return.
		m + `def g fn [[h:Any][Any][h]] end def w fn [[][Any][g m.f]] end w`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || codeOf(errI) != "signature_error" || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) {
			t.Errorf("%s: the interpreter's no-match on both lanes; got compiled=%v %v %v / %v %v", src, compiled, gotC, errC, gotI, errI)
		}
	}
	agreeOnBothLanes(t, m+`def g fn [[h:Any][Any][h]] end g 5`, "[5]")
	agreeOnBothLanes(t, m+`def g fn [[h:Any][Any][h]] end [(g 5) (g 'a')]`, "[[5 'a']]")
}
