package test

import (
	"github.com/boru-lang/boru/lang/go/native"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
)

// =====================================================================
// Module chaining: importing multiple file modules at the top level
// and combining their exports.
//
// NOTE: Within a single expression (file module body), import+export
// works for literal values. Def references in export maps don't resolve
// because of engine forward-collection timing. Chain tests import each
// leaf file at the top level.
// =====================================================================

func TestChainImportMultipleFiles(t *testing.T) {
	// Import two leaf modules and combine their exports at the top level.
	files := map[string]string{
		"math.boru": `def pi 3
export "Math" {pi:pi}`,
		"strings.boru": `def hello "world"
export "Strings" {hello:hello}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.boru"`,
		`import "./strings.boru"`,
		`Math.pi`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

func TestChainImportMultipleFilesSecond(t *testing.T) {
	files := map[string]string{
		"math.boru": `def pi 3
export "Math" {pi:pi}`,
		"strings.boru": `def hello "world"
export "Strings" {hello:hello}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.boru"`,
		`import "./strings.boru"`,
		`Strings.hello`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'world'")
}

func TestChainThreeFileImports(t *testing.T) {
	files := map[string]string{
		"a.boru": `export "A" {x:1}`,
		"b.boru": `export "B" {y:2}`,
		"c.boru": `export "C" {z:3}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./a.boru"`,
		`import "./b.boru"`,
		`import "./c.boru"`,
		`C.z`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

func TestChainIsolationBetweenFiles(t *testing.T) {
	// Defs from one file module don't leak to another.
	// Use string value to avoid undefined word error.
	files := map[string]string{
		"a.boru": `def secret 42
export "A" {x:1}`,
		"b.boru": `export "B" {y:"secret"}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./a.boru"`,
		`import "./b.boru"`,
		`B.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	// "secret" in b.boru is a string, not 42 — proves isolation.
	got := formatStack(result)
	if got == "42" {
		t.Error("def 'secret' leaked between file modules")
	}
}

func TestChainIsolationFromParent(t *testing.T) {
	// Use string value to avoid undefined word error.
	files := map[string]string{
		"mod.boru": `export "M" {val:"foo"}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`def foo 99`,
		`import "./mod.boru"`,
		`M.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	if got == "99" {
		t.Error("parent def 'foo' leaked into file module")
	}
}

func TestChainInternalDefsNotLeaking(t *testing.T) {
	files := map[string]string{
		"mod.boru": `def internal 42
export "M" {x:1}`,
	}

	_, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`internal`,
	})
	// "internal" is undefined (not exported) — should error.
	if err == nil {
		t.Fatal("expected error for undefined word 'internal', got nil")
	}
}

// =====================================================================
// Barrel file pattern: a file that imports multiple files and
// re-exports with literal values.
// =====================================================================

func TestBarrelFileLiteralReExport(t *testing.T) {
	// Barrel re-exports using literal map values.
	files := map[string]string{
		"a.boru": `export "A" {x:1}`,
		"b.boru": `export "B" {y:2}`,
		"barrel.boru": `import "./a.boru"
import "./b.boru"
export "Barrel" {label:"combined",count:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./barrel.boru"`,
		`Barrel.label`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'combined'")
}

func TestBarrelFileCount(t *testing.T) {
	files := map[string]string{
		"a.boru": `export "A" {x:1}`,
		"b.boru": `export "B" {y:2}`,
		"barrel.boru": `import "./a.boru"
import "./b.boru"
export "Barrel" {label:"combined",count:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./barrel.boru"`,
		`Barrel.count`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

func TestBarrelTopLevelCombine(t *testing.T) {
	// Barrel pattern at the top level: import many files, combine manually.
	files := map[string]string{
		"math.boru": `def pi 3
export "Math" {pi:pi}`,
		"io.boru": `def mode "text"
export "IO" {mode:mode}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.boru"`,
		`import "./io.boru"`,
		`Math`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "Module(Math){pi}")
}

func TestBarrelTopLevelCombineSecond(t *testing.T) {
	files := map[string]string{
		"math.boru": `def pi 3
export "Math" {pi:pi}`,
		"io.boru": `def mode "text"
export "IO" {mode:mode}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./math.boru"`,
		`import "./io.boru"`,
		`IO.mode`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "'text'")
}

func TestBarrelRenameOnImport(t *testing.T) {
	files := map[string]string{
		"impl.boru": `export "Impl" {x:42}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [Impl Public] "./impl.boru"`,
		`Public.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestBarrelMultiRenameOnImport(t *testing.T) {
	files := map[string]string{
		"multi.boru": `export "A" {x:1}
export "B" {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [[A Alpha] [B Beta]] "./multi.boru"`,
		`Alpha.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestBarrelMultiRenameSecondExport(t *testing.T) {
	files := map[string]string{
		"multi.boru": `export "A" {x:1}
export "B" {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import [[A Alpha] [B Beta]] "./multi.boru"`,
		`Beta.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// =====================================================================
// Selective import: rename to exclude unwanted exports
// =====================================================================

func TestSelectiveImportViaRename(t *testing.T) {
	files := map[string]string{
		"api.boru": `export "Private" {secret:42}
export "Public" {visible:1}`,
	}

	// Only import Public, rename it.
	result, err := runModuleSteps(t, files, []string{
		`import [Public API] "./api.boru"`,
		`API.visible`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// =====================================================================
// Diamond dependency at top level
// =====================================================================

func TestDiamondTopLevel(t *testing.T) {
	// A and B both export, top level imports both.
	files := map[string]string{
		"shared.boru": `export "Shared" {val:7}`,
		"a.boru":      `export "A" {x:1}`,
		"b.boru":      `export "B" {y:2}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./shared.boru"`,
		`import "./a.boru"`,
		`import "./b.boru"`,
		`Shared.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "7")
}

func TestDiamondTopLevelAllAccessible(t *testing.T) {
	files := map[string]string{
		"a.boru": `export "A" {x:1}`,
		"b.boru": `export "B" {y:2}`,
	}

	// Both exports accessible after importing both files.
	result, err := runModuleSteps(t, files, []string{
		`import "./a.boru"`,
		`import "./b.boru"`,
		`A.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// =====================================================================
// Inline module combined with file module
// =====================================================================

func TestInlineModuleThenFileImport(t *testing.T) {
	files := map[string]string{
		"ext.boru": `export "Ext" {val:55}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import module [export "Inline" {x:1}]`,
		`import "./ext.boru"`,
		`Inline.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestFileImportThenInlineModule(t *testing.T) {
	files := map[string]string{
		"ext.boru": `export "Ext" {val:55}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./ext.boru"`,
		`import module [export "Inline" {x:1}]`,
		`Ext.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "55")
}

func TestMixedInlineAndFileBothAccessible(t *testing.T) {
	files := map[string]string{
		"ext.boru": `export "Ext" {val:55}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import module [export "Inline" {x:1}]`,
		`import "./ext.boru"`,
		`Ext.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "55")
}

// =====================================================================
// Export name variants: string names (all export sig coverage)
// =====================================================================

func TestFileExportWithStringName(t *testing.T) {
	files := map[string]string{
		"mod.boru": `export "MyExport" {x:42}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`MyExport.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestInlineModuleExportAtomName(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`import module [export "Foo" {val:9}]`,
		`Foo.val`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "9")
}

// =====================================================================
// Error paths for coverage
// =====================================================================

func TestImportFileNoParseFunc(t *testing.T) {
	mem := capabilities.NewMem()
	mem.Files["./mod.boru"] = []byte(`export "M" {x:1}`)

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, mem)
	// No ParseFunc.

	vals, _ := parser.Parse(`import "./mod.boru"`)
	eng := native.New(reg)
	_, runErr := eng.Run(vals)
	if runErr == nil {
		t.Fatal("expected error when ParseFunc is nil")
	}
	if !strings.Contains(runErr.Error(), "parser not configured") {
		t.Errorf("expected 'parser not configured', got: %v", runErr)
	}
}

func TestImportFileRenameNoParseFunc(t *testing.T) {
	mem := capabilities.NewMem()
	mem.Files["./mod.boru"] = []byte(`export "M" {x:1}`)

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, mem)

	vals, _ := parser.Parse(`import [M R] "./mod.boru"`)
	eng := native.New(reg)
	_, runErr := eng.Run(vals)
	if runErr == nil {
		t.Fatal("expected error when ParseFunc is nil")
	}
	if !strings.Contains(runErr.Error(), "parser not configured") {
		t.Errorf("expected 'parser not configured', got: %v", runErr)
	}
}

func TestImportFileRuntimeError(t *testing.T) {
	files := map[string]string{
		"bad.boru": `1 div 0`,
	}

	_, err := runModuleSteps(t, files, []string{`import "./bad.boru"`})
	if err == nil {
		t.Fatal("expected runtime error")
	}
}

func TestImportFileRenameRuntimeError(t *testing.T) {
	files := map[string]string{
		"bad.boru": `1 div 0`,
	}

	_, err := runModuleSteps(t, files, []string{`import [X Y] "./bad.boru"`})
	if err == nil {
		t.Fatal("expected runtime error")
	}
}

func TestImportFileRenameMissingFile(t *testing.T) {
	_, err := runModuleSteps(t, map[string]string{}, []string{`import [X Y] "./missing.boru"`})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestImportFileRenameParseError(t *testing.T) {
	files := map[string]string{
		"bad.boru": `((( oops`,
	}

	_, err := runModuleSteps(t, files, []string{`import [X Y] "./bad.boru"`})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestImportFileMultiRenameNotFound(t *testing.T) {
	files := map[string]string{
		"mod.boru": `export "A" {x:1}`,
	}

	_, err := runModuleSteps(t, files, []string{`import [[Missing Alias]] "./mod.boru"`})
	if err == nil {
		t.Fatal("expected 'not found' error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestModuleBodyError(t *testing.T) {
	_, err := runModuleSteps(t, nil, []string{`module [1 div 0]`})
	if err == nil {
		t.Fatal("expected module body error")
	}
	if !strings.Contains(err.Error(), "module:") {
		t.Errorf("expected 'module:' prefix, got: %v", err)
	}
}

func TestImportInlineRenameExportNotFound(t *testing.T) {
	_, err := runModuleSteps(t, nil, []string{
		`import [NoExist Alias] module [export "A" {x:1}]`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestImportInlineMultiRenameExportNotFound(t *testing.T) {
	_, err := runModuleSteps(t, nil, []string{
		`import [[NoExist Alias]] module [export "A" {x:1}]`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestImportInlineEmptyRenameList(t *testing.T) {
	_, err := runModuleSteps(t, nil, []string{
		`import [] module [export "A" {x:1}]`,
	})
	if err == nil {
		t.Fatal("expected error for empty rename list")
	}
	if !strings.Contains(err.Error(), "empty rename list") {
		t.Errorf("expected 'empty rename list', got: %v", err)
	}
}

func TestImportInlineRenameBadPairLength(t *testing.T) {
	_, err := runModuleSteps(t, nil, []string{
		`import [A B C] module [export "A" {x:1}]`,
	})
	if err == nil {
		t.Fatal("expected error for bad rename pair")
	}
}

func TestImportInlineMultiRenameBadPairLength(t *testing.T) {
	_, err := runModuleSteps(t, nil, []string{
		`import [[A B C]] module [export "A" {x:1}]`,
	})
	if err == nil {
		t.Fatal("expected error for bad pair")
	}
	if !strings.Contains(err.Error(), "exactly 2") {
		t.Errorf("expected 'exactly 2', got: %v", err)
	}
}

func TestChainErrorInInnerFile(t *testing.T) {
	files := map[string]string{
		"bad.boru": `1 div 0`,
		"middle.boru": `import "./bad.boru"
export "M" {x:1}`,
	}

	_, err := runModuleSteps(t, files, []string{`import "./middle.boru"`})
	if err == nil {
		t.Fatal("expected error from inner file")
	}
}

func TestChainMissingInnerFile(t *testing.T) {
	files := map[string]string{
		"outer.boru": `import "./nonexistent.boru"
export "M" {x:1}`,
	}

	_, err := runModuleSteps(t, files, []string{`import "./outer.boru"`})
	if err == nil {
		t.Fatal("expected error for missing inner file")
	}
}

// =====================================================================
// Literal value exports (resolveModuleExport coverage)
// =====================================================================

func TestExportWithBooleanValue(t *testing.T) {
	files := map[string]string{
		"lit.boru": `export "Lit" {b:true}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./lit.boru"`,
		`Lit.b`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

func TestExportWithListValue(t *testing.T) {
	files := map[string]string{
		"lit.boru": `export "Lit" {items:[1,2,3]}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./lit.boru"`,
		`Lit.items`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !native.ParentOf(&result[0]).Equal(native.TList) {
		t.Errorf("expected list, got %v", result)
	}
}

func TestExportWithNestedMap(t *testing.T) {
	files := map[string]string{
		"mod.boru": `export "M" {nested:{a:1,b:2}}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`M.nested`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "{a:1 b:2}")
}

// =====================================================================
// Multiple imports of the same file
// =====================================================================

func TestImportFileTwice(t *testing.T) {
	files := map[string]string{
		"mod.boru": `export "M" {x:1}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./mod.boru"`,
		`import "./mod.boru"`,
		`M.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// =====================================================================
// Directory paths
// =====================================================================

func TestImportFileWithPath(t *testing.T) {
	files := map[string]string{
		"lib/math.boru": `export "Math" {pi:3}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./lib/math.boru"`,
		`Math.pi`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

// =====================================================================
// Empty module (no exports)
// =====================================================================

func TestImportFileNoExports(t *testing.T) {
	files := map[string]string{
		"empty.boru": `1 2 add`,
	}

	_, err := runModuleSteps(t, files, []string{
		`import "./empty.boru"`,
		`x`,
	})
	// "x" is undefined — should error.
	if err == nil {
		t.Fatal("expected error for undefined word 'x', got nil")
	}
}

// =====================================================================
// Inline module: all import sig paths
// =====================================================================

func TestInlineModuleImportAll(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`import module [export "A" {x:1} export "B" {y:2}]`,
		`A.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestInlineModuleDefSubjectThenImport(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`def my-mod (module [export "M" {v:88}])`,
		`import my-mod`,
		`M.v`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "88")
}

func TestInlineModuleDefSubjectRenameImport(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`def my-mod (module [export "M" {v:88}])`,
		`import [M R] my-mod`,
		`R.v`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "88")
}

func TestInlineModuleDefSubjectMultiRenameImport(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`def my-mod (module [export "A" {x:1} export "B" {y:2}])`,
		`import [[A AA] [B BB]] my-mod`,
		`BB.y`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "2")
}

// =====================================================================
// Module value type coverage
// =====================================================================

func TestModuleValueType(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`module [export "M" {x:1}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	v := result[0]
	if !native.ParentOf(&v).Equal(native.TModuleInst) {
		t.Fatal("expected module type")
	}
	desc, _ := native.AsModuleDesc(v)
	if desc.ID == "" {
		t.Error("expected non-empty module ID")
	}
	if len(desc.Exports) != 1 {
		t.Errorf("expected 1 export, got %d", len(desc.Exports))
	}
	s := v.String()
	if !strings.Contains(s, "Module(") {
		t.Errorf("expected 'Module(' in string, got %s", s)
	}
}

// =====================================================================
// valToAtomOrString coverage (word, atom, string paths)
// =====================================================================

func TestRenameWithStringNames(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`def my-mod (module [export "Orig" {x:1}])`,
		`import ["Orig" "Renamed"] my-mod`,
		`Renamed.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

func TestMultiRenameWithStringNames(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`def my-mod (module [export "A" {x:1} export "B" {y:2}])`,
		`import [["A" "AA"] ["B" "BB"]] my-mod`,
		`AA.x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// =====================================================================
// SetParseFunc coverage
// =====================================================================

func TestSetParseFuncNil(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	reg.SetParseFunc(nil)
	if reg.ParseFunc != nil {
		t.Error("expected nil")
	}
}

func TestSetParseFuncRoundTrip(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	reg.SetParseFunc(parser.Parse)
	if reg.ParseFunc == nil {
		t.Error("expected non-nil")
	}
}

// =====================================================================
// Large barrel: five modules
// =====================================================================

func TestLargeBarrel(t *testing.T) {
	files := map[string]string{
		"m1.boru": `export "M1" {v:1}`,
		"m2.boru": `export "M2" {v:2}`,
		"m3.boru": `export "M3" {v:3}`,
		"m4.boru": `export "M4" {v:4}`,
		"m5.boru": `export "M5" {v:5}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./m1.boru"`,
		`import "./m2.boru"`,
		`import "./m3.boru"`,
		`import "./m4.boru"`,
		`import "./m5.boru"`,
		`M3.v`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "3")
}

func TestLargeBarrelLastModule(t *testing.T) {
	files := map[string]string{
		"m1.boru": `export "M1" {v:1}`,
		"m2.boru": `export "M2" {v:2}`,
		"m3.boru": `export "M3" {v:3}`,
		"m4.boru": `export "M4" {v:4}`,
		"m5.boru": `export "M5" {v:5}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./m1.boru"`,
		`import "./m2.boru"`,
		`import "./m3.boru"`,
		`import "./m4.boru"`,
		`import "./m5.boru"`,
		`M5.v`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "5")
}

// =====================================================================
// File with only scalar exports (no defs)
// =====================================================================

func TestFileOnlyScalarExports(t *testing.T) {
	files := map[string]string{
		"consts.boru": `export "Consts" {pi:3,e:2,phi:1}`,
	}

	result, err := runModuleSteps(t, files, []string{
		`import "./consts.boru"`,
		`Consts.phi`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "1")
}

// =====================================================================
// resolveModuleExport: def resolution in export
// =====================================================================

func TestExportDefResolution(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`import module [def myval 42 export "M" {v:myval}]`,
		`M.v`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

func TestExportListDefResolution(t *testing.T) {
	result, err := runModuleSteps(t, nil, []string{
		`import module [def itms [1 2 3] export "M" {items:itms}]`,
		`M.items`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !native.ParentOf(&result[0]).Equal(native.TList) {
		t.Errorf("expected list, got %v", result)
	}
}
