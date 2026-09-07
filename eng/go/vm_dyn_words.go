package eng

import (
	"github.com/boru-lang/boru/compiler/go"
	"github.com/boru-lang/boru/core/go"
)

// dynFrameWordsAt is the word table of the OpCallDynFrame at pc in the code
// that holds it (CompiledFn.DynFrameWords): the replay's token region is a
// fn body's residual, so only a unit carries one — the main code never does.
func dynFrameWordsAt(p *compiler.Program, unit, pc int) []compiler.DynFrameWord {
	if unit < 0 || p == nil || unit >= len(p.Fns) {
		return nil
	}
	return p.Fns[unit].DynFrameWords[pc]
}

// dynApplyNameAt is the head-binding name of the trailing fn-value apply at
// pc in the code that holds it (CompiledFn.DynApplyName): only a fn unit
// carries one — a main-code apply names no frame binding.
func dynApplyNameAt(p *compiler.Program, unit, pc int) compiler.DynApplyHead {
	if unit < 0 || p == nil || unit >= len(p.Fns) {
		return compiler.DynApplyHead{}
	}
	return p.Fns[unit].DynApplyName[pc]
}

// writtenRun is how many of a replayed dispatch's arguments the interpreter's
// forward window consumed: the longest leading run whose region entries are
// not bare reads. Entries beyond the table's length cannot have been marked,
// so they count as written.
func writtenRun(argWords []compiler.DynFrameWord, n int) int {
	for i := 0; i < n; i++ {
		if i < len(argWords) && argWords[i].Read {
			return i
		}
	}
	return n
}

// callDynFrameWords is the whole-frame replay for a token region carrying
// BARE READS of frame bindings (CompiledFn.DynFrameWords — NUR123).
//
// The interpreter's stepWord does not substitute a binding whose value is a
// fn: the read "goes through normal Lookup", a WORD dispatch under the
// binding name — a 0-arg fn fires, an n-arg fn collects forward from the
// tokens after it and from the frame's stack below it, a no-match raises
// `cannot call `g“. The unit pushed the slot instead, so the region on the
// stack holds the VALUE where the interpreter's tape held the WORD. Every
// word-read entry that holds a fn at run time is installed as a frame
// binding under its name (InstallFrameBinding — the interpreter's own
// param install, which re-labels the fn under the name) and re-stepped as
// that Word through the interpreter's own dispatch; a compiled closure is
// bridged to a handler-bearing fn first (closureAsWord). A word-read entry
// holding plain data at run time is what stepWord substitutes — the value
// — and stays as it is; a region with no live fn and no tape-coupled token
// is therefore the residual already and skips the island (the identity fn
// over 5 costs no interpreter run).
//
// Returns handled=false when no entry converted, leaving the caller's
// value-semantics path (the Apply kernel's frame push, or the plain island)
// to decide exactly as it did before the words existed.
func (vc *vmContext) callDynFrameWords(reg *core.Registry, words []compiler.DynFrameWord, frameBase, base int, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, bool, error) {
	region := stack[base:]
	if !replayRegionLive(region, words) {
		return stack, true, nil
	}
	prefix := append([]core.Value(nil), stack[frameBase:base]...)
	tokens := append([]core.Value(nil), region...)
	var installed []string
	var lead core.Value
	for i, w := range words {
		// A QUOTED fn is data to a value re-step, but a word read dispatches
		// the BINDING, which holds the fn whatever its quote state
		// (`f (quote ([] => [42]))` fires it interpreted).
		if w.Name == "" || i >= len(tokens) || !core.IsAppliableFn(tokens[i]) {
			continue
		}
		fnv, ok := vc.closureAsWord(reg, tokens[i])
		if !ok {
			continue
		}
		// The interpreter's Function-slot arrival delivers a quoted fn
		// UNQUOTED into the frame binding (stepWordVal's arrival path); the
		// VM's CALL_USER binds the slot as delivered, so strip it here.
		fnv.Quoted = false
		core.InstallFrameBinding(reg, w.Name, fnv)
		installed = append(installed, w.Name)
		if i == 0 {
			lead = fnv
		}
		tokens[i] = core.WithPosAt(core.NewWord(w.Name), w.Pos)
	}
	if len(installed) == 0 {
		return nil, false, nil
	}
	// The frame bindings live for the island run only: popped in reverse,
	// the exact inverse of the pushes (the interpreter's frame teardown).
	defer func() {
		for i := len(installed) - 1; i >= 0; i-- {
			reg.Defs.Pop(installed[i])
		}
	}()
	// A word-read FnDefInfo LEAD over plain data that matches nothing is the
	// interpreter's no-match, raised here through the same builder its own
	// dispatch uses (NoMatchDiag, the failing tuple in written order) — no
	// island for an answer that is an error by construction. Only the shape
	// the interpreter would decide from these tokens alone qualifies: an
	// empty prefix (nothing below the region to stack-collect) and the lead
	// at the region's foot, the one word-read entry. A matched lead that the
	// Apply kernel declined (an interpreter-only body) and every other
	// shape keep the island.
	// The diagnostic reads the INSTALLED binding (Registry.Lookup), as the
	// interpreter's own sigError does: installDef compiles a stored fn
	// value's signatures (compileFnDef resolves BarrierAllForward to the
	// param count), and the no-match's "group the call in parens" help
	// keys on that compiled barrier — a raw FnDefInfo would lose the line.
	if len(installed) == 1 && words[0].Name != "" && len(prefix) == 0 && dynFrameSimpleWindow(region) {
		if fd := reg.Lookup(words[0].Name); fd != nil && wordLeadNoMatch(lead, region[1:]) {
			args := append([]core.Value(nil), region[1:]...)
			// Only the tokens the interpreter's forward window consumed reach
			// its tuple: it stops at the first BARE READ, which the pointer
			// had already substituted onto the value stack. words[i+1].Read
			// marks arg i as such a read, so the tuple is the leading run —
			// `(g 5 y 6)` reports `[5]`, not `[5 6]` (NUR122).
			written := args[:writtenRun(words[1:], len(args))]
			return nil, true, core.NoMatchDiag(vc.r.Source, words[0].Name, fd, written, words[0].Pos, core.ReorderHintFor(words[0].Name, fd, written))
		}
	}
	results, err := runIslandResolved(reg, prefix, tokens)
	if err != nil {
		return nil, true, stampAt(err, curDebug, pc, vc.r)
	}
	if err := vc.screenResults(results, "dynamic frame result", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (the replay island's results are interpreter residuals, tape-coupled only on a compiler bug) (§compiler)
		return nil, true, err
	}
	return append(stack[:frameBase], results...), true, nil
}

// wordLeadNoMatch reports whether a word dispatch of the fn `lead` over the
// forward tokens after it matches NOTHING. The interpreter's forward
// collection takes as many tokens as a signature needs and leaves the rest
// (`[g x]` over a 0-arg g FIRES g and leaves x for the count check), so the
// question is whether ANY prefix of the tokens fills some overload — not
// whether the whole window does.
func wordLeadNoMatch(lead core.Value, args []core.Value) bool {
	for n := 0; n <= len(args); n++ {
		if core.MatchFnSig(lead, args[:n]) != nil {
			return false
		}
	}
	return true
}

// replayRegionLive reports whether a replay token region holds anything the
// interpreter's re-step would DO something with: an appliable fn at a
// word-read entry (quoted or not — the binding dispatches), an unquoted one
// elsewhere (the Apply kernel applies it), or a tape-coupled token. A region
// of plain data re-steps to itself.
func replayRegionLive(region []core.Value, words []compiler.DynFrameWord) bool {
	for i, t := range region {
		if core.IsAppliableFn(t) && (!t.Quoted || (i < len(words) && words[i].Name != "")) {
			return true
		}
	}
	return tapeCoupled(region)
}

// closureAsWord makes a fn value dispatchable by NAME through the
// interpreter's word path. A FnDefInfo already is (InstallFrameBinding
// installs its own overloads). A compiled closure (ClosurePayload) has no
// signatures the interpreter can match, so it is bridged to a FnDefInfo
// carrying ONE handler-bearing signature over the unit's declared param
// types, whose handler runs the closure on the VM (invokeClosureOn — the
// closure's own return contract applies there). Declines a closure whose
// unit this context cannot resolve.
func (vc *vmContext) closureAsWord(reg *core.Registry, v core.Value) (core.Value, bool) {
	cl, ok := v.Data.(core.ClosurePayload)
	if !ok {
		return v, true
	}
	prog := vc.p
	if fp, foreign := vc.closureProgram(cl); foreign {
		prog = fp
	}
	if cl.Unit < 0 || cl.Unit >= len(prog.Fns) {
		return v, false
	}
	body := v
	fnv, ok := closureFnDef(&prog.Fns[cl.Unit], func(args []core.Value) ([]core.Value, error) {
		return vc.invokeClosureOn(reg, body, args)
	})
	if !ok {
		return v, false
	}
	return fnv, true
}

// closureFnDef builds the FnDefInfo a compiled closure stands in for on the
// interpreter: ONE handler-bearing signature over the unit's DECLARED param
// contract (CompiledFn.Params / ParamPatterns, seated by lamParamContract
// for a lambda's unit) — the signature the interpreter's frame binding
// matches under, so a `z:Integer` lambda handed a String no-matches there —
// whose handler applies the closure through invoke. A unit that recorded no
// contract (a token body) declines: guessing Any would apply where the
// interpreter refuses. Anonymous mirrors the source fn's flag
// (CompiledFn.Lambda): it is what parks a 0-arg lambda VALUE nothing calls
// at the pointer (ADR-016's gate), so the value-path bridge parks in the
// same places the interpreter's own value does.
func closureFnDef(fn *compiler.CompiledFn, invoke func(args []core.Value) ([]core.Value, error)) (core.Value, bool) {
	if len(fn.Params) != fn.NArgs {
		return core.Value{}, false
	}
	params := make([]core.FnParam, len(fn.Params))
	for i, t := range fn.Params {
		if t == nil {
			t = core.TAny
		}
		params[i] = core.FnParam{Type: t}
		if i < len(fn.ParamPatterns) && fn.ParamPatterns[i] != nil {
			params[i].Pattern = fn.ParamPatterns[i]
		}
	}
	// All-forward as the interpreter INSTALLS it: compileFnDef resolves a
	// boru fn's BarrierAllForward to len(Params), which is what its no-match
	// diagnostic reads (HasForwardSigs — the "group the call in parens"
	// suggestion); the bridge carries the same value so the two lanes'
	// diagnostics agree line for line.
	sig := core.Signature{Params: params, BarrierPos: len(params), Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return invoke(append([]core.Value(nil), a...))
	})}
	core.NormalizeSig(&sig)
	return core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{sig}, Anonymous: fn.Lambda}), true
}
