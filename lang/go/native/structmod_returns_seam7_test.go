package native

import (
	"testing"
)

// Seam-7 coverage for struct_module.go check-mode return-inference helpers
// and native_math / native_misc edge arms (design/TEST-SEAMS.10.md).

func TestB3DynamicContainerKind(t *testing.T) {
	// Dynamic Any carrier -> dynamic(Any) (the t==nil || Dynamic&&Any arm).
	if got := dynamicContainerKind(NewDynamicCarrier(TAny)); !ParentOf(&got).ConformsTo(TAny) {
		t.Fatalf("dynamic Any -> Any, got %v", got)
	}
	// Positive pairs: Map -> Map, List -> List, scalar -> Any.
	if got := dynamicContainerKind(NewMap(NewOrderedMap())); !ParentOf(&got).ConformsTo(TMap) {
		t.Fatalf("map -> Map, got %v", got)
	}
	if got := dynamicContainerKind(NewList(nil)); !ParentOf(&got).ConformsTo(TList) {
		t.Fatalf("list -> List, got %v", got)
	}
	if got := dynamicContainerKind(NewInteger(1)); !ParentOf(&got).ConformsTo(TAny) {
		t.Fatalf("scalar -> Any, got %v", got)
	}
}

func TestB3MergeReturnsNilType(t *testing.T) {
	// A carrier with no Parent has a nil ValueType -> dyn(Any) fallback.
	got := mergeReturns([]Value{MakeCarrierValue(nil, nil), NewMap(NewOrderedMap())}, nil)
	if len(got) != 1 || !ParentOf(&got[0]).ConformsTo(TAny) {
		t.Fatalf("mergeReturns(nil-type) must be Any, got %v", got)
	}
	// Positive pair: two maps -> dynamic(Map).
	got2 := mergeReturns([]Value{NewMap(NewOrderedMap()), NewMap(NewOrderedMap())}, nil)
	if len(got2) != 1 || !ParentOf(&got2[0]).ConformsTo(TMap) {
		t.Fatalf("mergeReturns(map,map) must be Map, got %v", got2)
	}
}

func TestB3ReifyReturnsAnyTarget(t *testing.T) {
	// A bare Any target resolves to type Any -> dyn(Any).
	got := reifyReturns([]Value{NewTypeLiteral(TAny), NewMap(NewOrderedMap())}, nil)
	if len(got) != 1 || !ParentOf(&got[0]).ConformsTo(TAny) {
		t.Fatalf("reifyReturns(Any) must be Any, got %v", got)
	}
	// Positive pair: wrong arity -> Any.
	if got := reifyReturns([]Value{NewTypeLiteral(TAny)}, nil); len(got) != 1 {
		t.Fatalf("reifyReturns(1 arg) must yield one carrier, got %v", got)
	}
}

func TestB3WithDecimalBadBody(t *testing.T) {
	r := seam5Reg(t)
	// Body is not a concrete list -> type error.
	if _, err := withDecimalHandler([]Value{NewMap(NewOrderedMap()), NewInteger(1)}, nil, nil, r); err == nil {
		t.Fatalf("with-decimal: expected body-not-a-list error")
	}
}

func TestB3ExportNoopNonMap(t *testing.T) {
	r := seam5Reg(t)
	done := r.Check.Begin()
	defer done()
	// In check mode with a concrete non-map arg, RequireConcreteMap fails and
	// the handler silently returns.
	out, err := exportNoopHandler([]Value{NewString("x"), NewInteger(1)}, nil, nil, r)
	if err != nil || out != nil {
		t.Fatalf("exportNoop(non-map): expected silent nil, got %v / %v", out, err)
	}
}
