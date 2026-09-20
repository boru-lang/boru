package modules

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// s7bRandHandler returns the raw Go handler for a named rand native bound
// to a seed-1 state, so tests can drive the error arms with crafted args
// (type literals / negative counts / empty inputs) that the wrapper sig
// would otherwise never let past matchSignature.
func s7bRandHandler(t *testing.T, name string) native.Handler {
	t.Helper()
	for _, nf := range randNativesForState(newRandState(1)) {
		if nf.Name == name {
			gi, ok := nf.Signatures[0].Impl.(*core.GoImpl)
			if !ok {
				t.Fatalf("%s: Impl is %T, want *core.GoImpl", name, nf.Signatures[0].Impl)
			}
			return gi.Handler
		}
	}
	t.Fatalf("rand native %q not found", name)
	return nil
}

// TestS7B_RandIntArgErrors drives rand-int's two AsConcreteInteger error
// arms: a non-concrete lo, then a non-concrete hi.
func TestS7B_RandIntArgErrors(t *testing.T) {
	r := randRegistry(t)
	h := s7bRandHandler(t, "rand-int")
	litI := native.NewTypeLiteral(native.TInteger)
	if _, err := h([]native.Value{litI, native.NewInteger(5)}, nil, nil, r); err == nil {
		t.Error("rand-int: non-concrete lo should error")
	}
	if _, err := h([]native.Value{native.NewInteger(0), litI}, nil, nil, r); err == nil {
		t.Error("rand-int: non-concrete hi should error")
	}
}

// TestS7B_RandStringArgErrors drives rand-string's charset / length /
// negative-length / empty-charset arms.
func TestS7B_RandStringArgErrors(t *testing.T) {
	r := randRegistry(t)
	h := s7bRandHandler(t, "rand-string")
	litS := native.NewTypeLiteral(native.TString)
	litI := native.NewTypeLiteral(native.TInteger)
	if _, err := h([]native.Value{litS, native.NewInteger(3)}, nil, nil, r); err == nil {
		t.Error("rand-string: non-concrete charset should error")
	}
	if _, err := h([]native.Value{native.NewString("abc"), litI}, nil, nil, r); err == nil {
		t.Error("rand-string: non-concrete length should error")
	}
	if _, err := h([]native.Value{native.NewString("abc"), native.NewInteger(-1)}, nil, nil, r); err == nil {
		t.Error("rand-string: negative length should error")
	}
	if _, err := h([]native.Value{native.NewString(""), native.NewInteger(5)}, nil, nil, r); err == nil {
		t.Error("rand-string: empty charset with length>0 should error")
	}
	// Boundary success: empty charset, zero length → "".
	out, err := h([]native.Value{native.NewString(""), native.NewInteger(0)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("rand-string empty/zero = %v, %v", out, err)
	}
	if s, _ := out[0].AsConcreteString(); s != "" {
		t.Errorf("rand-string empty/zero = %q, want empty", s)
	}
}

// TestS7B_RandOneOfErrors drives rand-one-of's non-concrete and empty-list
// arms.
func TestS7B_RandOneOfErrors(t *testing.T) {
	r := randRegistry(t)
	h := s7bRandHandler(t, "rand-one-of")
	if _, err := h([]native.Value{native.NewTypeLiteral(native.TList)}, nil, nil, r); err == nil {
		t.Error("rand-one-of: non-concrete list should error")
	}
	// A concrete but empty list (non-nil slice) reaches the n==0 arm.
	if _, err := h([]native.Value{native.NewList([]native.Value{})}, nil, nil, r); err == nil {
		t.Error("rand-one-of: empty list should error")
	}
}

// TestS7B_RandListOfInterpreterArms drives rand-list-of's interpreter path:
// bad n, negative n, non-concrete body, a body that errors, and a body that
// leaves no value.
func TestS7B_RandListOfInterpreterArms(t *testing.T) {
	r := randRegistry(t)
	h := s7bRandHandler(t, "rand-list-of")
	body := native.NewList([]native.Value{native.NewInteger(1)})
	if _, err := h([]native.Value{body, native.NewTypeLiteral(native.TInteger)}, nil, nil, r); err == nil {
		t.Error("rand-list-of: non-concrete n should error")
	}
	if _, err := h([]native.Value{body, native.NewInteger(-1)}, nil, nil, r); err == nil {
		t.Error("rand-list-of: negative n should error")
	}
	if _, err := h([]native.Value{native.NewTypeLiteral(native.TList), native.NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("rand-list-of: non-concrete body should error")
	}
	badBody := native.NewList([]native.Value{native.NewWord("s7b-undefined-word")})
	if _, err := h([]native.Value{badBody, native.NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("rand-list-of: erroring body should propagate")
	}
	emptyBody := native.NewList([]native.Value{})
	if _, err := h([]native.Value{emptyBody, native.NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("rand-list-of: body producing no value should error")
	}
	// The SAME no-value error, one lane over. The interpreter path forks on
	// IsSteplessWindow, and the empty body above is stepless — so after the
	// stepless fast path landed it stops at that fork's own guard and no
	// longer reaches the per-iteration loop's. `[1 drop]` holds a Word, so it
	// is not stepless: it runs a sub-engine per iteration and nets nothing,
	// which is the only shape that reaches the loop's guard.
	//
	// It has to be driven HERE rather than from the corpus, and that is a
	// fact about the word, not a convenience. rand-list-of declares BodyOut 1,
	// so compileClosureBody gives the closure a one-value contract and ANY
	// zero-net body fails to compile — a corpus row for this shape would
	// therefore add a compile failure, and TestCompiledCoverage pins compile failures
	// at 0. The guard stays live for user programs the compiler declines;
	// the handler test is what can reach it without moving that gate.
	dropBody := native.NewList([]native.Value{native.NewInteger(1), native.NewWord("drop")})
	if _, err := h([]native.Value{dropBody, native.NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("rand-list-of: a non-stepless body producing no value should error")
	}
}

// TestS7B_RandMapFromArms drives rand-map-from's non-concrete schema,
// non-list value, erroring body, and empty-body arms.
func TestS7B_RandMapFromArms(t *testing.T) {
	r := randRegistry(t)
	h := s7bRandHandler(t, "rand-map-from")
	if _, err := h([]native.Value{native.NewTypeLiteral(native.TMap)}, nil, nil, r); err == nil {
		t.Error("rand-map-from: non-concrete schema should error")
	}
	// Value that is not a list body.
	m1 := native.NewOrderedMap()
	m1.Set("a", native.NewInteger(5))
	if _, err := h([]native.Value{native.NewMap(m1)}, nil, nil, r); err == nil {
		t.Error("rand-map-from: non-list schema value should error")
	}
	// Body that errors.
	m2 := native.NewOrderedMap()
	m2.Set("a", native.NewList([]native.Value{native.NewWord("s7b-undefined-word")}))
	if _, err := h([]native.Value{native.NewMap(m2)}, nil, nil, r); err == nil {
		t.Error("rand-map-from: erroring body should propagate")
	}
	// Body that leaves no value.
	m3 := native.NewOrderedMap()
	m3.Set("a", native.NewList([]native.Value{}))
	if _, err := h([]native.Value{native.NewMap(m3)}, nil, nil, r); err == nil {
		t.Error("rand-map-from: body producing no value should error")
	}
}

// TestS7B_RandWithSeedArgError drives rand-with-seed's AsConcreteInteger
// arm: a DepScalar seed (`Integer gt 0`) matches the Integer sig slot yet
// is declined by AsConcreteInteger inside the handler.
func TestS7B_RandWithSeedArgError(t *testing.T) {
	r := randRegistry(t)
	values, perr := parser.Parse(`Rand.with-seed (Integer gt 0)`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	_, err := native.NewTop(r).Run(values)
	if err == nil {
		t.Fatal("Rand.with-seed (Integer gt 0) should be declined")
	}
	if !strings.Contains(err.Error(), "AsConcreteInteger") && !strings.Contains(err.Error(), "concrete Integer") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestS7B_RandWithSeedBuildError drives rand-with-seed's build-error arm:
// the module is built with the real registry, then the newDefaultRegistry
// seam is swapped to fail so the handler's buildRandExportsForState errors.
func TestS7B_RandWithSeedBuildError(t *testing.T) {
	r := randRegistry(t)
	orig := newDefaultRegistry
	t.Cleanup(func() { newDefaultRegistry = orig })
	newDefaultRegistry = func(_ ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errS7bRand
	}
	values, perr := parser.Parse(`Rand.with-seed 5`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	if _, err := native.NewTop(r).Run(values); err == nil {
		t.Error("Rand.with-seed 5 should surface the registry-build failure")
	}
}

// TestS7B_RandWithSeedReturnsFallback drives randWithSeedReturns' fallback
// arm: when buildRandExportsForState fails, the check-mode shape falls back
// to a bare Map carrier instead of a concrete method map.
func TestS7B_RandWithSeedReturnsFallback(t *testing.T) {
	orig := newDefaultRegistry
	t.Cleanup(func() { newDefaultRegistry = orig })
	newDefaultRegistry = func(_ ...func(*native.Registry)) (*native.Registry, error) {
		return nil, errS7bRand
	}
	out := randWithSeedReturns(nil, nil)
	if len(out) != 1 {
		t.Fatalf("randWithSeedReturns fallback: got %d values, want 1", len(out))
	}
	if !out[0].Parent.Equal(native.TMap) {
		t.Errorf("fallback carrier parent = %s, want Map", out[0].Parent.String())
	}
}

var errS7bRand = errS7b("rand: forced registry-build failure")

// errS7b is a tiny error type for seam-driven failure arms.
type errS7b string

func (e errS7b) Error() string { return string(e) }
