package core

// disjunctUnifier is the kernel-installed Unifier for user-defined
// disjunct types — types whose body is a (`tor`-joined) disjunction
// of alternatives such as `def Maybe (Integer tor none)`.
//
// Before this Unifier existed, `def Maybe (Integer tor none)` minted
// a lattice node parented at TDisjunct but left its Behavior as the
// kernel DefaultBehavior. DefaultBehavior.Match falls back to a plain
// lattice walk: it asks `v.Parent.ConformsTo(Maybe)`, which is false for
// every value (Integer's lattice path is Integer → Number → Scalar →
// Any, none of which is Maybe). So `42.Is(Maybe)` returned false at
// every dispatch site — fn sig matching, the `is` word, options
// fields, record fields — even though the language-level `unify` word
// (which goes through unifyDisjunct directly) accepted 42 happily.
//
// Like predicateUnifier, this lives on the lattice node so every
// "is v a Maybe?" path consults it.
type DisjunctUnifier struct {
	behaviorWrapper         // prev Behavior + Format/Equal/Compare delegation
	Alternatives    []Value // copy of the disjunct's alternatives
	typeName        string
}

// Match runs the disjunct alternatives against v via unifyDisjunct.
// The disjunct accepts v iff some alternative unifies with it. Type
// literals (Data==nil, !Carrier) pass through to the prev/DefaultBehavior
// walk — a bare Maybe-literal is "the type itself", not an inhabitant.
func (*DisjunctUnifier) ContentMembership() {}

func (d *DisjunctUnifier) Match(v Value, t *Type) bool {
	// Deliberately LOOSE on the newtype-alternative swap: `42` against
	// `(P tor String)` admits via the subtype rule even though
	// `42 is M` is false — dispatch decomposes the union to base
	// families, the one pinned boundary where a newtype alternative is
	// looser than `is` (edge-types-2's DIVERGENCE PIN). The `is` word
	// applies its post-unify value-identity check on top of this Match
	// (IsDisjunctTypeNode routes it past the short-circuit).
	return d.matchR(v, t, nil)
}

// matchR is Match with the enclosing unify chain's registry threaded
// into the alternatives walk (see isR).
func (d *DisjunctUnifier) matchR(v Value, t *Type, r *Registry) bool {
	// A TYPE literal is the type itself, not an inhabitant. The None literal
	// is not one: it is the value a missing member reads as, and a union
	// that names None admits it as it admits `none` (NUR324).
	if IsTypeLiteral(&v) {
		return baseBehavior(d.prev).Match(v, t)
	}
	_, err := unifyDisjunct(DisjunctInfo{Alternatives: d.Alternatives}, v, r)
	return err == nil
}

// IsDisjunctTypeNode reports whether v is a bare lattice node whose
// Behavior chain carries a DisjunctUnifier — the evaluated NAME of a
// disjunct type after the Stage 2 flip. The `is` handler uses it to
// route a concrete candidate through the full Unify + value-identity
// path (where the newtype-alternative swap is declined) instead of the
// Match short-circuit (which stays deliberately loose for dispatch).
func IsDisjunctTypeNode(v Value) bool {
	if !IsBareTypeNode(&v) {
		return false
	}
	u, ok := unifierOf(v.Behavior())
	if !ok {
		return false
	}
	_, ok = u.(*DisjunctUnifier)
	return ok
}

// Unify admits a concrete candidate iff some alternative unifies with
// it, yielding the alternative-unified result — never the node literal
// (the typed-def swap hazard). A concrete non-member fails
// definitively; a type-level pair defers to the structural rule. The
// Unify capability every membership kind carries
// (design/legacy/TYPE-REPRESENTATION.1.ignore §N3), so `def x:Maybe 5` against the
// node constraint runs the alternatives.
func (d *DisjunctUnifier) Unify(a, b Value, r *Registry) (Value, *UnifyError) {
	// The alternatives rule decides whenever exactly one side IS this
	// disjunct's node; the candidate may be concrete, a carrier, or a
	// TYPE-level operand (`OptNum unify None` — None is an
	// alternative), all of which the payload fold decided identically
	// for the body the node replaced. A same-node (or node-free) pair
	// settles structurally.
	aSelf := IsBareTypeNode(&a) && a.Behavior() == TypeBehavior(d)
	bSelf := IsBareTypeNode(&b) && b.Behavior() == TypeBehavior(d)
	if aSelf == bSelf {
		return unifySameOrSubtype(a, b)
	}
	candidate := a
	if aSelf {
		candidate = b
	}
	return unifyDisjunct(DisjunctInfo{Alternatives: d.Alternatives}, candidate, r)
}

// installDisjunctUnifier attaches a disjunctUnifier to def, wrapping
// any existing Behavior. Called by InstallType when minting a disjunct
// type so the alternatives drive every Is/Match call site.
func installDisjunctUnifier(def *Type, alternatives []Value, name string) {
	def.ensureTMeta().Behavior = &DisjunctUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		Alternatives:    alternatives,
		typeName:        name,
	}
}

// unifyDisjunct is the meet of a disjunct with a value. ONE rule: the
// meet is the disjunct of the meets of its alternatives with that value
// — the set intersection, and what `tand` answers for the same pair.
//
//   - The EMPTY disjunct denotes the empty set, so it IS `Never`: the
//     pair is settled as `Never` against the other side (which admits
//     only `Never` — and so another empty disjunct).
//   - Against `Any` the disjunct stands whole.
//   - Against another DISJUNCT, or against a TYPE — anything that is not
//     a value through and through (isPlainValue) — every alternative's
//     meet is gathered (unifyDisjunctMeets): `5 tor 6` against `Integer`
//     is `5 tor 6`.
//   - Against a VALUE every alternative that admits it answers that same
//     value, so the walk stops at the first. That is the rule above, not
//     an exception to it — and it is what every membership test pays
//     for.
//
// A map alternative is an open PATTERN, not a full schema: it admits a
// map carrying its keys (openUnifyMap), narrows a map shape that carries
// them (narrowMapShape), and meets another pattern as the two merged
// (mergeMapPatterns).
//
// Asymmetric by design: disj is always the disjunct side, val is the
// other side. The top dispatcher in unify.go handles the swap. r is the
// enclosing chain's registry, threaded into every alternative's walk.
func unifyDisjunct(disj DisjunctInfo, val Value, r *Registry) (Value, *UnifyError) {
	if len(disj.Alternatives) == 0 {
		return unifyInner(NewTypeLiteral(TNever), val, r)
	}

	// "any" unifies with the whole disjunct, preserving it. Covers
	// two value shapes: the bare type literal NewTypeLiteral(TAny)
	// (Data=nil; the value IS the TAny lattice node) and the Any-
	// typed carrier (Data=nil, Carrier=true, Parent=TAny). Shape asks
	// the node for a bare literal and the Parent for a carrier; probing
	// Parent() on both made every ROOT literal — Scalar, Node, Ideal,
	// Word, Type, whose lattice parent is Any — pass for Any here, so
	// `d( 'a' )` unified with `Node` (found by spec/tools/diff-fuzz.py).
	if !IsConcrete(&val) && Shape(val) == ShapeAny {
		return NewDisjunct(disj.Alternatives), nil
	}

	if IsDisjunct(&val) {
		other, _ := AsDisjunct(val)
		return unifyDisjunctMeets(disj.Alternatives, other.Alternatives, mergeMapPatterns, r)
	}
	if !isPlainValue(val) {
		return unifyDisjunctMeets(disj.Alternatives, []Value{val}, narrowMapShape, r)
	}

	for _, alt := range disj.Alternatives {
		// Concrete map alternative against a concrete map value uses
		// open (subset) matching — the disjunct alternative acts as a
		// pattern, not a full schema.
		if alt.Parent().Equal(TMap) && val.Parent().Equal(TMap) &&
			!IsRecordType(alt) && !IsRecordType(val) &&
			!IsTypedMap(&alt) && !IsTypedMap(&val) &&
			!IsOptionsType(alt) && !IsOptionsType(val) {
			if alt.Data() != nil && val.Data() != nil {
				if openUnifyMap(alt, val, r) {
					return val, nil
				}
				continue
			}
		}
		if unified, err := unifyInner(alt, val, r); err == nil {
			return unified, nil
		}
	}
	return Value{}, unifyFail("no disjunct alternative matched", NewDisjunct(disj.Alternatives), val)
}

// isPlainValue reports whether v is a VALUE through and through:
// concrete, not itself a type (a typed container, a disjunct, a function,
// a shape body — IsTypeBody), and, for a list or a map, holding only such
// values. `[1 2]` is one; `[Integer]`, `[:Integer]` and a check-mode
// carrier are not. DeepConcrete asks a different question — is there a
// payload to read at every level — and answers yes for a typed container.
func isPlainValue(v Value) bool {
	if !IsConcrete(&v) || IsTypeBody(&v) {
		return false
	}
	switch d := v.Data().(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if !isPlainValue(e) {
				return false
			}
		}
	case MapPayload:
		// A Go-API-built map with no OrderedMap has nothing to read.
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			if mv, _ := d.M.Get(k); !isPlainValue(mv) {
				return false
			}
		}
	}
	return true
}

// unifyDisjunctMeets is the meet of a disjunct's alternatives with a set
// of others: every alternative against every other, the meets that exist
// gathered into one canonical disjunct (SimplifyDisjunctAlts — a repeat
// or an absorbed alternative dropped, the rest in the total order). No
// pair unifying is a failure; exactly one surviving meet is that value,
// not a one-alternative disjunct. The others are a second disjunct's
// alternatives, or the one type standing against the disjunct; mapMeet is
// how a map alternative meets a map among them.
//
// Before this rule both shapes fell to the first-alternative walk
// (NUR386): `Integer tor String` unified with ITSELF was `Integer`, and
// `Scalar` unified with it was `Integer` too.
func unifyDisjunctMeets(alts, others []Value, mapMeet func(alt, other Value, r *Registry) (Value, bool), r *Registry) (Value, *UnifyError) {
	var meets []Value
	for _, alt := range alts {
		for _, other := range others {
			if isPlainConcreteMap(alt) && isPlainConcreteMap(other) {
				if meet, ok := mapMeet(alt, other, r); ok {
					meets = append(meets, meet)
				}
				continue
			}
			if meet, err := unifyInner(alt, other, r); err == nil {
				meets = append(meets, FlattenDisjunctAlts(meet)...)
			}
		}
	}
	meets = SimplifyDisjunctAlts(meets)
	switch len(meets) {
	case 0:
		return Value{}, unifyFail("no disjunct alternative matched", NewDisjunct(alts), NewDisjunct(others))
	case 1:
		return meets[0], nil
	default:
		return NewDisjunct(meets), nil
	}
}

// narrowMapShape is a map ALTERNATIVE against a map SHAPE — a map that
// is not a value through and through, `{a:Integer}`. The alternative is
// an open pattern and the shape is closed, so the test is the one a map
// value takes (openUnifyMap): every key the pattern names must unify with
// the shape's, a key the shape lacks standing as `Absent`. What differs
// is the answer. A value is already as narrow as it can be; a shape is
// not, so each key the pattern names is narrowed to the meet of the two:
// `{a:1}` against `{a:Integer b:String}` is `{a:1 b:String}`.
func narrowMapShape(pattern, shape Value, r *Registry) (Value, bool) {
	pm, _ := AsMap(pattern)
	sm, _ := AsMap(shape)
	narrowed := NewOrderedMap()
	for _, key := range sm.Keys() {
		val, _ := sm.Get(key)
		narrowed.Set(key, val)
	}
	for _, key := range pm.Keys() {
		want, _ := pm.Get(key)
		have, present := sm.Get(key)
		if !present {
			have = NewTypeLiteral(TAbsent)
		}
		meet, err := unifyWithin(want, have, r)
		if err != nil {
			return Value{}, false
		}
		if present {
			narrowed.Set(key, meet)
		}
	}
	return NewMap(narrowed), true
}

// mergeMapPatterns is the meet of two map ALTERNATIVES. A map
// alternative is an open pattern — it admits any map carrying its keys
// (unifyDisjunct's subset match) — so the maps BOTH admit are the ones
// carrying the keys of either: the meet is the pattern holding every
// key, a key both name holding the meet of its two values. Those values
// unify as openUnifyMap unifies them (closed below the first level), so
// one pattern against another is decided exactly as each is against a
// value. Keys keep the left pattern's order, then the right's extras.
//
// The one-sided subset match cannot serve here: `{a:1}` admits
// `{a:1 b:2}` but not the reverse, and which of two patterns is "the
// pattern" is only an accident of operand order.
func mergeMapPatterns(a, b Value, r *Registry) (Value, bool) {
	am, _ := AsMap(a)
	bm, _ := AsMap(b)
	merged := NewOrderedMap()
	for _, key := range am.Keys() {
		val, _ := am.Get(key)
		if other, shared := bm.Get(key); shared {
			meet, err := unifyWithin(val, other, r)
			if err != nil {
				return Value{}, false
			}
			val = meet
		}
		merged.Set(key, val)
	}
	for _, key := range bm.Keys() {
		if _, shared := am.Get(key); !shared {
			val, _ := bm.Get(key)
			merged.Set(key, val)
		}
	}
	return NewMap(merged), true
}
