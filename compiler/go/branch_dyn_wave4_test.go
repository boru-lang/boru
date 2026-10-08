package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Coverage for the computed-arm branch lowerings (lower.go
// lowerBothComputed / lowerComputedCond / lowerComputedBranch, emit.go
// computedArmCondOK), the paren-bounded dynamic apply (engine.go
// stepCloseParen's RecordDynApply seam, vm.go callDynApplyTop), the
// user-fn poly re-match (RecordUserPolyCall / lowerUserPolyCall /
// matchUserPoly), plain-check dispatch recoveries, and small dispatch
// error paths (insufficientArgsError, implicit end / curryOrStack).

// --- computed-arm branches -------------------------------------------------------

func registerUserPoly2(r *core.Registry) {
	registerDynWords(r)
	// Both overloads declare IDENTICAL returns — the poly-arm gate.
	core.InstallFnDef(r, "upoly2", core.FnDefInfo{
		Signatures: []core.Signature{
			{
				Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewWord("cadd"), core.NewWord("n"), core.NewInteger(1))),
				BarrierPos: core.BarrierAllForward,
			},
			{
				Params:     []core.FnParam{{Name: "s", Type: core.TString}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewInteger(-1))),
				BarrierPos: core.BarrierAllForward,
			},
		},
	})
}

func TestUserPolyReMatchInterpretedString(t *testing.T) {
	// The sibling overload dispatches for a String operand (interpreted
	// reference for the same word set).
	r := covRegistry(t, registerUserPoly2)
	out, err := core.NewTop(r).Run([]core.Value{core.NewWord("upoly2"), core.NewString("x")})
	if err != nil {
		t.Fatalf("upoly2 string: %v", err)
	}
	if got := renderAll(out); got != "-1" {
		t.Errorf("upoly2 string = %q", got)
	}
}

// --- plain-check dispatch recoveries ------------------------------------------------------

func plainCheckRun(t *testing.T, extra func(*core.Registry), tokens []core.Value) []core.CheckDiagnostic {
	t.Helper()
	r := covRegistry(t, extra)
	done := r.Check.Begin()
	_, err := core.NewTop(r).Run(tokens)
	if err != nil {
		t.Fatalf("plain check errored hard: %v", err)
	}
	diags := append([]core.CheckDiagnostic(nil), r.Check.Diagnostics...)
	done()
	return diags
}

func TestPlainCheckSingleOverloadUserFnDynamicArg(t *testing.T) {
	// A single-overload user fn over an unknown-type carrier is the
	// recoverable shape: NO no_signature diagnostic (compile recovers it
	// as a guarded CALL_USER).
	diags := plainCheckRun(t, registerDynWords, []core.Value{
		core.NewWord("ctwice"),
		core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
	})
	for _, d := range diags {
		if d.Code == "no_signature" && d.Word == "ctwice" {
			t.Errorf("recoverable dispatch flagged: %v", d)
		}
	}
}

func TestPlainCheckPolyNativeDynamicArg(t *testing.T) {
	// A multi-overload native over a dynamic operand types via the
	// reachable-overload partition — no hard error.
	diags := plainCheckRun(t, registerDynWords, []core.Value{
		core.NewWord("cdub"),
		core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
	})
	for _, d := range diags {
		if d.Severity == core.SeverityError {
			t.Errorf("dynamic poly dispatch errored: %v", d)
		}
	}
}

func TestInsufficientForwardArgsInParen(t *testing.T) {
	// `(cadd 1)` — the parked forward is orphaned at the close paren.
	r := covRegistry(t, nil)
	_, err := core.NewTop(r).Run([]core.Value{
		core.NewOpenParen(), core.NewWord("cadd"), core.NewInteger(1), core.NewCloseParen(),
	})
	if err == nil {
		t.Fatal("orphaned forward did not error")
	}
	if !strings.Contains(err.Error(), "cadd") {
		t.Errorf("error does not name the word: %v", err)
	}
}

type w4CountRecorder struct {
	lits, calls int
}

func (c *w4CountRecorder) OnPushLit(core.Value)    { c.lits++ }
func (c *w4CountRecorder) OnCall(string, int, int) { c.calls++ }

// --- small pure helpers ---------------------------------------------------------------------

func TestSigOrderArgsShapes(t *testing.T) {
	a, b, c := core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)
	// All-forward (nStack 0): unchanged.
	out := core.SigOrderArgs([]core.Value{a, b, c}, 0)
	if renderAll(out) != "1 | 2 | 3" {
		t.Errorf("all-forward = %s", renderAll(out))
	}
	// Two stack args below one forward: forward first, stack reversed.
	out = core.SigOrderArgs([]core.Value{a, b, c}, 2)
	if renderAll(out) != "3 | 2 | 1" {
		t.Errorf("mixed = %s", renderAll(out))
	}
	// Out-of-range nStack clamps to all-stack.
	out = core.SigOrderArgs([]core.Value{a, b}, 5)
	if renderAll(out) != "2 | 1" {
		t.Errorf("clamped = %s", renderAll(out))
	}
}

func TestFnConcreteSingleValuedOrCarrier(t *testing.T) {
	if !fnConcreteSingleValuedOrCarrier(core.NewCarrier(core.TFunction)) {
		t.Error("carrier declined")
	}
	one := core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "x", Type: core.TInteger}}, Returns: []*core.Type{core.TInteger},
		Impl: core.Boru([]core.Value{core.NewWord("x")}), BarrierPos: core.BarrierAllForward,
	}}})
	if !fnConcreteSingleValuedOrCarrier(one) {
		t.Error("single-return fn declined")
	}
	zero := core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "x", Type: core.TInteger}},
		Impl:   core.Boru([]core.Value{core.NewWord("x")}), BarrierPos: core.BarrierAllForward,
	}}})
	if fnConcreteSingleValuedOrCarrier(zero) {
		t.Error("zero-return fn admitted")
	}
	if fnConcreteSingleValuedOrCarrier(core.NewFunction(core.FnDefInfo{})) {
		t.Error("sig-less fn admitted")
	}
}

func TestCarrierOfLiteralHelper(t *testing.T) {
	c := core.CarrierOfLiteral(core.NewInteger(42))
	if !c.Carrier {
		t.Error("carrierOfLiteral not a carrier")
	}
}
