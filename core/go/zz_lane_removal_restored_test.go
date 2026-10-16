package core

import (
	"errors"

	"testing"
)

func TestInactiveConstructorSlots(t *testing.T) {
}

// TestIsInertReachForeignPayload: a value typed Reach whose payload is not a
// ReachInfo (AsReach errors) declines the standalone lens test exactly as
// the member test does — the const table never pools a malformed reach.

func TestS5AStepLiteralSpliceDynRecording(t *testing.T) {
	r := covRegistry(t, nil)
	sp := NewSplice(NewDynamicCarrier(TAny))
	e := s5aEng(t, r, []Value{sp}, 0)
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
	if IsSplice(e.Tape.Ref(0)) {
		t.Error("splice marker not expanded")
	}
}

// --- stepLiteral: check-mode dispatch hooks -------------------------------

func TestS5AAutoEvalListRecording(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)

	// Top-level: RecordMakeList.
	eTop := NewTop(r)
	if _, err := eTop.autoEvalList(NewEvalList([]Value{NewDynamicCarrier(TAny)}), false); err != nil {
		t.Fatalf("autoEvalList top: %v", err)
	}
	// Consumed inner: RecordMakeListInner.
	eSub := New(r)
	if _, err := eSub.autoEvalList(NewEvalList([]Value{NewDynamicCarrier(TAny)}), true); err != nil {
		t.Fatalf("autoEvalList inner: %v", err)
	}
}

// --- interp string / xml interp dynamic arms ------------------------------

func TestS5AInterpStringDynamicHole(t *testing.T) {
	r := covRegistry(t, nil)
	savedVCC := AnalysisImpl.ValueCarriesCarrier
	AnalysisImpl.ValueCarriesCarrier = func(v Value) bool { return v.Carrier() }
	defer func() { AnalysisImpl.ValueCarriesCarrier = savedVCC }()

	e := s5aEng(t, r, nil, 0)
	is := NewInterpString([]InterpPart{{Expr: []Value{NewCarrier(TInteger)}}})
	got, err := e.evalInterpString(is)
	if err != nil {
		t.Fatalf("evalInterpString: %v", err)
	}
	if !got.Carrier() || !got.Parent().Equal(TString) {
		t.Errorf("dynamic interp = %v, want String carrier", got)
	}
}

func TestS5AAutoEvalMapConsumedRecording(t *testing.T) {
	r := covRegistry(t, nil)
	s5aCheckOn(t, r)

	om := NewOrderedMap()
	om.Set("l", NewEvalList([]Value{NewDynamicCarrier(TAny)}))
	mv := NewMap(om)
	e := s5aEng(t, r, nil, 0)
	if _, err := e.AutoEvalMap(mv, false, true); err != nil {
		t.Fatalf("AutoEvalMap: %v", err)
	}
}

// --- exprHasEffect / constFoldContainerVal --------------------------------

func TestNUR099CapitalisedFnBodyRefused(t *testing.T) {
	r := newTestRegistry(t)
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
	if def := r.LookupTypeName("Nur099Pred"); def == nil || !def.Parent().Equal(TInteger) {
		t.Fatalf("the declared predicate mints at its input type, got %v", def)
	}
}

// TestNUR099IsFnBodiedValueArms pins the refusal's discriminator: only a
// function VALUE (a fn definition or a compiled closure) under Function is
// fn-bodied; a type literal sitting under Function (a module's exported
// member type) binds as an alias, and nothing outside Function qualifies.

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
		{"Function type literal", NewTypeLiteral(TFunction), false},
		{"other payload under Function", Value{d: &Datum{Parent: TFunction, Data: ListPayload{}}}, false},
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
