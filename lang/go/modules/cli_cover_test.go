package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// ADR-008 coverage for BuildCliModule's guard arms, which the behavioural
// suite (lang/spec/module-cli.tsv + cli_test.boru) cannot reach: every one of
// them is a loader failure, and a loader that failed would take the whole
// module — and therefore every behavioural case — with it.
//
// This is the exact twin of sift_cover_test.go, because BuildCliModule is the
// exact twin of BuildSiftModule. It should have landed with the module; it did
// not, and the gate caught the four statements.

// A parent with no parser configured is declined before any work.
func TestBuildCliModuleRequiresParser(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// No SetParseFunc on purpose.
	if _, berr := BuildCliModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parser not configured") {
		t.Errorf("BuildCliModule without a parser = %v, want parser-not-configured", berr)
	}
}

// A cached preamble parse failure propagates on every subsequent build. The
// parse runs under a sync.Once (already fired by the suite's successful
// builds), so the error arm is driven by injecting into the cached slot and
// restoring afterwards.
func TestBuildCliModulePreambleParseError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	if _, berr := BuildCliModule(reg); berr != nil {
		t.Fatalf("baseline build: %v", berr)
	}
	orig := cliParseErr
	cliParseErr = errors.New("injected preamble parse failure")
	defer func() { cliParseErr = orig }()
	if _, berr := BuildCliModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parse preamble") {
		t.Errorf("BuildCliModule with cached parse error = %v, want parse-preamble error", berr)
	}
}

// A parent whose module config carries an InitFunc has it applied to the
// sub-registry instead of the plain native.Register fallback.
func TestBuildCliModuleUsesInheritedInitFunc(t *testing.T) {
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
	if _, berr := BuildCliModule(reg); berr != nil {
		t.Fatalf("BuildCliModule with InitFunc: %v", berr)
	}
	if !called {
		t.Error("inherited Modules.InitFunc was not applied to the sub-registry")
	}
}

// A parent that explicitly cleared its module InitFunc drives the
// native.Register fallback arm (InheritConfig copies the nil through).
func TestBuildCliModuleRegisterFallbackWithoutInitFunc(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	reg.Modules.InitFunc = nil
	if _, berr := BuildCliModule(reg); berr != nil {
		t.Fatalf("BuildCliModule without InitFunc: %v", berr)
	}
}

// A sub-registry that cannot resolve the preamble's own imports surfaces the
// preamble run failure — the parent here carries no native-module resolver, so
// cli.boru's `import "boru:string-util"` fails inside the run.
func TestBuildCliModulePreambleRunError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	// No InstallResolver on purpose.
	if _, berr := BuildCliModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "run preamble") {
		t.Errorf("BuildCliModule without a resolver = %v, want run-preamble error", berr)
	}
}
