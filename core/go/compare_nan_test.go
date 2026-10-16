package core

import (
	"math"
	"testing"
)

// NaN comparison has two regimes (design/IEEE-754-COMPLIANCE.8.md Tier 0):
// the relational handlers lt/lte/gt/gte are IEEE-unordered (always false
// when a NaN is involved), while CompareValues — the total order behind
// cmp/tcmp/sort — gives NaN a defined slot (greatest). These tests pin
// both, and that they stay mutually consistent (the old bug: lte/gte/cmp
// disagreed with lt/gt/eq).

func fv(x float64) Value { return NewFloat(x) }

func TestNaNTotalOrder(t *testing.T) {
	nan := fv(math.NaN())
	cases := []struct {
		a, b Value
		want int
		name string
	}{
		{nan, fv(5), 1, "nan after finite"},
		{fv(5), nan, -1, "finite before nan"},
		{nan, nan, 0, "nan ties nan"},
		{fv(math.Inf(1)), nan, -1, "inf before nan"},
		{nan, fv(math.Inf(-1)), 1, "nan after -inf"},
		{fv(math.Inf(1)), fv(5), 1, "inf after finite"},
		{fv(math.Inf(-1)), fv(5), -1, "-inf before finite"},
	}
	for _, c := range cases {
		got, err := CompareValues(c.a, c.b)
		if err != nil {
			t.Errorf("%s: CompareValues error: %v", c.name, err)
			continue
		}
		if signOf(got) != c.want {
			t.Errorf("%s: CompareValues = %d, want %d", c.name, signOf(got), c.want)
		}
	}
}

func TestNaNRelationalUnordered(t *testing.T) {
	nan := fv(math.NaN())
	five := fv(5)
	// Each relational handler must return false for every pairing that
	// involves a NaN — in either argument position, and nan-vs-nan.
	handlers := map[string]Handler{
		"lt": LtHandler, "lte": LteHandler, "gt": GtHandler, "gte": GteHandler,
	}
	pairs := [][2]Value{{nan, five}, {five, nan}, {nan, nan}}
	for name, h := range handlers {
		for _, p := range pairs {
			res, err := h([]Value{p[0], p[1]}, nil, nil, nil)
			if err != nil {
				t.Errorf("%s(%v,%v): error %v", name, p[0], p[1], err)
				continue
			}
			b, _ := AsBoolean(res[0])
			if b {
				t.Errorf("%s(%v,%v) = true, want false (IEEE unordered)", name, p[0], p[1])
			}
		}
	}
	// Sanity: with NO NaN, the same handlers still order normally.
	res, _ := LtHandler([]Value{five, fv(9)}, nil, nil, nil) // args[0]=b=5, args[1]=a=9 → 9<5 → false
	if b, _ := AsBoolean(res[0]); b {
		t.Errorf("lt(5,9) infix-form = true, want false (9<5)")
	}
	res, _ = GtHandler([]Value{five, fv(9)}, nil, nil, nil) // 9>5 → true
	if b, _ := AsBoolean(res[0]); !b {
		t.Errorf("gt(5,9) infix-form = false, want true (9>5)")
	}
}

func TestIsNaNValueGuards(t *testing.T) {
	if !isNaNValue(addrOf(fv(math.NaN()))) {
		t.Error("isNaNValue(NaN) = false")
	}
	for _, v := range []Value{fv(1.5), fv(math.Inf(1)), NewInteger(3), NewString("x"), NewTypeLiteral(TFloat)} {
		if isNaNValue(&v) {
			t.Errorf("isNaNValue(%v) = true, want false", v)
		}
	}
}
