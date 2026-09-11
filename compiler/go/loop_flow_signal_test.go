package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// loop_flow_signal_test.go pins the fiftieth increment at the seam: a
// break/continue emits the FLOW signal op whether or not the loop it targets
// lives in this unit. The bare OpJmp it used to emit reached the right pc and
// skipped the two things only the signal does — trimming the round and
// popping the loop (NUR132).

// lfsOps returns the emitted opcodes.
func lfsOps(code []Instr) []Opcode {
	out := make([]Opcode, 0, len(code))
	for _, in := range code {
		out = append(out, in.Op)
	}
	return out
}

func TestLowerBreakContinueEmitTheFlowSignal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		inLoop bool
		fnUnit bool
		lower  func(lw *lowerer, ev *EmitEvent) string
		want   Opcode
	}{
		{"break inside this unit's loop", true, false, (*lowerer).lowerBreak, OpFlowBreak},
		{"break from a fn unit", false, true, (*lowerer).lowerBreak, OpFlowBreak},
		{"continue inside this unit's loop", true, false, (*lowerer).lowerContinue, OpFlowContinue},
		{"continue from a fn unit", false, true, (*lowerer).lowerContinue, OpFlowContinue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lw := w8lw()
			lw.isFnUnit = tc.fnUnit
			if tc.inLoop {
				lw.loops = []loopCtx{{nextPC: 7}}
			}
			if reason := tc.lower(lw, &EmitEvent{kind: evBreak}); reason != "" {
				t.Fatalf("lowering refused: %q", reason)
			}
			ops := lfsOps(*lw.code)
			if len(ops) != 1 || ops[0] != tc.want {
				t.Fatalf("emitted %v, want exactly [%v]", ops, tc.want)
			}
			// No pc rides on the signal: the VM reads the open loop's own
			// exitPC / nextPC, so a hole to patch would be a second source
			// of truth for the same jump.
			if (*lw.code)[0].Arg != 0 {
				t.Errorf("the flow signal must carry no target, got Arg=%d", (*lw.code)[0].Arg)
			}
		})
	}
}

func TestLowerBreakOutsideLoop(t *testing.T) {
	lw := w8lw() // no loop context, not a fn unit
	if reason := lw.lowerBreak(&EmitEvent{kind: evBreak, call: emitCall{pos: core.SrcPos{}}}); reason == "" {
		t.Fatal("a break with no loop and no frame to unwind into must refuse")
	} else if reason != "break outside a compiled loop (Stage 2)" {
		t.Fatalf("break-outside-loop reason = %q", reason)
	}
	if len(*lw.code) != 0 {
		t.Errorf("a refused break must emit nothing, got %v", lfsOps(*lw.code))
	}
}
