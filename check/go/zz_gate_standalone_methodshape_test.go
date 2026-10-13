package check

// Standalone-gate tests for method_shape.go's arrival models: the member-fn
// arrival's guard-owned decline of a 0-arg member landing, and the
// def-bound read model's stand-aside where a closure frame holds exactly
// the read's operands. Driven through the recorder double the method-shape
// suite already uses (zzmsRecorder, zz_cover_methodshape_test.go).

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// declineMemberFnArrival re-homes the read guard's decline: a member with a
// genuine 0-arg overload was exempted at the read on the promise that the
// arrival model owns its landing, so a landing the model cannot claim
// declines here with the guard's own reason. A member without a 0-arg
// overload was never exempted: its decline stays silent.
func TestGateStandaloneDeclineZeroArgMemberArrival(t *testing.T) {
	rec := newZZMSRecorder()
	zeroArg := core.NewFunction(core.FnDefInfo{Name: "zgsprop", Signatures: []core.Signature{{
		Impl: core.Boru([]core.Value{core.NewInteger(1)}),
	}}})
	if declineMemberFnArrival(rec, zeroArg) {
		t.Error("a declined landing reports the window not consumed")
	}
	if len(rec.reasons) != 1 || !strings.Contains(rec.reasons[0], "0-arg landing not modelable at zgsprop") {
		t.Errorf("a 0-arg member's landing declines with the guard's reason, got %v", rec.reasons)
	}

	rec = newZZMSRecorder()
	unary := core.NewFunction(core.FnDefInfo{Name: "zgsunary", Signatures: []core.Signature{{
		Params: []core.FnParam{{Name: "x", Type: core.TInteger}},
		Impl:   core.Boru([]core.Value{core.NewWord("x")}),
	}}})
	if declineMemberFnArrival(rec, unary) || len(rec.reasons) != 0 {
		t.Errorf("a member with no 0-arg overload declines silently, got %v", rec.reasons)
	}
}

// The same decline reached through the model: a member read whose value is
// a 0-arg MACRO (a shape the arrival model does not claim) declines with
// the re-homed reason and leaves the tape as it is.
func TestGateStandaloneMemberArrivalZeroArgMacroDeclines(t *testing.T) {
	r := zzmsReg(t)
	done := r.Check.Begin()
	defer done()
	rec := newZZMSRecorder()
	r.Check.Emit = rec
	read := core.NewDynamicCarrier(core.TAny)
	rec.members = map[string]core.Value{core.IDOf(&read): core.NewFunction(core.FnDefInfo{Name: "zgsmacro", Macro: true,
		Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}})}

	tape := []core.Value{read, core.NewEnd()}
	e := zzmsEngine(r, tape)
	if tryMemberFnArrivalDispatch(e, 0) {
		t.Fatal("the model must not claim a macro member's landing")
	}
	if len(rec.reasons) != 1 || !strings.Contains(rec.reasons[0], "0-arg landing not modelable at zgsmacro") {
		t.Errorf("the landing declines with the guard's reason, got %v", rec.reasons)
	}
	if e.Tape.Len() != len(tape) {
		t.Error("a declined landing leaves the tape untouched")
	}
}

// gsClosureRec is the method-shape recorder double inside a closure unit.
type gsClosureRec struct{ *zzmsRecorder }

func (gsClosureRec) InClosureUnit() bool { return true }

// tryShapedFnReadArrival's stand-aside: inside a closure unit, a def-bound
// fn read whose statement window is short but whose FRAME holds exactly the
// read's operands (the element under a closure body's `[a5]`, `def a5 (mk
// 5)  each [a5] xs`) is the body-tail trailing apply the unit's residual
// lowers: the model stands aside with the carrier on the stack and declines
// nothing. A deeper frame is no exact window: it declines as before.
func TestGateStandaloneShapedReadClosureFrameStandsAside(t *testing.T) {
	r := zzmsReg(t)
	done := r.Check.Begin()
	defer done()
	base := newZZMSRecorder()
	r.Check.Emit = gsClosureRec{base}
	carrier := core.NewCarrier(core.TFunction)
	base.defReads = map[string]string{core.IDOf(&carrier): "a5"}
	r.Check.NoteFnShape(carrier, core.FnShape{Arity: 1})

	exact := zzmsEngine(r, []core.Value{core.NewInteger(5), carrier})
	exact.Pointer = 1
	if tryMemberFnArrivalDispatch(exact, 1) {
		t.Fatal("the model must stand aside, not consume the window")
	}
	if len(base.reasons) != 0 || len(base.dynCalls) != 0 {
		t.Errorf("standing aside declines and records nothing (reasons %v, calls %d)", base.reasons, len(base.dynCalls))
	}
	if exact.Tape.Len() != 2 {
		t.Error("standing aside leaves the carrier on the stack")
	}

	deeper := zzmsEngine(r, []core.Value{core.NewInteger(4), core.NewInteger(5), carrier})
	deeper.Pointer = 2
	if tryMemberFnArrivalDispatch(deeper, 2) {
		t.Fatal("a deeper frame's short window must not be consumed")
	}
	if len(base.reasons) != 1 || !strings.Contains(base.reasons[0], "ends short of the wrapper's arity") {
		t.Errorf("a deeper frame declines as before, got %v", base.reasons)
	}
}
