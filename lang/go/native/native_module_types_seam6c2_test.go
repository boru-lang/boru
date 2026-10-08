package native

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Seam-6 coverage for native_module_types.go, typeinit.go, and setup.go
// (design/TEST-SEAMS.10.md).

func TestSeam6C2TypeInitErrRecordedAndSurfaced(t *testing.T) {
	saved := SwapTypeInitErrs(nil)
	t.Cleanup(func() { SwapTypeInitErrs(saved) })

	// The nil arm: recording no error records nothing.
	recordTypeInitErr(nil)
	if TypeInitError() != nil {
		t.Fatal("nil record must not create an init error")
	}

	// A recorded registration failure (the shape registerTemporalType-
	// style init code records; the Module/KeyVal registrations moved to
	// the kernel and record there — ADR-012 stage 2) surfaces from
	// DefaultRegistry at construction.
	recordTypeInitErr(errors.New("test: type already registered"))
	if _, err := DefaultRegistry(); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("DefaultRegistry must surface the init error, got %v", err)
	}
}

func TestSeam6C2DefaultRegistryEngRegistryError(t *testing.T) {
	old := newEngRegistry
	newEngRegistry = func() (*Registry, error) { return nil, errors.New("kernel boom") }
	t.Cleanup(func() { newEngRegistry = old })
	if _, err := DefaultRegistry(); err == nil || !strings.Contains(err.Error(), "kernel boom") {
		t.Fatalf("expected kernel init error, got %v", err)
	}
}

func TestSeam6C2DefaultRegistrySQLiteError(t *testing.T) {
	old := newSQLiteStoreFn
	newSQLiteStoreFn = func() (*SQLiteStore, error) { return nil, errors.New("store boom") }
	t.Cleanup(func() { newSQLiteStoreFn = old })
	if _, err := DefaultRegistry(); err == nil || !strings.Contains(err.Error(), "store boom") {
		t.Fatalf("expected sqlite init error, got %v", err)
	}
}

func TestSeam6C2DefaultRegistryProviderAndErr(t *testing.T) {
	// A provider runs; a bad registration inside it surfaces via r.Err().
	called := false
	_, err := DefaultRegistry(func(r *Registry) {
		called = true
		r.RegisterNativeFunc(NativeFunc{Name: "BadName"})
	})
	if !called {
		t.Fatal("provider must be invoked")
	}
	if err == nil {
		t.Fatal("expected registration error to surface from DefaultRegistry")
	}
	// Positive pair: a well-behaved provider yields a working registry.
	r, err := DefaultRegistry(func(r *Registry) {})
	if err != nil || r == nil {
		t.Fatalf("expected clean registry, got %v", err)
	}
}

func TestSeam6C2NewModuleNamespaceNilFields(t *testing.T) {
	v := NewModuleNamespace("E", nil, Value{})
	if ModuleNSOf(v) == nil {
		t.Fatal("namespace must carry the module-namespace facet")
	}
	if fields := moduleNSFields(v); fields == nil || len(fields.Keys()) != 0 {
		t.Fatal("nil fields must be defaulted to an empty map")
	}
	if !v.Parent.Equal(TMap) {
		t.Fatalf("namespace must be a plain Map, got %v", v.Parent)
	}
}

func TestSeam6C2ModuleNSFieldsNonNamespace(t *testing.T) {
	// A facet-less value answers nil fields.
	if moduleNSFields(NewInteger(1)) != nil {
		t.Fatal("integer must not answer namespace fields")
	}
	// A facet-carrying value whose payload is not a map answers nil too
	// (defensive: the facet is only ever stamped on maps by
	// NewModuleNamespace, but the reader must not assume).
	odd := core.WithModuleNS(NewInteger(1), "E", Value{})
	if moduleNSFields(odd) != nil {
		t.Fatal("non-map payload must answer nil fields")
	}
}

func TestSeam6C2AsModuleDescNonModule(t *testing.T) {
	if _, ok := AsModuleDesc(NewInteger(1)); ok {
		t.Fatal("integer must not unwrap as ModuleDesc")
	}
	// An ExtensionPayload whose Body is not a boxed *ModuleDesc (a host
	// payload, or a legacy by-value ModuleDesc) must not unwrap either.
	if _, ok := AsModuleDesc(Value{Parent: TModuleInst, Data: ExtensionPayload{Body: 42}}); ok {
		t.Fatal("non-descriptor ExtensionPayload must not unwrap")
	}
	if _, ok := AsModuleDesc(Value{Parent: TModuleInst, Data: ExtensionPayload{Body: ModuleDesc{ID: "m"}}}); ok {
		t.Fatal("a by-value ModuleDesc body must not unwrap — the payload boxes a pointer")
	}
	// A nil boxed pointer is defensively declined, never dereferenced.
	if _, ok := AsModuleDesc(Value{Parent: TModuleInst, Data: ExtensionPayload{Body: (*ModuleDesc)(nil)}}); ok {
		t.Fatal("a nil *ModuleDesc body must not unwrap")
	}
	// Positive pair.
	if d, ok := AsModuleDesc(NewModuleInstance(ModuleDesc{ID: "m"})); !ok || d.ID != "m" {
		t.Fatal("module instance must unwrap")
	}
	// NUR031: each NewModuleInstance call boxes a FRESH pointer, so two
	// instances built from one ModuleDesc are eq-distinct, while copies
	// of one instance share the identity.
	src := ModuleDesc{ID: "m"}
	inst := NewModuleInstance(src)
	if !core.ExactEqual(inst, inst) || !core.DeepEqual(inst, inst) {
		t.Fatal("a module instance must be eq/deq to itself")
	}
	if core.ExactEqual(inst, NewModuleInstance(src)) {
		t.Fatal("two separately-built instances must be eq-distinct (per-import-instance identity)")
	}
}

func TestSeam6C2ModuleExportGetArms(t *testing.T) {
	// A facet-less value answers nothing.
	if _, ok := moduleExportGet(NewInteger(1), "$name"); ok {
		t.Fatal("a facet-less value must answer no keys")
	}
	// A facet-carrying non-map answers synthetics but no fields.
	odd := core.WithModuleNS(NewInteger(1), "E", Value{})
	if _, ok := moduleExportGet(odd, "missing"); ok {
		t.Fatal("nil fields must answer no keys")
	}
	// The synthetic keys resolve from the facet.
	if name, ok := moduleExportGet(odd, "$name"); !ok {
		t.Fatal("$name must resolve")
	} else if s, _ := AsString(name); s != "E" {
		t.Fatalf("expected export name E, got %v", name)
	}
	mod := NewModuleInstance(ModuleDesc{Ref: "m"})
	fields := NewOrderedMap()
	fields.Set("a", NewInteger(1))
	ns := NewModuleNamespace("E", fields, mod)
	if got, ok := moduleExportGet(ns, "$module"); !ok || !got.Parent.Equal(TModuleInst) {
		t.Fatalf("$module must resolve to the Module descriptor, got %v", got)
	}
	if got, ok := moduleExportGet(ns, "a"); !ok {
		t.Fatal("field a must resolve")
	} else if n, _ := AsInteger(got); n != 1 {
		t.Fatalf("expected 1, got %v", got)
	}
	if _, ok := moduleExportGet(ns, "missing"); ok {
		t.Fatal("missing field must not resolve")
	}
}

func TestSeam6C2ModuleNamespaceBoundAndExport(t *testing.T) {
	r := seam5Reg(t)
	// Unbound name.
	if moduleNamespaceBound(r, "Nope") {
		t.Fatal("unbound name must not report a namespace")
	}
	if _, ok := moduleNamespaceExport(r, "Nope", "x"); ok {
		t.Fatal("unbound name must answer no exports")
	}
	// Bound, but not a namespace.
	r.Defs.Push("NotNS", NewInteger(5))
	if moduleNamespaceBound(r, "NotNS") {
		t.Fatal("a non-namespace binding must not report a namespace")
	}
	if _, ok := moduleNamespaceExport(r, "NotNS", "x"); ok {
		t.Fatal("a non-namespace binding must answer no exports")
	}
	// Bound namespace: present and missing exports.
	fields := NewOrderedMap()
	fields.Set("x", NewInteger(7))
	r.Defs.Push("NS", NewModuleNamespace("NS", fields, Value{}))
	if !moduleNamespaceBound(r, "NS") {
		t.Fatal("a bound namespace must report true")
	}
	if v, ok := moduleNamespaceExport(r, "NS", "x"); !ok {
		t.Fatal("export x must resolve")
	} else if n, _ := AsInteger(v); n != 7 {
		t.Fatalf("expected 7, got %v", v)
	}
	if _, ok := moduleNamespaceExport(r, "NS", "y"); ok {
		t.Fatal("a missing export must not resolve")
	}
}

func TestSeam6C2ModuleGetArms(t *testing.T) {
	// Not a module value at all.
	if _, ok := moduleGet(NewInteger(1), "kind"); ok {
		t.Fatal("integer must not answer module fields")
	}
	inst := NewModuleInstance(ModuleDesc{})
	// Kind defaults to inline.
	kind, ok := moduleGet(inst, "kind")
	if !ok {
		t.Fatal("kind must resolve")
	}
	if s, _ := AsString(kind); s != "inline" {
		t.Fatalf("empty kind must default to inline, got %v", kind)
	}
	// Unknown key.
	if _, ok := moduleGet(inst, "nope"); ok {
		t.Fatal("unknown key must not resolve")
	}
}

func TestSeam6C2ModuleInstGetReturnsArms(t *testing.T) {
	// Wrong arity: dynamic Any.
	out := moduleInstGetReturns([]Value{NewString("kind")}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TAny) {
		t.Fatalf("expected dynamic Any for wrong arity, got %v", out)
	}
	// Unknown key on a concrete pair: dynamic Any.
	inst := NewModuleInstance(ModuleDesc{Kind: "file"})
	out = moduleInstGetReturns([]Value{NewString("nope"), inst}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TAny) {
		t.Fatalf("expected dynamic Any for unknown key, got %v", out)
	}
	// Positive pairs: closed fields keep their types.
	out = moduleInstGetReturns([]Value{NewString("kind"), inst}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TString) {
		t.Fatalf("expected String carrier for kind, got %v", out)
	}
	out = moduleInstGetReturns([]Value{NewString("exports"), inst}, nil)
	if len(out) != 1 || !out[0].Parent.ConformsTo(TList) {
		t.Fatalf("expected List carrier for exports, got %v", out)
	}
}

func TestSeam6C2ModuleNSSyntheticAndMiss(t *testing.T) {
	r := seam5Reg(t)
	fields := NewOrderedMap()
	fields.Set("a", NewInteger(1))
	ns := NewModuleNamespace("E", fields, NewModuleInstance(ModuleDesc{Ref: "m"}))
	// Synthetics answer through the shared map handlers' hook…
	if v, ok := moduleNSGetSynthetic(ns, "$name"); !ok {
		t.Fatal("$name must resolve via the synthetic hook")
	} else if s, _ := AsString(v); s != "E" {
		t.Fatalf("expected E, got %v", v)
	}
	// …while plain keys and non-namespaces fall through to the map read.
	if _, ok := moduleNSGetSynthetic(ns, "a"); ok {
		t.Fatal("a plain export key must fall through to the map read")
	}
	if _, ok := moduleNSGetSynthetic(NewInteger(1), "$name"); ok {
		t.Fatal("a non-namespace must not answer synthetics")
	}
	// The strict-miss error carries the module wording + the export keys.
	err := moduleNSGetrMiss(r, ns, "zz", core.SrcPos{})
	if err == nil || !strings.Contains(err.Error(), `export "zz" not found in module`) {
		t.Fatalf("expected the module-flavored miss, got %v", err)
	}
}

func TestSeam6C2ModuleInstHandlers(t *testing.T) {
	r := seam5Reg(t)
	lit := NewTypeLiteral(TModuleInst)
	if _, err := getModuleInstHandler([]Value{NewString("kind"), lit}, nil, nil, r); err == nil {
		t.Fatal("get on a Module type literal must error")
	}
	if _, err := getrModuleInstHandler([]Value{NewString("kind"), lit}, nil, nil, r); err == nil {
		t.Fatal("getr on a Module type literal must error")
	}

	inst := NewModuleInstance(ModuleDesc{Kind: "file", Ref: "m"})
	// get: unknown key answers None.
	out, err := getModuleInstHandler([]Value{NewString("nope"), inst}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsNoneShape(out[0]) {
		t.Fatalf("expected None for unknown key, got %v / %v", out, err)
	}
	// getr: known key answers the field.
	out, err = getrModuleInstHandler([]Value{NewString("kind"), inst}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("expected kind value, got %v / %v", out, err)
	}
	if s, _ := AsString(out[0]); s != "file" {
		t.Fatalf("expected file, got %v", out[0])
	}
	// getr: unknown key raises not_found.
	_, err = getrModuleInstHandler([]Value{NewString("nope"), inst}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "not found in Module") {
		t.Fatalf("expected not_found, got %v", err)
	}
}
