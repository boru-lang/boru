package describe

// describe_seam6e_test.go — seam-arm tests (design/TEST-SEAMS.10.md) for
// the registry-construction and module-resolution failure arms, plus the
// crafted-descriptor arms of exportInfoFromDesc / exportWordNames /
// qualifiedExportInfo.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

func swapDefaultRegistryFail(t *testing.T) {
	t.Helper()
	orig := defaultRegistry
	defaultRegistry = func(policy.Policy, ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errors.New("registry boom")
	}
	t.Cleanup(func() { defaultRegistry = orig })
}

func swapModulesResolve(t *testing.T, fn func(string, *native.Registry) (native.ModuleDesc, error)) {
	t.Helper()
	orig := modulesResolve
	modulesResolve = fn
	t.Cleanup(func() { modulesResolve = orig })
}

func TestDescribeNameRegistryError(t *testing.T) {
	swapDefaultRegistryFail(t)
	var out bytes.Buffer
	if code := Run([]string{"add"}, &out); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "cannot load registry") {
		t.Errorf("out = %q, want registry error", out.String())
	}
}

func TestDescribeModulePathRegistryError(t *testing.T) {
	swapDefaultRegistryFail(t)
	var out bytes.Buffer
	if code := Run([]string{"boru:math-util"}, &out); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "cannot load registry") {
		t.Errorf("out = %q, want registry error", out.String())
	}
}

func TestDescribeNameModuleResolveError(t *testing.T) {
	swapModulesResolve(t, func(string, *native.Registry) (native.ModuleDesc, error) {
		return native.ModuleDesc{}, errors.New("resolve boom")
	})
	var out bytes.Buffer
	if code := Run([]string{"math-util"}, &out); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "cannot load module") {
		t.Errorf("out = %q, want cannot load module", out.String())
	}
}

func TestQualifiedExportInfoResolveError(t *testing.T) {
	swapModulesResolve(t, func(string, *native.Registry) (native.ModuleDesc, error) {
		return native.ModuleDesc{}, errors.New("resolve boom")
	})
	reg, err := newRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if info := qualifiedExportInfo(reg, "ArrayUtil.indices"); info != nil {
		t.Fatalf("info = %v, want nil when every module fails to resolve", info)
	}
}

func TestQualifiedExportInfoNonFnExport(t *testing.T) {
	om := native.NewOrderedMap()
	om.Set("notafn", native.NewString("just data"))
	swapModulesResolve(t, func(string, *native.Registry) (native.ModuleDesc, error) {
		return native.ModuleDesc{Exports: map[string]*native.OrderedMap{"NS": om}}, nil
	})
	reg, err := newRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if info := qualifiedExportInfo(reg, "NS.notafn"); info != nil {
		t.Fatalf("info = %v, want nil for a non-function export", info)
	}
}

func TestExportInfoFromDescSkipsNilAndNonFn(t *testing.T) {
	om := native.NewOrderedMap()
	om.Set("word", native.NewString("just data"))
	desc := native.ModuleDesc{Exports: map[string]*native.OrderedMap{
		"A": nil, // nil namespace map: skipped
		"B": om,  // has the word, but it is not a function: skipped
	}}
	if info := exportInfoFromDesc(desc, "word"); info != nil {
		t.Fatalf("info = %v, want nil", info)
	}
}

func TestExportWordNamesSkipsNil(t *testing.T) {
	om := native.NewOrderedMap()
	om.Set("w", native.NewString("x"))
	desc := native.ModuleDesc{Exports: map[string]*native.OrderedMap{
		"A": nil,
		"B": om,
	}}
	got := exportWordNames(desc)
	if len(got) != 1 || got[0] != "B.w" {
		t.Fatalf("exportWordNames = %v, want [B.w]", got)
	}
}
