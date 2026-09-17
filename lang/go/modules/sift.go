package modules

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/boru-lang/boru/lang/go/native"
)

// The boru:sift module — the Sift namespace, the semi-structured-text tier of
// the parsing stack (design/legacy/SIFT.0.ignore). Between StringUtil / read {fmt:'lines'}
// (primitives) and parse json / boru:parse (rigid formats / grammars), sift
// parses the loose line- and field-oriented text Unix tools and /proc files
// emit.
//
// sift is WRITTEN IN boru (siftSource below, embedded from sift.boru), following
// the boru:repl / boru:test hybrid pattern: the module body is parsed once and
// run in a fresh sub-registry that collects its `export "Sift" {…}`. The six
// format families (kv, blocks, columns, dsv, fixed, pattern), the spec engine,
// the coercion vocabulary, and the seven Sift words are all boru — the loader
// below is the only Go.
//
// Surface (v1, design decision "Pure boru, Sift.* surface"): every kind is
// reached through the Sift namespace — `Sift.parse <kind> <src>`,
// `Sift.define`, `Sift.kinds`, etc. A module body runs in a discarded
// sub-registry, so it cannot register a global `parse <kind>` macro kind the
// importing program would see (that needs the parent registry, which only the
// Go loader holds); the namespaced surface is the coherent pure-Boru form.

//go:embed sift.boru
var siftSource string

var (
	siftParseOnce sync.Once
	siftParsed    []native.Value
	siftParseErr  error
)

// BuildSiftModule loads the boru-implemented boru:sift module: it parses the
// embedded sift.boru once, runs it in a fresh sub-registry that inherits the
// parser + module resolver (so the body's own `import`s resolve), and collects
// the `export "Sift" {…}` map. Mirrors BuildReplModule.
func BuildSiftModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("sift: parser not configured")
	}
	siftParseOnce.Do(func() {
		siftParsed, siftParseErr = parent.ParseFunc(siftSource)
	})
	if siftParseErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("sift: parse preamble: %w", siftParseErr)
	}

	modReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, fmt.Errorf("sift: init: %w", err)
	}
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	modReg.Effects = parent.Effects
	modReg.InheritObserveHooks(parent)
	// Tag sift's sub-registry as a coverage target (coverage.go): sift's fn
	// bodies run on this registry, so a coverage run attributes their executed
	// rows to "boru:sift". Inert unless a coverage hook is armed. Registering the
	// source lets boru:test build the coverage denominator (all executable rows).
	modReg.SetCoverID("boru:sift")
	parent.RegisterCoverSource("boru:sift", siftSource)
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir
	modReg.Modules.InheritConfig(parent.Modules)
	if modReg.Modules.InitFunc != nil {
		modReg.Modules.InitFunc(modReg)
	} else {
		native.Register(modReg)
	}

	// Collect `export "Sift" {…}` from the preamble (the boru:test-style local
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

	tokens := append([]native.Value(nil), siftParsed...)
	sub := native.New(modReg)
	restoreAtt := modReg.SetInterpAttribution("module-load")
	_, runErr := sub.Run(tokens)
	restoreAtt()
	if runErr != nil {
		return native.ModuleDesc{}, fmt.Errorf("sift: run preamble: %w", runErr)
	}

	return native.ModuleDesc{
		ID:      parent.Modules.NextID(),
		Exports: exports,
	}, nil
}
