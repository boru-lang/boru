package lang

import (
	"fmt"
	"strings"
	"testing"
)

// list_member_carrier_test.go pins the LIST-MEMBER twin of the fn-carrier
// read corruption. Stage 1 substitutes a def-bound computed fn's CARRIER
// for a read of its name — right where the read is an operand, wrong where
// the name is a body TOKEN — and the thirty-eighth increment let the read
// through inside a nested body, leaving the compiler
// (RecordMakeListInner) as the one place that still refuses the carrier
// when it lands as a list ELEMENT. The guard's own ledger records what it
// costs to lose: the `each` DATA-argument spelling `each [1 2 3] [(f 5)]`
// assembled the data list as [carrier, 5], dropped each's own input list
// and compiled `[3 3]` for the interpreter's `[3]` — silently.
//
// The carrier reaches an element only where the read model cannot claim
// the wrapper's arity, so no dispatch consumes it: `FnUtil.flip sub/v`
// wraps the OVERLOADED `sub`, which has no one shape (the "closure shape
// unknown" neighbour of def_computed_fn_test.go). Every refusal row below
// is answered by the interpreter fallback, which is what makes the
// refusal sound rather than a lost answer; the positive neighbour is the
// SAME list literal over a shaped carrier (`def f (mk 1)`), whose read
// does dispatch and still compiles VM-native.
//
// The `/v` SPELLING joined the guard on 2026-09-19 (S1b-2). It used to
// sit outside on the premise that `fs/v` "resolves through Defs and never
// consults the side table" — but a computed fn has no Defs binding under
// analysis, so that read reported undefined_word and the program refused
// for that instead. stepWordVal now resolves the table, which makes the
// two spellings one convention and puts `[fs/v]` where `[fs 3 10]`
// already was: the compile model holds a CARRIER, not the value, so
// assembling it as an element would bake the wrong thing. The genuine
// data list that still compiles is the one whose member is a CONCRETE fn
// value — `def g fn […] end [g/v]`, a Defs binding the table never sees —
// which the last case pins.

const lmcFlip = `import "boru:fn-util"  def fs (FnUtil.flip sub/v) end `
const lmcMk = `def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f (mk 1) end `

// TestListMemberFnCarrierSoundRefusals pins the refusal: a table carrier
// at a list member refuses the assembly, and the interpreter still
// answers. The last case is the negative — a CONCRETE fn value at a list
// member (a Defs binding, no carrier) assembles and compiles.
func TestListMemberFnCarrierSoundRefusals(t *testing.T) {
	const reason = "computed fn read inside an unevaluated body"
	rows := []struct{ src, interp, note string }{
		{lmcFlip + `[(fs 3 10)]`, "[[-7]]", "the paren apply at the sole element"},
		{lmcFlip + `[fs 3 10]`, "[[-7]]", "the bare read, its args written after it"},
		{lmcFlip + `[1 (fs 3 10) 2]`, "[[1 -7 2]]", "a carrier between two plain elements"},
		{lmcFlip + `each [1 2 3] [(fs 3 10)]`, "[[3]]", "the each DATA argument — the guard's own shape: [1 2 3] is the body"},
		{lmcFlip + `[(fs 3 10)] each [1 2]`, "[[2]]", "the data list written before the each"},
		{lmcFlip + `{n: [(fs 3 10)]}`, "[{n:[-7]}]", "a LIST-valued map entry — the RecordMakeMap caller of the same record"},
		{lmcFlip + `[fs/v]`, "[[fn fs(Number, Number) or (Bytes, Bytes) or (Micron, Micron) or (String, String) or (Boolean, Boolean) or (Atom, Atom)]]", "the `/v` member: the same carrier, reached since S1b-2 through the table rather than reported undefined"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, got, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a refusal (%s)", c.src, c.note)
			continue
		}
		if !strings.Contains(got, reason) {
			t.Errorf("%q: refused %q, want a reason containing %q", c.src, got, reason)
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
	// The negative: a CONCRETE fn value at a list member is a genuine data
	// list — `g` is a Defs binding, so no carrier stands in for it and the
	// guard has no business with it. It compiles, and both lanes place the
	// value. This is what the guard must not take with it.
	const src = `def g fn [[b:Integer][Integer][add 1 b]] end [g/v]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, got, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if prog == nil {
		t.Errorf("`[g/v]`: refused (%q) — a concrete fn value at a list member is a genuine data list", got)
	}
	d, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, errI := d.RunInterp(src)
	if errI != nil || fmt.Sprint(gotI) != "[[fn g(Integer)]]" {
		t.Errorf("`[g/v]`: interpreter answers %v (%v), want [[fn g(Integer)]]", gotI, errI)
	}
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotC, compiled, errC := e.RunCompiled(src)
	if errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("`[g/v]`: compiled %v (compiled=%v, %v), want the interpreter's %v", gotC, compiled, errC, gotI)
	}
}

// TestListMemberFnCarrierParity pins the positive neighbour the guard must
// not take with it: a SHAPED carrier's read at a list member is a
// dispatch, so the element is the dispatch's result and the list literal
// compiles, agrees, and runs VM-native.
func TestListMemberFnCarrierParity(t *testing.T) {
	rows := []struct{ src, want string }{
		{lmcMk + `[(f 5)]`, "[[6]]"},
		{lmcMk + `[1 (f 5) 2]`, "[[1 6 2]]"},
		{lmcMk + `[(f 5) (f 6)]`, "[[6 7]]"},
	}
	for _, c := range rows {
		gotC, compiled, islands, errC := runCompiledNative(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled — want %s", c.src, c.want)
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
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q = %v, want %s", c.src, gotC, c.want)
		}
	}
}
