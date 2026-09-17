package core

import (
	"fmt"
	"strings"
)

// Instantiation of generic schemas (`of`, design/legacy/GENERICS.10.ignore; plan
// decisions D4/D5/D7/D8). The lang `of` word is a thin wrapper over
// InstantiateSchema; substitution recursion (nested `X of […]`,
// `Self of [T]`) re-enters here through GenInstRef resolution.

// PushGenBindings installs the placeholder type bindings for a spec's
// parameters (D1) so a downstream constructor body resolves them.
// Returns the names pushed, recorded on the spec for the consumer (or
// the orphan drain) to pop.
func PushGenBindings(r *Registry, spec *GenSpecInfo) {
	for i := range spec.Params {
		p := &spec.Params[i]
		if p.Node == nil {
			p.Node = MintTypeParam(r, *p)
		}
		r.Defs.PushType(p.Name, p.Node, NewTypeLiteral(p.Node))
		spec.Bound = append(spec.Bound, p.Name)
	}
}

// PopGenBindings removes exactly the placeholder bindings a spec
// pushed (D1 — the GenSpec-consuming constructor calls this after
// building its body; nested specs pop only their own names).
func PopGenBindings(r *Registry, spec *GenSpecInfo) {
	for i := len(spec.Bound) - 1; i >= 0; i-- {
		r.Defs.Pop(spec.Bound[i])
	}
	spec.Bound = nil
}

// genMemoKey builds the registry-local interning key for an
// instantiation (the const-interning pattern: a hidden Defs binding).
func genMemoKey(schemaID string, canon string) string {
	return "__gen:" + schemaID + ":" + canon
}

// canonTypeArg renders one instantiation argument canonically for the
// memo key and the instantiation node's display name. A named node
// renders by name; everything else by CanonValue.
func canonTypeArg(arg Value) string {
	// A NAMED argument evaluates to its minted node (the Stage 2 flip);
	// canonicalise by its recorded declaration content so the named
	// refinement and its inline spelling instantiate identically —
	// `(H of [Pos]) teq (H of [(Integer gt 0)])` (edge-types-3). Class
	// and surface content re-reaches the node-name arm below through
	// its payload back-pointer, exactly as the body value always did.
	if IsBareTypeNode(arg) {
		if body, ok := arg.TypeBody(); ok {
			arg = body
		}
	}
	if node := typeArgNode(arg); node != nil && node.Name() != "" {
		return node.Name()
	}
	return CanonValue(arg)
}

// typeArgNode returns the lattice node a type argument denotes, when
// it denotes one: a bare literal IS its node, an object/class type or
// surface carries its minted node in the payload.
func typeArgNode(arg Value) *Type {
	switch {
	case IsBareTypeNode(arg):
		return &arg
	case IsClassType(arg):
		oi, _ := AsClassType(arg)
		return oi.Type
	case IsSurfaceType(arg):
		si, _ := AsSurfaceType(arg)
		return si.Type
	}
	return nil
}

// containsTypeParam reports whether v (a type value) still references
// a placeholder — the F-bound / Self-recursion shape whose
// instantiation must be deferred (GenInstRef, D5).
func containsTypeParam(v Value) bool {
	if IsBareTypeNode(v) {
		return IsTypeParamNode(&v) || v.ID == TSelf.ID
	}
	if IsGenInstRef(v) {
		ref, _ := AsGenInstRef(v)
		if containsTypeParam(ref.Head) {
			return true
		}
		for _, a := range ref.Args {
			if containsTypeParam(a) {
				return true
			}
		}
		return false
	}
	switch {
	case IsRecordType(v):
		ri, _ := AsRecordType(v)
		for _, k := range ri.Fields.Keys() {
			fv, _ := ri.Fields.Get(k)
			if containsTypeParam(fv) {
				return true
			}
		}
	case IsClassType(v):
		oi, _ := AsClassType(v)
		for _, k := range oi.Fields.Keys() {
			fv, _ := oi.Fields.Get(k)
			if containsTypeParam(fv) {
				return true
			}
		}
	case IsTypedList(v) || IsTypedMap(v):
		ci, _ := AsChildType(v)
		// NOTE: a raw Word child (`[:T]` keeps T as a Word at schema
		// build time) cannot be classified without bindings; defer
		// detection stays literal-only and the word substitutes by
		// name at instantiation.
		return containsTypeParam(ci.Child)
	case IsDisjunct(v):
		di, _ := AsDisjunct(v)
		for _, alt := range di.Alternatives {
			if containsTypeParam(alt) {
				return true
			}
		}
	case IsNegation(v):
		ni, _ := AsNegation(v)
		return containsTypeParam(ni.Inner)
	case v.Parent != nil && v.Parent.Equal(TFnUndef):
		if fi, ok := v.Data.(FnUndefInfo); ok {
			for _, sig := range fi.Sigs {
				for _, p := range sig.Params {
					if p.Type != nil && (IsTypeParamNode(p.Type) || p.Type.ID == TSelf.ID) {
						return true
					}
					if p.Pattern != nil && containsTypeParam(*p.Pattern) {
						return true
					}
				}
				for _, rt := range sig.Returns {
					if rt != nil && (IsTypeParamNode(rt) || rt.ID == TSelf.ID) {
						return true
					}
				}
			}
		}
	}
	return false
}

// SubstituteTypeParams rewrites v with every placeholder reference
// replaced per bindings (parameter name → concrete type argument).
// Generalises SubstituteSelf across the type-shape kinds a schema body
// can carry. selfNode, when non-nil, is the in-flight instantiation
// node `Self` references resolve to (D5 — the knot-tying for
// recursive schemas).
func SubstituteTypeParams(r *Registry, v Value, bindings map[string]Value, selfNode *Type) (Value, error) {
	// A RAW WORD naming a parameter: positions the field-type resolver
	// does not reach (a typed-list/map CHILD like `[:T]` stays a Word
	// at schema-build time) substitute by name here. Words naming
	// nothing in the bindings pass through (they resolve, or fail,
	// exactly as they would in a non-generic body).
	if IsWord(v) {
		if w, err := AsWord(v); err == nil {
			if b, ok := bindings[w.Name]; ok {
				return b, nil
			}
			if w.Name == "Self" && selfNode != nil {
				return NewTypeLiteral(selfNode), nil
			}
		}
		return v, nil
	}

	// Bare placeholder reference.
	if IsBareTypeNode(v) {
		if name := TypeParamName(&v); name != "" {
			b, ok := bindings[name]
			if !ok {
				return Value{}, fmt.Errorf("unbound type parameter %q", name)
			}
			return b, nil
		}
		if v.ID == TSelf.ID && selfNode != nil {
			return NewTypeLiteral(selfNode), nil
		}
		return v, nil
	}

	// Deferred instantiation (Self of [T], nested X of [T]).
	if IsGenInstRef(v) {
		ref, _ := AsGenInstRef(v)
		head, err := SubstituteTypeParams(r, ref.Head, bindings, selfNode)
		if err != nil {
			return Value{}, err
		}
		args := make([]Value, len(ref.Args))
		for i, a := range ref.Args {
			sa, err := SubstituteTypeParams(r, a, bindings, selfNode)
			if err != nil {
				return Value{}, err
			}
			args[i] = sa
		}
		// A Self head resolved to the in-flight node: the reference IS
		// the enclosing instantiation (same args by construction in the
		// direct-recursion case) — but with different args (e.g.
		// Self of [(Box of [T])]) it is a genuinely new instantiation
		// of the same schema.
		if IsBareTypeNode(head) {
			hn := CanonicalType(r, &head)
			if sinfo, ok := SchemaInfoOf(hn); ok {
				return InstantiateSchema(r, sinfo, args)
			}
			if hn != nil && selfNode != nil && hn.ID == selfNode.ID {
				// Self resolved to the in-flight node and the schema
				// payload is reachable through its parent (the schema).
				if sinfo, ok := SchemaInfoOf(hn.Parent); ok {
					return InstantiateSchema(r, sinfo, args)
				}
				return NewTypeLiteral(selfNode), nil
			}
		}
		if sinfo, err := AsTypeSchema(head); err == nil {
			return InstantiateSchema(r, sinfo, args)
		}
		return Value{}, fmt.Errorf("of: head does not resolve to a generic schema (got %s)", head.String())
	}

	switch {
	case IsRecordType(v):
		ri, _ := AsRecordType(v)
		fields := NewOrderedMap()
		for _, k := range ri.Fields.Keys() {
			fv, _ := ri.Fields.Get(k)
			sv, err := SubstituteTypeParams(r, fv, bindings, selfNode)
			if err != nil {
				return Value{}, err
			}
			fields.Set(k, sv)
		}
		return NewRecordType(fields), nil

	case IsClassType(v):
		oi, _ := AsClassType(v)
		fields := NewOrderedMap()
		for _, k := range oi.Fields.Keys() {
			fv, _ := oi.Fields.Get(k)
			sv, err := SubstituteTypeParams(r, fv, bindings, selfNode)
			if err != nil {
				return Value{}, err
			}
			fields.Set(k, sv)
		}
		out := oi
		out.Fields = fields
		return NewValueRaw(v.Parent, out), nil

	case IsTypedList(v):
		ci, _ := AsChildType(v)
		child, err := SubstituteTypeParams(r, ci.Child, bindings, selfNode)
		if err != nil {
			return Value{}, err
		}
		return NewTypedList(child), nil

	case IsTypedMap(v):
		ci, _ := AsChildType(v)
		child, err := SubstituteTypeParams(r, ci.Child, bindings, selfNode)
		if err != nil {
			return Value{}, err
		}
		return NewTypedMap(child), nil

	case IsDisjunct(v):
		di, _ := AsDisjunct(v)
		alts := make([]Value, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			sa, err := SubstituteTypeParams(r, alt, bindings, selfNode)
			if err != nil {
				return Value{}, err
			}
			alts[i] = sa
		}
		return NewDisjunct(alts), nil

	case IsNegation(v):
		ni, _ := AsNegation(v)
		inner, err := SubstituteTypeParams(r, ni.Inner, bindings, selfNode)
		if err != nil {
			return Value{}, err
		}
		return NewNegation(inner), nil

	case v.Parent != nil && v.Parent.Equal(TFnUndef):
		fi, ok := v.Data.(FnUndefInfo)
		if !ok {
			return v, nil
		}
		out := FnUndefInfo{Sigs: make([]FnSigSpec, len(fi.Sigs))}
		for i, sig := range fi.Sigs {
			spec := FnSigSpec{
				Params:  make([]FnParam, len(sig.Params)),
				Returns: make([]*Type, len(sig.Returns)),
			}
			for j, p := range sig.Params {
				np := p
				if p.Type != nil {
					nt, err := substituteParamNode(r, p.Type, bindings, selfNode)
					if err != nil {
						return Value{}, err
					}
					np.Type = nt
				}
				if p.Pattern != nil {
					sp, err := SubstituteTypeParams(r, *p.Pattern, bindings, selfNode)
					if err != nil {
						return Value{}, err
					}
					np.Pattern = &sp
				}
				spec.Params[j] = np
			}
			for j, rt := range sig.Returns {
				nt, err := substituteParamNode(r, rt, bindings, selfNode)
				if err != nil {
					return Value{}, err
				}
				spec.Returns[j] = nt
			}
			out.Sigs[i] = spec
		}
		return NewFnUndef(out), nil
	}

	// Concrete values (class field defaults like {count:0}) and every
	// other shape pass through untouched.
	return v, nil
}

// substituteParamNode swaps a *Type slot (an fn-shape param/return)
// when it is a placeholder or Self. The binding must denote a lattice
// node — fn-shape slots hold *Type, so a structural binding (a record
// literal) cannot occupy one (v1 pin; noted in GENERICS.10.md).
func substituteParamNode(r *Registry, t *Type, bindings map[string]Value, selfNode *Type) (*Type, error) {
	if t == nil {
		return nil, nil
	}
	if t.ID == TSelf.ID && selfNode != nil {
		return selfNode, nil
	}
	name := TypeParamName(t)
	if name == "" {
		return t, nil
	}
	b, ok := bindings[name]
	if !ok {
		return nil, fmt.Errorf("unbound type parameter %q", name)
	}
	if node := typeArgNode(b); node != nil {
		return CanonicalType(r, node), nil
	}
	return nil, fmt.Errorf("type parameter %q: a generic fn-shape position needs a NAMED type argument, got %s", name, b.String())
}

// InstantiateSchema is the core of `of` (D4): validate arity and
// constraints, fill defaults (lazily substituted, D8), memoise per
// (schema, canonical args) via a hidden Defs binding, mint the
// instantiation node as a child of the schema node, and substitute the
// schema body. Returns the instantiated type VALUE (the body — what
// `make`, typed-def, and `is` consume); the minted node is its Parent.
//
// Args that still carry placeholders (inside another schema's body —
// F-bounds, Self recursion) defer: the result is a GenInstRef resolved
// at the enclosing instantiation.
func InstantiateSchema(r *Registry, info *TypeSchemaInfo, args []Value) (Value, error) {
	if info == nil || info.Type == nil {
		return Value{}, fmt.Errorf("of: schema has no minted node (declare it with def)")
	}

	// Deferred path: placeholders in the args.
	for _, a := range args {
		if containsTypeParam(a) {
			return NewGenInstRef(GenInstRef{Head: NewTypeLiteral(info.Type), Args: args}), nil
		}
	}

	// Arity: defaults fill missing trailing params (substituted
	// against the bindings collected so far — D8 lazy defaults).
	if len(args) > len(info.Params) {
		return Value{}, &BoruError{
			Code: "arity_mismatch",
			Detail: fmt.Sprintf("of: %s takes %d type parameter(s), got %d",
				schemaShortName(info), len(info.Params), len(args)),
		}
	}
	bindings := map[string]Value{}
	full := make([]Value, 0, len(info.Params))
	for i, p := range info.Params {
		var arg Value
		switch {
		case i < len(args):
			arg = args[i]
		case p.HasDefault:
			d, err := SubstituteTypeParams(r, p.Default, bindings, nil)
			if err != nil {
				return Value{}, fmt.Errorf("of: default for %s: %w", p.Name, err)
			}
			arg = d
		default:
			return Value{}, &BoruError{
				Code: "arity_mismatch",
				Detail: fmt.Sprintf("of: %s takes %d type parameter(s), got %d (no default for %s)",
					schemaShortName(info), len(info.Params), len(args), p.Name),
				Hint: "supply the missing type argument(s): " + schemaShortName(info) + " of [...]",
			}
		}
		// Constraint check (D7): Is-membership of the ARG TYPE in the
		// bound, via the registry-aware unifier so predicates,
		// disjuncts, negations, and surfaces all answer uniformly
		// (an abstract type arg checks containment — the surface and
		// lattice folds in unifyInner handle the literal-vs-literal
		// case).
		if p.HasBound {
			if _, uerr := UnifyExplainR(p.Bound, arg, r); uerr != nil {
				return Value{}, &BoruError{
					Code: "constraint_violation",
					Detail: fmt.Sprintf("of: type argument %s for parameter %s does not satisfy its bound %s",
						canonTypeArg(arg), p.Name, CanonValue(p.Bound)),
					Row: p.Pos.Row, Col: p.Pos.Col,
					Hint: "the bound was declared with `extends` at the schema's gen list",
				}
			}
		}
		bindings[p.Name] = arg
		full = append(full, arg)
	}

	// Memo lookup (const-interning pattern).
	canonArgs := make([]string, len(full))
	for i, a := range full {
		canonArgs[i] = canonTypeArg(a)
	}
	canon := strings.Join(canonArgs, " ")
	key := genMemoKey(info.Type.ID, canon)
	if body, ok := r.TopTypeBody(key); ok {
		return body, nil
	}

	// Mint the instantiation node as a child of the schema node, named
	// with the canonical instantiation string (D4; const-style
	// non-identifier names). Register the IN-FLIGHT node in the memo
	// BEFORE substituting the body so recursive references (Self of
	// [T]) tie the knot instead of looping.
	displayName := schemaShortName(info) + " of [" + canon + "]"
	node := r.Types.MintType(displayName, info.Type)
	r.Defs.PushType(key, node, NewTypeLiteral(node))

	body, err := SubstituteTypeParams(r, info.Body, bindings, node)
	if err != nil {
		r.Defs.Pop(key)
		r.Types.Retire(node)
		return Value{}, fmt.Errorf("of: %s: %w", displayName, err)
	}

	// Shape the instantiated body per schema kind.
	switch info.Kind {
	case SchemaClass:
		oi, aerr := AsClassType(body)
		if aerr != nil {
			r.Defs.Pop(key)
			r.Types.Retire(node)
			return Value{}, fmt.Errorf("of: %s: class schema body is not an object type", displayName)
		}
		oi.Type = node
		oi.Name = displayName
		body = NewValueRaw(node, oi)
	default:
		// Records / fnsig shapes are STRUCTURAL kinds: the substituted
		// body keeps its structural identity (Parent=TMap / TFnUndef —
		// the record/fn-shape machinery keys on it). The minted node
		// serves the memo (one body Value per instantiation, so `teq`
		// sees identity) and naming; nominal instance identity is the
		// CLASS story (SchemaClass above).
	}

	// Replace the provisional memo body with the final one.
	r.Defs.Pop(key)
	r.Defs.PushType(key, node, body)
	return body, nil
}

// schemaShortName is the last path segment of the schema's installed
// name (Class/Box → Box); falls back to the node name pre-install.
func schemaShortName(info *TypeSchemaInfo) string {
	name := info.Name
	if name == "" && info.Type != nil {
		name = info.Type.Name()
	}
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return "<schema>"
	}
	return name
}
