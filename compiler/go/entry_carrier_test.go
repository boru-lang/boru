package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// entryCarrier and lambdaCallbackInputs carry the entry rule into the compiled
// lambda's inputs: the element of a list, the value of a map entry, or — over
// a map, for a callback whose entry param is typed KeyVal — a KeyVal carrier
// whose v field carries the map's value type. A KeyVal-typed param over a
// LIST still gets the element carrier (the compile then declines).
func TestEntryCarrierAndCallbackInputs(t *testing.T) {
	r := newTestRegistry(t)
	list := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	om := core.NewOrderedMap()
	om.Set("a", core.NewInteger(1))
	m := core.NewMap(om)
	for _, c := range []struct {
		name   string
		data   core.Value
		keyVal bool
		want   *core.Type
	}{
		{"a list element", list, false, core.TInteger},
		{"a list element for a KeyVal-typed param", list, true, core.TInteger},
		{"a map value", m, false, core.TInteger},
		{"a map entry for a KeyVal-typed param", m, true, core.TKeyVal},
	} {
		// The KeyVal is a representative VALUE whose fields are carriers
		// (keyValCarrier); every other entry is a carrier itself.
		got := entryCarrier(r, c.data, c.keyVal)
		if !got.Parent.Equal(c.want) || (got.Carrier == c.want.Equal(core.TKeyVal)) {
			t.Errorf("%s: entryCarrier = %v, want a %s carrier", c.name, got, c.want.Name())
		}
	}
	kv := entryCarrier(r, m, true)
	fields, _ := core.AsMap(kv)
	if v, ok := fields.Get(core.KeyValV); !ok || !v.Parent.Equal(core.TInteger) || !v.Carrier {
		t.Errorf("the KeyVal carrier's v field = %v, want an Integer carrier", v)
	}

	spec := core.CallableSpec{BodyPos: 0}
	body := core.NewInteger(0) // a placeholder in the body slot: only the data operand is read
	for _, c := range []struct {
		word   string
		args   []core.Value
		keyVal bool
		want   []*core.Type
		shape  core.ClosureInShape
	}{
		{"filter", []core.Value{body, list}, true, []*core.Type{core.TInteger}, ClosureInValue},
		{"filter", []core.Value{body, m}, false, []*core.Type{core.TInteger}, ClosureInValue},
		{"filter", []core.Value{body, m}, true, []*core.Type{core.TKeyVal}, ClosureInValue},
		{"each", []core.Value{body, m}, true, []*core.Type{core.TKeyVal}, ClosureInValue},
		{"for-each", []core.Value{body, m}, false, []*core.Type{core.TInteger}, ClosureInValue},
		{"fold", []core.Value{body, m, core.NewInteger(0)}, true, []*core.Type{core.TInteger, core.TKeyVal}, ClosureInValue},
		{"fold", []core.Value{body, m, core.NewInteger(0)}, false, []*core.Type{core.TInteger, core.TInteger}, ClosureInValue},
		{"scan", []core.Value{body, m}, true, []*core.Type{core.TInteger, core.TKeyVal}, ClosureInValue},
		{"fold", []core.Value{body, list, core.NewInteger(0)}, true, []*core.Type{core.TInteger, core.TInteger}, ClosureInStackPair},
	} {
		ins, shape, ok := lambdaCallbackInputs(r, c.word, spec, c.args, c.keyVal)
		if !ok || shape != c.shape || len(ins) != len(c.want) {
			t.Errorf("%s keyVal=%v: inputs=%v shape=%v ok=%v", c.word, c.keyVal, ins, shape, ok)
			continue
		}
		for i, w := range c.want {
			if !ins[i].Parent.Equal(w) {
				t.Errorf("%s keyVal=%v: input %d = %v, want %s", c.word, c.keyVal, i, ins[i], w.Name())
			}
		}
	}
	// A map fold with no seed has no lambda convention.
	if _, _, ok := lambdaCallbackInputs(r, "fold", spec, []core.Value{body, m}, true); ok {
		t.Error("an unseeded map fold must decline")
	}
}
