package compiler

import "testing"

// TestUnitTailApply pins the seam the call-site collapse reads (the
// twenty-ninth increment): a nil recorder, a unit out of range and a unit
// with no tail apply miss; a unit whose finish lowered one reports its
// window width.
func TestUnitTailApply(t *testing.T) {
	if _, ok := (*EmitState)(nil).UnitTailApply(0); ok {
		t.Fatal("a nil recorder must miss")
	}
	es := NewEmitState()
	if _, ok := es.UnitTailApply(0); ok {
		t.Fatal("a unit out of range must miss")
	}
	es.fnRecs = append(es.fnRecs, &fnUnitRec{}, &fnUnitRec{dynTrailArity: 2})
	if _, ok := es.UnitTailApply(0); ok {
		t.Fatal("a unit with no tail apply must miss")
	}
	if n, ok := es.UnitTailApply(1); !ok || n != 2 {
		t.Fatalf("the window width rides out: %d %v", n, ok)
	}
}
