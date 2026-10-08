package core

import "errors"

// ExactEqualer is the optional capability interface for `eq` — the REFERENCE
// half of the two equalities — mirroring DeepEqualer for `deq` (NUR075). A
// type implementing it decides what "the same thing" means for its own
// values, as a DeepEqualer decides what "the same value" means.
//
// It is consulted at exactly the point DeepEqualer is: ExactEqual's terminal
// `false`, after every kernel arm, so it is strictly additive — it can only
// turn that `false` into a real answer, never override an arm above (a
// scalar leaf, a container's identity, a function's, a handle's). The two
// capabilities therefore reach the same values: the ones no kernel arm
// names, such as a host payload with no pointer to carry an identity. Before
// it, a type could extend `deq` and not `eq`, and the two halves of one word
// family were extensible on different terms.
//
// Return ErrNoExactEqualer to decline and let the walk continue, exactly as
// DeepEqualer does with ErrNoDeepEqualer.
type ExactEqualer interface {
	ExactEqualValues(a, b Value) (bool, error)
}

// ErrNoExactEqualer signals that an ExactEqualer-shaped Behavior has no body
// for this pair, so the parent-chain walk continues.
var ErrNoExactEqualer = errors.New("no exact equaler")

// exactEqualCapability walks the two values' lowest common ancestor up the
// parent chain looking for an ExactEqualer — deepEqualCapability's walk, so a
// type installs both equalities the same way. ok=false means nothing claimed
// the pair. A body that ERRORS is a decline, not inequality: ExactEqual is
// total and has no error channel, the rule deepEqualCapability states.
func exactEqualCapability(a, b Value) (result bool, ok bool) {
	for t := lowestCommonAncestor(ValueType(a), ValueType(b)); t != nil; t = t.Parent {
		ee, is := t.Behavior().(ExactEqualer)
		if !is {
			continue
		}
		res, err := ee.ExactEqualValues(a, b)
		if err != nil {
			continue
		}
		return res, true
	}
	return false, false
}
