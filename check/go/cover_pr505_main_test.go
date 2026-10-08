package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// cover_pr505_main_test.go pins the forward walk of
// checkModeFallbackPositionsFor (NUR180's per-signature recovery window) on
// the token shapes the plain gatherer's walk already skips, and the
// shortfall fill ending exactly at the window's size.

// TestCoverPR505MainFallbackForwardWalkSkipsGroupsAndMarkers: the forward
// walk ENTERS a nested group (its open raises the depth, its close lowers it
// without stopping the walk — only the enclosing group's close does) and
// steps over the tape's marker tokens (a Mark, a Forward), taking the
// compatible tokens around them in source order, up to the forward limit.
func TestCoverPR505MainFallbackForwardWalkSkipsGroupsAndMarkers(t *testing.T) {
	two := &core.Signature{Args: []*core.Type{core.TNumber, core.TNumber}, BarrierPos: core.BarrierAllForward}
	// tape: [zzw ( 1 ) Mark Forward 2], pointer on zzw
	e := engWithTape(t, []core.Value{
		core.NewWord("zzw"),
		core.NewOpenParen(), core.NewInteger(1), core.NewCloseParen(),
		core.NewMark("zzm"), core.NewForward(core.ForwardInfo{}),
		core.NewInteger(2),
	}, 0)
	pos, nStack := checkModeFallbackPositionsFor(e, two, core.WordInfo{Name: "zzw"})
	if len(pos) != 2 || pos[0] != 2 || pos[1] != 6 || nStack != 0 {
		t.Errorf("positions = %v nStack = %d, want [2 6] 0 (the group's token, then the one past the markers)", pos, nStack)
	}
	// The ENCLOSING group's close is a hard stop: [zzw 1 ) 2] takes only the 1,
	// and the stack beneath is empty, so the window stays short.
	e = engWithTape(t, []core.Value{
		core.NewWord("zzw"), core.NewInteger(1), core.NewCloseParen(), core.NewInteger(2),
	}, 0)
	pos, nStack = checkModeFallbackPositionsFor(e, two, core.WordInfo{Name: "zzw"})
	if len(pos) != 1 || pos[0] != 1 || nStack != 0 {
		t.Errorf("enclosing close: positions = %v nStack = %d, want [1] 0", pos, nStack)
	}
}

// TestCoverPR505MainFallbackShortfallStopsWhenFull: a forward run cut short
// by an incompatible token, over a stack too shallow for the remainder,
// fills the shortfall from the tokens after the run and ends exactly at the
// window's size, leaving the rest of the tape alone. [1 zzw 10 "s" "t"]
// under three Numbers: the written 10 fills sig[0], the stack's 1 the next,
// and the first token past the run the last; "t" is never taken. This is
// the sizing the shortfall loop's window-size bound (a //covergate:allow
// guard) relies on: the plain gatherer's walk hands back no more tokens than
// the window lacks, so the fill ends on its last entry and the bound never
// fires.
func TestCoverPR505MainFallbackShortfallStopsWhenFull(t *testing.T) {
	three := &core.Signature{Args: []*core.Type{core.TNumber, core.TNumber, core.TNumber}, BarrierPos: core.BarrierAllForward}
	e := engWithTape(t, []core.Value{
		core.NewInteger(1), core.NewWord("zzw"), core.NewInteger(10), core.NewString("s"), core.NewString("t"),
	}, 1)
	pos, nStack := checkModeFallbackPositionsFor(e, three, core.WordInfo{Name: "zzw"})
	if len(pos) != 3 || pos[0] != 0 || pos[1] != 2 || pos[2] != 3 || nStack != 1 {
		t.Errorf("positions = %v nStack = %d, want [0 2 3] 1 (full at three, the trailing token left)", pos, nStack)
	}
}
