package eng

import (
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	"github.com/boru-lang/boru/core/go"
)

// TestCallWindowAt pins the VM's read of a call's no-match window (NUR234):
// each operand kind from its home — the argument, the value, the caller's
// local, the stack beneath the call — in the main code's table or a unit's;
// no program, no table entry, an unknown unit or an operand the frame
// cannot reach is no window, and the contract reports the arguments.
func TestCallWindowAt(t *testing.T) {
	one, two, three, four := core.NewInteger(1), core.NewInteger(2), core.NewInteger(3), core.NewInteger(4)
	spec := []compiler.CallWindowOperand{
		{Kind: compiler.WinArg, Idx: 0},
		{Kind: compiler.WinValue, Value: four},
		{Kind: compiler.WinLocal, Idx: 1},
		{Kind: compiler.WinStack, Idx: 0},
	}
	p := &compiler.Program{
		CallWindows: map[int][]compiler.CallWindowOperand{3: spec},
		Fns:         []compiler.CompiledFn{{CallWindows: map[int][]compiler.CallWindowOperand{5: {}}}},
	}
	args, locals, stack := []core.Value{one}, []core.Value{two, three}, []core.Value{two, one}
	win, ok := callWindowAt(p, -1, 3, args, stack, locals)
	if !ok || len(win) != 4 || win[0].Data != one.Data || win[1].Data != four.Data || win[2].Data != three.Data || win[3].Data != one.Data {
		t.Fatalf("main-code window: %v %v", win, ok)
	}
	if win, ok := callWindowAt(p, 0, 5, nil, nil, nil); !ok || len(win) != 0 {
		t.Fatalf("a unit's empty window is a window: %v %v", win, ok)
	}
	for name, c := range map[string]struct {
		p    *compiler.Program
		unit int
		pc   int
		ops  []compiler.CallWindowOperand
	}{
		"no program":   {nil, -1, 3, nil},
		"no entry":     {p, -1, 4, nil},
		"unknown unit": {p, 7, 5, nil},
		"arg range":    {nil, -1, 0, []compiler.CallWindowOperand{{Kind: compiler.WinArg, Idx: 1}}},
		"local range":  {nil, -1, 0, []compiler.CallWindowOperand{{Kind: compiler.WinLocal, Idx: 2}}},
		"stack range":  {nil, -1, 0, []compiler.CallWindowOperand{{Kind: compiler.WinStack, Idx: 2}}},
	} {
		prog := c.p
		if c.ops != nil {
			prog = &compiler.Program{CallWindows: map[int][]compiler.CallWindowOperand{0: c.ops}}
		}
		if win, ok := callWindowAt(prog, c.unit, c.pc, args, stack, locals); ok {
			t.Errorf("%s: no window, got %v", name, win)
		}
	}
}
