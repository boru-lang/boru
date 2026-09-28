package core

import "testing"

// TestMarkShapeModel pins the check-mode instance MODEL's mark (NUR331): a
// ReturnsFn that surfaces a shape-only instance marks its methods' home
// registry, and a method homed there is no constant — neither standalone
// nor as a container member — while the same method homed in an unmarked
// registry still bakes as data. A member without a home and a non-fn member
// are left alone.
func TestMarkShapeModel(t *testing.T) {
	model, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	live, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	wrapper := func(home *Registry) Value {
		return NewFunction(FnDefInfo{Name: "rand-int", Registry: home,
			Signatures: []Signature{{Impl: &BoruImpl{Body: []Value{NewWord("rand-int")}}}}})
	}
	inst := NewOrderedMap()
	inst.Set("int", wrapper(model))
	inst.Set("native", NewFunction(FnDefInfo{Name: "add"})) // no home: nothing to mark
	inst.Set("n", NewInteger(1))                            // data: nothing to mark
	MarkShapeModel(inst)
	if !model.ShapeModel {
		t.Fatal("the model's home registry must be marked")
	}

	modelFn := wrapper(model)
	liveFn := wrapper(live)
	fd, _ := modelFn.Data.(FnDefInfo)
	if !fd.ShapeModelHomed() {
		t.Error("a method homed in the model registry is ShapeModelHomed")
	}
	lfd, _ := liveFn.Data.(FnDefInfo)
	if lfd.ShapeModelHomed() {
		t.Error("a method homed in a live registry is not ShapeModelHomed")
	}
	nat, _ := inst.Get("native")
	nfd, _ := nat.Data.(FnDefInfo)
	if nfd.ShapeModelHomed() {
		t.Error("a Go-built value (no home) is never ShapeModelHomed")
	}

	if IsInertConst(modelFn) || IsInertConstMember(modelFn) {
		t.Error("a model's method must not bake as a constant")
	}
	if !IsInertConst(liveFn) || !IsInertConstMember(liveFn) {
		t.Error("a live module method still bakes as data")
	}
	if IsInertConst(NewMap(inst)) {
		t.Error("a map holding a model's method must not bake as a constant")
	}
	if !IsInertConst(NewList([]Value{liveFn})) {
		t.Error("a list holding a live method still bakes")
	}
}
