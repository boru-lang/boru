package native

import (
	"testing"
)

// Coverage for the mini/emit VALUE forms' arms a surface program cannot
// reach through sig dispatch — mirrors the parse fn-form seams
// (native_macro_parsefn_test.go). See design/TEST-SEAMS.10.md.

// A fn with no signatures is not filter-shaped: the zero-sig arm is
// unreachable from surface fns (`fn` always yields ≥1 sig) but the exported
// predicate must not claim an uninhabited shape.
func TestMiniLangFnFilterShapedEmpty(t *testing.T) {
	if MiniLangFnFilterShaped(FnDefInfo{}) {
		t.Fatal("a fn with no signatures must not be filter-shaped")
	}
}

// A fn-family value whose payload is not an FnDefInfo is declined (the sig
// matcher never delivers one from surface syntax — a bare `Function` type
// literal is parented at Type; pinned by the §fnv signature_error rows).
func TestMiniFnExpandNonFnPayload(t *testing.T) {
	r := seam5Reg(t)
	_, err := miniHandler([]Value{NewValueRaw(TFunction, IntPayload{N: 1}), NewString("x")}, nil, nil, r)
	if err == nil {
		t.Fatal("mini: a non-FnDefInfo function payload must error")
	}
}

func TestEmitFnExpandNonFnPayload(t *testing.T) {
	r := seam5Reg(t)
	_, err := emitHandler([]Value{NewValueRaw(TFunction, IntPayload{N: 1}), NewMap(NewOrderedMap())}, nil, nil, r)
	if err == nil {
		t.Fatal("emit: a non-FnDefInfo function payload must error")
	}
}

// A TFunction CARRIER operand (a computed transducer/emitter under
// analysis) degrades to a dynamic value with the macro advisory.
func TestMiniFnExpandCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	out, err := miniHandler([]Value{NewCarrier(TFunction), NewString("x")}, nil, nil, r)
	if err != nil {
		t.Fatalf("mini: a carrier fn under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("mini: carrier fn should yield a dynamic carrier, got %v", Canon(out))
	}
}

func TestEmitFnExpandCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	out, err := emitHandler([]Value{NewCarrier(TFunction), NewMap(NewOrderedMap())}, nil, nil, r)
	if err != nil {
		t.Fatalf("emit: a carrier fn under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("emit: carrier fn should yield a dynamic carrier, got %v", Canon(out))
	}
}

// A bare word BOUND to a TFunction carrier takes the name-path carrier arm:
// degrade + advisory, with the binding recorded as used.
func TestMiniFnNameBoundCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Defs.Push("p", NewCarrier(TFunction))
	out, err := miniHandler([]Value{NewAtom("p"), NewString("x")}, nil, nil, r)
	if err != nil {
		t.Fatalf("mini: a carrier fn binding under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("mini: carrier fn binding should yield a dynamic carrier, got %v", Canon(out))
	}
	if !r.Check.DefsUsed["p"] {
		t.Fatal("mini: the transducer binding must be recorded as used")
	}
}

func TestEmitFnNameBoundCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Defs.Push("p", NewCarrier(TFunction))
	out, err := emitHandler([]Value{NewAtom("p"), NewMap(NewOrderedMap())}, nil, nil, r)
	if err != nil {
		t.Fatalf("emit: a carrier fn binding under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("emit: carrier fn binding should yield a dynamic carrier, got %v", Canon(out))
	}
	if !r.Check.DefsUsed["p"] {
		t.Fatal("emit: the emitter binding must be recorded as used")
	}
}

// Outside check mode a non-concrete fn-family binding is NOT a transducer:
// mini falls through to the ordinary unknown-kind error. (emit's equivalent
// falls through to its pre-existing auto path — no new statements.)
func TestMiniFnNameBoundTypeLiteralFallsThrough(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("p", NewTypeLiteral(TFunction))
	if _, err := miniHandler([]Value{NewAtom("p"), NewString("x")}, nil, nil, r); err == nil {
		t.Fatal("mini: a Function type-literal binding must fall through to unknown-kind")
	}
}
