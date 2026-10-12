package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// activeEmit is TheInactiveEmit with Active() flipped on — enough to walk
// parenPlacedFnCarrier PAST its recorder guard and onto the id guard, which
// is the only way to reach that arm from a package that cannot import the
// compiler (check sits below it). Every other method delegates to the
// embedded no-op, so nothing else in the hook can misbehave.
type activeEmit struct{ core.EmitRecorder }

func (activeEmit) Active() bool { return true }

// TestParenPlacedFnCarrierGuards covers parenPlacedFnCarrier's defensive
// declines. The hook is asked by fnReturnPark on EVERY user-paren collapse
// (NUR073's BROAD park), including passes with no registry at all, passes
// with no live recorder, and values carrying no id, so each guard is a live
// path rather than a formality.
func TestParenPlacedFnCarrierGuards(t *testing.T) {
	// No registry: the hook is reached through a core seam, so it cannot
	// assume the engine it is handed is a fully-armed one.
	if parenPlacedFnCarrier(&core.Engine{}, 0) {
		t.Error("an engine with no registry must decline")
	}
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	e := core.NewTop(reg)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(1)}, 4)
	if parenPlacedFnCarrier(e, 0) {
		t.Error("a pass with no live recorder must decline")
	}
	// Live recorder, id-less value: a freshly built literal carries no id,
	// so the side table cannot be keyed on it.
	reg.Check.Emit = activeEmit{core.TheInactiveEmit}
	if v := e.Tape.At(0); core.IDOf(&v) != "" {
		t.Fatalf("fixture drifted: expected an id-less literal, got %q", core.IDOf(&v))
	}
	if parenPlacedFnCarrier(e, 0) {
		t.Error("an id-less value must decline — there is nothing to look up")
	}
}
