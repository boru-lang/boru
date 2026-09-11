package core

import (
	"strings"
)

// This file owns the algorithms behind the type-system words —
// PathOf, TypeOf, IsRecordShape, IsValueOfType, InstallType — plus
// the helper rules that `is` / `typeof` / `pathof` / `enum` build on.
// The matching word registrations live in lang/go/engine/native_type.go;
// the engspec spec-runner installs minimal kernel-side fixtures.

// registerCoreTypeof installs `typeof v`. Returns a Type literal —
// the type of v, expressed as a value:
//
//	typeof 5            → Integer        (concrete value's exact type)
//	typeof 5.0          → Float
//	typeof "x"          → String
//	typeof true         → Boolean
//	typeof none         → None           (None has a single inhabitant)
//	typeof [ 1 2 ]      → List
//	typeof { a:1 }      → Map
//	typeof Integer      → Type            (ANY type literal → Type)
//	typeof List         → Type
//	typeof Any          → Type
//
// Rules:
//   - For `none` (the unique inhabitant of None), return None itself.
//   - For any other type literal (Data == nil), return `Type`.
//     Metatypes are collapsed — there is no ScalarType / NodeType /
//     ObjectType layer; the type-of-a-type-literal is uniformly `Type`.
//   - For a concrete value (Data != nil), return its full Parent.
//
// typeof's result is itself a type literal so it round-trips: passing
// the result to `is` or chaining `typeof typeof v` (always `Type`)
// produces the expected answers.
// PathOf returns the ancestry path of the type T (a Type literal, or
// any value whose Parent is a Type subtype — e.g. a Function/Disjunct
// value) as a List of Type literals, root first, leaf last (the
// declared signature contract is [:Type]; the runtime value is a
// regular List so that `is [literal-list]` comparisons against an
// untyped list template work):
//
//	pathof Integer          → [Scalar Number Integer]
//	pathof ProperString     → [Scalar String ProperString]
//	pathof List             → [Node List]
//	pathof Function         → [Type Function]
//	pathof Enum             → [Type Disjunct Enum]
//	pathof Type             → [Type]              (Type has no ancestors)
//	pathof None             → [None]
//
// Exported so lang's `pathof` registration (lang/go/engine/native_type.go)
// can wire dispatch into it without forking the algorithm.
func PathOf(t Value) Value {
	// Walk the ancestry root-first. A bare type literal IS its leaf
	// node, so start from t itself; any other value (a Function /
	// Disjunct / Enum value) contributes the ancestry of its type.
	start := t.Parent
	if IsBareTypeNode(t) {
		start = &t
	}
	var chain []*Type
	for d := start; d != nil; d = d.Parent {
		// Any is the universal lattice top; skip it as an ANCESTOR
		// so paths stay [Scalar Number Integer], not [Any Scalar
		// Number Integer]. When `pathof Any` is called directly the
		// chain is still [Any] — the skip only triggers once we've
		// already accumulated a leaf.
		if d.Equal(TAny) && len(chain) > 0 {
			break
		}
		chain = append([]*Type{d}, chain...)
	}
	elems := make([]Value, 0, len(chain))
	for _, d := range chain {
		elems = append(elems, NewTypeLiteral(d))
	}
	return NewList(elems)
}

// TypeOf returns the type of v — uniformly its Parent, expressed as
// a type-literal Value. After the type/value merge every value is a
// lattice node, so typeof is a single Parent hop, climbing the
// unified lattice that has Any at the top of the main hierarchy:
//
//	typeof 5        → Integer
//	typeof Integer  → Number
//	typeof Number   → Scalar
//	typeof Scalar   → Any        (Scalar's lattice parent is Any)
//	typeof Any      → Any        (saturates — top of the main hierarchy)
//	typeof none     → None       (none is None's sole inhabitant)
//	typeof None     → None       (None is a degenerate root — saturates)
//	typeof Never    → Never      (Never is a degenerate root — saturates)
func TypeOf(v Value) Value {
	if v.Parent == nil {
		return v
	}
	return NewTypeLiteral(v.Parent)
}

// IsRecordShape reports whether v is a non-empty map all of whose
// field values are themselves type bodies (type literals or nested
// record shapes). Independent of how the map was constructed
// (production boru `{x:Integer}` produces an explicit OrderedMap;
// the implicit-pair syntax inside fn signatures produces an Implicit
// map; both are treated as record shapes here when their values are
// type-shape values).
//
// The empty map `{}` is treated as a concrete value, not a shape,
// so `typeof { } → Map`. A mixed-content map like `{x:1 y:String}`
// has a concrete x payload and so is also NOT a record shape (typeof
// returns Map). Singleton-typed shapes still go via `is`'s structural
// unification path.
func IsRecordShape(v Value) bool {
	if !v.Parent.Equal(TMap) || !IsConcrete(v) {
		return false
	}
	m, _ := AsMap(v)
	if m == nil || m.Len() == 0 {
		return false
	}
	for _, k := range m.Keys() {
		fv, _ := m.Get(k)
		if IsBareTypeNode(fv) {
			continue // type literal (or None type literal)
		}
		if IsRecordShape(fv) {
			continue // nested shape
		}
		return false
	}
	return true
}

// IsValueOfType reports whether v satisfies type T. The canonical
// implementation of structural-conformance type-checking. Called by:
//   - the engspec test runner's kernel-level `is` word
//     (test/go/engspec/engspec_test.go) — the engine's `is` answers
//     "is v a T?" via this function directly.
//   - the production `enum` constructor (lang/go/native/native_type.go)
//     to validate that each element satisfies a typed-list child
//     constraint.
//   - the production `is` handler — for structural-pattern RHS
//     (typed list / typed map / record shape). Other RHS shapes
//     (Object/Table refinements, type literals, etc.) go through
//     paths in the production handler that add tag-identity
//     semantics on top.
//
// Rules:
//   - T is a typed list `[:T]`: v must be a concrete list and every
//     element must satisfy T (recursive IsValueOfType).
//   - T is a typed map `{:T}`: v must be a concrete map and every
//     value must satisfy T.
//   - T is a record-shape implicit map (`{x:Integer y:String}`):
//     every declared key must be present in v with a matching field
//     type. v must be a concrete map; extra keys in v are ignored.
//   - T is the bare metatype `Type` (Data == nil, Parent == Type): true
//     iff v is itself a type — any bare type literal, any structural
//     type body (record shape, typed list/map, disjunct, fn-shape),
//     or any Function / FunctionSignature / Disjunct / Enum value.
//     Concrete scalars / lists / maps and the value `none` are not.
//   - T is any other type literal (Data == nil): consult Behavior via
//     v.Is(&t) — DefaultBehavior is lattice subtyping, and per-kind
//     Unifiers (predicate, disjunct, dep-scalar, bare-refine) take
//     over for user-defined refinements.
//   - T is anything else: structural unification on (v, t).
func IsValueOfType(v, t Value) bool {
	// A NAMED type in the type slot evaluates to its minted node (the
	// Stage 2 flip); this predicate has always decided membership
	// against the declared content, which the node records
	// (design/TYPE-REPRESENTATION.1.md §N2). Catch-all bindings
	// (BindingBodyUnifier — record shapes, singletons, typed-container
	// literals) resolve to their body so the shape branches below keep
	// their subset semantics; kinds whose membership lives in a kernel
	// constraint Unifier (predicate / DepScalar / disjunct / negation /
	// FnUndef / surface) keep the node — the Unify fallback consults
	// the node's Behavior.
	if IsBareTypeNode(t) {
		resolve := !HasConstraintUnify(&t)
		if !resolve {
			if u, ok := unifierOf(t.Behavior()); ok {
				_, resolve = u.(*BindingBodyUnifier)
			}
		}
		if resolve {
			if content, ok := TypeContentOf(t); ok {
				t = content
			}
		}
	}
	if IsTypedList(t) {
		if !v.Parent.Equal(TList) || !IsConcrete(v) {
			return false
		}
		ci, _ := AsChildType(t)
		lst, _ := AsList(v)
		if lst.IsNil() {
			return false
		}
		for i := 0; i < lst.Len(); i++ {
			if !IsValueOfType(lst.Get(i), ci.Child) {
				return false
			}
		}
		return true
	}
	if IsTypedMap(t) {
		if !v.Parent.Equal(TMap) || !IsConcrete(v) {
			return false
		}
		ci, _ := AsChildType(t)
		vMap, _ := AsMap(v)
		if vMap == nil {
			return false
		}
		for _, k := range vMap.Keys() {
			vv, _ := vMap.Get(k)
			if !IsValueOfType(vv, ci.Child) {
				return false
			}
		}
		return true
	}
	// Map-as-type — record-shape conformance. Fires for both
	// Implicit (fn-sig pair-syntax) and explicit (`{x:Integer}`)
	// maps. The recursive IsValueOfType handles concrete-as-singleton
	// fields via the Unify fallback when t's field is a literal.
	// Subtypes like RecordTypeInfo / OptionsTypeInfo (whose AsMap
	// returns nil) fall through to Unify below.
	if _tMap, _tErr := AsMap(t); t.Parent.Equal(TMap) && t.Data != nil && _tErr == nil && _tMap != nil {
		if !v.Parent.Equal(TMap) || !IsConcrete(v) {
			return false
		}
		vMap, _ := AsMap(v)
		tMap, _ := AsMap(t)
		if vMap == nil || tMap == nil {
			return false
		}
		for _, k := range tMap.Keys() {
			tv, _ := tMap.Get(k)
			vv, ok := vMap.Get(k)
			if !ok {
				return false
			}
			if !IsValueOfType(vv, tv) {
				return false
			}
		}
		return true
	}
	if IsBareTypeNode(t) {
		// `v is Type` — the bare metatype: v satisfies it iff v is
		// itself a TYPE — not merely a value whose type would qualify:
		//
		//   - any bare type literal (`Integer`, `List`, `Any`, `Type`,
		//     …, Data == nil) — `Integer is Type`, `List is Type`, … are
		//     all true;
		//   - any structural type body (record shape `{x:Integer}`,
		//     typed list/map `[:T]` / `{:T}`, disjunct, fn-shape, …) and
		//     any Function / Disjunct / Enum / FunctionSignature *value*
		//     (whose Parent lives under Type/) — types too;
		//   - a concrete scalar / list / map, and the value `none`, are
		//     NOT types — `5 is Type`, `[1 2 3] is Type`, `none is Type`
		//     are false. Carriers are abstract VALUES, not types.
		//
		// Other Type/-rooted RHS (`Function`, `Disjunct`, `Enum`,
		// `FunctionSignature`, the legacy `ScalarType` / `NodeType` /
		// `ObjectType` metatypes) keep the plain subtype check below, so
		// `fn […] is Function` / `enum […] is Disjunct` still hold.
		if t.Equal(TType) {
			if v.Carrier {
				return false
			}
			return IsBareTypeNode(v) || IsTypeBody(v) || IsRecordShape(v) || v.Parent.ConformsTo(TType)
		}
		// Canonical dispatch site: route through Behavior so custom
		// type semantics (predicate types, dependent scalars, future
		// plugin types) get consulted. Default Behavior delegates to
		// the historical lattice walk.
		return v.Is(&t)
	}
	_, ok := Unify(v, t)
	return ok
}

// InstallType is the single kernel entry point for installing a
// named type body (`def Foo body`). Validates the body shape,
// rejects name clashes, and pushes onto the registry's type
// stack. Used by both the eng-internal core `def` word and the
// production boru `def` word in lang/go/engine. Changes to
// type-installation policy go here, not in a per-surface duplicate.
//
// Body acceptance is broad: a structural type body (IsTypeBody — type
// literal, disjunct, implicit map, typed list/map, ObjectType, …) OR a
// concrete scalar / list / map literal (IsLiteralTypeBody — `def Foo
// 1`, the singleton type whose only inhabitant is 1). The split keeps
// the inspect / fn-shape paths aligned with structural typing while
// letting users name singletons and value-shape types.
//
// When the body is an anonymous ObjectType (from `refine Object {…}`),
// binding it under NAME renames it `Object/NAME` (or `<parent>/NAME`
// validateTypeName runs the name checks every type binding must pass —
// capitalisation, no part conflicting with an existing type, and no clash
// with a registered function or a value def. Shared by InstallType (the
// `def` path) and the host-Go DefineMemberType path so both refuse to
// shadow a builtin/user type or mint under an invalid name.
func validateTypeName(r *Registry, name string) error {
	if !IsCapitalisedName(name) {
		return &BoruError{
			Code:   "type_error",
			Detail: "type " + name + ": type names must start with a capital letter",
		}
	}
	if !r.Defs.IsType(name) {
		if err := ValidateTypeNameParts(name, r.IsKnownPart); err != nil {
			// Wrap in the taxonomy like every sibling check — the raw
			// error leaked to hosts as a non-BoruError (the crossdiff
			// surfaced it as UNEXPECTED:…, not a code).
			return &BoruError{Code: "type_error", Detail: err.Error()}
		}
	}
	if r.Lookup(name) != nil {
		return &BoruError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a registered function",
		}
	}
	if r.Defs.Has(name) && !r.Defs.IsType(name) {
		return &BoruError{
			Code:   "type_error",
			Detail: "type " + name + ": name clash — already a def'd value",
		}
	}
	return nil
}

// when it inherits) so `typeof` / `is` report the nominal name.

// installTypeBinding stamps the declared content onto the minted node
// (Value.TypeBody — design/TYPE-REPRESENTATION.1.md §N2's node-side
// recovery) and pushes the type binding. Every MINTING branch of
// InstallType funnels through here; the alias arm adopts an existing
// node instead and never stamps. A bare-node pushed value (the refine
// newtype's renamed literal) carries no structure, so nothing is
// recorded for it.
func installTypeBinding(r *Registry, name string, def *Type, pushed Value) {
	if !IsBareTypeNode(pushed) {
		def.SetTypeBody(pushed)
	}
	r.Defs.PushType(name, def, pushed)
	r.NoteTypeInstall(name, pushed.Pos())
}

func InstallType(r *Registry, name string, body Value) error {
	if !IsTypeBody(body) && !IsLiteralTypeBody(body) {
		return &BoruError{
			Code:   "type_error",
			Detail: "type: body must be a type value or literal, got " + body.String(),
		}
	}
	if err := validateTypeName(r, name); err != nil {
		return err
	}
	return InstallTypeBody(r, name, body)
}

// InstallTypeBody is InstallType past its two ENTRY checks — the body-shape
// test and validateTypeName. It exists for the arm-resident type twin
// (core.ApplyResidentTypeBind), which re-installs a body the CHECK PASS
// already validated under this very name, into a registry whose bindings
// were rolled back for replay but whose registered name PARTS were not: run
// through the front door and validateTypeName rejects the replay on its own
// leftovers ("name part \"Big\" in \"Big\" conflicts with an existing type
// name" — measured, on element 0). Replay, never re-execution, is §6.5's
// rule for exactly this reason; what the replay still wants from the
// installer is the MINT, so each element gets its own node.
//
// Every other caller wants the checks: go through InstallType.
func InstallTypeBody(r *Registry, name string, body Value) error {
	// Seated with the operation, not with `def`'s capitalised arm, which
	// returned this call's result directly and so reached no notification at
	// all until 2026-09-04 (core/go/rebind_notify.go).
	noteRebind(r, name)
	if IsMicronType(body) {
		// `def Baron refine Micron {foo:String}` route: the family
		// Ideal's Construct produced a MicronTypeInfo body; mint the
		// kind under the family root and apply the root's capabilities
		// (ADR-012 rule 5): the SubtypeNamer naming rule and the
		// MicronSubtypeMinter Behavior (which carries the schema so
		// `make` can find it through the parent walk). The content
		// layer (basic/go/micron.go) supplies both; without it the
		// mint keeps DefaultBehavior.
		info, _ := AsMicronType(body)
		if err := validateSubtypeNameFor(body.Parent, name); err != nil {
			return err
		}
		def := r.Types.MintType(name, body.Parent)
		info.Name = name
		info.Type = def
		bhv := DefaultBehavior
		if m, ok := body.Parent.Behavior().(MicronSubtypeMinter); ok {
			bhv = m.MintSubtypeBehavior(def, &info)
		}
		def.ensureTMeta().Behavior = bhv
		installTypeBinding(r, name, def, NewValueRaw(def, info))
	} else if IsClassType(body) {
		info, _ := AsClassType(body)
		// Object types are class types now — sealed nominal records rooted
		// under Ideal/Class (the open Object container was removed).
		rootName := "Class"
		rootDef := TClass
		if info.Parent != nil {
			info.Name = info.Parent.Name + "/" + name
		} else {
			info.Name = rootName + "/" + name
		}
		for _, p := range strings.Split(info.Name, "/") {
			r.RegisterPart(p)
		}
		parentDef := rootDef
		if info.Parent != nil && info.Parent.Type != nil {
			parentDef = info.Parent.Type
		}
		def := r.Types.MintType(name, parentDef)
		body = NewClassType(def, info)
		installTypeBinding(r, name, def, body)
	} else if IsTypeSchema(body) {
		// `def Box gen [T] class {…}` route: mint the SCHEMA node where
		// the non-generic equivalent would mint (D3 — class schemas
		// under Ideal/Class, record schemas under the body's parent,
		// fnsig schemas under FunctionSignature) and attach a
		// schemaUnifier sharing the *TypeSchemaInfo payload.
		// Instantiation nodes mint as CHILDREN of this node at `of`
		// time, so bare `Box` admits any instantiation by plain
		// lattice ancestry.
		info, _ := AsTypeSchema(body)
		parent := TType
		switch info.Kind {
		case SchemaClass:
			parent = TClass
			r.RegisterPart("Class")
		case SchemaRecord:
			parent = TRecord
		case SchemaFnSig, SchemaFn:
			parent = TFnUndef
		}
		def := r.Types.MintType(name, parent)
		info.Name = parent.Name() + "/" + name
		info.Type = def
		InstallSchemaUnifier(def, info)
		r.RegisterPart(name)
		installTypeBinding(r, name, def, NewValueRaw(def, info))
	} else if IsSurfaceType(body) {
		// `def Shape surface {…}` route: mint a lattice node under
		// Ideal/Surface and attach a surfaceUnifier so dispatch / `is`
		// consult the conformance set `exposes` fills in. The payload
		// pointer is shared between body and unifier — a post-mint
		// `exposes` is visible through both.
		info, _ := AsSurfaceType(body)
		info.Name = "Surface/" + name
		r.RegisterPart("Surface")
		r.RegisterPart(name)
		def := r.Types.MintType(name, TSurface)
		info.Type = def
		installSurfaceUnifier(def, info, name)
		installTypeBinding(r, name, def, NewValueRaw(def, info))
	} else if inputT, isPred := PredicateInputType(body), IsDeclaredPredicateFn(body) || isPredicateFnValue(body); inputT != nil || isPred {
		// Predicate type with a concrete input type: mint the *Type
		// parented at the input rather than at TFunction so values
		// rewrapped by the typed-bind path inherit input-side
		// capabilities (Integer's Number-branch Comparer, etc.)
		// through the lattice walk. Predicate types declared with
		// `Any` input (the historical `fn [x:Any Any […]]` pattern)
		// fall through to the regular PushType path — they remain
		// gates, not dispatch categories.
		// An Any-input (or unannotated) predicate mints under Function
		// itself: it has no concrete base to inherit capabilities from,
		// but it is a DISPATCH CATEGORY like every other membership
		// kind — the pre-flip regime left it an unbridged catch-all
		// node ("a gate, not a dispatch category"), which is the same
		// dead-dispatch class NUR093 records for aliases. One rule for
		// every kind (design/TYPE-REPRESENTATION.1.md §N3).
		parent := inputT
		if parent == nil {
			parent = TFunction
		}
		def := r.Types.MintType(name, parent)
		// NOTE: the FnDef payload's Name is deliberately NOT stamped here
		// — canon renders predicate bodies, and a stamped name would
		// change their canonical ordering (compare.tsv §predicate-kind).
		// resolvePredicateRef finds the type for a body COPY via the
		// shared-construction reverse lookup (sameFnConstruction).
		// Attach a Unifier so the predicate runs at every Unify call
		// site (signature matching, options fields, record fields,
		// `make` constraints, the `unify` word). Without this, Unify
		// would take the lattice subtype path and admit any
		// base-compatible value without checking the predicate.
		// Phase 6 (predicate stamps): compile the predicate body to a
		// detached unit at CONSTRUCTION, so RunPredicate's InvokeCallback
		// runs it on the VM instead of the CallBoru interpreter fallback —
		// the same pre-publication in-place stamp module load applies to
		// its exports (the binding has not escaped this goroutine).
		// Declines (captures, compile refusals) keep the interpreter path;
		// only the REF is stamped, never the payload Name (the canon-
		// ordering note above stands).
		if r.RuntimeStampingEnabled() {
			if fd, isFn := body.Data.(FnDefInfo); isFn {
				// The stamp EVENT carries the type name so -compile-report
				// (and the stamp-report gates) attribute the unit; the
				// binding's payload Name stays empty (the canon note above).
				named := fd
				named.Name = name
				compiledRuntime.StampDetached(r, named, body.Pos())
			}
		}
		installPredicateUnifier(def, body, r, name)
		installTypeBinding(r, name, def, body)
	} else if IsRefinePrefab(body) {
		// `def Foo refine Integer` route: `refineBareHandler` minted
		// an anonymous refine prefab (MintRefinePrefab) and returned
		// its type literal. Rename the lattice node and bind the
		// renamed Foo literal as the body so resolving `Foo` pushes
		// the new subtype node (Parent = base, Rank in the external
		// band) rather than the original input type literal.
		def := r.Types.LookupByID(body.ID)
		if def == nil {
			return &BoruError{
				Code:   "type_error",
				Detail: "type " + name + ": refine prefab missing from lattice",
			}
		}
		// A bare nominal refine inside a family with a naming rule
		// (`def Workon refine Emailon` — the Micron -on rule, a
		// SubtypeNamer capability on the family Behaviors) is a
		// newtype: the rule applies.
		if err := validateSubtypeNameFor(def.Parent, name); err != nil {
			return err
		}
		def.ensureTMeta().Name = name
		body.ensureTMeta().Name = name
		// Attach a bareRefineUnifier so dispatch admits any value
		// whose declared type satisfies the base. Without this, the
		// fresh lattice node carries DefaultBehavior and the walk
		// rejects every value because the prefab sits BELOW its base
		// in the lattice, not above.
		//
		// Crucially we install on `def` (the lattice node consulted
		// by sig dispatch via `LookupTypeName`) but NOT on `body`
		// (the value returned by `Defs.Top` and consumed by the `is`
		// word). The two paths intentionally answer different
		// questions:
		//   - dispatch asks "can v be passed where T is expected?"
		//     → loose binding; admits base-compatible values.
		//   - `is`     asks "does v carry T's tag?"
		//     → strict identity; admits only narrowed/tagged values.
		// See lang/go/test/typed_def_test.go::TestRefineBareDistinctFromAlias
		// for the asymmetry assertion.
		if def.Parent != nil {
			installBareRefineUnifier(def, name)
		}
		installTypeBinding(r, name, def, body)
	} else if IsDisjunct(body) {
		// `def Maybe (Integer tor none)` route: mint a lattice node
		// parented at the body's lattice (TDisjunct) and attach a
		// disjunctUnifier so `42.Is(Maybe)` and sig dispatch consult
		// the alternatives. Without this Unifier the lattice walk
		// rejects every value because no value's parent chain reaches
		// the Maybe node. Same dispatch-vs-`is` asymmetry as bare
		// refine: install on `def`, not on `body`.
		di, _ := AsDisjunct(body)
		def := r.Types.MintType(name, body.Parent)
		installDisjunctUnifier(def, di.Alternatives, name)
		installTypeBinding(r, name, def, body)
	} else if fu, isFnUndef := body.Data.(FnUndefInfo); isFnUndef {
		// `def IntToStr fnsig Integer String` route: mint a lattice node
		// parented at the body's lattice (TFnUndef) and attach a
		// fnUndefUnifier so `f/v is IntToStr` AND sig dispatch on a
		// `g:IntToStr` parameter both consult the signature specs.
		// Without this Unifier the lattice walk rejects every function,
		// because no function's parent chain reaches the IntToStr node —
		// the same dispatch-vs-`is` asymmetry as the disjunct, negation
		// and DepScalar branches, and for the same reason. Install on
		// `def`, not on `body`.
		def := r.Types.MintType(name, body.Parent)
		installFnUndefUnifier(def, fu.Sigs, name)
		installTypeBinding(r, name, def, body)
	} else if IsNegation(body) {
		// `def NotStr (tnot String)` route: mint a lattice node parented
		// at the body's lattice (TNegation) and attach a negationUnifier
		// so `5.Is(NotStr)` and sig dispatch consult the complement
		// (admit v iff v does NOT match the inner type). Same dispatch-
		// vs-`is` asymmetry as the disjunct branch: install on `def`,
		// not on `body`.
		ni, _ := AsNegation(body)
		def := r.Types.MintType(name, body.Parent)
		installNegationUnifier(def, ni.Inner, name)
		installTypeBinding(r, name, def, body)
	} else if body.IsDepScalar() {
		// `def Big (Integer gt 10)` route: mint a lattice node
		// parented at the base scalar (the DepScalar's Parent — e.g.
		// Integer) and attach a depScalarUnifier so dispatch runs the
		// constraint. Without this Unifier the lattice walk would
		// reject every value (Big isn't on any value's parent chain).
		// Same dispatch-vs-`is` asymmetry: install on `def`, not on
		// `body`.
		di, err := body.AsDepScalar()
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return &BoruError{
				Code:   "type_error",
				Detail: "type " + name + ": DepScalar body unreadable: " + err.Error(),
			}
		}
		// Defensive: a dependent scalar over a base with a subtype
		// naming rule (a Micron kind) still mints under the family,
		// so the rule applies.
		if err := validateSubtypeNameFor(body.Parent, name); err != nil {
			return err
		}
		def := r.Types.MintType(name, body.Parent)
		installDepScalarUnifier(def, body.Parent, di, name)
		installTypeBinding(r, name, def, body)
	} else if IsBareTypeNode(body) {
		// ALIAS: `def Foo Integer`. The name ADOPTS the canonical
		// aliased node instead of minting an unbridged child — a fresh
		// mint under the aliased type carried DefaultBehavior, nothing
		// was ever tagged with it, and dispatch on an `x:Foo` parameter
		// rejected every value while `42 is Foo` (which consults the
		// body) accepted (NUR093). An alias claims the SAME type, so it
		// binds the same node; the newtype/alias distinction stands —
		// a newtype (`refine`) mints, an alias adopts. Route through
		// CanonicalType so the adopted pointer is the canonical *Type
		// and not a stack copy. The aliased family's SubtypeNamer rule
		// still applies (`def Mail Emailon` still refuses), keeping the
		// naming surface unchanged.
		canon := CanonicalType(r, &body)
		if err := validateSubtypeNameFor(canon, name); err != nil {
			return err
		}
		r.Defs.PushTypeAdopted(name, canon, body)
		r.NoteTypeInstall(name, body.Pos())
	} else {
		// Structural / singleton bodies (record shape, `def One 1`,
		// typed-container literals, refine Record/Options/Table bodies,
		// host shaped types, …) parent at their container type (Map /
		// List / Integer / …) and get a content-deciding
		// BindingBodyUnifier on the minted node, so dispatch and `is`
		// consult one structural rule — the DisjunctUnifier /
		// FnUndefUnifier discipline extended to the catch-all kinds
		// (NUR090's record-shape row, NUR093's singleton row).
		parent := body.Parent
		if err := validateSubtypeNameFor(parent, name); err != nil {
			return err
		}
		def := r.Types.MintType(name, parent)
		installBindingBodyUnifier(def, body, name)
		installTypeBinding(r, name, def, body)
	}
	for _, p := range strings.Split(name, "/") {
		r.RegisterPart(p)
	}
	return nil
}
