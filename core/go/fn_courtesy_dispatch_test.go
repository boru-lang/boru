package core

import "testing"

// TestFnCourtesyDispatches pins every branch of fnCourtesyDispatches, the
// standalone mirror of fnFallbackSig's 0-arg courtesy-dispatch condition
// (engine.go). stepWord consults it to decide whether a selected fallback
// signature is a genuine no-match (raise the rich sigError) or a courtesy
// dispatch (let fnFallbackSig run). matchSignature resolves a real 0-arg sig
// before the fallback at the live call site, so the positive verdict is only
// reachable through this direct probe — exactly what the unit test exists for.
func TestFnCourtesyDispatches(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	fn := &FnDefInfo{Name: "z"}

	// nil registry / nil fn short-circuit to false.
	if fnCourtesyDispatches(nil, "z", fn) {
		t.Fatal("nil registry should not courtesy-dispatch")
	}
	if fnCourtesyDispatches(r, "z", nil) {
		t.Fatal("nil fn should not courtesy-dispatch")
	}

	// Name not bound in the def stack → false.
	if fnCourtesyDispatches(r, "unbound", fn) {
		t.Fatal("unbound name should not courtesy-dispatch")
	}

	// Bound to a non-FnDefInfo value (a plain Function / any binding) → false.
	r.Defs.Push("plain", NewInteger(1))
	if fnCourtesyDispatches(r, "plain", fn) {
		t.Fatal("non-FnDefInfo binding should not courtesy-dispatch")
	}

	// Bound to an FnDefInfo but the aggregate declares no 0-arg handler sig
	// (only a forward-arg sig) → false.
	r.Defs.Push("fwd", Value{Parent: TFunction, Data: &FnDefInfo{Name: "fwd"}})
	fwdOnly := &FnDefInfo{Name: "fwd", Signatures: []Signature{{
		Args: []*Type{TInteger},
		Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return nil, nil
		}),
		BarrierPos: -1,
	}}}
	if fnCourtesyDispatches(r, "fwd", fwdOnly) {
		t.Fatal("a forward-only fn should not courtesy-dispatch")
	}

	// Bound to an FnDefInfo that declares a 0-arg non-fallback handler sig
	// → true (the courtesy-dispatch verdict).
	r.Defs.Push("z", Value{Parent: TFunction, Data: &FnDefInfo{Name: "z"}})
	zeroArg := &FnDefInfo{Name: "z", Signatures: []Signature{{
		Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
			return nil, nil
		}),
	}}}
	if !fnCourtesyDispatches(r, "z", zeroArg) {
		t.Fatal("a fn with a 0-arg handler sig should courtesy-dispatch")
	}
}
