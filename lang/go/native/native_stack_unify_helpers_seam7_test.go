package native

import (
	"math"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// Seam-7 coverage for the check-mode narrowers and helpers in
// native_stack.go, native_unify.go, native_helpers.go and
// native_definition_helpers.go (design/TEST-SEAMS.10.md).

func TestSeam7StackCheckNarrowers(t *testing.T) {
	r := seam5Reg(t)

	// pickCheckFullStack: empty stack → Any element.
	if got := pickCheckFullStack(nil, nil, r); len(got) != 1 || !got[0].Parent().ConformsTo(TAny) {
		t.Fatalf("pickCheckFullStack empty: %v", got)
	}
	// Homogeneous stack keeps the common type.
	hom := []Value{NewInteger(1), NewInteger(2)}
	if got := pickCheckFullStack(nil, hom, r); !got[len(got)-1].Parent().ConformsTo(TInteger) {
		t.Fatalf("pickCheckFullStack homogeneous: %v", got[len(got)-1].Parent())
	}
	// A stack whose common ancestor widens to Any breaks the loop early.
	mixed := []Value{NewInteger(1), NewMap(NewOrderedMap()), NewList(nil)}
	if got := pickCheckFullStack(nil, mixed, r); !got[len(got)-1].Parent().ConformsTo(TAny) {
		t.Fatalf("pickCheckFullStack mixed: %v", got[len(got)-1].Parent())
	}

	// rollCheckFullStack: empty → nil.
	if got := rollCheckFullStack(nil, nil, r); got != nil {
		t.Fatalf("rollCheckFullStack empty must be nil, got %v", got)
	}
	if got := rollCheckFullStack(nil, hom, r); len(got) != 2 {
		t.Fatalf("rollCheckFullStack: %v", got)
	}
}

func TestSeam7ListEdgeElemReturns(t *testing.T) {
	r := seam5Reg(t)
	popNarrow := listEdgeElemReturns(true)
	shiftNarrow := listEdgeElemReturns(false)

	// Wrong arity → fallback.
	if got := popNarrow(nil, r); len(got) != 2 || !got[1].Parent().ConformsTo(TAny) {
		t.Fatalf("pop narrow arity fallback: %v", got)
	}
	// Non-concrete arg → fallback.
	if got := popNarrow([]Value{NewTypeLiteral(TList)}, r); !got[1].Parent().ConformsTo(TAny) {
		t.Fatalf("pop narrow non-concrete fallback: %v", got)
	}
	// Empty list → fallback.
	if got := popNarrow([]Value{NewList(nil)}, r); !got[1].Parent().ConformsTo(TAny) {
		t.Fatalf("pop narrow empty fallback: %v", got)
	}
	// Undefined edge element → fallback.
	u := NewInteger(1)
	SetUndefined(&u, true)
	if got := popNarrow([]Value{NewList([]Value{u})}, r); !got[1].Parent().ConformsTo(TAny) {
		t.Fatalf("pop narrow undefined fallback: %v", got)
	}
	// Any-typed edge element → fallback.
	if got := shiftNarrow([]Value{NewList([]Value{NewDynamicCarrier(TAny)})}, r); !got[1].Parent().ConformsTo(TAny) {
		t.Fatalf("shift narrow Any fallback: %v", got)
	}
	// Dynamic (but typed) edge element propagates its gradual claim.
	got := popNarrow([]Value{NewList([]Value{NewDynamicCarrier(TInteger)})}, r)
	if !got[1].Parent().ConformsTo(TInteger) || !IsDynamic(&got[1]) {
		t.Fatalf("pop narrow dynamic element: %v dyn=%v", got[1].Parent(), IsDynamic(&got[1]))
	}
	// Concrete edge element narrows to its type.
	if got := popNarrow([]Value{NewList([]Value{NewInteger(9)})}, r); !got[1].Parent().ConformsTo(TInteger) {
		t.Fatalf("pop narrow concrete element: %v", got[1].Parent())
	}
}

func TestSeam7UnifyFoldable(t *testing.T) {
	// A bare type node folds.
	if !unifyFoldable(NewTypeLiteral(TInteger)) {
		t.Fatal("type literal must fold")
	}
	// A carrier does not.
	if unifyFoldable(NewCarrier(TInteger)) {
		t.Fatal("carrier must not fold")
	}
	// A concrete scalar folds.
	if !unifyFoldable(NewInteger(1)) {
		t.Fatal("scalar must fold")
	}
	// A concrete list of foldables folds; a list holding a carrier does not.
	if !unifyFoldable(NewList([]Value{NewInteger(1)})) {
		t.Fatal("list of scalars must fold")
	}
	if unifyFoldable(NewList([]Value{NewCarrier(TInteger)})) {
		t.Fatal("list holding a carrier must not fold")
	}
	// A broken list (Parent=List but non-list payload) does not fold.
	badList := NewInteger(1)
	badList.SetParent(TList)
	if unifyFoldable(badList) {
		t.Fatal("broken list must not fold")
	}
	// A concrete map of foldables folds; a map with a carrier value does not.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	if !unifyFoldable(NewMap(om)) {
		t.Fatal("map of scalars must fold")
	}
	bm := NewOrderedMap()
	bm.Set("a", NewCarrier(TInteger))
	if unifyFoldable(NewMap(bm)) {
		t.Fatal("map holding a carrier must not fold")
	}
	// A broken map does not fold.
	badMap := NewInteger(1)
	badMap.SetParent(TMap)
	if unifyFoldable(badMap) {
		t.Fatal("broken map must not fold")
	}
}

func TestSeam7UnifyFoldReturns(t *testing.T) {
	r := seam5Reg(t)

	// Non-foldable operand → fallback.
	fb := unifyFoldReturns([]Value{NewCarrier(TInteger), NewInteger(1)}, r)
	if len(fb) != 2 || !fb[1].Parent().ConformsTo(TBoolean) {
		t.Fatalf("unifyFoldReturns fallback: %v", fb)
	}

	// Foldable scalars fold to the real (String, Boolean) fail shape.
	out := unifyFoldReturns([]Value{NewInteger(2), NewInteger(3)}, r)
	if len(out) != 2 {
		t.Fatalf("unifyFoldReturns scalars: %v", out)
	}

	// A Disjunct unify result rides dynamic.
	disj, err := seam5Run(r, `tany [Integer String]`)
	if err != nil || len(disj) == 0 {
		t.Fatalf("build disjunct: %v / %v", disj, err)
	}
	dv := disj[len(disj)-1]
	if !IsDisjunct(&dv) {
		t.Skip("tany did not yield a disjunct value")
	}
	dout := unifyFoldReturns([]Value{dv, dv}, r)
	if len(dout) != 2 {
		t.Fatalf("unifyFoldReturns disjunct: %v", dout)
	}
	// The folded result is a dynamic carrier (type-as-value ridden dynamic).
	if !IsDynamic(&dout[0]) {
		t.Fatalf("expected disjunct result ridden dynamic, got %v dyn=%v", dout[0].Parent(), IsDynamic(&dout[0]))
	}
}

func TestSeam7DecimalContextNilRegistry(t *testing.T) {
	// A nil registry (or nil Contexts) yields the package default context.
	if decimalContext(nil) == nil {
		t.Fatal("decimalContext(nil) must return the default context")
	}
	r := seam5Reg(t)
	if decimalContext(r) == nil {
		t.Fatal("decimalContext(r) must return a context")
	}
}

func TestSeam7IsStaticZeroIntDivisor(t *testing.T) {
	// Non-concrete → false.
	if isStaticZeroIntDivisor(NewCarrier(TInteger)) {
		t.Fatal("carrier must not be a static zero divisor")
	}
	// A broken BigInteger (Parent claims BigInteger, integer payload) hits
	// the AsBigInteger-failure return.
	badBig := NewInteger(1)
	badBig.SetParent(TBigInteger)
	if isStaticZeroIntDivisor(badBig) {
		t.Fatal("broken bigint must fall through to false")
	}
	// A real zero Integer is a static zero divisor; a non-zero is not.
	if !isStaticZeroIntDivisor(NewInteger(0)) {
		t.Fatal("0 must be a static zero divisor")
	}
	if isStaticZeroIntDivisor(NewInteger(5)) {
		t.Fatal("5 must not be a static zero divisor")
	}
}

func TestSeam7ApdUnaryBigError(t *testing.T) {
	r := seam5Reg(t)
	// Build an apd-backed unary word; the BigInteger/BigDecimal sigs use
	// bigHandler, whose apd-error arm we drive with sqrt of a negative.
	nf := ApdUnaryNative("t-sqrt", math.Sqrt, (*apd.Context).Sqrt)
	bigHandler := nf.Signatures[2].Impl.(*GoImpl).Handler

	neg, err := seam5Run(r, `-0d4`)
	if err != nil || len(neg) == 0 {
		t.Fatalf("build negative bigint: %v / %v", neg, err)
	}
	if _, err := bigHandler([]Value{neg[len(neg)-1]}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "t-sqrt") {
		t.Fatalf("expected apd sqrt error, got %v", err)
	}

	// Positive: sqrt of a positive Big value succeeds.
	pos, err := seam5Run(r, `0d4`)
	if err != nil || len(pos) == 0 {
		t.Fatalf("build positive bigint: %v / %v", pos, err)
	}
	if _, err := bigHandler([]Value{pos[len(pos)-1]}, nil, nil, r); err != nil {
		t.Fatalf("apd sqrt positive: %v", err)
	}
}

func TestSeam7DefStackOnly(t *testing.T) {
	// A /s-modified word name is stack-only.
	if !defStackOnly(NewWordModified("foo", -1, true, false)) {
		t.Fatal("ForceStack word must be stack-only")
	}
	// A plain word name is not.
	if defStackOnly(NewWord("foo")) {
		t.Fatal("plain word must not be stack-only")
	}
	// A non-word value is not.
	if defStackOnly(NewInteger(1)) {
		t.Fatal("non-word must not be stack-only")
	}
}
