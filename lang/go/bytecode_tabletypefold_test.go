package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestModuleTableTypeFold: a module-exported Table TYPE reached through
// dot-access (`Test.TestSet`) now const-folds like any other structural type
// body. Previously only a BARE type node (`Test.TestCase`, Data==nil) folded;
// a Table type carries a TableTypeInfo payload, so the get declined "operand of
// unknown provenance ... at get" and the whole statement fell back. With Table
// admitted to the const-bakeable structural-type-body family, the get bakes the
// immutable type and the downstream make/is/istype compile natively.
//
// Positive: each form compiles WITHOUT a FALLBACK island and matches the
// interpreter. The negative half (a carrier in the row schema must keep the
// type OFF the const pool) is pinned directly at the eng level in
// TestIsInertConstTableTypeBody; here the no-regression guard is that a LOCALLY
// defined Table type bakes with the same parity, not just the module one.
func TestModuleTableTypeFold(t *testing.T) {
	const imp = `import "boru:test"  `
	cases := []struct{ src, want string }{
		{imp + `Test.TestSet istype`, "[true]"},
		{imp + `Test.TestCase istype`, "[true]"}, // bare type node — still folds
		{imp + `(make Test.TestSet [[name: "a" in: [1] out: 2]]) is Test.TestSet`, "[true]"},
	}
	for _, c := range cases {
		a, _ := New()
		prog, reason, _, _ := a.CompileCheck(c.src)
		if prog == nil {
			t.Errorf("%q: must compile (module Table-type fold), but declined: %q", c.src, reason)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile native (folded type), not island:\n%s", c.src, prog.Disassemble())
		}
		ar, _ := New()
		gotC, compiled, errC := ar.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: parity broke: compiled=%v gotC=%v errC=%v gotI=%v want=%s",
				c.src, compiled, gotC, errC, gotI, c.want)
		}
	}

	// NO-REGRESSION — admitting Table to the const whitelist must not diverge
	// for a LOCALLY defined Table type either (not just the module-fold path).
	// A locally-def'd Table type baked as a const must still produce the
	// interpreter's result; an unsound const-bake would break parity here.
	const local = `def Row refine Record [n:Integer]  def T refine Table Row  T istype`
	d, _ := New()
	dp, _, _, _ := d.CompileCheck(local)
	if dp == nil {
		t.Errorf("%q: a locally-def'd Table type must compile once Table bakes as a const", local)
	}
	dr, _ := New()
	gotC, compiled, errC := dr.RunCompiled(local)
	if noteCompileDefect(t, local, gotC, errC) {
		return
	}
	e, _ := New()
	gotI, errI := e.RunInterp(local)
	if errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[true]" {
		t.Errorf("%q: parity broke (compiled=%v): gotC=%v errC=%v gotI=%v errI=%v",
			local, compiled, gotC, errC, gotI, errI)
	}
}
