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

const lmcFlip = `import "boru:fn-util"  def fs (FnUtil.flip sub/v) end `
const lmcMk = `def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f (mk 1) end `

// TestListMemberFnCarrierSoundRefusals pins the refusal: a table carrier
// at a list member refuses the assembly, and the interpreter still
// answers. The last row is the negative — a GENUINE data list of the
// computed fn spells the member `fs/v`, which resolves through Defs and
// never consults the side table, so it is not this guard's business.
func TestListMemberFnCarrierSoundRefusals(t *testing.T) {
	const reason = "computed fn read inside an unevaluated body"
	rows := []struct{ src, interp, note string }{
		{lmcFlip + `[(fs 3 10)]`, "[[-7]]", "the paren apply at the sole element"},
		{lmcFlip + `[fs 3 10]`, "[[-7]]", "the bare read, its args written after it"},
		{lmcFlip + `[1 (fs 3 10) 2]`, "[[1 -7 2]]", "a carrier between two plain elements"},
		{lmcFlip + `each [1 2 3] [(fs 3 10)]`, "[[3]]", "the each DATA argument — the guard's own shape: [1 2 3] is the body"},
		{lmcFlip + `[(fs 3 10)] each [1 2]`, "[[2]]", "the data list written before the each"},
		{lmcFlip + `{n: [(fs 3 10)]}`, "[{n:[-7]}]", "a LIST-valued map entry — the RecordMakeMap caller of the same record"},
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
			t.Errorf("%q: compiled — expected a sound refusal (%s)", c.src, c.note)
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
	// The negative: `fs/v` is the fn VALUE, a legitimate list member. The
	// program still refuses (the overloaded wrapper's own diagnostics), but
	// NOT as this guard's corruption, and the interpreter places the value.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, got, _, cerr := a.CompileCheck(lmcFlip + `[fs/v]`)
	if cerr != nil {
		t.Fatal(cerr)
	}
	if prog != nil {
		t.Errorf("`[fs/v]`: compiled — the row's premise (a refusal for another reason) has moved")
	}
	if strings.Contains(got, reason) {
		t.Errorf("`[fs/v]`: refused as a corrupted body (%q) — a /v member is a genuine data list", got)
	}
	d, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, errI := d.RunInterp(lmcFlip + `[fs/v]`)
	if errI != nil || !strings.HasPrefix(fmt.Sprint(gotI), "[[fn fs(") {
		t.Errorf("`[fs/v]`: interpreter answers %v (%v), want the fn value inside the list", gotI, errI)
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
