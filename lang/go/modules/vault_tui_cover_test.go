package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
	parser "github.com/boru-lang/boru/parser/go"
)

// ADR-008 coverage for BuildVaultTuiModule's guard arms, mirroring
// sift_cover_test.go — boru:vault-tui is loaded the same way (embedded
// vault_tui.boru parsed once, run as a module body). The app's live
// behaviour is driven end-to-end by lang/go/test/app_vault_tui_test.go.

// A parent with no parser configured is declined before any work.
func TestBuildVaultTuiModuleRequiresParser(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// No SetParseFunc on purpose.
	if _, berr := BuildVaultTuiModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parser not configured") {
		t.Errorf("BuildVaultTuiModule without a parser = %v, want parser-not-configured", berr)
	}
}

// The module loads headlessly (no backends registered — it only
// defines words), exports the VaultTui namespace, and inherits the
// host capability slots by pointer when they ARE registered.
func TestBuildVaultTuiModuleLoadsAndInheritsCaps(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	if rErr := RegisterHostTui(reg, TuiSpec{
		Name: "virtual",
		Open: func() (tuikit.Backend, error) { return tuikit.NewVirtualBackend(4, 2), nil },
	}); rErr != nil {
		t.Fatal(rErr)
	}
	if rErr := RegisterHostVault(reg, VaultSpec{
		Name: "fake",
		Do:   func(string, map[string]any) (any, error) { return nil, nil },
	}); rErr != nil {
		t.Fatal(rErr)
	}
	desc, berr := BuildVaultTuiModule(reg)
	if berr != nil {
		t.Fatalf("build: %v", berr)
	}
	exports := desc.Exports["VaultTui"]
	if exports == nil {
		t.Fatal("no VaultTui export")
	}
	for _, k := range []string{"app", "run"} {
		if _, ok := exports.Get(k); !ok {
			t.Errorf("export %q missing", k)
		}
	}
	// docs cover the whole export surface
	docs := moduleDocs["boru:vault-tui"]
	for _, k := range exports.Keys() {
		if docs[k] == "" {
			t.Errorf("export %q has no doc line", k)
		}
	}
}

// A cached preamble parse failure propagates on every subsequent
// build (the parse runs under a sync.Once, so the arm is driven by
// injecting into the cached slot).
func TestBuildVaultTuiModulePreambleParseError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	if _, berr := BuildVaultTuiModule(reg); berr != nil {
		t.Fatalf("baseline build: %v", berr)
	}
	orig := vaultTuiParseErr
	vaultTuiParseErr = errors.New("injected preamble parse failure")
	defer func() { vaultTuiParseErr = orig }()
	if _, berr := BuildVaultTuiModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "parse preamble") {
		t.Errorf("BuildVaultTuiModule with cached parse error = %v, want parse-preamble error", berr)
	}
}

// A parent whose module config carries an InitFunc has it applied to
// the sub-registry (the InheritConfig + InitFunc arm).
func TestBuildVaultTuiModuleUsesInheritedInitFunc(t *testing.T) {
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
	if _, berr := BuildVaultTuiModule(reg); berr != nil {
		t.Fatalf("BuildVaultTuiModule with InitFunc: %v", berr)
	}
	if !called {
		t.Error("inherited Modules.InitFunc was not applied to the sub-registry")
	}
}

// A parent that explicitly cleared its module InitFunc drives the
// native.Register fallback arm (InheritConfig copies the nil through;
// the resolver still rides in from the parent's module config).
func TestBuildVaultTuiModuleRegisterFallbackWithoutInitFunc(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	InstallResolver(reg)
	reg.Modules.InitFunc = nil
	if _, berr := BuildVaultTuiModule(reg); berr != nil {
		t.Fatalf("BuildVaultTuiModule without InitFunc: %v", berr)
	}
}

// A sub-registry that cannot resolve the preamble's own imports
// surfaces the preamble run failure — the parent here carries no
// native-module resolver, so `import "boru:tui"` fails inside the run.
func TestBuildVaultTuiModulePreambleRunError(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	// No InstallResolver on purpose.
	if _, berr := BuildVaultTuiModule(reg); berr == nil ||
		!strings.Contains(berr.Error(), "run preamble") {
		t.Errorf("BuildVaultTuiModule without a resolver = %v, want run-preamble error", berr)
	}
}
