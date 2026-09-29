package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// NUR340: a region WRITTEN after the word — a paren group over a loop, `each
// (for 2 [1]) [x/u]` — is spread into the call's arguments by the
// interpreter, so the region-top rematch seat (push the const, swap) is no
// window of it. The trap falls to layoutOperands, which declines the variadic
// operand; the region BENEATH the word keeps the seat (the empty sim then
// declines on the seat's own discipline, TestLowerTrapVariadicRegionNotOnTop)
// — but only a region proven non-empty: one that may leave nothing (`if c []
// [1] each [x/u]`) has no top to swap under, and declines too (the Codex
// review of #521: a SWAP underflow compiled for each's signature_error).
func TestLowerTrapWrittenRegionDeclines(t *testing.T) {
	for _, c := range []struct {
		nFwd     int
		nonEmpty bool
		want     string
	}{
		{2, true, "rematch operands include a variadic loop result"},
		{1, true, "stack discipline: variadic rematch region is not on top"},
		{1, false, "rematch operands include a variadic loop result"},
	} {
		es := NewEmitState()
		cf := &CompiledFn{}
		lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
			sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{7: true}, nonEmpty: map[int]bool{7: c.nonEmpty}, promoted: map[int]int{}}
		ev := EmitEvent{kind: evTrap, trap: EmitTrap{
			rematchWord:    "each",
			rematchOps:     []EmitOperand{EventOperand(7, 0), ConstOperand(0)},
			rematchWritten: []int{0, 1},
			rematchNFwd:    c.nFwd,
		}}
		if reason := lw.lowerTrap(&ev); reason != c.want {
			t.Errorf("nFwd=%d nonEmpty=%v: reason = %q, want %q", c.nFwd, c.nonEmpty, reason, c.want)
		}
	}
}

// armNonEmpty: an arm reaches its merge with a value when it nets one that is
// a const, a fixed-count event, or a region itself proven non-empty; never
// when it nets nothing, nor when its value is a region that may be empty.
func TestArmNonEmpty(t *testing.T) {
	lw := &lowerer{variadic: map[int]bool{3: true, 4: true}, nonEmpty: map[int]bool{4: true}}
	for _, c := range []struct {
		hasOut bool
		out    EmitOperand
		want   bool
	}{
		{false, ConstOperand(0), false},
		{true, ConstOperand(0), true},
		{true, EventOperand(2, 0), true},
		{true, EventOperand(3, 0), false},
		{true, EventOperand(4, 0), true},
	} {
		if got := lw.armNonEmpty(c.hasOut, c.out); got != c.want {
			t.Errorf("armNonEmpty(%v, %+v) = %v, want %v", c.hasOut, c.out, got, c.want)
		}
	}
}

// RegionResult is the recorder seam core's optimisticOuter asks (NUR340):
// true exactly for a value a runtime-counted region produced — a
// value-producing loop or count-varying branch (variadicResult), a variadic
// native region (variadicRegion) — and false for any other produced value, an
// unproduced one, an empty id and a nil recorder.
func TestRegionResultReadsTheRegionMarks(t *testing.T) {
	es := NewEmitState()
	loop, native, plain := core.NewCarrier(core.TList), core.NewCarrier(core.TAny), core.NewCarrier(core.TInteger)
	loop.ID, native.ID, plain.ID = "loop", "native", "plain"
	es.setProduced(loop, 1)
	es.setProduced(native, 2)
	es.setProduced(plain, 3)
	es.eventInfo[1] = eventFlags{variadicResult: true}
	es.eventInfo[2] = eventFlags{variadicRegion: true}
	for _, c := range []struct {
		id   string
		want bool
	}{{"loop", true}, {"native", true}, {"plain", false}, {"unproduced", false}, {"", false}} {
		if got := es.RegionResult(c.id); got != c.want {
			t.Errorf("RegionResult(%q) = %v, want %v", c.id, got, c.want)
		}
	}
	var nilES *EmitState
	if nilES.RegionResult("loop") {
		t.Error("a nil recorder vouches for no region")
	}
}
