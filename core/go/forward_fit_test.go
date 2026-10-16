package core

import (
	"testing"
)

// TestForwardValueFits pins the interpreter's forward-scan fit over a run
// value (NUR357): a form or quoted slot captures any token, an Any slot
// takes any value, a typed slot a value of its type, and a bare type node
// is refused at a concrete-payload slot unless the slot takes types.
func TestForwardValueFits(t *testing.T) {
	mapSig := &Signature{Args: []*Type{TMap}, BarrierPos: -1}
	NormalizeSig(mapSig)
	anySig := &Signature{Args: []*Type{TAny}, BarrierPos: -1}
	NormalizeSig(anySig)
	intLit := NewTypeLiteral(TInteger)
	cases := []struct {
		name string
		sig  *Signature
		k    int
		v    Value
		want bool
	}{
		{"no signature", nil, 0, NewInteger(0), false},
		{"a position past the signature", mapSig, 1, NewInteger(0), false},
		{"a negative position", mapSig, -1, NewInteger(0), false},
		{"a Map at the Map slot", mapSig, 0, NewMap(NewOrderedMap()), true},
		{"an Integer at the Map slot", mapSig, 0, NewInteger(0), false},
		{"anything at an Any slot", anySig, 0, NewInteger(0), true},
		{"a type node at a concrete slot", &Signature{Args: []*Type{TInteger}}, 0, intLit, false},
		{"a type node at a type slot", &Signature{Args: []*Type{TInteger}, TypeArgs: map[int]bool{0: true}}, 0, intLit, true},
		{"a form slot", &Signature{Args: []*Type{TMap}, FormArgs: map[int]bool{0: true}}, 0, NewInteger(0), true},
		{"a quoted slot", &Signature{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}}, 0, NewInteger(0), true},
	}
	for _, c := range cases {
		if c.sig != nil && len(c.sig.Params) == 0 {
			NormalizeSig(c.sig)
		}
		if got := ForwardValueFits(c.sig, c.k, c.v); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	fits := []ForwardFit{{Sig: mapSig, Idx: 0}, {Sig: anySig, Idx: 0}}
	if !ForwardFitsAll(nil, NewInteger(0)) || !ForwardFitsAll(fits, NewMap(NewOrderedMap())) || ForwardFitsAll(fits, NewInteger(0)) {
		t.Error("every fit must hold: none, both, one missed")
	}
}

// TestForwardFitsListsTheViableCandidates pins the fits a dispatch
// publishes for a gradual operand it collected forward (NUR357): the
// candidates whose collection, stopped at the operand, could match over the
// stack beneath — a stack-only overload never reaches the operand, and one
// needing more values than the stack holds fails whatever the run holds.
