package lang

import (
	"fmt"
	"strings"
	"testing"
)

// The generic lane's first ROUTED dispatch, end to end (the sixty-fourth
// increment): region_desc.go's `k` pair — `def w fn [[a:Any b:Any][Any][a]]
// def k 5 def go fn [[][Any][w k 1]]` — dispatches `w` through its
// descriptor inside go's unit (DISPATCH_GENERIC in place of CALL_USER), so
// `k` is looked up at every execution as the interpreter looks it up: a
// rebind of k between two calls of go is answered by the SAME bytecode,
// with no re-recording, and an ESCAPED go (`def h go/v`) answers it too.
// Each row is judged against the interpreter.
func TestRoutedDispatchAnswersTheKPair(t *testing.T) {
	const w = `def w fn [[a:Any b:Any][Any][a]] end def k 5 end def go fn [[][Any][w k 1]] end `
	rows := []struct{ src, want string }{
		{w + `go`, "[5]"},
		{w + `go def k 7 end go`, "[5 7]"},
		{w + `def h go/v end (h) def k 7 end (h)`, "[5 7]"},
		{`def w fn [[a:Integer b:Integer][Integer][a add b]] end def k 5 end def go fn [[n:Integer][Integer][w k n]] end go 1 def k 10 end go 1`, "[6 11]"},
	}
	for _, c := range rows {
		dis := compileDisasm(t, c.src)
		if !strings.Contains(dis, "DISPATCH_GENERIC") || strings.Contains(dis, "CALL_USER   f1") {
			t.Errorf("%q: the fn-unit dispatch of w over the live slot k routes through its descriptor:\n%s", c.src, dis)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil || errI != nil {
			t.Errorf("%q: compiled=%v errC=%v errI=%v", c.src, compiled, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v, want %s", c.src, gotC, gotI, c.want)
		}
	}
	// The second spelling of the pair — k rebound to a FUNCTION — raises the
	// strict-barrier signature_error on both lanes; it raises in the check
	// pass, which runs go after the rebind, so the routed op never meets it
	// in a compiled run (its defer for that shape is pinned at the seam).
	src := w + `go def k fn [[][Integer][9]] end go`
	_, _, errC, _, errI := runBothEngines(t, src)
	if errC == nil || errI == nil || !strings.Contains(errC.Error(), "begins its own dispatch") || errC.Error() != errI.Error() {
		t.Errorf("the fn rebind raises the identical strict-barrier error on both lanes: compiled=%v interp=%v", errC, errI)
	}
}

// What routing leaves alone: a top-level dispatch (analysis order is program
// order, the bake IS the read), a claim with no live slot, and a span the
// host cannot drive (a group) keep their committed call.
func TestRoutedDispatchKeepsTheCommittedCallElsewhere(t *testing.T) {
	for _, src := range []string{
		`def w fn [[a:Any b:Any][Any][a]] end def k 5 end w k 1`,
		`def w fn [[a:Any b:Any][Any][a]] end def go fn [[n:Integer][Any][w n 1]] end go 5`,
		`def w fn [[a:Any b:Any][Any][a]] end def k 5 end def id fn [[x:Any][Any][x]] end def go fn [[][Any][w k (id 1)]] end go`,
	} {
		dis := compileDisasm(t, src)
		if strings.Contains(dis, "DISPATCH_GENERIC") {
			t.Errorf("%q: not a routed shape:\n%s", src, dis)
		}
	}
}
