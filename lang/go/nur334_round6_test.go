package lang

import "testing"

// nur334_round6_test.go pins the 2026-09-30 round-6 closes of NUR334's loud
// remainder. A call whose unit returns a tape-coupled result (a `/v` read of
// a `word` splice) takes its call-result island: the interpreter runs the
// call's statement again, the call's run written as the results it left.
// The island runs AFTER the call, so what the compiled code read in the
// statement before it is written as the value read there, never read again
// over the call's effects.

// TestNUR334CallResultReadsWritten: a def-bound word before the call on its
// level (`k f …`, folded to the constant the island writes), a member read
// before it (`c.n f …`, alone or in a paren, chained), and a member read
// written after a lazy list holding the call that the interpreter makes
// before the list's elements (`[(cg) f …] c.n`) — each written as the value
// the compiled code read. A body that undefs or rebinds `k`, or sets `c.n`,
// is the witness the read is not made again.
func TestNUR334CallResultReadsWritten(t *testing.T) {
	const f = `def f fn [[b:List][Any][def t 0 do b drop t/v]] end `
	const call = `f (quote [def t word [1 2] 1])`
	const k = f + `def k 3 end `
	const c = f + `def c (flex {n:0}) end `
	const counter = c + `def cg fn [[][Integer][c set 'n' (c.n add 1) drop c.n]] end `
	for _, r := range []struct{ src, want string }{
		// The register's witnesses.
		{k + `k f (quote [def t word [add] 1])`, "ERROR:cannot call `add`"},
		{counter + `[(cg) ` + call + `] c.n`, "[[1 1 2] 0]"},
		// A def-bound word before the call.
		{k + `k ` + call, "[3 1 2]"},
		{k + `k k f (quote [def t word [add] 1])`, "[6]"},
		{k + `k 4 f (quote [def t word [add] 1])`, "[7]"},
		{k + `1 k f (quote [def t word [add] 1])`, "[4]"},
		{k + `[k f (quote [def t word [add] 1])]`, "ERROR:cannot call `add`"},
		{k + `(k f (quote [def t word [add] 1]))`, "ERROR:cannot call `add`"},
		{k + `def r (k f (quote [def t word [add] 1])) end r`, "ERROR:cannot call `add`"},
		{k + `k f (quote [undef k def t word [1 2] 1])`, "[3 1 2]"},
		{k + `k f (quote [def k 5 def t word [1 2] 1])`, "[3 1 2]"},
		{f + `def k true end k ` + call, "[true 1 2]"},
		{f + `def k "s" end k f (quote [def t word [add 1] 1])`, "[s1]"},
		{f + `def k 3.5 end k f (quote [def t word [add] 1])`, "ERROR:cannot call `add`"},
		// Two defs sharing one value identity under different parents (a typed
		// re-def): the island writes the binding the NAME read, so the value
		// keeps its brand on both lanes (the Codex review of #529 found the
		// compiled lane answering Integer at random here).
		{f + `def Pos refine Integer end def a 3 end def b:Pos a end b f (quote [def t word [1] 1]) end drop typeof`, "[Pos]"},
		{f + `def Pos refine Integer end def a 3 end def b:Pos a end a f (quote [def t word [1] 1]) end drop typeof`, "[Integer]"},
		// A member read before the call, over a body that sets it.
		{c + `c.n f (quote [c set 'n' 5 drop def t word [1 2] 1])`, "[0 1 2]"},
		{c + `[c.n f (quote [c set 'n' 5 drop def t word [1 2] 1])]`, "[[0 1 2]]"},
		{c + `(c.n) f (quote [c set 'n' 5 drop def t word [1 2] 1])`, "[0 1 2]"},
		{c + `((c.n)) f (quote [c set 'n' 5 drop def t word [1 2] 1])`, "[0 1 2]"},
		{c + `c.n c.n f (quote [c set 'n' 5 drop def t word [add] 1])`, "[0]"},
		{f + `def c (flex {a:{b:0}}) end c.a.b f (quote [c.a set 'b' 5 drop def t word [1 2] 1])`, "[0 1 2]"},
		{f + `def c (flex [0]) end c.0 f (quote [c set 0 5 drop def t word [1 2] 1])`, "[0 1 2]"},
		// A read after a lazy list holding the call.
		{counter + `[(cg) ` + call + `] c.n c.n`, "[[1 1 2] 0 0]"},
		{counter + `[(cg) ` + call + `] (c.n)`, "[[1 1 2] 0]"},
		{counter + `[(cg) ` + call + `] c.n end c.n`, "[[1 1 2] 0 0]"},
		// The negative half: a binding the statement takes stays compiled.
		{counter + `[(cg) f (quote [def t 5 1])] c.n`, "[[1 5] 0]"},
		{k + `k f (quote [def t 5 1])`, "[3 5]"},
		{c + `c.n f (quote [c set 'n' 5 drop def t 7 1])`, "[0 7]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	// Still loud: a read of a compound binding (whose identity the island
	// could not write), and a call written after the lazy list that the
	// compiled code ran before it.
	for _, r := range []struct{ src, want string }{
		{f + `def k [1] end k f (quote [def t word [size] 1])`, "[1]"},
		{counter + `[(cg) ` + call + `] c.n add 1`, "[[1 1 2] 1]"},
	} {
		requireLoudDefer(t, r.src, "tape-coupled deopt result", r.want)
	}
}
