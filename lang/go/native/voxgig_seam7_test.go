package native

import (
	"fmt"
	"strings"
	"testing"
)

// Seam-7 coverage for the voxgig-struct convert-back arms and the
// filter/flatten/items/selector/validate/transform handlers
// (design/TEST-SEAMS.10.md). The convert-back failure arms are driven by
// swapping the shared structConvert seam to a function that always errors.

// b3FailConvert swaps structConvert to always error, restoring on cleanup.
func b3FailConvert(t *testing.T) {
	t.Helper()
	saved := structConvert
	t.Cleanup(func() { structConvert = saved })
	structConvert = func(any) (Value, error) { return Value{}, fmt.Errorf("boom-convert") }
}

func TestB3FlattenConvertBackFails(t *testing.T) {
	r := seam5Reg(t)
	b3FailConvert(t)
	// flattenDefaultHandler: convert-back error arm.
	if _, err := flattenDefaultHandler([]Value{NewList([]Value{NewInteger(1)})}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "flatten:") {
		t.Fatalf("flattenDefault: expected convert-back error, got %v", err)
	}
	// flattenDepthHandler: convert-back error arm (valid depth first).
	if _, err := flattenDepthHandler([]Value{NewInteger(1), NewList([]Value{NewInteger(1)})}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "flatten:") {
		t.Fatalf("flattenDepth: expected convert-back error, got %v", err)
	}
}

func TestB3FlattenDepthBadDepth(t *testing.T) {
	r := seam5Reg(t)
	// AsConcreteInteger error arm: a bare Integer type literal has nil data.
	if _, err := flattenDepthHandler([]Value{NewTypeLiteral(TInteger), NewList([]Value{NewInteger(1)})}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "flatten: depth") {
		t.Fatalf("flattenDepth: expected depth error, got %v", err)
	}
}

func TestB3FlattenDepthNegativeFull(t *testing.T) {
	r := seam5Reg(t)
	// Positive pair: -1 fully flattens without error.
	out, err := flattenDepthHandler([]Value{NewInteger(-1), NewList([]Value{NewList([]Value{NewInteger(1)})})}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("flattenDepth(-1): unexpected %v / %v", out, err)
	}
}

func TestB3ItemsConvertBackFails(t *testing.T) {
	r := seam5Reg(t)
	b3FailConvert(t)
	// itemsHandler: structConvert(pair[1]) error arm -> valVal="".
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	out, err := itemsHandler([]Value{NewMap(om)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("items: expected success with empty-string fallback, got %v / %v", out, err)
	}
}

func TestB3SelectorConvertBackFails(t *testing.T) {
	r := seam5Reg(t)
	b3FailConvert(t)
	// selectorHandler: convert-back error arm. query=[], children=[].
	if _, err := selectorHandler([]Value{NewList(nil), NewList(nil)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "selector:") {
		t.Fatalf("selector: expected convert-back error, got %v", err)
	}
}

func TestB3ValidateConvertBackFails(t *testing.T) {
	r := seam5Reg(t)
	b3FailConvert(t)
	// validateHandler: convert-back error arm. spec=String matches any string.
	if _, err := validateHandler([]Value{NewString("`string`"), NewString("x")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "validate:") {
		t.Fatalf("validate: expected convert-back error, got %v", err)
	}
}

func TestB3TransformConvertBackFails(t *testing.T) {
	r := seam5Reg(t)
	b3FailConvert(t)
	// transformHandler: convert-back error arm. spec=`a`, data={a:1}.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	if _, err := transformHandler([]Value{NewString("`a`"), NewMap(om)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "transform:") {
		t.Fatalf("transform: expected convert-back error, got %v", err)
	}
}

// TestB3AnyToValueNestedError covers the []any inner-conversion error arm of
// anyToValue directly: an unsupported Go type nested in a slice.
func TestB3AnyToValueNestedError(t *testing.T) {
	if _, err := anyToValue([]any{int32(5)}); err == nil {
		t.Fatal("expected nested unsupported-type error from []any")
	}
	// Positive pair.
	if _, err := anyToValue([]any{int64(5)}); err != nil {
		t.Fatalf("unexpected error on valid []any: %v", err)
	}
}

// TestB3FilterCallbackNonBoolean: filter's Function form reads the predicate's
// verdict strictly — a non-Boolean result is the word's loud filter_error,
// never a silent drop (the quotation form's rule, voxgig DX report T9.2).
func TestB3FilterCallbackNonBoolean(t *testing.T) {
	r := seam5Reg(t)
	_, err := seam5Run(r, `filter ([p:Any] => [p]) [1 2 3]`)
	if err == nil || !strings.Contains(err.Error(), "element 0: predicate must produce a Boolean, got Integer") {
		t.Fatalf("filter non-boolean callback: expected the loud filter_error, got %v", err)
	}
	// Positive pair: a Boolean verdict keeps the elements it admits.
	out, err := seam5Run(r, `filter ([p:Integer] => [p gt 1]) [1 2 3]`)
	if err != nil || len(out) != 1 || out[0].String() != "[2 3]" {
		t.Fatalf("filter Boolean callback: got %v / %v, want [2 3]", out, err)
	}
}

func TestB3FilterBodyNonConcrete(t *testing.T) {
	r := seam5Reg(t)
	// filterBodyHandler: non-concrete body list -> error.
	if _, err := filterBodyHandler([]Value{NewTypeLiteral(TList), NewList([]Value{NewInteger(1)})}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete body") {
		t.Fatalf("filterBody: expected concrete-body error, got %v", err)
	}
}

// TestB3ValueToAnyDepScalarFloat covers valueToAny's Float fallback arm: a
// dependent-type constraint value (Float gt 1.0) conforms to Float but declines
// the concrete accessor, so it renders via String().
func TestB3ValueToAnyDepScalarFloat(t *testing.T) {
	r := seam5Reg(t)
	out, err := seam5Run(r, `(Float gt 1.0)`)
	if err != nil || len(out) == 0 {
		t.Fatalf("could not build DepScalar float: %v / %v", out, err)
	}
	if _, ok := valueToAny(out[0]).(string); !ok {
		t.Fatalf("expected string fallback for DepScalar float, got %T", valueToAny(out[0]))
	}
	// Positive pair: a concrete float projects to float64.
	if _, ok := valueToAny(NewFloat(1.5)).(float64); !ok {
		t.Fatalf("expected float64 for concrete float")
	}
}

// TestB3ValidateRejects covers validateHandler's Validate-error arm: a spec
// that rejects the data surfaces as a validate error.
func TestB3ValidateRejects(t *testing.T) {
	r := seam5Reg(t)
	spec := NewOrderedMap()
	spec.Set("b", NewMap(NewOrderedMap()))
	data := NewOrderedMap()
	data.Set("a", NewInteger(1))
	if _, err := validateHandler([]Value{NewMap(spec), NewMap(data)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "validate:") {
		t.Fatalf("validate: expected rejection error, got %v", err)
	}
	// Positive pair: matching data validates.
	ok := NewOrderedMap()
	ok.Set("a", NewInteger(1))
	if _, err := validateHandler([]Value{NewMap(ok), NewMap(ok)}, nil, nil, r); err != nil {
		t.Fatalf("validate: expected success, got %v", err)
	}
}

func TestB3FilterReachNotReach(t *testing.T) {
	r := seam5Reg(t)
	// filterReachHandler: args[0] is not a Reach -> AsReach error.
	if _, err := filterReachHandler([]Value{NewInteger(1), NewList([]Value{NewInteger(1)})}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "filter:") {
		t.Fatalf("filterReach: expected AsReach error, got %v", err)
	}
}

// TestB3FilterReachApplyError covers the lens-form ApplyReach failure arms:
// a reach that can't apply to an element (dot on an Integer) surfaces the
// error, wrapped per-element (list) and per-key (map).
func TestB3FilterReachApplyError(t *testing.T) {
	// List branch: element error wrapping.
	if _, err := seam5Run(seam5Reg(t), `filter $.foo [1 2 3]`); err == nil ||
		!strings.Contains(err.Error(), "filter: element 0:") {
		t.Fatalf("filterReach list: expected element error, got %v", err)
	}
	// Map branch: key error wrapping.
	if _, err := seam5Run(seam5Reg(t), `filter $.foo {x:1 y:2}`); err == nil ||
		!strings.Contains(err.Error(), `filter: key "x":`) {
		t.Fatalf("filterReach map: expected key error, got %v", err)
	}
	// Positive pair: a lens that applies cleanly keeps the truthy entries.
	out, err := seam5Run(seam5Reg(t), `filter $.a [{a:true} {a:false}]`)
	if err != nil || len(out) != 1 {
		t.Fatalf("filterReach positive: unexpected %v / %v", out, err)
	}
}
