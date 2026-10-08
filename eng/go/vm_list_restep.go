package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// makeListReStep runs OpMakeListReStep (NUR295): a list literal an element
// of which the interpreter's evaluation may re-step over its neighbours. With
// no such element holding a value the tape dispatches, the list assembles as
// OpMakeList's does. Otherwise the elements re-step through the interpreter
// — the list's own evaluation — where the spec proves the window
// (ListReStepSpec.Island), every other element steps as itself (a placed fn
// value is data where it sits, and the island would re-step it), and no fn
// met captures a token through a `/q` slot (the compiled window holds a
// folded word's value where the interpreter's capture takes the word);
// elsewhere the op is a designed defer, loud.
func (vc *vmContext) makeListReStep(reg *core.Registry, spec compiler.ListReStepSpec, stack []core.Value, curDebug []core.SrcPos, pc int) ([]core.Value, error) {
	if len(stack) < spec.N {
		return nil, vmErrAt(curDebug, pc, "MAKE_LIST_RESTEP stack underflow")
	}
	base := len(stack) - spec.N
	elems := append([]core.Value(nil), stack[base:]...)
	hazard, quotes, inert := false, false, true
	fn := map[int]bool{}
	tokens := append([]core.Value(nil), elems...)
	for _, i := range spec.Fn {
		fn[i] = true
		if dynBodyValueReSteps(elems[i]) {
			hazard = true
			// A fn whose `/q` slot captures the next TOKEN takes the word
			// the literal after it was written as (spec.Words, NUR219);
			// with no word noted, the folded value is all this lane has.
			// Any other fn collects the value, which the element holds —
			// the word itself may name a frame slot the island cannot see.
			if !fnValueQuotes(elems[i]) {
				continue
			}
			if w, noted := spec.Words[i+1]; noted {
				tokens[i+1] = core.WithPosAt(core.NewWord(w.Name), w.Pos)
			} else {
				quotes = true
			}
		}
	}
	for i, v := range elems {
		inert = inert && (fn[i] || !dynBodyValueReSteps(v))
	}
	if hazard {
		if !spec.Island || quotes || !inert {
			return nil, vmDefer(reg, curDebug, pc, "vm:list-restep",
				"a list literal's element is a fn value the interpreter re-steps over its neighbours, where the compiled list holds it as data (NUR295); the compiled runtime cannot execute it")
		}
		results, err := vc.islandRun(reg, tokens)
		if err != nil {
			return nil, stampAt(err, curDebug, pc, reg)
		}
		if err := vc.screenResults(results, "list element", curDebug, pc); err != nil { //covergate:allow compiler/VM defensive arm; unreachable without a bytecode-level fault (§compiler)
			return nil, err
		}
		// A copy: the island's results are its pooled engine's buffer, which
		// the next island run reuses — a loop's lists aliased the last one.
		elems = append([]core.Value(nil), results...)
	}
	// Ascription hygiene, as OpMakeList: list elements are stored data.
	for i := range elems {
		elems[i] = core.StripAscribed(elems[i])
	}
	return append(stack[:base], core.NewList(elems)), nil
}

// quotesFirstSlot reports whether every signature of fnDef quotes its first
// forward slot, so its re-step always captures the next token (NUR219).
func quotesFirstSlot(fnDef core.FnDefInfo) bool {
	sigs := fnDef.OwnSigs()
	for i := range sigs {
		if len(sigs[i].Params) == 0 || !sigs[i].Params[0].Quote || sigs[i].BarrierPos == 0 {
			return false
		}
	}
	return len(sigs) > 0
}

// anyQuotesFirstSlot reports whether some signature of fnDef quotes its
// first forward slot, so its re-step may capture the next token (NUR219).
func anyQuotesFirstSlot(fnDef core.FnDefInfo) bool {
	sigs := fnDef.OwnSigs()
	for i := range sigs {
		if len(sigs[i].Params) > 0 && sigs[i].Params[0].Quote && sigs[i].BarrierPos != 0 {
			return true
		}
	}
	return false
}

// fnValueQuotes reports whether the fn value v has a `/q` parameter in any
// of its signatures — a slot whose re-step captures the next TOKEN rather
// than its value.
func fnValueQuotes(v core.Value) bool {
	var params []core.FnParam
	switch d := v.Data.(type) {
	case core.FnDefInfo:
		for i := range d.Signatures {
			params = append(params, d.Signatures[i].Params...)
		}
	case core.ClosurePayload:
		if prog, ok := d.Prog.(*compiler.Program); ok {
			params, _ = closureSigParams(&prog.Fns[d.Unit])
		}
	}
	for _, p := range params {
		if p.Quote {
			return true
		}
	}
	return false
}
