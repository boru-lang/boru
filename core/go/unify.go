package core

// Unify is the kernel's structural unifier — the intersection of two
// values in the lattice. Returns the unified value and true on
// success, or (Value{}, false) on failure.
//
// For callers that need a structured failure (which field, which
// element, what reason), use UnifyExplain.
//
// Dispatch model (this file):
//
//  1. ResolveWordsDeep preprocesses both sides so bare words inside
//     list/map literals participate as their semantic values.
//  2. Shape() classifies each side into one ValueShape.
//  3. The top dispatcher routes to a family handler keyed by the
//     "ruling" shape. Special degenerate roots (Never, None, Any) and
//     the Disjunct fold are handled inline; everything else routes to
//     a per-family file (unify_list.go, unify_map.go, unify_disjunct.go,
//     unify_dep.go, unify_fnsig.go).
//
// Each family handler receives both sides plus their shapes, and is
// responsible for canonicalizing the asymmetric arms (type-literal vs
// concrete, typed vs untyped, etc.) so the per-pair logic appears
// exactly once instead of mirrored.
//
// The narrowing fall-through (same type → ValuesEqual, subtype → take
// the narrower) lives at the end of this file: unifySameOrSubtype.
func Unify(a, b Value) (Value, bool) {
	v, err := UnifyExplain(a, b)
	return v, err == nil
}

// UnifyExplain is the structured-error counterpart to Unify. Returns
// (unified, nil) on success or (Value{}, *UnifyError) describing the
// failure path on mismatch. Use this when the caller needs to report
// which field/element failed (record field unification, options
// matching, `make` constraint checking, the lang-level `unify` word).
func UnifyExplain(a, b Value) (Value, *UnifyError) {
	a = ResolveWordsDeep(a)
	b = ResolveWordsDeep(b)
	return unifyInner(a, b, nil)
}

// UnifyR is Unify with a Registry to enable predicate-FnDef
// constraint evaluation. When one side is a predicate FnDef (or a
// Disjunct/ChildType-typed-collection containing one) and a Registry
// is provided, RunPredicate handles the matching. Without a Registry,
// behaves identically to Unify.
//
// Use this from sites that already have a Registry in hand and may
// receive predicate-typed constraints — record-field check, options-
// field check, the lang-level `unify` word.
func UnifyR(a, b Value, r *Registry) (Value, bool) {
	v, err := UnifyExplainR(a, b, r)
	return v, err == nil
}

// UnifyExplainR — see UnifyR. Returns structured failure.
//
// r travels EXPLICITLY through the whole unify: unifyInner takes it,
// and every family handler, fold, Unifier and helper that recurses
// back into unifyInner passes it on, so a nested step (list element,
// map field, disjunct alternative, record field, a Unifier's own
// re-entry) consults exactly the registry of the UnifyExplainR chain
// it belongs to. There is deliberately NO package-level state here:
// the process runs many engines on many goroutines (timer and
// interval bodies, the net acceptor's per-connection forks, spawned
// forks), and UnifyExplainR sits on hot paths in all of them, so an
// ambient stack would race (unify_race_test.go pins that).
//
// The chain ends where the ENGINE begins. When a step re-enters the
// engine — a predicate body run by PredicateUnifier through
// RunPredicate, a `behave unify` body — the code that body executes
// (its dispatch, its typed defs, its own `is`) starts unarmed, exactly
// as the same code does at top level; the body's registry is not the
// chain's r. The old ambient stack armed those re-entries by accident,
// from whichever UnifyExplainR happened to be in flight on ANY
// goroutine, so `[1 2] is Wrap` could admit inside a predicate body a
// call `(chk [1 2])` declined at top level. lang's
// TestPredicateBodyDispatchIsUnarmedLikeTopLevel pins the consistent
// rule on both engines.
func UnifyExplainR(a, b Value, r *Registry) (Value, *UnifyError) {
	// Registry-armed prepass (resolve.go): user-typed words inside
	// patterns and typed-container children (`[:Foo]`) resolve to
	// their bound bodies instead of degrading to Atoms (NUR060).
	a = ResolveWordsDeepR(a, r)
	b = ResolveWordsDeepR(b, r)
	return unifyInner(a, b, r)
}

// unifyWithin is the plain-Unify entry for a call made from INSIDE an
// in-flight unify — a Unifier's body check, the open-map subset walk a
// disjunct alternative runs, a bound check. It runs the same unarmed
// word-resolution prepass UnifyExplain runs, but keeps the registry of
// the enclosing chain (nil when the chain started at UnifyExplain /
// Unify) instead of dropping it at the boundary, so a nested typed
// container or predicate reference is decided exactly as it would be
// at the chain's top level.
func unifyWithin(a, b Value, r *Registry) (Value, *UnifyError) {
	a = ResolveWordsDeep(a)
	b = ResolveWordsDeep(b)
	return unifyInner(a, b, r)
}

// registryMatcher is the registry-threaded twin of TypeBehavior.Match
// that the kernel membership Behaviors whose Match recurses into the
// unifier (disjunct, negation, binding-body, type-parameter) implement:
// their public Match is matchR with a nil registry. isR consults it so
// an Is-membership question asked from INSIDE an armed unify keeps the
// chain's registry for the nested walk.
type registryMatcher interface {
	matchR(v Value, t *Type, r *Registry) bool
}

// isR is Value.Is for a membership question asked from inside an
// in-flight unify. When the chain is armed and t's Behavior is one of
// the kernel Behaviors whose Match recurses into the unifier, the
// registry is threaded into that walk; otherwise it is exactly v.Is(t)
// — a wrapped or foreign Behavior is dispatched through Is unchanged.
func isR(v Value, t *Type, r *Registry) bool {
	if r != nil && t != nil {
		if m, ok := t.Behavior().(registryMatcher); ok {
			return m.matchR(v, t, r)
		}
	}
	return v.Is(t)
}

// isPredicateFnValue reports whether v is a function value whose
// first signature has a single typed parameter — the shape a
// predicate type has.
// IsDeclaredPredicateFn reports whether v is a fn value whose author
// DECLARED it a membership test with `fnpred`. This is the explicit route
// into the predicate-type branch, and the one that does not consult the
// parameter count — ADR-016 forbids arity deciding how a function behaves,
// and isPredicateFnValue below does exactly that. NUR099 tracks retiring
// the arity route once the corpus has migrated to `fnpred`.
func IsDeclaredPredicateFn(v Value) bool {
	if v.Parent == nil || !v.Parent.Equal(TFunction) {
		return false
	}
	info, ok := v.Data.(FnDefInfo)
	return ok && info.Predicate
}

// MarkPredicateFn returns v with its fn payload marked a DECLARED predicate
// — `fnpred`'s half of the pair above. Total: a value carrying no FnDefInfo
// comes back unchanged, so a caller never has to guard the payload shape.
func MarkPredicateFn(v Value) Value {
	info, ok := v.Data.(FnDefInfo)
	if !ok {
		return v
	}
	info.Predicate = true
	return NewValueRaw(v.Parent, info)
}

// isPredicateFnValue reports whether v LOOKS like a predicate because it
// takes one parameter.
//
// DEPRECATED ROUTE (NUR099, NUR100). Routing on the parameter count is an
// arity-keyed exception, which ADR-016 forbids outright: it is why the same
// fn body means a callable function under a lowercase name and a membership
// test under a capitalised one, and why `def K fn [[a:Any b:Any]…]` binds a
// type nothing can inhabit instead of being declined. `fnpred` is the
// replacement (IsDeclaredPredicateFn); this stays only until the corpus has
// migrated, and is not to be extended.
func isPredicateFnValue(v Value) bool {
	if v.Parent == nil {
		return false
	}
	if !v.Parent.Equal(TFunction) {
		return false
	}
	info, ok := v.Data.(FnDefInfo)
	if !ok {
		return false
	}
	sig, ok := info.FirstOwnSig()
	if !ok {
		return false
	}
	return len(sig.Params) == 1
}

// resolvePredicateRef returns the predicate type's lattice NODE when v
// references a predicate type via name AND the type's Behavior is the
// predicateUnifier installed by InstallType. The Behavior check is
// what distinguishes a predicate TYPE from an ordinary 1-arg fn
// value — without it, every 1-arg fn would look like a predicate and
// hijack standard unification (e.g. FnUndef variance checks).
//
// Returning the node (not the body) lets the unifyInner pre-pass
// route through the node's own predicateUnifier.Unify, so a predicate
// referenced by name/word/fn-body and one reached by lattice dispatch
// share ONE membership verdict (no separate run-the-body path).
//
// Accepts these reference shapes:
//   - Bare type literal of a named predicate type (Pos's *Type).
//   - Word naming a predicate-typed def (`Pos` in `[:Pos]`).
//   - Atom naming a predicate-typed def (`Pos` after quote/resolve).
//   - The predicate's FnDef body value (a record/options field
//     constraint stored as the fn, not the type literal).
func resolvePredicateRef(v Value, r *Registry) (*Type, bool) {
	if r == nil {
		return nil, false
	}
	var name string
	switch {
	case IsWord(v):
		w, _ := AsWord(v)
		name = w.Name
	case v.Parent != nil && v.Parent.ConformsTo(TAtom) && v.Data != nil:
		w, _ := AsAtom(v)
		name = w
	case IsBareTypeNode(v) && v.ID != "" && v.Name() != "":
		name = v.Name()
	case isPredicateFnValue(v):
		// Direct FnDef body — try the FnDef's Name field. Predicate
		// types installed via `def Pos fn […]` carry Name="Pos" on
		// their FnDef payload after InstallType wires the binding.
		if info, ok := v.Data.(FnDefInfo); ok {
			name = info.Name
		}
		if name == "" {
			// Anonymous fn — try reverse lookup: walk the def table
			// for a typed binding whose body is a copy of THE SAME fn
			// construction as v. This is what catches record-field
			// constraints stored as the FnDef value (not the *Type
			// literal). Identity is by shared construction (see
			// sameFnConstruction) — the previous value-ID comparison
			// (body.Equal(&v)) no longer works for runtime mints under
			// the mode-gated ID elision.
			for _, n := range r.Defs.Names() {
				body, ok := r.TopTypeBody(n)
				if !ok {
					continue
				}
				if sameFnConstruction(body, v) {
					name = n
					break
				}
			}
		}
	}
	if name == "" {
		return nil, false
	}
	def := r.LookupTypeName(name)
	if def == nil {
		return nil, false
	}
	if _, ok := def.Behavior().(*PredicateUnifier); !ok {
		return nil, false
	}
	return def, true
}

// sameFnConstruction reports whether a and b are by-value copies of ONE
// fn construction. FnDefInfo copies share the Signatures backing array,
// so the first sig's address is a construction identity that survives
// the mode-gated ID elision (runtime-minted fn VALUES carry no ID).
// Value-ID equality is kept as a secondary probe for pass-minted values
// whose payloads were rebuilt (aggregate clones re-slice Signatures).
func sameFnConstruction(a, b Value) bool {
	ai, aok := a.Data.(FnDefInfo)
	bi, bok := b.Data.(FnDefInfo)
	if !aok || !bok {
		return false
	}
	if len(ai.Signatures) > 0 && len(bi.Signatures) > 0 &&
		&ai.Signatures[0] == &bi.Signatures[0] {
		return true
	}
	return a.ID != "" && a.ID == b.ID
}

// disjunctHasPredicate reports whether any alternative in the
// disjunct is a predicate fn value.
func disjunctHasPredicate(disj DisjunctInfo) bool {
	for _, alt := range disj.Alternatives {
		if isPredicateFnValue(alt) {
			return true
		}
	}
	return false
}

// unifyResolvedPredicate routes a candidate through a resolved predicate
// type's own predicateUnifier.Unify — the SINGLE predicate-membership
// path, shared with lattice dispatch. The candidate is unified against
// the node's type literal, so predicateUnifier.Unify admits it iff the
// predicate body accepts (via the shared unifyMembership contract).
func unifyResolvedPredicate(def *Type, candidate Value, r *Registry) (Value, *UnifyError) {
	pu, ok := def.Behavior().(*PredicateUnifier)
	if !ok {
		return Value{}, unifyFail("not a predicate type", candidate, NewTypeLiteral(def))
	}
	return pu.Unify(candidate, NewTypeLiteral(def), r)
}

// unifyDisjunctR is the Registry-aware disjunct walk. Tries each
// alternative via unifyInner with r so predicate-fn alternatives are
// evaluated correctly.
func unifyDisjunctR(disj DisjunctInfo, val Value, r *Registry) (Value, *UnifyError) {
	if !IsConcrete(val) && (val.Parent.Equal(TAny) || (&val).Equal(TAny)) {
		return NewDisjunct(disj.Alternatives), nil
	}
	for _, alt := range disj.Alternatives {
		if unified, err := unifyInner(alt, val, r); err == nil {
			return unified, nil
		}
	}
	return Value{}, unifyFail("no disjunct alternative matched", NewDisjunct(disj.Alternatives), val)
}

// unifyInner is the post-resolution dispatcher. All recursive calls
// inside the family handlers use this entry so ResolveWordsDeep runs
// exactly once per top-level call.
//
// r is the registry of the enclosing UnifyExplainR chain — nil when
// the chain started at UnifyExplain / Unify — and every handler that
// recurses hands it back here unchanged, so one armed call arms the
// whole structural walk beneath it.
//
// Pre-pass: if the chain is armed and either operand is a
// predicate-fn constraint (or a disjunct containing one), route
// through RunPredicate so structural contexts — typed-list child,
// typed-map child, record field, disjunct alternative — honor
// predicate types without each handler needing to know about
// Registry.
func unifyInner(a, b Value, r *Registry) (Value, *UnifyError) {
	if r != nil {
		if def, ok := resolvePredicateRef(a, r); ok && b.Data != nil {
			return unifyResolvedPredicate(def, b, r)
		}
		if def, ok := resolvePredicateRef(b, r); ok && a.Data != nil {
			return unifyResolvedPredicate(def, a, r)
		}
		if IsDisjunct(a) {
			disj, _ := AsDisjunct(a)
			if disjunctHasPredicate(disj) {
				return unifyDisjunctR(disj, b, r)
			}
		}
		if IsDisjunct(b) {
			disj, _ := AsDisjunct(b)
			if disjunctHasPredicate(disj) {
				return unifyDisjunctR(disj, a, r)
			}
		}
	}
	sa := Shape(a)
	sb := Shape(b)

	// Compound-type and degenerate-root folds, in priority order (see
	// unifyFolds). When a side is a rule's "ruling" shape the pair routes
	// to that rule's handler with the ruling side first; the asymmetric
	// handlers canonicalise internally, so a both-sides or swapped match
	// is immaterial. The TABLE ORDER is the dispatch precedence —
	// disjunct/negation/surface before the degenerate roots (so a
	// `String or None` disjunct folds before None's self-only rule),
	// object-type and type-parameter folds after. Checked before the
	// Behavior-driven LCA walk below; the bounded-type rule leads because
	// its ChildTypeInfo payload would otherwise be misread by a family
	// handler.
	// A membership NODE outranks the shape folds, mirroring the fold
	// precedence its BODY had: `OptNum unify None` must consult the
	// disjunct node's alternatives before None's self-only rule, exactly
	// as the disjunct fold preceded the None fold when the name
	// evaluated to the DisjunctInfo body (the Stage 2 flip,
	// design/legacy/TYPE-REPRESENTATION.1.ignore). One side only — a pair of
	// constraint nodes keeps the table order.
	aCons := IsBareTypeNode(a) && HasConstraintUnify(&a)
	bCons := IsBareTypeNode(b) && HasConstraintUnify(&b)
	if aCons != bCons {
		if v, err, hit := dispatchUnifier(a, b, r); hit {
			return v, err
		}
	}

	for i := range unifyFolds {
		f := &unifyFolds[i]
		if f.ruling(a, sa) {
			return f.fold(a, b, r)
		}
		if f.ruling(b, sb) {
			return f.fold(b, a, r)
		}
	}

	// Behavior-driven dispatch: walk the LCA of the two operand types
	// looking for a Unifier capability. The first non-opt-out Unifier
	// owns the result — same pattern CompareValues uses for Comparer.
	// Predicate types and refine-with-clause types auto-install
	// Unifiers (see core_type.go::InstallType) so narrowing into a
	// constrained type checks the constraint; external plugin types
	// and `behave unify/q` user installs also flow through here.
	if v, err, hit := dispatchUnifier(a, b, r); hit {
		return v, err
	}

	// Bare Node type literal: the abstract family root. A concrete
	// container would otherwise route to its family handler below,
	// which doesn't know the Node literal and would reject the pair —
	// breaking lattice transitivity (`{} is Map` is true and Map is a
	// child of Node, so `{} is Node` must be true too). Delegate to
	// the family handler with the corresponding family literal so the
	// per-family rules (Record/Table exclusions, Options projection)
	// stay in one place.
	aNodeLit := sa == ShapeTypeLiteral && denotedType(a).Equal(TNode)
	bNodeLit := sb == ShapeTypeLiteral && denotedType(b).Equal(TNode)
	if aNodeLit || bNodeLit {
		if aNodeLit && bNodeLit {
			return a, nil
		}
		other, so := b, sb
		if bNodeLit {
			other, so = a, sa
		}
		if IsMapShape(so) {
			return unifyMapFamily(NewTypeLiteral(TMap), ShapeTypeLiteral, other, so, r)
		}
		if IsListShape(so) {
			return unifyListFamily(NewTypeLiteral(TList), ShapeTypeLiteral, other, so, r)
		}
		// Node vs a narrower node-family type literal (Map, List,
		// FlexMap, FlexList, …) — the narrower literal wins.
		if so == ShapeTypeLiteral && denotedType(other).ConformsTo(TNode) {
			return other, nil
		}
		return Value{}, unifyFail("Node type literal needs a node-family value", a, b)
	}

	// Family handlers — any side in the family routes to that family's
	// owner, which canonicalizes argument order internally. A bare
	// type literal whose denoted type is List/Map also routes to the
	// corresponding family (e.g. `List unify [1,2]`).
	// nodeFamily folds the Flex literals into their family: a bare
	// `FlexList` / `FlexMap` literal routes to the same family handler,
	// which applies the nominal-subtype rule internally.
	aListLit := sa == ShapeTypeLiteral && nodeFamily(denotedType(a)).Equal(TList)
	bListLit := sb == ShapeTypeLiteral && nodeFamily(denotedType(b)).Equal(TList)
	if IsListShape(sa) || IsListShape(sb) || aListLit || bListLit {
		return unifyListFamily(a, sa, b, sb, r)
	}
	aMapLit := sa == ShapeTypeLiteral && nodeFamily(denotedType(a)).Equal(TMap)
	bMapLit := sb == ShapeTypeLiteral && nodeFamily(denotedType(b)).Equal(TMap)
	if IsMapShape(sa) || IsMapShape(sb) || aMapLit || bMapLit {
		return unifyMapFamily(a, sa, b, sb, r)
	}
	if sa == ShapeDepScalar || sb == ShapeDepScalar {
		return unifyDepScalar(a, sa, b, sb)
	}
	if sa == ShapeFnUndef || sb == ShapeFnUndef {
		return unifyFnUndefShape(a, sa, b, sb, r)
	}

	// General narrowing: type-literal-vs-concrete, same-type literal
	// compare, subtype relation. Handled together because they're all
	// just "pick the narrower side if compatible".
	return unifySameOrSubtype(a, b)
}

// unifyFold is one entry in the compound-type dispatch table consulted
// near the top of unifyInner. ruling reports whether a side is this
// rule's governing shape (by ValueShape, or by a payload predicate the
// Shape enum doesn't name — bounded type, surface, type parameter);
// fold receives the ruling side first, the other side second, and the
// registry of the enclosing chain (nil when unarmed) for the folds
// that recurse into unifyInner.
type unifyFold struct {
	ruling func(Value, ValueShape) bool
	fold   func(ruling, other Value, r *Registry) (Value, *UnifyError)
}

// unifyFolds is the priority-ordered fold table. The order reproduces
// the historical if-chain exactly: bounded type leads (its payload
// must not reach a family handler), then the union/complement/surface
// compound folds, then the degenerate roots, then the object-type and
// type-parameter folds. Each fold's rationale lives on its handler.
//
// Populated in init() rather than as a var initializer: the fold
// handlers call back into Unify → unifyInner, which reads this table,
// so a direct var initializer trips Go's initialization-cycle check.
// init() assigns after all package vars are set, breaking the static
// cycle while the runtime call graph is unchanged.
var unifyFolds []unifyFold

func init() {
	unifyFolds = []unifyFold{
		{func(v Value, _ ValueShape) bool { return IsBoundedType(v) }, unifyBoundedType},
		{func(_ Value, s ValueShape) bool { return s == ShapeDisjunct }, func(ruling, other Value, r *Registry) (Value, *UnifyError) {
			disj, _ := AsDisjunct(ruling)
			return unifyDisjunct(disj, other, r)
		}},
		{func(_ Value, s ValueShape) bool { return s == ShapeNegation }, func(ruling, other Value, r *Registry) (Value, *UnifyError) {
			neg, _ := AsNegation(ruling)
			return unifyNegation(neg, other, r)
		}},
		{func(v Value, _ ValueShape) bool { return IsSurfaceType(v) }, func(ruling, other Value, _ *Registry) (Value, *UnifyError) {
			return unifySurface(ruling, other)
		}},
		{func(_ Value, s ValueShape) bool { return s == ShapeNever }, foldDegenRoot("never", ShapeNever)},
		{func(_ Value, s ValueShape) bool { return s == ShapeNone }, foldNoneRoot},
		{func(_ Value, s ValueShape) bool { return s == ShapeAbsent }, foldDegenRoot("absent", ShapeAbsent)},
		{func(_ Value, s ValueShape) bool { return s == ShapeAny }, func(_, other Value, _ *Registry) (Value, *UnifyError) {
			// Any yields the other (more specific) side.
			return other, nil
		}},
		{func(v Value, _ ValueShape) bool { return IsClassType(v) }, unifyObjectType},
		{func(v Value, _ ValueShape) bool { return typeParamLitNode(v) != nil }, func(ruling, other Value, r *Registry) (Value, *UnifyError) {
			return unifyTypeParam(ruling, typeParamLitNode(ruling), other, r)
		}},
	}
}

// foldNoneRoot is the None root's fold: none unifies with none — and
// with the Any side of a pair (NUR069). `Any` is the top of the value
// domain: `TNone.ConformsTo(TAny)` already holds, `none is Any` is
// true, and `make`'s bare-literal field rule admits none for an `Any`
// constraint (MakeFieldValueR's ConformsTo arm) — the unifier was the
// one boundary that read `Any` as "anything except none", which split
// a record's declared-`Any` field between construction (admitted) and
// every pattern walk (declined). The intersection is the none side (the
// narrower). Stated here, not in the Any fold, because None outranks
// Any in the fold order; Never and Absent keep their self-only rule —
// in particular the `?:T` optional-key machinery depends on
// `Unify(Any, Absent)` declining (an `Any` field is required, not
// optional).
func foldNoneRoot(ruling, other Value, _ *Registry) (Value, *UnifyError) {
	if Shape(other) == ShapeAny {
		return ruling, nil
	}
	return foldDegenRoot("none", ShapeNone)(ruling, other, nil)
}

// foldDegenRoot builds the self-only fold a degenerate root (Never,
// None, Absent) uses: the pair unifies iff the other side is the same
// root, otherwise it is a definitive mismatch. The ruling side is
// returned on success (both-root pairs yield the first-checked side,
// matching the prior `if sa == sb { return a }` rule).
func foldDegenRoot(name string, root ValueShape) func(Value, Value, *Registry) (Value, *UnifyError) {
	return func(ruling, other Value, _ *Registry) (Value, *UnifyError) {
		if Shape(other) != root {
			return Value{}, unifyFail(name+" only unifies with "+name, ruling, other)
		}
		// Both sides denote this root. Return the concrete inhabitant when
		// exactly one side is concrete — matching the general type-literal-
		// vs-value convention (`unify Integer 5 → 5`, not `→ Integer`). The
		// value `none` unified against the `None` type literal must yield
		// `none`, so a membership test whose winning disjunct alternative is
		// the bare `None` literal (`none is (Integer tor None)`) sees a
		// result whose Parent matches the tested value rather than the None
		// root — without this the `is` handler's post-unify parent-equality
		// check wrongly rejected the match. Two literals (or two values)
		// keep the prior first-checked (`ruling`) side.
		if IsConcrete(other) && !IsConcrete(ruling) {
			return other, nil
		}
		return ruling, nil
	}
}

// unifyObjectType admits the non-ObjectType side by Is-membership in
// the object type's minted node (see the fold in unifyInner). Two
// object types unify only when they are the same type (nominal —
// matching the Record/Options exclusivity rules). r is the enclosing
// chain's registry, threaded into the membership walk (isR).
func unifyObjectType(a, b Value, r *Registry) (Value, *UnifyError) {
	ot, other := a, b
	if !IsClassType(ot) {
		ot, other = b, a
	}
	oi, err := AsClassType(ot)
	if err != nil || oi.Type == nil {
		return Value{}, unifyFail("object type has no minted node (declare it with def)", a, b)
	}
	if IsClassType(other) {
		ooi, oerr := AsClassType(other)
		if oerr == nil && ooi.ID == oi.ID {
			return ot, nil
		}
		return Value{}, unifyFail("distinct object types do not unify", a, b)
	}
	// A bare node literal: the class node itself (or a subclass node).
	if IsBareTypeNode(other) {
		if (&other).ConformsTo(oi.Type) {
			return other, nil
		}
		return Value{}, unifyFail("type does not conform to the object type", a, b)
	}
	// Concrete instance or check-mode carrier: Is-membership via the
	// node's Behavior (subclass instances conform by ancestry).
	if isR(other, oi.Type, r) {
		return other, nil
	}
	return Value{}, unifyFail("value is not an instance of the object type", a, b)
}

// unifySameOrSubtype is the general scalar-narrowing fall-through. By
// the time we reach here both sides are non-list, non-map, non-disjunct,
// non-depscalar, non-fnundef — so it's just type-literal vs concrete
// or two values along the same scalar lattice chain.
func unifySameOrSubtype(a, b Value) (Value, *UnifyError) {
	aType := denotedType(a)
	bType := denotedType(b)

	// Type literal unifies with any concrete whose type matches.
	if IsBareTypeNode(a) && b.Data != nil && bType.ConformsTo(aType) {
		return b, nil
	}
	if IsBareTypeNode(b) && a.Data != nil && aType.ConformsTo(bType) {
		return a, nil
	}

	// Same type → compare literal values.
	if aType.Equal(bType) {
		if ValuesEqual(a, b) {
			return a, nil
		}
		return Value{}, unifyFail("same type, different literal values", a, b)
	}

	// Subtype relation → narrower side wins.
	if aType.IsSubtypeOf(bType) {
		return a, nil
	}
	if bType.IsSubtypeOf(aType) {
		return b, nil
	}

	return Value{}, unifyFail("incompatible types", a, b)
}

// denotedType returns the lattice type the value denotes. For a bare
// type literal the value IS the lattice node; for a carrier or
// concrete value it is the Parent. A Data==nil value with an empty ID
// is a manually-constructed `Value{Parent: T}` (used in tests as a
// stand-in for a value of type T); treat its Parent as the denoted
// type since &v has no lattice identity to compare against.
func denotedType(v Value) *Type {
	if IsBareTypeNode(v) && v.ID != "" {
		return &v
	}
	return v.Parent
}
