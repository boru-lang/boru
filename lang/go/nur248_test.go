package lang

import "testing"

// TestNUR248TypeLiteralAtASlot pins NUR248's close: one matcher decides
// whether a type literal fills a signature slot, on both lanes. A bare type
// node (`Integer`) is refused at a concrete-payload slot — as every named
// call, def-bound lambda and word dispatch already refused it — and admitted
// at a `t:Type` or `Any` slot. The interpreter's stack-match fallback asked
// SigTypeMatches alone, so an anonymous lambda re-stepped at a paren's close
// took `Integer` for an Integer param; the VM's MatchFnSig asked the node's
// Parent — its SUPERTYPE — so it refused `Integer` at a Type slot. Both ask
// the matcher's own rule now (stackSlotAdmits).
func TestNUR248TypeLiteralAtASlot(t *testing.T) {
	for _, r := range []struct{ src, want string }{
		// Refused at a concrete slot: the lambda parks, on both lanes.
		{`(Integer ([0] => [1]))`, "[Integer fn (Integer)]"},
		{`(Integer ([a:Integer] => [7]))`, "[Integer fn (Integer)]"},
		{`[(Integer ([0] => [1]))]`, "[[Integer fn (Integer)]]"},
		{`def f fn [[a:Integer] [Any] [7]] end (Integer f)`, "ERROR:cannot call `f`"},
		// Admitted at a Type or an Any slot.
		{`(Integer ([t:Type] => [t]))`, "[Integer]"},
		{`(Integer ([a:Any] => [a]))`, "[Integer]"},
		{`def h fn [[f:Function] [Any] [(Integer f/v)]] end h ([t:Type] => [t])`, "[Integer]"},
		// A value keeps its match.
		{`(0 ([0] => [1]))`, "[1]"},
		{`(3 ([a:Integer] => [7]))`, "[7]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
}
