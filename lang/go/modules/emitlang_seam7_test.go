package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// s7aEmitHandler is a trivial value→string emit handler used to exercise the
// installer and constructor arms without depending on real encoders.
func s7aEmitHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	return []native.Value{native.NewString("x")}, nil
}

// TestW8InstallBuiltinEmitterDuplicate drives installBuiltinEmitter's
// duplicate-key compile failure — defensive: the fixed built-in set is disjoint, so
// only a drifted EmitKinds could collide.
func TestW8InstallBuiltinEmitterDuplicate(t *testing.T) {
	r := s7aVMReg(t)
	exports := native.NewOrderedMap()
	spec := EmitLangSpec{Name: "dupw8", Handler: s7aEmitHandler}
	if err := installBuiltinEmitter(exports, r, spec); err != nil {
		t.Fatalf("first install should succeed: %v", err)
	}
	if err := installBuiltinEmitter(exports, r, spec); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Errorf("second install of the same name should error, got %v", err)
	}
}

// TestS7A_EmitRegisterTombstone drives the EmitLang.register tombstone
// directly: an unconditional emit_registry_frozen raise with the migration
// hint.
func TestS7A_EmitRegisterTombstone(t *testing.T) {
	r := s7aVMReg(t)
	_, err := emitRegisterFrozenHandler(nil, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "emit_registry_frozen") {
		t.Errorf("register tombstone must raise emit_registry_frozen, got %v", err)
	}
}

// TestS7A_NewEmitLangFnValidation drives the value constructor's validation
// arms: empty name, nil handler, a name RegisterNativeFunc declines, and the
// sub-registry construction error (through the newDefaultRegistry seam).
func TestS7A_NewEmitLangFnValidation(t *testing.T) {
	if _, err := NewEmitLangFn(EmitLangSpec{Name: "", Handler: s7aEmitHandler}); err == nil {
		t.Error("NewEmitLangFn: empty Name should error")
	}
	if _, err := NewEmitLangFn(EmitLangSpec{Name: "okname"}); err == nil {
		t.Error("NewEmitLangFn: nil Handler should error")
	}
	if _, err := NewEmitLangFn(EmitLangSpec{Name: "not a word", Handler: s7aEmitHandler}); err == nil {
		t.Error("NewEmitLangFn: an invalid word name should error")
	}
	if v, err := NewEmitLangFn(EmitLangSpec{Name: "okv", Handler: s7aEmitHandler}); err != nil || !native.IsConcrete(v) {
		t.Errorf("NewEmitLangFn should build a concrete fn value, got %v (err %v)", v, err)
	}

	orig := newDefaultRegistry
	t.Cleanup(func() { newDefaultRegistry = orig })
	newDefaultRegistry = func(_ ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errors.New("s7a registry boom")
	}
	if _, err := NewEmitLangFn(EmitLangSpec{Name: "x", Handler: s7aEmitHandler}); err == nil {
		t.Error("NewEmitLangFn: a registry construction error should propagate")
	}
}

// TestS7A_EmitKindHandlerCodelessError drives emitKindHandler's fallback arm:
// an encode error whose text has no recognised EmitErrorCode maps to the
// generic emit_error code.
func TestS7A_EmitKindHandlerCodelessError(t *testing.T) {
	r := s7aVMReg(t)
	h := emitKindHandler("fake", func(native.Value, map[string]any) (string, error) {
		return "", fmt.Errorf("plain codeless failure")
	})
	_, err := h([]native.Value{native.NewInteger(1), native.NewMap(native.NewOrderedMap())}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "emit_error") {
		t.Errorf("a codeless encode error should map to emit_error, got %v", err)
	}
}

// TestS7A_EmitAutoNoNatural drives emitAutoHandler's no-natural-format arm: a
// Function value has no natural emit kind.
func TestS7A_EmitAutoNoNatural(t *testing.T) {
	r := s7aVMReg(t)
	fn := native.NewFunction(native.FnDefInfo{Name: "f"})
	_, err := emitAutoHandler([]native.Value{fn, native.NewMap(native.NewOrderedMap())}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "emit_no_natural") {
		t.Errorf("a Function has no natural emit kind, got %v", err)
	}
}
