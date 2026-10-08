package native

import (
	"strings"
	"testing"
)

// Seam-9 coverage (W9_nativeC) for reify.go: the StructUtil.reify
// hydration helpers (rawToValue, reifyFieldValue, targetTypeOf,
// resolveReifyTarget), driven directly with crafted decoded-JSON values
// and class/union targets.

func w9PointType(t *testing.T, r *Registry) Value {
	t.Helper()
	res, err := seam5Run(r, `def Point class {x:Integer}
Point`)
	if err != nil || len(res) == 0 {
		t.Fatalf("def Point: %v / %v", res, err)
	}
	return res[len(res)-1]
}

func TestW9RawToValueBranches(t *testing.T) {
	// map[string]any, including a $$-escaped key that unescapes (290-303).
	mv, err := rawToValue(map[string]any{"a": float64(1), "$$mongo": float64(2), "f": 1.5})
	if err != nil {
		t.Fatalf("rawToValue(map) = %v", err)
	}
	m, _ := AsMap(mv)
	if _, ok := m.Get("$mongo"); !ok {
		t.Fatalf("rawToValue must unescape $$mongo → $mongo, got keys %v", m.Keys())
	}

	// []any with an unsupported element propagates the error (308-310).
	if _, err := rawToValue([]any{int(5)}); err == nil {
		t.Fatalf("rawToValue([int]) must error on unsupported element")
	}

	// A bare unsupported Go type hits the default arm (314-315).
	if _, err := rawToValue(int(5)); err == nil ||
		!strings.Contains(err.Error(), "unsupported JSON value") {
		t.Fatalf("rawToValue(int) = %v, want unsupported error", err)
	}

	// A map with an unsupported nested value propagates the error (294-296).
	if _, err := rawToValue(map[string]any{"bad": int(5)}); err == nil {
		t.Fatalf("rawToValue(map with bad nested value) must error")
	}
}

func TestW9TargetTypeOfPredicate(t *testing.T) {
	r := seam5Reg(t)
	// A declared predicate (fnpred) is a constraint whose input type is the
	// recovery target (258.57,260.3).
	res, err := seam5Run(r, `fnpred [[n:Integer] [true]]`)
	if err != nil || len(res) == 0 {
		t.Fatalf("build predicate fn: %v / %v", res, err)
	}
	pred := res[len(res)-1]
	if ct := targetTypeOf(pred); ct == nil {
		t.Fatalf("targetTypeOf(predicate fn) = nil, want the param input type")
	}
}

func TestW9ReifyFieldValueRawError(t *testing.T) {
	r := seam5Reg(t)
	// Non-class constraint + unsupported raw → rawToValue error (221-223).
	if _, err := reifyFieldValue(int(5), NewTypeLiteral(TString), r); err == nil {
		t.Fatalf("reifyFieldValue(int) must surface rawToValue error")
	}
}

func TestW9ReifyFieldValueFallthrough(t *testing.T) {
	r := seam5Reg(t)
	// constraint carrier → targetTypeOf nil, MakeClassFieldValue rejects the
	// String, so the value passes through unchanged (240.2,240.17).
	out, err := reifyFieldValue(NewString("x"), NewCarrier(TInteger), r)
	if err != nil {
		t.Fatalf("reifyFieldValue fallthrough err = %v", err)
	}
	if s, _ := AsString(out); s != "x" {
		t.Fatalf("reifyFieldValue fallthrough = %v, want 'x'", out)
	}
}

func TestW9ReifyFieldValueNestedClassError(t *testing.T) {
	r := seam5Reg(t)
	point := w9PointType(t, r)
	// A nested class field whose input map has an unknown field errors
	// through the recursive hydration (198.18,200.5).
	if _, err := reifyFieldValue(map[string]any{"bogus": float64(1)}, point, r); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("reifyFieldValue nested-class error = %v", err)
	}
}

func TestW9TargetTypeOfCarrierNil(t *testing.T) {
	// A carrier is neither bare-type, DepScalar, predicate, nor concrete
	// non-type-body → nil (264.2,264.12).
	if ct := targetTypeOf(NewCarrier(TInteger)); ct != nil {
		t.Fatalf("targetTypeOf(carrier) = %v, want nil", ct)
	}
}

func TestW9ResolveReifyTargetUnionSkipNonClass(t *testing.T) {
	r := seam5Reg(t)
	point := w9PointType(t, r)
	// A union whose first alternative is not a class is skipped (161.25,162.13)
	// and the matching class member is returned.
	union := NewDisjunct([]Value{NewTypeLiteral(TInteger), point})
	info, err := resolveReifyTarget(union, "Point", r)
	if err != nil || info == nil {
		t.Fatalf("resolveReifyTarget(union, Point) = %v / %v", info, err)
	}
}
