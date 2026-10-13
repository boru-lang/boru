package core

import (
	"errors"
	"strings"
	"testing"
)

// The per-export module gate's remaining dispatch arms (NUR045). The
// end-to-end parity sweep lives in lang (policy_module_gate_test.go);
// these pin the arms that production reaches only through shapes the
// corpus does not carry — the host-callback CallBoru entry, the legacy
// stack-match fn-value path, and the VM's module-poly re-match — using
// the hand-built-program pattern of vm_policy_gate_test.go.

var errModDenied = errors.New("zz-policy: module export denied")

// denyExportChecker denies exactly one (module, export) pair and
// implements ModuleCallChecker only.
type denyExportChecker struct{ module, export string }

func (d denyExportChecker) CheckModuleCall(module, export string) error {
	if module == d.module && export == d.export {
		return errModDenied
	}
	return nil
}

func moduleGateReg(t *testing.T, module, export string) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	if err := r.Capabilities.Set(CapPolicy, denyExportChecker{module: module, export: export}); err != nil {
		t.Fatalf("install checker: %v", err)
	}
	return r
}

// CallBoru is the HOST-callback entry to a module fn (a fn value handed
// to Go and invoked outside the engine's own dispatch): it must gate on
// the stamped identity like every other route, or a host callback would
// be a way around the policy.
func TestModuleGateCallBoruArm(t *testing.T) {
	stamped := &FnSig{
		Params:     []FnParam{{Name: "x", Type: TInteger}},
		Returns:    []*Type{TInteger},
		Impl:       Boru([]Value{NewWord("x")}),
		BarrierPos: BarrierAllForward,
		ModuleCall: &ModuleCallID{Module: "boru:zz", Export: "denied"},
	}
	r := moduleGateReg(t, "boru:zz", "denied")
	if _, err := r.CallBoru(stamped, []Value{NewInteger(1)}, nil); err == nil ||
		!strings.Contains(err.Error(), "module export denied") {
		t.Fatalf("a denied module export must not run through CallBoru, got %v", err)
	}

	// Negative twin: an export the checker does not deny runs normally.
	allowed := *stamped
	allowed.ModuleCall = &ModuleCallID{Module: "boru:zz", Export: "fine"}
	out, err := r.CallBoru(&allowed, []Value{NewInteger(7)}, nil)
	if err != nil {
		t.Fatalf("an unruled export must run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, err := AsInteger(out[0]); err != nil || n != 7 {
		t.Errorf("body result = %v (%v), want 7", n, err)
	}

	// An UNSTAMPED sig (an ordinary, non-module fn) is never gated.
	plain := *stamped
	plain.ModuleCall = nil
	if _, err := r.CallBoru(&plain, []Value{NewInteger(3)}, nil); err != nil {
		t.Errorf("an unstamped fn must not be gated: %v", err)
	}
}

func TestStampedModuleCallNilAndNonFunction(t *testing.T) {
	// The compiled CALL_USER arm calls this with whatever registry the
	// unit carries; a nil one has no stamp to read.
	if id := StampedModuleCall(nil, "w"); id != nil {
		t.Errorf("a nil registry has no stamp, got %+v", id)
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// A name bound to a non-fn value contributes no stamp.
	r.Defs.Push("zz-val", NewInteger(1))
	if id := StampedModuleCall(r, "zz-val"); id != nil {
		t.Errorf("a value binding has no stamp, got %+v", id)
	}
}

// The legacy pure-stack dispatch path for a boru-defined fn VALUE
// (ExecFnDefSigStackMatch): a module fn reaching it carries the stamped
// identity on its own FnSig, and the gate must fire there too — this is
// the fn-value twin of execMatch's gate.
func TestModuleGateStackMatchArm(t *testing.T) {
	r := moduleGateReg(t, "boru:zz", "denied")
	fnDef := FnDefInfo{
		Anonymous: true,
		Signatures: []Signature{{
			Returns:    []*Type{TInteger},
			Impl:       Boru(parenBody(NewInteger(7))),
			BarrierPos: BarrierAllForward,
			ModuleCall: &ModuleCallID{Module: "boru:zz", Export: "denied"},
		}},
	}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewFunction(fnDef)}, StackHeadroom)
	e.Pointer = 0
	err := e.ExecFnDefSigStackMatch(0, &fnDef, nil)
	if err == nil || !strings.Contains(err.Error(), "module export denied") {
		t.Fatalf("the stack-match path must gate a stamped module fn, got %v", err)
	}

	// Negative twin: an unruled export dispatches through the same path.
	allowed := fnDef
	allowed.Signatures = []Signature{fnDef.Signatures[0]}
	allowed.Signatures[0].ModuleCall = &ModuleCallID{Module: "boru:zz", Export: "fine"}
	e2 := NewTop(r)
	e2.Tape = NewTape([]Value{NewFunction(allowed)}, StackHeadroom)
	e2.Pointer = 0
	if err := e2.ExecFnDefSigStackMatch(0, &allowed, nil); err != nil {
		t.Errorf("an unruled export must dispatch: %v", err)
	}
}
