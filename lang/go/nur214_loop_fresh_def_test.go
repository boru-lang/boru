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
	for _, c := range []struct{ src, want string }{
		// Zero trips: undefined_word on both lanes.
		{`def n (0 add 0) end for n [def x 5] x`, "undefined_word"},
		{`def n (0 add 0) end for n [def x (n add 5)] x`, "undefined_word"},
		{`def n (0 add 0) end for n [def l [i i]] l`, "undefined_word"},
		{`def n (0 add 0) end while [n gt 0] [def x 5 def n 0] x`, "undefined_word"},
		{`def f fn [[n:Integer][Any][for n [def x 5] x]] end f 0`, "undefined_word"},
		{`def n (0 add 0) end for n [def x i] def m (1 add 0) end for m [def x (x add 10)] x`, "undefined_word"},
		// The body ran: the last iteration's binding, as before.
		{`def n (2 add 0) end for n [def x i] x`, "[1]"},
		{`def n (3 add 0) end for n [def x i x]`, "[0 1 2]"},
		{`def f fn [[n:Integer][Any][for n [def x i] x]] end f 3`, "[2]"},
		{`def n (2 add 0) end while [n gt 0] [def y n def n (n sub 1)] y`, "[1]"},
		{`def n (2 add 0) end for n [def x i] for n [def x (x add 10)] x`, "[21]"},
		{`def n (2 add 0) end for n [def x 5] def g fn [[][Any][x]] end g`, "[5]"},
		{`def n (0 add 0) end for n [def x 1] def x 2 x`, "[2]"},
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

// TestCondBoundDynamicReadRaises pins NUR215's close. A name bound only on
// some paths — an arm's def with no pre binding (NUR110), a fresh def in a
// loop that may run zero times (NUR214) — is read through its bound-checked
// cell by the unit that binds it; a fn reading it DYNAMICALLY reaches the
// VM's dynamic-scope lookup instead, whose miss used to bail as an internal
// error. The Program names those cells (CondBoundNames), so the miss is the
// interpreter's undefined_word, raised at the read.
func TestCondBoundDynamicReadRaises(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def n (0 add 0) end for n [def x 5] def g fn [[][Any][x]] end g`, "undefined_word"},
		{`def c (1 gt 2) end if c [def k 5] [] def g fn [[][Any][k]] end g`, "undefined_word"},
		{`def n (2 add 0) end for n [def x 5] def g fn [[][Any][x]] end g`, "[5]"},
		{`def c (1 lt 2) end if c [def k 5] [] def g fn [[][Any][k]] end g`, "[5]"},
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
