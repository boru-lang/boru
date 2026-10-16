package core

import "errors"

// DeepEqualer is an optional capability interface for `deq`. A type
// implementing it decides what STRUCTURAL equality means for its own
// values.
//
// The kernel already routes `eq` through TypeBehavior.Equal — but only
// for same-Parent pairs, and in the deq path only inside the scalar
// families (scalarSemanticEqual). DeepEqual's own cascade names List,
// Map, Xml, flat instances, and the opaque Ideal handles one by one,
// and everything it does not name reaches a terminal `false`. So a
// type outside those families has no way to say two of its values are
// deeply equal — not from Go, and not from boru.
//
// This capability is consulted at exactly that terminal point, which
// makes it strictly additive: it can only turn a `false` into a real
// answer, never change one the cascade already gave. A type whose
// values are structurally reachable by an earlier arm (a refine over
// Map, say) is compared by that arm, as before.
//
// Return ErrNoDeepEqualer to decline and let the walk continue, exactly
// as Comparer does with ErrNoComparer.
type DeepEqualer interface {
	DeepEqualValues(a, b Value) (bool, error)
}

// ErrNoDeepEqualer signals that a DeepEqualer-shaped Behavior has no
// body for this pair, so the parent-chain walk continues.
var ErrNoDeepEqualer = errors.New("no deep equaler")

// DeepEqualerOwner returns the nearest ancestor of t (t included) whose
// Behavior implements DeepEqualer, or nil. It is the single-value
// counterpart of the pairwise walk below, used by the deq-key
// CLASSIFIER: a value whose chain carries a DeepEqualer is pairwise
// comparable, so DeqKey must not class it never-equal — otherwise
// DeepEqual and the DeqIndex behind `unique` / `member` / indices
// disagree about the same pair (the capability's answer would be
// unreachable through the index).
func DeepEqualerOwner(t *Type) *Type {
	for ; t != nil; t = ParentOf(t) {
		if _, ok := t.Behavior().(DeepEqualer); ok {
			return t
		}
	}
	return nil
}

// deepEqualCapability walks the two values' lowest common ancestor up
// the parent chain looking for a DeepEqualer — the same shape the
// Comparer walk uses, so a type installs equality and ordering the same
// way. ok=false means nothing claimed the pair.
//
// A body that ERRORS is treated as a decline, not as inequality:
// DeepEqual is total and has no error channel, and reporting "not
// equal" because a body failed would be a wrong answer rather than a
// missing one.
func deepEqualCapability(a, b Value) (result bool, ok bool) {
	for t := lowestCommonAncestor(ValueType(a), ValueType(b)); t != nil; t = ParentOf(t) {
		de, is := t.Behavior().(DeepEqualer)
		if !is {
			continue
		}
		res, err := de.DeepEqualValues(a, b)
		if err != nil {
			continue
		}
		return res, true
	}
	return false, false
}
