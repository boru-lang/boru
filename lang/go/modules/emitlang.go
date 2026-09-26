package modules

import (
	"fmt"
	"strings"

	"github.com/boru-lang/boru/lang/go/native"
)

// The boru:emitlang module — the EmitLang namespace of named emitters behind
// the core `emit` macro word. It is the symmetric inverse of boru:parselang:
// where a parser CONSUMES A SOURCE and yields a value, an emitter CONSUMES A
// VALUE and yields a string.
//
// Every emitter <name> is exported under the partitioned key `emit_<name>`
// and carries the STANDARD emitter signature:
//
//	EmitLang.emit_<name> : [ value:Any opts:Map ] [ String ]
//
// sig[0] is the value to render, sig[1] the named options (`{}` when the
// caller gave none). The core `emit` word expands `emit <kind> <opts?>
// <data>` to `EmitLang get emit_<kind> <data> <opts> end` — `data` is the
// required LAST surface argument while `opts` is the optional middle one.
//
// The kind namespace is FIXED: the built-in kinds below are the whole set,
// and registration was removed (`EmitLang.register` survives one release as
// a tombstone raising emit_registry_frozen). Custom emitters are Function
// VALUES — `emit <fn> <data>`, a def-bound name, or a Go-built NewEmitLangFn
// value — which are lexically scoped instead of sharing one flat namespace.
//
// Out-of-band exports (no `emit_` prefix — never reachable via `emit`):
//
//	EmitLang.register  — TOMBSTONE: raises emit_registry_frozen
//	EmitLang.kinds     — list the (fixed) emitter-kind atoms
//
// `emit_auto` is the natural-format dispatcher: it resolves a value's natural
// emit kind (Map/List→json, Table→csv, Xml→xml) and delegates to it.

// BuildEmitLangModule creates the "boru:emitlang" native module: the FIXED
// built-in emit kinds from native.EmitKinds() plus the out-of-band framework
// words (kinds / the register tombstone / emit_auto).
func BuildEmitLangModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()

	// ---- out-of-band: register (TOMBSTONE) ------------------------------
	// The emit kind namespace is fixed; registration was removed. The word
	// survives one release as an unconditional, hint-carrying raise so an
	// existing program fails loudly with the migration path instead of a
	// bare missing-export miss. DryPassWrap mirrors the raise statically, so
	// `boru check` flags the use too (the unquote/splice tombstone pattern).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "emitlang-register",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(emitRegisterFrozenHandler),
			ReturnsFn:  native.DryPassWrap(emitRegisterFrozenHandler, native.ReturnsStatic()),
		}},
	})
	exports.Set("register", wrapMiniFnDef("emitlang-register", [][]native.FnParam{{}},
		[]*native.Type{}, nil, subReg))

	// ---- out-of-band: kinds -------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "emitlang-kinds",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(emitKindsHandler(exports)),
		}},
	})
	exports.Set("kinds", wrapMiniFnDef("emitlang-kinds", [][]native.FnParam{{}},
		[]*native.Type{native.TList}, nil, subReg))

	// ---- emit_auto: the natural-format dispatcher ---------------------
	// The opts slot carries the UNION Options schema of auto's natural-target
	// kinds (json / csv / xml), so an unknown opts key is a hard dispatch
	// rejection rather than a silently ignored option (G6) — see
	// installHostEmitter for the per-kind rule.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "emitlang-auto",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny, native.TMap},
			Patterns:   map[int]native.Value{1: native.EmitAutoOptsSchema()},
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
			Impl:       native.Go(emitAutoHandler),
		}},
	})
	exports.Set("emit_auto", wrapMiniFnDef("emitlang-auto", [][]native.FnParam{
		{{Type: native.TAny}, {Type: native.TMap}},
	}, []*native.Type{native.TString}, nil, subReg))

	// ---- built-in emit kinds --------------------------------------------
	// The canonical walk-based emitter family (json, jsonic, yaml, csv, tsv,
	// toml, ini, xml) ships in the box — and IS the whole kind set.
	//
	// The kinds live in predefinedEmitters — a Map keyed by kind name — so no
	// per-kind key is hard-coded at the install site. Installation walks the
	// companion order slice, keeping EmitLang.kinds pinned to native.EmitKinds
	// order (module-emitlang.tsv).
	// ---- out-of-band: fn dispatch (compile-pass seam, NOT exported) ------
	// The runtime twin of `emit <fn> <opts?> <data>` for a leading operand
	// the compile pass could not see concretely (macro_fn_dispatch.go).
	registerMacroFnDispatch(subReg, parent, "emitlang-fn-dispatch", emitFnDispatchHandler,
		native.InstallEmitLangFnDispatch, 2, 3)

	byName, order := predefinedEmitters()
	for _, name := range order {
		if err := installBuiltinEmitter(exports, subReg, byName[name]); err != nil { //covergate:allow the built-in kind set is static and name-disjoint (native.EmitKinds), so the duplicate-key arm cannot fire; the installer's arm itself is driven directly by TestW8InstallBuiltinEmitterDuplicate (§modules)
			return native.ModuleDesc{}, err
		}
	}

	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"EmitLang": exports},
	}, nil
}

// EmitLangSpec describes a Go-implemented emitter for the value constructor
// (NewEmitLangFn). The standard [value:Any opts:Map] prefix is supplied
// automatically; the handler receives args[0]=value, args[1]=opts.
type EmitLangSpec struct {
	// Name is the emitter's name — used in the value's inner native word and
	// in error messages. Lowercase, like the name it will usually be bound to.
	Name string
	// Returns are the emitter's output types (nil → [String]).
	Returns []*native.Type
	// Handler implements the emitter. Required. Receives the value and opts.
	Handler native.Handler

	// optsSchema, when set (an Options type literal), rides the inner
	// native's opts slot as a dispatch Pattern: a CONCRETE opts map with a
	// key outside the schema is a hard signature rejection at check and run
	// time — the G6 options-typo fix (`emit json {prety:true}` must not
	// silently emit compact JSON). Unexported: only the BUILT-IN kinds
	// declare it (builtinEmitSpecs, from native.EmitOptsSchema — the exact
	// key set each Encode reads). A value-form emitter owns its key set, so
	// its opts stay an unchecked plain Map.
	optsSchema native.Value
}

// installBuiltinEmitter registers a shell native that calls the kind's
// handler and exports the standard trivial-delegation wrapper under
// emit_<name>.
func installBuiltinEmitter(exports *native.OrderedMap, subReg *native.Registry, spec EmitLangSpec) error {
	key := "emit_" + spec.Name
	if _, exists := exports.Get(key); exists {
		return fmt.Errorf("register emitter %q: already registered", spec.Name)
	}
	returns := spec.Returns
	if returns == nil {
		returns = []*native.Type{native.TString}
	}
	inner := "emitlang-host-" + spec.Name
	sig := native.Signature{
		Args:       []*native.Type{native.TAny, native.TMap},
		Returns:    returns,
		BarrierPos: -1,
		Impl:       native.Go(spec.Handler),
	}
	// A built-in kind's known opts keys ride as an Options-schema Pattern so
	// dispatch rejects a typo'd key outright (see EmitLangSpec.optsSchema).
	if native.IsOptionsType(spec.optsSchema) {
		sig.Patterns = map[int]native.Value{1: spec.optsSchema}
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       inner,
		Signatures: []native.Signature{sig},
	})
	params := []native.FnParam{{Type: native.TAny}, {Type: native.TMap}}
	exports.Set(key, wrapMiniFnDef(inner, [][]native.FnParam{params}, returns, nil, subReg))
	return nil
}

// predefinedEmitters returns the FIXED set of built-in emit kinds — the
// canonical walk-based emitter family — as a Map keyed by kind name, together
// with the pinned installation order (native.EmitKinds order). It is the
// single source of the predefined-kind set: BuildEmitLangModule installs every
// entry by walking the order slice, so no per-kind key is hard-coded at the
// call site while EmitLang.kinds stays byte-stable (module-emitlang.tsv). The
// names are disjoint (EmitKinds is disjoint), so each name keys exactly one
// spec.
func predefinedEmitters() (map[string]EmitLangSpec, []string) {
	specs := builtinEmitSpecs()
	byName := make(map[string]EmitLangSpec, len(specs))
	order := make([]string, len(specs))
	for i, spec := range specs {
		byName[spec.Name] = spec
		order[i] = spec.Name
	}
	return byName, order
}

// builtinEmitSpecs returns the emitter kinds backed by the canonical
// walk-based emitter core (native.EmitKinds()). The slice order mirrors
// EmitKinds() and is pinned by EmitLang.kinds (lang/spec/module-emitlang.tsv).
func builtinEmitSpecs() []EmitLangSpec {
	kinds := native.EmitKinds()
	specs := make([]EmitLangSpec, len(kinds))
	for i, k := range kinds {
		schema, _ := native.EmitOptsSchema(k.Name)
		specs[i] = EmitLangSpec{
			Name:       k.Name,
			Returns:    []*native.Type{native.TString},
			Handler:    emitKindHandler(k.Name, k.Encode),
			optsSchema: schema,
		}
	}
	return specs
}

// emitKindHandler builds the emitter native for one kind: it runs the kind's
// Encode over the value and opts, mapping an emit error to its boru code.
func emitKindHandler(kind string, encode func(native.Value, map[string]any) (string, error)) native.Handler {
	target := "emit_" + kind
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		s, err := encode(args[0], native.OptsToMap(args[1]))
		if err != nil {
			if code := native.EmitErrorCode(err); code != "" {
				return nil, r.BoruError(code, err.Error(), target)
			}
			return nil, r.BoruError("emit_error", kind+": "+err.Error(), target)
		}
		return []native.Value{native.NewString(s)}, nil
	}
}

// emitAutoHandler resolves a value's natural emit kind and delegates to that
// kind's encoder. No natural target → emit_no_natural (with a hint listing
// the built-in kinds).
func emitAutoHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	name, ok := native.NaturalEmitKind(args[0])
	if !ok {
		return nil, r.BoruErrorHint("emit_no_natural",
			"emit: this value has no natural emit format", "emit_auto",
			"name a kind explicitly: emit <kind> <data>. Kinds: "+native.EmitKindNames())
	}
	for _, k := range native.EmitKinds() {
		if k.Name == name {
			s, err := k.Encode(args[0], native.OptsToMap(args[1]))
			if err != nil {
				if code := native.EmitErrorCode(err); code != "" {
					return nil, r.BoruError(code, err.Error(), "emit_auto")
				}
				return nil, r.BoruError("emit_error", name+": "+err.Error(), "emit_auto") //covergate:allow module provably-invariant / grammar-defensive guard (§modules)
			}
			return []native.Value{native.NewString(s)}, nil
		}
	}
	return nil, r.BoruError("emit_no_natural", //covergate:allow module provably-invariant / grammar-defensive guard (§modules)
		"emit: no encoder for natural kind "+name, "emit_auto")
}

// emitRegisterFrozenHandler is EmitLang.register's TOMBSTONE: the emit kind
// namespace is FIXED (the built-in kinds are the whole set), so registration
// was removed. An unconditional, hint-carrying raise — the DryPassWrap
// mirror on its signature surfaces the same finding statically.
func emitRegisterFrozenHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	return nil, r.BoruErrorHint("emit_registry_frozen",
		"register: the emit kind namespace is fixed — registration was removed", "register",
		"pass the emitter as a Function value instead: def mye (fn [[value:Any opts:Map] [String] [...]])  emit mye <data> — Go hosts build one with NewEmitLangFn")
}

// emitKindsHandler lists the (fixed) emitter-kind atoms (emit_ stripped),
// in registration order. emit_auto is excluded — it is the natural-format
// dispatcher, not a named kind.
func emitKindsHandler(exports *native.OrderedMap) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		var kinds []native.Value
		for _, k := range exports.Keys() {
			if k == "emit_auto" {
				continue
			}
			if strings.HasPrefix(k, "emit_") {
				kinds = append(kinds, native.NewAtom(strings.TrimPrefix(k, "emit_")))
			}
		}
		return []native.Value{native.NewList(kinds)}, nil
	}
}
