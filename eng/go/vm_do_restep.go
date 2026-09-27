package eng

import (
	"slices"
	"strconv"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// doReStep settles a `do`'s results as the interpreter's step loop does
// (SigRef.ReStep, NUR317). A native's results are spliced back at its word
// and stepped, so a placed fn value the body left dispatches at the `do`,
// over the results after it: `do [if c [g/v] [0] 5]` is 7 5 interpreted, and
// the compiled `do` handed back fn g 5 — the check pass had stepped the fn
// inside its own run of the body, so nothing recorded after the call applies
// it. Results that hold no value the step loop dispatches stand. Otherwise
// the island steps them where it sees all the interpreter's step would:
// every value it dispatches takes no argument (it fires over nothing,
// whatever lies beneath or after it), or the results are isolated — nothing
// beneath them in the frame (bare) and the unit ending after the call.
// Anywhere else a fn's collection could reach a value the island does not
// hold, and it is a designed defer, as is a re-step that leaves another count
// than the program seats (ReStepOut).
func (vc *vmContext) doReStep(reg *core.Registry, s *compiler.SigRef, results []core.Value, bare bool, code []compiler.Instr, unit int, debug []core.SrcPos, pc int) ([]core.Value, error) {
	if !slices.ContainsFunc(results, core.FnValueDispatchesAtPointer) {
		return results, nil
	}
	isolated := bare && (pc+1 >= len(code) || code[pc+1].Op == compiler.OpRet)
	if !isolated && slices.ContainsFunc(results, takesArgsAtPointer) {
		return nil, vmDefer(reg, debug, pc, "vm:do-restep", s.Word+"'s result is a fn value the interpreter re-steps where the `do` stood, over values the compiled `do` cannot hand it (NUR317); the compiled runtime cannot execute it")
	}
	out, err := runIslandResolved(reg, nil, append([]core.Value(nil), results...))
	if err != nil {
		return nil, stampAt(err, debug, pc, reg)
	}
	// In a fn unit the interpreter's re-step meets the frame's tail markers,
	// where a named fn that matched nothing raises (NUR186); the island's
	// tape simply ends, as loopExitReStep's does.
	if unit >= 0 && slices.ContainsFunc(out, namedDispatchingFn) {
		return nil, vmDefer(reg, debug, pc, "vm:do-restep", s.Word+"'s named fn result meets its fn frame's tail, where the interpreter raises and the island's tape simply ends (NUR317); the compiled runtime cannot execute it")
	}
	if s.ReStepOut >= 0 && len(out) != s.ReStepOut {
		return nil, vmDefer(reg, debug, pc, "vm:do-restep", s.Word+"'s re-stepped results are "+strconv.Itoa(len(out))+" value(s) where the program seats "+strconv.Itoa(s.ReStepOut)+" (NUR317); the compiled runtime cannot execute it")
	}
	return out, nil
}

// takesArgsAtPointer reports whether v is a value the step loop dispatches
// that may take an argument: anything it dispatches but a fn whose every
// signature takes none.
func takesArgsAtPointer(v core.Value) bool {
	if !core.FnValueDispatchesAtPointer(v) {
		return false
	}
	fd, ok := v.Data.(core.FnDefInfo)
	if !ok {
		return true
	}
	sigs := fd.OwnSigs()
	return len(sigs) == 0 || slices.ContainsFunc(sigs, func(sig core.Signature) bool { return len(sig.Params) > 0 })
}
