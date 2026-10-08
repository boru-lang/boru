package native

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// Seam-6 coverage for native_module_module.go (design/TEST-SEAMS.10.md).
// Each error arm is paired with the adjacent positive path where one is
// expressible.

// seam6c2FailingResolveOps wraps a FileOps whose ResolvePath always fails,
// for the resolveBareModule working-directory error arm.
type seam6c2FailingResolveOps struct{ capabilities.FileOps }

func (seam6c2FailingResolveOps) ResolvePath(string) (string, error) {
	return "", errors.New("resolvepath declined")
}

func TestSeam6C2RunModuleBodyInitError(t *testing.T) {
	parent := seam5Reg(t)
	old := newSubRegistry
	newSubRegistry = func(_ ...func(*Registry)) (*Registry, error) {
		return nil, errors.New("subregistry boom")
	}
	t.Cleanup(func() { newSubRegistry = old })
	_, err := RunModuleBody(parent, nil)
	if err == nil || !strings.Contains(err.Error(), "module init") {
		t.Fatalf("expected module init error, got %v", err)
	}
}

func TestSeam6C2RunModuleBodyNilParentCapabilities(t *testing.T) {
	parent := seam5Reg(t)
	parent.Capabilities = nil
	_, err := RunModuleBody(parent, nil)
	if err == nil || !strings.Contains(err.Error(), "capability: nil registry") {
		t.Fatalf("expected nil-capability error, got %v", err)
	}
}

func TestSeam6C2RunModuleBodyNilChildCapabilities(t *testing.T) {
	parent := seam5Reg(t)
	if err := parent.Capabilities.Set(CapMemFileOps, capabilities.NewMem()); err != nil {
		t.Fatal(err)
	}
	old := newSubRegistry
	newSubRegistry = func(_ ...func(*Registry)) (*Registry, error) {
		sub, err := DefaultRegistry()
		if err != nil {
			return nil, err
		}
		sub.Capabilities = nil
		return sub, nil
	}
	t.Cleanup(func() { newSubRegistry = old })
	_, err := RunModuleBody(parent, nil)
	if err == nil || !strings.Contains(err.Error(), "capability: nil registry") {
		t.Fatalf("expected nil-capability error from child Set, got %v", err)
	}
}

func TestSeam6C2RunModuleBodyChildMissingFormatsAndExtensions(t *testing.T) {
	parent := seam5Reg(t)
	old := newSubRegistry
	newSubRegistry = func(_ ...func(*Registry)) (*Registry, error) {
		sub, err := DefaultRegistry()
		if err != nil {
			return nil, err
		}
		// A child with no format registry / extension map installed:
		// RunModuleBody must lazily create both before overlaying the
		// parent's entries.
		if _, err := sub.Capabilities.Delete(CapFormats); err != nil {
			return nil, err
		}
		if _, err := sub.Capabilities.Delete(CapExtensions); err != nil {
			return nil, err
		}
		return sub, nil
	}
	t.Cleanup(func() { newSubRegistry = old })
	desc, err := RunModuleBody(parent, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Src == nil {
		t.Fatal("expected module descriptor with body registry")
	}
	if HostFormats(desc.Src) == nil {
		t.Fatal("expected child formats map to be created and overlaid")
	}
	exts := HostExtensions(desc.Src)
	if exts == nil {
		t.Fatal("expected child extensions map to be created and overlaid")
	}
	if exts["json"] != "json" {
		t.Fatalf("expected default json extension mapping, got %q", exts["json"])
	}
}

func TestSeam6C2ExportTypeLiteralRejected(t *testing.T) {
	// The (Atom, Map) sig arm: a Map carrier satisfies positional
	// dispatch but is not a concrete export map.
	parent := seam5Reg(t)
	_, err := RunModuleBody(parent, []Value{NewWord("export"), NewAtom("ns"), NewCarrier(TMap)})
	if err == nil || !strings.Contains(err.Error(), "must be a concrete map") {
		t.Fatalf("expected concrete-map export error (atom arm), got %v", err)
	}

	// The (String, Map) sig arm.
	parent2 := seam5Reg(t)
	_, err = RunModuleBody(parent2, []Value{NewWord("export"), NewString("ns"), NewCarrier(TMap)})
	if err == nil || !strings.Contains(err.Error(), "must be a concrete map") {
		t.Fatalf("expected concrete-map export error (string arm), got %v", err)
	}

	// Positive pair: a concrete map exports fine.
	parent3 := seam5Reg(t)
	src := `import module [ def one 1 ; export "ns" {one: one} ]`
	if _, err := seam5Run(parent3, src); err != nil {
		t.Fatalf("concrete export must succeed, got %v", err)
	}
}

func TestSeam6C2ResolveModuleMainBadJSON(t *testing.T) {
	r := seam5Reg(t)
	mem := capabilities.NewMem()
	mem.Files["mod/.boru/boru.json"] = []byte("{not json")
	SetHostFileOps(r, mem)
	if got := resolveModuleMain(r, "mod"); got != "index.boru" {
		t.Fatalf("bad boru.json must fall back to index.boru, got %q", got)
	}
	// Positive pair: a valid main property is honoured.
	mem.Files["mod/.boru/boru.json"] = []byte(`{"main":"start.boru"}`)
	if got := resolveModuleMain(r, "mod"); got != "start.boru" {
		t.Fatalf("expected start.boru, got %q", got)
	}
}

func TestSeam6C2ResolveBareModuleResolvePathError(t *testing.T) {
	r := seam5Reg(t)
	SetHostFileOps(r, seam6c2FailingResolveOps{capabilities.NewMem()})
	r.BaseDir = ""
	_, err := resolveBareModule(r, "nope")
	if err == nil || !strings.Contains(err.Error(), "cannot resolve working directory") {
		t.Fatalf("expected working-directory error, got %v", err)
	}
	// Negative pair without the failing stub: a missing module reports
	// the search range instead.
	r2 := seam5Reg(t)
	SetHostFileOps(r2, capabilities.NewMem())
	_, err = resolveBareModule(r2, "nope")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestSeam6C2LoadModuleResourcesArms(t *testing.T) {
	r := seam5Reg(t)
	mem := capabilities.NewMem()
	SetHostFileOps(r, mem)

	// Malformed boru.json: silently no resources.
	mem.Files["mod/.boru/boru.json"] = []byte("{broken")
	desc := ModuleDesc{Exports: map[string]*OrderedMap{}}
	if err := loadModuleResources(r, "mod", &desc); err != nil {
		t.Fatalf("malformed boru.json must be ignored, got %v", err)
	}
	if _, ok := desc.Exports["resource"]; ok {
		t.Fatal("no resource export expected for malformed boru.json")
	}

	// Unsupported resource file format.
	mem.Files["mod/.boru/boru.json"] = []byte(`{"resource":{"k":"data.zzz"}}`)
	desc = ModuleDesc{Exports: map[string]*OrderedMap{}}
	err := loadModuleResources(r, "mod", &desc)
	if err == nil || !strings.Contains(err.Error(), "unsupported file format") {
		t.Fatalf("expected unsupported-format error, got %v", err)
	}

	// Read failure for a well-formed resource entry.
	mem.Files["mod/.boru/boru.json"] = []byte(`{"resource":{"k":"missing.json"}}`)
	desc = ModuleDesc{Exports: map[string]*OrderedMap{}}
	err = loadModuleResources(r, "mod", &desc)
	if err == nil || !strings.Contains(err.Error(), `resource "k"`) {
		t.Fatalf("expected resource read error, got %v", err)
	}

	// Positive pair: a resolvable resource lands in the export map.
	mem.Files["mod/.boru/boru.json"] = []byte(`{"resource":{"k":"ok.json"}}`)
	mem.Files["mod/ok.json"] = []byte(`{"a": 1}`)
	desc = ModuleDesc{Exports: map[string]*OrderedMap{}}
	if err := loadModuleResources(r, "mod", &desc); err != nil {
		t.Fatalf("resource load must succeed, got %v", err)
	}
	res, ok := desc.Exports["resource"]
	if !ok {
		t.Fatal("expected resource export")
	}
	if _, ok := res.Get("k"); !ok {
		t.Fatal("expected resource key k")
	}
}

func TestSeam6C2InstallExportsNamedSubset(t *testing.T) {
	r := seam5Reg(t)
	em := NewOrderedMap()
	em.Set("v", NewInteger(1))
	desc := ModuleDesc{ID: "m1", Exports: map[string]*OrderedMap{"pkg": em}}
	if err := installExports(r, desc, []string{"pkg", "absent"}); err != nil {
		t.Fatalf("named install must succeed, got %v", err)
	}
	if !r.Defs.Has("pkg") {
		t.Fatal("expected pkg binding to be installed")
	}
	if r.Defs.Has("absent") {
		t.Fatal("absent name must not create a binding")
	}
}

func TestSeam6C2TransplantSealedWordExtensionRejected(t *testing.T) {
	r := seam5Reg(t)
	em := NewOrderedMap()
	em.Set("def", NewFunction(NewWordExtension(OwnerProgram, "def", nil)))
	desc := ModuleDesc{ID: "m2", Ref: "evil", Exports: map[string]*OrderedMap{"pkg": em}}
	err := installExports(r, desc, nil)
	if err == nil || !strings.Contains(err.Error(), "sealed word") {
		t.Fatalf("expected sealed-word rejection, got %v", err)
	}
}

func TestSeam6C2ResolveNativeModInstallError(t *testing.T) {
	r := seam5Reg(t)
	em := NewOrderedMap()
	em.Set("def", NewFunction(NewWordExtension(OwnerProgram, "def", nil)))
	desc := ModuleDesc{ID: "m3", Ref: "boru:evil", Exports: map[string]*OrderedMap{"pkg": em}}
	r.Modules.Resolver = func(name string, _ *Registry) (ModuleDesc, error) {
		if name != "evil" {
			t.Fatalf("unexpected module name %q", name)
		}
		return desc, nil
	}
	err := resolveNativeMod(r, "boru:evil")
	if err == nil || !strings.Contains(err.Error(), "sealed word") {
		t.Fatalf("expected sealed-word rejection through resolver, got %v", err)
	}
}

func TestSeam6C2ResolveModuleExportFnDefValue(t *testing.T) {
	r := seam5Reg(t)
	out := resolveModuleExport(r, NewFunction(FnDefInfo{Name: "f0"}))
	if !out.Parent.Equal(TFunction) {
		t.Fatalf("expected FnDef result, got %v", out.Parent)
	}
	info, ok := out.Data.(FnDefInfo)
	if !ok || info.Registry != r {
		t.Fatal("expected module registry to be attached to the FnDef")
	}
}

func TestSeam6C2ResolveModuleExportAtomUnbound(t *testing.T) {
	r := seam5Reg(t)
	v := NewAtom("unbound-atom-export")
	out := resolveModuleExport(r, v)
	if !IsAtom(out) {
		t.Fatalf("unbound atom must resolve to itself, got %v", out)
	}
}

func TestSeam6C2ResolveModuleExportTypeBodyArms(t *testing.T) {
	r := seam5Reg(t)

	// Type binding whose body is an FnDef with no registry: rebound as FnDef.
	r.Defs.PushType("T1", TInteger, NewFunction(FnDefInfo{Name: "T1"}))
	out := resolveModuleExport(r, NewString("T1"))
	if !out.Parent.Equal(TFunction) {
		t.Fatalf("expected FnDef from type body, got %v", out.Parent)
	}
	if info, ok := out.Data.(FnDefInfo); !ok || info.Registry != r {
		t.Fatal("expected registry attached to type-body FnDef")
	}

	// Same, but a Function-typed body: rebound as Function.
	r.Defs.PushType("T2", TInteger, NewFunction(FnDefInfo{Name: "T2"}))
	out = resolveModuleExport(r, NewString("T2"))
	if !out.Parent.Equal(TFunction) {
		t.Fatalf("expected Function from type body, got %v", out.Parent)
	}

	// A non-fn type body is returned as-is.
	r.Defs.PushType("T3", TInteger, NewInteger(7))
	out = resolveModuleExport(r, NewString("T3"))
	if n, _ := AsInteger(out); n != 7 {
		t.Fatalf("expected type body value 7, got %v", out)
	}
}

func TestSeam6C2ResolveModuleExportDefArms(t *testing.T) {
	r := seam5Reg(t)

	r.Defs.Push("g1", NewFunction(FnDefInfo{Name: "g1"}))
	out := resolveModuleExport(r, NewString("g1"))
	if !out.Parent.Equal(TFunction) {
		t.Fatalf("expected FnDef from def stack, got %v", out.Parent)
	}
	if info, ok := out.Data.(FnDefInfo); !ok || info.Registry != r {
		t.Fatal("expected registry attached to def-stack FnDef")
	}

	r.Defs.Push("g2", NewFunction(FnDefInfo{Name: "g2"}))
	out = resolveModuleExport(r, NewString("g2"))
	if !out.Parent.Equal(TFunction) {
		t.Fatalf("expected Function from def stack, got %v", out.Parent)
	}
}
