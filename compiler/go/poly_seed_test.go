package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The checker's pick seeds a poly site only where a tag guard proves it:
// strict/concrete operands over tag-determined overloads (tags recorded),
// or a word's only overload of the arity (no tags).
func TestPolySeedFor(t *testing.T) {
	two := []core.Signature{{Args: []*core.Type{core.TInteger, core.TMap}}, {Args: []*core.Type{core.TInteger, core.TList}}}
	strict := []core.Value{core.NewInteger(0), core.NewCarrier(core.TMap)}
	s := polySeedFor(two, &two[0], strict)
	if s == nil || s.sig != &two[0] || len(s.tags) != 2 || s.tags[1] != core.TMap {
		t.Fatalf("strict operands seed with their tags, got %+v", s)
	}
	one := []core.Signature{{Args: []*core.Type{core.TAny, core.TAny}}}
	if s := polySeedFor(one, &one[0], []core.Value{core.NewDynamicCarrier(core.TAny), core.NewInteger(1)}); s == nil || s.tags != nil {
		t.Errorf("a word's only overload seeds without tags, got %+v", s)
	}
	pat := core.NewInteger(1)
	patSigs := []core.Signature{{Params: []core.FnParam{{Type: core.TInteger, Pattern: &pat}, {Type: core.TMap}}}}
	quoted := []core.Signature{{Args: []*core.Type{core.TAny, core.TMap}, QuoteArgs: map[int]bool{0: true}}, {Args: []*core.Type{core.TInteger, core.TList}}}
	asc := core.NewInteger(0)
	asc.SetAscribed(core.TNumber)
	for name, c := range map[string]struct {
		sigs []core.Signature
		sig  *core.Signature
		args []core.Value
	}{
		"no pick":         {two, nil, strict},
		"arity mismatch":  {two, &two[0], strict[:1]},
		"pattern":         {patSigs, &patSigs[0], strict},
		"dynamic operand": {two, &two[0], []core.Value{core.NewInteger(0), core.NewDynamicCarrier(core.TMap)}},
		"bare type":       {two, &two[0], []core.Value{core.NewTypeLiteral(core.TInteger), core.NewCarrier(core.TMap)}},
		"ascribed":        {two, &two[0], []core.Value{asc, core.NewCarrier(core.TMap)}},
		"no tag":          {two, &two[0], []core.Value{{}, core.NewCarrier(core.TMap)}},
		"wide quoted":     {quoted, &quoted[0], []core.Value{core.NewInteger(0), core.NewCarrier(core.TMap)}},
	} {
		if s := polySeedFor(c.sigs, c.sig, c.args); s != nil {
			t.Errorf("%s: must not seed, got %+v", name, s)
		}
	}
	// A quoted slot holding a Word or an Atom seeds.
	if s := polySeedFor(quoted, &quoted[0], []core.Value{core.NewAtom("k"), core.NewCarrier(core.TMap)}); s == nil {
		t.Error("an Atom in a quoted slot seeds")
	}
	es := &EmitState{pendingPolySeed: s}
	if es.takePolySeed() != s || es.pendingPolySeed != nil {
		t.Error("the pending seed is taken once")
	}
}
