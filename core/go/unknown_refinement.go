package core

// Refinements over a bound the analysis pass does not know (NUR308).
//
// A type whose content holds a refinement over a computed bound (`def T
// (Integer gte (size s))`, `def T ((Integer gt n) tor String)`) has a bound
// only the run knows: the check pass mints T over its PLACEHOLDER for the
// bound, the carrier, and cannot decide membership against it.

// HasUnknownRefinement reports whether a type's content holds a refinement
// whose bound the analysis pass does not know (NUR308) — a refinement, a
// union or negation holding one, reached through a named node's recorded
// body, or a typed container's child. Such a type's membership is the run's
// to decide, so the pass admits every value at it.
func HasUnknownRefinement(v Value) bool {
	return hasUnknownRefinement(v, 0)
}

func hasUnknownRefinement(v Value, depth int) bool {
	if depth > 32 {
		return false
	}
	if IsBareTypeNode(&v) {
		if body, ok := v.TypeBody(); ok {
			return hasUnknownRefinement(body, depth+1)
		}
		return false
	}
	switch d := v.Data().(type) {
	case DepScalarInfo:
		return !depBoundConst(d.Lo) || !depBoundConst(d.Hi)
	case DisjunctInfo:
		for _, alt := range d.Alternatives {
			if hasUnknownRefinement(alt, depth+1) {
				return true
			}
		}
	case NegationInfo:
		return hasUnknownRefinement(d.Inner, depth+1)
	case ChildTypeInfo:
		return hasUnknownRefinement(d.Child, depth+1)
	}
	return false
}
