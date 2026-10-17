package native

import (
	"strings"
	"testing"
)

// Seam-7 coverage for native_ref.go (design/TEST-SEAMS.10.md). The
// dispatch-modifier words (ref / usurp / stack-args / forward-args /
// force-arity) and apply / rebind are driven through their handlers with
// crafted atoms and values, covering the missing-name, non-atom,
// unbound (reg==nil), and non-fn arms. Positive calls are paired in.

func seam7Fn(t *testing.T, r *Registry) Value {
	t.Helper()
	out, err := seam5Run(r, `fn [[x:Integer] [Integer] [x]]`)
	if err != nil || len(out) == 0 {
		t.Fatalf("build fn value: %v / %v", out, err)
	}
	return out[len(out)-1]
}

// seam7RegWithBindings returns a registry with a fn `f` and a value `v5`.
func seam7RegWithBindings(t *testing.T) *Registry {
	t.Helper()
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def f fn [[x:Integer] [Integer] [x]] def v5 5`); err != nil {
		t.Fatalf("bindings: %v", err)
	}
	return r
}

func TestSeam7RefHandler(t *testing.T) {
	r := seam7RegWithBindings(t)

	// Positive: resolve a bound fn.
	out, err := valofHandler([]Value{NewAtom("f")}, nil, nil, r)
	if err != nil || len(out) != 1 || !out[0].Parent().Equal(TFunction) {
		t.Fatalf("valofHandler positive: %v / %v", out, err)
	}
	// Missing name.
	if _, err := valofHandler(nil, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "missing name") {
		t.Fatalf("expected missing-name error, got %v", err)
	}
	// Non-atom argument.
	if _, err := valofHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected an atom") {
		t.Fatalf("expected atom error, got %v", err)
	}
	// Unbound name with a nil registry (ResolveRef(nil) → not bound).
	if _, err := valofHandler([]Value{NewAtom("nope")}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not bound") {
		t.Fatalf("expected unbound error, got %v", err)
	}
	// A bound non-fn value comes back AS ITSELF — `valof` is total over
	// binding kinds, so there is no function-only rejection any more.
	out, verr := valofHandler([]Value{NewAtom("v5")}, nil, nil, r)
	if verr != nil {
		t.Fatalf("non-fn binding: unexpected error: %v", verr)
	}
	if len(out) != 1 {
		t.Fatalf("non-fn binding: got %v, want exactly one value", out)
	}
}

func TestSeam7UsurpHandlers(t *testing.T) {
	r := seam7RegWithBindings(t)
	fn := seam7Fn(t, r)

	// usurpHandler positive + non-fn (reg==nil) arm.
	if _, err := usurpHandler([]Value{fn}, nil, nil, r); err != nil {
		t.Fatalf("usurpHandler positive: %v", err)
	}
	if _, err := usurpHandler([]Value{NewInteger(3)}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "function value") {
		t.Fatalf("expected usurp non-fn error, got %v", err)
	}

	// usurpAtomHandler: positive, missing name, non-atom, unbound, non-fn.
	if _, err := usurpAtomHandler([]Value{NewAtom("f")}, nil, nil, r); err != nil {
		t.Fatalf("usurpAtomHandler positive: %v", err)
	}
	if _, err := usurpAtomHandler(nil, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "missing name") {
		t.Fatalf("expected usurp missing-name, got %v", err)
	}
	if _, err := usurpAtomHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected an atom") {
		t.Fatalf("expected usurp atom error, got %v", err)
	}
	if _, err := usurpAtomHandler([]Value{NewAtom("nope")}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not bound") {
		t.Fatalf("expected usurp unbound, got %v", err)
	}
	if _, err := usurpAtomHandler([]Value{NewAtom("v5")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "function word") {
		t.Fatalf("expected usurp illegal_ref, got %v", err)
	}
}

func TestSeam7RebarrierHandlers(t *testing.T) {
	r := seam7RegWithBindings(t)
	fn := seam7Fn(t, r)

	// Value forms (rebarrierResult) — positive + non-fn (reg==nil) arm.
	if _, err := stackArgsHandler([]Value{fn}, nil, nil, r); err != nil {
		t.Fatalf("stackArgsHandler positive: %v", err)
	}
	if _, err := forwardArgsHandler([]Value{fn}, nil, nil, r); err != nil {
		t.Fatalf("forwardArgsHandler positive: %v", err)
	}
	if _, err := stackArgsHandler([]Value{NewInteger(1)}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "function value") {
		t.Fatalf("expected stack-args non-fn error, got %v", err)
	}

	// By-name forms (rebarrierAtom).
	if _, err := stackArgsAtomHandler([]Value{NewAtom("f")}, nil, nil, r); err != nil {
		t.Fatalf("stackArgsAtomHandler positive: %v", err)
	}
	if _, err := forwardArgsAtomHandler([]Value{NewAtom("f")}, nil, nil, r); err != nil {
		t.Fatalf("forwardArgsAtomHandler positive: %v", err)
	}
	if _, err := stackArgsAtomHandler(nil, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "missing name") {
		t.Fatalf("expected rebarrier missing-name, got %v", err)
	}
	if _, err := stackArgsAtomHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected an atom") {
		t.Fatalf("expected rebarrier atom error, got %v", err)
	}
	if _, err := stackArgsAtomHandler([]Value{NewAtom("nope")}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not bound") {
		t.Fatalf("expected rebarrier unbound, got %v", err)
	}
	if _, err := stackArgsAtomHandler([]Value{NewAtom("v5")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "function word") {
		t.Fatalf("expected rebarrier illegal_ref, got %v", err)
	}
}

func TestSeam7ForceArityHandlers(t *testing.T) {
	r := seam7RegWithBindings(t)
	fn := seam7Fn(t, r)

	// forceArityHandler positive + non-fn (reg==nil) arm.
	if _, err := forceArityHandler([]Value{NewInteger(1), fn}, nil, nil, r); err != nil {
		t.Fatalf("forceArityHandler positive: %v", err)
	}
	if _, err := forceArityHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "force-arity") {
		t.Fatalf("expected force-arity non-fn error, got %v", err)
	}

	// forceArityAtomHandler: positive, non-atom, unbound, non-fn.
	if _, err := forceArityAtomHandler([]Value{NewInteger(1), NewAtom("f")}, nil, nil, r); err != nil {
		t.Fatalf("forceArityAtomHandler positive: %v", err)
	}
	if _, err := forceArityAtomHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected an atom") {
		t.Fatalf("expected force-arity atom error, got %v", err)
	}
	if _, err := forceArityAtomHandler([]Value{NewInteger(1), NewAtom("nope")}, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "not bound") {
		t.Fatalf("expected force-arity unbound, got %v", err)
	}
	if _, err := forceArityAtomHandler([]Value{NewInteger(1), NewAtom("v5")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "function word") {
		t.Fatalf("expected force-arity illegal_ref, got %v", err)
	}
}

func TestSeam7ApplyRebindHandlers(t *testing.T) {
	r := seam5Reg(t)
	fn := seam7Fn(t, r)

	// applyHandler positive.
	if _, err := applyHandler([]Value{fn}, nil, nil, r); err != nil {
		t.Fatalf("applyHandler positive: %v", err)
	}
	// Not a Function.
	if _, err := applyHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected Function") {
		t.Fatalf("expected apply Function error, got %v", err)
	}
	// Function-typed value with no FnDefInfo payload.
	bad := NewInteger(1)
	bad.SetParent(TFunction)
	if _, err := applyHandler([]Value{bad}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "FnDefInfo") {
		t.Fatalf("expected apply FnDefInfo error, got %v", err)
	}

	// applyReachHandler: positive + non-reach receiver error.
	reach, err := seam5Run(r, `$.a`)
	if err != nil || len(reach) == 0 {
		t.Fatalf("build reach: %v / %v", reach, err)
	}
	rv := reach[len(reach)-1]
	om := NewOrderedMap()
	om.Set("a", NewInteger(7))
	out, err := applyReachHandler([]Value{rv, NewMap(om)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("applyReachHandler positive: %v / %v", out, err)
	}
	if _, err := applyReachHandler([]Value{NewInteger(1), NewMap(om)}, nil, nil, r); err == nil {
		t.Fatal("expected apply reach non-reach error")
	}

	// rebindHandler: positive + non-reach error.
	if _, err := rebindHandler([]Value{rv, NewMap(om)}, nil, nil, r); err != nil {
		t.Fatalf("rebindHandler positive: %v", err)
	}
	if _, err := rebindHandler([]Value{NewInteger(1), NewMap(om)}, nil, nil, r); err == nil {
		t.Fatal("expected rebind non-reach error")
	}
}
