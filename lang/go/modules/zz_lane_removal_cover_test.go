package modules

import (
	"errors"
	"fmt"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/stackform"
	parser "github.com/boru-lang/boru/parser/go"
)

// Coverage the compiler lane's suites carried for this package
// (design/COMPILER-LANE-REMOVAL.0.md §4): the property shrinker's seam-driven
// error arms, restored over the interpreter-only shrinker signature, and
// Debug.stack asked with no run in progress.

// w9ParseTokens parses src into engine tokens for the shrink-helper bodies.
func w9ParseTokens(t *testing.T, src string) []native.Value {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return toks
}

// TestW9ShrinkFailingProgramArms drives shrinkFailingProgram's early error
// arms: the initial rand-instance failure, the empty-form guard and the
// per-candidate eval-registry failure.
func TestW9ShrinkFailingProgramArms(t *testing.T) {
	r := testRegistry(t)
	genBody := w9ParseTokens(t, "73")
	propBody := w9ParseTokens(t, "50 lt")

	// The initial BuildSeededRandInstance fails: ok=false.
	origInst := w9BuildSeededRandInstance
	w9BuildSeededRandInstance = func(int64) (*native.OrderedMap, error) {
		return nil, errors.New("w9 inst boom")
	}
	if _, _, _, ok := shrinkFailingProgram(r, genBody, native.NewList(propBody), 1, 5); ok {
		t.Error("rand-instance failure should yield ok=false")
	}
	w9BuildSeededRandInstance = origInst

	// An empty gen body compiles to an empty StackForm: ok=false.
	if _, _, _, ok := shrinkFailingProgram(r, nil, native.NewList(propBody), 1, 5); ok {
		t.Error("empty gen body should yield ok=false")
	}

	// The per-candidate eval registry fails: every candidate is Invalid, no
	// improvement, ok=false (but the eval-registry arm ran).
	origReg := w9BuildSeededRandRegistry
	t.Cleanup(func() { w9BuildSeededRandRegistry = origReg })
	w9BuildSeededRandRegistry = func(int64) (*native.Registry, error) {
		return nil, errors.New("w9 reg boom")
	}
	if _, _, _, ok := shrinkFailingProgram(r, genBody, native.NewList(propBody), 1, 5); ok {
		t.Error("eval-registry failure should yield ok=false")
	}
}

// TestW9ShrinkMaterialiseErrors drives shrinkFailingProgram's materialise
// error arms: the final eval-registry build and the final StackForm eval.
// Both are reached only after a SUCCESSFUL reduce, so a counting pass
// locates the materialise call and a second pass fails just it.
func TestW9ShrinkMaterialiseErrors(t *testing.T) {
	r := testRegistry(t)
	gen := w9ParseTokens(t, "73")
	prop := w9ParseTokens(t, "50 lt")
	const seed, maxSteps = int64(1), int64(200)

	// Fail the final (materialise) BuildSeededRandRegistry call.
	origReg := w9BuildSeededRandRegistry
	regCalls := 0
	w9BuildSeededRandRegistry = func(s int64) (*native.Registry, error) { regCalls++; return origReg(s) }
	if _, _, _, ok := shrinkFailingProgram(r, gen, native.NewList(prop), seed, maxSteps); !ok {
		w9BuildSeededRandRegistry = origReg
		t.Fatal("setup: expected a successful reduce that reaches materialise")
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
	if _, _, _, ok := shrinkFailingProgram(r, gen, native.NewList(prop), seed, maxSteps); ok {
		t.Error("materialise registry failure should yield ok=false")
	}
	w9BuildSeededRandRegistry = origReg

	// Fail the final (materialise) StackForm eval call.
	origEval := w9StackformEval
	t.Cleanup(func() { w9StackformEval = origEval })
	evalCalls := 0
	w9StackformEval = func(reg *native.Registry, f *stackform.StackForm) ([]native.Value, error) {
		evalCalls++
		return origEval(reg, f)
	}
	if _, _, _, ok := shrinkFailingProgram(r, gen, native.NewList(prop), seed, maxSteps); !ok {
		w9StackformEval = origEval
		t.Fatal("setup: expected a successful reduce that reaches materialise")
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
	if _, _, _, ok := shrinkFailingProgram(r, gen, native.NewList(prop), seed, maxSteps); ok {
		t.Error("materialise eval failure should yield ok=false")
	}
}

// TestW9ShrinkFailingInputEvalError drives shrinkFailingInput's
// per-candidate StackForm-eval failure arm via the w9StackformEval seam.
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
	shrunk, _, _ := shrinkFailingInput(r, native.NewInteger(100), native.NewList(propBody), 5)
	if i, _ := shrunk.AsConcreteInteger(); i != 100 {
		t.Errorf("eval-failing shrink should leave the input at 100, got %d", i)
	}
	// Positive twin: with the real eval the input shrinks below 100.
	w9StackformEval = orig
	shrunk, _, _ = shrinkFailingInput(r, native.NewInteger(100), native.NewList(propBody), 50)
	if i, _ := shrunk.AsConcreteInteger(); i >= 100 {
		t.Errorf("a working eval should shrink the failing input, got %d", i)
	}
}

// Debug.stack asked with no run in progress — no engine publishes a stack
// view — answers an empty list; inside a run it answers the live stack.
func TestDebugStackWithNoRunIsEmpty(t *testing.T) {
	r, _ := debugRegistry(t)
	out, err := w4Handler(t, seam6dDebugNative(t, "debug-stack"), 0, r)
	if err != nil || len(out) != 1 || out[0].String() != "[]" {
		t.Fatalf("Debug.stack with no run: %v / %v, want [[]]", out, err)
	}
	if got := fmt.Sprint(runDebug(t, r, `1 2 Debug.stack`)); got != "[1 2 [1 2]]" {
		t.Fatalf("Debug.stack inside a run = %s, want [1 2 [1 2]]", got)
	}
}
