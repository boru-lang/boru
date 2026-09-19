package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestClosureIsFnValue pins the fn-VALUE closure predicate (S1b-2): a unit is
// a fn value's when it was compiled from a `fn` / `=>` literal (Lambda) with
// its param contract recorded; a closure is one when its unit is and it
// carries the plain value shape, under a program the value can name.
func TestClosureIsFnValue(t *testing.T) {
	if UnitIsFnValue(nil) {
		t.Error("a nil unit is no fn value")
	}
	if UnitIsFnValue(&CompiledFn{Name: "do$body", NArgs: 1}) {
		t.Error("a callback body unit (no contract) is no fn value")
	}
	if UnitIsFnValue(&CompiledFn{Name: "fnval$body", Lambda: true, NArgs: 1}) {
		t.Error("a lambda unit with no recorded contract is no fn value")
	}
	fnUnit := CompiledFn{Name: "fnval$body", Lambda: true, NArgs: 1, Params: []*core.Type{core.TInteger}}
	if !UnitIsFnValue(&fnUnit) {
		t.Error("a lambda unit with its contract is a fn value's")
	}
	prog := &Program{Fns: []CompiledFn{fnUnit, {Name: "do$body"}}}

	if ClosureIsFnValue(core.NewInteger(1)) {
		t.Error("a non-closure value is no fn-value closure")
	}
	if ClosureIsFnValue(NewClosure(nil, 0, nil)) {
		t.Error("a closure with no program identity reads as data")
	}
	if ClosureIsFnValue(NewClosure(prog, 5, nil)) {
		t.Error("a unit the program cannot name reads as data")
	}
	if ClosureIsFnValue(NewClosure(prog, 1, nil)) {
		t.Error("a callback body unit's closure is no fn value")
	}
	shaped := NewClosure(prog, 0, nil)
	cl := shaped.Data.(core.ClosurePayload)
	cl.InShape = ClosureInKeyVal
	shaped.Data = cl
	if ClosureIsFnValue(shaped) {
		t.Error("a closure in a word's callback shape is that word's body, not a fn value")
	}
	if !ClosureIsFnValue(NewClosure(prog, 0, nil)) {
		t.Error("a fn value's closure in the plain value shape is one")
	}
}
