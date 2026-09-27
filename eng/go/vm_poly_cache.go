package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The poly inline cache. CALL_NATIVE_POLY re-matches a word's overloads on
// every execution because the checker could not commit one — an operand, or
// only the result, was gradual. At run time such a site is almost always
// monomorphic: the generic-dispatch census found every one of 143,772 poly
// executions over the real test suites picking the overload its site picked
// first. So each site remembers the overload it last picked and the tags of
// the window that picked it, and a window with the same tags takes that
// overload without re-matching (no Lookup aggregate walk past the dispatch
// cache, no MatchSignature scan, no result allocation).
//
// Sound by construction: it is used only where the word's overloads are
// TAG-DETERMINED (core.TagDeterminedSigs — no patterns, no type-literal
// slots, builtin tag-matched types) and every operand is tag-keyable
// (core.TagKeyable), where MatchSignature's first match is a function of the
// tags alone; the looked-up aggregate must be the one the entry was filled
// from (a redefinition or a new overload is a new aggregate); and a miss is
// simply the full re-match, which refills the entry. The guard runs before
// the handler, so there is nothing to undo.

// polyCacheEntry is one site's remembered pick.
type polyCacheEntry struct {
	fn       *core.FnDefInfo // the aggregate the entry was filled from
	eligible bool            // core.TagDeterminedSigs over fn's sigs
	sig      *core.Signature // the last pick (nil: none yet)
	tags     []*core.Type    // the window's tags at the last pick
	// The seed's revalidation (vm_poly_seed.go): the live aggregate it was
	// checked against and the live overload that is the seed there (nil:
	// the seed does not hold in seedFn).
	seedFn  *core.FnDefInfo
	seedSig *core.Signature
}

// polyCacheFor returns pr's entry, creating it.
func (vc *vmContext) polyCacheFor(pr *compiler.PolyRef) *polyCacheEntry {
	if vc.polyCache == nil {
		vc.polyCache = map[*compiler.PolyRef]*polyCacheEntry{}
	}
	e := vc.polyCache[pr]
	if e == nil {
		e = &polyCacheEntry{}
		vc.polyCache[pr] = e
	}
	return e
}

// hit returns the remembered pick when fn is the aggregate it came from and
// window carries the same tags, or nil.
func (e *polyCacheEntry) hit(fn *core.FnDefInfo, window []core.Value) *core.Signature {
	if e.sig == nil || e.fn != fn || len(e.tags) != len(window) {
		return nil
	}
	for i := range window {
		if window[i].Parent != e.tags[i] || !core.TagKeyable(window[i]) {
			return nil
		}
	}
	return e.sig
}

// fill records a full match's pick for window, when the pick may be keyed on
// its tags.
func (e *polyCacheEntry) fill(fn *core.FnDefInfo, sigs []core.Signature, window []core.Value, sig *core.Signature) {
	if e.fn != fn {
		e.fn, e.eligible, e.sig = fn, core.TagDeterminedSigs(sigs, len(window)), nil
	}
	e.sig = nil
	if !e.eligible {
		return
	}
	for i := range window {
		if !core.TagKeyable(window[i]) {
			return
		}
	}
	if cap(e.tags) < len(window) {
		e.tags = make([]*core.Type, len(window))
	}
	e.tags = e.tags[:len(window)]
	for i := range window {
		e.tags[i] = window[i].Parent
	}
	e.sig = sig
}
