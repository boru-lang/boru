package lang

import (
	"fmt"
	"strings"
	"testing"
)

// nested_body_fn_carrier_test.go pins the thirty-eighth increment: a read
// of a name def-bound to a Function CARRIER (a typed factory's declared
// return, `def f (mk 1)`) resolves through the fn-carrier side table inside
// a NESTED body too — a branch arm, a loop body, a `do` body — where the
// substitution used to decline at NestedBodyDepth > 0. An inline body
// models the read as the dispatch it is; a `do` body reaches the dyn-body
// backstop, whose sub-engine resolves the name through the DynEnv twin. The decline left `if c [(f 2)] [0]` reporting a
// FALSE undefined_word on the plain check and `do [(f 2)]` behind the
// check-diagnostics sentinel. A name bound to a CONCRETE produced closure
// (a lambda factory's) keeps the code-body gate: its read inside the
// body's unit resolves to the FnDefInfo whose home is outside the unit.

const nbfMk = `def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f (mk 1) end `

// TestNestedBodyFnCarrierParity pins the shapes that COMPILE, agree on
// both lanes and re-enter the interpreter through no VM island.
func TestNestedBodyFnCarrierParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{nbfMk + `do [(f 2)]`, "3 — the ledger row"},
		{nbfMk + `do [(f 2)] add 10`, "13 — the do result consumed downstream"},
		{nbfMk + `do [(f 2) (f 3)] add`, "7 — two reads in one do body"},
		{nbfMk + `do [(f 2) (f 3)]`, "3 4 — a multi-value do body"},
		{nbfMk + `if true [(f 2)] [0]`, "3 — a branch arm"},
		{nbfMk + `if false [(f 2)] [(f 5)]`, "6 — the else arm"},
		{nbfMk + `if true [def q (f 2) q] [0]`, "3 — an arm-local def of the result"},
		{nbfMk + `for 2 [(f 2)]`, "3 3 — a loop body"},
		{nbfMk + `for 2 [def q (f 2) q]`, "3 3 — a body-local def of the result"},
		{nbfMk + `def n 0 end while [n lt 2] [def n (n add 1) (f 3)] end 'z'`, "4 4 z — a while body"},
		{nbfMk + `def g fn [[y:Integer][Any][do [(f y) args drop]]] end g 7`, "8 — an args-bearing do body inside a fn (the dyn-body backstop)"},
		{nbfMk + `do [[1 2] each [(f 1)]]`, "[2 2] — a data-list read inside a do body"},
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

// TestNestedBodyFnCarrierPlainCheckClean pins the checker half: the plain
// check reports NO diagnostic for a nested-body read of a carrier-bound
// name — it used to report undefined_word (an ERROR) for a branch arm and
// a loop body on a correct program.
func TestNestedBodyFnCarrierPlainCheckClean(t *testing.T) {
	for _, src := range []string{
		nbfMk + `if true [(f 2)] [0]`,
		nbfMk + `for 2 [(f 2)]`,
		nbfMk + `do [(f 2)]`,
		nbfMk + `def n 0 end while [n lt 2] [def n (n add 1) (f 3)] end 'z'`,
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		res, cerr := a.Check(src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", src, cerr)
		}
		for _, d := range res.Diagnostics {
			if d.Code == "undefined_word" || d.Code == "unused_def" {
				t.Errorf("%q: plain check must not flag the read, got %v", src, d)
			}
		}
	}
	// The negative: an UNBOUND name inside the same arm is still flagged.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	res, cerr := a.Check(nbfMk + `if true [(zz 2)] [0]`)
	if cerr != nil {
		t.Fatal(cerr)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "undefined_word" && d.Word == "zz" {
			found = true
		}
	}
	if !found {
		t.Errorf("an unbound name in a branch arm must stay undefined_word, got %v", res.Diagnostics)
	}
}

// TestNestedBodyFnCarrierSoundRefusals pins the neighbours that REFUSE: a
// CONCRETE produced closure (a lambda factory's) read inside a code body
// keeps the gate — without it `do [(h 1)]` compiled and raised the
// captured param as undefined where the interpreter answers 8.
func TestNestedBodyFnCarrierSoundRefusals(t *testing.T) {
	const kk = `def kk k:Integer => [z:Integer => [add k z]] end def p (kk 7) end `
	rows := []struct{ src, reason, interp string }{
		{`def mkg g:Function => [v:Integer => [(g v)]] end def h (mkg (z:Integer => [add 7 z])) end do [(h 1)]`, "code body reads a def-bound compiled closure", "[8]"},
		{kk + `do [(p 1)]`, "code body reads a def-bound compiled closure", "[8]"},
		{kk + `if true [(p 1)] [0]`, "code body reads a def-bound compiled closure", "[8]"},
		// A carrier-bound read the OTHER gates still refuse, soundly.
		{nbfMk + `[1 2] each [(f 1)]`, "code-body word each (Stage 2)", "[[2 2]]"},
		{nbfMk + `if true [(f 2) (f 3)] [0]`, "then-branch result of unknown provenance", "[3 4]"},
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
		got, ierr := a.RunInterp(c.src)
		if ierr != nil || fmt.Sprint(got) != c.interp {
			t.Errorf("%q: interpreter %v/%v, want %s", c.src, got, ierr, c.interp)
		}
	}
}
