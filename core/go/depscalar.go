package core

import (
	"fmt"
	"strings"
)

// DepKind is a bit-field selector for the comparison encoded in a
// DepScalar value. One bit per primitive comparison; declaring it as
// a bit field (rather than a small enum) leaves room for combined
// constraints — a future range constraint can OR DepGTE | DepLTE into
// a single value with a low and a high bound. The current slice
// implements only the four single-comparison cases.
type DepKind uint8

const (
	DepGT  DepKind = 1 << iota // strictly greater than the bound
	DepGTE                     // greater than or equal to the bound
	DepLT                      // strictly less than the bound
	DepLTE                     // less than or equal to the bound
)

// String returns a short human-readable name for the comparison kind.
// Multiple bits set are joined with '|' so future combined constraints
// stay legible in error messages.
func (k DepKind) String() string {
	parts := make([]string, 0, 4)
	if k&DepGT != 0 {
		parts = append(parts, "gt")
	}
	if k&DepGTE != 0 {
		parts = append(parts, "gte")
	}
	if k&DepLT != 0 {
		parts = append(parts, "lt")
	}
	if k&DepLTE != 0 {
		parts = append(parts, "lte")
	}
	if len(parts) == 0 {
		return "?"
	}
	return strings.Join(parts, "|")
}

// DepBound is one side of a dependent-scalar constraint: a comparison
// against a concrete bound. Inclusive distinguishes weak (≤, ≥) from
// strict (<, >). The bound's Parent pins the base scalar type for the
// containing DepScalar.
type DepBound struct {
	Inclusive bool  // true → GTE/LTE, false → GT/LT
	Value     Value // the concrete bound being compared against
}

// DepScalarInfo is the payload carried by a Value whose Parent is one
// of the well-known scalar base types (Integer, Float, Number,
// String, Boolean, Atom) when the value represents a constraint
// rather than a concrete scalar.
//
// A DepScalar carries up to two bounds:
//
//   - Lo (lower bound) — values must satisfy `v > Lo.Value` (strict)
//     or `v >= Lo.Value` (inclusive).
//   - Hi (upper bound) — values must satisfy `v < Hi.Value` (strict)
//     or `v <= Hi.Value` (inclusive).
//
// Either side may be nil meaning "unbounded on this side". The two
// bounds are AND-combined; intervals like `[10, 20]` set both Lo and
// Hi. Same-side combinations (e.g. `gte 10 tand gte 5`) tighten to
// the more restrictive single bound during construction or
// unification — the dual-bound shape is reserved for genuine
// intervals where Lo and Hi are on opposite sides of the lattice.
//
// Empty intervals (Lo > Hi, or equal with strict on either side)
// reduce to Never *before* a DepScalarInfo with that shape can be
// constructed, so a populated info always represents a non-empty
// constraint.
//
// The Value's Parent IS the base scalar type — `typeof (Integer gte
// 0)` returns `Integer`. The DepScalarInfo payload is what
// distinguishes a constraint value from an ordinary scalar; IsDepScalar
// detects it via the payload type.
type DepScalarInfo struct {
	Lo *DepBound // nil = no lower bound
	Hi *DepBound // nil = no upper bound
}

// kindToBound translates a single DepKind into a (DepBound, isLower)
// pair. Returns (nil, false) for kinds that don't decompose into a
// single bound (zero, multi-bit, etc.). Used by NewDepScalar and
// combineDepScalars to feed the kind-tagged constructor APIs into
// the Lo/Hi storage.
func kindToBound(kind DepKind, value Value) (*DepBound, bool, bool) {
	switch kind {
	case DepGT:
		return &DepBound{Inclusive: false, Value: value}, true, true
	case DepGTE:
		return &DepBound{Inclusive: true, Value: value}, true, true
	case DepLT:
		return &DepBound{Inclusive: false, Value: value}, false, true
	case DepLTE:
		return &DepBound{Inclusive: true, Value: value}, false, true
	}
	return nil, false, false
}

// BoundToKind translates a (bound, isLower) pair back into the
// equivalent DepKind. Used by formatDepScalar so the surface form
// stays stable across the redesign (`(Integer gte 10 lte 20)`).
func BoundToKind(b *DepBound, lower bool) DepKind {
	if b == nil {
		return 0
	}
	if lower {
		if b.Inclusive {
			return DepGTE
		}
		return DepGT
	}
	if b.Inclusive {
		return DepLTE
	}
	return DepLT
}

// NewDepScalar builds a DepScalar Value from a comparison kind and a
// concrete bound. The bound's lattice ancestry is walked to find a
// well-known scalar base (Integer, Float, Number, String, Boolean,
// Atom); the resulting Value's Parent IS that base type, and the
// DepScalarInfo payload carries the bound. Returns a Value with a
// None Parent (and no payload) if the bound is not rooted at a
// supported scalar base or the kind doesn't decompose into a single
// bound — callers are expected to validate types before constructing.
func NewDepScalar(kind DepKind, bound Value) Value {
	base := canonicalBaseType(bound.Parent)
	if base == nil {
		return Value{Parent: TNone}
	}
	db, lower, ok := kindToBound(kind, bound)
	if !ok {
		return Value{Parent: TNone}
	}
	info := DepScalarInfo{}
	if lower {
		info.Lo = db
	} else {
		info.Hi = db
	}
	return NewValueRaw(base, info)
}

// canonicalBaseType walks t's ancestry from most-specific to root,
// returning the first node that DECLARES itself a refinement base
// (DeclareRefinementBase) — core's Integer, Float, Number, String, Boolean
// and Atom, basic's Bytes. Value-tagged subtypes (e.g. Number/Integer/42)
// strip down to their last declaring ancestor. Returns nil for types not
// rooted at a declared base.
func canonicalBaseType(t *Type) *Type {
	for d := t; d != nil; d = d.Parent {
		if d.tmeta != nil && d.tmeta.RefinementBase != nil {
			return d.tmeta.RefinementBase
		}
	}
	return nil
}

// DeclareRefinementBase declares t a base the comparison words refine
// (NUR009): `t gte bound`, `t lt bound`, `between lo hi t` build a
// refinement over it, checked through t's Comparer. A type's owner declares
// it where the type is registered — core the numeric, string, boolean and
// atom leaves below, basic the Bytes leaf — so no resolver hand-lists them.
// A nil t (a registration that failed) declares nothing.
func DeclareRefinementBase(t *Type) {
	if t == nil {
		return
	}
	t.ensureTMeta().RefinementBase = t
}

func init() {
	for _, t := range []*Type{TInteger, TFloat, TNumber, TString, TBoolean, TAtom} {
		DeclareRefinementBase(t)
	}
}

// IsDepScalar reports whether the value carries a DepScalar
// constraint. Detection is by payload type — the Value's Parent is
// just the base scalar (TInteger, TString, …), indistinguishable
// from an ordinary scalar at the lattice level.
func (v Value) IsDepScalar() bool {
	_, ok := v.Data.(DepScalarInfo)
	return ok
}

// AsDepScalar extracts the DepScalarInfo payload.
func (v Value) AsDepScalar() (DepScalarInfo, error) {
	if di, ok := v.Data.(DepScalarInfo); ok {
		return di, nil
	}
	return DepScalarInfo{}, fmt.Errorf("AsDepScalar: not a DepScalar value (got %T)", v.Data)
}

// depScalarCheck returns true if `value` satisfies every populated
// bound in info. Both Lo and Hi are AND-combined.
//
// A bound the analysis pass does not know — a computed one, the pass's
// carrier — makes the whole refinement the run's, its known side included:
// the carrier orders below every value, so a verdict over it was the
// lattice's, not the bound's (`def T (Integer lte (size s))` refused 3 at
// check time whatever s held), and a verdict the known side gives alone is
// baked with the placeholder rendered (`(Integer gte 5 lte Integer)`, where
// the run says `lte 2`). The pass admits, gradually; the run checks the
// real bounds (NUR231).
func depScalarCheck(info DepScalarInfo, value Value) bool {
	if info.Lo == nil && info.Hi == nil {
		return false
	}
	if (info.Lo != nil && !depBoundKnown(info.Lo)) || (info.Hi != nil && !depBoundKnown(info.Hi)) {
		return true
	}
	if info.Lo != nil && !depBoundCheck(info.Lo, true, value) {
		return false
	}
	if info.Hi != nil && !depBoundCheck(info.Hi, false, value) {
		return false
	}
	return true
}

// depBoundCheck applies a single-side bound to value. lower=true
// requires value > bound (or ≥ if Inclusive); lower=false requires
// value < bound (or ≤). Returns false on any CompareValues error so
// cross-type comparisons (e.g. Integer DepScalar vs String value)
// reject cleanly.
func depBoundCheck(b *DepBound, lower bool, value Value) bool {
	cmp, err := CompareValues(value, b.Value)
	if err != nil {
		return false
	}
	if lower {
		if b.Inclusive {
			return cmp >= 0
		}
		return cmp > 0
	}
	if b.Inclusive {
		return cmp <= 0
	}
	return cmp < 0
}

// boundsEqual reports whether two DepBound pointers represent the
// same constraint. Both nil → equal; one nil → not equal; otherwise
// Inclusive flag and structurally-equal Value.
func boundsEqual(a, b *DepBound) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Inclusive != b.Inclusive {
		return false
	}
	if !a.Value.Parent.Equal(b.Value.Parent) {
		return false
	}
	return ValuesEqual(a.Value, b.Value)
}

// depScalarsEqual reports whether two DepScalar payloads describe the
// same constraint: same Lo bound and same Hi bound (treating nil as
// "no bound on this side").
func depScalarsEqual(a, b DepScalarInfo) bool {
	return boundsEqual(a.Lo, b.Lo) && boundsEqual(a.Hi, b.Hi)
}

// formatDepScalar renders a DepScalar's display form. Single-bound is
// "(Leaf op bound)"; interval is "(Leaf op1 bound1 op2 bound2)" with
// the lower bound rendered first for stability. Each side is
// translated back through BoundToKind so the surface form matches
// the gt/gte/lt/lte vocabulary the user wrote.
func formatDepScalar(leaf string, info DepScalarInfo) string {
	switch {
	case info.Lo != nil && info.Hi != nil:
		return fmt.Sprintf("(%s %s %s %s %s)", leaf,
			BoundToKind(info.Lo, true), info.Lo.Value.String(),
			BoundToKind(info.Hi, false), info.Hi.Value.String())
	case info.Lo != nil:
		return fmt.Sprintf("(%s %s %s)", leaf,
			BoundToKind(info.Lo, true), info.Lo.Value.String())
	case info.Hi != nil:
		return fmt.Sprintf("(%s %s %s)", leaf,
			BoundToKind(info.Hi, false), info.Hi.Value.String())
	default:
		return fmt.Sprintf("(%s)", leaf)
	}
}

// renderDepScalar is the canonical Value-shaped wrapper around
// formatDepScalar. Every display surface in the engine — Value.String,
// ValToString, FormatValueJSON, FormatForPrint, boru_error stack
// rendering — funnels DepScalar values through here so the surface
// representation stays consistent across paths and the
// IsDepScalar→AsDepScalar dance happens in exactly one place.
//
// Returns the empty string if v isn't a DepScalar so callers can use
// `if s := renderDepScalar(v); s != ""` as a guarded alternative
// branch.
func renderDepScalar(v Value) string {
	if !v.IsDepScalar() {
		return ""
	}
	info, err := v.AsDepScalar()
	if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
		return ""
	}
	return formatDepScalar(v.Parent.Name(), info)
}

// tightenSameSide combines two same-side bounds (both lower, or both
// upper) into the tighter single bound. The caller has verified
// lower matches both inputs. Errors on the underlying compare
// propagate via ok=false (treated as Never).
func tightenSameSide(a, b *DepBound, lower bool) (*DepBound, bool) {
	cmp, err := CompareValues(a.Value, b.Value)
	if err != nil {
		return nil, false
	}
	// For lower bounds, the larger value is tighter; for upper bounds,
	// the smaller. When the bounds are equal, prefer the strict form
	// (GT over GTE, LT over LTE) — it's the narrower constraint.
	if cmp == 0 {
		strict := !a.Inclusive || !b.Inclusive
		return &DepBound{Inclusive: !strict, Value: a.Value}, true
	}
	if (lower && cmp > 0) || (!lower && cmp < 0) {
		return a, true
	}
	return b, true
}

// combineDepScalars computes the intersection of two DepScalar
// constraints over the same base type. Returns ok=false when the
// result is empty (no value satisfies both).
//
//	(Integer gt 5) tand (Integer lt 10) → interval (5, 10)
//	(Integer gte 10) tand (Integer gte 5) → gte 10 (tighter)
//	(Integer gt 10) tand (Integer lt 5) → empty (Never)
func combineDepScalars(a, b DepScalarInfo) (DepScalarInfo, bool) {
	out := DepScalarInfo{}
	// Tighten lower bounds first.
	switch {
	case a.Lo == nil:
		out.Lo = b.Lo
	case b.Lo == nil:
		out.Lo = a.Lo
	case !depBoundKnown(a.Lo) || !depBoundKnown(b.Lo):
		out.Lo = unknownBound(a.Lo, b.Lo)
	default:
		t, ok := tightenSameSide(a.Lo, b.Lo, true)
		if !ok {
			return DepScalarInfo{}, false
		}
		out.Lo = t
	}
	// Then upper bounds.
	switch {
	case a.Hi == nil:
		out.Hi = b.Hi
	case b.Hi == nil:
		out.Hi = a.Hi
	case !depBoundKnown(a.Hi) || !depBoundKnown(b.Hi):
		out.Hi = unknownBound(a.Hi, b.Hi)
	default:
		t, ok := tightenSameSide(a.Hi, b.Hi, false)
		if !ok {
			return DepScalarInfo{}, false
		}
		out.Hi = t
	}
	// Verify the resulting interval is non-empty — decided only over KNOWN
	// bounds: an unknown one is the analysis pass's carrier, which orders
	// below every value, so `(Integer lt (size s)) tand (Integer gt 5)` was
	// Never at compile time whatever s held (NUR231).
	if out.Lo != nil && out.Hi != nil && depBoundKnown(out.Lo) && depBoundKnown(out.Hi) {
		cmp, err := CompareValues(out.Lo.Value, out.Hi.Value)
		if err != nil {
			return DepScalarInfo{}, false
		}
		if cmp > 0 {
			return DepScalarInfo{}, false
		}
		// Equal bounds: empty unless both sides are inclusive.
		if cmp == 0 && (!out.Lo.Inclusive || !out.Hi.Inclusive) {
			return DepScalarInfo{}, false
		}
	}
	return out, true
}

// depBoundKnown reports whether a bound is a value the analysis pass knows
// (not its carrier for a computed one, NUR231).
func depBoundKnown(b *DepBound) bool { return IsConcrete(b.Value) }

// unknownBound picks, of two same-side bounds at least one of which the
// pass does not know, an unknown one: which is tighter is the run's to
// decide, and keeping the unknown keeps the result a type the run installs
// (HasUnknownRefinement) rather than one whose content looks known.
func unknownBound(a, b *DepBound) *DepBound {
	if !depBoundKnown(a) {
		return a
	}
	return b
}

// flipBound returns the opposite-side complement of a single bound: the
// complement of `> v` is `<= v`, of `>= v` is `< v`, and so on — same
// Value, negated inclusivity. complementWithinBase places the result on
// the correct Lo/Hi slot (a lower bound complements to an upper one).
func flipBound(b *DepBound) *DepBound {
	return &DepBound{Inclusive: !b.Inclusive, Value: b.Value}
}

// complementWithinBase returns the complement of a DepScalar constraint
// *within its base type* — the values of base that do NOT satisfy info:
//
//	tnot (Integer gt 0)        → Integer lte 0
//	tnot (Integer gte 0)       → Integer lt 0
//	tnot (Integer gt 5 lt 10)  → (Integer lte 5) tor (Integer gte 10)
//
// A single bound flips to the opposite side; an interval complements to
// the union of the two rays outside it. The FULL type complement also
// includes everything outside the base — NegateType wraps this in
// `… tor (tnot base)`.
func complementWithinBase(base *Type, info DepScalarInfo) Value {
	switch {
	case info.Lo != nil && info.Hi != nil:
		low := NewValueRaw(base, DepScalarInfo{Hi: flipBound(info.Lo)})
		high := NewValueRaw(base, DepScalarInfo{Lo: flipBound(info.Hi)})
		return NewDisjunct([]Value{low, high})
	case info.Lo != nil:
		return NewValueRaw(base, DepScalarInfo{Hi: flipBound(info.Lo)})
	case info.Hi != nil:
		return NewValueRaw(base, DepScalarInfo{Lo: flipBound(info.Hi)})
	default:
		return NewTypeLiteral(base)
	}
}

// MakeDepScalarSig builds the [TScalar, TScalar/type] -> [TScalar]
// signature variant for a comparison op. `Integer gte 10`, `String lt
// "z"`, `Float gte 1.5` all hit this sig: arg0 is the bound, arg1 is
// the base-type literal (TypeArgs[1]=true requires a type literal at
// that slot — the successor to the historical TScalarType slot). The
// result Value's Parent IS the base scalar type, with a DepScalarInfo
// payload carrying the bound. This sig sorts ahead of the [Any, Any]
// boolean sig (because its types are more specific), so concrete `5
// gte 10` still hits the boolean branch via the second match attempt.
//
// Used by ComparisonNatives to wire the same single-bound DepScalar
// constructor onto each of `lt`, `gt`, `lte`, `gte`.
//
// RunInCheckMode=true so `type G10 (Integer gt 10)` produces a real
// DepScalar value under static analysis — without it the check-mode
// pipeline would push a carrier (Data=nil) and downstream `def x:G10
// …` would have no constraint payload to reason about. The handler
// is a pure constructor with no registry side effects, so running it
// during check is safe.
func MakeDepScalarSig(opName string, kind DepKind) Signature {
	return Signature{
		Args:     []*Type{TScalar, TScalar},
		TypeArgs: map[int]bool{1: true},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
			// arg1 is the type-literal at the deep position. Reject
			// non-leaf bases — only the well-known scalar types map
			// to a supported DepScalar base.
			if IsConcrete(args[1]) {
				return nil, fmt.Errorf("%s: dependent constructor needs a scalar type literal, got concrete %s",
					opName, args[1].Parent.String())
			}
			base := canonicalBaseType(ValueType(args[1]))
			if base == nil {
				return nil, fmt.Errorf("%s: dependent constructor does not support base type %s",
					opName, ValueType(args[1]).String())
			}
			// Bound must be the same scalar base as the type literal.
			if !args[0].Parent.ConformsTo(base) {
				return nil, fmt.Errorf("%s: bound %s does not match dependent base %s",
					opName, args[0].Parent.String(), base.String())
			}
			dep := NewDepScalar(kind, args[0])
			// Check-mode bytecode recording: remember the concrete predicate
			// against its ID so a downstream operand — toCarrier strips the
			// DepScalarInfo to a bare base carrier, preserving the ID — can
			// recover the bound via origByID and bake it as a const.
			noteRefinementConstruct(r, dep, args[0])
			return []Value{dep}, nil
		}, RunInCheck()),
		Returns: []*Type{TScalar},
		BarrierPos:

		// The `between` word registration is defined in
		// lang/go/engine/native_compare.go alongside the other DepScalar
		// constructors (lt / gt / lte / gte). BetweenHandler below is the
		// exported algorithm primitive.
		-1,
	}
}

// noteRefinementConstruct tells the compile pass how a refinement it just
// built reaches the run: over KNOWN bounds it is a const (RememberOriginal,
// so a stripped operand recovers it); over a bound the pass does not know —
// a computed one, `Integer gt (size s)`, whose bound is the pass's carrier —
// the constructor call is recorded as the call it is, so the run builds the
// refinement over the real bound (NUR231).
func noteRefinementConstruct(r *Registry, dep Value, bounds ...Value) {
	if r == nil {
		return
	}
	for _, b := range bounds {
		if !IsConcrete(b) {
			// Only the analysis pass itself latches: a concrete sub-run (a
			// const-fold probe) records nothing, and a latch it left would be
			// consumed by the pass's next compile-time word.
			if r.analysisActive() {
				r.analysisRecorder().NoteRuntimeConstruct()
			}
			return
		}
	}
	r.analysisRecorder().RememberOriginal(dep)
}

func BetweenHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if IsConcrete(args[2]) {
		return nil, fmt.Errorf("between: type arg must be a scalar type literal, got concrete %s",
			args[2].Parent.String())
	}
	// args[2] is a type literal; its denoted lattice node is the value
	// itself (a by-value copy), not args[2].Parent (which is the
	// supertype after the type/value merge).
	base := canonicalBaseType(ValueType(args[2]))
	if base == nil {
		return nil, fmt.Errorf("between: unsupported base type %s",
			ValueType(args[2]).String())
	}
	if !args[0].Parent.ConformsTo(base) {
		return nil, fmt.Errorf("between: low bound %s does not match base %s",
			args[0].Parent.String(), base.String())
	}
	if !args[1].Parent.ConformsTo(base) {
		return nil, fmt.Errorf("between: high bound %s does not match base %s",
			args[1].Parent.String(), base.String())
	}
	info := DepScalarInfo{
		Lo: &DepBound{Inclusive: true, Value: args[0]},
		Hi: &DepBound{Inclusive: true, Value: args[1]},
	}
	dep := NewValueRaw(base, info)
	// An empty interval is Never — decided only over KNOWN bounds: a
	// computed one is the analysis pass's carrier, which orders below every
	// value, so `between 1 (size s) Integer` was Never at compile time
	// whatever s held (NUR231). Over an unknown bound the run decides.
	if IsConcrete(args[0]) && IsConcrete(args[1]) {
		cmp, err := CompareValues(args[0], args[1])
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return nil, fmt.Errorf("between: %w", err)
		}
		if cmp > 0 {
			return []Value{NewTypeLiteral(TNever)}, nil
		}
	}
	noteRefinementConstruct(r, dep, args[0], args[1])
	return []Value{dep}, nil
}
