package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	tabnasexpr "github.com/tabnas/expr/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// TestS7B_MathFloatMultiply drives evalApply's float multiplication arm:
// two Float operands promote the whole expression to Float.
func TestS7B_MathFloatMultiply(t *testing.T) {
	r := mcovReg(t)
	out := mcovRun(t, r, mcovImp+`mini math 'x*y' {x:1.5 y:2.0}`)
	f, err := out[0].AsConcreteFloat()
	if err != nil || f != 3.0 {
		t.Errorf("mini math float multiply = %v (err %v), want 3.0", out[0], err)
	}
}

// TestS7B_MathValueListRef drives evalFormula's non-pointer jsonic.ListRef
// arm: a value (not pointer) ListRef node folds through evalApply.
func TestS7B_MathValueListRef(t *testing.T) {
	op := &tabnasexpr.Op{Name: "positive-prefix"}
	node := jsonic.ListRef{Val: []any{op, float64(7)}}
	mv, err := evalFormula(node, map[string]mval{}, 0)
	if err != nil {
		t.Fatalf("evalFormula(value ListRef): %v", err)
	}
	if !mv.isInt || mv.i != 7 {
		t.Errorf("evalFormula(+7) = %+v, want isInt=true i=7", mv)
	}
}

// TestS7B_MathBoruToMvalBadPayloads drives boruToMval's AsInteger / AsFloat
// error arms: a value whose Parent conforms to Integer/Float but whose
// payload is not the matching numeric payload is declined (not coerced).
func TestS7B_MathBoruToMvalBadPayloads(t *testing.T) {
	badInt := native.NewValueRaw(native.TInteger, native.StrPayload{S: "x"})
	if _, ok := boruToMval(badInt); ok {
		t.Error("boruToMval: Integer-typed value with non-int payload should be declined")
	}
	badFloat := native.NewValueRaw(native.TFloat, native.StrPayload{S: "x"})
	if _, ok := boruToMval(badFloat); ok {
		t.Error("boruToMval: Float-typed value with non-float payload should be declined")
	}
}
