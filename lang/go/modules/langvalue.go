package modules

import (
	"fmt"

	"github.com/boru-lang/boru/lang/go/native"
)

// The VALUE constructors — the host-side successors of the removed
// registration APIs (RegisterHostParser / RegisterFormatParser's parse
// side). The kind namespaces are FIXED, so a Go-implemented custom language
// is handed to programs as a Function VALUE: bind it to a name
// ((*Boru).DefineValue or a def), export it from a module, or pass it to the
// macro word directly. The values carry the same framework shell the
// built-in kinds get (source resolution, pure folding), wrapped as a
// trivial-delegation FnDef over a private sub-registry — the modules/wrap.go
// contract — so they dispatch exactly like a ParseLang.parse_<kind> export.

// NewParseLangFn builds a ParseLang Function VALUE from spec: the
// source-resolving shell (parseSourceShell — a String or {src:…} map
// resolves before the handler runs) over spec.Handler, with the check-time
// pure fold wired when spec.Pure. The value satisfies the `parse` word's
// [source opts] contract (ParseLangFnSigWhy), so `parse <name> '<text>'`
// works once the value is bound to <name>.
func NewParseLangFn(spec ParseLangSpec) (native.Value, error) {
	if spec.Name == "" {
		return native.Value{}, fmt.Errorf("new parser fn: Name must not be empty")
	}
	if spec.Handler == nil {
		return native.Value{}, fmt.Errorf("new parser fn %q: Handler must not be nil", spec.Name)
	}
	// A parser yields exactly one result (the ParseLangFnSigWhy contract) —
	// reject a multi-return spec here rather than building a value every
	// parse surface would decline.
	if len(spec.Returns) > 1 {
		return native.Value{}, fmt.Errorf("new parser fn %q: a parser yields exactly one result (declare at most one Return)", spec.Name)
	}
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.Value{}, err
	}
	returns := spec.Returns
	if returns == nil {
		returns = []*native.Type{native.TAny}
	}
	inner := "parselang-value-" + spec.Name
	shell := parseSourceShell(spec.Handler)
	sig := native.Signature{
		Args:       []*native.Type{native.TAny, native.TMap},
		Returns:    returns,
		BarrierPos: -1,
		Impl:       native.Go(shell),
	}
	if spec.Pure {
		sig.ReturnsFn = pureParseFoldReturns(returns, shell)
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name:       inner,
		Signatures: []native.Signature{sig},
	})
	// RegisterNativeFunc records an invalid word name instead of installing
	// (ADR-005: no panics) — surface it as this constructor's error.
	if subReg.Lookup(inner) == nil {
		return native.Value{}, fmt.Errorf("new parser fn %q: name is not a valid word (use a plain lowercase name)", spec.Name)
	}
	return wrapMiniFnDef(inner, [][]native.FnParam{
		{{Type: native.TAny}, {Type: native.TMap}},
	}, returns, nil, subReg), nil
}

// NewFormatParserFn wraps a read Format's Decode (a DecodeOpter format
// additionally honours opts) as a ParseLang Function VALUE — the value-form
// successor of the removed RegisterFormatParser's parse side. The read side
// is unchanged: register the format with native.RegisterFormat separately
// when it should also be reachable from `read`.
func NewFormatParserFn(name string, f native.Format) (native.Value, error) {
	return NewParseLangFn(ParseLangSpec{
		Name:    name,
		Returns: []*native.Type{native.TAny},
		Handler: formatParseHandler(name, f),
	})
}

// NewMiniLangFn builds a mini-language Function VALUE from spec — the
// value-form successor of the removed RegisterHostMiniLang. The standard
// [src:String opts:Map] prefix is prepended to spec.Inputs (copied
// type/quote-only so every wrapper param stays unnamed — the
// trivial-delegation dispatch contract), so the value satisfies the `mini`
// word's prefix contract (MiniLangFnSigWhy): `mini <name> '<src>'` works
// once the value is bound to <name>. Exactly one Input makes the value
// FILTER-shaped — the `mini` expansion then produces a partially-applied
// Function awaiting the subject, like the built-in filter kinds (but with
// no named member type: those are keyed to built-in kinds).
func NewMiniLangFn(spec MiniLangSpec) (native.Value, error) {
	if spec.Name == "" {
		return native.Value{}, fmt.Errorf("new minilang fn: Name must not be empty")
	}
	if spec.Handler == nil {
		return native.Value{}, fmt.Errorf("new minilang fn %q: Handler must not be nil", spec.Name)
	}
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.Value{}, err
	}
	params := make([]native.FnParam, 0, 2+len(spec.Inputs))
	params = append(params,
		native.FnParam{Type: native.TString},
		native.FnParam{Type: native.TMap})
	for _, in := range spec.Inputs {
		params = append(params, native.FnParam{Type: in.Type, Quote: in.Quote})
	}
	args := make([]*native.Type, len(params))
	// A Quote:true Input rides as QuoteArgs on BOTH the inner native
	// signature (the trivial-delegation dispatch matches against it, so
	// forward collection /q-captures a bare word there) and the wrapper —
	// the documented FnParam.Quote contract.
	var quotes map[int]bool
	for i, p := range params {
		args[i] = p.Type
		if p.Quote {
			if quotes == nil {
				quotes = map[int]bool{}
			}
			quotes[i] = true
		}
	}
	inner := "minilang-value-" + spec.Name
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: inner,
		Signatures: []native.Signature{{
			Args:       args,
			QuoteArgs:  quotes,
			Returns:    spec.Returns,
			BarrierPos: -1,
			Impl:       native.Go(spec.Handler),
		}},
	})
	// RegisterNativeFunc records an invalid word name instead of installing
	// (ADR-005: no panics) — surface it as this constructor's error.
	if subReg.Lookup(inner) == nil {
		return native.Value{}, fmt.Errorf("new minilang fn %q: name is not a valid word (use a plain lowercase name)", spec.Name)
	}
	return wrapMiniFnDef(inner, [][]native.FnParam{params}, spec.Returns, quotes, subReg), nil
}

// NewEmitLangFn builds an emitter Function VALUE from spec — the value-form
// successor of the removed RegisterHostEmitter. The value carries the
// standard [value:Any opts:Map] prefix, so it satisfies the `emit` word's
// contract (EmitLangFnSigWhy): `emit <name> <data>` works once the value is
// bound to <name>. The emitter owns its opts key set (no Options-schema
// pattern — that is a built-in-kind refinement).
func NewEmitLangFn(spec EmitLangSpec) (native.Value, error) {
	if spec.Name == "" {
		return native.Value{}, fmt.Errorf("new emitter fn: Name must not be empty")
	}
	if spec.Handler == nil {
		return native.Value{}, fmt.Errorf("new emitter fn %q: Handler must not be nil", spec.Name)
	}
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.Value{}, err
	}
	returns := spec.Returns
	if returns == nil {
		returns = []*native.Type{native.TString}
	}
	inner := "emitlang-value-" + spec.Name
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: inner,
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny, native.TMap},
			Returns:    returns,
			BarrierPos: -1,
			Impl:       native.Go(spec.Handler),
		}},
	})
	// RegisterNativeFunc records an invalid word name instead of installing
	// (ADR-005: no panics) — surface it as this constructor's error.
	if subReg.Lookup(inner) == nil {
		return native.Value{}, fmt.Errorf("new emitter fn %q: name is not a valid word (use a plain lowercase name)", spec.Name)
	}
	return wrapMiniFnDef(inner, [][]native.FnParam{
		{{Type: native.TAny}, {Type: native.TMap}},
	}, returns, nil, subReg), nil
}
