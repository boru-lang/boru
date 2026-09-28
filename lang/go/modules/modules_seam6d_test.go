package modules

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-6 agent-D coverage for the modules package infrastructure: the
// registry-construction error arm of every Build*Module constructor and
// Install*Exports helper (driven through the newDefaultRegistry /
// newDefaultRegistryWithPolicy seams, design/TEST-SEAMS.10.md), the
// transplant-failure arms, the per-module install=false policy compile failure,
// stampExportProvenance's skip arms, registerCollisionInstall's check-mode
// re-analysis arm, and assorted small uncovered guards (gex compile seam,
// matrix-size, matrixFromRows row guard, mapStrList atoms, Sunday weekday).

// errSeam6D is the sentinel injected through the construction seams.
var errSeam6D = errors.New("seam6d: registry construction declined")

// seam6dFailRegistryAfter swaps the newDefaultRegistry seam for a stub that
// succeeds `succeed` times and then fails with errSeam6D, and the
// newDefaultRegistryWithPolicy seam for one that always fails. Both are
// restored via t.Cleanup.
func seam6dFailRegistryAfter(t *testing.T, succeed int) {
	t.Helper()
	orig := newDefaultRegistry
	origPol := newDefaultRegistryWithPolicy
	calls := 0
	newDefaultRegistry = func(providers ...func(*native.Registry)) (*native.Registry, error) {
		calls++
		if calls > succeed {
			return nil, errSeam6D
		}
		return orig(providers...)
	}
	newDefaultRegistryWithPolicy = func(p policy.Policy, providers ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errSeam6D
	}
	t.Cleanup(func() {
		newDefaultRegistry = orig
		newDefaultRegistryWithPolicy = origPol
	})
}

// TestSeam6DBuilderRegistryInitError drives the sub-registry construction
// error arm of every native module builder: when the default registry
// cannot be built, each Build*Module must decline loudly (never install a
// half-built module).
func TestSeam6DBuilderRegistryInitError(t *testing.T) {
	parent, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	parent.SetParseFunc(parser.Parse) // test/vm require a parser before the seam
	seam6dFailRegistryAfter(t, 0)
	for name, build := range modules {
		if _, berr := build(parent); berr == nil {
			t.Errorf("Build for %q with a failing registry seam: expected error, got nil", name)
		} else if !strings.Contains(berr.Error(), "seam6d") {
			t.Errorf("Build for %q: error %v does not carry the injected cause", name, berr)
		}
	}
	// The vm sub-engine construction arm (newDefaultRegistryWithPolicy).
	if _, serr := newSubEngineRegistry(parent, nil); serr == nil ||
		!strings.Contains(serr.Error(), "vm: init sub-engine") {
		t.Errorf("newSubEngineRegistry with failing seam: got %v, want 'vm: init sub-engine' error", err)
	}
	// The rand generator-replay registry arm.
	if _, rerr := BuildSeededRandRegistry(42); rerr == nil {
		t.Error("BuildSeededRandRegistry with failing seam: expected error, got nil")
	}
	// buildRandExportsForState's own arm.
	if _, xerr := buildRandExportsForState(newRandState(1)); xerr == nil {
		t.Error("buildRandExportsForState with failing seam: expected error, got nil")
	}
	// Resolve's builder-error propagation arm (the same failure surfaced
	// through the import resolver).
	if _, rerr := Resolve("minilang", parent); rerr == nil ||
		!strings.Contains(rerr.Error(), "seam6d") {
		t.Errorf("Resolve with failing registry seam: got %v, want the injected cause", rerr)
	}
}

// TestSeam6DInstallExportsRegistryInitError drives the Build-failure
// propagation arm of every Install*Exports test-setup helper.
func TestSeam6DInstallExportsRegistryInitError(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	seam6dFailRegistryAfter(t, 0)
	installers := map[string]func(*native.Registry) error{
		"math":   InstallMathExports,
		"array":  InstallArrayExports,
		"time":   InstallTimeExports,
		"matrix": InstallMatrixExports,
		"rand":   InstallRandExports,
		"test":   InstallTestExports,
		"struct": InstallStructExports,
		"io":     InstallIOExports,
		"net":    InstallNetExports,
		"log":    InstallLogExports,
		"debug":  InstallDebugExports,
		"query":  InstallQueryExports,
	}
	for name, install := range installers {
		if ierr := install(r); ierr == nil {
			t.Errorf("Install%sExports with failing registry seam: expected error, got nil", name)
		}
	}
}

// TestSeam6DRandWithSeedRegistryInitError drives BuildRandModule's SECOND
// construction site (the Rand.with-seed sub-registry): the default exports
// build succeeds, then the with-seed registry construction fails.
func TestSeam6DRandWithSeedRegistryInitError(t *testing.T) {
	parent, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	seam6dFailRegistryAfter(t, 1)
	if _, berr := BuildRandModule(parent); berr == nil {
		t.Error("BuildRandModule with the with-seed registry failing: expected error, got nil")
	}
}

// TestSeam6DInstallExportsHappyPaths pins the never-before-exercised
// Install*Exports happy bodies: the namespace def lands in the registry.
func TestSeam6DInstallExportsHappyPaths(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse) // the test module requires a parser
	for ns, install := range map[string]func(*native.Registry) error{
		"Test": InstallTestExports,
		"IO":   InstallIOExports,
		"Net":  InstallNetExports,
	} {
		if ierr := install(r); ierr != nil {
			t.Fatalf("install %s: %v", ns, ierr)
		}
		if _, ok := r.Defs.Top(ns); !ok {
			t.Errorf("install %s: namespace def not bound", ns)
		}
	}
}

// TestSeam6DTransplantFailurePropagates drives the transplant-error arms of
// InstallTimeExports / InstallMatrixExports / InstallNetExports through the transplantExtension
// seam — the real extensions always transplant cleanly onto a fresh
// registry, so only the seam can observe the propagation.
func TestSeam6DTransplantFailurePropagates(t *testing.T) {
	orig := transplantExtension
	transplantExtension = func(r *native.Registry, ext native.FnDefInfo, origin, owner string) error {
		return errSeam6D
	}
	t.Cleanup(func() { transplantExtension = orig })

	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if terr := InstallTimeExports(r); !errors.Is(terr, errSeam6D) {
		t.Errorf("InstallTimeExports transplant failure: got %v, want errSeam6D", terr)
	}
	if merr := InstallMatrixExports(r); !errors.Is(merr, errSeam6D) {
		t.Errorf("InstallMatrixExports transplant failure: got %v, want errSeam6D", merr)
	}
	// boru:net gained transplantable extensions with the Fetch accessor
	// overloads, so its install helper has the same arm.
	if nerr := InstallNetExports(r); !errors.Is(nerr, errSeam6D) {
		t.Errorf("InstallNetExports transplant failure: got %v, want errSeam6D", nerr)
	}
}

// TestSeam6DResolvePerModuleInstallFalse pins Resolve's per-module
// install=false compile failure: the modules scope is installed and import is
// allowed, but the module's own subscope is uninstalled.
func TestSeam6DResolvePerModuleInstallFalse(t *testing.T) {
	pol, err := policy.LoadInline(`{
		name: "no-math-module"
		scopes: {
			modules: { scopes: { "boru:math-util": { install: false } } }
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	// The refusal is CODED (NUR079), blaming the module's own subscope.
	if _, rerr := Resolve("math-util", r); rerr == nil ||
		!strings.Contains(rerr.Error(), "capability_not_installed") ||
		!strings.Contains(rerr.Error(), "modules.scopes.boru:math-util.install=false") {
		t.Errorf("Resolve under per-module install=false: got %v, want the coded install=false refusal", rerr)
	}
	// A sibling module stays importable — the compile failure is per-module.
	if _, okErr := Resolve("array-util", r); okErr != nil {
		t.Errorf("Resolve of an uninhibited module: %v", okErr)
	}
}

// TestSeam6DStampExportProvenanceSkips pins the two skip arms: a nil export
// map, and an export whose provenance is already fully populated.
func TestSeam6DStampExportProvenanceSkips(t *testing.T) {
	om := native.NewOrderedMap()
	pre := native.NewFunction(native.FnDefInfo{
		Name:   "w",
		Module: "boru:already",
		Export: "Already",
		Doc:    "already documented",
	})
	om.Set("w", pre)
	desc := native.ModuleDesc{
		Ref: "boru:seam6d",
		Exports: map[string]*native.OrderedMap{
			"Nil":     nil,
			"Already": om,
		},
	}
	stampExportProvenance(desc) // must not panic on the nil map
	got, _ := om.Get("w")
	fn, ok := native.FnDefFromValue(got)
	if !ok {
		t.Fatal("export is no longer a FnDef")
	}
	if fn.Module != "boru:already" || fn.Export != "Already" || fn.Doc != "already documented" {
		t.Errorf("pre-populated provenance must be left untouched, got %+v", fn)
	}
}

// (The registerCollisionInstall / surfaceRegisterCheckError seams died with
// register_common.go — the last register word became a tombstone and the
// shared idempotency machinery was deleted with the frozen namespaces.)

// TestSeam6DGexCompileSeamFailure drives gexCompile's regexp-compile error
// arm and its propagation (with hint) through miniGexHandler. The glob
// translation QuoteMetas every literal rune, so only the seam can fail.
func TestSeam6DGexCompileSeamFailure(t *testing.T) {
	orig := regexpCompile
	regexpCompile = func(expr string) (*regexp.Regexp, error) {
		return nil, errors.New("seam6d: compile declined")
	}
	t.Cleanup(func() { regexpCompile = orig })

	if _, err := gexCompile("seam6d-unique-a*"); err == nil ||
		!strings.Contains(err.Error(), "seam6d: compile declined") {
		t.Errorf("gexCompile under failing seam: got %v", err)
	}
	r, rerr := native.DefaultRegistry()
	if rerr != nil {
		t.Fatal(rerr)
	}
	_, herr := miniGexHandler([]native.Value{
		native.NewString("seam6d-unique-b*"),
		native.NewMap(native.NewOrderedMap()),
		native.NewString("subject"),
	}, nil, nil, r)
	if herr == nil || !strings.Contains(herr.Error(), "mini_parse_error") {
		t.Errorf("gex handler under failing compile: got %v, want mini_parse_error", herr)
	}
}

// TestSeam6DMatrixSizeHandler drives the (unexported) matrix-size inner
// native, which reports rows*cols — registered in the sub-registry but not
// exported (core `size` shadows it, ADR-001), so only a direct call reaches
// its handler.
func TestSeam6DMatrixSizeHandler(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := MintTensorTypes(r)
	n := findNativeWave3(t, matrixNatives(tt), "matrix-size")
	mat := tensorValue(tt.Matrix, TensorData{Shape: []int{2, 3}, Data: make([]float64, 6)})
	out, herr := w4Handler(t, n, 0, r, mat)
	if herr != nil {
		t.Fatalf("matrix-size: %v", herr)
	}
	if got, gerr := out[0].AsConcreteInteger(); gerr != nil || got != 6 {
		t.Errorf("matrix-size of 2x3 = %v (err %v), want 6", out[0], gerr)
	}
}

// TestSeam6DMatrixFromRowsRowNotList pins the per-row list guard for a
// non-first row (row 0 is a list, row 1 is a scalar).
func TestSeam6DMatrixFromRowsRowNotList(t *testing.T) {
	rows := native.NewList([]native.Value{
		native.NewList([]native.Value{native.NewInteger(1), native.NewInteger(2)}),
		native.NewInteger(3),
	})
	if _, err := matrixFromRows(rows); err == nil ||
		!strings.Contains(err.Error(), "row 1 is not a list") {
		t.Errorf("matrixFromRows with scalar row: got %v, want 'row 1 is not a list'", err)
	}
}

// TestSeam6DMapStrListAtomElements pins mapStrList's atom-element arm:
// atoms contribute their name, unrecognised elements are skipped.
func TestSeam6DMapStrListAtomElements(t *testing.T) {
	inner := native.NewList([]native.Value{
		native.NewString("s"),
		native.NewAtom("a"),
	})
	om := native.NewOrderedMap()
	om.Set("order", inner)
	m, err := native.AsMap(native.NewMap(om))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := mapStrList(m, "order")
	if !ok || len(got) != 2 || got[0] != "s" || got[1] != "a" {
		t.Errorf("mapStrList = %v (ok=%v), want [s a]", got, ok)
	}
}

// TestSeam6DWeekdaySunday pins the ISO weekday mapping for Sunday (Go's
// weekday 0 → ISO 7). 2024-03-17 is a Sunday.
func TestSeam6DWeekdaySunday(t *testing.T) {
	r := timeRegistry(t)
	res := runBoru(t, r, callTimeDot("weekday", native.NewDate(time.Date(2024, 3, 17, 0, 0, 0, 0, time.UTC))))
	got, err := res[len(res)-1].AsConcreteInteger()
	if err != nil || got != 7 {
		t.Errorf("weekday(2024-03-17) = %v (err %v), want 7", res[len(res)-1], err)
	}
}
