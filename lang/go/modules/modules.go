// Package modules provides built-in native boru modules that are imported
// using names of the form "boru:<name>". Each native module contains both
// Go-implemented words and boru code definitions.
//
// Native modules produce a ModuleDesc with exports, just like file-based
// modules. The exported words are accessed via dot notation:
//
//	import "boru:math-util"
//	0.5 MathUtil.sin          # access sin via the math export
//	3 MathUtil.min 7          # min of 3 and 7
package modules

import (
	"fmt"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// modules maps native module names to their builder functions.
// Each builder creates a sub-registry with the module's words and returns
// a ModuleDesc whose exports contain FnDef wrappers for those words.
var modules = map[string]func(parent *native.Registry) (native.ModuleDesc, error){
	"math-util":   BuildMathModule,
	"array-util":  BuildArrayModule,
	"time-util":   BuildTimeModule,
	"matrix-util": BuildMatrixModule,
	"bin-util":    BuildBinaryModule,
	"tui":         BuildTuiModule,
	"type-util":   BuildTypeModule,
	"fn-util":     BuildFnUtilModule,
	"vm":          BuildVMModule,
	"report":      BuildReportModule,
	"test":        BuildTestModule,
	"rand":        BuildRandModule,
	"query":       BuildQueryModule,
	"struct-util": BuildStructModule,
	"io":          BuildIOModule,
	"net":         BuildNetModule,
	"logic-util":  BuildLogicModule,
	"string-util": BuildStringModule,
	"minilang":    BuildMiniLangModule,
	"parselang":   BuildParseLangModule,
	"sift":        BuildSiftModule,
	"cli":         BuildCliModule,
	"emitlang":    BuildEmitLangModule,
	"parse":       BuildParseModule,
	"model":       BuildModelModule,
	"log":         BuildLogModule,
	"repl":        BuildReplModule,
	"debug":       BuildDebugModule,
	"scry":        BuildScryModule,
	"fmt":         BuildFmtModule,
	"vault":       BuildVaultModule,
	"vault-tui":   BuildVaultTuiModule,
}

// Resolve resolves a native module name and returns a ModuleDesc.
// The parent registry is used to generate module IDs and inherit context.
// This function is intended to be called from the import handler.
//
// Consults the policy installed on parent (if any). The modules
// scope must allow the "import" op with the resolved module ID; if
// the policy has modules.install=false, all imports are declined with
// modules_disabled.
func Resolve(name string, parent *native.Registry) (native.ModuleDesc, error) {
	moduleID := "boru:" + name
	if pol := native.HostPolicy(parent); pol != nil {
		// kind is always supplied: a where-predicate on an ABSENT arg passes
		// vacuously, so without it a profile's `where: {kind: ["file"]}`
		// admission (NUR079's file-module gate) would admit every native
		// module too. A refusal carries its policy code (PolicyRefusal), as
		// the file-module gate's does; the scope-level install:false is
		// Check's own first step.
		args := policy.Args{"module": moduleID, "kind": "native"}
		if err := pol.Check("modules", "import", args); err != nil {
			return native.ModuleDesc{}, native.PolicyRefusal(parent, "import", err)
		}
		// Per-module install:false check via the subscope.
		if !pol.Scope("modules").Scopes[moduleID].Installed() {
			return native.ModuleDesc{}, native.PolicyRefusal(parent, "import", &policy.Denied{
				Code:    policy.CodeCapabilityNotInstalled,
				Scope:   "modules",
				Op:      "import",
				Profile: pol.Name(),
				Blame:   "modules.scopes." + moduleID + ".install=false",
				Args:    args,
			})
		}
	}
	fn, ok := modules[name]
	if !ok {
		return native.ModuleDesc{}, fmt.Errorf("unknown native module: %s", moduleID)
	}
	desc, err := fn(parent)
	if err != nil {
		return desc, err
	}
	desc.Ref = moduleID
	desc.Kind = "native"
	stampExportProvenance(desc)
	return desc, nil
}

// stampExportProvenance records, for each exported function, its origin and
// one-line doc so `describe Namespace.word` can show where the word comes
// from and what it does: Module (the import id, e.g. "boru:array-util"),
// Export (the namespace, e.g. "ArrayUtil"), and Doc (from the central
// moduleDocs table, see docs.go). Done once here rather than in every
// per-module maker — the makers stay free of provenance/doc plumbing and the
// docs live as data in one place. Values a builder already populated are left
// untouched. Type exports and other non-function values are skipped.
func stampExportProvenance(desc native.ModuleDesc) {
	for ns, om := range desc.Exports {
		if om == nil {
			continue
		}
		for _, key := range om.Keys() {
			v, _ := om.Get(key)
			fn, ok := native.FnDefFromValue(v)
			if !ok {
				continue
			}
			changed := false
			if fn.Module == "" {
				fn.Module, changed = desc.Ref, true
			}
			if fn.Export == "" {
				fn.Export, changed = ns, true
			}
			if fn.Doc == "" {
				if d := moduleDocs[desc.Ref][key]; d != "" {
					fn.Doc, changed = d, true
				}
			}
			if len(fn.Examples) == 0 {
				if ex := moduleExamples[desc.Ref][key]; len(ex) > 0 {
					fn.Examples, changed = ex, true
				}
			}
			if !changed {
				continue
			}
			om.Set(key, native.NewFunction(*fn))
		}
	}
}

// InstallResolver wires the native-module resolver onto reg — the single
// production point where `import "boru:<name>"` is enabled. lang.New calls
// it, and any test harness that wants to mirror production module
// resolution must call it too. Centralising the wiring here keeps a test
// registry from silently diverging from production, the gap that hid the
// dropped sub-import Resolver: a file imported via "./lib.boru" could not
// itself import a native module because RunModuleBody never propagated the
// Resolver, and the test harnesses never installed one to begin with.
func InstallResolver(reg *native.Registry) {
	reg.Modules.Resolver = Resolve
}

// InstallMathExports builds the math module and installs its exports as defs
// in the given registry. This is a convenience for test setup — equivalent to
// what happens when boru code runs import "boru:math-util".
func InstallMathExports(r *native.Registry) error {
	desc, err := BuildMathModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallArrayExports builds the array module and installs its exports as defs
// in the given registry. This is a convenience for test setup — equivalent to
// what happens when boru code runs import "boru:array-util".
func InstallArrayExports(r *native.Registry) error {
	desc, err := BuildArrayModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallTimeExports builds the time module and installs its exports as defs.
func InstallTimeExports(r *native.Registry) error {
	desc, err := BuildTimeModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	// Transplant the module's word extensions (the temporal add / sub
	// overloads) like the real import path does — this helper is the
	// test/host shim for `import "boru:time-util"`, and bare add / sub
	// must gain the overloads under it too.
	for _, exportMap := range desc.Exports {
		for _, key := range exportMap.Keys() {
			v, _ := exportMap.Get(key)
			if ext, ok := native.IsWordExtension(v); ok {
				if err := transplantExtension(r, ext, "boru:time-util", "boru:time-util"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// InstallMatrixExports builds the matrix module and installs its exports as defs.
func InstallMatrixExports(r *native.Registry) error {
	desc, err := BuildMatrixModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	// Transplant the module's word extensions (the matrix add / sub /
	// mul overloads) like the real import path does.
	for _, exportMap := range desc.Exports {
		for _, key := range exportMap.Keys() {
			v, _ := exportMap.Get(key)
			if ext, ok := native.IsWordExtension(v); ok {
				if err := transplantExtension(r, ext, "boru:matrix-util", "boru:matrix-util"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// InstallRandExports builds the rand module and installs its exports as defs.
func InstallRandExports(r *native.Registry) error {
	desc, err := BuildRandModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallTestExports builds the test module and installs its exports as defs.
func InstallTestExports(r *native.Registry) error {
	desc, err := BuildTestModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallStructExports builds the struct module and installs its exports as
// defs — the convenience equivalent of running `import "boru:struct-util"` in test
// setup, so `StructUtil.merge` etc. resolve without wiring the full resolver.
func InstallStructExports(r *native.Registry) error {
	desc, err := BuildStructModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallIOExports builds the io module and installs its exports as defs —
// the convenience equivalent of running `import "boru:io"` in test setup, so
// `IO.read` etc. resolve without wiring the full resolver.
func InstallIOExports(r *native.Registry) error {
	desc, err := BuildIOModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallNetExports builds the net module and installs its exports as defs —
// the convenience equivalent of running `import "boru:net"` in test setup, so
// `Net.fetch` etc. resolve without wiring the full resolver.
func InstallNetExports(r *native.Registry) error {
	desc, err := BuildNetModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	// Transplant the module's word extensions (the Fetch accessor
	// overloads) like the real import path does. Without this the shim
	// and `import "boru:net"` would DISAGREE about whether a Response
	// answers `.status` — the same reason InstallTimeExports transplants.
	for _, exportMap := range desc.Exports {
		for _, key := range exportMap.Keys() {
			v, _ := exportMap.Get(key)
			if ext, ok := native.IsWordExtension(v); ok {
				if err := transplantExtension(r, ext, "boru:net", "boru:net"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// InstallLogExports builds the log module and installs its exports as defs —
// the convenience equivalent of running `import "boru:log"` in test setup, so
// `Log.info` etc. resolve without wiring the full resolver.
func InstallLogExports(r *native.Registry) error {
	desc, err := BuildLogModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// InstallDebugExports builds the debug module and installs its exports as
// defs — the convenience equivalent of running `import "boru:debug"` in test
// setup, so `Debug.tap` etc. resolve without wiring the full resolver.
func InstallDebugExports(r *native.Registry) error {
	desc, err := BuildDebugModule(r)
	if err != nil {
		return err
	}
	for name, exportMap := range desc.Exports {
		r.Defs.Push(name, native.NewMap(exportMap))
	}
	return nil
}

// Names returns the list of available native module names.
func Names() []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	return names
}
