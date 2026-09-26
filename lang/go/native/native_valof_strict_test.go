package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// native_valof_strict_test.go covers the check-mode half of the
// dispatch-modifier words' fn-operand contract. Until 2026-09-26
// recordGradualWrap declined the poly record for a TYPED Function carrier at
// a CompileFnHandlerStrict slot (NUR158: the VM delivered a compiled closure
// the native's FnDefInfo assertion refused); the natives wrap a compiled
// closure now (wrapCompiledClosure), so every gradual carrier records.
func TestRecordGradualWrapRecordsEveryGradualCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	typed := NewCarrier(TFunction)
	if typed.Dynamic {
		t.Fatal("fixture: a typed carrier is not dynamic")
	}
	out := []Value{NewDynamicCarrier(TFunction)}
	// No panic, no decline arm: the typed factory result, the sibling
	// wrap's dynamic Function, and a dynamic-Any read all reach the recorder.
	recordGradualWrap(r, "usurp", []Value{typed}, out)
	recordGradualWrap(r, "force-arity", []Value{NewInteger(2), typed}, out)
	recordGradualWrap(r, "usurp", []Value{NewDynamicCarrier(TFunction)}, out)
	recordGradualWrap(r, "usurp", []Value{NewDynamicCarrier(TAny)}, out)
	recordGradualWrap(nil, "usurp", []Value{typed}, out)
}

// TestWrapCompiledClosureDeclinesOffTheCompiledLane pins wrapCompiledClosure's
// refusals: no registry, a value that is not a compiled closure, and a
// closure met outside a VM run (no invoker, so no bridge) — each leaves the
// word to raise its own illegal_ref exactly as before. The wrapping itself
// is proved end to end in lang/go (sweep_cells_s2b_test.go).
func TestWrapCompiledClosureDeclinesOffTheCompiledLane(t *testing.T) {
	r := seam5Reg(t)
	cl := Value{Parent: TFunction, Data: core.ClosurePayload{Unit: 0}}
	if _, ok := wrapCompiledClosure(nil, "usurp", cl, 0); ok {
		t.Error("nil registry must decline")
	}
	if _, ok := wrapCompiledClosure(r, "usurp", NewInteger(1), 0); ok {
		t.Error("a non-closure must decline")
	}
	for _, w := range []string{"usurp", "stack-args", "forward-args", "force-arity"} {
		if _, ok := wrapCompiledClosure(r, w, cl, 2); ok {
			t.Errorf("%s: a closure outside a run has no bridge and must decline", w)
		}
	}
}
