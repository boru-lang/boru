package core

import (
	"errors"
	"testing"
)

// TestNUR099CapitalisedFnBodyRefused pins NUR099's refusal at the
// installer: a capitalised name declares a TYPE, and a function is a type
// only as a DECLARED predicate. An undeclared fn body — one parameter or
// two, a fn definition or a compiled closure — is refused with def_error
// and binds nothing; the same body marked by fnpred installs.
func TestNUR099CapitalisedFnBodyRefused(t *testing.T) {
	r := newTestRegistry(t)
	defer r.DisableRuntimeStamping()
	sig := func(params ...FnParam) FnDefInfo {
		return FnDefInfo{Registry: r, Signatures: []Signature{{
			Params: params,
			Impl:   Boru([]Value{NewBoolean(true)}),
		}}}
	}
	one := sig(FnParam{Name: "n", Type: TInteger})
	two := sig(FnParam{Name: "a", Type: TAny}, FnParam{Name: "b", Type: TAny})
	for _, c := range []struct {
		name string
		body Value
	}{
		{"Nur099One", NewFunction(one)},
		{"Nur099Two", NewFunction(two)},
		{"Nur099Closure", Value{Parent: TFunction, Data: ClosurePayload{Unit: 3}}},
	} {
		err := InstallType(r, c.name, c.body)
		var be *BoruError
		if !errors.As(err, &be) || be.Code != "def_error" {
			t.Errorf("%s: want def_error, got %v", c.name, err)
		}
		if r.Defs.Has(c.name) {
			t.Errorf("%s: a refused declaration must bind nothing", c.name)
		}
	}
	// The declared twin of the one-parameter body installs a predicate type.
	if err := InstallType(r, "Nur099Pred", MarkPredicateFn(NewFunction(one))); err != nil {
		t.Fatalf("a declared predicate must install: %v", err)
	}
	if def := r.LookupTypeName("Nur099Pred"); def == nil || !def.Parent.Equal(TInteger) {
		t.Fatalf("the declared predicate mints at its input type, got %v", def)
	}
}

// TestNUR099IsFnBodiedValueArms pins the refusal's discriminator: only a
// function VALUE (a fn definition or a compiled closure) under Function is
// fn-bodied; a type literal sitting under Function (a module's exported
// member type) binds as an alias, and nothing outside Function qualifies.
func TestNUR099IsFnBodiedValueArms(t *testing.T) {
	for _, c := range []struct {
		name string
		v    Value
		want bool
	}{
		{"fn definition", NewFunction(FnDefInfo{}), true},
		{"compiled closure", Value{Parent: TFunction, Data: ClosurePayload{Unit: 1}}, true},
		{"Function type literal", NewTypeLiteral(TFunction), false},
		{"other payload under Function", Value{Parent: TFunction, Data: ListPayload{}}, false},
		{"non-Function value", NewInteger(3), false},
		{"nil parent", Value{}, false},
	} {
		if got := isFnBodiedValue(c.v); got != c.want {
			t.Errorf("%s: isFnBodiedValue = %v, want %v", c.name, got, c.want)
		}
	}
	// PredicateInputType's non-Function arm: no input type outside Function.
	if got := PredicateInputType(NewInteger(3)); got != nil {
		t.Errorf("a non-Function value has no predicate input type, got %v", got)
	}
}
