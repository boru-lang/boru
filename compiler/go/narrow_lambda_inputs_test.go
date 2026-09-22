package compiler

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// narrowLambdaInputs binds a GRADUAL Any callback input to the lambda's
// declared param type, and only that: a typed input, an undeclared (Any)
// param and a strict carrier are left as they are, and the caller's slice is
// never mutated (S1b's apply shapes, 2026-09-22).
func TestNarrowLambdaInputs(t *testing.T) {
	gradual := func() core.Value { return check.NewElementCarrier(core.TAny) }
	lam := &core.Signature{Params: []core.FnParam{{Name: "a", Type: core.TInteger}, {Name: "b"}, {Name: "c", Type: core.TAny}, {Name: "d", Type: core.TString}}}
	inputs := []core.Value{gradual(), gradual(), gradual(), core.NewCarrier(core.TInteger)}
	out := narrowLambdaInputs(inputs, lam)
	if &out[0] == &inputs[0] {
		t.Fatal("a narrowed slice must be a copy, not the caller's")
	}
	if out[0].Dynamic || !out[0].Parent.Equal(core.TInteger) {
		t.Errorf("a gradual Any input under a declared Integer param must narrow to a strict Integer carrier, got %#v", out[0])
	}
	if !out[1].Dynamic || !out[1].Parent.Equal(core.TAny) {
		t.Errorf("an undeclared param keeps the gradual input, got %#v", out[1])
	}
	if !out[2].Dynamic || !out[2].Parent.Equal(core.TAny) {
		t.Errorf("a param declared Any keeps the gradual input, got %#v", out[2])
	}
	if out[3].Dynamic || !out[3].Parent.Equal(core.TInteger) {
		t.Errorf("a typed (strict) input is left alone whatever the param declares, got %#v", out[3])
	}
	if !inputs[0].Dynamic || !inputs[0].Parent.Equal(core.TAny) {
		t.Error("the caller's inputs must not be mutated")
	}
	// Nothing to narrow: the caller's slice comes back as it is.
	same := []core.Value{core.NewCarrier(core.TInteger)}
	if got := narrowLambdaInputs(same, &core.Signature{Params: []core.FnParam{{Type: core.TInteger}}}); &got[0] != &same[0] {
		t.Error("with nothing to narrow the input slice is returned unchanged")
	}
	// More params than inputs: the walk stops at the inputs' end.
	short := []core.Value{gradual()}
	if got := narrowLambdaInputs(short, lam); len(got) != 1 || got[0].Dynamic || !got[0].Parent.Equal(core.TInteger) {
		t.Errorf("a longer param list narrows only the inputs present, got %#v", got)
	}
}
