package lang

import (
	"fmt"
	"strings"
	"testing"
)

// closure_capture_test.go pins the twenty-sixth increment: the closure-capture
// family's three measured blockers (the handoff's frontier section) and the
// frame-binding rename that one of them needed.
//
// A returned lambda over a captured `g:Function` compiled only when its body
// was the paren-apply over typed params, `[(g x)]`. The bare-name apply
// `[g x]` and the `Any`-typed param `[[x:Any][Any][(g x)]]` both refused with
// the closure COUNT check — the residual [g, x] against one declared return —
// which fired before the whole-frame replay a fn unit reaches. A lambda
// VALUE unit is a fn in every way that matters at its finish (its count
// contract is enforced at invoke, a bare read of a captured fn is the word
// dispatch NUR123's replay seats), so it now takes the fn path
// (fnUnitRec.plainLambda) unless a param carries a value PATTERN, which the
// closure apply ops do not enforce. The replay's applicable count is
// names-aware for the count-mismatch path as it already was for the
// count-match one, so a gradual `x` beside `g` no longer competes for the
// apply. The `/v` read `[g/v]` returns the captured fn as data; that needed
// the VM to NAME a fn bound for a named param as the interpreter's frame
// binding does (nameFrameFns), or `(h 5)` would have rendered `fn (Integer)`
// for the interpreter's `fn g(Integer)` — a divergence the paren shape
// `(f (z:Integer => [z])) 3` already carried on the default lane.

const ccApp = `def app fn [[g:Function][Function][( fn `

// TestClosureCaptureParity pins the shapes that now agree on both lanes and
// COMPILE, positives and no-match negatives alike.
func TestClosureCaptureParity(t *testing.T) {
	rows := []struct{ src, note string }{
		// (a) the bare-name apply of a captured Function with a forward arg
		{ccApp + `[[x:Integer][Integer][g x]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)`, "15 — was refused (count check)"},
		{ccApp + `[[x:Integer][Integer][g x]] )]]  def h (app (z:String => [z]))  (h 5)`, "cannot call `g` on both lanes"},
		{ccApp + `[[x:Integer][Integer][g]] )]]  def h (app ([] => [42]))  (h 5)`, "42 — a 0-arg capture fires as the word"},
		// (c) an Any-typed inner param beside the captured fn
		{ccApp + `[[x:Any][Any][(g x)]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)`, "15 — was refused (count check)"},
		{ccApp + `[[x:Any][Any][(g x)]] )]]  def h (app (z:Integer => [mul 3 z]))  (h "s")`, "cannot call `g` — the gradual x holds a String"},
		{ccApp + `[[x:Any][Any][(g x)]] )]]  def h (app (z:String => [z]))  (h 5)`, "cannot call `g`"},
		// the control that always compiled
		{ccApp + `[[x:Integer][Integer][(g x)]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)`, "15"},
		// (b) the `/v` read returns the captured fn as DATA, named by its frame
		{ccApp + `[[x:Integer][Function][g/v]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5)`, "fn g(Integer) — the frame's name on both lanes"},
		// the rename on a plain fn body: the paren shape that diverged before
		{`def f fn [[g:Function][Function][g/v]]  (f (z:Integer => [z])) 3`, "fn g(Integer) 3 — was fn (Integer) 3"},
		{`def g fn [[x:Integer][Integer][x mul 3]]  def f fn [[h:Function][Function][h/v]]  (f g/v) 3`, "fn h(Integer) 3 — a named fn takes the param's name"},
		{`def app3 fn [[g:Function][Integer][(g 5)]]  app3 (z:String => [z])`, "the seated head name and the value's name agree"},
		{`import "boru:math-util"  def f fn [[g:Function][Any][(g 16.0)]]  f MathUtil.sqrt/v`, "4.0 — a module wrapper still dispatches"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestClosureCaptureSoundRefusals pins the neighbours that REFUSE (the default
// lane then answers on the interpreter), never a wrong value: the returned
// `/v` fn applied downstream, a gradual arg that turns out to be a fn (the
// interpreter's strict-barrier error), and the pattern-param lambda whose
// count refusal guards an apply the closure ops cannot check, and the
// gradual EVENT argument the replay cannot order. The two windows the
// replay's one-lead rule must keep declining — a second fn-typed read
// (`f (g x y)`) and a re-read across a bind (`g x def q 9 g q`) — are
// pinned where they always were, bytecode_chained_apply_test.go: the first
// cut of the names-aware count admitted both, and `f (g x y)` raised
// `cannot call `f“ compiled for the interpreter's 14.
func TestClosureCaptureSoundRefusals(t *testing.T) {
	rows := []string{
		ccApp + `[[x:Integer][Function][g/v]] )]]  def h (app (z:Integer => [mul 3 z]))  ((h 5) 2)`,
		ccApp + `[[x:Integer][Function][g/v]] )]]  def h (app (z:Integer => [mul 3 z]))  (h 5) 2`,
		ccApp + `[[x:Integer][Function][g/v]] )]]  def h (app (z:Integer => [mul 3 z]))  def q (h 5)  q 2`,
		ccApp + `[[x:Any][Any][(g x)]] )]]  def h (app (z:Any => [z]))  (h ([] => [42]))`,
		// a gradual EVENT result as the argument: not a word read, so it
		// competes for the apply (replayLeadApplicables) and the unit declines
		ccApp + `[[x:Any][Any][(g (x get "k"))]] )]]  def h (app (z:Integer => [mul 3 z]))  (h {k: 5})`,
		`def mk fn [[x:Integer][Function][(fn [[0][Integer][x]])]] ((mk 5) 1)`,
	}
	for _, src := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a sound refusal", src)
			continue
		}
		if reason == "" {
			t.Errorf("%q: refused without a reason", src)
		}
	}
}

// TestClosureCaptureOpenShapes pins, as MEASURED, what the rename leaves
// open: a MODULE WRAPPER bound for a named param keeps its own name and
// signatures on the compiled lane (installDef's rebinding path binds the
// inner native's overloads under the param's name, which the VM does not
// mirror), so its render differs.
func TestClosureCaptureOpenShapes(t *testing.T) {
	const src = `import "boru:math-util"  def f fn [[g:Function][Function][g/v]]  (f MathUtil.sqrt/v) 16.0`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || errC != nil || errI != nil {
		t.Fatalf("compiled=%v errC=%v errI=%v", compiled, errC, errI)
	}
	if c, i := strings.TrimSpace(fmt.Sprint(gotC)), strings.TrimSpace(fmt.Sprint(gotI)); !strings.Contains(c, "fn sqrt(Number) 16") || !strings.Contains(i, "fn g(") {
		t.Errorf("measured, open: compiled %s / interpreted %s", c, i)
	}
}
