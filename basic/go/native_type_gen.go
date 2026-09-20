package basic

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Generic-type words (design/legacy/GENERICS.10.ignore):
//
//	def Box gen [T] refine Record [value:T]
//	def Pair gen [(K extends Comparable) (V default Any)] refine Record [key:K value:V]
//	def Mapper gen [T U] fnsig [[T] [U]]
//	def b:(Box of [Integer]) {value:42}
//
// `gen` walks its parameter list, mints one PLACEHOLDER type node per
// parameter, pushes the placeholder type bindings (so the downstream
// constructor body resolves `T` while it builds — fn params and record
// fields both resolve type names at handler time), and produces a
// GenSpec. The GenSpec-aware constructor overloads (refine / class /
// fnsig / fn) consume it from the trailing stack position, build their
// body with the placeholders live, pop the bindings, and return a
// TypeSchema that `def` installs (InstallType's TypeSchema branch).
// `of` instantiates via core.InstantiateSchema (constraint check +
// per-(schema,args) memoised node mint).
var GenNatives = []NativeFunc{
	{
		Name: "gen",

		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(GenHandler, RunInCheck()),
			Returns:    []*Type{},
			BarrierPos: -1,
		}},
	},
	{
		// `T extends C` — inside a gen list entry. Infix form: the
		// bound C is collected forward, the placeholder literal T
		// (bound by gen before the entry evaluates) comes from the
		// stack.
		Name: "extends",

		Signatures: []Signature{{
			// Slot 0 (the bound) is an ORDINARY TAny slot: TAny admits
			// both type literals (rejectsTypeLiteral carves out TAny)
			// and payload-carrying bounds (DepScalar, surface, class,
			// disjunct). A TypeArgs slot here would win the match for
			// a word token via the word-as-Atom guess and then flunk a
			// DepScalar at collection. Slot 1 (the placeholder) is the
			// TypeArgs slot — it is always a bare placeholder literal.
			Args:       []*Type{TAny, TType},
			TypeArgs:   map[int]bool{1: true},
			Impl:       Go(ExtendsHandler, RunInCheck()),
			Returns:    []*Type{TGenParam},
			BarrierPos: 1,
		}},
	},
	{
		// `T default D` / `T extends C default D` — the second sig
		// chains off the GenParam `extends` produced.
		Name: "default",

		Signatures: []Signature{
			{
				// Chains off the GenParam `extends` produced. Slot 0
				// (the default value) is ordinary TAny — see extends.
				Args:       []*Type{TAny, TGenParam},
				Impl:       Go(defaultChainHandler, RunInCheck()),
				Returns:    []*Type{TGenParam},
				BarrierPos: 1,
			},
			{
				Args:       []*Type{TAny, TType},
				TypeArgs:   map[int]bool{1: true},
				Impl:       Go(DefaultBareHandler, RunInCheck()),
				Returns:    []*Type{TGenParam},
				BarrierPos: 1,
			},
		},
	},
	{
		// `Box of [Integer]` — infix form: the type-argument list is
		// collected forward, the schema comes from the stack.
		Name: "of",

		Signatures: []Signature{{
			Args:     []*Type{TList, TAny},
			TypeArgs: map[int]bool{1: true},
			Impl:     Go(OfHandler, RunInCheck()),
			Returns:  []*Type{TType},
			// BarrierPos 1: the arg LIST forward-collects, the schema
			// head always comes from the stack (the tor/tand/is swap
			// pattern). All-forward (-1) let a trailing-context `of`
			// defer and steal a LATER stack value at end-of-run
			// resolution.
			BarrierPos: 1,
		}},
	},
}

// GenHandler walks the parameter list. Each entry is either a bare
// capitalized word (unconstrained parameter) or a paren expression
// (`(T extends C)`, `(T default D)`, `(T extends C default D)`)
// evaluated with THIS parameter's placeholder pre-bound, so F-bounded
// constraints (`(T extends Container of [T])`) and later-parameter
// references to earlier ones work without special casing.
func GenHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("gen_error", "gen: needs a concrete list of type parameters", "gen")
	}
	lst, _ := AsList(args[0])
	if lst.Len() == 0 {
		return nil, r.BoruError("gen_error", "gen: parameter list must declare at least one parameter", "gen")
	}

	spec := &GenSpecInfo{}
	seen := map[string]bool{}
	fail := func(err error) ([]Value, error) {
		PopGenBindings(r, spec)
		return nil, err
	}

	for _, entry := range lst.Slice() {
		switch {
		case IsWord(entry):
			w, _ := AsWord(entry)
			if err := validateGenParamName(r, w.Name, seen); err != nil {
				return fail(err)
			}
			p := GenParam{Name: w.Name, Pos: entry.Pos()}
			p.Node = MintTypeParam(r, p)
			r.Defs.PushType(p.Name, p.Node, NewTypeLiteral(p.Node))
			spec.Bound = append(spec.Bound, p.Name)
			spec.Params = append(spec.Params, p)

		case IsParenExpr(entry):
			toks, _ := AsParenExpr(entry)
			if len(toks) == 0 || !IsWord(toks[0]) {
				return fail(r.BoruError("gen_error",
					"gen: a parenthesised parameter entry must start with the parameter name, e.g. (T extends Comparable)", "gen"))
			}
			w, _ := AsWord(toks[0])
			if err := validateGenParamName(r, w.Name, seen); err != nil {
				return fail(err)
			}
			// Pre-bind a provisional (unconstrained) placeholder so the
			// entry's own expression can reference it (F-bounds).
			prov := GenParam{Name: w.Name, Pos: entry.Pos()}
			prov.Node = MintTypeParam(r, prov)
			r.Defs.PushType(prov.Name, prov.Node, NewTypeLiteral(prov.Node))
			spec.Bound = append(spec.Bound, prov.Name)

			sub := New(r)
			body := make([]Value, len(toks))
			copy(body, toks)
			out, err := sub.Run(body)
			if err != nil {
				return fail(fmt.Errorf("gen: parameter %s: %w", w.Name, err))
			}
			if len(out) != 1 {
				return fail(r.BoruError("gen_error",
					fmt.Sprintf("gen: parameter entry (%s …) must produce one parameter spec, got %d values", w.Name, len(out)), "gen"))
			}
			var p GenParam
			switch {
			case IsGenParamValue(out[0]):
				p, _ = AsGenParamValue(out[0])
			case IsBareTypeNode(out[0]) && TypeParamName(&out[0]) == w.Name:
				// `(T)` — parens around a bare parameter.
				p = GenParam{Name: w.Name}
			default:
				return fail(r.BoruError("gen_error",
					fmt.Sprintf("gen: parameter entry (%s …) must use `extends` / `default`, got %s", w.Name, out[0].String()), "gen"))
			}
			p.Pos = entry.Pos()
			p.Node = prov.Node
			AttachGenBound(r, p.Node, p)
			spec.Params = append(spec.Params, p)

		default:
			return fail(r.BoruError("gen_error",
				fmt.Sprintf("gen: parameter entries are bare names or parenthesised specs, got %s", entry.String()), "gen"))
		}
	}
	// The spec travels OUT-OF-BAND to the next constructor (D2,
	// revised): returning it as a value would be captured by `def`'s
	// forward collection before refine/class/fnsig could see it.
	if err := r.SetPendingGen(spec); err != nil {
		return fail(err)
	}
	return nil, nil
}

// validateGenParamName enforces the type-name convention (capitalized)
// and uniqueness, and declines to shadow a same-named live binding in a
// confusing way only when it is itself a placeholder (nested gen with
// a reused name is almost certainly a bug).
func validateGenParamName(r *Registry, name string, seen map[string]bool) error {
	ch, _ := utf8.DecodeRuneInString(name)
	if !unicode.IsUpper(ch) {
		return r.BoruError("gen_error",
			fmt.Sprintf("gen: type parameter %q must start with a capital letter", name), "gen")
	}
	if seen[name] {
		return r.BoruError("gen_error",
			fmt.Sprintf("gen: duplicate type parameter %q", name), "gen")
	}
	seen[name] = true
	return nil
}

// ExtendsHandler builds a bounded GenParam. The left side must be a
// placeholder literal — i.e. `extends` is only meaningful inside a
// gen parameter entry, where gen has pre-bound the name.
func ExtendsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	bound := ResolveWordValue(args[0])
	ph := args[1]
	name := TypeParamName(&ph)
	if name == "" {
		return nil, r.BoruErrorHint("extends_outside_gen",
			fmt.Sprintf("extends: left side must be a gen type parameter, got %s", ph.String()), "extends",
			"hint: `extends` declares a parameter bound inside gen [...] — e.g. gen [(T extends Comparable)]")
	}
	if !IsTypeLiteral(bound) && !IsTypeBody(bound) {
		return nil, r.BoruError("extends_error",
			fmt.Sprintf("extends: bound must be a type, got %s", bound.String()), "extends")
	}
	return []Value{NewGenParamValue(GenParam{Name: name, Bound: bound, HasBound: true})}, nil
}

// defaultChainHandler attaches a default to the GenParam `extends`
// produced (`T extends C default D`).
func defaultChainHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	d := ResolveWordValue(args[0])
	p, _ := AsGenParamValue(args[1])
	p.Default = d
	p.HasDefault = true
	return []Value{NewGenParamValue(p)}, nil
}

// DefaultBareHandler builds an unconstrained-but-defaulted GenParam
// (`T default D`).
func DefaultBareHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	d := ResolveWordValue(args[0])
	ph := args[1]
	name := TypeParamName(&ph)
	if name == "" {
		return nil, r.BoruErrorHint("default_outside_gen",
			fmt.Sprintf("default: left side must be a gen type parameter, got %s", ph.String()), "default",
			"hint: `default` declares a parameter default inside gen [...] — e.g. gen [(E default Error)]")
	}
	return []Value{NewGenParamValue(GenParam{Name: name, Default: d, HasDefault: true})}, nil
}

// OfHandler instantiates a schema: `Box of [Integer]`. The head (from
// the stack) is the schema; the forward list holds the type arguments.
// A Self head (inside a schema body, D5) defers via GenInstRef.
func OfHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("type_error", "of: needs a concrete list of type arguments", "of")
	}
	head := ResolveWordValue(args[1])
	lst, _ := AsList(args[0])
	argv := make([]Value, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		argv[i] = resolveTypeArg(r, lst.Get(i))
	}

	// Self head: always deferred — resolved when the enclosing schema
	// instantiates (D5).
	if IsBareTypeNode(head) && head.ID == TSelf.ID {
		return []Value{NewGenInstRef(GenInstRef{Head: head, Args: argv})}, nil
	}

	// `Type of [B]` — Type as a builtin schema: the type of TYPE
	// LITERALS conforming to B. This is the referent of the /t word
	// suffix and the angle form (`Map/t` ≡ `Type<Map>` ≡
	// `(Type of [Map])`). The bound must denote a single lattice node
	// (a builtin or a NAMED user type, including named disjuncts);
	// structural bodies have no single node to conform to.
	if IsBareTypeNode(head) && head.ID == TType.ID {
		if len(argv) != 1 {
			return nil, r.BoruError("type_error",
				fmt.Sprintf("of: Type takes exactly one bound, got %d", len(argv)), "of")
		}
		// A NAMED type as the bound (incl. a named disjunct) must bind
		// to its MINTED lattice node — resolveTypeArg resolves a name
		// to its BODY (the right thing for schema instantiation), but
		// ConformsTo needs the node identity. The /t desugar delivers
		// the name as an ATOM precisely so it reaches here unresolved.
		var boundName string
		switch raw := lst.Get(0); {
		case IsAtom(raw):
			boundName, _ = AsAtom(raw)
		case IsWord(raw):
			w, _ := AsWord(raw)
			boundName = w.Name
		}
		if boundName != "" {
			// Minted user types first: a BODY type (named disjunct,
			// predicate) carries its body so satisfaction can unify
			// against it; a nominal type (refine) carries its literal.
			if def := r.LookupTypeName(boundName); def != nil {
				if body, ok := r.TopTypeBody(boundName); ok && !IsBareTypeNode(body) {
					return []Value{NewBoundedTypeBody(def, body)}, nil
				}
				return []Value{NewBoundedType(def)}, nil
			}
			// Kernel / builtin names.
			if bt, terr := resolveTypeName(boundName); terr == nil && bt != nil {
				return []Value{NewBoundedType(bt)}, nil
			}
		}
		bound := argv[0]
		if !IsBareTypeNode(bound) {
			return nil, r.BoruErrorHint("type_error",
				fmt.Sprintf("of: Type bound must be a named type, got %s", bound.String()), "of",
				"hint: name a structural bound first — def MapOrList (Map tor List), then MapOrList/t")
		}
		return []Value{NewBoundedType(CanonicalType(r, &bound))}, nil
	}

	var info *TypeSchemaInfo
	switch {
	case IsTypeSchema(head):
		info, _ = AsTypeSchema(head)
	case IsBareTypeNode(head):
		node := CanonicalType(r, &head)
		if si, ok := SchemaInfoOf(node); ok {
			info = si
		}
	}
	if info == nil {
		return nil, r.BoruErrorHint("type_error",
			fmt.Sprintf("of: %s is not a generic schema", head.String()), "of",
			"hint: declare one with `def Name gen [T] refine Record [...]` (or `... fnsig [...]`)")
	}
	inst, err := InstantiateSchema(r, info, argv)
	if err != nil {
		return nil, err
	}
	// A value-uninhabited instantiation is legal but worth flagging in
	// check mode (§7.6).
	if r.Check.IsActive() {
		for _, a := range argv {
			if IsBareTypeNode(a) && a.ID == TNever.ID {
				r.Check.AddDiagnostic(CheckDiagnostic{
					Code:   "static_warning",
					Detail: "of: instantiation with Never is value-uninhabited",
					Word:   "of",
				})
				break
			}
		}
	}
	return []Value{inst}, nil
}

// resolveTypeArg resolves one element of an `of` argument list: words
// resolve to their type binding (builtin or def'd — the list arrives
// auto-evaluated, but builtin type names inside word-context lists can
// survive as words), everything else passes through.
func resolveTypeArg(r *Registry, v Value) Value {
	if IsWord(v) {
		w, _ := AsWord(v)
		if tv, ok := r.ResolveTypedName(w.Name); ok {
			return tv
		}
	}
	if IsAtom(v) {
		name, _ := AsAtom(v)
		if tv, ok := r.ResolveTypedName(name); ok {
			return tv
		}
	}
	return ResolveWordValue(v)
}

// ---- GenSpec consumption by the type constructors (D2, revised) ----
//
// The constructors call TakePendingGen at handler entry. When a spec
// is pending they build their body with the placeholders still bound
// (gen pushed them; record fields and fn params resolve type names at
// handler time), pop the bindings, and wrap the result as a
// TypeSchema for InstallType.

// GenWrapSchema finishes a generic constructor: validates the body
// kind, pops the placeholder bindings, and wraps the TypeSchema.
func GenWrapSchema(r *Registry, spec *GenSpecInfo, body Value, kind SchemaKind) ([]Value, error) {
	PopGenBindings(r, spec)
	info := &TypeSchemaInfo{Params: spec.Params, Body: body, Kind: kind}
	return []Value{NewTypeSchema(TType, info)}, nil
}

// GenUnsupported reports a constructor that cannot host a gen spec in
// v1, popping the bindings so nothing leaks.
func GenUnsupported(r *Registry, spec *GenSpecInfo, word string, got string) error {
	PopGenBindings(r, spec)
	return r.BoruErrorHint("gen_unsupported_constructor",
		fmt.Sprintf("gen: %s cannot build a generic schema from %s in v1", word, got), word,
		"hint: generic schemas are `gen [...] refine Record [...]`, `gen [...] class {...}`, or `gen [...] fnsig [...]`")
}
