package core

import "testing"

// W9 unify.go coverage: unifyObjectType's nominal arms — operand swap, the
// no-minted-node reject, same/distinct class-type unification, and the
// bare-node conform / reject branches.

func w9ClassType(id string) Value {
	return NewClassType(TClass, ClassTypeInfo{ID: id, Name: "Class/" + id, Fields: NewOrderedMap()})
}

func TestW9UnifyObjectTypeSwapAndNoNode(t *testing.T) {
	classA := w9ClassType("T_a")

	// a is NOT a class type → the operands swap so the class side is `ot`.
	if _, err := unifyObjectType(NewInteger(1), classA, nil); err == nil {
		t.Error("an integer does not inhabit the object type")
	}

	// A class-type value whose minted node is nil → reject with the declare hint.
	noNode := NewValueRaw(TClass, ClassTypeInfo{Name: "Class/none", Fields: NewOrderedMap()})
	if _, err := unifyObjectType(noNode, NewInteger(1), nil); err == nil {
		t.Error("a class type with no minted node must reject")
	}
}

func TestW9UnifyObjectTypeSameAndDistinct(t *testing.T) {
	classA := w9ClassType("T_same")
	classAcopy := w9ClassType("T_same")
	classB := w9ClassType("T_other")

	// Same ID → unify to the class-type side.
	if _, err := unifyObjectType(classA, classAcopy, nil); err != nil {
		t.Errorf("identical class types should unify: %v", err)
	}
	// Distinct IDs → reject.
	if _, err := unifyObjectType(classA, classB, nil); err == nil {
		t.Error("distinct object types must not unify")
	}
}

func TestW9UnifyObjectTypeBareNode(t *testing.T) {
	classA := w9ClassType("T_bn")

	// A bare node conforming to the class node (TClass literal) → unify.
	if _, err := unifyObjectType(classA, NewTypeLiteral(TClass), nil); err != nil {
		t.Errorf("a conforming bare node should unify: %v", err)
	}
	// A bare node that does NOT conform (Integer literal) → reject.
	if _, err := unifyObjectType(classA, NewTypeLiteral(TInteger), nil); err == nil {
		t.Error("a non-conforming bare node must reject")
	}
}
