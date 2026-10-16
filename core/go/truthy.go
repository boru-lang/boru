package core

import "errors"

// Truther is an optional capability interface. A type implementing it
// decides what its own values mean in a BOOLEAN POSITION — `if`, the
// boolean connectives, loop conditions, `make Boolean`, every caller of
// CoerceBoolean.
//
// Without it, truthiness is the kernel's family cascade: booleans by
// value, numbers by magnitude, None false, containers by emptiness,
// strings by emptiness — and, for everything the cascade does not name,
// a fallback that RENDERS the value and calls it falsy only when the
// text is empty or "false". That fallback is a guess, and it is the
// asymmetry this capability closes: `Comparer` lets a type say how it
// orders, `Sizer` how it sizes, `Hasher` how it hashes, `Unifier` how it
// unifies — but until now nothing let it say when it is empty. A Time, a
// Matrix, a Fetch response, a user-defined Ideal: all simply truthy,
// whatever they hold.
//
// Return ErrNoTruther to decline and let the walk continue, exactly as
// Comparer does with ErrNoComparer.
type Truther interface {
	Truthy(v Value) (bool, error)
}

// ErrNoTruther signals that a Truther-shaped Behavior has no truthiness
// body for this value, so the parent-chain walk continues.
var ErrNoTruther = errors.New("no truther")

// truthinessOf walks v's parent chain for a Truther, nearest type first,
// so a descendant inherits its branch's rule. ok=false means no type in
// the chain claimed the value and the caller falls back to the kernel's
// family cascade.
//
// The walk runs BEFORE the cascade, so a type that opts in owns its own
// answer rather than having the cascade's guess imposed on it. No kernel
// Behavior implements Truther, so nothing built in changes.
//
// A body that ERRORS falls through to the cascade rather than forcing a
// verdict. CoerceBoolean is total and has no error channel — `if` cannot
// report one — so the choice is between a wrong answer and the built-in
// reading, and the built-in reading is the safer of the two.
func truthinessOf(v Value) (result bool, ok bool) {
	for t := ValueType(v); t != nil; t = t.Parent() {
		tr, is := t.Behavior().(Truther)
		if !is {
			continue
		}
		res, err := tr.Truthy(v)
		if err != nil {
			continue
		}
		return res, true
	}
	return false, false
}
