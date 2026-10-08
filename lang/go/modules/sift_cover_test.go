package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// ADR-008 coverage for BuildSiftModule's guard arms not reached by the
// behavioural suite (lang/spec/module-sift.tsv + sift_test.go) or the seam
// battery (modules_seam6d_test.go). Mirrors repl_cover_test.go — boru:sift is
// loaded the same way (embedded sift.boru parsed once, run as a module body).

// A parent with no parser configured is declined before any work.
func TestBuildSiftModuleRequiresParser(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// No SetParseFunc on purpose.
	if _, berr := BuildSiftModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parser not configured") {
		t.Errorf("BuildSiftModule without a parser = %v, want parser-not-configured", berr)
	}
}

// A cached preamble parse failure propagates on every subsequent build. The
// parse runs under a sync.Once (already fired by the suite's successful
// builds), so the error arm is driven by injecting into the cached slot and
// restoring afterwards.
func TestBuildSiftModulePreambleParseError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	if _, berr := BuildSiftModule(reg); berr != nil {
		t.Fatalf("baseline build: %v", berr)
	}
	orig := siftParseErr
	siftParseErr = errors.New("injected preamble parse failure")
	defer func() { siftParseErr = orig }()
	if _, berr := BuildSiftModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parse preamble") {
		t.Errorf("BuildSiftModule with cached parse error = %v, want parse-preamble error", berr)
	}
}

// A parent whose module config carries an InitFunc has it applied to the
// sub-registry (the InheritConfig + InitFunc arm) instead of the plain
// native.Register fallback.
func TestBuildSiftModuleUsesInheritedInitFunc(t *testing.T) {
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
	if _, berr := BuildSiftModule(reg); berr != nil {
		t.Fatalf("BuildSiftModule with InitFunc: %v", berr)
	}
	if !called {
		t.Error("inherited Modules.InitFunc was not applied to the sub-registry")
	}
}

// A parent that explicitly cleared its module InitFunc drives the
// native.Register fallback arm (InheritConfig copies the nil through).
func TestBuildSiftModuleRegisterFallbackWithoutInitFunc(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	reg.Modules.InitFunc = nil
	if _, berr := BuildSiftModule(reg); berr != nil {
		t.Fatalf("BuildSiftModule without InitFunc: %v", berr)
	}
}

// A sub-registry that cannot resolve the preamble's own imports surfaces the
// preamble run failure (the run-error arm) — the parent here carries no
// native-module resolver, so `import "boru:string-util"` fails inside the run.
func TestBuildSiftModulePreambleRunError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	// No InstallResolver on purpose.
	if _, berr := BuildSiftModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "run preamble") {
		t.Errorf("BuildSiftModule without a resolver = %v, want run-preamble error", berr)
	}
}
