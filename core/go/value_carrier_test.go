package core

import "testing"

// TestValueCarrier pins the widening of a value to its carrier (NUR323): a
// type literal is a TYPE value — its Parent is its supertype, not its type
// — so it widens to a Type carrier; the None literal to the None carrier;
// any other value to a carrier of its type, keeping its modality. The join
// and the typed list take a type-literal arm or element the same way.
func TestValueCarrier(t *testing.T) {
	if c := ValueCarrier(NewTypeLiteral(TInteger)); !c.Carrier || c.Dynamic || !c.Parent.Equal(TType) {
		t.Errorf("a type literal widens to a strict Type carrier, got %v", c)
	}
	if c := ValueCarrier(NewTypeLiteral(TNone)); !c.Carrier || !c.Parent.Equal(TNone) {
		t.Errorf("the None literal widens to the None carrier, got %v", c)
	}
	if c := ValueCarrier(NewInteger(5)); !c.Carrier || c.Dynamic || !c.Parent.Equal(TInteger) {
		t.Errorf("a value widens to a carrier of its type, got %v", c)
	}
	if c := ValueCarrier(NewDynamicCarrier(TString)); !c.Dynamic || !c.Parent.Equal(TString) {
		t.Errorf("a gradual value stays gradual, got %v", c)
	}
	// `if c [Integer] [5]`: a type arm joins as a Type value, never as a
	// value of Number.
	j := JoinCarriers(NewTypeLiteral(TInteger), NewInteger(5))
	for _, alt := range FlattenAlternatives(j) {
		if alt.Equal(TNumber) {
			t.Errorf("a type arm joined into Number: %v", j)
		}
	}
	if j2 := JoinCarriers(NewInteger(5), NewTypeLiteral(TInteger)); j2.Parent.Equal(TNumber) {
		t.Errorf("a type arm joined into Number: %v", j2)
	}
	tl := CarrierTypedListOf(NewTypeLiteral(TInteger))
	if ct, ok := tl.Data.(ChildTypeInfo); !ok || !ct.Child.Parent.Equal(TType) {
		t.Errorf("a list of a type value holds Type carriers, got %v", tl)
	}
}
