package native

import (
	"strings"
	"testing"
)

// Codex round-7 fixes (design/TYPED-CONTAINER-TAG-RETENTION.0.md):
//
//	P1 the list<->map index-merge handlers build a LIST result, so the LIST
//	   operand's [:T] governs — re-tag against it, not d2typedMergeOperand (which
//	   picked the map patch's tag and validated against the wrong contract).
//	P2 mergeReturns retains the governing operand's tag so a chained write after
//	   a same-kind typed merge is diagnosed in check.

// #1 — merging a map patch into a typed list validates against the LIST's
// element type, not the map's.
func TestMergeMapListEnforcesListTag(t *testing.T) {
	// {:String}{"0":"bad"} merge [:Integer][1] → the "bad" write must be rejected
	// against the destination list's Integer contract (was wrongly accepted).
	patch, r := tcValue(t, `def m:{:String} {"0":"bad"} m`)
	list, _ := tcValue(t, `def xs:[:Integer] [1] xs`)
	if _, err := mergeMapListHandler([]Value{patch, list}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("map-patch into [:Integer] list should reject a String, got %v", err)
	}
	// The symmetric handler: [:Integer] list merge {untyped map with a String}.
	xs, r2 := tcValue(t, `def xs:[:Integer] [1 2] xs`)
	m, _ := tcValue(t, `def m {"0":"bad"} m`)
	if _, err := mergeListMapHandler([]Value{xs, m}, nil, nil, r2); err == nil ||
		!strings.Contains(err.Error(), "does not conform to element type Integer") {
		t.Errorf("list [:Integer] merge {map with String} should reject, got %v", err)
	}
}

// #1 negatives — a conforming index-merge succeeds and keeps the list's tag.
func TestMergeMapListConforming(t *testing.T) {
	patch, r := tcValue(t, `def m {"0":9} m`)
	list, _ := tcValue(t, `def xs:[:Integer] [1 2] xs`)
	out, err := mergeMapListHandler([]Value{patch, list}, nil, nil, r)
	if err != nil {
		t.Fatalf("conforming map-list merge: %v", err)
	}
	if c, ok := out[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("merge result lost the [:Integer] tag")
	}
}

// #2 — mergeReturns retains the governing operand's tag (check residual), so a
// chained write after a same-kind typed merge is flagged.
func TestMergeReturnsTypedResidual(t *testing.T) {
	m, _ := tcValue(t, `def m:{:Integer} {a:1} m`)
	other, _ := tcValue(t, `def o {b:2} o`)
	res := mergeReturns([]Value{m, other}, nil)
	if c, ok := res[0].ElemConstraint(); !ok || !c.Equal(TInteger) {
		t.Errorf("mergeReturns residual lost the {:Integer} tag (chained writes would escape check)")
	}
	if ci, cerr := AsChildType(res[0]); cerr != nil || !ci.Child.Equal(TInteger) {
		t.Errorf("mergeReturns residual child = %v (err %v), want Integer", ParentOf(&res[0]), cerr)
	}
	// Untyped operands → a plain dynamic carrier (no tag).
	u1, _ := tcValue(t, `def a {x:1} a`)
	u2, _ := tcValue(t, `def b {y:2} b`)
	if _, ok := mergeReturns([]Value{u1, u2}, nil)[0].ElemConstraint(); ok {
		t.Errorf("untyped merge residual should carry no tag")
	}
}
