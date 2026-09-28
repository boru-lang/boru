package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// ReturnsNumericBinary's Integer result needs BOTH numeric operands to be
// Integers; any other numeric pair may carry a Float leaf at run time and
// types as a Number — strict over strictly-numeric operands, dynamic when an
// operand is gradual (2026-09-27; it was a blanket Integer default).
func TestReturnsNumericBinaryNumberResult(t *testing.T) {
	nb := ReturnsNumericBinary()
	integer := core.NewCarrier(core.TInteger)
	for _, c := range []struct {
		name    string
		a, b    core.Value
		want    *core.Type
		dynamic bool
	}{
		// Positive: both Integers stay Integer — including a concrete leaf
		// and a GRADUAL Integer, whose bound is still an Integer.
		{"int⊕int", integer, integer, core.TInteger, false},
		{"concrete int⊕int", core.NewInteger(2), integer, core.TInteger, false},
		{"dynamic(int)⊕int", core.NewDynamicCarrier(core.TInteger), integer, core.TInteger, false},
		// The fold-accumulator shape: a Number element (the join of
		// Integer and Float) summed with an Integer seed.
		{"number⊕int", core.NewCarrier(core.TNumber), integer, core.TNumber, false},
		{"int⊕number", integer, core.NewCarrier(core.TNumber), core.TNumber, false},
		// Gradual operands keep the optimistic modality.
		{"dynamic(any)⊕int", core.NewDynamicCarrier(core.TAny), integer, core.TNumber, true},
		{"int⊕dynamic(number)", integer, core.NewDynamicCarrier(core.TNumber), core.TNumber, true},
		// A STRICT non-Number operand only reaches here through the
		// no-match recovery, which re-matches at run time; it keeps the
		// historical Integer model (see the ReturnsNumericBinary note).
		{"strict any⊕int", core.NewCarrier(core.TAny), integer, core.TInteger, false},
		{"int⊕strict any", integer, core.NewCarrier(core.TAny), core.TInteger, false},
		{"strict any⊕dynamic(any)", core.NewCarrier(core.TAny), core.NewDynamicCarrier(core.TAny), core.TNumber, true},
	} {
		out := nb([]core.Value{c.a, c.b}, nil)
		if len(out) != 1 || !out[0].Parent.Equal(c.want) || out[0].Dynamic != c.dynamic {
			t.Errorf("%s: got %v (dynamic=%v), want %s (dynamic=%v)", c.name, out, len(out) == 1 && out[0].Dynamic, c.want.Name(), c.dynamic)
		}
	}
	// Negative: a Number operand never yields an Integer, and a Float leaf
	// still wins over the Number rule.
	if out := nb([]core.Value{core.NewCarrier(core.TNumber), core.NewCarrier(core.TNumber)}, nil); out[0].Parent.Equal(core.TInteger) {
		t.Errorf("number⊕number must not type as Integer: %v", out)
	}
	if out := nb([]core.Value{core.NewCarrier(core.TNumber), core.NewCarrier(core.TFloat)}, nil); !out[0].Parent.Equal(core.TFloat) {
		t.Errorf("number⊕float = %v, want Float", out)
	}
}
