package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestW9BuildTestModulePreambleErrors drives BuildTestModule's parse-preamble
// (test.go:59) and run-preamble (128) error arms via the w9ParsePreamble seam.
func TestW9BuildTestModulePreambleErrors(t *testing.T) {
	newParent := func() *native.Registry {
		r, err := native.DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		r.SetParseFunc(parser.Parse)
		return r
	}

	// 59-61: the seam reports a parse error.
	orig := w9ParsePreamble
	t.Cleanup(func() { w9ParsePreamble = orig })

	w9ParsePreamble = func(*native.Registry) ([]native.Value, error) {
		return nil, errors.New("w9 boom")
	}
	if _, err := BuildTestModule(newParent()); err == nil ||
		!strings.Contains(err.Error(), "parse preamble") {
		t.Errorf("BuildTestModule parse-error arm: got %v", err)
	}

	// 128-130: the seam returns tokens that parse-clean but run-fail (an
	// undefined word errors when the preamble body runs).
	w9ParsePreamble = func(*native.Registry) ([]native.Value, error) {
		return []native.Value{native.NewWord("w9_undefined_preamble_word")}, nil
	}
	if _, err := BuildTestModule(newParent()); err == nil ||
		!strings.Contains(err.Error(), "run preamble") {
		t.Errorf("BuildTestModule run-error arm: got %v", err)
	}
}

// TestW9RunCheckPropRandInstanceError drives runCheckProp's per-iteration
// BuildSeededRandInstance failure arm (test.go:746, 4 stmts) via the seam.
func TestW9RunCheckPropRandInstanceError(t *testing.T) {
	r := testRegistry(t)
	orig := w9BuildSeededRandInstance
	t.Cleanup(func() { w9BuildSeededRandInstance = orig })
	w9BuildSeededRandInstance = func(int64) (*native.OrderedMap, error) {
		return nil, errors.New("w9 rand-instance boom")
	}

	args := []native.Value{
		native.NewString("p"),
		native.NewList([]native.Value{native.NewInteger(1)}), // gen (never runs)
		native.NewList([]native.Value{native.NewInteger(1)}), // property
		native.NewInteger(2),                                 // runs
		native.NewInteger(1),                                 // seed
		native.NewInteger(0),                                 // maxShrinks (skip shrink)
	}
	out, err := runCheckProp(r, args)
	if err != nil {
		t.Fatalf("runCheckProp: %v", err)
	}
	m, _ := native.AsMap(out[0])
	okV, _ := m.Get("ok")
	if b, _ := okV.AsConcreteBoolean(); b {
		t.Error("expected the rand-instance failure to mark the property failed")
	}
}

// TestW9BuildTestModuleAtomExport drives BuildTestModule's Atom-form export
// collector (test.go:109-112) by seaming the preamble to an atom-named export;
// the module preamble's own `export "Test" …` only uses the String sig.
func TestW9BuildTestModuleAtomExport(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	orig := w9ParsePreamble
	t.Cleanup(func() { w9ParsePreamble = orig })
	w9ParsePreamble = func(*native.Registry) ([]native.Value, error) {
		return parser.Parse("export Foo/q {a:1}")
	}
	if _, err := BuildTestModule(r); err != nil {
		t.Fatalf("atom-form export preamble: %v", err)
	}
}
