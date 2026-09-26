package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// host_splice_test.go covers the recorder half of the hosted splice
// (2026-09-26): which signatures host (hostsSplice), where the record-time
// admission holds (hostSpliceHere), the whole-program admission Finalize
// asks (hostedSpliceAdmitted), the call site's own SigRef, and the
// uncaptured-arm reason a clause-list `if` carries into RecordBranch.

func TestHostsSplice(t *testing.T) {
	own, dyn := core.CompileOwnLowering, core.CompileDynBody
	if !hostsSplice(&core.Signature{CompileEffect: own | dyn}) {
		t.Error("a structured-lowering dyn-body word with no CallableSpec (`for`) hosts its splice")
	}
	for name, sig := range map[string]*core.Signature{
		"a clause-list dyn-body word (receive) runs its own bodies": {CompileEffect: dyn | core.CompileRunsBodyOnRegistry},
		"a structured word without the backstop keeps its decline":  {CompileEffect: own},
		"a closure-compiled body word runs its body through a seam": {CompileEffect: own | dyn, Callable: &core.CallableSpec{}},
	} {
		if hostsSplice(sig) {
			t.Errorf("%s: must not host a splice", name)
		}
	}
}

func TestHostSpliceHere(t *testing.T) {
	r := newTestRegistry(t)
	es := NewEmitState()
	if hostSpliceHere(r, es) {
		t.Error("no program registry bound yet: not the program's top level")
	}
	es.progReg = r
	if !hostSpliceHere(r, es) {
		t.Error("the program unit's top level on the program registry admits")
	}
	if hostSpliceHere(newTestRegistry(t), es) {
		t.Error("a module body's registry is not the program's")
	}
	es.frames = append(es.frames, nil)
	if hostSpliceHere(r, es) {
		t.Error("an open fragment (a branch arm, a loop body) declines")
	}
	es.frames = es.frames[:1]
	es.units = append(es.units, &emitUnit{})
	if hostSpliceHere(r, es) {
		t.Error("an open fn or closure unit declines")
	}
}

func TestHostedSpliceAdmitted(t *testing.T) {
	es := NewEmitState()
	if _, ok := es.hostedSpliceAdmitted(nil); !ok {
		t.Fatal("a program with no hosted splice is admitted")
	}
	out := core.NewCarrier(core.TList)
	out.ID = "loop-out"
	es.hostSplices = []int{2}
	es.frames[0] = []EmitEvent{{seq: 1}, {seq: 2}}
	es.producedBy[out.ID] = producer{seq: 2}
	if reason, ok := es.hostedSpliceAdmitted([]core.Value{out}); !ok {
		t.Fatalf("the last event, its one result the whole residual: admitted, got %q", reason)
	}
	other := core.NewCarrier(core.TList)
	other.ID = "other"
	for name, residual := range map[string][]core.Value{
		"a value beneath the loop":   {core.NewInteger(9), out},
		"a residual not the loop's":  {other},
		"no residual (a trap ended)": nil,
	} {
		if reason, ok := es.hostedSpliceAdmitted(residual); ok || !strings.Contains(reason, "over an empty stack") {
			t.Errorf("%s: want the empty-stack decline, got ok=%v %q", name, ok, reason)
		}
	}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 3})
	if reason, ok := es.hostedSpliceAdmitted([]core.Value{out}); ok || !strings.Contains(reason, "last statement") {
		t.Errorf("an event after the loop: want the last-statement decline, got ok=%v %q", ok, reason)
	}
	es.frames[0] = nil
	if _, ok := es.hostedSpliceAdmitted([]core.Value{out}); ok {
		t.Error("a top level with no events (rolled back) declines")
	}
}

func TestDynBodyPolyOnGradualBody(t *testing.T) {
	r := newTestRegistry(t)
	body := core.NewCarrier(core.TList)
	args := []core.Value{core.NewInteger(3), body}
	if dynBodyPoly(r, "for", args, body) {
		t.Error("a typed List body over a concrete count bakes the committed sig")
	}
	body.Dynamic = true
	if !dynBodyPoly(r, "for", []core.Value{core.NewInteger(3), body}, body) {
		t.Error("a gradual body re-matches at run time")
	}
}

func TestHostSpliceSigRefDisassembly(t *testing.T) {
	sig := core.Signature{Args: []*core.Type{core.TInteger, core.TList}, BarrierPos: -1}
	p := &Program{
		Code:  []Instr{{Op: OpCallNative, Arg: 0}},
		Debug: make([]core.SrcPos, 1),
		Sigs:  []SigRef{{Word: "for", Sig: &sig, HostSplice: true}},
	}
	if out := p.Disassemble(); !strings.Contains(out, "[hosted splice]") {
		t.Errorf("a hosted splice's CALL_NATIVE is marked in the disassembly:\n%s", out)
	}
}

func TestRecordBranchUncapturedReason(t *testing.T) {
	es := NewEmitState()
	tru := true
	es.RecordBranch(core.BranchRecord{ConstCond: &tru, HasElse: true, Uncaptured: "clause-list if: why"})
	if es.Compilable || es.Reason != "if: taken-branch not captured — clause-list if: why" {
		t.Fatalf("the uncaptured arm carries its reason: compilable=%v reason=%q", es.Compilable, es.Reason)
	}
}
