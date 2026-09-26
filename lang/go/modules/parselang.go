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

	// ---- out-of-band: fn dispatch (compile-pass seam, NOT exported) ------
	// parselang-fn-dispatch is the RUNTIME twin of the `parse` macro's
	// ParseLang-VALUE form for a parser operand that is not concrete under
	// analysis (a computed parser, a Function-typed binding). The compile
	// pass records ONE call (fn, source, opts) — the fn operand's provenance
	// is its own producing event — and at run time this word dispatches the
	// LIVE fn through the same `<fn> <source> <opts> end` sequence
	// parseFnExpand splices, enforcing the same ParseLangFnSigWhy contract
	// with byte-identical errors. NOT exported: the surface stays
	// `parse <fn>`.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-fn-dispatch",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny, native.TAny, native.TMap},
			Returns:    []*native.Type{native.TAny},
			BarrierPos: -1,
			// arg0 (the computed parser fn) is READ AS DATA: when it is bound to
			// an enclosing `def op (Parse.parser g)`, the compiled dyn-scope
			// operand uses OpLookupDynScopeData so the parser value PUSHES rather
			// than deferring on its FnDefInfo binding — matching the interpreter,
			// which passes the /q-captured name as data to parseFnDispatchHandler.
			FnDataArgs: map[int]bool{0: true},
			// arg0 is also INERT to the VM: parseFnDispatchHandler runs the
			// parser in its OWN sub-engine (never re-stepped on the VM tape), so
			// a CONCRETE parser fn value (a detached stamp binds the runtime-
			// constructed parser concretely) bakes as a plain const operand
			// rather than tripping the fn-value Stage-3 compile failure.
			FnInertArgs: map[int]bool{0: true},
			Impl:        native.Go(parseFnDispatchHandler),
		}},
	})
	if fn := subReg.Lookup("parselang-fn-dispatch"); fn != nil && len(fn.Signatures) == 1 {
		native.InstallParseLangFnDispatch(parent, &fn.Signatures[0])
	}
	// parselang-lead-dispatch is the same seam for a GRADUAL leading operand
	// (`parse m.p 'x'` over a container member the pass knows only as
	// dynamic(Any)): the value may be a parser fn or a kind name, so a
	// non-fn re-runs the `parse` word itself, whose classification decides
	// (the kind, or the literal-name error); a fn takes the fn dispatch.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parselang-lead-dispatch",
		Signatures: []native.Signature{{
			Args:        []*native.Type{native.TAny, native.TAny, native.TMap},
			Returns:     []*native.Type{native.TAny},
			BarrierPos:  -1,
			FnDataArgs:  map[int]bool{0: true},
			FnInertArgs: map[int]bool{0: true},
			Impl:        native.Go(parseLeadDispatchHandler),
		}},
	})
	if fn := subReg.Lookup("parselang-lead-dispatch"); fn != nil && len(fn.Signatures) == 1 {
		native.InstallParseLangLeadDispatch(parent, &fn.Signatures[0])
	}

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
				c.Dynamic = true
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
	if v.Carrier || v.Dynamic || v.Undefined || !native.IsConcrete(v) {
		return false
	}
	switch {
	case v.Parent.ConformsTo(native.TList):
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
	case v.Parent.ConformsTo(native.TMap):
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
		return v.Parent.ConformsTo(native.TScalar) || v.Parent.Equal(native.TNone)
	}
}

// resolveParseSource normalises a `parse` source argument to its String
// contents: a String passes through; a `{src:String}` map yields its src; a
// `{file:…}` map raises `parse_file_unsupported` (deferred in v1); anything
// else is a clear error.
func resolveParseSource(v native.Value, r *native.Registry) (string, error) {
	if v.Parent.ConformsTo(native.TString) && native.IsConcrete(v) {
		return v.AsConcreteString()
	}
	if v.Parent.Equal(native.TMap) && native.IsConcrete(v) {
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

// parseFnDispatchHandler is the runtime resolver behind the compiled
// `parse <fn>` value-form dispatch for a non-concrete parser operand (see
// the registration comment in BuildParseLangModule). args are
// [fn source opts]. It enforces the contract parseFnExpand enforces at
// expansion time — the operand must be a Function carrying the standard
// [source opts] prefix (native.ParseLangFnSigWhy) — with byte-identical
// errors, then replays the value-form expansion tail: the fn value followed
// by `source opts end`, stepped in a sub-engine over the calling registry.
func parseFnDispatchHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	// fnVal is the parser as the interpreter would see it: a COMPILED CLOSURE
	// (a factory's capturing lambda — `parse (mk '!') 'x'`) is read through
	// the compiled runtime's bridge, the FnDefInfo the interpreter minted for
	// the same source, for this one dispatch. Before, the closure reached the
	// FnDefInfo assertion below and raised "not a usable function value"
	// where the interpreter parsed (2026-09-26).
	fnVal := args[0]
	if native.IsCompiledClosure(fnVal) {
		if bv, bridged := closureFnView(r, fnVal); bridged {
			fnVal = bv
		}
	}
	fnDef, ok := fnVal.Data.(native.FnDefInfo)
	if !ok {
		return nil, r.BoruErrorHint("parse_error",
			"parse: the parser is not a usable function value", "parse",
			"pass a parser fn: parse (fn [[source:String opts:Map] [Any] [...]]) 'x'")
	}
	if why := native.ParseLangFnSigWhy(fnDef); why != "" {
		return nil, r.BoruErrorHint("parse_bad_signature",
			"parse: "+why, "parse",
			"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
	}
	// The value-form expansion tail — the fn value followed by `source opts
	// end` — used to be STEPPED in a sub-engine unconditionally, which is a
	// whole interpreter run sitting inside a compiled program: every
	// `parse <parser> …` corpus row carried one. Two lanes replace it, chosen
	// by what the parser VALUE actually is, and the token run stays as the
	// backstop for anything neither lane claims.
	//
	// callArgs are in SIG order, which for the enforced prefix is
	// [source, opts]: MatchFnSig tests args[j] against Params[j], MatchSignature
	// assigns the same way, and the token run reached that assignment by the
	// other route — `fn source opts` is forward form, and forward args fill sig
	// positions in written order (eng/go/CLAUDE.md §Signature Ordering).
	callArgs := []native.Value{args[1], args[2]}
	// A parser whose matched overload has a GO handler dispatches directly. Two
	// shapes reach this, and neither is a boru body the callback seam could run:
	//
	//   - `Parse.parser` mints a TRIVIAL-DELEGATION wrapper whose body is the
	//     single Word naming the Go native that runs the built grammar;
	//   - `def myp (Parse.parser g)` REBINDS that wrapper, and InstallDef's
	//     module-wrapper branch binds the inner native's Signatures verbatim
	//     under the new name, so the value carries Go sigs outright.
	//
	// Sending either through the callback seam is wrong, and wrong SILENTLY for
	// the second. Measured, with the callback seam taking every shape:
	//
	//	def acc (flex []) … Parse.matcher g lex 5 ([s:String] => [acc push {v:s} …])
	//	  interpreted  [{v:'hello'}]
	//	  compiled     []                       the matcher never ran
	//
	// and for the delegation wrapper, an error rather than a wrong answer —
	// CallBoru splices the body and re-dispatches its word over a frame whose
	// UNNAMED args sit stack-order, assigning the inner native's sig positions
	// in the OPPOSITE order:
	//
	//	signature_error: cannot call `parse-parser-1` — no signature matches
	//
	// The interpreter takes neither route: execFnDefLiteral dispatches a module
	// wrapper through execMatch on the matched signature (eng/go/CLAUDE.md
	// §Trivial-delegation wrapper short-circuit), which is what the token run
	// was reaching. parseFnNativeApply does the same.
	if res, done, err := parseFnNativeApply(r, fnDef, callArgs); done {
		return parseFnResult(r, res, err)
	}
	// A real boru parser body: the callback seam offers it to its compiled unit
	// before falling back to CallBoru.
	if sig := native.MatchFnSig(fnVal, callArgs); sig != nil {
		res, err := native.InvokeCallbackFn(r, &fnDef, sig, callArgs)
		return parseFnResult(r, res, err)
	}
	// The token run, for what neither lane claims: a signature these two
	// operands cannot fill (the prefix contract admits EXTRA params after
	// [source opts], and MatchFnSig then answers nil), or a delegation wrapper
	// whose inner native does not resolve. Both raise on the sub-engine exactly
	// what the interpreter raises for the same call.
	sub := native.NewTop(r)
	res, err := sub.Run([]native.Value{args[0], args[1], args[2], native.NewEnd()})
	return parseFnResult(r, res, err)
}

// parseLeadDispatchHandler is parselang-lead-dispatch: a leading operand that
// is not a fn at run time re-runs the `parse` word over the same operands
// (source and opts in its surface order), where the interpreter's own
// classification decides; a fn is parseFnDispatchHandler's.
func parseLeadDispatchHandler(args []native.Value, named map[string]native.Value, stack []native.Value, r *native.Registry) ([]native.Value, error) {
	if !args[0].Parent.ConformsTo(native.TFunction) {
		res, err := rerunMacroWord(r, "parse", []native.Value{args[0], args[2], args[1]})
		return parseFnResult(r, res, err)
	}
	return parseFnDispatchHandler(args, named, stack, r)
}

// closureFnView bridges a compiled closure parser to the FnDefInfo the
// interpreter's `parse` validates and dispatches (core.ClosureAsFnDef — valid
// for this one dispatch, under the running VM's invoker). The bridge carries
// the unit's param contract; its RETURN contract is the closure value's own
// (ClosurePayload.RetTypes), and an empty one is an anonymous lambda's
// conservative single `Any` — exactly the Returns the interpreter's lambda
// carries — so ParseLangFnSigWhy asks the bridge what it asks the source fn.
func closureFnView(r *native.Registry, v native.Value) (native.Value, bool) {
	bv, ok := native.ClosureAsFnDef(r, v)
	if !ok {
		return v, false
	}
	fd, isFn := bv.Data.(native.FnDefInfo)
	cl, isCl := v.Data.(core.ClosurePayload)
	if !isFn || !isCl { //covergate:allow the bridge answers ok only for a ClosurePayload operand and always hands back an FnDefInfo value (eng closureFnDef), so an ok bridge makes both assertions hold
		return v, false
	}
	rets := cl.RetTypes
	if len(rets) == 0 {
		rets = []*native.Type{native.TAny}
	}
	sigs := make([]native.Signature, len(fd.Signatures))
	copy(sigs, fd.Signatures)
	for i := range sigs {
		if len(sigs[i].Returns) == 0 {
			sigs[i].Returns = rets
		}
	}
	fd.Signatures = sigs
	bv.Data = fd
	return bv, true
}

// parseFnNativeApply dispatches a parser whose matched overload carries a GO
// handler, the way the interpreter's execFnDefLiteral wrapper branch and the
// VM's own dyn-apply (eng/go/vm.go::tryNativeFnApply) both do — this mirrors
// the latter arm for arm. done is false for a real BORU body (the callback seam
// owns those) and for a value no overload admits (the token run stands).
//
// Name RESOLUTION prefers the fn's OWN registry — a parser minted inside a
// module resolves its native there — and falls back to the value's own
// signatures, which is where a REBOUND wrapper carries them (`def myp
// (Parse.parser g)`: InstallDef's module-wrapper branch binds the inner
// native's sigs verbatim under the new name). The handler then runs against the
// DISPATCHING registry, which is what the interpreter's execMatch passes every
// native handler, so host state — the clock, policy, output — resolves the same
// on every lane.
func parseFnNativeApply(r *native.Registry, fnDef native.FnDefInfo, args []native.Value) ([]native.Value, bool, error) {
	reg, _ := core.FnHome(r, &fnDef)
	sigs := fnDef.Signatures
	if inner := reg.Lookup(fnDef.Name); inner != nil && len(inner.Signatures) > 0 {
		sigs = inner.Signatures
	}
	mr := native.MatchSignature(sigs, args, native.WordInfo{ArgCount: len(args)})
	if !nativeDispatchable(mr) {
		return nil, false, nil
	}
	res, err := mr.Sig.DispatchHandler()(mr.Args, r.Contexts.TopData(), nil, r)
	return res, true, err
}

// nativeDispatchable reports whether a match can run as a DIRECT native
// dispatch: an overload must have matched, and it must carry a Go handler.
//
// A BORU body is declined even when it carries one — a module-preamble fn's
// handler is a synthesized CallBoru, and running it from here would resolve the
// body's free words in the wrong scope. Offering that body to the callback seam
// (which routes through the fn's DEFINING registry) is the whole point of the
// other lane.
func nativeDispatchable(mr *native.MatchResult) bool {
	if mr == nil || mr.Sig == nil || mr.Sig.DispatchHandler() == nil {
		return false
	}
	_, isBoru := mr.Sig.Impl.(*native.BoruImpl)
	return !isBoru
}

// parseFnResult is the single exit every dispatch lane shares: surface a run
// error, else enforce the single-result contract (ParseLangFnSigWhy admits only
// single-return signatures).
func parseFnResult(r *native.Registry, res []native.Value, err error) ([]native.Value, error) {
	if err != nil {
		return nil, err
	}
	// Reachable, and it took splitting the lanes to see how: the TOKEN RUN can
	// leave three values when the parser does not apply at all. ParseLangFnSigWhy
	// admits EXTRA params after the [source opts] prefix, and a sub-engine
	// stepping `fn source opts end` over such a fn collects nothing and leaves
	// the fn and both operands as data. The guard's previous covergate:allow
	// claimed "a conforming run cannot yield ≠1 results" — true of the two
	// dispatch lanes, which match a signature first, and not true of the run
	// that stands when neither claims the call.
	if len(res) != 1 {
		return nil, r.BoruError("internal_error",
			fmt.Sprintf("parse fn: expected one result, got %d", len(res)), "parse")
	}
	return res, nil
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
