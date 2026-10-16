package modules

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/boru-lang/boru/lang/go/native"
)

// The boru:vault-tui module — the VaultTui namespace: the interactive
// vault TUI WRITTEN IN boru (vaultTuiSource below, embedded from
// vault_tui.boru), the application layer of design/VAULT-TUI-PORT.0.md.
// Follows the boru:sift hybrid pattern: the module body is parsed once
// and run in a fresh sub-registry that collects its `export "VaultTui"
// {…}`. The app itself is all boru — this loader is the only Go.
//
// The app reaches the terminal and the vault through the host seams
// (RegisterHostTui / RegisterHostVault). Both capability slots ride
// into the sub-registry by pointer below — the same
// ModuleInheritedCaps contract file modules get from RunModuleBody —
// so hosts MUST register their backends before the program imports
// "boru:vault-tui". With no backends the module still loads (it only
// defines words); the app raises `no_backend` when run.

//go:embed vault_tui.boru
var vaultTuiSource string

var (
	vaultTuiParseOnce sync.Once
	vaultTuiParsed    []native.Value
	vaultTuiParseErr  error
)

// BuildVaultTuiModule loads the boru-implemented boru:vault-tui module:
// it parses the embedded vault_tui.boru once, runs it in a fresh
// sub-registry that inherits the parser + module resolver + host
// capability slots, and collects the `export "VaultTui" {…}` map.
// Mirrors BuildSiftModule.
func BuildVaultTuiModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("vault-tui: parser not configured")
	}
	vaultTuiParseOnce.Do(func() {
		vaultTuiParsed, vaultTuiParseErr = parent.ParseFunc(vaultTuiSource)
	})
	if vaultTuiParseErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("vault-tui: parse preamble: %w", vaultTuiParseErr)
	}

	modReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, fmt.Errorf("vault-tui: init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	modReg.Effects = parent.Effects
	modReg.InheritObserveHooks(parent)
	// Coverage attribution: the app's fn bodies run on this registry
	// (coverage.go). Inert unless a coverage hook is armed.
	modReg.SetCoverID("boru:vault-tui")
	parent.RegisterCoverSource("boru:vault-tui", vaultTuiSource)
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir
	modReg.Modules.InheritConfig(parent.Modules)
	if modReg.Modules.InitFunc != nil {
		modReg.Modules.InitFunc(modReg)
	} else {
		native.Register(modReg)
	}
	// Host seams that opted in (ModuleInheritedCaps) ride into the
	// sub-registry by POINTER — the terminal and vault backends the app
	// dispatches against (RunModuleBody does the same for file modules).
	for _, capName := range native.ModuleInheritedCaps {
		if v, capOK, capErr := parent.Capabilities.Get(capName); capErr == nil && capOK {
			_ = modReg.Capabilities.Set(capName, v)
		}
	}

	// Collect `export "VaultTui" {…}` from the preamble (the boru:sift
	// local exporter; see modules/test.go for why RunModuleBody cannot
	// be reused).
	exports := map[string]*native.OrderedMap{}
	modReg.Defs.Delete("export")
	modReg.RegisterNativeFunc(native.NativeFunc{
		Name: "export",
		Signatures: []native.Signature{{
			Args: []*native.Type{native.TString, native.TMap},
			Impl: native.Go(func(eargs []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				name, _ := eargs[0].AsConcreteString()
				return resolveExport(modReg, exports, name, eargs[1])
			}),
			Returns: []*native.Type{}, BarrierPos: -1,
		}},
	})

	sub := native.New(modReg)
	_, runErr := sub.Run(vaultTuiParsed)
	if runErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("vault-tui: run preamble: %w", runErr)
	}

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: exports,
	}, nil
}
