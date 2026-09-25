package native

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// Macro system words. See design/MACROS.8.md and design/legacy/MACROS-PHASE1.10.ignore.
//
// - gensym (Phase 0): fresh non-colliding atoms.
// - macro (1c): the definer — an fn the expander runs on UNEVALUATED operand
//   forms, whose returned token list is spliced into the call site.
// - unquote / splice (1d): template escapes, recognized by the expander
//   (eng/go/macro_expand.go); they error if stepped outside an expansion.
// - macroexpand (1e): introspection — return the expansion without splicing.

var macroNatives = []NativeFunc{
	{
		Name: "gensym",
		// `gensym` mints a fresh, never-colliding atom (`tmp$G<n>`) — the
		// Common-Lisp temporary generator. Capture-free temporaries for
		// hand-written `word`/`__SP` macros today, and the manual-hygiene
		// tool before automatic hygiene (Phase 4) lands. Zero args.
		Signatures: []Signature{{
			Args: []*Type{},
			Impl: Go(gensymHandler, RunInCheck()),
			// Pure mint (a per-registry counter → a fresh `tmp$G<n>` atom), so
			// it runs in check mode too: a hand-hygiene macro that binds
			// `def g (gensym)` and splices `def unquote g …` needs a REAL name
			// during the check pass, or the expansion produces `def <empty>`
			// (invalid_word_name). Running it also keeps the check-time and
			// runtime gensym counters in lockstep, so the compiled expansion is
			// byte-identical to the interpreted one.
			Returns: []*Type{TAtom}, BarrierPos: 0,
		}},
	},
	{
		Name: "macro",
		// `macro [[params] [body]]` builds a macro: an fn whose every param is
		// raw-capture (FormArgs + NoEvalArgs + NoEvalMapArgs) and whose body
		// runs at expansion time, the returned template spliced into the call
		// site. NoEvalArgs{0}: the [[params][body]] list must not be evaluated
		// (the template body must not run at definition time).
		Signatures: []Signature{{
			Args:       []*Type{TList},
			NoEvalArgs: map[int]bool{0: true},
			Impl:       Go(macroHandler, RunInCheck()),
			// Pure construction, like fn/fnsig — runs in check mode so
			// the macro INSTALLS during the check pass: its uses then
			// expand on the tape (execMacro) and the checker (and the
			// bytecode recording pass) see the expanded stream, never
			// the raw-form operand span (plan R6 #29).
			Returns: []*Type{TFunction}, BarrierPos: -1,
		}},
	},
	{
		Name: "unquote",
		// Template escape — recognized by the expander. Stepped outside an
		// expansion it is a misuse and errors UNCONDITIONALLY, so the
		// DryPassWrap mirror flags a top-level stepped use at check time
		// (inside a template the expander consumes the word — it is never
		// dispatched, so the mirror cannot misfire there).
		Signatures: []Signature{{
			Args:      []*Type{TAny},
			Impl:      Go(unquoteOutsideMacroHandler),
			ReturnsFn: DryPassWrap(unquoteOutsideMacroHandler, ReturnsStatic(TAny)),
			Returns:   []*Type{TAny}, BarrierPos: -1,
		}},
	},
	{
		Name: "splice",
		// See unquote: an unconditional misuse error, mirrored statically.
		Signatures: []Signature{{
			Args:      []*Type{TAny},
			Impl:      Go(spliceOutsideMacroHandler),
			ReturnsFn: DryPassWrap(spliceOutsideMacroHandler, ReturnsStatic(TAny)),
			Returns:   []*Type{TAny}, BarrierPos: -1,
		}},
	},
	{
		Name: "macroexpand",
		// `macroexpand (mac args…)` — return the expanded token list as data
		// (a List) WITHOUT splicing/running it. Introspection for testing and
		// debugging. The macro call is captured raw (FormArgs{0}); see
		// macroexpandHandler.
		Signatures: []Signature{{
			Args:     []*Type{TAny},
			FormArgs: map[int]bool{0: true},
			Impl:     Go(macroexpandHandler),
			Returns:  []*Type{TList}, BarrierPos: -1,
		}},
	},
	{
		Name: "mini",
		// `mini <kind> <src> <opts?>` — embedded mini-languages
		// (design/MINILANG.5.md). A macro in effect: the call expands to the
		// STANDARD minilang call
		//
		//	MiniLang.lang_<kind> <src> <opts> end
		//
		// spliced at the call site (the trailing `end` stops the generated
		// word's forward collection, so kind-declared inputs always come
		// from the STACK and the tokens after the mini call are never
		// stolen). The kind names the expansion target and must be a
		// literal; it is resolved against the imported `MiniLang` namespace
		// at expansion time — unknown kinds fail loudly here, not
		// downstream. A missing opts is normalized to {}.
		//
		// RunInCheckMode: the handler is check-safe (the splice is built
		// from the collected values whether or not they are concrete), so
		// the checker steps the expansion and validates the call against
		// the kind's standard signature.
		//
		// The handler-contract declaration (design/HANDLER-MIGRATION-LINE.0.md,
		// S2a) on every overload — the quoted kind AND the fn value form — is
		// CompileResteps: the handler returns a SPLICE the tape runs, so
		// neither the kind atom nor the transducer fn is data a CALL_NATIVE
		// could bake (CompileQuoteInert / CompileReadsFn would promise
		// exactly that). The check engine steps the expansion and the
		// recorder follows it, unchanged; the flag names the refusal the
		// zero value used to make silently. Same for `emit` and `parse`.
		Signatures: []Signature{
			{
				Args:          []*Type{TAtom, TString, TMap},
				QuoteArgs:     map[int]bool{0: true},
				Impl:          Go(miniHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TAtom, TString},
				QuoteArgs:     map[int]bool{0: true},
				Impl:          Go(miniHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			// The mini-language VALUE form — `mini <fn> <src> <opts?>`:
			// the first operand IS the transducer, a fn (or a word bound to
			// one, handled in miniHandler) whose every signature opens with
			// the standard [src opts] prefix. Every fn value — anonymous
			// literal, def'd fn, module export — is TFunction (ADR-011).
			{
				Args:          []*Type{TFunction, TString, TMap},
				Impl:          Go(miniHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TFunction, TString},
				Impl:          Go(miniHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
		},
	},
	{
		Name: "emit",
		// `emit <kind> <opts?> <data>` (explicit) and `emit <opts?> <data>`
		// (auto) — the symmetric inverse of `parse`. The call expands to the
		// STANDARD emitter call
		//
		//	EmitLang get emit_<kind|auto> <data> <opts> end
		//
		// spliced at the call site. `data` is the required LAST surface
		// argument; `opts` is the optional middle one (disambiguated by
		// arity). Unlike `parse`, the kind is OPTIONAL: when the leading
		// operand is not a bare word naming a registered emit kind, the call
		// routes to emit_auto (the value's natural format). The data is often
		// a bare variable word, so position 0 is /q-captured (word→atom) and
		// reconstructed as a Word for the splice when it is data, not a kind —
		// so a variable evaluates normally at the call site.
		// Two sig families. The EXPLICIT-kind family declares slot 0 as TAtom
		// with QuoteArgs{0}: when the next surface token is a bare word (the
		// kind, or a bare-variable data word), the matcher's preferWordSig path
		// selects these and the /q forward-quote intercept captures the word as
		// an Atom (instead of dispatching it — exactly how `parse`'s TAtom slot
		// works). The AUTO family declares slot 0 as TAny (no quote): it matches
		// when the leading operand is a literal map/list/scalar (data-first,
		// no kind). The handler then classifies an Atom slot 0 as kind-or-data.
		Signatures: []Signature{
			// CompileResteps on the kind and fn forms: the splice (see mini).
			{
				Args:          []*Type{TAtom, TAny, TAny},
				QuoteArgs:     map[int]bool{0: true},
				Impl:          Go(emitHandler, RunInCheck()),
				Returns:       []*Type{TString},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TAtom, TAny},
				QuoteArgs:     map[int]bool{0: true},
				Impl:          Go(emitHandler, RunInCheck()),
				Returns:       []*Type{TString},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TAtom},
				QuoteArgs:     map[int]bool{0: true},
				Impl:          Go(emitHandler, RunInCheck()),
				Returns:       []*Type{TString},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			// The emitter VALUE form — `emit <fn> <opts?> <data>`: the first
			// operand IS the emitter, a fn whose every signature is
			// [value:Any opts:Map …] → [String …]. These sit BEFORE the auto
			// family: a Function conforms to TAny, so without them a fn
			// operand would silently route to emit_auto and JSON-emit the fn
			// value itself.
			{
				Args:          []*Type{TFunction, TAny, TAny},
				Impl:          Go(emitHandler, RunInCheck()),
				Returns:       []*Type{TString},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TFunction, TAny},
				Impl:          Go(emitHandler, RunInCheck()),
				Returns:       []*Type{TString},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:    []*Type{TAny, TAny},
				Impl:    Go(emitHandler, RunInCheck()),
				Returns: []*Type{TString}, BarrierPos: -1,
			},
			{
				Args:    []*Type{TAny},
				Impl:    Go(emitHandler, RunInCheck()),
				Returns: []*Type{TString}, BarrierPos: -1,
			},
		},
	},
	{
		Name: "parse",
		// `parse <kind> <opts?> <source>` — named parsers (the sibling of
		// `mini`; design/MINILANG.5.md + the ParseLang module). A macro in
		// effect: the call expands to the STANDARD parser call
		//
		//	ParseLang.parse_<kind> <source> <opts> end
		//
		// spliced at the call site. Unlike `mini`, the `source` is the
		// REQUIRED LAST surface argument (a String or a `{src:…}` Source
		// map) and `opts` is the optional MIDDLE one — disambiguated by
		// arity, so a Map source never collides with a Map opts. The kind
		// names the expansion target and must be a literal; it is resolved
		// against the imported `ParseLang` namespace at expansion time —
		// unknown kinds fail loudly here. A parser returns Any (an AST, a
		// transduction, …, per the language).
		//
		// The first operand may instead BE the parser — a ParseLang value,
		// i.e. a Function carrying the standard [source opts] signature
		// prefix (sigs 3-4, and the bare-name fallback in parseHandler for a
		// word bound to one): `parse <fn> <opts?> <source>` expands to the
		// direct call `<fn> <source> <opts> end` with no kind lookup, so a
		// parser can be used without registering it under a kind name.
		//
		// CompileResteps on every overload: the splice (see mini).
		Signatures: []Signature{
			{
				Args:          []*Type{TAtom, TMap, TAny},
				QuoteArgs:     map[int]bool{0: true},
				Impl:          Go(parseHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TAtom, TAny},
				QuoteArgs:     map[int]bool{0: true},
				Impl:          Go(parseHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TFunction, TMap, TAny},
				Impl:          Go(parseHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
			{
				Args:          []*Type{TFunction, TAny},
				Impl:          Go(parseHandler, RunInCheck()),
				Returns:       []*Type{TAny},
				BarrierPos:    -1,
				CompileEffect: CompileResteps,
			},
		},
	},
}

// gensymHandler returns a fresh atom whose name is guaranteed not to collide
// with any prior gensym in this registry (`tmp$G1`, `tmp$G2`, …).
func gensymHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return []Value{NewAtom(r.NextGensym())}, nil
}

// macroDegradedAdvisory records the A8 "macro not statically expandable
// here" INFO advisory (checker-accuracy-review.10.md): the expansion
// degrades to a dynamic value the checker cannot see through, and the
// silence should at least be loud. Non-gating — the degradation is the
// correct behavior; the advisory makes the precision loss visible.
func macroDegradedAdvisory(r *Registry, word, why string, pos SrcPos) {
	r.Check.AddDiagnostic(CheckDiagnostic{
		Code:   "macro_not_expandable",
		Detail: word + ": not statically expandable here — " + why + "; the result is dynamic and unchecked",
		Word:   word,
		Row:    pos.Row,
		Col:    pos.Col,
	})
}

// macroHandler builds a macro FnDef from `[[params] [body]]`. Mirrors the afn
// definer, but flags Macro=true and sets FormArgs/NoEvalArgs/NoEvalMapArgs on
// every param so operands arrive as raw code.
func macroHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	spec, err := RequireConcreteList(args[0], "macro")
	if err != nil {
		return nil, err
	}
	elems := spec.Slice()
	if len(elems) != 2 {
		return nil, r.BoruError("macro_error",
			fmt.Sprintf("macro: expected [[params] [body]] (2 elements), got %d", len(elems)), "macro")
	}
	// Macro params are plain NAMES of type Any (operands arrive as raw code,
	// so there is nothing to type-match). `[cond body]` → two named Any
	// params. (parseFnParams treats bare words as type names, which is wrong
	// here; typed macro params are a later extension.)
	paramsSpec := elems[0]
	if !paramsSpec.Parent.Equal(TList) || !IsConcrete(paramsSpec) {
		return nil, r.BoruError("macro_error", "macro: params must be a list of names", "macro")
	}
	pl, _ := AsList(paramsSpec)
	params := make([]FnParam, 0, pl.Len())
	for i := 0; i < pl.Len(); i++ {
		el := pl.Get(i)
		if !IsWord(el) {
			return nil, r.BoruError("macro_error",
				"macro: each param must be a bare name (typed params are a later extension)", "macro")
		}
		w, _ := AsWord(el)
		params = append(params, FnParam{Name: w.Name, Type: TAny})
	}
	if len(params) == 0 {
		return nil, r.BoruError("macro_error", "macro: needs at least one parameter", "macro")
	}
	barrierPos := len(params) // all operands forward-eligible

	// Body: a list runs as the template-producing program; a bare value is
	// wrapped (mirrors afn).
	var bodyElems []Value
	if elems[1].Parent.Equal(TList) && IsConcrete(elems[1]) {
		bl, _ := AsList(elems[1])
		bodyElems = bl.Slice()
	} else {
		bodyElems = []Value{elems[1]}
	}

	// Every param is raw-capture: FormArgs (word/paren/literal raw), NoEvalArgs
	// (a list operand stays un-evaluated), NoEvalMapArgs (a map operand keeps
	// its values un-evaluated). See design/legacy/MACROS-PHASE1.10.ignore §3/§3.1.
	form := make(map[int]bool, len(params))
	for i := range params {
		form[i] = true
	}
	sig := FnSig{
		Params:        params,
		Returns:       []*Type{TAny},
		Impl:          Boru(bodyElems),
		BarrierPos:    barrierPos,
		FormArgs:      form,
		NoEvalArgs:    form,
		NoEvalMapArgs: form,
	}
	fnDef := FnDefInfo{
		Signatures: []FnSig{sig},
		Macro:      true,
		Captured:   core.ComputeCaptures(r, &sig),
		// Home registry, as FnConstruct stamps it for `fn` and `=>`.
		Registry: r,
	}
	// (Re)constructing a macro invalidates any memoized expansions: a
	// redefined macro must re-expand at its call sites.
	r.MacroCacheClear()
	return []Value{NewFunction(fnDef)}, nil
}

// unquoteOutsideMacroHandler / spliceOutsideMacroHandler fire only when the
// word is stepped outside a macro template (the expander consumes them by name
// during expansion and they never reach dispatch there).
func unquoteOutsideMacroHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.BoruError("unquote_error", "unquote: only valid inside a macro template", "unquote")
}

func spliceOutsideMacroHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.BoruError("splice_error", "splice: only valid inside a macro template", "splice")
}

// macroexpandHandler returns the expansion of a macro call as a List, without
// splicing or running it. args[0] is the raw `(mac operand…)` form (FormArgs);
// it delegates to the kernel expander via core.ExpandMacroForm.
func macroexpandHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	toks, err := core.ExpandMacroForm(r, args[0])
	if err != nil {
		return nil, err
	}
	return []Value{NewList(toks)}, nil
}

// miniHandler expands `mini <kind> <src> <opts?>` into the standard minilang
// call `MiniLang.lang_<kind> <src> <opts> end`, returned as an __SP splice so
// the generated tokens re-step at the call site (the `word` mechanism).
// args[0]=kind (Atom via /q — must be literal), args[1]=src, args[2]=opts
// (2-arg form normalizes to {}). src/opts are spliced as collected — they may
// be carriers in check mode; only the kind must be concrete.
func miniHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if args[0].Parent.ConformsTo(TFunction) {
		return miniFnExpand(args[0], args, r)
	}
	kind, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, r.BoruErrorHint("mini_error",
			"mini: the kind must be a literal name", "mini",
			"write the kind as a bare word: mini re '[a-z]+'")
	}
	target := "lang_" + kind

	// Resolve the kind against the imported MiniLang namespace NOW —
	// unknown kinds are expansion-time errors at the call site.
	if !miniKindRegistered(r, target) {
		// A bare word BOUND to a Function is the mini-language VALUE form
		// spelled by name (`def dbl (fn …)  mini dbl 'ab'`) — mirror parse's
		// bound-variable fallback. A registered kind wins a collision
		// (checked above); an unbound name or a non-Function binding keeps
		// the unknown-kind handling below. A name def-bound to a COMPUTED
		// fn (`def m (mk)`) has no Defs binding under analysis — installDef
		// declines so the compiled closure machinery owns the name — and
		// resolves through the per-pass fn-carrier side table
		// (checkFnCarrierBind) instead, like parse.
		top, bound := r.Defs.Top(kind)
		if !bound || !top.Parent.ConformsTo(TFunction) {
			if v, hit := checkFnCarrierBind(r, kind); hit {
				top, bound = v, true
			} else {
				bound = false
			}
		}
		if bound {
			// The /q-captured word bypassed stepWord's use tracking.
			r.Check.RecordUse(kind)
			if _, isFn := top.Data.(FnDefInfo); isFn {
				return miniFnExpand(top, args, r)
			}
			if r.Check.IsActive() && !IsConcrete(top) {
				macroDegradedAdvisory(r, "mini", "the mini fn bound to "+kind+" is not a concrete value under analysis", args[0].Pos())
				return []Value{NewDynamicCarrier(TAny)}, nil
			}
		}
		if r.Check.IsActive() && !miniNamespaceBound(r) {
			// The import may be outside the checked fragment; degrade to a
			// dynamic value rather than a false-positive diagnostic. A bound
			// namespace WITHOUT the kind is a real bug in any mode. The
			// carrier is dynamic(Any) — the documented escape-hatch
			// modality — not strict Any, which would poison every typed
			// consumer downstream (checker-accuracy-review.10.md A8).
			//
			// For the COMPILE pass the interpreter raises mini_unknown_lang here
			// at runtime, so record a TERMINAL trap (top-level only) — the
			// compiled program then raises the byte-identical error instead of
			// declining on the dynamic carrier downstream. A nested call declines
			// the trap and keeps the lenient fallback.
			r.Check.Recorder().RecordTrap("mini_unknown_lang",
				fmt.Sprintf("mini: no mini-language %q is registered", kind), "mini",
				`import "boru:minilang" first; the kinds are fixed (MiniLang.kinds lists them) — pass a custom mini-language as a fn value: mini <fn> '…'`,
				args[0].Pos())
			macroDegradedAdvisory(r, "mini", "the boru:minilang import is outside the checked fragment", args[0].Pos())
			return []Value{NewDynamicCarrier(TAny)}, nil
		}
		return nil, r.BoruErrorHint("mini_unknown_lang",
			fmt.Sprintf("mini: no mini-language %q is registered", kind), "mini",
			`import "boru:minilang" first; the kinds are fixed (MiniLang.kinds lists them) — pass a custom mini-language as a fn value: mini <fn> '…'`)
	}

	opts := NewMap(NewOrderedMap())
	if len(args) == 3 {
		opts = args[2]
	}

	// Compile-hook path (design/MINILANG.5.md §13). When the kind registered
	// a Go compile hook AND src is concrete, compile at the call site and
	// splice the hook's tokens instead of the standard call. (Hooks are
	// BUILT-IN machinery now — the boru `register-compiled` surface died with
	// the frozen kind namespace; `re` is the shipping example.) A PLAIN
	// check pass stays on the standard `lang_<kind>` call (the semantic
	// reference for checking; a check-mode src is usually a carrier anyway)
	// — but a COMPILE pass must take the SAME path the interpreter takes,
	// or the recorded program bakes the transducer where the runtime runs
	// the hook (`mini up 'hi'` compiled [hi] vs interpreted [HI] — a live
	// miscompile the 2026-07-15 flip attempt caught). So the hook runs when
	// NOT checking, or when COMPILING with a concrete src: the GO hook is
	// pure expansion machinery over the source text (the §13 contract —
	// hooks memoize, deterministic over src+opts), so its spliced tokens
	// record exactly as the interpreter splices them. A compile pass that
	// cannot mirror the hook faithfully (non-concrete src/opts) DECLINES
	// instead of baking the transducer — no wrong answer, but no compile
	// either, so the shape stays an open defect.
	// `mini` has no expansion cache, so the hook re-runs whenever the call
	// is stepped — hooks memoize their compile (as `re` does). A
	// non-concrete runtime src falls back to the standard transducer call.
	if !r.Check.IsActive() || r.Check.Compiling {
		goHook, hasGo := miniGoHook(r, kind)
		if hasGo && r.Check.IsActive() {
			decline := func(why string) {
				if es := r.Check.Recorder(); es.Active() {
					es.MarkUncompilable("mini " + kind + ": " + why)
				}
			}
			// A TRANSDUCER-FAITHFUL kind (re) does not decline a non-concrete
			// src/opts: its hook and the standard transducer call share the same
			// runtime (miniCompiledPattern + reMatchResult), so a dynamic-src
			// invocation records the standard `lang_<kind>` call below and runs
			// identically. Only a kind whose hook bakes a src-specific plan (the
			// test `bf` uppercaser) declines, since baking the transducer instead
			// would drop the hook's semantics.
			if !miniHookFaithful(r, kind) {
				switch {
				case func() bool { _, serr := args[1].AsConcreteString(); return serr != nil }():
					decline("compile hook needs a concrete src at compile time")
				case !IsConcrete(opts):
					decline("compile hook needs concrete opts at compile time")
				}
			}
		}
		if hasGo {
			if src, serr := args[1].AsConcreteString(); serr == nil {
				var hookToks []Value
				var herr error
				if r.Check.IsActive() && !IsConcrete(opts) {
					// Declined above; keep the standard call for analysis.
					hookToks = nil
				} else {
					hookToks, herr = goHook(src, opts, r)
					// A COMPILE pass can only take the hook path when the
					// expansion RECORDS — every token statically
					// materialisable. A hook splicing a runtime carrier
					// (re's precompiled pattern) compiles via the standard
					// transducer call instead, which §13's contract makes
					// sound: the transducer is the semantic reference.
					if herr == nil && r.Check.IsActive() && !miniHookToksMaterialisable(hookToks) {
						hookToks = nil
					}
				}
				if herr != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
					return nil, herr
				}
				if hookToks == nil {
					goto standardCall
				}
				if partial, pok := miniPartialFn(r, kind, target, hookToks); pok {
					// No trailing End: the partial's sigs are STACK-ONLY
					// (BarrierPos 0), so it cannot steal following tokens —
					// and a spliced End LEAKS out of a paren group, ending
					// the OUTER word's statement (see miniPartialFn).
					return []Value{NewSplice(NewList([]Value{partial}))}, nil
				}
				return []Value{NewSplice(NewList(hookToks))}, nil
			}
		}
	}

standardCall:
	// Standard call tail. For a FILTER kind (every sig takes exactly one
	// input beyond the [src opts] prefix) the expansion produces a
	// PARTIALLY-APPLIED anonymous Function instead of a stranded call:
	// src and opts are bound, the subject is the remaining parameter.
	// Standard anonymous-function semantics then do the rest — the fn
	// auto-dispatches when a matching subject is on the stack (today's
	// filter behaviour, unchanged) and stays DATA when not, so it can
	// be bound with def or parked with /v and applied later. Value
	// kinds (hb / bb / math / micron — no input beyond the prefix) and
	// mixed kinds (bf, whose generator form must run bare) keep the
	// immediate call splice: conceptually every mini expansion is a
	// function in place; theirs is called on the spot.
	callTail := []Value{
		NewWord("MiniLang"), NewWord("dot"), NewWord(target),
		args[1], opts, NewEnd(),
	}
	if partial, pok := miniPartialFn(r, kind, target, callTail); pok {
		// No trailing End — see the hook-path comment above.
		return []Value{NewSplice(NewList([]Value{partial}))}, nil
	}
	return []Value{NewSplice(NewList(callTail))}, nil
}

// miniFnExpand expands the mini-language VALUE form `mini <fn> <src>
// <opts?>` into the direct call `<fn> <src> <opts> end`, spliced at the call
// site — the fn supplied by the caller instead of the MiniLang namespace. fn
// must be a concrete Function whose every signature opens with the standard
// [src opts] prefix — the shared MiniLangFnSigWhy contract — so a
// wrong-shaped fn fails loudly rather than
// silently mis-collecting its operands. A FILTER-shaped fn (every sig
// exactly [src opts subject]) expands to the same partially-applied
// Function a registered filter kind produces — subject pending — but with
// no MiniKind tag (member types are name-keyed to registered kinds). A
// Function-typed carrier under analysis degrades to a dynamic value like
// the other not-statically-expandable macro paths.
func miniFnExpand(fn Value, args []Value, r *Registry) ([]Value, error) {
	fnDef, ok := fn.Data.(FnDefInfo)
	if !ok {
		if r.Check.IsActive() && !IsConcrete(fn) {
			macroDegradedAdvisory(r, "mini", "the mini fn is not a concrete value under analysis", args[0].Pos())
			return []Value{NewDynamicCarrier(TAny)}, nil
		}
		// A fn-family value whose payload is not an FnDefInfo — defensive:
		// the sig matcher never delivers one from surface syntax.
		return nil, r.BoruErrorHint("mini_error",
			"mini: the mini-language is not a usable function value", "mini",
			"pass a transducer fn: mini (fn [[src:String opts:Map] [Any] [...]]) 'text'")
	}
	if why := MiniLangFnSigWhy(fnDef); why != "" {
		return nil, r.BoruErrorHint("mini_bad_signature",
			"mini: "+why, "mini",
			"declare the fn as fn [[src:String opts:Map …inputs] [outputs] [body]]")
	}
	opts := NewMap(NewOrderedMap())
	if len(args) == 3 {
		opts = args[2]
	}
	tail := []Value{fn, args[1], opts, NewEnd()}
	if MiniLangFnFilterShaped(fnDef) {
		// No trailing End on the partial splice — see miniPartialFn.
		partial := miniPartialFromSigs("fn", "", fnDef.Signatures, tail)
		return []Value{NewSplice(NewList([]Value{partial}))}, nil
	}
	return []Value{NewSplice(NewList(tail))}, nil
}

// miniSubjParam is the synthetic parameter name a mini partial binds
// its subject to — the body pushes it under the embedded standard call
// so the call's stack slot is filled exactly as an inline subject
// would fill it.
const miniSubjParam = "__mini_subject"

// miniKindExport returns the kind's wrapper FnDef from the bound
// MiniLang namespace.
func miniKindExport(r *Registry, target string) (Value, bool) {
	return moduleNamespaceExport(r, "MiniLang", target)
}

// miniPartialFn builds the partially-applied Function for a FILTER
// kind: an anonymous FnDef with one sig per wrapper overload, whose
// single parameter is the wrapper's subject slot and whose body pushes
// the subject then re-steps the expansion tail (the standard call or a
// compile hook's tokens). ok=false when the kind is not filter-shaped
// (any sig without exactly one post-prefix input), in which case the
// caller splices the plain call — today's behaviour.
func miniPartialFn(r *Registry, kind, target string, tail []Value) (Value, bool) {
	wrapper, ok := miniKindExport(r, target)
	if !ok {
		return Value{}, false
	}
	info, ok := wrapper.Data.(FnDefInfo)
	if !ok || !MiniLangFnFilterShaped(info) {
		return Value{}, false
	}
	return miniPartialFromSigs(kind, kind, info.Signatures, tail), true
}

// miniPartialFromSigs builds the partially-applied filter Function from the
// given FILTER-shaped sigs (every sig exactly [src opts subject]) — the
// shared core of the registered-kind path (miniPartialFn, kindTag = kind so
// the per-kind member types match) and the fn-VALUE form (miniFnExpand,
// kindTag = "" — an anonymous transducer has no name-keyed member type).
// label names the partial for downstream identity.
func miniPartialFromSigs(label, kindTag string, sigs []FnSig, tail []Value) Value {
	// The tail's trailing End is the bare-splice statement terminator;
	// inside a SIG BODY it instead ends the CALLER's statement when the
	// partial dispatches within an enclosing fn body (dropping pending
	// tokens like a trailing dot-chain), and the body list is already
	// self-delimiting — so strip it.
	for len(tail) > 0 && IsEnd(tail[len(tail)-1]) {
		tail = tail[:len(tail)-1]
	}
	body := append([]Value{NewWord(miniSubjParam)}, tail...)
	outSigs := make([]FnSig, 0, len(sigs))
	for _, ws := range sigs {
		outSigs = append(outSigs, FnSig{
			Params:  []FnParam{{Name: miniSubjParam, Type: ws.Params[2].Type}},
			Returns: ws.Returns,
			Impl:    Boru(body),
			// STACK-ONLY: a filter's subject comes from the stack (the
			// documented semantics), and a stack-only partial never
			// forward-collects the tokens after it — which is what lets
			// the splice drop the trailing End guard. A spliced End is
			// harmless at top level but LEAKS out of a paren group
			// (`(+re/…/) is T`, `apply (+re/…/) x`), ending the OUTER
			// word's statement and silently killing its dispatch.
			BarrierPos: 0,
		})
	}
	// Uniquely named per expansion: two partials of the same kind carry
	// different bound sources, and downstream identity keys on the name.
	miniPartialSeq++
	return NewFunction(FnDefInfo{
		Name:       fmt.Sprintf("mini-%s#%d", label, miniPartialSeq),
		Signatures: outSigs,
		Anonymous:  true,
		// The kind tag the per-kind member types (MiniLang.Re, …)
		// match on — the partial's NAMED type for fn params. Empty for
		// the fn-value form.
		MiniKind: kindTag,
	})
}

// miniPartialSeq disambiguates partial names process-wide.
var miniPartialSeq int

// (miniCompileExport / miniInvokeBoruCompile — the boru compile-hook plumbing —
// died with the frozen kind namespace: hooks are Go-only builtin machinery
// now, discovered via miniGoHook.)

// miniNamespaceBound reports whether the `MiniLang` namespace is bound to a
// module namespace in the current scope.
func miniNamespaceBound(r *Registry) bool {
	return moduleNamespaceBound(r, "MiniLang")
}

// miniKindRegistered reports whether `lang_<kind>` is an export of the bound
// MiniLang namespace.
func miniKindRegistered(r *Registry, target string) bool {
	_, ok := moduleNamespaceExport(r, "MiniLang", target)
	return ok
}

// parseHandler expands `parse <kind> <opts?> <source>` into the standard
// parser call `ParseLang.parse_<kind> <source> <opts> end`, returned as an
// __SP splice so the generated tokens re-step at the call site (the same
// mechanism as `mini`). args[0]=kind (Atom via /q — must be literal); the
// `source` is the required LAST surface arg, `opts` the optional middle one
// (2-arg form normalizes to {}). source/opts are spliced as collected — they
// may be carriers in check mode; only the kind must be concrete.
//
// A Function first operand is the ParseLang-VALUE form instead: the operand
// IS the parser (no kind lookup), and the call expands to the direct fn call
// — see parseFnExpand.
func parseHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if args[0].Parent.ConformsTo(TFunction) {
		return parseFnExpand(args[0], args, r)
	}
	kind, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, r.BoruErrorHint("parse_error",
			"parse: the kind must be a literal name", "parse",
			"write the kind as a bare word: parse calc 'x + y'")
	}
	target := "parse_" + kind

	// Resolve the kind against the imported ParseLang namespace NOW —
	// unknown kinds are expansion-time errors at the call site.
	if !parseKindRegistered(r, target) {
		// A bare word BOUND to a Function is the ParseLang-value form spelled
		// by name (`def myp (fn …)  parse myp '…'`) — mirror emit's
		// bound-variable reconstruction. A registered kind wins a collision
		// (checked above); an unbound name or a non-Function binding keeps
		// the unknown-kind handling below, so `parse nope 'x'` still raises
		// parse_unknown_lang unchanged. A name def-bound to a COMPUTED fn
		// (`def op (Parse.parser g)`) has no Defs binding under analysis —
		// installDef declines so the compiled closure machinery owns the
		// name — and resolves through the per-pass fn-carrier side table
		// (checkFnCarrierBind) instead.
		top, bound := r.Defs.Top(kind)
		if !bound || !top.Parent.ConformsTo(TFunction) {
			if v, hit := checkFnCarrierBind(r, kind); hit {
				top, bound = v, true
			} else {
				bound = false
			}
		}
		if bound {
			// The /q-captured word bypassed stepWord's use tracking, so an
			// only-used-as-a-parser fn would be falsely flagged unused_def.
			r.Check.RecordUse(kind)
			if _, isFn := top.Data.(FnDefInfo); isFn {
				return parseFnExpand(top, args, r)
			}
			if r.Check.IsActive() && !IsConcrete(top) {
				// A Function-typed carrier binding (a fn param, a computed
				// parser) under analysis: the parser is real at run time but
				// not expandable here — degrade like the other macro paths.
				// The COMPILE pass records the runtime fn dispatch against
				// the live binding so the result has provenance.
				macroDegradedAdvisory(r, "parse", "the parser fn bound to "+kind+" is not a concrete value under analysis", args[0].Pos())
				if out, ok := recordParseLangFnDispatch(r, top, args); ok {
					return []Value{out}, nil
				}
				return []Value{NewDynamicCarrier(TAny)}, nil
			}
			// A non-concrete non-carrier Function binding (the bare Function
			// type literal): not a parser — fall through to unknown-kind.
		}
		if r.Check.IsActive() && !parseNamespaceBound(r) {
			// The import may be outside the checked fragment; degrade to a
			// dynamic value rather than a false-positive diagnostic (mirror
			// of miniHandler). For the COMPILE pass record a TERMINAL trap
			// (top-level only) so the compiled program raises the byte-identical
			// parse_unknown_lang the interpreter raises here; a nested call
			// declines and keeps the lenient fallback.
			r.Check.Recorder().RecordTrap("parse_unknown_lang",
				fmt.Sprintf("parse: no parser %q is registered", kind), "parse",
				`import "boru:parselang" first; the kinds are fixed (ParseLang.kinds lists them) — pass a custom parser as a fn value: parse <fn> '…'`,
				args[0].Pos())
			macroDegradedAdvisory(r, "parse", "the boru:parselang import is outside the checked fragment", args[0].Pos())
			return []Value{NewDynamicCarrier(TAny)}, nil
		}
		return nil, r.BoruErrorHint("parse_unknown_lang",
			fmt.Sprintf("parse: no parser %q is registered", kind), "parse",
			`import "boru:parselang" first; the kinds are fixed (ParseLang.kinds lists them) — pass a custom parser as a fn value: parse <fn> '…'`)
	}

	source, opts := parseSurfaceOperands(args)
	toks := []Value{
		NewWord("ParseLang"), NewWord("dot"), NewWord(target),
		source, opts, NewEnd(),
	}
	return []Value{NewSplice(NewList(toks))}, nil
}

// parseSurfaceOperands maps the parse surface [kind|fn, opts?, source] to the
// standard parser call shape: source is the required LAST operand, opts the
// optional MIDDLE one (2-arg form normalizes to {}).
func parseSurfaceOperands(args []Value) (source, opts Value) {
	if len(args) == 3 {
		return args[2], args[1]
	}
	return args[1], NewMap(NewOrderedMap())
}

// parseFnExpand expands the ParseLang-VALUE form `parse <fn> <opts?>
// <source>` into the direct call `<fn> <source> <opts> end`, spliced at the
// call site — the token sequence the deferred dispatch replays for a
// resolved kind, with the fn value supplied by the caller instead of the
// ParseLang namespace. fn is the parser operand (args[0], or the value a
// bare word resolved to); it must be a concrete Function whose every
// signature opens with the standard [source opts] prefix — the shared
// ParseLangFnSigWhy contract — so a wrong-shaped fn fails loudly here
// rather than silently mis-collecting its operands. A Function-typed carrier
// under analysis (a computed parser) degrades to a dynamic value, exactly
// like the other not-statically-expandable macro paths.
func parseFnExpand(fn Value, args []Value, r *Registry) ([]Value, error) {
	fnDef, ok := fn.Data.(FnDefInfo)
	if !ok {
		if r.Check.IsActive() && !IsConcrete(fn) {
			macroDegradedAdvisory(r, "parse", "the parser fn is not a concrete value under analysis", args[0].Pos())
			// COMPILE pass: record the runtime fn dispatch so the result has
			// provenance and the program compiles — the fn operand's own
			// producing event is the proof. Pure check keeps the dynamic
			// degrade below.
			if out, ok := recordParseLangFnDispatch(r, fn, args); ok {
				return []Value{out}, nil
			}
			return []Value{NewDynamicCarrier(TAny)}, nil
		}
		// A fn-family value whose payload is not an FnDefInfo — defensive: the
		// sig matcher never delivers one from surface syntax (a bare `Function`
		// type literal is parented at Type and declines every parse sig), so
		// only a crafted or corrupted function value lands here.
		return nil, r.BoruErrorHint("parse_error",
			"parse: the parser is not a usable function value", "parse",
			"pass a parser fn: parse (fn [[source:String opts:Map] [Any] [...]]) 'x'")
	}
	if why := ParseLangFnSigWhy(fnDef); why != "" {
		return nil, r.BoruErrorHint("parse_bad_signature",
			"parse: "+why, "parse",
			"declare the fn as fn [[source:String opts:Map] [outputs] [body]]")
	}
	source, opts := parseSurfaceOperands(args)
	// COMPILE pass, NON-CONCRETE source (a fn-body parse over a `src:String`
	// param carrier — every voxgig-boru/template lexer): the direct expansion
	// `<fn> <source> <opts> end` dispatches the parser WORD, whose result is a
	// dynamic Any, so the compiler declines "unannotated or opaque word <fn>".
	// Route it through the SAME parselang-fn-dispatch CALL_NATIVE the
	// non-concrete-PARSER form uses: parseFnDispatchHandler replays the
	// identical tail in a sub-engine, so the compiled program is byte-identical
	// to the expansion. This is what makes a module-exported lexer compile — a
	// detached stamp binds the runtime-constructed parser as a concrete value,
	// so the non-concrete-parser branch above never fires for it. A CONCRETE
	// source (a top-level `parse op 'inc'`) keeps the expansion: the dispatch
	// yields a concrete result the compile pass folds, observation-free
	// (TestParseFnDispatchCheckObservationFree). Declines (pure check, no
	// boru:parselang import) also fall through to the expansion.
	if !IsConcrete(source) {
		if out, ok := recordParseLangFnDispatch(r, fn, args); ok {
			return []Value{out}, nil
		}
	}
	toks := []Value{fn, source, opts, NewEnd()}
	return []Value{NewSplice(NewList(toks))}, nil
}

// ParseLangFnSigWhy reports why fnDef cannot serve as a parser (a ParseLang)
// — every signature must open with the STANDARD parser prefix
// [source:(String|Any) opts:Map …] and declare exactly ONE return (a parser
// yields one result — the contract parselang-fn-dispatch enforces at run
// time, so the statically-expanded and computed forms agree) — or "" when
// it conforms. It is the ONE contract every parser surface enforces: the
// `parse` macro's fn-operand form (parseFnExpand), the compiled
// parselang-fn-dispatch, and the NewParseLangFn value constructor's callers.
func ParseLangFnSigWhy(fnDef FnDefInfo) string {
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) < 2 {
			return "every signature must start [source:String opts:Map …]"
		}
		p0 := sig.Params[0].Type
		if p0 == nil || !(p0.ConformsTo(TString) || p0.Equal(TAny)) ||
			sig.Params[1].Type == nil || !sig.Params[1].Type.ConformsTo(TMap) {
			return "every signature must start [source:String opts:Map …]"
		}
		if len(sig.Returns) != 1 {
			return "every signature must declare exactly one return (a parser yields one result)"
		}
	}
	return ""
}

// MiniLangFnSigWhy reports why fnDef cannot serve as a mini-language
// transducer — every signature must open with the standard prefix
// [src:String opts:Map …] — or "" when it conforms. It is the single
// contract shared by the `mini` macro's fn-operand form (miniFnExpand) and
// boru:minilang's MiniLang.register validation (modules/minilang.go), so
// both surfaces enforce byte-identical requirements.
func MiniLangFnSigWhy(fnDef FnDefInfo) string {
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) < 2 ||
			sig.Params[0].Type == nil || !sig.Params[0].Type.ConformsTo(TString) ||
			sig.Params[1].Type == nil || !sig.Params[1].Type.ConformsTo(TMap) {
			return "every signature must start with the standard prefix [src:String opts:Map …]"
		}
	}
	return ""
}

// MiniLangFnFilterShaped reports whether every signature is exactly the
// FILTER shape [src opts subject] — the shape whose `mini` expansion is a
// partially-applied Function (subject pending) rather than an immediate
// call. Shared by MiniLang.register's member-type mint decision and the
// fn-operand form's partial construction.
func MiniLangFnFilterShaped(fnDef FnDefInfo) bool {
	if len(fnDef.Signatures) == 0 {
		return false
	}
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) != 3 {
			return false
		}
	}
	return true
}

// EmitLangFnSigWhy reports why fnDef cannot serve as an emitter — every
// signature must be [value:Any opts:Map …] → [String …] (the first
// parameter exactly Any so the emitter accepts every value) — or "" when it
// conforms. It is the single contract shared by the `emit` macro's
// fn-operand form (emitFnExpand) and the NewEmitLangFn value constructor
// (modules/langvalue.go builds conforming values by construction).
func EmitLangFnSigWhy(fnDef FnDefInfo) string {
	for _, sig := range fnDef.Signatures {
		if len(sig.Params) < 2 {
			return "every signature must start [value:Any opts:Map …] and return a value"
		}
		p0 := sig.Params[0].Type
		if p0 == nil || !p0.Equal(TAny) {
			return "the first parameter must be value:Any so the emitter accepts every value"
		}
		p1 := sig.Params[1].Type
		if p1 == nil || !p1.ConformsTo(TMap) {
			return "every signature must start [value:Any opts:Map …] and return a String"
		}
		if len(sig.Returns) == 0 || sig.Returns[0] == nil || !sig.Returns[0].ConformsTo(TString) {
			return "every signature must return a String (emit is value→string)"
		}
	}
	return ""
}

// parseNamespaceBound reports whether the `ParseLang` namespace is bound to a
// module namespace in the current scope.
func parseNamespaceBound(r *Registry) bool {
	return moduleNamespaceBound(r, "ParseLang")
}

// capParseLangFnDispatch holds the parselang-fn-dispatch *Signature — the
// runtime resolver a compiled `parse <fn>` VALUE-form call dispatches
// through when the parser operand is not concrete under analysis (a
// computed parser, a Function-typed binding, a Parse.parser grammar
// parser). Installed once per registry when boru:parselang is built.
const capParseLangFnDispatch = "engine.parse.fn-dispatch"

// InstallParseLangFnDispatch records the parselang-side fn-dispatch
// signature so the `parse` macro's compile branch can record calls against
// it. Called from BuildParseLangModule with the importing registry.
func InstallParseLangFnDispatch(r *Registry, sig *Signature) {
	if r == nil || sig == nil {
		return
	}
	_ = r.Capabilities.Set(capParseLangFnDispatch, sig)
}

// recordParseLangFnDispatch records the ONE runtime call a compiled
// `parse <fn>` value-form dispatch lowers to when the parser operand is not
// concrete under analysis: CALL_NATIVE parselang-fn-dispatch(fn, source,
// opts) with a dynamic(Any) result registered as the event's output. The fn
// OPERAND's provenance is its own producing event — the recorded call that
// computed it — so the parse result reaches downstream consumers with
// provenance instead of declining "residual value of unknown provenance".
// Declines (pure check, suspended analysis, no boru:parselang import) leave
// the caller on the unrecorded dynamic degrade.
func recordParseLangFnDispatch(r *Registry, fn Value, args []Value) (Value, bool) {
	rec := r.Check.Recorder()
	if !r.Check.Compiling || !rec.Active() {
		return Value{}, false
	}
	sig, ok, _ := core.Cap[*Signature](r, capParseLangFnDispatch)
	if !ok || sig == nil {
		return Value{}, false
	}
	source, opts := parseSurfaceOperands(args)
	pos := fn.Pos()
	if pos.Row == 0 {
		// A COMPUTED fn value carries no source position — anchor the
		// recorded event at the source operand, which is written at the
		// `parse` call site, so a runtime raise stamps a real location.
		pos = source.Pos()
	}
	outs := []Value{NewDynamicCarrier(TAny)}
	rec.RecordCall("parselang-fn-dispatch", sig,
		[]Value{fn, source, opts}, outs, pos, true, false)
	return outs[0], true
}

// parseKindRegistered reports whether `parse_<kind>` is an export of the
// bound ParseLang namespace.
func parseKindRegistered(r *Registry, target string) bool {
	_, ok := moduleNamespaceExport(r, "ParseLang", target)
	return ok
}

// emitHandler expands `emit <kind> <opts?> <data>` (explicit) or `emit
// <opts?> <data>` (auto) into the standard emitter call `EmitLang get
// emit_<kind|auto> <data> <opts> end`, returned as an __SP splice so the
// generated tokens re-step at the call site (the same mechanism as `parse`).
//
// Position 0 is /q-captured (word→atom). Classification:
//   - leading operand is an Atom W and `emit_<W>` is registered in the bound
//     EmitLang namespace → EXPLICIT (kind=W); the remaining operands are
//     opts? then data.
//   - otherwise → AUTO (emit_auto); the leading operand is data (a /q'd bare
//     word is reconstructed as a Word so a variable evaluates) and any prior
//     operand is opts.
//
// data is always the LAST operand; opts the optional middle one. data/opts are
// spliced as collected — they may be carriers in check mode.
func emitHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// The emitter VALUE form — `emit <fn> <opts?> <data>` (sigs 4-7): the
	// first operand IS the emitter, so no kind classification happens.
	if len(args) >= 2 &&
		args[0].Parent.ConformsTo(TFunction) {
		return emitFnExpand(args[0], args, r)
	}
	// Try to read the leading operand as a kind name (a /q'd bare word).
	kind, isWord := "", false
	if a, err := args[0].AsConcreteAtom(); err == nil {
		kind, isWord = a, true
	}

	// A leading bare word with at least one operand after it is an EXPLICIT
	// kind attempt; a lone leading word is a bare-variable `emit m`.
	leadingKind := isWord && len(args) >= 2
	explicit := leadingKind && emitKindRegistered(r, "emit_"+kind)

	// A bare word BOUND to a Function (with data after it) is the emitter
	// VALUE form spelled by name (`def up (fn …)  emit up {a:1}`) — without
	// this it would fall to the auto form below and JSON-emit the fn value
	// itself. A registered kind wins a collision; a lone `emit f` stays a
	// bare-variable auto emit, unchanged. A name def-bound to a COMPUTED fn
	// has no Defs binding under analysis and resolves through the per-pass
	// fn-carrier side table (checkFnCarrierBind), like parse.
	if leadingKind && !explicit {
		top, bound := r.Defs.Top(kind)
		if !bound || !top.Parent.ConformsTo(TFunction) {
			if v, hit := checkFnCarrierBind(r, kind); hit {
				top, bound = v, true
			} else {
				bound = false
			}
		}
		if bound {
			// The /q-captured word bypassed stepWord's use tracking.
			r.Check.RecordUse(kind)
			if _, isFn := top.Data.(FnDefInfo); isFn {
				return emitFnExpand(top, args, r)
			}
			if r.Check.IsActive() && !IsConcrete(top) {
				macroDegradedAdvisory(r, "emit", "the emitter fn bound to "+kind+" is not a concrete value under analysis", args[0].Pos())
				return []Value{NewDynamicCarrier(TString)}, nil
			}
		}
	}

	// A leading bare word that is neither a registered kind nor a bound value is
	// an unknown-kind typo → loud expansion-time error (mirror parse), with the
	// check-mode dynamic-carrier fallback. A leading word that resolves to a
	// VALUE (e.g. an options map held in a variable: `def opts {…} emit opts
	// {a:1}`) is DATA/OPTS, not a kind, and falls through to the auto form.
	if leadingKind && !explicit && !r.Defs.Has(kind) {
		if r.Check.IsActive() && !emitNamespaceBound(r) {
			r.Check.Recorder().RecordTrap("emit_unknown_lang",
				fmt.Sprintf("emit: no emitter %q is registered", kind), "emit",
				`import "boru:emitlang" first; the kinds are fixed (EmitLang.kinds lists them) — pass a custom emitter as a fn value: emit <fn> <data>`,
				args[0].Pos())
			return []Value{NewDynamicCarrier(TString)}, nil
		}
		return nil, r.BoruErrorHint("emit_unknown_lang",
			fmt.Sprintf("emit: no emitter %q is registered", kind), "emit",
			`import "boru:emitlang" first; the kinds are fixed (EmitLang.kinds lists them) — pass a custom emitter as a fn value: emit <fn> <data>`)
	}

	// Reconstruct a /q'd leading bare word as a Word so the variable it names
	// (used as data in `emit m`, or as the options map in `emit opts {a:1}`)
	// evaluates at the call site rather than being spliced as a literal atom.
	lead := args[0]
	if isWord {
		lead = NewWord(kind)
	}

	target := "emit_auto"
	var data, opts Value
	if explicit {
		target = "emit_" + kind
		if len(args) == 3 {
			opts = args[1]
			data = args[2]
		} else { // len == 2
			opts = NewMap(NewOrderedMap())
			data = args[1]
		}
	} else {
		// Auto: `emit <opts?> <data>` — data is the last operand, opts the
		// optional leading one (reconstructed above when it is a bare word).
		if len(args) >= 2 {
			opts = lead
			data = args[len(args)-1]
		} else { // len == 1: the data
			opts = NewMap(NewOrderedMap())
			data = lead
		}
	}

	toks := []Value{
		NewWord("EmitLang"), NewWord("dot"), NewWord(target),
		data, opts, NewEnd(),
	}
	return []Value{NewSplice(NewList(toks))}, nil
}

// emitFnExpand expands the emitter VALUE form `emit <fn> <opts?> <data>`
// into the direct call `<fn> <data> <opts> end`, spliced at the call site —
// the fn supplied by the caller instead of the EmitLang namespace. fn must
// be a concrete Function whose every signature is [value:Any opts:Map …] →
// [String …] — the shared EmitLangFnSigWhy contract. A
// Function-typed carrier under analysis degrades to a
// dynamic String like the other not-statically-expandable macro paths.
func emitFnExpand(fn Value, args []Value, r *Registry) ([]Value, error) {
	fnDef, ok := fn.Data.(FnDefInfo)
	if !ok {
		if r.Check.IsActive() && !IsConcrete(fn) {
			macroDegradedAdvisory(r, "emit", "the emitter fn is not a concrete value under analysis", args[0].Pos())
			return []Value{NewDynamicCarrier(TString)}, nil
		}
		// A fn-family value whose payload is not an FnDefInfo — defensive:
		// the sig matcher never delivers one from surface syntax.
		return nil, r.BoruErrorHint("emit_error",
			"emit: the emitter is not a usable function value", "emit",
			"pass an emitter fn: emit (fn [[value:Any opts:Map] [String] [...]]) {a:1}")
	}
	if why := EmitLangFnSigWhy(fnDef); why != "" {
		return nil, r.BoruErrorHint("emit_bad_signature",
			"emit: "+why, "emit",
			"declare the fn as fn [[value:Any opts:Map] [String] [body]]")
	}
	var data, opts Value
	if len(args) == 3 {
		opts = args[1]
		data = args[2]
	} else {
		opts = NewMap(NewOrderedMap())
		data = args[1]
	}
	toks := []Value{fn, data, opts, NewEnd()}
	return []Value{NewSplice(NewList(toks))}, nil
}

// emitNamespaceBound reports whether the `EmitLang` namespace is bound to a
// module namespace in the current scope.
func emitNamespaceBound(r *Registry) bool {
	return moduleNamespaceBound(r, "EmitLang")
}

// emitKindRegistered reports whether `emit_<kind>` is an export of the bound
// EmitLang namespace.
func emitKindRegistered(r *Registry, target string) bool {
	_, ok := moduleNamespaceExport(r, "EmitLang", target)
	return ok
}
