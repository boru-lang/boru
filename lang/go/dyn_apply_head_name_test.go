package lang

import (
	"fmt"
	"strings"
	"testing"
)

// dyn_apply_head_name_test.go pins the trailing fn-value apply's DIAGNOSTIC
// (CompiledFn.DynApplyName, measured 2026-09-06).
//
// The interpreter reads the head of `(g 5)` as a WORD — stepWord routes a
// bound FnDefInfo through Registry.Lookup — so a no-match names the binding,
// anchors the caret at the read, and explains the failing overloads from the
// fn it found:
//
//	def app3 fn [[g:Function][Integer][(g 5)]]
//	app3 (z:String => [z])
//	  interpreted  cannot call `g` … --> 1:37 … candidate `g (String)` … help: …
//	  compiled     cannot call ``   … --> source position unknown
//
// The compiled apply holds a frame SLOT, which carries neither name nor
// position, so both had to ride in the bytecode. Both lowering routes seat
// the pair — the body-tail apply (`[(g 5)]`, emit.go) and the event apply
// (`[1 add (g 5)]`, lower.go) — and the VM builds the diagnostic through
// NoMatchDiag with the applied fn itself, since Registry.Lookup finds no
// frame binding on this lane.
//
// The `apply` WORD's flavour (OpCallDynApplyTop) is deliberately NOT seated:
// applyHandler re-steps the fn against the whole preceding stack, a different
// dispatch whose own divergence is recorded separately (NUR123).

// TestDynApplyHeadNameParity pins the shapes that now agree BYTE FOR BYTE —
// message, caret, notes and suggestion alike.
func TestDynApplyHeadNameParity(t *testing.T) {
	rows := []struct{ src, note string }{
		// the body-tail route: the apply IS the whole residual.
		{`def app3 fn [[g:Function][Integer][(g 5)]]  app3 (z:String => [z])`, "was `cannot call ``' with no position"},
		{`def app5 fn [[g:Function y:Integer][Integer][(g 5)]]  app5 (z:String => [z]) 7`, "a spare param below the apply"},
		{`def appc fn [[g:Function][Integer][(g (1 add 1))]]  appc (z:String => [z])`, "a paren-computed argument is written too"},
		// the event route: the apply's result is consumed, so it seats as
		// an operand rather than the body tail.
		{`def app7 fn [[g:Function][Integer][1 add (g 5)]]  app7 (z:String => [z])`, "consumed by a native word"},
		{`def app8 fn [[g:Function][Integer][def t (g 5)  t]]  app8 (z:String => [z])`, "consumed by a value-def"},
		// a head that MATCHES still applies: the table is diagnostic-only.
		{`def appok fn [[g:Function][Integer][(g 5)]]  appok (z:Integer => [z mul 2])`, "10 — no diagnostic at all"},
		// a capture read bare inside a returned lambda's body reaches the
		// same op through the closure unit.
		{`def mk fn [[g:Function][Function][( fn [[x:Integer][Any][(g 5)]] )]]  def q (mk (z:String => [z]))  (q 7)`, "the head is a CAPTURE slot"},
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

// TestDynApplyHeadNameDiagnostic pins the four parts the slot could not
// supply, so a regression that merely stops diverging (both lanes nameless)
// still fails.
func TestDynApplyHeadNameDiagnostic(t *testing.T) {
	const src = `def app3 fn [[g:Function][Integer][(g 5)]]  app3 (z:String => [z])`
	_, compiled, errC, _, _ := runBothEngines(t, src)
	if !compiled || errC == nil {
		t.Fatalf("compiled=%v err=%v, want a compiled program that raises", compiled, errC)
	}
	for _, want := range []string{
		"cannot call `g`",          // the binding NAME, from DynApplyName
		"1:37",                     // the READ's position, likewise
		"candidate `g (String)`",   // the overloads, from the applied fn itself
		"group the call in parens", // the forward-args help, via installedSigView
	} {
		if !strings.Contains(errC.Error(), want) {
			t.Errorf("compiled diagnostic missing %q:\n%s", want, errC)
		}
	}
}

// TestDynApplyHeadNameUnnamed pins the negative: a head that was NOT a bare
// read seats nothing, so the diagnostic keeps the nameless form it always
// had — and the two lanes still agree on the failure itself.
func TestDynApplyHeadNameUnnamed(t *testing.T) {
	rows := []struct{ src, note string }{
		// an `apply`-word arrival: OpCallDynApplyTop, never seated.
		{`def app9 fn [[g:Function][Integer][5 g/v apply]]  app9 (z:Integer => [z mul 2])`, "10 — the apply word's own dispatch"},
		// a `/v` delivery inside the paren is a VALUE, not a word read.
		{`def appv fn [[g:Function][Integer][(g/v 5)]]  appv (z:Integer => [z mul 2])`, "10 — a /v delivery"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Logf("%q: not compiled (%s) — refusal, not a divergence", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestDynApplyHeadNameNamelessArm pins the NEGATIVE half of the seat: a head
// the recorder did not see read bare seats no name, so the op's no-match keeps
// the nameless builder (RuntimeNoMatch over the applied fn's own name) it
// always used. Without a row here that arm is unreachable — every other
// no-match the corpus reaches now carries a name.
//
// The witness is a `/v` delivery, and it carries a PRE-EXISTING divergence of
// its own, unchanged by this increment and byte-identical on the commit before
// it (NUR124's direction — the compiled lane APPLIES where the interpreter
// PARKS):
//
//	def appv fn [[g:Function][Integer][(g/v 5)]]
//	appv (z:String => [z])
//	  interpreted  type_error: appv: expected 1 return value(s), got 2 — [fn g(String) 5]
//	  compiled     signature_error: cannot call `` — no signature matches …
//
// The interpreter leaves the /v-parked fn as DATA inside the paren; the VM's
// callDynTrailTop strips the stored value's construction-time quote to mirror
// a READ-substituted arrival and so applies it. That strip is deliberate and
// documented at its site; what this row pins is only that the resulting
// diagnostic stays NAMELESS, since `g/v` is a value delivery and not the word
// dispatch the interpreter would name.
func TestDynApplyHeadNameNamelessArm(t *testing.T) {
	const src = `def appv fn [[g:Function][Integer][(g/v 5)]]  appv (z:String => [z])`
	_, compiled, errC, _, errI := runBothEngines(t, src)
	if !compiled || errC == nil || errI == nil {
		t.Fatalf("compiled=%v errC=%v errI=%v, want both lanes raising", compiled, errC, errI)
	}
	if !strings.Contains(errC.Error(), "cannot call ``") {
		t.Errorf("a /v delivery is not a word read, so the no-match stays nameless:\n%s", errC)
	}
	if !strings.Contains(errI.Error(), "expected 1 return value(s), got 2") {
		t.Errorf("the interpreter still PARKS the /v fn — re-measure this record:\n%s", errI)
	}
}

// TestDynApplyHeadNameWrittenTupleDeclines records the boundary this
// increment does NOT close (measured 2026-09-06). The interpreter's no-match
// prints the WRITTEN tuple — the tokens its forward window consumed — and a
// WORD argument was already substituted onto the value stack by the pointer
// before the head dispatched, so that tuple is EMPTY:
//
//	def app5 fn [[g:Function y:Integer][Integer][(g y)]]
//	app5 (z:String => [z]) 7
//	  interpreted  note: candidate `g (String)` takes 1 argument, but none were supplied
//	  compiled     note: the argument was 7 (an Integer)
//	               note: candidate `g (String)` — argument 1: expected String, got 7
//
// Both lanes raise the same signature_error at the same place under the same
// name; only the tuple the notes describe differs. The SUCCEEDING shape
// agrees exactly (`app5 (z:Integer => [z mul 2]) 7` → 14 on both), so this is
// a diagnostic residue, not a semantic one — and closing it needs the
// recorder to record, per argument, whether the interpreter's window would
// have WRITTEN it, which is a separate seam from the head's name.
func TestDynApplyHeadNameWrittenTupleDeclines(t *testing.T) {
	const bad = `def app5 fn [[g:Function y:Integer][Integer][(g y)]]  app5 (z:String => [z]) 7`
	_, compiled, errC, _, errI := runBothEngines(t, bad)
	if !compiled || errC == nil || errI == nil {
		t.Fatalf("compiled=%v errC=%v errI=%v, want both lanes raising", compiled, errC, errI)
	}
	// The head name, position and taxonomy DO agree — that is this
	// increment's contract, and it holds for the word-argument shape too.
	for _, want := range []string{"cannot call `g`", "1:47"} {
		if !strings.Contains(errC.Error(), want) || !strings.Contains(errI.Error(), want) {
			t.Errorf("both lanes should carry %q:\ncompiled=%s\ninterp=%s", want, errC, errI)
		}
	}
	// The written tuple does not, and stays recorded until its own increment.
	if !strings.Contains(errI.Error(), "none were supplied") {
		t.Errorf("interpreter no longer prints the empty written tuple — re-measure this record:\n%s", errI)
	}
	if !strings.Contains(errC.Error(), "the argument was 7") {
		t.Errorf("compiled lane no longer prints the applied tuple — re-measure this record:\n%s", errC)
	}
	// The same shape SUCCEEDS identically when the head matches.
	good := `def app5 fn [[g:Function y:Integer][Integer][(g y)]]  app5 (z:Integer => [z mul 2]) 7`
	gotC, ok, ec, gotI, ei := runBothEngines(t, good)
	if !ok {
		t.Fatalf("%q: not compiled", good)
	}
	requireParity(t, good, gotC, ec, gotI, ei)
}

// TestWrittenTuplePrefixRule pins the CORRECTED statement of the boundary
// above (measured 2026-09-07, the twentieth increment). Everything here is
// pre-existing — byte-identical on `120fb37`, before the head-name work —
// and it overturns the rule the nineteenth increment recorded in three ways.
//
// (1) The tuple is a PREFIX, not a filter over argument kinds. Forward
// collection walks the tokens after the head LEFT TO RIGHT and stops at the
// first WORD read, which the pointer had already substituted onto the value
// stack. Measured on the interpreter with a 3-param callee:
//
//	(g 5 6 y)  written [5 6]
//	(g 5 y 6)  written [5]      <- decisive: a FILTER would say [5 6]
//	(g y 5 6)  written []
//
// (2) The scope is TWO ops, not one. The multi-argument rows lower to
// OpCallDynFrame, the whole-frame replay, whose diagnostic passes its whole
// arg window to NoMatchDiag exactly as the trailing apply's did. The
// one-argument family alone could not have shown either fact — a 1-arg
// prefix and a 1-arg filter are the same function.
//
// (3) The discriminator is SYNTACTIC, not an operand kind. A body-local
// bound to a literal lowers its read to PUSH_CONST — the same operand a
// written literal produces — and is still NOT written, because the
// interpreter saw a word:
//
//	def f fn [[g:Function][Integer][def y 7  (g y)]]   -> PUSH_CONST 7, written []
//	def f fn [[g:Function][Integer][(g 5)]]            -> PUSH_CONST 5, written [5]
//
// So closing this needs a signal neither lane has today: the engine notes
// only fn-admitting bare reads (noteWordRead's gate), while the tuple needs
// EVERY bare read marked, whatever its type. That is a kernel-side seam, not
// a compiler-side one, which is why this increment records rather than fixes.
func TestWrittenTuplePrefixRule(t *testing.T) {
	const c3 = `def f fn [[g:Function y:Integer][Integer][(g %s)]]  f ([a:String b:String c:String] => [a]) 7`
	rows := []struct{ args, wantInterp, note string }{
		{`5 6 y`, "the arguments were 5 (an Integer) and 6 (an Integer)", "prefix of two"},
		{`5 y 6`, "the argument was 5 (an Integer)", "prefix of one — a filter would say two"},
		{`y 5 6`, "takes 3 arguments, but none were supplied", "empty prefix"},
	}
	for _, c := range rows {
		src := fmt.Sprintf(c3, c.args)
		_, compiled, errC, _, errI := runBothEngines(t, src)
		if !compiled || errC == nil || errI == nil {
			t.Fatalf("(g %s): compiled=%v errC=%v errI=%v, want both lanes raising", c.args, compiled, errC, errI)
		}
		if !strings.Contains(errI.Error(), c.wantInterp) {
			t.Errorf("(g %s) %s: interpreter tuple moved — re-measure this record:\n%s", c.args, c.note, errI)
		}
		// Both lanes still agree on the failure itself; only the tuple differs.
		for _, want := range []string{"cannot call `g`", "signature_error"} {
			if !strings.Contains(errC.Error(), want) || !strings.Contains(errI.Error(), want) {
				t.Errorf("(g %s): both lanes should carry %q:\ncompiled=%s\ninterp=%s", c.args, want, errC, errI)
			}
		}
		// The compiled lane names every argument it applied, which is what
		// the prefix rule has to replace.
		if !strings.Contains(errC.Error(), "the arguments were") {
			t.Errorf("(g %s): compiled lane no longer prints the full tuple — re-measure:\n%s", c.args, errC)
		}
	}
}

// TestWrittenTupleConstFoldedLocalDeclines is (3) above on its own: the row
// that rules out "operand kind decides". `def y 7  (g y)` folds the read to a
// PUSH_CONST identical to a written literal's, and still diverges — so a fix
// keyed on the lowered operand would answer this one wrong while looking
// right on every literal row.
func TestWrittenTupleConstFoldedLocalDeclines(t *testing.T) {
	const folded = `def f fn [[g:Function][Integer][def y 7  (g y)]]  f (z:String => [z])`
	const literal = `def f fn [[g:Function][Integer][(g 5)]]  f (z:String => [z])`
	_, ok1, errC1, _, errI1 := runBothEngines(t, folded)
	if !ok1 || errC1 == nil || errI1 == nil {
		t.Fatalf("folded: compiled=%v errC=%v errI=%v", ok1, errC1, errI1)
	}
	if !strings.Contains(errI1.Error(), "none were supplied") {
		t.Errorf("a const-folded body-local read is still a WORD to the interpreter — re-measure:\n%s", errI1)
	}
	// Its literal twin, same lowered operand kind, DOES reach parity.
	gotC, ok2, errC2, gotI, errI2 := runBothEngines(t, literal)
	if !ok2 {
		t.Fatalf("%q: not compiled", literal)
	}
	requireParity(t, literal, gotC, errC2, gotI, errI2)
}
