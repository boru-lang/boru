package native

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"

	"github.com/cockroachdb/apd/v3"
)

// typeNatives covers the type-system words: refine, pathof, enum,
// typeof, is, teq, tpartial, guard, base, tor, tand, tany, tall,
// convert. New type ops follow the `t`-prefix convention — see
// design/legacy/TYPE-OPERATIONS.8.ignore.
//
// `Resource` and `Entity` (the builtin object types) are NOT installed
// via NativeFunc — they are user-typed values pushed onto the type
// stack. `installResourceTypes` handles those during Register.
var typeNatives = []NativeFunc{
	{
		// refine is the uniform type constructor — see
		// design/legacy/TYPE-UNIFORM.10.ignore. `refine BaseType arg`
		// builds a (sub)type:
		//   class {fields}              → class type (see the `class` word)
		//   refine <classtype> {fields} → class subtype (inheritance)
		//   refine Record [a:T b:U]    → record type (list of pairs)
		//   refine Table  (refine Record …) → table type
		//   refine BaseType            → a bare nominal subtype, no
		//                                added structure (the 1-arg form)
		//
		// Two signatures: a 2-arg structural form and a 1-arg bare form.
		// Because the 1-arg signature lets `refine` succeed with a
		// single argument, the word never defers to take a body from the
		// stack — so a nested constructor must be parenthesised:
		// `refine Table (refine Record […])`, not `refine Table refine
		// Record […]`. The 2-arg body is always a Node (a map or list
		// literal, or a record/object type value), typed TNode so the
		// matcher falls through to the 1-arg form when a non-Node token
		// (a following `def` / `behave` / `;`) comes next.
		Name: "refine",

		Signatures: []Signature{
			{
				Args:       []*Type{TAny, TNode},
				Impl:       Go(refineHandler, RunInCheck()),
				Returns:    []*Type{TType},
				BarrierPos: -1,
			},
			{
				Args:       []*Type{TAny},
				Impl:       Go(refineBareHandler, RunInCheck()),
				Returns:    []*Type{TType},
				BarrierPos: -1,
			},
		},
	},
	{
		// class {schema} — define a class type: a sealed nominal record
		// minted under Ideal/Class. Schema entries: a TYPE value
		// (`{name:String}`) declares a required field; a CONCRETE value
		// (`{retries:3}`) declares a default (and the field's type is
		// the value's own type). Instances are flat (defaults resolved
		// eagerly at make) and sealed (writing an undeclared field is a
		// sealed_field error). Subclassing reuses refine:
		// `def Bar refine Foo {…}`. See design/legacy/CLASS-OBJECT.10.ignore.
		Name: "class",

		Signatures: []Signature{{
			Args:       []*Type{TMap},
			Impl:       Go(classHandler, RunInCheck()),
			Returns:    []*Type{TType},
			BarrierPos: -1,
		}},
	},
	{
		// surface {schema} — declare a pure operation contract: a map
		// of operation name → fnsig shape with Self marking the
		// conforming type's positions. `def Shape surface {…}` mints it
		// under Ideal/Surface; `<Type> exposes Shape` declares (and
		// loudly checks) conformance. See design/legacy/SURFACES.10.ignore.
		Name: "surface",

		Signatures: []Signature{{
			Args:       []*Type{TMap},
			Impl:       Go(surfaceHandler, RunInCheck()),
			Returns:    []*Type{TType},
			BarrierPos: -1,
		}},
	},
	{
		// <Type> exposes <Surface> — explicit conformance: check the
		// overload table against every required shape (Self := Type;
		// contravariant params, covariant returns) and register the
		// type in the surface's conformance set. All-or-nothing, loud
		// (surface_unsatisfied lists every gap), idempotent.
		Name: "exposes",

		Signatures: []Signature{{
			Args:       []*Type{TAny, TAny},
			Impl:       Go(exposesHandler, RunInCheck()),
			Returns:    []*Type{},
			BarrierPos: 1,
		}},
	},
	{
		Name: "pathof",

		Signatures: []Signature{{
			Args:     []*Type{TAny},
			TypeArgs: map[int]bool{0: true},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{core.PathOf(args[0])}, nil
			}),
			Returns: []*Type{TList}, BarrierPos: -1,
		}},
	},
	{
		Name: "enum",

		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(enumHandler),
			Returns:    []*Type{TEnum},
			ReturnsFn:  enumReturns, BarrierPos: -1,
		}},
	},
	{
		Name:          "typeof",
		CompileEffect: CompileModuleFold | CompileIslandPure,

		Signatures: []Signature{{
			Args:      []*Type{TAny},
			Impl:      Go(typeofHandler),
			Returns:   []*Type{TType},
			ReturnsFn: typeofReturns, BarrierPos: -1,
			CompileEffect: CompileReadsFn, // reads a fn value's type, never invokes it
		}},
	},
	{
		Name:          "is",
		CompileEffect: CompileModuleFold | CompileIslandPure,

		Signatures: []Signature{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Impl:       Go(isHandler),
			Returns:    []*Type{TBoolean},
			// Membership reads the VALUE slot's lattice tag / runs the type's
			// own Match predicate over it — a fn value there (`(+re/…/) is
			// (MiniLang.Re)`, module-minilang.tsv) is DATA, never invoked
			// (Stage M2d, design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore). The TYPE
			// slot (position 0) is deliberately NOT inert: a concrete Function
			// there is a PREDICATE the handler INVOKES via RunPredicate
			// (`5 is Positive`), so whole-sig CompileReadsFn would miscompile —
			// the positional map keeps that slot on the compile failure path (pinned by
			// TestFnValueIntrospectionLowers' invoke negative).
			FnInertArgs: map[int]bool{1: true},
		}},
	},
	{
		// as is call-site signature selection via match-time dispatch
		// ascription (design/OPEN-WORDS.1.md §9): `v as T` returns v
		// UNCHANGED — payload, tag, rendering, equality — carrying an
		// ascription that makes the NEXT signature match treat v as a T.
		// Upcast-only (T must be an ancestor of v's tag), consumed at arg
		// delivery, so it selects exactly one dispatch. The delegation
		// escape for anchored override bodies: `set k v (m as FlexMap)`
		// dispatches the base FlexMap overload — the override's nominal
		// anchor cannot match the widened view — while the handler still
		// receives the real subtype-tagged m.
		Name:          "as",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{{
			Args:       []*Type{TAny, TAny},
			BarrierPos: 1,
			Impl:       Go(asHandler),
			Returns:    []*Type{TAny},
			ReturnsFn:  asReturns,
			// Both slots are DATA: the type operand is a literal the
			// handler validates (never a predicate to invoke, unlike
			// `is`), and the value slot mirrors `is`'s inert value rule.
			FnInertArgs: map[int]bool{0: true, 1: true},
		}},
	},
	{
		Name:          "teq",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{{
			Args:          []*Type{TAny, TAny},
			BarrierPos:    1,
			Impl:          Go(teqHandler),
			Returns:       []*Type{TBoolean},
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	{
		Name:          "tis",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{{
			Args:          []*Type{TAny, TAny},
			BarrierPos:    1,
			Impl:          Go(tisHandler),
			Returns:       []*Type{TBoolean},
			CompileEffect: CompileReadsFn, // reads the operands' lattice tags, never invokes
		}},
	},
	{
		Name: "guard",

		Signatures: []Signature{{
			Args:       []*Type{TAny, TBoolean},
			BarrierPos: 1,
			Impl:       Go(guardHandler),
			ReturnsFn:  guardReturns,
		}}},
	{
		Name: "base",

		Signatures: []Signature{{
			Args:      []*Type{TAny},
			Impl:      Go(baseHandler),
			ReturnsFn: ReturnsIdentity(0), BarrierPos: -1,
		}},
	},
	// `tor` (disjunct union) and `tand` (intersection) — type-level
	// connective words. Algorithm primitives live in eng
	// (core.TorHandler / core.TandHandler / core.TandValues); the
	// registrations here own the names and dispatch wiring.
	{
		Name:          "tor",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{{
			Args:          []*Type{TAny, TAny},
			BarrierPos:    1,
			Impl:          Go(core.TorHandler),
			ReturnsFn:     core.TorReturnsFn,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	{
		Name:          "tand",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{{
			Args:          []*Type{TAny, TAny},
			BarrierPos:    1,
			Impl:          Go(core.TandHandler),
			ReturnsFn:     core.TandReturnsFn,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	// `tnot` (type negation / complement) — closes the type algebra
	// under Boolean operations. `tnot T` matches v iff v does not match
	// T. Algorithm lives in eng (core.TnotHandler / core.NegateType).
	{
		Name:          "tnot",
		CompileEffect: CompileIslandPure,

		Signatures: []Signature{{
			Args:          []*Type{TAny},
			BarrierPos:    -1,
			Impl:          Go(core.TnotHandler),
			ReturnsFn:     core.TnotReturnsFn,
			CompileEffect: CompileReadsFn, // type-algebra reads fn-value types, never invokes
		}},
	},
	{
		Name: "tany",

		Signatures: []Signature{
			{Args: []*Type{TList}, Impl: Go(tanyHandler), Returns: []*Type{TAny}, ReturnsFn: typeAlgebraListFold(tanyHandler), BarrierPos: -1},
		},
	},
	{
		Name: "tall",

		Signatures: []Signature{
			{Args: []*Type{TList}, Impl: Go(tallHandler), Returns: []*Type{TAny}, ReturnsFn: typeAlgebraListFold(tallHandler), BarrierPos: -1},
		},
	},
	{
		Name:          "convert",
		CompileEffect: CompileModuleFold,

		Signatures: []Signature{
			// Ideal → Map / List (per-type IdealConverter; base Ideal → {} / [])
			{
				Args:     []*Type{TNode, TIdeal},
				TypeArgs: map[int]bool{0: true},
				Impl:     Go(convertIdealHandler),
				// convert yields a VALUE of the target type (like make), not the
				// target type literal — ReturnsFreshInstance mints a carrier OF
				// arg0's type so a downstream consumer (e.g. arithmetic on a
				// `convert Float`ed scalar) sees an inhabitant, not a bare node.
				ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1,
			},
			{
				Args:     []*Type{TScalar, TMap, TScalar},
				TypeArgs: map[int]bool{0: true},
				Patterns: map[int]Value{1: convertOptsPattern()},
				Impl:     Go(convert3Handler),
				// See the Ideal sig above: a VALUE of arg0's type, not the literal.
				// The dry pass surfaces the handler's own option-map
				// validation (unknown accuracy mode, places outside its
				// domain, a non-finite source) at check time when every
				// operand is a top-level literal — the handler is a pure
				// scalar conversion, so a guaranteed runtime error is
				// decidable right here.
				ReturnsFn: DryPassWrap(convert3Handler, ReturnsFreshInstance(0)), BarrierPos: -1,
			},
			{
				Args:     []*Type{TScalar, TScalar},
				TypeArgs: map[int]bool{0: true},
				Impl:     Go(convert2Handler),
				// See the Ideal sig above: a VALUE of arg0's type, not the
				// literal. The wrapper additionally flags the one TYPE-decidable
				// compile failure: a Float source into a Big target always raises.
				ReturnsFn: convertScalarReturns, BarrierPos: -1,
			},
			// NUR053 — the truthiness DOMAIN. `if` and `make Boolean` accept
			// any value; these two sigs make `convert Boolean` accept any value
			// too, so the three consumers of the One Truthiness Model share a
			// domain as well as a rule.
			//
			// They are Boolean-target-ONLY (arg0 is the `Boolean` type literal,
			// not `Scalar`) on purpose: the general scalar sigs above stay
			// Scalar-sourced, because there is no total presence rule to apply
			// for an Integer or a Date target — `convert Integer [1 2]` has no
			// meaning and must keep raising. Boolean is the one target whose
			// conversion IS a coercion, and `CoerceBoolean` — the very function
			// `if` applies — is already total over every value mode, so this is
			// signature ADMISSION, not new semantics: the handlers below needed
			// no change at all.
			//
			// Source order is NOT dispatch order — SortSignatures ranks by
			// specificity, and `Boolean` outranks `Scalar` at position 0, so
			// these claim every Boolean-target call including the Scalar ones.
			// That is harmless: they route to the same handlers, and the
			// ReturnsFn they replace (convertScalarReturns) only ever adds a
			// Float→Big diagnostic, which a Boolean target cannot reach.
			{
				Args:      []*Type{TBoolean, TAny},
				TypeArgs:  map[int]bool{0: true},
				Impl:      Go(convert2Handler),
				ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1,
			},
			// The options form is deliberately UNPATTERNED, unlike its Scalar
			// sibling, and validates inside the handler instead. With an `Any`
			// source at arity 2, a pattern here would be a trap: a malformed
			// options map fails the pattern, dispatch falls back to the arity-2
			// row, and the OPTIONS MAP itself is coerced as the source while the
			// real source strands on the stack — `convert Boolean {truthyy:true}
			// 'no'` answering true rather than false, from one typo'd key. So
			// this row claims the whole 3-arg Boolean shape unconditionally and
			// `convertBoolOptsHandler` names the bad key loudly. Because it
			// DELEGATES on a well-formed map, the fix is independent of how the
			// sorter happens to order the two rows.
			{
				Args:     []*Type{TBoolean, TMap, TAny},
				TypeArgs: map[int]bool{0: true},
				Impl:     Go(convertBoolOptsHandler),
				// Dry-passed like the Scalar options form, so a literal call with
				// a bad option key is reported at CHECK time, not only at run time.
				ReturnsFn: DryPassWrap(convertBoolOptsHandler, ReturnsFreshInstance(0)), BarrierPos: -1,
			},
		},
	},
}

// ---- table ----

func tableHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	target := args[0]
	if !IsRecordType(target) {
		return nil, fmt.Errorf("table: argument must be a record type, got %s", target.String())
	}
	_as0, _ := AsRecordType(target)
	return []Value{NewTableType(_as0)}, nil
}

// ---- refine (the type constructor) ----

// refineHandler implements `refine BaseType arg`, the uniform type
// constructor. It does not branch on the base type itself — dispatch
// is data-driven through the Ideal registry (r.Ideals): whichever
// type-kind claims the base value supplies the construction logic.
// See design/IDEAL.10.md. `refine` does not bind — pair it
// with `def` (`def Foo (refine …)`).
func refineHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	base := args[0]
	arg := args[1]
	// A pending gen spec (the preceding `gen [...]`) turns this
	// construction into a generic SCHEMA: the body is built with the
	// placeholder bindings still live (so [value:T] resolves), then
	// the bindings pop and the result wraps as a TypeSchema for
	// InstallType. v1 supports Record schemas through refine; classes
	// go through `class {...}`, fn shapes through `fnsig [...]`.
	if spec := r.TakePendingGen(); spec != nil {
		out, err := refinePlain(base, arg, r)
		if err != nil {
			PopGenBindings(r, spec)
			return nil, err
		}
		if !IsRecordType(out[0]) {
			return nil, genUnsupported(r, spec, "refine", out[0].String())
		}
		return genWrapSchema(r, spec, out[0], SchemaRecord)
	}
	return refinePlain(base, arg, r)
}

func refinePlain(base, arg Value, r *Registry) ([]Value, error) {
	// A NAMED base or argument evaluates to its minted node (the Stage
	// 2 flip); the kind dispatch and the constructors operate on the
	// declared structural content, which the node records
	// (design/legacy/TYPE-REPRESENTATION.1.ignore §N2). Bare bases with no content
	// (refine Integer, refine P) pass through unchanged.
	if body, ok := TypeContentOf(base); ok && IsBareTypeNode(base) {
		base = body
	}
	if body, ok := TypeContentOf(arg); ok && IsBareTypeNode(arg) {
		arg = body
	}
	ideal := r.Ideals.For(base)
	if ideal == nil {
		// Distinguish a disabled kind from an unknown base.
		if m := r.Ideals.Match(base); m != nil {
			return nil, r.BoruError("type_error",
				fmt.Sprintf("refine: the %s type-kind is not available in this registry", m.Name),
				"refine")
		}
		return nil, r.BoruError("type_error",
			fmt.Sprintf("refine: base must be Record, Table, or a class type, got %s", base.String()),
			"refine")
	}
	if ideal.Construct == nil {
		return nil, r.BoruError("type_error",
			fmt.Sprintf("refine: the %s type-kind cannot be constructed with `refine`", ideal.Name),
			"refine")
	}
	return ideal.Construct(base, arg, r)
}

// refineBareHandler implements the 1-arg `refine BaseType` form — a
// bare nominal subtype of BaseType with no added structure. It
// validates that the argument is a type and returns it unchanged; the
// paired `def Name` then mints a fresh subtype parented at BaseType
// (InstallType → MintType). `def Foo refine List` thus produces a
// distinct List subtype that can serve as a dispatch surface for
// `behave` — see design/legacy/TYPE-UNIFORM.10.ignore.
func refineBareHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	base := args[0]
	if !IsTypeBody(base) {
		return nil, r.BoruError("type_error",
			fmt.Sprintf("refine: argument must be a type, got %s", base.String()),
			"refine")
	}
	// Bare type-literal base: mint an anonymous user subtype now and
	// return its type literal. The paired `def Foo` (via InstallType)
	// renames the anonymous lattice node to "Foo". This split
	// distinguishes the subtype path (`def Foo refine Integer`) from
	// the alias path (`def Foo Integer`, where the body remains the
	// input type literal verbatim) — without this differentiation the
	// two surfaces would be indistinguishable downstream.
	if IsBareTypeNode(base) {
		// A NAMED base evaluates to its node (the Stage 2 flip). When
		// the node records declared content (a Micron/record schema, a
		// predicate constraint), return that content verbatim so the
		// paired `def` re-enters its branch dispatch exactly as it did
		// when the name denoted the body — the newtype inherits the
		// schema (design/legacy/TYPE-REPRESENTATION.1.ignore §N2).
		if content, ok := TypeContentOf(base); ok {
			return []Value{content}, nil
		}
		// Mint the refine prefab against the canonical lattice node
		// for base, so any user-installed Behavior on base (via
		// `behave`) propagates to the LCA walk for sibling subtypes
		// downstream. The prefab carries no Name; the paired `def`
		// recognises it (core.IsRefinePrefab) and renames-and-binds.
		anon := r.Types.MintRefinePrefab(CanonicalType(r, &base))
		return []Value{NewTypeLiteral(anon)}, nil
	}
	return []Value{base}, nil
}

// installIdeals fills in the type-level constructor (Ideal.Construct)
// on the kernel Ideals. The descriptors — Name, the Accepts dispatch
// predicate, and the value-level Instantiate — are registered by the
// eng kernel (registerKernelIdeals); type construction additionally
// reuses the surface object/record/table handlers, wired here. See
// design/IDEAL.10.md.
func installIdeals(r *Registry) {
	if obj := r.Ideals.Get("Object"); obj != nil {
		obj.Construct = func(base, arg Value, r *Registry) ([]Value, error) {
			// An existing class type builds a subtype of it
			// (`def Bar refine Foo {…}`). The bare-Object form is
			// REMOVED: classes are defined with the `class` word
			// (design/legacy/CLASS-OBJECT.10.ignore — no deprecated aliases).
			if IsClassType(base) {
				return objectWithParentHandler([]Value{arg, base}, nil, nil, r)
			}
			return nil, r.BoruErrorHint("refine_error",
				"refine Object is no longer the class form",
				"refine",
				"define a class instead: def Foo class {…}; subclass with def Bar refine Foo {…}")
		}
	}
	if rec := r.Ideals.Get("Record"); rec != nil {
		rec.Construct = func(base, arg Value, r *Registry) ([]Value, error) {
			// Records have no subtyping — only the bare Record literal
			// is a valid construction base.
			if base.Data != nil {
				return nil, r.BoruError("type_error",
					"refine: a record type has no subtyping — construct a Record from the bare Record literal",
					"refine")
			}
			// A record takes a LIST of field pairs — field order is
			// part of a record type's identity.
			if !arg.Parent.Equal(TList) {
				return nil, r.BoruError("type_error",
					"refine Record: a record takes a list of field pairs, e.g. [a:Integer b:String]",
					"refine")
			}
			return recordHandler([]Value{arg}, nil, nil, r)
		}
	}
	if tbl := r.Ideals.Get("Table"); tbl != nil {
		tbl.Construct = func(base, arg Value, r *Registry) ([]Value, error) {
			if base.Data != nil {
				return nil, r.BoruError("type_error",
					"refine: a table type has no subtyping — construct a Table from the bare Table literal",
					"refine")
			}
			return tableHandler([]Value{arg}, nil, nil, r)
		}
	}
	// Binary / BinarySpec — the binary-frame instance / spec membership types
	// (BinarySpec : Binary :: Class : Object). A binary frame is realised on the
	// class machinery: `def Header (refine BinarySpec [layout])` builds a sealed
	// CLASS whose fields are the layout's decoded types and which carries the raw
	// wire layout (ClassTypeInfo.BinaryLayout); `make Header {fields}` reuses the
	// class make path to produce a field-accessible INSTANCE; `convert Bytes`/
	// `unpack` are the Binary⇄Bytes codec (native_bytes.go). The two membership
	// types name the roles and answer `is`:
	//   - BinarySpec — a class TYPE carrying a layout (also the `refine` base).
	//   - Binary     — a class INSTANCE whose type carries a layout.
	// (Because instances reuse classes they are also `is Class` — a deliberate
	// simplification; see design/go-modules/BYTES.10.md §7.)
	if _, err := r.DefineMemberType("Binary", TClass, func(v Value) bool {
		_, ok := binaryInstanceLayout(v)
		return ok
	}); err != nil {
		recordTypeInitErr(fmt.Errorf("native_type: define Binary: %w", err))
	}
	bspec, err := r.DefineMemberType("BinarySpec", TClass, func(v Value) bool {
		_, ok := binarySpecLayout(v)
		return ok
	})
	if err != nil {
		recordTypeInitErr(fmt.Errorf("native_type: define BinarySpec: %w", err))
		return
	}
	r.Ideals.Register(&core.Ideal{
		Name:    "BinarySpec",
		Enabled: true,
		Accepts: func(v Value) bool { return IsBareTypeNode(v) && v.Equal(bspec) },
		Construct: func(base, arg Value, r *Registry) ([]Value, error) {
			segs, err := readBitSegments(arg, r, "refine")
			if err != nil {
				return nil, err
			}
			info := ClassTypeInfo{
				Fields:       binaryFieldSchema(segs),
				ID:           GenerateObjectTypeID(),
				BinaryLayout: arg,
			}
			def := r.Types.MintType(info.ID, TClass)
			return []Value{NewClassType(def, info)}, nil
		},
	})
}

// ---- enum ----

// enumHandler — `enum [a b c]` builds a fixed-enumeration type
// (Enum, a subtype of Disjunct) whose alternatives are the list's
// elements. Words become Atoms automatically so `enum [red green
// blue]` doesn't require quoting. When the list carries a child-type
// constraint (`[ :T a b c]`), each element is validated against T
// before being added.
// enumReturns runs the (pure) enumHandler in check mode when the element list
// is concrete, so `enum [red green blue]` produces the real Enum VALUE
// (carrying its DisjunctInfo alternatives) rather than a bare TEnum carrier.
// Without the alternatives the result fails IsTypeBody, so `def Color enum […]`
// was wrongly rejected ("body must be a type value or literal, got Enum") and
// `tcmp` / `is` over the enum lost its members (compare.tsv). A non-concrete
// element list falls back to the bare TEnum carrier.
func enumReturns(args []Value, _ *Registry) []Value {
	if len(args) == 1 && IsConcrete(args[0]) {
		if out, err := enumHandler(args, nil, nil, nil); err == nil {
			return out
		}
	}
	return []Value{NewCarrier(TEnum)}
}

func enumHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	list := args[0]
	if !IsConcrete(list) {
		return nil, &BoruError{Code: "type_error", Detail: "enum: argument must be a concrete list"}
	}
	var childType Value
	hasChild := false
	if IsTypedList(list) {
		ci, _ := AsChildType(list)
		childType = ci.Child
		hasChild = childType.Parent != nil
	}
	elems, _ := AsList(list)
	alts := make([]Value, 0, elems.Len())
	for i := 0; i < elems.Len(); i++ {
		e := elems.Get(i)
		if IsWord(e) {
			w, _ := AsWord(e)
			e = NewAtom(w.Name)
		}
		if hasChild && !IsValueOfType(e, childType) {
			return nil, &BoruError{
				Code:   "type_error",
				Detail: "enum: element " + e.String() + " does not satisfy child type " + childType.String(),
			}
		}
		alts = append(alts, e)
	}
	return []Value{NewEnum(alts)}, nil
}

// ---- typeof ----

// typeofReturns returns the PRECISE type of a CONCRETE argument as a type
// literal in check mode, so `def T (typeof v)` gets a valid type body and
// `is` / `tcmp` over it are precise — `typeof (const 1)` is the singleton type
// (its only member is 1), not the bare `Type` carrier the def-type validator
// rejects ("body must be a type value or literal, got Type"; class.tsv). A
// NON-concrete (carrier) argument keeps the bare Type carrier: its runtime
// value's exact type is statically unknown, and TypeOf of a carrier yields only
// the carrier's static Parent — no more useful than Type to the consumers that
// matter, and gating to concrete keeps the blast radius off every runtime
// `typeof`.
func typeofReturns(args []Value, r *Registry) []Value {
	// PURE-CHECK ONLY (diagnostics, not a real compile). Under a REAL compile
	// (CompileCheck), folding `typeof <concrete>` to its precise type literal
	// changes lowering — it makes an interpolation hole `${42 typeof}`
	// const-foldable (it must stay an INTERP op) and can register a type-named
	// literal that collides at a later `def` — so the compile path keeps the
	// bare Type carrier. In pure check the refinement is what lets
	// `def One (typeof (const 1))` get a valid singleton type body.
	if r != nil && !r.Check.Compiling && len(args) == 1 && IsConcrete(args[0]) {
		return []Value{TypeOf(args[0])}
	}
	return []Value{NewCarrier(TType)}
}

func typeofHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Delegate to the canonical borueng implementation, which returns
	// a Type literal: concrete value → exact Parent; type literal →
	// its metatype (Type); implicit-map record shape → its metatype;
	// the value `none` (unique inhabitant of None) → None.
	return []Value{TypeOf(args[0])}, nil
}

// ---- as ----

// asTargetType resolves `as`'s TYPE operand (sig position 0) to its
// canonical lattice node. Only a bare nominal type literal qualifies —
// ascription redirects DISPATCH along the nominal lattice, so structural
// bodies (records, predicates, disjuncts) and plain values decline with
// as_error rather than silently ascribing something dispatch cannot walk.
func asTargetType(r *Registry, t Value) (*Type, error) {
	if !core.IsTypeLiteral(t) {
		return nil, r.BoruErrorHint("as_error",
			"as needs a type to dispatch as, got "+t.String(),
			"as", "spell the target type by name: value as FlexMap")
	}
	tc := t
	return CanonicalType(r, &tc), nil
}

// asValidate enforces the upcast-only rule: the value's own tag must
// conform to the ascribed type. `as` WIDENS dispatch — it never claims a
// subtype the value does not carry, which is what keeps the ascription
// sound with no runtime representation change. A check-mode DYNAMIC
// carrier is admitted gradually (its bound overlaps the target on the
// tree lattice) — the runtime step re-runs this validation over the
// concrete value.
func asValidate(r *Registry, target *Type, v Value) error {
	decline := func() error {
		name := target.Leaf()
		own := "nothing"
		if v.Parent != nil {
			own = v.Parent.Leaf()
		}
		return r.BoruErrorHint("as_error",
			"as: "+name+" is not a supertype of the value's type "+own+
				" — as widens dispatch only",
			"as", "ascribe an ancestor of the value's own type; to construct a subtype use def x:"+name+" instead")
	}
	if core.IsTypeLiteral(v) || v.Parent == nil {
		return decline()
	}
	if v.Dynamic {
		// Tree lattice: two nodes overlap iff one is an ancestor of the
		// other, so gradual admission is the two-way conformance test.
		if v.Parent.ConformsTo(target) || target.ConformsTo(v.Parent) {
			return nil
		}
		return decline()
	}
	if !v.Parent.ConformsTo(target) {
		return decline()
	}
	return nil
}

// asHandler implements `v as T` at run time: validate the upcast, then
// return v itself carrying the match-time ascription. No payload copy, no
// reparent — typeof, is, rendering, and equality all still see the real
// value; only the NEXT signature match reads the ascription (and arg
// delivery strips it — the match-time-only rule, design/OPEN-WORDS.1.md §9).
func asHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	target, err := asTargetType(r, args[0])
	if err != nil {
		return nil, err
	}
	if err := asValidate(r, target, args[1]); err != nil {
		return nil, err
	}
	out := args[1]
	out.SetAscribed(target)
	return []Value{out}, nil
}

// asReturns is the check-mode model of `as` — the exact static mirror of
// asHandler over carriers: a provable violation (bad type operand, a
// non-ancestor target over a known tag) is a GUARANTEED runtime error and
// emits the same as_error as a mirror diagnostic; a valid ascription
// returns the input carrier ascribed, so check-mode dispatch of the
// consuming word resolves the SAME signature the runtime match will. A
// dynamic input the tree lattice cannot refute passes gradually, modeled
// as a carrier at the target (the runtime validation proves conformance
// before any consumer dispatches on it).
func asReturns(args []Value, r *Registry) []Value {
	degrade := func(err error) []Value {
		if check.CheckAtUncaughtTopLevel(r) {
			code, detail := "as_error", err.Error()
			var ae *core.BoruError
			if errors.As(err, &ae) {
				code, detail = ae.Code, ae.Detail
			}
			core.CheckAddUniqueDiagnostic(r, code, detail, "as", args[1].Pos())
		}
		return []Value{core.NewDynamicCarrier(TAny)}
	}
	target, terr := asTargetType(r, args[0])
	if terr != nil {
		return degrade(terr)
	}
	v := args[1]
	if verr := asValidate(r, target, v); verr != nil {
		return degrade(verr)
	}
	if v.Dynamic && !v.Parent.ConformsTo(target) {
		// Gradually admitted: statically all that is known post-as is
		// "conforms to target" (the runtime validation guarantees it).
		c := core.NewCarrier(target)
		c.SetAscribed(target)
		return []Value{core.WithPos(c, v)}
	}
	out := v
	out.SetAscribed(target)
	return []Value{out}
}

// ---- is ----

func isHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	a, b := args[1], args[0]
	// A NAMED structural type RHS evaluates to its minted node (the
	// Stage 2 flip); recover the declared content so the redirect and
	// Unify arms below see the body shapes they have always answered
	// (design/legacy/TYPE-REPRESENTATION.1.ignore §N2). The Object/Table/Micron
	// redirect kinds resolve unconditionally — their `is` verdict is
	// the tag-identity redirect below, which only their content shape
	// selects. Other nodes whose kind enforces membership through the
	// Behavior (HasConstraintUnify — predicate / DepScalar / disjunct /
	// negation / FnUndef / surface) keep the node: the bare-node branch
	// consults the Behavior directly (the one-predicate doctrine).
	if IsBareTypeNode(b) {
		if content, ok := TypeContentOf(b); ok {
			if IsClassType(content) || IsTableType(content) || core.IsMicronType(content) ||
				!core.HasConstraintUnify(&b) {
				b = content
			}
		}
	}
	// Object/Table refinement RHS: the body is a populated type value
	// (Data carries ClassTypeInfo/TableTypeInfo), but its denoted
	// lattice node is at b.Parent. For tag-identity ("does a carry
	// T's tag?") we want to compare a.Parent against the lattice
	// node. Without this, the handler falls through to UnifyR, which
	// returns the body (not the instance) and the downstream
	// ValuesEqual check rejects an instance-vs-body comparison even
	// though the instance plainly carries the type's tag.
	//
	// This is the `is` analogue of canonicalizing in dispatch — same
	// underlying question, different layers. Disjunct / DepScalar /
	// bare-refine bodies have Data==nil so they already route through
	// the b.Data==nil branch below; only Object/Table bodies need
	// this redirect.
	if (IsClassType(b) || IsTableType(b) || core.IsMicronType(b)) && b.Parent != nil {
		latticeNode := b.Parent
		return []Value{NewBoolean(a.Parent.Equal(latticeNode) || a.Parent.IsSubtypeOf(latticeNode))}, nil
	}
	// Surface RHS: membership is the conformance set, answered by the
	// minted node's surfaceUnifier via the v.Is(t) doctrine — explicit
	// `exposes` declarations only, walking a's parent chain so subclass
	// instances of an exposer conform.
	if IsSurfaceType(b) && b.Parent != nil {
		return []Value{NewBoolean(a.Is(b.Parent))}, nil
	}
	// Note: a consolidation attempt to delegate structural-pattern
	// RHS (typed list, typed map, record shape) to IsValueOfType was
	// tried and reverted. `IsValueOfType` uses subset semantics on
	// record shapes ("extra keys in v are ignored") while production
	// `is` uses strict-exact-shape. They look like the same operation
	// but answer different sub-questions. Real consolidation requires
	// choosing one shape semantic and migrating the loser; that's a
	// separate design call documented in PLAN.md.
	if b.Parent.Equal(TFnUndef) && IsAtom(a) {
		name, _ := AsAtom(a)
		if top, ok := r.Defs.Top(name); ok {
			if top.Parent.Equal(TFunction) {
				a = top
			}
		}
	}
	// A concrete function RHS is a PREDICATE (`3 is even?`). A bare
	// type node whose Parent happens to be TFunction is NOT — it is a
	// minted subtype of Function (the boru:minilang partial kinds,
	// MiniLang.Re / …) and must fall through to the type-literal
	// branch below, where a.Is(bNode) runs its member predicate.
	if b.Parent.Equal(TFunction) && !IsBareTypeNode(b) {
		_, matched, err := r.RunPredicate(b, a)
		if err != nil {
			return []Value{NewBoolean(false)}, nil
		}
		return []Value{NewBoolean(matched)}, nil
	}
	if IsBareTypeNode(b) {
		// b is a type literal; its denoted lattice node is &b (a
		// type literal is a by-value copy of its node). Post the
		// Any-root unification TType.Parent == TAny, so the old
		// `b.Parent.Root() == TType` check no longer identifies the
		// Type/-hierarchy — we test the node itself directly.
		bNode := &b
		if bNode.Equal(TType) {
			// `v is Type` — v must be a TYPE: a bare type literal, a
			// structural type body (record shape, typed list/map,
			// disjunct, fn-shape), or a Function / Disjunct / Enum /
			// FunctionSignature value. Concrete scalars / lists / maps
			// and the value `none` are not types; carriers are abstract
			// values, not types.
			if a.Carrier {
				return []Value{NewBoolean(false)}, nil
			}
			return []Value{NewBoolean(IsBareTypeNode(a) || IsTypeBody(a) || IsRecordShape(a) || a.Parent.ConformsTo(TType))}, nil
		}
		if bNode.ConformsTo(TType) {
			// Type/-rooted subtype RHS (`Function` / `Disjunct` / `Enum`
			// / `FunctionSignature`): route through the one-predicate
			// doctrine, a.Is(bNode) — for the default-behavior kernel
			// nodes this IS the old plain a.Parent.ConformsTo(bNode)
			// subtype check (DefaultBehavior.Match delegates there),
			// and a MEMBER type under the branch (the boru:minilang
			// partial kinds, MiniLang.Re / …) runs its predicate.
			//
			// EXCEPT a user DISJUNCT node with a concrete candidate: its
			// Match is deliberately loose on the newtype-alternative
			// swap (dispatch decomposes the union to base families —
			// edge-types-2's DIVERGENCE PIN), so `is` keeps its stricter
			// answer through the Unify + value-identity path below:
			// `42 is (P tor String)` is false while `42 g` dispatches.
			if !core.IsDisjunctTypeNode(b) || !IsConcrete(a) {
				return []Value{NewBoolean(a.Is(bNode))}, nil
			}
		}
		// A const singleton RHS (and every other membership type) answers
		// `is` through the shared Unify path below — `1 is (const 1)` →
		// true, `1.0 is (const 1)` → false (same-base strict) — so it needs
		// no special-case here. (Const used to route through its bespoke
		// Behavior.Match; converging it onto MemberBehavior gave it a
		// matching Unify, making this branch redundant.)
		//
		// Both sides are bare type literals: the question is purely
		// lattice subtyping. Settle directly via IsSubtypeOf rather
		// than via Unify, whose List/Map/DepScalar/FnDef branches
		// short-circuit family relationships and would reject a
		// user-minted subtype (e.g. `def Foo refine List`) against
		// its base family literal.
		if IsBareTypeNode(a) {
			aNode := &a
			return []Value{NewBoolean(aNode.Equal(bNode) || aNode.IsSubtypeOf(bNode))}, nil
		}
		// One-predicate doctrine (design/REFINE-NEWTYPE-VS-SUBSET.10.md):
		// a CONCRETE value against a bare-node RHS whose Behavior carries
		// a CUSTOM matcher asks that Match first — the same predicate
		// signature dispatch asks — so a type whose Match admits values
		// beyond nominal ancestry (boru:matrix-util's per-import tensor
		// mints match structurally across imports) answers `is`
		// consistently with dispatch. Gated OFF DefaultBehavior nodes:
		// their Match is plain conformance, whose `is` nuances the paths
		// below own (notably the pinned `none is Any → false` — absence
		// is outside Any's membership even though ConformsTo admits it).
		// A false here is not final: the Unify path below still owns the
		// membership types it always answered (const singletons,
		// DepScalar construction), so this only ADDS agreement, never
		// removes an admission.
		if bNode.Behavior() != nil && bNode.Behavior() != DefaultBehavior && a.Is(bNode) {
			// A DISJUNCT node's Match is deliberately loose on the
			// newtype-alternative swap (dispatch decomposes the union to
			// base families — edge-types-2's DIVERGENCE PIN). `is` keeps
			// its stricter answer by sending a concrete candidate through
			// the Unify + value-identity path below, where a swap
			// admission (the unified result being the alternative's bare
			// node, not the value) is declined: `42 is (P tor String)` is
			// false while `42 g` dispatches.
			if !core.IsDisjunctTypeNode(b) || !IsConcrete(a) {
				return []Value{NewBoolean(true)}, nil
			}
		}
	}
	unified, ok := core.UnifyR(a, b, r)
	if !ok {
		return []Value{NewBoolean(false)}, nil
	}
	resolved := ResolveWordsDeep(a)
	if !unified.Parent.Equal(resolved.Parent) {
		return []Value{NewBoolean(false)}, nil
	}
	if !ValuesEqual(unified, resolved) {
		return []Value{NewBoolean(false)}, nil
	}
	return []Value{NewBoolean(true)}, nil
}

// ---- teq ----

// teqHandler implements strict type equality. Both args must be
// IsTypeBody; otherwise return false. Bare type literals compare by
// lattice node Equal (ID identity), structural type bodies (record /
// disjunct / object / etc.) compare via ValuesEqual. Distinct from
// `is`, which is subtype-membership and is asymmetric on its RHS
// (`5 is Integer` true, `Integer is 5` false). `teq` is symmetric and
// rejects non-type values from either side.
func teqHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	a, b := args[1], args[0]
	if !IsTypeBody(a) || !IsTypeBody(b) {
		return []Value{NewBoolean(false)}, nil
	}
	if IsBareTypeNode(a) && IsBareTypeNode(b) {
		aNode := &a
		bNode := &b
		return []Value{NewBoolean(aNode.Equal(bNode))}, nil
	}
	return []Value{NewBoolean(ValuesEqual(a, b))}, nil
}

// ---- tis ----

// tisHandler implements `tis` — the pure-lattice variant of `is`. It
// reduces both operands to the lattice node they denote and answers ONE
// question: does a's node sit on b's node's parent chain (ConformsTo /
// IsAncestor)? Unlike `is`, it consults nothing else — no Behavior.Match,
// no Unify, no predicate run, no membership predicate, no structural-shape
// match. It traverses Value.Parent and only Value.Parent, so it is the
// nominal/lattice subtype test:
//
//	5 tis Integer            → true   (Integer ≤ Integer)
//	5 tis Number             → true   (Integer ≤ Number)
//	5 tis String             → false  (wrong family)
//	Integer tis Number       → true   (both literals: lattice subtyping)
//	5 tis Any                → true   (Any is the lattice top)
//
// The deliberate divergence from `is` is that `tis` is TAG-ONLY: a
// predicate / refine / membership RHS is reduced to the lattice node it
// hangs off, so `100 tis (Integer gt 10)` and `5 tis (Integer gt 10)` are
// BOTH true (base tag Integer ≤ Integer) where `is` runs the predicate and
// returns true / false respectively. Likewise a bare-refine newtype tag
// only matches up its own chain, not its base: `2 tis Pos` is false
// because Integer is not below the minted Pos node.
func tisHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	a, b := args[1], args[0]
	return []Value{NewBoolean(tisNode(a).ConformsTo(tisNode(b)))}, nil
}

// tisNode returns the lattice node a value denotes for `tis`: a bare type
// literal IS its node (a by-value copy, so &v carries the right ID / In /
// Out / Parent — mirroring isHandler's `bNode := &b`); any other value's
// node is its tag, v.Parent.
func tisNode(v Value) *Type {
	if IsBareTypeNode(v) {
		return &v
	}
	return v.Parent
}

// ---- tpartial ----

// TPartialModuleNatives holds `tpartial`, moved out of core into the
// boru:type-util module (TypeUtil.tpartial). Its handler stays here.
var TPartialModuleNatives = []NativeFunc{
	{
		Name: "tpartial",
		Signatures: []Signature{{
			Args:    []*Type{TAny},
			Impl:    Go(tpartialHandler),
			Returns: []*Type{TType}, BarrierPos: -1,
		}},
	},
}

// tpartialHandler wraps every field of a Record or Object type in
// `T | None`. Idempotent: a field whose value already includes None
// is left unchanged. For Object types, inherited fields are flattened
// into the result's own field map and the result is registered as a
// fresh anonymous Object type (lattice parent: Object root) — the
// partial is NOT a subtype of the input because boru's lattice runs
// the other way (a child requires more, not less).
func tpartialHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	t := args[0]
	// A NAMED type evaluates to its node (the Stage 2 flip); the
	// fields to partialize are the node's recorded content.
	if body, ok := TypeContentOf(t); ok && IsBareTypeNode(t) {
		t = body
	}
	switch {
	case IsRecordType(t):
		rec, _ := AsRecordType(t)
		return []Value{NewRecordType(partializeFields(rec.Fields))}, nil
	case IsClassType(t):
		info, _ := AsClassType(t)
		newFields := partializeFields(info.AllFields())
		id := GenerateObjectTypeID()
		newInfo := ClassTypeInfo{
			Fields: newFields,
			Parent: nil,
			ID:     id,
		}
		def := r.Types.MintType(id, TClass)
		return []Value{NewClassType(def, newInfo)}, nil
	default:
		return nil, r.BoruError("type_error",
			fmt.Sprintf("tpartial: argument must be a Record or Object type, got %s", t.String()),
			"tpartial")
	}
}

func partializeFields(fields *OrderedMap) *OrderedMap {
	result := NewOrderedMap()
	for _, k := range fields.Keys() {
		ft, _ := fields.Get(k)
		result.Set(k, makeOptionalType(ft))
	}
	return result
}

// makeOptionalType returns `t | None` as a Disjunct, or `t` unchanged
// if t already includes None as an alternative (or IS None).
func makeOptionalType(t Value) Value {
	if IsNoneShape(t) {
		return t
	}
	alts := FlattenDisjunctAlts(t)
	for _, alt := range alts {
		if IsNoneShape(alt) {
			return t
		}
	}
	alts = append(alts, NewTypeLiteral(TNone))
	simplified := SimplifyDisjunctAlts(alts)
	if len(simplified) == 1 {
		return simplified[0]
	}
	return NewDisjunct(simplified)
}

// ---- guard ----

// guardReturns is the check-mode return for `guard`: it yields the
// guarded value when the condition holds, else None — so the result type
// is value|None, not Any. Narrowing only; the runtime result is always
// the value or None, both covered by the join.
func guardReturns(args []Value, _ *Registry) []Value {
	if len(args) == 0 {
		return []Value{NewCarrier(TAny)}
	}
	return []Value{JoinCarriers(args[0], NewCarrier(TNone))}
}

func guardHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	val := args[0]
	cond, err := args[1].AsConcreteBoolean()
	if err != nil {
		return nil, fmt.Errorf("guard: condition must be Boolean, got %s", args[1].Parent.String())
	}
	if cond {
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

// ---- base ----

func baseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	v := args[0]
	// For a type literal the denoted lattice node is &v (the value
	// IS the type); for a concrete value it's v.Parent.
	var t *Type
	if IsBareTypeNode(v) {
		t = &v
	} else {
		t = v.Parent
	}
	result, err := BaseValue(t)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// torHandler, torReturnsFn, tandHandler: moved to eng/go/core_boolean.go.
// tand's `tall` reduction (below) calls core.TandValues directly.

// ---- tany / tall ----

// typeAlgebraListFold is the check-mode fold for tany/tall: both are PURE
// type computations (FlattenDisjunctAlts / SimplifyDisjunctAlts /
// TandValues) over a concrete list, so when every element is statically
// known — a concrete value, a bare type node, a DepScalar constraint, a
// disjunct — the checked result IS the runtime result. Any carrier /
// dynamic / undefined element falls back to the declared dynamic(Any).
func typeAlgebraListFold(handler Handler) ReturnsFunc {
	return func(args []Value, r *Registry) []Value {
		dyn := []Value{NewDynamicCarrier(TAny)}
		if len(args) != 1 || !IsConcrete(args[0]) {
			return dyn
		}
		list, err := AsList(args[0])
		if err != nil || list.IsNil() {
			return dyn
		}
		for i := 0; i < list.Len(); i++ {
			e := list.Get(i)
			if e.Carrier || e.Dynamic || e.Undefined {
				return dyn
			}
		}
		out, herr := handler(args, nil, nil, r)
		if herr != nil || len(out) != 1 {
			return dyn
		}
		// A Disjunct/Enum RESULT is a type-as-VALUE; left concrete, every
		// carrier-side consumer (and the soundness comparator) reads a
		// checked disjunct as a union CARRIER of its alternatives instead.
		// Ride it as a dynamic carrier bound to the disjunct — same value
		// knowledge, gradual modality.
		if IsDisjunct(out[0]) {
			return []Value{NewDynamicCarrierValue(out[0])}
		}
		return out
	}
}

func tanyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("tany_error", "tany: expected a concrete list", "tany")
	}
	list, _ := AsList(args[0])
	n := list.Len()
	if n == 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	if n == 1 {
		return []Value{list.Get(0)}, nil
	}
	var alts []Value
	for i := 0; i < n; i++ {
		alts = append(alts, FlattenDisjunctAlts(list.Get(i))...)
	}
	simplified := SimplifyDisjunctAlts(alts)
	if len(simplified) == 0 {
		return []Value{NewTypeLiteral(TNever)}, nil
	}
	if len(simplified) == 1 {
		return []Value{simplified[0]}, nil
	}
	return []Value{NewDisjunct(simplified)}, nil
}

func tallHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("tall_error", "tall: expected a concrete list", "tall")
	}
	list, _ := AsList(args[0])
	n := list.Len()
	if n == 0 {
		return []Value{NewTypeLiteral(TAny)}, nil
	}
	acc := list.Get(0)
	for i := 1; i < n; i++ {
		acc = TandValues(acc, list.Get(i))
	}
	return []Value{acc}, nil
}

// ---- convert ----

// convertOptsPattern returns the Options pattern for the 3-arg
// `convert` variant:
// {base?: String|None, truthy?: Boolean|None, accuracy?: String|None,
//
//	places?: Integer|None}.
//
// Every field is spelled `T|None` (not a bare type literal) so it is
// OPTIONAL: a bare type literal has no options-default and would make
// the key required (unify_options.go::optionsDefault), breaking every
// 3-arg call that omits it. Absent keys simply do not appear in the
// runtime map, so the handler's presence checks (`m.Get`) read them as
// unset.
func convertOptsPattern() Value {
	baseOpts := NewOrderedMap()
	baseOpts.Set("base", NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)}))
	baseOpts.Set("truthy", NewDisjunct([]Value{NewTypeLiteral(TBoolean), NewTypeLiteral(TNone)}))
	// accuracy enables (and disambiguates) the otherwise-declined
	// Float → BigDecimal conversion — see floatToBigDecimal.
	baseOpts.Set("accuracy", NewDisjunct([]Value{NewTypeLiteral(TString), NewTypeLiteral(TNone)}))
	// places is the companion to accuracy:"round" — the number of
	// decimal places to round the Float's exact value to.
	baseOpts.Set("places", NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TNone)}))
	return NewOptionsType(baseOpts)
}

// convertBoolOptsKinds is the options-map contract convertOptsPattern
// expresses as an OptionsType, restated as data so the Boolean form can
// CHECK it rather than match on it. Key → the type its value must
// conform to; a `none` value is always allowed (the pattern's `tor None`
// arm), meaning "option not supplied".
var convertBoolOptsKinds = map[string]*Type{
	"base":     TString,
	"truthy":   TBoolean,
	"accuracy": TString,
	"places":   TInteger,
}

// convertBoolOptsHandler is `convert Boolean <opts> <src>`. It exists
// because the Boolean form's SOURCE slot is `Any` (NUR053), which makes
// a pattern on the options slot actively harmful: a map that failed the
// pattern would fall through to the arity-2 row and be coerced AS THE
// SOURCE, silently answering for the wrong value while the real source
// stranded on the stack. So this row matches every 3-arg Boolean call
// and validates here, naming the offending key.
//
// On a well-formed map it delegates unchanged to convert3Handler, which
// is what makes the repair independent of signature sort order: whichever
// of the two Boolean rows the sorter puts first, a valid call computes
// the same answer.
func convertBoolOptsHandler(args []Value, named map[string]Value, body []Value, r *Registry) ([]Value, error) {
	// Arity guard FIRST, and it declines rather than delegating: the
	// no-signature recovery can assume a sig with a short arg window, and
	// convert3Handler indexes args[2] unconditionally — so delegating a
	// short window would panic, which this codebase does not permit
	// (ADR-005). Found by the paired negative test, not by inspection.
	if len(args) != 3 {
		return nil, r.BoruError("convert_error",
			"convert Boolean: the options form needs a target, an options map and a source",
			"convert")
	}
	if IsConcrete(args[1]) {
		if m, err := AsMap(args[1]); err == nil && m != nil {
			for _, k := range m.Keys() {
				want, known := convertBoolOptsKinds[k]
				if !known {
					return nil, r.BoruErrorHint("convert_error",
						"convert Boolean: unknown option key "+k,
						"convert", "known options are base, truthy, accuracy, places")
				}
				v, _ := m.Get(k)
				if v.Parent == nil || v.Parent.ConformsTo(TNone) || v.Parent.ConformsTo(want) {
					continue
				}
				return nil, r.BoruError("convert_error",
					"convert Boolean: option "+k+" expects "+want.Name(), "convert")
			}
		}
	}
	return convert3Handler(args, named, body, r)
}

// yamlTruthy maps a string to a boolean using the YAML 1.1 boolean
// token set (matched case-insensitively, surrounding whitespace
// trimmed). It reports whether the string was a recognised token; a
// caller that gets ok=false falls back to presence coercion.
func yamlTruthy(s string) (val bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "on":
		return true, true
	case "n", "no", "false", "off":
		return false, true
	default:
		return false, false
	}
}

// coerceBooleanTruthy is the `convert Boolean {truthy: true} <src>`
// rule: a String is first matched against the YAML boolean tokens
// (yamlTruthy); anything not a recognised token — and any non-String
// source — falls back to the ordinary presence coercion (CoerceBoolean).
// It never raises.
func coerceBooleanTruthy(src Value) bool {
	if src.Parent.ConformsTo(TString) {
		if val, ok := yamlTruthy(ValToString(src)); ok {
			return val
		}
	}
	return CoerceBoolean(src)
}

// convertTo performs the actual scalar-type conversion.
func convertTo(src Value, targetType *Type, base string) (Value, error) {
	switch {
	case targetType.ConformsTo(TString):
		if base == "" {
			return NewString(ValToString(src)), nil
		}
		if !src.Parent.ConformsTo(TInteger) {
			return Value{}, fmt.Errorf("convert: base %q only supported for integer to string", base)
		}
		n, _ := AsInteger(src)
		var s string
		switch base {
		case "hex":
			s = strconv.FormatInt(n, 16)
		case "HEX":
			s = strings.ToUpper(strconv.FormatInt(n, 16))
		case "bin":
			s = strconv.FormatInt(n, 2)
		case "oct":
			s = strconv.FormatInt(n, 8)
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		return NewString(s), nil

	case targetType.ConformsTo(TFloat):
		// Big → Float is a deliberately lossy projection (the exact
		// value is approximated to the nearest binary float64).
		if src.Parent.ConformsTo(TBigInteger) || src.Parent.ConformsTo(TBigDecimal) {
			f, err := AsFloatApprox(src)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %s to float", src.Parent.Leaf())
			}
			return NewFloat(f), nil
		}
		text := ValToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to float", text)
		}
		return NewFloat(f), nil

	case targetType.ConformsTo(TBigInteger):
		return convertToBigInteger(src, base)

	case targetType.ConformsTo(TBigDecimal):
		return convertToBigDecimal(src, base)

	case targetType.ConformsTo(TNumber) || targetType.ConformsTo(TInteger):
		// Big → Integer: BigInteger range-checks int64; BigDecimal
		// truncates toward zero, then range-checks.
		if src.Parent.ConformsTo(TBigInteger) {
			n, _ := AsBigInteger(src)
			if !n.IsInt64() {
				return Value{}, fmt.Errorf("convert: %s overflows Integer (int64) range", FormatBigInteger(n))
			}
			return NewInteger(n.Int64()), nil
		}
		if src.Parent.ConformsTo(TBigDecimal) {
			n, err := bigDecimalToBigIntTrunc(src)
			if err != nil {
				return Value{}, err
			}
			if !n.IsInt64() {
				return Value{}, fmt.Errorf("convert: %s overflows Integer (int64) range", FormatBigInteger(n))
			}
			return NewInteger(n.Int64()), nil
		}
		// Numeric source: convert directly instead of stringifying and
		// re-parsing. An Integer passes through; a Float truncates toward
		// zero. Previously `convert Integer 3.0` stringified to "3.0" and
		// then failed ParseInt on the decimal point (WAT-AUDIT.5.md §Q/W).
		// A base implies a string source in a particular radix, so the
		// fast path only applies to the plain (base == "") form.
		if base == "" {
			if src.Parent.ConformsTo(TInteger) {
				n, _ := AsInteger(src)
				return NewInteger(n), nil
			}
			if src.Parent.ConformsTo(TFloat) {
				f, _ := AsFloat(src)
				if math.IsNaN(f) || math.IsInf(f, 0) || f < float64(math.MinInt64) || f >= float64(math.MaxInt64) {
					return Value{}, fmt.Errorf("convert: %s overflows Integer (int64) range", ValToString(src))
				}
				return NewInteger(int64(f)), nil
			}
		}
		text := ValToString(src)
		if base == "" {
			n, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return Value{}, fmt.Errorf("convert: cannot convert %q to number", text)
			}
			return NewInteger(n), nil
		}
		var numBase int
		switch base {
		case "hex":
			numBase = 16
		case "bin":
			numBase = 2
		case "oct":
			numBase = 8
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		n, err := strconv.ParseInt(text, numBase, 64)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to number (base %d)", text, numBase)
		}
		return NewInteger(n), nil

	case targetType.ConformsTo(TBoolean):
		// Boolean is a COERCION, not a parse — it is exactly the
		// truthiness rule words like `if` apply (CoerceBoolean): a value
		// is false iff it is absent/empty (empty String, 0, none, empty
		// collection) and true otherwise. String CONTENT is never
		// inspected, so "false" and "true" are ordinary non-empty strings
		// and both coerce to true. Turning the literal words "true" /
		// "false" into booleans is a separate parse operation that lives
		// elsewhere, not here.
		return NewBoolean(CoerceBoolean(src)), nil

	case targetType.Equal(TAtom):
		return NewAtom(ValToString(src)), nil

	default:
		return Value{}, fmt.Errorf("convert: unsupported target type %s", targetType)
	}
}

// convertScalarReturns wraps the fresh-instance result model for the
// [Scalar Scalar] convert overload with its one TYPE-decidable compile failure:
// a Float source into a Big target ALWAYS raises (convertToBigInteger /
// convertToBigDecimal reject exactly by source type — no value can pass),
// so a source the checker has PROVEN Float (strict, non-dynamic) is
// flagged on the top-level straight line with the byte-identical runtime
// message. Every other shape keeps the fresh-instance carrier untouched.
func convertScalarReturns(args []Value, r *Registry) []Value {
	// args[0] is the TARGET type literal — the value IS its lattice node,
	// so resolve it via ValueType (its .Parent is the node's parent).
	if atUncaughtTopLevel(r) && len(args) == 2 && !args[1].Dynamic &&
		args[1].Parent != nil && args[1].Parent.ConformsTo(TFloat) && ValueType(args[0]) != nil {
		switch {
		case ValueType(args[0]).ConformsTo(TBigInteger):
			core.CheckAddUniqueDiagnostic(r, "convert_error",
				"convert: cannot convert Float to BigInteger (a binary Float is inexact; convert to Integer first)",
				"convert", args[1].Pos())
		case ValueType(args[0]).ConformsTo(TBigDecimal):
			core.CheckAddUniqueDiagnostic(r, "convert_error",
				"convert: cannot convert Float to BigDecimal (a binary Float is already rounded; build the BigDecimal from a String or the 0d literal)",
				"convert", args[1].Pos())
		}
	}
	return ReturnsFreshInstance(0)(args, r)
}

// convertToBigInteger converts a scalar source to BigInteger. Exact
// sources (Integer, BigInteger) and the truncated integer part of a
// BigDecimal are accepted; a String is parsed exactly (base-aware). A
// Float is DECLINED — a binary Float is inexact, so silently absorbing it
// into an arbitrary-precision exact type would re-introduce the very
// rounding error the Big types exist to avoid (convert to Integer first
// if a truncating projection is really wanted).
func convertToBigInteger(src Value, base string) (Value, error) {
	switch {
	case src.Parent.ConformsTo(TBigInteger):
		return src, nil
	case src.Parent.ConformsTo(TBigDecimal):
		n, err := bigDecimalToBigIntTrunc(src)
		if err != nil {
			return Value{}, err
		}
		return NewBigInteger(n), nil
	case src.Parent.ConformsTo(TFloat):
		return Value{}, fmt.Errorf("convert: cannot convert Float to BigInteger (a binary Float is inexact; convert to Integer first)")
	case src.Parent.ConformsTo(TInteger):
		i, _ := src.AsConcreteInteger()
		return NewBigInteger(big.NewInt(i)), nil
	default:
		text := ValToString(src)
		var numBase int
		switch base {
		case "", "dec":
			numBase = 10
		case "hex", "HEX":
			numBase = 16
		case "bin":
			numBase = 2
		case "oct":
			numBase = 8
		default:
			return Value{}, fmt.Errorf("convert: unknown base %q", base)
		}
		n, ok := new(big.Int).SetString(text, numBase)
		if !ok {
			return Value{}, fmt.Errorf("convert: cannot convert %q to BigInteger", text)
		}
		return NewBigInteger(n), nil
	}
}

// convertToBigDecimal converts a scalar source to BigDecimal. Integer and
// BigInteger widen exactly; BigDecimal is returned as-is; a String is
// parsed exactly via apd. A Float is DECLINED for the same reason as
// convertToBigInteger (the float is already rounded — WAT Exhibit L).
func convertToBigDecimal(src Value, base string) (Value, error) {
	if base != "" {
		return Value{}, fmt.Errorf("convert: base %q is not supported for BigDecimal", base)
	}
	switch {
	case src.Parent.ConformsTo(TBigDecimal):
		return src, nil
	case src.Parent.ConformsTo(TBigInteger):
		n, _ := AsBigInteger(src)
		d, _, err := apd.NewFromString(n.String())
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return Value{}, fmt.Errorf("convert: cannot convert %s to BigDecimal", FormatBigInteger(n))
		}
		return NewBigDecimal(d), nil
	case src.Parent.ConformsTo(TFloat):
		return Value{}, fmt.Errorf("convert: cannot convert Float to BigDecimal (a binary Float is already rounded; build the BigDecimal from a String or the 0d literal)")
	case src.Parent.ConformsTo(TInteger):
		i, _ := src.AsConcreteInteger()
		return NewBigDecimal(apd.New(i, 0)), nil
	default:
		text := ValToString(src)
		d, _, err := apd.NewFromString(text)
		if err != nil {
			return Value{}, fmt.Errorf("convert: cannot convert %q to BigDecimal", text)
		}
		return NewBigDecimal(d), nil
	}
}

// bigDecimalToBigIntTrunc returns the integer part of a BigDecimal,
// truncated toward zero (the C / Go float→int convention). Done via the
// plain decimal text so the sign and scale are handled uniformly.
func bigDecimalToBigIntTrunc(src Value) (*big.Int, error) {
	d, err := AsBigDecimal(src)
	if err != nil {
		return nil, err
	}
	s := d.Text('f')
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if s == "" || s == "-" { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		s = "0"
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("convert: cannot truncate %s to an integer", FormatBigDecimal(d))
	}
	return n, nil
}

// floatToBigDecimal performs the opt-in Float → BigDecimal conversion
// that convertToBigDecimal declines by default. A binary Float is inexact,
// so there is no single honest BigDecimal for it; the `accuracy` option
// forces the caller to state which reading they want:
//
//   - "exact"    the true dyadic value the float64 holds. 0.1 becomes
//     0d0.1000000000000000055511151231257827021181583404541015625.
//   - "shortest" the shortest decimal that round-trips to the same
//     float64 — what the literal reads as. 0.1 becomes 0d0.1.
//   - "round"    the exact value rounded to `places` decimal places,
//     half away from zero (the companion `places` option is
//     required; e.g. 3.14159 with places 2 becomes 0d3.14).
//
// A non-finite Float (NaN / ±Inf) has no decimal expansion and is
// declined. This is reached only from the 3-arg convert with an accuracy
// option present — without it, Float → BigDecimal stays a hard error.
//
// `places` is carried as an int64 so an out-of-range request is caught
// before any int narrowing (a 32-bit `int(places)` could otherwise wrap
// a billion into a small or negative value and slip past the bound).
func floatToBigDecimal(f float64, accuracy string, places int64, placesSet bool) (Value, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Value{}, fmt.Errorf("convert: cannot convert non-finite Float (%s) to BigDecimal", FormatFloat(f))
	}
	switch accuracy {
	case "exact":
		if placesSet {
			return Value{}, fmt.Errorf("convert: accuracy %q does not take a places option", accuracy)
		}
		// The exact value of a binary float is dyadic (denominator a
		// power of two), so it terminates as a finite decimal. big.Rat
		// holds it exactly; FloatString(k) with k = log2(denominator)
		// renders every fractional digit without rounding.
		r := new(big.Rat).SetFloat64(f)
		k := r.Denom().BitLen() - 1
		d, _, err := apd.NewFromString(r.FloatString(k))
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return Value{}, fmt.Errorf("convert: cannot convert Float to BigDecimal")
		}
		return NewBigDecimal(applyFloatSign(f, d)), nil
	case "shortest":
		if placesSet {
			return Value{}, fmt.Errorf("convert: accuracy %q does not take a places option", accuracy)
		}
		// apd.SetFloat64 uses strconv's shortest round-tripping form
		// (it already carries the sign bit, including a negative zero).
		d := new(apd.Decimal)
		if _, err := d.SetFloat64(f); err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return Value{}, fmt.Errorf("convert: cannot convert Float to BigDecimal")
		}
		return NewBigDecimal(d), nil
	case "round":
		if !placesSet {
			return Value{}, fmt.Errorf("convert: accuracy \"round\" requires a places option (the number of decimal places)")
		}
		if places < 0 {
			return Value{}, fmt.Errorf("convert: places must be non-negative, got %d", places)
		}
		// A float64 has at most maxFloatFractionDigits fractional digits
		// (the smallest subnormal, 2^-1074), so rounding to more places
		// than that only appends zeros while FloatString would still
		// materialise every one — an unbounded string from a small
		// number. Reject the impractical request rather than hang.
		if places > maxFloatFractionDigits {
			return Value{}, fmt.Errorf("convert: places %d exceeds the maximum %d (a float64 has no more fractional digits)", places, maxFloatFractionDigits)
		}
		// FloatString rounds the exact rational value to `places` digits,
		// half away from zero — so the rounding sees the true stored
		// value, not the already-shortened display form.
		r := new(big.Rat).SetFloat64(f)
		d, _, err := apd.NewFromString(r.FloatString(int(places)))
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return Value{}, fmt.Errorf("convert: cannot convert Float to BigDecimal")
		}
		return NewBigDecimal(applyFloatSign(f, d)), nil
	default:
		return Value{}, fmt.Errorf("convert: unknown accuracy %q (want \"exact\", \"shortest\", or \"round\")", accuracy)
	}
}

// maxFloatFractionDigits is the most fractional decimal digits any finite
// float64 can carry: the smallest positive subnormal is 2^-1074, whose
// exact decimal expansion has exactly 1074 places.
const maxFloatFractionDigits = 1074

// applyFloatSign restores a negative sign bit that big.Rat drops for a
// zero: big.Rat has no signed zero, so the exact/round expansions of
// -0.0 (and of a negative value that rounds to zero) come back as a
// positive 0d0. apd's BigDecimal can represent signed zero, and boru
// treats -0.0 as a first-class Float, so we preserve math.Signbit here
// to stay consistent with the shortest mode (which keeps it natively).
func applyFloatSign(f float64, d *apd.Decimal) *apd.Decimal {
	if math.Signbit(f) && d.IsZero() {
		d.Negative = true
	}
	return d
}

func convertIdealHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	target := args[0]
	src := args[1]
	if target.Data != nil {
		return nil, r.BoruError("convert_error", "convert: first argument must be a type literal (Map or List)", "convert")
	}
	t := ValueType(target)
	switch {
	case t.Equal(TMap):
		m, err := core.ConvertIdealToMap(src)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return nil, r.BoruError("convert_error", "convert to Map: "+err.Error(), "convert")
		}
		return []Value{m}, nil
	case t.Equal(TList):
		l, err := core.ConvertIdealToList(src)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return nil, r.BoruError("convert_error", "convert to List: "+err.Error(), "convert")
		}
		return []Value{l}, nil
	default:
		return nil, r.BoruError("convert_error", "convert: an Ideal converts only to Map or List, got "+t.String(), "convert")
	}
}

func convert2Handler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	targetType := args[0]
	src := args[1]
	if targetType.Data != nil {
		return nil, r.BoruError("convert_error", fmt.Sprintf("convert: first argument must be a type literal, got %s", targetType.Parent), "convert")
	}
	result, err := convertTo(src, ValueType(targetType), "")
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

func convert3Handler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	targetType := args[0]
	opts := args[1]
	src := args[2]
	if targetType.Data != nil {
		return nil, r.BoruError("convert_error", fmt.Sprintf("convert: first argument must be a type literal, got %s", targetType.Parent), "convert")
	}

	base := ""
	truthy := false
	accuracy := ""
	var places int64
	placesSet := false
	if opts.Data != nil {
		m, _ := AsMap(opts)
		if m != nil {
			if bv, ok := m.Get("base"); ok {
				base = ValToString(bv)
			}
			if tv, ok := m.Get("truthy"); ok {
				truthy, _ = AsBoolean(tv)
			}
			if av, ok := m.Get("accuracy"); ok && av.Parent != nil && av.Parent.ConformsTo(TString) {
				accuracy = ValToString(av)
			}
			if pv, ok := m.Get("places"); ok && pv.Parent != nil && pv.Parent.ConformsTo(TInteger) {
				places, _ = AsInteger(pv)
				placesSet = true
			}
		}
	}

	// `truthy: true` turns a Boolean conversion into a YAML-style parse
	// (yes/no/true/false/on/off, then presence fallback). It only applies
	// to a Boolean target; for any other target it is inert.
	if truthy && ValueType(targetType).ConformsTo(TBoolean) {
		return []Value{NewBoolean(coerceBooleanTruthy(src))}, nil
	}

	// `accuracy` is the explicit opt-in that lets a Float become a
	// BigDecimal (convertToBigDecimal declines it by default because a
	// binary Float is inexact). It applies only to a Float → BigDecimal
	// conversion; for any other source/target it is inert, exactly like
	// truthy on a non-Boolean target.
	if accuracy != "" && src.Parent.ConformsTo(TFloat) && ValueType(targetType).ConformsTo(TBigDecimal) {
		f, err := src.AsConcreteFloat()
		if err != nil {
			return nil, fmt.Errorf("convert: Float source must be a concrete value, not a dependent-type constraint")
		}
		result, err := floatToBigDecimal(f, accuracy, places, placesSet)
		if err != nil {
			return nil, err
		}
		return []Value{result}, nil
	}

	result, err := convertTo(src, ValueType(targetType), base)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}
