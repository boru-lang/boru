package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// ADR-008 coverage for BuildReplModule's guard arms not reached by the
// behavioural suite (repl_test.go) or the seam battery (modules_seam6d_test.go).

// A parent with no parser configured is declined before any work.
func TestBuildReplModuleRequiresParser(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// No SetParseFunc on purpose.
	if _, berr := BuildReplModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parser not configured") {
		t.Errorf("BuildReplModule without a parser = %v, want parser-not-configured", berr)
	}
}

// A cached preamble parse failure propagates on every subsequent build.
// The parse runs under a sync.Once, so the error arm is driven by
// injecting into the cached slot (the once has already fired for the
// suite's successful builds) and restoring afterwards.
func TestBuildReplModulePreambleParseError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	// Ensure the once has fired with the real preamble first.
	if _, berr := BuildReplModule(reg); berr != nil {
		t.Fatalf("baseline build: %v", berr)
	}
	orig := replParseErr
	replParseErr = errors.New("injected preamble parse failure")
	defer func() { replParseErr = orig }()
	if _, berr := BuildReplModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parse preamble") {
		t.Errorf("BuildReplModule with cached parse error = %v, want parse-preamble error", berr)
	}
}

// A parent whose module config carries an InitFunc has it applied to the
// sub-registry (the InheritConfig + InitFunc arm) instead of the plain
// native.Register fallback.
func TestBuildReplModuleUsesInheritedInitFunc(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	called := false
	reg.Modules.InitFunc = func(r *native.Registry) {
		called = true
		native.Register(r)
		InstallResolver(r)
	}
	if _, berr := BuildReplModule(reg); berr != nil {
		t.Fatalf("BuildReplModule with InitFunc: %v", berr)
	}
	if !called {
		t.Error("inherited Modules.InitFunc was not applied to the sub-registry")
	}
}

// A parent that explicitly cleared its module InitFunc drives the
// native.Register fallback arm (InheritConfig copies the nil through).
func TestBuildReplModuleRegisterFallbackWithoutInitFunc(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	reg.Modules.InitFunc = nil
	if _, berr := BuildReplModule(reg); berr != nil {
		t.Fatalf("BuildReplModule without InitFunc: %v", berr)
	}
}

// A sub-registry that cannot resolve the preamble's own imports surfaces
// the preamble run failure (the run-error arm) — the parent here carries
// no native-module resolver, so `import "boru:net"` fails inside the
// preamble run.
func TestBuildReplModulePreambleRunError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	// No InstallResolver on purpose.
	if _, berr := BuildReplModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "run preamble") {
		t.Errorf("BuildReplModule without a resolver = %v, want run-preamble error", berr)
	}
}
