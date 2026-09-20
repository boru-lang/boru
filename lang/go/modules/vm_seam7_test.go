package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	parser "github.com/boru-lang/boru/parser/go"
)

// s7aVMReg builds a parent registry wired like production import resolution
// (parser installed, module resolver wired) — the shape BuildVMModule and the
// vm handlers expect.
func s7aVMReg(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	InstallResolver(r)
	return r
}

// s7aVMHandler returns the Go handler of the named vm native (white-box: the
// handlers are closures over parent, so we drive them directly to reach the
// arg-validation / policy-load / sub-engine-init error arms that a dot-access
// dispatch could never feed a malformed value to).
func s7aVMHandler(t *testing.T, parent *native.Registry, name string) native.Handler {
	t.Helper()
	for _, nf := range vmNatives(parent) {
		if nf.Name == name {
			h := nf.Signatures[0].DispatchHandler()
			if h == nil {
				t.Fatalf("vm native %q has no dispatch handler", name)
			}
			return h
		}
	}
	t.Fatalf("vm native %q not found", name)
	return nil
}

// TestS7A_VMBuildNoParser drives BuildVMModule's parser-not-configured arm: a
// parent whose ParseFunc is nil cannot run a sub-engine, so the build declines.
func TestS7A_VMBuildNoParser(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.ParseFunc = nil // ensure the guard arm is reached
	if _, err := BuildVMModule(r); err == nil ||
		!strings.Contains(err.Error(), "parser not configured") {
		t.Fatalf("expected parser-not-configured error, got %v", err)
	}
}

// TestS7A_VMHandlerBadArg drives every vm word's AsConcreteString error arm by
// feeding a non-concrete String (a bare type literal) straight to the handler.
func TestS7A_VMHandlerBadArg(t *testing.T) {
	parent := s7aVMReg(t)
	badStr := native.NewTypeLiteral(native.TString)
	emptyMap := native.NewMap(native.NewOrderedMap())
	cases := []struct {
		name string
		args []native.Value
	}{
		{"vm-run", []native.Value{badStr}},
		{"vm-run-sandbox", []native.Value{badStr}},
		{"vm-run-compute", []native.Value{badStr}},
		{"vm-parse", []native.Value{badStr}},
		{"vm-check", []native.Value{badStr}},
		{"vm-compile", []native.Value{badStr}},
		{"vm-run-with", []native.Value{badStr, emptyMap}},
	}
	for _, c := range cases {
		h := s7aVMHandler(t, parent, c.name)
		if _, err := h(c.args, nil, nil, parent); err == nil {
			t.Errorf("%s: expected an error for a non-concrete String arg", c.name)
		}
	}
}

// TestS7A_VMPolicyLoadError drives the three run words' profile-load error
// arms via the s7aPolicyLoad seam — the built-in sandbox / compute profiles
// otherwise always compile.
func TestS7A_VMPolicyLoadError(t *testing.T) {
	parent := s7aVMReg(t)
	orig := s7aPolicyLoad
	s7aPolicyLoad = func(string) (policy.Policy, error) {
		return nil, fmt.Errorf("s7a-boom-load")
	}
	t.Cleanup(func() { s7aPolicyLoad = orig })

	code := native.NewString("1")
	for _, name := range []string{"vm-run", "vm-run-sandbox", "vm-run-compute"} {
		h := s7aVMHandler(t, parent, name)
		if _, err := h([]native.Value{code}, nil, nil, parent); err == nil ||
			!strings.Contains(err.Error(), "s7a-boom-load") {
			t.Errorf("%s: expected the seeded policy-load error, got %v", name, err)
		}
	}
}

// TestS7A_VMRunWithBadPolicyMap drives vm-run-with's policyFromMapValue error
// arm: a concrete code string but a non-concrete (carrier) policy map.
func TestS7A_VMRunWithBadPolicyMap(t *testing.T) {
	parent := s7aVMReg(t)
	h := s7aVMHandler(t, parent, "vm-run-with")
	args := []native.Value{native.NewString("1"), native.NewDynamicCarrier(native.TMap)}
	if _, err := h(args, nil, nil, parent); err == nil ||
		!strings.Contains(err.Error(), "Vm.run-with") {
		t.Errorf("expected a Vm.run-with policy error, got %v", err)
	}
}

// TestS7A_VMRunParseError drives runInSubEngine's parse-failure arm through the
// vm-run handler with malformed source (policy loads fine; the parse fails).
func TestS7A_VMRunParseError(t *testing.T) {
	parent := s7aVMReg(t)
	h := s7aVMHandler(t, parent, "vm-run")
	if _, err := h([]native.Value{native.NewString("1 add (((")}, nil, nil, parent); err == nil ||
		!strings.Contains(err.Error(), "vm: parse") {
		t.Errorf("expected a vm parse error, got %v", err)
	}
}

// TestS7A_VMSubEngineInitError drives every sub-engine-construction error arm
// (runInSubEngine / checkInSubEngine / compileInSubEngine, and the vm-check /
// vm-compile handlers that propagate them) via the shared
// newDefaultRegistryWithPolicy seam.
func TestS7A_VMSubEngineInitError(t *testing.T) {
	parent := s7aVMReg(t)
	orig := newDefaultRegistryWithPolicy
	newDefaultRegistryWithPolicy = func(policy.Policy, ...func(*native.Registry)) (*native.Registry, error) {
		return nil, fmt.Errorf("s7a-boom-init")
	}
	t.Cleanup(func() { newDefaultRegistryWithPolicy = orig })

	for _, name := range []string{"vm-run", "vm-check", "vm-compile"} {
		h := s7aVMHandler(t, parent, name)
		if _, err := h([]native.Value{native.NewString("1")}, nil, nil, parent); err == nil ||
			!strings.Contains(err.Error(), "s7a-boom-init") {
			t.Errorf("%s: expected the seeded sub-engine init error, got %v", name, err)
		}
	}
}

// TestS7A_VMCheckSynthDiag drives checkInSubEngine's synthesized-diagnostic arm
// (a hard check error that left no diagnostic): `gen` outside a type
// constructor raises at check time with no diagnostic, so check synthesizes a
// check_error entry.
func TestS7A_VMCheckSynthDiag(t *testing.T) {
	parent := s7aVMReg(t)
	v, err := checkInSubEngine(parent, "gen [x:Integer]")
	if err != nil {
		t.Fatalf("checkInSubEngine must not raise: %v", err)
	}
	got := native.Canon([]native.Value{v})
	if !strings.Contains(got, "check_error") || !strings.Contains(got, "ok:false") {
		t.Errorf("expected a synthesized check_error diagnostic, got %s", got)
	}
}

// TestS7A_VMCompileArms drives compileInSubEngine's three decline arms:
// runErr (check error), a check diagnostic, and a check-mode-suppressed runtime
// error — plus hasCheckError's iteration via the check-diagnostics case.
func TestS7A_VMCompileArms(t *testing.T) {
	parent := s7aVMReg(t)
	cases := []struct{ name, src, wantReason string }{
		{"runErr", "gen [x:Integer]", "check error"},
		{"checkDiag", "totally-undefined-word", "check diagnostics"},
		{"suppressed", `def f fn [[] [Any] [unpack [a] {b:1}]]  f`,
			"check-mode suppressed a runtime error (uncompilable)"},
	}
	for _, c := range cases {
		v, err := compileInSubEngine(parent, c.src)
		if err != nil {
			t.Fatalf("%s: compileInSubEngine must not raise: %v", c.name, err)
		}
		got := native.Canon([]native.Value{v})
		if !strings.Contains(got, c.wantReason) {
			t.Errorf("%s: reason %q not found in %s", c.name, c.wantReason, got)
		}
	}
}

// TestS7A_VMSeverityStringEmpty pins severityString's empty→"info" arm and its
// pass-through for a real severity.
func TestS7A_VMSeverityStringEmpty(t *testing.T) {
	if got := severityString(""); got != "info" {
		t.Errorf(`severityString("") = %q, want "info"`, got)
	}
	if got := severityString(native.SeverityWarning); got != string(native.SeverityWarning) {
		t.Errorf("severityString(warning) = %q, want %q", got, native.SeverityWarning)
	}
}

// TestS7A_VMPolicyFromMapValueErrors pins policyFromMapValue's two compile failure
// arms: a non-concrete (carrier) value and a concrete non-map value.
func TestS7A_VMPolicyFromMapValueErrors(t *testing.T) {
	if _, err := policyFromMapValue(native.NewDynamicCarrier(native.TMap)); err == nil ||
		!strings.Contains(err.Error(), "type literal or carrier") {
		t.Errorf("carrier map should be rejected, got %v", err)
	}
	if _, err := policyFromMapValue(native.NewInteger(5)); err == nil {
		t.Error("a concrete non-map value should be rejected")
	}
}
