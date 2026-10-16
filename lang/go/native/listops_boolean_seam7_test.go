package native

import (
	"strings"
	"testing"
)

// Seam-7 coverage for listops.go (push/pop/unshift/shift, plain + flex) and
// native_boolean.go edge arms (design/TEST-SEAMS.10.md).

func TestB3PushUnshiftAdoptError(t *testing.T) {
	r := seam5Reg(t)
	lst := NewList([]Value{NewInteger(1)})
	// An empty FlexList contains flex but has no concrete backing slice, so
	// AdoptIntoNode (immutable snapshot) fails.
	emptyFlex := NewFlexList(nil)
	if _, err := pushHandler([]Value{emptyFlex, lst}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "push") {
		t.Fatalf("push: expected AdoptIntoNode error, got %v", err)
	}
	if _, err := unshiftHandler([]Value{emptyFlex, lst}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "unshift") {
		t.Fatalf("unshift: expected AdoptIntoNode error, got %v", err)
	}
	// Positive pair: pushing a plain element works.
	out, err := pushHandler([]Value{NewInteger(9), lst}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("push positive: unexpected %v / %v", out, err)
	}
}

func TestB3FlexReceiverErrors(t *testing.T) {
	r := seam5Reg(t)
	notFlex := NewList([]Value{NewInteger(1)})
	elem := NewInteger(9)
	if _, err := pushFlexHandler([]Value{elem, notFlex}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected a FlexList") {
		t.Fatalf("pushFlex: expected FlexList receiver error, got %v", err)
	}
	if _, err := popFlexHandler([]Value{notFlex}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected a FlexList") {
		t.Fatalf("popFlex: expected FlexList receiver error, got %v", err)
	}
	if _, err := unshiftFlexHandler([]Value{elem, notFlex}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected a FlexList") {
		t.Fatalf("unshiftFlex: expected FlexList receiver error, got %v", err)
	}
	if _, err := shiftFlexHandler([]Value{notFlex}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected a FlexList") {
		t.Fatalf("shiftFlex: expected FlexList receiver error, got %v", err)
	}
}

func TestB3FlexAdoptError(t *testing.T) {
	r := seam5Reg(t)
	flexRecv := NewFlexList([]Value{NewInteger(1)})
	// A list containing an empty list: AdoptIntoFlex recurses into the empty
	// inner list, whose FlexDeepCopy fails.
	badElem := NewList([]Value{NewList(nil)})
	if _, err := pushFlexHandler([]Value{badElem, flexRecv}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "push") {
		t.Fatalf("pushFlex: expected AdoptIntoFlex error, got %v", err)
	}
	if _, err := unshiftFlexHandler([]Value{badElem, NewFlexList([]Value{NewInteger(1)})}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "unshift") {
		t.Fatalf("unshiftFlex: expected AdoptIntoFlex error, got %v", err)
	}
}

func TestB3BooleanNonConcrete(t *testing.T) {
	// anyHandler: non-concrete -> false (OR identity).
	out, err := anyHandler([]Value{NewTypeLiteral(TList)}, nil, nil, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("any: unexpected %v / %v", out, err)
	}
	if b, _ := AsBoolean(out[0]); b {
		t.Fatalf("any(non-concrete) must be false, got %v", out[0])
	}
	// allHandler: non-concrete -> true (AND identity).
	out2, err := allHandler([]Value{NewTypeLiteral(TList)}, nil, nil, nil)
	if err != nil || len(out2) != 1 {
		t.Fatalf("all: unexpected %v / %v", out2, err)
	}
	if b, _ := AsBoolean(out2[0]); !b {
		t.Fatalf("all(non-concrete) must be true, got %v", out2[0])
	}
	// Positive pair: concrete lists reduce as expected.
	pos, _ := anyHandler([]Value{NewList([]Value{NewBoolean(false), NewBoolean(true)})}, nil, nil, nil)
	if b, _ := AsBoolean(pos[0]); !b {
		t.Fatalf("any([false true]) must be true")
	}
}

func TestB3OperandJoinArity(t *testing.T) {
	// operandJoinReturns with != 2 args -> Any carrier fallback.
	got := operandJoinReturns([]Value{NewInteger(1)}, nil)
	if len(got) != 1 || !got[0].Parent().ConformsTo(TAny) {
		t.Fatalf("operandJoin arity!=2 must yield an Any carrier, got %v", got)
	}
	// Positive pair: two operands join their carriers.
	got2 := operandJoinReturns([]Value{NewInteger(1), NewString("x")}, nil)
	if len(got2) != 1 {
		t.Fatalf("operandJoin(2) must yield one carrier, got %v", got2)
	}
}
