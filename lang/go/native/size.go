package native

import (
	core "github.com/boru-lang/boru/core/go"
)

// The "size" word is registered via the consolidated Natives slice in
// natives.go. It reports the natural size of any value through the
// kernel's Sizer behaviour — see core.SizeOf for the per-type rules.
func sizeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewInteger(int64(SizeOf(args[0])))}, nil
}

// sizeReturns folds `size` over a CONCRETE list or map to its statically-known
// element count — the same value SizeOf computes at run time, but available to
// the checker so a downstream `for (xs size) [...]` sees a concrete count
// (enabling zero-count loop pruning when the collection is statically empty —
// module-test:38's `for (subs size)` over `subs: []`). A non-concrete receiver,
// or a non-list/map shape whose size depends on runtime content, keeps the
// declared Integer carrier so nothing is baked unsoundly. Strings keep the
// carrier: their `size` is the rune count, faithful to fold, but the loop-prune
// use only needs containers, and a static string fold has no consumer here.
func sizeReturns(args []Value, _ *Registry) []Value {
	carrier := []Value{NewCarrier(TInteger)}
	if len(args) != 1 {
		return carrier
	}
	v := args[0]
	if !IsConcrete(&v) {
		return carrier
	}
	// The fold below assumes SizeOf answers with the PHYSICAL element
	// count — true only when the Sizer that owns the dispatch is the
	// kernel family node's. A user type in the chain (a refinement with
	// `behave size` installed) answers with its own body, so folding
	// the container length would make the checker model a size the
	// runtime never reports (and prune loops that actually run).
	if owner := core.SizeOwner(ParentOf(&v)); owner != nil && OriginOf(owner) != core.OriginBuiltin {
		return carrier
	}
	switch {
	case ParentOf(&v).ConformsTo(TList):
		if lst, err := AsList(v); err == nil && !lst.IsNil() {
			return []Value{NewInteger(int64(lst.Len()))}
		}
	case ParentOf(&v).ConformsTo(TMap):
		if m, err := AsMap(v); err == nil && m != nil {
			return []Value{NewInteger(int64(m.Len()))}
		}
	}
	return carrier
}
