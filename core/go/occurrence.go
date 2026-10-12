package core

// The occurrence record (design/VALUE-NO-COPY.0.md §6 phase 3).
//
// A Value's header keeps what the value IS — its Parent, its payload, its
// type-node facet (tmeta), its element constraint, its module-namespace
// facet and the kind flags — and reaches the rest of its occurrence state
// through one pointer: its identity token (the ID a type node is known by,
// or a check pass's provenance key), the dispatch ascription (`v as T`) and
// a gradual read's source binding (check mode). Most runtime values carry
// none of it, so the pointer is nil and a copy of the header carries 8
// bytes for all of it instead of the fields themselves.
//
// The record is immutable once a Value refers to it. A setter never writes
// a record in place: it builds a new one and repoints the ONE Value it was
// called on. Every other copy of the header keeps the record it had, so a
// write reaches exactly the holder it was made through — the tape cell, the
// stack cell, the binding, the arg slot — as a write to a by-value field
// did. That is rule R3 of the exceptions list ("occurrence state lives in
// the slot") under the by-value representation: the slot's header is the
// slot's own, and the record its header points at is never shared for
// writing.
//
// The occurrence FLAGS (quoted, eval, undefined, failed dispatch, reach
// group, dynamic) are one byte of the header instead, in the padding the
// one-byte kind flags leave: the same per-holder rule, no pointer to
// follow on a read, and no record to build on a write, which matters
// because the engine clears a flag on every delivered argument and a
// parsed token carries an identity (a record of its own).
type occurrence struct {
	id      string
	asc     *Type
	dynFrom *string
}

// occFlags are the per-occurrence flags, one bit each.
type occFlags uint8

const (
	occQuoted occFlags = 1 << iota
	occEval
	occUndefined
	occFailedDispatch
	occReachGroup
	occDynamic
)

// occOf is the occurrence state of v by value (the zero record when none).
func occOf(v *Value) occurrence {
	if v.occ == nil {
		return occurrence{}
	}
	return *v.occ
}

// setOcc repoints v at a record holding rec: nil for the empty state, a new
// record otherwise.
func setOcc(v *Value, rec occurrence) {
	if rec == (occurrence{}) {
		v.occ = nil
		return
	}
	v.occ = newOcc(rec)
}

// newOcc allocates a record; a separate function so the allocation happens
// only on the path that needs one (setOcc's parameter never escapes).
func newOcc(rec occurrence) *occurrence {
	o := new(occurrence)
	*o = rec
	return o
}

// idOcc is the record for an identity alone (nil for none), for a
// constructor that builds its Value in one literal.
func idOcc(id string) *occurrence {
	if id == "" {
		return nil
	}
	return newOcc(occurrence{id: id})
}

func occFlag(v *Value, f occFlags) bool { return v.flags&f != 0 }

func setOccFlag(v *Value, f occFlags, on bool) {
	if on {
		v.flags |= f
	} else {
		v.flags &^= f
	}
}

// IsQuoted reports whether v was produced by `quote` (or is quoted data):
// a quoted value is never auto-evaluated.
func IsQuoted(v *Value) bool { return occFlag(v, occQuoted) }

// SetQuoted marks or clears v's quotedness, for this holder only.
func SetQuoted(v *Value, on bool) { setOccFlag(v, occQuoted, on) }

// IsEval reports whether v is a parser-created list that auto-evaluates at
// the end of a Run.
func IsEval(v *Value) bool { return occFlag(v, occEval) }

// SetEval marks or clears v's end-of-run auto-evaluation, for this holder
// only.
func SetEval(v *Value, on bool) { setOccFlag(v, occEval, on) }

// IsUndefined reports whether v is the atom an undefined word became (an
// error if it is left on the result stack).
func IsUndefined(v *Value) bool { return occFlag(v, occUndefined) }

// SetUndefined marks or clears v as an undefined word's atom, for this
// holder only.
func SetUndefined(v *Value, on bool) { setOccFlag(v, occUndefined, on) }

// IsFailedDispatch reports whether v is a named Function value a dispatch
// attempt left on the tape because no signature matched. CHECK MODE ONLY:
// at runtime the failure raises at the dispatch site
// (design/legacy/FN-VALUE-DISPATCH.0.ignore), so no such value survives;
// analysis continues past the finding, and the mark is how a later
// check-mode consumer tells dispatch wreckage from a value the program
// meant to produce (defWordExtension reads it).
func IsFailedDispatch(v *Value) bool { return occFlag(v, occFailedDispatch) }

// SetFailedDispatch marks or clears v as dispatch wreckage, for this holder
// only.
func SetFailedDispatch(v *Value, on bool) { setOccFlag(v, occFailedDispatch, on) }

// IsReachGroup reports whether v is an OPEN-PAREN marker that the REACH
// LOWERING emitted rather than the user writing — the synthetic group `a.b`
// expands to (expandReach → lowerReach). A function value alone inside
// such a group must not dispatch there; see execFnDefLiteral. Only the open
// marker carries it, since that is the side the dispatch test looks at, and
// IsOpenParen keys on Parent alone so every other marker consumer is
// unaffected.
func IsReachGroup(v *Value) bool { return occFlag(v, occReachGroup) }

// SetReachGroup marks or clears v as a reach-lowered group, for this holder
// only.
func SetReachGroup(v *Value, on bool) { setOccFlag(v, occReachGroup, on) }

// IsDynamic reports whether v is a bounded gradual carrier (Elixir-style
// dynamic(T) — design/legacy/dynamic-modality-report.10.ignore). It implies
// Carrier. Its Parent/Data is a BOUND, not a proven type: at a signature
// boundary it matches the slot unless PROVABLY disjoint from it
// (not-disjoint rule), rather than by strict ConformsTo. Set only on
// carriers the checker cannot prove exactly (escape hatches); cleared by a
// successful guard, which discharges the gradual obligation.
func IsDynamic(v *Value) bool { return occFlag(v, occDynamic) }

// SetDynamic marks or clears v as a gradual carrier, for this holder only.
func SetDynamic(v *Value, on bool) { setOccFlag(v, occDynamic, on) }

// IdentityOnly is a Value carrying the identity id and nothing else: the
// key an ID-keyed table is asked with when all its caller holds is the ID
// (a recorded landing's own value, a hazard's lead).
func IdentityOnly(id string) Value { return Value{occ: idOcc(id)} }

// IDOf is v's identity token: the canonical ID a type node (and every
// literal copy of it) is known by, the provenance key a check pass mints
// for a value it tracks, or "" for the overwhelming majority of runtime
// values, which have none.
func IDOf(v *Value) string {
	if v.occ == nil {
		return ""
	}
	return v.occ.id
}

// SetID gives this holder of v the identity id ("" clears it); every other
// copy keeps the identity it had.
func SetID(v *Value, id string) {
	if IDOf(v) == id {
		return
	}
	rec := occOf(v)
	rec.id = id
	setOcc(v, rec)
}
