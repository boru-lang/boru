package core

// The occurrence record (design/VALUE-NO-COPY.0.md §6 phase 3).
//
// A Value's header keeps what the value IS — its Parent, its payload, its
// type-node facet (tmeta, which also holds a type node's own identity), its
// element constraint, its module-namespace facet and the kind flags — and
// reaches the rest of its occurrence state through one pointer: its source
// position, its identity token (a parsed token's, or a check pass's
// provenance key; a copy of a type node reads the node's from tmeta unless
// this holder was given another), the dispatch ascription (`v as T`), a
// gradual read's source binding (check mode) and a quoted atom's referent.
// Most runtime values carry none of it, so the pointer is nil and a copy of
// the header carries 8 bytes for all of it instead of the fields themselves.
//
// A position is minted once (by the parser, or where a value is given a
// position it did not have) together with the record that carries it ALONE
// (position.only), so threading a position onto a value whose occurrence
// holds nothing else — a fresh result, a type node's literal, a minted
// marker — points the value at that record and allocates nothing. Only a
// value that already holds other occurrence state (an identity, an
// ascription, a source binding) and takes a DIFFERENT position needs a
// record of its own.
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
// One record is the exception, by design: a pooled Loop's position (Loop.at,
// loop.go) is re-aimed when the Loop is reused at another call site, and
// only the loop's own synthetic tokens — its mark, parens and move, which
// live inside the one region it runs — refer to it. Nothing threads a loop
// token's position onto a value (a call's results take the call-site word's,
// stampResultCells), so no value that outlives the region sees it change.
//
// The occurrence FLAGS (quoted, eval, undefined, failed dispatch, reach
// group, dynamic, and the seam marks: applied, return-trimmed, signature
// matched) are two bytes of the header instead, in the padding the
// one-byte kind flags leave: the same per-holder rule, no pointer to
// follow on a read, and no record to build on a write, which matters
// because the engine clears a flag on every delivered argument and a
// parsed token carries an identity (a record of its own).
type occurrence struct {
	id string
	// name is the name a binding gave this holder's fn value (SetFnName):
	// a frame's param, a def's store. It is the holder's, as a binding's
	// name is the binding's — the fn's payload keeps the name it was
	// constructed under.
	name    string
	asc     *Type
	dynFrom *string
	pos     *position
	// rec is the recorder key a recording pass stamped on this holder: the
	// event that produced its value and the result index (RecKey).
	rec RecKey
	// referent is what this holder's atom named when it was quoted or its
	// program loaded (SetAtomReferent): a reference to the value bound then,
	// which a later rebind does not reach (a capture, the exceptions list's
	// C5). The holder's, as an identity is: the atom's payload is its name
	// alone, so atoms with one name stay one atom to equality and canonical
	// form whatever each holder captured.
	referent *Value
}

// RecKey is a recorded value's recorder key (design/VALUE-NO-COPY.0.md §5:
// recorder identity is keyed by (event, result index)): the event of a
// recording pass that produced this holder's value and the value's index
// among that event's results. The recorder stamps it on the holder an
// event's results flow on (SetRecKey) and reads it back from whichever copy
// of that holder reaches a later event (RecKeyOf) — provenance carried with
// the occurrence, as a position is, never looked up through the value's
// identity token. Pass and Gen are the recorder's own bookkeeping: which
// recording pass stamped the key, and which issuance of Seq (a rolled-back
// round reissues its event numbers). The zero key is "no producer".
type RecKey struct {
	Pass uint32
	Gen  uint32
	Seq  int32
	Idx  int32
}

// position is a source position and the occurrence record that carries it
// alone ({pos: this}), allocated as one object. Positions are immutable
// once minted and shared by every holder they are threaded onto.
type position struct {
	SrcPos
	only occurrence
}

// newPosition mints a position (and its own record).
func newPosition(p SrcPos) *position {
	np := &position{SrcPos: p}
	np.only.pos = np
	return np
}

// positionedRecord is a record and the position it carries, allocated as
// one object: SetPos on a value that already holds other occurrence state.
type positionedRecord struct {
	at  position
	rec occurrence
}

// newPositionedRecord is rec carrying a newly minted position p.
func newPositionedRecord(rec occurrence, p SrcPos) *occurrence {
	pr := &positionedRecord{at: position{SrcPos: p}, rec: rec}
	pr.at.only.pos = &pr.at
	pr.rec.pos = &pr.at
	return &pr.rec
}

// idCleared is the identity a holder of a type node's copy is given when
// its identity is cleared: the node's own (tmeta) would otherwise show
// through. IDOf reads it as "".
const idCleared = "\x00"

// posOnly reports whether rec carries a position and nothing else.
func posOnly(rec *occurrence) bool {
	return rec.id == "" && rec.name == "" && rec.asc == nil && rec.dynFrom == nil && rec.rec == RecKey{} && rec.referent == nil
}

// occFlags are the per-occurrence flags, one bit each.
type occFlags uint16

const (
	occQuoted occFlags = 1 << iota
	occEval
	occUndefined
	occFailedDispatch
	occReachGroup
	occDynamic
	occApplied
	occRetTrim
	occSigMatched
)

// occOf is the occurrence state of v by value (the zero record when none).
func occOf(v *Value) occurrence {
	if v.occ == nil {
		return occurrence{}
	}
	return *v.occ
}

// setOcc repoints v at a record holding rec: nil for the empty state, the
// position's own record for a position alone, a new record otherwise.
func setOcc(v *Value, rec occurrence) {
	switch {
	case rec == (occurrence{}):
		v.occ = nil
	case posOnly(&rec):
		v.occ = &rec.pos.only
	default:
		v.occ = newOcc(rec)
	}
}

// posOf is v's position, nil when it has none.
func posOf(v *Value) *position {
	if v.occ == nil {
		return nil
	}
	return v.occ.pos
}

// setPosRef gives this holder of v the position p (nil clears it), keeping
// the rest of its occurrence state. A value holding nothing else points at
// the position's own record; only a value with other state and a different
// position takes a new record.
func setPosRef(v *Value, p *position) {
	if v.occ == nil {
		if p != nil {
			v.occ = &p.only
		}
		return
	}
	if v.occ.pos == p {
		return
	}
	rec := *v.occ
	rec.pos = p
	setOcc(v, rec)
}

// derefPos is the source position p stands for (the zero SrcPos for none):
// a word step snapshots its cell's position before the tape moves under it.
func derefPos(p *position) SrcPos {
	if p == nil {
		return SrcPos{}
	}
	return p.SrcPos
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

// IsApplied reports `apply`'s ONE-SHOT signal that an application was
// explicitly ASKED for at this fn value — set only on a fn whose only
// signatures are 0-arg (MarkApplied), and consumed by the very next
// re-step in execFnDefLiteral, which clears it alongside ReachGroup under
// the same Quoted-transience discipline.
//
// It exists because the inert-lambda gate in execFnDefLiteral keys on
// Anonymous, and that gate is load-bearing: it is what makes `def f ([] =>
// [42])` bind f to the FUNCTION rather than to 42. But keying an
// APPLICATION on it made ORIGIN decide the outcome, which ADR-016 forbids —
//
//	def z fn [[] [Integer] [42]]   z/v apply  ->  42
//	def f ([] => [42])             f/v apply  ->  fn f   (was)
//
// The flag separates the two questions the gate had conflated: "is this
// value data?" (origin) and "did someone ask to call it?" (this). A lambda
// sitting on the stack still parks; the same lambda behind an explicit
// `apply` dispatches through the ONE re-step path every other arity
// already uses, so natives keep their Go handlers, context mutations land
// in the caller's frame, and the check pass models the result by
// re-stepping exactly as runtime does. Macros are deliberately NOT covered:
// applying a macro is never a stack-value dispatch
// (design/legacy/MACROS-PHASE1.10.ignore §5, D4). The flag was a field of
// the fn payload until no-copy phase 3, so setting it copied the payload.
func IsApplied(v *Value) bool { return occFlag(v, occApplied) }

// SetApplied marks or clears v's application signal, for this holder only.
func SetApplied(v *Value, on bool) { setOccFlag(v, occApplied, on) }

// IsRetTrim reports that a compiled closure VALUE was handed through the
// fn-VALUE seam (InvokeCallbackBody): the handler would hand a FnDefInfo to
// InvokeCallbackFn, whose CallBoru path checks the declared TYPES over the
// aligned residual and never raises on the COUNT (enforceCallBoruReturns) —
// walk's hooks, the map-iteration each/fold, filter's Function form. Off,
// the closure came through the TOKEN seam (InvokeBody: each over a list,
// apply, a paren call), where the fn value is stepped and __RC enforces the
// count. Set on the holder at the seam, never on the stored closure: the
// same closure can cross both.
func IsRetTrim(v *Value) bool { return occFlag(v, occRetTrim) }

// SetRetTrim marks or clears v's fn-VALUE seam crossing, for this holder
// only.
func SetRetTrim(v *Value, on bool) { setOccFlag(v, occRetTrim, on) }

// IsSigMatched reports that the seam handing a compiled closure VALUE its
// args has already matched the closure's OWN signature, with the args in
// signature order: the closure bridge (a bridged FnDefInfo's handler —
// closureAsWord, ClosureAsFnDef — runs after the interpreter's dispatch
// matched), and a native seam that matched the value's contract over the
// lambda-shaped args it hands (the map arm's KeyVal, filter's entry, walk's
// payload). Off, the closure reached the TOKEN seam (InvokeBody: each over
// a list, a paren call) with its inputs in STACK order, and a fn-VALUE
// closure is matched there the way the interpreter matches a stepped value,
// top down, before its unit runs, and declines to the stepping path when
// nothing matches (S1b-2; eng/go/vm_fnvalue_seam.go). Set on the holder at
// the seam, never on the stored closure.
func IsSigMatched(v *Value) bool { return occFlag(v, occSigMatched) }

// SetSigMatched marks or clears v's matched-signature seam mark, for this
// holder only.
func SetSigMatched(v *Value, on bool) { setOccFlag(v, occSigMatched, on) }

// IDOf is v's identity token: the canonical ID a type node (and every
// literal copy of it) is known by — the node's own, in its tmeta — the
// provenance key a check pass mints for a value it tracks, a parsed token's
// identity, or "" for the overwhelming majority of runtime values, which
// have none.
func IDOf(v *Value) string {
	if v.occ != nil && v.occ.id != "" {
		if v.occ.id == idCleared {
			return ""
		}
		return v.occ.id
	}
	if d := v.d; d != nil && d.tmeta != nil {
		return d.tmeta.id
	}
	return ""
}

// SetID gives this holder of v the identity id ("" clears it); every other
// copy keeps the identity it had. A copy of a type node that is given its
// node's own identity back drops the override. A new identity is a new
// value to a recording pass: the holder's recorder key goes with the old
// one (ShareIdentity carries both).
func SetID(v *Value, id string) {
	if IDOf(v) == id {
		return
	}
	own := ""
	if v.body().tmeta != nil {
		own = v.body().tmeta.id
	}
	rec := occOf(v)
	switch {
	case id == own:
		rec.id = ""
	case id == "":
		rec.id = idCleared
	default:
		rec.id = id
	}
	rec.rec = RecKey{}
	setOcc(v, rec)
}

// ShareIdentity gives this holder of dst src's identity and recorder key:
// dst is the same value as src to a recording pass (a narrowed binding, a
// fold keeping its result's identity, a join of two arms that only
// narrowed one binding).
func ShareIdentity(dst, src *Value) {
	SetID(dst, IDOf(src))
	SetRecKey(dst, RecKeyOf(src))
}

// RecKeyOf is the recorder key a recording pass stamped on this holder of v
// (SetRecKey), the zero key when none did.
func RecKeyOf(v *Value) RecKey {
	if v.occ == nil {
		return RecKey{}
	}
	return v.occ.rec
}

// SetRecKey stamps this holder of v with the recorder key k (the zero key
// clears it); every other copy keeps the key it had, so a stamp reaches the
// results an event hands on and the copies made of them after, never a
// binding or an argument the event read.
func SetRecKey(v *Value, k RecKey) {
	if RecKeyOf(v) == k {
		return
	}
	rec := occOf(v)
	rec.rec = k
	setOcc(v, rec)
}

// FnNameOf is the name a fn value answers to through this holder: the name
// a binding gave the holder (SetFnName), else the name its payload was
// constructed under — a fn definition's Name, a compiled closure's RetName
// — and "" for an anonymous value or a value that is no fn.
func FnNameOf(v *Value) string {
	if v.occ != nil && v.occ.name != "" {
		return v.occ.name
	}
	switch d := v.Data().(type) {
	case *FnDefInfo:
		return d.Name
	}
	return ""
}

// HolderFnName is the name a binding gave this holder of a fn value
// (SetFnName), "" when none did — the holder's own name, without the
// payload's fallback.
func HolderFnName(v *Value) string {
	if v.occ == nil {
		return ""
	}
	return v.occ.name
}

// SetFnName gives this holder of the fn value v the name a binding knows
// it by — a frame's param, a def — and every other copy keeps the name it
// had: the payload is shared, never copied to carry a binding's name (the
// exceptions list's D8, retired in phase 3).
func SetFnName(v *Value, name string) {
	if HolderFnName(v) == name {
		return
	}
	rec := occOf(v)
	rec.name = name
	setOcc(v, rec)
}
