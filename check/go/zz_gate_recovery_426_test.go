package check

// Gate test for the empty-analysis fallback in spliceAnonCheckResult
// (check_recovery.go:426 — `if len(result) == 0`).
//
// spliceAnonCheckResult is the check-mode dispatch for an ANONYMOUS body-bearing
// fn value that is CALLED. It hands the lambda's body to AnalyseFnBody and
// splices the residual carrier stack in place of the args + the fn literal. The
// arm under test is the contract AnalyseFnBody documents: "an empty or nil
// return means the analyser aborted — callers should treat that as an Any
// carrier." Without it the splice would install ZERO values where the call site
// expects the lambda's result, silently shortening the residual stack.
//
// Two distinct input states drive INTO the arm, and both are reachable from the
// engine's anon dispatch. Note that engine.go's three anon call sites
// (ExecFnDefSigStackMatch, ~5480/5531/5569) branch on `checkMode` BEFORE the
// `len(sig.Body()) > 0` guard that the non-anonymous fn-value path uses, so an
// empty-bodied lambda is NOT filtered out upstream:
//
//  1. Empty body — `Impl: core.Boru(nil)` makes sig.Body() nil, and AnalyseFnBody
//     returns nil at its very first guard (`if len(body) == 0`), above the memo,
//     the quota and the recursion cycle-breaker.
//
//  2. Non-empty body with a ZERO-VALUE residual — a body whose sole word declares
//     `Returns: []` produces no carrier at all, so the full analysis runs to
//     completion and still hands back an empty stack. Params are NAMED here, so
//     RunFnBodyOnce binds them as defs rather than pre-pushing them onto the body
//     stack; an unnamed param would leave its arg on the stack and the residual
//     would not be empty.
//
// The boundary case is pinned separately: a body that DOES produce a value must
// splice that value, not the Any fallback, or the arm would be over-firing.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// gate426Registry builds a cov registry plus `g426void`, a native word that
// declares zero returns — the driver for the zero-residual body state.
func gate426Registry(t *testing.T) *core.Registry {
	t.Helper()
	return covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "g426void",
			Signatures: []core.Signature{{
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return nil, nil
				}),
				Returns:    []*core.Type{},
				BarrierPos: -1,
			}},
		})
	})
}

// gate426Call dispatches a one-named-param anonymous lambda over a single
// Integer arg through the engine's anon check-mode path and returns the
// resulting tape.
func gate426Call(t *testing.T, r *core.Registry, body []core.Value, returns []*core.Type) []core.Value {
	t.Helper()
	fnDef := zzAnonDef([]core.FnParam{{Name: "n", Type: core.TInteger}}, returns, body)
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(7), core.NewFunction(fnDef)}, core.StackHeadroom)
	e.Pointer = 1
	if err := e.ExecFnDefSigStackMatch(1, &fnDef, []core.Value{core.NewInteger(7)}); err != nil {
		t.Fatalf("ExecFnDefSigStackMatch: %v", err)
	}
	return e.Tape.Snapshot()
}

// wantSingleAnyCarrier asserts the splice installed exactly the arm's fallback:
// one non-dynamic Any carrier.
func wantSingleAnyCarrier(t *testing.T, tape []core.Value) {
	t.Helper()
	if len(tape) != 1 {
		t.Fatalf("residual: got %d values %v, want exactly 1", len(tape), tape)
	}
	got := tape[0]
	if !got.Carrier {
		t.Errorf("residual must be a carrier, got %#v", got)
	}
	if !got.Parent.Equal(core.TAny) {
		t.Errorf("residual carrier type: got %v, want Any", got.Parent)
	}
	if core.IsDynamic(&got) {
		t.Error("the arm builds core.NewCarrier(core.TAny), which is not dynamic")
	}
}

// TestGaterecovery426EmptyBodyAnalysisFallsBackToAny drives state (1): the
// lambda carries no body at all, so AnalyseFnBody bails at its first guard and
// the arm supplies the Any carrier.
func TestGaterecovery426EmptyBodyAnalysisFallsBackToAny(t *testing.T) {
	r := gate426Registry(t)
	defer r.Check.Begin()()

	// Pin the precondition directly: this exact call is what the arm guards.
	if got := AnalyseFnBody(r, "", []string{"n"}, nil, []core.Value{core.NewInteger(7)}, nil, nil, true); len(got) != 0 {
		t.Fatalf("precondition: an empty body must analyse to an empty stack, got %v", got)
	}

	wantSingleAnyCarrier(t, gate426Call(t, r, nil, nil))
}

// TestGaterecovery426ZeroResidualBodyFallsBackToAny drives state (2): the body
// is present and analysed in full, but its sole word declares zero returns, so
// the residual carrier stack comes back empty anyway.
func TestGaterecovery426ZeroResidualBodyFallsBackToAny(t *testing.T) {
	r := gate426Registry(t)
	defer r.Check.Begin()()

	body := []core.Value{core.NewWord("g426void")}
	if got := AnalyseFnBody(r, "", []string{"n"}, body, []core.Value{core.NewInteger(7)}, nil, nil, true); len(got) != 0 {
		t.Fatalf("precondition: a zero-return body must analyse to an empty stack, got %v", got)
	}

	wantSingleAnyCarrier(t, gate426Call(t, r, body, nil))
}

// TestGaterecovery426ProductiveBodySkipsTheFallback pins the boundary: when
// AnalyseFnBody DOES return a residual, the arm must not fire and the analysed
// carrier — not a bare Any — reaches the tape.
func TestGaterecovery426ProductiveBodySkipsTheFallback(t *testing.T) {
	r := gate426Registry(t)
	defer r.Check.Begin()()

	body := parenBody(core.NewWord("cadd"), core.NewWord("n"), core.NewWord("n"))
	analysed := AnalyseFnBody(r, "", []string{"n"}, body, []core.Value{core.NewInteger(7)}, nil,
		[]*core.Type{core.TInteger}, true)
	if len(analysed) != 1 {
		t.Fatalf("precondition: a productive body must analyse to 1 value, got %v", analysed)
	}

	tape := gate426Call(t, r, body, []*core.Type{core.TInteger})
	if len(tape) != 1 {
		t.Fatalf("residual: got %d values %v, want exactly 1", len(tape), tape)
	}
	if tape[0].Carrier && tape[0].Parent.Equal(core.TAny) {
		t.Error("a productive body must not be replaced by the arm's Any fallback")
	}
}
