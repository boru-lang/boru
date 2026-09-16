package core

import (
	"errors"
	"testing"
)

// RunUnit starts a fresh VM run entered at a specific compiled fn unit, binding
// per-call args to the leading param slots and the ref's captures to the
// trailing slots — the durable-callback entry point (serve-raw handlers,
// spawned processes) that fires AFTER the enclosing RunProgram returned. These
// tests drive it with hand-built units, mirroring the vm_steplimit / vm_seam
// style, since eng has no def/fn words to compile a real body.

func runUnitReg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// isInternalErr classifies exactly the internal_error BoruError (and nothing
// else): a foreign error and a genuine boru runtime error both report false, so
// InvokeCallback returns them straight through rather than masking with a
// fallback.
func TestIsInternalErr(t *testing.T) {
	if !IsInternalErr(makeBoruError("internal_error", "boom", "", "", "")) {
		t.Error("internal_error BoruError must classify true")
	}
	if IsInternalErr(makeBoruError("signature_error", "no match", "", "", "")) {
		t.Error("a genuine boru runtime error must classify false")
	}
	if IsInternalErr(errors.New("foreign")) {
		t.Error("a non-BoruError must classify false")
	}
	if IsInternalErr(nil) {
		t.Error("nil must classify false")
	}
}

// TestIsVMDefer: only a BoruError carrying the VMDefer marker classifies as a
// designed VM defer — a user `raise internal_error` (same public code, no
// marker) does not, so the `do` escape hatch catches it (Codex P2 on #469).
func TestIsVMDefer(t *testing.T) {
	defer_ := &BoruError{Code: "internal_error", Detail: "bytecode: internal: …", VMDefer: true}
	if !IsVMDefer(defer_) {
		t.Error("a VMDefer-marked internal_error classifies as a defer")
	}
	userInternal := makeBoruError("internal_error", "boom", "", "", "")
	if IsVMDefer(userInternal) {
		t.Error("a user-raised internal_error (no marker) is not a defer")
	}
	if IsVMDefer(errors.New("foreign")) || IsVMDefer(nil) {
		t.Error("a non-BoruError and nil are not defers")
	}
}
