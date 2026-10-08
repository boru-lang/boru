package native

import (
	"fmt"
	"strings"
)

// This file holds the surface words — `surface` (declare a pure
// operation contract) and `exposes` (declare + check that a type
// provides it). See design/legacy/SURFACES.10.ignore.
//
//	def Shape surface {area: fnsig [[Self] [Float]]}
//	Circle exposes Shape          # loud completeness check, then membership
//
// A surface carries NO bodies and NO state: each schema entry maps an
// operation name to an fnsig shape, with `Self` marking the positions
// the conforming type occupies. Conformance is EXPLICIT (no structural
// duck typing): `exposes` checks the word's overload table with
// Self := candidate substituted (contravariant params, covariant
// returns — core.FnSigSatisfiesSpec), then records the candidate in the
// surface's conformance set. Membership (sig dispatch, `is`, the type
// algebra) is a parent-chain probe of that set via eng.surfaceUnifier,
// so subclass instances of an exposer conform.

// surfaceHandler implements `surface {schema}` — build an anonymous
// surface type from a map of operation name → fnsig shape. `def Name
// surface {…}` then mints it under Ideal/Surface (InstallType).
func surfaceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[0], "surface")
	if err != nil {
		return nil, r.BoruError("surface_error",
			fmt.Sprintf("surface: argument must be a concrete map of operation shapes, got %s", args[0].String()), "surface")
	}
	if len(m.Keys()) == 0 {
		return nil, r.BoruError("surface_error",
			"surface: schema must declare at least one operation", "surface")
	}
	required := NewOrderedMap()
	for _, key := range m.Keys() {
		v, _ := m.Get(key)
		v = ResolveWordValue(v)
		if !v.Parent.Equal(TFnUndef) {
			return nil, r.BoruError("surface_error",
				fmt.Sprintf("surface: operation %q must be an fnsig shape (e.g. {%s: fnsig [[Self] [Float]]}), got %s",
					key, key, v.String()), "surface")
		}
		required.Set(key, v)
	}
	info := &SurfaceInfo{Required: required, Conform: map[string]bool{}}
	return []Value{NewSurfaceType(TSurface, info)}, nil
}

// exposesHandler implements `<Type> exposes <Surface>` — the explicit
// conformance declaration. For every required operation it asks the
// word's overload table whether some signature satisfies the shape
// with Self := candidate (core.FnDefHasSig). All-or-nothing and loud:
// any unsatisfied operation raises surface_unsatisfied listing every
// gap with its expected substituted shape. Success registers the
// candidate's lattice node in the conformance set. Idempotent.
func exposesHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	surfVal := ResolveWordValue(args[0])
	candVal := ResolveWordValue(args[1])

	// A NAMED surface evaluates to its node (the Stage 2 flip); the
	// shared payload is the node's recorded content.
	if body, ok := TypeContentOf(surfVal); ok && IsBareTypeNode(surfVal) {
		surfVal = body
	}
	sinfo, err := AsSurfaceType(surfVal)
	if err != nil {
		return nil, r.BoruError("exposes_error",
			fmt.Sprintf("exposes: right side must be a surface type, got %s", surfVal.String()), "exposes")
	}
	node, boruErr := exposerNode(r, candVal)
	if boruErr != nil {
		return nil, boruErr
	}
	if sinfo.Conform[node.ID] {
		return nil, nil
	}

	var gaps []string
	for _, op := range sinfo.Required.Keys() {
		shapeVal, _ := sinfo.Required.Get(op)
		undef, ok := shapeVal.Data.(FnUndefInfo)
		if !ok {
			gaps = append(gaps, fmt.Sprintf("%s: required shape is not statically known (declare it with fnsig)", op))
			continue
		}
		fn := r.Lookup(op)
		for _, spec := range undef.Sigs {
			want := SubstituteSelf(spec, node)
			if fn == nil {
				gaps = append(gaps, fmt.Sprintf("%s: no such word — needs %s", op, renderSpec(want)))
				continue
			}
			if !FnDefHasSig(*fn, want) {
				gaps = append(gaps, fmt.Sprintf("%s: no signature satisfies %s", op, renderSpec(want)))
			}
		}
	}
	if len(gaps) > 0 {
		return nil, r.BoruError("surface_unsatisfied",
			fmt.Sprintf("exposes: %s does not expose %s:\n  %s",
				node.Name(), sinfo.Name, strings.Join(gaps, "\n  ")), "exposes")
	}
	sinfo.Conform[node.ID] = true
	return nil, nil
}

// exposerNode resolves the candidate type value to its canonical
// lattice node — the identity stored in the conformance set and walked
// by surfaceUnifier.Match. Classes carry their node in the payload;
// bare type literals (builtins, refine newtypes) ARE their node.
func exposerNode(r *Registry, v Value) (*Type, error) {
	switch {
	case IsClassType(v):
		info, _ := AsClassType(v)
		if info.Type != nil {
			return info.Type, nil
		}
	case IsBareTypeNode(v):
		if canon := CanonicalType(r, &v); canon != nil {
			return canon, nil
		}
	}
	return nil, r.BoruError("exposes_error",
		fmt.Sprintf("exposes: left side must be a named type (a class or refined type), got %s", v.String()), "exposes")
}

// renderSpec renders a substituted FnSigSpec for the
// surface_unsatisfied gap listing: fn [[Circle] [Float]].
func renderSpec(spec FnSigSpec) string {
	var params, returns []string
	for _, p := range spec.Params {
		if p.Type != nil {
			params = append(params, p.Type.Name())
		}
	}
	for _, t := range spec.Returns {
		if t != nil {
			returns = append(returns, t.Name())
		}
	}
	return fmt.Sprintf("fn [[%s] [%s]]", strings.Join(params, " "), strings.Join(returns, " "))
}
