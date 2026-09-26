package modules

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	basic "github.com/boru-lang/boru/basic/go"
	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// The boru:minilang module — the MiniLang namespace of embedded
// mini-languages behind the core `mini` macro word. See
// design/MINILANG.5.md.
//
// Every mini-language <name> is exported under the partitioned key
// `lang_<name>` and carries the STANDARD minilang signature:
//
//	MiniLang.lang_<name> : [ src:String opts:Map …inputs ] [ …outputs ]
//
// sig[0] is the minilang source text, sig[1] the named parameters
// (`{}` when the caller gave none), and any further positions are
// kind-declared inputs that a `mini` call site fills from the STACK
// (the expansion's trailing `end` stops forward collection). The core
// `mini` word expands `mini <kind> <src> <opts?>` to
// `MiniLang.lang_<kind> <src> <opts> end`; the desugared call is
// always equivalent.
//
// The kind namespace is FIXED: the built-in kinds below are the whole set,
// and registration was removed (`MiniLang.register` and
// `MiniLang.register-compiled` survive one release as tombstones raising
// mini_registry_frozen). Custom mini-languages are Function VALUES —
// `mini <fn> <src>`, a def-bound name, or a Go-built NewMiniLangFn value —
// which are lexically scoped instead of sharing one flat namespace.
//
// Out-of-band exports (no `lang_` prefix — never reachable via `mini`):
//
//	MiniLang.register           — TOMBSTONE: raises mini_registry_frozen
//	MiniLang.register-compiled  — TOMBSTONE: raises mini_registry_frozen
//	MiniLang.kinds              — list the (fixed) kind atoms

// miniRePatterns memoizes compiled Go regexps keyed by pattern source,
// so a mini call in a loop compiles its pattern once per process.
var (
	miniRePatMu    sync.Mutex
	miniRePatterns = map[string]*regexp.Regexp{}
)

func miniCompiledPattern(src string) (*regexp.Regexp, error) {
	miniRePatMu.Lock()
	defer miniRePatMu.Unlock()
	if re, ok := miniRePatterns[src]; ok {
		return re, nil
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, err
	}
	miniRePatterns[src] = re
	return re, nil
}

// BuildMiniLangModule creates the "boru:minilang" native module.
func BuildMiniLangModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// Compiled carriers escape to the importer (the hook splices them
	// into the parent's tape), so the mint draws its ID from the
	// importing tree's counter.
	subReg.Types.AdoptSeqFrom(parent.Types)
	tMini := subReg.Types.MintType("MiniLangCompiled", native.TIdeal)
	exports := native.NewOrderedMap()
	// Growth ledger (Phase 6 M3): the kind namespace is frozen, so NO
	// program-reachable word installs new keys into this export map at run
	// time (the register words are tombstones). Registering the map with an
	// empty growth set lets the checker fold every missing-key read to a
	// PROVABLY stable None (`MiniLang.Gen` — module-minilang.tsv) instead of
	// the blanket fold decline unregistered maps get. See
	// eng/go/module_export_growth.go.
	core.RegisterModuleExportGrowth(parent, exports)

	// langs collects the FIXED built-in kinds by NAME (a Map) — re, bf, gex,
	// math, hb, bb, micron, m, jp, jq, xp, sp — so no per-kind lang_<name>
	// key is hard-coded at the registration sites. Once every kind is
	// registered its FnDef is published into exports under the partitioned
	// lang_<name> key in ONE place (below), keeping MiniLang.kinds pinned to
	// the insertion order (module-minilang.tsv). The exotic wiring a couple of
	// kinds also need — re's compile hook + run-re consumer, micron's
	// tombstone — stays out-of-band on exports, unchanged.
	langs := native.NewOrderedMap()

	// mintMiniFnType mints the NAMED member type for a filter kind's
	// partially-applied Function and exports it under the capitalized
	// kind name (`MiniLang.Re`, `MiniLang.Gex`, …). The member
	// predicate matches any Function whose FnDefInfo carries the
	// kind's MiniKind tag — Parent stays TFunction, so every existing
	// fn-value code path is untouched, while `is` and typed fn params
	// ([m:Rex] after `def Rex MiniLang.Re`) dispatch on the specific
	// kind. Per-import mint, like MiniLangCompiled above (and the
	// boru:io StreamKind precedent for exported member types). typeof
	// still reports Function — the member type is a constraint, the
	// same convention DepScalar types follow.
	mintMiniFnType := func(kind string) {
		name := strings.ToUpper(kind[:1]) + kind[1:]
		if _, exists := exports.Get(name); exists { //covergate:allow module provably-invariant / grammar-defensive guard (§modules)
			return // defensive: the built-in kind set is name-disjoint
		}
		// The standard member-type path: since ADR-011 collapsed
		// Word/__FN into Type/Function, EVERY fn value (def-bound
		// partials included) conforms to TFunction, so MintMemberType's
		// parent gate admits them and the historical
		// MintTypeWithBehavior workaround is gone.
		t := subReg.Types.MintMemberType(name, native.TFunction,
			func(v native.Value) bool {
				info, ok := v.Data.(native.FnDefInfo)
				return ok && info.MiniKind == kind
			})
		exports.Set(name, native.NewTypeLiteral(t))
	}

	// Wrapper params are UNNAMED — the trivial-delegation short-circuit in
	// execFnDefLiteral requires Body=[Word(inner)] with all-unnamed Params
	// (named params would route through CallBoru name-binding and starve
	// the inner native). See lang/go/CLAUDE.md "Module FnDef Wrappers".
	stdPrefix := []native.FnParam{
		{Type: native.TString}, // src
		{Type: native.TMap},    // opts
	}

	// ---- kind: re — Go regular-expression match -----------------------
	// [src opts subject:String] → [Map] with the match structure
	// {ok ms fst lst n}; each match is {m i e g} (text, byte start/end,
	// capture groups — an unmatched group is none). opts: {limit:I}
	// caps the number of matches (default all).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-re",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TString},
			Returns:    []*native.Type{native.TMap},
			ReturnsFn:  miniReShapeReturns,
			BarrierPos: -1,
			Impl:       native.Go(miniReHandler),
		}},
	})
	langs.Set("re", wrapMiniFnDef("minilang-re", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TString}),
	}, []*native.Type{native.TMap}, nil, subReg))

	// ---- compiled re: the `run-re` consumer + the expansion-time hook -----
	// `run-re` takes the precompiled carrier (sig position 0) + opts + the
	// stack subject; the `re` compile hook (registered on parent below)
	// compiles the pattern at the call site and splices a run-re call.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-run-re",
		Signatures: []native.Signature{{
			Args:       []*native.Type{tMini, native.TMap, native.TString},
			Returns:    []*native.Type{native.TMap},
			ReturnsFn:  miniReShapeReturns,
			BarrierPos: -1,
			Impl:       native.Go(miniRunReHandler),
		}},
	})
	exports.Set("run-re", wrapMiniFnDef("minilang-run-re", [][]native.FnParam{
		{{Type: tMini}, {Type: native.TMap}, {Type: native.TString}},
	}, []*native.Type{native.TMap}, nil, subReg))
	native.RegisterMiniCompileGoHook(parent, "re", miniReCompileFor(tMini))
	// The `re` hook is transducer-faithful: whether the pattern compiles at
	// build time (the hook) or dispatches the standard MiniLang.lang_re call
	// (the transducer), both share miniCompiledPattern + reMatchResult — the
	// same runtime. So a DYNAMIC-src `mini re (pat) {}` records the standard
	// call instead of declining. (`bf` and other plan-baking kinds stay
	// unmarked — their hook semantics can't be reproduced by the transducer.)
	native.MarkMiniCompileHookFaithful(parent, "re")
	mintMiniFnType("re")

	// ---- kind: bf — brainfuck ------------------------------------------
	// Filter form  [src opts input:String] → [String]: the stack value is
	// the `,` input stream. Generator form [src opts] → [String]: input
	// comes from opts.in (default ""). opts: {in:S, steps:I} — steps is
	// the execution budget (default 1e6; exceeding it raises rather than
	// hanging).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-bf",
		Signatures: []native.Signature{
			{
				Args:       []*native.Type{native.TString, native.TMap, native.TString},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Impl:       native.Go(miniBfHandler),
			},
			{
				Args:       []*native.Type{native.TString, native.TMap},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Impl:       native.Go(miniBfHandler),
			},
		},
	})
	langs.Set("bf", wrapMiniFnDef("minilang-bf", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TString}),
		stdPrefix,
	}, []*native.Type{native.TString}, nil, subReg))

	// ---- kind: gex — glob-expression selector -------------------------
	// [src opts subject:Any] → [Any]. gex is a SELECTOR (gex's `.on`), not
	// a match-extractor like `re`: a List subject is filtered to matching
	// elements, a Map to entries whose key matches, and a scalar returns
	// itself when it matches or None otherwise. The pattern is anchored
	// (whole-subject), with `**`/`*?` for a literal `*`/`?`. See gex.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-gex",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TAny},
			ReturnsFn:  miniGexShapeReturns,
			BarrierPos: -1,
			Impl:       native.Go(miniGexHandler),
		}},
	})
	langs.Set("gex", wrapMiniFnDef("minilang-gex", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TAny}, nil, subReg))
	mintMiniFnType("gex")

	// ---- kind: math — traditional maths formula evaluator --------------
	// [src opts] → [Number]. Evaluate a formula like `x*y-z^2` (operators
	// + - * / % ^, unary +/-, parens) whose variables are bound by the
	// named params (opts). Backed by the tabnas/expr Pratt parser; numeric
	// coercion follows boru's integer/float domain rules. See minilang_math.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-math",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TNumber},
			BarrierPos: -1,
			Impl:       native.Go(miniMathHandler),
		}},
	})
	langs.Set("math", wrapMiniFnDef("minilang-math", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TNumber}, nil, subReg))

	// ---- kind: hb — hex Bytes literal ----------------------------------
	// [src opts] → [Bytes]. `+hb/deadbeef/` (≡ mini hb 'deadbeef') decodes
	// an even-length hex string to a Bytes constant. Whitespace and `_` in
	// the source are ignored, so `+hb/de_ad_be_ef/` groups for readability.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-hb",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TBytes},
			BarrierPos: -1,
			Impl:       native.Go(miniHexBytesHandler),
		}},
	})
	langs.Set("hb", wrapMiniFnDef("minilang-hb", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TBytes}, nil, subReg))

	// ---- kind: bb — binary Bytes literal -------------------------------
	// [src opts] → [Bytes]. `+bb/01001100/` decodes a string of 0/1 bits
	// (a multiple of 8, MSB-first per byte) to a Bytes constant. Whitespace
	// and `_` are ignored, so `+bb/01001100_11110000/` groups for clarity.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-bb",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TBytes},
			BarrierPos: -1,
			Impl:       native.Go(miniBinBytesHandler),
		}},
	})
	langs.Set("bb", wrapMiniFnDef("minilang-bb", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TBytes}, nil, subReg))

	// ---- kind: micron (short form: m) — Micron literal ------------------
	// [src opts] → [Micron]. `+m:alice@example.com` (≡ mini m
	// 'alice@example.com') parses the source with the ONE merged tabnas
	// grammar (basic.MicronFromString): each builtin Micron leaf owns a
	// tabnas literal grammar and the (*Tabnas).Merge combination
	// dispatches on shape — Emailon, then Urlon, then Pathon — so the
	// literal returns the appropriate type. Pathon's grammar accepts any
	// whitespace-free source, so it is the catch-all: `+m:a/b` is a
	// Pathon, and a micron literal never fails to parse.
	// URL sources contain `:` and `/`, so pick a delimiter outside the
	// source — `+m|https://x.com/a` — or the first `:`/`/` closes the
	// literal early (the standard closed-form rule).
	// The literal grammar is FIXED to the builtin Micron leaves — the
	// `+m` merge is not extensible (the former MiniLang.micron user-shape
	// hook is a tombstone below).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-micron",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap},
			Returns:    []*native.Type{native.TMicron},
			BarrierPos: -1,
			Impl:       native.Go(miniMicronHandler),
		}},
	})
	micronFnDef := wrapMiniFnDef("minilang-micron", [][]native.FnParam{stdPrefix},
		[]*native.Type{native.TMicron}, nil, subReg)
	langs.Set("micron", micronFnDef)
	langs.Set("m", micronFnDef)

	// ---- out-of-band: micron (TOMBSTONE) --------------------------------
	// The `+m` literal grammar is FIXED to the builtin Micron leaves
	// (Emailon, Urlon, Pathon) — the user-shape hook was removed with the
	// frozen kind namespaces (the un-namespaced literal dispatch is the
	// same collision surface as the kind atoms). The word survives one
	// release as an unconditional, hint-carrying raise; DryPassWrap
	// mirrors it statically so `boru check` flags the use too.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-micron-lit",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(miniMicronLitFrozenHandler),
			ReturnsFn:  native.DryPassWrap(miniMicronLitFrozenHandler, native.ReturnsStatic()),
		}},
	})
	exports.Set("micron", wrapMiniFnDef("minilang-micron-lit", [][]native.FnParam{{}},
		[]*native.Type{}, nil, subReg))

	// ---- kind: jp — JSONPath query (github.com/ohler55/ojg) -------------
	// [src opts doc:Any] → [List]. Run a JSONPath query over the stack
	// subject — a Node (Map/List), Object, Array, Table or Record — and
	// return the matched nodes as a List. See minilang_query.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-jp",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniJsonPathHandler),
		}},
	})
	langs.Set("jp", wrapMiniFnDef("minilang-jp", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TList}, nil, subReg))
	mintMiniFnType("jp")

	// ---- kind: jq — jq filter (github.com/itchyny/gojq) ----------------
	// [src opts doc:Any] → [List]. Run a jq filter over the stack subject
	// (the same data shapes as jp) and return its output stream as a List.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-jq",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TAny},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniJqHandler),
		}},
	})
	langs.Set("jq", wrapMiniFnDef("minilang-jq", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TAny}),
	}, []*native.Type{native.TList}, nil, subReg))
	mintMiniFnType("jq")

	// ---- kind: xp — XPath query (github.com/antchfx/xpath) -------------
	// [src opts doc:Xml] → [List]. Run an XPath expression over the stack
	// Node/Xml subject and return the result as a List — matched nodes for a
	// node-set (an element as its Node/Xml value, an attribute/text node as a
	// String), or a one-element list for a scalar count/string/boolean result.
	// See minilang_xpath.go.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-xp",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString, native.TMap, native.TXml},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniXPathHandler),
		}},
	})
	langs.Set("xp", wrapMiniFnDef("minilang-xp", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TXml}),
	}, []*native.Type{native.TList}, nil, subReg))
	mintMiniFnType("xp")

	// ---- kind: sp — structure-path over boru structure (Map/List) -------
	// [src opts doc:Any] → [List]. Run an XPath expression over the stack
	// Map/List subject and return the result as a List. Same engine as `xp`
	// (github.com/antchfx/xpath), but over boru native structure rather than
	// Node/Xml — the XPath-shaped query layer for boru data. A matched element
	// comes back as its source boru value; a text/scalar result as a
	// String/Number/Boolean. See minilang_sp.go.
	// Two overloads — a Map subject and a List subject — rather than one
	// TAny: the `mini` filter partial (native_macro.go::miniPartialFn) takes
	// its subject param type from the wrapper sig, and a lone TAny param does
	// not auto-apply to a preceding stack value (TAny cannot distinguish "a
	// subject is present" from "none"). Concrete TMap / TList params make the
	// infix-form `doc mini sp '…'` dispatch, like `xp`'s TXml subject.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-sp",
		Signatures: []native.Signature{
			{
				Args:       []*native.Type{native.TString, native.TMap, native.TMap},
				Returns:    []*native.Type{native.TList},
				BarrierPos: -1,
				Impl:       native.Go(miniSpHandler),
			},
			{
				Args:       []*native.Type{native.TString, native.TMap, native.TList},
				Returns:    []*native.Type{native.TList},
				BarrierPos: -1,
				Impl:       native.Go(miniSpHandler),
			},
		},
	})
	langs.Set("sp", wrapMiniFnDef("minilang-sp", [][]native.FnParam{
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TMap}),
		append(append([]native.FnParam{}, stdPrefix...), native.FnParam{Type: native.TList}),
	}, []*native.Type{native.TList}, nil, subReg))
	mintMiniFnType("sp")

	// Publish the built-in kinds under their partitioned lang_<name> keys.
	// They were collected by name in `langs` (a Map), so the lang_ key is
	// formed in exactly ONE place here rather than hard-coded per kind;
	// iterating in insertion order keeps MiniLang.kinds pinned
	// (module-minilang.tsv).
	for _, name := range langs.Keys() {
		def, _ := langs.Get(name)
		exports.Set("lang_"+name, def)
	}

	// ---- out-of-band: register (TOMBSTONE) ------------------------------
	// The mini kind namespace is fixed; registration was removed. The word
	// survives one release as an unconditional, hint-carrying raise so an
	// existing program fails loudly with the migration path instead of a
	// bare missing-export miss. DryPassWrap mirrors the raise statically, so
	// `boru check` flags the use too (the unquote/splice tombstone pattern).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-register",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(miniRegisterFrozenHandler),
			ReturnsFn:  native.DryPassWrap(miniRegisterFrozenHandler, native.ReturnsStatic()),
		}},
	})
	exports.Set("register", wrapMiniFnDef("minilang-register", [][]native.FnParam{{}},
		[]*native.Type{}, nil, subReg))

	// ---- out-of-band: kinds ----------------------------------------------
	// MiniLang.kinds → List of the (fixed) kind atoms (lang_ stripped).
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-kinds",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl:       native.Go(miniKindsHandler(exports)),
		}},
	})
	exports.Set("kinds", wrapMiniFnDef("minilang-kinds", [][]native.FnParam{{}},
		[]*native.Type{native.TList}, nil, subReg))

	// ---- out-of-band: register-compiled (TOMBSTONE) -----------------------
	// The boru compile-hook surface died with the frozen namespace (an
	// expansion-time macro rewrite is unrepresentable as a runtime value);
	// the word survives one release as the same unconditional raise.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "minilang-register-compiled",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(miniRegisterCompiledFrozenHandler),
			ReturnsFn:  native.DryPassWrap(miniRegisterCompiledFrozenHandler, native.ReturnsStatic()),
		}},
	})
	exports.Set("register-compiled", wrapMiniFnDef("minilang-register-compiled", [][]native.FnParam{{}},
		[]*native.Type{}, nil, subReg))

	// ---- out-of-band: fn dispatch (compile-pass seam, NOT exported) ------
	// The runtime twin of `mini <fn> <src> <opts?>` for a leading operand the
	// compile pass could not see concretely (macro_fn_dispatch.go).
	registerMacroFnDispatch(subReg, parent, "minilang-fn-dispatch", miniFnDispatchHandler,
		native.InstallMiniLangFnDispatch, 2, 3)

	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"MiniLang": exports},
	}, nil
}

// MiniLangSpec describes a Go-implemented mini-language for the value
// constructor (NewMiniLangFn). The standard minilang prefix
// [src:String opts:Map] is supplied automatically — a host declares only
// the additional stack Inputs (zero or more, AFTER the prefix), the
// Returns, and the Handler. The handler receives args[0]=src,
// args[1]=opts, args[2..]=Inputs in order, exactly like a built-in kind.
type MiniLangSpec struct {
	// Name is the mini-language's name — used in the value's inner native
	// word and in error messages. Lowercase, like the name it will usually
	// be bound to.
	Name string
	// Inputs are the kind's stack inputs AFTER the [src opts] prefix.
	// Nil/empty for a generator kind (inputs come from opts). Params
	// should be unnamed (only Type / Quote are read) so the wrapper keeps
	// the trivial-delegation dispatch short-circuit. Exactly ONE input
	// makes the value FILTER-shaped: `mini <name> <src>` then expands to a
	// partially-applied Function awaiting the subject.
	Inputs []native.FnParam
	// Returns are the kind's output types (nil = no declared output).
	Returns []*native.Type
	// Handler implements the mini-language at runtime — the immediate
	// transducer `[src opts …inputs] → outputs`.
	Handler native.Handler
}

// wrapMiniFnDef builds the module FnDef wrapper for an inner native —
// the shared makeWrapFnDef trivial-delegation shape, spelled for the
// mini-language builders' multi-overload tables (every overload shares
// one returns tuple and one QuoteArgs map).
func wrapMiniFnDef(wordName string, overloads [][]native.FnParam, returns []*native.Type, quoteArgs map[int]bool, subReg *native.Registry) native.Value {
	sigs := make([]wrapSig, len(overloads))
	for i, params := range overloads {
		sigs[i] = wrapSig{params: params, returns: returns, quoteArgs: quoteArgs}
	}
	return makeWrapFnDef(wordName, subReg, sigs...)
}

// miniOptInt reads an Integer option from an opts map, returning def
// when the key is absent. A present-but-non-integer value is an error.
func miniOptInt(opts native.Value, key string, def int64) (int64, error) {
	m, _ := native.AsMap(opts)
	if m == nil {
		return def, nil
	}
	v, ok := m.Get(key)
	if !ok {
		return def, nil
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, fmt.Errorf("opts.%s: %w", key, err)
	}
	return n, nil
}

// miniOptString reads a String option, "" default.
func miniOptString(opts native.Value, key string) (string, error) {
	m, _ := native.AsMap(opts)
	if m == nil {
		return "", nil
	}
	v, ok := m.Get(key)
	if !ok {
		return "", nil
	}
	s, err := v.AsConcreteString()
	if err != nil {
		return "", fmt.Errorf("opts.%s: %w", key, err)
	}
	return s, nil
}

// miniDropGrouping strips ASCII whitespace and underscores so a binary/hex
// source can be grouped for readability (`+hb/de_ad_be_ef/`, `+bb/0100_1100/`).
func miniDropGrouping(s string) string {
	return strings.Map(func(c rune) rune {
		switch c {
		case ' ', '\t', '\n', '\r', '_':
			return -1
		}
		return c
	}, s)
}

// miniMicronLitFrozenHandler is MiniLang.micron's TOMBSTONE: the `+m`
// literal grammar is FIXED to the builtin Micron leaves (Emailon, Urlon,
// Pathon), so the user-shape hook was removed with the frozen kind
// namespaces. An unconditional, hint-carrying raise — the DryPassWrap
// mirror on its signature surfaces the same finding statically.
func miniMicronLitFrozenHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	return nil, r.BoruErrorHint("mini_registry_frozen",
		"micron: the +m literal grammar is fixed to the builtin Micron leaves — the user-shape hook was removed", "micron",
		"parse a custom Micron source with a parser fn value and construct the instance with make (def Nameon refine Micron {…} still works; only the literal sugar is builtin-only)")
}

// miniMicronHandler is the micron/m transducer — args[0]=src, args[1]=opts
// (none defined). It parses with the ONE builtin merged grammar
// (basic.MicronFromString): the leaf set is fixed — Emailon, then Urlon,
// then the Pathon catch-all — so there is no per-registry state and no
// merge rebuild.
func miniMicronHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_parse_error", fmt.Sprintf("micron: src: %v", err), "lang_micron")
	}
	v, merr := basic.MicronFromString(src)
	if merr != nil {
		return nil, r.BoruError("mini_parse_error", fmt.Sprintf("micron: %v", merr), "lang_micron")
	}
	return []native.Value{v}, nil
}

// miniHexBytesHandler — args[0]=src, args[1]=opts. Decodes an even-length
// hex string to Bytes. The value is built via core.FromNative (the
// []byte→Bytes bridge), which runs here at runtime where the bridge is live.
func miniHexBytesHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_parse_error", fmt.Sprintf("hb: src: %v", err), "lang_hb")
	}
	b, derr := hex.DecodeString(miniDropGrouping(src))
	if derr != nil {
		return nil, r.BoruErrorHint("mini_parse_error", fmt.Sprintf("hb: %v", derr), "lang_hb",
			"use an even number of hex digits, e.g. +hb/deadbeef/")
	}
	return []native.Value{core.FromNative(b)}, nil
}

// miniBinBytesHandler — args[0]=src, args[1]=opts. Decodes a string of 0/1
// bits (a multiple of 8, MSB-first per byte) to Bytes.
func miniBinBytesHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_parse_error", fmt.Sprintf("bb: src: %v", err), "lang_bb")
	}
	bits := miniDropGrouping(src)
	if len(bits)%8 != 0 {
		return nil, r.BoruErrorHint("mini_parse_error",
			fmt.Sprintf("bb: bit count %d is not a multiple of 8", len(bits)), "lang_bb",
			"pad to whole bytes, e.g. +bb/01001100/")
	}
	out := make([]byte, len(bits)/8)
	for i := range out {
		var v byte
		for j := 0; j < 8; j++ {
			v <<= 1
			switch bits[i*8+j] {
			case '1':
				v |= 1
			case '0':
			default:
				return nil, r.BoruErrorHint("mini_parse_error",
					fmt.Sprintf("bb: %q is not a binary digit", string(bits[i*8+j])), "lang_bb",
					"use only 0 and 1, e.g. +bb/01001100/")
			}
		}
		out[i] = v
	}
	return []native.Value{core.FromNative(out)}, nil
}

// miniReHandler — args[0]=src, args[1]=opts, args[2]=subject.
func miniReHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("re: src: %v", err), "lang_re")
	}
	subject, err := args[2].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("re: subject: %v", err), "lang_re")
	}
	limit, err := miniOptInt(args[1], "limit", -1)
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("re: %v", err), "lang_re")
	}
	re, cerr := miniCompiledPattern(src)
	if cerr != nil {
		return nil, r.BoruErrorHint("mini_parse_error",
			fmt.Sprintf("re: %v", cerr), "lang_re",
			"fix the pattern — note backslashes in '…'/\"…\" strings need doubling ('\\\\d'); backtick strings are backslash-safe")
	}
	return []native.Value{reMatchResult(re, subject, limit)}, nil
}

// miniReShapeReturns is the check-mode ReturnsFn for the `re` kind (and
// its compiled `run-re` consumer): the handler ALWAYS builds the standard
// match structure {ok:Boolean ms:List fst:<match> lst:<match> n:Integer},
// with each match shaped {m:String i:Integer e:Integer g:List} — see
// reMatchResult. Surfacing that shape as a record-schema carrier lets a
// field read (`r.n`, `r.ok`, `r.fst.m`) narrow via getNodeReturns instead
// of ending dynamic(Any). GRADUAL by construction: every carrier is
// dynamic, so a run-time absence (fst/lst are unset when nothing matched)
// is discharged by a guard, never a committed strict read.
func miniReShapeReturns(_ []native.Value, _ *native.Registry) []native.Value {
	match := func() native.Value {
		m := native.NewOrderedMap()
		m.Set("m", native.NewTypeLiteral(native.TString))
		m.Set("i", native.NewTypeLiteral(native.TInteger))
		m.Set("e", native.NewTypeLiteral(native.TInteger))
		m.Set("g", native.NewTypeLiteral(native.TList))
		return native.NewDynamicCarrierValue(native.NewRecordType(m))
	}
	fields := native.NewOrderedMap()
	fields.Set("ok", native.NewTypeLiteral(native.TBoolean))
	fields.Set("ms", native.NewTypeLiteral(native.TList))
	fields.Set("fst", match())
	fields.Set("lst", match())
	fields.Set("n", native.NewTypeLiteral(native.TInteger))
	return []native.Value{native.NewDynamicCarrierValue(native.NewRecordType(fields))}
}

// miniGexShapeReturns is the check-mode ReturnsFn for the `gex` kind:
// the handler is subject-shape-driven (see miniGexHandler) — a List
// subject filters to a List, a Map filters to a Map, and a scalar
// returns ITSELF when it matches or None otherwise. Only what the
// handler guarantees is declared: a statically-List/Map subject yields
// a dynamic carrier of that container type; a scalar-typed subject
// yields dynamic(<subject-type> tor None); anything unknown (dynamic /
// Any-bounded) stays dynamic(Any).
func miniGexShapeReturns(args []native.Value, _ *native.Registry) []native.Value {
	dyn := []native.Value{native.NewDynamicCarrier(native.TAny)}
	if len(args) != 3 {
		return dyn
	}
	subject := args[2]
	st := native.ValueType(subject)
	if st == nil || subject.Dynamic || st.Equal(native.TAny) {
		return dyn
	}
	switch {
	case st.ConformsTo(native.TList):
		return []native.Value{native.NewDynamicCarrier(native.TList)}
	case st.ConformsTo(native.TMap):
		return []native.Value{native.NewDynamicCarrier(native.TMap)}
	case st.ConformsTo(native.TScalar):
		return []native.Value{native.NewDynamicCarrierValue(native.NewDisjunct([]native.Value{
			native.NewTypeLiteral(st), native.NewTypeLiteral(native.TNone),
		}))}
	default:
		return dyn
	}
}

// runeOffsetScanner converts ascending BYTE offsets in a subject to RUNE
// indices in one incremental forward scan (O(len subject) total). Top-level
// regexp match spans arrive ordered and non-overlapping, so each converted
// offset is >= its predecessor and the scan never restarts; a zero-width
// match (i == e, or a match starting where the previous one ended) repeats
// an offset, which advances nothing and returns the same rune index.
type runeOffsetScanner struct {
	subject string
	bytePos int
	runePos int
}

func (sc *runeOffsetScanner) runeIdx(byteOff int) int {
	for sc.bytePos < byteOff {
		_, size := utf8.DecodeRuneInString(sc.subject[sc.bytePos:])
		sc.bytePos += size
		sc.runePos++
	}
	return sc.runePos
}

// reMatchResult builds the standard re match structure {ok ms fst lst n} —
// shared by the runtime kind (lang_re, compiles per call via the memo) and the
// compiled consumer (run-re, gets a precompiled regexp from the carrier).
// Go's regexp reports BYTE offsets; each match's i/e are converted to RUNE
// indices (the unit every boru index means — slice/size compose directly,
// NUR047) via one incremental runeOffsetScanner pass over the subject.
func reMatchResult(re *regexp.Regexp, subject string, limit int64) native.Value {
	idxs := re.FindAllStringSubmatchIndex(subject, int(limit))
	matches := make([]native.Value, 0, len(idxs))
	sc := &runeOffsetScanner{subject: subject}
	for _, ix := range idxs {
		mm := native.NewOrderedMap()
		mm.Set("m", native.NewString(subject[ix[0]:ix[1]]))
		mm.Set("i", native.NewInteger(int64(sc.runeIdx(ix[0]))))
		mm.Set("e", native.NewInteger(int64(sc.runeIdx(ix[1]))))
		groups := make([]native.Value, 0, len(ix)/2-1)
		for g := 1; g < len(ix)/2; g++ {
			s, e := ix[2*g], ix[2*g+1]
			if s < 0 {
				groups = append(groups, native.NewNone())
			} else {
				groups = append(groups, native.NewString(subject[s:e]))
			}
		}
		mm.Set("g", native.NewList(groups))
		matches = append(matches, native.NewMap(mm))
	}

	out := native.NewOrderedMap()
	out.Set("ok", native.NewBoolean(len(matches) > 0))
	out.Set("ms", native.NewList(matches))
	if len(matches) > 0 {
		out.Set("fst", matches[0])
		out.Set("lst", matches[len(matches)-1])
	}
	out.Set("n", native.NewInteger(int64(len(matches))))
	return native.NewMap(out)
}

// ---- compiled re: the carrier type + the `run-re` consumer + the hook ----

// The MiniLangCompiled carrier type — the inert value a compile hook
// splices in place of the DSL source, wrapping the kind's precompiled
// artifact (for `re`, a *regexp.Regexp) in an ExtensionPayload the
// kernel never inspects — is a per-import module mint (former global
// FixedID 5003, retired): BuildMiniLangModule mints it and threads it
// to the run-re consumer and the `re` compile hook. See
// MintTemporalModuleTypes / MintTensorTypes for the pattern.

// miniRunReHandler — args[0]=compiled carrier, args[1]=opts, args[2]=subject.
// The compiled consumer for `re`: the pattern was compiled at the call site by
// miniReCompile, so this just matches.
func miniRunReHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ep, ok := args[0].Data.(core.ExtensionPayload)
	if !ok {
		return nil, r.BoruError("mini_error", "run-re: not a compiled pattern", "run-re")
	}
	re, ok := ep.Body.(*regexp.Regexp)
	if !ok {
		return nil, r.BoruError("mini_error", "run-re: not a compiled pattern", "run-re")
	}
	subject, err := args[2].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("re: subject: %v", err), "run-re")
	}
	limit, err := miniOptInt(args[1], "limit", -1)
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("re: %v", err), "run-re")
	}
	return []native.Value{reMatchResult(re, subject, limit)}, nil
}

// miniReCompile is the `re` compile hook: it compiles the pattern at the call
// site and splices `MiniLang get run-re <compiled> <opts> end` — the inert
// precompiled carrier plus the consumer word, so the per-call cost is just the
// match (the compile is memoized by miniCompiledPattern).
//
// On a MALFORMED pattern it defers to the standard `lang_re` call rather than
// raising here: the carrier can't be built, and deferring keeps the error
// byte-identical to the transducer — which is what compiled mode runs (the
// bytecode recorder, in check mode, never takes the compile-hook path), so
// compiled/interpreted parity holds. Valid patterns still get the carrier.
func miniReCompileFor(tMini *native.Type) func(string, native.Value, *native.Registry) ([]native.Value, error) {
	return func(src string, opts native.Value, _ *native.Registry) ([]native.Value, error) {
		re, cerr := miniCompiledPattern(src)
		if cerr != nil {
			return []native.Value{
				native.NewWord("MiniLang"), native.NewWord("dot"), native.NewWord("lang_re"),
				native.NewString(src), opts, native.NewEnd(),
			}, nil
		}
		carrier := core.NewExtension(tMini, re)
		return []native.Value{
			native.NewWord("MiniLang"), native.NewWord("dot"), native.NewWord("run-re"),
			carrier, opts, native.NewEnd(),
		}, nil
	}
}

// miniBfDefaultSteps is the default brainfuck execution budget — loud
// failure instead of a hang on a runaway program.
const miniBfDefaultSteps = 1_000_000

// miniBfHandler — args[0]=src, args[1]=opts, optional args[2]=input.
// Input precedence: stack input (3-arg form) > opts.in > "".
func miniBfHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("bf: src: %v", err), "lang_bf")
	}
	input := ""
	if len(args) == 3 {
		input, err = args[2].AsConcreteString()
		if err != nil {
			return nil, r.BoruError("mini_error", fmt.Sprintf("bf: input: %v", err), "lang_bf")
		}
	} else {
		input, err = miniOptString(args[1], "in")
		if err != nil {
			return nil, r.BoruError("mini_error", fmt.Sprintf("bf: %v", err), "lang_bf")
		}
	}
	steps, err := miniOptInt(args[1], "steps", miniBfDefaultSteps)
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("bf: %v", err), "lang_bf")
	}

	out, bferr := runBrainfuck(src, input, steps)
	if bferr != nil {
		code := "mini_eval_error"
		hint := "the program exceeded its limits — raise opts {steps:N} or fix the loop"
		if bferr.parse {
			code = "mini_parse_error"
			hint = "balance the [ ] brackets"
		}
		return nil, r.BoruErrorHint(code, fmt.Sprintf("bf: %s", bferr.msg), "lang_bf", hint)
	}
	return []native.Value{native.NewString(out)}, nil
}

type bfError struct {
	msg   string
	parse bool
}

// runBrainfuck interprets the eight-command language over a 30000-cell
// byte tape (cells wrap mod 256; the pointer does NOT wrap — leaving the
// tape is an error). `,` reads from input, yielding 0 at EOF. maxSteps
// bounds executed instructions.
func runBrainfuck(prog, input string, maxSteps int64) (string, *bfError) {
	// Precompute the bracket jump table.
	jump := make(map[int]int)
	var stack []int
	for pc := 0; pc < len(prog); pc++ {
		switch prog[pc] {
		case '[':
			stack = append(stack, pc)
		case ']':
			if len(stack) == 0 {
				return "", &bfError{msg: fmt.Sprintf("unbalanced ']' at offset %d", pc), parse: true}
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			jump[open] = pc
			jump[pc] = open
		}
	}
	if len(stack) > 0 {
		return "", &bfError{msg: fmt.Sprintf("unbalanced '[' at offset %d", stack[len(stack)-1]), parse: true}
	}

	cells := make([]byte, 30000)
	ptr, in := 0, 0
	var out strings.Builder
	var steps int64
	for pc := 0; pc < len(prog); pc++ {
		steps++
		if steps > maxSteps {
			return "", &bfError{msg: fmt.Sprintf("step budget %d exceeded", maxSteps)}
		}
		switch prog[pc] {
		case '+':
			cells[ptr]++
		case '-':
			cells[ptr]--
		case '>':
			ptr++
			if ptr >= len(cells) {
				return "", &bfError{msg: fmt.Sprintf("pointer out of range at offset %d", pc)}
			}
		case '<':
			ptr--
			if ptr < 0 {
				return "", &bfError{msg: fmt.Sprintf("pointer out of range at offset %d", pc)}
			}
		case '.':
			out.WriteByte(cells[ptr])
		case ',':
			if in < len(input) {
				cells[ptr] = input[in]
				in++
			} else {
				cells[ptr] = 0
			}
		case '[':
			if cells[ptr] == 0 {
				pc = jump[pc]
			}
		case ']':
			if cells[ptr] != 0 {
				pc = jump[pc]
			}
		}
	}
	return out.String(), nil
}

// miniRegisterFrozenHandler is MiniLang.register's TOMBSTONE: the mini kind
// namespace is FIXED (the built-in kinds are the whole set), so registration
// was removed. An unconditional, hint-carrying raise — the DryPassWrap
// mirror on its signature surfaces the same finding statically.
func miniRegisterFrozenHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	return nil, r.BoruErrorHint("mini_registry_frozen",
		"register: the mini kind namespace is fixed — registration was removed", "register",
		"pass the mini-language as a Function value instead: def myl (fn [[src:String opts:Map] [Any] [...]])  mini myl '...' — Go hosts build one with NewMiniLangFn")
}

// miniRegisterCompiledFrozenHandler is MiniLang.register-compiled's
// TOMBSTONE: the boru compile-hook surface died with the frozen namespace —
// an expansion-time macro rewrite is unrepresentable as a runtime value, so
// there is no value-form successor. The raise carries the closest migration
// path (a plain fn value; the per-call compile becomes the fn's own work).
func miniRegisterCompiledFrozenHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	return nil, r.BoruErrorHint("mini_registry_frozen",
		"register-compiled: the mini kind namespace is fixed — compile hooks were removed with registration", "register-compiled",
		"pass the mini-language as a Function value instead (mini <fn> '...') and memoize any expensive compile inside the fn")
}

// miniKindsHandler lists the (fixed) kind atoms (lang_ stripped), in
// registration order.
func miniKindsHandler(exports *native.OrderedMap) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		var kinds []native.Value
		for _, k := range exports.Keys() {
			if strings.HasPrefix(k, "lang_") {
				kinds = append(kinds, native.NewAtom(strings.TrimPrefix(k, "lang_")))
			}
		}
		return []native.Value{native.NewList(kinds)}, nil
	}
}
