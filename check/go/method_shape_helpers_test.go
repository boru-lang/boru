package check

// Unit coverage for method_shape.go's value-level helpers — the pure
// predicates under the shaped-method dispatch model. The model's driver
// (TryShapedMethodDispatch) steps a live engine mid-tape and is exercised
// end-to-end from the layers above; these helpers are plain functions
// over core values, the class the corpus cannot address directly.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestEvalFixedWindowToken(t *testing.T) {
	fixed := []core.Value{
		core.NewInteger(1),
		core.NewString("s"),
		core.NewBoolean(true),
		core.NewFloat(1.5),
		core.NewAtom("a"),
		core.NewList([]core.Value{core.NewInteger(1), core.NewString("x")}),
	}
	for _, v := range fixed {
		if !evalFixedWindowToken(v) {
			t.Errorf("%s must be evaluation-fixed", v.Parent().Leaf())
		}
	}

	notFixed := []core.Value{
		core.NewCarrier(core.TInteger),
		core.NewDynamicCarrier(core.TAny),
		core.NewWord("w"),
		core.NewList([]core.Value{core.NewWord("w")}),
	}
	for _, v := range notFixed {
		if evalFixedWindowToken(v) {
			t.Errorf("%s must NOT be evaluation-fixed", v.Parent().Leaf())
		}
	}

	// A literal map of fixed values is fixed; one holding a word is not.
	m := core.NewOrderedMap()
	m.Set("k", core.NewInteger(1))
	if !evalFixedWindowToken(core.NewMap(m)) {
		t.Error("a map of fixed values must be evaluation-fixed")
	}
	m2 := core.NewOrderedMap()
	m2.Set("k", core.NewWord("w"))
	if evalFixedWindowToken(core.NewMap(m2)) {
		t.Error("a map holding a word must NOT be evaluation-fixed")
	}
}

func TestStatementWindowBoundary(t *testing.T) {
	if !statementWindowBoundary(core.NewEnd()) {
		t.Error("End is a statement boundary")
	}
	if !statementWindowBoundary(core.NewCloseParen()) {
		t.Error("CloseParen is a statement boundary")
	}
	if statementWindowBoundary(core.NewInteger(1)) {
		t.Error("an integer is not a boundary")
	}
	if statementWindowBoundary(core.NewWord("w")) {
		t.Error("a word is not a boundary")
	}
}

func TestAllZeroArgSigsAndFnDefName(t *testing.T) {
	zero := core.FnDefInfo{Name: "z", Signatures: []core.FnSig{{
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}}}
	if !allZeroArgSigs(&zero) {
		t.Error("a fn whose only sig takes no params is all-zero-arg")
	}
	one := core.FnDefInfo{Name: "o", Signatures: []core.FnSig{{
		Params:  []core.FnParam{{Name: "n", Type: core.TInteger}},
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}}}
	if allZeroArgSigs(&one) {
		t.Error("a fn with a 1-arg sig is not all-zero-arg")
	}

	if got := fnDefName(core.NewFunction(one)); got != "o" {
		t.Errorf("fnDefName = %q, want the fn's own name", got)
	}
	if got := fnDefName(core.NewInteger(3)); got != "fn value" {
		t.Errorf("fnDefName on a non-fn = %q, want the fallback", got)
	}
}

func TestDynamicBoundConformsToFunction(t *testing.T) {
	fnDef := core.FnDefInfo{Signatures: []core.FnSig{{
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}}}
	if !dynamicBoundConformsToFunction(core.NewFunction(fnDef)) {
		t.Error("a function value conforms")
	}
	if dynamicBoundConformsToFunction(core.NewInteger(1)) {
		t.Error("an integer does not conform")
	}
	// A disjunct with a Function-type alternative conforms — the
	// alternative is a bare type literal whose lattice identity is the
	// represented type.
	dis := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TFunction),
		core.NewTypeLiteral(core.TNone),
	})
	if !dynamicBoundConformsToFunction(dis) {
		t.Error("a Function|None disjunct conforms on its Function alternative")
	}
	disNo := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger),
		core.NewTypeLiteral(core.TString),
	})
	if dynamicBoundConformsToFunction(disNo) {
		t.Error("an Integer|String disjunct does not conform")
	}
}
