package check

// Complements the spec/index.tsv corpus: the corpus drives
// CheckAtIndices end to end over concrete lists; these tests cover the
// branches a row cannot reach — the typed-list-carrier length
// refinement, the unknown-length and non-list declines, and
// CheckListIndex (the single-index variant no fixture word wires).

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestStaticListLen(t *testing.T) {
	// A concrete list literal reports its exact count.
	if n, ok := StaticListLen(core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})); !ok || n != 2 {
		t.Errorf("concrete list = (%d,%v), want (2,true)", n, ok)
	}
	// A non-list reports false.
	if _, ok := StaticListLen(core.NewInteger(3)); ok {
		t.Error("an integer has no static list length")
	}
	// A plain (unrefined) list carrier reports false.
	if _, ok := StaticListLen(core.NewCarrier(core.TList)); ok {
		t.Error("an unrefined carrier has no known length")
	}
	// A typed-list carrier with a Len refinement reports it.
	ln := 3
	refined := core.NewCarrier(core.TList)
	refined.SetData(core.ChildTypeInfo{Len: &ln})
	if n, ok := StaticListLen(refined); !ok || n != 3 {
		t.Errorf("refined carrier = (%d,%v), want (3,true)", n, ok)
	}
}

func TestCheckListIndex(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	data := core.NewList([]core.Value{core.NewInteger(10), core.NewInteger(20)})

	// In range: silent.
	CheckListIndex(r, core.NewInteger(1), data, "getr")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("in-range index flagged: %+v", r.Check.Diagnostics)
	}
	// Unknown length: silent.
	CheckListIndex(r, core.NewInteger(9), core.NewCarrier(core.TList), "getr")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatal("unknown-length container must stay silent")
	}
	// Provably out of range: flagged.
	CheckListIndex(r, core.NewInteger(5), data, "getr")
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("provable OOB must flag exactly once, got %+v", r.Check.Diagnostics)
	}
}
