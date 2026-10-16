package core

// Region extent — the syntactic span a compiled region descriptor covers
// (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// A statement's extent is NOT statically bounded: its own operands can move
// it, because a paren group at position 0 that `def`s a FUNCTION word
// creates a forward-collection barrier at position 1 that did not exist when
// the scan began. Which slots a statement owns is an OUTPUT of running it.
// So the descriptor's unit is a REGION — everything to the next HARD
// delimiter — whose boundaries are syntactic and cannot shift under any
// binding, and collection returns a cursor saying where it actually stopped.
//
// The hard delimiters are `end`, a close paren, and the window's end. An
// OPEN paren is not one: it opens a nested scope the region contains.
//
// Why this walk is sound as a bound, which is not obvious. Two pieces of
// error-selection state are read by scanning BACKWARD from the pointer, and
// neither scan stops at `end`:
//
//   - strandedForwardError picks the Forward it blames (engine.go:6543),
//   - stepEnd picks the Forward it drains (engine.go:6604).
//
// A Forward parked in an EARLIER region would therefore be visible from
// inside this one, and a region-bounded descriptor could not carry it. It
// cannot happen, because at most ONE Forward is ever live per paren scope:
// insertForward is the sole parking site; its bare-word caller is guarded by
// the strict barrier (commit-or-strand runs before dispatch, and neither
// outcome reaches a second park); and its execFnDefLiteral caller — a
// function VALUE forward-collecting, which the barrier does NOT guard — is
// held shut by the arrival loop routing that value into the parked slot
// instead. `end` then drains the one that exists. Pinned in
// region_extent_test.go; the two raise texts it separates are pinned as
// lang/spec/forward-barrier.tsv §9.
//
// The same invariant bounds the diagnostic state F2 widened this list with:
// IsFnShapeTypedBindingContext, pendingForwardFunc and polyReachBound all
// key on the enclosing collector's Forward, so the collector they find is
// always the region's own.

// RegionEnd returns the exclusive index at which the region starting at
// `from` ends: the first hard delimiter at or after it, or the window's
// length. Nested paren scopes are CONTAINED, not terminated — an open paren
// carries the walk to its matching close, so a region spans a whole group
// rather than stopping inside one.
//
// Out-of-range and negative starts clamp; the result is always in
// [max(from,0), w.Len()].
func RegionEnd(w CollectWindow, from int) int {
	if !windowPresent(w) {
		return 0
	}
	n := w.Len()
	if from < 0 {
		from = 0
	}
	if from > n {
		return n
	}
	depth := 0
	for i := from; i < n; i++ {
		v := w.At(i)
		switch {
		case IsOpenParen(&v):
			depth++
		case IsCloseParen(&v):
			// A close paren at depth 0 ends the region: it belongs to a
			// group that OPENED before `from`, so the region sits inside
			// that group and cannot outlive it.
			if depth == 0 {
				return i
			}
			depth--
		case IsEnd(&v):
			// `end` inside a nested group ends that group's statement, not
			// this region.
			if depth == 0 {
				return i
			}
		}
	}
	return n
}

// RegionSlotCount is the number of tokens the region starting at `from`
// spans, delimiter excluded.
func RegionSlotCount(w CollectWindow, from int) int {
	if !windowPresent(w) {
		return 0
	}
	if from < 0 {
		from = 0
	}
	end := RegionEnd(w, from)
	if end < from {
		return 0
	}
	return end - from
}

// windowPresent reports whether w is a usable window.
//
// A plain `w == nil` is NOT enough, and the gap is the classic Go one: a
// TYPED nil (`var t *Tape; RegionEnd(t, 0)`) is a non-nil interface holding
// a nil pointer, so the plain check passes it through and (*Tape).Len
// dereferences it. Panics are forbidden outside annotated init-time
// registration (ADR-005), and a function that advertises nil handling has to
// deliver it for the shape a caller will actually produce.
//
// The type assertion covers *Tape because *Tape is the only implementer
// today. A future window that can be typed-nil must either be added here or
// make its own methods nil-safe; the compile-time assertion in
// collect_kernel.go is what will bring a new implementer to this comment.
func windowPresent(w CollectWindow) bool {
	if w == nil {
		return false
	}
	if t, ok := w.(*Tape); ok && t == nil {
		return false
	}
	return true
}
