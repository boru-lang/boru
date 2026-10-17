package modules

import (
	"fmt"
	core "github.com/boru-lang/boru/core/go"
	"strings"

	"github.com/boru-lang/boru/lang/go/native"
)

// The boru:parselang module — the ParseLang namespace of named parsers
// behind the core `parse` macro word. It is the sibling of boru:minilang
// (design/MINILANG.5.md): same macro mechanics, same registration story,
// a separate namespace. Where a mini-language PRODUCES A VALUE, a parser
// CONSUMES A SOURCE and yields whatever its language defines — typically an
// AST, but the return type is Any (could be a transduction, etc.).
//
// Every parser <name> is exported under the partitioned key `parse_<name>`
// and carries the STANDARD parser signature:
//
//	ParseLang.parse_<name> : [ source:String opts:Map ] [ Any ]
//
// sig[0] is the (already-resolved) source text, sig[1] the named options
// (`{}` when the caller gave none). The core `parse` word expands
// `parse <kind> <opts?> <source>` to `ParseLang.parse_<kind> <source> <opts>
// end` — note `source` is the required LAST surface argument (a String or a
// `{src:…}` Source map) while `opts` is the optional middle one. The first
// surface argument may instead BE the parser — a fn value (or a word bound
// to one) carrying the standard prefix — which `parse` calls directly with
// no kind lookup (the ParseLang-value form; see native_macro.go
// parseFnExpand).
//
// The kind namespace is FIXED: the built-in kinds below are the whole set,
// and registration was removed (`ParseLang.register` survives one release
// as a tombstone raising parse_registry_frozen). Custom parsers are
// Function VALUES — `parse <fn> …`, a def-bound name, or a Go-built
// NewParseLangFn value — which are lexically scoped instead of sharing one
// flat namespace.
//
// Out-of-band exports (no `parse_` prefix — never reachable via `parse`):
//
//	ParseLang.register  — TOMBSTONE: raises parse_registry_frozen
//	ParseLang.kinds     — list the (fixed) parser-kind atoms
//	ParseLang.source    — resolve a source value (String | {src:…}) to a String
//
// Source resolution: a String passes through; a `{src:String}` map yields
// its src; a `{file:…}` map raises `parse_file_unsupported` (deferred in
// v1). Every framework-built parser (the built-in kinds, NewParseLangFn
// values, Parse.parser values) gets resolution for free — parseSourceShell
// resolves the source before the parser body runs. A hand-written boru
// parser fn receives the source as given and may call `ParseLang.source`
// to normalise it.

// BuildParseLangModule creates the "boru:parselang" native module: the FIXED
// built-in parser kinds (the tabnas family + aontu) plus the out-of-band
// framework words (kinds / source / the register tombstone).
func BuildParseLangModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()

	// ---- out-of-band: register (TOMBSTONE) ------------------------------
	// The parse kind namespace is fixed; registration was removed. The word
	// survives one release as an unconditional, hint-carrying raise so an
	// existing program fails loudly with the migration path instead of a
	// bare missing-export miss. DryPassWrap mirrors the raise statically, so
	// `boru check` flags the use too (the unquote/splice tombstone pattern).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-register",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(parseRegisterFrozenHandler),
			ReturnsFn:  native.DryPassWrap(parseRegisterFrozenHandler, native.ReturnsStatic()),
		}},
	})
	exports.Set("register", wrapMiniFnDef("parselang-register", [][]native.FnParam{{}},
		[]*native.Type{}, nil, subReg))

	// ---- out-of-band: kinds -------------------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-kinds",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(parseKindsHandler(exports)),
		}},
	})
	exports.Set("kinds", wrapMiniFnDef("parselang-kinds", [][]native.FnParam{{}},
		[]*native.Type{native.TList}, nil, subReg))

	// ---- out-of-band: source ------------------------------------------
	// ParseLang.source <source> resolves a String or {src:…} Source map to
	// a String (so boru parsers can opt into the same normalisation host
	// parsers get automatically).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-source",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny},
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
			Impl:       native.Go(parseSourceHandler),
		}},
	})
	exports.Set("source", wrapMiniFnDef("parselang-source", [][]native.FnParam{
		{{Type: native.TAny}},
	}, []*native.Type{native.TString}, nil, subReg))

	// ---- the FIXED built-in parser kinds ---------------------------------
	// The tabnas parser family (ini, json, jsonic, json5, jsonc, csv, toml,
	// yaml, xml, zon, markdown, feed) ships in the box, so `import
	// "boru:parselang"` then `parse <kind> '<text>'` works out of the box.
	// Each gets source resolution (String or {src:…}) for free
	// (parseSourceShell). aontu (github.com/rjrodger/aontu, a CUE-inspired
	// unification config dialect with no Go port) ships as a hand-written
	// parser in native (see native/aontu.go) and installs the same way.
	// This set is the WHOLE kind namespace — nothing else ever installs
	// into it.
	//
	// The kinds live in predefinedParsers — a Map keyed by kind name (aontu
	// included, no longer a special-cased append) — so no per-kind key is
	// hard-coded at the install site. Installation walks the companion order
	// slice, keeping ParseLang.kinds pinned to the tabnas order followed by
	// aontu (module-parselang.tsv §3).
	byName, order := predefinedParsers()
	for _, name := range order {
		if err := installBuiltinParser(exports, subReg, byName[name]); err != nil { //covergate:allow the built-in kind set is static and name-disjoint (TabnasKinds + aontu), so the duplicate-key arm cannot fire; the installer's arm itself is driven directly by TestW8InstallBuiltinParserDuplicate (§modules)
			return native.ModuleDesc{}, err
		}
	}

	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"ParseLang": exports},
	}, nil
}

// ParseLang is the type of a parse_<lang> parser function — the Go
// implementation behind one registered `parse <kind>` word. It is the
// STANDARD parser signature (see the module comment above) in handler form:
// the framework resolves the source (String or {src:…}) BEFORE the function
// runs, so args[0] is the resolved source String and args[1] the opts Map
// (`{}` when the caller gave none); the ctx/stack slots are unused by the
// parse framing. The function returns the parse result — typically one
// value (an AST, a transduction, …) matching the declared Returns types —
// or a raised parse error.
//
// Every Go-implemented parser carries this type: the built-in kinds
// (tabnasParseHandler, aontuParserSpec), the read-format bridge
// (formatParseHandler), the Parse-builder bridge (parseGrammar.parseHandler),
// and host parsers supplied via ParseLangSpec.Handler.
type ParseLang func(args []native.Value, ctx map[string]native.Value, stack []native.Value, r *native.Registry) ([]native.Value, error)

// ParseLangSpec describes a Go-implemented parser for the value
// constructor (NewParseLangFn). The standard [source:String opts:Map]
// prefix is supplied automatically and the source is RESOLVED to a String
// before the handler runs, so a handler receives args[0]=source:String,
// args[1]=opts.
type ParseLangSpec struct {
	// Name labels the parser (it becomes part of the built value's inner
	// word name, so it must be a plain lowercase word like "calc").
	Name string
	// Returns are the parser's output types (nil → [Any]).
	Returns []*native.Type
	// Handler implements the parser (a ParseLang). Required. Receives the
	// resolved source String and the opts Map.
	Handler ParseLang
	// Pure declares the handler a PURE function of (source, opts): no
	// registry mutation, no I/O, deterministic output. A pure parser's
	// result over a fully-concrete source + opts is folded at CHECK time
	// (pureParseFoldReturns), so a literal-source `parse <kind> '<text>'`
	// types as its actual parse result instead of dynamic(Any). Only the
	// built-in kinds set this — host parsers are unknown code and stay
	// unfolded unless the host opts in.
	Pure bool
}

// formatParseHandler adapts a read Format into a ParseLang: it decodes
// the resolved source (forwarding opts when the format is opts-aware) and
// returns the first decoded value.
func formatParseHandler(name string, f native.Format) ParseLang {
	target := "parse_" + name
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString() // resolved by the framework
		if err != nil {
			return nil, r.BoruError("parse_error", name+": src: "+err.Error(), target)
		}
		var out []native.Value
		var perr error
		if d, ok := f.(native.DecodeOpter); ok {
			out, perr = d.DecodeOpts(src, native.OptsToMap(args[1]))
		} else {
			out, perr = f.Decode(src)
		}
		if perr != nil {
			return nil, r.BoruErrorHint("parse_syntax_error",
				name+": "+native.FirstCleanLine(perr.Error()), target,
				"check that the source is well-formed "+name)
		}
		if len(out) == 0 {
			return []native.Value{native.NewTypeLiteral(native.TNone)}, nil
		}
		return []native.Value{out[0]}, nil
	}
}

// installBuiltinParser registers a shell native that resolves the source
// value to a String and then calls the kind's handler, and exports the
// standard trivial-delegation wrapper under parse_<name>. The dispatch
// source slot is TAny so a String OR a {src:…}/{file:…} Source map matches
// the signature. Build-time only: the built-in kinds are the whole
// namespace.
func installBuiltinParser(exports *native.OrderedMap, subReg *native.Registry, spec ParseLangSpec) error {
	key := "parse_" + spec.Name
	if _, exists := exports.Get(key); exists {
		return fmt.Errorf("register parser %q: already registered", spec.Name)
	}
	shell := parseSourceShell(spec.Handler)
	inner := "parselang-host-" + spec.Name
	sig := native.Signature{
		Args:       []*native.Type{native.TAny, native.TMap},
		Returns:    spec.Returns,
		BarrierPos: -1,
		Impl:       native.Go(shell),
	}
	if spec.Pure {
		sig.ReturnsFn = pureParseFoldReturns(spec.Returns, shell)
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       inner,
		Signatures: []native.Signature{sig},
	})
	params := []native.FnParam{{Type: native.TAny}, {Type: native.TMap}}
	exports.Set(key, wrapMiniFnDef(inner, [][]native.FnParam{params}, spec.Returns, nil, subReg))
	return nil
}

// parseSourceShell wraps a ParseLang in the framework's source resolution:
// the dispatch source slot is TAny (a String OR a {src:…}/{file:…} Source
// map), resolved to a String before the parser runs. Shared by
// installHostParser (the built-in kinds) and Parse.parser's returned
// values (parse.go), so every framework-built parser resolves its source
// identically.
func parseSourceShell(handler ParseLang) native.Handler {
	return func(args []native.Value, named map[string]native.Value, stack []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := resolveParseSource(args[0], r)
		if err != nil {
			return nil, err
		}
		resolved := make([]native.Value, len(args))
		copy(resolved, args)
		resolved[0] = native.NewString(src)
		return handler(resolved, named, stack, r)
	}
}

// pureParseFoldReturns is the check-mode ReturnsFn for a PURE parser kind:
// when the source and opts operands are fully concrete data, it runs the
// real handler (a pure function of its inputs) and surfaces the CONCRETE
// parse result, so a literal-source `parse json '{"a":1}'` types field
// reads precisely instead of ending dynamic(Any). shell is the
// source-RESOLVING wrapper installHostParser builds around the kind's
// ParseLang — NOT the bare ParseLang itself: at fold time args[0] may still
// be a literal {src:…} map (parseFoldableValue admits concrete maps), and
// the shell performs the same resolution the runtime path does. Anything non-concrete —
// a computed source, a carrier-bearing opts map — falls back to the
// declared-Returns dynamic carriers, exactly what declaredReturnCarriers
// would have produced (declared Any rides dynamic). A handler ERROR also
// falls back, silently: the raise is the runtime's job (and may be caught
// by an enclosing `do`), so the check pass must not flag it here.
func pureParseFoldReturns(returns []*native.Type, shell native.Handler) native.ReturnsFunc {
	return func(args []native.Value, r *native.Registry) []native.Value {
		fallback := make([]native.Value, len(returns))
		for i, t := range returns {
			c := native.NewCarrier(t)
			if t.Equal(native.TAny) {
				core.SetDynamic(&c, true)
			}
			fallback[i] = c
		}
		if len(args) != 2 || !parseFoldableValue(args[0]) || !parseFoldableValue(args[1]) {
			return fallback
		}
		out, err := shell(args, nil, nil, r)
		if err != nil || len(out) != len(returns) {
			return fallback
		}
		return out
	}
}

// parseFoldableValue reports v is fully-concrete INERT DATA — a concrete
// scalar (String/Number/Boolean/Atom family, or none), or a concrete
// list/map whose members are recursively so. Only such a value may be fed
// to a pure parse handler at check time: a carrier or dynamic anywhere
// inside (e.g. a computed opts field that defaults during the fold but is
// real at run time) could make the folded result diverge from the runtime
// call, which would be an unsound commitment. Identity-bearing payloads
// (stores, class instances, fn values, timers) are NOT inert — their
// check-time state is not their runtime state — so they decline the fold.
func parseFoldableValue(v native.Value) bool {
	if v.Carrier() || core.IsDynamic(&v) || core.IsUndefined(&v) || !native.IsConcrete(&v) {
		return false
	}
	switch {
	case v.Parent().ConformsTo(native.TList):
		l, err := native.AsList(v)
		if err != nil || l.IsNil() {
			return false
		}
		for i := 0; i < l.Len(); i++ {
			if !parseFoldableValue(l.Get(i)) {
				return false
			}
		}
		return true
	case v.Parent().ConformsTo(native.TMap):
		m, err := native.AsMap(v)
		if err != nil || m == nil {
			return false
		}
		for _, k := range m.Keys() {
			e, _ := m.Get(k)
			if !parseFoldableValue(e) {
				return false
			}
		}
		return true
	default:
		return v.Parent().ConformsTo(native.TScalar) || v.Parent().Equal(native.TNone)
	}
}

// resolveParseSource normalises a `parse` source argument to its String
// contents: a String passes through; a `{src:String}` map yields its src; a
// `{file:…}` map raises `parse_file_unsupported` (deferred in v1); anything
// else is a clear error.
func resolveParseSource(v native.Value, r *native.Registry) (string, error) {
	if v.Parent().ConformsTo(native.TString) && native.IsConcrete(&v) {
		return v.AsConcreteString()
	}
	if v.Parent().Equal(native.TMap) && native.IsConcrete(&v) {
		m, _ := native.AsMap(v)
		if m == nil {
			return "", r.BoruError("parse_bad_source", "parse: source map is empty", "parse")
		}
		if _, ok := m.Get("file"); ok {
			return "", r.BoruErrorHint("parse_file_unsupported",
				"parse: {file:…} source is not yet supported", "parse",
				"use an inline string or {src:'…'} for now")
		}
		if s, ok := native.MapFieldString(m, "src"); ok {
			return s, nil
		}
		return "", r.BoruErrorHint("parse_bad_source",
			"parse: source map must have a 'src' String field", "parse",
			"write {src:'…'} or pass the source string directly")
	}
	return "", r.BoruErrorHint("parse_bad_source",
		"parse: source must be a String or a {src:…} map", "parse",
		"e.g. parse calc 'x + y'  or  parse calc {src:'x + y'}")
}

// parseSourceHandler is the ParseLang.source word: resolve a source value to
// its String.
func parseSourceHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := resolveParseSource(args[0], r)
	if err != nil {
		return nil, err
	}
	return []native.Value{native.NewString(s)}, nil
}

// parseRegisterFrozenHandler is ParseLang.register's TOMBSTONE: the parse
// kind namespace is FIXED (the built-in kinds are the whole set), so
// registration was removed. An unconditional, hint-carrying raise — the
// DryPassWrap mirror on its signature surfaces the same finding statically.
func parseRegisterFrozenHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	return nil, r.BoruErrorHint("parse_registry_frozen",
		"register: the parse kind namespace is fixed — registration was removed", "register",
		"pass the parser as a Function value instead: def myp (fn [[source:String opts:Map] [Any] [...]])  parse myp '...' — Go hosts build one with NewParseLangFn")
}

// predefinedParsers returns the FIXED set of built-in parser kinds — the
// tabnas parser family plus aontu — as a Map keyed by kind name, together
// with the pinned installation order (the tabnas order from
// native.TabnasKinds followed by aontu). It is the single source of the
// predefined-lang set: BuildParseLangModule installs every entry by walking
// the order slice, so no per-kind key is hard-coded at the call site while
// ParseLang.kinds stays byte-stable (module-parselang.tsv §3). The names are
// disjoint (TabnasKinds is disjoint, aontu is not a tabnas kind), so each
// name keys exactly one spec.
func predefinedParsers() (map[string]ParseLangSpec, []string) {
	specs := append(tabnasParserSpecs(), aontuParserSpec())
	byName := make(map[string]ParseLangSpec, len(specs))
	order := make([]string, len(specs))
	for i, spec := range specs {
		byName[spec.Name] = spec
		order[i] = spec.Name
	}
	return byName, order
}

// tabnasParserSpecs returns the parser kinds backed by the tabnas parser
// family, built from the shared native.TabnasKinds() core (which read also
// consumes — see lang/go/native/tabnas.go). Every kind ships built-in —
// importing boru:parselang is enough to use `parse <kind> '<text>'`, no host
// registration needed.
//
// The slice order mirrors native.TabnasKinds() and is pinned by
// ParseLang.kinds (lang/spec/module-parselang.tsv §3).
func tabnasParserSpecs() []ParseLangSpec {
	kinds := native.TabnasKinds()
	specs := make([]ParseLangSpec, len(kinds))
	for i, k := range kinds {
		specs[i] = ParseLangSpec{
			Name:    k.Name,
			Returns: []*native.Type{k.Returns},
			Handler: tabnasParseHandler(k.Name, k.Parse, k.Convert),
			// The tabnas decoders are pure functions of (source, opts) —
			// no registry state, no I/O — so a literal source folds at
			// check time.
			Pure: true,
		}
	}
	return specs
}

// aontuParserSpec is the built-in aontu parse kind. Its handler decodes the
// resolved source via native.AontuParse and converts the generic result
// (map[string]any / []any / scalars) to a boru Node of Maps and Lists; a
// decode or unification failure raises [boru/parse_syntax_error].
func aontuParserSpec() ParseLangSpec {
	return ParseLangSpec{
		Name:    "aontu",
		Returns: []*native.Type{native.TAny},
		// AontuParse is a pure decode + unification over the source text —
		// same fold contract as the tabnas family.
		Pure: true,
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
			src, err := args[0].AsConcreteString() // resolved by the framework
			if err != nil {
				return nil, r.BoruError("parse_error", "aontu: src: "+err.Error(), "parse_aontu")
			}
			v, perr := native.AontuParse(src, native.OptsToMap(args[1]))
			if perr != nil {
				return nil, r.BoruErrorHint("parse_syntax_error",
					native.FirstCleanLine(perr.Error()), "parse_aontu",
					"check that the source is well-formed aontu")
			}
			return []native.Value{native.AnyToValue(v)}, nil
		},
	}
}

// tabnasParseHandler builds the ParseLang for one tabnas kind: it runs
// the decoder over the (already-resolved) source string and converts the
// result to a boru Value. A decode failure raises [boru/parse_syntax_error].
func tabnasParseHandler(kind string, parse native.TabnasParser, convert func(any) native.Value) ParseLang {
	target := "parse_" + kind
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString() // resolved by the framework
		if err != nil {
			return nil, r.BoruError("parse_error", kind+": src: "+err.Error(), target)
		}
		v, perr := parse(src, native.OptsToMap(args[1]))
		if perr != nil {
			return nil, r.BoruErrorHint("parse_syntax_error",
				kind+": "+native.FirstCleanLine(perr.Error()), target,
				"check that the source is well-formed "+kind)
		}
		return []native.Value{convert(v)}, nil
	}
}

// parseKindsHandler lists the registered parser-kind atoms (parse_ stripped),
// in registration order.
func parseKindsHandler(exports *native.OrderedMap) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		var kinds []native.Value
		for _, k := range exports.Keys() {
			if strings.HasPrefix(k, "parse_") {
				kinds = append(kinds, native.NewAtom(strings.TrimPrefix(k, "parse_")))
			}
		}
		return []native.Value{native.NewList(kinds)}, nil
	}
}
