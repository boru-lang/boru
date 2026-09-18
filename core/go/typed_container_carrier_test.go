package core

import "testing"

func TestUnifyTypedContainerCarrierGuard(t *testing.T) {
	child := NewTypeLiteral(TInteger)
	lc := NewCarrier(TFlexList)
	out, uerr := unifyTypedListWithConcrete(lc, child, nil)
	if uerr != nil {
		t.Fatalf("list carrier: %v", uerr)
	}
	if c, ok := out.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("list carrier not tagged with [:Integer]")
	}
	mc := NewCarrier(TFlexMap)
	outm, merr := unifyTypedMapWithConcrete(mc, child, nil)
	if merr != nil {
		t.Fatalf("map carrier: %v", merr)
	}
	if c, ok := outm.ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("map carrier not tagged with {:Integer}")
	}
}
