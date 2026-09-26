package lang

import (
	"fmt"
	"testing"
)

// TestNUR078BareFnNameCalls pins NUR078's close: ADR-011 as amended (the
// 2026-08-17 clause-2 ruling, re-affirmed 2026-08-26) — a bare name bound to
// a function CALLS at every slot, and passing one as an argument is spelled
// `/v`. The slot type used to decide: before a `Function`-typed slot a bare
// name resolved as a reference (stepWord's TFunction intercept and its three
// sibling sites), so `h zero` answered what `h zero/v` answers.
//
// The positive rows are the reference spellings, which must reach every slot
// the bare spelling used to — a def-bound name, a module member, a map
// member, in forward and stack form — agreeing on both lanes, compiled. The
// negative rows are the bare spellings: the name is a call head, so the word
// before it is left without its argument and raises, on both lanes. The
// compiled lane may DECLINE a bare name whose binding the check pass holds
// as a carrier (a factory's result, a fn-typed param — a no-match it cannot
// prove definite), but it may never answer a value: collecting by value
// what the interpreter calls was the divergence this closes.
func TestNUR078BareFnNameCalls(t *testing.T) {
	const zh = `def zero fn [[][Integer][7]] end def h fn [[f:Function][Integer][42]] end `
	const inc = `def inc fn [[n:Integer][Integer][n add 1]] end `
	const modInc = `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end `
	const modBig = `import module [def big fn [[p:Any][Boolean][p.value gt 1]] export "M" {big: big/v}] end `
	const mkF = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def f (mk 10) end `
	for _, c := range []struct{ src, want string }{
		{zh + `h zero/v`, "[42]"},
		{inc + `each inc/v [1 2 3]`, "[[2 3 4]]"},
		{modInc + `each M.inc/v [1 2 3]`, "[[2 3 4]]"},
		{modInc + `[1 2 3] each M.inc/v`, "[[2 3 4]]"},
		{modBig + `filter M.big/v [1 2 3]`, "[[2 3]]"},
		{inc + `def m {f: inc/v} end [1 2 3] each m.f/v`, "[[2 3 4]]"},
		{`def one fn [[][Integer][1]] end def m {f: one/v} end if true m.f/v [2]`, "[1]"},
		{mkF + `each f/v [1 2 3]`, "[[11 12 13]]"},
		{inc + `def run fn [[f:Function xs:List][List][each f/v xs]] end run inc/v [1 2 3]`, "[[2 3 4]]"},
		// A claim-less member read is an operand whatever the slot: `inc`
		// cannot claim a list, so the read is the value (NUR038's rule,
		// with no slot-typed exemption left).
		{modInc + `each M.inc [1 2 3]`, "[[2 3 4]]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
	for _, c := range []struct{ src, code string }{
		// The bare name is a call head: h collects nothing and raises.
		{zh + `h zero`, "signature_error"},
		{inc + `each inc [1 2 3]`, "signature_error"},
		// A member read that WOULD claim the next token calls.
		{modBig + `filter M.big [1 2 3]`, "signature_error"},
		// The check pass's carriers: a factory-bound name, a fn-typed param.
		{mkF + `each f [1 2 3]`, "signature_error"},
		{inc + `def run fn [[f:Function xs:List][List][each f xs]] end run inc/v [1 2 3]`, "signature_error"},
	} {
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		if codeOf(errI) != c.code || len(gotI) != 0 {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.code)
			continue
		}
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 {
			t.Errorf("%s: compiled %v [%s] %s, interp [%s] %s", c.src, gotC, codeOf(errC), detailOf(errC), codeOf(errI), detailOf(errI))
		}
	}
}
