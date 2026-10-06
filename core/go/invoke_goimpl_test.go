package core

import "testing"

// A Go-IMPLEMENTED signature reaching the callback seam's interpreter fallback
// runs its handler over the matched args (fn-util's composed values under
// filter, a parked native word), where CallBoru — a body splicer — would run
// the frame over nothing and hand the inputs back. A boru-bodied signature
// keeps CallBoru.
func TestInvokeCallbackDispatchesGoSignature(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var seen []Value
	goSig := Signature{
		Params: []FnParam{{Name: "x", Type: TInteger}},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			seen = args
			return []Value{NewBoolean(true)}, nil
		}),
		BarrierPos: 1,
	}
	NormalizeSig(&goSig)
	res, err := InvokeCallback(r, &goSig, []Value{NewInteger(7)}, nil)
	if err != nil || len(res) != 1 || !res[0].Parent.Equal(TBoolean) {
		t.Fatalf("a Go signature's handler must answer: res=%v err=%v", res, err)
	}
	if len(seen) != 1 || !seen[0].Parent.Equal(TInteger) {
		t.Errorf("the handler saw %v, want the matched arg 7", seen)
	}
	// The boru-bodied twin runs through CallBoru: its body leaves 1 above the
	// unnamed input it ignores.
	boruSig := Signature{Params: []FnParam{{Type: TInteger}}, Impl: Boru([]Value{NewInteger(1)}), BarrierPos: 1}
	NormalizeSig(&boruSig)
	res, err = InvokeCallback(r, &boruSig, []Value{NewInteger(7)}, nil)
	if err != nil || len(res) == 0 {
		t.Fatalf("a boru signature runs its body: res=%v err=%v", res, err)
	}
	if n, _ := AsInteger(res[len(res)-1]); n != 1 {
		t.Errorf("the boru body's result = %v, want 1 on top", res)
	}
}
