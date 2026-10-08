package modules

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/stackform"
	parser "github.com/boru-lang/boru/parser/go"
)

// w9ParseTokens parses src into engine tokens for the shrink-helper bodies.
func w9ParseTokens(t *testing.T, src string) []native.Value {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return toks
}

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

// TestW9ShrinkFailingProgramArms drives shrinkFailingProgram's early error
// arms: the initial rand-instance failure (908), the empty-form guard (914)
// and the per-candidate eval-registry failure (929).
func TestW9ShrinkFailingProgramArms(t *testing.T) {
	r := testRegistry(t)
	genBody := w9ParseTokens(t, "73")
	propBody := w9ParseTokens(t, "50 lt")

	// 908: initial BuildSeededRandInstance fails → ok=false.
	origInst := w9BuildSeededRandInstance
	w9BuildSeededRandInstance = func(int64) (*native.OrderedMap, error) {
		return nil, errors.New("w9 inst boom")
	}
	if _, _, _, ok := shrinkFailingProgram(r, genBody, nil, native.NewList(propBody), 1, 5); ok {
		t.Error("908: rand-instance failure should yield ok=false")
	}
	w9BuildSeededRandInstance = origInst

	// 914: an empty gen body compiles to an empty StackForm → ok=false.
	if _, _, _, ok := shrinkFailingProgram(r, nil, nil, native.NewList(propBody), 1, 5); ok {
		t.Error("914: empty gen body should yield ok=false")
	}

	// 929: the per-candidate eval registry fails → every candidate Invalid,
	// no improvement, ok=false (but the eval-registry arm ran).
	origReg := w9BuildSeededRandRegistry
	t.Cleanup(func() { w9BuildSeededRandRegistry = origReg })
	w9BuildSeededRandRegistry = func(int64) (*native.Registry, error) {
		return nil, errors.New("w9 reg boom")
	}
	if _, _, _, ok := shrinkFailingProgram(r, genBody, nil, native.NewList(propBody), 1, 5); ok {
		t.Error("929: eval-registry failure should yield ok=false")
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

// TestW9ShrinkMaterialiseErrors drives shrinkFailingProgram's materialise-phase
// error arms: the final eval-registry build (test.go:973) and the final
// StackForm eval (977). Both are reached only after a *successful* reduce, so a
// counting pass locates the materialise call and a second pass fails just it.
func TestW9ShrinkMaterialiseErrors(t *testing.T) {
	r := testRegistry(t)
	gen := w9ParseTokens(t, "73")
	prop := w9ParseTokens(t, "50 lt")
	const seed, maxSteps = int64(1), int64(200)

	// 973: fail the final (materialise) BuildSeededRandRegistry call.
	origReg := w9BuildSeededRandRegistry
	regCalls := 0
	w9BuildSeededRandRegistry = func(s int64) (*native.Registry, error) { regCalls++; return origReg(s) }
	if _, _, _, ok := shrinkFailingProgram(r, gen, nil, native.NewList(prop), seed, maxSteps); !ok {
		w9BuildSeededRandRegistry = origReg
		t.Fatal("973 setup: expected a successful reduce that reaches materialise")
	}
	total := regCalls
	regCalls = 0
	w9BuildSeededRandRegistry = func(s int64) (*native.Registry, error) {
		regCalls++
		if regCalls == total {
			return nil, errors.New("w9 materialise reg boom")
		}
		return origReg(s)
	}
	if _, _, _, ok := shrinkFailingProgram(r, gen, nil, native.NewList(prop), seed, maxSteps); ok {
		t.Error("973: materialise registry failure should yield ok=false")
	}
	w9BuildSeededRandRegistry = origReg

	// 977: fail the final (materialise) StackForm eval call.
	origEval := w9StackformEval
	t.Cleanup(func() { w9StackformEval = origEval })
	evalCalls := 0
	w9StackformEval = func(reg *native.Registry, f *stackform.StackForm) ([]native.Value, error) {
		evalCalls++
		return origEval(reg, f)
	}
	if _, _, _, ok := shrinkFailingProgram(r, gen, nil, native.NewList(prop), seed, maxSteps); !ok {
		w9StackformEval = origEval
		t.Fatal("977 setup: expected a successful reduce that reaches materialise")
	}
	totalE := evalCalls
	evalCalls = 0
	w9StackformEval = func(reg *native.Registry, f *stackform.StackForm) ([]native.Value, error) {
		evalCalls++
		if evalCalls == totalE {
			return nil, errors.New("w9 materialise eval boom")
		}
		return origEval(reg, f)
	}
	if _, _, _, ok := shrinkFailingProgram(r, gen, nil, native.NewList(prop), seed, maxSteps); ok {
		t.Error("977: materialise eval failure should yield ok=false")
	}
}

// TestW9ShrinkFailingInputEvalError drives shrinkFailingInput's per-candidate
// StackForm-eval failure arm (test.go:1027) via the w9StackformEval seam.
func TestW9ShrinkFailingInputEvalError(t *testing.T) {
	r := testRegistry(t)
	propBody := w9ParseTokens(t, "50 lt")

	orig := w9StackformEval
	t.Cleanup(func() { w9StackformEval = orig })
	w9StackformEval = func(*native.Registry, *stackform.StackForm) ([]native.Value, error) {
		return nil, errors.New("w9 eval boom")
	}
	// A large integer generates shrink candidates; each candidate's eval
	// fails, so the reducer rejects them and the input stays unchanged.
	shrunk, _, _ := shrinkFailingInput(r, native.NewInteger(100), nil, native.NewList(propBody), 5)
	if i, _ := shrunk.AsConcreteInteger(); i != 100 {
		t.Errorf("eval-failing shrink should leave the input at 100, got %d", i)
	}
}
