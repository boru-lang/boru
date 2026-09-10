package lang

// A RETURNED CLOSURE IS PARKED (measured 2026-09-05). A user fn's returned
// closure is placed data where it lands; only a paren rewind over two or
// more survivors, or a read that dispatches (a bare name, a member read),
// turns it into a call (design/PAREN-RESTEP-RULE.0.md). The compiled lane
// applied every residual lead a paren had not placed, so `mk 7` compiled to
// 8 against the interpreter's `fn (Integer) 7` — on the DEFAULT lane, exit
// 0, and older than every stage on this branch. callResultPlaced
// (compiler/go/emit.go) is the fix; these rows pin the rule on both lanes,
// including the shapes that must still apply (a paren, a code body's frame,
// a member read, a multi-output user call) so the fix cannot over-reach.

import (
	"fmt"
	"strings"
	"testing"
)

const mk1 = `def mk fn [[] [Function] [([y:Integer] => [y add 1])]]`

func TestReturnedClosureParkParity(t *testing.T) {
	rows := []struct{ src, note string }{
		// parked: a bare user call's result at the program residual
		{mk1 + `  mk 7`, "fn (Integer) 7 — was 8"},
		{mk1 + `  mk 7 8`, "fn (Integer) 7 8 — was 8 8"},
		{mk1 + `  mk 7 add 1`, "fn (Integer) 8 — was 9"},
		{mk1 + `  mk`, "fn (Integer)"},
		{`def mk2 fn [[x:Integer] [Function] [([y:Integer] => [y add x])]]  mk2 1 7`, "fn (Integer) 7 — was 8"},
		{`def mk2 fn [[x:Integer] [Function] [([y:Integer] => [y add x])]]  (mk2 1) 7`, "paren-placed, one survivor"},
		// parked: the arrival apply of a USER member returning a closure
		{`def mk fn [[x:Any] [Any] [([y:Integer] => [y add 1])]]  def m {p: mk/v}  m.p 5 7`, "an Any-typed return of the arrival apply: parked — was 8"},
		// parked: a Go-impl fn value returned by a user fn
		{`import "boru:math-util" def f fn [[] [Any] [MathUtil.sqrt/v]]  f 16.0`, "fn sqrt(Number) 16.0 — was 4.0"},
		// family C: two dynamic results live at once are two placed values
		{`def f fn [[x:Any] [Any] [x]] def m {p: f/v} m.p 5 m.p 7`, "5 7 — was refused"},
		{`def f fn [[x:Any] [Any] [x]] def m {p: f/v} m.p 5 7`, "5 7"},
		// a def-bound factory result applies by its BINDING's read, and its
		// arity is provable off the baked lambda's own signature (the
		// eleventh increment) — the paren and the bare read alike
		{mk1 + `  def h (mk) h 7`, "8 — was refused, the closure shape unknown"},
		{mk1 + `  def h (mk) (h 7)`, "8"},
		// still applies: a paren rewind over two survivors
		{mk1 + `  (mk 7)`, "8"},
		{mk1 + `  ((mk) 7)`, "8"},
		{`def mk2 fn [[x:Integer] [Function] [([y:Integer] => [y add x])]]  ((mk2 1) 7)`, "8"},
		// still applies: a code body's frame rewinds, and a native word's
		// returned fn auto-applies to what follows
		{mk1 + `  do [mk 7]`, "8"},
		{mk1 + `  do [mk] 7`, "8"},
		{mk1 + `  do [mk 7] 8`, "8 8"},
		{`def mk fn [[a:Integer] [Function] [(fn [[b:Integer] [Integer] [a add b]])]] do [(mk 1) 2]`, "3"},
		// still applies: a member read dispatches
		{`def c class {op: (fn [[x:Integer] [Integer] [x add 1]])} (make c {}).op 5`, "6"},
		// a list literal never rewinds
		{mk1 + `  [mk 7]`, "[fn (Integer) 7]"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// Inside a fn body the park is the same, and the interpreter's return-count
// check runs BEFORE its frame would rewind: `[mk 7]` under one declared
// return is a two-value residual and a type_error on both lanes. The
// compiled lane used to replay the frame (OpCallDynFrame) and answer 8. The
// error's position differs by the NUR118 blame rule (the definition on the
// compiled lane, the call on the interpreter), so the pin is code and
// message, not position.
func TestReturnedClosureParkInFnBodyRaisesTheCountError(t *testing.T) {
	for _, src := range []string{
		mk1 + `  def g fn [[] [Any] [mk 7]] g`,
		mk1 + `  def g fn [[] [Any] [mk 7 add 1]] g`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: must compile natively (the count mismatch compiles to the RET check); err=%v", src, errC)
			continue
		}
		if errC == nil || errI == nil {
			t.Errorf("%q: both lanes must raise; compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
			continue
		}
		first := func(err error) string { return strings.SplitN(err.Error(), "\n", 2)[0] }
		if first(errC) != first(errI) {
			t.Errorf("%q: error text differs:\n  compiled: %s\n  interp:   %s", src, first(errC), first(errI))
		}
	}
}

// The shapes the park deliberately leaves to the interpreter, with their
// reasons: a MULTI-output user call's leading closure was re-stepped inside
// its own frame (the check model did not), and a def-read of a computed
// closure is a bare-name dispatch whose closure shape the compiler cannot
// recover. Both answer through the interpreter.
func TestReturnedClosureParkSoundFallbacks(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	rows := []struct{ src, reason string }{
		{mk1 + `  def g fn [[] [Any Any] [mk 7]] g`, "dynamic value precedes residual args (fn-value-call boundary)"},
		// A user fn returning a fn it was HANDED: the interpreter renders the
		// value under the PARAM's name (`fn g(Integer)`), which the raw
		// runtime value cannot reproduce — the residual's render gate holds
		// (callResultRenderKnown; NUR119 records the paren-placed and
		// Any-typed spellings that pre-date it).
		{`def app fn [[g:Function][Function][g/v]]  app (z:Integer => [mul 3 z])`, "unconsumed fn-value carrier in residual (closure render)"},
		{`def app fn [[g:Function][Function][g/v]]  app (z:Integer => [mul 3 z]) 7`, "unconsumed fn-value carrier in residual (closure render)"},
		{`def sq (z:Integer => [mul z z])  def app fn [[g:Function][Function][g/v]]  app sq/v`, "unconsumed fn-value carrier in residual (closure render)"},
		// The arrival apply of a user member returning a FUNCTION (`m.p 5 7`
		// over `{p: mk/v}`): parked, and its render is unknowable — the
		// member is applied through the island, not a compiled unit — so
		// the gate holds. It compiled to 8 before the park.
		{`def mk fn [[x:Any] [Function] [([y:Integer] => [y add 1])]]  def m {p: mk/v}  m.p 5 7`, "unconsumed fn-value carrier in residual (closure render)"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — this shape has graduated; move it to the parity rows", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal drifted: want %q in %q", c.src, c.reason, reason)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if compiled {
			t.Errorf("%q: expected the interpreter fallback", c.src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: engine divergence on the fallback: compiled=%v/%v interp=%v/%v", c.src, gotC, errC, gotI, errI)
		}
	}
}

// TestApplyWordClaimsParkedResult pins NUR124's discriminator (measured
// 2026-09-07, the twenty-second increment).
//
// `5 (mk 3)` answered 15 compiled where the park rule leaves
// `[5 fn (Integer)]` — a silent wrong value on the DEFAULT lane, exit 0.
// Every sibling arm of resolveDynamicApply already consulted the park rule;
// trailingApply checked only the shape.
//
// It stayed open for two increments because the sound fix refused a corpus
// row: `10 (mk2 5) apply` parks identically and then APPLIES, because the
// trailing word dispatches the parked value on purpose — and both lower to
// the same OpCallDynamicTrailing, so the residual cannot separate them. The
// unit-scoped pendingApply cannot either: it returns false outright when no
// fn unit is open, which is exactly the program residual's case.
//
// What can: `apply` records through RecordCall as an IDENTITY (args[0].ID ==
// outs[0].ID — the check engine returns the fn concrete and re-steps it), and
// that ID is the one trailingApply meets. EmitState.appliedByWord marks it
// program-wide.
func TestApplyWordClaimsParkedResult(t *testing.T) {
	const mk = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  `
	const mk2 = `def mk2 fn [[x:Integer] [Function] [([x:Integer] => [x add 1])]]  `

	// The apply word claims the parked value: still compiles, still applies.
	src := mk2 + `10 (mk2 5) apply`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Error("`… apply` must keep compiling — the corpus row the previous attempt refused")
	}
	requireParity(t, src, gotC, errC, gotI, errI)
	if fmt.Sprint(gotI) != "[11]" {
		t.Errorf("the apply word applies the parked result: %v", gotI)
	}

	// Nothing claims it: the arm declines rather than applies. A refusal is
	// the sound fallback — the default lane then answers on the interpreter.
	// The forty-third increment's residual rebuild does NOT take this shape:
	// its callable screen stands aside for a residual that may hold a
	// Function, because a re-push is a data push where the interpreter
	// re-steps (NUR131).
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(mk + `5 (mk 3)`)
	if cerr != nil {
		t.Fatalf("check: %v", cerr)
	}
	if prog != nil {
		t.Error("an unclaimed parked result must not compile to an apply")
	}
	if !strings.Contains(reason, "call result above a literal") {
		t.Errorf("refusal = %q, want the existing residual-shape site", reason)
	}
	// And the value both lanes agree on is the PARKED pair.
	d, _ := New()
	gotI2, errI2 := d.RunInterp(mk + `5 (mk 3)`)
	if errI2 != nil || fmt.Sprint(gotI2) != "[5 fn (Integer)]" {
		t.Errorf("the park rule leaves both values: %v/%v", gotI2, errI2)
	}

	// Shapes the discriminator must leave alone, all previously passing.
	for _, s := range []string{mk + `(mk 3) 5`, mk + `5 (mk 3) 7`, mk + `1 2 (mk 3)`} {
		gc, ok, ec, gi, ei := runBothEngines(t, s)
		if !ok {
			t.Logf("%q: not compiled (refusal, not a divergence)", s)
			continue
		}
		requireParity(t, s, gc, ec, gi, ei)
	}
}

// TestShuffleRestepTimingDeclines records NUR124's remaining half, measured
// 2026-09-07 and pre-existing. It is TWO defects, not the one the record
// described, and two of the obvious rows agree for the wrong reason.
//
// AXIS 1 — timing, and not about closures at all. A plain FnDefInfo diverges
// as soon as anything follows the shuffle:
//
//	[g/v] each [5 swap drop]
//	  interpreted  each_error: body produced no result
//	  compiled     [5]
//
// `[g] 5 -> [g,5] swap -> [5,g]`: the interpreter applies g THERE, giving
// [15], and drop empties the stack. The compiled body leaves g, drop removes
// it, body ends [5]. So the interpreter re-steps AT THE SHUFFLE and the
// compiled body at BODY END, if at all.
//
// AXIS 2 — a compiled closure is not re-stepped even at body end:
// `[(mk 3)] each [5 swap]` answers [fn (Integer)] where the FnDefInfo twin
// answers [15], same body, only the payload differing.
//
// The TRAP rows are pinned here deliberately. `[5 swap]` and `[5 over]` over
// an FnDefInfo PASS, only because nothing follows the shuffle so the two
// timings coincide. A family built from them would report this fixed.
func TestShuffleRestepTimingDeclines(t *testing.T) {
	const g = `def g fn [[x:Integer][Integer][x mul 3]]  `
	const mkc = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  `

	// The trap rows: they agree, and must keep agreeing — but agreeing here
	// is not evidence the timing is right.
	for _, src := range []string{g + `[g/v] each [5 swap]`, g + `[g/v] each [5 over]`, mkc + `[(mk 3)] each [dup drop 5]`} {
		gc, ok, ec, gi, ei := runBothEngines(t, src)
		if !ok {
			t.Logf("%q: not compiled", src)
			continue
		}
		requireParity(t, src, gc, ec, gi, ei)
	}

	// Axis 1: an FnDefInfo, with a token AFTER the shuffle. This is the row
	// that separates shuffle-time from body-end re-stepping — FIXED (the
	// twenty-fourth increment, restep_deopt_test.go): the compiled body
	// re-steps swap's results through a RE-STEP deopt, so g applies at the
	// shuffle and drop empties the body on both lanes.
	gotC, ok, errC, gotI, errI := runBothEngines(t, g+`[g/v] each [5 swap drop]`)
	if !ok {
		t.Fatal("the timing row must still compile — a refusal would hide the defect")
	}
	if errI == nil || !strings.Contains(errI.Error(), "body produced no result") {
		t.Errorf("the interpreter applies AT the shuffle, so drop empties the body: %v/%v", gotI, errI)
	}
	requireParity(t, g+`[g/v] each [5 swap drop]`, gotC, errC, gotI, errI)

	// Axis 2: same body, payload the only difference — FIXED (the
	// twenty-fifth increment, TestClosureValueReStepParity): the island's
	// sub-engine bridges the ClosurePayload to a dispatchable fn, so both
	// lanes apply the shuffled closure. Asserted by VALUE on both sides.
	gotC2, ok2, errC2, gotI2, errI2 := runBothEngines(t, mkc+`[(mk 3)] each [5 swap]`)
	if !ok2 {
		t.Fatal("the closure row must still compile")
	}
	if errI2 != nil || fmt.Sprint(gotI2) != "[[15]]" {
		t.Errorf("the interpreter applies the shuffled closure: %v/%v", gotI2, errI2)
	}
	if errC2 != nil || fmt.Sprint(gotC2) != "[[15]]" {
		t.Errorf("the compiled lane applies it too: %v/%v", gotC2, errC2)
	}
}
