package native

import (
	core "github.com/boru-lang/boru/core/go"
)

// inspectNatives installs `inspect` — the machine-readable
// counterpart of `help` for words, values, and types.
//
//	inspect NAME   — NAME is a bare word or a quoted atom. Resolves it
//	                 (type stack, then def stack, then registered word)
//	                 and returns an `Inspect` value — a Map describing
//	                 the thing.
//	inspect V      — anything else (a concrete value, a type literal, a
//	                 record / table / disjunct / enum / typed list /
//	                 typed map / fn-shape / dependent scalar). Returns
//	                 the type inspection of V.
//
// The result shape:
//
//	word inspection:  { name, kind:native|defined|unknown,
//	                    value?, signatures:[{args:[…]}…] }
//	                    (`value` only for a plain `def`-bound value)
//	type inspection:  { name?, type:"<leaf>", kind, …kind-specific }
//	  • for a TYPE value: type = "Type", struct = underlying leaf;
//	  • for a CONCRETE value: type = Parent leaf, no `struct`.
//	  record / table → fields:{k:"<leaf>" …}
//	  object         → parent:"…"?, fields:{k:"<leaf>" …} (incl. inherited)
//	  disjunct       → alternatives:["…" …]
//	  typed_list / typed_map → child:"…"
//	  function_shape → signatures:[{params:[…] returns:[…]} …]
//	  dependent_scalar → leaf, lo:{kind,value}?, hi:{kind,value}?
//	  literal        → (just kind)
//
// The returned value carries Parent `Inspect` but its payload is an
// OrderedMap so it renders and round-trips like a map. Algorithms
// (IsRecordType / AsClassType / IsDepScalar / …) live in eng; this
// file owns the word name and dispatch wiring.
var inspectNatives = []NativeFunc{
	{
		Name: "inspect",

		Signatures: []Signature{
			// /q captures the upcoming Word as an Atom; the same sig
			// also matches an explicit Atom on the stack (per
			// signature.go §1.5 — Atom/q subsumes Atom).
			{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}, Impl: Go(inspectAtomHandler), Returns: []*Type{TInspect}, BarrierPos: -1, CompileEffect: CompileReadsFn | CompileQuoteInert}, // bare-word inspect (`inspect foo/q`) bakes the inert Atom const + CALL_NATIVE
			{Args: []*Type{TAny}, Impl: Go(inspectTypeHandler), Returns: []*Type{TInspect}, BarrierPos: -1, CompileEffect: CompileReadsFn},                                                        // reads a fn value, never invokes
		},
	},

	// canon — render any value as canonical boru source (core.CanonValue):
	// the same renderer the spec suites and the doc-example harness
	// compare against. For DATA values — scalars, atoms (`name/q`),
	// paths, none, type literals, plain and flex Node trees — the
	// result is ROUND-TRIPPABLE source: evaluating it (e.g. via
	// `Vm.run`) reproduces an equivalent value, with flexness marked at
	// every level (`(flex {a:(flex [1])})`). Values whose identity is
	// the live container (Store, Array, object instances, functions,
	// timers) still render, but rebuilding them from source produces a
	// FRESH thing — identity does not round-trip, only the rendering.
	{
		Name: "canon",

		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(canonHandler), Returns: []*Type{TString}, BarrierPos: -1,
				CompileEffect: CompileReadsFn}, // renders a fn value's content, never invokes it
		},
	},
}

func canonHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewString(core.CanonValue(args[0]))}, nil
}

func inspectAtomHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, _ := args[0].AsConcreteAtom()
	if tv, ok := r.TopTypeBody(name); ok {
		return []Value{buildTypeInspection(name, tv)}, nil
	}
	if top, ok := r.Defs.Top(name); ok {
		// FnDef / Function defs are functions, not types — report
		// their sig structure via buildInspection instead of treating
		// the def as a type body.
		if IsTypeBody(&top) && !top.Parent.Equal(TFunction) {
			return []Value{buildTypeInspection(name, top)}, nil
		}
	}
	// Builtin arm of the canonical cascade: any type registered after
	// parser init reaches this handler as a captured Atom (`inspect
	// Date`), exactly like a user type — without this arm it rendered
	// kind:unknown (NUR059).
	if t, ok := core.ResolveBuiltinTypeName(name); ok {
		return []Value{buildTypeInspection(name, NewTypeLiteral(t))}, nil
	}
	return []Value{buildInspection(r, name)}, nil
}

func inspectTypeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{buildTypeInspection("", args[0])}, nil
}

// buildInspection constructs a word-inspection map for the named word.
func buildInspection(r *Registry, name string) Value {
	result := NewOrderedMap()
	result.Set("name", NewString(name))

	fn := r.Lookup(name)
	if fn == nil {
		if r.Defs.Has(name) {
			result.Set("kind", NewAtom("defined"))
			// A plain `def`-bound value (not a registered word): include
			// the value itself, e.g. `def f 99; inspect f` → {…value:99…}.
			if v, ok := r.Defs.Top(name); ok {
				result.Set("type", NewString(v.Parent.Leaf()))
				result.Set("value", v)
			}
			result.Set("signatures", NewList(nil))
			return NewValueRaw(TInspect, MapPayload{M: result})
		}
		result.Set("kind", NewAtom("unknown"))
		result.Set("signatures", NewList(nil))
		return NewValueRaw(TInspect, MapPayload{M: result})
	}

	// Every inspection reports a `type`: a word bound to a function is a
	// value like any other, and by ADR-011 there is exactly one Function
	// type for it to have. Without this the word form was the only
	// inspection with no `type` key at all, so `inspect f` and
	// `inspect 2` answered different questions.
	result.Set("type", NewString(TFunction.Leaf()))

	ownSigs := fn.OwnSigs()
	isDefined := false
	for i := range ownSigs {
		if len(ownSigs[i].Body()) > 0 {
			isDefined = true
			break
		}
	}
	if isDefined {
		result.Set("kind", NewAtom("defined"))
	} else {
		result.Set("kind", NewAtom("native"))
	}

	// Iterate the full aggregate dispatch table (which, for a boru fn,
	// includes the synthetic 0-arg fallback rendered as {args:[]}) so the
	// inspect surface matches the dispatcher's view.
	var sigMaps []Value
	for _, sig := range fn.Signatures {
		sm := NewOrderedMap()

		var argVals []Value
		for _, argType := range sig.ArgTypes() {
			argVals = append(argVals, NewString(argType.Leaf()))
		}
		if argVals == nil {
			argVals = []Value{}
		}
		sm.Set("args", NewList(argVals))

		// The RETURN types. Omitting them made `inspect` an incomplete
		// answer to "what is this function's signature?" — half a
		// contract, with no way to recover the other half. Shared with
		// `describe` (SigReturnNames) so the two surfaces agree.
		retVals := []Value{}
		for _, retName := range SigReturnNames(name, sig) {
			retVals = append(retVals, NewString(retName))
		}
		sm.Set("returns", NewList(retVals))

		sigMaps = append(sigMaps, NewMap(sm))
	}
	if sigMaps == nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		sigMaps = []Value{}
	}
	result.Set("signatures", NewList(sigMaps))

	return NewValueRaw(TInspect, MapPayload{M: result})
}

// buildTypeInspection constructs a type-inspection map for a type value.
func buildTypeInspection(name string, tv Value) Value {
	result := NewOrderedMap()

	if name != "" {
		result.Set("name", NewString(name))
	}

	// A TYPE value (a bare type literal, or a structural type body) has
	// type-of `*Type`; its underlying structure leaf goes to `struct`.
	// A concrete value reports its own Parent leaf as `type` and has no
	// `struct`.
	if IsBareTypeNode(&tv) || IsTypeBody(&tv) || IsRecordShape(tv) {
		result.Set("type", NewString("Type"))
		result.Set("struct", NewString(TypeNameOf(tv)))
	} else {
		result.Set("type", NewString(tv.Parent.Leaf()))
	}

	switch {
	case IsRecordType(tv):
		result.Set("kind", NewAtom("record"))
		rt, _ := AsRecordType(tv)
		fields := NewOrderedMap()
		for _, k := range rt.Fields.Keys() {
			v, _ := rt.Fields.Get(k)
			fields.Set(k, NewString(TypeNameOf(v)))
		}
		result.Set("fields", NewMap(fields))

	case IsRecordShape(tv):
		result.Set("kind", NewAtom("record"))
		m, _ := AsMap(tv)
		fields := NewOrderedMap()
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			fields.Set(k, NewString(TypeNameOf(v)))
		}
		result.Set("fields", NewMap(fields))

	case IsClassType(&tv):
		result.Set("kind", NewAtom("object"))
		oi, _ := AsClassType(tv)
		if oi.Parent != nil {
			result.Set("parent", NewString(oi.Parent.Name))
		}
		af := oi.AllFields()
		fields := NewOrderedMap()
		for _, k := range af.Keys() {
			v, _ := af.Get(k)
			fields.Set(k, NewString(TypeNameOf(v)))
		}
		result.Set("fields", NewMap(fields))

	case IsTableType(tv):
		result.Set("kind", NewAtom("table"))
		tt, _ := AsTableType(tv)
		fields := NewOrderedMap()
		for _, k := range tt.Record.Fields.Keys() {
			v, _ := tt.Record.Fields.Get(k)
			fields.Set(k, NewString(TypeNameOf(v)))
		}
		result.Set("fields", NewMap(fields))

	case IsDisjunct(&tv):
		result.Set("kind", NewAtom("disjunct"))
		di, _ := AsDisjunct(tv)
		alts := make([]Value, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			alts[i] = NewString(TypePathOf(alt))
		}
		result.Set("alternatives", NewList(alts))

	case IsTypedList(&tv):
		result.Set("kind", NewAtom("typed_list"))
		ci, _ := AsChildType(tv)
		result.Set("child", NewString(TypePathOf(ci.Child)))

	case IsTypedMap(&tv):
		result.Set("kind", NewAtom("typed_map"))
		ci, _ := AsChildType(tv)
		result.Set("child", NewString(TypePathOf(ci.Child)))

	case ValueType(tv).Equal(TFnUndef):
		result.Set("kind", NewAtom("function_shape"))
		uInfo, _ := tv.Data.(FnUndefInfo)
		sigs := make([]Value, 0, len(uInfo.Sigs))
		for _, spec := range uInfo.Sigs {
			sig := NewOrderedMap()
			params := make([]Value, len(spec.Params))
			for i, p := range spec.Params {
				params[i] = NewString(p.Type.Leaf())
			}
			sig.Set("params", NewList(params))
			rets := make([]Value, len(spec.Returns))
			for i, ret := range spec.Returns {
				rets[i] = NewString(ret.Leaf())
			}
			sig.Set("returns", NewList(rets))
			sigs = append(sigs, NewMap(sig))
		}
		result.Set("signatures", NewList(sigs))

	case tv.IsDepScalar():
		result.Set("kind", NewAtom("dependent_scalar"))
		info, _ := tv.AsDepScalar()
		result.Set("leaf", NewString(tv.Parent.Name()))
		if info.Lo != nil {
			lo := NewOrderedMap()
			lo.Set("kind", NewString(BoundToKind(info.Lo, true).String()))
			lo.Set("value", info.Lo.Value)
			result.Set("lo", NewMap(lo))
		}
		if info.Hi != nil {
			hi := NewOrderedMap()
			hi.Set("kind", NewString(BoundToKind(info.Hi, false).String()))
			hi.Set("value", info.Hi.Value)
			result.Set("hi", NewMap(hi))
		}

	default:
		result.Set("kind", NewAtom("literal"))
	}

	return NewValueRaw(TInspect, MapPayload{M: result})
}
