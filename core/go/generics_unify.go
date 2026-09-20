package core

import "fmt"

// Binding inference for generic fns (design/legacy/GENERICS.10.ignore §9.2.2,
// Phase 4): at each call of a generic fn, the type parameters bind
// from the actual arguments' types — a placeholder param slot binds
// typeof(arg); a typed-list pattern over a placeholder binds the
// union of the element types. Conflicting evidence for one parameter
// takes the union (`tor`-merge), per the design — runtime calls are
// never rejected for parameter inconsistency (the checker reports
// precision losses in Phase 5).

// ResolveSigChildParam resolves a typed-list/typed-map param pattern
// whose child constraint is a raw Word naming a LIVE gen placeholder
// (`xs:[:T]` while `gen [T]`'s bindings are pushed). Definition time
// is the only moment the name can resolve — the placeholder bindings
// pop when the generic fn's def completes — so ResolveSigType calls
// this while parsing the signature. Non-placeholder children (builtin
// literals, concrete patterns, user-type Words) pass through
// untouched.
func ResolveSigChildParam(r *Registry, v Value) Value {
	if r == nil {
		return v
	}
	ci, err := AsChildType(v)
	if err != nil {
		return v
	}
	var child Value
	if IsTypedList(ci.Child) || IsTypedMap(ci.Child) {
		// A nested container child (`m:{:{:Integer}}`) resolves its
		// own child recursively.
		child = ResolveSigChildParam(r, ci.Child)
		if ExactEqual(child, ci.Child) {
			return v
		}
	} else if w, werr := AsWord(ci.Child); werr == nil {
		if def := r.LookupTypeName(w.Name); def != nil && IsTypeParamNode(def) {
			child = NewTypeLiteral(def)
		} else if tv, ok := r.TopTypeBody(w.Name); ok && IsTypeBody(tv) {
			// A user-type child (`xs:[:Foo]`) resolves at sig install,
			// exactly when the named param types resolve (ResolveSigType).
			child = tv
		} else if t, ok := ResolveBuiltinTypeName(w.Name); ok {
			// The builtin arm of the canonical cascade (ADR-012 rule 4):
			// post-opacity, `xs:[:Integer]` arrives here as a Word.
			child = NewTypeLiteral(t)
		} else {
			return v
		}
	} else {
		return v
	}
	if IsTypedMap(v) {
		if len(ci.Entries) > 0 {
			return NewTypedMapWithEntries(child, ci.Entries)
		}
		return NewTypedMap(child)
	}
	if len(ci.Elements) > 0 {
		return NewTypedListWithElements(child, ci.Elements)
	}
	return NewTypedList(child)
}

// ResolveSigRecordFields is ResolveSigChildParam's STRUCTURAL-RECORD twin: it
// resolves the field TYPE WORDS of an inline record pattern — the
// `{pretty:Boolean}` of `fn [[o:{pretty:Boolean}] …]` — to type values at sig
// install, exactly when the named param types resolve (ResolveSigType).
//
// A NAMED record type gets this for free: `type R record {pretty:Boolean}`
// DISPATCHES `record`, so its field map is evaluated and ResolveDefType's
// pattern already carries type values. The INLINE spelling never evaluates —
// the map rides inside the fn-spec LIST, which is inert data — so its field
// values arrive as bare Word tokens and nothing downstream re-resolved them.
// The dispatcher tolerated that (Unify resolves the name at match time), but
// the schema-bearing param carrier copies the pattern's fields VERBATIM
// (check's recordSchemaCarrier), so a body field read narrowed to the WORD's
// own type: `o.pretty` checked as dynamic(Word), not dynamic(Boolean). That
// carrier then arrived at the step loop as a Word-typed value with no
// WordInfo and raised a NAMELESS `undefined_word` — on the compile path only,
// because the plain check pass builds the coarse `{:Any}` carrier and never
// reads the schema. NUR103.
//
// Resolution follows ResolveSigChildParam's cascade — type-param node, user
// type body, builtin name — and leaves everything else untouched, so a field
// pinned to a literal VALUE (`{status:'ok'}` — a value is a type, ADR-010)
// keeps its value pattern and a field naming no type keeps its word. A nested
// inline record (`{a:{b:Integer}}`) resolves recursively, so a chained read
// narrows through it the same way a nested NAMED record already does, and a
// field type EXPRESSION (`{a:(Integer tor String)}`) evaluates, exactly as
// ResolveChildTypeExpr evaluates a typed container's paren child.
//
// It deliberately does NOT delegate to ResolveFieldType, the cascade `record`
// and `class` run over their own field maps. That resolver also EVALUATES a
// concrete List field as code, which is right for a type DECLARATION and
// wrong for a dispatch PATTERN, where `{a:[1 2]}` pins the field to that
// list. The two share the type-name cascade, not the value arms.
func ResolveSigRecordFields(r *Registry, v Value) Value {
	if r == nil {
		return v
	}
	mp, ok := v.Data.(MapPayload)
	if !ok || mp.M == nil {
		return v
	}
	// ONE pass, results held: a field type EXPRESSION is evaluated, and a
	// scan-then-rebuild would run it twice — visibly, for an expression with
	// an effect.
	keys := mp.M.Keys()
	resolved := make([]Value, len(keys))
	changed := false
	for i, k := range keys {
		fv, _ := mp.M.Get(k)
		rv, hit := resolveSigFieldType(r, fv)
		resolved[i] = rv
		changed = changed || hit
	}
	// Identity matters here: the pattern is stored on the signature and
	// compared by the dispatcher, so an all-concrete field map is handed
	// back as the SAME value rather than an equal copy.
	if !changed {
		return v
	}
	out := NewOrderedMap()
	out.Implicit = mp.M.Implicit
	out.Meta = mp.M.Meta
	for i, k := range keys {
		out.Set(k, resolved[i])
	}
	res := NewMap(out)
	res.pos = v.pos
	return res
}

// resolveSigFieldType resolves ONE record-pattern field's type expression and
// reports whether it changed. A bare Word naming a type becomes that type
// (ResolveSigChildParam's cascade, arm for arm); a nested inline record map
// recurses. Anything else — a literal value pattern, a typed container, a
// word naming no type — is returned unchanged.
func resolveSigFieldType(r *Registry, fv Value) (Value, bool) {
	w, werr := AsWord(fv)
	if werr != nil {
		if _, isMap := fv.Data.(MapPayload); isMap {
			nested := ResolveSigRecordFields(r, fv)
			return nested, !ExactEqual(nested, fv)
		}
		if IsParenExpr(fv) {
			return resolveSigFieldExpr(r, fv)
		}
		return fv, false
	}
	if def := r.LookupTypeName(w.Name); def != nil && IsTypeParamNode(def) {
		return NewTypeLiteral(def), true
	}
	if tv, ok := r.TopTypeBody(w.Name); ok && IsTypeBody(tv) {
		return tv, true
	}
	if t, ok := ResolveBuiltinTypeName(w.Name); ok {
		return NewTypeLiteral(t), true
	}
	return fv, false
}

// resolveSigFieldExpr evaluates a record-pattern field whose type is an
// EXPRESSION — `{a:(Integer tor String)}`, `{b:(Box of [Integer])}`. In the
// inline spelling the paren span is inert data inside the fn-spec list, so
// the field kept a ParenExpr the dispatcher could never match: `fn
// [[o:{a:(Integer tor String)}] …]` declined `{a:7}` outright, where the same
// field written through `refine Record [{a:(Integer tor String)}]` — which
// dispatches, and so evaluates — admits it. Same asymmetry as the bare type
// word, one level up.
//
// Failure is SILENT and leaves the field alone: a sig install is not a place
// to raise, and a paren that does not evaluate to a single value was never a
// type constraint to begin with (it keeps whatever meaning the dispatcher
// already gave it). ResolveChildTypeExpr, the typed-container twin, reports
// its error because its caller is positioned to attach it to the param.
func resolveSigFieldExpr(r *Registry, fv Value) (Value, bool) {
	toks, terr := AsParenExpr(fv)
	if terr != nil { //covergate:allow IsParenExpr at the sole call site requires the ParenExpr payload, so AsParenExpr cannot fail here
		return fv, false
	}
	sub := New(r)
	input := make([]Value, 0, len(toks)+2)
	input = append(input, NewOpenParen())
	input = append(input, toks...)
	input = append(input, NewCloseParen())
	out, rerr := sub.Run(input)
	if rerr != nil || len(out) != 1 || !IsTypeBody(out[0]) {
		return fv, false
	}
	return out[0], true
}

// ResolveChildTypeExpr evaluates a typed-list/map child constraint
// that arrived as an unevaluated ParenExpr — `[:(Pair of [String
// Integer])]` in data context parses with the paren span as the
// child payload, and nothing downstream evaluates it. Returns the
// rebuilt typed list/map (or v unchanged when the child is not a
// ParenExpr). The expression must produce exactly one type value.
func ResolveChildTypeExpr(r *Registry, v Value) (Value, error) {
	if r == nil || (!IsTypedList(v) && !IsTypedMap(v)) {
		return v, nil
	}
	ci, err := AsChildType(v)
	if err != nil { //covergate:allow IsTypedList/IsTypedMap above both require Data.(ChildTypeInfo), so AsChildType cannot fail here
		return v, nil
	}
	// An angle/type-bound child (`[:Box<Integer>]`) arrives as a sugar
	// marker; lower it to its paren form so the evaluation below sees
	// the spelt-out `[:(Box of [Integer])]`.
	childExpr := ci.Child
	if sinfo, sok := AsSugar(childExpr); sok {
		if exp, serr := SugarExpansion(r, sinfo, childExpr, false); serr == nil && len(exp) == 1 {
			childExpr = exp[0]
		}
	}
	if !IsParenExpr(childExpr) {
		return v, nil
	}
	toks, terr := AsParenExpr(childExpr)
	if terr != nil {
		return v, nil
	}
	sub := New(r)
	input := make([]Value, 0, len(toks)+2)
	input = append(input, NewOpenParen())
	input = append(input, toks...)
	input = append(input, NewCloseParen())
	out, rerr := sub.Run(input)
	if rerr != nil {
		return v, fmt.Errorf("child type expression: %w", rerr)
	}
	if len(out) != 1 {
		return v, fmt.Errorf("child type expression must produce one type, got %d values", len(out))
	}
	child := out[0]
	if IsTypedMap(v) {
		if len(ci.Entries) > 0 {
			return NewTypedMapWithEntries(child, ci.Entries), nil
		}
		return NewTypedMap(child), nil
	}
	if len(ci.Elements) > 0 {
		return NewTypedListWithElements(child, ci.Elements), nil
	}
	return NewTypedList(child), nil
}

// typeParamLitNode returns the placeholder node when v is a bare type
// literal of a minted gen-param node, nil otherwise. The Behavior
// pointer survives the literal's by-value copy, so detection works on
// the copy without a registry lookup.
func typeParamLitNode(v Value) *Type {
	if !IsBareTypeNode(v) {
		return nil
	}
	if IsTypeParamNode(&v) {
		return &v
	}
	return nil
}

// unifyTypeParam admits the other side of a unification against a
// placeholder literal by the parameter's BOUND (genParamUnifier.Match:
// unconstrained → anything; `extends C` → Is-membership in C). The
// fold exists because placeholder literals reach Unify embedded in
// structural patterns — a generic fn's `xs:[:T]` param unifies each
// list element against the placeholder — and the standard narrowing
// rule (ConformsTo) has no admission path from a concrete value into
// a Type/TypeParam node. r is the enclosing chain's registry, threaded
// into the bound walk (isR).
func unifyTypeParam(lit Value, node *Type, other Value, r *Registry) (Value, *UnifyError) {
	// The same placeholder on both sides (memo keys, `[T] vs [T]`).
	if IsBareTypeNode(other) && other.ID == node.ID {
		return lit, nil
	}
	if isR(other, node, r) {
		return other, nil
	}
	return Value{}, unifyFail("value does not satisfy the type parameter's bound", lit, other)
}

// genBinder accumulates type-parameter bindings during call-site and
// schema inference. Both InferGenBindings and InferSchemaBindings share
// it: an isParam gate (only declared parameters bind) and a merge that
// tor-unions repeated evidence (design/legacy/GENERICS.10.ignore §9.2.2 — runtime
// calls are never rejected for parameter inconsistency).
type genBinder struct {
	bindings map[string]Value
	isParam  map[string]bool
}

func newGenBinder(params []GenParam) *genBinder {
	b := &genBinder{bindings: map[string]Value{}, isParam: make(map[string]bool, len(params))}
	for _, p := range params {
		b.isParam[p.Name] = true
	}
	return b
}

func (b *genBinder) merge(name string, t Value) {
	if !b.isParam[name] {
		return
	}
	if prev, ok := b.bindings[name]; ok {
		b.bindings[name] = UnionType(prev, t)
		return
	}
	b.bindings[name] = t
}

// paramNameOf returns the type-parameter name a constraint value
// references: the placeholder node's name when v survived parsing as the
// minted placeholder literal (its genParamUnifier Behavior pointer
// survives the by-value copy), or the bare Word name when v is still a
// raw Word naming the parameter. Empty when v references no parameter.
// The single definition of the "placeholder-literal-or-raw-Word" idiom
// that inferFromChildPattern and InferSchemaBindings both need.
func paramNameOf(v Value) string {
	if name := TypeParamName(&v); name != "" {
		return name
	}
	if IsWord(v) {
		if w, err := AsWord(v); err == nil {
			return w.Name
		}
	}
	return ""
}

// InferGenBindings walks a signature's params alongside the call args
// and collects parameter-name → type-value bindings.
func InferGenBindings(spec *GenSpecInfo, params []FnParam, args []Value) map[string]Value {
	if spec == nil {
		return map[string]Value{}
	}
	b := newGenBinder(spec.Params)
	for i, p := range params {
		if i >= len(args) {
			break
		}
		arg := args[i]
		// Placeholder param slot: T binds the arg's own type.
		if name := TypeParamName(p.Type); name != "" {
			if arg.Parent != nil {
				b.merge(name, NewTypeLiteral(arg.Parent))
			}
			continue
		}
		// Typed-list/map pattern over a placeholder (`xs:[:T]`,
		// `m:{:T}`): T binds the union of the element types.
		if p.Pattern != nil && (IsTypedList(*p.Pattern) || IsTypedMap(*p.Pattern)) {
			inferFromChildPattern(b, *p.Pattern, arg)
		}
	}
	return b.bindings
}

// inferFromChildPattern handles one `[:T]` / `{:T}` param pattern
// against its call argument. The pattern child survives signature
// parsing as either the placeholder literal (ResolveSigChildParam) or
// a raw Word naming the parameter. The argument contributes element
// evidence three ways: a concrete list/map merges each element's
// type; a typed-list/map value or CARRIER (check mode strips list
// literals to typed-list carriers) merges the child constraint
// directly.
func inferFromChildPattern(b *genBinder, pattern, arg Value) {
	ci, err := AsChildType(pattern)
	if err != nil {
		return
	}
	name := paramNameOf(ci.Child)
	if name == "" || !b.isParam[name] {
		return
	}
	switch {
	case IsTypedList(arg) || IsTypedMap(arg):
		if aci, aerr := AsChildType(arg); aerr == nil {
			switch {
			case IsBareTypeNode(aci.Child):
				b.merge(name, aci.Child)
			case aci.Child.Parent != nil:
				b.merge(name, NewTypeLiteral(aci.Child.Parent))
			}
		}
	case arg.Parent != nil && arg.Parent.Equal(TList) && IsConcrete(arg):
		if lst, lerr := AsList(arg); lerr == nil {
			for j := 0; j < lst.Len(); j++ {
				el := lst.Get(j)
				if el.Parent != nil {
					b.merge(name, NewTypeLiteral(el.Parent))
				}
			}
		}
	case arg.Parent != nil && arg.Parent.Equal(TMap) && IsConcrete(arg):
		if m, merr := AsMap(arg); merr == nil && m != nil {
			for _, k := range m.Keys() {
				if v, ok := m.Get(k); ok && v.Parent != nil {
					b.merge(name, NewTypeLiteral(v.Parent))
				}
			}
		}
	}
}

// InferSchemaBindings infers a generic schema's type-parameter
// bindings from a concrete construction body (Phase 7, §9.2.2 + D12):
// each schema field whose declared constraint is a placeholder merges
// typeof(field value); a `[:T]`/`{:T}` child constraint merges the
// union of the element types. Conflicting evidence tor-merges — same
// contract as call-site inference for generic fns.
func InferSchemaBindings(info *TypeSchemaInfo, body Value) map[string]Value {
	if info == nil {
		return map[string]Value{}
	}
	b := newGenBinder(info.Params)
	fields := schemaFields(info)
	if fields == nil || body.Parent == nil || !body.Parent.Equal(TMap) || !IsConcrete(body) {
		return b.bindings
	}
	bm, err := AsMap(body)
	if err != nil || bm == nil {
		return b.bindings
	}
	for _, fname := range fields.Keys() {
		c, _ := fields.Get(fname)
		v, ok := bm.Get(fname)
		if !ok {
			continue
		}
		// Direct placeholder constraint: the field survives schema
		// construction as the placeholder literal (fields resolve while
		// the gen bindings are live) or, defensively, a raw Word.
		if pname := paramNameOf(c); pname != "" && b.isParam[pname] {
			if v.Parent != nil {
				b.merge(pname, NewTypeLiteral(v.Parent))
			}
			continue
		}
		if IsTypedList(c) || IsTypedMap(c) {
			inferFromChildPattern(b, c, v)
		}
	}
	return b.bindings
}

// schemaFields returns the field-constraint map of a record or class
// schema body, nil for fn-shape schemas (no fields to infer from).
func schemaFields(info *TypeSchemaInfo) *OrderedMap {
	if IsRecordType(info.Body) {
		rt, _ := AsRecordType(info.Body)
		return rt.Fields
	}
	if oi, err := AsClassType(info.Body); err == nil {
		return oi.Fields
	}
	return nil
}

// InferAndInstantiateSchema resolves a generic schema used directly as
// a construction constraint — `make Box {value:42}` or `def b:Box
// {value:42}` — by inferring the parameter bindings from the body and
// instantiating (Phase 7, resolving D12: a fully-defaulted schema
// auto-instantiates even without field evidence). An uninferable,
// undefaulted parameter is a loud unbound_param — never a silent Any.
func InferAndInstantiateSchema(r *Registry, schema Value, body Value) (Value, error) {
	info, err := AsTypeSchema(schema)
	if err != nil {
		return Value{}, err
	}
	bindings := InferSchemaBindings(info, body)
	args := make([]Value, 0, len(info.Params))
	for i := range info.Params {
		p := &info.Params[i]
		if b, ok := bindings[p.Name]; ok {
			args = append(args, b)
			continue
		}
		if p.HasDefault {
			// Defaults fill positionally at the tail inside
			// InstantiateSchema — stop here, but a BOUND parameter
			// after this one would be mis-positioned, so reject that
			// mix loudly rather than guessing.
			for j := i + 1; j < len(info.Params); j++ {
				if _, later := bindings[info.Params[j].Name]; later {
					return Value{}, r.BoruError("unbound_param",
						fmt.Sprintf("%s: parameter %s could not be inferred but a later parameter could — instantiate explicitly with `%s of [...]`",
							info.Name, p.Name, info.Name), "make")
				}
			}
			break
		}
		return Value{}, r.BoruErrorHint("unbound_param",
			fmt.Sprintf("%s: type parameter %s cannot be inferred from the construction body", info.Name, p.Name),
			"make",
			"instantiate explicitly: "+info.Name+" of [...]")
	}
	return InstantiateSchema(r, info, args)
}

// GenBindingCarrier converts one inferred binding value into a
// check-mode carrier: a binding that denotes a lattice node becomes a
// carrier of that node; a tor-merged disjunct becomes a disjunct
// carrier (the JoinCarriers representation); anything else falls back
// to its Parent.
func GenBindingCarrier(r *Registry, b Value) Value {
	if node := typeArgNode(b); node != nil {
		return NewCarrier(CanonicalType(r, node))
	}
	if IsDisjunct(b) {
		c := b
		c.Carrier = true
		return c
	}
	if b.Parent != nil {
		return NewCarrier(b.Parent)
	}
	return NewCarrier(TAny)
}

// InstallGenCallBindings infers and installs the body-scoped type
// bindings for one generic-fn call. At RUNTIME it MUST be called
// AFTER the body's def snapshot is taken: the bindings are then torn
// down by the existing DefCleanup truncation — NOT by the undef tail,
// whose capitalised-name path would Retire the bound type's canonical
// node (undef T with T→Integer must never retire Integer). Returns
// the installed names so check-mode callers (which have no DefCleanup
// frame) can pop them explicitly via Defs.Pop — also non-retiring.
func InstallGenCallBindings(r *Registry, spec *GenSpecInfo, params []FnParam, args []Value) []string {
	return InstallGenBindingMap(r, spec, InferGenBindings(spec, params, args))
}

// InstallGenBindingMap installs an already-inferred binding map as
// type bindings, in declared-parameter order. See
// InstallGenCallBindings for the teardown contract.
func InstallGenBindingMap(r *Registry, spec *GenSpecInfo, bindings map[string]Value) []string {
	var installed []string
	for _, p := range spec.Params {
		b, ok := bindings[p.Name]
		if !ok {
			continue
		}
		node := typeArgNode(b)
		if node == nil {
			node = b.Parent
		}
		r.Defs.PushType(p.Name, CanonicalType(r, node), b)
		installed = append(installed, p.Name)
	}
	return installed
}
