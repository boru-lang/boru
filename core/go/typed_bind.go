package core

import "fmt"

// RunTypedBind executes one OpBindTyped: the runtime validate/reparent step of
// a typed value-def (`def x:Pos n`) whose constraint is a refinement and whose
// body was DYNAMIC at compile time. It is the compiled mirror of the
// interpreter's defTypedHandler refinement branches (lang/go/native/
// native_definition.go) — the SAME helpers (RunPredicate / Unify /
// ReparentValue), the SAME error strings, in the SAME order — so an error
// raised here is byte-identical to the interpreter's (a plain, position-less
// error: defTypedHandler raises via fmt.Errorf, and the interpreter surfaces
// it unstamped — stampErrPos only positions BoruErrors).
//
// Returns the value the binding installs: reparented to the refine newtype /
// named predicate type where the interpreter reparents (so a downstream
// `typeof` renders the refined type and sig dispatch keys off it), the
// predicate's transformed output for a value-transforming predicate body, or
// the value unchanged (base tag kept) for a DepScalar subset.
func RunTypedBind(r *Registry, spec *TypedBindSpec, v Value) (Value, error) {
	switch spec.Kind {
	case TypedBindPredicate:
		if spec.Cons == nil {
			return Value{}, fmt.Errorf("bytecode: internal: typed bind %s has no predicate constraint", spec.Name)
		}
		// Mirrors defTypedHandler's predicate branch: run the predicate fn over
		// the runtime value (the check pass short-circuits RunPredicate on
		// carriers, so this is the FIRST real evaluation), then reparent iff the
		// record-time decision matched the interpreter's (a named, non-builtin
		// predicate type with a concrete declared input — spec.Def non-nil).
		out, matched, err := r.RunPredicate(*spec.Cons, v)
		if err != nil {
			return Value{}, fmt.Errorf("def %s: predicate type %s: %w", spec.Name, spec.Describe, err)
		}
		if !matched {
			return Value{}, fmt.Errorf("def %s: value %s does not satisfy predicate type %s",
				spec.Name, v.String(), spec.Describe)
		}
		if spec.Def != nil {
			out = ReparentValue(out, CanonicalType(r, spec.Def))
		}
		return out, nil
	case TypedBindRefine:
		// Mirrors defTypedHandler's bare-refine branch: unify against the
		// nearest BUILTIN ancestor (walking past intervening user refines), then
		// reparent a copy of the value to the newtype. CanonicalType keeps the
		// *Type live for module-sub-registry mints (the main TypeTable misses
		// their IDs and the carried pointer IS the canonical node).
		def := CanonicalType(r, spec.Def)
		if def == nil {
			return Value{}, fmt.Errorf("bytecode: internal: typed bind %s has no refine target", spec.Name)
		}
		root := def.Parent
		for root != nil && root.Origin == OriginUserDef {
			root = root.Parent
		}
		if root == nil {
			return Value{}, fmt.Errorf("def %s: refine subtype %s has no builtin ancestor",
				spec.Name, spec.Describe)
		}
		if _, ok := Unify(v, NewTypeLiteral(root)); !ok {
			return Value{}, r.BoruError("type_error",
				fmt.Sprintf("def %s: value %s does not unify with declared type %s",
					spec.Name, v.String(), spec.Describe), spec.Name)
		}
		return ReparentValue(v, def), nil
	case TypedBindDepScalar:
		if spec.Cons == nil {
			return Value{}, fmt.Errorf("bytecode: internal: typed bind %s has no DepScalar constraint", spec.Name)
		}
		// Mirrors defTypedHandler's Unify tail for a DepScalar constraint: the
		// self-contained predicate (unifyDepScalar / depScalarCheck, no registry)
		// admits or declines the runtime value; the binding is Unify's result (the
		// value with its base tag — DepScalar subsets never reparent).
		unified, ok := Unify(v, *spec.Cons)
		if !ok {
			return Value{}, r.BoruError("type_error",
				fmt.Sprintf("def %s: value %s does not unify with declared type %s",
					spec.Name, v.String(), spec.Describe), spec.Name)
		}
		return unified, nil
	}
	return Value{}, fmt.Errorf("bytecode: internal: typed bind %s has invalid kind %d", spec.Name, spec.Kind)
}
