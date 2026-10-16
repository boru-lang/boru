package calc

import (
	"math"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The numeric handlers' guard arms are unreachable through dispatch
// (the signatures admit only numbers), so drive them directly: a
// non-numeric payload must surface AsNumber's error, never a panic or
// a zero-value result.
func TestNumHandlerRejectsNonNumbers(t *testing.T) {
	h := numHandler(func(a, b float64) (float64, error) { return a + b, nil }, true)
	str := core.NewString("x")
	num := core.NewInteger(1)
	if _, err := h([]core.Value{str, num}, nil, nil, nil); err == nil {
		t.Error("numHandler with string arg[0]: want error, got none")
	}
	if _, err := h([]core.Value{num, str}, nil, nil, nil); err == nil {
		t.Error("numHandler with string arg[1]: want error, got none")
	}
}

// preferInt must NOT truncate to Integer when the result is
// non-finite — an Inf/NaN result stays Float even for two Integer
// inputs.
func TestNumHandlerNonFiniteStaysFloat(t *testing.T) {
	inf := numHandler(func(a, b float64) (float64, error) { return math.Inf(1), nil }, true)
	out, err := inf([]core.Value{core.NewInteger(1), core.NewInteger(2)}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("inf handler: %v %v", out, err)
	}
	if !core.ParentOf(&out[0]).ConformsTo(core.TFloat) {
		t.Errorf("Inf result type = %s, want Float", core.ParentOf(&out[0]))
	}
	nan := numHandler(func(a, b float64) (float64, error) { return math.NaN(), nil }, true)
	out, err = nan([]core.Value{core.NewInteger(1), core.NewInteger(2)}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("nan handler: %v %v", out, err)
	}
	if !core.ParentOf(&out[0]).ConformsTo(core.TFloat) {
		t.Errorf("NaN result type = %s, want Float", core.ParentOf(&out[0]))
	}
}

// The unary handlers share the same guard shape; sqrt of a negative is
// the in-language error arm, and a wrong-typed direct arg hits the
// AsNumber guard.
func TestUnaryHandlerErrorArms(t *testing.T) {
	c, _ := newCalc(t)
	if _, err := c.Eval("-4 sqrt"); err == nil ||
		!strings.Contains(err.Error(), "negative input") {
		t.Errorf("sqrt -4: got %v, want negative-input error", err)
	}
	// Direct call with a non-number: the registered handler must error.
	fn := c.Registry.Lookup("sqrt")
	if fn == nil || len(fn.Signatures) == 0 {
		t.Fatal("sqrt not registered")
	}
	impl, ok := fn.Signatures[0].Impl.(*core.GoImpl)
	if !ok {
		t.Fatalf("sqrt impl is %T, want *GoImpl", fn.Signatures[0].Impl)
	}
	if _, err := impl.Handler([]core.Value{core.NewString("x")}, nil, nil, nil); err == nil {
		t.Error("sqrt on string: want error, got none")
	}
}
