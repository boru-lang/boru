package lang

import (
	"fmt"
	"testing"
)

// TestNUR220DynamicApplyParksAnonymousZeroArg pins NUR220's close. The
// interpreter's ANONYMOUS-0-ARG PARK (execFnDefLiteral) holds a lambda VALUE
// with an empty window as data, unless `apply` asked for the application: a
// map-each lambda's `kv.v` over a `([] => [5])` member is the lambda. The
// VM's dynamic apply entries (dynApplyEnter / dynApplyForeign) did not read
// the gate, so the whole-frame replay entered the lambda's stamped unit and
// answered 5. They park now (dynApplyParks), and an `apply` over a lone
// gradual lead — whose identity result the registered-output arm used to
// elide, dropping the Applied mark with it — declines to the strategies
// that model it. A named 0-arg fn keeps firing: its only call form is
// nullary.
func TestNUR220DynamicApplyParksAnonymousZeroArg(t *testing.T) {
	const m = `def m {x: ([] => [5])} end `
	for _, c := range []struct{ src, want string }{
		{m + `each ([kv:Any] => [kv.v]) m`, "[{x:fn}]"},
		{m + `each ([kv:Any] => [kv get "v"]) m`, "[{x:fn}]"},
		// Asked for: `apply` marks the value, and it fires.
		{m + `each ([kv:Any] => [kv.v apply]) m`, "[{x:5}]"},
		{m + `each ([kv:Any] => [(kv.v apply)]) m`, "[{x:5}]"},
		{`def f ([] => [42]) end f/v apply`, "[42]"},
		{`def mk fn [[][Function][([] => [9])]] end (mk) apply`, "[9]"},
		// A NAMED 0-arg member is not parked — the gate reads anonymity,
		// and only the application question, never origin otherwise.
		{`def one fn [[][Integer][1]] end def m {x: one/v} end each ([kv:Any] => [kv.v]) m`, "[{x:1}]"},
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

// TestNUR221LandedLeadIsNoApplyEventLead pins NUR221's close. The gradual
// apply event (OpCallDynApplyOne) applies its lead to the one value beneath
// — right for an INERT lead, wrong for one the interpreter re-steps where it
// stands: `3 kv.v apply` over an inc member applies the member to 3 at its
// own step, and apply then raises over the 4, where the event applied it
// once and answered 4. A lead whose producer carries a re-step landing now
// takes the dynamic-lead decline; a `/v` read and a user paren's placed
// result stay the event's, as does the bare read with no apply after it.
func TestNUR221LandedLeadIsNoApplyEventLead(t *testing.T) {
	const m = `def inc fn [[n:Integer][Integer][n add 1]] end def m {x: inc/v} end `
	for _, c := range []struct{ src, want string }{
		{m + `each ([kv:Any] => [3 kv.v/v apply]) m`, "[{x:4}]"},
		{m + `each ([kv:Any] => [3 (kv.v) apply]) m`, "[{x:4}]"},
		{m + `each ([kv:Any] => [3 (kv get "v") apply]) m`, "[{x:4}]"},
		{m + `each ([kv:Any] => [3 kv.v]) m`, "[{x:4}]"},
		{`def app fn [[nd:Any m:Map] [Any] [nd (m get "inc") apply]] end def rules {inc: ([x:Integer] => [x add 1])} end app 5 rules`, "[6]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
	// Negative: the re-stepped member claims the 3, and apply raises over
	// the result on both lanes — never the 4 the event answered.
	for _, src := range []string{
		m + `each ([kv:Any] => [3 kv.v apply]) m`,
		`def m {x: ([n:Integer] => [n add 1])} end each ([kv:Any] => [3 kv.v apply]) m`,
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, src)
		if codeOf(errI) != "signature_error" || len(gotI) != 0 {
			t.Fatalf("%s: interpreter %v / %v, want apply's no-match", src, gotI, errI)
		}
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 {
			t.Errorf("%s: compiled %v / %v, want the interpreter's %v", src, gotC, errC, errI)
		}
	}
}
