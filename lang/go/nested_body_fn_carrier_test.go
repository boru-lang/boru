package lang

import (
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
