package core

// Seam-8 (cluster W8_eng_rest): in-package unit tests for previously-
// unreached error / edge arms in depscalar.go. All calls are direct with
// crafted Values (sealed constructors then field mutation) per
// design/TEST-SEAMS.10.md.

import "testing"

// w8NilTypeVal returns a concrete value whose Parent is nil, so
// ValueType(v) == nil and CompareValues(v, _) errors ("cannot compare
// values with nil type"). This is the only way to make CompareValues
// fail, and it drives every "compare errored" guard in depscalar.go.
func w8NilTypeVal() Value {
	v := NewInteger(5)
	v.SetParent(nil)
	return v
}

func TestW8DepScalarCheckNoBounds(t *testing.T) {
	// An info with neither Lo nor Hi matches nothing.
	if depScalarCheck(DepScalarInfo{}, NewInteger(1)) {
		t.Fatal("empty DepScalarInfo must not match")
	}
	// Sanity: a populated bound still matches a satisfying value.
	info := DepScalarInfo{Lo: &DepBound{Inclusive: false, Value: NewInteger(0)}}
	if !depScalarCheck(info, NewInteger(1)) {
		t.Fatal("1 must satisfy (gt 0)")
	}
}

func TestW8DepBoundCheckCompareError(t *testing.T) {
	// A value with nil ValueType makes CompareValues error; the bound
	// check rejects cleanly (returns false).
	b := &DepBound{Inclusive: true, Value: NewInteger(0)}
	if depBoundCheck(b, true, w8NilTypeVal()) {
		t.Fatal("compare error must yield false")
	}
}

func TestW8BoundsEqualDifferentParent(t *testing.T) {
	a := &DepBound{Inclusive: true, Value: NewInteger(1)}
	b := &DepBound{Inclusive: true, Value: NewString("x")}
	if boundsEqual(a, b) {
		t.Fatal("bounds with different parent types are not equal")
	}
}

func TestW8TightenSameSideCompareError(t *testing.T) {
	a := &DepBound{Inclusive: false, Value: w8NilTypeVal()}
	b := &DepBound{Inclusive: false, Value: NewInteger(1)}
	if _, ok := tightenSameSide(a, b, true); ok {
		t.Fatal("compare error must yield ok=false")
	}
}

func TestW8CombineDepScalarsLoTightenError(t *testing.T) {
	a := DepScalarInfo{Lo: &DepBound{Value: w8NilTypeVal()}}
	b := DepScalarInfo{Lo: &DepBound{Value: NewInteger(1)}}
	if _, ok := combineDepScalars(a, b); ok {
		t.Fatal("Lo tighten compare error must yield ok=false")
	}
}

func TestW8CombineDepScalarsHiTightenError(t *testing.T) {
	a := DepScalarInfo{Hi: &DepBound{Value: w8NilTypeVal()}}
	b := DepScalarInfo{Hi: &DepBound{Value: NewInteger(1)}}
	if _, ok := combineDepScalars(a, b); ok {
		t.Fatal("Hi tighten compare error must yield ok=false")
	}
}

func TestW8CombineDepScalarsHiTightenOK(t *testing.T) {
	// Two same-side upper bounds tighten to the smaller; no Lo, so the
	// final interval check is skipped and out.Hi is assigned.
	a := DepScalarInfo{Hi: &DepBound{Inclusive: true, Value: NewInteger(5)}}
	b := DepScalarInfo{Hi: &DepBound{Inclusive: true, Value: NewInteger(3)}}
	out, ok := combineDepScalars(a, b)
	if !ok {
		t.Fatal("same-side upper bounds must combine")
	}
	if out.Hi == nil {
		t.Fatal("expected an upper bound in result")
	}
	if got, _ := AsInteger(out.Hi.Value); got != 3 {
		t.Fatalf("tighter upper bound should be 3, got %d", got)
	}
}

func TestW8CombineDepScalarsIntervalCompareError(t *testing.T) {
	// a contributes Lo (nil-type value), b contributes Hi; neither switch
	// arm compares, so control reaches the final interval check where
	// CompareValues(out.Lo, out.Hi) errors.
	a := DepScalarInfo{Lo: &DepBound{Value: w8NilTypeVal()}}
	b := DepScalarInfo{Hi: &DepBound{Value: NewInteger(1)}}
	if _, ok := combineDepScalars(a, b); ok {
		t.Fatal("interval compare error must yield ok=false")
	}
}

func TestW8ComplementWithinBaseUnbounded(t *testing.T) {
	// An info with neither bound complements to the bare base literal.
	got := complementWithinBase(TInteger, DepScalarInfo{})
	if !IsBareTypeNode(&got) {
		t.Fatalf("expected a bare type literal, got %v", got)
	}
	if !ValueType(got).Equal(TInteger) {
		t.Fatalf("expected Integer literal, got %v", got)
	}
}
