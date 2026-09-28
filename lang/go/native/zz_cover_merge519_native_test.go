package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestMerge519CloneReturnsTypeLiteral pins clone's check residual for a type
// VALUE (NUR323): CloneValue shares a type literal, so the clone is a type,
// and the residual is its Type carrier — never a carrier of the node's Parent
// (the Integer node's Parent is Number, which would admit `add`). The
// negative twin is a plain value, whose residual is a carrier of its own type.
func TestMerge519CloneReturnsTypeLiteral(t *testing.T) {
	out := cloneReturnsFn([]Value{NewTypeLiteral(TInteger)}, nil)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(TType) {
		t.Fatalf("clone of a type literal must type as a Type carrier, got %v", out)
	}
	if out[0].Parent.Equal(TInteger.Parent) {
		t.Errorf("clone of Integer must not type as its Parent %v", TInteger.Parent)
	}

	out = cloneReturnsFn([]Value{NewInteger(5)}, nil)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(TInteger) {
		t.Errorf("clone of 5 must type as an Integer carrier, got %v", out)
	}
}

// TestMerge519BehaveReturnsSeesOnlyConcreteCheckCalls pins behaveReturns'
// entry screen: it notes a behaviour only inside an ACTIVE check pass and only
// for a concrete behaviour name — no registry, an inactive pass, or a computed
// (carrier) name notes nothing. The positive half is the same `make` call with
// a concrete name in check mode, which notes the maker.
func TestMerge519BehaveReturnsSeesOnlyConcreteCheckCalls(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	r.InitRootContext()
	Register(r)

	src := `def P class {a:Integer} end def mk fn [[x:Any] [P] [make P {a: 42}]] end`
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := NewTop(r).Run(toks); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	fnVal, ok := r.Defs.Top("mk")
	if !ok || !fnVal.Parent.Equal(TFunction) {
		t.Fatalf("mk must bind a fn value, got %v (ok=%v)", fnVal, ok)
	}
	pType := r.LookupTypeName("P")
	if pType == nil {
		t.Fatal("P must be a type")
	}
	noted := func() bool { return r.Check.BehaveMakers[core.CanonicalType(r, pType)] }

	name := NewString("make")

	// No registry, and a registry outside check mode: nothing is noted.
	if out := behaveReturns([]Value{name, fnVal}, nil); out != nil {
		t.Errorf("a nil registry returns nothing, got %v", out)
	}
	if out := behaveReturns([]Value{name, fnVal}, r); out != nil || noted() {
		t.Errorf("an inactive pass notes nothing (out=%v, noted=%v)", out, noted())
	}

	r.Check.Mode = true
	// A computed behaviour name: the pass cannot see which slot it fills.
	if out := behaveReturns([]Value{NewCarrier(TString), fnVal}, r); out != nil || noted() {
		t.Errorf("a carrier name notes nothing (out=%v, noted=%v)", out, noted())
	}

	// Positive: a concrete `make` in check mode notes P's maker.
	if out := behaveReturns([]Value{name, fnVal}, r); out != nil {
		t.Errorf("behave leaves nothing, got %v", out)
	}
	if !noted() {
		t.Error("a concrete make behave in check mode must note P's maker")
	}
}
