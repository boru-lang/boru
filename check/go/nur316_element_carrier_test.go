package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestElementCarrierOf pins NUR316's element carrier: a typed list whose
// element is gradual hands out a gradual element, a strict one strict, and
// an untyped one the dynamic Any it always did.
func TestElementCarrierOf(t *testing.T) {
	dyn := core.NewCarrier(core.TInteger)
	dyn.Dynamic = true
	if c := ElementCarrierOf(core.CarrierTypedListOf(dyn)); !c.Parent.Equal(core.TInteger) || !c.Dynamic {
		t.Errorf("a gradual element stays gradual: %v dynamic=%v", c.Parent, c.Dynamic)
	}
	if c := ElementCarrierOf(core.NewCarrierTypedList(core.TInteger)); !c.Parent.Equal(core.TInteger) || c.Dynamic {
		t.Errorf("a strict element stays strict: %v dynamic=%v", c.Parent, c.Dynamic)
	}
	if c := ElementCarrierOf(core.NewCarrier(core.TList)); !c.Dynamic {
		t.Errorf("an untyped list's element is dynamic Any: %v dynamic=%v", c.Parent, c.Dynamic)
	}
	if c := ElementCarrierFromValue(core.CarrierTypedListOf(dyn)); !c.Dynamic {
		t.Errorf("ElementCarrierFromValue keeps the gradual element: dynamic=%v", c.Dynamic)
	}
}

// TestPoisonFlexShapes pins NUR315's walk: a shaped container and every
// shape reachable from it — through its claims, a concrete list or map
// holding one — is poisoned, once each, and anything else is left alone.
func TestPoisonFlexShapes(t *testing.T) {
	inner, _ := MintFlexListShapeCarrier(core.NewList([]core.Value{core.NewInteger(1)}), 0)
	outer, _ := MintFlexShapeCarrier(core.NewMap(core.NewOrderedMap()), 0)
	oss, _ := StoreShapeOf(outer)
	oss.RecordKey("l", inner)
	oss.RecordKey("self", outer)
	PoisonFlexShapes(outer)
	iss, _ := StoreShapeOf(inner)
	if !oss.KeysPoisoned || !iss.ValsPoisoned {
		t.Errorf("the shape and the one its claim holds are poisoned: outer=%v inner=%v", oss.KeysPoisoned, iss.ValsPoisoned)
	}
	held, _ := MintFlexListShapeCarrier(core.NewList([]core.Value{core.NewInteger(2)}), 0)
	m := core.NewOrderedMap()
	m.Set("k", held)
	PoisonFlexShapes(core.NewList([]core.Value{core.NewMap(m)}))
	if hss, _ := StoreShapeOf(held); !hss.ValsPoisoned {
		t.Error("a shape a concrete list's map holds is poisoned")
	}
	PoisonFlexShapes(core.NewInteger(3)) // nothing to poison
	deep := core.NewInteger(0)
	for i := 0; i < flexShapeMaxDepth+3; i++ {
		deep = core.NewList([]core.Value{deep})
	}
	PoisonFlexShapes(deep) // bounded
}
