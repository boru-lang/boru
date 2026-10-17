package core

import "testing"

// ApplyBindTwin is the runtime half of §6.5's regime: one placed OpBindTwin
// re-performing one recorded transition. Each kind's arm — and the
// write-back pairing that leaves a written-back def's push to its Push-mode
// OpBindGlobal — is pinned here directly, against the same DefTable
// surface the sandbox harness proved at corpus scale.

func TestRestoreBindingsForReplayKeepsModuleLedger(t *testing.T) {
	r := newTestRegistry(t)
	if r.Modules == nil {
		t.Skip("registry has no module ledger")
	}
	minted := r.Types.MintType("ReplayT", TInteger)
	r.Defs.PushType("ReplayT", minted, NewTypeLiteral(TInteger))
	snap := r.SnapshotBindings()

	// The pass: a def, an undef's retirement, an import.
	r.Defs.Push("passed", NewInteger(1))
	r.Defs.Pop("ReplayT")
	r.Types.Retire(minted)
	r.Modules.Loaded["imported"] = ModuleDesc{Ref: "imported"}

	r.RestoreBindingsForReplay(snap)

	if _, ok := r.Defs.Top("passed"); ok {
		t.Error("the def was not rolled back for its twin to replay")
	}
	if r.Types.LookupByID(IDOf(minted)) != minted {
		t.Error("the retirement was not re-admitted for the undef twin to re-apply")
	}
	if _, ok := r.Modules.Loaded["imported"]; !ok {
		t.Error("the module ledger must stay PASS-FINAL — rolling it back re-imports")
	}

	// Zero-value and nil safety, like every sandbox entry point.
	r.Defs.Push("held", NewInteger(2))
	r.RestoreBindingsForReplay(BindingSandbox{})
	if _, ok := r.Defs.Top("held"); !ok {
		t.Error("the zero sandbox must restore nothing")
	}
	var nilReg *Registry
	nilReg.RestoreBindingsForReplay(BindingSandbox{valid: true})
}
