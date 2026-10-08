package lang

import (
	"fmt"
	"strings"
	"testing"
)

// OpCallNativePoly's FAITHFUL no-match raise (plan 3c): a poly recorded with a
// PolyNoMatchSpec — the check pass proved, at the failed-dispatch tape state
// it recovered from, that the interpreter's sigError diagnostic is
// rebuildable from the runtime operand window — raises the byte-identical
// signature_error in the VM instead of deferring the whole run to the
// interpreter. The three rows cover the written-tuple derivation modes the
// spec must reproduce: a single unclaimed forward token (`(x add 1)` renders
// one argument), the full stack prefix (`1 x add` renders both), and the
// stack fallback after a word breaks the forward walk (`(x add x)` renders
// one).
func TestPolyNoMatchRaisesByteIdentical(t *testing.T) {
	rows := []string{
		`def m (flex {k:[1 2]})
def x (m get "k")
(x add 1)`,
		`def m (flex {k:[1 2]})
def x (m get "k")
1 x add`,
		`def m (flex {k:[1 2]})
def x (m get "k")
(x add x)`,
	}
	for _, src := range rows {
		t.Run(fmt.Sprintf("%.20s", strings.ReplaceAll(src, "\n", " ")), func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, ran, errC := a.RunCompiled(src)
			if noteCompileDefect(t, src, nil, errC) {
				return
			}
			if !ran {
				t.Fatal("the row must run COMPILED — the poly no-match raise owns it, not a fallback")
			}
			if errC == nil {
				t.Fatal("the compiled poly no-match must raise")
			}
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			_, errI := b.RunInterp(src)
			if errI == nil {
				t.Fatal("the interpreter must raise")
			}
			if errC.Error() != errI.Error() {
				t.Errorf("poly no-match raise is not byte-identical:\n=== COMPILED ===\n%v\n=== INTERP ===\n%v", errC, errI)
			}
		})
	}
}

// TestPolyNoMatchAfterEffectRaisesCanonical pins the bug the raise closes: a
// side effect BEFORE a poly no-match used to trip the C1 effect fence — the
// fallback was blocked ("output was already emitted") and the user saw an
// internal_error telling them to report a compiler bug instead of the
// canonical signature_error. With the spec-gated raise the compiled run keeps
// its effects (printed exactly once) and raises the interpreter's error.
func TestPolyNoMatchAfterEffectRaisesCanonical(t *testing.T) {
	const src = `print "pre-effect"
def m (flex {k:[1 2]})
def x (m get "k")
(x add 1)`
	var outC, outI strings.Builder
	a := mustNew(t)
	a.SetOutput(&outC)
	_, ran, errC := a.RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !ran {
		t.Fatal("the effectful row must run COMPILED — deferring here is the fence-blocked bug")
	}
	b := mustNew(t)
	b.SetOutput(&outI)
	_, errI := b.RunInterp(src)
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("effectful poly no-match parity:\n=== COMPILED ===\n%v\n=== INTERP ===\n%v", errC, errI)
	}
	if got := outC.String(); got != outI.String() || strings.Count(got, "pre-effect") != 1 {
		t.Errorf("the effect must run exactly once: compiled output %q, interp output %q", outC.String(), outI.String())
	}
}

// TestPolyNoMatchDeeperStackKeepsDefer — the written-tuple bound (the
// local-add lesson applied to polys): with a third concrete value below the
// 2-operand window, the interpreter's stack-prefix derivation renders THREE
// arguments where the window holds two, so the spec's window mapping declines
// at record time and the sound defer stays — parity via the interpreter
// fallback.
func TestPolyNoMatchDeeperStackKeepsDefer(t *testing.T) {
	const src = `def m (flex {k:[1 2]})
def x (m get "k")
9 1 x add`
	a := mustNew(t)
	_, _, errC := a.RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	b := mustNew(t)
	_, errI := b.RunInterp(src)
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("fallback parity: compiled-path err %v != interp err %v", errC, errI)
	}
}

// TestUserPolyNoMatchAfterEffectRaisesRich pins the user-poly half of the
// fence-blocked fix (3c part 2, the bounded alt): a laundered-Any
// multi-overload call whose runtime value matches no arm, AFTER a print
// effect. The open-fallback arm keeps byte-identity by re-running (pinned by
// TestUserPolyNoMatchErrorAgreement); here nothing re-runs, and
// the defer's DeferAlt surfaces the rich signature_error — canonical Detail,
// live-value candidate verdicts — instead of an internal error telling the
// user to report a compiler bug. Best-effort by design: the rendered tuple
// is the operand window, which may be wider than the tape-derived tuple the
// interpreter would show.
func TestUserPolyNoMatchAfterEffectRaisesRich(t *testing.T) {
	const src = `def g fn [[a:Integer] [String] ["i"] [a:String] [String] ["s"]]
print "pre-effect"
def get-k fn [[m:Map] [Any] [m get "k"]]
def x (get-k {k:[1 2]})
(g x)`
	var out strings.Builder
	a := mustNew(t)
	a.SetOutput(&out)
	_, ran, err := a.RunCompiled(src)
	if noteCompileDefect(t, src, nil, err) {
		return
	}
	if !ran {
		t.Fatal("the effectful row must run COMPILED (the fence owns the arm)")
	}
	if codeOf(err) != "signature_error" {
		t.Fatalf("err = %v, want the rich signature_error alt", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "cannot call `g` — no signature matches the arguments") ||
		!strings.Contains(msg, "candidate `g (Integer)`") {
		t.Errorf("the alt must carry the canonical detail and candidate verdicts:\n%v", msg)
	}
	if strings.Contains(msg, "report this as a compiler bug") {
		t.Errorf("the compiler-bug note must be gone:\n%v", msg)
	}
	if strings.Count(out.String(), "pre-effect") != 1 {
		t.Errorf("the effect must run exactly once, output %q", out.String())
	}
}

// TestPolyNoMatchDeeperStackShapeRaisesCanonical — the former bounded edge:
// the deeper-stack native shape (`9 1 x add`, the whole prefix beneath the
// word) used to decline the faithful spec (the written tuple was wider than
// the poly's window) AND fail the alt's arity-uniformity screen (`add` has
// 3-arg overloads), so the bail kept an internal error. The attempted-window
// twin (NUR172: rematchWritten derives the tuple sigError renders) lets the
// record prove the raise over the three values the interpreter reports, so
// the shape raises the canonical signature_error on both lanes, after the
// effect ran exactly once.
func TestPolyNoMatchDeeperStackShapeRaisesCanonical(t *testing.T) {
	const src = `print "pre-effect"
def m (flex {k:[1 2]})
def x (m get "k")
9 1 x add`
	var out strings.Builder
	a := mustNew(t)
	a.SetOutput(&out)
	_, _, errC := a.RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	var outI strings.Builder
	b := mustNew(t)
	b.SetOutput(&outI)
	_, errI := b.RunInterp(src)
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Fatalf("the deeper-stack shape must raise byte-identically:\n=== COMPILED ===\n%v\n=== INTERP ===\n%v", errC, errI)
	}
	if !strings.Contains(errC.Error(), "the arguments were [1 2] (a FlexList), 1 (an Integer) and 9 (an Integer)") {
		t.Errorf("the raise must report the attempted window:\n%v", errC)
	}
	if strings.Count(out.String(), "pre-effect") != 1 {
		t.Errorf("the effect must run exactly once, output %q", out.String())
	}
}
