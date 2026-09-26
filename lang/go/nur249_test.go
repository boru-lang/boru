package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR249ZeroArgCarrierUnderAWindow pins NUR249's silent half: a fn-typed
// carrier read by NAME and applied over a paren window, which turns out
// 0-arg at run time, fires over nothing and leaves its window beside its
// result on both lanes (NUR176) — n+1 values where the record seats one. In
// place (a residual, a RET tail) the run is the interpreter's answer; a
// layout a later event consumes seated the one, a silent wrong answer under
// a no-contract fn (`[(k 5)]` compiled `[7 [5]]`). That op raises now
// (DynApplyHead.OneResult), booked on the bail ledger rather than declining
// every named carrier apply — the comparator convention's `(a b comp)` —
// at compile time. A lead that fits, or a 0-arg one whose run nets exactly
// one value, answers on both lanes.
func TestNUR249ZeroArgCarrierUnderAWindow(t *testing.T) {
	const c = `def c fn [[] [Integer] [7]] end `
	for _, r := range []struct{ src, want string }{
		{c + `def h fn [[k:Function] [] [(k 5)]] end h c/v`, "[7 5]"},
		{`def inc fn [[n:Integer] [Integer] [n add 1]] end def h fn [[k:Function] [] [[(k 5)]]] end h inc/v`, "[[6]]"},
		{`def n0 fn [[] [] []] end def h fn [[k:Function] [] [[(k 5)]]] end h n0/v`, "[[5]]"},
	} {
		agreeOnBothLanes(t, r.src, r.want)
	}
	for _, r := range []struct{ src, interp string }{
		{c + `def h fn [[k:Function] [] [[(k 5)]]] end h c/v`, "[[7 5]]"},
		{c + `def h fn [[k:Function b:Boolean] [] [if b [[(k 5)]] [0]]] end h c/v true`, "[[7 5]]"},
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(r.src)
		if !noteCompileDefect(t, r.src, gotC, errC) || !compiled || !strings.Contains(fmt.Sprint(errC), "a 0-arg lead left 2 values") {
			t.Errorf("%s: want the one-result raise, got %v compiled=%v err=%v", r.src, gotC, compiled, errC)
		}
		if gotI, errI := mustNew(t).RunInterp(r.src); errI != nil || fmt.Sprint(gotI) != r.interp {
			t.Errorf("%s: interp got %v / %v, want %s", r.src, gotI, errI, r.interp)
		}
	}
}
