package native

import (
	"testing"
)

// W9_nativeB — coverage for native_type.go type-construction / algebra /
// membership arms (refine, is, base, tany/tall folds, makeOptionalType,
// installIdeals init guards). See design/TEST-SEAMS.10.md.

// --- refinePlain: an Ideal with no Construct ---

func TestW9RefinePlainKindNotConstructable(t *testing.T) {
	r := seam5Reg(t)
	// The Resource Ideal has an Instantiate but no Construct, so `refine
	// Resource {…}` reaches the "cannot be constructed with refine" arm.
	res, ok := r.Defs.Top(resourceDefKey("Resource"))
	if !ok {
		t.Fatal("Resource schema not bound under the hidden key")
	}
	if _, err := refinePlain(res, NewMap(NewOrderedMap()), r); err == nil {
		t.Fatal("refine Resource must reject (kind not constructable)")
	}
}

// --- Binary / BinarySpec member predicates run via `is` ---

func TestW9BinaryMembershipPredicatesRun(t *testing.T) {
	r := seam5Reg(t)
	bin := r.LookupTypeName("Binary")
	bspec := r.LookupTypeName("BinarySpec")
	if bin == nil || bspec == nil {
		t.Fatalf("Binary/BinarySpec not minted: bin=%v bspec=%v", bin, bspec)
	}
	// A class instance (parent conforms to Class) runs each member type's
	// predicate (binaryInstanceLayout / binarySpecLayout) via the member
	// Behavior's Match — the closures installed in installIdeals. A plain
	// class instance has no binary layout, so both return false, but the
	// closure bodies execute.
	if _, err := seam5Run(r, `def C class {a:Integer}  def c:C {a:1}`); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cv, ok := r.Defs.Top("c")
	if !ok {
		t.Fatal("c not bound")
	}
	if cv.Is(bin) {
		t.Fatal("a plain class instance is not Binary")
	}
	if cv.Is(bspec) {
		t.Fatal("a plain class instance is not BinarySpec")
	}
}

// --- installIdeals init-error guards (Binary / BinarySpec already bound) ---

func TestW9InstallIdealsDuplicateMemberTypes(t *testing.T) {
	r := seam5Reg(t)
	// Shadow Binary/BinarySpec with plain VALUE bindings so DefineMemberType's
	// validateTypeName raises "name clash — already a def'd value" for both,
	// exercising installIdeals' two init-error record arms.
	r.Defs.Push("Binary", NewInteger(1))
	r.Defs.Push("BinarySpec", NewInteger(1))

	// Snapshot the package-global init-error accumulator; installIdeals
	// records init errors we must not leak into other tests.
	saved := SwapTypeInitErrs(nil)
	t.Cleanup(func() { SwapTypeInitErrs(saved) })

	before := len(TypeInitErrs())
	installIdeals(r)
	if len(TypeInitErrs()) < before+2 {
		t.Fatalf("re-installIdeals should record both Binary and BinarySpec clash errors; before=%d after=%d",
			before, len(TypeInitErrs()))
	}
}

// --- isHandler: a carrier `is Type` is false ---

func TestW9IsHandlerCarrierIsType(t *testing.T) {
	r := seam5Reg(t)
	// args[1] (the value side) is a carrier, args[0] (the type side) is the
	// Type literal → the `a.Carrier` short-circuit returns false.
	out, err := isHandler([]Value{NewTypeLiteral(TType), NewCarrier(TInteger)}, nil, nil, r)
	if err != nil {
		t.Fatalf("isHandler carrier is Type: %v", err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Fatal("a carrier is not a Type")
	}
}

// --- makeOptionalType: a disjunct already carrying None is unchanged ---

func TestW9MakeOptionalTypeCollapsesToOne(t *testing.T) {
	// `Any | None` simplifies to a single alternative (Any absorbs None), so
	// makeOptionalType returns simplified[0] rather than a Disjunct.
	out := makeOptionalType(NewTypeLiteral(TAny))
	if IsDisjunct(&out) {
		t.Fatalf("Any|None should collapse to one alt, got disjunct %s", out.String())
	}
	// Positive pair: a plain type gains a None alternative (a 2-alt disjunct).
	opt := makeOptionalType(NewTypeLiteral(TInteger))
	if !IsDisjunct(&opt) {
		t.Fatalf("Integer should become Integer|None disjunct, got %s", opt.String())
	}
	// And an already-optional disjunct is returned unchanged (idempotent).
	already := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TNone)})
	if got := makeOptionalType(already); !ValuesEqual(got, already) {
		t.Fatalf("already-optional type must be unchanged, got %s", got.String())
	}
}

// --- baseHandler: an unsupported type has no base value ---

func TestW9BaseHandlerUnsupportedType(t *testing.T) {
	// `base Function` — BaseValue has no zero for the Function type.
	if _, err := baseHandler([]Value{NewTypeLiteral(TFunction)}, nil, nil, nil); err == nil {
		t.Fatal("base of Function must error (no base value)")
	}
	// Positive pair: a supported type yields its zero.
	out, err := baseHandler([]Value{NewTypeLiteral(TInteger)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("base Integer: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 0 {
		t.Fatalf("base Integer want 0, got %d", n)
	}
}

// --- typeAlgebraListFold: non-list-concrete arg, and handler yielding !=1 ---

func TestW9TypeAlgebraListFoldNonListConcrete(t *testing.T) {
	r := seam5Reg(t)
	fold := typeAlgebraListFold(tanyHandler)
	// A concrete Map is IsConcrete but AsList yields nil → dynamic fallback.
	out := fold([]Value{NewMap(NewOrderedMap())}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("non-list concrete arg should fall back to a dynamic carrier, got %v", Canon(out))
	}
}

func TestW9TypeAlgebraListFoldHandlerBadArity(t *testing.T) {
	r := seam5Reg(t)
	var bad Handler = func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewInteger(1), NewInteger(2)}, nil // len != 1
	}
	fold := typeAlgebraListFold(bad)
	lst := NewList([]Value{NewInteger(1), NewInteger(2)})
	out := fold([]Value{lst}, r)
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("a handler returning !=1 value should fall back to a dynamic carrier, got %v", Canon(out))
	}
}
