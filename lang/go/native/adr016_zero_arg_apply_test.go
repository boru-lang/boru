package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// runTail evaluates src and returns its trailing value.
func runTail(t *testing.T, r *Registry, src string) Value {
	t.Helper()
	out, err := seam5Run(r, src)
	if err != nil || len(out) == 0 {
		t.Fatalf("run %q: %v / %v", src, out, err)
	}
	return out[len(out)-1]
}

// parkedFn parks a fn with `/v` and returns the Function VALUE. A bare
// 0-arg NAMED fn would dispatch on the spot (which is half of the
// asymmetry under test), so only the parked form yields what `apply`
// is contracted on.
func parkedFn(t *testing.T, r *Registry, src string) Value {
	t.Helper()
	return runTail(t, r, src)
}

// TestADR016ZeroArgApplyIgnoresOrigin pins NUR077 §5 Hole 1: `apply` on a
// fn whose only signatures are 0-arg produces the APPLIED result whether
// the fn is anonymous or named.
//
// Before the fix, applyHandler handed the value back for the engine to
// re-step, and the re-step reaches execFnDefLiteral's inert-lambda gate,
// which parks a 0-arg ANONYMOUS value. So `f/v apply` answered `fn f` for
// a lambda and `42` for the named twin — origin deciding behaviour, which
// ADR-016 forbids.
//
// The assertion is deliberately END-TO-END rather than a direct
// applyHandler call: the handler no longer performs the call, it MARKS the
// value (markApplied) and the ordinary re-step dispatches it. Testing the
// handler's return in isolation would pin the mark and miss the semantics.
func TestADR016ZeroArgApplyIgnoresOrigin(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"anonymous lambda", `def f ([] => [42]) end f/v apply`},
		{"named fn", `def z fn [[] [Integer] [42]] end z/v apply`},
	} {
		r := seam5Reg(t)
		got := runTail(t, r, tc.src)
		n, err := AsInteger(got)
		if err != nil || n != 42 {
			t.Errorf("%s: want the APPLIED result 42, got %v (err %v) — "+
				"origin must not decide at arity 0", tc.name, got, err)
		}
	}
}

// TestADR016ApplyKeepsArgTakingFnInert pins the restriction that keeps the
// fix from eclipsing an arg-taking overload (the NUR035 hazard): a fn with
// any arg-taking signature is still handed back for the engine to re-step
// against its stack args, which is what makes `5 inc/v apply` work.
func TestADR016ApplyKeepsArgTakingFnInert(t *testing.T) {
	r := seam5Reg(t)
	for _, src := range []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]] end inc/v`,
		`def ov fn [[] [Integer] [99] [n:Integer] [Integer] [n add 1]] end ov/v`,
	} {
		out, err := applyHandler([]Value{parkedFn(t, r, src)}, nil, nil, r)
		if err != nil {
			t.Fatalf("applyHandler %q: %v", src, err)
		}
		if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
			t.Errorf("%q: an arg-taking fn must come back as the FUNCTION for "+
				"the re-step to bind its args, got %v", src, out)
		}
	}
}

// TestADR016AppliedMarkIsNotACall pins the SEAM the fix turns on: apply
// states that a call was asked for and leaves the calling to the re-step.
// An earlier revision called from the handler instead — through
// the since-retired CallBoruFn — and that second dispatch path is what
// produced the three divergences the tests below cover. If this ever goes back to returning
// a non-Function, those regressions are back too.
func TestADR016AppliedMarkIsNotACall(t *testing.T) {
	r := seam5Reg(t)
	out, err := applyHandler(
		[]Value{parkedFn(t, r, `def f ([] => [42]) end f/v`)}, nil, nil, r)
	if err != nil {
		t.Fatalf("applyHandler: %v", err)
	}
	if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
		t.Fatalf("apply must MARK, not call: want the Function back, got %v", out)
	}
	if _, ok := FnDefOf(out[0].Data); !ok || !core.IsApplied(&out[0]) {
		t.Errorf("want the one-shot Applied mark set for the re-step, got %+v", out[0])
	}
}

// TestADR016ApplyKeepsNativeHandler pins that a NATIVE 0-arg fn keeps its
// Go handler under apply. Calling from the handler ran the native's EMPTY
// boru body instead, so `valof context apply typeof` consumed the function
// and left `typeof` with no argument.
func TestADR016ApplyKeepsNativeHandler(t *testing.T) {
	r := seam5Reg(t)
	got := runTail(t, r, `valof context apply typeof`)
	if s := got.String(); s != "Store" {
		t.Errorf("want the native handler's Store, got %v — a native 0-arg fn "+
			"must not be routed through an empty boru body", s)
	}
}

// TestADR016ApplyPreservesCallerContext pins that a 0-arg body's context
// mutations land in the CALLER's frame. Calling from the handler ran the
// body in a sub-engine, so the mutation was lost and `f/v apply` diverged
// both from the direct call and from the compiled path.
func TestADR016ApplyPreservesCallerContext(t *testing.T) {
	const prelude = `context set x/q 1 end ` +
		`def f fn [[] [Integer] [context set x/q 2 end 42]] end `
	for _, tc := range []struct{ name, src string }{
		{"direct call", prelude + `f context get x/q`},
		{"via apply", prelude + `f/v apply context get x/q`},
	} {
		r := seam5Reg(t)
		got := runTail(t, r, tc.src)
		n, err := AsInteger(got)
		if err != nil || n != 2 {
			t.Errorf("%s: want the mutated context value 2, got %v (err %v)",
				tc.name, got, err)
		}
	}
}

// TestADR016ApplyResultIsModelledByCheck pins that the check pass carries
// the APPLIED result forward, so a downstream consumer type-checks against
// it. Because the mark is a value stamp both engines set (markApplied),
// the check engine walks the same gate the interpreter does. The earlier
// revision applied only at runtime and had to declare the shape
// uncompilable; `f/v apply add 1` then ran to 43 but failed `check` with
// no_signature over (Function, Integer).
func TestADR016ApplyResultIsModelledByCheck(t *testing.T) {
	const src = `def f ([] => [42]) end f/v apply add 1`

	r := seam5Reg(t)
	got := runTail(t, r, src)
	if n, err := AsInteger(got); err != nil || n != 43 {
		t.Fatalf("interpreted: want 43, got %v (err %v)", got, err)
	}

	rc := seam5Reg(t)
	if _, err := seam5Check(rc, src); err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range rc.Check.Diagnostics {
		if d.Severity == SeverityError {
			t.Errorf("check must model the applied result, got %s: %s",
				d.Code, d.Detail)
		}
	}
}
