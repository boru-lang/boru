package native

import (
	"strings"
	"testing"
)

// Seam-6 coverage for native_object_record.go error arms
// (design/TEST-SEAMS.10.md).

func TestSeam6C2RecordHandlerArms(t *testing.T) {
	r := seam5Reg(t)

	// Not a list at all.
	_, err := recordHandler([]Value{NewInteger(1)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "must be a list") {
		t.Fatalf("expected list-type error, got %v", err)
	}

	// A List carrier is list-shaped but not concrete.
	_, err = recordHandler([]Value{NewCarrier(TList)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "concrete list") {
		t.Fatalf("expected concrete-list error, got %v", err)
	}

	// An element that is map-shaped but not concrete.
	_, err = recordHandler([]Value{NewList([]Value{NewCarrier(TMap)})}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "concrete pair") {
		t.Fatalf("expected concrete-pair error, got %v", err)
	}

	// Positive pair: a single-pair map list mints a record type.
	pair := NewOrderedMap()
	pair.Set("a", NewTypeLiteral(TInteger))
	out, err := recordHandler([]Value{NewList([]Value{NewMap(pair)})}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("expected record type, got %v / %v", out, err)
	}
}

func TestSeam6C2ClassHandlerArms(t *testing.T) {
	r := seam5Reg(t)

	// Non-map argument, no pending gen.
	_, err := classHandler([]Value{NewInteger(1)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "must be a map") {
		t.Fatalf("expected map-type error, got %v", err)
	}

	// Non-map argument WITH a pending gen spec: the placeholder
	// bindings are unwound before erroring.
	if err := r.SetPendingGen(&GenSpecInfo{}); err != nil {
		t.Fatal(err)
	}
	_, err = classHandler([]Value{NewInteger(1)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "must be a map") {
		t.Fatalf("expected map-type error with gen, got %v", err)
	}
	if r.PendingGen() != nil {
		t.Fatal("pending gen must have been consumed")
	}

	// Map-shaped but not concrete, without and with a pending gen.
	_, err = classHandler([]Value{NewCarrier(TMap)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "concrete map") {
		t.Fatalf("expected concrete-map error, got %v", err)
	}
	if err := r.SetPendingGen(&GenSpecInfo{}); err != nil {
		t.Fatal(err)
	}
	_, err = classHandler([]Value{NewCarrier(TMap)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "concrete map") {
		t.Fatalf("expected concrete-map error with gen, got %v", err)
	}
	if r.PendingGen() != nil {
		t.Fatal("pending gen must have been consumed")
	}

	// Positive pair: a concrete schema map mints a class type.
	schema := NewOrderedMap()
	schema.Set("a", NewTypeLiteral(TInteger))
	out, err := classHandler([]Value{NewMap(schema)}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsClassType(&out[0]) {
		t.Fatalf("expected class type, got %v / %v", out, err)
	}
}

func TestSeam6C2ObjectWithParentHandlerArms(t *testing.T) {
	r := seam5Reg(t)

	// First argument must be a map.
	_, err := objectWithParentHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "must be a map") {
		t.Fatalf("expected map-type error, got %v", err)
	}

	// Map-shaped but not concrete.
	_, err = objectWithParentHandler([]Value{NewCarrier(TMap), NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "concrete map") {
		t.Fatalf("expected concrete-map error, got %v", err)
	}

	// Parent must be an object type.
	empty := NewMap(NewOrderedMap())
	_, err = objectWithParentHandler([]Value{empty, NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "parent must be an object type") {
		t.Fatalf("expected parent-type error, got %v", err)
	}

	// A parent ClassTypeInfo with no minted *Type falls back to the
	// TClass branch node.
	parentFields := NewOrderedMap()
	parentFields.Set("a", NewTypeLiteral(TInteger))
	bareParent := NewValueRaw(TClass, ClassTypeInfo{Fields: parentFields, ID: "seam6c2-bare"})
	childFields := NewOrderedMap()
	childFields.Set("b", NewTypeLiteral(TString))
	out, err := objectWithParentHandler([]Value{NewMap(childFields), bareParent}, nil, nil, r)
	if err != nil || len(out) != 1 || !IsClassType(&out[0]) {
		t.Fatalf("expected extended class type, got %v / %v", out, err)
	}
	info, err2 := AsClassType(out[0])
	if err2 != nil || info.Parent == nil {
		t.Fatalf("expected parent info attached, got %v", err2)
	}
}
