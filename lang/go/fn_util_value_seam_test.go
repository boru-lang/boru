package lang

import (
	"fmt"
	"testing"
)

// fn_util_value_seam_test.go pins the fn-util wrapper at the token seam
// (2026-09-24, the interp-entry census's callbacks.tsv:L154): a
// self-contained Go-implemented fn value handed to a native's body seam
// (`each h/v xs`) applies on its own signatures, top-down over the seam's
// inputs with the rest left beneath, as the quotation body's trailing
// apply already did — where it was stepped on the interpreter once per
// element. The rows enter nothing; a `flip`ped wrapper (a usurped
// signature, never applied natively) and filter's predicate seam still
// enter, pinned open.

const fuSeam = `import "boru:fn-util"  def inc x:Integer => [add 1 x] end def dbl x:Integer => [mul 2 x] end def sub2 fn [[a:Integer b:Integer][Integer][a sub b]] end `

var fnUtilValueSeamRows = []struct {
	label, src, want string
	native           bool
}{
	{"compose under each", fuSeam + `def h (FnUtil.compose inc/v dbl/v) end each h/v [1 2 3]`, "[[3 5 7]]", true},
	{"a one-param wrapper under fold takes the element, leaves the accumulator", fuSeam + `def h (FnUtil.compose inc/v dbl/v) end 1 fold h/v [1 2]`, "[5]", true},
	{"partial under fold", fuSeam + `def p (FnUtil.partial sub2/v 10) end 0 fold p/v [1 2 3]`, "[7]", true},
	{"partial under scan", fuSeam + `def p (FnUtil.partial sub2/v 10) end scan p/v [1 2 3]`, "[[1 8 7]]", true},
	{"memoize under each", fuSeam + `def d (FnUtil.memoize inc/v) end each d/v [1 2]`, "[[2 3]]", true},
	// The module factory's closure read as an each body: the recorder's
	// registry follows the running engine again after the inline module's
	// body (BindRegistry's restore), so the def's shape claim lands where
	// the read looks.
	{"a module factory's closure as an each body", `import module [def mk fn k:Integer Function [([n:Integer] => [n add k])] export "M" {mk: mk/v}] end def a5 (M.mk 5) end each [a5] [1 2 3]`, "[[6 7 8]]", true},
	{"a module factory's closure applied", `import module [def mk fn k:Integer Function [([n:Integer] => [n add k])] export "M" {mk: mk/v}] end def a5 (M.mk 5) end (a5 7)`, "[12]", true},
	// A usurped (flipped) signature keeps the interpreter's collection
	// around its handler — the seam's native apply declines it (open).
	{"flip under fold (open)", fuSeam + `def fs (FnUtil.flip sub2/v) end 100 fold fs/v [1 2 3]`, "[94]", false},
	// filter's predicate seam runs a Go-implemented value through the
	// callback path, which has no native arm yet (open).
	{"compose under filter (open)", fuSeam + `def h (FnUtil.compose inc/v dbl/v) end filter h/v [1 2 3]`, "[[]]", false},
}

func TestFnUtilValueSeamParityAndNoEntry(t *testing.T) {
	for _, row := range fnUtilValueSeamRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != row.want {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}

// TestFnUtilValueSeamNoMatchRaisesAlike: the wrapper's own no-match under
// the seam is the interpreter's error, named and positioned alike.
func TestFnUtilValueSeamNoMatchRaisesAlike(t *testing.T) {
	src := fuSeam + `def h (FnUtil.compose inc/v dbl/v) end each h/v ['s']`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || codeOf(errI) != "signature_error" || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 || len(gotI) != 0 {
		t.Errorf("%q: compiled %v [%s] %s (%v), interp %v [%s] %s", src, gotC, codeOf(errC), detailOf(errC), compiled, gotI, codeOf(errI), detailOf(errI))
	}
}
