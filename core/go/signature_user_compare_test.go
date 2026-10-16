package core

import "testing"

// TestCompareSignaturesUserComparerOnPatterns verifies that a user-
// defined Comparer attached to a Type drives the reversed sig sort
// when sig Patterns are concrete instances of that Type.
//
// The probe type "Inverted" wraps an Integer payload but its Comparer
// flips the natural order — given two Inverted values, smaller wraps
// "greater". CompareSignatures consults the LCA Comparer when
// comparing the per-slot pattern Values (sigSlotValue), so the sigs
// must end up sorted by the inverted scheme, reversed once more by
// CompareSignatures' own outer reversal.
//
// Outcome:
//   - Default Number comparer: 5 < 10, reversed → sig[10] before sig[5].
//   - Inverted (here):         5 > 10, reversed → sig[5]  before sig[10].
//
// Seeing sig[5] first is the proof that the user Comparer is being
// consulted on the Pattern values.
func TestCompareSignaturesUserComparerOnPatterns(t *testing.T) {
	tt := NewDynamicTypeTable()
	inverted := tt.MintTypeWithBehavior("Inverted", TInteger, invertedNumberBehavior{})

	p5 := Value{d: &Datum{Parent: inverted, Data: IntPayload{N: 5}}}
	p10 := Value{d: &Datum{Parent: inverted, Data: IntPayload{N: 10}}}

	sigs := []Signature{
		{Params: []FnParam{{Type: inverted, Pattern: &p10}}, BarrierPos: -1},
		{Params: []FnParam{{Type: inverted, Pattern: &p5}}, BarrierPos: -1},
	}
	SortSignatures(sigs)

	got0, _ := AsInteger(*sigs[0].Params[0].Pattern)
	got1, _ := AsInteger(*sigs[1].Params[0].Pattern)
	if got0 != 5 || got1 != 10 {
		t.Errorf("inverted Comparer not consulted: got [%d, %d], want [5, 10]", got0, got1)
	}
}

// TestCompareSignaturesDefaultComparerOnPatterns is the control case
// for TestCompareSignaturesUserComparerOnPatterns — with the kernel
// Number Comparer in play, the larger Integer pattern wins the
// reversed sort.
func TestCompareSignaturesDefaultComparerOnPatterns(t *testing.T) {
	pat5, pat10 := NewInteger(5), NewInteger(10)
	sigs := []Signature{
		{Params: []FnParam{{Type: TInteger, Pattern: &pat5}}, BarrierPos: -1},
		{Params: []FnParam{{Type: TInteger, Pattern: &pat10}}, BarrierPos: -1},
	}
	SortSignatures(sigs)

	got0, _ := AsInteger(*sigs[0].Params[0].Pattern)
	got1, _ := AsInteger(*sigs[1].Params[0].Pattern)
	if got0 != 10 || got1 != 5 {
		t.Errorf("default Number Comparer didn't drive sort: got [%d, %d], want [10, 5]", got0, got1)
	}
}

// invertedNumberBehavior implements Comparer with the order of the
// kernel Number Comparer flipped. It opts out for type literals (no
// concrete payload) by deferring to the lattice fallback — only
// concrete-value pairs are inverted.
type invertedNumberBehavior struct{ defaultBehavior }

func (invertedNumberBehavior) formatDelegate() {}

func (invertedNumberBehavior) Compare(a, b Value) (int, error) {
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if IsBareTypeNode(&a) && IsBareTypeNode(&b) {
		return litVsLitOrder(a, b), nil
	}
	an, _ := AsInteger(a)
	bn, _ := AsInteger(b)
	switch {
	case bn < an:
		return -1, nil
	case bn > an:
		return 1, nil
	default:
		return 0, nil
	}
}
