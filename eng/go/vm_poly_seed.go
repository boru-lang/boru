package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// seeded returns the live overload a poly site's seeded pick (PolyRef.Seed —
// the checker's own overload, compiler/go/poly_seed.go) stands for, when the
// runtime window passes the seed's guard, or nil. Every operand must be
// tag-keyable (concrete, unascribed, no carrier); then either its tag equals
// the one the pick was proven for (SeedTags) or, for a word with one overload
// of this arity, that overload matches the window positionally.
//
// The seed is a signature of the aggregate the CHECK pass saw; the call
// dispatches from the LIVE one. So the seed stands only for the live
// overload with the same implementation (Signature.Impl — the identity the
// user-poly drift check reads), and only while that overload is what the
// live aggregate's first match picks: checked once per live aggregate (a
// redefinition or a new overload is a new aggregate) by a full match over a
// window that passes the guard. Until a window has validated it, and after a
// validation fails, the caller re-matches as before.
func (e *polyCacheEntry) seeded(pr *compiler.PolyRef, fn *core.FnDefInfo, sigs []core.Signature, window []core.Value) *core.Signature {
	if pr.Seed == nil || fn == nil || !seedGuardHolds(pr, window) {
		return nil
	}
	if e.seedFn == fn {
		if e.seedSig != nil && (pr.SeedTags != nil || seedMatches(window, e.seedSig)) {
			return e.seedSig
		}
		return nil
	}
	e.seedFn, e.seedSig = fn, nil
	var live *core.Signature
	same := 0
	for i := range sigs {
		if sigs[i].TotalArgs() != len(window) {
			continue
		}
		same++
		if sigs[i].Impl == pr.Seed.Impl {
			live = &sigs[i]
		}
	}
	if live == nil || !core.TagDeterminedSigs(sigs, len(window)) || (pr.SeedTags == nil && same != 1) {
		return nil
	}
	if mr := core.MatchSignature(sigs, window, core.WordInfo{ArgCount: len(window)}); mr == nil || mr.Sig != live {
		return nil
	}
	e.seedSig = live
	return live
}

// seedGuardHolds is the seed's tag guard over the window (sig order).
func seedGuardHolds(pr *compiler.PolyRef, window []core.Value) bool {
	if pr.SeedTags != nil && len(pr.SeedTags) != len(window) {
		return false
	}
	for i, v := range window {
		if !core.TagKeyable(v) || (pr.SeedTags != nil && v.Parent != pr.SeedTags[i]) {
			return false
		}
	}
	return true
}

// seedMatches is the tag-free guard's positional match of the word's only
// overload of the arity.
func seedMatches(window []core.Value, sig *core.Signature) bool {
	_, ok := core.FlexibleMatch(window, sig)
	return ok
}
