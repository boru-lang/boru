package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The poly inline cache returns a pick only for the aggregate and the tags
// it was filled with, and fills only where the pick is keyed by tags.
func TestPolyCacheEntry(t *testing.T) {
	sigs := []core.Signature{{Args: []*core.Type{core.TInteger}}, {Args: []*core.Type{core.TString}}}
	fn := &core.FnDefInfo{Name: "w", Signatures: sigs}
	vc := &vmContext{}
	pr := &compiler.PolyRef{Word: "w"}
	e := vc.polyCacheFor(pr)
	if vc.polyCacheFor(pr) != e {
		t.Fatal("one entry per site")
	}
	one := []core.Value{core.NewInteger(1)}
	if e.hit(fn, one) != nil {
		t.Error("an empty entry has no pick")
	}
	e.fill(fn, sigs, one, &sigs[0])
	if e.hit(fn, []core.Value{core.NewInteger(7)}) != &sigs[0] {
		t.Error("the same tags take the pick")
	}
	if e.hit(fn, []core.Value{core.NewString("s")}) != nil {
		t.Error("other tags miss")
	}
	if e.hit(&core.FnDefInfo{Name: "w", Signatures: sigs}, one) != nil {
		t.Error("another aggregate (a redefinition) misses")
	}
	if e.hit(fn, []core.Value{core.NewInteger(1), core.NewInteger(2)}) != nil {
		t.Error("another window length misses")
	}
	if e.hit(fn, []core.Value{core.NewCarrier(core.TInteger)}) != nil {
		t.Error("a non-keyable operand misses")
	}
	// A non-keyable window leaves no pick; nor does an ineligible aggregate.
	e.fill(fn, sigs, []core.Value{core.NewTypeLiteral(core.TInteger)}, &sigs[0])
	if e.sig != nil {
		t.Error("a bare type operand is not cached")
	}
	pat := core.NewInteger(1)
	patSigs := []core.Signature{{Params: []core.FnParam{{Type: core.TInteger, Pattern: &pat}}}}
	patFn := &core.FnDefInfo{Name: "p", Signatures: patSigs}
	e.fill(patFn, patSigs, one, &patSigs[0])
	if e.sig != nil || e.hit(patFn, one) != nil {
		t.Error("a pattern overload set is never cached")
	}
}
