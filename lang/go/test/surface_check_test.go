package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"
)

// checkSrc runs src through lang.Boru.Check and returns the result +
// error. Shared by the S2 surface-checker tests below.
func checkSrc(t *testing.T, src string) (lang.CheckResult, error) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	seedBoru(a)
	return a.Check(src)
}

// diagCodes flattens a CheckResult's diagnostic codes for assertions.
func diagCodes(res lang.CheckResult) []string {
	out := make([]string, 0, len(res.Diagnostics))
	for _, d := range res.Diagnostics {
		out = append(out, d.Code)
	}
	return out
}

const surfaceCheckPreamble = `
def Shape surface {area: (fnsig [[Self] [Float]])}
def Circle class {r:1.0}
def area fn [[c:Circle] [Float] [3.14]]
Circle exposes Shape
`

// TestCheckSurfaceShapeTyping pins S2's core (design/legacy/SURFACES.10.ignore):
// a required operation called on a SURFACE-typed carrier types via
// the contract's shape — clean, no no_signature degrade — and the
// result type is the shape's declared return, not Any.
func TestCheckSurfaceShapeTyping(t *testing.T) {
	res, err := checkSrc(t, surfaceCheckPreamble+`
def x:Shape (make Circle {r:1.0})
area x
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, d := range res.Diagnostics {
		if d.Code == "no_signature" || d.Code == "type_error" {
			t.Errorf("surface-typed call degraded: %s: %s", d.Code, d.Detail)
		}
	}
	if len(res.Stack) != 1 || !strings.Contains(res.Stack[0], "Float") {
		t.Errorf("residual = %v, want the shape's Float return", res.Stack)
	}
}

// TestCheckExposesStaticallyVerified pins that `exposes` runs its
// completeness check in check mode: a missing operation and a
// wrong-return overload both fail `boru check`, not just the runtime.
func TestCheckExposesStaticallyVerified(t *testing.T) {
	_, err := checkSrc(t, `
def Shape surface {area: (fnsig [[Self] [Float]])}
def Circle class {r:1.0}
Circle exposes Shape
`)
	if err == nil || !strings.Contains(err.Error(), "surface_unsatisfied") {
		t.Errorf("missing op: want surface_unsatisfied at check time, got %v", err)
	}

	_, err = checkSrc(t, `
def Shape surface {area: (fnsig [[Self] [Float]])}
def Circle class {r:1.0}
def area fn [[c:Circle] [String] ["big"]]
Circle exposes Shape
`)
	if err == nil || !strings.Contains(err.Error(), "surface_unsatisfied") {
		t.Errorf("wrong return: want surface_unsatisfied at check time, got %v", err)
	}
}

// TestCheckSurfaceShapeNegatives pins what the S2 path must NOT
// swallow: a word the surface does not require still degrades through
// the assume-sig path with its diagnostic, and a typed binding whose
// value type does not expose the surface is a type error.
func TestCheckSurfaceShapeNegatives(t *testing.T) {
	// `size` is not in Shape's contract — the carrier gives the S2
	// path no licence; the normal no_signature diagnostic must
	// survive for genuinely unmatched calls.
	res, err := checkSrc(t, surfaceCheckPreamble+`
def x:Shape (make Circle {r:1.0})
x mul 2
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, c := range diagCodes(res) {
		if c == "no_signature" {
			found = true
		}
	}
	if !found {
		t.Errorf("non-required op on a surface carrier should still degrade loudly; diags = %v", diagCodes(res))
	}

	// A non-exposing value type must not statically unify with the
	// surface-typed binding.
	res, err = checkSrc(t, `
def Shape surface {area: (fnsig [[Self] [Float]])}
def Square class {side:1.0}
def x:Shape (make Square {side:1.0})
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found = false
	for _, d := range res.Diagnostics {
		if d.Code == "type_error" && strings.Contains(d.Detail, "Shape") {
			found = true
		}
	}
	if !found {
		t.Errorf("non-exposer bound to a Shape-typed def should be a check type_error; diags = %v", diagCodes(res))
	}
}
