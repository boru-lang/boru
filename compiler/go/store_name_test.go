package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestSeatStoreName pins the lowerer's def-name seat for a promoted store
// (the twenty-ninth increment): nothing without a recorder or a table,
// nothing for a slot no def named, and the name at the store's pc.
func TestSeatStoreName(t *testing.T) {
	code := []Instr{}
	var tbl map[int]string
	(&lowerer{code: &code, storeNames: &tbl}).seatStoreName(0, 0)
	if tbl != nil {
		t.Fatal("no recorder: nothing seated")
	}
	es := NewEmitState()
	es.defNameAt = map[seqIdx]string{{seq: 2, idx: 0}: "h"}
	(&lowerer{es: es, code: &code}).seatStoreName(2, 0)
	(&lowerer{es: es, code: &code, storeNames: &tbl}).seatStoreName(2, 1)
	if tbl != nil {
		t.Fatal("no table, or a slot no def named: nothing seated")
	}
	code = append(code, Instr{Op: OpPushConst}, Instr{Op: OpPushConst})
	(&lowerer{es: es, code: &code, storeNames: &tbl}).seatStoreName(2, 0)
	if tbl[2] != "h" {
		t.Fatalf("the name seats at the store's pc: %v", tbl)
	}
	_ = core.TAny
}
