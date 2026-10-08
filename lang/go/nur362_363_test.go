package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The NUR362 fixtures: a zero-argument fn whose declared result is Any, so
// the check pass holds its result as a gradual operand and the call it
// feeds re-matches at run time.
const (
	nur362Key  = `def h fn [[] [Any] ["k"]] end `
	nur362Int  = `def h fn [[] [Any] [5]] end `
	nur362Err  = `def h fn [[] [Any] [do [raise "boom"] error []]] end `
	nur362Map  = `def h fn [[] [Any] [{k:3}]] end `
	nur362Bar  = `def g fn [[a:Integer | b:Integer] [Integer] [a sub b]] end `
	nur362Bar2 = `def g fn [[a:Integer | b:Integer] [Integer] [a sub b] [a:String | b:Integer] [Integer] [b]] end `
	nur362Mix  = `def g fn [[a:String b:Map] [Any] [b] [a:String | b:Integer] [Any] [b add 1]] end `
)

// TestNUR362BarredPolyAgrees pins NUR362: a poly re-match read its operand
// window flat, as if any overload could take it, where every overload of
// `get` (and has/getr/dot/dotr/is/or/tis/teq/as/default) has barrier 1 — its
// forward collection takes one written operand and reads the rest off the
// stack beneath the word. `get (h) {k:1}` answered 1 compiled; the
// interpreter finds the stack empty and raises get's signature_error. The
// record now carries the dispatch's layout and the run plans the window as
// the interpreter does: a raise over its tape, or the overload the plan
// takes over the very operands the call holds.
func TestNUR362BarredPolyAgrees(t *testing.T) {
	for _, src := range []string{
		nur362Key + `get (h) {k:1}`,
		nur362Key + `get (h) {k:1} end`,
		nur362Key + `[get (h) {k:1}]`,
		nur362Key + `def m {k:1} get (h) m`,
		`def h fn [[] [Any] [0]] end def l [1 2] get (h) l`,
		`def h fn [[] [Any] [0]] end get (h) [1 2]`,
		nur362Key + `def f fn [[] [Any] [get (h) {k:1}]] end f`,
		nur362Key + `def f fn [[m:Map] [Any] [get (h) m]] end f {k:1}`,
		nur362Key + `has (h) {k:1}`,
		nur362Key + `getr (h) {k:1}`,
		nur362Key + `dot (h) {k:1}`,
		nur362Key + `dotr (h) {k:1}`,
		nur362Int + `is (h) Integer`,
		nur362Int + `or (h) 7`,
		nur362Int + `default (h) 7`,
		nur362Int + `as (h) Integer`,
		`def h fn [[] [Any] [Integer]] end tis (h) Integer`,
		`def h fn [[] [Any] [Integer]] end teq (h) Integer`,
		// The matched twin: the pass took get's `[String Error]` overload
		// (barrier 2) over both written operands; a Map at run time is no
		// Error, and the `[String Node]` overload's plan stops at its barrier.
		nur362Map + `get "k" (h)`,
		nur362Map + `def f fn [[] [Any] [get "k" (h)]] end f`,
		// The plan takes the window the call holds: the Error overload.
		nur362Err + `get "message" (h)`,
		nur362Err + `def f fn [[] [Any] [get "message" (h)]] end [f f]`,
		// A constant beneath no barred overload's slot takes: the plan still
		// either takes the call's window or raises.
		nur362Err + `5 get "message" (h)`,
		nur362Map + `5 get "message" (h)`,
		nur362Map + `def f fn [[] [Any] [5 get "k" (h)]] end f`,
		// A word after the operands: the recovery's trap plans the window.
		`def z 1 ` + nur362Key + `get (h) {k:1} z`,
		`def z 1 ` + nur362Key + `def f fn [[] [Any] [get (h) {k:1} z]] end f`,
		`def z 1 ` + nur362Err + `get "message" (h) z`,
		// Unbarred: one written operand, a stack operand beneath.
		nur362Key + `{k:2} get (h) {k:1}`,
		nur362Key + `def m {k:2} m get (h) {k:1}`,
		nur362Key + `def m {k:2} m get (h)`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR362BarredUserFnAgrees pins the user-fn half of NUR362: a `|`
// barrier in a user fn's signature. A recovered call over a window whose
// written operands reach past the barrier (`g (h) 7`) recorded a guarded
// CALL_USER binding both — `-2` for the interpreter's signature_error — and
// now leaves the dispatch to the unmatched-dispatch trap, which plans it;
// the completion of a forward collection stopped by the barrier (`1 g (h)
// 7`) was islanded as a stack-only drift window that laid the written
// operand beneath the word — `[1 2]` for `[4 7]`; and a multi-overload
// user poly whose arms' barriers differ re-matched flat, taking the
// `[a:String | b:Integer]` arm over both written operands.
func TestNUR362BarredUserFnAgrees(t *testing.T) {
	for _, src := range []string{
		nur362Int + nur362Bar + `g (h) 7`,
		nur362Int + nur362Bar + `[g (h) 7]`,
		nur362Int + nur362Bar2 + `g (h) 7`,
		nur362Int + nur362Bar + `1 g (h) 7`,
		nur362Int + nur362Bar2 + `1 g (h) 7`,
		`def h fn [[] [Any] ["s"]] end ` + nur362Bar2 + `1 g (h) 7`,
		nur362Int + nur362Bar2 + `1 g (h)`,
		nur362Int + `1 add (h) 7`,
		nur362Int + `1 sub (h) "x"`,
		// The user poly plans its window over the live binding: a raise
		// where no arm takes it, the arm whose plan takes the very operands.
		nur362Int + nur362Mix + `g "x" (h)`,
		`def h fn [[] [Any] [{q:1}]] end ` + nur362Mix + `g "x" (h)`,
		nur362Int + nur362Mix + `def f fn [[] [Any] [g "x" (h)]] end f`,
		`def h fn [[] [Any] [{q:1}]] end ` + nur362Mix + `def f fn [[] [Any] [g "x" (h)]] end [f f]`,
		nur362Int + nur362Mix + `"y" g "x" (h)`,
		`def h fn [[] [Any] [{q:1}]] end ` + nur362Mix + `"y" g "x" (h)`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR362BarredWindowDeclines pins the loud remainder: where the
// interpreter's plan could take a window the compiled call does not hold —
// a value beneath the operands a barred arm reads instead of the written
// one (`9 g "x" (h)` is `[10 5]`) — or no island can plan it (the recovered
// call in a fn body), the program declines rather than re-match flat.
func TestNUR362BarredWindowDeclines(t *testing.T) {
	for _, tc := range []struct{ src, reason string }{
		{nur362Int + nur362Mix + `9 g "x" (h)`, "NUR362"},
		{nur362Int + nur362Bar + `def f fn [[] [Any] [g (h) 7]] end f`, "unmatched dispatch recovered at g"},
		// A list literal after the operands leaves no exact layout to plan
		// over: the committed dispatch declines, and the recovered one
		// leaves the trap, which declines too.
		{`def z 1 ` + nur362Err + `get "message" (h) [z]`, "NUR362"},
		{`def z 1 ` + nur362Key + `get (h) {k:1} [z]`, "unmatched dispatch recovered at get"},
	} {
		prog, reason, _, err := mustNew(t).CompileCheck(tc.src)
		if prog != nil || err != nil || !strings.Contains(reason, tc.reason) {
			t.Errorf("%q: want the decline %q, got prog=%v reason=%q err=%v", tc.src, tc.reason, prog != nil, reason, err)
		}
		if _, errI := mustNew(t).RunInterp(tc.src); tc.reason == "NUR362" && errI != nil {
			t.Errorf("%q: the interpreter answers: %v", tc.src, errI)
		}
	}
}

// TestNUR363DefThenReadInCallback pins NUR363: a callback body's `def v
// (mk) v` over mk's gradual fn. The def's install re-pushes its source,
// and the read's deopt point — tested at the read's first push — took that
// push for the read's: the island resumed at the read before the def had
// run, and the interpreter met `v` unbound. The install's push is the def's
// own now, as a dynamic-scope bind's is, so the point tests at the read.
func TestNUR363DefThenReadInCallback(t *testing.T) {
	for _, body := range []string{
		`each [def v (mk) v] [1]`,
		`each [add 1 def v (mk) v] [1]`,
		`each [drop def v (mk) v] [1]`,
		`each [def v (mk) v] [1 2]`,
		`each [def v (mk) end v] [1]`,
		`each [def v (mk) [v]] [1]`,
		`each [def v (mk) v] [1] end v`,
		// The read under a forward-drift window: its value rides in bare,
		// since the read undoes the paren's placement; a `/v` read keeps it.
		`each [def v (mk) v add 1] [1]`,
		`each [def v (mk) v/v add 1] [1]`,
		`def f fn [[x:Integer] [Any] [def v (mk) v add 1]] end f 1`,
		`def f fn [[x:Integer] [Any] [def v (mk) v]] end each [f] [1]`,
		`def f fn [[] [Any] [def v (mk) v]] end f`,
	} {
		requireCompiledParity(t, nur361Fn+body)
		requireCompiledParity(t, nur361Data+body)
	}
}

// TestNUR363DriftWindowReadOfALambda pins the window's lead over an
// anonymous fn: the read dispatches it too (`[[43]]`), where the bare call
// result stays parked data and add refuses it (NUR287).
func TestNUR363DriftWindowReadOfALambda(t *testing.T) {
	for _, src := range []string{
		`def mk fn [[] [Any] [([] => [42])]] end each [def v (mk) v add 1] [1]`,
		`def mk fn [[] [Any] [([] => [42])]] end 5 mk add 1`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR363ReadStillDispatches is NUR363's negative twin: the fixed point
// must still test — a callback reading a def of a fn value dispatches it
// (the interpreter's `[[5]]`), never pushes the fn as data.
func TestNUR363ReadStillDispatches(t *testing.T) {
	src := nur361Fn + `each [def v (mk) v] [1]`
	got, compiled, err := mustNew(t).RunCompiled(src)
	if !compiled || err != nil || fmt.Sprint(got) != "[[5]]" {
		t.Fatalf("%q: compiled=%v got %v err %v, want [[5]]", src, compiled, got, err)
	}
}
