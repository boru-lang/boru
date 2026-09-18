package native

import "testing"

// native_valof_strict_test.go covers the check-mode half of the
// dispatch-modifier words' handler-contract declaration (the fn-operand
// pilot, design/HANDLER-MIGRATION-LINE.0.md): recordGradualWrap honours a
// CompileFnHandlerStrict value-form sig by declining the poly record for a
// TYPED Function carrier, and strictFnSlotWord reads the declaration.
func TestStrictFnSlotWord(t *testing.T) {
	r := seam5Reg(t)
	if strictFnSlotWord(r, "no-such-word-here") {
		t.Fatal("an unregistered word declares nothing")
	}
	if strictFnSlotWord(r, "sub") {
		t.Fatal("sub has no fn slot and must not read as strict")
	}
	for _, w := range []string{"usurp", "stack-args", "forward-args", "force-arity"} {
		if !strictFnSlotWord(r, w) {
			t.Errorf("%s: value form must declare CompileFnHandlerStrict", w)
		}
	}
}

func TestRecordGradualWrapHonoursStrictSlot(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	typed := NewCarrier(TFunction)
	if typed.Dynamic {
		t.Fatal("fixture: a typed carrier is not dynamic")
	}
	out := []Value{NewDynamicCarrier(TFunction)}
	// A typed Function carrier at a strict slot: declined before the
	// recorder is consulted (no poly record, no panic).
	recordGradualWrap(r, "usurp", []Value{typed}, out)
	recordGradualWrap(r, "force-arity", []Value{NewInteger(2), typed}, out)
	// The two gradual carriers that keep their poly record: the dynamic
	// Function carrier a sibling wrap produced, and a dynamic-Any read.
	recordGradualWrap(r, "usurp", []Value{NewDynamicCarrier(TFunction)}, out)
	recordGradualWrap(r, "usurp", []Value{NewDynamicCarrier(TAny)}, out)
	// A non-strict word with a typed Function carrier records as before.
	recordGradualWrap(r, "sub", []Value{typed}, out)
}
