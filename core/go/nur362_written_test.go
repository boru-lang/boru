package core

import "testing"

// TestBarrierBars pins NUR362's predicate: an overload bars a window of
// nFwd written operands when its barrier stops its forward collection short
// of them and it then reads the stack — never an all-forward overload, a
// fallback, or one whose barrier reaches every written operand.
func TestBarrierBars(t *testing.T) {
	key := Signature{Args: []*Type{TString, TMap}, BarrierPos: 1}
	all := Signature{Args: []*Type{TString, TMap}, BarrierPos: 2}
	open := Signature{Args: []*Type{TString, TMap}, BarrierPos: -1}
	one := Signature{Args: []*Type{TString}, BarrierPos: 1}
	fb := Signature{Args: []*Type{TString, TMap}, BarrierPos: 0, Fallback: true}
	for _, tc := range []struct {
		sigs []Signature
		nFwd int
		want bool
	}{
		{[]Signature{key}, 2, true},
		{[]Signature{all, key}, 2, true},
		{[]Signature{key}, 1, false},
		{[]Signature{all}, 2, false},
		{[]Signature{open}, 2, false},
		{[]Signature{one}, 2, false},
		{[]Signature{fb}, 1, false},
		{nil, 2, false},
	} {
		if got := BarrierBars(tc.sigs, tc.nFwd); got != tc.want {
			t.Errorf("BarrierBars(%v, %d) = %v, want %v", tc.sigs, tc.nFwd, got, tc.want)
		}
	}
}
