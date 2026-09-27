package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The seed guard hands back the window only for keyable operands carrying
// the proven tags, or matching a word's only overload.
func TestPolySeedWindow(t *testing.T) {
	sig := &core.Signature{Args: []*core.Type{core.TInteger, core.TMap}}
	tagged := &compiler.PolyRef{Arity: 2, Seed: sig, SeedTags: []*core.Type{core.TInteger, core.TMap}}
	m := core.NewMap(nil)
	stack := []core.Value{m, core.NewInteger(1)}
	if w := polySeedWindow(tagged, stack); len(w) != 2 || w[0].Parent != core.TInteger {
		t.Fatalf("matching tags give the window in sig order, got %v", w)
	}
	for name, c := range map[string]struct {
		pr    *compiler.PolyRef
		stack []core.Value
	}{
		"no seed":       {&compiler.PolyRef{Arity: 2}, stack},
		"underflow":     {tagged, stack[:1]},
		"tag count":     {&compiler.PolyRef{Arity: 2, Seed: sig, SeedTags: []*core.Type{core.TInteger}}, stack},
		"other tag":     {tagged, []core.Value{m, core.NewString("s")}},
		"carrier":       {tagged, []core.Value{m, core.NewCarrier(core.TInteger)}},
		"sole no-match": {&compiler.PolyRef{Arity: 2, Seed: sig}, []core.Value{core.NewString("s"), core.NewInteger(1)}},
	} {
		if w := polySeedWindow(c.pr, c.stack); w != nil {
			t.Errorf("%s: want no window, got %v", name, w)
		}
	}
	if w := polySeedWindow(&compiler.PolyRef{Arity: 2, Seed: sig}, stack); w == nil {
		t.Error("the only overload matching positionally gives the window")
	}
}

// A shape guard holds for any Map of its exact shape; a fn guard for its fn.
func TestSpecGuardsHoldShape(t *testing.T) {
	mk := func(v core.Value) core.Value {
		m := core.NewOrderedMap()
		m.Set("a", v)
		return core.NewMap(m)
	}
	shape, ok := core.ShapeOf(mk(core.NewInteger(1)), 8)
	if !ok {
		t.Fatal("shape")
	}
	guards := []compiler.SpecGuard{{Param: 0, Fn: shape}}
	if !specGuardsHold(guards, []core.Value{mk(core.NewInteger(9))}) {
		t.Error("the same shape holds")
	}
	if specGuardsHold(guards, []core.Value{mk(core.NewString("x"))}) {
		t.Error("another tag fails the guard")
	}
}
