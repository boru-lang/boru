package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// emit_markwindow_test.go pins the mark-window island's emit-side guards
// (plan Phase 5, L-DO part 2b) the corpus rows don't reach: the
// fragment-anchor decline in markWindowShape, the static-fn-value window arm
// in resolveDynamicApply, and verifyMarkWindow's mismatch arms. The landed
// shapes themselves are pinned end-to-end in
// lang/go/bytecode_markwindow_test.go.

// A residual[0] whose variadic producer lowers OUTSIDE the top-level frame (a
// fragment event) must decline: lowerEvents reads markBefore only over
// frames[0], so arming would emit no OpStackMark and the VM island would
// raise where the interpreter succeeds.
func TestMarkWindowShapeFragmentAnchorDeclines(t *testing.T) {
	es := NewEmitState()
	v0, v1 := core.NewCarrier(core.TAny), core.NewCarrier(core.TAny)
	es.producedBy[v0.ID] = producer{seq: 3, idx: 0}
	es.producedBy[v1.ID] = producer{seq: 3, idx: 1}
	es.eventInfo[3] = eventFlags{variadicResult: true}
	if seq, ok := es.markWindowShape([]core.Value{v0, v1}, nil); ok {
		t.Fatalf("fragment anchor must decline, got seq %d", seq)
	}
	// The same shape with the producer IN the top-level frame arms at it.
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 3, kind: evCall})
	if seq, ok := es.markWindowShape([]core.Value{v0, v1}, nil); !ok || seq != 3 {
		t.Fatalf("top-level anchor must arm at seq 3, got %d/%v", seq, ok)
	}
}

// A STATIC fn value preceding residual args inside an armed window rides the
// island (auto-apply is exactly what the re-step reproduces); the same shape
// without the window keeps its compile failure.
func TestResolveDynamicApplyFnValueWindowArm(t *testing.T) {
	es := NewEmitState()
	lw := &lowerer{es: es, p: &Program{}}
	residual := []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TFunction), core.NewCarrier(core.TInteger)}
	es.markWindowSeq = 7
	_, op, reason := es.resolveDynamicApply(lw, residual)
	if op != OpCallDynMixedFromMark || reason != "" {
		t.Fatalf("armed window must island the fn value: op=%v reason=%q", op, reason)
	}
	es.markWindowSeq = 0
	_, op, reason = es.resolveDynamicApply(lw, residual)
	if op != 0 || !strings.Contains(reason, "fn value precedes") {
		t.Fatalf("unarmed shape must keep the compile failure: op=%v reason=%q", op, reason)
	}
}

// verifyMarkWindow pins residual == lowered sim stack: a slot produced by a
// DIFFERENT event (same length) and a length drift both decline; the exact
// match passes.
func TestVerifyMarkWindowMismatchArms(t *testing.T) {
	lw := &lowerer{vm: []vmSlot{{seq: 1, idx: 0}}}
	if reason := lw.verifyMarkWindow([]EmitOperand{EventOperand(1, 0)}); reason != "" {
		t.Fatalf("exact match must pass, got %q", reason)
	}
	if reason := lw.verifyMarkWindow([]EmitOperand{EventOperand(2, 0)}); reason == "" {
		t.Fatal("a slot from a different event must decline")
	}
	if reason := lw.verifyMarkWindow(nil); reason == "" {
		t.Fatal("a length drift must decline")
	}
}
