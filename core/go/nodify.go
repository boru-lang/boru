package core

import "errors"

// Nodifier is an optional capability interface. Types implementing
// it expose a "project to a Node or Scalar" transformation — used by
// the `nodify` word to map a value into boru's data subset (Integer,
// String, Boolean, Atom, List, Map, …) without going through a
// serialised JSON string. The result is a Value, not a []byte;
// callers that need a JSON-encoded string compose this with a
// downstream encoder (the `jsonify` word does exactly that).
//
// Conventions mirror Comparer: a wrapper Behavior that holds the
// capability slot but no body returns ErrNoNodifier so the
// NodifyValue walk continues the parent chain instead of treating
// the wrapper as the final Nodifier.
type Nodifier interface {
	Nodify(v Value) (Value, error)
}

// ErrNoNodifier signals that a Nodifier-shaped Behavior wrapper has
// no body installed for this capability — NodifyValue keeps walking.
var ErrNoNodifier = errors.New("eng: no nodifier in this Behavior")

// NodifyValue projects v into a Node or Scalar by walking v.Parent's
// parent chain looking for a Nodifier. If none is registered, v is
// returned unchanged — the natural default for values already in
// data-shape (Integer, String, Map, List, …).
//
// No auto-recursion: the result Value is whatever the registered body
// produces. Bodies that need to project nested Object fields call
// `nodify` again inside themselves.
func NodifyValue(v Value) (Value, error) {
	if v.Parent() == nil {
		return v, nil
	}
	for t := v.Parent(); t != nil; t = t.Parent() {
		n, ok := t.Behavior().(Nodifier)
		if !ok {
			continue
		}
		out, err := n.Nodify(v)
		if errors.Is(err, ErrNoNodifier) {
			continue
		}
		return out, err
	}
	return v, nil
}
