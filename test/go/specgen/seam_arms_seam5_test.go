package main

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// run()'s registry-construction arm: only drivable through the seam
// (native.DefaultRegistry cannot fail on a healthy build).
func TestRunRegistryFailure(t *testing.T) {
	prev := newNativeRegistry
	t.Cleanup(func() { newNativeRegistry = prev })
	newNativeRegistry = func(...func(*core.Registry)) (*core.Registry, error) {
		return nil, errors.New("registry boom")
	}
	if _, err := run("1"); err == nil || !strings.Contains(err.Error(), "registry boom") {
		t.Errorf("run with failing registry: got %v, want registry boom", err)
	}
}

// canonOf's parse- and run-error arms are unreachable through the
// extendFive worker (CompileCheck has already parsed and vetted the
// candidate), so they are pinned directly.
func TestCanonOfErrorArms(t *testing.T) {
	reg, err := newNativeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := canonOf(reg, "]"); got != "" {
		t.Errorf("canonOf(unparsable) = %q, want \"\"", got)
	}
	if got := canonOf(reg, "drop"); got != "" {
		t.Errorf("canonOf(run-error) = %q, want \"\"", got)
	}
	if got := canonOf(reg, "1"); got == "" {
		t.Error("canonOf(\"1\") = \"\", want the canonical value")
	}
}
