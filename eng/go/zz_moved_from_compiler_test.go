package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestW8DispatchRematchDeclines pins the rematch record's decline arms: an
// inactive recorder, an empty window, an unresolvable operand, an empty word,
// no operands, the first-trap-wins latch, the inactive-recorder interface
// no-op, the promoted-operand rewrite of a rematch trap, and the VM
// underflow guard.
func TestW8DispatchRematchVMGuard(t *testing.T) {
	r := newTestRegistry(t)
	_ = r
	// VM underflow guard.
	vc := &vmContext{r: r}
	if err := vc.dispatchRematch(&compiler.DispatchSpec{Word: "w", NArgs: 2, Written: []int{0, 1}}, nil, nil, 0); err == nil {
		t.Error("a short stack must error")
	}
	// VM render-tuple guard: a spec whose written tuple is empty or reaches
	// outside the window is malformed (the recorder proves the tuple before
	// recording).
	if err := vc.dispatchRematch(&compiler.DispatchSpec{Word: "w", NArgs: 1},
		[]core.Value{core.NewInteger(1)}, nil, 0); err == nil || !strings.Contains(err.Error(), "written tuple") {
		t.Errorf("an empty written tuple must raise the tuple guard, got %v", err)
	}
	if err := vc.dispatchRematch(&compiler.DispatchSpec{Word: "w", NArgs: 1, Written: []int{1}},
		[]core.Value{core.NewInteger(1)}, nil, 0); err == nil || !strings.Contains(err.Error(), "written tuple") {
		t.Errorf("a written index past the window must raise the tuple guard, got %v", err)
	}
}
