package shrink

import (
	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/stackform"
)

// ShrinkCost computes the total cost of a StackForm under the given
// Policy. Layers two adjustments on top of stackform.Cost:
//
//  1. PushLit cost includes the literal's "complexity" — digit count
//     for integers, length for strings, list length for lists. This
//     makes PushLit(0) genuinely cheaper than PushLit(42) so the
//     reducer accepts integer-shrinking; PushLit("") cheaper than
//     PushLit("hello world") for string shrinking; etc.
//
//  2. Call cost includes a per-class adjustment from Policy.Weight —
//     Frozen calls price high to discourage rewriting; Transparent
//     calls price low so dropping them is the obvious win. See
//     policy.go for the calibration.
//
// Quote bodies recursively apply ShrinkCost so the policy reaches
// into nested forms.
func ShrinkCost(form *stackform.StackForm, policy *Policy) int {
	if form == nil {
		return 0
	}
	c := 0
	for _, op := range form.Ops {
		switch o := op.(type) {
		case stackform.PushLit:
			c += 1 + literalComplexity(o.V)
		case stackform.Call:
			c += 2
			c += policy.Weight(policy.Classify(o.Name))
		case stackform.Quote:
			c += 1 + ShrinkCost(o.Body, policy)
		case stackform.DoEval:
			c++
		}
	}
	return c
}

// literalComplexity is a description-length proxy for a single
// literal value. Used by ShrinkCost so cheaper-shaped literals lower
// the overall cost, making the reducer accept literal-shrinking
// mutations even though the Op count stays the same.
//
// Calibration (modest scaling so literal complexity doesn't dominate
// Op-count cost in long programs):
//
//   - Integer: bit-length of |n|, +1 for negative sign.
//     0→0, 1→1, 2→1, 8→4, 1024→10. Halving reliably reduces cost.
//   - Float: same as integer (uses the integer-cast magnitude).
//   - String: byte length. "" → 0, "hello" → 5.
//   - Boolean: 1 for true, 0 for false. Lets the reducer accept
//     true→false even though both PushLits have the same Op base.
//   - List: element count + recursive complexity of each element.
//   - Other (atom, map, paths, …): 1 placeholder.
func literalComplexity(v core.Value) int {
	if core.ParentOf(&v) == nil {
		return 0
	}
	switch {
	case core.ParentOf(&v).ConformsTo(core.TInteger):
		if n, err := core.AsInteger(v); err == nil {
			return intMagnitude(n)
		}
	case core.ParentOf(&v).ConformsTo(core.TFloat):
		if f, err := core.AsFloat(v); err == nil {
			return intMagnitude(int64(f))
		}
	case core.ParentOf(&v).ConformsTo(core.TString):
		if s, err := core.AsString(v); err == nil {
			return len(s)
		}
	case core.ParentOf(&v).ConformsTo(core.TBoolean):
		if b, err := core.AsBoolean(v); err == nil {
			if b {
				return 1
			}
		}
		return 0
	case core.ParentOf(&v).ConformsTo(core.TList):
		if lst, err := core.RequireConcreteList(v, "literalComplexity"); err == nil {
			c := lst.Len()
			for i := 0; i < lst.Len(); i++ {
				c += literalComplexity(lst.Get(i))
			}
			return c
		}
	}
	return 1
}

// intMagnitude is the description-length proxy for an integer:
// |n| itself (capped at maxIntCost so pathological inputs don't
// overflow), plus 1 if negative. Linear scaling means every n →
// n-1 step lowers cost — the shrinker's n-1 candidate reliably
// converges on the exact minimum violator.
//
// Linear over log-based: a log model (bit-length) groups 8..15
// into the same cost bucket and the reducer stalls inside the
// bucket; linear makes every value distinguishable.
func intMagnitude(n int64) int {
	const maxIntCost = 1 << 20 // 1M — sanity cap, well below int max
	negAdj := 0
	if n < 0 {
		negAdj = 1
		n = -n
	}
	if n > maxIntCost {
		n = maxIntCost
	}
	return int(n) + negAdj
}
