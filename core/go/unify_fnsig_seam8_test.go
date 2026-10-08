package core

// Seam-8 (cluster W8_eng_rest): in-package unit test for the
// sa==ShapeFnUndef canonicalization arm of unifyFnUndefShape. Per
// design/TEST-SEAMS.10.md.

import "testing"

func TestW8UnifyFnUndefShapeLeftUndef(t *testing.T) {
	undef := NewFnUndef(FnUndefInfo{}) // empty constraint matches any fn
	fn := NewValueRaw(TFunction, FnDefInfo{})
	if Shape(undef) != ShapeFnUndef {
		t.Fatalf("precondition: FnUndef value must have ShapeFnUndef, got %v", Shape(undef))
	}
	got, err := unifyFnUndefShape(undef, Shape(undef), fn, Shape(fn), nil)
	if err != nil {
		t.Fatalf("empty FnUndef must unify with a function: %v", err)
	}
	if _, ok := got.Data.(FnDefInfo); !ok {
		t.Fatalf("result should be the function value, got %v", got)
	}
}
