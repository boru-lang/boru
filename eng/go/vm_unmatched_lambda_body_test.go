package eng

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestUnmatchedLambdaBody pins the token seam's per-element contract match
// for a lambda-derived callback BODY unit (NUR155): a quoted body, a unit
// the closure cannot name, a unit with no contract of its own and a
// matching window all stand aside (the ordinary invoke follows); a list
// body's no-match hands the inputs back with the closure on top, rendered
// as the interpreter renders the lambda; a map body's no-match raises the
// map arm's signature_error.
func TestUnmatchedLambdaBody(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	p := &compiler.Program{Fns: []compiler.CompiledFn{
		{Name: "each$body", NParams: 1, NArgs: 1, NLocals: 1, Params: []*core.Type{core.TInteger}, Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: []core.SrcPos{{}}},
		{Name: "tok$body", NParams: 1, NArgs: 1, NLocals: 1, Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: []core.SrcPos{{}}},
		{Name: "fold$body", NParams: 2, NArgs: 2, NLocals: 2, Params: []*core.Type{core.TInteger, core.TInteger}, InShape: compiler.ClosureInStackPair, Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: []core.SrcPos{{}}},
		{Name: "map$body", NParams: 2, NArgs: 2, NLocals: 2, Params: []*core.Type{core.TInteger, core.TAny}, InShape: compiler.ClosureInKeyVal, Code: []compiler.Instr{{Op: compiler.OpRet}}, Debug: []core.SrcPos{{}}},
	}}
	vc := &vmContext{p: p, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	closure := func(unit int, shape core.ClosureInShape) (core.Value, core.ClosurePayload) {
		cl := core.ClosurePayload{Prog: p, Unit: unit, InShape: shape, Ident: core.NewFnIdentity()}
		return core.NewValueRaw(core.TFunction, cl), cl
	}
	standsAside := func(name string, body core.Value, cl core.ClosurePayload, inputs ...core.Value) {
		t.Helper()
		if _, _, ran := vc.unmatchedLambdaBody(r, body, cl, inputs); ran {
			t.Errorf("%s: the seam must stand aside", name)
		}
	}
	body, cl := closure(0, compiler.ClosureInValue)
	quoted := body
	quoted.Quoted = true
	standsAside("a quoted body", quoted, cl, core.NewString("s"))
	_, badCl := closure(9, compiler.ClosureInValue)
	standsAside("a unit the closure cannot name", body, badCl, core.NewString("s"))
	tokBody, tokCl := closure(1, compiler.ClosureInValue)
	standsAside("a token body with no contract", tokBody, tokCl, core.NewString("s"))
	standsAside("a matching element", body, cl, core.NewInteger(5))

	// A list body's no-match: the inputs come back with the closure on top,
	// rendered as the lambda.
	res, err, ran := vc.unmatchedLambdaBody(r, body, cl, []core.Value{core.NewString("s")})
	if !ran || err != nil {
		t.Fatalf("no-match: ran=%v err=%v", ran, err)
	}
	if len(res) != 2 || !res[0].Parent.ConformsTo(core.TString) || res[1].String() != "fn (Integer)" {
		t.Errorf("no-match = %v, want ['s' fn (Integer)]", res)
	}
	// The stack-pair shape matches over the REVERSED pair, as the bind seats
	// it: (acc=1, elem="s") is a no-match on the element's slot.
	pairBody, pairCl := closure(2, compiler.ClosureInStackPair)
	standsAside("a stack pair both admitted", pairBody, pairCl, core.NewInteger(1), core.NewInteger(2))
	res, err, ran = vc.unmatchedLambdaBody(r, pairBody, pairCl, []core.Value{core.NewInteger(1), core.NewString("s")})
	if !ran || err != nil || len(res) != 3 || res[2].String() != "fn (Integer, Integer)" {
		t.Errorf("stack-pair no-match = %v (ran=%v err=%v), want the pair with the closure on top", res, ran, err)
	}
	// A map body's no-match raises the map arm's error.
	mapBody, mapCl := closure(3, compiler.ClosureInKeyVal)
	_, err, ran = vc.unmatchedLambdaBody(r, mapBody, mapCl, []core.Value{core.NewString("s"), core.NewKeyVal("a", core.NewInteger(1), 0, 1)})
	var be *core.BoruError
	if !ran || !errors.As(err, &be) || be.Code != "signature_error" {
		t.Errorf("map no-match: ran=%v err=%v, want the map arm's signature_error", ran, err)
	}
}
