package core

// unifyFnUndefShape handles unification when at least one side is a
// FnUndef (structural function-type constraint). The other side must
// be a function value (FnDef or Function) whose signatures cover the
// FnUndef pattern.
//
// The structural-subtyping rule itself (contravariant params,
// covariant returns, Pattern compatibility) lives in fnsig.go;
// this file only owns the kernel-side dispatch. r is the enclosing
// unify chain's registry, threaded into the rule's Pattern-compatibility
// unify (fnSigSatisfiesSpecR).
func unifyFnUndefShape(a Value, sa ValueShape, b Value, sb ValueShape, r *Registry) (Value, *UnifyError) {
	// Canonicalize: undef on the left, fn on the right.
	var undef, fn Value
	var fnShape ValueShape
	if sa == ShapeFnUndef {
		undef, fn = a, b
		fnShape = sb
	} else {
		undef, fn = b, a
		fnShape = sa
	}
	if fnShape != ShapeFunction {
		return Value{}, unifyFail("FnUndef requires a function value on the other side", a, b)
	}
	if fnUndefMatchesFnDefR(undef, fn, r) {
		return fn, nil
	}
	return Value{}, unifyFail("function signatures do not satisfy FnUndef constraint", a, b)
}
