package native

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// The "module", "import", and "export" words. The "module" and
// "import" words live in miscNatives (native_misc.go); "export" is
// registered dynamically inside RunModuleBody on the per-module
// sub-registry. The big bag of helpers (loadFileModule,
// installExports, resolveBareModule, etc.) lives below.

// ModuleInheritedCaps lists host-capability slots that module
// sub-registries inherit by POINTER from their importing parent. A
// host seam (RegisterHostX) appends its key via init() so file/inline
// module bodies resolve the same host backends as the top level; the
// snapshot is taken at import time, so such seams must be registered
// BEFORE the importing program runs (embedders register at startup).
//
// The four slots below are seeded here rather than appended by a module
// builder, precisely because of that ordering rule: they belong to THIS
// package, and a program whose first `import "boru:io"` happens inside a
// module body would otherwise snapshot the list before the io module had
// added them — the body would then read an empty environment, an empty
// argument vector, a stream that is never a terminal, and its own second
// buffer over stdin, while the importing program read the truth.
//
//   - CapStdinLines — the ONE buffered reader over r.Input; a second one
//     would consume the importer's bytes.
//   - CapEnv, CapScriptArgs — a module body is part of the same program
//     and sees the same world (boru:cli renders help wrapped to COLUMNS).
//   - CapStreamProbe — likewise for "is this stream a terminal", which is
//     half of the colour decision.
//
// Each inherited value is the POLICY-WRAPPED view its SetHostX installer
// put there, so inheritance never widens what a profile allows.
var ModuleInheritedCaps = []string{CapStdinLines, CapEnv, CapScriptArgs, CapStreamProbe}

// RunModuleBody creates an isolated module engine, runs the given values,
// and returns a ModuleDesc with the collected exports.
func RunModuleBody(parent *Registry, elems []Value) (ModuleDesc, error) {
	return runModuleBodyCover(parent, elems, "", "")
}

// runModuleBodyCover is RunModuleBody with optional coverage tagging: when
// coverID is non-empty AND a coverage run is armed, the module's sub-registry
// is tagged so its module-load rows AND its later fn-body rows attribute to
// coverID, and coverSrc is registered as the denominator source. A file import
// under `Test.cover` passes the import string + file text; every other caller
// passes "" (no tagging, no cost).
func runModuleBodyCover(parent *Registry, elems []Value, coverID, coverSrc string) (ModuleDesc, error) {
	// `boru check` runs this for real (see the CheckMode note below and eng's
	// "module_body_executed_in_check" severity entry), so say so. The command
	// is documented as "type-check without running", and the body's non-effect
	// work — every def, every computation — happens here. Emitted per
	// execution and not deduped: file modules are never cached, so the count
	// IS the finding, and the residual effect classes multiply with it.
	//
	// Scoped to a PURE check pass. A compile pass also runs the body, but
	// there that execution is the program's own (the VM does not re-import),
	// so it is not a finding and its effects stay real — the same split
	// modelEffects turns on below.
	//
	// KNOWN UNDERCOUNT, deliberate: a module imported from inside a module
	// body draws no entry. The advisory lands on parent.Check, and a module
	// sub-registry owns its own CheckState by design
	// (design/module-fn-checkstate-ownership.1.md §3.2), so a nested body's
	// entry would be recorded where nothing reads it. The nested body IS
	// modelled — only its report is missing.
	inPureCheck := parent != nil && parent.Check.IsActive() && !parent.Check.Compiling
	if inPureCheck {
		ref := coverID
		if ref == "" {
			ref = "an inline module body"
		}
		parent.Check.AddDiagnostic(CheckDiagnostic{
			Code: "module_body_executed_in_check",
			Detail: "check executed " + ref + " for real: `import` runs during " +
				"check so the module's exports can be typed, and check mode is " +
				"not propagated into a module body (carrier-stripping would " +
				"destroy the concrete export names). Its filesystem writes and " +
				"its output were MODELLED — a mem-over-host overlay and a " +
				"discarded writer — so neither escaped; a network send and a " +
				"stdin read are not modelled and still do",
			Word: "import",
		})
	}
	modReg, err := newSubRegistry()
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("module init: %w", err)
	}
	// Mark this registry as MODULE scope: a module body may extend a
	// core word only with user-typed signatures (the open-words
	// module-scope safety rule — see Registry.ModuleScope and
	// design/OPEN-WORDS.0.md). Top-level programs are unrestricted.
	modReg.ModuleScope = true
	// The module's minted types (refine newtypes, classes) escape to the
	// importer through its exports, so they must draw IDs from the
	// importing tree's counter — a fresh counter would let the module's
	// Nth mint collide with the parent's Nth mint, making one module's
	// newtype teq-identical to another's. See eng TypeTable.mintID and
	// lang/spec/module-instance.tsv §7.
	modReg.Types.AdoptSeqFrom(parent.Types)
	modReg.Output = parent.Output
	modReg.ErrOutput = parent.ErrOutput
	modReg.Input = parent.Input
	// The effect ledger rides with the writers: a module body's non-writer
	// effects (file writes, net sends) must count against the SAME fence
	// the parent's compiled request armed (eng effects.go, C1). The
	// observability hooks ride along too — a module body's interpreter
	// entries and runtime bails report to the request's observers.
	modReg.Effects = parent.Effects
	modReg.InheritObserveHooks(parent)
	// Coverage (coverage.go): tag the module sub-registry BEFORE its body runs,
	// so the module-load rows (def / export lines) attribute too — not just the
	// fn-body rows exercised later. Gated on an armed coverage run (CoverageArmed
	// via the just-inherited hook), so an ordinary import tags nothing.
	if coverID != "" && parent.CoverageArmed() {
		modReg.SetCoverID(coverID)
		parent.RegisterCoverSource(coverID, coverSrc)
	}
	// A compiled execution arms runtime stamping on the top registry; module
	// bodies (and the store words their fns execute later — service `add`,
	// codec resolution) run on THIS sub-registry, so the policy must ride
	// along or module-constructed callbacks would never stamp.
	modReg.InheritRuntimeStamping(parent)
	// Inherit host-installed capabilities so the module body can read
	// files, encode/decode formats, and query the SQLite store using
	// the same backends as the parent registry.
	if ops := HostFileOps(parent); ops != nil {
		SetHostFileOps(modReg, ops)
	}
	mem, ok, err := parent.Capabilities.Get(CapMemFileOps)
	if err != nil {
		return ModuleDesc{}, err
	}
	if ok {
		if err := modReg.Capabilities.Set(CapMemFileOps, mem); err != nil {
			return ModuleDesc{}, err
		}
	}
	// Host seams that opted in (ModuleInheritedCaps) ride into the module
	// sub-registry by POINTER, so a module body resolves the same host
	// backends as the top level (e.g. the boru:tui terminal backend).
	for _, capName := range ModuleInheritedCaps {
		if v, capOK, capErr := parent.Capabilities.Get(capName); capErr == nil && capOK {
			_ = modReg.Capabilities.Set(capName, v)
		}
	}
	// Overlay the parent's host-registered formats and extension mappings
	// onto the child's own defaults so a module body's reads resolve custom
	// host formats — and host overrides of the built-ins — exactly as the
	// parent does. Without this, IO.read on a host-registered extension works
	// at top level but silently decodes as text during import. Overlaying
	// (rather than replacing the map) preserves the child's defaults when the
	// parent merely added entries.
	if pf := HostFormats(parent); pf != nil {
		cf := HostFormats(modReg)
		if cf == nil {
			cf = map[string]Format{}
			SetHostFormats(modReg, cf)
		}
		for n, f := range pf {
			cf[n] = f
		}
	}
	if pe := HostExtensions(parent); pe != nil {
		ce := HostExtensions(modReg)
		if ce == nil {
			ce = DefaultExtensions()
			SetHostExtensions(modReg, ce)
		}
		for ext, n := range pe {
			ce[ext] = n
		}
	}
	// The policy rides into the module body (NUR079). Without this the
	// sub-registry has no CapPolicy, and HostPolicy(modReg) returns nil —
	// which every gate that resolves the policy itself reads as
	// allow-everything. The result was a bypass by relocation: a gated
	// call refused at top level was permitted one file deeper, defeating
	// the shipped `sandbox` / `read-only` / `compute` profiles and an
	// explicit `--deny`.
	//
	// The capability-wrapping gates (fileops, and anything else installed
	// through a SetHostX hook) were never part of the hole: they inherit
	// the parent's ALREADY-WRAPPED backend by pointer above, so their
	// enforcement rode along. The hole was confined to the gates that call
	// HostPolicy(r) at dispatch — `modules.import` and the network words
	// among them — which is why moving an `import "boru:net"` into a file
	// module let its fetch through while a file write in the same body
	// stayed refused.
	//
	// Set AFTER the SetHostX inheritance above, deliberately: those hooks
	// auto-wrap with HostPolicy(r) when one is present, so installing the
	// policy first would wrap the parent's already-permissioned backend a
	// second time. One wrap, one decision, one refusal message.
	SetHostPolicy(modReg, HostPolicy(parent))
	modReg.ParseFunc = parent.ParseFunc
	modReg.BaseDir = parent.BaseDir
	modReg.BaseFile = parent.BaseFile
	// The TCO kill switch must follow module code: intra-module tail
	// recursion runs on THIS registry (module fns are InstallFnDef'd
	// here and dispatch via execMatch inside CallBoru's sub-engine), so
	// a host that disables elision on the parent expects module frames
	// to nest too. Counters stay per-registry.
	modReg.TCO.Disable = parent.TCO.Disable
	// The module's own source text, for error excerpts from fns that
	// run later via CallBoru on this sub-registry (file imports set it
	// to the module file; inline modules inherit the entry source).
	modReg.Source = parent.Source
	// CheckMode is deliberately NOT propagated to the module sub-
	// registry. Module bodies need concrete string literals (used as
	// export names / map keys) which carrier-stripping under CheckMode
	// destroys. A typo inside an inline module body therefore raises
	// a hard `undefined_word` error from stepWord and aborts the
	// import; the user sees a clear single-error diagnostic and can
	// fix the body before re-running. Top-level / if / do / for / fn
	// bodies all stay in CheckMode and collect every typo as usual.
	//
	// What IS propagated is the other half of what check mode does. Check
	// mode strips values to carriers AND substitutes handlers; only the
	// first breaks module bodies, so ModelEffects carries the second across
	// on its own: values stay concrete, effect BACKENDS are substituted.
	// That is what makes `boru check` honest about "type-check without
	// running" — the body still runs, and still produces the exports the
	// checker needs to type `Mod.v`, but its writes go nowhere.
	//
	// Propagated transitively (a module body's own imports inherit it) and
	// derived from the PARENT's state rather than set by the caller, so
	// there is no import path that can forget it.
	//
	// The two substitutions:
	//
	//   - filesystem — a mem-over-host overlay, installed lazily by
	//     EffectiveFileOps/hostOrModelled once ModelEffects is on. Every fs
	//     mutation in the tree already routes through EffectiveFileOps
	//     (write, folder, remove, open, lock, mmap, temp, positioned
	//     writes), so one substitution covers the whole class, and reads
	//     still resolve — a write-then-read-back body sees its own bytes.
	//   - output — io.Discard in place of the writers copied above, so a
	//     body that prints at load draws nothing during check. Discarding
	//     rather than counting is also what keeps the compiled effect fence
	//     (eng effects.go) honest in the safe direction: nothing escaped, so
	//     nothing should block a later interpreter fallback.
	//
	// Input is NOT substituted, and that is the rule not an omission: the
	// mode models WRITES. Reads stay real so no body that loads today can
	// start failing under check — an EOF stdin would make a body that reads
	// input at load report a check error for a program that runs fine.
	// Network sends stay real too, and note WHY, because a substitutable
	// transport does exist (capabilities.HTTPOps, which fetch resolves its
	// http.Client transport from) — so the reason is not "no seam". It is
	// that no synthetic response is safe to invent: a fabricated status
	// takes a wrong branch and a fabricated body fails a real parse, so
	// modelling a fetch would break bodies that work, which is the one test
	// the write classes pass and this one does not. Both residuals are
	// recorded in design/verse-report-defects-investigation.0.md §D.
	//
	// !Compiling IS THE WHOLE CORRECTNESS ARGUMENT, and it is not obvious:
	// on the compiled path a module body runs ONLY in the compile pass. The
	// compile pass runs `import` for real (RunInCheck) and bakes the
	// exports, and the VM program does not re-import — so that single
	// execution is the RUN's, not a preview of it. Gating on IsActive alone
	// therefore does not merely suppress a duplicate: it deletes a module
	// body's load-time print/write from a default run entirely (measured —
	// `import module [ print 'loading' … ]` went from one line to none).
	//
	// Compiling splits the two passes exactly. A pure check pass
	// (Check.Begin — the `check` subcommand, the CLI's quiet pre-flight,
	// lang.Boru.Check) is nobody's execution and is modelled; a compile pass
	// (Check.BeginCompilePass — RunCompiled/CompileCheck) is the execution
	// and stays real. Under the CLI's default `check-then-run` that turns
	// the §D doubling into exactly ONE real execution per importer, which is
	// what a run should have had all along.
	if inPureCheck || parent.Check.ModelsEffects() {
		modReg.Check.ModelEffects = true
		modReg.Output, modReg.ErrOutput = io.Discard, io.Discard
	}

	// Inherit the module CONFIG (InitFunc + Resolver) as a unit, then
	// run InitFunc to seed the child sub-registry with native words.
	// Using ModuleRegistry.InheritConfig rather than copying fields one
	// at a time is what keeps a future config field from being silently
	// dropped here — the Resolver omission that broke `import "boru:math-util"`
	// from file-imported modules (native imports only worked at the top
	// level) was exactly that field-by-field bug.
	modReg.Modules.InheritConfig(parent.Modules)
	if modReg.Modules.InitFunc != nil {
		modReg.Modules.InitFunc(modReg)
	}

	// Inherit parent context so module can read parent values.
	// The module's Run will push its own copy-on-write layer on top.
	if parentCtx := parent.Contexts.Top(); parentCtx != nil {
		modReg.Contexts.PushExisting(parentCtx)
	}

	exports := make(map[string]*OrderedMap)

	exportHandler := func(name string, rawMap ReadMap) {
		resolved := NewOrderedMap()
		for _, key := range rawMap.Keys() {
			val, _ := rawMap.Get(key)
			resolved.Set(key, resolveModuleExport(modReg, val))
		}
		exports[name] = resolved
	}

	// Drop the inherited top-level no-op `export` (registered in the
	// default registry so files run standalone — see §8.3) before
	// installing the real collecting handler. RegisterNativeFunc APPENDS
	// signatures to an existing word, so without this delete the no-op's
	// (Atom|String, Map) sigs would shadow the collecting sigs and module
	// exports would be silently discarded.
	modReg.Defs.Delete("export")
	modReg.RegisterNativeFunc(NativeFunc{
		Name: "export",

		Signatures: []Signature{
			{
				Args: []*Type{TAtom, TMap},
				Impl: Go(func(eargs []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if !IsConcrete(eargs[1]) {
						return nil, parent.BoruError("export_error", "export: value must be a concrete map, got type literal", "export")
					}
					_as1, _ := eargs[0].AsConcreteAtom()
					_m, _ := AsMap(eargs[1])
					exportHandler(_as1, _m)
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			},
			{
				Args: []*Type{TString, TMap},
				Impl: Go(func(eargs []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					if !IsConcrete(eargs[1]) {
						return nil, parent.BoruError("export_error", "export: value must be a concrete map, got type literal", "export")
					}
					_as2, _ := eargs[0].AsConcreteString()
					_m, _ := AsMap(eargs[1])
					exportHandler(_as2, _m)
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			},
		},
	})

	// A module body is code: unquoted words (as the parser yields them)
	// run; quoted strings and atoms are DATA and are left untouched. A
	// string is never silently promoted to a word and dispatched — a
	// quoted value whose text happens to name a word (`"def"`, `"print"`)
	// stays a string, the same contract `do` and every other evaluator
	// honour.
	input := make([]Value, len(elems))
	copy(input, elems)
	// The module's identity is drawn BEFORE the body runs so every type
	// the body mints (refine / class) is owner-stamped with this
	// module's token — the provenance the ownership-anchored signature
	// rules verify at def-merge and transplant time
	// (design/OPEN-WORDS.1.md §4).
	modID := parent.Modules.NextID()
	modReg.Types.MintOwner = "module#" + modID
	sub := New(modReg)
	// C4 attribution (plan Phase 10): the body run IS the module load — a
	// sanctioned interpreter entry reported under its named seam.
	restoreAtt := modReg.SetInterpAttribution("module-load")
	_, err = sub.Run(input)
	restoreAtt()
	if err != nil {
		return ModuleDesc{}, err
	}

	// Detached-stamp the module's fn bindings (design/RUNTIME-STAMPING.0.md
	// Phase 4a): every module-scope def holding an eligible capture-free fn
	// compiles to its own unit here, at load — the ONE pre-publication moment
	// where the def binding and the export map still share each fn's impl
	// pointer, so the in-place stamp reaches both. The module-export apply
	// seam (execFnDefSig) then runs a stamped fn on the VM from any caller.
	// StampFnValueInPlace declines silently per fn (policy off, capturing,
	// refusing body), leaving that fn interpreting exactly as before. The
	// stamps happen after the whole body ran so cross-fn deps resolve, and
	// their dep snapshots freeze against the completed module scope.
	if modReg.RuntimeStampingEnabled() {
		for _, name := range modReg.Defs.Names() {
			if v, ok := modReg.Defs.Top(name); ok {
				StampFnValueInPlace(modReg, v)
			}
		}
	}

	desc := ModuleDesc{
		ID:      modID,
		Exports: exports,
		Kind:    "inline", // overridden by loadFileModule / Resolve
		// The body registry — import transports escaped type machinery
		// (adopted type nodes, constructing ideals) from here, which is
		// what lets a FACADE module re-export another module's type
		// literal and keep it constructible (adoptEscapedTypes).
		Src: modReg,
	}
	return desc, nil
}

// isFilePath returns true if the string looks like a file path
// (starts with "/", "./", or "../").
func isFilePath(path string) bool {
	return strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "../")
}

// isDataFile returns true if the path has a data file extension
// (.json, .jsonic, .csv, .tsv) — the formats `import` loads as values.
// Parser-backed formats (ini/xml/yaml/…) are deliberately excluded: they
// are read-only and reached via `read`, not `import`.
func isDataFile(r *Registry, path string) bool {
	f := formatFromExt(r, path)
	return f == "json" || f == "jsonic" || f == "csv" || f == "tsv"
}

// resolveModuleMain checks for .boru/boru.json in the given directory and
// returns the main file specified there. If the file doesn't exist or has
// no main property, returns "index.boru".
func resolveModuleMain(r *Registry, dir string) string {
	data, err := EffectiveFileOps(r).ReadFile(filepath.Join(dir, ".boru", "boru.json"))
	if err != nil {
		return "index.boru"
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "index.boru"
	}
	if main, ok := m["main"].(string); ok && main != "" {
		return main
	}
	return "index.boru"
}

// resolveImportPath resolves a file import path. If the registry has a BaseDir
// set (i.e. we are inside a module loaded from a file), relative paths are
// resolved against that directory. Otherwise the path is returned as-is
// (FileOps.ReadFile will resolve it against the process CWD).
// If the resolved path has no file extension, checks .boru/boru.json for a main
// property, falling back to index.lang.
func resolveImportPath(r *Registry, path string) string {
	resolved := path
	if r.BaseDir != "" && !filepath.IsAbs(path) {
		resolved = filepath.Join(r.BaseDir, path)
	}
	if filepath.Ext(resolved) == "" {
		main := resolveModuleMain(r, resolved)
		resolved = filepath.Join(resolved, main)
	}
	return resolved
}

// resolveBareModule resolves a bare module name (e.g. "foo") by searching for
// .boru/foo/ starting from the importing module's directory (BaseDir) or the
// current working directory, and walking up parent directories, following the
// CommonJS node_modules resolution pattern.
// Checks .boru/boru.json for a main property, falling back to index.lang.
func resolveBareModule(r *Registry, name string) (string, error) {
	var startDir string
	if r.BaseDir != "" {
		startDir = r.BaseDir
	} else {
		var err error
		startDir, err = EffectiveFileOps(r).ResolvePath(".")
		if err != nil {
			return "", fmt.Errorf("import: cannot resolve working directory: %w", err)
		}
	}

	dir := startDir
	for {
		modDir := filepath.Join(dir, ".boru", name)
		main := resolveModuleMain(r, modDir)
		candidate := filepath.Join(modDir, main)
		if _, err := EffectiveFileOps(r).ReadFile(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("import: module %q not found (searched .boru/%s/ from %s to /)", name, name, startDir)
}

// loadDataFile reads a data file (.json, .jsonic, .csv, .tsv) and returns
// the result as a boru value on the stack. Uses doRead so CSV/TSV files
// get the same table + SQLite handling as the read word.
func loadDataFile(parent *Registry, path string) ([]Value, error) {
	format := formatFromExt(parent, path)
	if format == "" {
		return nil, parent.BoruError("import_error", fmt.Sprintf("import: unknown format for %s", path), "import")
	}
	resolved := resolveImportPath(parent, path)
	result, err := doRead(parent, resolved, "utf8", format, "lf", nil, SrcPos{})
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	return result, nil
}

// loadFileModule reads a file, parses it as boru, and runs it as a module.
// The child module's BaseDir is set to the directory of the loaded file so
// that relative imports inside it resolve correctly.
func loadFileModule(parent *Registry, path string) (ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return ModuleDesc{}, fmt.Errorf("import: parser not configured for file import")
	}

	resolved := resolveImportPath(parent, path)

	data, err := EffectiveFileOps(parent).ReadFile(resolved)
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %w", err)
	}

	parsed, err := parent.ParseFunc(string(data))
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: parse %s: %w", resolved, err)
	}

	// Temporarily set parent BaseDir / BaseFile / Source so the child
	// module inherits the loaded file's directory, path, and SOURCE
	// TEXT (RunModuleBody copies all three; the source is what lets an
	// error raised later inside the module's fns render the module
	// file's excerpt — decision DX report finding 4).
	modDir := filepath.Dir(resolved)
	savedDir, savedFile, savedSrc := parent.BaseDir, parent.BaseFile, parent.Source
	parent.BaseDir = modDir
	parent.BaseFile = resolved
	parent.Source = string(data)
	// Pass the import string + file text as the coverage id/source so a
	// `Test.cover [ import "./mod.boru" … ]` body can report `Test.coverage
	// "./mod.boru"` (the tagging happens BEFORE the body runs, inside the cover
	// variant, and is a no-op unless a coverage run is armed).
	desc, err := runModuleBodyCover(parent, parsed, path, string(data))
	parent.BaseDir, parent.BaseFile, parent.Source = savedDir, savedFile, savedSrc
	if err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %s: %w", resolved, err)
	}
	desc.Ref = path
	desc.Kind = "file"
	desc.File = resolved
	desc.Folder = modDir
	// NUR045: stamp the per-export policy identity as soon as the
	// module's addressable id is known (see StampModuleCallGates).
	StampModuleCallGates(desc.Exports, desc.Ref)

	// If the module's boru.json declares resources, load them as a
	// "resource" export so they are available as Module.resource.key.
	if err := loadModuleResources(parent, modDir, &desc); err != nil {
		return ModuleDesc{}, fmt.Errorf("import: %s: %w", resolved, err)
	}

	return desc, nil
}

// loadModuleResources checks the module's .boru/boru.json for a "resource"
// property (map of key→filename). For each entry it loads the data file
// from the module directory and adds a "resource" export to the descriptor.
func loadModuleResources(r *Registry, modDir string, desc *ModuleDesc) error {
	data, err := EffectiveFileOps(r).ReadFile(filepath.Join(modDir, ".boru", "boru.json"))
	if err != nil {
		return nil // no boru.json — nothing to do
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	resMap, ok := m["resource"].(map[string]any)
	if !ok || len(resMap) == 0 {
		return nil
	}

	resources := NewOrderedMap()
	for key, v := range resMap {
		filename, ok := v.(string)
		if !ok {
			return fmt.Errorf("resource %q: value must be a string filename", key)
		}
		format := formatFromExt(r, filename)
		if format == "" {
			return fmt.Errorf("resource %q: unsupported file format %q", key, filename)
		}
		filePath := filepath.Join(modDir, filename)
		vals, err := doRead(r, filePath, "utf8", format, "lf", nil, SrcPos{})
		if err != nil {
			return fmt.Errorf("resource %q: %w", key, err)
		}
		if len(vals) > 0 {
			resources.Set(key, vals[0])
		}
	}

	desc.Exports["resource"] = resources
	return nil
}

// installExports installs all exports from a module descriptor as defs.
// If names is nil, all exports are installed using their original names.
// Each export is bound as a plain namespace Map whose module-namespace
// facet points at the shared Module instance (NewModuleNamespace).
// Word-extension clones found in the export maps are
// additionally transplanted into r (see transplantWordExtensions) —
// the only error path.
// linkModuleDebugParent links each export-captured module sub-registry
// to the IMPORTING registry (eng SetDebugParent), so engines running
// module fn bodies resolve the importer's LIVE debug hook — the
// `boru debug` session's stepping descends into module fns, and its
// suppression/detach stay authoritative (a copied hook could fire after
// the session disarmed itself). Shared by every import form; a no-op
// for exports that captured no registry.
func linkModuleDebugParent(r *Registry, desc ModuleDesc) {
	for _, exportMap := range desc.Exports {
		if exportMap != nil {
			for _, key := range exportMap.Keys() {
				if v, ok := exportMap.Get(key); ok {
					if fn, isFn := FnDefFromValue(v); isFn && fn.Registry != nil {
						fn.Registry.SetDebugParent(r)
					}
				}
			}
		}
	}
}

func installExports(r *Registry, desc ModuleDesc, names []string) error {
	linkModuleDebugParent(r, desc)
	mod := NewModuleInstance(desc)
	if names == nil {
		// SORTED, not a map range. The bind ledger's twins are appended in
		// this order, and the arm-residency bridge pairs twins to def-site
		// events by occurrence — so an order that differs run to run makes
		// the compiled stream non-reproducible for a module exporting two or
		// more namespaces. One namespace is trivially stable, which is why no
		// ledgered row exposes it; that is a reason to fix it rather than a
		// reason it does not matter.
		for _, name := range sortedExportNames(desc) {
			InstallDef(r, name, NewModuleNamespace(name, desc.Exports[name], mod))
		}
		return transplantWordExtensions(r, desc)
	}
	for _, name := range names {
		if exportMap, ok := desc.Exports[name]; ok {
			InstallDef(r, name, NewModuleNamespace(name, exportMap, mod))
		}
	}
	return transplantWordExtensions(r, desc)
}

// sortedExportNames gives installExports a deterministic order over the
// export map. See its caller for why the order is load-bearing.
func sortedExportNames(desc ModuleDesc) []string {
	names := make([]string, 0, len(desc.Exports))
	for name := range desc.Exports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// transplantWordExtensions scans a module's exports for word-extension
// clones (`def <locked word> fn […]` in the module body, exported like
// any other fn) and merges each one's unlocked signatures into the
// importing registry's same-named word — the implicit top-level def
// the importer consented to by importing (design/OPEN-WORDS.0.md
// §2.4). One level only: the transplant lands in r and stops; only a
// re-export propagates further (the re-exporting module's clone then
// carries the signatures under ITS origin — it takes ownership).
// Provenance is the module ref, so a diamond re-import is idempotent
// and quiet, while two different modules extending the same unlocked
// tuple raise [boru/extend_conflict]. When the base word does not
// resolve in r, nothing transplants and the export stays a plain
// namespaced binding (§4.9 — M.word still works). The transplanted
// handlers remain closed over the module's sub-registry, so module-
// private helpers keep resolving.
func transplantWordExtensions(r *Registry, desc ModuleDesc) error {
	adoptEscapedTypes(r, desc)
	origin := desc.Ref
	if origin == "" {
		// An inline module has no ref; its per-import instance ID is its
		// provenance (each inline `import module […]` is a distinct module).
		origin = "inline#" + desc.ID
	}
	// The exporting module's own mint owner — the AUTHOR the transplant
	// admission verifies source-clone anchors against (host clones carry
	// ExtOwner instead). Src is the module's body/sub-registry.
	owner := ""
	if desc.Src != nil && desc.Src.Types != nil {
		owner = desc.Src.Types.MintOwner
	}
	for _, exportMap := range desc.Exports {
		for _, key := range exportMap.Keys() {
			v, _ := exportMap.Get(key)
			ext, ok := IsWordExtension(v)
			if !ok {
				continue
			}
			if err := TransplantExtension(r, ext, origin, owner); err != nil {
				return err
			}
		}
	}
	return nil
}

// adoptEscapedTypes walks a module's exports for BARE TYPE LITERALS —
// module-minted types the importer can now reference (`make
// MatrixUtil.Matrix …`, `x is TimeUtil.Timezone`) — and transports each
// one's escaped machinery into the importing registry:
//
//   - the canonical *Type node is ADOPTED into r's TypeTable (same
//     pointer, no re-mint), so the compiled OpPushType path
//     (r.Types.LookupByID) resolves it — without this every
//     force-compiled program using an exported module type aborts with
//     "unresolvable type operand";
//   - the type's constructing IDEAL, when the module's own registry
//     has one and the importer has neither a matching nor a same-named
//     ideal, is registered in r — so `make` keeps working when the
//     literal arrives through a FACADE module that re-exports it (the
//     facade's body registry acquired the ideal when IT imported the
//     defining module, so the transport chains one level per import).
//
// Both steps are idempotent per type: re-adoption of a known ID is a
// no-op and an already-satisfied ideal is left alone. desc.Src is nil
// for descriptors that predate the field (host tests) — nothing to
// transport then.
func adoptEscapedTypes(r *Registry, desc ModuleDesc) {
	if desc.Src == nil {
		return
	}
	for _, exportMap := range desc.Exports {
		for _, key := range exportMap.Keys() {
			v, _ := exportMap.Get(key)
			if !IsBareTypeNode(v) {
				continue
			}
			canonical := desc.Src.Types.LookupByID(v.ID)
			if canonical == nil {
				continue // a builtin literal — the VM's Builtin fallback covers it
			}
			r.Types.Adopt(canonical)
			lit := NewTypeLiteral(canonical)
			if r.Ideals.Match(lit) != nil {
				continue // the importer can already construct it
			}
			ideal := desc.Src.Ideals.Match(lit)
			if ideal == nil || r.Ideals.Get(ideal.Name) != nil {
				continue // nothing to transport / a same-named ideal owns the slot
			}
			r.Ideals.Register(ideal)
		}
	}
}

// installRenamedExports applies a rename list to module exports and installs them.
func installRenamedExports(r *Registry, desc ModuleDesc, renameList []Value) error {
	if len(renameList) == 0 {
		return fmt.Errorf("import: empty rename list")
	}
	linkModuleDebugParent(r, desc)
	mod := NewModuleInstance(desc)

	if renameList[0].Parent.Equal(TList) {
		// Multiple rename pairs: [[from1 to1] [from2 to2] ...]
		for _, pair := range renameList {
			pairElems, _ := AsList(pair)
			if pairElems.Len() != 2 {
				return fmt.Errorf("import: rename pair must have exactly 2 elements")
			}
			fromName := valToAtomOrString(pairElems.Get(0))
			toName := valToAtomOrString(pairElems.Get(1))
			exportMap, ok := desc.Exports[fromName]
			if !ok {
				return fmt.Errorf("import: export %q not found in module", fromName)
			}
			InstallDef(r, toName, NewModuleNamespace(fromName, exportMap, mod))
		}
	} else {
		// Single rename pair: [from to]
		if len(renameList) != 2 {
			return fmt.Errorf("import: rename list must have exactly 2 elements (from to)")
		}
		fromName := valToAtomOrString(renameList[0])
		toName := valToAtomOrString(renameList[1])
		exportMap, ok := desc.Exports[fromName]
		if !ok {
			return fmt.Errorf("import: export %q not found in module", fromName)
		}
		InstallDef(r, toName, NewModuleNamespace(fromName, exportMap, mod))
	}
	// Renaming the namespace does not opt out of the module's word
	// extensions — the extension targets the base word, not the
	// namespace name. The firewall idiom is the opt-out.
	return transplantWordExtensions(r, desc)
}

// installSingleRename renames the single export in a module to newName.
// If the module has zero or more than one export, an error is returned.
func installSingleRename(r *Registry, desc ModuleDesc, newName string) error {
	linkModuleDebugParent(r, desc)
	mod := NewModuleInstance(desc)
	if len(desc.Exports) == 0 {
		return fmt.Errorf("import: module has no exports to rename")
	}
	if len(desc.Exports) != 1 {
		return fmt.Errorf("import: rename requires module with exactly one export, got %d", len(desc.Exports))
	}
	for name, exportMap := range desc.Exports {
		InstallDef(r, newName, NewModuleNamespace(name, exportMap, mod))
	}
	return transplantWordExtensions(r, desc)
}

// resolveModuleExport resolves an export map value through the module's
// def stacks. If the value is a string, atom, or word that names a def'd word,
// the def body is returned. Otherwise the value is returned as-is.
func resolveModuleExport(modReg *Registry, v Value) Value {
	// A function value — typically produced by `name/v` in the export
	// map, which auto-evaluates to the bound fn as data — must carry the
	// module registry so it executes in module scope (resolving module-
	// private words) when called after import.
	if fnDef, ok := v.Data.(FnDefInfo); ok {
		// An exported word (the public API) IS used — record it so check mode
		// does not falsely flag every exported impl as unused_def. The `name/v`
		// reference form auto-evaluates to the bound fn HERE (as data), so the
		// name was never stepped through the normal dispatch/ResolveRef use path.
		modReg.Check.RecordUse(fnDef.Name)
		if fnDef.Registry == nil {
			fnDef.Registry = modReg
			return NewFunction(fnDef)
		}
		return v
	}
	// A bare name (word/string/atom) resolves by lookup in the module
	// registry. After auto-eval this path is reached mainly in check
	// mode and for type/value names that resolved to themselves.
	var name string
	if IsWord(v) {
		_as3, _ := AsWord(v)
		name = _as3.Name
	} else if v.Parent.ConformsTo(TString) {
		name, _ = AsString(v)
	} else if IsAtom(v) {
		name, _ = AsAtom(v)
	} else {
		return v
	}
	// A bare-name export value (`{ f: impl }`, `{ Color: Color }`) is a use of
	// that def/type — record it so unused_def does not flag the public API.
	modReg.Check.RecordUse(name)
	// Resolution order: r.Types (canonical home for user-defined
	// types post-§5.2) wins, then DefStacks (value defs and the
	// fn-def stash). Without the r.Types check, exports of named
	// types (`export "color" {Color:Color}`) would leave the value
	// side as an unresolved Word.
	if tv, ok := modReg.TopTypeBody(name); ok {
		if fnDef, ok := tv.Data.(FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
				return NewFunction(fnDef)
			}
		}
		return tv
	}
	if val, ok := modReg.Defs.Top(name); ok {
		// Tag FnDef values with the module's registry so they can
		// execute in the correct context (closure semantics).
		if fnDef, ok := val.Data.(FnDefInfo); ok {
			if fnDef.Registry == nil {
				fnDef.Registry = modReg
				return NewFunction(fnDef)
			}
		}
		return val
	}
	return v
}

// isNativeModImport returns true if the path looks like a native module
// import (starts with "boru:").
func isNativeModImport(path string) bool {
	return strings.HasPrefix(path, "boru:")
}

// resolveNativeMod resolves a native module import (e.g. "boru:math-util").
// The module name is extracted from the "boru:" prefix and resolved via the
// registry's NativeModResolver callback. The resolver returns a ModuleDesc
// whose exports are installed as defs, just like file-based modules.
// Each native module is loaded at most once per registry.
func resolveNativeMod(r *Registry, path string) error {
	name := strings.TrimPrefix(path, "boru:")
	if name == "" {
		return fmt.Errorf("import: empty native module name in %q", path)
	}
	if desc, ok := r.Modules.LoadedDesc(name); ok {
		// Already resolved once in this registry. Re-bind any namespace
		// defs that are no longer present — a fn-body / property-body
		// import installs the namespace via InstallDef, which CallBoru's
		// def-cleanup then strips, leaving the module marked loaded but
		// `pkg` unbound for the next call. Rebinding only the absent names
		// keeps repeated top-level imports from stacking shadow bindings.
		// See §11b.1 in the DX report.
		ensureExportsBound(r, desc)
		// Re-run the word-extension transplant too: an `undef add` may
		// have popped the module's extension, and a re-import is the
		// natural way to restore it. Idempotent by origin, so a plain
		// repeated import installs nothing.
		return transplantWordExtensions(r, desc)
	}
	if r.Modules.Resolver == nil {
		return fmt.Errorf("import: native module resolver not configured (cannot import %q)", path)
	}
	desc, err := r.Modules.Resolver(name, r)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	// NUR045: stamp the per-export policy identity before the exports
	// are installed, so every binding that flows out of installExports
	// (and every sig copied from one) carries the gate.
	StampModuleCallGates(desc.Exports, "boru:"+name)
	if err := installExports(r, desc, nil); err != nil {
		return err
	}
	r.Modules.MarkLoaded(name, desc)
	return nil
}

// ResolveAnyModule resolves a module reference to its descriptor WITHOUT
// installing its exports, mirroring import's native / bare / file dispatch. It
// lets tooling — notably `boru describe` — load and introspect a module that is
// not one of the resolver's compiled-in builtins ("attempt to load a module if
// unknown"). For a "boru:" reference the registry must have a module Resolver
// installed (modules.InstallResolver). Data-file references are rejected: they
// have no exports to describe.
func ResolveAnyModule(r *Registry, ref string) (ModuleDesc, error) {
	if isNativeModImport(ref) {
		name := strings.TrimPrefix(ref, "boru:")
		if name == "" {
			return ModuleDesc{}, fmt.Errorf("empty native module name in %q", ref)
		}
		if r.Modules.Resolver == nil {
			return ModuleDesc{}, fmt.Errorf("native module resolver not configured (cannot resolve %q)", ref)
		}
		return r.Modules.Resolver(name, r)
	}
	if isDataFile(r, ref) {
		return ModuleDesc{}, fmt.Errorf("%q is a data file, not a module", ref)
	}
	if !isFilePath(ref) {
		resolved, err := resolveBareModule(r, ref)
		if err != nil {
			return ModuleDesc{}, err
		}
		return loadFileModule(r, resolved)
	}
	return loadFileModule(r, ref)
}

// ensureExportsBound re-installs any module-namespace defs that are not
// currently bound. Used when re-importing an already-resolved native
// module whose namespace binding was torn down (e.g. by CallBoru's
// fn-body def-cleanup). Only absent names are installed, so a plain
// repeated top-level import stays a no-op rather than stacking bindings.
// The re-bind MUST be a real facet-carrying namespace, exactly like the
// first-load installExports — a facet-LESS Map here would make the
// re-bound namespace a different value kind (moduleNSFields fails), so
// mini/parse kind lookups and `$module`/`$name` reads would break after
// any teardown+re-import.
func ensureExportsBound(r *Registry, desc ModuleDesc) {
	mod := NewModuleInstance(desc)
	// SORTED, for installExports' reason: these installs note bind-ledger
	// transitions, and anything pairing against them by occurrence order
	// must not be reading from a map.
	for _, name := range sortedExportNames(desc) {
		if !r.Defs.Has(name) {
			InstallDef(r, name, NewModuleNamespace(name, desc.Exports[name], mod))
		}
	}
}

// valToAtomOrString extracts a string from a Value that is an atom, string, or word.
func valToAtomOrString(v Value) string {
	if IsWord(v) {
		_as4, _ := AsWord(v)
		return _as4.Name
	}
	if IsAtom(v) {
		_as5, _ := AsAtom(v)
		return _as5
	}
	if v.Parent.ConformsTo(TString) {
		_as6, _ := AsString(v)
		return _as6
	}
	return v.String()
}
