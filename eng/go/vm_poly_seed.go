package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// polySeedWindow tests a poly site's seeded pick (PolyRef.Seed — the
// checker's own overload, compiler/go/poly_seed.go) against the runtime
// operands and returns the argument window (sig order) when it holds, nil
// otherwise. Every operand must be tag-keyable (concrete, unascribed, no
// carrier); then either its tag equals the one the pick was proven for
// (SeedTags) or, for a word with one overload of this arity, that overload
// matches the window positionally. The caller re-matches when it returns
// nil, so a failed guard costs only the test.
func polySeedWindow(pr *compiler.PolyRef, stack []core.Value) []core.Value {
	n := pr.Arity
	if pr.Seed == nil || len(stack) < n || (pr.SeedTags != nil && len(pr.SeedTags) != n) {
		return nil
	}
	for i := 0; i < n; i++ {
		v := stack[len(stack)-1-i]
		if !core.TagKeyable(v) || (pr.SeedTags != nil && v.Parent != pr.SeedTags[i]) {
			return nil
		}
	}
	window := make([]core.Value, n)
	for i := 0; i < n; i++ {
		window[i] = stack[len(stack)-1-i]
	}
	if pr.SeedTags == nil {
		if _, ok := core.FlexibleMatch(window, pr.Seed); !ok {
			return nil
		}
	}
	return window
}
