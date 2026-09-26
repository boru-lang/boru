package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR234ContractNoMatchReportsTheAttemptedWindow pins NUR234's close: a
// compiled user call whose gradual argument fails its param contract raises
// over the window the interpreter's failed dispatch reports — the written
// run, which a bare word read ends, filled from the stack beneath to the
// smallest arity — not over the call's arguments. Every row compiles and the
// two lanes' errors agree to the byte, notes included; note is the one that
// tells the rows apart.
func TestNUR234ContractNoMatchReportsTheAttemptedWindow(t *testing.T) {
	const fg = `def f fn [[n:String] [Integer] [0]] end def g fn [[a:String b:String] [Integer] [0]] end `
	rows := []struct{ src, note string }{
		{fg + `each ([e:Any] => [f e]) [5]`, "takes 1 argument, but none were supplied"},
		{fg + `each ([e:Any] => [7 f e]) [5]`, "the argument was 7"},
		{fg + `each ([e:Any] => [f e 7]) [5]`, "takes 1 argument, but none were supplied"},
		{fg + `each ([e:Any] => [9 e f]) [5]`, "the arguments were 5 (an Integer) and 9"},
		{fg + `each ([e:Any] => [g "x" e]) [5]`, "the argument was 'x'"},
		{fg + `each ([e:Any] => [g e "x"]) [5]`, "takes 2 arguments, but none were supplied"},
		{fg + `each ([e:Any] => [e g "x"]) [5]`, "the arguments were 'x' (a ProperString) and 5"},
		{fg + `each ([e:Any] => ["x" e g]) [5]`, "the arguments were 5 (an Integer) and 'x'"},
		{fg + `each ([e:Any] => [f (e)]) [5]`, "the argument was 5"},
		// a result beneath the call: on the stack, and promoted to a slot
		{fg + `each ([e:Any] => [(e add 1) f e]) [5]`, "the argument was 6"},
		{fg + `each ([e:Any] => [def r (e add 1) r f e r]) [5]`, "the argument was 6"},
		// a param read beneath the call
		{fg + `def h fn [[a:Any b:Any] [Integer] [a f b]] end h 5 6`, "the argument was 5"},
		{fg + `def h fn [[a:Any b:Any] [Integer Integer] [a g "x" b]] end h 5 6`, "the arguments were 'x' (a ProperString) and 5"},
		// four values fill the window
		{fg + `def h fn [[x:Any] [Integer] [1 2 3 4 x f x]] end h 5`, "the arguments were 5 (an Integer), 4 (an Integer), 3 (an Integer) and 2"},
		// the main code's table
		{fg + `def k fn [[] [Any] [5]] end def v (k) 7 f v`, "the argument was 7"},
	}
	for _, r := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, r.src)
		if !compiled {
			t.Errorf("%s: must compile, got %v", r.src, errC)
			continue
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%s:\n compiled %v / %v\n interp   %v / %v", r.src, gotC, errC, gotI, errI)
			continue
		}
		if errI == nil || !strings.Contains(errI.Error(), "signature_error") || !strings.Contains(errI.Error(), r.note) {
			t.Errorf("%s: want a signature_error noting %q, got %v", r.src, r.note, errI)
		}
	}
	// The window is the failure's alone: a gradual argument that fits runs.
	agreeOnBothLanes(t, fg+`each ([e:Any] => [f e]) ["s"]`, "[[0]]")
	agreeOnBothLanes(t, fg+`def h fn [[a:Any b:Any] [Integer Integer] [a f b]] end h 5 "s"`, "[5 0]")
}
