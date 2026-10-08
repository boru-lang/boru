package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// w8TestErr parses+runs src against r and returns the run error (nil if none).
func w8TestErr(t *testing.T, r *native.Registry, src string) error {
	t.Helper()
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, rerr := native.NewTop(r).Run(vals)
	return rerr
}

// TestW8ResolveTestExportTypeBody drives resolveTestExport's TopTypeBody
// FnDef/Function arms (test.go:185/187-188/190): an export naming a TYPE whose
// body is a registry-less FnDef / Function value.
func TestW8ResolveTestExportTypeBody(t *testing.T) {
	modReg := mcovReg(t)

	// FnDef-typed body → NewFnDef (187/188).
	defA := modReg.Types.MintType("WTypeFn", native.TIdeal)
	modReg.Defs.PushType("WTypeFn", defA, native.NewFunction(native.FnDefInfo{Name: "wtf"}))
	outA := resolveTestExport(modReg, native.NewWord("WTypeFn"))
	if _, ok := outA.Data.(native.FnDefInfo); !ok {
		t.Errorf("resolveTestExport(type→FnDef) = %v, want an FnDef value", outA)
	}

	// Function-typed body → NewFunction (190).
	defB := modReg.Types.MintType("WTypeFunc", native.TIdeal)
	modReg.Defs.PushType("WTypeFunc", defB, native.NewFunction(native.FnDefInfo{Name: "wtf2"}))
	outB := resolveTestExport(modReg, native.NewWord("WTypeFunc"))
	if outB.Parent == nil || !outB.Parent.Equal(native.TFunction) {
		t.Errorf("resolveTestExport(type→Function) parent = %v, want Function", outB.Parent)
	}
}

// TestW8ResolveTestExportValueFnDef drives resolveTestExport's Defs.Top FnDef
// arm (test.go:197/198): an export naming a value bound to a registry-less
// FnDef value.
func TestW8ResolveTestExportValueFnDef(t *testing.T) {
	modReg := mcovReg(t)
	modReg.Defs.Push("wvfd", native.NewFunction(native.FnDefInfo{Name: "vf"}))
	out := resolveTestExport(modReg, native.NewWord("wvfd"))
	if _, ok := out.Data.(native.FnDefInfo); !ok {
		t.Errorf("resolveTestExport(value FnDef) = %v, want an FnDef value", out)
	}
}

// TestW8RunCheckPropBadBodies drives runCheckProp's gen/property
// RequireConcreteList compile failures (test.go:717/721).
func TestW8RunCheckPropBadBodies(t *testing.T) {
	r := testRegistry(t)
	goodList := native.NewList([]native.Value{native.NewInteger(0)})
	litL := native.NewTypeLiteral(native.TList)
	one := native.NewInteger(1)

	// 717: non-concrete gen body.
	if _, err := runCheckProp(r, []native.Value{native.NewString("n"), litL, goodList, one, one, one}); err == nil {
		t.Error("runCheckProp: non-concrete gen should error")
	}
	// 721: non-concrete property body.
	if _, err := runCheckProp(r, []native.Value{native.NewString("n"), goodList, litL, one, one, one}); err == nil {
		t.Error("runCheckProp: non-concrete property should error")
	}
}

// TestW8TestBadBodies drives the interpreter-path RequireConcreteList compile failures
// of Test.describe (251), Test.test (303) and Assert.throws (447): a
// type-literal List passes the sig but is declined as a body.
func TestW8TestBadBodies(t *testing.T) {
	r := testRegistry(t)
	if err := w8TestErr(t, r, `Test.describe "g" List`); err == nil {
		t.Error("Test.describe with a non-concrete body should error")
	}
	if err := w8TestErr(t, r, `Test.test "n" List`); err == nil {
		t.Error("Test.test with a non-concrete body should error")
	}
	if err := w8TestErr(t, r, `Assert.throws List`); err == nil {
		t.Error("Assert.throws with a non-concrete body should error")
	}
}

// TestW8ReportFailNoDetail drives report()'s FAIL-without-detail arm
// (test.go:1197): a spec case that fails the deep-equality check records
// error=None and no failing-input, so its report line carries no detail.
func TestW8ReportFailNoDetail(t *testing.T) {
	r := testRegistry(t)
	runTestBoru(t, r, `
		def double fn [[n:Integer] [Integer] [n 2 mul]]
		def bad-spec {
		  name: "bad"
		  subject: double/q
		  cases: [ {name: "wrong" in: [3] out: 7} ]
		  subs: []
		}
		bad-spec Test.run-spec
	`)
	out := runTestBoru(t, r, `Test.report`)
	s, _ := native.AsString(out[0])
	if !strings.Contains(s, "FAIL: wrong") {
		t.Errorf("report should carry a detail-less FAIL line for the failing case:\n%s", s)
	}
}

// TestW8CheckPropShrinks drives runCheckProp's shrink machinery
// (shrinkFailingProgram / shrinkFailingInput and their eval arms) via a
// threshold property that fails for large draws and passes for small ones,
// with shrinking enabled.
func TestW8CheckPropShrinks(t *testing.T) {
	r := testRegistry(t)
	// property: value < 50; gen draws [0,100); shrinking searches smaller.
	runTestBoru(t, r, `Test.check-prop "thresh" [r.int 0 100] [50 lt] 40 7 200`)
	// A constant-false property forces the value-level fallback path too.
	runTestBoru(t, r, `Test.check-prop "always" [r.int 0 100] [false] 5 3 200`)
	// A CONSTANT gen body (no rand dependence) has a self-contained
	// StackForm, so the gen-program reducer can re-evaluate its shrink
	// candidates: candidates below the threshold pass the property (the
	// evalFn Pass arm), those at/above it stay failing.
	runTestBoru(t, r, `Test.check-prop "const" [73] [50 lt] 3 1 200`)
}

// TestW8BuildTestModuleNoInitFunc drives BuildTestModule's native.Register
// fallback (test.go:85): a parent registry with no module InitFunc seeds the
// module sub-registry directly.
func TestW8BuildTestModuleNoInitFunc(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	r.Modules.InitFunc = nil // force the direct-Register fallback
	if _, err := BuildTestModule(r); err != nil {
		t.Fatalf("BuildTestModule with no InitFunc: %v", err)
	}
}
