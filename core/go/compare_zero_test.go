package core

import (
	"math"
	"math/big"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// Signed zeros have two regimes, mirroring NaN (NUR013,
// design/IEEE-754-COMPLIANCE.8.md §5.10): the total order behind
// cmp/tcmp/sort follows IEEE totalOrder and places the Float -0.0
// strictly before every positive zero, while the relational words
// lt/lte/gt/gte keep the IEEE §5.11 rule (-0 == +0, so lt/gt are false
// and lte/gte true) and eq/deq stay true (Go's float ==). These tests
// pin both regimes, the cross-leaf slotting (Integer 0 and the Big
// zeros — including a BigDecimal negative-zero spelling — are +0), and
// transitivity across the -0.0 / 0 / 0d0 triangle.

func negZero() Value { return NewFloat(math.Copysign(0, -1)) }

// bigDecZero returns a BigDecimal zero. neg=true sets apd's Negative
// flag (the `-0` spelling) — it must STILL slot as +0: big.Rat has no
// signed zero, so the flag cannot leak through toRatExact's Text('f')
// path into the total order.
func bigDecZero(t *testing.T, neg bool) Value {
	t.Helper()
	d, _, err := new(apd.Decimal).SetString("0")
	if err != nil {
		t.Fatalf("apd SetString: %v", err)
	}
	d.Negative = neg
	return NewBigDecimal(d)
}

func TestSignedZeroTotalOrder(t *testing.T) {
	nz := negZero()
	cases := []struct {
		a, b Value
		want int
		name string
	}{
		// Float pair: totalOrder places -0 before +0.
		{nz, fv(0), -1, "-0.0 before 0.0"},
		{fv(0), nz, 1, "0.0 after -0.0"},
		{nz, negZero(), 0, "-0.0 ties -0.0"},
		{fv(0), fv(0), 0, "0.0 ties 0.0"},
		// Cross-leaf: Integer 0 slots as +0.
		{NewInteger(0), nz, 1, "Integer 0 after -0.0"},
		{nz, NewInteger(0), -1, "-0.0 before Integer 0"},
		// Big branch: BigDecimal / BigInteger zeros slot as +0.
		{bigDecZero(t, false), nz, 1, "0d0 after -0.0"},
		{nz, bigDecZero(t, false), -1, "-0.0 before 0d0"},
		{bigDecZero(t, true), nz, 1, "apd negative-zero still slots as +0"},
		{NewBigInteger(big.NewInt(0)), nz, 1, "BigInteger 0 after -0.0"},
		// Transitivity across the triangle: -0.0 < 0, 0 = 0d0, -0.0 < 0d0.
		{NewInteger(0), bigDecZero(t, false), 0, "Integer 0 ties 0d0"},
		// Non-zero magnitude ties are untouched (signedZeroOrder's
		// both-false arm) — cross-leaf equivalence survives.
		{NewInteger(1), fv(1), 0, "1 ties 1.0"},
		{bigDecOne(t), fv(1), 0, "0d1 ties 1.0 (big branch tie)"},
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

func bigDecOne(t *testing.T) Value {
	t.Helper()
	d, _, err := new(apd.Decimal).SetString("1")
	if err != nil {
		t.Fatalf("apd SetString: %v", err)
	}
	return NewBigDecimal(d)
}

// TestSignedZeroRelationalEqual pins the IEEE §5.11 carve-out: the
// relational words treat every ±0 pair as EQUAL — across all four
// numeric leaves — even though the total order now separates them.
func TestSignedZeroRelationalEqual(t *testing.T) {
	nz := negZero()
	pz := fv(0)
	iz := NewInteger(0)
	dz := bigDecZero(t, false)
	type rel struct {
		name string
		h    Handler
		want bool // for an EQUAL pair
	}
	rels := []rel{
		{"lt", LtHandler, false},
		{"gt", GtHandler, false},
		{"lte", LteHandler, true},
		{"gte", GteHandler, true},
	}
	pairs := [][2]Value{
		{nz, pz}, {pz, nz}, {nz, negZero()},
		{nz, iz}, {iz, nz},
		{nz, dz}, {dz, nz},
	}
	for _, r := range rels {
		for _, p := range pairs {
			res, err := r.h([]Value{p[0], p[1]}, nil, nil, nil)
			if err != nil {
				t.Errorf("%s(%v,%v): error %v", r.name, p[0], p[1], err)
				continue
			}
			b, _ := AsBoolean(res[0])
			if b != r.want {
				t.Errorf("%s(%v,%v) = %v, want %v (IEEE -0 == +0)", r.name, p[0], p[1], b, r.want)
			}
		}
	}
	// Sanity: a non-zero pair against -0.0 still orders relationally —
	// the guard fires ONLY for ±0 pairs.
	res, _ := LtHandler([]Value{fv(5), nz}, nil, nil, nil) // args[1]=a=-0.0, args[0]=b=5 → -0.0 < 5 → true
	if b, _ := AsBoolean(res[0]); !b {
		t.Errorf("lt(-0.0, 5.0) = false, want true")
	}
}

// TestSignedZeroEqualityUnchanged pins that eq/deq keep the IEEE
// equality (-0.0 == 0.0) — the total order now distinguishes a pair
// that eq/deq equate, the exact inverse of the NaN asymmetry.
func TestSignedZeroEqualityUnchanged(t *testing.T) {
	nz := negZero()
	if !ExactEqual(nz, fv(0)) {
		t.Error("ExactEqual(-0.0, 0.0) = false, want true (IEEE -0 == +0)")
	}
	if !DeepEqual(nz, fv(0)) {
		t.Error("DeepEqual(-0.0, 0.0) = false, want true")
	}
	if !ExactEqual(NewInteger(0), nz) {
		t.Error("ExactEqual(0, -0.0) = false, want true")
	}
}

func TestSignedZeroGuards(t *testing.T) {
	nz := negZero()
	// isNegZeroFloat: only the concrete Float -0.0 qualifies.
	if !isNegZeroFloat(addrOf(nz)) {
		t.Error("isNegZeroFloat(addrOf(-0.0)) = false")
	}
	for _, v := range []Value{fv(0), fv(-5), NewInteger(0), NewString("x"), bigDecZero(t, true)} {
		if isNegZeroFloat(addrOf(v)) {
			t.Errorf("isNegZeroFloat(addrOf(%v)) = true, want false", v)
		}
	}
	// A Float carrier conforms to TFloat but has no payload — the
	// AsFloat error branch.
	if isNegZeroFloat(addrOf(NewCarrier(TFloat))) {
		t.Error("isNegZeroFloat(addrOf(Float carrier)) = true, want false")
	}
	// isZeroNumber: zero magnitude across leaves; NaN fails toRatExact.
	for _, v := range []Value{fv(0), nz, NewInteger(0), bigDecZero(t, false)} {
		if !isZeroNumber(addrOf(v)) {
			t.Errorf("isZeroNumber(addrOf(%v)) = false, want true", v)
		}
	}
	for _, v := range []Value{fv(1), NewInteger(3), NewString("0"), fv(math.NaN())} {
		if isZeroNumber(addrOf(v)) {
			t.Errorf("isZeroNumber(addrOf(%v)) = true, want false", v)
		}
	}
	// signedZeroPair: needs a -0.0 on one side AND zero magnitude on both.
	if !signedZeroPair(addrOf(nz), addrOf(fv(0))) || !signedZeroPair(addrOf(NewInteger(0)), addrOf(nz)) {
		t.Error("signedZeroPair(addrOf(±0 pair)) = false, want true")
	}
	if signedZeroPair(addrOf(fv(0)), addrOf(NewInteger(0))) {
		t.Error("signedZeroPair(addrOf(+0), addrOf(+0)) = true, want false (no -0.0 side)")
	}
	if signedZeroPair(addrOf(nz), addrOf(fv(5))) {
		t.Error("signedZeroPair(addrOf(-0.0), addrOf(5.0)) = true, want false (non-zero side)")
	}
	// signedZeroOrder arms.
	if got := signedZeroOrder(nz, negZero()); got != 0 {
		t.Errorf("signedZeroOrder(-0.0, -0.0) = %d, want 0", got)
	}
	if got := signedZeroOrder(nz, fv(0)); got != -1 {
		t.Errorf("signedZeroOrder(-0.0, 0.0) = %d, want -1", got)
	}
	if got := signedZeroOrder(fv(0), nz); got != 1 {
		t.Errorf("signedZeroOrder(0.0, -0.0) = %d, want 1", got)
	}
}
