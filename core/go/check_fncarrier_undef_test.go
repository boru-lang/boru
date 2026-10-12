package core

import "testing"

// TestDropCheckFnCarrierBind pins the `undef` half of the fn-carrier side
// table (the P1 review finding on PR #397). The table is a SECOND binding
// store for a name — installDef declines a computed fn, so the name lives
// only here — and if `undef` pops Defs without dropping the table, the two
// stores disagree and a later read resolves a carrier whose binding is gone.
func TestDropCheckFnCarrierBind(t *testing.T) {
	r := compileCheckRegistry(t)
	carrier := NewCarrier(TFunction)
	SetID(&carrier, "v1")
	NoteCheckFnCarrierBind(r, "f", carrier)
	if _, hit := CheckFnCarrierBind(r, "f"); !hit {
		t.Fatal("the bind must be recorded before the drop")
	}

	DropCheckFnCarrierBind(r, "f")
	if _, hit := CheckFnCarrierBind(r, "f"); hit {
		t.Error("undef must leave no carrier behind — a stale hit lets the " +
			"compiler model an application of a binding that is gone")
	}

	// Dropping an absent name, and dropping with no table at all, are both
	// no-ops: `undef` runs for every name, not only carrier-bound ones.
	DropCheckFnCarrierBind(r, "never-bound")
	DropCheckFnCarrierBind(compileCheckRegistry(t), "f")
}
