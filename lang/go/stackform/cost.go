package stackform

// Cost returns a description-length metric for a StackForm — used
// by the PBT reducer to drive failure-preserving shrinking.
//
// The cost model is a flat per-Op weight today. The PBT plan's
// Stage 4 will layer a word-transparency policy on top (frozen vs
// transparent words get different weights, see
// design/legacy/boru_property_based_reduction_report.10.ignore §8-9), but those
// adjustments live in the lang-layer shrink package — kernel-level
// stackform stays cost-policy-neutral.
//
// Current weights:
//
//	PushLit  → 1 + complexity of the literal
//	Call     → 2 (covers the name + arity overhead)
//	Quote    → 1 + cost of the body
//	DoEval   → 1
//
// The "complexity of the literal" is currently a constant 0 for
// scalars and the length of contained values for lists / maps —
// a placeholder until the PBT shrinker needs something more
// sophisticated.
func Cost(form *StackForm) int {
	if form == nil {
		return 0
	}
	c := 0
	for _, op := range form.Ops {
		switch o := op.(type) {
		case PushLit:
			c += 1 + litComplexity(o.V)
		case Call:
			c += 2
		case Quote:
			c += 1 + Cost(o.Body)
		case DoEval:
			c++
		}
	}
	return c
}

// litComplexity is a placeholder. Scalars cost 0 beyond the PushLit
// itself; structural values cost their child count. The PBT
// shrinker may swap this out for a more nuanced metric.
func litComplexity(_ interface{}) int {
	// Intentionally simple for v1. The shrinker is the natural place
	// to tune this — extend via the policy layer there.
	return 0
}
