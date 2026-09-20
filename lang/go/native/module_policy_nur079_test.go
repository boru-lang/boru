package native

import (
	"errors"
	"testing"
)

// NUR079 regression: a module body must be governed by the same policy as
// the code that imported it.
//
// The bypass this guards was reachable by RELOCATION, not by any privileged
// operation: a gated call declined at top level was permitted verbatim one
// file deeper, because the module sub-registry carried no CapPolicy and
// HostPolicy returns nil — which every gate that resolves the policy itself
// reads as allow-everything.
//
// The capability-wrapping gates were never in the hole (they inherit the
// parent's already-wrapped backend by pointer), so a file write inside a
// module body stayed declined while a network fetch in the same body went
// through. That asymmetry is why the assertions below are on the POLICY
// SLOT and on a policy-resolving gate, not on fileops.
//
// captureSubRegistry installs a newSubRegistry seam that records the
// sub-registry handed to the module body, so a test can inspect what the
// body actually ran under.
func captureSubRegistry(t *testing.T) func() *Registry {
	t.Helper()
	var captured *Registry
	old := newSubRegistry
	newSubRegistry = func(opts ...func(*Registry)) (*Registry, error) {
		r, err := old(opts...)
		captured = r
		return r, err
	}
	t.Cleanup(func() { newSubRegistry = old })
	return func() *Registry { return captured }
}

func TestNUR079ModuleBodyInheritsPolicy(t *testing.T) {
	parent, err := DefaultRegistryWithPolicy(loadPolicy(t, "read-only"))
	if err != nil {
		t.Fatal(err)
	}
	get := captureSubRegistry(t)

	if _, err := RunModuleBody(parent, nil); err != nil {
		t.Fatalf("RunModuleBody: %v", err)
	}

	sub := get()
	if sub == nil {
		t.Fatal("no sub-registry was created")
	}
	if got := HostPolicy(sub); got == nil {
		t.Fatal("module body ran with NO policy: the NUR079 bypass is open — " +
			"a gated call declined at top level would be permitted one file deeper")
	}
	// The gate that was actually escaping is one that resolves the policy at
	// dispatch, so assert through such a gate rather than only on the slot:
	// read-only denies networking, so the body's fetch must now decline.
	if err := checkFetchPolicy(sub, "https://example.com/foo"); err == nil {
		t.Error("checkFetchPolicy permitted a fetch inside a module body under " +
			"read-only; the policy is installed but not reaching the gate")
	} else {
		var be *BoruError
		if !errors.As(err, &be) {
			t.Errorf("compile failure should be a coded BoruError, got %T (%v)", err, err)
		}
	}
}

// The negative half: no policy on the parent must stay no policy on the
// child. HostPolicy's nil result is the documented "no permissions
// configured" posture, and inheriting must not manufacture one — a module
// body under an unconfigured parent has to keep running unrestricted.
func TestNUR079ModuleBodyWithoutPolicyStaysUnrestricted(t *testing.T) {
	parent := seam5Reg(t)
	if HostPolicy(parent) != nil {
		t.Fatal("test precondition: parent should carry no policy")
	}
	get := captureSubRegistry(t)

	if _, err := RunModuleBody(parent, nil); err != nil {
		t.Fatalf("RunModuleBody: %v", err)
	}

	sub := get()
	if sub == nil {
		t.Fatal("no sub-registry was created")
	}
	if got := HostPolicy(sub); got != nil {
		t.Errorf("module body gained a policy its parent never had: %#v", got)
	}
	if err := checkFetchPolicy(sub, "https://example.com/foo"); err != nil {
		t.Errorf("an unconfigured parent must leave the body unrestricted: %v", err)
	}
}
