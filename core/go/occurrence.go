package core

// The occurrence record (design/VALUE-NO-COPY.0.md §6 phase 3).
//
// A Value's header keeps what the value IS — its Parent, its payload, its
// type-node facet (tmeta), its element constraint, its module-namespace
// facet and the kind flags — and reaches the state of THIS OCCURRENCE of it
// through one pointer: the dispatch ascription (`v as T`), a gradual read's
// source binding (check mode), and the occurrence flags (quoted, eval,
// undefined, failed dispatch, reach group, dynamic). Most values carry none
// of it, so the pointer is nil and a copy of the header carries 8 bytes for
// all of it instead of the fields themselves.
//
// The record is immutable once a Value refers to it. A setter never writes
// a record in place: it builds a new one, or takes an interned flags-only
// one, and repoints the ONE Value it was called on. Every other copy of the
// header keeps the record it had, so a write reaches exactly the holder it
// was made through — the tape cell, the stack cell, the binding, the arg
// slot — as a write to a by-value field did. That is rule R3 of the
// exceptions list ("occurrence state lives in the slot") under the by-value
// representation: the slot's header is the slot's own, and the record its
// header points at is never shared for writing.
type occurrence struct {
	asc     *Type
	dynFrom *string
	flags   occFlags
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
	occFlagCombos = 1 << iota // number of flag combinations
)

// flagsOnly interns one record per flag combination, for a value whose only
// occurrence state is its flags: a flag write on such a value repoints it
// here instead of allocating. Never written after init.
var flagsOnly [occFlagCombos]occurrence

func init() {
	for i := range flagsOnly {
		flagsOnly[i].flags = occFlags(i)
	}
}

// occOf is the occurrence state of v by value (the zero record when none).
func occOf(v *Value) occurrence {
	if v.occ == nil {
		return occurrence{}
	}
	return *v.occ
}

// setOcc repoints v at a record holding rec: nil for the empty state, an
// interned record for flags alone, a new record otherwise.
func setOcc(v *Value, rec occurrence) {
	if rec.asc == nil && rec.dynFrom == nil {
		if rec.flags == 0 {
			v.occ = nil
		} else {
			v.occ = &flagsOnly[rec.flags]
		}
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

func occFlag(v *Value, f occFlags) bool { return v.occ != nil && v.occ.flags&f != 0 }

func setOccFlag(v *Value, f occFlags, on bool) {
	if occFlag(v, f) == on {
		return
	}
	rec := occOf(v)
	rec.flags ^= f
	setOcc(v, rec)
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
