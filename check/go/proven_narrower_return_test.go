package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// A body residual proving an exact leaf scalar under a declared `Any`
// surfaces that type; anything else keeps the declaration's rules.
func TestProvenNarrowerReturn(t *testing.T) {
	ints := core.NewCarrier(core.TInteger)
	typed := []core.FnParam{{Name: "x", Type: core.TInteger}}
	if _, ok := provenNarrowerReturn(core.TAny, []core.FnParam{{Name: "x", Type: core.TAny}}, []core.Value{ints}, 1, 0); ok {
		t.Error("an Any-param fn's result stays gradual")
	}
	if c, ok := provenNarrowerReturn(core.TAny, typed, []core.Value{ints}, 1, 0); !ok || !c.Parent().Equal(core.TInteger) || core.IsDynamic(&c) {
		t.Errorf("an Integer carrier under [Any] narrows, got %v %v", c, ok)
	}
	for name, c := range map[string]struct {
		t   *core.Type
		stk []core.Value
	}{
		"count miss":  {core.TAny, []core.Value{ints, ints}},
		"Number decl": {core.TNumber, []core.Value{ints}},
		"concrete":    {core.TAny, []core.Value{core.NewInteger(1)}},
		"dynamic":     {core.TAny, []core.Value{core.NewDynamicCarrier(core.TInteger)}},
		"upper bound": {core.TAny, []core.Value{core.NewCarrier(core.TNode)}},
		"String":      {core.TAny, []core.Value{core.NewCarrier(core.TString)}},
		"no parent":   {core.TAny, []core.Value{core.ValueOf(&core.Datum{Carrier: true})}},
		"nil decl":    {nil, []core.Value{ints}},
	} {
		if _, ok := provenNarrowerReturn(c.t, typed, c.stk, 1, 0); ok {
			t.Errorf("%s: must not narrow", name)
		}
	}
	if allParamsTyped([]core.FnParam{{Name: "x", Type: core.TAny}}) || allParamsTyped([]core.FnParam{{Name: "x"}}) {
		t.Error("an Any or untyped param is not typed")
	}
	if !allParamsTyped([]core.FnParam{{Name: "x", Type: core.TInteger}}) {
		t.Error("an Integer param is typed")
	}
	// A body holding a `do` may answer a trapped Error in the proven slot.
	do := core.NewWord("do")
	if !bodyTrapsErrors([]core.Value{core.NewList([]core.Value{do})}) || bodyTrapsErrors([]core.Value{core.NewInteger(1)}) {
		t.Error("a body's do traps; a body without one does not")
	}
	reg, _ := core.NewRegistry()
	if _, ok := refinedDeclaredReturn(reg, core.TAny, typed, nil, 0, 1, []core.Value{ints}, true); ok {
		t.Error("a trapping body's residual narrows nothing")
	}
	if _, ok := refinedDeclaredReturn(reg, core.TAny, typed, nil, 0, 1, []core.Value{ints}, false); !ok {
		t.Error("a plain body's Integer residual narrows")
	}
}
