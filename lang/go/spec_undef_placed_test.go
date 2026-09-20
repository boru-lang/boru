package lang

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The sixty-eighth increment: a speculative undef of an ENCLOSING binding —
// an `undef` inside a region the runtime may never execute (a branch arm, a
// loop or while body, a fn body), which the check pass keeps in its model
// (the wrapped-undef leniency: a region that never runs must raise
// nothing) — is PLACED and its reads are LIVE, where the sixty-seventh
// increment declined it. The model generalises the binding's value in place
// (core.GeneraliseSpecUndef), the recorder seats the pop at its site
// (OpUndefDynScope), every later read of the name is a live lookup whose
// miss raises the interpreter's undefined_word at the read token
// (Program.SpecUndefNames), and a loop re-rounds so a read recorded before
// the undef in the same body is re-recorded live. NUR144's rows, the branch
// arm, the while body, the fn body and the module-call row of #463 all
// compile and answer as the interpreter does, position included — with an
// effect before the undef too, where a defer's re-run would be fenced.
func TestSpeculativeUndefIsPlacedAndReadLive(t *testing.T) {
	compiled := []struct {
		src, want string
		live      bool // a read of the name follows the region: lowered as a live lookup
		routed    bool // the read is a forward word slot of a dispatch that routes (DISPATCH_GENERIC resolves it from its window)
	}{
		// NUR144's stack-operand row: the fn unit re-records its read live.
		{`def k 5 end def f fn [[][Integer][k add 2]] end for 2 [ f  undef k ]`, "undefined_word@1:35", true, false},
		{`def k 5 end if true [undef k] [] k`, "undefined_word@1:34", true, false},
		{`def k 5 end if false [undef k] [] k`, "[5]", true, false},
		{`def k 5 end for 2 [ undef k ] k`, "undefined_word@1:31", true, false},
		// The compiled program looped for ever before the sixty-seventh.
		{`def k 5 end while [k eq 5] [undef k] 9`, "undefined_word@1:20", true, false},
		{`def k 5 end def f fn [[][Integer][undef k 1]] end f k`, "undefined_word@1:53", true, false},
		{`def k 5 end def f fn [[][Integer][if true [undef k] [] k]] end f`, "undefined_word@1:56", true, false},
		{`def k 5 end for 2 [ if (k eq 5) [undef k] [] ] 9`, "undefined_word@1:25", true, false},
		{`def k [1 2] end if true [undef k] [] k`, "undefined_word@1:38", true, false},
		{`import module [def m fn [[] [Integer] [1]] export "M" {m:m/v}] end def k 5 end if true [M.m drop undef k 1] [1] k`, "undefined_word@1:113", true, false},
		// An effect before the undef: a defer's re-run would be fenced into
		// an internal error; the miss raises the word instead.
		{`def k 5 end if true [(print "x") undef k] [] k`, "undefined_word@1:46", true, false},
		{`def k 5 end for 2 [ (print "x") undef k ] k`, "undefined_word@1:43", true, false},
		{`def k 5 end if true [undef k] [] (print "z") k`, "undefined_word@1:46", true, false},
		// A root def after the region re-binds the name: the twin replays it,
		// and the read after it bakes the new binding.
		{`def k 5 end if true [undef k] [] def k 6 end k`, "[6]", false, false},
		{`def k 5 end for 2 [ undef k ] def k 7 end k`, "[7]", false, false},
		// Two undefs of one binding: the second pops nothing on either lane.
		{`def k 5 end if true [undef k undef k] [] 1`, "[1]", false, false},
		// A read BEFORE the region keeps its bake, routed or const.
		{`def k 5 end k for 2 [ undef k ] 9`, "[5 9]", false, false},
		{`def k 5 end def go fn [[][Integer][add k 1]] end go if true [undef k] [] 9`, "[6 9]", false, true},
		// Every read is seated AT ITS TOKEN (review of #464): the lookup, and
		// the undefined_word it raises, executes before a later effect and
		// before the value is consumed — at the first k, with nothing printed;
		// two reads in one statement each carry their own caret; a read the
		// undef follows in the same loop body raises on the second pass.
		{`def k 5 end if true [undef k] [] k (print "x") k`, "undefined_word@1:34", true, false},
		{`def k 5 end if true [undef k] [] k k add`, "undefined_word@1:34", true, false},
		{`def k 5 end for 2 [ k undef k ] 9`, "undefined_word@1:21", true, false},
		{`def k 5 end if false [undef k] [] k k add`, "[10]", true, false},
		{`def k 5 end if false [undef k] [] k drop 9`, "[9]", true, false},
		// A dynamic code body's read after the region (review of #464): the
		// root def is installed ONCE — its twin's replay — so the placed undef
		// pops the binding the body then misses, and `do` catches the raise on
		// both lanes.
		{`def k 5 end if true [undef k] [] do [k]`, "", false, false},
		// A FORWARD-slot read of the popped name ROUTES (the sixty-ninth
		// increment): the op resolves the slot from its window as the
		// interpreter collects it — a typed slot takes the unbound word as a
		// Word value and no signature matches, at the dispatching word; an Any
		// slot claims it and the token then dispatches, undefined_word at the
		// token — at root and in a unit, at the mono, poly and user records.
		{`def k 5 end if true [undef k] [] add k 1`, "signature_error@1:34", false, true},
		{`def k 5 end if false [undef k] [] add k 1`, "[6]", false, true},
		{`def k 5 end if true [undef k] [] [1 2] get k`, "undefined_word@1:44", false, true},
		{`def k 5 end if true [undef k] [] size k`, "undefined_word@1:39", false, true},
		{`def k 5 end def go fn [[][Integer][add k 1]] end for 2 [ go  undef k ]`, "signature_error@1:36", false, true},
		{`def k 5 end def go fn [[][Integer][add k 1]] end if true [undef k] [] go`, "signature_error@1:36", false, true},
		{`def k 5 end def go fn [[][Integer][sub 1 k]] end if true [undef k] [] go`, "signature_error@1:36", false, true},
		{`def k 5 end def g fn [[x:Integer][Integer][x add 1]] end if true [undef k] [] g k`, "signature_error@1:79", false, true},
		{`def k 5 end def g fn [[x:Any][Any][x]] end if true [undef k] [] g k`, "undefined_word@1:67", false, true},
		{`def k 5 end def f fn [[x:Any][Any][x get k]] end if true [undef k] [] f {k:1}`, "undefined_word@1:42", false, true},
		{`def k 5 end def f fn [[x:Any][Any][add x k]] end if true [undef k] [] f 1`, "signature_error@1:36", false, true},
		// A routed slot inside a loop the analysis re-rounds (review of
		// #465): the discarded round's placeholder mark is pruned with its
		// events, so the stabilised round's read keeps its lookup.
		{`def k 5 end if false [undef k] [] (print "x") def a 1 end for 2 [def a (a add 0.5) end k add k 1 drop]`, "[5 5]", true, true},
	}
	for _, c := range compiled {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: must compile, got reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if dis := prog.Disassemble(); !strings.Contains(dis, "UNDEF_DYN_SCOPE") || strings.Contains(dis, "LOOKUP_DYN_SCOPE") != c.live || strings.Contains(dis, "DISPATCH_GENERIC") != c.routed {
			t.Errorf("%q: the undef is placed and the read after it is live or routed:\n%s", c.src, dis)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		// The first line — code, detail, position — is the parity; the
		// did-you-mean line below it is NUR146's: the compiled raise
		// suggests over the registry, the interpreter's over a registry
		// that holds the frame's loop iterator as a def binding too.
		requireParityHead(t, c.src, gotC, errC, gotI, errI)
		if code, _, isErr := strings.Cut(c.want, "@"); isErr {
			var ce, ie *core.BoruError
			if !errors.As(errC, &ce) || !errors.As(errI, &ie) || ce.Code != code || ie.Code != code {
				t.Errorf("%q: both lanes raise %s: compiled=%v interp=%v", c.src, code, errC, errI)
				continue
			}
			at := fmt.Sprintf("%s@%d:%d", code, ce.Row, ce.Col)
			if at != c.want || ce.Row != ie.Row || ce.Col != ie.Col {
				t.Errorf("%q: the compiled raise sits where the interpreter's does: compiled %d:%d interp %d:%d want %s", c.src, ce.Row, ce.Col, ie.Row, ie.Col, c.want)
			}
			continue
		}
		if c.want == "" {
			if errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
				t.Errorf("%q: both lanes answer alike: compiled=%v/%v interp=%v/%v", c.src, gotC, errC, gotI, errI)
			}
			continue
		}
		if errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled=%v/%v, want %s", c.src, gotC, errC, c.want)
		}
	}
	// What still declines, each through the undef site, and answers as the
	// interpreter does under the hatch: a recording the placement cannot
	// seat (an each body's first run, a `do` inside a loop, the never-running
	// error handler the leniency exists for), a binding the model declines
	// to generalise (a fn-family value, a type, a param, a fn-local), a name
	// a loop carries (its reads are slot reads — the loop's joined twin
	// replays one install for N iterations, so `for 2 [ def k 6 ] undef k k`
	// answered the pre-loop 5 for the interpreter's 6 on main), and a `def`
	// of the name inside the region that undefs it. A FORWARD-slot read of
	// the popped name routes since the sixty-ninth increment (the compiled
	// rows above); one whose dispatch cannot route — a region the op cannot
	// drive, here a paren group in the window — still declines.
	declined := []struct{ src, reason string }{
		{`def k 5 end [1 2] each [undef k] k`, "undef of the enclosing binding `k`"},
		{`def k 5 end def f fn [[][Integer][k add 2]] end for 2 [ f  do [undef k] ]`, "undef of the enclosing binding `k`"},
		{`def x 1 end do [7] error [undef x 9] x`, "undef of the enclosing binding `x`"},
		{`def g fn [[][Integer][1]] end if true [undef g] [] 2`, "undef of the enclosing binding `g`"},
		{`def T Integer end if true [undef T] [] 1`, "undef of the enclosing binding `T`"},
		{`def f fn [[k:Integer][Integer][if true [undef k] [] k]] end f 1`, "undef of the enclosing binding `k`"},
		{`def f fn [[][Integer][def j 1 if true [undef j] [] j]] end f`, "undef of the enclosing binding `j`"},
		{`def k 5 end for 2 [ def k 6 ] undef k k`, "undef of the loop-carried def `k`"},
		{`def k 5 end for 2 [ def k 6 ] if true [undef k] [] k`, "undef of the enclosing binding `k`"},
		{`def k 5 end for 2 [ def k 6 for 1 [ undef k ] ] k`, "def of `k` inside the region that undefs it"},
		{`def k 5 end if true [undef k def k 6] [] k`, "def of `k` inside the region that undefs it"},
		{`def k 5 end for 2 [ undef k def k 6 ] k`, "def of `k` inside the region that undefs it"},
		{`def k 5 end def f fn [[][Integer][if true [undef k] [] def k 6 end k]] end f k`, "def of `k` inside the region that undefs it"},
		{`def k 5 end if true [undef k] [] add k (1 add 1)`, "forward-slot read of `k` after a placed undef"},
		// The same at the user and poly records, and — review round of
		// #465 — a generalised slot BEYOND the claim: the claim stops at a
		// paren group or a body-local name, and the word after it is still
		// this dispatch's forward operand (a lookup there raised
		// undefined_word where the interpreter no-matches at the word).
		{`def k 5 end def g fn [[x:Integer y:Integer][Integer][x add y]] end if true [undef k] [] g k (1 add 1)`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def g fn [[x:Integer y:Integer][Integer][x add y]] end if true [undef k] [] g (1 add 1) k`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def g fn [[x:Integer y:Integer][Integer][x add y]] end if false [undef k] [] g (1 add 1) k`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def g fn [[a:Integer b:Integer][Integer][a add b]] end def f fn [[][Integer][def j 1 end g j k]] end if true [undef k] [] f`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def f fn [[x:Any][Any][add k (x)]] end if true [undef k] [] f 1`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def f fn [[x:Any][Any][add (x) k]] end if true [undef k] [] f 1`, "forward-slot read of `k` after a placed undef"},
		// The poly record (a `get` over a gradual operand re-matches at run
		// time): a container literal in the window is undrivable, and a slot
		// the loop's carried name declines is unrouted.
		{`def k 5 end def f fn [[x:Any][Any][x get k [9]]] end if true [undef k] [] f {k:1}`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def f fn [[x:Any][Any][for 2 [def j 1 end x get j drop x get k]]] end if true [undef k] [] f {k:1}`, "forward-slot read of `k` after a placed undef"},
		// A `/v` read takes no tag hook: the rescue declines it rather than seat
		// it late.
		{`def k 5 end if true [undef k] [] add k k/v`, "read of `k` after a placed undef the placement did not seat"},
	}
	// A dispatch the static match cannot commit — a multi-arm user fn over a
	// generalised Any or disjunct binding, or beside one — never reaches the
	// generic seat: the rematch trap's operand layout declines it (the read's
	// identity is its event's, not the binding's the window resolves), and
	// the recovery's arm plan declines a plain carrier (review of #465). Both
	// are upstream of the undef site, and the hatch answers as the
	// interpreter does.
	declined = append(declined, []struct{ src, reason string }{
		{`def c false end def id fn [[x:Any][Any][x]] end def k (id 5) end def g fn [[x:Integer][Integer][x] [x:String][String][x]] end if c [undef k] [] g k`, "rematch operand is not on top (rematch of g)"},
		{`def c false end def m {e: true} end def k (if (m "e" get) [7] ["s"]) end def g fn [[x:Integer][Integer][x] [x:String][String][x]] end if c [undef k] [] g k`, "rematch operand is not on top (rematch of g)"},
		{`def c false end def m {e: true} end def v (if (m "e" get) [7] ["s"]) end def k 5 end def g2 fn [[a:Integer x:Integer][Integer][a] [a:Integer x:String][Integer][a add 1]] end if c [undef k] [] g2 k v`, "unmatched dispatch recovered at g2"},
	}...)
	for _, c := range declined {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: %v", c.src, cerr)
		}
		if prog != nil || !strings.Contains(reason, c.reason) {
			t.Errorf("%q: want the compile failure %q…, got compiled=%v reason=%q", c.src, c.reason, prog != nil, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
	// An undef of a binding made INSIDE the region, and of a name never
	// bound at all, compile as they did.
	inRegion := []struct{ src, want string }{
		{`def k 5 end for 2 [ def k 6 undef k ] 9`, "[9]"},
		{`def f fn [[][Integer][def j 1 undef j 2]] end f`, "[2]"},
		{`def k 5 end for 2 [ def j 1 undef j ] 9`, "[9]"},
		{`def k 5 end undef k end def k 6 end k`, "[6]"},
		{`def f fn [[][Integer][undef nope 1]] end f`, "[1]"},
		{`def k 5 end if true [undef nope 1] [2] k`, "[1 5]"},
	}
	for _, c := range inRegion {
		gotC, ran, errC, gotI, errI := runBothEngines(t, c.src)
		if !ran || errC != nil || errI != nil {
			t.Errorf("%q: an in-region undef compiles: ran=%v errC=%v errI=%v", c.src, ran, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled=%v interp=%v, want %s", c.src, gotC, gotI, c.want)
		}
	}
}

// The binding predates the compiled request (a long-lived registry, review
// of #464): the check pass generalised it in place with no twin to trigger
// the replay's restore, so the live entry held the pass's carrier — the
// program answered the carrier for the interpreter's 5. A program that
// placed a speculative undef restores the base before it runs, twins or
// not, and the request after it reads the binding the base holds.
func TestSpeculativeUndefAcrossRequests(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{`def k 5`, `if false [undef k] [] k`, `k`, `if true [undef k] [] 1`, `k`} {
		gotC, ran, errC := a.RunCompiled(src)
		gotI, errI := b.RunInterp(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if !ran && src != `k` {
			t.Errorf("%q: compiles on the long-lived registry: %v", src, errC)
		}
		requireParityHead(t, src, gotC, errC, gotI, errI)
		if src == `k` && errI == nil && (errC != nil || fmt.Sprint(gotC) != "[5]") {
			t.Errorf("%q: the base binding stands after the request: %v %v", src, gotC, errC)
		}
	}
}
