package lang

import (
	"fmt"
	"strings"
	"testing"
)

// literal_read_test.go pins the thirty-third increment: a `/v` read of a
// def-bound CAPTURING fn LITERAL (`def kk (fn r:Integer Any [mul n r])`
// inside a fn body, then `(g kk/v)`) resolves to the literal's closure
// operand — its unit pushed with the captures, built at the first read and
// cached on the bind — where the read used to refuse "fn call operand of
// unknown provenance": the literal has no producing event, and the read is
// a fresh wrap of the binding (ResolveRef). The push carries the def name,
// so the value renders as the interpreter's binding does (`fn kk(Integer)`).
// The alias holds only while every captured name is unbound since the def
// (the literal snapshotted its captures; a rebind of one would make the
// read-site construction see the new value). This is the CPS rows' first
// gate; their refusal moved to the then-arm apply.

// TestLiteralReadParity pins the shapes that now COMPILE, agree on both
// lanes and run VM-native.
func TestLiteralReadParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{`def g f:Function => [(f 3)] end def w n:Integer => [def kk (fn r:Integer Any [mul n r]) (g kk/v)] end (w 5)`, "15 — a fn-body-local capturing literal handed to a Function param"},
		{`def g f:Function => [(f 3)] end def w n:Integer => [def kk (r:Integer => [mul n r]) (g kk/v)] end (w 5)`, "15 — the arrow spelling of the literal"},
		{`def g f:Function => [(f 3)] end def w n:Integer => [def kk (fn r:Integer Any [mul n r]) def m 2 (g kk/v)] end (w 5)`, "15 — an unrelated bind between the def and the read"},
		{`def g f:Function => [f/v] end def w n:Integer => [def kk (fn r:Integer Any [mul n r]) (g kk/v)] end ((w 5) 4)`, "20 — the closure handed through and applied at the main program"},
		{`def w n:Integer => [def kk (fn r:Integer Any [mul n r]) kk/v] end (w 5)`, "fn kk(Integer) — the read returned as the body's value renders under the def name"},
		{`def w n:Integer => [def kk (fn r:Integer Any [mul n r]) kk/v] end (w 5) 4`, "fn kk(Integer) 4 — parked with a token after it"},
		{`def w n:Integer => [def kk (fn r:Integer Any [mul n r]) 3 kk/v apply] end (w 5)`, "15 — the read applied by the apply word inside the body"},
		{`def g f:Function => [(f 3)] end def n 5 end def kk (fn r:Integer Any [mul n r]) end (g kk/v)`, "15 — the main program's read of a top-level capturing literal"},
	}
	for _, c := range rows {
		gotC, compiled, islands, errC := runCompiledNative(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		if len(islands) > 0 {
			t.Errorf("%q: re-enters the interpreter (%s)", c.src, islands[0])
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestLiteralReadSoundRefusals pins the neighbours that still REFUSE, with
// the interpreter's own answer.
func TestLiteralReadSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// a captured fn-local rebound between the def and the read: the
		// literal snapshotted n = 5, a read-site construction would see 7
		{`def g f:Function => [(f 3)] end def w n:Integer => [def kk (fn r:Integer Any [mul n r]) def n 7 (g kk/v)] end (w 5)`, "unknown provenance", "[15]"},
		// the literal redefined by another capturing literal: installDef's
		// fn-body refusal (the thirty-first increment)
		{`def g f:Function => [(f 3)] end def w n:Integer => [def kk (fn r:Integer Any [mul n r]) def kk (fn r:Integer Any [add n r]) (g kk/v)] end (w 5)`, "redefined inside a fn body", "[8]"},
		// the read inside a branch arm: the arm's gradual result may be a fn
		// the interpreter re-steps (residualLeadReStepped), a separate hold
		{`def g f:Function => [(f 3)] end def w n:Integer => [def kk (fn r:Integer Any [mul n r]) if (n gt 0) [(g kk/v)] [0]] end (w 5) (w 0)`, "then-branch result of unknown provenance", "[15 0]"},
		// the CPS row: the read resolves now, the then-arm's pending apply
		// over the k:Function param is not the body tail
		{`def factk fn [[n:Integer k:Function][Any][ if (lte 1 n) [ 1 k/v apply ] [ def kk ( fn r:Integer Any [ def m (mul n r) m k/v apply ] ) (factk (sub 1 n) kk/v) ] ]] end (factk 5 (v:Integer => [v]))`, "not at the body tail", "[120]"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a sound refusal", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refused %q, want %q", c.src, reason, c.reason)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter answers %v (%v), want %s", c.src, gotI, errI, c.interp)
		}
	}
}
