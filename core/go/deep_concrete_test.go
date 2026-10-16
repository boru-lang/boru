package core

import "testing"

// DeepConcrete is the gate a check-mode model must use when it ROUTES on a
// container's interior. IsConcrete is shallow, so these tests are mostly
// about the gap between the two: values IsConcrete accepts and DeepConcrete
// must reject.

func TestDeepConcreteScalars(t *testing.T) {
	for _, v := range []Value{
		NewInteger(1), NewString("s"), NewBoolean(true), NewAtom("a"), NewNone(),
	} {
		if !DeepConcrete(v) {
			t.Errorf("%s is concrete and has no interior — it must be deep-concrete", v.Parent())
		}
	}
	// A carrier is not concrete at any depth.
	if DeepConcrete(NewCarrier(TInteger)) {
		t.Error("a carrier is not deep-concrete")
	}
	if DeepConcrete(NewDynamicCarrier(TString)) {
		t.Error("a dynamic carrier is not deep-concrete")
	}
	if DeepConcrete(Value{}) {
		t.Error("a zero value is not deep-concrete")
	}
}

// The gap that motivated the predicate: a container whose SHELL is concrete
// while an element is not. IsConcrete says yes, DeepConcrete says no.
func TestDeepConcreteRejectsCarrierInterior(t *testing.T) {
	m := NewOrderedMap()
	m.Set("enc", NewCarrier(TString))
	optsMap := NewMap(m)
	if !IsConcrete(&optsMap) {
		t.Fatal("fixture: a map literal with a carrier field is still IsConcrete — that IS the trap")
	}
	if DeepConcrete(optsMap) {
		t.Error("a map with a carrier field must not be deep-concrete")
	}

	lst := NewList([]Value{NewInteger(1), NewCarrier(TInteger)})
	if !IsConcrete(&lst) {
		t.Fatal("fixture: a list literal with a carrier element is still IsConcrete")
	}
	if DeepConcrete(lst) {
		t.Error("a list with a carrier element must not be deep-concrete")
	}

	// Nesting: the carrier is two levels down.
	inner := NewOrderedMap()
	inner.Set("k", NewDynamicCarrier(TAny))
	outer := NewOrderedMap()
	outer.Set("tls", NewMap(inner))
	if DeepConcrete(NewMap(outer)) {
		t.Error("a nested carrier must be found")
	}

	// The positive control: the same shapes, fully concrete.
	okMap := NewOrderedMap()
	okMap.Set("enc", NewString("bytes"))
	if !DeepConcrete(NewMap(okMap)) {
		t.Error("a fully concrete map must be deep-concrete")
	}
	if !DeepConcrete(NewList([]Value{NewInteger(1), NewString("x")})) {
		t.Error("a fully concrete list must be deep-concrete")
	}
	// An empty container has no interior to disprove.
	if !DeepConcrete(NewList(nil)) || !DeepConcrete(NewMap(NewOrderedMap())) {
		t.Error("empty containers are deep-concrete")
	}
}

func TestDeepConcreteDepthCap(t *testing.T) {
	// Past the cap the answer is "cannot prove it" — false, the safe
	// direction — rather than an unbounded walk.
	v := NewInteger(1)
	for i := 0; i <= deepConcreteMaxDepth+1; i++ {
		v = NewList([]Value{v})
	}
	if DeepConcrete(v) {
		t.Error("a structure deeper than the cap must decline rather than recurse")
	}
	// A nil-backed map payload declines too (nothing to walk).
	if DeepConcrete(NewValueRaw(TMap, MapPayload{})) {
		t.Error("a nil-backed map payload is not deep-concrete")
	}
}
