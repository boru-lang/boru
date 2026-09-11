package compiler

import "testing"

// opsHaveVariadicResult is the screen a fn RET needs and the program
// residual does not: an event whose RESULT COUNT is runtime-variable cannot
// be spilled to one frame local, because the slot would hold one value for a
// run of a different length. The program residual absorbs such an event; a
// RET does not, so reconcileResults asks this before taking the rebuild.
func TestOpsHaveVariadicResult(t *testing.T) {
	bare := &lowerer{}
	if bare.opsHaveVariadicResult([]EmitOperand{EventOperand(1, 0)}) {
		t.Fatal("a lowerer with no EmitState can screen nothing; it must not claim a variadic")
	}
	es := NewEmitState()
	es.eventInfo = map[int]eventFlags{
		1: {},
		2: {variadicResult: true},
	}
	lw := &lowerer{es: es}
	if lw.opsHaveVariadicResult([]EmitOperand{EventOperand(1, 0), ConstOperand(0)}) {
		t.Fatal("a fixed-count event and a const are not variadic")
	}
	if !lw.opsHaveVariadicResult([]EmitOperand{EventOperand(1, 0), EventOperand(2, 0)}) {
		t.Fatal("an event marked variadicResult must be found wherever it sits")
	}
}
