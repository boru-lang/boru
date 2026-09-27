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
