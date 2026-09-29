package eng

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestPlacedApplyNeverRestarts: a shaped paren apply whose lead RAN and
// returned results that miss the claimed count never re-runs its statement's
// island, even when the results look like the paren's placement (the lead,
// then its values): the apply already ran, and a run value carries no
// identity to tell "took none of them" from "ran and returned them" — the
// re-run could repeat the statement's effects (the Codex review of #520). It
// is the host-contract violation, island or none; only a lead that is not
// appliable at all (never ran) takes the island.
func TestPlacedApplyNeverRestarts(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	seven := core.NewInteger(7)
	seven.ID = "seven"
	var lead core.Value
	placing := func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return []core.Value{lead, args[0]}, nil
	}
	lead = scFn("placer", 1, placing)
	lead.ID = "lead"
	stack := []core.Value{seven, lead}
	island := []core.Value{core.NewInteger(9)}
	spec := func() *compiler.DynMethodSpec {
		return &compiler.DynMethodSpec{Word: "f", NArgs: 1, NOut: 1, Paren: true, Restart: true, Root: true, RetPC: 2, Island: island}
	}

	if res, _, err := vc.callDynMethod(r, spec(), 0, stack, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "host-registered shape claim") {
		t.Errorf("a lead that ran and returned its placement is the count violation, never the island: %v %v", res, err)
	}

	s := spec()
	s.Restart = false
	if _, _, err := vc.callDynMethod(r, s, 0, stack, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "host-registered shape claim") {
		t.Errorf("a placing apply with no island is the count violation: %v", err)
	}

	eight := core.NewInteger(8)
	lead = scFn("placer", 1, func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return []core.Value{lead, eight}, nil
	})
	lead.ID = "lead"
	if _, _, err := vc.callDynMethod(r, spec(), 0, []core.Value{seven, lead}, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "host-registered shape claim") {
		t.Errorf("an apply that ran and missed the count is the violation, island or none: %v", err)
	}
}
