package core

import "testing"

// CallbackWantsKeyVal is the map-iteration words' entry rule: a callback
// whose LAST param is typed KeyVal asks for the entry {k v i n}; every other
// callback — an untyped param, a Map param, a value that is no fn — takes
// the entry's value. Positive and negative shapes side by side.
func TestCallbackWantsKeyVal(t *testing.T) {
	sig := func(types ...*Type) Signature {
		ps := make([]FnParam, len(types))
		for i, ty := range types {
			ps[i] = FnParam{Type: ty}
		}
		return Signature{Params: ps, BarrierPos: len(ps)}
	}
	fn := func(sigs ...Signature) Value {
		return Value{Parent: TFunction, Data: FnDefInfo{Signatures: sigs}}
	}
	for _, c := range []struct {
		name string
		fn   Value
		want bool
	}{
		{"a KeyVal entry param", fn(sig(TKeyVal)), true},
		{"fold's (accumulator, KeyVal)", fn(sig(TInteger, TKeyVal)), true},
		{"any signature asking for the entry wins", fn(sig(TInteger), sig(TInteger, TKeyVal)), true},
		{"an Any param takes the value", fn(sig(TAny)), false},
		{"an untyped param takes the value", fn(sig(nil)), false},
		{"a Map param takes the value: a KeyVal is a Map, a Map is no KeyVal", fn(sig(TMap)), false},
		{"a KeyVal accumulator with a value entry", fn(sig(TKeyVal, TInteger)), false},
		{"a nullary signature", fn(sig()), false},
		{"no signature at all", fn(), false},
		{"a token quotation is no fn", NewList(nil), false},
		{"a scalar is no fn", NewInteger(1), false},
	} {
		if got := CallbackWantsKeyVal(c.fn); got != c.want {
			t.Errorf("%s: CallbackWantsKeyVal = %v, want %v", c.name, got, c.want)
		}
	}
}
