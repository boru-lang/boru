package lang

import (
	"fmt"
	"testing"
)

// TestLoopFreshDefZeroTrips pins NUR214's close. A FRESH def inside a loop
// that may run zero times is bound after the loop only if the body ran —
// the interpreter raises undefined_word on the read otherwise. The compiled
// lane carries the name in a frame slot with no init (NoteLoopFresh): the
// zero slot is "unbound", every body def stores into it, and the read after
// the loop is bound-checked, so it raises exactly where the interpreter
// does; it used to answer the body's constant (`5`) or push the unset slot
// (an empty stack), silently.
func TestLoopFreshDefZeroTrips(t *testing.T) {
	// Since phase 2 (design/IMMUTABLE-DEF.1.md §2.1) a loop body is a BLOCK:
	// a fresh def inside it is the body's own whatever the trip count, so
	// the read after the loop is undefined_word on the interpreter and the
	// compiled lane stops at the check — the rule, not NUR214's bound-checked
	// cell, decides.
	for _, c := range []struct{ src, name string }{
		{`def n (0 add 0) end for n [def x 5] x`, "x"},
		{`def n (0 add 0) end for n [def x (n add 5)] x`, "x"},
		{`def n (0 add 0) end for n [def l [i i]] l`, "l"},
		{`def f fn [[n:Integer][Any][for n [def x 5] x]] end f 0`, "x"},
		{`def n (2 add 0) end for n [def x i] x`, "x"},
		{`def f fn [[n:Integer][Any][for n [def x i] x]] end f 3`, "x"},
		{`def n (2 add 0) end for n [def x 5] def g fn [[][Any][x]] end g`, "x"},
	} {
		requireBlockRule(t, c.src, "ERROR:undefined word: "+c.name, "check diagnostics")
	}
	// The intended spelling carries the value in a var cell: the pre-loop
	// value when the loop ran zero times, the last iteration's assignment
	// otherwise — on both lanes.
	for _, c := range []struct{ src, want string }{
		{`def n (0 add 0) end var x 0 for n [var x 5] x`, "[0]"},
		{`def n (0 add 0) end var x 0 for n [var x (n add 5)] x`, "[0]"},
		{`def n (0 add 0) end var l [] for n [var l [i i]] l`, "[[]]"},
		{`var m (0 add 0) end var x 0 while [m gt 0] [var x 5 var m 0] x`, "[0]"},
		{`def f fn [[n:Integer][Any][var x 0 for n [var x 5] x]] end f 0`, "[0]"},
		{`def n (0 add 0) end var x 0 for n [var x i] def m (1 add 0) end for m [var x (x add 10)] x`, "[10]"},
		{`def n (2 add 0) end var x 0 for n [var x i] x`, "[1]"},
		{`def n (3 add 0) end for n [def x i x]`, "[0 1 2]"},
		{`def f fn [[n:Integer][Any][var x 0 for n [var x i] x]] end f 3`, "[2]"},
		{`var m (2 add 0) end var y 0 while [m gt 0] [var y m var m (m sub 1)] y`, "[1]"},
		{`def n (2 add 0) end var x 0 for n [var x i] for n [var x (x add 10)] x`, "[21]"},
		{`def n (2 add 0) end var x 0 for n [var x 5] def g fn [[][Any][x]] end g`, "[5]"},
		{`def n (0 add 0) end for n [def x 1] def x 2 x`, "[2]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestCondBoundDynamicReadRaises pins NUR215's close. A name bound only on
// some paths — an arm's def with no pre binding (NUR110), a fresh def in a
// loop that may run zero times (NUR214) — is read through its bound-checked
// cell by the unit that binds it; a fn reading it DYNAMICALLY reaches the
// VM's dynamic-scope lookup instead, whose miss used to bail as an internal
// error. The Program names those cells (CondBoundNames), so the miss is the
// interpreter's undefined_word, raised at the read.
func TestCondBoundDynamicReadRaises(t *testing.T) {
	// Since phase 2 an arm's or a body's def is its own: a fn reading the
	// name dynamically after the construct finds it unbound on every path,
	// on the interpreter; the compiled lane stops at the check.
	for _, c := range []struct{ src, name string }{
		{`def n (0 add 0) end for n [def x 5] def g fn [[][Any][x]] end g`, "x"},
		{`def c (1 gt 2) end if c [def k 5] [] def g fn [[][Any][k]] end g`, "k"},
		{`def n (2 add 0) end for n [def x 5] def g fn [[][Any][x]] end g`, "x"},
		{`def c (1 lt 2) end if c [def k 5] [] def g fn [[][Any][k]] end g`, "k"},
	} {
		requireBlockRule(t, c.src, "ERROR:undefined word: "+c.name, "check diagnostics")
	}
	// The var spelling: the fn reads the module's cell, assigned or not.
	for _, c := range []struct{ src, want string }{
		{`def c (1 gt 2) end var k 0 if c [var k 5] [] def g fn [[][Any][k]] end g`, "[0]"},
		{`def c (1 lt 2) end var k 0 if c [var k 5] [] def g fn [[][Any][k]] end g`, "[5]"},
		{`def n (0 add 0) end var x 0 for n [var x 5] def g fn [[][Any][x]] end g`, "[0]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestLoopRoundRollbackDropsEventNotes pins the rollback fix NUR214's fresh
// carry exposed. A non-final loop-analysis round is rolled back and its event
// seqs are reissued; the per-event notes keyed by seq (eventInfo, the landing
// and re-step notes) must go with the round, or the stabilised round's event
// at a reissued seq inherits them. Here the discarded round's routed `add`
// left its GENERIC flag on the seq the next round's `if` branch took, so the
// branch dispatch was no longer elided and declined as a code-body word.
// Both lanes must compile the loop and agree; the zero-trip and early-break
// rows are the negative halves.
func TestLoopRoundRollbackDropsEventNotes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`var seen 0 for 4 [def idx i if (idx lt 2) [break] [] var seen (seen add 1)] seen`, "[0]"},
		{`var seen 0 for 4 [def idx i if (idx gt 1) [break] [] var seen (seen add 1)] seen`, "[2]"},
		{`def til fn [[xs:List] [Integer] [var seen 0 for (xs size) [def idx i if ((xs idx get) 0 lt) [break] [] var seen (seen add 1)] end seen]] (til [1 2 -1 4])`, "[2]"},
		{`def til fn [[xs:List] [Integer] [var seen 0 for (xs size) [def idx i if ((xs idx get) 0 lt) [break] [] var seen (seen add 1)] end seen]] (til [])`, "[0]"},
		// The zero-trip loop with a COMPUTED count: the body never runs, so
		// the pre-loop value stands. (The read of the body's own `idx` after
		// the loop is the block rule's leak — block_scope_rule_test.go.)
		{`def n (0 add 0) end var seen 0 for n [def idx i if (idx lt 2) [break] [] var seen (seen add 1)] seen`, "[0]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		answer := func(got []any, err error) string {
			if err != nil {
				return codeOf(err)
			}
			return fmt.Sprint(got)
		}
		if got := answer(gotI, errI); got != c.want {
			t.Errorf("%q: interpreted %s, want %s", c.src, got, c.want)
		}
		if got := answer(gotC, errC); !compiled || got != c.want {
			t.Errorf("%q: compiled %s (compiled=%v), want %s", c.src, got, compiled, c.want)
		}
	}
}
