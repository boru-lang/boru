package eng

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// nur336_place_test.go pins placedAsIs, the test the shaped paren apply
// (callDynMethod) runs over an apply whose results miss the claimed count:
// its results are the lead and the values after it, unchanged and in the
// paren's order, only when the lead took none of them and nothing ran — the
// interpreter's paren placed them, and the statement's island answers that
// shape (NUR336). The lang suite drives the arm end to end
// (TestNUR336PlacedParenApply).
func TestPlacedAsIs(t *testing.T) {
	val := func(id string) core.Value {
		v := core.NewInteger(1)
		v.ID = id
		return v
	}
	fn, a, b := val("fn"), val("a"), val("b")
	for _, c := range []struct {
		name    string
		results []core.Value
		want    bool
	}{
		{"the lead then the values", []core.Value{fn, a, b}, true},
		{"one value fewer", []core.Value{fn, a}, false},
		{"another lead", []core.Value{a, a, b}, false},
		{"the values out of order", []core.Value{fn, b, a}, false},
	} {
		if got := placedAsIs(c.results, fn, []core.Value{a, b}); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPlacedApplyTakesTheIsland drives the arm placedAsIs guards: a paren
// apply whose lead PLACED itself and its values — a fn that took none of
// them, so the paren holds them as data — where the program reads beneath
// what it leaves (no DynMethodSpec.Place) runs the statement's island, as a
// data lead does. An apply that ran and missed the claimed count is the
// host-contract violation it always was, island or none, and so is one that
// placed with no island planned.
//
// The island's own write of the lead is what a program meets today: the
// compiler plans the paren's lead as a placed guard write (parenLead), and a
// fn value written there would dispatch on the interpreter's step — the
// designed NUR297 defer, loud, never a wrong answer
// (`def mk fn [[] [Map] [{f: ([] => [9])}]] end def m (mk) end (m.f 7) drop`).
func TestPlacedApplyTakesTheIsland(t *testing.T) {
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

	res, _, err := vc.callDynMethod(r, spec(), 0, stack, seam7Dbg, 0)
	if err != nil || len(res) != 1 || res[0].String() != "9" {
		t.Errorf("a placing apply runs the statement's island: %v %v", res, err)
	}

	// The lead's own write, as the compiler plans it: a fn value the
	// interpreter would apply where the island writes it defers.
	s := spec()
	s.Island = []core.Value{core.NewParenExpr([]core.Value{core.NewWord("f"), seven})}
	s.Substs = []compiler.RestartSubst{{Path: []int{0, 0}, Span: 1, Src: compiler.RestartSrc{Kind: compiler.RestartGuard}, Placed: true}}
	if _, _, err := vc.callDynMethod(r, s, 0, stack, seam7Dbg, 0); err == nil || !core.IsVMDefer(err) || !strings.Contains(err.Error(), "parks it") {
		t.Errorf("the island's write of a fn lead defers: %v", err)
	}

	s = spec()
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
