package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The seed stands only for the live overload with its implementation, once
// a window passing its guard has validated it against the live aggregate;
// another aggregate (a redefinition) revalidates, and an aggregate without
// the seed's implementation never takes it.
func TestPolySeeded(t *testing.T) {
	impl := core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, nil
	})
	other := core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, nil
	})
	seedSig := core.Signature{Args: []*core.Type{core.TInteger, core.TMap}, Impl: impl}
	sigs := []core.Signature{{Args: []*core.Type{core.TString, core.TMap}, Impl: other}, seedSig}
	fn := &core.FnDefInfo{Name: "w", Signatures: sigs}
	tagged := &compiler.PolyRef{Arity: 2, Seed: &seedSig, SeedTags: []*core.Type{core.TInteger, core.TMap}}
	m := core.NewMap(nil)
	win := []core.Value{core.NewInteger(1), m}
	e := &polyCacheEntry{}
	if got := e.seeded(tagged, fn, fn.Signatures, win); got != &fn.Signatures[1] {
		t.Fatalf("a validated seed stands for the live overload, got %v", got)
	}
	if got := e.seeded(tagged, fn, fn.Signatures, win); got != &fn.Signatures[1] {
		t.Error("a validated seed is reused for the same aggregate")
	}
	for name, w := range map[string][]core.Value{
		"other tag": {core.NewString("s"), m}, "carrier": {core.NewCarrier(core.TInteger), m}, "count": {m},
	} {
		if e.seeded(tagged, fn, fn.Signatures, w) != nil {
			t.Errorf("%s: the guard fails", name)
		}
	}
	// A redefinition that drops the seed's implementation: never taken.
	redef := &core.FnDefInfo{Name: "w", Signatures: []core.Signature{{Args: []*core.Type{core.TInteger, core.TMap}, Impl: other}}}
	if e.seeded(tagged, redef, redef.Signatures, win) != nil {
		t.Error("an aggregate without the seed's implementation never takes it")
	}
	if e.seeded(tagged, redef, redef.Signatures, win) != nil {
		t.Error("nor on a later call over the same aggregate")
	}
	// A more specific overload installed ahead of the seed: the first match
	// is not the seed, so it does not stand.
	shadow := &core.FnDefInfo{Name: "w", Signatures: []core.Signature{{Args: []*core.Type{core.TInteger, core.TMap}, Impl: other}, seedSig}}
	if (&polyCacheEntry{}).seeded(tagged, shadow, shadow.Signatures, win) != nil {
		t.Error("a shadowing overload ahead of the seed keeps the re-match")
	}
	pat := core.NewInteger(1)
	patSigs := []core.Signature{{Params: []core.FnParam{{Type: core.TInteger, Pattern: &pat}, {Type: core.TMap}}}, seedSig}
	patFn := &core.FnDefInfo{Name: "w", Signatures: patSigs}
	if (&polyCacheEntry{}).seeded(tagged, patFn, patFn.Signatures, win) != nil {
		t.Error("a live aggregate that is not tag-determined keeps the re-match")
	}
	if (&polyCacheEntry{}).seeded(&compiler.PolyRef{Arity: 2}, fn, fn.Signatures, win) != nil || (&polyCacheEntry{}).seeded(tagged, nil, nil, win) != nil {
		t.Error("no seed, or no live aggregate: no seed pick")
	}
	// The tag-free mode: the word's only overload of the arity, guarded by
	// its positional match on every call.
	one := &core.FnDefInfo{Name: "w", Signatures: []core.Signature{seedSig}}
	sole := &compiler.PolyRef{Arity: 2, Seed: &seedSig}
	e1 := &polyCacheEntry{}
	if e1.seeded(sole, one, one.Signatures, win) != &one.Signatures[0] {
		t.Error("the only overload stands once validated")
	}
	if e1.seeded(sole, one, one.Signatures, []core.Value{core.NewString("s"), m}) != nil {
		t.Error("the only overload's positional match is the guard")
	}
	e2 := &polyCacheEntry{}
	if e2.seeded(sole, one, one.Signatures, []core.Value{core.NewString("s"), m}) != nil || e2.seeded(sole, one, one.Signatures, win) != nil {
		t.Error("a window that fails validation leaves the seed unvalidated for that aggregate")
	}
	if (&polyCacheEntry{}).seeded(sole, fn, fn.Signatures, win) != nil {
		t.Error("the tag-free seed needs the ONLY overload of the arity")
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
