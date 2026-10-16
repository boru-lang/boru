package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestBuildLiteralConstructsFromParts: each evaluation of a fresh literal's
// build is a new spine — a new outer list, a new nested list, a new map —
// around the shared parts: the kept member is the same instance every time
// (`((mk) get 0) eq c`), while two evaluations are two values (`(mk) eq
// (mk)` is false). A fn literal is a new function per evaluation, a baked
// mutable container an independent copy, a shared part the part itself.
func TestBuildLiteralConstructsFromParts(t *testing.T) {
	member := withID(core.NewList([]core.Value{core.NewInteger(9)}), "bind-c")
	inner := core.NewList([]core.Value{member})
	m := core.NewOrderedMap()
	m.Set("k", core.NewInteger(3))
	m.Meta = map[string]any{"tag": 1}
	mapLit := core.NewMap(m)
	fnv := core.NewFunction(core.FnDefInfo{Anonymous: true})
	flex := core.NewFlexList([]core.Value{core.NewInteger(2)})
	outer, three, empty := core.NewList(nil), core.NewInteger(3), core.NewMap(nil)
	b := &compiler.LiteralBuild{Kind: compiler.BuildList, Val: &outer, Parts: []compiler.LiteralBuild{
		{Kind: compiler.BuildList, Val: &inner, Parts: []compiler.LiteralBuild{{Kind: compiler.BuildShared, Val: &member}}},
		{Kind: compiler.BuildShared, Val: &member},
		{Kind: compiler.BuildMap, Val: &mapLit, Keys: []string{"k"}, Implicit: true, Meta: m.Meta, Parts: []compiler.LiteralBuild{{Kind: compiler.BuildShared, Val: &three}}},
		{Kind: compiler.BuildFn, Val: &fnv},
		{Kind: compiler.BuildCopy, Val: &flex},
		{Kind: compiler.BuildMap, Val: &empty},
	}}
	one, two := buildLiteral(b), buildLiteral(b)
	e1, _ := core.AsList(one)
	e2, _ := core.AsList(two)
	if e1.Len() != 6 || e2.Len() != 6 {
		t.Fatalf("built %d and %d parts, want 6", e1.Len(), e2.Len())
	}
	if core.IDOf(&one) == core.IDOf(&two) || &e1.Elems()[0] == &e2.Elems()[0] {
		t.Fatalf("two evaluations must be two instances: %q %q", core.IDOf(&one), core.IDOf(&two))
	}
	if a, b := e1.Get(0), e2.Get(0); core.IDOf(&a) == core.IDOf(&b) {
		t.Errorf("the nested literal must be constructed per evaluation")
	}
	if a, b := e1.Get(1), e2.Get(1); core.IDOf(&a) != "bind-c" || core.IDOf(&b) != "bind-c" {
		t.Errorf("the kept member must be the binding's instance: %q %q", core.IDOf(&a), core.IDOf(&b))
	}
	got := e1.Get(2)
	gm, _ := core.AsMap(got)
	if v, ok := gm.Get("k"); !ok || core.Canon([]core.Value{v}) != "3" {
		t.Errorf("the map must carry its entry: %v", got)
	}
	if om := got.Data().(core.MapPayload).M; !om.Implicit || om.Meta["tag"] != 1 || om == m {
		t.Errorf("the map must be a new OrderedMap carrying the marks: %+v", om)
	}
	f1, f2 := e1.Get(3), e2.Get(3)
	d1, _ := core.FnDefOf(f1.Data())
	d2, _ := core.FnDefOf(f2.Data())
	if d1 == d2 {
		t.Errorf("a fn literal must be a new function per evaluation")
	}
	if c := e1.Get(4); c.Data().(*core.FlexListData) == flex.Data().(*core.FlexListData) {
		t.Errorf("a baked flex list must be an independent copy")
	}
	if em := e1.Get(5); em.Data().(core.MapPayload).M != nil {
		t.Errorf("a map with no entries stays empty")
	}
	if s := buildLiteral(&compiler.LiteralBuild{Kind: compiler.BuildShared, Val: &member}); core.IDOf(&s) != "bind-c" {
		t.Errorf("a shared build is its part")
	}
}
