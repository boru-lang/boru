package modules

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/boru-lang/boru/lang/go/native"
)

// The boru:cli module — the Cli namespace, command-line argument parsing
// (design/CLI-PROGRAMS.0.md §8, design/CLI-PROGRAMS.1.md §2–3).
//
// cli is WRITTEN IN boru (cliSource below, embedded from cli.boru), following the
// boru:sift precedent: the module body is parsed once and run in a fresh
// sub-registry that collects its `export "Cli" {…}`. The flag grammar, the
// usage renderer, and the dispatch decision are all boru; the loader below is
// the only Go.
//
// The surface splits along one line — what touches the world:
//
//	Cli.parse / Cli.dispatch / Cli.usage   pure: argv and a spec map in, a
//	                                       decision map out. No IO, ever, so
//	                                       they run under the spec runner with
//	                                       nothing installed, and every arm is
//	                                       reachable from a test.
//	Cli.main                               the imperative shell: prints, exits,
//	                                       and reads the terminal's width and
//	                                       colour support to hand to Cli.usage.
//
// argv is always an explicit PARAMETER rather than an internal `IO.args` read:
// `boru test` cannot inject an argument vector (IO.args is empty there), so a
// parser that read it internally would be untestable by construction.

//go:embed cli.boru
var cliSource string

var (
	cliParseOnce sync.Once
	cliParsed    []native.Value
	cliParseErr  error
)

// BuildCliModule loads the boru-implemented boru:cli module: it parses the
// embedded cli.boru once, runs it in a fresh sub-registry that inherits the
// parser + module resolver (so the body's own `import`s resolve), and collects
// the `export "Cli" {…}` map. Mirrors BuildSiftModule.
func BuildCliModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("cli: parser not configured")
	}
	cliParseOnce.Do(func() {
		cliParsed, cliParseErr = parent.ParseFunc(cliSource)
	})
	if cliParseErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("cli: parse preamble: %w", cliParseErr)
	}

	modReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, fmt.Errorf("cli: init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	modReg.Effects = parent.Effects
	modReg.InheritObserveHooks(parent)
	// Coverage tagging (coverage.go): cli's fn bodies run on this registry, so
	// a coverage run attributes their executed rows to "boru:cli", and
	// registering the source gives boru:test the denominator. Inert unless a
	// coverage hook is armed — but load-bearing when one is: the boru-line
	// coverage gate (cli_coverage_test.go) is the only thing standing between
	// cli.boru and untested code, since nothing meta-tests that a boru module
	// has such a gate at all.
	modReg.SetCoverID("boru:cli")
	parent.RegisterCoverSource("boru:cli", cliSource)
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir
	modReg.Modules.InheritConfig(parent.Modules)
	// Host seams that opted in (ModuleInheritedCaps) ride into the module's
	// sub-registry: Cli.main reads the environment (COLUMNS, NO_COLOR) and the
	// terminal probe through boru:io, and those are host capabilities, not
	// ambient state. Without the copy they would read as uninstalled here
	// while the importing program saw the real ones.
	if parent.Capabilities != nil && modReg.Capabilities != nil {
		for _, capName := range native.ModuleInheritedCaps {
			if v, capOK, capErr := parent.Capabilities.Get(capName); capErr == nil && capOK {
				_ = modReg.Capabilities.Set(capName, v)
			}
		}
	}
	if modReg.Modules.InitFunc != nil {
		modReg.Modules.InitFunc(modReg)
	} else {
		native.Register(modReg)
	}

	// Collect `export "Cli" {…}` from the preamble (the boru:test-style local
	// exporter; see modules/test.go for why RunModuleBody cannot be reused).
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
	_, runErr := sub.Run(cliParsed)
	if runErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("cli: run preamble: %w", runErr)
	}

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: exports,
	}, nil
}
