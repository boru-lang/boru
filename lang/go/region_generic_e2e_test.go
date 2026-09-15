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

// The shapes the review of #460 found, each judged against the interpreter.
// The first two keep their committed call — a body-local callee (its unit
// is reached by index; a live lookup finds no binding) and a `/v` operand
// (the op defers on it unconditionally) — so they compile and run compiled
// as before the route existed. (The third decline, a callee with captures,
// is pinned at the seam: no def-bound closure call over a live slot
// compiles today, refusing earlier on its read window.) The next two route
// WITH the lead's modifiers: `w/f` over
// a mixed barrier claims both operands forward live as it did at the
// record, and `w/1` selects the one-operand overload live as the record
// did. The last is the pattern pair — overloads `[0]` and `[n:Integer]`
// differ only in the pattern — where the unit compiled for `[0]` must not
// answer the live match of `[n:Integer]`: the rebind re-records `go`
// (the memo keeps its key on a routed read, review of #461), the second
// unit commits to the other arm, and both lanes answer 100 then 7; at the
// seam, the unit's identity includes its patterns, so a live mismatch no
// call site could re-record defers rather than raise the other arm's
// argument.
func TestRoutedDispatchReviewShapes(t *testing.T) {
	rows := []struct {
		src, want string
		routed    bool
		compiled  bool
	}{
		{`def k 5 end def zzouter fn [[x:Integer][Integer][def zzinner fn [[a:Any b:Any][Integer][x]] end zzinner k 1]] end zzouter 42`, "[42]", false, true},
		{`def w fn [[a:Any b:Any][Any][a]] end def k 5 end def go fn [[][Any][w k/v 1]] end go`, "[5]", false, true},
		{`def w fn [[a:Any | b:Any][Any][a]] end def k 5 end def go fn [[][Any][w/f k 1]] end go`, "[5]", true, true},
		{`def w fn [[a:Any][Any][a] [a:Any b:Any][Any][b]] end def k 5 end def go fn [[][Any][w/1 k 1 drop]] end go`, "[5]", true, true},
		{`def w fn [[0][Integer][100] [n:Integer][Integer][n]] end def k 0 end def go fn [[][Integer][w k]] end go def k 7 end go`, "[100 7]", true, true},
	}
	for _, c := range rows {
		dis := compileDisasm(t, c.src)
		if strings.Contains(dis, "DISPATCH_GENERIC") != c.routed {
			t.Errorf("%q: routed=%v, want %v:\n%s", c.src, !c.routed, c.routed, dis)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errC != nil || errI != nil || fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v/%v interp=%v/%v, want %s", c.src, gotC, errC, gotI, errI, c.want)
		}
		if compiled != c.compiled {
			t.Errorf("%q: ran compiled=%v, want %v", c.src, compiled, c.compiled)
		}
	}
}

// The NATIVE seat routes too (the sixty-fifth increment): a native dispatch
// inside a fn unit whose claim carries a live word slot — `add k 1` — goes
// through its descriptor, and the op's native arm calls the
// LIVE handler over the live operands. The same three spellings as the user
// seat, judged against the interpreter, and the disassembly names the
// unit-less route.
func TestRoutedDispatchAnswersTheNativeSeat(t *testing.T) {
	rows := []struct{ src, want string }{
		{`def k 5 end def go fn [[][Integer][add k 1]] end go`, "[6]"},
		{`def k 5 end def go fn [[][Integer][add k 1]] end go def k 7 end go`, "[6 8]"},
		{`def k 5 end def go fn [[][Integer][add k 1]] end def h go/v end (h) def k 7 end (h)`, "[6 8]"},
		{`def k 5 end def go fn [[n:Integer][Integer][add k n]] end go 1 def k 10 end go 1`, "[6 11]"},
	}
	for _, c := range rows {
		dis := compileDisasm(t, c.src)
		if !strings.Contains(dis, "(native)") {
			t.Errorf("%q: the fn-unit native dispatch over the live slot k routes through its descriptor, unit-less:\n%s", c.src, dis)
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
	// A list literal in the span is not drivable — the interpreter evaluates
	// a data list's contents on arrival — so the committed call stays.
	if dis := compileDisasm(t, `def k 5 end def go fn [[][Integer][size [k 1]]] end go`); strings.Contains(dis, "DISPATCH_GENERIC") {
		t.Errorf("a list literal in the span keeps the committed call:\n%s", dis)
	}
}

// A routed slot is a DYNAMIC-SCOPE read: the registry at the moment of the
// dispatch, where the interpreter resolves it. So every binding of the name
// the compiled program makes in a FRAME lowers the registry-visible
// BIND_DYN_SCOPE twin (the name joins routedNames at the route; a root def's
// bind twin replays it already), and the routed read sees the binding the
// interpreter sees — a fn body's `def k 9` before the call (torn down with
// the frame: `go f go` is `5 9 5`), a param of the name, and a top-level
// loop's carried rebind, whose frame-slot STORE alone the registry never
// saw (`for 2 [ go  def k 9 ]` answered `5 5` for the interpreter's `5 9`).
// Both found off the corpus on the sixty-fifth increment's tree; the loop
// shape first refused (the census does not admit a new refusal site), the
// fn-body shape was found closing that refusal. A read of a name a loop
// ALREADY carries keeps its committed call (the other order).
func TestRoutedReadSeesEveryBindOfItsName(t *testing.T) {
	rows := []struct{ src, want string }{
		// A fn body shadows the module binding before the routed call.
		{`def k 5 end def w fn [[a:Any b:Any][Any][a]] end def go fn [[][Any][w k 1]] end def f fn [[][Any][def k 9 go]] end go f go`, "[5 9 5]"},
		{`def k 5 end def go fn [[][Integer][add k 1]] end def f fn [[][Integer][def k 9 go]] end go f go`, "[6 10 6]"},
		// A top-level loop carries the name the unit reads live.
		{`def w fn [[a:Any b:Any][Any][a]] end def k 5 end def go fn [[][Any][w k 1]] end for 2 [ go  def k 9 ]`, "[5 9]"},
		{`def k 5 end def go fn [[][Integer][add k 1]] end for 2 [ go  def k 9 ] k`, "[6 10 9]"},
		{`def k 5 end def go fn [[][Integer][add k 1]] end for 2 [ go  def k (k add 1) ] k`, "[6 7 7]"},
		{`def w fn [[a:Integer b:Integer][Integer][a]] end def k 5 end def go fn [[][Integer][w k 1]] end for 2 [ go  def k 9 ]`, "[5 9]"},
		// A PARAM of the name shadows it for the callee's routed read.
		{`def k 5 end def w fn [[a:Any b:Any][Any][a]] end def z fn [[][Any][w k 1]] end def go fn [[k:Integer][Any][z]] end z go 9 z`, "[5 9 5]"},
	}
	for _, c := range rows {
		dis := compileDisasm(t, c.src)
		if !strings.Contains(dis, "DISPATCH_GENERIC") || !strings.Contains(dis, "BIND_DYN_SCOPE") {
			t.Errorf("%q: the read routes and the frame's bind is twinned into the registry:\n%s", c.src, dis)
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
	// A ROOT rebind owes no dyn twin: its bind twin replays it at its
	// position, which is what the k pair has always read.
	if dis := compileDisasm(t, `def k 5 end def go fn [[][Integer][add k 1]] end go def k 7 end go`); !strings.Contains(dis, "DISPATCH_GENERIC") || strings.Contains(dis, "BIND_DYN_SCOPE") {
		t.Errorf("a root rebind of a routed name keeps its bind twin alone:\n%s", dis)
	}
	// The other order: the loop carries k before any unit reads it, and the
	// read keeps its committed call.
	src := `def k 5 end for 2 [ def k 9 ] def go fn [[][Any][add k 1]] end go`
	if dis := compileDisasm(t, src); strings.Contains(dis, "DISPATCH_GENERIC") {
		t.Errorf("a read of a carried name keeps its committed call:\n%s", dis)
	}
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	requireParity(t, src, gotC, errC, gotI, errI)
}

// The review of #461's three, each judged against the interpreter, each
// with an effect before the rebind (the shape where a defer's fallback is
// fenced and the user saw an internal error): a value-dependent divergent
// word whose check-time call recorded no result (`10 div y` caught at 0),
// a rebind that changes the word's overload (`set` over a Map, then a
// Store), and a module native reached through its wrapper (`StructUtil.clone
// k`, dispatched in the module's registry). The first two re-record on the
// rebind (the memo keeps its key); the third routes and resolves its lead
// in the descriptor's registry.
func TestRoutedDispatchReviewOfTheNativeSeat(t *testing.T) {
	rows := []string{
		`print "before" end def y 0 end def f fn [[] [Any] [10 div y]] end do [f] end drop end def y 2 end f`,
		`print "before" end def k {} end def f fn [[] [] [set x/q 1 k]] end f end drop end def k (context) end f`,
		`print 1 import "boru:struct-util" end def k {a:1} end def f fn [[] [Map] [StructUtil.clone k]] end f`,
	}
	for _, src := range rows {
		c, err := New()
		if err != nil {
			t.Fatal(err)
		}
		var bails []string
		disarm := c.ArmRuntimeBailHook(func(ev BailEvent) { bails = append(bails, ev.Site) })
		gotC, compiled, errC := c.RunCompiled(src)
		disarm()
		d, _ := New()
		gotI, errI := d.RunInterp(src)
		if !compiled || len(bails) != 0 || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: want a compiled run with no defer and the interpreter's answer:\n  compiled=%v bails=%v\n  C=%v/%v\n  I=%v/%v", src, compiled, bails, gotC, errC, gotI, errI)
		}
	}
	// The divergent word keeps its committed call: no route at all.
	if dis := compileDisasm(t, `def y 2 end def f fn [[] [Any] [10 div y]] end f`); strings.Contains(dis, "DISPATCH_GENERIC") {
		t.Errorf("a value-dependent divergent word is never routed:\n%s", dis)
	}
}
