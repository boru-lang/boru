package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestPlacedWindowSplit pins NUR312's split of a drift window around its
// placed (paren-wrapped) values: no split where no placed value would
// dispatch, the resolved prefix and the rest where one would, and a refusal
// where a value beneath the last placed one is not plain data.
func TestPlacedWindowSplit(t *testing.T) {
	g := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	op, cp, word, three := core.NewOpenParen(), core.NewCloseParen(), core.NewWord("add"), core.NewInteger(3)
	if _, _, split, refused := placedWindowSplit([]core.Value{core.NewInteger(2), op, core.NewInteger(5), cp, word, three}); split || refused {
		t.Error("a placed datum keeps the verbatim island")
	}
	in, toks, split, refused := placedWindowSplit([]core.Value{core.NewInteger(2), op, g, cp, word, three})
	if !split || refused || len(in) != 2 || !in[1].Parent.Equal(core.TFunction) || len(toks) != 2 {
		t.Errorf("a placed fn starts the island after it: in=%v toks=%v split=%v refused=%v", in, toks, split, refused)
	}
	list := core.NewList([]core.Value{core.NewInteger(1)})
	if _, _, split, refused := placedWindowSplit([]core.Value{list, op, g, cp, word, three}); split || !refused {
		t.Error("a value beneath the placed fn that is not plain data refuses")
	}
	if _, _, split, refused := placedWindowSplit([]core.Value{op, g, cp, op, core.NewInteger(4), cp, word, three}); !split || refused {
		t.Error("two placed values: the island starts after the last")
	}
}

// TestNamedDispatchingFn pins loopExitReStep's frame-tail test (NUR314): a
// named fn value the pointer would dispatch, as a definition or a compiled
// closure, and nothing else — a lambda, a quoted fn, data.
func TestNamedDispatchingFn(t *testing.T) {
	fnv := func(p core.Payload, quoted bool) core.Value {
		return core.Value{Parent: core.TFunction, Data: p, Quoted: quoted}
	}
	for _, c := range []struct {
		why  string
		v    core.Value
		want bool
	}{
		{"a named definition", fnv(core.FnDefInfo{Name: "g"}, false), true},
		{"a named lambda", fnv(core.FnDefInfo{Name: "g", Anonymous: true}, false), false},
		{"a quoted definition", fnv(core.FnDefInfo{Name: "g"}, true), false},
		{"a named closure", fnv(core.ClosurePayload{Named: true}, false), true},
		{"an anonymous closure", fnv(core.ClosurePayload{}, false), false},
		{"data", core.NewInteger(7), false},
	} {
		if got := namedDispatchingFn(c.v); got != c.want {
			t.Errorf("%s: namedDispatchingFn = %v, want %v", c.why, got, c.want)
		}
	}
}
