package core

import "testing"

// fn_shape_test.go pins the thirty-fifth increment's kernel pieces: the
// self-contained Go-impl fn value class (IsSelfContainedGoFnDef — fn-util's
// produced wrappers, applied on their OWN signatures) and the per-pass fn
// shape claim (CheckState.FnShapes — the arity a producing word's check-mode
// ReturnsFn claims for a computed-fn carrier).

// fnShapeGoSig builds the one all-forward Go-impl signature of fn-util's
// produced wrappers (goFnValue's shape) over nParams Any params.
func fnShapeGoSig(nParams int) Signature {
	params := make([]FnParam, nParams)
	for i := range params {
		params[i] = FnParam{Type: TAny}
	}
	sig := Signature{Params: params, BarrierPos: nParams, Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		return []Value{NewInteger(7)}, nil
	})}
	NormalizeSig(&sig)
	return sig
}

func TestIsSelfContainedGoFnDef(t *testing.T) {
	self := FnDefInfo{Name: "const", Anonymous: true, Signatures: []Signature{fnShapeGoSig(1)}}
	if !IsSelfContainedGoFnDef(self) {
		t.Error("an anonymous fn value whose own sigs are all Go handlers is self-contained")
	}
	named := self
	named.Anonymous = false
	if IsSelfContainedGoFnDef(named) {
		t.Error("a non-anonymous fn resolves through its name — not self-contained")
	}
	macro := self
	macro.Macro = true
	if IsSelfContainedGoFnDef(macro) {
		t.Error("a macro is data, applied only by name")
	}
	inner := NewFunction(self)
	wrapper := self
	wrapper.Wrap = WrapRebarrier
	wrapper.Wraps = &inner
	if IsSelfContainedGoFnDef(wrapper) {
		t.Error("a modifier wrapper re-dispatches what it wraps — excluded")
	}
	reversed := self
	reversed.ArgsReversed = true
	if IsSelfContainedGoFnDef(reversed) {
		t.Error("usurp's one-way mark excludes the value")
	}
	boru := FnDefInfo{Anonymous: true, Signatures: []Signature{{Impl: Boru([]Value{NewInteger(1)})}}}
	if IsSelfContainedGoFnDef(boru) {
		t.Error("a boru-bodied lambda needs the interpreter's frame")
	}
	if IsSelfContainedGoFnDef(FnDefInfo{Anonymous: true}) {
		t.Error("a sig-less value has nothing to dispatch on")
	}
}

func TestNoteFnShapeArms(t *testing.T) {
	r := newTestRegistry(t)
	out := NewCarrier(TFunction)
	// Inactive: no claim is recorded.
	r.Check.NoteFnShape(out, 1)
	if r.Check.FnShapes != nil {
		t.Fatal("an inactive check must not record a fn shape")
	}
	done := r.Check.Begin()
	r.Check.NoteFnShape(Value{}, 1) // no identity to key on
	r.Check.NoteFnShape(out, -1)    // a handler that raises instead of building the wrapper
	if r.Check.FnShapes != nil {
		t.Fatal("an unidentified carrier and a negative count are no claim")
	}
	r.Check.NoteFnShape(out, 2)
	if n, ok := r.Check.FnShapeArity(out.ID); !ok || n != 2 {
		t.Errorf("claim = %d/%v, want 2", n, ok)
	}
	zero := NewCarrier(TFunction)
	r.Check.NoteFnShape(zero, 0)
	if n, ok := r.Check.FnShapeArity(zero.ID); !ok || n != 0 {
		t.Errorf("a 0-param wrapper is a claim: %d/%v", n, ok)
	}
	if _, ok := r.Check.FnShapeArity(""); ok {
		t.Error("an empty ID has no claim")
	}
	if _, ok := r.Check.FnShapeArity("nope"); ok {
		t.Error("an unclaimed ID has no claim")
	}
	var none *CheckState
	if _, ok := none.FnShapeArity(out.ID); ok {
		t.Error("a nil check state has no claim")
	}
	// The header clone carries the claims; the next Begin resets them.
	if n, ok := r.Check.Clone().FnShapeArity(out.ID); !ok || n != 2 {
		t.Error("Clone must carry the fn shapes")
	}
	done()
	defer r.Check.Begin()()
	if r.Check.FnShapes != nil {
		t.Error("Begin must reset the per-pass fn shapes")
	}
}
