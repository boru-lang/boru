package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// residual_rebuild_test.go pins the SEAM of the program-residual rebuild
// (the forty-third increment): the four shapes it stands aside for, and the
// code it emits when it does not.
//
// The declines are the half no whole-program row can show. Three of them are
// preconditions of the spill itself — there is nothing to spill, the value is
// not on the simulated stack, or the "value" is a runtime-variadic run rather
// than one stack entry — and the fourth is an already-emitted OpStackMark
// that indexes the very stack a spill would empty. Each is dialled directly
// here, the way the VM's own mark-op tests dial their counts.
//
// lang/go/residual_rebuild_test.go is the whole-program half.

// rbLowerer builds a lowerer over a simulated stack of single-result event
// slots — seq 1, 2, 3, … bottom-first — with somewhere to emit.
func rbLowerer(seqs ...int) *lowerer {
	lw := &lowerer{p: &Program{}, variadic: map[int]bool{}}
	lw.code = &lw.p.Code
	lw.debug = &lw.p.Debug
	for _, s := range seqs {
		lw.vm = append(lw.vm, vmSlot{seq: s, idx: 0})
	}
	return lw
}

func TestResidualRebuildSeatsAPermutation(t *testing.T) {
	// The frontier row's shape: two call results, residual reversed.
	lw := rbLowerer(1, 2)
	if !lw.seatResidualRebuild([]EmitOperand{EventOperand(2, 0), EventOperand(1, 0)}, core.SrcPos{}) {
		t.Fatal("a permutation of the simulated stack must rebuild")
	}
	// Two spills (top-down: seq 2 → l0, seq 1 → l1), then the residual
	// pushed bottom-first: seq 2 (l0), seq 1 (l1).
	want := []Instr{
		{Op: OpStoreLocal, Arg: 0},
		{Op: OpStoreLocal, Arg: 1},
		{Op: OpPushLocal, Arg: 0},
		{Op: OpPushLocal, Arg: 1},
	}
	if !instrsEqual(lw.p.Code, want) {
		t.Errorf("code = %v, want %v", lw.p.Code, want)
	}
	if len(lw.vm) != 2 {
		t.Errorf("simulated stack = %v, want the two re-pushed values", lw.vm)
	}
	if lw.numLocals != 2 {
		t.Errorf("numLocals = %d, want the 2 spill temps counted", lw.numLocals)
	}
}

// A residual naming ONE call result twice (`0 pick`) reads a single spill
// temp twice — the reason the spill map keeps the first temp per slot rather
// than allocating per residual entry.
func TestResidualRebuildDuplicatesFromOneTemp(t *testing.T) {
	lw := rbLowerer(1)
	if !lw.seatResidualRebuild([]EmitOperand{EventOperand(1, 0), EventOperand(1, 0)}, core.SrcPos{}) {
		t.Fatal("a duplicated call result must rebuild")
	}
	want := []Instr{
		{Op: OpStoreLocal, Arg: 0},
		{Op: OpPushLocal, Arg: 0},
		{Op: OpPushLocal, Arg: 0},
	}
	if !instrsEqual(lw.p.Code, want) {
		t.Errorf("code = %v, want %v", lw.p.Code, want)
	}
	if lw.numLocals != 1 {
		t.Errorf("numLocals = %d, want one temp for the one value", lw.numLocals)
	}
}

// A simulated-stack entry NO residual operand names is spilled and simply not
// pushed back — the `drop` shape. Spilling it is what makes the stack empty
// before the rebuild pushes, so the residual is exactly what comes back.
func TestResidualRebuildDropsAnUnnamedResult(t *testing.T) {
	lw := rbLowerer(1, 2)
	if !lw.seatResidualRebuild([]EmitOperand{EventOperand(1, 0)}, core.SrcPos{}) {
		t.Fatal("a residual that drops a result must rebuild")
	}
	want := []Instr{
		{Op: OpStoreLocal, Arg: 0},
		{Op: OpStoreLocal, Arg: 1},
		{Op: OpPushLocal, Arg: 1},
	}
	if !instrsEqual(lw.p.Code, want) {
		t.Errorf("code = %v, want %v", lw.p.Code, want)
	}
}

// TestSeatProgramResidualScreensACallable pins the CALLER's screen — the
// one that is about semantics rather than the spill's mechanics. A residual
// that may carry a Function never reaches the rebuild, because a re-push is
// a DATA push where the interpreter re-steps the value (NUR131).
func TestSeatProgramResidualScreensACallable(t *testing.T) {
	// The shape the rebuild would otherwise take: an inert operand seated
	// beneath a call result.
	ops := []EmitOperand{ConstOperand(0), EventOperand(1, 0)}
	plain := []core.Value{core.NewInteger(9), core.NewCarrier(core.TInteger)}
	callable := []core.Value{core.NewInteger(9), core.NewCarrier(core.TFunction)}

	lw := rbLowerer(1)
	lw.p.Consts = []core.Value{core.NewInteger(9)}
	if reason := lw.seatProgramResidual(ops, plain, core.SrcPos{}); reason != "" {
		t.Fatalf("a non-callable residual must rebuild, got %q", reason)
	}

	lw = rbLowerer(1)
	lw.p.Consts = []core.Value{core.NewInteger(9)}
	reason := lw.seatProgramResidual(ops, callable, core.SrcPos{})
	if reason == "" {
		t.Fatal("a residual that may carry a callable must keep the seating's refusal")
	}
	if len(lw.p.Code) != 0 {
		t.Errorf("the screen must emit nothing, got %v", lw.p.Code)
	}
	// A DYNAMIC entry counts as possibly-callable too: the model does not
	// bound it, and the screen is deliberately wide here (NUR129's trade).
	dyn := core.NewCarrier(core.TInteger)
	dyn.Dynamic = true
	lw = rbLowerer(1)
	lw.p.Consts = []core.Value{core.NewInteger(9)}
	if lw.seatProgramResidual(ops, []core.Value{core.NewInteger(9), dyn}, core.SrcPos{}) == "" {
		t.Error("a dynamic residual entry must keep the refusal")
	}
}

func TestResidualRebuildDeclines(t *testing.T) {
	// Nothing on the simulated stack: there is no call result to seat, so
	// the in-place seating owns the shape (a const-only residual) and its
	// refusal is the honest one.
	if rbLowerer().seatResidualRebuild([]EmitOperand{ConstOperand(0)}, core.SrcPos{}) {
		t.Error("an empty simulated stack must decline")
	}
	// An operand whose event left NOTHING on the simulated stack: there is
	// no value to spill for it, so the rebuild cannot produce it.
	lw := rbLowerer(1)
	if lw.seatResidualRebuild([]EmitOperand{EventOperand(9, 0)}, core.SrcPos{}) {
		t.Error("an operand absent from the simulated stack must decline")
	}
	// The SAME event, a different result index: a multi-result call's other
	// slot is a different value and is equally absent.
	if lw.seatResidualRebuild([]EmitOperand{EventOperand(1, 1)}, core.SrcPos{}) {
		t.Error("a result index absent from the simulated stack must decline")
	}
	// A runtime-variadic REGION operand: its run is not one spillable stack
	// entry — OpStoreLocal would take the run's last value and leave the
	// rest — so the count is not the static seat and the rebuild stands
	// aside exactly as the region plans do.
	lw = rbLowerer(1, 2)
	lw.variadic[2] = true
	if lw.seatResidualRebuild([]EmitOperand{EventOperand(2, 0), EventOperand(1, 0)}, core.SrcPos{}) {
		t.Error("a variadic region operand must decline")
	}
	// An armed mark plan. Each of the three opens an OpStackMark BEFORE the
	// residual is seated, and a mark indexes a stack depth — emptying the
	// stack under it would leave the mark pointing past the top.
	ops := []EmitOperand{EventOperand(2, 0), EventOperand(1, 0)}
	for _, c := range []struct {
		name string
		arm  func(*lowerer)
	}{
		{"mark window", func(l *lowerer) { l.markBefore = map[int]bool{1: true} }},
		{"region prefix", func(l *lowerer) { l.regionPrefixSeq = 1 }},
		{"region collect", func(l *lowerer) { l.collectAtSeq = 1 }},
	} {
		t.Run(c.name, func(t *testing.T) {
			lw := rbLowerer(1, 2)
			c.arm(lw)
			if lw.seatResidualRebuild(ops, core.SrcPos{}) {
				t.Error("an armed mark plan must decline")
			}
			if len(lw.p.Code) != 0 {
				t.Errorf("a decline must emit nothing, got %v", lw.p.Code)
			}
		})
	}
}

func instrsEqual(got, want []Instr) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].Op != want[i].Op || got[i].Arg != want[i].Arg {
			return false
		}
	}
	return true
}
