package core

import "testing"

// TestCarrierTypedListOf pins NUR316's typed-list carrier: a disjunct element
// stands as it is, a gradual element's type stays gradual, and any other
// element's type is strict.
func TestCarrierTypedListOf(t *testing.T) {
	dyn := NewCarrier(TInteger)
	dyn.Dynamic = true
	for _, c := range []struct {
		why     string
		v       Value
		dynamic bool
	}{
		{"a strict element", NewCarrier(TInteger), false},
		{"a gradual element", dyn, true},
	} {
		ci, err := AsChildType(CarrierTypedListOf(c.v))
		if err != nil || !ci.Child.Parent.Equal(TInteger) || ci.Child.Dynamic != c.dynamic {
			t.Errorf("%s: child %v dynamic=%v (err %v), want Integer dynamic=%v", c.why, ci.Child.Parent, ci.Child.Dynamic, err, c.dynamic)
		}
	}
	d := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if ci, err := AsChildType(CarrierTypedListOf(d)); err != nil || !IsDisjunct(ci.Child) {
		t.Errorf("a disjunct element stands as it is: %v (err %v)", ci.Child, err)
	}
}

// TestStoreShapePoison pins NUR315's poison: every keyed and unkeyed claim
// goes, later writes record nothing, a clone keeps the mark, and a nil shape
// is a no-op.
func TestStoreShapePoison(t *testing.T) {
	var none *StoreShapeInfo
	none.Poison()
	s := &StoreShapeInfo{}
	s.RecordKey("a", NewCarrier(TInteger))
	s.RecordVal(NewCarrier(TInteger))
	s.Poison()
	if _, ok := s.LookupKey("a"); ok {
		t.Error("a poisoned shape claims no key")
	}
	if _, ok := s.LookupVals(); ok {
		t.Error("a poisoned shape claims no element")
	}
	s.RecordKey("b", NewCarrier(TString))
	if _, ok := s.LookupKey("b"); ok || s.KeyTypes != nil {
		t.Error("a poisoned shape records no later key")
	}
	if cp := s.CloneShape(); !cp.KeysPoisoned || !cp.ValsPoisoned {
		t.Error("a clone keeps the poison")
	}
}
