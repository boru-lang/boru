package lang

import (
	"fmt"
	"strings"
	"testing"
)

// closure_read_model_test.go pins the thirty-sixth increment: the read model
// (the thirty-fifth increment's tryShapedFnReadArrival) over a def-bound
// PRODUCED closure. The def of a produced closure claims the shape its
// producer builds (the closure unit's param count, the unit's own single
// closure out-op recursing for a factory of factories), so the read models
// the word dispatch at the read; a claim whose RESULT is a claimed fn (a
// curried chain's next level) makes the read's result a shaped Function
// carrier, so the next def binds a shaped carrier and the chain compiles
// level by level. The residual classifier's flattened window lost the
// statement for these too: `h 2 ; 3` over a two-param closure lowered as
// `h 2 3` (compiled 12 for the interpreter's signature_error).

const crmMk = `def mk fn [[g:Function][Function][( fn [[v:Integer][Integer][(g v)]] )]] end def h (mk (z:Integer => [add 7 z])) end `
const crmMk2w = `def mk fn [[g:Function][Function][( fn [[v:Integer w:Integer][Integer][(g v) add w]] )]] end def h (mk (z:Integer => [add 7 z])) end `
const crmChain = `def mk2 fn [[a:Integer][Function][( fn [[b:Integer][Function][( fn [[c:Integer][Integer][add a (add b c)]] )]] )]] end def f1 (mk2 1) end def f2 (f1 2) end `

func TestClosureReadModelParity(t *testing.T) {
	rows := []struct{ src, note string }{
		{crmMk + `typeof (h 5)`, "Integer — the apply collapses at the read, typeof takes its result (the ledger row)"},
		{`def mkc2 fn [[g:Function][Function][( fn [[v:Integer][Integer][(g v)]] )]] end def h2 (mkc2 (z:Integer => [mul 3 z])) end (h2 5) (h2 10)`, "15 30 — repeated reads of the bound closure (the ledger row)"},
		{crmChain + `(f2 3)`, "6 — the three-level curried chain through def bindings (the ledger row)"},
		{`import "boru:fn-util"  def sub2 fn [[a:Integer b:Integer][Integer][a sub b]] end def c (FnUtil.curry sub2/v) end def c10 (c 10) end (c10 3)`, "7 — the fn-util curry chain (the ledger row)"},
		{`import "boru:fn-util"  def sub3 fn [[a:Integer b:Integer c:Integer][Integer][a sub (b sub c)]] end def c (FnUtil.curry sub3/v) end def c1 (c 10) end def c2 (c1 3) end (c2 1)`, "8 — three curry levels"},
		{crmMk + `(h 5)`, "12 — the plain read"},
		{crmMk + `h 5 ; 9`, "12 9 — the read's window is its statement"},
		{crmMk2w + `(h 2 3)`, "12 — a two-param closure's full window"},
		{`def mkf fn [[n:Integer][Function][( fn [[x:Integer][Integer][x add n]] )]] end def f (mkf 1) end (f 2) (f 3)`, "3 4 — a capturing closure read twice"},
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

// TestClosureReadModelSoundRefusals pins the neighbours that REFUSE, with
// the interpreter's own answer.
func TestClosureReadModelSoundRefusals(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		// the token past the arity stays inside the paren with the result
		{crmMk + `(h 5 9)`, "bounded by a paren", "[12 9]"},
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
	// The flattened-window miscompile the claim closes: the interpreter
	// dispatches `h` at the `;` with one argument and raises; the residual
	// classifier saw [h, 2, 3] and applied both (compiled 12).
	for _, src := range []string{crmMk2w + `h 2 ; 3`, crmMk2w + `(h 2 ; 3)`} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — the window crosses the statement", src)
			continue
		}
		if !strings.Contains(reason, "the statement ends short of the wrapper's arity") {
			t.Errorf("%q: refused %q", src, reason)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if _, errI := d.RunInterp(src); errI == nil || !strings.Contains(errI.Error(), "cannot call `h`") {
			t.Errorf("%q: interpreter answers %v, want the signature_error", src, errI)
		}
	}
}

// TestClosureReadModelFilterBodyIslanded pins the filter-body twin as
// measured: it compiles and agrees (the filter_error on both lanes) but the
// filter body islands, so it stays ledgered as islanded.
func TestClosureReadModelFilterBodyIslanded(t *testing.T) {
	for _, src := range []string{
		crmMk + `filter [1 2] [gt 0 (h 5)]`,
		`def mk fn [[a:Integer][Function][( fn [[b:Integer][Integer][add a b]] )]] end def f (mk 1) end each [1 2 3] [(f 1)]`,
	} {
		gotC, compiled, islands, errC := runCompiledNative(t, src)
		if !compiled {
			t.Fatalf("%q: not compiled", src)
		}
		if len(islands) == 0 {
			t.Errorf("%q: runs VM-native now — graduate it from the ledger", src)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(src)
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}
