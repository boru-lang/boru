package native

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// bodySigOver builds the throwaway signature a handler dispatches a raw body
// with — params over the body's tokens, one Any return.
func bodySigOver(params []FnParam, body []Value) *FnSig {
	return &FnSig{Params: params, Returns: []*Type{TAny}, Impl: Boru(append([]Value(nil), body...)), BarrierPos: -1}
}

// TestStampBodySigHostsAndRemembers: on an armed registry a raw body over a
// handler's params stamps once per shape — the returned signature carries the
// unit's ref, its params typed by the inputs — and the memo answers the next
// application of the same body and shape with the SAME signature; a body the
// compile declines keeps the throwaway signature and is remembered as
// declined (one stamp event, not two).
func TestStampBodySigHostsAndRemembers(t *testing.T) {
	r := stampSvcReg(t, true)
	body := NewList([]Value{NewString("c"), NewString("d")})
	sig := bodySigOver([]FnParam{{Type: TAny}}, body.Data.(core.ListPayload).Elems)
	in := []Value{NewString("x")}
	stamped := StampBodySig(r, sig, body, in)
	if stamped == sig || compiler.CompiledRef(stamped) == nil {
		t.Fatalf("an armed stamp returns a signature carrying the unit's ref")
	}
	if len(stamped.Params) != 1 || stamped.Params[0].Type != core.TokenBodyInputType(in[0]) {
		t.Fatalf("the unit's param is typed by the input: %v", stamped.Params)
	}
	if again := StampBodySig(r, sig, body, in); again != stamped {
		t.Fatal("the same body and shape answers from the memo")
	}
	if other := StampBodySig(r, sig, body, []Value{NewInteger(1)}); other == stamped || compiler.CompiledRef(other) == nil {
		t.Fatal("another input type is another shape, stamped on its own")
	}
	res, err := core.InvokeCallback(r, stamped, in, nil)
	if err != nil || len(res) != 3 {
		t.Fatalf("the hosted unit answers the frame's whole residual — the input beneath the two atoms: %v %v", res, err)
	}
	if last, _ := res[2].AsConcreteString(); last != "d" {
		t.Fatalf("the residual's top is the body's last atom: %v", res)
	}
	// The declined body: `args` is context-dependent inside a stored unit.
	declined := NewList([]Value{NewWord("args"), NewWord("dot"), NewInteger(0)})
	dsig := bodySigOver([]FnParam{{Type: TAny}}, declined.Data.(core.ListPayload).Elems)
	before := len(r.StampEvents())
	if got := StampBodySig(r, dsig, declined, in); got != dsig {
		t.Fatal("a declined body keeps the throwaway signature")
	}
	if got := StampBodySig(r, dsig, declined, in); got != dsig {
		t.Fatal("a declined body keeps the throwaway signature on the next application too")
	}
	if n := len(r.StampEvents()) - before; n != 1 {
		t.Fatalf("a declined shape pays the compile once, got %d stamp events", n)
	}
}

// TestStampBodySigKeepsTheFrame: every shape the stamp stands aside from
// returns the throwaway signature unchanged — an unarmed registry (the
// interpreter lane), a nil signature or registry, an input count that is not
// the params', a Go-backed or empty body, a body with no name, and an input
// that does not conform to the declared param.
func TestStampBodySigKeepsTheFrame(t *testing.T) {
	body := NewList([]Value{NewInteger(1)})
	toks := body.Data.(core.ListPayload).Elems
	in := []Value{NewInteger(5)}
	sig := bodySigOver([]FnParam{{Type: TAny}}, toks)
	if StampBodySig(stampSvcReg(t, false), sig, body, in) != sig {
		t.Fatal("unarmed: the throwaway signature")
	}
	r := stampSvcReg(t, true)
	if StampBodySig(nil, sig, body, in) != sig || StampBodySig(r, nil, body, in) != nil {
		t.Fatal("a nil registry or signature stands aside")
	}
	if StampBodySig(r, sig, body, nil) != sig {
		t.Fatal("an input count that is not the params' stands aside")
	}
	goSig := &FnSig{Params: []FnParam{{Type: TAny}}, Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) { return nil, nil })}
	if StampBodySig(r, goSig, body, in) != goSig {
		t.Fatal("a Go-backed signature stands aside")
	}
	empty := bodySigOver([]FnParam{{Type: TAny}}, nil)
	if StampBodySig(r, empty, NewList(nil), in) != empty {
		t.Fatal("an empty body stands aside")
	}
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	unnamed := NewList([]Value{NewMap(om), NewWord("size")})
	unnamed.ID = ""
	usig := bodySigOver([]FnParam{{Type: TAny}}, unnamed.Data.(core.ListPayload).Elems)
	if StampBodySig(r, usig, unnamed, in) != usig {
		t.Fatal("a body with no name stands aside")
	}
	mapSig := bodySigOver([]FnParam{{Name: "r", Type: TMap}}, toks)
	if StampBodySig(r, mapSig, body, in) != mapSig {
		t.Fatal("an input that does not conform to the declared param keeps the frame")
	}
}
