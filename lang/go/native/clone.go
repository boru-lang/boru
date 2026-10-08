package native

// cloneHandler produces a deep, independent copy of any value via the
// kernel's universal CloneValue. Mutable containers (List, Map, FlexList,
// Array, Store, Object instance, Table, Error payload) are duplicated so
// the clone can diverge without aliasing the source; immutable payloads
// (scalars, times, type bodies, functions) are shared. The original's
// type is preserved — a refine subtype, typed list/map, or object type
// survives the copy — and pointer-backed graphs are cloned cycle-safely.
//
// This replaces the former valueToAny → voxgigstruct.Clone → anyToValue
// round-trip, which only understood the JSON-ish subset (scalars, plain
// lists, plain maps) and silently dropped or mistyped everything else
// (Store, Array, Table, Error, Tensor, refine types, …).
//
// The "clone" word is registered via the consolidated Natives slice in
// natives.go.
func cloneHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{CloneValue(args[0])}, nil
}

// cloneReturnsFn types `clone` as the SAME type as its input — CloneValue
// preserves the type (a refine subtype, typed list/map, object/class type all
// survive the copy), so the result is never the poison dynamic(Any) the bare
// TAny sig would declare. A FRESH carrier of the input's Parent keeps the
// result's emit identity distinct from the source — a clone is a new value
// (`(clone p) eq p` is false) — so operand provenance stays unambiguous.
func cloneReturnsFn(args []Value, _ *Registry) []Value {
	if len(args) == 0 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	// CloneValue preserves a {:T}/[:T] tag at runtime, so the check residual must
	// too (#8, Codex round 8). A plain list/map clone uses the both-fields typed
	// residual (read narrowing + chained-write enforcement); a flex/other kind
	// keeps its exact Parent with the elem pointer (d2RetainElem) so flex-ness and
	// the write-check survive. An untagged source keeps the bare carrier.
	src := args[0]
	if _, ok := src.ElemConstraint(); ok {
		switch {
		case src.Parent.Equal(TList):
			return []Value{d2TypedListResidual(src)}
		case src.Parent.Equal(TMap):
			return []Value{d2TypedMapResidual(src)}
		default:
			return []Value{d2RetainElem(NewCarrier(src.Parent), src)}
		}
	}
	if IsTypeLiteral(src) {
		return []Value{ValueCarrier(src)} // a type VALUE (NUR323)
	}
	return []Value{NewCarrier(src.Parent)}
}
