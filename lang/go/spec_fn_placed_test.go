package lang

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The seventieth increment: a fn def inside a runtime-conditional body at
// module scope is SPECULATIVE. Its install is placed at its site
// (OpBindResident through the interpreter's own installer, so an
// overlapping redefinition — family L — drops the standing overload exactly
// as the arm's run does), its dispatches route with a live lead
// (DISPATCH_GENERIC, at root too, with or without a forward slot) and run
// the live signature's own unit, and a miss raises the interpreter's
// undefined_word at the word. Measured on main before the increment: the
// arm's def replayed as a root twin BEFORE the branch and the dispatch was
// a committed call, so `def m {e: false}  if (m "e" get) [def f fn […]] []
// f 1` answered the arm's 101 where the interpreter raises.
func TestConditionalFnDefIsSpeculative(t *testing.T) {
	const arm = `[def f fn [[x:Integer][Integer][x add 100]] end]`
	const outer = `def f fn [[x:Integer][Integer][x add 1]] end `
	compiled := []struct{ src, want string }{
		// A fresh def: bound exactly when the arm ran.
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, "undefined_word@1:89"},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`, "[101]"},
		// The stack form (no window: a slot-less descriptor), an effect before.
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] 1 f`, "undefined_word@1:91"},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] (print "x") 1 f`, "[101]"},
		// An overlapping redefinition (family L): the outer overload or the
		// shadow, each running its own unit.
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, "[2]"},
		{outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`, "[101]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] (print "x") 1 f`, "[2]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [def f fn [[x:Integer][Integer][x add 200]] end] f 1`, "[201]"},
		// An undef or a root redefinition after the arm, on either path.
		{outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] undef f 9`, "[9]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] undef f 9`, "[9]"},
		{outer + `def m {e: false} end if (m "e" get) ` + arm + ` [] def f fn [[x:Integer][Integer][x add 7]] end f 1`, "[8]"},
		{outer + `def m {e: true} end if (m "e" get) ` + arm + ` [] def f fn [[x:Integer][Integer][x add 7]] end f 1`, "[8]"},
		// Dispatched from a unit, a nested arm, a paren group, an each body.
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] def g fn [[][Integer][f 1]] end g`, "[101]"},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] def g fn [[][Integer][f 1]] end g`, "undefined_word@1:111"},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] if true [f 1] [0]`, "[101]"},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] if true [f 1] [0]`, "undefined_word@1:98"},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] (f 1) add 1`, "[102]"},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] (f 1)`, "undefined_word@1:90"},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] [1 2] each [f]`, ""},
		// A window the host cannot drive — a group, a list literal among the
		// forward tokens — takes the slot-less descriptor the stack form
		// takes: the operands are compiled and pushed as the claim (the
		// corpus's finding on #466, third half: sift's `cloop (opts get
		// "cols") 0 []`).
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] f (1 add 1)`, "[102]"},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] f (1 add 1)`, "undefined_word@1:89"},
		{`def m {e: true} end if (m "e" get) [def h fn [[xs:List n:Integer][Integer][(size xs) add n]] end] [] h [1 2] (1 add 1)`, "[4]"},
		{`def m {e: false} end if (m "e" get) [def h fn [[xs:List n:Integer][Integer][(size xs) add n]] end] [] h [1 2] (1 add 1)`, "undefined_word@1:103"},
		// A gradual operand's value is matched live, as the stack form's is.
		{`def id fn [[x:Any][Any][x]] end def m {e: true} end if (m "e" get) ` + arm + ` [] f (id 5)`, "[105]"},
		{`def id fn [[x:Any][Any][x]] end def m {e: false} end if (m "e" get) ` + arm + ` [] f (id 5)`, "undefined_word@1:121"},
		// Inside a fn body the fresh def is a FRAME binding: the placed
		// install is OpBindDynScope, unwound at the unit's RET as the
		// interpreter's teardown pops it, and the routed lead — frame-local,
		// registry-visible — resolves in the frame that holds it.
		{`def m {e: false} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g`, "undefined_word@1:111"},
		{`def m {e: true} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g`, "[101]"},
		{`def m {e: true} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g g`, "[101 101]"},
		{`def m {e: false} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] (print "x") f 1]] end g`, "undefined_word@1:123"},
		// Review of #466: an EMPTY body's unit is located too (the identity
		// is the signature's declaration site); the replaced outer is an
		// exported module fn, compiled in ITS module's registry (k is the
		// module's 10, not main's 50) with its contract; an in-arm undef of
		// the family is placed with the install.
		{`def m {e: true} end if (m "e" get) [def f fn [[][][]] end] [] (print "x") f`, ""},
		{`import module [def k 10 end def q fn [[x:Integer][Integer][x add k]] end export "A" {q:q/v}] end def k 50 end def f A.q/v end def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, "[11]"},
		{`import module [def k 10 end def q fn [[x:Integer][Integer][x add k]] end export "A" {q:q/v}] end def k 50 end def f A.q/v end def m {e: true} end if (m "e" get) ` + arm + ` [] f 1`, "[101]"},
		{`def m {e: true} end if (m "e" get) [def f fn [[x:Integer][Integer][x add 100]] end undef f 1] [] 9`, "[1 9]"},
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
		// The arm's install is placed, the dispatch routes, and the only
		// root twins of f are the OUTER defs' before the branch (none for a
		// fresh family).
		twins := 0
		if i := strings.Index(c.src, " if "); i >= 0 {
			twins = strings.Count(c.src[:i], "def f ")
		}
		dispatched := !strings.Contains(c.src, "undef f") // the undef rows dispatch nothing
		install := "BIND_RESIDENT"                        // at root; inside a unit the frame-scoped install
		if strings.Contains(c.src, "def g fn [[][Integer][if") {
			install = "BIND_DYN_SCOPE"
		}
		if dis := prog.Disassemble(); !strings.Contains(dis, install) || strings.Contains(dis, "DISPATCH_GENERIC") != dispatched || strings.Count(dis, "bind twin def f") != twins {
			t.Errorf("%q: the arm's install is placed, the dispatch routes, %d root twin(s) of f:\n%s", c.src, twins, dis)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
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
	// What refuses, through the undef site, and answers as the interpreter
	// does under the hatch: a def the recorder cannot place (a loop body —
	// it re-rounds and carries its defs by slot; a fn body's REPLACE — the
	// interpreter's drop-then-push leaves the frame's depth unchanged, so
	// its replacement outlives the call, which no frame-scoped install
	// reproduces; an each body and a `do` body — the resident bridge's and
	// the closure compile's), a dispatch the op cannot drive (the user-poly
	// and rematch seats), and a `/v` read of the value. A CAPTURING closure's conditional def is not a const the
	// placement can bake: it keeps the closure machinery and family L's own
	// refusal. A fresh def in BOTH arms keeps the join's older fn-carrier
	// refusal.
	refused := []struct{ src, reason string }{
		{outer + `for 2 ` + arm + ` f 1`, "fn 'f' redefined inside a conditional body"},
		{outer + `def m {e: true} end while [m "e" get] [def f fn [[x:Integer][Integer][x add 100]] end def m {e: false} end] f 1`, "fn 'f' redefined inside a conditional body"},
		{outer + `def m {e: true} end for 2 [if (m "e" get) ` + arm + ` []] f 1`, "fn 'f' redefined inside a conditional body"},
		{`def m {e: false} end for 2 [if (m "e" get) ` + arm + ` []] 9`, "fn `f` defined inside a conditional body where the compiled program cannot place"},
		{outer + `def m {e: false} end def g fn [[][Integer][if (m "e" get) ` + arm + ` [] f 1]] end g`, "fn 'f' redefined inside a conditional body"},
		{`def kk k:Integer => [z:Integer => [add k z]] end def p (kk 7) end if true [def p (kk 8)] 3 p/v apply`, "fn 'p' redefined inside a conditional body"},
		// A LAMBDA declares no output signature, so it carries no
		// declaration site for the routed op to locate its unit by: as the
		// outer and as the placed value, the placement refuses.
		{`def f (x:Integer => [x add 1]) end def m {e: false} end if (m "e" get) ` + arm + ` [] f 1`, "fn 'f' redefined inside a conditional body"},
		{`def m {e: true} end if (m "e" get) [def f (x:Integer => [x add 100]) end] [] f 1`, "fn `f` defined inside a conditional body where the compiled program cannot place"},
		{`def m {e: false} end [1 2] each [if (m "e" get) ` + arm + ` []] f 1`, "fn `f` defined inside a conditional body where the compiled program cannot place"},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [] f/v`, "value read of the conditionally-defined fn `f`"},
		// An undrivable window keeps one refusal: a slot naming a value a
		// placed speculative undef generalised owes a live lookup the
		// compiled operand baked.
		{`def k 5 end def m {e: false} end if (m "e" get) [undef k] [] if (m "e" get) [def f fn [[x:Integer y:Integer][Integer][x add y]] end] [] f (1 add 1) k`, "forward-slot read of `k` after a placed undef"},
		{`def m {e: true} end if (m "e" get) ` + arm + ` [] 1 f/v apply`, "value read of the conditionally-defined fn `f`"},
		{`def m {e: false} end if (m "e" get) ` + arm + ` [def f fn [[x:Integer][Integer][x add 200]] end] f 1`, "def-bound computed fn apply"},
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
}

// The binding is placed on a long-lived registry (the check pass's join
// pushes the model's fn for the request; the run restores the base before
// it runs, as a placed undef's does), so a request after a not-taken arm
// finds the name unbound, and one after a taken arm finds the arm's fn.
func TestConditionalFnDefAcrossRequests(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	const arm = `if (m "e" get) [def f fn [[x:Integer][Integer][x add 100]] end] [] 1`
	// The last three: an in-arm undef of the family is placed with its
	// install (review of #466), so the taken arm leaves nothing behind.
	const armUndef = `if (m "e" get) [def f fn [[x:Integer][Integer][x add 100]] end undef f 1] [] 2`
	for _, src := range []string{`def m {e: false}`, arm, `f 1`, `def m {e: true}`, arm, `f 1`, `undef f 2`, `f 1`, armUndef, `f 1`} {
		gotC, ran, errC := a.RunCompiled(src)
		gotI, errI := b.RunInterp(src)
		if noteCompileDefect(t, src, gotC, errC) {
			continue
		}
		if !ran && errI == nil {
			t.Errorf("%q: compiles on the long-lived registry: %v", src, errC)
		}
		requireParityHead(t, src, gotC, errC, gotI, errI)
	}
}
