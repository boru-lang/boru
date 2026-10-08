package core

import (
	"errors"
	"strings"
	"testing"
)

// withErrorCodeRegistry isolates the package-level enumeration around a test
// that registers into it, so a deliberately-bad registration cannot leak into
// the shared registry every other test builds a Registry against.
func withErrorCodeRegistry(t *testing.T) {
	t.Helper()
	savedReg := errorCodeRegistry
	savedErrs := errorCodeInitErrs
	copied := make(map[string]ErrorCode, len(savedReg))
	for k, v := range savedReg {
		copied[k] = v
	}
	errorCodeRegistry = copied
	t.Cleanup(func() {
		errorCodeRegistry = savedReg
		errorCodeInitErrs = savedErrs
	})
}

// TestErrorCodesEnumerationIsPopulated is the floor: the kernel's own codes
// are registered by the package initialiser, so a build that links eng can
// answer "what can this raise?" without scanning source. If this drops to
// zero the initialiser stopped running and every drift check downstream
// (test/go/docexamples/errorcodes_test.go) passes vacuously.
func TestErrorCodesEnumerationIsPopulated(t *testing.T) {
	codes := ErrorCodes()
	if len(codes) < 40 {
		t.Fatalf("only %d codes registered — the kernel list alone had 45 when "+
			"this was written; the initialiser is not running", len(codes))
	}
	// Sorted by name, so callers can present the list without re-sorting.
	for i := 1; i < len(codes); i++ {
		if codes[i-1].Code >= codes[i].Code {
			t.Fatalf("ErrorCodes() is not sorted: %q before %q",
				codes[i-1].Code, codes[i].Code)
		}
	}
	// A code every layer agrees on, and the negative half: a name nothing
	// registers must report absent, because "absent" is the answer that tells
	// a caller a `case` arm on it can never fire.
	if ec, ok := LookupErrorCode("undefined_word"); !ok || ec.Owner != OwnerKernel {
		t.Errorf("undefined_word = (%+v, %v), want an OwnerKernel entry", ec, ok)
	}
	if _, ok := LookupErrorCode("no_such_code_exists"); ok {
		t.Error("an unregistered name must report absent, not resolve")
	}
}

// TestRegisterErrorCodesRejectsABadName pins the naming rule. It is enforced
// at registration rather than by review because the rule's whole purpose is
// to hold for codes nobody has looked at yet.
func TestRegisterErrorCodesRejectsABadName(t *testing.T) {
	withErrorCodeRegistry(t)
	errorCodeInitErrs = nil

	RegisterErrorCodes("boru:test", "NotSnakeCase")
	err := ErrorCodeInitError()
	if err == nil {
		t.Fatal("a code that cannot be spelled in a case arm must be declined")
	}
	if !strings.Contains(err.Error(), "NotSnakeCase") {
		t.Errorf("the error must name the offending code, got %v", err)
	}
	if _, ok := LookupErrorCode("NotSnakeCase"); ok {
		t.Error("a declined code must not be registered anyway")
	}
}

// TestRegisterErrorCodesRejectsTwoOwners pins the other init-time compile failure.
// Two layers each believing they define a code is precisely the drift the
// enumeration exists to catch: whichever registers second would silently
// inherit the other's meaning.
func TestRegisterErrorCodesRejectsTwoOwners(t *testing.T) {
	withErrorCodeRegistry(t)
	errorCodeInitErrs = nil

	RegisterErrorCodes("boru:one", "shared_code_probe")
	if err := ErrorCodeInitError(); err != nil {
		t.Fatalf("the first registration must succeed: %v", err)
	}
	// Same owner again is a NO-OP, not an error: a layer whose initialiser
	// runs twice must stay harmless.
	RegisterErrorCodes("boru:one", "shared_code_probe")
	if err := ErrorCodeInitError(); err != nil {
		t.Fatalf("re-registering under the same owner must be a no-op: %v", err)
	}

	RegisterErrorCodes("boru:two", "shared_code_probe")
	err := ErrorCodeInitError()
	if err == nil {
		t.Fatal("two owners for one code must be declined")
	}
	if !strings.Contains(err.Error(), "boru:one") || !strings.Contains(err.Error(), "boru:two") {
		t.Errorf("the error must name both claimants, got %v", err)
	}
	// The first owner keeps it — a declined second claim must not overwrite.
	if ec, _ := LookupErrorCode("shared_code_probe"); ec.Owner != "boru:one" {
		t.Errorf("owner = %q, want the first claimant to keep it", ec.Owner)
	}
}

// TestNewRegistrySurfacesErrorCodeInitError covers the ADR-005 path: an
// init-time programmer error is returned from construction rather than
// panicking or being silently accepted into a dispatch contract.
//
// Driven through the errorCodeInitError seam (eng_rest_seam9.go) for the same
// reason as its builtinInitError twin: in a valid build the real function is
// always nil — every layer's list conforms, and the doc gate keeps it that
// way — so the seam is the only way to reach the arm without corrupting
// shared package state.
func TestNewRegistrySurfacesErrorCodeInitError(t *testing.T) {
	orig := errorCodeInitError
	t.Cleanup(func() { errorCodeInitError = orig })
	errorCodeInitError = func() error { return errors.New("codes init boom") }

	r, err := NewRegistry()
	if err == nil {
		t.Fatal("NewRegistry must report an error-code init failure")
	}
	if r != nil {
		t.Error("no Registry should be handed back alongside the error")
	}
	if !strings.Contains(err.Error(), "codes init boom") {
		t.Errorf("the underlying error must survive, got %v", err)
	}
}
