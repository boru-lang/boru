package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// macro_fn_dispatch.go — the RUNTIME twins of the `emit` / `mini` macros'
// leading-operand dispatch, for a compiled call whose leading operand was
// not concrete under analysis (a factory's returned emitter / transducer, a
// container member read — native.recordMacroFnDispatch). The macro itself
// SPLICES tokens the interpreter re-steps; a compiled program has no tape,
// so these natives take the macro's operands in surface order and do, at
// run time, exactly what the interpreter's macro does with them:
//
//   - a fn (or a compiled closure, read through the runtime's bridge for this
//     one dispatch) is the VALUE form: the same signature contract with the
//     same error, then the fn applied to the standard [subject opts] prefix
//     — through the callback seam for a boru body, and the token run
//     `<fn> <subject> <opts> end` otherwise (the expansion itself); a mini
//     FILTER fn yields the very partial the expansion builds;
//   - anything else re-runs the macro WORD over the same operands, where
//     the interpreter's classification (a kind name, emit's auto form, the
//     literal-name error) decides.
//
// NOT exported from the namespaces: the surface stays `emit <fn> …` /
// `mini <fn> …`. The result count is the applied fn's own; the compiled call
// records it as a runtime-variadic region.

// macroFnDispatchSigs is one Any-typed signature per surface arity: the
// operands ride in surface order, so every runtime value matches and the
// handler classifies it. arg0 is read as DATA (a def-bound computed fn
// pushes rather than dispatching) and is INERT to the VM (the handler runs
// it; it is never re-stepped on the tape) — parselang-fn-dispatch's flags.
func macroFnDispatchSigs(h native.Handler, arities ...int) []native.Signature {
	sigs := make([]native.Signature, 0, len(arities))
	for _, n := range arities {
		args := make([]*native.Type, n)
		for i := range args {
			args[i] = native.TAny
		}
		sigs = append(sigs, native.Signature{
			Args:        args,
			Returns:     []*native.Type{native.TAny},
			BarrierPos:  -1,
			FnDataArgs:  map[int]bool{0: true},
			FnInertArgs: map[int]bool{0: true},
			Impl:        native.Go(h),
		})
	}
	return sigs
}

// registerMacroFnDispatch registers the dispatch native in the module's
// sub-registry and installs it into the importing registry.
func registerMacroFnDispatch(subReg, parent *native.Registry, name string, h native.Handler, install func(*native.Registry, *native.FnDefInfo), arities ...int) {
	subReg.RegisterNativeFunc(native.NativeFunc{Name: name, Signatures: macroFnDispatchSigs(h, arities...)})
	install(parent, subReg.Lookup(name))
}

// macroFnValue is the leading operand as the interpreter's macro sees it:
// ok=false for a value that is not a function at all (the word re-run
// classifies it); a compiled closure is bridged (closureFnView).
func macroFnValue(r *native.Registry, v native.Value) (native.Value, bool) {
	if !v.Parent.ConformsTo(native.TFunction) {
		return v, false
	}
	if native.IsCompiledClosure(v) {
		if bv, bridged := closureFnView(r, v); bridged {
			return bv, true
		}
	}
	return v, true
}

// rerunMacroWord re-runs the macro word over its surface operands in a
// sub-engine — the interpreter's own classification of a leading operand
// that is not a fn.
func rerunMacroWord(r *native.Registry, word string, args []native.Value) ([]native.Value, error) {
	toks := make([]native.Value, 0, len(args)+2)
	toks = append(toks, native.NewWord(word))
	toks = append(toks, args...)
	toks = append(toks, native.NewEnd())
	return native.NewTop(r).Run(toks)
}

// applyMacroFn applies a validated fn value to [subject opts]: the callback
// seam for a matched boru-bodied overload (its compiled unit when it has
// one), the expansion's token run for everything else — the operand as
// given (a closure the sub-engine bridges), not the bridge.
func applyMacroFn(r *native.Registry, operand, fnVal native.Value, fnDef native.FnDefInfo, subject, opts native.Value) ([]native.Value, error) {
	callArgs := []native.Value{subject, opts}
	if sig := native.MatchFnSig(fnVal, callArgs); sig != nil && !native.IsCompiledClosure(operand) {
		if _, boru := sig.Impl.(*native.BoruImpl); boru {
			return native.InvokeCallbackFn(r, &fnDef, sig, callArgs)
		}
	}
	return native.NewTop(r).Run([]native.Value{operand, subject, opts, native.NewEnd()})
}

// emitFnDispatchHandler is emitlang-fn-dispatch: `emit <lead> <opts?> <data>`.
func emitFnDispatchHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	fnVal, isFn := macroFnValue(r, args[0])
	if !isFn {
		return rerunMacroWord(r, "emit", args)
	}
	fnDef, ok := fnVal.Data.(native.FnDefInfo)
	if !ok {
		return nil, r.BoruErrorHint("emit_error",
			"emit: the emitter is not a usable function value", "emit",
			"pass an emitter fn: emit (fn [[value:Any opts:Map] [String] [...]]) {a:1}")
	}
	if why := native.EmitLangFnSigWhy(fnDef); why != "" {
		return nil, r.BoruErrorHint("emit_bad_signature",
			"emit: "+why, "emit",
			"declare the fn as fn [[value:Any opts:Map] [String] [body]]")
	}
	data, opts := args[len(args)-1], native.NewMap(native.NewOrderedMap())
	if len(args) == 3 {
		opts = args[1]
	}
	return applyMacroFn(r, args[0], fnVal, fnDef, data, opts)
}

// miniFnDispatchHandler is minilang-fn-dispatch: `mini <lead> <src> <opts?>`.
func miniFnDispatchHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	fnVal, isFn := macroFnValue(r, args[0])
	if !isFn {
		return rerunMacroWord(r, "mini", args)
	}
	fnDef, ok := fnVal.Data.(native.FnDefInfo)
	if !ok {
		return nil, r.BoruErrorHint("mini_error",
			"mini: the mini-language is not a usable function value", "mini",
			"pass a transducer fn: mini (fn [[src:String opts:Map] [Any] [...]]) 'text'")
	}
	if why := native.MiniLangFnSigWhy(fnDef); why != "" {
		return nil, r.BoruErrorHint("mini_bad_signature",
			"mini: "+why, "mini",
			"declare the fn as fn [[src:String opts:Map …inputs] [outputs] [body]]")
	}
	opts := native.NewMap(native.NewOrderedMap())
	if len(args) == 3 {
		opts = args[2]
	}
	if native.MiniLangFnFilterShaped(fnDef) {
		return []native.Value{native.MiniPartialFromFn(fnDef, []native.Value{args[0], args[1], opts, native.NewEnd()})}, nil
	}
	return applyMacroFn(r, args[0], fnVal, fnDef, args[1], opts)
}
