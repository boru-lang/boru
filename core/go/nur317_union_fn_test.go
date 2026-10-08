package core

import "testing"

// TestUnionMayBeFn pins NUR317's union test: a union carrier with a fn
// alternative may hold the fn, one without may not, and a union that is not
// a carrier (a type body) or a carrier that is not a union is neither.
func TestUnionMayBeFn(t *testing.T) {
	union := func(carrier bool, alts ...*Type) Value {
		var vs []Value
		for _, a := range alts {
			vs = append(vs, NewTypeLiteral(a))
		}
		u := NewDisjunct(vs)
		u.Carrier = carrier
		return u
	}
	for _, c := range []struct {
		why  string
		v    Value
		want bool
	}{
		{"a union with a fn alternative", union(true, TInteger, TFunction), true},
		{"a union of data", union(true, TInteger, TString), false},
		{"a union type body", union(false, TInteger, TFunction), false},
		{"a fn carrier", NewCarrier(TFunction), false},
		{"data", NewInteger(7), false},
	} {
		if got := UnionMayBeFn(c.v); got != c.want {
			t.Errorf("%s: UnionMayBeFn = %v, want %v", c.why, got, c.want)
		}
	}
}
