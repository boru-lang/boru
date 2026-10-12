package check

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// Static index / size checking for check mode.
//
// The runtime errors on an out-of-bounds list index (`getr`, `set`) or
// out-of-bounds selector (`at`). This pass catches the provable cases
// statically: when a list's length is known and an index is provably
// outside [0, length), it records an `index_out_of_range` diagnostic
// (SeverityError — every consuming word errors at runtime on the index,
// so the finding is a guaranteed failure, not a suspicion).
//
// Soundness is the whole game here — a false positive (flagging an
// access that is actually in bounds) would be far worse than a missed
// one. Two rules keep it sound:
//
//   - The length used for the comparison must be the EXACT length or an
//     UPPER bound. A concrete list reports its exact element count; a
//     length-refined carrier (ChildTypeInfo.Len) must never understate
//     its length. An overestimate only causes a missed detection.
//   - An index is flagged only when EVERY value it could take is out of
//     range: a concrete literal is its own single value; a DepScalar's
//     whole interval must lie outside [0, length).
//
// When either the length or the index bound is unknown, the checker
// stays silent.

// StaticListLen returns the statically-known length of a list value and
// true, or (0, false) when the length cannot be determined. A concrete
// list literal reports its exact element count; a typed-list carrier
// reports its ChildTypeInfo.Len refinement when present. A plain
// (unrefined) carrier, a non-list, or a type literal reports false.
func StaticListLen(v core.Value) (int, bool) {
	if !v.Parent.ConformsTo(core.TList) {
		return 0, false
	}
	if core.IsConcrete(&v) {
		if list, err := core.AsList(v); err == nil && !list.IsNil() {
			return list.Len(), true
		}
		return 0, false
	}
	if ct, ok := v.Data.(core.ChildTypeInfo); ok && ct.Len != nil {
		return *ct.Len, true
	}
	return 0, false
}

// minIndex returns the smallest integer an index value can take and
// true, or false when there is no lower bound the checker can use. A
// concrete integer is its own minimum; a DepScalar contributes its
// lower bound (rounded up for a strict `gt`).
func minIndex(idx core.Value) (int64, bool) {
	if idx.IsDepScalar() {
		info, err := idx.AsDepScalar()
		if err != nil || info.Lo == nil {
			return 0, false
		}
		n, err := info.Lo.Value.AsConcreteInteger()
		if err != nil {
			return 0, false
		}
		if !info.Lo.Inclusive { // gt n → smallest is n+1
			n++
		}
		return n, true
	}
	if core.IsConcrete(&idx) && idx.Parent.Equal(core.TInteger) {
		n, err := idx.AsConcreteInteger()
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// maxIndex returns the largest integer an index value can take and
// true, or false when there is no upper bound. Used for the negative-
// index case: an index whose maximum is below zero is always invalid.
func maxIndex(idx core.Value) (int64, bool) {
	if idx.IsDepScalar() {
		info, err := idx.AsDepScalar()
		if err != nil || info.Hi == nil {
			return 0, false
		}
		n, err := info.Hi.Value.AsConcreteInteger()
		if err != nil {
			return 0, false
		}
		if !info.Hi.Inclusive { // lt n → largest is n-1
			n--
		}
		return n, true
	}
	if core.IsConcrete(&idx) && idx.Parent.Equal(core.TInteger) {
		n, err := idx.AsConcreteInteger()
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// indexProvablyOOB reports whether idx is provably out of bounds for a
// list of length n, with a human-readable reason. It returns false
// unless every value idx could take lies outside [0, n).
func indexProvablyOOB(idx core.Value, n int) (string, bool) {
	// Too high: the smallest value the index can take is already past
	// the last valid position.
	if lo, ok := minIndex(idx); ok && lo >= int64(n) {
		if idx.IsDepScalar() {
			return fmt.Sprintf("index (minimum %d) is out of bounds for a list of length %d", lo, n), true
		}
		return fmt.Sprintf("index %d is out of bounds for a list of length %d", lo, n), true
	}
	// Negative: the largest value the index can take is below zero
	// (negative indices are not supported — the runtime errors on them).
	if hi, ok := maxIndex(idx); ok && hi < 0 {
		if idx.IsDepScalar() {
			return fmt.Sprintf("index (maximum %d) is negative", hi), true
		}
		return fmt.Sprintf("index %d is negative", hi), true
	}
	return "", false
}

// emitIndexOOB records an index_out_of_range diagnostic stamped with
// the offending index value's source position (when known). A
// RuntimeMirror: the consuming word errors at runtime on exactly this
// index, and the recording model is unaffected — inside an error-catching
// `do` body AddDiagnostic re-attributes it to a caught info finding.
func emitIndexOOB(r *core.Registry, word, detail string, pos core.SrcPos) {
	r.Check.AddDiagnostic(core.CheckDiagnostic{
		Code:          "index_out_of_range",
		Detail:        word + ": " + detail,
		Word:          word,
		Row:           pos.Row,
		Col:           pos.Col,
		RuntimeMirror: true,
	})
}

// CheckListIndex is the check-mode bounds check for a single-integer
// index access (`getr` on a list). It is a no-op unless the container's
// length is statically known and idx is provably out of range — so it
// silently ignores map/object containers, unknown-length carriers, and
// unknown indices.
func CheckListIndex(r *core.Registry, idx, container core.Value, word string) {
	n, ok := StaticListLen(container)
	if !ok {
		return
	}
	if detail, oob := indexProvablyOOB(idx, n); oob {
		emitIndexOOB(r, word, detail, idx.Pos())
	}
}

// CheckAtIndices is the check-mode bounds check for `at`, whose first
// argument is a list of integer indices selecting from the data list.
// Each statically-known index is checked against the data length; a
// provably out-of-range one is flagged. No-op unless the data length is
// known and the indices are a concrete list.
func CheckAtIndices(r *core.Registry, indices, data core.Value, word string) {
	n, ok := StaticListLen(data)
	if !ok {
		return
	}
	if !core.IsConcrete(&indices) || !indices.Parent.ConformsTo(core.TList) {
		return
	}
	list, err := core.AsList(indices)
	if err != nil || list.IsNil() {
		return
	}
	for i := 0; i < list.Len(); i++ {
		idx := list.Get(i)
		if detail, oob := indexProvablyOOB(idx, n); oob {
			emitIndexOOB(r, word, detail, idx.Pos())
		}
	}
}
