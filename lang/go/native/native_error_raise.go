package native

import (
	"fmt"
	core "github.com/boru-lang/boru/core/go"
)

// This file holds the user-facing error words (design/legacy/ERRORS.8.ignore §2,
// retiring DX report T9.6):
//
//	raise "boom"                          code user_error
//	raise bad_input "expected a list"     explicit code (bare word ok)
//	raise {code: bad_input, message: "…", got: 42}
//
// `raise` constructs the same BoruError native handlers return, so the
// engine's existing abort/catch machinery applies unchanged: uncaught
// it formats as [boru/<code>], and `do [body] error [handler]` catches
// it as the same Ideal/Error value native errors produce. Extra spec-
// map keys ride along on the Error value (ErrorInfo.Data) for
// programmatic handlers; the formatter prints code + message only.
//
// The Error value's fields are readable in handlers via dot access /
// get: `e.code` (atom), `e.message` (string), plus any raise payload
// keys; `convert Map e` projects the same fields.
var errorNatives = []NativeFunc{
	{
		Name: "raise",
		// The /q'd error-code Atom is inert data; raise bakes + CALL_NATIVE,
		// raising the byte-identical error at run time (it declares no returns).
		// CompileDiverges marks it as an always-raising terminal, so a closure
		// body ending in `raise` (a caught `do [raise …]` / `error` handler tail)
		// compiles with no RET — the error propagates and the catcher wraps it.
		CompileEffect: CompileQuoteInert | CompileDiverges,

		Signatures: []Signature{
			// raise <code> <message> — the /q'd Atom position lets a
			// bare word name the code (raise bad_input "…").
			{
				Args:      []*Type{TAtom, TString},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(raiseCodeMessageHandler),
				Returns:   []*Type{}, ReturnsFn: raiseReturns, BarrierPos: -1,
			},
			// raise <message> — code defaults to user_error.
			{
				Args:    []*Type{TString},
				Impl:    Go(raiseMessageHandler),
				Returns: []*Type{}, ReturnsFn: raiseReturns, BarrierPos: -1,
			},
			// raise <spec> — {code:… message:…} required; remaining
			// keys are preserved on the Error value for the handler.
			{
				Args:    []*Type{TMap},
				Impl:    Go(raiseSpecHandler),
				Returns: []*Type{}, ReturnsFn: raiseReturns, BarrierPos: -1,
			},
		},
	},
	{
		// get on an Error value — code/message plus any raise payload
		// keys. Registered here (not native_storage.go) to keep the
		// whole Error surface in one file. `get` EVALUATES its key: the
		// String AND Atom overloads accept an already-evaluated key (a
		// string, or a variable bound to `code/q`), but NEITHER quotes a
		// bare word — the bare-word-quoting (QuoteArgs) variant lives on
		// `dot`. Mirrors the get/dot split in native_storage.go.
		Name:          "get",
		CompileEffect: CompileModuleFold | CompileIslandPure,

		Signatures: []Signature{
			{Args: []*Type{TString, TError}, Impl: Go(getErrorFieldHandler), ReturnsFn: errorFieldReturns, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAtom, TError}, Impl: Go(getErrorFieldHandler), ReturnsFn: errorFieldReturns, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		// dot on an Error value — the `.`-sugar target; quotes a bare-word
		// key as a literal field name (`err.code` ≡ `err dot code`).
		Name:          "dot",
		CompileEffect: CompileModuleFold | CompileIslandPure,

		Signatures: []Signature{
			{Args: []*Type{TString, TError}, Impl: Go(getErrorFieldHandler), ReturnsFn: errorFieldReturns, Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAtom, TError}, QuoteArgs: map[int]bool{0: true}, Impl: Go(getErrorFieldHandler), ReturnsFn: errorFieldReturns, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
}

// raiseReturns is the check-mode mirror of an UNCONDITIONAL raise: the
// word always raises (even malformed args raise raise_error), so a raise
// dispatched on the top-level straight line — outside every fn body
// (FnBodyDepth), nested branch/loop/quotation body (NestedBodyDepth), and
// error-catching `do` body (CaughtBodyDepth, via CheckAddUniqueDiagnostic)
// — is a program that provably errors, flagged statically. Guard idioms
// (`if (n eq 0) [raise "zero"]`) live in nested bodies and stay silent.
// The diagnostic is a RuntimeMirror (via CheckAddUniqueDiagnostic): the
// compile pass models raise as a diverging terminal (CompileDiverges) and
// the compile failure loop skips mirrors, so the row still compiles and raises
// identically. The residual model is unchanged: raise produces no value.
func raiseReturns(args []Value, r *Registry) []Value {
	if atUncaughtTopLevel(r) && len(args) > 0 {
		detail := "raise: this raise is unconditionally reached — the program always errors"
		if msg, err := args[len(args)-1].AsConcreteString(); err == nil && len(args) <= 2 {
			code := "user_error"
			if len(args) == 2 {
				if c, cerr := args[0].AsConcreteAtom(); cerr == nil {
					code = c
				}
			}
			detail = fmt.Sprintf("raise: unconditionally raises [boru/%s]: %s", code, msg)
		}
		core.CheckAddUniqueDiagnostic(r, "unconditional_raise", detail, "raise", args[0].Pos())
	}
	return []Value{}
}

func raiseMessageHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	msg, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("raise_error", "raise: message must be a concrete string", "raise")
	}
	return nil, r.BoruError("user_error", msg, "raise")
}

func raiseCodeMessageHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	code, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, r.BoruError("raise_error", "raise: code must be an atom (a bare word works: raise bad_input \"…\")", "raise")
	}
	msg, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("raise_error", "raise: message must be a concrete string", "raise")
	}
	return nil, r.BoruError(code, msg, "raise")
}

func raiseSpecHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[0], "raise")
	if err != nil {
		return nil, r.BoruError("raise_error", "raise: spec must be a concrete map", "raise")
	}
	codeVal, hasCode := m.Get("code")
	msgVal, hasMsg := m.Get("message")
	if !hasCode || !hasMsg {
		return nil, r.BoruErrorHint("raise_error",
			"raise: spec map needs both 'code' and 'message' keys", "raise",
			`hint: raise {code: bad_input, message: "expected a list"} — extra keys ride along for the handler`)
	}
	code, cerr := codeVal.AsConcreteAtom()
	if cerr != nil {
		if s, serr := codeVal.AsConcreteString(); serr == nil {
			code = s
		} else {
			return nil, r.BoruError("raise_error",
				fmt.Sprintf("raise: code must be an atom or string, got %s", codeVal.String()), "raise")
		}
	}
	msg, merr := msgVal.AsConcreteString()
	if merr != nil {
		return nil, r.BoruError("raise_error",
			fmt.Sprintf("raise: message must be a string, got %s", msgVal.String()), "raise")
	}
	var data *OrderedMap
	for _, k := range m.Keys() {
		if k == "code" || k == "message" {
			continue
		}
		if data == nil {
			data = NewOrderedMap()
		}
		v, _ := m.Get(k)
		data.Set(k, v)
	}
	ae := MakeBoruError(code, msg, "raise", r.Source, "")
	ae.Data = data
	return nil, ae
}

// errorFieldReturns narrows a literal-key read off an Error carrier to the
// field's known type — the static mirror of getErrorFieldHandler's two
// well-known keys: `code`→Atom∪None (none when the error carries no stable
// code) and `message`→String. A payload key (an arbitrary raise-spec entry)
// or a computed/absent key stays dynamic(Any). Gradual (a caught Error may
// lack the key), discharged by the runtime read; plain-check-gated so the
// compile pass keeps the prior strict-Any return byte-identical.
func errorFieldReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if r == nil || r.Check.Compiling {
		// Compile pass: reproduce the prior Returns:[TAny] EXACTLY — a declared
		// Any return is a DYNAMIC Any carrier (declaredReturnCarriers marks it so),
		// not a strict one; a strict Any would fail the downstream error dispatch
		// and decline native compilation (compiled-coverage compile failure gate).
		return dyn
	}
	if len(args) < 1 || !IsConcrete(args[0]) {
		return dyn
	}
	switch getKey(args[0]) {
	case "code":
		return []Value{NewDynamicCarrierValue(JoinCarriers(NewCarrier(TAtom), NewCarrier(TNone)))}
	case "message":
		return []Value{NewDynamicCarrier(TString)}
	}
	return dyn
}

// getErrorFieldHandler reads a field off an Error value: "code" (an
// atom; none when the error carries no stable code), "message" (the
// short description), or any key the raising spec map carried. An
// unknown key yields none, mirroring map reads.
func getErrorFieldHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := ValToString(args[0])
	ei, err := AsError(args[1])
	if err != nil {
		return nil, r.BoruError("get_error", "get: not an error value", "get")
	}
	switch key {
	case "code":
		if ei.Code == "" {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{NewAtom(ei.Code)}, nil
	case "message":
		return []Value{NewString(ei.Message)}, nil
	}
	if ei.Data != nil {
		if v, ok := ei.Data.Get(key); ok {
			return []Value{v}, nil
		}
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}
