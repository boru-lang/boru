package eng_test

import "testing"

// TestBlockScopeBattery pins the VM's half of block scoping
// (design/IMMUTABLE-DEF.1.md §2.1, phase 2) on the standalone kernel: a fn
// body's type binding ends with the frame on both lanes — the unit's RET
// releases the name's reservation (unwindDynBinds) as the interpreter's
// frame teardown does — so the second call's bind finds the name free,
// where NUR167's rule had the second call conflict.
func TestBlockScopeBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		{input: "def f fn [[n:Integer] [Integer] [def T (refine Integer) n]] f 1 f 2", want: "1 2"},
		{input: "def f fn [[n:Integer] [Boolean] [def T (refine Integer) n is T]] f 1 f 2", want: "true true"},
	})
}
