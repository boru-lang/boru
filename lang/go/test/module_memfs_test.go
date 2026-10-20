package test

import (
	"path/filepath"
	"testing"

	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
)

// runMemFSModuleSteps sets up an in-memory filesystem with pre-populated files,
// enables __sys.fs.mem=true, and runs boru steps against it.
// This validates the full pipeline: folder + write + import on in-memory FS.
func runMemFSModuleSteps(t *testing.T, files map[string]string, steps []string) ([]native.Value, error) {
	t.Helper()

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg) // production module wiring (lang.New)
	{
		prev := reg.Modules.InitFunc
		reg.Modules.InitFunc = func(child *native.Registry) {
			if prev != nil {
				prev(child)
			}
			registerIOWords(child)
		}
	}
	registerIOWords(reg) // read/write moved to boru:io; expose bare for this suite

	// Create an in-memory FS and pre-populate it with module files.
	mem := capabilities.NewMem()
	for path, content := range files {
		// The model keys a file by its resolved, OS-native path.
		mem.Files[filepath.Clean(filepath.FromSlash(path))] = []byte(content)
	}
	if err := reg.Capabilities.Set(native.CapMemFileOps, capabilities.FileOps(mem)); err != nil {
		t.Fatalf("set capability: %v", err)
	}

	// Enable in-memory FS via __sys.fs.mem = true.
	eng := native.New(reg)
	_, err = eng.Run([]native.Value{
		native.NewWord("context"), native.NewWord("dot"), native.NewWord("__sys"),
		native.NewWord("dot"), native.NewWord("fs"),
		native.NewWord("set"), native.NewWord("mem"), native.NewBoolean(true),
	})
	if err != nil {
		t.Fatalf("enable mem fs: %v", err)
	}

	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			return nil, err
		}
		result, err = eng.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// --- Basic module import on in-memory FS ---

func TestMemFSModuleBasicImport(t *testing.T) {
	files := map[string]string{
		"config.boru": `export "Config" {version:42,name:"myapp"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./config.boru"`,
		`Config.version`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestMemFSModuleStringExport(t *testing.T) {
	files := map[string]string{
		"config.boru": `export "Config" {version:42,name:"myapp"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./config.boru"`,
		`Config.name`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'myapp'")
}

// --- Module with function export ---

func TestMemFSModuleFunctionExport(t *testing.T) {
	files := map[string]string{
		"math.boru": `def double fn [[n:Integer] [Integer] [n add n]]
export "Math" {double:double/v}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./math.boru"`,
		`5 Math.double`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

// A 0-arg function exported via /v is stored as the function (not fired
// while the export map is built) and dispatches when accessed as
// `pkg.fn`. The export map auto-evaluates, so a bare `zero` would fire
// its 0-arg signature there and freeze the export to zero's result; the
// /v ref is resolved to the fn value directly instead.
func TestMemFSModuleZeroArgFunctionExport(t *testing.T) {
	files := map[string]string{
		"z.boru": `def zero fn [[] [Integer] [42]]
export "Z" {zero:zero/v}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./z.boru"`,
		`Z.zero`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// Negative: a bare function export (no /v) is auto-evaluated and
// dispatched while the export map is built — a fn needing args has no
// matching 0-arg signature, so it errors rather than silently exporting
// a reference. Functions must be exported with /v.
func TestMemFSModuleBareFunctionExportErrors(t *testing.T) {
	files := map[string]string{
		"math.boru": `def double fn [[n:Integer] [Integer] [n add n]]
export "Math" {double:double}`,
	}
	_, err := runMemFSModuleSteps(t, files, []string{
		`import "./math.boru"`,
	})
	if err == nil {
		t.Fatal("expected error: a bare fn export dispatches at build time; use /v")
	}
}

// --- Module with directory structure (.boru/boru.json) ---

func TestMemFSModuleWithBoruJson(t *testing.T) {
	files := map[string]string{
		"mymod/index.boru":      `export "API" {x:42}`,
		"mymod/.boru/boru.json": `{"name":"mymod","main":"index.boru"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./mymod"`,
		`API.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- Module with custom main file ---

func TestMemFSModuleCustomMain(t *testing.T) {
	files := map[string]string{
		"lib/core.boru":       `export "Core" {pi:3}`,
		"lib/.boru/boru.json": `{"name":"lib","main":"core.boru"}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./lib"`,
		`Core.pi`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

// --- Two modules imported ---

func TestMemFSModuleTwoImports(t *testing.T) {
	files := map[string]string{
		"math.boru": `def add1 fn [[n:Integer] [Integer] [n add 1]]
export "Math" {add1:add1/v}`,
		"strings.boru": `def greet fn [[s:String] [String] ["hello " add s]]
export "Strings" {greet:greet/v}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./math.boru"`,
		`import "./strings.boru"`,
		`9 Math.add1`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "10")
}

// --- Module with folder + write (full pipeline) ---

func TestMemFSModuleFolderWriteImport(t *testing.T) {
	// Start with empty FS — use folder and write to create everything from Boru.
	result, err := runMemFSModuleSteps(t, nil, []string{
		// Create module directory structure using Pathon
		`folder (make Pathon ["mymod"])`,
		`folder (make Pathon ["mymod" ".boru"])`,
		// Write boru.json using Pathon
		`write (make Pathon ["mymod" ".boru" "boru.json"]) "{\"name\":\"mymod\",\"main\":\"index.boru\"}"`,
		// Write module source using Pathon
		`write (make Pathon ["mymod" "index.boru"]) "export \"API\" {answer:42}"`,
		// Import and use
		`import "./mymod"`,
		`API.answer`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// --- Verify module exports are isolated ---

func TestMemFSModuleExportIsolation(t *testing.T) {
	files := map[string]string{
		"mod.boru": `def secret 999
export "Pub" {visible:1}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`Pub.visible`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}
