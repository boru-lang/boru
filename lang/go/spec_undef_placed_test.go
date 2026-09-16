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
// increment refused it. The model generalises the binding's value in place
// (core.GeneraliseSpecUndef), the recorder seats the pop at its site
// (OpUndefDynScope), every later read of the name is a live lookup whose
// miss raises the interpreter's undefined_word at the read token
// (Program.SpecUndefNames), and a loop re-rounds so a read recorded before
// the undef in the same body is re-recorded live. NUR144's rows, the branch
// arm, the while body, the fn body and the module-call row of #463 all
// compile and answer as the interpreter does, position included — with an
// effect before the undef too, where a defer's re-run would be fenced.
func TestSpeculativeUndefIsPlacedAndReadLive(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	compiled := []struct {
		src, want string
		live      bool // a read of the name follows the region: lowered as a live lookup
	}{
		// NUR144's stack-operand row: the fn unit re-records its read live.
		{`def k 5 end def f fn [[][Integer][k add 2]] end for 2 [ f  undef k ]`, "undefined_word@1:35", true},
		{`def k 5 end if true [undef k] [] k`, "undefined_word@1:34", true},
		{`def k 5 end if false [undef k] [] k`, "[5]", true},
		{`def k 5 end for 2 [ undef k ] k`, "undefined_word@1:31", true},
		// The compiled program looped for ever before the sixty-seventh.
		{`def k 5 end while [k eq 5] [undef k] 9`, "undefined_word@1:20", true},
		{`def k 5 end def f fn [[][Integer][undef k 1]] end f k`, "undefined_word@1:53", true},
		{`def k 5 end def f fn [[][Integer][if true [undef k] [] k]] end f`, "undefined_word@1:56", true},
		{`def k 5 end for 2 [ if (k eq 5) [undef k] [] ] 9`, "undefined_word@1:25", true},
		{`def k [1 2] end if true [undef k] [] k`, "undefined_word@1:38", true},
		{`import module [def m fn [[] [Integer] [1]] export "M" {m:m/v}] end def k 5 end if true [M.m drop undef k 1] [1] k`, "undefined_word@1:113", true},
		// An effect before the undef: a defer's re-run would be fenced into
		// an internal error; the miss raises the word instead.
		{`def k 5 end if true [(print "x") undef k] [] k`, "undefined_word@1:46", true},
		{`def k 5 end for 2 [ (print "x") undef k ] k`, "undefined_word@1:43", true},
		{`def k 5 end if true [undef k] [] (print "z") k`, "undefined_word@1:46", true},
		// A root def after the region re-binds the name: the twin replays it,
		// and the read after it bakes the new binding.
		{`def k 5 end if true [undef k] [] def k 6 end k`, "[6]", false},
		{`def k 5 end for 2 [ undef k ] def k 7 end k`, "[7]", false},
		// Two undefs of one binding: the second pops nothing on either lane.
		{`def k 5 end if true [undef k undef k] [] 1`, "[1]", false},
		// A read BEFORE the region keeps its bake, routed or const.
		{`def k 5 end k for 2 [ undef k ] 9`, "[5 9]", false},
		{`def k 5 end def go fn [[][Integer][add k 1]] end go if true [undef k] [] 9`, "[6 9]", false},
		// Every read is seated AT ITS TOKEN (review of #464): the lookup, and
		// the undefined_word it raises, executes before a later effect and
		// before the value is consumed — at the first k, with nothing printed;
		// two reads in one statement each carry their own caret; a read the
		// undef follows in the same loop body raises on the second pass.
		{`def k 5 end if true [undef k] [] k (print "x") k`, "undefined_word@1:34", true},
		{`def k 5 end if true [undef k] [] k k add`, "undefined_word@1:34", true},
		{`def k 5 end for 2 [ k undef k ] 9`, "undefined_word@1:21", true},
		{`def k 5 end if false [undef k] [] k k add`, "[10]", true},
		{`def k 5 end if false [undef k] [] k drop 9`, "[9]", true},
		// A dynamic code body's read after the region (review of #464): the
		// root def is installed ONCE — its twin's replay — so the placed undef
		// pops the binding the body then misses, and `do` catches the raise on
		// both lanes.
		{`def k 5 end if true [undef k] [] do [k]`, "", false},
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
		if dis := prog.Disassemble(); !strings.Contains(dis, "UNDEF_DYN_SCOPE") || strings.Contains(dis, "LOOKUP_DYN_SCOPE") != c.live {
			t.Errorf("%q: the undef is placed and the read after it is live:\n%s", c.src, dis)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		// The first line — code, detail, position — is the parity; the
		// did-you-mean line below it is NUR146's: the compiled raise
		// suggests over the registry, the interpreter's over a registry
		// that holds the frame's loop iterator as a def binding too.
		requireParityHead(t, c.src, gotC, errC, gotI, errI)
		if strings.HasPrefix(c.want, "undefined_word@") {
			var ce, ie *core.BoruError
			if !errors.As(errC, &ce) || !errors.As(errI, &ie) || ce.Code != "undefined_word" || ie.Code != "undefined_word" {
				t.Errorf("%q: both lanes raise undefined_word: compiled=%v interp=%v", c.src, errC, errI)
				continue
			}
			at := fmt.Sprintf("undefined_word@%d:%d", ce.Row, ce.Col)
			if at != c.want || ce.Row != ie.Row || ce.Col != ie.Col {
				t.Errorf("%q: the compiled raise sits at the read token: compiled %d:%d interp %d:%d want %s", c.src, ce.Row, ce.Col, ie.Row, ie.Col, c.want)
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
	// What still refuses, each through the undef site, and answers as the
	// interpreter does under the hatch: a recording the placement cannot
	// seat (an each body's first run, a `do` inside a loop, the never-running
	// error handler the leniency exists for), a binding the model declines
	// to generalise (a fn-family value, a type, a param, a fn-local), a name
	// a loop carries (its reads are slot reads — the loop's joined twin
	// replays one install for N iterations, so `for 2 [ def k 6 ] undef k k`
	// answered the pre-loop 5 for the interpreter's 6 on main), a `def` of
	// the name inside the region that undefs it, and a FORWARD-slot read of
	// the popped name (the interpreter
	// collects the unbound word as a Word value and raises the no-match at
	// the dispatching word — the routed dispatch's arm, not the lookup's).
	refused := []struct{ src, reason string }{
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
		{`def k 5 end if true [undef k] [] add k 1`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def go fn [[][Integer][add k 1]] end for 2 [ go  undef k ]`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def go fn [[][Integer][sub 1 k]] end if true [undef k] [] go`, "forward-slot read of `k` after a placed undef"},
		// The same slot at a USER call and at a POLY native dispatch (a `get`
		// over a gradual operand re-matches at run time): each record refuses it.
		{`def k 5 end def g fn [[x:Integer][Integer][x add 1]] end if true [undef k] [] g k`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def f fn [[x:Any][Any][add x k]] end if true [undef k] [] f 1`, "forward-slot read of `k` after a placed undef"},
		{`def k 5 end def f fn [[x:Any][Any][x get k]] end if true [undef k] [] f {k:1}`, "forward-slot read of `k` after a placed undef"},
	}
	for _, c := range refused {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: %v", c.src, cerr)
		}
		if prog != nil || !strings.Contains(reason, c.reason) {
			t.Errorf("%q: want the refusal %q…, got compiled=%v reason=%q", c.src, c.reason, prog != nil, reason)
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
		if !ran && src != `k` {
			t.Errorf("%q: compiles on the long-lived registry: %v", src, errC)
		}
		requireParityHead(t, src, gotC, errC, gotI, errI)
		if src == `k` && errI == nil && (errC != nil || fmt.Sprint(gotC) != "[5]") {
			t.Errorf("%q: the base binding stands after the request: %v %v", src, gotC, errC)
		}
	}
}
