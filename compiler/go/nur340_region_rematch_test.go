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
// declines on the seat's own discipline, TestLowerTrapVariadicRegionNotOnTop).
func TestLowerTrapWrittenRegionDeclines(t *testing.T) {
	for _, c := range []struct {
		nFwd int
		want string
	}{
		{2, "rematch operands include a variadic loop result"},
		{1, "stack discipline: variadic rematch region is not on top"},
	} {
		es := NewEmitState()
		cf := &CompiledFn{}
		lw := &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
			sigIdx: map[*core.Signature]int{}, variadic: map[int]bool{7: true}, promoted: map[int]int{}}
		ev := EmitEvent{kind: evTrap, trap: EmitTrap{
			rematchWord:    "each",
			rematchOps:     []EmitOperand{EventOperand(7, 0), ConstOperand(0)},
			rematchWritten: []int{0, 1},
			rematchNFwd:    c.nFwd,
		}}
		if reason := lw.lowerTrap(&ev); reason != c.want {
			t.Errorf("nFwd=%d: reason = %q, want %q", c.nFwd, reason, c.want)
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
