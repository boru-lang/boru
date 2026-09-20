package modules

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/native"
	tabnasabnf "github.com/tabnas/abnf/go"
	tabnas "github.com/tabnas/parser/go"
)

// The boru:parse module — the Parse namespace, a builder DSL for defining a
// CUSTOM parser and exposing it as a new `parse <name>` kind. Where
// boru:parselang ships built-in decoders (json, csv, yaml, …) and lets a boru
// fn register as a parser, boru:parse lets a user CONSTRUCT a parser from
// grammar: an ABNF grammar, a declarative rule map ("grammar as Map
// subtypes"), custom lex matchers, and semantic actions that build custom
// data types. It is built on github.com/tabnas/parser/go (the engine under
// the jsonic layer) and github.com/tabnas/abnf/go (ABNF → grammar).
//
// The module is BUILD-ONLY: there is no standalone run word. Building a
// grammar culminates in `Parse.parser <grammar>`, which finalizes it into a
// ParseLang Function VALUE (the boru:parselang framework shell around the
// grammar's parse handler). The user binds or passes the value and runs it
// with the ordinary `parse` macro's value form:
//
//	import "boru:parse"
//	import "boru:parselang"
//	def g Parse.grammar                              # mint a builder, bind it
//	Parse.action g '@op:o:INC' ([nd:Any] => [1])     # custom data via a mark
//	Parse.abnf g "op = \"inc\" / \"dec\"" {start:'op'}
//	def op (Parse.parser g)                          # → a ParseLang value
//	parse op 'inc'                                   # → 1
//
// The Grammar carrier is a shared pointer (mutated in place, like Store /
// Array), so it is bound to a `def` and threaded through each builder word as
// the first argument; the builder words return nothing. Building is deferred:
// tokens, declarative rules and ABNF installs are replayed at Parse.parser
// (when every mark action is known), and custom matchers are applied last.

// parseGrammar is the host-backed builder a Parse.grammar call mints. It
// carries the tabnas engine under construction plus the boru-fn-backed
// callbacks (mark actions) and the deferred ABNF installs. Pointer-backed so
// the chainable words mutate one instance.
type parseGrammar struct {
	j           *tabnas.Tabnas        // the engine under construction
	markActions tabnasabnf.ActionsMap // ABNF mark/phase ref -> boru-fn-backed AltActions
	inlineSeq   int                   // counter for generated declarative action refs

	// steps are the deferred grammar-building operations (tokens, declarative
	// rules, ABNF installs) in call order. They run at Parse.parser, when
	// every mark action is known. matchers are applied LAST, after every
	// grammar step, so an ABNF install (which can rebuild the lexer) cannot
	// drop a custom lex matcher.
	steps    []func() error
	matchers []pendingMatcher

	// registered marks the builder as consumed by Parse.parser. The carrier
	// is a shared pointer and the registered parser handler closes over g/g.j,
	// so the builder is SINGLE-USE: any further builder word or a second
	// register on a consumed grammar raises rather than mutating a finalized
	// parser. Build a fresh Parse.grammar for a second parser.
	registered bool

	// firstErr captures the first error raised by a boru-fn callback (a
	// matcher or action) during a parse; the parser handler surfaces it as a
	// parse error. Callbacks never panic (ADR-005) — they record here instead.
	firstErr error
	// r is the active registry for the current parse, set by the parser
	// handler for the duration of one j.Parse call so callbacks can dispatch
	// boru fns re-entrantly. Single-threaded parse → safe; nil-guarded.
	r *native.Registry
}

type pendingMatcher struct {
	name string
	prio int
	fn   native.Value
}

// setErr records the first callback error (first wins).
func (g *parseGrammar) setErr(err error) {
	if g.firstErr == nil {
		g.firstErr = err
	}
}

// ensureOpen errors when the builder has already been registered (single-use).
func (g *parseGrammar) ensureOpen(word string, r *native.Registry) error {
	if g.registered {
		return r.BoruErrorHint("parse_grammar_done",
			word+": this grammar was already finalized into a parser", word,
			"a Parse.grammar builder is single-use — build a fresh one for another parser")
	}
	return nil
}

// applyMatchers installs the pending custom lex matchers onto g.j and re-sorts
// Config.CustomMatchers by priority. The tabnas lexer assumes the slice is
// priority-sorted as it interleaves custom matchers with the built-in
// match/fixed/text bands, so an out-of-order matcher (a lower priority added
// after a higher one) would otherwise run too late to claim its tokens — the
// same reason eng/go/parser/grammar.go::addMatcher sorts after appending.
func (g *parseGrammar) applyMatchers() {
	cfg := g.j.Config()
	for _, mt := range g.matchers {
		cfg.CustomMatchers = append(cfg.CustomMatchers, &tabnas.MatcherEntry{
			Name:     mt.name,
			Priority: mt.prio,
			Match:    g.wrapMatcher(mt.fn),
		})
	}
	sort.SliceStable(cfg.CustomMatchers, func(i, k int) bool {
		return cfg.CustomMatchers[i].Priority < cfg.CustomMatchers[k].Priority
	})
}

// composeAltActions collapses a list of mark actions into one AltAction (run
// in order); a single action passes through unchanged.
func composeAltActions(acts []tabnasabnf.ActionFn) tabnas.AltAction {
	if len(acts) == 1 {
		return acts[0]
	}
	return func(rule *tabnas.Rule, ctx *tabnas.Context) {
		for _, a := range acts {
			a(rule, ctx)
		}
	}
}

// The ParseGrammar carrier type — wrapping the *tabnas.Tabnas engine +
// callbacks in an ExtensionPayload the kernel never inspects — is a
// per-import module mint (former global FixedID 5005, retired):
// BuildParseModule mints it into the sub-registry and threads it to
// the grammar constructor. See MintTemporalModuleTypes /
// MintTensorTypes for the pattern.

// asParseGrammar unwraps a Grammar carrier, or errors with a clear message.
func asParseGrammar(v native.Value, word string, r *native.Registry) (*parseGrammar, error) {
	ep, ok := v.Data.(core.ExtensionPayload)
	if ok {
		if g, ok := ep.Body.(*parseGrammar); ok {
			return g, nil
		}
	}
	return nil, r.BoruErrorHint("parse_bad_grammar",
		word+": expected a Parse grammar (from Parse.grammar)", word,
		"build one with `Parse.grammar` first, then chain Parse.abnf / Parse.rule / …")
}

// BuildParseModule creates the "boru:parse" native module.
func BuildParseModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()

	// Grammar values escape to the importer, so the mint draws its ID
	// from the importing tree's counter.
	subReg.Types.AdoptSeqFrom(parent.Types)
	gT := subReg.Types.MintType("ParseGrammar", native.TIdeal)

	// ---- grammar — mint a fresh builder --------------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-grammar",
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{gT},
			BarrierPos: -1,
			Impl:       native.Go(parseGrammarHandlerFor(gT)),
		}},
	})
	exports.Set("grammar", wrapMiniFnDef("parse-grammar", [][]native.FnParam{{}},
		[]*native.Type{gT}, nil, subReg))

	// ---- abnf — install an ABNF grammar (deferred to the finalizer) ----
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-abnf",
		Signatures: []native.Signature{
			{
				Args:       []*native.Type{gT, native.TString, native.TMap},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Impl:       native.Go(parseAbnfHandler),
			},
			{
				Args:       []*native.Type{gT, native.TString},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Impl:       native.Go(parseAbnfHandler),
			},
		},
	})
	exports.Set("abnf", wrapMiniFnDef("parse-abnf", [][]native.FnParam{
		{{Type: gT}, {Type: native.TString}, {Type: native.TMap}},
		{{Type: gT}, {Type: native.TString}},
	}, []*native.Type{}, nil, subReg))

	// ---- rule — declarative grammar rule (the Map-subtype form) --------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-rule",
		Signatures: []native.Signature{{
			Args:       []*native.Type{gT, native.TAtom, native.TMap},
			QuoteArgs:  map[int]bool{1: true},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(parseRuleHandler),
		}},
	})
	exports.Set("rule", wrapMiniFnDef("parse-rule", [][]native.FnParam{
		{{Type: gT}, {Type: native.TAtom, Quote: true}, {Type: native.TMap}},
	}, []*native.Type{}, map[int]bool{1: true}, subReg))

	// ---- spec — the WHOLE grammar as one declarative map ----------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-spec",
		Signatures: []native.Signature{{
			Args:          []*native.Type{gT, native.TMap},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // action/matcher fns in the map are stored, not invoked
			Impl:          native.Go(parseSpecHandler),
			// Check-mode hook: the map's section/shape rules are
			// map-decidable, so a dry pass validates a concrete spec map
			// during analysis (parseSpecReturns).
			ReturnsFn: parseSpecReturns,
		}},
	})
	exports.Set("spec", wrapMiniFnDef("parse-spec", [][]native.FnParam{
		{{Type: gT}, {Type: native.TMap}},
	}, []*native.Type{}, nil, subReg))

	// ---- token — register a fixed lexer token --------------------------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-token",
		Signatures: []native.Signature{{
			// name is a token identifier like "#PL" (special chars) → a String.
			Args:       []*native.Type{gT, native.TString, native.TString},
			Returns:    []*native.Type{},
			BarrierPos: -1,
			Impl:       native.Go(parseTokenHandler),
		}},
	})
	exports.Set("token", wrapMiniFnDef("parse-token", [][]native.FnParam{
		{{Type: gT}, {Type: native.TString}, {Type: native.TString}},
	}, []*native.Type{}, nil, subReg))

	// ---- matcher — register a boru-fn-backed custom lex matcher --------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-matcher",
		Signatures: []native.Signature{{
			Args:          []*native.Type{gT, native.TAtom, native.TInteger, native.TFunction},
			QuoteArgs:     map[int]bool{1: true},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // the fn is stored, not invoked on the tape
			Impl:          native.Go(parseMatcherHandler),
		}},
	})
	exports.Set("matcher", wrapMiniFnDef("parse-matcher", [][]native.FnParam{
		{{Type: gT}, {Type: native.TAtom, Quote: true}, {Type: native.TInteger}, {Type: native.TFunction}},
	}, []*native.Type{}, map[int]bool{1: true}, subReg))

	// ---- action — attach a boru-fn-backed semantic action (mark) -------
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-action",
		Signatures: []native.Signature{{
			Args:          []*native.Type{gT, native.TString, native.TFunction},
			Returns:       []*native.Type{},
			BarrierPos:    -1,
			CompileEffect: native.CompileStoresFn, // the fn is stored, not invoked on the tape
			Impl:          native.Go(parseActionHandler),
		}},
	})
	exports.Set("action", wrapMiniFnDef("parse-action", [][]native.FnParam{
		{{Type: gT}, {Type: native.TString}, {Type: native.TFunction}},
	}, []*native.Type{}, nil, subReg))

	// ---- parser — finalize into a ParseLang Function VALUE --------------
	// Parse.parser <grammar> → Function. The fn-returning finalizer: it
	// replays the deferred grammar steps and yields a ParseLang value — a
	// fn carrying the standard [source opts] prefix — for the caller to
	// bind (`def calc (Parse.parser g)  parse calc 'src'`) or pass directly
	// (`parse (Parse.parser g) 'src'`). The parse kind namespace is FIXED
	// (built-in kinds only), so a grammar parser is never installed under a
	// kind name; the value form is its whole surface, and the compiled path
	// dispatches it through the recorded parselang-fn-dispatch — the fn
	// operand's producing event (this call) is the provenance.
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "parse-parser",
		Signatures: []native.Signature{{
			Args:       []*native.Type{gT},
			Returns:    []*native.Type{native.TFunction},
			BarrierPos: -1,
			Impl:       native.Go(parseParserHandlerFor(subReg)),
			// Check-mode: the grammar carrier is never concrete under
			// analysis, so the parser is always a dynamic Function — the
			// `parse` macro's carrier branch handles the dispatch.
			ReturnsFn: parseParserReturns,
		}},
	})
	exports.Set("parser", wrapMiniFnDef("parse-parser", [][]native.FnParam{
		{{Type: gT}},
	}, []*native.Type{native.TFunction}, nil, subReg))

	// ---- type exports: the grammar-as-Map-subtypes --------------------
	// Parse.RuleSpec / Parse.AltSpec are Map-subtype (Options) type literals
	// mirroring the tabnas GrammarRuleSpec / GrammarAltSpec shape, so a
	// declarative grammar's structure is documented and inspectable as a
	// proper type (e.g. `Parse.AltSpec` renders `options{s:String …}`). The
	// `Parse.rule` converter accepts a plain map of these fields; the types
	// name the schema. Every field is optional.
	exports.Set("AltSpec", parseAltSpecType())
	exports.Set("RuleSpec", parseRuleSpecType())

	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{"Parse": exports},
	}, nil
}

// parseAltSpecType is the Options type literal for a single grammar
// alternate — the declarative GrammarAltSpec fields a user may set. Returned
// as a structural type-literal Value (a Map subtype), exported as
// Parse.AltSpec.
func parseAltSpecType() native.Value {
	f := native.NewOrderedMap()
	f.Set("s", native.NewTypeLiteral(native.TString)) // token spec, e.g. "#NR #CL"
	f.Set("p", native.NewTypeLiteral(native.TString)) // push rule
	f.Set("r", native.NewTypeLiteral(native.TString)) // replace rule
	f.Set("b", native.NewTypeLiteral(native.TAny))    // backtrack (Integer or rule ref)
	f.Set("a", native.NewTypeLiteral(native.TAny))    // action: a Function, a "@ref" String, or a list of either
	f.Set("g", native.NewTypeLiteral(native.TString)) // group tags
	f.Set("n", native.NewTypeLiteral(native.TMap))    // counter increments
	f.Set("c", native.NewTypeLiteral(native.TMap))    // declarative condition ({'n.counter':n} → $eq)
	f.Set("u", native.NewTypeLiteral(native.TMap))    // custom props
	f.Set("k", native.NewTypeLiteral(native.TMap))    // propagated custom props
	return native.NewOptionsType(f)
}

// parseRuleSpecType is the Options type literal for a rule — its open / close
// alternate lists (each element an AltSpec map). Exported as Parse.RuleSpec.
func parseRuleSpecType() native.Value {
	f := native.NewOrderedMap()
	f.Set("open", native.NewTypeLiteral(native.TList))
	f.Set("close", native.NewTypeLiteral(native.TList))
	return native.NewOptionsType(f)
}

// ---- handlers --------------------------------------------------------

func parseGrammarHandlerFor(gT *native.Type) native.Handler {
	return func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		g := &parseGrammar{
			j:           tabnas.Make(), // bare engine — the user's grammar is the whole grammar
			markActions: tabnasabnf.ActionsMap{},
		}
		return []native.Value{core.NewExtension(gT, g)}, nil
	}
}

// parseAbnfHandler — args[0]=grammar, args[1]=src, args[2]=opts (optional).
func parseAbnfHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.abnf", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.abnf", r); err != nil {
		return nil, err
	}
	src, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("parse_bad_abnf", fmt.Sprintf("Parse.abnf: src: %v", err), "Parse.abnf")
	}
	var opts native.Value
	if len(args) > 2 {
		opts = args[2]
	}
	g.addAbnfStep(src, abnfOptsFrom(opts))
	return nil, nil
}

// addAbnfStep defers an ABNF install to Parse.parser (when every mark
// action is known). Shared by Parse.abnf and the abnf section of
// Parse.spec.
func (g *parseGrammar) addAbnfStep(src string, o *tabnasabnf.AbnfConvertOptions) {
	g.steps = append(g.steps, func() error {
		var acts tabnasabnf.ActionsMap
		if len(g.markActions) > 0 {
			acts = g.markActions
		}
		if _, ierr := tabnasabnf.Install(g.j, src, o, acts); ierr != nil {
			return fmt.Errorf("abnf: %s", native.FirstCleanLine(ierr.Error()))
		}
		return nil
	})
}

// parseSpecHandler — args[0]=grammar, args[1]=the WHOLE grammar as one
// declarative map, mirroring the tabnas GrammarSpec document shape
// (grammarspec.go: {options, rule, ref, v}) plus the two boru-side
// extension sections tabnas has no JSON home for:
//
//	Parse.spec g {
//	  options: {fixed:{token:{'#PL':'+'}} rule:{start:'op'} …}  # tabnas OptionsMap (MapToOptions)
//	  ref:     {'@op:o:INC': (…)}                # name → fn, or a LIST of fns (run in order)
//	  rule:    {extra:{open:[{s:'#PL'}]}}        # name → GrammarRuleSpec map (Parse.rule shape)
//	  v:       1                                 # optional builtin config-schema version gate
//	  abnf:    {src:'op = "inc"' start:'op'}     # EXTENSION: String | {src:… +abnf opts} | a LIST of either
//	  matcher: {skip:{priority:1000000 fn:(…)}}  # EXTENSION: name → {priority fn} custom lex matchers
//	}
//
// Every section is optional; an unknown section key is a loud error.
// Sections apply in tabnas' order — options (with v) first, then refs,
// rules, ABNF — regardless of key order, and matchers are collected
// for the apply-last pass Parse.parser runs (exactly as with the
// chained builder words, which this form is equivalent to and composes
// with). ref entries feed the same named-action table Parse.action
// fills, so they serve both ABNF marks ('@rule:phase' /
// '@rule:o|c:MARK') and rule-alt `a:'@name'` references. Fixed tokens
// have no section of their own — they are tabnas options
// (options.fixed.token), exactly as in a serialized tabnas grammar.
func parseSpecHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.spec", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.spec", r); err != nil {
		return nil, err
	}
	return nil, applySpecMap(g, args[1], r, false)
}

// parseSpecReturns is Parse.spec's check-mode hook: the map argument is
// usually CONCRETE under analysis (the grammar carrier is not), and its
// section/shape rules are map-decidable — so a dry validation pass runs
// against a scratch builder and surfaces each shape error as a check
// diagnostic with the byte-identical runtime message. The pass is
// LENIENT: a non-concrete section or field value (a carrier riding in
// the map) is skipped, never flagged.
func parseSpecReturns(args []native.Value, r *native.Registry) []native.Value {
	if r != nil && r.Check.IsActive() && len(args) == 2 && native.IsConcrete(args[1]) {
		scratch := &parseGrammar{j: tabnas.Make(), markActions: tabnasabnf.ActionsMap{}}
		if err := applySpecMap(scratch, args[1], r, true); err != nil {
			code, detail := "parse_bad_spec", err.Error()
			var ae *core.BoruError
			if errors.As(err, &ae) {
				code, detail = ae.Code, ae.Detail
			}
			core.CheckAddUniqueDiagnostic(r, code, detail, "Parse.spec", args[1].Pos())
		}
	}
	return []native.Value{}
}

// applySpecMap decomposes a whole-grammar map onto a builder. In
// lenient mode (the check-mode dry pass) a non-concrete value is
// skipped where the strict (runtime) mode requires a concrete shape.
func applySpecMap(g *parseGrammar, specV native.Value, r *native.Registry, lenient bool) error {
	m, merr := native.RequireConcreteMap(specV, "Parse.spec")
	if merr != nil {
		return r.BoruError("parse_bad_spec", fmt.Sprintf("Parse.spec: %v", merr), "Parse.spec")
	}
	for _, k := range m.Keys() {
		switch k {
		case "options", "ref", "rule", "v", "abnf", "matcher":
		default:
			return r.BoruErrorHint("parse_bad_spec",
				fmt.Sprintf("Parse.spec: unknown section %q", k), "Parse.spec",
				"sections: options, ref, rule, v (the tabnas GrammarSpec shape), plus abnf and matcher")
		}
	}
	// A section whose value is not a concrete map is a shape error at
	// runtime and a skip under the lenient dry pass.
	section := func(key, want string) (native.ReadMap, error) {
		v, ok := m.Get(key)
		if !ok {
			return nil, nil
		}
		if lenient && !native.IsConcrete(v) {
			return nil, nil
		}
		sm, _ := native.AsMap(v)
		if sm == nil {
			return nil, r.BoruError("parse_bad_spec",
				fmt.Sprintf("Parse.spec: the %s section must be a map of %s", key, want), "Parse.spec")
		}
		return sm, nil
	}

	// options (+ v): the tabnas OptionsMap, applied FIRST — exactly as
	// tabnas.Grammar applies a GrammarSpec's options before its rules.
	// Fixed tokens ride here (options.fixed.token), the start rule too
	// (options.rule.start), so a fully declarative grammar needs no
	// section beyond options + rule + ref. The optional v key is the
	// builtin config-schema version gate, passed through verbatim.
	om, err := section("options", "tabnas options (fixed / rule / space / …)")
	if err != nil {
		return err
	}
	schemaV := 0
	if vv, ok := m.Get("v"); ok && !(lenient && !native.IsConcrete(vv)) {
		n, verr := vv.AsConcreteInteger()
		if verr != nil {
			return r.BoruError("parse_bad_spec",
				fmt.Sprintf("Parse.spec: v must be an Integer, got %s", vv.String()), "Parse.spec")
		}
		schemaV = int(n)
		// The version gate is map-decidable — enforce it here (fail
		// fast, and the check-mode dry pass flags it) rather than at
		// the deferred register replay. Mirrors tabnas.Grammar's gate.
		if schemaV < 0 || schemaV > tabnas.BUILTIN_SCHEMA_VERSION {
			return r.BoruError("parse_bad_spec",
				fmt.Sprintf("Parse.spec: v requires builtin schema version %d, but this engine supports up to %d", schemaV, tabnas.BUILTIN_SCHEMA_VERSION), "Parse.spec")
		}
	}
	if om != nil || schemaV != 0 {
		gs := &tabnas.GrammarSpec{V: schemaV}
		if om != nil {
			gs.OptionsMap = specAnyMap(om)
		}
		g.steps = append(g.steps, func() error {
			if gerr := g.j.Grammar(gs); gerr != nil {
				return fmt.Errorf("options: %v", gerr)
			}
			return nil
		})
	}

	// ref: {'@name': fn | [fn …]} — the named-action table, before the
	// rules/abnf that reference the entries. Feeds the same table
	// Parse.action fills, so a ref serves both ABNF marks and rule-alt
	// a:'@name' references.
	am, err := section("ref", "'@name' → Function")
	if err != nil {
		return err
	}
	for _, ref := range tmKeys(am) {
		if !strings.HasPrefix(ref, "@") {
			return r.BoruErrorHint("parse_bad_action",
				fmt.Sprintf("Parse.spec: ref %q must start with '@'", ref), "Parse.spec",
				"use @name (rule-alt a: reference), @rule:phase (bo/ao/bc/ac) or @rule:o|c:MARK")
		}
		fv, _ := am.Get(ref)
		fns := []native.Value{fv}
		if fv.Parent.ConformsTo(native.TList) && native.IsConcrete(fv) {
			lst, _ := native.AsList(fv)
			fns = lst.Slice()
		}
		for _, fn := range fns {
			if lenient && !native.IsConcrete(fn) {
				continue
			}
			if !isCallableValue(fn) {
				return r.BoruError("parse_bad_action",
					fmt.Sprintf("Parse.spec: ref %s: the value must be a Function (or a list of Functions), got %s", ref, fn.String()), "Parse.spec")
			}
			g.markActions[ref] = append(g.markActions[ref], g.wrapAction(fn))
		}
	}

	// rule: {name: {open:[…] close:[…]}} — the Parse.rule shape per entry.
	rm, err := section("rule", "name → {open:[…] close:[…]}")
	if err != nil {
		return err
	}
	for _, name := range tmKeys(rm) {
		specEntry, _ := rm.Get(name)
		if lenient && !native.IsConcrete(specEntry) {
			continue
		}
		gs, rerr := ruleMapToSpec(g, name, specEntry, r)
		if rerr != nil {
			return rerr
		}
		name := name
		g.steps = append(g.steps, func() error {
			if gerr := g.j.Grammar(gs); gerr != nil {
				return fmt.Errorf("rule %q: %v", name, gerr)
			}
			return nil
		})
	}

	// abnf: String | {src:… start:… tag:… builtins:… marks:…} | [entry …]
	if v, ok := m.Get("abnf"); ok && !(lenient && !native.IsConcrete(v)) {
		entries := []native.Value{v}
		if v.Parent.ConformsTo(native.TList) && native.IsConcrete(v) {
			lst, _ := native.AsList(v)
			entries = lst.Slice()
		}
		for _, e := range entries {
			if lenient && !native.IsConcrete(e) {
				continue
			}
			var src string
			var opts native.Value
			switch {
			case e.Parent.ConformsTo(native.TString) && native.IsConcrete(e):
				src, _ = e.AsConcreteString()
			case e.Parent.ConformsTo(native.TMap) && native.IsConcrete(e):
				em, _ := native.AsMap(e)
				s, ok := native.MapFieldString(em, "src")
				if em == nil || !ok {
					return r.BoruError("parse_bad_abnf",
						"Parse.spec: an abnf entry map must carry a src:String field (plus optional start/tag/builtins/marks)", "Parse.spec")
				}
				src = s
				opts = e // start/tag/builtins/marks read off the same map
			default:
				return r.BoruError("parse_bad_abnf",
					fmt.Sprintf("Parse.spec: the abnf section must be a String, a {src:…} map, or a list of those, got %s", e.String()), "Parse.spec")
			}
			g.addAbnfStep(src, abnfOptsFrom(opts))
		}
	}

	// matcher: {name: {priority fn}} — collected here; the builder
	// applies matchers LAST at register, after every grammar step.
	mm, err := section("matcher", "name → {priority:Integer fn:Function}")
	if err != nil {
		return err
	}
	for _, name := range tmKeys(mm) {
		ev, _ := mm.Get(name)
		if lenient && !native.IsConcrete(ev) {
			continue
		}
		em, _ := native.AsMap(ev)
		if em == nil {
			return r.BoruError("parse_bad_matcher",
				fmt.Sprintf("Parse.spec: matcher %s: the entry must be a {priority fn} map", name), "Parse.spec")
		}
		prio, ok := native.MapFieldInteger(em, "priority")
		if !ok {
			return r.BoruError("parse_bad_matcher",
				fmt.Sprintf("Parse.spec: matcher %s: priority must be an Integer", name), "Parse.spec")
		}
		fn, ok := em.Get("fn")
		if !ok || (!isCallableValue(fn) && !(lenient && !native.IsConcrete(fn))) {
			return r.BoruError("parse_bad_matcher",
				fmt.Sprintf("Parse.spec: matcher %s: fn must be a Function", name), "Parse.spec")
		}
		g.matchers = append(g.matchers, pendingMatcher{name: name, prio: int(prio), fn: fn})
	}
	return nil
}

// tmKeys is the nil-tolerant key walk for the optional sections.
func tmKeys(m native.ReadMap) []string {
	if m == nil {
		return nil
	}
	return m.Keys()
}

// parseRuleHandler — args[0]=grammar, args[1]=name (atom), args[2]=spec map.
func parseRuleHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.rule", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.rule", r); err != nil {
		return nil, err
	}
	name, err := args[1].AsConcreteAtom()
	if err != nil {
		return nil, r.BoruError("parse_bad_rule", fmt.Sprintf("Parse.rule: name: %v", err), "Parse.rule")
	}
	gs, err := ruleMapToSpec(g, name, args[2], r)
	if err != nil {
		return nil, err
	}
	g.steps = append(g.steps, func() error {
		if gerr := g.j.Grammar(gs); gerr != nil {
			return fmt.Errorf("rule %q: %v", name, gerr)
		}
		return nil
	})
	return nil, nil
}

// parseTokenHandler — args[0]=grammar, args[1]=name (atom), args[2]=literal.
func parseTokenHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.token", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.token", r); err != nil {
		return nil, err
	}
	name, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("parse_bad_token", fmt.Sprintf("Parse.token: name: %v", err), "Parse.token")
	}
	lit, err := args[2].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("parse_bad_token", fmt.Sprintf("Parse.token: literal: %v", err), "Parse.token")
	}
	g.steps = append(g.steps, func() error {
		g.j.Token(name, lit)
		return nil
	})
	return nil, nil
}

// parseMatcherHandler — args[0]=grammar, args[1]=name, args[2]=priority, args[3]=fn.
func parseMatcherHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.matcher", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.matcher", r); err != nil {
		return nil, err
	}
	name, err := args[1].AsConcreteAtom()
	if err != nil {
		return nil, r.BoruError("parse_bad_matcher", fmt.Sprintf("Parse.matcher: name: %v", err), "Parse.matcher")
	}
	prio, err := args[2].AsConcreteInteger()
	if err != nil {
		return nil, r.BoruError("parse_bad_matcher", fmt.Sprintf("Parse.matcher: priority: %v", err), "Parse.matcher")
	}
	g.matchers = append(g.matchers, pendingMatcher{name: name, prio: int(prio), fn: args[3]})
	return nil, nil
}

// parseActionHandler — args[0]=grammar, args[1]=ref, args[2]=fn.
func parseActionHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	g, err := asParseGrammar(args[0], "Parse.action", r)
	if err != nil {
		return nil, err
	}
	if err := g.ensureOpen("Parse.action", r); err != nil {
		return nil, err
	}
	ref, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("parse_bad_action", fmt.Sprintf("Parse.action: ref: %v", err), "Parse.action")
	}
	if !strings.HasPrefix(ref, "@") {
		return nil, r.BoruErrorHint("parse_bad_action",
			fmt.Sprintf("Parse.action: ref %q must start with '@'", ref), "Parse.action",
			"use @rule:phase (bo/ao/bc/ac) or @rule:o|c:MARK")
	}
	g.markActions[ref] = append(g.markActions[ref], g.wrapAction(args[2]))
	return nil, nil
}

// parseParserHandlerFor builds the terminal finalizer word. It finalizes
// the builder — replaying the deferred grammar steps (tokens, rules, ABNF
// installs) now that every mark action is known, then applying custom
// matchers LAST (priority-sorted) so a grammar install cannot drop or
// reorder them — and returns the parser as a ParseLang Function VALUE: the
// source-resolving framework shell (parseSourceShell) over the grammar's
// parse handler, wrapped as a trivial-delegation FnDef in the boru:parse
// sub-registry so it dispatches exactly like a ParseLang.parse_<kind>
// export does (and satisfies ParseLangFnSigWhy for the `parse` value form).
func parseParserHandlerFor(subReg *native.Registry) native.Handler {
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		g, err := asParseGrammar(args[0], "Parse.parser", r)
		if err != nil {
			return nil, err
		}
		if err := g.ensureOpen("Parse.parser", r); err != nil {
			return nil, err
		}
		for _, step := range g.steps {
			if serr := step(); serr != nil {
				return nil, r.BoruErrorHint("parse_bad_grammar",
					fmt.Sprintf("Parse.parser: %s", serr.Error()),
					"Parse.parser", "check the grammar is well-formed")
			}
		}
		g.applyMatchers()
		// Single-use: the finalized parser closes over g/g.j; further builder
		// words or a second finalize must not mutate it.
		g.registered = true

		// Uniquely named per finalize: two parsers carry different grammars,
		// and downstream identity keys on the name. (Hyphen-digits, not the
		// mini-partial `#N` convention — this name IS a registered word and
		// must pass ValidateWordName.)
		parseParserSeq++
		inner := fmt.Sprintf("parse-parser-%d", parseParserSeq)
		subReg.RegisterNativeFunc(native.NativeFunc{
			Name: inner,
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TAny, native.TMap},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Impl:       native.Go(parseSourceShell(g.parseHandler("parser"))),
			}},
		})
		return []native.Value{wrapMiniFnDef(inner, [][]native.FnParam{
			{{Type: native.TAny}, {Type: native.TMap}},
		}, []*native.Type{native.TAny}, nil, subReg)}, nil
	}
}

// parseParserSeq disambiguates finalized-parser names process-wide.
var parseParserSeq int

// parseParserReturns is parse-parser's check-mode value: the grammar
// carrier is never concrete under analysis, so the parser is an abstract
// Function carrier — PLAIN, not dynamic: the declared return type is exact
// (the finalizer always yields a conforming parser fn), and a plain typed
// carrier is what lets the compile pass record both this call and the
// downstream parselang-fn-dispatch without tripping the dynamic-operand
// compile failures. The `parse` macro's carrier branch handles the dispatch.
func parseParserReturns(_ []native.Value, _ *native.Registry) []native.Value {
	return []native.Value{native.NewCarrier(native.TFunction)}
}

// parseHandler builds the ParseLang (the ParseLangSpec handler) that runs
// the constructed parser over a source string and converts the result. It
// binds g.r for the duration of the parse (so callbacks dispatch boru fns)
// and surfaces the first callback error as a parse error.
func (g *parseGrammar) parseHandler(kind string) ParseLang {
	target := "parse_" + kind
	return func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		src, err := args[0].AsConcreteString() // resolved by the framework
		if err != nil {
			return nil, r.BoruError("parse_error", kind+": src: "+err.Error(), target)
		}
		g.firstErr = nil
		g.r = r
		out, perr := g.j.Parse(src)
		g.r = nil
		if g.firstErr != nil {
			return nil, r.BoruErrorHint("parse_syntax_error",
				kind+": "+native.FirstCleanLine(g.firstErr.Error()), target,
				"a grammar action/matcher raised this error")
		}
		if perr != nil {
			return nil, r.BoruErrorHint("parse_syntax_error",
				kind+": "+native.FirstCleanLine(perr.Error()), target,
				"check that the source is well-formed "+kind)
		}
		return []native.Value{native.AnyToValue(out)}, nil
	}
}

// wrapAction wraps a boru fn as a tabnas semantic action (an AltAction): it
// hands the fn the rule's current node (as a boru Value), and stores the fn's
// returned Value back on the node — which, because nodes may now BE boru
// Values (see the native.AnyToValue pass-through), is how an action emits a
// custom-typed value into the parse result.
func (g *parseGrammar) wrapAction(fn native.Value) tabnas.AltAction {
	return func(rule *tabnas.Rule, _ *tabnas.Context) {
		if g.firstErr != nil || g.r == nil {
			return
		}
		node := native.AnyToValue(rule.Node)
		res, err := callParseFn(g.r, fn, []native.Value{node})
		if err != nil {
			g.setErr(err)
			return
		}
		if len(res) > 0 {
			rule.Node = res[len(res)-1]
		}
	}
}

// wrapMatcher wraps a boru fn as a tabnas custom lex matcher. The fn receives
// the unconsumed source as a String and returns either None (no match) or a
// Map {src:String tin:String? val:Any?}: src is the matched prefix, tin the
// emitted token name (default "#TX" — a text token that flows to the value
// rule), val an optional token value. A malformed return records firstErr.
func (g *parseGrammar) wrapMatcher(fn native.Value) tabnas.LexMatcher {
	return func(lex *tabnas.Lex, _ *tabnas.Rule) *tabnas.Token {
		if g.firstErr != nil || g.r == nil {
			return nil
		}
		cur := lex.Cursor()
		if cur.SI >= len(lex.Src) {
			return nil
		}
		rest := lex.Src[cur.SI:]
		res, err := callParseFn(g.r, fn, []native.Value{native.NewString(rest)})
		if err != nil {
			g.setErr(err)
			return nil
		}
		if len(res) == 0 {
			return nil
		}
		top := res[len(res)-1]
		if !native.IsConcrete(top) || top.Parent.ConformsTo(native.TNone) {
			return nil // no match
		}
		m, _ := native.AsMap(top)
		if m == nil {
			g.setErr(fmt.Errorf("matcher must return None or a {src:String …} map"))
			return nil
		}
		matched, hasSrc := native.MapFieldString(m, "src")
		if !hasSrc || matched == "" {
			g.setErr(fmt.Errorf("matcher result must have a non-empty 'src' String"))
			return nil
		}
		if !strings.HasPrefix(rest, matched) {
			g.setErr(fmt.Errorf("matcher 'src' %q is not a prefix of the unconsumed source", matched))
			return nil
		}
		tinName := "#TX"
		if s, ok := native.MapFieldString(m, "tin"); ok && s != "" {
			tinName = s
		}
		tin := tabnas.TinTX
		if tinName != "#TX" {
			tin = g.j.Token(tinName)
		}
		var val any = matched
		if vv, ok := m.Get("val"); ok {
			val = native.ValueToAny(vv)
		}
		tkn := lex.Token(tinName, tin, val, matched)
		cur.SI += len(matched)
		cur.CI += utf8.RuneCountInString(matched)
		return tkn
	}
}

// callParseFn invokes a boru function value (a matcher or action callback)
// against args, handling both the compiled-closure and interpreter-FnDef
// shapes — the same seam filter's callback uses (native/filter.go).
func callParseFn(r *native.Registry, fn native.Value, args []native.Value) ([]native.Value, error) {
	if native.IsCompiledClosure(fn) {
		return native.InvokeBody(r, fn, args)
	}
	sig := native.MatchFnSig(fn, args)
	if sig == nil {
		return nil, fmt.Errorf("no matching callback signature")
	}
	var fnDef *native.FnDefInfo
	if fd, ok := fn.Data.(native.FnDefInfo); ok {
		fnDef = &fd
	}
	// InvokeCallbackFn, not r.CallBoru: a matcher/action written in the grammar's
	// own module resolves its free words there
	// (design/FUNCTION-VALUE-SCOPE.0.md), and the body is offered to the VM before
	// the compile failure.
	return core.InvokeCallbackFn(r, fnDef, sig, args)
}

// abnfOptsFrom reads the {start tag builtins marks} option map into the
// AbnfConvertOptions. Builtins defaults true (most ABNF grammars reference
// the core rules ALPHA/DIGIT/…); a nil/empty map keeps the defaults.
func abnfOptsFrom(opts native.Value) *tabnasabnf.AbnfConvertOptions {
	o := &tabnasabnf.AbnfConvertOptions{Builtins: true}
	m, _ := native.AsMap(opts)
	if m == nil {
		return o
	}
	if s, ok := native.MapFieldString(m, "start"); ok {
		o.Start = s
	}
	if s, ok := native.MapFieldString(m, "tag"); ok {
		o.Tag = s
	}
	if b, ok := native.MapFieldBoolean(m, "builtins"); ok {
		o.Builtins = b
	}
	if b, ok := native.MapFieldBoolean(m, "marks"); ok {
		o.Marks = b
	}
	return o
}

// ruleMapToSpec converts a declarative rule map into a one-rule GrammarSpec.
// The rule map mirrors tabnas GrammarRuleSpec: {open:[alt…] close:[alt…]},
// each alt an AltSpec map {s p r b a g n}. An inline Function in `a` is wrapped
// as an action and referenced from the spec's Ref table.
func ruleMapToSpec(g *parseGrammar, name string, specV native.Value, r *native.Registry) (*tabnas.GrammarSpec, error) {
	m, _ := native.AsMap(specV)
	if m == nil {
		return nil, r.BoruError("parse_bad_rule",
			fmt.Sprintf("Parse.rule %q: spec must be a {open:[…] close:[…]} map", name), "Parse.rule")
	}
	gs := &tabnas.GrammarSpec{Ref: map[string]any{}}
	rs := &tabnas.GrammarRuleSpec{}

	build := func(key string) (any, error) {
		v, ok := m.Get(key)
		if !ok {
			return nil, nil
		}
		lst, lerr := native.AsList(v)
		if lerr != nil {
			return nil, r.BoruError("parse_bad_rule",
				fmt.Sprintf("Parse.rule %q: %s must be a list of alternates", name, key), "Parse.rule")
		}
		alts := make([]*tabnas.GrammarAltSpec, 0, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			alt, err := altMapToSpec(g, gs, lst.Get(i), name, r)
			if err != nil {
				return nil, err
			}
			alts = append(alts, alt)
		}
		return alts, nil
	}

	open, err := build("open")
	if err != nil {
		return nil, err
	}
	rs.Open = open
	closeAlts, err := build("close")
	if err != nil {
		return nil, err
	}
	rs.Close = closeAlts

	gs.Rule = map[string]*tabnas.GrammarRuleSpec{name: rs}
	return gs, nil
}

// altMapToSpec converts a single alternate map to a GrammarAltSpec.
func altMapToSpec(g *parseGrammar, gs *tabnas.GrammarSpec, altV native.Value, rule string, r *native.Registry) (*tabnas.GrammarAltSpec, error) {
	m, _ := native.AsMap(altV)
	if m == nil {
		return nil, r.BoruError("parse_bad_rule",
			fmt.Sprintf("Parse.rule %q: each alternate must be a map", rule), "Parse.rule")
	}
	alt := &tabnas.GrammarAltSpec{}
	if s, ok := native.MapFieldString(m, "s"); ok {
		alt.S = s
	}
	if s, ok := native.MapFieldString(m, "p"); ok {
		alt.P = s
	}
	if s, ok := native.MapFieldString(m, "r"); ok {
		alt.R = s
	}
	if s, ok := native.MapFieldString(m, "g"); ok {
		alt.G = s
	}
	if bv, ok := m.Get("b"); ok {
		switch {
		case bv.Parent.ConformsTo(native.TInteger) && native.IsConcrete(bv):
			n, _ := bv.AsConcreteInteger()
			alt.B = int(n)
		case bv.Parent.ConformsTo(native.TString) && native.IsConcrete(bv):
			s, _ := bv.AsConcreteString()
			alt.B = s
		}
	}
	if nv, ok := m.Get("n"); ok {
		nm, _ := native.AsMap(nv)
		if nm != nil {
			counters := map[string]int{}
			for _, k := range nm.Keys() {
				cv, _ := nm.Get(k)
				if cv.Parent.ConformsTo(native.TInteger) && native.IsConcrete(cv) {
					ci, _ := cv.AsConcreteInteger()
					counters[k] = int(ci)
				}
			}
			alt.N = counters
		}
	}
	if av, ok := m.Get("a"); ok {
		switch {
		case isCallableValue(av):
			ref := fmt.Sprintf("@__inline_%d", g.inlineSeq)
			g.inlineSeq++
			gs.Ref[ref] = g.wrapAction(av)
			alt.A = ref
		case av.Parent.ConformsTo(native.TString) && native.IsConcrete(av):
			s, _ := av.AsConcreteString()
			alt.A = s
			// A string ref (e.g. '@hit') names an action registered via
			// Parse.action; wire it into the spec's Ref map so tabnas.Grammar
			// can resolve it (only inline fns auto-populate Ref otherwise). An
			// unregistered ref is left alone so tabnas raises "unresolved ref".
			if acts, ok := g.markActions[s]; ok && len(acts) > 0 {
				gs.Ref[s] = composeAltActions(acts)
			}
		case av.Parent.ConformsTo(native.TList) && native.IsConcrete(av):
			// The tabnas list-of-actions form: refs and inline fns run
			// in order, each seeing the previous action's node.
			lst, _ := native.AsList(av)
			refs := make([]any, 0, lst.Len())
			for i := 0; i < lst.Len(); i++ {
				el := lst.Get(i)
				switch {
				case isCallableValue(el):
					ref := fmt.Sprintf("@__inline_%d", g.inlineSeq)
					g.inlineSeq++
					gs.Ref[ref] = g.wrapAction(el)
					refs = append(refs, ref)
				case el.Parent.ConformsTo(native.TString) && native.IsConcrete(el):
					s, _ := el.AsConcreteString()
					if acts, ok := g.markActions[s]; ok && len(acts) > 0 {
						gs.Ref[s] = composeAltActions(acts)
					}
					refs = append(refs, s)
				default:
					return nil, r.BoruError("parse_bad_rule",
						fmt.Sprintf("Parse.rule %q: a: each action must be a Function or a '@ref' String, got %s", rule, el.String()), "Parse.rule")
				}
			}
			alt.A = refs
		}
	}
	// c — a DECLARATIVE condition map, matched by $eq. Each KEY is a dotted
	// path onto a rule property, ROOTED at one tabnas exposes: the counters
	// `n` (`{'n.dlist':0}`), user data `u`, kept data `k`, `d`/`i`/`name`/
	// `state`, the token slots (`o`/`c`/`o0`/`o1`/`c0`/`c1`/`oN`/`cN`),
	// `node`, `spec`, or the rule-graph links `parent`/`child`/`prev`/`next`.
	// A BARE counter name (`{'dlist':0}`) is NOT that path and never was:
	// it cannot resolve, so the guard fails open — silently doing nothing.
	// tabnas/parser v0.8.0 rejects it while the grammar is built, which is
	// how two such dead conditions in this repo were found. The FuncRef
	// condition form takes parser-internal types (tabnas.AltCond) and is not
	// expressible from boru; the declarative map is.
	if cv, ok := m.Get("c"); ok {
		cm, ok := specDataMap(cv)
		if !ok {
			return nil, r.BoruError("parse_bad_rule",
				fmt.Sprintf("Parse.rule %q: c: the condition must be a declarative map", rule), "Parse.rule")
		}
		alt.C = cm
	}
	// u / k — custom (and propagated-custom) props: plain data maps.
	if uv, ok := m.Get("u"); ok {
		um, ok := specDataMap(uv)
		if !ok {
			return nil, r.BoruError("parse_bad_rule",
				fmt.Sprintf("Parse.rule %q: u: custom props must be a map", rule), "Parse.rule")
		}
		alt.U = um
	}
	if kv, ok := m.Get("k"); ok {
		km, ok := specDataMap(kv)
		if !ok {
			return nil, r.BoruError("parse_bad_rule",
				fmt.Sprintf("Parse.rule %q: k: propagated props must be a map", rule), "Parse.rule")
		}
		alt.K = km
	}
	return alt, nil
}

// specDataMap converts a concrete boru map to a plain map[string]any for
// the declarative alt fields (c / u / k). Integers convert to int at
// every depth — the width tabnas' declarative normalizers (conditions,
// MapToOptions) match on.
func specDataMap(v native.Value) (map[string]any, bool) {
	m, _ := native.AsMap(v)
	if m == nil {
		return nil, false
	}
	return specAnyMap(m), true
}

// specAnyMap is the deep ReadMap → map[string]any conversion behind
// specDataMap and the Parse.spec options section.
func specAnyMap(m native.ReadMap) map[string]any {
	out := make(map[string]any, m.Len())
	for _, k := range m.Keys() {
		fv, _ := m.Get(k)
		out[k] = specAnyValue(fv)
	}
	return out
}

func specAnyValue(v native.Value) any {
	switch {
	case v.Parent.ConformsTo(native.TInteger) && native.IsConcrete(v):
		n, _ := v.AsConcreteInteger()
		return int(n)
	case v.Parent.ConformsTo(native.TMap) && native.IsConcrete(v):
		if m, _ := native.AsMap(v); m != nil {
			return specAnyMap(m)
		}
	case v.Parent.ConformsTo(native.TList) && native.IsConcrete(v):
		if l, err := native.AsList(v); err == nil {
			out := make([]any, l.Len())
			for i := 0; i < l.Len(); i++ {
				out[i] = specAnyValue(l.Get(i))
			}
			return out
		}
	}
	return core.ToNative(v)
}

// isCallableValue reports whether v is a function-like value (a Function or an
// FnDef) usable as an inline grammar action.
func isCallableValue(v native.Value) bool {
	if v.Parent == native.TFunction {
		return true
	}
	_, ok := v.Data.(native.FnDefInfo)
	return ok
}
